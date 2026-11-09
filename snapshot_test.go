package phonym_test

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"math/big"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/iilei/phonym"
)

// Golden files, shared with the Python reference (reference/python/snapshot.py):
//
//	testdata/family.txt                 the frozen family code, one line per slot
//	testdata/names/<slot>-<release>.txt sample IDs per release, short and full form
//
// The registry is append-only, so a released names file never changes; a diff in
// one means a release was renamed.
const (
	familyGolden = "family.txt"
	namesDir     = "names"
	overflow     = "<overflow>"
	snapshotSeed = 20261109 // seed the Python reference used to draw the sample IDs
	minShort     = 5        // width of the "short" column header
	hugeBits     = 64       // IDs beyond this many bits get their own table
)

// update rewrites the golden files from the Go implementation. The Python
// reference (reference/python/snapshot.py --check) must then agree as well.
var (
	update   = flag.Bool("update", false, "rewrite testdata snapshots from the Go implementation")
	proquint = regexp.MustCompile(`^[bdfghjklmnprstvz][aiou][bdfghjklmnprstvz][aiou][bdfghjklmnprstvz]$`)
)

// sampleRow is one sample ID of a release: its short and full form.
type sampleRow struct {
	id          *big.Int
	short, full string
}

func TestFamilySnapshot(t *testing.T) {
	t.Parallel()
	body := make([]string, 0, phonym.Slots)
	for slot := range phonym.Slots {
		names := make([]string, phonym.Siblings)
		for lo := range phonym.Siblings {
			name := phonym.FamilyName(slot, lo)
			if proquint.MatchString(name) {
				t.Fatalf("family name %q is a proquint word", name)
			}
			got, err := phonym.FamilyParse(name)
			if err != nil || got != [2]int{slot, lo} {
				t.Fatalf("FamilyParse(%q) = %v, %v; want %d, %d", name, got, err, slot, lo)
			}
			names[lo] = name
		}
		body = append(body, fmt.Sprintf("%3d  %s", slot, strings.Join(names, " ")))
	}
	header := []string{
		"# phonym family code",
		fmt.Sprintf("# length=2 K=%d slots=%d capacity=%d stride=%d offset=%d blocklist1=%s sha256=%s",
			phonym.Siblings, phonym.Slots, phonym.FamilyCapacity, phonym.FamilyStride, phonym.FamilyOffset,
			blocklistDigest(t), digest(body)),
		"# one line per release slot: its 64 family names, sibling 0 (x mod K = 0) first",
	}
	compareGolden(t, familyGolden, strings.Join(append(header, body...), "\n")+"\n")
}

func TestNamesSnapshot(t *testing.T) {
	t.Parallel()
	ids := sampleIDs(t)
	want := map[string]bool{}
	for slot, r := range phonym.Releases() {
		if r.Style() == nil {
			continue // placeholders never issued names
		}
		name := path.Join(namesDir, fmt.Sprintf("%02d-%s.txt", slot, r.Label))
		want[name] = true
		compareGolden(t, name, renderNames(t, slot, r, ids))
	}
	entries, err := fs.ReadDir(os.DirFS("testdata"), namesDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if name := path.Join(namesDir, e.Name()); !want[name] && !*update {
			t.Errorf("%s belongs to no release", name)
		}
	}
}

// renderNames writes one release's names file: two tables, so the few huge IDs
// don't set the column widths for all the others.
func renderNames(t *testing.T, slot int, r phonym.Release, ids []*big.Int) string {
	t.Helper()
	var small, huge []sampleRow
	all := make([]string, 0, len(ids))
	for _, id := range ids {
		row := sampleRow{id, encodeChecked(t, id, r.Label, true), encodeChecked(t, id, r.Label, false)}
		all = append(all, fmt.Sprintf("%x %s %s", id, row.short, row.full))
		if id.BitLen() <= hugeBits {
			small = append(small, row)
		} else {
			huge = append(huge, row)
		}
	}
	status := ""
	if r.Retired {
		status = " retired"
	}
	out := []string{
		fmt.Sprintf("# phonym names: %s (slot %d)%s", r.Label, slot, status),
		fmt.Sprintf("# ids=%d K=%d seed=%d blocklist1=%s sha256=%s",
			len(ids), phonym.Siblings, snapshotSeed, blocklistDigest(t), digest(all)),
	}
	out = append(out, table("IDs up to 64 bits", small)...)
	out = append(out, table("IDs beyond 64 bits", huge)...)
	return strings.Join(out, "\n") + "\n"
}

// table renders rows with columns as wide as their widest entry.
func table(title string, rows []sampleRow) []string {
	idW, shortW := 2, minShort
	for _, r := range rows {
		idW = max(idW, len(r.id.Text(16)))
		shortW = max(shortW, len(r.short))
	}
	out := []string{"#", "# " + title, fmt.Sprintf("# %*s  %-*s  full", idW-2, "id", shortW, "short")}
	for _, r := range rows {
		out = append(out, fmt.Sprintf("%*s  %-*s  %s", idW, r.id.Text(16), shortW, r.short, r.full))
	}
	return out
}

// encodeChecked encodes one ID (full or short) and checks that it decodes back.
func encodeChecked(t *testing.T, id *big.Int, label string, short bool) string {
	t.Helper()
	encode, release := phonym.Encode, ""
	if short {
		encode, release = phonym.EncodeShort, label
	}
	name, err := encode(id, label, phonym.IncludeRetired())
	if errors.Is(err, phonym.ErrOverflow) {
		return overflow
	}
	if err != nil {
		t.Fatalf("encode(%s, %s): %v", id, label, err)
	}
	got, err := phonym.Decode(name, release)
	if err != nil || got.ID.Cmp(id) != 0 || got.Release != label || got.Short != short {
		t.Fatalf("Decode(%q, %q) = %+v, %v; want %s in %s", name, release, got, err, id, label)
	}
	if !short && isProquint(name) {
		t.Fatalf("full form %q is a proquint", name)
	}
	for w := range strings.SplitSeq(name, "-") {
		if nearlyBlocked(w, phonym.BlockedEntries()) {
			t.Fatalf("%q has a blocked word", name)
		}
	}
	return name
}

// sampleIDs reads the IDs from the first release's golden file: the sample is data,
// so ports don't need the Python reference's random generator.
func sampleIDs(t *testing.T) []*big.Int {
	t.Helper()
	first := phonym.Releases()[0]
	data, err := fs.ReadFile(os.DirFS("testdata"), path.Join(namesDir, "00-"+first.Label+".txt"))
	if err != nil {
		t.Fatal(err)
	}
	var ids []*big.Int
	for line := range strings.SplitSeq(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 || strings.HasPrefix(line, "#") {
			continue
		}
		id, ok := new(big.Int).SetString(fields[0], 16)
		if !ok {
			t.Fatalf("bad id %q", fields[0])
		}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		t.Fatal("no sample ids in golden file")
	}
	return ids
}

// blocklistDigest is the start of the blocklist file's sha256, recorded in the headers.
func blocklistDigest(t *testing.T) string {
	t.Helper()
	data, err := fs.ReadFile(os.DirFS("blocklists"), "1.txt")
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])[:16]
}

func digest(lines []string) string {
	sum := sha256.Sum256([]byte(strings.Join(lines, "\n")))
	return hex.EncodeToString(sum[:])[:16]
}

func compareGolden(t *testing.T, name, got string) {
	t.Helper()
	if *update {
		file := filepath.Join("testdata", filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(file), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(got), 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := fs.ReadFile(os.DirFS("testdata"), name)
	if err != nil {
		t.Fatal(err)
	}
	if got == string(want) {
		return
	}
	gotLines, wantLines := strings.Split(got, "\n"), strings.Split(string(want), "\n")
	for i := range min(len(gotLines), len(wantLines)) {
		if gotLines[i] != wantLines[i] {
			t.Fatalf("%s differs at line %d:\n got: %q\nwant: %q", name, i+1, gotLines[i], wantLines[i])
		}
	}
	t.Fatalf("%s differs in length: got %d lines, want %d", name, len(gotLines), len(wantLines))
}

func isProquint(name string) bool {
	for w := range strings.SplitSeq(name, "-") {
		if !proquint.MatchString(w) {
			return false
		}
	}
	return true
}

// TestFamilyNamesUnique checks every 2-syllable family name, issued or not, against
// the whole slot space: a name either fails to decode, or it is exactly the name of
// the slot and sibling it decodes to. Otherwise one ID would have two family names.
func TestFamilyNamesUnique(t *testing.T) {
	t.Parallel()
	for i := range phonym.FamilyCapacity {
		name := phonym.FamilyAt(i)
		ref, err := phonym.FamilyParse(name)
		if err != nil {
			continue
		}
		if back := phonym.FamilyName(ref[0], ref[1]); back != name {
			t.Fatalf("family name %q decodes to slot %d sibling %d, whose name is %q", name, ref[0], ref[1], back)
		}
	}
}
