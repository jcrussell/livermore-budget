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
 * @property {string[]} [folds] the ids a synthetic aggregate stands for.
 *   PRESENT ONLY ON capColumn'S AGGREGATE and on no node any document
 *   publishes: the cap folds a column's tail by VALUE, which nothing in the
 *   parent chain records, so caveatsFor cannot reach those ids by walking. It
 *   is optional because every real node lacks it.
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
 * @property {FiscStepDoc[]} [steps]  what this year's rungs draw, one per
 *   declared step, resolved for this year by the packager
 * @property {string} chart_title
 */

/**
 * One rung's document for one year: where to fetch it and what its caveats
 * link to, verbatim from export.stepView.
 *
 * @typedef {Object} FiscStepDoc
 * @property {string} stem
 * @property {string} path
 * @property {FiscCaveatRef[]} caveats
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
 * @property {FiscDrillStep[]} [steps]
 * @property {string} [root]
 */

/**
 * @typedef {Object} FiscTierCap
 * @property {number} tier
 * @property {number} cap  how many nodes the tier may hold before its tail is
 *   folded into one aggregate; see capColumn for why a cap is needed at all.
 */

/**
 * One hop of the chain the packager ships, verbatim from export.DrillStep.
 *
 * @typedef {Object} FiscDrillStep
 * @property {number} from  the tier whose nodes open, in the chart on screen
 *   before they do
 * @property {string} [projection]  the document this step draws; absent means
 *   the same one as the step before
 * @property {number[]} tiers  the tier set drawn once one has
 * @property {FiscTierCap[]} [caps]  per tier; a tier with none is drawn whole
 * @property {string} back  what the breadcrumb's return control says
 * @property {string} tail  the plural noun a capped aggregate is counted in
 * @property {string} description  the chart's long description once a node
 *   has opened on this step, in the packager's words
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
 * layer is the COLUMN d3-sankey put the node in, which is not depth: depth is
 * the longest path to the node, and layer is what the align function returned
 * after clamping. columnShare totals a column and needs the second.
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

/**
 * How this page drills: the chain of steps the packager declared, empty for a
 * page that opens nothing.
 *
 * PER VIEW AND NEVER A CONSTANT HERE, for render_tiers' reason: the spine and
 * the two fund-flows pages are drawn by the same script from different
 * hierarchies, and a page that declares nothing keeps the isolate-on-click
 * behaviour it has always had.
 *
 * A CHAIN, READ AS A PATH. Step k opens a node of the chart k rungs deep, so a
 * node in an opened view is itself openable exactly when a step exists at the
 * next depth; stepAt is the one reader of that rule. The packager validates the
 * chain as a path (export.validateSteps), and this file assumes no more than
 * that: fisc-ko1j.12 is the shape it cannot express.
 *
 * @type {FiscDrillStep[]}
 */
const STEPS = CONFIG && Array.isArray(CONFIG.steps)
  ? CONFIG.steps.filter((s) => s && Array.isArray(s.tiers) && s.tiers.length > 0)
  : [];

/**
 * The step that opens a node of the chart at this depth, or null when nothing
 * at that depth opens.
 * @param {number} depth  how many nodes are open, 0 on the overview
 * @returns {FiscDrillStep | null}
 */
function stepAt(depth) {
  return depth >= 0 && depth < STEPS.length ? STEPS[depth] : null;
}

/**
 * The node whose subtree this page draws, or "" for the whole document.
 *
 * A REFUSAL AVOIDED RATHER THAN A PREFERENCE EXPRESSED. Spending draws tiers
 * {3,4} of a document that also carries eleven tier-0 revenue nodes, and
 * foldDocument refuses a node it cannot place -- so without this the page draws
 * nothing at all rather than drawing half of something. Measured before it
 * existed: "cannot draw fund-flows: node revenue/charges-for-services is tier 0
 * and no ancestor of it is a tier this page draws (3, 4)".
 */
const ROOT = CONFIG && typeof CONFIG.root === "string" ? CONFIG.root : "";

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
  // STARTED FROM THE HIERARCHY, NOT FROM THE COPY IT WAS HANDED. layOut passes
  // a LAID node -- a shallow copy of a FOLDED node -- whose parent foldDocument
  // sets to "" when the ancestor it folded to was filtered away. That is right
  // for the folded document, whose well-formedness is about nodes it carries,
  // and it stops this walk dead: `if (!at.parent) return ""` on the first hop.
  //
  // Measured before the fix: every fund and every division on every opened
  // view resolved to "", so each rendered entirely in --muted. groupIndex
  // prefers the FETCHED document, so what is walked here is where the node
  // really sits.
  let at = groupIndex.get(node.id) || node;
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
/**
 * One opened node: which it is, the document its chart is shaped FROM, and the
 * step that opened it.
 *
 * THE DOCUMENT IS ON THE RUNG AND NOT LOOKED UP, because two rungs can draw two
 * documents: a step that names a projection opens a node of one file into a
 * chart of another. Everything that has to know which file is on screen --
 * the counts line, the hue walk, the caveats, the citations -- asks the rung
 * rather than the page.
 * @typedef {Object} Rung
 * @property {string} id  the node opened
 * @property {FiscProjection} doc  the document this rung's chart is shaped from,
 *   unfolded
 * @property {FiscDrillStep} step  the step that opened it
 */

/**
 * The nodes the chart is opened into, outermost first; empty on the overview.
 *
 * A STACK, BECAUSE THE DRILL IS A CHAIN. Step k opens a node of the chart k
 * rungs deep, so a node in an opened view is itself openable whenever a step
 * exists at the next depth -- drillable asks exactly that -- and the breadcrumb
 * shows one rung per step taken. The chain is read as a path: depth k was
 * opened by STEPS[k] and by nothing else.
 * @type {Rung[]}
 */
let drilled = [];
/**
 * The year's document as fetched, before any fold: what the overview is shaped
 * from, and what the first step opens a node of.
 *
 * A DRILL RESHAPES FROM THE FILE, not from what is on screen. Folding a folded
 * document would ask for tier 3 in a document whose tier 3 has already been
 * collapsed into tier 2 -- the nodes are gone, and the fold would throw or, if
 * it did not, draw the overview again with a breadcrumb over it.
 *
 * ONE DOCUMENT PER DEPTH, and this is depth 0's. A rung carries its own, which
 * is this one for a step that names no projection and a fetched file for a
 * step that does; docAt reads them as one sequence.
 * @type {FiscProjection | null}
 */
let fetched = null;
/**
 * Step documents already fetched this year, by path, so returning to a rung and
 * opening it again does not fetch its file twice.
 *
 * FETCHED LAZILY ON THE FIRST DRILL, not eagerly with the year: a reader who
 * never opens a node never pays for the file. DROPPED BY showYear, because a
 * step's document belongs to the year it was opened in and the path a step
 * resolves to is a claim about the year on screen.
 * @type {Map<string, FiscProjection>}
 */
let stepDocs = new Map();
/**
 * The drill's own gesture token, bumped by every push and pop of the stack.
 *
 * A DRILL THAT FETCHES CAN BE OVERTAKEN, by a second click while its file is in
 * flight or by a pop of the rung it was opened from, and a drill that lands
 * after either would push onto a stack that is no longer the one it was opened
 * against. Compared after the await, beside the year token: a year switch
 * mid-drill bumps that one, and the drill stands down for it too.
 */
let opening = 0;
/**
 * The year on screen, so paintCounts can be called without one in hand.
 * @type {FiscYear | null}
 */
let shownYear = null;
/**
 * The chart description the page shipped, so returning from a drill can restore
 * it. Captured on first paint rather than read from the config, because it is
 * the TEMPLATE's string -- the packager sends the subject to the client and the
 * description only into the markup.
 * @type {string}
 */
let baseDescription = "";
/**
 * The pointer to the flow table, lifted off the shipped description so a drill
 * can keep it.
 *
 * WHY IT HAS TO SURVIVE: the flow table ships inside a closed <details>, which
 * is out of the accessibility tree until it is opened, so this sentence is the
 * only route to it a reader who cannot see the page has. Replacing the whole
 * <desc> on a drill dropped it.
 *
 * TAKEN BY POSITION, NOT BY ITS WORDS. Matching the sentence here would be a
 * second copy of wording the templates own, and the two would drift the first
 * time either was edited. Both templates put it last;
 * TestTheTablePointerIsTheLastSentenceOfEveryChartDescription pins that.
 * @type {string}
 */
let tablePointer = "";
/**
 * The nodes as laid out, so columnShare can total the column a mark is in.
 *
 * SEPARATE FROM projection, which holds the FOLDED document and carries no
 * geometry: which column a node is in is d3-sankey's answer, not the file's,
 * and two nodes of one tier can land in one column while a tier the page skips
 * lands in none.
 * @type {LaidNode[]}
 */
let laidNodes = [];
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
 * The tier a node folds to under a tier set, or "" when it has no drawn
 * ancestor.
 *
 * THE NON-THROWING HALF OF foldDocument'S FIRST LOOP. The fold refuses a node it
 * cannot place, because there the tier set is meant to describe the whole
 * document and a node outside it is a fault. filterToNode asks the same
 * question for the opposite purpose: which links are IN this drill's scope at
 * all, where an unplaceable end is an ordinary answer rather than an error.
 *
 * @param {Map<string,FiscNode>} byID
 * @param {FiscNode} n
 * @param {Set<number>} drawn
 * @returns {string}
 */
function foldTarget(byID, n, drawn) {
  let at = n;
  for (let hops = 0; !drawn.has(at.tier); hops++) {
    const up = at.parent ? byID.get(at.parent) : undefined;
    if (!up || hops > 8) return "";
    at = up;
  }
  return at.id;
}

/**
 * The document restricted to one node's own money.
 *
 * WHY A FILTER AND NOT AN EXPANSION. fisc-ppkq measured that expanding one node
 * in place does not draw on the vendored d3-sankey: it takes the column count
 * from topology and clamps the align into it, so an expanded group's funds land
 * in the same column as the divisions while the unexpanded ribbons span two --
 * which tools/jscheck/layout.mjs's bands() refuses. Filtering keeps every tier
 * set uniform, which is the only shape this build lays out.
 *
 * THE RULE IS "TARGET IN THE SUBTREE, BOTH ENDS PLACEABLE", and both halves are
 * needed because the two pages that drill want opposite things from the same
 * function:
 *
 *   - Revenue opens a fund group into its funds at tiers {0,3}. The context
 *     column, the revenue sources, sits OUTSIDE the subtree and must be kept.
 *     Requiring both ends inside would delete it and draw a column of funds fed
 *     by nothing.
 *   - Spending opens a division into its object categories at tiers {4,5}. The
 *     fund-to-division link pointing INTO the subtree must be dropped, because
 *     fund/100 has no ancestor at tier 4 or 5 and foldDocument would refuse the
 *     whole document over it.
 *
 * Keying on the target says which end is the fine one; the placeability test
 * says what this drill's tier set has room for. Neither is a silent loss: a
 * link dropped here is one the declared tier set has no column for, which is a
 * statement the view made when it declared them.
 *
 * @param {FiscProjection} doc
 * @param {string} id
 * @param {number[]} tiers
 * @returns {FiscProjection}
 */
function filterToNode(doc, id, tiers) {
  const byID = new Map(doc.nodes.map((n) => [n.id, n]));
  // A NAME THIS DOCUMENT DOES NOT CARRY IS A FAULT IN THE VIEW, and it must say
  // so. Unchecked, an unknown id gives an empty subtree, no links, and
  // d3-sankey dying on the empty graph with "RangeError: Invalid array
  // length" -- a stack trace where a sentence belongs. Live the day these pages
  // get the year control unviewedDocuments still declares as pending: a root or
  // an opened node valid in one column need not exist in another.
  if (!byID.has(id)) {
    throw new Error("cannot draw " + doc.projection + ": this page asks for node " + id +
      ", which the document does not carry");
  }
  const drawn = new Set(tiers);

  // The subtree: the node and everything whose parent chain reaches it. Walked
  // upward per node rather than downward from the root, because a node names
  // its parent and nothing names its children.
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

  const links = doc.links.filter((l) => {
    if (!inside.has(l.target)) return false;
    const src = byID.get(l.source);
    const dst = byID.get(l.target);
    if (!src || !dst) return false;
    return foldTarget(byID, src, drawn) !== "" && foldTarget(byID, dst, drawn) !== "";
  });

  // Only the nodes those links touch, and their ancestors up to the drawn
  // tiers. Handing foldDocument a node it cannot place would make it refuse the
  // document, and the nodes it cannot place here are precisely the ones this
  // drill is not about.
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
 * The document the chart at one depth is shaped from: the year's at depth 0,
 * and the rung's own below that.
 *
 * WHY DEPTH AND NOT RUNG: a rung's label lives in the document it was opened
 * FROM, which is the chart one depth up -- the node a reader clicked is gone
 * from the chart it opened into, and across a document switch it may not exist
 * there at all. paintBreadcrumb asks for depth k's document to name rung k.
 * @param {number} depth
 * @returns {FiscProjection | null}
 */
function docAt(depth) {
  return depth <= 0 ? fetched : (drilled[depth - 1] ? drilled[depth - 1].doc : null);
}

/**
 * The document the chart on screen is shaped from.
 * @returns {FiscProjection | null}
 */
function drawnDoc() {
  return docAt(drilled.length);
}

/**
 * The tier set the document on screen was shaped by.
 *
 * ONE READER FOR EVERY DECLARATION. Everything downstream of the shaping -- the
 * column alignment, and anything else that has to know which tier is which
 * column -- needs the set the document was actually folded to, and that is
 * RENDER_TIERS on an overview and the opening step's tiers on a rung. Asking
 * for RENDER_TIERS directly is right in exactly one of those states, which is
 * how the drill first shipped a chart that could not be laid out at all: a
 * wrong answer here is not a wrong-looking chart, it is a throw inside
 * d3-sankey's ordering pass.
 *
 * @returns {number[]}
 */
function activeTiers() {
  return drilled.length ? drilled[drilled.length - 1].step.tiers : RENDER_TIERS;
}

/**
 * Writes the flow count.
 *
 * THE FLOW COUNT IS A CLAIM ABOUT THE CHART, so it counts the marks that were
 * drawn rather than the rows the file holds. On a page drawn whole the two are
 * the same number and this is the packager's figure verbatim. On a page that
 * folds they are not: fund-flows.json holds 175 links over 145 nodes and the
 * Revenue chart draws 29 over 17, and printing the file's figures there would
 * have the page miscount what the reader can see.
 *
 * THE FACT TOTAL IS THE DRAWN DOCUMENT'S, and the word "drawn" is what two
 * documents add to the rule. "The document's" was one number while a page had
 * one file; a step that names a projection opens a node of one file into a
 * chart of another, and shownYear.counts.facts is the first file's total. Read
 * at depth 1 it would print "from 33 of the document's 120 facts" over a chart
 * of fund flows -- weighing one document's ribbons against another's file. So
 * the total comes from the document the chart was shaped from: the year's own
 * figure, which the packager stamped from that document, while the year's
 * document is the one drawn, and the drawn document's own metadata.counts
 * below that. Folding cites nothing away is NOT why the total holds; it is true
 * only at tiers {0,2,4}, where the fund-to-division link that survives carries
 * the same facts as the object rows folding into it, and no shipped page folds
 * that way. Measured: Revenue at {0,2} folds the whole spending side into
 * self-loops and drops them, so 190 of the document's 239 cited facts are
 * behind what it draws; Spending at {3,4} carries the other 49. They partition
 * it exactly, which is what two pages splitting one document should do.
 *
 * The number is still the document's ON A PAGE SHOWING THE WHOLE DOCUMENT,
 * because THE GAP IS THE POINT -- the claim project.Counts.Facts is built on:
 * "the gap between Facts and Links is the part of the schedule the chart cannot
 * show, and stating both is what makes it visible". Replacing it with a count
 * of the facts actually cited would close that gap and quietly stop saying so;
 * on the spine, where 120 facts sit behind 58 flows because 50 are printed
 * zeros and 12 are stocks, it would delete the sentence's whole subject.
 *
 * AN OPENED NODE IS NOT THAT PAGE. Drilled into one of six fund groups, the
 * document's 280 facts are not what the reader is being shown the gap to --
 * "21 flows between 13 nodes, from 280 facts" invites them to weigh a sixth of
 * a chart against the whole file, which is not a gap that means anything. So a
 * drilled chart counts the facts its own ribbons cite, and the sentence gains
 * the words that say which of the two it is doing.
 *
 * A DOCUMENT THAT CARRIES NO metadata.counts IS COUNTED BY ITS RIBBONS ALONE,
 * rather than dereferenced: this runs mid-repaint, after the breadcrumb and the
 * chart name, and a throw here is the split page fisc-bsg is about. Every
 * document `fisc export` writes carries the block; the guard is for a file that
 * is not one of those.
 *
 * SEPARATE FROM paintYearWords BECAUSE A DRILL CHANGES IT TOO. It was inline
 * there while a year switch was the only thing that could change what is drawn;
 * opening a fund group changes it just as completely, and a counts line left
 * describing the overview under a drilled chart is the same false statement one
 * gesture over.
 */
function paintCounts() {
  const counts = maybeEl("counts-line");
  if (!counts || !shownYear) return;
  const links = projection ? projection.links.length : shownYear.counts.links;
  const nodes = projection ? projection.nodes.length : shownYear.counts.nodes;
  // PLURALS, because a drilled division can draw one ribbon. Fire
  // Administration and General Services each spend on a single object category,
  // so opening either used to read "1 flows between 2 nodes" -- a sentence that
  // was unreachable while the smallest chart on the site had 29 marks.
  const plural = (/** @type {number} */ n, /** @type {string} */ word) =>
    n + " " + word + (n === 1 ? "" : "s");
  // BOTH NUMBERS, ALWAYS, and the gap between them stated rather than implied.
  //
  // This printed the document's fact total on every undrilled page, justified
  // by "a page showing the whole document" -- a condition that is false of any
  // chart that filters or folds before it draws: a rung of the chain reads
  // "33 flows between 34 nodes" over ribbons citing 141 of 280. Naming one
  // number and meaning the other is the failure; naming one when there are
  // two is what lets it happen.
  //
  // Saying both keeps what project.Counts.Facts is built on -- "the gap between
  // Facts and Links is the part of the schedule the chart cannot show, and
  // stating both is what makes it visible" -- and makes it visible on a page
  // that draws a slice as well as on one that draws the lot. The spine reads
  // "58 flows between 25 nodes, from 58 of the document's 120 facts", where the
  // 62 it does not draw are the printed zeros and the stocks.
  let from = ", from " + plural(shownYear.counts.facts, "fact");
  if (projection) {
    const cited = new Set();
    for (const l of projection.links) {
      for (const id of l.fact_ids) cited.add(id);
    }
    const doc = drawnDoc();
    const own = doc && doc !== fetched && doc.metadata && doc.metadata.counts
      ? doc.metadata.counts.facts : undefined;
    const total = doc === fetched || !doc ? shownYear.counts.facts
      : typeof own === "number" ? own : cited.size;
    from = cited.size === total
      ? ", from " + plural(cited.size, "fact")
      : ", from " + cited.size + " of the document's " + plural(total, "fact");
  }
  counts.textContent = plural(links, "flow") + " between " + plural(nodes, "node") + from;
}

/**
 * Whether activating this node opens it.
 *
 * ONE TIER PER DEPTH AND NO OTHER. A node at the next step's `from` opens;
 * everything else on the page is an endpoint of the flow rather than a
 * container of it, and offering to open a revenue category would promise a
 * decomposition the document does not carry. Where no step exists at this
 * depth nothing opens, which is every node of a chain's last rung. An
 * aggregate is excluded by name -- it can sit at a step's `from` tier now that
 * caps are per tier -- and would have nothing to open into anyway, being
 * several documents' worth of small funds rather than one thing.
 *
 * @param {{id: string, tier: number}} d
 * @returns {boolean}
 */
function drillable(d) {
  const step = stepAt(drilled.length);
  return Boolean(step) && d.tier === step.from && !isAggregate(d.id);
}

/**
 * Whether focus is inside the chart or its breadcrumb, asked while the element
 * it is on still exists.
 *
 * IN THE CHART, not merely "not the body". Escape pressed from the flow
 * table's <summary> or from the footer while drilled would otherwise yank
 * focus into the chart -- which is the outcome restoreFocus's own comment
 * calls a defect, produced by the test that was supposed to prevent it.
 * @returns {boolean}
 */
function focusInChart() {
  const active = document.activeElement;
  const chart = maybeEl("chart");
  return Boolean(active) && Boolean(chart) &&
    (active === chart || (typeof chart.contains === "function" && chart.contains(active)) ||
      (typeof active.closest === "function" && active.closest(".breadcrumb") !== null));
}

/**
 * The year's entry for the rung a step at `depth` opens: the file it draws and
 * the caveat refs its marks link to, or null when the year on screen was
 * packaged with none.
 *
 * READ OFF THE YEAR, NEVER JOINED HERE. A step's document is per fiscal year --
 * FY2026-27's fund groups open into fund-flows-2027, not into the one file a
 * stem maps to in CONFIG.projections -- and which file that is belongs to the
 * packager, which resolves every step for every year into the year's own
 * config entry. This file resolves nothing: it reads the entry for the year on
 * screen at the depth being opened, and refuses when there is none rather than
 * draw a file the year was never told about.
 * @param {number} depth
 * @returns {FiscStepDoc | null}
 */
function stepDocAt(depth) {
  const steps = shownYear && Array.isArray(shownYear.steps) ? shownYear.steps : [];
  const entry = steps[depth];
  return entry && typeof entry.path === "string" && entry.path ? entry : null;
}

/**
 * The document a step draws, fetched and vetted on the first drill that needs
 * it and cached for the rest of the year; null when it could not be had, with
 * the reader told unless `superseded` says nobody is waiting.
 *
 * EVERY GUARD showYear RUNS, RUN HERE TOO. isDocument, understands and
 * drawableSankey each fail closed for a reason that is about the file rather
 * than about which gesture asked for it, and a click that skipped one would
 * draw at depth 1 a document the year control would refuse at depth 0.
 * loadDocument is the one place they are sequenced.
 * @param {FiscDrillStep} step
 * @param {number} depth  the depth the step opens from
 * @param {FiscProjection} from  the document of the chart the step opens from
 * @param {() => boolean} superseded
 * @returns {Promise<FiscProjection | null>}
 */
async function stepDocument(step, depth, from, superseded) {
  if (!step.projection) return from;
  const entry = stepDocAt(depth);
  if (!entry) {
    if (!superseded()) {
      fail("That could not be opened: this page's step names a document, " +
        step.projection + ", that the year on screen was not packaged with.");
    }
    return null;
  }
  const path = entry.path;
  const cached = stepDocs.get(path);
  if (cached) return cached;
  const doc = await loadDocument(path, superseded);
  if (doc) stepDocs.set(path, doc);
  return doc;
}

/**
 * Replaces the stack with `next` and repaints everything the shape decides,
 * or leaves the page exactly as it was and tells the reader why.
 *
 * IT RESHAPES FROM THE RUNG'S FILE and repaints the same set showYear repaints
 * on a year switch, for the same reason: the legend, the flow table, the
 * inferred list and the counts line are all statements about what the reader
 * is looking at, and a drill changes what that is as completely as a year does.
 *
 * SHAPE AND LAY OUT BEFORE MUTATING ANYTHING, which is showYear's contract
 * (fisc-bsg) and was not the drill's. layOut ran LAST here, inside the render
 * call, after the counts line, the breadcrumb, the legend, the inferred list
 * and the table had all been rewritten -- so a throw from it left the page
 * describing a chart it had not drawn: counts reading "0 flows between 0
 * nodes", an empty table, and a breadcrumb naming the node the reader had
 * opened, over the previous chart. Everything that can throw is in the two
 * lines inside the try, and both run while the page is still wholly the one
 * the reader was looking at. THE STACK IS SWAPPED FIRST because shapeFor and
 * layOut read it, and swapped back on a throw: the restore is of the whole
 * stack, not of one id.
 *
 * THE PIN AND THE ISOLATION ARE CLEARED, because both hold a node id and a
 * drill can remove the node they name -- opening a fund group deletes the group
 * itself from the drawn set. showYear clears them for exactly this reason on a
 * year switch; this is the same hazard one gesture over.
 *
 * @param {Rung[]} next
 * @returns {boolean} whether the new depth is on screen
 */
function redrawStack(next) {
  // ASKED BEFORE ANYTHING IS REPAINTED. The element focus is on is one the
  // repaint below removes, so after it there is nothing left to ask about.
  const hadFocus = focusInChart();
  const was = drilled;
  drilled = next;
  let drawn;
  let laid;
  try {
    const doc = drawnDoc();
    if (!doc) throw new Error("no document to open");
    drawn = shapeFor(doc);
    laid = layOut(drawn);
  } catch (e) {
    // BACK TO WHERE THE READER WAS, not to a blank page. shapeFor throws on a
    // document its tier set cannot describe, which is a fault in this view's
    // declaration rather than in the reader's click, and leaving the chart
    // drawn as it was is the only outcome that does not punish them for it.
    drilled = was;
    fail("That could not be opened: " + (e instanceof Error ? e.message : String(e)));
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
  buildLegend();
  paintChartHint();
  buildDerivedList();
  buildTable();
  render(laid);
  restoreFocus(hadFocus);
  return true;
}

/**
 * Opens one node of the chart on screen, one rung deeper.
 *
 * ASYNC BECAUSE THE FIRST RUNG OF A DOCUMENT-SWITCHING CHAIN FETCHES, and that
 * gives every guard in loadDocument a caller that is a click. The await sits
 * between the fetch and the repaint and nothing is mutated before it; after it
 * the gesture asks whether it has been overtaken -- by a later drill, a pop, or
 * a year switch -- and stands down rather than push a rung onto a stack that is
 * no longer the one it was opened against. A year switch mid-drill would
 * otherwise draw the year the reader left.
 *
 * @param {string} id
 * @returns {Promise<string>} DREW, SUPERSEDED or FAILED
 */
async function drillDown(id) {
  const depth = drilled.length;
  const step = stepAt(depth);
  const from = docAt(depth);
  if (!step || !from) return FAILED;
  const mine = ++opening;
  const token = switching;
  const overtaken = () => mine !== opening || token !== switching;
  const doc = await stepDocument(step, depth, from, overtaken);
  if (overtaken()) return SUPERSEDED;
  if (!doc) return FAILED;
  return redrawStack(drilled.concat([{ id: id, doc: doc, step: step }])) ? DREW : FAILED;
}

/**
 * Closes rungs until `depth` remain: 0 is the overview.
 *
 * SYNCHRONOUS, because every document a shallower rung needs is already on the
 * stack. It bumps the drill token so a drill in flight from a rung being closed
 * stands down instead of landing on the shorter stack.
 * @param {number} depth
 */
function drillUp(depth) {
  if (depth < 0 || depth >= drilled.length) return;
  opening++;
  redrawStack(drilled.slice(0, depth));
}

/**
 * What a click or a key does to a node that opens: drills, and banners a
 * rejection rather than losing it.
 *
 * The rejection path is the last resort, as wireYears' is. drillDown catches
 * the throws it knows -- the fetch, the shape, the layout -- so what reaches
 * here is a repaint that threw, and a click handler has nowhere else to put it.
 * @param {string} id
 */
function openNode(id) {
  void drillDown(id).catch((e) => fail("The chart failed to draw: " + String(e)));
}

/**
 * Puts focus somewhere real after a drill has replaced the chart.
 *
 * A KEYBOARD DRILL DESTROYS THE ELEMENT THAT WAS FOCUSED. The node a reader
 * tabbed to and pressed Enter on is exactly the node opening removes, and
 * paintBreadcrumb's replaceChildren does the same to the button on the way back
 * -- so focus fell to <body> in both directions, on a page whose own lede says
 * "tab to one and press Enter". A reader would have to tab in from the top of
 * the document again after every gesture the page invites.
 *
 * IT MOVES FOCUS ONLY IF IT WAS ALREADY IN THE CHART, because stealing it from
 * a reader who clicked with a mouse, or who is somewhere else on the page
 * entirely, would be its own defect.
 *
 * THE CALLER DECIDES THAT, AND HAS TO. This read document.activeElement itself,
 * and read it AFTER paintBreadcrumb and render had already detached the focused
 * element -- so it early-returned in exactly the two directions it was written
 * for and fired only in the case its own comment says must not happen. The
 * question has to be asked while the answer still exists.
 *
 * @param {boolean} hadFocus whether focus was inside the chart before the
 *   repaint that just replaced it.
 */
function restoreFocus(hadFocus) {
  if (!hadFocus) return;
  // focus() IS ON HTMLElement AND SVGElement, NOT ON Element, so the runtime
  // test stays and the cast is what tells tsc --checkJs the same thing. The
  // test is not redundant with the cast: the SVG marks are <g> elements and the
  // jscheck stub's nodes are plain objects, neither of which is obliged to have
  // it.
  const focus = (/** @type {Element | null} */ target) => {
    const el = /** @type {any} */ (target);
    if (!el || typeof el.focus !== "function") return false;
    el.focus();
    return true;
  };
  // THE INNERMOST RUNG'S CONTROL, NOT THE FIRST CHILD. Under N rungs the bar
  // holds N return controls, and children[0] is the outermost -- the way back
  // to the overview -- which is not where a reader two rungs deep came from.
  // The last control is the one that closes the rung just opened.
  const bar = maybeEl("breadcrumb");
  if (drilled.length && bar) {
    const controls = Array.from(bar.children || []).filter((c) =>
      String(/** @type {any} */ (c).tagName || "").toLowerCase() === "button");
    if (focus(controls[controls.length - 1] || null)) return;
  }
  const chart = maybeEl("chart");
  focus(chart ? chart.querySelector("g.node") : null);
}

/**
 * Names the chart for a screen reader, for the state it is actually in.
 *
 * A DRILL CHANGES WHAT THE CHART IS OF as completely as a year switch changes
 * which document it is, and nothing rewrote these two elements on one. After
 * opening a fund group the chart still announced its opening state and
 * described "six fund groups" that were no longer drawn -- only to the readers
 * who cannot see the marks disagree.
 *
 * IT APPENDS RATHER THAN REPLACES the name, so the page's own words survive:
 * the subject is the view's, built in Go, and this says which part of it is on
 * screen. Returning to the overview puts both back.
 */
function paintChartName() {
  // EVERY RUNG, OUTERMOST FIRST, so a reader two deep hears the whole path:
  // "opened into General Fund, then Patrol". One rung reads as it always did.
  const trail = drilled.map((_, k) => labelOfRung(k)).join(", then ");
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
  // THE TABLE POINTER CLOSES EVERY DEPTH'S DESCRIPTION, not only the first
  // rung's: it is captured once from the served sentence and the closed flow
  // table is out of the accessibility tree, so this sentence is the only route
  // to it a reader who cannot see the page has, however deep they are.
  //
  // THE STEP'S OWN WORDS SAY WHAT THE COLUMNS ARE. A sentence composed here
  // read "<node> on the left, and what it is made of on the right", which is
  // false of the first rung of the shipped chain: opening a fund group draws
  // revenue categories on the left and the group's funds beside them, and the
  // group itself is gone from the chart. The packager ships a description per
  // step, in the caller's words, and this file adds only what it owns -- the
  // rung's name, the way back and the table pointer.
  if (drilled.length) {
    const step = drilled[drilled.length - 1].step;
    const said = step && typeof step.description === "string" && step.description
      ? step.description
      : trail + " on the left, and what it is made of on the right.";
    desc.textContent = "Opened into " + trail + ". " + said +
      " Use the breadcrumb above the chart, or press Escape, to go back. " + tablePointer;
    return;
  }
  desc.textContent = baseDescription;
}

/**
 * The last sentence of a server-rendered description, with the template's own
 * line wrapping collapsed.
 *
 * Returns "" for a description of one sentence, which is what a caller-supplied
 * ChartDescription with no template suffix would be -- appending nothing beats
 * appending half of the chart's own sentence.
 *
 * @param {string} s
 * @returns {string}
 */
function lastSentence(s) {
  // ANY TERMINATOR, not just a period: export.View accepts ".", "!" and "?" as
  // the close of a caller's description, and splitting on ". " alone let the
  // other two run into the template's sentence.
  //
  // AN UNTERMINATED DESCRIPTION CANNOT REACH HERE. The packager refuses one --
  // see export.View.validate's endsASentence arm, which exists because every
  // fixture in this repo happened to end in a period and hid the case.
  const parts = String(s).replace(/\s+/g, " ").trim().split(/[.!?]\s+/);
  return parts.length < 2 ? "" : parts[parts.length - 1].trim();
}

/**
 * Says what a click does, for the state the chart is actually in.
 *
 * THE INSTRUCTION GOES FALSE THE MOMENT A READER FOLLOWS IT. On a chain the
 * question is whether a step exists BELOW this depth, not whether one is open:
 * "nothing here opens further" was true of every opened view while the drill
 * was one hop, and is false of depth 1 on a two-step chain. The page went on
 * saying "click a node in the right-hand column to open it" over a chart
 * where nothing opened, and the swatch sentence was worse: conditional on the
 * OPENING state's legend, and buildLegend draws no swatches in any opened view.
 *
 * THE COLUMN IS READ OFF THE CHART, NOT ASSUMED. This said "right-hand column"
 * unconditionally, which held while every declared step opened the finest tier
 * its chart drew and stopped holding on the spine, whose fund groups are its
 * MIDDLE column. openableColumn names the column the step's tier is drawn in.
 *
 * AND WHETHER ANYTHING OPENS IS ASKED OF THE DRAWN NODES, not of the chain: a
 * step exists below depth 1 for every fund group, and only the General Fund
 * draws a node at its `from` tier -- the other five groups' charts end at their
 * funds. Telling a reader to click a column that is not there is the same
 * defect as telling them to click one that does not open.
 *
 * Server-rendered for the opening state, so it survives with JavaScript off --
 * where it is also true, because without a script nothing can be opened at all.
 */
function paintChartHint() {
  const hint = maybeEl("chart-hint");
  if (!hint || !STEPS.length) return;
  const anyOpens = Boolean(projection) && projection.nodes.some(drillable);
  const column = anyOpens ? openableColumn() : "";
  if (drilled.length) {
    hint.textContent = "This is " + labelOfRung(drilled.length - 1) +
      ", broken into its parts. " +
      (anyOpens
        ? "Click a node in the " + column + " column to open it further, or tab to one and press Enter."
        : "Nothing here opens further; go back to open another.");
    return;
  }
  const swatches = buildLegendCount();
  hint.textContent = (anyOpens
    ? "Click a node in the " + column + " column to open it into its parts, " +
      "or tab to one and press Enter."
    : "Nothing on this chart opens.") +
    (swatches ? " A fund swatch follows one group's money without opening anything." : "");
}

/**
 * Which column of the chart on screen the next step's nodes are in --
 * "left-hand", "middle" or "right-hand" -- or "" when no step opens here.
 *
 * BY TIER, WHICH IS WHAT PLACES A COLUMN. layOut aligns columns on the tier
 * set the document was shaped by, so the drawn tiers in ascending order are
 * the columns left to right, and a step's `from` is one of them. Two drawn
 * tiers have no middle; more than three would make "middle" ambiguous, and
 * no document here draws more than three at once.
 * @returns {string}
 */
function openableColumn() {
  const step = stepAt(drilled.length);
  if (!step || !projection) return "";
  const tiers = [...new Set(projection.nodes.map((n) => n.tier))].sort((a, b) => a - b);
  const at = tiers.indexOf(step.from);
  if (at < 0 || tiers.length < 2) return "";
  if (at === 0) return "left-hand";
  if (at === tiers.length - 1) return "right-hand";
  return "middle";
}

/** How many fund-group swatches the legend is showing. */
function buildLegendCount() {
  const legend = maybeEl("legend");
  return legend ? legend.children.length : 0;
}

/**
 * Draws the trail back out of a drill.
 *
 * THE ONLY WAY BACK THAT IS ALWAYS VISIBLE. Escape also pops -- see main() --
 * but a reader who arrived by clicking has no reason to expect a keystroke, and
 * the node they clicked is no longer on the chart to click again: opening a
 * fund group removes the group. Without this the drill is a trapdoor.
 */
function paintBreadcrumb() {
  const bar = maybeEl("breadcrumb");
  if (!bar) return;
  if (!drilled.length) {
    bar.replaceChildren();
    bar.setAttribute("hidden", "");
    return;
  }
  bar.removeAttribute("hidden");
  // ONE RETURN CONTROL PER RUNG, EACH CLOSING TO ITS OWN DEPTH. Rung k's
  // control says what the chart k deep is -- the step's `back` -- and pops the
  // stack to k rungs, so a reader two deep can return one rung or two.
  //
  // THE WORDS ARE THE VIEW'S, not derived from the tier number. A first draft
  // read `from === 2 ? "fund groups" : "divisions"`, which is a mapping this
  // file has no way to keep true: a third page drilling from a third tier
  // would get "divisions" and nobody would find out from a test.
  const controls = drilled.map((rung, k) => {
    const back = h("button", "crumb-back");
    back.textContent = "\u2190 " + (rung.step.back || "Back");
    back.setAttribute("type", "button");
    back.addEventListener("click", () => drillUp(k));
    return back;
  });
  const here = h("span", "crumb-here", labelOfRung(drilled.length - 1));
  bar.replaceChildren(...controls, here);
}

/**
 * The printed label of the node rung k opened, from the document it was opened
 * FROM.
 *
 * NOT FROM THE DRAWN ONE, which is the point: the breadcrumb names the node the
 * reader opened, and opening it is what removes it from the drawn set. And not
 * from the rung's own document either: across a document switch the node was
 * clicked in the chart one depth up, and that chart's file is the one that
 * prints its label.
 * @param {number} k
 * @returns {string}
 */
function labelOfRung(k) {
  const rung = drilled[k];
  if (!rung) return "";
  const doc = docAt(k);
  const n = doc ? doc.nodes.find((x) => x.id === rung.id) : null;
  return n ? n.label : rung.id;
}

/**
 * The document as this page draws it: the overview, or one node opened.
 *
 * THE ORDER IS filter, cap, fold, AND IT IS NOT INTERCHANGEABLE.
 *
 *   - filter first, because the cap ranks a column by size and the sizes that
 *     matter are the ones inside the node being opened. Capping the citywide
 *     column and then filtering would keep the eight biggest funds in the CITY
 *     and show a group most of whose funds had already been discarded.
 *   - cap before fold, because the cap produces several ribbons from one source
 *     to the aggregate and the fold is what merges them -- summing the values
 *     and unioning the fact ids and locators. Capping afterwards would leave
 *     parallel ribbons between one pair of nodes, and an aggregate that cited a
 *     strict subset of the pages its figure was read from.
 *
 * @param {FiscProjection} doc
 * @returns {FiscProjection}
 */
function shapeFor(doc) {
  const rung = drilled.length ? drilled[drilled.length - 1] : null;
  if (!rung) {
    // THE ROOT IS A FILTER TOO, and the same one: a page that draws one node's
    // subtree is a page permanently opened into it. Composing them would be
    // wrong -- the node a reader opens is already inside the root -- so a drill
    // filters to what was clicked and an overview to what was declared.
    return foldDocument(ROOT ? filterToNode(doc, ROOT, RENDER_TIERS) : doc);
  }
  // ROOT DOES NOT REACH HERE. It is the spine's vocabulary -- the node whose
  // subtree THIS PAGE's overview draws -- and a rung filters to the node the
  // reader opened, which is inside the root on a page that has one and is a
  // node of another document entirely on a step that switched.
  const step = rung.step;
  let shaped = filterToNode(doc, rung.id, step.tiers);
  // EVERY CAP THE STEP DECLARES, COARSEST TIER FIRST. Folding a coarse node
  // removes its descendants (capColumn's orphaned()), which changes which fine
  // nodes are left to rank; capping the fine tier first would rank divisions
  // of a fund about to be folded away. The order is the step's tier order, not
  // the caps' declaration order, for the same reason the cap is looked up by
  // the tier it names.
  for (const tier of step.tiers) {
    const cap = (step.caps || []).find((c) => c.tier === tier);
    if (cap) shaped = capColumn(shaped, tier, cap.cap, rung.id, step.tail);
  }
  const drawn = foldDocument(shaped, step.tiers);

  // EVERY AGGREGATE'S PARENT IS PUT BACK AFTER THE FOLD, and it has to be here
  // rather than in capColumn. capColumn runs first and parents the aggregate at
  // the node being opened, which is true; foldDocument then re-points every
  // retained node's parent at its folded ancestor and blanks the ones whose
  // ancestor is not in the document -- which the opened node never is, since
  // opening it is what filtered it away. So the aggregate came out of the fold
  // parentless and drew in --muted among its coloured siblings.
  //
  // AT THE CURRENT RUNG, NOT THE FIRST: two rungs deep the folded tail is
  // inside the node opened last, and parenting it at the outer rung would be a
  // claim about the hierarchy the walk cannot confirm.
  //
  // It is restored rather than exempted from the fold, because the fold's rule
  // is about the document's own well-formedness and this is a claim about the
  // FILE's hierarchy, which is what fundGroupOf walks.
  return Object.assign({}, drawn, {
    nodes: drawn.nodes.map((n) =>
      (isAggregate(n.id) ? Object.assign({}, n, { parent: rung.id }) : n)),
  });
}

/**
 * The prefix every capped tail's id carries.
 *
 * NOT A FIGURE THE CITY PRINTED, and the label says so in words rather than
 * relying on this comment: it reads "N smaller funds", which is a count of rows
 * and not a line item. The amount on its ribbons is a sum of printed figures,
 * exactly as every folded ribbon's is.
 */
const AGGREGATE_PREFIX = "aggregate/tail/";

/**
 * The id of the node one tier's capped tail is folded into.
 *
 * PER TIER, BECAUSE A STEP CAN CAP TWO. One id for every fold put two
 * aggregates in one document the moment both a step's caps engaged -- two
 * nodes with one id, which d3-sankey keys by id and the fold merges by id, so
 * the second tail's ribbons landed on the first tail's node. Latent on the
 * committed corpus, where the division column never exceeds its cap while the
 * fund column does, and latent is how it ships.
 * @param {number} tier
 * @returns {string}
 */
function aggregateID(tier) {
  return AGGREGATE_PREFIX + tier;
}

/**
 * Whether an id names a capped tail of any tier.
 * @param {string} id
 * @returns {boolean}
 */
function isAggregate(id) {
  return id.startsWith(AGGREGATE_PREFIX);
}

/**
 * Folds all but the largest `cap` nodes of one tier into a single node.
 *
 * WHY A CAP IS NEEDED AT ALL, and it is the half of fisc-ppkq that bead got
 * wrong. It says "rescaling is what makes special-revenue's 32 funds legible".
 * Measured against dist/data/fund-flows.json at 7ded1c6, laying the drilled
 * graph out with the shipped d3 at this file's own constants: rescaled to its
 * own total, that group still puts 22 of its 49 ribbons under one pixel,
 * because the concentration is WITHIN the group -- fund/200 alone is 34.9% of
 * it and the bottom two are 0.034%. Rescaling cannot fix a distribution.
 *
 * At cap 8 the same graph comes to 2 sub-pixel ribbons, and the capital group
 * from 4 to 1. For comparison the drill-down page these replace ships 7.
 *
 * (That read "capital goes from 4 to 0" for one commit. The 0 was measured at
 * cap 6 during the search for a cap and quoted against cap 8, which is the
 * defect AGENTS.md's "Before you quote a number" exists to name, committed in a
 * comment about measurement.)
 *
 * IT IS THE SAME OPERATION AS THE FOLD, which is what makes it citable: values
 * sum, fact ids and locators union, so the aggregate ribbon cites every page
 * its figure was read from. What it is not is a node of the document's own
 * hierarchy, so it carries no parent and inherits no hue.
 *
 * @param {FiscProjection} doc
 * @param {number} tier
 * @param {number} cap
 * @param {string} opened  the node the column is inside, which the aggregate is
 *   parented to
 * @param {string} noun  the step's plural noun for the tier's rows
 * @returns {FiscProjection}
 */
function capColumn(doc, tier, cap, opened, noun) {
  const atTier = doc.nodes.filter((n) => n.tier === tier);
  // AN AGGREGATE OF ONE IS WORSE THAN NO AGGREGATE. This engaged at cap + 1, so
  // a column of 9 against a cap of 8 folded a single fund into a node labelled
  // "1 smaller funds" -- one figure the city printed, erased from the chart,
  // the table and the tooltip, relabelled ungrammatically, and listed under
  // "What we inferred" as though the grouping of one thing were an inference.
  // fund-group/enterprise has exactly 9, so this was shipping.
  //
  // The threshold is cap + 1 rather than cap, which means a column of exactly
  // cap + 1 is drawn WHOLE: one more mark than the cap asks for is a better
  // answer than one fewer plus a box saying "1 smaller".
  if (atTier.length <= cap + 1) return doc;

  /** @type {Map<string, number>} */
  const size = new Map();
  for (const l of doc.links) size.set(l.target, (size.get(l.target) || 0) + l.value_cents);
  // Ties broken by id, so the set kept is the same on every build of the same
  // document. A cap that reordered under an unstable sort would move which
  // funds a reader sees between two identical exports.
  const ranked = atTier.slice().sort((a, b) =>
    (size.get(b.id) || 0) - (size.get(a.id) || 0) || (a.id < b.id ? -1 : 1));
  const kept = new Set(ranked.slice(0, cap).map((n) => n.id));
  const folded = ranked.slice(cap);

  // THE NOUN IS THE VIEW'S. It read `tier === 3 ? "funds" : "categories"`,
  // which is the same tier-number-to-word mapping paintBreadcrumb refuses two
  // functions below, written by the same hand in the same commit.
  // Pluralised even though the threshold above now guarantees at least two,
  // because the two rules are in different functions and only one of them is
  // about grammar. paintCounts learned the same lesson one function away.
  const word = noun || "items";
  const label = folded.length + " smaller " +
    (folded.length === 1 ? word.replace(/s$/, "") : word);
  // derived: true, AND IT IS THE INVARIANT RATHER THAN A FLAG. The city printed
  // no line item called "24 smaller funds"; this node is ours, and shipping it
  // as printed made the page state the opposite in four places at once -- a
  // solid rather than dashed mark, a "printed by the city" chip in the tooltip
  // and the detail panel, an aria-label ending "printed by the city", and an
  // absence from "What we inferred", which is the list that exists to be
  // complete. The screen-reader path is the one that stated it most plainly.
  //
  // Its VALUE is still every cent a printed figure, summed exactly as the fold
  // sums a merged ribbon. What is inferred is the GROUPING, and that is what
  // the rationale says.
  const aggregate = {
    id: aggregateID(tier), label: label, tier: tier,
    // PARENTED TO THE NODE BEING OPENED, which is true -- every item folded
    // into it is inside that node -- and is what gives the mark its group's
    // hue instead of --muted. It was "" and drew grey among coloured siblings.
    parent: opened,
    constraint_tier: "",
    // A ROLE, because an empty one renders as a bordered empty .chip in both
    // the tooltip and the detail panel: a box with nothing in it, beside chips
    // that say something.
    role: "aggregate",
    // THE IDS IT SWALLOWED, so a caveat about one of them still reaches the
    // mark that now stands for it. caveatsFor walks the tier hierarchy, which
    // covers foldDocument's fold and NOT this one -- the tail is folded by
    // value, not by ancestry, so nothing in the parent chain records it.
    // IT RECORDS THE DESCENDANTS TOO, not only the tail. orphaned() below
    // removes anything parented beneath a folded node, and recording only the
    // tail left a caveat naming one of those descendants losing its badge for
    // the same reason the tail nodes would have. Latent today, since the one
    // caveat naming nodes names fund groups and fund/100, none of which is ever
    // in a tail; latent is how it would ship.
    folds: [],
    derived: true,
    rationale: "Our grouping, not a line the city printed: the " + folded.length +
      " smallest " + word + " in this column are drawn as one " +
      "mark because they cannot be drawn separately. Every figure inside it is printed; " +
      "the box around them is ours.",
    source_note: "The " + folded.length + " smallest of " + atTier.length +
      " by value, at this page's cap of " + cap + ".",
  };
  const tail = new Set(folded.map((n) => n.id));
  const remap = (/** @type {string} */ id) => (tail.has(id) ? aggregateID(tier) : id);

  // A FOLDED NODE'S DESCENDANTS GO WITH IT. Removing a tail node while leaving
  // anything parented to it produces a document whose child names a parent it
  // does not carry, and foldDocument then refuses the whole drill -- so the
  // reader gets a banner on a click that worked a moment ago. Latent today,
  // because fund/100 is the only tier-3 node with children and it is never in
  // any tail, and left latent is exactly how it would ship.
  const byID = new Map(doc.nodes.map((n) => [n.id, n]));
  const orphaned = (/** @type {{parent: string}} */ n) => {
    let up = n.parent;
    for (let hops = 0; up && hops < 9; hops++) {
      if (tail.has(up)) return true;
      const above = byID.get(up);
      up = above ? above.parent : "";
    }
    return false;
  };

  // FILLED HERE AND NOT AT THE LITERAL, because orphaned() is declared below it
  // -- a const in the temporal dead zone, which throws rather than reading as
  // undefined. Both halves of what the aggregate swallowed are known by this
  // point: the tail itself, and everything parented beneath it.
  const dropped = doc.nodes.filter(orphaned).map((n) => n.id);
  aggregate.folds = folded.map((n) => n.id).concat(dropped);

  const nodes = doc.nodes
    .filter((n) => (n.tier !== tier || kept.has(n.id)) && !orphaned(n))
    .concat([aggregate]);
  // AND THE LINKS THAT NAMED THEM GO TOO. Dropping the descendants without
  // dropping their links leaves foldDocument refusing "link aggregate/tail ->
  // dept/x names a node the document does not carry" -- which is the outcome
  // the filter above was added to prevent, reached one field over. Both halves
  // are latent on the shipped corpus, and latent is the state the comment above
  // claims not to leave things in.
  const present = new Set(nodes.map((n) => n.id));
  return Object.assign({}, doc, {
    nodes: nodes,
    links: doc.links
      .map((l) => Object.assign({}, l, { source: remap(l.source), target: remap(l.target) }))
      .filter((l) => present.has(l.source) && present.has(l.target)),
  });
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
 * @param {number[]} [tiers] the tier set to fold to, defaulting to the page's own
 *   RENDER_TIERS. A drill passes its own: the tier set a page OPENS ON and the
 *   one it opens INTO are two declarations, not one.
 * @returns {FiscProjection} doc itself when the tier set draws every tier.
 */
function foldDocument(doc, tiers) {
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
    // does.
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
  // re-pointing exists to prevent.
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
  // BUILT FROM THE FETCHED DOCUMENT AS WELL AS THE DRAWN ONE, because a colour
  // is a property of where a node sits in the real hierarchy and not of what
  // this page happens to draw. Built from the drawn nodes alone, every opened
  // view rendered in --muted: filterToNode keeps only what the drawn tiers
  // need, so a fund's fund-group ancestor is absent and fundGroupOf's walk
  // stops at the first parent it cannot resolve. Measured before the fix:
  // fundGroupOf returned "" for every node on all six opened fund groups.
  //
  // THE FETCHED HIERARCHY WINS, and the drawn nodes only fill ids it does not
  // have. foldDocument RE-POINTS a retained node's parent at its folded
  // ancestor and sets it to "" when that ancestor was filtered away -- which is
  // right for the folded document, whose own well-formedness is about nodes it
  // carries, and useless for a colour, which is about where the node really
  // sits. Taking the drawn parent leaves the walk stopping at the first "".
  // The aggregate is the node the drawn set contributes: the file has never
  // heard of it.
  //
  // THE FETCHED HIERARCHY IS THE DRAWN DOCUMENT'S, NOT THE YEAR'S. A rung can
  // be shaped from a different file than depth 0, and merging the year's
  // document over it would resolve the hue walk against the other document's
  // parents -- the failure fundGroupOf's comment records as every mark in
  // --muted, reached from the other side.
  const previous = groupIndex;
  groupIndex = new Map(doc.nodes.map((n) => [n.id, n]));
  const source = drawnDoc();
  if (source) {
    for (const n of source.nodes) groupIndex.set(n.id, n);
  }

  // WHICH COLUMN A NODE IS DRAWN IN IS A PROPERTY OF ITS TIER ONCE THIS PAGE
  // FOLDS. sankeyJustify aligns link-less sinks to the LAST column, which is
  // right for a document drawn whole and wrong the moment tiers can be skipped:
  // a node terminating early is shoved across the chart to sit among nodes it
  // shares nothing with. The spine is drawn whole and keeps sankeyJustify
  // exactly, which is why the crossing figures pinned in tools/jscheck do not
  // move.
  //
  // THE TIER SET IS THE ONE THE DOCUMENT WAS SHAPED BY, not the page's own, and
  // that distinction only exists because a page can open a node. A drilled
  // document is folded to its step's tiers -- {0,3} on Revenue against the page's
  // {0,2} -- so aligning on RENDER_TIERS gives every tier-3 fund
  // indexOf === -1, which d3 clamps to column 0. Measured: that leaves the
  // layer array with a hole and d3-sankey dies inside its own ordering pass
  // with "Cannot read properties of undefined (reading 'sort')" -- a blank
  // chart under a banner, on the first click of a feature whose whole point is
  // the click.
  const tiers = activeTiers();
  const align = tiers.length
    ? /** @param {LaidNode} d */ (d) => tiers.indexOf(d.tier)
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
  let graph;
  try {
    graph = sankey({
      nodes: doc.nodes.map((n) => Object.assign({}, n)),
      links: doc.links.map((l) => Object.assign({}, l, { value: l.value_cents })),
    });
  } catch (e) {
    // THE INDEX GOES BACK WITH THE THROW. It has to be assigned before the
    // sankey runs, because nodeRank walks it from inside d3's sort -- and a
    // document that will not lay out must not leave it describing that
    // document, or paint() on the next theme change recolours the chart still
    // on screen against a hierarchy it was never drawn from.
    groupIndex = previous;
    throw e;
  }
  restackLinks(graph);
  // Held for columnShare, which needs the LAID nodes: a share is of the column
  // d3-sankey put a mark in, and only this graph knows which that is.
  laidNodes = graph.nodes;
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
    // aria-pressed ONLY WHERE ACTIVATION IS A TOGGLE. The isolation is one, and
    // the legend announces its copy of it the same way; applyEmphasis keeps
    // this in step. Opening a node is NOT: it replaces the chart and the node
    // itself is gone from the result, so there is no pressed state to return
    // to and nothing the attribute could ever be true of. Announcing a toggle
    // that never toggles is worse than announcing nothing, because a reader who
    // hears "not pressed" is told there is a state to change.
    .attr("aria-pressed", /** @param {LaidNode} d */ (d) => (drillable(d) ? null : "false"))
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
    // ACTIVATION MEANS DIFFERENT THINGS ON DIFFERENT PAGES, declared per view
    // rather than decided here, and that is the answer to fisc-ppkq's third
    // objection: the click and the keydown are already spoken for by
    // setIsolated, and a second meaning on the same activation of the same
    // element is a new interaction contract rather than a reuse.
    //
    // It is declared and not overloaded. A page with a drill has TWO drawn
    // tiers, and isolating a node on a two-column graph says almost nothing --
    // every link a node has is already adjacent to it, so dimming the rest
    // dims a column the reader was not looking at. A page without one has
    // three, where following one fund group through the middle is the whole
    // point. So the pages that drill are exactly the pages that do not need to
    // isolate, and no page has to do both from one gesture.
    .on("click", /** @param {MouseEvent} e @param {LaidNode} d */ (e, d) => {
      e.stopPropagation();
      pin(d);
      const echo = d.id === keyActivation.id && e.timeStamp - keyActivation.at < 500;
      if (echo) return;
      if (drillable(d)) {
        openNode(d.id);
        return;
      }
      setIsolated(isolated === d.id ? "" : d.id);
    })
    .on("keydown", /** @param {KeyboardEvent} e @param {LaidNode} d */ (e, d) => {
      if (e.key !== "Enter" && e.key !== " ") return;
      if (e.repeat) return;
      e.preventDefault();
      keyActivation = { id: d.id, at: e.timeStamp };
      // Escape unpins while leaving focus where it was, so the panel can be
      // empty here even though focus already pinned this node once.
      pin(d);
      if (drillable(d)) {
        openNode(d.id);
        return;
      }
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
  // Left alone on a node that opens rather than toggles; see render(), which
  // does not give it the attribute at all.
  svg.selectAll("g.node").attr("aria-pressed", /** @param {LaidNode} d */ (d) =>
    (drillable(d) ? null : String(d.id === isolated && isolated !== "")));
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
  // WHAT ACTIVATING IT DOES, for a reader who cannot see that some marks open
  // and others dim. aria-pressed was correctly removed from a node that opens
  // -- there is no state to return to -- and removing it left a node announcing
  // NOTHING about the difference: the same words for a mark that replaces the
  // whole chart and one that dims the rest of it.
  // EVERY NODE SAYS WHAT ACTIVATING IT DOES. This gave the opening sentence to
  // drillable nodes and nothing at all to the others on the same page -- so
  // the eleven revenue categories, which still isolate and still carry a
  // toggling aria-pressed, announced no action whatever while the mark beside
  // them announced one.
  const what = drillable(d) ? ", opens into its parts" : ", follow this money";
  return d.label + ", total " + fmt(d.value) +
    (d.derived ? ", inferred by us" : ", printed by the city") + what;
}

/**
 * Where a caveat's full text is, or "" when this site has no caveats page.
 *
 * COMPOSED FROM THE DRAWN DOCUMENT'S OWN SUMMARIES rather than from the id,
 * because the anchor is per (document, caveat) -- one id carries different
 * text in different documents -- and the packager is the only party that
 * knows which stem the document on screen came from. Looking the id up in
 * what the page was handed is exact; rebuilding the fragment here would be a
 * second speller of a rule internal/export owns.
 *
 * THE DRAWN DOCUMENT'S REFS, NOT THE YEAR'S. caveatsFor reads the drawn
 * document's metadata.caveats, and at a rung over a switched document the
 * year's refs are the spine's -- so every caveat on a depth-1 mark resolved
 * to "" here, which is indistinguishable from a site with no caveats page,
 * and the panel rendered the summary with no link (fisc-ko1j.13). The year's
 * entry for the rung carries that document's refs; this reads them.
 *
 * @param {string} id
 * @returns {string}
 */
function caveatHref(id) {
  const refs = drilled.length
    ? (stepDocAt(drilled.length - 1) || { caveats: [] }).caveats
    : shownYear ? shownYear.caveats : [];
  if (!Array.isArray(refs)) return "";
  const ref = refs.find((c) => c.id === id);
  return ref && ref.href ? ref.href : "";
}

/**
 * The caveats that are about one drawn mark.
 *
 * RESOLVED THROUGH THE HIERARCHY, because applies_to names ids in the FILE and
 * a drawn node is often a fold of several of them. A caveat about
 * fund-group/internal-service should mark that group on the spine, where it is
 * drawn -- and on a page that folds its funds into it, where the id the caveat
 * names is a node the reader can see. So a caveat applies to a drawn node when
 * one of its targets IS that node or has it as an ancestor.
 *
 * THIS IS THE FOLD HAZARD fisc-yj4w.8 WAS FILED FOR, and it is the reason the
 * badge is not a map lookup: matching ids directly would leave the mark silent
 * on every page that folds, which is a test that passes whether the caveat
 * applies or the resolution is broken.
 *
 * @param {string} id
 * @returns {FiscCaveat[]}
 */
function caveatsFor(id) {
  if (!projection || !projection.metadata || !Array.isArray(projection.metadata.caveats)) {
    return [];
  }
  // THE AGGREGATE STANDS FOR THE IDS IT SWALLOWED. capColumn folds by VALUE,
  // not by ancestry, so the walk below cannot see that relationship -- the
  // node it folded has no parent pointing at the aggregate and never will.
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
  return projection.metadata.caveats.filter((c) =>
    Array.isArray(c.applies_to) && c.applies_to.some(reaches));
}

/**
 * The share one mark is of the money in its column, as a percentage.
 *
 * IT IS ARITHMETIC AND SAYS SO. Every figure this site publishes is one the
 * city printed; a share is not, and the word "of" carries that -- "8.4% of this
 * column" is self-evidently a ratio rather than a line item, in a way that a
 * bare "8.4%" beside a dollar figure would not be. It is computed from the
 * DRAWN values, so on an opened node it is a share of that node's own total,
 * which is what the reader is looking at.
 *
 * @param {LaidNode} d
 * @returns {string}
 */
function columnShare(d) {
  if (!d.value) return "";
  let total = 0;
  let siblings = 0;
  for (const other of laidNodes) {
    if (other.layer === d.layer) {
      total += other.value;
      siblings++;
    }
  }
  // NO SHARE OF A COLUMN OF ONE. An opened division's left column is that
  // division alone, so this printed "our 100.0% of this column" on the rung's
  // headline mark -- a derived chip carrying a figure that is 100% by
  // construction rather than by measurement. A share says how a column divides,
  // and an undivided one has nothing to say.
  if (!total || siblings < 2) return "";
  const pct = (100 * d.value) / total;
  // A CEILING AS WELL AS A FLOOR. toFixed(1) rounds, so a mark that is 99.9943%
  // of a divided column renders "100.0" -- the exact chip the siblings guard
  // above exists to prevent, reached by arithmetic instead of by topology.
  // Reproduced on committed data: fund-flows-2024-actual, revenue opened on
  // debt-service, where transfers/in is that share of a two-node column.
  //
  // ">99.9" AND "<0.1" ARE BOTH HONEST and "100.0" is not: the first two say a
  // figure is outside what one decimal can carry, and the third asserts a whole
  // that the presence of a sibling denies.
  const shown = pct < 0.1 ? "<0.1" : pct > 99.9 ? ">99.9" : pct.toFixed(1);
  // "◇" AND "our" BOTH, because the chip is small and a reader skims it. The
  // diamond is this site's mark for an inference everywhere else; the word is
  // what survives being read aloud.
  return "\u25c7 our " + shown + "% of this column";
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
    const share = columnShare(n);
    if (share) {
      meta.append(document.createTextNode(" "));
      // chip derived, LIKE EVERY OTHER FIGURE ON THIS SITE THAT WE COMPUTED. It
      // sat in a plain .chip beside "printed by the city", which is the one
      // adjacency this project's whole premise is about. A share is arithmetic
      // over two printed figures and is not itself printed anywhere.
      meta.append(h("span", "chip derived", share));
    }
    // A CAVEAT ABOUT THIS MARK, SAID AT THE MARK. The caveats page carries all
    // of them and every page links to it, which is right for the ones about a
    // schedule -- and wrong for the ones about a single node, which a reader
    // meets while looking at that node and not while reading a list.
    const cavs = caveatsFor(n.id);
    if (cavs.length) {
      meta.append(document.createTextNode(" "));
      meta.append(h("span", "chip caveat", cavs.length === 1
        ? "\u26a0 1 caveat" : "\u26a0 " + cavs.length + " caveats"));
    }
    tip.append(meta);
    if (n.rationale) tip.append(h("div", "tip-meta", n.rationale));
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
    const share = columnShare(n);
    if (share) chips.append(h("span", "chip derived", share));
    panel.append(chips);
    if (n.rationale) panel.append(h("p", "why", n.rationale));
    if (n.source_note) panel.append(h("p", "subtle", n.source_note));
    // THE CAVEAT IN FULL IS ONE CLICK AWAY, and the summary is here. The
    // tooltip can only afford the line; this panel is where a reader has asked
    // for the detail, so it is where the link belongs. The href is the same
    // anchor every caveat summary on the page uses -- composed by the packager
    // per (document, caveat), so it lands on THIS year's copy of the sentence.
    for (const c of caveatsFor(n.id)) {
      const why = h("p", "why");
      why.append(document.createTextNode("\u26a0 " + c.summary + " "));
      const href = caveatHref(c.id);
      if (href) why.append(link("Read it in full", href));
      panel.append(why);
    }
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

/**
 * Draws one swatch per fund group on the chart, each a toggle for that group's
 * isolation.
 *
 * EMPTY ON EVERY OPENED VIEW, BY RULE AND NOT BY ACCIDENT. It was empty there
 * already, because filterToNode keeps no fund-group node once a group is
 * opened -- but that is a side effect, and on a page that opens the spine the
 * legend vanishing on the first click is a decision the code has to own. The
 * decision: an opened view is one node's subtree, every mark in it resolves
 * to the one fund group above that node, and a key that distinguishes groups
 * has nothing to distinguish. The breadcrumb names the group and every ribbon
 * wears its hue. And the swatch is a toggle on a NODE id -- setIsolated dims
 * whatever is not adjacent to it -- so a swatch for a group not on the chart
 * would dim the whole chart. fisc-ko1j.11, which opens a revenue category
 * whose subtree spans groups, is where this rule would have to change, and
 * with it the isolation it rests on.
 */
function buildLegend() {
  if (!projection) return;
  const legend = el("legend");
  legend.replaceChildren();
  if (drilled.length) return;
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
  // NO ARM FOR metadata.caveats, AND THE REASON IS THIS GATE'S OWN RULE. It
  // refuses a document over every key the draw DEREFERENCES, because a missing
  // one throws mid-repaint and leaves a half-painted page. caveatsFor
  // dereferences neither: it tests Array.isArray on the block and on each
  // applies_to and returns [] otherwise.
  //
  // So an arm here would refuse a whole chart over a key whose absence costs a
  // BADGE. A reader holding a cached pre-caveats document -- the case this gate
  // exists for, since the year files are fetched lazily with no cache-busting --
  // would get a banner instead of a chart that draws perfectly minus one chip.
  // That is a worse outcome than the one being prevented, and it was shipped
  // for one commit on a justification that read "caveatsFor dereferences
  // applies_to", which the guarded code makes false.
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
 * Fetches one document and vets it, returning it, or null with the reader told
 * why -- unless `superseded` says nobody is waiting, in which case nothing is
 * painted and null is returned without a word.
 *
 * ONE FETCH PATH FOR THE YEAR AND FOR A STEP. Every guard here -- the HTTP
 * status, the body that will not parse, isDocument, understands, drawableSankey
 * -- has two callers now, and a click's fetch that skipped one would draw at
 * depth 1 a file the year control refuses at depth 0. `superseded` IS
 * CONSULTED BEFORE EVERY BANNER: without that, a reader who switched away
 * while a fetch was failing got the file:// remediation banner -- role="alert"
 * -- pasted over a year that drew correctly, the page asserting something
 * untrue about what is on screen.
 *
 * BOTH AWAITS ARE INSIDE THE TRY. response.json() used to sit outside it, so a
 * 200 with a truncated or malformed body rejected out of this function entirely
 * -- into main()'s .catch on the opening path, and into nothing at all from the
 * year control, which is a page half-repainted between two years with no
 * banner.
 *
 * @param {string} path
 * @param {() => boolean} superseded
 * @returns {Promise<FiscProjection | null>}
 */
async function loadDocument(path, superseded) {
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
      ? "Could not read " + path + ": the file is not valid JSON, so it is " +
        "truncated or was not the document this page expected."
      : "Could not load " + path + ". If you opened this file directly, the browser " +
        "blocks the request: serve the directory over HTTP instead, e.g. " +
        "python3 -m http.server -d dist 8000");
    return null;
  }
  if (superseded()) return null;
  // The fetched file is what actually gets drawn, and it is a separate
  // document from the config: the packager stamps the config from the
  // projection it was handed, so agreeing with the config is not evidence the
  // file on the wire agrees too.
  if (!isDocument(doc, path)) return null;
  if (!understands(doc.schema_version, path)) return null;
  if (!drawableSankey(doc, path)) return null;
  return doc;
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
  // page draws a year the control does not show. Compared after every await,
  // inside loadDocument and once more here.
  const token = ++switching;
  const doc = await loadDocument(year.path, () => token !== switching);
  if (token !== switching) return SUPERSEDED;
  if (!doc) return FAILED;

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
  fetched = doc;
  // A DRILL BELONGS TO THE YEAR IT WAS MADE IN, and this reset has to happen
  // BEFORE the shaping rather than with the pin and the isolation below it. A
  // fund group opened in FY2025-26 may not exist in FY2023-24 -- that column
  // carries a seventh, permanent (fisc-zojk) -- and shapeFor would filter the
  // new document to a subtree of nothing and hand d3-sankey an empty graph.
  // The pin and the isolation are cleared after the draw because they only
  // decorate it; this decides what is drawn.
  //
  // THE WHOLE STACK, AND THE STEP DOCUMENTS WITH IT. A step's file was fetched
  // for the year it was opened in, and the next drill fetches it again for
  // this one rather than draw the year the reader left one rung down.
  drilled = [];
  stepDocs = new Map();
  const drawn = shapeFor(doc);
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
  paintBreadcrumb();
  buildLegend();
  paintChartHint();
  buildDerivedList();
  buildTable();
  render(laid);
  return DREW;
}

/**
 * Replaces every word on the page that belongs to a year: the hero, the tiles,
 * the caveats, the caveat count in their summary, the lede, the flow count, the
 * chart's accessible name and description (through paintChartName), the
 * footer's basis and its data-file citation, and the document title.
 *
 * THE LIST IS EXHAUSTIVE ON PURPOSE. It read "the tiles, the caveats, the lede
 * and the flow count" while the function wrote four more, and a doc comment
 * that undercounts its own writes is how the next per-year string gets added to
 * the template and forgotten here -- which is the fisc-kwq / fisc-yi4 / fisc-iyt
 * defect three times over. If you add a write, add it above.
 *
 * The page already carries the opening year's, rendered server-side so the
 * headline survives with JavaScript off. This swaps them for another year's,
 * and every string it writes was built by the packager -- except the counts
 * line, whose shape depends on what is drawn, so paintCounts composes it from
 * the packager's counts and tools/jscheck/year.mjs pins its undrilled wording
 * to the template's own sentence.
 * @param {FiscYear} year
 */
function paintYearWords(year) {
  const tile = (f) => {
    const el = h("div", "tile" + (f.kind ? " " + f.kind : ""));
    el.appendChild(h("div", "label", f.label));
    el.appendChild(h("div", "value", f.value));
    el.appendChild(h("div", "note", f.note));
    return el;
  };

  // TWO CONTAINERS, EACH OWNED WHOLE. The hero sits above the chart and the
  // rest of the tile row inside a closed disclosure below it, so one
  // replaceChildren over #figures would paint the headline into the collapsed
  // panel and leave the tile above the chart reading the year the reader left.
  //
  // maybeEl for both: chart.html.tmpl renders neither, deliberately -- see the
  // comment at the head of its <main>, which is about why a page drawing one
  // grain of one document must publish no total.
  const hero = maybeEl("hero");
  if (hero) hero.replaceChildren(tile(year.hero));

  const figures = maybeEl("figures");
  if (figures) figures.replaceChildren(...year.figures.map(tile));

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

  shownYear = year;
  paintCounts();

  // THE CHART'S ACCESSIBLE NAME IS BUILT IN GO, like every other string this
  // function writes. It was composed here from a literal, and the moment a
  // second page drew a chart that literal was WRONG on it: the drill-down's
  // template names a diagram "by fund and division", and the first repaint
  // replaced that with the spine's wording -- so two different charts announced
  // themselves identically to a screen reader. Same defect as fisc-rn0, which
  // is why sankeyTitle exists, reached through the one string that had not been
  // moved yet.
  // DELEGATED, so a year switch and a drill cannot write this element
  // differently. paintChartName also restores the <desc>, which paintYearWords
  // never touched and which a drill rewrites.
  paintChartName();

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
      // ESCAPE UNDOES ONE THING, AND THE INNERMOST ONE FIRST. A pin or an
      // isolation is a selection inside the chart; an opened node is the chart.
      // Doing both on one press meant a reader dismissing a provenance panel
      // also lost the group they had opened -- and the comment beside it
      // claimed the opposite, which is how it got written. Two presses close
      // both, in the order a reader made them.
      // AN OPENED NODE IS CLOSED ONLY WHEN THERE IS NOTHING INSIDE IT TO
      // CLEAR, AND ONE RUNG AT A TIME. drillUp repaints everything and clears
      // the pin and the isolation itself, so this branch does nothing else;
      // two rungs deep, one press closes the inner rung and leaves the outer,
      // which is the order the reader opened them in.
      if (!pinned && !isolated && drilled.length) {
        drillUp(drilled.length - 1);
        return;
      }
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
