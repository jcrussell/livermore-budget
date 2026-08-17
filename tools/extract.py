#!/usr/bin/env python3
"""Extract page text and word geometry from the source PDFs with poppler.

Run via `make extract`. This is the ONLY Python in the project, and Go never
shells out to it: fisc reads the committed artifacts under data/extracted/ and
needs neither this script nor the PDFs.

Deliberately does not read data/sources.yaml. It discovers work from
data/pdf/<doc-id>.pdf and records the sha256 it computed into the manifest;
`fisc verify` then cross-checks that against the registry. Two parties
recording the hash independently is a real check -- both reading the same file
would not be. It also keeps this script free of a YAML dependency, which
matters more than it sounds: the standard library has no YAML parser and PyPI
is not reachable from the extraction environment.

Output per document:

    data/extracted/<doc-id>/
        manifest.json          what was extracted, with what, and its hashes
        pages/pNNNN.txt        page text, from `pdftotext -layout`
        geometry/pNNNN.json    per-word text and bboxes, from `pdftotext -bbox`

Two substrates, because neither is sufficient alone. `-layout` reproduces the
printed column grid using runs of spaces; that is what a human reads and what
the mapping rules anchor on. But a sparse row emits only the tokens that were
printed, and nothing says which column each one belongs to, so a positional
read can file a figure under the wrong year -- CIP p40 row PB202617 prints one
figure, `500,000`, and it belongs to FY 2026-27; taking tokens left to right
makes it FY 2024-25. That is precisely the "plausible wrong value" this project
fails closed on. `-bbox` carries the x-position that settles it: the token sits
at x 391.1, under the FY 2026-27 header at 369.9-418.9. It is also what keeps
the p67 ten-revenue-rows claim reproducible (fisc-c00). See fisc-yqv.1.

What geometry does NOT do, said plainly because the opposite is easy to
assume: it gives column identity for tokens that are PRESENT. It does not
recover a value the PDF never put in its text layer. On that same CIP p40, rows
PB200654 and PB202617 print `-` in their intervening FY columns and those
dashes are in neither substrate -- not in `-layout`, `-raw`, default mode, nor
in the word boxes -- because they are drawn as non-text. Row PB200429 on the
same page does carry its dashes, so this is per-row, not a flag we chose
wrong. "Absent is not zero" is decided downstream and cannot be decided from
these artifacts alone.

The page text is `.txt`, not `.md`, on purpose: GitHub renders `.md` in its
blob view and collapses the runs of spaces that ARE the column grid, which
would silently break the site's provenance deep links.

Deliberately NOT done here:

  * No amounts are parsed, cleaned, or interpreted. This script moves text.
  * No table detection. The previous extractor (xberg) emitted a tables/*.json
    grid that silently dropped data rows on exactly the pages this project
    needs -- Budget Book p167 prints a ~40-line department-by-object grid and
    seven lines survived; p130 prints 13 Miscellaneous Revenue rows and none
    did. `-layout` plus `-bbox` recovers all of it, no rule ever used the table
    substrate, and one honest substrate beats two that disagree.
  * No OCR and no network. poppler's native text path only.

Everything is byte-stable across runs on the same input and poppler version,
which is what makes committing the artifacts worthwhile.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import pathlib
import re
import shutil
import subprocess
import sys
import xml.etree.ElementTree as ET

# Bump when this script's output contract changes in a way that alters bytes.
# Recorded in the manifest so a diff is attributable.
#
# 2: content_filter.strip_repeating_text=False (xberg). See fisc-c00.
# 3: poppler replaces xberg. pages/pNNNN.md becomes pages/pNNNN.txt from
#    `pdftotext -layout`, and the tables/ substrate is replaced by
#    geometry/pNNNN.json word boxes from `pdftotext -bbox`. xberg lost data
#    rows on the pages the project is built on (Budget Book p167, p130) and
#    its table grid could not express column position at all. See fisc-yqv.
EXTRACTOR_VERSION = 3

PDFTOTEXT = "pdftotext"
PDFINFO = "pdfinfo"

# `pdftotext -bbox` emits XHTML in the XHTML namespace.
XHTML = "{http://www.w3.org/1999/xhtml}"

REPO = pathlib.Path(__file__).resolve().parents[1]
PDF_DIR = REPO / "data" / "pdf"
OUT_DIR = REPO / "data" / "extracted"


# --------------------------------------------------------------------------
# determinism helpers
# --------------------------------------------------------------------------

def sha256_bytes(b: bytes) -> str:
    return hashlib.sha256(b).hexdigest()


def sha256_file(p: pathlib.Path) -> str:
    h = hashlib.sha256()
    with p.open("rb") as f:
        for chunk in iter(lambda: f.read(1 << 20), b""):
            h.update(chunk)
    return h.hexdigest()


def canonical_json(obj) -> bytes:
    """Serialize deterministically: sorted keys, fixed separators, LF, and a
    trailing newline so the file is a well-formed text file."""
    return (json.dumps(obj, sort_keys=True, indent=1, ensure_ascii=False) + "\n").encode()


def geometry_json(payload: dict) -> bytes:
    """canonical_json, except the words array gets one compact row per line.

    Same guarantees -- sorted keys, LF, trailing newline, no reliance on
    iteration order -- but a word is `[x0,y0,x1,y1,"text"]` on a single line
    instead of ten lines with each bbox float on its own. There are 208,221
    words in this corpus; indent=1 spent 18.7 MiB on them for information that
    fits in 7.6, and one line per word is also what makes the file greppable
    and gives a diff one line per word that moved.
    """
    parts = []
    for key in sorted(payload):
        value = payload[key]
        if key == "words":
            if value:
                rows = ",\n".join(
                    "  " + json.dumps(w, ensure_ascii=False, separators=(",", ":"))
                    for w in value
                )
                body = f"[\n{rows}\n ]"
            else:
                body = "[]"
        else:
            body = json.dumps(value, sort_keys=True, ensure_ascii=False)
        parts.append(f" {json.dumps(key)}: {body}")
    return ("{\n" + ",\n".join(parts) + "\n}\n").encode()


def write_atomic(path: pathlib.Path, data: bytes) -> None:
    """Write via a temp file in the SAME directory, then rename.

    Same directory matters: rename is only atomic within a filesystem.
    """
    path.parent.mkdir(parents=True, exist_ok=True)
    tmp = path.with_name(path.name + ".tmp")
    with tmp.open("wb") as f:
        f.write(data)
        f.flush()
        os.fsync(f.fileno())
    tmp.replace(path)


def round_bbox(x_min, y_min, x_max, y_max) -> list[float]:
    """Round a bbox to 2dp.

    poppler prints coordinates with six decimals of a value that is itself a
    font-metric computation. The extra digits are not information, and they are
    the sort of thing that changes across builds; rounding keeps a diff of the
    committed artifacts readable.
    """
    return [round(float(v), 2) for v in (x_min, y_min, x_max, y_max)]


# --------------------------------------------------------------------------
# poppler
#
# poppler has NO structured error channel. pdftotext writes free-form English
# to stderr -- "Syntax Warning: Bad annotation destination", "Syntax Error:
# Suspects object is wrong type (boolean)", "no word list" -- and exits 0 in
# every one of those cases. It cannot tell us which page a document-level
# complaint refers to, whether the complaint cost us any content, or even
# reliably whether "Error" is worse than "Warning": the same prefix covers a
# damaged xref and an unread metadata key.
#
# So we record what we can actually stand behind:
#
#   errors   -- a non-zero exit, or output we could not parse. These are hard
#               failures; the script exits non-zero and the affected page has
#               no artifact rather than a plausible-looking wrong one.
#   warnings -- every stderr line poppler produced, attributed to the stage and
#               page whose invocation produced it, deduplicated with a count.
#
# The consequence is worth stating plainly, because it is a real regression
# from the xberg manifest: a run whose `errors` is empty is NOT a promise that
# every page came out whole. It only says poppler never gave up. Reading
# `warnings` is the human's job.
# --------------------------------------------------------------------------

class PopplerError(RuntimeError):
    """A poppler invocation failed in a way we can actually detect."""


def poppler_version() -> str:
    """Parse the version from `pdftotext -v`, which writes it to stderr."""
    try:
        proc = subprocess.run([PDFTOTEXT, "-v"], capture_output=True)
    except OSError as e:
        raise SystemExit(f"cannot run {PDFTOTEXT}: {e}\nhint: install poppler-utils")
    banner = (proc.stderr + proc.stdout).decode("utf-8", "replace")
    m = re.search(r"pdftotext version (\S+)", banner)
    if not m:
        raise SystemExit(f"cannot parse {PDFTOTEXT} version from: {banner.strip()!r}")
    return m.group(1)


def run_poppler(argv: list[str], stage: str, page: int | None,
                stderr_log: list[tuple[str, int | None, str]]) -> bytes:
    """Run a poppler tool, log its stderr, and return stdout.

    Raises PopplerError on a non-zero exit. stderr is logged either way: the
    stderr of a failed run is the only description of the failure poppler has.
    """
    proc = subprocess.run(argv, capture_output=True)
    for line in proc.stderr.decode("utf-8", "replace").splitlines():
        line = line.strip()
        if line:
            stderr_log.append((stage, page, line))
    if proc.returncode != 0:
        raise PopplerError(f"{argv[0]} exited {proc.returncode}")
    return proc.stdout


def summarize_stderr(log: list[tuple[str, int | None, str]]) -> list[dict]:
    """Fold the per-invocation stderr into one entry per distinct message.

    Document-level complaints repeat on every page -- the CIP's Suspects
    warning would otherwise be 323 identical manifest entries -- so the pages
    are collected instead. `count` is the number of times poppler said it,
    which can exceed len(pages) when one invocation says it twice.
    """
    agg: dict[tuple[str, str], list[int | None]] = {}
    for stage, page, message in log:
        agg.setdefault((stage, message), []).append(page)
    out = []
    for (stage, message), pages in sorted(agg.items()):
        entry = {"stage": stage, "message": message, "count": len(pages)}
        numbered = sorted({p for p in pages if p is not None})
        if numbered:
            entry["pages"] = numbered
        out.append(entry)
    return out


def page_count(pdf: pathlib.Path, stderr_log) -> int:
    info = run_poppler([PDFINFO, str(pdf)], "pdfinfo", None, stderr_log)
    m = re.search(r"^Pages:\s+(\d+)$", info.decode("utf-8", "replace"), re.M)
    if not m:
        raise PopplerError(f"{PDFINFO} reported no page count for {pdf.name}")
    return int(m.group(1))


def page_text(pdf: pathlib.Path, page: int, stderr_log) -> bytes:
    """One page of `pdftotext -layout`, as bytes.

    Kept as bytes deliberately: this script moves text, and decoding it here
    would be the first step toward cleaning it up.
    """
    raw = run_poppler(
        [PDFTOTEXT, "-f", str(page), "-l", str(page), "-layout", str(pdf), "-"],
        "layout", page, stderr_log,
    )
    # pdftotext terminates each page with a form feed. In a one-page-per-file
    # layout that separator separates nothing, and it would leave the file not
    # ending in a newline.
    raw = raw.rstrip(b"\f")
    if raw and not raw.endswith(b"\n"):
        raw += b"\n"
    return raw or b"\n"


def page_geometry(doc_id: str, pdf: pathlib.Path, page: int, stderr_log) -> dict:
    """One page of `pdftotext -bbox`, parsed into word boxes."""
    raw = run_poppler(
        [PDFTOTEXT, "-f", str(page), "-l", str(page), "-bbox", str(pdf), "-"],
        "bbox", page, stderr_log,
    )
    try:
        root = ET.fromstring(raw)
    except ET.ParseError as e:
        raise PopplerError(f"cannot parse -bbox XHTML for page {page}: {e}")

    pages = root.findall(f".//{XHTML}page")
    if len(pages) != 1:
        raise PopplerError(f"-bbox emitted {len(pages)} page elements for page {page}, want 1")
    el = pages[0]

    words = []
    for w in el.iter(f"{XHTML}word"):
        attrs = (w.get("xMin"), w.get("yMin"), w.get("xMax"), w.get("yMax"))
        if any(a is None for a in attrs):
            raise PopplerError(f"-bbox word without a full bbox on page {page}")
        # [x0, y0, x1, y1, text]. A flat row, not an object: at 208k words the
        # key names would outweigh the values, and the shape is fixed anyway.
        words.append(round_bbox(*attrs) + ["".join(w.itertext())])

    # Sorted by position, top-to-bottom then left-to-right, rather than left in
    # poppler's emission order. That order is a reading-order heuristic which
    # reorders columns on its own judgement and is not documented as stable;
    # -layout already carries reading order, and what geometry/ exists to
    # answer is "what is at this y, and in what x-order" -- which is this sort.
    # The trailing key components only break ties, for a total order.
    words.sort(key=lambda w: (w[1], w[0], w[3], w[2], w[4]))

    # Page size in the same units and rounding as the word boxes, so a caller
    # can compare the two without knowing how either was produced.
    size = round_bbox(0, 0, el.get("width") or 0, el.get("height") or 0)
    return {
        "schema_version": 1,
        "doc_id": doc_id,
        "page": page,
        "width": size[2],
        "height": size[3],
        "words": words,
    }


# --------------------------------------------------------------------------
# extraction
# --------------------------------------------------------------------------

def clean_output(out: pathlib.Path) -> None:
    """Remove previously emitted artifacts before writing new ones.

    Without this a re-extraction that yields fewer pages leaves the previous
    run's files on disk and in git. They would be absent from the manifest, but
    any consumer that globs the directory would read stale data as current.
    Also clears .tmp files an interrupted write_atomic left behind.
    """
    for sub in ("pages", "geometry"):
        d = out / sub
        if d.is_dir():
            for f in d.iterdir():
                if f.is_file():
                    f.unlink()
    # tables/ was xberg's; extractor_version 3 does not emit it. Removed whole
    # rather than emptied so a stale directory does not outlive the substrate.
    tables = out / "tables"
    if tables.is_dir():
        shutil.rmtree(tables)
    for f in out.glob("*.tmp"):
        f.unlink()


def extract_doc(doc_id: str, pdf: pathlib.Path, out_root: pathlib.Path, version: str) -> dict:
    stderr_log: list[tuple[str, int | None, str]] = []
    errors: list[dict] = []
    n_pages = page_count(pdf, stderr_log)

    out = out_root / doc_id
    clean_output(out)
    artifacts: dict[str, dict] = {}

    def emit(rel: str, data: bytes) -> None:
        write_atomic(out / rel, data)
        artifacts[rel] = {"sha256": sha256_bytes(data), "bytes": len(data)}

    blank = 0
    for page in range(1, n_pages + 1):
        try:
            text = page_text(pdf, page, stderr_log)
        except PopplerError as e:
            errors.append({"stage": "layout", "page": page, "message": str(e)})
        else:
            if not text.strip():
                blank += 1
            emit(f"pages/p{page:04d}.txt", text)

        try:
            geom = page_geometry(doc_id, pdf, page, stderr_log)
        except PopplerError as e:
            errors.append({"stage": "bbox", "page": page, "message": str(e)})
        else:
            emit(f"geometry/p{page:04d}.json", geometry_json(geom))

    # Capped: a document-wide failure produces one error per page, and 600
    # identical lines bury whatever else the run said. All of them are in the
    # manifest, which is the copy that matters.
    for e in errors[:10]:
        print(f"  error: p{e['page']} {e['stage']}: {e['message']}", file=sys.stderr)
    if len(errors) > 10:
        print(f"  ... and {len(errors) - 10} more errors, all in manifest.json", file=sys.stderr)

    manifest = {
        # 2: the poppler contract. Page artifacts moved to pages/pNNNN.txt, the
        # tables/ namespace became geometry/pNNNN.json, six keys were dropped
        # and poppler_version added, and warnings/errors changed element shape.
        # A reader pinned to 1 would otherwise parse this file happily and then
        # report every page as "no such page", which is a documented NORMAL
        # result -- so the version has to be what fails, loudly.
        "schema_version": 2,
        "doc_id": doc_id,
        "source_file": str(pdf.relative_to(REPO)),
        "source_sha256": sha256_file(pdf),
        "source_bytes": pdf.stat().st_size,
        "poppler_version": version,
        "extractor_version": EXTRACTOR_VERSION,
        "page_count": n_pages,
        "blank_page_count": blank,
        # Every stderr line poppler produced. Not decoration: it is the only
        # account poppler gives of what it could not read, and it does not
        # affect the exit code because poppler itself does not think these are
        # fatal. See the note above run_poppler.
        "warnings": summarize_stderr(stderr_log),
        # Recorded, not just logged. A run that failed on some pages must be
        # visible to fisc verify rather than looking clean.
        "errors": errors,
        "artifacts": dict(sorted(artifacts.items())),
    }
    write_atomic(out / "manifest.json", canonical_json(manifest))
    return manifest


def discover() -> dict[str, pathlib.Path]:
    return {p.stem: p for p in sorted(PDF_DIR.glob("*.pdf"))}


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    ap.add_argument("--doc", action="append", metavar="ID",
                    help="document id to extract (repeatable); default is all")
    ap.add_argument("--out", type=pathlib.Path, default=OUT_DIR,
                    help="output root (default: data/extracted)")
    ap.add_argument("--list", action="store_true", help="list discoverable documents and exit")
    args = ap.parse_args()

    available = discover()
    if not available:
        print(f"no PDFs found in {PDF_DIR}", file=sys.stderr)
        print("hint: git lfs pull", file=sys.stderr)
        return 1

    if args.list:
        for doc_id, p in available.items():
            print(f"{doc_id}\t{p.stat().st_size:>10,} bytes")
        return 0

    selected = args.doc or list(available)
    if unknown := [d for d in selected if d not in available]:
        print(f"unknown document id(s): {', '.join(unknown)}", file=sys.stderr)
        print(f"hint: available ids are {', '.join(available)}", file=sys.stderr)
        return 2

    version = poppler_version()
    print(f"poppler {version}", file=sys.stderr)

    failed = []
    for doc_id in selected:
        print(f"extracting {doc_id} ...", file=sys.stderr)
        try:
            m = extract_doc(doc_id, available[doc_id], args.out, version)
        except PopplerError as e:
            print(f"  failed: {e}", file=sys.stderr)
            failed.append(doc_id)
            continue
        print(
            f"  {m['page_count']} pages ({m['blank_page_count']} blank), "
            f"{len(m['artifacts'])} artifacts, "
            f"{len(m['warnings'])} distinct poppler warnings",
            file=sys.stderr,
        )
        if m["errors"]:
            failed.append(doc_id)

    if failed:
        # Artifacts and manifests are still written -- they record what went
        # wrong -- but the exit code must not say everything is fine.
        print(f"\nextraction reported errors for: {', '.join(failed)}", file=sys.stderr)
        print("hint: the errors are recorded in each manifest.json", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
