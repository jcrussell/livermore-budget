// screen.test.mjs -- the chart's card full screen: the button the page shows
// only where the browser offers it, and the chart laid out at its own width
// in and out of it; and the zoom. jsdom has no Fullscreen API, so each test
// stubs the one the page calls.

import { describe, test } from "node:test";
import assert from "node:assert/strict";
import { loadApp, bootedApp, split, settle, fire, drawnBody, stylesheet, pageFixture, refusals, listenerErrors } from "./testlib.mjs";

/**
 * Stubs the Fullscreen API on a loaded page, as a browser offering it does:
 * requestFullscreen makes the element the document's fullscreenElement and
 * fires fullscreenchange, and exitFullscreen undoes both. `width` is the
 * window's innerWidth once full screen; `media`, when given, is the page's
 * matchMedia stub, moved to the same width as a browser's viewport is.
 */
function stubFullscreen(window, document, width, media) {
  let shown = null;
  const was = window.innerWidth;
  Object.defineProperty(document, "fullscreenEnabled", { value: true, configurable: true });
  Object.defineProperty(document, "fullscreenElement", { get: () => shown, configurable: true });
  const change = (to, px) => {
    shown = to;
    Object.defineProperty(window, "innerWidth", { value: px, configurable: true, writable: true });
    if (media) media.setViewport(px);
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
    assert.equal(max, app.widthOf(app.openingColumns()) + "px");
    assert.equal(full, before);
    assert.equal(viewBoxWidth(document), before);
    assert.equal(app.isFullscreen(), false);
    assert.equal(button.textContent, config.wording.full_screen);
  });

  // Mutations: delete the .card:fullscreen .chart-body rule, or cap its
  // --chart-room with --chart-max again, and this goes red: the room read
  // is refused.
  test("full screen on a wide screen gives the chart the screen's width less the rail, so a split that panned fits, at the one scale, its viewBox unchanged", async (t) => {
    const screen = 4000;
    const loaded = await loadApp({ viewport: 1400 });
    const { app, window, document } = loaded;
    Object.defineProperty(window, "innerWidth", { value: 1400, configurable: true, writable: true });
    stubFullscreen(window, document, screen, loaded.media);
    await app.boot();
    await settle();
    await split(app, "fund-group/general");
    const read = () => {
      const b = drawnBody(document, window);
      return b.refused ? b : { full: b.full, beside: b.beside, room: b.room, drawn: b.drawn, stretch: b.stretch, rail: b.rail };
    };
    const before = { body: read(), viewBox: viewBoxWidth(document), scale: drawnScale(document, window).scale };
    fire(document.getElementById("chart-fullscreen"), "click");
    await settle();
    const full = { body: read(), viewBox: viewBoxWidth(document), scale: drawnScale(document, window).scale };
    const max = app.widthOf(app.openingColumns());
    t.diagnostic(`at 1400px the room is ${before.body.room}px; full screen at ${screen}px it is ${full.body.room}px ` +
      `(cushion ${app.SCREEN_CUSHION}, rail ${app.RAIL_WIDTH}) against --chart-max ${max}px; the split chart draws ` +
      `${full.body.drawn * full.body.stretch}px wide, viewBox ${full.viewBox}`);
    assert.deepEqual({
      full: full.body.full, beside: full.body.beside, room: full.body.room, wider: full.body.room > max,
      fits: full.body.drawn * full.body.stretch <= full.body.room, panned: before.body.drawn * before.body.stretch > before.body.room,
      scale: Number(full.scale.toFixed(9)), viewBox: full.viewBox,
    }, {
      full: true, beside: true, room: screen - app.SCREEN_CUSHION - app.RAIL_WIDTH, wider: true, fits: true, panned: true,
      scale: Number(before.scale.toFixed(9)), viewBox: before.viewBox,
    });
    assert.equal(Number(full.scale.toFixed(9)), 1);
  });
});

/**
 * Lays the chart out where jsdom lays nothing out: the window `height` px
 * tall, and the svg `width` px wide times app.js's --chart-stretch, as
 * style.css draws it, with its top `top` px down the document,
 * or `full` px down the card while the card is full screen (the card at the
 * screen's top, unscrolled). Each is read when roomHeight asks, so a test
 * moves the window by reassigning `at`.
 */
function stubRoom(window, document, at) {
  Object.defineProperty(window, "innerHeight", { get: () => at.height, configurable: true });
  const svg = document.getElementById("chart");
  svg.getBoundingClientRect = () => {
    const top = document.fullscreenElement ? at.full : at.top;
    const width = at.width * Number(document.documentElement.style.getPropertyValue("--chart-stretch") || 1);
    return { top, left: 0, width, height: 0, right: width, bottom: top, x: 0, y: top };
  };
  document.getElementById("chart-card").getBoundingClientRect = () =>
    ({ top: 0, left: 0, width: 0, height: at.height, right: 0, bottom: at.height, x: 0, y: 0 });
  return at;
}

/** The canvas the chart on screen is drawn on: its viewBox's height. */
function viewBoxHeight(document) {
  return Number(document.getElementById("chart").getAttribute("viewBox").split(" ")[3]);
}

/** The lowest laid node's bottom, to 1e-6: where d3-sankey's extent ended. */
function laidBottom(app) {
  return Number(Math.max(...app.laidNodes.map((n) => n.y1)).toFixed(6));
}

/** The next frame, where askRoom draws, and what it settles. */
async function frame(window) {
  await new Promise((r) => window.requestAnimationFrame(() => r()));
  await settle();
}

describe("the chart laid out to the window's height", () => {
  // Mutations: drop ctx.room from sankey.js's layOut, and the viewBox is the
  // room while the nodes stop at the floor; drop the `* laid / r.width` from
  // roomHeight, and the room is in screen px, half the units it should be;
  // drop ctx.room from its render, and the viewBox stays on the floor while
  // the nodes run past it. Drop testlib.mjs's close of the last page, and a
  // frame the full-screen tests above asked for draws their chart here.
  test("a window taller than the floor lays the chart out to the room, in the chart's own units; a shorter one leaves it on the floor", async (t) => {
    const grow = () => app.CONFIG.overview.sankey.grow;
    const at = { height: 1080, top: 100, full: 0, width: 660 };
    const loaded = await loadApp();
    const { app, window, document } = loaded;
    stubRoom(window, document, at);
    await app.boot();
    await frame(window);
    // The svg drawn 660px wide over a 1320-wide viewBox: a px is two units.
    const room = Math.floor((at.height - at.top - app.ROOM_CUSHION) * app.chartWidth(app.openingColumns()) / at.width);
    const tall = { room: app.roomHeight(), viewBox: viewBoxHeight(document), bottom: laidBottom(app) };
    t.diagnostic(`a ${at.height}px window, the chart ${at.top}px down drawn ${at.width}px wide: room ${tall.room} units, ` +
      `viewBox ${tall.viewBox}, nodes to ${tall.bottom} (floor ${grow().floor})`);
    assert.deepEqual(tall, { room, viewBox: room, bottom: room - 12 });
    assert.ok(room > grow().floor, "the room is no taller than the floor, so this proves less than it says");

    at.height = 768;
    at.width = 1320;
    window.dispatchEvent(new window.Event("resize"));
    await frame(window);
    const short = { room: app.roomHeight(), viewBox: viewBoxHeight(document), bottom: laidBottom(app) };
    t.diagnostic(`a ${at.height}px window, the chart drawn at scale 1: room ${short.room}, viewBox ${short.viewBox}`);
    assert.deepEqual(short, { room: at.height - at.top - app.ROOM_CUSHION, viewBox: grow().floor, bottom: grow().floor - 12 });
  });

  // Mutations: drop wireRoom's resize listener, and the window's growing
  // leaves the canvas where it was; reset heldHeight on every render rather
  // than at a new room, and Collapse all shortens the canvas under the
  // reader; drop that reset, and a shorter window keeps the taller canvas.
  test("a resize draws the chart again at the new room, a fold within one room never shortens the canvas, and a shorter window does", async (t) => {
    const grow = () => app.CONFIG.overview.sankey.grow;
    const at = { height: 1080, top: 100, full: 0, width: 1320 };
    const loaded = await loadApp();
    const { app, window, document } = loaded;
    stubRoom(window, document, at);
    await app.boot();
    await frame(window);
    const opened = viewBoxHeight(document);
    at.height = 1480;
    window.dispatchEvent(new window.Event("resize"));
    window.dispatchEvent(new window.Event("resize"));
    await frame(window);
    const grown = viewBoxHeight(document);
    app.expandAll();
    await settle();
    const all = viewBoxHeight(document);
    app.collapseAll();
    await settle();
    const folded = viewBoxHeight(document);
    at.height = 900;
    window.dispatchEvent(new window.Event("resize"));
    await frame(window);
    const shrunk = viewBoxHeight(document);
    t.diagnostic(`viewBox height: ${opened} at a 1080px window, ${grown} at 1480, ${all} everything split, ` +
      `${folded} folded back, ${shrunk} at 900`);
    assert.deepEqual({ opened, grown, all: all > grown, folded, shrunk }, {
      opened: 1080 - at.top - app.ROOM_CUSHION, grown: 1480 - at.top - app.ROOM_CUSHION, all: true, folded: all,
      shrunk: grow().floor,
    });
  });

  // Mutations: drop the canvas comparison from fitRoom, so any new room draws
  // the chart again, and both arms go red: the menu the reader opened is
  // closed by a room under the floor, and by every room on a chart that
  // declares no grow. Leave heldRoom where it was when fitRoom draws nothing,
  // and the next fold, laid out at the new room, lets the held canvas go and
  // Collapse all shortens it under the reader. Every move here is past
  // CHROME_SLACK, so none is left alone for being small.
  for (const declared of [true, false]) {
    test(`a new room that moves no canvas draws nothing again, and the menu a reader opened stays open (${declared ? "grow declared" : "no grow"})`, async (t) => {
      const config = structuredClone(pageFixture().config);
      if (!declared) delete config.overview.sankey.grow;
      const at = { height: 900, top: 100, full: 0, width: 1320 };
      const loaded = await loadApp({ config });
      const { app, window, document } = loaded;
      stubRoom(window, document, at);
      await app.boot();
      await frame(window);
      const opened = viewBoxHeight(document);
      const node = app.laidNodes.find((n) => n.sourceLinks.length > 0);
      app.openNodeMenu(node, { left: 0, top: 0 });
      assert.ok(document.getElementById("node-menu") !== null, "no menu opened, so this proves less than it says");
      // Under the floor with grow declared; anywhere without it.
      at.height = declared ? 700 : 1480;
      window.dispatchEvent(new window.Event("resize"));
      await frame(window);
      const moved = { viewBox: viewBoxHeight(document), menu: document.getElementById("node-menu") !== null };
      t.diagnostic(`viewBox ${opened} at a 900px window (room ${900 - at.top - app.ROOM_CUSHION}), ${moved.viewBox} at ` +
        `${at.height}px (room ${app.roomHeight()}); menu open after: ${moved.menu}`);
      assert.deepEqual(moved, { viewBox: opened, menu: true });
      if (!declared) return;
      assert.equal(opened, app.CONFIG.overview.sankey.grow.floor);
      // Everything split runs past the floor, so a room under it moves no
      // canvas; the fold after it holds the canvas it had.
      app.expandAll();
      await settle();
      const all = viewBoxHeight(document);
      at.height = 500;
      window.dispatchEvent(new window.Event("resize"));
      await frame(window);
      const still = viewBoxHeight(document);
      app.collapseAll();
      await settle();
      const folded = viewBoxHeight(document);
      t.diagnostic(`viewBox ${all} everything split, ${still} at a 500px window, ${folded} folded back`);
      assert.deepEqual({ past: all > opened, still, folded }, { past: true, still: all, folded: all });
      // A room that does move the canvas still draws it again.
      at.height = 1480;
      window.dispatchEvent(new window.Event("resize"));
      await frame(window);
      assert.deepEqual({ viewBox: viewBoxHeight(document), menu: document.getElementById("node-menu") !== null },
        { viewBox: 1480 - at.top - app.ROOM_CUSHION, menu: false });
    });
  }

  // Mutation: compare the canvas at the new room with the viewBox on screen
  // rather than with the canvas the nodes were laid on, and the held canvas
  // (the viewBox) never equals a collapsed chart's: the resize draws the chart
  // again, the viewBox goes to the floor and the menu closes.
  test("after Expand all and Collapse all hold the canvas, a room that moves no canvas keeps it and the open menu", async (t) => {
    const at = { height: 900, top: 100, full: 0, width: 1320 };
    const loaded = await loadApp();
    const { app, window, document } = loaded;
    stubRoom(window, document, at);
    await app.boot();
    await frame(window);
    const opened = viewBoxHeight(document);
    app.expandAll();
    await settle();
    const all = viewBoxHeight(document);
    app.collapseAll();
    await settle();
    const folded = viewBoxHeight(document);
    app.openNodeMenu(app.laidNodes.find((n) => n.sourceLinks.length > 0), { left: 0, top: 0 });
    assert.ok(document.getElementById("node-menu") !== null, "no menu opened, so this proves less than it says");
    at.height = 700;
    window.dispatchEvent(new window.Event("resize"));
    await frame(window);
    const moved = { viewBox: viewBoxHeight(document), menu: document.getElementById("node-menu") !== null };
    t.diagnostic(`viewBox ${opened} opened, ${all} everything split, ${folded} folded back, ${moved.viewBox} at a ` +
      `${at.height}px window; menu open after: ${moved.menu}`);
    assert.deepEqual({ held: folded === all && all > opened, ...moved }, { held: true, viewBox: all, menu: true });
  });

  // Mutation: lay fitRoom out with no prior (layOut(doc, undefined, room)),
  // and the split's columns are ordered by nodeRank at the new room.
  test("a resize that moves the canvas keeps every column's nodes in the order the reader's splits seated them", async (t) => {
    const at = { height: 1080, top: 100, full: 0, width: 1320 };
    const loaded = await loadApp();
    const { app, window, document } = loaded;
    stubRoom(window, document, at);
    await app.boot();
    await frame(window);
    for (let i = 0; i < 4; i++) {
      const n = app.laidNodes.find((d) => app.splittable(d) && !app.expansions.includes(d.id));
      if (n) await split(app, n.id);
    }
    const order = () => [...new Set(app.laidNodes.map((n) => n.x0))].sort((a, b) => a - b)
      .map((x) => app.laidNodes.filter((n) => n.x0 === x).sort((a, b) => a.y0 - b.y0).map((n) => n.id).join(" "));
    const seated = order();
    const fresh = app.formOnScreen().layOut(app.shown.doc, {
      tiers: app.RENDER_TIERS, columns: app.layoutColumns(app.shown.doc),
      groupOf: (n) => app.fundGroupOf(app.hueIndex(app.shown.doc), n), placeOf: app.fundGroupPlace,
      chart: app.chartOnScreen(), parentOf: app.parentOf, room: app.laidRoom,
    }).nodes;
    const ranked = [...new Set(fresh.map((n) => n.x0))].sort((a, b) => a - b)
      .map((x) => fresh.filter((n) => n.x0 === x).sort((a, b) => a.y0 - b.y0).map((n) => n.id).join(" "));
    const viewBox = viewBoxHeight(document);
    at.height = 1480;
    window.dispatchEvent(new window.Event("resize"));
    await frame(window);
    const resized = { order: order(), redrawn: viewBoxHeight(document) !== viewBox };
    t.diagnostic(`split at [${app.expansions.join(", ")}]: ${seated.filter((c, i) => c !== ranked[i]).length} column(s) ` +
      `seated otherwise than nodeRank; redrawn at a 1480px window ${resized.redrawn}`);
    assert.ok(seated.some((c, i) => c !== ranked[i]), "the splits seat every column as nodeRank does, so this proves less than it says");
    assert.deepEqual(resized, { order: seated, redrawn: true });
  });

  // Mutation: drop the CHROME_SLACK arm from fitRoom, and the phone's address
  // bar hiding draws the chart again: the viewBox moves and the menu closes.
  // Make the arm ignore the width, and the turned phone's arm goes red; make
  // it ignore the chart's top, and the pushed arm does.
  test("on a phone the address bar showing or hiding leaves the chart as drawn; a taller move, or a new width, draws it again", async (t) => {
    const at = { height: 800, top: 100, full: 0, width: 360 };
    const loaded = await loadApp();
    const { app, window, document } = loaded;
    Object.defineProperty(window, "innerWidth", { value: 360, configurable: true, writable: true });
    stubRoom(window, document, at);
    await app.boot();
    await frame(window);
    const opened = viewBoxHeight(document);
    assert.ok(opened > app.CONFIG.overview.sankey.grow.floor, "the room is under the floor, so a room change moves no canvas anyway");
    const menu = () => document.getElementById("node-menu") !== null;
    const openMenu = () => app.openNodeMenu(app.laidNodes.find((n) => n.sourceLinks.length > 0), { left: 0, top: 0 });
    const resize = async (height, width) => {
      at.height = height;
      if (width) {
        Object.defineProperty(window, "innerWidth", { value: width, configurable: true, writable: true });
        at.width = width;
      }
      window.dispatchEvent(new window.Event("resize"));
      await frame(window);
      return { viewBox: viewBoxHeight(document), menu: menu() };
    };
    openMenu();
    const bar = await resize(800 - app.CHROME_SLACK + 1);
    const back = await resize(800);
    const taller = await resize(800 - app.CHROME_SLACK);
    openMenu();
    // The window drawn at 680px, 10px taller and turned to a new width.
    const turned = await resize(690, 480);
    // What the page paints above the chart moves its top in a window that
    // has not moved, as boot's last fitRoom finds it.
    openMenu();
    at.top += 40;
    const pushed = { drawn: app.fitRoom(), menu: menu() };
    t.diagnostic(`a 360px phone, 800px tall: viewBox ${opened}; the bar hidden ${JSON.stringify(bar)}, shown again ` +
      `${JSON.stringify(back)}; ${app.CHROME_SLACK}px shorter ${JSON.stringify(taller)}; a new width ${JSON.stringify(turned)}; ` +
      `the top 40px lower ${JSON.stringify(pushed)}`);
    assert.deepEqual({ bar, back, taller: taller.viewBox < opened && !taller.menu, turned: !turned.menu, pushed },
      { bar: { viewBox: opened, menu: true }, back: { viewBox: opened, menu: true }, taller: true, turned: true,
        pushed: { drawn: true, menu: false } });
  });

  // Mutation: move heldRoom to the room now in fitRoom's CHROME_SLACK arm
  // and lay redraw out at roomHeight in place of layRoom, and the split with
  // the bar hidden is laid out at the taller room: the bar shown again, the
  // next split keeps that taller canvas over nodes laid at the shorter one,
  // a blank band under them. Default layOut's room to roomHeight, and a
  // layOut handed no room with the bar hidden lays out at the taller one.
  test("on a phone, splits as the address bar hides and shows lay out at the room the chart was drawn at, the nodes filling the canvas", async (t) => {
    const at = { height: 800, top: 100, full: 0, width: 360 };
    const loaded = await loadApp();
    const { app, window, document } = loaded;
    Object.defineProperty(window, "innerWidth", { value: 360, configurable: true, writable: true });
    stubRoom(window, document, at);
    await app.boot();
    await frame(window);
    const drawn = app.laidRoom;
    const resize = async (height) => {
      at.height = height;
      window.dispatchEvent(new window.Event("resize"));
      await frame(window);
    };
    const splitOne = async () => {
      const n = app.laidNodes.find((d) => app.splittable(d) && !app.expansions.includes(d.id));
      assert.ok(n !== undefined, "no node left to split, so this proves less than it says");
      await split(app, n.id);
      return { id: n.id, room: app.laidRoom, viewBox: viewBoxHeight(document), bottom: laidBottom(app) };
    };
    await resize(800 + app.CHROME_SLACK - 1);
    assert.ok(app.roomHeight() > drawn, "the bar hidden moved no room, so this proves less than it says");
    const hidden = await splitOne();
    await resize(800);
    const shown = await splitOne();
    await resize(800 + app.CHROME_SLACK - 1);
    app.layOut(app.shown.doc);
    const bare = app.laidRoom;
    t.diagnostic(`a 360px phone drawn at room ${drawn}; the bar hidden, split ${JSON.stringify(hidden)}; ` +
      `shown again, split ${JSON.stringify(shown)}; hidden again, layOut with no room at ${bare}`);
    assert.deepEqual({ hidden: [hidden.room, hidden.viewBox - hidden.bottom], shown: [shown.room, shown.viewBox - shown.bottom], bare },
      { hidden: [drawn, 12], shown: [drawn, 12], bare: drawn });
  });

  // Mutation: call render() bare from fitRoom again, and the planted throw
  // escapes the frame to the window with no banner.
  test("a room redraw whose layout throws leaves the chart as drawn and says so", async (t) => {
    const at = { height: 900, top: 100, full: 0, width: 1320 };
    const loaded = await loadApp();
    const { app, window, document } = loaded;
    const errors = listenerErrors(window);
    stubRoom(window, document, at);
    await app.boot();
    await frame(window);
    const before = { viewBox: viewBoxHeight(document), nodes: app.laidNodes.map((n) => n.id).join(" ") };
    const sankey = app.FORMS.get("sankey");
    app.FORMS.set("sankey", Object.assign({}, sankey, {
      layOut: () => { throw new Error("planted: no layout"); },
    }));
    try {
      at.height = 1480;
      window.dispatchEvent(new window.Event("resize"));
      await frame(window);
    } finally {
      app.FORMS.set("sankey", sankey);
    }
    const refused = {
      viewBox: viewBoxHeight(document), nodes: app.laidNodes.map((n) => n.id).join(" "),
      banner: refusals(document).map((b) => b.textContent).join(" | "), errors: errors.length,
    };
    t.diagnostic(`a layout that throws at a 1480px window: ${refused.banner}; ${refused.errors} error(s) to the window`);
    assert.deepEqual(refused, { ...before, banner: "The chart could not be fitted to the window: planted: no layout", errors: 0 });
  });

  // Mutation: let wholeOf measure the room again (roomHeight) in place of the
  // one layOut is handed, and the whole is laid out on a taller canvas than
  // the chart drawn from the same draw.
  test("a filtered draw lays the chart and its whole out at one room, measured once", async (t) => {
    const at = { height: 1080, top: 100, full: 0, width: 1320 };
    const loaded = await loadApp();
    const { app, window, document } = loaded;
    stubRoom(window, document, at);
    await app.boot();
    await frame(window);
    // Every read of the window's height finds it taller than the last.
    Object.defineProperty(window, "innerHeight", { get: () => (at.height += 100), configurable: true });
    const node = app.laidNodes.find((n) => n.targetLinks.length > 0 && n.sourceLinks.length === 0);
    app.filterOutNode(node.id);
    await settle();
    assert.ok(app.whole !== null, "the filter changed nothing drawn, so there is no whole to compare");
    const bottom = (nodes) => Number(Math.max(...nodes.map((n) => n.y1)).toFixed(6));
    t.diagnostic(`filtered out ${node.id}: the drawn chart to ${bottom(app.laidNodes)}, its whole to ${bottom(app.whole.nodes)}, ` +
      `laid at room ${app.laidRoom}`);
    assert.equal(bottom(app.whole.nodes), bottom(app.laidNodes));
  });

  // Mutation: drop askRoom from onFullscreenChange, and full screen draws
  // the chart at the room the page had (the stub fires no resize).
  test("entering full screen draws the chart to the card's room, and leaving it shortens it again", async (t) => {
    const grow = () => app.CONFIG.overview.sankey.grow;
    const at = { height: 1080, top: 400, full: 60, width: 1320 };
    const loaded = await loadApp({ viewport: 1920 });
    const { app, window, document } = loaded;
    stubFullscreen(window, document, 1920, loaded.media);
    stubRoom(window, document, at);
    await app.boot();
    await frame(window);
    const page = viewBoxHeight(document);
    fire(document.getElementById("chart-fullscreen"), "click");
    await frame(window);
    const full = viewBoxHeight(document);
    fire(document.getElementById("chart-fullscreen"), "click");
    await frame(window);
    const left = viewBoxHeight(document);
    t.diagnostic(`viewBox height: ${page} on the page (room ${at.height - at.top - app.ROOM_CUSHION}), ` +
      `${full} full screen, ${left} after`);
    assert.deepEqual({ page, full, left },
      { page: grow().floor, full: at.height - at.full - app.ROOM_CUSHION, left: grow().floor });
  });

  // Mutation: drop Math.min(g.ceiling, ...) from canvasHeight, and the canvas
  // is the whole room.
  test("the ceiling still caps a canvas the room would make taller", async (t) => {
    const grow = () => app.CONFIG.overview.sankey.grow;
    const at = { height: 100000, top: 100, full: 0, width: 1320 };
    const loaded = await loadApp();
    const { app, window, document } = loaded;
    stubRoom(window, document, at);
    await app.boot();
    await frame(window);
    t.diagnostic(`room ${app.roomHeight()}, viewBox ${viewBoxHeight(document)}, ceiling ${grow().ceiling}`);
    assert.equal(viewBoxHeight(document), grow().ceiling);
    assert.equal(laidBottom(app), grow().ceiling - 12);
  });
});

/**
 * How many screen px one viewBox unit of the chart draws at, as style.css
 * sizes the svg from what app.js hands it (drawnBody).
 */
function drawnScale(document, window) {
  const body = drawnBody(document, window);
  if (body.refused) return body;
  return { scale: body.drawn * body.stretch / viewBoxWidth(document) };
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

    // A tooltip at the transform, then a click splits.
    const mark = markOf(document, "fund-group/general");
    fire(mark, "pointerenter");
    assert.ok(!document.getElementById("tooltip").hasAttribute("hidden"), "no tooltip at the transform");
    fire(mark, "click", { timeStamp: 10000 });
    await settle();
    assert.deepEqual(app.expansions, ["fund-group/general"], "a click at the transform did not split the mark");
    // The new chart is drawn whole, and its split is still something to reset.
    assert.equal(app.zoomOf().k, 1);
    assert.equal(layer().getAttribute("transform"), "translate(0,0) scale(1)");
    assert.ok(!svg.classList.contains("zoomed"));
    assert.ok(!reset.hasAttribute("disabled"), "a split chart offers no Reset view");
  });

  // Mutation: drop the filter terms from atOpeningView and a filter alone
  // leaves Reset view disabled; drop the expansions term and a split does.
  test("a filter alone enables Reset view, and clicking it clears the filter and the hash", async (t) => {
    const { app, document, window } = await bootedApp();
    const reset = document.getElementById("chart-zoom-reset");
    const opened = { disabled: reset.hasAttribute("disabled"), hash: "#y=" + app.yearKey(app.shownYear) };
    const box = [...document.querySelectorAll("#filter-group-list input")].find((b) => b.value === "fund-group/general");
    box.checked = false;
    fire(box, "change");
    const filtered = { disabled: reset.hasAttribute("disabled"), hash: window.location.hash, zoom: app.zoomOf().k };
    fire(reset, "click");
    await settle();
    const after = { disabled: reset.hasAttribute("disabled"), hash: window.location.hash, groups: app.filter.groups.size, checked: box.checked };
    t.diagnostic(`opened ${JSON.stringify(opened)}; filtered ${JSON.stringify(filtered)}; reset ${JSON.stringify(after)}`);
    assert.deepEqual(filtered, { disabled: false, hash: opened.hash + "&hide=fund-group/general", zoom: 1 });
    assert.deepEqual(after, { disabled: true, hash: opened.hash, groups: 0, checked: true });
    assert.equal(opened.disabled, true);
  });

  test("a split, a pin and a zoom together are all undone by one Reset view", async () => {
    const { app, document, window } = await bootedApp();
    const reset = document.getElementById("chart-zoom-reset");
    const hash = "#y=" + app.yearKey(app.shownYear);
    await split(app, "fund-group/general");
    const split1 = !reset.hasAttribute("disabled");
    app.pin(app.laidNodes.find((n) => n.id !== "fund-group/general"));
    app.zoomBy(2);
    fire(reset, "click");
    await settle();
    const z = app.zoomOf();
    assert.deepEqual({ split1, x: app.expansions.length, pinned: app.pinned === null, zoom: [z.k, z.x, z.y], hash: window.location.hash, disabled: reset.hasAttribute("disabled") },
      { split1: true, x: 0, pinned: true, zoom: [1, 0, 0], hash, disabled: true });
    assert.equal(refusals(document).length, 0);
  });

  // Mutation: in resetView, unpin and resetZoom before moveTo, and the pin
  // and the zoom arms go red.
  test("a Reset view that cannot draw lets nothing go and adds no history step, and says it was not made", async () => {
    const { app, document, window } = await bootedApp();
    const reset = document.getElementById("chart-zoom-reset");
    const box = [...document.querySelectorAll("#filter-group-list input")].find((b) => b.value === "fund-group/general");
    box.checked = false;
    fire(box, "change");
    const node = app.laidNodes.find((n) => n.id !== "fund-group/general");
    app.pin(node);
    app.zoomBy(2);
    const was = { hash: window.location.hash, steps: window.history.length, zoom: app.zoomOf().k, pinned: app.pinned && app.pinned.id };
    const d3 = globalThis.d3;
    const sankey = d3.sankey;
    d3.sankey = () => { throw new Error("the layout refused"); };
    try {
      fire(reset, "click");
      await settle();
    } finally {
      d3.sankey = sankey;
    }
    const after = { hash: window.location.hash, steps: window.history.length, zoom: app.zoomOf().k, pinned: app.pinned && app.pinned.id };
    assert.deepEqual(after, was);
    assert.equal(app.filter.groups.size, 1, "the filter the reset could not lay out is kept");
    const said = refusals(document).map((b) => b.textContent);
    assert.equal(said.length, 1);
    assert.match(said[0], /^The view was not wholly reset: /);
    assert.ok(!reset.hasAttribute("disabled"), "a view still to reset offers Reset view");
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
    await settle();
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
    }, { scroller: true, stretched: true, atRoot: 1, before: false, after: true, k: 1 }, detail);
  });

  // Mutation: hand --chart-max widthOf(HELD_COLUMNS) and stretch over it, as
  // one scale fitted to four columns, and this goes red at 0.81.
  test("the opening chart draws at full size at a 1400px viewport, and a split keeps it so", async (t) => {
    const viewport = 1400;
    const { app, document, window } = await bootedApp({ viewport });
    Object.defineProperty(window, "innerWidth", { value: viewport, configurable: true, writable: true });
    // To a billionth: the stretch is a quotient of two widths.
    const scale = () => { const s = drawnScale(document, window); return s.refused ? s : { scale: Number(s.scale.toFixed(9)) }; };
    const root = scale();
    await split(app, "fund-group/general");
    const opened = scale();
    const detail = `at a ${viewport}px viewport the root draws at ${JSON.stringify(root)}, ` +
      `${app.layoutColumns(app.shown.doc)} columns at ${JSON.stringify(opened)}`;
    t.diagnostic(detail);
    assert.deepEqual({ root, opened }, { root: { scale: 1 }, opened: { scale: 1 } }, detail);
  });

  /**
   * The chart split to its finest grain in a 1000px scroller, with jsdom's
   * missing layout planted: the svg drawn at its laid width, the scroller's
   * scrollLeft a plain number.
   */
  async function scrolling() {
    const loaded = await bootedApp();
    loaded.app.expandAll();
    await settle();
    const svg = loaded.document.getElementById("chart");
    const scroller = svg.parentElement;
    const laid = viewBoxWidth(loaded.document);
    let left = 0;
    Object.defineProperty(scroller, "clientWidth", { value: 1000, configurable: true });
    Object.defineProperty(scroller, "scrollLeft", { get: () => left, set: (v) => { left = v; }, configurable: true });
    svg.getBoundingClientRect = () => ({ x: 0, y: 0, left: 0, top: 0, right: laid, bottom: 0, width: laid, height: 0 });
    return Object.assign(loaded, { svg, scroller, left: () => left });
  }

  /** A press, a move of `dx` and a release on `on`, as a mouse delivers them. */
  function drag(on, dx) {
    fire(on, "pointerdown", { button: 0, clientX: 500, clientY: 300 });
    fire(on, "pointermove", { button: 0, clientX: 500 + dx, clientY: 300 });
    fire(on, "pointerup", { button: 0, clientX: 500 + dx, clientY: 300 });
  }

  // Mutation: drop wirePan's call from wireZoom and this goes red, the
  // scroller left at 0.
  test("a drag across the chart pans its scroller, and swallows the click that ends it", async (t) => {
    const { app, document, scroller, left } = await scrolling();
    const ribbon = document.querySelector("#chart path.link");
    const mark = markOf(document, "fund-group/general");
    drag(ribbon, -300);
    const panned = left();
    const was = app.expansions.length;
    fire(mark, "click", { timeStamp: 1000 });
    await settle();
    // What a click on the mark would have done: pinned it, splitting it where it was shut.
    const swallowed = (app.expansions.length - was) + "|" + (app.pinned ? app.pinned.id : "");
    drag(document.getElementById("chart"), 120);
    t.diagnostic(`dragged 300px left from a ribbon: scrollLeft ${panned}; 120px back from the background: ${left()}`);
    assert.deepEqual({ panned, swallowed, back: left(), scroller: scroller.classList.contains("chart-scroll") },
      { panned: 300, swallowed: "0|", back: 180, scroller: true });
  });

  // Mutation: set swallow at a drag's end whatever ended it, and clear it
  // only at the next pointerdown or click, and this goes red: both later
  // clicks are swallowed and pin nothing.
  test("a drag swallows only its own click: not one a task later, and none after a cancelled pointer", async (t) => {
    const { app, document } = await scrolling();
    const ribbon = document.querySelector("#chart path.link");
    const id = "fund-group/general";
    const pinnedAfter = () => (app.pinned ? app.pinned.id : "");
    drag(ribbon, -300);
    await new Promise((r) => setTimeout(r, 5));
    fire(markOf(document, id), "click", { timeStamp: 1000 });
    await settle();
    const later = pinnedAfter();
    app.unpin();
    fire(ribbon, "pointerdown", { button: 0, clientX: 500, clientY: 300 });
    fire(ribbon, "pointermove", { button: 0, clientX: 200, clientY: 300 });
    fire(ribbon, "pointercancel", { button: 0, clientX: 200, clientY: 300 });
    fire(markOf(document, id), "click", { timeStamp: 5000 });
    await settle();
    const cancelled = pinnedAfter();
    t.diagnostic(`a click a task after a drag pinned "${later}"; one after a cancelled drag "${cancelled}"`);
    assert.deepEqual({ later, cancelled }, { later: id, cancelled: id });
  });

  test("a click after a press that moves under DRAG_SLOP still opens the mark", async (t) => {
    const { app, document, left } = await scrolling();
    app.collapseAll();
    await settle();
    const id = "fund-group/general";
    drag(markOf(document, id), app.DRAG_SLOP - 1);
    fire(markOf(document, id), "click", { timeStamp: 1000 });
    await settle();
    t.diagnostic(`moved ${app.DRAG_SLOP - 1}px then clicked: split [${app.expansions}], scrollLeft ${left()}`);
    assert.deepEqual({ split: app.expansions.slice(), left: left() }, { split: [id], left: 0 });
  });

  /**
   * A press on `on` as a browser delivers it: pointerdown, then mousedown,
   * whose default, uncancelled, focuses the mark pressed.
   */
  function press(on) {
    fire(on, "pointerdown", { button: 0, clientX: 500, clientY: 300 });
    if (!fire(on, "mousedown", { button: 0, clientX: 500, clientY: 300 }).defaultPrevented) on.focus();
  }

  // Mutation: answer `unasked` from restoring alone, so a pointer's focus
  // pins, and this goes red: the drag pins the ribbon it started on.
  test("a drag that starts on a mark focuses it and leaves the pin as it was, and a mark focused from the keyboard still pins", async (t) => {
    const { app, document } = await scrolling();
    const [ribbon, other] = document.querySelectorAll("#chart path.link");
    press(ribbon);
    fire(ribbon, "pointermove", { button: 0, clientX: 200, clientY: 300 });
    fire(ribbon, "pointerup", { button: 0, clientX: 200, clientY: 300 });
    fire(ribbon, "click", { timeStamp: 1000 });
    const dragged = { focused: document.activeElement === ribbon, pinned: app.pinned !== null };
    // The keyboard moves focus by a key, which ends the press.
    fire(other, "keydown", { key: "Tab" });
    other.focus();
    const keyed = app.pinned !== null && app.pinned === other.__data__;
    t.diagnostic(`the drag focused its ribbon ${dragged.focused}, pinned ${dragged.pinned}; the keyboard's focus pinned ${keyed}`);
    assert.deepEqual({ dragged, keyed }, { dragged: { focused: true, pinned: false }, keyed: true });
  });

  // Mutation: cancel the press's mousedown in wirePan again, and this goes
  // red: focus stays in the search box, so its list stays open and the
  // click leaves the ribbon unfocused.
  test("a click on a mark moves focus to it, which closes the search list a reader typed into", async (t) => {
    const { app, document } = await scrolling();
    const input = document.getElementById("search-input");
    input.focus();
    input.value = "fund";
    input.dispatchEvent(new globalThis.Event("input", { bubbles: true }));
    const list = document.getElementById("search-list");
    const opened = !list.hasAttribute("hidden");
    const ribbon = document.querySelector("#chart path.link");
    press(ribbon);
    fire(ribbon, "pointerup", { button: 0, clientX: 500, clientY: 300 });
    fire(ribbon, "click", { timeStamp: 1000 });
    const after = { focused: document.activeElement === ribbon, open: !list.hasAttribute("hidden"),
      pinned: app.pinned !== null && app.pinned === ribbon.__data__ };
    t.diagnostic(`list opened ${opened}; after the click: ${JSON.stringify(after)}`);
    assert.deepEqual({ opened, after }, { opened: true, after: { focused: true, open: false, pinned: true } });
  });

  // Mutation: reveal every mark focused in the chart, `pointing` or not
  // (wireZoom's focusin), and this goes red: the member the split focuses
  // scrolls the chart away from the mark the click held in place.
  test("a click that splits a mark leaves the scroller where the split held the mark", async (t) => {
    const { app, document, svg, scroller, left } = await scrolling();
    app.collapseAll();
    await settle();
    // The rightmost mark that splits, its members laid further right still,
    // with the scroller put where the mark is in sight at its card's right edge.
    const d = app.laidNodes.filter((n) => app.splittable(n)).reduce((a, b) => (b.x0 > a.x0 ? b : a));
    const id = d.id;
    const scale = svg.getBoundingClientRect().width / viewBoxWidth(document);
    const from = Math.max(0, Math.round(d.x1 * scale) - 1000 + 20);
    // Where the split alone holds the scroller: opened with focus outside the chart.
    scroller.scrollLeft = from;
    app.expandNode(id);
    await settle();
    const held = left();
    app.collapseAll();
    await settle();
    scroller.scrollLeft = from;
    const mark = markOf(document, id);
    press(mark);
    fire(mark, "pointerup", { button: 0, clientX: 500, clientY: 300 });
    fire(mark, "click", { timeStamp: 1000 });
    await settle();
    const focused = document.activeElement && document.activeElement.__data__ ? document.activeElement.__data__.id : "";
    t.diagnostic(`split ${id} from scrollLeft ${from} with no focus in the chart: scrollLeft ${held}; by a click: ${left()}, focus on ${focused}`);
    assert.ok(focused !== "" && focused !== id, "the click's split moved focus to no member, so this asserts nothing");
    assert.deepEqual({ split: app.expansions.slice(), left: left() }, { split: [id], left: held });
  });

  // Mutation: hide the tooltip on every mark's blur (the marks' hide hook
  // hideTip again), and this goes red: the press on B blurs A and hides
  // B's tooltip under the pointer.
  test("a press moving focus from one mark to another keeps the pointer's tooltip, and a blur hides one the keyboard's focus showed", async (t) => {
    const { document } = await scrolling();
    const [a, b] = document.querySelectorAll("#chart path.link");
    const tip = document.getElementById("tooltip");
    const at = { button: 0, clientX: 500, clientY: 300 };
    fire(a, "pointerenter", at);
    press(a);
    fire(a, "pointerup", at);
    fire(a, "click", { timeStamp: 1000 });
    fire(a, "pointerleave", at);
    fire(b, "pointerenter", at);
    press(b);
    const pressed = !tip.hidden;
    fire(b, "pointerup", at);
    fire(b, "pointerleave", at);
    fire(a, "keydown", { key: "Tab" });
    a.focus();
    const keyed = !tip.hidden;
    document.getElementById("search-input").focus();
    const blurred = !tip.hidden;
    t.diagnostic(`tooltip shown after pressing B ${pressed}; on A's keyboard focus ${keyed}; after A's blur ${blurred}`);
    assert.deepEqual({ pressed, keyed, blurred }, { pressed: true, keyed: true, blurred: false });
  });

  // Mutation: clear `pointing` on pointerup and pointercancel alone (no
  // keydown or contextmenu arm in wireZoom), and this goes red: the
  // keyboard's focus after a press whose release never came pins nothing.
  test("a press whose release the page never hears leaves the keyboard's focus pinning", async (t) => {
    const { app, document, window } = await scrolling();
    const [ribbon, other, third, fourth] = document.querySelectorAll("#chart path.link");
    const pins = (mark) => mark.__data__ === app.pinned;
    fire(ribbon, "pointerdown", { button: 0, clientX: 500, clientY: 300 });
    fire(other, "keydown", { key: "Tab" });
    other.focus();
    const afterKey = pins(other);
    fire(ribbon, "pointerdown", { button: 0, clientX: 500, clientY: 300 });
    window.dispatchEvent(new window.Event("blur"));
    third.focus();
    const afterBlur = pins(third);
    // A right press opens the browser's context menu, which takes its release.
    fire(ribbon, "pointerdown", { button: 2, clientX: 500, clientY: 300 });
    fire(ribbon, "contextmenu", { button: 2, clientX: 500, clientY: 300 });
    await new Promise((r) => setTimeout(r, 0));
    fourth.focus();
    const afterRight = pins(fourth);
    t.diagnostic(`focus pinned after a keydown ${afterKey}, after the window's blur ${afterBlur}, after a right press ${afterRight}`);
    assert.deepEqual({ afterKey, afterBlur, afterRight }, { afterKey: true, afterBlur: true, afterRight: true });
  });

  // Mutation: listen for the press's end on the scroller alone and this goes
  // red: the release outside it leaves the press live, and the hover pans.
  test("a press released outside the scroller ends there, and a hover after it pans nothing", async (t) => {
    const { document, left } = await scrolling();
    const ribbon = document.querySelector("#chart path.link");
    fire(ribbon, "pointerdown", { button: 0, clientX: 500, clientY: 300 });
    fire(ribbon, "pointermove", { button: 0, clientX: 501, clientY: 900 });
    fire(document.body, "pointerup", { button: 0, clientX: 501, clientY: 900 });
    fire(ribbon, "pointermove", { buttons: 0, clientX: 200, clientY: 300 });
    t.diagnostic(`released below the scroller, then hovered 300px left: scrollLeft ${left()}`);
    assert.equal(left(), 0);
  });

  // Mutation: drop the target test from wirePan's pointerdown and this goes
  // red: the press on the scrollbar pans the scroller under the thumb.
  test("a press on the scroller's own scrollbar is the scrollbar's", async (t) => {
    const { scroller, left } = await scrolling();
    drag(scroller, -300);
    t.diagnostic(`dragged 300px on the scroller itself: scrollLeft ${left()}`);
    assert.equal(left(), 0);
  });

  // Mutation: cancel the press's mousedown in wirePan again, and this goes
  // red: the press moves no focus out of the menu, which stays open.
  test("a press on the chart closes the ⊕ menu, by moving focus out of it", async (t) => {
    const { app, document } = await bootedApp();
    const id = "expenditure/capital-outlay";
    const glyph = [...document.querySelectorAll("#chart g.glyph")].find((g) => g.__data__ && g.__data__.id === id);
    assert.ok(glyph !== undefined, "no glyph beside " + id);
    fire(glyph, "click", { timeStamp: 2000 });
    const opened = Boolean(document.getElementById("split-menu"));
    const ribbon = document.querySelector("#chart path.link");
    press(ribbon);
    const open = Boolean(document.getElementById("split-menu"));
    fire(ribbon, "pointerup", { button: 0, clientX: 500, clientY: 300 });
    t.diagnostic(`menu opened ${opened}; menu open after the press ${open}`);
    assert.deepEqual({ opened, open, menu: app.chartMenu === null }, { opened: true, open: false, menu: true });
  });

  test("zoomed in, a drag is the zoom's to pan, and the scroller stays where it is", async (t) => {
    const { app, document, left } = await scrolling();
    app.zoomBy(2);
    drag(document.querySelector("#chart path.link"), -300);
    t.diagnostic(`at k ${app.zoomOf().k}, a 300px drag left scrollLeft at ${left()}`);
    assert.equal(left(), 0);
  });

  // Mutation: hand --chart-max widthOf(columns) and --chart-stretch 1 again,
  // scaling the chart to fit its card, and this goes red: the split shrinks
  // every mark and draws the fund groups nearer the revenue column.
  test("opening a split moves no column it leaves and rescales nothing", async (t) => {
    const viewport = 1400;
    const { app, document, window } = await bootedApp({ viewport });
    Object.defineProperty(window, "innerWidth", { value: viewport, configurable: true, writable: true });
    // From the node opened: a source's members stand before it, and the
    // scroller, not the canvas, takes up the column they add.
    const opened = "revenue/intergovernmental";
    const where = (id) => {
      const x = (at) => app.laidNodes.find((n) => n.id === at).x0;
      const s = drawnScale(document, window);
      return s.refused ? s : { scale: s.scale, x: (x(id) - x(opened)) * s.scale };
    };
    const before = where("fund-group/general");
    await split(app, opened);
    const after = where("fund-group/general");
    const detail = `at a ${viewport}px viewport, fund-group/general from ${opened} before the split ${JSON.stringify(before)}, ` +
      `after ${JSON.stringify(after)}; ${app.layoutColumns(app.shown.doc)} columns`;
    t.diagnostic(detail);
    assert.deepEqual({ after, drawn: before.x > 0 }, { after: before, drawn: true }, detail);
  });

  // fisc-ylxh.7. Mutation: let a plain wheel zoom in full screen again and
  // this goes red there.
  test("a wheel zooms only with Ctrl or Cmd, in full screen too, and one finger or the mouse pans the zoom only once zoomed", async () => {
    const loaded = await loadApp({ viewport: 1000 });
    const { app, window, document } = loaded;
    stubFullscreen(window, document, 4000);
    await app.boot();
    await settle();
    /** What d3.zoom takes, as [plain wheel, sideways wheel, Ctrl wheel, Cmd wheel, one finger, two, mouse, right button]. */
    const takes = () => [
      app.zoomFilter({ type: "wheel" }), app.zoomFilter({ type: "wheel", deltaX: 40 }),
      app.zoomFilter({ type: "wheel", ctrlKey: true }), app.zoomFilter({ type: "wheel", metaKey: true }),
      app.zoomFilter({ type: "touchstart", touches: [1] }), app.zoomFilter({ type: "touchstart", touches: [1, 2] }),
      app.zoomFilter({ type: "mousedown", button: 0 }), app.zoomFilter({ type: "mousedown", button: 2 }),
    ];
    const whole = takes();
    fire(document.getElementById("chart-fullscreen"), "click");
    await settle();
    const full = { on: app.isFullscreen(), takes: takes() };
    app.zoomBy(2);
    const zoomed = takes();
    const want = [false, false, true, true, false, true, false, false];
    assert.deepEqual({ whole, full, zoomed },
      { whole: want, full: { on: true, takes: want }, zoomed: [false, false, true, true, true, true, true, false] });
  });
});

describe("the toolbar's buttons", () => {
  // Mutation: drop the .chart-tools disabled rule and this goes red: a
  // disabled button takes button's pointer and button:hover's wash.
  test("disabled, show no pointer, no hover wash and a faded face", () => {
    const css = stylesheet().replace(/\/\*[\s\S]*?\*\//g, "");
    const rules = [...css.matchAll(/([^{}]+)\{([^}]*)\}/g)].map((m) => ({
      selectors: m[1].split(",").map((x) => x.trim()), body: m[2],
    }));
    const of = (selector) => rules.filter((r) => r.selectors.includes(selector)).map((r) => r.body).join(";");
    const disabled = of(".chart-tools button:disabled");
    const hovered = of(".chart-tools button:disabled:hover");
    assert.deepEqual({
      cursor: /cursor:\s*default\s*;/.test(disabled), faded: /opacity:\s*0?\.5\s*;/.test(disabled),
      unwashed: /background:\s*var\(--surface-1\)\s*;/.test(hovered),
    }, { cursor: true, faded: true, unwashed: true });
  });
});
