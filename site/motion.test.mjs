// motion.test.mjs -- a split keeps the reader's place: the marks a redraw
// keeps keep their elements, a column keeps its order, a new column grows out
// of its parent and moves only where the reader allows motion, and the node
// acted on stays where it was on screen. Through the shipped app.js over the
// pinned columns.

import { describe, test } from "node:test";
import assert from "node:assert/strict";

import { bootedApp, loadApp, split, settle } from "./testlib.mjs";

/** Every mark and ribbon element of the chart on screen, by id. */
function elementsOf(app, document) {
  const marks = new Map();
  for (const g of document.querySelectorAll("#chart g.nodes > g:not([data-leaving])")) marks.set(g.__data__.id, g);
  const ribbons = new Map();
  for (const p of document.querySelectorAll("#chart g.links > path:not([data-leaving])")) ribbons.set(app.linkKey(p.__data__), p);
  return { marks, ribbons };
}

/** The ids of one column of the laid chart, top to bottom. */
function columnOrder(app, layer) {
  return app.laidNodes.filter((n) => n.layer === layer).sort((a, b) => a.y0 - b.y0).map((n) => n.id);
}

const OPENED = "revenue/intergovernmental";

describe("a redraw keeps the reader's place", () => {
  // Mutation: drop the key functions in the form's render and every element
  // is rebound by index, so a surviving mark's element moves to another id.
  test("a mark and a ribbon the split keeps keep their elements", async (t) => {
    const { app, document } = await bootedApp();
    const was = elementsOf(app, document);
    await split(app, OPENED);
    const now = elementsOf(app, document);
    const lost = [];
    let kept = 0;
    for (const kind of ["marks", "ribbons"]) {
      for (const [id, el] of was[kind]) {
        if (!now[kind].has(id)) continue;
        kept++;
        if (now[kind].get(id) !== el) lost.push(`${kind}: ${id}`);
      }
    }
    t.diagnostic(`${kept} mark(s) and ribbon(s) kept across opening ${OPENED}; ${lost.length} on a new element`);
    assert.ok(kept > 0, "the split keeps no mark, so identity is not tested");
    assert.deepEqual(lost.slice(0, 5), []);
  });

  // Mutation: drop the seats in layOutAt's nodeSort and nodeRank's barycentre
  // re-sorts the opened node's column around its members.
  test(`opening ${OPENED} keeps its column's order, and a fresh chart is laid as nodeRank lays it`, async (t) => {
    const { app } = await bootedApp();
    const before = columnOrder(app, app.laidNodes.find((n) => n.id === OPENED).layer);
    await split(app, OPENED);
    // Read again: a source's members open a column before it.
    const layer = app.laidNodes.find((n) => n.id === OPENED).layer;
    const after = columnOrder(app, layer).filter((id) => before.includes(id));
    // The same chart laid fresh: what a reader who never split it sees.
    const fresh = app.formOnScreen().layOut(app.shown.doc, {
      tiers: app.RENDER_TIERS, columns: app.layoutColumns(app.shown.doc),
      groupOf: (n) => app.fundGroupOf(app.groupIndex, n), placeOf: app.fundGroupPlace, chart: app.chartOnScreen(),
    });
    const freshOrder = fresh.nodes.filter((n) => n.layer === layer).sort((a, b) => a.y0 - b.y0)
      .map((n) => n.id).filter((id) => before.includes(id));
    t.diagnostic(`column ${layer} before: ${before.join(", ")}; after: ${after.join(", ")}; laid fresh: ${freshOrder.join(", ")}`);
    assert.deepEqual(after, before);
    assert.notDeepEqual(freshOrder, before, "laid fresh the column keeps its order too, so the seats are not tested");
  });

  // A source's members stand before it, so they grow out of its left edge
  // and pay into it; every other split's grow out of its right edge.
  // Mutation: edgeOf answering the right edge whatever the side and the
  // source's members start at its right edge.
  for (const [opened, side] of [[OPENED, "left"], ["fund-group/general", "right"]]) {
    test(`a new column grows out of ${opened}'s ${side} edge, and a fold shrinks back into it`, async (t) => {
      const { app, document } = await bootedApp({ motion: true });
      const parent = app.laidNodes.find((n) => n.id === opened);
      const edge = side === "left" ? parent.x0 : parent.x1;
      // Read before a timer runs: the start is what the redraw leaves synchronously.
      assert.ok(app.expandNode(opened), `${opened} would not split`);
      const members = new Set(app.splitsOf(app.fetched, opened)[0].members);
      const { marks, ribbons } = elementsOf(app, document);
      const grown = [...marks].filter(([id]) => members.has(id));
      const starts = grown.map(([, g]) => Number(g.querySelector("rect").getAttribute("x")));
      const widths = grown.map(([, g]) => Number(g.querySelector("rect").getAttribute("width")));
      const hub = [...ribbons].filter(([, p]) => side === "left"
        ? p.__data__.target.id === opened && members.has(p.__data__.source.id)
        : p.__data__.source.id === opened && members.has(p.__data__.target.id));
      const hubWidths = hub.map(([, p]) => Number(p.getAttribute("stroke-width")));
      const hidden = [...document.querySelectorAll("#chart g.nodes text")].every((x) => x.getAttribute("opacity") === "0");
      t.diagnostic(`${grown.length} member(s) start at x ${[...new Set(starts)].join(", ")} (the parent's ${side} edge ${edge}), ` +
        `${hub.length} hub ribbon(s) at stroke-width ${[...new Set(hubWidths)].join(", ")}; labels hidden ${hidden}`);
      assert.ok(grown.length > 0 && hub.length > 0, "the split drew no member or no hub ribbon");
      assert.deepEqual({ starts: [...new Set(starts)], widths: [...new Set(widths)], hub: [...new Set(hubWidths)], hidden },
        { starts: [edge], widths: [0], hub: [0], hidden: true });

      app.finishMotion();
      const ends = grown.map(([id, g]) => Number(g.querySelector("rect").getAttribute("x")) === app.laidNodes.find((n) => n.id === id).x0);
      const shown = [...document.querySelectorAll("#chart g.nodes text")].every((x) => !x.hasAttribute("opacity"));
      assert.deepEqual({ ends: ends.every(Boolean), shown }, { ends: true, shown: true });

      // Folded back, the marks it drew leave inert and shrink into the parent
      // drawn now: its members, and theirs where it opened straight through.
      const open = new Set(app.laidNodes.map((n) => n.id));
      app.collapseNode(opened);
      await settle();
      const now = app.laidNodes.find((n) => n.id === opened);
      const gone = [...open].filter((id) => !app.laidNodes.some((n) => n.id === id)).sort();
      const leaving = [...document.querySelectorAll("#chart g.nodes > g[data-leaving]")];
      const inert = leaving.every((g) => !g.hasAttribute("tabindex") && g.getAttribute("aria-hidden") === "true");
      t.diagnostic(`folded: ${leaving.length} mark(s) leaving, inert ${inert}, into ${opened} at x ${now.x0}-${now.x1}`);
      assert.ok(gone.length >= members.size, `${gone.length} marks gone`);
      assert.deepEqual({ leaving: leaving.map((g) => g.__data__.id).sort(), inert }, { leaving: gone, inert: true });
      app.finishMotion();
      assert.equal(document.querySelectorAll("#chart g.nodes > g[data-leaving]").length, 0);
    });
  }

  // Mutation: motionMs answering MOTION_MS whatever the reader asks and the
  // members start at the parent's edge here.
  test("under prefers-reduced-motion a split is at its end when it returns", async () => {
    const { app, document } = await bootedApp();
    await split(app, OPENED);
    const members = new Set(app.splitsOf(app.fetched, OPENED)[0].members);
    const placed = app.laidNodes.filter((n) => members.has(n.id)).every((n) => {
      const g = [...document.querySelectorAll("#chart g.nodes > g")].find((x) => x.__data__.id === n.id);
      return Boolean(g) && Number(g.querySelector("rect").getAttribute("x")) === n.x0;
    });
    assert.deepEqual({
      motion: app.motionMs(), placed,
      labels: [...document.querySelectorAll("#chart g.nodes text")].some((x) => x.hasAttribute("opacity")),
    }, { motion: 0, placed: true, labels: false });
  });

  test("the anchor's arithmetic: the scroll that puts a node back where it was", async () => {
    const app0 = (await loadApp()).app;
    const was = { left: 300, top: 500 };
    // Drawn at 0.5px per unit, the svg's left edge 20px left of the screen's.
    const svg = { left: -20, top: 100, width: 1000 };
    assert.deepEqual(app0.anchorShift(was, svg, 2000, { x0: 640, y0: 900 }), { dx: 0, dy: 50 });
    assert.deepEqual(app0.anchorShift(was, svg, 2000, { x0: 700, y0: 780 }), { dx: 30, dy: -10 });
    assert.deepEqual(app0.anchorShift(was, { left: 0, top: 0, width: 0 }, 2000, { x0: 700, y0: 900 }), { dx: 0, dy: 0 });
  });

  // Mutation: drop keepInPlace's call from redraw and the opened node is
  // drawn where the new layout puts it, off where the reader was looking.
  test("the node a reader opens stays where it was on screen, in the scroller and on the page", async (t) => {
    const { app, document, window } = await bootedApp();
    const svg = document.getElementById("chart");
    const scroller = svg.parentElement;
    // jsdom lays nothing out: a page scrolled to `top` and a scroller to
    // `left`, the svg drawn at 0.8px per unit 140px down the page.
    // Scrolled across first, so the scroller has room to follow the node left.
    let left = 200;
    let top = 0;
    const k = 0.8;
    Object.defineProperty(scroller, "scrollLeft", { get: () => left, set: (v) => { left = v; }, configurable: true });
    window.scrollBy = (x, y) => { top += y; };
    svg.getBoundingClientRect = () => {
      const w = Number(svg.getAttribute("viewBox").split(" ")[2]) * k;
      return { x: -left, y: 140 - top, left: -left, top: 140 - top, right: w - left, bottom: 0, width: w, height: 0 };
    };
    const onScreen = (id) => {
      const d = app.laidNodes.find((n) => n.id === id);
      return { left: Math.round(-left + d.x0 * k), top: Math.round(140 - top + d.y0 * k) };
    };
    // A rect where the laid chart puts it: what the page reads the node's place off.
    window.Element.prototype.getBoundingClientRect = function () {
      const d = this.__data__;
      if (!d || typeof d.x0 !== "number") return { x: 0, y: 0, left: 0, top: 0, right: 0, bottom: 0, width: 0, height: 0 };
      const at = { left: -left + d.x0 * k, top: 140 - top + d.y0 * k };
      return { x: at.left, y: at.top, ...at, right: at.left + 14 * k, bottom: at.top + 10, width: 14 * k, height: 10 };
    };
    const id = "fund-group/general";
    const before = onScreen(id);
    const laidBefore = app.laidNodes.find((n) => n.id === id);
    const was = { x0: laidBefore.x0, y0: laidBefore.y0 };
    await split(app, id);
    const laidAfter = app.laidNodes.find((n) => n.id === id);
    const after = onScreen(id);
    t.diagnostic(`${id} laid at (${was.x0}, ${was.y0.toFixed(1)}) then (${laidAfter.x0}, ${laidAfter.y0.toFixed(1)}); ` +
      `on screen ${JSON.stringify(before)} then ${JSON.stringify(after)}, scrolled ${left.toFixed(1)} across and ${top.toFixed(1)} down`);
    assert.ok(laidAfter.y0 !== was.y0 || laidAfter.x0 !== was.x0, "the split does not move the node, so the anchor is not tested");
    assert.deepEqual(after, before);
  });

  // Mutation: expansionSlots placing every member at its parent's slot + 1
  // and the source's members stand after it, pushing every fund group a
  // column right of where the reader saw it.
  test(`opening ${OPENED}, a source, stands its members in the column before it and moves no fund group on screen`, async (t) => {
    const { app, document, window } = await bootedApp();
    const svg = document.getElementById("chart");
    const scroller = svg.parentElement;
    // jsdom lays nothing out: the scroller at `left`, unscrolled, the svg
    // drawn at 0.8px per unit 140px down the page, as the anchor test above.
    let left = 0;
    let top = 0;
    const k = 0.8;
    Object.defineProperty(scroller, "scrollLeft", { get: () => left, set: (v) => { left = v; }, configurable: true });
    window.scrollBy = (x, y) => { top += y; };
    svg.getBoundingClientRect = () => {
      const w = Number(svg.getAttribute("viewBox").split(" ")[2]) * k;
      return { x: -left, y: 140 - top, left: -left, top: 140 - top, right: w - left, bottom: 0, width: w, height: 0 };
    };
    window.Element.prototype.getBoundingClientRect = function () {
      const d = this.__data__;
      if (!d || typeof d.x0 !== "number") return { x: 0, y: 0, left: 0, top: 0, right: 0, bottom: 0, width: 0, height: 0 };
      const at = { left: -left + d.x0 * k, top: 140 - top + d.y0 * k };
      return { x: at.left, y: at.top, ...at, right: at.left + 14 * k, bottom: at.top + 10, width: 14 * k, height: 10 };
    };
    /** Each drawn node's left edge on screen, by id, for the ids `keep` keeps. */
    const onScreen = (keep) => new Map(app.laidNodes.filter((n) => keep(n.id)).map((n) => [n.id, Math.round(-left + n.x0 * k)]));
    const groups = (id) => id.startsWith("fund-group/");
    const before = onScreen((id) => groups(id) || id === OPENED);
    await split(app, OPENED);
    const after = onScreen((id) => groups(id) || id === OPENED);
    const hub = app.laidNodes.find((n) => n.id === OPENED);
    const members = new Set(app.splitsOf(app.fetched, OPENED)[0].members);
    const laid = app.laidNodes.filter((n) => members.has(n.id));
    const layers = [...new Set(laid.map((n) => n.layer))];
    t.diagnostic(`${OPENED} in column ${hub.layer}, its ${laid.length} member(s) in column(s) ${layers.join(", ")}; ` +
      `scrolled ${left.toFixed(1)} across; ${before.size - 1} fund group(s) at x ${[...before].filter(([id]) => groups(id)).map(([, x]) => x).join(", ")} ` +
      `then ${[...after].filter(([id]) => groups(id)).map(([, x]) => x).join(", ")}`);
    assert.ok(laid.length > 0 && before.size > 1, "the split drew no member or the chart no fund group, so nothing is tested");
    assert.deepEqual(layers, [hub.layer - 1]);
    assert.deepEqual([...after], [...before]);
    // The scroller made room on the left, and scrolled back it shows them.
    assert.ok(left > 0 && laid.every((n) => n.x0 >= 0), `scrolled ${left} across`);
  });
});
