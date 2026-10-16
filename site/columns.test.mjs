// columns.test.mjs -- the chart's columns are the expansion's slots: how wide
// the page lays the chart out, that an extend split widens it by the slots it
// inserts, and that a split whose chart will not draw is refused with the
// chart left as it was.

import { describe, test } from "node:test";
import assert from "node:assert/strict";

import { bootedApp, split, settle, pageFixture, columnFixture, refusals } from "./testlib.mjs";

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

  // Mutation: leave the pin set in fail() and this goes red: Pin is shown
  // pressed over the refusal, and the hover previews nothing.
  test("refused, lets the pin go: a swatch isolated after shows Pin unpressed over the refusal, and a hover previews", async (t) => {
    const broken = structuredClone(columnFixture("fy2026-adopted"));
    const sales = broken.nodes.findIndex((n) => n.id === "revenue/taxes/sales");
    broken.schedules.expansion.splits.push({ node: sales, key: "lines-planted", form: "refine", alternative: true });
    const config = structuredClone(pageFixture().config);
    config.split_labels["lines-planted"] = "planted lines";
    const { app, document } = await bootedApp({ config, checkedStem: YEARS[0].stem, plan: { [YEARS[0].path]: { doc: broken } } });
    app.pin(app.laidNodes.find((n) => n.id === "fund-group/general"));
    const drew = app.expandNode("revenue/taxes/sales", "lines-planted");
    await settle();
    document.querySelector("#legend [data-group]").click();
    const button = document.getElementById("detail-pin");
    const pressed = !button.hidden && button.getAttribute("aria-pressed") === "true";
    const over = refusals(document).length === 1 && document.getElementById("detail").textContent === refusals(document)[0].textContent;
    const hovered = app.laidNodes.find((n) => n.id === "revenue/taxes/sales");
    app.preview(hovered);
    const previewed = app.previewed ? app.previewed.id : "";
    t.diagnostic(`refused ${!drew}; Pin pressed over the refusal ${pressed}; the hover previewed "${previewed}"`);
    assert.deepEqual({ drew, over, pressed, previewed }, { drew: false, over: true, pressed: false, previewed: "revenue/taxes/sales" });
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
