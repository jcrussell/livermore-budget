# The ACFR ten-year history contract

Two documents over the ACFR Statistical Section's fund-balance schedules, frozen
on `docs/revenue-trends-contract.md`'s precedent: a published document shape is
a decision in this repository, not a bullet inside the bead that implements it
(`fisc-oakx.4`).

| stem                       | schedule                                                | scope                           | facts |
| -------------------------- | ------------------------------------------------------- | ------------------------------- | ----- |
| `changes-in-fund-balances` | ACFR p168, Changes in Fund Balances of Governmental Funds | `acfr-changes-in-fund-balances` | 220   |
| `fund-balances`            | ACFR p167, Fund Balances of Governmental Funds            | `acfr-fund-balances`            | 90    |

Every figure is already in `facts/facts.jsonl` — no new extraction, no new
mapping. The schedule titled "pp.168-169" publishes only p168's rows; see
"What is not published".

## Where they go

`<output>/data/<stem>.json`, beside `data/revenue-trends.json`. Each stem is its
projection's `Name()` verbatim, for the trends contract's stated reason: a
projection publishing one document spanning many columns must not go through
`project.PublishedStem`, which would write byte-identical files under stems that
lie about which year each covers.

## Shape

The `revenue-trends.json` shape exactly — both projections build through the
same `seriesSpec` core and return `project.TrendsDocument`, which is what lets
`trend-points-tie-to-facts` and `trend-series-are-complete` read them with no
new check. Differences of content, not of shape:

- **Ten columns, FY2016–FY2025, all `audited`,** so every column carries
  `comparable_group: "audited"` — one group, where the trends document has
  three. A comparison may be carried across any pair of years, and the pages
  say so.
- **`fund` is 0 on every series and `fund_name` is `""`.** These schedules'
  rows are a fund's components or an aggregate across funds, never a numbered
  fund. The tables render no Fund column; the printed block headings carry the
  identity instead.
- **p167's two blocks print the same row labels** (Nonspendable, Restricted,
  Committed, Unassigned — both blocks; Assigned — General Fund only). The
  series id tells them apart, as does `fund_group`: `general` on the General
  Fund block, `""` on All Other Governmental Funds, which spans fund types and
  so honestly carries none.
- The changes document's expenditure rows are on the **ACFR's function axis**
  (`fire`, `police`, `library-function`, …), not the Budget Book's department
  or object axes. Its one `fund_balance` series is the printed excess line,
  category `fund-balance/excess-of-revenues`.

## The stated column floor

`project.HistoryColumns` states the ten columns; `Slices` reads the store
exhaustively. The floor is what makes a corpus that silently lost a year go
red — `TestPublishedDocumentsAreWhatTheCorpusBuilds` compares the declaration
against the shipped bytes in both directions, and `published-projection-built`
holds the store to it on every `fisc verify`.

## What is not published

- **The printed total rows** (`Total revenues`, `Total Expenditures`,
  `Total general fund`, `Total all other governmental funds`). Each block's
  components tie to its printed total at build time, exactly, in all ten
  columns of all four blocks.
- **p169 whole**: the Other financing sources (uses) block and Net change in
  fund balances. The block's printed 2023 total repeats the 2022 figure — the
  components sum to $39,259,064 against a printed $(1,767,367) — and Net
  change 2023 carries the same copy, so neither line has an honest printed
  total to stand under (`fisc-qyrw`).
- p169's row-shaped non-amounts (the debt-service ratio note), per
  `fisc-9tn4`'s decision: recognised non-amount columns are read, never
  published.

## What guards the figures, and what does not

- `excess-of-revenues-identity` recomputes the printed excess from the
  published revenue and expenditure rows per column, at zero tolerance — the
  corroboration tie, holding in all ten columns.
- Build-time block totals, as above; the store-level witnesses
  (`fact-offset-points-at-token` and its siblings); and the two trend checks
  over both documents.
- `fisc export` refuses a document whose own counts disagree with the cells a
  page lays out, a series no declared section claims, and a section claiming
  no series.
- **Nothing guards a whole column filed under the wrong year.** Both pages
  print bare-year headers, which the parser's geometry guard refuses
  (`fisc-wiyg`), and a column shifted whole would still tie to its own printed
  total. This is the schedules' open exposure, carried here from the retired
  `unprojectedScopes` declarations.

## Cross-page ties measured and found dead

Recorded so nobody re-plans them: p167's year-over-year total deltas match
p169's Net change in only 3 of the 9 comparable years -- 2017, 2020 and 2022,
the last of them a column p169 copies into 2023; and p41's ending General Fund
balance is $56,424 from p167's 2025 components ($87.10M printed against
$87,043,576 summed, `fisc-y242`). The p41 disagreement ships as a caveat on the
`fund-balances` document. The two ties that do hold in the Statistical Section
(p187↔p189, p193↔p192) are between pages this lane does not publish; see
`docs/acfr-statistical-section.md`.

## The reader surface

`history.html` renders `changes-in-fund-balances` and `balances.html` renders
`fund-balances`, both through `site/history.html.tmpl`: server-rendered tables
in the trends page's mould, no `app.js`, no d3, and therefore no
`tools/jscheck` module owed. The printed block headings arrive as
`export.View.Sections` — the caller's words, validated against the document in
both directions. A chart over these series is a separate, later change.
