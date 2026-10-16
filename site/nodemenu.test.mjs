// nodemenu.test.mjs -- a node's context menu: a right-click, Shift+F10 or
// the ContextMenu key on a node (the contextmenu event the browser fires on
// the focused mark for either key) opens it, and its three choices follow the
// node's money, filter the node out, and show only its money. What the two
// filters remove is said on the filter line by name, Reset puts it back, and
// the hash carries it. Every assertion is over ids, strings and booleans.

import { describe, test } from "node:test";
import assert from "node:assert/strict";
import { bootedApp, loadApp, split, settle, fire, refusals, dollars } from "./testlib.mjs";

const OPENS = "fund-group/general";

/** The drawn mark for one node id, or a throw. */
function markOf(document, id) {
  const m = [...document.querySelectorAll("#chart g.node")].find((g) => g.__data__ && g.__data__.id === id);
  if (!m) throw new Error(`${id} is not drawn`);
  return m;
}

/**
 * A drawn node that is neither a fund group nor a mark of ours, with
 * ribbons both in and out where one has them: what the menu's choices are
 * tried on. The first such by id, so the choice is stable.
 */
function plainNode(app) {
  const plain = app.laidNodes.filter((n) => !app.isGroupID(n.id) && !app.isSmaller(n.id) &&
    (n.sourceLinks.length + n.targetLinks.length) > 0).sort((a, b) => (a.id < b.id ? -1 : 1));
  const both = plain.find((n) => n.sourceLinks.length && n.targetLinks.length);
  return (both || plain[0]).id;
}

/** The ids of the menu's buttons' choices, in order; [] with no menu open. */
function menuActs(document) {
  const menu = document.getElementById("node-menu");
  return menu ? [...menu.querySelectorAll("button")].map((b) => b.dataset.act) : [];
}

/** Clicks one choice of the node menu open now. */
function choose(document, act) {
  const b = [...document.querySelectorAll("#node-menu button")].find((x) => x.dataset.act === act);
  if (!b) throw new Error(`the node menu offers no ${act}`);
  b.click();
}

/** Every drawn ribbon's two ends, as "source -> target". */
function drawnRibbons(document) {
  return [...document.querySelectorAll("#chart path.link")].map((p) => p.__data__.source.id + " -> " + p.__data__.target.id);
}

const pinnedID = (app) => (app.pinned && typeof app.pinned.id === "string" ? app.pinned.id : "");

describe("a node's context menu", () => {
  // Mutation: drop the contextmenu binding on g.node in sankey.js and the
  // right-click and the keys open nothing; open the menu from a keydown arm
  // for ContextMenu or Shift+F10 too, and the key's own keydown opens it,
  // so on Windows the contextmenu its keyup fires opens the browser's over it.
  test("opens on a right-click and on the keys' contextmenu with three choices, focus on the first; Escape closes it onto the node, pinning nothing; a keydown opens nothing", async (t) => {
    const { app, document } = await bootedApp();
    const id = plainNode(app);
    const mark = markOf(document, id);
    const right = fire(mark, "contextmenu", { button: 2, clientX: 40, clientY: 40 });
    const acts = menuActs(document);
    const menu = document.getElementById("node-menu");
    const named = menu ? menu.getAttribute("aria-label") : "";
    const focused = Boolean(menu) && document.activeElement === menu.querySelector("button");
    const words = menu ? [...menu.querySelectorAll("button")].map((b) => b.textContent) : [];
    fire(menu, "keydown", { key: "Escape" });
    const closed = !document.getElementById("node-menu");
    const back = document.activeElement === mark;
    const pinnedAfterEscape = pinnedID(app);
    fire(mark, "keydown", { key: "F10", shiftKey: true });
    fire(mark, "keydown", { key: "ContextMenu" });
    const byKeydown = menuActs(document);
    app.closeChartMenu();
    // A key's contextmenu, as a browser fires it on the focused mark: at 0, 0.
    const keyed = fire(mark, "contextmenu", { button: 0, clientX: 0, clientY: 0 });
    const byKey = menuActs(document);
    app.closeChartMenu();
    t.diagnostic(`${id}: the menu "${named}" offers ${JSON.stringify(words)}`);
    assert.equal(right.defaultPrevented, true, "the browser's own menu opened over the node's");
    assert.deepEqual(acts, ["follow", "out", "only"]);
    assert.deepEqual(words, [app.say("follow_money"), app.say("filter_out_node"), app.say("show_only_node")]);
    assert.equal(named, app.say("node_menu", { label: app.nameOf(app.laidNodes.find((n) => n.id === id)) }));
    assert.ok(focused, "the menu opened without focus on its first choice");
    assert.ok(closed, "Escape in the menu left it open");
    assert.ok(back, "Escape did not put focus back on the node");
    assert.equal(pinnedAfterEscape, "", "focus going back to the node pinned it");
    assert.deepEqual(byKeydown, [], "a keydown opened the menu the key's contextmenu opens");
    assert.equal(keyed.defaultPrevented, true);
    assert.deepEqual(byKey, ["follow", "out", "only"]);
  });

  // Mutation: tell a key's contextmenu by 0, 0 alone (no keyedMenu) and
  // this goes red: the keyed menu opens at the coordinates Chromium gives
  // the event, where the pointer would have been.
  test("a menu key's contextmenu at the mark's coordinates opens the menu beside the mark; a right-click opens it at the pointer", async (t) => {
    const { app, document } = await bootedApp();
    const m = markOf(document, plainNode(app));
    const placed = () => {
      const menu = document.getElementById("node-menu");
      const at = menu ? menu.style.left + " " + menu.style.top : "";
      app.closeChartMenu();
      return at;
    };
    fire(m, "contextmenu", { button: 2, clientX: 100, clientY: 200 });
    const pointer = placed();
    fire(m, "keydown", { key: "ContextMenu" });
    fire(m, "contextmenu", { button: 0, clientX: 100, clientY: 200 });
    const contextMenuKey = placed();
    fire(m, "keydown", { key: "F10", shiftKey: true });
    fire(m, "contextmenu", { button: 0, clientX: 100, clientY: 200 });
    const shiftF10 = placed();
    fire(m, "contextmenu", { button: 2, clientX: 100, clientY: 200 });
    const after = placed();
    t.diagnostic(`pointer ${pointer}; ContextMenu ${contextMenuKey}; Shift+F10 ${shiftF10}; a right-click after ${after}`);
    assert.deepEqual({ pointer, after }, { pointer: "100px 200px", after: "100px 200px" });
    assert.ok(contextMenuKey !== "" && contextMenuKey !== pointer, contextMenuKey);
    assert.ok(shiftF10 !== "" && shiftF10 !== pointer, shiftF10);
  });

  test("a ribbon has no menu: a right-click on one leaves the browser's own", async () => {
    const { document } = await bootedApp();
    const ribbon = document.querySelector("#chart path.link");
    const e = fire(ribbon, "contextmenu", { button: 2, clientX: 40, clientY: 40 });
    assert.equal(e.defaultPrevented, false);
    assert.equal(document.getElementById("node-menu"), null);
  });

  // Mutation: make the Follow choice do nothing and this goes red; close the
  // menu on a choice without refocus and focus drops to the body.
  test("Follow this money follows the node and pins it, so the drawer's Follow is pressed", async () => {
    const { app, document } = await bootedApp();
    const id = plainNode(app);
    fire(markOf(document, id), "contextmenu", { button: 2, clientX: 40, clientY: 40 });
    choose(document, "follow");
    assert.equal(document.getElementById("node-menu"), null);
    assert.ok(document.activeElement === markOf(document, id), "focus did not go back to the node followed");
    assert.equal(app.filter.isolated, id);
    assert.equal(pinnedID(app), id);
    assert.equal(document.getElementById("detail-follow").getAttribute("aria-pressed"), "true");
    assert.equal(markOf(document, id).getAttribute("aria-pressed"), "true");
  });

  // Mutation: make the Filter out choice do nothing, or drop the nodes arm
  // from core.filterDocument, and this goes red.
  test("Filter this out removes every ribbon with an end at the node, the line names it, a neighbour's figure stays the view's, and Reset puts it back", async (t) => {
    const { app, document } = await bootedApp();
    const id = plainNode(app);
    const node = app.laidNodes.find((n) => n.id === id);
    const name = app.nameOf(node);
    const neighbour = (node.sourceLinks[0] || node.targetLinks[0]);
    const other = neighbour.source.id === id ? neighbour.target.id : neighbour.source.id;
    const figure = app.markFigure(app.laidNodes.find((n) => n.id === other));
    const before = drawnRibbons(document).length;
    const expected = app.projection.links.filter((l) => l.source === id || l.target === id);
    fire(markOf(document, id), "contextmenu", { button: 2, clientX: 40, clientY: 40 });
    choose(document, "out");
    const on = document.activeElement;
    assert.ok(on && on.matches && on.matches("#chart g.node"), "focus left the chart when its node was filtered out");
    const after = drawnRibbons(document);
    const said = document.getElementById("filter-said").textContent;
    const tally = app.hiddenTally();
    t.diagnostic(`${id}: ${before} ribbon(s) drawn, ${after.length} after; the line says: ${said}`);
    assert.deepEqual([...app.filter.nodes], [id]);
    assert.ok(!app.laidNodes.some((n) => n.id === id), "the node filtered out is still drawn");
    assert.equal(after.length, before - expected.length);
    assert.equal(tally.flows, expected.length);
    assert.ok(said.startsWith(app.say("filter_out", { names: name })), said);
    assert.ok(said.includes(app.say("filter_hidden", { cents: dollars(tally.cents), flows: tally.flows })), said);
    const kept = app.laidNodes.find((n) => n.id === other);
    if (kept) assert.equal(app.markFigure(kept), figure, `${other}'s figure moved with the filter`);
    assert.equal(refusals(document).length, 0);
    fire(document.getElementById("filter-reset"), "click");
    assert.equal(app.filter.nodes.size, 0);
    assert.equal(drawnRibbons(document).length, before);
    assert.equal(document.getElementById("filter-line").hasAttribute("hidden"), true);
  });

  // Mutation: make the Show only choice do nothing, or return null from
  // onlyKeeps always, and this goes red.
  test("Show only this draws only the ribbons Follow leaves undimmed, the line names the node, and Reset puts the rest back", async (t) => {
    const { app, document } = await bootedApp();
    const id = plainNode(app);
    const name = app.nameOf(app.laidNodes.find((n) => n.id === id));
    const before = drawnRibbons(document).length;
    fire(markOf(document, id), "contextmenu", { button: 2, clientX: 40, clientY: 40 });
    choose(document, "only");
    assert.ok(document.activeElement === markOf(document, id), "focus did not go back to the node shown alone");
    const drawn = [...document.querySelectorAll("#chart path.link")].map((p) => p.__data__);
    const astray = drawn.filter((l) => !app.follows(id, l)).map((l) => l.source.id + " -> " + l.target.id);
    const said = document.getElementById("filter-said").textContent;
    t.diagnostic(`${id}: ${before} ribbon(s) drawn, ${drawn.length} shown alone; the line says: ${said}`);
    assert.equal(app.filter.only, id);
    assert.ok(drawn.length > 0 && drawn.length < before, `showing ${id} alone drew ${drawn.length} of ${before}`);
    assert.deepEqual(astray, []);
    assert.equal(drawn.length, app.projection.links.filter((l) => l.source === id || l.target === id).length);
    assert.ok(said.startsWith(app.say("filter_only", { label: name })), said);
    assert.equal(refusals(document).length, 0);
    fire(document.getElementById("filter-reset"), "click");
    assert.equal(app.filter.only, "");
    assert.equal(drawnRibbons(document).length, before);
  });
});

describe("the node filters beside the others", () => {
  // Mutation: let dropPin keep a node's isolation and this goes red: the
  // chart follows a node the drawer no longer shows.
  test("filtering out or showing alone lets a followed node's pin and isolation go together; filtering out the node shown alone puts the rest back", async () => {
    const { app, document } = await bootedApp();
    const id = plainNode(app);
    app.followNode(id);
    assert.equal(app.filter.isolated, id);
    app.filterOutNode(id);
    assert.deepEqual({ isolated: app.filter.isolated, pinned: pinnedID(app), out: [...app.filter.nodes] },
      { isolated: "", pinned: "", out: [id] });
    app.resetFilters();
    const other = app.laidNodes.find((n) => n.id !== id && !app.isGroupID(n.id)).id;
    app.followNode(other);
    app.showOnlyNode(id);
    assert.deepEqual({ isolated: app.filter.isolated, pinned: pinnedID(app) }, { isolated: "", pinned: "" });
    app.filterOutNode(id);
    assert.deepEqual({ only: app.filter.only, out: [...app.filter.nodes] }, { only: "", out: [id] });
    app.showOnlyNode(id);
    assert.deepEqual({ only: app.filter.only, out: [...app.filter.nodes] }, { only: id, out: [] });
    assert.equal(refusals(document).length, 0);
  });

  test("showing a node alone, then filtering out a neighbour, removes that neighbour's ribbon too and names both", async () => {
    const { app, document } = await bootedApp();
    const id = plainNode(app);
    app.showOnlyNode(id);
    const node = app.laidNodes.find((n) => n.id === id);
    const ribbons = node.sourceLinks.concat(node.targetLinks);
    const l = ribbons[0];
    const other = l.source.id === id ? l.target : l.source;
    const otherName = app.nameOf(other);
    const before = ribbons.length;
    app.filterOutNode(other.id);
    const said = document.getElementById("filter-said").textContent;
    assert.equal(app.filter.only, id);
    assert.ok(drawnRibbons(document).length < before, "the neighbour's ribbon is still drawn");
    assert.ok(said.includes(app.say("filter_out", { names: otherName })), said);
    assert.ok(said.includes(app.say("filter_only", { label: app.nameOf(node) })), said);
  });

  // Mutation: decide onlyKeeps against the shaped chart alone, before the
  // other removals, and this goes red: `only` removes every ribbon the
  // others leave, an empty chart or a refusal.
  test("a node shown alone is inert where the other removals leave none of its ribbons: filtered out too, or in a hidden group", async (t) => {
    const { app, document } = await bootedApp();
    const id = plainNode(app);
    const before = drawnRibbons(document).length;
    const at = (x) => app.projection.links.filter((l) => l.source === x || l.target === x).length;
    const label = (x) => app.say("filter_only", { label: app.nameOf(app.laidNodes.find((n) => n.id === x)) });
    const outLabel = label(id);
    const out = { ok: app.setFilter({ nodes: new Set([id]), only: id }), drawn: drawnRibbons(document).length,
      said: document.getElementById("filter-said").textContent };
    app.resetFilters();
    const groupLabel = label(OPENS);
    const group = { ok: app.setFilter({ groups: new Set([OPENS]), only: OPENS }), drawn: drawnRibbons(document).length,
      said: document.getElementById("filter-said").textContent };
    t.diagnostic(`${id} out and alone: ${out.drawn} of ${before}; ${OPENS} hidden and alone: ${group.drawn} of ${before}`);
    assert.deepEqual({ ok: out.ok, drawn: out.drawn, named: out.said.includes(outLabel) },
      { ok: true, drawn: before - at(id), named: false });
    assert.deepEqual({ ok: group.ok, drawn: group.drawn, named: group.said.includes(groupLabel) },
      { ok: true, drawn: before - at(OPENS), named: false });
    assert.equal(refusals(document).length, 0);
  });

  // Mutation: drop setFilter's isolate after the redraw and this goes red:
  // the isolation passed with a removal goes with the pin the redraw drops,
  // and setFilter still says the state on screen is the one asked for.
  test("an isolation passed with a removal follows the node and pins it", async () => {
    const { app, document } = await bootedApp();
    const id = plainNode(app);
    const node = app.laidNodes.find((n) => n.id === id);
    const near = new Set(node.sourceLinks.map((l) => l.target.id).concat(node.targetLinks.map((l) => l.source.id), [id]));
    const far = app.laidNodes.find((n) => !near.has(n.id) && !app.isGroupID(n.id) && !app.isSmaller(n.id) &&
      app.projection.nodes.some((p) => p.id === n.id));
    assert.ok(far, `every node is ${id}'s neighbour`);
    const ok = app.setFilter({ nodes: new Set([far.id]), isolated: id });
    assert.deepEqual({ ok, isolated: app.filter.isolated, pinned: pinnedID(app),
      pressed: document.getElementById("detail-follow").getAttribute("aria-pressed") },
    { ok: true, isolated: id, pinned: id, pressed: "true" });
  });

  test("a node filtered out stays out through a split, and one shown alone is inert where the chart draws none of it", async () => {
    const { app, document } = await bootedApp();
    const id = plainNode(app);
    app.filterOutNode(id);
    await split(app, OPENS);
    assert.deepEqual([...app.filter.nodes], [id]);
    assert.ok(!drawnRibbons(document).some((r) => r.split(" -> ").includes(id)), `${id} draws a ribbon after the split`);
    app.resetFilters();
    const drawn = drawnRibbons(document).length;
    app.setFilter({ only: "no/such-node" });
    assert.equal(app.filter.only, "no/such-node");
    assert.equal(drawnRibbons(document).length, drawn);
    assert.equal(document.getElementById("filter-line").hasAttribute("hidden"), true);
    assert.equal(refusals(document).length, 0);
  });
});

describe("the menu and the pin", () => {
  // Mutation: offer Filter out and Show only on every mark (openNodeMenu's
  // concat unconditional) and this goes red: a smaller-flows mark offers two
  // choices that change the hash and draw nothing new.
  test("a smaller-flows mark offers Follow alone: the filters remove ribbons of the shaped chart's nodes", async (t) => {
    const { app, document } = await bootedApp();
    app.setFilter({ smallest: app.SMALLEST_STOPS[3] });
    const smaller = app.laidNodes.find((n) => app.isSmaller(n.id));
    assert.ok(smaller, "the smallest-flow filter drew no smaller-flows mark");
    const id = smaller.id;
    fire(markOf(document, id), "contextmenu", { button: 2, clientX: 40, clientY: 40 });
    const acts = menuActs(document);
    app.closeChartMenu();
    t.diagnostic(`${id} offers ${JSON.stringify(acts)}`);
    assert.deepEqual(acts, ["follow"]);
  });

  // Mutation: make redraw or setFilter null the pin alone (no dropPin) and
  // this goes red: a split and a filter each leave the chart following a
  // node the drawer no longer shows.
  test("a split or a filter that drops the pin lets a followed node go with it, and a fund group's isolation stays", async () => {
    const { app } = await bootedApp();
    const id = plainNode(app);
    app.followNode(id);
    await split(app, OPENS);
    assert.ok(app.laidNodes.some((n) => n.id === id), `${id} is not drawn after the split, so this asserts nothing`);
    const split_ = { isolated: app.filter.isolated, pinned: pinnedID(app) };
    app.followNode(id);
    app.setFilter({ smallest: app.SMALLEST_STOPS[1] });
    const filtered = { isolated: app.filter.isolated, pinned: pinnedID(app) };
    app.setIsolated(OPENS);
    app.setFilter({ smallest: 0 });
    assert.deepEqual({ split_, filtered, group: app.filter.isolated },
      { split_: { isolated: "", pinned: "" }, filtered: { isolated: "", pinned: "" }, group: OPENS });
  });

  // Mutation: set `pointing` on a primary press alone (wireZoom's
  // pointerdown) and this goes red: the right press's focus pins the node,
  // and pin's rule lets the node followed go before the menu opens.
  test("a right press focuses a node without pinning it, so the node followed stays followed under its menu", async () => {
    const { app, document } = await bootedApp();
    const id = plainNode(app);
    const other = app.laidNodes.find((n) => n.id !== id && !app.isGroupID(n.id) && !app.isSmaller(n.id)).id;
    app.followNode(other);
    const mark = markOf(document, id);
    fire(mark, "pointerdown", { button: 2, clientX: 40, clientY: 40 });
    mark.focus();
    fire(mark, "contextmenu", { button: 2, clientX: 40, clientY: 40 });
    const acts = menuActs(document);
    const state = { pinned: pinnedID(app), isolated: app.filter.isolated };
    app.closeChartMenu();
    fire(document.body, "pointerup", { button: 2 });
    await settle();
    assert.deepEqual(acts, ["follow", "out", "only"]);
    assert.deepEqual(state, { pinned: other, isolated: other });
  });

  // Mutation: name a node filtered out by nameOf alone (no outNames) and
  // this goes red: after the next render the line names it by its bare label.
  test("a node filtered out is named on the filter line as it was drawn, after later renders too", async (t) => {
    const { app, document } = await bootedApp();
    app.expandAll();
    await settle();
    const named = app.projection.nodes.map((n) => app.laidNodes.find((l) => l.id === n.id))
      .find((n) => n && !app.isGroupID(n.id) && app.nameOf(n) !== n.label);
    assert.ok(named, "no drawn node is named other than by its label");
    const name = app.nameOf(named);
    app.filterOutNode(named.id);
    const next = app.laidNodes.find((n) => n.id !== named.id && !app.isGroupID(n.id) && !app.isSmaller(n.id) &&
      app.projection.nodes.some((p) => p.id === n.id));
    app.filterOutNode(next.id);
    const said = document.getElementById("filter-said").textContent;
    t.diagnostic(`${named.id} drawn as ${JSON.stringify(name)}, labelled ${JSON.stringify(named.label)}; the line says: ${said}`);
    assert.ok(said.includes(name), said);
    assert.equal(refusals(document).length, 0);
  });
});

describe("the node filters in the URL", () => {
  // Mutation: drop `out` or `only` from viewState and the reloaded page
  // draws the whole chart.
  test("a node filtered out and a node shown alone round-trip through the hash", async (t) => {
    const { app, window } = await bootedApp();
    const id = plainNode(app);
    const node = app.laidNodes.find((n) => n.id === id);
    const l = node.sourceLinks.concat(node.targetLinks)[0];
    const other = l.source.id === id ? l.target.id : l.source.id;
    app.showOnlyNode(id);
    app.filterOutNode(other);
    const hash = window.location.hash;
    const want = { out: [other], only: id, links: app.shown.doc.links.length };
    const again = await loadApp();
    again.window.history.replaceState(null, "", hash);
    await again.app.boot();
    await settle();
    t.diagnostic(hash);
    assert.equal(hash, app.hashOf(app.viewState()));
    assert.ok(hash.includes("only=") && hash.includes("out="), hash);
    assert.deepEqual({ out: [...again.app.filter.nodes], only: again.app.filter.only, links: again.app.shown.doc.links.length }, want);
    assert.deepEqual(refusals(again.document).map((b) => b.textContent), []);
  });

  // Mutation: restore moveTo's bare setIsolated for a node and this goes
  // red: the chart follows a node with nothing pinned and Follow hidden.
  test("a link following a node pins it, so the drawer shows it with Follow pressed; a group's is the swatch's", async () => {
    const { app, document } = await bootedApp();
    const id = plainNode(app);
    const node = await app.restoreView(app.hashOf(Object.assign({}, app.viewState(), { iso: id })));
    const follow = document.getElementById("detail-follow");
    const followed = { got: node, isolated: app.filter.isolated, pinned: pinnedID(app), hidden: follow.hidden,
      pressed: follow.getAttribute("aria-pressed") };
    app.unpin();
    const group = await app.restoreView(app.hashOf(Object.assign({}, app.viewState(), { iso: OPENS })));
    assert.deepEqual({ followed, group: { got: group, isolated: app.filter.isolated, pinned: pinnedID(app) } }, {
      followed: { got: app.DREW, isolated: id, pinned: id, hidden: false, pressed: "true" },
      group: { got: app.DREW, isolated: OPENS, pinned: "" },
    });
  });

  // Mutation: drop `out` and `only` from restoreView's fallback and this
  // goes red: the refused link on the year on screen leaves the node shown
  // alone under a banner saying the opening view is shown.
  test("a refused link on the year on screen shows the opening view, no node filtered out or shown alone", async () => {
    const { app, document } = await bootedApp();
    const id = plainNode(app);
    const node = app.laidNodes.find((n) => n.id === id);
    const l = node.sourceLinks.concat(node.targetLinks)[0];
    app.filterOutNode(l.source.id === id ? l.target.id : l.source.id);
    app.showOnlyNode(id);
    const hash = app.hashOf(Object.assign({}, app.viewState(), { hide: ["fund-group/bogus"] }));
    const got = await app.restoreView(hash);
    assert.equal(got, app.FAILED);
    assert.deepEqual({ only: app.filter.only, out: [...app.filter.nodes] }, { only: "", out: [] });
    assert.equal(refusals(document).length, 1);
  });
});
