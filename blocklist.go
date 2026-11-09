package phonym

import (
	_ "embed" // blocklist files
	"fmt"
	"strings"
)

// minEntry is the shortest entry a blocklist may contain.
const minEntry = 3

var (
	//go:embed blocklists/1.txt
	blocklist1Text string

	// blocklist1 is frozen: a change is a new file and a new blocklist, used by new releases.
	blocklist1 = mustBlocklist(blocklist1Text)
)

// blocklist holds entries a word of a name must not (nearly) be (BLOCKLIST RULE).
//
// A word is not allowed if it contains an entry and has at most one letter besides
// it: the word is the entry, or the entry with one letter before or after it
// ("puta", "aputa", "putas"; "baputa" is fine). The automaton state keeps head, the
// word so far while it can still become such a word: a prefix of an entry, or one
// letter plus a prefix of an entry. A word's last syllable is checked against it.
type blocklist struct {
	entries  map[string]bool
	prefixes map[string]bool // every prefix of every entry, the entry included
}

// parseBlocklist reads one entry per line; blank lines and # comments are ignored.
func parseBlocklist(text string) (*blocklist, error) {
	b := &blocklist{entries: map[string]bool{}, prefixes: map[string]bool{}}
	for n, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if len(line) < minEntry || strings.Trim(line, "abcdefghijklmnopqrstuvwxyz") != "" {
			return nil, fmt.Errorf("line %d: %q is not %d+ letters a-z", n+1, line, minEntry)
		}
		if b.entries[line] {
			return nil, fmt.Errorf("line %d: duplicate entry %q", n+1, line)
		}
		b.entries[line] = true
		for i := 1; i <= len(line); i++ {
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

// advanceHead returns the head after appending syl: the word so far while it can
// still become a blocked word, deadHead after.
func (b *blocklist) advanceHead(head, syl string) string {
	if head == deadHead {
		return deadHead
	}
	if w := head + syl; b.prefixes[w] || b.prefixes[w[1:]] {
		return w
	}
	return deadHead
}

// wordBlocked reports whether a word ending with syl after head is an entry, or an
// entry with one letter before or after it.
func (b *blocklist) wordBlocked(head, syl string) bool {
	if head == deadHead {
		return false
	}
	w := head + syl
	return b.entries[w] || b.entries[w[1:]] || b.entries[w[:len(w)-1]]
}
