package phonym_test

import (
	"math/big"
	"strings"
	"testing"

	"github.com/iilei/phonym"
)

// FuzzRoundTrip: every ID encodes in every release and decodes back, in full and short form.
func FuzzRoundTrip(f *testing.F) {
	for _, seed := range [][]byte{{}, {0}, {1}, {0x40}, {0xff, 0xff, 0xff, 0x0f}, make([]byte, 32)} {
		f.Add(seed, uint8(0))
	}
	releases := phonym.Releases()
	f.Fuzz(func(t *testing.T, raw []byte, pick uint8) {
		if len(raw) > 64 { // 512 bits is plenty; longer inputs only slow the fuzzer down
			raw = raw[:64]
		}
		id := new(big.Int).SetBytes(raw)
		label := releases[int(pick)%len(releases)].Label
		for _, short := range []bool{false, true} {
			encode, release := phonym.Encode, ""
			if short {
				encode, release = phonym.EncodeShort, label
			}
			name, err := encode(id, label)
			if err != nil {
				continue // fixed-shape releases overflow; covered by the unit tests
			}
			d, err := phonym.Decode(name, release)
			if err != nil {
				t.Fatalf("Decode(%q, %q): %v", name, release, err)
			}
			if d.ID.Cmp(id) != 0 || d.Release != label || d.Short != short {
				t.Fatalf("%s in %s: %q decoded to %+v", id, label, name, d)
			}
		}
	})
}

// FuzzDecode: Decode never panics, and an accepted name is the only name of its ID.
// In a strict (non-joinable) release, the normalized input must equal the canonical
// encoding; anything else would be a second name for the same ID.
func FuzzDecode(f *testing.F) {
	for _, seed := range []string{
		"badil-durjav", "Badil Durjav", "bago-voslav", "danla-gilmi-worfov", "bajoba", "lusab-babad",
		"wuvnas", "ba-wuvnas", "", "-", "\u00ad", "ba\u2013durkus", "\u212aaba-durkus",
	} {
		f.Add(seed, uint8(0))
	}
	releases := phonym.Releases()
	joinable := map[string]bool{}
	for _, r := range releases {
		if r.Style() != nil && r.Style().Joinable {
			joinable[r.Label] = true
		}
	}
	f.Fuzz(func(t *testing.T, input string, pick uint8) {
		label := releases[int(pick)%len(releases)].Label
		for _, release := range []string{"", label} {
			d, err := phonym.Decode(input, release)
			if err != nil {
				continue
			}
			encode := phonym.Encode
			if d.Short {
				encode = phonym.EncodeShort
			}
			canonical, err := encode(d.ID, d.Release, phonym.IncludeRetired())
			if err != nil {
				t.Fatalf("%q decoded to %s in %s, which does not encode: %v", input, d.ID, d.Release, err)
			}
			words, err := phonym.Normalize(input)
			if err != nil {
				t.Fatalf("%q decoded but does not normalize: %v", input, err)
			}
			got, want := strings.Join(words, "-"), canonical
			if joinable[d.Release] { // JOIN RULE: separators inside the given name don't count
				got, want = strings.ReplaceAll(got, "-", ""), strings.ReplaceAll(want, "-", "")
			}
			if got != want {
				t.Fatalf("%q (normalized %q) decodes to %s in %s, whose name is %q: two names for one ID",
					input, got, d.ID, d.Release, canonical)
			}
		}
	})
}
