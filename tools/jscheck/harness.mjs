// harness.mjs — load site/app.js the way a browser would, and hand its
// functions back for checking.
//
// site/app.js is served to readers exactly as it is committed: no bundler, no
// npm, no module system. So the only faithful way to test it is to run that
// same file, with the same vendored d3, and reach into it — not to copy the
// functions into a test file, which would check a copy and let the shipped one
// drift.
//
// Node is the only dependency and it stays OFF THE DEPLOY PATH, the way
// tools/extract.py stays off the build path: `make site` and `fisc export`
// never run this, CI runs it as its own step, and a contributor without node
// can still build and serve the site.
//
// WHAT THE STUBS ARE FOR. app.js ends by calling main(), which fetches and
// draws. That needs a DOM, and a hand-rolled DOM would be a second thing to get
// right — its bugs would read as app.js's. So main() is allowed to run and to
// FAIL, into app.js's own fail() path, and the stubs below are exactly what
// that path touches and nothing more. The functions this harness actually
// checks are pure: they take a graph and return numbers.

import { readFileSync } from "node:fs";
import { createContext, runInContext } from "node:vm";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

export const repoRoot = join(dirname(fileURLToPath(import.meta.url)), "..", "..");

/**
 * Where the page fetches Go's rung answer, read out of internal/export's own
 * constant rather than spelled again here.
 *
 * READ AND NOT COPIED, FOR spineRenderTiers' REASON. The packager writes this
 * string into window.FISC_CONFIG and writes the FILE at the same string; a
 * fixture carrying its own copy would serve the answer at a URL the site does
 * not use and every check over it would pass against a page no reader gets.
 */
export const RUNGS_PATH = (() => {
  const src = readFileSync(join(repoRoot, "internal", "export", "export.go"), "utf8");
  const m = /\nconst RungsPath = "([^"]+)"/.exec(src);
  if (!m) throw new Error("internal/export/export.go declares no RungsPath");
  return m[1];
})();

/**
 * Go's rung answer, from the committed artifact.
 *
 * THE SAME BYTES THE SITE SERVES: pkg/cmd/export's
 * TestTheRungArtifactIsWhatGoComputes pins testdata/rungs.json to what
 * buildAll ships at [RUNGS_PATH], so serving this file to the page is serving
 * the file a reader gets.
 */
export function rungsAnswer() {
  return JSON.parse(readFileSync(join(repoRoot, "testdata", "rungs.json"), "utf8"));
}

/**
 * A DOM stub covering exactly what app.js touches before it gives up.
 *
 * Every method is the smallest thing that keeps the load path from throwing.
 * If app.js starts touching something new, this throws a named error rather
 * than returning undefined and failing somewhere less obvious.
 */
/**
 * The ids site/index.html.tmpl actually renders.
 *
 * The stub answers for these and returns null for anything else, which is the
 * whole point: a stub that fabricates an element for every id cannot model a
 * MISSING one, and "missing" is the condition that took the chart down on a
 * single-year build. It is also a second, cheaper check on the template — an id
 * app.js reaches for and the template never renders shows up here as a null.
 *
 * Kept in step with the template by hand. That is a real cost and it is the
 * smaller one: the alternative is parsing Go templates in JavaScript.
 */
const TEMPLATE_IDS = new Set([
  "breadcrumb", "caveats", "chart-hint", "caveats-count", "caveats-view", "chart", "chart-desc",
  "chart-title", "counts-line", "derived-list", "derived-view", "detail", "figures",
  "figures-view", "flow-table", "hero", "lede-year", "legend", "page-basis",
  "sources-view", "table-view", "theme-toggle", "tooltip", "year-toggle",
  // site/index.html.tmpl: the column control's two
  // steppers and the count between them, which is also its live region.
  "column-fewer", "column-count", "column-more",
]);

/**
 * Attributes the TEMPLATE ships on an element, applied when the stub first
 * hands that element out.
 *
 * MODELLING THE IDS AND NOT THE ATTRIBUTES MADE A CHECK UNFALSIFIABLE. node()
 * starts every element with `attributes: {}`, so seam.mjs's
 * `getAttribute("disabled") === null` was true BEFORE app.js ran -- while its
 * detail string claimed "the template's disabled attribute was removed, which
 * is what wireYears' enhancement is". Deleting
 * `group.removeAttribute("disabled")` from app.js left the whole suite green,
 * and the page it would have shipped has a year toggle no reader can ever
 * operate: index.html.tmpl renders the fieldset `disabled` on purpose, because
 * without app.js the control cannot do anything.
 *
 * Same hand-maintenance cost as TEMPLATE_IDS above, and the same reason for
 * paying it: the alternative is parsing Go templates in JavaScript. Add an
 * entry here when the template starts shipping an attribute a check reads.
 */
const TEMPLATE_ATTRIBUTES = {
  // site/index.html.tmpl: <fieldset id="year-toggle" ... disabled>
  "year-toggle": { disabled: "" },
  // site/index.html.tmpl: <nav id="breadcrumb" ...
  // hidden>. Shipped hidden for the year toggle's reason -- there is no drill
  // to come back from until the reader opens one, and with JavaScript off
  // there never is.
  "breadcrumb": { hidden: "" },
  // site/*.html.tmpl: <button id="column-fewer" ... disabled> and its twin.
  // Shipped disabled for the year group's reason -- the column budget is
  // entirely a client decision -- so without these two entries "wireColumns
  // enables the control" would be true before app.js ran, which is the
  // unfalsifiable shape the year toggle's entry above was added for.
  "column-fewer": { disabled: "" },
  "column-more": { disabled: "" },
};

const XHTML_NS = "http://www.w3.org/1999/xhtml";
const SVG_NS = "http://www.w3.org/2000/svg";

/** DOCUMENT_POSITION_PRECEDING and _FOLLOWING; see node().compareDocumentPosition. */
const DOCUMENT_POSITION_PRECEDING = 2;
const DOCUMENT_POSITION_FOLLOWING = 4;

/**
 * The ids the template renders inside a foreign namespace.
 *
 * ONE ENTRY, AND IT IS THE ONE render() DRAWS INTO. `#chart` is an <svg> in
 * both chart templates, so d3's creatorInherit takes the createElementNS branch
 * off it and every mark below inherits the SVG namespace -- which is the path a
 * browser takes and therefore the one worth modelling. Everything else the
 * template renders is HTML and takes the default.
 *
 * Same hand-maintenance cost and same justification as TEMPLATE_IDS and
 * TEMPLATE_ATTRIBUTES above.
 */
const TEMPLATE_NAMESPACES = {
  // site/index.html.tmpl: <svg class="sankey" id="chart">
  "chart": SVG_NS,
};

/**
 * One simple selector — `tag`, `.class` or `tag.class` — as a predicate, or
 * null for a shape this grammar does not answer.
 *
 * NULL IS THE ANSWER AND NOT A THROW, for the reason seam.mjs gives against
 * making the stub throw on an unanswerable selector at all: fail() and
 * clearRefusal() both call querySelector, so a throw lands inside app.js's own
 * error path and comes back out as a refusal banner. An unanswered shape has to
 * read as an empty result.
 */
function simpleMatcher(part) {
  const m = /^([a-z][\w-]*)?(?:\.([\w-]+))?$/.exec(part);
  if (!m || (!m[1] && !m[2])) return null;
  const tag = m[1];
  const cls = m[2];
  return (/** @type {any} */ n) =>
    (!tag || n.tagName === tag) &&
    (!cls || (typeof n.className === "string" && n.className.split(/\s+/).includes(cls)));
}

/** Every descendant of `root`, in tree order, that the predicate answers for. */
function descendants(root, match) {
  const found = [];
  const walk = (/** @type {any} */ n) => {
    for (const c of n.children || []) {
      if (match(c)) found.push(c);
      walk(c);
    }
  };
  walk(root);
  return found;
}

/**
 * Descendants of `root` the stub's selector grammar matches.
 *
 * ONE SHAPE PER CALL SITE IN app.js AND NOT ONE MORE. This is coverage written
 * down rather than a CSS engine: a selector engine here would be a second
 * implementation of a thing the browser already has, and what these checks are
 * about is whether app.js WRITES to what it finds. Each shape names the calls
 * it exists for, so a shape nothing calls reads as dead grammar and a call
 * nothing answers reads as an empty result rather than as a fabricated element.
 *
 *   `.class`    fail() and clearRefusal() ask their `main` for ".refusal".
 *               While this answered null the banner could be painted twice and
 *               could never be taken down.
 *   `tag`       setIsolated() asks the legend for "button", which is the
 *               legend's copy of the isolation; and render()'s own data joins
 *               go through `svg.selectAll("g")`, `gLinks.selectAll("path")` and
 *               `gNodes.selectAll("g")` before they bind anything.
 *   `tag.class` paint() asks the chart for "path.link", applyEmphasis() for
 *               "path.link" and "g.node", and restoreFocus() asks it for the
 *               "g.node" to put focus on after a drill replaced the chart.
 *   `A B`       paint() asks the chart for "g.node rect", which is the one
 *               descendant pair on the page. Each half is one of the shapes
 *               above.
 *
 * Anything else returns nothing, which is the honest answer for a stub that
 * does not parse CSS.
 */
function matching(root, sel) {
  const parts = String(sel).trim().split(/\s+/);
  if (parts.length > 2) return [];
  const matchers = parts.map(simpleMatcher);
  if (matchers.some((m) => m === null)) return [];
  const first = descendants(root, matchers[0]);
  if (matchers.length === 1) return first;
  const found = [];
  for (const outer of first) {
    for (const inner of descendants(outer, matchers[1])) {
      if (!found.includes(inner)) found.push(inner);
    }
  }
  return found;
}

function domStub(ids = TEMPLATE_IDS, viewport = 0, seed = null) {
  /** Every element the stub hands out, by id, so a check can read one back. */
  const byId = new Map();

  const node = (id, ns) => {
    const self = {
      id: id || "",
      tagName: "",
      // className IS THE `class` ATTRIBUTE AND NOT A SECOND FIELD BESIDE IT,
      // because in a browser they are one thing and d3 only ever writes the
      // attribute. While these were separate, every `.attr("class", ...)`
      // render() makes landed somewhere no selector looked: the two <g>
      // containers were created and `g.links` and `g.nodes` matched neither.
      get className() { return self.attributes.class || ""; },
      set className(v) { self.attributes.class = String(v); },
      dataset: {},
      children: [],
      textContent: "",
      innerHTML: "",
      style: { setProperty() {} },
      classList: { add() {}, remove() {}, toggle() {} },
      attributes: {},
      // ownerDocument AND namespaceURI ARE WHAT MAKE render() RUN AT ALL, and
      // they are the trap a naive `#chart` node walks into: d3's creatorInherit
      // reads both off the element it is appending to, so a stub node that
      // defines neither throws at `svg.append("g")` -- and app.js catches that
      // throw and paints a refusal banner. A check written over that state is
      // green because the gate fired, never because the page drew. chart.mjs
      // refuses it on every state it drives.
      get ownerDocument() { return document; },
      namespaceURI: ns || XHTML_NS,
      setAttribute(name, value) { self.attributes[name] = String(value); },
      getAttribute(name) { return name in self.attributes ? self.attributes[name] : null; },
      removeAttribute(name) { delete self.attributes[name]; },
      addEventListener(type, fn) { (self.listeners[type] ||= []).push(fn); },
      removeEventListener(type, fn) {
        const list = self.listeners[type];
        const i = list ? list.indexOf(fn) : -1;
        if (i >= 0) list.splice(i, 1);
      },
      listeners: {},
      appendChild(c) { c.parent = self; self.children.push(c); return c; },
      append(...c) { for (const n of c) n.parent = self; self.children.push(...c); },
      prepend(c) { c.parent = self; self.children.unshift(c); },
      replaceChildren(...c) { for (const n of c) n.parent = self; self.children = c; },
      // HONOURS ITS SECOND ARGUMENT, AND MOVES RATHER THAN COPIES. d3's data
      // join appends through EnterNode.appendChild, which is
      // `parent.insertBefore(node, this._next)`, and selection.order() then
      // re-inserts a node that is already attached. While this ignored `ref`
      // and unshifted, that second call attached each node a second time:
      // measured on the spine overview, 25 marks answered a "g.node" lookup 49
      // times and 58 ribbons answered "path.link" 115 times.
      insertBefore(c, ref) {
        const was = c.parent ? c.parent.children.indexOf(c) : -1;
        if (was >= 0) c.parent.children.splice(was, 1);
        c.parent = self;
        const i = ref ? self.children.indexOf(ref) : -1;
        if (i >= 0) self.children.splice(i, 0, c);
        else self.children.push(c);
        return c;
      },
      removeChild(c) {
        const i = self.children.indexOf(c);
        if (i >= 0) self.children.splice(i, 1);
        if (c.parent === self) c.parent = null;
        return c;
      },
      // d3's selection.order() calls this on every pair it is asked to keep in
      // sequence, and reads only DOCUMENT_POSITION_FOLLOWING out of it. Answered
      // for siblings, which is the only comparison order() makes.
      compareDocumentPosition(other) {
        if (other === self) return 0;
        if (!other || other.parent !== self.parent || !self.parent) return 0;
        const kids = self.parent.children;
        return kids.indexOf(other) > kids.indexOf(self)
          ? DOCUMENT_POSITION_FOLLOWING : DOCUMENT_POSITION_PRECEDING;
      },
      get parentNode() { return self.parent; },
      // remove() actually detaches, and that is not tidiness. clearRefusal()
      // finds the banner with querySelector and calls remove() on it; while
      // this was a no-op, a check asserting "the banner is gone" could not
      // fail, because the banner was never gone and never there.
      remove() {
        const i = self.parent ? self.parent.children.indexOf(self) : -1;
        if (i >= 0) self.parent.children.splice(i, 1);
      },
      parent: null,
      // focus() RECORDS WHERE FOCUS WENT, on the stub and on
      // document.activeElement, so a check can ask which control restoreFocus
      // chose. While nodes had no focus method the function's `typeof
      // el.focus === "function"` test made every call a no-op, and "focus
      // lands on the innermost rung's control" was not a claim any check could
      // make.
      focus() { focused = self; document.activeElement = self; },
      querySelector: (sel) => {
        const planted = self.selectable && self.selectable[sel];
        if (planted) return Array.isArray(planted) ? planted[0] : planted;
        return matching(self, sel)[0] || null;
      },
      querySelectorAll: (sel) => {
        const planted = self.selectable && self.selectable[sel];
        if (planted) return planted;
        return matching(self, sel);
      },
      // selectable is how a check plants what a selector should find. The stub
      // does not parse CSS -- it answers by exact selector string -- because a
      // selector engine here would be a second implementation of a thing the
      // browser already has, and what these checks are about is whether app.js
      // WRITES to what it finds.
      //
      // FOUR SHAPES ARE ANSWERED FOR REAL, and matching above is where they are
      // enumerated against the call sites that need them. Answering a stated
      // handful of shapes is not a selector engine; it is the difference
      // between a check and a decoration -- while `.refusal` alone was answered,
      // a check asserting a banner's ABSENCE passed whether or not app.js was
      // correct, and while `g.node` was answered by nothing every class,
      // attribute and handler render() writes was written by the page and read
      // by nothing.
      selectable: null,
    };
    return self;
  };

  /** The element focus() was last called on; see node().focus. */
  let focused = null;
  const document = {
    title: "",
    documentElement: node(),
    body: node(),
    // SETTABLE, because focusInChart reads it before a repaint and a check that
    // wants restoreFocus to act has to put focus in the chart first -- exactly
    // what a reader who tabbed to a mark and pressed Enter has done.
    activeElement: null,
    // Ids are REMEMBERED, so a check can ask what the page was told to show.
    // A fresh node per call would make every read return an empty element and
    // every assertion below vacuously true.
    getElementById: (id) => {
      if (!ids.has(id)) return null;
      if (!byId.has(id)) {
        const el = node(id, TEMPLATE_NAMESPACES[id]);
        // The template's own attributes, before app.js sees the element. A
        // check that asserts app.js REMOVED one has nothing to observe
        // otherwise -- see TEMPLATE_ATTRIBUTES.
        for (const [name, value] of Object.entries(TEMPLATE_ATTRIBUTES[id] || {})) {
          el.setAttribute(name, value);
        }
        byId.set(id, el);
      }
      return byId.get(id);
    },
    createElement: (tag) => { const n = node(); n.tagName = tag; return n; },
    // A text node is a node with words and no tag. It was missing, and the gap
    // did not read as one: the throw surfaced through main()'s own .catch as a
    // refusal banner, so a check asserting "a banner appears" PASSED -- on the
    // harness failing, not on app.js reporting. A stub that is incomplete in a
    // way app.js turns into its own error path is worse than one that is
    // obviously incomplete.
    createTextNode: (text) => { const n = node(); n.textContent = String(text); return n; },
    // THE ELEMENT CARRIES THE NAMESPACE IT WAS CREATED WITH, which is what lets
    // creatorInherit walk down from the <svg>: every <g>, <rect>, <text> and
    // <tspan> render() appends inherits the SVG namespace from the element it
    // is appended to, the way a browser gives them.
    createElementNS: (ns, tag) => { const n = node("", ns); n.tagName = tag; return n; },
    // AN `#id` IS ANSWERED OUT OF getElementById, which is the whole of what
    // makes `D3.select("#chart")` resolve to the element the stub already
    // models for the template's id -- with no check having to plant one, and
    // with a page the template did not render still answering null.
    querySelector: (sel) => {
      if (/^#[\w-]+$/.test(sel)) return document.getElementById(sel.slice(1));
      return selectable[sel] ? selectable[sel][0] : null;
    },
    // The stub used to return [] unconditionally, which made every loop over a
    // selector a no-op and every check of one vacuously green. A check that
    // cannot fail is worse than no check: paintYearWords' footer-path loop
    // landed under exactly that and `make js` passed without running it once.
    querySelectorAll: (sel) => selectable[sel] || [],
    // RECORDED, not discarded. main() attaches the page's only global keyboard
    // affordance here, and while this was a no-op nothing could tell an
    // attached Escape handler from a missing one -- which is the whole of
    // fisc-8cg's first defect. A check reads them back through
    // app.dom.documentListeners and can dispatch one.
    addEventListener(type, fn) { (documentListeners[type] ||= []).push(fn); },
  };
  const documentListeners = {};
  // What a selector finds, by exact selector string. Populated by a check with
  // plant(); empty means the page rendered no such element, which is a state
  // worth modelling rather than one to fabricate around.
  const selectable = {};
  document.plant = (sel, ...nodes) => {
    selectable[sel] = nodes;
    return nodes;
  };
  document.node = node;
  const storage = new Map(Object.entries(seed || {}).map(([k, v]) => [k, String(v)]));
  // What window.matchMedia("...").addEventListener was told to follow, BY QUERY
  // and then by type. main() guards on `typeof window.matchMedia === "function"`,
  // so while matchMedia was undefined that branch never ran under any check and
  // "the page follows the OS theme" was unfalsifiable.
  //
  // KEYED BY QUERY BECAUSE THE PAGE NOW FOLLOWS TWO DIFFERENT ONES. While this
  // was one flat list, a width listener and the OS-theme listener were
  // indistinguishable in it, and setOSDark would have called both -- so "the
  // page follows the viewport" would have been green under a page that followed
  // the theme setting instead. The exported reader is followers() below; the map
  // itself is deliberately not exported, so a check still reaching for the old
  // `.change` throws rather than counting zero.
  const mediaListeners = {};
  const OS_DARK = "(prefers-color-scheme: dark)";
  /** What the OS is currently asking for. See matchMedia below and setOSDark. */
  let osDark = false;
  /**
   * How wide the reader's window is, in CSS px.
   *
   * ZERO BY DEFAULT, AND THAT IS THE LOAD-BEARING CHOICE. A harness renders
   * nothing, so it has no viewport, and every (min-width: N) query answers
   * false -- which is what keeps the page at NARROW_COLUMNS and every figure
   * pinned across this directory a figure of the chart it was measured on. A
   * check that wants a wide window passes one and says why.
   */
  let viewportWidth = Number(viewport) || 0;
  /**
   * Whether a query matches, ANSWERED FROM THE QUERY ITSELF.
   *
   * It used to answer osDark to everything it was asked, which is the shape
   * getComputedStyle's comment above names: a stub that answers a question it
   * was never asked does not fail, it stops testing. A width query would have
   * come back "the OS is in light mode" -> false, so a check driving the column
   * control would have measured nothing at all. An unmodelled query THROWS by
   * name rather than guessing.
   */
  const mediaMatches = (query) => {
    if (query === OS_DARK) return osDark;
    const min = /^\(min-width:\s*(\d+)px\)$/.exec(query);
    if (min) return viewportWidth >= Number(min[1]);
    throw new Error(`harness: matchMedia was asked ${JSON.stringify(query)}, ` +
      "which this stub does not model");
  };
  const notify = (query) => {
    const matches = mediaMatches(query);
    for (const fn of ((mediaListeners[query] || {}).change) || []) fn({ matches, media: query });
  };
  const windowListeners = {};
  return {
    byId,
    document,
    documentListeners,
    get focused() { return focused; },
    // Switches the OS theme and notifies whoever is following it, which is what
    // a reader's machine does at sunset. It notifies the OS query ALONE: a
    // width listener has no business firing because the sun went down.
    setOSDark(dark) {
      osDark = Boolean(dark);
      notify(OS_DARK);
    },
    /**
     * Resizes the reader's window and notifies every width query whose ANSWER
     * CHANGED, which is the half of matchMedia that is not `.matches`: a
     * browser fires a query when it crosses its threshold and not when the
     * window merely moves, so a check that wants a listener called has to
     * cross one.
     */
    setViewport(px) {
      const was = {};
      for (const q of Object.keys(mediaListeners)) was[q] = mediaMatches(q);
      viewportWidth = Number(px) || 0;
      for (const q of Object.keys(mediaListeners)) {
        if (q !== OS_DARK && mediaMatches(q) !== was[q]) notify(q);
      }
    },
    get viewportWidth() { return viewportWidth; },
    /** Who is following one query, so a check can count them apart. */
    followers: (query, type = "change") => ((mediaListeners[query] || {})[type] || []),
    mediaMatches,
    windowListeners,
    localStorage: {
      getItem: (k) => (storage.has(k) ? storage.get(k) : null),
      setItem: (k, v) => storage.set(k, String(v)),
      // REMOVAL IS A DISTINCT GESTURE AND NOT setItem(""). app.js clears the
      // column override by removing the key, because a stored "" would read
      // back as a choice this build cannot honour rather than as no choice.
      removeItem: (k) => { storage.delete(k); },
    },
    /** What the page has written, so a check can read a preference back. */
    storage,
    // getComputedStyle ANSWERS FROM THE SHIPPED STYLESHEET, and that is not
    // polish. While it returned "#000000" for every name it was asked, it
    // returned a truthy colour for properties that DO NOT EXIST -- so renaming
    // every entry of FUND_COLOR_VAR to a name appearing nowhere in style.css,
    // which in a browser paints every ribbon and every legend swatch with no
    // colour at all, left the whole suite green. A stub that answers a question
    // it was never asked is the fisc-dn9 shape: it does not fail, it stops
    // testing.
    getComputedStyle: () => ({
      getPropertyValue: (name) => (customProperties.has(name) ? "#000000" : ""),
    }),
    // matchMedia answers PER QUERY, through mediaMatches above. app.js reads
    // .matches for the
    // opening palette and attaches a change listener to follow the OS mid-visit;
    // both are behaviour worth checking, and neither was reachable while this
    // was undefined. prefersDark() consults the saved theme first, so every
    // existing layout check is unaffected by this becoming a function -- which
    // was verified by re-running them, not assumed.
    // matches is a GETTER over a mutable flag, so a check can switch the OS
    // theme mid-visit -- which is the only way to reach the state where
    // prefersDark() changes its answer under a button that has already
    // rendered. While this was the literal `false`, "the page follows the OS
    // theme" could be asserted only as "a listener is attached", and the
    // listener could do the wrong thing in silence.
    matchMedia: (query) => ({
      get matches() { return mediaMatches(query); },
      media: query,
      addEventListener(type, fn) { ((mediaListeners[query] ||= {})[type] ||= []).push(fn); },
      removeEventListener() {},
    }),
    // No fetch by default: main() is meant to give up here, in app.js's own
    // error path. A check that wants a document passes one to loadApp, which
    // installs it BEFORE app.js runs -- assigning one afterwards is too late,
    // because main() is called at file scope and has already reached its fetch.
    //
    // THE RUNG ANSWER IS THE ONE EXCEPTION, and it has to be. main() fetches
    // it BEFORE the first year, and refuses the page when it cannot be read --
    // so a default that rejected it would stop every check in this directory
    // at a banner about rungs.json, and "main() gives up at the document
    // fetch" would become a sentence no check could reach. It is answered from
    // the committed artifact, which is the file the site serves.
    fetch: (path) => (path === RUNGS_PATH
      ? Promise.resolve({ ok: true, status: 200, json: () => Promise.resolve(rungsAnswer()) })
      : Promise.reject(new Error("harness: no network"))),
    requestAnimationFrame: (fn) => fn(),
    addEventListener(type, fn) { (windowListeners[type] ||= []).push(fn); },
  };
}

/**
 * NAMES is what the harness reaches for. Every entry is a top-level `const` or
 * `function` in app.js, which in a script's lexical scope is unreachable from
 * outside — hence the trailing assignment appended below. That appended line is
 * the ONLY difference between what runs here and what a browser runs, and it
 * adds no behaviour: it reads bindings that already exist.
 */
const NAMES = [
  "FUND_ORDER", "nodeRank", "restackLinks", "understands", "isFundGroup",
  "paintYearWords", "wireYears", "showYear", "maybeEl", "SCHEMA_VERSION",
  "NODE_WIDTH", "NODE_PADDING", "CHART_HEIGHT", "LABEL_GUTTER",
  // chartWidth REPLACED THE CHART_WIDTH CONSTANT, and layout.mjs and fold.mjs
  // build their own d3.sankey from it: a chart of four columns is laid out
  // wider, so a harness holding the old constant would measure every crossing
  // and every label clearance against a width the page no longer draws that
  // chart at.
  "chartWidth", "BAND",
  // layOut AND foldDocument ARE EXPORTED BECAUSE layout.mjs REIMPLEMENTED THE
  // FIRST OF THEM. Its layout() builds its own d3.sankey from the constants
  // above, which was fine while the only thing to get wrong was a constant --
  // and it meant every figure that file pins (195 crossings, $457,434,169, the
  // 14 stale pairs) stayed green NO MATTER WHAT layOut DID. layout() still
  // exists, because the alternative sorts it measures cannot be reached through
  // layOut, which hard-codes its nodeSort;
  // what is new is a check that the two agree on the golden graph, so the
  // reimplementation is now pinned to the shipped function rather than trusted
  // to match it.
  "layOut", "foldDocument", "fundGroupOf", "RENDER_TIERS",
  // alignFor IS THE ALIGNER THE PAGE ACTUALLY USES, taken off the page for the
  // reason every constant above it is: layout.mjs builds its own d3.sankey,
  // and a hard-coded nodeAlign there would measure a chart the page does not
  // draw the moment a view declares a column order.
  "alignFor",
  // THE DRILL, WHICH SHIPPED WITH NO CHECK TOUCHING IT AT ALL; drill.mjs's
  // header says how that happened and what it cost. drill.mjs drives drillDown
  // and drillUp, which are the real entry points -- what a click, a breadcrumb
  // control and Escape call -- and shapeFor; the rest are here so a check can
  // measure one stage without the repaint. STEPS is the tree as app.js read it
  // off the config, and stepFor is its one reader.
  "shapeFor", "filterLinks", "heldBy", "heldFor", "answeredRung", "capColumn", "drillable", "drillDown", "drillUp",
  // THE COLUMN BUDGET AND THE SET IT TRIMS. activeTiers is what every column
  // reader on the page goes through, and a check that spelled a step's tiers
  // itself would measure the widened window under the narrow budget the page
  // ships and call it wide. setColumnBudget is the seam the column control
  // calls, which is why a check drives the widening through it rather than by
  // editing a step.
  "activeTiers", "drawnColumns", "setColumnBudget",
  // AND THE TWO CONSTANTS THE CONTROL IS BOUNDED BY. WIDE_COLUMNS is the
  // stylesheet's cap restated in the script, and layout.mjs compares the two
  // rather than spelling either; COLUMN_QUERIES is the responsive rule, whose
  // thresholds layout.mjs re-derives from that same cap's cushion.
  "WIDE_COLUMNS", "COLUMN_QUERIES", "NARROW_COLUMNS",
  "STEPS", "stepFor", "aggregateID", "isAggregate", "residualID", "isResidual",
  "isCarried", "carryResidual", "withinNode", "docAt", "drawnDoc",
  // THE GAP, which is the other mark a rung can stand beside an opened node:
  // markGap is reached directly for the refusals, which a click cannot produce
  // while the committed corpus ties, and isGap is what tells it from a residual.
  "markGap", "gapID", "isGap",
  "loadDocument", "labelOfRung", "openableColumns", "joinOr", "linkClass", "markContra",
  // THE TRAIL, NOT ONLY ONE RUNG'S WORDS. labelOfRung answers for a rung alone
  // and cannot see a sibling it reads the same as, so the qualifying rule is
  // its own function and is reached here rather than re-spelled.
  "trailOfRungs",
  "caveatsFor", "columnShare", "caveatHref", "showTip", "pin",
  // THE LABEL RULE AND THE WORDS IT PLACES. layout.mjs measures whether a label
  // has room where it was anchored, which needs the rule, the column it keys on
  // and the TEXT -- a box measured from a label this file spelled itself would
  // be a box the page never draws.
  "columnOf", "labelPlacement", "markCents", "fmtShortSigned",
  // AND THE QUALIFIER, which is the second thing a mark's words can come from.
  // A label check that measured d.label alone would measure the document rather
  // than the drawing, and it is the drawing that repeats a word: labelQualifiers
  // is what decides a mark gets a second line and labelLineShift is where that
  // line goes.
  "labelQualifiers", "labelLineShift",
  // THE WINDOW. drill.mjs drives it through drillDown like everything else
  // here; windowFor is reached directly for the refusals, which have no route
  // through a click because export.validateSteps refuses them first. The rest
  // is the cross-tab vocabulary a partition ribbon carries, in the four places
  // one mark can carry it.
  "windowFor", "depthOfDocument", "carriedSource", "isPartitionNode", "PARTITION_NOTE",
  "linkDescription", "nodeDescription",
  "paintBreadcrumb",
  // paint IS EXPORTED SO ITS LEGEND LOOP CAN BE REACHED AT ALL. It queries
  // "#legend button .key", and the swatches that selector finds do not exist
  // until buildLegend has run -- so a check cannot plant them before the draw
  // and cannot plant them mid-draw either. Calling paint() after the draw, with
  // the swatches buildLegend actually created planted, is the only order in
  // which that loop executes. render() still calls it; this adds no behaviour.
  "paint",
  // THE GESTURES, REACHED BY NAME BECAUSE render() CANNOT BE. The stub answers
  // no "#chart" selector, so d3 lays render()'s selections over a null node:
  // no <g> is created, no attribute is written and no handler is registered.
  // A click or keydown closure written inline there would execute in no check
  // however many checks this directory grows, which is why the three gestures
  // are functions in app.js rather than closures. nodeClass and nodeFlags are
  // here for the same reason one layer over: the class and the marker a node
  // is drawn with are unreadable off a chart that draws nothing.
  "clickNode", "doubleClickNode", "keyNode", "ACTIVATION_WINDOW",
  "nodeClass", "nodeFlags",
  // THE EXPANSION, WHICH IS A REDRAW AND NOT A RUNG. expandTier and
  // collapseTier are what the gesture and the breadcrumb chip call; a check
  // drives them directly for the same reason it drives the gestures by name,
  // and reaches the chip itself through the bar paintBreadcrumb fills.
  "expandable", "expandTier", "collapseTier",
];

// main IS DELIBERATELY NOT IN NAMES. It is invoked at file scope, so by the time
// the appended line runs it is already in flight; exporting it would hand back
// the FUNCTION, and calling that boots a second page -- a second theme listener,
// a second change listener, and a ++switching that supersedes the first opening
// fetch. A check that wants to observe the opening showYear resolves its own
// injected fetch and drains microtasks instead. See settle().
//
// `switching` is likewise absent and must stay so: it is a `let`, and the
// appended line would snapshot its VALUE at load time rather than expose the
// live token. A check observes the token's effect, never the variable.

/**
 * Loads the vendored d3 bundles and site/app.js into one context.
 *
 * opts.ids is the set of element ids the page is to be treated as having
 * rendered; it defaults to every id the template emits. Passing a smaller set is
 * how a check models a page the template rendered conditionally — a single-year
 * build has no year toggle. A bare Set is accepted as shorthand for it.
 *
 * opts.config replaces FISC_CONFIG, which is how a check gets a TWO-YEAR page:
 * wireYears returns early on fewer than two years, so under the default config
 * there is no toggle, no change listener, and nothing a check about switching
 * years can drive.
 *
 * opts.fetch replaces the failing default, which is how a check drives the
 * document path at all — a successful year, a rejected one, a malformed body.
 * Both are installed before app.js runs; see below for why that is the only
 * moment either can be installed.
 *
 * opts.viewport is how wide the reader's window is, which decides what every
 * (min-width: N) query answers; 0 -- no viewport at all -- is the default, and
 * is what keeps the column budget at its floor for every check that does not
 * ask otherwise.
 *
 * opts.storage seeds localStorage before app.js reads it, which is the only
 * moment a saved preference can be there: wireColumns and wireTheme both read
 * theirs during main(), at file scope.
 * @param {Set<string>|{ids?: Set<string>, config?: object, fetch?: Function,
 *   checkedStem?: string, viewport?: number, storage?: object}} [opts]
 */
/**
 * Every CSS custom property site/style.css defines, so the stub can tell a name
 * the stylesheet carries from one it does not.
 *
 * A regex over the shipped file rather than a maintained list, for the reason
 * selectorsIn scans app.js: a list beside the file is the thing that goes stale.
 * It over-collects slightly -- a `--name:` inside a comment would count -- which
 * is the safe direction, since the failure this guards is a name that exists
 * NOWHERE.
 */
const customProperties = new Set(
  [...readFileSync(join(repoRoot, "site", "style.css"), "utf8")
    .matchAll(/(--[\w-]+)\s*:/g)].map((m) => m[1]),
);

/**
 * The stylesheet the site ships, as text.
 *
 * READ AS TEXT AND NOT AS CSS, and every caller has to keep that in mind:
 * nothing in this tree renders or parses a stylesheet (fisc-6at), so a check
 * over this string can say what style.css DECLARES and never what a browser
 * laid out. That is still worth having where the declaration is a number the
 * page's own geometry has to agree with -- the chart's width allowance is one,
 * and it is the only figure in the file that app.js can contradict.
 */
export function stylesheet() {
  return readFileSync(join(repoRoot, "site", "style.css"), "utf8");
}

export function loadApp(opts = {}) {
  // A bare Set is the old signature and still means "the ids the page
  // rendered". Kept because that is what most checks want and an options
  // object for one field reads worse at every call site.
  const o = opts instanceof Set ? { ids: opts } : opts;
  const stub = domStub(o.ids, o.viewport, o.storage);
  const sandbox = { console, Intl, ...stub };
  sandbox.window = sandbox;
  sandbox.globalThis = sandbox;
  sandbox.self = sandbox;
  const ctx = createContext(sandbox);

  for (const f of ["d3.min.js", "d3-sankey.min.js"]) {
    runInContext(readFileSync(join(repoRoot, "site", "vendor", f), "utf8"), ctx, { filename: f });
  }
  if (!sandbox.d3 || typeof sandbox.d3.sankey !== "function") {
    throw new Error("vendored d3 did not define d3.sankey");
  }

  // A config the page can accept, so main() gets past its schema gate AND past
  // its empty-years guard, and dies at the fetch instead — as deep into the real
  // code path as a harness with no network can go.
  //
  // The years matter. A config without them made main() stop at the guard, so
  // wireYears never ran under any check, and a dead `if (!group)` in it took the
  // whole chart down on a single-year build with every check green. A stub that
  // stops early does not fail; it stops testing.
  sandbox.FISC_CONFIG = {
    schema_version: 1,
    primary: "sankey",
    projections: { sankey: "data/sankey.json" },
    years: [{
      year: 2026, label: "FY 2025-26", stem: "sankey", path: "data/sankey.json",
      basis: "adopted",
      hero: { label: "l", value: "v", note: "n", kind: "hero" },
      figures: [{ label: "l", value: "v", note: "n", kind: "" }],
      caveats: [{ id: "c", summary: "s", href: "caveats.html#caveat-x--c" }],
      counts: { facts: 1, nodes: 1, links: 1 },
    }],
    docs: {},
    // THE RUNG ANSWER'S PATH, ON A CONFIG THAT DECLARES NO STEPS, and that is
    // the app.js condition rather than the packager's. main() fetches this
    // whenever the key is present and refuses the page when it cannot be read,
    // so leaving it out here would mean the fetch, its schema gate and its
    // shape gate ran under no check that does not build a drill config of its
    // own. Which PAGES get the key is internal/export.rungsFor's rule and the
    // Go test beside it holds that end.
    rungs: RUNGS_PATH,
  };

  // BOTH SEAMS ARE APPLIED BEFORE app.js RUNS, and that is not a style choice.
  // app.js ends in `main().catch(...)` at file scope, so the moment
  // runInContext returns, main has already read FISC_CONFIG, wired the toggle
  // and reached its fetch. Anything installed afterwards is installed after the
  // code that would have used it.
  if (o.config) sandbox.FISC_CONFIG = o.config;
  if (o.fetch) sandbox.fetch = o.fetch;

  // THE YEAR RADIOS THE TEMPLATE RENDERS, planted before app.js runs.
  //
  // index.html.tmpl emits one <input type="radio" value="{{$y.Stem}}"> per
  // published year inside the fieldset, `checked` on the LAST -- the newest,
  // while the list itself runs oldest first. Mirroring that is what lets a check
  // tell the reader's restored selection apart from the page's own default. The
  // stub knew
  // the fieldset existed and nothing about its contents, so "the page opens on
  // the year the control is showing" could not be asked at all -- and the
  // browser's own form-state restoration, which is what makes that question
  // matter, had nothing to act on. checkedStem overrides which one is checked,
  // which is exactly what a soft reload or a Back navigation does.
  //
  // Same hand-maintenance cost and same justification as TEMPLATE_IDS and
  // TEMPLATE_ATTRIBUTES: this mirrors the template by hand because the
  // alternative is parsing Go templates in JavaScript.
  const group = (o.ids || TEMPLATE_IDS).has("year-toggle")
    ? stub.document.getElementById("year-toggle") : null;
  if (group) {
    const years = sandbox.FISC_CONFIG.years || [];
    for (const y of years) {
      const input = stub.document.createElement("input");
      input.setAttribute("type", "radio");
      input.id = "year-" + y.stem;
      input.value = y.stem;
      // THE TEMPLATE'S OWN RULE: checked is the newest year, which is the last
      // of a list the packager orders oldest first.
      input.checked = o.checkedStem
        ? y.stem === o.checkedStem
        : y === years[years.length - 1];
      group.appendChild(input);
      const label = stub.document.createElement("label");
      label.setAttribute("for", input.id);
      label.textContent = y.label;
      group.appendChild(label);
    }
  }

  const src = readFileSync(join(repoRoot, "site", "app.js"), "utf8");
  // projection, drilled AND fetched ARE `let` BINDINGS, and a check has to be
  // able to ask what is on SCREEN rather than what a function returned. They are
  // not in NAMES above for that reason -- a name in that list is copied into an
  // object literal, which captures the value at load time. They are exported
  // through getters, because assigning the binding into an object literal
  // captures the value at load time -- which for all of them is the empty
  // state, so every check reading them would have been reading a constant. That
  // is the shape this directory exists to refuse.
  //
  // drilled IS THE STACK ITSELF, outermost rung first: a check reads the depth
  // off its length and the opened node off its last rung's id, and tells the
  // year's document from a rung's by comparing `doc` against `fetched`.
  const exported = `\n;globalThis.__harness = { ${NAMES.join(", ")},` +
    ` get projection() { return projection; },` +
    ` get drilled() { return drilled; },` +
    ` get fetched() { return fetched; },` +
    // columnBudget AND columnOverride ARE `let` BINDINGS TOO, and the second is
    // the one a check cannot infer: a budget of 3 at a narrow viewport looks
    // identical whether the reader chose it or nobody did, and the difference
    // is the whole of whether the next media change moves the page.
    ` get columnBudget() { return columnBudget; },` +
    // isolated IS A `let` TOO, and it is the whole subject of the gesture
    // split: which node's money the chart is following is not derivable from
    // the stack, from the projection or from any DOM the stub can see.
    ` get isolated() { return isolated; },` +
    ` get columnOverride() { return columnOverride; } };\n`;
  runInContext(src + exported, ctx, { filename: "app.js" });

  const app = sandbox.__harness;
  for (const n of NAMES) {
    if (app[n] === undefined) throw new Error(`app.js no longer defines ${n}`);
  }
  // The getters answer undefined only if the binding vanished; null and "" are
  // their legitimate empty states, so they are checked for presence separately.
  for (const n of ["projection", "drilled", "fetched", "isolated"]) {
    if (!(n in app)) throw new Error(`app.js no longer defines ${n}`);
  }
  app.d3 = sandbox.d3;
  // The file's own text, so a check can pin the figures app.js QUOTES against
  // the figures it PRODUCES. Without this the two drift apart silently: editing
  // a comment to say 200 crossings would leave every check green.
  app.source = src;
  // The stub itself, so a check can read back what the page was told to show.
  app.dom = stub;
  return app;
}

/**
 * Every selector site/app.js passes to querySelector/querySelectorAll, and how
 * this stub answers it.
 *
 * THIS LIST IS THE STUB'S COVERAGE, WRITTEN DOWN. The stub does not parse CSS —
 * see the rationale on `selectable` below — so its reach is not something a
 * reader can infer from the code: it is the union of one parsed shape and
 * whatever the checks happen to plant. While that union was implicit, two of
 * app.js's selectors were answered by nothing and the code behind them ran in
 * no check, silently. `buildTable()` returned at its `if (!body) return` guard
 * every time, and `paint()`'s legend loop iterated an empty list. Neither read
 * as a gap; both read as green checks (fisc-wcy).
 *
 * seam.mjs asserts this map's keys are EXACTLY the selector literals in
 * app.js — so a selector added to the page that nothing here answers fails
 * `make js` by name, and a selector removed from the page leaves a stale entry
 * that fails the same way. `unanswered` is a real and declared state: it says
 * the code behind that selector runs in no check, which is worth printing
 * rather than discovering later.
 */
export const KNOWN_SELECTORS = {
  ".refusal": {
    how: "parsed",
    note: "a bare class against the element's own descendants (matching); used by fail() and clearRefusal()",
  },
  "main": {
    how: "planted",
    note: "lifecycle.mjs and seam.mjs plant a <main> so fail()'s banner is reachable",
  },
  "[data-year-path]": {
    how: "planted",
    note: "year.mjs plants the footer's citation wrapper",
  },
  "a": {
    how: "planted",
    note: "year.mjs sets selectable.a on that wrapper",
  },
  "tbody": {
    how: "planted",
    note: "year.mjs sets selectable.tbody on #flow-table; the stub's flow-table node has no children, so no grammar could find one",
  },
  "g.node": {
    how: "parsed",
    note: "the tag.class shape against the element's own descendants (matching), over the <g> marks render() really appends now that the stub answers #chart. It is what restoreFocus finds to put focus on after a drill has replaced the chart, and what applyEmphasis reaches to write aria-pressed on; tools/jscheck/chart.mjs reads both off the drawn marks",
  },
  "#legend button .key": {
    how: "planted",
    note: "year.mjs plants the swatches buildLegend created, then calls paint()",
  },
  "button": {
    how: "parsed",
    note: "the bare-tag shape against the element's own descendants (matching); setIsolated asks the legend for the buttons buildLegend created, which are real children of the stub's #legend node and carry the tagName createElement gave them",
  },
};

/**
 * The selector literals site/app.js actually passes, read out of the file.
 *
 * Deliberately a scan of the SHIPPED source rather than a list maintained beside
 * it: a list would be the thing that goes stale, which is the failure this
 * whole file exists to catch one layer down.
 */
export function selectorsIn(source) {
  const found = new Set();
  // ALL THREE QUOTING FORMS, because this check's whole purpose is that a
  // selector added to the page which nothing answers fails `make js` by name --
  // and a regex matching only double quotes cannot see one written with single
  // quotes or as a template literal. Measured: rewriting a call as
  // el('legend').querySelectorAll('button.fund') left the new, unanswerable
  // selector entirely invisible. Nothing in this repo lints JS quote style, so
  // the scan cannot assume one.
  const re = /querySelector(?:All)?\(\s*(["'`])((?:[^\\]|\\.)*?)\1/g;
  let m;
  while ((m = re.exec(source)) !== null) found.add(m[2]);
  // A selector built by concatenation or interpolation is not a literal and
  // cannot be scanned. Report it as one unanswerable entry rather than passing
  // over it in silence.
  const dynamic = /querySelector(?:All)?\(\s*(?!["'`])/g;
  if (dynamic.test(source)) found.add("(a computed selector this scan cannot read)");
  return found;
}

/**
 * The committed drill-down document, FY2025-26 -- 238 nodes, 251 links.
 *
 * SEPARATE FROM goldenGraph RATHER THAN A PARAMETER ON IT, so that every
 * existing caller keeps meaning what it meant. It is a capture of what `fisc
 * export` writes, pinned to that by a Go test; testdata/README.md says why the
 * two fund-flows fixtures are the only ones in the tree not derived by hand.
 */
export function goldenFundFlows() {
  return JSON.parse(readFileSync(join(repoRoot, "testdata", "fund-flows.golden.json"), "utf8"));
}

/**
 * The other column the merged page reaches: FY2026-27's drill-down, 237
 * nodes, 249 links, captured and pinned the way goldenFundFlows' is.
 *
 * A SECOND LOADER RATHER THAN A YEAR PARAMETER, for goldenFundFlows' own
 * reason: a check that names the year it measures cannot be handed the other
 * one by a default. The two documents are the same 280 facts read down a
 * different printed column, and they differ in shape -- fund/207 prints a
 * dash in this column and is not a node here -- which is why every drill pin
 * is taken over both rather than one standing in for the other.
 */
export function goldenFundFlows2027() {
  return JSON.parse(readFileSync(join(repoRoot, "testdata", "fund-flows-2027.golden.json"), "utf8"));
}

export function goldenGraph() {
  return JSON.parse(readFileSync(join(repoRoot, "testdata", "sankey.golden.json"), "utf8"));
}

/**
 * The second spine column, FY2026-27's: a capture pinned to `fisc export` by
 * TestTheSankey2027FixtureIsTheDocumentTheSiteDraws, the way goldenFundFlows2027
 * is.
 *
 * IT EXISTS FOR THE RESIDUAL. The drill read nothing off the spine but the
 * clicked node's id and label, so one spine golden served both years' paths.
 * The residual node copies the spine's own links onto the opened group, and
 * the two columns differ exactly where the declared set says they do --
 * FY2026-27 general's change in working capital is a contribution out where
 * FY2025-26's is a draw in -- so a check over FY2025-26's spine twice would
 * never see fund-balance/contribution carried at all.
 */
export function goldenGraph2027() {
  return JSON.parse(readFileSync(join(repoRoot, "testdata", "sankey-2027.golden.json"), "utf8"));
}

/**
 * The residual set as internal/check/residual.go declares it: spine
 * endpoint id to the reason a fund-level schedule cannot decompose it.
 *
 * READ OFF THE GO SOURCE RATHER THAN SPELLED HERE. The set has one
 * declaration, in the check that proves the identity it closes; the packager
 * ships it to the client as steps[0].residual, and a literal here would be the
 * second copy that declaration exists to prevent -- a fixture driving the
 * client under a set the site may no longer ship, kept green by nothing.
 * There is no seam from Go to node, so this is a parse of the map literal:
 * a key, then one or more concatenated interpreted string literals, then a
 * comma. It throws on a shape it does not recognise rather than return a
 * partial set, because a partial set would have every residual check measure
 * a client the site does not run.
 */
export function residualDeclaration() {
  return parseResidualLiteral(
    readFileSync(join(repoRoot, "internal", "check", "residual.go"), "utf8"));
}

/**
 * Budget Book pp.85-125's upper block as `fisc export` writes it, one loader
 * per published spine column.
 *
 * TWO LOADERS AND NOT A YEAR PARAMETER, for goldenFundFlows2027's reason. The
 * object-category window splices a spine year onto the cross-tab column of the
 * same year, and the two columns differ in shape and in what they reconcile to:
 * capital-outlay reaches five divisions in FY2025-26 and four in FY2026-27, and
 * only FY2026-27 carries the declared 250,000 gap against p0067. A check served
 * one capture under both paths would pin one year twice and leave the other
 * with nothing able to see it go wrong.
 */
export function goldenSpending() {
  return JSON.parse(
    readFileSync(join(repoRoot, "testdata", "department-spending.golden.json"), "utf8"));
}

export function goldenSpending2027() {
  return JSON.parse(
    readFileSync(join(repoRoot, "testdata", "department-spending-2027.golden.json"), "utf8"));
}

/**
 * The node ids at a step's `from` that `doc` decomposes: what the packager
 * ships as a year's `steps[].opens` (export.openableNodes), read off the same
 * document rather than copied from a list.
 *
 * THE HARNESS BUILDS THE YEAR CONFIG AND SO IT HAS TO BUILD THIS TOO. Every
 * other per-year field here is composed the way the packager composes it --
 * the counts line, the chart title, the caveat refs -- and this is the same
 * kind of thing one step further in. What makes it safe is that it is a
 * DERIVATION over the committed capture and not a spelling of a set: the
 * goldens are pinned against `fisc export` by Go tests, so the input is the
 * site's, and drill.mjs's walk opens every id this returns.
 *
 * TWO THINGS NARROW IT AND THEY ARE NOT THE SAME THING. The far end must land
 * in a column BEYOND the centre -- not merely in a tier the step draws, which a
 * fund's ribbon from its own group satisfies and which would declare all sixty
 * funds openable. And the ribbon must run in the direction windowFor will draw
 * it: that call asks the step document for the half AWAY from the kept flank,
 * so a ribbon pointing INTO the opened node from beyond it is not a chart.
 *
 * THE SECOND OF THOSE IS LATENT ON THE COMMITTED CORPUS, measured rather than
 * assumed: every published document runs its ribbons coarse-to-fine across each
 * window's centre, so no shipped step can tell a direction-aware reading from a
 * direction-blind one. It is checked in seam.mjs over a document that can.
 *
 * @param {{nodes: {id: string, tier: number}[], links: {source: string, target: string}[]}} doc
 * @param {{from: number, tiers: number[], keep?: number[]}} step
 * @returns {string[] | null} null for a step that keeps no flank and declares no set
 */
export function openableFrom(doc, step) {
  const keep = step.keep || [];
  if (!keep.length) return null;
  const tiers = step.tiers;
  const m = keep.length;
  const n = tiers.length;
  const matches = (/** @type {number[]} */ want) => want.every((t, k) => t === keep[k]);
  const left = matches(tiers.slice(0, m).reverse());
  const right = !left && matches(tiers.slice(n - m));
  if (!left && !right) {
    throw new Error(`a step keeping tier(s) ${keep.join(", ")} and drawing ` +
      `${tiers.join(", ")} has neither end as that flank, so which half it opens ` +
      `a node into cannot be read`);
  }
  const outward = left ? tiers.slice(m + 1) : tiers.slice(0, n - 1 - m);
  const tier = new Map(doc.nodes.map((d) => [d.id, d.tier]));
  const out = new Set();
  for (const l of doc.links) {
    const near = left ? l.source : l.target;
    const far = left ? l.target : l.source;
    if (tier.get(near) !== step.from) continue;
    if (!outward.includes(/** @type {number} */ (tier.get(far)))) continue;
    out.add(near);
  }
  if (!out.size) {
    throw new Error(`a step opening tier ${step.from} draws no ribbon into tier(s) ` +
      `${outward.join(", ")}, so the rung is one no reader could ever reach`);
  }
  return [...out].sort();
}

/**
 * Budget Book pp.85-125's LOWER block as `fisc export` writes it, one loader
 * per published spine column.
 *
 * TWO LOADERS AND NOT A YEAR PARAMETER, for goldenSpending2027's reason and a
 * sharper case than that one's: these two columns draw different FUNDS.
 * Measured off the captures -- 58 funds reach a department in FY2025-26 and 55
 * in FY2026-27, out of the 63 the schedule names in every column -- so a check
 * served one capture under both paths could not see a fund stop being
 * decomposed, which is exactly the state the fund-departments step's
 * openability set exists to keep a reader out of.
 */
export function goldenFunding() {
  return JSON.parse(
    readFileSync(join(repoRoot, "testdata", "department-funding.golden.json"), "utf8"));
}

export function goldenFunding2027() {
  return JSON.parse(
    readFileSync(join(repoRoot, "testdata", "department-funding-2027.golden.json"), "utf8"));
}

/**
 * Budget Book p76 as `fisc export` writes it, one loader per published spine
 * column.
 *
 * TWO LOADERS AND NOT A YEAR PARAMETER, for goldenSpending2027's reason: p76
 * prints different figures in its two budget columns, so each document ties to
 * its own printed grand total and a check served one capture under both paths
 * could not see a year join to the wrong document.
 *
 * THIS IS THE ONE FIXTURE WHOSE RIBBONS SUM TO TWICE ITS SCHEDULE, and it is
 * not a defect in the capture. The page names both ends of every movement, so
 * the document draws a receiving leg AND a paying leg for each printed figure,
 * carrying one transfer_id between them. The transfers step draws the receiving
 * half; a check that summed every link here and compared it with p76 would be
 * out by a factor of two by construction.
 */
export function goldenTransfers() {
  return JSON.parse(
    readFileSync(join(repoRoot, "testdata", "transfers-by-fund.golden.json"), "utf8"));
}

export function goldenTransfers2027() {
  return JSON.parse(
    readFileSync(join(repoRoot, "testdata", "transfers-by-fund-2027.golden.json"), "utf8"));
}

/**
 * The gap set as internal/structure/cuts.go declares it: the tier-5 node id
 * an object category is drawn at, and the reason the two schedules print that
 * cell at two figures. It is the exceptions the departmentwide cut declares
 * against the spine, which is what check.SpendingGaps derives from.
 *
 * READ OFF THE GO SOURCE FOR residualDeclaration's REASON. The site ships
 * check.SpendingGaps(), which is this declaration keyed by node id; a literal
 * here would be a second copy, and the object-category window would be
 * measured under a set the site may no longer declare.
 *
 * THE KEYS ARE THE DECLARATION AND THE SENTENCE IS NOT. SpendingGaps composes
 * each reason with the column, both pinned figures and the bead in front of
 * the text parsed here, and internal/check's own
 * TestSpendingGapsIsTheSameDeclarationTheCheckReads is what holds those figures
 * to the declaration. What this side needs is WHICH nodes carry a gap, and a
 * reason the client can be seen to put on the mark verbatim -- composing the
 * prefix here would be the second spelling the parse exists to avoid.
 */
export function spendingGapDeclaration() {
  return parseSpendingGaps(
    readFileSync(join(repoRoot, "internal", "structure", "cuts.go"), "utf8"));
}

/**
 * The parse behind [spendingGapDeclaration], over source text so seam.mjs can
 * drive it over literals the file does not contain.
 *
 * BudgetBookExceptions is a slice literal of Exception entries, one `{ ... },`
 * block at two tabs each; the ones this reads are those whose Cut is the
 * departmentwide cut. Counted twice, by categories pinned and by node ids
 * returned, so an entry in a shape the pattern cannot follow throws rather
 * than leaving the window measured under a shorter set. The node id is
 * composed from the category the way internal/check's spendingCategoryNode
 * does, which is the one thing about the shipped map this cannot read out of
 * a literal.
 * @param {string} src
 * @returns {Record<string, string>}
 */
export function parseSpendingGaps(src) {
  const fn = src.match(/func BudgetBookExceptions\(\) \[\]Exception \{\n([\s\S]*?)\n\}\n/);
  if (!fn) {
    throw new Error("internal/structure/cuts.go declares no BudgetBookExceptions literal to read");
  }
  const entries = fn[1].split(/\n\t\t\{\n/).slice(1).map((e) => e.split(/\n\t\t\},/)[0]);
  const out = {};
  const categories = new Set();
  let read = 0;
  for (const e of entries) {
    const cut = e.match(/^\t\t\tCut:\s+"((?:[^"\\]|\\.)*)"/m);
    if (!cut) throw new Error("an exception entry names no Cut the pattern can follow");
    if (JSON.parse(`"${cut[1]}"`) !== "departmentwide") continue;
    read++;
    const reason = e.match(/^\t\t\tReason:\s*((?:"(?:[^"\\]|\\.)*"\s*\+?\s*)+),/m);
    if (!reason) {
      throw new Error("a departmentwide exception carries no Reason the pattern can follow");
    }
    const text = (reason[1].match(/"(?:[^"\\]|\\.)*"/g) || []).map((p) => JSON.parse(p)).join("");
    const cells = e.match(/AxisCategory: "(?:[^"\\]|\\.)*"/g) || [];
    if (cells.length === 0) throw new Error("a departmentwide exception pins no category");
    for (const c of cells) {
      const category = JSON.parse(c.slice("AxisCategory: ".length));
      categories.add(category);
      const id = "expenditure/" + category;
      out[id] = id in out ? out[id] + " " + text : text;
    }
  }
  const got = Object.keys(out).length;
  if (read === 0 || got === 0 || got !== categories.size) {
    throw new Error(`parsed ${got} gap entries from ${read} departmentwide exception(s) pinning ` +
      `${categories.size} categories`);
  }
  for (const [id, reason] of Object.entries(out)) {
    if (!reason) throw new Error(`gap node ${id} parsed with an empty reason`);
  }
  return out;
}

/**
 * The parse behind residualDeclaration, over source text rather than the file,
 * so seam.mjs can hand it a literal the file does not contain.
 *
 * THE KEY COUNT DOES NOT PRESUPPOSE THE QUOTING, and that is the whole point of
 * counting twice. Counting `^\t"` would be the entry pattern's own assumption
 * spelled a second way: a key neither pattern follows would be skipped by both,
 * got would equal keys, and the partial set would be returned in silence -- a
 * guard vacuous for exactly the shape it exists for. An entry begins at one tab
 * and a non-space; continuations are indented further. fisc-0flg.
 */
export function parseResidualLiteral(src) {
  const m = src.match(/var residualNodes = map\[string\]string\{\n([\s\S]*?)\n\}\n/);
  if (!m) throw new Error("internal/check/residual.go declares no residualNodes map literal to read");
  const out = {};
  const entry = /"((?:[^"\\]|\\.)*)":\s*((?:"(?:[^"\\]|\\.)*"\s*\+?\s*)+),/g;
  let e;
  while ((e = entry.exec(m[1])) !== null) {
    const pieces = e[2].match(/"(?:[^"\\]|\\.)*"/g) || [];
    out[JSON.parse(`"${e[1]}"`)] = pieces.map((p) => JSON.parse(p)).join("");
  }
  // Every entry accounted for, counted by the literal's own entry lines, so a
  // key or a reason in a shape the entry pattern cannot follow is a throw and
  // not a missing endpoint.
  const keys = (m[1].match(/^\t(?!\/\/)\S/gm) || []).length;
  const got = Object.keys(out).length;
  if (got === 0 || got !== keys) {
    throw new Error(`parsed ${got} residual entries from a literal with ${keys} keys`);
  }
  for (const [id, reason] of Object.entries(out)) {
    if (!reason) throw new Error(`residual endpoint ${id} parsed with an empty reason`);
  }
  return out;
}

/**
 * The step shape the packager declares, in declaration order, read out of
 * pkg/cmd/export/data.go: `from`, `tiers` and `caps` for each step.
 *
 * THE SAME ARGUMENT AS [stepDescriptions], ONE FIELD SET OVER. drill.mjs held
 * these as a hand-kept literal and nothing compared it to views() -- the Go
 * test pins views() against a literal in the Go test file and reads nothing in
 * this directory. Measured: changing `{Tier: 3, Cap: 8}` to `Cap: 9` in
 * data.go AND in that test literal left `go test` and `make js` green, with
 * jscheck still measuring special-revenue's fold under a cap of 8 while the
 * site would ship 9. The tier sets are what decide whether a rung can be laid
 * out at all, so a drifted copy measures a chart the reader is not shown.
 *
 * Found by pass two of /code-review, one commit after the description half was
 * closed and the contract doc was edited to claim a Go test held these.
 */
export function stepShapes() {
  return parseStepShapes(
    readFileSync(join(repoRoot, "pkg", "cmd", "export", "data.go"), "utf8"));
}

/**
 * The spine view's declared column order, read out of pkg/cmd/export/data.go.
 *
 * READ AND NOT COPIED, FOR stepShapes' REASON ONE FIELD OVER. This list is what
 * app.js aligns every column on (alignFor), so a fixture carrying its own copy
 * would lay the goldens out in an order the site does not ship and every pin
 * over them would agree with the copy. It is also what places a kept flank:
 * tier 2 is adjacent to tier 0 in {0,2,5} and not in {0,5,2}, and a window
 * measured against the wrong order is a window drawn the wrong way round.
 *
 * A MISSING OR UNREADABLE DECLARATION THROWS rather than defaulting to the
 * empty list, which is the "drawn whole" state: defaulted, this file would
 * quietly measure the chart the spine drew before it had an order at all.
 */
export function spineRenderTiers() {
  return parseSpineRenderTiers(
    readFileSync(join(repoRoot, "pkg", "cmd", "export", "data.go"), "utf8"));
}

/**
 * The parse behind [spineRenderTiers], over source text so seam.mjs can drive
 * it over literals data.go does not contain.
 * @param {string} src
 * @returns {number[]}
 */
/**
 * A page config that draws the spine the way index.html declares it: the column
 * order off pkg/cmd/export/data.go, over the two published years.
 *
 * FOR THE FILES THAT LAY THE SPINE OUT WITHOUT OPENING ANYTHING. loadApp's own
 * default config declares no order, which is the state a page drawn whole is
 * in -- and index.html is no longer in it, so a layout measured under that
 * default measures a chart the site does not draw.
 */
export function spineConfig() {
  return Object.assign(twoYearConfig(), { render_tiers: spineRenderTiers() });
}

export function parseSpineRenderTiers(src) {
  const view = src.match(/spine := export\.View\{\n([\s\S]*?)\n\t\}\n/);
  if (!view) throw new Error("pkg/cmd/export/data.go declares no spine view literal to read");
  const m = view[1].match(/^\t\tRenderTiers:\s*\[\]int\{([\d,\s]*)\},$/m);
  if (!m) {
    throw new Error("the spine view in pkg/cmd/export/data.go declares no RenderTiers " +
      "this can read; a page that opens a node declares its column order");
  }
  const tiers = m[1].split(",").map((t) => t.trim()).filter(Boolean).map(Number);
  if (!tiers.length || tiers.some((t) => !Number.isInteger(t))) {
    throw new Error("the spine's RenderTiers literal did not parse as integers: " + m[1]);
  }
  return tiers;
}

/**
 * The parse behind [stepShapes], over source text rather than the file, so
 * seam.mjs can drive it over literals data.go does not contain -- including the
 * reflow that used to drop a step in silence.
 *
 * A LIST-VALUED `After` IS WHY THIS READS FIELDS AND NOT PATTERNS. `After:
 * \s*"..."` matched a string and defaulted to "" when it did not match, so the
 * day the field became []string every step would have parsed as a ROOT: three
 * steps read as three edges out of the spine's chart, drill.mjs measuring a
 * tree the site does not ship, and the step count agreeing with itself. Every
 * field this returns is either read or refused now, and the three counts below
 * are what catch an entry this missed.
 */
export function parseStepShapes(src) {
  // EVERY []export.DrillStep LITERAL, IN SOURCE ORDER, AND NOT THE FIRST. The
  // spine's steps are joined to two different projections -- pp.127-140 and
  // pp.85-125 -- and views() declares each group under its own guard, so a
  // corpus missing one document keeps the other's drill. Reading one literal
  // returned three steps of a four-step tree and every walk over it would have
  // measured a chart missing the one the site had just added.
  const literals = [...src.matchAll(/\[\]export\.DrillStep\{\n([\s\S]*?)\n\t\t\}/g)];
  if (!literals.length) {
    throw new Error("pkg/cmd/export/data.go declares no []export.DrillStep literal to read");
  }
  const block = literals.map((m) => m[1]).join("\n");
  const out = [];
  // ENTRIES ARE SLICED ON THE BRACE THAT OPENS ONE, not on a field inside it,
  // and the cross-check counts DIFFERENT markers. Slicing on `From:` made the
  // parse and its guard ask the same question: a step whose From was not at
  // exactly four tabs was skipped by both, so the count agreed with itself and
  // nothing threw. Measured -- reflowing step 1's opener to `{From: 4,`, which
  // gofmt accepts, dropped that step and attributed its `{Tier: 5, Cap: 8}` to
  // step 0, silently. That is the defect this function cited fisc-0flg for and
  // then repeated; found by pass three of /code-review.
  const starts = [...block.matchAll(/^\t{3}\{/gm)];
  for (const [i, m] of starts.entries()) {
    const body = block.slice(m.index, i + 1 < starts.length ? starts[i + 1].index : undefined);
    const from = body.match(/From:\s*(\d+),/);
    const tiers = body.match(/Tiers:\s*\[\]int\{([\d,\s]*)\}/);
    // A CAP MAY NAME ITS OWN NOUN. `{Tier: 4, Cap: 24, Tail: "divisions"}`
    // is one cap and not zero: a pattern closing on the Cap figure read the
    // fund-group step as capping one tier the moment its division cap gained
    // a Tail, and every check measuring that step would have measured a
    // column the site folds as one drawn whole.
    const caps = [...body.matchAll(/\{Tier:\s*(\d+),\s*Cap:\s*(\d+)(?:,\s*Tail:\s*"((?:[^"\\]|\\.)*)")?\}/g)];
    // KEY IS READ AND REQUIRED, FOR From's REASON ONE FIELD OVER. The Go type
    // requires one on every step and validateSteps refuses a view without, so
    // a step this parse read with no key is a step it did not read -- a
    // truncated slice or a brace it missed -- and not a step the site ships.
    const key = body.match(/Key:\s*"((?:[^"\\]|\\.)*)",/);
    // AFTER IS READ AND REQUIRED FOR THE SAME REASON, and the list is what
    // makes the requirement necessary rather than tidy: the Go type takes
    // []string and validateSteps refuses a step naming no chart at all, so a
    // step here with no readable After is a step this did not read. `[^}]*`
    // spans newlines deliberately -- gofmt is free to break a long list, and a
    // parse that read only the first line would return a shorter list and call
    // a step reachable from two charts reachable from one.
    const after = body.match(/After:\s*\[\]string\{([^}]*)\}/);
    // THE SIDE IS SPELLED AS THE CONSTANT, not as a string: data.go writes
    // `Side: export.SideSource`, which export.go keeps as a constant so that a
    // caller's "Source" cannot validate and mean nothing. A pattern that read
    // only a quoted literal returned "" for the shipped step, and every check
    // here measured a source-side step as opening the node its links point AT.
    const side = body.match(/Side:\s*(?:"((?:[^"\\]|\\.)*)"|export\.SideSource),/);
    const role = body.match(/Role:\s*"((?:[^"\\]|\\.)*)",/);
    // KEEP IS ABSENT OR READ, NEVER DEFAULTED -- the same discipline as Key and
    // After one field over, arrived at from the other side. A step declaring no
    // window declares no Keep, so absence is a value here; but a Keep this
    // cannot read is a window the site draws and no check sees, which is worse
    // than either. So: nothing declared, nothing returned; something declared
    // and unreadable, refused.
    const keep = body.match(/Keep:\s*\[\]int\{([\d,\s]*)\}/);
    // A WIDENING IS READ OR REFUSED FOR Keep's REASON, and the list matters
    // here in the same way After's does: `widen` is the ORDER the columns beyond
    // the window's three are dropped in, so a parse that read the first entry of
    // two would have a check measure a chart trimmed to a width the site does
    // not offer. The pattern spans newlines because gofmt is free to break the
    // literal, and an entry it cannot read as []int is refused below rather
    // than returned as no widening at all.
    const widen = body.match(/Widen:\s*\[\]int\{([\d,\s]*)\}/);
    if (!from) throw new Error(`step ${i} in data.go declares no From this can read`);
    if (!tiers) throw new Error(`step ${i} in data.go declares no Tiers literal this can read`);
    if (!key) throw new Error(`step ${i} in data.go declares no Key this can read`);
    if (!after) {
      throw new Error(`step ${i} in data.go declares no After list this can read; every ` +
        `step names the charts it opens from, and a root names "" among them`);
    }
    if (/Keep:/.test(body) && !keep) {
      throw new Error(`step ${i} in data.go declares a Keep this cannot read as []int`);
    }
    if (/Widen:/.test(body) && !widen) {
      throw new Error(`step ${i} in data.go declares a Widen this cannot read as []int`);
    }
    // THE LIST IS GO STRING LITERALS AND NOTHING ELSE. Reading only what the
    // quote pattern finds would take `[]string{"", stepKeyConst}` for a
    // one-entry list -- the shape the residual parse was fixed for (fisc-0flg)
    // arriving one field over -- so what is left after the literals, the commas
    // and the whitespace have been removed has to be empty.
    const text = after[1];
    const entries = [...text.matchAll(/"((?:[^"\\]|\\.)*)"/g)].map((q) => q[1]);
    if (text.replace(/"(?:[^"\\]|\\.)*"/g, "").replace(/[\s,]/g, "") !== "") {
      throw new Error(`step ${i} in data.go declares After as \`[]string{${text.trim()}}\`, ` +
        `which this cannot read as a list of string literals`);
    }
    const shape = {
      key: key[1],
      after: entries,
      // ABSENT IS THE DECLARED DEFAULT HERE, unlike Key and After. "" is what
      // the Go zero value means on each of these -- the opened node is the end
      // its ribbons point AT, and every node at the tier opens -- so a literal
      // omitting them is read rather than refused.
      side: side ? (side[1] === undefined ? "source" : side[1]) : "",
      role: role ? role[1] : "",
      from: Number(from[1]),
      tiers: tiers[1].split(",").map((x) => x.trim()).filter(Boolean).map(Number),
      caps: caps.map((c) => (c[3] === undefined
        ? { tier: Number(c[1]), cap: Number(c[2]) }
        : { tier: Number(c[1]), cap: Number(c[2]), tail: c[3] })),
    };
    // THE KEY IS ABSENT WHEN THE FIELD IS, which is what the wire does:
    // `json:"keep,omitempty"` ships no key for a step that keeps nothing, and a
    // shape carrying `keep: []` would have a client read an empty flank where
    // the site sends none.
    if (keep) shape.keep = keep[1].split(",").map((x) => x.trim()).filter(Boolean).map(Number);
    if (widen) shape.widen = widen[1].split(",").map((x) => x.trim()).filter(Boolean).map(Number);
    out.push(shape);
  }
  // THE MARKERS ARE INDEPENDENT, which is the whole point: braces say how many
  // entries the literal has, `From:`, `Key:` and `After:` say how many steps
  // declare one each, and they can disagree. An entry whose brace this missed,
  // or a From this read into the wrong entry, moves one count and not the
  // others. The field counts are deliberately loose on indentation so that a
  // reflow changes the PARSE and not the CHECK.
  const froms = (block.match(/From:\s*\d+,/g) || []).length;
  const keys = (block.match(/Key:\s*"/g) || []).length;
  const afters = (block.match(/After:\s*\[\]string\{/g) || []).length;
  if (out.length === 0 || out.length !== froms || out.length !== keys || out.length !== afters) {
    throw new Error(`parsed ${out.length} steps from a literal declaring ${froms} From, ` +
      `${keys} Key and ${afters} After fields`);
  }
  return out;
}

/**
 * The step descriptions the packager declares, in declaration order, read out of
 * pkg/cmd/export/data.go.
 *
 * THE SENTENCE THE CLIENT IS MEASURED UNDER HAS TO BE THE SENTENCE THE SITE
 * SHIPS. drill.mjs used to spell its own copy, and nothing held the two equal:
 * the Go test pins views() against a literal in the test and never reads this
 * directory, so rewording data.go and its test together left `make js` green
 * and put the new wording in front of readers. fisc-vsu8.
 *
 * Same shape and same reason as [parseResidualLiteral]: a parse of the Go
 * source, because there is no seam from Go to node, and a throw rather than a
 * partial answer.
 */
export function stepDescriptions() {
  const src = readFileSync(join(repoRoot, "pkg", "cmd", "export", "data.go"), "utf8");
  const out = [];
  const decl = /Description:\s*((?:"(?:[^"\\]|\\.)*"\s*\+?\s*)+),/g;
  let d;
  while ((d = decl.exec(src)) !== null) {
    const pieces = d[1].match(/"(?:[^"\\]|\\.)*"/g) || [];
    out.push(pieces.map((x) => JSON.parse(x)).join(""));
  }
  // Counted twice, and the second count does NOT presuppose the first pattern's
  // shape -- that was fisc-0flg, where both counts made the same assumption and
  // the guard could not see the case it existed for.
  const declared = (src.match(/^\s*Description:/gm) || []).length;
  if (out.length === 0 || out.length !== declared) {
    throw new Error(
      `parsed ${out.length} step descriptions from a file declaring ${declared}`);
  }
  for (const [i, text] of out.entries()) {
    if (!text) throw new Error(`step description ${i} parsed empty`);
  }
  return out;
}

/**
 * Drains the microtask queue so an in-flight main()/showYear() can finish.
 *
 * THE SANDBOX HAS NO TIMERS. It is `{ console, Intl, ...stub }` — no
 * setTimeout, no queueMicrotask — so there is nothing to schedule against and
 * nothing to wait on but promise resolution itself. Awaiting an already-resolved
 * promise N times lets N chained `await`s downstream run, and app.js's deepest
 * path (fetch → json → understands → paint) is a handful of them.
 *
 * The count is generous rather than tuned. A check that needed a precise number
 * would be asserting on how many awaits app.js happens to contain today, which
 * is exactly the kind of claim that goes stale silently; over-draining costs
 * microseconds and asserts nothing.
 */
export async function settle(turns = 50) {
  for (let i = 0; i < turns; i++) await Promise.resolve();
}

/**
 * A two-year FISC_CONFIG, which is what it takes to have a toggle at all.
 *
 * wireYears returns early on fewer than two years (app.js), so the default
 * single-year config leaves the control unwired and every check about switching
 * years driving nothing. Paths differ per year so a fetch stub can tell which
 * one it was asked for.
 */
export function twoYearConfig() {
  const year = (y, label, stem) => ({
    year: y, label, stem, path: `data/${stem}.json`, basis: "adopted",
    // Spelled the way buildSankeyPage's sankeyTitle composes it, because that
    // is what this fixture is a model OF. app.js reads year.title straight into
    // document.title; a config missing the key sets the title to "undefined",
    // which is how the lifecycle checks caught this fixture drifting.
    title: `City of Livermore budget flows \u2014 ${label}`,
    hero: { label: "h", value: "v", note: "n", kind: "hero" },
    figures: [{ label: "l", value: "v", note: "n", kind: "" }],
    caveats: [{ id: "c", summary: "s", href: "caveats.html#caveat-x--c" }],
    counts: { facts: 1, nodes: 1, links: 1 },
  });
  return {
    schema_version: 1,
    primary: "sankey",
    projections: { sankey: "data/sankey.json", "sankey-2027": "data/sankey-2027.json" },
    years: [year(2026, "FY 2025-26", "sankey"), year(2027, "FY 2026-27", "sankey-2027")],
    // POPULATED, AND IT IS LOAD-BEARING. citations() opens with
    // `const doc = CONFIG.docs[source.doc_id]; if (!doc) continue;`, so an
    // EMPTY docs map makes it return before it reaches
    // `for (const page of source.pages)` -- and every required-key check for a
    // `[].pages` key then passes because the GATE fired, never because a throw
    // was prevented. That is the green-but-dead shape this file already warns
    // about one fixture over. With this populated, deleting the
    // metadata.sources[].pages or links[].locators[].pages arm from
    // drawableSankey produces a real TypeError inside buildTable, which is
    // what those checks are supposed to be standing in front of.
    docs: {
      "livermore-budget-fy2026-2027": {
        title: "Adopted Budget FY2026-2027",
        publisher: "City of Livermore",
        pdf_url: "https://example.invalid/budget.pdf",
        page_text_base: "extracted/livermore-budget-fy2026-2027/pages/",
        records_base: "facts/livermore-budget-fy2026-2027/pages/",
      },
    },
  };
}

/**
 * A fetch stub whose responses a check controls, one per path.
 *
 * `plan` maps a path to what that fetch should do: a `{doc}` to resolve with, a
 * `{reject}` to fail with immediately, a `{hang: true}` to never settle — which
 * is what "a click DURING the opening fetch" needs — or a `{settle}` callback
 * handed the resolve/reject pair, which is how a check places a fetch outcome
 * AFTER a later event.
 *
 * It records the paths asked for, in order, so a check can assert which years
 * were actually requested rather than inferring it from what got painted.
 */
export function plannedFetch(plan) {
  const asked = [];
  // THE RUNG ANSWER IS PLANNED BY DEFAULT AND OVERRIDABLE. Every check that
  // drives main() reaches it before the first year, so a plan that did not
  // answer it would refuse the page and test the banner instead of whatever
  // the check is about -- while a check whose subject IS that banner still
  // plans its own entry, which wins.
  const full = Object.assign({ [RUNGS_PATH]: { doc: rungsAnswer() } }, plan);
  const fetch = (path) => {
    asked.push(path);
    const entry = full[path];
    if (!entry) return Promise.reject(new Error(`harness: no plan for ${path}`));
    if (entry.hang) return new Promise(() => {});
    // A promise the CHECK settles, which is the only way to place a fetch
    // outcome after a later event. An immediate rejection is handled in the
    // next microtask, so by the time a check could click anything the failure
    // has already been dealt with -- and "a fetch that fails AFTER the reader
    // switched away" is precisely the ordering fisc-8cg's second defect is
    // about. entry.settle is handed the resolve/reject pair.
    if (entry.settle) {
      return new Promise((resolve, reject) => entry.settle({
        resolve: (doc) => resolve({ ok: true, status: 200, json: () => Promise.resolve(doc) }),
        reject,
      }));
    }
    if ("reject" in entry) return Promise.reject(entry.reject);
    return Promise.resolve({
      ok: entry.ok !== false,
      status: entry.status || 200,
      // json() is a function so a check can make the BODY fail while the
      // response succeeds -- a 200 with a truncated document, which app.js
      // reaches through a different path from a refused fetch.
      json: () => (entry.badBody
        ? Promise.reject(new SyntaxError("Unexpected end of JSON input"))
        : Promise.resolve(entry.doc)),
    });
  };
  fetch.asked = asked;
  return fetch;
}

/** The .refusal banners currently attached to a planted <main>. */
export function refusals(main) {
  return main.children.filter((c) => c.className === "refusal");
}

/**
 * Resolves one check to {name, ok, detail}, turning a throw into a failure.
 *
 * IT LIVES HERE RATHER THAN IN run.mjs SO A CHECK CAN ASSERT ON IT. The runner
 * being synchronous is the defect that made every async check in this directory
 * unfalsifiable (fisc-dn9), and it cannot be detected from a check's own `ok`:
 * under a synchronous runner `Boolean(promise)` is TRUE for every promise, so no
 * promise-valued ok can ever report false, whatever it resolves to. The only
 * falsifiable form is a check that calls this and inspects what comes back —
 * which is why seam.mjs does exactly that, and why an obvious-looking
 * `ok: Promise.resolve(true)` tripwire was no tripwire at all.
 *
 * A check that THROWS is a failed check and not a crashed run: an async check
 * drives real app.js code and can reject for the same reasons the page can, and
 * a rejection taking the process down would report nothing about the others.
 */
export async function settleCheck(c) {
  try {
    return { name: c.name, ok: Boolean(await c.ok), detail: await c.detail };
  } catch (e) {
    return {
      name: c.name,
      ok: false,
      detail: `the check itself threw: ${e && e.stack ? e.stack : String(e)}`,
    };
  }
}
