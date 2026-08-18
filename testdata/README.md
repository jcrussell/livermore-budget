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

Every fixture page carries **both** substrates — `pages/<doc>-pNNNN.txt` and
`geometry/<doc>-pNNNN.json`. A document holding one without the other is not a
shape the extractor can produce, so building one in a test would let it pass
against a corpus that cannot exist.

Each fixture is here because it encodes a specific failure mode found while
validating the approach — none is a generic sample.

## Pages

| Fixture | Source | Why |
|---|---|---|
| `pages/budget-p0066.txt` | Budget Book p66 | The citywide spine. Every control total ties to this page. Rows are "label followed by 4 numbers" (2 fund groups × 2 fiscal years), and it is the **label anchor** for p67. It also carries the running footer `BUDGET FY 2025-27 … Page 62`, whose bare page number parses as an amount — which is why every block on this page states a `stop_at`. |
| `pages/budget-p0067.txt` | Budget Book p67 | The spine's continuation, and the nastiest case in the corpus: **no row labels at all** — identity is positional from p66, so a single missing row mismaps every row beneath it. This fixture is also the evidence for `fisc-c00`: it used to carry nine revenue rows because xberg's `strip_repeating_text` deleted the third of three identical all-dash rows, and the count assertion (72 ≠ 80) is what caught it. Any mapping engine must fail loudly here, not guess. |
| `pages/budget-p0167.txt` | Budget Book p167 | General Fund expenditures by department × object category, and the page with the **tightest line structure in the corpus**: the smallest gap between two printed lines is 7.99pt against a 4.16pt clustering tolerance, and the gap in question is a wrapped department name (`Devel`) sitting immediately above a data row. It is also where a *wrapped label lands between a row's figures and the next row's label* (`fisc-0cs`), so the labelled read cannot yet read it — it is here for the geometry, not for a rule. |
| `pages/budget-p0076.txt` | Budget Book p76 | The transfer schedule, and the fixture that retired a bead's premise: it was believed unextractable and slated for hand transcription until poppler replaced xberg, and it is 22 ordinary labelled rows. It is here for three things at once — a row whose identity is **two printed fields** (`Transfer From X … to Y`) separated by an arbitrary run of spaces, which `Row.Label` cannot express without encoding the page's kerning; a **footnote marker** trailing every data row — `(1)` through `(10)`, keyed to the descriptions at the foot of the page — which `amount.Parse` reads as a *parenthesised negative* because that is how these documents write one; and a second `fisc-0cs` wrapped label (`Connection`) outside the department pages. Its budget columns tie to the printed total to the cent and its historical columns miss by millions, so it is also the counter-example to "declare the difference and move on". |
| `pages/budget-p0127.txt` | Budget Book p127 | General Fund revenue by source, and the **contra-revenue** case: ERAF and RPTTF are negative rows (~26% of gross property tax) that a Sankey cannot render as links. Also shows the 4-column shape (FY23-24 actual, FY24-25 revised, FY25-26, FY26-27) that makes every revenue line a trend series. |

## Geometry

`pdftotext -bbox` word boxes, one file per fixture page, read by `internal/geom`
and by the resolver's column guard. Each is `[x0, y0, x1, y1, "text"]` per word,
in points, y increasing downward.

| Fixture | Source | Why |
|---|---|---|
| `geometry/budget-p0066.json` | Budget Book p66 | The labelled half of the spine. Four columns, and the page whose worst intra-line y0 spread (0.96pt against a 4.55pt tolerance) sets the floor the line grouping has to tolerate — and that one cluster is a mixed label/header line, `EXPENDITURES: General Fund Enterprise Funds`, not a data row. |
| `geometry/budget-p0067.json` | Budget Book p67 | The label-less half, and the fixture the column guard is proved on: eight columns, 24 data lines, every one carrying exactly eight value words including four byte-identical all-dash rows. Its header line prints `FY 2025-26 / FY 2026-27` once per fund group and nowhere else on the page, which is what lets a rule name its columns without an ordinal. |
| `geometry/budget-p0076.json` | Budget Book p76 | The second substrate for the transfer schedule, and the evidence that nothing is missing from the first: the page prints two unusually wide vertical gaps (21pt where rows are otherwise 13pt apart) and geometry shows they are whitespace, not dropped rows. It is read by no test beyond the verbatim-copy check: it is committed because the page-text fixture's 22 rows are only trustworthy while the second substrate agrees, and the day something disputes them this is what settles it. |
| `geometry/budget-p0127.json` | Budget Book p127 | The four-column **labelled** shape, so the guard's labelled path is exercised against a real page rather than only against p66's four columns inside a padded block. |
| `geometry/budget-p0167.json` | Budget Book p167 | The evidence for the filing rule. This page's `FY 2025-26` header spans x 421.99–469.01 while every figure under it *ends* at 477.8–477.9 — the figures are right-aligned to a grid offset ~+9pt right of the header text, so filing a value by whether it overlaps its header places none of them, and the tokens it loses are zero dashes. |

### CIP

| Fixture | Source | Why |
|---|---|---|
| `pages/cip-p0029.txt` + `geometry/cip-p0029.json` | CIP p29 | `fisc-j5p`: the page prints one figure of $21,130,083 as **`$21130 083`**, two whitespace-delimited tokens, so `amount.Parse("$21130")` succeeds and the rest of the row shifts one place. `internal/amount`'s digit-splitting guard cannot see it, because the split falls across a token boundary. Geometry can: both tokens' right edges (280.45 and 299.67) fall in the same column band, which ends at 301.54. The whole row is shattered this way — `$ 6 190 000` is four words — so it is also the corpus's worst case for a substrate that has lost its thousands separators. |
| `pages/cip-p0040.txt` + `geometry/cip-p0040.json` | CIP p40 | The sparse grid, and the limit of what geometry can do. Row `PB200654` prints two figures with six columns between them and `-layout` emits exactly two tokens, which is the mis-filing this whole guard exists to prevent. But the page *prints* a `-` in each of those six columns and **those dashes are in no substrate at all** — not `-layout`, `-raw`, the default mode, or `-bbox` — because they are drawn as non-text, while `PB200429` on the same page does carry its dashes. So the row fails closed and cannot be read (`fisc-8ln`). |

## Projections

| Fixture | Source | Why |
|---|---|---|
| `sankey.golden.json` | Budget Book pp. 66–67, FY2026 | The frozen `sankey.json` contract as a worked example — real figures, real fact ids, all four `link.kind` values, and both `derived: true` nodes. Hand-derived in Wave 0 so `internal/export` and `site/app.js` could be built before `internal/project` existed. Once the Go projection lands it must reproduce this byte-for-byte; until then it is the only thing pinning the contract. See [`docs/sankey-contract.md`](../docs/sankey-contract.md). |
