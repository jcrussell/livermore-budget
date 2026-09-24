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

`<output>/data/<stem>.json`, beside `data/revenue-trends.json`. These three are
the documents that state no fiscal year or basis, so they fold into no column
and are published under names of their own. Each stem is its projection's
`Name()` verbatim, for the trends contract's stated reason: a
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

  The **pages do not print that word.** `basis` is component 7 of the fact id,
  so these figures cannot be re-based without rewriting every one of their ids
  and moving `facts/facts.jsonl`; but both schedules sit in the section p161
  heads "Statistical Section (Unaudited)", which each document says in its own
  `statistical-section-unaudited` caveat. `internal/export` reads that caveat
  and labels the column chips and cell tooltips `unaudited` instead. The
  document's basis and the page's label are two different claims and this is the
  one place they diverge.
- **`fund` is `null` on every series and `fund_name` is `""`.** These
  schedules' rows are a fund's components or an aggregate across funds, never a
  numbered fund, and no fund is numbered 0 -- `null` is the store's own spelling
  of an absent fund, carried through from `fact.Fact.Fund`. The tables render no
  Fund column; the printed block headings carry the identity instead.
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
  total. This is the schedules' open exposure, carried here from the
  declarations that used to sit in `internal/check`.

## p41, the General Fund summary, and why no document draws it

ACFR MD&A p41, the General Fund's condensed Statement of Revenues,
Expenditures and Changes in Fund Balances, is mapped at scope
`acfr-general-fund-summary`: 20 FY2024-25 figures at the millions grain, where
the pp.167-169 schedules print dollars. It is a cut of the hierarchy
(`structure.ACFRCuts`, at the spine's own grain) and no published document
selects it. This section carries the declaration that used to say so in
`internal/check`, with the argument for it; a check that a cut is drawn by
some document or declared undrawn is `fisc-a5ii`.

**What the page prints.** Four blocks, three of them mapped whole: ten revenue
rows tying to the printed Total Revenues of 157.20 exactly; two transfer legs,
0.53 in and (25.72) out, tying to the printed Total Other Financing Sources
(Uses) of (25.19) exactly; and the three fund-balance lines that close the
statement, beginning 92.10, change (5.00), ending 87.10. The fourth, the
expenditure block, is mapped in part: the five divisions of the General
Government function and none of its other rows. All in millions to two
decimals, so the least significant printed digit is $10,000.

**It is undrawn because it is a different year on a different basis**, not
because it restates money some other scope already publishes. The Budget Book
spine is FY2026 and FY2027 adopted; this is FY2025 audited, and the spine
prints no audited column. That is now a refusal by declaration rather than a
measurement: the two cuts sit at one level and share no basis, so
`peers-overlap-only-by-declared-identity` refuses the pair by name on every
run, and a view holding both would be refused by `structure.NewView`. The
history pages draw ten years of the two statistical-section scopes -- a
section the ACFR itself labels (Unaudited) -- and this page can join neither
table: it is the General Fund alone where pp.168-169 combine all governmental
funds, and its ending balance does not tie to p167's components (see the
section below).

**What holds these facts.** `fund-balance-identity` reaches three of the 20 --
beginning + change == ending, the same identity it asserts over the spine's
twelve cells -- plus `CheckTotals` at build time on the other 17. The General
Government block is the one place in this corpus that ties only within a
tolerance, and the build report says so on every run rather than counting it
as a clean tie: its five divisions print 18.44 against a printed subtotal of
18.45, one printed unit over five terms, so the bound derived from the page is
half a unit per row -- $25,000, which is what this column's $10,000 is
admitted by and is also the most any column of this rule could be out by and
still tie (`fisc-1wr.2`). It is also the only rule reading a total the
document prints above its own rows, which is bounded by pinning that total to
the section anchor's own line (`fisc-h96o`).

**The remaining exposure is stated rather than absorbed.** This page declares
no `column_headers`, because its printed headers are the bare years 2025 and
2024 and the parser refuses a header `amount.Parse` accepts, and because the
geometry line pairing fails on the page anyway (a stray "0.0" with no row
label sits inside the line tolerance of the row above, giving 51 geometry
lines against 52 text lines). So no geometry column guard stands over any of
these figures -- the widest tolerance in the corpus over the fewest guards,
which is why the bound is half a unit per row and not the worst case that
would also add the printed total's own half unit.
`internal/mapping`'s `TestACFRp0041HasNoGeometryColumnGuard` measures both
halves.

**What is still not mapped** is the rest of the expenditure block: six of its
nine other top-level rows are the ACFR's remaining functions -- Fire, Police,
Public Works, Community Development, Economic Development, Library -- and
unlike General Government's five divisions they are departments
`fact-departments-resolve` cannot accept (`fisc-xudn`). The other three,
Capital Outlay, Principal and Interest and fiscal charges, are not departments
at all and are blocked by nothing; they are simply unmapped. No row of the
mapped block carries a department either, for a reason of its own -- two of
the five name departments covering several divisions -- see
`data/taxonomy.yaml`'s general-government entry. The FY2024 column of every
block is present and skipped: three of the four printed blocks miss in it --
revenue by $200,000, expenditure by $100,000 and the fund balances by
$100,000, twenty, ten and ten printed units, with only Other Financing Sources
tying. The General Government sub-block misses by $170,000 on top of that,
seventeen units, which is why the tolerance that admits its $10,000 in FY2025
comes nowhere near admitting FY2024. The four blocks are the ones the page
prints, and General Government is inside one of them.

## Cross-page ties measured and found dead

Recorded so nobody re-plans them: p167's year-over-year total deltas match
p169's Net change in only 3 of the 9 comparable years -- 2017, 2020 and 2022,
the last of them a column p169 copies into 2023; and p41 — MD&A's condensed
statement, printed in millions — gives an ending General Fund balance of $87.10
where p167's 2025 components sum to $87,043,576, a figure the audited statement
(p54) prints to the dollar (`fisc-y242`). The p41 disagreement ships as a caveat
on the `fund-balances` document. The two ties that do hold in the Statistical Section
(p187↔p189, p193↔p192) are between pages this lane does not publish; see
`docs/acfr-statistical-section.md`.

## The reader surface

`history.html` renders `changes-in-fund-balances` and `balances.html` renders
`fund-balances`, both through `site/history.html.tmpl`: server-rendered tables
in the trends page's mould, no `app.js`, no d3, and therefore no
client test owed. The printed block headings arrive as
`export.View.Sections` — the caller's words, validated against the document in
both directions. A chart over these series is a separate, later change.
