// A fund's mark has a maximum height: clampFunds cuts any fund taller than
// FUND_MAX_SHARE of the canvas to it, its ribbons narrow into the cut face
// and keep their width at the far end, and the page marks the cut and says so.

import { before, describe, test } from "node:test";
import assert from "node:assert/strict";

import { bootedApp, columnFixture, fire, loadD3, pageFixture, settle } from "./testlib.mjs";
import * as core from "./core.js";
import * as sankey from "./sankey.js";

/** The frontiers measured: the root, the General Fund opened, everything open. */
function frontiers(doc) {
  const root = core.rootFrontier(doc);
  const all = core.expandAllFrom(doc, root).frontier;
  return [
    { name: "root", frontier: root },
    { name: "fund-group/general by funds", frontier: core.expand(doc, root, "fund-group/general", "funds") },
    { name: "everything by primary splits", frontier: all },
  ];
}

const sum = (xs) => xs.reduce((a, b) => a + b, 0);
const key = (l) => sankey.linkKey(l);

describe("a fund's mark is drawn no taller than its maximum", () => {
  /** @type {{label: string, drawn: any, laid: any, height: number, most: number}[]} */
  const charts = [];
  before(() => {
    loadD3();
    const page = pageFixture();
    globalThis.FISC_CONFIG = page.config;
    const chart = page.config.overview;
    for (const year of page.config.years) {
      const column = columnFixture(year.path.replace(/\.json$/, ""));
      const doc = core.scheduleOf(column, "expansion");
      const index = new Map(doc.nodes.map((n) => [n.id, n]));
      const groups = column.fund_groups.map((g) => g.id);
      for (const f of frontiers(doc)) {
        const drawn = sankey.markContra(sankey.expansionOf(doc, f.frontier, chart, new Set()));
        const slots = sankey.slotsOf(drawn.nodes, chart.sankey.tiers);
        const height = sankey.canvasHeight(chart, drawn.nodes, 0);
        const laid = sankey.layOutAt(drawn, {
          tiers: chart.sankey.tiers, columns: new Set(slots.values()).size,
          groupOf: (n) => core.fundGroupOf(index, n), placeOf: (id) => groups.indexOf(id),
        }, height);
        charts.push({ label: `${year.label}, ${f.name}`, drawn, laid, height, most: Math.round(sankey.FUND_MAX_SHARE * (height - 24)) });
      }
    }
  });

  test("no fund's mark is taller than FUND_MAX_SHARE of the canvas, and the marks cut are the ones pinned", (t) => {
    const cut = [];
    for (const c of charts) {
      const over = c.laid.nodes.filter((n) => sankey.isFundMark(n) && n.y1 - n.y0 > c.most + 1e-9).map((n) => n.id);
      assert.deepEqual(over, [], `${c.label}: a fund's mark over ${c.most}px`);
      for (const n of c.laid.nodes.filter((m) => m.clamped)) {
        t.diagnostic(`${c.label}: ${n.id} cut to ${c.most}px, ${(100 * n.clamped).toFixed(1)}% of its height`);
        cut.push(`${c.label}: ${n.id}`);
      }
    }
    assert.deepEqual(cut, CUT);
  });

  test("at a cut face its ribbons' end widths sum to the mark's height, and each face's to its own share of it", (t) => {
    let faces = 0;
    for (const c of charts) {
      for (const n of c.laid.nodes.filter((m) => m.clamped)) {
        const out = n.sourceLinks.map((l) => sankey.endWidth(l, "source"));
        const into = n.targetLinks.map((l) => sankey.endWidth(l, "target"));
        const h = n.y1 - n.y0;
        assert.ok(Math.abs(Math.max(sum(out), sum(into)) - h) <= 1e-6,
          `${c.label}: ${n.id}'s fuller face sums to ${Math.max(sum(out), sum(into))}px against a ${h}px mark`);
        for (const [face, ls, end, at] of [["leaving", n.sourceLinks, "source", "y0"], ["arriving", n.targetLinks, "target", "y1"]]) {
          if (!ls.length) continue;
          faces++;
          assert.ok(Math.abs(sum(ls.map((l) => sankey.endWidth(l, end))) - n.clamped * sum(ls.map((l) => l.width))) <= 1e-6,
            `${c.label}: ${n.id}'s ${face} face is not its ribbons cut by the mark's ratio`);
          // Stacked from the mark's top, each under the last.
          let y = n.y0;
          for (const l of ls) {
            const w = sankey.endWidth(l, end);
            assert.ok(Math.abs(l[at] - (y + w / 2)) <= 1e-6, `${c.label}: ${n.id}'s ${face} face is not stacked at its cut widths`);
            y += w;
          }
        }
      }
    }
    t.diagnostic(`${faces} cut faces stacked at their cut widths`);
    assert.ok(faces > 0, "no face is cut, so this proves nothing");
  });

  test("a ribbon keeps its width at an end whose mark is drawn whole", (t) => {
    let kept = 0;
    for (const c of charts) {
      for (const l of c.laid.links) {
        if (!l.source.clamped) { assert.equal(sankey.endWidth(l, "source"), l.width, `${c.label}: ${key(l)} at its source`); kept++; }
        if (!l.target.clamped) { assert.equal(sankey.endWidth(l, "target"), l.width, `${c.label}: ${key(l)} at its target`); kept++; }
        // Tapered where its two ends are cut by ratios a visible width apart.
        // Mutation: compare the end widths exactly in isTapered, and the
        // synthetic city's General Fund and its one fund, cut by ratios ulps
        // apart, taper.
        const ratio = (n) => (sankey.isClamped(n) ? n.clamped : 1);
        assert.equal(sankey.isTapered(l), Math.abs(ratio(l.source) - ratio(l.target)) * l.width > sankey.TAPER_EPSILON,
          `${c.label}: ${key(l)} tapers wrongly`);
      }
    }
    t.diagnostic(`${kept} ribbon ends at a mark drawn whole, each at d3's width`);
  });

  test("no ribbon is lost or drawn twice, and no mark leaves its column's canvas", () => {
    for (const c of charts) {
      assert.deepEqual(c.laid.links.map(key).sort(), c.drawn.links.map(key).sort(), c.label);
      const off = c.laid.nodes.filter((n) => n.y0 < 12 - 1e-6 || n.y1 > c.height - 12 + 1e-6).map((n) => n.id);
      assert.deepEqual(off, [], `${c.label}: a mark drawn off its canvas`);
    }
  });
});

describe("the page marks a cut mark and says so", () => {
  test("the opening chart draws each cut mark's strokes and tapered ribbons, and its tip says it is cut", async (t) => {
    const { app, document } = await bootedApp();
    app.expandNode("fund-group/general", "funds");
    await settle();
    const chart = document.getElementById("chart");
    const marks = [...chart.querySelectorAll("g.node")];
    const cut = marks.filter((g) => g.__data__.clamped);
    t.diagnostic(`cut: ${cut.map((g) => g.__data__.id).join(", ")}`);
    assert.ok(cut.length > 0, "nothing on this chart is cut, so this proves nothing");
    for (const g of marks) {
      const p = g.querySelector("path.cut");
      const id = g.__data__.id;
      assert.ok(p !== null, `${id} has no cut path`);
      assert.equal(p.getAttribute("display"), g.__data__.clamped ? null : "none", `${id}'s cut is shown wrongly`);
      if (g.__data__.clamped) assert.equal(p.getAttribute("d"), sankey.cutMark(g.__data__), id);
    }
    const ribbons = [...chart.querySelectorAll("path.link")];
    assert.deepEqual(ribbons.map((p) => key(p.__data__)).sort(), app.projection.links.map(key).sort(),
      "the ribbons drawn are not the chart's, one each");
    const tapered = ribbons.filter((p) => p.classList.contains("taper"));
    t.diagnostic(`${tapered.length} of ${ribbons.length} ribbons drawn as tapered bands`);
    assert.ok(tapered.length > 0, "no ribbon is drawn tapered");
    for (const p of ribbons) {
      const d = p.__data__;
      const band = sankey.isTapered(d) && !app.isHairline(app.CONFIG.overview, d);
      assert.equal(p.classList.contains("taper"), band, key(d));
      if (band) {
        assert.equal(p.getAttribute("d"), sankey.taperPath(d), key(d));
        assert.ok(p.style.getPropertyValue("--ribbon") !== "", `${key(d)} is handed no colour to fill with`);
      }
    }
    const g = cut[0];
    fire(g, "pointerenter", { clientX: 10, clientY: 10 });
    const tip = document.getElementById("tooltip");
    const said = [...tip.querySelectorAll(".tip-cut")].map((e) => e.textContent);
    assert.deepEqual(said, [sankey.clampNote(g.__data__)]);
    assert.match(said[0], /^◇ our drawing: this mark is cut to \d+% of its full height/);
    t.diagnostic(said[0]);
    // A mark drawn whole says nothing of the kind.
    fire(marks.find((m) => !m.__data__.clamped), "pointerenter", { clientX: 10, clientY: 10 });
    assert.equal(tip.querySelectorAll(".tip-cut").length, 0);
  });
});

// Mutation: test `!d.clamped` in cutMark and clampNote again and a mark cut
// to a ratio of 0 draws no strokes and says nothing of its cut; say the cut
// is "so its column fits" and the wording arm goes red.
test("a mark cut to nothing on a canvas too short for any is still drawn and said as cut", () => {
  const mark = { id: "fund-group/general", role: "fund_group", layer: 1, x0: 10, x1: 20, y0: 0, y1: 40, sourceLinks: [], targetLinks: [] };
  sankey.clampFunds({ nodes: [mark], links: [] }, 0, 1);
  assert.equal(mark.clamped, 0, "the canvas leaves the mark no height, so its ratio is 0");
  assert.ok(sankey.isClamped(mark));
  assert.notEqual(sankey.cutMark(mark), "");
  assert.match(sankey.clampNote(mark), /cut to <1% of its full height/);
  assert.match(sankey.clampNote(mark), /not drawn to scale/, "the cut only shortens the mark; the note says it is off scale");
  assert.doesNotMatch(sankey.clampNote(mark), /fits/, "the column fits uncut; the cut keeps one mark from swamping it");
  const whole = { id: "fund-group/other", role: "fund_group", layer: 1, x0: 0, x1: 0, y0: 0, y1: 0 };
  assert.ok(!sankey.isClamped(whole));
  assert.equal(sankey.cutMark(whole), "");
  assert.equal(sankey.clampNote(whole), "");
});

// The cut closes the column up under the cut mark by the height it took off,
// keeping every gap d3 laid, and recentres the column where it stood.
// Mutation: drop the move of the marks under the cut and the gap under it
// opens by the height freed.
test("a cut column closes up under the cut, keeping each gap d3 laid, and recentres", () => {
  const mark = (id, y0, y1) => ({ id, role: "fund", tier: 3, layer: 1, x0: 0, x1: 10, y0, y1, sourceLinks: [], targetLinks: [] });
  const [big, b, c] = [mark("fund/100", 0, 160), mark("fund/610", 190, 200), mark("fund/620", 220, 240)];
  const centre = (ns) => (Math.min(...ns.map((n) => n.y0)) + Math.max(...ns.map((n) => n.y1))) / 2;
  const before = centre([big, b, c]);
  sankey.clampFunds({ nodes: [big, b, c], links: [] }, 0, 200);
  assert.ok(sankey.isClamped(big), "the planted mark is not tall enough to cut");
  assert.deepEqual({ under: b.y0 - big.y1, next: c.y0 - b.y1, centre: centre([big, b, c]) }, { under: 30, next: 20, centre: before });
});

// layOut clamps between 12 and height - 12, so a canvas under 24px hands
// clampFunds a bottom above its top.
// Mutation: drop the floor on clampFunds' most -- the mark's height and its
// ribbon's cut width go negative and the test goes red.
test("a canvas shorter than its margins cuts a mark to nothing, never below", () => {
  const link = { width: 30 };
  const mark = { id: "fund-group/general", role: "fund_group", layer: 1, x0: 10, x1: 20, y0: 5, y1: 45, sourceLinks: [link], targetLinks: [] };
  sankey.clampFunds({ nodes: [mark], links: [link] }, 12, 20 - 12);
  assert.equal(mark.y1 - mark.y0, 0, "the mark's drawn height");
  assert.equal(mark.clamped, 0, "the mark's cut ratio");
  assert.equal(link.w0, 0, "the ribbon's width at the cut face");
  assert.equal(link.w1, 30, "the ribbon's width at its far end");
});

/**
 * The marks cut over the synthetic city's columns: its General Fund, planted
 * as most of every year's money (synthcity.Default), cut at the root, opened
 * into its one fund, and with everything open.
 */
const CUT = [
  "FY 2025-26, root: fund-group/general",
  "FY 2025-26, fund-group/general by funds: fund-group/general",
  "FY 2025-26, fund-group/general by funds: fund/100",
  "FY 2025-26, everything by primary splits: fund-group/general",
  "FY 2025-26, everything by primary splits: fund/100",
  "FY 2026-27, root: fund-group/general",
  "FY 2026-27, fund-group/general by funds: fund-group/general",
  "FY 2026-27, fund-group/general by funds: fund/100",
  "FY 2026-27, everything by primary splits: fund-group/general",
  "FY 2026-27, everything by primary splits: fund/100",
];
