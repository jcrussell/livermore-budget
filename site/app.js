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
 * @property {string} [contra]  the words for a link the schedule printed as a
 *   reduction, e.g. "printed as a reduction of Property Taxes". PRESENT ONLY
 *   ON A LINK THE CLIENT FLIPPED: a published link carries a signed
 *   value_cents and no such field; markContra draws the negative ones at
 *   their magnitude and records here what the sign meant.
 * @property {boolean} [partition]  the ribbon divides one printed table along
 *   a second axis rather than following money the schedule prints as moving
 *   that way. THE PROJECTION'S, unlike contra: the client cannot tell a
 *   cross-tab from a chain by looking at a graph, and a page that guessed
 *   would be deciding what a published table means. PARTITION_NOTE carries the
 *   words, in one place, for every mark that shows them.
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
 * @property {string} [rungs]  where Go's answer for every rung this page opens
 *   is served; absent means nobody answers this page's rungs
 */

/**
 * Go's answer for every rung the drill walks, at every column budget: what
 * `fisc export` writes and this page reads rather than deriving.
 *
 * @typedef {Object} FiscRungs
 * @property {number} schema_version
 * @property {FiscRungColumn[]} columns
 */

/**
 * One published year's rungs, by the spine document's stem.
 * @typedef {Object} FiscRungColumn
 * @property {string} stem
 * @property {FiscRung[]} rungs
 */

/**
 * One opened path at one column budget.
 * @typedef {Object} FiscRung
 * @property {string[]} path  the nodes opened, outermost first
 * @property {number} width  the column budget this entry answers for
 * @property {string} step  the key of the step that opened the last node
 * @property {FiscDrawnTier[]} draws  every column the window draws, in the
 *   order it draws them
 */

/**
 * One column of one rung: which nodes it holds, and how many its folded tail
 * stands for.
 *
 * `ids` is written even when empty, so a column answered with nothing is told
 * apart from a column not answered at all; `carried`, `cap` and `hidden` are
 * omitted at zero, so a reader that compared undefined would refuse every
 * unfolded column.
 * @typedef {Object} FiscDrawnTier
 * @property {number} tier
 * @property {string} role  centre, flank or outward
 * @property {number} [cap]
 * @property {number} [candidates]  how many document nodes the window reaches
 * @property {string[]} ids
 * @property {string[]} [carried]
 * @property {number} [hidden]
 */

/**
 * @typedef {Object} FiscTierCap
 * @property {number} tier
 * @property {number} cap  how many nodes the tier may hold before its tail is
 *   folded into one aggregate; see capColumn for why a cap is needed at all.
 * @property {string} [tail]  the plural noun this tier's tail is counted in;
 *   absent, the step's own
 */

/**
 * One step of the tree the packager ships, verbatim from export.DrillStep.
 *
 * @typedef {Object} FiscDrillStep
 * @property {string} key  what other steps name this one by
 * @property {string[]} after  the keys of the steps whose charts this one
 *   opens from, carrying "" for the view's own chart
 * @property {string} [side]  which end of a link the opened node is: absent
 *   for the end links point at, "source" for the end they come from
 * @property {string} [role]  which nodes at `from` open, by node.role; absent
 *   opens every node at the tier
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
 * @property {Record<string,string>} [residual]  the endpoints of the chart
 *   this step opens FROM whose flow the document it draws does not
 *   decompose, id to reason -- the check's declaration as the packager
 *   shipped it; absent on a step that switches no document
 * @property {Record<string,string>} [gaps]  the nodes this step OPENS whose
 *   total the document it draws does not reach, id to the declared reason the
 *   two documents print one cell at two figures; absent on a step that makes
 *   no claim that its opened nodes balance
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
 * How this page drills: the steps the packager declared, empty for a page
 * that opens nothing.
 *
 * PER VIEW AND NEVER A CONSTANT HERE, for render_tiers' reason: the spine and
 * the two fund-flows pages are drawn by the same script from different
 * hierarchies, and a page that declares nothing keeps the isolate-on-click
 * behaviour it has always had.
 *
 * A TREE, READ BY KEY. Each step names the charts it opens from -- `after`,
 * a list carrying "" for the view's own -- and the tier its nodes are at, so
 * two steps can open from one chart: the spine's fund groups and its revenue
 * categories open into different views of the same document, and a depth
 * cannot tell them apart. Several entries are one chart reachable from
 * several, which is the other direction and the same list. stepFor is the one
 * reader of that rule. The packager validates the tree
 * (export.validateSteps): keys unique, every entry naming an earlier step, at
 * most one step per (after, from, role).
 *
 * @type {FiscDrillStep[]}
 */
const STEPS = CONFIG && Array.isArray(CONFIG.steps)
  // A STEP WITH NO KEY OR NO PARENTAGE IS NOT A STEP OF THE TREE, and is
  // dropped as one with no tiers is. What is refused is parentage that is
  // ABSENT, and the list refuses it where the string did: a step whose `after`
  // was dropped carries `undefined`, which is not an array and falls out here
  // rather than being read as a root. A step read as a root is one its own
  // children match by "" -- measured, on a config whose steps carried no keys:
  // Patrol opened into Patrol without end, because the division step matched
  // from its own chart. A root says so by carrying "" IN the list, and the
  // packager requires that rather than an empty one (export.validateSteps).
  ? CONFIG.steps.filter((s) => s && Array.isArray(s.tiers) && s.tiers.length > 0 &&
      typeof s.key === "string" && Array.isArray(s.after) &&
      s.after.every((a) => typeof a === "string"))
  : [];

/**
 * The key of the step whose chart is on screen, "" on the overview.
 * @returns {string}
 */
function openedKey() {
  return drilled.length ? drilled[drilled.length - 1].step.key : "";
}

/**
 * The step this node of the chart on screen opens into, or null when it opens
 * nothing.
 *
 * THREE MATCHES AND NOT A DEPTH. The step opens from the chart on screen
 * (`after` CONTAINS the rung's key), from this node's tier (`from`), and --
 * when it names one -- from nodes in this role. Depth alone answered while the
 * steps were a line; on the spine two steps open from depth 0, one per tier.
 *
 * THE FIRST MATCH IS MEMBERSHIP AND NOT EQUALITY, which is what lets one chart
 * be reached from several: a step listing two keys is one view the reader can
 * arrive at by either route, and it opens the same way whichever they took.
 *
 * THE ROLE IS THE PACKAGER'S GATE, MATCHED AND NOT INFERRED. transfers/in and
 * fund-balance/draw sit at tier 0 beside the ten revenue categories, and
 * pp.127-140 print nothing beneath either; a step declaring `role:
 * "revenue_source"` opens the categories and leaves the endpoints as the
 * flow's ends. Deriving the same answer from an id prefix or from what the
 * step document happens to carry would be this file deciding what a tier
 * means, which paintBreadcrumb's comment refuses.
 *
 * AND A FOURTH MATCH THAT IS A LOOKUP RATHER THAN A RULE: whether the year's
 * own document for that step decomposes THIS NODE. A role says what a node is,
 * which is the right question for a flow endpoint and the wrong one for a fund
 * -- Budget Book pp.85-125 name no row for 6 of the 61 funds the drill-down
 * draws in FY2025-26, and nothing about fund/511 distinguishes it from
 * fund/512. The packager reads the set off each year's document
 * (export.openableNodes) so this file decides nothing: a step that ships no
 * `opens` declares no such set and every node at its tier opens, which is what
 * the transfers step and every step before this one did.
 *
 * IT IS HERE AND NOT IN drillDown, because the affordance is the thing at
 * stake. The click already fails closed -- filterLinks refuses a node its
 * document does not carry, in words -- and what that produces is a mark drawn
 * with the triangle, announced as openable, that banners when a reader
 * activates it. Measured before this clause existed, over both committed
 * columns: drillDown(fund/511) failed and left the chart on fund-group/capital.
 *
 * @param {{id?: string, tier: number, role?: string}} node
 * @returns {FiscDrillStep | null}
 */
function stepFor(node) {
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
 * Whether the year's document for `step` draws anything under `id`.
 *
 * TRUE WHEN NOTHING SAYS OTHERWISE, and the asymmetry is deliberate. The
 * packager omits `opens` from a step that declares no set, and refuses to ship
 * an EMPTY one -- a window whose document decomposes nothing at its opened tier
 * is a rung no reader could reach, and export.stepDocuments reports it by name
 * rather than writing `[]` here for this function to read as "nothing opens".
 * So a missing key has exactly one meaning and this can default open.
 *
 * A NODE WITH NO ID IS OPEN FOR THE SAME REASON. stepFor is asked about a
 * `{tier, role}` shape by callers that have no node in hand, and answering
 * "closed" to those would close a rung on a question that was never asked.
 *
 * @param {FiscDrillStep} step
 * @param {string | undefined} id
 * @returns {boolean}
 */
function stepDecomposes(step, id) {
  if (!id) return true;
  const entry = stepDocFor(step);
  if (!entry || !Array.isArray(entry.opens)) return true;
  return entry.opens.includes(id);
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
const CHART_HEIGHT = 820;

/**
 * Room reserved either side of the plot for node labels, in px. Sized from
 * the widest label this data produces — "Fund Balance Contribution  $12.8M ◇"
 * at roughly 230px — because a label that does not fit must not be clipped,
 * and there is nowhere else for a sankey node's name to go.
 *
 * IT DOES NOT GROW WITH THE COLUMN COUNT. A gutter is what a label anchored
 * OUTWARD runs into, and labelPlacement anchors outward on the two end columns
 * alone: every interior label is centred above its own rect. So two gutters
 * serve a chart of any width, and adding one per column would buy room for
 * labels no column asks for.
 */
const LABEL_GUTTER = 250;

/**
 * The clear horizontal run between one column's rects and the next's, in px:
 * what a ribbon crosses.
 *
 * IT IS THE BAND AND NOT THE PITCH, which is what makes it the constant to
 * hold fixed as columns are added. d3-sankey spreads its columns over the
 * extent at (width - NODE_WIDTH)/(columns - 1), so a chart sized by chartWidth
 * below gives every band exactly this many px whatever the count.
 *
 * 319 BECAUSE THREE COLUMNS MUST COME TO 1180 EXACTLY. That was the chart's
 * fixed design width while three columns was the only shape, and every figure
 * layout.mjs pins -- the crossings, the overlapped value, every label's
 * clearance -- is of a chart laid out at it. A band chosen for its own sake
 * would move all of them at once and none of them for a reason.
 */
const BAND = 319;

/**
 * How wide a chart of `n` columns is laid out, in px.
 *
 * The chart is laid out at a fixed size and scaled by the viewBox, rather than
 * re-laid-out at the container's width. A sankey's labels do not reflow: at
 * 700px the three columns and their labels collide, and the only honest fixes
 * are a horizontal scrollbar or a fixed design width that shrinks as a whole.
 * This is the second, and a fourth column makes the design width a function of
 * the count rather than a constant.
 *
 * THE DRAWING SCALES, THE CONTAINER DOES NOT. The viewBox is what fits this
 * width into whatever room style.css gives the <svg>, so a wider chart in the
 * same container is the same picture drawn smaller. That is why the column
 * budget is asked of the viewport (COLUMN_QUERIES) rather than taken whenever
 * a step offers one.
 *
 * @param {number} n
 * @returns {number}
 */
function chartWidth(n) {
  // A CHART HAS A COLUMN. d3-sankey divides by (columns - 1) and a count of 0
  // or 1 has no band at all; clamping here keeps the width finite rather than
  // letting a degenerate tier set reach the extent.
  const columns = Math.max(1, n);
  return 2 * LABEL_GUTTER + BAND * (columns - 1) + NODE_WIDTH * columns;
}

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
 * A figure with its sign, for a mark whose figure the schedule prints as a
 * reduction.
 *
 * THE MINUS IS THE FIRST SIGNAL AND THE COLOUR THE SECOND, for the reason the
 * series table gives its contra rows a minus rather than the city's
 * parentheses: screen readers do not announce parentheses at default settings
 * and would read the figure aloud as positive, and a hue is never the only
 * signal on this site. U+2212 rather than a hyphen, so it reads as a sign.
 * @param {number} cents
 * @returns {string}
 */
function fmtSigned(cents) {
  return (cents < 0 ? "\u2212" : "") + fmt(Math.abs(cents));
}

/** @param {number} cents */
function fmtShortSigned(cents) {
  return (cents < 0 ? "\u2212" : "") + fmtShort(Math.abs(cents));
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
 * @property {FiscProjection} chart  the chart that was ON SCREEN when this rung
 *   was opened: shaped, capped, folded and carried, as the reader saw it.
 *
 *   RECORDED RATHER THAN RECOMPUTED, because a window's kept flank is a filter
 *   of it and a rung is reshaped long after the click -- Escape pops back to a
 *   rung whose parent chart is no longer on screen, and shapeFor would have
 *   nothing to filter. It is the chart, not the file: docAt() answers for the
 *   file one depth up, and the flank the reader came from is the one they were
 *   looking at, capped tail, residual and all.
 * @property {Set<number>} [expanded]  the tiers of THIS rung's chart the reader
 *   has opened out, drawn at every mark they hold rather than at the step's cap.
 *
 *   ON THE RUNG AND NOT ON THE PAGE, because the stack is the path and an
 *   expansion is a property of one chart on it. Tier 3 is capped in the
 *   fund-group window and drawn whole in the fund window, and a page-level set
 *   would carry "show me all of them" from the first into the second, out of a
 *   pop, and into the next fiscal year -- whose column is not the same column.
 *
 *   IT IS CARRIED THROUGH A DRILL BY CONSTRUCTION, because `chart` above is
 *   recorded rather than recomputed: a window opened from an expanded chart
 *   keeps the flank as the reader saw it, expansion and all, with no code here.
 *   LATENT on the committed corpus -- measured, every kept flank the shipped
 *   steps declare is a single-node column or the spine's ten categories, so no
 *   flank has ever carried a capped tail -- and latent is how it ships.
 * @property {number[]} [dropped]  the widened tiers this rung's document left
 *   empty, so activeTiers stops asking for them.
 *
 *   IT OUTLIVES THE BUDGET THAT REVEALED IT, and that is right rather than
 *   convenient: "this document draws nothing at that tier" is a property of the
 *   document, so a reader who widens the page later is not shown a column that
 *   was empty when it was last asked for. A narrower budget drops it anyway.
 */

/**
 * The nodes the chart is opened into, outermost first; empty on the overview.
 *
 * A STACK, BECAUSE A PATH THROUGH THE TREE IS STILL A LINE. Each rung records
 * the step that opened it, the next step is the one naming that step's key
 * (stepFor), and the breadcrumb shows one rung per step taken. Two steps can
 * open from one chart, and one reader can still only take one of them at a
 * time, so what is on screen is always a path even though what is declared
 * is not.
 * @type {Rung[]}
 */
let drilled = [];
/**
 * The fewest columns any chart this page draws is laid out in: a window's kept
 * flank, the node the reader opened and what it opens into.
 *
 * A FLOOR AND NOT A DEFAULT. export.validateSteps holds a window to exactly
 * these three plus one per widening, so a budget under it would drop a column
 * that is not optional and leave the centre against a wall.
 */
const NARROW_COLUMNS = 3;
/**
 * The most columns this page has room to draw.
 *
 * IT IS A MEASUREMENT OF THE STYLESHEET, NOT A TASTE. style.css caps
 * .chart-wrap's --chart-room at chartWidth(4), and the chart is an <svg> with a
 * viewBox: a budget above this does not draw a wider chart, it draws the same
 * picture smaller. tools/jscheck/layout.mjs reads the cap off the shipped
 * declaration and this constant off the shipped script, so raising one without
 * the other goes red rather than shipping a reader who asks for a column and
 * gets less chart.
 *
 * NOTHING ASKS FOR A FIFTH EITHER: the widest step this site declares is the
 * fund step's four tiers, so a budget of 5 would change no chart on the page
 * even with room for one. Both halves of what reopening that needs are in
 * fisc-ipif.
 */
const WIDE_COLUMNS = 4;

/**
 * The viewport widths that buy a column beyond the floor, and what each buys.
 *
 * 1569 IS chartWidth(4) PLUS THE STYLESHEET'S OWN 56px CUSHION, and it is that
 * rather than a round 1500 because the threshold has to be the width at which
 * the fourth column FITS: --chart-room is calc(100vw - 56px) below its cap, so
 * at a 1500px viewport a four-column chart is drawn at 95% of the width it was
 * laid out at. A breakpoint chosen for looking like a breakpoint reintroduces
 * the defect in miniature. layout.mjs re-derives this from the two files.
 *
 * ASKED THROUGH matchMedia AND NOT THROUGH resize, because a query is the
 * question being asked -- "is there room for another column" is a threshold,
 * not a stream of widths -- and because matchMedia is already feature-checked
 * here for the OS theme and already observable to a check.
 *
 * ONE-DIRECTIONAL, AND THE FLOOR IS NARROW_COLUMNS. A wide viewport can ADD a
 * column; a narrow one cannot take the page below three, because three is what
 * a window IS -- windowFor refuses fewer -- and because chartWidth is a fixed
 * design width scaled by the viewBox, so narrowing the window shrinks the
 * picture rather than reflowing it (see chartWidth). Nothing here reads a
 * viewport as a reason to draw FEWER columns than the floor.
 */
const COLUMN_QUERIES = [
  { query: "(min-width: 1569px)", columns: WIDE_COLUMNS },
];

/**
 * How many columns the chart may draw, which is what decides whether a step's
 * widened columns are asked for (activeTiers).
 *
 * A MODULE-LEVEL let, AND THAT IS NOT THE THING RENDER_TIERS FORBIDS. That
 * comment refuses a constant because a tier set is WHICH TIERS A DOCUMENT IS
 * DRAWN AT -- a property of a hierarchy, which only a view can declare, and
 * which applied to the wrong document refuses or corrupts. A budget is a
 * property of the READER'S WINDOW and it never names a tier: every tier drawn
 * is still one the step declared, and the budget only chooses how many of them
 * to take (activeTiers). So it is per-reader rather than per-view, and there is
 * no document it could be wrong about.
 *
 * SEEDED BEFORE THE FIRST FETCH AND MOVED ONLY THROUGH setColumnBudget.
 * wireColumns owns both.
 */
let columnBudget = NARROW_COLUMNS;

/**
 * The column count the reader asked for, or null when they have not asked.
 *
 * THE READER OUTRANKS THE VIEWPORT UNTIL THEY HAND IT BACK. While this is set,
 * a media query firing changes nothing -- otherwise a reader who stepped down
 * to three would be silently returned to four by rotating a tablet. Stepping
 * back to the count the viewport itself would give clears it, which is the only
 * way back to following the window and is why this is a separate value rather
 * than being read off columnBudget: the two are equal in exactly the state
 * where the reader has NOT chosen.
 */
let columnOverride = null;

/**
 * Sets how many columns the chart may draw, and says whether that moved.
 *
 * THE CALLER REDRAWS, THIS DOES NOT. A budget change is a relayout of whatever
 * is on screen, and the two callers that will want one -- a media query firing
 * and a reader's override -- differ in what else they repaint; a redraw from
 * inside here would also fire during the opening paint, before there is a
 * document to lay out.
 *
 * CLAMPED RATHER THAN REFUSED, because the caller is a media query and not a
 * declaration: a viewport with room for two columns is a real state, and a
 * chart of two columns is not.
 *
 * @param {number} n
 * @returns {boolean} whether the budget changed
 */
function setColumnBudget(n) {
  const want = Math.max(NARROW_COLUMNS, Math.floor(Number(n)) || NARROW_COLUMNS);
  if (want === columnBudget) return false;
  columnBudget = want;
  return true;
}

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
 * How long after one activation of a node a later event on the same node is
 * taken to be part of that same activation, in milliseconds.
 *
 * ONE WINDOW FOR TWO ECHOES, because they are the same problem twice. Assistive
 * tech may synthesise a click from the Enter or Space it has just delivered,
 * and a pointer always delivers two clicks before the dblclick they compose
 * into. In both cases a later event belongs to an activation already handled,
 * and in both cases the only thing telling it from a real second activation is
 * how soon it arrived on the same node.
 */
const ACTIVATION_WINDOW = 500;

/**
 * The node and timestamp of the last Enter/Space activation, so that the click
 * some assistive tech synthesises from that same key press does not undo it.
 * @type {{id:string, at:number}}
 */
let keyActivation = { id: "", at: -Infinity };

/**
 * The node a click last isolated, when, and what had been isolated before it,
 * so that the double click a pair of clicks composes into can put that back.
 *
 * THE CLICK IS NOT DEBOUNCED, AND THIS IS WHAT THAT COSTS. A mark carrying both
 * handlers delivers click, click, dblclick. Waiting the window out before
 * acting on the first would put ACTIVATION_WINDOW of lag on every isolate, on
 * every node, to serve a gesture most readers never make -- so the clicks act
 * at once and the dblclick puts back what they changed. That is keyActivation's
 * shape with the replaced state carried along beside the timestamp.
 *
 * `was` IS RECORDED ONCE PER WINDOW AND NOT ONCE PER CLICK. The second click of
 * a pair arrives with the first click's isolation already applied, so
 * refreshing `was` on it would record the state the gesture itself produced and
 * faithfully restore that.
 * @type {{id:string, at:number, was:string}}
 */
let clickIsolate = { id: "", at: -Infinity, was: "" };

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
 * needed because the windows that call it want opposite things from it:
 *
 *   - A fund group's window keeps the spine's own revenue categories beside the
 *     group at tiers {0,2}. That context column sits OUTSIDE the subtree and
 *     must be kept; requiring both ends inside would delete it and draw a group
 *     fed by nothing.
 *   - A division's window keeps the fund that pays for it at {3,4}, off a chart
 *     that also carries the fund group above it. The group-to-fund link points
 *     at a node the subtree does not hold and is excluded by the rule above;
 *     what the placeability test is for is the end no column can hold at all,
 *     which is why a tier set naming two adjacent columns can be handed a
 *     document carrying six.
 *
 * Keying on the target says which end is the fine one; the placeability test
 * says what this drill's tier set has room for. Neither is a silent loss: a
 * link dropped here is one the declared tier set has no column for, which is a
 * statement the view made when it declared them -- and scoped() is what keeps
 * that sentence true, by refusing the one case it was false of.
 *
 * @param {FiscProjection} doc
 * @param {string} id
 * @param {number[]} tiers
 * @returns {FiscProjection}
 */
function filterToNode(doc, id, tiers) {
  return filterLinks(doc, id, tiers, (l, inside) => inside.has(l.target));
}

/**
 * The document restricted to the money leaving one node's own lines.
 *
 * A SIBLING OF filterToNode AND NOT A PARAMETER ON IT, because the two
 * disagree about what "inside" means. That one keeps a link whose TARGET is in
 * the subtree, so the column feeding the opened node stays as context; this
 * one keeps a link whose SOURCE is, so the column the opened node feeds stays.
 * A revenue category is a tier-0 node with nothing pointing at it, and asked
 * through filterToNode it answers an empty graph with no error -- the id is
 * known, so the guard that fires on an unknown one does not -- and d3-sankey
 * dies on the empty graph with "RangeError: Invalid array length". One
 * function answering both questions by flag is how a caller gets the wrong
 * one.
 *
 * THE RULE IS "SOURCE IN THE SUBTREE, BOTH ENDS PLACEABLE". Asked of the spine
 * for a revenue category at tiers {0,2}, it keeps the category and the fund
 * groups the spine draws its money reaching -- which is the kept half of that
 * category's window, and the half that puts the node the reader clicked back on
 * the screen. Asked of pp.127-140 for the same category it would keep every
 * fund a line lands in and draw the category nowhere, since there the category
 * is the source of nothing: the schedule prints money at the line, and the
 * category is the line's parent.
 *
 * @param {FiscProjection} doc
 * @param {string} id
 * @param {number[]} tiers
 * @returns {FiscProjection}
 */
function filterFromNode(doc, id, tiers) {
  return filterLinks(doc, id, tiers, (l, inside) => inside.has(l.source));
}

/**
 * The filter both drills share: the links `keeps` admits whose ends this tier
 * set can place, and the nodes those links need.
 *
 * AN EMPTY RESULT IS REFUSED BY NAME, in the sentence the guard on an unknown
 * id already uses. Every other route to an empty graph ends in d3-sankey's
 * "RangeError: Invalid array length", a stack trace where a sentence belongs;
 * a node the document carries and draws nothing under is a fault in the view
 * or the document, not in the reader's click.
 *
 * @param {FiscProjection} doc
 * @param {string} id
 * @param {number[]} tiers
 * @param {(l: FiscLink, inside: Set<string>) => boolean} keeps
 * @returns {FiscProjection}
 */
function filterLinks(doc, id, tiers, keeps) {
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
  const inside = withinNode(doc, id);
  const placeable = scoped(doc, tiers);

  const links = doc.links.filter((l) => {
    if (!keeps(l, inside)) return false;
    const src = byID.get(l.source);
    const dst = byID.get(l.target);
    if (!src || !dst) return false;
    return placeable(src) && placeable(dst);
  });
  if (!links.length) {
    throw new Error("cannot draw " + doc.projection + ": nothing flows between tiers " +
      tiers.join(", ") + " for node " + id + ", so there is no chart to open it into");
  }

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
 * Whether a link end has a column in this tier set, or a throw when the
 * question cannot honestly be answered "no".
 *
 * A DROPPED END AND A BROKEN CHAIN LOOKED THE SAME. The filters drop a link
 * whose end folds to nothing, and the comment above calls that "a statement
 * the view made when it declared its tiers" -- true of a tier the view left
 * out, and false of a node whose parent chain is broken, which foldTarget also
 * answers "" for. Measured (fisc-ng17): with revenue-line/taxes/property/eraf's
 * parent blanked, the opened General Fund drew Property Taxes at $79,318,762
 * against p127's $64,143,762, because ERAF is a contra row and dropping it
 * removed a negative -- with identical node and link counts and no banner.
 * foldDocument would have refused the node, but a rung filters first and the
 * fold never saw it.
 *
 * TWO SHAPES ARE BROKEN, AND ONE IS NOT. A parent naming a node the document
 * does not carry is broken outright. A node with no drawn ancestor while OTHER
 * nodes of its tier have one is broken too: the view found a column for that
 * tier, and this node's chain is what failed to reach it. A tier no node of
 * which can be placed is the view's own declaration -- the fund column under a
 * division opened at {4,5}, the revenue column under Spending's old {3,4} --
 * and its links are dropped as before. What this cannot see is a whole tier
 * losing its parents at once, which node-hierarchy-well-formed refuses Go-side;
 * the drill-down's line tier is the arm fisc-ko1j.10 added for exactly that.
 *
 * @param {FiscProjection} doc
 * @param {number[]} tiers
 * @returns {(n: FiscNode) => boolean}
 */
function scoped(doc, tiers) {
  const byID = new Map(doc.nodes.map((n) => [n.id, n]));
  const drawn = new Set(tiers);
  /** Tiers at which some node has a drawn ancestor. */
  const placed = new Set();
  for (const n of doc.nodes) {
    if (foldTarget(byID, n, drawn) !== "") placed.add(n.tier);
  }
  return (n) => {
    if (foldTarget(byID, n, drawn) !== "") return true;
    let at = n;
    for (let hops = 0; at.parent && hops < 9; hops++) {
      const up = byID.get(at.parent);
      if (!up) {
        throw new Error("cannot draw " + doc.projection + ": node " + at.id + " names parent " +
          at.parent + ", which the document does not carry");
      }
      at = up;
    }
    if (placed.has(n.tier)) {
      throw new Error("cannot draw " + doc.projection + ": node " + n.id + " is tier " + n.tier +
        " and reaches no tier this page draws (" + tiers.join(", ") + "), while other tier-" +
        n.tier + " nodes do; its parent chain is broken");
    }
    return false;
  };
}

/**
 * The subtree of one node: its id and every id whose parent chain reaches it.
 * Walked upward per node rather than downward from the root, because a node
 * names its parent and nothing names its children.
 * @param {FiscProjection} doc
 * @param {string} id
 * @returns {Set<string>}
 */
function withinNode(doc, id) {
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
 * Where Go's rung answer is served, or "" on a page nobody answers the rungs
 * of. export.RungsPath, through the config, so the file the site writes and
 * the URL the page asks for are one string.
 */
const RUNGS_PATH = CONFIG && typeof CONFIG.rungs === "string" ? CONFIG.rungs : "";

/**
 * The rung-answer schema this client reads.
 *
 * SEPARATE FROM SCHEMA_VERSION, because they version different things: that
 * one is the projection documents' and is stamped by internal/project, this
 * one is the rung answer's and is stamped by the packager. A single constant
 * would tie a change in what a chart MEANS to a change in what Go says it
 * DRAWS, and neither bump implies the other.
 */
const RUNGS_SCHEMA = 4;

/**
 * Go's answer for every rung, by stem, budget and path; null until it lands,
 * and null forever on a page that is told of no answer.
 * @type {Map<string, FiscRung> | null}
 */
let rungAnswers = null;

/**
 * One rung's key in `rungAnswers`. The separator is a unit separator rather
 * than a slash or a space, because a node id carries both and a stem carries
 * the second.
 * @param {string} stem
 * @param {number} width
 * @param {string[]} path
 */
function rungKey(stem, width, path) {
  return stem + "" + width + "" + path.join("");
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
 * TRIMMED TO THE COLUMN BUDGET, WHICH IS WHERE A WIDENED COLUMN IS DROPPED. A
 * step's `widen` names columns of its own `tiers` that a narrow client does
 * without, in the order they go, so trimming is a filter over the set the step
 * declares and never an addition to it -- and dropping from the END of that
 * order is what makes a narrowed window a narrower window rather than a hole.
 * A step declaring no widening is returned whole at any budget.
 *
 * AND THE COLUMNS THE RUNG ITSELF DROPPED, which is a different question with
 * the same answer: a widened tier the drawn document left empty is not a
 * column, whatever the budget (dropEmptyColumns).
 *
 * @returns {number[]}
 */
function activeTiers() {
  if (!drilled.length) return RENDER_TIERS;
  const rung = drilled[drilled.length - 1];
  const tiers = rung.step.tiers;
  const widen = rung.step.widen || [];
  const drop = new Set(rung.dropped || []);
  // THE COUNT IS RE-ASKED AFTER EVERY DROP, not computed once: a tier the rung
  // already dropped as empty is an entry of this same order, and subtracting a
  // fixed shortfall would "drop" it a second time and leave the window a column
  // over budget.
  for (let k = widen.length - 1; k >= 0 && tiers.length - drop.size > columnBudget; k--) {
    drop.add(widen[k]);
  }
  return drop.size ? tiers.filter((t) => !drop.has(t)) : tiers;
}

/**
 * How many columns wide the chart on screen is laid out.
 *
 * A CHART THAT DECLARES NO COLUMN ORDER IS LAID OUT NARROW. Its columns are
 * d3's own inference from topology (alignFor), which is not known until after
 * the layout the width is an input to -- and NARROW_COLUMNS is the width every
 * such chart was drawn at before a step could ask for a fourth column.
 *
 * @returns {number}
 */
function drawnColumns() {
  return activeTiers().length || NARROW_COLUMNS;
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
 * A WINDOW DRAWS TWO DOCUMENTS AND THE SENTENCE REPORTS THEM APART, each
 * against its own total. A kept flank's ribbons come off the chart above, so
 * their facts are that document's; counted into the drawn document's share they
 * state one document's figure as a fraction of another's, which on a window is
 * a body of ribbons rather than a footnote -- twelve of the thirteen the
 * General Fund's window draws. The partition is by the STEM a
 * carried mark records and not by the flag: the fund and division windows keep
 * a flank of the same document they draw, whose facts ARE the drawn document's.
 * Measured through drillDown over every view the page opens, both published
 * columns: partitioning by stem gives the same two numbers, view for view, as
 * partitioning the drawn ribbons' fact ids by membership of the drawn
 * document's own -- which is what tools/jscheck/drill.mjs asserts, from the
 * committed goldens rather than from this function.
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
  // "37 flows between 39 nodes" over ribbons citing 141 of 280. Naming one
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
    const doc = drawnDoc();
    const drawnStem = doc ? doc.projection : "";
    const byID = new Map(projection.nodes.map((n) => [n.id, n]));
    // THE STEM OF A MARK THIS DOCUMENT HAS NEVER HEARD OF, and "" for one it
    // has. A window's kept flank is carried in the sense carried_from records
    // whichever document it came from, so the flag alone cannot tell the fund
    // window's flank -- fund-flows kept on a fund-flows chart -- from the
    // object category's, which is the spine's.
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
      // A CARRIED FLOW CITES THE CHART ABOVE, NOT THIS DOCUMENT. Its facts are
      // the other document's, and counting them here would report one
      // document's facts as a share of another's total.
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
      from = cited.size === total
        ? ", from " + plural(cited.size, "fact")
        : ", from " + cited.size + " of the document's " + plural(total, "fact");
    } else {
      // THE RIBBONS ARE SPLIT BEFORE EITHER FACT COUNT IS GIVEN, so neither
      // number is left attached to the whole chart. "14 flows ..., from 29 of
      // the document's 73 facts" reads as a claim about all 14 while it is one
      // about 9 of them, which is the same sentence the carried ones were
      // wrongly inside.
      from = ": " + (projection.links.length - carried) + " citing " + cited.size +
        " of the document's " + plural(total, "fact") + ", and " + carried +
        " carried unchanged from the chart above";
      // NAMED ONLY WHERE ONE DOCUMENT IS NAMEABLE. carriedSource resolves a
      // stem against the stack; two stems, or a document with no counts block,
      // leave the clause as the count of ribbons alone rather than weigh the
      // carried facts against a total that is not theirs.
      const src = stems.size === 1 ? carriedSource(Array.from(stems)[0]) : null;
      const theirs = src ? factsIn(src, 0) : 0;
      if (theirs) from += ", citing " + above.size + " of its " + plural(theirs, "fact");
    }
  }
  counts.textContent = plural(links, "flow") + " between " + plural(nodes, "node") + from;
}

/**
 * Whether activating this node opens it.
 *
 * A NODE OPENS WHEN A STEP OPENS FROM IT, and that is stepFor's three matches:
 * the chart on screen, the node's tier, and its role where the step names
 * one. All three of the spine's drawn columns hold something that opens, into
 * two documents: a fund group into its funds, a revenue category into the lines
 * pp.127-140 print under it with the fund groups it reaches kept beside them,
 * and an object category into the divisions pp.85-125 give it. The categories
 * were excluded here while the document carried nothing beneath them -- the
 * offer would have promised a decomposition no page printed -- and since it
 * carries the line tier the
 * exclusion is by role and not by tier: transfers/in and fund-balance/draw
 * share tier 0 with the categories, are the flow's ends rather than
 * containers of it, and the step's role leaves them closed.
 *
 * An aggregate is excluded by name -- it can sit at a step's `from` tier now
 * that caps are per tier -- and would have nothing to open into anyway, being
 * several documents' worth of small funds rather than one thing. So are the
 * residual node and the endpoints carried with it: an endpoint that leaves
 * the group is placed at the last drawn tier, which is exactly where the
 * next step opens from, and it is a flow's end rather than a container of
 * anything.
 *
 * @param {{id: string, tier: number, role?: string}} d
 * @returns {boolean}
 */
function drillable(d) {
  return Boolean(stepFor(d)) && !isAggregate(d.id) && !isCarried(d.id);
}

/**
 * Whether a mark is a folded tail this chart can draw out into the marks it
 * stands for.
 *
 * EXPANDING IS NOT OPENING, AND THIS IS NOT drillable's CLAUSE RELAXED.
 * drillable excludes an aggregate for a reason that has not changed: it stands
 * for several documents' worth of small rows and there is no node to open it
 * INTO. What a reader may do to it is draw the column it was folded out of at
 * full length, which is a redraw of the chart they are on rather than a rung.
 * So the two predicates are disjoint by construction and nothing has to order
 * them.
 *
 * BY THE CAP THIS RUNG DECLARES, NOT BY THE PREFIX ALONE. An aggregate can also
 * arrive on a chart inside a kept flank, folded by the cap of the chart above;
 * this rung's `expanded` set does not reach it, so offering the gesture there
 * would be an affordance that redraws the same chart. Latent on the committed
 * corpus, whose kept flanks are single-node columns, and latent is how it would
 * ship.
 *
 * @param {{id: string, tier: number}} d
 * @returns {boolean}
 */
function expandable(d) {
  const rung = drilled.length ? drilled[drilled.length - 1] : null;
  if (!rung || !isAggregate(d.id)) return false;
  if (rung.expanded && rung.expanded.has(d.tier)) return false;
  return (rung.step.caps || []).some((c) => c.tier === d.tier);
}

/**
 * The tiers of the chart on screen the reader has drawn out, in the order the
 * chart lays its columns out in.
 *
 * IN COLUMN ORDER BECAUSE THE BREADCRUMB SHOWS ONE CHIP PER TIER and two of
 * them in an order nothing decides would swap between redraws of the same
 * chart. activeTiers is the same list openableColumns names columns off.
 * @returns {number[]}
 */
function expandedTiers() {
  const rung = drilled.length ? drilled[drilled.length - 1] : null;
  if (!rung || !rung.expanded) return [];
  return activeTiers().filter((t) => rung.expanded.has(t));
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
 * The year's entry for the rung a step opens: the file it draws and the
 * caveat refs its marks link to, or null when the year on screen was packaged
 * with none.
 *
 * READ OFF THE YEAR, NEVER JOINED HERE. A step's document is per fiscal year --
 * FY2026-27's fund groups open into fund-flows-2027, not into the one file a
 * stem maps to in CONFIG.projections -- and which file that is belongs to the
 * packager, which resolves every step for every year into the year's own
 * config entry. This file resolves nothing: it reads the entry for the year on
 * screen for the step being opened, and refuses when there is none rather than
 * draw a file the year was never told about.
 *
 * BY THE STEP'S PLACE IN THE DECLARATION, NOT BY DEPTH. The packager writes
 * one entry per declared step in declaration order (export.stepDocuments),
 * and two steps open from depth 0 on the spine, so the depth names two
 * entries and the step names one.
 * @param {FiscDrillStep} step
 * @returns {FiscStepDoc | null}
 */
function stepDocFor(step) {
  const steps = shownYear && Array.isArray(shownYear.steps) ? shownYear.steps : [];
  const at = CONFIG && Array.isArray(CONFIG.steps) ? CONFIG.steps.indexOf(step) : -1;
  const entry = at >= 0 ? steps[at] : undefined;
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
 * @param {FiscProjection} from  the document of the chart the step opens from
 * @param {() => boolean} superseded
 * @returns {Promise<FiscProjection | null>}
 */
async function stepDocument(step, from, superseded) {
  if (!step.projection) return from;
  const entry = stepDocFor(step);
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
    // RESHAPED AND NOT JUST RE-LAID, so the chart drawn at three columns is the
    // chart three columns would have drawn: the fold, the caps and the
    // placeability test all take the tier set, and a document shaped at four
    // columns and laid out at three would be a fourth shape nothing else
    // produces. It terminates because each pass adds at least one entry of a
    // finite `widen` list to the rung's dropped set and never removes one.
    while (dropEmptyColumns(drawn)) drawn = shapeFor(doc);
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
 * IT ASKS TWICE, ABOUT DIFFERENT THINGS. The tokens catch a gesture that
 * STARTED after this one; the document identity catches a year switch that was
 * already in flight when this one started, which no token can see because the
 * bump happened first. Both orderings end in a chart whose parts come from two
 * fiscal years, so neither check is redundant.
 *
 * @param {string} id
 * @returns {Promise<string>} DREW, SUPERSEDED or FAILED
 */
async function drillDown(id) {
  const depth = drilled.length;
  const from = docAt(depth);
  // THE NODE THE READER ACTIVATED, OFF THE CHART THEY ACTIVATED IT ON. Which
  // step opens it is a question about that node -- its tier and its role --
  // and not about the depth; the argument is unchanged and only the document
  // it is asked of moves. It has to move, because a window's kept flank is
  // drawn from the chart above and its nodes need not exist in the rung's file
  // at all: asked of the file, a fund group kept beside a departmentwide
  // document is a node that document does not carry, and the click returns
  // FAILED in silence. An id the chart does not draw, or one drillable
  // refuses, opens nothing.
  const chart = projection;
  const node = chart ? chart.nodes.find((n) => n.id === id) : undefined;
  const step = node && drillable(node) ? stepFor(node) : null;
  if (!step || !from || !chart) return FAILED;
  const mine = ++opening;
  const token = switching;
  const overtaken = () => mine !== opening || token !== switching;
  const doc = await stepDocument(step, from, overtaken);
  // THE TOKEN CANNOT SEE A SWITCH THAT WAS ALREADY IN FLIGHT. `switching` is
  // bumped when showYear STARTS, so a drill begun while a year fetch is
  // outstanding captures the already-bumped value and compares equal when the
  // new spine lands. The stack then rebuilds from `from`, the document the
  // reader clicked on, under the new year's title: measured, an FY2026-27
  // title and residual over FY2025-26 fund figures, in one chart.
  //
  // ASKING ABOUT THE DOCUMENT ANSWERS BOTH ORDERINGS, because it is the thing
  // the gesture was actually opened against rather than a count of gestures.
  // fisc-bccu's neighbour, found by pass two of /code-review.
  if (overtaken() || docAt(depth) !== from) return SUPERSEDED;
  if (!doc) return FAILED;
  return redrawStack(drilled.concat([{ id: id, doc: doc, step: step, chart: chart }]))
    ? DREW : FAILED;
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
 * Draws the column a folded tail was cut out of at every mark it holds.
 *
 * NO FETCH AND NO RUNG. The marks are already in the rung's own document --
 * capColumn discarded them on the way to the screen, not on the way off the
 * wire -- so this is the chart the reader is on, reshaped. The breadcrumb does
 * not move, because nothing has been opened.
 *
 * A REPLACED RUNG AND NOT A MUTATED ONE. redrawStack restores the whole stack
 * when the reshape throws, and a set mutated in place would survive that
 * restore -- leaving the reader on the chart they had, over a rung that says it
 * is expanded and would redraw expanded at the next repaint.
 *
 * @param {{tier: number}} d
 */
function expandTier(d) {
  const at = drilled.length - 1;
  const rung = drilled[at];
  if (!rung) return;
  const expanded = new Set(rung.expanded || []);
  expanded.add(d.tier);
  redrawStack(drilled.slice(0, at).concat([Object.assign({}, rung, { expanded: expanded })]));
}

/**
 * Folds an expanded column back into its tail: what the breadcrumb's chip does.
 *
 * THE ONLY WAY BACK, AND THAT IS WHY IT IS A CONTROL RATHER THAN A GESTURE. The
 * mark a reader expanded is the one mark expanding removes, so there is nothing
 * left on the chart to double click; and Escape already means "pop one rung",
 * which is unambiguous only while nothing else is stacked.
 * @param {number} tier
 */
function collapseTier(tier) {
  const at = drilled.length - 1;
  const rung = drilled[at];
  if (!rung || !rung.expanded || !rung.expanded.has(tier)) return;
  const expanded = new Set(rung.expanded);
  expanded.delete(tier);
  redrawStack(drilled.slice(0, at).concat([Object.assign({}, rung, { expanded: expanded })]));
}

/**
 * One click on a node: it follows that node's money, whether or not the node
 * also opens.
 *
 * NAMED RATHER THAN INLINE IN render(), and that is what makes the gesture
 * checkable at all. The stub tools/jscheck runs against answers no "#chart"
 * selector, so d3 lays its selection over a null node and every handler
 * render() registers is registered on nothing -- a closure there executes in no
 * check, however many checks the page has. Here it is reached by name.
 *
 * THE ECHO GUARD IS ON THE ACTIVATION AND NOT ON THE DEVICE: a click on the
 * node a key has just activated is that key's own click, synthesised by
 * assistive tech that would otherwise undo what the key did. Every other click
 * still isolates, including one synthesised by tech that sent no key at all.
 * @param {LaidNode} d
 * @param {number} at the event's timestamp
 */
function clickNode(d, at) {
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
 * THE RESTORE IS OBSERVABLE ON A NODE THAT DOES NOT OPEN, and that is where it
 * earns its place. On one that does, drawChart clears the isolation anyway --
 * an id from the chart being replaced need not exist on the chart replacing it
 * -- so there the restore buys only the frame: the emphasis the reader sees
 * last before the redraw is their own, rather than a flash of the node they
 * happen to be double clicking.
 *
 * THE RECORD IS SPENT WHETHER OR NOT ANYTHING OPENED, so a later double click
 * on the same node cannot restore a state two gestures old.
 * @param {LaidNode} d
 * @param {number} at the event's timestamp
 */
function doubleClickNode(d, at) {
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
 * SPACE IS THE COST OF THE SPLIT AND IS ANNOUNCED RATHER THAN HIDDEN. A
 * role="button" conventionally activates on Space, and here Space is the one
 * key that never opens. nodeDescription says so on the mark, paintChartHint
 * says so on the page, and aria-keyshortcuts carries both keys -- because the
 * convention this departs from is one a reader is entitled to rely on until
 * told otherwise.
 *
 * ENTER FALLS BACK TO THE ISOLATE ON A NODE THAT DOES NOT OPEN, rather than
 * doing nothing. A role="button" with a dead Enter is worse than one whose
 * Enter and Space agree, and on those marks they always did: 319 of the 343
 * nodes the chain's views draw open into nothing, and their aria-pressed
 * toggle is the only thing an activation there could mean.
 * @param {LaidNode} d
 * @param {string} key
 * @param {number} at the event's timestamp
 */
function keyNode(d, key, at) {
  keyActivation = { id: d.id, at: at };
  // Escape unpins while leaving focus where it was, so the panel can be
  // empty here even though focus already pinned this node once.
  pin(d);
  if (key === "Enter" && drillable(d)) {
    openNode(d.id);
    return;
  }
  // THE SAME KEY FOR THE SAME KIND OF THING. Enter is "show me what is inside
  // this mark" on both, and which of the two it does is the mark's business
  // rather than a second keystroke's; Space still follows the money on either.
  if (key === "Enter" && expandable(d)) {
    expandTier(d);
    return;
  }
  setIsolated(isolated === d.id ? "" : d.id);
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
  //
  // ASKED FOR BY CLASS AND NOT BY TAG, because the bar holds a second kind of
  // button: one chip per expanded column, which undoes an expansion rather than
  // a rung. Under "the last button" a keyboard drill onto a chart whose column
  // the reader had expanded landed focus on the chip -- the one control in the
  // bar that does not go back.
  const bar = maybeEl("breadcrumb");
  if (drilled.length && bar) {
    const controls = Array.from(bar.children || []).filter((c) =>
      String(/** @type {any} */ (c).className || "").split(" ").indexOf("crumb-back") >= 0);
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
 * MIDDLE column. openableColumns names every column that holds a node which
 * opens -- all three on the spine, whose revenue categories and object
 * categories open as well as its fund groups.
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
  const column = anyOpens ? joinOr(openableColumns()) : "";
  // THE COLUMN IS DROPPED FROM THE SENTENCE RATHER THAN LEFT BLANK IN IT. A
  // chart whose columns nobody declared has no left and no right to name
  // (openableColumns), and "Click a node in the  column" is worse than the
  // shorter true sentence. internal/export refuses the view that would produce
  // it; a config handed to this file has still said it.
  const where = column ? " in the " + column + " column" : "";
  // THE ISOLATE IS NAMED ON EVERY VIEW, including one where nothing opens. It
  // is the gesture every node of every chart has, and the sentence that used to
  // stop at "nothing here opens further" left a reader on those charts told
  // only what they could not do.
  //
  // AND SPACE IS NAMED BECAUSE IT IS THE SURPRISE. A role="button" activates on
  // Space by convention and here Space never opens; a reader is entitled to
  // that convention until the page says otherwise, so the page says otherwise.
  const follows = " A single click, or Space, follows one node's money.";
  // THE FOLDED TAIL IS NAMED ONLY WHERE THERE IS ONE, and it is a sentence of
  // its own rather than a clause inside the opening one: on five of the nine
  // views that draw a tail, the tail is the ONLY thing the chart offers, and
  // "nothing here opens further" was the whole of what those readers were told.
  const tails = Boolean(projection) && projection.nodes.some(expandable);
  const expands = tails
    ? " The folded mark is several of them drawn as one; double click it, or tab to it and " +
      "press Enter, to draw them separately."
    : "";
  if (drilled.length) {
    hint.textContent = "This is " + labelOfRung(drilled.length - 1) +
      ", broken into its parts. " +
      (anyOpens
        ? "Double click a node" + where + " to open it further, or tab to one and press Enter."
        : "Nothing here opens further; go back to open another.") + follows + expands;
    return;
  }
  const swatches = buildLegendCount();
  hint.textContent = (anyOpens
    ? "Double click a node" + where + " to open it into its parts, " +
      "or tab to one and press Enter."
    : "Nothing on this chart opens.") + follows +
    (swatches ? " A fund swatch follows one group's money without opening anything." : "");
}

/**
 * Which columns of the chart on screen hold a node that opens -- "left-hand",
 * "middle", "right-hand", left to right -- or none when nothing here opens.
 *
 * IN THE DECLARED ORDER AND NOT IN TIER ORDER. layOut aligns a node on
 * indexOf(tier) in the set the document was shaped by (alignFor), so THAT list
 * is the columns left to right; a sort by tier number agrees with it only
 * while the declaration happens to ascend. A window's need not -- {2,5,4}
 * draws fund groups, the object category they pay for, then the divisions
 * spending it -- and sorted, this would name the middle column "right-hand"
 * and send a reader to click the wrong one.
 *
 * NARROWED TO THE TIERS THE CHART ACTUALLY DRAWS, because a declared column
 * can come out empty and telling a reader to click a column that is not there
 * is the same defect as telling them to click one that does not open. Measured
 * over every view the page opens, both published columns: none of them comes
 * out short, so the narrowing changes no sentence on the committed corpus. It
 * stays because a tier set is a DECLARATION and a document need not fill it --
 * the drill opens into a schedule that decomposes one fund of sixty-one -- and
 * a hint naming a column nothing is drawn in is the same defect one step on.
 * Two columns have no middle; a chart drawn whole
 * declares no order at all and is named nothing here, which is the same answer
 * as "nothing opens" and reaches the reader as paintChartHint's other
 * sentence.
 *
 * ASKED OF THE DRAWN NODES, NOT OF THE STEPS: a step opens from a tier, and
 * which of that tier's nodes open is drillable's answer -- on the spine's
 * left-hand column three of thirteen do not.
 * @returns {string[]}
 */
function openableColumns() {
  if (!projection) return [];
  const drawn = new Set(projection.nodes.map((n) => n.tier));
  const tiers = activeTiers().filter((t) => drawn.has(t));
  if (tiers.length < 2) return [];
  const opening = new Set(projection.nodes.filter(drillable).map((n) => n.tier));
  return tiers.map((tier, at) => {
    if (!opening.has(tier)) return "";
    return at === 0 ? "left-hand" : at === tiers.length - 1 ? "right-hand" : "middle";
  }).filter(Boolean);
}

/**
 * A list of phrases as English: "a", "a or b", "a, b or c".
 *
 * A join ON " or " READS AS A CHOICE OF TWO HOWEVER MANY THERE ARE. Three
 * openable columns came out "the left-hand or middle or right-hand column",
 * which is not a sentence anyone writes, and the spine reaches three the day
 * its right-hand column opens. No serial comma: the last separator is the
 * conjunction alone.
 * @param {string[]} parts
 * @returns {string}
 */
function joinOr(parts) {
  if (parts.length < 3) return parts.join(" or ");
  return parts.slice(0, -1).join(", ") + " or " + parts[parts.length - 1];
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
  // ONE CHIP PER EXPANDED COLUMN, AND IT IS THE ONLY WAY BACK. Expanding
  // removes the mark that was expanded, so unlike every other state this page
  // holds there is nothing on the chart left to gesture at; and the chart's own
  // words cannot say it, because "32 funds" is not a thing the reader asked for
  // until they asked for it.
  //
  // THE COUNT IS READ OFF THE CHART ON SCREEN rather than remembered from the
  // tail's label. The tail said how many it hid; the chip says how many are
  // drawn, which is the claim a reader can check by counting marks.
  const chips = expandedTiers().map((tier) => {
    const n = columnSize(tier);
    const chip = h("button", "crumb-expanded",
      "showing all " + n + " " + tailNoun(tier) + " ×");
    chip.setAttribute("type", "button");
    // THE GLYPH IS NOT THE LABEL. A screen reader reads "×" as "times" or as
    // nothing at all, and a control whose accessible name is "showing all 32
    // funds" does not say that pressing it stops showing them.
    chip.setAttribute("aria-label",
      "Showing all " + n + " " + tailNoun(tier) + "; fold the smallest back into one mark");
    chip.addEventListener("click", () => collapseTier(tier));
    return chip;
  });
  // THE CHIPS COME AFTER THE TRAIL, WHICH restoreFocus HAD TO BE TOLD ABOUT.
  // It took the LAST <button> in this bar as the way back from the rung just
  // opened; a chip is a button in this bar, so a keyboard drill onto a chart
  // whose column was already expanded would have put focus on "fold these back"
  // instead. It asks for the return control by class now.
  bar.replaceChildren(...controls, here, ...chips);
}

/**
 * How many marks of its own a drawn column holds.
 *
 * THE COLUMN'S OWN, NOT EVERY MARK AT THAT TIER. A residual, a gap and the
 * endpoints carried with them are placed at a drawn tier and are not parts of
 * the opened node -- isCarried is the page's one reader of that -- and a kept
 * flank's marks are the chart above's. Counting them would put a number in the
 * breadcrumb a reader counting marks in the column disagrees with.
 * @param {number} tier
 * @returns {number}
 */
function columnSize(tier) {
  if (!projection) return 0;
  return projection.nodes.filter((n) =>
    n.tier === tier && !isCarried(n.id) && !n.carried_from).length;
}

/**
 * What this rung's step calls the rows of one capped column.
 *
 * THE CAP'S WORD WHERE IT HAS ONE, which is capColumn's rule reached from the
 * other side: one step caps two columns under two nouns, and a chip that said
 * "funds" over the categories would be the tier-number-to-word mapping
 * paintBreadcrumb refuses one function up.
 * @param {number} tier
 * @returns {string}
 */
function tailNoun(tier) {
  const rung = drilled.length ? drilled[drilled.length - 1] : null;
  if (!rung) return "items";
  const cap = (rung.step.caps || []).find((c) => c.tier === tier);
  return (cap && cap.tail) || rung.step.tail || "items";
}

/**
 * The printed label of the node rung k opened, from the document it was opened
 * FROM.
 *
 * THE WORDS ARE THE CHART THE READER CLICKED ON'S, and that is the rule rather
 * than a consequence of the node going away. A one-sided step does remove it:
 * opening a fund group filters the group itself out of the drawn set, so there
 * is nothing there to read. A window does not -- its centre IS the node that
 * was clicked, drawn in the middle column -- and the answer has to be the same
 * either way, because the same breadcrumb names both. windowFor takes the
 * centre's record off the chart on screen for this reason, so the mark and the
 * crumb agree by construction; this reads the file that chart was shaped from
 * rather than depending on that.
 *
 * AND NOT THE RUNG'S OWN DOCUMENT: across a document switch the node was
 * clicked in the chart one depth up, and that chart's file is the one that
 * prints its label. Both documents may carry the id and print different words
 * for it, which is what tools/jscheck/drill.mjs relabels its fixtures to catch.
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
 * The rungs' names, outermost first, with any two that read alike told apart.
 *
 * THE COLLISION IS THE CITY'S AND NOT THIS PAGE'S. Budget Book p66 prints
 * "General Fund" as a fund-group column header and p255 prints it as fund 100's
 * name, so a reader two rungs into that group is told "opened into General
 * Fund, then General Fund" and nothing says which is which. Neither label can
 * be changed: both are the words the city printed over the box, and this
 * projection's rule is that pp.66-67's words win.
 *
 * SO THE TRAIL IS WHERE IT IS RESOLVED, and with the step's declared noun --
 * `tier === 2 ? "fund group" : "fund"` is the construct paintBreadcrumb's
 * comment refuses, one function further out. EVERY member of a colliding set is
 * qualified rather than all but the last: "General Fund (fund group), then
 * General Fund" leaves the second one still asking which General Fund it is.
 *
 * A rung whose step declares no noun is drawn unqualified. The packager refuses
 * one (validateSteps), and inventing a word here to cover a config that got
 * past it would be this file naming the tiers after all.
 * @returns {string[]}
 */
function trailOfRungs() {
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
 * THE RESIDUAL AND THE CONTRA MARKING COME LAST AND IN THAT ORDER, whichever
 * shape the rung took; everything before them is sideOf's, once for a chart
 * with a side and twice for a window.
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
    return markContra(foldDocument(ROOT ? filterToNode(doc, ROOT, RENDER_TIERS) : doc), doc);
  }
  // ROOT DOES NOT REACH HERE. It is the spine's vocabulary -- the node whose
  // subtree THIS PAGE's overview draws -- and a rung filters to the node the
  // reader opened, which is inside the root on a page that has one and is a
  // node of another document entirely on a step that switched.
  const step = rung.step;
  // A WINDOW OR A SIDE, AND THE STEP SAYS WHICH. A step that keeps a flank
  // draws two half-charts spliced on the node the reader clicked; one that
  // keeps none draws a single filtered chart, and its SIDE picks the filter:
  // the opened node is the end its links point at, or the end they come from,
  // and the two filters disagree about what "inside" means (filterFromNode).
  const drawn = (step.keep && step.keep.length)
    ? windowFor(rung.chart, doc, rung)
    : sideOf(doc, rung, activeTiers(), step.side === "source" ? filterFromNode : filterToNode);
  // LAST, AFTER THE CAP AND THE FOLD, because neither may touch it: the cap
  // ranks the group's own parts and the residual is not one of them, and the
  // fold merges by folded ends and these ends are the chart above's.
  //
  // AND THE CONTRA MARKING AFTER THAT, because it is about what the fold LEFT
  // negative: {1,0,2} is the one tier set that draws a category's printed lines
  // at all, and two of them are reductions; every other view reaches the
  // category as one net cell, where nothing is negative.
  //
  // AND THE GAP LAST OF THE THREE THAT ADD MARKS, because it is a statement
  // about the whole drawn chart: what the opened node takes in against what it
  // sends out, once everything that is going to stand beside it does. Only
  // markContra follows, and it reclassifies ribbons rather than moving a cent.
  return markContra(markGap(carryResidual(drawn, docAt(drilled.length - 1), rung), rung), doc);
}

/**
 * Drops a widened column the drawn document left empty, so the chart is laid
 * out at the columns it has.
 *
 * A COLUMN BUDGET IS A REQUEST AND NOT A SHAPE. d3-sankey takes its column
 * count from TOPOLOGY -- the deepest node -- and clamps the aligner into it, so
 * a tier set naming a column nothing is drawn in does not draw a narrower chart:
 * it draws the columns it has, spread across an extent sized for one more, with
 * every band wider than the one the label rule was measured against. Asking the
 * drawn document instead is what openableColumns already does one sentence over,
 * for the same reason: a declaration is not a promise the document fills it.
 *
 * DROPPED AND NOT REFUSED. Five of the six fund groups have no tier-4 node at
 * all -- pp.167-170 decompose the General Fund and no other -- so a widened
 * window that refused an empty column would turn those five into a banner, and
 * a reader with a wide screen would be shown less than a reader with a narrow
 * one. The narrower chart is exactly the one the narrow budget draws.
 *
 * ONLY A WIDENED COLUMN. The flank, the centre and the first column of the
 * decomposition are what the step promised; an empty one of those is a fault in
 * the view or the document, and sideOf's own guards say so by name.
 *
 * @param {FiscProjection} drawn
 * @returns {boolean} whether anything was dropped
 */
function dropEmptyColumns(drawn) {
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
 * THE CAPS ARE THE STEP'S AND THE TIERS ARE THE CALLER'S, which is the whole
 * reason this takes both. A window's two halves are two documents, and folding
 * them together would hand foldDocument two parent chains at once -- the
 * invariant carryResidual already preserves by copying links rather than
 * merging documents. Each half is shaped whole and on its own, and the splice
 * happens after.
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
 * @param {Rung} rung
 * @param {number[]} tiers the columns this chart draws, in order
 * @param {(doc: FiscProjection, id: string, tiers: number[]) => FiscProjection} filter
 * @returns {FiscProjection}
 */
function sideOf(doc, rung, tiers, filter) {
  const step = rung.step;
  let shaped = filter(doc, rung.id, tiers);
  const inside = withinNode(doc, rung.id);
  // EVERY CAP THE STEP DECLARES FOR THESE COLUMNS, IN THE DECLARED ORDER.
  // Folding a coarse node removes its descendants (capColumn's orphaned()),
  // which changes which fine nodes are left to rank; capping the fine tier
  // first would rank divisions of a fund about to be folded away. The order is
  // the tier order this chart draws in, not the caps' declaration order, for
  // the same reason the cap is looked up by the tier it names.
  //
  // THE TAIL'S PARENT IS THE OPENED NODE ONLY WHEN THE WHOLE COLUMN IS INSIDE
  // IT, and that is asked of the document rather than assumed. A fund group's
  // funds and a category's lines are inside the node that opened them, and
  // the tail inherits its hue through it; an object category's divisions are
  // read off a schedule with no fund axis at all, and a tail parented to the
  // category would claim a place in a hierarchy they are not in. It gets "",
  // which is --muted, which is what "no single fund group" looks like
  // everywhere else on this page.
  /** @type {Map<number, string>} */
  const parentOf = new Map();
  for (const tier of tiers) {
    const cap = (step.caps || []).find((c) => c.tier === tier);
    // AN EXPANDED TIER IS SKIPPED HERE AND NOWHERE ELSE, which is what keeps
    // the filter/cap/fold order above intact: the column is still filtered to
    // what is inside the opened node and still folded to the tiers this chart
    // draws, and the only stage it misses is the one the reader asked it to.
    if (!cap || (rung.expanded && rung.expanded.has(tier))) continue;
    const column = shaped.nodes.filter((n) => n.tier === tier);
    const parent = column.every((n) => inside.has(n.id)) ? rung.id : "";
    parentOf.set(tier, parent);
    // THE NOUN IS THE CAP'S WHERE IT NAMES ONE, and the step's otherwise: the
    // fund-group step caps its funds under its own noun and its divisions under
    // the cap's, and one word cannot count both tails.
    shaped = capColumn(shaped, tier, cap.cap, parent, cap.tail || step.tail);
  }
  const drawn = foldDocument(shaped, tiers);

  // EVERY AGGREGATE'S PARENT IS PUT BACK AFTER THE FOLD, and it has to be here
  // rather than in capColumn. capColumn runs first and parents the aggregate
  // as the loop above decided, which is true; foldDocument then re-points every
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
      (isAggregate(n.id) ? Object.assign({}, n, { parent: parentOf.get(n.tier) || "" }) : n)),
  });
}

/**
 * A window on the node the reader clicked: the flank they came from on one
 * side, the step document's decomposition on the other, and that node between
 * them.
 *
 * THREE COLUMNS IS THE NARROWEST SHAPE AND NOT THE ONLY ONE. The flank is as
 * many columns as the step keeps and the decomposition as many as it draws, so
 * the centre is at index keep.length from the kept end whatever those are. The
 * two ends are read the way export.validateSteps reads them -- the kept flank
 * is Tiers' first keep.length columns REVERSED, because Keep is nearest-centre
 * first, or its last keep.length in order -- so a declaration that passes the
 * packager and a chart drawn here cannot disagree about which side is which.
 *
 * TWO QUESTIONS, SO TWO CALLS, AND NEITHER FILTER CHANGES. filterToNode keeps a
 * link whose TARGET is inside the clicked node; filterFromNode keeps one whose
 * SOURCE is. A window needs both -- its decomposition on one side and the
 * context hop on the other -- and answering both from one call would mean a
 * third filter whose "inside" meant neither thing. So the kept flank and the
 * new one are separate charts, spliced on the centre.
 *
 *   kept flank on the LEFT   kept: filterToNode(on screen), new: filterFromNode(step)
 *   kept flank on the RIGHT  kept: filterFromNode(on screen), new: filterToNode(step)
 *
 * EACH HALF IS ASKED FOR THE COLUMNS IT DRAWS, CENTRE INCLUDED, so the two
 * overlap in exactly one column and the splice has something to splice on.
 *
 * WHICH WAY IT SLIDES IS THE POSITION OF THE KEPT TIER IN THE STEP'S OWN
 * COLUMN ORDER, and internal/export's validateSteps has already refused a step
 * whose columns disagree with the chart it opens from. This reads the
 * declaration and fails closed on one it cannot read, because a window drawn
 * the wrong way round is a chart that lays out and means something else.
 *
 * THE KEPT FLANK COMES OFF THE CHART ON SCREEN, NOT OFF A FILE, and that is
 * not an optimisation. Its nodes need not exist in the step's document at all:
 * a departmentwide document has no fund axis, so every fund group in that
 * window's flank would be a node drillDown could not find and the click would
 * return FAILED in silence. Filtering the chart on screen removes that by
 * construction, lets the kept flank carry a capped tail or a residual it
 * already drew, and keeps filterLinks' fail-closed guards applicable, because
 * the chart on screen is itself a well-formed document.
 *
 * THE CENTRE'S RECORD IS THE ON-SCREEN CHART'S, so the mark names the node in
 * the words the reader clicked -- which is the rule paintBreadcrumb follows one
 * function over, reached from the other side. It is NOT marked carried: a
 * carried mark is one the drawn document has never heard of, and the centre is
 * the node that document decomposes.
 *
 * @param {FiscProjection | null} onScreen the drawn chart the rung was opened from
 * @param {FiscProjection} stepDoc the document the step draws
 * @param {Rung} rung
 * @returns {FiscProjection}
 */
function windowFor(onScreen, stepDoc, rung) {
  const step = rung.step;
  // THE COLUMNS ON SCREEN AND NOT THE COLUMNS DECLARED. A step may offer more
  // than the budget draws, and a half shaped at a column the chart does not lay
  // out would splice in nodes with nowhere to be.
  const tiers = activeTiers();
  const keep = step.keep || [];
  const deep = keep.length;
  const n = tiers.length;
  // WHICH END THE FLANK IS AT IS READ OFF THE COLUMN ORDER, exactly as
  // export.validateSteps reads it: a left flank is the first `deep` columns
  // reversed, a right flank the last `deep` in order. Neither matching is a
  // declaration this cannot draw, and a window drawn the wrong way round lays
  // out fine and means something else -- so it is refused in words.
  const flank = (/** @type {number[]} */ want) =>
    want.length === deep && want.every((t, k) => t === keep[k]);
  const keptLeft = deep > 0 && n >= deep + 2 && flank(tiers.slice(0, deep).reverse());
  const keptRight = deep > 0 && n >= deep + 2 && flank(tiers.slice(n - deep));
  const centre = keptLeft ? deep : n - 1 - deep;
  if (!onScreen || (!keptLeft && !keptRight) || tiers[centre] !== step.from) {
    throw new Error("cannot draw " + stepDoc.projection + ": this step keeps tier(s) " +
      keep.join(", ") + " and draws tiers " + tiers.join(", ") + " opening tier " +
      step.from + ", which is not a window: a window is the kept flank at ONE end, " +
      "outermost first, the opened tier next to it, at least one column of what it " +
      "opens into, and a chart on screen to take the flank from");
  }
  // THE CENTRE IS IN BOTH HALVES, and each half gets the columns on its own
  // side of it: the flank plus the centre off the chart above, the centre plus
  // everything the step opens it into off the step's document.
  const kept = keptLeft
    ? sideOf(onScreen, rung, tiers.slice(0, centre + 1), filterToNode)
    : sideOf(onScreen, rung, tiers.slice(centre), filterFromNode);
  const fresh = keptLeft
    ? sideOf(stepDoc, rung, tiers.slice(centre), filterFromNode)
    : sideOf(stepDoc, rung, tiers.slice(0, centre + 1), filterToNode);

  // carried_from IS SET WHERE IT IS ABSENT AND NEVER CLEARED. A flank node
  // that was already carried onto the chart above -- a residual's endpoint --
  // keeps the stem it came from, because that is the document its figure and
  // its caveats are of, however many rungs it is passed down.
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
  // THE SPLICED DOCUMENT IS THE STEP DOCUMENT'S, which is what makes its
  // projection name, its metadata and its caveats the drawn chart's: the kept
  // flank is a guest on it, and says so on every node it brought.
  return Object.assign({}, fresh, {
    nodes: nodes,
    links: kept.links.concat(fresh.links),
  });
}

/**
 * Draws every link the fold leaves negative as a contra ribbon: forward, at
 * its magnitude, carrying the words for what the schedule printed.
 *
 * WHAT A NEGATIVE LINK IS. pp.127-140 print ERAF and the RPTTF reduction as
 * reductions of Property Taxes -- rows in parentheses, netted into the
 * category's total -- and fund-flows publishes each as a line whose links carry
 * its signed figure, into the fund it reduces and back into the category it is
 * printed under. Folded to the category they vanish into the net cell; drawn as
 * that category's own lines they stand on their own, and a sankey has no ribbon
 * of negative width.
 *
 * NOT A REVERSED LINK, THOUGH THAT WAS THE FIRST DESIGN. A reduction pointed
 * backwards gives its line a depth one past the mark it reduces, and the
 * vendored d3-sankey sizes its column count from the deepest node: the view
 * came out with one column more than its step declares and the last of them
 * empty, and its layering pass throws on the hole -- "Cannot read properties of
 * undefined (reading 'sort')", the same failure layOut's comment records for a
 * misaligned tier set. So the ribbon runs the way every other ribbon runs, at
 * the printed size, and what makes it a reduction is said three ways: the class
 * render() gives it, the sign every figure carries, and the sentence on the mark.
 *
 * THE CENTRE'S FIGURE IS GROSS OF ITS REDUCTIONS, and contraNote says so on
 * the mark. d3-sankey sizes a node at the larger of what enters and what
 * leaves, and a contra ribbon enters; the Property Taxes category stands at the
 * sum of every ribbon into it, which is p127's total before ERAF and the RPTTF
 * reduction come off, while the two ribbons leaving it are the spine's own
 * cells and come to that total net. The step's description says the same for
 * the chart as a whole.
 *
 * THE WORDS NAME THE PARENT IN THE FILE, not in the drawn document: the fold
 * blanks a line's parent, and it is the category p127 prints the reduction
 * under that the reader should hear. A source the file does not carry -- a
 * capped tail whose folded rows net to a reduction, which no committed column
 * produces -- is named for what it is rather than for a category it is not.
 *
 * @param {FiscProjection} drawn  shaped and folded
 * @param {FiscProjection} file  the document it was shaped from, unfolded
 * @returns {FiscProjection} drawn itself when nothing in it is negative
 */
function markContra(drawn, file) {
  if (!drawn.links.some((l) => l.value_cents < 0)) return drawn;
  const byID = new Map(file.nodes.map((n) => [n.id, n]));
  const under = (/** @type {string} */ id) => {
    const n = byID.get(id);
    const up = n && n.parent ? byID.get(n.parent) : undefined;
    return up ? "printed as a reduction of " + up.label : "printed rows netting to a reduction";
  };
  return Object.assign({}, drawn, {
    links: drawn.links.map((l) => (l.value_cents < 0
      ? Object.assign({}, l, { value_cents: -l.value_cents, contra: under(l.source) })
      : l)),
  });
}

/**
 * Whether every ribbon on a laid node is a contra one: a line the schedule
 * prints as a reduction, whose own figure is therefore negative.
 * @param {LaidNode} d
 * @returns {boolean}
 */
function isContraNode(d) {
  const links = d.sourceLinks.concat(d.targetLinks);
  return links.length > 0 && links.every((l) => Boolean(l.contra));
}

/**
 * Whether every ribbon on a laid node is a partition one, so the mark's whole
 * figure is a cross-tab total rather than money that moved through it.
 *
 * ALL OR NOTHING, AS isContraNode IS. A node with one partition ribbon among
 * flows is a node whose sentence would be true of part of it, and a label that
 * qualified the whole mark would be wrong about the rest.
 * @param {LaidNode} d
 * @returns {boolean}
 */
function isPartitionNode(d) {
  const links = d.sourceLinks.concat(d.targetLinks);
  return links.length > 0 && links.every((l) => Boolean(l.partition));
}

/**
 * The figure a laid mark prints: negative for a contra ribbon and for a line
 * whose every ribbon is one, and d3's value otherwise.
 * @param {LaidLink | LaidNode} d
 * @returns {number}
 */
function markCents(d) {
  if (isLink(d)) {
    const l = /** @type {LaidLink} */ (d);
    return l.contra ? -l.value_cents : l.value_cents;
  }
  const n = /** @type {LaidNode} */ (d);
  return isContraNode(n) ? -n.value : n.value;
}

/**
 * For a node with contra ribbons among others, what its figure is gross of and
 * what it comes to net, or "" for every other node.
 *
 * IT IS ARITHMETIC AND SAYS SO, with columnShare's diamond and word: the
 * mark's own figure less TWICE what the reductions contributed to it, since
 * each is drawn at its magnitude and so was added where the schedule
 * subtracts it. Measured on FY2025-26: the Property Taxes category, the centre
 * of its own window, is sized at $103,430,092 — that is
 * $16,985,339 of reductions among $86,444,753 of additions, and
 * p127 prints $69,459,414, which nothing on the mark would say. That net is
 * what the two ribbons LEAVING the centre come to, so the note is what makes
 * the two sides of one mark agree.
 * @param {LaidNode} d
 * @returns {string}
 */
function contraNote(d) {
  const arriving = d.targetLinks.reduce((sum, l) => sum + l.value, 0);
  const leaving = d.sourceLinks.reduce((sum, l) => sum + l.value, 0);
  const side = arriving >= leaving ? d.targetLinks : d.sourceLinks;
  const reduced = side.filter((l) => l.contra).reduce((sum, l) => sum + l.value_cents, 0);
  if (!reduced || reduced === d.value) return "";
  return "\u25c7 our reading: " + fmt(reduced) + " of this is printed as reductions, so " +
    fmt(d.value - 2 * reduced) + " net of them";
}

/**
 * What a partition ribbon is, in the one place the words for it live.
 *
 * EVERY MARK THAT SHOWS IT SHOWS THE SAME SENTENCE -- the tooltip, the detail
 * panel, the flow table and the screen-reader label -- because four spellings
 * of one claim about the documents is four things to keep true. contra's words
 * come off the link because they name that row's own parent; this claim is the
 * same wherever it appears.
 *
 * THE DIRECTION DRAWN IS NOT A DIRECTION THE CITY PRINTED. Budget Book
 * pp.85-125 print one matrix of cells, divisions down and object categories
 * across; a chart can read it either way round and neither reading is money
 * moving. Drawing it forward and saying so is markContra's precedent: a ribbon
 * is never reversed, and what the shape means is carried in a class and a
 * sentence.
 */
const PARTITION_NOTE = "a cross-tab: one printed table read along a second axis, " +
  "not money moving in the direction drawn";

/**
 * The classes a ribbon is drawn with.
 * @param {LaidLink} d
 * @returns {string}
 */
function linkClass(d) {
  return "link" + (d.derived ? " derived" : "") + (d.contra ? " contra" : "") +
    (d.partition ? " partition" : "");
}

/**
 * The classes a node is drawn with.
 *
 * `opens` IS AN AFFORDANCE AND NOT A RESTATEMENT OF THE DOCUMENT. The other two
 * say what a mark IS -- an inference, a reduction of the category it is printed
 * under -- and are read off the node. This one says what the reader may do to
 * it, which is why it is drillable's answer: whether a mark opens depends on
 * the view's declared steps and on the chart it is drawn on, not on any field
 * the packager wrote.
 *
 * `expands` IS THE SAME KIND OF CLAIM ABOUT THE OTHER GESTURE, and it is
 * expandable's answer for expandable's reason. A folded tail is the other mark
 * a reader expects something to happen on -- measured over the chain, 9 of the
 * 44 views it opens draw one -- and what happens is a redraw of the column it
 * was cut out of rather than a rung.
 *
 * THE TWO ARE DISJOINT AND THE CLASS DOES NOT ENFORCE THAT. drillable refuses
 * an aggregate by name and expandable requires one, so no mark can carry both;
 * that is a property of the two predicates, asserted where they are measured,
 * rather than a precedence written here.
 * @param {LaidNode} d
 * @returns {string}
 */
function nodeClass(d) {
  return "node" + (d.derived ? " derived" : "") + (isContraNode(d) ? " contra" : "") +
    (drillable(d) ? " opens" : "") + (expandable(d) ? " expands" : "");
}

/**
 * The markers one node's label carries in its flag tspan.
 *
 * GLYPHS AND NOT WORDS: the label is already at the edge of its gutter. Each is
 * spelled out somewhere a reader can reach -- the diamond by the legend, the
 * tooltip and the derived list, the triangle by nodeDescription and by the
 * chart hint.
 *
 * THE FLAG TSPAN IS THE ONLY CHANNEL LEFT. Hue is spoken for by the palette's
 * contrast rule, the dashed rect by `derived`, --critical by `contra`, and a
 * dash pattern by the two a ribbon already carries. A glyph in the label
 * survives forced-colors and greyscale, costs no hue, and is read aloud.
 *
 * THEY COMPOSE, because nothing stops a node being an inference that also
 * opens. They draw in that order with nothing between them, so a pair costs
 * the label two glyph widths -- which is the width tools/jscheck/layout.mjs
 * fits it against, by calling this rather than by spelling it a second time.
 *
 * ONE PAIR IS REAL AND THE OTHER IS LATENT, and the difference is worth
 * stating. A folded tail is ALWAYS an inference -- capColumn writes derived on
 * it because the city printed no line called "24 smaller funds" -- so every
 * mark that expands carries the diamond beside the plus, on 9 of the 44 views
 * the chain opens. The diamond-and-triangle pair is the one no committed
 * document produces, and a rule written only for the marks that exist would be
 * a rule the first derived openable node breaks silently, in the label's own
 * gutter.
 *
 * NO MARK CARRIES THREE. Nothing can, because drillable and expandable are
 * disjoint; the gutter was measured against two.
 * @param {LaidNode} d
 * @returns {string}
 */
function nodeFlags(d) {
  const marks = (d.derived ? "\u25c7" : "") + (drillable(d) ? "\u25b8" : "") +
    (expandable(d) ? "\u229e" : "");
  return marks ? "  " + marks : "";
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
 * The prefix the residual node's id carries, followed by the opened node's id.
 *
 * PER OPENED NODE, as the aggregate's is per tier: one rung draws one
 * residual, beside the node it opened, and naming it after that node is what
 * keeps two rungs' residuals from ever sharing an id in one document.
 */
const RESIDUAL_PREFIX = "residual/";

/**
 * The id of the node an opened node's undecomposed flows are carried onto.
 * @param {string} opened
 * @returns {string}
 */
function residualID(opened) {
  return RESIDUAL_PREFIX + opened;
}

/**
 * Whether an id names a residual node.
 * @param {string} id
 * @returns {boolean}
 */
function isResidual(id) {
  return id.startsWith(RESIDUAL_PREFIX);
}

/**
 * The prefix a gap node's id carries, followed by the opened node's id.
 *
 * ITS OWN PREFIX AND NOT THE RESIDUAL'S, because the two marks make different
 * claims and a check counting one must not find the other: a residual is money
 * the chart above prints that the drawn document carries no row for, copied
 * across with its citations, and a gap is one cell two schedules print at two
 * figures, which no page prints at all. markGap says which is which.
 */
const GAP_PREFIX = "gap/";

/**
 * The id of the node an opened node's undecomposed difference is drawn at.
 * @param {string} opened
 * @returns {string}
 */
function gapID(opened) {
  return GAP_PREFIX + opened;
}

/**
 * Whether an id names a gap node.
 * @param {string} id
 * @returns {boolean}
 */
function isGap(id) {
  return id.startsWith(GAP_PREFIX);
}

/**
 * Whether an id names the residual node, the gap node, or an endpoint carried
 * with the residual: a mark the rung on screen added beside the opened node's
 * parts, which is not one of them and opens into nothing.
 *
 * THE GATE IS THE RESIDUAL'S DECLARED ENDPOINTS, NOT THE carried_from FLAG,
 * and the difference is the window feature itself. A window's kept flank is
 * carried onto the rung in exactly the sense that field records -- its figures
 * and its caveats are the chart above's -- and it MUST open: a fund group kept
 * beside a revenue category is the node the reader slides on to next. What
 * closes a mark here is the step's own residual declaration, which names the
 * endpoints whose money this chart cannot decompose, plus the residual node
 * itself. Widening this to "anything carried" is the one-line simplification
 * that would draw the window and refuse every click in it.
 * @param {string} id
 * @returns {boolean}
 */
function isCarried(id) {
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
 * THE DRILL PUTS ONE DOCUMENT INSIDE THE OTHER, AND THE TOTALS DO NOT MATCH.
 * The spine prints a fund group's inflow and outflow whole; the fund-level
 * schedule prints the same money by fund and by division and carries no row
 * for a fund-balance draw, a reserve increase or a transfer out. Drawn as is,
 * the opened General Fund shows 144,650,802 flowing out of a group the chart
 * above said takes in 159,388,024 (FY2025-26, dollars), and nothing tells the
 * reader why. The difference is the RESIDUAL: money the city printed at group
 * grain and nowhere finer. `fisc verify`'s drill-reconciles-across-documents
 * proves it is exactly the declared endpoints' share; this is what makes it
 * visible.
 *
 * CARRIED, NOT COMPUTED. Every link added here is a link of the chart above
 * with its value_cents, fact_ids, locators, kind and derived flag untouched --
 * only the group end is re-pointed, onto the residual node. Those links are
 * already covered by link-values-tie-to-facts and
 * link-locators-match-their-facts, so the figure on screen is a published one
 * with its provenance intact. Nothing here sums, subtracts or allocates, and
 * tools/jscheck/drill.mjs holds each carried link byte-equal to its original.
 *
 * NOT RE-POINTED ONTO A FUND. Capital has 11 funds and internal-service 5,
 * and pp.127-140 do not say which one a draw belongs to; attributing it would
 * invent an allocation. So the residual sits BESIDE the funds, parented to
 * the group, and the rule is the same for general, whose group has one fund:
 * its transfers out and reserve increase leave beside fund/100 rather than
 * through it. Conservative, never wrong, and free of a branch for the
 * one-fund case that nothing published would justify.
 *
 * THE NODE SET IS THE CHECK'S DECLARATION, READ OFF THE STEP. step.residual is
 * check.ResidualNodes() as the packager shipped it, ids to reasons; this file
 * spells no endpoint, and it carries each reason into the node's rationale so
 * the reader is told why a flow has no fund in the words the check declares
 * it in. Which of those endpoints are residual for THIS group is the
 * documents' own answer, under the same whole-or-nothing rule the check
 * applies: a declared endpoint's link into the group is carried only where
 * the step document carries NOTHING from that endpoint into the group's
 * parts, because where it carries any it carries all of it -- transfers in
 * reach eight funds of three groups to the cent, and copying those links too
 * would draw 21,045,597 twice (FY2025-26, dollars). A split is a finding the
 * check reports; this file does not look for one.
 *
 * THE OUTFLOW SIDE ONLY WHERE THE STEP DOCUMENT DECOMPOSES THE GROUP -- where
 * it carries a flow out of one of the group's parts. Every other group's money
 * ends at its funds, which publish no outflow; absent is not zero, and drawing
 * capital's transfers out as residual against an outflow of nothing would
 * state an identity no document holds. On the committed corpus that is
 * general alone, in both columns.
 *
 * THE IMBALANCE IS THE POINT. The node's inflow and outflow differ by
 * construction -- general's in FY2025-26 is 1,514,554 in and 14,737,222 out --
 * and d3-sankey sizes a node at the larger of the two, so the difference
 * shows on the mark rather than being balanced away. drill.mjs re-measures
 * both figures over both columns.
 *
 * WHY A CLIENT-SIDE DERIVED NODE IS RIGHT HERE, because the next reader will
 * ask. The capped tail is the precedent: derived: true, a rationale, a source
 * note, checked by drill.mjs and not by derived-nodes-justified. This is a
 * weaker claim than that one, because the aggregate SUMS and this COPIES.
 * The derived-node rule binds projections, and this page is neither a
 * projection nor a scenario; and the residual is a statement about the PAIR
 * of documents, which neither document can hold -- the page is the only place
 * both exist at once. Putting it in the fund-flows projection is refused on
 * its own grounds: it would put all-funds-gross and revenue-by-fund in one
 * scope set, which projection-scopes-are-disjoint refuses by name, and it
 * would read as capital being decomposed and silently drop the
 * only-the-General-Fund caveat.
 *
 * A GROUP WITH NOTHING TO CARRY DRAWS NOTHING. Three groups' transfers in are
 * decomposed whole and they draw no fund-balance row, so no node is added and
 * no endpoint is copied; a residual node with no links would be the
 * aggregate-of-nothing one tier up.
 *
 * @param {FiscProjection} drawn  the rung's document, shaped and folded
 * @param {FiscProjection | null} from  the document of the chart the rung was
 *   opened from
 * @param {Rung} rung
 * @returns {FiscProjection}
 */
function carryResidual(drawn, from, rung) {
  const step = rung.step;
  const residual = step.projection && from && step.residual && typeof step.residual === "object"
    ? step.residual : null;
  if (!residual) return drawn;
  const opened = rung.id;
  const inside = withinNode(rung.doc, opened);
  // WHAT THIS CHART DRAWS OF THE OPENED NODE'S PARTS, and not what the step
  // document could draw. The tier set decides it: {0,2,3} draws a group's funds
  // and no division, so its funds publish no outflow HERE however finely
  // pp.167-170 decompose them, and carrying an outflow against an outflow of
  // nothing would state an identity this chart does not hold. That is the rule
  // this file already applies to the five groups no schedule decomposes,
  // reached through the columns rather than through the file.
  //
  // The group node itself carries no flow in any published document -- it
  // exists to hold the hierarchy -- and is excluded by name rather than by
  // assumption.
  const decomposed = drawn.links.some((l) => inside.has(l.source) && l.source !== opened);
  const carriesFrom = (/** @type {string} */ e) =>
    rung.doc.links.some((l) => l.source === e && inside.has(l.target));
  const carriesTo = (/** @type {string} */ e) =>
    rung.doc.links.some((l) => l.target === e && inside.has(l.source));

  const id = residualID(opened);
  /** @type {FiscLink[]} */
  const links = [];
  /**
   * The chart's own copy of each link re-pointed below, dropped as it is: a
   * window keeps a flank of the chart above, so the links this re-points are
   * ALREADY DRAWN, pointing at the opened node. Copying the file's instead
   * would leave both -- run, not predicted: transfers/in left tier 0 at
   * 960,800 against the 480,400 p0067 prints for it, and the opened group
   * stood 1,514,554 taller than the ribbons under it with nothing saying so.
   * @type {Set<FiscLink>}
   */
  const spliced = new Set();
  /**
   * One endpoint's links off the chart above: this rung's own where it draws
   * them, and the FILE's where it does not, which is every step that keeps no
   * flank. The two cannot both be taken.
   * @param {(l: FiscLink) => boolean} want
   * @returns {FiscLink[]}
   */
  const above = (want) => {
    const here = drawn.links.filter(want);
    return here.length ? here : from.links.filter(want);
  };
  /** @type {Map<string, boolean>} endpoint id to whether its flow arrives */
  const ends = new Map();
  // Endpoints in id order, so the rationale reads the same on every build.
  for (const e of Object.keys(residual).sort()) {
    if (!carriesFrom(e)) {
      for (const l of above((l) => l.source === e && l.target === opened)) {
        links.push(Object.assign({}, l, { target: id }));
        ends.set(e, true);
        spliced.add(l);
      }
    }
    if (decomposed && !carriesTo(e)) {
      for (const l of above((l) => l.source === opened && l.target === e)) {
        links.push(Object.assign({}, l, { source: id }));
        ends.set(e, false);
        spliced.add(l);
      }
    }
  }
  if (!links.length) return drawn;

  // THE ENDPOINTS COME WITH THEIR LINKS, placed at the first drawn tier when
  // the flow arrives and the last when it leaves. Their own tiers are the
  // chart above's columns, which the step's tier set need not contain, and a
  // tier layOut's align cannot place is clamped to the first column -- the
  // shape drill.mjs records d3-sankey dying on.
  const tiers = step.tiers;
  const have = new Set(drawn.nodes.map((n) => n.id));
  const fromByID = new Map(from.nodes.map((n) => [n.id, n]));
  /** @type {FiscNode[]} */
  const added = [];
  for (const [e, arrives] of ends) {
    const node = fromByID.get(e);
    if (!node || have.has(e)) continue;
    added.push(Object.assign({}, node, {
      tier: arrives ? tiers[0] : tiers[tiers.length - 1], parent: "",
      // THAT THIS MARK IS NOT OF THE DRAWN DOCUMENT. Its caveats, and the
      // anchors for them, belong to the chart it was carried from, and
      // caveatsFor and caveatHref branch on this. Without it a figure lost the
      // qualification its own document attaches to it the moment it was
      // carried -- transfers/in and transfers/out carry transfer-legs-unpaired
      // on the spine and arrived here unmarked. fisc-bccu.
      //
      // THE STEM IS READ, NOT ONLY RECORDED. carriedSource resolves it
      // against the documents on the stack, which is what a window needs:
      // every window carries a flank, so a carried mark can sit two rungs
      // down with a chart above it that is not the spine, and a lookup fixed
      // at depth 0 would resolve its caveats and its source list to the wrong
      // document.
      carried_from: from.projection || "",
    }));
  }

  // THE RESIDUAL STANDS AT THE TIER THE GROUP'S PARTS ARE DRAWN AT: the
  // shallowest drawn tier of any node inside the group, read off the step
  // document rather than named, because "the fund tier" is that document's
  // vocabulary and not this file's.
  let tier = Infinity;
  for (const n of rung.doc.nodes) {
    if (n.id !== opened && inside.has(n.id) && tiers.includes(n.tier) && n.tier < tier) tier = n.tier;
  }
  if (!Number.isFinite(tier)) {
    throw new Error("cannot draw " + rung.doc.projection + ": " + opened +
      " has no part at a tier this step draws to stand the residual beside");
  }

  const labels = new Map(from.nodes.map((n) => [n.id, n.label]));
  const reasons = Array.from(ends.keys()).map((e) => (labels.get(e) || e) + ": " + residual[e] + ".");
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
    const doc = CONFIG && CONFIG.docs ? CONFIG.docs[docID] : undefined;
    const pages = Array.from(cited.get(docID) || []).sort((a, b) => a - b);
    return (doc && doc.title ? doc.title : docID) + " " +
      (pages.length === 1 ? "p." : "pp.") + pages.join(", ");
  }).join("; ");

  const node = {
    id: id,
    label: "Not broken down by fund",
    tier: tier,
    parent: opened,
    constraint_tier: "",
    role: "residual",
    derived: true,
    rationale: "Money the chart above prints for " + (labels.get(opened) || opened) +
      " as a whole and that the schedule this chart is drawn from does not split by fund, so " +
      "no fund here receives or pays it. It is drawn beside the funds rather than attributed " +
      "to one, and what flows in and what flows out need not balance: the difference is what " +
      "that schedule does not break down. " + reasons.join(" "),
    source_note: "Carried, not computed: " + links.length + " flow" + (links.length === 1 ? "" : "s") +
      " of the chart above with figures and citations unchanged \u2014 " + where + ".",
  };
  return Object.assign({}, drawn, {
    nodes: drawn.nodes.concat(added, [node]),
    links: drawn.links.filter((l) => !spliced.has(l)).concat(links),
  });
}

/**
 * States as a mark of its own the difference between what the chart above
 * sends into the opened node and what the document this rung draws breaks that
 * node into.
 *
 * ABSORBED IS THE FAILURE THIS EXISTS TO REFUSE. d3-sankey sizes a node at the
 * larger of what enters and what leaves, so a centre taking 130,502,087 from
 * the chart above and sending 130,252,087 into its parts draws at the larger
 * figure with 250,000 of node height and no ribbon against it. Nothing on the
 * page says so, and a reader who does not measure the marks sees a chart that
 * balances. That cell is FY2026-27 services-and-supplies, p0067 against Budget
 * Book pp.85-125, and it is the one the site actually draws.
 *
 * NOT carryResidual, AND THE LINE BETWEEN THEM IS carried VERSUS derived. That
 * function copies published links of the chart above onto a node beside the
 * parts, figures and citations untouched; nothing in it sums, subtracts or
 * allocates. A gap has no link to copy -- both documents draw the cell, at
 * figures that differ -- so the only mark that can state it is a derived one,
 * whose value is the difference and whose words are the packager's declaration.
 *
 * A STEP THAT DECLARES ANY GAP CLAIMS EVERY NODE IT OPENS BALANCES, and this
 * is where that claim has teeth: a shortfall on a node the declaration does not
 * name throws rather than drawing, which is the client-side twin of `fisc
 * verify`'s spending-window-reconciles. A step declaring none is left alone --
 * an opened fund group is deliberately unbalanced and says so on its residual,
 * and a rule that demanded balance everywhere would refuse it.
 *
 * THE AMOUNT IS NOT DECLARED AND CANNOT BE. A gap is per fiscal column and a
 * step is declared once for every year the view lists, so what rides on the
 * declaration is the REASON -- which names its own column, because the same
 * sentence is shown under both years and one that did not would be wrong under
 * the other.
 *
 * THE TWO SIDES ARE THE SIGNED ONES, WHICH IS WHY THIS RUNS BEFORE markContra.
 * A reduction is drawn forward at its magnitude, so after that pass a centre
 * taking its category's gross and sending the spine's net looks like a
 * shortfall of twice the reductions -- the Property Taxes window is exactly
 * that shape, $103,430,092 arriving against $69,459,414 leaving. The identity
 * the two schedules actually hold is the signed one, and it is the signed one
 * this reads: measured over both published columns, every one of the ten
 * revenue categories balances here to the cent.
 *
 * IT CARRIES NO kind. A kind says which boundary the money crosses, and the
 * difference between two schedules crosses nothing either of them printed;
 * `derived` is the claim this mark can make, and it makes it in the class, the
 * tooltip, the flow table and "What we inferred".
 *
 * @param {FiscProjection} drawn  the rung's chart, shaped, folded and spliced
 * @param {Rung} rung
 * @returns {FiscProjection} drawn itself where the step declares no gap at all,
 *   or the node it opened balances
 */
function markGap(drawn, rung) {
  const gaps = rung.step.gaps;
  if (!gaps || typeof gaps !== "object") return drawn;
  const opened = rung.id;
  const centre = drawn.nodes.find((n) => n.id === opened);
  if (!centre) {
    throw new Error("cannot draw " + (drawn.projection || "this chart") + ": " + opened +
      " is not a mark of it, so the gap this step declares has nothing to be stated against");
  }
  let into = 0;
  let outOf = 0;
  for (const l of drawn.links) {
    if (l.target === opened) into += l.value_cents;
    if (l.source === opened) outOf += l.value_cents;
  }
  const gap = into - outOf;
  if (gap === 0) return drawn;
  const declared = Object.prototype.hasOwnProperty.call(gaps, opened) ? gaps[opened] : "";
  if (!declared) {
    throw new Error("cannot draw " + (drawn.projection || "this chart") + ": the chart above " +
      "sends " + fmt(into) + " into " + centre.label + " and this one draws " + fmt(outOf) +
      " of it, a difference of " + fmt(Math.abs(gap)) + " that no declaration on this step " +
      "accounts for; the two documents have drifted apart");
  }
  // THE SHORT SIDE DECIDES WHERE THE MARK GOES, which is the same question in
  // both directions and needs no knowledge of which half of a window came from
  // which file: too little leaving stands at the last drawn column, too little
  // arriving at the first.
  const tiers = rung.step.tiers;
  const id = gapID(opened);
  const node = {
    id: id,
    label: "Difference between the two schedules",
    tier: gap > 0 ? tiers[tiers.length - 1] : tiers[0],
    // PARENTLESS, WHICH DRAWS IT --muted, and that is the claim: it belongs to
    // neither document's hierarchy.
    parent: "",
    constraint_tier: "",
    role: "gap",
    derived: true,
    rationale: "The chart above puts " + fmt(into) + " through " + centre.label +
      " and the schedule this chart is drawn from accounts for " + fmt(outOf) + " of it. " +
      declared + " This mark is what is left, drawn so that the ribbons and the node agree; " +
      "no page prints it as a figure of its own.",
    source_note: "Derived, not published: one document's total for this cell less the other's, " +
      "taken from the two charts on screen. `fisc verify` holds that difference to the figure " +
      "the reason above declares.",
  };
  const link = gap > 0
    ? { source: opened, target: id, value_cents: gap }
    : { source: id, target: opened, value_cents: -gap };
  return Object.assign({}, drawn, {
    nodes: drawn.nodes.concat([node]),
    links: drawn.links.concat([Object.assign({
      kind: "", transfer_id: "", fact_ids: [], locators: [], derived: true,
    }, link)]),
  });
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
 * @param {string} opened  the node the aggregate is parented to: the opened
 *   node when the whole column is inside it, "" when it spans fund groups
 * @param {string} noun  the plural noun for the tier's rows
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

  // RANKED BY THE LARGER OF INFLOW AND OUTFLOW, which is d3-sankey's own node
  // value and the height the reader sees. Inflow alone ranked every column
  // this page capped until a category opened into its lines: a line is the
  // SOURCE of everything it carries and takes in nothing, so under inflow
  // every line tied at zero and the tail was whichever eight sorted last by
  // id. On the columns capped before -- funds and divisions -- the two agree,
  // because a fund's outflow never exceeds its inflow and a division's equals
  // it, and tools/jscheck/drill.mjs pins every opened view at the figures it
  // had under inflow.
  //
  // BY MAGNITUDE, because a contra row is a printed line as large as its
  // figure. Ranked signed, ERAF at -$15,175,000 is the smallest line in
  // Property Taxes and the tail folds a reduction in with the additions it is
  // labelled "smaller" than; ranked by magnitude it is the second largest,
  // which is what p127 prints.
  /** @type {Map<string, number>} */
  const inflow = new Map();
  /** @type {Map<string, number>} */
  const outflow = new Map();
  for (const l of doc.links) {
    inflow.set(l.target, (inflow.get(l.target) || 0) + Math.abs(l.value_cents));
    outflow.set(l.source, (outflow.get(l.source) || 0) + Math.abs(l.value_cents));
  }
  const size = (/** @type {string} */ id) => Math.max(inflow.get(id) || 0, outflow.get(id) || 0);
  // Ties broken by id, so the set kept is the same on every build of the same
  // document. A cap that reordered under an unstable sort would move which
  // funds a reader sees between two identical exports.
  const ranked = atTier.slice().sort((a, b) =>
    size(b.id) - size(a.id) || (a.id < b.id ? -1 : 1));
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
    // PARENTED AS THE CALLER DECIDED: at the node being opened when every
    // item folded into it is inside that node, which is what gives the mark
    // its group's hue instead of --muted -- it was "" and drew grey among
    // coloured siblings -- and "" where the column spans fund groups, which
    // shapeFor asks of the document.
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
 * pair and the kind, summing values and unioning fact ids. A link whose ends
 * fold to the SAME node is dropped: it was a flow inside what is now one box. That is the
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
    // ONE RIBBON PER KIND BETWEEN A FOLDED PAIR, so a ribbon's kind is true of
    // all of it. Keying the merge on the pair alone would draw an internal
    // service charge and money crossing the city's boundary as one ribbon.
    // Measured over every view the page opens, on both published columns:
    // exactly two drawn pairs carry both kinds, the Intergovernmental line's
    // rollup into its category and Use of Money and Property's -- the 2 of
    // pp.127-140's 93 rows that reach the five Internal Service Funds as an
    // internal service charge and the rest of the city as external revenue.
    // fund-flows publishes each as two (1,0) links, so this branch is what
    // keeps them two ribbons rather than what makes them two, and the tooltip
    // and the table name each for what it is.
    const key = source + "\u001f" + target + "\u001f" + l.kind;
    const at = merged.get(key);
    const ids = cited.get(key);
    if (!at || !ids) {
      merged.set(key, Object.assign({}, l, { source: source, target: target }));
      cited.set(key, new Set(l.fact_ids));
      located.set(key, new Set(locatorKeys(l.locators)));
      continue;
    }
    // A PRINTED LEG AND AN INFERRED ONE CANNOT MERGE, and it is this project's
    // oldest rule: published is not
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
      : a.target < b.target ? -1 : a.target > b.target ? 1
      : a.kind < b.kind ? -1 : a.kind > b.kind ? 1 : 0));

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
 * Which column d3-sankey puts a node in: the position of its tier in the
 * declared order, or d3's own justify when nothing was declared.
 *
 * A DECLARED ORDER IS A CLAIM ABOUT POSITION AND sankeyJustify IS NOT. Justify
 * derives the columns from topology and puts a link-less sink in the LAST one,
 * which is right for a document drawn whole and wrong the moment tiers can be
 * skipped: a node terminating early is shoved across the chart to sit among
 * nodes it shares nothing with. It is also what leaves "the column to the left
 * of this one" with no answer, which is why a view that opens a node has to
 * declare its columns -- internal/export's View.RenderTiers refuses one that
 * does not.
 *
 * THE SPINE'S OWN FIGURES DO NOT MOVE WHEN IT DECLARES ITS ORDER, and that is
 * measured rather than argued: tools/jscheck/layout.mjs lays the committed
 * goldens out through this function, so the crossing and overlap figures below
 * are the ones the page draws under whatever the page declares. Both aligners
 * were run over both published spine columns and agreed to the digit, because
 * the spine's tier 0 is pure source and its tier 5 pure sink and justify's own
 * rule puts a link-less sink where indexOf puts tier 5.
 *
 * THE ORDER MAY BE NON-MONOTONIC, and a window is why: {2,5,4} draws fund
 * groups, then the object category they pay for, then the divisions that spend
 * it, and indexOf says so where a sort by tier number would not.
 *
 * THE TIER SET IS THE CALLER'S, not the page's, and that distinction exists
 * because a page can open a node. A drilled document is folded to its step's
 * tiers -- {0,3} on Revenue against the page's {0,2} -- so aligning on the
 * page's set gives every tier-3 fund indexOf === -1, which d3 clamps to column
 * 0. Measured: that leaves the layer array with a hole and d3-sankey dies
 * inside its own ordering pass with "Cannot read properties of undefined
 * (reading 'sort')" -- a blank chart under a banner, on the first click of a
 * feature whose whole point is the click.
 *
 * @param {number[]} tiers
 * @returns {(d: LaidNode) => number}
 */
function alignFor(tiers) {
  return tiers.length
    ? /** @param {LaidNode} d */ (d) => tiers.indexOf(d.tier)
    : D3.sankeyJustify;
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

  const sankey = D3.sankey()
    .nodeId(/** @param {LaidNode} d */ (d) => d.id)
    .nodeWidth(NODE_WIDTH)
    .nodePadding(NODE_PADDING)
    .nodeAlign(alignFor(activeTiers()))
    // Supplying this switches d3's own ordering pass off, which is what makes
    // the fund column's colour adjacency a property of the page rather than of
    // the library. nodeRank puts the crossing count back.
    .nodeSort(/** @param {LaidNode} a @param {LaidNode} b */ (a, b) =>
      nodeRank(a) - nodeRank(b) || b.value - a.value)
    // THE EXTENT IS SIZED FROM THE COLUMNS THIS CHART DRAWS, and render() reads
    // the same count for the viewBox: a drawing laid out at one width inside a
    // viewBox of another is the whole chart stretched or squeezed.
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
 * The column a node was drawn in, as an index into the declared order.
 *
 * NOT d.depth, WHICH IS THE LONGEST PATH TO THE NODE and answers a different
 * question. The two agree on a chart every path through which is the same
 * length -- the spine's is -- which is why labelling keyed on depth for as long
 * as the spine was the only chart. Measured on the committed corpus: the
 * fund-group window {0,2,3} draws one node in column 2 whose depth is 1,
 * because no ribbon reaches it from column 1.
 *
 * NOT d3's d.layer EITHER: layer is the clamped output of the aligner this same
 * declaration is handed to, so reading it back is a second source of one fact.
 * The exception is a view that declares no column order at all, where
 * sankeyJustify chose the columns and d.layer is the only record of what it
 * chose.
 *
 * @param {LaidNode} d
 * @returns {number}
 */
function columnOf(d) {
  const tiers = activeTiers();
  // d3 clamps an aligner's answer into the drawn range, so a node whose tier
  // the view does not declare is drawn in column 0 and is labelled as one.
  return tiers.length ? Math.max(0, tiers.indexOf(d.tier)) : d.layer;
}

/**
 * Where a node's label goes: the side it is anchored on, and the point it is
 * anchored at.
 *
 * A LABEL MAY RUN OUTWARD ONLY INTO A GUTTER. The extent reserves LABEL_GUTTER
 * px outside the first and last columns and nothing at all between columns, so
 * the first column's label reads leftward out of the chart and the last
 * column's rightward. An interior column has neither gutter: a label anchored
 * to the right of its rect claims the band the next column's ribbons arrive
 * through, and the further right that column sits the less of that band is
 * left. Centred over its own rect it claims half as much on each side, in the
 * NODE_PADDING gap above the rect rather than across the middle of it.
 *
 * AN INTERIOR COLUMN IS NOT A FUTURE SHAPE. Three columns have one already:
 * the spine's fund groups, and the centre of every window a step opens.
 *
 * @param {LaidNode} d
 * @param {number} last the largest column index this chart drew
 * @returns {{anchor: string, x: number, y: number, dy: string|null}}
 */
function labelPlacement(d, last) {
  const col = columnOf(d);
  const middle = (d.y0 + d.y1) / 2;
  if (col === 0) return { anchor: "end", x: d.x0 - 10, y: middle, dy: "0.35em" };
  if (col >= last) return { anchor: "start", x: d.x1 + 10, y: middle, dy: "0.35em" };
  // NO dy ON THE INTERIOR PLACEMENT: the baseline is already where the text
  // belongs, 5px clear of the rect's top edge, and a half-em shift down would
  // drop the glyphs onto the rect the label names.
  return { anchor: "middle", x: (d.x0 + d.x1) / 2, y: d.y0 - 5, dy: null };
}

/**
 * The qualifier each mark needs to be told from the ones drawn beside it: its
 * parent's label, on every mark whose own label another mark in the same column
 * also carries, and nothing at all anywhere else.
 *
 * AN AMBIGUITY IS A PROPERTY OF THE COLUMN AND NOT OF THE NODE, which is why
 * this is here and not in the document. fund-flows names a tier-5 cell by its
 * object category and carries the division in `parent`. In a division's own
 * window that is the right label -- the division is the mark to its left and
 * the breadcrumb says it -- and in the fund window, whose fourth column draws
 * the largest cells of eight different divisions, it draws six marks reading
 * "Wages & Benefits". The pair does not fit on one line either way: measured by
 * tools/jscheck/layout.mjs, "Fire Administration — Services & Supplies" wants
 * 348px of a gutter that is 250px wide, so the qualifier is a line of its own
 * and is spent only where a reader could not otherwise tell two marks apart.
 *
 * THE PARENT IS LOOKED UP IN THE DOCUMENT, NOT IN THE LAID GRAPH. A window
 * draws the tiers its step declares, so a mark's parent is routinely not on
 * screen -- and the qualifier is the word for where the mark came from, which
 * is a fact about the document rather than about what is drawn.
 *
 * A DUPLICATE WHOSE PARENT IS UNNAMEABLE GETS "" AND STAYS AMBIGUOUS. There is
 * no such mark on the committed corpus; the label check names one if it appears
 * rather than this inventing a word for it.
 *
 * @param {LaidNode[]} nodes
 * @returns {Map<string, string>}
 */
function labelQualifiers(nodes) {
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
 * line before it.
 *
 * THE QUALIFIER GOES ABOVE THE LABEL IN BOTH PLACEMENTS, and the two differ in
 * what that costs. An outward label is anchored on its rect's middle, so the
 * pair straddles the middle and the block stays centred on the mark it names.
 * An interior one is already sitting in the NODE_PADDING gap above its rect,
 * with nowhere below to go, so the qualifier is lifted a whole line further and
 * the label line does not move.
 *
 * THE INTERIOR BRANCH IS REASONED AND NOT MEASURED. No column but the last
 * repeats a label on the committed corpus -- measured over all six goldens, the
 * only duplicates anywhere are fund-flows' 44 tier-5 cells, and tier 5 is drawn
 * last in every window that reaches it -- so nothing draws this branch and no
 * check can see it. fisc-xhqt carries the measurement and what would retire it;
 * the vertical arm in tools/jscheck/layout.mjs is what would name an interior
 * pair if a real document ever drew one.
 *
 * @param {string} anchor
 * @returns {{qualifier: string, label: string}}
 */
function labelLineShift(anchor) {
  return anchor === "middle"
    ? { qualifier: "-1.15em", label: "1.15em" }
    : { qualifier: "-0.6em", label: "1.15em" };
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
  const width = chartWidth(drawnColumns());
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
    .attr("class", /** @param {LaidLink} d */ (d) => linkClass(d))
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
    .attr("class", /** @param {LaidNode} d */ (d) => nodeClass(d))
    .attr("tabindex", 0)
    .attr("role", "button")
    // aria-pressed ON EVERY NODE, BECAUSE EVERY NODE IS A TOGGLE. It is the
    // isolation, and the legend announces its copy of it the same way;
    // applyEmphasis keeps this in step. A node that opens is no exception: its
    // single click and its Space isolate exactly as every other node's do.
    // OPENING IS NOT THE TOGGLE and must never be announced as one -- it
    // replaces the chart and takes the node with it, so there is no pressed
    // state to return to. What a node opens into is announced by the label.
    .attr("aria-pressed", "false")
    .attr("aria-label", /** @param {LaidNode} d */ (d) => nodeDescription(d))
    // BOTH KEYS ON EVERY NODE, because both activate every node. Which of them
    // opens is nodeDescription's sentence: aria-keyshortcuts is a list of keys
    // and has no slot for what a key means, so it can only fail to mention one.
    .attr("aria-keyshortcuts", "Enter Space")
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
    //
    // A SECOND MEANING ON ONE ACTIVATION OF ONE ELEMENT IS A NEW INTERACTION
    // CONTRACT AND NOT A REUSE, which is fisc-ppkq's third objection and still
    // holds. What has changed is the answer to it: the contract is now TWO
    // gestures rather than one. A single click and Space isolate, on every node
    // of every view; a double click and Enter open the nodes that open. Neither
    // gesture carries two meanings, and no view has to choose.
    //
    // AN OPENED CHART IS WORTH ISOLATING ON, which is what makes two gestures
    // necessary rather than merely possible. A window is three columns at its
    // narrowest, and tools/jscheck/drill.mjs walks the chain and measures it:
    // all 44 views it opens draw three at the page's own budget, and at a
    // four-column budget one of them draws four. None draws two. So every
    // opened view has a middle column, and dimming everything not adjacent to
    // one node takes real ribbons off it.
    //
    // AND THE ISOLATE IS THE GESTURE MOST MARKS HAVE. Of the 343 nodes those
    // views draw, 24 open. Putting the drill on the single click would give 7%
    // of an opened chart's marks one meaning and 93% of them another, on the
    // same mark shape, told apart only by trying one.
    .on("click", /** @param {MouseEvent} e @param {LaidNode} d */ (e, d) => {
      e.stopPropagation();
      clickNode(d, e.timeStamp);
    })
    // THE DOUBLE CLICK IS WIRED ON EVERY NODE AND NOT ONLY ON ONE THAT OPENS.
    // A reader who double clicks a mark that does not open has still made two
    // clicks, and those two have already toggled the isolation on and off
    // again; without this the gesture would silently discard whatever was
    // isolated before it. preventDefault is for the text selection a double
    // click otherwise leaves across the label.
    .on("dblclick", /** @param {MouseEvent} e @param {LaidNode} d */ (e, d) => {
      e.stopPropagation();
      e.preventDefault();
      doubleClickNode(d, e.timeStamp);
    })
    .on("keydown", /** @param {KeyboardEvent} e @param {LaidNode} d */ (e, d) => {
      if (e.key !== "Enter" && e.key !== " ") return;
      if (e.repeat) return;
      e.preventDefault();
      keyNode(d, e.key, e.timeStamp);
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

  // AN UNQUALIFIED LABEL DRAWS EXACTLY WHAT IT DREW BEFORE. The qualifier tspan
  // is empty on those marks and carries neither an x nor a dy, so it starts no
  // line and shifts nothing; only a mark a reader could confuse with another
  // pays the second line.
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
  return d.source.label + " to " + d.target.label + ", " + fmtSigned(markCents(d)) + ", " +
    (/** @type {Record<string,string>} */ (KIND_LABEL)[d.kind] || d.kind) +
    (d.contra ? ", " + d.contra : "") +
    (d.partition ? ", " + PARTITION_NOTE : "") +
    (d.derived ? ", inferred by us" : ", printed by the city");
}

/**
 * @param {LaidNode} d
 * @returns {string}
 */
function nodeDescription(d) {
  // WHAT EACH GESTURE DOES, for a reader who cannot see which marks carry the
  // triangle. Every node says it, because every node has two gestures now and a
  // mark that named neither would leave a keyboard reader to discover the
  // difference by pressing keys and watching a chart they cannot watch.
  //
  // THE SENTENCE NAMES SPACE ON A NODE THAT OPENS AND NOT ON ONE THAT DOES NOT,
  // which is not an inconsistency: on a mark that opens, Space is the key that
  // does the OTHER thing, and that is the whole of what has to be learned. On a
  // mark that does not, every activation means the same thing and there is no
  // split to announce.
  // AND WHAT THE FOLDED TAIL'S GESTURE DOES, in the words the chip that undoes
  // it uses. "Opens into its parts" would be the wrong sentence on a mark that
  // opens nothing: what the reader gets is this column drawn at every mark it
  // holds, on the chart they are already on.
  const what = drillable(d)
    ? ", opens into its parts on a double click or Enter; a single click or Space follows " +
      "this money"
    : expandable(d)
      ? ", draws all of them separately on a double click or Enter; a single click or Space " +
        "follows this money"
      : ", follow this money";
  const note = contraNote(d);
  // THE CROSS-TAB SENTENCE REACHES A READER WHO CANNOT SEE THE RIBBONS. The
  // class on the ribbon and the chip in the tooltip both need eyes; a mark
  // whose every flow is a partition announces the same qualification here, in
  // the words the chips stand for.
  return d.label + ", total " + fmtSigned(markCents(d)) +
    (d.derived ? ", inferred by us" : ", printed by the city") +
    (isPartitionNode(d) ? ", " + PARTITION_NOTE : "") +
    (note ? ", " + note.replace(/^\u25c7 /, "") : "") + what;
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
 * EXCEPT FOR A CARRIED MARK, WHOSE CAVEAT CAME FROM THE CHART ABOVE. The step
 * document declares nothing about a mark it does not carry, so resolving a
 * carried mark's caveat against the rung lands back on fisc-ko1j.13's symptom
 * by the other route: a summary with no link. `carried` is the caller's,
 * because the id here is a CAVEAT's and the question is about the NODE the
 * caveat was read off -- one caveat can mark a carried node and a drawn one on
 * the same chart. fisc-bccu.
 *
 * THE STEM PICKS THE DEPTH AND THE DEPTH PICKS THE REFS. A mark carried from
 * the spine takes the year's anchors and one carried from a fund-flows chart
 * takes that step's, which is a difference only a chain deeper than one hop
 * can have -- and every window has one, since a window keeps a flank of
 * whatever it opened from. A stem no document on the stack carries gets NO
 * anchor rather than the year's: a summary without a link is a visible loss,
 * and a link into the wrong document's caveats page is not.
 *
 * @param {string} id
 * @param {string} [carried]  the projection stem the mark this caveat was read
 *   off was carried from, so its anchors are that document's
 * @returns {string}
 */
function caveatHref(id, carried) {
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
  if (!projection) return [];
  // A CARRIED MARK IS OF THE CHART ABOVE, AND SO ARE ITS CAVEATS. carryResidual
  // copies the spine's endpoints onto the rung; the rung's own document has
  // never heard of them, so filtering its caveats returns nothing however the
  // walk below resolves. fisc-bccu.
  const carried = projection.nodes.find((n) => n.id === id && n.carried_from);
  const source = carried ? carriedSource(carried.carried_from) : projection;
  if (!source || !source.metadata || !Array.isArray(source.metadata.caveats)) {
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
  return source.metadata.caveats.filter((c) =>
    Array.isArray(c.applies_to) && c.applies_to.some(reaches));
}

/**
 * Where on the stack the document named by a projection stem sits, or -1.
 *
 * DEEPEST FIRST, because a carried mark came from the chart immediately above
 * it and a chain may draw one document at several depths. The answer is a
 * DEPTH and not a document because the two things resolved from it live in
 * different places: the document itself is docAt's, and the caveat anchors for
 * it are the year's at depth 0 and the step's below that.
 *
 * @param {string} stem
 * @returns {number}
 */
function depthOfDocument(stem) {
  for (let depth = drilled.length; depth >= 0; depth--) {
    const doc = docAt(depth);
    if (doc && doc.projection === stem) return depth;
  }
  return -1;
}

/**
 * The document a carried mark came from, resolved BY THE STEM IT RECORDS.
 *
 * NOT docAt(0), AND NOT drilled[0].doc EITHER. The first is the spine, which is
 * the chart above only while nothing below depth 1 carries anything; the second
 * is the step document, which is the chart the mark was carried ONTO. Every
 * window keeps a flank of the chart it opens from, so a carried mark two rungs
 * down came from a fund-flows chart and not from the spine, and a caveat or a
 * source list resolved at depth 0 would be another document's.
 *
 * NULL RATHER THAN A GUESS when no document on the stack carries that stem: a
 * mark losing its caveats is visible, and a mark wearing the wrong document's
 * is not.
 *
 * @param {string} stem
 * @returns {FiscProjection | null}
 */
function carriedSource(stem) {
  const at = depthOfDocument(stem);
  return at < 0 ? null : docAt(at);
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
  const color = asLink ? linkColor(/** @type {LaidLink} */ (d)) : nodeColor(/** @type {LaidNode} */ (d));

  // Values lead, labels follow: here the reader already knows what they are
  // pointing at and wants the number.
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
    meta.append(h("span", l.derived ? "chip derived" : "chip", l.derived ? "◇ inferred" : "printed"));
    if (l.contra) {
      meta.append(document.createTextNode(" "));
      meta.append(h("span", "chip contra", "reduction"));
    }
    if (l.partition) {
      meta.append(document.createTextNode(" "));
      meta.append(h("span", "chip partition", "cross-tab"));
    }
    tip.append(meta);
    // THE SENTENCE, NOT ONLY THE CHIP: "reduction" says what kind of row this
    // is, and the words say what it reduces. "cross-tab" is the same shape one
    // claim over.
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

  panel.append(h("div", "amount", fmtSigned(markCents(d))));
  panel.append(h("div", "", asLink
    ? /** @type {LaidLink} */ (d).source.label + " → " + /** @type {LaidLink} */ (d).target.label
    : /** @type {LaidNode} */ (d).label));

  const chips = h("div", "prov");
  if (asLink) {
    const l = /** @type {LaidLink} */ (d);
    chips.append(h("span", "chip", /** @type {Record<string,string>} */ (KIND_LABEL)[l.kind] || l.kind));
    chips.append(h("span", l.derived ? "chip derived" : "chip", l.derived ? "◇ our inference" : "printed by the city"));
    if (l.contra) chips.append(h("span", "chip contra", "reduction"));
    if (l.partition) chips.append(h("span", "chip partition", "cross-tab"));
    panel.append(chips);
    if (l.contra) panel.append(h("p", "why", l.contra));
    if (l.partition) panel.append(h("p", "why", PARTITION_NOTE));
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
    const note = contraNote(n);
    if (note) panel.append(h("p", "why", note));
    // THE CAVEAT IN FULL IS ONE CLICK AWAY, and the summary is here. The
    // tooltip can only afford the line; this panel is where a reader has asked
    // for the detail, so it is where the link belongs. The href is the same
    // anchor every caveat summary on the page uses -- composed by the packager
    // per (document, caveat), so it lands on THIS year's copy of the sentence.
    for (const c of caveatsFor(n.id)) {
      const why = h("p", "why");
      why.append(document.createTextNode("\u26a0 " + c.summary + " "));
      const href = caveatHref(c.id, n.carried_from);
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
  // BUT "THE DOCUMENT'S" MEANS THE ONE THE MARK IS OF. A carried endpoint and
  // the residual beside it are of the chart above, so falling back to the drawn
  // document sent a reader to pp.127-140 for a figure printed on p.66 -- and
  // the residual's own source_note names p.66 two lines higher, so the panel
  // contradicted itself. Measured before this: 54 anchors, none of them p.66,
  // on every carried mark in both columns. Same seam as the caveats, and for
  // the same reason. fisc-bccu's sibling, found by pass two of /code-review.
  //
  // The label stays "Sources:" and not "Records:" -- citations() emits PDF,
  // extracted-text AND records anchors, so naming it for the last would name a
  // third of the row.
  const prov = h("div", "prov");
  prov.append(h("span", "subtle", "Sources:"));
  const mark = asLink ? null : /** @type {LaidNode} */ (d);
  // A CARRIED MARK RECORDS ITS STEM AND THE RESIDUAL DOES NOT. The residual is
  // ours rather than any document's -- carryResidual builds it here -- and the
  // chart its copied links came from is by construction the one the rung was
  // opened from, so that is where it is asked for. Both fall back to the drawn
  // document rather than to nothing.
  const of = mark && mark.carried_from ? carriedSource(mark.carried_from)
    : mark && isResidual(mark.id) ? docAt(drilled.length - 1)
      : null;
  const ofDocument = (of && of.metadata && of.metadata.sources) || projection.metadata.sources;
  for (const c of citations(asLink ? /** @type {LaidLink} */ (d).locators : ofDocument)) {
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
 * EMPTY ON EVERY OPENED VIEW, BY RULE AND NOT BY ACCIDENT, and the rule is the
 * only thing holding it: a window keeps a whole flank of the chart it was
 * opened from, and the revenue category's window keeps the fund-group column
 * itself.
 *
 * THE DECISION RESTS ON WHAT A SWATCH IS, not on what an opened view draws. A
 * swatch is a toggle on a NODE id: setIsolated dims whatever is not adjacent to
 * that node. Where an opened view carries no fund-group node there is nothing
 * for a swatch to toggle, and where a window keeps that column there is one
 * swatch per mark in it, each dimming everything the reader opened the node to
 * see. A legend that meant a group rather than a node needs emphasis resolved
 * through fundGroupOf, which is a second emphasis model and is filed as
 * fisc-0jy9; that a category's funds carry nothing saying which group each is
 * in is fisc-b4a6.
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
    // A CONTRA ROW READS AS THE SCHEDULE PRINTED IT: a signed figure, and in
    // place of "printed" the words for what it reduces, so the table says the
    // same thing the tooltip and the ribbon do.
    if (l.contra) tr.className = "contra";
    tr.append(h("td", "", labels.get(l.source) || l.source));
    tr.append(h("td", "", labels.get(l.target) || l.target));
    tr.append(h("td", "num", fmtSigned(l.contra ? -l.value_cents : l.value_cents)));
    tr.append(h("td", "", /** @type {Record<string,string>} */ (KIND_LABEL)[l.kind] || l.kind));
    tr.append(h("td", "", l.derived ? "◇ inferred"
      : l.contra ? l.contra
        : l.partition ? PARTITION_NOTE : "printed"));
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

/**
 * How many columns the reader's window has room for, which is never fewer than
 * the floor.
 *
 * matchMedia is feature-checked for prefersDark()'s reason and answered the
 * same way when it is missing: a page that cannot ask about the viewport draws
 * the narrow chart, which is a chart, rather than declining to draw one.
 * @returns {number}
 */
function viewportColumns() {
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
 * A SAVED VALUE OUT OF RANGE IS NOT CLAMPED, IT IS DISCARDED. A 5 left in
 * storage by a build whose ceiling was higher is not a choice between the
 * options this one offers, and clamping it to 4 would put the page in the
 * overridden state -- deaf to the viewport -- on behalf of a reader who never
 * asked for 4.
 *
 * localStorage throws rather than returning null in some privacy modes, which
 * is why this is wrapped; wireTheme's setItem is wrapped for the same reason.
 * @returns {number | null}
 */
function savedColumns() {
  try {
    const raw = localStorage.getItem("fisc-columns");
    if (raw === null) return null;
    const n = Math.floor(Number(raw));
    if (!Number.isFinite(n) || n < NARROW_COLUMNS || n > WIDE_COLUMNS) return null;
    return n;
  } catch (e) {
    return null;
  }
}

/**
 * Brings the +/- control back into agreement with the budget.
 *
 * DISABLED AT THE BOUNDS IS HOW THE FLOOR IS DISCOVERABLE. Three is not a
 * number this page can explain in the header, and a minus that visibly cannot
 * be pressed says it without a sentence. The attribute is set rather than the
 * property, so it is the same thing the template ships and the same thing
 * wireYears removes.
 */
function syncColumns() {
  const count = maybeEl("column-count");
  if (count) count.textContent = columnBudget + " columns";
  const bound = (/** @type {string} */ id, /** @type {boolean} */ atBound) => {
    const button = maybeEl(id);
    if (!button) return;
    if (atBound) button.setAttribute("disabled", "");
    else button.removeAttribute("disabled");
  };
  bound("column-fewer", columnBudget <= NARROW_COLUMNS);
  bound("column-more", columnBudget >= WIDE_COLUMNS);
}

/**
 * Puts the wanted budget into effect and repaints the chart if that moved it.
 *
 * THE REPAINT IS redrawStack(drilled) AND NOTHING ELSE. The rung's document and
 * the chart it was opened from are both already recorded, so the same stack
 * reshaped at the new budget is the whole of the work: no rung is popped, no
 * file is fetched a second time, and redrawStack shapes and lays out before it
 * mutates a single element (fisc-bsg), so a budget that will not lay out leaves
 * the reader on the chart they were already looking at rather than under
 * another chart's controls.
 *
 * IT ASKS drawnColumns AND NOT columnBudget WHETHER TO REDRAW. The two are
 * different questions: the overview is drawn at RENDER_TIERS whatever the
 * budget, so raising it there changes no column and a redraw would only clear
 * the reader's pin and their isolation for nothing.
 *
 * @param {boolean} redraw false during boot, where there is no document yet
 */
function applyColumns(redraw) {
  const before = drawnColumns();
  const moved = setColumnBudget(columnOverride === null ? viewportColumns() : columnOverride);
  syncColumns();
  if (!moved || !redraw || !projection) return;
  if (drawnColumns() === before) return;
  redrawStack(drilled);
}

/**
 * Takes the reader's step, records it as theirs, and repaints.
 *
 * RECORDING IT IS WHAT MAKES IT SURVIVE THE NEXT MEDIA CHANGE. Setting the
 * budget alone leaves columnOverride null, and the first query to fire after
 * that -- a rotation, a window drag across the threshold -- silently returns
 * the page to the viewport's answer over the reader's. lifecycle.mjs drives
 * exactly that.
 *
 * @param {number} delta
 */
function stepColumns(delta) {
  const want = Math.min(WIDE_COLUMNS, Math.max(NARROW_COLUMNS, columnBudget + delta));
  if (want === columnBudget) return;
  // BACK TO THE VIEWPORT'S OWN ANSWER IS A RELEASE, NOT A CHOICE. It is the
  // reader's way of handing the decision back, and without it the first press
  // of either button would deafen the page to the window for good.
  const released = want === viewportColumns();
  columnOverride = released ? null : want;
  try {
    if (released) localStorage.removeItem("fisc-columns");
    else localStorage.setItem("fisc-columns", String(want));
  } catch (e) { /* private mode: the choice still holds for this visit */ }
  applyColumns(true);
}

/**
 * Wires the column control and the queries that move it when the reader has
 * expressed no preference.
 *
 * IT SETS THE BUDGET AND DOES NOT DRAW. main() calls this before the first
 * fetch, for wireYears' reason -- every affordance is live for the whole of the
 * opening fetch -- and at that point there is no document to lay out: a redraw
 * here would reach redrawStack's "no document to open" and banner a refusal at
 * a reader who has done nothing. The opening draw reads columnBudget like any
 * other.
 *
 * The buttons ship disabled, as the year group does and for the same reason,
 * and enabling them is this function's enhancement; syncColumns immediately
 * re-disables whichever one is at its bound.
 */
function wireColumns() {
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
 * Reports whether a fetched rung answer carries the shape this page reads,
 * refusing visibly when it does not.
 *
 * THE RULE IS drawableSankey's: every key the page DEREFERENCES, and no more.
 * An answer missing one of them is not a chart drawn slightly wrong, it is a
 * TypeError inside a click handler, which is the shape that leaves a reader
 * looking at a chart nothing will admit is broken.
 *
 * NOT A SECOND IMPLEMENTATION OF THE WALK THAT WROTE IT. Whether Go's answer
 * is RIGHT is pkg/cmd/export's TestTheRungArtifactIsWhatGoComputes and
 * tools/jscheck/chart.mjs; all this asks is whether it can be read at all.
 *
 * @param {any} doc
 * @param {string} what
 */
function readableRungs(doc, what) {
  const missing = [];
  if (!Array.isArray(doc.columns)) missing.push("columns");
  else if (doc.columns.some((c) => typeof c.stem !== "string" || !Array.isArray(c.rungs))) {
    missing.push("columns[].stem, columns[].rungs");
  } else if (doc.columns.some((c) => c.rungs.some((r) => !Array.isArray(r.path) ||
    typeof r.width !== "number" || !Array.isArray(r.draws)))) {
    missing.push("columns[].rungs[].path, .width, .draws");
  } else if (doc.columns.some((c) => c.rungs.some((r) => r.draws.some((d) =>
    typeof d.tier !== "number" || !Array.isArray(d.ids))))) {
    // ONE ELEMENT DEEPER, AND ids IS THE KEY THAT NEEDS IT. Go writes it even
    // when empty, precisely so a column answered with nothing can be told from
    // a column left out -- and a reader that accepted the absence would read
    // the first as the second and draw a column Go says holds nothing.
    missing.push("columns[].rungs[].draws[].tier, .ids");
  }
  if (!missing.length) return true;
  fail(
    "This page will not open anything: " + what + " declares schema_version " +
    RUNGS_SCHEMA + ", which promises " + missing.join(", ") + ", and the file does " +
    "not carry " + (missing.length === 1 ? "it" : "them") + ". Your browser may be " +
    "holding a copy from before the last update — reload the page. Otherwise the " +
    "file is truncated or is not the answer this page expected. Nothing on the " +
    "page was changed."
  );
  return false;
}

/**
 * Fetches Go's rung answer and indexes it, or refuses in words.
 *
 * ITS OWN FETCH AND NOT loadDocument's, because every guard in that one is
 * about a SANKEY document -- drawableSankey names nodes, links and locators,
 * and this file carries none of them. Sharing it would mean a flag deciding
 * which half of the vetting applies, which is the shape that ships a file
 * vetted by the wrong half.
 *
 * NO SUPERSEDED CALLBACK, because nothing can overtake it: it is fetched once
 * on the page-load path, before the first year is drawn, and it answers every
 * year and every budget. A year switch does not refetch it.
 *
 * @param {string} path
 * @returns {Promise<Map<string, FiscRung> | null>}
 */
async function loadRungs(path) {
  let doc;
  try {
    const response = await fetch(path);
    if (!response.ok) {
      fail("Could not load " + path + ": HTTP " + response.status + ". It is what this " +
        "page opens a node with, so nothing has been drawn.");
      return null;
    }
    doc = await response.json();
  } catch (e) {
    // The two sentences loadDocument tells apart, told apart here for the same
    // reason: "serve it over HTTP" is useless advice to someone already doing
    // that, and `e.name` rather than instanceof because an error thrown parsing
    // a response body need not come from this realm's constructor.
    fail(e && e.name === "SyntaxError"
      ? "Could not read " + path + ": the file is not valid JSON, so it is truncated " +
        "or was not the answer this page expected."
      : "Could not load " + path + ". If you opened this file directly, the browser " +
        "blocks the request: serve the directory over HTTP instead, e.g. " +
        "python3 -m http.server -d dist 8000");
    return null;
  }
  if (!isDocument(doc, path)) return null;
  if (doc.schema_version !== RUNGS_SCHEMA) {
    fail("This page will not open anything: " + path + " declares schema_version " +
      doc.schema_version + ", and this page reads schema_version " + RUNGS_SCHEMA +
      ". Opening a node against an answer of another shape would draw a chart that " +
      "is wrong rather than one that fails. Nothing on the page was changed.");
    return null;
  }
  if (!readableRungs(doc, path)) return null;
  const by = new Map();
  for (const column of doc.columns) {
    for (const rung of column.rungs) by.set(rungKey(column.stem, rung.width, rung.path), rung);
  }
  return by;
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
  wireColumns();
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

  // BEFORE THE FIRST DRAW AND AFTER EVERYTHING IS WIRED. Which nodes a column
  // holds is Go's answer now, so a page that cannot read it can draw no rung;
  // fetching it first means a failure is a banner over a page that has drawn
  // NOTHING, rather than a click that dies at a reader who has been looking at
  // a chart. The wiring above still happens either way, for fisc-8cg's reason:
  // an affordance disabled by a failed fetch stays disabled for the visit.
  if (RUNGS_PATH) {
    rungAnswers = await loadRungs(RUNGS_PATH);
    if (!rungAnswers) return;
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
