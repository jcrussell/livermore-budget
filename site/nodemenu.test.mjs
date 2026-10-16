// nodemenu.test.mjs -- a node's context menu: a right-click, Shift+F10 or
// the ContextMenu key on a node opens it, and its three choices follow the
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
  // right-click opens nothing; drop the keydown arm and Shift+F10 does not.
  test("opens on a right-click, Shift+F10 and the ContextMenu key with three choices, focus on the first; Escape closes it onto the node, pinning nothing", async (t) => {
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
    const byShift = menuActs(document);
    app.closeChartMenu();
    fire(mark, "keydown", { key: "ContextMenu" });
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
    assert.deepEqual(byShift, ["follow", "out", "only"]);
    assert.deepEqual(byKey, ["follow", "out", "only"]);
  });

  test("a ribbon has no menu: a right-click on one leaves the browser's own", async () => {
    const { document } = await bootedApp();
    const ribbon = document.querySelector("#chart path.link");
    const e = fire(ribbon, "contextmenu", { button: 2, clientX: 40, clientY: 40 });
    assert.equal(e.defaultPrevented, false);
    assert.equal(document.getElementById("node-menu"), null);
  });

  // Mutation: make the Follow choice do nothing and this goes red.
  test("Follow this money follows the node and pins it, so the drawer's Follow is pressed", async () => {
    const { app, document } = await bootedApp();
    const id = plainNode(app);
    fire(markOf(document, id), "contextmenu", { button: 2, clientX: 40, clientY: 40 });
    choose(document, "follow");
    assert.equal(document.getElementById("node-menu"), null);
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
  // Mutation: keep filter.isolated in filterOutNode and showOnlyNode and
  // this goes red: the chart follows a node the drawer no longer shows.
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
});
