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

**4. p67 omits rows that are all-zero across its four fund groups.** The
`Licenses & Permits` row simply is not emitted. A naive positional zip would
silently shift every label after the gap — precisely the silent-mismapping
failure the whole provenance design exists to catch. Mapping rules therefore
need an explicit `omitted_rows` declaration plus a row-count assertion that
fails loudly:

```
p67 row-count mismatch: 72 values is not 10 rows x 8 columns.
Declared omissions: []
```

The column-total check caught this immediately, which is direct evidence for
the tier-2 reconciliation design.

**5. `\-` is a seventh amount-corruption mode.** The extracted markdown escapes
some dashes, so a backslash-dash appears as a distinct zero token alongside the
six modes already catalogued (whitespace-split digits, em-dash-as-zero glued to
the next column, `$` bleeding one cell right, parenthesis negatives, `-` vs `''`
meaning zero vs absent, and varying units).

## Cost

The whole corpus — 786 pages across three PDFs — extracts in **10 seconds**
with `disable_ocr=True, layout=None`, fully offline, no model downloads.

## Status

The reference implementation is **not production code**. It exists to prove the
approach and to give the real implementation (`internal/mapping`, bead
`fisc-mq4.7`) a known-good target to reproduce.
