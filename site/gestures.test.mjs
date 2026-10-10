// gestures.test.mjs -- the SEQUENCES a reader's gestures make on the drawn
// chart: which key splits and which follows the money, an isolation surviving
// the two clicks under a double click, the chooser between a node's two
// splits, the chip, Alt or Shift with a double click and Escape folding a
// split back, Expand all and Collapse all, a tail drawn out, and where focus
// lands after a split has replaced the element it was on.
//
// Every gesture is a dispatched event with a planted timeStamp, because the
// activation guard compares a click's stamp against the key that may have
// synthesised it. Every assertion is over ids, counts and booleans: a failing
// assertion over a laid mark formats the whole graph (fisc-yj4w.33).

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

/** The id of the mark focus is on, or what else it is on. */
function focusedID(document) {
  const a = document.activeElement;
  if (!a) return "nothing";
  return a.__data__ && typeof a.__data__.id === "string" ? a.__data__.id : a.tagName + "." + a.getAttribute("class");
}

/** The chip row's fold-back chips, as their words. */
function chips(document) {
  return [...document.querySelectorAll("#breadcrumb button.crumb-expanded")];
}

/** The chip row's Expand all and Collapse all buttons, in that order. */
function bulk(document) {
  return [...document.querySelectorAll("#breadcrumb button.crumb-bulk")];
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

describe("the two keys", () => {
  test("Enter splits and Space isolates, and neither does the other", async (t) => {
    const { app, document } = await bootedApp();
    const state = () => ({ split: app.expansions.join(","), isolated: app.filter.isolated });

    fire(markOf(app, document, OPENS), "keydown", { key: "Enter", timeStamp: 1000 });
    await settle();
    const enter = state();
    const kept = app.laidNodes.filter((n) => n.id === "revenue/taxes/property" || n.id === "transfers/in").length;
    app.collapseAll();
    await settle();
    fire(markOf(app, document, OPENS), "keydown", { key: " ", timeStamp: 5000 });
    await settle();
    const space = state();
    app.setIsolated("");
    app.collapseAll();
    await split(app, WORST);
    const inert = inertMember(app, WORST);
    // Enter still activates a mark with nothing to split: a role="button"
    // whose Enter does nothing is worse than one whose two keys agree.
    fire(markOf(app, document, inert), "keydown", { key: "Enter", timeStamp: 9000 });
    await settle();
    const inertEnter = state();

    assert.deepEqual(enter, { split: OPENS, isolated: "" });
    assert.equal(kept, 2, "splitting the General Fund group took another root off the chart");
    assert.deepEqual(space, { split: "", isolated: OPENS });
    assert.deepEqual(inertEnter, { split: WORST, isolated: inert });
    t.diagnostic(`Enter on ${OPENS} split it in place; Space on it followed it; Enter on ${inert} followed it`);
  });

  test("a double click splits, and leaves the isolation the reader had", async (t) => {
    // THE SEED IS THE WHOLE TEST. A double click delivers click, click,
    // dblclick, and on a chart following nothing those two clicks toggle an
    // isolation on and off and land back on "" without any help. It starts
    // from a mark the reader was already following, the only state in which
    // the two answers differ.
    const { app, document } = await bootedApp();
    await split(app, WORST);
    const inert = inertMember(app, WORST);
    const seed = "revenue/taxes/sales";
    fire(markOf(app, document, seed), "click", { timeStamp: 0 });
    const seeded = app.filter.isolated;
    fire(markOf(app, document, inert), "click", { timeStamp: 1000 });
    const firstClick = app.filter.isolated;
    fire(markOf(app, document, inert), "click", { timeStamp: 1050 });
    const secondClick = app.filter.isolated;
    fire(markOf(app, document, inert), "dblclick", { timeStamp: 1060 });
    await settle();
    const inertDouble = { split: app.expansions.join(","), isolated: app.filter.isolated };

    // A SECOND PAGE AND NOT THE SAME ONE WOUND BACK: seeding the same mark
    // again on a chart still following it clears the seed.
    const second = await bootedApp();
    const m2 = (id) => markOf(second.app, second.document, id);
    fire(m2(seed), "click", { timeStamp: 0 });
    fire(m2(OPENS), "click", { timeStamp: 1000 });
    fire(m2(OPENS), "click", { timeStamp: 1050 });
    fire(m2(OPENS), "dblclick", { timeStamp: 1060 });
    await settle();
    const opensDouble = { split: second.app.expansions.join(","), isolated: second.app.filter.isolated };

    assert.equal(seeded, seed);
    assert.equal(firstClick, inert);
    assert.equal(secondClick, "");
    assert.deepEqual(inertDouble, { split: WORST, isolated: seed });
    // The seed survives the split too: the sales tax is still drawn.
    assert.deepEqual(opensDouble, { split: OPENS, isolated: seed });
    t.diagnostic(`following "${seed}", a double click on ${inert} went "${firstClick}" then ` +
      `"${secondClick}" under the reader and came back to "${inertDouble.isolated}"; the same gesture on ${OPENS} split it`);
  });
});

describe("a node that splits two ways", () => {
  test("Enter and a double click take the first declared split, and the chooser in its chip draws the other", async (t) => {
    const { app, document } = await bootedApp();
    const splits = app.splitsHere({ id: TWO_WAYS }).map((sp) => sp.key);
    assert.ok(splits.length === 2, `${TWO_WAYS} splits ${splits.length} way(s)`);
    fire(markOf(app, document, TWO_WAYS), "dblclick", { timeStamp: 0 });
    await settle();
    assert.deepEqual(app.expansions, [TWO_WAYS]);
    const group = document.querySelector("#breadcrumb .crumb-group");
    assert.ok(group, "no chooser beside the chip of a node that splits two ways");
    assert.equal(group.getAttribute("role"), "group");
    assert.ok(group.getAttribute("aria-label").includes(app.nodeOf(TWO_WAYS).label), group.getAttribute("aria-label"));
    const choices = () => [...group.ownerDocument.querySelectorAll("#breadcrumb .crumb-choice")]
      .map((b) => ({ words: b.textContent, pressed: b.getAttribute("aria-pressed") }));
    const before = choices();
    assert.deepEqual(before, splits.map((k, i) => ({ words: app.splitWords(k), pressed: String(i === 0) })));
    // The chooser's other split replaces the first: one way per node.
    const other = [...document.querySelectorAll("#breadcrumb .crumb-choice")][1];
    other.click();
    await settle();
    assert.deepEqual(app.expansions, [TWO_WAYS + app.ALTERNATIVE_MARK + splits[1]]);
    const after = choices();
    assert.deepEqual(after.map((c) => c.pressed), ["false", "true"]);
    t.diagnostic(`${TWO_WAYS}: the chooser offers ${JSON.stringify(before.map((c) => c.words))}; ` +
      `pressing the second split it ${splits[1]}`);
    assert.deepEqual(refusals(document).map((b) => b.textContent), []);
  });
});

describe("folding a split back", () => {
  test("the node's chip folds it, and every split inside it, back into one mark", async (t) => {
    const { app, document } = await bootedApp();
    await split(app, OPENS, "fund/100", WORST);
    const chipWords = chips(document).map((c) => c.textContent);
    assert.equal(chipWords.length, 3);
    assert.ok(chipWords[0].includes(app.nodeOf(OPENS).label), chipWords[0]);
    chips(document)[0].click();
    await settle();
    t.diagnostic(`chips ${JSON.stringify(chipWords)}; the first folded the General Fund group and fund/100 with it`);
    assert.deepEqual(app.expansions, [WORST]);
    assert.ok(app.laidNodes.some((n) => n.id === OPENS), "the folded node is not drawn again");
  });

  test("Alt or Shift with a double click on a member folds back the node it was split out of", async () => {
    const { app, document } = await bootedApp();
    for (const held of ["altKey", "shiftKey"]) {
      await split(app, WORST);
      const member = inertMember(app, WORST);
      fire(markOf(app, document, member), "dblclick", { timeStamp: 0, [held]: true });
      await settle();
      assert.deepEqual(app.expansions, [], `${held} with a double click on ${member}`);
      assert.ok(app.laidNodes.some((n) => n.id === WORST), `${held}: ${WORST} is not drawn again`);
    }
    // On a root it folds nothing and splits nothing.
    fire(markOf(app, document, OPENS), "dblclick", { timeStamp: 5000, altKey: true });
    await settle();
    assert.deepEqual(app.expansions, []);
  });

  test("Escape undoes the last split, innermost first, after a pin and an isolation", async (t) => {
    const { app, document } = await bootedApp();
    await split(app, OPENS, "fund/100");
    app.setIsolated("revenue/taxes/sales");
    escape(document);
    await settle();
    const first = { split: app.expansions.join(","), isolated: app.filter.isolated };
    escape(document);
    await settle();
    const second = app.expansions.join(",");
    escape(document);
    await settle();
    const third = app.expansions.join(",");
    escape(document);
    await settle();
    assert.deepEqual(first, { split: OPENS + ",fund/100", isolated: "" });
    assert.equal(second, OPENS);
    assert.equal(third, "");
    assert.deepEqual(refusals(document).map((b) => b.textContent), []);
    t.diagnostic("Escape cleared the isolation, then undid fund/100, then the General Fund group, then did nothing");
  });

  test("Expand all splits every node to its finest grain by its first split, and Collapse all folds everything back", async (t) => {
    const { app, document } = await bootedApp();
    const [all, none] = bulk(document);
    assert.ok(all && none, "the chip row carries no Expand all and Collapse all");
    assert.equal(document.getElementById("breadcrumb").hasAttribute("hidden"), false);
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

for (const year of YEARS) {
  describe(`${year.label}: the folded tail`, () => {
    test(`${year.label}: Enter on a split's tail draws its members out, Space still follows its money, and its chip folds them back`, async (t) => {
      const { app, document } = await onYear(year.stem);
      await split(app, WORST);
      const tail = app.projection.nodes.find((n) => app.isAggregate(n.id));
      assert.ok(tail, `${WORST} drew no folded tail`);
      const offered = app.projection.nodes.filter((n) => app.expandable(n)).map((n) => n.id);
      assert.deepEqual(offered, [tail.id], "the tail is the one mark the chart offers to draw out");
      const capped = app.projection.nodes.length;

      // SPACE GOES FIRST AND MUST NOT DRAW OUT: it is the key that follows the
      // money on every other mark.
      fire(markOf(app, document, tail.id), "keydown", { key: " ", timeStamp: 0 });
      await settle();
      const spaceFollowed = app.filter.isolated;
      app.setIsolated("");
      const m = markOf(app, document, tail.id);
      m.focus();
      const before = new Set(app.laidNodes.map((n) => n.id));
      fire(m, "keydown", { key: "Enter", timeStamp: app.ACTIVATION_WINDOW * 10 });
      await settle();
      const drawnOut = app.projection.nodes.length;
      const members = new Set(app.splitsHere({ id: WORST })[0].members);
      const fresh = app.laidNodes.filter((n) => members.has(n.id) && !before.has(n.id)).sort((a, b) => a.y0 - b.y0);
      const on = focusedID(document);
      assert.equal(spaceFollowed, tail.id, "Space follows the tail's money");
      assert.deepEqual(app.expansions, [WORST, tail.id]);
      assert.ok(fresh.length > 0, "drawing the tail out drew no new member");
      assert.equal(on, fresh[0].id, "focus is not on the topmost member the tail drew out");
      assert.ok(app.pinned === null, "restoring focus pinned the mark");
      const tailChip = chips(document).find((c) => c.textContent.includes(app.nodeOf(WORST).label) && c !== chips(document)[0]);
      assert.ok(tailChip, "no chip for the tail drawn out");
      tailChip.click();
      await settle();
      assert.deepEqual(app.expansions, [WORST]);
      assert.equal(app.projection.nodes.length, capped);
      t.diagnostic(`${WORST} capped draws ${capped} node(s); Enter on "${tail.label}" drew ${drawnOut}, focus on ${on}`);
    });
  });
}

describe("focus after a split", () => {
  test("lands on the topmost new member, unpinned, and the next Enter acts on it", async (t) => {
    const { app, document } = await bootedApp();
    const m = markOf(app, document, OPENS);
    m.focus();
    fire(m, "keydown", { key: "Enter", timeStamp: 1000 });
    await settle();
    const members = new Set(app.splitsHere({ id: OPENS })[0].members);
    const top = app.laidNodes.filter((n) => members.has(n.id)).sort((a, b) => a.y0 - b.y0)[0];
    const on = focusedID(document);
    t.diagnostic(`after Enter on ${OPENS} focus is on ${on}`);
    assert.equal(on, top.id);
    assert.ok(app.pinned === null, "restoring focus pinned the mark");
    // Escape undoes the split, and focus goes back to the node it split.
    escape(document);
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

