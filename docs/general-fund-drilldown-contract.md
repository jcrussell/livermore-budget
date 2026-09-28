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

**The rollup is drawn in a category's own window and nowhere else.** That
window is the chart it was published for: `{1,0,2}` puts the category between the
lines printed under it and the fund groups it reaches, and the rollups are its
whole left half. Every other tier set drops them, each for its own reason: where
tier 1 folds to tier 0 both ends fold to the category and a link whose ends fold
together is dropped, and at `{2,5,4}`, `{2,3,4}` and `{3,4,5}` neither end has a
column. The client's tests assert both halves over every view the page opens.

**A fund is rolled up under its group the same way, once per kind.** The `(2,3)`
link runs from `fund-group/<type>` to `fund/<n>`, carries that fund's whole
inflow — every cell including a transfer's, unlike the `(1,0)` rollup, because a
fund takes its transfers in exactly as it takes its taxes — and cites every cell
behind it. The categories reach the FUNDS directly at `(0,3)`, so without it a
chart putting a fund group between them and its own funds draws the group at
zero. That chart is the `{0,2,3}` window the spine's middle column
opens into, and the rollups are its whole right half. Measured over both adopted
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
`wages-and-benefits` is spent by 22 divisions, so a bare
`expenditure/wages-and-benefits` node could not have 22 parents. The prefix stays
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

FY2025-26: `280 = 233 + 47`, with 228 cited twice, over 238 nodes and 412 links.
`counts-reconcile` re-derives all six from the published links. The
228 is 44 expenditure rows plus the 184 revenue rows that earned a flow; the 47
that print a dash are in neither.

**The 47 uncited facts are the 47 revenue rows that print a dash**, exactly,
because the cell a zero is tested at is the printed ROW. Drawn at category grain
the two sets come apart: measured on this column, six rows printing a dash sit
inside a category cell that is not zero, and a category's link cites them while
no line's link does. Neither reading moves any money — a dash adds nothing to a
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
`headline-transfer-residual` would report it **green** — the figure being the sum
of the facts, and the facts being one leg. That is a published number that is
arithmetically correct and means nothing.

So the three headline checks read `Subject.Graphs`, the documents whose TYPE
publishes a headline, and the six structural checks read `Subject.LinkedDocuments`,
every document made of nodes and links. The predicate is structural, never a test
of whether the figures in a headline happen to be non-zero.

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
the site: this document holds one adopted column of their rows, and the Revenue
tables print all four the schedule carries — FY2023-24 actual, FY2024-25 revised
and both adopted years. A row found in both is one printed figure shown once in
each, so neither view is a second measurement of it. One sentence serves both
documents, so a reader cannot find the two pages disagreeing about it.

## Tiers

| tier | nodes, FY2025-26 | of 238 |
|---|---|---|
| 0 | 10 revenue categories, each the **target** of its own lines' rollups and the source of nothing, plus `transfers/in`, which is the source of a flow into every fund it reaches | 11 |
| 1 | printed revenue rows that were not a printed zero | 93 |
| 2 | fund groups, each the **source** of its own funds' rollups and the target of nothing | 6 |
| 3 | funds that took in money this column | 61 |
| 4 | divisions, not departments — a fact of THIS document's scopes holds a division slug in its `department` field; pp.85-125's funding rows hold a department in the same field, which is why `department-funding` draws them at `department/<slug>` and not at `dept/<slug>` | 23 |
| 5 | division x object cells that were not a printed zero | 44 |

The tier-1 count is 93 and not the 101 rows `data/taxonomy.yaml` declares,
because a row that prints a dash in every fund of this column earns no link — the
same rule as tier 3's. The tier-3 count is 61 and not the 70 funds pp.131-140
print, for that rule again. The tier-4 count is 23 and not 11: pp.167-170 print
eleven DEPARTMENT totals over 23 divisions, and the divisions are what the facts
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
  fund pays for — is drawn by `department-funding`, which is what the fund
  column of this document opens into for every fund but 100.

  A fund pp.173-183 print spending for and pp.127-140 print no revenue for is
  drawn with nothing flowing into it, and its window cannot keep a flank, so
  it opens into nothing. Which funds those are differs by column; each rung's
  answer names them under `needs`.
- **Transfers between funds.** p76's legs are scope `transfers-by-fund`, which
  this document does not select because the two overlap `revenue-by-fund` on
  `transfer_in`. They have a document of their own, `transfers-by-fund`, which
  the spine's Transfers In opens into.

## Drawing it: the fold

**This document cannot be drawn as it stands, and that is arithmetic rather
than an aesthetic judgement.** Its fund column is 61 nodes. `site/app.js` lays a
Sankey out in 796px of usable height at `NODE_PADDING = 14`; d3-sankey shrinks
the padding to fit — `min(14, 796/60) = 13.267` — and then divides what is left
among the values, and what is left is nothing. Every node height and every link
width comes out at exactly **0.0000px**. A view added without the fold publishes
a blank chart with every check in this repository green, which is why the
document shipped as data for four days with `unviewedDocuments` declaring in
writing that no page rendered it.

Height does not fix it. At **zero** padding 24 of the 61 funds are still
sub-pixel and 45 are under 8px, because the General Fund alone is 49% of the
column; the smallest fund reaches one pixel at a canvas 64,203px tall.

So the client folds. The rule, in full:

- **A page declares the tiers it draws**, as `render_tiers` in `FISC_CONFIG`.
  It is per view and never a constant in `app.js`: the spine and this document
  are drawn by the same script from different hierarchies. Applying one page's
  set to the other document **refuses** rather than corrupts — `{0,2,4}` over
  the spine throws, because every spine node is parentless and a tier-5 node has
  no drawn ancestor to fold to — which is the better of the two failures and
  still a broken page. A page that declares nothing is drawn whole, by exactly the code that
  drew it before the fold existed.
- **Each node folds to its nearest ancestor whose tier the page draws**,
  following `parent`.
- **Links fold with their ends** and merge on the folded pair and the `kind`,
  summing `value_cents` and unioning both `fact_ids` and `locators`. One
  ribbon per kind, so a ribbon's kind is true of all of it: no fold through
  `parent` puts two kinds on one pair in any published column, but a revenue
  category's capped fund tail does, because its funds span every group and
  the internal-service funds take a line's money as an internal service
  charge.

  The locator union is not decoration. `buildTable` and `pin` render the
  **folded** document, and the fold builds a merged ribbon by copying its first
  leg — so without an explicit union a ribbon would cite a strict subset of the
  pages its figure was read from, and a reader clicking through would land on a
  shard holding part of the number they were shown. It de-duplicates on
  `(doc_id, page)`, which is something the fact-id union cannot express: two
  facts on one page are two ids and one locator. Measured on FY2025-26 at the
  `{0,2,4}` set the page then drew: 52 folded rows carried 79 locators, at most
  5 on any one row.

  This is what makes the flow table's `Source` column true. Without it, at that
  set, every row printed the document's own 18 pages — 1,872 anchors saying
  nothing about the row they sat in; with it, 237, each naming the pages that
  row's figure came from. The chain's rungs fold differently and the client's
  tests pin their own row counts.
- **A link whose ends fold to the same node is dropped.** It was a flow inside
  what is now one box. This is the tier-4-to-5 case warned about above, and it
  **cites nothing away**: the fund-to-department link that survives carries the
  same money and the same facts, over every cell including the printed zeros,
  which is what `facts_cited_twice` counts. The `(1,0)` rollups are the same
  case on the revenue side — both ends fold to the category — and the line's own
  flows into the funds carry every fact they cited. Measured on FY2025-26: 233
  facts cited by 412 links before the fold, 233 by 52 after.
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

### One page draws it: the spine opens into it

No view draws this document whole. It is drawn as rungs opened from
`index.html`, the site's one chart page, declared once in `views()`; the
client's tests spell none of it a second time.

Every rung of that chain is a WINDOW: the node the reader clicked in the middle,
one flank kept off the chart they clicked it on, and its own decomposition on
the other side. **Three columns is the narrowest such window** — the client's column budget
starts at 3, and a window at least `chartWidth(4)` plus the stylesheet's cushion
wide buys a fourth. The tier sets in the table below are what a narrow reader
sees.

A step may declare more columns than that budget draws: `widen` names the
columns of its own `tiers` a narrower client does without, and it drops them
from the end of `widen`; they sit at the end away from the kept flank. The fund-group step
widens `{0,2,3,4,5}` by `{4,5}` and the fund step `{2,3,4,5}` by `{5}`, so their
rows below state both. A widened
column the document leaves empty is dropped and the chart re-laid at the columns
it has, rather than refused: five of the six fund groups have no tier-4 node,
and a wide screen must not show a reader less than a narrow one.

| depth | document | draws | opening a node draws | caps |
|---|---|---|---|---|
| 0 | `sankey` | the spine, whole | a fund group (tier 2); or a revenue category (tier 0); or an object category (tier 5) | — |
| 1 | `fund-flows` | `{0,2,3}` keeping tier 0 of the chart above: the spine's own revenue categories on the left, the opened group in the middle, its funds on the right, with the money pp.127-140 split by no fund carried past the centre onto one derived mark beside them — widening to `{0,2,3,4,5}` where there is room: the General Fund's divisions, and the object categories every fund spends on, through its divisions for the General Fund and straight from pp.173-183 for every other | a fund (tier 3) — the General Fund into its divisions, every other fund pp.85-125 name into its departments | tier 3 at 8; tier 4 at 24 (divisions); tier 5 at 8 (object rows) |
| 2 | `fund-flows` | `{2,3,4}` keeping tier 2: the group, the opened fund, the divisions that spend it — widening to `{2,3,4,5}`, their object-category cells, where there is room for a fourth column | a division (tier 4) | tier 4 at 24; tier 5 at 8 (object rows) |
| 3 | `fund-flows` | `{3,4,5}` keeping tier 3: the fund, the opened division, its object categories | nothing | tier 5 at 8 |
| 2 | `department-funding` | `{2,3,4}` keeping tier 2: the group, the opened fund, the departments pp.85-125 print it paying for | nothing | — |
| 1 | `fund-flows` | `{1,0,2}` keeping tier 2: the lines pp.127-140 print under the opened category on the left, the category itself in the middle, and the spine's own fund groups for it on the right; a line printed as a reduction draws as a contra ribbon at its magnitude, into the centre | nothing | tier 1 at 8 (lines) |
| 1 | `department-spending` | `{2,5,4}` keeping tier 2: the fund groups that fund the opened object category, the category, and the divisions pp.85-125 print spending it | nothing | tier 4 at 8 (divisions) |
| 1 | `transfers-by-fund` | `{2,3}`, no flank: the funds p76 prints paying each transfer on the left, the funds receiving them on the right | nothing | — |
| 1 | `transfers-out` | `{3,5}`, no flank: the funds p76 and p222 print paying each transfer out on the left, the operating and CIP funds receiving them on the right | nothing | tier 3 at 10; tier 5 at 10 (funds) |

The steps are a tree and not a chain: several open from the spine's chart, told
apart by the tier they open from and, where two share a tier, by the node's role.
The client walks it by key (`after` names the step whose chart a step opens
from) and never by depth.

**Nothing on a kept flank opens.** A window draws its kept column at that
column's share of the CENTRE — the spine's cell for one revenue category into a
fund group, not the group's own inflow — so a step opening one of those marks
would draw a node taking one figure in and sending its whole decomposition out.
Measured over both adopted columns with the revenue-category window declared
among the fund-group step's parents: the Contributions & Outsourced window
keeps `fund-group/general` at 76,360 and opening it drew 157,873,470 leaving —
157,797,110 of node height with no ribbon under it. `export.validateSteps`
refuses that declaration by name.

**Only the General Fund opens into DIVISIONS, and every other fund a department
draws on opens into DEPARTMENTS.** pp.167-170 decompose that fund alone, so the
fund step declares `role: "general_fund"`; `fund-departments` is its sibling,
declared with `role: "fund"` on the same `(after, from)` — the one place on the
site where two steps open one tier of one chart, which `validateSteps` admits
exactly when both name a role and the roles differ.

**A role cannot say which funds pp.85-125 name, so the packager reads it off the
document.** Six of the 61 funds this document draws in FY2025-26 and seven of
the 60 in FY2026-27 are named by no row of those pages, and nothing about
`fund/511` distinguishes it from `fund/512`; the set is different in every
printed column — 13 funds in FY2023-24 against 6 in FY2025-26 — while a step is
declared once for every year the view lists. So `site/app.js`'s `decomposable`
reads, per year's document and per step, the node ids at its `from` that the
document decomposes at the tiers the step declares, with the same reach the
chart is drawn with (`reaching`); `stepFor` takes that as a fourth match beside
the key, the tier and the role, and a window step further asks that the chart on
screen send a kept flank into the node. Nothing ships the set: what is offered
to open and what draws are one rule in one language. Measured without the
fourth match: `drillDown(fund/511)` failed and left the chart on
`fund-group/capital`, over a mark drawn with the open affordance.

**The step document is the spine year's, joined on column.** A spine stem
opens into the `fund-flows` stem carrying the same fiscal year on the same
basis: `sankey` into `fund-flows`, `sankey-2027` into `fund-flows-2027`. The
join is on `Columns`, not on the order `project.PublishedDocuments` declares,
and the packager resolves it per year into the page's config, beside that document's own caveat
links, so the client joins nothing. `fund-flows-2024-actual` and
`fund-flows-2025-revised` have no spine year to be opened from, because
pp.66-67 print no actual and no revised column; they stay declared in
`unviewedDocuments`.

**A rung places the document only because it filtered first**, and that is a
refusal rather than a preference. This document carries eleven tier-0 revenue
nodes with no ancestor at tier 3 or 4, and the fold refuses a node it cannot
place — so `{4,5}` over the whole document draws *nothing*, not a partial
chart. Filtering to the opened node is what leaves a set the fold can place.

**A rung cites a slice, and says so — and a window cites two documents, which
the sentence keeps apart.** At `{0,2,4}` the fold cites nothing away; a rung
filtered to one node cannot, and a window also draws a flank of the chart it was
opened from, whose ribbons cite that document and not this one. So the counts
line partitions the ribbons before it gives either fact count. The drill tests
under `site/` walk every node the page offers at every width and hold each
opened view's drawn nodes and ribbons to the properties below.

### Opening a node: filter, cap, fold

**Expanding a node in place is not a per-node interaction on this d3-sankey**
(`fisc-ppkq`). The vendored build takes the column count from topology and
clamps the align into it, so expanding one group draws the funds and the
divisions in the same column while the unexpanded ribbons span two — which
the client's layout test refuses. The shape that works is
filtering to one node and rescaling to its own total.

**Rescaling alone is not enough, and `fisc-ppkq` says it is.** Measured:
filtered to special revenue and rescaled to that group's own total, 22 of its 49
ribbons still lay out under one pixel, because the concentration is *within* the
group — `fund/200` alone is 34.9% of it and the smallest two are 0.034%.
Rescaling cannot fix a distribution.

**So a drill also caps its fine column.** Above the step's `TierCap.Cap` for
that tier, the tail by value folds into one aggregate. At cap 8 special revenue
draws 2 sub-pixel ribbons instead of 22, and capital 1 instead of 4. The cap is
inert at depth 3, where the widest division spends on two object categories.

**The aggregate node is `derived: true`**, with a rationale and a source note.
Its *value* is every cent a printed figure, summed exactly as the fold sums a
merged ribbon; what is inferred is the *grouping*, and the published-is-not-
derived rule is about which of those the page claims. It shipped for one commit
as `derived: false` — drawn solid, chipped "printed by the city", and absent
from "what we inferred".

**The order is filter, cap, fold, and it is not interchangeable.** Filter first,
because the cap ranks a column by size and the sizes that matter are the ones
inside the node being opened. Cap before fold, because the cap produces several
ribbons from one source to the aggregate and the fold is what merges them,
summing the values and unioning the fact ids and locators.

The client's tests re-measure all of this on every `make js`, opening
every node the chain offers — 6 fund groups and, under the General Fund, 23
divisions — rather than a sample.

**Tier 5 is not a one-constant alternative.** Drawing `{0,2,4,5}` puts 29 of the
44 object nodes under one pixel (smallest 0.030px), and that column's labels are
23× "Services & Supplies" and 21× "Wages & Benefits". The object grain is not
*hidden* by the fold; it is unrenderable at this canvas, and offering it whole
needs a view that rescales to one division, which the division step is; the fund
step's fourth column shows it only capped. Measured over all 23 divisions, the smallest ribbon in any
opened view is 51.38px, at Patrol; the drill tests under `site/` print the
smallest ribbon of every view they open.

**What the fold does not fix.** At the `{0,2,4}` set, seven of the 52 ribbons
laid out under 1px and four of the 40 node rects under 2px, and `render()` floors
both — `Math.max(1, width - RIBBON_GAP)` and `Math.max(2, y1 - y0)` — so those
marks do not encode their values. The client's tests pin **both** counts,
so neither can grow unnoticed. The chain's rungs are better on this and not
free of it, and neither count is the same in both years: the General Fund at
depth 1 draws 2 sub-pixel ribbons of 37 in FY2025-26 and 3 of 37 in FY2026-27,
and special revenue, capped, 2 of 22 and then 1 of 22. The fold tests under
`site/` print both counts per column.

**The page describes the folded document, not the fetched one.** The legend, the
flow table, the inferred list and the flow count are all statements about what
the reader is looking at; pointing them at the file would put a 175-row table
beside a 52-ribbon chart. The footer still links the unfolded file, and the
merged links still name every fact behind every ribbon.

## Go's half: the declarations

**Go ships what only Go can know, and computes no rung.** A step is declared
once in `views()` and reaches the page as `FISC_CONFIG.steps`: its `tiers` in
draw order, the `keep` flank and `from` centre, the `role` that opens it, the
`caps` a column may fold under, the `widen` a wider viewport buys, the
`residual` endpoints with the reason each is carried and the `residual_grain`
its mark is named by, and the `gaps` licences — the cents per column, from
`structure.BudgetBookExceptions`, that two schedules are declared to differ by
at one node. `validateSteps` refuses a declaration whose shape the client could
not draw, and `capMergesNoPrintedWithInferred` a cap under which the client's
fold would merge a printed ribbon with an inferred one. The shape is
[`schema/page.schema.json`](../schema/page.schema.json), compared against the
emitted bytes at the write.

**The client computes everything a rung draws from that and the column
document**: which ribbons a rung holds (`reaching`: the near end inside the
opened node by parent chain, both ends placeable, folded ends running forward
in the step's column order), which nodes it offers to open (`decomposable`,
the same reach), the roles of its columns (`flankIsLeft`, by position), the
figure a centre prints net of reductions (`markAmounts`), the residual's
ribbons, ends, tier and figures (`carryResidual`), and the gap, held to the
shipped licence and refused — banner, chart unchanged — where the two
documents differ by any other figure (`markGap`). A residual's leaving legs
exist by the step's declared tiers, not by the columns a budget draws, so its
figures do not move with the viewport.

**What licenses a client-side sum** is that every summand is a published
figure: each unfolded ribbon's `value_cents` is in the column document the page
has already fetched, with its own `fact_ids`, which `link-values-tie-to-facts`
witnesses. A merged ribbon, a tail, a residual's two figures and a gap's
difference are sums and differences over those and nothing else.

**What holds the fold** is the property tests in `site/fold.test.mjs`, over
every rung the page offers at every width in both pinned columns, each with a
mutation that reddens it: what arrives at and leaves the opened node is the
same before and after the cap and the fold; the fact ids cited and the cents
drawn are the filtered ribbons' exactly, so no ribbon is lost or drawn twice;
and a capped column holds at most `cap + 1` marks, its tail's members and the
kept marks together being the whole column. The trade accepted with
`fisc-bjqa`: Go no longer refuses at build a step whose document decomposes
nothing for a year; the client walk in `site/*.test.mjs`, a CI-required check,
refuses it.

So, in one line: **Go declares what may fold and the client decides how much
fits.** The `caps` column of the table under "One page draws it" and each step's
`widen` are declarations; `capColumn` and `foldDocument` in `site/app.js` are
what spend them.
