# Test fixtures

Real artifacts copied verbatim from `data/extracted/`, so Go tests never need
Python, the source PDFs, or the network. Regenerate with `make extract` and
re-copy if the extractor's output contract changes.

"Copied verbatim" is the property everything here rests on, and it is checkable:
each page fixture's sha256 equals the one `data/extracted/<doc>/manifest.json`
records for the artifact it came from. A test that passes against these
fixtures therefore passes against the real corpus. Do not hand-edit one — a
fixture that has drifted from the extraction is a test that proves nothing about
what `fisc build` will read.

Page fixtures are `.txt`, not `.md`, for the same reason `data/extracted/`
switched: `pdftotext -layout` encodes the printed column grid in runs of
spaces, and GitHub's blob view renders markdown and collapses them.

Each fixture is here because it encodes a specific failure mode found while
validating the approach — none is a generic sample.

## Pages

| Fixture | Source | Why |
|---|---|---|
| `pages/budget-p0066.txt` | Budget Book p66 | The citywide spine. Every control total ties to this page. Rows are "label followed by 4 numbers" (2 fund groups × 2 fiscal years), and it is the **label anchor** for p67. It also carries the running footer `BUDGET FY 2025-27 … Page 62`, whose bare page number parses as an amount — which is why every block on this page states a `stop_at`. |
| `pages/budget-p0067.txt` | Budget Book p67 | The spine's continuation, and the nastiest case in the corpus: **no row labels at all** — identity is positional from p66, so a single missing row mismaps every row beneath it. This fixture is also the evidence for `fisc-c00`: it used to carry nine revenue rows because xberg's `strip_repeating_text` deleted the third of three identical all-dash rows, and the count assertion (72 ≠ 80) is what caught it. Any mapping engine must fail loudly here, not guess. |
| `pages/budget-p0127.txt` | Budget Book p127 | General Fund revenue by source, and the **contra-revenue** case: ERAF and RPTTF are negative rows (~26% of gross property tax) that a Sankey cannot render as links. Also shows the 4-column shape (FY23-24 actual, FY24-25 revised, FY25-26, FY26-27) that makes every revenue line a trend series. |

## Projections

| Fixture | Source | Why |
|---|---|---|
| `sankey.golden.json` | Budget Book pp. 66–67, FY2026 | The frozen `sankey.json` contract as a worked example — real figures, real fact ids, all four `link.kind` values, and both `derived: true` nodes. Hand-derived in Wave 0 so `internal/export` and `site/app.js` could be built before `internal/project` existed. Once the Go projection lands it must reproduce this byte-for-byte; until then it is the only thing pinning the contract. See [`docs/sankey-contract.md`](../docs/sankey-contract.md). |
