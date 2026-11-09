package phonym

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// Style names, as used in release labels.
const (
	styleSoft    = "soft"
	styleCrisp   = "crisp"
	styleFlowing = "flowing"
)

var (
	// semverWithBuild is semver 2.0 (semver.org) with build metadata required.
	semverWithBuild = regexp.MustCompile(
		`^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)` +
			`(?:-((?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*)(?:\.(?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*))*))?` +
			`\+([0-9a-zA-Z-]+(?:\.[0-9a-zA-Z-]+)*)$`)

	// Released styles. Never edit one: add a new style and a new release instead.
	// The shared lists below are never modified; a change needs a new list.
	softEndings    = []string{"a", "i", "el", "an", "il"}
	crispEndings   = []string{"is", "os", "ar", "on", "ir", "o"}
	flowingEndings = []string{"a", "o", "i"}
	softCodas      = []string{"", "n", "l"}
	crispCodas     = []string{"", "n", "r", "s"}
	flowingCodas   = []string{""}

	soft10 = &Style{
		rules: rules1, blocks: blocklist1, Name: styleSoft, Onsets: "blmnrvdg", Codas: softCodas,
		EndSuffixes: softEndings, Harmony: true,
	}
	crisp10 = &Style{
		rules: rules1, blocks: blocklist1, Name: styleCrisp, Onsets: "bdfgkpstz", Codas: crispCodas,
		EndSuffixes: crispEndings, Harmony: false,
	}
	flowing10 = &Style{
		rules: rules1, blocks: blocklist1, Name: styleFlowing, Onsets: "lmnrvsdt", Codas: flowingCodas,
		EndSuffixes: flowingEndings, Harmony: true,
	}
	soft11 = &Style{
		rules: rules1, blocks: blocklist1, Name: styleSoft, Onsets: "blmnrvdgjw", Codas: softCodas,
		EndSuffixes: softEndings, Harmony: true,
	}
	crisp11 = &Style{
		rules: rules1, blocks: blocklist1, Name: styleCrisp, Onsets: "bdfgkpstzh", Codas: crispCodas,
		EndSuffixes: crispEndings, Harmony: false,
	}
	flowing11 = &Style{
		rules: rules1, blocks: blocklist1, Name: styleFlowing, Onsets: "lmnrvsdtjw", Codas: flowingCodas,
		EndSuffixes: flowingEndings, Harmony: true,
	}
	soft12 = &Style{
		rules: rules1, blocks: blocklist1, Name: styleSoft, Onsets: "blmnrvdgjw", Codas: softCodas,
		EndSuffixes: softEndings, Harmony: true, Split: []int{2, 2},
	}

	// registry is frozen and append-only: the index is what names store. Entries may
	// only be appended or retired; never reorder, edit or remove one.
	registry = []Release{
		{Label: "1.0.0+soft", style: soft10},
		{Label: "1.0.0+crisp", style: crisp10},
		{Label: "1.0.0+flowing", style: flowing10},
		{Label: "1.1.0+soft", style: soft11},
		{Label: "1.1.0+crisp", style: crisp11},
		{Label: "1.1.0+flowing", style: flowing11},
		{Label: "1.2.0+soft", style: soft12},
	}

	labelIndex = map[string]int{}
	codes      = make([]lazyCode, len(registry))
)

type (
	// Release is one registry entry. A name stores its release's position in the
	// registry, never the label.
	Release struct {
		style *Style

		// Label is semver with the style as build metadata, e.g. "1.1.0+crisp".
		// Empty for a placeholder.
		Label string
		// Retired releases issue no new names, but their names keep decoding.
		// Retirement is one-way.
		Retired bool
	}

	// lazyCode builds a release's automaton on first use.
	lazyCode struct {
		code *code
		err  error
		once sync.Once
	}
)

func init() {
	if err := validateRegistry(registry); err != nil {
		panic("phonym: " + err.Error())
	}
	for i, r := range registry {
		if r.Label != "" {
			labelIndex[r.Label] = i
		}
	}
}

// Placeholder returns a retired entry without label or style. It keeps a slot
// from ever being used, e.g. when its family names turn out to be offensive.
func Placeholder() Release {
	return Release{Retired: true}
}

// Releases returns a copy of the registry in slot order.
func Releases() []Release {
	return append([]Release(nil), registry...)
}

// Style returns the release's style, or nil for a placeholder.
func (r Release) Style() *Style {
	return r.style
}

// codeFor returns the automaton of the release in slot i.
func codeFor(i int) (*code, error) {
	lc := &codes[i]
	lc.once.Do(func() {
		lc.code, lc.err = registry[i].style.build()
		if lc.err != nil {
			lc.err = fmt.Errorf("%w: %w", errStyle, lc.err)
		}
	})
	return lc.code, lc.err
}

// validateRegistry checks one registry on its own.
func validateRegistry(reg []Release) error {
	if len(reg) > Slots {
		return fmt.Errorf("%d entries > %d family slots", len(reg), Slots)
	}
	seen := map[string]bool{}
	for i, r := range reg {
		if (r.Label == "") != (r.style == nil) {
			return fmt.Errorf("slot %d: label and style must both be set or both be empty", i)
		}
		if r.Label == "" {
			if !r.Retired {
				return fmt.Errorf("slot %d: placeholders must be retired", i)
			}
			continue
		}
		if !semverWithBuild.MatchString(r.Label) {
			return fmt.Errorf("slot %d: %q is not semver with +build", i, r.Label)
		}
		if seen[r.Label] {
			return fmt.Errorf("slot %d: duplicate label %q", i, r.Label)
		}
		seen[r.Label] = true
		if major, _, _ := strings.Cut(r.Label, "."); major != strconv.Itoa(Generation) {
			return fmt.Errorf("slot %d: %q: major must equal generation %d", i, r.Label, Generation)
		}
	}
	return nil
}

// validateEvolution checks that a later registry only appends and retires entries.
func validateEvolution(old, next []Release) error {
	if len(next) < len(old) {
		return fmt.Errorf("entries removed: %d -> %d", len(old), len(next))
	}
	for i, a := range old {
		b := next[i]
		if a.Label != b.Label {
			return fmt.Errorf("slot %d (%s): label changed", i, a.Label)
		}
		if !a.style.equal(b.style) {
			return fmt.Errorf("slot %d (%s): style changed", i, a.Label)
		}
		if a.Retired && !b.Retired {
			return fmt.Errorf("slot %d (%s): un-retired", i, a.Label)
		}
	}
	return nil
}
