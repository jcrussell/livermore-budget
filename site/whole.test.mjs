// The whole-budget page, testdata/whole.golden.html: fund-flows drawn out of
// each year's column at every tier it has a column of, its group tier
// dropped, folded as it opens, and its sub-pixel ribbons drawn as hairlines
// the page says it draws. Every figure here is a client sum over cited cents.

import { describe, test } from "node:test";
import assert from "node:assert/strict";

import { bootedApp, wholeFixture, pageFixture, expandAll, clickYear, settle, fire } from "./testlib.mjs";

/** Every pinned year's whole-budget chart, booted once per test. */
async function wholePage(config) {
  const page = wholeFixture();
  return bootedApp({ html: page.html, config: config || page.config });
}

/**
 * Each year the page lists, switched to and settled, with the page handed to
 * `visit`.
 */
async function eachYear(loaded, visit) {
  for (const year of loaded.config.years) {
    clickYear(loaded.document, year.stem);
    await settle();
    assert.equal(loaded.app.shownYear.stem, year.stem, `the page did not switch to ${year.label}`);
    await visit(year);
  }
}

/** The ids of the nodes of the document's dropped tiers. */
function droppedIn(app, doc) {
  const drop = app.CONFIG.overview.sankey.drop;
  return new Set(doc.nodes.filter((n) => drop.includes(n.tier)).map((n) => n.id));
}

describe("the whole budget drawn out", () => {
  test("holds every link of the column the drop keeps, at its folded ends, each once, citing what it merges", async (t) => {
    const loaded = await wholePage();
    const { app } = loaded;
    await eachYear(loaded, async (year) => {
      expandAll(app);
      await settle();
      const raw = app.fetched;
      const drawn = app.unmarkContra(app.projection);
      const gone = droppedIn(app, raw);
      const tiers = new Set(app.layoutTiers(drawn));
      const byID = new Map(raw.nodes.map((n) => [n.id, n]));
      const end = (id) => app.foldTarget(byID, byID.get(id), tiers);
      // Every kept link at its folded ends, summed by (source, target, kind):
      // what the drawn chart must hold, ribbon for ribbon.
      const want = new Map();
      for (const l of raw.links) {
        if (gone.has(l.source) || gone.has(l.target)) continue;
        const [s, g] = [end(l.source), end(l.target)];
        if (s === g) continue;
        const key = `${s} > ${g} (${l.kind})`;
        want.set(key, (want.get(key) || 0) + l.value_cents);
      }
      const got = new Map();
      for (const l of drawn.links) {
        const key = `${l.source} > ${l.target} (${l.kind})`;
        assert.ok(!got.has(key), `${year.label}: ${key} is drawn twice`);
        got.set(key, l.value_cents);
        assert.ok(l.fact_ids.length > 0 && l.locators.length > 0, `${year.label}: ${key} cites nothing`);
      }
      assert.deepEqual(got, want, `${year.label}: the drawn ribbons are not the kept links at their folded ends`);
      // No fact the document cites is lost by the drop.
      const cited = new Set(drawn.links.flatMap((l) => l.fact_ids));
      const lost = raw.links.flatMap((l) => l.fact_ids).filter((id) => !cited.has(id));
      assert.deepEqual([...new Set(lost)], [], `${year.label}: facts no drawn ribbon cites`);
      const total = [...got.values()].reduce((a, c) => a + c, 0);
      t.diagnostic(`${year.label}: ${got.size} ribbons, ${drawn.nodes.length} nodes, ${cited.size} facts cited, ` +
        `signed sum ${total} cents, ${gone.size} dropped nodes`);
    });
  });

  test("draws each fund at the inflow its group's own link states for it, which is the link the drop leaves out", async (t) => {
    const loaded = await wholePage();
    const { app } = loaded;
    await eachYear(loaded, async (year) => {
      expandAll(app);
      await settle();
      const raw = app.fetched;
      const gone = droppedIn(app, raw);
      const stated = new Map();
      for (const l of raw.links) {
        if (gone.has(l.source)) stated.set(l.target, (stated.get(l.target) || 0) + l.value_cents);
      }
      const drawn = app.unmarkContra(app.projection);
      const into = new Map();
      for (const l of drawn.links) into.set(l.target, (into.get(l.target) || 0) + l.value_cents);
      const off = [...stated].filter(([id, c]) => into.get(id) !== c).map(([id, c]) => `${id}: stated ${c}, drawn ${into.get(id)}`);
      t.diagnostic(`${year.label}: ${stated.size} funds, each drawn at its group's statement of it`);
      assert.ok(stated.size > 0, `${year.label}: the drop left out no link, so this proves nothing`);
      assert.deepEqual(off, []);
    });
  });
});

describe("the whole budget folded as it opens", () => {
  test("folds each capped column to its cap and one tail that draws out, citing every fact the drawn-out chart cites", async (t) => {
    const loaded = await wholePage();
    const { app } = loaded;
    const caps = app.CONFIG.overview.sankey.caps;
    assert.ok(caps.length > 0, "the page declares no cap, so nothing here folds");
    await eachYear(loaded, async (year) => {
      const folded = app.projection;
      const tails = folded.nodes.filter((n) => app.expandable(n));
      for (const c of caps) {
        const column = folded.nodes.filter((n) => n.tier === c.tier);
        assert.ok(column.length <= c.cap + 1, `${year.label}: tier ${c.tier} draws ${column.length} marks over a cap of ${c.cap}`);
      }
      const foldedFacts = new Set(folded.links.flatMap((l) => l.fact_ids));
      const foldedSum = folded.links.reduce((a, l) => a + Math.abs(l.value_cents), 0);
      expandAll(app);
      await settle();
      const out = app.projection;
      const outFacts = new Set(out.links.flatMap((l) => l.fact_ids));
      t.diagnostic(`${year.label}: ${tails.length} tails over ${folded.nodes.length} marks, drawn out to ${out.nodes.length}; ` +
        `${foldedFacts.size} facts cited folded, ${outFacts.size} drawn out`);
      assert.ok(tails.length > 0, `${year.label}: nothing folded, so nothing draws out`);
      assert.deepEqual([...outFacts].filter((id) => !foldedFacts.has(id)), [], `${year.label}: facts the fold stops citing`);
      assert.ok(foldedSum > 0);
      assert.equal(out.nodes.filter((n) => app.isAggregate(n.id)).length, 0, `${year.label}: a tail is left after every one drew out`);
      for (const c of caps) app.collapseTier(c.tier);
      await settle();
      assert.equal(app.projection.nodes.length, folded.nodes.length, `${year.label}: folding back did not return the opening chart`);
    });
  });

  test("a tail drawn out shows its fold-back chip on the overview, and the chip folds it back", async () => {
    const { app, document } = await wholePage();
    const bar = document.getElementById("breadcrumb");
    assert.ok(bar.hidden, "the overview's bar shows before anything is drawn out");
    const tail = app.projection.nodes.find((n) => app.expandable(n));
    assert.ok(tail, "the opening chart offers no tail");
    app.expandTier(tail);
    await settle();
    const chip = bar.querySelector(".crumb-expanded");
    assert.ok(!bar.hidden && chip, "a drawn-out column on the overview has no chip to fold it back");
    assert.match(chip.textContent, /^showing all \d+ \S/);
    chip.dispatchEvent(new globalThis.MouseEvent("click", { bubbles: true }));
    await settle();
    assert.ok(bar.hidden, "the chip did not fold the column back");
    assert.ok(app.projection.nodes.some((n) => n.id === tail.id), "the tail did not come back");
  });

  test("a year switch opens the next year folded", async () => {
    const loaded = await wholePage();
    const { app, document, config } = loaded;
    expandAll(app);
    await settle();
    assert.ok(!app.projection.nodes.some((n) => app.isAggregate(n.id)));
    clickYear(document, config.years[0].stem);
    await settle();
    assert.ok(app.projection.nodes.some((n) => app.expandable(n)), "the next year opened drawn out");
  });
});

/**
 * A laid link as a string. KEYS, NOT THE LAID LINKS: a laid link reaches the
 * whole graph through its nodes, and a failing assert over one formats that
 * graph until the host runs out of memory.
 * @param {any} d
 */
const key = (d) => (d ? `${d.source.id} > ${d.target.id} #${d.index}` : String(d));

describe("hairlines", () => {
  test("every ribbon under a pixel of the canvas is drawn as a hairline under a wider hit target, and the page says how many", async (t) => {
    const { app, document } = await wholePage();
    const chart = document.getElementById("chart");
    const line = document.getElementById("hairline-line");
    assert.equal(chart.getAttribute("viewBox").split(" ")[3], String(app.CONFIG.overview.sankey.height),
      "the chart is not laid out on the canvas its hint declares");
    const check = (state) => {
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
    const folded = check("folded");
    assert.ok(folded.thin.length > 0, "the opening chart draws no hairline, so this proves less than it says");
    expandAll(app);
    await settle();
    const out = check("drawn out");
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

  test("the spine, on the form's own canvas, draws none and says nothing, though some of its ribbons are under a pixel", async (t) => {
    const { document } = await bootedApp();
    const thin = [...document.querySelectorAll("#chart path.link")].filter((p) => p.__data__.width < 1).length;
    t.diagnostic(`the spine draws ${thin} ribbons under a pixel at its stroke floor`);
    assert.equal(document.querySelectorAll("#chart path.link.hairline").length, 0);
    assert.equal(document.querySelectorAll("#chart path.link-hit").length, 0);
    assert.ok(document.getElementById("hairline-line").hidden);
    assert.ok(pageFixture().config.overview.sankey.height === undefined);
  });
});

describe("the whole budget's hints", () => {
  test("only sankey.js reads the overview's canvas, drop and caps", async (t) => {
    const config = structuredClone(wholeFixture().config);
    /** @type {{prop: string, frame: string}[]} */
    const reads = [];
    config.overview.sankey = new Proxy(config.overview.sankey, {
      get(target, prop) {
        const frame = ((new Error().stack || "").split("\n")[2] || "").trim();
        reads.push({ prop: String(prop), frame });
        return target[prop];
      },
    });
    const { app } = await wholePage(config);
    expandAll(app);
    await settle();
    const props = new Set(reads.map((r) => r.prop));
    t.diagnostic(`${reads.length} reads of ${[...props].join(", ")}`);
    for (const p of ["height", "drop", "caps", "tiers"]) assert.ok(props.has(p), `nothing read ${p}`);
    const outside = reads.filter((r) => !r.frame.includes("site/sankey.js"));
    assert.deepEqual([...new Set(outside.map((r) => `${r.prop} read at ${r.frame}`))], []);
  });
});
