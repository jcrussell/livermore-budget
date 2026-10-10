// columns.test.mjs -- the chart's columns are the expansion's slots: how wide
// the page lays the chart out, that an extend split widens it by the slot it
// inserts, and that a split whose chart will not draw is refused with the
// chart left as it was.

import { describe, test } from "node:test";
import assert from "node:assert/strict";

import { bootedApp, split, settle, pageFixture, columnFixture, refusals } from "./testlib.mjs";

const YEARS = pageFixture().config.years;

describe("the columns are the slots the chart draws", () => {
  test("the page hands the stylesheet the width the chart is laid out at, and an extend split widens it by one slot", async (t) => {
    const { app, document } = await bootedApp({ checkedStem: YEARS[0].stem });
    const handed = () => document.documentElement.style.getPropertyValue("--chart-max");
    const columns = () => app.layoutColumns(app.shown.doc);
    const root = { columns: columns(), handed: handed() };
    // A refine split draws its members where the node stood; fund/100's
    // divisions extend into a slot of their own.
    await split(app, "fund-group/general");
    const refined = columns();
    await split(app, "fund/100");
    const extended = { columns: columns(), handed: handed() };
    t.diagnostic(`the root lays ${root.columns} columns at ${root.handed}; refined, ${refined}; ` +
      `with fund/100's divisions, ${extended.columns} at ${extended.handed}`);
    assert.equal(root.columns, 3);
    assert.equal(root.handed, app.widthOf(3) + "px");
    assert.equal(refined, 3);
    assert.equal(extended.columns, 4);
    assert.equal(extended.handed, app.widthOf(4) + "px");
    app.collapseAll();
    await settle();
    assert.equal(handed(), root.handed, "folding everything back does not narrow the chart again");
  });
});

describe("a split whose chart will not draw", () => {
  test("is refused with the banner, and the chart, the splits and the chip row stay as they were", async (t) => {
    // A split of the sales tax with no member: the root never asks it, and
    // drawing it leaves the category's ribbons ending at a node not drawn.
    const broken = structuredClone(columnFixture("fy2026-adopted"));
    const sales = broken.nodes.findIndex((n) => n.id === "revenue/taxes/sales");
    broken.schedules.expansion.splits.push({ node: sales, key: "lines-planted", form: "refine", alternative: true });
    const { app, document } = await bootedApp({ checkedStem: YEARS[0].stem, plan: { [YEARS[0].path]: { doc: broken } } });
    assert.deepEqual(refusals(document).map((b) => b.textContent), [], "the root read a link it should not");
    await split(app, "fund-group/general");
    const laid = app.laidNodes.map((n) => n.id).sort().join(",");
    const chips = document.getElementById("breadcrumb").textContent;
    const drew = app.expandNode("revenue/taxes/sales", "lines-planted");
    await settle();
    const banners = refusals(document).map((b) => b.textContent);
    t.diagnostic(`the planted split drew ${drew}; banners ${JSON.stringify(banners)}`);
    assert.equal(drew, false);
    assert.equal(banners.length, 1);
    assert.ok(banners[0].startsWith("That could not be split: "), banners[0]);
    assert.deepEqual(app.expansions, ["fund-group/general"]);
    assert.equal(app.laidNodes.map((n) => n.id).sort().join(","), laid);
    assert.equal(document.getElementById("breadcrumb").textContent, chips);
  });
});
