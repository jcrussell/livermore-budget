// screen.test.mjs -- the chart's card full screen: the button the page shows
// only where the browser offers it, and the chart laid out at its own width
// in and out of it; and the zoom. jsdom has no Fullscreen API, so each test
// stubs the one the page calls.

import { describe, test } from "node:test";
import assert from "node:assert/strict";
import { loadApp, bootedApp, split, settle, fire, stylesheet } from "./testlib.mjs";

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
    // A split that inserts two columns: the group's one fund after it, and
    // fund/100's divisions, opened straight through, after that.
    await split(app, "fund-group/general");
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
    assert.equal(before, app.chartWidth(5));
    assert.equal(max, app.widthOf(app.HELD_COLUMNS) + "px");
    assert.equal(full, before);
    assert.equal(viewBoxWidth(document), before);
    assert.equal(app.isFullscreen(), false);
    assert.equal(button.textContent, config.wording.full_screen);
  });
});

/**
 * How many screen px one viewBox unit of the chart draws at, as style.css
 * sizes the svg from what app.js hands it. Nothing here renders CSS, so the
 * two declarations are read by selector and held to their shape; any other
 * shape is a refusal, which no measured answer equals.
 */
function drawnScale(document, window) {
  const css = stylesheet().replace(/\/\*[\s\S]*?\*\//g, "");
  const rule = (selector) => {
    const at = css.indexOf(selector + " {");
    return at < 0 ? "" : css.slice(at + selector.length + 2, css.indexOf("}", at));
  };
  const room = /--chart-room:\s*min\(var\(--chart-max, 100%\), calc\(100vw - var\(--chart-cushion\)\)\)/.test(rule(".chart-wrap"));
  const cushion = /--chart-cushion:\s*(\d+)px/.exec(rule(".chart-wrap"));
  const width = /(^|[\s;])width:\s*calc\(var\(--chart-room\) \* var\(--chart-stretch, 1\)\)/.test(rule(".chart-scroll > svg.sankey"));
  if (!room || !cushion || !width) return { refused: `room ${room}, cushion ${Boolean(cushion)}, svg width ${width}` };
  const style = document.documentElement.style;
  const drawn = Math.min(parseFloat(style.getPropertyValue("--chart-max")), window.innerWidth - Number(cushion[1])) *
    Number(style.getPropertyValue("--chart-stretch"));
  return { scale: drawn / viewBoxWidth(document) };
}

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

  // Mutation: drop scrollToMark's call from revealMark and this goes red,
  // the last column's mark left past the scroller's right edge.
  test("a chart past BAND_FLOOR scrolls in its card, and a mark focused in its last column is scrolled into sight", async (t) => {
    const { app, document } = await bootedApp();
    const root = document.documentElement.style;
    const atRoot = root.getPropertyValue("--chart-stretch");
    app.expandAll();
    await settle();
    const svg = document.getElementById("chart");
    const scroller = svg.parentElement;
    const laid = viewBoxWidth(document);
    const columns = app.layoutColumns(app.shown.doc);
    // jsdom lays nothing out: the svg is drawn at its laid width, in a card 1000px wide.
    let left = 0;
    Object.defineProperty(scroller, "clientWidth", { value: 1000, configurable: true });
    Object.defineProperty(scroller, "scrollLeft", { get: () => left, set: (v) => { left = v; }, configurable: true });
    svg.getBoundingClientRect = () => ({ x: 0, y: 0, left: 0, top: 0, right: laid, bottom: 0, width: laid, height: 0 });
    const last = app.laidNodes.reduce((a, b) => (b.x0 > a.x0 ? b : a));
    const at = (last.x0 + last.x1) / 2;
    const seen = () => at >= left && at <= left + 1000;
    const before = seen();
    markOf(document, last.id).focus();
    const detail = `${columns} columns laid at ${laid}px, --chart-stretch ${root.getPropertyValue("--chart-stretch")} ` +
      `(${atRoot} at the root); ${last.id} at x ${at.toFixed(0)}; in sight before ${before}, scrolled to ${left.toFixed(0)}`;
    t.diagnostic(detail);
    assert.deepEqual({
      scroller: scroller.classList.contains("chart-scroll"),
      stretched: Number(root.getPropertyValue("--chart-stretch")) > 1,
      atRoot: Number(atRoot),
      before, after: seen(), k: app.zoomOf().k,
    }, { scroller: true, stretched: true, atRoot: app.widthOf(3) / app.widthOf(app.HELD_COLUMNS), before: false, after: true, k: 1 }, detail);
  });

  // Mutation: hand --chart-max widthOf(columns) and --chart-stretch 1 again,
  // scaling the chart to fit its card, and this goes red: the split shrinks
  // every mark and moves the revenue column left.
  test("opening a split moves no column it leaves and rescales nothing", async (t) => {
    const viewport = 1400;
    const { app, document, window } = await bootedApp({ viewport });
    Object.defineProperty(window, "innerWidth", { value: viewport, configurable: true, writable: true });
    const where = (id) => {
      const d = app.laidNodes.find((n) => n.id === id);
      const s = drawnScale(document, window);
      return s.refused ? s : { scale: s.scale, x: d ? d.x0 * s.scale : null };
    };
    const before = where("revenue/taxes/sales");
    await split(app, "revenue/intergovernmental");
    const after = where("revenue/taxes/sales");
    const detail = `at a ${viewport}px viewport, revenue/taxes/sales before the split ${JSON.stringify(before)}, ` +
      `after ${JSON.stringify(after)}; ${app.layoutColumns(app.shown.doc)} columns`;
    t.diagnostic(detail);
    assert.deepEqual({ after, drawn: typeof before.x === "number" }, { after: before, drawn: true }, detail);
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
