// fold.test.mjs — the client's fold: capColumn and foldDocument, and the
// claim that the drill-down could not be drawn without them. The fold is the
// client's alone; the unfolded schedule is what every figure is held to.

import { before, describe, test } from "node:test";
import assert from "node:assert/strict";

import {
  loadApp, bootedApp, opened, expandAll, settle, columnFixture, goldenGraph, pageFixture, publishedColumns,
} from "./testlib.mjs";

/** A module drawing one tier set: the pinned config with render_tiers replaced. */
async function drawing(tiers) {
  const config = structuredClone(pageFixture().config);
  config.render_tiers = tiers;
  return (await loadApp({ config })).app;
}

/** One published column's fund-flows schedule, as the page rehydrates it. */
function fundFlows(app, stem) {
  const doc = app.scheduleOf(columnFixture(stem), "fund-flows");
  assert.ok(doc, `${stem} carries no fund-flows schedule`);
  return doc;
}

/**
 * The tier set the fold's own clauses are exercised at, which is not a view:
 * the only three-tier fold over the WHOLE document, placing every node without
 * a filter, which a link whose ends fold together, a drawn tier no folded
 * link touches, and a re-pointed parent all need to be visible.
 */
const DRAWN = [0, 2, 4];

/**
 * A drill-down in miniature: nine nodes carrying every shape the fold has a
 * rule for. Each node is here because a clause of foldDocument is about it:
 * fund/100 and fund/101 are two funds of one group (two links MERGE);
 * fund/640 a second group (the merge is keyed); dept/fire a drawn tier reached
 * through a fund; expenditure/fire/* a tier not drawn, folding onto its own
 * parent (the SELF-LOOP); fund-group/capital a drawn tier no folded link
 * touches (the DROP); dept/parks retained while its own group is dropped (the
 * re-pointed parent must WALK PAST it).
 */
function miniature() {
  const roleOf = (id) => (id.startsWith("fund-group/") ? "fund_group" : "");
  const node = (id, tier, parent) => ({
    id, label: id, tier, parent, constraint_tier: "", role: roleOf(id),
    derived: false, rationale: "", source_note: "",
  });
  const link = (source, target, cents, facts, kind = "external",
                locators = [{ doc_id: "d", pages: [1] }]) => ({
    source, target, value_cents: cents, kind, transfer_id: "",
    fact_ids: facts, locators, derived: false,
  });
  return {
    schema_version: 1,
    projection: "miniature",
    metadata: { sources: [{ doc_id: "d", pages: [1] }] },
    nodes: [
      node("revenue/tax", 0, ""),
      node("fund-group/general", 2, ""),
      node("fund-group/enterprise", 2, ""),
      node("fund-group/capital", 2, ""),
      node("fund/100", 3, "fund-group/general"),
      node("fund/101", 3, "fund-group/general"),
      node("fund/640", 3, "fund-group/enterprise"),
      node("fund/700", 3, "fund-group/capital"),
      node("dept/fire", 4, "fund/100"),
      node("dept/parks", 4, "fund/700"),
      node("expenditure/fire/wages", 5, "dept/fire"),
    ],
    links: [
      link("revenue/tax", "fund/100", 100, ["a"]),
      link("revenue/tax", "fund/101", 200, ["b"]),
      link("revenue/tax", "fund/640", 400, ["c"]),
      link("fund/100", "dept/fire", 90, ["d"]),
      link("dept/fire", "expenditure/fire/wages", 90, ["d"]),
      link("fund/640", "dept/parks", 50, ["e"]),
    ],
  };
}

/** Every fact id any link cites. */
function cited(doc) {
  const out = new Set();
  for (const l of doc.links) for (const id of l.fact_ids) out.add(id);
  return out;
}

/** The smallest mark the layout produced, before render() floors it. */
function smallest(graph) {
  return {
    node: Math.min(...graph.nodes.map((n) => n.y1 - n.y0)),
    ribbon: Math.min(...graph.links.map((l) => l.width)),
    columns: [...graph.nodes.reduce((m, n) => m.set(n.depth, (m.get(n.depth) || 0) + 1), new Map())]
      .sort((a, b) => a[0] - b[0]).map((e) => e[1]),
  };
}

describe("the tail's note carries the figure the tail is drawn at", () => {
  // LAID OUT AS shapeFor LAYS IT OUT, markContra after the fold: a reduction
  // is drawn at its magnitude, and layOut alone would size it signed.
  // FIVE LINES INTO ONE CATEGORY, ONE OF THEM A REDUCTION, capped at two: the
  // tail is $60, -$30 and $10, which merge into one ribbon of $40 -- not the
  // $100 their magnitudes come to, which is what the ranking sorts by.
  function lines() {
    const node = (id, tier, parent) => ({
      id, label: id, tier, parent, constraint_tier: "", role: "",
      derived: false, rationale: "", source_note: "",
    });
    const link = (source, target, cents, kind = "external") => ({
      source, target, value_cents: cents, kind, transfer_id: "",
      fact_ids: [source], locators: [{ doc_id: "d", pages: [1] }], derived: false,
    });
    return { node, link,
      schema_version: 1, projection: "lines", metadata: { sources: [{ doc_id: "d", pages: [1] }] },
      nodes: [node("revenue/tax", 2, ""), node("line/a", 0, ""), node("line/b", 0, ""),
        node("line/c", 0, ""), node("line/d", 0, ""), node("line/e", 0, "")],
      links: [link("line/a", "revenue/tax", 10000), link("line/b", "revenue/tax", 8000),
        link("line/c", "revenue/tax", 6000), link("line/d", "revenue/tax", -3000), link("line/e", "revenue/tax", 1000)],
    };
  }
  test("a reduction folded into the tail is netted, as the merged ribbon draws it", async (t) => {
    const app = await drawing(DRAWN);
    const capped = app.capColumn(lines(), 0, 2, "", "lines");
    const tail = capped.nodes.find((n) => app.isAggregate(n.id));
    assert.ok(tail, "nothing folded");
    assert.equal(tail.label, "3 smaller lines");
    const folded = app.foldDocument(capped);
    const drawnAt = app.layOut(app.markContra(folded)).nodes.find((n) => n.id === tail.id).value;
    t.diagnostic(`the tail is laid out at ${drawnAt} cents and tailFigure reads ${app.tailFigure(folded, tail.id)}`);
    assert.equal(drawnAt, 4000);
    assert.equal(app.tailFigure(folded, tail.id), 4000);
  });

  // THE PAGE'S OWN CAPPING FINISHES THE NOTE AFTER THE FOLD: only the merge
  // makes the tail's three ribbons the one ribbon the chart draws.
  test("the page's capping writes the figure the fold leaves, not the one before it", async (t) => {
    const app = await drawing(DRAWN);
    const doc = lines();
    // Opened on the category the lines run into, which every line is drawn for.
    const rung = { id: "revenue/tax", step: { caps: [{ tier: 0, cap: 2 }], tail: "lines" } };
    const side = app.sideOf(doc, rung, [0, 2], false);
    const tail = side.nodes.find((n) => app.isAggregate(n.id));
    assert.ok(tail, "nothing folded");
    t.diagnostic(`the tail's note reads "${tail.source_note}"`);
    assert.ok(tail.source_note.endsWith(", together $40."), tail.source_note);
  });

  // A REDUCTION OF ANOTHER KIND IS NOT NETTED, because foldDocument merges by
  // kind as well as ends.
  test("a reduction of another kind is drawn beside the addition, not netted against it", async (t) => {
    const app = await drawing(DRAWN);
    const doc = lines();
    doc.links[3] = doc.link("line/d", "revenue/tax", -3000, "internal");
    const folded = app.foldDocument(app.capColumn(doc, 0, 2, "", "lines"));
    const tail = folded.nodes.find((n) => app.isAggregate(n.id));
    const drawnAt = app.layOut(app.markContra(folded)).nodes.find((n) => n.id === tail.id).value;
    t.diagnostic(`${folded.links.filter((l) => l.source === tail.id).length} ribbon(s) leave the tail, laid out at ${drawnAt}`);
    assert.equal(drawnAt, 10000);
    assert.equal(app.tailFigure(folded, tail.id), drawnAt);
  });

  // A FOLDED NODE'S CHILDREN STAY, hung from the tail: a fund group's window
  // folds its smallest funds, and their object categories are money the
  // column beyond them must still carry.
  test("a folded node's children hang from the tail and carry their money onward", async (t) => {
    const app = await drawing([2, 3, 5]);
    const doc = lines();
    const funds = ["fund/1", "fund/2", "fund/3", "fund/4"];
    doc.nodes = [doc.node("fund-group/g", 2, "")];
    doc.links = [];
    funds.forEach((f, i) => {
      doc.nodes.push(doc.node(f, 3, "fund-group/g"), doc.node(`expenditure/${f}/x`, 5, f));
      doc.links.push(doc.link("fund-group/g", f, 1000 * (4 - i)), doc.link(f, `expenditure/${f}/x`, 1000 * (4 - i)));
    });
    const capped = app.capColumn(doc, 3, 1, "fund-group/g", "funds");
    const tail = capped.nodes.find((n) => app.isAggregate(n.id));
    const into5 = (d) => d.links.filter((l) => d.nodes.find((n) => n.id === l.target).tier === 5)
      .reduce((a, l) => a + l.value_cents, 0);
    const hung = capped.nodes.filter((n) => n.parent === tail.id).map((n) => n.id);
    t.diagnostic(`${tail.label} folds ${JSON.stringify(tail.folds)}; ${hung.length} child(ren) hang from it; ` +
      `${into5(capped)} of ${into5(doc)} cents still reach the fifth tier`);
    assert.deepEqual(tail.folds, ["fund/2", "fund/3", "fund/4"]);
    assert.deepEqual(hung.sort(), ["expenditure/fund/2/x", "expenditure/fund/3/x", "expenditure/fund/4/x"]);
    assert.equal(into5(capped), into5(doc));
    assert.equal(into5(app.foldDocument(capped)), into5(doc));
  });

  // A SECOND CAP RE-POINTS WHAT THE FIRST TAIL HELD: capping the categories
  // after the lines merges the first tail's ribbons into ones between two
  // tails, and a reduction among them nets only then.
  test("a second cap's merge is what the first tail is drawn at", async (t) => {
    const app = await drawing(DRAWN);
    const doc = lines();
    doc.nodes.push(doc.node("revenue/fees", 2, ""), doc.node("revenue/fines", 2, ""));
    doc.links.push(doc.link("line/c", "revenue/fees", -2000), doc.link("line/e", "revenue/fines", 500),
      doc.link("line/a", "revenue/fees", 1), doc.link("line/b", "revenue/fines", 1));
    const capped = app.capColumn(app.capColumn(doc, 0, 2, "", "lines"), 2, 1, "", "categories");
    const folded = app.foldDocument(capped);
    const laid = app.layOut(app.markContra(folded));
    const tails = folded.nodes.filter((n) => app.isAggregate(n.id));
    assert.equal(tails.length, 2);
    for (const tail of tails) {
      const drawnAt = laid.nodes.find((n) => n.id === tail.id).value;
      t.diagnostic(`${tail.label} laid out at ${drawnAt}`);
      assert.equal(app.tailFigure(folded, tail.id), drawnAt);
    }
  });
});

describe("the drill-down is drawable only folded", () => {
  const COLUMNS = [
    { label: "FY 2025-26", stem: "fy2026-adopted", hairlines: 7, slivers: 4 },
    { label: "FY 2026-27", stem: "fy2027-adopted", hairlines: 8, slivers: 5 },
  ];
  let whole, drill;
  before(async () => {
    whole = await drawing([]);
    drill = await drawing(DRAWN);
  });
  for (const col of COLUMNS) {
    test(`${col.label}: the drill-down cannot be drawn unfolded, and that is not a matter of degree`, (t) => {
      const raw = fundFlows(whole, col.stem);
      const flat = smallest(whole.layOut(raw));
      t.diagnostic(`unfolded: ${raw.nodes.length} nodes over ${flat.columns.length} columns (${flat.columns.join("/")}), smallest node ${flat.node.toFixed(4)}px and smallest ribbon ${flat.ribbon.toFixed(4)}px`);
      // === and not assert.equal: d3 hands back -0 for a mark of no height. A
      // ribbon of no width can come back a rounding error either side of zero.
      assert.ok(flat.node === 0, `smallest node ${flat.node}`);
      assert.ok(Math.abs(flat.ribbon) < 1e-9, `smallest ribbon ${flat.ribbon}`);
    });
    test(`${col.label}: folded, every node has height and every ribbon has width`, (t) => {
      const folded = drill.foldDocument(fundFlows(drill, col.stem));
      const drawn = smallest(drill.layOut(folded));
      t.diagnostic(`folded to tiers ${DRAWN.join("/")}: ${folded.nodes.length} nodes over ${drawn.columns.length} columns (${drawn.columns.join("/")}), smallest node ${drawn.node.toFixed(3)}px, smallest ribbon ${drawn.ribbon.toFixed(3)}px`);
      assert.ok(drawn.node > 0);
      assert.ok(drawn.ribbon > 0);
    });
    test(`${col.label}: the count of marks too small to encode their value is pinned`, (t) => {
      const folded = drill.foldDocument(fundFlows(drill, col.stem));
      const laid = drill.layOut(folded);
      const hairlines = laid.links.filter((l) => l.width < 1).length;
      const slivers = laid.nodes.filter((n) => n.y1 - n.y0 < 2).length;
      t.diagnostic(`${hairlines} of ${folded.links.length} ribbons lay out under 1px and are drawn at 1px; ${slivers} of ${folded.nodes.length} node rects lay out under 2px and are drawn at 2px`);
      assert.equal(hairlines, col.hairlines);
      assert.equal(slivers, col.slivers);
    });
    // AT THE FUND-GROUP WINDOW'S OWN COLUMNS: a fold with no object column
    // collapses pp.172-183's fund-to-object ribbons onto their group, and
    // those facts are cited by nothing else.
    test(`${col.label}: the fold to the fund-group window's columns cites nothing away`, async (t) => {
      const wide = await drawing([0, 2, 3, 4, 5]);
      const raw = fundFlows(wide, col.stem);
      const folded = wide.foldDocument(raw);
      const before_ = cited(raw), after = cited(folded);
      t.diagnostic(`${before_.size} facts cited by ${raw.links.length} links before the fold, ${after.size} by ${folded.links.length} after`);
      assert.equal(after.size, before_.size);
      for (const id of before_) assert.ok(after.has(id), `${id} cited away`);
    });
    test(`${col.label}: a node inherits its fund group through parent, at a tier set that needs it`, async (t) => {
      const deep = await drawing([0, 2, 4, 5]);
      const doc = deep.foldDocument(fundFlows(deep, col.stem));
      deep.layOut(doc);
      const objects = doc.nodes.filter((n) => n.id.startsWith("expenditure/"));
      const depts = doc.nodes.filter((n) => n.id.startsWith("dept/"));
      // A fund's own object cells (pp.172-183) resolve to the group its fund
      // node is parented to in the document; a division's to the General Fund.
      const raw = new Map(fundFlows(deep, col.stem).nodes.map((n) => [n.id, n]));
      const groupOf = (n) => n.id.startsWith("expenditure/fund/")
        ? raw.get(raw.get(n.id).parent).parent : "fund-group/general";
      const own = objects.filter((n) => n.id.startsWith("expenditure/fund/"));
      t.diagnostic(`${objects.length} object cells (${own.length} of them a fund's own) and ${depts.length} divisions resolve through parent`);
      assert.ok(objects.length > 0 && depts.length > 0 && own.length > 0);
      assert.ok(own.some((n) => groupOf(n) !== "fund-group/general"));
      for (const n of objects) assert.equal(deep.fundGroupOf(n), groupOf(n), n.id);
      for (const n of depts) assert.equal(deep.fundGroupOf(n), "fund-group/general", n.id);
    });
  }
});

describe("foldDocument's clauses, on the miniature", () => {
  let drill, mini, byPair;
  before(async () => {
    drill = await drawing(DRAWN);
    mini = drill.foldDocument(miniature());
    byPair = new Map(mini.links.map((l) => [l.source + " -> " + l.target, l]));
  });
  test("the fold merges two funds of one group onto one ribbon, keeping both facts", (t) => {
    const l = byPair.get("revenue/tax -> fund-group/general");
    assert.ok(l, "no revenue/tax -> fund-group/general link at all");
    t.diagnostic(`${l.value_cents} cents citing ${l.fact_ids.join("+")}`);
    assert.equal(l.value_cents, 300);
    assert.deepEqual(l.fact_ids, ["a", "b"]);
  });
  test("a folded ribbon's locators are the union of its legs', pages ascending", () => {
    const doc = miniature();
    doc.links[0].locators = [{ doc_id: "d", pages: [3] }];
    doc.links[1].locators = [{ doc_id: "d", pages: [1] }, { doc_id: "acfr", pages: [7] }];
    const l = drill.foldDocument(doc).links.find((x) => x.source === "revenue/tax" && x.target === "fund-group/general");
    assert.deepEqual(l.locators, [{ doc_id: "acfr", pages: [7] }, { doc_id: "d", pages: [1, 3] }]);
  });
  test("two legs read off one page fold to ONE locator, not two", () => {
    const doc = miniature();
    doc.links[0].locators = [{ doc_id: "d", pages: [1] }];
    doc.links[1].locators = [{ doc_id: "d", pages: [1] }];
    const l = drill.foldDocument(doc).links.find((x) => x.source === "revenue/tax" && x.target === "fund-group/general");
    assert.deepEqual(l.locators, [{ doc_id: "d", pages: [1] }]);
    assert.deepEqual(l.fact_ids, ["a", "b"]);
  });
  test("a link whose ends fold together is dropped rather than drawn as a loop", (t) => {
    t.diagnostic(mini.links.map((l) => l.source + "->" + l.target).join(", "));
    assert.equal(mini.links.length, 4);
    assert.ok(!mini.links.some((l) => l.source === l.target));
    assert.ok(byPair.has("fund-group/general -> dept/fire"));
  });
  test("a drawn tier no folded link touches is not drawn", (t) => {
    t.diagnostic(mini.nodes.map((n) => n.id).join(", "));
    assert.ok(!mini.nodes.some((n) => n.id === "fund-group/capital"));
    assert.equal(mini.nodes.length, 5);
  });
  test("a folded node's parent names a node the folded document still carries", (t) => {
    t.diagnostic(mini.nodes.map((n) => n.id + "<-" + (n.parent || "root")).join(", "));
    for (const n of mini.nodes) assert.ok(!n.parent || mini.nodes.some((m) => m.id === n.parent), n.id);
    assert.equal(mini.nodes.find((n) => n.id === "dept/fire").parent, "fund-group/general");
    assert.equal(mini.nodes.find((n) => n.id === "dept/parks").parent, "");
  });
  test("a node the tier set cannot place stops the draw rather than vanishing from it", () => {
    const orphan = miniature();
    orphan.nodes.push({ id: "stray", label: "stray", tier: 9, parent: "", constraint_tier: "", role: "", derived: false, rationale: "", source_note: "" });
    orphan.links.push({ source: "revenue/tax", target: "stray", value_cents: 1, kind: "external", transfer_id: "", fact_ids: ["z"], locators: [{ doc_id: "d", pages: [1] }], derived: false });
    assert.throws(() => drill.foldDocument(orphan), (e) => /stray/.test(e.message) && /tier 9/.test(e.message));
  });
  // The fold is the one place a printed leg and an inferred one could meet in
  // one ribbon, so it is the fold that refuses them.
  test("a merged ribbon sums and cites every leg, and refuses a printed leg beside an inferred one", () => {
    const leg = { source: "revenue/tax", target: "fund/101", value_cents: 7, kind: "external", transfer_id: "", fact_ids: ["y"], locators: [{ doc_id: "d", pages: [1] }], derived: false };
    const printed = miniature();
    printed.links.push(leg);
    const l = drill.foldDocument(printed).links.find((x) => x.source === "revenue/tax" && x.target === "fund-group/general");
    assert.ok(l);
    assert.equal(l.derived, false);
    assert.equal(l.value_cents, 307);
    assert.deepEqual([...l.fact_ids].sort(), ["a", "b", "y"]);
    const mixed = miniature();
    mixed.links.push(Object.assign({}, leg, { derived: true }));
    assert.throws(() => drill.foldDocument(mixed), /merges a printed flow and an inferred one/);
  });
});

describe("the spine's fund groups", () => {
  test("the parent walk cannot move the spine, because the spine has no parents", async (t) => {
    const whole = await drawing([]);
    const spine = goldenGraph();
    t.diagnostic(`all ${spine.nodes.length} spine nodes are parentless`);
    for (const n of spine.nodes) {
      assert.equal(n.parent, "", n.id);
      assert.equal(whole.fundGroupOf(n), whole.isFundGroup(n) ? n.id : "", n.id);
    }
  });

  /**
   * The spine drawn for real over a column, and the legend the draw built.
   * `alter` edits a clone of the pinned FY2026 column before it is served.
   */
  async function spineLegend(alter) {
    const col = columnFixture("fy2026-adopted");
    if (alter) alter(col);
    const { app, document } = await bootedApp({ checkedStem: "sankey", plan: { "fy2026-adopted.json": { doc: col } } });
    const buttons = [...document.querySelectorAll("#legend [data-node]")];
    return {
      ids: buttons.map((b) => b.dataset.node),
      vars: buttons.map((b) => b.querySelector("[data-var]")?.dataset.var),
      column: app.layOut(app.shapeFor(app.projection)).nodes.filter(app.isFundGroup).sort((a, b) => a.y0 - b.y0).map((n) => n.id),
      served: app.fundGroups().map((g) => g.id),
    };
  }
  /** Adds a fund-group node modelled on debt-service, with one inflow, under `id`. */
  function withGroup(col, id, label, listed) {
    const model = col.nodes.findIndex((n) => n.id === "fund-group/debt-service");
    col.nodes.push(Object.assign({}, col.nodes[model], { id, label }));
    const idx = col.nodes.length - 1;
    const sched = col.schedules.sankey;
    sched.nodes.push({ node: idx });
    const link = sched.links.find((l) => l.to === model);
    sched.links.push(Object.assign({}, link, { to: idx }));
    col.tiers.find((t) => t.tier === col.nodes[model].tier).nodes.push(idx);
    if (listed) col.fund_groups.push({ id, slot: 0 });
  }

  test("the legend is the column's fund groups, in the order it shipped them, and each has flows", async (t) => {
    const spine = await spineLegend();
    t.diagnostic(spine.ids.map((id) => id.replace("fund-group/", "")).join(", "));
    assert.ok(spine.served.length > 0);
    assert.deepEqual(spine.ids, spine.served);
  });
  test("a fund group the stylesheet has no hue for is still in the legend, last, and muted", async (t) => {
    const spine = await spineLegend();
    const seventh = await spineLegend((c) => withGroup(c, "fund-group/permanent", "Permanent Funds", true));
    t.diagnostic(seventh.ids.map((id, i) => id.replace("fund-group/", "") + " " + seventh.vars[i]).join(", "));
    assert.equal(seventh.ids.length, spine.served.length + 1);
    assert.deepEqual(seventh.ids, seventh.served);
    assert.equal(seventh.ids.at(-1), "fund-group/permanent");
    assert.equal(seventh.vars.at(-1), "--muted");
    assert.ok(seventh.vars.slice(0, -1).every((v) => v !== "--muted"));
  });
  test("a fund group is one by its role, even where its id is not under fund-group/", async (t) => {
    const spine = await spineLegend();
    const odd = await spineLegend((c) => withGroup(c, "fund-type/permanent", "Permanent Funds", true));
    t.diagnostic(`${odd.ids.length} swatch(es), last ${odd.ids.at(-1)} drawn ${odd.vars.at(-1)}; the fund column ends ${odd.column.at(-1)}`);
    assert.equal(odd.ids.length, spine.ids.length + 1);
    assert.equal(odd.ids.at(-1), "fund-type/permanent");
    assert.equal(odd.column.at(-1), "fund-type/permanent");
    assert.equal(odd.vars.at(-1), "--muted");
  });
});

describe("the cap is what makes a fund group's column drawable", () => {
  /** The window's ribbons capped at the shipped budget and drawn out whole. */
  async function measure(group) {
    const { app } = await bootedApp({ checkedStem: "sankey" });
    app.setColumnBudget(3);
    await opened(app, group);
    const read = () => {
      const w = app.layOut(app.projection).links.map((l) => l.width);
      return { ribbons: w.length, sub: w.filter((x) => x < 1).length, min: Math.min(...w).toFixed(3) };
    };
    const capped = read();
    expandAll(app);
    await settle();
    const whole = read();
    const at3 = app.projection.nodes.filter((n) => n.tier === 3);
    const weight = (id) => app.projection.links.filter((l) => l.source === id || l.target === id)
      .reduce((a, l) => a + Math.abs(l.value_cents), 0);
    const shares = at3.map((n) => weight(n.id)).sort((a, b) => b - a);
    const sum = shares.reduce((a, b) => a + b, 0);
    return { capped, whole, top: (100 * shares[0] / sum).toFixed(1), bottom: (100 * shares.at(-1) / sum).toFixed(3) };
  }
  test("capping special-revenue and capital removes every sub-pixel ribbon the whole column draws", async (t) => {
    const sr = await measure("fund-group/special-revenue");
    const cap = await measure("fund-group/capital");
    t.diagnostic(`special-revenue ${sr.capped.ribbons} ribbons / ${sr.capped.sub} sub-pixel / ${sr.capped.min}px capped, ${sr.whole.ribbons} / ${sr.whole.sub} / ${sr.whole.min}px whole, largest fund ${sr.top}% and smallest ${sr.bottom}% of the column; capital ${cap.capped.ribbons} / ${cap.capped.sub} capped, ${cap.whole.ribbons} / ${cap.whole.sub} whole`);
    assert.equal(sr.capped.sub, 0);
    assert.equal(cap.capped.sub, 0);
    assert.ok(sr.whole.sub > 0);
  });
});

// The parent-chain arm node-hierarchy-well-formed keeps off every
// column the export writes.
describe("a parent the document does not carry", () => {
  test("a node whose parent chain breaks belongs to no fund group", async () => {
    const whole = await drawing([]);
    const stray = { id: "fund/999", label: "stray", tier: 3, parent: "fund-group/ghost", role: "",
      constraint_tier: "", derived: false, rationale: "", source_note: "" };
    assert.equal(whole.fundGroupOf(stray), "");
    const rooted = Object.assign({}, stray, { parent: "" });
    assert.equal(whole.fundGroupOf(rooted), "");
  });
});

// THE FOLD'S OWN PROPERTIES, over every rung the shipped steps can open on the
// pinned columns, at the widest tier set the step declares and the narrowest
// the budget leaves. Each half of a chart is filtered, capped and folded by
// the same three functions the page draws with, and what comes out is held to
// what went in: the sums, the citations and the caps. Nothing here compares a
// figure to anything Go computed; the unfolded schedule is the only witness.
describe("the fold preserves what it folds, on every rung at every width", () => {
  const CONFIG = pageFixture().config;
  // Every year the page publishes: a year added to the page is walked once its
  // column is pinned, and refused until it is.
  const STEMS = publishedColumns();

  test("every published year's column is pinned, so the walk below reaches it", () => {
    for (const stem of STEMS) {
      assert.doesNotThrow(() => columnFixture(stem), `the page publishes ${stem} and testdata/ pins no column for it`);
    }
    assert.ok(STEMS.length > 0, "the pinned page publishes no year, so the walk below covers nothing");
  });

  /**
   * The tier sets one step is drawn at: a viewport buys the widened columns in
   * their declared order, so every prefix of `widen` over the rest.
   */
  function widths(step) {
    const keep = new Set(step.keep || []);
    const fresh = step.tiers.filter((t) => !keep.has(t));
    const widen = (step.widen || []).filter((t) => fresh.includes(t));
    const out = [];
    for (let k = 0; k <= widen.length; k++) {
      const bought = new Set(widen.slice(0, k));
      out.push(fresh.filter((t) => !widen.includes(t) || bought.has(t)));
    }
    return out;
  }

  /** Cents and cited fact ids over a set of links. */
  function totals(links) {
    const cited = new Set();
    let cents = 0;
    for (const l of links) {
      cents += l.value_cents;
      for (const id of l.fact_ids) cited.add(id);
    }
    return { cents, cited: [...cited].sort() };
  }

  for (const stem of STEMS) {
    test(`${stem}: every rung's sums, citations and caps survive the cap and the fold`, async (t) => {
      const app = (await loadApp({ config: CONFIG })).app;
      const column = columnFixture(stem);
      const wrong = [];
      let rungs = 0;
      let capped = 0;
      let merged = 0;
      /** The schedule a step draws: its own, or the one the step before it draws. */
      const drawsFrom = (step) => {
        for (let s = step, hops = 0; s && hops < 9; hops++) {
          if (s.projection) return s.projection;
          s = CONFIG.steps.find((x) => x.key === s.after[0]);
        }
        return "";
      };
      for (const step of CONFIG.steps) {
        const key = drawsFrom(step);
        const doc = app.scheduleOf(column, key);
        assert.ok(doc, `${stem} carries no schedule ${key} for step ${step.key}`);
        const window = Boolean(step.keep && step.keep.length);
        const nearIsSource = window ? app.flankIsLeft(step) : step.side === app.SIDE_SOURCE;
        const opens = [...app.decomposable(step, doc)].sort();
        assert.ok(opens.length > 0, `${stem}: step ${step.key} decomposes no node of ${key}`);
        for (const id of opens) {
          for (const tiers of widths(step)) {
            rungs++;
            const where = `${stem} ${step.key} ${id} at {${tiers}}`;
            const rung = { id, step, doc };
            const raw = app.filterLinks(doc, id, tiers, app.reaching(doc, id, nearIsSource, tiers));
            const drawn = app.sideOf(doc, rung, tiers, nearIsSource);
            // A ribbon whose two ends fold to one node is drawn by nothing,
            // and the fold drops it: it is money inside one drawn mark.
            const byID = new Map(raw.nodes.map((n) => [n.id, n]));
            const set = new Set(tiers);
            const folds = (end) => app.foldTarget(byID, byID.get(end), set);
            const kept = raw.links.filter((l) => folds(l.source) !== folds(l.target));
            // (a) SUMS, in total and at the opened node.
            const want = totals(kept);
            const got = totals(drawn.links);
            if (want.cents !== got.cents) wrong.push(`${where}: ${want.cents} cents filtered, ${got.cents} drawn`);
            const at = (links, node) => links.filter((l) => l.source === node || l.target === node)
              .reduce((a, l) => a + l.value_cents, 0);
            if (drawn.nodes.some((n) => n.id === id) && at(kept, id) !== at(drawn.links, id)) {
              wrong.push(`${where}: ${at(kept, id)} cents touch ${id} unfolded, ${at(drawn.links, id)} drawn`);
            }
            // (b) NO RIBBON LOST OR DUPLICATED: the same facts cited, and the
            // same cents -- a ribbon dropped loses its facts, one drawn twice
            // doubles its cents.
            if (want.cited.join() !== got.cited.join()) {
              const lost = want.cited.filter((f) => !got.cited.includes(f));
              const invented = got.cited.filter((f) => !want.cited.includes(f));
              wrong.push(`${where}: ${lost.length} fact(s) cited away, ${invented.length} cited from nowhere`);
            }
            merged += kept.length - drawn.links.length;
            // (c) CAPS: a capped column holds at most cap + 1 marks, and where
            // it folded, the tail's members and the kept marks are the column.
            for (const cap of step.caps || []) {
              if (!tiers.includes(cap.tier)) continue;
              const own = drawn.nodes.filter((n) => n.tier === cap.tier);
              const tail = own.find((n) => app.isAggregate(n.id));
              if (own.length > cap.cap + 1) wrong.push(`${where}: ${own.length} marks at tier ${cap.tier}, capped at ${cap.cap}`);
              if (!tail) continue;
              capped++;
              const whole = raw.nodes.filter((n) => n.tier === cap.tier).map((n) => n.id).sort();
              const stood = own.filter((n) => n !== tail).map((n) => n.id).concat(tail.folds).sort();
              if (whole.join() !== stood.join()) wrong.push(`${where}: the column holds ${whole.length} and the tail plus the kept stand for ${stood.length}`);
              if (tail.folds.length < 2) wrong.push(`${where}: a tail of ${tail.folds.length}`);
            }
          }
        }
      }
      t.diagnostic(`${stem}: ${rungs} rung-widths, ${capped} folded tails, ${merged} ribbons merged or dropped by the fold`);
      assert.ok(rungs > 0 && capped > 0 && merged > 0);
      assert.deepEqual(wrong, []);
    });
  }
});

describe("a reduction read back off the chart on screen", () => {
  // windowFor takes a kept flank off the drawn chart, which markContra has
  // flipped; unmarkContra is what lets the flank be summed at its printed sign
  // and drawn again with its sentence.
  test("unmarkContra undoes markContra, and marking again keeps every sentence", async () => {
    const app = (await loadApp()).app;
    for (const stem of ["fy2026-adopted", "fy2027-adopted"]) {
      const printed = fundFlows(app, stem);
      assert.ok(printed.links.some((l) => l.value_cents < 0), `${stem} prints no reduction, so nothing here is held`);
      const shown = app.markContra(printed);
      const back = app.unmarkContra(shown);
      assert.deepEqual(back.links.map((l) => l.value_cents), printed.links.map((l) => l.value_cents),
        `${stem}: a reduction read back off the screen is not at its printed sign`);
      const again = app.markContra(back);
      assert.deepEqual(again.links.map((l) => l.contra || ""), shown.links.map((l) => l.contra || ""),
        `${stem}: marking the chart a second time dropped a reduction's sentence`);
    }
  });
});

describe("a window's kept flank", () => {
  // The flank comes off rung.chart, the chart as drawn, which markContra has
  // flipped; windowFor reads it back at the sign it was printed at. Planted:
  // no published flank carries a reduction today.
  test("keeps a reduction it carries at its printed sign", async () => {
    const { app } = await bootedApp();
    await opened(app, "fund-group/general");
    const rung = app.drilled[app.drilled.length - 1];
    const planted = structuredClone(app.markContra(rung.chart));
    const i = planted.links.findIndex((l) => l.target === "fund-group/general" && l.value_cents > 0);
    assert.ok(i >= 0, "the spine sends nothing into fund-group/general, so no flank ribbon was planted");
    planted.links[i] = Object.assign({}, planted.links[i], { contra: "printed as a reduction of Test" });
    const want = -planted.links[i].value_cents;
    const window = app.windowFor(planted, rung.doc, Object.assign({}, rung, { chart: planted }));
    const kept = window.links.find((l) => l.source === planted.links[i].source && l.target === "fund-group/general");
    assert.ok(kept, "the planted ribbon is not in the window's flank");
    assert.equal(kept.value_cents, want, "a reduction in the kept flank is read at the screen's sign, not its printed one");
  });
});

describe("which nodes a step opens", () => {
  // reaching admits a ribbon only if it runs from an earlier drawn column to a
  // later one. No pinned document has a backward ribbon, so one is planted: a
  // fund group whose parts' ribbons all run against the column order is not
  // offered, and the same group with its ribbons as printed is.
  test("a node whose only ribbons run backwards does not open", async () => {
    const app = (await loadApp()).app;
    const step = pageFixture().config.steps.find((s) => s.key === "fund-group");
    const printed = fundFlows(app, "fy2026-adopted");
    const group = "fund-group/general";
    assert.ok(app.decomposable(step, printed).has(group), `${group} does not open over the printed document`);
    const inside = app.withinNode(printed, group);
    const planted = structuredClone(printed);
    planted.links = planted.links.map((l) => inside.has(l.source)
      ? Object.assign({}, l, { source: l.target, target: l.source })
      : l);
    assert.ok(!app.decomposable(step, planted).has(group),
      `${group} is offered though every ribbon from its parts runs against the column order`);
  });
});

describe("a printed flow and an inferred one", () => {
  // No published fold merges the two, so one is planted: two lines of one
  // category into one fund fold into one ribbon at the category grain, and
  // one of them is marked inferred.
  test("are never merged into one ribbon", async () => {
    const app = (await loadApp()).app;
    const doc = structuredClone(fundFlows(app, "fy2026-adopted"));
    const byID = new Map(doc.nodes.map((n) => [n.id, n]));
    const pair = new Map();
    let planted = null;
    for (const l of doc.links) {
      const src = byID.get(l.source);
      if (!src || src.tier !== 1 || byID.get(l.target).tier !== 3) continue;
      const key = src.parent + "\u001f" + l.target + "\u001f" + l.kind;
      if (pair.has(key)) { planted = l; break; }
      pair.set(key, l);
    }
    assert.ok(planted, "no two lines of one category reach one fund, so nothing folds together");
    assert.doesNotThrow(() => app.foldDocument(doc, [0, 2, 3, 4, 5]), "the printed document folds");
    planted.derived = true;
    assert.throws(() => app.foldDocument(doc, [0, 2, 3, 4, 5]), /merges a printed flow and an inferred one/);
  });
});
