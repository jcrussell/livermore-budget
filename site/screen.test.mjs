// screen.test.mjs -- the chart's card full screen: the button the page shows
// only where the browser offers it, the column budget re-asked at the
// full-screen width, and the reader's budget given back on leaving. jsdom has
// no Fullscreen API, so each test stubs the one the page calls.

import { describe, test } from "node:test";
import assert from "node:assert/strict";
import { loadApp, bootedApp, opened, settle, fire } from "./testlib.mjs";

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

  test("going full screen re-asks the column budget at the full-screen width, and leaving gives the reader's back", async (t) => {
    const loaded = await loadApp({ viewport: 1000 });
    const { app, window, document, config } = loaded;
    stubFullscreen(window, document, 4000);
    await app.boot();
    await settle();
    const button = document.getElementById("chart-fullscreen");
    assert.ok(!button.hasAttribute("hidden"), "the browser offers full screen and the button is hidden");
    assert.equal(button.textContent, config.wording.full_screen);
    // A window that widens: on the overview the budget moves nothing.
    await opened(app, "fund-group/general");
    const before = { columns: app.drawnColumns(), width: viewBoxWidth(document) };

    fire(button, "click");
    await settle();
    const full = { columns: app.drawnColumns(), width: viewBoxWidth(document) };
    assert.ok(app.isFullscreen(), "the card is not the full-screen element");
    assert.equal(button.textContent, config.wording.full_screen_exit);

    fire(button, "click");
    await settle();
    const after = { columns: app.drawnColumns(), width: viewBoxWidth(document) };
    t.diagnostic(`fund-group/general at a 1000px viewport: ${before.columns} columns, ${before.width}px; ` +
      `full screen at 4000px: ${full.columns} columns, ${full.width}px; after: ${after.columns} columns, ${after.width}px`);
    assert.equal(before.columns, app.NARROW_COLUMNS);
    assert.equal(full.columns, app.OFFERED_COLUMNS, "full screen did not draw the chart at the full-screen budget");
    assert.equal(full.width, app.chartWidth(app.OFFERED_COLUMNS));
    assert.deepEqual(after, before, "leaving full screen did not give the reader's budget back");
    assert.equal(button.textContent, config.wording.full_screen);
  });

  test("columnsForWidth answers what COLUMN_QUERIES would at that viewport", async () => {
    const { app } = await loadApp();
    for (const px of [0, 1000, app.widthOf(4) + app.CHART_CUSHION - 1, app.widthOf(4) + app.CHART_CUSHION, 10000]) {
      const want = app.COLUMN_QUERIES.reduce((most, q) => (px >= Number(/(\d+)px/.exec(q.query)[1]) ? Math.max(most, q.columns) : most), app.NARROW_COLUMNS);
      assert.equal(app.columnsForWidth(px), want, `at ${px}px`);
    }
  });
});
