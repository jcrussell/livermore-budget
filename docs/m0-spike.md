# M0 spike findings

Throwaway validation run before any schema was frozen. The question it had to
answer: **can we get labelled, checkable budget figures out of these PDFs, and
from which substrate?**

Answer: **yes, from page text.** The reference implementation in
[`m0-spike/spine.py`](m0-spike/spine.py) maps 136 facts from Budget Book p66–67
and all 16 column totals tie *exactly* to the totals the document itself prints.

## Verified numbers

| Figure | Value |
|---|---|
| General Fund FY2025-26 revenues | $157,873,470 |
| General Fund FY2025-26 expenditures | $144,650,802 |
| All-funds FY2025-26 revenues | $299,969,007 |

Every fund-group column (General, Enterprise, Capital, Debt Service, Special
Revenue, Internal Service) reconciles against its printed `TOTAL REVENUES:` /
`TOTAL EXPENDITURES:` line for both budget years.

## Findings that shaped the design

**1. Page text is the primary substrate, not tables.** Table extraction reaches
only 46% of the Budget Book's money-bearing pages (62% ACFR, 52% CIP), and the
misses are concentrated on the pages that matter most: the citywide spine
(p66–67), the transfer schedule (p76), and revenue-by-fund (p127–140) all yield
*zero* table objects. Page text carries the same content and is complete.

**2. The spine's grammar is "label followed by N numbers."** p66 carries the row
labels plus the General Fund and Enterprise columns — 4 values per row, being
2 fund groups × 2 fiscal years.

**3. p67 has no row labels at all.** It is a positional continuation of p66 for
the other four fund groups (8 values per row). Row identity comes from p66's
order, which means a logical table can span pages with the label page as the
anchor.

**4. The row-count assertion caught an extractor bug — not a document quirk.**
The spike read 72 values on p67 where 10 rows × 8 columns requires 80, and
failed loudly:

```
p67 row-count mismatch: 72 values is not 10 rows x 8 columns.
Declared omissions: []
```

This was originally written up as "p67 omits rows that are all-zero across its
four fund groups," and `Licenses & Permits` was declared in `omitted_rows` to
make the count work. **That reading was wrong.** `pdftotext -bbox` puts ten
revenue rows on p67, at the same ten y-positions as p66's ten labelled rows —
the document omits nothing. Rows 8–10 (`Sales Taxes`, `Fines & Forfeitures`,
`Licenses & Permits`) are three consecutive all-dash lines, and xberg's
`content_filter.strip_repeating_text` — a header/footer dedup heuristic that
defaults to on — deleted the third as boilerplate. It is disabled in
`tools/extract.py` as of `EXTRACTOR_VERSION = 2`; see bead `fisc-c00`.

The lesson is stronger than the original one. A positional read of a label-less
continuation page identifies rows by order, so a row silently removed *by our
own pipeline* mismaps every row beneath it. The count assertion is the only
thing standing between that and 16 published facts carrying the wrong
`row_label` — which is part of the fact id. Fail-closed paid for itself here.

**5. `\-` is a seventh amount-corruption mode.** The extracted markdown escapes
some dashes, so a backslash-dash appears as a distinct zero token alongside the
six modes already catalogued (whitespace-split digits, em-dash-as-zero glued to
the next column, `$` bleeding one cell right, parenthesis negatives, `-` vs `''`
meaning zero vs absent, and varying units).

## Cost

The whole corpus — 786 pages across three PDFs — extracts in **10 seconds**
with `disable_ocr=True, layout=None`, fully offline, no model downloads.

## Retrieving the source PDFs

`www.livermoreca.gov` is behind Akamai bot protection and returns **HTTP 403 to
a bare `curl`** (an `AkamaiGHost` error page, not a network failure). A full
browser header set gets through:

```bash
curl -sSL --compressed \
  -A "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36" \
  -H "Accept: text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8" \
  -H "Accept-Language: en-US,en;q=0.9" \
  -H "Sec-Fetch-Dest: document" -H "Sec-Fetch-Mode: navigate" \
  -H "Sec-Fetch-Site: none" -H "Sec-Fetch-User: ?1" \
  -H "Upgrade-Insecure-Requests: 1" \
  "https://www.livermoreca.gov/home/showpublisheddocument/12813" -o budget.pdf
```

Document permalinks take the form `/home/showpublisheddocument/<id>`. The
longer form with a trailing .NET-ticks value is a cache-buster; both serve the
same bytes, and `data/sources.yaml` records each.

All three committed PDFs were re-downloaded and verified **byte-for-byte
identical** to the city's published files on 2026-08-15. Because Git LFS uses
sha256 as its object ID, the LFS OIDs double as a second copy of that check.

## Status

The reference implementation is **not production code**. It exists to prove the
approach and to give the real implementation (`internal/mapping`, bead
`fisc-mq4.7`) a known-good target to reproduce.
