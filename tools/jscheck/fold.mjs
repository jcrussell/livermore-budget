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
// The documents laid out here are testdata/fund-flows.golden.json and
// testdata/fund-flows-2027.golden.json, each pinned by a Go test to what
// `fisc export` writes. See testdata/README.md for why those two fixtures are
// captures rather than derivations, and what the Go tests buy.

import {
  loadApp, goldenGraph, goldenFundFlows, goldenFundFlows2027, plannedFetch, settle, spineConfig,
} from "./harness.mjs";

/**
 * The two fund-flows columns the page reaches and what the fold measures on
 * each: how many marks the {0,2,4} fold lays out below the size render()
 * floors them at.
 *
 * BOTH COLUMNS, BECAUSE THEY FOLD DIFFERENTLY. The same 280 facts read down
 * two printed columns give 238 nodes in FY2025-26 and 237 in FY2026-27 --
 * fund/207 prints a dash in the second and is not a node there -- and the
 * fold's marks-too-small count moves with the money: 7 ribbons and 4 rects
 * in one year, 8 and 5 in the other. A pin taken over one column would have
 * stayed green while the other's chart changed underneath it (fisc-ko1j.6).
 * Measured 2026-09-12 through the shipped foldDocument and layOut.
 *
 * THE SUB-PIXEL COUNTS ARE A PROPERTY OF THE FOLD, NOT OF THE DOCUMENT'S
 * SIZE. At {0,2,4} a tier-1 revenue line folds onto its category, which is
 * drawn, so all 238 nodes and 251 links lay out as the same 40 nodes and 52
 * ribbons that 11 revenue categories and their funds alone would.
 */
const COLUMNS = [
  { label: "FY 2025-26", golden: goldenFundFlows, hairlines: 7, slivers: 4 },
  { label: "FY 2026-27", golden: goldenFundFlows2027, hairlines: 8, slivers: 5 },
];

/**
 * The tier set the fold's own CLAUSES are exercised at, which is not a view.
 *
 * NO VIEW DECLARES {0,2,4}. The site draws this document only by opening the
 * spine into it, and each rung filters to one node before it folds (see
 * drill.mjs). The set is kept because it is the only three-tier fold over the
 * WHOLE document -- it places every node without a filter -- and three tiers
 * is what several of the rules below need to be visible at all: a link whose
 * ends fold together, a drawn tier no folded link touches, and a re-pointed
 * parent all need a middle column to fold through.
 *
 * IT ALSO HAS A PROPERTY THE RUNGS DO NOT, deliberately: at {0,2,4} the fold
 * cites nothing away, because the fund-to-division link that survives carries
 * the same facts as the object rows that fold into it. A rung cites a slice,
 * since it filtered first -- the General Fund at {0,3,4} cites 135 of 280 --
 * and that is correct rather than a loss: the counts line says so (app.js's
 * paintCounts), and drill.mjs pins it.
 */
const DRAWN = [0, 2, 4];

/**
 * An app whose page config asks for a tier set.
 *
 * RENDER_TIERS IS READ AT MODULE LOAD, from FISC_CONFIG, so a tier set cannot
 * be handed to an app after the fact — which is the point of it being config
 * rather than a constant, and means each set needs its own instance.
 */
function appDrawing(tiers, fetch, extra) {
  return loadApp({
    fetch,
    config: Object.assign({
      schema_version: 1,
      primary: "fund-flows",
      projections: { "fund-flows": "data/fund-flows.json" },
      render_tiers: tiers,
      years: [{
        year: 2026, label: "FY 2025-26", stem: "fund-flows",
        path: "data/fund-flows.json", basis: "adopted",
        hero: { label: "l", value: "v", note: "n", kind: "hero" },
        figures: [], caveats: [],
        counts: { facts: 280, nodes: 238, links: 251 },
        chart_title: "Sankey diagram of the FY 2025-26 adopted budget",
      }],
      docs: {},
    }, extra || {}),
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
 *   dept/parks           retained, but its own fund group is the dropped one:
 *                        the re-pointed parent must WALK PAST it, not name it
 */
function miniature() {
  const node = (id, tier, parent) => ({
    id, label: id, tier, parent, constraint_tier: "", role: "",
    derived: false, rationale: "", source_note: "",
  });
  // locators default to one page of doc "d" so every synthetic link is shaped
  // like a published one; the locator cases below pass their own.
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
      // Paid by a fund of ANOTHER group, so dept/parks survives the fold while
      // fund-group/capital -- the group its own parent chain leads to -- takes
      // in nothing and is dropped.
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

/**
 * The spine opened for real -- main() fetches the committed document, lays it
 * out and repaints -- and the legend the repaint built, read back from the DOM.
 *
 * THE ONE PAGE WITH A LEGEND. Every opened view is legend-less by rule, which
 * drill.mjs pins, so the spine's overview is where the palette's order meets
 * the DOM and the only place buildLegend's output can be read.
 */
async function spineLegend() {
  const app = loadApp({ config: spineConfig(),
    fetch: plannedFetch({ "data/sankey.json": { doc: goldenGraph() } }) });
  await settle();
  const legend = app.dom.byId.get("legend");
  return legend ? legend.children.map((b) => b.dataset.node) : [];
}

export async function checks() {
  const legend = await spineLegend();
  const whole = loadApp();
  // THE SPINE AS index.html DECLARES IT, which `whole` is not: that one carries
  // no tier set, which is the "drawn whole" state the unfolded fund-flows
  // comparisons below need and index.html is no longer in.
  const spineApp = loadApp({ config: spineConfig() });
  const drill = appDrawing(DRAWN);

  const folding = attempt(() => drill.foldDocument(miniature()));
  const mini = folding.ok ? folding.value : { nodes: [], links: [] };
  const byPair = new Map(mini.links.map((l) => [l.source + " -> " + l.target, l]));

  // The spine, laid out by the function the page ships, against the same graph
  // laid out by layout.mjs's local rebuild of it.
  const spine = attempt(() => spineApp.layOut(goldenGraph()));

  const out = [];
  for (const col of COLUMNS) {
    const raw = col.golden();
    const foldingReal = attempt(() => drill.foldDocument(raw));
    const folded = foldingReal.ok ? foldingReal.value : { nodes: [], links: [] };

    const unfolded = attempt(() => smallest(whole.layOut(raw)));
    const laid = attempt(() => drill.layOut(folded));
    const drawn = laid.ok ? smallest(laid.value) : null;
    // BOTH FLOORS, NOT JUST THE RIBBON ONE. render() floors a ribbon at 1px
    // (Math.max(1, width - RIBBON_GAP)) and a node rect at 2px (Math.max(2, y1 -
    // y0)), and the first version of this pin counted only ribbons -- so four
    // node rects were being drawn at a size that does not encode their value,
    // under a check whose whole claim is that such marks "cannot grow unnoticed".
    const hairlines = laid.ok ? laid.value.links.filter((l) => l.width < 1).length : -1;
    const slivers = laid.ok ? laid.value.nodes.filter((n) => n.y1 - n.y0 < 2).length : -1;
    const flat = unfolded.ok ? unfolded.value : null;
    out.push(
    {
      // THE CLAIM THE WHOLE EPIC RESTS ON. Not "the chart is cramped": every
      // node and every ribbon measures exactly zero, so a view added without
      // the fold publishes a blank chart with every other check green.
      name: `${col.label}: the drill-down cannot be drawn unfolded, and that is not a matter of degree`,
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
      name: `${col.label}: folded, every node has height and every ribbon has width`,
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
      name: `${col.label}: the count of marks too small to encode their value is pinned`,
      ok: hairlines === col.hairlines && slivers === col.slivers,
      detail: `${hairlines} of ${folded.links.length} ribbons lay out under 1px and are ` +
              `drawn at 1px (want ${col.hairlines}); ${slivers} of ${folded.nodes.length} ` +
              `node rects lay out under 2px and are drawn at 2px (want ${col.slivers})`,
    },
    {
      // The whole justification for dropping 44 links: the fund-to-department
      // link that survives carries the same money AND the same facts, over
      // every cell including the printed zeros. If that ever stops being true
      // the page starts citing less than it draws.
      name: `${col.label}: the fold cites nothing away`,
      ok: (() => {
        const before = cited(raw);
        const after = cited(folded);
        return before.size === after.size && [...before].every((id) => after.has(id));
      })(),
      detail: `${cited(raw).size} facts cited by ${raw.links.length} links before the fold, ` +
              `${cited(folded).size} by ${folded.links.length} after`,
    },
    {
      // THE PALETTE WAS WHOLLY DEAD ON THIS DOCUMENT and this is what says it
      // is not any more. Unfolded, 0 of 251 links have a fund-group end, so
      // linkColor returned --muted for every ribbon and buildLegend rendered
      // six swatches over nodes with no flows to isolate. Folded, the fund
      // groups ARE the middle column: 52 of 52 links touch one.
      // THE INHERITANCE IS DORMANT AT THE TIER SET THE PAGE SHIPS, and this is
      // the check that keeps it honest rather than merely present. At {0,2,4}
      // every folded link already has a fund-group END, so linkColor and
      // nodeRank never reach the parent walk: reverting nodeRank to
      // FUND_ORDER.indexOf(other.id) leaves every other check in this tree
      // green. Measured, 2026-08-28.
      //
      // It is not dead code -- it is what the fold's re-pointed parents are
      // FOR, and the moment tier 5 is drawn every department-to-object link
      // depends on it -- so it is exercised here at the tier set that reaches
      // it. Without this check the whole of fisc-5miz.3 would be unfalsifiable.
      name: `${col.label}: a node inherits its fund group through parent, at a tier set that needs it`,
      ok: (() => {
        const deep = appDrawing([0, 2, 4, 5]);
        const doc = deep.foldDocument(col.golden());
        // layOut is what populates the index fundGroupOf walks.
        deep.layOut(doc);
        const objects = doc.nodes.filter((n) => n.id.startsWith("expenditure/"));
        const depts = doc.nodes.filter((n) => n.id.startsWith("dept/"));
        return objects.length === 44 && depts.length === 23 &&
               objects.every((n) => deep.fundGroupOf(n) === "fund-group/general") &&
               depts.every((n) => deep.fundGroupOf(n) === "fund-group/general");
      })(),
      detail: "all 44 object cells and all 23 divisions resolve to fund-group/general " +
              "through parent, which is the only thing that would colour or rank them",
    },
    );
  }

  out.push(
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
      // THE UNION IS WHY THE FLOW TABLE IS HONEST. buildTable renders the
      // FOLDED document, and foldDocument builds each merged ribbon with
      // Object.assign from its first leg -- so without an explicit union the
      // ribbon would carry that leg's locators and cite a strict subset of the
      // pages its figure was read from. A reader clicking "records p2" would
      // get a shard holding part of the number they were shown.
      name: "a folded ribbon's locators are the union of its legs', pages ascending",
      ok: (() => {
        const doc = miniature();
        // The two legs of revenue/tax -> fund-group/general, read off
        // different pages of one document, plus a second document on one of
        // them so the doc-level grouping is exercised too.
        doc.links[0].locators = [{ doc_id: "d", pages: [3] }];
        doc.links[1].locators = [{ doc_id: "d", pages: [1] }, { doc_id: "acfr", pages: [7] }];
        const folded = drill.foldDocument(doc);
        const l = folded.links.find((x) =>
          x.source === "revenue/tax" && x.target === "fund-group/general");
        return Boolean(l) && JSON.stringify(l.locators) === JSON.stringify([
          { doc_id: "acfr", pages: [7] },
          { doc_id: "d", pages: [1, 3] },
        ]);
      })(),
      detail: "documents ascending and pages ascending within each, which is the shape " +
              "internal/project publishes -- citations() reads a link's locators and " +
              "metadata.sources with the same code, so the two must not differ",
    },
    {
      // THE ONE THING THE FACT-ID UNION CANNOT SHOW. Two facts on one page are
      // two ids and one locator, so a fold that merely concatenated locators
      // would render the same page twice in the Source column and nothing
      // about fact_ids would look wrong.
      name: "two legs read off one page fold to ONE locator, not two",
      ok: (() => {
        const doc = miniature();
        doc.links[0].locators = [{ doc_id: "d", pages: [1] }];
        doc.links[1].locators = [{ doc_id: "d", pages: [1] }];
        const folded = drill.foldDocument(doc);
        const l = folded.links.find((x) =>
          x.source === "revenue/tax" && x.target === "fund-group/general");
        // Both facts survive; only the duplicated page collapses.
        return Boolean(l) &&
               JSON.stringify(l.locators) === JSON.stringify([{ doc_id: "d", pages: [1] }]) &&
               JSON.stringify(l.fact_ids) === JSON.stringify(["a", "b"]);
      })(),
      detail: "two ids, one page; the citation de-duplicates and the fact list does not",
    },
    {
      // fund/100 -> dept/fire becomes fund-group/general -> dept/fire, and
      // dept/fire -> expenditure/fire/wages becomes dept/fire -> dept/fire and
      // goes. Four links in, three out.
      name: "a link whose ends fold together is dropped rather than drawn as a loop",
      ok: folding.ok && mini.links.length === 4 &&
          !mini.links.some((l) => l.source === l.target) &&
          byPair.has("fund-group/general -> dept/fire"),
      detail: mini.links.map((l) => l.source + "->" + l.target).join(", "),
    },
    {
      name: "a drawn tier no folded link touches is not drawn",
      ok: !mini.nodes.some((n) => n.id === "fund-group/capital") &&
          mini.nodes.length === 5,
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
          (mini.nodes.find((n) => n.id === "dept/fire") || {}).parent === "fund-group/general" &&
          // THE DROPPED-ANCESTOR CASE. dept/parks folds to itself and survives,
          // but its own fund group takes in nothing and is dropped, so a
          // one-step re-point would name a node the folded document does not
          // carry -- the exact dead-inheritance failure the re-pointing exists
          // to prevent, reached from the other side.
          (mini.nodes.find((n) => n.id === "dept/parks") || {}).parent === "",
      detail: mini.nodes.map((n) => n.id + "<-" + (n.parent || "root")).join(", "),
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
          transfer_id: "", fact_ids: ["z"], locators: [{ doc_id: "d", pages: [1] }],
          derived: false,
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
      // PUBLISHED IS NOT DERIVED, and a merge is where the two could quietly
      // become one mark. OR-ing the flag draws the merged ribbon dashed and
      // lists its WHOLE amount under "what we inferred" -- a false statement
      // about a figure the city printed most of. Latent today (all four
      // published columns carry zero derived links) and refused rather than
      // left to the day one does.
      name: "a printed flow and an inferred one are not folded into one mark",
      ok: (() => {
        const mixed = miniature();
        // Same folded pair as revenue/tax -> fund-group/general, one leg
        // inferred: fund/101 is the general group's second fund.
        mixed.links.push({
          source: "revenue/tax", target: "fund/101", value_cents: 7, kind: "external",
          transfer_id: "", fact_ids: ["y"], locators: [{ doc_id: "d", pages: [1] }],
          derived: true,
        });
        try {
          drill.foldDocument(mixed);
          return false;
        } catch (e) {
          return String(e.message).includes("printed flow and an inferred one");
        }
      })(),
      detail: "the merged ribbon would be drawn dashed over an amount the city " +
              "printed most of, and listed whole under what we inferred",
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
        const local = localLayout(spineApp);
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
    {
      // READ OFF THE PAGE, NOT DERIVED FROM THE FOLD: main() drew the spine and
      // buildLegend built these buttons, one per fund group the chart touches,
      // so equality with FUND_ORDER says both that all six have flows and that
      // they are keyed in the palette's order.
      name: "the legend is the six fund groups, in the palette's order, and each has flows",
      ok: JSON.stringify(legend) === JSON.stringify(whole.FUND_ORDER),
      detail: legend.length
        ? legend.map((id) => id.replace("fund-group/", "")).join(", ")
        : "the legend is empty",
    },
  );
  return out;
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
    .nodeAlign(app.alignFor(app.RENDER_TIERS))
    .nodeSort((a, b) => app.nodeRank(a) - app.nodeRank(b) || b.value - a.value)
    .extent([[app.LABEL_GUTTER, 12],
             [app.chartWidth(app.drawnColumns()) - app.LABEL_GUTTER,
              app.CHART_HEIGHT - 12]]);
  const graph = sankey({
    nodes: doc.nodes.map((n) => Object.assign({}, n)),
    links: doc.links.map((l) => Object.assign({}, l, { value: l.value_cents })),
  });
  app.restackLinks(graph);
  return graph;
}
