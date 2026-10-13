// columns.test.mjs -- the chart's columns are the expansion's slots: how wide
// the page lays the chart out, that an extend split widens it by the slot it
// inserts, and that a split whose chart will not draw is refused with the
// chart left as it was.

import { describe, test } from "node:test";
import assert from "node:assert/strict";

import { bootedApp, split, settle, pageFixture, columnFixture, refusals } from "./testlib.mjs";

const YEARS = pageFixture().config.years;

describe("the columns are the slots the chart draws", () => {
  test("the page hands the stylesheet the held width and the chart's stretch over it, and an extend split widens it by one slot", async (t) => {
    const { app, document } = await bootedApp({ checkedStem: YEARS[0].stem });
    const handed = () => document.documentElement.style.getPropertyValue("--chart-max") + " x" +
      document.documentElement.style.getPropertyValue("--chart-stretch");
    const held = (n) => app.widthOf(app.HELD_COLUMNS) + "px x" + app.widthOf(n) / app.widthOf(app.HELD_COLUMNS);
    const columns = () => app.layoutColumns(app.shown.doc);
    const root = { columns: columns(), handed: handed() };
    // Every split extends: the group's funds take a slot of their own after
    // it, and fund/100's divisions one after theirs.
    await split(app, "fund-group/general");
    const funds = columns();
    await split(app, "fund/100");
    const extended = { columns: columns(), handed: handed() };
    t.diagnostic(`the root lays ${root.columns} columns at ${root.handed}; with the funds, ${funds}; ` +
      `with fund/100's divisions, ${extended.columns} at ${extended.handed}`);
    assert.equal(root.columns, 3);
    assert.equal(root.handed, held(3));
    assert.equal(funds, 4);
    assert.equal(extended.columns, 5);
    assert.equal(extended.handed, held(5));
    app.collapseAll();
    await settle();
    assert.equal(handed(), root.handed, "folding everything back does not narrow the chart again");
  });
});

describe("a split whose chart will not draw", () => {
  test("is refused with the banner, and the chart, the splits and the chip row stay as they were", async (t) => {
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
