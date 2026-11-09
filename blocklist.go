package phonym

import (
	_ "embed" // blocklist files
	"fmt"
	"strings"
)

// minFragment is the shortest fragment a blocklist may contain.
const minFragment = 3

var (
	//go:embed blocklists/1.txt
	blocklist1Text string

	// blocklist1 is frozen: a change is a new file and a new blocklist, used by new releases.
	blocklist1 = mustBlocklist(blocklist1Text)
)

// blocklist holds fragments that must never appear inside a word (BLOCKLIST RULE).
//
// A syllable is not allowed if the word so far plus the syllable contains a
// fragment. The automaton state keeps tail, the longest suffix of the word that is
// a proper prefix of some fragment: any fragment the next syllable completes starts
// inside tail + syllable, so checking that string is enough.
type blocklist struct {
	fragments map[string]bool
	prefixes  map[string]bool
	maxLen    int
}

// parseBlocklist reads one fragment per line; blank lines and # comments are ignored.
func parseBlocklist(text string) (*blocklist, error) {
	b := &blocklist{fragments: map[string]bool{}, prefixes: map[string]bool{}}
	for n, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if len(line) < minFragment || strings.Trim(line, "abcdefghijklmnopqrstuvwxyz") != "" {
			return nil, fmt.Errorf("line %d: %q is not %d+ letters a-z", n+1, line, minFragment)
		}
		if b.fragments[line] {
			return nil, fmt.Errorf("line %d: duplicate fragment %q", n+1, line)
		}
		b.fragments[line] = true
		b.maxLen = max(b.maxLen, len(line))
		for i := 1; i < len(line); i++ {
			b.prefixes[line[:i]] = true
		}
	}
	return b, nil
}

// mustBlocklist parses an embedded blocklist; a bad file is a build error.
func mustBlocklist(text string) *blocklist {
	b, err := parseBlocklist(text)
	if err != nil {
		panic("phonym: blocklist: " + err.Error())
	}
	return b
}

// advance returns the tail after appending syl, and false if syl would complete a fragment.
func (b *blocklist) advance(tail, syl string) (string, bool) {
	w := tail + syl
	for i := range len(w) {
		for j := i + 1; j <= min(len(w), i+b.maxLen); j++ {
			if b.fragments[w[i:j]] {
				return "", false
			}
		}
	}
	for i := range len(w) { // longest suffix first
		if b.prefixes[w[i:]] {
			return w[i:], true
		}
	}
	return "", true
}
