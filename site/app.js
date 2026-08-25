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
 * @property {string[]} caveats
 */

/**
 * @typedef {Object} FiscDoc
 * @property {string} title
 * @property {string} publisher
 * @property {string} pdf_url
 * @property {string} page_text_base
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
 * @property {FiscFigure} hero
 * @property {FiscFigure[]} figures
 * @property {string[]} caveats
 * @property {{facts:number, nodes:number, links:number}} counts
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
 * The hue a link wears is its fund group's: every link in this projection has
 * exactly one fund-group end, so the ribbon says which fund the money passed
 * through. Colour follows the entity, never the value or the rank.
 * @param {LaidLink} link
 * @returns {string}
 */
function linkColor(link) {
  const fund = isFundGroup(link.source) ? link.source : link.target;
  const name = /** @type {Record<string,string>} */ (FUND_COLOR_VAR)[fund.id];
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
    const at = FUND_ORDER.indexOf(other.id);
    // Every link in this graph has exactly one fund-group end, so this skips
    // nothing today; it is here so that a link that did not would be ignored
    // rather than counted as position zero.
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

/* ------------------------------------------------------------------ *
 * Chart
 * ------------------------------------------------------------------ */

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

function render() {
  if (!projection) return;
  const svg = D3.select("#chart");
  const width = CHART_WIDTH;
  const height = CHART_HEIGHT;

  // No width or height attributes: the viewBox plus width:100% in the
  // stylesheet is what makes the drawing scale with its container.
  svg.attr("viewBox", "0 0 " + width + " " + height);
  svg.selectAll("g").remove();

  const sankey = D3.sankey()
    .nodeId(/** @param {LaidNode} d */ (d) => d.id)
    .nodeWidth(NODE_WIDTH)
    .nodePadding(NODE_PADDING)
    .nodeAlign(D3.sankeyJustify)
    // Supplying this switches d3's own ordering pass off, which is what makes
    // the fund column's colour adjacency a property of the page rather than of
    // the library. nodeRank puts the crossing count back.
    .nodeSort(/** @param {LaidNode} a @param {LaidNode} b */ (a, b) =>
      nodeRank(a) - nodeRank(b) || b.value - a.value)
    .extent([[LABEL_GUTTER, 12], [width - LABEL_GUTTER, height - 12]]);

  // d3-sankey mutates its input, so it gets a copy and the fetched document
  // stays the thing the table and the detail panel read from.
  /** @type {{nodes:LaidNode[], links:LaidLink[]}} */
  const graph = sankey({
    nodes: projection.nodes.map((n) => Object.assign({}, n)),
    links: projection.links.map((l) => Object.assign({}, l, { value: l.value_cents })),
  });
  restackLinks(graph);

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

  const prov = h("div", "prov");
  prov.append(h("span", "subtle", "Sources:"));
  for (const c of citations(projection.metadata.sources)) {
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
  const cites = citations(projection.metadata.sources);

  for (const l of projection.links) {
    const tr = document.createElement("tr");
    tr.append(h("td", "", labels.get(l.source) || l.source));
    tr.append(h("td", "", labels.get(l.target) || l.target));
    tr.append(h("td", "num", fmt(l.value_cents)));
    tr.append(h("td", "", /** @type {Record<string,string>} */ (KIND_LABEL)[l.kind] || l.kind));
    tr.append(h("td", "", l.derived ? "◇ inferred" : "printed"));
    tr.append(h("td", "ids", l.fact_ids.join(" ")));
    const td = h("td", "");
    for (const c of cites) {
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

function wireTheme() {
  const button = /** @type {HTMLButtonElement} */ (el("theme-toggle"));
  const sync = () => {
    const dark = prefersDark();
    button.setAttribute("aria-pressed", String(dark));
    button.textContent = dark ? "Light mode" : "Dark mode";
  };
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
 * Fetches and draws one published year.
 *
 * Everything the page says in WORDS comes from CONFIG.years, which the packager
 * built in Go for every year. This function composes no figure and no caveat of
 * its own: doing so would put the prose in two languages and let a tile disagree
 * with the chart beneath it about the same schedule.
 *
 * @param {FiscYear} year
 * @returns {Promise<boolean>} whether the year was drawn
 */
let switching = 0;

async function showYear(year) {
  // A switch token, because two switches can be in flight at once: a reader who
  // clicks twice gets two fetches, and without this the SLOWER one wins and the
  // page draws a year the control does not show. Compared after every await.
  const token = ++switching;

  let response;
  try {
    response = await fetch(year.path);
  } catch (e) {
    // The overwhelmingly likely cause is file:// — Chrome blocks fetch from a
    // file: origin, so the page loads and the chart never arrives. Say the
    // fix rather than the error.
    fail("Could not load " + year.path + ". If you opened this file directly, the browser " +
      "blocks the request: serve the directory over HTTP instead, e.g. " +
      "python3 -m http.server -d dist 8000");
    return false;
  }
  if (token !== switching) return false;
  if (!response.ok) {
    fail("Could not load " + year.path + ": HTTP " + response.status);
    return false;
  }
  const doc = /** @type {FiscProjection} */ (await response.json());
  if (token !== switching) return false;
  // The fetched file is what actually gets drawn, and it is a separate
  // document from the config: the packager stamps the config from the
  // projection it was handed, so agreeing with the config is not evidence the
  // file on the wire agrees too.
  if (!understands(doc.schema_version, year.path)) return false;

  projection = doc;
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
  render();
  return true;
}

/**
 * Replaces the words that belong to a year: the tiles, the caveats, the lede
 * and the flow count.
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
  if (caveats) caveats.replaceChildren(...year.caveats.map((c) => h("li", "", c)));

  const lede = maybeEl("lede-year");
  if (lede) lede.textContent = year.label + " " + year.basis;

  const counts = maybeEl("counts-line");
  if (counts) {
    counts.textContent = year.counts.links + " flows between " + year.counts.nodes +
      " nodes, from " + year.counts.facts + " facts";
  }

  const title = maybeEl("chart-title");
  if (title) title.textContent = "Sankey diagram of the " + year.label + " " + year.basis + " budget";

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

  document.title = "City of Livermore budget flows — " + year.label;
}

/**
 * Wires the year radio group.
 *
 * The control is rendered server-side and already shows the right year, so this
 * only adds the behaviour. A year that fails to load leaves the radio where the
 * reader put it and shows the refusal: moving it back would claim the page is
 * showing a year it is not.
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
    void showYear(year);
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
  if (!await showYear(years[0])) return;

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
    // marks.
    window.matchMedia("(prefers-color-scheme: dark)").addEventListener("change", paint);
  }
}

main().catch((e) => fail("The chart failed to draw: " + String(e)));
