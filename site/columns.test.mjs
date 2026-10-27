// columns.test.mjs -- the chart's columns are the expansion's slots: how wide
// the page lays the chart out, that an extend split widens it by the slots it
// inserts, and that a split whose chart will not draw is refused with the
// chart left as it was.

import { describe, test } from "node:test";
import assert from "node:assert/strict";

import { bootedApp, split, settle, pageFixture, columnFixture, refusals } from "./testlib.mjs";
import * as sankey from "./sankey.js";

const YEARS = pageFixture().config.years;

describe("the columns are the slots the chart draws", () => {
  test("the page hands the stylesheet the opening chart's width and the chart's stretch over it, and the General Fund group, opened straight through its one fund, widens it by two slots", async (t) => {
    const { app, document } = await bootedApp({ checkedStem: YEARS[0].stem });
    const handed = () => document.documentElement.style.getPropertyValue("--chart-max") + " x" +
      document.documentElement.style.getPropertyValue("--chart-stretch");
    const held = (n) => app.widthOf(app.openingColumns()) + "px x" + app.widthOf(n) / app.widthOf(app.openingColumns());
    const columns = () => app.layoutColumns(app.shown.doc);
    const root = { columns: columns(), handed: handed() };
    // Every split extends: the group's one fund takes a slot of its own
    // after it, and, the group opening straight through it, fund/100's
    // divisions one after that.
    await split(app, "fund-group/general");
    const extended = { columns: columns(), handed: handed() };
    t.diagnostic(`the root lays ${root.columns} columns at ${root.handed}; ` +
      `with the group open, ${extended.columns} at ${extended.handed}`);
    assert.equal(root.columns, 3);
    assert.equal(root.handed, held(3));
    assert.equal(extended.columns, 5);
    assert.equal(extended.handed, held(5));
    app.collapseAll();
    await settle();
    assert.equal(handed(), root.handed, "folding everything back does not narrow the chart again");
  });
});

// A miniature expansion: a source whose lines pay into it, one line split
// again the same way, and a group whose funds it pays into.
function sources() {
  const node = (id, tier, parent = "") => ({ id, label: id, tier, parent });
  const link = (source, target, repeats) => ({ source, target, value_cents: 1, kind: "external", fact_ids: ["f"], repeats });
  return {
    projection: "expansion",
    nodes: [
      node("revenue/x", 0), node("revenue-line/x/a", 1, "revenue/x"), node("revenue-line/x/a/b", 1, "revenue-line/x/a"),
      node("fund-group/g", 2), node("fund/1", 3, "fund-group/g"),
    ],
    links: [
      link("revenue-line/x/a/b", "revenue-line/x/a", "revenue-line/x/a/b -> fund/1 external"),
      link("revenue-line/x/a", "revenue/x", "revenue-line/x/a/b -> fund/1 external"),
      link("revenue/x", "fund-group/g", "revenue-line/x/a/b -> fund/1 external"),
      link("fund-group/g", "fund/1", "revenue-line/x/a/b -> fund/1 external"),
    ],
    splits: [
      { node: "revenue/x", key: "lines", form: "extend", alternative: false, side: "before" },
      { node: "revenue-line/x/a", key: "parts", form: "extend", alternative: false, side: "before" },
      { node: "fund-group/g", key: "funds", form: "extend", alternative: false, side: "after" },
    ],
  };
}

describe("a split stands its members on the side it declares", () => {
  // Mutation: expansionSlots placing every member at its parent's slot + 1
  // and the source's line stands after it, in its group's way; infer a side
  // from the links between a node and its members, a split none runs
  // between standing after it, and the linkless copy's line stands after
  // its source.
  test("a source's members, and theirs, stand before it; a group's after it; whatever links run between them", async () => {
    const app = sankey;
    for (const doc of [sources(), Object.assign(sources(), { links: [] })]) {
      const slots = app.expansionSlots(doc, [0, 2]);
      const at = Object.fromEntries(slots);
      assert.ok(at["revenue-line/x/a/b"] < at["revenue-line/x/a"] && at["revenue-line/x/a"] < at["revenue/x"] &&
        at["revenue/x"] < at["fund-group/g"] && at["fund-group/g"] < at["fund/1"], JSON.stringify(at));
      assert.ok([...slots.values()].every((k) => k >= 0), JSON.stringify(at));
    }
  });

  // Mutation: default an undeclared side to after for a split no link runs
  // between, and the line is placed after its source with nothing saying so;
  // drop the back refusal and the line's part stands in its source's slot.
  test("a split declaring no side, or a member opening back toward its grandparent, is refused", async () => {
    const app = sankey;
    for (const side of [undefined, "left"]) {
      const none = sources();
      none.splits[0] = Object.assign({}, none.splits[0], { side });
      assert.throws(() => app.expansionSlots(none, [0, 2]), /which declares no side of revenue\/x its members stand on/);
    }
    const back = sources();
    back.splits[1] = Object.assign({}, back.splits[1], { side: "after" });
    assert.throws(() => app.expansionSlots(back, [0, 2]), /opening back toward revenue\/x/);
  });

  // A flow between two members of an inflow hub, re-pointed onto it as
  // expansion-splits-tie's own plant does (TestAFlowBetweenAHubsMembersIsALeaf),
  // runs from a member back into its node beside the node's leg to that
  // member: the slots place it on the declared side, and d3 refuses the
  // circle. That is why the stitch and the check refuse it; this holds the
  // page to a banner and the chart it had rather than a half-drawn one.
  test("a leaf between two members of one hub, drawn back into it, is refused with the chart left as it was", async (t) => {
    const year = YEARS.find((y) => y.year === 2027);
    const planted = structuredClone(columnFixture(year.path.replace(/\.json$/, "")));
    const at = (id) => planted.nodes.findIndex((n) => n.id === id);
    const group = at("fund-group/enterprise"), from = at("fund/610"), to = at("fund/620");
    const leaf = "revenue-line/charges-for-services/charges-for-services -> fund/620 external";
    const flow = "fund/610 -> fund/620 external";
    const links = planted.schedules.expansion.links;
    const leg = links.find((l) => l.from === group && l.to === to && l.repeats === leaf);
    const moved = links.find((l) => l.from === at("revenue/charges-for-services") && l.to === group && l.repeats === leaf);
    assert.ok(leg !== undefined && moved !== undefined, "the fixture draws no leaf onto the group to plant from");
    Object.assign(leg, { repeats: flow });
    Object.assign(moved, { from, repeats: flow });
    const { app, document } = await bootedApp({ checkedStem: year.stem, plan: { [year.path]: { doc: planted } } });
    assert.deepEqual(refusals(document).map((b) => b.textContent), [], "the root drew the plant");
    const laid = app.laidNodes.map((n) => n.id).sort().join(",");
    const drew = app.expandNode("fund-group/enterprise");
    await settle();
    const banners = refusals(document).map((b) => b.textContent);
    t.diagnostic(`opening the group drew ${drew}; banners ${JSON.stringify(banners)}`);
    assert.equal(drew, false);
    assert.deepEqual(banners, ["That could not be split: circular link"]);
    assert.deepEqual(app.expansions, []);
    assert.equal(app.laidNodes.map((n) => n.id).sort().join(","), laid);
  });
});

describe("an extend chain has the slots its root has on its side", () => {
  /**
   * A root and a chain of `depth` extend splits below it, each member's link
   * running from its parent (`after`) or into it.
   */
  const chain = (depth, after) => {
    const root = after ? "fund-group/g" : "revenue/x";
    const ids = [root];
    for (let i = 1; i <= depth; i++) ids.push(root + "/" + i);
    return {
      projection: "expansion",
      nodes: ids.map((id, i) => ({ id, label: id, tier: after ? 2 : 0, parent: i ? ids[i - 1] : "" })),
      links: ids.slice(1).map((id, i) => {
        const [source, target] = after ? [ids[i], id] : [id, ids[i]];
        return { source, target, value_cents: 1, kind: "external", fact_ids: ["f"] };
      }),
      splits: ids.slice(0, -1).map((node) => ({ node, key: "k", form: "extend", alternative: false, side: after ? "after" : "before" })),
    };
  };

  // Mutation: say SLOT_DEPTH as the limit, as if a chain had every slot of
  // its column on each side, and this goes red.
  test("four slots after its root and five before, and a chain past them is refused saying so", async (t) => {
    const app = sankey;
    assert.equal(app.expansionSlots(chain(4, true), [0, 2]).size, 5);
    assert.equal(app.expansionSlots(chain(5, false), [0, 2]).size, 6);
    let after = "";
    let before = "";
    try { app.expansionSlots(chain(5, true), [0, 2]); } catch (e) { after = String(e.message); }
    try { app.expansionSlots(chain(6, false), [0, 2]); } catch (e) { before = String(e.message); }
    t.diagnostic(after);
    t.diagnostic(before);
    assert.ok(after.endsWith("extended past the 4 slots after its root"), after);
    assert.ok(before.endsWith("extended past the 5 slots before its root"), before);
  });
});

describe("a split whose chart will not draw", () => {
  test("is refused with the banner, and the chart, the splits and the toolbar stay as they were", async (t) => {
    // A split of the sales tax in a form this page does not draw: the root
    // never asks it, and opening it is refused rather than drawn as extend.
    const broken = structuredClone(columnFixture("fy2026-adopted"));
    const sales = broken.nodes.findIndex((n) => n.id === "revenue/taxes/sales");
    broken.schedules.expansion.splits.push({ node: sales, key: "lines-planted", form: "refine", alternative: true });
    // Worded, so opening it reaches the form and not the missing words.
    const config = structuredClone(pageFixture().config);
    config.split_labels["lines-planted"] = "planted lines";
    const { app, document } = await bootedApp({ config, checkedStem: YEARS[0].stem, plan: { [YEARS[0].path]: { doc: broken } } });
    assert.deepEqual(refusals(document).map((b) => b.textContent), [], "the root read a link it should not");
    await split(app, "fund-group/general");
    const laid = app.laidNodes.map((n) => n.id).sort().join(",");
    const bulk = ["chart-expand-all", "chart-collapse-all"].map((id) => document.getElementById(id).disabled).join(",");
    const drew = app.expandNode("revenue/taxes/sales", "lines-planted");
    await settle();
    const banners = refusals(document).map((b) => b.textContent);
    t.diagnostic(`the planted split drew ${drew}; banners ${JSON.stringify(banners)}`);
    assert.equal(drew, false);
    assert.equal(banners.length, 1);
    assert.ok(banners[0].startsWith("That could not be split: "), banners[0]);
    assert.deepEqual(app.expansions, ["fund-group/general"]);
    assert.equal(app.laidNodes.map((n) => n.id).sort().join(","), laid);
    assert.equal(["chart-expand-all", "chart-collapse-all"].map((id) => document.getElementById(id).disabled).join(","), bulk);
  });

  test("opened by a click, leaves the refusal in the panel rather than pinning the node over it", async (t) => {
    // A node the root draws with no split of its own, given only a split in
    // a form this page does not draw: a click asks for it and is refused.
    const broken = structuredClone(columnFixture("fy2026-adopted"));
    const id = "fund-balance/draw";
    broken.schedules.expansion.splits.push({ node: broken.nodes.findIndex((n) => n.id === id), key: "lines-planted", form: "refine", alternative: true });
    const config = structuredClone(pageFixture().config);
    config.split_labels["lines-planted"] = "planted lines";
    const { app, document } = await bootedApp({ config, checkedStem: YEARS[0].stem, plan: { [YEARS[0].path]: { doc: broken } } });
    const d = app.laidNodes.find((n) => n.id === id);
    assert.ok(d !== undefined && app.splittable(d), `${id} is not drawn splittable, so nothing here is tested`);
    app.clickNode(d);
    await settle();
    const banners = refusals(document).map((b) => b.textContent);
    const panel = document.getElementById("detail").textContent;
    t.diagnostic(`banners ${JSON.stringify(banners)}; the panel says: ${panel.slice(0, 120)}`);
    assert.equal(banners.length, 1);
    assert.ok(banners[0].startsWith("That could not be split: "), banners[0]);
    assert.equal(panel, banners[0]);
    assert.ok(app.pinned === null, app.pinned && app.pinned.id);
  });

  // Mutation: refuse redraw's throw through fail() and this goes red: the
  // pin and the isolation go, the drawer shows the refusal, and a split
  // refused leaves the chart as it was with the reader's place lost.
  test("refused, keeps the pin and the isolation: the drawer still shows the node followed, Follow pressed, under the banner", async (t) => {
    const broken = structuredClone(columnFixture("fy2026-adopted"));
    const id = "revenue/taxes/sales";
    broken.schedules.expansion.splits.push({ node: broken.nodes.findIndex((n) => n.id === id), key: "lines-planted", form: "refine", alternative: true });
    const config = structuredClone(pageFixture().config);
    config.split_labels["lines-planted"] = "planted lines";
    const { app, document } = await bootedApp({ config, checkedStem: YEARS[0].stem, plan: { [YEARS[0].path]: { doc: broken } } });
    app.followNode(id);
    const drew = app.expandNode(id, "lines-planted");
    await settle();
    const follow = document.getElementById("detail-follow");
    const banners = refusals(document).map((b) => b.textContent);
    const got = { drew, banners: banners.length, pinned: app.pinned ? app.pinned.id : "", isolated: app.filter.isolated,
      follow: !follow.hidden && follow.getAttribute("aria-pressed") === "true",
      drawer: document.getElementById("detail").textContent === banners[0] };
    t.diagnostic(JSON.stringify(got));
    assert.deepEqual(got, { drew: false, banners: 1, pinned: id, isolated: id, follow: true, drawer: false });
  });

  // Mutation: let fail() keep a followed node's isolation and this goes
  // red: the chart follows a node the drawer, holding the refusal, no
  // longer shows.
  test("a refusal that is no rollback lets the pin go, and a followed node's isolation with it", async () => {
    const { app, document } = await bootedApp({ checkedStem: YEARS[0].stem });
    const id = "revenue/taxes/sales";
    app.followNode(id);
    app.fail("planted");
    assert.deepEqual({ pinned: app.pinned ? app.pinned.id : "", isolated: app.filter.isolated,
      drawer: document.getElementById("detail").textContent }, { pinned: "", isolated: "", drawer: "planted" });
  });
});

describe("a schedule declaring a split in a form this page does not draw", () => {
  test("is refused with the banner when the year is drawn, not drawn as extend", async (t) => {
    const broken = structuredClone(columnFixture("fy2026-adopted"));
    const general = broken.nodes.findIndex((n) => n.id === "fund-group/general");
    const sp = broken.schedules.expansion.splits.find((x) => x.node === general);
    assert.ok(sp, "the General Fund group declares no split");
    sp.form = "refine";
    const { app, document } = await bootedApp({ checkedStem: YEARS[0].stem, plan: { [YEARS[0].path]: { doc: broken } } });
    const banners = refusals(document).map((b) => b.textContent);
    t.diagnostic(`banners ${JSON.stringify(banners)}`);
    assert.equal(banners.length, 1);
    assert.ok(banners[0].includes('declared "refine", and this page draws a split only as extend'), banners[0]);
    assert.equal(app.laidNodes.length, 0, "the year drew a chart");
  });
});
