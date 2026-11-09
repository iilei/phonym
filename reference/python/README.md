# Python reference prototype

The design of phonym was worked out in these scripts. They are kept as an
independent second implementation: `snapshot.py --check` must reproduce
`testdata/` exactly, just like the Go tests. They are not a library and not the
planned Python port.

| File | Contents |
|---|---|
| `syllable_automaton.py` | ranking over the automaton; the base syllable rules |
| `family_demo.py` | the frozen family code (SHAPE and GENERATION rules) |
| `styles_demo.py` | styles, the release registry, INPUT and JOIN rules, short form; running it prints its checks |
| `snapshot.py` | writes or checks `testdata/family_snapshot.txt` and `testdata/names_snapshot.txt` |

```sh
python3 snapshot.py --check   # compare against ../../testdata
python3 snapshot.py           # regenerate (then `go test ./...` must still pass)
python3 styles_demo.py        # run the prototype's own checks
```

Known differences from the Go implementation, which is the reference:

- The INPUT RULE here queries Unicode properties (`str.isspace`, category `Pd`)
  instead of the explicit table in `input.go`. Both agree on everything tested.
- The base `END` set in `syllable_automaton.py` contains unused entries. No
  release uses it.
