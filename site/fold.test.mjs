// fold.test.mjs — the client's fold: capColumn and foldDocument, the claim
// that the drill-down could not be drawn without them, and the expansion as
// the page caps it at every split it offers. The fold is the client's alone;
// the unfolded schedule is what every figure is held to.

import { before, describe, test } from "node:test";
import assert from "node:assert/strict";

import {
  loadApp, bootedApp, columnFixture, goldenGraph, pageFixture, publishedColumns, unmarkContra, expansionStates,
} from "./testlib.mjs";

/** A module drawing one tier set: the pinned config with the overview's tiers replaced. */
async function drawing(tiers) {
  const config = structuredClone(pageFixture().config);
  config.overview.sankey.tiers = tiers;
  return (await loadApp({ config })).app;
}

/** `doc` folded to the nodes at `tiers`, the page's own tiers by default. */
function foldAt(app, doc, tiers) {
  return app.foldDocument(doc, app.tierFrontier(doc, tiers || app.RENDER_TIERS));
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
    const folded = foldAt(app, capped);
    const drawnAt = app.layOut(app.markContra(folded)).nodes.find((n) => n.id === tail.id).value;
    t.diagnostic(`the tail is laid out at ${drawnAt} cents and tailFigure reads ${app.tailFigure(folded, tail.id)}`);
    assert.equal(drawnAt, 4000);
    assert.equal(app.tailFigure(folded, tail.id), 4000);
  });

  // A REDUCTION OF ANOTHER KIND IS NOT NETTED, because foldDocument merges by
  // kind as well as ends.
  test("a reduction of another kind is drawn beside the addition, not netted against it", async (t) => {
    const app = await drawing(DRAWN);
    const doc = lines();
    doc.links[3] = doc.link("line/d", "revenue/tax", -3000, "internal");
    const folded = foldAt(app, app.capColumn(doc, 0, 2, "", "lines"));
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
    assert.equal(into5(foldAt(app, capped)), into5(doc));
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
    const folded = foldAt(app, capped);
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
    { label: "FY 2025-26", stem: "fy2026-adopted", hairlines: 6, slivers: 3 },
    { label: "FY 2026-27", stem: "fy2027-adopted", hairlines: 8, slivers: 4 },
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
      const folded = foldAt(drill, fundFlows(drill, col.stem));
      const drawn = smallest(drill.layOut(folded));
      t.diagnostic(`folded to tiers ${DRAWN.join("/")}: ${folded.nodes.length} nodes over ${drawn.columns.length} columns (${drawn.columns.join("/")}), smallest node ${drawn.node.toFixed(3)}px, smallest ribbon ${drawn.ribbon.toFixed(3)}px`);
      assert.ok(drawn.node > 0);
      assert.ok(drawn.ribbon > 0);
    });
    test(`${col.label}: the count of marks too small to encode their value is pinned`, (t) => {
      const folded = foldAt(drill, fundFlows(drill, col.stem));
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
      const folded = foldAt(wide, raw);
      const before_ = cited(raw), after = cited(folded);
      t.diagnostic(`${before_.size} facts cited by ${raw.links.length} links before the fold, ${after.size} by ${folded.links.length} after`);
      assert.ok(before_.size > 0, `${col.stem}: the schedule cites no fact, so none can be cited away`);
      assert.equal(after.size, before_.size);
      for (const id of before_) assert.ok(after.has(id), `${id} cited away`);
    });
    test(`${col.label}: a node inherits its fund group through parent, at a tier set that needs it`, async (t) => {
      const deep = await drawing([0, 2, 4, 5]);
      const doc = foldAt(deep, fundFlows(deep, col.stem));
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
    mini = foldAt(drill, miniature());
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
    const l = foldAt(drill, doc).links.find((x) => x.source === "revenue/tax" && x.target === "fund-group/general");
    assert.deepEqual(l.locators, [{ doc_id: "acfr", pages: [7] }, { doc_id: "d", pages: [1, 3] }]);
  });
  test("two legs read off one page fold to ONE locator, not two", () => {
    const doc = miniature();
    doc.links[0].locators = [{ doc_id: "d", pages: [1] }];
    doc.links[1].locators = [{ doc_id: "d", pages: [1] }];
    const l = foldAt(drill, doc).links.find((x) => x.source === "revenue/tax" && x.target === "fund-group/general");
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
    assert.throws(() => foldAt(drill, orphan), (e) => /stray/.test(e.message) && /tier 9/.test(e.message));
  });
  // The fold is the one place a printed leg and an inferred one meet in one
  // ribbon, so it is the fold that says the merge is ours.
  test("a merged ribbon sums and cites every leg, and is drawn inferred where any leg is", () => {
    const leg = { source: "revenue/tax", target: "fund/101", value_cents: 7, kind: "external", transfer_id: "", fact_ids: ["y"], locators: [{ doc_id: "d", pages: [1] }], derived: false };
    const printed = miniature();
    printed.links.push(leg);
    const l = foldAt(drill, printed).links.find((x) => x.source === "revenue/tax" && x.target === "fund-group/general");
    assert.ok(l);
    assert.equal(l.derived, false);
    assert.equal(l.value_cents, 307);
    assert.deepEqual([...l.fact_ids].sort(), ["a", "b", "y"]);
    const mixed = miniature();
    mixed.links.push(Object.assign({}, leg, { derived: true }));
    const m = foldAt(drill, mixed).links.find((x) => x.source === "revenue/tax" && x.target === "fund-group/general");
    assert.ok(m);
    assert.equal(m.derived, true);
    assert.equal(m.value_cents, 307);
    assert.deepEqual([...m.fact_ids].sort(), ["a", "b", "y"]);
  });
});

describe("the spine's fund groups", () => {
  test("the parent walk cannot move the spine, because the spine has no parents", async (t) => {
    const whole = await drawing([]);
    const spine = goldenGraph();
    t.diagnostic(`all ${spine.nodes.length} spine nodes are parentless`);
    assert.ok(spine.nodes.length > 0, "the spine golden carries no node, so no parent is walked");
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
    const buttons = [...document.querySelectorAll("#legend [data-group]")];
    return {
      ids: buttons.map((b) => b.dataset.group),
      vars: buttons.map((b) => b.querySelector("[data-var]")?.dataset.var),
      column: app.layOut(app.shapeFor(app.projection)).nodes.filter(app.isFundGroup).sort((a, b) => a.y0 - b.y0).map((n) => n.id),
      served: app.fundGroups().map((g) => g.id),
      drawn: app.projection.nodes.filter(app.isFundGroup).map((n) => n.id),
    };
  }
  /**
   * Adds a fund-group node modelled on debt-service, with one inflow, under
   * `id`, to the schedule the page draws: a root of the expansion, as the
   * spine's groups are.
   */
  function withGroup(col, id, label, listed) {
    const model = col.nodes.findIndex((n) => n.id === "fund-group/debt-service");
    col.nodes.push(Object.assign({}, col.nodes[model], { id, label }));
    const idx = col.nodes.length - 1;
    const sched = col.schedules.expansion;
    sched.nodes.push({ node: idx });
    // The expansion prints a group's money at its funds, so the inflow is
    // one of the model group's funds', retargeted.
    const funds = new Set(sched.nodes.filter((n) => n.parent === "fund-group/debt-service").map((n) => n.node));
    const link = sched.links.find((l) => funds.has(l.to));
    sched.links.push(Object.assign({}, link, { to: idx }));
    if (listed && !col.fund_groups.some((g) => g.id === id)) col.fund_groups.push({ id, slot: 0 });
  }

  test("the legend is the column's fund groups the spine draws, in the order it shipped them, and each has flows", async (t) => {
    const spine = await spineLegend();
    t.diagnostic(spine.ids.map((id) => id.replace("fund-group/", "")).join(", "));
    assert.ok(spine.served.length > 0);
    assert.deepEqual(spine.ids, spine.served.filter((id) => spine.drawn.includes(id)));
  });
  test("a fund group the stylesheet has no hue for is still in the legend, last, and muted", async (t) => {
    const spine = await spineLegend();
    const seventh = await spineLegend((c) => withGroup(c, "fund-group/permanent", "Permanent Funds", true));
    t.diagnostic(seventh.ids.map((id, i) => id.replace("fund-group/", "") + " " + seventh.vars[i]).join(", "));
    assert.equal(seventh.ids.length, spine.ids.length + 1);
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

describe("a reduction read back off the chart on screen", () => {
  // markContra flips a reduction to draw it and keeps what it was: undone,
  // the chart is at its printed sign (the expansion walk below) and marks
  // again with its sentence.
  test("markContra can be undone, and marking again keeps every sentence", async () => {
    const app = (await loadApp()).app;
    for (const stem of ["fy2026-adopted", "fy2027-adopted"]) {
      const printed = fundFlows(app, stem);
      assert.ok(printed.links.some((l) => l.value_cents < 0), `${stem} prints no reduction, so nothing here is held`);
      const shown = app.markContra(printed);
      const back = unmarkContra(shown);
      assert.deepEqual(back.links.map((l) => l.value_cents), printed.links.map((l) => l.value_cents),
        `${stem}: a reduction read back off the screen is not at its printed sign`);
      const again = app.markContra(back);
      assert.deepEqual(again.links.map((l) => l.contra || ""), shown.links.map((l) => l.contra || ""),
        `${stem}: marking the chart a second time dropped a reduction's sentence`);
    }
  });
});

describe("a printed flow and an inferred one", () => {
  // Planted: two lines of one category into one fund fold into one ribbon at
  // the category grain, and one of them is marked inferred. A cross-tab
  // slice beside a flow is still refused, with the fold's own type.
  test("merge into one ribbon drawn inferred, and a cross-tab slice beside a flow is refused", async () => {
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
    const ribbon = () => foldAt(app, doc, [0, 2, 3, 4, 5]).links
      .find((l) => l.source === byID.get(planted.source).parent && l.target === planted.target && l.kind === planted.kind);
    assert.equal(ribbon().derived, false, "the printed legs fold to an inferred ribbon");
    planted.derived = true;
    assert.equal(ribbon().derived, true, "a merge holding an inferred leg is drawn printed");
    assert.ok(ribbon().fact_ids.includes(planted.fact_ids[0]), "the inferred leg's fact is cited away");
    planted.derived = false;
    planted.partition = true;
    assert.throws(() => foldAt(app, doc, [0, 2, 3, 4, 5]), /cannot draw as one, a cross-tab slice and a flow that is not one/);
    assert.throws(() => foldAt(app, doc, [0, 2, 3, 4, 5]), app.FoldRefusal, "the refusal is not the fold's own type");
  });

  test("a reduction's words do not name a ribbon it was only one leg of", async (t) => {
    const app = (await loadApp()).app;
    const doc = structuredClone(fundFlows(app, "fy2026-adopted"));
    const byID = new Map(doc.nodes.map((n) => [n.id, n]));
    const byKey = new Map();
    for (const l of doc.links) {
      const src = byID.get(l.source);
      if (!src || src.tier !== 1 || byID.get(l.target).tier !== 3 || l.contra) continue;
      const key = src.parent + "\u001f" + l.target + "\u001f" + l.kind;
      byKey.set(key, (byKey.get(key) || []).concat([l]));
    }
    const legs = [...byKey.values()].find((ls) => ls.length > 1);
    assert.ok(legs, "no two lines of one category reach one fund, so nothing folds together");
    const [first] = legs;
    const merged = () => foldAt(app, doc, [0, 2, 3, 4, 5]).links
      .find((l) => l.source === byID.get(first.source).parent && l.target === first.target && l.kind === first.kind);
    t.diagnostic(`${legs.length} lines of ${byID.get(first.source).parent} fold into one ribbon to ${first.target}`);
    const words = "a reduction every leg prints";
    for (const l of legs) l.contra = words;
    assert.equal(merged().contra, words, "legs that print the same reduction lost its words");
    legs[legs.length - 1].contra = "";
    assert.equal(merged().contra, "", "one leg's reduction words named the merged ribbon");
  });
});

/** Cents and cited fact ids over a set of links. */
function totals(links) {
  const ids = new Set();
  let cents = 0;
  for (const l of links) {
    cents += l.value_cents;
    for (const id of l.fact_ids) ids.add(id);
  }
  return { cents, cited: [...ids].sort() };
}

// THE CAP'S OWN PROPERTIES, over every chart of the expansion the page
// offers on the pinned columns: each is capped by expansionOf and held to the
// same frontier folded whole, the unfolded schedule's one fold. Nothing here
// compares a figure to anything Go computed.
describe("the expansion's caps preserve what they fold, at every split the page offers", () => {
  const CONFIG = pageFixture().config;
  const STEMS = publishedColumns();

  test("every published year's column is pinned, so the walk below reaches it", () => {
    for (const stem of STEMS) {
      assert.doesNotThrow(() => columnFixture(stem), `the page publishes ${stem} and testdata/ pins no column for it`);
    }
    assert.ok(STEMS.length > 0, "the pinned page publishes no year, so the walk below covers nothing");
  });

  for (const stem of STEMS) {
    test(`${stem}: every chart's sums, citations, caps and tail notes survive the cap`, async (t) => {
      const app = (await loadApp({ config: CONFIG })).app;
      const doc = app.scheduleOf(columnFixture(stem), "expansion");
      const chart = CONFIG.overview;
      const caps = chart.sankey.split_caps;
      const wrong = [];
      let charts = 0;
      let tails = 0;
      let merged = 0;
      for (const st of expansionStates(app, doc)) {
        charts++;
        const raw = app.foldDocument(doc, st.frontier);
        const drawn = unmarkContra(app.expansionOf(doc, st.frontier, chart, st.whole));
        // (a) SUMS, and (b) NO RIBBON LOST OR DUPLICATED: the same cents and
        // the same facts -- a ribbon dropped loses its facts, one drawn twice
        // doubles its cents.
        const want = totals(raw.links);
        const got = totals(drawn.links);
        if (want.cents !== got.cents) wrong.push(`${st.where}: ${want.cents} cents folded whole, ${got.cents} drawn`);
        if (want.cited.join() !== got.cited.join()) {
          const lost = want.cited.filter((f) => !got.cited.includes(f));
          const invented = got.cited.filter((f) => !want.cited.includes(f));
          wrong.push(`${st.where}: ${lost.length} fact(s) cited away, ${invented.length} cited from nowhere`);
        }
        merged += raw.links.length - drawn.links.length;
        // (c) CAPS: each open split a cap names draws at most cap members,
        // its derived remainder among them, and a tail, and where it folded,
        // its tail's members and the kept are the split, the tail holding no
        // derived node.
        const ids = new Set(drawn.nodes.map((n) => n.id));
        for (const sp of doc.splits) {
          const cap = caps.find((c) => c.split === sp.key);
          if (!cap || app.openSplitOf(doc, st.frontier, sp.node) !== sp.key) continue;
          const members = app.splitsOf(doc, sp.node).find((x) => x.key === sp.key).members.filter((m) => st.frontier.has(m));
          const tailID = app.splitTailID(sp.node, sp.key);
          const tail = drawn.nodes.find((n) => n.id === tailID);
          const kept = members.filter((m) => ids.has(m));
          const derived = new Set(doc.nodes.filter((n) => n.derived).map((n) => n.id));
          if (st.whole.size && tail) wrong.push(`${st.where}: ${tailID} is drawn with its split drawn out`);
          if (!st.whole.size && kept.length + (tail ? 1 : 0) > cap.cap + 1) {
            wrong.push(`${st.where}: ${sp.node} by ${sp.key} draws ${kept.length} members${tail ? " and a tail" : ""}, capped at ${cap.cap}`);
          }
          if (tail && tail.folds.some((m) => derived.has(m))) {
            wrong.push(`${st.where}: ${tailID} folds a derived node`);
          }
          if (!tail) {
            if (kept.length !== members.length) wrong.push(`${st.where}: ${sp.node} by ${sp.key} drops a member with no tail`);
            continue;
          }
          tails++;
          const stood = kept.concat(tail.folds).sort();
          if (stood.join() !== members.slice().sort().join()) {
            wrong.push(`${st.where}: ${sp.node} by ${sp.key} holds ${members.length} and the tail plus the kept stand for ${stood.length}`);
          }
          if (tail.folds.length < 2) wrong.push(`${st.where}: a tail of ${tail.folds.length}`);
          // THE NOTE SAYS THE SMALLEST PRINTED ROWS: no printed row kept is
          // smaller than one folded, sized as the cap ranks them. A kept
          // remainder may be smaller than either.
          const size = (id) => {
            let into = 0;
            let out = 0;
            for (const l of raw.links) {
              if (l.target === id) into += Math.abs(l.value_cents);
              if (l.source === id) out += Math.abs(l.value_cents);
            }
            return Math.max(into, out);
          };
          const smallestKept = Math.min(...kept.filter((m) => !derived.has(m)).map(size));
          const largestFolded = tail.folds.reduce((x, m) => (size(m) > size(x) ? m : x));
          if (size(largestFolded) > smallestKept) {
            wrong.push(`${st.where}: ${tailID} folds ${largestFolded}, larger than a printed row kept`);
          }
          // THE NOTE CARRIES THE FIGURE THE TAIL IS DRAWN AT, which only the
          // fold after the cap knows.
          const figure = app.say("aggregate_together", { figure: app.fmt(app.tailFigure(drawn, tailID)) });
          if (!tail.source_note.endsWith(figure)) wrong.push(`${st.where}: ${tailID}'s note does not end "${figure}"`);
          // ITS TOTAL IS THE SPLIT'S MEMBERS, remainder included, and less
          // what it folds is the cap, read off the note.
          if (members.length - tail.folds.length !== cap.cap) {
            wrong.push(`${st.where}: ${tailID} folds ${tail.folds.length} of ${members.length}, leaving other than the cap of ${cap.cap}`);
          }
          const stated = app.say("aggregate_note", { folded: tail.folds.length, total: members.length, cap: cap.cap });
          if (!tail.source_note.startsWith(stated)) wrong.push(`${st.where}: ${tailID}'s note reads "${tail.source_note}", not "${stated}"`);
        }
      }
      t.diagnostic(`${stem}: ${charts} charts, ${tails} folded tails, ${merged} ribbons merged by the cap`);
      assert.ok(charts > 0 && tails > 0 && merged > 0);
      assert.deepEqual(wrong.slice(0, 5), []);
    });
  }
});

// A TAIL FOLDING A LINE PRINTED AS A REDUCTION is drawn as the lines()
// fixture above pins it: netted where the merge takes the reduction, at its
// size and disclosed by contraNote where it does not. No pinned column
// reaches either case. This walk holds that, and goes red the year a
// reduction ranks below a cap, so whoever reads that red reads the tail's
// note against the laid mark rather than trusting the fixture.
describe("no capped tail of the expansion folds a line printed as a reduction", () => {
  const CONFIG = pageFixture().config;
  const STEMS = publishedColumns();

  /**
   * Every tail the page's caps fold at every split it offers, each with the
   * ribbons its members draw folded whole, at their printed signs, and
   * whether the split it folds held a reduction at all, folded or kept.
   */
  function tailsOf(app, doc) {
    const out = [];
    const reduces = (l) => Boolean(l.contra) || l.value_cents < 0;
    for (const st of expansionStates(app, doc).filter((x) => !x.whole.size)) {
      const raw = app.foldDocument(doc, st.frontier);
      const drawn = app.expansionOf(doc, st.frontier, CONFIG.overview, st.whole);
      for (const tail of drawn.nodes.filter((n) => app.isAggregate(n.id))) {
        const own = app.splitTailOf(tail.id);
        const members = new Set(tail.folds);
        const split = new Set(app.splitsOf(doc, own.node).find((x) => x.key === own.key).members);
        const ribbons = raw.links.filter((l) => members.has(l.source) || members.has(l.target));
        out.push({
          where: st.where, tail, members: [...members],
          reductions: ribbons.filter(reduces),
          ranked: raw.links.some((l) => reduces(l) && (split.has(l.source) || split.has(l.target))),
          folded: ribbons.reduce((n, l) => n + l.value_cents, 0),
        });
      }
    }
    return out;
  }

  for (const stem of STEMS) {
    test(`${stem}: every capped tail at every split folds no reduction`, async (t) => {
      const app = (await loadApp({ config: CONFIG })).app;
      const found = tailsOf(app, app.scheduleOf(columnFixture(stem), "expansion"));
      const folded = found.filter((f) => f.reductions.length)
        .map((f) => `${f.where}, ${f.tail.id}: ` + f.reductions.map((l) => `${l.source} -> ${l.target} ${l.value_cents}`).join(", "));
      const ranked = found.filter((f) => f.ranked).length;
      t.diagnostic(`${stem}: ${found.length} capped tails, ${found.reduce((n, f) => n + f.members.length, 0)} members folded, ` +
        `${ranked} of the tails' splits held a reduction, ${folded.length} tails folded one`);
      assert.ok(found.length > 0, "no cap folded anything, so this walk holds nothing");
      assert.deepEqual(folded, []);
    });
  }

  // PLANTED: the lowest-ranked member of one tail has its ribbons turned into
  // reductions, and the walk above must name it.
  test("the walk names a reduction the moment a cap folds one", async (t) => {
    const app = (await loadApp({ config: CONFIG })).app;
    const doc = structuredClone(app.scheduleOf(columnFixture("fy2026-adopted"), "expansion"));
    const before = tailsOf(app, doc).find((f) => f.where === "fund-group/special-revenue by funds");
    assert.ok(before, "the special-revenue group's split folds no tail");
    assert.equal(before.reductions.length, 0, "the unplanted tail folds a reduction, so the plant proves nothing");
    const member = before.members[before.members.length - 1];
    const ribbons = doc.links.filter((l) => l.source === member || l.target === member);
    assert.ok(ribbons.length > 0, `${member} has no ribbon to plant on`);
    for (const l of ribbons) {
      l.value_cents = -l.value_cents;
      l.contra = "printed as a reduction of Test";
    }
    const after = tailsOf(app, doc).find((f) => f.where === before.where && f.tail.id === before.tail.id);
    assert.ok(after && after.members.includes(member), `${member} is no longer folded once planted`);
    t.diagnostic(`planted on ${member}: the tail's note reads "${after.tail.source_note}" ` +
      `where its folded ribbons sum to ${after.folded} cents; unplanted, ${before.folded}`);
    assert.ok(after.reductions.length > 0, "the planted reduction was folded and not named");
    assert.ok(after.reductions.every((l) => l.source === member || l.target === member), "the walk named a reduction the plant did not make");
  });
});

// A HUB IS DRAWN BY REPEATING LEAVES, and the fold is what adds them: over
// every chart of the expansion the page offers on the pinned columns, each
// open split whose links to its members repeat leaves is drawn with ribbons
// between its node and its members (or their tail) that sum, in cents and in
// facts, to the leaves those links repeat -- each re-pointed to end at the
// node, or for a node that takes nothing in, to start at it, its legs then
// running from its members. A leg lost reads short, a leg drawn twice reads
// long, and a shut split draws no ribbon to or from a member.
describe("an open hub's ribbons are the leaves it repeats", () => {
  const CONFIG = pageFixture().config;

  for (const stem of publishedColumns()) {
    test(`${stem}: at every split the page offers, a hub draws its leaves once each`, async (t) => {
      const app = (await loadApp({ config: CONFIG })).app;
      const doc = app.scheduleOf(columnFixture(stem), "expansion");
      const wrong = [];
      let hubs = 0;
      let shut = 0;
      let toMembers = 0;
      let fromMembers = 0;
      for (const st of expansionStates(app, doc)) {
        const drawn = unmarkContra(app.expansionOf(doc, st.frontier, CONFIG.overview, st.whole));
        // A ribbon's cells are what the tally reads it as: all of it, once.
        for (const l of drawn.links) {
          const cells = app.cellsOf(l);
          const inCells = cells ? Object.values(cells).reduce((s, c) => s + c, 0) : l.value_cents;
          if (inCells !== l.value_cents) wrong.push(`${st.where}: ${l.source} -> ${l.target} carries ${l.value_cents} and its cells ${inCells}`);
        }
        for (const sp of doc.splits) {
          const members = new Set(app.splitsOf(doc, sp.node).find((x) => x.key === sp.key).members);
          const tail = app.splitTailID(sp.node, sp.key);
          const at = (id) => members.has(id) || id === tail;
          const ribbons = drawn.links.filter((l) => (l.source === sp.node && at(l.target)) || (l.target === sp.node && at(l.source)));
          if (app.openSplitOf(doc, st.frontier, sp.node) !== sp.key) {
            shut++;
            if (ribbons.length) wrong.push(`${st.where}: ${sp.node} by ${sp.key} is shut and draws ${ribbons.length} ribbon(s) to its members`);
            continue;
          }
          const legs = doc.links.filter((l) => l.repeats &&
            ((l.source === sp.node && members.has(l.target)) || (l.target === sp.node && members.has(l.source))));
          if (!legs.length) continue;
          hubs++;
          if (legs.some((l) => l.source === sp.node)) toMembers++;
          if (legs.some((l) => l.target === sp.node)) fromMembers++;
          const keys = new Set(legs.map((l) => l.repeats));
          const leaves = doc.links.filter((l) => keys.has(l.repeats) && !legs.includes(l) &&
            (l.target === sp.node || l.source === sp.node));
          if (leaves.length !== keys.size) wrong.push(`${st.where}: ${keys.size} leaves repeated and ${leaves.length} read`);
          const want = totals(leaves);
          const got = totals(ribbons);
          if (want.cents !== got.cents) wrong.push(`${st.where}: ${sp.node}'s ribbons to its members carry ${got.cents}, its leaves ${want.cents}`);
          if (want.cited.join() !== got.cited.join()) wrong.push(`${st.where}: ${sp.node}'s ribbons to its members cite other than its leaves`);
        }
      }
      t.diagnostic(`${stem}: ${hubs} open hubs held to their leaves, ${toMembers} by legs to their members and ` +
        `${fromMembers} by legs from them; ${shut} shut splits drawing no member`);
      assert.ok(toMembers > 0 && fromMembers > 0, "no open split repeats a leaf one way or the other, so this walk holds nothing there");
      assert.deepEqual(wrong.slice(0, 5), []);
    });
  }
});
