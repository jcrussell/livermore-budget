#!/usr/bin/env python3
"""Extract page text and tables from the source PDFs with xberg.

Run via `make extract`. This is the ONLY Python in the project, and Go never
shells out to it: fisc reads the committed artifacts under data/extracted/ and
needs neither this script nor the PDFs.

Deliberately does not read data/sources.yaml. It discovers work from
data/pdf/<doc-id>.pdf and records the sha256 it computed into the manifest;
`fisc verify` then cross-checks that against the registry. Two parties
recording the hash independently is a real check -- both reading the same file
would not be -- and it keeps this script free of a YAML dependency.

Output per document:

    data/extracted/<doc-id>/
        manifest.json          what was extracted, with what, and its hashes
        pages/pNNNN.md         page text -- the PRIMARY mapping substrate
        tables/pNNNN-tNN.json  detected tables, ordinal within the page

Everything is byte-stable across runs on the same input and xberg version,
which is what makes committing the artifacts worthwhile.
"""

from __future__ import annotations

import argparse
import asyncio
import hashlib
import json
import os
import pathlib
import re
import sys
import unicodedata

import xberg
from xberg.options import ExtractInput

# Bump when this script's output contract changes in a way that alters bytes.
# Recorded in the manifest so a diff is attributable.
EXTRACTOR_VERSION = 1

# Bump when normalize_cell changes. Separate from EXTRACTOR_VERSION so that
# "xberg changed" stays distinguishable from "our normalizer changed" -- the
# two demand different remediation.
NORMALIZER_VERSION = 1

# Native path only: no OCR, no layout models, no network, no HuggingFace
# download. Table extraction is on by default and needs no models.
CONFIG = {
    "use_cache": False,
    "disable_ocr": True,
    "output_format": "markdown",
    "pages": {"extract_pages": True},
    "pdf_options": {
        "extract_tables": True,
        "extract_metadata": True,
    },
}

REPO = pathlib.Path(__file__).resolve().parents[1]
PDF_DIR = REPO / "data" / "pdf"
OUT_DIR = REPO / "data" / "extracted"

# Tokens that carry no label information: digits, separators, currency, and
# the several dash characters that all mean "zero" in these documents.
_NUMERIC = re.compile(r"^[\d,.$()%+‐-―-]*$")


# --------------------------------------------------------------------------
# determinism helpers
# --------------------------------------------------------------------------

def normalize_cell(s: str) -> str:
    """Canonicalize a cell for hashing and comparison.

    Collapses the several ways these PDFs spell the same thing so that a
    cosmetic change upstream does not read as a content change. Deliberately
    preserves case and digits: this feeds integrity hashes, not matching.
    """
    s = unicodedata.normalize("NFKC", s)
    s = s.replace(" ", " ").replace("­", "")       # NBSP, soft hyphen
    s = re.sub(r"[‐-―−]", "-", s)             # dashes, minus
    s = s.replace("\\-", "-")                                # escaped markdown dash
    return re.sub(r"\s+", " ", s).strip()


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


def round_bbox(bb) -> list[float] | None:
    if bb is None:
        return None
    return [round(float(v), 2) for v in (bb.x0, bb.y0, bb.x1, bb.y1)]


# --------------------------------------------------------------------------
# table serialization
# --------------------------------------------------------------------------

def label_fingerprint(cells: list[list[str]]) -> str:
    """Hash only the non-numeric tokens of a table.

    This is the content-INDEPENDENT half of a locator: it survives the numbers
    changing (a new fiscal year) but still distinguishes one schedule from
    another. The integrity hash below is what catches content drift.
    """
    words = [
        normalize_cell(c)
        for row in cells
        for c in row
        if c and not _NUMERIC.match(normalize_cell(c))
    ]
    return "sha256:" + sha256_bytes("␟".join(words).encode())


def content_hash(cells: list[list[str]]) -> str:
    """Hash the full normalized grid. Integrity, not identity."""
    body = "␞".join("␟".join(normalize_cell(c) for c in row) for row in cells)
    return "sha256:" + sha256_bytes(body.encode())


def find_page_line(page_lines: list[str], row: list[str]) -> int | None:
    """Index of the page-text line this row most likely came from.

    Used later by the dropped-column check: if the page text for a row's line
    holds more numbers than the table has columns, extraction lost one. Best
    effort -- returns None when the row has no usable label.
    """
    for cell in row:
        label = normalize_cell(cell)
        if not label or _NUMERIC.match(label):
            continue
        for i, line in enumerate(page_lines):
            if label in normalize_cell(line):
                return i
        return None
    return None


def serialize_table(doc_id: str, page: int, ordinal: int, table, page_lines: list[str]) -> dict:
    cells = [[c for c in row] for row in table.cells]
    return {
        "schema_version": 1,
        "doc_id": doc_id,
        "page": page,
        "ordinal": ordinal,
        "bbox": round_bbox(table.bounding_box),
        "n_rows": len(cells),
        "n_cols": max((len(r) for r in cells), default=0),
        "ragged": len({len(r) for r in cells}) > 1,
        "cells": cells,
        "row_page_lines": [find_page_line(page_lines, r) for r in cells],
        "label_fingerprint": label_fingerprint(cells),
        "content_hash": content_hash(cells),
    }


# --------------------------------------------------------------------------
# extraction
# --------------------------------------------------------------------------

async def run_xberg(path: pathlib.Path):
    res = await xberg.extract(ExtractInput(kind="uri", uri=str(path)), CONFIG)
    for err in res.errors:
        print(f"  error: {err.error_type}: {err.message}", file=sys.stderr)
    if not res.results:
        raise SystemExit(f"extraction produced no document for {path}")
    return res.results[0]


def extract_doc(doc_id: str, pdf: pathlib.Path, out_root: pathlib.Path) -> dict:
    doc = asyncio.run(run_xberg(pdf))
    out = out_root / doc_id
    artifacts: dict[str, dict] = {}

    def emit(rel: str, data: bytes) -> None:
        write_atomic(out / rel, data)
        artifacts[rel] = {"sha256": sha256_bytes(data), "bytes": len(data)}

    # Pages. The primary substrate: complete where tables are not.
    pages = sorted(doc.pages or [], key=lambda p: p.page_number)
    page_lines: dict[int, list[str]] = {}
    blank = 0
    for pg in pages:
        text = pg.content or ""
        page_lines[pg.page_number] = text.splitlines()
        if pg.is_blank:
            blank += 1
        if not text.endswith("\n"):
            text += "\n"
        emit(f"pages/p{pg.page_number:04d}.md", text.encode())

    # Tables, sorted by position rather than by xberg's iteration order --
    # which is NOT document order (ACFR p34 emits one logical table as three
    # fragments in reverse) and is not documented as stable across machines.
    def position(t):
        bb = round_bbox(t.bounding_box) or [0.0, 0.0, 0.0, 0.0]
        return (t.page_number, bb[1], bb[0])

    per_page: dict[int, int] = {}
    for t in sorted(doc.tables, key=position):
        page = t.page_number
        per_page[page] = per_page.get(page, 0) + 1
        ordinal = per_page[page]
        payload = serialize_table(doc_id, page, ordinal, t, page_lines.get(page, []))
        emit(f"tables/p{page:04d}-t{ordinal:02d}.json", canonical_json(payload))

    warnings = [
        {"source": w.source, "message": w.message}
        for w in getattr(doc, "processing_warnings", []) or []
    ]

    manifest = {
        "schema_version": 1,
        "doc_id": doc_id,
        "source_file": str(pdf.relative_to(REPO)),
        "source_sha256": sha256_file(pdf),
        "source_bytes": pdf.stat().st_size,
        "xberg_version": xberg.__version__,
        "extractor_version": EXTRACTOR_VERSION,
        "normalizer_version": NORMALIZER_VERSION,
        "config": CONFIG,
        "config_sha256": sha256_bytes(canonical_json(CONFIG)),
        "page_count": len(pages),
        "blank_page_count": blank,
        "table_count": len(doc.tables),
        "pages_with_tables": len(per_page),
        "warnings": warnings,
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

    for doc_id in selected:
        print(f"extracting {doc_id} ...", file=sys.stderr)
        m = extract_doc(doc_id, available[doc_id], args.out)
        print(
            f"  {m['page_count']} pages ({m['blank_page_count']} blank), "
            f"{m['table_count']} tables on {m['pages_with_tables']} pages, "
            f"{len(m['artifacts'])} artifacts",
            file=sys.stderr,
        )
    return 0


if __name__ == "__main__":
    sys.exit(main())
