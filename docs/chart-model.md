# The chart model: one model, drawn by forms

Evidence for the rule in AGENTS.md, "Go vets, JavaScript renders", and the
contract the decision bead `fisc-c2v6` froze. This file holds what the layers
are, what each may compute, and the measurements that fixed the seams.

## Seven layers, each computing only what the one below cannot

| Layer | Where | May compute | Crosses the seam as |
|---|---|---|---|
| Facts | `facts/facts.jsonl` | nothing: one figure the city printed | `schema/fact.schema.json` |
| Structure | `internal/structure` | which cells exist; which differences between two schedules are licensed (a residue, a gap in cents) | Go only |
| Projection | `internal/project` | one graph per printed schedule: nodes `(id, label, tier, parent, role, derived)` and links `(value_cents, kind, fact_ids, locators, derived, partition, contra)`, every figure cited | `schema/projection.schema.json` |
| Column | `internal/export` | one node table per `(fiscal_year, basis)` and, per schedule, its parent edges and links indexed into that table; re-keys and orders, computes no new figure | `schema/column.schema.json` |
| Declarations | `internal/export`, authored in `data/views.yaml` and loaded by `pkg/cmd/export` | which schedule the page's chart draws (`primary`: the expansion schedule, whose `splits` say how each node opens in place), every sentence the page says, and per chart a **form** with that form's hints under its own key | `schema/page.schema.json`, as `window.FISC_CONFIG` |
| Client core | `site/core.js` | sums over cited figures: schedule assembly, hierarchy by parent chain, the frontier a list of splits comes to, the fold to that frontier, the cap, provenance union, years, wording, and the client marks with their figures: a capped split's tail and the smaller-flows mark, the reductions among a mark's ribbons, the pages cited around a node. Reads no form hint. | `schema/mark.schema.json` for the marks it makes |
| Form renderers | `site/sankey.js` | what depends on the screen and the mark type: which open splits a cap folds and into which tail, the slot each drawn node stands in, how tall the canvas grows, where a mark stands, layout, paint. Reads only its own hints. | nothing: a renderer emits pixels |

A figure the client shows is one Go cited, or a sum or union of such figures.
A count of drawn members or a share of one drawn sum in another is a sentence
about what is drawn, not a figure: the counts line, the search count, the
filter line's merged flows, the caveat count, and the column share are of
this kind, and each carries `◇`. A client
mark is derived, says so, and carries its rationale and source note like any
derived node.

## What a second form needs, and where it already is

Measured against `schema/column.schema.json` for a treemap of one schedule:

| Need | Where it is |
|---|---|
| The hierarchy under an opened node | `schedules[k].nodes[].parent`, walked by the core |
| A node's figure | the sum of the links at the node, or of its subtree's leaves; the core sums, under the licence every merged ribbon already has |
| Provenance of a cell | the union of `fact_ids` and `locators` over the links summed |
| Which cells open, and into what | the expansion schedule's `splits`: each node's `key`, `form` and `alternative`, read by the core (`splitsOf`, `expand`) |
| A remainder between a printed cell and its members | a derived node Go stitched, drawn like any other |
| Colour | `fund_groups[].slot` |
| Which tiers to tile; figure direction | absent by design: the form's own hint object |

So a second form adds one value to `chart_form` in `schema/enums.schema.json`,
one closed hint object under its own key in `schema/page.schema.json`, and one
renderer module, which `site/embed.go` embeds and the export ships as a
verbatim asset. The column document does not change.

## Why hints nest under the form's key

The expansion schedule's splits (`node`, `key`, `form`, `alternative`) are
read by the core and vetted by Go with the schedule. A form's hints (`tiers`,
`grow`, `split_caps` for the Sankey) are read by that renderer and by that
form's validation arm. Nesting them under `sankey` lets a closed schema refuse
a hint foreign to the declared form at the write, with no hand-written arm,
and gives the generated client typedefs one type per form.

## Why the client modules hold no state

`site/testlib.mjs` loads `app.js` through a cache-busting query so each test
gets a fresh module instance, because `app.js` reads `FISC_CONFIG` and `d3` at
import and keeps the list of splits and the fetched column as module state. A
module `app.js` imports by relative path carries no such query, so Node
instantiates it once per process and every test shares it. The core and each
renderer therefore keep no module-level `let` and read no global at import: a
global is read at call time or passed in. `site/modules.test.mjs` holds that
by importing each module with no `FISC_CONFIG` and no `d3` installed. The
core's one module-level store is a memo of assembled schedules keyed by the
column object, each entry a pure function of its key.

## Counts: which side is canonical

Go's `years[].counts` and `schedules[].counts` state what the document holds,
unfolded. The client's recount states what the drawn chart holds after the
filter, the cap and the fold, which only the client can know. Both are
published, each named for what it is.

## A lens

A lens restricts what is drawn to the nodes upstream or downstream of a
declared anchor. It never totals: the headline stays a named cut over an
antichain, so no figure the city never printed acquires a total. The walk
stops at a transfer edge, because transfers make the flow graph cyclic and a
closure that crossed one could swallow most of the city. The client walks, as
it folds; Go precomputes no closure. The declaration's syntax is `fisc-l8s5`.

## The renderer interface

A form module exports one object, and `app.js` reaches a form through it and
in no other way: it imports no helper of a form's, and reads no hint. The
members, as `FormRenderer` in `site/app.js` types them:

```js
/** @typedef {{
 *  form: string,
 *  tiersOf(chart): number[],                    // the declared tiers, from its own hints: the spine's columns, which a root of the expansion is placed by
 *  width(columns): number,                      // the px a chart of this many columns is laid out at
 *  columnsOf(drawn, tiers): number,             // how many columns a shaped chart is laid out in
 *  shape(doc, view): FiscProjection,            // fold to the frontier, cap the open splits, stamp slots, flip reductions; throws to refuse; view is {chart, frontier, whole}
 *  layOut(drawn, ctx): Laid,                    // pure of the page; ctx.before seats a redrawn chart's columns
 *  render(laid, ctx): () => void,               // writes the DOM, keyed, moving from ctx.before; returns what ends the motion now
 *  paint(ctx): void,                            // repaints on a theme change
 *  cents(d): number,                            // the figure a laid mark prints, signed
 *  share(d, laid): string,                      // a laid node's share of what it is drawn among, in words; "" where none
 *  reduction(d): boolean,                       // whether a laid node's every ribbon is a printed reduction
 *  crossTab(d): boolean,                        // whether a laid node's whole figure is a cross-tab total
 *  reductionNote(d): string,                    // how much of a laid node's figure is printed as reductions, in words; ""
 *  hairlines(laid, chart): number,              // how many laid ribbons it draws as hairlines, wider than their share
 * }} FormRenderer */
```

`Laid` is `{nodes: LaidNode[], links: LaidLink[]}` as `app.js` types them: a
node with `x0, x1, y0, y1, value, layer, depth, sourceLinks, targetLinks`, a
link with its `source` and `target` resolved to nodes and a `width`. The page
reads a laid mark's geometry and its ribbons itself, for emphasis, focus and
the table; what a laid mark's figure and ribbons MEAN -- its signed cents
after the form has flipped a reduction to draw it, its share of a column, a
note on what is printed as reductions -- it asks of the form that laid it,
through the readings of a laid mark above, because only that layout knows. What is the
mark's own and no form's -- whether it is a ribbon (`isLink`), the pages it
cites -- the core answers.

A renderer calls the core for every figure (`foldDocument`, `capColumn`,
`tailFigure`, `reducedOf`) and computes none of its own; it hands the core
what only it knows, which is where a mark stands. `app.js` registers
renderers by form in `FORMS` and refuses, with a banner and before anything
is fetched, a chart whose form it does not hold; `site/lifecycle.test.mjs`
holds that refusal.
