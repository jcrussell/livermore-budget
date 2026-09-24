// testlib.mjs — the one helper the client's tests share.
//
// It installs a browser (jsdom over the page Go pins), loads the vendored d3
// into it, and imports the SHIPPED site/app.js afresh for each test. Nothing
// here parses Go source or assembles an artifact Go already writes: the page,
// the columns and the rung answer are read out of testdata/, where Go tests
// hold each byte for byte to what `fisc export` serves.
//
// NOT DISCOVERED BY `node --test`: only *.test.mjs is. Import it.

import { JSDOM } from "jsdom";
import { readFileSync } from "node:fs";
import { runInThisContext } from "node:vm";
import { dirname, join } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
export const repoRoot = join(here, "..");
const APP = join(here, "app.js");

/** The shipped stylesheet, as text, for a test about a rule jsdom cannot evaluate. */
export function stylesheet() {
  return readFileSync(join(here, "style.css"), "utf8");
}

let d3Loaded = false;

/**
 * Loads the vendored d3 bundles into THIS realm, once per process.
 *
 * runInThisContext AND NOT require: the bundles are UMD, and under require
 * they take the CommonJS branch and ask for d3-array, which is not installed
 * and is not meant to be. Run as scripts they take the browser branch and set
 * globalThis.d3, which is what app.js reads. One realm, so a value d3 builds
 * is deep-equal to one a test writes.
 */
export function loadD3() {
  if (d3Loaded) return;
  for (const f of ["d3.min.js", "d3-sankey.min.js"]) {
    runInThisContext(readFileSync(join(here, "vendor", f), "utf8"), { filename: f });
  }
  if (!globalThis.d3 || typeof globalThis.d3.sankey !== "function") {
    throw new Error("the vendored d3 did not define d3.sankey");
  }
  d3Loaded = true;
}

/**
 * The served page and the config it carries, from testdata/index.golden.html.
 *
 * THE CONFIG IS CUT OUT OF THE PAGE rather than pinned on its own, so there is
 * one copy of it: what a reader's browser evaluates is what a test hands the
 * module.
 */
export function pageFixture() {
  const html = readFileSync(join(repoRoot, "testdata", "index.golden.html"), "utf8");
  const open = "window.FISC_CONFIG = ";
  const i = html.indexOf(open);
  if (i < 0) throw new Error("the pinned page carries no window.FISC_CONFIG");
  const rest = html.slice(i + open.length);
  const j = rest.indexOf(";</script>");
  if (j < 0) throw new Error("the pinned page's FISC_CONFIG assignment is unterminated");
  return { html, config: JSON.parse(rest.slice(0, j)) };
}

/** One published column as the page fetches it: testdata/<stem>.column.json. */
export function columnFixture(stem) {
  return JSON.parse(readFileSync(join(repoRoot, "testdata", stem + ".column.json"), "utf8"));
}

/** Go's rung answer, testdata/rungs.json. */
export function rungsFixture() {
  return JSON.parse(readFileSync(join(repoRoot, "testdata", "rungs.json"), "utf8"));
}

/** The FY2026 spine as a projection document, testdata/sankey.golden.json. */
export function goldenGraph() {
  return JSON.parse(readFileSync(join(repoRoot, "testdata", "sankey.golden.json"), "utf8"));
}

/**
 * A matchMedia that answers from a viewport width and an OS theme, and fires
 * its change listeners when an answer crosses.
 *
 * jsdom has no matchMedia, and app.js guards every use with a typeof check, so
 * without this the column control and the theme follow would run in no test.
 * IT REFUSES A QUERY IT WAS NEVER TAUGHT rather than answering false: a stub
 * that answers a question it does not understand does not fail, it stops
 * testing.
 */
function mediaStub(width, osDark) {
  const OS_DARK = "(prefers-color-scheme: dark)";
  const state = { width, osDark };
  const listeners = new Map();
  const answer = (q) => {
    if (q === OS_DARK) return state.osDark;
    const m = /^\(min-width:\s*(\d+)px\)$/.exec(q);
    if (!m) throw new Error("the matchMedia stub cannot answer " + JSON.stringify(q));
    return state.width >= Number(m[1]);
  };
  const notify = (q) => {
    for (const fn of listeners.get(q) || []) fn({ matches: answer(q), media: q });
  };
  return {
    matchMedia: (q) => ({
      get matches() { return answer(q); },
      media: q,
      addEventListener(type, fn) {
        if (type === "change") listeners.set(q, (listeners.get(q) || []).concat(fn));
      },
      removeEventListener() {},
    }),
    /** Resizes the window and notifies every query whose answer changed. */
    setViewport(px) {
      const was = new Map([...listeners.keys()].map((q) => [q, answer(q)]));
      state.width = Number(px) || 0;
      for (const q of listeners.keys()) if (q !== OS_DARK && was.get(q) !== answer(q)) notify(q);
    },
    setOSDark(dark) { state.osDark = Boolean(dark); notify(OS_DARK); },
    /** Who follows one query, so a test can count listeners apart. */
    followers: (q) => listeners.get(q) || [],
    get viewportWidth() { return state.width; },
  };
}

/**
 * Installs a browser over one page: jsdom's window and document become the
 * globals app.js reads, the shipped stylesheet is injected so custom
 * properties resolve, and matchMedia is the stub above.
 *
 * THE URL MATTERS: an opaque origin makes localStorage throw.
 */
export function installBrowser({ html, storage, viewport = 1000, osDark = false } = {}) {
  const dom = new JSDOM(html, { url: "http://localhost/", pretendToBeVisual: true });
  const { window } = dom;
  for (const k of ["window", "document", "HTMLElement", "Element", "Node", "SVGElement",
    "Event", "KeyboardEvent", "MouseEvent", "CustomEvent", "navigator", "localStorage",
    "getComputedStyle", "requestAnimationFrame"]) {
    // defineProperty, because node declares navigator with a getter only.
    Object.defineProperty(globalThis, k, { value: window[k], configurable: true, writable: true });
  }
  const style = window.document.createElement("style");
  style.textContent = stylesheet();
  window.document.head.appendChild(style);
  for (const [k, v] of Object.entries(storage || {})) window.localStorage.setItem(k, v);
  const media = mediaStub(viewport, osDark);
  window.matchMedia = media.matchMedia;
  return { window, document: window.document, media };
}

/**
 * A fetch whose answers a test plans, one entry per path.
 *
 * An entry is `{doc}` to resolve with; `null` for a 404; `{status, ok: false}`
 * for another refusal; `{hang: true}` to never settle; `{reject}` to fail;
 * `{settle}` to be handed the resolve/reject pair, which is how a test places
 * a fetch outcome after a later event; `{doc, badBody: true}` for a 200 whose
 * body will not parse. The two pinned columns and the rung answer are planned
 * by default and an entry in `plan` wins over them.
 *
 * THE STAMP IS FILLED IN AND NEVER OVERWRITTEN: app.js refuses an artifact
 * whose generated_by disagrees with the page's exported_by, so a planned body
 * gets the page's stamp unless the test planted its own -- which a test about
 * that refusal does.
 */
export function plannedFetch(plan, stamp) {
  const asked = [];
  const full = Object.assign({
    "rungs.json": { doc: rungsFixture() },
    "fy2026-adopted.json": { doc: columnFixture("fy2026-adopted") },
    "fy2027-adopted.json": { doc: columnFixture("fy2027-adopted") },
  }, plan || {});
  for (const entry of Object.values(full)) {
    if (entry && entry.doc && typeof entry.doc === "object" && stamp !== undefined &&
        !Object.hasOwn(entry.doc, "generated_by")) {
      entry.doc.generated_by = stamp;
    }
  }
  const fetch = (path) => {
    asked.push(path);
    const entry = full[path];
    if (entry === undefined) return Promise.reject(new Error("testlib: no plan for " + path));
    if (entry === null) {
      return Promise.resolve({ ok: false, status: 404, json: () => Promise.reject(new Error("404")) });
    }
    if (entry.hang) return new Promise(() => {});
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
      json: () => (entry.badBody
        ? Promise.reject(new SyntaxError("Unexpected end of JSON input"))
        : Promise.resolve(entry.doc)),
    });
  };
  fetch.asked = asked;
  return fetch;
}

/** Lets every pending promise and I/O callback run. */
export async function settle() {
  for (let i = 0; i < 3; i++) await new Promise((r) => setImmediate(r));
}

let loads = 0;

/**
 * A fresh page: the browser installed, the globals planted, the shipped
 * app.js imported anew.
 *
 * A NEW MODULE INSTANCE PER CALL, by a cache-busting query on the import URL,
 * because app.js reads FISC_CONFIG and d3 at import and keeps its state in
 * module-level bindings. Nothing runs at import; call `app.boot()` and then
 * `settle()` to have the page draw, or call its functions directly.
 *
 * Defaults are the pinned page's: its config, both its columns and the rung
 * answer planned, the newest year checked. `checkedStem` moves the radio the
 * way a restored form state would. `config: null` hands the module no config
 * at all.
 */
export async function loadApp(o = {}) {
  loadD3();
  const page = pageFixture();
  const config = o.config === undefined ? page.config : o.config;
  const browser = installBrowser({
    html: o.html === undefined ? page.html : o.html,
    storage: o.storage, viewport: o.viewport, osDark: o.osDark,
  });
  if (o.checkedStem) {
    for (const r of browser.document.querySelectorAll("#year-toggle input[type=radio]")) {
      r.checked = r.value === o.checkedStem;
    }
  }
  const fetch = o.fetch || plannedFetch(o.plan, config ? config.exported_by : undefined);
  globalThis.FISC_CONFIG = config;
  globalThis.fetch = fetch;
  const app = await import(pathToFileURL(APP).href + "?load=" + (++loads));
  return { app, ...browser, fetch, config };
}

/** The page booted and settled: loadApp, then app.boot(). */
export async function bootedApp(o = {}) {
  const loaded = await loadApp(o);
  await loaded.app.boot();
  await settle();
  return loaded;
}

/** The node on top of the drill stack, or "" on the overview. */
export function topOf(app) {
  return app.drilled.length ? app.drilled[app.drilled.length - 1].id : "";
}

/**
 * Opens nodes in turn through the real entry point and waits for each
 * repaint, throwing by name where one does not open.
 *
 * THE RETURN VALUE IS CHECKED AS WELL AS THE STACK: drillDown swallows its own
 * throw and leaves the stack as it was, so a truthiness test on the stack
 * passes from the second node onward and a test would measure the chart that
 * was already there.
 */
export async function opened(app, ...ids) {
  for (const id of ids) {
    const outcome = await app.drillDown(id);
    await settle();
    if (outcome !== "drew" || topOf(app) !== id) {
      throw new Error(`drillDown(${id}) ${outcome} and left the chart on ${topOf(app) || "the overview"}`);
    }
  }
}

/**
 * Opens every folded tail on the chart on screen until none is left, the way
 * the breadcrumb's gesture does, and returns how many columns it opened.
 *
 * NOT A LOOP OVER TODAY'S TIERS: expanding one column can leave another
 * foldable column drawn where the first was hiding it. The bound turns a fold
 * that re-engages into a named failure rather than a hang.
 */
export function expandAll(app) {
  for (let done = 0; done < 32; done++) {
    const tail = app.projection.nodes.find((n) => app.expandable(n));
    if (!tail) return done;
    app.expandTier({ tier: tail.tier });
  }
  throw new Error("a chart still offers a column to expand after 32 expansions");
}

/**
 * Walks every view the chart offers, at every depth, reopening from the
 * overview for each so a rung never reuses the chart a sibling was opened
 * from. `visit(path, depth)` runs with the view on screen. Returns how many
 * were visited and the first refusal's message, or "".
 */
export async function everyOffer(app, visit, maxDepth = 8) {
  let visited = 0;
  const walk = async (path) => {
    if (path.length > maxDepth) throw new Error(`the drill went ${path.length} rungs deep at ${path.join(" > ")}`);
    app.drillUp(0);
    await settle();
    await opened(app, ...path);
    const offers = app.projection.nodes.filter((n) => app.drillable(n)).map((n) => n.id);
    for (const id of offers) {
      app.drillUp(0);
      await settle();
      await opened(app, ...path, id);
      visited++;
      const next = path.concat([id]);
      await visit(next, next.length);
      await walk(next);
    }
  };
  try {
    await walk([]);
  } catch (e) {
    app.drillUp(0);
    return { visited, refused: e && e.message ? e.message : String(e) };
  }
  app.drillUp(0);
  return { visited, refused: "" };
}

/** The refusal banners on the page's <main>. */
export function refusals(document) {
  return [...document.querySelectorAll("main .refusal")];
}

/**
 * Checks one year's radio and fires the change a browser would: on the input,
 * bubbling to the #year-toggle fieldset wireYears listens on. Dispatched on
 * the fieldset itself, e.target is the fieldset, which has no value, and the
 * handler returns without switching.
 */
export function clickYear(document, stem) {
  const group = document.getElementById("year-toggle");
  if (!group) throw new Error("the page rendered no year-toggle to click");
  let picked = null;
  for (const r of group.querySelectorAll("input[type=radio]")) {
    r.checked = r.value === stem;
    if (r.checked) picked = r;
  }
  if (!picked) throw new Error("the year control offers no radio for " + stem);
  picked.dispatchEvent(new globalThis.Event("change", { bubbles: true }));
}

/**
 * Fires one event on a drawn element the way a browser would.
 *
 * THE TIMESTAMP IS PLANTED, because the activation guard compares a click's
 * timeStamp against the key that may have synthesised it, and jsdom stamps
 * every event with the clock.
 */
export function fire(element, type, extra = {}) {
  const { timeStamp, ...init } = extra;
  const Ctor = type.startsWith("key") ? globalThis.KeyboardEvent : globalThis.MouseEvent;
  const e = new Ctor(type, { bubbles: true, cancelable: true, ...init });
  if (timeStamp !== undefined) Object.defineProperty(e, "timeStamp", { value: timeStamp });
  element.dispatchEvent(e);
  return e;
}

/**
 * Collects what a DOM listener throws. jsdom reports a listener's exception
 * to window's error event rather than to the dispatcher, so without this a
 * gesture that throws past its guard is invisible to the test that fired it
 * and "did not throw" is vacuous.
 */
export function listenerErrors(window) {
  const errors = [];
  window.addEventListener("error", (ev) => {
    errors.push(String(ev.error || ev.message));
    ev.preventDefault();
  });
  return errors;
}

/**
 * Records every keydown listener the page attaches to the document, so a test
 * can count the Escape handler the way it counts theme followers. Install
 * before boot.
 */
export function keydownListeners(document) {
  const attached = [];
  const original = document.addEventListener.bind(document);
  document.addEventListener = (type, fn, ...rest) => {
    if (type === "keydown") attached.push(fn);
    return original(type, fn, ...rest);
  };
  return attached;
}
