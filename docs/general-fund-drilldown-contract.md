# `fund-flows.json` — the General Fund drill-down

The citywide spine (`docs/sankey-contract.md`) answers *how big is the budget and
where does it come from*, at the grain the city prints on pp.66-67: six fund
groups, ten revenue categories, four object categories. It cannot answer *which
fund*, and it cannot answer *which department*, because pp.66-67 publish neither
axis.

This document answers those. It reads Budget Book **pp.127-140** (revenue by
fund), **pp.167-170** (General Fund expenditure by department) and
**pp.173-183** (every other fund's expenditure by object), and it is a
second document rather than a deeper spine because a headline is a property of a
single-grain file — see *No headline*.

**The spine is not retired by this and must not be.** It remains the only
document that publishes the city's printed totals, and the two are reconciled per
cell, at zero tolerance, by `cuts-tie-along-the-lattice`'s `revenue-detail` and
`general-fund-departments` comparisons, which read the fact store and need no
graph.

**The drawn document is reconciled against the spine as well**, by
`revenue-lines-tie-to-their-categories`: the links this file publishes into its
funds sum, per `(kind, category, fund group)`, to the spine's own cell, with each
fund's group read from `data/funds.yaml` rather than from this document's parent
edges. It is the same arithmetic one step later — over links rather than over
facts — and it is the only thing that asserts a line is drawn under the category
`data/taxonomy.yaml` declares it under.

## Files

Four, one per printed column — a flow diagram of two budgets is not a chart of
anything:

Each is the `fund-flows` schedule of its own column, and the column is the file
a reader fetches:

| stem | column | published at |
|---|---|---|
| `fund-flows-2024-actual` | FY2023-24 actual | `fy2024-actual.json` |
| `fund-flows-2025-revised` | FY2024-25 revised | `fy2025-revised.json` |
| `fund-flows` | FY2025-26 adopted | `fy2026-adopted.json` |
| `fund-flows-2027` | FY2026-27 adopted | `fy2027-adopted.json` |

A column is published only when **both** schedules print it. One alone would draw
a whole revenue side against an empty expenditure side, which reads as a city
that stopped spending.

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

p172's General Fund block is scope `general-fund-by-category` and is not read:
it is pp.167-170's money a grain coarser, and `structure.ViewOf` refuses the
scope set that holds both.

`nodes` and `links` are `project.Node` and `project.Link`, byte-identical in
shape to the spine's. **No key is added to either type**, which is why one
`schema_version` still spans every document this project publishes: the envelope
is what the version versions, and this document renames no key and coins no new
one.

What follows about the shape is not obvious and is load-bearing.

**A revenue category is reached only through its lines.** Every revenue link
leaves a tier-1 node, so `revenue/taxes/property` is the source of nothing; it is
emitted because the hierarchy needs it, the way the fund groups are. A client
folding the lines to a coarser tier finds the box to put them in, and that fold
reproduces the category-grain links — same values, same fact ids, same locators,
same order (`TestTheLineTierFoldsToTheCategoryLinks`).

**A line is rolled back up into its category, once per kind.** The rollup is a
`(1,0)` link whose value is the sum of that printed row's own cells and whose
citation is exactly those cells — *not* the dashes among them, which earn no link
on this side of the document and stay in `counts.facts_uncited`. It is published
because a category is the source of every other link it touches, so a chart that
puts it *between* its lines and the funds would otherwise draw it at zero.

**Per kind**, because one printed row reaches the five Internal Service Funds as
an internal service charge and the rest of the city as external revenue — 2 of
the 93 lines in both adopted columns — and one ribbon carrying both would publish
an internal service charge as money crossing the city's boundary.
`checkDistinctLinks` and the client's fold allow two links on one pair exactly
when the kinds differ.

**The rollups are published and drawn nowhere.** The expansion schedule
(`docs/sankey-contract.md`) takes this document's lines-into-funds links and
its object rows, and hangs a line from its category and a fund from its group
by `parent`, so splitting a category or a group in place needs no rollup; the
rollups stay in this file for a reader of it. A tier set that draws them puts
the category between the lines printed under it and the fund groups it
reaches, `{1,0,2}`; every other tier set drops them, each for its own reason:
where tier 1 folds to tier 0 both ends fold to the category and a link whose
ends fold together is dropped, and at `{2,5,4}`, `{2,3,4}` and `{3,4,5}`
neither end has a column.

**A fund is rolled up under its group the same way, once per kind.** The `(2,3)`
link runs from `fund-group/<type>` to `fund/<n>`, carries that fund's whole
inflow — every cell including a transfer's, unlike the `(1,0)` rollup, because a
fund takes its transfers in exactly as it takes its taxes — and cites every cell
behind it. The categories reach the FUNDS directly at `(0,3)`, so without it a
chart putting a fund group between them and its own funds draws the group at
zero. That chart is `{0,2,3}`, and the rollups are its whole right half.
Measured over both adopted
columns, each group's rollups sum to within the declared residual of the cell
pp.66-67 print for it, and to the cent for the three groups that carry no
residual.

**A contra row is a negative link on its own line.** pp.127-140 print ERAF and
RPTTF Reduction in parentheses inside the Property Taxes subtotal, and the
negative rides on the line's own link: any other placement — a reversed
positive link, a sign-decomposed endpoint — folds to a flow the category grain
never had. Measured on the committed store, the negative links are two per
adopted column, both into `fund/100` under `taxes/property`, plus
`prior-year-unsecured` in FY2023-24 actual, which is an ordinary row that was
negative that year rather than a declared contra (`fisc-9psv`).

**A tier-5 id carries its division.** `Node.Parent` is one string and
`wages-and-benefits` is spent by nearly every General Fund division, so a bare
`expenditure/wages-and-benefits` node could not have one parent per division. The prefix stays
`expenditure/` because that is the id form the tier table gives tier 5.

**Six or seven `fund-group/<type>` nodes carry no link.** Every `parent` must
resolve to a node of the same document, so a fund's fund group is emitted even
when nothing flows through it. The spine's rule — nodes exist because links do —
is deliberately broken here: these exist because the hierarchy does.

**The middle link is emitted rather than folded by the client.** The obvious
design is finest-grain links only, and it does not survive a mixed-grain
document: fold a tier-4-to-5 link up to tier 3 and it becomes `fund/100 ->
fund/100`, a self-loop, while a client rendering tiers 0/3/4 would find every
department node with no inbound link and a value of zero. A sankey needs a link
at each adjacent tier pair it can be drawn at.

## `metadata`

The block is every graph document's, in the key order
`TestTheGraphMetadataKeyOrderIsFrozen` pins; this document is the one that
lists three `scopes`, and it carries no `headline`.

Three scopes may sit in one document only because they are **disjoint by kind**
or by footprint:
pp.127-140 publish revenue and `transfer_in`, pp.167-170 publish expenditure, and
every cell key carries kind. `peers-overlap-only-by-declared-identity` asserts
it, fails closed on any undeclared overlap, and holds a pair that restates the
same money to a named identity — `revenue-by-fund` with `transfers-by-fund`
overlap by $21,045,597 of FY2026 `transfer_in`, under
`a-transfer-in-is-printed-at-both-ends`.

## `counts`

The block and its identity, `facts = facts_cited + facts_uncited`, are every
graph document's; what is particular to this one is that `facts_cited_twice`
is not zero. An uncited fact here is a printed zero, since neither schedule
prints a stock row. It is stated as *uncited* rather than *in a zero cell*
because those are not the same set here:
an expenditure fact whose own object cell nets to zero is **still carried** by
the fund-to-department link that sums the division, since that link's value and
its citation are both taken over every cell including the zero ones. Counting
zero cells instead would overstate the gap and the identity would not close.

`facts_cited_twice` is published because **`links` is not a partition of
`facts_cited` in this document**, which the spine's shape would lead a reader to
assume. **Both sides** have a summing link above the cell, and the revenue side
has two: every expenditure fact is behind its own object row and the division
total that includes it, and every revenue fact behind a flow is behind that flow,
its line's rollup into the category and its fund's rollup under the group.
**Summing every link's `value_cents` counts both sides' money more than twice
over.** Fold within one tier pair; never across the whole graph. The count is of
FACTS behind more than one link, so a row behind three counts once.

The document's own `counts` carry the figures, and `counts-reconcile`
re-derives all six from the published links.

**The uncited facts are exactly the rows that print a dash**, because the cell a
zero is tested at is the printed ROW: revenue and transfers-in rows of
pp.127-140, and expenditure rows of pp.173-183. Drawn at category grain the two
sets come apart: a row printing a dash can sit inside a category cell that is
not zero, and a category's link would cite it while no line's link does. Neither reading moves any money — a dash adds nothing to a
sum — so the difference is a citation and not a figure.

## No headline

A headline is a property of a **single-grain** document. The moment one file
holds the same money at two grains, "the total" is ambiguous, and no key
disambiguates it because the ambiguity is in the accumulation rather than in the
key. So the spine's headline is not accumulated as its cells are drawn: it is a
sum over a named view of the structure, `structure.ViewOf`, which refuses a
scope set one of whose cuts decomposes another over money both print before
anything is summed.

Giving this document one is not the cheap way out either. A revenue-side
drill-down has transfers **in** and no transfers out, so
`transfer_residual_cents` would publish −21,045,597 and
`headline-transfer-residual` would report it **green** — each transfer figure
being the sum of its links, and the links being one leg. That is a published number that is
arithmetically correct and means nothing.

So the three headline checks read `Subject.spine`, the documents the site
publishes as the spine, and the six structural checks read `Subject.linkedDocuments`,
every document made of nodes and links. The predicate is a declaration, never a
test of whether a document carries a headline block or whether the figures in
one happen to be non-zero.

## `constraint_tier`

Every tier-3 fund node carries one, here and in every other document that
draws a fund, and the value is **ours**: our reading of the Description of
Funds narrative, pp.258-261. `node.derived` stays `false` — the city prints the
fund — and the disclosure rides on `source_note` and `rationale` beside it. The
document also carries the disclosure sentence in
`metadata.caveats` under the id `constraint-tier-is-our-reading`, and
`constraint-tier-vocabulary` finds it by that id and then compares its text
against `project.ConstraintTierCaveat()` rather than against prose written
twice. Both halves are asserted: an id present with drifted text is a finding of
its own, because an anchor that still resolves over a weakened sentence is the
quieter failure.

See `docs/sankey-contract.md`'s `constraint_tier` section for the argument, and
for why `""` and `unknown` are different claims.

## The schedule this document shares

`metadata.caveats` also carries `the-revenue-schedule-is-published-twice`, which
`revenue-trends` carries too. Budget Book pp.127-140 are drawn in two places on
the site: this document holds one adopted column of their rows, and the Over
time page prints its revenue lines in all four the schedule carries — FY2023-24
actual, FY2024-25 revised and both adopted years. A row found in both is one printed figure shown once in
each, so neither view is a second measurement of it. One sentence serves both
documents, so a reader cannot find the two pages disagreeing about it.

## Tiers

| tier | nodes |
|---|---|
| 0 | revenue categories, each the **target** of its own lines' rollups and the source of nothing, plus `transfers/in`, which is the source of a flow into every fund it reaches |
| 1 | printed revenue rows that were not a printed zero |
| 2 | fund groups, each the **source** of its own funds' rollups and the target of nothing |
| 3 | funds that took in or spent money this column |
| 4 | divisions, not departments — a fact of THIS document's scopes holds a division slug in its `department` field; pp.85-125's funding rows hold a department in the same field, which is why `department-funding` draws them at `department/<slug>` and not at `dept/<slug>` |
| 5 | object rows that were not a printed zero: a General Fund division's, or another fund's own |

Tier 1 holds fewer rows than `data/taxonomy.yaml` declares, because a row that
prints a dash in every fund of the column earns no link; tier 3 holds fewer funds
than the pages print, for that rule again. Tier 4 holds divisions and not the
departments pp.167-170 total them under, because the divisions are what the facts
carry.

## What this document does not answer

- **Where the General Fund's money goes after the departments.** It does not
  balance: FY2025-26 takes in $157,873,470 and spends $144,650,802 through
  eleven departments, and the $13,222,668 difference is transfers out plus the
  change in working capital, printed on pp.66-67 and not carried here. There is
  **no invented sink node** — the caveat says so instead.
- **What the funds other than the General Fund spend, by division.** pp.167-170
  decompose that fund alone; pp.173-183 print every other fund by object and
  not by division, and this document draws them straight from the fund to its
  categories. pp.85-125's Department Funding Sources — which departments each
  fund pays for — is published as `department-funding`, and no split of the
  expansion reaches it (`fisc-yj4w.34.9`).

  A fund pp.173-183 print spending for and pp.127-140 print no revenue for is
  drawn with nothing flowing into it. Which funds those are differs by
  column.
- **Transfers between funds.** p76's legs are scope `transfers-by-fund`, which
  this document does not select because the two overlap `revenue-by-fund` on
  `transfer_in`. They have a document of their own, `transfers-by-fund`, whose
  payers the expansion splits Transfers In into.

## Drawing it: the fold

**This document cannot be drawn as it stands, and that is arithmetic rather
than an aesthetic judgement.** Its fund column is 61 nodes. `site/app.js` lays a
Sankey out in 796px of usable height at `NODE_PADDING = 14`; d3-sankey shrinks
the padding to fit — `min(14, 796/60) = 13.267` — and then divides what is left
among the values, and what is left is nothing. Every node height and every link
width comes out at exactly **0.0000px**. A view added without the fold publishes
a blank chart with every check in this repository green.

Height does not fix it. At **zero** padding 24 of the 61 funds are still
sub-pixel and 45 are under 8px, because the General Fund alone is 49% of the
column; the smallest fund reaches one pixel at a canvas 64,203px tall.

So the client folds. The rule, in full:

- **The fold is to a frontier of node ids**: the expansion's, or a tier set's
  (`tierFrontier`, every node at one of its tiers). A tier set applied to a
  document it does not describe **refuses** rather than corrupts — `{0,2,4}`
  over the spine throws, because every spine node is parentless and a tier-5
  node has no drawn ancestor to fold to.
- **Each node folds to its nearest ancestor-or-self in the frontier**,
  following `parent`.
- **Links fold with their ends** and merge on the folded pair and the `kind`,
  summing `value_cents` and unioning both `fact_ids` and `locators`. One
  ribbon per kind, so a ribbon's kind is true of all of it: no fold through
  `parent` puts two kinds on one pair in any published column, but a capped
  tail can, where its members take one source's money as different kinds:
  the internal-service funds take a line's money as an internal service
  charge.

  The locator union is not decoration. `buildTable` and `pin` render the
  **folded** document, and the fold builds a merged ribbon by copying its first
  leg — so without an explicit union a ribbon would cite a strict subset of the
  pages its figure was read from, and a reader clicking through would land on a
  shard holding part of the number they were shown. It de-duplicates on
  `(doc_id, page)`, which is something the fact-id union cannot express: two
  facts on one page are two ids and one locator. Measured on FY2025-26 at the
  `{0,2,4}` set: 52 folded rows carried 79 locators, at most
  5 on any one row.

  This is what makes the flow table's `Source` column true. Without it, at that
  set, every row printed the document's own 18 pages — 1,872 anchors saying
  nothing about the row they sat in; with it, 237, each naming the pages that
  row's figure came from.
- **A link whose ends fold to the same node is dropped.** It was a flow inside
  what is now one box. This is the tier-4-to-5 case warned about above, and it
  **cites nothing away**: the fund-to-department link that survives carries the
  same money and the same facts, over every cell including the printed zeros,
  which is what `facts_cited_twice` counts. The `(1,0)` rollups are the same
  case on the revenue side — both ends fold to the category — and the line's own
  flows into the funds carry every fact they cited; `site/fold.test.mjs` holds
  the citations equal before and after the fold at every split the page
  offers.
- **A retained node's `parent` is re-pointed at its own folded ancestor**, so
  the folded document satisfies client-side what `node-hierarchy-well-formed`
  asserts of the published one.
- **A node no folded link touches is not drawn.** A zero-degree node gets depth
  0 and value 0 from d3-sankey, which draws as a labelled rectangle of no height
  in the first column.
- **A node with no drawn ancestor stops the draw.** The tier set does not
  describe the document, and both ways of carrying on are worse: dropping it
  loses a column silently, keeping it leaves a node with no column to be drawn
  in.

### The page draws it through the expansion

No view draws this document. `index.html` draws the expansion schedule, which
is stitched per spine column out of this document's lines-into-funds links,
its object rows and the schedules beside it (`docs/sankey-contract.md`), and
a reader reaches this document's grain by splitting nodes in place: a revenue
category into its lines, a fund group into its funds, the General Fund into
its divisions, an object category into its object rows or the divisions that
spend it. `fund-flows-2024-actual` and `fund-flows-2025-revised` have no spine
column to be stitched into, because pp.66-67 print no actual and no revised
column; they stay declared in `unviewedDocuments`.

**A split's members are capped, and its tail is ours.** `sankey.split_caps`
declares, per split key, how many members are drawn before the rest fold into
one tail inside the node they split; `capColumn` decides which, by value. The
tail is `derived: true`, with a rationale and a source note: its *value* is
every cent a printed figure, summed exactly as the fold sums a merged ribbon;
what is inferred is the *grouping*. A reader draws a tail out at full length.

**The order is fold, cap, fold, and it is not interchangeable.** The first
fold draws the frontier, so the cap ranks a split's members by what they carry
on the chart drawn. The cap produces several ribbons from one source to the
tail, and the second fold is what merges them, summing the values and unioning
the fact ids and locators.

**The page describes the folded document, not the fetched one.** The legend, the
flow table, the inferred list and the flow count are all statements about what
the reader is looking at. The footer still links the unfolded file, and the
merged links still name every fact behind every ribbon.

## Go's half: the declarations

**Go ships what only Go can know, and computes no frontier.** The expansion
schedule carries every node the stitch can draw, each split as `node`, `key`,
`form` and `alternative`, and the derived remainders that tie a split to its
parent, every figure cited. The page's chart declares its **form** with that
form's hints under its own key, which is what may fold: the Sankey's `tiers`,
`grow` and `split_caps`. The schema refuses a hint foreign to the declared
form; `validateGrowth` and `validateSplitCaps` refuse a declaration the form
could not honour. The layers and the seams are in
[`chart-model.md`](chart-model.md).

**The client computes everything a frontier draws from that and the column
document.** The core sums: the frontier a list of splits comes to (`expand`),
which nodes a parent chain holds, the fold, the cap, the marks and their
figures. The Sankey renderer fits: which open splits a cap folds, the slot
each drawn node stands in, and how tall the canvas grows.

**What licenses a client-side sum** is that every summand is a published
figure: each unfolded ribbon's `value_cents` is in the column document the page
has already fetched, with its own `fact_ids`, which `link-values-tie-to-facts`
witnesses. A merged ribbon and a tail are sums over those and nothing else.

**What holds the fold** is the property tests in `site/fold.test.mjs`, over
every split the page offers in both pinned columns, each with a mutation that
reddens it: the cents drawn and the fact ids cited are the same before and
after the cap and the fold, so no ribbon is lost or drawn twice; and an open
split a cap names holds at most `cap + 1` marks, its tail's members and the
kept marks together being the whole split.

So, in one line: **Go declares what may fold and the client decides how much
is drawn.** `split_caps` is a declaration; `capColumn` and `foldDocument` are
what spend it.
