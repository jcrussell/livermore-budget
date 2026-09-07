# The ACFR Statistical Section, table by table (pp.163-194)

The survey fisc-oakx.1 asked for: every table classified before any is mapped.
All figures were measured off `data/extracted/livermore-acfr-fy2025/pages/pNNNN.txt`
at commit cbd1e72, by reading the pages and re-computing the arithmetic quoted
here; the p169, p187 and p192 entries were re-measured off the same pages at
commit 3da39e1; nothing is carried forward from a prose summary.
`amount.Parse` behavior was probed against `internal/amount` at cbd1e72.
Re-derive the counts from the pages before relying on them — this file does
not re-measure itself.

28 tables over pp.163-194. p162 is prose, p195 is blank. A page range in the
first column is one logical table straddling pages.

The section's divider, p161, reads in full: "Statistical Section (Unaudited)".
The auditor's report (p29) disclaims the section by name: "Our opinions on the
basic financial statements do not cover the other information, and we do not
express an opinion or any form of assurance thereon." So no table below carries
the audit opinion, whatever basis its figures were first published under — any
page that publishes one owes the reader that label.

## The classification

Column counts exclude the row-label column. "Years" is the table's year shape:
**10y** ten year-columns, **10y-T** ten year-rows (transposed), **2c** the
"Current Year and Nine Years Ago" two-column convention, **snap** a single-date
snapshot. Quantities use the closed vocabulary proposed below. "Money" is yes
when the table prints dollar figures a rule could publish as integer cents (the
`amount` quantity); it does not claim a `mapping.Kind` fits — see fisc-7jtl.

| Pages | Table | Years | Cols | Column quantities | Money | Unit | Arithmetic guard |
|---|---|---|---|---|---|---|---|
| p163 | Net Position by Component | 10y | 10 | all amount | yes | $ | 3 total rows sum per column; Primary government = governmental + business-type, component by component (2022 NICA: 329,890,590 + 140,388,714 = 470,279,304) |
| p164-166 | Changes in Net Position | 10y | 10 | all amount | yes | $ | section totals sum per column; net (expense) revenue = program revenues − expenses across pages; **defective, see below** |
| p167 | Fund Balances of Governmental Funds | 10y | 10 | all amount | yes | $ | 2 total rows sum per column (2016 GF: 47,139,536 ties) |
| p168-169 | Changes in Fund Balances | 10y | 10 | amount; last row percentage | yes | $ | revenue and expenditure totals sum per column (2016 ties both); excess = revenues − expenditures; **defective, see below** |
| p170 | Taxable Sales by Category | 10y | 10 | all amount | yes | $k, declared "In thousands"; calendar years 2015-2024 | total row sums per column (2015 and 2024 tie) |
| p171 | Direct and Overlapping Sales Tax Rates | 10y | 10 | all percentage | no | % | Total Rate row sums (2016: 9.50%) |
| p172 | Principal Sales Tax Payers | 2c | 2 | none — names only, no figures | no | — | none |
| p173 | Assessed Value and Estimated Taxable Property | 10y-T | 5 | 4 amount, 1 percentage | yes | $ | Total Taxable = Common + Utility + Unsecured per row (2015-16 ties) |
| p174 | Property Tax Rates | 10y | 10 | all number (4dp rates per $1,000; one 3dp cell, 2024 LVJUSD "0.054") | no | per $1,000 | total row sums (2016: 1.1277) |
| p175 | Principal Property Taxpayers | 2c | 5 | 2025: amount, number (rank), percentage; 2016: amount, number | yes | $ | both total rows sum (1,540,405,375 and 666,125,508 tie); sparse rows |
| p176 | Property Tax Levies and Collections | 10y-T | 5 | amount, amount, percentage, amount, amount | yes | $ | per row: levied − collected = delinquent (2016 ties); % of levy = collected / levied |
| p177 | Ratios of Outstanding Debt by Type | 10y-T | 10 | 8 amount, percentage, amount_per_unit | yes | $ | total = sum of 7 debt columns per row (2025 ties; 2024 short by exactly 176,292, its own Financed Purchases cell); % ties to p180 personal income; per capita ties to p180 population |
| p178 | Direct and Overlapping Governmental Activities Debt | snap | 3 | percentage, amount, amount | yes | $ | share = % × outstanding per row (Alameda ties ±1); subtotals and gross/net direct debt sum |
| p179 | Legal Debt Margin Information | 10y | 10 | amount; last row percentage | yes | $ | margin = limit − applicable debt per column; side block: 15% × 26,264,328,578 = 3,939,649,287 ties |
| p180 | Demographic and Economic Statistics | 10y-T | 4 | number, amount, amount_per_unit, percentage | yes | $k for Total Personal Income, **derived not declared**; calendar years 2015-2024 | per row: per capita = income × 1000 / population (2015 ties); cross-ties to p177 |
| p181 | Population Demographics | snap | 2 | number, percentage | no | — | total = sum of 18 age rows (84,849 ties); percentages total 100.0% printed, sum 100.1 |
| p182 | Principal Employers | 2c | 6 | number-or-RANGE, number (rank), percentage, twice | no | — | % total rows sum (31.12 and 27.65 tie); employee columns have no total |
| p183 | FTE City Employees by Function | 10y | 10 | all number (decimal FTEs) | no | FTE | total row sums per column (2016: 395.00 ties) |
| p184 | Capital Assets Statistics | 10y | 10 | all number | no | mixed (miles, yards, MG…) per row label | **none** |
| p185 | Operating Indicators | 10y | 10 | all number (integer and decimal mixed) | no | mixed per row label | **none** |
| p186 | Water and Sewer Rates | snap | varies | amount_per_unit throughout | no | $/meter-size, $/CCF | Total Meter Charge = City + Zone 7 per row (5/8": 54.49 ties) |
| p187 | Water Revenues by Class of User | snap | varies | rates amount_per_unit; revenue sub-table amount + percentage | yes | $/unit; $ | revenue sub-table: 27,360,043 + 5,249,618 = 32,609,661 ties, = p189's FY2025 Charges for services exactly; rate sub-table **defective, see below** |
| p188 | Sewer Connection Fees | snap | varies | fees amount_per_unit; DUE sub-table number | no | $/unit; mg, DUEs | DUE sub-table: all three columns sum (43,203 / 1,957 / 34,662 tie) |
| p189 | Sewer System Historical Operating Results | 10y | 10 | amount; coverage row number | yes | $ | gross revenues total sums per column (2016 and 2025 tie); coverage = LAVWMA net revenues / debt service (2016: 2.73 ties); net after obligations recomputes; **defective, see below** |
| p190-191 | Schedule of Insurance | snap | 5 | none — free text; dollar figures embedded in prose cells | no | — | **none** |
| p192 | Redevelopment Historical Tax Revenues | 9 year-columns, FY2016-17 to FY2024-25 | 9 | amount; Percentage Change row percentage | yes | $k, declared "($ in 000s)" | five sum identities per column: total AV, gross, tax revenues and net recompute in all nine years; incremental fails in four (FY2016-17 off 100, FY2018-19 and FY2020-21 off 1, FY2021-22 off 588 — see below); cross-tie to p194, five of nine years, see below |
| p193 | Redevelopment Ten Largest Property Owners | snap | 3 | amount, percentage, percentage (+ appeals sub-table: number, percentage, amount) | yes | $ | top-ten total sums (418,308,954 ties); % recompute against the two printed denominators, which tie to p192's FY2024-25 row ×1000 |
| p194 | Redevelopment Actual and Projected Tax Revenues | 17 year-rows: 9 actual + **8 projected** | 6 | 5 amount (2 parenthesized-negative), 1 number (coverage) | yes | $ | per row: tax revenues = gross − SB 2557 − 33676 (FY2016-17 ties); coverage = tax revenues / debt service (3.18 ties) |

Headline counts, all re-derivable from the rows above: **16 ten-year tables**
(12 year-column + 4 transposed), **3 two-column**, **1 nine-year** (p192, under
a "Last Ten Fiscal Years" title), **1 seventeen-row** (p194, 9 actual + 8
projected, also titled "Last Ten"), **7 snapshots**. **17 tables print
publishable money, 11 print none.** **4 tables have no arithmetic guard at
all**: p172, p184, p185, p190-191 — and p172 and p190-191 print no figures, so
the two that publish nothing *and* check nothing are p184 and p185.

## Row-shaped non-amounts: four tables

fisc-9tn4 settled a *column* kind on transposed p177. In the section's dominant
orientation (years across), the non-amount is a **row** spanning every year
column, which a column quantity cannot express:

- p169 — "Debt service as a percentage of noncapital expenditures": 6.6% … 4.9%
- p179 — "Total net debt applicable to the limit as a percentage of debt limit": 0.00% ×10
- p189 — "LAVWMA Debt Service Coverage": 2.73 … 10.61 (a `number` row, not a percentage)
- p192 — "Percentage Change": 10.67% … 13.15%

Skipping the row is not an out: `parseRow` (`internal/mapping/resolve.go`)
parses before consulting `row.Skip || col.Skip`, and the trailing arm refuses a
figure-bearing line after the last mapped row. The rows must be *read*.

## Shapes that fit no grammar

Fail closed on every one; none may be read as a nearby plausible number.

- **Ranges** — p182's 2016 employee column: "5600-6000", "1000-1400",
  "500-700", and one printed as "900-100" (Wente), which is not even a valid
  range. A range must never be read as its lower bound.
- **Not-available markers** — p185: "NA", "N/A", and cells holding only a
  footnote marker, "(2)", "(3)". Absent-is-not-zero territory, and distinct
  from `-` (which on these pages means zero-or-none per the amount rules).
- **Text in a money column** — p186's Zone 7 column prints "exempt" among
  dollar rates.
- **Free-text figure cells** — p190-191's LIMITS column: "$5,000,000 per
  occurrence excess of $12,500,000 …". Dollar figures inside prose, not cells.
- **Split rows** — p182 prints Livermore Area Recreation and Parks District's
  rank on one line and its "500-700  1.28%" on the next; its Comcast row has a
  label and no figures at all.

## Sparse rows: genuine absent-is-not-zero

- p175: six 2016-only taxpayers print nothing in the 2025 columns, and six
  2025 taxpayers (Westcore Bravo, Longfellow, Lam Research, Arkay, Livermore
  Multifamily, Pool 2) print nothing in 2016. Not in that year's top ten ≠
  zero assessed value.
- p182: GILLIG, FormFactor, Lam Research, Topcon have no 2016 cells; Wente,
  RGW, ValleyCare have no 2025 cells.
- p164-165: Economic Development and Stormwater begin mid-decade ("-" before
  their first year on p164, truly empty cells on p165's Stormwater 2016).

## Printed defects, measured

These are the city's arithmetic, not extraction error, and the batches must not
absorb them. This section originally said to declare them with
`stated_total_deltas`, and batch one measured that instruction wrong:
`fisc-2sd`'s mechanism is for the city's own ≤$5 rounding, and a totals row a
whole year out of position, or a cell copied from the prior year, is not a
rounding delta — a delta sized to one is the fabricated declaration
`Rule.TotalSpansParts`' doc comment names. What batch one did instead is the
precedent: p169's Other Financing block declares no total and stays read-only
(`fisc-qyrw`), and pp.164-166 wait (`fisc-xd6r`). Every figure below was
recomputed from the page named.

- **p164 — the Total business-type activities expenses row is right-shifted
  one year for 2017-2023.** Each of those seven printed totals equals the
  *previous* year's column sum: printed 2017 = 43,470,416 = the 2016 sum,
  printed 2018 = 48,717,616 = the 2017 sum, … printed 2023 = 49,523,387 = the
  2022 sum. The true 2023 sum, 51,821,693, is printed nowhere. 2016, 2024 and
  2025 tie. The Total primary government expenses row is internally consistent
  with the shifted figures.
- **p165 — Net (Expense) Revenue, Governmental, 2023** prints (95,830,768), a
  copy of 2022; recomputed 35,823,779 − 137,384,411 = −101,560,632. The
  primary-government total is consistent with the copy.
- **p166 — Change in Net Position, Governmental, 2023** prints 10,611,708, a
  copy of 2022; the totals row is consistent with the copy.
- **p169 — Total other financing sources (uses), 2023** prints (1,767,367), a
  copy of 2022; the column's three components sum to 41,558,955 + 6,672,696 −
  8,972,587 = 39,259,064. The transfers pair alone, 6,672,696 − 8,972,587 =
  −2,299,891, is p166's Transfers (net) 2023 exactly — the pair, not the block
  total, because 2023 is the one column that also prints Proceeds from long
  term debt. **Net change in fund balances 2023** prints (16,975,824), a copy
  of 2022; recomputed 21,438,989 + 39,259,064 = 60,698,053. The other nine
  columns tie exactly on both rows.
- **p189 — Total LAVWMA Debt Service 2023** prints 2,082,021, a copy of 2022;
  its own two components sum to 1,849,444. The coverage row is consistent with
  the copy.
- **p192 — Incremental Assessed Values** fail to recompute as total − base
  (70,060) in four of nine years: FY2016-17 prints 653,272 against 723,232 −
  70,060 = 653,172, off 100 (thousand); FY2018-19 prints 734,848 against
  734,847, off 1; FY2020-21 prints 803,990 against 803,989, off 1; FY2021-22
  prints 844,092 against 913,564 − 70,060 = 843,504, off 588. The two off-by-1
  years are plausibly the page's own $000s rounding; the 588 — $588,000 — is
  not rounding. The other five years recompute exactly, as do the page's other
  four identities in all nine years.
- **p187 — Recycled Water, Total Variable Cost** prints 2.77 against a City
  Distribution Cost of 2.8 and a `-` in the wholesale column — a published
  zero, not an absent cell, so the row claims 2.8 + 0 and prints 2.77. The
  section's other five rate rows tie City + Zone 7 = Total exactly.

Cross-table and cross-year ties that looked like guards and are not, measured:

- **p163 year-over-year deltas do not equal p166's Change in Net Position**
  except in 3 of the 9 comparable years: 2017 (15,880,225), 2020 (8,304,751)
  and 2022 (10,611,708). Elsewhere they diverge (2018: delta 6,840,766 vs
  printed 19,161,491; 2024: 7,628,754 vs 15,415,297; 2025: 41,770,382 vs
  43,098,461). Restatements are not shown.
- **p167 deltas vs p169's Net change**: ties in the same 3 of 9 years, 2017
  (17,414,645), 2020 (4,868,219) and 2022 (-16,975,824), and not elsewhere
  (2018 off 190,263, 2025 off 2,667).
- **Both tables' 2022 tie is worth no confidence**: 2023 prints a copy of 2022
  on p166 and on p169, the defect recorded above, so the matching column is
  one the pages already disagree about downstream.
- **p192 vs p194 gross tax revenues**: tie to the thousand for FY2016-17
  through FY2020-21, diverge after (FY2021-22: 8,746k vs 8,297,623).
- **p177's footnote (1) says personal income is Alameda County's; the ratio
  arithmetic says otherwise.** 88,896,362 / 3,573,663,000 = 2.49% ≈ the
  printed 2.5%, and 3,573,663 is p180's *Livermore* Total Personal Income.
  The column ties to p180, footnote notwithstanding.

The tie that does hold exactly: **p187's sewer revenue sub-table sums to
32,609,661, which is p189's FY2025 Charges for services to the dollar**, and
p193's two denominators are p192's FY2024-25 Total and Incremental AV ×1000.

## The geometry guard: 14 tables cannot have it — not 28

fisc-oakx.1's brief said every table heads its columns with bare years and so
none can carry `column_headers`. Measured, that is true of **14 of 28**:
`amount.Parse("2016", dollars)` succeeds, `internal/mapping/parse.go` refuses a
`column_headers` entry `amount.Parse` accepts, and an entry may be null only
for a column headed by nothing — so p163, p164-166, p167, p168-169, p170,
p171, p172, p174, p175, p179, p182, p183, p184, p185 cannot opt in. That is
all of batch one. p189 is marginal (its columns also print "Audited", and one
prints "2017(5)", both rejected by `amount.Parse`).

The other 13 head their columns with text the parser accepts as headers: the
transposed tables (p173, p176, p177, p180, p194), the snapshots (p178, p181,
p186, p187, p188, p190-191, p193), and p192, whose headers are "FY2016-17"
style — rejected by `amount.Parse`, hence declarable. Declarable is
necessary, not sufficient: fisc-oakx.2 measured p177 and the page **cannot**
carry the guard after all — its footnote "(1)" is its own `-layout` line but
sits 2.6pt above its sentence, inside geometry's 4.66pt line tolerance, so the
substrates disagree 23 lines to 22 and the pairing refuses. That is p41's
failure mode, which header parseability cannot predict, so each of the other
12 needs its pairing measured before a rule counts on the guard.
`TestACFRDebtPageCannotCarryTheColumnGuard` re-measures p177's. The gap over
the 14 is accepted and tracked as fisc-wiyg; arithmetic per the table above is
their — and p177's — only guard.

## Recommendation: column quantity plus a whole-row override

Counted from the table above:

- **18 tables** need a non-default quantity on at least one column (or a
  non-amount rule default): the 4 transposed, p171, p174, p175, p178,
  p181-p188, p193, p194. The remaining 10 are all-amount (p163-p170, p179,
  p189, p192, apart from override rows) or print no figures (p172, p190-191).
- **4 tables** need a row-shaped override: p169, p179, p189, p192. Column-only
  cannot express them, and one of the four is in batch one.
- **0 tables** need a per-cell mechanism: every override row is uniform across
  its year columns, and the only mixed column — p182's counts-and-ranges —
  fails closed on the range whatever the mechanism, so cell-level buys nothing.

So: keep fisc-9tn4's column quantity as settled, and add a row override
following the existing idiom — `Row.Kind` / `Row.EffectiveKind(rule)`
(`internal/mapping/rule.go`) already lets a row override a rule-level default,
and `quantity` should get the same pair. Both belong in fisc-oakx.2's schema
change (one migration, not two); the row arm's mutation proofs can land with
batch one's p169 if oakx.2 proves only p177, but the *field* must exist first.

Two constraints the implementation inherits, both measured:

- **The non-amount grammars must not consult `Rule.Units`.** p180 needs
  `units: thousands` for its one amount column while its per-capita column is
  plain dollars and its population column is a count.
- **Thousands is not a quantity.** `internal/amount` already declares
  `dollars`, `thousands`, `millions`; p170 and p192 are ordinary amount tables
  under `units: thousands`. p180's thousands is *derived* (from p177's ratio),
  not declared on the page — the rule declaring it should say so where the
  mapping records rationale.

## The closed vocabulary: `quantity`, four values

`kind` is taken by `mapping.Kind`. The set is closed: a token that fits no
declared quantity's grammar is an error, never a guess — that is what p182's
ranges, p185's "NA" and p186's "exempt" are the test of.

- **`amount`** — the default; dollars at the rule's declared units; integer
  cents; **the only quantity that publishes**. Every committed rule keeps its
  meaning with no edit.
- **`amount_per_unit`** — a dollar-shaped token whose unit is per-something:
  per capita (p177 c10, p180 c3), per meter size or CCF or dwelling unit
  (p186, p187, p188). Read, never published. This is fisc-9tn4's Per Capita
  hazard as a class: probed, "$ 1,009", "32.50", "16.5", "2.8" and "1,044"
  all parse cleanly as dollars, so nothing downstream would catch one filed
  as `amount`.
- **`percentage`** — trailing `%`: p171, p173 c5, p175 c3, p176 c3, p177 c9,
  p178 c1, p180 c4, p181 c2, p182, p193, and the override rows on p169, p179,
  p192. `amount.Parse` rejects these loudly, which is the good failure.
- **`number`** — a unitless figure, grouped or decimal: counts (p180 c1,
  p181, p182, p184, p185, p188), FTEs (p183), ranks (p175, p182), rates per
  $1,000 (p174), coverage ratios (p189's row, p194 c6). One value rather than
  count/ratio/decimal splits because all are read-not-published — the only
  boundary that guards anything is amount-versus-everything-else, and one
  value keeps p183, p184 and p185 on a rule-level default instead of row
  overrides for p185's mixed integer and decimal rows.

Deliberately **not** in the set: a range quantity. p182's 2016 employee column
is omitted, and "900-100" is the argument — this document's ranges cannot even
be trusted to be ranges.

## What this sizes

Batch one (fisc-oakx.3, pp.163-169) needed: the quantity field with its row
override (p169), no geometry guard anywhere, and a decision on p163's kind
(fisc-7jtl) — net position is not a `fund_balance`, so p163 waits at
fisc-311g and the defective pp.164-166 at fisc-xd6r. Its planned p163↔p166 and
p167↔p169 corroboration ties do not hold as printed; the p167 FY2025 ↔ ACFR
p41 tie was then measured dead too (fisc-y242), and the tie the batch landed
is within-page: p168's excess = revenues − expenditures, all ten columns
(`excess-of-revenues-identity`). The four all-non-amount tables
(p181, p183, p184, p185) publish zero facts under fisc-9tn4 and two of them
also carry no arithmetic; mapping those is a reader-surface decision for
fisc-oakx.4, not a default. p194's eight projected rows are the document's own
forecast: they are printed figures and may be read, but publishing them as
history would be false — whatever batch takes p192-194 must separate actual
from projected per AGENTS.md's "A forecast is not a fact".
