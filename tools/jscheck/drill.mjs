// drill.mjs — the chart opening one node, measured rather than assumed.
//
// WHY THIS FILE EXISTS. The drill shipped with `go build`, `go test ./...`,
// `make js` and `fisc verify` all green and NOT ONE CHECK touching it: not the
// entry point a click calls, not filterToNode, not capColumn, not drillable,
// not paintBreadcrumb, and neither of the two tier sets the site actually
// declares. Grepping for the
// names is what turned that up, which is the cheapest way and the one that
// should not have been necessary.
//
// Two defects in the same range were reachable only by measurement: layOut
// aligned columns on RENDER_TIERS while a drilled document is
// folded to its step's tiers, so d3-sankey died inside its own ordering pass; and
// a {3,4} tier set could not draw at all, because the document's tier-0 revenue
// nodes have no ancestor at tier 3 or 4. Both are shapes a Go test cannot see
// and a reader meets on the first click.
//
// EVERY FIGURE HERE IS PINNED, NOT BOUNDED, for layout.mjs's reason: a bound
// that holds is not evidence a number is still the number, and these are the
// numbers pkg/cmd/export/data.go's comments quote to justify the tier sets and
// the caps.

import { readFileSync } from "node:fs";
import { join } from "node:path";

import {
  loadApp, goldenFundFlows, goldenGraph, plannedFetch, settle, refusals, twoYearConfig, repoRoot,
} from "./harness.mjs";

/**
 * The one chart page the site ships, verbatim from pkg/cmd/export/data.go's
 * views(): the spine, drawn whole, whose fund groups open into fund-flows and
 * whose divisions open into their object categories.
 *
 * COPIED RATHER THAN IMPORTED because there is no seam: views() is Go and this
 * is node. So the copy is a claim, and TestViewsOpensOnTheSpineAndGivesYears
 * ToItAlone is what keeps it honest from the other side -- it asserts these
 * exact steps off the real view list, field by field, descriptions included.
 * If you change either, both sides go red and that is the point.
 *
 * THE PER-YEAR JOIN IS NOT HERE. The Go side declares YearProjections and the
 * packager resolves them into each year's `steps` entries; the client reads
 * those and joins nothing. stepDocsFor below is what the packager ships for
 * one year, and the year check in year.mjs is where the join is measured.
 */
const PAGE = {
  steps: [
    {
      from: 2, projection: "fund-flows", tiers: [0, 3, 4],
      caps: [{ tier: 3, cap: 8 }, { tier: 4, cap: 24 }],
      back: "All fund groups", tail: "funds",
      description: "The revenue categories on the left flow into this fund group's own " +
        "funds, rescaled to the group's total — the citywide chart cannot show " +
        "them, because the General Fund alone is half the fund column and the " +
        "smallest fund is a thirty-thousandth of it. Only the General Fund continues " +
        "into the divisions that spend it: Budget Book pp.167-170 decompose that " +
        "fund alone, so every other group's money ends at its funds — not " +
        "missing, but not broken down in any published schedule.",
    },
    {
      from: 4, tiers: [4, 5], caps: [{ tier: 5, cap: 8 }],
      back: "All divisions", tail: "categories",
      description: "The division on the left flows into the object categories it " +
        "spends on, on the right — that division's cells of Budget Book " +
        "pp.167-170, rescaled to its total.",
    },
  ],
  // Measured: the spine's 58 links over 25 nodes cite 58 of its 120 facts;
  // the 62 it does not draw are the printed zeros and the stocks.
  overview: {
    nodes: 25, links: 58,
    counts: "58 flows between 25 nodes, from 58 of the document's 120 facts",
  },
  // The node whose opened view the fund cap is FOR. Its 32 funds are the
  // shape fisc-ppkq said rescaling would fix and measurement said it would
  // not.
  worst: "fund-group/special-revenue",
  // The node whose opened view the category cap is inert on: no division
  // spends on more than a handful of object categories. Pinned so that stops
  // being true loudly.
  inert: "dept/patrol",
  // HOW MANY VIEWS THE CHAIN OPENS: six fund groups, and the General Fund's
  // 23 divisions -- only that group draws a node at the second step's tier.
  openedViews: 29,
};

/** The caveat ids each committed golden carries, for the refs a year ships. */
const SPINE_CAVEATS = [
  "transfer-legs-unpaired", "internal-service-is-outside-the-external-headline",
  "working-capital-is-a-stock", "permanent-funds-have-no-column",
];
const FUND_FLOWS_CAVEATS = [
  "constraint-tier-is-our-reading", "mixed-grain-double-counts",
  "only-the-general-fund-is-decomposed",
];

/** Caveat refs the way the packager composes them: one anchor per (stem, id). */
function refsFor(stem, ids) {
  return ids.map((id) => ({ id, summary: "s", href: `caveats.html#caveat-${stem}--${id}` }));
}

/**
 * What the packager ships as one year's `steps`: the document each rung
 * draws, resolved for that year, with that document's caveat refs. The
 * same-document second step resolves to the first's document, so there is one
 * entry per declared step.
 */
function stepDocsFor(stem) {
  const entry = { stem, path: `data/${stem}.json`, caveats: refsFor(stem, FUND_FLOWS_CAVEATS) };
  return PAGE.steps.map(() => Object.assign({}, entry));
}

/**
 * The spine page carrying the chain, opened through main() over the two
 * committed documents -- `plan` overriding what any path answers, `tweak`
 * editing the config before app.js reads it.
 *
 * TWO YEARS, EACH WITH ITS OWN STEP DOCUMENTS. FY2026-27 answers with the same
 * goldens under its own paths, because only FY2025-26 has a committed golden
 * (fisc-ko1j.6); what the year arms measure is WHICH path a drill asks for,
 * not what comes back.
 *
 * THE SPINE'S LABEL FOR THE GROUP IS MADE DISTINCT, because both committed
 * documents print "General Fund" for fund-group/general and a rung named
 * from the wrong document would be invisible. The breadcrumb, the chart name
 * and the hint name the node the reader clicked in the words of the chart
 * they clicked it on -- the spine's -- and not the step document's.
 */
async function opened(plan, tweak) {
  const config = twoYearConfig();
  config.projections["fund-flows"] = "data/fund-flows.json";
  config.projections["fund-flows-2027"] = "data/fund-flows-2027.json";
  config.steps = PAGE.steps;
  config.years = config.years.map((y, i) => Object.assign({}, y, {
    counts: { facts: 120, nodes: 25, links: 58 },
    chart_title: `Sankey diagram of the ${y.label} adopted budget`,
    caveats: refsFor(y.stem, SPINE_CAVEATS),
    steps: stepDocsFor(i === 0 ? "fund-flows" : "fund-flows-2027"),
  }));
  if (tweak) tweak(config);
  const spine = goldenGraph();
  spine.nodes.find((n) => n.id === "fund-group/general").label = "General Fund group";
  const fetch = plannedFetch(Object.assign({
    "data/sankey.json": { doc: spine },
    "data/sankey-2027.json": { doc: spine },
    "data/fund-flows.json": { doc: goldenFundFlows() },
    "data/fund-flows-2027.json": { doc: goldenFundFlows() },
  }, plan || {}));
  const app = loadApp({ config, fetch });
  const body = app.dom.document.node();
  app.dom.document.getElementById("flow-table").selectable = { tbody: body };
  const main = app.dom.document.node();
  app.dom.document.plant("main", main);
  await settle();
  return { app, fetch, main, body };
}

/** What the DOM was told to show. */
function shown(app, body) {
  const el = (id) => app.dom.byId.get(id);
  const crumb = el("breadcrumb");
  return {
    counts: el("counts-line") ? el("counts-line").textContent : "",
    rows: body.children.length,
    crumbHidden: crumb ? crumb.getAttribute("hidden") !== null : true,
    crumbText: crumb ? crumb.children.map((c) => c.textContent).join(" | ") : "",
  };
}

/** The node on top of the stack, or "" on the overview. */
function topOf(app) {
  const stack = app.drilled;
  return stack.length ? stack[stack.length - 1].id : "";
}

/**
 * Opens one node through the real entry point and waits for the repaint.
 *
 * THE RETURN VALUE IS CHECKED AS WELL AS THE STACK. drillDown swallows its own
 * throw and leaves the stack as it was, so a truthiness test on the stack
 * passes from the second node onward and measure() silently records the chart
 * that was already there -- the defect the `=== id` guard in mustOpen was
 * written for, which a three-outcome return now states in words.
 */
async function openInto(app, id) {
  const outcome = await app.drillDown(id);
  await settle();
  return outcome;
}

/** openInto, or a thrown error naming what the chart was left on. */
async function mustOpen(app, id) {
  const outcome = await openInto(app, id);
  if (outcome !== "drew" || topOf(app) !== id) {
    throw new Error(`drillDown(${id}) ${outcome} and left the chart on ` +
      (topOf(app) || "the overview"));
  }
}

/** The smallest ribbon and how many lay out under a pixel. */
function measure(app, doc) {
  const laid = app.layOut(doc);
  const widths = laid.links.map((l) => l.width);
  return {
    links: laid.links.length,
    nodes: laid.nodes.length,
    smallest: Math.min(...widths),
    hairlines: widths.filter((w) => w < 1).length,
  };
}

/**
 * Every view the chain opens, each reached through the real entry points and
 * visited while it is on screen: the six fund groups at depth 1, and under
 * each, every node the depth-1 chart offers to open -- which is the General
 * Fund's 23 divisions and nothing under the other five.
 *
 * EVERY VIEW, NOT A SAMPLE, for the reason the two page loops this replaces
 * gave: one defect showed up on every drill and one on none of them, and a
 * sample catches the first and misses the second. The divisions are read off
 * the DRAWN depth-1 chart through drillable, so a division the chart draws
 * but does not offer to open is a view this walk does not know about -- and
 * PAGE.openedViews pins how many it found.
 *
 * Stops at the first refusal and returns it, so a caller can report which
 * node would not open rather than measuring the chart that was already there.
 * @param {any} app
 * @param {(where: string, depth: number) => Promise<void> | void} visit
 * @returns {Promise<{visited: number, refused: string}>}
 */
async function everyOpenedView(app, visit) {
  const groups = goldenGraph().nodes.filter((n) => n.tier === PAGE.steps[0].from);
  let visited = 0;
  try {
    for (const g of groups) {
      app.drillUp(0);
      await mustOpen(app, g.id);
      visited++;
      await visit(g.id, 1);
      const divisions = app.projection.nodes.filter((n) => app.drillable(n)).map((n) => n.id);
      for (const d of divisions) {
        app.drillUp(1);
        await mustOpen(app, d);
        visited++;
        await visit(`${g.id} > ${d}`, 2);
      }
    }
  } catch (e) {
    app.drillUp(0);
    return { visited, refused: e && e.message ? e.message : String(e) };
  }
  app.drillUp(0);
  return { visited, refused: "" };
}

/** A spine opened into one node, or into a node and then one beneath it. */
async function at(app, ...ids) {
  app.drillUp(0);
  for (const id of ids) await mustOpen(app, id);
}

export async function checks() {
  const out = [];
  const raw = goldenFundFlows();

  {
    const { app, body } = await opened();
    const before = shown(app, body);
    out.push({
      name: "the overview draws the spine whole, and its counts line describes it",
      ok: before.counts === PAGE.overview.counts && before.rows === PAGE.overview.links &&
          before.crumbHidden,
      detail: `counts "${before.counts}", ${before.rows} table rows, breadcrumb ` +
              (before.crumbHidden ? "hidden" : "SHOWING with nothing opened"),
    });

    const drawn = [];
    const walk = await everyOpenedView(app, (where, depth) => {
      drawn.push(Object.assign({ where, depth }, measure(app, app.projection)));
    });
    // THE WORST DIVISION IS PINNED BY NAME AND BY WIDTH: fisc-ko1j's own
    // measurement of depth 2 is Patrol at 51.38px, and PAGE.inert is that
    // node because it is the worst, not a division picked at random.
    const deep = drawn.filter((d) => d.depth === 2);
    const worstDeep = deep.length ? deep.reduce((a, b) => (b.smallest < a.smallest ? b : a)) : null;
    out.push({
      name: "every node the chain offers to open draws when opened, at both depths",
      ok: walk.refused === "" && walk.visited === PAGE.openedViews && drawn.length === walk.visited &&
          Boolean(worstDeep) && worstDeep.where.endsWith(" > " + PAGE.inert) &&
          worstDeep.smallest.toFixed(2) === "51.38",
      detail: walk.refused
        ? `after ${walk.visited} view(s), refused: ${walk.refused}`
        : `${walk.visited} views opened (want ${PAGE.openedViews}); smallest ribbon over all ` +
          `of them ${Math.min(...drawn.map((d) => d.smallest)).toFixed(3)}px; the narrowest ` +
          `depth-2 ribbon is ${worstDeep ? `${worstDeep.where} at ${worstDeep.smallest.toFixed(2)}px` : "nowhere"}`,
    });

    // THE CAP IS THE POINT OF THIS FILE, and it does not engage at both
    // depths. fisc-ppkq says rescaling to a group's own total is what makes
    // its funds legible; measured, it is not, and the cap is what is -- but
    // only where a column is wide enough to need one. special-revenue has 32
    // funds and the cap folds 24 of them; the widest division spends on two
    // object categories and the category cap never fires at all.
    //
    // BOTH FACTS ARE PINNED, not just the first. A check that asserted the cap
    // engages everywhere would fail at depth 2 for being right, and one that
    // asserted it nowhere would go quiet the day a division gains a ninth
    // category and the column starts folding without anyone deciding to.
    const uncappedSteps = PAGE.steps.map((s) => Object.assign({}, s, {
      caps: s.caps.map((c) => ({ tier: c.tier, cap: 1000 })),
    }));
    const { app: noCap } = await opened(null, (c) => { c.steps = uncappedSteps; });
    await at(app, PAGE.worst);
    const worst = measure(app, app.projection);
    await at(noCap, PAGE.worst);
    const worstUncapped = measure(noCap, noCap.projection);
    const engaged = worst.links < worstUncapped.links;
    out.push({
      name: "the fund cap is what makes the worst group's column drawable",
      // PINNED, NOT BOUNDED: 32 funds uncapped lay 22 of 49 ribbons under a
      // pixel, and capped at 8 they lay 2 of 22. These are the figures
      // pkg/cmd/export/data.go quotes for the cap.
      ok: engaged && worstUncapped.links === 49 && worstUncapped.hairlines === 22 &&
          worst.links === 22 && worst.hairlines === 2,
      detail: `${PAGE.worst} capped: ${worst.links} ribbons, ${worst.hairlines} under 1px; ` +
        `uncapped: ${worstUncapped.links} ribbons, ${worstUncapped.hairlines} under 1px; the cap ` +
        `${engaged ? "folded a tail" : "folded nothing"}`,
    });
    await at(app, "fund-group/general", PAGE.inert);
    const inert = measure(app, app.projection);
    await at(noCap, "fund-group/general", PAGE.inert);
    const inertUncapped = measure(noCap, noCap.projection);
    out.push({
      name: "the category cap is inert two rungs deep, because no division is wide enough to need it",
      ok: inert.links === inertUncapped.links && inert.hairlines === inertUncapped.hairlines &&
          inert.links > 0,
      detail: `${PAGE.inert} capped: ${inert.links} ribbons, ${inert.hairlines} under 1px; ` +
        `uncapped: ${inertUncapped.links} ribbons; smallest ribbon ${inert.smallest.toFixed(2)}px`,
    });
    app.drillUp(0);
    noCap.drillUp(0);
  }

  // THE CAP IS READ OFF THE TIER IT NAMES, NOT OFF ITS POSITION. The packager
  // ships a step's caps as a list in declaration order, and a step may cap a
  // coarse tier before its fine one. Two decoys, each refuting one wrong
  // reading: caps listed coarse-first must draw exactly the declared view, so
  // a client taking caps[0] folds the fund column at the wrong number and
  // fails here; and a step capping ONLY the coarse tier must draw its fund
  // column whole, so a client taking any cap it finds folds when nothing
  // asked it to. Measured: no group has more than ten revenue sources, so a
  // cap of 8 on tier 0 folds nothing there.
  {
    const step = PAGE.steps[0];
    const fund = step.caps.find((c) => c.tier === 3);
    const opensWorst = async (caps) => {
      const steps = [Object.assign({}, step, { caps }), PAGE.steps[1]];
      const { app } = await opened(null, (c) => { c.steps = steps; });
      await at(app, PAGE.worst);
      return measure(app, app.projection);
    };
    const asDeclared = await opensWorst(step.caps);
    const coarseFirst = await opensWorst([{ tier: 0, cap: 1000 }, ...step.caps]);
    const coarseOnly = await opensWorst([{ tier: 0, cap: fund.cap }]);
    const uncapped = await opensWorst([]);
    out.push({
      name: "a step's cap is looked up by the tier it names, not by its position",
      ok: coarseFirst.links === asDeclared.links &&
          coarseOnly.links === uncapped.links &&
          uncapped.links > asDeclared.links,
      detail: `${PAGE.worst} at the declared caps: ${asDeclared.links} ribbons; coarse tier ` +
        `listed first: ${coarseFirst.links}; only the coarse tier capped: ${coarseOnly.links}; ` +
        `no caps: ${uncapped.links}`,
    });
  }

  // EVERY MARK KNOWS ITS FUND GROUP, which is what colours it. Built from the
  // DRAWN nodes alone this returned "" for every node on every opened view --
  // filterToNode keeps only what the drawn tiers need, so a fund's fund-group
  // ancestor is absent and the walk stops at the first parent it cannot
  // resolve -- and across the document switch it returned "" again when layOut
  // merged the YEAR's hierarchy over the drawn nodes rather than the rung's.
  // A REVENUE SOURCE BELONGS TO NO FUND GROUP and correctly resolves to "" --
  // it is money arriving, not money held -- AND SO DOES A USE ON THE SPINE:
  // pp.66-67's "Wages & Benefits" is every group's at once, and nodeColor draws
  // it --muted on purpose. What must resolve is anything on the fund side of
  // the hierarchy, which is what carries a hue: a fund group, an aggregate the
  // cap made beneath one, or any node the FETCHED document places under a
  // parent. Read off the goldens' parent field rather than an id prefix,
  // because the spine's uses share the expenditure/ prefix with fund-flows'
  // object cells and belong to no group.
  {
    const { app } = await opened();
    const parentOf = new Map();
    for (const n of [...goldenGraph().nodes, ...raw.nodes]) parentOf.set(n.id, n.parent);
    const onTheFundSide = (/** @type {{id: string}} */ n) =>
      app.isFundGroup(n) || app.isAggregate(n.id) || Boolean(parentOf.get(n.id));
    const unresolved = (/** @type {{nodes: any[]}} */ d) => {
      app.layOut(d);
      return d.nodes.filter((n) => onTheFundSide(n) && app.fundGroupOf(n) === "");
    };
    const bad = unresolved(app.projection).map((n) => n.id);
    const walk = await everyOpenedView(app, (where) => {
      bad.push(...unresolved(app.projection).map((x) => where + " > " + x.id));
    });
    const groups = [...new Set(app.projection.nodes.map((n) => app.fundGroupOf(n)))]
      .filter(Boolean).sort();
    out.push({
      name: "every mark on the fund side knows its group, on the overview and in every opened view",
      ok: walk.refused === "" && bad.length === 0 && groups.length === 6,
      detail: bad.length
        ? `${bad.length} mark(s) resolve to no group: ${bad.slice(0, 4).join(", ")}`
        : `the overview draws ${JSON.stringify(groups)}, and all ${walk.visited} opened ` +
          "views resolve every fund-side mark",
    });
  }

  // THE AGGREGATE IS OURS AND SAYS SO. The city printed no line item called
  // "24 smaller funds", and this node shipped for one commit with
  // derived: false -- drawn solid rather than dashed, chipped "printed by the
  // city" in the tooltip and the detail panel, announced as printed in its
  // aria-label, and absent from "What we inferred", which is the list that
  // exists to be complete. That is the published-is-not-derived invariant
  // broken in output, and stated most plainly to the readers who cannot see the
  // mark.
  {
    const { app } = await opened();
    await at(app, PAGE.worst);
    const agg = app.projection.nodes.find((n) => n.id === app.aggregateID(3));
    out.push({
      name: "the capped tail is marked as ours, not as something the city printed",
      // THE COUNT IS ASSERTED AT TWO OR MORE, not just matched as digits.
      // capColumn engaged at cap + 1, so a column of 9 against a cap of 8 folded
      // ONE city-printed fund into a derived node labelled "1 smaller funds" --
      // and this regex accepted it. fund-group/enterprise has exactly 9.
      ok: Boolean(agg) && agg.derived === true && agg.rationale !== "" &&
          agg.source_note !== "" && /^(\d+) smaller funds$/.test(agg.label) &&
          Number(agg.label.split(" ")[0]) >= 2,
      detail: agg
        ? `"${agg.label}" derived=${agg.derived}, rationale ` +
          (agg.rationale ? `"${agg.rationale.slice(0, 48)}..."` : "MISSING") +
          (agg.source_note ? ", source note present" : ", SOURCE NOTE MISSING")
        : "no aggregate node: the cap folded nothing on the view it is needed for",
    });

    // NO AGGREGATE ANYWHERE COVERS FEWER THAN TWO. The check above looks at
    // one group; this looks at every opened view, because the shape that
    // shipped -- a column of exactly cap + 1 -- occurs on precisely one of the
    // 29 and would be invisible to a sample.
    const ones = [];
    const walk = await everyOpenedView(app, (where) => {
      for (const a of app.projection.nodes.filter((x) => app.isAggregate(x.id))) {
        if (Number(a.label.split(" ")[0]) < 2) ones.push(where + ": " + a.label);
      }
    });
    out.push({
      name: "no opened view folds a single printed figure into an aggregate of one",
      ok: walk.refused === "" && ones.length === 0,
      detail: ones.length
        ? ones.join("; ")
        : `every aggregate across all ${walk.visited} opened views covers two or more`,
    });
  }

  // A CAVEAT ABOUT A NODE REACHES THAT NODE, THROUGH THE FOLD AND ACROSS THE
  // DOCUMENT SWITCH. applies_to names ids in the FILE and a drawn mark is
  // often a fold of several of them, so a direct id match would leave the
  // badge silent on every folded view -- and a check asserting "no badge"
  // would pass whether the caveat does not apply or the resolution is broken.
  // At depth 0 the spine's own caveats mark its own nodes; at depth 1 the
  // DRAWN document is fund-flows, whose only-the-general-fund caveat names
  // fund/100, which the General Fund's view draws directly.
  //
  // MEASURED AND RECORDED, NOT ASSERTED: which of the other five groups' views
  // carry a mark. That caveat names each truncated GROUP, and an opened group
  // is gone from its own chart, so its funds inherit nothing -- the detail
  // line says how many depth-1 views carry a mark, and fisc-ko1j.5's residual
  // node is where the truncation becomes visible on those.
  {
    const { app } = await opened();
    app.layOut(app.projection);
    const marks = (/** @type {any} */ a) =>
      a.projection.nodes.filter((n) => a.caveatsFor(n.id).length > 0).map((n) => n.id);
    const spineMarks = marks(app);
    const perGroup = {};
    await everyOpenedView(app, (where, depth) => {
      if (depth !== 1) return;
      app.layOut(app.projection);
      perGroup[where] = marks(app);
    });
    const general = perGroup["fund-group/general"] || [];
    const markedGroups = Object.keys(perGroup).filter((g) => perGroup[g].length > 0);
    out.push({
      name: "a caveat about one node reaches that node at depth 0 and, over the other document, at depth 1",
      ok: spineMarks.includes("fund-group/internal-service") && spineMarks.includes("transfers/in") &&
          general.includes("fund/100"),
      detail: `spine marks ${JSON.stringify(spineMarks)}; opened into general ` +
        `${JSON.stringify(general)}; ${markedGroups.length} of ${Object.keys(perGroup).length} ` +
        `depth-1 views carry a mark (${markedGroups.map((g) => g.replace("fund-group/", "")).join(", ")})`,
    });
  }

  // THE BADGE HAS TO REACH THE READER, not merely be computable. caveatsFor
  // resolving correctly and showTip/pin never calling it are indistinguishable
  // from every other check here -- proved by stubbing caveatsFor out of both,
  // which left make js at 77 of 77. So these drive the two renderers and read
  // back what the DOM was told to show.
  //
  // THE LINK IS THE DRAWN DOCUMENT'S, AND ITS ANCHOR IS PINNED. At depth 1
  // caveatHref used to look the id up in the YEAR's refs, which are the
  // spine's, and returned "" -- the same "" a site with no caveats page
  // returns -- so the panel rendered the summary with no "Read it in full"
  // (fisc-ko1j.13). The packager now ships each rung's refs beside the year's,
  // and the anchor asserted here is fund-flows', not sankey's.
  for (const where of [
    { open: [], sharesColumn: true, stem: "sankey" },
    { open: ["fund-group/general"], sharesColumn: false, stem: "fund-flows" },
  ]) {
    const { app } = await opened();
    await at(app, ...where.open);
    const name = where.open.length ? "opened into " + where.open.join(" > ") : "the overview";
    app.layOut(app.projection);
    const marked = app.projection.nodes.find((n) => app.caveatsFor(n.id).length > 0);
    if (!marked) {
      out.push({
        name: `${name}: a marked node reaches the tooltip and the panel`,
        ok: false,
        detail: "no node at this depth carries a caveat, so this asserts nothing",
      });
      continue;
    }
    const laid = app.layOut(app.projection).nodes.find((n) => n.id === marked.id);
    const text = (/** @type {any} */ el) => {
      const parts = [];
      const walk = (/** @type {any} */ n) => {
        if (n.textContent) parts.push(n.textContent);
        for (const c of n.children || []) walk(c);
      };
      walk(el);
      return parts.join(" ");
    };
    app.showTip({ target: app.dom.byId.get("chart"), clientX: 0, clientY: 0 }, laid);
    const tip = text(app.dom.byId.get("tooltip"));
    app.pin(laid);
    const panel = text(app.dom.byId.get("detail"));
    const caveat = app.caveatsFor(marked.id)[0].id;
    const href = app.caveatHref(caveat);
    const wantHref = `caveats.html#caveat-${where.stem}--${caveat}`;
    out.push({
      name: `${name}: a marked node reaches the tooltip and the panel, and links to its own document's caveat`,
      // THE SHARE IS EXPECTED WHERE THE COLUMN DIVIDES AND NOWHERE ELSE, and
      // where it appears it carries its derived marking: a share is
      // arithmetic over two printed figures and sits beside "printed by the
      // city", which is the one adjacency this project's premise is about.
      // The spine's marked node is one of twelve sources; the General Fund's
      // is fund/100, alone in its column, where a share would read "our
      // 100.0% of this column" -- a derived chip carrying a figure that is
      // 100% by construction.
      ok: tip.includes("caveat") &&
          (where.sharesColumn
            ? tip.includes("◇ our ") && tip.includes("of this column")
            : !tip.includes("of this column")) &&
          panel.includes("Read it in full") && href === wantHref,
      detail: `${marked.id}: tooltip mentions ${tip.includes("caveat") ? "a caveat" : "NO caveat"} and ` +
              `${tip.includes("of this column")
                ? (tip.includes("◇ our ") ? "a share marked as ours" : "an UNMARKED share")
                : "no share, which is right for a column of one"}; panel ` +
              `${panel.includes("Read it in full") ? "links to the full text" : "does NOT link"}; ` +
              `href "${href}" (want "${wantHref}")`,
    });
  }

  // THE VALUE-FOLD ESCAPE HATCH IS DRIVEN, and it was not. capColumn folds a
  // column's tail by VALUE, and nothing in the parent chain records that -- so
  // caveatsFor's ancestor walk cannot see it and a caveat naming a swallowed
  // node would lose its badge silently. The `folds` field exists for exactly
  // that, and deleting it left make js at 78 of 78: the same
  // computed-but-never-driven shape the commit that added it reports fixing for
  // the renderers, committed in the same hunk.
  //
  // NO CAVEAT IN THE CORPUS NAMES A NODE THAT ENDS UP IN A TAIL, so this drives
  // it directly rather than through a caveat: it asks whether the aggregate
  // claims the nodes it removed, which is the property caveatsFor depends on.
  {
    const { app } = await opened();
    await at(app, PAGE.worst);
    const aggID = app.aggregateID(3);
    const agg = app.projection.nodes.find((n) => n.id === aggID);
    const drawn = new Set(app.projection.nodes.map((n) => n.id));
    const claimed = agg ? agg.folds : [];
    const stillDrawn = claimed.filter((id) => drawn.has(id));

    // THE PROPERTY ITSELF, driven rather than inferred: a caveat naming a
    // swallowed id must resolve to the aggregate. No caveat in the corpus names
    // one, so the check supplies its own and puts the document back -- which is
    // the only way to exercise a path the data does not currently reach, and
    // better than asserting the field's shape and calling it covered.
    const before = app.projection.metadata.caveats;
    app.projection.metadata.caveats = [{
      id: "probe", summary: "s", text: "t", applies_to: [claimed[0]],
    }];
    const found = app.caveatsFor(aggID).map((c) => c.id);
    app.projection.metadata.caveats = [{
      id: "probe", summary: "s", text: "t", applies_to: ["revenue/taxes/property"],
    }];
    const spurious = app.caveatsFor(aggID).map((c) => c.id);
    app.projection.metadata.caveats = before;
    app.drillUp(0);
    out.push({
      name: "the capped tail records what it swallowed, so a caveat naming one can find it",
      ok: Boolean(agg) && claimed.length >= 2 && stillDrawn.length === 0 &&
          found.length === 1 && spurious.length === 0,
      detail: agg
        ? `${claimed.length} id(s) folded in, ${stillDrawn.length} still drawn separately ` +
          `(want 0); a caveat naming ${claimed[0]} resolves to the aggregate ` +
          `${found.length === 1 ? "yes" : "NO"}, and one naming an unrelated node ` +
          `${spurious.length === 0 ? "does not" : "WRONGLY DOES"}`
        : "no aggregate on the view whose column the cap is for",
    });
  }

  // THE DESCENDANTS HALF, WHICH THE CORPUS CANNOT EXERCISE. capColumn removes
  // the tail AND anything parented beneath it, and records both in `folds`.
  // Only the first half is reachable through the shipped documents: fund/100 is
  // the sole tier-3 node with children and it is never in a tail, so deleting
  // the second half leaves every other check in this file green. Rather than
  // record that as a known gap, this hands capColumn a document that has the
  // shape -- which is what the harness is for.
  {
    const { app } = await opened();
    const node = (/** @type {string} */ id, /** @type {number} */ tier,
      /** @type {string} */ parent) =>
      ({ id, label: id, tier, parent, constraint_tier: "", role: "", derived: false,
        rationale: "", source_note: "" });
    const link = (/** @type {string} */ a, /** @type {string} */ b,
      /** @type {number} */ v) =>
      ({ source: a, target: b, value_cents: v, kind: "external", transfer_id: "",
        fact_ids: [], locators: [], derived: false });
    const doc = {
      projection: "probe",
      metadata: { sources: [], caveats: [] },
      nodes: [
        node("revenue/x", 0, ""),
        node("fund/1", 3, ""), node("fund/2", 3, ""), node("fund/3", 3, ""),
        node("dept/beneath-a-folded-fund", 4, "fund/3"),
      ],
      links: [
        link("revenue/x", "fund/1", 900), link("revenue/x", "fund/2", 90),
        link("revenue/x", "fund/3", 9), link("fund/3", "dept/beneath-a-folded-fund", 9),
      ],
    };
    const capped = app.capColumn(doc, 3, 1, "", "funds");
    const agg = capped.nodes.find((n) => n.id === app.aggregateID(3));
    const ids = capped.nodes.map((n) => n.id);
    out.push({
      name: "the capped tail records the descendants it removed, not only the tail itself",
      ok: Boolean(agg) &&
          agg.folds.indexOf("dept/beneath-a-folded-fund") >= 0 &&
          ids.indexOf("dept/beneath-a-folded-fund") < 0 &&
          !capped.links.some((l) => l.target === "dept/beneath-a-folded-fund"),
      detail: agg
        ? `aggregate claims ${JSON.stringify(agg.folds)}; the descendant is ` +
          `${ids.indexOf("dept/beneath-a-folded-fund") < 0 ? "removed" : "STILL DRAWN"} and ` +
          `its link is ${capped.links.some((l) => l.target === "dept/beneath-a-folded-fund")
            ? "STILL PRESENT" : "gone"}`
        : "capColumn folded nothing at cap 1 over three nodes",
    });
  }

  // NO SHARE EVER READS 100%. columnShare is suppressed for a column of one --
  // where 100% is true and says nothing -- and toFixed(1) can still round to it
  // on a column that IS divided: reproduced on committed data, where
  // fund-flows-2024-actual opened on debt-service puts transfers/in at 99.9943%
  // of a two-node column. A chip asserting a whole that the sibling beside it
  // denies is the shape the suppression exists to prevent, reached by
  // arithmetic instead of by topology.
  //
  // THE OVERVIEW AND EVERY OPENED VIEW, because the shape occurs on one column
  // of one year and a sample would miss it.
  {
    const hundreds = [];
    const { app } = await opened();
    const scan = (/** @type {string} */ where) => {
      for (const n of app.layOut(app.projection).nodes) {
        const share = app.columnShare(n);
        if (share.includes("100.0%") || share.includes("100%")) {
          hundreds.push(where + " " + n.id + ": " + share);
        }
      }
    };
    scan("overview");
    const walk = await everyOpenedView(app, (where) => scan(where));
    // AND THE CASE THE SHIPPED FIXTURE CANNOT REACH. The rounding happens on
    // fund-flows-2024-actual, opened on debt-service, and these checks fetch the
    // FY2025-26 golden -- the only fund-flows document committed. Removing the
    // ceiling left the scan above green for that reason alone, which is a check
    // passing because its fixture is the wrong year. So the split is built:
    // 99.9943% of a two-node column, the real proportion, laid out by the real
    // layOut.
    const near = {
      nodes: [
        { id: "a", label: "a", tier: 0, parent: "", constraint_tier: "", role: "",
          derived: false, rationale: "", source_note: "" },
        { id: "big", label: "big", tier: 2, parent: "", constraint_tier: "", role: "",
          derived: false, rationale: "", source_note: "" },
        { id: "tiny", label: "tiny", tier: 2, parent: "", constraint_tier: "", role: "",
          derived: false, rationale: "", source_note: "" },
      ],
      links: [
        { source: "a", target: "big", value_cents: 999943, kind: "external",
          transfer_id: "", fact_ids: [], locators: [], derived: false },
        { source: "a", target: "tiny", value_cents: 57, kind: "external",
          transfer_id: "", fact_ids: [], locators: [], derived: false },
      ],
    };
    const laid = app.layOut(near);
    const big = laid.nodes.find((n) => n.id === "big");
    const rounded = app.columnShare(big);
    out.push({
      name: "no share on any view claims 100% of a column that has more than one mark",
      ok: walk.refused === "" && hundreds.length === 0 && !rounded.includes("100"),
      detail: hundreds.length
        ? hundreds.slice(0, 3).join("; ")
        : `every share across the overview and all ${walk.visited} opened views is under 100%; ` +
          `a 99.9943% mark of a two-node column reads "${rounded}"`,
    });
  }

  // ---------------------------------------------------------------- the chain
  //
  // 0 -> 1 -> 2 -> 1 -> 0 through the real entry points, over the two
  // committed documents at once, each depth read back from the DOM: the
  // stack, the fetches, the counts line, the chart's name and description,
  // the breadcrumb, the hint, the legend, the flow table and where focus went.
  out.push(...(await walkChain()));

  // FIVE REFUSAL PATHS, EACH WITH ITS NEW CALLER. isDocument, understands,
  // drawableSankey and the fetch's own two failures had exactly one caller --
  // showYear -- and drillDown is the second. A click that reached a guard
  // showYear did not, or skipped one it did, would draw at depth 1 a file the
  // year control refuses at depth 0, and none of the year arms could tell.
  // Each arm here plans one failure for the step document and asserts the
  // drill FAILED, the reader was told in the words that name the fault, the
  // stack is still empty, and the spine's own sentence is still on screen.
  //
  // THE FIFTH IS THE JOIN'S OWN: a year the packager shipped with no step
  // entries. The client resolves nothing itself, so a year with no entry
  // refuses in words rather than falling back to the one file the stem maps
  // to -- which is the file the reader would have been shown under the wrong
  // year.
  for (const refusal of [
    { name: "a 404", plan: { "data/fund-flows.json": { ok: false, status: 404 } }, says: "HTTP 404" },
    { name: "a body that is not JSON", plan: { "data/fund-flows.json": { badBody: true } }, says: "not valid JSON" },
    {
      name: "a document at the wrong schema_version",
      plan: { "data/fund-flows.json": { doc: Object.assign({}, goldenFundFlows(), { schema_version: 2 }) } },
      says: "declares schema_version 2",
    },
    { name: "a null body", plan: { "data/fund-flows.json": { doc: null } }, says: "not a document at all" },
    {
      name: "a year packaged with no step document",
      tweak: (c) => { delete c.years[0].steps; },
      says: "the year on screen was not packaged with",
      unfetched: true,
    },
  ]) {
    const { app, fetch, main, body } = await opened(refusal.plan, refusal.tweak);
    const before = shown(app, body);
    const outcome = await openInto(app, "fund-group/general");
    const after = shown(app, body);
    const banners = refusals(main).map((b) => b.textContent);
    const asked = fetch.asked.includes("data/fund-flows.json");
    out.push({
      name: `the drill refuses ${refusal.name}, in words that name the fault`,
      ok: outcome === "failed" && app.drilled.length === 0 &&
          banners.length === 1 && banners[0].includes(refusal.says) &&
          after.counts === before.counts && after.crumbHidden &&
          asked === !refusal.unfetched,
      detail: `drillDown came to "${outcome}" with ${app.drilled.length} rung(s) on the stack; ` +
        `${banners.length} banner(s)${banners.length ? `, reading "${banners[0].slice(0, 90)}..."` : ""}; ` +
        `the step file was ${asked ? "" : "not "}asked for; the counts line still reads "${after.counts}"`,
    });
  }

  // BOTH CAPS ENGAGING AT ONCE, WHICH THE CORPUS CANNOT REACH. The chain caps
  // the fund column at 8 and the division column at 24, and the General Fund
  // has exactly 23 divisions -- so on every committed document one cap folds
  // and the other never does, and an aggregate id shared by every fold is
  // never seen colliding. It would have: two nodes with one id, and the fold
  // and d3-sankey both key by id. Same shape as the descendants arm above --
  // hand the code a document with the shape, rather than record a known gap.
  {
    const node = (id, tier, parent) =>
      ({ id, label: id, tier, parent, constraint_tier: "", role: "", derived: false,
        rationale: "", source_note: "" });
    const link = (a, b, v) =>
      ({ source: a, target: b, value_cents: v, kind: "external", transfer_id: "",
        fact_ids: [], locators: [], derived: false });
    const doc = {
      schema_version: 1, projection: "probe",
      metadata: { sources: [], caveats: [], counts: { facts: 8, nodes: 10, links: 8 } },
      nodes: [
        node("revenue/x", 0, ""), node("fund-group/g", 2, ""),
        node("fund/a", 3, "fund-group/g"), node("fund/b", 3, "fund-group/g"),
        node("fund/c", 3, "fund-group/g"), node("fund/d", 3, "fund-group/g"),
        node("dept/1", 4, "fund/a"), node("dept/2", 4, "fund/a"),
        node("dept/3", 4, "fund/a"), node("dept/4", 4, "fund/a"),
        node("expenditure/p", 5, "dept/1"), node("expenditure/q", 5, "dept/1"),
        node("expenditure/r", 5, "dept/1"),
      ],
      links: [
        link("revenue/x", "fund/a", 1000), link("revenue/x", "fund/b", 100),
        link("revenue/x", "fund/c", 10), link("revenue/x", "fund/d", 1),
        link("fund/a", "dept/1", 400), link("fund/a", "dept/2", 300),
        link("fund/a", "dept/3", 200), link("fund/a", "dept/4", 100),
        link("dept/1", "expenditure/p", 200), link("dept/1", "expenditure/q", 150),
        link("dept/1", "expenditure/r", 50),
      ],
    };
    // Four funds and four divisions against caps of 2: both columns exceed
    // cap + 1, so both fold two. The second step caps the category column at
    // 1 over three categories, so a cap engages TWO RUNGS DEEP as well -- the
    // aggregate there must be parented at dept/1, the rung it is inside, and
    // not at fund-group/g, the first. The committed corpus never caps at
    // depth 2 (no division spends on more than a handful of categories), so
    // "current rung, not first" is a claim only this document can test.
    const steps = [
      { from: 2, tiers: [0, 3, 4], caps: [{ tier: 3, cap: 2 }, { tier: 4, cap: 2 }], back: "Back", tail: "funds" },
      { from: 4, tiers: [4, 5], caps: [{ tier: 5, cap: 1 }], back: "Up", tail: "categories" },
    ];
    const app = loadApp({
      fetch: plannedFetch({ "data/probe.json": { doc } }),
      config: {
        schema_version: 1, primary: "probe", projections: { probe: "data/probe.json" },
        render_tiers: [0, 2], steps,
        years: [{
          year: 2026, label: "FY", stem: "probe", path: "data/probe.json", basis: "adopted",
          hero: { label: "l", value: "v", note: "n", kind: "hero" }, figures: [], caveats: [],
          counts: { facts: 8, nodes: 10, links: 8 }, chart_title: "t",
        }],
        docs: {},
      },
    });
    app.dom.document.getElementById("flow-table").selectable = { tbody: app.dom.document.node() };
    app.dom.document.plant("main", app.dom.document.node());
    await settle();
    const outcome = await openInto(app, "fund-group/g");
    const aggs = app.projection.nodes.filter((n) => app.isAggregate(n.id));
    const ids = [...new Set(aggs.map((a) => a.id))].sort();
    const tiers = aggs.map((a) => a.tier).sort();
    const parents = [...new Set(aggs.map((a) => a.parent))];
    const laid = outcome === "drew" ? app.layOut(app.projection) : null;
    const division = laid ? laid.nodes.find((n) => n.id === "dept/1") : null;
    const fineAgg = laid ? laid.nodes.find((n) => n.id === app.aggregateID(4)) : null;
    const divisionOpens = Boolean(division) && app.drillable(division);
    const aggOpens = Boolean(fineAgg) && app.drillable(fineAgg);
    out.push({
      name: "two caps on one step fold into two aggregates, one per tier, both parented at the opened node",
      ok: outcome === "drew" && aggs.length === 2 && ids.length === 2 &&
          tiers.join(",") === "3,4" && parents.length === 1 && parents[0] === "fund-group/g" &&
          divisionOpens && Boolean(fineAgg) && !aggOpens,
      detail: outcome === "drew"
        ? `${aggs.length} aggregate(s) with ids ${JSON.stringify(ids)} at tiers ${JSON.stringify(tiers)}, ` +
          `parent(s) ${JSON.stringify(parents)}; at depth 1 dept/1 ` +
          `${divisionOpens ? "opens" : "does NOT open"} and the division ` +
          `aggregate ${aggOpens ? "WRONGLY opens" : "does not"}`
        : `the probe could not be opened: ${outcome}`,
    });

    const outcome2 = outcome === "drew" ? await openInto(app, "dept/1") : "not attempted";
    const deep = app.projection.nodes.filter((n) => app.isAggregate(n.id));
    out.push({
      name: "a cap engaging two rungs deep parents its aggregate at the current rung, not the first",
      ok: outcome2 === "drew" && deep.length === 1 && deep[0].id === app.aggregateID(5) &&
          deep[0].parent === "dept/1" && app.drilled.length === 2,
      detail: outcome2 === "drew"
        ? `at depth 2 the category column folds into ${JSON.stringify(deep.map((a) => a.id))} ` +
          `with parent ${JSON.stringify(deep.map((a) => a.parent))}, want ["dept/1"]`
        : `opening dept/1 came to "${outcome2}"`,
    });
  }

  // THE LEGEND IS EMPTY ON AN OPENED VIEW BY RULE, and the rule needs a shape
  // that would draw a swatch without it. On every shipped step and on the
  // chain, filterToNode drops the fund-group node the moment it is opened, so
  // the legend is empty whether or not buildLegend decides anything -- which
  // makes the decision unfalsifiable on the corpus. A step opening a fund into
  // {2,4} keeps the fund's GROUP as the drawn ancestor of a folded fund, so
  // fund-group/general is on the chart at depth 1 and FUND_ORDER knows it: a
  // buildLegend that only read the drawn nodes would draw one swatch, whose
  // toggle isolates the only group on the chart.
  {
    const node = (id, tier, parent) =>
      ({ id, label: id, tier, parent, constraint_tier: "", role: "", derived: false,
        rationale: "", source_note: "" });
    const link = (a, b, v) =>
      ({ source: a, target: b, value_cents: v, kind: "external", transfer_id: "",
        fact_ids: [], locators: [], derived: false });
    const doc = {
      schema_version: 1, projection: "probe",
      metadata: { sources: [], caveats: [], counts: { facts: 3, nodes: 5, links: 3 } },
      nodes: [
        node("revenue/x", 0, ""), node("fund-group/general", 2, ""),
        node("fund/100", 3, "fund-group/general"),
        node("dept/1", 4, "fund/100"), node("dept/2", 4, "fund/100"),
      ],
      links: [
        link("revenue/x", "fund/100", 100),
        link("fund/100", "dept/1", 60), link("fund/100", "dept/2", 40),
      ],
    };
    const app = loadApp({
      fetch: plannedFetch({ "data/probe.json": { doc } }),
      config: {
        schema_version: 1, primary: "probe", projections: { probe: "data/probe.json" },
        render_tiers: [0, 2],
        steps: [{ from: 3, tiers: [2, 4], back: "Back", tail: "divisions" }],
        years: [{
          year: 2026, label: "FY", stem: "probe", path: "data/probe.json", basis: "adopted",
          hero: { label: "l", value: "v", note: "n", kind: "hero" }, figures: [], caveats: [],
          counts: { facts: 3, nodes: 5, links: 3 }, chart_title: "t",
        }],
        docs: {},
      },
    });
    app.dom.document.getElementById("flow-table").selectable = { tbody: app.dom.document.node() };
    app.dom.document.plant("main", app.dom.document.node());
    await settle();
    const legend = app.dom.document.getElementById("legend");
    const swatches0 = legend.children.length;
    const outcome = await openInto(app, "fund/100");
    const groupDrawn = app.projection.nodes.some((n) => n.id === "fund-group/general");
    const swatches1 = legend.children.length;
    out.push({
      name: "an opened view draws no legend even where a fund-group node survives the filter",
      ok: outcome === "drew" && swatches0 === 1 && groupDrawn && swatches1 === 0,
      detail: `overview ${swatches0} swatch(es); opened into fund/100 the group node is ` +
        `${groupDrawn ? "drawn" : "NOT drawn, so this asserts nothing"} and the legend holds ` +
        `${swatches1} -- one would be a key to a chart of one hue, toggling the only group on it`,
    });
  }

  return out;
}

/** The page's words at one depth, beyond what shown() reads. */
function words(app) {
  const el = (id) => app.dom.byId.get(id);
  const text = (id) => (el(id) ? String(el(id).textContent).replace(/\s+/g, " ").trim() : "");
  const crumb = el("breadcrumb");
  return {
    counts: text("counts-line"),
    title: text("chart-title"),
    desc: text("chart-desc"),
    hint: text("chart-hint"),
    legend: el("legend") ? el("legend").children.length : -1,
    crumbControls: crumb ? crumb.children.filter((c) => c.tagName === "button").map((c) => c.textContent) : [],
    crumbHere: crumb ? crumb.children.filter((c) => c.tagName === "span").map((c) => c.textContent).join("") : "",
    crumbHidden: crumb ? crumb.getAttribute("hidden") !== null : true,
    depth: app.drilled.length,
    drawnIsYears: app.drawnDoc() === app.fetched,
  };
}

/**
 * 0 -> 1 -> 2 -> 1 -> 0 over the chain, each depth read back from the DOM.
 *
 * THE SERVED DESCRIPTION IS THE TEMPLATE'S, planted so the table pointer
 * exists to be kept: the stub ships an empty <desc>, and an empty pointer
 * makes "ends with the pointer" true of any string. The pointer is taken as
 * everything after the template's first sentence, which coincides with
 * app.js's "last sentence" rule only because index.html.tmpl's <desc> is two
 * sentences long -- a different rule on purpose, so this is not the client's
 * split asserted against itself. That the pointer IS the last sentence is
 * the Go side's claim (TestAClosedFlowTableIsNotDescribedAsListedBelow).
 */
async function walkChain() {
  const out = [];
  const { app, fetch, body } = await opened();
  const desc = app.dom.document.getElementById("chart-desc");
  const served = templateDesc("index.html.tmpl", "");
  desc.textContent = served;
  const pointer = served.slice(served.indexOf(". ") + 2);
  const step0 = PAGE.steps[0];
  const step1 = PAGE.steps[1];

  const at0 = words(app);
  const asked0 = fetch.asked.slice();
  // getElementById AND NOT byId.get: render() reaches the chart through d3's
  // querySelector, so until focusInChart asks for it by id the stub has never
  // handed it out and byId has no entry to put focus on.
  const withFocus = () => {
    app.dom.document.activeElement = app.dom.document.getElementById("chart");
  };

  withFocus();
  const open1 = await openInto(app, "fund-group/general");
  const at1 = words(app);
  const rows1 = body.children.length;
  const focus1 = app.dom.focused ? app.dom.focused.textContent : "";
  const asked1 = fetch.asked.slice();
  const m1 = open1 === "drew" ? measure(app, app.projection) : null;
  // ASKED AT DEPTH 1, not later: drillable reads the stack, and by the time
  // `ok` is evaluated the walk is back on the overview.
  const patrol = app.projection.nodes.find((n) => n.id === "dept/patrol");
  const fund100 = app.projection.nodes.find((n) => n.id === "fund/100");
  const patrolOpens = Boolean(patrol) && app.drillable(patrol);
  const fund100Opens = Boolean(fund100) && app.drillable(fund100);
  const muted1 = app.projection.nodes
    .filter((n) => !n.id.startsWith("revenue/") && app.fundGroupOf(n) === "").map((n) => n.id);
  // THE DIVISION CAP IS INERT ON THE CORPUS AND PINNED INERT: 23 divisions
  // under a cap of 24, so no aggregate at tier 4 -- the day a 24th division
  // appears the column starts folding, and this is what says so.
  const divisions1 = app.projection.nodes.filter((n) => n.id.startsWith("dept/")).length;
  const foldedDivisions1 = app.projection.nodes.some((n) => n.id === app.aggregateID(4));
  const cited1 = new Set();
  for (const l of app.projection.links) for (const id of l.fact_ids) cited1.add(id);

  withFocus();
  const open2 = await openInto(app, "dept/patrol");
  const at2 = words(app);
  const focus2 = app.dom.focused ? app.dom.focused.textContent : "";
  const asked2 = fetch.asked.slice();
  const m2 = open2 === "drew" ? measure(app, app.projection) : null;
  const anyOpens2 = app.projection.nodes.some((n) => app.drillable(n));

  // ESCAPE, ONE RUNG AT A TIME, through the handler main() attached. The
  // breadcrumb's controls call drillUp(k) directly; Escape is the other route
  // and the one whose "innermost first" rule the bead names.
  const escape = () => {
    for (const fn of app.dom.documentListeners.keydown || []) fn({ key: "Escape" });
  };
  withFocus();
  escape();
  const back1 = words(app);
  const focusBack1 = app.dom.focused ? app.dom.focused.textContent : "";
  escape();
  const back0 = words(app);
  const asked3 = fetch.asked.slice();

  // AND THE CACHE: opening again fetches nothing. Then the breadcrumb's own
  // controls, each closing TO ITS OWN DEPTH: from depth 2 the inner control
  // lands on depth 1 and the outer one on the overview -- a bar whose every
  // control went to the overview would pass the Escape arm above unnoticed.
  await openInto(app, "fund-group/general");
  await openInto(app, "dept/patrol");
  const asked4 = fetch.asked.slice();
  const click = (control) => { for (const fn of (control && control.listeners.click) || []) fn({}); };
  const controlsAt = () =>
    app.dom.byId.get("breadcrumb").children.filter((c) => c.tagName === "button");
  click(controlsAt()[1]);
  const inner = words(app);
  await openInto(app, "dept/patrol");
  click(controlsAt()[0]);
  const back0b = words(app);

  // A GROUP WITH NO DIVISIONS SAYS SO. Five of the six groups draw nothing at
  // the second step's tier, so the depth-1 hint over them must say nothing
  // opens further rather than point at a column that is not there.
  await openInto(app, "fund-group/capital");
  const capital = words(app);
  const capitalOpens = app.projection.nodes.some((n) => app.drillable(n));
  app.drillUp(0);

  out.push({
    name: "chain: the step document is fetched on the first drill and not before",
    ok: asked0.length === 1 && !asked0.includes("data/fund-flows.json") &&
        asked1.length === 2 && asked1[1] === "data/fund-flows.json",
    detail: `main() asked for ${JSON.stringify(asked0)}; the first drill added ` +
      `${JSON.stringify(asked1.slice(asked0.length))}`,
  });
  out.push({
    name: "chain: the overview's hint names the column that opens, which is the spine's middle one",
    // "MIDDLE", READ OFF THE CHART. The spine draws tiers 0, 2 and 5 and its
    // fund groups are tier 2; a hint saying "right-hand column" here would
    // send the reader to the uses, which do not open.
    ok: at0.hint === "Click a node in the middle column to open it into its parts, or tab to " +
        "one and press Enter. A fund swatch follows one group's money without opening anything." &&
        at0.legend === 6 && at0.desc === served,
    detail: `hint "${at0.hint}"; legend ${at0.legend} swatches`,
  });
  out.push({
    name: "chain: depth 1 draws the General Fund at {0,3,4} from the other document, and every sentence says so",
    // 34 nodes, 33 links and 2 sub-pixel ribbons is fisc-ko1j's own
    // measurement of this view, reproduced here through the shipped functions.
    ok: open1 === "drew" && at1.depth === 1 && !at1.drawnIsYears &&
        Boolean(m1) && m1.nodes === 34 && m1.links === 33 && m1.hairlines === 2 &&
        divisions1 === 23 && !foldedDivisions1 &&
        at1.counts === `33 flows between 34 nodes, from ${cited1.size} of the document's 280 facts` &&
        rows1 === 33 &&
        at1.title === "Sankey diagram of the FY 2025-26 adopted budget, opened into General Fund group" &&
        at1.crumbControls.join("|") === "← All fund groups" && at1.crumbHere === "General Fund group" &&
        at1.hint === "This is General Fund group, broken into its parts. Click a node in the " +
          "right-hand column to open it further, or tab to one and press Enter." &&
        at1.legend === 0 &&
        at1.desc === "Opened into General Fund group. " + step0.description +
          " Use the breadcrumb above the chart, or press Escape, to go back. " + pointer &&
        patrolOpens && Boolean(fund100) && !fund100Opens && muted1.length === 0 &&
        focus1 === "← All fund groups",
    detail: open1 === "drew"
      ? `${m1.nodes} nodes, ${m1.links} links, ${m1.hairlines} under 1px; counts "${at1.counts}"; ` +
        `title "${at1.title}"; breadcrumb ${JSON.stringify(at1.crumbControls)} + "${at1.crumbHere}"; ` +
        `hint "${at1.hint}"; legend ${at0.legend} -> ${at1.legend} swatches; desc ` +
        `${at1.desc.startsWith("Opened into General Fund group. " + step0.description) ? "carries" : "LACKS"} ` +
        `the step's description and ${at1.desc.endsWith(pointer) ? "keeps" : "DROPS"} the table pointer; ` +
        `${divisions1} divisions drawn${foldedDivisions1 ? " AND a tier-4 aggregate" : ", none folded"}; ` +
        `a division ${patrolOpens ? "opens" : "does NOT open"} and a fund ` +
        `${fund100Opens ? "WRONGLY opens" : "does not"}; ${muted1.length} fund-side mark(s) ` +
        `resolve to no group${muted1.length ? " (" + muted1.slice(0, 3).join(", ") + ")" : ""}; ` +
        `focus on "${focus1}"`
      : `opening the General Fund came to "${open1}"`,
  });
  out.push({
    name: "chain: depth 2 draws Patrol at {4,5}, names both rungs, keeps the table pointer, and opens nothing further",
    ok: open2 === "drew" && at2.depth === 2 && Boolean(m2) && m2.links > 0 &&
        asked2.length === asked1.length &&
        at2.title.endsWith(", opened into General Fund group, then Patrol") &&
        at2.crumbControls.join("|") === "← All fund groups|← All divisions" &&
        at2.crumbHere === "Patrol" &&
        at2.hint === "This is Patrol, broken into its parts. Nothing here opens further; go back to open another." &&
        at2.desc === "Opened into General Fund group, then Patrol. " + step1.description +
          " Use the breadcrumb above the chart, or press Escape, to go back. " + pointer &&
        !anyOpens2 && at2.legend === 0 &&
        focus2 === "← All divisions",
    detail: open2 === "drew"
      ? `${m2.nodes} nodes, ${m2.links} links, smallest ribbon ${m2.smallest.toFixed(2)}px; title ` +
        `"${at2.title}"; breadcrumb ${JSON.stringify(at2.crumbControls)} + "${at2.crumbHere}"; ` +
        `hint "${at2.hint}"; desc "${at2.desc.slice(0, 60)}..."; ` +
        `${anyOpens2 ? "SOMETHING still opens" : "nothing opens"}; no second fetch; focus on "${focus2}"`
      : `opening Patrol came to "${open2}"`,
  });
  out.push({
    name: "chain: Escape closes one rung at a time, and each depth comes back as it was",
    ok: back1.depth === 1 && back1.counts === at1.counts && back1.title === at1.title &&
        back1.crumbControls.join("|") === at1.crumbControls.join("|") &&
        back1.hint === at1.hint && back1.desc === at1.desc &&
        focusBack1 === "← All fund groups" &&
        back0.depth === 0 && back0.counts === at0.counts && back0.title === at0.title &&
        back0.crumbHidden && back0.legend === 6 && back0.hint === at0.hint &&
        back0.desc === served && back0.drawnIsYears &&
        asked3.length === asked1.length,
    detail: `after one Escape: depth ${back1.depth}, counts "${back1.counts}", focus on "${focusBack1}"; ` +
      `after two: depth ${back0.depth}, counts "${back0.counts}", legend ${back0.legend}, ` +
      `breadcrumb ${back0.crumbHidden ? "hidden" : "SHOWING"}`,
  });
  out.push({
    name: "chain: reopening fetches nothing, and each breadcrumb control closes to its own depth",
    ok: asked4.length === asked1.length &&
        inner.depth === 1 && inner.counts === at1.counts &&
        back0b.depth === 0 && back0b.counts === at0.counts && back0b.crumbHidden,
    detail: `${asked4.length} fetch(es) after 0->1->2->1->0->1->2, want ${asked1.length}; the inner ` +
      `control left depth ${inner.depth} reading "${inner.counts}", and the outermost left depth ` +
      `${back0b.depth} reading "${back0b.counts}"`,
  });
  out.push({
    name: "chain: a group with no divisions says nothing opens further, rather than naming a column that is not there",
    ok: capital.depth === 1 && !capitalOpens &&
        capital.hint === "This is Capital Funds, broken into its parts. Nothing here opens further; go back to open another." &&
        capital.desc.startsWith("Opened into Capital Funds. " + step0.description),
    detail: `opened into capital: ${capitalOpens ? "SOMETHING opens" : "nothing opens"}; hint "${capital.hint}"`,
  });
  return out;
}

/**
 * A template's chart <desc> as the browser receives it, with the packager's slot
 * filled and the template's line wrapping collapsed.
 *
 * READ FROM THE TEMPLATE for year.mjs's reason: an expectation typed here is a
 * copy of a sentence the template owns, and the copy is what stays green while
 * the original drifts.
 */
function templateDesc(file, description) {
  const src = readFileSync(join(repoRoot, "site", file), "utf8");
  const m = src.match(/<desc id="chart-desc">([\s\S]*?)<\/desc>/);
  if (!m) throw new Error(file + " renders no #chart-desc to pin against");
  return m[1].replace(/\s+/g, " ").trim().replaceAll("{{.ChartDescription}}", description);
}

