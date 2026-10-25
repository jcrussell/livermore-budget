# The ACFR ten-year history contract

Two documents over the ACFR Statistical Section's fund-balance schedules:

| stem                       | schedule                                                | scope                           |
| -------------------------- | ------------------------------------------------------- | ------------------------------- |
| `changes-in-fund-balances` | ACFR p168, Changes in Fund Balances of Governmental Funds | `acfr-changes-in-fund-balances` |
| `fund-balances`            | ACFR p167, Fund Balances of Governmental Funds            | `acfr-fund-balances`            |

Each ships at `<output>/data/<stem>.json` under its projection's `Name()`, like
`revenue-trends`: it spans many columns and names none. The schedule titled
"pp.168-169" publishes only p168's rows; see "What is not published".

## Shape

The `revenue-trends.json` shape exactly: both build through the same
`seriesSpec` core into `project.TrendsDocument`, so `trend-points-tie-to-facts`
and `trend-series-are-complete` read them. Differences of content:

- **Ten columns, FY2016–FY2025, all `audited`**, one `comparable_group`. The
  pages do not print that word: both schedules sit in the section p161 heads
  "Statistical Section (Unaudited)". `basis` is component 7 of the fact id, so
  re-basing would rewrite every id; instead each document carries a
  `statistical-section-unaudited` caveat, and `internal/export` reads it to label
  the column chips and cell tooltips `unaudited`.
- **`fund` is `null` on every series and `fund_name` is `""`.** These rows are a
  fund's components or an aggregate across funds, never a numbered fund. The
  printed block headings carry the identity.
- **p167's two blocks print the same row labels** (Nonspendable, Restricted,
  Committed, Unassigned — both blocks; Assigned — General Fund only). The series
  id tells them apart, as does `fund_group`: `general` on the General Fund block,
  `""` on All Other Governmental Funds, which spans fund types.
- The changes document's expenditure rows are on the **ACFR's function axis**
  (`fire`, `police`, `library-function`, …), not the Budget Book's department or
  object axes. Its one `fund_balance` series is the printed excess line, category
  `fund-balance/excess-of-revenues`.

`project.HistoryColumns` states the ten columns, so a corpus that lost a year
goes red: `TestPublishedDocumentsAreWhatTheCorpusBuilds` and
`published-projection-built` hold the store to it.

## What is not published

- **The printed total rows** (`Total revenues`, `Total Expenditures`,
  `Total general fund`, `Total all other governmental funds`). Each block's
  components tie to its printed total at build time, exactly, in all ten columns
  of all four blocks.
- **p169 whole**: the Other financing sources (uses) block and Net change in fund
  balances. The block's printed 2023 total repeats the 2022 figure — the
  components sum to $39,259,064 against a printed $(1,767,367) — and Net change
  2023 carries the same copy (`fisc-qyrw`).
- p169's row-shaped non-amounts (the debt-service ratio note): recognised
  non-amount columns are read, never published (`fisc-9tn4`).

## What guards the figures, and what does not

- `excess-of-revenues-identity` recomputes the printed excess from the published
  revenue and expenditure rows per column, at zero tolerance; it holds in all ten.
- Build-time block totals; the store-level witnesses; the two trend checks.
- `fisc export` refuses a document whose counts disagree with the cells a page
  lays out, a series no declared section claims, and a section claiming none.
- **Nothing guards a whole column filed under the wrong year.** Both pages print
  bare-year headers, which the geometry guard refuses (`fisc-wiyg`), and a column
  shifted whole still ties to its own printed total.

## p41, the General Fund summary, and why no document draws it

ACFR MD&A p41, the General Fund's condensed Statement of Revenues, Expenditures
and Changes in Fund Balances, is mapped at scope `acfr-general-fund-summary`: 20
FY2024-25 figures in millions, where pp.167-169 print dollars. It is a cut of the
hierarchy (`structure.ACFRCuts`) and no published document selects it
(`fisc-a5ii`).

**What the page prints.** Four blocks, three mapped whole: ten revenue rows tying
to the printed Total Revenues of 157.20 exactly; two transfer legs, 0.53 in and
(25.72) out, tying to the printed Total Other Financing Sources (Uses) of (25.19)
exactly; and beginning 92.10, change (5.00), ending 87.10. The expenditure block
is mapped in part: the five divisions of the General Government function. All in
millions to two decimals, so the least significant printed digit is $10,000.

**It is undrawn because it is a different year on a different basis.** The spine
is FY2026 and FY2027 adopted; this is FY2025 audited.
`peers-overlap-only-by-declared-identity` refuses the pair by name, and
`structure.NewView` a view holding both. It can join neither history table: it is
the General Fund alone where pp.168-169 combine all governmental funds, and its
ending balance does not tie to p167's components (below).

**What holds these facts.** `fund-balance-identity` reaches three of the 20
(beginning + change == ending), and `CheckTotals` the other 17. The General
Government block is the one place in the corpus that ties only within a
tolerance, and the build report says so: its five divisions print 18.44 against a
printed subtotal of 18.45, one printed unit over five terms, so the bound is half
a unit per row — $25,000 (`fisc-1wr.2`). It is also the only rule reading a total
printed above its rows, bounded by pinning that total to the section anchor's own
line (`fisc-h96o`).

**No geometry column guard stands over these figures.** The printed headers are
the bare years 2025 and 2024, which the parser refuses as headers, and the
geometry line pairing fails anyway: a stray "0.0" with no row label sits inside
the line tolerance of the row above, giving 51 geometry lines against 52 text
lines. `internal/mapping`'s `TestACFRp0041HasNoGeometryColumnGuard` measures both.

**Still not mapped**: six of the expenditure block's nine other top-level rows are
the ACFR's remaining functions — Fire, Police, Public Works, Community
Development, Economic Development, Library — which `fact-departments-resolve`
cannot accept (`fisc-xudn`); Capital Outlay, Principal and Interest and fiscal
charges are simply unmapped. No mapped row carries a department: two of the five
divisions name departments covering several divisions (`data/taxonomy.yaml`'s
general-government entry). The FY2024 column is skipped: revenue misses by
$200,000, expenditure by $100,000 and the fund balances by $100,000 (twenty, ten
and ten printed units), only Other Financing Sources tying, and General
Government misses by a further $170,000.

## Cross-page ties measured and found dead

p167's year-over-year total deltas match p169's Net change in only 3 of the 9
comparable years — 2017, 2020 and 2022, the last a column p169 copies into 2023.
p41 gives an ending General Fund balance of $87.10 million where p167's 2025
components sum to $87,043,576, a figure the audited statement (p54) prints to
the dollar (`fisc-y242`); it ships as a caveat on `fund-balances`. The two ties
that hold in the Statistical Section (p187↔p189, p193↔p192) are between pages
not published.

## The reader surface

`balances.html` renders `fund-balances` server-side through
`site/history.html.tmpl`, its block headings arriving as `export.View.Sections`
and validated against the document both ways. `changes-in-fund-balances` is
`change.html`'s document, drawn by `site/change.js`.
