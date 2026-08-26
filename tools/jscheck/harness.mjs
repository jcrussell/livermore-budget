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
  "caveats", "chart", "chart-desc", "chart-title", "counts-line", "derived-list",
  "detail", "figures", "flow-table", "lede-year", "legend", "table-view",
  "theme-toggle", "tooltip", "year-toggle",
]);

/**
 * Descendants of `root` carrying a bare class selector's class.
 *
 * Only `.name` is understood. Anything else returns nothing, which is the
 * honest answer for a stub that does not parse CSS — and it is visible as an
 * empty result rather than as a fabricated element.
 */
function byClass(root, sel) {
  if (!/^\.[\w-]+$/.test(sel)) return [];
  const want = sel.slice(1);
  const found = [];
  const walk = (n) => {
    for (const c of n.children || []) {
      if (typeof c.className === "string" && c.className.split(/\s+/).includes(want)) found.push(c);
      walk(c);
    }
  };
  walk(root);
  return found;
}

function domStub(ids = TEMPLATE_IDS) {
  /** Every element the stub hands out, by id, so a check can read one back. */
  const byId = new Map();

  const node = (id) => {
    const self = {
      id: id || "",
      tagName: "",
      className: "",
      dataset: {},
      children: [],
      textContent: "",
      innerHTML: "",
      style: { setProperty() {} },
      classList: { add() {}, remove() {}, toggle() {} },
      attributes: {},
      setAttribute(name, value) { self.attributes[name] = String(value); },
      getAttribute(name) { return name in self.attributes ? self.attributes[name] : null; },
      removeAttribute(name) { delete self.attributes[name]; },
      addEventListener(type, fn) { (self.listeners[type] ||= []).push(fn); },
      listeners: {},
      appendChild(c) { c.parent = self; self.children.push(c); return c; },
      append(...c) { for (const n of c) n.parent = self; self.children.push(...c); },
      prepend(c) { c.parent = self; self.children.unshift(c); },
      replaceChildren(...c) { for (const n of c) n.parent = self; self.children = c; },
      insertBefore(c) { c.parent = self; self.children.unshift(c); return c; },
      // remove() actually detaches, and that is not tidiness. clearRefusal()
      // finds the banner with querySelector and calls remove() on it; while
      // this was a no-op, a check asserting "the banner is gone" could not
      // fail, because the banner was never gone and never there.
      remove() {
        const i = self.parent ? self.parent.children.indexOf(self) : -1;
        if (i >= 0) self.parent.children.splice(i, 1);
      },
      parent: null,
      querySelector: (sel) => {
        const planted = self.selectable && self.selectable[sel];
        if (planted) return Array.isArray(planted) ? planted[0] : planted;
        return byClass(self, sel)[0] || null;
      },
      querySelectorAll: (sel) => {
        const planted = self.selectable && self.selectable[sel];
        if (planted) return planted;
        return byClass(self, sel);
      },
      // selectable is how a check plants what a selector should find. The stub
      // does not parse CSS -- it answers by exact selector string -- because a
      // selector engine here would be a second implementation of a thing the
      // browser already has, and what these checks are about is whether app.js
      // WRITES to what it finds.
      //
      // ONE SHAPE IS ANSWERED FOR REAL, and byClass below is it: a bare class
      // selector against the element's own descendants. app.js uses exactly one
      // -- `content.querySelector(".refusal")`, in both fail() and
      // clearRefusal() -- and while it answered null the banner could be
      // painted twice and could never be taken down, so a check asserting a
      // banner's ABSENCE passed whether or not app.js was correct. Answering
      // one selector shape is not a selector engine; it is the difference
      // between a check and a decoration.
      selectable: null,
    };
    return self;
  };

  const document = {
    title: "",
    documentElement: node(),
    body: node(),
    // Ids are REMEMBERED, so a check can ask what the page was told to show.
    // A fresh node per call would make every read return an empty element and
    // every assertion below vacuously true.
    getElementById: (id) => {
      if (!ids.has(id)) return null;
      if (!byId.has(id)) byId.set(id, node(id));
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
    createElementNS: (_ns, tag) => { const n = node(); n.tagName = tag; return n; },
    querySelector: (sel) => selectable[sel] ? selectable[sel][0] : null,
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
  const storage = new Map();
  // What window.matchMedia("...").addEventListener was told to follow. main()
  // guards on `typeof window.matchMedia === "function"`, so while matchMedia was
  // undefined that branch never ran under any check and "the page follows the OS
  // theme" was unfalsifiable.
  const mediaListeners = {};
  const windowListeners = {};
  return {
    byId,
    document,
    documentListeners,
    windowListeners,
    mediaListeners,
    localStorage: {
      getItem: (k) => (storage.has(k) ? storage.get(k) : null),
      setItem: (k, v) => storage.set(k, String(v)),
    },
    getComputedStyle: () => ({ getPropertyValue: () => "#000000" }),
    // matchMedia answers, and reports NOT-dark. app.js reads .matches for the
    // opening palette and attaches a change listener to follow the OS mid-visit;
    // both are behaviour worth checking, and neither was reachable while this
    // was undefined. prefersDark() consults the saved theme first, so every
    // existing layout check is unaffected by this becoming a function -- which
    // was verified by re-running them, not assumed.
    matchMedia: (query) => ({
      matches: false,
      media: query,
      addEventListener(type, fn) { (mediaListeners[type] ||= []).push(fn); },
      removeEventListener() {},
    }),
    // No fetch by default: main() is meant to give up here, in app.js's own
    // error path. A check that wants a document passes one to loadApp, which
    // installs it BEFORE app.js runs -- assigning one afterwards is too late,
    // because main() is called at file scope and has already reached its fetch.
    fetch: () => Promise.reject(new Error("harness: no network")),
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
  "NODE_WIDTH", "NODE_PADDING", "CHART_WIDTH", "CHART_HEIGHT", "LABEL_GUTTER",
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
 * @param {Set<string>|{ids?: Set<string>, config?: object, fetch?: Function}} [opts]
 */
export function loadApp(opts = {}) {
  // A bare Set is the old signature and still means "the ids the page
  // rendered". Kept because that is what most checks want and an options
  // object for one field reads worse at every call site.
  const o = opts instanceof Set ? { ids: opts } : opts;
  const stub = domStub(o.ids);
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
      caveats: ["c"],
      counts: { facts: 1, nodes: 1, links: 1 },
    }],
    docs: {},
  };

  // BOTH SEAMS ARE APPLIED BEFORE app.js RUNS, and that is not a style choice.
  // app.js ends in `main().catch(...)` at file scope, so the moment
  // runInContext returns, main has already read FISC_CONFIG, wired the toggle
  // and reached its fetch. Anything installed afterwards is installed after the
  // code that would have used it.
  if (o.config) sandbox.FISC_CONFIG = o.config;
  if (o.fetch) sandbox.fetch = o.fetch;

  const src = readFileSync(join(repoRoot, "site", "app.js"), "utf8");
  const exported = `\n;globalThis.__harness = { ${NAMES.join(", ")} };\n`;
  runInContext(src + exported, ctx, { filename: "app.js" });

  const app = sandbox.__harness;
  for (const n of NAMES) {
    if (app[n] === undefined) throw new Error(`app.js no longer defines ${n}`);
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

/** The committed worked example, which is FY2026 and is what the claims are about. */
export function goldenGraph() {
  return JSON.parse(readFileSync(join(repoRoot, "testdata", "sankey.golden.json"), "utf8"));
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
    hero: { label: "h", value: "v", note: "n", kind: "hero" },
    figures: [{ label: "l", value: "v", note: "n", kind: "" }],
    caveats: ["c"],
    counts: { facts: 1, nodes: 1, links: 1 },
  });
  return {
    schema_version: 1,
    primary: "sankey",
    projections: { sankey: "data/sankey.json", "sankey-2027": "data/sankey-2027.json" },
    years: [year(2026, "FY 2025-26", "sankey"), year(2027, "FY 2026-27", "sankey-2027")],
    docs: {},
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
  const fetch = (path) => {
    asked.push(path);
    const entry = plan[path];
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
