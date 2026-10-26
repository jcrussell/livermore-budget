# Test fixtures

Real artifacts copied from `data/extracted/`, each here for one named failure
mode; none is a generic sample. How to keep them in step with the corpus is
AGENTS.md, "Testing". Every page fixture carries both substrates,
`pages/<doc>-pNNNN.txt` and `geometry/<doc>-pNNNN.json`, because one without
the other is not a shape the extractor produces.

## Pages

| Fixture | Source | Why |
|---|---|---|
| `pages/budget-p0066.txt` | Budget Book p66 | The citywide spine and the **label anchor** for p67, whose running footer `BUDGET FY 2025-27 … Page 62` parses as an amount, which is why every block on it states a `stop_at`. |
| `pages/budget-p0067.txt` | Budget Book p67 | The spine's continuation with **no row labels at all**, identity positional from p66, so one missing row mismaps every row beneath it (`fisc-c00`). |
| `pages/budget-p0167.txt` | Budget Book p167 | The **tightest line structure in the corpus** (7.99pt between lines against a 4.16pt tolerance), a wrapped department name between a row's figures and the next label (`fisc-0cs`), and the city's misspelt `DEVELOPEMENT` total row. |
| `pages/budget-p0076.txt` | Budget Book p76 | The transfer schedule, whose rows are identified by **two printed fields** (`Transfer From X … to Y`) and trail a footnote marker `(1)`-`(10)` that `amount.Parse` reads as a parenthesised negative. |
| `pages/budget-p0080.txt`, `pages/budget-p0081.txt` | Budget Book pp.80-81 | The debt service schedules, the corpus's only **rows that are years**, including the band `2031-2035` that must never be read as its first year. |
| `pages/budget-p0222.txt` | Budget Book p222 | The CIP funding bridge, whose rows are identified by **three printed fields** (`601   600       Airport`) and whose one gap holds two wrapped fragments. |
| `pages/budget-p0175.txt` | Budget Book p175 | A fund's **printed total wraps** (`Total County Meas BB-` / `Bike/Pedestrian`), so `total_row_tail` has to be read off the page. |
| `pages/budget-p0186.txt` | Budget Book p186 | A **section heading on a line of its own between rows**, neither a wrapped label nor a figure, which is why `headings` exists. |
| `pages/budget-p0187.txt` | Budget Book p187 | The label-less **uses half** of p186's transposed grid, where a rule's category sits on the column and a first data line prints as many `$` figures as the total line. |
| `pages/budget-p0190.txt` | Budget Book p190 | The **mixed gap**: a footnote marker `1` and a heading on consecutive lines between two rows (`TestMixedGapOfFigureAndHeading`). |
| `pages/budget-p0206.txt` | Budget Book p206 | The labels for p207, read by the real-page `omitted_cells` test. |
| `pages/budget-p0207.txt` | Budget Book p207 | The blanks `omitted_cells` exists for: County Measure D's line reads `505,784 \| - \| - \| (blank) \| 505,784 \| (858,450)`, and (358,666) + 6,000 - 505,784 = (858,450) against p206. |
| `pages/budget-p0188.txt`, `p0189`, `p0191` … `p0205`, `p0208`, `p0209` | Budget Book pp.188-209 | The rest of the fund-balance schedule, so `TestPublishedFundBalancesSummaryRowsAreTheirBlockTotals` can join each year's summary block to block totals up to two page pairs later. |
| `pages/budget-p0168.txt` | Budget Book p168 | The two-level rounding chain: `ADMINISTRATIVE SERVICES TOTAL` $6,311,564 equals its division totals while its object rows give $6,311,563, which is why `File.Rollups` sums printed totals (`fisc-3bl`). |
| `pages/budget-p0169.txt` | Budget Book p169 | The schedule's only page that is both a **tail and a head**, carrying three of the four wrapping division labels and two opposite-signed `stated_total_deltas`. |
| `pages/budget-p0170.txt` | Budget Book p170 | `General Fund Total Expenses`, $144,650,802 and $149,014,579, p66's General Fund `TOTAL EXPENDITURES` to the cent, and Fire Administration's three statements of one figure inside nine lines. |
| `pages/budget-p0127.txt` | Budget Book p127 | The **contra-revenue** rows ERAF and RPTTF (~26% of gross property tax) and the 4-column shape that makes every revenue line a trend series. |
| `pages/budget-p0128.txt` | Budget Book p128 | **Two** discrepant blocks of opposite sign on one page (`Total Other Taxes` +$1, `Total Licenses & Permits` -$2), so a declaration keyed by page cannot express them (`fisc-2sd`). |
| `pages/budget-p0129.txt` | Budget Book p129 | The longest block in the corpus, twenty rows guarded only by whitespace, and the head of `Contributions Outsourced`, whose total is on the next page. |
| `pages/budget-p0130.txt` | Budget Book p130 | Three anchor problems: a total whose block has no rows on the page, `Miscellaneous Revenue` occurring three times, and the `Total General Fund` rollup over ten category rules. |
| `pages/budget-p0131.txt` | Budget Book p131 | Revenue rows **and** a `Transfers In` row inside one printed `Total <fund>`, the case `Row.Kind` was promoted for. |
| `pages/budget-p0132.txt` | Budget Book p132 | A **fund-group boundary mid-page**, and the `Water` family where the string occurs six times. |
| `pages/budget-p0133.txt` | Budget Book p133 | The **unindented layout** (with pp.136-138), so a column guard tuned on the indented pages does not transfer, and fund names carrying digits and `-`. |
| `pages/budget-p0134.txt` | Budget Book p134 | Three fund-group boundaries on one page and the corpus's only **Permanent Funds** section, which pp.66-67 give no column (`fisc-u8o`, `fisc-u54`). |
| `pages/budget-p0135.txt` | Budget Book p135 | The Special Revenue run opens, with `SOEF (Social Op Endowment)`, a fund name in the parentheses this corpus uses for negatives. |
| `pages/budget-p0136.txt` | Budget Book p136 | `Police Evidence`, fund 211 by arithmetic rather than lookup (`fisc-nne`, `fisc-8dz`), with four funds zero in both budget years. |
| `pages/budget-p0137.txt` | Budget Book p137 | The `Grant - ` prefix family, six fund headings sharing a prefix, the case an exact-match resolver is for. |
| `pages/budget-p0138.txt` | Budget Book p138 | `City Street Sweeping Fee`, whose heading is on p137 and whose rows and total are here, the mirror of p129's straddle. |
| `pages/budget-p0139.txt` | Budget Book p139 | The corpus's only `Oth Financing Source` row, $500 that the citywide roll-ups p63 and p192 both exclude (`fisc-94b`). |
| `pages/budget-p0140.txt` | Budget Book p140 | `Total Sources`, which exceeds the fund totals it closes by ~$57M, differently per column, the tree's only `Rollup.Unassertable` entry (`fisc-wev`). |

## Geometry

`pdftotext -bbox` word boxes, one file per fixture page, read by `internal/geom`
and by the resolver's column guard. Each is `[x0, y0, x1, y1, "text"]` per word,
in points, y increasing downward.

| Fixture | Source | Why |
|---|---|---|
| `geometry/budget-p0066.json` | Budget Book p66 | Its worst intra-line y0 spread (0.96pt against a 4.55pt tolerance) sets the floor line grouping must tolerate. |
| `geometry/budget-p0067.json` | Budget Book p67 | The page the column guard is proved on: eight columns, every data line carrying exactly eight value words, four of them byte-identical all-dash rows. |
| `geometry/budget-p0076.json` | Budget Book p76 | Shows the page's two wide vertical gaps are whitespace, not dropped rows. |
| `geometry/budget-p0080.json`, `geometry/budget-p0081.json` | Budget Book pp.80-81 | Places each figure under its fund type where a row stops short, as p81's 2036-2040 line does. |
| `geometry/budget-p0222.json` | Budget Book p222 | Shows a wrapped description line carries label text and no figure. |
| `geometry/budget-p0175.json` | Budget Book p175 | Shows each wrapped total's tail is label text at the label column's x-extent. |
| `geometry/budget-p0186.json` | Budget Book p186 | A footnote superscript the geometry clusters into the line below while `-layout` prints it alone (`TestAFootnoteSuperscriptPairs`, `TestP186ReadsGuarded`). |
| `geometry/budget-p0187.json` | Budget Book p187 | The overhanging grid, where a band ending at the gap's midpoint files a Transfers Out figure one column over (`TestP187IsPlacedByTheColumnGuard`). |
| `geometry/budget-p0190.json` | Budget Book p190 | The same clustered superscript as p186's, over a label line printing no figure. |
| `geometry/budget-p0206.json` | Budget Book p206 | p207's label page, so the fixture carries both substrates. |
| `geometry/budget-p0207.json` | Budget Book p207 | The overhanging grid: `800,550` ends at x 221.09, past the 219.42 midpoint between headers (`TestP207IsPlacedByTheColumnGuard`). |
| `geometry/budget-p0188.json`, `p0189`, `p0191` … `p0205`, `p0208`, `p0209` | Budget Book pp.188-209 | The rest of the fund-balance schedule, which the published rules read guarded on every part. |
| `geometry/budget-p0168.json` | Budget Book p168 | pp.167-170 print `-` in every zero cell, so every object row carries exactly four value words and a short row is a dropped figure. |
| `geometry/budget-p0169.json` | Budget Book p169 | Shows the wrapped division labels' tails are label text, not values that lost their row. |
| `geometry/budget-p0170.json` | Budget Book p170 | The schedule's sparsest object rows, where a mis-filed column would be least visible by eye. |
| `geometry/budget-p0127.json` | Budget Book p127 | Exercises the guard's labelled path against a real four-column page. |
| `geometry/budget-p0167.json` | Budget Book p167 | The `FY 2025-26` header spans x 421.99–469.01 while every figure under it ends at 477.8–477.9, so filing by header overlap places none of them. |
| `geometry/budget-p0128.json` … `geometry/budget-p0140.json` | Budget Book pp.128-140 | The revenue schedule's grid is per page — indented on pp.131/132/134/135/139/140, at the margin on pp.133/136-138 — so a guard proved on one layout says nothing about the other. |

### ACFR

| Fixture | Source | Why |
|---|---|---|
| `pages/acfr-p0177.txt` + `geometry/acfr-p0177.json` | ACFR p177 | The outstanding-debt schedule behind `amount.TestLeadingMinusIsReallyPositive` and the quantity channel's percentage and per-capita columns (`TestACFRDebtPercentageColumnNeedsItsQuantity`, `TestACFRDebtPerCapitaParsesCleanlyAsAmount`), whose FY2024 total is short by exactly its Financed Purchases column, $176,292. |
| `pages/acfr-p0041.txt` + `geometry/acfr-p0041.json` | ACFR p41 | The General Fund's condensed statement in millions to two decimals, with an orphan `0.0` between the last revenue row and its total that breaks the geometry pairing (51 lines against 52) and the corpus's only document-derived tolerance (`fisc-1wr.2`, `fisc-h96o`, `fisc-hcus`). |
| `pages/acfr-p0054.txt` + `geometry/acfr-p0054.json` | ACFR p54 | The audited governmental funds statement, mapped by no rule, whose General Fund ending balance $87,043,576 `TestP167ComponentsTieToP54NotP41` ties p167's components to, where p41's `87.10` does not. |
| `pages/acfr-p0167.txt` + `geometry/acfr-p0167.json` | ACFR p167 | Two blocks with identical row labels but one (no Assigned line in All Other Governmental Funds), the wrong-fund proof `TestACFRFundBalancesWrongBlockIsRefused`, and a detached `$` per column. |
| `pages/acfr-p0168.txt` + `geometry/acfr-p0168.json` | ACFR p168 | The three-line wrapped label "Excess of Revenues / over (under) / expenditures" that `excess-of-revenues-identity` recomputes in all ten columns. |
| `pages/acfr-p0169.txt` + `geometry/acfr-p0169.json` | ACFR p169 | Read whole and published nowhere: its 2023 total copies 2022 (`fisc-qyrw`), and its last row is the whole-row quantity channel's real-page consumer (`TestACFRChangesRowQuantityIsLoadBearing`). |

### CIP

| Fixture | Source | Why |
|---|---|---|
| `pages/cip-p0029.txt` + `geometry/cip-p0029.json` | CIP p29 | `$21130 083`, one figure split across two tokens so `amount.Parse("$21130")` succeeds, which only geometry's shared column band catches (`fisc-j5p`). |
| `pages/cip-p0040.txt` + `geometry/cip-p0040.json` | CIP p40 | Row `PB200654`'s printed dashes are in no substrate at all, drawn as non-text, so the row fails closed and cannot be read (`fisc-8ln`). |

## Projections

A pinned page is named after the path `fisc export` writes it at:
`<page>.golden.html` is the page at `<page>.html`, so `index.golden.html` is
the glance page and `flows.golden.html` the Sankey.

| Fixture | Source | Why |
|---|---|---|
| `sankey.golden.json` | Budget Book pp. 66–67, FY2026 | The hand-derived worked example of the spine document ([`docs/outputs/sankey.md`](../docs/outputs/sankey.md)), which `TestSankeyReproducesGoldenFile` requires Go to reproduce byte for byte; regenerate it only with a rule cheap enough to check its diff by eye. |
| `fund-flows.golden.json` | Budget Book pp. 127–140 + 167–170, FY2026 | **A capture of Go's output**, not a derivation, held line for line to what `buildProjections` builds by `TestTheFundFlowsFixtureIsTheDocumentTheSiteDraws`; check a recapture by re-deriving every link's locators and `value_cents` from `facts/facts.jsonl`. |
| `index.golden.html` | Budget Book pp. 66–67 and pp. 85–125, both published years, over the client subset | The glance page over `clientSubset`, held to Go's bytes by `TestTheGlanceFixtureIsWhatGoWrites`. |
| `flows.golden.html` | The spine page, both published years, over the client subset | The Sankey page whose DOM and `window.FISC_CONFIG` the client tests boot, held decoded and whole to what Go writes by `TestTheClientFixturesHoldWhatTheClientReads`. |
| `fy2026-adopted.column.json` | Budget Book pp. 66–67, 76, 85–125, 127–140, 167–170, FY2026, over the client subset | The column document the page fetches, over `clientSubset` (every fund group with a non-zero figure kept whole, `TestTheClientSubsetDrawsEveryGroupWhole`), held by `TestTheClientFixturesHoldWhatTheClientReads` and broken once per fault by `TestTheClientFixtureChecksCanFail`. |
| `fy2027-adopted.column.json` | The same pages, FY2027 | The second column, which differs in shape — `fund/207`, capital-outlay's divisions and the declared 250,000 gap — so a year joined to the wrong document is visible. |
| `department.golden.html` | Budget Book pp. 85–125, every column, over the client subset | The department page and its config, held to what Go writes by `TestTheDepartmentFixtureHoldsWhatTheClientReads`. |
| `fy2024-actual.column.json` | Budget Book pp. 85–125 and the other schedules of FY2024's actual column, over the client subset | The one column printing a Transfers Out row in pp.85-125's upper block and five departments held apart by a declared exception. |
| `capital.golden.html` | Budget Book pp. 225–235, over the client subset | The capital page, held to Go's bytes by `TestTheCapitalFixtureIsWhatGoWrites`. |
| `change.golden.html` | ACFR p168 and Budget Book pp. 127–140, over the client subset | The Over time page, held to Go's bytes by `TestTheChangeFixtureIsWhatGoWrites`. |
| `ledger.golden.html` | The expansion schedule, over the client subset | The bars page's config and markup, no figures, held to Go's bytes by `TestTheSidesFixtureIsWhatGoWrites`. |
| `caveats.golden.html` | Every caveat over the client subset | The caveats page, whose fragments `site/theme.test.mjs` opens, held to Go's bytes by `TestTheCaveatsFixtureIsWhatGoWrites`. |
