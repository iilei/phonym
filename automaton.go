package phonym

import (
	"fmt"
	"math"
	"math/big"
	"math/bits"
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
	// saturated marks a 64-bit count that doesn't fit; the big.Int count is used then.
	saturated = math.MaxUint64
	// notAllowed marks a syllable that may not follow a state.
	notAllowed = -1
)

// startState is the state before the first syllable.
var startState = state{coda: "^"}

type (
	// state is what the next syllable may depend on: the automaton state after a syllable.
	// It is only used while the automaton is built; afterwards states are numbers.
	state struct {
		coda     string // coda of the previous syllable; "^" before the first one
		harmony  string // locked vowel class; "" while only neutral vowels were seen
		onset    string // onset of the previous syllable; "" before the first one
		head     string // the word so far while it can still become a blocked word; deadHead after
		clusters int    // onset clusters used so far in this word (styles that limit them)
		extras   int    // extra nuclei used so far in this word (styles that limit them)
	}

	// parseNode is a position in the word and the automaton state there.
	parseNode struct {
		pos int
		q   int32 // -1: the end of the word
	}
	parseEdge struct{ to, syl int32 }

	stepFunc  func(q state, syl string) state
	allowFunc func(q state, syl string, last bool) bool

	// code ranks and unranks syllable sequences accepted by one automaton.
	//
	// Syllables and states are numbers: state 0 is the start, syllable i is syl[i] in
	// inventory order. Count tables grow on demand, so there is no length limit; only
	// counts are stored (count[n][q] per length and state), and unranking adds them up
	// as it goes. Counts are kept twice: as uint64 for the common case, and as big.Int
	// once they no longer fit.
	code struct {
		sylID   map[string]int32
		syl     []string
		succ    [2][][]int32 // succ[last][q]: syllables allowed after state q, in order
		pos     [2][][]int16 // pos[last][q][s]: position of syllable s in succ[last][q], or notAllowed
		next    [][]int32    // next[q][i]: state after succ[0][q][i]
		count   [][]*big.Int // count[n][q]: valid n-syllable continuations from state q
		count64 [][]uint64   // the same counts as uint64, or saturated
		mu      sync.Mutex
		maxSyl  int
	}
)

// newCode discovers the reachable states and builds the successor tables.
// Syllable order fixes the order of names; state discovery order affects nothing.
func newCode(inv syllableSet, step stepFunc, allowed allowFunc) *code {
	if len(inv.names) > math.MaxInt16 {
		panic("phonym: too many syllables for the position tables")
	}
	c := &code{syl: inv.names, sylID: make(map[string]int32, len(inv.names))}
	for i, s := range inv.names {
		c.sylID[s] = int32(i)
		c.maxSyl = max(c.maxSyl, len(s))
	}
	ids := map[state]int32{startState: 0}
	states := []state{startState}
	for qi := 0; qi < len(states); qi++ { // states grows while new ones are found
		q := states[qi]
		for last := range 2 {
			var succ []int32
			pos := make([]int16, len(inv.names))
			for s, name := range inv.names {
				pos[s] = notAllowed
				if allowed(q, name, last == 1) {
					pos[s] = int16(len(succ)) //nolint:gosec // bounded by the MaxInt16 check above
					succ = append(succ, int32(s))
				}
			}
			c.succ[last] = append(c.succ[last], succ)
			c.pos[last] = append(c.pos[last], pos)
		}
		next := make([]int32, len(c.succ[0][qi]))
		for i, s := range c.succ[0][qi] {
			nx := step(q, inv.names[s])
			id, ok := ids[nx]
			if !ok {
				id = int32(len(states)) //nolint:gosec // states are far fewer than 2^31
				ids[nx] = id
				states = append(states, nx)
			}
			next[i] = id
		}
		c.next = append(c.next, next)
	}
	one := make([]*big.Int, len(states))
	one64 := make([]uint64, len(states))
	for i := range one {
		one[i], one64[i] = big.NewInt(1), 1
	}
	c.count, c.count64 = [][]*big.Int{one}, [][]uint64{one64}
	return c
}

// countsUpTo extends the count tables to length n and returns a snapshot of them.
// Levels are never modified once built, so the snapshot is safe to read unlocked.
func (c *code) countsUpTo(n int) ([][]*big.Int, [][]uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for len(c.count) <= n {
		m := len(c.count) // computing count[m]
		prev, prev64 := c.count[m-1], c.count64[m-1]
		level, level64 := make([]*big.Int, len(c.next)), make([]uint64, len(c.next))
		for q := range c.next {
			if m == 1 {
				level[q] = big.NewInt(int64(len(c.succ[1][q])))
				level64[q] = uint64(len(c.succ[1][q]))
				continue
			}
			sum := new(big.Int)
			var sum64, carry uint64
			for _, nx := range c.next[q] {
				sum.Add(sum, prev[nx])
				if carry == 0 && prev64[nx] != saturated {
					sum64, carry = bits.Add64(sum64, prev64[nx], 0)
				} else {
					carry = 1
				}
			}
			level[q] = sum
			level64[q] = sum64
			if carry != 0 || sum64 == saturated {
				level64[q] = saturated
			}
		}
		c.count, c.count64 = append(c.count, level), append(c.count64, level64)
	}
	return c.count[:n+1], c.count64[:n+1]
}

// capacity is the number of k-syllable sequences. The result must not be modified.
func (c *code) capacity(k int) *big.Int {
	count, _ := c.countsUpTo(k)
	return count[k][0]
}

// unrank returns the k-syllable sequence with rank x (0 <= x < capacity(k)).
func (c *code) unrank(x *big.Int, k int) ([]string, error) {
	count, count64 := c.countsUpTo(k)
	if x.Sign() < 0 || x.Cmp(count[k][0]) >= 0 {
		return nil, fmt.Errorf("%w: %s out of range for length %d", ErrOverflow, x, k)
	}
	if count64[k][0] != saturated { // every count on the way is at most count[k][0]
		return c.unrank64(x.Uint64(), k, count64), nil
	}
	rest := new(big.Int).Set(x)
	q, out := int32(0), make([]string, 0, k)
	for remaining := k; remaining > 1; remaining-- {
		for j, nx := range c.next[q] {
			if n := count[remaining-1][nx]; rest.Cmp(n) >= 0 {
				rest.Sub(rest, n)
				continue
			}
			out = append(out, c.syl[c.succ[0][q][j]])
			q = nx
			break
		}
	}
	// the last syllable: every allowed one completes exactly one name
	return append(out, c.syl[c.succ[1][q][rest.Int64()]]), nil
}

// unrank64 is unrank when all counts fit in a uint64.
func (c *code) unrank64(rest uint64, k int, count64 [][]uint64) []string {
	q, out := int32(0), make([]string, 0, k)
	for remaining := k; remaining > 1; remaining-- {
		for j, nx := range c.next[q] {
			if n := count64[remaining-1][nx]; rest >= n {
				rest -= n
				continue
			}
			out = append(out, c.syl[c.succ[0][q][j]])
			q = nx
			break
		}
	}
	return append(out, c.syl[c.succ[1][q][rest]])
}

// rank returns the rank of a syllable sequence among sequences of its length.
func (c *code) rank(syls []string) (*big.Int, error) {
	count, count64 := c.countsUpTo(len(syls))
	fast := count64[len(syls)][0] != saturated // the rank is below count[len][0]
	q, x, x64 := int32(0), new(big.Int), uint64(0)
	for i, name := range syls {
		last := 0
		if i == len(syls)-1 {
			last = 1
		}
		s, ok := c.sylID[name]
		j := int16(notAllowed)
		if ok {
			j = c.pos[last][q][s]
		}
		if j == notAllowed {
			return nil, fmt.Errorf("%w: syllable %q not allowed here", ErrInvalidName, name)
		}
		if last == 1 {
			x64 += uint64(j) //nolint:gosec // j >= 0 here
			if !fast {
				x.Add(x, big.NewInt(int64(j)))
			}
			break
		}
		remaining := len(syls) - i
		for _, nx := range c.next[q][:j] {
			if fast {
				x64 += count64[remaining-1][nx]
			} else {
				x.Add(x, count[remaining-1][nx])
			}
		}
		q = c.next[q][j]
	}
	if fast {
		return new(big.Int).SetUint64(x64), nil
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

// parseGraph builds every way the automaton can read word: nodes[0] is the start,
// and edges[i] are the syllables that can follow nodes[i].
func (c *code) parseGraph(word string) ([]parseNode, [][]parseEdge) {
	n := len(word)
	nodes := []parseNode{{0, 0}}
	edges := [][]parseEdge{nil}
	ids := map[parseNode]int32{nodes[0]: 0}
	for i := 0; i < len(nodes); i++ { // nodes grows while new ones are found
		v := nodes[i]
		if v.q < 0 {
			continue
		}
		for j := v.pos + 1; j <= min(n, v.pos+c.maxSyl); j++ {
			s, ok := c.sylID[word[v.pos:j]]
			if !ok {
				continue
			}
			to := parseNode{n, -1}
			if j < n {
				p := c.pos[0][v.q][s]
				if p == notAllowed {
					continue
				}
				to = parseNode{j, c.next[v.q][p]}
			} else if c.pos[1][v.q][s] == notAllowed {
				continue
			}
			id, seen := ids[to]
			if !seen {
				id = int32(len(nodes)) //nolint:gosec // at most len(word) * states nodes
				ids[to] = id
				nodes, edges = append(nodes, to), append(edges, nil)
			}
			edges[i] = append(edges[i], parseEdge{id, s})
		}
	}
	return nodes, edges
}

// parse splits a word into syllables the automaton accepts. It is iterative
// (no recursion limit) and finds all parses, so ambiguity is detected.
func (c *code) parse(word string) ([]string, error) {
	nodes, edges := c.parseGraph(word)
	// ways[v]: number of parses from v to the end, capped at 2. Every edge moves
	// forward in the word, so nodes are settled from the end of the word backwards.
	order := make([]int32, len(nodes))
	for i := range order {
		order[i] = int32(i)
	}
	sort.SliceStable(order, func(a, b int) bool { return nodes[order[a]].pos > nodes[order[b]].pos })
	ways := make([]int, len(nodes))
	for _, v := range order {
		if nodes[v].q < 0 {
			ways[v] = 1
			continue
		}
		for _, e := range edges[v] {
			ways[v] = min(parseCap, ways[v]+ways[e.to])
		}
	}
	if ways[0] != 1 {
		return nil, fmt.Errorf("%w: %q has %d parse(s)", ErrInvalidName, word, ways[0])
	}
	var out []string
	for v := int32(0); nodes[v].q >= 0; {
		for _, e := range edges[v] {
			if ways[e.to] > 0 {
				out = append(out, c.syl[e.syl])
				v = e.to
				break
			}
		}
	}
	return out, nil
}
