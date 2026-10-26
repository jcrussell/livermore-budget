// nodemenu.test.mjs -- a node's context menu (right-click, Shift+F10, the
// ContextMenu key, M, or the drawer's More): Follow, Filter out, Show only,
// the filter line, Reset and the hash. Assertions are over ids, strings and
// booleans.

import { describe, test } from "node:test";
import assert from "node:assert/strict";
import { bootedApp, loadApp, split, revealed, settle, fire, refusals, dollars, columnFixture, listenerErrors } from "./testlib.mjs";

const OPENS = "fund-group/general";

/** The drawn mark for one node id, or a throw. */
function markOf(document, id) {
  const m = [...document.querySelectorAll("#chart g.node")].find((g) => g.__data__ && g.__data__.id === id);
  if (!m) throw new Error(`${id} is not drawn`);
  return m;
}

/** The first drawn node by id that is no fund group or mark, with ribbons in and out where any has. */
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

/**
 * The non-group node with most ribbons to non-group nodes, and its first such
 * neighbour by id (a fund group filtered out is the panel's). The overview
 * draws no such ribbon.
 */
function plainPair(app) {
  const end = (l, id) => (l.source.id === id ? l.target : l.source);
  const plain = (n) => !app.isGroupID(n.id) && !app.isSmaller(n.id);
  const others = (n) => n.sourceLinks.concat(n.targetLinks).map((l) => end(l, n.id)).filter(plain)
    .sort((a, b) => (a.id < b.id ? -1 : 1));
  const nodes = app.laidNodes.filter(plain).sort((a, b) => others(b).length - others(a).length || (a.id < b.id ? -1 : 1));
  if (!nodes.length || !others(nodes[0]).length) throw new Error("no drawn node has a neighbour that is no fund group");
  return { id: nodes[0].id, other: others(nodes[0])[0] };
}

const pinnedID = (app) => (app.pinned && typeof app.pinned.id === "string" ? app.pinned.id : "");

describe("a node's context menu", () => {
  // Mutation: drop g.node's contextmenu binding and neither click nor keys
  // open it; open it from a ContextMenu/Shift+F10 keydown too and on Windows
  // the keyup's contextmenu opens the browser's menu over it.
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

  // Mutation: tell a key's contextmenu by 0, 0 alone (no keyedMenu) and the
  // menu opens at Chromium's coordinates; by 0, 0 as well and a corner
  // right-click opens beside the mark.
  test("a menu key's contextmenu at the mark's coordinates opens the menu beside the mark; a right-click opens it at the pointer, at 0, 0 too", async (t) => {
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
    fire(m, "contextmenu", { button: 2, clientX: 0, clientY: 0 });
    const corner = placed();
    t.diagnostic(`pointer ${pointer}; ContextMenu ${contextMenuKey}; Shift+F10 ${shiftF10}; a right-click after ${after}; at 0, 0 ${corner}`);
    assert.deepEqual({ pointer, after, corner }, { pointer: "100px 200px", after: "100px 200px", corner: "0px 0px" });
    assert.ok(contextMenuKey !== "" && contextMenuKey !== pointer, contextMenuKey);
    assert.ok(shiftF10 !== "" && shiftF10 !== pointer, shiftF10);
  });

  // Mutation: drop M from the node's keydown and on macOS Safari (no
  // contextmenu for either key) no key opens the menu.
  test("M on a focused node opens its menu beside the mark, focus on the first choice; M with a modifier opens nothing", async () => {
    const { app, document } = await bootedApp();
    const id = plainNode(app);
    const mark = markOf(document, id);
    mark.focus();
    const ctrl = fire(mark, "keydown", { key: "m", ctrlKey: true });
    const withCtrl = menuActs(document);
    const plain = fire(mark, "keydown", { key: "m" });
    const acts = menuActs(document);
    const menu = document.getElementById("node-menu");
    const focused = Boolean(menu) && document.activeElement === menu.querySelector("button");
    const placed = menu ? menu.style.left + " " + menu.style.top : "";
    app.closeChartMenu();
    assert.deepEqual({ withCtrl, ctrl: ctrl.defaultPrevented, acts, plain: plain.defaultPrevented, focused },
      { withCtrl: [], ctrl: false, acts: ["follow", "out", "only"], plain: true, focused: true });
    assert.ok(placed !== "", "the menu M opened has no place");
    assert.ok(mark.getAttribute("aria-keyshortcuts").split(" ").includes("M"), mark.getAttribute("aria-keyshortcuts"));
  });

  // Mutation: hide More (syncPinButton) or drop its handler (wireDrawer) and
  // a phone cannot filter a node or show it alone.
  test("the drawer's More opens the menu of the node it shows, and its choices act on that node; it is hidden for a ribbon and for nothing", async () => {
    const { app, document } = await bootedApp();
    const more = document.getElementById("detail-menu");
    const idle = more.hidden;
    const ribbon = document.querySelector("#chart path.link");
    fire(ribbon, "click");
    const onRibbon = more.hidden;
    app.unpin();
    const id = plainNode(app);
    const name = app.nameOf(app.laidNodes.find((n) => n.id === id));
    app.pin(app.laidNodes.find((n) => n.id === id));
    const onNode = more.hidden;
    fire(more, "click");
    const acts = menuActs(document);
    const named = document.getElementById("node-menu") ? document.getElementById("node-menu").getAttribute("aria-label") : "";
    choose(document, "out");
    assert.deepEqual({ idle, onRibbon, onNode, acts, out: [...app.filter.nodes] },
      { idle: true, onRibbon: true, onNode: false, acts: ["follow", "out", "only"], out: [id] });
    assert.equal(named, app.say("node_menu", { label: name }));
    assert.equal(refusals(document).length, 0);
  });

  // Mutation: no `from` in wireDrawer's openNodeMenu and Escape lands on the
  // chart mark, and More never says it is expanded.
  test("the drawer's More says the menu is expanded while it is open, and Escape puts focus back on More", async () => {
    const { app, document } = await bootedApp();
    const more = document.getElementById("detail-menu");
    const id = plainNode(app);
    app.pin(app.laidNodes.find((n) => n.id === id));
    const shut = more.getAttribute("aria-expanded");
    more.focus();
    fire(more, "click");
    const open = { expanded: more.getAttribute("aria-expanded"), controls: more.getAttribute("aria-controls"),
      popup: more.getAttribute("aria-haspopup") };
    fire(document.getElementById("node-menu"), "keydown", { key: "Escape" });
    const closed = { menu: document.getElementById("node-menu") === null, expanded: more.getAttribute("aria-expanded"),
      back: document.activeElement === more, pinned: pinnedID(app) };
    assert.deepEqual({ shut, open, closed }, {
      shut: "false",
      open: { expanded: "true", controls: "node-menu", popup: null },
      closed: { menu: true, expanded: "false", back: true, pinned: id },
    });
  });

  // Mutation: drop wireDrawer's pressedOpen and the press closes the menu
  // and the click reopens it.
  test("More pressed while its menu is open closes it, focus on More, where the press focuses More and where it does not", async () => {
    const { app, document } = await bootedApp();
    const more = document.getElementById("detail-menu");
    const id = plainNode(app);
    app.pin(app.laidNodes.find((n) => n.id === id));
    const press = (focuses) => {
      more.focus();
      fire(more, "click");
      const opened = document.getElementById("node-menu") !== null;
      fire(more, "pointerdown", { button: 0 });
      if (focuses) more.focus();
      fire(document.body, "pointerup", { button: 0 });
      fire(more, "click");
      return { opened, menu: document.getElementById("node-menu") !== null,
        expanded: more.getAttribute("aria-expanded"), back: document.activeElement === more };
    };
    const focusing = press(true);
    await settle();
    const not = press(false);
    await settle();
    const shut = { opened: true, menu: false, expanded: "false", back: true };
    assert.deepEqual({ focusing, not }, { focusing: shut, not: shut });
    assert.equal(refusals(document).length, 0);
  });

  // Mutation: drop openMenu's restoreFocus after a choice and focus falls to
  // the body once the redraw hides More.
  test("a filter chosen from the drawer's More puts focus on the chart, on the node where it is still drawn", async () => {
    const { app, document } = await bootedApp();
    const more = document.getElementById("detail-menu");
    const id = plainNode(app);
    const from = (act) => {
      app.pin(app.laidNodes.find((n) => n.id === id));
      more.focus();
      fire(more, "click");
      choose(document, act);
      const on = document.activeElement;
      return { hidden: more.hidden, chart: Boolean(on && on.matches && on.matches("#chart g.node")),
        node: Boolean(on && on.__data__ && on.__data__.id === id) };
    };
    const only = from("only");
    fire(document.getElementById("filter-reset"), "click");
    const out = from("out");
    assert.deepEqual({ only, out }, {
      only: { hidden: true, chart: true, node: true },
      out: { hidden: true, chart: true, node: false },
    });
    assert.equal(refusals(document).length, 0);
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
    await split(app, OPENS);
    const { id, other } = plainPair(app);
    app.showOnlyNode(id);
    const node = app.laidNodes.find((n) => n.id === id);
    const otherName = app.nameOf(other);
    const before = drawnRibbons(document).length;
    app.filterOutNode(other.id);
    const said = document.getElementById("filter-said").textContent;
    assert.equal(app.filter.only, id);
    assert.ok(drawnRibbons(document).length < before, "the neighbour's ribbon is still drawn");
    assert.ok(said.includes(app.say("filter_out", { names: otherName })), said);
    assert.ok(said.includes(app.say("filter_only", { label: app.nameOf(node) })), said);
  });

  // Mutation: drop filterFor's throw where nothing of the node survives and
  // each state draws with `only` naming nothing drawn; refuse through fail()
  // and the followed node's pin and isolation go.
  test("a node shown alone whose ribbons the other removals all remove is refused, the chart, the state and the pin as they were", async (t) => {
    const { app, document } = await bootedApp();
    const id = plainNode(app);
    const before = drawnRibbons(document).length;
    // A node with one ribbon, shown alone, then its one neighbour filtered out.
    const lone = app.laidNodes.find((n) => !app.isGroupID(n.id) && !app.isSmaller(n.id) &&
      app.projection.nodes.some((p) => p.id === n.id) && n.sourceLinks.length + n.targetLinks.length === 1);
    assert.ok(lone, "no drawn node has one ribbon, so the neighbour case asserts nothing");
    const neighbour = (lone.sourceLinks[0] || lone.targetLinks[0]);
    const other = neighbour.source.id === lone.id ? neighbour.target.id : neighbour.source.id;
    const tried = [];
    /** Tries a state and reads what is on screen after. */
    const attempt = (name, act) => {
      const was = { only: app.filter.only, out: [...app.filter.nodes], hidden: [...app.filter.groups], drawn: drawnRibbons(document).length };
      app.clearRefusal();
      const ok = act();
      const banners = refusals(document).map((b) => b.textContent);
      tried.push({ name, ok, kept: JSON.stringify({ only: app.filter.only, out: [...app.filter.nodes], hidden: [...app.filter.groups],
        drawn: drawnRibbons(document).length }) === JSON.stringify(was), said: banners.length === 1 &&
        banners[0].startsWith("The chart could not be filtered: the other filters leave none of ") });
    };
    app.followNode(id);
    attempt("out and alone", () => app.setFilter({ nodes: new Set([id]), only: id }));
    const followed = { pinned: pinnedID(app), isolated: app.filter.isolated };
    app.unpin();
    attempt("its group hidden and alone", () => app.setFilter({ groups: new Set([OPENS]), only: OPENS }));
    app.showOnlyNode(lone.id);
    attempt("its one neighbour out", () => { app.filterOutNode(other); return null; });
    t.diagnostic(`${before} ribbons; ${lone.id} alone, ${other} out: ${JSON.stringify(tried)}`);
    assert.deepEqual(followed, { pinned: id, isolated: id });
    assert.deepEqual(tried, [
      { name: "out and alone", ok: false, kept: true, said: true },
      { name: "its group hidden and alone", ok: false, kept: true, said: true },
      { name: "its one neighbour out", ok: null, kept: true, said: true },
    ]);
    assert.equal(app.filter.only, lone.id);
  });

  // Mutation: drop filterFor's throw where nothing of the node survives and
  // the link draws with `only` and `out` both set, no banner.
  test("a link showing a node alone and filtering it out falls back to the opening view, saying why", async () => {
    const { app, document } = await bootedApp();
    const id = plainNode(app);
    const got = await app.restoreView(app.hashOf(Object.assign({}, app.viewState(), { only: id, out: [id] })));
    const banners = refusals(document).map((b) => b.textContent);
    assert.deepEqual({ got, only: app.filter.only, out: [...app.filter.nodes], banners: banners.length },
      { got: app.FAILED, only: "", out: [], banners: 1 });
    assert.ok(banners[0].includes("the chart could not be filtered as it names"), banners[0]);
  });

  // Mutation: drop setFilter's isolate after the redraw and the isolation
  // goes with the dropped pin while setFilter still claims success.
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
  // Mutation: make openNodeMenu's concat unconditional and a smaller-flows
  // mark offers two choices that only change the hash.
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

  // Mutation: null the pin alone in redraw or setFilter (no dropPin) and the
  // chart follows a node the drawer no longer shows.
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

  // Mutation: set `pointing` on a primary press alone and the right press's
  // focus pins the node, dropping the follow before the menu opens.
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

describe("the filter line's names", () => {
  // Mutation: drop keepOutNames from setFilter and the tail is named by the
  // earlier chart's qualifier.
  test("a name kept for a node filtered out from the menu goes with Reset, so a link filtering it out of another chart names it as that chart does", async (t) => {
    const { app, document } = await bootedApp();
    await split(app, "revenue/licenses-and-permits", "revenue/taxes/property");
    const id = "aggregate/tail/lines/revenue/licenses-and-permits";
    const chosen = app.laidNodes.find((n) => n.id === id);
    assert.ok(chosen !== undefined, `${id} is not drawn`);
    const stale = app.nameOf(chosen);
    app.filterOutNode(id);
    app.resetFilters();
    const view = Object.assign({}, app.viewState(), { x: ["revenue/licenses-and-permits"] });
    const drawn = await app.restoreView(app.hashOf(view));
    const node = app.laidNodes.find((n) => n.id === id);
    const fresh = node ? app.nameOf(node) : "";
    const got = await app.restoreView(app.hashOf(Object.assign({}, view, { out: [id] })));
    const said = document.getElementById("filter-said").textContent;
    t.diagnostic(`${id} drawn as ${JSON.stringify(stale)} when chosen, as ${JSON.stringify(fresh)} split at licenses alone; the line says: ${said}`);
    assert.deepEqual([drawn, got], [app.DREW, app.DREW]);
    assert.ok(fresh !== "" && fresh !== stale, "the second chart names the tail as the first did, so this asserts nothing");
    assert.ok(said.startsWith(app.say("filter_out", { names: fresh })), said);
  });

  // Mutation: drop showYear's outNames.clear() and the tail is named by the
  // first year's qualifier.
  test("a name kept for a node filtered out from the menu goes with a year switch, so a link keeping it filtered out names it as the new chart does", async (t) => {
    const { app, document } = await bootedApp();
    await split(app, "revenue/licenses-and-permits", "revenue/taxes/property");
    const id = "aggregate/tail/lines/revenue/licenses-and-permits";
    const chosen = app.laidNodes.find((n) => n.id === id);
    assert.ok(chosen !== undefined, `${id} is not drawn`);
    const stale = app.nameOf(chosen);
    const opening = app.yearKey(app.shownYear);
    const other = app.CONFIG.years.map((y) => app.yearKey(y)).find((k) => k !== opening);
    assert.ok(other, "the page publishes one year, so no link moves to another");
    app.filterOutNode(id);
    const got = await app.restoreView(app.hashOf(Object.assign({}, app.viewState(),
      { year: other, x: ["revenue/licenses-and-permits"] })));
    const node = app.projection.nodes.find((n) => n.id === id);
    const said = document.getElementById("filter-said").textContent;
    t.diagnostic(`${id} drawn as ${JSON.stringify(stale)} in ${opening}; in ${other} split at licenses alone the line says: ${said}`);
    assert.deepEqual({ got, out: [...app.filter.nodes] }, { got: app.DREW, out: [id] });
    assert.ok(node !== undefined && app.nameOf(node) !== stale, "the new chart's name is the stale one, so this asserts nothing");
    assert.ok(said.startsWith(app.say("filter_out", { names: app.nameOf(node) })), said);
  });

  // Mutation: paint the filter line before setFilter's render and it names
  // the tail by the pre-filter chart's qualifier.
  test("a node shown alone is named on the filter line as the filtered chart draws it, when what is drawn changes its qualifier", async (t) => {
    const { app, document } = await bootedApp();
    await split(app, "revenue/licenses-and-permits", "revenue/taxes/property");
    const id = "aggregate/tail/lines/revenue/licenses-and-permits";
    const name = () => {
      const n = app.laidNodes.find((x) => x.id === id);
      return n ? app.nameOf(n) : "";
    };
    const before = name();
    app.showOnlyNode(id);
    const after = name();
    const said = document.getElementById("filter-said").textContent;
    t.diagnostic(`${id} drawn as ${JSON.stringify(before)}, then alone as ${JSON.stringify(after)}; the line says: ${said}`);
    assert.ok(before !== "" && after !== "" && before !== after, "showing the tail alone left its name as it was, so this asserts nothing");
    assert.ok(said.startsWith(app.say("filter_only", { label: after })), said);
  });
});

describe("a node shown alone under the smallest-flow filter", () => {
  // Mutation: drop foldSmaller's `keep` arm (or filterDocument's keeps) and
  // a ribbon of the node shown alone merges into a smaller-flows ribbon; if
  // all merged, every draw is refused until Reset.
  test("raising the smallest flow above every ribbon of the node shown alone draws them as themselves, by the slider and by the link", async (t) => {
    const { app, document } = await bootedApp();
    const at = (n) => n.sourceLinks.concat(n.targetLinks);
    const big = (n) => Math.max(...at(n).map((l) => Math.abs(l.value)));
    const plain = app.laidNodes.filter((n) => !app.isGroupID(n.id) && !app.isSmaller(n.id) && at(n).length &&
      at(n).every((l) => !l.derived && !l.contra && !app.isSmaller(l.source.id) && !app.isSmaller(l.target.id)) &&
      app.SMALLEST_STOPS.some((s) => s > big(n))).sort((a, b) => (a.id < b.id ? -1 : 1));
    assert.ok(plain.length, "no drawn node's ribbons are all under a stop, so the fold asserts nothing");
    const node = plain[0];
    const k = app.SMALLEST_STOPS.findIndex((s) => s > big(node));
    const stop = app.SMALLEST_STOPS[k];
    const id = node.id;
    const own = () => [...document.querySelectorAll("#chart path.link")].map((p) => p.__data__)
      .filter((l) => l.source.id === id || l.target.id === id);
    app.showOnlyNode(id);
    const before = own().length;
    const range = document.getElementById("filter-smallest");
    range.value = String(k);
    fire(range, "input");
    const after = own();
    const slid = { only: app.filter.only, smallest: app.filter.smallest, ribbons: after.length,
      merged: after.filter((l) => l.merged).length, refused: refusals(document).length };
    const hash = app.hashOf(app.viewState());
    const again = await loadApp();
    again.window.history.replaceState(null, "", hash);
    await again.app.boot();
    await settle();
    const linked = { only: again.app.filter.only, smallest: again.app.filter.smallest, refused: refusals(again.document).length };
    t.diagnostic(`${id} alone, its largest ribbon ${dollars(big(node))}, under ${dollars(stop)}: ${hash}`);
    assert.deepEqual({ slid, linked }, {
      slid: { only: id, smallest: stop, ribbons: before, merged: 0, refused: 0 },
      linked: { only: id, smallest: stop, refused: 0 },
    });
  });
});

describe("the node filters in the URL", () => {
  // Mutation: drop `out` or `only` from viewState and the reloaded page
  // draws the whole chart.
  test("a node filtered out and a node shown alone round-trip through the hash", async (t) => {
    const { app, window } = await bootedApp();
    await split(app, OPENS);
    const pair = plainPair(app);
    t.diagnostic(`${pair.id} alone, ${pair.other.id} filtered out`);
    const id = pair.id;
    const other = pair.other.id;
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

  // Mutation: drop moveTo's catch around showYear and restoreView rejects,
  // leaving `out` and `only` in the state.
  test("a link to another year whose node shown alone the other filters empty falls back to the opening view, at boot and on Back", async (t) => {
    const { app, document, window } = await bootedApp();
    const id = plainNode(app);
    const opening = app.yearKey(app.shownYear);
    const other = app.CONFIG.years.map((y) => app.yearKey(y)).find((k) => k !== opening);
    assert.ok(other, "the page publishes one year, so no link moves to another");
    const hash = app.hashOf({ year: other, hide: [], hidekind: [], out: [id], only: id, min: 0, iso: "", x: [] });
    let got;
    try {
      got = await app.restoreView(hash);
    } catch (e) {
      got = "threw: " + e.message;
    }
    const popped = { got, year: app.yearKey(app.shownYear), only: app.filter.only, out: [...app.filter.nodes],
      banners: refusals(document).length, drawn: drawnRibbons(document).length > 0 };
    const again = await loadApp();
    again.window.history.replaceState(null, "", hash);
    await again.app.boot();
    await settle();
    const booted = { year: again.app.shownYear ? again.app.yearKey(again.app.shownYear) : "", only: again.app.filter.only,
      out: [...again.app.filter.nodes], banners: refusals(again.document).length,
      drawn: again.document.querySelectorAll("#chart path.link").length > 0 };
    const said = refusals(again.document).map((b) => b.textContent).join(" | ");
    t.diagnostic(`${hash}: on Back ${JSON.stringify(popped)}; at boot ${JSON.stringify(booted)}: ${said}`);
    assert.deepEqual(popped, { got: app.FAILED, year: opening, only: "", out: [], banners: 1, drawn: true });
    assert.deepEqual(booted, { year: opening, only: "", out: [], banners: 1, drawn: true });
    assert.ok(said.includes("the other filters leave none of "), said);
    assert.ok(window.location.hash === app.hashOf(app.viewState()), window.location.hash);
  });

  // Mutation: keep drawnNames through showYear's draw (no clear) and the
  // refusal names the node in the year on screen's words.
  test("a link to another year refused for its node shown alone names the node in that year's words", async (t) => {
    const other = columnFixture("fy2027-adopted");
    const { app, document } = await bootedApp({ checkedStem: "sankey", plan: { "fy2027-adopted.json": { doc: other } } });
    const id = plainNode(app);
    const there = other.nodes.find((n) => n.id === id);
    assert.ok(there, `${id} is not in the other year's schedule`);
    there.label = "Renamed in the other year";
    const drawnAs = app.nameOf(app.laidNodes.find((n) => n.id === id));
    const year = app.CONFIG.years.find((y) => y.path.includes("fy2027-adopted"));
    assert.ok(year && app.shownYear !== year, "the page does not open on another year than fy2027-adopted");
    await app.restoreView(app.hashOf({ year: app.yearKey(year), hide: [], hidekind: [], out: [id], only: id, min: 0, iso: "", x: [] }));
    const said = refusals(document).map((b) => b.textContent).join(" | ");
    t.diagnostic(`${id}, drawn as "${drawnAs}": ${said}`);
    assert.ok(said.includes("none of Renamed in the other year's money"), said);
  });

  // Mutation: set filter = removals before moveTo's showYear and a year
  // switch overtaking the link keeps the link's node filtered out.
  test("a link to another year that the reader's year switch overtakes leaves none of its removals on the year they picked", async (t) => {
    const { app } = await bootedApp();
    const id = plainNode(app);
    const opening = app.shownYear;
    const other = app.CONFIG.years.map((y) => app.yearKey(y)).find((k) => k !== app.yearKey(opening));
    assert.ok(other, "the page publishes one year, so no link moves to another");
    const link = app.restoreView(app.hashOf({ year: other, hide: [], hidekind: [], out: [id], only: "", min: 0, iso: "", x: [] }));
    const click = app.showYear(opening);
    const got = { link: await link, click: await click };
    const after = { year: app.yearKey(app.shownYear), out: [...app.filter.nodes], hidden: app.shown.hidden.length };
    t.diagnostic(`${JSON.stringify(got)}; ${JSON.stringify(after)}`);
    assert.deepEqual({ got, after }, { got: { link: app.SUPERSEDED, click: app.DREW },
      after: { year: app.yearKey(opening), out: [], hidden: 0 } });
  });

  // Mutation: drop `out` and `only` from restoreView's fallback and the node
  // stays shown alone under an "opening view" banner.
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

describe("the node filters and an open hub", () => {
  const HUB = "fund-group/enterprise";
  /** What arrives at and leaves the hub on the chart drawn, in cents. */
  const through = (app, links) => {
    let arriving = 0;
    let leaving = 0;
    for (const l of links) {
      if (l.target === HUB) arriving += l.value_cents;
      if (l.source === HUB) leaving += l.value_cents;
    }
    return { arriving, leaving };
  };
  /** The hub's member drawn with the most money from it. */
  const memberOf = (app) => app.shown.doc.links.filter((l) => l.source === HUB && !app.isMark(l.target))
    .sort((a, b) => b.value_cents - a.value_cents)[0].target;

  // Mutation: pass no leafCut from filterFor and filtering the member leaves
  // its leaves arriving at the hub (unbalanced); showing it alone draws
  // nothing into the hub.
  test("filtering out a member draws the hub balanced with none of its leaves; showing it alone draws its legs and the leaves feeding it, balanced", async (t) => {
    const { app, document } = await bootedApp();
    await split(app, HUB);
    const member = memberOf(app);
    const inside = app.withinNode(app.fetched, member);
    const carries = (l) => Object.keys(app.cellsOf(l) || {}).some((k) => app.cellIn(k, inside) !== "none");
    const before = through(app, app.shown.doc.links);
    app.filterOutNode(member);
    const out = { ...through(app, app.shown.doc.links), carrying: app.shown.doc.links.filter(carries).length,
      drawn: app.laidNodes.some((n) => n.id === member) };
    const said = document.getElementById("filter-said").textContent;
    app.resetFilters();
    app.showOnlyNode(member);
    const links = app.shown.doc.links;
    const astray = links.filter((l) => l.source !== member && l.target !== member &&
      Object.keys(app.cellsOf(l) || { "": 0 }).some((k) => app.cellIn(k, inside) !== "all"));
    const alone = { ...through(app, links), astray: astray.length, drawn: app.laidNodes.some((n) => n.id === member) };
    t.diagnostic(`${member} of ${HUB}: ${JSON.stringify({ before, out, alone })}; filtered out, the line says: ${said}`);
    assert.equal(before.arriving, before.leaving, "the hub is not balanced unfiltered, so balance says nothing");
    assert.deepEqual({ balanced: out.arriving === out.leaving, carrying: out.carrying, drawn: out.drawn },
      { balanced: true, carrying: 0, drawn: false });
    assert.ok(out.arriving < before.arriving, "filtering the member out took nothing from the hub");
    assert.deepEqual({ balanced: alone.arriving === alone.leaving, astray: alone.astray, drawn: alone.drawn },
      { balanced: true, astray: 0, drawn: true });
    assert.ok(alone.arriving > 0, "showing the member alone draws nothing arriving at the hub");
    assert.equal(refusals(document).length, 0);
  });

  // Mutation: drop state.groups from leafCut and the legend's group filter
  // draws whole the legs carrying the group's cells into the hub, which then
  // takes in more than it gives; the menu's route still cuts them.
  test("a fund group hidden from the legend cuts its leaves from a before hub, drawing the hub balanced as the menu's route does", async (t) => {
    const { app, document } = await bootedApp();
    const BEFORE = "revenue/charges-for-services";
    const G = "fund-group/enterprise";
    await revealed(app, BEFORE);
    await split(app, BEFORE);
    const at = (links) => {
      let arriving = 0;
      let leaving = 0;
      for (const l of links) {
        if (l.target === BEFORE) arriving += l.value_cents;
        if (l.source === BEFORE) leaving += l.value_cents;
      }
      return { arriving, leaving };
    };
    const drawn = () => app.shown.doc.links.map((l) => `${l.source} -> ${l.target} ${l.kind} ${l.value_cents}`).sort();
    const before = at(app.shown.doc.links);
    const legend = app.setFilter({ groups: new Set([G]) });
    const byLegend = { ...at(app.shown.doc.links), ribbons: drawn() };
    app.resetFilters();
    assert.ok(app.laidNodes.some((n) => n.id === G), `${G} is not drawn, so the menu cannot filter it out`);
    app.filterOutNode(G);
    const byNode = drawn();
    t.diagnostic(`${BEFORE} unfiltered ${JSON.stringify(before)}; ${G} hidden from the legend ` +
      `${JSON.stringify({ arriving: byLegend.arriving, leaving: byLegend.leaving })}`);
    assert.equal(before.arriving, before.leaving, "the hub is not balanced unfiltered, so balance says nothing");
    assert.equal(legend, true);
    assert.equal(byLegend.arriving, byLegend.leaving);
    assert.ok(byLegend.leaving < before.leaving, `hiding ${G} took nothing from the hub`);
    assert.deepEqual(byNode, byLegend.ribbons);
    assert.equal(refusals(document).length, 0);
  });

  // Mutation: pass filterDocument the state unfiltered by drawn nodes (no
  // `st`) and a fund folded under Collapse all still cuts its leaves, hiding
  // money the line names no node for.
  test("a node filtered out that a collapse folds away is inert: nothing hidden, nothing said", async (t) => {
    const { app, document } = await bootedApp();
    await split(app, HUB);
    const member = memberOf(app);
    app.filterOutNode(member);
    const out = app.shown.hidden.length;
    app.collapseAll();
    await settle();
    const after = { kept: [...app.filter.nodes], drawn: app.projection.nodes.some((n) => n.id === member),
      hidden: app.shown.hidden.length, line: document.getElementById("filter-line").hasAttribute("hidden") };
    t.diagnostic(`${member} filtered out of the open ${HUB} hides ${out} ribbon(s); collapsed, ${JSON.stringify(after)}`);
    assert.ok(out > 0, `filtering ${member} out hid nothing, so inertness says nothing`);
    assert.deepEqual(after, { kept: [member], drawn: false, hidden: 0, line: true });
  });

  // Mutation: drop filterDocument's inHidden group arm (nodes.has(g)) and
  // the open group's funds' balance ribbons, which repeat no leaf, stay drawn
  // though the legend's filter removes them.
  test("filtering out an open fund group is the legend's group filter: the same money removed, and the same state", async (t) => {
    const { app, document } = await bootedApp();
    await split(app, OPENS);
    const name = app.nameOf(app.laidNodes.find((n) => n.id === OPENS));
    const drawn = () => app.shown.doc.links.map((l) => `${l.source} -> ${l.target} ${l.kind} ${l.value_cents}`).sort();
    app.setFilter({ groups: new Set([OPENS]) });
    const byLegend = drawn();
    app.resetFilters();
    app.filterOutNode(OPENS);
    const byNode = drawn();
    const said = document.getElementById("filter-said").textContent;
    t.diagnostic(`${OPENS}: ${byLegend.length} ribbon(s) drawn under the legend's filter, ${byNode.length} filtered out; the line says: ${said}`);
    assert.deepEqual(byNode, byLegend);
    assert.deepEqual({ groups: [...app.filter.groups], nodes: [...app.filter.nodes] }, { groups: [OPENS], nodes: [] });
    assert.ok(!said.includes(name), said);
  });

  // Mutation: drop filterOutNode's group arm and the box stays checked,
  // rechecking restores nothing, and search offers its funds.
  test("a fund group filtered out from the menu is the panel's: its box unchecks, search offers none of its funds, and rechecking the box brings it back", async (t) => {
    const { app, document } = await bootedApp();
    const g = "fund-group/enterprise";
    const box = [...document.querySelectorAll("#filter-group-list input")].find((b) => b.value === g);
    const member = app.fetched.nodes.find((n) => n.id.startsWith("fund/") && app.fundGroupOf(n) === g && !app.laidNodes.some((d) => d.id === n.id));
    assert.ok(member, `the schedule holds no undrawn member of ${g}`);
    const offered = () => app.searchMatches(member.label).some((n) => n.id === member.id);
    const before = { checked: box.checked, offered: offered() };
    fire(markOf(document, g), "contextmenu", { button: 2, clientX: 40, clientY: 40 });
    choose(document, "out");
    const out = { checked: box.checked, offered: offered(), drawn: app.laidNodes.some((n) => n.id === g),
      hash: app.viewState().hide.includes(g) };
    box.checked = true;
    fire(box, "change");
    const back = { checked: box.checked, offered: offered(), drawn: app.laidNodes.some((n) => n.id === g),
      groups: app.filter.groups.size, nodes: app.filter.nodes.size };
    t.diagnostic(`${g}, searched by its member ${member.id} ("${member.label}")`);
    assert.deepEqual({ before, out, back }, {
      before: { checked: true, offered: true },
      out: { checked: false, offered: false, drawn: false, hash: true },
      back: { checked: true, offered: true, drawn: true, groups: 0, nodes: 0 },
    });
    assert.equal(refusals(document).length, 0);
  });

  // Mutation: ignore filterRefusal (no `refused` on the choices) and
  // Services & Supplies' choices are enabled then refused with a banner.
  // Saying the message instead fails too (it names ids), as does
  // refused_part given another drawn ribbon's ends.
  test("with the General Fund open by divisions a filter the redraw would refuse is offered disabled, saying why in names; one it draws is offered", async (t) => {
    const { app, document } = await bootedApp();
    await split(app, OPENS);
    const offered = (id) => {
      fire(markOf(document, id), "contextmenu", { button: 2, clientX: 40, clientY: 40 });
      const menu = document.getElementById("node-menu");
      const seen = {};
      for (const b of menu.querySelectorAll("button")) {
        const why = b.getAttribute("aria-describedby");
        const said = why ? document.getElementById(why) : null;
        seen[b.dataset.act] = { disabled: b.disabled, why: said ? said.textContent : "" };
      }
      app.closeChartMenu();
      return seen;
    };
    const S = "expenditure/services-and-supplies";
    const refused = offered(S);
    const works = offered("fund/100");
    t.diagnostic(`${S}: ${JSON.stringify(refused)}; fund/100: ${JSON.stringify(works)}`);
    const byID = new Map(app.projection.nodes.map((n) => [n.id, n]));
    // The ribbon the redraw's own filter refuses to cut, by its ends.
    const blocks = (next) => {
      try {
        app.filterFor(app.projection, Object.assign({}, app.filter, next));
      } catch (e) {
        if (byID.has(e.source) && byID.has(e.target)) {
          return app.say("refused_part", { source: app.nameOf(byID.get(e.source)), target: app.nameOf(byID.get(e.target)) });
        }
      }
      return "(no ribbon blocks it)";
    };
    const want = { out: blocks({ nodes: new Set([S]) }), only: blocks({ only: S }) };
    for (const act of ["out", "only"]) {
      assert.equal(refused[act].disabled, true, `${act} on ${S} is offered`);
      assert.equal(refused[act].why, want[act], act);
      assert.ok(!app.projection.nodes.some((n) => refused[act].why.includes(n.id)), refused[act].why);
      assert.deepEqual(works[act], { disabled: false, why: "" }, act);
    }
    assert.deepEqual(refused.follow, { disabled: false, why: "" });
    // What the menu refuses, the redraw refuses.
    app.filterOutNode(S);
    assert.equal(refusals(document).length, 1);
  });

  // Mutation: filterRefusal's AloneEmpty arm dropped and this goes red: the
  // menu says refused_other, naming no node.
  test("a filter out leaving none of the money of the node shown alone is offered disabled, naming that node", async (t) => {
    const { app, document } = await bootedApp();
    const lone = app.laidNodes.find((n) => !app.isGroupID(n.id) && !app.isSmaller(n.id) &&
      app.projection.nodes.some((p) => p.id === n.id) && n.sourceLinks.length + n.targetLinks.length === 1);
    assert.ok(lone !== undefined, "no drawn node has one ribbon, so nothing here is tested");
    const neighbour = (lone.sourceLinks[0] || lone.targetLinks[0]);
    const other = neighbour.source.id === lone.id ? neighbour.target.id : neighbour.source.id;
    const name = app.nameOf(lone);
    app.showOnlyNode(lone.id);
    await settle();
    fire(markOf(document, other), "contextmenu", { button: 2, clientX: 40, clientY: 40 });
    const b = [...document.querySelectorAll("#node-menu button")].find((x) => x.dataset.act === "out");
    const why = b && b.getAttribute("aria-describedby") ? document.getElementById(b.getAttribute("aria-describedby")).textContent : "";
    t.diagnostic(`${lone.id} alone, ${other}'s Filter this out: ${why}`);
    assert.deepEqual({ disabled: b ? b.disabled : null, why }, { disabled: true, why: app.say("refused_alone_empty", { label: name }) });
    assert.equal(refusals(document).length, 0);
  });

  // Mutation: drop searchMatches' core.withinNode arm and Police (inside the
  // filtered General Fund) is offered; drop the `only` arm and an outside
  // enterprise fund is offered and Enter refuses it.
  test("search offers no node within a node filtered out, nor one outside the money of the node shown alone", async (t) => {
    const { app, document } = await bootedApp();
    await split(app, OPENS);
    const within = app.withinNode(app.fetched, "fund/100");
    app.filterOutNode("fund/100");
    const police = app.searchMatches("Police").map((n) => n.id);
    const inside = app.fetched.nodes.filter((n) => within.has(n.id) && n.id !== "fund/100" && /police/i.test(n.label)).map((n) => n.id);
    app.resetFilters();
    app.showOnlyNode("fund/100");
    const g = "fund-group/enterprise";
    const member = app.fetched.nodes.find((n) => n.id.startsWith("fund/") && app.fundGroupOf(n) === g && !app.laidNodes.some((d) => d.id === n.id));
    assert.ok(member, `the schedule holds no undrawn member of ${g}`);
    const offered = app.searchMatches(member.label).map((n) => n.id);
    const kept = app.searchMatches("Police").filter((n) => !app.laidNodes.some((d) => d.id === n.id)).map((n) => n.id);
    t.diagnostic(`fund/100 out: "Police" offers ${JSON.stringify(police)} of ${JSON.stringify(inside)} within; ` +
      `fund/100 alone: "${member.label}" offers ${JSON.stringify(offered)}, "Police" undrawn ${JSON.stringify(kept)}`);
    assert.ok(inside.length > 0, "no node within fund/100 is labelled Police, so nothing here is tested");
    assert.deepEqual(police.filter((id) => within.has(id)), []);
    assert.ok(!offered.includes(member.id), `${member.id} is offered under fund/100 alone`);
    // What it does offer, Enter never says the filters leave out (a redraw
    // it refuses for a part of a restated total is fisc-ylxh.124's).
    for (const id of kept) {
      app.chooseSearch(id, true);
      const said = refusals(document).map((b) => b.textContent).join(" | ");
      assert.ok(!said.includes("out of the chart"), `${id} is offered and Enter says: ${said}`);
      app.showOnlyNode("fund/100");
    }
  });

  // Mutation: use core.withinNode alone in filteredOutWithin (no tails) and
  // the tail's member is offered and Enter redraws its money.
  test("search offers no member of a tail filtered out, and Enter on one draws nothing", async (t) => {
    const { app, document } = await bootedApp();
    await split(app, "fund-group/special-revenue");
    const tail = app.projection.nodes.find((n) => app.expandable(n) && app.laidNodes.some((d) => d.id === n.id));
    assert.ok(tail, "the split draws no tail, so nothing here is tested");
    fire(markOf(document, tail.id), "contextmenu", { button: 2, clientX: 40, clientY: 40 });
    choose(document, "out");
    await settle();
    const member = app.nodeOf(tail.folds[0]);
    const offered = app.searchMatches(member.label).map((n) => n.id);
    const before = app.expansions.slice();
    const drew = app.chooseSearch(member.id, true);
    await settle();
    t.diagnostic(`${tail.id} folds ${JSON.stringify(tail.folds)} and is filtered out; "${member.label}" offers ${JSON.stringify(offered)}`);
    assert.deepEqual({ offered: offered.includes(member.id), drew, expansions: app.expansions, out: [...app.filter.nodes],
      drawn: app.laidNodes.some((d) => d.id === member.id) },
    { offered: false, drew: false, expansions: before, out: [tail.id], drawn: false });
  });

  // Mutation: read only drawn filtered nodes in filteredOutWithin and Police
  // Administration is offered, then refused, its reveal redrawing the hub.
  test("search offers no node within a node filtered out the chart folds away", async (t) => {
    const { app } = await bootedApp();
    await split(app, OPENS);
    app.filterOutNode("fund/100");
    await settle();
    app.collapseNode(OPENS);
    await settle();
    const id = "dept/police-administration";
    const inert = !app.projection.nodes.some((n) => n.id === "fund/100");
    const offered = app.searchMatches("Police Administration").map((n) => n.id);
    t.diagnostic(`fund/100 out and ${inert ? "folded" : "drawn"}; "Police Administration" offers ${JSON.stringify(offered)}`);
    assert.deepEqual({ inert, offered: offered.includes(id) }, { inert: true, offered: false });
  });

  // Mutation: drop liveFilters' "only:" arm and the member is offered; Enter
  // makes `only` inert and draws the whole chart.
  test("search offers no member of the tail shown alone, and Enter on one draws nothing", async (t) => {
    const { app, document } = await bootedApp();
    await split(app, "fund-group/special-revenue");
    const tail = app.projection.nodes.find((n) => app.expandable(n) && app.laidNodes.some((d) => d.id === n.id));
    assert.ok(tail !== undefined, "the split draws no tail, so nothing here is tested");
    fire(markOf(document, tail.id), "contextmenu", { button: 2, clientX: 40, clientY: 40 });
    choose(document, "only");
    await settle();
    const member = app.nodeOf(tail.folds[0]);
    const offered = app.searchMatches(member.label).map((n) => n.id);
    const before = app.expansions.slice();
    const drew = app.chooseSearch(member.id, true);
    await settle();
    t.diagnostic(`${tail.id} folds ${JSON.stringify(tail.folds)} and is shown alone; "${member.label}" offers ${JSON.stringify(offered)}`);
    assert.deepEqual({ offered: offered.includes(member.id), drew, expansions: app.expansions, only: app.filter.only,
      drawn: app.laidNodes.some((d) => d.id === tail.id) },
    { offered: false, drew: false, expansions: before, only: tail.id, drawn: true });
  });

  // Mutation: drop liveFilters' "out:" arm and Enter opens the parent the
  // other way, folding the filtered node and redrawing its money.
  test("search offers no node whose reveal folds a node filtered out, and Enter on one draws nothing", async (t) => {
    const { app, document } = await bootedApp();
    const alt = app.fetched.splits.find((sp) => sp.alternative);
    assert.ok(alt !== undefined, "the schedule declares no alternative, so nothing here is tested");
    const parent = alt.node;
    await revealed(app, parent);
    await split(app, parent);
    const x = app.laidNodes.find((n) => { const m = app.nodeOf(n.id); return m && m.parent === parent && !m.alternative &&
      !app.isGroupID(n.id) && !app.isSmaller(n.id) && app.filterRefusal({ nodes: new Set([n.id]) }) === ""; });
    assert.ok(x !== undefined, `${parent}'s split draws no member a filter out is offered on, so nothing here is tested`);
    const out = x.id;
    app.filterOutNode(out);
    await settle();
    const member = app.fetched.nodes.find((n) => n.parent === parent && n.alternative === alt.key &&
      !app.laidNodes.some((d) => d.id === n.id));
    assert.ok(member !== undefined, `${parent}'s ${alt.key} has no undrawn member, so nothing here is tested`);
    const offered = app.searchMatches(member.label).map((n) => n.id);
    const before = app.expansions.slice();
    const drew = app.chooseSearch(member.id, true);
    await settle();
    t.diagnostic(`${parent} open by its first split, ${out} out; "${member.label}" (${alt.key}) offers ${JSON.stringify(offered)}`);
    assert.deepEqual({ offered: offered.includes(member.id), drew, expansions: app.expansions, out: [...app.filter.nodes],
      drawn: app.laidNodes.some((d) => d.id === out) },
    { offered: false, drew: false, expansions: before, out: [out], drawn: false });
  });

  // Mutation: drop searchMatches' catch around a reveal and typing paints no
  // list, with a banner.
  test("a match whose reveal throws while a node is shown alone is offered, and the search still lists every other", async (t) => {
    const { app, document } = await bootedApp();
    await split(app, OPENS);
    app.showOnlyNode("fund/100");
    const undrawn = () => app.searchMatches("Police").filter((n) => !app.laidNodes.some((d) => d.id === n.id)).map((n) => n.id);
    const kept = undrawn();
    assert.ok(kept.length > 1, `"Police" offers ${kept.length} undrawn match(es) under fund/100 alone, so nothing here is tested`);
    const throws = kept[0];
    const sankey = app.FORMS.get("sankey");
    app.FORMS.set("sankey", Object.assign({}, sankey, {
      shape: (doc, o) => {
        if (o.frontier.has(throws)) throw new Error("planted: " + throws + " cannot be shaped");
        return sankey.shape(doc, o);
      },
    }));
    let listed;
    try {
      const input = document.getElementById("search-input");
      input.value = "Police";
      fire(input, "input");
      listed = [...document.querySelectorAll("#search-list li")].map((li) => li.dataset.id);
    } finally {
      app.FORMS.set("sankey", sankey);
    }
    t.diagnostic(`${throws}'s reveal throws; "Police" lists ${listed.length}`);
    assert.deepEqual({ missing: kept.filter((id) => !listed.includes(id)), banners: refusals(document).length },
      { missing: [], banners: 0 });
  });

  // Mutations: the input's guard back to fail and pin, follow and drawer go
  // with the banner; drop the arrows' or Escape's guard and its throw
  // reaches the window with no banner.
  test("a search that throws says so for typing, the arrows and Escape, and leaves the pin, the node followed and the drawer", async (t) => {
    const { app, document, window } = await bootedApp();
    const errors = listenerErrors(window);
    const id = plainNode(app);
    app.followNode(id);
    const drawer = document.getElementById("detail").textContent;
    const input = document.getElementById("search-input");
    const list = document.getElementById("search-list");
    const said = {};
    const tryKey = (name, act) => {
      app.clearRefusal();
      app.closeSearch();
      list.replaceChildren = () => { throw new Error("planted: the list cannot be painted"); };
      try {
        act();
      } finally {
        delete list.replaceChildren;
      }
      said[name] = { banners: refusals(document).length, pinned: pinnedID(app), isolated: app.filter.isolated,
        drawer: document.getElementById("detail").textContent === drawer };
    };
    tryKey("input", () => { input.value = "a"; fire(input, "input"); });
    tryKey("ArrowDown", () => fire(input, "keydown", { key: "ArrowDown" }));
    tryKey("Escape", () => fire(input, "keydown", { key: "Escape" }));
    t.diagnostic(`${id} followed; the list planted to throw on painting: ${JSON.stringify(said)}; ${errors.length} listener error(s)`);
    const kept = { banners: 1, pinned: id, isolated: id, drawer: true };
    assert.deepEqual({ said, errors }, { said: { input: kept, ArrowDown: kept, Escape: kept }, errors: [] });
  });

  // Mutations: chooseSearch's "The filters leave" refusal back to fail, and
  // Enter or a click loses pin, follow and drawer; Enter's or the option's
  // guard back to fail, and a throwing list close loses them too.
  test("a search choice refused or throwing says so once and leaves the pin, the node followed and the drawer", async (t) => {
    const { app, document, window } = await bootedApp();
    const errors = listenerErrors(window);
    app.setFilter({ smallest: app.SMALLEST_STOPS[1] });
    const id = plainNode(app);
    app.followNode(id);
    const drawer = document.getElementById("detail").textContent;
    const before = app.expansions.slice();
    const input = document.getElementById("search-input");
    // Folded into the smaller flows once revealed under this filter, so
    // listed (the smallest filter is no node filter) and refused by Enter.
    const hit = app.fetched.nodes.find((n) => n.label === "Animal Shelter Fees & Charges");
    assert.ok(hit !== undefined && !app.laidNodes.some((d) => d.id === hit.id),
      "Animal Shelter Fees & Charges is no undrawn node, so nothing here is tested");
    const said = {};
    const tryChoice = (name, how, plant) => {
      app.clearRefusal();
      input.value = hit.label;
      fire(input, "input");
      const options = [...document.querySelectorAll("#search-list li")];
      const at = options.findIndex((li) => li.dataset.id === hit.id);
      const list = document.getElementById("search-list");
      if (plant) list.replaceChildren = () => { throw new Error("planted: the list cannot be closed"); };
      try {
        if (at < 0) said[name] = "not listed";
        else if (how === "click") options[at].click();
        else {
          for (let k = 0; k <= at; k++) fire(input, "keydown", { key: "ArrowDown" });
          fire(input, "keydown", { key: "Enter" });
        }
      } finally {
        if (plant) delete list.replaceChildren;
      }
      if (at < 0) return;
      const banners = refusals(document);
      said[name] = { banners: banners.length, refused: banners.length === 1 && (plant ? banners[0].textContent.includes("planted")
        : banners[0].textContent === "The filters leave " + hit.label + " out of the chart"),
      pinned: pinnedID(app), isolated: app.filter.isolated, drawer: document.getElementById("detail").textContent === drawer,
      expansions: app.expansions.join() === before.join() };
    };
    tryChoice("Enter", "key", false);
    tryChoice("click", "click", false);
    tryChoice("Enter throwing", "key", true);
    tryChoice("click throwing", "click", true);
    t.diagnostic(`${id} followed under the smallest filter at ${app.SMALLEST_STOPS[1]}; ${hit.id} chosen: ${JSON.stringify(said)}; ${errors.length} listener error(s)`);
    const kept = { banners: 1, refused: true, pinned: id, isolated: id, drawer: true, expansions: true };
    assert.deepEqual({ said, errors }, { said: { "Enter": kept, "click": kept, "Enter throwing": kept, "click throwing": kept }, errors: [] });
  });

  // Mutation: redraw's refusal saying the filter's message, and this goes
  // red: the banner names fund/100 and the department by id.
  test("a search reveal the filters refuse to draw says why in names, and leaves the chart", async (t) => {
    const { app, document } = await bootedApp();
    const S = "expenditure/services-and-supplies";
    // Filtering S out cuts nothing with the General Fund folded; revealing
    // the fund draws it open, where filtering S out cuts part of a ribbon.
    app.filterOutNode(S);
    await settle();
    const id = "fund/100";
    assert.ok(!app.laidNodes.some((d) => d.id === id), `${id} is drawn, so nothing here is revealed`);
    const st = app.expansionState(app.fetched, app.revealEntries(id));
    const after = app.formOnScreen().shape(app.fetched, { chart: app.CONFIG.overview, frontier: st.frontier, whole: st.whole });
    const byID = new Map(after.nodes.map((n) => [n.id, n]));
    let want = "(the reveal is drawn)";
    try {
      app.filterFor(after);
    } catch (e) {
      if (byID.has(e.source) && byID.has(e.target)) {
        want = "That could not be found: " + app.say("redraw_part", { source: app.nameOf(byID.get(e.source)), target: app.nameOf(byID.get(e.target)) });
      }
    }
    const before = app.expansions.slice();
    const drew = app.chooseSearch(id, true);
    await settle();
    const banners = refusals(document).map((b) => b.textContent);
    t.diagnostic(`${S} out; Enter on ${id} says ${JSON.stringify(banners)}`);
    assert.deepEqual({ drew, banners, expansions: app.expansions, out: [...app.filter.nodes] },
      { drew: false, banners: [want], expansions: before, out: [S] });
    assert.ok(!app.fetched.nodes.some((n) => banners[0].includes(n.id)), banners[0]);
  });

  // Mutation: drop moveTo's routing of a listed group out of `out` and the
  // group lands in filter.nodes, its box checked, the hash saying out=.
  test("a link naming a fund group the panel lists among out= hides it as its box does", async (t) => {
    const { app, document } = await bootedApp();
    const g = "fund-group/enterprise";
    const got = await app.restoreView(app.hashOf(Object.assign({}, app.viewState(), { out: [g] })));
    await settle();
    const box = [...document.querySelectorAll("#filter-group-list input")].find((b) => b.value === g);
    const st = app.viewState();
    t.diagnostic(`#…&out=${g} drew ${got}; the hash now says hide=${JSON.stringify(st.hide)} out=${JSON.stringify(st.out)}`);
    assert.deepEqual({ got, groups: [...app.filter.groups], nodes: [...app.filter.nodes], checked: box ? box.checked : null,
      drawn: app.laidNodes.some((n) => n.id === g), hide: st.hide, out: st.out },
    { got: "drew", groups: [g], nodes: [], checked: false, drawn: false, hide: [g], out: [] });
    assert.equal(refusals(document).length, 0);
  });
});
