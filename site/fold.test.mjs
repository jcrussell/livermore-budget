// fold.test.mjs — the client's fold: capColumn and foldDocument, and the
// claim that the drill-down could not be drawn without them.
//
// The fold is the client's fitting step (AGENTS.md, "Go vets, JavaScript
// renders"): Go declares what MAY fold and the client decides how much fits.
// So what is tested here is the client's own function over Go's pinned
// documents and over a miniature that carries every shape the fold has a
// clause for. Nothing here compares a figure to Go's answer.

import { before, describe, test } from "node:test";
import assert from "node:assert/strict";

import {
  loadApp, bootedApp, opened, expandAll, settle, columnFixture, goldenGraph, pageFixture,
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
      // === and not assert.equal: d3 hands back -0 for a mark of no height.
      assert.ok(flat.node === 0, `smallest node ${flat.node}`);
      assert.ok(flat.ribbon === 0, `smallest ribbon ${flat.ribbon}`);
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
    test(`${col.label}: the fold cites nothing away`, (t) => {
      const raw = fundFlows(drill, col.stem);
      const folded = drill.foldDocument(raw);
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
      t.diagnostic(`${objects.length} object cells and ${depts.length} divisions resolve through parent`);
      assert.ok(objects.length > 0 && depts.length > 0);
      for (const n of objects) assert.equal(deep.fundGroupOf(n), "fund-group/general", n.id);
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
  // Whether a printed leg and an inferred one may meet in one merge is Go's
  // question, refused at the export by validateSteps (the views test named for
  // a cap under which a printed flow and an inferred one would merge). The
  // fold does not ask it again: the merged ribbon's flag is the first leg's.
  test("a merged ribbon keeps the first leg's provenance flag and sums every leg", () => {
    const mixed = miniature();
    mixed.links.push({ source: "revenue/tax", target: "fund/101", value_cents: 7, kind: "external", transfer_id: "", fact_ids: ["y"], locators: [{ doc_id: "d", pages: [1] }], derived: true });
    const l = drill.foldDocument(mixed).links.find((x) => x.source === "revenue/tax" && x.target === "fund-group/general");
    assert.ok(l);
    assert.equal(l.derived, false);
    assert.equal(l.value_cents, 307);
    assert.deepEqual([...l.fact_ids].sort(), ["a", "b", "y"]);
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
    if (listed) col.fund_groups.push({ id, slug: id.split("/")[1] });
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

// fisc-7477: the parent-chain arm node-hierarchy-well-formed keeps off every
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
