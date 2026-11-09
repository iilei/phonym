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
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/iilei/phonym"
)

const (
	familyGolden = "family_snapshot.txt"
	namesGolden  = "names_snapshot.txt"
	overflow     = "<overflow>"
	snapshotSeed = 20261109 // seed the Python reference used to draw the sample IDs
	idWidth      = 64       // hex digits in the id column (256 bits)
	minNameWidth = 24
)

// update rewrites the golden files from the Go implementation. The Python
// reference (reference/python/snapshot.py --check) must then agree as well.
var (
	update   = flag.Bool("update", false, "rewrite testdata snapshots from the Go implementation")
	proquint = regexp.MustCompile(`^[bdfghjklmnprstvz][aiou][bdfghjklmnprstvz][aiou][bdfghjklmnprstvz]$`)
)

func TestFamilySnapshot(t *testing.T) {
	var body []string
	for slot := range phonym.Slots {
		for lo := range phonym.Siblings {
			name := phonym.FamilyName(slot, lo)
			if proquint.MatchString(name) {
				t.Fatalf("family name %q is a proquint word", name)
			}
			got, err := phonym.FamilyParse(name)
			if err != nil || got != [2]int{slot, lo} {
				t.Fatalf("FamilyParse(%q) = %v, %v; want %d, %d", name, got, err, slot, lo)
			}
			body = append(body, fmt.Sprintf("%4d  %3d  %5d  %s", slot, lo, phonym.FamilyIndex(name), name))
		}
	}
	params := fmt.Sprintf("length=2 K=%d slots=%d capacity=%d stride=%d offset=%d",
		phonym.Siblings, phonym.Slots, phonym.FamilyCapacity, phonym.FamilyStride, phonym.FamilyOffset)
	got := table("phonym family code snapshot", params,
		"slot   lo  index  name", "----  ---  -----  --------", body)
	compareGolden(t, familyGolden, got)
}

func TestNamesSnapshot(t *testing.T) {
	ids := sampleIDs(t)
	type row struct {
		slot        int
		label       string
		id          *big.Int
		short, full string
	}
	var rows []row
	retired, holes := []string{}, []string{}
	releases := phonym.Releases()
	for slot, r := range releases {
		if r.Style() == nil {
			holes = append(holes, strconv.Itoa(slot))
			continue
		}
		if r.Retired {
			retired = append(retired, strconv.Itoa(slot))
		}
		for _, id := range ids {
			full := encodeChecked(t, id, r.Label, false)
			short := encodeChecked(t, id, r.Label, true)
			rows = append(rows, row{slot, r.Label, id, short, full})
		}
	}
	width := minNameWidth
	for _, r := range rows {
		width = max(width, len(r.short))
	}
	body := make([]string, 0, len(rows))
	for _, r := range rows {
		body = append(body, fmt.Sprintf("%4d  %-14s  %0*x  %-*s  %s",
			r.slot, r.label, idWidth, r.id, width, r.short, r.full))
	}
	params := fmt.Sprintf("releases=%d retired=%s placeholders=%s ids=%d K=%d seed=%d",
		len(releases), list(retired), list(holes), len(ids), phonym.Siblings, snapshotSeed)
	columns := fmt.Sprintf("slot  release         %-*s  %-*s  full", idWidth, "id", width, "short")
	rule := "----  --------------  " + strings.Repeat("-", idWidth) + "  " + strings.Repeat("-", width) +
		"  " + strings.Repeat("-", minNameWidth)
	compareGolden(t, namesGolden, table("phonym name snapshot (short and full form)", params, columns, rule, body))
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
	return name
}

// sampleIDs reads the IDs from the golden names file: the sample is data, so
// ports don't need the Python reference's random generator.
func sampleIDs(t *testing.T) []*big.Int {
	t.Helper()
	data, err := fs.ReadFile(os.DirFS("testdata"), namesGolden)
	if err != nil {
		t.Fatal(err)
	}
	var ids []*big.Int
	first := ""
	for line := range strings.SplitSeq(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 || strings.HasPrefix(line, "#") {
			continue
		}
		if first == "" {
			first = fields[1]
		}
		if fields[1] != first {
			break
		}
		id, ok := new(big.Int).SetString(fields[2], 16)
		if !ok {
			t.Fatalf("bad id %q", fields[2])
		}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		t.Fatal("no sample ids in golden file")
	}
	return ids
}

func table(title, params, columns, rule string, body []string) string {
	sum := sha256.Sum256([]byte(strings.Join(body, "\n")))
	header := []string{
		"# " + title,
		"# " + params + " sha256=" + hex.EncodeToString(sum[:])[:16],
		"#",
		"# " + columns,
		"# " + rule,
	}
	return strings.Join(append(header, body...), "\n") + "\n"
}

func compareGolden(t *testing.T, name, got string) {
	t.Helper()
	if *update {
		if err := os.WriteFile(filepath.Join("testdata", name), []byte(got), 0o600); err != nil {
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

func list(xs []string) string {
	if len(xs) == 0 {
		return "none"
	}
	return strings.Join(xs, ",")
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
