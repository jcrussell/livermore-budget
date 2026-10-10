// Hairlines: the index page's sub-pixel ribbons, on the canvas sankey.grow
// sizes, drawn a screen pixel wide under a wider hit target, and the page
// says how many.

import { describe, test } from "node:test";
import assert from "node:assert/strict";

import { bootedApp, pageFixture, expandAll, settle, fire } from "./testlib.mjs";

const key = (d) => (d ? `${d.source.id} > ${d.target.id} #${d.index}` : String(d));

describe("hairlines", () => {
  test("every ribbon under a pixel of the canvas is drawn as a hairline under a wider hit target, and the page says how many", async (t) => {
    const { app, document } = await bootedApp();
    const chart = document.getElementById("chart");
    const line = document.getElementById("hairline-line");
    const check = (state) => {
      assert.equal(chart.getAttribute("viewBox").split(" ")[3], String(app.canvasHeight(app.CONFIG.overview, app.shown.doc.nodes)),
        `${state}: the chart is not laid out on the canvas its hint grows to`);
      const laid = [...chart.querySelectorAll("path.link")].map((p) => p.__data__);
      const thin = laid.filter((d) => d.width < app.HAIRLINE);
      assert.ok(thin.every((d) => app.isHairline(app.CONFIG.overview, d)));
      const hairlines = [...chart.querySelectorAll("path.link.hairline")];
      const hits = [...chart.querySelectorAll("g.link-hits path.link-hit")];
      t.diagnostic(`${state}: ${laid.length} ribbons, ${thin.length} under a pixel, ${hits.length} hit targets`);
      assert.deepEqual(hairlines.map((p) => key(p.__data__)), thin.map(key), `${state}: the hairline class is not the sub-pixel ribbons`);
      assert.deepEqual(hits.map((p) => key(p.__data__)), thin.map(key), `${state}: the hit targets are not the hairlines'`);
      assert.ok(hits.every((p) => p.getAttribute("aria-hidden") === "true" && !p.hasAttribute("tabindex")),
        `${state}: a hit target is in the accessibility tree or the tab order`);
      assert.ok(hairlines.every((p) => p.getAttribute("tabindex") === "0"), `${state}: a hairline is not focusable`);
      assert.equal(line.hidden, thin.length === 0, `${state}: the sentence's visibility`);
      assert.equal(line.textContent, thin.length ? app.say("hairline", { n: thin.length }) : "");
      return { thin, hairlines, hits };
    };
    const folded = check("the root");
    assert.ok(folded.thin.length > 0, "the opening chart draws no hairline, so this proves less than it says");
    app.expandAll();
    await settle();
    check("everything split");
    expandAll(app);
    await settle();
    const out = check("everything split, every tail drawn out");
    // The pointer on a hit target is the pointer on its ribbon.
    const hit = out.hits[0];
    fire(hit, "pointerenter", { clientX: 10, clientY: 10 });
    const tip = document.getElementById("tooltip");
    assert.ok(!tip.hidden && tip.textContent.length > 0, "hovering a hairline's hit target shows no tooltip");
    fire(hit, "click");
    assert.equal(key(app.pinned), key(hit.__data__), "a click on a hairline's hit target pins another flow");
    // Focus is on the ribbon itself.
    const ribbon = out.hairlines[out.hairlines.length - 1];
    ribbon.focus();
    assert.equal(key(app.pinned), key(ribbon.__data__), "focusing a hairline does not pin it");
  });

  // The spine's canvas grows (sankey.grow) from the 820px floor its busiest
  // column does not reach, so it is laid out as on the form's own canvas and
  // its thin ribbons are hairlines the page says it draws.
  test("the spine, on the canvas it declares grows, draws its ribbons under a pixel as hairlines and says how many", async (t) => {
    const { app, document } = await bootedApp();
    const links = [...document.querySelectorAll("#chart path.link")];
    const thin = links.filter((p) => p.__data__.width < 1).map((p) => key(p.__data__));
    const height = document.getElementById("chart").getAttribute("viewBox").split(" ")[3];
    t.diagnostic(`the spine draws ${thin.length} of ${links.length} ribbons under a pixel on a ${height}px canvas`);
    assert.ok(thin.length > 0, "the spine draws no ribbon under a pixel, so this proves less than it says");
    assert.deepEqual([...document.querySelectorAll("#chart path.link.hairline")].map((p) => key(p.__data__)), thin);
    assert.deepEqual([...document.querySelectorAll("#chart path.link-hit")].map((p) => key(p.__data__)), thin);
    assert.equal(document.getElementById("hairline-line").textContent, app.say("hairline", { n: thin.length }));
    assert.equal(height, String(pageFixture().config.overview.sankey.grow.floor));
    assert.ok(pageFixture().config.overview.sankey.height === undefined);
  });
});

