# `fund-flows.json` — the General Fund drill-down

The citywide spine (`docs/sankey-contract.md`) answers *how big is the budget and
where does it come from*, at the grain the city prints on pp.66-67: six fund
groups, ten revenue categories, four object categories. It cannot answer *which
fund*, and it cannot answer *which department*, because pp.66-67 publish neither
axis.

This document answers those. It reads Budget Book **pp.127-140** (revenue by
fund) and **pp.167-170** (General Fund expenditure by department), and it is a
second document rather than a deeper spine because a headline is a property of a
single-grain file — see *No headline*.

**The spine is not retired by this and must not be.** It remains the only
document that publishes the city's printed totals, and the two are reconciled per
cell, at zero tolerance, by `revenue-detail-ties-to-spine` and
`expenditure-detail-ties-to-spine`, which read the fact store and need no graph.

## Files

Four, one per printed column — a flow diagram of two budgets is not a chart of
anything:

| stem | column |
|---|---|
| `data/fund-flows-2024-actual.json` | FY2023-24 actual |
| `data/fund-flows-2025-revised.json` | FY2024-25 revised |
| `data/fund-flows.json` | FY2025-26 adopted |
| `data/fund-flows-2027.json` | FY2026-27 adopted |

A column is published only when **both** schedules print it. One alone would draw
a whole revenue side against an empty expenditure side, which reads as a city
that stopped spending.

## Shape

```
tier 0  revenue/<category>, transfers/in
          |   one link per netted (kind, category, fund) cell
tier 3  fund/<n>                     parent = fund-group/<type>
          |   one link per (fund, division) — the SUM over its object rows
tier 4  dept/<division>              parent = fund/100
          |   one link per netted (department, category) cell
tier 5  expenditure/<division>/<object>   parent = dept/<division>
```

`nodes` and `links` are `project.Node` and `project.Link`, byte-identical in
shape to the spine's. **No key is added to either type**, which is why one
`schema_version` still spans every document this project publishes: the envelope
is what the version versions, and this document renames no key and coins no new
one.

Three things about the shape are not obvious and are load-bearing.

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

Key order, frozen and pinned by `TestTheFundFlowsMetadataKeyOrderMatchesTheContract`:

```
generated_by, scopes, currency, units,
fiscal_year, fiscal_year_label, basis, sources, counts, caveats
```

`scopes` is a **list**, and it is the only key that differs from the envelope the
spine and the trends document carry. A document of two schedules writing
`scopes[0]` into a singular `scope` would publish one schedule as the whole of
it; `TestTheMultiScopeEnvelopeIsTheEnvelopeWithOneKeyPluralised` pins that this
is the *only* difference, so a reader who knows where `generated_by` and `units`
sit in one document finds them in the other.

Two scopes may sit in one document only because they are **disjoint by kind**:
pp.127-140 publish revenue and `transfer_in`, pp.167-170 publish expenditure, and
every cell key carries kind. `projection-scopes-are-disjoint` asserts it, fails
closed on any undeclared pair, and refuses the pairs that restate the same money
— including `revenue-by-fund` with `transfers-by-fund`, which overlap by
$21,045,597 of FY2026 `transfer_in`.

## `counts`

**This is not the spine's `counts` block and its `facts_cited` does not mean the
same thing.**

```
facts, facts_cited, facts_uncited, facts_cited_twice, nodes, links
```

The identity is

```
facts = facts_cited + facts_uncited
```

with no stock term, because neither schedule prints a stock row. It is stated as
*uncited* rather than *in a zero cell* because those are not the same set here:
an expenditure fact whose own object cell nets to zero is **still carried** by
the fund-to-department link that sums the division, since that link's value and
its citation are both taken over every cell including the zero ones. Counting
zero cells instead would overstate the gap and the identity would not close.

`facts_cited_twice` is published because **`links` is not a partition of
`facts_cited` in this document**, which the spine's shape would lead a reader to
assume. Every expenditure fact is behind two links: its own object row and the
division total that includes it. **Summing every link's `value_cents`
double-counts the expenditure side by exactly this much.** Fold within one tier
pair; never across the whole graph.

FY2025-26: `280 = 239 + 41`, with 44 cited twice, over 145 nodes and 175 links.
`fund-flows-counts-reconcile` re-derives all six from the published links.

## No headline

A headline is a property of a **single-grain** document. The moment one file
holds the same money at two grains, "the total" is ambiguous, and no key
disambiguates it because the ambiguity is in the accumulation rather than in the
key.

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

Every tier-3 fund node carries one, and the value is **ours**: our reading of the
Description of Funds narrative, pp.258-261. `node.derived` stays `false` — the
city prints the fund — and the disclosure rides on `source_note` and `rationale`
beside it. The document also carries the disclosure sentence in
`metadata.caveats` under the id `constraint-tier-is-our-reading`, and
`constraint-tier-vocabulary` finds it by that id and then compares its text
against `project.ConstraintTierCaveat()` rather than against prose written
twice. Both halves are asserted: an id present with drifted text is a finding of
its own, because an anchor that still resolves over a weakened sentence is the
quieter failure.

See `docs/sankey-contract.md`'s `constraint_tier` section for the argument, and
for why `""` and `unknown` are different claims.

## Tiers

| tier | nodes, FY2025-26 | of 145 |
|---|---|---|
| 0 | 10 revenue categories plus `transfers/in` | 11 |
| 2 | fund groups, **all six untouched by any link** | 6 |
| 3 | funds that took in money this column | 61 |
| 4 | divisions, not departments — a fact's `department` field holds a division slug | 23 |
| 5 | division x object cells that were not a printed zero | 44 |

The tier-3 count is 61 and not the 70 funds pp.131-140 print, because nine of
them print zero in this column and a zero-valued cell earns no link. The tier-4
count is 23 and not 11: pp.167-170 print eleven DEPARTMENT totals over 23
divisions, and the divisions are what the facts carry.

Tier 1 does not exist; see `docs/sankey-contract.md`.

## What this document does not answer

- **Where the General Fund's money goes after the departments.** It does not
  balance: FY2025-26 takes in $157,873,470 and spends $144,650,802 through
  eleven departments, and the $13,222,668 difference is transfers out plus the
  change in working capital, printed on pp.66-67 and not carried here. There is
  **no invented sink node** — the caveat says so instead.
- **What the other six fund groups spend.** pp.167-170 decompose the General
  Fund alone. The money is not missing; the schedule that would break it down is
  not published. pp.72-75 give a per-fund expenses column and are the schedule a
  wider key would be for.
- **Transfers between funds.** p76's legs are scope `transfers-by-fund` and no
  projection selects it, because it overlaps `revenue-by-fund` on `transfer_in`.
  It needs a document of its own (fisc-9gh).

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
- **Links fold with their ends** and merge on the folded pair, summing
  `value_cents` and unioning both `fact_ids` and `locators`. Two links of
  different `kind` folding onto one ribbon is refused rather than resolved; it
  occurs in no published column.

  The locator union is not decoration. `buildTable` and `pin` render the
  **folded** document, and the fold builds a merged ribbon by copying its first
  leg — so without an explicit union a ribbon would cite a strict subset of the
  pages its figure was read from, and a reader clicking through would land on a
  shard holding part of the number they were shown. It de-duplicates on
  `(doc_id, page)`, which is something the fact-id union cannot express: two
  facts on one page are two ids and one locator. Measured on FY2025-26 at the
  `{0,2,4}` set the page then drew: 52 folded rows carried 79 locators, at most
  5 on any one row.

  This is what makes the flow table's `Source` column true. It used to print
  the document's own 18 pages identically on every row — 1,872 anchors saying
  nothing about the row they sat in — and became 237, each naming the pages that
  row's figure came from. Those figures are kept as the measurement that
  justified the union; the two pages that now draw this document fold it
  differently and carry their own row counts, which `tools/jscheck/drill.mjs`
  pins.
- **A link whose ends fold to the same node is dropped.** It was a flow inside
  what is now one box. This is the tier-4-to-5 case warned about above, and it
  **cites nothing away**: the fund-to-department link that survives carries the
  same money and the same facts, over every cell including the printed zeros,
  which is what `facts_cited_twice` counts. Measured on FY2025-26: 239 facts
  cited by 175 links before the fold, 239 by 52 after.
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

### Two pages draw it, and neither draws tiers 0, 2 and 4

**That set was `drilldown.html`, which no longer exists.** It drew revenue
source, fund group and division in one chart — 52 links over columns of
11 / 6 / 23 — and it is now two pages, split where the money changes hands:

| page | draws | opening a node draws | root |
|---|---|---|---|
| `revenue.html` | `{0,2}` — 11 categories into 6 fund groups, 29 links | `{0,3}` — that group's own funds | — |
| `spending.html` | `{3,4}` — the General Fund into 23 divisions, 23 links | `{4,5}` — that division's object categories | `fund/100` |

**Spending needs a root and Revenue does not**, and that is a refusal rather
than a preference. This document carries eleven tier-0 revenue nodes with no
ancestor at tier 3 or 4, and the fold refuses a node it cannot place — so
`{3,4}` over the whole document draws *nothing*, not a partial chart. `{0,2}`
places every node.

**Neither page has the property that the old tier set had**, and it is worth
saying which one: at `{0,2,4}` the fold cites nothing away, because the
surviving fund-to-division link carries the same facts as the object rows that
fold into it. At `{0,2}` the whole spending side folds to self-loops and is
dropped — Revenue's ribbons cite 190 of the document's 239 facts and Spending's
the other 49. They partition it exactly; neither page can claim to carry it all,
and both say so.

### Opening a node: filter, cap, fold

**Expanding a node in place is not a per-node interaction on this d3-sankey**
(`fisc-ppkq`). The vendored build takes the column count from topology and
clamps the align into it, so expanding one group draws the funds and the
divisions in the same column while the unexpanded ribbons span two — which
`tools/jscheck/layout.mjs`'s `bands()` refuses. The shape that works is
filtering to one node and rescaling to its own total.

**Rescaling alone is not enough, and `fisc-ppkq` says it is.** Measured:
filtered to special revenue and rescaled to that group's own total, 22 of its 49
ribbons still lay out under one pixel, because the concentration is *within* the
group — `fund/200` alone is 34.9% of it and the smallest two are 0.034%.
Rescaling cannot fix a distribution.

**So a drill also caps its fine column.** Above `Drill.Cap` nodes, the tail by
value folds into one aggregate. At cap 8 special revenue draws 2 sub-pixel
ribbons instead of 22, and capital 1 instead of 4. The cap is inert on Spending,
whose widest division spends on two object categories.

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

`tools/jscheck/drill.mjs` re-measures all of this on every `make js`, opening
every node both pages offer — 6 fund groups and 23 divisions — rather than a
sample.

**Tier 5 is not a one-constant alternative.** Drawing `{0,2,4,5}` puts 29 of the
44 object nodes under one pixel (smallest 0.030px), and that column's labels are
23× "Services & Supplies" and 21× "Wages & Benefits". The object grain is not
*hidden* by the fold; it is unrenderable at this canvas, and offering it needs a
view that rescales to one division rather than a fourth column — which is what
Spending's drill is. Measured over all 23 divisions, the smallest ribbon in any
opened view is 51.4px.

**What the fold does not fix.** At the `{0,2,4}` set, seven of the 52 ribbons
laid out under 1px and four of the 40 node rects under 2px, and `render()` floors
both — `Math.max(1, width - RIBBON_GAP)` and `Math.max(2, y1 - y0)` — so those
marks do not encode their values. `tools/jscheck/fold.mjs` pins **both** counts,
so neither can grow unnoticed. The shipped pages are better on this and not
free of it: Revenue's overview draws 5 sub-pixel ribbons of 29 and Spending's
1 of 23.

**The page describes the folded document, not the fetched one.** The legend, the
flow table, the inferred list and the flow count are all statements about what
the reader is looking at; pointing them at the file would put a 175-row table
beside a 52-ribbon chart. The footer still links the unfolded file, and the
merged links still name every fact behind every ribbon.
