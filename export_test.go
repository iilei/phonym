package phonym

import (
	"fmt"
	"math/big"
	"slices"
	"strings"
)

// Test-only access to internals, for the external phonym_test package.

const (
	// FamilyCapacity is the number of 2-syllable family names.
	FamilyCapacity = familyCapacity
	// FamilyStride and FamilyOffset are the slot-spreading constants.
	FamilyStride = familyStride
	FamilyOffset = familyOffset
)

var (
	// ValidateRegistry and ValidateEvolution expose the registry checks.
	ValidateRegistry  = validateRegistry
	ValidateEvolution = validateEvolution

	// Styles of the current registry, for building trial registries.
	Soft       = soft
	Crisp      = crisp
	SoftDouble = softDouble
)

// FamilyName returns the family name of a release slot and sibling.
func FamilyName(slot, lo int) string { return familyName(slot, lo) }

// FamilyParse decodes a family name into [slot, sibling].
func FamilyParse(word string) ([2]int, error) {
	ref, err := familyParse(word)
	return [2]int{ref.slot, ref.lo}, err
}

// FamilyAt returns the 2-syllable family name with rank i, issued or not.
func FamilyAt(i int) string {
	syls, err := familyAutomaton().unrank(big.NewInt(int64(i)), familyLen)
	if err != nil {
		panic(err)
	}
	return strings.Join(syls, "")
}

// FamilyIndex returns a family name's rank among all 2-syllable family names.
func FamilyIndex(word string) int {
	c := familyAutomaton()
	syls, err := c.parse(word)
	if err != nil {
		panic(err)
	}
	idx, err := c.rank(syls)
	if err != nil {
		panic(err)
	}
	return int(idx.Int64())
}

// Normalize exposes the INPUT RULE.
func Normalize(name string) ([]string, error) { return normalize(name) }

// NewRelease builds a registry entry for tests.
func NewRelease(label string, style *Style, retired bool) Release {
	return Release{Label: label, style: style, Retired: retired}
}

// WithJoinable returns a copy of a style with Joinable set (and Grow cleared), for JOIN RULE tests.
func WithJoinable(s *Style) *Style {
	c := *s
	c.Codas = slices.Clone(s.Codas)
	c.EndSuffixes = slices.Clone(s.EndSuffixes)
	c.Split = slices.Clone(s.Split)
	c.Joinable = true
	c.Grow = false // Grow excludes Joinable
	return &c
}

// WithRegistry runs fn with a trial registry and restores the real one afterwards.
// Tests using it must not run in parallel.
func WithRegistry(reg []Release, fn func()) {
	if err := validateRegistry(reg); err != nil {
		panic(err)
	}
	savedReg, savedIdx, savedCodes := registry, labelIndex, codes
	defer func() { registry, labelIndex, codes = savedReg, savedIdx, savedCodes }()
	registry = reg
	labelIndex = map[string]int{}
	for i, r := range reg {
		if r.Label != "" {
			labelIndex[r.Label] = i
		}
	}
	codes = make([]lazyCode, len(reg))
	fn()
}

// BlockedEntries returns the entries of blocklist 1, sorted.
func BlockedEntries() []string {
	out := make([]string, 0, len(blocklist1.entries))
	for e := range blocklist1.entries {
		out = append(out, e)
	}
	slices.Sort(out)
	return out
}

// ParseBlocklist exposes the blocklist file parser.
func ParseBlocklist(text string) error {
	_, err := parseBlocklist(text)
	return err
}

// CheckUniqueSplit exposes the SPLIT RULE check.
func CheckUniqueSplit(onsets, codas, nuclei []string) error {
	return checkUniqueSplit(onsets, codas, nuclei)
}

// FamilyTableFromAutomaton generates the family table from the family automaton.
func FamilyTableFromAutomaton() (string, error) {
	c := familyAutomaton()
	if got := c.capacity(familyLen); got.Cmp(big.NewInt(familyCapacity)) != 0 {
		return "", fmt.Errorf("family automaton capacity %s, want %d", got, familyCapacity)
	}
	var b strings.Builder
	b.WriteString("# phonym family table: the issued family names in index order (spread slot * 64 + sibling).\n")
	b.WriteString("# Generated from the family automaton: go test -run TestFamilyTable -update. Do not edit.\n")
	for i := range Slots * Siblings {
		syls, err := c.unrank(big.NewInt(int64(i)), familyLen)
		if err != nil {
			return "", err
		}
		b.WriteString(strings.Join(syls, "") + "\n")
	}
	return b.String(), nil
}

// FamilyTableEmbedded is the embedded family table.
func FamilyTableEmbedded() string { return familyNamesText }
