# revenue-trends.json

Four printed columns of one detail schedule, Budget Book pp.127-140, *Revenue
Sources by Fund*, as one series per printed row. It ships at
`data/revenue-trends.json`. Its shape is `schema/series.schema.json`; how it is
built is in `go doc ./internal/project Trends`.

## Reading it

- **Money is signed integer cents.** Nine points are negative, all printed in
  parentheses: General Fund ERAF and RPTTF Reduction in all four columns, and
  Prior Year - Unsecured in the FY2023-24 actual column alone. A series is a
  printed row, so contra rows are not netted and summing a fund's series needs
  no special case.
- **A printed `-` is a published zero.** No point is dropped for being zero,
  so a missing point means a row the city stopped printing.
- **`label` is not unique across funds.** "Property Taxes" is printed by four
  funds and "Use of Money & Prop" by thirty-eight; `fund` and `fund_name` say
  whose it is.
- **`series_id` is stable across mappings.** It hashes the row's identity
  without the rule that read it or its year, so mapping a fifth year adds
  points to the same series.
- `counts.facts` and `counts.points` are computed independently; every fact in
  scope becomes a point, so the two are equal.

## The four columns are not one measurement

| column | basis | what it is |
|---|---|---|
| FY 2023-24 | `actual` | money that moved |
| FY 2024-25 | `revised` | a mid-year re-forecast |
| FY 2025-26 | `adopted` | an intention |
| FY 2026-27 | `adopted` | an intention |

`comparable_group` says which columns may be compared arithmetically: the two
adopted years share a group, being one two-year budget; the other two stand
alone. The document publishes no growth rate or other arithmetic between
columns, and a line drawn across them breaks at the actual-to-budget boundary.

FY2024 `actual` is the Budget Book's own restatement, not the ACFR's audited
figure, and is never labelled `audited`.

## What summing the series reproduces

Sum every series into its fund group and compare with p63 Table 2, *Total
Sources – All Funds* (revenues plus transfers in, the two kinds this schedule
carries). Twenty of the twenty-eight cells tie exactly; the differences, in
dollars:

| fund group | FY2023-24 | FY2024-25 | FY2025-26 | FY2026-27 |
|---|---:|---:|---:|---:|
| general | −737,455 | −914,206 | −480,400 | −486,735 |
| enterprise | 0 | 0 | 0 | 0 |
| debt-service | 0 | 0 | 0 | 0 |
| permanent | 0 | 0 | 0 | 0 |
| capital | −1 | −4,125,627 | 0 | 0 |
| special-revenue | +1 | +500 | 0 | 0 |
| internal-service | 0 | 0 | 0 | 0 |

- **general, all four columns.** pp.127-130 print no *General Fund Transfers
  In* row. In the adopted years the gap is exactly the spine's `TRANSFER IN:`
  General Fund cell, 480,400 and 486,735, published from p76 under scope
  `transfers-by-fund`. pp.66-67 print no actual or revised column, so nothing
  on the spine reconciles the other two.
- **capital FY2024-25, −4,125,627** (`fisc-zl9`). General Fund CIP Reserves has
  no section on pp.131-140. p63 prints 34,839,275; the schedule's funds sum to
  30,713,648.
- **special-revenue FY2024-25, +500** (`fisc-94b`). p139 prints `Oth Financing
  Source - 500 - -` in the Police Donations fund; p63 and p192, two roll-ups
  produced differently, both print 19,666,119, exactly $500 less than the 38
  printed `Total <fund>` lines. The document includes the line because the city
  printed it on the page it reads.
- **capital −1 and special-revenue +1, FY2023-24 only.** The document's own
  rounding: eleven blocks are off by at most $5, every one in the FY2023-24
  actual column, p131 and p135 among them (`fisc-2sd`).

`metadata.caveats` states the capital FY2024-25 hole and the General Fund
Transfers In gap, with their figures.

**`permanent` is not on the spine.** Its one series, fund 470 *Transfers In*,
19,533 in FY2023-24 and zero in the other three, ties to p63 exactly; pp.66-67
print no Permanent column (`fisc-u8o`).

The schedule's 79 printed `Total <fund>` and section lines are not series; each
is held at build time, at zero tolerance, against the rows above it.
