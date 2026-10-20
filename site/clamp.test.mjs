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
  let all = root;
  for (let changed = true; changed;) {
    changed = false;
    for (const id of [...all]) {
      const splits = core.splitsOf(doc, id);
      if (!splits.length || core.openSplitOf(doc, all, id) !== "") continue;
      all = core.expand(doc, all, id, splits.find((sp) => !sp.alternative).key);
      changed = true;
    }
  }
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
        assert.equal(sankey.isTapered(l), Boolean(l.source.clamped) !== Boolean(l.target.clamped) ||
          (Boolean(l.source.clamped) && l.source.clamped !== l.target.clamped), `${c.label}: ${key(l)} tapers wrongly`);
      }
    }
    t.diagnostic(`${kept} ribbon ends at a mark drawn whole, each at d3's width`);
  });

  // d3 packs the General Fund's group NODE_PADDING above the group under it,
  // measured; the cut keeps that gap rather than opening the height it took.
  test("a cut column closes up under the cut: the mark under each cut one sits NODE_PADDING below it", (t) => {
    let under = 0;
    for (const c of charts) {
      for (const n of c.laid.nodes.filter((m) => m.clamped)) {
        const column = c.laid.nodes.filter((m) => m.layer === n.layer).sort((a, b) => a.y0 - b.y0);
        const gaps = column.slice(1).map((m, i) => +(m.y0 - column[i].y1).toFixed(6));
        t.diagnostic(`${c.label}: ${n.id}'s column gaps ${gaps.join(", ") || "none, it stands alone"}`);
        const next = column[column.indexOf(n) + 1];
        if (!next) continue;
        under++;
        assert.ok(Math.abs(next.y0 - n.y1 - sankey.NODE_PADDING) <= 1e-6,
          `${c.label}: ${next.id} sits ${next.y0 - n.y1}px under ${n.id}`);
      }
    }
    assert.ok(under > 0, "no cut mark has one under it, so this proves nothing");
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

/**
 * The marks cut over the pinned columns, measured: the General Fund's group,
 * and fund/100 where the group opens into it. Everything open grows the
 * canvas (sankey.grow) past where any fund needs cutting.
 */
const CUT = [
  "FY 2025-26, root: fund-group/general",
  "FY 2025-26, fund-group/general by funds: fund-group/general",
  "FY 2025-26, fund-group/general by funds: fund/100",
  "FY 2026-27, root: fund-group/general",
  "FY 2026-27, fund-group/general by funds: fund-group/general",
  "FY 2026-27, fund-group/general by funds: fund/100",
];
