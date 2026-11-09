import random
import re
from itertools import product
from math import gcd

import syllable_automaton as sa   # only the generic SyllableCode, State and START

# --- frozen family code ----------------------------------------------------
# Everything below is a frozen copy: it must never follow edits to the base
# rules in syllable_automaton.py, or every family name would be renumbered.

F_ONSETS = "bdfgklmnprstvzhjw"
F_NUCLEI = "aeiou"
F_CODAS = ("", "n", "r", "s", "l", "v")
F_VOICED = frozenset("bdgvz")
F_VOICELESS = frozenset("ptkfsh")
F_VOICING_EXCEPTIONS = frozenset({("s", "b"), ("s", "d"), ("s", "g")})
F_CLUSTERS = frozenset({("s", "z"), ("n", "m"), ("r", "l"), ("l", "r")})
F_HARMONY = {"e": "front", "i": "front", "o": "back", "u": "back", "a": None}

F_PARTS = {o + n + c: (o, n, c)
           for o, n, c in product(F_ONSETS, F_NUCLEI, F_CODAS) if o != c}
F_SYLLABLES = list(F_PARTS)

# Family-style endings, as vowel+coda suffixes. Disjoint from the given-name
# END so a family name can never be mistaken for a given name (word-order safety).
FAMILY_SUFFIXES = {"en", "in", "ov", "av", "as", "us", "ul"}
FAMILY_END = {s for s in F_SYLLABLES if s[1:] in FAMILY_SUFFIXES}

dead = {x for x in FAMILY_SUFFIXES if not any(s[1:] == x for s in F_SYLLABLES)}
assert not dead, f"suffixes matching no syllable: {dead}"
assert not FAMILY_END & sa.END, f"overlap with given END: {FAMILY_END & sa.END}"


F_BLOCKLIST = sa.BLOCKLIST1


def family_step(q, syl):
    onset, nucleus, coda = F_PARTS[syl]
    return sa.State(coda, q.harmony or F_HARMONY[nucleus], onset, F_BLOCKLIST.advance(q.tail, syl) or "")


# SHAPE RULE (frozen): every family name is CVC-CVC, i.e. 6 letters with a
# consonant pair in the middle. Proquint words are always 5 letters of strictly
# alternating CVCVC, so a phonid family name - and therefore a phonid name -
# can never be a proquint. The first syllable must be closed; the given-name
# rule "no two closed syllables in a row" does not apply inside family names.
FAMILY_SHAPE = re.compile(r"^[^aeiou][aeiou][^aeiou]{2}[aeiou][^aeiou]$")

# GENERATION RULE (frozen): the family-name shape identifies the generation, and
# the generation is the semver major of every release in the registry. CVC-CVC is
# generation 1. A new shape (e.g. CVV-CVC) means a new generation and major 2.
GENERATION = 1


class UnsupportedFormat(ValueError):
    pass


def family_allowed(q, syl, is_last):
    onset, nucleus, coda = F_PARTS[syl]
    if q.coda == "^" and not coda:            # first syllable must be closed
        return False
    if is_last and syl not in FAMILY_END:
        return False
    if F_BLOCKLIST.advance(q.tail, syl) is None:
        return False
    if q.harmony and F_HARMONY[nucleus] not in (None, q.harmony):
        return False
    if onset == q.onset:
        return False
    if q.coda in ("^", ""):
        return True
    if onset == q.coda:
        return False
    if (q.coda, onset) in F_CLUSTERS:
        return False
    if q.coda in F_VOICED and onset in F_VOICELESS:
        return False
    if (q.coda in F_VOICELESS and onset in F_VOICED
            and (q.coda, onset) not in F_VOICING_EXCEPTIONS):
        return False
    return True


FAMILY = sa.SyllableCode(F_SYLLABLES, family_step, family_allowed, sa.START)
FAMILY_LEN = 2                                # every family name: exactly 2 syllables
K = 64                                        # siblings per release (frozen layout)

CAP = FAMILY.capacity(FAMILY_LEN)
VERSIONS = CAP // K                           # version slots available


def _stride(n):
    s = round(n * 0.6180339887)               # golden-ratio stride spreads neighbours
    while gcd(s, n) != 1:
        s += 1
    return s


STRIDE = _stride(VERSIONS)
STRIDE_INV = pow(STRIDE, -1, VERSIONS)
OFFSET = 97                                   # frozen; keeps v0 off the first name


def family_name(v, lo=0):
    """Version spread by stride; the K siblings of a version stay adjacent."""
    slot = (v * STRIDE + OFFSET) % VERSIONS
    return "".join(FAMILY.unrank(slot * K + lo, FAMILY_LEN))


def family_parse(name):
    if not FAMILY_SHAPE.match(name):
        raise UnsupportedFormat(
            f"{name!r}: not a generation-{GENERATION} family name (CVC-CVC, 6 letters)")
    syls = FAMILY.parse(name)
    if len(syls) != FAMILY_LEN:
        raise UnsupportedFormat(f"{name!r}: family name must have {FAMILY_LEN} syllables")
    idx = FAMILY.rank(syls)
    if idx >= VERSIONS * K:                   # the CAP % K leftover names are never issued
        raise UnsupportedFormat(f"{name!r}: not an issued family name")
    slot, lo = divmod(idx, K)
    return ((slot - OFFSET) * STRIDE_INV) % VERSIONS, lo


# --- given name + family name ---------------------------------------------

GIVEN = sa.SyllableCode(sa.SYLLABLES, sa.step, sa.allowed, sa.START)


def opt1(x, v):
    return f"{GIVEN.encode(x)} {family_name(v)}"


def opt3(x, v):
    hi, lo = divmod(x, K)
    return f"{GIVEN.encode(hi)} {family_name(v, lo)}"


def dec3(name):
    given, family = name.split(" ")
    v, lo = family_parse(family)
    return v, GIVEN.decode(given) * K + lo


if __name__ == "__main__":
    print(f"family capacity {CAP:,} -> {VERSIONS:,} versions x {K} siblings, stride {STRIDE}")

    # exhaustive: every (version, sibling) round-trips, and family names are unique
    seen = set()
    for v in range(VERSIONS):
        for lo in range(K):
            f = family_name(v, lo)
            assert family_parse(f) == (v, lo)
            seen.add(f)
    assert len(seen) == VERSIONS * K          # CAP % K leftover names stay unused
    print("family round trip ok")

    # no name, given or family, has two consecutive syllables with the same onset
    def stutters(name, code):
        syls = code.parse(name)
        return any(a[0] == b[0] for a, b in zip(syls, syls[1:]))
    assert not any(stutters(f, FAMILY) for f in seen)
    sample = [GIVEN.encode(x) for x in range(0, 0x10000000, 9973)]
    assert not any(stutters(g, GIVEN) for g in sample)
    print(f"no stutter in {len(seen):,} family + {len(sample):,} sampled given names")

    print("\nfamily names, versions 0..5 (option 1):")
    print("  " + "  ".join(f"v{v}={family_name(v)}" for v in range(6)))

    random.seed(7)
    xs = [random.randrange(0x1000000, 0x10000000) for _ in range(8)]
    print(f"\n{'hex':>9}  {'option 1 (v=3)':<28} option 3 (v=3, K={K})")
    for x in xs:
        n3 = opt3(x, 3)
        assert dec3(n3) == (3, x)
        print(f"{x:#09x}  {opt1(x, 3):<28} {n3}")

    base = 0xA3F2C10
    print("\nconsecutive IDs, option 3:")
    for x in range(base, base + 4):
        print(f"{x:#09x}  {opt3(x, 3)}")
