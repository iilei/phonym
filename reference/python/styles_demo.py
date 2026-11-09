"""Releases: the family name carries a registry index, the given name the ID.

Each release is a semver label whose build metadata names the style, e.g.
"1.0.0+soft". The name stores the index into REGISTRY, never the label.
"""
import random
import re
import unicodedata
from dataclasses import dataclass, replace
from functools import lru_cache
from itertools import product

import family_demo as fam          # frozen family code: slot <-> 2-syllable family name
import syllable_automaton as sa    # SyllableCode, State, START, shared constants


@dataclass(frozen=True)
class Style:
    name: str
    onsets: str
    codas: tuple[str, ...]
    end_suffixes: frozenset[str]     # vowel+coda suffixes allowed on the last syllable
    harmony: bool
    # split: the shape of the given name, as syllables per word.
    #   ()      one given word, variable length: short IDs give short names,
    #           large IDs grow by a syllable when needed ("manwudoma").
    #   (2, 2)  two given words of exactly 2 syllables each ("liwa-givel"),
    #           like a double first name. Each word is a complete name on its
    #           own: it has its own END syllable, and harmony and cluster rules
    #           restart at the hyphen. The words act as digits of one number:
    #           value = word1 * capacity(2) + word2, so capacity is fixed
    #           (product of the word capacities) and larger IDs raise
    #           OverflowError instead of growing.
    #   (2, 3)  same, with a 3-syllable second word.
    # The shape is fixed per release: the decoder expects exactly len(split)
    # given words, so a separator always means a word boundary, never data.
    split: tuple[int, ...] = ()
    # joinable: ignore separators inside the given name when decoding, so
    # "liwagivel" (split) or "baran-geven-ladan" (single word) are accepted.
    # See JOIN RULE below. Encoding is unaffected.
    joinable: bool = False
    # blocklist: fragments that never appear inside a word (BLOCKLIST RULE).
    blocklist: sa.Blocklist | None = None
    # onset_clusters: extra onsets of two consonants ("br", "kl"), after the single ones.
    onset_clusters: tuple[str, ...] = ()
    # final_nuclei: extra nuclei (diphthongs) allowed only in a word's last syllable.
    final_nuclei: tuple[str, ...] = ()
    # initial_vowel: a word may start with a vowel-only syllable ("amira").
    initial_vowel: bool = False
    # max_clusters: at most this many onset clusters per word; 0 means no limit.
    max_clusters: int = 0
    # vowels: the single-vowel nuclei, a subset of aeiou in that order.
    vowels: str = "aeiou"
    # extra_nuclei: long vowels or diphthongs allowed anywhere in a word ("aa", "ie").
    extra_nuclei: tuple[str, ...] = ()
    # max_extra_nuclei: at most this many extra nuclei per word; 0 means no limit.
    max_extra_nuclei: int = 0
    # grow: when the split's words are full, add words of the last word's length.
    # Names are numbered in blocks by word count (all 2-word names first), so the
    # order is kept and there is no upper limit. Requires split; not with joinable.
    grow: bool = False

    def build(self) -> sa.SyllableCode:
        assert not self.grow or (self.split and not self.joinable), f"{self.name}: grow needs split, not joinable"
        onsets = list(self.onsets) + list(self.onset_clusters) + ([""] if self.initial_vowel else [])
        nuclei = list(self.vowels) + list(self.extra_nuclei) + list(self.final_nuclei)
        sa.check_unique_split(onsets, self.codas, nuclei)
        parts = {o + n + c: (o, n, c)
                 for o, n, c in product(onsets, nuclei, self.codas) if not (o and o == c)}
        syllables = list(parts)
        end = {s for s, (_, n, c) in parts.items() if n + c in self.end_suffixes}
        final_only = set(self.final_nuclei)
        clusters = set(self.onset_clusters)
        extras = set(self.extra_nuclei)

        dead = {x for x in self.end_suffixes if not any(n + c == x for _, n, c in parts.values())}
        assert not dead, f"{self.name}: dead suffixes {dead}"
        clash = end & fam.FAMILY_END
        assert not clash, f"{self.name}: endings overlap family endings {clash}"

        bl = self.blocklist

        def step(q, syl):
            onset, nucleus, coda = parts[syl]
            harmony = (q.harmony or sa.harmony_class(nucleus)) if self.harmony else None
            return sa.State(coda, harmony, onset,
                            q.clusters + (onset in clusters) if self.max_clusters else 0,
                            q.extras + (nucleus in extras) if self.max_extra_nuclei else 0,
                            bl.advance_head(q.head, syl) if bl else None)

        def allowed(q, syl, is_last):
            onset, nucleus, coda = parts[syl]
            first = onset[:1]                     # boundary rules look at the first consonant
            if is_last and syl not in end:
                return False
            if not onset and q.coda != "^":
                return False                      # vowel-only syllables only start a word
            if nucleus in final_only and not is_last:
                return False
            if self.max_clusters and onset in clusters and q.clusters >= self.max_clusters:
                return False
            if self.max_extra_nuclei and nucleus in extras and q.extras >= self.max_extra_nuclei:
                return False
            if bl and is_last and bl.word_blocked(q.head, syl):
                return False
            if self.harmony and q.harmony and sa.harmony_class(nucleus) not in (None, q.harmony):
                return False
            if onset and onset == q.onset:
                return False
            if q.coda in ("^", ""):
                return True
            if first == q.coda or coda:
                return False
            if (q.coda, first) in {("s", "z"), ("n", "m"), ("r", "l"), ("l", "r")}:
                return False
            if q.coda in sa.VOICED and first in sa.VOICELESS:
                return False
            if (q.coda in sa.VOICELESS and first in sa.VOICED
                    and (q.coda, first) not in sa.VOICING_EXCEPTIONS):
                return False
            return True

        return sa.SyllableCode(syllables, step, allowed, sa.START)


# Style definitions. Editing one changes its output, so it needs a new release.
SOFT_ONSETS, SOFT_CODAS = "blmnrvdgjwhsz", ("", "n", "l", "r", "s")
SOFT_ENDS = frozenset({"a", "i", "el", "an", "il", "ar", "is"})
STYLES = {
    # soft: vowel harmony, mostly voiced and liquid onsets, light codas
    "soft":    Style("soft",    onsets=SOFT_ONSETS, codas=SOFT_CODAS, end_suffixes=SOFT_ENDS, harmony=True,
                     blocklist=sa.BLOCKLIST1),
    "crisp":   Style("crisp",   onsets="bdfgkpstzh",  codas=("", "n", "r", "s"),
                     end_suffixes=frozenset({"is", "os", "ar", "on", "ir", "o"}), harmony=False, blocklist=sa.BLOCKLIST1),
    "flowing": Style("flowing", onsets="lmnrvsdtjw",  codas=("",),
                     end_suffixes=frozenset({"a", "o", "i"}), harmony=True, blocklist=sa.BLOCKLIST1),
    # lyric: a vowel-only first syllable and the diphthongs ai/oi as endings
    "lyric":   Style("lyric",   onsets="blmnrvdgjw",  codas=("", "n", "l"),
                     end_suffixes=frozenset({"a", "i", "el", "an", "il", "ai", "oi"}), harmony=True,
                     blocklist=sa.BLOCKLIST1, initial_vowel=True, final_nuclei=("ai", "oi")),
    # nordic: crisp plus onset clusters, one per word
    "nordic":  Style("nordic",  onsets="bdfgkpstzh",  codas=("", "n", "r", "s"),
                     end_suffixes=frozenset({"is", "os", "ar", "on", "ir", "o"}), harmony=False,
                     blocklist=sa.BLOCKLIST1,
                     onset_clusters=("br", "dr", "gr", "kr", "pr", "tr", "fr", "bl", "gl", "kl", "pl", "fl"),
                     max_clusters=1),
    # orkish: dark vowels, hard codas
    "orkish":  Style("orkish",  onsets="gkrzdbmn",    codas=("", "g", "k", "r", "z"), vowels="aou",
                     end_suffixes=frozenset({"ag", "ak", "uk", "ug", "ok", "ar", "or", "uz", "oz"}),
                     harmony=False, blocklist=sa.BLOCKLIST1),
    # elfish: light vowels, liquid onsets, at most one special vowel (ie, ae, io) per word
    "elfish":  Style("elfish",  onsets="lnmrvsfh",    codas=("", "n", "l", "r", "s"), vowels="aeio",
                     end_suffixes=frozenset({"iel", "ion", "es", "is", "a", "e", "il", "ir", "ael", "ien"}),
                     harmony=False, blocklist=sa.BLOCKLIST1, onset_clusters=("th",),
                     extra_nuclei=("ie", "ae", "io"), max_extra_nuclei=1, initial_vowel=True),
    # alien: long vowels, one onset cluster per word
    "alien":   Style("alien",   onsets="zkvtj",       codas=("", "n"), vowels="aiuo",
                     end_suffixes=frozenset({"aan", "uun", "oon", "on", "ii", "uu", "a", "i"}),
                     harmony=False, blocklist=sa.BLOCKLIST1, onset_clusters=("zv", "kv", "tl", "kt"),
                     max_clusters=1, extra_nuclei=("aa", "ii", "uu", "oo")),
    # soft-double: soft as a double given name of 2-syllable words that grows (bala-bema, bala-bema-lani)
    "soft-double": Style("soft", onsets=SOFT_ONSETS, codas=SOFT_CODAS, end_suffixes=SOFT_ENDS, harmony=True,
                         blocklist=sa.BLOCKLIST1, split=(2, 2), grow=True),
}


@dataclass(frozen=True)
class Release:
    label: str | None   # semver with build metadata naming the style, e.g. "1.0.0+soft"
    style: Style | None
    # RETIREMENT RULE (frozen): a retired release issues no new names, but its
    # existing names keep decoding. Retirement is one-way: an entry can go from
    # retired=False to retired=True in a later registry, never back.
    retired: bool = False


def placeholder() -> Release:
    """A slot skipped before it was ever used (e.g. its family names are offensive).

    It holds the slot's position so later releases don't land there; it never
    issues or decodes names."""
    return Release(None, None, retired=True)


# Frozen, append-only: the index is what names store. Never reorder, edit or remove.
REGISTRY = [
    Release("1.0.0+soft",        STYLES["soft"]),
    Release("1.0.0+crisp",       STYLES["crisp"]),
    Release("1.0.0+flowing",     STYLES["flowing"]),
    Release("1.0.0+lyric",       STYLES["lyric"]),
    Release("1.0.0+nordic",      STYLES["nordic"]),
    Release("1.0.0+orkish",      STYLES["orkish"]),
    Release("1.0.0+elfish",      STYLES["elfish"]),
    Release("1.0.0+alien",       STYLES["alien"]),
    Release("1.0.0+soft-double", STYLES["soft-double"]),
]

# semver 2.0 (semver.org), build metadata required
SEMVER_WITH_BUILD = re.compile(
    r"^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)"
    r"(?:-((?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*)"
    r"(?:\.(?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*))*))?"
    r"\+([0-9a-zA-Z-]+(?:\.[0-9a-zA-Z-]+)*)$"
)


def validate_registry(registry: list[Release]) -> None:
    half = [i for i, r in enumerate(registry) if (r.label is None) != (r.style is None)]
    assert not half, f"label and style must both be set or both be None: slots {half}"
    live_holes = [i for i, r in enumerate(registry) if r.label is None and not r.retired]
    assert not live_holes, f"placeholders must be retired: slots {live_holes}"
    labels = [r.label for r in registry if r.label is not None]
    bad = [lb for lb in labels if not SEMVER_WITH_BUILD.match(lb)]
    assert not bad, f"not semver with +build: {bad}"
    dupes = {lb for lb in labels if labels.count(lb) > 1}
    assert not dupes, f"duplicate labels: {dupes}"
    assert len(registry) <= fam.VERSIONS, f"{len(registry)} > {fam.VERSIONS} family slots"
    wrong_major = [lb for lb in labels if lb.split(".", 1)[0] != str(fam.GENERATION)]
    assert not wrong_major, f"major must equal generation {fam.GENERATION}: {wrong_major}"


def validate_evolution(old: list[Release], new: list[Release]) -> None:
    """A later registry may only append entries and retire existing ones."""
    assert len(new) >= len(old), f"entries removed: {len(old)} -> {len(new)}"
    for i, (a, b) in enumerate(zip(old, new)):
        what = "label" if a.label != b.label else "style"
        assert (a.label, a.style) == (b.label, b.style), f"slot {i} ({a.label}): {what} changed"
        assert not (a.retired and not b.retired), f"slot {i} ({a.label}) un-retired"


validate_registry(REGISTRY)
INDEX = {r.label: i for i, r in enumerate(REGISTRY) if r.label is not None}


class RetiredRelease(ValueError):
    pass


@lru_cache
def code_for(index: int) -> sa.SyllableCode:
    return REGISTRY[index].style.build()


def _shape(style: Style, words: int) -> tuple[int, ...]:
    """Syllables per word for a name of `words` given words (grow repeats the last length)."""
    return style.split + (style.split[-1],) * (words - len(style.split))


def _capacity(code: sa.SyllableCode, shape: tuple[int, ...]) -> int:
    total = 1
    for k in shape:
        total *= code.capacity(k)
    return total


def given_capacity(index: int) -> int | None:
    """Exact capacity of a split given name; None = unbounded (variable length or grow)."""
    style, code = REGISTRY[index].style, code_for(index)
    if not style.split or style.grow:
        return None
    return _capacity(code, style.split)


def _encode_fixed(code: sa.SyllableCode, shape: tuple[int, ...], x: int) -> str:
    parts = []
    for k in reversed(shape):                 # mixed radix, first part most significant
        x, digit = divmod(x, code.capacity(k))
        parts.append("".join(code.unrank(digit, k)))
    return "-".join(reversed(parts))


def encode_given(index: int, hi: int) -> str:
    style, code = REGISTRY[index].style, code_for(index)
    if not style.split:
        return code.encode(hi)
    if style.grow:
        words = len(style.split)
        while hi >= _capacity(code, _shape(style, words)):
            hi -= _capacity(code, _shape(style, words))
            words += 1
        return _encode_fixed(code, _shape(style, words), hi)
    cap = _capacity(code, style.split)
    if hi >= cap:
        raise OverflowError(f"{REGISTRY[index].label} holds {cap:,} given names, got {hi:,}")
    return _encode_fixed(code, style.split, hi)


# JOIN RULE (frozen format rule, identical for every release that sets joinable):
#   Encoding is unaffected: the canonical form always separates the given parts.
#   Decoding ignores every separator inside the given name: all words before the
#   family name are concatenated. A single-word release decodes the result as its
#   one given word. A split release re-splits it into the style's syllable
#   counts; exactly one split must parse, otherwise the name is rejected.
#   The family name is always the last word and must be intact. A published
#   release's joinable value never changes.

def _resplit(code: sa.SyllableCode, joined: str, split: tuple[int, ...]) -> list[list[str]]:
    """All ways to cut `joined` into consecutive words of the given syllable counts."""
    if not split:
        return [[]] if not joined else []
    k, rest = split[0], split[1:]
    found = []
    for cut in range(1, len(joined) + 1):
        try:
            syls = code.parse(joined[:cut])
        except ValueError:
            continue
        if len(syls) == k:
            found += [[joined[:cut], *tail] for tail in _resplit(code, joined[cut:], rest)]
    return found


def _canonical_parts(style: Style, code: sa.SyllableCode, label: str,
                     parts: list[str]) -> list[str]:
    """Apply the JOIN RULE."""
    split = style.split
    if style.joinable and not split:
        return ["".join(parts)]
    if style.joinable:
        candidates = _resplit(code, "".join(parts), split)
        if len(candidates) != 1:
            raise ValueError(f"{label}: {''.join(parts)!r} splits {len(candidates)} ways into {split}")
        return candidates[0]
    return list(parts)


def _decode_parts(style: Style, code: sa.SyllableCode, label: str, parts: list[str]) -> int:
    split = style.split
    parts = _canonical_parts(style, code, label, parts)
    if not split:
        if len(parts) != 1:
            raise ValueError(f"{label} expects 1 given part(s), got {len(parts)}")
        return code.decode(parts[0])
    if style.grow:
        if len(parts) < len(split):
            raise ValueError(f"{label} expects at least {len(split)} given part(s), got {len(parts)}")
        shape = _shape(style, len(parts))
        offset = sum(_capacity(code, _shape(style, n)) for n in range(len(split), len(parts)))
    else:
        if len(parts) != len(split):
            raise ValueError(f"{label} expects {len(split)} given part(s), got {len(parts)}")
        shape, offset = split, 0
    hi = 0
    for part, k in zip(parts, shape):
        syls = code.parse(part)
        if len(syls) != k:
            raise ValueError(f"{part!r}: expected {k} syllables, got {len(syls)}")
        hi = hi * code.capacity(k) + code.rank(syls)
    return offset + hi


def decode_given(index: int, parts: list[str]) -> int:
    r = REGISTRY[index]
    return _decode_parts(r.style, code_for(index), r.label, parts)


def encode(x: int, label: str, *, allow_retired: bool = False) -> str:
    """Canonical form: lowercase, hyphen-separated, family name last.

    allow_retired is for snapshots and tests only: it shows which name an ID
    had in a retired release."""
    index = INDEX[label]
    if REGISTRY[index].retired and not allow_retired:
        raise RetiredRelease(f"{label} is retired: its names still decode, but it issues no new ones")
    hi, lo = divmod(x, fam.K)
    return f"{encode_given(index, hi)}-{fam.family_name(index, lo)}"


def encode_short(x: int, label: str, *, allow_retired: bool = False) -> str:
    """Short form: given name(s) only, carrying the whole ID. Decoding needs the release."""
    index = INDEX[label]
    if REGISTRY[index].retired and not allow_retired:
        raise RetiredRelease(f"{label} is retired: its names still decode, but it issues no new ones")
    return encode_given(index, x)


# INPUT RULE (frozen format rule; widening it later would make decoders disagree):
#   1. Format characters are removed: soft hyphen, zero-width space/non-joiner/
#      joiner, BOM. They mark nothing the user typed, so they never separate words.
#   2. Separators: any whitespace (incl. no-break and thin spaces), any Unicode
#      dash punctuation (category Pd), minus sign, underscore, middle dot.
#      Runs of separators count as one; leading/trailing ones are ignored.
#   3. ASCII letters are lowercased; afterwards only a-z may remain. Everything else is
#      rejected: '.' and '+' (reserved), URL/path characters, apostrophes,
#      diacritics and look-alike letters from other scripts (never folded).
FORMAT_CHARS = frozenset("­​‌‍﻿")
EXTRA_SEPARATORS = frozenset("_·−")
RESERVED = frozenset(".+")


class InvalidName(ValueError):
    pass


def _is_separator(ch: str) -> bool:
    return ch.isspace() or ch in EXTRA_SEPARATORS or unicodedata.category(ch) == "Pd"


def normalize(name: str) -> list[str]:
    """Split a name into lowercase a-z words according to the INPUT RULE."""
    words, word = [], []
    for ch in name:
        if ch in FORMAT_CHARS:
            continue
        if _is_separator(ch):
            if word:
                words.append("".join(word))
                word = []
            continue
        low = chr(ord(ch) + 32) if "A" <= ch <= "Z" else ch   # ASCII only: never fold look-alikes
        if not "a" <= low <= "z":
            what = "reserved" if ch in RESERVED else "not allowed"
            raise InvalidName(f"{ch!r} (U+{ord(ch):04X} {unicodedata.name(ch, '?')}) {what}")
        word.append(low)
    if word:
        words.append("".join(word))
    if not words:
        raise InvalidName("empty name")
    return words


def decode(name: str, release: str | None = None) -> tuple[str, int]:
    """Decode a full or short form; returns (release label, ID).

    Full form: the last word parses as a family name. The release comes from the
    name; `release` is ignored, and the caller can compare the returned label.
    Short form: anything else. Needs `release`, which must be a known label."""
    words = normalize(name)
    try:
        index, lo = fam.family_parse(words[-1])
    except ValueError:
        if release is None:
            raise InvalidName(f"{name.strip()!r}: not a full form (no family name); "
                              "a short form needs a release") from None
        if release not in INDEX:
            raise InvalidName(f"unknown release {release!r}") from None
        return release, decode_given(INDEX[release], words)
    given, family = words[:-1], words[-1]
    if not given:
        raise InvalidName(f"{name.strip()!r}: family name without a given name")
    if index >= len(REGISTRY):
        raise ValueError(f"{family!r}: unknown release slot {index}")
    if REGISTRY[index].style is None:
        raise InvalidName(f"{family!r}: slot {index} is a placeholder and never issued names")
    return REGISTRY[index].label, decode_given(index, given) * fam.K + lo


if __name__ == "__main__":
    print(f"{len(REGISTRY)} of {fam.VERSIONS} family slots used\n")

    for i, r in enumerate(REGISTRY):
        c = code_for(i)
        if r.style.split:
            cap = given_capacity(i)
            shape = '+'.join(map(str, r.style.split))
            info = (f"split {shape}, grows by {r.style.split[-1]}-syllable words" if cap is None
                    else f"split {shape}: {cap:,} given names ({cap.bit_length() - 1}b)")
        else:
            info = "bits per length  " + "  ".join(
                f"{k}:{c.capacity(k).bit_length() - 1}b" for k in range(3, 7))
        print(f"[{i}] {r.label:<14} {len(c.states):>4} states   {info}")

    random.seed(7)
    xs = [random.randrange(0x1000000, 0x10000000) for _ in range(4)]
    print()
    for r in REGISTRY:
        names = []
        for x in xs:
            n = encode(x, r.label)
            assert decode(n) == (r.label, x)
            names.append(f"{n:<24}")
        print(f"{r.label:<14} " + "".join(names))
    print("\nround trip ok (release label and ID recovered from the name)")

    # 1.2.0+soft: every 7-digit hex ID fits, and display forms decode the same
    top = 0xFFFFFFF
    n = encode(top, "1.0.0+soft-double")
    print(f"\n{top:#09x} -> {n}")
    display = " ".join(n.title().rsplit("-", 1))          # "Hans-Peter Müller" shape
    for variant in [n.replace("-", " "), display, f"  {n.upper()}  ",
                    n.replace("-", "_")]:
        assert decode(variant) == ("1.0.0+soft-double", top), variant
        print(f"  accepted: {variant!r}")
    for broken in [n.replace("-", "", 1), n.split("-", 1)[1]]:
        try:
            decode(broken)
        except ValueError as e:
            print(f"  rejected: {broken!r}: {e}")
        else:
            raise SystemExit(f"accepted malformed name {broken!r}")

    # negative checks: the guards must reject these
    soft = STYLES["soft"]
    for broken, why in [
        (REGISTRY + [Release("1.0.0+soft", soft)], "duplicate label"),
        (REGISTRY + [Release("1.1.0", soft)], "missing +build"),
        (REGISTRY + [Release("v1.1.0+soft", soft)], "not semver"),
    ]:
        try:
            validate_registry(broken)
        except AssertionError as e:
            print(f"rejected ({why}): {e}")
        else:
            raise SystemExit(f"guard missed: {why}")

    # JOIN RULE: exercised on a throwaway joinable style (nothing added to REGISTRY)
    join_style = replace(STYLES["soft-double"], joinable=True, grow=False)
    join_code = join_style.build()
    strict_n = encode(top, "1.0.0+soft-double")
    given = strict_n.rsplit("-", 1)[0].split("-")             # ['danla', 'gilmi']
    hi = decode_given(INDEX["1.0.0+soft-double"], given)
    print("\njoinable variant of 1.2.0+soft:")
    for form in [given, ["".join(given)], [given[0][:3], given[0][3:] + given[1]]]:
        assert _decode_parts(join_style, join_code, "test+soft.joinable", form) == hi
        print(f"  accepted: {'-'.join(form)!r}")
    # exhaustive: every joined 2+2 given name re-splits exactly one way
    k1, k2 = join_style.split
    count = 0
    for a in range(join_code.capacity(k1)):
        p1 = "".join(join_code.unrank(a, k1))
        for b in range(0, join_code.capacity(k2), 7):
            p2 = "".join(join_code.unrank(b, k2))
            assert len(_resplit(join_code, p1 + p2, join_style.split)) == 1, p1 + p2
            count += 1
    print(f"  unique re-split for {count:,} joined names (every 7th second part)")

    # INPUT RULE: one case per character class
    print("\ninput rule:")
    expect = ("1.0.0+soft-double", top)
    accepted = {
        "hyphen U+2010":          "danla‐gilmi‐vupav",
        "non-breaking hyphen":    "danla‑gilmi‑vupav",
        "figure dash U+2012":     "danla‒gilmi‒vupav",
        "en dash":                "danla–gilmi–vupav",
        "em dash":                "danla—gilmi—vupav",
        "minus sign":             "danla−gilmi−vupav",
        "no-break space":         "danla gilmi vupav",
        "thin space":             "danla gilmi vupav",
        "narrow no-break space":  "danla gilmi vupav",
        "tab and doubled spaces": "danla\t gilmi  vupav",
        "middle dot":             "danla·gilmi·vupav",
        "underscore":             "danla_gilmi_vupav",
        "mixed separators":       "danla – gilmi_ vupav",
        "soft hyphen in a word":  "dan­la-gil­mi-vupav",
        "zero-width chars":       "﻿danla​-gil‍mi-vu‌pav",
        "mixed case":             "DaNlA-gIlMi-VuPaV",
        "surrounding separators": " —danla-gilmi-vupav— ",
    }
    # the literals above use "vupav" as a placeholder family name; swap in the real one
    F = encode(top, "1.0.0+soft-double").rsplit("-", 1)[1]
    accepted = {k: v.replace("vupav", F) for k, v in accepted.items()}
    # and "danla", "gilmi" as placeholder given words; swap in the real ones too
    G1, G2 = encode(top, "1.0.0+soft-double").split("-")[:2]
    accepted = {k: v.replace("danla", G1).replace("gilmi", G2) for k, v in accepted.items()}
    accepted["soft hyphen in a word"] = (G1[:3] + "\u00ad" + G1[3:] + "-" + G2[:3] + "\u00ad" + G2[3:]
                                         + "-" + F)
    accepted["zero-width chars"] = ("\ufeff" + G1 + "\u200b-" + G2[:3] + "\u200d" + G2[3:] + "-"
                                    + F[:2] + "\u200c" + F[2:])
    accepted["mixed case"] = G1.upper() + "-" + G2.title() + "-" + "".join(
        c.upper() if i % 2 == 0 else c for i, c in enumerate(F))
    for what, variant in accepted.items():
        assert decode(variant) == expect, (what, variant)
        print(f"  accepted  {what}")

    rejected = {
        "dot (reserved)":         "danla.gilmi-vupav",
        "plus (reserved)":        "danla-gilmi+vupav",
        "slash":                  "danla/gilmi-vupav",
        "colon":                  "danla:gilmi-vupav",
        "at sign":                "danla@gilmi-vupav",
        "hash":                   "danla-gilmi#vupav",
        "question mark":          "danla-gilmi-vupav?",
        "ampersand":              "danla&gilmi-vupav",
        "apostrophe":             "dan'la-gilmi-vupav",
        "right single quote":     "dan’la-gilmi-vupav",
        "okina":                  "danʻla-gilmi-vupav",
        "cyrillic a":             "dаnla-gilmi-vupav",
        "greek omicron":          "danla-gilmi-vupοv",
        "u with diaeresis":       "danla-gilmi-vüpav",
        "combining diaeresis":    "danla-gilmi-vüpav",
        "digit":                  "danla-gilmi-vupav2",
        "soft hyphen as separator": "danla­gilmi-vupav",   # strict release: one given word
        "family name only":       "vupav",
    }
    rejected = {k: v.replace("vupav", F) for k, v in rejected.items()}
    vowel = next(i for i, c in enumerate(F) if c in "aeiou")
    rejected["greek omicron"] = "danla-gilmi-" + F[:vowel] + "ο" + F[vowel + 1:]
    rejected["u with diaeresis"] = "danla-gilmi-" + F[:vowel] + "ü" + F[vowel + 1:]
    rejected["combining diaeresis"] = "danla-gilmi-" + F[:vowel + 1] + "̈" + F[vowel + 1:]
    rejected["proquint"] = "lusab-babad"
    for what, variant in rejected.items():
        try:
            decode(variant)
        except ValueError as e:
            print(f"  rejected  {what:<26} {e}")
        else:
            raise SystemExit(f"accepted {what}: {variant!r}")

    # older single-word releases go through the same rule
    for i, r in enumerate(REGISTRY[:6]):
        n = encode(xs[0], r.label)
        assert decode(n.replace("-", "–").upper()) == (r.label, xs[0])
    print("  older releases decode through the same rule")

    # JOIN RULE for a single-word release: throwaway joinable copy of 1.0.0+soft
    single = replace(STYLES["soft"], joinable=True)
    single_code = single.build()
    strict = encode(2**64 - 1, "1.0.0+soft")              # the snapshot row
    word = strict.rsplit("-", 1)[0]
    hi = decode_given(INDEX["1.0.0+soft"], [word])
    print(f"\njoinable variant of 1.0.0+soft ({strict}):")
    for form in [[word], [word[:5], word[5:10], word[10:]],
                 [word[i:i + 3] for i in range(0, len(word), 3)]]:
        assert _decode_parts(single, single_code, "test+soft.joinable", form) == hi
        print(f"  accepted: {'-'.join(form)!r}")
    # the strict original still rejects the same input
    try:
        decode("baran-geven-" + strict[10:])
    except ValueError as e:
        print(f"  strict 1.0.0+soft still rejects it: {e}")
    else:
        raise SystemExit("strict release accepted a broken-up given name")
    # damaged letters stay errors: a dash can be ignored, a missing letter cannot
    try:
        _decode_parts(single, single_code, "test+soft.joinable", [word[:5], word[6:]])
    except ValueError as e:
        print(f"  rejected (letter lost): {e}")
    else:
        print("  NOTE: a dropped letter still decoded (to a different ID)")

    # major-version rule
    try:
        validate_registry(REGISTRY + [Release("2.0.0+soft", STYLES["soft"])])
    except AssertionError as e:
        print(f"\nrejected (major != generation): {e}")
    else:
        raise SystemExit("major-version rule missed 2.0.0")

    # RETIREMENT RULE: a trial registry that retires 1.0.0+lyric, adds a placeholder
    # and a new release after it (the real REGISTRY is restored afterwards)
    print("\nretirement:")
    real_registry, real_index = REGISTRY, INDEX
    old_name = encode(xs[0], "1.0.0+lyric")
    trial = (real_registry[:3] + [replace(real_registry[3], retired=True)] + real_registry[4:]
             + [placeholder(), Release("1.1.0+crisp", STYLES["crisp"])])
    validate_registry(trial)
    validate_evolution(real_registry, trial)
    REGISTRY, INDEX = trial, {r.label: i for i, r in enumerate(trial) if r.label is not None}
    code_for.cache_clear()
    try:
        assert decode(old_name) == ("1.0.0+lyric", xs[0])
        print(f"  retired release still decodes: {old_name!r}")
        try:
            encode(xs[0], "1.0.0+lyric")
        except RetiredRelease as e:
            print(f"  encode refused: {e}")
        else:
            raise SystemExit("retired release issued a name")
        hole = "ba-" + fam.family_name(len(real_registry), 0)
        try:
            decode(hole)
        except InvalidName as e:
            print(f"  placeholder slot: {e}")
        else:
            raise SystemExit("placeholder slot decoded")
        new_name = encode(xs[0], "1.1.0+crisp")
        assert decode(new_name) == ("1.1.0+crisp", xs[0])
        print(f"  same ID re-issued in slot {len(real_registry) + 1}: {new_name!r}")
    finally:
        REGISTRY, INDEX = real_registry, real_index
        code_for.cache_clear()

    # evolution violations, each tested on its own
    unretired = [replace(r, retired=False) if i == 3 else r for i, r in enumerate(trial)]
    for old, new, why in [
        (real_registry, real_registry[:-1], "entry removed"),
        (real_registry, real_registry[:3] + [replace(real_registry[3], style=STYLES["soft"])]
         + real_registry[4:], "style edited"),
        (real_registry, real_registry[:3] + [replace(real_registry[3], label="1.0.1+lyric")]
         + real_registry[4:], "label edited"),
        (trial, unretired, "un-retired"),
        (trial, trial[:len(real_registry)] + [Release("1.2.0+soft", STYLES["soft"])]
         + trial[len(real_registry) + 1:],
         "placeholder reused"),
    ]:
        try:
            validate_evolution(old, new)
        except AssertionError as e:
            print(f"  rejected ({why}): {e}")
        else:
            raise SystemExit(f"evolution guard missed: {why}")

    # SHORT FORM
    print("\nshort form:")
    for r in REGISTRY:
        for x in xs + [0, 1, 63, 64]:
            try:
                s = encode_short(x, r.label)
            except OverflowError:
                continue
            assert decode(s, r.label) == (r.label, x), (r.label, x, s)
        try:
            short = encode_short(xs[0], r.label)
        except OverflowError:
            short = "<overflow>"
        print(f"  {r.label:<14} {short:<16} full: {encode(xs[0], r.label)}")
    s = encode_short(xs[0], "1.0.0+soft")
    try:
        decode(s)
    except InvalidName as e:
        print(f"  rejected without release: {e}")
    else:
        raise SystemExit("short form decoded without a release")
    full = encode(xs[0], "1.0.0+crisp")
    assert decode(full, "1.0.0+soft") == ("1.0.0+crisp", xs[0])
    print(f"  full form ignores the context release: {full!r} -> 1.0.0+crisp")
    # soft-double grows instead of overflowing: two words, then three, ...
    for x in (2**23, 2**40, 2**64 - 1):
        s_ = encode_short(x, "1.0.0+soft-double")
        assert decode(s_, "1.0.0+soft-double") == ("1.0.0+soft-double", x)
        print(f"  soft-double grows: {x:#x} -> {s_}")
