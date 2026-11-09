# phonym

Pronounceable, versioned names for integers, and back.

```
4711        → badil-durjav            (1.1.0+soft)
0x62e6b43   → manwudoma-durmav
0x62e6b43   → besanzoso-vorjas        (1.1.0+crisp)
4711        → bala-bema-wonzus        (1.2.0+soft, double given name)
```

phonym maps every non-negative integer to a name made of a **given name** and a
**family name**, and every name back to its integer. The mapping is an
order-preserving bijection, has no upper limit (256-bit IDs work), and is defined
by a frozen, versioned spec: [SPEC.md](SPEC.md).

> **Status:** pre-release. The spec is a draft and not yet frozen, so names may
> still change before 1.0.

## Why another one

- **Names describe themselves.** The family name carries the *release*, the spec
  version the name was made with. `badil-durjav` decodes without any
  configuration, and a decoder can tell when a name is newer than it knows.
- **Releases make the vocabulary evolvable.** Sound rules, styles and (planned)
  blocklists change through new releases. Old names keep decoding; an ID can be
  re-issued under a newer release and both names still decode to the same ID.
  A release can be retired: it stops issuing names, its names keep working.
- **Real phonotactics.** Names come from a finite automaton over syllables with
  context-sensitive rules: vowel harmony, consonant clusters, voicing, no
  stuttering, chosen endings. Ranking over the automaton keeps the mapping exact
  and dense: no retries, no gaps, no bias.
- **Never a proquint.** Family names always have the shape CVC-CVC (6 letters),
  so a phonym name can never be mistaken for a proquint.
- **Short form.** Where the release is known from context, the given name alone
  (`bajoba`) is about as short as fixed-length schemes, and keeps growing past
  their limits.

## Install

```sh
go install github.com/iilei/phonym/cmd/phonym@latest
```

## Command line

```sh
phonym 4711                          # badil-durjav   (default release 1.1.0+soft)
phonym -release 1.1.0+crisp 4711     # bago-voslav
phonym -short 4711                   # bajoba
phonym decode "Badil Durjav"         # 4711  1.1.0+soft  full
phonym decode -release 1.1.0+soft bajoba
phonym releases                      # list the registry
```

IDs may be decimal or prefixed with `0x`, `0o` or `0b`. Decoding accepts any case
and common separators (spaces, dashes of all kinds, underscores); see the INPUT
RULE in the spec.

## Library

```go
name, err := phonym.EncodeUint64(4711, "1.1.0+crisp") // "bago-voslav"
short, err := phonym.EncodeShort(big.NewInt(4711), "1.1.0+crisp") // "bagando"

d, err := phonym.Decode("bago-voslav", "")      // d.ID = 4711, d.Release = "1.1.0+crisp"
d, err = phonym.Decode("bagando", "1.1.0+crisp") // short forms need the release
```

Errors wrap one of `ErrInvalidName`, `ErrUnsupportedFormat`,
`ErrUnsupportedRelease`, `ErrUnknownRelease`, `ErrRetiredRelease` and
`ErrOverflow`; use `errors.Is`.

phonym preserves order and does not scramble IDs. If sequential IDs should not
look sequential, permute them yourself before encoding (for example with a
Feistel network or `(a·x + c) mod n`).

## Releases

| Slot | Release | Given name |
|---|---|---|
| 0–2 | `1.0.0+soft`, `1.0.0+crisp`, `1.0.0+flowing` | one word, variable length |
| 3–5 | `1.1.0+soft`, `1.1.0+crisp`, `1.1.0+flowing` | one word, variable length |
| 6 | `1.2.0+soft` | two words of 2 syllables (`bala-bema`) |

The semver major equals the family-name *generation* (the shape CVC-CVC is
generation 1). The registry is append-only.

## Conformance

`testdata/family_snapshot.txt` and `testdata/names_snapshot.txt` are golden
files. The Go tests reproduce them byte for byte, and so does the Python
prototype the design was developed in (`reference/python/`). Any port must do
the same.

```sh
go test ./...                                  # includes the golden files
python3 reference/python/snapshot.py --check   # the reference agrees
```

## Prior art

- [Proquint](https://arxiv.org/html/0901.4016) (Wilkerson, 2009; now an
  [IETF draft](https://www.ietf.org/archive/id/draft-rayner-proquint-11.html)):
  fixed CVCVC words of 16 bits. The starting point.
- [phonid](https://github.com/iilei/phonid): the predecessor, with configurable
  CV templates and pools. Unmaintained in favour of phonym.
- [Babbler](https://github.com/verhas/babbler): fixed 4-syllable names for IDs up
  to 1,643,523, scrambled, with a small blocklist and no decoder.
- Koremutake, Urbit's `@p`, Bubble Babble, FIPS-181: syllable-based encodings and
  pronounceable password generators with fixed syllable sets or without exact
  reversibility.

## License

[MIT](LICENSE)
