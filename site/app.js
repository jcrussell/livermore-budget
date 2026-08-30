// @ts-check
/**
 * fisc — the published page's client script.
 *
 * Plain browser JavaScript with JSDoc types, checked with `tsc --checkJs`.
 * There is no bundler and no npm in the deploy path: this file is served
 * exactly as it is committed, alongside the vendored d3 bundles.
 *
 * The division of labour with the Go side is deliberate. `window.FISC_CONFIG`
 * carries the metadata the page needs before it has fetched anything — the
 * fiscal year, the headline totals, the caveats, where the source documents
 * live — and it carries the projection's own metadata block verbatim, so the
 * page and the JSON cannot disagree about a figure. Everything bulky (nodes,
 * links, fact ids) is fetched from data/<projection>.json, so a reader can
 * curl the provenance file on its own.
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
 * @property {string} scope
 * @property {string} currency
 * @property {string} units
 * @property {FiscSource[]} sources
 * @property {Record<string, number>} headline
 * @property {{facts:number, nodes:number, links:number}} counts
 * @property {FiscCaveat[]} caveats
 */

/**
 * One thing the document cannot show.
 *
 * IT WAS A BARE STRING until the caveats got a page of their own. `text` is
 * that string, unchanged; `summary` is the line a page shows in its place, and
 * `id` is the anchor it links to. `applies_to` names the nodes the caveat is
 * about, and is EMPTY for a caveat about the schedule rather than about any
 * mark -- empty means document-wide, not "not filled in".
 * @typedef {Object} FiscCaveat
 * @property {string} id
 * @property {string} summary
 * @property {string} text
 * @property {string[]} applies_to
 */

/**
 * A caveat as the PAGE shows it: a line, and somewhere to go for the rest.
 *
 * NO `text`, on purpose. The client renders summaries and links to the caveats
 * page; a client that had the paragraph in hand would eventually print it, and
 * printing it under the chart is the thing this page stopped doing.
 *
 * `href` IS EMPTY WHEN THE SITE HAS NO CAVEATS PAGE -- a single-view export
 * writes index.html and nothing else -- and the renderer falls back to plain
 * text rather than shipping a link that 404s.
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
 * One published fiscal year, with every word that belongs to it. The packager
 * builds these in Go for all of them; the client only chooses.
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
 * @property {string} chart_title
 */

/**
 * @typedef {Object} FiscConfig
 * @property {number} schema_version
 * @property {string} exported_by
 * @property {string} primary
 * @property {Record<string, string>} projections
 * @property {FiscYear[]} years
 * @property {FiscMetadata} metadata
 * @property {Record<string, FiscDoc>} docs
 * @property {number[]} [render_tiers]
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
 * A node after d3-sankey has laid it out. d3 mutates the objects it is given,
 * so this extends FiscNode rather than replacing it.
 * @typedef {FiscNode & {x0:number, x1:number, y0:number, y1:number, value:number,
 *   sourceLinks:LaidLink[], targetLinks:LaidLink[], depth:number}} LaidNode
 */

/**
 * A link after layout: source and target are node objects, not ids.
 * @typedef {Omit<FiscLink,"source"|"target"> & {source:LaidNode, target:LaidNode,
 *   value:number, width:number, y0:number, y1:number, index:number}} LaidLink
 */

/** d3 and d3-sankey are vendored UMD bundles with no type declarations. */
const D3 = /** @type {any} */ (/** @type {any} */ (globalThis).d3);

/** @type {FiscConfig} */
const CONFIG = /** @type {any} */ (globalThis).FISC_CONFIG;

/**
 * The projection schema this client draws.
 *
 * This is the third copy of one constant — project.SchemaVersion stamps the
 * document, export.SchemaVersion gates the packager, and this gates the
 * browser — and it is the copy nothing compiles against, so it is the one that
 * would drift silently. A test in internal/export reads this file and pins the
 * literal below to the producer's constant; if you change it here, change it
 * there, and expect that test to say so if you do not.
 *
 * The gate matters because a schema bump changes what the graph MEANS rather
 * than what it is spelled like. A version-2 document read by this code would
 * draw a chart that is WRONG, not one that fails, and a wrong chart of public
 * money is the single outcome this project exists to avoid. So it refuses.
 */
const SCHEMA_VERSION = 1;

/**
 * The fund-group column, top to bottom, and with it the categorical slot each
 * fund group wears (position 1 gets slot 1, and so on — see style.css).
 *
 * This order is measured, not chosen for looks. Ribbons stack at a node in
 * column order, so the fund colours that touch are the consecutive pairs of
 * whichever funds are present at that node. Over the pairs that actually
 * occur in this graph, this ordering's worst pair is CVD dE 9.1 light / 8.4
 * dark (target 8) and normal-vision dE 19.6 / 19.3 (floor 15). The obvious
 * orderings do not clear that: sorting the column by size drops the worst
 * dark pair to 6.9, and one ordering collapses it to 1.6. Re-run the dataviz
 * validator over the touching pairs before changing this.
 * @type {string[]}
 */
const FUND_ORDER = [
  "fund-group/internal-service",
  "fund-group/capital",
  "fund-group/general",
  "fund-group/special-revenue",
  "fund-group/enterprise",
  "fund-group/debt-service",
];

/** Fund group id -> the CSS custom property holding its hue. */
const FUND_COLOR_VAR = {
  "fund-group/internal-service": "--fund-internal-service",
  "fund-group/capital": "--fund-capital",
  "fund-group/general": "--fund-general",
  "fund-group/special-revenue": "--fund-special-revenue",
  "fund-group/enterprise": "--fund-enterprise",
  "fund-group/debt-service": "--fund-debt-service",
};

/**
 * The node tiers this page draws, coarsest first; empty draws the document as
 * it stands.
 *
 * THIS IS PER-VIEW CONFIGURATION AND MUST NEVER BECOME A CONSTANT IN THIS FILE.
 * The spine and the drill-down are drawn by the same script from documents with
 * different hierarchies: the spine publishes tiers 0, 2 and 5 and is drawn
 * whole, while the drill-down publishes 0, 2, 3, 4 and 5 and is drawn at 0/2/4.
 *
 * Applying one page's set to the other document REFUSES rather than corrupts,
 * and the distinction is worth stating because the first version of this
 * comment got it wrong: {0,2,4} over the spine does not quietly fold its
 * expenditure column away, it throws, because every spine node is parentless
 * and a tier-5 node has no drawn ancestor to fold to. That is the better of the
 * two failures and it is still a broken page, which is what makes the tier set
 * something a view declares rather than something this file assumes.
 *
 * Absent, the fold is skipped entirely rather than run with a set covering
 * every tier, so a page that does not opt in is laid out by exactly the code
 * that laid it out before the fold existed.
 * @type {number[]}
 */
const RENDER_TIERS = CONFIG && Array.isArray(CONFIG.render_tiers) ? CONFIG.render_tiers : [];

/** Human wording for link.kind. The JSON's vocabulary is not English. */
const KIND_LABEL = {
  external: "external money",
  internal_transfer: "transfer between funds",
  internal_service: "internal service charge",
  fund_balance: "fund balance movement",
};

const NODE_WIDTH = 14;
const NODE_PADDING = 14;
/** The surface gap that separates stacked ribbons, in px (1px each side). */
const RIBBON_GAP = 2;
/**
 * The chart is laid out at a fixed size and scaled by the viewBox, rather
 * than re-laid-out at the container's width. A sankey's labels do not reflow:
 * at 700px the three columns and their labels collide, and the only honest
 * fixes are a horizontal scrollbar or a fixed design width that shrinks as a
 * whole. This is the second.
 */
const CHART_WIDTH = 1180;
const CHART_HEIGHT = 820;

/**
 * Room reserved either side of the plot for node labels, in px. Sized from
 * the widest label this data produces — "Fund Balance Contribution  $12.8M ◇"
 * at roughly 230px — because a label that does not fit must not be clipped,
 * and there is nowhere else for a sankey node's name to go.
 */
const LABEL_GUTTER = 250;

const money = new Intl.NumberFormat("en-US", {
  style: "currency",
  currency: "USD",
  maximumFractionDigits: 0,
});
const moneyCompact = new Intl.NumberFormat("en-US", {
  style: "currency",
  currency: "USD",
  notation: "compact",
  maximumFractionDigits: 1,
});

/** @param {number} cents */
function fmt(cents) {
  return money.format(cents / 100);
}

/** @param {number} cents */
function fmtShort(cents) {
  return moneyCompact.format(cents / 100);
}

/**
 * @param {string} id
 * @returns {HTMLElement}
 */
function el(id) {
  const found = document.getElementById(id);
  if (!found) throw new Error("missing element #" + id);
  return found;
}

/**
 * The element with this id, or null if the template did not render one.
 *
 * el() THROWS on a missing id, deliberately: most of this file addresses
 * elements the template always renders, and a silent null there would surface
 * as a blank region rather than as the broken template it is. But some elements
 * are conditional -- the year toggle exists only when more than one year is
 * published -- and for those `if (!el(id))` is not a guard at all, it is an
 * exception one line earlier. Reaching for el() there took the whole chart down
 * on a single-year build.
 * @param {string} id
 * @returns {HTMLElement | null}
 */
function maybeEl(id) {
  return document.getElementById(id);
}

/**
 * @param {string} name CSS custom property, including the leading dashes.
 * @returns {string}
 */
function cssVar(name) {
  return getComputedStyle(document.documentElement).getPropertyValue(name).trim();
}

/**
 * @param {FiscNode | LaidNode} node
 * @returns {boolean}
 */
function isFundGroup(node) {
  return node.id.startsWith("fund-group/");
}

/**
 * The fund group a node belongs to, walking node.parent until it reaches one.
 *
 * THE SPINE HAS NO HIERARCHY AND THIS IS WHY THE WALK IS SAFE THERE. Every one
 * of testdata/sankey.golden.json's 25 nodes carries parent: "", so the loop
 * exits on its first test and every node answers for itself exactly as
 * isFundGroup did. The drill-down is the first document with parents to walk:
 * a fund's group is its parent, and a department's is its fund's.
 *
 * Returns "" for a node with no fund group above it -- tier 0 revenue sources
 * on both documents, and any node whose chain runs out. The callers all treat
 * "" as "no categorical slot", which is what --muted means.
 * @param {FiscNode | LaidNode} node
 * @returns {string}
 */
function fundGroupOf(node) {
  let at = node;
  // Bounded by the hierarchy's depth; the guard is against a parent cycle in a
  // malformed document, which node-hierarchy-well-formed rejects Go-side but
  // this file cannot assume it ran.
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
 * The hue a link wears is its fund group's, inherited through node.parent when
 * neither end IS one. Colour follows the entity, never the value or the rank.
 *
 * A link with a fund group at one end takes it. Otherwise both ends are inside
 * one group's subtree -- the drill-down's department-to-object links are the
 * case, all of them under fund/100 -- and the group they share is the honest
 * answer. Ends in two different groups cannot happen: a link between subtrees
 * would have to cross a fund group boundary, and the fold puts a fund-group
 * node at that boundary. If it ever does, "" falls through to --muted, which
 * says "no single fund group" rather than picking one of the two.
 * @param {LaidLink} link
 * @returns {string}
 */
function linkColor(link) {
  const source = fundGroupOf(link.source);
  const target = fundGroupOf(link.target);
  const group = isFundGroup(link.source) ? link.source.id
    : isFundGroup(link.target) ? link.target.id
    : source === target ? source
    : "";
  const name = /** @type {Record<string,string>} */ (FUND_COLOR_VAR)[group];
  return name ? cssVar(name) : cssVar("--muted");
}

/**
 * @param {LaidNode} node
 * @returns {string}
 */
function nodeColor(node) {
  const name = /** @type {Record<string,string>} */ (FUND_COLOR_VAR)[node.id];
  return name ? cssVar(name) : cssVar("--muted");
}

/**
 * Sort key inside a column: where in the fund column this node's money sits.
 *
 * A fund group is simply its own place in FUND_ORDER. Everything else takes
 * the value-weighted mean position of the fund groups it touches, so a node
 * comes to rest opposite the funds it actually feeds or draws on. That is the
 * barycentre heuristic, and it is roughly what d3 would compute for itself if
 * this file were not overriding it -- which it has to, because supplying a
 * .nodeSort() at all is what pins the fund column to the palette's order, and
 * d3's own pass would reorder it.
 *
 * Sorting on this is worth most of what the chart's legibility was losing. Laid
 * out under node with the vendored d3 and restacked, FY2026 comes to 195 ribbon
 * crossings and $457,434,169 of overlapping ribbon, against 285 and
 * $966,956,035 under a sort by size, 297 under the input order, and 246 under
 * d3's own pass. tools/jscheck re-measures every one of those figures on every
 * run and pins it, so editing this comment without re-measuring fails.
 *
 * An exact search -- one-sided crossing minimisation is solvable for columns
 * this small -- reaches 177 crossings, but spends $491M of overlap doing it, so
 * the two are points on a frontier rather than a right and a wrong answer. A
 * rule that reads the data is worth more here than 18 crossings: it needs no
 * re-derivation when a category is added or the fiscal year rolls over. That
 * search does not live in the tree, so unlike the figures above it is the one
 * number here nothing re-checks.
 *
 * Ties are real and wanted. Three revenue categories touch only the General
 * Fund, so all three score exactly its index and fall to the caller's tie-break
 * on value, which stacks them beside their fund largest first.
 *
 * ON THE DRILL-DOWN THE TIES ARE THE RULE RATHER THAN THE EXCEPTION, and that
 * is a property of the document, not a defect here. All 23 department nodes
 * carry parent: "fund/100", so every one of them scores the General Fund's
 * index exactly and the whole column falls through to size-descending. Said
 * plainly because the bead this landed under (fisc-5miz.3) expected the
 * inheritance to give that column an order, and it does not: there is only one
 * fund group above it to inherit from.
 * @param {LaidNode} node
 * @returns {number}
 */
function nodeRank(node) {
  const fund = FUND_ORDER.indexOf(node.id);
  if (fund >= 0) return fund;

  let weight = 0;
  let place = 0;
  for (const l of node.sourceLinks.concat(node.targetLinks)) {
    const other = l.source === node ? l.target : l.source;
    // The neighbour's fund GROUP, not the neighbour: on the spine every link
    // has a fund-group end and this is the end itself, so the figures below are
    // unchanged. On the drill-down the ends are funds and departments, and
    // without the walk every one of them scores -1 and the column degenerates
    // to the size ordering measured as the worst of the four.
    const at = FUND_ORDER.indexOf(fundGroupOf(other));
    // A node in no fund group is ignored rather than counted as position zero.
    if (at < 0) continue;
    place += at * l.value;
    weight += l.value;
  }
  // A node whose links were all dropped as zero-valued has no position to
  // average. It sorts to the top and its own value breaks the tie.
  return weight === 0 ? 0 : place / weight;
}

/**
 * Citations for a set of source documents: the city's PDF opened at the page,
 * and the committed extraction of that page's text.
 *
 * page_text_base is normally a RELATIVE path into this site: `fisc export`
 * copies the cited pages' committed text into the output tree, so a reader
 * checking provenance loads it from the same origin as the page and needs no
 * third party to be up. Do not assume a scheme, and do not compose it with
 * `new URL(base)` — the browser resolves it against the document for us.
 *
 * When the export was told to cite a remote instead (--source-browse-url), the
 * base is an absolute URL into a browsable copy of the repository: github.com's
 * blob view and never raw.githubusercontent.com. Not for LFS reasons —
 * data/extracted/ is ordinary git and the raw host would serve it fine — but
 * because the blob view is the one a reader can use: the file with line
 * numbers, its history, and the rest of the document beside it. The artifacts
 * are .txt precisely so that view shows them verbatim; markdown would be
 * rendered and the runs of spaces that ARE the printed column grid would
 * collapse.
 *
 * The PDFs are the LFS half of the repository, which is why a PDF citation
 * goes to the city's own URL with #page=N rather than to GitHub at all.
 * @param {FiscSource[]} sources
 * @returns {{label:string, href:string}[]}
 */
function citations(sources) {
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
      // The third shape, and the one a link's own locators resolve through:
      // the fact-store shard holding every record read off this page. Same
      // zero-pad rule as the text file, which is why this lives here rather
      // than in a sibling function -- one place spells the client's half of a
      // citation.
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
function h(tag, className, text) {
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
function link(label, href) {
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
let projection = null;
/** Node id whose flows are isolated, or "" for all of them. */
let isolated = "";
/** @type {LaidNode | LaidLink | null} */
let pinned = null;
/**
 * The node and timestamp of the last Enter/Space activation, so that the click
 * some assistive tech synthesises from that same key press does not undo it.
 * @type {{id:string, at:number}}
 */
let keyActivation = { id: "", at: -Infinity };

/**
 * Every node of the document being laid out, by id, so fundGroupOf can walk
 * node.parent upward.
 *
 * LAYOUT STATE, NOT PAGE STATE. layOut assigns it before anything that can
 * throw, from the document it was handed and nothing else, so it never
 * describes a document other than the one the chart was last laid out from.
 * That is what lets it survive into paint(), which recolours the existing
 * ribbons on a theme change without laying anything out again.
 * @type {Map<string, FiscNode>}
 */
let groupIndex = new Map();

/* ------------------------------------------------------------------ *
 * Chart
 * ------------------------------------------------------------------ */

/**
 * Rebuild a set of doc\u001fpage keys as the FiscSource[] the packager
 * publishes: documents ascending, pages ascending within each, each once.
 *
 * The shape is not incidental. citations() is the client's whole URL
 * vocabulary and it reads metadata.sources and a link's locators with the same
 * code, so a fold that produced a differently-ordered list would make the same
 * page render as a different citation depending on whether the reader was
 * looking at the spine or the drill-down.
 * @param {Set<string>|undefined} keys
 * @returns {FiscSource[]}
 */
function regroupLocators(keys) {
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
 * Folds a document to the tiers this page draws.
 *
 * WHY A DOCUMENT IS FOLDED AT ALL, because it is the whole reason the
 * drill-down has a page. fund-flows.json's fund column is 61 nodes. d3-sankey
 * shrinks nodePadding to fit -- min(14, 796/60) = 13.267 -- and then divides
 * what is left among the values, and what is left is nothing: every node height
 * and every link width comes out at exactly zero. Nor is that a padding
 * problem. At zero padding 24 of the 61 funds are still sub-pixel and 45 are
 * under 8px, because the General Fund alone is 49% of the column; the smallest
 * fund reaches one pixel at a canvas 64,203px tall. The column cannot be drawn,
 * at any height, and folding it to its six fund groups is what makes the
 * document renderable.
 *
 * THE RULE. Each node folds to its nearest ancestor whose tier this page draws,
 * following node.parent. Links fold with their ends and merge on the folded
 * pair, summing values and unioning fact ids. A link whose ends fold to the
 * SAME node is dropped: it was a flow inside what is now one box. That is the
 * drill-down's department-to-object links, which fold to fund/100 -> fund/100 --
 * docs/general-fund-drilldown-contract.md warns about exactly this shape -- and
 * dropping them cites nothing away, because the fund-to-department link that
 * survives carries the same money AND the same facts, over every cell including
 * the printed zeros. That is what facts_cited_twice counts.
 *
 * IT FAILS CLOSED ON A NODE IT CANNOT PLACE. A node with no drawn ancestor
 * means the tier set does not describe this document, and the two ways of
 * carrying on are both worse than stopping: drop it and the page silently loses
 * a column, keep it and it has no column to be drawn in. The throw reaches
 * showYear's caller and paints a banner over a page that is still internally
 * consistent, which is the same contract layOut has.
 *
 * @param {FiscProjection} doc
 * @returns {FiscProjection} doc itself when this page draws every tier.
 */
function foldDocument(doc) {
  if (!RENDER_TIERS.length) return doc;
  const byID = new Map(doc.nodes.map((n) => [n.id, n]));
  const drawn = new Set(RENDER_TIERS);

  /** @type {Map<string,string>} */
  const foldsTo = new Map();
  for (const n of doc.nodes) {
    let at = n;
    for (let hops = 0; !drawn.has(at.tier); hops++) {
      const up = at.parent ? byID.get(at.parent) : undefined;
      if (!up || hops > 8) {
        throw new Error("cannot draw " + doc.projection + ": node " + n.id +
          " is tier " + n.tier + " and no ancestor of it is a tier this page draws (" +
          RENDER_TIERS.join(", ") + ")");
      }
      at = up;
    }
    foldsTo.set(n.id, at.id);
  }

  /** @type {Map<string, FiscLink>} */
  const merged = new Map();
  /** @type {Map<string, Set<string>>} */
  const cited = new Map();
  // Locators fold with their ends exactly as fact_ids do, keyed doc\u001fpage so
  // two legs read off ONE page collapse to one locator -- something the
  // fact-id union cannot show, because two facts on one page are two ids.
  // Without this a merged ribbon would carry the FIRST leg's locators, copied
  // by the Object.assign below, and cite a strict subset of the pages its
  // figure was read from.
  /** @type {Map<string, Set<string>>} */
  const located = new Map();
  /** @param {FiscSource[]} ss @returns {string[]} */
  const locatorKeys = (ss) => {
    const out = [];
    for (const s of ss || []) {
      for (const p of s.pages) out.push(s.doc_id + "\u001f" + p);
    }
    return out;
  };
  for (const l of doc.links) {
    const source = foldsTo.get(l.source);
    const target = foldsTo.get(l.target);
    // d3-sankey throws on a link naming a node the document does not carry;
    // this is the same fault one step earlier, with the id in the message.
    if (!source || !target) {
      throw new Error("cannot draw " + doc.projection + ": link " + l.source +
        " -> " + l.target + " names a node the document does not carry");
    }
    if (source === target) continue;
    const key = source + "\u001f" + target;
    const at = merged.get(key);
    const ids = cited.get(key);
    if (!at || !ids) {
      merged.set(key, Object.assign({}, l, { source: source, target: target }));
      cited.set(key, new Set(l.fact_ids));
      located.set(key, new Set(locatorKeys(l.locators)));
      continue;
    }
    // Two links of different kinds folding onto one ribbon would leave that
    // ribbon's tooltip and table row naming a kind that is true of only part of
    // it. It does not occur in any published column; if it ever does, stop.
    if (at.kind !== l.kind) {
      throw new Error("cannot draw " + doc.projection + ": " + source + " -> " + target +
        " folds together a " + at.kind + " flow and a " + l.kind + " one");
    }
    // A PRINTED LEG AND AN INFERRED ONE CANNOT MERGE, for the same reason two
    // kinds cannot, and it is this project's oldest rule: published is not
    // derived. OR-ing the flag draws the merged ribbon dashed and lists its
    // WHOLE amount under "what we inferred", which is a false statement about a
    // figure the city printed most of. Latent -- all four published columns
    // carry zero derived links -- and refused rather than left to the day one
    // does. Found by /code-review, 2026-08-28.
    if (at.derived !== l.derived) {
      throw new Error("cannot draw " + doc.projection + ": " + source + " -> " + target +
        " folds together a printed flow and an inferred one, which cannot be drawn as one mark");
    }
    at.value_cents += l.value_cents;
    // A transfer id names one leg of one transfer and cannot survive a merge.
    if (at.transfer_id !== l.transfer_id) at.transfer_id = "";
    for (const id of l.fact_ids) ids.add(id);
    // The union runs AFTER the kind and derived guards above, so a leg that
    // cannot be merged throws with its own message rather than dying on a
    // locators field the fixture happened not to carry.
    const locs = located.get(key);
    if (locs) for (const k of locatorKeys(l.locators)) locs.add(k);
  }

  const links = Array.from(merged.entries())
    .map(([key, l]) => Object.assign(l, {
      fact_ids: Array.from(cited.get(key) || []).sort(),
      // Rebuilt into the SAME shape internal/project publishes -- documents
      // ascending, pages ascending within each -- so a folded link and an
      // unfolded one are indistinguishable to citations().
      locators: regroupLocators(located.get(key)),
    }))
    .sort((a, b) => (a.source < b.source ? -1 : a.source > b.source ? 1
      : a.target < b.target ? -1 : a.target > b.target ? 1 : 0));

  // A node the folded links do not touch is not drawable: d3-sankey gives a
  // zero-degree node depth 0 and value 0, so it lands in the first column as a
  // labelled rectangle of no height. The unfolded drill-down carries six of
  // them -- the fund groups exist to carry the hierarchy, not a flow -- and
  // after the fold every one of them is touched, so on the documents this
  // project publishes today the filter removes nothing. It is here because
  // "every node is drawable" is a property of the fold, not of the data.
  const touched = new Set();
  for (const l of links) {
    touched.add(l.source);
    touched.add(l.target);
  }
  // A RETAINED NODE'S parent IS RE-POINTED AT ITS OWN FOLDED ANCESTOR, because
  // a folded document has to be as well-formed as the one it came from. Left
  // alone, dept/administrative-services still claims parent "fund/100" -- a
  // node the fold just removed -- and every reader of the hierarchy silently
  // gets nothing: fundGroupOf's walk stops at the first unresolvable parent, so
  // the inheritance this same change added to linkColor and nodeRank would be
  // dead on the one document it was added for. This is the client-side twin of
  // node-hierarchy-well-formed, which asserts Go-side that every parent
  // resolves within its own document.
  //
  // A node whose parent folded INTO IT has no parent left to name, and says so.
  //
  // THE WALK CONTINUES PAST AN ANCESTOR THE FILTER DROPPED, which is the whole
  // reason this is a loop rather than one lookup. A fund group with no flows of
  // its own is removed above while a node beneath it survives -- a division
  // whose fund group takes in nothing but which is itself paid by another
  // group -- and re-pointing at it would leave the folded document naming a
  // node it does not carry, which is exactly the dead-inheritance failure this
  // re-pointing exists to prevent. Found by /code-review, 2026-08-28.
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
 * d3-sankey does this itself, but too early to be right. Its relaxation loop
 * sorts a node's links by where the other end sits and then moves nodes again,
 * and the last move is never followed by another sort, so a node can be left
 * handing its ribbons out in an order its neighbours no longer sit in. The
 * result is a pair of ribbons that cross immediately at the node face, for no
 * reason in the data -- 14 of them on FY2026. tools/jscheck counts them before
 * this function runs, and counts the 14 CROSSINGS they cost, which falls from
 * 209 to 195. It deliberately does not re-count the pairs afterwards: this
 * function sorts by the same key that count is derived from, so zero after is a
 * tautology and would assert nothing. Redoing the sort against the final
 * positions is the whole fix.
 *
 * Widths are not touched, only the order they are stacked in, so each node's
 * ribbons still fill exactly its own height.
 * @param {{nodes:LaidNode[], links:LaidLink[]}} graph
 */
function restackLinks(graph) {
  /** @param {(l:LaidLink) => LaidNode} end */
  const byOtherEnd = (end) =>
    /** @param {LaidLink} a @param {LaidLink} b */ (a, b) =>
      end(a).y0 - end(b).y0 || a.index - b.index;

  for (const node of graph.nodes) {
    node.sourceLinks.sort(byOtherEnd((l) => l.target));
    node.targetLinks.sort(byOtherEnd((l) => l.source));
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
 * Lays a document out, touching nothing on the page.
 *
 * IT IS SEPARATE FROM render() SO THAT A DOCUMENT WHICH WILL NOT DRAW CANNOT
 * LEAVE THE PAGE SHOWING TWO FISCAL YEARS AT ONCE. Everything in the draw that
 * can throw is here: d3-sankey rejects a link naming a node the document does
 * not carry, and restackLinks walks what it returns. While this ran inside
 * render() -- after paintYearWords, buildLegend and buildTable had already
 * repainted -- a throw left the new year's tiles, caveats, lede and <title>
 * over the OLD year's chart, with a citation link pointing at the year that was
 * not drawn (fisc-bsg).
 *
 * So showYear lays out FIRST, while the page is still wholly the previous year,
 * and only then repaints. A throw here propagates out of showYear to the change
 * handler's .catch, which paints a banner over a page that is still internally
 * consistent. Re-ordering the repaint alone would not have done it: a throw
 * after buildLegend leaves the page split the other way.
 *
 * @param {FiscProjection} doc
 */
function layOut(doc) {
  // Assigned from the document being laid out, before anything that can throw,
  // so fundGroupOf never walks a parent chain belonging to another document.
  groupIndex = new Map(doc.nodes.map((n) => [n.id, n]));

  // WHICH COLUMN A NODE IS DRAWN IN IS A PROPERTY OF ITS TIER ONCE THIS PAGE
  // FOLDS. sankeyJustify aligns link-less sinks to the LAST column, which is
  // right for a document drawn whole and wrong the moment tiers can be skipped:
  // a node terminating early is shoved across the chart to sit among nodes it
  // shares nothing with. The spine is drawn whole and keeps sankeyJustify
  // exactly, which is why the crossing figures pinned in tools/jscheck do not
  // move.
  const align = RENDER_TIERS.length
    ? /** @param {LaidNode} d */ (d) => RENDER_TIERS.indexOf(d.tier)
    : D3.sankeyJustify;

  const sankey = D3.sankey()
    .nodeId(/** @param {LaidNode} d */ (d) => d.id)
    .nodeWidth(NODE_WIDTH)
    .nodePadding(NODE_PADDING)
    .nodeAlign(align)
    // Supplying this switches d3's own ordering pass off, which is what makes
    // the fund column's colour adjacency a property of the page rather than of
    // the library. nodeRank puts the crossing count back.
    .nodeSort(/** @param {LaidNode} a @param {LaidNode} b */ (a, b) =>
      nodeRank(a) - nodeRank(b) || b.value - a.value)
    .extent([[LABEL_GUTTER, 12], [CHART_WIDTH - LABEL_GUTTER, CHART_HEIGHT - 12]]);

  // d3-sankey mutates its input, so it gets a copy and the fetched document
  // stays the thing the table and the detail panel read from.
  /** @type {{nodes:LaidNode[], links:LaidLink[]}} */
  const graph = sankey({
    nodes: doc.nodes.map((n) => Object.assign({}, n)),
    links: doc.links.map((l) => Object.assign({}, l, { value: l.value_cents })),
  });
  restackLinks(graph);
  return graph;
}

/**
 * Draws a laid-out graph. Pass the result of layOut(); omitted, it lays the
 * current projection out itself, which is the non-atomic path and is only for
 * a caller that has nothing else on the page to keep consistent.
 * @param {{nodes:LaidNode[], links:LaidLink[]}} [laid]
 */
function render(laid) {
  if (!projection) return;
  const graph = laid || layOut(projection);
  const svg = D3.select("#chart");
  const width = CHART_WIDTH;
  const height = CHART_HEIGHT;

  // No width or height attributes: the viewBox plus width:100% in the
  // stylesheet is what makes the drawing scale with its container.
  svg.attr("viewBox", "0 0 " + width + " " + height);
  svg.selectAll("g").remove();

  const gLinks = svg.append("g").attr("class", "links");
  const gNodes = svg.append("g").attr("class", "nodes");

  gLinks.selectAll("path")
    .data(graph.links)
    .join("path")
    .attr("class", /** @param {LaidLink} d */ (d) => "link" + (d.derived ? " derived" : ""))
    .attr("d", D3.sankeyLinkHorizontal())
    // The 2px surface gap is the separator between stacked ribbons; a stroke
    // drawn around each one would be data-weight ink doing white's job.
    .attr("stroke-width", /** @param {LaidLink} d */ (d) => Math.max(1, d.width - RIBBON_GAP))
    .attr("tabindex", 0)
    .attr("role", "button")
    .attr("aria-label", /** @param {LaidLink} d */ (d) => linkDescription(d))
    .on("pointerenter", /** @param {PointerEvent} e @param {LaidLink} d */ (e, d) => showTip(e, d))
    .on("pointermove", /** @param {PointerEvent} e @param {LaidLink} d */ (e, d) => showTip(e, d))
    .on("pointerleave", hideTip)
    .on("focus", /** @param {FocusEvent} e @param {LaidLink} d */ (e, d) => { showTip(e, d); pin(d); })
    .on("blur", hideTip)
    .on("click", /** @param {MouseEvent} e @param {LaidLink} d */ (e, d) => { e.stopPropagation(); pin(d); });

  const node = gNodes.selectAll("g")
    .data(graph.nodes)
    .join("g")
    .attr("class", /** @param {LaidNode} d */ (d) => "node" + (d.derived ? " derived" : ""))
    .attr("tabindex", 0)
    .attr("role", "button")
    // The isolation is a toggle, and the legend announces its copy of it the
    // same way. applyEmphasis keeps this in step.
    .attr("aria-pressed", "false")
    .attr("aria-label", /** @param {LaidNode} d */ (d) => nodeDescription(d))
    .on("pointerenter", /** @param {PointerEvent} e @param {LaidNode} d */ (e, d) => showTip(e, d))
    .on("pointermove", /** @param {PointerEvent} e @param {LaidNode} d */ (e, d) => showTip(e, d))
    .on("pointerleave", hideTip)
    .on("focus", /** @param {FocusEvent} e @param {LaidNode} d */ (e, d) => { showTip(e, d); pin(d); })
    .on("blur", hideTip)
    // Activating a node isolates its flows, the same toggle the legend does
    // for a fund group. Layout gets this chart down to 195 ribbon crossings and
    // no further -- the rest are structural in a graph this dense -- so the way
    // through them is to take one flow out at a time.
    //
    // Both paths are here because neither covers everyone. An SVG
    // g[role=button] does not synthesise a click from Enter the way a real
    // button does, so click alone leaves the toggle mouse-only. Screen
    // readers vary: some pass the key through and synthesise nothing, some
    // synthesise a click and swallow the key, and some do both -- and that
    // last case would fire the toggle twice and land back where it started,
    // for exactly the readers the keydown was added for.
    //
    // Hence the guard, which is on the activation and not on the input
    // device: a click on the node a key just activated is that key's own
    // click. Every other click still toggles, including one synthesised by
    // assistive tech that sent no key at all.
    //
    // Focus itself must not isolate. Tabbing the columns would strobe the
    // whole chart, which is also why a held key is ignored.
    .on("click", /** @param {MouseEvent} e @param {LaidNode} d */ (e, d) => {
      e.stopPropagation();
      pin(d);
      const echo = d.id === keyActivation.id && e.timeStamp - keyActivation.at < 500;
      if (!echo) setIsolated(isolated === d.id ? "" : d.id);
    })
    .on("keydown", /** @param {KeyboardEvent} e @param {LaidNode} d */ (e, d) => {
      if (e.key !== "Enter" && e.key !== " ") return;
      if (e.repeat) return;
      e.preventDefault();
      keyActivation = { id: d.id, at: e.timeStamp };
      // Escape unpins while leaving focus where it was, so the panel can be
      // empty here even though focus already pinned this node once.
      pin(d);
      setIsolated(isolated === d.id ? "" : d.id);
    });

  node.append("rect")
    .attr("x", /** @param {LaidNode} d */ (d) => d.x0)
    .attr("y", /** @param {LaidNode} d */ (d) => d.y0)
    .attr("width", /** @param {LaidNode} d */ (d) => d.x1 - d.x0)
    .attr("height", /** @param {LaidNode} d */ (d) => Math.max(2, d.y1 - d.y0))
    .attr("rx", 2);

  // Every node is directly labelled. That is the relief the palette's
  // contrast check requires, and it is why the chart still reads for someone
  // who cannot separate two of the hues.
  const label = node.append("text")
    .attr("class", "halo")
    .attr("y", /** @param {LaidNode} d */ (d) => (d.y0 + d.y1) / 2)
    .attr("dy", "0.35em")
    .attr("x", /** @param {LaidNode} d */ (d) => (d.depth === 0 ? d.x0 - 10 : d.x1 + 10))
    .attr("text-anchor", /** @param {LaidNode} d */ (d) => (d.depth === 0 ? "end" : "start"));

  label.append("tspan").text(/** @param {LaidNode} d */ (d) => d.label);
  label.append("tspan")
    .attr("class", "value")
    .text(/** @param {LaidNode} d */ (d) => "  " + fmtShort(d.value));
  label.append("tspan")
    .attr("class", "flag")
    // A short marker, not the word: the label is already at the edge of its
    // gutter. The dashed outline, the legend, the tooltip and the table all
    // spell out what the diamond means.
    .text(/** @param {LaidNode} d */ (d) => (d.derived ? "  ◇" : ""));

  paint();
  applyEmphasis();
}

/** Re-reads the palette from CSS and repaints. Called after a theme change. */
function paint() {
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
 * Every path into this state goes through here -- the legend, a click on a
 * node, Escape -- because the legend's pressed button and the dimming are two
 * renderings of the same one variable, and the two drift apart the moment
 * either is set on its own. Isolating a fund group by clicking its node has to
 * light its legend button; isolating a revenue node has to clear whichever
 * button was lit.
 * @param {string} id
 */
function setIsolated(id) {
  isolated = id;
  for (const element of el("legend").querySelectorAll("button")) {
    const button = /** @type {HTMLElement} */ (element);
    const pressed = isolated !== "" && button.dataset.node === isolated;
    button.setAttribute("aria-pressed", String(pressed));
  }
  applyEmphasis();
}

/** Applies the isolation and the pinned selection to every mark. */
function applyEmphasis() {
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
 * @param {LaidLink} d
 * @returns {string}
 */
function linkDescription(d) {
  return d.source.label + " to " + d.target.label + ", " + fmt(d.value_cents) + ", " +
    (/** @type {Record<string,string>} */ (KIND_LABEL)[d.kind] || d.kind) +
    (d.derived ? ", inferred by us" : ", printed by the city");
}

/**
 * @param {LaidNode} d
 * @returns {string}
 */
function nodeDescription(d) {
  return d.label + ", total " + fmt(d.value) +
    (d.derived ? ", inferred by us" : ", printed by the city");
}

/**
 * @param {LaidLink | LaidNode} d
 * @returns {boolean}
 */
function isLink(d) {
  return Object.prototype.hasOwnProperty.call(d, "fact_ids");
}

/**
 * @param {MouseEvent | FocusEvent} event
 * @param {LaidLink | LaidNode} d
 */
function showTip(event, d) {
  const tip = el("tooltip");
  tip.replaceChildren();

  const asLink = isLink(d);
  const value = asLink ? /** @type {LaidLink} */ (d).value_cents : /** @type {LaidNode} */ (d).value;
  const color = asLink ? linkColor(/** @type {LaidLink} */ (d)) : nodeColor(/** @type {LaidNode} */ (d));

  // Values lead, labels follow: here the reader already knows what they are
  // pointing at and wants the number.
  tip.append(h("div", "tip-value", fmt(value)));

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
    meta.append(h("span", l.derived ? "chip derived" : "chip", l.derived ? "◇ inferred" : "printed"));
    tip.append(meta);
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
    tip.append(meta);
    if (n.rationale) tip.append(h("div", "tip-meta", n.rationale));
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

function hideTip() {
  el("tooltip").hidden = true;
}

/**
 * Pins a flow or node into the detail panel, which is where the provenance
 * links live: a tooltip you cannot click is no place for a citation.
 * @param {LaidLink | LaidNode} d
 */
function pin(d) {
  pinned = d;
  const panel = el("detail");
  panel.replaceChildren();
  if (!projection) return;

  const asLink = isLink(d);
  const value = asLink ? /** @type {LaidLink} */ (d).value_cents : /** @type {LaidNode} */ (d).value;

  panel.append(h("div", "amount", fmt(value)));
  panel.append(h("div", "", asLink
    ? /** @type {LaidLink} */ (d).source.label + " → " + /** @type {LaidLink} */ (d).target.label
    : /** @type {LaidNode} */ (d).label));

  const chips = h("div", "prov");
  if (asLink) {
    const l = /** @type {LaidLink} */ (d);
    chips.append(h("span", "chip", /** @type {Record<string,string>} */ (KIND_LABEL)[l.kind] || l.kind));
    chips.append(h("span", l.derived ? "chip derived" : "chip", l.derived ? "◇ our inference" : "printed by the city"));
    panel.append(chips);
    const ids = h("div", "facts", "Facts: " + l.fact_ids.join(" "));
    panel.append(ids);
  } else {
    const n = /** @type {LaidNode} */ (d);
    chips.append(h("span", "chip", n.role.replace(/_/g, " ")));
    if (n.constraint_tier) chips.append(h("span", "chip", "constraint: " + n.constraint_tier));
    chips.append(h("span", n.derived ? "chip derived" : "chip", n.derived ? "◇ our inference" : "printed by the city"));
    panel.append(chips);
    if (n.rationale) panel.append(h("p", "why", n.rationale));
    if (n.source_note) panel.append(h("p", "subtle", n.source_note));
  }

  // THE CITATIONS ARE THE MARK'S OWN WHEN THE MARK HAS ANY, and the whole
  // document's otherwise.
  //
  // A LINK cites its locators: the pages its own facts were read from. Before
  // this, pinning any mark rendered the same list -- on the drill-down that is
  // 18 pages x 2 shapes, identical for every one of the 52 ribbons, which
  // tells a reader where the CHART came from and nothing about the flow they
  // clicked.
  //
  // A NODE keeps the document's, and that asymmetry is stated rather than left
  // to be noticed: a node is an aggregation point and cites no facts, so there
  // is nothing narrower to show.
  //
  // The label stays "Sources:" and not "Records:" -- citations() emits PDF,
  // extracted-text AND records anchors, so naming it for the last would name a
  // third of the row.
  const prov = h("div", "prov");
  prov.append(h("span", "subtle", "Sources:"));
  for (const c of citations(asLink ? /** @type {LaidLink} */ (d).locators : projection.metadata.sources)) {
    prov.append(link(c.label, c.href));
  }
  panel.append(prov);
  applyEmphasis();
}

/* ------------------------------------------------------------------ *
 * Legend, derived list, table
 * ------------------------------------------------------------------ */

function buildLegend() {
  if (!projection) return;
  const legend = el("legend");
  legend.replaceChildren();
  const byID = new Map(projection.nodes.map((n) => [n.id, n]));
  for (const id of FUND_ORDER) {
    const node = byID.get(id);
    if (!node) continue;
    const button = document.createElement("button");
    button.type = "button";
    button.dataset.node = id;
    button.setAttribute("aria-pressed", "false");
    const key = h("span", "key");
    key.dataset.var = /** @type {Record<string,string>} */ (FUND_COLOR_VAR)[id];
    button.append(key);
    button.append(document.createTextNode(node.label));
    button.addEventListener("click", () => setIsolated(isolated === id ? "" : id));
    legend.append(button);
  }
}

function buildDerivedList() {
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
    const flows = links.filter((l) => l.source === n.id || l.target === n.id);
    if (flows.length) {
      const total = flows.reduce((sum, l) => sum + l.value_cents, 0);
      li.append(h("div", "subtle",
        flows.length + " flow" + (flows.length === 1 ? "" : "s") + " totalling " + fmt(total) + ": " +
        flows.map((l) => (labels.get(l.source) || l.source) + " → " + (labels.get(l.target) || l.target)).join("; ")));
    }
    list.append(li);
  }
  // A link can be derived while both its endpoints are published, so the
  // orphans have to be listed too or the page would claim nothing was inferred
  // while drawing an inferred flow. Every derived link touching a derived node
  // is already accounted for above.
  const named = new Set(nodes.map((n) => n.id));
  const orphans = links.filter((l) => !named.has(l.source) && !named.has(l.target));
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

function buildTable() {
  if (!projection) return;
  const body = el("flow-table").querySelector("tbody");
  if (!body) return;
  body.replaceChildren();
  const labels = new Map(projection.nodes.map((n) => [n.id, n.label]));

  for (const l of projection.links) {
    const tr = document.createElement("tr");
    tr.append(h("td", "", labels.get(l.source) || l.source));
    tr.append(h("td", "", labels.get(l.target) || l.target));
    tr.append(h("td", "num", fmt(l.value_cents)));
    tr.append(h("td", "", /** @type {Record<string,string>} */ (KIND_LABEL)[l.kind] || l.kind));
    tr.append(h("td", "", l.derived ? "◇ inferred" : "printed"));
    tr.append(h("td", "ids", l.fact_ids.join(" ")));
    // PER ROW, not per document. The column header says "Source" and until
    // this it printed the same 36 anchors on all 52 drill-down rows -- a
    // Source column that is the same for every row is a lie by repetition.
    // Now it is the pages that row's own figure was read from: 237 anchors
    // over the whole table, each about the row it sits in. The links read
    // projection.links, which is the FOLDED document (see showYear), so the
    // fold's locator union is what makes these complete.
    const td = h("td", "");
    for (const c of citations(l.locators)) {
      td.append(link(c.label, c.href));
      td.append(document.createTextNode(" "));
    }
    tr.append(td);
    body.append(tr);
  }
}

/* ------------------------------------------------------------------ *
 * Theme
 * ------------------------------------------------------------------ */

/**
 * Whether the page is currently dark: the stamped theme wins, and the OS
 * setting decides when there is none.
 *
 * matchMedia is feature-checked rather than assumed. It is missing in some
 * embedded and headless renderers, and a theme toggle is no reason for the
 * chart not to draw.
 * @returns {boolean}
 */
function prefersDark() {
  const stamped = document.documentElement.dataset.theme;
  if (stamped === "dark") return true;
  if (stamped === "light") return false;
  return typeof window.matchMedia === "function" &&
    window.matchMedia("(prefers-color-scheme: dark)").matches;
}

/**
 * Brings the theme button's label and aria-pressed back into agreement with the
 * page, and it is a file-scope function rather than a closure inside wireTheme
 * for one reason: the OS-theme listener in main() has to be able to call it.
 *
 * WHILE IT WAS A CLOSURE THE BUTTON INVERTED UNDER AN OS SWITCH. A reader with
 * nothing in localStorage opens in light: prefersDark() is false, so the button
 * reads "Dark mode" with aria-pressed="false". The OS switches to dark at
 * sunset; the stylesheet's prefers-color-scheme rule darkens the page and
 * paint() re-reads the palette -- but the listener was wired to paint alone, so
 * the button still announces aria-pressed="false" on a dark page, and because
 * prefersDark() now returns true, clicking the control labelled "Dark mode"
 * makes the page LIGHT.
 */
function syncTheme() {
  const button = maybeEl("theme-toggle");
  if (!button) return;
  const dark = prefersDark();
  button.setAttribute("aria-pressed", String(dark));
  button.textContent = dark ? "Light mode" : "Dark mode";
}

function wireTheme() {
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
 * Boot
 * ------------------------------------------------------------------ */

/** Returns the provenance panel to its unpinned state. */
function resetDetail() {
  const panel = el("detail");
  panel.replaceChildren();
  panel.append(h("p", "subtle", "Select a flow or a node to pin its provenance here."));
}

/**
 * Refuses to draw, visibly.
 *
 * The provenance panel alone is not enough: it sits below the chart, and a
 * reader who never scrolls past an empty diagram would read the blank space as
 * "still loading" rather than as "this page declined to render". So the message
 * also goes into a banner at the top of the content, with role="alert" so it is
 * announced rather than merely present. A console line would be worse still —
 * it is invisible to everyone the page is for.
 *
 * @param {string} message
 */
function fail(message) {
  const panel = el("detail");
  panel.replaceChildren();
  panel.append(h("p", "", message));

  const content = document.querySelector("main");
  if (content) {
    const banner = h("div", "refusal", message);
    banner.setAttribute("role", "alert");
    // Replace rather than stack: two refusals in one visit are one story, and
    // a second bar under the first reads as two separate faults.
    const existing = content.querySelector(".refusal");
    if (existing) existing.remove();
    content.prepend(banner);
  }
}

/**
 * Removes the refusal banner, if one is showing.
 *
 * fail() was terminal when it was written -- the page gave up and the banner
 * stayed for the visit -- so nothing ever needed to take one down. A year switch
 * can recover from a failed one, and a stale role="alert" sitting above a chart
 * that did draw is the page asserting something untrue about what the reader is
 * looking at.
 */
function clearRefusal() {
  const content = document.querySelector("main");
  if (!content) return;
  const existing = content.querySelector(".refusal");
  if (existing) existing.remove();
}

/**
 * Reports whether a document is one this client understands, refusing visibly
 * when it is not.
 *
 * Note which way this fails: it returns false and the caller stops, rather
 * than drawing what it can. A partial chart of public money asserts the part
 * it drew is the whole, which is the same class of false claim as a wrong
 * total.
 *
 * @param {number} got the document's schema_version
 * @param {string} what what to name in the message
 * @returns {boolean}
 */
function understands(got, what) {
  if (got === SCHEMA_VERSION) return true;
  const why = got > SCHEMA_VERSION
    ? "The data is newer than this page. If you have visited before, a cached copy of " +
      "app.js may be the cause; reload to pick up the current one."
    : "The data is older than this page.";
  fail(
    "This page will not draw " + what + ": it declares schema_version " + got +
    ", and this page renders schema_version " + SCHEMA_VERSION + ". " + why +
    " Drawing it anyway would produce a chart that is wrong rather than one that fails."
  );
  return false;
}

/**
 * Reports whether a fetched body is a document at all.
 * @param {any} doc
 * @param {string} what
 */
function isDocument(doc, what) {
  if (doc && typeof doc === "object") return true;
  // SEPARATE FROM drawableSankey AND RUN BEFORE understands(), because
  // understands takes doc.schema_version and would dereference a null first --
  // which is a real answer from a server: HTTP 200 with the body `null` parses
  // fine. While this check lived inside drawableSankey it could never run, and
  // the reader got "TypeError: Cannot read properties of null" instead of a
  // sentence.
  fail(
    "This page will not draw " + what + ": the file is not a document at all. " +
    "It is most likely an error page served with a success status. Nothing on " +
    "the page was changed."
  );
  return false;
}

/**
 * Reports whether a sankey document carries the shape its schema_version
 * promises, refusing visibly if it does not.
 *
 * IT IS DELIBERATELY NOT PART OF understands(). That function takes a version
 * NUMBER and every word of its refusal is about version skew -- "the data is
 * newer than this page", "reload to pick up the current one". A document at the
 * right version that is simply truncated would get a message that is false, and
 * the reader would go clear a cache that was never the problem. Two different
 * failures, two different sentences. It runs AFTER understands for the same
 * reason: a schema_version 2 document may legitimately have none of these keys,
 * and telling its reader the file is truncated would be the wrong diagnosis.
 *
 * WHAT IT BUYS IS THE SENTENCE, AND NOT THE ATOMICITY -- measured, because the
 * two are easy to conflate. Laying out before repainting (see layOut) is what
 * keeps the page whole for nodes and links: `nodes.map` throws inside layOut, so
 * with those two arms deleted the page STILL refuses without a split repaint.
 * What changes is what the reader is told, and tools/jscheck asserts the wording
 * for that reason.
 *
 * THE buildTable KEYS ARE THE EXCEPTION AND ARE WHY THIS LIST IS NOT A GUESS.
 * Three of them are NOT covered by layOut, and every one is reached from
 * buildTable, which is the LAST step of the repaint, so a throw lands after
 * paintYearWords, buildLegend and buildDerivedList have run, leaving the page
 * reading one year over another year's chart. That is fisc-bsg exactly, reached
 * one function past its fix, and it survived a commit because tools/jscheck
 * planted no <tbody> and so buildTable returned at its first line in every
 * lifecycle check.
 *
 *   links[].fact_ids          buildTable, `l.fact_ids.join(" ")`
 *   links[].locators          buildTable, `citations(l.locators)`
 *   links[].locators[].pages  citations(), `for (const page of source.pages)`
 *
 * links[].fact_ids was missing while this comment already stated the rule
 * below, which is fisc-60r: a schema_version 1 document whose links lack
 * fact_ids passed here AND passed layOut -- neither touches the key -- and
 * threw inside buildTable.
 *
 * TWO OF THE FOUR ARMS ARE NOW KEPT FOR A WEAKER REASON, and saying so is the
 * point of a list that claims not to be a guess. metadata.sources and
 * metadata.sources[].pages used to be reached from buildTable, through
 * citations(projection.metadata.sources). They are not any more: fisc-5hxr
 * moved the flow table onto each link's OWN locators, and the only remaining
 * reader of the document-scope list is pin(), for a node mark. pin runs on a
 * click, after the repaint has finished, so a throw there breaks the detail
 * panel rather than leaving one year's words over another year's chart. The
 * arms stay -- a panel that throws at a reader is still a defect the gate can
 * name in words -- but they are no longer fisc-bsg cases and must not be cited
 * as though they were. links[].locators IS one: buildTable dereferences it on
 * every row of every repaint.
 *
 * The per-element arms cost one scan each of links and sources, both of which
 * the repaint already walks more than once.
 *
 * THE RULE THIS LIST FOLLOWS, then: every key the draw DEREFERENCES before it
 * could report a failure. Not every key the contract names -- a client that
 * re-validated the whole document would be a second implementation of
 * `fisc verify` -- and not fewer, or the gate is decorative. A document whose
 * links name nodes it does not carry still passes here and throws in layOut,
 * correctly, because that is what the last-resort .catch is for.
 * @param {any} doc
 * @param {string} what
 */
function drawableSankey(doc, what) {
  const missing = [];
  if (!Array.isArray(doc.nodes)) missing.push("nodes");
  if (!Array.isArray(doc.links)) missing.push("links");
  else if (doc.links.some((l) => !Array.isArray(l.fact_ids))) missing.push("links[].fact_ids");
  else if (doc.links.some((l) => !Array.isArray(l.locators))) missing.push("links[].locators");
  else if (doc.links.some((l) => l.locators.some((s) => !Array.isArray(s.pages)))) {
    // ONE ELEMENT DEEPER, for the same reason metadata.sources[].pages is:
    // citations() does `for (const page of source.pages)` and a locator
    // carrying only a doc_id passes every arm above. On the spine there is no
    // fold and layOut never touches locators, so such a document reaches
    // buildTable and throws there -- after paintYearWords, buildLegend and
    // buildDerivedList have repainted. That is fisc-bsg exactly, and it is the
    // arm fisc-5hxr forgot for the key it introduced.
    missing.push("links[].locators[].pages");
  }
  if (!doc.metadata || typeof doc.metadata !== "object") missing.push("metadata");
  else if (!Array.isArray(doc.metadata.sources)) missing.push("metadata.sources");
  else if (doc.metadata.sources.some((s) => !Array.isArray(s.pages))) {
    missing.push("metadata.sources[].pages");
  }
  if (!missing.length) return true;
  // THE THIRD CAUSE IS NAMED BECAUSE IT IS THE LIKELIEST AND THE ONLY ONE THE
  // READER CAN FIX. The site publishes no cache-busting on data/<stem>.json and
  // the year documents are fetched lazily on click, so a browser can hold a
  // pre-deploy document beside a post-deploy app.js -- and every key this gate
  // has gained since launch reaches the reader that way first. Telling them the
  // file is truncated when their copy is merely old sends them to file a bug
  // about a file that is fine.
  fail(
    "This page will not draw " + what + ": it declares schema_version " +
    SCHEMA_VERSION + ", which promises " + missing.join(", ") + ", and the file " +
    "does not carry " + (missing.length === 1 ? "it" : "them") + ". Your browser " +
    "may be holding a copy from before the last update — reload the page. " +
    "Otherwise the file is truncated or is not the document this page expected. " +
    "Nothing on the page was changed."
  );
  return false;
}

/**
 * What one showYear attempt came to.
 *
 * THREE OUTCOMES AND NOT A BOOLEAN, because "did not draw" was two different
 * facts wearing one answer and the caller could not tell them apart. SUPERSEDED
 * means a later switch took over and this attempt stood down, which is a normal
 * thing that happens whenever a reader clicks twice; FAILED means the year could
 * not be shown and the reader has been told. Reporting `false` for both is what
 * let a superseded opening fetch read as a page that had given up (fisc-8cg).
 */
const DREW = "drew";
const SUPERSEDED = "superseded";
const FAILED = "failed";

/**
 * Fetches and draws one published year.
 *
 * Everything the page says in WORDS comes from CONFIG.years, which the packager
 * built in Go for every year. This function composes no figure and no caveat of
 * its own: doing so would put the prose in two languages and let a tile disagree
 * with the chart beneath it about the same schedule.
 *
 * @param {FiscYear} year
 * @returns {Promise<string>} DREW, SUPERSEDED or FAILED
 */
let switching = 0;

async function showYear(year) {
  // A switch token, because two switches can be in flight at once: a reader who
  // clicks twice gets two fetches, and without this the SLOWER one wins and the
  // page draws a year the control does not show. Compared after every await.
  const token = ++switching;

  // BOTH AWAITS ARE INSIDE THE TRY. response.json() used to sit outside it, so
  // a 200 with a truncated or malformed body rejected out of this function
  // entirely -- into main()'s .catch on the opening path, and into nothing at
  // all from the year control, which is a page half-repainted between two years
  // with no banner.
  let doc;
  try {
    const response = await fetch(year.path);
    if (token !== switching) return SUPERSEDED;
    if (!response.ok) {
      fail("Could not load " + year.path + ": HTTP " + response.status);
      return FAILED;
    }
    doc = /** @type {FiscProjection} */ (await response.json());
  } catch (e) {
    // THE TOKEN IS CHECKED BEFORE THE BANNER, as it is at every other exit.
    // Without it, a reader who switched away while a fetch was failing got the
    // file:// remediation banner -- role="alert" -- pasted over a year that drew
    // correctly: the page asserting something untrue about what is on screen.
    if (token !== switching) return SUPERSEDED;
    // For a rejected fetch the overwhelmingly likely cause is file:// -- Chrome
    // blocks fetch from a file: origin, so the page loads and the chart never
    // arrives. Say the fix rather than the error. A body that will not parse is
    // a different fault and gets its own sentence, because "serve it over HTTP"
    // is useless advice to someone already doing that.
    // `e.name` and not `e instanceof SyntaxError`: instanceof compares against
    // THIS realm's constructor, and an error thrown by a response body parsed
    // in another one is not an instance of it. In a browser the two realms are
    // the same and both work, which is what makes the difference invisible --
    // under tools/jscheck's vm they are not, the instanceof arm was dead, and
    // the branch below could never have been shown to work at all.
    fail(e && e.name === "SyntaxError"
      ? "Could not read " + year.path + ": the file is not valid JSON, so it is " +
        "truncated or was not the document this page expected."
      : "Could not load " + year.path + ". If you opened this file directly, the browser " +
        "blocks the request: serve the directory over HTTP instead, e.g. " +
        "python3 -m http.server -d dist 8000");
    return FAILED;
  }
  if (token !== switching) return SUPERSEDED;
  // The fetched file is what actually gets drawn, and it is a separate
  // document from the config: the packager stamps the config from the
  // projection it was handed, so agreeing with the config is not evidence the
  // file on the wire agrees too.
  if (!isDocument(doc, year.path)) return FAILED;
  if (!understands(doc.schema_version, year.path)) return FAILED;
  if (!drawableSankey(doc, year.path)) return FAILED;

  // LAY OUT BEFORE MUTATING ANYTHING. Every throw left in the draw is in here
  // -- a link naming a node the document does not carry is the realistic one --
  // and while this ran at the END of the repaint, such a throw left the page
  // showing the new year's words over the old year's chart (fisc-bsg). Doing it
  // first means a failure propagates to the change handler's .catch with the
  // page still wholly the year it was already on.
  // THE FOLD IS PART OF THE LAY-OUT AND SITS INSIDE THE SAME GUARANTEE. It
  // throws on a node it cannot place, which is a fault in this page's tier set
  // rather than in the document, and it must throw here -- before the repaint --
  // for the same reason layOut does.
  const drawn = foldDocument(doc);
  const laid = layOut(drawn);

  // THE FOLDED DOCUMENT IS THE ONE THE PAGE DESCRIBES, not the one it fetched.
  // The legend, the flow table, the inferred list, the tooltips and the detail
  // panel all read this, and every one of them is a statement about what the
  // reader is looking at. Pointing them at the unfolded document would put a
  // 175-row table beside a 52-ribbon chart. Nothing is lost by it: the fold
  // unions the fact ids it merges, so the table still names every fact behind
  // every ribbon, and the footer still links the unfolded file it came from.
  projection = drawn;
  // A refusal from an earlier attempt is about a year no longer on screen, and
  // fail() only ever added banners because it used to be the end of the story.
  // Now that a switch can recover, a stale role="alert" left above a correct
  // chart is a false statement the page keeps making.
  clearRefusal();
  // A pin and an isolation belong to the year they were made in: a fund group
  // selected in FY2026 may not carry the same flows in FY2027, and a provenance
  // panel left on screen would cite fact ids from a document no longer drawn.
  pinned = null;
  isolated = "";
  resetDetail();
  hideTip();

  paintYearWords(year);
  buildLegend();
  buildDerivedList();
  buildTable();
  render(laid);
  return DREW;
}

/**
 * Replaces every word on the page that belongs to a year: the tiles, the
 * caveats, the caveat count in their summary, the lede, the flow count, the
 * chart's accessible title, the footer's basis and its data-file citation, and
 * the document title.
 *
 * THE LIST IS EXHAUSTIVE ON PURPOSE. It read "the tiles, the caveats, the lede
 * and the flow count" while the function wrote four more, and a doc comment
 * that undercounts its own writes is how the next per-year string gets added to
 * the template and forgotten here -- which is the fisc-kwq / fisc-yi4 / fisc-iyt
 * defect three times over. If you add a write, add it above.
 *
 * The page already carries the opening year's, rendered server-side so the
 * headline survives with JavaScript off. This swaps them for another year's,
 * and every string it writes was built by the packager.
 * @param {FiscYear} year
 */
function paintYearWords(year) {
  const figures = maybeEl("figures");
  if (figures) {
    figures.replaceChildren(...[year.hero].concat(year.figures).map((f) => {
      const tile = h("div", "tile" + (f.kind ? " " + f.kind : ""));
      tile.appendChild(h("div", "label", f.label));
      tile.appendChild(h("div", "value", f.value));
      tile.appendChild(h("div", "note", f.note));
      return tile;
    }));
  }

  const caveats = maybeEl("caveats");
  // THE SUMMARY, WRAPPED IN ITS LINK -- and the link is what makes showing a
  // summary honest rather than a truncation. The paragraph still exists, on a
  // page of its own, and c.href names THIS year's copy of it: a caveat id can
  // carry different text in different documents, so a href built once for the
  // opening year would send a reader who switched to FY2026-27 to FY2025-26's
  // sentence. The packager composes it per year for that reason.
  //
  // NO LINK WHEN THERE IS NO PAGE. A single-view export has no caveats.html,
  // and an anchor into a file that was never written is worse than a plain
  // line: it looks like there is more to read.
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

  // THE CAVEAT COUNT IS PER-YEAR, and it is in a <summary> the reader uses to
  // decide whether to open the list at all. FY2025-26 carries four and
  // FY2026-27 five, so a count painted once says "4 reasons" over a list of
  // five -- a disclosure that under-reports itself, which is worse than no
  // count. maybeEl and not el: only index.html.tmpl renders this id.
  const caveatCount = maybeEl("caveats-count");
  if (caveatCount) caveatCount.textContent = String(year.caveats.length);

  const lede = maybeEl("lede-year");
  if (lede) lede.textContent = year.label + " " + year.basis;

  // THE FLOW COUNT IS A CLAIM ABOUT THE CHART, so it counts the marks that were
  // drawn rather than the rows the file holds. On a page drawn whole the two
  // are the same number and this is the packager's figure verbatim. On a page
  // that folds they are not: fund-flows.json holds 175 links over 145 nodes and
  // the chart beside this sentence draws 52 over 40, and printing the file's
  // figures there would have the page miscount what the reader can see. The
  // fact total stays the document's, because folding cites nothing away.
  const counts = maybeEl("counts-line");
  if (counts) {
    const links = projection ? projection.links.length : year.counts.links;
    const nodes = projection ? projection.nodes.length : year.counts.nodes;
    counts.textContent = links + " flows between " + nodes +
      " nodes, from " + year.counts.facts + " facts";
  }

  // THE CHART'S ACCESSIBLE NAME IS BUILT IN GO, like every other string this
  // function writes. It was composed here from a literal, and the moment a
  // second page drew a chart that literal was WRONG on it: the drill-down's
  // template names a diagram "by fund and division", and the first repaint
  // replaced that with the spine's wording -- so two different charts announced
  // themselves identically to a screen reader. Same defect as fisc-rn0, which
  // is why sankeyTitle exists, reached through the one string that had not been
  // moved yet. Found by /code-review, 2026-08-28.
  const title = maybeEl("chart-title");
  if (title && year.chart_title) title.textContent = year.chart_title;

  // The footer's "Scope X, basis Y" sentence is a claim about the document ON
  // SCREEN -- the comment beside it in the template says so in as many words --
  // and the basis half is per-year. Left unpainted, a reader who switches to a
  // year published on another basis gets a lede reading "FY 2026-27 proposed"
  // and a footer three screens down still reading "basis adopted": one page
  // stating two different things about one document. The scope half is NOT
  // repainted and must not be; see the template comment for why.
  const basis = maybeEl("page-basis");
  if (basis) basis.textContent = year.basis;

  // The footer's "drawn from" link names a file that IS year-specific --
  // data/sankey.json and data/sankey-2027.json are different documents -- so it
  // has to follow the switch. Left alone it cited the opening year's file for a
  // chart drawn from another one, which is a provenance link that disagrees with
  // the figures beside it: the exact defect the citations exist to prevent,
  // reached through the toggle rather than through the packager.
  for (const el of document.querySelectorAll("[data-year-path]")) {
    const a = el.querySelector("a");
    if (a) {
      a.setAttribute("href", year.path);
      a.textContent = year.path;
    }
  }

  // BUILT IN GO, like every other string here. This composed the title from a
  // literal copied out of buildSankeyPage's fallback -- the one write in this
  // function the packager had not made -- and it did it unconditionally, so a
  // View that configured its own Title had it replaced during the opening
  // showYear, before the reader touched anything.
  document.title = year.title;
}

/**
 * The year the CONTROL is showing, which is not always the first one.
 *
 * index.html.tmpl hard-codes `checked` on years[0] and the radios carry no
 * autocomplete="off", so Chrome and Firefox both RESTORE the reader's own
 * selection across a soft reload (F5) and across a Back navigation. main() used
 * to open on years[0] unconditionally, so: select FY 2026-27, follow a nav link,
 * press Back -- the toggle comes back reading FY 2026-27 while the lede, the
 * tiles, the chart, the <title>, the footer basis and the data-year-path
 * citation are all FY 2025-26. No change event fires on a restore, so it never
 * self-corrects.
 *
 * index.html.tmpl's own comment calls that state worse than no control at all,
 * and it is right: the styling agrees with the wrong year too.
 *
 * OPENING ON THE RESTORED YEAR RATHER THAN FORCING THE RADIO BACK. Both close
 * the gap. This one does what the reader expects -- their selection survived
 * the navigation, so honour it -- where forcing years[0] would silently discard
 * it and look like the page ignoring a click.
 *
 * It walks the fieldset's children and reads the checked property, rather than
 * asking querySelector for the checked input. That needs no selector engine, so
 * tools/jscheck can drive it -- and the stub's declared-selector check keeps
 * app.js honest about which selectors it uses, so adding one here would have to
 * be declared there too.
 * @param {FiscYear[]} years
 * @returns {FiscYear} always one of `years`; years[0] when nothing is checked
 */
function checkedYear(years) {
  const group = maybeEl("year-toggle");
  if (group) {
    for (const input of group.children) {
      if (!input.checked) continue;
      const year = years.find((y) => y.stem === input.value);
      // A checked radio naming a stem this config does not publish is a stale
      // restore -- the page was rebuilt with different years since. Fall
      // through to years[0] rather than draw nothing.
      if (year) return year;
    }
  }
  return years[0];
}

/**
 * Wires the year radio group.
 *
 * The control is rendered server-side, so this only adds the behaviour. A year
 * that fails to load leaves the radio where the reader put it and shows the
 * refusal: moving it back would claim the page is showing a year it is not.
 *
 * IT DOES NOT ASSUME THE CONTROL SHOWS THE FIRST YEAR, which main() is where
 * that matters -- see checkedYear.
 * @param {FiscYear[]} years
 */
function wireYears(years) {
  const group = maybeEl("year-toggle");
  if (!group || years.length < 2) return;
  // The template ships it disabled, because without this file the control
  // cannot do anything. Enabling it here is the enhancement.
  group.removeAttribute("disabled");
  group.addEventListener("change", (e) => {
    const target = /** @type {HTMLInputElement} */ (e.target);
    const year = years.find((y) => y.stem === target.value);
    if (!year) return;
    // A .catch, which main() has had all along and this has not. showYear no
    // longer rejects for a bad document -- both awaits are inside its try -- so
    // this is the last resort rather than the handler for a known case, and it
    // must say so rather than repeat the fetch advice. Without it a rejection
    // here is unhandled: no banner, and the page left mid-repaint.
    void showYear(year).catch((e) => fail("The chart failed to draw: " + String(e)));
  });
}

async function main() {
  wireTheme();
  // Before the fetch, not after: FISC_CONFIG carries the projection's own
  // metadata block, and the headline the page has already rendered from it
  // server-side is read under the same contract as the graph.
  if (!understands(CONFIG.schema_version, "this page's data")) return;

  const years = CONFIG.years || [];
  if (!years.length) {
    fail("This page was packaged without any published year, so there is nothing to draw.");
    return;
  }
  wireYears(years);

  // EVERYTHING THE PAGE WIRES IS WIRED BEFORE THE FIRST FETCH, and that ordering
  // is the fix rather than a tidy-up (fisc-8cg).
  //
  // These two used to sit AFTER `if (!await showYear(years[0])) return;`, so any
  // outcome but success cost the reader both of them for the rest of the visit.
  // Two routes reached that, and the second is why widening showYear's return
  // value was not enough on its own:
  //
  //   - SUPERSEDED. wireYears enables the control above, before this await, so
  //     the toggle is live for the whole of the opening fetch. A reader who
  //     clicks during it bumps the switch token, the opening attempt stands
  //     down, and main() returned -- while the clicked year drew from its own
  //     showYear, leaving a page that looks entirely healthy.
  //   - FAILED, then recovered. clearRefusal exists precisely because "a year
  //     switch can recover from a failed one": the opening fetch is refused,
  //     main() returns, the reader clicks the other year, it succeeds and takes
  //     the banner down. Same healthy-looking page, same two dead affordances,
  //     reached through the outcome a three-state return leaves alone.
  //
  // Neither listener depends on a chart existing. paint() re-reads the palette
  // over whatever marks are on screen, which before the first draw is none, and
  // the Escape handler clears state that is already clear. So there is nothing
  // to sequence and no reason to wait.
  document.addEventListener("keydown", (e) => {
    if (e.key === "Escape") {
      hideTip();
      pinned = null;
      // The panel is the pin made visible, so clearing one without the other
      // leaves provenance on screen for a flow that is no longer selected.
      resetDetail();
      setIsolated("");
    }
  });
  if (typeof window.matchMedia === "function") {
    // Following the OS mid-visit means re-reading the palette, because the
    // hues are custom properties and d3 wrote the resolved values onto the
    // marks -- AND re-syncing the button, because prefersDark() has just
    // changed its answer underneath it. Wiring paint alone left the control
    // announcing the opposite of the page it sits on; see syncTheme.
    window.matchMedia("(prefers-color-scheme: dark)").addEventListener("change", () => {
      syncTheme();
      paint();
    });
  }

  // Last, and its outcome is deliberately not acted on. showYear has already
  // told the reader if it failed, and nothing is left for main() to do or to
  // skip. Keeping the await means an opening failure still reaches main()'s
  // .catch if it ever throws rather than returning FAILED.
  //
  // checkedYear, not years[0]: the browser may have restored a selection the
  // server-rendered page knows nothing about.
  await showYear(checkedYear(years));
}

main().catch((e) => fail("The chart failed to draw: " + String(e)));
