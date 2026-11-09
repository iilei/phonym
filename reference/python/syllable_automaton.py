"""Order-preserving bijection between integers and pronounceable names.

Names are syllable sequences accepted by a finite automaton. Encoding
unranks a path through the automaton; decoding ranks it.
"""
from bisect import bisect_right
from itertools import accumulate, product
from typing import NamedTuple

# --- phonotactics (the "matrix") -------------------------------------------

ONSETS = list("bdfgklmnprstvzhjw")       # mandatory, single consonant
NUCLEI = list("aeiou")
CODAS = ["", "n", "r", "s", "l", "v"]

# Obstruent voicing; sonorants (l m n r) belong to neither set.
VOICED = set("bdgvz")
VOICELESS = set("ptkfsh")                # h: so "v" + "h" is blocked too

# Voiceless coda + voiced onset is banned, except these (coda, onset) pairs,
# which stay allowed because they sound natural (cf. asbest, wisdom, disgust).
VOICING_EXCEPTIONS = {("s", "b"), ("s", "d"), ("s", "g")}

# syl -> (onset, nucleus, coda); onset ≠ coda, so no "rer", "sis", "vav"
PARTS = {o + n + c: (o, n, c)
         for o, n, c in product(ONSETS, NUCLEI, CODAS) if o != c}
SYLLABLES = list(PARTS)

# Vowel harmony: the first non-neutral vowel locks the name to its class.
HARMONY = {"e": "front", "i": "front", "o": "back", "u": "back", "a": None}


class State(NamedTuple):
    coda: str             # coda of previous syllable; "^" before the first one
    harmony: str | None   # locked vowel class, None while only neutrals seen
    onset: str            # onset of previous syllable; "" before the first one


START = State("^", None, "")


def step(q: State, syl: str) -> State:
    """Automaton transition: state after emitting `syl` from state `q`."""
    onset, nucleus, coda = PARTS[syl]
    return State(coda, q.harmony or HARMONY[nucleus], onset)


# Only these may close a name. Tune the sound here.
END = {s for s in SYLLABLES if s[1:] in {"a", "o", "i", "an", "on", "ar", "is", "el", "iv" }} | {"iel", "mar", "fir"}

def allowed(q: State, syl: str, is_last: bool) -> bool:
    """May `syl` follow state `q` at this position?"""
    onset, nucleus, coda = PARTS[syl]
    if is_last and syl not in END:
        return False
    if q.harmony and HARMONY[nucleus] not in (None, q.harmony):
        return False                      # front/back clash: "kelu", "tobi"
    if onset == q.onset:
        return False                      # same-onset stutter: "va-vav", "ka-ki"
    if q.coda in ("^", ""):
        return True
    if onset == q.coda:                   # no doubled consonant: "an" + "na"
        return False
    if coda:                              # no two closed syllables in a row
        return False
    if (q.coda, onset) in {("s", "z"), ("n", "m"), ("r", "l"), ("l", "r")}:
        return False
    if q.coda in VOICED and onset in VOICELESS:   # "vk" would devoice to "fk"
        return False
    if (q.coda in VOICELESS and onset in VOICED
            and (q.coda, onset) not in VOICING_EXCEPTIONS):
        return False
    return True


# --- automaton ranking -----------------------------------------------------

class SyllableCode:
    """Ranking over the automaton. Count tables grow on demand: no length limit."""

    def __init__(self, syllables, step, allowed, start):
        self.step = step
        self.start = start

        # discover reachable states; syllable order fixes the ordering of names.
        # succ is keyed by (state, is_last) so the final slot has its own inventory
        self.succ, frontier, states = {}, [start], {start}
        while frontier:
            q = frontier.pop()
            for last in (False, True):
                self.succ[q, last] = [s for s in syllables if allowed(q, s, last)]
            for s in self.succ[q, False]:
                if (nxt := step(q, s)) not in states:
                    states.add(nxt)
                    frontier.append(nxt)
        self.states = states

        # count[n][q] = number of valid n-syllable continuations from state q
        # prefix[n-1][q][i] = continuations covered by succ[q][:i] when n remain
        self.count = [{q: 1 for q in states}]
        self.prefix = []
        self.index = {key: {s: i for i, s in enumerate(lst)}
                      for key, lst in self.succ.items()}
        self.max_syl_len = max(map(len, syllables))

    def _ensure(self, n: int) -> None:
        while len(self.count) <= n:
            m = len(self.count)                       # computing count[m]
            prev, last = self.count[m - 1], m == 1
            pre = {q: [0, *accumulate(1 if last else prev[self.step(q, s)]
                                      for s in self.succ[q, last])]
                   for q in self.states}
            self.prefix.append(pre)
            self.count.append({q: sums[-1] for q, sums in pre.items()})

    # fixed length ---------------------------------------------------------

    def capacity(self, k: int) -> int:
        self._ensure(k)
        return self.count[k][self.start]

    def unrank(self, x: int, k: int) -> list[str]:
        if not 0 <= x < self.capacity(k):
            raise ValueError(f"{x} out of range for length {k}")
        q, out = self.start, []
        for remaining in range(k, 0, -1):
            pre = self.prefix[remaining - 1][q]
            i = bisect_right(pre, x) - 1
            x -= pre[i]
            syl = self.succ[q, remaining == 1][i]
            out.append(syl)
            q = self.step(q, syl)
        return out

    def rank(self, syls: list[str]) -> int:
        self._ensure(len(syls))
        q, x = self.start, 0
        for remaining, syl in zip(range(len(syls), 0, -1), syls):
            i = self.index[q, remaining == 1][syl]   # KeyError -> illegal name
            x += self.prefix[remaining - 1][q][i]
            q = self.step(q, syl)
        return x

    # variable length: shorter names first, so 0 -> shortest name --------

    def encode(self, x: int) -> str:
        if x < 0:
            raise ValueError("negative IDs are not encodable")
        k = 1
        while x >= self.capacity(k):
            x -= self.capacity(k)
            k += 1
        return "".join(self.unrank(x, k))

    def decode(self, name: str) -> int:
        syls = self.parse(name)
        return sum(self.capacity(k) for k in range(1, len(syls))) + self.rank(syls)

    # string -> syllables; must be unambiguous for the code to be a bijection

    def parse(self, name: str) -> list[str]:
        def go(pos, q):
            if pos == len(name):
                return [[]]
            results = []
            for j in range(pos + 1, min(len(name), pos + self.max_syl_len) + 1):
                syl = name[pos:j]
                if syl in self.index[q, j == len(name)]:
                    results += [[syl, *rest] for rest in go(j, self.step(q, syl))]
            return results

        parses = go(0, self.start)
        if len(parses) != 1:
            raise ValueError(f"{name!r}: {len(parses)} parses")
        return parses[0]


if __name__ == "__main__":
    code = SyllableCode(SYLLABLES, step, allowed, START)
    print(f"{len(code.states)} reachable states")

    for k in range(1, 5):
        print(f"len {k}: {code.capacity(k):>12,} names  ({code.capacity(k).bit_length() - 1} bits)")

    for x in [0, 1, 2, 1_000, 5_000, 123_456, 9_876_543, 2**32 - 1]:
        name = code.encode(x)
        assert code.decode(name) == x
        print(f"{x:>12,} -> {name}")

    # exhaustive round trip + order preservation for lengths 1..2
    names = [code.encode(x) for x in range(code.capacity(1) + code.capacity(2))]
    assert [code.decode(n) for n in names] == list(range(len(names)))
    # harmony holds: no name mixes front and back vowels
    for n in names:
        classes = {HARMONY[v] for v in n if v in HARMONY} - {None}
        assert len(classes) <= 1, n
    print("round trip ok, harmony ok")
