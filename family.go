package phonym

import (
	_ "embed" // the family table
	"fmt"
	"math/big"
	"strings"
	"sync"
)

// Frozen family code of generation 1. Everything here is a frozen copy and must
// never follow changes elsewhere, or every family name would be renumbered.
const (
	// Generation is the family-name generation; it equals the semver major of every release.
	Generation = 1
	// Siblings (K) is the number of IDs that share a given name and differ in the family name.
	Siblings = 64
	// Slots is the number of release slots the family code provides.
	Slots = 384

	familyLen      = 2
	familyCapacity = 24627
	familyStride   = 239 // round(Slots * 0.6180339887) = 237, then the next value coprime to Slots
	familyOffset   = 97
	familyOnsets   = "bdfgklmnprstvzhjw"
)

var (
	familyCodas    = []string{"", "n", "r", "s", "l", "v"}
	familySuffixes = []string{"en", "in", "ov", "av", "as", "us", "ul"}
	familyRules    = &rules{
		nuclei: "aeiou",
		harmony: map[string]string{
			"e": harmonyFront, "i": harmonyFront, "o": harmonyBack, "u": harmonyBack, "a": "",
		},
		voiced:            "bdgvz",
		voiceless:         "ptkfsh",
		voicingExceptions: map[pair]bool{{"s", "b"}: true, {"s", "d"}: true, {"s", "g"}: true},
		clusterBans: map[pair]bool{
			{"s", "z"}: true, {"n", "m"}: true, {"r", "l"}: true, {"l", "r"}: true,
		},
	}

	// familyBlocklist is checked once, when the family code is frozen.
	familyBlocklist = blocklist1

	// familyNamesText is the frozen family code as a table: the Slots*Siblings issued
	// names in index order (spread slot * Siblings + sibling). It is generated from the
	// family automaton below and checked against it by the tests.
	//go:embed familynames.txt
	familyNamesText string

	familyAutomatonOnce sync.Once
	familyAutomatonCode *code

	familyEndOnce sync.Once
	familyEnd     map[string]bool

	familyTableOnce  sync.Once
	familyTableNames []string
	familyTableIndex map[string]int
	familyInvert     int // familyStride^-1 mod Slots
)

// familyRef is a decoded family name: release slot and sibling (the ID's low bits).
type familyRef struct{ slot, lo int }

// familyEndings returns the syllables that may end a family name.
func familyEndings() map[string]bool {
	familyEndOnce.Do(func() {
		inv := inventory(familyOnsets, familyRules.nuclei, familyCodas)
		familyEnd = map[string]bool{}
		for _, s := range inv.names {
			for _, suf := range familySuffixes {
				if s[1:] == suf {
					familyEnd[s] = true
				}
			}
		}
	})
	return familyEnd
}

// familyAutomaton builds the family automaton, which generates the family table.
// Encoding and decoding use the table; the automaton is only used to build and
// check it.
func familyAutomaton() *code {
	familyAutomatonOnce.Do(func() {
		inv := inventory(familyOnsets, familyRules.nuclei, familyCodas)
		familyAutomatonCode = newCode(inv, familyStepper(inv), familyAllower(inv))
	})
	return familyAutomatonCode
}

// familyTable loads the embedded family table once.
func familyTable() ([]string, map[string]int) {
	familyTableOnce.Do(func() {
		for line := range strings.SplitSeq(familyNamesText, "\n") {
			if line != "" && !strings.HasPrefix(line, "#") {
				familyTableNames = append(familyTableNames, line)
			}
		}
		if len(familyTableNames) != Slots*Siblings {
			panic(fmt.Sprintf("phonym: family table has %d names, want %d", len(familyTableNames), Slots*Siblings))
		}
		familyTableIndex = make(map[string]int, len(familyTableNames))
		for i, name := range familyTableNames {
			familyTableIndex[name] = i
		}
		familyInvert = int(new(big.Int).ModInverse(big.NewInt(familyStride), big.NewInt(Slots)).Int64())
	})
	return familyTableNames, familyTableIndex
}

// familyStepper returns the family code's transition function.
func familyStepper(inv syllableSet) stepFunc {
	return func(q state, syl string) state {
		p := inv.parts[syl]
		h := q.harmony
		if h == "" {
			h = familyRules.class(p.nucleus)
		}
		head := familyBlocklist.advanceHead(q.head, syl)
		return state{coda: p.coda, harmony: h, onset: p.onset, head: head}
	}
}

// familyAllower returns the family code's rule check. SHAPE RULE: CVC-CVC. The first
// syllable must be closed; "no two closed syllables in a row" does not apply here.
func familyAllower(inv syllableSet) allowFunc {
	return func(q state, syl string, last bool) bool {
		p := inv.parts[syl]
		switch {
		case q.coda == "^" && p.coda == "":
			return false
		case last && !familyEndings()[syl]:
			return false
		case last && familyBlocklist.wordBlocked(q.head, syl):
			return false
		case familyRules.harmonyClash(q.harmony, p.nucleus):
			return false
		case p.onset == q.onset:
			return false
		case q.coda == "^" || q.coda == "":
			return true
		}
		return familyRules.boundaryOK(q.coda, p.onset)
	}
}

// familyName returns the family name of a release slot and sibling (0 <= lo < Siblings).
func familyName(slot, lo int) string {
	names, _ := familyTable()
	spread := (slot*familyStride + familyOffset) % Slots
	return names[spread*Siblings+lo]
}

// familyParse decodes a family name into its release slot and sibling.
func familyParse(word string) (familyRef, error) {
	if !familyShape(word) {
		return familyRef{}, fmt.Errorf("%w: %q is not a generation-%d family name (CVC-CVC, 6 letters)",
			ErrUnsupportedFormat, word, Generation)
	}
	_, index := familyTable()
	i, ok := index[word] // the familyCapacity % Siblings leftover names are not in the table
	if !ok {
		return familyRef{}, fmt.Errorf("%w: %q is not an issued family name", ErrUnsupportedFormat, word)
	}
	spread := i / Siblings
	slot := ((spread-familyOffset)%Slots + Slots) % Slots * familyInvert % Slots
	return familyRef{slot: slot, lo: i % Siblings}, nil
}

// familyShape checks the SHAPE RULE on letters only: the pattern CVCCVC
// (C consonant, V vowel). A proquint word (5 letters) can never match.
func familyShape(word string) bool {
	const pattern = "CVCCVC"
	if len(word) != len(pattern) {
		return false
	}
	for i := range len(pattern) {
		isVowel := strings.IndexByte("aeiou", word[i]) >= 0
		if isVowel != (pattern[i] == 'V') || word[i] < 'a' || word[i] > 'z' {
			return false
		}
	}
	return true
}
