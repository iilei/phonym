"""Write the fixed-width snapshot tables (or verify them).

  family_snapshot.txt  every family name: slot, low bits, spread index
  names_snapshot.txt   full names (given + family) for fixed sample IDs per release

usage: python3 snapshot.py           write both files into ../../testdata
       python3 snapshot.py --check   compare against the files in ../../testdata
"""
import hashlib
import os
import random
import re
import sys

import family_demo as fam
import styles_demo as st

PROQUINT = re.compile(r"^[bdfghjklmnprstvz][aiou][bdfghjklmnprstvz][aiou][bdfghjklmnprstvz]$")


def is_proquint(name: str) -> bool:
    return all(PROQUINT.match(word) for word in name.split("-"))

# last column unpadded everywhere: no trailing whitespace (the repo's pre-commit hook strips it)


def _table(title: str, params: str, columns: str, rule: str, body: list[str]) -> str:
    digest = hashlib.sha256("\n".join(body).encode()).hexdigest()[:16]
    header = [f"# {title}", f"# {params} sha256={digest}", "#", f"# {columns}", f"# {rule}"]
    return "\n".join(header + body) + "\n"


# --- family code ------------------------------------------------------------

def render_family() -> str:
    body = []
    for slot in range(fam.VERSIONS):
        for lo in range(fam.K):
            name = fam.family_name(slot, lo)
            assert fam.FAMILY_SHAPE.match(name) and not PROQUINT.match(name), name
            index = fam.FAMILY.rank(fam.FAMILY.parse(name))
            body.append(f"{slot:>4}  {lo:>3}  {index:>5}  {name}")
    params = (f"length={fam.FAMILY_LEN} K={fam.K} slots={fam.VERSIONS} capacity={fam.CAP}"
              f" stride={fam.STRIDE} offset={fam.OFFSET} blocklist1={blocklist_digest()}")
    return _table("phonym family code snapshot", params,
                  "slot   lo  index  name", "----  ---  -----  --------", body)


# --- full names -------------------------------------------------------------

def sample_ids() -> list[int]:
    """Fixed sample: boundary values (up to 2^256 - 1) plus seeded random IDs of hex length 1..16."""
    edges = {0, 1, fam.K - 1, fam.K, fam.K + 1,
             0xFFFFFF, 0x1000000, 0xFFFFFFF, 0x10000000,
             2**32 - 1, 2**40 - 1, 2**48 - 1, 2**56 - 1, 2**64 - 1, 2**64,
             2**128 - 1, 2**256 - 1}
    rng = random.Random(20261109)                 # frozen seed: the sample never changes
    for digits in range(1, 17):
        lo, hi = 16 ** (digits - 1), 16 ** digits
        edges.update(rng.randrange(lo, hi) for _ in range(2))
    return sorted(edges)


def render_names() -> str:
    rows = []
    for index, release in enumerate(st.REGISTRY):
        if release.style is None:                 # placeholder: never issued names
            continue
        for x in sample_ids():
            try:
                full = st.encode(x, release.label, allow_retired=True)
            except OverflowError:
                full = "<overflow>"
            else:
                assert st.decode(full) == (release.label, x), (release.label, x, full)
                assert not is_proquint(full), full
            try:
                short = st.encode_short(x, release.label, allow_retired=True)
            except OverflowError:
                short = "<overflow>"
            else:
                assert st.decode(short, release.label) == (release.label, x), (release.label, x, short)
            rows.append((index, release.label, x, short, full))
    width = max(24, max(len(r[3]) for r in rows))          # short column padded, full last
    body = [f"{i:>4}  {label:<14}  {x:064x}  {short:<{width}}  {full}"
            for i, label, x, short, full in rows]
    retired = [i for i, r in enumerate(st.REGISTRY) if r.retired and r.style is not None]
    holes = [i for i, r in enumerate(st.REGISTRY) if r.style is None]
    fmt = lambda xs: ",".join(map(str, xs)) or "none"
    params = (f"releases={len(st.REGISTRY)} retired={fmt(retired)} placeholders={fmt(holes)}"
              f" ids={len(sample_ids())} K={fam.K} seed=20261109 blocklist1={blocklist_digest()}")
    return _table("phonym name snapshot (short and full form)", params,
                  f"slot  release         id                                                                {'short':<{width}}  full",
                  "----  --------------  ----------------------------------------------------------------  " + "-" * width + "  " + "-" * 24, body)


BLOCKLIST1 = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "..", "blocklists", "1.txt")


def blocklist_digest() -> str:
    with open(BLOCKLIST1, "rb") as f:
        return hashlib.sha256(f.read()).hexdigest()[:16]


TESTDATA = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "..", "testdata")
FILES = {os.path.join(TESTDATA, "family_snapshot.txt"): render_family,
         os.path.join(TESTDATA, "names_snapshot.txt"): render_names}

if __name__ == "__main__":
    failed = False
    for path, render in FILES.items():
        text = render()
        if "--check" in sys.argv:
            try:
                with open(path) as f:
                    old = f.read()
            except FileNotFoundError:
                old = None
            ok = old == text
            failed |= not ok
            print(f"{path} {'matches' if ok else 'DIFFERS'}")
        else:
            with open(path, "w") as f:
                f.write(text)
            print(f"wrote {path} ({text.count(chr(10))} lines)")
    sys.exit(1 if failed else 0)
