"""Write the fixed-width snapshot tables (or verify them).

  family.txt                 the frozen family code: one line per release slot, its 64 names
  names/<slot>-<release>.txt short and full form of fixed sample IDs, one file per release

A released file never changes (the registry is append-only): a diff in an existing
names file means a release was renamed, which is a breaking change.

usage: python3 snapshot.py           write the files into ../../testdata
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


HERE = os.path.dirname(os.path.abspath(__file__))
TESTDATA = os.path.join(HERE, "..", "..", "testdata")
BLOCKLIST1 = os.path.join(HERE, "..", "..", "blocklists", "1.txt")


def blocklist_digest() -> str:
    with open(BLOCKLIST1, "rb") as f:
        return hashlib.sha256(f.read()).hexdigest()[:16]


# --- family code ------------------------------------------------------------

def render_family() -> str:
    body = []
    for slot in range(fam.VERSIONS):
        names = [fam.family_name(slot, lo) for lo in range(fam.K)]
        for name in names:
            assert fam.FAMILY_SHAPE.match(name) and not PROQUINT.match(name), name
        body.append(f"{slot:>3}  " + " ".join(names))
    digest = hashlib.sha256("\n".join(body).encode()).hexdigest()[:16]
    header = [
        "# phonym family code",
        f"# length={fam.FAMILY_LEN} K={fam.K} slots={fam.VERSIONS} capacity={fam.CAP}"
        f" stride={fam.STRIDE} offset={fam.OFFSET} blocklist1={blocklist_digest()} sha256={digest}",
        "# one line per release slot: its 64 family names, sibling 0 (x mod K = 0) first",
    ]
    return "\n".join(header + body) + "\n"


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


def render_names(index: int, release) -> str:
    rows = []
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
        rows.append((x, short, full))
    digest = hashlib.sha256("\n".join(f"{x:x} {sh} {full}" for x, sh, full in rows).encode()).hexdigest()[:16]
    status = " retired" if release.retired else ""
    out = [
        f"# phonym names: {release.label} (slot {index}){status}",
        f"# ids={len(rows)} K={fam.K} seed=20261109 blocklist1={blocklist_digest()} sha256={digest}",
    ]
    # two tables, so the few huge IDs don't set the column widths for all the others
    for title, part in [("IDs up to 64 bits", [r for r in rows if r[0].bit_length() <= 64]),
                        ("IDs beyond 64 bits", [r for r in rows if r[0].bit_length() > 64])]:
        id_w = max(2, max(len(f"{x:x}") for x, _, _ in part))
        short_w = max(5, max(len(sh) for _, sh, _ in part))
        out += ["#", f"# {title}", f"# {'id':>{id_w - 2}}  {'short':<{short_w}}  full"]
        out += [f"{x:>{id_w}x}  {sh:<{short_w}}  {full}" for x, sh, full in part]
    return "\n".join(out) + "\n"


def names_path(index: int, release) -> str:
    return os.path.join("names", f"{index:02d}-{release.label}.txt")


def expected_files() -> dict[str, str]:
    files = {"family.txt": render_family()}
    for index, release in enumerate(st.REGISTRY):
        if release.style is not None:              # placeholders never issued names
            files[names_path(index, release)] = render_names(index, release)
    return files


if __name__ == "__main__":
    files = expected_files()
    present = {os.path.relpath(os.path.join(d, f), TESTDATA)
               for d, _, fs in os.walk(TESTDATA) for f in fs if f.endswith(".txt")}
    if "--check" in sys.argv:
        failed = False
        for rel, text in files.items():
            try:
                with open(os.path.join(TESTDATA, rel), encoding="utf-8") as f:
                    ok = f.read() == text
            except FileNotFoundError:
                ok = False
            failed |= not ok
            print(f"{rel} {'matches' if ok else 'DIFFERS'}")
        for rel in sorted(present - set(files)):
            failed = True
            print(f"{rel} UNEXPECTED (no release writes it)")
        sys.exit(1 if failed else 0)
    for rel in sorted(present - set(files)):
        os.remove(os.path.join(TESTDATA, rel))
        print(f"removed {rel}")
    os.makedirs(os.path.join(TESTDATA, "names"), exist_ok=True)
    for rel, text in files.items():
        with open(os.path.join(TESTDATA, rel), "w", encoding="utf-8") as f:
            f.write(text)
    print(f"wrote {len(files)} files into {os.path.normpath(TESTDATA)}")
