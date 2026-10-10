// screen.test.mjs -- the chart's card full screen: the button the page shows
// only where the browser offers it, and the chart laid out at its own width
// in and out of it; and the zoom. jsdom has no Fullscreen API, so each test
// stubs the one the page calls.

import { describe, test } from "node:test";
import assert from "node:assert/strict";
import { loadApp, bootedApp, split, settle, fire } from "./testlib.mjs";

/**
 * Stubs the Fullscreen API on a loaded page, as a browser offering it does:
 * requestFullscreen makes the element the document's fullscreenElement and
 * fires fullscreenchange, and exitFullscreen undoes both. `width` is the
 * window's innerWidth once full screen.
 */
function stubFullscreen(window, document, width) {
  let shown = null;
  const was = window.innerWidth;
  Object.defineProperty(document, "fullscreenEnabled", { value: true, configurable: true });
  Object.defineProperty(document, "fullscreenElement", { get: () => shown, configurable: true });
  const change = (to, px) => {
    shown = to;
    Object.defineProperty(window, "innerWidth", { value: px, configurable: true, writable: true });
    document.dispatchEvent(new window.Event("fullscreenchange"));
    return Promise.resolve();
  };
  window.Element.prototype.requestFullscreen = function requestFullscreen() { return change(this, width); };
  document.exitFullscreen = () => change(null, was);
}

/** The width the chart on screen is laid out at, read off the viewBox render() wrote. */
function viewBoxWidth(document) {
  return Number(document.getElementById("chart").getAttribute("viewBox").split(" ")[2]);
}

describe("full screen on the chart's card", () => {
  test("the button stays hidden where the browser offers no full screen", async () => {
    const { document } = await bootedApp();
    assert.ok(document.getElementById("chart-fullscreen").hasAttribute("hidden"));
  });

  test("going full screen shows the card and says the way back, and the chart keeps the width it is laid out at", async (t) => {
    const loaded = await loadApp({ viewport: 1000 });
    const { app, window, document, config } = loaded;
    stubFullscreen(window, document, 4000);
    await app.boot();
    await settle();
    const button = document.getElementById("chart-fullscreen");
    assert.ok(!button.hasAttribute("hidden"), "the browser offers full screen and the button is hidden");
    assert.equal(button.textContent, config.wording.full_screen);
    // A split that inserts a column: fund/100's divisions after the funds.
    await split(app, "fund-group/general", "fund/100");
    const before = viewBoxWidth(document);
    const max = document.documentElement.style.getPropertyValue("--chart-max");

    fire(button, "click");
    await settle();
    const full = viewBoxWidth(document);
    assert.ok(app.isFullscreen(), "the card is not the full-screen element");
    assert.equal(button.textContent, config.wording.full_screen_exit);

    fire(button, "click");
    await settle();
    t.diagnostic(`fund/100 split: ${before}px laid, --chart-max ${max}; full screen ${full}px; after ${viewBoxWidth(document)}px`);
    assert.equal(before, app.chartWidth(4));
    assert.equal(max, app.chartWidth(4) + "px");
    assert.equal(full, before);
    assert.equal(viewBoxWidth(document), before);
    assert.equal(app.isFullscreen(), false);
    assert.equal(button.textContent, config.wording.full_screen);
  });
});

/** The drawn mark for one node id. */
function markOf(document, id) {
  const m = [...document.querySelectorAll("#chart g.node")].find((g) => g.__data__ && g.__data__.id === id);
  if (!m) throw new Error(`${id} is not drawn`);
  return m;
}

/** The chart's region in sight under the zoom now, in the chart's own coordinates. */
function inSight(app) {
  const t = app.zoomOf();
  const extent = app.zoomer.extent().call(app.zoomer, null);
  return { from: t.invert(extent[0]), to: t.invert(extent[1]) };
}

describe("zoom and pan on the chart", () => {
  test("the zoom moves the layer the form drew into and never the svg, labels counter-scale, and a mark still splits at any transform", async (t) => {
    const { app, document, config } = await bootedApp();
    const svg = document.getElementById("chart");
    const layer = () => svg.querySelector(":scope > g.zoom-layer");
    const reset = document.getElementById("chart-zoom-reset");
    assert.ok(!reset.hasAttribute("hidden"));
    assert.equal(reset.textContent, config.wording.zoom_reset);
    assert.ok(reset.hasAttribute("disabled"), "Reset view is offered on a chart nothing has zoomed");

    app.zoomer.transform(globalThis.d3.select(svg), globalThis.d3.zoomIdentity.translate(-600, -400).scale(3));
    t.diagnostic(`layer transform ${layer().getAttribute("transform")}; svg transform ${svg.getAttribute("transform")}; ` +
      `--label-scale ${svg.style.getPropertyValue("--label-scale")}`);
    assert.equal(svg.getAttribute("transform"), null, "the zoom transformed the svg itself");
    assert.equal(layer().getAttribute("transform"), "translate(-600,-400) scale(3)");
    assert.equal(Number(svg.style.getPropertyValue("--label-scale")), 1 / 3);
    assert.ok(svg.classList.contains("zoomed"));
    assert.ok(!reset.hasAttribute("disabled"));

    // A tooltip and an isolation at the transform, then a double click splits.
    const mark = markOf(document, "fund-group/general");
    fire(mark, "pointerenter");
    assert.ok(!document.getElementById("tooltip").hasAttribute("hidden"), "no tooltip at the transform");
    fire(mark, "click", { timeStamp: 10000 });
    assert.equal(app.filter.isolated, "fund-group/general");
    fire(mark, "dblclick", { timeStamp: 10100 });
    await settle();
    assert.deepEqual(app.expansions, ["fund-group/general"], "a double click at the transform did not split the mark");
    // The new chart is drawn whole.
    assert.equal(app.zoomOf().k, 1);
    assert.equal(layer().getAttribute("transform"), "translate(0,0) scale(1)");
    assert.ok(!svg.classList.contains("zoomed"));
    assert.ok(reset.hasAttribute("disabled"));
  });

  test("+ - and 0 zoom while focus is in the chart, and Reset view puts it back whole", async () => {
    const { app, document } = await bootedApp();
    const mark = markOf(document, "fund-group/general");
    mark.focus();
    fire(mark, "keydown", { key: "+" });
    assert.equal(app.zoomOf().k, app.ZOOM_STEP);
    fire(mark, "keydown", { key: "+" });
    fire(mark, "keydown", { key: "-" });
    assert.equal(Number(app.zoomOf().k.toFixed(9)), app.ZOOM_STEP);
    fire(mark, "keydown", { key: "0" });
    assert.equal(app.zoomOf().k, 1);
    // Keys outside the chart zoom nothing.
    fire(document.body, "keydown", { key: "+" });
    assert.equal(app.zoomOf().k, 1);
    app.zoomBy(4);
    fire(document.getElementById("chart-zoom-reset"), "click");
    const z = app.zoomOf();
    assert.deepEqual([z.k, z.x, z.y], [1, 0, 0]);
  });

  test("search pans the mark it goes to into sight when the zoom has left it out", async (t) => {
    const { app, document } = await bootedApp();
    // Zoomed into the top-left corner, a mark low in the right-hand column is out of sight.
    app.zoomer.transform(globalThis.d3.select(document.getElementById("chart")), globalThis.d3.zoomIdentity.scale(4));
    const far = app.laidNodes.reduce((a, b) => ((b.x0 + b.y0) > (a.x0 + a.y0) ? b : a));
    const centre = [(far.x0 + far.x1) / 2, (far.y0 + far.y1) / 2];
    const seen = (s) => centre[0] >= s.from[0] && centre[0] <= s.to[0] && centre[1] >= s.from[1] && centre[1] <= s.to[1];
    const before = seen(inSight(app));
    assert.ok(app.chooseSearch(far.id, false));
    const after = inSight(app);
    t.diagnostic(`${far.id} at (${centre.map((v) => v.toFixed(0))}); in sight before: ${before}; after, the view runs ` +
      `(${after.from.map((v) => v.toFixed(0))}) to (${after.to.map((v) => v.toFixed(0))}) at k ${app.zoomOf().k}`);
    assert.equal(before, false, "the planted zoom already shows the mark, so the pan is never asked");
    assert.ok(seen(after), "search went to a mark the zoom leaves out of sight");
    assert.equal(app.zoomOf().k, 4, "search changed the zoom rather than panning");
  });

  test("a wheel zooms only with Ctrl or Cmd or in full screen, and one finger pans only once zoomed", async () => {
    const { app } = await bootedApp();
    assert.equal(app.zoomFilter({ type: "wheel" }), false);
    assert.equal(app.zoomFilter({ type: "wheel", ctrlKey: true }), true);
    assert.equal(app.zoomFilter({ type: "wheel", metaKey: true }), true);
    assert.equal(app.zoomFilter({ type: "touchstart", touches: [1] }), false);
    assert.equal(app.zoomFilter({ type: "touchstart", touches: [1, 2] }), true);
    assert.equal(app.zoomFilter({ type: "mousedown", button: 0 }), true);
    assert.equal(app.zoomFilter({ type: "mousedown", button: 2 }), false);
    app.zoomBy(2);
    assert.equal(app.zoomFilter({ type: "touchstart", touches: [1] }), true);
  });
});
