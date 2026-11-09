package phonym

import (
	"fmt"
	"math/big"
	"sort"
	"strings"
	"sync"
)

const (
	// maxEmptyLengths bounds the search for a non-empty length in encodeVar. A style
	// that produces no names for this many consecutive lengths produces none at all.
	maxEmptyLengths = 8
	// parseCap caps the parse count: one parse is all that matters, two means ambiguous.
	parseCap = 2
	// deadHead marks a word that can no longer become a blocked word (not a letter).
	deadHead = "!"
)

// startState is the state before the first syllable.
var startState = state{coda: "^"}

type (
	// state is what the next syllable may depend on: the automaton state after a syllable.
	state struct {
		coda     string // coda of the previous syllable; "^" before the first one
		harmony  string // locked vowel class; "" while only neutral vowels were seen
		onset    string // onset of the previous syllable; "" before the first one
		head     string // the word so far while it can still become a blocked word; deadHead after
		clusters int    // onset clusters used so far in this word (styles that limit them)
		extras   int    // extra nuclei used so far in this word (styles that limit them)
	}

	// succKey selects a successor list: the state, and whether the next syllable is the last.
	succKey struct {
		q    state
		last bool
	}

	stepFunc  func(q state, syl string) state
	allowFunc func(q state, syl string, last bool) bool

	// code ranks and unranks syllable sequences accepted by one automaton.
	// Count tables grow on demand, so there is no length limit. Only counts are
	// stored (count[n][q] per length and state); unranking adds them up as it goes,
	// so memory stays at states x lengths numbers.
	code struct {
		step   stepFunc
		succ   map[succKey][]string
		index  map[succKey]map[string]int
		id     map[state]int // state -> position in states
		next   [][]int       // next[q][i]: state after succ[q, not last][i]
		count  [][]*big.Int  // count[n][q]: valid n-syllable continuations from state q
		states []state
		mu     sync.Mutex
		maxSyl int
	}
)

// newCode discovers the reachable states and builds the successor lists.
// Syllable order fixes the order of names; state discovery order affects nothing.
func newCode(inv syllableSet, step stepFunc, allowed allowFunc) *code {
	c := &code{
		step:  step,
		succ:  map[succKey][]string{},
		index: map[succKey]map[string]int{},
	}
	seen := map[state]bool{startState: true}
	frontier := []state{startState}
	for len(frontier) > 0 {
		q := frontier[len(frontier)-1]
		frontier = frontier[:len(frontier)-1]
		c.states = append(c.states, q)
		for _, last := range []bool{false, true} {
			key := succKey{q, last}
			idx := map[string]int{}
			for _, s := range inv.names {
				if allowed(q, s, last) {
					idx[s] = len(c.succ[key])
					c.succ[key] = append(c.succ[key], s)
				}
			}
			c.index[key] = idx
		}
		for _, s := range c.succ[succKey{q, false}] {
			if next := step(q, s); !seen[next] {
				seen[next] = true
				frontier = append(frontier, next)
			}
		}
	}
	for _, s := range inv.names {
		c.maxSyl = max(c.maxSyl, len(s))
	}
	c.id = make(map[state]int, len(c.states))
	for i, q := range c.states {
		c.id[q] = i
	}
	c.next = make([][]int, len(c.states))
	for i, q := range c.states {
		succ := c.succ[succKey{q, false}]
		c.next[i] = make([]int, len(succ))
		for j, s := range succ {
			c.next[i][j] = c.id[step(q, s)]
		}
	}
	one := make([]*big.Int, len(c.states))
	for i := range one {
		one[i] = big.NewInt(1)
	}
	c.count = [][]*big.Int{one}
	return c
}

// countsUpTo extends the count tables to length n and returns a snapshot of them.
// Levels are never modified once built, so the snapshot is safe to read unlocked.
func (c *code) countsUpTo(n int) [][]*big.Int {
	c.mu.Lock()
	defer c.mu.Unlock()
	for len(c.count) <= n {
		m := len(c.count) // computing count[m]
		prev := c.count[m-1]
		level := make([]*big.Int, len(c.states))
		for i, q := range c.states {
			if m == 1 {
				level[i] = big.NewInt(int64(len(c.succ[succKey{q, true}])))
				continue
			}
			sum := new(big.Int)
			for _, nx := range c.next[i] {
				sum.Add(sum, prev[nx])
			}
			level[i] = sum
		}
		c.count = append(c.count, level)
	}
	return c.count[:n+1]
}

// capacity is the number of k-syllable sequences. The result must not be modified.
func (c *code) capacity(k int) *big.Int {
	return c.countsUpTo(k)[k][0] // state 0 is startState
}

// unrank returns the k-syllable sequence with rank x (0 <= x < capacity(k)).
func (c *code) unrank(x *big.Int, k int) ([]string, error) {
	count := c.countsUpTo(k)
	if x.Sign() < 0 || x.Cmp(count[k][0]) >= 0 {
		return nil, fmt.Errorf("%w: %s out of range for length %d", ErrOverflow, x, k)
	}
	rest := new(big.Int).Set(x)
	qi, out := 0, make([]string, 0, k)
	for remaining := k; remaining > 1; remaining-- {
		succ := c.succ[succKey{c.states[qi], false}]
		for j, nx := range c.next[qi] {
			if n := count[remaining-1][nx]; rest.Cmp(n) >= 0 {
				rest.Sub(rest, n)
				continue
			}
			out = append(out, succ[j])
			qi = nx
			break
		}
	}
	// the last syllable: every allowed one completes exactly one name
	out = append(out, c.succ[succKey{c.states[qi], true}][rest.Int64()])
	return out, nil
}

// rank returns the rank of a syllable sequence among sequences of its length.
func (c *code) rank(syls []string) (*big.Int, error) {
	count := c.countsUpTo(len(syls))
	qi, x := 0, new(big.Int)
	for i, syl := range syls {
		remaining := len(syls) - i
		j, ok := c.index[succKey{c.states[qi], remaining == 1}][syl]
		if !ok {
			return nil, fmt.Errorf("%w: syllable %q not allowed here", ErrInvalidName, syl)
		}
		if remaining == 1 {
			x.Add(x, big.NewInt(int64(j)))
			break
		}
		for _, nx := range c.next[qi][:j] {
			x.Add(x, count[remaining-1][nx])
		}
		qi = c.next[qi][j]
	}
	return x, nil
}

// encodeVar encodes x with variable length: all shorter names come first.
func (c *code) encodeVar(x *big.Int) (string, error) {
	if x.Sign() < 0 {
		return "", fmt.Errorf("%w: negative id", ErrOverflow)
	}
	rest := new(big.Int).Set(x)
	empty := 0
	for k := 1; ; k++ {
		capK := c.capacity(k)
		if rest.Cmp(capK) < 0 {
			syls, err := c.unrank(rest, k)
			if err != nil {
				return "", err
			}
			return strings.Join(syls, ""), nil
		}
		if capK.Sign() == 0 {
			empty++
			if empty >= maxEmptyLengths {
				return "", fmt.Errorf("%w: style produces no names", ErrOverflow)
			}
		} else {
			empty = 0
		}
		rest.Sub(rest, capK)
	}
}

// decodeVar is the inverse of encodeVar.
func (c *code) decodeVar(word string) (*big.Int, error) {
	syls, err := c.parse(word)
	if err != nil {
		return nil, err
	}
	x, err := c.rank(syls)
	if err != nil {
		return nil, err
	}
	for k := 1; k < len(syls); k++ {
		x.Add(x, c.capacity(k))
	}
	return x, nil
}

// parse splits a word into syllables the automaton accepts. It is iterative
// (no recursion limit) and finds all parses, so ambiguity is detected.
func (c *code) parse(word string) ([]string, error) {
	type node struct {
		q   state
		pos int
	}
	type edge struct {
		syl string
		to  node
	}
	n := len(word)
	root := node{q: startState}
	edges := map[node][]edge{}
	order := []node{root}
	seen := map[node]bool{root: true}
	for i := 0; i < len(order); i++ {
		v := order[i]
		for j := v.pos + 1; j <= min(n, v.pos+c.maxSyl); j++ {
			syl := word[v.pos:j]
			if _, ok := c.index[succKey{v.q, j == n}][syl]; !ok {
				continue
			}
			to := node{q: c.step(v.q, syl), pos: j}
			edges[v] = append(edges[v], edge{syl, to})
			if !seen[to] {
				seen[to] = true
				order = append(order, to)
			}
		}
	}
	// ways[v]: number of parses from v to the end, capped at 2.
	sort.SliceStable(order, func(a, b int) bool { return order[a].pos > order[b].pos })
	ways := map[node]int{}
	for _, v := range order {
		if v.pos == n {
			ways[v] = 1
			continue
		}
		w := 0
		for _, e := range edges[v] {
			w = min(parseCap, w+ways[e.to])
		}
		ways[v] = w
	}
	if ways[root] != 1 {
		return nil, fmt.Errorf("%w: %q has %d parse(s)", ErrInvalidName, word, ways[root])
	}
	var out []string
	for v := root; v.pos < n; {
		for _, e := range edges[v] {
			if ways[e.to] > 0 {
				out = append(out, e.syl)
				v = e.to
				break
			}
		}
	}
	return out, nil
}
