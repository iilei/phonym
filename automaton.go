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
)

// startState is the state before the first syllable.
var startState = state{coda: "^"}

type (
	// state is what the next syllable may depend on: the automaton state after a syllable.
	state struct {
		coda     string // coda of the previous syllable; "^" before the first one
		harmony  string // locked vowel class; "" while only neutral vowels were seen
		onset    string // onset of the previous syllable; "" before the first one
		tail     string // end of the word that could still grow into a blocked fragment
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

	// tables are the count and prefix-sum tables up to some length. Levels are never
	// modified once built, so a snapshot of the slices is safe to read without a lock.
	tables struct {
		count  []map[state]*big.Int   // count[n][q]: valid n-syllable continuations from q
		prefix []map[state][]*big.Int // prefix[n-1][q][i]: continuations covered by succ[q][:i]
	}

	// code ranks and unranks syllable sequences accepted by one automaton.
	// Count tables grow on demand, so there is no length limit.
	code struct {
		step   stepFunc
		succ   map[succKey][]string
		index  map[succKey]map[string]int
		tab    tables
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
	one := map[state]*big.Int{}
	for _, q := range c.states {
		one[q] = big.NewInt(1)
	}
	c.tab.count = []map[state]*big.Int{one}
	return c
}

// tablesUpTo extends the tables to length n and returns a snapshot of them.
func (c *code) tablesUpTo(n int) tables {
	c.mu.Lock()
	defer c.mu.Unlock()
	for len(c.tab.count) <= n {
		m := len(c.tab.count) // computing count[m]
		prev, last := c.tab.count[m-1], m == 1
		count := map[state]*big.Int{}
		prefix := map[state][]*big.Int{}
		for _, q := range c.states {
			succ := c.succ[succKey{q, last}]
			sums := make([]*big.Int, 0, len(succ)+1)
			acc := new(big.Int)
			sums = append(sums, new(big.Int))
			for _, s := range succ {
				if last {
					acc.Add(acc, big.NewInt(1))
				} else {
					n, ok := prev[c.step(q, s)]
					if !ok {
						panic(fmt.Sprintf("phonym: state after %q was not discovered", s))
					}
					acc.Add(acc, n)
				}
				sums = append(sums, new(big.Int).Set(acc))
			}
			prefix[q] = sums
			count[q] = sums[len(sums)-1]
		}
		c.tab.count = append(c.tab.count, count)
		c.tab.prefix = append(c.tab.prefix, prefix)
	}
	return tables{count: c.tab.count[:n+1], prefix: c.tab.prefix[:n]}
}

// capacity is the number of k-syllable sequences. The result must not be modified.
func (c *code) capacity(k int) *big.Int {
	return c.tablesUpTo(k).count[k][startState]
}

// unrank returns the k-syllable sequence with rank x (0 <= x < capacity(k)).
func (c *code) unrank(x *big.Int, k int) ([]string, error) {
	t := c.tablesUpTo(k)
	if x.Sign() < 0 || x.Cmp(t.count[k][startState]) >= 0 {
		return nil, fmt.Errorf("%w: %s out of range for length %d", ErrOverflow, x, k)
	}
	rest := new(big.Int).Set(x)
	q, out := startState, make([]string, 0, k)
	for remaining := k; remaining > 0; remaining-- {
		pre := t.prefix[remaining-1][q]
		i := sort.Search(len(pre), func(j int) bool { return pre[j].Cmp(rest) > 0 }) - 1
		rest.Sub(rest, pre[i])
		syl := c.succ[succKey{q, remaining == 1}][i]
		out = append(out, syl)
		q = c.step(q, syl)
	}
	return out, nil
}

// rank returns the rank of a syllable sequence among sequences of its length.
func (c *code) rank(syls []string) (*big.Int, error) {
	t := c.tablesUpTo(len(syls))
	q, x := startState, new(big.Int)
	for i, syl := range syls {
		remaining := len(syls) - i
		key := succKey{q, remaining == 1}
		j, ok := c.index[key][syl]
		if !ok {
			return nil, fmt.Errorf("%w: syllable %q not allowed here", ErrInvalidName, syl)
		}
		x.Add(x, t.prefix[remaining-1][q][j])
		q = c.step(q, syl)
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
