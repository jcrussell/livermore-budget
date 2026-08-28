// fold.mjs — the checks for site/app.js's fold, and for the claim that the
// drill-down could not be drawn without it.
//
// WHY THIS FILE EXISTS AT ALL. fund-flows.json was built, checked, published
// and rendered by no page for four days, and the reason was recorded in
// pkg/cmd/export/data.go as prose: its middle column is 61 nodes, and a Sankey
// laid out at CHART_HEIGHT 820 gives every one of them zero height. Prose is
// how the FIRST version of that sentence came to state the mechanism wrongly
// (it said the padding "needs 840px", and d3-sankey shrinks the padding to fit
// before it runs out of anything else). The measurements below are the same
// claims where a wrong one fails.
//
// The document laid out here is testdata/fund-flows.golden.json, which a Go
// test pins to what `fisc export` writes. See testdata/README.md for why that
// fixture is a capture rather than a derivation, and what the Go test buys.

import { loadApp, goldenGraph, goldenFundFlows } from "./harness.mjs";

/** The tiers the drill-down page draws: revenue source, fund group, division. */
const DRAWN = [0, 2, 4];

/**
 * An app whose page config asks for a tier set.
 *
 * RENDER_TIERS IS READ AT MODULE LOAD, from FISC_CONFIG, so a tier set cannot
 * be handed to an app after the fact — which is the point of it being config
 * rather than a constant, and means each set needs its own instance.
 */
function appDrawing(tiers) {
  return loadApp({
    config: {
      schema_version: 1,
      primary: "fund-flows",
      projections: { "fund-flows": "data/fund-flows.json" },
      render_tiers: tiers,
      years: [{
        year: 2026, label: "FY 2025-26", stem: "fund-flows",
        path: "data/fund-flows.json", basis: "adopted",
        hero: { label: "l", value: "v", note: "n", kind: "hero" },
        figures: [], caveats: [],
        counts: { facts: 280, nodes: 145, links: 175 },
      }],
      docs: {},
    },
  });
}

/**
 * A drill-down in miniature: nine nodes carrying every shape the fold has a
 * rule for.
 *
 * WRITTEN HERE RATHER THAN IN testdata/, so the fixture and the assertion read
 * on one screen. It is not a sample of anything — each node is present because
 * a clause of foldDocument is about it:
 *
 *   fund/100, fund/101   two funds of one group, so two links MERGE
 *   fund/640             a second group, so the merge is keyed and not global
 *   dept/fire            a tier the page draws, reached through a fund
 *   expenditure/fire/*   a tier it does not, folding onto its own parent: the
 *                        SELF-LOOP, which is the case the contract warns about
 *   fund-group/capital   a drawn tier that no folded link touches: the DROP
 */
function miniature() {
  const node = (id, tier, parent) => ({
    id, label: id, tier, parent, constraint_tier: "", role: "",
    derived: false, rationale: "", source_note: "",
  });
  const link = (source, target, cents, facts, kind = "external") => ({
    source, target, value_cents: cents, kind, transfer_id: "",
    fact_ids: facts, derived: false,
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
      node("dept/fire", 4, "fund/100"),
      node("expenditure/fire/wages", 5, "dept/fire"),
    ],
    links: [
      link("revenue/tax", "fund/100", 100, ["a"]),
      link("revenue/tax", "fund/101", 200, ["b"]),
      link("revenue/tax", "fund/640", 400, ["c"]),
      link("fund/100", "dept/fire", 90, ["d"]),
      link("dept/fire", "expenditure/fire/wages", 90, ["d"]),
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
    columns: new Map([...graph.nodes.reduce((m, n) =>
      m.set(n.depth, (m.get(n.depth) || 0) + 1), new Map())].sort()),
  };
}

/**
 * A layout that may not happen, as a value rather than as a thrown error.
 *
 * EVERY LAY-OUT IN THIS FILE GOES THROUGH THIS, and the reason is a mutation
 * test: reverting foldDocument's self-loop drop makes d3-sankey throw on a
 * circular link, and while these were computed eagerly that surfaced as "a
 * whole check module threw before producing any check" -- a red run, but one
 * that names the harness rather than the defect. Now the check about self-loops
 * is the check that fails.
 */
function attempt(fn) {
  try {
    return { ok: true, value: fn() };
  } catch (e) {
    return { ok: false, why: (e && e.message) || String(e) };
  }
}

export function checks() {
  const whole = loadApp();
  const drill = appDrawing(DRAWN);

  const folding = attempt(() => drill.foldDocument(miniature()));
  const mini = folding.ok ? folding.value : { nodes: [], links: [] };
  const byPair = new Map(mini.links.map((l) => [l.source + " -> " + l.target, l]));

  const raw = goldenFundFlows();
  const foldingReal = attempt(() => drill.foldDocument(raw));
  const folded = foldingReal.ok ? foldingReal.value : { nodes: [], links: [] };

  const unfolded = attempt(() => smallest(whole.layOut(raw)));
  const laid = attempt(() => drill.layOut(folded));
  const drawn = laid.ok ? smallest(laid.value) : null;
  const hairlines = laid.ok ? laid.value.links.filter((l) => l.width < 1).length : -1;
  const flat = unfolded.ok ? unfolded.value : null;

  // The spine, laid out by the function the page ships, against the same graph
  // laid out by layout.mjs's local rebuild of it.
  const spine = attempt(() => whole.layOut(goldenGraph()));

  return [
    {
      // THE CLAIM THE WHOLE EPIC RESTS ON. Not "the chart is cramped": every
      // node and every ribbon measures exactly zero, so a view added without
      // the fold publishes a blank chart with every other check green.
      name: "the drill-down cannot be drawn unfolded, and that is not a matter of degree",
      ok: Boolean(flat) && flat.node === 0 && flat.ribbon === 0,
      detail: flat
        ? `unfolded: ${raw.nodes.length} nodes over ${flat.columns.size} columns ` +
          `(${[...flat.columns.values()].join("/")}), smallest node ` +
          `${flat.node.toFixed(4)}px and smallest ribbon ${flat.ribbon.toFixed(4)}px`
        : `laying the unfolded document out threw: ${unfolded.why}`,
    },
    {
      // MEASURED HEIGHTS AND WIDTHS, NOT PADDING ARITHMETIC. The obvious check
      // is (n-1) * NODE_PADDING < the usable height, and it is worthless: it
      // holds at 308 < 796 for the tier set below AND at 602 < 796 for one that
      // puts 29 of 44 nodes under a pixel. What makes a chart drawable is the
      // size of its marks.
      name: "folded, every node has height and every ribbon has width",
      ok: Boolean(drawn) && drawn.node > 0 && drawn.ribbon > 0,
      detail: drawn
        ? `folded to tiers ${DRAWN.join("/")}: ${folded.nodes.length} nodes over ` +
          `${drawn.columns.size} columns (${[...drawn.columns.values()].join("/")}), ` +
          `smallest node ${drawn.node.toFixed(3)}px, smallest ribbon ` +
          `${drawn.ribbon.toFixed(3)}px`
        : `laying the folded document out threw: ${laid.why}`,
    },
    {
      // The honest half of the one above. Marks this small are floored by
      // render() -- Math.max(2, height) and Math.max(1, width - RIBBON_GAP) --
      // so they are drawn at a size that no longer encodes their value. The
      // page has to say so; this counts how many it has to say it about, and
      // fails if that number grows.
      name: "the count of marks too small to encode their value is pinned",
      ok: hairlines === 7,
      detail: `${hairlines} of ${folded.links.length} ribbons lay out under 1px ` +
              "and are drawn at 1px",
    },
    {
      name: "the fold merges two funds of one group onto one ribbon, keeping both facts",
      ok: (() => {
        const l = byPair.get("revenue/tax -> fund-group/general");
        return Boolean(l) && l.value_cents === 300 &&
               JSON.stringify(l.fact_ids) === JSON.stringify(["a", "b"]);
      })(),
      detail: byPair.has("revenue/tax -> fund-group/general")
        ? `${byPair.get("revenue/tax -> fund-group/general").value_cents} cents citing ` +
          byPair.get("revenue/tax -> fund-group/general").fact_ids.join("+")
        : "no revenue/tax -> fund-group/general link at all",
    },
    {
      // fund/100 -> dept/fire becomes fund-group/general -> dept/fire, and
      // dept/fire -> expenditure/fire/wages becomes dept/fire -> dept/fire and
      // goes. Four links in, three out.
      name: "a link whose ends fold together is dropped rather than drawn as a loop",
      ok: folding.ok && mini.links.length === 3 &&
          !mini.links.some((l) => l.source === l.target) &&
          byPair.has("fund-group/general -> dept/fire"),
      detail: mini.links.map((l) => l.source + "->" + l.target).join(", "),
    },
    {
      name: "a drawn tier no folded link touches is not drawn",
      ok: !mini.nodes.some((n) => n.id === "fund-group/capital") &&
          mini.nodes.length === 4,
      detail: mini.nodes.map((n) => n.id).join(", ") +
              " -- fund-group/capital is a tier this page draws and has no flow",
    },
    {
      // Without this, dept/fire still claims parent "fund/100", which the fold
      // removed, and fundGroupOf's walk stops there and returns "". Every
      // consumer of the hierarchy then silently gets nothing on the one
      // document the hierarchy was added for.
      name: "a folded node's parent names a node the folded document still carries",
      ok: mini.nodes.every((n) => !n.parent || mini.nodes.some((m) => m.id === n.parent)) &&
          (mini.nodes.find((n) => n.id === "dept/fire") || {}).parent === "fund-group/general",
      detail: mini.nodes.map((n) => n.id + "<-" + (n.parent || "root")).join(", "),
    },
    {
      // The whole justification for dropping 44 links: the fund-to-department
      // link that survives carries the same money AND the same facts, over
      // every cell including the printed zeros. If that ever stops being true
      // the page starts citing less than it draws.
      name: "the fold cites nothing away",
      ok: (() => {
        const before = cited(raw);
        const after = cited(folded);
        return before.size === after.size && [...before].every((id) => after.has(id));
      })(),
      detail: `${cited(raw).size} facts cited by ${raw.links.length} links before the fold, ` +
              `${cited(folded).size} by ${folded.links.length} after`,
    },
    {
      name: "a node the tier set cannot place stops the draw rather than vanishing from it",
      ok: (() => {
        const orphan = miniature();
        orphan.nodes.push({
          id: "stray", label: "stray", tier: 9, parent: "", constraint_tier: "",
          role: "", derived: false, rationale: "", source_note: "",
        });
        orphan.links.push({
          source: "revenue/tax", target: "stray", value_cents: 1, kind: "external",
          transfer_id: "", fact_ids: ["z"], derived: false,
        });
        try {
          drill.foldDocument(orphan);
          return false;
        } catch (e) {
          return String(e.message).includes("stray") && String(e.message).includes("tier 9");
        }
      })(),
      detail: "dropping it loses a column silently; keeping it leaves a node with no " +
              "column to be drawn in",
    },
    {
      // THE REIMPLEMENTATION IS NOW PINNED TO THE SHIPPED FUNCTION. layout.mjs
      // measures 195 crossings against its own rebuild of layOut, which stayed
      // green no matter what layOut did -- so this compares the two, and a
      // change to the shipped nodeAlign, nodeSort or extent that moved the
      // spine would now show up as a difference here.
      name: "the spine the page ships is the spine layout.mjs measures",
      ok: (() => {
        if (!spine.ok) return false;
        const graph = spine.value;
        const local = localLayout(whole);
        return graph.nodes.length === local.nodes.length &&
               graph.nodes.every((n, i) => n.id === local.nodes[i].id &&
                 Math.abs(n.x0 - local.nodes[i].x0) < 1e-9 &&
                 Math.abs(n.y0 - local.nodes[i].y0) < 1e-9 &&
                 Math.abs(n.y1 - local.nodes[i].y1) < 1e-9) &&
               graph.links.every((l, i) => Math.abs(l.width - local.links[i].width) < 1e-9 &&
                 Math.abs(l.y0 - local.links[i].y0) < 1e-9);
      })(),
      detail: spine.ok
        ? `${spine.value.nodes.length} nodes and ${spine.value.links.length} ribbons in ` +
          "the same places under layOut() and under layout.mjs's rebuild of it"
        : `laying the spine out threw: ${spine.why}`,
    },
    {
      // Every node of the spine carries parent: "", so fundGroupOf answers for
      // the node itself and the inheritance cannot move anything. Asserted
      // rather than assumed, because "provably additive" was the claim the
      // whole palette change was landed on.
      name: "the parent walk cannot move the spine, because the spine has no parents",
      ok: goldenGraph().nodes.every((n) => n.parent === "") &&
          goldenGraph().nodes.every((n) =>
            whole.fundGroupOf(n) === (whole.isFundGroup(n) ? n.id : "")),
      detail: `all ${goldenGraph().nodes.length} spine nodes are parentless, so ` +
              "fundGroupOf is isFundGroup by another name there",
    },
  ];
}

/**
 * layout.mjs's rebuild of layOut, duplicated here for the one check that
 * compares the two. Kept in step with that file by the check itself: if they
 * diverge, the comparison against the shipped function fails in one of them.
 */
function localLayout(app) {
  const doc = goldenGraph();
  const sankey = app.d3.sankey()
    .nodeId((d) => d.id)
    .nodeWidth(app.NODE_WIDTH)
    .nodePadding(app.NODE_PADDING)
    .nodeAlign(app.d3.sankeyJustify)
    .nodeSort((a, b) => app.nodeRank(a) - app.nodeRank(b) || b.value - a.value)
    .extent([[app.LABEL_GUTTER, 12],
             [app.CHART_WIDTH - app.LABEL_GUTTER, app.CHART_HEIGHT - 12]]);
  const graph = sankey({
    nodes: doc.nodes.map((n) => Object.assign({}, n)),
    links: doc.links.map((l) => Object.assign({}, l, { value: l.value_cents })),
  });
  app.restackLinks(graph);
  return graph;
}
