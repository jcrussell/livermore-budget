// drawer.test.mjs -- the chart first: one toolbar above it, the legend,
// filters and key in a rail beside it, and the provenance drawer beside it
// on the other side, previewing what is hovered and holding what is pinned;
// and the width arithmetic that puts all three side by side at full size.

import { describe, test } from "node:test";
import assert from "node:assert/strict";
import { loadApp, bootedApp, split, settle, fire, drawnBody, stylesheet } from "./testlib.mjs";

/** The px BESIDE_QUERY asks the viewport for. */
function besideWidth(app) {
  const m = /^\(min-width: (\d+)px\)$/.exec(app.BESIDE_QUERY);
  if (!m) throw new Error("BESIDE_QUERY is not a min-width query: " + app.BESIDE_QUERY);
  return Number(m[1]);
}

/** A page booted at `viewport`, its window's innerWidth that wide too. */
async function at(viewport, o = {}) {
  const loaded = await bootedApp({ viewport, ...o });
  Object.defineProperty(loaded.window, "innerWidth", { value: viewport, configurable: true, writable: true });
  return loaded;
}

/** The ids of a ribbon's ends, or "" for nothing: what an assertion is handed in place of a mark. */
const ends = (d) => (d ? d.source.id + " -> " + d.target.id : "");

describe("the provenance drawer", () => {
  // Mutation: leave #detail where it was, below the chart and outside the
  // drawer, and this goes red: no #drawer #detail sits beside the chart.
  test("it sits beside the chart, after it, in the chart's body, and the card full screen takes it along", async () => {
    const { document } = await bootedApp();
    const body = document.getElementById("chart-body");
    const order = [...body.children].map((c) => c.id || c.className);
    const detail = document.querySelector("#chart-body > #drawer > #detail");
    assert.deepEqual({ order, drawer: Boolean(detail), inCard: Boolean(document.querySelector("#chart-card #drawer")) },
      { order: ["rail", "chart-wrap", "drawer"], drawer: true, inCard: true });
  });

  // Mutations: put #detail back below the chart, outside the drawer, and
  // this goes red, no #drawer #detail to read; hand the form showTip for its
  // tip rather than hover, and it goes red, nothing previewed.
  test("hovering a ribbon previews it, a second hover replaces the preview, a click pins, and no hover replaces a pin", async (t) => {
    const { app, document, config } = await bootedApp();
    const drawer = () => document.querySelector("#drawer #detail").textContent;
    const button = document.getElementById("detail-pin");
    const empty = drawer();
    const [a, b, c] = [...document.querySelectorAll("#chart path.link")];
    fire(a, "pointerenter");
    const previewA = { text: drawer(), previewed: ends(app.previewed), pinned: ends(app.pinned), shown: !button.hidden, pressed: button.getAttribute("aria-pressed") };
    fire(a, "pointerleave");
    const kept = drawer();
    fire(b, "pointerenter");
    const previewB = drawer();
    fire(b, "click");
    const pinnedB = { text: drawer(), pinned: ends(app.pinned), previewed: ends(app.previewed), pressed: button.getAttribute("aria-pressed") };
    fire(c, "pointerenter");
    const afterHover = drawer();
    t.diagnostic(`empty: "${empty.trim()}"; previewing ${previewA.previewed}; pinned ${pinnedB.pinned}`);
    assert.equal(empty.trim(), config.wording.detail_empty);
    assert.deepEqual(previewA, {
      text: previewA.text, previewed: ends(a.__data__), pinned: "", shown: true, pressed: "false",
    });
    assert.ok(previewA.text.includes(a.__data__.source.label), "the preview does not name the hovered ribbon");
    assert.equal(kept, previewA.text, "leaving the mark emptied the preview, so the pointer cannot reach its links");
    assert.notEqual(previewB, previewA.text);
    assert.deepEqual(pinnedB, { text: previewB, pinned: ends(b.__data__), previewed: "", pressed: "true" });
    assert.equal(afterHover, pinnedB.text, "a hover replaced the pin");
  });

  test("the drawer's button pins what it previews and lets a pin go, and a node's click pins it there", async () => {
    const { app, document, config } = await bootedApp();
    const drawer = () => document.querySelector("#drawer #detail").textContent;
    const button = document.getElementById("detail-pin");
    const hidden0 = button.hidden;
    const r = document.querySelector("#chart path.link");
    fire(r, "pointerenter");
    fire(button, "click");
    const pinned = ends(app.pinned);
    const hot = document.querySelectorAll("#chart path.link.hot").length;
    fire(button, "click");
    const unpinned = { pinned: ends(app.pinned), text: drawer().trim(), hidden: button.hidden, hot: document.querySelectorAll("#chart path.link.hot").length };
    const node = [...document.querySelectorAll("#chart g.node")].find((g) => g.__data__.id === "fund-group/general");
    fire(node, "click", { timeStamp: 5000 });
    const nodePinned = app.pinned ? app.pinned.id : "";
    assert.deepEqual({ hidden0, pinned, hot, unpinned, nodePinned, names: drawer().includes(node.__data__.label) }, {
      hidden0: true, pinned: ends(r.__data__), hot: 1,
      unpinned: { pinned: "", text: config.wording.detail_empty, hidden: true, hot: 0 },
      nodePinned: "fund-group/general", names: true,
    });
  });

  // Mutation: drop position: sticky from the drawer's rule and this goes red.
  test("beside the chart the drawer is sticky, so it stays in sight while the page scrolls the chart", () => {
    const css = stylesheet().replace(/\/\*[\s\S]*?\*\//g, "");
    const at = css.indexOf(".chart-body[data-beside] > .drawer {");
    const rule = at < 0 ? "" : css.slice(at, css.indexOf("}", at));
    const grid = css.slice(css.indexOf(".chart-body[data-beside] {"), css.indexOf("}", css.indexOf(".chart-body[data-beside] {")));
    assert.deepEqual({
      sticky: /position:\s*sticky/.test(rule), top: /\btop:\s*\d+px/.test(rule),
      fits: /max-height:\s*calc\(100vh - \d+px\)/.test(rule), scrolls: /overflow-y:\s*auto/.test(rule),
      // A stretched grid item is as tall as its row, and sticks to nothing.
      start: /align-items:\s*start/.test(grid),
    }, { sticky: true, top: true, fits: true, scrolls: true, start: true });
  });

  // The measurement the drawer's side is chosen by: the marks drawn inside
  // the card at the opening chart and once a split is open, and how far each
  // sits from the card's left and right edges on average.
  test("once a split is open the marks in the card sit nearer its right edge, the drawer's side, and an opened chart runs past it", async (t) => {
    const { app, document } = await bootedApp();
    const card = app.widthOf(app.openingColumns());
    const measure = () => {
      const xs = app.laidNodes.map((n) => (n.x0 + n.x1) / 2).concat([...document.querySelectorAll("#chart path.link")]
        .map((p) => (p.__data__.source.x1 + p.__data__.target.x0) / 2));
      const inCard = xs.filter((x) => x <= card);
      const mean = (f) => inCard.reduce((s, x) => s + f(x), 0) / inCard.length;
      return { left: Math.round(mean((x) => x)), right: Math.round(mean((x) => card - x)), past: xs.length - inCard.length };
    };
    const root = measure();
    await split(app, "fund-group/general");
    const opened = measure();
    t.diagnostic(`in a ${card}px card, mean px to the left and right edges: at the root ${root.left} and ${root.right}, ` +
      `${root.past} past the right edge; with fund-group/general open ${opened.left} and ${opened.right}, ${opened.past} past it`);
    assert.deepEqual({ nearerRight: opened.right < opened.left, runsPast: opened.past > 0 }, { nearerRight: true, runsPast: true });
  });
});

describe("the rail", () => {
  // Mutation: leave the rail's contents shown whatever railOpen says
  // (paintRail) and this goes red on the collapsed rail.
  test("it collapses to its tab and expands again, and the hash and the history do not move", async () => {
    const { app, document, window } = await at(2400);
    await split(app, "fund-group/general");
    const toggle = document.getElementById("rail-toggle");
    const body = document.getElementById("chart-body");
    const content = document.getElementById("rail-content");
    const state = () => ({
      expanded: toggle.getAttribute("aria-expanded"), rail: body.dataset.rail,
      hidden: content.hidden, hash: window.location.hash, history: window.history.length,
    });
    const hash = window.location.hash;
    const history = window.history.length;
    const open = state();
    fire(toggle, "click");
    const closed = state();
    const scale = drawnBody(document, window);
    fire(toggle, "click");
    const again = state();
    assert.deepEqual({ open, closed, again, hashIsView: app.hashOf(app.viewState()) === hash, room: scale.room }, {
      open: { expanded: "true", rail: "open", hidden: false, hash, history },
      closed: { expanded: "false", rail: "closed", hidden: true, hash, history },
      again: { expanded: "true", rail: "open", hidden: false, hash, history },
      hashIsView: true, room: app.widthOf(app.openingColumns()),
    });
    assert.ok(hash.includes("x="), "the hash carries no split, so nothing here could have moved it");
  });

  test("it opens by default only where it sits beside the chart, and the layout follows the viewport", async () => {
    // One page at a time: app.js reads the global window, which the last page loaded replaces.
    const said = (l) => ({
      beside: l.document.getElementById("chart-body").hasAttribute("data-beside"),
      open: l.app.railOpen, shown: !l.document.getElementById("rail-toggle").hidden,
    });
    const wide = await at(2400);
    const before = { wide: said(wide) };
    wide.media.setViewport(1000);
    const shrunk = said(wide).beside;
    const narrow = await at(1000);
    before.narrow = said(narrow);
    narrow.media.setViewport(2400);
    const grown = said(narrow).beside;
    assert.deepEqual({ before, grown, shrunk }, {
      before: { wide: { beside: true, open: true, shown: true }, narrow: { beside: false, open: false, shown: true } },
      grown: true, shrunk: false,
    });
  });
});

describe("the width arithmetic beside the chart", () => {
  // Mutation: BESIDE_QUERY without RAIL_WIDTH and DRAWER_WIDTH, and this goes
  // red: at the query's own width the panels take the chart's room.
  test("at BESIDE_QUERY's own width the rail and the drawer fit beside the opening chart at full size, open or collapsed, and a split keeps it so", async (t) => {
    const probe = await loadApp();
    const px = besideWidth(probe.app);
    const { app, document, window } = await at(px);
    const full = app.widthOf(app.openingColumns());
    const read = () => {
      const b = drawnBody(document, window);
      return b.refused ? b : { beside: b.beside, scale: b.room / full, fits: b.width + b.cushion <= px };
    };
    const open = read();
    const x = (id) => app.laidNodes.find((n) => n.id === id).x0;
    const before = x("revenue/taxes/sales");
    await split(app, "revenue/intergovernmental");
    const split1 = { ...read(), moved: x("revenue/taxes/sales") !== before };
    fire(document.getElementById("rail-toggle"), "click");
    const collapsed = read();
    const b = drawnBody(document, window);
    t.diagnostic(`BESIDE_QUERY ${app.BESIDE_QUERY}: the opening chart ${full}px + cushion ${app.CHART_CUSHION} + rail ` +
      `${app.RAIL_WIDTH} + drawer ${app.DRAWER_WIDTH}; open ${JSON.stringify(open)}, split ${JSON.stringify(split1)}, ` +
      `rail collapsed ${JSON.stringify(collapsed)} (body ${b.width}px)`);
    const want = { beside: true, scale: 1, fits: true };
    assert.deepEqual({ open, split1, collapsed }, { open: want, split1: { ...want, moved: false }, collapsed: want });
  });

  test("a pixel narrower the three stack, and the chart keeps its full size where the viewport has room for it alone", async () => {
    const probe = await loadApp();
    const px = besideWidth(probe.app) - 1;
    const { app, document, window } = await at(px);
    const b = drawnBody(document, window);
    assert.deepEqual({ beside: b.beside, panels: b.panels, room: b.room }, { beside: false, panels: 0, room: app.widthOf(app.openingColumns()) });
  });
});

describe("the toolbar", () => {
  test("search, Expand all, Collapse all, Reset view and Full screen sit in one row above the chart, and each still works", async () => {
    const loaded = await loadApp();
    const { app, window, document } = loaded;
    let shown = null;
    Object.defineProperty(document, "fullscreenEnabled", { value: true, configurable: true });
    Object.defineProperty(document, "fullscreenElement", { get: () => shown, configurable: true });
    window.Element.prototype.requestFullscreen = function requestFullscreen() {
      shown = this;
      document.dispatchEvent(new window.Event("fullscreenchange"));
      return Promise.resolve();
    };
    await app.boot();
    await settle();
    const bar = document.getElementById("chart-toolbar");
    const inBar = ["search", "breadcrumb", "chart-zoom-reset", "chart-fullscreen"].map((id) => bar.contains(document.getElementById(id)));
    const bulk = [...bar.querySelectorAll("button.crumb-bulk")];
    const [expandAll, collapseAll] = bulk;
    fire(expandAll, "click");
    await settle();
    const expanded = app.expansions.length;
    fire([...bar.querySelectorAll("button.crumb-bulk")][1], "click");
    await settle();
    const collapsed = app.expansions.length;
    const input = document.getElementById("search-input");
    input.value = "general";
    input.dispatchEvent(new window.Event("input", { bubbles: true }));
    const listed = document.querySelectorAll("#search-list [role=option]").length;
    app.zoomBy(2);
    fire(document.getElementById("chart-zoom-reset"), "click");
    const k = app.zoomOf().k;
    fire(document.getElementById("chart-fullscreen"), "click");
    await settle();
    assert.deepEqual({
      inBar, bulk: bulk.map((b) => b.textContent), collapseAll: Boolean(collapseAll), expanded: expanded > 0, collapsed,
      listed: listed > 0, k, full: app.isFullscreen(), drawerFull: shown.contains(document.getElementById("drawer")),
      railFull: shown.contains(document.getElementById("rail")),
    }, {
      inBar: [true, true, true, true], bulk: [app.CONFIG.wording.expand_all, app.CONFIG.wording.collapse_all],
      collapseAll: true, expanded: true, collapsed: 0, listed: true, k: 1, full: true, drawerFull: true, railFull: true,
    });
  });
});
