# phonym

Pronounceable, versioned names for integers, and back.

```text
4711        → badil-duljav            (1.1.0+soft)
0x62e6b43   → manwuduma-dulmav
0x62e6b43   → besanhagon-vunfus       (1.1.0+crisp)
4711        → bala-bema-wovlov        (1.2.0+soft, double given name)
```

phonym maps every non-negative integer to a name made of a **given name** and a
**family name**, and every name back to its integer. The mapping is an
order-preserving bijection, has no upper limit (256-bit IDs work), and is defined
by a frozen, versioned spec: [SPEC.md](SPEC.md).

> **Status:** pre-release. The spec is a draft and not yet frozen, so names may
> still change before 1.0.

## Why another one

- **Names describe themselves.** The family name carries the *release*, the spec
  version the name was made with. `badil-duljav` decodes without any
  configuration, and a decoder can tell when a name is newer than it knows.
- **Releases make the vocabulary evolvable.** Sound rules, styles and the
  blocklist change through new releases. Old names keep decoding; an ID can be
  re-issued under a newer release and both names still decode to the same ID.
  A release can be retired: it stops issuing names, its names keep working.
- **Real phonotactics.** Names come from a finite automaton over syllables with
  context-sensitive rules: vowel harmony, consonant clusters, voicing, no
  stuttering, chosen endings. Ranking over the automaton keeps the mapping exact
  and dense: no retries, no gaps, no bias.
- **No swearwords.** A versioned blocklist (`blocklists/1.txt`, 22 languages) keeps
  sexual, bodily and gory terms and slurs from being a word of a name, or nearly one:
  an entry is accepted only inside a word with at least two more letters. Family names
  are included; plain pejoratives are allowed. Extending it means new
  releases; existing names keep decoding.
- **Never a proquint.** Family names always have the shape CVC-CVC (6 letters),
  so a phonym name can never be mistaken for a proquint.
- **Short form.** Where the release is known from context, the given name alone
  (`bajowa`) is about as short as fixed-length schemes, and keeps growing past
  their limits.

## Install

```sh
go install github.com/iilei/phonym/cmd/phonym@latest
```

## Command line

```sh
phonym 4711                          # badil-duljav   (default release 1.1.0+soft)
phonym -release 1.1.0+crisp 4711     # bago-vunzov
phonym -short 4711                   # bajowa
phonym decode "Badil Duljav"         # 4711  1.1.0+soft  full
phonym decode -release 1.1.0+soft bajowa
phonym releases                      # list the registry
```

IDs may be decimal or prefixed with `0x`, `0o` or `0b`. Decoding accepts any case
and common separators (spaces, dashes of all kinds, underscores); see the INPUT
RULE in the spec.

## Library

```go
name, err := phonym.EncodeUint64(4711, "1.1.0+crisp") // "bago-vunzov"
short, err := phonym.EncodeShort(big.NewInt(4711), "1.1.0+crisp") // "bagekir"

d, err := phonym.Decode("bago-vunzov", "")      // d.ID = 4711, d.Release = "1.1.0+crisp"
d, err = phonym.Decode("bagekir", "1.1.0+crisp") // short forms need the release
```

Errors wrap one of `ErrInvalidName`, `ErrUnsupportedFormat`,
`ErrUnsupportedRelease`, `ErrUnknownRelease`, `ErrRetiredRelease` and
`ErrOverflow`; use `errors.Is`.

phonym preserves order and does not scramble IDs. If sequential IDs should not
look sequential, permute them yourself before encoding (for example with a
Feistel network or `(a·x + c) mod n`).

## Releases

| Slot | Release                                      | Given name                                            |
| ---- | -------------------------------------------- | ----------------------------------------------------- |
| 0–2  | `1.0.0+soft`, `1.0.0+crisp`, `1.0.0+flowing` | one word, variable length                             |
| 3–5  | `1.1.0+soft`, `1.1.0+crisp`, `1.1.0+flowing` | one word, variable length                             |
| 6    | `1.2.0+soft`                                 | two words of 2 syllables (`bala-bema`)                |
| 7    | `1.3.0+lyric`                                | melodic: may start with a vowel, may end in `ai`/`oi` |
| 8    | `1.3.0+nordic`                               | hard: one onset cluster per word (`kobrokofako`)      |
| 9    | `1.4.0+orkish`                               | dark and harsh (`bazgorunuk`)                         |
| 10   | `1.4.0+elfish`                               | light and flowing (`vaelmoniltha`)                    |
| 11   | `1.4.0+alien`                                | strange, long vowels (`ktakovaakon`)                  |

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

## Development

Tool versions are pinned in `mise.toml`, and CI runs the same mise tasks, so a failure reproduces locally with the
same command.

```sh
mise install          # Go, golangci-lint, rumdl, goreleaser, pre-commit
mise run hooks        # install the git hooks (.pre-commit-config.yaml)
mise run check        # fmt:check, vet, lint, lint:md, golden:check, test (race), build
mise run golden:update  # after a format change: regenerate testdata/ from the reference, check Go agrees
mise run release:snapshot  # build the release archives locally into dist/release
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

[MIT](LICENSE), except `blocklists/`: CC BY 4.0, because the lists contain
fragments adapted from the [List of Dirty, Naughty, Obscene, and Otherwise Bad
Words](https://github.com/LDNOOBW/List-of-Dirty-Naughty-Obscene-and-Otherwise-Bad-Words).
Each list's header names its sources and changes.
