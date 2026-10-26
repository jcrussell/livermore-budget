# The ACFR ten-year history

Two documents over the ACFR Statistical Section's fund-balance schedules, each
at `data/<stem>.json` in the shape of
[`revenue-trends.json`](revenue-trends.md) (`schema/series.schema.json`). How
they are built is in `go doc ./internal/project FundBalances` and
`FundBalanceChanges`.

| stem | schedule | page |
|---|---|---|
| `fund-balances` | Fund Balances of Governmental Funds | `balances.html` |
| `changes-in-fund-balances` | Changes in Fund Balances of Governmental Funds, p168's rows only | `change.html` |

## Reading them

- **Ten columns, FY2016–FY2025, basis `audited`, one `comparable_group`.** The
  pages do not print that word: both schedules sit in the section p161 heads
  "Statistical Section (Unaudited)". The basis is part of every fact id, so it
  stays; each document carries a `statistical-section-unaudited` caveat, and
  the site labels these columns `unaudited`.
- **`fund` is `null` and `fund_name` is `""` on every series.** These rows are a
  fund's balance components or an aggregate across funds, never a numbered
  fund; the printed block headings carry the identity.
- **p167's two blocks print the same row labels** (Nonspendable, Restricted,
  Committed, Unassigned in both; Assigned in the General Fund only). The series
  id tells them apart, as does `fund_group`: `general` on the General Fund
  block, `""` on All Other Governmental Funds, which spans fund types.
- **The changes document's expenditure rows are on the ACFR's function axis**
  (`fire`, `police`, `library-function`, …), not the Budget Book's department
  or object axes. Its one `fund_balance` series is the printed excess line.

## What is not published, and why

- **The printed total rows** (`Total revenues`, `Total Expenditures`, `Total
  general fund`, `Total all other governmental funds`). Each block's components
  tie to its printed total exactly, in all ten columns of all four blocks.
- **p169 whole**: Other financing sources (uses) and Net change in fund
  balances. The block's printed 2023 total repeats the 2022 figure: the
  components sum to $39,259,064 against a printed $(1,767,367), and Net change
  2023 carries the same copy (`fisc-qyrw`).
- p169's debt-service ratio note: recognised as a non-amount and never
  published (`fisc-9tn4`).

The printed excess of revenues over expenditures recomputes from the published
revenue and expenditure rows in all ten columns, to the dollar.

**Nothing guards a whole column filed under the wrong year.** Both pages print
bare-year headers, which the geometry column guard cannot use (`fisc-wiyg`),
and a column shifted whole still ties to its own printed total.

## p41, the General Fund summary, is mapped and not drawn

ACFR MD&A p41, the General Fund's condensed Statement of Revenues, Expenditures
and Changes in Fund Balances, is mapped at scope `acfr-general-fund-summary`:
20 FY2024-25 figures in millions to two decimals, so the least significant
printed digit is $10,000. No published document selects it (`fisc-a5ii`).

**What the page prints.** Four blocks, three mapped whole:

- ten revenue rows, tying to the printed Total Revenues of 157.20 exactly;
- two transfer legs, 0.53 in and (25.72) out, tying to the printed Total Other
  Financing Sources (Uses) of (25.19) exactly;
- beginning balance 92.10, change (5.00), ending 87.10.

The expenditure block is mapped in part: the five divisions of the General
Government function. They print 18.44 against a printed subtotal of 18.45, one
printed unit over five terms, so this is the one place in the corpus tied
within a tolerance: half a unit per row, $25,000 (`fisc-1wr.2`).

**It is undrawn because it is a different year on a different basis.** The
spine is FY2026 and FY2027 adopted; this is FY2025 audited. It joins neither
history table either: it is the General Fund alone where pp.168-169 combine all
governmental funds, and its ending balance does not tie to p167 (below).

**No geometry column guard stands over these figures.** The printed headers are
the bare years 2025 and 2024, and a stray "0.0" with no row label sits inside
the line tolerance of the row above, giving 51 geometry lines against 52 text
lines on the committed `testdata/` copies of p41.

**Not mapped.**

- Six of the expenditure block's other top-level rows are ACFR functions
  (Fire, Police, Public Works, Community Development, Economic Development,
  Library) with no department in the taxonomy (`fisc-xudn`).
- Capital Outlay, Principal and Interest and fiscal charges are not mapped.
- No mapped row carries a department: two of the five divisions name
  departments that cover several divisions.
- The FY2024 column is skipped. Revenue misses by $200,000, expenditure by
  $100,000 and the fund balances by $100,000 (twenty, ten and ten printed
  units); only Other Financing Sources ties, and General Government misses by
  a further $170,000.

## Cross-page ties measured and found dead

- p167's year-over-year total deltas match p169's Net change in only 3 of the 9
  comparable years: 2017, 2020 and 2022, the last a column p169 copies into
  2023.
- p41 gives an ending General Fund balance of $87.10 million. p167's 2025
  components sum to $87,043,576, a figure the audited statement (p54) prints
  to the dollar (`fisc-y242`). This ships as a caveat on `fund-balances`.
- The two ties that hold in the Statistical Section (p187↔p189, p193↔p192) are
  between pages not published.
