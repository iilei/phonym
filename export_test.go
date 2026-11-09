package phonym

import (
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
	Soft10  = soft10
	Soft11  = soft11
	Crisp11 = crisp11
	Soft12  = soft12
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
	syls, err := family().unrank(big.NewInt(int64(i)), familyLen)
	if err != nil {
		panic(err)
	}
	return strings.Join(syls, "")
}

// FamilyIndex returns a family name's rank among all 2-syllable family names.
func FamilyIndex(word string) int {
	c := family()
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

// WithJoinable returns a copy of a style with Joinable set, for JOIN RULE tests.
func WithJoinable(s *Style) *Style {
	c := *s
	c.Codas = slices.Clone(s.Codas)
	c.EndSuffixes = slices.Clone(s.EndSuffixes)
	c.Split = slices.Clone(s.Split)
	c.Joinable = true
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
