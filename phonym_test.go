package phonym_test

import (
	"errors"
	"fmt"
	"math/big"
	"strings"
	"testing"

	"github.com/iilei/phonym"
)

const (
	soft11 = "1.1.0+soft"
	crisp  = "1.1.0+crisp"
	soft12 = "1.2.0+soft"
)

func mustEncode(t *testing.T, id uint64, release string) string {
	t.Helper()
	name, err := phonym.EncodeUint64(id, release)
	if err != nil {
		t.Fatal(err)
	}
	return name
}

func mustDecode(t *testing.T, name, release string) phonym.Decoded {
	t.Helper()
	d, err := phonym.Decode(name, release)
	if err != nil {
		t.Fatalf("Decode(%q, %q): %v", name, release, err)
	}
	return d
}

func TestRoundTripAndOrder(t *testing.T) {
	t.Parallel()
	for _, r := range phonym.Releases() {
		var prev *big.Int
		for _, id := range []uint64{0, 1, 63, 64, 65, 4711, 1 << 20, 0xfffffff} {
			name := mustEncode(t, id, r.Label)
			d := mustDecode(t, name, "")
			if d.ID.Uint64() != id || d.Release != r.Label || d.Short {
				t.Errorf("%s: %d -> %q -> %+v", r.Label, id, name, d)
			}
			if prev != nil && d.ID.Cmp(prev) <= 0 {
				t.Errorf("%s: order not preserved at %d", r.Label, id)
			}
			prev = d.ID
		}
	}
}

func TestBigIDs(t *testing.T) {
	t.Parallel()
	for _, bits := range []uint{64, 128, 256, 512} {
		id := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), bits), big.NewInt(1))
		name, err := phonym.Encode(id, soft11)
		if err != nil {
			t.Fatal(err)
		}
		if d := mustDecode(t, name, ""); d.ID.Cmp(id) != 0 {
			t.Errorf("2^%d-1 -> %q -> %s", bits, name, d.ID)
		}
	}
}

func TestNegativeAndUnknown(t *testing.T) {
	t.Parallel()
	if _, err := phonym.Encode(big.NewInt(-1), soft11); !errors.Is(err, phonym.ErrOverflow) {
		t.Errorf("negative id: %v", err)
	}
	if _, err := phonym.EncodeUint64(1, "9.9.9+none"); !errors.Is(err, phonym.ErrUnknownRelease) {
		t.Errorf("unknown release: %v", err)
	}
}

func TestShortForm(t *testing.T) {
	t.Parallel()
	id := big.NewInt(0x62e6b43)
	short, err := phonym.EncodeShort(id, soft11)
	if err != nil {
		t.Fatal(err)
	}
	if short != "muruboganva" {
		t.Errorf("short form = %q", short)
	}
	if d := mustDecode(t, short, soft11); d.ID.Cmp(id) != 0 || !d.Short {
		t.Errorf("decode short: %+v", d)
	}
	if _, err := phonym.Decode(short, ""); !errors.Is(err, phonym.ErrInvalidName) {
		t.Errorf("short form without release: %v", err)
	}
	full := mustEncode(t, id.Uint64(), crisp)
	if d := mustDecode(t, full, soft11); d.Release != crisp || d.Short {
		t.Errorf("full form must ignore the context release: %+v", d)
	}
	if _, err := phonym.EncodeShort(big.NewInt(1<<23), soft12); !errors.Is(err, phonym.ErrOverflow) {
		t.Errorf("split short capacity: %v", err)
	}
}

func TestSplitRelease(t *testing.T) {
	t.Parallel()
	name := mustEncode(t, 0xfffffff, soft12)
	if name != "danla-gilmi-worfov" {
		t.Errorf("got %q", name)
	}
	for _, broken := range []string{"danlagilmi-worfov", "gilmi-worfov"} {
		if _, err := phonym.Decode(broken, ""); !errors.Is(err, phonym.ErrInvalidName) {
			t.Errorf("%q: %v", broken, err)
		}
	}
	if _, err := phonym.EncodeUint64(1<<29, soft12); !errors.Is(err, phonym.ErrOverflow) {
		t.Errorf("split capacity: %v", err)
	}
}

func TestInputRule(t *testing.T) {
	t.Parallel()
	const want = 0xfffffff
	fam := strings.TrimPrefix(mustEncode(t, want, soft12), "danla-gilmi-")
	accepted := map[string]string{
		"hyphen U+2010":          "danla\u2010gilmi\u2010" + fam,
		"non-breaking hyphen":    "danla\u2011gilmi\u2011" + fam,
		"en dash":                "danla\u2013gilmi\u2013" + fam,
		"em dash":                "danla\u2014gilmi\u2014" + fam,
		"minus sign":             "danla\u2212gilmi\u2212" + fam,
		"no-break space":         "danla\u00a0gilmi\u00a0" + fam,
		"thin space":             "danla\u2009gilmi\u2009" + fam,
		"narrow no-break space":  "danla\u202fgilmi\u202f" + fam,
		"tab and doubled spaces": "danla\t gilmi  " + fam,
		"middle dot":             "danla\u00b7gilmi\u00b7" + fam,
		"underscore":             "danla_gilmi_" + fam,
		"soft hyphen in a word":  "dan\u00adla-gil\u00admi-" + fam,
		"zero-width chars":       "\ufeffdanla\u200b-gil\u200dmi-" + fam[:2] + "\u200c" + fam[2:],
		"display form":           "Danla-Gilmi " + strings.ToUpper(fam[:1]) + fam[1:],
		"upper case":             "  DANLA-GILMI-" + strings.ToUpper(fam) + "  ",
	}
	for what, input := range accepted {
		if d, err := phonym.Decode(input, ""); err != nil || d.ID.Uint64() != want {
			t.Errorf("%s: %q -> %+v, %v", what, input, d, err)
		}
	}
	rejected := map[string]string{
		"dot (reserved)":           "danla.gilmi-" + fam,
		"plus (reserved)":          "danla-gilmi+" + fam,
		"slash":                    "danla/gilmi-" + fam,
		"apostrophe":               "dan'la-gilmi-" + fam,
		"right single quote":       "dan\u2019la-gilmi-" + fam,
		"cyrillic a":               "d\u0430nla-gilmi-" + fam,
		"kelvin sign":              "danla-gilmi-" + "\u212a" + fam[1:],
		"u with diaeresis":         "danla-gilmi-" + fam[:1] + "\u00fc" + fam[2:],
		"combining diaeresis":      "danla-gilmi-" + fam[:2] + "\u0308" + fam[2:],
		"digit":                    "danla-gilmi-" + fam + "2",
		"soft hyphen as separator": "danla\u00adgilmi-" + fam,
		"family name only":         fam,
		"proquint":                 "lusab-babad",
		"empty":                    " - ",
	}
	for what, input := range rejected {
		if _, err := phonym.Decode(input, ""); !errors.Is(err, phonym.ErrInvalidName) {
			t.Errorf("%s: %q: want ErrInvalidName, got %v", what, input, err)
		}
	}
}

func TestProquintFamilyShape(t *testing.T) {
	t.Parallel()
	for _, word := range []string{"babad", "lusab", "wuvnas"} { // proquints, and a leftover name
		if _, err := phonym.FamilyParse(word); !errors.Is(err, phonym.ErrUnsupportedFormat) {
			t.Errorf("%q: %v", word, err)
		}
	}
}

func TestJoinRule(t *testing.T) {
	soft := phonym.WithJoinable(phonym.Soft12)
	single := phonym.WithJoinable(phonym.Soft10)
	trial := append(phonym.Releases(),
		phonym.NewRelease("1.3.0+soft.joinable", soft, false),
		phonym.NewRelease("1.3.0+softsingle.joinable", single, false))
	phonym.WithRegistry(trial, func() {
		name, err := phonym.EncodeUint64(0xfffffff, "1.3.0+soft.joinable")
		if err != nil {
			t.Fatal(err)
		}
		given, fam, _ := strings.Cut(name, "-")
		given2, fam, _ := strings.Cut(fam, "-")
		for _, form := range []string{given + given2, given[:3] + "-" + given[3:] + given2} {
			if d := mustDecode(t, form+"-"+fam, ""); d.ID.Uint64() != 0xfffffff {
				t.Errorf("joinable %q -> %s", form, d.ID)
			}
		}
		long, err := phonym.Encode(new(big.Int).Lsh(big.NewInt(1), 64), "1.3.0+softsingle.joinable")
		if err != nil {
			t.Fatal(err)
		}
		word, fam, _ := strings.Cut(long, "-")
		chunks := []string{}
		for i := 0; i < len(word); i += 3 {
			chunks = append(chunks, word[i:min(i+3, len(word))])
		}
		want := new(big.Int).Lsh(big.NewInt(1), 64)
		if d := mustDecode(t, strings.Join(chunks, "-")+"-"+fam, ""); d.ID.Cmp(want) != 0 {
			t.Errorf("single-word joinable: %s", d.ID)
		}
	})
}

func TestRetirement(t *testing.T) {
	current := phonym.Releases()
	oldName := mustEncode(t, 0x62e6b43, soft11)
	trial := append([]phonym.Release{}, current...)
	trial[3] = phonym.NewRelease(soft11, phonym.Soft11, true)
	trial = append(trial, phonym.Placeholder(), phonym.NewRelease("1.3.0+crisp", phonym.Crisp11, false))
	if err := phonym.ValidateEvolution(current, trial); err != nil {
		t.Fatal(err)
	}
	phonym.WithRegistry(trial, func() {
		if d := mustDecode(t, oldName, ""); d.ID.Uint64() != 0x62e6b43 || d.Release != soft11 {
			t.Errorf("retired release must still decode: %+v", d)
		}
		if _, err := phonym.EncodeUint64(1, soft11); !errors.Is(err, phonym.ErrRetiredRelease) {
			t.Errorf("retired release issued a name: %v", err)
		}
		hole := "ba-" + phonym.FamilyName(len(current), 0)
		if _, err := phonym.Decode(hole, ""); !errors.Is(err, phonym.ErrInvalidName) {
			t.Errorf("placeholder slot: %v", err)
		}
		newName, err := phonym.EncodeUint64(0x62e6b43, "1.3.0+crisp")
		if err != nil {
			t.Fatal(err)
		}
		if d := mustDecode(t, newName, ""); d.ID.Uint64() != 0x62e6b43 || d.Release != "1.3.0+crisp" {
			t.Errorf("re-issued name: %+v", d)
		}
	})
	if _, err := phonym.Decode(
		"ba-"+phonym.FamilyName(len(current), 0),
		"",
	); !errors.Is(
		err,
		phonym.ErrUnsupportedRelease,
	) {
		t.Errorf("unknown slot after restore: %v", err)
	}
}

func TestRegistryChecks(t *testing.T) {
	t.Parallel()
	current := phonym.Releases()
	plus := func(r phonym.Release) []phonym.Release { return append(append([]phonym.Release{}, current...), r) }
	bad := map[string][]phonym.Release{
		"duplicate label":  plus(phonym.NewRelease(soft11, phonym.Soft11, false)),
		"missing +build":   plus(phonym.NewRelease("1.4.0", phonym.Soft11, false)),
		"not semver":       plus(phonym.NewRelease("v1.4.0+soft", phonym.Soft11, false)),
		"major 2":          plus(phonym.NewRelease("2.0.0+soft", phonym.Soft11, false)),
		"live placeholder": plus(phonym.NewRelease("", nil, false)),
	}
	for what, reg := range bad {
		if err := phonym.ValidateRegistry(reg); err == nil {
			t.Errorf("%s: accepted", what)
		}
	}
	retired := append([]phonym.Release{}, current...)
	retired[3] = phonym.NewRelease(soft11, phonym.Soft11, true)
	evolutions := map[string][2][]phonym.Release{
		"entry removed": {current, current[:len(current)-1]},
		"style edited":  {current, replaceAt(current, 3, phonym.NewRelease(soft11, phonym.Soft10, false))},
		"label edited":  {current, replaceAt(current, 3, phonym.NewRelease("1.1.1+soft", phonym.Soft11, false))},
		"un-retired":    {retired, current},
	}
	for what, pair := range evolutions {
		if err := phonym.ValidateEvolution(pair[0], pair[1]); err == nil {
			t.Errorf("%s: accepted", what)
		}
	}
}

func replaceAt(reg []phonym.Release, i int, r phonym.Release) []phonym.Release {
	out := append([]phonym.Release{}, reg...)
	out[i] = r
	return out
}

func ExampleEncode() {
	name, _ := phonym.EncodeUint64(4711, "1.1.0+crisp")
	short, _ := phonym.EncodeShort(big.NewInt(4711), "1.1.0+crisp")
	d, _ := phonym.Decode(name, "")
	fmt.Println(name, short, d.ID, d.Release)
	// Output: bago-voslav bagando 4711 1.1.0+crisp
}
