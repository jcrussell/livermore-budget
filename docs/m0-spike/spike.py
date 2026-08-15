"""M0 spike — throwaway. Dump per-page text + tables for the three PDFs.

Not production code. The real extractor is tools/extract.py (E2).
"""
import asyncio
import json
import pathlib
import sys

import xberg
from xberg.options import ExtractInput

CFG = {
    "use_cache": False,
    "disable_ocr": True,
    "output_format": "markdown",
    "pages": {"extract_pages": True},
    "pdf_options": {"extract_tables": True, "extract_metadata": True},
}

REPO = pathlib.Path(__file__).resolve().parents[2]
DATA = REPO / "data" / "pdf"
OUT = pathlib.Path(__file__).parent / "spike"

DOCS = {
    "budget": "livermore-budget-fy2026-2027.pdf",
    "cip": "livermore-cip-fy2026-2030.pdf",
    "acfr": "livermore-acfr-fy2025.pdf",
}


async def extract(path):
    res = await xberg.extract(ExtractInput(kind="uri", uri=str(path)), CFG)
    for err in res.errors:
        print(f"  ERROR {err.error_type}: {err.message}", file=sys.stderr)
    return res.results[0]


def dump(key, doc):
    d = OUT / key
    (d / "pages").mkdir(parents=True, exist_ok=True)
    for pg in doc.pages or []:
        (d / "pages" / f"p{pg.page_number:04d}.md").write_text(pg.content or "")
    tables = []
    for t in doc.tables:
        bb = t.bounding_box
        tables.append({
            "page": t.page_number,
            "bbox": None if bb is None else [bb.x0, bb.y0, bb.x1, bb.y1],
            "n_rows": len(t.cells),
            "n_cols": max((len(r) for r in t.cells), default=0),
            "cells": t.cells,
        })
    (d / "tables.json").write_text(json.dumps(tables, indent=1, sort_keys=True))
    print(f"{key}: {len(doc.pages or [])} pages, {len(doc.tables)} tables -> {d}")


if __name__ == "__main__":
    for key, name in DOCS.items():
        dump(key, asyncio.run(extract(DATA / name)))
