// @ts-check
/**
 * fisc — the published page's client script.
 *
 * Served exactly as committed; the page template boots it by importing [boot].
 * What the tests drive is exported, so they import the shipped file. At import
 * it reads its two globals once, d3 (D3) and window.FISC_CONFIG (CONFIG), and
 * derives RENDER_TIERS and BESIDE_QUERY from the config; it draws nothing and
 * binds no listener until [boot] runs. It holds the page's state, so the tests
 * load it fresh for each page they build (site/testlib.mjs). Nothing runs
 * tsc: `@ts-check` is for an editor.
 *
 * Go validates every artifact against schema/ before writing it, so nothing
 * here re-checks a shape. What this file refuses is what only it can answer:
 * a 200 carrying an error page, a file cached from before the last deploy,
 * and a chart its own sums contradict.
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
  SCHEMA_VERSION, say, kindLabel, fmt, fmtSigned, el, maybeEl, cssVar, h, link, citations, balancesOf, PARTITION_NOTE, isFundGroup, scheduleOf, isLink, tableRows,
} from "./core.js";
import { SANKEY } from "./sankey.js";
// Re-exported so the tests reach the shipped modules through the one file
// they import; this file calls a form only through the renderer it registers.
export * from "./core.js";
export * from "./sankey.js";

/**
 * A node as schema/projection.schema.json#/properties/nodes/items has it,
 * which scheduleOf assembles from a column document's node table and a
 * schedule's parent edges, plus what the client sets on it. fixedValue: the
 * figure d3-sankey sizes the node at where its drawn ribbons do not add up to
 * it; slot: the column its shaping placed it in.
 * @typedef {Record<string, any> & {fixedValue?: number, slot?: number}} FiscNode
 */
/** schema/mark.schema.json, a node this client makes: an aggregate or a smaller-flows mark. @typedef {Record<string, any>} FiscMark */

/** schema/projection.schema.json#/properties/links/items. @typedef {Record<string, any>} FiscLink */
/** schema/locator.schema.json. @typedef {Record<string, any>} FiscSource */
/** schema/projection.schema.json#/properties/metadata. @typedef {Record<string, any>} FiscMetadata */
/** schema/caveat.schema.json. @typedef {Record<string, any>} FiscCaveat */
/** schema/page.schema.json#/properties/years/items/properties/caveats/items. @typedef {Record<string, any>} FiscCaveatRef */
/** schema/page.schema.json#/properties/docs/additionalProperties. @typedef {Record<string, any>} FiscDoc */
/** schema/page.schema.json#/properties/years/items/properties/hero. @typedef {Record<string, any>} FiscFigure */
/** schema/page.schema.json#/properties/years/items. @typedef {Record<string, any>} FiscYear */
/** schema/page.schema.json, the page's window.FISC_CONFIG. @typedef {Record<string, any>} FiscConfig */
/** schema/page.schema.json#/properties/overview, the page's own chart: its form and hints. @typedef {Record<string, any>} FiscChart */
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
 * What the page is drawing a chart of: the schedule folded and fitted, and
 * the state it is fitted to. frontier: the expansion schedule's node ids
 * drawn; whole: its splits drawn whole rather than capped, node\u001fkey.
 * @typedef {{chart: FiscChart, frontier: ReadonlySet<string> | null, whole: ReadonlySet<string>}} ShapeView
 */

/**
 * What a form module exports: one object app.js hands every chart of that
 * form to, and the only way this file reaches a form. A renderer reads of a
 * chart its own hints, calls core.js for every figure, and computes none of
 * its own. Each member is set out in docs/chart-model.md; the readings over a
 * laid mark (cents, share, reduction, crossTab, reductionNote) are the form's
 * because only the layout that drew the mark knows what its drawn figure and
 * ribbons mean.
 * @typedef {{
 *   form: string,
 *   tiersOf(chart: FiscChart): number[],
 *   tailWord(chart: FiscChart, key: string): string | null,
 *   width(columns: number): number,
 *   columnsOf(drawn: FiscProjection, tiers: number[]): number,
 *   shape(doc: FiscProjection, view: ShapeView): FiscProjection,
 *   layOut(drawn: FiscProjection, ctx: {tiers: number[], columns: number, groupOf: (n: FiscNode | LaidNode) => string, placeOf: (id: string) => number, chart: FiscChart, before?: Map<string, Box>, parentOf?: (id: string) => string}): {nodes: LaidNode[], links: LaidLink[]},
 *   hairlines(graph: {nodes: LaidNode[], links: LaidLink[]}, chart: FiscChart): number,
 *   render(graph: {nodes: LaidNode[], links: LaidLink[]}, ctx: Record<string, any>): () => void,
 *   paint(ctx: {svg: any, colour: {link: (d: LaidLink) => string, node: (d: LaidNode) => string}}): void,
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
 * @param {FiscChart} chart
 * @returns {FormRenderer}
 */
export function formFor(chart) {
  const renderer = FORMS.get(chart.form);
  if (!renderer) throw new Error("no renderer here draws a " + chart.form + " chart");
  return renderer;
}

/**
 * The node tiers this page's own chart draws, coarsest first: the overview's
 * hints, the spine's column order a root of the expansion is placed by. Per
 * view and never a constant here. Absent skips the fold entirely.
 * @type {number[]}
 */
export const RENDER_TIERS = (() => {
  const overview = CONFIG && CONFIG.overview;
  const renderer = overview ? FORMS.get(overview.form) : null;
  return renderer ? renderer.tiersOf(overview) : [];
})();

/**
 * The form of the page's chart when no renderer here draws it, or "" when
 * one does.
 * @returns {string}
 */
export function undrawableForm() {
  const overview = CONFIG.overview || {};
  return FORMS.has(overview.form) ? "" : String(overview.form);
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
 * draw is kept and is inert, so a split or a year change carries it to
 * wherever it applies again; the isolation is cleared where the redraw no
 * longer draws what it names.
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
 * What the reader has split open on the expansion schedule, in the order they
 * did it: a node split by its first declared split ("id"), by another
 * ("id~key", ALTERNATIVE_MARK), or a split's tail drawn out (the tail's own
 * id, core.splitTailID). The frontier and the splits drawn whole are read off
 * it (expansionState), so it is the view, and the hash's `x`. Replaced whole,
 * never mutated, and moved only through redraw.
 * @type {readonly string[]}
 */
export let expansions = [];
/** The expansion schedule's node ids drawn, read off `expansions`; null on any other document. @type {ReadonlySet<string> | null} */
export let frontier = null;
/** The splits drawn whole rather than capped, node\u001fkey, read off `expansions`. @type {ReadonlySet<string>} */
export let wholeSplits = new Set();
/** What separates a node from the split key it was split by, in an entry of `expansions`. */
export const ALTERNATIVE_MARK = "~";

/**
 * The px `100vw` counts that the window does not: body padding plus a classic
 * scrollbar. The page's, not a form's; style.css records it independently as
 * --chart-cushion.
 */
export const CHART_CUSHION = 56;

/**
 * How wide the widest form here lays `n` columns out, in px.
 * @param {number} n
 * @returns {number}
 */
export function widthOf(n) {
  return Math.max(...Array.from(FORMS.values(), (r) => r.width(n)));
}

/**
 * How many columns the chart opens on: one per tier it renders. Its width is
 * the one every chart is drawn at the scale of.
 * @returns {number}
 */
export function openingColumns() {
  return Math.max(1, RENDER_TIERS.length);
}

/**
 * A chart of `n` columns' laid width over the opening chart's: what
 * style.css's --chart-stretch multiplies the card's room by to size the svg.
 * So every chart draws at the one scale the opening chart fills its card at,
 * full size wherever the viewport has room for it, a split moves no column it
 * leaves and rescales none, and a wider chart runs past the card's right edge.
 * @param {number} n
 * @returns {number}
 */
export function chartStretch(n) {
  return widthOf(n) / widthOf(openingColumns());
}

/**
 * The px the rail beside the chart takes when open, its gap to the chart
 * included. style.css records it independently as --rail-width.
 */
export const RAIL_WIDTH = 208;

/**
 * The px the provenance drawer beside the chart takes, its gap to the chart
 * included. style.css records it independently as --drawer-width.
 */
export const DRAWER_WIDTH = 288;

/**
 * The viewport the rail and the drawer sit beside the chart on, and the rail
 * opens by default on: room for the opening chart at full size with both
 * open beside it. Narrower, the three stack and the chart keeps the width.
 */
export const BESIDE_QUERY = "(min-width: " +
  (widthOf(openingColumns()) + CHART_CUSHION + RAIL_WIDTH + DRAWER_WIDTH) + "px)";

/**
 * The year's document as fetched, before any fold: the schedule the page's
 * chart draws, CONFIG.primary out of the year's column. What a split's
 * members and a node's parent are read off.
 * @type {FiscProjection | null}
 */
export let fetched = null;
/**
 * The column document the year on screen was fetched from, one per (fiscal
 * year, basis).
 * @type {any}
 */
export let column = null;
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
 * The nodes as laid out, so shareOf can hand the form a mark's column. Separate
 * from projection because columns are d3-sankey's answer, not the file's.
 * @type {LaidNode[]}
 */
export let laidNodes = [];
/**
 * What puts the chart's moving marks at their end now, as the form's render
 * returned it (finishMotion).
 * @type {() => void}
 */
let motionEnd = () => {};
/**
 * The tallest canvas this page has drawn the chart on, in its own units: a
 * fold never shortens the canvas, so nothing below the chart jumps.
 */
export let heldHeight = 0;
/** How long a redraw's marks move for, in ms, where the reader allows motion. */
export const MOTION_MS = 350;
/** What the drawer holds until the reader lets it go. @type {LaidNode | LaidLink | null} */
export let pinned = null;
/** What the drawer shows under the pointer while nothing is pinned. @type {LaidNode | LaidLink | null} */
export let previewed = null;
/** Whether the rail beside the chart is open; the page's, never the hash's. */
export let railOpen = false;
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
 * Writes the flow count: the marks drawn, not the rows the file holds, and the
 * facts those ribbons cite against the drawn document's own total -- both
 * stated, so the gap the chart cannot show stays visible.
 *
 * A document with no metadata.counts is counted by its ribbons alone rather
 * than dereferenced: a throw here, mid-repaint, splits the page (fisc-bsg).
 */
export function paintCounts() {
  const counts = maybeEl("counts-line");
  if (!counts || !shownYear) return;
  const links = projection ? projection.links.length : shownYear.counts.links;
  const nodes = projection ? projection.nodes.length : shownYear.counts.nodes;
  // PLURALS ARE THE WORDING'S: a filtered chart can draw one ribbon.
  let text = say("counts", { links, nodes, facts: shownYear.counts.facts });
  if (projection) {
    const cited = new Set();
    for (const l of projection.links) for (const id of l.fact_ids) cited.add(id);
    const total = shownYear.counts.facts;
    text = cited.size === total
      ? say("counts", { links, nodes, facts: cited.size })
      : say("counts_partial", { links, nodes, cited: cited.size, facts: total });
  }
  counts.textContent = text;
}

/**
 * Whether the year's document is the expansion schedule, which splits a node
 * in place, rather than a document drawn at a tier set.
 * @returns {boolean}
 */
export function expanding() {
  return core.isExpansionSchedule(fetched);
}

/**
 * One node of the year's document, unfolded, or undefined.
 * @param {string} id
 * @returns {FiscNode | undefined}
 */
export function nodeOf(id) {
  return fetched ? fetched.nodes.find((n) => n.id === id) : undefined;
}

/**
 * Every way a drawn node splits, as Go declared it, first declared first;
 * none for a tail, for a node of any other document, and before a year has
 * drawn.
 * @param {{id: string}} d
 * @returns {{key: string, form: string, alternative: boolean, members: string[]}[]}
 */
export function splitsHere(d) {
  if (!expanding() || !fetched || core.isAggregate(d.id)) return [];
  return core.splitsOf(fetched, d.id);
}

/**
 * What one member of the split a node belongs to is called, in the page's
 * words (core.memberNoun): "" for a node of any other document, and for one
 * no split holds. A label the city prints for a member in its parent's own
 * words is told apart by it.
 * @param {string} id
 * @returns {string}
 */
export function memberNounOf(id) {
  return expanding() && fetched ? core.memberNoun(fetched, id) : "";
}

/**
 * The words a split's members are called in, from the page's split_labels.
 * Refused, not guessed, for a key the page publishes none for.
 * @param {string} key
 * @returns {string}
 */
export function splitWords(key) {
  const words = (CONFIG.split_labels || {})[key];
  if (!words) throw new Error("cannot name split " + key + ": the page publishes no words for it");
  return words;
}

/**
 * The entry of `expansions` that splits `id` by `key`: the id alone for its
 * first declared split, so the hash of an ordinary split carries no key.
 * @param {string} id
 * @param {string} key
 * @returns {string}
 */
export function entryOf(id, key) {
  const splits = fetched ? core.splitsOf(fetched, id) : [];
  return splits.length && splits[0].key === key ? id : id + ALTERNATIVE_MARK + key;
}

/**
 * What one entry of `expansions` says, against the expansion schedule: a node
 * split by a declared split, or a split's tail drawn out. Refused, naming the
 * entry, where it names a node with no such split, or a tail of no split.
 * @param {FiscProjection} doc
 * @param {string} entry
 * @returns {{kind: "split", id: string, key: string} | {kind: "tail", id: string, node: string, key: string}}
 */
export function parseEntry(doc, entry) {
  const tail = core.splitTailOf(entry);
  if (tail) {
    if (!core.splitsOf(doc, tail.node).some((sp) => sp.key === tail.key)) {
      throw new Error(entry + " is the tail of no split " + doc.projection + " declares");
    }
    return { kind: "tail", id: entry, node: tail.node, key: tail.key };
  }
  const at = entry.indexOf(ALTERNATIVE_MARK);
  const id = at < 0 ? entry : entry.slice(0, at);
  const splits = core.splitsOf(doc, id);
  const key = at < 0 ? (splits.length ? splits[0].key : "") : entry.slice(at + 1);
  if (!splits.some((sp) => sp.key === key)) {
    throw new Error(doc.projection + " declares no split of " + id + (at < 0 ? "" : " by " + key));
  }
  return { kind: "split", id, key };
}

/**
 * The frontier and the splits drawn whole that a list of expansions comes to
 * on the expansion schedule, each entry applied in order from the root
 * frontier (core.expand). Refused, naming the entry, where one names a node
 * the frontier before it does not draw, or a tail of a split not open.
 * @param {FiscProjection} doc
 * @param {readonly string[]} entries
 * @returns {{frontier: Set<string>, whole: Set<string>}}
 */
export function expansionState(doc, entries) {
  let open = core.rootFrontier(doc);
  const drawnWhole = new Set();
  for (const entry of entries) {
    const e = parseEntry(doc, entry);
    if (e.kind === "tail") {
      if (core.openSplitOf(doc, open, e.node) !== e.key) {
        throw new Error(entry + " draws out a split of " + e.node + " the chart does not have open");
      }
      drawnWhole.add(e.node + "\u001f" + e.key);
      continue;
    }
    if (!open.has(e.id)) throw new Error(e.id + " is not drawn where it is split");
    open = core.expand(doc, open, e.id, e.key);
  }
  return { frontier: open, whole: drawnWhole };
}

/**
 * Whether an entry of `expansions` is about `id` or a node inside it: what
 * folding `id` back, or splitting it another way, takes off the list.
 * @param {string} entry
 * @param {Set<string>} inside core.withinNode of the node
 * @returns {boolean}
 */
function entryInside(entry, inside) {
  if (!fetched) return false;
  const e = parseEntry(fetched, entry);
  return inside.has(e.kind === "tail" ? e.node : e.id);
}

/**
 * Whether activating this node splits it: the expansion schedule declares a
 * split of it, and the chart draws it with none open. A tail draws out
 * instead (expandable), so the two are disjoint.
 * @param {{id: string}} d
 * @returns {boolean}
 */
export function splittable(d) {
  if (!frontier || !fetched || !frontier.has(d.id) || !splitsHere(d).length) return false;
  return core.openSplitOf(fetched, frontier, d.id) === "";
}

/**
 * Whether a mark is a folded tail this chart can draw out at full length: a
 * split's tail, on the expansion schedule.
 * @param {{id: string}} d
 * @returns {boolean}
 */
export function expandable(d) {
  return core.isAggregate(d.id) && expanding() && Boolean(core.splitTailOf(d.id));
}

/**
 * Whether focus is in the chart or its chip row, asked while its element
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
 * Asks of every node of the chart on screen what the paints ask per mark --
 * whether it splits, whether it draws out, and what a member printed in its
 * parent's words is called -- so a declaration those
 * questions would throw on is met inside the caller's rollback rather than
 * thrown from paintChartHint or render over a page already half repainted.
 */
export function askAffordances() {
  if (!projection) return;
  for (const n of projection.nodes) {
    splittable(n);
    expandable(n);
    if (core.labelledAsParent(groupIndex, n.id)) memberNounOf(n.id);
  }
}

/**
 * Draws the chart at `next`, a list of expansions, and repaints everything it
 * decides, or leaves the page exactly as it was and tells the reader why.
 *
 * Everything that can throw on a served document -- the expansions read back,
 * shape, lay-out, table rows and what the paints ask (askAffordances) -- runs
 * in the try while the page is still the reader's, and the state is restored
 * whole on a throw. The pin is cleared because the panel holds a mark of the
 * chart replaced, and the isolation where the redraw draws nothing it names.
 *
 * @param {readonly string[]} next
 * @param {string} [refused] the banner's opening words when the redraw is refused
 * @param {() => string} [landOn] the mark focus goes to on the redrawn chart,
 *   asked after it is laid out
 * @param {string} [anchor] the node the reader acted on, kept where it is on
 *   screen (keepInPlace)
 * @returns {boolean} whether `next` is on screen
 */
export function redraw(next, refused = "That could not be drawn", landOn, anchor) {
  finishMotion();
  // Asked before the repaint removes the element focus is on.
  const hadFocus = focusInChart();
  const prior = boxesOf(laidNodes);
  const held = anchor ? screenBoxOf(anchor) : null;
  const was = { expansions, frontier, wholeSplits, laidNodes, groupIndex, projection, shown, whole };
  let laid;
  let rows;
  try {
    if (!fetched) throw new Error("there is no chart on screen");
    if (expanding()) {
      const state = expansionState(fetched, next);
      frontier = state.frontier;
      wholeSplits = state.whole;
    }
    expansions = next;
    const drawn = shapeFor(fetched);
    const filtered = filterFor(drawn);
    const unfiltered = wholeOf(drawn, filtered);
    laid = layOut(filtered.doc, prior);
    // Inside the try: building a row is the last step of a draw that can
    // throw on a document. The table is the view's every flow, filtered or
    // not; the filter line says what the chart leaves out.
    rows = tableRows(drawn);
    projection = drawn;
    shown = filtered;
    whole = unfiltered;
    askAffordances();
  } catch (e) {
    ({ expansions, frontier, wholeSplits, laidNodes, groupIndex, projection, shown, whole } = was);
    fail(refused + ": " + (e instanceof Error ? e.message : String(e)));
    return false;
  }
  clearRefusal();
  // What the last year switch folded back is said until the reader moves.
  paintSplitsLine(null, [], null);
  pinned = null;
  if (filter.isolated && !laidNodes.some((n) => isolates(filter.isolated, n))) {
    filter = Object.freeze(Object.assign({}, filter, { isolated: "" }));
  }
  resetDetail();
  hideTip();
  paintChips();
  paintChartName();
  paintCounts();
  buildLegend();
  paintFilterLine();
  paintChartHint();
  buildDerivedList();
  buildTable(rows);
  render(laid, prior);
  if (anchor && held) keepInPlace(anchor, held);
  restoreFocus(hadFocus, landOn ? landOn() : "");
  // A split and its undoing are steps a reader walks back with Back.
  recordView("push");
  return true;
}

/**
 * Splits a drawn node by one of its declared splits, its first by default:
 * the members drawn beside it, and every other node left as it is. A node
 * split another way is folded back first, so the list holds one way per
 * node. Focus goes to the topmost new member.
 * @param {string} id
 * @param {string} [key]
 * @returns {boolean} whether the split is on screen
 */
export function expandNode(id, key) {
  if (!fetched) return false;
  const splits = core.splitsOf(fetched, id);
  const split = key === undefined ? splits[0] : splits.find((sp) => sp.key === key);
  if (!split) return false;
  // Open already, by its own entry or straight through its parent's: the
  // list takes no entry that draws nothing.
  if (frontier && frontier.has(id) && core.openSplitOf(fetched, frontier, id) === split.key) return true;
  const inside = core.withinNode(fetched, id);
  const next = expansions.filter((e) => !entryInside(e, inside)).concat([entryOf(id, split.key)]);
  const before = new Set(laidNodes.map((n) => n.id));
  const members = new Set(split.members.concat([core.splitTailID(id, split.key)]));
  return redraw(next, "That could not be split", () => firstNewAt((n) => members.has(n.id), before), id);
}

/**
 * Folds a split node back into one mark: its entry and every entry inside it
 * taken off the list, from the node it opened straight through
 * (core.straightFrom), so a node opened with its parent folds with it. Focus
 * goes back to that node.
 * @param {string} id
 * @returns {boolean} whether anything was folded
 */
export function collapseNode(id) {
  if (!fetched) return false;
  const from = core.straightFrom(fetched, id);
  const inside = core.withinNode(fetched, from);
  const next = expansions.filter((e) => !entryInside(e, inside));
  if (next.length === expansions.length) return false;
  return redraw(next, "That could not be folded back", () => from, from);
}

/**
 * Folds back the node a mark was split out of: its parent, or the node a
 * tail's split belongs to. The Alt or Shift double click.
 * @param {{id: string}} d
 * @returns {boolean} whether anything was folded
 */
export function collapseParentOf(d) {
  const tail = core.splitTailOf(d.id);
  const own = nodeOf(d.id);
  const parent = tail ? tail.node : own ? own.parent : "";
  return Boolean(parent) && collapseNode(parent);
}

/**
 * Undoes the last expansion the reader made: Escape. Focus goes to the node
 * it split, or to the tail it drew out.
 * @returns {boolean} whether anything was undone
 */
export function undoLast() {
  if (!expansions.length || !fetched) return false;
  const last = parseEntry(fetched, expansions[expansions.length - 1]);
  return redraw(expansions.slice(0, -1), "That could not be undone", () => last.id, last.id);
}

/**
 * Splits every node the chart draws unsplit by its first declared split, and
 * every node that exposes, until none is left: the whole budget at its finest
 * grain, its tails still folded.
 * @returns {boolean} whether the chart moved
 */
export function expandAll() {
  if (!fetched || !frontier) return false;
  let open = frontier;
  const next = expansions.slice();
  for (let pass = 0; pass < 16; pass++) {
    const shut = fetched.nodes.filter((n) => open.has(n.id) && core.splitsOf(fetched, n.id).length &&
      core.openSplitOf(fetched, open, n.id) === "");
    if (!shut.length) break;
    for (const n of shut) {
      const key = core.splitsOf(fetched, n.id)[0].key;
      next.push(entryOf(n.id, key));
      open = core.expand(fetched, open, n.id, key);
    }
  }
  if (next.length === expansions.length) return false;
  return redraw(next, "That could not be split");
}

/**
 * Folds every split back: the chart as the page opens.
 * @returns {boolean} whether the chart moved
 */
export function collapseAll() {
  if (!expansions.length) return false;
  return redraw([], "That could not be folded back");
}

/**
 * Draws out a split's folded tail at every member it holds: the split drawn
 * whole. Focus goes to the topmost member it drew.
 * @param {{id: string}} d
 * @returns {boolean} whether it is drawn out
 */
export function drawOutTail(d) {
  const tail = core.splitTailOf(d.id);
  if (!expanding() || !tail || !fetched) return false;
  const split = core.splitsOf(fetched, tail.node).find((sp) => sp.key === tail.key);
  const members = new Set(split ? split.members : []);
  const before = new Set(laidNodes.map((n) => n.id));
  return redraw(expansions.concat([d.id]), "That could not be drawn out",
    () => firstNewAt((n) => members.has(n.id), before));
}

/**
 * The topmost laid mark `among` holds that `before` does not, else the
 * topmost `among` holds, else "": where focus goes after a split or a tail is
 * drawn out, so the next key acts on what the reader asked for.
 * @param {(n: LaidNode) => boolean} among
 * @param {Set<string>} before the ids laid before the redraw
 * @returns {string}
 */
export function firstNewAt(among, before) {
  const marks = laidNodes.filter(among).sort((a, b) => a.y0 - b.y0);
  const fresh = marks.find((n) => !before.has(n.id));
  return fresh ? fresh.id : marks.length ? marks[0].id : "";
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
 * One click on a node: it follows that node's money, whether or not it splits.
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
 * A double click on a node: it splits the node, or with Alt or Shift held
 * folds back the node it was split out of, having first put back whatever the
 * two clicks underneath it isolated.
 *
 * THE RECORD IS SPENT WHETHER OR NOT ANYTHING SPLIT, so a later double click
 * cannot restore a state two gestures old.
 * @param {LaidNode} d
 * @param {number} at the event's timestamp
 * @param {boolean} [fold] Alt or Shift was held
 */
export function doubleClickNode(d, at, fold = false) {
  if (d.id === clickIsolate.id && at - clickIsolate.at < ACTIVATION_WINDOW) {
    setIsolated(clickIsolate.was);
  }
  clickIsolate = { id: "", at: -Infinity, was: "" };
  if (fold) {
    collapseParentOf(d);
    return;
  }
  openOffered(d);
}

/**
 * Does what activating a node offers, and says whether it offered anything:
 * the node split by its first declared split (splittable), else the folded
 * tail drawn out (expandable). The one rule Enter on a mark, a double click
 * and the search's Enter share.
 * @param {{id: string}} d
 * @returns {boolean}
 */
export function openOffered(d) {
  if (splittable(d)) {
    expandNode(d.id);
    return true;
  }
  if (expandable(d)) {
    drawOutTail(d);
    return true;
  }
  return false;
}

/**
 * Enter or Space on a node: Enter splits a node that splits, and Space follows
 * the money.
 *
 * SPACE NEVER SPLITS, against the role="button" convention, so nodeDescription,
 * paintChartHint and aria-keyshortcuts all announce it. Enter falls back to
 * the isolate on a node that does not split, so no button has a dead Enter.
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
 * Puts focus somewhere real after a redraw has replaced the focused element.
 *
 * ONLY IF FOCUS WAS ALREADY IN THE CHART, and the caller must say so: by the
 * time this runs the repaint has detached the focused element, so
 * document.activeElement no longer knows.
 *
 * @param {boolean} hadFocus whether focus was inside the chart before the
 *   repaint that just replaced it.
 * @param {string} [onto] the id of the mark focus lands on, or "" for the
 *   first mark
 */
export function restoreFocus(hadFocus, onto = "") {
  if (!hadFocus) return;
  // WITHOUT PINNING: otherwise the next Escape spends itself clearing that pin
  // instead of undoing the next split. The focus handlers ask `restoring`.
  restoring = true;
  try {
    restoreFocusTo(onto);
  } finally {
    restoring = false;
  }
}

/** Whether focus is being restored by restoreFocus rather than moved by the reader. */
let restoring = false;

/**
 * restoreFocus's choice of target, made while `restoring` is set.
 * @param {string} onto
 */
function restoreFocusTo(onto) {
  // focus() is on HTMLElement and SVGElement, not Element; the SVG <g> marks
  // are not obliged to have it, so the runtime test stays.
  const focus = (/** @type {Element | null} */ target) => {
    const el = /** @type {any} */ (target);
    if (!el || typeof el.focus !== "function") return false;
    el.focus();
    return true;
  };
  const chart = maybeEl("chart");
  const named = onto && chart
    ? D3.select(chart).selectAll("g.node").filter(/** @param {LaidNode} d */ (d) => d.id === onto).node()
    : null;
  if (focus(named)) return;
  focus(chart ? chart.querySelector("g.node") : null);
}

/**
 * Names the chart for a screen reader: the year's own title, and the
 * description the page shipped.
 */
export function paintChartName() {
  const title = maybeEl("chart-title");
  if (title && shownYear && shownYear.chart_title) title.textContent = shownYear.chart_title;
  const desc = maybeEl("chart-desc");
  if (!desc) return;
  if (!baseDescription) baseDescription = desc.textContent;
  desc.textContent = baseDescription;
}

/**
 * Says what a click does, for the chart on screen.
 *
 * WHETHER ANYTHING SPLITS IS ASKED OF THE DRAWN NODES, not of the declared
 * splits: the filters can remove every node that would.
 */
export function paintChartHint() {
  const hint = maybeEl("chart-hint");
  if (!hint) return;
  if (!expanding()) return;
  const chart = chartDrawn();
  const anyOpens = Boolean(chart) && chart.nodes.some(splittable);
  // The isolate is named on every view, and Space with it, because Space
  // never splits against the role="button" convention.
  const follow = " " + say("follow");
  // A sentence of its own: the tail can be the only thing a chart offers.
  const tails = Boolean(chart) && chart.nodes.some(expandable);
  const expands = tails ? " " + say("expand") : "";
  const folds = expansions.length ? " " + say("fold_back") : "";
  const swatch = buildLegendCount() ? " " + say("swatch") : "";
  hint.textContent = (anyOpens ? say("open_into") : say("nothing_opens")) + follow + expands + folds + swatch;
}

/** How many fund-group swatches the legend is showing. */
export function buildLegendCount() {
  const legend = maybeEl("legend");
  return legend ? legend.children.length : 0;
}

/**
 * Draws the chip row above the chart: on the expansion, Expand all and
 * Collapse all, then one chip per node split open, in the order the reader
 * split them, each folding its node back, with a chooser beside a node that
 * splits more than one way, and one chip per tail drawn out; on any other
 * document, nothing. The only always-visible way to fold a split back, since
 * splitting can remove the node that was clicked.
 */
export function paintChips() {
  const bar = maybeEl("breadcrumb");
  if (!bar) return;
  /** @type {HTMLElement[]} */
  const items = [];
  if (expanding() && fetched) {
    const bulk = (/** @type {string} */ words, /** @type {boolean} */ live, /** @type {() => void} */ act) => {
      const b = h("button", "crumb-bulk", words);
      b.setAttribute("type", "button");
      if (!live) b.setAttribute("disabled", "");
      b.addEventListener("click", () => guarded("redraw the chart", act));
      return b;
    };
    const shut = Boolean(projection) && /** @type {FiscProjection} */ (projection).nodes.some(splittable);
    items.push(bulk(say("expand_all"), shut, expandAll), bulk(say("collapse_all"), expansions.length > 0, collapseAll));
    const doc = fetched;
    const entries = expansions.map((entry) => parseEntry(doc, entry));
    const labelOf = (/** @type {string} */ id) => {
      const n = nodeOf(id);
      return n ? n.label : id;
    };
    // THE COLLISION IS THE CITY'S: two nodes split open that it prints in the
    // same words -- a fund group and its one fund -- are each told apart by
    // what they split into.
    const splitLabels = entries.filter((e) => e.kind === "split").map((e) => labelOf(e.id));
    for (const e of entries) {
      if (e.kind === "tail") {
        items.push(tailChip(e.node, e.key));
        continue;
      }
      const label = labelOf(e.id);
      const twice = splitLabels.filter((l) => l === label).length > 1;
      items.push(splitChip(e.id, e.key, twice ? label + " (" + splitWords(e.key) + ")" : label));
    }
  }
  bar.replaceChildren(...items);
  if (items.length) bar.removeAttribute("hidden");
  else bar.setAttribute("hidden", "");
}

/**
 * One chip of the chip row: a button saying `text`, named `label` for a
 * screen reader since the glyph is not the label, doing `act`.
 * @param {string} text
 * @param {string} label
 * @param {() => void} act
 * @returns {HTMLElement}
 */
export function foldBackChip(text, label, act) {
  const chip = h("button", "crumb-expanded", text);
  chip.setAttribute("type", "button");
  chip.setAttribute("aria-label", label);
  chip.addEventListener("click", () => guarded("fold this back", act));
  return chip;
}

/**
 * A node split open's chip, which folds it back, and beside a node that
 * splits more than one way the chooser between its splits, each a button
 * pressed for the split drawn.
 * @param {string} id
 * @param {string} key
 * @param {string} label what the chip calls the node
 * @returns {HTMLElement}
 */
function splitChip(id, key, label) {
  const chip = foldBackChip(say("chip_fold", { label }),
    say("chip_fold_label", { label, split: splitWords(key) }), () => collapseNode(id));
  const splits = fetched ? core.splitsOf(fetched, id) : [];
  if (splits.length < 2) return chip;
  const group = h("span", "crumb-group");
  group.setAttribute("role", "group");
  group.setAttribute("aria-label", say("split_chooser", { label }));
  group.append(chip, ...splits.map((sp) => {
    const choice = h("button", "crumb-choice", splitWords(sp.key));
    choice.setAttribute("type", "button");
    choice.setAttribute("aria-pressed", String(sp.key === key));
    choice.addEventListener("click", () => guarded("split this mark", () => {
      if (sp.key !== key) expandNode(id, sp.key);
    }));
    return choice;
  }));
  return group;
}

/**
 * A split drawn out of its tail's chip, which folds the smallest back into
 * the tail. Its count is the members drawn, read off the chart, less a
 * derived remainder, which is not one of the split's word.
 * @param {string} node
 * @param {string} key
 * @returns {HTMLElement}
 */
function tailChip(node, key) {
  const n = nodeOf(node);
  const label = n ? n.label : node;
  const split = fetched ? core.splitsOf(fetched, node).find((sp) => sp.key === key) : undefined;
  const drawn = new Set(projection ? projection.nodes.map((x) => x.id) : []);
  const count = split ? split.members.filter((m) => drawn.has(m)).length : 0;
  const word = formOnScreen().tailWord(chartOnScreen(), key) || splitWords(key);
  const id = core.splitTailID(node, key);
  return foldBackChip(say("tail_chip", { label, n: count, word }), say("tail_chip_label", { label, n: count, word }),
    () => redraw(expansions.filter((e) => e !== id), "That could not be folded back", () => id));
}

/**
 * The document as this page draws it, through its form: the expansion folded
 * to the frontier, or another document folded to the page's tier set.
 *
 * @param {FiscProjection} doc
 * @returns {FiscProjection}
 */
export function shapeFor(doc) {
  return formOnScreen().shape(doc, { chart: CONFIG.overview, frontier, whole: wholeSplits });
}

/**
 * The chart on screen's declaration: the page's own.
 * @returns {FiscChart}
 */
export function chartOnScreen() {
  return CONFIG.overview;
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
 * answered by splittable and expandable rather than read off the node; the
 * two predicates are disjoint, which the class does not enforce.
 * @param {LaidNode} d
 * @returns {string}
 */
export function nodeClass(d) {
  return "node" + (d.derived ? " derived" : "") + (formOnScreen().reduction(d) ? " contra" : "") +
    (splittable(d) ? " opens" : "") + (expandable(d) ? " expands" : "");
}

/**
 * The glyphs one node's label carries in its flag tspan: glyphs, not words,
 * because the label is at the edge of its gutter, and a glyph survives
 * forced-colors and greyscale. They compose, so a pair costs two glyph widths;
 * splittable and expandable are disjoint, so no mark carries three.
 * @param {LaidNode} d
 * @returns {string}
 */
export function nodeFlags(d) {
  const marks = (d.derived ? "◇" : "") + (splittable(d) ? "▸" : "") +
    (expandable(d) ? "⊞" : "");
  return marks ? "  " + marks : "";
}

/**
 * Lays a shaped document out through its form, binding the page: the hue
 * index is the drawn nodes overwritten by the unfolded document the chart was
 * shaped from, because a colour is where a node really sits and the fold
 * re-points parents; the drawn set contributes only what the file lacks, such
 * as a tail. Assigned only once the layout has not thrown, or the next paint()
 * would recolour the chart on screen against a document it was not drawn from.
 * @param {FiscProjection} doc
 * @param {Map<string, Box>} [prior] the chart it replaces, whose columns it keeps the order of
 * @returns {{nodes:LaidNode[], links:LaidLink[]}}
 */
export function layOut(doc, prior) {
  const { graph, index } = laidOf(doc, prior);
  groupIndex = index;
  // For shareOf: only the laid graph knows a mark's column.
  laidNodes = graph.nodes;
  return graph;
}

/**
 * layOut's layout, assigning nothing: the graph the form lays `doc` out as,
 * and the hue index it was laid against.
 * @param {FiscProjection} doc
 * @param {Map<string, Box>} [prior] layOut's
 * @returns {{graph: {nodes:LaidNode[], links:LaidLink[]}, index: Map<string, FiscNode>}}
 */
function laidOf(doc, prior) {
  const index = hueIndex(doc);
  // Nothing to lay out where the filters removed every ribbon; d3-sankey
  // throws on an empty graph.
  const graph = !doc.links.length ? { nodes: [], links: [] } : formOnScreen().layOut(doc, {
    tiers: RENDER_TIERS,
    columns: layoutColumns(doc),
    groupOf: (/** @type {FiscNode | LaidNode} */ n) => core.fundGroupOf(index, n),
    placeOf: fundGroupPlace,
    chart: chartOnScreen(),
    before: prior,
    parentOf: parentOf,
  });
  return { graph, index };
}

/**
 * How many columns wide a shaped chart is laid out: the form's count of the
 * columns its nodes are placed in, which a filter can lower.
 * @param {FiscProjection} doc
 * @returns {number}
 */
export function layoutColumns(doc) {
  return formOnScreen().columnsOf(doc, RENDER_TIERS);
}

/**
 * layOut's hue index for a document: its nodes overwritten by the unfolded
 * document it was shaped from (fetched).
 * @param {FiscProjection} doc
 * @returns {Map<string, FiscNode>}
 */
export function hueIndex(doc) {
  const index = new Map(doc.nodes.map((n) => [n.id, n]));
  if (fetched) {
    for (const n of fetched.nodes) index.set(n.id, n);
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
 * figures summed as each is stated (a reduction negative) with each cell
 * once (core.distinctCents), and every fact they cite. A client sum over
 * cited cents, and the only one the filter line states.
 * @param {{hidden: FiscLink[]} | null} [result]
 * @returns {{flows: number, cents: number, facts: Set<string>}}
 */
export function hiddenTally(result = shown) {
  const facts = new Set();
  const hidden = result ? result.hidden : [];
  const form = formOnScreen();
  for (const l of hidden) for (const id of l.fact_ids) facts.add(id);
  const cents = core.distinctCents(hidden, (l) => form.cents(/** @type {any} */ (l)), core.leafCents(fetched));
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
 *
 * The form draws into the one group the zoom moves, kept from chart to
 * chart so a mark keeps its element; `prior`, the chart replaced, is what
 * the marks move from, over MOTION_MS unless the reader asks for less motion.
 * @param {{nodes:LaidNode[], links:LaidLink[]}} [laid]
 * @param {Map<string, Box>} [prior]
 */
export function render(laid, prior) {
  const doc = chartDrawn();
  if (!doc) return;
  finishMotion();
  const graph = laid || layOut(doc);
  const svg = D3.select("#chart");
  // Never the svg, whose viewBox is the chart's own coordinates.
  let layer = svg.select(":scope > g.zoom-layer");
  if (layer.empty()) layer = svg.append("g").attr("class", "zoom-layer");
  const columns = layoutColumns(doc);
  // style.css's --chart-room reads these: the card breaks out of main to the
  // opening chart's width whatever the columns, so its left edge never moves,
  // and the svg is that room stretched by the columns drawn.
  document.documentElement.style.setProperty("--chart-max", widthOf(openingColumns()) + "px");
  document.documentElement.style.setProperty("--chart-stretch", String(chartStretch(columns)));
  const ended = formOnScreen().render(graph, {
    svg: svg,
    layer: layer,
    before: prior,
    parentOf: parentOf,
    motion: prior && prior.size ? motionMs() : 0,
    held: heldHeight,
    tiers: RENDER_TIERS,
    columns: columns,
    chart: chartOnScreen(),
    // The unfolded nodes the chart was folded from, which a same-word mark's
    // qualifier is read off (labelQualifiers).
    doc: { nodes: Array.from(groupIndex.values()) },
    noun: memberNounOf,
    classes: { link: linkClass, node: nodeClass, flags: nodeFlags },
    describe: { link: linkDescription, node: nodeDescription },
    figure: markFigure,
    on: {
      tip: hover, hide: hideTip, pin: pin, guarded: guarded,
      click: clickNode, dblclick: doubleClickNode, key: keyNode,
      restoring: () => restoring,
    },
  });
  motionEnd = typeof ended === "function" ? ended : () => {};
  const box = String(svg.attr("viewBox") || "").split(/\s+/).map(Number);
  if (box.length === 4 && box[3] > heldHeight) heldHeight = box[3];
  paintHairlines(graph);
  paint();
  applyEmphasis();
  // A new chart starts unzoomed: the transform was about the marks it replaced.
  bindZoom();
  // The options name marks of the chart just replaced.
  closeSearch();
}

/**
 * A laid node's box in the chart's own coordinates.
 * @typedef {{x0: number, x1: number, y0: number, y1: number}} Box
 */

/**
 * Laid nodes' boxes by id: what a redraw keeps the order of and moves from.
 * @param {LaidNode[]} nodes
 * @returns {Map<string, Box>}
 */
export function boxesOf(nodes) {
  return new Map(nodes.map((n) => [n.id, { x0: n.x0, x1: n.x1, y0: n.y0, y1: n.y1 }]));
}

/**
 * The node a mark hangs from in the schedule on screen: a tail's split node,
 * else its parent, else "". What a new mark grows out of and a leaving one
 * shrinks into.
 * @param {string} id
 * @returns {string}
 */
export function parentOf(id) {
  const tail = core.splitTailOf(id);
  if (tail) return tail.node;
  const n = nodeOf(id);
  return n && n.parent ? n.parent : "";
}

/**
 * Whether the reader allows motion: false under prefers-reduced-motion.
 * @returns {number} how long a redraw's marks move for, in ms
 */
export function motionMs() {
  const reduced = typeof window.matchMedia === "function" &&
    window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  return reduced ? 0 : MOTION_MS;
}

/** Puts every moving mark of the chart at its end now. */
export function finishMotion() {
  const end = motionEnd;
  motionEnd = () => {};
  end();
}

/**
 * Where a node's mark is on screen, in px, or null where none is drawn.
 * @param {string} id
 * @returns {{left: number, top: number} | null}
 */
function screenBoxOf(id) {
  const svg = maybeEl("chart");
  if (!svg) return null;
  for (const g of svg.querySelectorAll("g.nodes > g")) {
    const d = /** @type {any} */ (g).__data__;
    if (!d || d.id !== id) continue;
    const r = (g.querySelector("rect") || g).getBoundingClientRect();
    // A mark with no size on screen is one the browser has not laid out.
    return r.width || r.height ? { left: r.left, top: r.top } : null;
  }
  return null;
}

/**
 * How far to scroll so a node laid at `d` in a viewBox `laid` wide, drawn in
 * an svg at `svg` on screen with the zoom at the whole chart, is where it was
 * on screen (`was`), in px: sideways in the chart's scroller, and down the
 * page.
 * @param {{left: number, top: number}} was
 * @param {{left: number, top: number, width: number}} svg
 * @param {number} laid the viewBox's width
 * @param {{x0: number, y0: number}} d
 * @returns {{dx: number, dy: number}}
 */
export function anchorShift(was, svg, laid, d) {
  const k = laid > 0 ? svg.width / laid : 0;
  if (!(k > 0)) return { dx: 0, dy: 0 };
  return { dx: svg.left + d.x0 * k - was.left, dy: svg.top + d.y0 * k - was.top };
}

/**
 * Scrolls the chart's scroller and the page so the node the reader acted on
 * stays where it was on screen (anchorShift).
 * @param {string} id
 * @param {{left: number, top: number}} was
 */
function keepInPlace(id, was) {
  const svg = maybeEl("chart");
  const d = laidNodes.find((n) => n.id === id);
  if (!svg || !d) return;
  const box = String(svg.getAttribute("viewBox") || "").split(/\s+/).map(Number);
  const r = svg.getBoundingClientRect();
  const { dx, dy } = anchorShift(was, { left: r.left, top: r.top, width: r.width }, box[2], d);
  const scroller = svg.parentElement;
  if (dx && scroller && scroller.classList.contains("chart-scroll")) {
    scroller.scrollLeft = Math.max(0, scroller.scrollLeft + dx);
  }
  if (dy && typeof window.scrollBy === "function") window.scrollBy(0, dy);
}

/**
 * Says beside the chart how many of its ribbons the form draws as hairlines,
 * wider than their share, and hides the sentence where it draws none.
 * @param {{nodes: LaidNode[], links: LaidLink[]}} graph
 */
export function paintHairlines(graph) {
  const line = maybeEl("hairline-line");
  if (!line) return;
  const n = formOnScreen().hairlines(graph, chartOnScreen());
  line.hidden = n === 0;
  line.textContent = n ? say("hairline", { n }) : "";
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
 * fetched and nothing split or folded, and a change of the isolation alone
 * repaints the emphasis. A redraw the chart refuses -- a filter that cuts
 * every ribbon into a column the expansion placed after another, which
 * layOutAt refuses -- leaves the old state, chart and panel exactly as they
 * were and says why.
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
  finishMotion();
  const hadFocus = focusInChart();
  const prior = boxesOf(laidNodes);
  let filtered;
  let unfiltered;
  let laid;
  try {
    filtered = filterFor(projection);
    unfiltered = wholeOf(projection, filtered);
    laid = layOut(filtered.doc, prior);
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
  render(laid, prior);
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
 * Reset. Open by default where the rail is (BESIDE_QUERY); the reader's own
 * toggle wins after that.
 */
export function wireFilters() {
  const panel = /** @type {HTMLDetailsElement | null} */ (maybeEl("filters"));
  if (!panel) return;
  panel.removeAttribute("hidden");
  if (besideMatches()) panel.open = true;
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
  // words of the chip that undoes it, since it splits nothing.
  const what = splittable(d) ? say("desc_opens") : expandable(d) ? say("desc_expands") : say("desc_follows");
  const form = formOnScreen();
  const mark = markOfView(d);
  const note = form.reductionNote(mark);
  // The cross-tab qualification, for a reader who cannot see the ribbons.
  const drawn = drawnAt(d);
  const gone = hiddenAt(d.id);
  return d.label + ", total " + fmtSigned(markFigure(d)) +
    (drawn ? ", " + drawn : "") +
    ", " + say(d.derived ? "inferred_by_us" : "printed_by_city") +
    (form.crossTab(mark) ? ", " + PARTITION_NOTE : "") +
    (note ? ", " + note.replace(/^\u25c7 /, "") : "") + (gone ? ", " + gone : "") + what;
}

/**
 * Where a caveat's full text is, or "" when this site has no caveats page.
 *
 * LOOKED UP IN THE REFS GO HANDED THE PAGE, never rebuilt from the id: the
 * anchor is per (document, caveat). A caveat the year carries no ref for gets
 * no anchor: a missing link is visible, a link to the wrong document is not.
 *
 * @param {string} id
 * @returns {string}
 */
export function caveatHref(id) {
  const refs = shownYear ? shownYear.caveats : [];
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
  // Every document is scheduleOf's, which gives metadata a caveats list.
  return projection.metadata.caveats.filter((c) =>
    Array.isArray(c.applies_to) && c.applies_to.some(reaches));
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
      meta.append(h("span", "chip caveat", say("caveat_chip", { n: cavs.length })));
    }
    tip.append(meta);
    for (const b of balancesOf(n)) {
      const said = h("div", "tip-meta", b.words);
      if (b.derived) said.append(" ", h("span", "chip derived", say("inferred_chip")));
      tip.append(said);
    }
    if (n.rationale) tip.append(h("div", "tip-meta", n.rationale));
    const note = form.reductionNote(markOfView(n));
    if (note) tip.append(h("div", "tip-meta", note));
    for (const c of cavs) tip.append(h("div", "tip-meta", "\u26a0 " + c.summary));
    const gone = hiddenAt(n.id);
    if (gone) tip.append(h("div", "tip-meta", gone));
  }
  tip.append(h("div", "tip-meta", "Select for sources."));

  tip.hidden = false;
  const wrap = tip.parentElement;
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
 * The pointer on a mark: its tooltip, and its provenance previewed in the
 * drawer. Focus is not a hover: it pins.
 * @param {Event} event
 * @param {LaidLink | LaidNode} d
 */
export function hover(event, d) {
  showTip(event, d);
  if (typeof event.type === "string" && event.type.startsWith("pointer")) preview(d);
}

/**
 * Shows a mark in the drawer while nothing is pinned. Nothing the pointer
 * crosses replaces a pin, and the last mark previewed stays until another
 * is, so the pointer can leave the chart for the drawer's links.
 * @param {LaidLink | LaidNode} d
 */
export function preview(d) {
  if (pinned || previewed === d) return;
  previewed = d;
  paintDetail(d);
}

/**
 * Pins a flow or node into the drawer, where the citations are clickable,
 * until the reader pins another or lets it go.
 * @param {LaidLink | LaidNode} d
 */
export function pin(d) {
  pinned = d;
  previewed = null;
  paintDetail(d);
  applyEmphasis();
}

/** Lets the drawer's pin go: the drawer and the marks return to their unpinned state. */
export function unpin() {
  pinned = null;
  resetDetail();
  applyEmphasis();
}

/**
 * The drawer's button says whether what it shows is pinned, and is offered
 * only while it shows a mark.
 */
export function syncPinButton() {
  const button = maybeEl("detail-pin");
  if (!button) return;
  button.hidden = !(pinned || previewed);
  button.setAttribute("aria-pressed", pinned ? "true" : "false");
}

/** Wires the drawer's button: it pins the mark previewed, or lets the pin go. */
export function wireDrawer() {
  const button = maybeEl("detail-pin");
  if (!button) return;
  button.addEventListener("click", () => guarded("pin this", () => {
    if (pinned) unpin();
    else if (previewed) pin(previewed);
  }));
}

/**
 * Writes a mark's provenance into the drawer.
 * @param {LaidLink | LaidNode} d
 */
export function paintDetail(d) {
  const panel = el("detail");
  panel.replaceChildren();
  syncPinButton();
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
    // A PRINTED BALANCE IS CITED AT ITSELF, as a link is: its fact and its
    // pages, not the document's. A derived change says so, and why.
    for (const b of balancesOf(n)) {
      const said = h("p", "subtle", b.words + " ");
      if (b.derived) said.append(h("span", "chip derived", say("our_inference")), " ", b.rationale + " ");
      said.append(h("span", "facts", (b.fact_ids.length > 1 ? "Facts: " : "Fact: ") + b.fact_ids.join(" ")));
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
      const href = caveatHref(c.id);
      if (href) why.append(link("Read it in full", href));
      panel.append(why);
    }
  }

  // THE CITATIONS ARE THE MARK'S OWN WHEN IT HAS ANY: a link's locators, else
  // a mark's, else the sources of the document drawn.
  const prov = h("div", "prov");
  prov.append(h("span", "subtle", "Sources:"));
  const mark = asLink ? null : /** @type {LaidNode} */ (d);
  const cited = asLink ? /** @type {LaidLink} */ (d).locators : (mark && mark.locators) || projection.metadata.sources;
  for (const c of citations(cited)) {
    prov.append(link(c.label, c.href));
  }
  panel.append(prov);
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
 * toggle for following that GROUP's money (follows). Once a group splits, its
 * funds are coloured with no group node on screen, and the swatch is the only
 * key to their hue.
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
      // "inferred": a tail's own note counts the flows it FOLDS, a
      // different set, and an unqualified count would read as correcting it.
      li.append(h("div", "subtle", say("inferred_flows", {
        flows: flows.length, figure: fmt(total),
        ends: flows.map((l) => (labels.get(l.source) || l.source) + " → " + (labels.get(l.target) || l.target)).join("; "),
      })));
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
 * The marks and nodes whose label holds the query, case-folded, those that
 * start with it first: every mark drawn, and on the expansion every node of
 * the schedule the chart does not draw, which Enter reveals. A node in a
 * fund group the filters remove is not offered; one the kind or smallest
 * filter would remove once revealed is, and Enter says so and draws nothing.
 * @param {string} query
 * @returns {(LaidNode | FiscNode)[]}
 */
export function searchMatches(query) {
  const q = String(query).trim().toLocaleLowerCase();
  if (!q) return [];
  const folded = (/** @type {{label: string}} */ n) => String(n.label).toLocaleLowerCase();
  /** @type {(LaidNode | FiscNode)[]} */
  const pool = laidNodes.slice();
  if (expanding() && fetched) {
    const drawn = new Set(laidNodes.map((n) => n.id));
    const index = new Map(fetched.nodes.map((n) => [n.id, n]));
    for (const n of fetched.nodes) {
      if (drawn.has(n.id) || filter.groups.has(core.fundGroupOf(index, n)) || filter.groups.has(n.id)) continue;
      pool.push(n);
    }
  }
  return pool.filter((n) => folded(n).includes(q)).sort((a, b) =>
    Number(!folded(a).startsWith(q)) - Number(!folded(b).startsWith(q)) ||
    folded(a).localeCompare(folded(b)) || (a.id < b.id ? -1 : a.id > b.id ? 1 : 0));
}

/**
 * The expansions that draw `id`: the ones on screen, with each node above it
 * split, root first, by the split the next node down is a member of (a node
 * split another way is folded back first, as expandNode does), and the tail
 * of its own split drawn out where the cap would fold it. core.reveal's
 * chain, as a list of expansions so Back can walk it.
 * @param {string} id
 * @returns {string[]}
 */
export function revealEntries(id) {
  if (!fetched) return expansions.slice();
  const doc = fetched;
  const chain = [];
  for (let at = nodeOf(id), hops = 0; at && at.parent && hops < 9; at = nodeOf(at.parent), hops++) chain.unshift(at);
  let next = expansions.slice();
  for (const member of chain) {
    const splits = core.splitsOf(doc, member.parent);
    const key = member.alternative || (splits.length ? splits[0].key : "");
    const open = expansionState(doc, next).frontier;
    if (core.openSplitOf(doc, open, member.parent) === key) continue;
    const inside = core.withinNode(doc, member.parent);
    next = next.filter((e) => !entryInside(e, inside)).concat([entryOf(member.parent, key)]);
  }
  const own = chain[chain.length - 1];
  if (own) {
    const state = expansionState(doc, next);
    const drawn = formOnScreen().shape(doc, { chart: CONFIG.overview, frontier: state.frontier, whole: state.whole });
    const splits = core.splitsOf(doc, own.parent);
    const key = own.alternative || (splits.length ? splits[0].key : "");
    const tail = core.splitTailID(own.parent, key);
    if (!drawn.nodes.some((n) => n.id === id) && drawn.nodes.some((n) => n.id === tail)) next.push(tail);
  }
  return next;
}

/**
 * Whether the chart drawn at `next` under the filters on screen draws `id`,
 * so a reveal the filters would undo leaves the chart as it is. True where
 * the shape throws: redraw is what reports that.
 * @param {string[]} next
 * @param {string} id
 * @returns {boolean}
 */
function revealedUnderFilter(next, id) {
  if (!fetched) return false;
  try {
    const state = expansionState(fetched, next);
    const drawn = formOnScreen().shape(fetched, { chart: CONFIG.overview, frontier: state.frontier, whole: state.whole });
    return filterFor(drawn).doc.links.some((l) => l.source === id || l.target === id);
  } catch {
    return true;
  }
}

/**
 * What one option says: the mark's label, its role where another option reads
 * alike, and what Enter does where the chart offers more than following it.
 * @param {LaidNode | FiscNode} n
 * @param {(LaidNode | FiscNode)[]} hits every option listed
 * @returns {string}
 */
export function searchOptionWords(n, hits) {
  const alike = hits.some((o) => o !== n && o.label === n.label);
  const laid = laidNodes.some((x) => x.id === n.id);
  const offer = !laid ? say("search_reveals") : splittable(n) ? say("search_opens") : expandable(n) ? say("search_expands") : "";
  return n.label + (alike ? " (" + String(n.role).replace(/_/g, " ") + ")" : "") + (offer ? ", " + offer : "");
}

/** The search's options as listed, and the one the arrows are on (-1 for none). */
export let searchState = { hits: /** @type {(LaidNode | FiscNode)[]} */ ([]), active: -1 };

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
 * Takes the reader to one mark of the chart drawn, or to a node of the
 * expansion it does not draw, splitting the nodes above it until it is
 * (revealEntries). With `open`, the search's Enter, it does what activating
 * a drawn mark offers (openOffered), the rule the chart's own Enter and
 * double click use, and focuses a revealed node; otherwise, or where nothing
 * splits, it follows the mark's money and focuses it, which scrolls it into
 * view and pins it. Says whether the mark is on the chart.
 * @param {string} id
 * @param {boolean} open
 * @returns {boolean}
 */
export function chooseSearch(id, open) {
  const input = /** @type {HTMLInputElement | null} */ (maybeEl("search-input"));
  let d = laidNodes.find((n) => n.id === id);
  closeSearch();
  let revealed = false;
  if (!d && expanding() && nodeOf(id)) {
    const next = revealEntries(id);
    if (!revealedUnderFilter(next, id)) {
      fail("The filters leave " + /** @type {FiscNode} */ (nodeOf(id)).label + " out of the chart");
      return false;
    }
    if (!redraw(next, "That could not be found")) return false;
    d = laidNodes.find((n) => n.id === id);
    revealed = true;
  }
  if (!d) return false;
  if (input) input.value = d.label;
  if (!revealed && open && openOffered(d)) {
    if (input) input.value = "";
    return true;
  }
  if (revealed && open) {
    if (input) input.value = "";
  } else {
    setIsolated(d.id);
  }
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
 * without undoing a split.
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
      // The document's Escape undoes a split; in the search box it closes the
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

/* ------------------------------------------------------------------ *
 * The rail and the drawer beside the chart
 * ------------------------------------------------------------------ */

/** Whether the viewport has room for the rail and the drawer beside the chart. */
export function besideMatches() {
  return typeof window.matchMedia === "function" && window.matchMedia(BESIDE_QUERY).matches;
}

/**
 * Lays the rail and the drawer beside the chart where BESIDE_QUERY matches
 * and stacks them where it does not, following the viewport in and out of
 * full screen; and wires the rail's toggle, open by default where it is
 * beside the chart. The reader's toggle wins after that.
 */
export function wireLayout() {
  const body = maybeEl("chart-body");
  if (!body) return;
  const follow = () => { body.toggleAttribute("data-beside", besideMatches()); };
  follow();
  if (typeof window.matchMedia === "function") window.matchMedia(BESIDE_QUERY).addEventListener("change", follow);
  railOpen = besideMatches();
  const toggle = maybeEl("rail-toggle");
  if (toggle) {
    toggle.removeAttribute("hidden");
    toggle.addEventListener("click", () => setRail(!railOpen));
  }
  paintRail();
}

/**
 * Opens or collapses the rail. The rail is the page's, not the view's: the
 * hash and the history do not move.
 * @param {boolean} open
 */
export function setRail(open) {
  railOpen = open;
  paintRail();
}

/** Paints the rail as `railOpen` says: its toggle, its contents, and the column it takes. */
export function paintRail() {
  const body = maybeEl("chart-body");
  if (body) body.dataset.rail = railOpen ? "open" : "closed";
  const toggle = maybeEl("rail-toggle");
  if (toggle) toggle.setAttribute("aria-expanded", String(railOpen));
  const content = maybeEl("rail-content");
  if (content) content.hidden = !railOpen;
}

/* ------------------------------------------------------------------ *
 * Full screen
 * ------------------------------------------------------------------ */

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
 * button, Escape, or the browser's own control. The button says which way it
 * goes now; the zoom's wheel and pan rules read the state when asked
 * (zoomFilter).
 */
export function onFullscreenChange() {
  const button = maybeEl("chart-fullscreen");
  if (button) button.textContent = say(isFullscreen() ? "full_screen_exit" : "full_screen");
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
 * Whether d3.zoom takes this event. THE SAME RULE IN AND OUT OF FULL SCREEN:
 * a wheel zooms only with Ctrl or Cmd held (a trackpad's pinch arrives as
 * one), so a plain wheel or a trackpad scrolls the page and pans the chart's
 * scroller sideways; two fingers always pinch; and one finger or the primary
 * button pans the zoom only once zoomed in, so at the whole chart a drag pans
 * the scroller (wirePan). No secondary button, no Ctrl-click.
 * @param {any} event
 * @returns {boolean}
 */
export function zoomFilter(event) {
  if (event.type === "wheel") return Boolean(event.ctrlKey || event.metaKey);
  if (event.type === "touchstart") return (event.touches && event.touches.length > 1) || zoomOf().k > 1;
  return !event.ctrlKey && !event.button && zoomOf().k > 1;
}

/**
 * How far, in screen px, a press moves before it is a drag that pans rather
 * than a click.
 */
export const DRAG_SLOP = 4;

/**
 * Pans the chart's scroller sideways under a mouse or pen dragged across the
 * chart, anywhere on it, while the zoom is at the whole chart; zoomed in, the
 * zoom's own drag pans instead (zoomFilter). A press that moves less than
 * DRAG_SLOP is left a click; one that moves further takes the pointer and
 * swallows the click that ends it, so a drag ending on a mark neither follows
 * nor pins it. Touch is the scroller's own.
 * @param {HTMLElement} scroller the .chart-scroll the chart sits in
 */
export function wirePan(scroller) {
  /** @type {{id: number, x: number, left: number, moved: boolean} | null} */
  let drag = null;
  let swallow = false;
  scroller.addEventListener("pointerdown", (e) => {
    swallow = false;
    if (e.button !== 0 || e.pointerType === "touch" || zoomOf().k !== 1) return;
    drag = { id: e.pointerId, x: e.clientX, left: scroller.scrollLeft, moved: false };
  });
  scroller.addEventListener("pointermove", (e) => {
    if (!drag || e.pointerId !== drag.id) return;
    const dx = e.clientX - drag.x;
    if (!drag.moved) {
      if (Math.abs(dx) < DRAG_SLOP) return;
      drag.moved = true;
      // Taken only once it is a drag: a captured press clicks the scroller,
      // not the mark under it.
      if (typeof scroller.setPointerCapture === "function") {
        try { scroller.setPointerCapture(e.pointerId); } catch { /* a pointer already gone */ }
      }
    }
    scroller.scrollLeft = drag.left - dx;
  });
  /** @param {PointerEvent} e */
  const end = (e) => {
    if (!drag || e.pointerId !== drag.id) return;
    swallow = drag.moved;
    drag = null;
  };
  scroller.addEventListener("pointerup", end);
  scroller.addEventListener("pointercancel", end);
  scroller.addEventListener("click", (e) => {
    if (!swallow) return;
    swallow = false;
    e.stopPropagation();
    e.preventDefault();
  }, true);
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
 * A double click splits or follows a node, so it never zooms.
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
 * of sight, then scrolls the chart's scroller to it where the chart is wider
 * than its card, and leaves either alone where the mark is in sight, so a
 * click on a visible mark never moves the chart under the pointer.
 * @param {LaidNode} d
 */
export function revealMark(d) {
  if (!zoomer) return;
  const svg = maybeEl("chart");
  if (!svg) return;
  const t = zoomOf();
  const extent = zoomer.extent().call(svg);
  const cx = (d.x0 + d.x1) / 2;
  const cy = (d.y0 + d.y1) / 2;
  if (t.k !== 1 || t.x !== 0 || t.y !== 0) {
    const [left, top] = t.invert(extent[0]);
    const [right, bottom] = t.invert(extent[1]);
    if (cx < left || cx > right || cy < top || cy > bottom) zoomer.translateTo(D3.select(svg), cx, cy);
  }
  scrollToMark(svg, cx, extent[1][0] - extent[0][0]);
}

/**
 * Scrolls the scroller the svg sits in so the chart's x `cx`, under the zoom
 * now, is in sight, centring it where it is not; a no-op where the svg sits
 * in no .chart-scroll or draws no wider than it.
 * @param {Element} svg
 * @param {number} cx in the chart's own coordinates
 * @param {number} laid the viewBox's width
 */
function scrollToMark(svg, cx, laid) {
  const scroller = svg.parentElement;
  if (!scroller || !scroller.classList.contains("chart-scroll") || !(laid > 0)) return;
  const drawn = svg.getBoundingClientRect().width;
  const at = zoomOf().apply([cx, 0])[0] * drawn / laid;
  const from = scroller.scrollLeft;
  const view = scroller.clientWidth;
  if (drawn <= view || (at >= from && at <= from + view)) return;
  scroller.scrollLeft = Math.max(0, Math.min(drawn - view, at - view / 2));
}

/**
 * Wires what zooms the chart besides the pointer: + and - and 0 while focus
 * is in the chart, the Reset view button, and a mark the keyboard focuses
 * panned into sight; and the drag that pans the chart's scroller (wirePan).
 * Once, at boot: the svg outlives every render.
 */
export function wireZoom() {
  const svg = maybeEl("chart");
  if (!svg) return;
  const scroller = svg.parentElement;
  if (scroller && scroller.classList.contains("chart-scroll")) wirePan(scroller);
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
 * name, the fund groups and link kinds the filters remove, the smallest flow
 * drawn as itself in cents, the isolation, and the expansions in the order
 * the reader made them.
 * @typedef {{year: string, hide: string[], hidekind: string[], min: number, iso: string, x: string[]}} ViewState
 */

/** The hash's fields, in the order hashOf writes them. */
export const HASH_FIELDS = Object.freeze(["y", "hide", "hidekind", "min", "iso", "x"]);

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
    hide: Array.from(filter.groups).sort(),
    hidekind: Array.from(filter.kinds).sort(),
    min: filter.smallest,
    iso: filter.isolated,
    // In order and not sorted: each entry is applied to the frontier the
    // ones before it leave (expansionState).
    x: expansions.slice(),
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
 * `hide` and `hidekind` comma-separated, `min` in cents, `iso`, and `x`, the
 * expansions comma-separated in the order they were made.
 * @param {ViewState} st
 * @returns {string}
 */
export function hashOf(st) {
  const parts = ["y=" + hashValue(st.year)];
  if (st.hide.length) parts.push("hide=" + st.hide.map(hashValue).join(","));
  if (st.hidekind.length) parts.push("hidekind=" + st.hidekind.map(hashValue).join(","));
  if (st.min) parts.push("min=" + st.min);
  if (st.iso) parts.push("iso=" + hashValue(st.iso));
  if (st.x.length) parts.push("x=" + st.x.map(hashValue).join(","));
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
  const hide = list("hide");
  const hidekind = list("hidekind");
  const x = list("x");
  if ([...hide, ...hidekind, ...x].some((id) => !id)) return { kind: "refused", why: "it holds an empty name" };
  const min = fields.has("min") ? fields.get("min") || "" : "0";
  if (!/^\d+$/.test(min) || !Number.isSafeInteger(Number(min))) {
    return { kind: "refused", why: "its smallest flow, " + min + ", is not a whole number of cents" };
  }
  return {
    kind: "view",
    state: { year, hide: hide.sort(), hidekind: hidekind.sort(), min: Number(min), iso: text(fields.get("iso") || ""), x },
  };
}

/**
 * Writes the view on screen into the URL: a new history entry for a split or
 * its undoing, or a year ("push"), the entry on screen rewritten for a filter
 * or an isolation ("replace"). Nothing where the hash already says it, while a
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
 * group the year's column does not carry, an expansion the year's schedule
 * cannot make (expansionState), or an isolation naming a mark the chart does
 * not draw. A group or kind the view does not draw is inert, as the panel's
 * are. Expansions are applied before the isolation, which can name a member
 * only a split draws.
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
  if (st.x.join("\n") !== expansions.join("\n")) {
    if (!expanding() || !fetched) return refuse("this chart splits nothing");
    try {
      expansionState(fetched, st.x);
    } catch (e) {
      return refuse(year.label + " cannot draw it: " + (e instanceof Error ? e.message : String(e)));
    }
    if (!redraw(st.x)) return refuse("the chart could not be split as it names");
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
  const fallback = { year: yearKey(opening), hide: [], hidekind: [], min: 0, iso: "", x: [] };
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

/** Returns the drawer to its state with nothing previewed; the caller lets any pin go. */
export function resetDetail() {
  previewed = null;
  const panel = el("detail");
  panel.replaceChildren();
  panel.append(h("p", "subtle", say("detail_empty")));
  syncPinButton();
}

/**
 * Refuses to draw, visibly: in the drawer and in a role="alert" banner at the
 * top, since on a narrow viewport the drawer sits below a chart a reader may
 * never scroll past.
 *
 * @param {string} message
 */
export function fail(message) {
  previewed = null;
  const panel = el("detail");
  panel.replaceChildren();
  panel.append(h("p", "", message));
  const button = maybeEl("detail-pin");
  if (button) button.hidden = true;

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
 * One fetch per year; the page selects its schedule out of what it accepted.
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
    fail("This page will not draw: the year on screen carries no schedule " +
      "called " + key + ".");
    return null;
  }
  return doc;
}


/**
 * What one showYear attempt came to. SUPERSEDED (a later switch took over) is
 * not FAILED (the reader has been told).
 */
export const DREW = "drew";
export const SUPERSEDED = "superseded";
export const FAILED = "failed";

/**
 * Which of a list of expansions a year's expansion schedule can make, each
 * applied in order to what the ones kept before it leave: kept, and the
 * rest dropped -- a split of a node the year does not carry, and everything
 * made inside it.
 * @param {FiscProjection} doc the new year's expansion schedule
 * @param {readonly string[]} entries
 * @returns {{kept: string[], dropped: string[]}}
 */
export function carriedOver(doc, entries) {
  /** @type {string[]} */
  const kept = [];
  /** @type {string[]} */
  const dropped = [];
  for (const entry of entries) {
    try {
      expansionState(doc, kept.concat([entry]));
      kept.push(entry);
    } catch (e) {
      dropped.push(entry);
    }
  }
  return { kept, dropped };
}

/**
 * Says which splits a year switch folded back, in the words of the year
 * they were made on, or hides the line where it folded none.
 * @param {FiscYear | null} year the year switched to
 * @param {readonly string[]} dropped carriedOver's
 * @param {FiscProjection | null} from the schedule they were made on
 */
export function paintSplitsLine(year, dropped, from) {
  const line = maybeEl("splits-line");
  if (!line) return;
  if (!year || !dropped.length || !from) {
    line.textContent = "";
    line.setAttribute("hidden", "");
    return;
  }
  const labels = [];
  for (const entry of dropped) {
    const e = parseEntry(from, entry);
    const id = e.kind === "tail" ? e.node : e.id;
    const n = from.nodes.find((x) => x.id === id);
    const label = n ? n.label : id;
    if (!labels.includes(label)) labels.push(label);
  }
  line.textContent = say("splits_dropped", { year: year.label, nodes: core.joinAnd(labels), n: dropped.length });
  line.removeAttribute("hidden");
}

/** The latest switch token; an attempt holding an older one stands down. */
export let switching = 0;

/**
 * Fetches and draws one published year. Every word comes from CONFIG.years;
 * this composes no figure or caveat of its own.
 *
 * @param {FiscYear} year
 * @param {readonly string[]} [carry] expansions made on the year on screen,
 *   kept where the new year's schedule can make them (carriedOver), the rest
 *   folded back and named in #splits-line
 * @returns {Promise<string>} DREW, SUPERSEDED or FAILED
 */
export async function showYear(year, carry = []) {
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
  const was = { column, fetched, expansions, frontier, wholeSplits, laidNodes, groupIndex, projection, shown, whole };
  const before = fetched;
  column = loaded;
  fetched = doc;
  expansions = [];
  frontier = core.isExpansionSchedule(doc) ? core.rootFrontier(doc) : null;
  wholeSplits = new Set();
  /** @type {string[]} */
  let dropped = [];
  let laid;
  let rows;
  try {
    if (frontier && carry.length) {
      const over = carriedOver(doc, carry);
      const state = expansionState(doc, over.kept);
      expansions = over.kept;
      frontier = state.frontier;
      wholeSplits = state.whole;
      dropped = over.dropped;
    }
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
    ({ column, fetched, expansions, frontier, wholeSplits, laidNodes, groupIndex, projection, shown, whole } = was);
    throw e;
  }

  clearRefusal();
  // A pin and an isolation belong to the year they were made in.
  pinned = null;
  filter = Object.freeze(Object.assign({}, filter, { isolated: "" }));
  resetDetail();
  hideTip();

  paintYearWords(year);
  paintSplitsLine(year, dropped, before);
  paintChips();
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

  // Shared with every redraw, so the two cannot name the chart differently.
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
    // The splits the reader made go with them where the year can make them.
    void showYear(year, expansions).then((got) => { if (got === FAILED) back(); }, (e) => {
      back();
      fail("The chart failed to draw: " + String(e));
    });
  });
}

export async function main() {
  wireTheme();
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
  wireLayout();
  wireDrawer();
  wireFilters();
  wireSearch();
  wireFullscreen();
  wireZoom();

  // Wired before the first fetch (fisc-8cg): a superseded or failed opening
  // year can still be followed by one that draws, and it needs these.
  document.addEventListener("keydown", (e) => {
    if (e.key === "Escape") {
      // Escape undoes one thing, innermost first: a pin or isolation, then the
      // last split. The redraw clears the pin itself.
      if (!pinned && !filter.isolated && expansions.length) {
        guarded("undo the last split", undoLast);
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
 * The template calls this; importing the module draws nothing.
 */
export function boot() {
  return main().catch((e) => fail("The chart failed to draw: " + String(e)));
}
