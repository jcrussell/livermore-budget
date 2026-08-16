# Test fixtures

Real artifacts copied verbatim from `data/extracted/`, so Go tests never need
Python, the source PDFs, or the network. Regenerate with `make extract` and
re-copy if the extractor's output contract changes.

Each fixture is here because it encodes a specific failure mode found while
validating the approach — none is a generic sample.

## Pages

| Fixture | Source | Why |
|---|---|---|
| `pages/budget-p0066.md` | Budget Book p66 | The citywide spine. Every control total ties to this page. Rows are "label followed by 4 numbers" (2 fund groups × 2 fiscal years), and it is the **label anchor** for p67. |
| `pages/budget-p0067.md` | Budget Book p67 | The spine's continuation, and the nastiest case in the corpus: **no row labels at all** — identity is positional from p66, so a single missing row mismaps every row beneath it. This fixture is also the evidence for `fisc-c00`: it used to carry nine revenue rows because xberg's `strip_repeating_text` deleted the third of three identical all-dash rows, and the count assertion (72 ≠ 80) is what caught it. Any mapping engine must fail loudly here, not guess. |
| `pages/budget-p0127.md` | Budget Book p127 | General Fund revenue by source, and the **contra-revenue** case: ERAF and RPTTF are negative rows (~26% of gross property tax) that a Sankey cannot render as links. Also shows the 4-column shape (FY23-24 actual, FY24-25 revised, FY25-26, FY26-27) that makes every revenue line a trend series. |

## Tables

| Fixture | Source | Why |
|---|---|---|
| `tables/acfr-p0034-t01.json` | ACFR p34 | Extraction **silently dropped a column** — 5 columns for a 6-column schedule — and the row labels are gone entirely, so every row is numeric-only. The content hash cannot detect this, because it only covers what was extracted. This is the fixture for the dropped-column check. |
| `tables/acfr-p0012-t01.json`, `tables/acfr-p0013-t01.json` | ACFR pp. 12–13 | Two members of a **12-way byte-identical group** (the repeated letter-of-transmittal header). Proof that a content hash is not an identity, and the fixture for locator ambiguity: resolution must report the candidates, not silently take the first. |
| `tables/budget-p0131-t01.json` | Budget Book p131 | One member of a **20-way identical group** — the same problem, larger, in the primary document. |

## Note on numeric-only rows

`acfr-p0034-t01.json` has `row_page_lines` all null, because `find_page_line`
locates a row by its non-numeric cells and these rows have none. That is
expected and is itself the signal: a table whose rows cannot be located in the
page text has lost its label column. Across the ACFR, **75% of rows and 86% of
tables** locate successfully.

Those figures were 81% and 98% before a fix, and the drop is an improvement.
The classifier that decides whether a cell is a label previously ignored
whitespace, so merged numeric cells like `"$ 483.9"` and
`"$ 7,783,173 $ 20,710,479"` — 1,366 of 14,624 corpus cells — were treated as
label words. Rows then "located" by matching a number against the page text,
which is a false positive, and `label_fingerprint` embedded those numbers,
destroying its purpose as an identity that survives a new fiscal year. The
lower number is the honest one.
