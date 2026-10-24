// gestures.test.mjs -- the SEQUENCES a reader's gestures make on the drawn
// chart: one activation -- a click, Enter or Space -- opening a node that
// splits, folding an open hub back and pinning one that does neither, the ⊕
// menu choosing between a node's two splits, ⊖ folding a split back, Escape
// letting the pin and the isolation go and folding nothing, the trace a
// hover draws, Expand all and Collapse all, a tail drawn out, and where
// focus lands after a split has replaced the element it was on.
//
// Every gesture is a dispatched event with a planted timeStamp, because the
// node's and the glyph's activation guard compares a click's stamp against
// the key that may have synthesised it. Every assertion is over ids, counts and booleans: a
// failing assertion over a laid mark formats the whole graph.

import { describe, test } from "node:test";
import assert from "node:assert/strict";
import { bootedApp, split, settle, fire, pageFixture, clickYear, refusals } from "./testlib.mjs";

const PAGE = pageFixture().config;
/** The published years, newest last as the page lists them. */
const YEARS = PAGE.years;

/** A root that splits one way; a root that splits two ways; the group whose split folds a tail. */
const OPENS = "fund-group/general";
const TWO_WAYS = "expenditure/capital-outlay";
const WORST = "fund-group/special-revenue";

/**
 * A page booted and switched to `stem`. The page opens on the newest year, so
 * every other year is reached the way a reader reaches it, through the control.
 */
async function onYear(stem, o = {}) {
  const loaded = await bootedApp(o);
  if (loaded.app.shownYear.stem !== stem) {
    clickYear(loaded.document, stem);
    await settle();
  }
  assert.equal(loaded.app.shownYear.stem, stem);
  return loaded;
}

/** The drawn mark for one node id, or a throw naming what the chart has split. */
function markOf(app, document, id) {
  const m = [...document.querySelectorAll("#chart g.node")]
    .find((g) => g.__data__ && g.__data__.id === id);
  if (!m) throw new Error(`${id} is not drawn with [${app.expansions.join(", ")}] split`);
  return m;
}

/** The glyph drawn beside one node, or a throw naming what the chart has split. */
function glyphOf(app, document, id) {
  const g = [...document.querySelectorAll("#chart g.glyph")]
    .find((x) => x.__data__ && x.__data__.id === id);
  if (!g) throw new Error(`${id} draws no glyph with [${app.expansions.join(", ")}] split`);
  return g;
}

/** The kind of glyph beside one node: "open", "fold" or "". */
function glyphKind(document, id) {
  const g = [...document.querySelectorAll("#chart g.glyph")].find((x) => x.__data__ && x.__data__.id === id);
  if (!g) return "";
  return g.classList.contains("glyph-open") ? "open" : g.classList.contains("glyph-fold") ? "fold" : "?";
}

/** The id of the mark focus is on, or what else it is on. */
function focusedID(document) {
  const a = document.activeElement;
  if (!a) return "nothing";
  return a.__data__ && typeof a.__data__.id === "string" ? a.__data__.id : a.tagName + "." + a.getAttribute("class");
}

/** The id of the node pinned in the drawer, or "". */
function pinnedID(app) {
  return app.pinned && typeof app.pinned.id === "string" ? app.pinned.id : "";
}

/** The toolbar's Expand all and Collapse all buttons, in that order. */
function bulk(document) {
  return [document.getElementById("chart-expand-all"), document.getElementById("chart-collapse-all")];
}

/** Escape, dispatched on the document the way a keyboard delivers it. */
function escape(document) {
  fire(document, "keydown", { key: "Escape" });
}

/** How many drawn nodes split or draw out: what Expand all leaves none of. */
function offers(app) {
  return app.projection.nodes.filter((n) => app.splittable(n)).length;
}

/** A member of a split node's first split that neither splits nor draws out. */
function inertMember(app, id) {
  const members = app.splitsHere({ id })[0].members;
  const n = app.projection.nodes.find((x) => members.includes(x.id) && !app.splittable(x) && !app.expandable(x));
  if (!n) throw new Error(`every member of ${id} drawn splits or draws out`);
  return n.id;
}

describe("one activation", () => {
  // Mutation: restore the double click as the only gesture that splits (a
  // click pinning alone) and this goes red, the click splitting nothing.
  test("a click, Enter or Space splits a node that splits, and pins it in the drawer", async (t) => {
    const { app, document } = await bootedApp();
    const state = () => ({ split: app.expansions.join(","), pinned: pinnedID(app), isolated: app.filter.isolated });
    const seen = {};
    for (const [gesture, fireIt] of [
      ["click", (m) => fire(m, "click", { timeStamp: 1000 })],
      ["Enter", (m) => fire(m, "keydown", { key: "Enter", timeStamp: 1000 })],
      ["Space", (m) => fire(m, "keydown", { key: " ", timeStamp: 1000 })],
    ]) {
      app.collapseAll();
      await settle();
      fireIt(markOf(app, document, OPENS));
      await settle();
      seen[gesture] = state();
    }
    const kept = app.laidNodes.filter((n) => n.id === "revenue/taxes/property" || n.id === "transfers/in").length;
    t.diagnostic(`one activation of ${OPENS}: ${JSON.stringify(seen)}`);
    for (const gesture of ["click", "Enter", "Space"]) {
      assert.deepEqual(seen[gesture], { split: OPENS, pinned: OPENS, isolated: "" }, gesture);
    }
    assert.equal(kept, 2, "splitting the General Fund group took another root off the chart");
  });

  // Mutation: drop activate's fold arm (a click on an open hub pinning only)
  // and this goes red, the second activation leaving the split open.
  test("a second click, Enter or Space on an open hub folds it back, and it stays pinned", async (t) => {
    const { app, document, config } = await bootedApp();
    const seen = {};
    // Each stamp a second past the last, so no activation is another's echo.
    let at = 0;
    for (const [gesture, fireIt] of [
      ["click", (m) => fire(m, "click", { timeStamp: (at += 1000) })],
      ["Enter", (m) => fire(m, "keydown", { key: "Enter", timeStamp: (at += 1000) })],
      ["Space", (m) => fire(m, "keydown", { key: " ", timeStamp: (at += 1000) })],
    ]) {
      app.collapseAll();
      await settle();
      fire(markOf(app, document, OPENS), "click", { timeStamp: (at += 1000) });
      await settle();
      const opened = { split: app.expansions.join(","), pinned: pinnedID(app) };
      const named = markOf(app, document, OPENS).getAttribute("aria-label").endsWith(config.wording.desc_folds);
      fireIt(markOf(app, document, OPENS));
      await settle();
      seen[gesture] = { opened, named, folded: { split: app.expansions.join(","), pinned: pinnedID(app), glyph: glyphKind(document, OPENS) } };
    }
    t.diagnostic(`two activations of ${OPENS}: ${JSON.stringify(seen)}`);
    for (const gesture of ["click", "Enter", "Space"]) {
      assert.deepEqual(seen[gesture], {
        opened: { split: OPENS, pinned: OPENS }, named: true,
        folded: { split: "", pinned: OPENS, glyph: "open" },
      }, gesture);
    }
    assert.deepEqual(refusals(document).map((b) => b.textContent), []);
  });

  test("a click on a hub opened straight through from its parent folds the parent, and pins it", async () => {
    const { app, document } = await bootedApp();
    await split(app, OPENS, WORST);
    assert.equal(glyphKind(document, "fund/100"), "fold");
    fire(markOf(app, document, "fund/100"), "click", { timeStamp: 1000 });
    await settle();
    assert.deepEqual({ split: app.expansions.join(","), pinned: pinnedID(app) }, { split: WORST, pinned: OPENS });
  });

  // Mutation: drop clickNode's guard and this goes red, the synthesised
  // click folding the hub the key just opened.
  test("a node's key and the click it synthesises are one activation, and a later click folds", async () => {
    const { app, document } = await bootedApp();
    fire(markOf(app, document, OPENS), "keydown", { key: "Enter", timeStamp: 1000 });
    await settle();
    fire(markOf(app, document, OPENS), "click", { timeStamp: 1050 });
    await settle();
    const echoed = { split: app.expansions.join(","), pinned: pinnedID(app) };
    fire(markOf(app, document, OPENS), "click", { timeStamp: 1000 + app.ACTIVATION_WINDOW });
    await settle();
    const later = { split: app.expansions.join(","), pinned: pinnedID(app) };
    assert.deepEqual({ echoed, later }, { echoed: { split: OPENS, pinned: OPENS }, later: { split: "", pinned: OPENS } });
  });

  // Mutation: drop clickNode's and glyphNode's doubled guard and this goes
  // red, the second click of a double click folding the hub the first opened.
  test("a double click on a node or its glyph is one activation, and a click beyond the window folds", async (t) => {
    const seen = {};
    for (const [on, target] of [
      ["node", (app, document) => markOf(app, document, OPENS)],
      ["glyph", (app, document) => glyphOf(app, document, OPENS)],
    ]) {
      const { app, document } = await bootedApp();
      const state = () => ({ split: app.expansions.join(","), glyph: glyphKind(document, OPENS) });
      fire(target(app, document), "click", { timeStamp: 1000 });
      await settle();
      fire(target(app, document), "click", { timeStamp: 1300 });
      await settle();
      const doubled = state();
      fire(target(app, document), "click", { timeStamp: 1000 + app.ACTIVATION_WINDOW });
      await settle();
      seen[on] = { doubled, later: state() };
    }
    t.diagnostic(`two clicks 300ms apart, then one at the window's edge: ${JSON.stringify(seen)}`);
    const want = { doubled: { split: OPENS, glyph: "fold" }, later: { split: "", glyph: "open" } };
    assert.deepEqual(seen, { node: want, glyph: want });
  });

  test("a click on a node that does not split pins it, and splits and follows nothing", async (t) => {
    const { app, document } = await bootedApp();
    await split(app, WORST);
    const inert = inertMember(app, WORST);
    fire(markOf(app, document, inert), "click", { timeStamp: 1000 });
    await settle();
    const clicked = { split: app.expansions.join(","), pinned: pinnedID(app), isolated: app.filter.isolated };
    // The drawer names it, and offers to follow its money.
    const follow = document.getElementById("detail-follow");
    const offered = { hidden: follow.hidden, pressed: follow.getAttribute("aria-pressed") };
    fire(follow, "click");
    const followed = { isolated: app.filter.isolated, pressed: follow.getAttribute("aria-pressed") };
    fire(follow, "click");
    t.diagnostic(`a click on ${inert}: ${JSON.stringify(clicked)}; its Follow button ${JSON.stringify(offered)} then ${JSON.stringify(followed)}`);
    assert.deepEqual(clicked, { split: WORST, pinned: inert, isolated: "" });
    assert.deepEqual(offered, { hidden: false, pressed: "false" });
    assert.deepEqual(followed, { isolated: inert, pressed: "true" });
    assert.equal(app.filter.isolated, "");
  });

  // Mutation: drop the clear in pin and the second mark's pin leaves the
  // chart following the first, the drawer's Follow unpressed.
  test("pinning another mark lets a followed node go: a node's pin, a ribbon's, the Pin button's release", async (t) => {
    const { app, document } = await bootedApp();
    await split(app, WORST);
    const members = app.splitsHere({ id: WORST })[0].members
      .filter((id) => app.laidNodes.some((n) => n.id === id) && !app.splittable({ id }) && !app.expandable({ id }));
    assert.ok(members.length >= 2, `${WORST} draws ${members.length} inert members`);
    const [first, second] = members;
    const follow = document.getElementById("detail-follow");
    // Clicks far enough apart that none is the second of a double click.
    let at = 1000;
    const click = async (id) => {
      fire(markOf(app, document, id), "click", { timeStamp: (at += 4000) });
      await settle();
    };
    const followFirst = async () => {
      await click(first);
      fire(follow, "click");
      return app.filter.isolated;
    };
    const state = () => ({
      pinned: pinnedID(app), isolated: app.filter.isolated,
      follow: follow.hidden ? "hidden" : follow.getAttribute("aria-pressed"),
      firstPressed: markOf(app, document, first).getAttribute("aria-pressed"),
    });
    const seen = {};
    seen.followed = await followFirst();
    await click(second);
    seen.node = state();
    seen.again = await followFirst();
    const ribbon = [...document.querySelectorAll("#chart path.link")].find((p) => p.__data__.target.id !== first && p.__data__.source.id !== first);
    fire(ribbon, "click");
    seen.ribbon = Object.assign(state(), { ribbonPinned: app.pinned === ribbon.__data__ });
    seen.last = await followFirst();
    fire(document.getElementById("detail-pin"), "click");
    seen.unpinned = state();
    t.diagnostic(`follow ${first}, then: ${JSON.stringify(seen)}`);
    assert.deepEqual(seen, {
      followed: first,
      node: { pinned: second, isolated: "", follow: "false", firstPressed: "false" },
      again: first,
      ribbon: { pinned: "", isolated: "", follow: "hidden", firstPressed: "false", ribbonPinned: true },
      last: first,
      unpinned: { pinned: "", isolated: "", follow: "hidden", firstPressed: "false" },
    });
  });

  // Mutation: let pin clear a group's isolation too and the swatch is let go.
  test("a fund group a legend swatch follows survives a pin", async () => {
    const { app, document } = await bootedApp();
    const swatch = document.querySelector("#legend button");
    const group = swatch.dataset.group;
    fire(swatch, "click");
    const inert = app.laidNodes.find((n) => !app.offers(n) && n.id !== group);
    fire(markOf(app, document, inert.id), "click", { timeStamp: 1000 });
    await settle();
    assert.deepEqual(
      { pinned: pinnedID(app), isolated: app.filter.isolated, pressed: swatch.getAttribute("aria-pressed") },
      { pinned: inert.id, isolated: group, pressed: "true" });
  });

  // Mutation: drop the clear in pin and the split of one node keeps another
  // followed.
  test("an activation that splits keeps the isolation only where the hub it pins is the node followed", async (t) => {
    const { app, document } = await bootedApp();
    const at = app.laidNodes.find((n) => app.splittable(n) && !app.isGroupID(n.id));
    assert.ok(at, "the opening chart draws a node that splits and is no fund group");
    const hub = at.id;
    const other = app.laidNodes.find((n) => n.id !== hub && !app.isGroupID(n.id)).id;
    const seen = {};
    app.setIsolated(other);
    fire(markOf(app, document, hub), "click", { timeStamp: 1000 });
    await settle();
    seen.another = { split: app.expansions.join(","), pinned: pinnedID(app), isolated: app.filter.isolated };
    // Folded back, then followed and split again.
    fire(markOf(app, document, hub), "click", { timeStamp: 5000 });
    await settle();
    app.setIsolated(hub);
    fire(markOf(app, document, hub), "click", { timeStamp: 9000 });
    await settle();
    seen.itself = { split: app.expansions.join(","), pinned: pinnedID(app), isolated: app.filter.isolated };
    t.diagnostic(`${hub} split while following ${other}, then itself: ${JSON.stringify(seen)}`);
    assert.deepEqual(seen, {
      another: { split: hub, pinned: hub, isolated: "" },
      itself: { split: hub, pinned: hub, isolated: hub },
    });
  });

  // Mutation: fold on ⊖ by collapseNode alone (glyphNode not through
  // foldPinned) and the glyph arm goes red: the fold drops the pin and the
  // node's Follow with it, where the second click keeps both. A node and
  // not a fund group, whose isolation a redraw keeps without its pin.
  test("folding the node followed keeps it pinned and followed, by a second click and by its ⊖ alike", async (t) => {
    const seen = {};
    for (const gesture of ["click", "glyph"]) {
      const { app, document } = await bootedApp();
      await split(app, TWO_WAYS);
      app.followNode(TWO_WAYS);
      const target = gesture === "click" ? markOf(app, document, TWO_WAYS) : glyphOf(app, document, TWO_WAYS);
      fire(target, "click", { timeStamp: 1000 });
      await settle();
      seen[gesture] = { split: app.expansions.join(","), pinned: pinnedID(app), isolated: app.filter.isolated };
    }
    t.diagnostic(JSON.stringify(seen));
    const want = { split: "", pinned: TWO_WAYS, isolated: TWO_WAYS };
    assert.deepEqual(seen, { click: want, glyph: want });
  });

  // Mutation: drop the clear in fail and the chart follows a node with no
  // Follow button on screen to let it go.
  test("a refusal lets a followed node go with the pin", async () => {
    const { app, document } = await bootedApp();
    await split(app, WORST);
    const inert = inertMember(app, WORST);
    fire(markOf(app, document, inert), "click", { timeStamp: 1000 });
    await settle();
    fire(document.getElementById("detail-follow"), "click");
    const followed = app.filter.isolated;
    app.fail("refused");
    assert.deepEqual(
      { followed, pinned: pinnedID(app), isolated: app.filter.isolated, pressed: markOf(app, document, inert).getAttribute("aria-pressed") },
      { followed: inert, pinned: "", isolated: "", pressed: "false" });
  });
});

describe("the glyphs", () => {
  test("⊖ beside an open hub folds it, and every split inside it, back; ⊕ beside it again", async (t) => {
    const { app, document } = await bootedApp();
    await split(app, OPENS, WORST);
    const before = { opens: glyphKind(document, OPENS), worst: glyphKind(document, WORST) };
    fire(glyphOf(app, document, WORST), "click", { timeStamp: 1000 });
    await settle();
    const folded = { split: app.expansions.join(","), worst: glyphKind(document, WORST) };
    // Enter on fund/100's ⊖ folds what it opened straight through from.
    fire(glyphOf(app, document, "fund/100"), "keydown", { key: "Enter", timeStamp: 5000 });
    await settle();
    const through = { split: app.expansions.join(","), opens: glyphKind(document, OPENS) };
    t.diagnostic(`${JSON.stringify(before)}; ⊖ on ${WORST}: ${JSON.stringify(folded)}; Enter on fund/100's ⊖: ${JSON.stringify(through)}`);
    assert.deepEqual(before, { opens: "fold", worst: "fold" });
    assert.deepEqual(folded, { split: OPENS, worst: "open" });
    assert.deepEqual(through, { split: "", opens: "open" });
    assert.deepEqual(refusals(document).map((b) => b.textContent), []);
  });

  test("a glyph's key and the click it synthesises are one activation", async () => {
    const { app, document } = await bootedApp();
    const g = glyphOf(app, document, OPENS);
    fire(g, "keydown", { key: "Enter", timeStamp: 1000 });
    await settle();
    // The same element, now ⊖: its synthesised click must not fold it.
    fire(g, "click", { timeStamp: 1050 });
    await settle();
    assert.deepEqual(app.expansions, [OPENS]);
  });

  test("glyphs carry the page's words and are reached by the keyboard", async () => {
    const { app, document, config } = await bootedApp();
    const g = glyphOf(app, document, OPENS);
    const label = app.nodeOf(OPENS).label;
    const split0 = app.splitWords(app.splitsHere({ id: OPENS })[0].key);
    assert.deepEqual({ role: g.getAttribute("role"), tab: g.getAttribute("tabindex"), label: g.getAttribute("aria-label") },
      { role: "button", tab: "0", label: config.wording.glyph_open.replace("{label}", label).replace("{split}", split0) });
    assert.equal(glyphOf(app, document, TWO_WAYS).getAttribute("aria-label"),
      config.wording.glyph_choose.replace("{label}", app.nodeOf(TWO_WAYS).label));
    await split(app, OPENS);
    assert.equal(glyphOf(app, document, OPENS).getAttribute("aria-label"), config.wording.glyph_fold.replace("{label}", label));
  });
});

describe("a node that splits two ways", () => {
  test("an activation takes the first declared split, and ⊕ offers both in a menu", async (t) => {
    const { app, document } = await bootedApp();
    const splits = app.splitsHere({ id: TWO_WAYS }).map((sp) => sp.key);
    assert.ok(splits.length === 2, `${TWO_WAYS} splits ${splits.length} way(s)`);
    fire(markOf(app, document, TWO_WAYS), "click", { timeStamp: 0 });
    await settle();
    assert.deepEqual(app.expansions, [TWO_WAYS]);

    // ⊕ beside the node shut opens a menu of its ways; Escape there closes
    // it and lets nothing else go.
    app.collapseAll();
    await settle();
    const glyph = glyphOf(app, document, TWO_WAYS);
    assert.equal(glyph.getAttribute("aria-haspopup"), "true");
    fire(glyph, "click", { timeStamp: 2000 });
    const menu = document.getElementById("split-menu");
    assert.ok(menu, "⊕ on a node that splits two ways opened no menu");
    const offered = [...menu.querySelectorAll("button")].map((b) => b.textContent);
    const focused = document.activeElement === menu.querySelector("button");
    fire(menu, "keydown", { key: "Escape" });
    const closed = !document.getElementById("split-menu");
    fire(glyph, "click", { timeStamp: 3000 });
    [...document.querySelectorAll("#split-menu button")][1].click();
    await settle();
    t.diagnostic(`${TWO_WAYS}: ⊕'s menu ${JSON.stringify(offered)}; ` +
      `its second split drew [${app.expansions}]`);
    assert.deepEqual(offered, splits.map((k) => app.splitWords(k)));
    assert.ok(focused, "the menu opened without focus on its first way");
    assert.ok(closed, "Escape in the menu left it open");
    assert.deepEqual(app.expansions, [TWO_WAYS + app.ALTERNATIVE_MARK + splits[1]]);
    assert.equal(pinnedID(app), TWO_WAYS);
    assert.equal(document.getElementById("split-menu"), null);
    assert.deepEqual(refusals(document).map((b) => b.textContent), []);
  });
});

describe("the ⊕ menu under a mouse", () => {
  // Mutation: let focusout close the menu during a press in it again and
  // this goes red: the menu is gone before the click, and nothing splits.
  test("a choice clicked after its press blurred the menu to nothing still splits the node by it", async (t) => {
    const { app, document } = await bootedApp();
    const splits = app.splitsHere({ id: TWO_WAYS }).map((sp) => sp.key);
    fire(glyphOf(app, document, TWO_WAYS), "click", { timeStamp: 2000 });
    const choice = [...document.querySelectorAll("#split-menu button")][1];
    fire(choice, "pointerdown", { button: 0 });
    document.activeElement.dispatchEvent(new document.defaultView.FocusEvent("focusout", { bubbles: true, relatedTarget: null }));
    const open = Boolean(document.getElementById("split-menu"));
    fire(choice, "pointerup", { button: 0 });
    fire(choice, "click", { button: 0 });
    await settle();
    t.diagnostic(`the menu open after the press blurred it: ${open}; the click split [${app.expansions}]`);
    assert.deepEqual({ open, split: app.expansions.slice(), menu: document.getElementById("split-menu") === null },
      { open: true, split: [TWO_WAYS + app.ALTERNATIVE_MARK + splits[1]], menu: true });
  });
});

describe("folding a split back", () => {
  test("folding a node folds it, and every split inside it, back into one mark", async (t) => {
    const { app } = await bootedApp();
    await split(app, OPENS, WORST);
    const opened = app.expansions.slice();
    app.collapseNode(OPENS);
    await settle();
    t.diagnostic(`[${opened}] split; folding ${OPENS} folded fund/100 with it`);
    assert.deepEqual(app.expansions, [WORST]);
    assert.ok(app.laidNodes.some((n) => n.id === OPENS), "the folded node is not drawn again");
  });

  // Mutation: restore Escape undoing the last split once nothing is pinned or
  // followed, and this goes red at the second Escape.
  test("Escape lets the pin and the isolation go, and folds no split", async (t) => {
    const { app, document } = await bootedApp();
    await split(app, OPENS, WORST);
    const inert = inertMember(app, WORST);
    fire(markOf(app, document, inert), "click", { timeStamp: 1000 });
    app.setIsolated(inert);
    const held = { split: app.expansions.join(","), pinned: pinnedID(app) };
    escape(document);
    await settle();
    const first = { split: app.expansions.join(","), isolated: app.filter.isolated, pinned: pinnedID(app) };
    escape(document);
    await settle();
    const second = app.expansions.join(",");
    t.diagnostic(`Escape: ${JSON.stringify(first)}, then [${second}]`);
    assert.deepEqual(held, { split: OPENS + "," + WORST, pinned: inert });
    assert.deepEqual(first, { split: OPENS + "," + WORST, isolated: "", pinned: "" });
    assert.equal(second, OPENS + "," + WORST, "Escape with nothing pinned folded a split");
    assert.deepEqual(refusals(document).map((b) => b.textContent), []);
  });

  // Mutation: let Escape clear only what unpin clears and this goes red: a
  // fund group's swatch stays pressed and the chart dimmed.
  test("Escape lets a fund group's isolation go too", async () => {
    const { app, document } = await bootedApp();
    app.setIsolated(OPENS);
    escape(document);
    await settle();
    assert.equal(app.filter.isolated, "");
  });

  // Mutation: hand focus over whenever the pressed button ends disabled,
  // held or not, and this goes red: a click that never took focus (Safari's)
  // leaves it on Collapse all, which a later Space or Enter presses.
  test("a bulk button its own press disables hands focus to the other only where it held focus", async () => {
    const keyedPage = await bootedApp();
    const [all] = bulk(keyedPage.document);
    all.focus();
    all.click();
    await settle();
    const keyed = keyedPage.document.activeElement.id;
    const { document } = await bootedApp();
    const before = document.activeElement.id || document.activeElement.tagName;
    const [all2] = bulk(document);
    all2.click();
    await settle();
    const clicked = document.activeElement.id || document.activeElement.tagName;
    assert.deepEqual({ keyed, before, clicked, disabled: all2.disabled },
      { keyed: "chart-collapse-all", before: "BODY", clicked: "BODY", disabled: true });
  });

  test("Expand all splits every node to its finest grain by its first split, and Collapse all folds everything back", async (t) => {
    const { app, document } = await bootedApp();
    const [all, none] = bulk(document);
    assert.ok(all && none, "the toolbar carries no Expand all and Collapse all");
    assert.deepEqual([all.hidden, none.hidden], [false, false]);
    assert.deepEqual([all.textContent, none.textContent], [app.say("expand_all"), app.say("collapse_all")]);
    assert.deepEqual([all.disabled, none.disabled], [false, true]);
    const roots = app.laidNodes.length;
    all.click();
    await settle();
    const opened = { split: app.expansions.length, offers: offers(app), laid: app.laidNodes.length };
    const [all2, none2] = bulk(document);
    assert.deepEqual([all2.disabled, none2.disabled], [true, false]);
    none2.click();
    await settle();
    t.diagnostic(`the root lays ${roots} marks; Expand all made ${opened.split} split(s) and lays ${opened.laid}`);
    assert.ok(opened.split > 0 && opened.laid > roots);
    assert.equal(opened.offers, 0, "Expand all left a node that splits unsplit");
    assert.deepEqual(app.expansions, []);
    assert.equal(app.laidNodes.length, roots);
  });
});

describe("the trace", () => {
  /** The ids of the ribbons and marks drawn on the path, and whether the chart is tracing. */
  const lit = (document) => ({
    tracing: document.getElementById("chart").classList.contains("tracing"),
    ribbons: [...document.querySelectorAll("#chart path.link.on-path")].map((p) => p.__data__.source.id + ">" + p.__data__.target.id),
    marks: [...document.querySelectorAll("#chart g.node.on-path")].map((g) => g.__data__.id),
  });

  // Mutation: drop the trace's call from the node's pointerenter and this
  // goes red, nothing on the path.
  test("hovering a node draws its path upstream and downstream over the rest, and leaving lets it go", async (t) => {
    const { app, document } = await bootedApp();
    await split(app, WORST);
    app.setIsolated(OPENS);
    const dimmed = document.querySelectorAll("#chart .dim").length;
    const m = markOf(app, document, WORST);
    const d = m.__data__;
    fire(m, "pointerenter");
    const on = lit(document);
    // One ribbon in, one out, and one a hop further out: the path's own.
    const inOf = d.targetLinks.map((l) => l.source.id + ">" + l.target.id);
    const outOf = d.sourceLinks.map((l) => l.source.id + ">" + l.target.id);
    const hop = d.sourceLinks.flatMap((l) => l.target.sourceLinks).map((l) => l.source.id + ">" + l.target.id);
    const all = document.querySelectorAll("#chart path.link").length;
    fire(m, "pointerleave");
    const off = lit(document);
    t.diagnostic(`hovering ${WORST}: ${on.ribbons.length} of ${all} ribbon(s) and ${on.marks.length} mark(s) on its path`);
    assert.ok(on.tracing, "the chart is not tracing under the pointer");
    assert.ok(on.marks.includes(WORST));
    assert.deepEqual(inOf.concat(outOf, hop).filter((k) => !on.ribbons.includes(k)), [], "a ribbon on the path is not drawn on it");
    assert.ok(hop.length > 0 && inOf.length > 0, "the planted node has no path to trace");
    assert.ok(on.ribbons.length < all, "the trace drew every ribbon on its path");
    assert.deepEqual(off, { tracing: false, ribbons: [], marks: [] });
    assert.equal(document.querySelectorAll("#chart .dim").length, dimmed, "the trace's leave moved the isolation's dimming");
  });

  // Mutation: walk the laid topology alone through the hub (pathOf with no
  // leaf filter), and this goes red: every source of the split lights, and
  // every leg out of it.
  test("hovering a member of an open hub lights only the sources whose leaves it repeats, and a source only the legs its leaves reach", async (t) => {
    const { app, document } = await bootedApp();
    const hub = "fund-group/special-revenue";
    const member = "fund/210";
    await split(app, hub);
    const m = markOf(app, document, member);
    const key = (x) => x.source.id + ">" + x.target.id;
    const ribbons = [...document.querySelectorAll("#chart path.link")].map((p) => p.__data__);
    // The leaves a ribbon draws, read off its cells: a key not opening with the separator.
    const leaves = (l) => Object.keys(l.cells || {}).filter((k) => !k.startsWith("\u001f"));
    const legs = ribbons.filter((l) => l.source.id === hub && l.target.id === member);
    const mine = new Set(legs.flatMap(leaves));
    const into = ribbons.filter((l) => l.target.id === hub);
    const want = into.filter((l) => leaves(l).some((k) => mine.has(k))).map(key).sort();
    fire(m, "pointerenter");
    const on = lit(document);
    fire(m, "pointerleave");
    const got = on.ribbons.filter((k) => k.endsWith(">" + hub)).sort();
    // And down from one of those sources: only the legs its own leaves reach.
    const source = into.find((l) => key(l) === want[0]);
    const theirs = new Set(into.filter((l) => l.source === source.source).flatMap(leaves));
    const outOf = ribbons.filter((l) => l.source.id === hub);
    const wantLegs = outOf.filter((l) => leaves(l).some((k) => theirs.has(k))).map(key).sort();
    fire(markOf(app, document, source.source.id), "pointerenter");
    const legsLit = lit(document).ribbons.filter((k) => k.startsWith(hub + ">")).sort();
    t.diagnostic(`hovering ${member}: ${got.length} of ${into.length} ribbon(s) into ${hub} lit, ${want.length} carry its leaves; ` +
      `hovering ${source.source.id}: ${legsLit.length} of ${outOf.length} leg(s) lit, ${wantLegs.length} carry its leaves`);
    assert.ok(want.length > 0 && want.length < into.length, "every source of the hub or none carries the member's leaves, so this asserts nothing");
    assert.ok(wantLegs.length < outOf.length, "the source's leaves reach every member, so this asserts nothing");
    assert.deepEqual({ got, legsLit }, { got: want, legsLit: wantLegs });
  });

  // Mutation: carry the leaves a walk came by on past the member (pathOf's
  // `keys` taken as `came` at every node), and this goes red: the member's
  // ribbons into the second hub go dark, and up from them its leg in does.
  test("through two open hubs in a chain, a member's own ribbons on and back stay on the path, each hub narrowing by its own leaves", async (t) => {
    const { app, document } = await bootedApp();
    const hub = WORST;
    const member = "fund/210";
    const next = "expenditure/wages-and-benefits";
    await split(app, hub, next);
    const key = (x) => x.source.id + ">" + x.target.id;
    const paths = [...document.querySelectorAll("#chart path.link")];
    const ribbons = paths.map((p) => p.__data__);
    const leaves = (l) => Object.keys(l.cells || {}).filter((k) => !k.startsWith("\u001f"));
    const leg = paths.find((p) => key(p.__data__) === hub + ">" + member);
    const on = paths.find((p) => key(p.__data__) === member + ">" + next);
    assert.ok(leg && on, `${hub} -> ${member} or ${member} -> ${next} is not drawn`);
    // Down from the leg: the member's ribbon into the second hub.
    fire(leg, "pointerenter");
    const down = lit(document).ribbons;
    fire(leg, "pointerleave");
    // Up from the member's ribbon on: its leg in, and through the first hub
    // only the sources whose leaves the member repeats.
    fire(on, "pointerenter");
    const up = lit(document).ribbons;
    fire(on, "pointerleave");
    const mine = new Set(ribbons.filter((l) => key(l) === hub + ">" + member).flatMap(leaves));
    const into = ribbons.filter((l) => l.target.id === hub);
    const want = into.filter((l) => leaves(l).some((k) => mine.has(k))).map(key).sort();
    const got = up.filter((k) => k.endsWith(">" + hub)).sort();
    t.diagnostic(`hovering ${hub} -> ${member}: ${member} -> ${next} lit ${down.includes(key(on.__data__))}; ` +
      `hovering ${member} -> ${next}: leg in lit ${up.includes(key(leg.__data__))}, ${got.length} of ${into.length} ribbon(s) into ${hub} lit, ${want.length} carry its leaves`);
    assert.ok(want.length > 0 && want.length < into.length, "every source of the hub or none carries the member's leaves, so this asserts nothing");
    assert.deepEqual(
      { onward: down.includes(key(on.__data__)), upstream: up.includes(key(leg.__data__)), got },
      { onward: true, upstream: true, got: want });
  });

  test("hovering a ribbon draws it, its source's path upstream and its target's downstream", async () => {
    const { app, document } = await bootedApp();
    const r = [...document.querySelectorAll("#chart path.link")].find((p) => p.__data__.target.id === OPENS);
    assert.ok(r, `no ribbon into ${OPENS}`);
    const l = r.__data__;
    fire(r, "pointerenter");
    const on = lit(document);
    const key = (x) => x.source.id + ">" + x.target.id;
    // A sibling ribbon into the same node is on neither path.
    const sibling = l.target.targetLinks.find((x) => x.source.id !== l.source.id);
    fire(r, "pointerleave");
    assert.ok(on.ribbons.includes(key(l)));
    assert.ok(l.target.sourceLinks.every((x) => on.ribbons.includes(key(x))), "the target's ribbons out are not on the path");
    assert.ok(sibling && !on.ribbons.includes(key(sibling)), "a sibling ribbon is drawn on the path");
    assert.deepEqual(lit(document), { tracing: false, ribbons: [], marks: [] });
  });
});

for (const year of YEARS) {
  describe(`${year.label}: the folded tail`, () => {
    test(`${year.label}: Space or Enter on a split's tail draws its members out, and folding the split and opening it again folds them back into the tail`, async (t) => {
      const { app, document } = await onYear(year.stem);
      await split(app, WORST);
      const tail = app.projection.nodes.find((n) => app.isAggregate(n.id));
      assert.ok(tail, `${WORST} drew no folded tail`);
      const offered = app.projection.nodes.filter((n) => app.expandable(n)).map((n) => n.id);
      assert.deepEqual(offered, [tail.id], "the tail is the one mark the chart offers to draw out");
      const capped = app.projection.nodes.length;

      for (const key of [" ", "Enter"]) {
        const m = markOf(app, document, tail.id);
        m.focus();
        const before = new Set(app.laidNodes.map((n) => n.id));
        fire(m, "keydown", { key, timeStamp: 0 });
        await settle();
        const drawnOut = app.projection.nodes.length;
        const members = new Set(app.splitsHere({ id: WORST })[0].members);
        const fresh = app.laidNodes.filter((n) => members.has(n.id) && !before.has(n.id)).sort((a, b) => a.y0 - b.y0);
        const on = focusedID(document);
        assert.deepEqual(app.expansions, [WORST, tail.id], JSON.stringify(key));
        assert.ok(fresh.length > 0, "drawing the tail out drew no new member");
        assert.equal(on, fresh[0].id, "focus is not on the topmost member the tail drew out");
        assert.ok(app.pinned === null, "restoring focus pinned the mark");
        app.collapseNode(WORST);
        await split(app, WORST);
        assert.deepEqual(app.expansions, [WORST]);
        assert.equal(app.projection.nodes.length, capped);
        t.diagnostic(`${WORST} capped draws ${capped} node(s); ${JSON.stringify(key)} on "${tail.label}" drew ${drawnOut}, focus on ${on}`);
      }
    });
  });
}

describe("a split of one member", () => {
  // fund-group/general's one member is fund/100, in both pinned years.
  const ONLY = "fund/100";
  /** The drawn marks fund/100's own split opens into. */
  const divisionsDrawn = (app) => {
    const members = new Set(app.splitsHere({ id: ONLY })[0].members);
    return app.laidNodes.filter((n) => members.has(n.id)).length;
  };

  for (const year of YEARS) {
    test(`${year.label}: opens straight through, in one gesture and one step, and folds back whole`, async (t) => {
      const { app, document } = await onYear(year.stem);
      assert.equal(app.splitsHere({ id: OPENS })[0].members.length, 1, `${OPENS} no longer splits into one member`);
      fire(markOf(app, document, OPENS), "click", { timeStamp: 0 });
      await settle();
      const opened = { x: app.expansions.slice(), divisions: divisionsDrawn(app), offers: app.splittable({ id: ONLY }) };
      t.diagnostic(`${year.label}: a click on ${OPENS} drew ${opened.divisions} of ${ONLY}'s divisions with [${opened.x}] split`);
      assert.ok(opened.divisions > 0, `one activation of ${OPENS} drew none of ${ONLY}'s divisions`);
      // ⊖ on the member folds back what its parent opened straight through from.
      fire(glyphOf(app, document, ONLY), "click", { timeStamp: 1000 });
      await settle();
      const folded = { x: app.expansions.slice(), divisions: divisionsDrawn(app) };
      fire(markOf(app, document, OPENS), "keydown", { key: "Enter", timeStamp: 5000 });
      await settle();
      fire(glyphOf(app, document, OPENS), "click", { timeStamp: 9000 });
      await settle();
      const hub = { x: app.expansions.slice(), divisions: divisionsDrawn(app) };
      assert.deepEqual(opened.x, [OPENS]);
      assert.equal(opened.offers, false, `${ONLY} still offers to split once its group is open`);
      assert.deepEqual(folded, { x: [], divisions: 0 });
      assert.deepEqual(hub, { x: [], divisions: 0 });
      assert.deepEqual(refusals(document).map((b) => b.textContent), []);
    });
  }
});

describe("focus after a split", () => {
  test("lands on the topmost new member with the node opened pinned, and ⊖ puts it back on the node", async (t) => {
    const { app, document } = await bootedApp();
    const m = markOf(app, document, OPENS);
    m.focus();
    fire(m, "keydown", { key: "Enter", timeStamp: 1000 });
    await settle();
    const members = new Set(app.splitsHere({ id: OPENS })[0].members);
    const top = app.laidNodes.filter((n) => members.has(n.id)).sort((a, b) => a.y0 - b.y0)[0];
    const on = focusedID(document);
    t.diagnostic(`after Enter on ${OPENS} focus is on ${on}, ${pinnedID(app)} pinned`);
    assert.equal(on, top.id);
    assert.equal(pinnedID(app), OPENS, "the drawer does not hold the node opened");
    const g = glyphOf(app, document, OPENS);
    g.focus();
    fire(g, "keydown", { key: "Enter", timeStamp: 5000 });
    await settle();
    assert.deepEqual(app.expansions, []);
    assert.equal(focusedID(document), OPENS);
  });
});

describe("the search box", () => {
  /** Types `text` into the search box as a reader does: the value, then input. */
  const type = (document, text) => {
    const input = document.getElementById("search-input");
    input.focus();
    input.value = text;
    input.dispatchEvent(new globalThis.Event("input", { bubbles: true }));
    return input;
  };
  /** One key on the search box. */
  const key = (input, k) => fire(input, "keydown", { key: k });
  /** The options listed, as the ids of the marks they name. */
  const options = (document) => [...document.querySelectorAll("#search-list [role=option]")].map((o) => o.dataset.id);
  /** Arrows down until the option naming `id` is the active one. */
  const arrowTo = (document, input, id) => {
    const at = options(document).indexOf(id);
    assert.ok(at >= 0, `no option names ${id}: ${options(document).join(", ")}`);
    for (let k = 0; k <= at; k++) key(input, "ArrowDown");
    const active = document.getElementById(input.getAttribute("aria-activedescendant"));
    assert.equal(active.dataset.id, id);
    assert.equal(active.getAttribute("aria-selected"), "true");
    return active;
  };
  const banners = (document) => refusals(document).map((b) => b.textContent);

  test("typing lists the marks drawn whose label holds it, the arrows move along them, and Enter splits one the chart's own Enter splits", async (t) => {
    const { app, document } = await bootedApp();
    assert.equal(document.getElementById("search").hasAttribute("hidden"), false);
    const input = type(document, "general");
    const listed = options(document);
    const status = document.getElementById("search-status").textContent;
    t.diagnostic(`"general" lists ${JSON.stringify(listed)}; the status says "${status}"`);
    assert.ok(listed.includes(OPENS));
    assert.equal(input.getAttribute("role"), "combobox");
    assert.equal(input.getAttribute("aria-expanded"), "true");
    assert.equal(status, app.say("search_count", { n: listed.length }));
    const active = arrowTo(document, input, OPENS);
    assert.ok(active.textContent.includes(app.say("search_opens")), active.textContent);
    // Up and back down lands on it again, wrapping through the last option where it is the first.
    key(input, "ArrowUp");
    key(input, "ArrowDown");
    assert.equal(document.getElementById(input.getAttribute("aria-activedescendant")).dataset.id, OPENS);
    key(input, "Enter");
    await settle();
    assert.deepEqual(app.expansions, [OPENS]);
    assert.deepEqual(banners(document), []);
    assert.equal(input.value, "");
    assert.equal(input.getAttribute("aria-expanded"), "false");
  });

  test("Enter on a mark that splits nothing follows its money and focuses it, as the chart's own Enter does", async (t) => {
    const { app, document } = await bootedApp();
    await split(app, WORST);
    const inert = inertMember(app, WORST);
    const input = type(document, app.nodeOf(inert).label);
    const active = arrowTo(document, input, inert);
    assert.ok(!active.textContent.includes(app.say("search_opens")), active.textContent);
    key(input, "Enter");
    await settle();
    const on = focusedID(document);
    t.diagnostic(`Enter on "${active.textContent}" left [${app.expansions.join(", ")}] split, following ${app.filter.isolated}, focus on ${on}`);
    assert.deepEqual(app.expansions, [WORST]);
    assert.equal(app.filter.isolated, inert);
    assert.equal(on, inert);
    assert.equal(app.pinned && app.pinned.id, inert);
    assert.deepEqual(banners(document), []);
  });

  test("a mark the filters removed is not offered, and Escape in the box undoes no split", async () => {
    const { app, document } = await bootedApp();
    app.setFilter({ groups: new Set([OPENS]) });
    const input = type(document, "general");
    assert.ok(!options(document).includes(OPENS), "a mark the filters removed is offered");
    app.resetFilters();
    await split(app, OPENS);
    type(document, "fund");
    assert.equal(input.getAttribute("aria-expanded"), "true");
    key(input, "Escape");
    assert.equal(input.getAttribute("aria-expanded"), "false");
    assert.equal(input.value, "fund");
    key(input, "Escape");
    assert.equal(input.value, "");
    await settle();
    assert.deepEqual(app.expansions, [OPENS], "Escape in the search box undid a split");
    assert.deepEqual(banners(document), []);
  });

  test("a node the chart does not draw is offered, and Enter splits the nodes above it until it is drawn, and focuses it", async (t) => {
    const { app, document } = await bootedApp();
    const id = "dept/patrol";
    const input = type(document, app.nodeOf(id).label);
    const active = arrowTo(document, input, id);
    assert.ok(active.textContent.includes(app.say("search_reveals")), active.textContent);
    key(input, "Enter");
    await settle();
    const on = focusedID(document);
    t.diagnostic(`Enter on "${active.textContent}" split [${app.expansions.join(", ")}]; focus on ${on}`);
    assert.deepEqual(app.expansions.slice(0, 1), ["fund-group/general"]);
    assert.ok(app.laidNodes.some((n) => n.id === id), `${id} is not drawn`);
    assert.equal(on, id);
    assert.equal(input.value, "");
    assert.deepEqual(banners(document), []);
  });

  test("a node folded into a tail is revealed with the tail drawn out, and an alternative's member by its split", async (t) => {
    const { app, document } = await bootedApp();
    await split(app, WORST);
    const folded = app.splitsHere({ id: WORST })[0].members.find((m) => !app.laidNodes.some((n) => n.id === m));
    assert.ok(folded, `${WORST}'s split folds no member`);
    app.collapseAll();
    await settle();
    assert.ok(app.chooseSearch(folded, true), `${folded} was not revealed`);
    await settle();
    const tailed = app.expansions.slice();
    const alternative = "dept/information-technology/capital-outlay";
    app.collapseAll();
    await settle();
    assert.ok(app.chooseSearch(alternative, true), `${alternative} was not revealed`);
    await settle();
    t.diagnostic(`${folded}: [${tailed.join(", ")}]; ${alternative}: [${app.expansions.join(", ")}]`);
    assert.deepEqual(tailed, [WORST, app.splitTailID(WORST, "funds")]);
    assert.deepEqual(app.expansions, ["expenditure/capital-outlay" + app.ALTERNATIVE_MARK + "divisions"]);
    assert.ok(app.laidNodes.some((n) => n.id === alternative));
    assert.deepEqual(banners(document), []);
  });

  test("a reveal of a node keeps its own extend split open", async () => {
    const { app } = await bootedApp();
    const entry = TWO_WAYS + app.ALTERNATIVE_MARK + "divisions";
    await split(app, entry);
    assert.deepEqual(app.revealEntries(TWO_WAYS), [entry]);
  });

  test("a node the smallest-flow filter would remove once revealed leaves the chart as it is, and says so", async () => {
    const { app, document } = await bootedApp();
    app.setFilter({ smallest: Number.MAX_SAFE_INTEGER });
    await settle();
    assert.deepEqual(banners(document), []);
    const id = "dept/patrol";
    assert.equal(app.chooseSearch(id, true), false, `${id} was revealed past the filter`);
    await settle();
    assert.deepEqual(app.expansions, []);
    assert.equal(banners(document).length, 1, "the refusal is not said");
    assert.ok(banners(document)[0].includes(app.nodeOf(id).label), banners(document)[0]);
  });

  test("a label nothing in the schedule carries offers nothing, and Enter on it does nothing", async () => {
    const { app, document } = await bootedApp();
    const input = type(document, "no such label anywhere");
    assert.deepEqual(options(document), []);
    assert.equal(input.getAttribute("aria-expanded"), "false");
    assert.equal(document.getElementById("search-status").textContent, app.say("search_none", { query: "no such label anywhere" }));
    key(input, "ArrowDown");
    key(input, "Enter");
    await settle();
    assert.deepEqual(app.expansions, []);
    assert.equal(app.filter.isolated, "");
  });

  test("a click on an option follows that mark without splitting it", async () => {
    const { app, document } = await bootedApp();
    type(document, "general");
    const option = [...document.querySelectorAll("#search-list [role=option]")].find((o) => o.dataset.id === OPENS);
    fire(option, "click");
    await settle();
    assert.deepEqual(app.expansions, []);
    assert.equal(app.filter.isolated, OPENS);
    assert.equal(focusedID(document), OPENS);
  });
});

