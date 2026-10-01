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
  SCHEMA_VERSION, say, kindLabel, money, fmt, fmtSigned, fmtShortSigned, joinOr, el, maybeEl, cssVar, h, link, citations, ledeOf, PARTITION_NOTE, isFundGroup, scheduleOf, withinNode, scoped, foldTarget, regroupLocators, FOLD_REFUSES_MIXED, capColumn, tailFigure, isAggregate, residualID, isResidual, gapID, isGap, tableRows,
} from "./core.js";
import * as sankey from "./sankey.js";
import {
  NODE_WIDTH, NODE_PADDING, RIBBON_GAP, CHART_HEIGHT, LABEL_GUTTER, chartWidth, CHART_CUSHION, SIDE_SOURCE, reaching, sideOf, flankHolds, decomposable, markAmounts, markContra, isContraNode, isPartitionNode, isLink, markCents, contraBand, residualFlows, contraNote, markGap, restackLinks, alignFor, labelLineShift, SANKEY,
} from "./sankey.js";
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
 * layer is the column d3 put it in, which is not depth; columnShare needs layer.
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

/** sankey.windowFor over the columns on screen. */
export function windowFor(onScreen, stepDoc, rung) {
  return sankey.windowFor(onScreen, stepDoc, rung, activeTiers());
}

/** sankey.carryResidual over the columns on screen. */
export function carryResidual(drawn, from, rung) {
  return sankey.carryResidual(drawn, from, rung, activeTiers());
}

/** sankey.dropEmptyColumns over the innermost rung and the columns on screen. */
export function dropEmptyColumns(drawn) {
  const rung = drilled.length ? drilled[drilled.length - 1] : null;
  return rung ? formFor(rung.step).refit(drawn, rung, activeTiers()) : false;
}

/** sankey.columnOf over the columns on screen. */
export function columnOf(d) {
  return sankey.columnOf(activeTiers(), d);
}

/** sankey.labelPlacement over the columns on screen. */
export function labelPlacement(d, last) {
  return sankey.labelPlacement(activeTiers(), d, last);
}

/** sankey.labelQualifiers over the document on screen and its columns. */
export function labelQualifiers(nodes) {
  return sankey.labelQualifiers(projection, activeTiers(), nodes);
}

/** sankey.columnShare over the chart's laid nodes. */
export function columnShare(d) {
  return sankey.columnShare(laidNodes, d);
}

/** sankey.nodeRank over the drawn index and the column's fund-group order. */
export function nodeRank(node) {
  return sankey.nodeRank(fundGroupOf, fundGroupPlace, node);
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
 * form to. A renderer reads of a step the generic fields and its own hints,
 * calls core.js for every figure, and computes none of its own.
 * @typedef {{
 *   form: string,
 *   tiersOf(chart: FiscChart | FiscDrillStep): number[],
 *   caps(chart: FiscChart | FiscDrillStep): FiscTierCap[],
 *   columns(chart: FiscDrillStep, rung: Rung | null, budget: number): number[],
 *   offers(step: FiscDrillStep, doc: FiscProjection, onScreen: FiscProjection | null, id: string): boolean,
 *   shape(doc: FiscProjection, rung: Rung | null, from: FiscProjection | null, tiers: number[]): FiscProjection,
 *   refit(drawn: FiscProjection, rung: Rung | null, tiers: number[]): boolean,
 *   layOut(drawn: FiscProjection, ctx: {tiers: number[], columns: number, groupOf: (n: FiscNode | LaidNode) => string, placeOf: (id: string) => number}): {nodes: LaidNode[], links: LaidLink[]},
 *   render(graph: {nodes: LaidNode[], links: LaidLink[]}, ctx: Record<string, any>): void,
 *   paint(ctx: {svg: any, colour: {link: (d: LaidLink) => string, node: (d: LaidNode) => string}}): void,
 *   widest(steps: FiscDrillStep[]): number,
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
 * Whether opening `id` on `step` would draw anything: the step's document
 * sends at least one ribbon between the node's parts and the columns the step
 * opens into, and on a window step the chart on screen sends a kept flank into
 * it. Asked with the reach the chart is drawn with (reaching), so what is
 * offered and what draws are one rule. A node with no id is open too.
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
  return formFor(step).offers(step, doc, projection, id);
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
 * A link's hue: the fund group at either end, else the group both ends share,
 * else "" (--muted) rather than picking one of two.
 * @param {LaidLink} link
 * @returns {string}
 */
export function linkColor(link) {
  const source = fundGroupOf(link.source);
  const target = fundGroupOf(link.target);
  const group = isFundGroup(link.source) ? link.source.id
    : isFundGroup(link.target) ? link.target.id
    : source === target ? source
    : "";
  return cssVar(fundColorVar(group));
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
/** Node id whose flows are isolated, or "" for all of them. */
export let isolated = "";
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
 * @property {number[]} [dropped]  widened tiers this rung's document left
 *   empty, so activeTiers stops asking for them; kept after the budget changes.
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
 * The viewport widths that buy a column beyond the floor. Each threshold is
 * chartWidth(n) plus CHART_CUSHION, computed so it is the width at which the
 * nth column fits. Only ever adds columns; never below NARROW_COLUMNS.
 * @type {{query: string, columns: number}[]}
 */
export const COLUMN_QUERIES = (() => {
  const out = [];
  for (let n = NARROW_COLUMNS + 1; n <= OFFERED_COLUMNS; n++) {
    out.push({ query: "(min-width: " + (chartWidth(n) + CHART_CUSHION) + "px)", columns: n });
  }
  return out;
})();

/**
 * The widest chart this page can be asked to draw, in px, handed to style.css
 * as --chart-max so the stylesheet holds no second spelling of it.
 */
export const CHART_MAX = chartWidth(OFFERED_COLUMNS);

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
 * The nodes as laid out, so columnShare can total a mark's column. Separate
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
 * The tier set the document on screen was shaped by: RENDER_TIERS on an
 * overview, the opening step's tiers on a rung, less the widened columns the
 * budget drops (from the END of `widen`, so a narrowed window has no hole) and
 * the columns the rung dropped as empty (dropEmptyColumns). A wrong answer
 * here throws inside d3-sankey's ordering pass.
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
 * it is neither an aggregate, which has no one node to open into, nor a
 * carried mark, which is a flow's end rather than a container.
 *
 * @param {{id: string, tier: number, role?: string}} d
 * @returns {boolean}
 */
export function drillable(d) {
  return Boolean(stepFor(d)) && !isAggregate(d.id) && !isCarried(d.id);
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
  if (!rung || !isAggregate(d.id)) return false;
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
 * Replaces the stack with `next` and repaints everything the shape decides,
 * or leaves the page exactly as it was and tells the reader why.
 *
 * Everything that can throw -- shape, lay-out, table rows -- runs in the try
 * while the page is still the reader's. The stack is swapped first because
 * shapeFor and layOut read it, and restored whole on a throw. Pin and
 * isolation are cleared because a drill can remove the node they name.
 *
 * @param {Rung[]} next
 * @param {string} [refused] the banner's opening words when the redraw is refused
 * @returns {boolean} whether the new depth is on screen
 */
export function redrawStack(next, refused = "That could not be opened") {
  // Asked before the repaint removes the element focus is on.
  const hadFocus = focusInChart();
  const was = drilled;
  // THE RUNG THIS REDRAW CLOSES, if it closes one: the mark that opened it is
  // on the chart being returned to, and is where focus goes back to.
  const popped = was.length > next.length ? was[next.length].id : "";
  const laidWas = laidNodes;
  const groupsWas = groupIndex;
  drilled = next;
  let drawn;
  let laid;
  let rows;
  try {
    const doc = drawnDoc();
    if (!doc) throw new Error("no document to open");
    drawn = shapeFor(doc);
    // Reshaped, not just re-laid: the fold, caps and placeability all take the
    // tier set. Terminates: each pass adds an entry of the finite `widen` list
    // to the dropped set and never removes one.
    while (dropEmptyColumns(drawn)) drawn = shapeFor(doc);
    laid = layOut(drawn);
    // Inside the try: building a row is the last step of a draw that can
    // throw on a document.
    rows = tableRows(drawn);
  } catch (e) {
    // Back to where the reader was: a throw here is a fault in the view's
    // declaration, not in the reader's click.
    drilled = was;
    laidNodes = laidWas;
    groupIndex = groupsWas;
    fail(refused + ": " + (e instanceof Error ? e.message : String(e)));
    return false;
  }
  clearRefusal();
  projection = drawn;
  pinned = null;
  isolated = "";
  resetDetail();
  hideTip();
  paintBreadcrumb();
  paintChartName();
  paintCounts();
  // The count and steppers describe the chart on screen, which a drill changes.
  syncColumns();
  buildLegend();
  paintChartHint();
  buildDerivedList();
  buildTable(rows);
  render(laid);
  restoreFocus(hadFocus, popped);
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
  redrawStack(drilled.slice(0, at).concat([Object.assign({}, rung, { expanded: expanded })]));
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
  clickIsolate = { id: d.id, at: at, was: within ? clickIsolate.was : isolated };
  setIsolated(isolated === d.id ? "" : d.id);
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
  if (drillable(d)) openNode(d.id);
  else if (expandable(d)) expandTier(d);
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
  if (key === "Enter" && drillable(d)) {
    openNode(d.id);
    return;
  }
  if (key === "Enter" && expandable(d)) {
    expandTier(d);
    return;
  }
  setIsolated(isolated === d.id ? "" : d.id);
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
 * rung, to the rung's own return control.
 *
 * @param {boolean} hadFocus whether focus was inside the chart before the
 *   repaint that just replaced it.
 * @param {string} [popped] the id of the node whose rung this repaint closed,
 *   or "" where it opened one
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
  // What the columns are is the step's own description, from Go.
  if (drilled.length) {
    const step = drilled[drilled.length - 1].step;
    const said = step && typeof step.description === "string" && step.description
      ? step.description
      : trail + " on the left, and what it is made of on the right.";
    desc.textContent = "Opened into " + trail + ". " + said + " " + say("go_back") + " " + tablePointer;
    return;
  }
  desc.textContent = baseDescription;
}

/**
 * Says what a click does, for the chart on screen.
 *
 * WHETHER ANYTHING OPENS, AND IN WHICH COLUMN, IS ASKED OF THE DRAWN NODES,
 * not of the declared chain: a step can exist below this depth with no node
 * on screen at its `from` tier.
 */
export function paintChartHint() {
  const hint = maybeEl("chart-hint");
  if (!hint || !STEPS.length) return;
  const anyOpens = Boolean(projection) && projection.nodes.some(drillable);
  const column = anyOpens ? joinOr(openableColumns()) : "";
  // An undeclared column order has no left or right to name: drop the clause.
  const where = column ? say("in_column", { columns: column }) : "";
  // The isolate is named on every view, and Space with it, because Space
  // never opens against the role="button" convention.
  const follows = " " + say("follow");
  // A sentence of its own: the tail can be the only thing a chart offers.
  const tails = Boolean(projection) && projection.nodes.some(expandable);
  const expands = tails ? " " + say("expand") : "";
  if (drilled.length) {
    hint.textContent = say("opened_hint", { label: labelOfRung(drilled.length - 1) }) + " " +
      (anyOpens ? say("open_further", { where }) : say("nothing_further")) + follows + expands;
    return;
  }
  const swatches = buildLegendCount();
  hint.textContent = (anyOpens ? say("open_into", { where }) : say("nothing_opens")) + follows +
    (swatches ? " " + say("swatch") : "");
}

/**
 * Which columns of the chart on screen hold a node that opens, named left to
 * right, or none when nothing here opens.
 *
 * IN THE DECLARED ORDER AND NOT IN TIER ORDER: layOut aligns on the declared
 * set, and a window's ({2,5,4}) does not ascend. Narrowed to the tiers the
 * chart actually draws, because a declaration need not be filled.
 * @returns {string[]}
 */
export function openableColumns() {
  if (!projection) return [];
  const drawn = new Set(projection.nodes.map((n) => n.tier));
  const tiers = activeTiers().filter((t) => drawn.has(t));
  if (tiers.length < 2) return [];
  const opening = new Set(projection.nodes.filter(drillable).map((n) => n.tier));
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
 * word where it has one, since one step can cap two columns under two nouns.
 * @param {number} tier
 * @returns {string}
 */
export function tailNoun(tier) {
  const rung = drilled.length ? drilled[drilled.length - 1] : null;
  if (!rung) return "items";
  const cap = formFor(rung.step).caps(rung.step).find((c) => c.tier === tier);
  return (cap && cap.tail) || rung.step.tail || "items";
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
  return formFor(chartOnScreen()).shape(doc, rung, from, activeTiers());
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
  return "node" + (d.derived ? " derived" : "") + (isContraNode(d) ? " contra" : "") +
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
  const index = new Map(doc.nodes.map((n) => [n.id, n]));
  const source = drawnDoc();
  if (source) {
    for (const n of source.nodes) index.set(n.id, n);
  }
  const graph = formFor(chartOnScreen()).layOut(doc, {
    tiers: activeTiers(),
    columns: drawnColumns(),
    groupOf: (/** @type {FiscNode | LaidNode} */ n) => core.fundGroupOf(index, n),
    placeOf: fundGroupPlace,
  });
  groupIndex = index;
  // For columnShare: only the laid graph knows a mark's column.
  laidNodes = graph.nodes;
  return graph;
}

/**
 * Draws the chart on screen through its form, then paints it and applies the
 * emphasis. The gestures, descriptions and classes are this file's: they read
 * the page's state, which a renderer never holds.
 * @param {{nodes:LaidNode[], links:LaidLink[]}} [laid]
 */
export function render(laid) {
  if (!projection) return;
  const graph = laid || layOut(projection);
  formFor(chartOnScreen()).render(graph, {
    svg: D3.select("#chart"),
    tiers: activeTiers(),
    columns: drawnColumns(),
    doc: projection,
    classes: { link: linkClass, node: nodeClass, flags: nodeFlags },
    describe: { link: linkDescription, node: nodeDescription },
    on: {
      tip: showTip, hide: hideTip, pin: pin, guarded: guarded,
      click: clickNode, dblclick: doubleClickNode, key: keyNode,
      restoring: () => restoring,
    },
  });
  paint();
  applyEmphasis();
}

/** Colours the chart through its form, and the legend's swatches here. */
export function paint() {
  if (!CONFIG || !CONFIG.overview) return;
  formFor(chartOnScreen()).paint({
    svg: D3.select("#chart"),
    colour: { link: linkColor, node: nodeColor },
  });
  for (const button of document.querySelectorAll("#legend button .key")) {
    const swatch = /** @type {HTMLElement} */ (button);
    const name = swatch.dataset.var;
    if (name) swatch.style.background = cssVar(name);
  }
}

/**
 * Isolates one node's flows, or clears the isolation for "".
 *
 * Every path into this state goes through here, so the legend's pressed button
 * and the dimming cannot drift apart.
 * @param {string} id
 */
export function setIsolated(id) {
  isolated = id;
  for (const element of el("legend").querySelectorAll("button")) {
    const button = /** @type {HTMLElement} */ (element);
    const pressed = isolated !== "" && button.dataset.node === isolated;
    button.setAttribute("aria-pressed", String(pressed));
  }
  applyEmphasis();
}

/** Applies the isolation and the pinned selection to every mark. */
export function applyEmphasis() {
  const svg = D3.select("#chart");
  svg.selectAll("path.link").classed("dim", /** @param {LaidLink} d */ (d) =>
    isolated !== "" && d.source.id !== isolated && d.target.id !== isolated);
  svg.selectAll("path.link").classed("hot", /** @param {LaidLink} d */ (d) => d === pinned);
  svg.selectAll("g.node").classed("dim", /** @param {LaidNode} d */ (d) => {
    if (isolated === "") return false;
    if (d.id === isolated) return false;
    return !d.sourceLinks.concat(d.targetLinks).some((l) =>
      l.source.id === isolated || l.target.id === isolated);
  });
  svg.selectAll("g.node").attr("aria-pressed", /** @param {LaidNode} d */ (d) =>
    String(d.id === isolated && isolated !== ""));
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
  return d.source.label + " to " + d.target.label + ", " + fmtSigned(markCents(d)) +
    (kind ? ", " + kind : "") +
    (d.contra ? ", " + d.contra : "") +
    (d.partition ? ", " + PARTITION_NOTE : "") +
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
  const note = contraNote(d);
  // The cross-tab qualification, for a reader who cannot see the ribbons.
  const flows = residualFlows(d);
  return d.label + (flows ? ", " + flows : ", total " + fmtSigned(markCents(d))) +
    ", " + say(d.derived ? "inferred_by_us" : "printed_by_city") +
    (isPartitionNode(d) ? ", " + PARTITION_NOTE : "") +
    (note ? ", " + note.replace(/^\u25c7 /, "") : "") + what;
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
  const color = asLink ? linkColor(/** @type {LaidLink} */ (d)) : nodeColor(/** @type {LaidNode} */ (d));

  tip.append(h("div", "tip-value", fmtSigned(markCents(d))));

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
    const share = columnShare(n);
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
    if (n.rationale) tip.append(h("div", "tip-meta", n.rationale));
    const note = contraNote(n);
    if (note) tip.append(h("div", "tip-meta", note));
    for (const c of cavs) tip.append(h("div", "tip-meta", "\u26a0 " + c.summary));
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

  panel.append(h("div", "amount", fmtSigned(markCents(d))));
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
    // A derived ribbon with no fact of its own speaks in its derived end's words.
    const end = l.source.derived ? l.source : l.target;
    if (l.fact_ids.length) panel.append(h("div", "facts", "Facts: " + l.fact_ids.join(" ")));
    else if (l.derived && end.source_note) panel.append(h("p", "subtle", end.source_note));
  } else {
    const n = /** @type {LaidNode} */ (d);
    chips.append(h("span", "chip", n.role.replace(/_/g, " ")));
    if (n.constraint_tier) chips.append(h("span", "chip", "constraint: " + n.constraint_tier));
    chips.append(h("span", n.derived ? "chip derived" : "chip", say(n.derived ? "our_inference" : "printed_by_city")));
    const share = columnShare(n);
    if (share) chips.append(h("span", "chip derived", share));
    panel.append(chips);
    const flows = residualFlows(n);
    if (flows) panel.append(h("p", "subtle", flows));
    if (n.rationale) panel.append(h("p", "why", n.rationale));
    if (n.source_note) panel.append(h("p", "subtle", n.source_note));
    const note = contraNote(n);
    if (note) panel.append(h("p", "why", note));
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
 * Draws one swatch per fund group on the chart, each a toggle for that group's
 * isolation.
 *
 * EMPTY ON EVERY OPENED VIEW, BY RULE: a swatch toggles a NODE id, and on an
 * opened view that would dim what the reader opened the node to see. A legend
 * meaning a group rather than a node is fisc-0jy9.
 */
export function buildLegend() {
  if (!projection) return;
  const legend = el("legend");
  legend.replaceChildren();
  if (drilled.length) return;
  // The document's groups in Go's order; an unnamed one sorts last.
  const groups = projection.nodes.filter(isFundGroup).slice()
    .sort((a, b) => fundGroupPlace(a.id) - fundGroupPlace(b.id) ||
      (a.id < b.id ? -1 : a.id > b.id ? 1 : 0));
  for (const node of groups) {
    const button = document.createElement("button");
    button.type = "button";
    button.dataset.node = node.id;
    button.setAttribute("aria-pressed", "false");
    const key = h("span", "key");
    key.dataset.var = fundColorVar(node.id);
    button.append(key);
    button.append(document.createTextNode(node.label));
    button.addEventListener("click", () => setIsolated(isolated === node.id ? "" : node.id));
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
  const moved = setColumnBudget(columnOverride === null ? viewportColumns() : columnOverride);
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

  // Swapped because shapeFor reads them; swapped back on a throw.
  const was = { column, fetched, drilled, laidNodes, groupIndex };
  column = loaded;
  fetched = doc;
  drilled = [];
  let drawn;
  let laid;
  let rows;
  try {
    drawn = shapeFor(doc);
    laid = layOut(drawn);
    rows = tableRows(drawn);
  } catch (e) {
    ({ column, fetched, drilled, laidNodes, groupIndex } = was);
    throw e;
  }

  // The folded document is the one the page describes; the fold unions the
  // fact ids it merges, so nothing is lost.
  projection = drawn;
  clearRefusal();
  // A pin and an isolation belong to the year they were made in.
  pinned = null;
  isolated = "";
  resetDetail();
  hideTip();

  paintYearWords(year);
  paintBreadcrumb();
  syncColumns();
  buildLegend();
  paintChartHint();
  buildDerivedList();
  buildTable(rows);
  render(laid);
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
    const back = () => {
      if (!shownYear) return;
      for (const r of group.querySelectorAll("input[type=radio]")) r.checked = r.value === shownYear.stem;
    };
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
  // separately and carries no stamp, so a constant is the only handshake.
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

  // Wired before the first fetch (fisc-8cg): a superseded or failed opening
  // year can still be followed by one that draws, and it needs these.
  document.addEventListener("keydown", (e) => {
    if (e.key === "Escape") {
      // Escape undoes one thing, innermost first: a pin or isolation, then one
      // rung. drillUp clears the pin and isolation itself.
      if (!pinned && !isolated && drilled.length) {
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

  // The outcome is deliberately ignored: showYear has told the reader.
  await showYear(checkedYear(years));
}

/**
 * Boots the page: [main], with its last-resort catch turned into a refusal.
 * The template calls this; nothing runs at import.
 */
export function boot() {
  return main().catch((e) => fail("The chart failed to draw: " + String(e)));
}
