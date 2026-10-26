# `fund-flows.json` — the General Fund drill-down

The citywide spine ([`sankey.md`](sankey.md)) prints six fund
groups, ten revenue categories and four object categories on pp.66-67, and
neither a fund nor a department axis. This document reads Budget Book
**pp.127-140** (revenue by fund), **pp.167-170** (General Fund expenditure by
department) and **pp.173-183** (every other fund's expenditure by object). It is
a second document rather than a deeper spine because a headline belongs to a
single-grain file (*No headline*).

The spine remains the only document that publishes the city's printed totals.
The two are reconciled per cell, at zero tolerance, by
`cuts-tie-along-the-lattice`'s `revenue-detail` and `general-fund-departments`
comparisons over the fact store, and the drawn links by
`revenue-lines-tie-to-their-categories`: the links into the funds sum, per
`(kind, category, fund group)`, to the spine's cell, each fund's group read from
`data/funds.yaml`.

## Files

| stem | column | published at |
|---|---|---|
| `fund-flows-2024-actual` | FY2023-24 actual | `fy2024-actual.json` |
| `fund-flows-2025-revised` | FY2024-25 revised | `fy2025-revised.json` |
| `fund-flows` | FY2025-26 adopted | `fy2026-adopted.json` |
| `fund-flows-2027` | FY2026-27 adopted | `fy2027-adopted.json` |

A column is published only when **both** schedules print it; one alone would draw
a revenue side against an empty expenditure side.

## Shape

```
tier 0  revenue/<category>, transfers/in
tier 1  revenue-line/<line>          parent = revenue/<category>
          |   one link per (line, kind) back INTO the category, the sum of
          |   that printed row's own cells
          |   one link per netted (kind, line, fund) cell, and one per
          |   (transfers/in, fund) cell straight from tier 0
tier 2  fund-group/<type>
          |   one link per (fund, kind) INTO the fund, that fund's own inflow
tier 3  fund/<n>                     parent = fund-group/<type>
          |   one link per (fund, division) — the SUM over its object rows
tier 4  dept/<division>              parent = fund/100
          |   one link per netted (department, category) cell
tier 5  expenditure/<division>/<object>   parent = dept/<division>
tier 5  expenditure/fund/<n>/<object>     parent = fund/<n>, one link per
          netted (fund, category) cell of pp.173-183, from the fund itself
```

p172's General Fund block (scope `general-fund-by-category`) is not read: it is
pp.167-170's money a grain coarser, and `structure.ViewOf` refuses the scope set
holding both. `nodes` and `links` are `project.Node` and `project.Link`, the
spine's types.

**A revenue category is reached only through its lines.** Every revenue link
leaves a tier-1 node; the category is emitted because the hierarchy needs it.
`revenue-lines-tie-to-their-categories` holds each line to its category's cells;
nothing holds the folded fact ids, locators or order against the category links
(`fisc-pf7f.3`).

**A line is rolled back up into its category, once per kind.** The `(1,0)` link's
value is the sum of that printed row's own cells and it cites exactly those
cells, not the dashes among them. Per kind, because one printed row reaches the
Internal Service Funds as an internal service charge and the rest of the city as
external revenue — 2 of the 93 lines in both adopted columns.

**A fund is rolled up under its group the same way.** The `(2,3)` link carries
the fund's whole inflow, transfers included, and cites every cell behind it.
Over both adopted columns each group's rollups sum to within the declared
residual of the cell pp.66-67 print for it, and to the cent for the three groups
that carry no residual. The expansion hangs lines and funds by `parent` instead,
so the rollups are drawn by no view.

**A contra row is a negative link on its own line.** pp.127-140 print ERAF and
RPTTF Reduction in parentheses inside the Property Taxes subtotal. The negative
links are two per adopted column, both into `fund/100` under `taxes/property`,
plus `prior-year-unsecured` in FY2023-24 actual, an ordinary row that was
negative that year (`fisc-9psv`).

**A tier-5 id carries its division.** `wages-and-benefits` is spent by nearly
every General Fund division, and `Node.Parent` is one string.

**Six or seven `fund-group/<type>` nodes carry no link**: every `parent` must
resolve to a node of the same document.

**The middle link is emitted, not folded by the client.** Folded up to tier 3, a
tier-4-to-5 link becomes `fund/100 -> fund/100`, and a chart at tiers 0/3/4
would find every department with no inbound link.

## `metadata`

This document lists three `scopes` and carries no `headline`. Three scopes may
share a document because they are disjoint by kind or footprint: pp.127-140
publish revenue and `transfer_in`, pp.167-170 publish expenditure.
`peers-overlap-only-by-declared-identity` fails closed on any undeclared overlap
and holds a pair restating the same money to a named identity: `revenue-by-fund`
and `transfers-by-fund` overlap by $21,045,597 of FY2026 `transfer_in`, under
`a-transfer-in-is-printed-at-both-ends`.

## `counts`

An uncited fact here is a printed zero; neither schedule prints a stock row.
**The uncited facts are exactly the rows that print a dash**, because the cell a
zero is tested at is the printed row. An expenditure fact whose own object cell
nets to zero is still carried by the fund-to-department link that sums the
division.

**`facts_cited_twice` is not zero**: every expenditure fact is behind its object
row and the division total, and every revenue fact behind a flow is behind that
flow, its line's rollup and its fund's rollup. Summing every link's
`value_cents` counts the money more than twice. `counts-reconcile` re-derives
all six counts from the published links.

## No headline

Once one file holds the same money at two grains, "the total" is ambiguous. The
spine's headline is a sum over a named view, `structure.ViewOf`. A revenue-side
drill-down has transfers in and none out, so `transfer_residual_cents` would
publish −21,045,597 and `headline-transfer-residual` would pass it. The headline
checks read `Subject.spine` and the structural checks `Subject.linkedDocuments`,
each a declaration.

## `constraint_tier`

Every tier-3 fund node carries one, and the value is ours: our reading of the
Description of Funds narrative, pp.258-261 (see
[`sankey.md`](sankey.md), `constraint_tier`). The document
carries the disclosure in `metadata.caveats` under
`constraint-tier-is-our-reading`, and `constraint-tier-vocabulary` holds its text
to `project.ConstraintTierCaveat()`.

## The schedule this document shares

`metadata.caveats` carries `the-revenue-schedule-is-published-twice`, which
`revenue-trends` carries too: pp.127-140 are drawn here in one adopted column and
on the Over time page in all four the schedule carries. A row found in both is
one printed figure shown twice.

## Tiers

| tier | nodes |
|---|---|
| 0 | revenue categories, each the target of its own lines' rollups and the source of nothing, plus `transfers/in` |
| 1 | printed revenue rows that were not a printed zero |
| 2 | fund groups, each the source of its own funds' rollups and the target of nothing |
| 3 | funds that took in or spent money this column |
| 4 | divisions — this document's facts hold a division slug in `department`; pp.85-125's hold a department, drawn at `department/<slug>` |
| 5 | object rows that were not a printed zero: a General Fund division's, or another fund's own |

## What this document does not answer

- **Where the General Fund's money goes after the departments.** FY2025-26 takes
  in $157,873,470 and spends $144,650,802 through eleven departments; the
  $13,222,668 difference is transfers out plus the change in working capital,
  printed on pp.66-67 and not carried here. There is no invented sink node — the
  caveat says so.
- **What the other funds spend, by division.** pp.173-183 print them by object,
  not division. pp.85-125's Department Funding Sources is `department-funding`,
  and no split of the expansion reaches it (`fisc-yj4w.34.9`). A fund pp.173-183
  print spending for and pp.127-140 print no revenue for is drawn with nothing
  flowing in.
- **Transfers between funds.** p76's legs are `transfers-by-fund`, which overlaps
  `revenue-by-fund` on `transfer_in`.

## Drawing it

**Unfolded, it draws blank.** Its fund column is 61 nodes. In 796px of usable
height at `NODE_PADDING = 14`, d3-sankey shrinks the padding to
`min(14, 796/60) = 13.267` and leaves nothing for the values: every node height
and link width is 0.0000px. At zero padding 24 of the 61 funds are still
sub-pixel and 45 under 8px, the General Fund being 49% of the column; the
smallest fund reaches one pixel at a canvas 64,203px tall.

So the client folds to a frontier of node ids (the expansion's, or a tier set's):
each node to its nearest ancestor-or-self in the frontier; links with their ends,
merged on the folded pair and `kind`, summing `value_cents` and unioning
`fact_ids` and `locators`; a link whose ends fold together dropped; a node no
folded link touches not drawn; a node with no drawn ancestor stopping the draw.
The locator union matters: on FY2025-26 at `{0,2,4}`, 52 folded rows carry 79
locators, at most 5 a row, where copying the first leg's would print the
document's 18 pages on every row.

No view draws this document. `flows.html` draws the expansion schedule, stitched
per spine column out of this document's lines-into-funds links, its object rows
and the schedules beside it. `fund-flows-2024-actual` and
`fund-flows-2025-revised` have no spine column, pp.66-67 printing no actual or
revised column; they stay in `unviewedDocuments`.

A split's members are capped (`sankey.split_caps`, `capColumn`), and the tail is
`derived: true`: its value is a sum of printed figures, its grouping ours. The
order is fold, cap, fold. `site/fold.test.mjs` holds the cents drawn and the fact
ids cited equal before and after, at every split the page offers in both pinned
columns. The layers and the Go/client seam are in
[`chart-model.md`](../design/chart-model.md).
