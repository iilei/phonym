package phonym

import (
	"errors"
	"fmt"
	"math/big"
	"slices"
	"strings"
)

// errStyle is returned when a registry entry's style fails to build.
var errStyle = errors.New("invalid style")

// Style defines how given names sound and are shaped. Released styles are frozen:
// a change needs a new release.
type Style struct {
	rules *rules

	// Name is the style's name, used in release labels ("soft").
	Name string
	// Onsets are the onset consonants, in order; the order fixes the order of names.
	Onsets string
	// Codas are the allowed codas, in order; "" is an open syllable.
	Codas []string
	// EndSuffixes are the vowel+coda suffixes allowed on the last syllable of a word.
	EndSuffixes []string
	// Harmony enables vowel harmony (e i front, o u back, a neutral).
	Harmony bool
	// Split is the shape of the given name in syllables per word. Empty means one
	// word of variable length; [2 2] means two words of exactly 2 syllables each,
	// combined like digits: value = word1 * capacity(2) + word2.
	Split []int
	// Joinable makes decoding ignore separators inside the given name (JOIN RULE).
	Joinable bool
}

// equal reports whether two styles produce the same names.
func (s *Style) equal(o *Style) bool {
	if s == nil || o == nil {
		return s == o
	}
	return s.rules == o.rules && s.Name == o.Name && s.Onsets == o.Onsets &&
		slices.Equal(s.Codas, o.Codas) && slices.Equal(sortedCopy(s.EndSuffixes), sortedCopy(o.EndSuffixes)) &&
		s.Harmony == o.Harmony && slices.Equal(s.Split, o.Split) && s.Joinable == o.Joinable
}

// build creates the style's automaton and checks its endings.
func (s *Style) build() (*code, error) {
	inv := inventory(s.Onsets, s.rules.nuclei, s.Codas)
	end, err := s.endings(inv)
	if err != nil {
		return nil, err
	}
	return newCode(inv, s.stepper(inv), s.allower(inv, end)), nil
}

// endings returns the syllables that may close a word, and checks them: every
// suffix must match a syllable, and no ending may be a family ending.
func (s *Style) endings(inv syllableSet) (map[string]bool, error) {
	end := map[string]bool{}
	var dead []string
	for _, suf := range s.EndSuffixes {
		matched := false
		for _, syl := range inv.names {
			if syl[1:] == suf {
				end[syl] = true
				matched = true
			}
		}
		if !matched {
			dead = append(dead, suf)
		}
	}
	if len(dead) > 0 {
		return nil, fmt.Errorf("style %s: suffixes matching no syllable: %v", s.Name, dead)
	}
	family()
	for syl := range end {
		if familyEnd[syl] {
			return nil, fmt.Errorf("style %s: ending %q overlaps the family endings", s.Name, syl)
		}
	}
	return end, nil
}

// stepper returns the style's transition function.
func (s *Style) stepper(inv syllableSet) stepFunc {
	return func(q state, syl string) state {
		p := inv.parts[syl]
		h := ""
		if s.Harmony {
			h = q.harmony
			if h == "" {
				h = s.rules.harmony[p.nucleus]
			}
		}
		return state{coda: p.coda, harmony: h, onset: p.onset}
	}
}

// allower returns the style's rule check for a syllable after state q.
func (s *Style) allower(inv syllableSet, end map[string]bool) allowFunc {
	return func(q state, syl string, last bool) bool {
		p := inv.parts[syl]
		switch {
		case last && !end[syl]:
			return false
		case s.Harmony && s.rules.harmonyClash(q.harmony, p.nucleus):
			return false
		case p.onset == q.onset:
			return false
		case q.coda == "^" || q.coda == "":
			return true
		case p.coda != "": // no two closed syllables in a row
			return false
		}
		return s.rules.boundaryOK(q.coda, p.onset)
	}
}

// givenCapacity is the number of given names of a split style; nil means unlimited.
func givenCapacity(s *Style, c *code) *big.Int {
	if len(s.Split) == 0 {
		return nil
	}
	total := big.NewInt(1)
	for _, k := range s.Split {
		total.Mul(total, c.capacity(k))
	}
	return total
}

// encodeGiven encodes the given name(s) for value x.
func encodeGiven(s *Style, c *code, x *big.Int) (string, error) {
	if len(s.Split) == 0 {
		return c.encodeVar(x)
	}
	if capacity := givenCapacity(s, c); x.Sign() < 0 || x.Cmp(capacity) >= 0 {
		return "", fmt.Errorf("%w: split %v holds %s given names", ErrOverflow, s.Split, capacity)
	}
	rest := new(big.Int).Set(x)
	words := make([]string, len(s.Split))
	digit := new(big.Int)
	for i := len(s.Split) - 1; i >= 0; i-- { // mixed radix, first word most significant
		rest.DivMod(rest, c.capacity(s.Split[i]), digit)
		syls, err := c.unrank(digit, s.Split[i])
		if err != nil {
			return "", err
		}
		words[i] = strings.Join(syls, "")
	}
	return strings.Join(words, "-"), nil
}

// decodeGiven decodes the given words (separators already removed by normalize).
func decodeGiven(s *Style, c *code, words []string) (*big.Int, error) {
	words, err := canonicalWords(s, c, words)
	if err != nil {
		return nil, err
	}
	if want := max(1, len(s.Split)); len(words) != want {
		return nil, fmt.Errorf("%w: expected %d given word(s), got %d", ErrInvalidName, want, len(words))
	}
	if len(s.Split) == 0 {
		return c.decodeVar(words[0])
	}
	x := new(big.Int)
	for i, w := range words {
		syls, err := c.parse(w)
		if err != nil {
			return nil, err
		}
		if len(syls) != s.Split[i] {
			return nil, fmt.Errorf("%w: %q has %d syllables, expected %d", ErrInvalidName, w, len(syls), s.Split[i])
		}
		r, err := c.rank(syls)
		if err != nil {
			return nil, err
		}
		x.Mul(x, c.capacity(s.Split[i])).Add(x, r)
	}
	return x, nil
}

// canonicalWords applies the JOIN RULE: for joinable styles, separators inside
// the given name are ignored and the words are cut again at the syllable counts.
func canonicalWords(s *Style, c *code, words []string) ([]string, error) {
	if !s.Joinable {
		return words, nil
	}
	joined := strings.Join(words, "")
	if len(s.Split) == 0 {
		return []string{joined}, nil
	}
	cuts := resplit(c, joined, s.Split)
	if len(cuts) != 1 {
		return nil, fmt.Errorf("%w: %q splits %d ways into %v", ErrInvalidName, joined, len(cuts), s.Split)
	}
	return cuts[0], nil
}

// resplit returns all ways to cut a word into consecutive words of the given syllable counts.
func resplit(c *code, joined string, split []int) [][]string {
	if len(split) == 0 {
		if joined == "" {
			return [][]string{{}}
		}
		return nil
	}
	var found [][]string
	for cut := 1; cut <= len(joined); cut++ {
		syls, err := c.parse(joined[:cut])
		if err != nil || len(syls) != split[0] {
			continue
		}
		for _, tail := range resplit(c, joined[cut:], split[1:]) {
			found = append(found, append([]string{joined[:cut]}, tail...))
		}
	}
	return found
}

// sortedCopy returns a sorted copy of a string slice.
func sortedCopy(xs []string) []string {
	out := slices.Clone(xs)
	slices.Sort(out)
	return out
}
