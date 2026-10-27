// The client tests' shared harness: jsdom over the skeletons under
// testdata/client/ (held to schema/bindings.json), the vendored d3, and the
// SHIPPED site/app.js imported afresh per test. Every artifact is read from
// testdata/, never assembled here.

import { JSDOM } from "jsdom";
import { existsSync, readFileSync } from "node:fs";
import assert from "node:assert/strict";
import { runInThisContext } from "node:vm";
import { dirname, join } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
export const repoRoot = join(here, "..");
const APP = join(here, "app.js");
const THEME = join(here, "theme.js");

/** The shipped stylesheet, as text, for a test about a rule jsdom cannot evaluate. */
export function stylesheet() {
  return readFileSync(join(here, "style.css"), "utf8");
}

/** The px the stylesheet declares once for a custom property, as `--name: Npx`. */
export function cssPx(name) {
  const found = [...stylesheet().matchAll(new RegExp(`${name}:\\s*(\\d+)px`, "g"))].map((m) => Number(m[1]));
  if (found.length !== 1) throw new Error(`style.css sets ${name} ${found.length} times`);
  return found[0];
}

/**
 * How style.css lays out the chart's body at innerWidth from what app.js
 * hands it: rail beside or not, its px, the room, the drawn width, the
 * whole width, in or out of full screen. Nothing renders CSS, so each
 * declaration is held to its shape; any other shape is a refusal.
 */
export function drawnBody(document, window) {
  const css = stylesheet().replace(/\/\*[\s\S]*?\*\//g, "");
  const rule = (selector) => {
    const m = new RegExp(`(?:^|[\\n}])\\s*${selector.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")} \\{([^}]*)\\}`).exec(css);
    return m ? m[1] : "";
  };
  // The variables are the card's and the chart body inherits them; the
  // body's own rules give its width and its grid.
  const card = rule("#chart-card");
  const body = rule(".chart-body");
  const besideVars = rule("#chart-card:has(> .chart-body[data-beside])");
  const beside = rule(".chart-body[data-beside]");
  const closed = rule('#chart-card:has(> .chart-body[data-beside][data-rail="closed"])');
  const screen = rule("#chart-card:fullscreen");
  const shapes = {
    room: /--chart-room:\s*min\(var\(--chart-max, 100%\), calc\(100vw - var\(--chart-cushion\) - var\(--chart-panels\)\)\)/.test(card),
    width: /(^|[\s;])width:\s*calc\(var\(--chart-room\) \+ var\(--chart-panels\)\)/.test(body),
    stacked: /--chart-panels:\s*0px/.test(card),
    drawnStacked: /--chart-drawn:\s*min\(var\(--chart-max, 100%\), var\(--chart-room\)\)/.test(card),
    drawnBeside: /--chart-drawn:\s*var\(--chart-max, 100%\)/.test(besideVars),
    panels: /--chart-panels:\s*var\(--rail-now\)\s*;/.test(besideVars),
    grid: /grid-template-columns:\s*var\(--rail-now\) var\(--chart-room\)\s*;/.test(beside),
    open: /--rail-now:\s*var\(--rail-width\)/.test(besideVars),
    tab: /--rail-now:\s*var\(--rail-tab\)/.test(closed),
    screen: /--chart-room:\s*calc\(100vw - var\(--screen-cushion\) - var\(--chart-panels\)\)/.test(screen),
    svg: /(^|[\s;])width:\s*calc\(var\(--chart-drawn\) \* var\(--chart-stretch, 1\)\)/.test(rule(".chart-scroll > svg.sankey")),
  };
  const px = (name) => {
    const m = new RegExp(`--${name}:\\s*(\\d+)px`).exec(card);
    return m ? Number(m[1]) : null;
  };
  const sizes = { cushion: px("chart-cushion"), screenCushion: px("screen-cushion"), rail: px("rail-width"), tab: px("rail-tab") };
  const bad = Object.keys(shapes).filter((k) => !shapes[k]).concat(Object.keys(sizes).filter((k) => sizes[k] === null));
  if (bad.length) return { refused: `style.css's chart body is not the shape read here: ${bad.join(", ")}` };
  const element = document.getElementById("chart-body");
  const isBeside = element.hasAttribute("data-beside");
  const full = Boolean(document.fullscreenElement) && document.fullscreenElement === document.getElementById("chart-card");
  const panels = isBeside ? (element.dataset.rail === "closed" ? sizes.tab : sizes.rail) : 0;
  const style = document.documentElement.style;
  const max = parseFloat(style.getPropertyValue("--chart-max"));
  const room = full ? window.innerWidth - sizes.screenCushion - panels : Math.min(max, window.innerWidth - sizes.cushion - panels);
  return {
    beside: isBeside, full, panels, room, drawn: isBeside ? max : Math.min(max, room), width: room + panels, ...sizes,
    stretch: Number(style.getPropertyValue("--chart-stretch")),
  };
}

let d3Loaded = false;

/**
 * Once per process, by runInThisContext not require: under require the UMD
 * bundles ask for d3-array, which is not installed.
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

/** testdata/client/flows.html and testdata/client/flows.config.json. */
export function pageFixture() {
  return {
    html: readFileSync(join(repoRoot, "testdata", "client", "flows.html"), "utf8"),
    config: clientConfig("flows"),
  };
}

/** testdata/client/<name>.config.json. */
function clientConfig(name) {
  return JSON.parse(readFileSync(join(repoRoot, "testdata", "client", name + ".config.json"), "utf8"));
}

/**
 * testdata/client/<name>.html with testdata/client/<name>.config.json in its
 * empty `<script id>`, as the template carries it.
 */
export function configuredPage(name, id) {
  const skeleton = readFileSync(join(repoRoot, "testdata", "client", name + ".html"), "utf8");
  const empty = `<script type="application/json" id="${id}"></script>`;
  if (!skeleton.includes(empty)) throw new Error(`the ${name} skeleton carries no empty ${id}`);
  const config = clientConfig(name);
  return { html: skeleton.replace(empty, empty.replace("></", ">" + JSON.stringify(config) + "</")), config };
}

/** A fresh jsdom over `html`, at `url` when given, with `module` imported and booted on its document. */
export async function bootedDocument(html, module, url) {
  const dom = new JSDOM(html, url ? { url } : undefined);
  const mod = await import(module);
  const doc = dom.window.document;
  mod.boot(doc);
  return { mod, doc, window: dom.window };
}

/**
 * installBrowser over `html` at `hash`, `boot` run on its window fetching
 * `plan`, settled. The hash is settled before boot: a fragment navigation
 * fires popstate in its own task, so the page never hears the test's.
 */
export async function bootedAt(html, boot, hash, plan, stamp) {
  const browser = installBrowser({ html });
  if (hash) browser.window.location.hash = hash;
  await settle();
  browser.window.fetch = plannedFetch(plan, stamp);
  await boot(browser.window);
  await settle();
  return browser;
}

/**
 * Fixture stems: each year's served path less .json.
 * @returns {string[]}
 */
export function publishedColumns() {
  return pageFixture().config.years.map((y) => y.path.replace(/\.json$/, ""));
}

/** One published column as the page fetches it: testdata/<stem>.column.json. */
export function columnFixture(stem) {
  return JSON.parse(readFileSync(join(repoRoot, "testdata", stem + ".column.json"), "utf8"));
}

/** The FY2026 spine as the page draws it: the sankey schedule of its column. */
export async function spineGraph() {
  const core = await import(pathToFileURL(join(here, "core.js")).href);
  return core.scheduleOf(columnFixture("fy2026-adopted"), "sankey");
}

/** Throws on an untaught query: answering false would silently stop testing. */
function mediaStub(width, osDark, motion) {
  const OS_DARK = "(prefers-color-scheme: dark)";
  const REDUCED = "(prefers-reduced-motion: reduce)";
  const state = { width, osDark, motion };
  const listeners = new Map();
  const answer = (q) => {
    if (q === OS_DARK) return state.osDark;
    if (q === REDUCED) return !state.motion;
    const max = /^\(max-width:\s*(\d+)px\)$/.exec(q);
    if (max) return state.width <= Number(max[1]);
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

/** The page installBrowser installed last, closed when it installs the next. */
let lastWindow = null;

/**
 * jsdom over one page as app.js's globals, with the stylesheet injected so
 * custom properties resolve. The URL matters: an opaque origin makes
 * localStorage throw. Reduced motion unless `motion`, since jsdom runs no
 * d3 transition to its end.
 */
export function installBrowser({ html, storage, viewport = 1000, osDark = false, motion = false } = {}) {
  // A frame the last page asked for (app.js's askRoom) would otherwise run
  // its module against this page's globals.
  if (lastWindow) lastWindow.close();
  const dom = new JSDOM(html, { url: "http://localhost/", pretendToBeVisual: true });
  const { window } = dom;
  lastWindow = window;
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
  const media = mediaStub(viewport, osDark, motion);
  window.matchMedia = media.matchMedia;
  return { window, document: window.document, media };
}

/**
 * Planned answers per path: `{doc}`, `null` (404), `{status, ok: false}`,
 * `{hang: true}`, `{reject}`, `{settle}`, `{doc, badBody: true}`. Pinned
 * columns by default; a planted generated_by is never overwritten.
 */
export function plannedFetch(plan, stamp) {
  const asked = [];
  const full = Object.assign({
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
 * A new module instance per call (cache-busting query): app.js reads its
 * globals at import and keeps state. `config: null` hands it none.
 */
export async function loadApp(o = {}) {
  loadD3();
  const page = pageFixture();
  const config = o.config === undefined ? page.config : o.config;
  const browser = installBrowser({
    html: o.html === undefined ? page.html : o.html,
    storage: o.storage, viewport: o.viewport, osDark: o.osDark, motion: o.motion,
  });
  if (o.checkedStem) {
    const select = browser.document.getElementById("year-select");
    if (select) select.value = keyOfStem(o.checkedStem, page.config);
  }
  const fetch = o.fetch || plannedFetch(o.plan, config ? config.exported_by : undefined);
  globalThis.FISC_CONFIG = config;
  globalThis.fetch = fetch;
  // The page's top bar boots site/theme.js before app.js; jsdom runs neither.
  (await import(pathToFileURL(THEME).href)).boot(browser.window);
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

/**
 * Splits through expandNode, "id~key" for a non-first split. The return
 * value is checked as well as the list: expandNode swallows its throw into
 * a banner.
 */
export async function split(app, ...entries) {
  for (const entry of entries) {
    const at = entry.indexOf(app.ALTERNATIVE_MARK);
    const id = at < 0 ? entry : entry.slice(0, at);
    const key = at < 0 ? undefined : entry.slice(at + 1);
    const drew = app.expandNode(id, key);
    await settle();
    // Drawn split, by its own entry or straight through its parent's.
    const want = key || app.splitsHere({ id })[0].key;
    if (!drew || !app.frontier || app.openSplitOf(app.fetched, app.frontier, id) !== want) {
      const banner = [...globalThis.document.querySelectorAll("main .refusal")].map((b) => b.textContent).join(" | ");
      throw new Error(`expandNode(${entry}) left the chart split at [${app.expansions.join(", ")}]: ${banner}`);
    }
  }
}

/** revealChain walked through the page's gestures, skipping open splits. */
export async function revealed(app, id) {
  for (const { node, key } of app.revealChain(app.fetched, id)) {
    if (app.frontier && app.openSplitOf(app.fetched, app.frontier, node) === key) continue;
    await split(app, app.entryOf(node, key));
  }
}

/** The form's shape at a frontier, no DOM: what app.projection holds after a redraw there. */
export function shapedAt(app, frontier, whole = new Set()) {
  return app.formOnScreen().shape(app.fetched, { chart: app.chartOnScreen(), frontier, whole });
}

/**
 * Draws out every tail to a fixed point (drawing one can expose another),
 * found by shaping with no DOM and redrawn once; any a filter's fold still
 * offers goes by its gesture. Bounded, so a re-engaging fold fails, not hangs.
 */
export function expandAll(app) {
  if (!app.expanding()) return 0;
  let entries = app.expansions.slice();
  for (let round = 0; ; round++) {
    if (round === 64) throw new Error("a chart still folds a tail after 64 rounds of drawing them out");
    const state = app.expansionState(app.fetched, entries);
    const shaped = shapedAt(app, state.frontier, state.whole);
    const tails = shaped.nodes.filter((n) => app.expandable(n) && !entries.includes(n.id)).map((n) => n.id);
    if (!tails.length) break;
    entries = entries.concat(tails);
  }
  const batched = entries.length - app.expansions.length;
  if (batched && !app.redraw(entries, "That could not be drawn out")) {
    throw new Error(`drawing out ${batched} tail(s) at once was refused`);
  }
  for (let more = 0; more < 64; more++) {
    const tail = app.projection.nodes.find((n) => app.expandable(n));
    if (!tail) return batched + more;
    if (!app.drawOutTail(tail)) throw new Error(`${tail.id} would not draw out`);
  }
  throw new Error(`a chart still offers a tail to draw out after ${batched} at once and 64 more by gesture`);
}

/** Each split alone from the root, and everything by first splits, capped and drawn out whole. */
export function expansionStates(app, doc) {
  const root = app.rootFrontier(doc);
  const out = [];
  for (const sp of doc.splits) {
    out.push({ where: `${sp.node} by ${sp.key}`, frontier: app.expand(doc, app.reveal(doc, root, sp.node), sp.node, sp.key) });
  }
  out.push({ where: "everything by primary splits", frontier: app.expandAllFrom(doc, root).frontier });
  const every = new Set(doc.splits.map((sp) => sp.node + "\u001f" + sp.key));
  return out.flatMap((st) => [Object.assign({ whole: new Set() }, st),
    Object.assign({}, st, { where: st.where + ", drawn out", whole: every })]);
}

/**
 * A state shaped, filtered and laid out as the page does, nothing rendered.
 * layOut sets the page's laidNodes and groupIndex.
 * @param {any} app a booted app
 * @param {{frontier: Set<string>, whole: Set<string>}} st
 * @param {any} prior null to lay cold (deep link, year switch), or app.boxesOf of the chart opened from
 */
export function layOutState(app, st, prior) {
  if (prior === undefined) throw new Error("layOutState: say which chart the state is laid against, or null for none");
  return app.layOut(app.filterFor(shapedAt(app, st.frontier, st.whole)).doc, prior || undefined, app.laidRoom);
}

/** The refusal banners on the page's <main>. */
export function refusals(document) {
  return [...document.querySelectorAll("main .refusal")];
}

/** The select is keyed on the column; these tests name years by stem. */
export function keyOfStem(stem, config = globalThis.FISC_CONFIG || pageFixture().config) {
  const year = (config.years || []).find((y) => y.stem === stem);
  return year ? year.path.replace(/\.json$/, "") : stem;
}

/** Picks one year in the top bar's select by its stem and fires change. */
export function clickYear(document, stem) {
  const select = document.getElementById("year-select");
  if (!select) throw new Error("the page rendered no year select to pick from");
  const key = keyOfStem(stem);
  if (![...select.options].some((o) => o.value === key)) throw new Error("the year select offers no option for " + stem);
  select.value = key;
  select.dispatchEvent(new globalThis.Event("change", { bubbles: true }));
}

/** `timeStamp` is plantable: the activation guard compares it. */
export function fire(element, type, extra = {}) {
  const { timeStamp, ...init } = extra;
  const Ctor = type.startsWith("key") ? globalThis.KeyboardEvent : globalThis.MouseEvent;
  const e = new Ctor(type, { bubbles: true, cancelable: true, ...init });
  if (timeStamp !== undefined) Object.defineProperty(e, "timeStamp", { value: timeStamp });
  element.dispatchEvent(e);
  return e;
}

/** jsdom reports a listener's throw to window's error event, so without this "did not throw" is vacuous. */
export function listenerErrors(window) {
  const errors = [];
  window.addEventListener("error", (ev) => {
    errors.push(String(ev.error || ev.message));
    ev.preventDefault();
  });
  return errors;
}

/** Records every keydown listener attached to the document. Install before boot. */
export function keydownListeners(document) {
  const attached = [];
  const original = document.addEventListener.bind(document);
  document.addEventListener = (type, fn, ...rest) => {
    if (type === "keydown") attached.push(fn);
    return original(type, fn, ...rest);
  };
  return attached;
}

const wholeDollars = new Intl.NumberFormat("en-US", { style: "currency", currency: "USD", maximumFractionDigits: 0 });
const compactDollars = new Intl.NumberFormat("en-US", {
  style: "currency", currency: "USD", notation: "compact", maximumFractionDigits: 1,
});

/** Cents as the page should print them, formatted here so an expectation cannot move with app.js's formatter. */
export function dollars(cents) {
  return (cents < 0 ? "−" : "") + wholeDollars.format(Math.abs(cents) / 100);
}

/** The short form a mark's own label carries, formatted here for the same reason. */
export function shortDollars(cents) {
  return (cents < 0 ? "−" : "") + compactDollars.format(Math.abs(cents) / 100);
}

/** Shown, pinned, label, page hrefs, and whether focus is inside. */
export function tipState(document) {
  const tip = document.getElementById("tooltip");
  const label = tip.querySelector(".tip-label");
  return {
    shown: !tip.hidden,
    pinned: tip.dataset.pinned === "true",
    label: label ? label.textContent : "",
    hrefs: [...tip.querySelectorAll(".facts a.page-text[href]")].map((a) => a.getAttribute("href")),
    pdfs: [...tip.querySelectorAll(".facts a.page-pdf[href]")].map((a) => a.getAttribute("href")),
    unlinked: [...tip.querySelectorAll(".facts [data-unlinked]")].map((x) => x.textContent),
    focused: tip.contains(document.activeElement) && document.activeElement !== document.body,
  };
}

/**
 * One PDF per page-text link, in order, each at that page; the faults.
 * @param {{hrefs: string[], pdfs: string[]}} tip a tipState
 * @returns {string[]}
 */
export function pdfFaults(tip) {
  if (!tip.hrefs.length) return ["the tip links no page"];
  if (tip.pdfs.length !== tip.hrefs.length) return [`${tip.hrefs.length} page texts and ${tip.pdfs.length} PDF links`];
  return tip.hrefs.flatMap((h, i) => {
    const m = /\/p(\d{4})\.txt$/.exec(h);
    const want = m ? "#page=" + Number(m[1]) : "";
    return want && tip.pdfs[i].startsWith("https://") && tip.pdfs[i].endsWith(want) ? [] : [`${h} beside ${tip.pdfs[i]}`];
  });
}

/** Whether a page-text link names a page text committed under data/. */
export function pageTextExists(href) {
  return /^extracted\/[^/]+\/pages\/p\d{4}\.txt$/.test(href) && existsSync(join(repoRoot, "data", href));
}

/**
 * In order: click `a` pins with focus in it; hovering `b`, leaving `a` keep
 * it; Escape unpins to `a`; Enter pins; an outside click unpins; Space
 * pins. Every `[data-pins]` is a button. Returns `a`'s pinned label.
 */
export function pinGestures(window, a, b) {
  const { document } = window;
  const marks = [...document.querySelectorAll("[data-pins]")];
  assert.ok(marks.length >= 2, "fewer than two marks pin a tip");
  const roleless = marks.filter((m) => m.getAttribute("role") !== "button").map((m) => m.dataset.id || m.className);
  assert.deepEqual(roleless, [], "a mark that pins a tip is not named a button");
  fire(a, "click");
  const pinned = tipState(document);
  assert.ok(pinned.shown && pinned.pinned, "a click pinned no tip");
  assert.ok(pinned.focused, "the focus did not move into the pinned tip");
  fire(b, "pointerenter");
  fire(b, "pointermove");
  fire(a, "pointerleave");
  assert.equal(tipState(document).label, pinned.label, "hovering another mark replaced the pinned tip");
  assert.ok(tipState(document).pinned && tipState(document).shown, "hovering elsewhere unpinned the tip");
  fire(document.activeElement, "keydown", { key: "Escape" });
  // The focus is back on the mark, and no tip shows until the next hover or
  // focus.
  assert.ok(!tipState(document).pinned, "Escape left the tip pinned");
  assert.ok(!tipState(document).shown, "Escape left a tip showing");
  assert.ok(document.activeElement === a, "Escape did not hand the focus back to the mark");
  fire(a, "keydown", { key: "Enter" });
  assert.ok(tipState(document).pinned, "Enter pinned no tip");
  assert.equal(tipState(document).label, pinned.label);
  fire(document.body, "click");
  assert.ok(!tipState(document).shown && !tipState(document).pinned, "a click outside left the tip pinned");
  fire(a, "keydown", { key: " " });
  assert.ok(tipState(document).pinned, "Space pinned no tip");
  fire(document.activeElement, "keydown", { key: "Escape" });
  return pinned.label;
}

/** markContra undone, so the drawn chart sums at the signs printed. */
export function unmarkContra(drawn) {
  if (!drawn.links.some((l) => l.contra && l.value_cents > 0)) return drawn;
  return Object.assign({}, drawn, {
    links: drawn.links.map((l) => l.contra && l.value_cents > 0
      ? Object.assign({}, l, { value_cents: -l.value_cents })
      : l),
  });
}

/** Every tile of a side's tree, parents before children, each with its depth. */
export function sideTiles(roots) {
  const out = [];
  const walk = (tiles, depth) => {
    for (const t of tiles) {
      out.push({ tile: t, depth });
      if (t.children) walk(t.children, depth + 1);
    }
  };
  walk(roots, 0);
  return out;
}
