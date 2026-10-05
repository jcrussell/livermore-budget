// form.test.mjs — a SECOND FORM FITS THE SEAM. A test-only renderer that ships
// nothing and computes no figure the core does not is registered beside the
// Sankey, one shipped step is redeclared in its form, and the page drills
// into it and back out. What the page says about the chart -- the trail, the
// counts, the table, the derived list -- comes from the generic layer, so it
// must read the same whichever form drew the marks. Go refuses this form at
// the write (internal/export/views_test.go), which is why the proof is here.

import { describe, test } from "node:test";
import assert from "node:assert/strict";
import { loadApp, settle, opened, refusals, pageFixture, dollars, stepByKey } from "./testlib.mjs";

/** The shipped step this file redeclares in the stub form. */
const STEP = "division";
/** A division under fund/100 in both pinned columns. */
const PATH = ["fund-group/general", "fund/100", "dept/city-attorney"];

/**
 * The stub renderer over the shipped app's exports. Every member app.js
 * calls on a renderer is here; every figure is core.js's (withinNode and
 * foldDocument, reached through app), and the layout is one row per node.
 * The readings of a laid mark are the stub's own: a mark's figure is the
 * cents its ribbons carry, and it states no share, no reduction and no
 * cross-tab, so the page's words over a stub rung are what those answers
 * make them.
 * @param {any} app
 * @param {{cents: number}} asked how many times the page asked the stub for a figure
 */
function stubRenderer(app, asked) {
  const STUB = Object.freeze({
    form: "stub",
    tiersOf(chart) { return (chart.stub && chart.stub.depth) || []; },
    caps() { return []; },
    columns(chart) { return STUB.tiersOf(chart); },
    width(columns) { return 300 * columns; },
    cents(d) { asked.cents++; return app.isLink(d) ? d.value_cents : d.value; },
    share() { return ""; },
    reduction() { return false; },
    crossTab() { return false; },
    reductionNote() { return ""; },
    offers(step, doc, onScreen, id) {
      const inside = app.withinNode(doc, id);
      return doc.links.some((l) => inside.has(l.source) && inside.has(l.target));
    },
    shape(doc, rung, from, tiers) {
      if (!rung) throw new Error("the stub draws no overview");
      const inside = app.withinNode(doc, rung.id);
      return app.foldDocument(Object.assign({}, doc, {
        nodes: doc.nodes.filter((n) => inside.has(n.id)),
        links: doc.links.filter((l) => inside.has(l.source) && inside.has(l.target)),
      }), tiers);
    },
    layOut(drawn, ctx) {
      const nodes = drawn.nodes.map((n, i) => {
        const column = ctx.tiers.indexOf(n.tier);
        return Object.assign({}, n, {
          x0: column * 100, x1: column * 100 + 10, y0: i * 20, y1: i * 20 + 16,
          value: 0, sourceLinks: [], targetLinks: [], depth: column, layer: column,
        });
      });
      const byID = new Map(nodes.map((n) => [n.id, n]));
      const links = drawn.links.map((l, i) => Object.assign({}, l, {
        source: byID.get(l.source), target: byID.get(l.target),
        value: l.value_cents, width: 1, y0: 0, y1: 0, index: i,
      }));
      for (const l of links) {
        l.source.sourceLinks.push(l);
        l.target.targetLinks.push(l);
        l.source.value += l.value;
      }
      return { nodes, links };
    },
    render(graph, ctx) {
      ctx.svg.selectAll("g").remove();
      ctx.svg.append("g").attr("class", "nodes").selectAll("g")
        .data(graph.nodes)
        .join("g")
        .attr("class", (d) => ctx.classes.node(d))
        .attr("aria-label", (d) => ctx.describe.node(d))
        .append("rect");
    },
    paint() {},
    widest(steps) { return steps.reduce((most, s) => Math.max(most, STUB.tiersOf(s).length), 0); },
  });
  return STUB;
}

/** The pinned config with the division step redeclared in the stub form. */
function stubConfig(record) {
  const config = structuredClone(pageFixture().config);
  const step = stepByKey(config, STEP);
  delete step.sankey;
  step.form = "stub";
  step.stub = record ? record({ depth: [4, 5] }) : { depth: [4, 5] };
  return config;
}

/** The page's words about the chart on screen, read off the real DOM. */
function words(document) {
  const text = (id) => String(document.getElementById(id).textContent).replace(/\s+/g, " ").trim();
  const crumb = document.getElementById("breadcrumb");
  return {
    counts: text("counts-line"),
    desc: text("chart-desc"),
    back: [...crumb.querySelectorAll("button.crumb-back")].map((b) => b.textContent),
    here: [...crumb.querySelectorAll("span.crumb-here")].map((s) => s.textContent).join(""),
    rows: document.querySelectorAll("#flow-table tbody tr").length,
    derived: [...document.querySelectorAll("#derived-list li")].map((li) => li.textContent.trim()),
    marks: document.querySelectorAll("#chart g.node").length,
    ribbons: document.querySelectorAll("#chart path.link").length,
  };
}

describe("a second form fits the seam", () => {
  test("a step in a form registered beside the Sankey drills in through the generic layer and back out into a Sankey rung", async (t) => {
    /** @type {{prop: string, frame: string}[]} */
    const reads = [];
    const config = stubConfig((hints) => new Proxy(hints, {
      get(target, prop) {
        reads.push({ prop: String(prop), frame: ((new Error().stack || "").split("\n")[2] || "").trim() });
        return target[prop];
      },
    }));
    const { app, document, fetch } = await loadApp({ config });
    const asked = { cents: 0 };
    app.FORMS.set("stub", stubRenderer(app, asked));
    await app.boot();
    await settle();
    assert.deepEqual(refusals(document), [], "the page boots with the stub registered");
    assert.ok(fetch.asked.length > 0);

    await opened(app, ...PATH);
    const step = config.steps.find((s) => s.key === STEP);
    const drawn = app.projection;
    const fund = app.docAt(2);
    const division = fund.nodes.find((n) => n.id === PATH[2]);
    const inside = app.withinNode(fund, PATH[2]);
    // What the stub draws is what the division contains, folded to its depth.
    assert.deepEqual(drawn.nodes.map((n) => n.id).sort(), [...inside].sort());
    assert.ok(drawn.links.length > 0);
    assert.deepEqual([...new Set(drawn.nodes.map((n) => n.tier))].sort(), [4, 5]);

    const w = words(document);
    const cited = new Set(drawn.links.flatMap((l) => l.fact_ids));
    const total = fund.metadata.counts.facts;
    t.diagnostic(`${PATH.join(" > ")} drawn by the stub: ${drawn.nodes.length} marks, ${drawn.links.length} ribbons; ` +
      `the page says ${JSON.stringify(w.counts)}; trail ${JSON.stringify(w.back)} > ${JSON.stringify(w.here)}`);
    // The trail, the counts, the table and the derived list are the page's.
    assert.deepEqual(w.back, app.drilled.map((r) => app.say("back_control", { back: r.step.back })));
    assert.equal(w.here, division.label);
    assert.equal(w.counts, cited.size === total
      ? app.say("counts", { links: drawn.links.length, nodes: drawn.nodes.length, facts: cited.size })
      : app.say("counts_partial", { links: drawn.links.length, nodes: drawn.nodes.length, cited: cited.size, facts: total }));
    assert.equal(w.rows, drawn.links.length);
    assert.deepEqual(w.derived, drawn.nodes.some((n) => n.derived) ? w.derived : [app.say("none_inferred")]);
    // The description is the step's, framed by the trail the page composed.
    assert.ok(w.desc.includes(step.description), w.desc);
    assert.ok(w.desc.includes(division.label), w.desc);
    // The stub drew the marks and no ribbons; the Sankey draws both.
    assert.equal(w.marks, drawn.nodes.length);
    assert.equal(w.ribbons, 0);
    assert.equal(app.activeTiers().length, 2);

    // THE FIGURE A MARK SHOWS IS THE STUB'S cents, asked through the
    // renderer: the panel, the tooltip and the aria label print what the stub
    // answers, and none of them carries a share chip or a reduction note,
    // which the stub states none of.
    const paying = [...document.querySelectorAll("#chart g.node")].find((g) => g.__data__.sourceLinks.length > 0);
    assert.ok(paying, "no stub mark sends a ribbon, so no figure is summed");
    const d = paying.__data__;
    const cents = d.sourceLinks.reduce((s, l) => s + l.value_cents, 0);
    const before = asked.cents;
    app.pin(d);
    app.showTip({ target: document.getElementById("chart"), clientX: 0, clientY: 0 }, d);
    const shown = {
      panel: document.querySelector("#detail .amount").textContent,
      tip: document.querySelector("#tooltip .tip-value").textContent,
      aria: paying.getAttribute("aria-label"),
      chips: [...document.querySelectorAll("#detail .chip.derived, #tooltip .chip.derived")].map((c) => c.textContent),
    };
    t.diagnostic(`${d.id} carries ${cents} cents by the stub's reading; the panel says ${JSON.stringify(shown.panel)}, ` +
      `the aria label ${JSON.stringify(shown.aria)}; the stub's cents was asked ${asked.cents - before} time(s) for the pin and tip`);
    assert.ok(cents > 0);
    assert.equal(shown.panel, dollars(cents));
    assert.equal(shown.tip, dollars(cents));
    assert.ok(shown.aria.includes(`, total ${dollars(cents)},`), shown.aria);
    assert.ok(asked.cents - before >= 2, "the pin and the tip read the figure without asking the stub");
    assert.deepEqual(shown.chips.filter((c) => c.includes("%")), [], "a share chip the stub never stated");

    // Only the stub reads the stub's hints.
    const outside = reads.filter((r) => !r.frame.includes("form.test.mjs"));
    assert.ok(reads.length > 0, "the drill read no stub hint, so the seam was not exercised");
    assert.deepEqual([...new Set(outside.map((r) => `${r.prop} read at ${r.frame}`))], []);

    // Back out through the breadcrumb: the rung above is a Sankey again.
    // Control k pops to depth k; the third closes the stub rung alone.
    document.querySelectorAll("#breadcrumb button.crumb-back")[2].click();
    await settle();
    assert.equal(app.drilled.length, 2);
    const back = words(document);
    assert.equal(back.here, app.docAt(1).nodes.find((n) => n.id === PATH[1]).label);
    assert.ok(back.ribbons > 0, "the Sankey rung draws ribbons");
    assert.equal(back.marks, app.projection.nodes.length);
    assert.deepEqual(refusals(document), []);
  });
});
