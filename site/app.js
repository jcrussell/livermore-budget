// @ts-check
/**
 * fisc — the published page's client script, served exactly as committed and
 * booted by [boot]. At import it reads d3 and window.FISC_CONFIG once and
 * draws nothing; it holds the page's state, so tests load it fresh per page.
 * Go schema-checks every artifact, so this file refuses only what it alone
 * can see: an error page served 200, a stale cached file, and a chart its own
 * sums contradict.
 */

/* global d3 */

/* Wire shapes are schema/'s; a typedef names its schema path, and lists only
 * fields this file adds. */

import * as core from "./core.js";
import {
  SCHEMA_VERSION, PROJECTION_VERSION, say, kindLabel, fmt, fmtSigned, el, maybeEl, cssVar, h, link, citations, balancesOf, PARTITION_NOTE, isFundGroup, scheduleOf, isLink, leafDraws, drawsAgain, cellsOf,
} from "./core.js";
import { hashValue, selectYear, wireYearSelect, yearKey as columnKey } from "./format.js";
import { SANKEY } from "./sankey.js";
import { THEME_EVENT } from "./theme.js";
// Re-exported so the tests reach the shipped modules through the one file
// they import; this file calls a form only through the renderer it registers.
export * from "./core.js";
export * from "./sankey.js";

/**
 * schema/projection.schema.json#/properties/nodes/items, plus fixedValue (the
 * figure d3-sankey sizes the node at where its ribbons fall short) and slot.
 * @typedef {Record<string, any> & {id: string, label: string, tier: number, fixedValue?: number, slot?: number}} FiscNode
 */
/** schema/mark.schema.json, a node this client makes: an aggregate or a smaller-flows mark. @typedef {Record<string, any> & {id: string, label: string, tier: number}} FiscMark */

/** schema/projection.schema.json#/properties/links/items, the fields both a link and a laid link carry. @typedef {{kind: string, value_cents: number, locators: FiscSource[], contra: string}} LinkFields */
/** schema/projection.schema.json#/properties/links/items. @typedef {Record<string, any> & LinkFields} FiscLink */
/** schema/locator.schema.json. @typedef {{doc_id: string, pages: number[]}} FiscSource */
/** schema/projection.schema.json#/properties/metadata. @typedef {Record<string, any>} FiscMetadata */
/** schema/caveat.schema.json. @typedef {Record<string, any>} FiscCaveat */
/** schema/page.schema.json#/properties/years/items/properties/caveats/items. @typedef {Record<string, any>} FiscCaveatRef */
/** schema/page.schema.json#/properties/docs/additionalProperties. @typedef {Record<string, any>} FiscDoc */
/** schema/page.schema.json#/properties/years/items/properties/hero. @typedef {Record<string, any>} FiscFigure */
/** schema/page.schema.json#/properties/years/items. @typedef {Record<string, any>} FiscYear */
/** schema/page.schema.json, the page's window.FISC_CONFIG. @typedef {Record<string, any>} FiscConfig */
/** schema/page.schema.json#/properties/overview, the page's own chart: its form and hints. @typedef {Record<string, any>} FiscChart */
/** schema/projection.schema.json, as scheduleOf assembles it from a column document. @typedef {Record<string, any> & {nodes: FiscNode[], links: FiscLink[], metadata: FiscMetadata}} FiscProjection */

/**
 * A node as d3-sankey laid it out. layer is d3's column, not depth.
 * @typedef {FiscNode & {x0:number, x1:number, y0:number, y1:number, value:number,
 *   sourceLinks:LaidLink[], targetLinks:LaidLink[], depth:number,
 *   layer:number, clamped?:number}} LaidNode
 */

/**
 * @typedef {Record<string, any> & LinkFields & {source:LaidNode, target:LaidNode,
 *   value:number, width:number, y0:number, y1:number, index:number, w0?:number, w1?:number}} LaidLink
 */

/** d3 and d3-sankey are vendored UMD bundles with no type declarations. */
export const D3 = /** @type {any} */ (/** @type {any} */ (globalThis).d3);

/** @type {FiscConfig} */
export const CONFIG = /** @type {any} */ (globalThis).FISC_CONFIG;

/**
 * @param {FiscNode | LaidNode} node
 * @returns {string}
 */
export function fundGroupOf(node) {
  return core.fundGroupOf(groupIndex, node);
}

/**
 * A laid mark's share among the chart's laid nodes, in the form's words.
 * @param {LaidNode} d
 * @returns {string}
 */
export function shareOf(d) {
  return formOnScreen().share(d, laidNodes);
}

/**
 * frontier: schedule ids drawn; whole: splits drawn uncapped, as node\u001fkey.
 * @typedef {{chart: FiscChart, frontier: ReadonlySet<string> | null, whole: ReadonlySet<string>}} ShapeView
 */

/**
 * A form module: the only way this file reaches a form. It reads only its
 * own hints and computes no figure; see docs/chart-model.md.
 * @typedef {{
 *   form: string,
 *   tiersOf(chart: FiscChart): number[],
 *   width(columns: number): number,
 *   columnsOf(drawn: FiscProjection, tiers: number[]): number,
 *   shape(doc: FiscProjection, view: ShapeView): FiscProjection,
 *   layOut(drawn: FiscProjection, ctx: {tiers: number[], columns: number, groupOf: (n: FiscNode | LaidNode) => string, placeOf: (id: string) => number, chart: FiscChart, before?: Map<string, Box>, parentOf?: (id: string) => string, room?: number}): {nodes: LaidNode[], links: LaidLink[]},
 *   canvas(nodes: LaidNode[], ctx: {chart: FiscChart, room: number}): number,
 *   hairlines(graph: {nodes: LaidNode[], links: LaidLink[]}, chart: FiscChart): number,
 *   render(graph: {nodes: LaidNode[], links: LaidLink[]}, ctx: Record<string, any>): () => void,
 *   paint(ctx: {svg: any, colour: {link: (d: LaidLink) => string, node: (d: LaidNode) => string}}): void,
 *   cents(d: LaidLink | LaidNode): number,
 *   share(d: LaidNode, laid: LaidNode[]): string,
 *   reduction(d: LaidNode): boolean,
 *   crossTab(d: LaidNode): boolean,
 *   reductionNote(d: LaidNode): string,
 *   drawnNote(d: LaidNode): string,
 * }} FormRenderer
 */

/**
 * Renderers by form. A chart naming an absent form is refused before any
 * fetch: the wrong renderer would draw a wrong chart rather than fail.
 * @type {Map<string, FormRenderer>}
 */
export const FORMS = new Map([["sankey", SANKEY]]);

/**
 * The overview's node tiers, coarsest first. Absent skips the fold.
 * @type {number[]}
 */
export const RENDER_TIERS = (() => {
  const overview = CONFIG && CONFIG.overview;
  const renderer = overview ? FORMS.get(overview.form) : null;
  return renderer ? renderer.tiersOf(overview) : [];
})();

/**
 * The page chart's form when no renderer draws it, else "".
 * @returns {string}
 */
export function undrawableForm() {
  const overview = CONFIG.overview || {};
  return FORMS.has(overview.form) ? "" : String(overview.form);
}

/**
 * The drawn column's fund groups, in Go's order; an open set.
 * @returns {{id: string, slot: number}[]}
 */
export function fundGroups() {
  return (column && Array.isArray(column.fund_groups)) ? column.fund_groups : [];
}

/**
 * nodeRank and buildLegend must agree on this order.
 * @param {string} id
 * @returns {number}
 */
export function fundGroupPlace(id) {
  return fundGroups().findIndex((g) => g.id === id);
}

/**
 * A link's fund group: either end's fund group, else the group both ends
 * share, else the one end's that has one, else "" rather than pick one of two.
 * @param {LaidLink} link
 * @param {Map<string, FiscNode>} [index]
 * @returns {string}
 */
export function linkGroup(link, index = groupIndex) {
  const source = core.fundGroupOf(index, link.source);
  const target = core.fundGroupOf(index, link.target);
  return isFundGroup(link.source) ? link.source.id
    : isFundGroup(link.target) ? link.target.id
    : source === target ? source
    : !target ? source
    : !source ? target
    : "";
}

/**
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
 * A fund group's hue property, --muted where it has none; shared with the legend.
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
 * What the reader removed or followed. smallest is in cents, 0 for all.
 * Replaced whole, moved only through setFilter and setIsolated. A group,
 * kind or node not drawn stays and is inert, so it reapplies after a split
 * or year change.
 * @typedef {{groups: ReadonlySet<string>, kinds: ReadonlySet<string>, nodes: ReadonlySet<string>, only: string, smallest: number, isolated: string}} FilterState
 */
/** The state that removes and follows nothing. @type {Readonly<FilterState>} */
export const NO_FILTER = Object.freeze({ groups: new Set(), kinds: new Set(), nodes: new Set(), only: "", smallest: 0, isolated: "" });
/** @type {Readonly<FilterState>} */
export let filter = NO_FILTER;
/**
 * The filtered chart on screen and the ribbons removed; null before a draw.
 * @type {{doc: FiscProjection, hidden: FiscLink[]} | null}
 */
export let shown = null;
/**
 * The chart laid out unfiltered, where a filter is on: figures are read off
 * it (markFigure), so a filter never restates a mark's figure.
 * @type {{nodes: LaidNode[], links: LaidLink[]} | null}
 */
export let whole = null;
/** Smallest-flow stops in cents: every flow, then $10k to $10M by tens. */
export const SMALLEST_STOPS = Object.freeze([0, 1000000, 10000000, 100000000, 1000000000]);

/**
 * Splits opened, in order: "id", "id~key" (ALTERNATIVE_MARK), or a tail id.
 * The view and the hash's `x`; moved only through redraw.
 * @type {readonly string[]}
 */
export let expansions = [];
/** The expansion schedule's node ids drawn, read off `expansions`; null on any other document. @type {ReadonlySet<string> | null} */
export let frontier = null;
/** The splits drawn whole rather than capped, node\u001fkey, read off `expansions`. @type {ReadonlySet<string>} */
export let wholeSplits = new Set();
/** What separates a node from the split key it was split by, in an entry of `expansions`. */
export const ALTERNATIVE_MARK = "~";

/** px kept clear under the chart: the scrollbar of a panning chart. */
export const ROOM_CUSHION = 16;

/**
 * The chart's room in its own units: window height below the unscrolled
 * chart top, less ROOM_CUSHION. 0 where nothing is laid out.
 * @returns {number}
 */
export function roomHeight() {
  const svg = maybeEl("chart");
  if (!svg) return 0;
  const r = svg.getBoundingClientRect();
  if (!(r.width > 0)) return 0;
  const box = String(svg.getAttribute("viewBox") || "").split(/\s+/).map(Number);
  const laid = box.length === 4 && box[2] > 0 ? box[2] : widthOf(openingColumns());
  return Math.max(0, Math.floor((window.innerHeight - chartTop(r) - ROOM_CUSHION) * laid / r.width));
}

/**
 * @param {{top: number}} r the svg's bounding rect
 * @returns {number}
 */
function chartTop(r) {
  const card = isFullscreen() ? maybeEl("chart-card") : null;
  return card ? r.top - card.getBoundingClientRect().top + card.scrollTop : r.top + window.scrollY;
}

/**
 * @param {number} n
 * @returns {number}
 */
export function widthOf(n) {
  return Math.max(...Array.from(FORMS.values(), (r) => r.width(n)));
}

/**
 * One column per rendered tier; every chart is drawn at its scale.
 * @returns {number}
 */
export function openingColumns() {
  return Math.max(1, RENDER_TIERS.length);
}

/**
 * --chart-stretch: `n` columns' width over the opening chart's, so a split
 * moves and rescales no column and a wider chart pans.
 * @param {number} n
 * @returns {number}
 */
export function chartStretch(n) {
  return widthOf(n) / widthOf(openingColumns());
}

/** Rail beside the chart at or above this width, stacked above it below. */
export const BESIDE_QUERY = "(min-width: 1280px)";

/**
 * The year's document before any fold; splits and parents are read off it.
 * @type {FiscProjection | null}
 */
export let fetched = null;
/** @type {any} */
export let column = null;
/** @type {FiscYear | null} */
export let shownYear = null;
/**
 * The template's chart description, captured on first paint.
 * @type {string}
 */
export let baseDescription = "";
/** @type {LaidNode[]} */
export let laidNodes = [];
/** @type {Map<string, string>} */
let drawnNames = new Map();

/**
 * The name a node filtered out was drawn by, since no later render names it.
 * Lives while the node stays in `filter.nodes` within one year.
 * @type {Map<string, string>}
 */
const outNames = new Map();

/** Drops every name in outNames whose node `filter.nodes` no longer holds. */
function keepOutNames() {
  for (const id of [...outNames.keys()]) if (!filter.nodes.has(id)) outNames.delete(id);
}

/**
 * @param {{id: string, label: string}} d
 * @returns {string}
 */
export function nameOf(d) {
  return drawnNames.get(d.id) || d.label;
}
/** @type {() => void} */
let motionEnd = () => {};
/**
 * Tallest canvas drawn at heldRoom: a fold never shortens it, so nothing
 * below jumps. Another room lets it go.
 */
export let heldHeight = 0;
/** The room (roomHeight) the chart on screen was drawn at, which heldHeight holds within. */
export let heldRoom = 0;
/** The room the last graph layOut returned was laid out at, which render draws its viewBox at. */
export let laidRoom = 0;
/** Whether a redraw at the room is asked for and not yet drawn (askRoom). */
let roomAsked = false;
/** The window the chart was drawn in (fitRoom). */
let drawnWindow = { width: 0, height: 0, full: false, top: 0 };
/**
 * px the window height may move, all else unmoved, and keep the chart: more
 * than a phone's toolbars, less than a resize or a rotation.
 */
export const CHROME_SLACK = 120;
/** chartTop is a float that drifts sub-pixel on a zoomed phone as it scrolls. */
export const TOP_SLACK = 1;
/** How long a redraw's marks move for, in ms, where the reader allows motion. */
export const MOTION_MS = 350;
/** What the drawer holds until the reader lets it go. @type {LaidNode | LaidLink | null} */
export let pinned = null;
/** What the drawer shows under the pointer while nothing is pinned. @type {LaidNode | LaidLink | null} */
export let previewed = null;
/** Whether the rail beside the chart is open; the page's, never the hash's. */
export let railOpen = false;
/** ms in which a click counts as the one assistive tech synthesised from Enter or Space. */
export const ACTIVATION_WINDOW = 500;

/** @type {{id:string, at:number}} */
export let keyActivation = { id: "", at: -Infinity };

/**
 * Last click activation, so a double click's second click on the hub the
 * first opened is one activation.
 * @type {{id:string, at:number}}
 */
export let pointerActivation = { id: "", at: -Infinity };

/**
 * @param {string} id
 * @param {number} at
 * @returns {boolean}
 */
function doubled(id, at) {
  const since = at - pointerActivation.at;
  return pointerActivation.id === id && since >= 0 && since < ACTIVATION_WINDOW;
}

/** @type {LaidNode | LaidLink | null} */
export let traced = null;

/**
 * The last laid document's nodes by id; paint() reads it on a theme change.
 * @type {Map<string, FiscNode>}
 */
export let groupIndex = new Map();

/* ---- Chart ---- */

/**
 * Writes the flow count: marks drawn, and facts cited against the document's
 * total. No metadata.counts counts ribbons only: a throw mid-repaint splits
 * the page.
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

/** @returns {boolean} */
export function expanding() {
  return core.isExpansionSchedule(fetched);
}

/**
 * @param {string} id
 * @returns {FiscNode | undefined}
 */
export function nodeOf(id) {
  return fetched ? fetched.nodes.find((n) => n.id === id) : undefined;
}

/**
 * A drawn node's declared splits; none for a tail or another document.
 * @param {{id: string}} d
 * @returns {{key: string, form: string, alternative: boolean, members: string[]}[]}
 */
export function splitsHere(d) {
  if (!expanding() || !fetched || core.isAggregate(d.id)) return [];
  return core.splitsOf(fetched, d.id);
}

/**
 * core.memberNoun for `id`, or "".
 * @param {string} id
 * @returns {string}
 */
export function memberNounOf(id) {
  return expanding() && fetched ? core.memberNoun(fetched, id) : "";
}

/**
 * Refused, not guessed, for a key with no split_labels entry.
 * @param {string} key
 * @returns {string}
 */
export function splitWords(key) {
  const words = (CONFIG.split_labels || {})[key];
  if (!words) throw new Error("cannot name split " + key + ": the page publishes no words for it");
  return words;
}

/**
 * The id alone for its first split, so an ordinary split's hash has no key.
 * @param {string} id
 * @param {string} key
 * @returns {string}
 */
export function entryOf(id, key) {
  const splits = fetched ? core.splitsOf(fetched, id) : [];
  return splits.length && splits[0].key === key ? id : id + ALTERNATIVE_MARK + key;
}

/**
 * Refused, naming the entry, where no such split or tail exists.
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
 * Applies entries in order from the root frontier (core.expand); refused,
 * naming the entry, where it names an undrawn node or an unopened tail.
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
 * Disjoint from expandable: a tail draws out instead.
 * @param {{id: string}} d
 * @returns {boolean}
 */
export function splittable(d) {
  return glyphOf(d) === "open";
}

/**
 * @param {{id: string}} d
 * @returns {boolean}
 */
export function foldable(d) {
  return glyphOf(d) === "fold";
}

/**
 * "open" (⊕), "fold" (⊖) or "". splittable and foldable read it, so the
 * glyph drawn and the click are one rule.
 * @param {FiscProjection | null} doc
 * @param {ReadonlySet<string> | null} front
 * @param {{id: string}} d
 * @returns {"open" | "fold" | ""}
 */
export function glyphAt(doc, front, d) {
  if (!front || !doc || !front.has(d.id) || core.isAggregate(d.id) || !core.splitsOf(doc, d.id).length) return "";
  return core.openSplitOf(doc, front, d.id) === "" ? "open" : "fold";
}

/**
 * @param {{id: string}} d
 * @returns {"open" | "fold" | ""}
 */
export function glyphOf(d) {
  return glyphAt(fetched, frontier, d);
}

/**
 * @param {{id: string}} d
 * @returns {boolean}
 */
export function expandable(d) {
  return core.isAggregate(d.id) && expanding() && Boolean(core.splitTailOf(d.id));
}

/**
 * Not merely "not the body": a redraw from the panel or drawer must not
 * pull focus into the chart.
 * @returns {boolean}
 */
export function focusInChart() {
  const active = document.activeElement;
  const chart = maybeEl("chart");
  return Boolean(active) && Boolean(chart) &&
    (active === chart || (typeof /** @type {HTMLElement} */ (chart).contains === "function" && /** @type {HTMLElement} */ (chart).contains(active)));
}

/**
 * Asks every node what the paints ask, so a declaration they would throw on
 * is met inside the caller's rollback, not over a half-repainted page.
 */
export function askAffordances() {
  if (!projection) return;
  for (const n of projection.nodes) {
    splittable(n);
    foldable(n);
    expandable(n);
    if (core.labelledAsParent(groupIndex, n.id)) memberNounOf(n.id);
  }
}

/**
 * Draws `next` and repaints, or leaves the page as it was and says why.
 * Everything that can throw runs before the page changes; state is restored
 * whole on a throw. The pin and a node's isolation are cleared (dropPin).
 * @param {readonly string[]} next
 * @param {string} [refused] the refusal banner's opening words
 * @param {() => string} [landOn] the mark focus lands on, asked after layout
 * @param {string} [anchor] the node kept where it is on screen
 * @returns {boolean} whether `next` is on screen
 */
export function redraw(next, refused = "That could not be drawn", landOn, anchor) {
  finishMotion();
  // Asked before the repaint removes the element focus is on.
  const hadFocus = focusInChart();
  const prior = boxesOf(laidNodes);
  const held = anchor ? screenBoxOf(anchor) : null;
  const was = { expansions, frontier, wholeSplits, laidNodes, laidRoom, groupIndex, projection, shown, whole };
  let laid;
  /** @type {FiscProjection | undefined} */
  let drawn;
  try {
    if (!fetched) throw new Error("there is no chart on screen");
    if (expanding()) {
      const state = expansionState(fetched, next);
      frontier = state.frontier;
      wholeSplits = state.whole;
    }
    expansions = next;
    drawn = shapeFor(fetched);
    const filtered = filterFor(drawn);
    const room = layRoom();
    const unfiltered = wholeOf(drawn, filtered, room);
    laid = layOut(filtered.doc, prior, room);
    projection = drawn;
    shown = filtered;
    whole = unfiltered;
    askAffordances();
  } catch (e) {
    ({ expansions, frontier, wholeSplits, laidNodes, laidRoom, groupIndex, projection, shown, whole } = was);
    const words = drawn ? refusalWords(e, drawn, "redraw_part", "redraw_alone_empty") : "";
    refuseInPlace(refused + ": " + (words || (e instanceof Error ? e.message : String(e))));
    return false;
  }
  clearRefusal();
  // What the last year switch folded back is said until the reader moves.
  paintSplitsLine(null, [], null);
  dropPin();
  resetDetail();
  hideTip();
  paintBulk();
  paintChartName();
  paintCounts();
  buildLegend();
  paintChartHint();
  render(laid, prior);
  // After the render: the line names nodes as the chart now draws them.
  paintFilterLine();
  if (anchor && held) keepInPlace(anchor, held);
  restoreFocus(hadFocus, landOn ? landOn() : "");
  // A split and its undoing are steps a reader walks back with Back.
  recordView("push");
  return true;
}

/**
 * Opens a split (the first by default); a node split another way is folded
 * first. Focus goes to the topmost new member.
 * @param {string} id
 * @param {string} [key]
 * @returns {boolean}
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
 * Folds a node back, with every entry opened straight through it
 * (core.straightFrom). Focus returns to it.
 * @param {string} id
 * @returns {boolean}
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
 * Splits every unsplit node, and what that exposes, until none is left
 * (core.expandAllFrom); tails stay folded.
 * @returns {boolean} whether the chart moved
 */
export function expandAll() {
  if (!fetched || !frontier) return false;
  const { splits } = core.expandAllFrom(fetched, frontier);
  if (!splits.length) return false;
  return redraw(expansions.concat(splits.map((sp) => entryOf(sp.node, sp.key))), "That could not be split");
}

/** @returns {boolean} whether the chart moved */
export function collapseAll() {
  if (!expansions.length) return false;
  return redraw([], "That could not be folded back");
}

/**
 * Draws a folded tail out whole; focus goes to its topmost member.
 * @param {{id: string}} d
 * @returns {boolean}
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
 * Where focus goes after a redraw: the topmost new mark in `among`, else the
 * topmost in `among`, else "".
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
 * Runs a gesture and turns a throw into a refusal banner: a throw from a DOM
 * listener reaches nobody and leaves the page mid-render.
 * @template T
 * @param {string} gesture what the reader did, for the sentence
 * @param {() => T} run
 * @param {(message: string) => void} [refuse] fail, or refuseInPlace
 * @returns {T | undefined}
 */
export function guarded(gesture, run, refuse = fail) {
  try {
    return run();
  } catch (e) {
    refuse("This page could not " + gesture + ": " + String(e) +
      ". The chart on screen is unchanged; the file it was drawn from is most likely " +
      "not one this site published.");
    return undefined;
  }
}

/**
 * A click on a node. The echo guard matters: assistive tech's synthesised
 * click, and a double click's second, land on the hub just opened and would
 * fold it (keyActivation, pointerActivation).
 * @param {LaidNode} d
 * @param {number} [at] the event's timestamp
 */
export function clickNode(d, at = Infinity) {
  if (keyActivation.id === d.id && at - keyActivation.at < ACTIVATION_WINDOW) return;
  if (doubled(d.id, at)) return;
  pointerActivation = { id: d.id, at: at };
  activate(d);
}

/**
 * Opens or folds what a node offers and pins the result; a tail drawn out
 * pins nothing. A refused open keeps any earlier pin (refuseInPlace).
 * @param {LaidNode} d
 */
function activate(d) {
  closeChartMenu();
  const followed = followsNode() ? filter.isolated : "";
  if (foldable(d) && fetched) {
    foldPinned(d);
    return;
  }
  if (!offers(d)) {
    pin(d);
    return;
  }
  if (!openOffered(d)) return;
  const kept = laidNodes.find((n) => n.id === d.id);
  if (kept) pinKept(kept, followed);
}

/**
 * Folds a hub and pins it (or what it folded into), re-following it where it
 * was followed: a second click and ⊖ fold alike.
 * @param {LaidNode} d
 */
function foldPinned(d) {
  if (!fetched) return;
  const followed = followsNode() ? filter.isolated : "";
  const from = core.straightFrom(fetched, d.id);
  if (!collapseNode(d.id)) return;
  const kept = laidNodes.find((n) => n.id === d.id) || laidNodes.find((n) => n.id === from);
  if (kept) pinKept(kept, followed);
}

/**
 * Pins what a redraw kept and re-follows it if it was followed: the redraw's
 * dropPin let that isolation go.
 * @param {LaidNode} kept
 * @param {string} followed the node followed before, or ""
 */
function pinKept(kept, followed) {
  pin(kept);
  if (followed !== "" && followed === kept.id) setIsolated(followed);
}

/**
 * @param {{id: string}} d
 * @returns {boolean}
 */
export function offers(d) {
  return splittable(d) || expandable(d);
}

/**
 * Splits a node, else draws its tail out; false if neither or refused. The
 * one rule a node's activation and the search's Enter share.
 * @param {{id: string}} d
 * @returns {boolean}
 */
export function openOffered(d) {
  if (splittable(d)) return expandNode(d.id);
  if (expandable(d)) return drawOutTail(d);
  return false;
}

/**
 * @param {LaidNode} d
 * @param {number} [at] the event's timestamp
 */
export function keyNode(d, at = -Infinity) {
  keyActivation = { id: d.id, at: at };
  activate(d);
}

/**
 * A glyph's screen-reader words, and whether ⊕ opens a menu (more than one split).
 * @param {{id: string, label: string}} d
 * @param {"open" | "fold"} kind
 * @returns {{label: string, menu: boolean}}
 */
export function glyphWords(d, kind) {
  const label = nameOf(d);
  if (kind === "fold") return { label: say("glyph_fold", { label }), menu: false };
  const splits = splitsHere(d);
  if (splits.length > 1) return { label: say("glyph_choose", { label }), menu: true };
  return { label: say("glyph_open", { label, split: splits.length ? splitWords(splits[0].key) : "" }), menu: false };
}

/**
 * @param {LaidNode} d
 * @returns {{kind: "open" | "fold", label: string, menu: boolean} | null}
 */
export function glyphFor(d) {
  const kind = glyphOf(d);
  return kind ? Object.assign({ kind }, glyphWords(d, kind)) : null;
}

/**
 * A glyph activated: ⊖ folds (foldPinned); ⊕ opens, or offers a menu of
 * splits. Same echo guard as the node: the redraw between key and
 * synthesised click turns ⊕ into ⊖.
 * @param {LaidNode} d
 * @param {"open" | "fold"} kind
 * @param {number} at the event's timestamp
 * @param {boolean} [byKey] Enter or Space, not a click
 * @param {Element} [glyph] the element a menu is placed by
 */
export function glyphNode(d, kind, at, byKey = false, glyph) {
  if (!byKey && keyActivation.id === d.id && at - keyActivation.at < ACTIVATION_WINDOW) return;
  if (!byKey && doubled(d.id, at)) return;
  if (byKey) keyActivation = { id: d.id, at: at };
  else pointerActivation = { id: d.id, at: at };
  closeChartMenu();
  if (kind === "fold") {
    foldPinned(d);
    return;
  }
  if (glyphWords(d, kind).menu) {
    openSplitMenu(d, glyph);
    return;
  }
  activate(d);
}

/**
 * The one menu open on the chart (split menu or context menu), or null.
 * back is where focus returns: a mark, or the button that opened it.
 * @type {{id: string, menu: HTMLElement, back: "g.glyph" | "g.node" | HTMLElement} | null}
 */
export let chartMenu = null;

/**
 * A menu button; `refused` disables it and says why under it.
 * @typedef {{words: string, act: () => void, doing: string, data?: Record<string, string>, refused?: string}} MenuChoice
 */

/**
 * Opens a menu beside a node, focus on the first button. A choice, Escape
 * or focus leaving closes it and returns focus to `back`.
 * @param {{id: string, element: string, label: string, back: "g.glyph" | "g.node" | HTMLElement, at: {left: number, top: number} | null, choices: MenuChoice[]}} o
 *   `at`: top left in viewport px, or null for the chart's top left
 */
function openMenu(o) {
  closeChartMenu();
  const tip = maybeEl("tooltip");
  const wrap = tip ? tip.parentElement : null;
  if (!wrap) return;
  const menu = h("div", "chart-menu");
  menu.id = o.element;
  menu.setAttribute("role", "group");
  menu.setAttribute("aria-label", o.label);
  for (const c of o.choices) {
    const choice = h("button", "chart-menu-choice", c.words);
    choice.setAttribute("type", "button");
    Object.assign(choice.dataset, c.data || {});
    choice.addEventListener("click", (e) => {
      e.stopPropagation();
      guarded(c.doing, () => {
        // Focus back on the node's mark first, so a redraw the choice makes
        // finds it in the chart and puts it back (restoreFocus).
        closeChartMenu(true);
        c.act();
        // A choice that hides the button the menu came from (a filter
        // dropping the pin hides More) would leave focus on the body.
        if (typeof o.back !== "string" && o.back.hidden) restoreFocus(true, o.id);
      });
    });
    menu.append(choice);
    if (c.refused) {
      /** @type {HTMLButtonElement} */ (choice).disabled = true;
      const why = h("span", "chart-menu-why", c.refused);
      why.id = o.element + "-why-" + menu.childElementCount;
      choice.setAttribute("aria-describedby", why.id);
      menu.append(why);
    }
  }
  menu.addEventListener("keydown", (e) => {
    if (e.key !== "Escape") return;
    e.stopPropagation();
    closeChartMenu(true);
  });
  // A press in the menu keeps it open until its click lands: where a click
  // focuses no button (Safari, Firefox on macOS) the press blurs the menu.
  let pressing = false;
  const open = () => chartMenu !== null && chartMenu.menu === menu;
  menu.addEventListener("pointerdown", () => {
    pressing = true;
    const doc = menu.ownerDocument;
    const released = () => {
      doc.removeEventListener("pointerup", released, true);
      doc.removeEventListener("pointercancel", released, true);
      setTimeout(() => {
        pressing = false;
        if (open() && !menu.contains(doc.activeElement)) closeChartMenu();
      }, 0);
    };
    doc.addEventListener("pointerup", released, true);
    doc.addEventListener("pointercancel", released, true);
  });
  menu.addEventListener("focusout", (e) => {
    const to = /** @type {Node | null} */ (e.relatedTarget);
    if (pressing || (to && menu.contains(to))) return;
    if (open()) closeChartMenu();
  });
  const box = wrap.getBoundingClientRect();
  menu.style.left = Math.max(0, o.at ? o.at.left - box.left : 0) + "px";
  menu.style.top = Math.max(0, o.at ? o.at.top - box.top : 0) + "px";
  wrap.append(menu);
  chartMenu = { id: o.id, menu: menu, back: o.back };
  if (typeof o.back !== "string") {
    o.back.setAttribute("aria-expanded", "true");
    o.back.setAttribute("aria-controls", o.element);
  }
  const first = menu.querySelector("button");
  if (first) /** @type {HTMLElement} */ (first).focus();
}

/**
 * @param {LaidNode} d
 * @param {Element} [glyph]
 */
export function openSplitMenu(d, glyph) {
  const id = d.id;
  const at = glyph ? glyph.getBoundingClientRect() : null;
  openMenu({
    id: id,
    element: "split-menu",
    label: say("split_chooser", { label: nameOf(d) }),
    back: "g.glyph",
    at: at ? { left: at.right + 4, top: at.top } : null,
    choices: splitsHere(d).map((sp) => ({
      words: splitWords(sp.key),
      doing: "split this mark",
      data: { key: sp.key },
      act: () => {
        const followed = followsNode() ? filter.isolated : "";
        if (!expandNode(id, sp.key)) return;
        const kept = laidNodes.find((n) => n.id === id);
        if (kept) pinKept(kept, followed);
      },
    })),
  });
}

/**
 * A node's context menu: Follow, Filter out, Show only. The filters only on
 * a node of the shaped chart (a smaller-flows mark has no ribbons of its
 * own); one the redraw would refuse is offered disabled.
 * @param {LaidNode} d
 * @param {{left: number, top: number} | null} [at] the pointer, viewport px
 * @param {Element} [mark] the node's element
 * @param {HTMLElement} [from] the button that opened it, focus's return
 */
export function openNodeMenu(d, at, mark, from) {
  const id = d.id;
  hideTip();
  const box = !at && mark ? (mark.querySelector("rect") || mark).getBoundingClientRect() : null;
  openMenu({
    id: id,
    element: "node-menu",
    label: say("node_menu", { label: nameOf(d) }),
    back: from || "g.node",
    at: at || (box ? { left: box.right + 4, top: box.top } : null),
    choices: /** @type {MenuChoice[]} */ ([
      { words: say("follow_money"), doing: "follow this money", data: { act: "follow" }, act: () => followNode(id) },
    ]).concat(projection && projection.nodes.some((n) => n.id === id) ? [
      { words: say("filter_out_node"), doing: "filter this out", data: { act: "out" }, act: () => filterOutNode(id),
        refused: filterRefusal(filterOutStep(id)) },
      { words: say("show_only_node"), doing: "show only this", data: { act: "only" }, act: () => showOnlyNode(id),
        refused: filterRefusal(showOnlyStep(id)) },
    ] : []),
  });
}

/** @param {string} id */
export function followNode(id) {
  const d = laidNodes.find((n) => n.id === id);
  if (!d) return;
  pin(d);
  setIsolated(id);
}

/**
 * Removes a node's ribbons and its leaves from every other (leafCut). A
 * listed fund group goes in `filter.groups`, so box, search, line and hash
 * agree. The line names it as drawn when chosen (outNames).
 * @param {string} id
 */
export function filterOutNode(id) {
  const next = filterOutStep(id);
  const d = laidNodes.find((n) => n.id === id);
  if (d && next.nodes) outNames.set(id, nameOf(d));
  setFilter(next);
}

/**
 * @param {string} id
 * @returns {Partial<FilterState>}
 */
function filterOutStep(id) {
  const only = filter.only === id ? "" : filter.only;
  if (listedGroup(id)) return { groups: new Set(filter.groups).add(id), only: only };
  return { nodes: new Set(filter.nodes).add(id), only: only };
}

/**
 * @param {string} id
 * @returns {boolean}
 */
function listedGroup(id) {
  return isGroupID(id) && fundGroupPlace(id) >= 0;
}

/**
 * Draws only what the node's Follow leaves undimmed (onlyKeeps), cut by
 * leaf elsewhere (leafCut); un-filters the node if it was out.
 * @param {string} id
 */
export function showOnlyNode(id) {
  setFilter(showOnlyStep(id));
}

/**
 * @param {string} id
 * @returns {Partial<FilterState>}
 */
function showOnlyStep(id) {
  const nodes = new Set(filter.nodes);
  nodes.delete(id);
  return { nodes: nodes, only: id };
}

/**
 * @param {boolean} [refocus]
 * @returns {boolean} whether a menu was open
 */
export function closeChartMenu(refocus = false) {
  if (!chartMenu) return false;
  const { id, menu, back } = chartMenu;
  chartMenu = null;
  menu.remove();
  if (typeof back !== "string") {
    back.setAttribute("aria-expanded", "false");
    back.removeAttribute("aria-controls");
    if (refocus && !back.hidden) back.focus();
    return true;
  }
  if (refocus) {
    const chart = maybeEl("chart");
    const mark = chart ? D3.select(chart).selectAll(back).filter(/** @param {LaidNode} n */ (n) => n.id === id).node() : null;
    if (!mark || typeof mark.focus !== "function") return true;
    // WITHOUT PINNING: focus going back where the menu took it from is not
    // the reader moving it.
    restoring = true;
    try {
      mark.focus();
    } finally {
      restoring = false;
    }
  }
  return true;
}

/**
 * Every ribbon and node on a mark's path, to the sources and the sinks.
 * Through an open hub the walk narrows to ribbons sharing a leaf, so a
 * member climbs only the sources it repeats; each further hub narrows by
 * its own leaves.
 * @param {LaidNode | LaidLink} d
 * @returns {{links: Set<LaidLink>, nodes: Set<LaidNode>}}
 */
export function pathOf(d) {
  /** @type {Set<LaidLink>} */
  const links = new Set();
  /** @type {Set<LaidNode>} */
  const nodes = new Set();
  const draws = leafDraws(shown ? shown.doc : null);
  /** @param {LaidLink} l @returns {string[]} the leaves l draws that another ribbon draws too */
  const again = (l) => Object.keys(cellsOf(l) || {}).filter((k) => (draws.get(k) || 0) > 1);
  /** The leaves each ribbon was walked on by; "" for every one. @type {Map<LaidLink, Set<string>>} */
  const walked = new Map();
  /**
   * @param {LaidNode} from
   * @param {boolean} up
   * @param {string[] | null} by leaves followed, or null for every one
   */
  const walk = (from, up, by) => {
    /** @type {Array<[LaidNode, string[] | null]>} */
    const stack = [[from, by]];
    while (stack.length) {
      const [n, came] = /** @type {[LaidNode, string[] | null]} */ (stack.pop());
      nodes.add(n);
      const onward = up ? n.targetLinks : n.sourceLinks;
      // Past the hub the leaves repeat through: the walk is whole again.
      const keys = came && onward.some((l) => again(l).some((k) => came.includes(k))) ? came : null;
      for (const l of onward) {
        const own = again(l);
        let next = own.length ? (keys ? own.filter((k) => keys.includes(k)) : own) : null;
        if (next && !next.length) continue;
        const had = walked.get(l) || new Set();
        if (had.has("") || (next && next.every((k) => had.has(k)))) continue;
        for (const k of next || [""]) had.add(k);
        walked.set(l, had);
        links.add(l);
        stack.push([up ? l.source : l.target, next]);
      }
    }
  };
  if (isLink(d)) {
    const l = /** @type {LaidLink} */ (d);
    const own = again(l);
    links.add(l);
    walk(l.source, true, own.length ? own : null);
    walk(l.target, false, own.length ? own : null);
  } else {
    walk(/** @type {LaidNode} */ (d), true, null);
    walk(/** @type {LaidNode} */ (d), false, null);
  }
  return { links, nodes };
}

/** @param {LaidNode | LaidLink} d */
export function trace(d) {
  if (traced === d) return;
  traced = d;
  applyEmphasis();
}

/** The pointer off a mark: the path it traced let go. */
export function untrace() {
  if (!traced) return;
  traced = null;
  applyEmphasis();
}

/**
 * Refocuses after a redraw, only if focus was in the chart: the caller must
 * say so, as the focused element is already detached.
 * @param {boolean} hadFocus
 * @param {string} [onto] the mark's id, or "" for the first mark
 */
export function restoreFocus(hadFocus, onto = "") {
  if (!hadFocus) return;
  // WITHOUT PINNING: the drawer keeps the node the reader opened, not the
  // member focus lands on. The focus handlers ask it through `unasked`.
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
 * A pointer is pressed on the chart, or released this task: focus it causes
 * neither pins nor scrolls. A context menu, keydown, blur or lost capture
 * ends it, so an unheard release cannot strand the keyboard.
 */
let pointing = false;

/** @param {string} onto */
function restoreFocusTo(onto) {
  // focus() is on HTMLElement and SVGElement, not Element; the SVG <g> marks
  // are not obliged to have it, so the runtime test stays.
  const focus = (/** @type {Element | null} */ target) => {
    const el = /** @type {any} */ (target);
    if (!el || typeof el.focus !== "function") return false;
    el.focus({ preventScroll: pointing });
    return true;
  };
  const chart = maybeEl("chart");
  const named = onto && chart
    ? D3.select(chart).selectAll("g.node").filter(/** @param {LaidNode} d */ (d) => d.id === onto).node()
    : null;
  if (focus(named)) return;
  focus(chart ? chart.querySelector("g.node") : null);
}

/** Names the chart for a screen reader. */
export function paintChartName() {
  const title = maybeEl("chart-title");
  if (title && shownYear && shownYear.chart_title) title.textContent = shownYear.chart_title;
  const desc = maybeEl("chart-desc");
  if (!desc) return;
  if (!baseDescription) baseDescription = desc.textContent;
  desc.textContent = baseDescription;
}

/**
 * Says what a click does. Asked of drawn nodes, not declared splits: the
 * filters can remove every node that would split.
 */
export function paintChartHint() {
  const hint = maybeEl("chart-hint");
  if (!hint) return;
  if (!expanding()) return;
  const chart = chartDrawn();
  const anyOpens = Boolean(chart) && /** @type {FiscProjection} */ (chart).nodes.some(splittable);
  // The trace, the pin and Escape are named on every view: they are what an
  // activation does where nothing opens.
  const follow = " " + say("follow");
  // A sentence of its own: the tail can be the only thing a chart offers.
  const tails = Boolean(chart) && /** @type {FiscProjection} */ (chart).nodes.some(expandable);
  const expands = tails ? " " + say("expand") : "";
  const folds = expansions.length ? " " + say("fold_back") : "";
  const legend = maybeEl("legend");
  const swatch = legend && legend.children.length ? " " + say("swatch") : "";
  hint.textContent = (anyOpens ? say("open_into") : say("nothing_opens")) + follow + expands + folds + swatch;
}

/**
 * Expand all / Collapse all, each live only where it would move the chart;
 * hidden off the expansion schedule.
 */
export function paintBulk() {
  const on = expanding() && Boolean(fetched);
  const shut = on && Boolean(projection) && /** @type {FiscProjection} */ (projection).nodes.some(splittable);
  /** @type {[string, string, boolean][]} */
  const buttons = [["chart-expand-all", "expand_all", shut], ["chart-collapse-all", "collapse_all", on && expansions.length > 0]];
  for (const [id, words, live] of buttons) {
    const b = maybeEl(id);
    if (!b) continue;
    b.textContent = say(words);
    if (on) b.removeAttribute("hidden");
    else b.setAttribute("hidden", "");
    if (live) b.removeAttribute("disabled");
    else b.setAttribute("disabled", "");
  }
}

/**
 * A button its own press disables hands focus to the other, so the keyboard
 * is not dropped on the body; a press that took no focus (Safari) hands none.
 */
export function wireBulk() {
  const all = maybeEl("chart-expand-all");
  const none = maybeEl("chart-collapse-all");
  /** @type {[HTMLElement | null, HTMLElement | null, () => boolean][]} */
  const pairs = [[all, none, expandAll], [none, all, collapseAll]];
  for (const [b, other, act] of pairs) {
    if (!b) continue;
    b.addEventListener("click", () => {
      const held = document.activeElement === b;
      guarded("redraw the chart", act);
      if (held && b.hasAttribute("disabled") && other && !other.hasAttribute("disabled")) other.focus();
    });
  }
}

/**
 * @param {FiscProjection} doc
 * @returns {FiscProjection}
 */
export function shapeFor(doc) {
  return formOnScreen().shape(doc, { chart: CONFIG.overview, frontier, whole: wholeSplits });
}

/** @returns {FiscChart} */
export function chartOnScreen() {
  return CONFIG.overview;
}

/** @returns {FormRenderer} */
export function formOnScreen() {
  const chart = chartOnScreen();
  const renderer = FORMS.get(chart.form);
  if (!renderer) throw new Error("no renderer here draws a " + chart.form + " chart");
  return renderer;
}

/**
 * @param {LaidLink} d
 * @returns {string}
 */
export function linkClass(d) {
  return "link" + (d.derived ? " derived" : "") + (d.contra ? " contra" : "") +
    (d.partition ? " partition" : "");
}

/**
 * `opens` and `expands` are disjoint (splittable, expandable); the class
 * does not enforce it.
 * @param {LaidNode} d
 * @returns {string}
 */
export function nodeClass(d) {
  return "node" + (d.derived ? " derived" : "") + (formOnScreen().reduction(d) ? " contra" : "") +
    (splittable(d) ? " opens" : "") + (expandable(d) ? " expands" : "");
}

/**
 * Glyphs, not words: the label is at its gutter's edge, and a glyph survives
 * forced-colors and greyscale. ⊕/⊖ are drawn beside the rect instead.
 * @param {LaidNode} d
 * @returns {string}
 */
export function nodeFlags(d) {
  const marks = (d.derived ? "◇" : "") + (expandable(d) ? "⊞" : "");
  return marks ? "  " + marks : "";
}

/**
 * Lays out and binds the page. The hue index prefers the unfolded document,
 * since the fold re-points parents; assigned only after layout succeeds, or
 * paint() would recolour against a document not drawn.
 * @param {FiscProjection} doc
 * @param {Map<string, Box>} [prior] the chart replaced, whose column order it keeps
 * @param {number} [room]
 * @returns {{nodes:LaidNode[], links:LaidLink[]}}
 */
export function layOut(doc, prior, room = layRoom()) {
  const { graph, index } = laidOf(doc, prior, room);
  groupIndex = index;
  laidRoom = room;
  // For shareOf: only the laid graph knows a mark's column.
  laidNodes = graph.nodes;
  return graph;
}

/**
 * layOut, assigning nothing.
 * @param {FiscProjection} doc
 * @param {Map<string, Box> | undefined} prior
 * @param {number} room
 * @returns {{graph: {nodes:LaidNode[], links:LaidLink[]}, index: Map<string, FiscNode>}}
 */
function laidOf(doc, prior, room) {
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
    room: room,
  });
  return { graph, index };
}

/**
 * A filter can lower this.
 * @param {FiscProjection} doc
 * @returns {number}
 */
export function layoutColumns(doc) {
  return formOnScreen().columnsOf(doc, RENDER_TIERS);
}

/**
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

/** filterFor's refusal where nothing of the node shown alone survives; carries its id. */
export class AloneEmpty extends Error {
  /**
   * @param {string} message
   * @param {string} node
   */
  constructor(message, node) {
    super(message);
    this.node = node;
  }
}

/**
 * core.filterDocument with fund groups read through the hue index and
 * leaves by leafCut. Throws where the filter line cannot state the removal
 * once, and where nothing of the node shown alone survives: `only` never
 * lapses to drawing everything.
 * @param {FiscProjection} doc
 * @param {Readonly<FilterState>} [state]
 * @returns {{doc: FiscProjection, hidden: FiscLink[]}}
 */
export function filterFor(doc, state = filter) {
  const index = hueIndex(doc);
  const drawn = new Set(doc.nodes.map((n) => n.id));
  const st = Object.assign({}, state, { nodes: new Set(Array.from(state.nodes || []).filter((id) => drawn.has(id))) });
  const keeps = onlyKeeps(doc, st, index);
  const out = core.filterDocument(doc, (n) => core.fundGroupOf(index, n), st, keeps, leafCut(doc, st, keeps));
  if (keeps && !out.doc.links.some(keeps)) {
    const n = doc.nodes.find((x) => x.id === st.only);
    throw new AloneEmpty("the other filters leave none of " + (n ? nameOf(n) : st.only) + "'s money to show alone", st.only);
  }
  // Asked here, inside every draw's try: a removal whose money the filter
  // line cannot state once (core.distinctCents) is refused, not drawn.
  hiddenTally(out);
  return out;
}

/**
 * The schedule leaves within nodes filtered out, groups hidden and the node
 * shown alone (core.LeafCut). Null where nothing is filtered by node or
 * group, or off the expansion schedule, whose ribbons repeat no leaf.
 * @param {FiscProjection} doc the shaped chart
 * @param {Readonly<FilterState>} state
 * @param {((l: FiscLink) => boolean) | null} keeps onlyKeeps over doc
 * @returns {{out: Set<string>, groups: Set<string>, only: Set<string> | null} | null}
 */
export function leafCut(doc, state, keeps) {
  const schedule = fetched;
  const byID = new Map(doc.nodes.map((n) => [n.id, n]));
  const nodes = Array.from(state.nodes || []);
  const groupIDs = Array.from(state.groups || []);
  if (!schedule || !core.isExpansionSchedule(schedule) || (!nodes.length && !groupIDs.length && !keeps)) return null;
  const index = hueIndex(doc);
  const raw = new Set(schedule.nodes.map((n) => n.id));
  const inside = (/** @type {string} */ id) => scheduleWithin(schedule, raw, byID, id);
  const out = new Set();
  const groups = new Set();
  for (const id of groupIDs) for (const x of inside(id)) groups.add(x);
  for (const id of nodes) for (const x of inside(id)) (isGroupID(id, index) ? groups : out).add(x);
  return { out: out, groups: groups, only: keeps ? inside(state.only) : null };
}

/**
 * Schedule ids within a shaped node: itself, or a tail's folded members.
 * @param {FiscProjection} schedule
 * @param {Set<string>} raw the ids of the schedule's nodes
 * @param {Map<string, FiscNode>} byID the shaped chart's nodes
 * @param {string} id
 * @returns {Set<string>}
 */
function scheduleWithin(schedule, raw, byID, id) {
  const n = byID.get(id);
  const from = raw.has(id) ? [id] : (n && Array.isArray(n.folds) ? n.folds : []).filter((f) => raw.has(f));
  const out = new Set();
  for (const f of from) for (const x of core.withinNode(schedule, f)) out.add(x);
  return out;
}

/**
 * Schedule ids within any node in `filter.nodes`: neither offered by search
 * nor drawn by its Enter. An undrawn id counts, as revealing it would draw
 * it as a hub its filter then cuts.
 * @returns {Set<string>}
 */
function filteredOutWithin() {
  const out = new Set();
  if (!fetched) return out;
  const raw = new Set(fetched.nodes.map((n) => n.id));
  const byID = new Map((projection ? projection.nodes : []).map((n) => [n.id, n]));
  for (const id of filter.nodes) for (const x of scheduleWithin(fetched, raw, byID, id)) out.add(x);
  return out;
}

/**
 * The ribbons `only` keeps whole: those Follow leaves undimmed (follows).
 * Null where nothing is shown alone or the node draws no ribbon (inert).
 * @param {FiscProjection} doc
 * @param {Readonly<FilterState>} state
 * @param {Map<string, FiscNode>} index the hue index
 * @returns {((l: FiscLink) => boolean) | null}
 */
export function onlyKeeps(doc, state, index) {
  const id = state.only;
  if (!id) return null;
  const byID = new Map(doc.nodes.map((n) => [n.id, n]));
  const end = (/** @type {string} */ at) => byID.get(at) || /** @type {FiscNode} */ ({ id: at });
  const keeps = (/** @type {FiscLink} */ l) =>
    follows(id, /** @type {any} */ ({ source: end(l.source), target: end(l.target) }), index);
  return doc.links.some(keeps) ? keeps : null;
}

/** @returns {FiscProjection | null} */
export function chartDrawn() {
  return shown ? shown.doc : projection;
}

/**
 * What the filters removed: ribbon count, cents with each cell once
 * (core.distinctCents), and facts cited. The filter line's only client sum.
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

/** @returns {{flows: number, ribbons: number}} */
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
 * What the filters removed at one mark, in words, or "": a mark is sized by
 * what is drawn, so the reader is told what it omits.
 * @param {string} id
 * @returns {string}
 */
export function hiddenAt(id) {
  const touching = (shown ? shown.hidden : []).filter((l) => l.source === id || l.target === id);
  if (!touching.length) return "";
  return say("filter_hidden_at", { cents: fmtSigned(hiddenTally({ hidden: touching }).cents), flows: touching.length });
}

/**
 * The shaped chart laid out unfiltered where filters change it, else null.
 * @param {FiscProjection} drawn the shaped chart
 * @param {{doc: FiscProjection}} filtered filterFor's answer over it
 * @param {number} room
 * @returns {{nodes: LaidNode[], links: LaidLink[]} | null}
 */
export function wholeOf(drawn, filtered, room) {
  return filtered.doc === drawn ? null : laidOf(drawn, undefined, room).graph;
}

/**
 * The same node in `whole`, else `d`.
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
 * @param {LaidLink | LaidNode} d
 * @returns {number}
 */
export function markFigure(d) {
  return formOnScreen().cents(markOfView(d));
}

/**
 * The drawn part of a filtered node's figure, said beside it, never in its slot.
 * @param {LaidLink | LaidNode} d
 * @returns {string}
 */
export function drawnAt(d) {
  const drawn = formOnScreen().cents(d);
  if (isLink(d) || drawn === markFigure(d)) return "";
  return say("filter_drawn_at", { cents: fmtSigned(drawn) });
}

/**
 * Draws, paints and emphasises the chart. Gestures, descriptions and classes
 * are this file's, since they read page state. The zoomed group is kept
 * between charts so a mark keeps its element and moves from `prior`.
 * @param {{nodes:LaidNode[], links:LaidLink[]}} [laid]
 * @param {Map<string, Box>} [prior]
 */
export function render(laid, prior) {
  const doc = chartDrawn();
  if (!doc) return;
  finishMotion();
  // The marks the pointer traced and the menu a glyph opened are the chart's
  // replaced.
  traced = null;
  closeChartMenu();
  const graph = laid || layOut(doc);
  // A chart at another room holds nothing of the canvas the last one held.
  if (laidRoom !== heldRoom) {
    heldHeight = 0;
    heldRoom = laidRoom;
  }
  // A form that names no mark leaves every mark its label.
  drawnNames = new Map();
  const svg = D3.select("#chart");
  // Never the svg, whose viewBox is the chart's own coordinates.
  let layer = svg.select(":scope > g.zoom-layer");
  if (layer.empty()) layer = svg.append("g").attr("class", "zoom-layer");
  const columns = layoutColumns(doc);
  // style.css's --chart-room: the card breaks out to the opening chart's
  // width so its left edge never moves; the svg stretches by columns drawn.
  document.documentElement.style.setProperty("--chart-max", widthOf(openingColumns()) + "px");
  document.documentElement.style.setProperty("--chart-stretch", String(chartStretch(columns)));
  const ended = formOnScreen().render(graph, {
    svg: svg,
    layer: layer,
    before: prior,
    parentOf: parentOf,
    motion: prior && prior.size ? motionMs() : 0,
    held: heldHeight,
    room: laidRoom,
    tiers: RENDER_TIERS,
    columns: columns,
    chart: chartOnScreen(),
    // The unfolded nodes the chart was folded from, which a same-word mark's
    // qualifier is read off (labelQualifiers).
    doc: { nodes: Array.from(groupIndex.values()) },
    noun: memberNounOf,
    named: (/** @type {Map<string, string>} */ names) => { drawnNames = names; },
    classes: { link: linkClass, node: nodeClass, flags: nodeFlags },
    describe: { link: linkDescription, node: nodeDescription },
    figure: markFigure,
    glyph: glyphFor,
    on: {
      tip: hover, hide: hideTipOf, pin: pin, guarded: guarded,
      click: clickNode, key: keyNode, glyph: glyphNode, menu: openNodeMenu,
      trace: trace, untrace: untrace,
      unasked: () => restoring || pointing,
    },
  });
  motionEnd = typeof ended === "function" ? ended : () => {};
  const box = String(svg.attr("viewBox") || "").split(/\s+/).map(Number);
  if (box.length === 4 && box[3] > heldHeight) heldHeight = box[3];
  drawnWindow = windowNow();
  paintHairlines(graph);
  paint();
  applyEmphasis();
  // A new chart starts unzoomed: the transform was about the marks it replaced.
  bindZoom();
  // The options name marks of the chart just replaced.
  closeSearch();
}

/** @typedef {{x0: number, x1: number, y0: number, y1: number}} Box */

/**
 * @param {LaidNode[]} nodes
 * @returns {Map<string, Box>}
 */
export function boxesOf(nodes) {
  return new Map(nodes.map((n) => [n.id, { x0: n.x0, x1: n.x1, y0: n.y0, y1: n.y1 }]));
}

/**
 * What a mark grows out of and shrinks into: a tail's split node, else its
 * parent, else "".
 * @param {string} id
 * @returns {string}
 */
export function parentOf(id) {
  const tail = core.splitTailOf(id);
  if (tail) return tail.node;
  const n = nodeOf(id);
  return n && n.parent ? n.parent : "";
}

/** @returns {number} redraw motion in ms; 0 under prefers-reduced-motion */
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
 * Scroll, in px, that puts node `d` back where it was on screen (`was`).
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

/** @param {{nodes: LaidNode[], links: LaidLink[]}} graph */
export function paintHairlines(graph) {
  const line = maybeEl("hairline-line");
  if (!line) return;
  const n = formOnScreen().hairlines(graph, chartOnScreen());
  line.hidden = n === 0;
  line.textContent = n ? say("hairline", { n }) : "";
}

/**
 * Colours the chart and legend. wireTheme is wired before main()'s form
 * refusal, so this must do nothing on such a page.
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
 * The only path into isolation, so the legend's pressed button and the
 * dimming cannot drift apart. "" clears.
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
  syncPinButton();
  recordView("replace");
}

/**
 * @param {string} id
 * @param {Map<string, FiscNode>} [index]
 * @returns {boolean}
 */
export function isGroupID(id, index = groupIndex) {
  const n = index.get(id);
  return Boolean(n) && isFundGroup(/** @type {FiscNode} */ (n));
}

/**
 * A ribbon isolating `id` follows: an end at the node, or for a group its
 * drawn group or either end's. One rule for swatch and mark, so following a
 * group never follows less than its node.
 * @param {string} id
 * @param {LaidLink} l
 * @param {Map<string, FiscNode>} [index]
 * @returns {boolean}
 */
export function follows(id, l, index = groupIndex) {
  if (l.source.id === id || l.target.id === id) return true;
  if (!isGroupID(id, index)) return false;
  return linkGroup(l, index) === id || core.fundGroupOf(index, l.source) === id || core.fundGroupOf(index, l.target) === id;
}

/**
 * A mark the isolation is about, not one it reaches by a ribbon.
 * @param {string} id
 * @param {LaidNode} d
 * @returns {boolean}
 */
export function isolates(id, d) {
  return d.id === id || (isGroupID(id) && fundGroupOf(d) === id);
}

/**
 * An isolation the chart draws nothing of is inert, not a chart dimmed whole.
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
  paintReset();
  // THE TRACE IS ITS OWN CLASS, never dim: it is let go on the pointer's
  // leave, and the isolation under it must be there when it is.
  const path = traced ? pathOf(traced) : null;
  svg.classed("tracing", Boolean(path));
  svg.selectAll("path.link").classed("on-path", /** @param {LaidLink} d */ (d) => Boolean(path) && /** @type {NonNullable<typeof path>} */ (path).links.has(d));
  svg.selectAll("g.node").classed("on-path", /** @param {LaidNode} d */ (d) => Boolean(path) && /** @type {NonNullable<typeof path>} */ (path).nodes.has(d));
}

/**
 * @param {ReadonlySet<string>} a
 * @param {ReadonlySet<string>} b
 * @returns {boolean}
 */
function sameSet(a, b) {
  return a.size === b.size && Array.from(a).every((x) => b.has(x));
}

/**
 * @param {Partial<FilterState>} next
 * @param {Readonly<FilterState>} was
 * @returns {Readonly<FilterState>}
 */
function filterStateOf(next, was) {
  const smallest = next.smallest === undefined ? was.smallest : Math.max(0, Math.floor(Number(next.smallest)) || 0);
  return Object.freeze({
    groups: new Set(next.groups || was.groups),
    kinds: new Set(next.kinds || was.kinds),
    nodes: new Set(next.nodes || was.nodes),
    only: next.only === undefined ? was.only : next.only,
    smallest: smallest,
    isolated: next.isolated === undefined ? was.isolated : next.isolated,
  });
}

/**
 * Why filterFor would refuse `next`, in words, or "".
 * @param {Partial<FilterState>} next
 * @returns {string}
 */
export function filterRefusal(next) {
  if (!projection) return "";
  const doc = projection;
  try {
    filterFor(doc, filterStateOf(next, filter));
    return "";
  } catch (e) {
    return refusalWords(e, doc, "refused_part", "refused_alone_empty") || say("refused_other");
  }
}

/**
 * Told by type (core.PartialCut, AloneEmpty), never by message, which names
 * ids. "" for another throw or a node `doc` lacks.
 * @param {unknown} e
 * @param {FiscProjection} doc
 * @param {string} part
 * @param {string} alone
 * @returns {string}
 */
function refusalWords(e, doc, part, alone) {
  const node = (/** @type {string} */ id) => doc.nodes.find((n) => n.id === id);
  if (e instanceof core.PartialCut) {
    const from = node(e.source);
    const to = node(e.target);
    if (from && to) return say(part, { source: nameOf(from), target: nameOf(to) });
  }
  if (e instanceof AloneEmpty) {
    const shown = node(e.node);
    if (shown) return say(alone, { label: nameOf(shown) });
  }
  return "";
}

/**
 * Moves the filter state: a removal re-lays-out, an isolation alone
 * repaints. A refused redraw leaves state, chart and panel as they were. A
 * new isolation is followed and pinned so the redraw's dropPin keeps it.
 * @param {Partial<FilterState>} next
 * @returns {boolean} whether `next` is on screen
 */
export function setFilter(next) {
  const was = filter;
  const state = filterStateOf(next, was);
  const isolate = (/** @type {string} */ id) => {
    if (id !== "" && !isGroupID(id) && laidNodes.some((n) => n.id === id)) followNode(id);
    else setIsolated(id);
  };
  if (sameSet(was.groups, state.groups) && sameSet(was.kinds, state.kinds) && sameSet(was.nodes, state.nodes) &&
    was.only === state.only && was.smallest === state.smallest) {
    if (next.isolated === undefined) setIsolated(state.isolated);
    else isolate(next.isolated);
    return true;
  }
  filter = state;
  if (!projection) {
    keepOutNames();
    syncFilterPanel();
    return true;
  }
  finishMotion();
  const hadFocus = focusInChart();
  const focused = hadFocus ? /** @type {any} */ (document.activeElement).__data__ : null;
  const prior = boxesOf(laidNodes);
  let filtered;
  let unfiltered;
  let laid;
  try {
    filtered = filterFor(projection);
    const room = layRoom();
    unfiltered = wholeOf(projection, filtered, room);
    laid = layOut(filtered.doc, prior, room);
  } catch (e) {
    filter = was;
    keepOutNames();
    syncFilterPanel();
    refuseInPlace("The chart could not be filtered: " + (e instanceof Error ? e.message : String(e)));
    return false;
  }
  shown = filtered;
  whole = unfiltered;
  keepOutNames();
  dropPin();
  resetDetail();
  hideTip();
  buildLegend();
  syncFilterPanel();
  paintChartHint();
  render(laid, prior);
  paintFilterLine();
  restoreFocus(hadFocus, focused && typeof focused.id === "string" ? focused.id : "");
  // An isolation of what the redraw draws nothing of is cleared above, not
  // put back.
  if (next.isolated !== undefined && next.isolated !== filter.isolated &&
    (next.isolated === "" || laidNodes.some((n) => isolates(next.isolated || "", n)))) isolate(next.isolated);
  recordView("replace");
  return true;
}

/** Puts every removal and the isolation back: the filter line's Reset. */
export function resetFilters() {
  setFilter(NO_FILTER);
}

/** Says what the filters leave out; hidden where they leave out nothing. */
export function paintFilterLine() {
  const line = maybeEl("filter-line");
  const said = maybeEl("filter-said");
  if (!line || !said) return;
  const gone = hiddenTally();
  const merged = foldedTally();
  const parts = [];
  const drawn = projection ? projection.nodes : [];
  const alone = filter.only && projection && onlyKeeps(projection, filter, hueIndex(projection))
    ? drawn.find((n) => n.id === filter.only) : undefined;
  if (alone) parts.push(say("filter_only", { label: nameOf(alone) }));
  const out = drawn.filter((n) => filter.nodes.has(n.id));
  if (out.length) parts.push(say("filter_out", { names: out.map((n) => outNames.get(n.id) || nameOf(n)).join("; ") }));
  if (gone.flows) parts.push(say("filter_hidden", { cents: fmtSigned(gone.cents), flows: gone.flows }));
  if (merged.flows) {
    parts.push(say("filter_folded", { flows: merged.flows, ribbons: merged.ribbons, threshold: fmt(filter.smallest) }));
  }
  said.textContent = parts.join(" ");
  if (parts.length) line.removeAttribute("hidden");
  else line.setAttribute("hidden", "");
  // The line sits above the chart: toggling it moves the chart's top in an
  // unmoved window.
  if (chartDrawn()) drawnWindow = Object.assign({}, drawnWindow, { top: windowNow().top });
  paintReset();
}

/**
 * @param {number} cents
 * @returns {string}
 */
export function smallestWords(cents) {
  return cents > 0 ? fmt(cents) : say("smallest_all");
}

/**
 * Checked means shown.
 * @param {string} id
 * @param {string} words
 * @param {string} swatch custom property, or ""
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

/** Per year: fund groups in Go's order, hueless last and --muted, as the legend. */
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

/** So a refused change or a Reset leaves no box disagreeing with the state. */
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
    // The stop at or below the threshold.
    range.value = String(Math.max(0, SMALLEST_STOPS.findLastIndex((stop) => stop <= filter.smallest)));
    range.setAttribute("aria-valuetext", smallestWords(filter.smallest));
  }
  const out = maybeEl("filter-smallest-value");
  if (out) out.textContent = smallestWords(filter.smallest);
}

/** Open by default beside the rail (BESIDE_QUERY); the reader's toggle wins after. */
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

/* ---- Tooltip, detail panel ---- */

/**
 * A ribbon's provenance phrase. A printed figure re-pointed onto a mark of
 * ours gets a third phrase, and one redrawn through a split a fourth: no
 * page prints it at those ends.
 * @param {boolean} derived
 * @param {boolean} ontoOurs either end is a mark we drew
 * @param {boolean} [again] another ribbon draws a leaf it draws
 * @returns {string}
 */
export function provenanceOf(derived, ontoOurs, again = false) {
  if (derived) return say("inferred_by_us");
  if (again) return say("repeats_note");
  return ontoOurs ? say("carried_note") : say("printed_by_city");
}

/**
 * @param {LaidLink} d
 * @returns {string}
 */
export function linkDescription(d) {
  const kind = kindLabel(d.kind);
  return nameOf(d.source) + " to " + nameOf(d.target) + ", " + fmtSigned(formOnScreen().cents(d)) +
    (kind ? ", " + kind : "") +
    (d.contra ? ", " + d.contra : "") +
    (d.partition ? ", " + PARTITION_NOTE : "") +
    (d.merged ? ", " + d.merged : "") +
    ", " + provenanceOf(d.derived, Boolean(d.source.derived || d.target.derived), drawsAgain(d, leafDraws(projection)));
}

/**
 * @param {LaidNode} d
 * @returns {string}
 */
export function nodeDescription(d) {
  // WHAT AN ACTIVATION DOES, for a reader who cannot see the glyph: it opens
  // a node that splits, folds a hub, draws out a tail, and otherwise pins.
  const what = splittable(d) ? say("desc_opens") : foldable(d) ? say("desc_folds") :
    expandable(d) ? say("desc_expands") : say("desc_follows");
  const form = formOnScreen();
  const mark = markOfView(d);
  const note = form.reductionNote(mark);
  // The cross-tab qualification, for a reader who cannot see the ribbons.
  const drawn = drawnAt(d);
  const gone = hiddenAt(d.id);
  return nameOf(d) + ", total " + fmtSigned(markFigure(d)) +
    (drawn ? ", " + drawn : "") +
    ", " + say(d.derived ? "inferred_by_us" : "printed_by_city") +
    (form.crossTab(mark) ? ", " + PARTITION_NOTE : "") +
    (note ? ", " + note.replace(/^\u25c7 /, "") : "") + (gone ? ", " + gone : "") + what;
}

/**
 * Through the hierarchy, not by id: a drawn node is often a fold of several.
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
  return projection.metadata.caveats.filter((/** @type {FiscCaveat} */ c) =>
    Array.isArray(c.applies_to) && c.applies_to.some(reaches));
}

/**
 * The chips the tooltip and the detail panel both wear for a mark, in order.
 * `words` names the say() keys for printed and inferred; `again` and `share`
 * are a link's drawsAgain and a node's shareOf, passed in so this reads no
 * state.
 * @param {LaidLink | LaidNode} d
 * @param {{printed: string, inferred: string}} words
 * @param {{again?: boolean, share?: string}} seen
 * @returns {{cls: string, words: string}[]}
 */
export function chipsOf(d, words, seen) {
  /** @type {{cls: string, words: string}[]} */
  const out = [];
  const chip = (/** @type {string} */ cls, /** @type {string} */ w) => out.push({ cls, words: w });
  if (isLink(d)) {
    const l = /** @type {LaidLink} */ (d);
    const kind = kindLabel(l.kind);
    if (kind) chip("chip", kind);
    const lent = Boolean(l.source.derived || l.target.derived);
    chip(l.derived || lent || seen.again ? "chip derived" : "chip",
      say(l.derived ? words.inferred : seen.again ? "repeats_chip" : lent ? "carried_chip" : words.printed));
    if (l.contra) chip("chip contra", "reduction");
    if (l.partition) chip("chip partition", "cross-tab");
  } else {
    const n = /** @type {LaidNode} */ (d);
    chip("chip", n.role.replace(/_/g, " "));
    if (n.constraint_tier) chip("chip", "constraint: " + n.constraint_tier);
    chip(n.derived ? "chip derived" : "chip", say(n.derived ? words.inferred : words.printed));
    // chip derived: a share is computed, never printed.
    if (seen.share) chip("chip derived", seen.share);
  }
  return out;
}

/** @param {LaidLink | LaidNode} d */
function seenOf(d) {
  return isLink(d)
    ? { again: drawsAgain(/** @type {LaidLink} */ (d), leafDraws(projection)) }
    : { share: shareOf(/** @type {LaidNode} */ (d)) };
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
      ? nameOf(/** @type {LaidLink} */ (d).source) + " → " + nameOf(/** @type {LaidLink} */ (d).target)
      : nameOf(/** @type {LaidNode} */ (d))));
  tip.append(label);

  const chips = chipsOf(d, { printed: "printed_chip", inferred: "inferred_chip" }, seenOf(d));
  // A caveat about a node, said at the mark.
  const cavs = asLink ? [] : caveatsFor(/** @type {LaidNode} */ (d).id);
  if (cavs.length) chips.push({ cls: "chip caveat", words: say("caveat_chip", { n: cavs.length }) });
  const meta = h("div", "tip-meta");
  chips.forEach((c, i) => meta.append(...(i ? [" "] : []), h("span", c.cls, c.words)));
  tip.append(meta);
  if (asLink) {
    const l = /** @type {LaidLink} */ (d);
    // The chip says what kind of row; the sentence says what it reduces.
    if (l.contra) tip.append(h("div", "tip-meta", l.contra));
    if (l.partition) tip.append(h("div", "tip-meta", PARTITION_NOTE));
    if (l.merged) tip.append(h("div", "tip-meta", l.merged));
    tip.append(h("div", "facts", l.fact_ids.join(" ")));
  } else {
    const n = /** @type {LaidNode} */ (d);
    for (const b of balancesOf(n)) {
      const said = h("div", "tip-meta", b.words);
      if (b.derived) said.append(" ", h("span", "chip derived", say("inferred_chip")));
      tip.append(said);
    }
    if (n.rationale) tip.append(h("div", "tip-meta", n.rationale));
    const note = form.reductionNote(markOfView(n));
    if (note) tip.append(h("div", "tip-meta", note));
    // The laid mark and not the view's: the drawing is what is cut.
    const cut = form.drawnNote(n);
    if (cut) tip.append(h("div", "tip-meta tip-cut", cut));
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
  tipFocused = false;
  el("tooltip").hidden = true;
}

/** Whether the tooltip on screen is one a mark's focus showed, which its blur hides. */
let tipFocused = false;

/**
 * On a blur, hides only a tooltip focus showed: a press between marks blurs
 * one while the pointer's tooltip is on the other.
 * @param {Event} [event]
 */
function hideTipOf(event) {
  if (event && event.type === "blur" && !tipFocused) return;
  hideTip();
}

/**
 * Tooltip, and under the pointer a drawer preview. Keyboard focus pins via
 * the form's focus handler; the pointer's does not (`unasked`).
 * @param {MouseEvent | FocusEvent} event
 * @param {LaidLink | LaidNode} d
 */
export function hover(event, d) {
  showTip(event, d);
  tipFocused = event.type === "focus";
  if (typeof event.type === "string" && event.type.startsWith("pointer")) preview(d);
}

/**
 * Previews while nothing is pinned; the last preview stays so the pointer
 * can reach the drawer's links.
 * @param {LaidLink | LaidNode} d
 */
export function preview(d) {
  if (pinned || previewed === d) return;
  previewed = d;
  paintDetail(d);
}

/**
 * Pinning anything but the followed node drops a node's isolation
 * (followsNode), so the chart never follows a node the drawer hides; a fund
 * group's stays.
 * @param {LaidLink | LaidNode} d
 */
export function pin(d) {
  pinned = d;
  previewed = null;
  paintDetail(d);
  if (followsNode() && (isLink(d) || d.id !== filter.isolated)) setIsolated("");
  else applyEmphasis();
}

/** A node's isolation goes with the pin, its Follow button gone. */
export function unpin() {
  pinned = null;
  resetDetail();
  if (followsNode()) setIsolated("");
  else applyEmphasis();
}

/**
 * pin's rule, after a redraw lays out and before it repaints: a node's
 * isolation goes, a group's stays unless nothing drawn carries it.
 */
function dropPin() {
  pinned = null;
  if (followsNode()) filter = Object.freeze(Object.assign({}, filter, { isolated: "" }));
  if (filter.isolated && !laidNodes.some((n) => isolates(filter.isolated, n))) {
    filter = Object.freeze(Object.assign({}, filter, { isolated: "" }));
  }
}

/** @returns {boolean} */
function followsNode() {
  return filter.isolated !== "" && !isGroupID(filter.isolated);
}

/** Pin offered only while a mark shows; Follow only while a node shows. */
export function syncPinButton() {
  const button = maybeEl("detail-pin");
  if (button) {
    button.hidden = !(pinned || previewed);
    button.setAttribute("aria-pressed", pinned ? "true" : "false");
  }
  const id = followable();
  const more = maybeEl("detail-menu");
  if (more) more.hidden = id === "" || !laidNodes.some((n) => n.id === id);
  const follow = maybeEl("detail-follow");
  if (!follow) return;
  follow.hidden = id === "";
  follow.setAttribute("aria-pressed", String(id !== "" && filter.isolated === id));
}

/** @returns {string} */
export function followable() {
  const d = pinned || previewed;
  return d && !isLink(d) ? d.id : "";
}

/**
 * Pin, Follow and More. More opens the node menu: the one route to it with
 * no right button or keyboard, as on a phone.
 */
export function wireDrawer() {
  const more = maybeEl("detail-menu");
  if (more) {
    // A press on More blurs its menu shut before the click lands (openMenu's
    // focusout), so whether to close is read at the press.
    let pressedOpen = false;
    more.addEventListener("pointerdown", () => {
      pressedOpen = chartMenu !== null && chartMenu.back === more;
      const doc = more.ownerDocument;
      const released = () => {
        doc.removeEventListener("pointerup", released, true);
        doc.removeEventListener("pointercancel", released, true);
        setTimeout(() => { pressedOpen = false; }, 0);
      };
      doc.addEventListener("pointerup", released, true);
      doc.addEventListener("pointercancel", released, true);
    });
    more.addEventListener("click", (e) => guarded("open this mark's menu", () => {
      e.stopPropagation();
      const wasOpen = pressedOpen || (chartMenu !== null && chartMenu.back === more);
      pressedOpen = false;
      if (wasOpen) {
        closeChartMenu();
        more.focus();
        return;
      }
      const id = followable();
      const d = laidNodes.find((n) => n.id === id);
      if (!d) return;
      const chart = maybeEl("chart");
      const mark = chart ? D3.select(chart).selectAll("g.node").filter(/** @param {LaidNode} n */ (n) => n.id === id).node() : null;
      openNodeMenu(d, null, mark || undefined, more);
    }));
  }
  const button = maybeEl("detail-pin");
  if (button) {
    button.addEventListener("click", () => guarded("pin this", () => {
      if (pinned) unpin();
      else if (previewed) pin(previewed);
    }));
  }
  const follow = maybeEl("detail-follow");
  if (!follow) return;
  follow.addEventListener("click", () => guarded("follow this money", () => {
    const id = followable();
    if (id) setIsolated(filter.isolated === id ? "" : id);
  }));
}

/**
 * The pages a mark cites. A node cites every page its ribbons cite on the
 * unfiltered chart (markOfView), so a filter hides no page its figure
 * stands on. Only a ribbonless node cites the document, and says so.
 * @param {LaidLink | LaidNode} d
 * @returns {{words: string, cited: FiscSource[]}}
 */
export function sourcesOf(d) {
  if (isLink(d)) return { words: "Sources:", cited: /** @type {LaidLink} */ (d).locators };
  const n = /** @type {LaidNode} */ (d);
  if (n.locators) return { words: "Sources:", cited: n.locators };
  const at = markOfView(n);
  const ribbons = (at.sourceLinks || []).concat(at.targetLinks || []);
  if (ribbons.length) return { words: "Sources:", cited: core.pagesCitedBy(ribbons) };
  return { words: say("sources_chart"), cited: projection ? projection.metadata.sources : [] };
}

/** @param {LaidLink | LaidNode} d */
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
    ? nameOf(/** @type {LaidLink} */ (d).source) + " → " + nameOf(/** @type {LaidLink} */ (d).target)
    : nameOf(/** @type {LaidNode} */ (d))));

  const chips = h("div", "prov");
  chips.append(...chipsOf(d, { printed: "printed_by_city", inferred: "our_inference" }, seenOf(d))
    .map((c) => h("span", c.cls, c.words)));
  panel.append(chips);
  if (asLink) {
    const l = /** @type {LaidLink} */ (d);
    if (l.contra) panel.append(h("p", "why", l.contra));
    if (l.partition) panel.append(h("p", "why", PARTITION_NOTE));
    if (l.merged) panel.append(h("p", "why", l.merged));
    // A derived ribbon with no fact of its own speaks in its derived end's words.
    const end = l.source.derived ? l.source : l.target;
    if (l.fact_ids.length) panel.append(h("div", "facts", "Facts: " + l.fact_ids.join(" ")));
    else if (l.derived && end.source_note) panel.append(h("p", "subtle", end.source_note));
  } else {
    const n = /** @type {LaidNode} */ (d);
    // A printed balance cites its own fact and pages; a derived change says why.
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
    // Looked up in Go's refs, never rebuilt from the id: a missing link is
    // visible, a link to the wrong paragraph is not.
    const refs = shownYear && Array.isArray(shownYear.caveats) ? shownYear.caveats : [];
    for (const c of caveatsFor(n.id)) {
      const why = h("p", "why");
      why.append(document.createTextNode("\u26a0 " + c.summary + " "));
      const ref = refs.find((r) => r.id === c.id);
      if (ref && ref.href) why.append(link("Read it in full", ref.href));
      panel.append(why);
    }
  }

  const prov = h("div", "prov");
  const sources = sourcesOf(d);
  prov.append(h("span", "subtle", sources.words));
  for (const c of citations(sources.cited)) {
    prov.append(link(c.label, c.href));
  }
  panel.append(prov);
}

/* ---- Legend and search ---- */

/**
 * In Go's order; an unordered group sorts last, by id.
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
 * @param {string} id
 * @returns {string}
 */
export function groupLabel(id) {
  const n = groupIndex.get(id) ||
    (column && Array.isArray(column.nodes) ? column.nodes.find((/** @type {any} */ x) => x && x.id === id) : null);
  return n && n.label ? n.label : id;
}

/**
 * One follow-toggle swatch per drawn fund group. Once a group splits, the
 * swatch is the only key to its funds' hue.
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
  // The heading ships hidden: with no script there are no buttons to head.
  const head = maybeEl("legend-head");
  if (head) head.hidden = legend.children.length === 0;
}

/* ---- Search ---- */

/**
 * Marks and undrawn schedule nodes whose label holds the query, prefix
 * matches first. Not offered: anything the group or node filters remove,
 * or a reveal that would lapse a node filter or, under `only`, be left out
 * (revealedUnderFilter). One the kind or smallest filter would remove is
 * offered, and Enter says so and draws nothing.
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
    const doc = fetched;
    const drawn = new Set(laidNodes.map((n) => n.id));
    const index = new Map(doc.nodes.map((n) => [n.id, n]));
    const out = filteredOutWithin();
    const asks = filter.only !== "" || (projection !== null && liveFilters(projection).size > 0);
    /** @type {Map<string, RevealAsked>} */
    const memo = new Map();
    /** @type {Map<string, FiscProjection>} */
    const shaped = new Map();
    for (const n of doc.nodes) {
      if (drawn.has(n.id) || filter.groups.has(core.fundGroupOf(index, n)) || filter.groups.has(n.id) || out.has(n.id)) continue;
      // Asked of a match alone: the reveal is a shape and a filter.
      if (asks && folded(n).includes(q) && !revealedUnderFilter(n.id, { shaped, memo, drawn: filter.only !== "" })) continue;
      pool.push(n);
    }
  }
  return pool.filter((n) => folded(n).includes(q)).sort((a, b) =>
    Number(!folded(a).startsWith(q)) - Number(!folded(b).startsWith(q)) ||
    folded(a).localeCompare(folded(b)) || (a.id < b.id ? -1 : a.id > b.id ? 1 : 0));
}

/**
 * The expansions that draw `id`: core.revealChain as a list Back can walk,
 * folding a node split another way first and drawing out a capping tail.
 * @param {string} id
 * @param {Map<string, FiscProjection>} [shaped] memo of each chain's shaped chart
 * @returns {string[]}
 */
export function revealEntries(id, shaped = new Map()) {
  if (!fetched) return expansions.slice();
  const doc = fetched;
  const chain = core.revealChain(doc, id);
  let next = expansions.slice();
  for (const { node, key } of chain) {
    const open = expansionState(doc, next).frontier;
    if (core.openSplitOf(doc, open, node) === key) continue;
    const inside = core.withinNode(doc, node);
    next = next.filter((e) => !entryInside(e, inside)).concat([entryOf(node, key)]);
  }
  const own = chain[chain.length - 1];
  if (own) {
    const at = next.join("\n");
    let drawn = shaped.get(at);
    if (!drawn) {
      const state = expansionState(doc, next);
      drawn = formOnScreen().shape(doc, { chart: CONFIG.overview, frontier: state.frontier, whole: state.whole });
      shaped.set(at, drawn);
    }
    const tail = core.splitTailID(own.node, own.key);
    if (!drawn.nodes.some((n) => n.id === id) && drawn.nodes.some((n) => n.id === tail)) next.push(tail);
  }
  return next;
}

/**
 * "out:"/"only:" entries for each node filter still biting on `doc`.
 * @param {FiscProjection} doc
 * @returns {Set<string>}
 */
function liveFilters(doc) {
  const ids = new Set(doc.nodes.map((n) => n.id));
  const live = new Set(Array.from(filter.nodes).filter((id) => ids.has(id)).map((id) => "out:" + id));
  if (filter.only !== "" && onlyKeeps(doc, filter, hueIndex(doc))) live.add("only:" + filter.only);
  return live;
}

/**
 * Null where the shape threw; `ends` null where the filter threw.
 * @typedef {{lapses: boolean, ends: Set<string> | null} | null} RevealAsked
 */

/**
 * Whether revealing `id` keeps every live node filter live and (unless
 * `drawn` is false) draws `id` under them. True where anything throws:
 * redraw reports that.
 * @param {string} id
 * @param {{shaped?: Map<string, FiscProjection>, memo?: Map<string, RevealAsked>, drawn?: boolean}} [o]
 * @returns {boolean}
 */
function revealedUnderFilter(id, o = {}) {
  if (!fetched) return false;
  const doc = fetched;
  const memo = o.memo || new Map();
  let asked;
  try {
    const next = revealEntries(id, o.shaped);
    const key = next.join("\n");
    if (!memo.has(key)) memo.set(key, askReveal(doc, next));
    asked = memo.get(key);
  } catch {
    return true;
  }
  if (!asked) return true;
  if (asked.lapses) return false;
  return o.drawn === false || !asked.ends || asked.ends.has(id);
}

/**
 * @param {FiscProjection} doc the expansion schedule
 * @param {string[]} next
 * @returns {RevealAsked}
 */
function askReveal(doc, next) {
  let drawn;
  try {
    const state = expansionState(doc, next);
    drawn = formOnScreen().shape(doc, { chart: CONFIG.overview, frontier: state.frontier, whole: state.whole });
  } catch {
    return null;
  }
  const after = liveFilters(drawn);
  const lapses = projection !== null && Array.from(liveFilters(projection)).some((f) => !after.has(f));
  if (lapses) return { lapses: true, ends: null };
  try {
    const ends = new Set();
    for (const l of filterFor(drawn).doc.links) ends.add(l.source).add(l.target);
    return { lapses: false, ends: ends };
  } catch {
    return { lapses: false, ends: null };
  }
}

/** The search's options as listed, and the one the arrows are on (-1 for none). */
export let searchState = { hits: /** @type {(LaidNode | FiscNode)[]} */ ([]), active: -1 };

/** Lists matches with the arrows on none. */
export function paintSearch() {
  const input = /** @type {HTMLInputElement | null} */ (maybeEl("search-input"));
  const list = maybeEl("search-list");
  const status = maybeEl("search-status");
  if (!input || !list) return;
  const hits = searchMatches(input.value);
  searchState = { hits, active: -1 };
  list.replaceChildren(...hits.map((n, k) => {
    // Label, role where options read alike, and what Enter does.
    const alike = hits.some((o) => o !== n && o.label === n.label);
    const laid = laidNodes.some((x) => x.id === n.id);
    const offer = !laid ? say("search_reveals") : splittable(n) ? say("search_opens") : expandable(n) ? say("search_expands") : "";
    const words = n.label + (alike ? " (" + String(n.role).replace(/_/g, " ") + ")" : "") + (offer ? ", " + offer : "");
    const option = h("li", "search-option", words);
    option.id = "search-option-" + k;
    option.setAttribute("role", "option");
    option.setAttribute("aria-selected", "false");
    option.dataset.id = n.id;
    option.addEventListener("click", () => guarded("find this mark", () => chooseSearch(n.id, false), refuseInPlace));
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

/** @param {number} delta 1 or -1 */
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
 * Goes to a mark, revealing it if undrawn (revealEntries). With `open` (the
 * search's Enter) it does what activation offers (openOffered); otherwise
 * it follows and focuses it. A refusal leaves pin, isolation and drawer
 * as they were (refuseInPlace).
 * @param {string} id
 * @param {boolean} open
 * @returns {boolean} whether the mark is on the chart
 */
export function chooseSearch(id, open) {
  const input = /** @type {HTMLInputElement | null} */ (maybeEl("search-input"));
  let d = laidNodes.find((n) => n.id === id);
  closeSearch();
  let revealed = false;
  if (!d && expanding() && nodeOf(id)) {
    /** @type {Map<string, FiscProjection>} */
    const shaped = new Map();
    if (filteredOutWithin().has(id) || !revealedUnderFilter(id, { shaped })) {
      refuseInPlace("The filters leave " + /** @type {FiscNode} */ (nodeOf(id)).label + " out of the chart");
      return false;
    }
    const next = revealEntries(id, shaped);
    if (!redraw(next, "That could not be found")) return false;
    d = laidNodes.find((n) => n.id === id);
    revealed = true;
  }
  if (!d) return false;
  if (input) input.value = d.label;
  if (!revealed && open && offers(d)) {
    if (openOffered(d) && input) input.value = "";
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

/** Escape closes the list, then clears the box, never undoing a split. */
export function wireSearch() {
  const box = maybeEl("search");
  const input = /** @type {HTMLInputElement | null} */ (maybeEl("search-input"));
  const list = maybeEl("search-list");
  if (!box || !input || !list) return;
  box.removeAttribute("hidden");
  // Listing changes nothing on screen, so a throw leaves everything as it was.
  input.addEventListener("input", () => guarded("find a mark", paintSearch, refuseInPlace));
  input.addEventListener("keydown", (e) => {
    if (e.key === "ArrowDown" || e.key === "ArrowUp") {
      e.preventDefault();
      const delta = e.key === "ArrowDown" ? 1 : -1;
      guarded("find a mark", () => moveSearch(delta), refuseInPlace);
      return;
    }
    if (e.key === "Enter") {
      const chosen = searchState.hits[searchState.active];
      if (!chosen) return;
      e.preventDefault();
      guarded("open this mark", () => chooseSearch(chosen.id, true), refuseInPlace);
      return;
    }
    if (e.key === "Escape") {
      // Escape here must not reach the document's unpin.
      e.stopPropagation();
      if (!list.hasAttribute("hidden")) closeSearch();
      else {
        input.value = "";
        guarded("find a mark", paintSearch, refuseInPlace);
      }
    }
  });
  // A click on an option would blur the box first and close the list under it.
  list.addEventListener("mousedown", (e) => e.preventDefault());
  input.addEventListener("blur", () => closeSearch());
}

/* ---- Theme ---- */

/** d3 wrote resolved hues onto the marks, so a theme switch repaints. */
export function wireTheme() {
  document.addEventListener(THEME_EVENT, () => paint());
}

/* ---- The rail beside the chart ---- */

/** Whether the viewport has room for the rail beside the chart. */
export function besideMatches() {
  return typeof window.matchMedia === "function" && window.matchMedia(BESIDE_QUERY).matches;
}

/** Beside or above per BESIDE_QUERY, tracking full screen; open by default beside. */
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
    // The rail is the page's, not the view's: hash and history do not move.
    toggle.addEventListener("click", () => { railOpen = !railOpen; paintRail(); });
  }
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

/* ---- Full screen ---- */

/** @returns {boolean} */
export function isFullscreen() {
  const card = maybeEl("chart-card");
  return Boolean(card) && document.fullscreenElement === card;
}

/** However full screen changed: button, Escape or the browser's own control. */
export function onFullscreenChange() {
  const button = maybeEl("chart-fullscreen");
  if (button) button.textContent = say(isFullscreen() ? "full_screen_exit" : "full_screen");
  askRoom();
}

/** One redraw per frame however often asked (wireRoom). */
export function askRoom() {
  if (roomAsked) return;
  roomAsked = true;
  const draw = () => {
    roomAsked = false;
    fitRoom();
  };
  if (typeof window.requestAnimationFrame === "function") window.requestAnimationFrame(draw);
  else window.setTimeout(draw, 0);
}

/** @returns {{width: number, height: number, full: boolean, top: number}} */
function windowNow() {
  const svg = maybeEl("chart");
  const top = svg ? chartTop(svg.getBoundingClientRect()) : 0;
  return { width: window.innerWidth, height: window.innerHeight, full: isFullscreen(), top };
}

/**
 * Redraws in place only where the room changed enough to lay out on another
 * canvas. A height-only move under CHROME_SLACK is the same window
 * (sameWindow): redrawing for a phone's address bar would close what the
 * reader opened as they scroll. A throwing layout leaves the chart and says why.
 * @returns {boolean} whether it was drawn again
 */
export function fitRoom() {
  const doc = chartDrawn();
  if (!doc) return false;
  const now = windowNow();
  if (sameWindow(now)) return false;
  const room = roomHeight();
  if (room === heldRoom) return false;
  const form = formOnScreen();
  const chart = chartOnScreen();
  if (form.canvas(laidNodes, { chart, room }) === form.canvas(laidNodes, { chart, room: laidRoom })) {
    heldRoom = room;
    drawnWindow = now;
    return false;
  }
  // layOut assigns nothing until its layout has returned.
  let laid;
  try {
    laid = layOut(doc, boxesOf(laidNodes), room);
  } catch (e) {
    refuseInPlace("The chart could not be fitted to the window: " + (e instanceof Error ? e.message : String(e)));
    return false;
  }
  render(laid);
  return true;
}

/**
 * @param {{width: number, height: number, full: boolean, top: number}} now
 * @returns {boolean}
 */
function sameWindow(now) {
  return now.width === drawnWindow.width && now.full === drawnWindow.full &&
    Math.abs(now.top - drawnWindow.top) < TOP_SLACK && Math.abs(now.height - drawnWindow.height) < CHROME_SLACK;
}

/**
 * heldRoom while sameWindow, else roomHeight.
 * @returns {number}
 */
function layRoom() {
  return chartDrawn() && sameWindow(windowNow()) ? heldRoom : roomHeight();
}

/**
 * "Show the chart anyway": sets data-chart-anyway, moves focus to the card
 * (the button folds away), and asks for room since the card drew folded.
 */
export function wireNarrow() {
  const button = maybeEl("narrow-show");
  if (!button) return;
  button.removeAttribute("hidden");
  button.addEventListener("click", () => {
    document.documentElement.setAttribute("data-chart-anyway", "");
    const card = maybeEl("chart-card");
    if (card) {
      card.setAttribute("tabindex", "-1");
      card.focus();
      card.addEventListener("blur", () => card.removeAttribute("tabindex"), { once: true });
    }
    askRoom();
  });
}

/** Follows the window's size with the chart's room (askRoom). */
export function wireRoom() {
  window.addEventListener("resize", askRoom);
}

/** Shown only where the browser offers full screen. */
export function wireFullscreen() {
  const button = maybeEl("chart-fullscreen");
  const card = maybeEl("chart-card");
  if (!button || !card || !document.fullscreenEnabled || typeof card.requestFullscreen !== "function") return;
  button.textContent = say("full_screen");
  button.removeAttribute("hidden");
  // A browser refusal changes nothing and fires no fullscreenchange.
  button.addEventListener("click", () => {
    void Promise.resolve(isFullscreen() ? document.exitFullscreen() : card.requestFullscreen()).catch(() => {});
  });
  document.addEventListener("fullscreenchange", onFullscreenChange);
}

/* ---- Zoom and pan ---- */

/** The furthest the chart zooms in; 1 is the chart whole, and it zooms out no further. */
export const ZOOM_MAX = 8;

/** How far one press of + or - zooms. */
export const ZOOM_STEP = 1.5;

/**
 * Rebound by every render, which resets the transform.
 * @type {any}
 */
export let zoomer = null;

/** @returns {{k: number, x: number, y: number, invert: (p: number[]) => number[], apply: (p: number[]) => number[]}} */
export function zoomOf() {
  const svg = maybeEl("chart");
  return svg ? D3.zoomTransform(svg) : D3.zoomIdentity;
}

/**
 * The same rule in and out of full screen: the wheel zooms only with Ctrl or
 * Cmd (a pinch arrives so), else scrolls; two fingers pinch; a one-finger or
 * primary drag pans the zoom only once zoomed in (else wirePan).
 * @param {any} event
 * @returns {boolean}
 */
export function zoomFilter(event) {
  if (event.type === "wheel") return Boolean(event.ctrlKey || event.metaKey);
  if (event.type === "touchstart") return (event.touches && event.touches.length > 1) || zoomOf().k > 1;
  return !event.ctrlKey && !event.button && zoomOf().k > 1;
}

/** Screen px a press moves before it is a drag rather than a click. */
export const DRAG_SLOP = 4;

/**
 * Pans the scroller under a mouse or pen drag at the whole chart (zoomed in,
 * the zoom pans). Past DRAG_SLOP it captures the pointer and swallows the
 * ending click, so a drag ending on a mark neither follows nor pins. The
 * scroller's own scrollbar and touch are left to it.
 * @param {HTMLElement} scroller the .chart-scroll
 */
export function wirePan(scroller) {
  /** @type {{id: number, x: number, left: number, moved: boolean} | null} */
  let drag = null;
  let swallow = false;
  scroller.addEventListener("pointerdown", (e) => {
    swallow = false;
    drag = null;
    if (e.button !== 0 || e.pointerType === "touch" || zoomOf().k !== 1 || e.target === scroller) return;
    drag = { id: e.pointerId, x: e.clientX, left: scroller.scrollLeft, moved: false };
  });
  scroller.addEventListener("pointermove", (e) => {
    if (!drag || e.pointerId !== drag.id) return;
    const dx = e.clientX - drag.x;
    if (!drag.moved) {
      if (Math.abs(dx) < DRAG_SLOP) return;
      drag.moved = true;
      // Captured only once a drag: a captured press clicks the scroller, not the mark.
      if (typeof scroller.setPointerCapture === "function") {
        try { scroller.setPointerCapture(e.pointerId); } catch { /* a pointer already gone */ }
      }
    }
    scroller.scrollLeft = drag.left - dx;
  });
  // The drag's click lands in its pointerup's task; any later click is another's.
  /** @param {PointerEvent} e */
  const end = (e) => {
    if (!drag || e.pointerId !== drag.id) return;
    swallow = drag.moved && e.type === "pointerup";
    drag = null;
    if (swallow) setTimeout(() => { swallow = false; }, 0);
  };
  scroller.ownerDocument.addEventListener("pointerup", end);
  scroller.ownerDocument.addEventListener("pointercancel", end);
  scroller.addEventListener("click", (e) => {
    if (!swallow) return;
    swallow = false;
    e.stopPropagation();
    e.preventDefault();
  }, true);
}

/**
 * Labels counter-scale through --label-scale to keep their size.
 * @param {{k: number, x: number, y: number, toString: () => string}} t
 */
export function applyZoom(t) {
  const svg = D3.select("#chart");
  svg.select("g.zoom-layer").attr("transform", t.toString());
  const node = svg.node();
  if (!node) return;
  node.style.setProperty("--label-scale", String(1 / t.k));
  node.classList.toggle("zoomed", t.k !== 1 || t.x !== 0 || t.y !== 0);
  paintReset();
}

/**
 * Unzoomed, unfiltered, unpinned, unsplit; the year is not part of it.
 * @returns {boolean}
 */
export function atOpeningView() {
  const t = zoomOf();
  return t.k === 1 && t.x === 0 && t.y === 0 && pinned === null && expansions.length === 0 &&
    filter.groups.size === 0 && filter.kinds.size === 0 && filter.nodes.size === 0 &&
    filter.only === "" && filter.smallest === 0 && filter.isolated === "";
}

/** Offers Reset view wherever there is something to reset (atOpeningView). */
export function paintReset() {
  const reset = maybeEl("chart-zoom-reset");
  if (!reset) return;
  if (atOpeningView()) reset.setAttribute("disabled", "");
  else reset.removeAttribute("disabled");
}

/**
 * Reset view to `#y=<year>` in one history step. Pin and zoom go only once
 * the opening view drew; a partial failure adds no step, keeps them,
 * rewrites the URL to what is on screen and says so.
 * @returns {Promise<string>} DREW, SUPERSEDED or FAILED
 */
export async function resetView() {
  if (!shownYear) return FAILED;
  restoringView = true;
  let moved;
  try {
    moved = await moveTo({ year: yearKey(shownYear), hide: [], hidekind: [], out: [], only: "", min: 0, iso: "", x: [] });
  } finally {
    restoringView = false;
  }
  if (moved.outcome === DREW) {
    unpin();
    resetZoom();
    recordView("push");
  } else if (moved.outcome === FAILED) {
    refuseInPlace("The view was not wholly reset: " + (moved.why || "the chart could not be drawn"));
    recordView("replace");
  }
  paintReset();
  return moved.outcome;
}

/**
 * The extent is stated: d3's default reads the viewBox through SVG geometry.
 * A double click is two activations, so it never zooms.
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

/** @param {number} factor */
export function zoomBy(factor) {
  if (zoomer) zoomer.scaleBy(D3.select("#chart"), factor);
}

/** Puts the chart back whole: the 0 key while the chart has the focus, and Reset view once it has redrawn the year. */
export function resetZoom() {
  if (zoomer) zoomer.transform(D3.select("#chart"), D3.zoomIdentity);
}

/**
 * Pans and scrolls a mark into sight only where it is out of sight, so a
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

/** Once, at boot: the svg outlives every render. */
export function wireZoom() {
  const svg = maybeEl("chart");
  if (!svg) return;
  const scroller = svg.parentElement;
  if (scroller && scroller.classList.contains("chart-scroll")) wirePan(scroller);
  const doc = svg.ownerDocument;
  const released = () => { pointing = false; };
  svg.addEventListener("pointerdown", () => { pointing = true; }, true);
  // A context menu ends the press too: the browser's own takes its release.
  for (const type of ["pointerup", "pointercancel", "contextmenu"]) doc.addEventListener(type, () => { if (pointing) setTimeout(released, 0); }, true);
  doc.addEventListener("lostpointercapture", released, true);
  const win = doc.defaultView;
  if (win) win.addEventListener("keydown", released, true);
  // Not in capture: an element's blur passes the window on its way down.
  if (win) win.addEventListener("blur", released);
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
    if (!pointing && d && typeof d.x0 === "number" && d.sourceLinks) revealMark(d);
  });
  const reset = maybeEl("chart-zoom-reset");
  if (reset) {
    reset.textContent = say("zoom_reset");
    reset.removeAttribute("hidden");
    reset.addEventListener("click", () => {
      void resetView().catch((e) => fail("The chart failed to draw: " + String(e)));
    });
  }
}

/* ---- The view in the URL ---- */

/**
 * The view as location.hash says it; min in cents, x in the order made.
 * @typedef {{year: string, hide: string[], hidekind: string[], out: string[], only: string, min: number, iso: string, x: string[]}} ViewState
 */

/** The hash's fields, in the order hashOf writes them. */
export const HASH_FIELDS = Object.freeze(["y", "hide", "hidekind", "out", "only", "min", "iso", "x"]);

/**
 * Read once at boot; a refused hash shows this year.
 * @type {FiscYear | null}
 */
export let openingYear = null;

/** Whether restoreView is moving the page, so nothing it draws writes the URL. */
let restoringView = false;

/**
 * @param {FiscYear} year
 * @returns {string}
 */
export function yearKey(year) {
  return columnKey(year.path);
}

/**
 * Null before a year has drawn. Lists are sorted, so one view has one hash.
 * @returns {ViewState | null}
 */
export function viewState() {
  if (!shownYear) return null;
  return {
    year: yearKey(shownYear),
    hide: Array.from(filter.groups).sort(),
    hidekind: Array.from(filter.kinds).sort(),
    out: Array.from(filter.nodes).sort(),
    only: filter.only,
    min: filter.smallest,
    iso: filter.isolated,
    // Unsorted: each entry applies to the frontier the earlier ones leave.
    x: expansions.slice(),
  };
}

/**
 * `#y=<year>`, then each non-default field.
 * @param {ViewState} st
 * @returns {string}
 */
export function hashOf(st) {
  const parts = ["y=" + hashValue(st.year)];
  if (st.hide.length) parts.push("hide=" + st.hide.map(hashValue).join(","));
  if (st.hidekind.length) parts.push("hidekind=" + st.hidekind.map(hashValue).join(","));
  if (st.out.length) parts.push("out=" + st.out.map(hashValue).join(","));
  if (st.only) parts.push("only=" + hashValue(st.only));
  if (st.min) parts.push("min=" + st.min);
  if (st.iso) parts.push("iso=" + hashValue(st.iso));
  if (st.x.length) parts.push("x=" + st.x.map(hashValue).join(","));
  return "#" + parts.join("&");
}

/**
 * "none" for an empty hash or in-page anchor; "refused", with why, for
 * anything not read whole. Nothing is guessed.
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
  const out = list("out");
  const x = list("x");
  if ([...hide, ...hidekind, ...out, ...x].some((id) => !id)) return { kind: "refused", why: "it holds an empty name" };
  const min = fields.has("min") ? fields.get("min") || "" : "0";
  if (!/^\d+$/.test(min) || !Number.isSafeInteger(Number(min))) {
    return { kind: "refused", why: "its smallest flow, " + min + ", is not a whole number of cents" };
  }
  return {
    kind: "view",
    state: {
      year, hide: hide.sort(), hidekind: hidekind.sort(), out: out.sort(), only: text(fields.get("only") || ""),
      min: Number(min), iso: text(fields.get("iso") || ""), x,
    },
  };
}

/**
 * "push" for a split, unsplit or year; "replace" for a filter or isolation.
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
 * Moves the page to a view or says why not. Undrawn groups, kinds and nodes
 * are inert. Expansions go before the isolation, which can name a member
 * only a split draws.
 * @param {ViewState} st
 * @returns {Promise<{outcome: string, why?: string, error?: string}>}
 */
async function moveTo(st) {
  const refuse = (/** @type {string} */ why) => ({ outcome: FAILED, why });
  const year = (CONFIG.years || []).find((/** @type {FiscYear} */ y) => yearKey(y) === st.year);
  if (!year) return refuse("this page publishes no year " + st.year);
  const kinds = CONFIG.kind_labels || {};
  const kind = st.hidekind.find((k) => !Object.prototype.hasOwnProperty.call(kinds, k));
  if (kind !== undefined) return refuse("this page names no flow kind " + kind);
  const removals = { groups: new Set(st.hide), kinds: new Set(st.hidekind), nodes: new Set(st.out), only: st.only, smallest: st.min };
  if (!shownYear || shownYear !== year || !projection) {
    let got;
    try {
      // The removals become the page's filter only once the year draws.
      got = await showYear(year, [], Object.assign({}, removals, { isolated: "" }));
    } catch (e) {
      const error = e instanceof Error ? e.message : String(e);
      return { outcome: FAILED, why: year.label + " cannot be drawn as it names: " + error, error: error };
    }
    if (got !== DREW) return { outcome: got, why: "the year " + st.year + " could not be drawn" };
    syncYearControl();
  } else if (!setFilter(Object.assign({}, removals, { isolated: "" }))) {
    return refuse("the chart could not be filtered as it names");
  }
  const group = st.hide.find((g) => fundGroupPlace(g) < 0);
  if (group !== undefined) return refuse(year.label + " carries no fund group " + group);
  // A listed fund group named in `out` is hidden as its box hides it.
  const listed = Array.from(filter.nodes).filter(listedGroup);
  if (listed.length) {
    const groups = new Set(filter.groups);
    for (const id of listed) groups.add(id);
    const nodes = new Set(Array.from(filter.nodes).filter((id) => !groups.has(id)));
    if (!setFilter({ groups: groups, nodes: nodes })) return refuse("the chart could not be filtered as it names");
  }
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
  // A followed node is pinned (pin's rule); a group's swatch is its own toggle.
  if (st.iso && !isGroupID(st.iso)) followNode(st.iso);
  else setIsolated(st.iso);
  return { outcome: DREW };
}

/**
 * Shows the hash's view, or refuses it and shows the opening view, never a
 * nearest guess; then rewrites the URL to what is on screen.
 * @param {string} hash
 * @returns {Promise<string>} DREW, SUPERSEDED or FAILED
 */
export async function restoreView(hash) {
  const read = parseHash(hash);
  const opening = openingYear || (CONFIG.years || [])[(CONFIG.years || []).length - 1];
  /** @type {ViewState} */
  const fallback = { year: yearKey(opening), hide: [], hidekind: [], out: [], only: "", min: 0, iso: "", x: [] };
  const named = read.kind !== "none";
  let moved = /** @type {{outcome: string, why?: string, error?: string}} */ ({ outcome: FAILED, why: read.kind === "refused" ? read.why : "" });
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
  // The opening view itself throwing is said as any draw that fails is.
  if (!named) {
    if (moved.error !== undefined) fail("The chart failed to draw: " + moved.error);
    return FAILED;
  }
  // REFUSED: the opening view, then the banner, since a year drawing clears it.
  restoringView = true;
  let back;
  try {
    back = await moveTo(fallback);
  } finally {
    restoringView = false;
  }
  if (back.outcome === SUPERSEDED) return SUPERSEDED;
  if (back.error !== undefined) {
    fail("This link names a view this page does not draw: " + moved.why +
      ", and the opening view failed to draw: " + back.error);
    return FAILED;
  }
  fail("This link names a view this page does not draw: " + moved.why +
    ". The page shows its opening view instead.");
  if (back.outcome === DREW) recordView("replace");
  return FAILED;
}

/* ---- Boot ---- */

/** Returns the drawer to its state with nothing previewed; the caller lets any pin go. */
export function resetDetail() {
  previewed = null;
  const panel = el("detail");
  panel.replaceChildren();
  panel.append(h("p", "subtle", say("detail_empty")));
  syncPinButton();
}

/**
 * Refuses visibly: in the drawer and a role="alert" banner, since a reader
 * may never scroll to the drawer. Drops the pin.
 * @param {string} message
 */
export function fail(message) {
  const had = pinned;
  pinned = null;
  if (followsNode()) setIsolated("");
  else if (had) applyEmphasis();
  refuseInPlace(message);
}

/**
 * Refuses a change that was rolled back, keeping pin, isolation and a
 * pinned drawer as they were.
 * @param {string} message
 */
export function refuseInPlace(message) {
  if (!pinned) {
    previewed = null;
    const panel = el("detail");
    panel.replaceChildren();
    panel.append(h("p", "", message));
    for (const id of ["detail-pin", "detail-follow", "detail-menu"]) {
      const button = maybeEl(id);
      if (button) button.hidden = true;
    }
  }

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

/** So a recovering year switch leaves no stale alert. */
export function clearRefusal() {
  const content = document.querySelector("main");
  if (!content) return;
  const existing = content.querySelector(".refusal");
  if (existing) existing.remove();
}



/**
 * @param {any} doc
 * @param {string} what
 */
export function isDocument(doc, what) {
  if (doc && typeof doc === "object") return true;
  // A 200 with body `null` is a real server answer, not a shape for a schema.
  fail(
    "This page will not draw " + what + ": the file is not a document at all. " +
    "It is most likely an error page served with a success status. Nothing on " +
    "the page was changed."
  );
  return false;
}


/**
 * Fetches and vets a column, or returns null having told the reader, unless
 * `superseded`, which is asked before every banner. Both awaits are in the
 * try, so a malformed body is refused rather than rejected.
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
    // A rejected fetch is likely file://. `e.name`, not instanceof: another
    // realm's error fails instanceof.
    fail(e && /** @type {any} */ (e).name === "SyntaxError"
      ? "Could not read " + path + ": the file is not valid JSON, so it is " +
        "truncated or was not the document this page expected."
      : "Could not load " + path + ". If you opened this file directly, the browser " +
        "blocks the request: serve the directory over HTTP instead, e.g. " +
        "python3 -m http.server -d dist 8000");
    return null;
  }
  if (superseded()) return null;
  if (!isDocument(doc, path)) return null;
  // The one check Go cannot make: which deploy's copy the browser holds.
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


/** SUPERSEDED (a later switch took over) is not FAILED (the reader was told). */
export const DREW = "drew";
export const SUPERSEDED = "superseded";
export const FAILED = "failed";

/**
 * Entries the new year can make, applied in order; the rest, and all made
 * inside them, dropped.
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
  /** @type {string[]} */
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
 * Fetches and draws a year; every word comes from CONFIG.years.
 * @param {FiscYear} year
 * @param {readonly string[]} [carry] expansions to keep where they apply
 * @param {Readonly<FilterState> | null} [removals] filter to draw under; null for the page's
 * @returns {Promise<string>} DREW, SUPERSEDED or FAILED
 */
export async function showYear(year, carry = [], removals = null) {
  // Without the token, the slower of two in-flight fetches wins.
  const token = ++switching;
  const superseded = () => token !== switching;
  const loaded = await loadColumn(year.path, superseded);
  if (superseded()) return SUPERSEDED;
  if (!loaded) return FAILED;

  const doc = selectSchedule(loaded, CONFIG.primary);
  if (!doc) return FAILED;

  // Swapped because shapeFor and askAffordances read them; restored on a throw.
  const was = { column, fetched, expansions, frontier, wholeSplits, laidNodes, laidRoom, groupIndex, projection, shown, whole, drawnNames };
  const before = fetched;
  column = loaded;
  // A refusal must name a node by this year's label.
  drawnNames = new Map();
  fetched = doc;
  expansions = [];
  frontier = core.isExpansionSchedule(doc) ? core.rootFrontier(doc) : null;
  wholeSplits = new Set();
  /** @type {string[]} */
  let dropped = [];
  let laid;
  try {
    if (frontier && carry.length) {
      const over = carriedOver(doc, carry);
      const state = expansionState(doc, over.kept);
      expansions = over.kept;
      frontier = state.frontier;
      wholeSplits = state.whole;
      dropped = over.dropped;
    }
    // The fold unions the fact ids it merges, so nothing is lost.
    const drawn = shapeFor(doc);
    const filtered = filterFor(drawn, removals || filter);
    const room = layRoom();
    const unfiltered = wholeOf(drawn, filtered, room);
    laid = layOut(filtered.doc, undefined, room);
    projection = drawn;
    shown = filtered;
    whole = unfiltered;
    askAffordances();
  } catch (e) {
    ({ column, fetched, expansions, frontier, wholeSplits, laidNodes, laidRoom, groupIndex, projection, shown, whole, drawnNames } = was);
    throw e;
  }

  clearRefusal();
  // A pin and an isolation belong to the year they were made in.
  pinned = null;
  filter = Object.freeze(Object.assign({}, removals || filter, { isolated: "" }));
  // A name drawn in another year is not this year's.
  outNames.clear();
  resetDetail();
  hideTip();

  paintYearWords(year);
  paintSplitsLine(year, dropped, before);
  paintBulk();
  buildLegend();
  buildFilterPanel();
  paintChartHint();
  render(laid);
  paintFilterLine();
  recordView("push");
  return DREW;
}

/**
 * Every per-year word on the page. Exhaustive: add any new per-year write here.
 * @param {FiscYear} year
 */
export function paintYearWords(year) {
  const hero = maybeEl("hero");
  if (hero) hero.replaceChildren(h("span", "label", year.hero.label), ": ", h("strong", "value", year.hero.value));

  const lede = maybeEl("lede-year");
  if (lede) lede.textContent = year.lede;

  shownYear = year;
  syncYearControl();
  paintCounts();

  // Shared with every redraw, so the two cannot name the chart differently.
  paintChartName();

  document.title = year.title;
}

/**
 * Browsers restore a select across reload and Back without firing change, so
 * the page opens on it; else the newest (CONFIG.years is oldest first).
 * @param {FiscYear[]} years
 * @returns {FiscYear}
 */
export function checkedYear(years) {
  const select = /** @type {HTMLSelectElement | null} */ (maybeEl("year-select"));
  // A key this config does not publish is a stale restore.
  const year = select ? years.find((y) => yearKey(y) === select.value) : undefined;
  return year || years[years.length - 1];
}

/** Also carries the year onto the top bar's links. */
export function syncYearControl() {
  if (shownYear) selectYear(document, yearKey(shownYear));
}

/**
 * A year that fails to load moves the select back.
 * @param {FiscYear[]} years
 */
export function wireYears(years) {
  wireYearSelect(document, (key) => {
    const year = years.find((y) => yearKey(y) === key);
    if (!year) return;
    // A refused switch leaves the page on its year, so the control goes back.
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
  // app.js is cached apart and carries no stamp, so a constant is the only
  // handshake with the page. Nothing holds the modules to each other (fisc-mokd).
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
  // The projection is versioned apart from the page.
  if (CONFIG.projection_version !== PROJECTION_VERSION) {
    fail("This page will not draw: its columns assemble to projection_version " +
      CONFIG.projection_version + " and this script reads projection_version " +
      PROJECTION_VERSION + ". A cached copy of the script is the likely cause, so " +
      "reload to pick up the current one. Drawing it anyway would produce a chart " +
      "that is wrong rather than one that fails.");
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
  wireBulk();
  wireFullscreen();
  wireNarrow();
  wireRoom();
  wireZoom();

  // Wired before the first fetch: a failed opening year can be followed by one that draws.
  document.addEventListener("keydown", (e) => {
    if (e.key === "Escape") {
      // Escape undoes no split: one gesture can open several at once.
      if (closeChartMenu(true)) return;
      hideTip();
      // unpin lets a node's isolation go; a fund group's is the swatch's.
      unpin();
      if (filter.isolated) setIsolated("");
    }
  });
  if (typeof window.matchMedia === "function") {
    // d3 wrote resolved hues, so an OS theme switch repaints.
    window.matchMedia("(prefers-color-scheme: dark)").addEventListener("change", () => paint());
  }

  openingYear = checkedYear(years);
  // Back and Forward walk the views this page recorded (recordView).
  window.addEventListener("popstate", () => {
    void restoreView(window.location.hash).catch((e) => fail("The chart failed to draw: " + String(e)));
  });
  // Ignored: restoreView and showYear have told the reader.
  await restoreView(window.location.hash);
  // What paints above the chart after first draw can move its top; refit now,
  // so nothing is pending after boot.
  fitRoom();
}

/** The template calls this; importing draws nothing. */
export function boot() {
  return main().catch((e) => fail("The chart failed to draw: " + String(e)));
}
