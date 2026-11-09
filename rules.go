package phonym

import (
	"fmt"
	"slices"
	"strings"
)

// Vowel harmony classes; a neutral vowel has the class "".
const (
	harmonyFront = "front"
	harmonyBack  = "back"
)

// rules1 are the rules of generation 1.
var rules1 = &rules{
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

type (
	// pair is a (coda, next onset) combination across a syllable boundary.
	pair struct{ coda, onset string }

	// parts are the pieces of one syllable.
	parts struct{ onset, nucleus, coda string }

	// syllableSet is an ordered syllable inventory with each syllable's parts.
	syllableSet struct {
		parts map[string]parts
		names []string
	}

	// rules are the generation-1 phonotactic tables shared by the released styles.
	// They are frozen: a change would rename every name of every release using them.
	rules struct {
		harmony           map[string]string // vowel -> class; "" is neutral
		voicingExceptions map[pair]bool     // voiceless coda + voiced onset, allowed anyway
		clusterBans       map[pair]bool
		nuclei            string
		voiced            string
		voiceless         string
	}
)

// inventory builds the syllables onset x nucleus x coda from single letters.
func inventory(onsets, nuclei string, codas []string) syllableSet {
	return inventoryOf(strings.Split(onsets, ""), strings.Split(nuclei, ""), codas)
}

// inventoryOf builds the syllables onset x nucleus x coda in that order, leaving
// out syllables whose onset equals their coda ("rer", "sis"). An empty onset is a
// vowel-only syllable.
func inventoryOf(onsets, nuclei, codas []string) syllableSet {
	inv := syllableSet{parts: map[string]parts{}}
	for _, o := range onsets {
		for _, n := range nuclei {
			for _, cd := range codas {
				if o != "" && o == cd {
					continue
				}
				p := parts{o, n, cd}
				s := p.onset + p.nucleus + p.coda
				inv.names = append(inv.names, s)
				inv.parts[s] = p
			}
		}
	}
	return inv
}

// checkUniqueSplit is the SPLIT RULE: every syllable but the first has a consonant
// onset, so vowel runs are nuclei and the consonants between two nuclei are coda +
// onset. That split is unique when no two (coda, onset) pairs spell the same letters.
func checkUniqueSplit(onsets, codas, nuclei []string) error {
	isVowels := func(s string) bool { return s != "" && strings.Trim(s, "aeiou") == "" }
	noVowels := func(s string) bool { return !strings.ContainsAny(s, "aeiou") }
	for _, n := range nuclei {
		if !isVowels(n) {
			return fmt.Errorf("nucleus %q is not vowels only", n)
		}
	}
	for _, x := range slices.Concat(onsets, codas) {
		if !noVowels(x) {
			return fmt.Errorf("onset or coda %q contains a vowel", x)
		}
	}
	seen := map[string]pair{}
	for _, c := range codas {
		for _, o := range onsets {
			if o == "" {
				continue // vowel-only syllables only start a word
			}
			if prev, ok := seen[c+o]; ok {
				return fmt.Errorf("ambiguous split of %q: %q+%q or %q+%q", c+o, prev.coda, prev.onset, c, o)
			}
			seen[c+o] = pair{c, o}
		}
	}
	return nil
}

// class is the harmony class of a vowel or diphthong: that of its first non-neutral vowel.
func (r *rules) class(nucleus string) string {
	for _, v := range nucleus {
		if c := r.harmony[string(v)]; c != "" {
			return c
		}
	}
	return ""
}

// harmonyClash reports whether a nucleus conflicts with a locked harmony class.
func (r *rules) harmonyClash(locked, nucleus string) bool {
	class := r.class(nucleus)
	return locked != "" && class != "" && class != locked
}

// boundaryOK checks the rules between a closed syllable (coda) and the next onset.
func (r *rules) boundaryOK(coda, onset string) bool {
	if onset == coda || r.clusterBans[pair{coda, onset}] {
		return false
	}
	if contains(r.voiced, coda) && contains(r.voiceless, onset) {
		return false
	}
	if contains(r.voiceless, coda) && contains(r.voiced, onset) && !r.voicingExceptions[pair{coda, onset}] {
		return false
	}
	return true
}

// contains reports whether s is one of the single letters in set.
func contains(set, s string) bool {
	if len(s) != 1 {
		return false
	}
	for i := range len(set) {
		if set[i] == s[0] {
			return true
		}
	}
	return false
}
