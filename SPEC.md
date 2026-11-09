# phonym spec (draft)

Status: draft, not yet frozen. The Go package in this repository is the reference
implementation; `reference/python/` holds the prototype the design was worked out
in. Both reproduce `testdata/` exactly. A pure Python port is planned.
"Frozen" marks rules that can never change once names are published.

## 1. Core encoding

- Order-preserving bijection between non-negative integers and names: encoding
  unranks a path through a finite automaton over syllables, decoding ranks it.
- No built-in scrambling. Users who want non-sequential-looking names permute
  the ID themselves before encoding.
- `count[n][q]` is the number of valid n-syllable continuations from state `q`.
  Prefix sums over each state's successor list plus a binary search pick each
  syllable. States are discovered by search from `START`.
- Variable length: all shorter names come before longer ones; `encode(0)` is the
  shortest name.
- Covers all non-negative integers (no upper limit) for variable-length shapes.
  Count tables use big integers once they outgrow 64 bits.

## 2. Syllables (base inventory)

- Shape: one required onset consonant, one vowel, at most one coda consonant.
  This makes splitting a word into syllables unambiguous.
- Onsets `bdfgklmnprstvzhjw`; vowels `aeiou`; codas none, `n r s l v`.
- Syllables with onset = coda are excluded from the inventory (`rer`, `sis`).
- The inventory order is defined by the lists above (onset, then vowel, then coda)
  and fixes the order of names.

## 3. State and rules

- State: (previous coda, harmony class, previous onset, blocklist tail). Transition `step(q, syl)`.
- Position rules use the remaining length: the last syllable must be in END;
  `START` selects the initial inventory.

| Rule             | Detail                                                                                      |
| ---------------- | ------------------------------------------------------------------------------------------- |
| Vowel harmony    | `e i` front, `o u` back, `a` neutral; first non-neutral vowel fixes the class (per style)   |
| Stutter          | no two consecutive syllables with the same onset                                            |
| Doubling         | coda ≠ next onset                                                                           |
| Closed syllables | no two closed syllables in a row (given names only)                                         |
| Banned clusters  | `s‑z`, `n‑m`, `r‑l`, `l‑r`                                                                  |
| Voicing          | voiced coda before voiceless onset banned; voiceless before voiced banned except `sb sd sg` |
| END              | only listed vowel+coda endings may close a word                                             |

Voiced consonants are `bdgvz`, voiceless ones `ptkfsh`.

BLOCKLIST RULE (frozen): a blocklist is a versioned file of fragments
(`blocklists/<n>.txt`: one per line, lowercase `a–z`, at least 3 letters, `#`
comments). A syllable is not allowed if the word so far plus the syllable contains
a fragment. The state therefore also keeps `tail`, the longest suffix of the word
that is a proper prefix of some fragment: any fragment the next syllable completes
starts inside `tail + syllable`, so checking that string is enough. The rule
applies per word: separators reset it, so a fragment spanning two words is not
blocked. Each style names one blocklist version; extending the list means a new
file and new releases. `blocklists/1.txt` (315 fragments, 22 languages) applies to
all releases and to the family code. It combines hand-curated fragments with ones
adapted from LDNOOBW (CC BY 4.0): transliterated as a reader would say them,
reduced to what phonym can spell, minimised, and reviewed by hand to remove
ordinary words, names, misread transliterations and neutral or medical terms. The
file's header records sources, commit, changes and its license (CC BY 4.0).

## 4. Name structure: given name(s) + family name

- The family name is always the last word. It carries the release slot and the
  ID's low bits: `family number = slot · K + (x mod K)`.
- The family code is frozen and keeps its own copy of inventory and rules.
- SHAPE RULE (frozen): every family name is CVC-CVC (6 letters, consonant pair in
  the middle). Inside family names the first syllable must be closed and the
  "no two closed in a row" rule does not apply. Family endings:
  `en in ov av as us ul`, disjoint from all given-name endings.
- Proquint separation: proquint words are always 5 letters of strict CVCVC, so a
  family name, and therefore a name, can never be a proquint.
- Slot spreading: `slot' = (slot · STRIDE + OFFSET) mod SLOTS`; siblings
  (`x mod K`) stay adjacent, so sorting by given name then family name gives ID order.
- The family code applies blocklist 1 once, at the freeze. A later blocklist can't
  reach family names; a slot whose names turn out offensive can only be retired.
- Frozen constants: `K = 64`, `FAMILY_LEN = 2`, capacity 24,572, `SLOTS = 383`,
  `STRIDE = 237`, `OFFSET = 97`.
- Only `SLOTS · K = 24,512` family names are ever issued. The 60 names with a
  higher index are rejected when decoding (`UnsupportedFormat`); otherwise they
  would wrap around and act as second names for existing IDs.
- GENERATION RULE (frozen): the family-name shape identifies the generation, and
  the generation is the semver major of every release. CVC-CVC is generation 1.
  A future generation must use a different shape (e.g. CVV-CVC), which
  generation-1 decoders reject.

## 5. Releases

- The registry is a frozen, append-only list; a name stores the list position,
  never the label.
- Labels are semver with the style as build metadata: `1.1.0+crisp`. The major
  must equal the generation. Editing a style means a new release; old style
  definitions stay so old names keep decoding.
- Registry checks: semver with `+build`, unique labels, at most `SLOTS` entries,
  major = generation.
- A decoder that doesn't know a slot rejects the name (needs a newer registry).
- RETIREMENT RULE (frozen): each entry has a `retired` flag. A retired release
  issues no new names (`RetiredRelease`), but its existing names keep decoding.
  Retirement is one-way: an entry can go from not retired to retired in a later
  registry, never back.
- Placeholders: an entry with no label and no style, always retired. It keeps a
  slot from ever being used, e.g. when that slot's family names turn out to be
  offensive before any release used it. Names in a placeholder slot are rejected
  (`InvalidName`).
- Registry evolution: a later registry may only append entries and retire
  existing ones. Removing an entry, editing a label or style, un-retiring, and
  reusing a placeholder are all rejected.
- Retirement only affects encoding, so a decoder with an older registry still
  decodes names of releases retired since. An encoder with an older registry
  could still issue names in a release retired since; keep encoders current.

| #   | Label                      | Notes                                       |
| --- | -------------------------- | ------------------------------------------- |
| 0–2 | `1.0.0+soft/crisp/flowing` | original styles                             |
| 3–5 | `1.1.0+soft/crisp/flowing` | extra onsets: soft/flowing `j w`, crisp `h` |
| 6   | `1.2.0+soft`               | `split = (2, 2)`                            |

## 6. Styles

- Fields: `onsets, codas, end_suffixes, harmony, split, joinable`; one automaton per style.
- `split`: shape of the given name in syllables per word. `()` is one word of
  variable length. `(2, 2)` is two fixed words combined like digits
  (word1 · capacity + word2); fixed capacity, overflow error beyond it. The
  decoder expects exactly `len(split)` given words.
- `joinable`: decoding ignores separators inside the given name (see JOIN RULE).
  Allowed for single-word and split releases. No registered release uses it yet.
- Build-time checks: no dead endings; no overlap with family endings.

## 7. Text form

- Canonical form: lowercase `a–z` words joined by hyphens, family name last
  (`danla-gilmi-worfov`). Title case or spaces are display forms only.
- INPUT RULE (frozen):
  1. Remove format characters: U+00AD, U+200B–U+200D, U+FEFF.
  2. Separators: whitespace, dash punctuation, U+2212, `_`, U+00B7; runs count as one.
  3. ASCII letters `A–Z` are lowercased; afterwards only `a–z` may remain.
     Lowercasing is ASCII only: Unicode case mapping would fold look-alikes such
     as the Kelvin sign U+212A into `k`. `.` and `+` are reserved; URL
     characters, apostrophes, diacritics, look-alike letters and digits are
     rejected, never folded.
  The separator and format-character tables are explicit code point lists
  (`input.go`, generated once from Unicode 16.0 whitespace and dash punctuation).
- JOIN RULE (frozen, per release via `joinable`): output unchanged. Decoding
  joins all words before the family name; a single-word release decodes the
  result as its one given word, a split release re-cuts it at its syllable
  counts (exactly one cut must work). Misplaced hyphens are accepted on purpose.
  The family name must be intact. A release's `joinable` value never changes.
- Compare canonical forms or decoded `(release, id)`, never raw strings.

## 8. Short form

- The **full form** (`bagijan-dusfav`) describes itself: the family name carries
  the release. The **short form** (`banranolwa`) is given name(s) only; the
  release comes from context (a configured default, the field or URL it appears
  in). Use the full form for storage, APIs and anywhere a name must stand alone;
  use the short form for display where the release is known.
- Layout: the short form is not "the full form without the family name". The
  full form stores `x mod K` in the family name, so the short form encodes the
  whole ID in the given name: `short = given(x)`, while `full = given(x div K) +
  family(slot, x mod K)`. Converting between the two needs the release and is
  lossless.
- Same style, same rules: split shapes, `joinable`, the INPUT and JOIN rules
  apply unchanged. Order is preserved within a release.
- Capacity: fixed shapes hold K times fewer IDs in the short form. For example
  `1.2.0+soft` (2+2) holds 6,943,225 IDs short instead of 444,366,400 full.
  Variable-length shapes have no limit in either form.
- Retired releases issue no short forms either.
- Telling the forms apart: a name is in full form exactly when its last word
  parses as a family name (SHAPE RULE plus a family ending). Given-name endings
  never overlap family endings (a build-time check for every style), so a
  short-form last word can never parse as a family name. Decoders therefore
  accept both forms: full form without context, short form only with a release
  given by the caller.
- Decoding API: `decode(name, release=None) -> (release, id)`. A full form
  always decodes as the release in its family name and ignores `release`; the
  caller compares the returned label if the context expects a specific release.
  Anything else is treated as a short form of `release`, and is rejected when no
  release is given.
- With a release given, a string that isn't a full form is tried as a short form.
  So a proquint or a full form with a broken family name can decode as a short
  form of that release (most likely with `joinable`). Without a release, both
  are rejected.
- No guarantees the full form has: a short form doesn't identify its release,
  the same short form can decode in several releases, and it can be a valid
  proquint (measured: up to 2.2% for `1.2.0+soft`). Machine-to-machine use must
  use the full form.
- Length, IDs uniform in Babbler's range 0 to 1,643,523 (Babbler: always 9
  characters):

  | Release         | Full | Short |
  | --------------- | ---- | ----- |
  | `1.1.0+flowing` | 12.9 | 9.0   |
  | `1.1.0+crisp`   | 14.1 | 9.0   |
  | `1.1.0+soft`    | 13.8 | 9.1   |
  | `1.2.0+soft`    | 17.2 | 10.4  |

  Short forms are about as long as Babbler's names on that range; shorter for
  small IDs, and they keep going past 1.6M.

## 9. Portability (frozen requirements for every implementation)

All implementations (Go reference, Python port, any later port) must produce
identical names. The spec therefore never relies on language behaviour:

1. **Constants are literals.** Values derived by a formula are written into the
   spec as numbers, and implementations use the literal. In particular
   `STRIDE = 237` is frozen as a number; the golden-ratio formula
   (`round(SLOTS · 0.6180339887)`, then the next value coprime to `SLOTS`) is
   only a comment explaining its origin. No floating point anywhere in
   encoding or decoding.
2. **Every order is defined explicitly.** The syllable order comes from the
   inventory lists (onset, vowel, coda, in list order); each state's successors
   are ordered by syllable order. Implementations must not iterate hash maps
   (Go maps, Python sets) where order affects output. The order in which states
   are discovered does not affect any count, so it is left free.
3. **Unicode handling uses an explicit table.** The INPUT RULE's separators and
   format characters are a fixed list of code points (`input.go`), not a query of
   Unicode properties (`isspace`, category `Pd`, Go's `unicode.IsSpace`), whose
   results change with Unicode versions and differ between runtimes. (The Python
   prototype still queries properties; a Python port must use the table.)
4. **Parsing is iterative.** Splitting a word into syllables must not use
   recursion, so long names (e.g. 256-bit IDs, around 40 syllables) can't hit a
   recursion limit (Python's is about 1,000). The algorithm must find all
   parses, so ambiguity is still detected.

Also required:

- Big integers wherever counts or IDs can exceed 64 bits (Go `math/big`, Python
  `int`). Implementations may use a 64-bit fast path, but it must give exactly
  the same names as the big-integer path.
- Only `divmod` on non-negative integers; negative IDs are rejected.
- No hash functions anywhere in the format.

## 10. Conformance

- `family_snapshot.txt`: every family name with slot, low bits and spread index.
  The header records the layout constants and a sha256 over the rows.
- `names_snapshot.txt`: short and full form for 48 fixed IDs per release:
  boundary values up to 2²⁵⁶ − 1 and seeded random IDs of 1–16 hex digits,
  including `<overflow>` rows for fixed-shape releases.
- The sample IDs are data: implementations read them from the file's id column,
  so no port needs the Python random generator.
- Fixed-width tables, no trailing whitespace. Every implementation must reproduce
  both files byte for byte. CI checks the Go implementation (`go test`) and the
  Python reference (`reference/python/snapshot.py --check`) on every change.

## 11. Decided against

- Built-in scrambling (users permute IDs themselves).
- A check syllable. If ever needed, the preferred form is integer arithmetic,
  `((x + index) mod 251) mod len(candidates)`, as a feature of a new release.

## 12. Open items

1. ~~Short form~~: done, see section 8.
2. ~~Slot retirement flag~~: done, see the RETIREMENT RULE in section 5.
3. ~~Blocklist~~: done, see the BLOCKLIST RULE in section 3 (`blocklists/1.txt`,
   315 fragments). Review the list before the freeze: after it, family names can't
   be changed any more.
4. Frozen copies for styles: in Go, released styles reference the generation-1
   rule tables (`rules1`) explicitly, separate from the family code's own copy.
   A rule change must come as a new rules table and new releases, never an edit.
5. ~~Typed errors~~: done in Go (`ErrInvalidName`, `ErrUnsupportedFormat`,
   `ErrUnsupportedRelease`, `ErrUnknownRelease`, `ErrRetiredRelease`, `ErrOverflow`).
6. The prototype's base END set still contains entries that match nothing or are
   redundant (`iel`, `mar`). It isn't used by any release; drop it from the prototype.
7. ~~Project~~: phonym succeeds phonid, which is unmaintained.
