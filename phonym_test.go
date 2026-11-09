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

// title upper-cases the first letter, like a display form.
func title(w string) string { return strings.ToUpper(w[:1]) + w[1:] }

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
	if strings.Contains(short, "-") || short == strings.SplitN(mustEncode(t, id.Uint64(), soft11), "-", 2)[0] {
		t.Errorf("short form %q must be one word and carry the whole ID", short)
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
	words := strings.Split(name, "-")
	if len(words) != 3 {
		t.Fatalf("got %q, want two given words and a family name", name)
	}
	joined := words[0] + words[1] + "-" + words[2] // the inner separator dropped: not joinable
	dropped := words[1] + "-" + words[2]           // a given word missing
	for _, broken := range []string{joined, dropped} {
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
	words := strings.Split(mustEncode(t, want, soft12), "-") // two given words, then the family name
	g1, g2, fam := words[0], words[1], words[2]
	accepted := map[string]string{
		"hyphen U+2010":          g1 + "\u2010" + g2 + "\u2010" + fam,
		"non-breaking hyphen":    g1 + "\u2011" + g2 + "\u2011" + fam,
		"en dash":                g1 + "\u2013" + g2 + "\u2013" + fam,
		"em dash":                g1 + "\u2014" + g2 + "\u2014" + fam,
		"minus sign":             g1 + "\u2212" + g2 + "\u2212" + fam,
		"no-break space":         g1 + "\u00a0" + g2 + "\u00a0" + fam,
		"thin space":             g1 + "\u2009" + g2 + "\u2009" + fam,
		"narrow no-break space":  g1 + "\u202f" + g2 + "\u202f" + fam,
		"tab and doubled spaces": g1 + "\t " + g2 + "  " + fam,
		"middle dot":             g1 + "\u00b7" + g2 + "\u00b7" + fam,
		"underscore":             g1 + "_" + g2 + "_" + fam,
		"soft hyphen in a word":  g1[:3] + "\u00ad" + g1[3:] + "-" + g2[:2] + "\u00ad" + g2[2:] + "-" + fam,
		"zero-width chars":       "\ufeff" + g1 + "\u200b-" + g2[:2] + "\u200d" + g2[2:] + "-" + fam[:2] + "\u200c" + fam[2:],
		"display form":           title(g1) + "-" + title(g2) + " " + title(fam),
		"upper case":             "  " + strings.ToUpper(g1+"-"+g2+"-"+fam) + "  ",
	}
	for what, input := range accepted {
		if d, err := phonym.Decode(input, ""); err != nil || d.ID.Uint64() != want {
			t.Errorf("%s: %q -> %+v, %v", what, input, d, err)
		}
	}
	rejected := map[string]string{
		"dot (reserved)":           g1 + "." + g2 + "-" + fam,
		"plus (reserved)":          g1 + "-" + g2 + "+" + fam,
		"slash":                    g1 + "/" + g2 + "-" + fam,
		"apostrophe":               g1[:2] + "'" + g1[2:] + "-" + g2 + "-" + fam,
		"right single quote":       g1[:2] + "\u2019" + g1[2:] + "-" + g2 + "-" + fam,
		"cyrillic a":               "d\u0430" + g1[2:] + "-" + g2 + "-" + fam,
		"kelvin sign":              g1 + "-" + g2 + "-" + "\u212a" + fam[1:],
		"u with diaeresis":         g1 + "-" + g2 + "-" + fam[:1] + "\u00fc" + fam[2:],
		"combining diaeresis":      g1 + "-" + g2 + "-" + fam[:2] + "\u0308" + fam[2:],
		"digit":                    g1 + "-" + g2 + "-" + fam + "2",
		"soft hyphen as separator": g1 + "\u00ad" + g2 + "-" + fam,
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
	leftover := phonym.FamilyAt(phonym.Slots * phonym.Siblings) // capacity % Siblings names are never issued
	for _, word := range []string{"babad", "lusab", leftover} {
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
	// Output: bago-vilzav bagezar 4711 1.1.0+crisp
}

func TestBlocklist(t *testing.T) {
	t.Parallel()
	fragments := phonym.BlockedFragments()
	if len(fragments) == 0 {
		t.Fatal("blocklist 1 is empty")
	}
	for slot, r := range phonym.Releases() {
		fam := phonym.FamilyName(slot, 0)
		for _, f := range fragments {
			if d, err := phonym.Decode(f+"-"+fam, ""); err == nil {
				t.Errorf("%s: blocked %q decoded as %s", r.Label, f, d.ID)
			}
		}
	}
	for i := range phonym.FamilyCapacity {
		if name := phonym.FamilyAt(i); containsFragment(name, fragments) {
			t.Fatalf("family name %q contains a blocked fragment", name)
		}
	}
	for _, bad := range []string{"ab\n", "Fuck\n", "fu-k\n", "abc\nabc\n", "k\u00fcrv\n"} {
		if err := phonym.ParseBlocklist(bad); err == nil {
			t.Errorf("ParseBlocklist(%q) accepted", bad)
		}
	}
	if err := phonym.ParseBlocklist("# comment\n\nabc\n"); err != nil {
		t.Errorf("valid blocklist rejected: %v", err)
	}
}

func containsFragment(word string, fragments []string) bool {
	for _, f := range fragments {
		if strings.Contains(word, f) {
			return true
		}
	}
	return false
}

func TestSplitRule(t *testing.T) {
	t.Parallel()
	nuclei := []string{"a", "e", "i", "o", "u"}
	ok := []string{"b", "t", "k", "br", "tr", "kl"}
	if err := phonym.CheckUniqueSplit(ok, []string{"", "n", "r", "s"}, nuclei); err != nil {
		t.Errorf("clusters without matching codas: %v", err)
	}
	// coda t + onset r and coda "" + onset tr both spell "tr": a-tra or at-ra
	if err := phonym.CheckUniqueSplit([]string{"r", "tr"}, []string{"", "t"}, nuclei); err == nil {
		t.Error("ambiguous inventory accepted")
	}
	if err := phonym.CheckUniqueSplit([]string{"b"}, []string{""}, []string{"ab"}); err == nil {
		t.Error("consonant in a nucleus accepted")
	}
}

func TestLyricAndNordicShapes(t *testing.T) {
	t.Parallel()
	clusters := []string{"br", "dr", "gr", "kr", "pr", "tr", "fr", "bl", "gl", "kl", "pl", "fl"}
	var vowelStart, diphthongEnd int
	for i := range uint64(3000) {
		id := i*2654435761%(1<<40) + i // spread over many lengths, deterministic
		lyric, err := phonym.EncodeShort(new(big.Int).SetUint64(id), "1.3.0+lyric")
		if err != nil {
			t.Fatal(err)
		}
		for j := 1; j < len(lyric); j++ {
			if strings.ContainsRune("aeiou", rune(lyric[j])) && strings.ContainsRune("aeiou", rune(lyric[j-1])) &&
				j != len(lyric)-1 {
				t.Fatalf("lyric %q: two vowels before the end", lyric)
			}
		}
		if strings.ContainsRune("aeiou", rune(lyric[0])) {
			vowelStart++
		}
		if strings.HasSuffix(lyric, "ai") || strings.HasSuffix(lyric, "oi") {
			diphthongEnd++
		}
		nordic, err := phonym.EncodeShort(new(big.Int).SetUint64(id), "1.3.0+nordic")
		if err != nil {
			t.Fatal(err)
		}
		// b d g k p t f are never codas, so every "br", "kl", ... is an onset cluster
		n := 0
		for _, c := range clusters {
			n += strings.Count(nordic, c)
		}
		if n > 1 {
			t.Fatalf("nordic %q has %d clusters", nordic, n)
		}
	}
	if vowelStart == 0 || diphthongEnd == 0 {
		t.Errorf("lyric features never used: %d vowel starts, %d diphthong endings", vowelStart, diphthongEnd)
	}
}

func TestFantasyShapes(t *testing.T) {
	t.Parallel()
	count := func(w string, subs ...string) int {
		n := 0
		for _, s := range subs {
			n += strings.Count(w, s)
		}
		return n
	}
	for i := range uint64(3000) {
		id := new(big.Int).SetUint64(i*2654435761%(1<<40) + i)
		orkish, err := phonym.EncodeShort(id, "1.4.0+orkish")
		if err != nil {
			t.Fatal(err)
		}
		if strings.ContainsAny(orkish, "ei") {
			t.Fatalf("orkish %q uses a light vowel", orkish)
		}
		elfish, err := phonym.EncodeShort(id, "1.4.0+elfish")
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(elfish, "u") || count(elfish, "ie", "ae", "io") > 1 {
			t.Fatalf("elfish %q: u, or more than one special vowel", elfish)
		}
		alien, err := phonym.EncodeShort(id, "1.4.0+alien")
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(alien, "e") || count(alien, "zv", "kv", "tl", "kt") > 1 {
			t.Fatalf("alien %q: e, or more than one cluster", alien)
		}
	}
}
