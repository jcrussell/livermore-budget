// @ts-check
/**
 * fisc — the published page's client script.
 *
 * Served exactly as committed; the page template boots it by importing [boot].
 * Every top-level binding is exported so the tests import the shipped file,
 * and nothing runs at import. Nothing runs tsc: `@ts-check` is for an editor.
 *
 * Go validates every artifact against schema/ before writing it, so nothing
 * here re-checks a shape. What this file refuses is what no schema can answer:
 * a 200 carrying an error page, and a file cached from before the last deploy.
 */

/* global d3 */

/**
 * @typedef {Object} FiscNode
 * @property {string} id
 * @property {string} label
 * @property {number} tier
 * @property {string} parent
 * @property {string} constraint_tier
 * @property {string} role
 * @property {boolean} derived
 * @property {string} rationale
 * @property {string} source_note
 * @property {string[]} [folds] the ids a synthetic aggregate stands for; only
 *   on capColumn's aggregate, whose members no parent chain records.
 * @property {number} [in_cents] a residual's or a gap's figure, summed here over
 *   the ribbons carried onto it or the difference it stands for
 * @property {number} [out_cents]
 * @property {FiscSource[]} [locators] a gap's citations: every page a ribbon
 *   touching the opened node was read from, in either document
 * @property {string} [carried_from] the stem of the document a node was carried
 *   from into a window, set once and never cleared
 * @property {number} [fixedValue] the figure d3-sankey sizes the node at where
 *   its drawn ribbons do not add up to it (markAmounts)
 */

/**
 * @typedef {Object} FiscLink
 * @property {string} source
 * @property {string} target
 * @property {number} value_cents
 * @property {string} kind
 * @property {string} transfer_id
 * @property {string[]} fact_ids
 * @property {FiscSource[]} locators
 * @property {boolean} derived
 * @property {string} [contra]  the document's words for a link the schedule
 *   prints as a reduction; present only on such a link.
 * @property {boolean} [partition]  the ribbon divides one printed table along
 *   a second axis rather than following money; the projection's call, since
 *   the client cannot tell a cross-tab from a chain.
 */

/**
 * @typedef {Object} FiscSource
 * @property {string} doc_id
 * @property {number[]} pages
 */

/**
 * @typedef {Object} FiscMetadata
 * @property {string} generated_by
 * @property {number} fiscal_year
 * @property {string} fiscal_year_label
 * @property {string} basis
 * @property {string[]} scopes
 * @property {string} currency
 * @property {string} units
 * @property {FiscSource[]} sources
 * @property {Record<string, number>} headline
 * @property {{facts:number, nodes:number, links:number}} counts
 * @property {FiscCaveat[]} caveats
 */

/**
 * One thing the document cannot show. An empty `applies_to` means
 * document-wide, not "not filled in".
 * @typedef {Object} FiscCaveat
 * @property {string} id
 * @property {string} summary
 * @property {string} text
 * @property {string[]} applies_to
 */

/**
 * A caveat as the page shows it: a summary and a link, deliberately no `text`.
 * `href` is empty when the site has no caveats page, and the renderer falls
 * back to plain text.
 * @typedef {Object} FiscCaveatRef
 * @property {string} id
 * @property {string} summary
 * @property {string} href
 */

/**
 * @typedef {Object} FiscDoc
 * @property {string} title
 * @property {string} publisher
 * @property {string} pdf_url
 * @property {string} page_text_base
 * @property {string} records_base
 */

/**
 * @typedef {Object} FiscFigure
 * @property {string} label
 * @property {string} value
 * @property {string} note
 * @property {string} kind
 */

/**
 * One published fiscal year, with every word that belongs to it.
 * @typedef {Object} FiscYear
 * @property {number} year
 * @property {string} label
 * @property {string} stem
 * @property {string} path
 * @property {string} basis
 * @property {string} title
 * @property {FiscFigure} hero
 * @property {FiscFigure[]} figures
 * @property {FiscCaveatRef[]} caveats
 * @property {{facts:number, nodes:number, links:number}} counts
 * @property {FiscStepDoc[]} [steps]  one per declared step, resolved for this year
 * @property {string} chart_title
 */

/**
 * What one rung's document discloses for one year.
 * @typedef {Object} FiscStepDoc
 * @property {FiscCaveatRef[]} caveats
 */

/**
 * @typedef {Object} FiscConfig
 * @property {number} schema_version
 * @property {string} exported_by
 * @property {string} primary
 * @property {FiscYear[]} years
 * @property {FiscMetadata} metadata
 * @property {Record<string, FiscDoc>} docs
 * @property {number[]} [render_tiers]
 * @property {FiscDrillStep[]} [steps]
 * @property {string} [root]
 * @property {Record<string, string>} wording  templates say() fills
 */

/**
 * @typedef {Object} FiscTierCap
 * @property {number} tier
 * @property {number} cap  how many nodes the tier holds before its tail folds
 * @property {string} [tail]  the plural noun the tail is counted in; absent,
 *   the step's own
 */

/**
 * One step of the drill tree, verbatim from export.DrillStep.
 * @typedef {Object} FiscDrillStep
 * @property {string} key
 * @property {string[]} after  the steps this one opens from; "" is the view's
 *   own chart
 * @property {string} [side]  absent for the end links point at, "source" for
 *   the end they come from
 * @property {string} [role]  which nodes at `from` open; absent opens all
 * @property {number} from  the tier whose nodes open
 * @property {string} [projection]  the schedule this step draws; absent means
 *   the previous step's
 * @property {number[]} tiers
 * @property {number[]} [keep]  the flank that stays drawn beside the opened node
 * @property {number[]} [widen]  the tiers columns beyond the window's own buy, in order
 * @property {string} [noun]
 * @property {FiscTierCap[]} [caps]  a tier with none is drawn whole
 * @property {string} back
 * @property {string} tail
 * @property {string} description
 * @property {Record<string,string>} [residual]  endpoints whose flow the drawn
 *   document does not decompose, id to reason
 * @property {string} [residual_grain]  the grain the drawn document does not
 *   split that money by, which names the residual mark; present with `residual`
 * @property {Record<string,FiscGap[]>} [gaps]  opened nodes whose total the
 *   drawn document does not reach, id to the licence for each column it
 *   differs in
 */

/**
 * One column's licence for a gap: the signed cents the chart above carries
 * over what the drawn document accounts for, and why.
 * @typedef {Object} FiscGap
 * @property {number} fiscal_year
 * @property {string} basis
 * @property {number} cents
 * @property {string} reason
 */

/**
 * @typedef {Object} FiscProjection
 * @property {number} schema_version
 * @property {string} projection
 * @property {FiscMetadata} metadata
 * @property {FiscNode[]} nodes
 * @property {FiscLink[]} links
 */

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
 * The projection schema this client draws. A test in internal/export pins
 * this literal to the producer's constant. A newer schema would draw a wrong
 * chart rather than fail, so the page refuses it.
 */
export const SCHEMA_VERSION = 1;

/**
 * The node tiers this page draws, coarsest first. Per view and never a
 * constant here: the spine and the drill-down have different hierarchies, and
 * one's set over the other throws. Absent skips the fold entirely.
 * @type {number[]}
 */
export const RENDER_TIERS = (CONFIG && CONFIG.render_tiers) || [];

/**
 * The steps this page drills through, read by key; empty opens nothing.
 * Not filtered here: the packager validates the tree and a filter would drop
 * a step silently, leaving nodes that will not open and no banner.
 * @type {FiscDrillStep[]}
 */
export const STEPS = (CONFIG && CONFIG.steps) || [];

/**
 * One sentence of the page's wording, filled in. `{name:one|many}` appends the
 * singular or plural word; an unfilled placeholder is left as written.
 * @param {string} key
 * @param {Record<string, string | number>} [vars]
 * @returns {string}
 */
export function say(key, vars) {
  const template = CONFIG.wording[key];
  return template.replace(/\{(\w+)(?::([^|}]*)\|([^}]*))?\}/g, (whole, name, one, many) => {
    if (!vars || !(name in vars)) return whole;
    const v = vars[name];
    return one === undefined ? String(v) : v + " " + (v === 1 ? one : many);
  });
}

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
  if (!decomposable(step, doc).has(id)) return false;
  if (!step.keep || !step.keep.length || !projection) return true;
  const keep = new Set(step.keep);
  const keptTiers = step.tiers.filter((t) => keep.has(t) || t === step.from);
  return filterLinks(projection, id, keptTiers,
    reaching(projection, id, !flankIsLeft(step), keptTiers)).links.length > 0;
}

/**
 * The ids at a step's opened tier that its document decomposes, computed once
 * per (document, step) because nodeClass asks per mark per paint. The fresh
 * half at every declared tier, widened ones included: the step promised them.
 * @type {WeakMap<FiscProjection, Map<string, Set<string>>>}
 */
const decomposed = new WeakMap();

/**
 * @param {FiscDrillStep} step
 * @param {FiscProjection} doc
 * @returns {Set<string>}
 */
export function decomposable(step, doc) {
  let byStep = decomposed.get(doc);
  if (!byStep) {
    byStep = new Map();
    decomposed.set(doc, byStep);
  }
  const have = byStep.get(step.key);
  if (have) return have;
  const out = new Set();
  for (const n of doc.nodes) {
    if (n.tier !== step.from || (step.role && n.role !== step.role)) continue;
    if (freshHalf(step, doc, n.id).links.length) out.add(n.id);
  }
  byStep.set(step.key, out);
  return out;
}

/**
 * The tiers a step opens into, in its column order: every declared tier but
 * the kept flank, widened ones included. Widen fits a viewport and does not
 * change what the document draws.
 * @param {FiscDrillStep} step
 * @returns {number[]}
 */
export function freshTiers(step) {
  const keep = new Set(step.keep || []);
  return step.tiers.filter((t) => !keep.has(t));
}

/**
 * What a step's document draws for one node at the tiers the step declares,
 * unfolded: the reach the chart is drawn with (reaching), asked at every
 * declared column rather than at the columns a budget draws. The answer to
 * whether a node opens (decomposable) and to whether its residual's leaving
 * legs exist (carryResidual), which must not move with the viewport.
 * @param {FiscDrillStep} step
 * @param {FiscProjection} doc
 * @param {string} id
 * @returns {FiscProjection}
 */
export function freshHalf(step, doc, id) {
  const window = Boolean(step.keep && step.keep.length);
  const tiers = freshTiers(step);
  const nearIsSource = window ? flankIsLeft(step) : step.side === SIDE_SOURCE;
  return filterLinks(doc, id, tiers, reaching(doc, id, nearIsSource, tiers));
}

/** DrillStep.Side for a step opening the node its chart's links come FROM. */
export const SIDE_SOURCE = "source";

/**
 * Whether a window step keeps its flank at the LEFT end of its columns: the
 * kept tier is drawn before the opened one. Declared by position, as
 * validateSteps holds it, so the two halves' sides are read off one list.
 * @param {FiscDrillStep} step
 * @returns {boolean}
 */
export function flankIsLeft(step) {
  return step.tiers.indexOf(step.keep[0]) < step.tiers.indexOf(step.from);
}


/** Human wording for link.kind. The JSON's vocabulary is not English. */
export const KIND_LABEL = {
  external: "external money",
  internal_transfer: "transfer between funds",
  internal_service: "internal service charge",
  fund_balance: "fund balance movement",
};

export const NODE_WIDTH = 14;
export const NODE_PADDING = 14;
/** The surface gap that separates stacked ribbons, in px (1px each side). */
export const RIBBON_GAP = 2;
export const CHART_HEIGHT = 820;

/**
 * Room either side of the plot for node labels, in px. Only the two end
 * columns anchor labels outward, so it does not grow with the column count.
 */
export const LABEL_GUTTER = 250;

/**
 * The clear run between one column's rects and the next's, in px. Held fixed
 * as columns are added; 319 makes chartWidth(3) main's 1180px.
 */
export const BAND = 319;

/**
 * How wide a chart of `n` columns is laid out, in px. Labels do not reflow, so
 * the chart is laid out at a fixed width and scaled by the viewBox; the column
 * budget is asked of the viewport (COLUMN_QUERIES) for that reason.
 * @param {number} n
 * @returns {number}
 */
export function chartWidth(n) {
  // d3-sankey divides by (columns - 1); clamp so the width stays finite.
  const columns = Math.max(1, n);
  return 2 * LABEL_GUTTER + BAND * (columns - 1) + NODE_WIDTH * columns;
}

/**
 * The px `100vw` counts that the window does not: body padding plus a classic
 * scrollbar. style.css records it independently as --chart-cushion; no test
 * holds the two together.
 */
export const CHART_CUSHION = 56;

export const money = new Intl.NumberFormat("en-US", {
  style: "currency",
  currency: "USD",
  maximumFractionDigits: 0,
});
export const moneyCompact = new Intl.NumberFormat("en-US", {
  style: "currency",
  currency: "USD",
  notation: "compact",
  maximumFractionDigits: 1,
});

/** @param {number} cents */
export function fmt(cents) {
  return money.format(cents / 100);
}

/** @param {number} cents */
export function fmtShort(cents) {
  return moneyCompact.format(cents / 100);
}

/**
 * A figure with its sign. A minus rather than parentheses, because screen
 * readers do not announce parentheses; U+2212 so it reads as a sign.
 * @param {number} cents
 * @returns {string}
 */
export function fmtSigned(cents) {
  return (cents < 0 ? "\u2212" : "") + fmt(Math.abs(cents));
}

/** @param {number} cents */
export function fmtShortSigned(cents) {
  return (cents < 0 ? "\u2212" : "") + fmtShort(Math.abs(cents));
}

/**
 * @param {string} id
 * @returns {HTMLElement}
 */
export function el(id) {
  const found = document.getElementById(id);
  if (!found) throw new Error("missing element #" + id);
  return found;
}

/**
 * The element with this id, or null. For conditional elements such as the
 * year toggle: el() throws, so `if (!el(id))` is no guard.
 * @param {string} id
 * @returns {HTMLElement | null}
 */
export function maybeEl(id) {
  return document.getElementById(id);
}

/**
 * @param {string} name CSS custom property, including the leading dashes.
 * @returns {string}
 */
export function cssVar(name) {
  return getComputedStyle(document.documentElement).getPropertyValue(name).trim();
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
 * @param {FiscNode | LaidNode} node
 * @returns {boolean}
 */
export function isFundGroup(node) {
  // The role, not an id prefix: column.schema.json holds the role to an enum.
  return node.role === "fund_group";
}

/**
 * The fund group a node belongs to, walking node.parent; "" for none, which
 * callers draw as --muted.
 * @param {FiscNode | LaidNode} node
 * @returns {string}
 */
export function fundGroupOf(node) {
  // Start from groupIndex, not the laid copy: the fold blanks a node's parent
  // when its ancestor is filtered away, which would stop the walk on hop one.
  let at = groupIndex.get(node.id) || node;
  // Bounded against a parent cycle in a malformed document.
  for (let hops = 0; hops < 8; hops++) {
    if (isFundGroup(at)) return at.id;
    if (!at.parent) return "";
    const up = groupIndex.get(at.parent);
    if (!up) return "";
    at = up;
  }
  return "";
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
 * The custom property holding a fund group's hue, or --muted when style.css
 * has none for its slug. Shared so the legend swatch matches the chart.
 * @param {string} id
 * @returns {string}
 */
export function fundColorVar(id) {
  const group = fundGroups().find((g) => g.id === id);
  if (!group) return "--muted";
  const name = "--fund-" + group.slug;
  return cssVar(name) ? name : "--muted";
}

/**
 * Sort key inside a column. A fund group is its own place; anything else is
 * the value-weighted mean place of the fund groups it touches (a barycentre).
 * Supplying a nodeSort is what pins the fund column to the palette's order.
 * The layout tests under site/ measure the alternatives and pin this one.
 * Ties fall to the caller's tie-break on value.
 * @param {LaidNode} node
 * @returns {number}
 */
export function nodeRank(node) {
  if (isFundGroup(node)) return fundGroupPlace(node.id);

  let weight = 0;
  let place = 0;
  for (const l of node.sourceLinks.concat(node.targetLinks)) {
    const other = l.source === node ? l.target : l.source;
    // The neighbour's fund group, not the neighbour: on the drill-down the
    // ends are funds and departments.
    const group = fundGroupOf(other);
    // A node in no fund group is ignored rather than counted as position zero.
    if (!group) continue;
    const at = fundGroupPlace(group);
    place += at * l.value;
    weight += l.value;
  }
  // A node whose links were all dropped as zero-valued has no position to
  // average. It sorts to the top and its own value breaks the tie.
  return weight === 0 ? 0 : place / weight;
}

/**
 * Citations for a set of source documents: the city's PDF at the page, the
 * committed page text, and the fact-store shard for the page.
 *
 * page_text_base is usually relative: do not assume a scheme or compose it
 * with `new URL(base)`. When absolute it points at a blob view, never
 * raw.githubusercontent.com.
 * @param {FiscSource[]} sources
 * @returns {{label:string, href:string}[]}
 */
export function citations(sources) {
  /** @type {{label:string, href:string}[]} */
  const out = [];
  for (const source of sources) {
    const doc = CONFIG.docs[source.doc_id];
    if (!doc) continue;
    for (const page of source.pages) {
      if (doc.pdf_url) {
        out.push({ label: "PDF p" + page, href: doc.pdf_url + "#page=" + page });
      }
      if (doc.page_text_base) {
        const padded = String(page).padStart(4, "0");
        out.push({ label: "extracted p" + page, href: doc.page_text_base + "p" + padded + ".txt" });
      }
      if (doc.records_base) {
        const padded = String(page).padStart(4, "0");
        out.push({ label: "records p" + page, href: doc.records_base + "p" + padded + ".jsonl" });
      }
    }
  }
  return out;
}

/**
 * @param {string} tag
 * @param {string} [className]
 * @param {string} [text]
 * @returns {HTMLElement}
 */
export function h(tag, className, text) {
  const node = document.createElement(tag);
  if (className) node.className = className;
  // Labels come out of the data file; they are inserted as text, never as
  // markup.
  if (text !== undefined) node.textContent = text;
  return node;
}

/**
 * @param {string} label
 * @param {string} href
 * @returns {HTMLAnchorElement}
 */
export function link(label, href) {
  const a = document.createElement("a");
  a.textContent = label;
  a.href = href;
  a.rel = "noopener";
  return a;
}

/* ------------------------------------------------------------------ *
 * State
 * ------------------------------------------------------------------ */

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
export const OFFERED_COLUMNS = STEPS.reduce(
  (most, s) => Math.max(most, (s.tiers || []).length),
  NARROW_COLUMNS,
);

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
 * Rebuild doc\u001fpage keys as FiscSource[] in the packager's order:
 * documents ascending, pages ascending within each, each once. Any other order
 * would render one page as a different citation on the spine and in a drill.
 * @param {Set<string>|undefined} keys
 * @returns {FiscSource[]}
 */
export function regroupLocators(keys) {
  /** @type {Map<string, number[]>} */
  const byDoc = new Map();
  for (const k of keys || []) {
    const cut = k.indexOf("\u001f");
    const doc = k.slice(0, cut);
    const page = Number(k.slice(cut + 1));
    const pages = byDoc.get(doc);
    if (pages) pages.push(page);
    else byDoc.set(doc, [page]);
  }
  return Array.from(byDoc.keys()).sort().map((doc) => ({
    doc_id: doc,
    pages: (byDoc.get(doc) || []).sort((a, b) => a - b),
  }));
}

/**
 * The id a node folds to under a tier set, or "" when it has no drawn
 * ancestor. The non-throwing half of foldDocument's first loop: to filterLinks
 * an unplaceable end is an ordinary answer, not a fault.
 * @param {Map<string,FiscNode>} byID
 * @param {FiscNode} n
 * @param {Set<number>} drawn
 * @returns {string}
 */
export function foldTarget(byID, n, drawn) {
  let at = n;
  for (let hops = 0; !drawn.has(at.tier); hops++) {
    const up = at.parent ? byID.get(at.parent) : undefined;
    if (!up || hops > 8) return "";
    at = up;
  }
  return at.id;
}

/**
 * The filter every rung uses: a ribbon of `doc` is drawn when its near end --
 * the target, or the source where the opened node is the end links come FROM
 * -- is the opened node or one of its parts by parent chain, AND its folded
 * ends run forward in the drawn column order. Membership alone admits cycles:
 * a fund window's flank parents a department's rows under the fund, so
 * department -> row folds to department -> fund, which d3-sankey refuses as a
 * circular link. The rule is the hierarchy's, not the ribbons': a walk along
 * ribbons would miss transfers/in, which no ribbon touches and whose payers
 * are its children.
 *
 * An end that folds to nothing is passed through; placing it is scoped()'s
 * question.
 *
 * @param {FiscProjection} doc
 * @param {string} opened
 * @param {boolean} nearIsSource
 * @param {number[]} tiers  the columns drawn, in order
 * @returns {(src: FiscNode, dst: FiscNode, byID: Map<string,FiscNode>, drawn: Set<number>) => boolean}
 */
export function reaching(doc, opened, nearIsSource, tiers) {
  const inside = withinNode(doc, opened);
  const at = new Map(tiers.map((t, i) => [t, i]));
  return (src, dst, byID, drawn) => {
    if (!inside.has(nearIsSource ? src.id : dst.id)) return false;
    const from = foldTarget(byID, src, drawn);
    const to = foldTarget(byID, dst, drawn);
    if (from === "" || to === "") return true;
    const a = at.get((byID.get(from) || src).tier);
    const b = at.get((byID.get(to) || dst).tier);
    return a !== undefined && b !== undefined && a < b;
  };
}

/**
 * The links `holds` admits whose ends this tier set can place, and the nodes
 * those links need. Refuses nothing: an id nothing flows for is an empty
 * answer, which decomposable reads as "does not open" and windowFor refuses.
 *
 * @param {FiscProjection} doc
 * @param {string} id
 * @param {number[]} tiers
 * @param {(src: FiscNode, dst: FiscNode, byID: Map<string,FiscNode>, drawn: Set<number>) => boolean} holds
 * @returns {FiscProjection}
 */
export function filterLinks(doc, id, tiers, holds) {
  const byID = new Map(doc.nodes.map((n) => [n.id, n]));
  const drawn = new Set(tiers);
  const placeable = scoped(doc, tiers);
  const links = doc.links.filter((l) => {
    const src = byID.get(l.source);
    const dst = byID.get(l.target);
    if (!src || !dst) return false;
    if (!holds(src, dst, byID, drawn)) return false;
    return placeable(src) && placeable(dst);
  });

  // Only the nodes those links touch, up to the drawn tiers: any other node
  // would make foldDocument refuse the document.
  const keep = new Set();
  for (const l of links) {
    for (const end of [l.source, l.target]) {
      let at = byID.get(end);
      for (let hops = 0; at && hops < 9; hops++) {
        keep.add(at.id);
        if (drawn.has(at.tier)) break;
        at = at.parent ? byID.get(at.parent) : undefined;
      }
    }
  }
  return Object.assign({}, doc, {
    nodes: doc.nodes.filter((n) => keep.has(n.id)),
    links: links,
  });
}

/**
 * Whether a link end has a column in this tier set. A broken parent chain is
 * Go's to refuse at the write (node-hierarchy-well-formed), so a node with no
 * drawn ancestor is simply not placed.
 *
 * @param {FiscProjection} doc
 * @param {number[]} tiers
 * @returns {(n: FiscNode) => boolean}
 */
export function scoped(doc, tiers) {
  const byID = new Map(doc.nodes.map((n) => [n.id, n]));
  const drawn = new Set(tiers);
  return (n) => foldTarget(byID, n, drawn) !== "";
}

/**
 * The subtree of one node: its id and every id whose parent chain reaches it.
 * Walked upward per node rather than downward from the root, because a node
 * names its parent and nothing names its children.
 * @param {FiscProjection} doc
 * @param {string} id
 * @returns {Set<string>}
 */
export function withinNode(doc, id) {
  const byID = new Map(doc.nodes.map((n) => [n.id, n]));
  const inside = new Set();
  for (const n of doc.nodes) {
    let at = n;
    for (let hops = 0; at && hops < 9; hops++) {
      if (at.id === id) {
        inside.add(n.id);
        break;
      }
      at = at.parent ? byID.get(at.parent) : undefined;
    }
  }
  return inside;
}

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
  const tiers = rung.step.tiers;
  const widen = rung.step.widen || [];
  const drop = new Set(rung.dropped || []);
  // Re-asked after every drop: a tier already dropped as empty is an entry of
  // this same order, and a fixed shortfall would drop it twice.
  for (let k = widen.length - 1; k >= 0 && tiers.length - drop.size > at; k--) {
    drop.add(widen[k]);
  }
  return drop.size ? tiers.filter((t) => !drop.has(t)) : tiers;
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
  return (rung.step.caps || []).some((c) => c.tier === d.tier);
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
    tablePointer = lastSentence(baseDescription);
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
 * The last sentence of a server-rendered description, with the template's own
 * line wrapping collapsed.
 *
 * Returns "" for a description of one sentence.
 *
 * @param {string} s
 * @returns {string}
 */
export function lastSentence(s) {
  // ANY TERMINATOR: Go accepts ".", "!" and "?" to close a description, and
  // refuses an unterminated one.
  const parts = String(s).replace(/\s+/g, " ").trim().split(/[.!?]\s+/);
  return parts.length < 2 ? "" : parts[parts.length - 1].trim();
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

/**
 * A list of phrases as English: "a", "a or b", "a, b or c" (no serial comma).
 * @param {string[]} parts
 * @returns {string}
 */
export function joinOr(parts) {
  if (parts.length < 3) return parts.join(" or ");
  return parts.slice(0, -1).join(", ") + " or " + parts[parts.length - 1];
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
  const cap = (rung.step.caps || []).find((c) => c.tier === tier);
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
  if (!rung) {
    return markContra(foldDocument(doc));
  }
  const step = rung.step;
  const drawn = (step.keep && step.keep.length)
    ? windowFor(rung.chart, doc, rung)
    : sideOf(doc, rung, activeTiers(), step.side === SIDE_SOURCE);
  // THE MARKS COME AFTER THE CAP AND THE FOLD, which must not touch them, in
  // this order: the amount a node prints net of reductions is read off the
  // fresh ribbons before any mark is added, then the residual, then the gap
  // over what the residual left, then markContra over what the fold left
  // negative.
  const from = docAt(drilled.length - 1);
  return markContra(markGap(carryResidual(markAmounts(drawn, rung), from, rung), from, rung));
}

/**
 * Drops a widened column the drawn document left empty, so the chart is laid
 * out at the columns it has.
 *
 * d3-sankey takes its column count from topology, so an empty declared column
 * would stretch the others rather than narrow the chart. DROPPED AND NOT
 * REFUSED, and only a widened column: the step promised the others.
 *
 * @param {FiscProjection} drawn
 * @returns {boolean} whether anything was dropped
 */
export function dropEmptyColumns(drawn) {
  const rung = drilled.length ? drilled[drilled.length - 1] : null;
  const widen = rung && rung.step.widen ? rung.step.widen : [];
  if (!widen.length) return false;
  const has = new Set(drawn.nodes.map((n) => n.tier));
  const gone = activeTiers().filter((t) => widen.indexOf(t) >= 0 && !has.has(t));
  if (!gone.length) return false;
  rung.dropped = (rung.dropped || []).concat(gone);
  return true;
}

/**
 * One filtered, capped and folded chart of a node: a whole rung on a step that
 * keeps no flank, and one half of a window on a step that does.
 *
 * THE ORDER IS filter, cap, fold, AND IT IS NOT INTERCHANGEABLE: the cap must
 * rank sizes inside the opened node, and the fold is what merges the cap's
 * parallel ribbons and unions their citations.
 *
 * @param {FiscProjection} doc
 * @param {Rung} rung
 * @param {number[]} tiers the columns this chart draws, in order
 * @param {boolean} nearIsSource whether the opened node is the end the drawn
 *   ribbons come FROM
 * @returns {FiscProjection}
 */
export function sideOf(doc, rung, tiers, nearIsSource) {
  const step = rung.step;
  let shaped = filterLinks(doc, rung.id, tiers, reaching(doc, rung.id, nearIsSource, tiers));
  const inside = withinNode(doc, rung.id);
  // CAPS RUN IN THE TIER ORDER THIS CHART DRAWS: folding a coarse node
  // removes descendants a finer cap would otherwise rank.
  //
  // THE TAIL'S PARENT IS THE OPENED NODE ONLY WHEN THE WHOLE COLUMN IS INSIDE
  // IT, asked of the document; otherwise "" (--muted).
  /** @type {Map<number, string>} */
  const parentOf = new Map();
  for (const tier of tiers) {
    const cap = (step.caps || []).find((c) => c.tier === tier);
    // An expanded tier skips the cap and only the cap.
    if (!cap || (rung.expanded && rung.expanded.has(tier))) continue;
    const column = shaped.nodes.filter((n) => n.tier === tier);
    const parent = column.every((n) => inside.has(n.id)) ? rung.id : "";
    parentOf.set(tier, parent);
    shaped = capColumn(shaped, tier, cap.cap, parent, cap.tail || step.tail);
  }
  const drawn = foldDocument(shaped, tiers);

  // EVERY AGGREGATE'S PARENT IS PUT BACK AFTER THE FOLD, which blanks it
  // because the opened node was filtered away. Its note is finished here with
  // the figure only the folded document knows.
  return Object.assign({}, drawn, {
    nodes: drawn.nodes.map((n) => (isAggregate(n.id)
      ? Object.assign({}, n, {
        parent: parentOf.get(n.tier) || "",
        source_note: n.source_note + ", together " + fmt(tailFigure(drawn, n.id)) + ".",
      })
      : n)),
  });
}

/**
 * What a folded tail carries, at the height d3-sankey draws it: the larger of
 * what arrives and what leaves, each ribbon at its magnitude as markContra
 * draws a reduction.
 *
 * READ OFF THE FOLDED DOCUMENT, because the fold nets, re-points and drops
 * ribbons capColumn produced.
 *
 * @param {FiscProjection} doc a folded document
 * @param {string} id the tail's id
 * @returns {number} cents
 */
export function tailFigure(doc, id) {
  let arriving = 0;
  let leaving = 0;
  for (const l of doc.links) {
    if (l.target === id) arriving += Math.abs(l.value_cents);
    if (l.source === id) leaving += Math.abs(l.value_cents);
  }
  return Math.max(arriving, leaving);
}

/**
 * A window on the node the reader clicked: the flank they came from on one
 * side, the step document's decomposition on the other, and that node between
 * them.
 *
 * TWO DOCUMENTS, SO TWO HALVES, spliced on the centre: folding them together
 * would hand foldDocument two parent chains. Which side is which is read off
 * the step: the kept tiers are the flank, the opened tier the centre, and the
 * rest is what the node opens into.
 *
 * THE KEPT FLANK COMES OFF THE CHART ON SCREEN, NOT OFF A FILE: its nodes need
 * not exist in the step's document at all. The centre's record is the
 * on-screen chart's too, and is not marked carried.
 *
 * @param {FiscProjection} onScreen the drawn chart the rung was opened from
 * @param {FiscProjection} stepDoc the document the step draws
 * @param {Rung} rung
 * @returns {FiscProjection}
 */
export function windowFor(onScreen, stepDoc, rung) {
  const step = rung.step;
  // The columns on screen, not the columns declared.
  const tiers = activeTiers();
  const keep = new Set(step.keep);
  // THE CENTRE IS THE ONLY COLUMN BOTH HALVES HOLD, so a ribbon of one cannot
  // land in a column of the other. With the flank on the left the opened node
  // is the TARGET of the kept half and the SOURCE of the fresh one; on the
  // right, the reverse.
  const keptLeft = flankIsLeft(step);
  const keptTiers = tiers.filter((t) => keep.has(t) || t === step.from);
  const freshOnScreen = tiers.filter((t) => !keep.has(t));
  const kept = sideOf(onScreen, rung, keptTiers, !keptLeft);
  const fresh = sideOf(stepDoc, rung, freshOnScreen, keptLeft);
  // THE KEPT CENTRE MUST HOLD THE OPENED NODE: a window whose flank sends
  // nothing into it is refused rather than drawn as its fresh half alone.
  if (!kept.nodes.some((n) => n.id === rung.id)) {
    throw new Error("cannot draw " + stepDoc.projection + ": the chart on screen sends nothing " +
      "between tiers " + keptTiers.join(", ") + " and " + rung.id + ", so there is no flank to keep");
  }

  // carried_from IS SET WHERE ABSENT AND NEVER CLEARED: a node keeps the stem
  // whose figure and caveats it carries, however many rungs down.
  const stem = onScreen.projection || "";
  /** @type {Set<string>} */
  const have = new Set();
  /** @type {FiscNode[]} */
  const nodes = [];
  for (const n of kept.nodes) {
    have.add(n.id);
    nodes.push(n.id === rung.id || n.carried_from
      ? n
      : Object.assign({}, n, { carried_from: stem }));
  }
  for (const n of fresh.nodes) {
    if (have.has(n.id)) continue;
    have.add(n.id);
    nodes.push(n);
  }
  // The spliced document is the step document's; the kept flank is a guest.
  return Object.assign({}, fresh, {
    nodes: nodes,
    links: kept.links.concat(fresh.links),
  });
}

/**
 * Whether a residual's leaving leg is drawn at these columns: a leaving
 * endpoint stands at the step's last declared tier (carryResidual).
 *
 * @param {FiscDrillStep} step
 * @param {number[]} tiers the columns the chart draws
 * @returns {boolean}
 */
export function leavingLegDrawn(step, tiers) {
  return tiers.includes(step.tiers[step.tiers.length - 1]);
}

/**
 * The region a mark's arriving ribbons hang into, for a mark printed net of
 * reductions drawn forward, and null for every other mark.
 *
 * The box is the published net figure, so the gross arriving stack overhangs
 * it; drawing the overhang avoids a box sized to a figure no page prints. It
 * is pixels and carries no cents.
 *
 * @param {LaidNode} d
 * @returns {{y:number, height:number} | null}
 */
export function contraBand(d) {
  if (!d.targetLinks.some((l) => l.contra)) return null;
  const arriving = d.targetLinks.reduce((sum, l) => sum + l.width, 0);
  const excess = arriving - (d.y1 - d.y0);
  // Half a pixel, not zero: no hairline band for reductions that round away.
  return excess > 0.5 ? { y: d.y1, height: excess } : null;
}

/**
 * Draws the opened node at the figure it prints where its drawn ribbons do
 * not add up to it: a node a schedule prints a reduction under, the reduction
 * drawn forward at its magnitude by markContra. The figure is the signed sum
 * of the ribbons arriving at it from the columns the step opens into --
 * the same one the citywide chart labels the node with -- and is read before
 * any mark is added or any sign is flipped.
 *
 * d3-sankey would otherwise size the node at the gross its forward-drawn
 * ribbons add to, a figure no page prints. Setting the value here means every
 * surface reads one figure. A sum at or below zero would rescale its whole
 * column silently, so it is refused.
 *
 * @param {FiscProjection} drawn  shaped and folded, before any mark
 * @param {Rung} rung
 * @returns {FiscProjection} drawn itself when no arriving ribbon is a reduction
 */
export function markAmounts(drawn, rung) {
  const keep = new Set(rung.step.keep || []);
  const tierOf = new Map(drawn.nodes.map((n) => [n.id, n.tier]));
  let sum = 0;
  let contra = false;
  for (const l of drawn.links) {
    if (l.target !== rung.id || keep.has(tierOf.get(l.source))) continue;
    sum += l.value_cents;
    if (l.value_cents < 0) contra = true;
  }
  if (!contra) return drawn;
  if (sum <= 0) {
    throw new Error("cannot draw " + drawn.projection + ": the ribbons into " + rung.id +
      " net to " + sum + " cents, which is no height to draw it at");
  }
  return Object.assign({}, drawn, {
    nodes: drawn.nodes.map((n) => (n.id === rung.id ? Object.assign({}, n, { fixedValue: sum }) : n)),
  });
}

/**
 * Draws every ribbon the schedule prints as a reduction forward, at its
 * magnitude, carrying the sentence the document put on it.
 *
 * A RIBBON IS NEVER REVERSED: a backward ribbon adds a column d3-sankey's
 * layering pass throws on. The contra words are the document's, from Go.
 *
 * The two arms here are about the drawn chart only: a fold sums ribbons, so a
 * merged ribbon can go negative with no printed reduction (named for what it
 * is) or positive despite one (loses the sentence).
 *
 * @param {FiscProjection} drawn  shaped and folded
 * @returns {FiscProjection} drawn itself when nothing in it is negative
 */
export function markContra(drawn) {
  if (!drawn.links.some((l) => l.value_cents < 0 || l.contra)) return drawn;
  return Object.assign({}, drawn, {
    links: drawn.links.map((l) => {
      if (l.value_cents < 0) {
        return Object.assign({}, l, {
          value_cents: -l.value_cents,
          contra: l.contra || "printed rows netting to a reduction",
        });
      }
      return l.contra ? Object.assign({}, l, { contra: "" }) : l;
    }),
  });
}

/**
 * Whether every ribbon on a laid node is a contra one: a line the schedule
 * prints as a reduction, whose own figure is therefore negative.
 * @param {LaidNode} d
 * @returns {boolean}
 */
export function isContraNode(d) {
  const links = d.sourceLinks.concat(d.targetLinks);
  return links.length > 0 && links.every((l) => Boolean(l.contra));
}

/**
 * Whether every ribbon on a laid node is a partition one, so the mark's whole
 * figure is a cross-tab total rather than money that moved through it. All or
 * nothing, as isContraNode is.
 * @param {LaidNode} d
 * @returns {boolean}
 */
export function isPartitionNode(d) {
  const links = d.sourceLinks.concat(d.targetLinks);
  return links.length > 0 && links.every((l) => Boolean(l.partition));
}

/**
 * The figure a laid mark prints: negative for a contra ribbon and for a line
 * whose every ribbon is one, the larger of a residual's two sides, and d3's
 * value otherwise. A residual's is carryResidual's sum even where the width
 * holds legs back.
 * @param {LaidLink | LaidNode} d
 * @returns {number}
 */
export function markCents(d) {
  if (isLink(d)) {
    const l = /** @type {LaidLink} */ (d);
    return l.contra ? -l.value_cents : l.value_cents;
  }
  const n = /** @type {LaidNode} */ (d);
  if (isResidual(n.id)) return Math.max(n.in_cents || 0, n.out_cents || 0);
  return isContraNode(n) ? -n.value : n.value;
}

/**
 * A residual's two figures, where money both enters and leaves it; "" for
 * every other mark.
 * @param {LaidNode} d
 * @returns {string}
 */
export function residualFlows(d) {
  if (!isResidual(d.id) || !d.in_cents || !d.out_cents) return "";
  return fmt(d.in_cents) + " in, " + fmt(d.out_cents) + " out";
}

/**
 * How much of a mark's figure is printed as reductions, for a node with contra
 * ribbons among others, and "" for every other node -- a contra line's own
 * mark included, whose figure is the reduction and is already signed.
 * @param {LaidNode} d
 * @returns {string}
 */
export function contraNote(d) {
  if (isContraNode(d)) return "";
  const arriving = d.targetLinks.reduce((sum, l) => sum + l.value, 0);
  const leaving = d.sourceLinks.reduce((sum, l) => sum + l.value, 0);
  const side = arriving >= leaving ? d.targetLinks : d.sourceLinks;
  const reduced = side.filter((l) => l.contra).reduce((sum, l) => sum + l.value_cents, 0);
  if (!reduced) return "";
  return "\u25c7 our reading: " + fmt(reduced) + " of this category is printed as reductions, " +
    "drawn here at their size";
}

/**
 * What a partition ribbon is: the one sentence every mark showing it uses.
 * The direction drawn is not one the city printed; the ribbon is never
 * reversed, and the class and this sentence carry what it means.
 */
export const PARTITION_NOTE = "a cross-tab: one printed table read along a second axis, " +
  "not money moving in the direction drawn";

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
 * The prefix every capped tail's id carries.
 */
export const AGGREGATE_PREFIX = "aggregate/tail/";

/**
 * The id of the node one tier's capped tail is folded into. Per tier, because
 * a step can cap two columns and d3-sankey and the fold both key by id.
 * @param {number} tier
 * @returns {string}
 */
export function aggregateID(tier) {
  return AGGREGATE_PREFIX + tier;
}

/**
 * Whether an id names a capped tail of any tier.
 * @param {string} id
 * @returns {boolean}
 */
export function isAggregate(id) {
  return id.startsWith(AGGREGATE_PREFIX);
}

/**
 * The prefix the residual node's id carries, followed by the opened node's id.
 */
export const RESIDUAL_PREFIX = "residual/";

/**
 * The id of the node an opened node's undecomposed flows are carried onto.
 * @param {string} opened
 * @returns {string}
 */
export function residualID(opened) {
  return RESIDUAL_PREFIX + opened;
}

/**
 * Whether an id names a residual node.
 * @param {string} id
 * @returns {boolean}
 */
export function isResidual(id) {
  return id.startsWith(RESIDUAL_PREFIX);
}

/**
 * The prefix a gap node's id carries, followed by the opened node's id. Not
 * the residual's: a residual is carried, a gap is derived.
 */
export const GAP_PREFIX = "gap/";

/**
 * Whether an id names a gap node.
 * @param {string} id
 * @returns {boolean}
 */
export function isGap(id) {
  return id.startsWith(GAP_PREFIX);
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
 * Adds to a rung's drawn document the flows the chart it was opened from
 * prints for the opened node and the document it draws does not decompose,
 * copied verbatim onto one derived node beside the node's parts.
 *
 * CARRIED, NOT COMPUTED: every link added is a link of the chart above with
 * only its group end re-pointed, so its figure and provenance are untouched.
 * The endpoints are the step's declaration, whole or nothing each, in sorted
 * id order: an endpoint's inflow is carried only where the drawn document
 * carries nothing from it into the opened node, its outflow only where the
 * step's document decomposes the node at the tiers the step declares and
 * carries nothing from inside to it; where both, the outflow's placement
 * wins. An endpoint's ribbons come off
 * the chart on screen where it draws any, else off the file, never both: the
 * window's flank is already on screen, and taking both doubled transfers/in
 * against what p0067 prints. The mark's two figures are the sums of the
 * ribbons carried each way, the leaving ones counted whether or not this width
 * draws their column, so the figures do not move with the viewport. Its
 * inflow and outflow differ by construction, and d3-sankey shows that on the
 * mark.
 *
 * Only across a document switch: a step that draws the document before it has
 * no second grain to be residual by.
 *
 * @param {FiscProjection} drawn  the rung's document, shaped and folded
 * @param {FiscProjection | null} from  the document of the chart the rung was
 *   opened from
 * @param {Rung} rung
 * @returns {FiscProjection}
 */
export function carryResidual(drawn, from, rung) {
  const step = rung.step;
  const residual = step.residual;
  if (!residual || !from || !step.projection) return drawn;
  const opened = rung.id;
  const doc = rung.doc;
  const id = residualID(opened);
  const inside = withinNode(doc, opened);
  if (!inside.has(opened)) {
    throw new Error("cannot draw " + doc.projection + ": it does not carry " + opened +
      ", so nothing can be residual beside its parts");
  }
  // DECOMPOSED IS THE STEP'S DECLARATION, NOT THIS WIDTH: whether the document
  // sends anything out of a part of the opened node at a tier the step
  // declares, each part read at the column it folds to there. Asked of the
  // drawn window instead, a budget that drops the parts' outward column would
  // drop the leaving legs from the mark's figure as well as from the chart.
  const declared = freshHalf(step, doc, opened);
  const byDocID = new Map(doc.nodes.map((n) => [n.id, n]));
  const declaredTiers = new Set(freshTiers(step));
  const decomposed = declared.links.some((l) => {
    const part = foldTarget(byDocID, /** @type {FiscNode} */ (byDocID.get(l.source)), declaredTiers);
    return part !== "" && part !== opened && inside.has(part);
  });
  const carriesFrom = (/** @type {string} */ e) => doc.links.some((l) => l.source === e && inside.has(l.target));
  const carriesTo = (/** @type {string} */ e) => doc.links.some((l) => l.target === e && inside.has(l.source));
  /** @type {FiscLink[]} */
  const links = [];
  /**
   * The chart's own copy of each link re-pointed below, dropped: a window's
   * flank already draws them, and keeping both would double-count.
   * @type {Set<FiscLink>}
   */
  const spliced = new Set();
  /**
   * One endpoint's links off the chart above: this rung's own where it draws
   * them, else the file's. Never both.
   * @param {(l: FiscLink) => boolean} want
   * @returns {FiscLink[]}
   */
  const above = (want) => {
    const here = drawn.links.filter(want);
    return here.length ? here : from.links.filter(want);
  };
  /** @type {Map<string, boolean>} endpoint id to whether its flow arrives */
  const ends = new Map();
  // A LEAVING LEG IS DROPPED WHERE THE BUDGET DROPS ITS COLUMN, as every other
  // ribbon of that column is; placed in the last column the chart has, it
  // would draw a ribbon of no length.
  const leaves = leavingLegDrawn(step, activeTiers());
  // Counted, not inferred from the endpoints: a mark with no leaving flow
  // holds nothing back and its note must not say otherwise.
  let withheld = 0;
  let inCents = 0;
  let outCents = 0;
  for (const e of Object.keys(residual).sort()) {
    if (!String(residual[e] || "").trim()) {
      throw new Error("cannot draw " + doc.projection + ": " + e +
        " is carried onto the residual mark with no reason declared for it");
    }
    if (!carriesFrom(e)) {
      for (const l of above((l) => l.source === e && l.target === opened)) {
        links.push(Object.assign({}, l, { target: id }));
        inCents += l.value_cents;
        ends.set(e, true);
        spliced.add(l);
      }
    }
    if (!decomposed || carriesTo(e)) continue;
    const leaving = above((l) => l.source === opened && l.target === e);
    for (const l of leaving) outCents += l.value_cents;
    if (!leaves) {
      withheld += leaving.length;
      continue;
    }
    for (const l of leaving) {
      links.push(Object.assign({}, l, { source: id }));
      ends.set(e, false);
      spliced.add(l);
    }
  }

  // A MARK NONE OF WHOSE FLOWS THIS WIDTH DRAWS IS NOT DRAWN: every flow of an
  // enterprise or special revenue residual leaves through the widened column,
  // and at three columns it would be a box of no height citing nothing.
  if (!links.length) return drawn;

  // THE MARK STANDS AT THE SHALLOWEST DECLARED TIER OF ANY PART OF THE OPENED
  // NODE, read off the step's unfolded document; a node with no part at a
  // declared tier has nowhere to stand it.
  let tier = -1;
  for (const n of doc.nodes) {
    if (n.id !== opened || !inside.has(n.id)) {
      if (inside.has(n.id) && step.tiers.includes(n.tier) && (tier < 0 || n.tier < tier)) tier = n.tier;
    }
  }
  if (tier < 0) {
    throw new Error("cannot draw " + doc.projection + ": " + opened +
      " has no part at a tier this step draws to stand the residual beside");
  }

  // ENDPOINTS STAND AT THE FIRST DRAWN TIER WHEN THEIR FLOW ARRIVES AND THE
  // LAST WHEN IT LEAVES -- drawn, not declared: an undrawn declared tier is
  // clamped to the first column and the ribbon runs backwards. Filtered in the
  // step's own order, which is a column order and not a sorted set.
  const tiers = step.tiers.filter((t) => drawn.nodes.some((n) => n.tier === t));
  const have = new Set(drawn.nodes.map((n) => n.id));
  const fromByID = new Map(from.nodes.map((n) => [n.id, n]));
  /** @type {FiscNode[]} */
  const added = [];
  for (const [e, arrives] of ends) {
    const node = fromByID.get(e);
    if (!node || have.has(e)) continue;
    added.push(Object.assign({}, node, {
      tier: arrives ? tiers[0] : tiers[tiers.length - 1], parent: "",
      // Its caveats belong to the chart it was carried from; caveatsFor,
      // caveatHref and carriedSource resolve this stem against the stack.
      carried_from: from.projection || "",
    }));
  }

  /** @type {Map<string, Set<number>>} */
  const cited = new Map();
  for (const l of links) {
    for (const s of l.locators || []) {
      const pages = cited.get(s.doc_id) || new Set();
      for (const p of s.pages) pages.add(p);
      cited.set(s.doc_id, pages);
    }
  }
  const where = Array.from(cited.keys()).sort().map((docID) => {
    const d = CONFIG && CONFIG.docs ? CONFIG.docs[docID] : undefined;
    const pages = Array.from(cited.get(docID) || []).sort((a, b) => a - b);
    return (d && d.title ? d.title : docID) + " " +
      (pages.length === 1 ? "p." : "pp.") + pages.join(", ");
  }).join("; ");

  // The words: the label names the grain, the rationale carries the step's
  // reason for every endpoint in sorted order, whichever legs this width draws.
  const label = (/** @type {string} */ n) => {
    const own = doc.nodes.find((g) => g.id === n && g.label);
    if (own) return own.label;
    const theirs = from.nodes.find((g) => g.id === n && g.label);
    return theirs ? theirs.label : n;
  };
  const grain = step.residual_grain || "";
  const reasons = Object.keys(residual).sort().filter((e) => ends.has(e) || residualTouches(e))
    .map((e) => label(e) + ": " + residual[e] + ".");
  /**
   * Whether an endpoint's flow was carried in either direction, drawn or
   * withheld: only those are named in the rationale.
   * @param {string} e
   */
  function residualTouches(e) {
    return (!carriesFrom(e) && above((l) => l.source === e && l.target === opened).length > 0) ||
      (decomposed && !carriesTo(e) && above((l) => l.source === opened && l.target === e).length > 0);
  }
  const node = {
    id: id,
    label: "Not split by " + grain + " here",
    tier: tier,
    parent: opened,
    constraint_tier: "",
    role: "residual",
    derived: true,
    in_cents: inCents,
    out_cents: outCents,
    // No plural is formed from the grain.
    rationale: "Money the chart above prints for " + label(opened) + " as a whole and " +
      "that the schedule this chart is drawn from does not split by " + grain + ", so " +
      "no " + grain + " here receives or pays it. It is drawn beside the opened node's " +
      "parts rather than attributed to one of them, and what flows in and what flows " +
      "out need not balance: the difference is what that schedule does not break " +
      "down. " + reasons.join(" "),
    // It cites the ribbons carried at this width.
    source_note: "Carried, not computed: " + links.length + " flow" + (links.length === 1 ? "" : "s") +
      " of the chart above with figures and citations unchanged — " + where + "." +
      (withheld ? " " + (withheld === 1 ? "The flow" : "The " + withheld + " flows") + " leaving it " +
        (withheld === 1 ? "is" : "are") + " drawn where there is room for a further column." : ""),
  };
  return Object.assign({}, drawn, {
    nodes: drawn.nodes.concat(added, [node]),
    links: drawn.links.filter((l) => !spliced.has(l)).concat(links),
  });
}

/**
 * States as a mark of its own the difference between what the chart above
 * sends into the opened node and what the document this rung draws breaks that
 * node into, where the step licenses exactly that difference in this column.
 *
 * Without it d3-sankey absorbs the difference into node height with no ribbon
 * against it, and the chart looks balanced. A gap has no published link to
 * copy, so unlike carryResidual's mark this one is derived. The sums are
 * signed, reductions negative, as they stand before markContra; after that
 * pass a centre would read short by twice the reductions. Too little leaving
 * stands the mark at the last declared tier, too little arriving at the
 * first. It carries no kind: the difference crosses no printed boundary.
 *
 * FAILS CLOSED: a difference no licence accounts for, one licensed in another
 * column or at another figure, and a licence for a centre that balances are
 * each a throw, so the two documents drifting apart is a banner and not a
 * chart. The licence's cents is structure's, shipped on the step.
 *
 * @param {FiscProjection} drawn  the rung's chart, shaped, folded and spliced
 * @param {FiscProjection | null} from  the document of the chart the rung was
 *   opened from, which the mark cites alongside the drawn one
 * @param {Rung} rung
 * @returns {FiscProjection} drawn itself where the step declares no gap at all,
 *   or the node it opened balances unlicensed
 */
export function markGap(drawn, from, rung) {
  const step = rung.step;
  const gaps = step.gaps;
  if (!gaps || typeof gaps !== "object") return drawn;
  const opened = rung.id;
  if (!drawn.nodes.some((n) => n.id === opened)) {
    throw new Error("cannot draw " + drawn.projection + ": " + opened + " is not a mark of the " +
      "drawn chart, so the gap this step declares has nothing to be stated against");
  }
  let into = 0;
  let outOf = 0;
  for (const l of drawn.links) {
    if (l.target === opened) into += l.value_cents;
    if (l.source === opened) outOf += l.value_cents;
  }
  const gap = into - outOf;
  const meta = drawn.metadata || /** @type {any} */ ({});
  const where = "FY" + meta.fiscal_year + " " + meta.basis;
  const licence = (gaps[opened] || [])
    .filter((g) => g.fiscal_year === meta.fiscal_year && g.basis === meta.basis && g.reason).pop();
  if (gap === 0 && !licence) return drawn;
  if (gap === 0) {
    throw new Error("cannot draw " + drawn.projection + ": the step declares a gap of " + licence.cents +
      " cents on " + opened + " in " + where + " and the chart balances there");
  }
  if (!licence) {
    throw new Error("cannot draw " + drawn.projection + ": the chart above sends " + into + " into " +
      opened + " and this one draws " + outOf + " of it, a difference of " + Math.abs(gap) +
      " cents that no declaration on this step accounts for in " + where +
      "; the two documents have drifted apart");
  }
  if (licence.cents !== gap) {
    throw new Error("cannot draw " + drawn.projection + ": the step declares a gap of " + licence.cents +
      " cents on " + opened + " in " + where + " and the charts differ there by " + gap);
  }
  const tiers = step.tiers;
  const locators = citedAround(opened, from ? [from, rung.doc] : [rung.doc]);
  if (!locators.length) {
    throw new Error("cannot draw " + drawn.projection + ": no ribbon touching " + opened +
      " cites a page, so the gap on it could cite none");
  }
  const centreNode = drawn.nodes.find((n) => n.id === opened && n.label);
  const centre = centreNode ? centreNode.label : opened;
  const column = meta.fiscal_year_label + " " + meta.basis;
  const lead = gap > 0
    ? "In " + column + ", the chart above puts " + fmt(into) + " through " + centre +
      " and the schedule this chart is drawn from accounts for " + fmt(outOf) + " of it, " +
      fmt(gap) + " less."
    : "In " + column + ", the schedule this chart is drawn from accounts for " + fmt(outOf) +
      " through " + centre + ", " + fmt(-gap) + " more than the " + fmt(into) +
      " the chart above puts through it.";
  const id = gapID(opened);
  // THE SIDE THE FIGURE IS ON IS THE SIDE THE MARK STANDS ON: too little
  // leaving arrives at the mark, too little arriving leaves it.
  const node = {
    id: id,
    label: "Difference between the two schedules",
    tier: gap > 0 ? tiers[tiers.length - 1] : tiers[0],
    // Parentless, so drawn --muted: it belongs to neither document.
    parent: "",
    constraint_tier: "",
    role: "gap",
    derived: true,
    in_cents: gap > 0 ? gap : 0,
    out_cents: gap > 0 ? 0 : -gap,
    rationale: lead + " " + licence.reason + " This mark is that " + fmt(Math.abs(gap)) +
      ", drawn so that the ribbons and the node agree; no page prints it as a figure of its own.",
    source_note: "Derived, not published: one document's total for this cell less the " +
      "other's. Each total is built from figures `fisc verify` ties to the pages the city " +
      "printed, and the difference is the one declared for this column; no page prints " +
      "it as a figure of its own.",
    locators: locators,
  };
  const link = gap > 0
    ? { source: opened, target: id, value_cents: gap }
    : { source: id, target: opened, value_cents: -gap };
  return Object.assign({}, drawn, {
    nodes: drawn.nodes.concat([node]),
    links: drawn.links.concat([Object.assign({
      kind: "", transfer_id: "", fact_ids: [], locators: locators, derived: true,
    }, link)]),
  });
}

/**
 * Every page cited by a ribbon of any of `docs` with an end at `opened` or
 * inside it, merged per document, documents and pages sorted: the pages of
 * the two totals a gap subtracts.
 * @param {string} opened
 * @param {FiscProjection[]} docs
 * @returns {FiscSource[]}
 */
export function citedAround(opened, docs) {
  const keys = new Set();
  for (const doc of docs) {
    const inside = withinNode(doc, opened);
    for (const l of doc.links) {
      if (!inside.has(l.source) && !inside.has(l.target)) continue;
      for (const s of l.locators || []) {
        for (const p of s.pages) keys.add(s.doc_id + "\u001f" + p);
      }
    }
  }
  return regroupLocators(keys);
}

/**
 * The id of the node an opened node's gap is drawn under.
 * @param {string} opened
 * @returns {string}
 */
export function gapID(opened) {
  return GAP_PREFIX + opened;
}

/**
 * Folds all but the largest `cap` nodes of one tier into a single node.
 *
 * A cap, not a rescale: the concentration is within a column, and the fold
 * tests measure the sub-pixel ribbons it removes. Values sum and fact ids and
 * locators union, as in the fold, so the aggregate stays citable. What MAY
 * fold is Go's (DrillStep.Caps); which nodes it keeps is this page's.
 *
 * @param {FiscProjection} doc
 * @param {number} tier
 * @param {number} cap
 * @param {string} opened  the node the aggregate is parented to: the opened
 *   node when the whole column is inside it, "" when it spans fund groups
 * @param {string} noun  the plural noun for the tier's rows
 * @returns {FiscProjection} doc itself when the column fits whole
 */
export function capColumn(doc, tier, cap, opened, noun) {
  const atTier = doc.nodes.filter((n) => n.tier === tier);
  // AN AGGREGATE OF ONE IS WORSE THAN NO AGGREGATE, so a column of cap + 1 is
  // drawn whole.
  if (atTier.length <= cap + 1) return doc;

  // RANKED BY THE LARGER OF INFLOW AND OUTFLOW, d3-sankey's node value: a
  // line takes in nothing, so inflow alone ties every line at zero. By
  // magnitude, because a contra row is a printed line as large as its figure.
  /** @type {Map<string, number>} */
  const inflow = new Map();
  /** @type {Map<string, number>} */
  const outflow = new Map();
  for (const l of doc.links) {
    inflow.set(l.target, (inflow.get(l.target) || 0) + Math.abs(l.value_cents));
    outflow.set(l.source, (outflow.get(l.source) || 0) + Math.abs(l.value_cents));
  }
  const size = (/** @type {string} */ id) => Math.max(inflow.get(id) || 0, outflow.get(id) || 0);
  // Ties broken by id, so the kept set is stable across builds.
  const ranked = atTier.slice().sort((a, b) =>
    size(b.id) - size(a.id) || (a.id < b.id ? -1 : 1));
  const kept = new Set(ranked.slice(0, cap).map((n) => n.id));
  const folded = ranked.slice(cap);

  // The noun is the view's. Pluralised though the threshold guarantees two,
  // because only this rule is about grammar.
  const word = noun || "items";
  const label = folded.length + " smaller " +
    (folded.length === 1 ? word.replace(/s$/, "") : word);
  // derived: true IS THE INVARIANT: the city printed no line called "N
  // smaller funds". Every cent inside is printed; the grouping is inferred.
  const aggregate = {
    id: aggregateID(tier), label: label, tier: tier,
    parent: opened,
    constraint_tier: "",
    // An empty role renders as an empty .chip.
    role: "aggregate",
    // THE IDS IT SWALLOWED, so caveatsFor still reaches them: the tail is
    // folded by value, which no parent chain records.
    folds: [],
    derived: true,
    rationale: "Our grouping, not a line the city printed: the " + folded.length +
      " smallest " + word + " in this column are drawn as one " +
      "mark because they cannot be drawn separately. Every figure inside it is printed; " +
      "the box around them is ours.",
    source_note: "",
  };
  const tail = new Set(folded.map((n) => n.id));
  const remap = (/** @type {string} */ id) => (tail.has(id) ? aggregateID(tier) : id);

  aggregate.folds = folded.map((n) => n.id);

  // A FOLDED NODE'S CHILDREN HANG FROM THE TAIL: foldDocument refuses a child
  // naming a parent the document does not carry, and dropping the child would
  // drop the money it carries onward -- a folded fund's object categories, in
  // a fund group's window.
  const nodes = doc.nodes
    .filter((n) => n.tier !== tier || kept.has(n.id))
    .map((n) => (tail.has(n.parent) ? Object.assign({}, n, { parent: aggregateID(tier) }) : n))
    .concat([aggregate]);
  // And the links that named them, for the same refusal.
  const present = new Set(nodes.map((n) => n.id));
  const links = doc.links
    .map((l) => Object.assign({}, l, { source: remap(l.source), target: remap(l.target) }))
    .filter((l) => present.has(l.source) && present.has(l.target));

  // THE FIGURE IS NOT HERE: tailFigure finishes the note after the fold and
  // any later cap have re-pointed these ribbons.
  aggregate.source_note = "The " + folded.length + " smallest of " + atTier.length +
    " by value, at this page's cap of " + cap;

  return Object.assign({}, doc, { nodes: nodes, links: links });
}

/**
 * Folds a document to the tiers this page draws: each node to its nearest
 * drawn ancestor, links merged on the folded pair and kind with values summed
 * and fact ids unioned. A link folding onto one node is dropped; the link
 * that survives carries the same money and facts.
 *
 * FAILS CLOSED on a node with no drawn ancestor: dropping it loses a column
 * silently, keeping it leaves it nowhere to draw.
 *
 * @param {FiscProjection} doc
 * @param {number[]} [tiers] the tier set to fold to, defaulting to
 *   RENDER_TIERS; a drill passes its own.
 * @returns {FiscProjection} doc itself when the tier set draws every tier.
 */
export function foldDocument(doc, tiers) {
  const wanted = tiers || RENDER_TIERS;
  if (!wanted.length) return doc;
  const byID = new Map(doc.nodes.map((n) => [n.id, n]));
  const drawn = new Set(wanted);

  /** @type {Map<string,string>} */
  const foldsTo = new Map();
  for (const n of doc.nodes) {
    const to = foldTarget(byID, n, drawn);
    if (!to) {
      throw new Error("cannot draw " + doc.projection + ": node " + n.id +
        " is tier " + n.tier + " and no ancestor of it is a tier this page draws (" +
        wanted.join(", ") + ")");
    }
    foldsTo.set(n.id, to);
  }

  /** @type {Map<string, FiscLink>} */
  const merged = new Map();
  /** @type {Map<string, Set<string>>} */
  const cited = new Map();
  // Locators union as fact_ids do, keyed doc\u001fpage; without this a merged
  // ribbon cites only its first leg's pages.
  /** @type {Map<string, Set<string>>} */
  const located = new Map();
  /** @param {FiscSource[]} ss @returns {string[]} */
  const locatorKeys = (ss) => {
    const out = [];
    for (const s of ss) {
      for (const p of s.pages) out.push(s.doc_id + "\u001f" + p);
    }
    return out;
  };
  for (const l of doc.links) {
    const source = foldsTo.get(l.source);
    const target = foldsTo.get(l.target);
    if (source === target) continue;
    // ONE RIBBON PER KIND BETWEEN A FOLDED PAIR, so a ribbon's kind is true of
    // all of it.
    const key = source + "\u001f" + target + "\u001f" + l.kind;
    const at = merged.get(key);
    const ids = cited.get(key);
    if (!at || !ids) {
      merged.set(key, Object.assign({}, l, { source: source, target: target }));
      // ITERATED, NOT new Set(l.fact_ids): that would turn an absent fact_ids
      // into a ribbon citing nothing instead of a throw.
      const first = new Set();
      for (const id of l.fact_ids) first.add(id);
      cited.set(key, first);
      located.set(key, new Set(locatorKeys(l.locators)));
      continue;
    }
    // A PRINTED LEG AND AN INFERRED ONE NEVER MEET HERE -- the export refuses
    // such a cap -- so the first leg's derived flag is true of all of it.
    at.value_cents += l.value_cents;
    // A transfer id names one leg of one transfer and cannot survive a merge.
    if (at.transfer_id !== l.transfer_id) at.transfer_id = "";
    for (const id of l.fact_ids) ids.add(id);
    const locs = located.get(key);
    if (locs) for (const k of locatorKeys(l.locators)) locs.add(k);
  }

  const links = Array.from(merged.entries())
    .map(([key, l]) => Object.assign(l, {
      fact_ids: Array.from(cited.get(key) || []).sort(),
      // The shape Go publishes, so citations() cannot tell a folded link.
      locators: regroupLocators(located.get(key)),
    }))
    .sort((a, b) => (a.source < b.source ? -1 : a.source > b.source ? 1
      : a.target < b.target ? -1 : a.target > b.target ? 1
      : a.kind < b.kind ? -1 : a.kind > b.kind ? 1 : 0));

  // An untouched node is not drawable: d3-sankey lands it in the first column
  // at zero height.
  const touched = new Set();
  for (const l of links) {
    touched.add(l.source);
    touched.add(l.target);
  }
  // A RETAINED NODE'S parent IS RE-POINTED AT ITS FOLDED ANCESTOR, so every
  // parent resolves in the folded document; "" where its parent folded into
  // it. A loop, because the walk continues past an ancestor the filter above
  // dropped.
  const nodes = doc.nodes.filter((n) => touched.has(n.id)).map((n) => {
    let up = n.parent ? foldsTo.get(n.parent) : "";
    for (let hops = 0; up && up !== n.id && !touched.has(up); hops++) {
      const above = byID.get(up);
      up = above && above.parent && hops < 8 ? foldsTo.get(above.parent) : "";
    }
    return Object.assign({}, n, { parent: up && up !== n.id ? up : "" });
  });

  return Object.assign({}, doc, { nodes: nodes, links: links });
}

/**
 * Re-stacks each node's ribbons in the order of the ends they run to.
 *
 * d3-sankey sorts before its last relaxation move, so ribbons can cross at the
 * node face for no reason in the data. Widths are untouched.
 * @param {{nodes:LaidNode[], links:LaidLink[]}} graph
 */
export function restackLinks(graph) {
  /** @param {(l:LaidLink) => LaidNode} end */
  const byOtherEnd = (end) =>
    /** @param {LaidLink} a @param {LaidLink} b */ (a, b) =>
      end(a).y0 - end(b).y0 || a.index - b.index;

  for (const node of graph.nodes) {
    node.sourceLinks.sort(byOtherEnd((l) => l.target));
    // CONTRA LAST ON THE ARRIVING SIDE, so reductions stack inside the band.
    // A tiebreak, not a reorder: nodeRank's crossing count rests on that.
    node.targetLinks.sort((a, b) =>
      Number(Boolean(a.contra)) - Number(Boolean(b.contra)) ||
      byOtherEnd((l) => l.source)(a, b));
  }
  for (const node of graph.nodes) {
    let leaving = node.y0;
    for (const l of node.sourceLinks) {
      l.y0 = leaving + l.width / 2;
      leaving += l.width;
    }
    let arriving = node.y0;
    for (const l of node.targetLinks) {
      l.y1 = arriving + l.width / 2;
      arriving += l.width;
    }
  }
}

/**
 * Which column d3-sankey puts a node in: the position of its tier in the
 * declared order, or d3's own justify when nothing was declared.
 *
 * Justify puts a link-less sink in the last column, wrong once tiers can be
 * skipped. The order may be non-monotonic, so it is indexOf, not a sort. The
 * tier set is the caller's: aligning a drilled document on the page's set
 * gives indexOf -1, and d3-sankey dies in its ordering pass.
 *
 * @param {number[]} tiers
 * @returns {(d: LaidNode) => number}
 */
export function alignFor(tiers) {
  return tiers.length
    ? /** @param {LaidNode} d */ (d) => tiers.indexOf(d.tier)
    : D3.sankeyJustify;
}

/**
 * Lays a document out, touching nothing on the page.
 *
 * SEPARATE FROM render() SO A DOCUMENT THAT WILL NOT DRAW CANNOT LEAVE TWO
 * FISCAL YEARS ON THE PAGE: everything that can throw is here, and showYear
 * calls it before repainting anything (fisc-bsg).
 *
 * @param {FiscProjection} doc
 */
export function layOut(doc) {
  // THE HUE INDEX: the drawn nodes, overwritten by the unfolded document this
  // rung was shaped from, because a colour is where a node really sits and
  // the fold re-points parents. The drawn set contributes only what the file
  // lacks, such as the aggregate. Assigned before anything can throw.
  const previous = groupIndex;
  groupIndex = new Map(doc.nodes.map((n) => [n.id, n]));
  const source = drawnDoc();
  if (source) {
    for (const n of source.nodes) groupIndex.set(n.id, n);
  }

  const sankey = D3.sankey()
    .nodeId(/** @param {LaidNode} d */ (d) => d.id)
    .nodeWidth(NODE_WIDTH)
    .nodePadding(NODE_PADDING)
    .nodeAlign(alignFor(activeTiers()))
    // Supplying this switches d3's own ordering pass off.
    .nodeSort(/** @param {LaidNode} a @param {LaidNode} b */ (a, b) =>
      nodeRank(a) - nodeRank(b) || b.value - a.value)
    // The same column count render() sizes the viewBox from.
    .extent([[LABEL_GUTTER, 12],
      [chartWidth(drawnColumns()) - LABEL_GUTTER, CHART_HEIGHT - 12]]);

  // d3-sankey mutates its input, so it gets a copy and the fetched document
  // stays the thing the table and the detail panel read from.
  /** @type {{nodes:LaidNode[], links:LaidLink[]}} */
  let graph;
  try {
    graph = sankey({
      nodes: doc.nodes.map((n) => Object.assign({}, n)),
      links: doc.links.map((l) => Object.assign({}, l, { value: l.value_cents })),
    });
  } catch (e) {
    // THE INDEX GOES BACK WITH THE THROW, or the next paint() recolours the
    // chart on screen against a document it was not drawn from.
    groupIndex = previous;
    throw e;
  }
  restackLinks(graph);
  // For columnShare: only the laid graph knows a mark's column.
  laidNodes = graph.nodes;
  return graph;
}

/**
 * The column a node was drawn in, as an index into the declared order.
 *
 * NOT d.depth, the longest path to the node, which differs wherever no ribbon
 * reaches a node from the column before it. Not d.layer either, except in a
 * view that declares no column order, where sankeyJustify chose the columns.
 *
 * @param {LaidNode} d
 * @returns {number}
 */
export function columnOf(d) {
  const tiers = activeTiers();
  // d3 clamps an undeclared tier into column 0, so it is labelled as one.
  return tiers.length ? Math.max(0, tiers.indexOf(d.tier)) : d.layer;
}

/**
 * Where a node's label goes: the side it is anchored on, and the point.
 *
 * A LABEL MAY RUN OUTWARD ONLY INTO A GUTTER, and only the first and last
 * columns have one. An interior label is centred above its rect, in the
 * NODE_PADDING gap, rather than across the ribbons the next column receives.
 *
 * @param {LaidNode} d
 * @param {number} last the largest column index this chart drew
 * @returns {{anchor: string, x: number, y: number, dy: string|null}}
 */
export function labelPlacement(d, last) {
  const col = columnOf(d);
  const middle = (d.y0 + d.y1) / 2;
  if (col === 0) return { anchor: "end", x: d.x0 - 10, y: middle, dy: "0.35em" };
  if (col >= last) return { anchor: "start", x: d.x1 + 10, y: middle, dy: "0.35em" };
  // No dy: a half-em shift down would drop the glyphs onto the rect.
  return { anchor: "middle", x: (d.x0 + d.x1) / 2, y: d.y0 - 5, dy: null };
}

/**
 * Each mark's parent label, on every mark whose label another mark in the same
 * column also carries; nothing elsewhere.
 *
 * AN AMBIGUITY IS A PROPERTY OF THE COLUMN, NOT THE NODE, so it is decided here
 * rather than in the document. The parent is looked up in the document, not the
 * laid graph, because a window routinely does not draw it. A duplicate whose
 * parent is unnameable gets "" and stays ambiguous rather than invented.
 *
 * @param {LaidNode[]} nodes
 * @returns {Map<string, string>}
 */
export function labelQualifiers(nodes) {
  /** @type {Map<string, LaidNode[]>} */
  const sharing = new Map();
  for (const n of nodes) {
    const key = columnOf(n) + "\u0000" + n.label;
    const seen = sharing.get(key);
    if (seen) seen.push(n);
    else sharing.set(key, [n]);
  }
  /** @type {Map<string, string>} */
  const out = new Map();
  for (const shared of sharing.values()) {
    if (shared.length < 2) continue;
    for (const n of shared) {
      const parent = projection
        ? projection.nodes.find((p) => p.id === n.parent)
        : null;
      out.set(n.id, parent ? parent.label : "");
    }
  }
  return out;
}

/**
 * The dy each of a qualified label's two lines is drawn at, relative to the
 * line before it. The qualifier goes above: an outward pair straddles the
 * rect's middle; an interior label has nowhere below to go, so the qualifier
 * is lifted a whole line and the label stays put.
 *
 * NO COMMITTED VIEW DRAWS THE INTERIOR BRANCH, so no check sees it; fisc-xhqt.
 *
 * @param {string} anchor
 * @returns {{qualifier: string, label: string}}
 */
export function labelLineShift(anchor) {
  return anchor === "middle"
    ? { qualifier: "-1.15em", label: "1.15em" }
    : { qualifier: "-0.6em", label: "1.15em" };
}

/**
 * Draws a laid-out graph. Omitting `laid` lays the projection out here, which
 * is non-atomic with anything else on the page.
 * @param {{nodes:LaidNode[], links:LaidLink[]}} [laid]
 */
export function render(laid) {
  if (!projection) return;
  const graph = laid || layOut(projection);
  const svg = D3.select("#chart");
  const width = chartWidth(drawnColumns());
  const height = CHART_HEIGHT;

  // No width or height attributes: the viewBox makes the drawing scale.
  svg.attr("viewBox", "0 0 " + width + " " + height);
  svg.selectAll("g").remove();

  const gLinks = svg.append("g").attr("class", "links");
  const gNodes = svg.append("g").attr("class", "nodes");

  gLinks.selectAll("path")
    .data(graph.links)
    .join("path")
    .attr("class", /** @param {LaidLink} d */ (d) => linkClass(d))
    .attr("d", D3.sankeyLinkHorizontal())
    // The surface gap, not a stroke, separates stacked ribbons.
    .attr("stroke-width", /** @param {LaidLink} d */ (d) => Math.max(1, d.width - RIBBON_GAP))
    .attr("tabindex", 0)
    .attr("role", "button")
    .attr("aria-label", /** @param {LaidLink} d */ (d) => linkDescription(d))
    .on("pointerenter", /** @param {PointerEvent} e @param {LaidLink} d */ (e, d) => showTip(e, d))
    .on("pointermove", /** @param {PointerEvent} e @param {LaidLink} d */ (e, d) => showTip(e, d))
    .on("pointerleave", hideTip)
    .on("focus", /** @param {FocusEvent} e @param {LaidLink} d */ (e, d) => guarded("show this flow", () => { if (restoring) return; showTip(e, d); pin(d); }))
    .on("blur", hideTip)
    .on("click", /** @param {MouseEvent} e @param {LaidLink} d */ (e, d) => guarded("pin this flow", () => { e.stopPropagation(); pin(d); }));

  const node = gNodes.selectAll("g")
    .data(graph.nodes)
    .join("g")
    .attr("class", /** @param {LaidNode} d */ (d) => nodeClass(d))
    .attr("tabindex", 0)
    .attr("role", "button")
    // aria-pressed is the isolation, on every node. OPENING IS NOT THE TOGGLE
    // and must never be announced as one: it replaces the chart, leaving no
    // pressed state to return to. The label announces what a node opens into.
    .attr("aria-pressed", "false")
    .attr("aria-label", /** @param {LaidNode} d */ (d) => nodeDescription(d))
    // Both keys activate every node; which one opens is nodeDescription's to say.
    .attr("aria-keyshortcuts", "Enter Space")
    .on("pointerenter", /** @param {PointerEvent} e @param {LaidNode} d */ (e, d) => showTip(e, d))
    .on("pointermove", /** @param {PointerEvent} e @param {LaidNode} d */ (e, d) => showTip(e, d))
    .on("pointerleave", hideTip)
    .on("focus", /** @param {FocusEvent} e @param {LaidNode} d */ (e, d) => guarded("show this mark", () => { if (restoring) return; showTip(e, d); pin(d); }))
    .on("blur", hideTip)
    // TWO GESTURES, ONE MEANING EACH: a single click and Space isolate on every
    // node; a double click and Enter open the nodes that open.
    //
    // Both click and keydown, because an SVG g[role=button] synthesises no click
    // from Enter, and some screen readers send both -- which would toggle twice.
    // So the guard is on the activation: a click on the node a key just
    // activated is that key's own click. Focus must not isolate, and a held key
    // is ignored, or tabbing would strobe the chart.
    .on("click", /** @param {MouseEvent} e @param {LaidNode} d */ (e, d) => {
      guarded("pin this mark", () => {
        e.stopPropagation();
        clickNode(d, e.timeStamp);
      });
    })
    // ON EVERY NODE, not only one that opens: its two clicks have already
    // toggled the isolation twice, and this restores what was isolated before.
    // preventDefault stops the double click selecting the label.
    .on("dblclick", /** @param {MouseEvent} e @param {LaidNode} d */ (e, d) => {
      guarded("open this mark", () => {
        e.stopPropagation();
        e.preventDefault();
        doubleClickNode(d, e.timeStamp);
      });
    })
    .on("keydown", /** @param {KeyboardEvent} e @param {LaidNode} d */ (e, d) => {
      if (e.key !== "Enter" && e.key !== " ") return;
      if (e.repeat) return;
      e.preventDefault();
      guarded("act on this mark", () => keyNode(d, e.key, e.timeStamp));
    });

  node.append("rect")
    .attr("x", /** @param {LaidNode} d */ (d) => d.x0)
    .attr("y", /** @param {LaidNode} d */ (d) => d.y0)
    .attr("width", /** @param {LaidNode} d */ (d) => d.x1 - d.x0)
    .attr("height", /** @param {LaidNode} d */ (d) => Math.max(2, d.y1 - d.y0))
    .attr("rx", 2);

  // On every mark, displayed on the ones that hang: the vendored d3 selection
  // has no filter(). pointer-events none, because the band is not the mark.
  node.append("rect")
    .attr("class", "contra-band")
    .attr("display", /** @param {LaidNode} d */ (d) => (contraBand(d) ? null : "none"))
    .attr("x", /** @param {LaidNode} d */ (d) => d.x0)
    .attr("y", /** @param {LaidNode} d */ (d) => (contraBand(d) || { y: 0 }).y)
    .attr("width", /** @param {LaidNode} d */ (d) => d.x1 - d.x0)
    .attr("height", /** @param {LaidNode} d */ (d) => (contraBand(d) || { height: 0 }).height)
    .attr("pointer-events", "none");

  // Every node is directly labelled: the relief the palette's contrast check
  // requires.
  const lastColumn = Math.max(...graph.nodes.map(columnOf));
  const qualified = labelQualifiers(graph.nodes);
  /** @param {LaidNode} d */
  const qualifierOf = (d) => qualified.get(d.id) || "";
  const label = node.append("text")
    .attr("class", "halo")
    .attr("y", /** @param {LaidNode} d */ (d) => labelPlacement(d, lastColumn).y)
    .attr("dy", /** @param {LaidNode} d */ (d) => labelPlacement(d, lastColumn).dy)
    .attr("x", /** @param {LaidNode} d */ (d) => labelPlacement(d, lastColumn).x)
    .attr("text-anchor", /** @param {LaidNode} d */ (d) => labelPlacement(d, lastColumn).anchor);

  // On an unqualified mark the qualifier tspan is empty with no x or dy, so it
  // starts no line.
  label.append("tspan")
    .attr("class", "qualifier")
    .attr("x", /** @param {LaidNode} d */ (d) =>
      (qualifierOf(d) ? labelPlacement(d, lastColumn).x : null))
    .attr("dy", /** @param {LaidNode} d */ (d) =>
      (qualifierOf(d)
        ? labelLineShift(labelPlacement(d, lastColumn).anchor).qualifier
        : null))
    .text(/** @param {LaidNode} d */ (d) => qualifierOf(d));
  label.append("tspan")
    .attr("x", /** @param {LaidNode} d */ (d) =>
      (qualifierOf(d) ? labelPlacement(d, lastColumn).x : null))
    .attr("dy", /** @param {LaidNode} d */ (d) =>
      (qualifierOf(d)
        ? labelLineShift(labelPlacement(d, lastColumn).anchor).label
        : null))
    .text(/** @param {LaidNode} d */ (d) => d.label);
  label.append("tspan")
    .attr("class", "value")
    .text(/** @param {LaidNode} d */ (d) => "  " + fmtShortSigned(markCents(d)));
  label.append("tspan")
    .attr("class", "flag")
    .text(/** @param {LaidNode} d */ (d) => nodeFlags(d));

  paint();
  applyEmphasis();
}

/** Re-reads the palette from CSS and repaints. Called after a theme change. */
export function paint() {
  D3.select("#chart").selectAll("path.link")
    .attr("stroke", /** @param {LaidLink} d */ (d) => linkColor(d));
  D3.select("#chart").selectAll("g.node rect")
    .attr("fill", /** @param {LaidNode} d */ (d) => nodeColor(d));
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
  if (derived) return "inferred by us";
  return ontoOurs ? CARRIED_NOTE : "printed by the city";
}

/** What a printed figure re-pointed onto a derived mark is: the only spelling. */
export const CARRIED_NOTE = "figure printed by the city, re-pointed onto a mark of ours";

/** CARRIED_NOTE's short form, for a chip. */
export const CARRIED_CHIP = "\u25c7 re-pointed by us";

/**
 * @param {LaidLink} d
 * @returns {string}
 */
export function linkDescription(d) {
  const kind = /** @type {Record<string,string>} */ (KIND_LABEL)[d.kind] || d.kind;
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
  const what = drillable(d)
    ? ", opens into its parts on a double click or Enter; a single click or Space follows " +
      "this money"
    : expandable(d)
      ? ", draws all of them separately on a double click or Enter; a single click or Space " +
        "follows this money"
      : ", follow this money";
  const note = contraNote(d);
  // The cross-tab qualification, for a reader who cannot see the ribbons.
  const flows = residualFlows(d);
  return d.label + (flows ? ", " + flows : ", total " + fmtSigned(markCents(d))) +
    (d.derived ? ", inferred by us" : ", printed by the city") +
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
  if (!source || !source.metadata || !Array.isArray(source.metadata.caveats)) {
    return [];
  }
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
 * The share one mark is of the money in its column, as a percentage.
 *
 * IT IS ARITHMETIC AND SAYS SO: no page prints it. Computed from the DRAWN
 * values, a residual's included.
 *
 * @param {LaidNode} d
 * @returns {string}
 */
export function columnShare(d) {
  if (!d.value) return "";
  let total = 0;
  let siblings = 0;
  for (const other of laidNodes) {
    if (other.layer === d.layer) {
      total += other.value;
      siblings++;
    }
  }
  // No share of a column of one: it is 100% by construction.
  if (!total || siblings < 2) return "";
  const pct = (100 * d.value) / total;
  // A CEILING AS WELL AS A FLOOR: toFixed(1) would round a divided column's
  // largest share to "100.0", a whole its sibling denies.
  const shown = pct < 0.1 ? "<0.1" : pct > 99.9 ? ">99.9" : pct.toFixed(1);
  // "our" is what survives the diamond being read aloud.
  return "\u25c7 our " + shown + "% of this column";
}

/**
 * @param {LaidLink | LaidNode} d
 * @returns {boolean}
 */
export function isLink(d) {
  return Object.prototype.hasOwnProperty.call(d, "fact_ids");
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
    meta.append(h("span", "chip", /** @type {Record<string,string>} */ (KIND_LABEL)[l.kind] || l.kind));
    meta.append(document.createTextNode(" "));
    const lentTo = Boolean(l.source.derived || l.target.derived);
    meta.append(h("span", l.derived || lentTo ? "chip derived" : "chip",
      l.derived ? "◇ inferred" : lentTo ? CARRIED_CHIP : "printed"));
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
    meta.append(h("span", n.derived ? "chip derived" : "chip", n.derived ? "◇ inferred" : "printed"));
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
    const kind = /** @type {Record<string,string>} */ (KIND_LABEL)[l.kind] || l.kind;
    if (kind) chips.append(h("span", "chip", kind));
    const lent = Boolean(l.source.derived || l.target.derived);
    chips.append(h("span", l.derived || lent ? "chip derived" : "chip",
      l.derived ? "◇ our inference" : lent ? CARRIED_CHIP : "printed by the city"));
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
    chips.append(h("span", n.derived ? "chip derived" : "chip", n.derived ? "◇ our inference" : "printed by the city"));
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

/**
 * Which derived node an inferred flow is listed under, or "" for one whose
 * endpoints are both printed.
 *
 * ONE ENTRY PER FLOW, so a flow with a derived node at both ends is not listed
 * twice. The mark it arrives at wins: an inferred flow is evidence about the
 * mark that receives it.
 *
 * @param {FiscLink} l
 * @returns {string}
 */
export function homeOf(l) {
  if (!projection) return "";
  const derived = new Set(projection.nodes.filter((n) => n.derived).map((n) => n.id));
  if (derived.has(l.target)) return l.target;
  if (derived.has(l.source)) return l.source;
  return "";
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
    li.append(h("div", "why", "This flow is inferred; both endpoints are printed by the city."));
    li.append(h("div", "subtle", fmt(l.value_cents)));
    list.append(li);
  }

  if (!nodes.length && !orphans.length) {
    list.append(h("li", "subtle", "Nothing on this chart is inferred: every node and flow is printed by the city."));
  }
}

export function tableRows(doc) {
  const out = [];
  if (!doc) return out;
  const labels = new Map(doc.nodes.map((n) => [n.id, n.label]));
  const ours = new Set(doc.nodes.filter((n) => n.derived).map((n) => n.id));

  for (const l of doc.links) {
    const tr = document.createElement("tr");
    // A contra row reads as printed: signed, with the words for what it reduces.
    if (l.contra) tr.className = "contra";
    tr.append(h("td", "", labels.get(l.source) || l.source));
    tr.append(h("td", "", labels.get(l.target) || l.target));
    tr.append(h("td", "num", fmtSigned(l.contra ? -l.value_cents : l.value_cents)));
    tr.append(h("td", "", /** @type {Record<string,string>} */ (KIND_LABEL)[l.kind] || l.kind));
    tr.append(h("td", "", l.derived ? "◇ inferred"
      : l.contra ? l.contra
        : l.partition ? PARTITION_NOTE
          : ours.has(l.source) || ours.has(l.target) ? CARRIED_CHIP : "printed"));
    tr.append(h("td", "ids", l.fact_ids.join(" ")));
    // PER ROW, not per document: the pages this row's own figure was read from.
    const td = h("td", "");
    for (const c of citations(l.locators)) {
      td.append(link(c.label, c.href));
      td.append(document.createTextNode(" "));
    }
    tr.append(td);
    out.push(tr);
  }
  return out;
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
 * One schedule of a column document, with the shared node table's indices
 * resolved to ids, a node's identity and annotations taken from the table and
 * its parent from the schedule, and the keys the column omits (empty strings,
 * false booleans) filled in.
 *
 * @param {any} column
 * @param {string} key the schedule to read -- a step's `projection`
 * @returns {FiscProjection | null} null when the column carries no such schedule
 */
export function scheduleOf(column, key) {
  const sched = column && column.schedules ? column.schedules[key] : null;
  if (!sched) return null;
  const table = Array.isArray(column.nodes) ? column.nodes : [];

  // A malformed schedule throws below, before showYear writes a word.

  const nodes = sched.nodes.map((n) => {
    const base = table[n.node] || {};
    return {
      id: base.id, label: base.label, tier: base.tier,
      role: base.role || "", derived: Boolean(base.derived),
      constraint_tier: base.constraint_tier || "",
      rationale: base.rationale || "", source_note: base.source_note || "",
      // The one field a schedule states for itself: where the node hangs in
      // its own hierarchy.
      parent: n.parent || "",
    };
  });
  const links = sched.links.map((l) => {
    const from = table[l.from] || {};
    const to = table[l.to] || {};
    return {
      source: from.id, target: to.id,
      value_cents: l.value_cents, kind: l.kind,
      transfer_id: l.transfer_id || "",
      // Not defaulted: a default would invent provenance.
      fact_ids: l.fact_ids, locators: l.locators,
      derived: Boolean(l.derived), partition: Boolean(l.partition),
      // Defaulted: the schema carries contra only on a link printed negative.
      contra: l.contra || "",
    };
  });
  const col = column.column || {};
  return /** @type {any} */ ({
    schema_version: SCHEMA_VERSION,
    projection: key,
    nodes, links,
    metadata: {
      generated_by: column.generated_by || "",
      fiscal_year: col.fiscal_year, fiscal_year_label: col.label,
      basis: col.basis,
      scopes: sched.scopes,
      currency: "USD", units: "cents",
      sources: sched.sources,
      headline: sched.headline || {},
      counts: sched.counts || {},
      caveats: sched.caveats || [],
    },
  });
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
  if (lede) lede.textContent = year.label + " " + year.basis;

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
