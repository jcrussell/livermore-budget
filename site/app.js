// @ts-check
/**
 * fisc — the published page's client script.
 *
 * Served exactly as committed; the page template boots it by importing [boot].
 * Every top-level binding is exported so the tests import the shipped file,
 * and nothing runs at import. Nothing runs tsc: `@ts-check` is for an editor.
 *
 * Go validates every artifact against schema/ before writing it, so nothing
 * here re-checks a shape. What this file refuses is what only it can answer:
 * a 200 carrying an error page, a file cached from before the last deploy,
 * and a chart its own sums contradict -- a gap that drifts from its licence,
 * a window with no flank, a reduction's node that sums to nothing.
 */

/* global d3 */

/*
 * The wire shapes are schema/'s. Go validates every artifact against its
 * schema before writing it, so a typedef below names one shape by its schema
 * path and states none of its fields; the fields a typedef does list are ones
 * this file adds, which no artifact carries.
 */

import * as core from "./core.js";
import {
  SCHEMA_VERSION, say, kindLabel, fmt, fmtSigned, joinOr, el, maybeEl, cssVar, h, link, citations, balancesOf, PARTITION_NOTE, isFundGroup, scheduleOf, isMark, isResidual, isGap, isLink, residualFlows, tableRows,
} from "./core.js";
import { SANKEY } from "./sankey.js";
// Re-exported so the tests reach the shipped modules through the one file
// they import; this file calls a form only through the renderer it registers.
export * from "./core.js";
export * from "./sankey.js";

/**
 * A node as schema/projection.schema.json#/properties/nodes/items has it,
 * which scheduleOf assembles from a column document's node table and a
 * schedule's parent edges, plus what the client sets on it. carried_from: the
 * stem of the document a node was carried from into a window. fixedValue: the
 * figure d3-sankey sizes the node at where its drawn ribbons do not add up to
 * it.
 * @typedef {Record<string, any> & {carried_from?: string, fixedValue?: number}} FiscNode
 */
/** schema/mark.schema.json, a node this client makes: an aggregate, a residual or a gap. @typedef {Record<string, any>} FiscMark */

/** schema/projection.schema.json#/properties/links/items. @typedef {Record<string, any>} FiscLink */
/** schema/locator.schema.json. @typedef {Record<string, any>} FiscSource */
/** schema/projection.schema.json#/properties/metadata. @typedef {Record<string, any>} FiscMetadata */
/** schema/caveat.schema.json. @typedef {Record<string, any>} FiscCaveat */
/** schema/page.schema.json#/properties/years/items/properties/caveats/items. @typedef {Record<string, any>} FiscCaveatRef */
/** schema/page.schema.json#/properties/docs/additionalProperties. @typedef {Record<string, any>} FiscDoc */
/** schema/page.schema.json#/properties/years/items/properties/hero. @typedef {Record<string, any>} FiscFigure */
/** schema/page.schema.json#/properties/years/items. @typedef {Record<string, any>} FiscYear */
/** schema/page.schema.json#/properties/years/items/properties/steps/items. @typedef {Record<string, any>} FiscStepDoc */
/** schema/page.schema.json, the page's window.FISC_CONFIG. @typedef {Record<string, any>} FiscConfig */
/** schema/page.schema.json#/$defs/sankey_hints, a chart's Sankey hints. @typedef {Record<string, any>} FiscSankeyHints */
/** schema/page.schema.json#/$defs/sankey_hints/properties/caps/items. @typedef {Record<string, any>} FiscTierCap */
/** schema/page.schema.json#/properties/overview, the page's own chart: its form and hints. @typedef {Record<string, any>} FiscChart */
/** schema/page.schema.json#/properties/steps/items, one step of the drill tree. @typedef {Record<string, any>} FiscDrillStep */
/** schema/page.schema.json#/properties/steps/items/properties/gaps/additionalProperties/items. @typedef {Record<string, any>} FiscGap */
/** schema/projection.schema.json, as scheduleOf assembles it from a column document. @typedef {Record<string, any>} FiscProjection */

/**
 * A node after d3-sankey has laid it out (d3 mutates what it is given).
 * layer is the column d3 put it in, which is not depth; the form's share reads layer.
 * @typedef {FiscNode & {x0:number, x1:number, y0:number, y1:number, value:number,
 *   sourceLinks:LaidLink[], targetLinks:LaidLink[], depth:number,
 *   layer:number}} LaidNode
 */

/**
 * A link after layout: source and target are node objects, not ids.
 * @typedef {Omit<FiscLink,"source"|"target"> & {source:LaidNode, target:LaidNode,
 *   value:number, width:number, y0:number, y1:number, index:number}} LaidLink
 */

/** d3 and d3-sankey are vendored UMD bundles with no type declarations. */
export const D3 = /** @type {any} */ (/** @type {any} */ (globalThis).d3);

/** @type {FiscConfig} */
export const CONFIG = /** @type {any} */ (globalThis).FISC_CONFIG;

/**
 * core.foldDocument over the page's own tier set unless a drill passes its
 * own. The state-free rule is core.js's; this binds the overview.
 * @param {FiscProjection} doc
 * @param {number[]} [tiers]
 * @returns {FiscProjection}
 */
export function foldDocument(doc, tiers) {
  return core.foldDocument(doc, tiers || RENDER_TIERS);
}

/**
 * core.fundGroupOf over the drawn document's index.
 * @param {FiscNode | LaidNode} node
 * @returns {string}
 */
export function fundGroupOf(node) {
  return core.fundGroupOf(groupIndex, node);
}

/**
 * A laid mark's share of what it is drawn among, in the form's words, over
 * the chart's laid nodes; "" where the form states none.
 * @param {LaidNode} d
 * @returns {string}
 */
export function shareOf(d) {
  return formOnScreen().share(d, laidNodes);
}

/**
 * core.homeOf over the document on screen.
 * @param {FiscLink} l
 * @returns {string}
 */
export function homeOf(l) {
  return core.homeOf(projection, l);
}

/**
 * What a form module exports: one object app.js hands every chart of that
 * form to, and the only way this file reaches a form. A renderer reads of a
 * step the generic fields and its own hints, calls core.js for every figure,
 * and computes none of its own. Each member is set out in
 * docs/chart-model.md; the readings over a laid mark (cents, share,
 * reduction, crossTab, reductionNote) are the form's because only the
 * layout that drew the mark knows what its drawn figure and ribbons mean.
 * @typedef {{
 *   form: string,
 *   tiersOf(chart: FiscChart | FiscDrillStep): number[],
 *   caps(chart: FiscChart | FiscDrillStep): FiscTierCap[],
 *   columns(chart: FiscDrillStep, rung: Rung | null, budget: number): number[],
 *   width(columns: number): number,
 *   offers(step: FiscDrillStep, doc: FiscProjection, onScreen: FiscProjection | null, id: string, budgets: number[]): boolean,
 *   shape(doc: FiscProjection, rung: Rung | null, from: FiscProjection | null, tiers: number[]): FiscProjection,
 *   layOut(drawn: FiscProjection, ctx: {tiers: number[], columns: number, groupOf: (n: FiscNode | LaidNode) => string, placeOf: (id: string) => number}): {nodes: LaidNode[], links: LaidLink[]},
 *   render(graph: {nodes: LaidNode[], links: LaidLink[]}, ctx: Record<string, any>): void,
 *   paint(ctx: {svg: any, colour: {link: (d: LaidLink) => string, node: (d: LaidNode) => string}}): void,
 *   widest(steps: FiscDrillStep[]): number,
 *   cents(d: LaidLink | LaidNode): number,
 *   share(d: LaidNode, laid: LaidNode[]): string,
 *   reduction(d: LaidNode): boolean,
 *   crossTab(d: LaidNode): boolean,
 *   reductionNote(d: LaidNode): string,
 * }} FormRenderer
 */

/**
 * The renderers this script holds, by form. Every chart the page declares
 * names its form, and one naming a form not here is refused before anything
 * is fetched: the wrong renderer draws a chart that is wrong rather than one
 * that fails. A second form registers here and nowhere else.
 * @type {Map<string, FormRenderer>}
 */
export const FORMS = new Map([["sankey", SANKEY]]);

/**
 * The renderer for a chart, by its declared form.
 * @param {FiscChart | FiscDrillStep} chart
 * @returns {FormRenderer}
 */
export function formFor(chart) {
  const renderer = FORMS.get(chart.form);
  if (!renderer) throw new Error("no renderer here draws a " + chart.form + " chart");
  return renderer;
}

/**
 * The node tiers this page's own Sankey draws, coarsest first: the overview's
 * hints. Per view and never a constant here: the spine and the drill-down
 * have different hierarchies, and one's set over the other throws. Absent
 * skips the fold entirely.
 * @type {number[]}
 */
export const RENDER_TIERS = (() => {
  const overview = CONFIG && CONFIG.overview;
  const renderer = overview ? FORMS.get(overview.form) : null;
  return renderer ? renderer.tiersOf(overview) : [];
})();


/**
 * The form of the first chart this page declares that no renderer here draws,
 * or "" when every one is drawable. The overview and every step declare one.
 * @returns {string}
 */
export function undrawableForm() {
  const charts = [CONFIG.overview || {}].concat(STEPS);
  const missing = charts.find((c) => !FORMS.has(c.form));
  return missing ? String(missing.form) : "";
}

/**
 * The steps this page drills through, read by key; empty opens nothing.
 * Not filtered here: the packager validates the tree and a filter would drop
 * a step silently, leaving nodes that will not open and no banner.
 * @type {FiscDrillStep[]}
 */
export const STEPS = (CONFIG && CONFIG.steps) || [];

/**
 * The key of the step whose chart is on screen, "" on the overview.
 * @returns {string}
 */
export function openedKey() {
  return drilled.length ? drilled[drilled.length - 1].step.key : "";
}

/**
 * The step this node of the chart on screen opens into, or null.
 *
 * Four matches, not a depth: `after` contains the rung's key, `from` is the
 * node's tier, `role` (when named) is the node's, and the year's document
 * decomposes this node. The role is matched, never inferred from an id.
 *
 * @param {{id?: string, tier: number, role?: string}} node
 * @returns {FiscDrillStep | null}
 */
export function stepFor(node) {
  const key = openedKey();
  for (const s of STEPS) {
    if (!s.after.includes(key)) continue;
    if (s.from !== node.tier) continue;
    if (s.role && s.role !== node.role) continue;
    if (!stepDecomposes(s, node.id)) continue;
    return s;
  }
  return null;
}

/**
 * Whether opening `id` on `step` would draw at every budget (BUDGETS): the
 * form's answer (offers). For the Sankey, the step's document decomposes the
 * node into every column the step promises and does not keep at the columns
 * each budget draws, and on a window step the chart on screen sends a kept
 * flank into it, each asked as the chart is drawn, so what is offered and
 * what draws are one rule. A node with no id is open too.
 *
 * @param {FiscDrillStep} step
 * @param {string | undefined} id
 * @returns {boolean}
 */
export function stepDecomposes(step, id) {
  if (!id) return true;
  const doc = step.projection ? scheduleOf(column, step.projection) : drawnDoc();
  // A column carrying no such schedule offers the node, so the drill can say
  // which schedule is missing rather than the node silently not opening.
  if (!doc) return true;
  return formFor(step).offers(step, doc, projection, id, BUDGETS);
}

/**
 * The fund groups the column on screen draws, in Go's order. The set is open,
 * so it is never a constant here.
 * @returns {{id: string, slug: string}[]}
 */
export function fundGroups() {
  return (column && Array.isArray(column.fund_groups)) ? column.fund_groups : [];
}

/**
 * A fund group's place in the drawn order; nodeRank and buildLegend must agree
 * on it.
 * @param {string} id
 * @returns {number}
 */
export function fundGroupPlace(id) {
  return fundGroups().findIndex((g) => g.id === id);
}

/**
 * The fund group a link is drawn in: the fund group at either end, else the
 * group both ends share, else the group of the one end that has one -- a
 * revenue line or an endpoint is in none, and the fund it reaches says
 * which group the money is in -- else "" (--muted) rather than picking one
 * of two.
 * @param {LaidLink} link
 * @returns {string}
 */
export function linkGroup(link) {
  const source = fundGroupOf(link.source);
  const target = fundGroupOf(link.target);
  return isFundGroup(link.source) ? link.source.id
    : isFundGroup(link.target) ? link.target.id
    : source === target ? source
    : !target ? source
    : !source ? target
    : "";
}

/**
 * A link's hue: its group's (linkGroup).
 * @param {LaidLink} link
 * @returns {string}
 */
export function linkColor(link) {
  return cssVar(fundColorVar(linkGroup(link)));
}

/**
 * @param {LaidNode} node
 * @returns {string}
 */
export function nodeColor(node) {
  return cssVar(fundColorVar(node.id));
}

/**
 * The custom property holding a fund group's hue: the slot the column ships
 * for it, or --muted for a group with none or a slot style.css has no hue for.
 * Shared so the legend swatch matches the chart.
 * @param {string} id
 * @returns {string}
 */
export function fundColorVar(id) {
  const group = fundGroups().find((g) => g.id === id);
  if (!group) return "--muted";
  if (!(group.slot > 0)) return "--muted";
  const name = "--fund-slot-" + group.slot;
  return cssVar(name) ? name : "--muted";
}

/** @type {FiscProjection | null} */
export let projection = null;
/**
 * What the reader has asked the chart to show. groups: the fund groups whose
 * ribbons are removed; kinds: the link kinds removed; smallest: the smallest
 * flow drawn as itself, in cents, 0 for every one (core.filterDocument);
 * isolated: the node or fund group whose money is followed, or "" for all of
 * it (follows). Replaced whole, never mutated, and moved only through
 * setFilter and setIsolated. A group or kind the chart on screen does not
 * draw is kept and is inert, so a drill or a year change carries it to
 * wherever it applies again; the isolation alone is cleared by both, since
 * either can remove the node it names.
 * @typedef {{groups: ReadonlySet<string>, kinds: ReadonlySet<string>, smallest: number, isolated: string}} FilterState
 */
/** The state that removes and follows nothing. @type {Readonly<FilterState>} */
export const NO_FILTER = Object.freeze({ groups: new Set(), kinds: new Set(), smallest: 0, isolated: "" });
/** @type {Readonly<FilterState>} */
export let filter = NO_FILTER;
/**
 * The chart on screen as the filters leave it: the document laid out, and
 * the ribbons of the shaped chart they removed. Null before the first draw.
 * @type {{doc: FiscProjection, hidden: FiscLink[]} | null}
 */
export let shown = null;
/**
 * The chart on screen laid out with no filter, where the filters change what
 * is drawn, else null: what a node's figure is read off (markFigure), so a
 * filter chooses which cited marks are drawn and never restates a mark's
 * figure. Moved with `shown`, inside the same rollback.
 * @type {{nodes: LaidNode[], links: LaidLink[]} | null}
 */
export let whole = null;
/**
 * The smallest-flow control's stops, in cents: every flow, then $10,000 up
 * to $10,000,000 by tens, a scale of magnitudes rather than of dollars.
 */
export const SMALLEST_STOPS = Object.freeze([0, 1000000, 10000000, 100000000, 1000000000]);
/**
 * One opened node. The document is on the rung, not looked up, because a step
 * that names a projection opens a node of one file into a chart of another.
 * @typedef {Object} Rung
 * @property {string} id  the node opened
 * @property {FiscProjection} doc  the document this rung's chart is shaped
 *   from, unfolded
 * @property {FiscDrillStep} step  the step that opened it
 * @property {FiscProjection} chart  the chart on screen when this rung was
 *   opened, as the reader saw it. Recorded, not recomputed: a pop reshapes a
 *   rung whose parent chart is gone.
 * @property {Set<number>} [expanded]  the tiers of this rung's chart drawn
 *   whole rather than capped. Per rung, so it does not leak into a pop, the
 *   next window or the next year.
 */

/**
 * The nodes the chart is opened into, outermost first; empty on the overview.
 * @type {Rung[]}
 */
export let drilled = [];
/**
 * The fewest columns a window is laid out in: kept flank, opened node and what
 * it opens into. A floor, not a default.
 */
export const NARROW_COLUMNS = 3;
/**
 * The most columns any step this page declares can ask for: the longest
 * `tiers`. COLUMN_QUERIES answers the room; the reader gets the smaller.
 */
export const OFFERED_COLUMNS = Math.max(NARROW_COLUMNS,
  ...Array.from(FORMS.values(), (r) => r.widest(STEPS.filter((s) => s.form === r.form))));
/**
 * Every budget the page can draw at, NARROW_COLUMNS to OFFERED_COLUMNS: what
 * a node is offered at (stepDecomposes), since the reader can change the
 * width with the node open.
 */
export const BUDGETS = Object.freeze(Array.from({ length: OFFERED_COLUMNS - NARROW_COLUMNS + 1 },
  (_, k) => NARROW_COLUMNS + k));

/**
 * The px `100vw` counts that the window does not: body padding plus a classic
 * scrollbar. The page's, not a form's; style.css records it independently as
 * --chart-cushion.
 */
export const CHART_CUSHION = 56;

/**
 * How wide the widest form here lays `n` columns out, in px: the width at
 * which the nth column fits whichever chart the page draws.
 * @param {number} n
 * @returns {number}
 */
export function widthOf(n) {
  return Math.max(...Array.from(FORMS.values(), (r) => r.width(n)));
}

/**
 * The viewport widths that buy a column beyond the floor. Each threshold is
 * widthOf(n) plus CHART_CUSHION, computed so it is the width at which the
 * nth column fits. Only ever adds columns; never below NARROW_COLUMNS.
 * @type {{query: string, columns: number}[]}
 */
export const COLUMN_QUERIES = (() => {
  const out = [];
  for (let n = NARROW_COLUMNS + 1; n <= OFFERED_COLUMNS; n++) {
    out.push({ query: "(min-width: " + (widthOf(n) + CHART_CUSHION) + "px)", columns: n });
  }
  return out;
})();

/**
 * The widest chart this page can be asked to draw, in px, handed to style.css
 * as --chart-max so the stylesheet holds no second spelling of it.
 */
export const CHART_MAX = widthOf(OFFERED_COLUMNS);

/**
 * How many columns the chart may draw (activeTiers). Per reader, not per view:
 * it never names a tier, only how many of a step's declared tiers to take.
 * Moved only through setColumnBudget.
 */
export let columnBudget = NARROW_COLUMNS;

/**
 * The column count the reader asked for, or null when they have not asked.
 *
 * While set, a media query firing changes nothing. Separate from columnBudget
 * because the two are equal exactly when the reader has NOT chosen.
 */
export let columnOverride = null;

/**
 * Sets how many columns the chart may draw, and says whether that moved. The
 * caller redraws. Clamped rather than refused, because the caller is a media
 * query.
 *
 * @param {number} n
 * @returns {boolean} whether the budget changed
 */
export function setColumnBudget(n) {
  const want = Math.max(NARROW_COLUMNS, Math.floor(Number(n)) || NARROW_COLUMNS);
  if (want === columnBudget) return false;
  columnBudget = want;
  return true;
}

/**
 * The year's document as fetched, before any fold. A drill reshapes from this,
 * never from the folded document on screen, whose deeper tiers are gone.
 * @type {FiscProjection | null}
 */
export let fetched = null;
/**
 * The column document the year on screen was fetched from, one per (fiscal
 * year, basis). A drill selects schedules out of it and fetches nothing.
 * @type {any}
 */
export let column = null;
/**
 * The drill's gesture token, bumped by every push and pop, so a drill whose
 * fetch was overtaken stands down after the await.
 */
export let opening = 0;
/**
 * The year on screen, so paintCounts can be called without one in hand.
 * @type {FiscYear | null}
 */
export let shownYear = null;
/**
 * The chart description the page shipped, captured on first paint because it
 * is the template's string and not in the config.
 * @type {string}
 */
export let baseDescription = "";
/**
 * The pointer to the flow table, lifted off the shipped description so a drill
 * keeps it: the table sits in a closed <details>, out of the accessibility
 * tree, and this sentence is a screen reader's only route to it. Taken by
 * position (the template puts it last), not by its words.
 * @type {string}
 */
export let tablePointer = "";
/**
 * The nodes as laid out, so shareOf can hand the form a mark's column. Separate
 * from projection because columns are d3-sankey's answer, not the file's.
 * @type {LaidNode[]}
 */
export let laidNodes = [];
/** @type {LaidNode | LaidLink | null} */
export let pinned = null;
/**
 * How soon, in ms, a later event on the same node counts as part of one
 * activation: the click assistive tech synthesises from Enter/Space, and the
 * two clicks a pointer delivers before their dblclick.
 */
export const ACTIVATION_WINDOW = 500;

/**
 * The node and timestamp of the last Enter/Space activation, so that the click
 * some assistive tech synthesises from that same key press does not undo it.
 * @type {{id:string, at:number}}
 */
export let keyActivation = { id: "", at: -Infinity };

/**
 * The node a click last isolated, when, and what was isolated before, so the
 * dblclick a pair of clicks composes into can put that back. Clicks are not
 * debounced. `was` is recorded once per window, not per click, or the second
 * click would record the state the first produced.
 * @type {{id:string, at:number, was:string}}
 */
export let clickIsolate = { id: "", at: -Infinity, was: "" };

/**
 * Every node of the document last laid out, by id, so fundGroupOf can walk
 * node.parent. Layout state: paint() reads it on a theme change without
 * laying out again.
 * @type {Map<string, FiscNode>}
 */
export let groupIndex = new Map();

/* ------------------------------------------------------------------ *
 * Chart
 * ------------------------------------------------------------------ */

/**
 * The document the chart at one depth is shaped from: the year's at depth 0,
 * the rung's own below. By depth because rung k's label lives in the document
 * one depth up, where the clicked node still exists (paintBreadcrumb).
 * @param {number} depth
 * @returns {FiscProjection | null}
 */
export function docAt(depth) {
  return depth <= 0 ? fetched : (drilled[depth - 1] ? drilled[depth - 1].doc : null);
}

/**
 * The document the chart on screen is shaped from.
 * @returns {FiscProjection | null}
 */
export function drawnDoc() {
  return docAt(drilled.length);
}

/**
 * The tier set the document on screen is shaped by: RENDER_TIERS on an
 * overview, and on a rung the form's columns for the opening step at this
 * budget, which the rung keeps no record of: every budget is asked afresh,
 * so a width the reader moves to draws what it would have drawn opened
 * there. A wrong answer here throws inside d3-sankey's ordering pass.
 *
 * @returns {number[]}
 */
export function activeTiers(budget) {
  if (!drilled.length) return RENDER_TIERS;
  const at = budget === undefined ? columnBudget : budget;
  const rung = drilled[drilled.length - 1];
  return formFor(rung.step).columns(rung.step, rung, at);
}

/**
 * How many columns wide the chart on screen is laid out. A chart declaring no
 * column order is laid out at NARROW_COLUMNS: d3 infers its columns only after
 * the layout the width feeds.
 *
 * @param {number} [budget] the width to ask about; the current one by default
 * @returns {number}
 */
export function drawnColumns(budget) {
  return activeTiers(budget).length || NARROW_COLUMNS;
}

/**
 * Writes the flow count: the marks drawn, not the rows the file holds, and the
 * facts those ribbons cite against the drawn document's own total -- both
 * stated, so the gap the chart cannot show stays visible.
 *
 * A window's carried ribbons cite another document and are counted apart,
 * partitioned by the stem a carried mark records rather than by the flag: the
 * fund and division windows keep a flank of the document they draw.
 *
 * A document with no metadata.counts is counted by its ribbons alone rather
 * than dereferenced: a throw here, mid-repaint, splits the page (fisc-bsg).
 */
export function paintCounts() {
  const counts = maybeEl("counts-line");
  if (!counts || !shownYear) return;
  const links = projection ? projection.links.length : shownYear.counts.links;
  const nodes = projection ? projection.nodes.length : shownYear.counts.nodes;
  // PLURALS ARE THE WORDING'S: a drilled division can draw one ribbon.
  let text = say("counts", { links, nodes, facts: shownYear.counts.facts });
  if (projection) {
    const doc = drawnDoc();
    const drawnStem = doc ? doc.projection : "";
    const byID = new Map(projection.nodes.map((n) => [n.id, n]));
    // The stem of a mark this document has never heard of, "" for one it has:
    // carried_from alone cannot tell the fund window's own-document flank from
    // the object category's, which is the spine's.
    const guestOf = (/** @type {string} */ id) => {
      const n = byID.get(id);
      const of = n && n.carried_from ? n.carried_from : "";
      return of && of !== drawnStem ? of : "";
    };
    const cited = new Set();
    const above = new Set();
    /** @type {Set<string>} the documents the carried ribbons came from */
    const stems = new Set();
    let carried = 0;
    for (const l of projection.links) {
      // A carried flow cites the chart above, not this document.
      const of = guestOf(l.source) || guestOf(l.target);
      if (of || isResidual(l.source) || isResidual(l.target)) {
        carried++;
        if (of) stems.add(of);
        for (const id of l.fact_ids) above.add(id);
        continue;
      }
      for (const id of l.fact_ids) cited.add(id);
    }
    const factsIn = (/** @type {FiscProjection | null} */ d, /** @type {number} */ fallback) =>
      !d || d === fetched ? shownYear.counts.facts
        : d.metadata && d.metadata.counts && typeof d.metadata.counts.facts === "number"
          ? d.metadata.counts.facts : fallback;
    const total = factsIn(doc, cited.size);
    if (!carried) {
      text = cited.size === total
        ? say("counts", { links, nodes, facts: cited.size })
        : say("counts_partial", { links, nodes, cited: cited.size, facts: total });
    } else {
      // Split before either fact count is given, so neither number is attached
      // to the whole chart.
      text = say("counts_carried", { links, nodes, own: projection.links.length - carried,
        cited: cited.size, facts: total, carried });
      // Named only where one document with a counts block is nameable;
      // otherwise the clause is the count of ribbons alone.
      const src = stems.size === 1 ? carriedSource(Array.from(stems)[0]) : null;
      const theirs = src ? factsIn(src, 0) : 0;
      if (theirs) text += say("counts_carried_from", { above: above.size, theirs });
    }
  }
  counts.textContent = text;
}

/**
 * Whether activating this node opens it: a step opens from it (stepFor), and
 * it is neither a mark this client made (an aggregate or a smaller-flows
 * mark has no one node to open into) nor a carried mark, which is a flow's
 * end rather than a container.
 *
 * @param {{id: string, tier: number, role?: string}} d
 * @returns {boolean}
 */
export function drillable(d) {
  return Boolean(stepFor(d)) && !isMark(d.id) && !isCarried(d.id);
}

/**
 * Whether a mark is a folded tail this rung can draw out at full length: a
 * redraw of the chart the reader is on, not a rung, so disjoint from
 * drillable. By the cap THIS rung declares, not the prefix alone, since an
 * aggregate in a kept flank was folded by the chart above.
 *
 * @param {{id: string, tier: number}} d
 * @returns {boolean}
 */
export function expandable(d) {
  const rung = drilled.length ? drilled[drilled.length - 1] : null;
  if (!rung || !core.isAggregate(d.id)) return false;
  if (rung.expanded && rung.expanded.has(d.tier)) return false;
  return formFor(rung.step).caps(rung.step).some((c) => c.tier === d.tier);
}

/**
 * The tiers of the chart on screen the reader has drawn out, in column order
 * so the breadcrumb's chips do not swap between redraws.
 */
export function expandedTiers() {
  const rung = drilled.length ? drilled[drilled.length - 1] : null;
  if (!rung || !rung.expanded) return [];
  return activeTiers().filter((t) => rung.expanded.has(t));
}

/**
 * Whether focus is in the chart or its breadcrumb, asked while its element
 * still exists. Not merely "not the body": Escape from the flow table or the
 * footer must not pull focus into the chart.
 * @returns {boolean}
 */
export function focusInChart() {
  const active = document.activeElement;
  const chart = maybeEl("chart");
  return Boolean(active) && Boolean(chart) &&
    (active === chart || (typeof chart.contains === "function" && chart.contains(active)) ||
      (typeof active.closest === "function" && active.closest(".breadcrumb") !== null));
}

/**
 * The year's entry for a step -- the caveat refs its marks link to -- or null
 * when the year on screen was packaged with none. Indexed by the step's place
 * in the declaration, not by depth: two steps open from depth 0.
 * @param {FiscDrillStep} step
 * @returns {FiscStepDoc | null}
 */
export function stepDocFor(step) {
  const steps = (shownYear && shownYear.steps) || [];
  const at = STEPS.indexOf(step);
  const entry = at >= 0 ? steps[at] : undefined;
  return entry || null;
}

/**
 * The document a step draws: a schedule selected out of the year's column, or
 * the document it opened from where it names none. Null when the column
 * carries no such schedule (the reader is told) or the drill was superseded.
 * @param {FiscDrillStep} step
 * @param {FiscProjection} from  the document of the chart the step opens from
 * @param {() => boolean} superseded
 * @returns {Promise<FiscProjection | null>}
 */
export async function stepDocument(step, from, superseded) {
  if (!step.projection) return from;
  if (superseded()) return null;
  return selectSchedule(column, step.projection);
}

/**
 * Asks of every node of the chart on screen what the paints ask per mark
 * and per paint -- whether it opens, and whether it expands -- so every step
 * schedule those questions assemble, and every flank and decomposition they
 * read, is met inside the caller's rollback rather than thrown from
 * paintChartHint or render over a page already half repainted. Nothing is
 * kept here: scheduleOf, decomposable and flankHolds memoise on the
 * documents themselves, so the paints read what this asked.
 *
 * Called with `projection` already the chart being drawn, since the
 * questions read it.
 */
export function askAffordances() {
  if (!projection) return;
  for (const n of projection.nodes) {
    drillable(n);
    expandable(n);
  }
}

/**
 * Replaces the stack with `next` and repaints everything the shape decides,
 * or leaves the page exactly as it was and tells the reader why.
 *
 * Everything that can throw on a served document -- shape, lay-out, table
 * rows, and what the paints ask of the other schedules (askAffordances) --
 * runs in the try while the page is still the reader's. The stack and the
 * chart are swapped first because shapeFor, layOut and the questions read
 * them, and restored whole on a throw. Pin and isolation are cleared because
 * a drill can remove the node they name.
 *
 * @param {Rung[]} next
 * @param {string} [refused] the banner's opening words when the redraw is refused
 * @param {() => string} [landOn] the mark focus goes to on the redrawn chart,
 *   asked after it is laid out, where the redraw closes no rung
 * @returns {boolean} whether the new depth is on screen
 */
export function redrawStack(next, refused = "That could not be opened", landOn) {
  // Asked before the repaint removes the element focus is on.
  const hadFocus = focusInChart();
  const was = drilled;
  // THE RUNG THIS REDRAW CLOSES, if it closes one: the mark that opened it is
  // on the chart being returned to, and is where focus goes back to.
  const popped = was.length > next.length ? was[next.length].id : "";
  const laidWas = laidNodes;
  const groupsWas = groupIndex;
  const chartWas = projection;
  const shownWas = shown;
  const wholeWas = whole;
  drilled = next;
  let drawn;
  let laid;
  let rows;
  try {
    const doc = drawnDoc();
    if (!doc) throw new Error("no document to open");
    drawn = shapeFor(doc);
    const filtered = filterFor(drawn);
    const unfiltered = wholeOf(drawn, filtered);
    laid = layOut(filtered.doc);
    // Inside the try: building a row is the last step of a draw that can
    // throw on a document. The table is the view's every flow, filtered or
    // not; the filter line says what the chart leaves out.
    rows = tableRows(drawn);
    projection = drawn;
    shown = filtered;
    whole = unfiltered;
    askAffordances();
  } catch (e) {
    // Back to where the reader was: a throw here is a fault in the view's
    // declaration, not in the reader's click.
    drilled = was;
    laidNodes = laidWas;
    groupIndex = groupsWas;
    projection = chartWas;
    shown = shownWas;
    whole = wholeWas;
    fail(refused + ": " + (e instanceof Error ? e.message : String(e)));
    return false;
  }
  clearRefusal();
  pinned = null;
  filter = Object.freeze(Object.assign({}, filter, { isolated: "" }));
  resetDetail();
  hideTip();
  paintBreadcrumb();
  paintChartName();
  paintCounts();
  // The count and steppers describe the chart on screen, which a drill changes.
  syncColumns();
  buildLegend();
  paintFilterLine();
  paintChartHint();
  buildDerivedList();
  buildTable(rows);
  render(laid);
  restoreFocus(hadFocus, popped || (landOn ? landOn() : ""));
  // A drill or a pop is a step a reader walks back with Back; a redraw on
  // the same path (a width, a tail) leaves the hash as it was.
  recordView("push");
  return true;
}

/**
 * Opens one node of the chart on screen, one rung deeper.
 *
 * After the await it stands down if overtaken: the tokens catch a gesture
 * that started later, the document identity a year switch already in flight
 * when this one started.
 *
 * @param {string} id
 * @returns {Promise<string>} DREW, SUPERSEDED or FAILED
 */
export async function drillDown(id) {
  const depth = drilled.length;
  const from = docAt(depth);
  // The node comes off the chart the reader activated it on: a kept flank's
  // nodes need not exist in the rung's file. Every refusal is said, because an
  // unexplained FAILED looks like a page that did nothing.
  const chart = projection;
  if (!chart || !from) {
    fail("That could not be opened: there is no chart on screen to open it from.");
    return FAILED;
  }
  const node = chart.nodes.find((n) => n.id === id);
  if (!node) {
    fail("That could not be opened: the chart on screen draws no mark called " + id + ".");
    return FAILED;
  }
  const step = drillable(node) ? stepFor(node) : null;
  if (!step) {
    fail("That could not be opened: no step on this page opens " + node.label + ".");
    return FAILED;
  }
  const mine = ++opening;
  const token = switching;
  const overtaken = () => mine !== opening || token !== switching;
  const doc = await stepDocument(step, from, overtaken);
  // `switching` is bumped when showYear STARTS, so a drill begun during a year
  // fetch compares equal when the new spine lands; asking whether the document
  // it was opened against is still at this depth catches that ordering.
  if (overtaken() || docAt(depth) !== from) return SUPERSEDED;
  if (!doc) return FAILED;
  return redrawStack(drilled.concat([{ id: id, doc: doc, step: step, chart: chart }]))
    ? DREW : FAILED;
}

/**
 * Closes rungs until `depth` remain: 0 is the overview. Bumps the drill token
 * so a drill in flight from a closed rung stands down.
 * @param {number} depth
 */
export function drillUp(depth) {
  if (depth < 0 || depth >= drilled.length) return;
  opening++;
  redrawStack(drilled.slice(0, depth), "That chart could not be closed");
}

/**
 * What a click or key does to a node that opens: drills, and banners whatever
 * drillDown did not catch itself.
 * @param {string} id
 */
export function openNode(id) {
  void drillDown(id).catch((e) => fail("The chart failed to draw: " + String(e)));
}

/**
 * Draws the column a folded tail was cut out of at every mark it holds: the
 * rung reshaped, with no fetch and no new rung. Replaced rather than mutated,
 * so redrawStack's restore on a throw undoes it.
 *
 * @param {{tier: number}} d
 */
export function expandTier(d) {
  const at = drilled.length - 1;
  const rung = drilled[at];
  if (!rung) return;
  const expanded = new Set(rung.expanded || []);
  expanded.add(d.tier);
  const before = new Set(laidNodes.filter((n) => n.tier === d.tier).map((n) => n.id));
  redrawStack(drilled.slice(0, at).concat([Object.assign({}, rung, { expanded: expanded })]),
    undefined, () => firstNewAt(d.tier, before));
}

/**
 * The topmost laid mark at `tier` that `before` does not hold, else the
 * topmost at `tier`, else "": where focus goes after a tail is drawn out, so
 * the next key acts in the column the reader expanded rather than on the
 * rung's return control.
 * @param {number} tier
 * @param {Set<string>} before the ids laid at `tier` before the expansion
 * @returns {string}
 */
export function firstNewAt(tier, before) {
  const column = laidNodes.filter((n) => n.tier === tier).sort((a, b) => a.y0 - b.y0);
  const fresh = column.find((n) => !before.has(n.id));
  return fresh ? fresh.id : column.length ? column[0].id : "";
}

/**
 * Folds an expanded column back into its tail: the breadcrumb chip's action.
 * A control rather than a gesture because expanding removed the mark a reader
 * would click, and Escape already means "pop one rung".
 * @param {number} tier
 */
export function collapseTier(tier) {
  const at = drilled.length - 1;
  const rung = drilled[at];
  if (!rung || !rung.expanded || !rung.expanded.has(tier)) return;
  const expanded = new Set(rung.expanded);
  expanded.delete(tier);
  redrawStack(drilled.slice(0, at).concat([Object.assign({}, rung, { expanded: expanded })]),
    "That column could not be folded back");
}

/**
 * Runs a reader's gesture and turns anything it throws into a refusal banner.
 *
 * A DOM LISTENER IS THE ONE PLACE A THROW REACHES NOBODY: without this a throw
 * leaves the page mid-render with no banner. It validates nothing; it only
 * decides where the failure lands.
 *
 * @template T
 * @param {string} gesture  what the reader did, for the sentence
 * @param {() => T} run
 * @returns {T | undefined}
 */
export function guarded(gesture, run) {
  try {
    return run();
  } catch (e) {
    fail("This page could not " + gesture + ": " + String(e) +
      ". The chart on screen is unchanged; the file it was drawn from is most likely " +
      "not one this site published.");
    return undefined;
  }
}

/**
 * One click on a node: it follows that node's money, whether or not it opens.
 *
 * THE ECHO GUARD IS ON THE ACTIVATION AND NOT ON THE DEVICE: a click on the
 * node a key has just activated is that key's own click, synthesised by
 * assistive tech, and would otherwise undo what the key did.
 * @param {LaidNode} d
 * @param {number} at the event's timestamp
 */
export function clickNode(d, at) {
  pin(d);
  if (d.id === keyActivation.id && at - keyActivation.at < ACTIVATION_WINDOW) return;
  const within = d.id === clickIsolate.id && at - clickIsolate.at < ACTIVATION_WINDOW;
  clickIsolate = { id: d.id, at: at, was: within ? clickIsolate.was : filter.isolated };
  setIsolated(filter.isolated === d.id ? "" : d.id);
}

/**
 * A double click on a node: it opens the node, having first put back whatever
 * the two clicks underneath it isolated.
 *
 * THE RECORD IS SPENT WHETHER OR NOT ANYTHING OPENED, so a later double click
 * cannot restore a state two gestures old.
 * @param {LaidNode} d
 * @param {number} at the event's timestamp
 */
export function doubleClickNode(d, at) {
  if (d.id === clickIsolate.id && at - clickIsolate.at < ACTIVATION_WINDOW) {
    setIsolated(clickIsolate.was);
  }
  clickIsolate = { id: "", at: -Infinity, was: "" };
  openOffered(d);
}

/**
 * Opens what activating a node offers, and says whether it offered anything:
 * the step it opens into (drillable), else the folded tail it draws out
 * (expandable). The one rule Enter on a mark, a double click and the
 * search's Enter share.
 * @param {LaidNode} d
 * @returns {boolean}
 */
export function openOffered(d) {
  if (drillable(d)) {
    openNode(d.id);
    return true;
  }
  if (expandable(d)) {
    expandTier(d);
    return true;
  }
  return false;
}

/**
 * Enter or Space on a node: Enter opens a node that opens, and Space follows
 * the money.
 *
 * SPACE NEVER OPENS, against the role="button" convention, so nodeDescription,
 * paintChartHint and aria-keyshortcuts all announce it. Enter falls back to
 * the isolate on a node that does not open, so no button has a dead Enter.
 * @param {LaidNode} d
 * @param {string} key
 * @param {number} at the event's timestamp
 */
export function keyNode(d, key, at) {
  keyActivation = { id: d.id, at: at };
  // Escape unpins while leaving focus where it was, so the panel can be
  // empty here even though focus already pinned this node once.
  pin(d);
  if (key === "Enter" && openOffered(d)) return;
  setIsolated(filter.isolated === d.id ? "" : d.id);
}

/**
 * Puts focus somewhere real after a drill has replaced the focused element.
 *
 * ONLY IF FOCUS WAS ALREADY IN THE CHART, and the caller must say so: by the
 * time this runs the repaint has detached the focused element, so
 * document.activeElement no longer knows.
 *
 * Up a rung, focus returns to the mark the reader opened; where that mark is
 * folded away, to the innermost return control, then the first mark. Down a
 * rung, to the rung's own return control. On the same rung, to the mark the
 * redraw names (a column drawn out of its tail names its first new mark).
 *
 * @param {boolean} hadFocus whether focus was inside the chart before the
 *   repaint that just replaced it.
 * @param {string} [popped] the id of the mark focus lands on first: the node
 *   whose rung this repaint closed, or the one a same-rung redraw names; ""
 *   where it opened a rung
 */
export function restoreFocus(hadFocus, popped = "") {
  if (!hadFocus) return;
  // WITHOUT PINNING: otherwise the next Escape spends itself clearing that pin
  // instead of closing the next rung. The focus handlers ask `restoring`.
  restoring = true;
  try {
    restoreFocusTo(popped);
  } finally {
    restoring = false;
  }
}

/** Whether focus is being restored by restoreFocus rather than moved by the reader. */
let restoring = false;

/**
 * restoreFocus's choice of target, made while `restoring` is set.
 * @param {string} popped
 */
function restoreFocusTo(popped) {
  // focus() is on HTMLElement and SVGElement, not Element; the SVG <g> marks
  // are not obliged to have it, so the runtime test stays.
  const focus = (/** @type {Element | null} */ target) => {
    const el = /** @type {any} */ (target);
    if (!el || typeof el.focus !== "function") return false;
    el.focus();
    return true;
  };
  // THE INNERMOST RUNG'S CONTROL, asked for by class: the bar also holds
  // expansion chips, which do not go back.
  const chart = maybeEl("chart");
  const left = popped && chart
    ? D3.select(chart).selectAll("g.node").filter(/** @param {LaidNode} d */ (d) => d.id === popped).node()
    : null;
  if (focus(left)) return;
  const bar = maybeEl("breadcrumb");
  if (drilled.length && bar) {
    const controls = Array.from(bar.children || []).filter((c) =>
      String(/** @type {any} */ (c).className || "").split(" ").indexOf("crumb-back") >= 0);
    if (focus(controls[controls.length - 1] || null)) return;
  }
  focus(chart ? chart.querySelector("g.node") : null);
}

/**
 * Names the chart for a screen reader, for the rung it is actually on.
 *
 * IT APPENDS TO the served name rather than replacing it; the overview puts
 * both back.
 */
export function paintChartName() {
  // Every rung, outermost first: "opened into General Fund, then Patrol".
  const trail = trailOfRungs().join(", then ");
  const title = maybeEl("chart-title");
  if (title && shownYear && shownYear.chart_title) {
    title.textContent = drilled.length
      ? shownYear.chart_title + ", opened into " + trail
      : shownYear.chart_title;
  }
  const desc = maybeEl("chart-desc");
  if (!desc) return;
  if (!baseDescription) {
    baseDescription = desc.textContent;
    tablePointer = say("table_pointer");
  }
  // THE TABLE POINTER CLOSES EVERY DEPTH'S DESCRIPTION: the closed flow table
  // is out of the accessibility tree, so this sentence is the only route to it.
  // What the columns are is the step's own description, from Go, which
  // requires one on every step.
  if (drilled.length) {
    const step = drilled[drilled.length - 1].step;
    desc.textContent = "Opened into " + trail + ". " + step.description + heldBack() + " " + say("go_back") + " " + tablePointer;
    return;
  }
  desc.textContent = baseDescription;
}

/**
 * The sentence the chart on screen holds back for want of a column, led by a
 * space, or "": a residual this width draws no flow of has no mark to carry
 * its note, so the hint and the description say it.
 * @returns {string}
 */
function heldBack() {
  return projection && projection.held_back ? " " + projection.held_back : "";
}

/**
 * Says what a click does, for the chart on screen.
 *
 * WHETHER ANYTHING OPENS, AND IN WHICH COLUMN, IS ASKED OF THE DRAWN NODES,
 * not of the declared chain: a step can exist below this depth with no node
 * on screen at its `from` tier, and the filters can remove every one there is.
 */
export function paintChartHint() {
  const hint = maybeEl("chart-hint");
  if (!hint || !STEPS.length) return;
  const chart = chartDrawn();
  const anyOpens = Boolean(chart) && chart.nodes.some(drillable);
  const column = anyOpens ? joinOr(openableColumns()) : "";
  // An undeclared column order has no left or right to name: drop the clause.
  const where = column ? say("in_column", { columns: column }) : "";
  // The isolate is named on every view, and Space with it, because Space
  // never opens against the role="button" convention.
  const follow = " " + say("follow");
  // A sentence of its own: the tail can be the only thing a chart offers.
  const tails = Boolean(chart) && chart.nodes.some(expandable);
  const expands = tails ? " " + say("expand") : "";
  const swatch = buildLegendCount() ? " " + say("swatch") : "";
  if (drilled.length) {
    hint.textContent = say("opened_hint", { label: labelOfRung(drilled.length - 1) }) + " " +
      (anyOpens ? say("open_further", { where }) : say("nothing_further")) + follow + expands + swatch + heldBack();
    return;
  }
  hint.textContent = (anyOpens ? say("open_into", { where }) : say("nothing_opens")) + follow + swatch;
}

/**
 * Which columns of the chart on screen hold a node that opens, named left to
 * right, or none when nothing here opens.
 *
 * IN THE DECLARED ORDER AND NOT IN TIER ORDER: layOut aligns on the declared
 * set, and a window's ({2,5,4}) does not ascend. Narrowed to the tiers the
 * chart actually draws, because a declaration need not be filled and the
 * filters can empty one.
 * @returns {string[]}
 */
export function openableColumns() {
  const chart = chartDrawn();
  if (!chart) return [];
  const drawn = new Set(chart.nodes.map((n) => n.tier));
  const tiers = activeTiers().filter((t) => drawn.has(t));
  if (tiers.length < 2) return [];
  const opening = new Set(chart.nodes.filter(drillable).map((n) => n.tier));
  const inner = tiers.length === 3 ? ["column_middle"] : ["column_second", "column_third"];
  const names = tiers.map((tier, at) => {
    if (!opening.has(tier)) return "";
    const key = at === 0 ? "column_left" : at === tiers.length - 1 ? "column_right" : inner[at - 1];
    return key ? say(key) : null;
  });
  // A COLUMN THE WORDING HAS NO NAME FOR DROPS THE CLAUSE, rather than leave a
  // list that reads as every column that opens.
  if (names.includes(null)) return [];
  return /** @type {string[]} */ (names.filter(Boolean));
}

/** How many fund-group swatches the legend is showing. */
export function buildLegendCount() {
  const legend = maybeEl("legend");
  return legend ? legend.children.length : 0;
}

/**
 * Draws the trail back out of a drill: the only way back that is always
 * visible, since opening can remove the node that was clicked.
 */
export function paintBreadcrumb() {
  const bar = maybeEl("breadcrumb");
  if (!bar) return;
  if (!drilled.length) {
    bar.replaceChildren();
    bar.setAttribute("hidden", "");
    return;
  }
  bar.removeAttribute("hidden");
  // One return control per rung, popping to its own depth, in the step's own
  // `back` words rather than any mapping from tier number.
  const controls = drilled.map((rung, k) => {
    const back = h("button", "crumb-back");
    back.textContent = say("back_control", { back: rung.step.back });
    back.setAttribute("type", "button");
    back.addEventListener("click", () => drillUp(k));
    return back;
  });
  const here = h("span", "crumb-here", labelOfRung(drilled.length - 1));
  // One chip per expanded column, the only way to fold it back. Its count is
  // what is drawn, read off the chart, not what the tail said it hid.
  const chips = expandedTiers().map((tier) => {
    const n = columnSize(tier);
    const chip = h("button", "crumb-expanded",
      "showing all " + n + " " + tailNoun(tier) + " ×");
    chip.setAttribute("type", "button");
    // THE GLYPH IS NOT THE LABEL: a screen reader reads "×" as "times".
    chip.setAttribute("aria-label",
      "Showing all " + n + " " + tailNoun(tier) + "; fold the smallest back into one mark");
    chip.addEventListener("click", () => collapseTier(tier));
    return chip;
  });
  // Chips are buttons too, which is why restoreFocus asks by class.
  bar.replaceChildren(...controls, here, ...chips);
}

/**
 * How many marks of its own a drawn column holds: not the carried residual,
 * gap or kept flank placed at that tier.
 * @param {number} tier
 * @returns {number}
 */
export function columnSize(tier) {
  if (!projection) return 0;
  return projection.nodes.filter((n) =>
    n.tier === tier && !isCarried(n.id) && !n.carried_from).length;
}

/**
 * What this rung's step calls the rows of one capped column: the cap's own
 * word where it has one, since one step can cap two columns under two nouns,
 * else the step's, which Go requires. "" on the overview, which caps nothing.
 * @param {number} tier
 * @returns {string}
 */
export function tailNoun(tier) {
  const rung = drilled.length ? drilled[drilled.length - 1] : null;
  if (!rung) return "";
  const cap = formFor(rung.step).caps(rung.step).find((c) => c.tier === tier);
  return (cap && cap.tail) || rung.step.tail;
}

/**
 * The printed label of the node rung k opened, from the document it was opened
 * FROM, not the rung's own: across a document switch both may carry the id
 * under different words, and the reader clicked the first.
 * @param {number} k
 * @returns {string}
 */
export function labelOfRung(k) {
  const rung = drilled[k];
  if (!rung) return "";
  const doc = docAt(k);
  const n = doc ? doc.nodes.find((x) => x.id === rung.id) : null;
  return n ? n.label : rung.id;
}

/**
 * The rungs' names, outermost first, with any two that read alike told apart.
 *
 * THE COLLISION IS THE CITY'S: the printed labels cannot change, so every
 * member of a colliding set is qualified with its step's declared noun.
 * @returns {string[]}
 */
export function trailOfRungs() {
  const words = drilled.map((_, k) => labelOfRung(k));
  return words.map((w, k) => {
    if (!words.some((other, j) => j !== k && other === w)) return w;
    const noun = drilled[k].step && drilled[k].step.noun;
    return noun ? w + " (" + noun + ")" : w;
  });
}

/**
 * The document as this page draws it: the overview, or one node opened.
 *
 * @param {FiscProjection} doc
 * @returns {FiscProjection}
 */
export function shapeFor(doc) {
  const rung = drilled.length ? drilled[drilled.length - 1] : null;
  const from = rung ? docAt(drilled.length - 1) : null;
  return formOnScreen().shape(doc, rung, from, activeTiers());
}

/**
 * The chart on screen's declaration: the innermost rung's step, or the
 * overview.
 * @returns {FiscChart | FiscDrillStep}
 */
export function chartOnScreen() {
  return drilled.length ? drilled[drilled.length - 1].step : CONFIG.overview;
}

/**
 * The renderer that drew the chart on screen: what every reading of a laid
 * mark -- its figure, its share, what its ribbons say -- is asked of.
 * @returns {FormRenderer}
 */
export function formOnScreen() {
  return formFor(chartOnScreen());
}

/**
 * The classes a ribbon is drawn with.
 * @param {LaidLink} d
 * @returns {string}
 */
export function linkClass(d) {
  return "link" + (d.derived ? " derived" : "") + (d.contra ? " contra" : "") +
    (d.partition ? " partition" : "");
}

/**
 * The classes a node is drawn with. `opens` and `expands` are affordances,
 * answered by drillable and expandable rather than read off the node; the two
 * predicates are disjoint, which the class does not enforce.
 * @param {LaidNode} d
 * @returns {string}
 */
export function nodeClass(d) {
  return "node" + (d.derived ? " derived" : "") + (formOnScreen().reduction(d) ? " contra" : "") +
    (drillable(d) ? " opens" : "") + (expandable(d) ? " expands" : "");
}

/**
 * The glyphs one node's label carries in its flag tspan: glyphs, not words,
 * because the label is at the edge of its gutter, and a glyph survives
 * forced-colors and greyscale. They compose, so a pair costs two glyph widths,
 * which the layout test fits by calling this; drillable and expandable are
 * disjoint, so no mark carries three.
 * @param {LaidNode} d
 * @returns {string}
 */
export function nodeFlags(d) {
  const marks = (d.derived ? "\u25c7" : "") + (drillable(d) ? "\u25b8" : "") +
    (expandable(d) ? "\u229e" : "");
  return marks ? "  " + marks : "";
}

/**
 * Whether an id names the residual node, the gap node, or an endpoint carried
 * with the residual: a mark the rung on screen added beside the opened node's
 * parts, which is not one of them and opens into nothing.
 *
 * KEYED ON THE STEP'S DECLARED RESIDUAL ENDPOINTS, NOT carried_from: a
 * window's kept flank is carried too and must still open. Widening this to
 * "anything carried" would refuse every click in a window.
 * @param {string} id
 * @returns {boolean}
 */
export function isCarried(id) {
  if (isResidual(id) || isGap(id)) return true;
  const rung = drilled.length ? drilled[drilled.length - 1] : null;
  return Boolean(rung && rung.step.residual &&
    Object.prototype.hasOwnProperty.call(rung.step.residual, id));
}

/**
 * Lays a shaped document out through its form, binding the page: the hue
 * index is the drawn nodes overwritten by the unfolded document this rung was
 * shaped from, because a colour is where a node really sits and the fold
 * re-points parents; the drawn set contributes only what the file lacks, such
 * as the aggregate. Assigned only once the layout has not thrown, or the next
 * paint() would recolour the chart on screen against a document it was not
 * drawn from.
 * @param {FiscProjection} doc
 * @returns {{nodes:LaidNode[], links:LaidLink[]}}
 */
export function layOut(doc) {
  const { graph, index } = laidOf(doc);
  groupIndex = index;
  // For shareOf: only the laid graph knows a mark's column.
  laidNodes = graph.nodes;
  return graph;
}

/**
 * layOut's layout, assigning nothing: the graph the form lays `doc` out as,
 * and the hue index it was laid against.
 * @param {FiscProjection} doc
 * @returns {{graph: {nodes:LaidNode[], links:LaidLink[]}, index: Map<string, FiscNode>}}
 */
function laidOf(doc) {
  const index = hueIndex(doc);
  // Nothing to lay out where the filters removed every ribbon; d3-sankey
  // throws on an empty graph.
  const graph = !doc.links.length ? { nodes: [], links: [] } : formOnScreen().layOut(doc, {
    tiers: layoutTiers(doc),
    columns: layoutColumns(doc),
    groupOf: (/** @type {FiscNode | LaidNode} */ n) => core.fundGroupOf(index, n),
    placeOf: fundGroupPlace,
  });
  return { graph, index };
}

/**
 * The columns a chart is laid out in: the tiers on screen (activeTiers), less
 * any the filters emptied, since d3-sankey throws on an empty leading or
 * interior column and a filter can empty any of them. A chart the filters
 * did not touch keeps every tier on screen.
 * @param {FiscProjection} doc
 * @returns {number[]}
 */
export function layoutTiers(doc) {
  const tiers = activeTiers();
  if (!doc.filtered) return tiers;
  const held = new Set(doc.nodes.map((n) => n.tier));
  return tiers.filter((t) => held.has(t));
}

/**
 * How many columns wide a chart is laid out: drawnColumns, or for a chart
 * the filters touched, the columns it still fills.
 * @param {FiscProjection} doc
 * @returns {number}
 */
export function layoutColumns(doc) {
  return doc.filtered ? (layoutTiers(doc).length || NARROW_COLUMNS) : drawnColumns();
}

/**
 * layOut's hue index for a document: its nodes overwritten by the unfolded
 * document the rung was shaped from (drawnDoc).
 * @param {FiscProjection} doc
 * @returns {Map<string, FiscNode>}
 */
export function hueIndex(doc) {
  const index = new Map(doc.nodes.map((n) => [n.id, n]));
  const source = drawnDoc();
  if (source) {
    for (const n of source.nodes) index.set(n.id, n);
  }
  return index;
}

/**
 * A shaped chart under a filter state's removals: core.filterDocument with
 * each node's fund group read through the index the chart is coloured by.
 * @param {FiscProjection} doc
 * @param {Readonly<FilterState>} [state]
 * @returns {{doc: FiscProjection, hidden: FiscLink[]}}
 */
export function filterFor(doc, state = filter) {
  const index = hueIndex(doc);
  return core.filterDocument(doc, (n) => core.fundGroupOf(index, n), state);
}

/**
 * The chart the filters leave on screen, or the shaped one where nothing has
 * been drawn under them yet.
 * @returns {FiscProjection | null}
 */
export function chartDrawn() {
  return shown ? shown.doc : projection;
}

/**
 * What the filters removed from the chart on screen: how many ribbons, their
 * figures summed as each is stated (a reduction negative), and every fact
 * they cite. A client sum over cited cents, and the only one the filter line
 * states.
 * @param {{hidden: FiscLink[]} | null} [result]
 * @returns {{flows: number, cents: number, facts: Set<string>}}
 */
export function hiddenTally(result = shown) {
  const facts = new Set();
  let cents = 0;
  const hidden = result ? result.hidden : [];
  for (const l of hidden) {
    cents += formOnScreen().cents(/** @type {any} */ (l));
    for (const id of l.fact_ids) facts.add(id);
  }
  return { flows: hidden.length, cents: cents, facts: facts };
}

/**
 * The ribbons the smallest-flow filter merged on the chart on screen: how
 * many printed ribbons, and how many merged ribbons they are drawn as.
 * @returns {{flows: number, ribbons: number}}
 */
export function foldedTally() {
  let flows = 0;
  let ribbons = 0;
  for (const l of shown ? shown.doc.links : []) {
    if (!Array.isArray(l.legs)) continue;
    flows += l.legs.length;
    ribbons++;
  }
  return { flows: flows, ribbons: ribbons };
}

/**
 * What the filters removed that touched one mark, in the page's words, or ""
 * where they removed nothing of it: a mark is sized by what is drawn, so a
 * reader is told what it is not showing.
 * @param {string} id
 * @returns {string}
 */
export function hiddenAt(id) {
  const touching = (shown ? shown.hidden : []).filter((l) => l.source === id || l.target === id);
  if (!touching.length) return "";
  return say("filter_hidden_at", { cents: fmtSigned(hiddenTally({ hidden: touching }).cents), flows: touching.length });
}

/**
 * A shaped chart laid out as its view draws it with no filter, where the
 * filters change what is drawn of it, else null: `whole` for that chart.
 * @param {FiscProjection} drawn the shaped chart
 * @param {{doc: FiscProjection}} filtered filterFor's answer over it
 * @returns {{nodes: LaidNode[], links: LaidLink[]} | null}
 */
export function wholeOf(drawn, filtered) {
  return filtered.doc === drawn ? null : laidOf(drawn).graph;
}

/**
 * The laid mark a reading of `d`'s figure is asked of: the same node in the
 * chart laid with no filter (whole), else `d` -- a link, a chart the filters
 * do not touch, or a mark only the filters draw.
 * @template {LaidLink | LaidNode} T
 * @param {T} d
 * @returns {T}
 */
export function markOfView(d) {
  if (!whole || isLink(d)) return d;
  const found = whole.nodes.find((n) => n.id === d.id);
  return found ? /** @type {T} */ (/** @type {unknown} */ (found)) : d;
}

/**
 * The figure a mark is said at: the form's reading of it over the view's every
 * ribbon (markOfView), whatever the filters leave drawn.
 * @param {LaidLink | LaidNode} d
 * @returns {number}
 */
export function markFigure(d) {
  return formOnScreen().cents(markOfView(d));
}

/**
 * The part of a node's figure the filters leave drawn, in the page's words,
 * or "" where they draw all of it: said beside the figure, never in its slot.
 * @param {LaidLink | LaidNode} d
 * @returns {string}
 */
export function drawnAt(d) {
  const drawn = formOnScreen().cents(d);
  if (isLink(d) || drawn === markFigure(d)) return "";
  return say("filter_drawn_at", { cents: fmtSigned(drawn) });
}

/**
 * Draws the chart on screen through its form, then paints it and applies the
 * emphasis. The gestures, descriptions and classes are this file's: they read
 * the page's state, which a renderer never holds.
 * @param {{nodes:LaidNode[], links:LaidLink[]}} [laid]
 */
export function render(laid) {
  const doc = chartDrawn();
  if (!doc) return;
  const graph = laid || layOut(doc);
  const svg = D3.select("#chart");
  // The form draws into a fresh group, which is what the zoom moves: never
  // the svg, whose viewBox is the chart's own coordinates.
  svg.selectAll("g").remove();
  const layer = svg.append("g").attr("class", "zoom-layer");
  formOnScreen().render(graph, {
    svg: svg,
    layer: layer,
    tiers: layoutTiers(doc),
    columns: layoutColumns(doc),
    doc: doc,
    classes: { link: linkClass, node: nodeClass, flags: nodeFlags },
    describe: { link: linkDescription, node: nodeDescription },
    figure: markFigure,
    on: {
      tip: showTip, hide: hideTip, pin: pin, guarded: guarded,
      click: clickNode, dblclick: doubleClickNode, key: keyNode,
      restoring: () => restoring,
    },
  });
  paint();
  applyEmphasis();
  // A new chart starts unzoomed: the transform was about the marks it replaced.
  bindZoom();
  // The options name marks of the chart just replaced.
  closeSearch();
}

/**
 * Colours the chart through its form, and the legend's swatches here. Nothing
 * on a page main() refused for its form: the theme toggle is wired before
 * that refusal and repaints through here.
 */
export function paint() {
  if (!CONFIG || !CONFIG.overview || undrawableForm()) return;
  formOnScreen().paint({
    svg: D3.select("#chart"),
    colour: { link: linkColor, node: nodeColor },
  });
  for (const button of document.querySelectorAll("#legend button .key, #filters .key")) {
    const swatch = /** @type {HTMLElement} */ (button);
    const name = swatch.dataset.var;
    if (name) swatch.style.background = cssVar(name);
  }
}

/**
 * Follows one node's or one fund group's money, or clears the isolation for
 * "".
 *
 * Every path into this state goes through here, so the legend's pressed button
 * and the dimming cannot drift apart.
 * @param {string} id
 */
export function setIsolated(id) {
  filter = Object.freeze(Object.assign({}, filter, { isolated: id }));
  for (const element of el("legend").querySelectorAll("button")) {
    const button = /** @type {HTMLElement} */ (element);
    const pressed = id !== "" && button.dataset.group === id;
    button.setAttribute("aria-pressed", String(pressed));
  }
  applyEmphasis();
  recordView("replace");
}

/**
 * Whether an id names a fund group of the chart on screen, by its role.
 * @param {string} id
 * @returns {boolean}
 */
export function isGroupID(id) {
  const n = groupIndex.get(id);
  return Boolean(n) && isFundGroup(n);
}

/**
 * Whether a laid ribbon carries the money isolating `id` follows: an end at
 * that node, or, where `id` is a fund group, the group it is drawn in or the
 * group of either end. ONE RULE FOR EVERY CALLER, swatch or mark: a group's
 * own ribbons are always in it, so following the group never follows less
 * than following its node.
 * @param {string} id
 * @param {LaidLink} l
 * @returns {boolean}
 */
export function follows(id, l) {
  if (l.source.id === id || l.target.id === id) return true;
  if (!isGroupID(id)) return false;
  return linkGroup(l) === id || fundGroupOf(l.source) === id || fundGroupOf(l.target) === id;
}

/**
 * Whether a laid mark is one the isolation of `id` is ABOUT rather than one it
 * reaches by a ribbon: the node itself, or a mark in the fund group `id` names.
 * @param {string} id
 * @param {LaidNode} d
 * @returns {boolean}
 */
export function isolates(id, d) {
  return d.id === id || (isGroupID(id) && fundGroupOf(d) === id);
}

/**
 * Applies a filter state's isolation, and the pinned selection, to every
 * mark. An isolation the chart on screen draws nothing of is inert, not a
 * chart dimmed whole: a group the view lacks, or a node the redraw removed.
 * @param {Readonly<FilterState>} [state]
 */
export function applyEmphasis(state = filter) {
  const svg = D3.select("#chart");
  const id = state.isolated;
  const on = id !== "" && laidNodes.some((n) => isolates(id, n));
  svg.selectAll("path.link").classed("dim", /** @param {LaidLink} d */ (d) => on && !follows(id, d));
  svg.selectAll("path.link").classed("hot", /** @param {LaidLink} d */ (d) => d === pinned);
  svg.selectAll("g.node").classed("dim", /** @param {LaidNode} d */ (d) =>
    on && !isolates(id, d) && !d.sourceLinks.concat(d.targetLinks).some((l) => follows(id, l)));
  svg.selectAll("g.node").attr("aria-pressed", /** @param {LaidNode} d */ (d) =>
    String(on && d.id === id));
}

/**
 * Whether two sets hold the same members.
 * @param {ReadonlySet<string>} a
 * @param {ReadonlySet<string>} b
 * @returns {boolean}
 */
function sameSet(a, b) {
  return a.size === b.size && Array.from(a).every((x) => b.has(x));
}

/**
 * Moves the filter state to `next`, field by field over the state now: a
 * change of what is removed re-lays-out the chart on screen with nothing
 * fetched and no rung touched, and a change of the isolation alone repaints
 * the emphasis. A redraw the chart refuses leaves the old state, chart and
 * panel exactly as they were and says why.
 * @param {Partial<FilterState>} next
 * @returns {boolean} whether the state now on screen is `next`
 */
export function setFilter(next) {
  const was = filter;
  const smallest = next.smallest === undefined ? was.smallest : Math.max(0, Math.floor(Number(next.smallest)) || 0);
  const state = Object.freeze({
    groups: new Set(next.groups || was.groups),
    kinds: new Set(next.kinds || was.kinds),
    smallest: smallest,
    isolated: next.isolated === undefined ? was.isolated : next.isolated,
  });
  if (sameSet(was.groups, state.groups) && sameSet(was.kinds, state.kinds) && was.smallest === state.smallest) {
    setIsolated(state.isolated);
    return true;
  }
  filter = state;
  if (!projection) {
    syncFilterPanel();
    return true;
  }
  const hadFocus = focusInChart();
  let filtered;
  let unfiltered;
  let laid;
  try {
    filtered = filterFor(projection);
    unfiltered = wholeOf(projection, filtered);
    laid = layOut(filtered.doc);
  } catch (e) {
    filter = was;
    syncFilterPanel();
    fail("The chart could not be filtered: " + (e instanceof Error ? e.message : String(e)));
    return false;
  }
  shown = filtered;
  whole = unfiltered;
  pinned = null;
  resetDetail();
  hideTip();
  buildLegend();
  paintFilterLine();
  syncFilterPanel();
  paintChartHint();
  render(laid);
  restoreFocus(hadFocus);
  recordView("replace");
  return true;
}

/** Puts every removal and the isolation back: the filter line's Reset. */
export function resetFilters() {
  setFilter(NO_FILTER);
}

/**
 * Says what the filters leave out of the chart on screen, beside it: the
 * ribbons removed as a client sum over their cited cents, and the ribbons
 * merged by the smallest-flow filter. Hidden where they leave out nothing.
 */
export function paintFilterLine() {
  const line = maybeEl("filter-line");
  const said = maybeEl("filter-said");
  if (!line || !said) return;
  const gone = hiddenTally();
  const merged = foldedTally();
  const parts = [];
  if (gone.flows) parts.push(say("filter_hidden", { cents: fmtSigned(gone.cents), flows: gone.flows }));
  if (merged.flows) {
    parts.push(say("filter_folded", { flows: merged.flows, ribbons: merged.ribbons, threshold: fmt(filter.smallest) }));
  }
  said.textContent = parts.join(" ");
  if (parts.length) line.removeAttribute("hidden");
  else line.setAttribute("hidden", "");
}

/**
 * The words a smallest-flow stop is said in.
 * @param {number} cents
 * @returns {string}
 */
export function smallestWords(cents) {
  return cents > 0 ? fmt(cents) : say("smallest_all");
}

/**
 * The stop at or below `cents`: where the control rests for a threshold set
 * between stops.
 * @param {number} cents
 * @returns {number}
 */
export function smallestStop(cents) {
  let at = 0;
  SMALLEST_STOPS.forEach((stop, k) => {
    if (stop <= cents) at = k;
  });
  return at;
}

/**
 * One labelled checkbox of the filter panel, with a swatch when it is a fund
 * group's. Checked means shown.
 * @param {string} id
 * @param {string} words
 * @param {string} swatch the custom property its swatch wears, or ""
 * @param {(shownNow: boolean) => void} onChange
 * @returns {HTMLElement}
 */
function filterChoice(id, words, swatch, onChange) {
  const label = h("label", "filter-choice");
  const box = /** @type {HTMLInputElement} */ (document.createElement("input"));
  box.type = "checkbox";
  box.value = id;
  box.checked = true;
  box.addEventListener("change", () => onChange(box.checked));
  label.append(box);
  if (swatch) {
    const key = h("span", "key");
    key.dataset.var = swatch;
    key.setAttribute("aria-hidden", "true");
    label.append(key);
  }
  label.append(document.createTextNode(words));
  return label;
}

/**
 * Fills the filter panel's fund groups for the column on screen, in Go's
 * fund_groups order with the hues the legend wears; a group with no hue is
 * last and --muted, as in the legend. Per year, since the column is.
 */
export function buildFilterPanel() {
  const list = maybeEl("filter-group-list");
  if (!list) return;
  list.replaceChildren(...fundGroups().map((g) => filterChoice(g.id, groupLabel(g.id), fundColorVar(g.id),
    (on) => {
      const groups = new Set(filter.groups);
      if (on) groups.delete(g.id);
      else groups.add(g.id);
      setFilter({ groups });
    })));
  syncFilterPanel();
  paint();
}

/**
 * Brings every control of the filter panel back into agreement with the
 * state, so a refused change or a Reset leaves no box saying otherwise.
 */
export function syncFilterPanel() {
  const panel = maybeEl("filters");
  if (!panel) return;
  for (const box of panel.querySelectorAll("#filter-group-list input")) {
    const input = /** @type {HTMLInputElement} */ (box);
    input.checked = !filter.groups.has(input.value);
  }
  for (const box of panel.querySelectorAll("#filter-kind-list input")) {
    const input = /** @type {HTMLInputElement} */ (box);
    input.checked = !filter.kinds.has(input.value);
  }
  const range = /** @type {HTMLInputElement | null} */ (maybeEl("filter-smallest"));
  if (range) {
    range.value = String(smallestStop(filter.smallest));
    range.setAttribute("aria-valuetext", smallestWords(filter.smallest));
  }
  const out = maybeEl("filter-smallest-value");
  if (out) out.textContent = smallestWords(filter.smallest);
}

/**
 * Wires the filter panel the template ships hidden: the flow kinds Go ships
 * labels for, the All and None buttons, the smallest-flow stops and the
 * Reset. Open by default where the viewport buys a column beyond the floor;
 * the reader's own toggle wins after that.
 */
export function wireFilters() {
  const panel = /** @type {HTMLDetailsElement | null} */ (maybeEl("filters"));
  if (!panel) return;
  panel.removeAttribute("hidden");
  const wide = COLUMN_QUERIES.length && typeof window.matchMedia === "function" &&
    window.matchMedia(COLUMN_QUERIES[0].query).matches;
  if (wide) panel.open = true;
  const kinds = maybeEl("filter-kind-list");
  if (kinds) {
    kinds.replaceChildren(...Object.keys(CONFIG.kind_labels || {}).map((k) => filterChoice(k, kindLabel(k), "",
      (on) => {
        const next = new Set(filter.kinds);
        if (on) next.delete(k);
        else next.add(k);
        setFilter({ kinds: next });
      })));
  }
  const all = maybeEl("filter-all");
  if (all) all.addEventListener("click", () => setFilter({ groups: new Set() }));
  const none = maybeEl("filter-none");
  if (none) none.addEventListener("click", () => setFilter({ groups: new Set(fundGroups().map((g) => g.id)) }));
  const range = /** @type {HTMLInputElement | null} */ (maybeEl("filter-smallest"));
  if (range) {
    range.min = "0";
    range.max = String(SMALLEST_STOPS.length - 1);
    range.step = "1";
    range.addEventListener("input", () => setFilter({ smallest: SMALLEST_STOPS[Number(range.value)] || 0 }));
  }
  const reset = maybeEl("filter-reset");
  if (reset) reset.addEventListener("click", resetFilters);
  syncFilterPanel();
}

/* ------------------------------------------------------------------ *
 * Tooltip, detail panel
 * ------------------------------------------------------------------ */

/**
 * A ribbon's provenance phrase.
 *
 * A PRINTED FIGURE RE-POINTED ONTO A MARK OF OURS GETS A THIRD PHRASE: the
 * cents and citation are the city's but the far end is not, so neither
 * "printed by the city" nor "inferred by us" is true of it.
 *
 * @param {boolean} derived  the ribbon's own flag
 * @param {boolean} ontoOurs  whether either end is a mark we drew
 * @returns {string}
 */
export function provenanceOf(derived, ontoOurs) {
  if (derived) return say("inferred_by_us");
  return ontoOurs ? say("carried_note") : say("printed_by_city");
}

/**
 * @param {LaidLink} d
 * @returns {string}
 */
export function linkDescription(d) {
  const kind = kindLabel(d.kind);
  return d.source.label + " to " + d.target.label + ", " + fmtSigned(formOnScreen().cents(d)) +
    (kind ? ", " + kind : "") +
    (d.contra ? ", " + d.contra : "") +
    (d.partition ? ", " + PARTITION_NOTE : "") +
    (d.merged ? ", " + d.merged : "") +
    ", " + provenanceOf(d.derived, Boolean(d.source.derived || d.target.derived));
}

/**
 * @param {LaidNode} d
 * @returns {string}
 */
export function nodeDescription(d) {
  // WHAT EACH GESTURE DOES, for a reader who cannot see the triangle. Space is
  // named only where it differs from Enter; the folded tail's sentence uses the
  // words of the chip that undoes it, since it opens nothing.
  const what = drillable(d) ? say("desc_opens") : expandable(d) ? say("desc_expands") : say("desc_follows");
  const form = formOnScreen();
  const mark = markOfView(d);
  const note = form.reductionNote(mark);
  // The cross-tab qualification, for a reader who cannot see the ribbons.
  const flows = residualFlows(d);
  const drawn = drawnAt(d);
  const gone = hiddenAt(d.id);
  return d.label + (flows ? ", " + flows : ", total " + fmtSigned(markFigure(d))) +
    (drawn ? ", " + drawn : "") +
    ", " + say(d.derived ? "inferred_by_us" : "printed_by_city") +
    (form.crossTab(mark) ? ", " + PARTITION_NOTE : "") +
    (note ? ", " + note.replace(/^\u25c7 /, "") : "") + (gone ? ", " + gone : "") + what;
}

/**
 * Where a caveat's full text is, or "" when this site has no caveats page.
 *
 * LOOKED UP IN THE REFS GO HANDED THE PAGE, never rebuilt from the id: the
 * anchor is per (document, caveat). The refs are the drawn document's -- the
 * step's below depth 0 -- except for a carried mark, whose caveat is the
 * document it was carried from. A stem no document on the stack carries gets
 * no anchor: a missing link is visible, a link to the wrong document is not.
 *
 * @param {string} id
 * @param {string} [carried]  the projection stem the mark this caveat was read
 *   off was carried from, so its anchors are that document's
 * @returns {string}
 */
export function caveatHref(id, carried) {
  const at = carried ? depthOfDocument(carried) : drilled.length;
  if (at < 0) return "";
  const refs = at > 0
    ? (stepDocFor(drilled[at - 1].step) || { caveats: [] }).caveats
    : shownYear ? shownYear.caveats : [];
  if (!Array.isArray(refs)) return "";
  const ref = refs.find((c) => c.id === id);
  return ref && ref.href ? ref.href : "";
}

/**
 * The caveats that are about one drawn mark.
 *
 * RESOLVED THROUGH THE HIERARCHY, not by id: a caveat applies to a drawn node
 * when one of its targets is that node or has it as an ancestor, since a drawn
 * node is often a fold of several ids.
 *
 * @param {string} id
 * @returns {FiscCaveat[]}
 */
export function caveatsFor(id) {
  if (!projection) return [];
  // A carried mark's caveats are the document it was carried from.
  const carried = projection.nodes.find((n) => n.id === id && n.carried_from);
  const source = carried ? carriedSource(carried.carried_from) : projection;
  // Every document is scheduleOf's, which gives metadata a caveats list.
  if (!source) return [];
  // The aggregate folds by value, not ancestry, so the walk cannot reach its ids.
  const drawnNode = projection.nodes.find((n) => n.id === id);
  const swallowed = drawnNode && Array.isArray(drawnNode.folds) ? drawnNode.folds : [];
  const reaches = (/** @type {string} */ target) => {
    if (swallowed.indexOf(target) >= 0) return true;
    let at = groupIndex.get(target);
    for (let hops = 0; at && hops < 9; hops++) {
      if (at.id === id) return true;
      at = at.parent ? groupIndex.get(at.parent) : undefined;
    }
    return false;
  };
  return source.metadata.caveats.filter((c) =>
    Array.isArray(c.applies_to) && c.applies_to.some(reaches));
}

/**
 * Where on the stack the document named by a projection stem sits, or -1.
 * Deepest first: a chain may draw one document at several depths.
 *
 * @param {string} stem
 * @returns {number}
 */
export function depthOfDocument(stem) {
  for (let depth = drilled.length; depth >= 0; depth--) {
    const doc = docAt(depth);
    if (doc && doc.projection === stem) return depth;
  }
  return -1;
}

/**
 * The document a carried mark came from, resolved by the stem it records --
 * not the spine, since a mark two rungs down was carried from the rung above.
 * Null rather than a guess when no document on the stack carries that stem.
 *
 * @param {string} stem
 * @returns {FiscProjection | null}
 */
export function carriedSource(stem) {
  const at = depthOfDocument(stem);
  return at < 0 ? null : docAt(at);
}

/**
 * @param {MouseEvent | FocusEvent} event
 * @param {LaidLink | LaidNode} d
 */
export function showTip(event, d) {
  const tip = el("tooltip");
  tip.replaceChildren();

  const asLink = isLink(d);
  const form = formOnScreen();
  const color = asLink ? linkColor(/** @type {LaidLink} */ (d)) : nodeColor(/** @type {LaidNode} */ (d));

  tip.append(h("div", "tip-value", fmtSigned(markFigure(d))));
  const drawn = drawnAt(d);
  if (drawn) tip.append(h("div", "tip-meta tip-drawn", drawn));

  const label = h("div", "tip-label");
  const key = h("span", "line-key");
  key.style.background = color;
  label.append(key);
  label.append(document.createTextNode(
    asLink
      ? /** @type {LaidLink} */ (d).source.label + " → " + /** @type {LaidLink} */ (d).target.label
      : /** @type {LaidNode} */ (d).label));
  tip.append(label);

  const meta = h("div", "tip-meta");
  if (asLink) {
    const l = /** @type {LaidLink} */ (d);
    const kind = kindLabel(l.kind);
    if (kind) {
      meta.append(h("span", "chip", kind));
      meta.append(document.createTextNode(" "));
    }
    const lentTo = Boolean(l.source.derived || l.target.derived);
    meta.append(h("span", l.derived || lentTo ? "chip derived" : "chip",
      say(l.derived ? "inferred_chip" : lentTo ? "carried_chip" : "printed_chip")));
    if (l.contra) {
      meta.append(document.createTextNode(" "));
      meta.append(h("span", "chip contra", "reduction"));
    }
    if (l.partition) {
      meta.append(document.createTextNode(" "));
      meta.append(h("span", "chip partition", "cross-tab"));
    }
    tip.append(meta);
    // The chip says what kind of row; the sentence says what it reduces.
    if (l.contra) tip.append(h("div", "tip-meta", l.contra));
    if (l.partition) tip.append(h("div", "tip-meta", PARTITION_NOTE));
    if (l.merged) tip.append(h("div", "tip-meta", l.merged));
    tip.append(h("div", "facts", l.fact_ids.join(" ")));
  } else {
    const n = /** @type {LaidNode} */ (d);
    meta.append(h("span", "chip", n.role.replace(/_/g, " ")));
    if (n.constraint_tier) {
      meta.append(document.createTextNode(" "));
      meta.append(h("span", "chip", "constraint: " + n.constraint_tier));
    }
    meta.append(document.createTextNode(" "));
    meta.append(h("span", n.derived ? "chip derived" : "chip", say(n.derived ? "inferred_chip" : "printed_chip")));
    const share = shareOf(n);
    if (share) {
      meta.append(document.createTextNode(" "));
      // chip derived: a share is computed, never printed.
      meta.append(h("span", "chip derived", share));
    }
    // A caveat about this mark, said at the mark.
    const cavs = caveatsFor(n.id);
    if (cavs.length) {
      meta.append(document.createTextNode(" "));
      meta.append(h("span", "chip caveat", cavs.length === 1
        ? "\u26a0 1 caveat" : "\u26a0 " + cavs.length + " caveats"));
    }
    tip.append(meta);
    const flows = residualFlows(n);
    if (flows) tip.append(h("div", "tip-meta", flows));
    for (const b of balancesOf(n)) tip.append(h("div", "tip-meta", b.words));
    if (n.rationale) tip.append(h("div", "tip-meta", n.rationale));
    const note = form.reductionNote(markOfView(n));
    if (note) tip.append(h("div", "tip-meta", note));
    for (const c of cavs) tip.append(h("div", "tip-meta", "\u26a0 " + c.summary));
    const gone = hiddenAt(n.id);
    if (gone) tip.append(h("div", "tip-meta", gone));
  }
  tip.append(h("div", "tip-meta", "Select for sources."));

  tip.hidden = false;
  const wrap = el("chart").parentElement;
  if (!wrap) return;
  const box = wrap.getBoundingClientRect();
  const target = /** @type {Element} */ (event.target);
  let x = 0;
  let y = 0;
  if (event instanceof MouseEvent) {
    x = event.clientX - box.left + 14;
    y = event.clientY - box.top + 14;
  } else {
    const r = target.getBoundingClientRect();
    x = r.left - box.left + r.width / 2;
    y = r.bottom - box.top + 8;
  }
  tip.style.left = Math.min(Math.max(0, x), Math.max(0, box.width - tip.offsetWidth - 4)) + "px";
  tip.style.top = Math.min(y, Math.max(0, box.height - tip.offsetHeight - 4)) + "px";
}

export function hideTip() {
  el("tooltip").hidden = true;
}

/**
 * Pins a flow or node into the detail panel, where the citations are
 * clickable.
 * @param {LaidLink | LaidNode} d
 */
export function pin(d) {
  pinned = d;
  const panel = el("detail");
  panel.replaceChildren();
  if (!projection) return;

  const asLink = isLink(d);
  const form = formOnScreen();

  panel.append(h("div", "amount", fmtSigned(markFigure(d))));
  const drawn = drawnAt(d);
  if (drawn) panel.append(h("p", "subtle", drawn));
  panel.append(h("div", "", asLink
    ? /** @type {LaidLink} */ (d).source.label + " → " + /** @type {LaidLink} */ (d).target.label
    : /** @type {LaidNode} */ (d).label));

  const chips = h("div", "prov");
  if (asLink) {
    const l = /** @type {LaidLink} */ (d);
    const kind = kindLabel(l.kind);
    if (kind) chips.append(h("span", "chip", kind));
    const lent = Boolean(l.source.derived || l.target.derived);
    chips.append(h("span", l.derived || lent ? "chip derived" : "chip",
      say(l.derived ? "our_inference" : lent ? "carried_chip" : "printed_by_city")));
    if (l.contra) chips.append(h("span", "chip contra", "reduction"));
    if (l.partition) chips.append(h("span", "chip partition", "cross-tab"));
    panel.append(chips);
    if (l.contra) panel.append(h("p", "why", l.contra));
    if (l.partition) panel.append(h("p", "why", PARTITION_NOTE));
    if (l.merged) panel.append(h("p", "why", l.merged));
    // A derived ribbon with no fact of its own speaks in its derived end's words.
    const end = l.source.derived ? l.source : l.target;
    if (l.fact_ids.length) panel.append(h("div", "facts", "Facts: " + l.fact_ids.join(" ")));
    else if (l.derived && end.source_note) panel.append(h("p", "subtle", end.source_note));
  } else {
    const n = /** @type {LaidNode} */ (d);
    chips.append(h("span", "chip", n.role.replace(/_/g, " ")));
    if (n.constraint_tier) chips.append(h("span", "chip", "constraint: " + n.constraint_tier));
    chips.append(h("span", n.derived ? "chip derived" : "chip", say(n.derived ? "our_inference" : "printed_by_city")));
    const share = shareOf(n);
    if (share) chips.append(h("span", "chip derived", share));
    panel.append(chips);
    const flows = residualFlows(n);
    if (flows) panel.append(h("p", "subtle", flows));
    // A PRINTED BALANCE IS CITED AT ITSELF, as a link is: its fact and its
    // pages, not the document's.
    for (const b of balancesOf(n)) {
      const said = h("p", "subtle", b.words + " ");
      said.append(h("span", "facts", "Fact: " + b.fact_id));
      for (const c of citations(b.locators)) {
        said.append(document.createTextNode(" "));
        said.append(link(c.label, c.href));
      }
      panel.append(said);
    }
    if (n.rationale) panel.append(h("p", "why", n.rationale));
    if (n.source_note) panel.append(h("p", "subtle", n.source_note));
    const note = form.reductionNote(markOfView(n));
    if (note) panel.append(h("p", "why", note));
    const gone = hiddenAt(n.id);
    if (gone) panel.append(h("p", "subtle", gone));
    for (const c of caveatsFor(n.id)) {
      const why = h("p", "why");
      why.append(document.createTextNode("\u26a0 " + c.summary + " "));
      const href = caveatHref(c.id, n.carried_from);
      if (href) why.append(link("Read it in full", href));
      panel.append(why);
    }
  }

  // THE CITATIONS ARE THE MARK'S OWN WHEN IT HAS ANY: a link's locators, else
  // the sources of the document the mark is OF -- for a carried mark or the
  // residual, the chart above, not the drawn one.
  const prov = h("div", "prov");
  prov.append(h("span", "subtle", "Sources:"));
  const mark = asLink ? null : /** @type {LaidNode} */ (d);
  // The residual records no stem; its links came from the chart the rung was
  // opened from.
  const of = mark && mark.carried_from ? carriedSource(mark.carried_from)
    : mark && isResidual(mark.id) ? docAt(drilled.length - 1)
      : null;
  const ofDocument = (of && of.metadata && of.metadata.sources) || projection.metadata.sources;
  const cited = asLink ? /** @type {LaidLink} */ (d).locators : (mark && mark.locators) || ofDocument;
  for (const c of citations(cited)) {
    prov.append(link(c.label, c.href));
  }
  panel.append(prov);
  applyEmphasis();
}

/* ------------------------------------------------------------------ *
 * Legend, derived list, table
 * ------------------------------------------------------------------ */

/**
 * The fund groups the laid marks are in, in Go's order; one the column does
 * not order sorts last, by id.
 * @returns {string[]}
 */
export function drawnGroups() {
  const present = new Set();
  for (const n of laidNodes) {
    const g = fundGroupOf(n);
    if (g) present.add(g);
  }
  const place = (/** @type {string} */ id) => {
    const at = fundGroupPlace(id);
    return at < 0 ? Infinity : at;
  };
  return Array.from(present).sort((a, b) => place(a) - place(b) || (a < b ? -1 : a > b ? 1 : 0));
}

/**
 * A fund group's printed label: off the documents the chart was laid from,
 * else the column's node table, else its id.
 * @param {string} id
 * @returns {string}
 */
export function groupLabel(id) {
  const n = groupIndex.get(id) ||
    (column && Array.isArray(column.nodes) ? column.nodes.find((x) => x && x.id === id) : null);
  return n && n.label ? n.label : id;
}

/**
 * Draws one swatch per fund group the chart on screen draws a mark in, each a
 * toggle for following that GROUP's money (follows). On every view: a
 * category window or an endpoint's view colours funds of several groups with
 * no group node on screen, and the swatch is the only key to those hues.
 */
export function buildLegend() {
  if (!projection) return;
  const legend = el("legend");
  legend.replaceChildren();
  for (const id of drawnGroups()) {
    const button = document.createElement("button");
    button.type = "button";
    button.dataset.group = id;
    button.setAttribute("aria-pressed", String(filter.isolated === id));
    const key = h("span", "key");
    key.dataset.var = fundColorVar(id);
    button.append(key);
    button.append(document.createTextNode(groupLabel(id)));
    button.addEventListener("click", () => setIsolated(filter.isolated === id ? "" : id));
    legend.append(button);
  }
}

export function buildDerivedList() {
  if (!projection) return;
  const list = el("derived-list");
  list.replaceChildren();
  const nodes = projection.nodes.filter((n) => n.derived);
  const links = projection.links.filter((l) => l.derived);
  const labels = new Map(projection.nodes.map((n) => [n.id, n.label]));

  for (const n of nodes) {
    const li = document.createElement("li");
    li.append(h("div", "what", "◇ " + n.label));
    if (n.rationale) li.append(h("div", "why", n.rationale));
    if (n.source_note) li.append(h("div", "subtle", n.source_note));
    const flows = links.filter((l) => homeOf(l) === n.id);
    if (flows.length) {
      const total = flows.reduce((sum, l) => sum + l.value_cents, 0);
      // "inferred": a residual's own note counts the flows it CARRIES, a
      // different set, and an unqualified count would read as correcting it.
      li.append(h("div", "subtle",
        flows.length + " inferred flow" + (flows.length === 1 ? "" : "s") + " totalling " + fmt(total) + ": " +
        flows.map((l) => (labels.get(l.source) || l.source) + " → " + (labels.get(l.target) || l.target)).join("; ")));
    }
    list.append(li);
  }
  // A link can be derived while both its endpoints are printed.
  const orphans = links.filter((l) => homeOf(l) === "");
  for (const l of orphans) {
    const li = document.createElement("li");
    li.append(h("div", "what", "◇ " +
      (labels.get(l.source) || l.source) + " → " + (labels.get(l.target) || l.target)));
    li.append(h("div", "why", say("flow_inferred")));
    li.append(h("div", "subtle", fmt(l.value_cents)));
    list.append(li);
  }

  if (!nodes.length && !orphans.length) {
    list.append(h("li", "subtle", say("none_inferred")));
  }
}

/**
 * Writes rows tableRows already built, in one swap.
 *
 * Kept apart from building them so a throw while building leaves the old table
 * whole rather than half-built under another year's heading (fisc-bsg).
 */
export function buildTable(rows) {
  const body = el("flow-table").querySelector("tbody");
  if (!body) return;
  body.replaceChildren(...rows);
}

/* ------------------------------------------------------------------ *
 * Search
 * ------------------------------------------------------------------ */

/**
 * The marks of the chart drawn whose label holds `query`, case-folded: those
 * whose label starts with it first, then in label order. Only what is laid
 * out, so a mark the view does not draw -- another rung's, or one the filters
 * removed -- is never offered. None for a blank query.
 * @param {string} query
 * @returns {LaidNode[]}
 */
export function searchMatches(query) {
  const q = String(query).trim().toLocaleLowerCase();
  if (!q) return [];
  const folded = (/** @type {LaidNode} */ n) => String(n.label).toLocaleLowerCase();
  return laidNodes.filter((n) => folded(n).includes(q)).sort((a, b) =>
    Number(!folded(a).startsWith(q)) - Number(!folded(b).startsWith(q)) ||
    folded(a).localeCompare(folded(b)) || (a.id < b.id ? -1 : a.id > b.id ? 1 : 0));
}

/**
 * What one option says: the mark's label, its role where another option reads
 * alike, and what Enter does where the chart offers more than following it.
 * @param {LaidNode} n
 * @param {LaidNode[]} hits every option listed
 * @returns {string}
 */
export function searchOptionWords(n, hits) {
  const alike = hits.some((o) => o !== n && o.label === n.label);
  const offer = drillable(n) ? say("search_opens") : expandable(n) ? say("search_expands") : "";
  return n.label + (alike ? " (" + String(n.role).replace(/_/g, " ") + ")" : "") + (offer ? ", " + offer : "");
}

/** The search's options as listed, and the one the arrows are on (-1 for none). */
export let searchState = { hits: /** @type {LaidNode[]} */ ([]), active: -1 };

/**
 * Lists the marks matching the search box's text, says how many, and puts the
 * arrows on none of them.
 */
export function paintSearch() {
  const input = /** @type {HTMLInputElement | null} */ (maybeEl("search-input"));
  const list = maybeEl("search-list");
  const status = maybeEl("search-status");
  if (!input || !list) return;
  const hits = searchMatches(input.value);
  searchState = { hits, active: -1 };
  list.replaceChildren(...hits.map((n, k) => {
    const option = h("li", "search-option", searchOptionWords(n, hits));
    option.id = "search-option-" + k;
    option.setAttribute("role", "option");
    option.setAttribute("aria-selected", "false");
    option.dataset.id = n.id;
    option.addEventListener("click", () => guarded("find this mark", () => chooseSearch(n.id, false)));
    return option;
  }));
  const open = hits.length > 0;
  if (open) list.removeAttribute("hidden");
  else list.setAttribute("hidden", "");
  input.setAttribute("aria-expanded", String(open));
  input.removeAttribute("aria-activedescendant");
  if (status) {
    status.textContent = !input.value.trim() ? ""
      : open ? say("search_count", { n: hits.length }) : say("search_none", { query: input.value.trim() });
  }
}

/**
 * Moves the arrows one option along, wrapping, and opens the list where it is
 * closed.
 * @param {number} delta 1 or -1
 */
export function moveSearch(delta) {
  const input = maybeEl("search-input");
  const list = maybeEl("search-list");
  if (!input || !list) return;
  if (list.hasAttribute("hidden")) paintSearch();
  const n = searchState.hits.length;
  if (!n) return;
  const at = searchState.active < 0 ? (delta > 0 ? 0 : n - 1) : (searchState.active + delta + n) % n;
  searchState = { hits: searchState.hits, active: at };
  Array.from(list.children).forEach((option, k) => option.setAttribute("aria-selected", String(k === at)));
  const chosen = list.children[at];
  input.setAttribute("aria-activedescendant", chosen.id);
  const scroll = /** @type {any} */ (chosen).scrollIntoView;
  if (typeof scroll === "function") scroll.call(chosen, { block: "nearest" });
}

/** Closes the search's list, leaving the text the reader typed. */
export function closeSearch() {
  const input = maybeEl("search-input");
  const list = maybeEl("search-list");
  if (!input || !list) return;
  list.replaceChildren();
  list.setAttribute("hidden", "");
  input.setAttribute("aria-expanded", "false");
  input.removeAttribute("aria-activedescendant");
  searchState = { hits: [], active: -1 };
}

/**
 * Takes the reader to one mark of the chart drawn. With `open`, the search's
 * Enter, it opens what activating the mark offers (openOffered), the rule
 * the chart's own Enter and double click use; otherwise, or where nothing
 * opens, it follows the mark's money and focuses it, which scrolls it into
 * view and pins it. Says whether the mark was on the chart.
 * @param {string} id
 * @param {boolean} open
 * @returns {boolean}
 */
export function chooseSearch(id, open) {
  const input = /** @type {HTMLInputElement | null} */ (maybeEl("search-input"));
  const d = laidNodes.find((n) => n.id === id);
  closeSearch();
  if (!d) return false;
  if (input) input.value = d.label;
  if (open && openOffered(d)) {
    if (input) input.value = "";
    return true;
  }
  setIsolated(d.id);
  revealMark(d);
  const chart = maybeEl("chart");
  const mark = chart ? D3.select(chart).selectAll("g.node").filter(/** @param {LaidNode} n */ (n) => n.id === d.id).node() : null;
  if (mark && typeof mark.scrollIntoView === "function") mark.scrollIntoView({ block: "nearest", inline: "nearest" });
  if (mark && typeof mark.focus === "function") mark.focus();
  return true;
}

/**
 * Wires the search box the template ships hidden: typing lists the marks of
 * the chart drawn, the arrows move along them, Enter takes the reader to the
 * one the arrows are on, and Escape closes the list, then clears the box,
 * without closing a rung.
 */
export function wireSearch() {
  const box = maybeEl("search");
  const input = /** @type {HTMLInputElement | null} */ (maybeEl("search-input"));
  const list = maybeEl("search-list");
  if (!box || !input || !list) return;
  box.removeAttribute("hidden");
  input.addEventListener("input", () => paintSearch());
  input.addEventListener("keydown", (e) => {
    if (e.key === "ArrowDown" || e.key === "ArrowUp") {
      e.preventDefault();
      moveSearch(e.key === "ArrowDown" ? 1 : -1);
      return;
    }
    if (e.key === "Enter") {
      const chosen = searchState.hits[searchState.active];
      if (!chosen) return;
      e.preventDefault();
      guarded("open this mark", () => chooseSearch(chosen.id, true));
      return;
    }
    if (e.key === "Escape") {
      // The document's Escape closes a rung; in the search box it closes the
      // list, then empties the box.
      e.stopPropagation();
      if (!list.hasAttribute("hidden")) closeSearch();
      else {
        input.value = "";
        paintSearch();
      }
    }
  });
  // A click on an option would blur the box first and close the list under it.
  list.addEventListener("mousedown", (e) => e.preventDefault());
  input.addEventListener("blur", () => closeSearch());
}

/* ------------------------------------------------------------------ *
 * Theme
 * ------------------------------------------------------------------ */

/**
 * Whether the page is dark: the stamped theme wins, then the OS setting.
 * matchMedia is missing in some headless renderers, hence the check.
 * @returns {boolean}
 */
export function prefersDark() {
  const stamped = document.documentElement.dataset.theme;
  if (stamped === "dark") return true;
  if (stamped === "light") return false;
  return typeof window.matchMedia === "function" &&
    window.matchMedia("(prefers-color-scheme: dark)").matches;
}

/**
 * Brings the theme button's label and aria-pressed back into agreement with the
 * page. File-scope so main()'s OS-theme listener can call it too; without that
 * an OS switch leaves the button announcing the opposite of what it does.
 */
export function syncTheme() {
  const button = maybeEl("theme-toggle");
  if (!button) return;
  const dark = prefersDark();
  button.setAttribute("aria-pressed", String(dark));
  button.textContent = dark ? "Light mode" : "Dark mode";
}

export function wireTheme() {
  const button = /** @type {HTMLButtonElement} */ (el("theme-toggle"));
  const sync = syncTheme;
  button.addEventListener("click", () => {
    const dark = !prefersDark();
    document.documentElement.dataset.theme = dark ? "dark" : "light";
    try {
      localStorage.setItem("fisc-theme", dark ? "dark" : "light");
    } catch (e) { /* private mode: the toggle still works for this visit */ }
    sync();
    paint();
  });
  sync();
}

/**
 * How many columns the reader's window has room for, never fewer than the
 * floor; without matchMedia, the floor.
 * @returns {number}
 */
export function viewportColumns() {
  if (typeof window.matchMedia !== "function") return NARROW_COLUMNS;
  let most = NARROW_COLUMNS;
  for (const q of COLUMN_QUERIES) {
    if (q.columns > most && window.matchMedia(q.query).matches) most = q.columns;
  }
  return most;
}

/**
 * The reader's saved choice, or null when there is none this build can honour.
 *
 * An out-of-range value is discarded, not clamped: clamping would deafen the
 * page to the viewport on behalf of a reader who never chose that width.
 * localStorage throws in some privacy modes.
 * @returns {number | null}
 */
export function savedColumns() {
  try {
    const raw = localStorage.getItem("fisc-columns");
    if (raw === null) return null;
    const n = Math.floor(Number(raw));
    if (!Number.isFinite(n) || n < NARROW_COLUMNS || n > OFFERED_COLUMNS) return null;
    return n;
  } catch (e) {
    return null;
  }
}

/**
 * Brings the +/- control back into agreement with the budget. Disabled at the
 * bounds is how the floor is discoverable.
 */
export function syncColumns() {
  const count = maybeEl("column-count");
  if (count) count.textContent = drawnColumns() + " columns";
  const bound = (/** @type {string} */ id, /** @type {boolean} */ atBound) => {
    const button = maybeEl(id);
    if (!button) return;
    if (atBound) button.setAttribute("disabled", "");
    else button.removeAttribute("disabled");
  };
  // Would it move THIS CHART, not the budget: only a step declaring `widen`
  // has a second width.
  const had = document.activeElement;
  bound("column-fewer", nextBudget(-1) === null);
  bound("column-more", nextBudget(1) === null);
  // A disabled button drops focus to <body>, so the sibling takes it.
  const pair = [maybeEl("column-fewer"), maybeEl("column-more")];
  const at = had ? pair.indexOf(/** @type {HTMLElement} */ (had)) : -1;
  const other = at < 0 ? null : pair[1 - at];
  if (at >= 0 && had.hasAttribute("disabled") && other && !other.hasAttribute("disabled")) other.focus();
}

/**
 * Puts the wanted budget into effect and repaints the chart if that moved it.
 *
 * It asks drawnColumns, not columnBudget, whether to redraw: the overview is
 * drawn at RENDER_TIERS whatever the budget, and a needless redraw clears the
 * reader's pin and isolation.
 *
 * @param {boolean} redraw false during boot, where there is no document yet
 * @returns {boolean} false when the redraw was refused
 */
export function applyColumns(redraw) {
  const before = drawnColumns();
  const budget = columnBudget;
  const moved = setColumnBudget(fullscreenColumns !== null ? fullscreenColumns
    : columnOverride === null ? viewportColumns() : columnOverride);
  let drew = true;
  // A REFUSED REDRAW LEAVES THE OLD CHART, so the budget goes back with it.
  if (moved && redraw && projection && drawnColumns() !== before &&
      !redrawStack(drilled, "The chart could not be redrawn at " + drawnColumns() + " columns")) {
    columnBudget = budget;
    drew = false;
  }
  syncColumns();
  return drew;
}

/**
 * The nearest budget in the direction of `delta` at which the chart on screen
 * draws a different number of columns, or null. A budget wider than the chart
 * draws what the chart's own width does, so one step of the budget can move
 * nothing: a reader on a four-column window at a budget of five would find
 * the minus doing nothing.
 *
 * @param {number} delta -1 or 1
 * @returns {number | null}
 */
export function nextBudget(delta) {
  const now = drawnColumns();
  for (let want = columnBudget + delta; want >= NARROW_COLUMNS && want <= OFFERED_COLUMNS; want += delta) {
    if (drawnColumns(want) !== now) return want;
  }
  return null;
}

/**
 * Takes the reader's step, records it as theirs so the next media change does
 * not undo it, and repaints.
 *
 * @param {number} delta
 */
export function stepColumns(delta) {
  const want = nextBudget(delta);
  if (want === null) return;
  // Stepping back to the viewport's own budget releases the override. The
  // budget and not what this chart draws: a four-column chart draws the same
  // at four and five, and the reader's four still matters on a wider one.
  const released = want === viewportColumns();
  const override = columnOverride;
  columnOverride = released ? null : want;
  // A REFUSED STEP IS NOT THE READER'S CHOICE: kept, it would fail on every visit.
  if (!applyColumns(true)) {
    columnOverride = override;
    return;
  }
  try {
    if (released) localStorage.removeItem("fisc-columns");
    else localStorage.setItem("fisc-columns", String(want));
  } catch (e) { /* private mode: the choice still holds for this visit */ }
}

/**
 * Wires the column control and the viewport queries that move it.
 *
 * It sets the budget and does not draw: main() calls it before the first
 * fetch, when a redraw would banner "no document to open" at a reader who has
 * done nothing.
 */
export function wireColumns() {
  // style.css's --chart-room reads this; unset, it falls back to 100%.
  document.documentElement.style.setProperty("--chart-max", CHART_MAX + "px");
  columnOverride = savedColumns();
  const fewer = maybeEl("column-fewer");
  const more = maybeEl("column-more");
  if (fewer && more) {
    fewer.removeAttribute("disabled");
    more.removeAttribute("disabled");
    fewer.addEventListener("click", () => stepColumns(-1));
    more.addEventListener("click", () => stepColumns(1));
  }
  if (typeof window.matchMedia === "function") {
    for (const q of COLUMN_QUERIES) {
      window.matchMedia(q.query).addEventListener("change", () => applyColumns(true));
    }
  }
  applyColumns(false);
}

/* ------------------------------------------------------------------ *
 * Full screen
 * ------------------------------------------------------------------ */

/**
 * The column budget the chart's card was given on going full screen, asked
 * of the full-screen width, or null while it is not full screen. While set it
 * wins over the reader's choice and the viewport's (applyColumns); leaving
 * full screen clears it, which puts theirs back.
 * @type {number | null}
 */
export let fullscreenColumns = null;

/**
 * The most columns, NARROW_COLUMNS to OFFERED_COLUMNS, that fit `px` of
 * width: COLUMN_QUERIES' thresholds asked of a width rather than of a media
 * query, since a full-screen element has no query of its own.
 * @param {number} px
 * @returns {number}
 */
export function columnsForWidth(px) {
  let most = NARROW_COLUMNS;
  for (const q of COLUMN_QUERIES) {
    if (q.columns > most && px >= widthOf(q.columns) + CHART_CUSHION) most = q.columns;
  }
  return most;
}

/**
 * Whether the chart's card is the element the document shows full screen.
 * @returns {boolean}
 */
export function isFullscreen() {
  const card = maybeEl("chart-card");
  return Boolean(card) && document.fullscreenElement === card;
}

/**
 * Follows the document in or out of full screen, however it got there: the
 * button, Escape, or the browser's own control. In, the budget is re-asked at
 * the full-screen width; out, it is given back; either way the chart redraws
 * where that moves it, and the button says which way it goes now.
 */
export function onFullscreenChange() {
  fullscreenColumns = isFullscreen() ? columnsForWidth(window.innerWidth) : null;
  const button = maybeEl("chart-fullscreen");
  if (button) button.textContent = say(isFullscreen() ? "full_screen_exit" : "full_screen");
  applyColumns(true);
}

/**
 * The button's gesture: the card into full screen, or the document out of it.
 * A refusal by the browser changes nothing, and fullscreenchange never fires.
 * @returns {Promise<void>}
 */
export function toggleFullscreen() {
  const card = maybeEl("chart-card");
  if (!card) return Promise.resolve();
  const asked = isFullscreen() ? document.exitFullscreen() : card.requestFullscreen();
  return Promise.resolve(asked).catch(() => {});
}

/**
 * Shows the full-screen button where the browser offers full screen and wires
 * it; elsewhere it stays hidden, as the template ships it.
 */
export function wireFullscreen() {
  const button = maybeEl("chart-fullscreen");
  const card = maybeEl("chart-card");
  if (!button || !card || !document.fullscreenEnabled || typeof card.requestFullscreen !== "function") return;
  button.textContent = say("full_screen");
  button.removeAttribute("hidden");
  button.addEventListener("click", () => { void toggleFullscreen(); });
  document.addEventListener("fullscreenchange", onFullscreenChange);
}

/* ------------------------------------------------------------------ *
 * Zoom and pan
 * ------------------------------------------------------------------ */

/** The furthest the chart zooms in; 1 is the chart whole, and it zooms out no further. */
export const ZOOM_MAX = 8;

/** How far one press of + or - zooms. */
export const ZOOM_STEP = 1.5;

/**
 * The d3.zoom behaviour bound to the chart drawn, or null before one is.
 * Rebound by every render, which is what resets the transform.
 * @type {any}
 */
export let zoomer = null;

/**
 * The chart's zoom transform now, the identity before any zoom.
 * @returns {{k: number, x: number, y: number, invert: (p: number[]) => number[], apply: (p: number[]) => number[]}}
 */
export function zoomOf() {
  const svg = maybeEl("chart");
  return svg ? D3.zoomTransform(svg) : D3.zoomIdentity;
}

/**
 * Whether d3.zoom takes this event. A wheel zooms only with Ctrl or Cmd held
 * (a trackpad's pinch arrives as one) or in full screen, and one finger pans
 * only once zoomed or in full screen, so the page still scrolls past a chart
 * nobody asked to zoom. Two fingers always pinch. Otherwise d3's own rule: no
 * secondary button, no Ctrl-click.
 * @param {any} event
 * @returns {boolean}
 */
export function zoomFilter(event) {
  if (event.type === "wheel") return Boolean(event.ctrlKey || event.metaKey) || isFullscreen();
  if (event.type === "touchstart") {
    return (event.touches && event.touches.length > 1) || zoomOf().k > 1 || isFullscreen();
  }
  return !event.ctrlKey && !event.button;
}

/**
 * Moves the chart's layer to a transform: the marks scale and pan, the labels
 * counter-scale through --label-scale so they stay their own size, the svg
 * clips what the zoom pushes past its edge, and Reset view is offered only
 * where there is something to reset.
 * @param {{k: number, toString: () => string}} t
 */
export function applyZoom(t) {
  const svg = D3.select("#chart");
  svg.select("g.zoom-layer").attr("transform", t.toString());
  const node = svg.node();
  if (!node) return;
  node.style.setProperty("--label-scale", String(1 / t.k));
  node.classList.toggle("zoomed", t.k !== 1 || t.x !== 0 || t.y !== 0);
  const reset = maybeEl("chart-zoom-reset");
  if (reset) {
    if (t.k === 1 && t.x === 0 && t.y === 0) reset.setAttribute("disabled", "");
    else reset.removeAttribute("disabled");
  }
}

/**
 * Binds d3.zoom to the chart just drawn, over its viewBox, and puts it at the
 * identity. The extent is stated, not d3's default, which reads the viewBox
 * through SVG geometry; the translate extent keeps the chart on the canvas.
 * A double click opens or follows a node, so it never zooms.
 */
export function bindZoom() {
  const svg = D3.select("#chart");
  const box = String(svg.attr("viewBox") || "").split(/\s+/).map(Number);
  if (box.length !== 4 || !box.every(Number.isFinite)) return;
  const extent = [[box[0], box[1]], [box[0] + box[2], box[1] + box[3]]];
  zoomer = D3.zoom()
    .scaleExtent([1, ZOOM_MAX])
    .extent(extent)
    .translateExtent(extent)
    .filter(zoomFilter)
    .on("zoom", (/** @type {any} */ e) => applyZoom(e.transform));
  svg.call(zoomer).on("dblclick.zoom", null);
  zoomer.transform(svg, D3.zoomIdentity);
}

/**
 * Zooms by a factor about the chart's centre; a no-op before a chart is drawn.
 * @param {number} factor
 */
export function zoomBy(factor) {
  if (zoomer) zoomer.scaleBy(D3.select("#chart"), factor);
}

/** Puts the chart back whole: Reset view, and the 0 key. */
export function resetZoom() {
  if (zoomer) zoomer.transform(D3.select("#chart"), D3.zoomIdentity);
}

/**
 * Pans a laid node to the centre of the view where the zoom has left it out
 * of sight, and leaves the view alone where it is in sight, so a click on a
 * visible mark never moves the chart under the pointer.
 * @param {LaidNode} d
 */
export function revealMark(d) {
  if (!zoomer) return;
  const svg = maybeEl("chart");
  if (!svg) return;
  const t = zoomOf();
  if (t.k === 1 && t.x === 0 && t.y === 0) return;
  const extent = zoomer.extent().call(svg);
  const [left, top] = t.invert(extent[0]);
  const [right, bottom] = t.invert(extent[1]);
  const cx = (d.x0 + d.x1) / 2;
  const cy = (d.y0 + d.y1) / 2;
  if (cx >= left && cx <= right && cy >= top && cy <= bottom) return;
  zoomer.translateTo(D3.select(svg), cx, cy);
}

/**
 * Wires what zooms the chart besides the pointer: + and - and 0 while focus
 * is in the chart, the Reset view button, and a mark the keyboard focuses
 * panned into sight. Once, at boot: the svg outlives every render.
 */
export function wireZoom() {
  const svg = maybeEl("chart");
  if (!svg) return;
  svg.addEventListener("keydown", (e) => {
    if (e.ctrlKey || e.metaKey || e.altKey) return;
    if (e.key === "+" || e.key === "=") zoomBy(ZOOM_STEP);
    else if (e.key === "-" || e.key === "_") zoomBy(1 / ZOOM_STEP);
    else if (e.key === "0") resetZoom();
    else return;
    e.preventDefault();
  });
  svg.addEventListener("focusin", (e) => {
    const d = /** @type {any} */ (e.target).__data__;
    if (d && typeof d.x0 === "number" && d.sourceLinks) revealMark(d);
  });
  const reset = maybeEl("chart-zoom-reset");
  if (reset) {
    reset.textContent = say("zoom_reset");
    reset.removeAttribute("hidden");
    reset.addEventListener("click", resetZoom);
  }
}

/* ------------------------------------------------------------------ *
 * The view in the URL
 * ------------------------------------------------------------------ */

/**
 * The view on screen as location.hash says it: the year by its column's file
 * name, the opened path outermost first, the fund groups and link kinds the
 * filters remove, the smallest flow drawn as itself in cents, and the
 * isolation. A folded tail drawn out is not in it.
 * @typedef {{year: string, open: string[], hide: string[], hidekind: string[], min: number, iso: string}} ViewState
 */

/** The hash's fields, in the order hashOf writes them. */
export const HASH_FIELDS = Object.freeze(["y", "open", "hide", "hidekind", "min", "iso"]);

/**
 * The year the page opens on where the URL names no view: the control's,
 * read once at boot. A refused hash shows this year's overview.
 * @type {FiscYear | null}
 */
export let openingYear = null;

/** Whether restoreView is moving the page, so nothing it draws writes the URL. */
let restoringView = false;

/**
 * A year's name in the hash: its column's file name less .json, unique per
 * year because each year is one column file.
 * @param {FiscYear} year
 * @returns {string}
 */
export function yearKey(year) {
  return String(year.path).replace(/\.json$/, "");
}

/**
 * The view on screen, or null before a year has drawn. Lists are sorted, so
 * one view has one hash.
 * @returns {ViewState | null}
 */
export function viewState() {
  if (!shownYear) return null;
  return {
    year: yearKey(shownYear),
    open: drilled.map((r) => r.id),
    hide: Array.from(filter.groups).sort(),
    hidekind: Array.from(filter.kinds).sort(),
    min: filter.smallest,
    iso: filter.isolated,
  };
}

/**
 * One value of the hash, escaped except for the slashes every id holds, so
 * the hash stays readable.
 * @param {string} v
 * @returns {string}
 */
function hashValue(v) {
  return encodeURIComponent(v).replace(/%2F/g, "/");
}

/**
 * A view as a hash: `#y=<year>`, then each field not at its default --
 * `open`, `hide` and `hidekind` comma-separated, `min` in cents, `iso`.
 * @param {ViewState} st
 * @returns {string}
 */
export function hashOf(st) {
  const parts = ["y=" + hashValue(st.year)];
  if (st.open.length) parts.push("open=" + st.open.map(hashValue).join(","));
  if (st.hide.length) parts.push("hide=" + st.hide.map(hashValue).join(","));
  if (st.hidekind.length) parts.push("hidekind=" + st.hidekind.map(hashValue).join(","));
  if (st.min) parts.push("min=" + st.min);
  if (st.iso) parts.push("iso=" + hashValue(st.iso));
  return "#" + parts.join("&");
}

/**
 * A hash read back into a view. "none" for an empty one and for an in-page
 * anchor (no "="), which name no view; "refused", with why, for one this
 * file cannot read whole: a field it does not write or one written twice, no
 * year, an empty id, or a smallest flow that is not whole cents. Nothing is
 * guessed.
 * @param {string} hash
 * @returns {{kind: "none"} | {kind: "refused", why: string} | {kind: "view", state: ViewState}}
 */
export function parseHash(hash) {
  const body = String(hash || "").replace(/^#/, "");
  if (!body || body.indexOf("=") < 0) return { kind: "none" };
  /** @type {Map<string, string>} */
  const fields = new Map();
  for (const part of body.split("&")) {
    const at = part.indexOf("=");
    const name = at < 0 ? part : part.slice(0, at);
    if (!HASH_FIELDS.includes(name)) return { kind: "refused", why: "it carries a field, " + name + ", this page does not write" };
    if (fields.has(name)) return { kind: "refused", why: "it names " + name + " twice" };
    fields.set(name, at < 0 ? "" : part.slice(at + 1));
  }
  /** @type {(raw: string) => string} */
  const text = (raw) => {
    try {
      return decodeURIComponent(raw);
    } catch (e) {
      return "";
    }
  };
  const list = (/** @type {string} */ name) => {
    const raw = fields.get(name);
    return raw === undefined || raw === "" ? [] : raw.split(",").map(text);
  };
  const year = text(fields.get("y") || "");
  if (!year) return { kind: "refused", why: "it names no year" };
  const open = list("open");
  const hide = list("hide");
  const hidekind = list("hidekind");
  if ([...open, ...hide, ...hidekind].some((id) => !id)) return { kind: "refused", why: "it holds an empty name" };
  const min = fields.has("min") ? fields.get("min") || "" : "0";
  if (!/^\d+$/.test(min) || !Number.isSafeInteger(Number(min))) {
    return { kind: "refused", why: "its smallest flow, " + min + ", is not a whole number of cents" };
  }
  return {
    kind: "view",
    state: { year, open, hide: hide.sort(), hidekind: hidekind.sort(), min: Number(min), iso: text(fields.get("iso") || "") },
  };
}

/**
 * Writes the view on screen into the URL: a new history entry for a drill, a
 * pop or a year ("push"), the entry on screen rewritten for a filter or an
 * isolation ("replace"). Nothing where the hash already says it, while a
 * restore is moving the page, or before a year has drawn.
 * @param {"push" | "replace"} how
 */
export function recordView(how) {
  if (restoringView) return;
  const st = viewState();
  if (!st) return;
  const hash = hashOf(st);
  if (window.location.hash === hash) return;
  try {
    if (how === "push") window.history.pushState(null, "", hash);
    else window.history.replaceState(null, "", hash);
  } catch (e) { /* a sandboxed frame refuses history; the view still draws */ }
}

/**
 * Moves the page to a view, field by field, and says why where it cannot: a
 * year the page does not publish, a flow kind it names no label for, a fund
 * group the year's column does not carry, a mark on the path that the chart
 * it would open from does not offer, or an isolation naming a mark the chart
 * does not draw. A group or kind the view does not draw is inert, as the
 * panel's are. The opened path is walked through drillDown from the deepest
 * rung the page already shares with it, so each rung is offered by the one
 * rule the chart's own gestures use.
 * @param {ViewState} st
 * @returns {Promise<{outcome: string, why?: string}>} DREW, SUPERSEDED, or FAILED with why
 */
async function moveTo(st) {
  const refuse = (/** @type {string} */ why) => ({ outcome: FAILED, why });
  const year = (CONFIG.years || []).find((/** @type {FiscYear} */ y) => yearKey(y) === st.year);
  if (!year) return refuse("this page publishes no year " + st.year);
  const kinds = CONFIG.kind_labels || {};
  const kind = st.hidekind.find((k) => !Object.prototype.hasOwnProperty.call(kinds, k));
  if (kind !== undefined) return refuse("this page names no flow kind " + kind);
  const removals = { groups: new Set(st.hide), kinds: new Set(st.hidekind), smallest: st.min };
  if (!shownYear || shownYear !== year || !projection) {
    filter = Object.freeze(Object.assign({}, removals, { isolated: "" }));
    const got = await showYear(year);
    if (got !== DREW) return { outcome: got, why: "the year " + st.year + " could not be drawn" };
    syncYearControl();
  } else if (!setFilter(Object.assign({}, removals, { isolated: "" }))) {
    return refuse("the chart could not be filtered as it names");
  }
  const group = st.hide.find((g) => fundGroupPlace(g) < 0);
  if (group !== undefined) return refuse(year.label + " carries no fund group " + group);
  let shared = 0;
  while (shared < drilled.length && shared < st.open.length && drilled[shared].id === st.open[shared]) shared++;
  if (shared < drilled.length) {
    drillUp(shared);
    if (drilled.length !== shared) return refuse("the chart could not be closed to its opened path");
  }
  for (const id of st.open.slice(shared)) {
    const node = projection ? projection.nodes.find((n) => n.id === id) : undefined;
    if (!node || !drillable(node)) {
      return refuse((drilled.length ? labelOfRung(drilled.length - 1) : "the overview") + " offers no mark " + id + " to open");
    }
    const got = await drillDown(id);
    if (got !== DREW) return { outcome: got, why: id + " could not be opened" };
  }
  if (st.iso && !laidNodes.some((n) => isolates(st.iso, n))) return refuse("the chart draws no mark " + st.iso + " to follow");
  setIsolated(st.iso);
  return { outcome: DREW };
}

/**
 * Shows the view a hash names, or refuses it: the reader is told why in the
 * refusal banner and shown the page's opening view (openingYear's overview,
 * nothing filtered), never the nearest view the hash might have meant. A
 * hash naming no view shows the opening view. The URL is then rewritten to
 * say the view on screen.
 * @param {string} hash
 * @returns {Promise<string>} DREW, SUPERSEDED or FAILED
 */
export async function restoreView(hash) {
  const read = parseHash(hash);
  const opening = openingYear || (CONFIG.years || [])[(CONFIG.years || []).length - 1];
  /** @type {ViewState} */
  const fallback = { year: yearKey(opening), open: [], hide: [], hidekind: [], min: 0, iso: "" };
  const named = read.kind !== "none";
  let moved = /** @type {{outcome: string, why?: string}} */ ({ outcome: FAILED, why: read.kind === "refused" ? read.why : "" });
  restoringView = true;
  try {
    if (read.kind !== "refused") moved = await moveTo(read.kind === "view" ? read.state : fallback);
  } finally {
    restoringView = false;
  }
  if (moved.outcome === SUPERSEDED) return SUPERSEDED;
  if (moved.outcome === DREW) {
    if (named) recordView("replace");
    return DREW;
  }
  if (!named) return FAILED;
  // REFUSED: the opening view, then the banner, since a year drawing clears it.
  restoringView = true;
  let back;
  try {
    back = await moveTo(fallback);
  } finally {
    restoringView = false;
  }
  if (back.outcome === SUPERSEDED) return SUPERSEDED;
  fail("This link names a view this page does not draw: " + moved.why +
    ". The page shows its opening view instead.");
  if (back.outcome === DREW) recordView("replace");
  return FAILED;
}

/* ------------------------------------------------------------------ *
 * Boot
 * ------------------------------------------------------------------ */

/** Returns the provenance panel to its unpinned state. */
export function resetDetail() {
  const panel = el("detail");
  panel.replaceChildren();
  panel.append(h("p", "subtle", "Select a flow or a node to pin its provenance here."));
}

/**
 * Refuses to draw, visibly: in the panel and in a role="alert" banner at the
 * top, since the panel sits below a chart a reader may never scroll past.
 *
 * @param {string} message
 */
export function fail(message) {
  const panel = el("detail");
  panel.replaceChildren();
  panel.append(h("p", "", message));

  const content = document.querySelector("main");
  if (content) {
    const banner = h("div", "refusal", message);
    banner.setAttribute("role", "alert");
    // Replace rather than stack.
    const existing = content.querySelector(".refusal");
    if (existing) existing.remove();
    content.prepend(banner);
  }
}

/**
 * Removes the refusal banner, if one is showing, so a year switch that
 * recovers does not leave a stale alert above a chart that drew.
 */
export function clearRefusal() {
  const content = document.querySelector("main");
  if (!content) return;
  const existing = content.querySelector(".refusal");
  if (existing) existing.remove();
}



/**
 * Reports whether a fetched body is a document at all.
 * @param {any} doc
 * @param {string} what
 */
export function isDocument(doc, what) {
  if (doc && typeof doc === "object") return true;
  // A 200 whose body is `null` is a real server answer; this runs before
  // anything dereferences the body. Not a shape check: no schema reaches what
  // a server answered.
  fail(
    "This page will not draw " + what + ": the file is not a document at all. " +
    "It is most likely an error page served with a success status. Nothing on " +
    "the page was changed."
  );
  return false;
}


/**
 * Fetches one document and vets it, returning it, or null with the reader told
 * why -- unless `superseded` says nobody is waiting, in which case nothing is
 * painted and null is returned without a word.
 *
 * One fetch per year; a drill selects a schedule out of what it accepted.
 * `superseded` is consulted before every banner, so a failure the reader has
 * switched away from never alerts over a year that drew. Both awaits are
 * inside the try, so a malformed body is refused here rather than rejecting.
 *
 * @param {string} path
 * @param {() => boolean} superseded
 * @returns {Promise<FiscProjection | null>}
 */
export async function loadColumn(path, superseded) {
  let doc;
  try {
    const response = await fetch(path);
    if (superseded()) return null;
    if (!response.ok) {
      fail("Could not load " + path + ": HTTP " + response.status);
      return null;
    }
    doc = /** @type {FiscProjection} */ (await response.json());
  } catch (e) {
    if (superseded()) return null;
    // A rejected fetch is most likely file://, so say the fix. `e.name`, not
    // instanceof: a body parsed in another realm fails instanceof.
    fail(e && e.name === "SyntaxError"
      ? "Could not read " + path + ": the file is not valid JSON, so it is " +
        "truncated or was not the document this page expected."
      : "Could not load " + path + ". If you opened this file directly, the browser " +
        "blocks the request: serve the directory over HTTP instead, e.g. " +
        "python3 -m http.server -d dist 8000");
    return null;
  }
  if (superseded()) return null;
  if (!isDocument(doc, path)) return null;
  // The one check Go cannot make: which COPY the browser holds. The site has
  // no cache-busting, and the stamp names the commit, so a column from any
  // other deploy fails here, before any repaint (fisc-bsg).
  if (doc.generated_by !== CONFIG.exported_by) {
    fail("This page will not draw " + path + ": the page was packaged by " +
      CONFIG.exported_by + " and this file by " + (doc.generated_by || "an unstated build") +
      ". Your browser is most likely holding a copy from before the last update — " +
      "reload the page. Drawing them together would produce a chart that is wrong " +
      "rather than one that fails. Nothing on the page was changed.");
    return null;
  }
  return doc;
}

/**
 * One schedule of the column on screen, vetted as it is first selected.
 *
 * @param {any} col
 * @param {string} key
 * @returns {FiscProjection | null}
 */
export function selectSchedule(col, key) {
  const doc = scheduleOf(col, key);
  if (!doc) {
    fail("That could not be opened: the year on screen carries no schedule " +
      "called " + key + ".");
    return null;
  }
  return doc;
}


/**
 * What one showYear attempt came to. SUPERSEDED (a later switch took over) is
 * not FAILED (the reader has been told), and a boolean conflated them.
 */
export const DREW = "drew";
export const SUPERSEDED = "superseded";
export const FAILED = "failed";

/** The latest switch token; an attempt holding an older one stands down. */
export let switching = 0;

/**
 * Fetches and draws one published year. Every word comes from CONFIG.years;
 * this composes no figure or caveat of its own.
 *
 * @param {FiscYear} year
 * @returns {Promise<string>} DREW, SUPERSEDED or FAILED
 */
export async function showYear(year) {
  // Without the token, the slower of two in-flight fetches wins.
  const token = ++switching;
  const superseded = () => token !== switching;
  const loaded = await loadColumn(year.path, superseded);
  if (superseded()) return SUPERSEDED;
  if (!loaded) return FAILED;

  const doc = selectSchedule(loaded, CONFIG.primary);
  if (!doc) return FAILED;

  // Swapped because shapeFor and askAffordances read them; swapped back on a
  // throw, so nothing below writes a word of this year over the last one's
  // chart.
  const was = { column, fetched, drilled, laidNodes, groupIndex, projection, shown, whole };
  column = loaded;
  fetched = doc;
  drilled = [];
  let laid;
  let rows;
  try {
    // The folded document is the one the page describes; the fold unions the
    // fact ids it merges, so nothing is lost.
    const drawn = shapeFor(doc);
    const filtered = filterFor(drawn);
    const unfiltered = wholeOf(drawn, filtered);
    laid = layOut(filtered.doc);
    rows = tableRows(drawn);
    projection = drawn;
    shown = filtered;
    whole = unfiltered;
    askAffordances();
  } catch (e) {
    ({ column, fetched, drilled, laidNodes, groupIndex, projection, shown, whole } = was);
    throw e;
  }

  clearRefusal();
  // A pin and an isolation belong to the year they were made in.
  pinned = null;
  filter = Object.freeze(Object.assign({}, filter, { isolated: "" }));
  resetDetail();
  hideTip();

  paintYearWords(year);
  paintBreadcrumb();
  syncColumns();
  buildLegend();
  buildFilterPanel();
  paintFilterLine();
  paintChartHint();
  buildDerivedList();
  buildTable(rows);
  render(laid);
  recordView("push");
  return DREW;
}

/**
 * Replaces every word on the page that belongs to a year: the hero, the tiles,
 * the caveats, the caveat count in their summary, the lede, the flow count, the
 * chart's accessible name and description (through paintChartName), the
 * footer's basis and its data-file citation, and the document title.
 *
 * The list is exhaustive: add any new per-year write to it. Every string is
 * the packager's except the counts line, which paintCounts composes.
 * @param {FiscYear} year
 */
export function paintYearWords(year) {
  const tile = (f) => {
    const el = h("div", "tile" + (f.kind ? " " + f.kind : ""));
    el.appendChild(h("div", "label", f.label));
    el.appendChild(h("div", "value", f.value));
    el.appendChild(h("div", "note", f.note));
    return el;
  };

  // maybeEl: only index.html.tmpl renders these, since a page drawing one
  // grain of one document must publish no total.
  const hero = maybeEl("hero");
  if (hero) hero.replaceChildren(tile(year.hero));

  const figures = maybeEl("figures");
  if (figures) figures.replaceChildren(...year.figures.map(tile));

  const caveats = maybeEl("caveats");
  // c.href is per year: one caveat id can carry different text in different
  // documents. Empty when the export wrote no caveats page.
  if (caveats) {
    caveats.replaceChildren(...year.caveats.map((c) => {
      const li = h("li");
      if (!c.href) {
        li.textContent = c.summary;
        return li;
      }
      li.appendChild(link(c.summary, c.href));
      return li;
    }));
  }

  // The caveat count is per year.
  const caveatCount = maybeEl("caveats-count");
  if (caveatCount) caveatCount.textContent = String(year.caveats.length);

  const lede = maybeEl("lede-year");
  if (lede) lede.textContent = year.lede;

  shownYear = year;
  paintCounts();

  // Shared with the drill, so the two cannot name the chart differently.
  paintChartName();

  // The footer's basis is per year; its scope is not repainted (see the
  // template).
  const basis = maybeEl("page-basis");
  if (basis) basis.textContent = year.basis;

  // The footer's "drawn from" link names a year-specific file.
  for (const el of document.querySelectorAll("[data-year-path]")) {
    const a = el.querySelector("a");
    if (a) {
      a.setAttribute("href", year.path);
      a.textContent = year.path;
    }
  }

  document.title = year.title;
}

/**
 * The year the CONTROL is showing. Browsers restore a radio selection across
 * reload and Back without firing change, so the page opens on the restored
 * year. The fallback is the newest: CONFIG.years is oldest first.
 *
 * @param {FiscYear[]} years
 * @returns {FiscYear} always one of `years`; the newest when nothing is checked
 */
export function checkedYear(years) {
  const group = maybeEl("year-toggle");
  if (group) {
    for (const input of group.children) {
      if (!input.checked) continue;
      const year = years.find((y) => y.stem === input.value);
      // A stem this config does not publish is a stale restore.
      if (year) return year;
    }
  }
  return years[years.length - 1];
}

/** Moves the year control to the year on screen. */
export function syncYearControl() {
  const group = maybeEl("year-toggle");
  if (!group || !shownYear) return;
  for (const r of group.querySelectorAll("input[type=radio]")) {
    const radio = /** @type {HTMLInputElement} */ (r);
    radio.checked = radio.value === shownYear.stem;
  }
}

/**
 * Wires the server-rendered year radio group. A year that fails to load moves
 * the radio back to the year still on screen.
 * @param {FiscYear[]} years
 */
export function wireYears(years) {
  const group = maybeEl("year-toggle");
  if (!group || years.length < 2) return;
  // The template ships it disabled for the no-script page.
  group.removeAttribute("disabled");
  group.addEventListener("change", (e) => {
    const target = /** @type {HTMLInputElement} */ (e.target);
    const year = years.find((y) => y.stem === target.value);
    if (!year) return;
    // A refused switch leaves the page on the year it was, so the control goes
    // back to it; the banner says what failed.
    const back = syncYearControl;
    void showYear(year).then((got) => { if (got === FAILED) back(); }, (e) => {
      back();
      fail("The chart failed to draw: " + String(e));
    });
  });
}

export async function main() {
  wireTheme();
  wireColumns();
  // This script against the page it was served in: app.js is cached
  // separately and carries no stamp, so a constant is the only handshake. It
  // holds the page to core.js; nothing yet holds the three modules to each
  // other (fisc-mokd).
  if (CONFIG.schema_version !== SCHEMA_VERSION) {
    fail("This page will not draw: it was packaged for schema_version " +
      CONFIG.schema_version + " and this script renders schema_version " +
      SCHEMA_VERSION + ". " + (CONFIG.schema_version > SCHEMA_VERSION
        ? "The page is newer than this script; a cached copy of app.js is the " +
          "likely cause, so reload to pick up the current one."
        : "The page is older than this script.") +
      " Drawing it anyway would produce a chart that is wrong rather than one " +
      "that fails.");
    return;
  }

  const form = undrawableForm();
  if (form) {
    fail("This page will not draw: it declares a " + form + " chart, and this script " +
      "draws " + Array.from(FORMS.keys()).join(", ") + ". Drawing it with another " +
      "renderer would produce a chart that is wrong rather than one that fails.");
    return;
  }

  const years = CONFIG.years || [];
  if (!years.length) {
    fail("This page was packaged without any published year, so there is nothing to draw.");
    return;
  }
  wireYears(years);
  wireFilters();
  wireSearch();
  wireFullscreen();
  wireZoom();

  // Wired before the first fetch (fisc-8cg): a superseded or failed opening
  // year can still be followed by one that draws, and it needs these.
  document.addEventListener("keydown", (e) => {
    if (e.key === "Escape") {
      // Escape undoes one thing, innermost first: a pin or isolation, then one
      // rung. drillUp clears the pin and isolation itself.
      if (!pinned && !filter.isolated && drilled.length) {
        drillUp(drilled.length - 1);
        return;
      }
      hideTip();
      pinned = null;
      resetDetail();
      setIsolated("");
    }
  });
  if (typeof window.matchMedia === "function") {
    // d3 wrote resolved hues onto the marks, so an OS switch re-paints; and
    // prefersDark() has changed underneath the button.
    window.matchMedia("(prefers-color-scheme: dark)").addEventListener("change", () => {
      syncTheme();
      paint();
    });
  }

  openingYear = checkedYear(years);
  // Back and Forward walk the views this page recorded (recordView).
  window.addEventListener("popstate", () => {
    void restoreView(window.location.hash).catch((e) => fail("The chart failed to draw: " + String(e)));
  });
  // The outcome is deliberately ignored: restoreView and showYear have told
  // the reader.
  await restoreView(window.location.hash);
}

/**
 * Boots the page: [main], with its last-resort catch turned into a refusal.
 * The template calls this; nothing runs at import.
 */
export function boot() {
  return main().catch((e) => fail("The chart failed to draw: " + String(e)));
}
