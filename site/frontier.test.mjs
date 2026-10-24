// frontier.test.mjs — the one fold, read over the expansion schedule: a
// frontier of node ids, opened and shut by the pure helpers core.js exports,
// held to the spine Go printed beside it in the same column.
//
// EVERY ASSERT IS OVER KEYS AND NUMBERS. A failing assert over a folded
// document formats the whole graph, which this host cannot hold.

import { before, describe, test } from "node:test";
import assert from "node:assert/strict";

// core.js itself and not core.js: core.js binds its own foldDocument to the
// page's tier set, and the frontier fold is core.js's.
import * as core from "./core.js";
import { columnFixture } from "./testlib.mjs";

// `inferred`: the spine's printed cells the root frontier draws inferred.
// None: where a remainder leg Go derives folds back into a printed cell, Go
// names the cell in `completes` on every leg of it, and the whole set is
// drawn printed. `completing`: the cells Go declares so, each drawn whole at
// the root and, once its fund group opens, its remainder a derived ribbon of
// its own at the cents named.
const COLUMNS = [
  { label: "FY 2025-26", stem: "fy2026-adopted", inferred: [], completing: [] },
  { label: "FY 2026-27", stem: "fy2027-adopted", inferred: [],
    completing: [{
      cell: "fund-group/internal-service -> expenditure/services-and-supplies internal_service",
      group: "fund-group/internal-service",
      remainder: "fund-group-only/internal-service > expenditure/services-and-supplies (internal_service) = 25000000 inferred",
    }] },
];

/** The expansion's fold: core.foldDocument as the expansion calls it. */
const fold = (doc, frontier) => core.foldDocument(doc, frontier);

const cell = (s, t, kind) => `${s} > ${t} (${kind})`;

/** A frontier as one comparable string list. */
const ids = (f) => [...f].sort();

/**
 * Expand all (core.expandAllFrom) with no `alt`; with one, every node opened
 * by that alternative where it offers it and by its primary split elsewhere,
 * a frontier the page offers no single gesture for.
 */
function expandAll(doc, alt) {
  if (!alt) return core.expandAllFrom(doc, core.rootFrontier(doc)).frontier;
  let f = core.rootFrontier(doc);
  for (let changed = true; changed;) {
    changed = false;
    for (const id of [...f]) {
      const splits = core.splitsOf(doc, id);
      if (!splits.length || core.openSplitOf(doc, f, id) !== "") continue;
      const pick = splits.find((sp) => sp.alternative && sp.key === alt) || splits.find((sp) => !sp.alternative);
      f = core.expand(doc, f, id, pick.key);
      changed = true;
    }
  }
  return f;
}

/**
 * What a folded document must keep of the document it folds, as lists of
 * strings: empty lists when it keeps everything.
 */
function audit(doc, spine, frontier) {
  const byID = new Map(doc.nodes.map((n) => [n.id, n]));
  const rootOf = (id) => {
    let at = byID.get(id);
    while (at.parent) at = byID.get(at.parent);
    return at.id;
  };
  const drawn = fold(doc, frontier);

  // THE SPINE'S CELLS, EVERY ONE AT ITS CENTS: each ribbon summed onto the
  // spine node it is drawn inside, ribbons inside one spine node aside.
  const want = new Map();
  for (const l of spine.links) {
    const k = cell(l.source, l.target, l.kind);
    want.set(k, (want.get(k) || 0) + l.value_cents);
  }
  const got = new Map();
  for (const l of drawn.links) {
    const [s, t] = [rootOf(l.source), rootOf(l.target)];
    if (s === t) continue;
    const k = cell(s, t, l.kind);
    got.set(k, (got.get(k) || 0) + l.value_cents);
  }
  const sums = [];
  for (const k of new Set([...want.keys(), ...got.keys()])) {
    if (want.get(k) !== got.get(k)) sums.push(`${k}: spine ${want.get(k)}, drawn ${got.get(k)}`);
  }

  // EVERY DRAWN NODE'S SUMS: what a node draws in and out is the links that
  // cross into and out of the part of the schedule it holds -- its subtree,
  // less the subtrees of the drawn nodes inside it -- under the alternatives
  // the frontier opens.
  const within = new Map([...frontier].map((id) => [id, core.withinNode(doc, id)]));
  const owner = new Map();
  for (const [id, inside] of within) {
    for (const x of inside) {
      const held = owner.get(x);
      if (!held || within.get(held).has(id)) owner.set(x, id);
    }
  }
  const linked = { in: new Map(), out: new Map() };
  const ribboned = { in: new Map(), out: new Map() };
  const add = (m, k, v) => m.set(k, (m.get(k) || 0) + v);
  for (const l of doc.links) {
    if (l.alternative && core.openSplitOf(doc, frontier, l.source) !== l.alternative) continue;
    const [s, t] = [owner.get(l.source), owner.get(l.target)];
    if (!s || !t) {
      sums.push(`${cell(l.source, l.target, l.kind)} is read and held by no drawn node`);
      continue;
    }
    if (s === t) continue;
    add(linked.out, s, l.value_cents);
    add(linked.in, t, l.value_cents);
  }
  for (const l of drawn.links) {
    add(ribboned.out, l.source, l.value_cents);
    add(ribboned.in, l.target, l.value_cents);
  }
  for (const side of ["in", "out"]) {
    for (const id of new Set([...linked[side].keys(), ...ribboned[side].keys()])) {
      const [w, g] = [linked[side].get(id) || 0, ribboned[side].get(id) || 0];
      if (w !== g) sums.push(`${id} ${side}: its links ${w}, drawn ${g}`);
    }
  }

  const ribbon = new Map(drawn.links.map((l) => [cell(l.source, l.target, l.kind), l]));
  const facts = [];
  for (const l of drawn.links) {
    const k = cell(l.source, l.target, l.kind);
    if (!l.fact_ids.length || !l.locators.length) facts.push(`${k} cites nothing`);
  }
  // NO FACT LOST: a link whose two ends are drawn, and whose alternative is
  // open, is cited whole by the ribbon between them.
  for (const l of doc.links) {
    if (l.source === l.target || !frontier.has(l.source) || !frontier.has(l.target)) continue;
    if (l.alternative && core.openSplitOf(doc, frontier, l.source) !== l.alternative) continue;
    const r = ribbon.get(cell(l.source, l.target, l.kind));
    const cited = new Set(r ? r.fact_ids : []);
    for (const f of l.fact_ids) if (!cited.has(f)) facts.push(`${f} of ${cell(l.source, l.target, l.kind)} cited by no ribbon`);
  }
  // NO FACT DUPLICATED: no fact is cited by more ribbons than links cite it.
  const links = new Map();
  for (const l of doc.links) for (const f of l.fact_ids) links.set(f, (links.get(f) || 0) + 1);
  const ribbons = new Map();
  for (const l of drawn.links) for (const f of l.fact_ids) ribbons.set(f, (ribbons.get(f) || 0) + 1);
  for (const [f, n] of ribbons) if (n > (links.get(f) || 0)) facts.push(`${f} cited by ${n} ribbons and ${links.get(f) || 0} links`);

  // NO NODE OF A SHUT ALTERNATIVE IS DRAWN, and every drawn node is one
  // the frontier holds.
  const nodes = [];
  for (const n of drawn.nodes) {
    if (!frontier.has(n.id)) nodes.push(`${n.id} is drawn and not in the frontier`);
    if (n.alternative && core.openSplitOf(doc, frontier, n.parent) !== n.alternative) {
      nodes.push(`${n.id} is drawn while ${n.parent}'s ${n.alternative} is shut`);
    }
  }
  return { drawn, sums, facts, nodes };
}

/** The facts any ribbon cites. */
const citedBy = (d) => new Set(d.links.flatMap((l) => l.fact_ids));

describe("the expansion schedule, folded to a frontier", () => {

  for (const col of COLUMNS) {
    describe(col.label, () => {
      let doc, spine;
      before(() => {
        const column = columnFixture(col.stem);
        doc = core.scheduleOf(column, "expansion");
        spine = core.scheduleOf(column, "sankey");
        assert.ok(doc && spine, `${col.stem} carries no expansion or no spine`);
      });

      // CELLS AND CENTS, NOT CITATIONS: a leaf cites a finer page than the
      // spine's group-grain cell, so the fact ids differ by design.
      test("the root frontier draws the spine's nodes, cells and cents exactly", (t) => {
        const root = core.rootFrontier(doc);
        const drawn = fold(doc, root);
        const mine = drawn.links.map((l) => `${cell(l.source, l.target, l.kind)} = ${l.value_cents}`).sort();
        const theirs = spine.links.map((l) => `${cell(l.source, l.target, l.kind)} = ${l.value_cents}`).sort();
        t.diagnostic(`root frontier: ${root.size} ids; drawn ${drawn.nodes.length} nodes and ${drawn.links.length} ribbons, ` +
          `citing ${citedBy(drawn).size} facts; the spine ${spine.nodes.length} nodes and ${spine.links.length} links`);
        assert.deepEqual(ids(drawn.nodes.map((n) => n.id)), ids(spine.nodes.map((n) => n.id)));
        assert.deepEqual(mine, theirs);
      });

      // A PRINTED CELL A REMAINDER FOLDS BACK INTO is drawn printed, because
      // its legs are exactly the set Go names it on: the declaration settles
      // it.
      test("the root frontier draws every cell printed where the spine prints it", (t) => {
        const root = core.rootFrontier(doc);
        const printed = new Map(spine.links.map((l) => [cell(l.source, l.target, l.kind), Boolean(l.derived)]));
        const differ = fold(doc, root).links.map((l) => [cell(l.source, l.target, l.kind), Boolean(l.derived)])
          .filter(([k, d]) => printed.get(k) !== d).map(([k, d]) => `${k}${d ? "" : " printed"}`);
        t.diagnostic(`${differ.length} cell(s) drawn inferred where the spine prints them: ${differ.join("; ") || "none"}`);
        assert.deepEqual(differ, col.inferred);
        const named = [...new Set(doc.links.filter((l) => l.completes).map((l) => l.completes))].sort();
        assert.deepEqual(named, col.completing.map((c) => c.cell), "the cells Go declares completing sets on");
      });

      // ANY PART OF A COMPLETING SET IS NOT THE CELL: with its fund group
      // open, the remainder is drawn as its own derived ribbon, and its
      // funds' legs printed.
      test("a completing set's remainder draws inferred once its fund group opens", (t) => {
        for (const c of col.completing) {
          const open = core.expand(doc, core.rootFrontier(doc), c.group, "funds");
          const drawn = fold(doc, open).links.filter((l) => l.completes === c.cell)
            .map((l) => `${cell(l.source, l.target, l.kind)} = ${l.value_cents}${l.derived ? " inferred" : ""}`);
          t.diagnostic(`${c.group} open: ${drawn.length} ribbons of ${c.cell}`);
          assert.deepEqual(drawn.filter((d) => d.endsWith(" inferred")), [c.remainder]);
        }
      });

      test("every split of every node keeps the spine's sums and its facts, and shuts back to where it opened", (t) => {
        const root = core.rootFrontier(doc);
        const rootFacts = citedBy(fold(doc, root));
        let checked = 0;
        for (const sp of doc.splits) {
          const from = core.reveal(doc, root, sp.node);
          const open = core.expand(doc, from, sp.node, sp.key);
          const at = `${sp.node} by ${sp.key}`;
          const { drawn, sums, facts, nodes } = audit(doc, spine, open);
          assert.deepEqual(sums, [], `${at}: sums`);
          assert.deepEqual(facts, [], `${at}: facts`);
          assert.deepEqual(nodes, [], `${at}: nodes`);
          const cited = citedBy(drawn);
          assert.deepEqual([...rootFacts].filter((f) => !cited.has(f)), [], `${at}: the spine's facts cited away`);
          const members = core.splitsOf(doc, sp.node).find((s) => s.key === sp.key).members;
          assert.ok(members.length > 0, `${at} has no members`);
          assert.ok(members.some((m) => drawn.nodes.some((n) => n.id === m)), `${at}: no member drawn`);
          assert.equal(core.openSplitOf(doc, open, sp.node), sp.key, at);
          // A split opened straight through its parent's shuts with it.
          const top = core.straightFrom(doc, sp.node);
          const before = top === sp.node ? from : core.reveal(doc, root, top);
          assert.deepEqual(ids(core.collapse(doc, open, top)), ids(before), `${at}: collapse does not invert expand`);
          checked++;
        }
        t.diagnostic(`${checked} splits opened one at a time`);
        assert.equal(checked, doc.splits.length);
        assert.ok(checked > 0);
      });

      test("everything expanded keeps the spine's sums and its facts, one alternative at a time", (t) => {
        const alts = [...new Set(doc.splits.filter((sp) => sp.alternative).map((sp) => sp.key))].sort();
        assert.ok(alts.length > 0, "no alternative split to read");
        for (const alt of ["", ...alts]) {
          const all = expandAll(doc, alt);
          const { drawn, sums, facts, nodes } = audit(doc, spine, all);
          const at = alt ? `everything, by ${alt} where offered` : "everything, by primary splits";
          t.diagnostic(`${at}: ${all.size} ids; drawn ${drawn.nodes.length} nodes and ${drawn.links.length} ribbons, ` +
            `citing ${citedBy(drawn).size} facts`);
          assert.deepEqual(sums, [], `${at}: sums`);
          assert.deepEqual(facts, [], `${at}: facts`);
          assert.deepEqual(nodes, [], `${at}: nodes`);
          const withAlt = drawn.nodes.filter((n) => n.alternative).length;
          if (alt) assert.ok(withAlt > 0, `${at}: no member of the alternative drawn`);
          else assert.equal(withAlt, 0, `${at}: a member of an alternative drawn`);
          let shut = all;
          for (const id of core.rootFrontier(doc)) shut = core.collapse(doc, shut, id);
          assert.deepEqual(ids(shut), ids(core.rootFrontier(doc)), `${at}: collapsing every root does not restore the spine`);
        }
      });

      test("a node opens one way at a time, and refuses a split it does not have", () => {
        const sp = doc.splits.find((s) => s.alternative);
        const primary = core.splitsOf(doc, sp.node).find((s) => !s.alternative);
        const root = core.rootFrontier(doc);
        const one = core.expand(doc, root, sp.node, primary.key);
        const other = core.expand(doc, one, sp.node, sp.key);
        assert.equal(core.openSplitOf(doc, other, sp.node), sp.key);
        assert.deepEqual(ids(other), ids(core.expand(doc, root, sp.node, sp.key)), "the first split is not shut");
        assert.throws(() => core.expand(doc, root, sp.node, "no-such-split"), /has no split no-such-split/);
        const both = new Set([...one, ...core.splitsOf(doc, sp.node).find((s) => s.key === sp.key).members]);
        assert.throws(() => core.openSplitOf(doc, both, sp.node), /two ways/);
      });
    });
  }
});

// PLANTED: no pinned column has a lone member that splits by an alternative
// of one member. P opens straight through to M by M's primary split; the
// reader switches M to its alternative, whose one member Y opens straight
// through. Mutation: let straightFrom climb through an alternative member and
// this goes red: folding Y climbs to P and folds everything.
describe("a fold climbs only through what expand opened straight through", () => {
  const node = (id, parent = "", alternative = "") => Object.assign({ id, tier: 0, parent, label: id, role: "r" }, alternative ? { alternative } : {});
  const split = (n, key, alternative = false) => ({ node: n, key, form: "extend", alternative });
  const doc = {
    projection: "synthetic",
    nodes: [node("P"), node("M", "P"), node("A", "M"), node("B", "M"), node("Y", "M", "alt"), node("Y1", "Y"), node("Y2", "Y")],
    links: [],
    splits: [split("P", "one"), split("M", "rows"), split("M", "alt", true), split("Y", "rows")],
  };

  test("folding the alternative's lone member folds its parent's alternative, not the parent's parent", (t) => {
    const opened = core.expand(doc, core.rootFrontier(doc), "P", "one");
    const switched = core.expand(doc, opened, "M", "alt");
    const top = core.straightFrom(doc, "Y");
    t.diagnostic(`P open straight through: [${ids(opened)}]; M by alt: [${ids(switched)}]; folding Y folds ${top}`);
    assert.deepEqual({ opened: ids(opened), switched: ids(switched), top, fromM: core.straightFrom(doc, "M"), fromA: core.straightFrom(doc, "A") },
      { opened: ["A", "B", "M", "P"], switched: ["M", "P", "Y", "Y1", "Y2"], top: "M", fromM: "P", fromA: "A" });
  });
});

// PLANTED: no pinned column declares an alternative before its node's
// primary split. M lists "alt" first; A is a member of M's primary split
// "rows", so revealing A's member opens M by "rows", the split A belongs to,
// and not the first M declares. revealChain is the chain app.js
// revealEntries walks too, so this is the page's reveal.
// Mutation: key a member with no alternative by its parent's first split
// and the chain names "alt", and reveal throws that M's alt does not draw A.
describe("revealing a node opens each ancestor by the split the node below belongs to", () => {
  const node = (id, parent = "", alternative = "") => Object.assign({ id, tier: 0, parent, label: id, role: "r" }, alternative ? { alternative } : {});
  const split = (n, key, alternative = false) => ({ node: n, key, form: "extend", alternative });
  const doc = {
    projection: "synthetic",
    nodes: [node("P"), node("M", "P"), node("A", "M"), node("B", "M"), node("Y", "M", "alt"), node("A1", "A"), node("A2", "A"),
      node("Z", "B")],
    links: [],
    splits: [split("P", "one"), split("M", "alt", true), split("M", "rows"), split("A", "more")],
  };

  test("an alternative declared first is not the split a primary member is revealed by", () => {
    assert.deepEqual(core.revealChain(doc, "A1"), [{ node: "P", key: "one" }, { node: "M", key: "rows" }, { node: "A", key: "more" }]);
    assert.deepEqual(core.revealChain(doc, "Y"), [{ node: "P", key: "one" }, { node: "M", key: "alt" }]);
    const open = core.reveal(doc, core.rootFrontier(doc), "A1");
    assert.ok(open.has("A1") && open.has("A2"), `revealing A1 draws [${ids(open)}]`);
  });

  // Mutation: let revealChain key such a parent "" and reveal returns a
  // frontier without Z, saying nothing.
  test("a node under a parent with no split it belongs to is refused, not left undrawn", () => {
    assert.throws(() => core.revealChain(doc, "Z"), /B declares no split Z belongs to/);
  });
});

// PLANTED: on the pinned columns one pass of Expand all opens everything,
// since each root's split exposes no node with a split of its own, and no
// node declares an alternative first. Here P's split exposes M and N, M
// splits again, and M declares its alternative before its primary split.
// Mutation: stop expandAllFrom after one pass and M stays shut; open each
// node by its first declared split and M opens by "alt".
describe("Expand all opens what a split exposes, until nothing is left shut", () => {
  const node = (id, parent = "", alternative = "") => Object.assign({ id, tier: 0, parent, label: id, role: "r" }, alternative ? { alternative } : {});
  const split = (n, key, alternative = false) => ({ node: n, key, form: "extend", alternative });
  const doc = {
    projection: "synthetic",
    nodes: [node("P"), node("M", "P"), node("N", "P"), node("A", "M"), node("B", "M"), node("C", "M", "alt")],
    links: [],
    splits: [split("P", "one"), split("M", "alt", true), split("M", "rows")],
  };

  test("a node a split exposes is split in turn, by its primary split", () => {
    const all = core.expandAllFrom(doc, core.rootFrontier(doc));
    assert.deepEqual(all.splits, [{ node: "P", key: "one" }, { node: "M", key: "rows" }]);
    assert.deepEqual(ids(all.frontier), ["A", "B", "M", "N", "P"]);
  });
});

// A completing set one fold breaks stays broken through the next: the
// capped expansion folds a folded document again, and there the ribbons
// left naming a set are all that set's count could see.
describe("a completing set a fold breaks", () => {
  const node = (id, tier, parent = "") => ({ id, tier, parent, label: id, role: "r" });
  const link = (id, source, value, extra = {}) => Object.assign(
    { source, target: "t", kind: "k", value_cents: value, fact_ids: [id], locators: [] }, extra);
  const doc = {
    projection: "synthetic",
    nodes: [node("g", 0), node("s1", 0, "g"), node("s2", 0, "g"), node("n", 0, "g"), node("s3", 0), node("t", 1)],
    links: [
      link("a", "s1", 100, { completes: "C" }),
      link("b", "s2", 200, { completes: "C" }),
      link("x", "n", 400),
      link("c", "s3", 800, { completes: "C", derived: true }),
    ],
  };

  test("its lone derived leg is drawn inferred, folded once or twice", () => {
    const once = fold(doc, ["g", "s3", "t"]);
    const twice = fold(once, once.nodes.map((n) => n.id));
    for (const [name, d] of [["once", once], ["twice", twice]]) {
      const c = d.links.find((l) => l.source === "s3");
      assert.ok(c !== undefined, name + ": no ribbon from s3");
      assert.equal(Boolean(c.derived), true, name + ": the derived leg of a broken set is drawn printed");
      assert.equal(c.completes, undefined, name + ": a broken set's ribbon still names it");
    }
  });

  test("a set every leg of which is a ribbon of its own keeps its name, and folds whole", () => {
    const once = fold(doc, ["s1", "s2", "n", "s3", "t"]);
    const named = once.links.filter((l) => l.completes === "C").map((l) => l.source).sort();
    assert.deepEqual(named, ["s1", "s2", "s3"]);
    assert.equal(Boolean(once.links.find((l) => l.source === "s3").derived), true,
      "a part of the set is drawn printed");
  });
});
