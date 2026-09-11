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
  loadApp, goldenFundFlows, goldenFundFlows2027, goldenGraph, goldenGraph2027, plannedFetch,
  stepDescriptions,
  settle, refusals, twoYearConfig, repoRoot, residualDeclaration,
} from "./harness.mjs";

/**
 * The one chart page the site ships, verbatim from pkg/cmd/export/data.go's
 * views(): the spine, drawn whole, whose fund groups open into fund-flows and
 * whose divisions open into their object categories.
 *
 * THE TIER SETS AND CAPS ARE COPIED because there is no seam: views() is Go and
 * this is node. So that copy is a claim, and TestViewsOpensOnTheSpineAndGives
 * YearsToItAlone is what keeps it honest from the other side -- it asserts
 * these exact steps off the real view list, field by field. If you change
 * either, both sides go red and that is the point.
 *
 * THE DESCRIPTIONS ARE NOT COPIED, AND THAT TEST IS NOT WHAT HOLDS THEM. It
 * pins views() against a literal in the test file and reads nothing in this
 * directory, so a rewording applied to data.go and to that literal together
 * left this file measuring the client under a sentence the site had stopped
 * shipping -- with `make js` green and the new wording in dist/index.html.
 * They are read out of the Go source now (stepDescriptions), the way the
 * residual set is. fisc-vsu8.
 *
 * THE PER-YEAR JOIN IS NOT HERE. The Go side declares YearProjections and the
 * packager resolves them into each year's `steps` entries; the client reads
 * those and joins nothing. stepDocsFor below is what the packager ships for
 * one year, and the year check in year.mjs is where the join is measured.
 *
 * THE RESIDUAL IS NOT COPIED EITHER. views() reads it off check.ResidualNodes
 * and the Go test pins that; this side reads the same declaration out of the
 * Go source (residualDeclaration), so the client is measured under the set
 * the site ships and no third spelling of five ids exists to drift.
 */
// THE SENTENCES ARE READ OUT OF THE PACKAGER, NOT SPELLED HERE.
//
// A copy would check the copy: the Go test pins views() against a literal in
// the test, so rewording data.go and that literal together left this file
// measuring the client under a sentence the site no longer shipped, with every
// gate green. fisc-vsu8, and the same argument as residualDeclaration's.
const STEP_DESCRIPTIONS = stepDescriptions();

const PAGE = {
  steps: [
    {
      from: 2, projection: "fund-flows", tiers: [0, 3, 4],
      caps: [{ tier: 3, cap: 8 }, { tier: 4, cap: 24 }],
      back: "All fund groups", tail: "funds",
      residual: residualDeclaration(),
      description: STEP_DESCRIPTIONS[0],
    },
    {
      from: 4, tiers: [4, 5], caps: [{ tier: 5, cap: 8 }],
      back: "All divisions", tail: "categories",
      description: STEP_DESCRIPTIONS[1],
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

/**
 * The two fund-flows columns the page reaches, one per spine year, and the
 * figures each measured pin below takes over them.
 *
 * ONE ENTRY PER PUBLISHED SPINE YEAR, AND EVERY MEASURED PIN RUNS OVER BOTH.
 * The packager joins each year to its own fund-flows document (stepStems), so
 * a check that serves one year's capture under both paths pins that year's
 * figures twice over and leaves the other year's drill with nothing able to
 * see it go wrong (fisc-ko1j.6). The two documents are the same 280 facts
 * read down different printed columns and they differ in shape: fund/207
 * prints a dash in FY2026-27 and is not a node there, so special-revenue has
 * 31 funds against 32 and its tail folds 23. Where a figure differs it is
 * stated here as two numbers rather than one.
 *
 * EVERY FIGURE IS PINNED, NOT BOUNDED, for the file's reason. Measured
 * through the shipped entry points over the two committed captures. The
 * General Fund's depth-1 tuple is 39 nodes and 37 links citing 141 of 280 in
 * both years, and the sub-pixel count is NOT the same in both -- 2 in
 * FY2025-26 and 3 in FY2026-27 -- which is why COLUMNS carries it per column
 * and walkChain pins it once per column rather than once.
 */
const COLUMNS = [
  {
    stem: "sankey", label: "FY 2025-26", step: "fund-flows", golden: goldenFundFlows,
    spine: goldenGraph,
    // The narrowest depth-2 ribbon, which is Patrol's in both years.
    worstDeep: "51.38",
    // PAGE.worst at the declared caps and uncapped: ribbons, and how many of
    // them lay out under a pixel.
    capped: { links: 22, hairlines: 2 }, uncapped: { links: 49, hairlines: 22 },
    tail: "24 smaller funds",
    // fund/100's share of the fund column's inflow, and how many times the
    // smallest fund's inflow it is: the two figures step 0's description
    // rounds to "half" and "less than a thirty-thousandth".
    share: "49.18", ratio: 31575,
    // The General Fund's depth-1 tuple with the residual drawn: nodes, links
    // and sub-pixel ribbons. 34 / 33 / 2 before the residual; the five marks
    // and four ribbons added are the residual node, its four endpoints and
    // the four spine links carried onto it.
    general: { nodes: 39, links: 37, hairlines: 2 },
    // THE RESIDUAL PER GROUP, IN CENTS, MEASURED OFF fisc export's OWN
    // sankey.json AND fund-flows.json (2026-09-11) under the check's
    // whole-or-nothing rule and independently of app.js: a declared
    // endpoint's spine link is residual where the fund-level document
    // carries nothing from it into the group's funds, and the outflow side is
    // stated only for the group the fund-level document decomposes. A group
    // absent here draws no residual node at all: special-revenue, enterprise
    // and debt-service have their transfers in decomposed to the cent and
    // draw no fund-balance row.
    residual: {
      // 1,034,154 draw + 480,400 transfers in; 4,699,425 reserve increase +
      // 10,037,797 transfers out.
      "fund-group/general": { in: 151455400, out: 1473722200, carried: 4 },
      "fund-group/capital": { in: 250021300, out: 0, carried: 1 },
      "fund-group/internal-service": { in: 614753300, out: 0, carried: 1 },
    },
  },
  {
    stem: "sankey-2027", label: "FY 2026-27", step: "fund-flows-2027", golden: goldenFundFlows2027,
    spine: goldenGraph2027,
    worstDeep: "67.02",
    capped: { links: 22, hairlines: 1 }, uncapped: { links: 47, hairlines: 18 },
    tail: "23 smaller funds",
    share: "50.79", ratio: 54786,
    // One more hairline than FY2025-26: transfers in at 486,735 lays out
    // under a pixel beside the draw's absence.
    general: { nodes: 39, links: 37, hairlines: 3 },
    residual: {
      // 486,735 transfers in and NO draw -- general's change in working
      // capital turns positive this year, so it leaves as 2,351,098 of
      // fund-balance/contribution, beside 3,332,607 reserve increase and
      // 10,146,598 transfers out. The endpoint a FY2025-26-only set would
      // have missed.
      "fund-group/general": { in: 48673500, out: 1583030300, carried: 4 },
      "fund-group/capital": { in: 1012941600, out: 0, carried: 1 },
      "fund-group/internal-service": { in: 716064500, out: 0, carried: 1 },
    },
  },
];

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
 * The spine page carrying the chain, opened through main() on `column`'s year
 * over the committed documents -- `plan` overriding what any path answers,
 * `tweak` editing the config before app.js reads it.
 *
 * TWO YEARS, EACH WITH ITS OWN SPINE AND ITS OWN STEP DOCUMENT, AND EACH
 * ANSWERED WITH ITS OWN CAPTURE. The spine used to be FY2025-26's under both
 * paths, because the drill read nothing off it but the clicked node's id and
 * label; the residual node copies the spine's own links, and the columns
 * differ exactly where the declared set says they do, so each path now
 * answers its year's capture. The page opens on `column` the way a restored
 * radio would (checkedStem), so a check runs over the second column without
 * switching to it -- the switch is year.mjs's subject.
 *
 * THE SPINE'S LABEL FOR THE GROUP IS MADE DISTINCT, because both committed
 * documents print "General Fund" for fund-group/general and a rung named
 * from the wrong document would be invisible. The breadcrumb, the chart name
 * and the hint name the node the reader clicked in the words of the chart
 * they clicked it on -- the spine's -- and not the step document's.
 */
async function opened(plan, tweak, column = COLUMNS[0]) {
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
  const spineOf = (/** @type {() => any} */ load) => {
    const spine = load();
    spine.nodes.find((n) => n.id === "fund-group/general").label = "General Fund group";
    return spine;
  };
  const fetch = plannedFetch(Object.assign({
    "data/sankey.json": { doc: spineOf(goldenGraph) },
    "data/sankey-2027.json": { doc: spineOf(goldenGraph2027) },
    "data/fund-flows.json": { doc: goldenFundFlows() },
    "data/fund-flows-2027.json": { doc: goldenFundFlows2027() },
  }, plan || {}));
  const app = loadApp({ config, fetch, checkedStem: column.stem });
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

  for (const col of COLUMNS) {
    const { app, body, fetch } = await opened(null, null, col);
    const before = shown(app, body);
    out.push({
      name: `${col.label}: the overview draws the spine whole, from its own year's file, and its counts line describes it`,
      ok: before.counts === PAGE.overview.counts && before.rows === PAGE.overview.links &&
          before.crumbHidden && fetch.asked.join() === `data/${col.stem}.json`,
      detail: `counts "${before.counts}", ${before.rows} table rows, breadcrumb ` +
              (before.crumbHidden ? "hidden" : "SHOWING with nothing opened") +
              `; main() asked for ${JSON.stringify(fetch.asked)}`,
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
    // THE STEP FILE IS THE COLUMN'S OWN, asserted on the wire: a walk that
    // drew every view from the other year's file would pin that year twice.
    const stepAsked = fetch.asked.filter((p) => p.startsWith("data/fund-flows"));
    out.push({
      name: `${col.label}: every node the chain offers to open draws when opened, at both depths`,
      ok: walk.refused === "" && walk.visited === PAGE.openedViews && drawn.length === walk.visited &&
          Boolean(worstDeep) && worstDeep.where.endsWith(" > " + PAGE.inert) &&
          worstDeep.smallest.toFixed(2) === col.worstDeep &&
          stepAsked.join() === `data/${col.step}.json`,
      detail: walk.refused
        ? `after ${walk.visited} view(s), refused: ${walk.refused}`
        : `${walk.visited} views opened (want ${PAGE.openedViews}) from ${JSON.stringify(stepAsked)}; ` +
          `smallest ribbon over all of them ${Math.min(...drawn.map((d) => d.smallest)).toFixed(3)}px; ` +
          `the narrowest depth-2 ribbon is ` +
          `${worstDeep ? `${worstDeep.where} at ${worstDeep.smallest.toFixed(2)}px` : "nowhere"} ` +
          `(want ${col.worstDeep})`,
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
    const { app: noCap } = await opened(null, (c) => { c.steps = uncappedSteps; }, col);
    await at(app, PAGE.worst);
    const worst = measure(app, app.projection);
    await at(noCap, PAGE.worst);
    const worstUncapped = measure(noCap, noCap.projection);
    const engaged = worst.links < worstUncapped.links;
    out.push({
      name: `${col.label}: the fund cap is what makes the worst group's column drawable`,
      // PINNED, NOT BOUNDED: in FY2025-26, 32 funds uncapped lay 22 of 49
      // ribbons under a pixel and capped at 8 they lay 2 of 22; in FY2026-27,
      // 31 funds lay 18 of 47 and then 1 of 22. The first pair is the figure
      // pkg/cmd/export/data.go quotes for the cap.
      ok: engaged && worstUncapped.links === col.uncapped.links &&
          worstUncapped.hairlines === col.uncapped.hairlines &&
          worst.links === col.capped.links && worst.hairlines === col.capped.hairlines,
      detail: `${PAGE.worst} capped: ${worst.links} ribbons, ${worst.hairlines} under 1px ` +
        `(want ${col.capped.links}, ${col.capped.hairlines}); uncapped: ${worstUncapped.links} ` +
        `ribbons, ${worstUncapped.hairlines} under 1px (want ${col.uncapped.links}, ` +
        `${col.uncapped.hairlines}); the cap ${engaged ? "folded a tail" : "folded nothing"}`,
    });
    await at(app, "fund-group/general", PAGE.inert);
    const inert = measure(app, app.projection);
    await at(noCap, "fund-group/general", PAGE.inert);
    const inertUncapped = measure(noCap, noCap.projection);
    out.push({
      name: `${col.label}: the category cap is inert two rungs deep, because no division is wide enough to need it`,
      ok: inert.links === inertUncapped.links && inert.hairlines === inertUncapped.hairlines &&
          inert.links > 0,
      detail: `${PAGE.inert} capped: ${inert.links} ribbons, ${inert.hairlines} under 1px; ` +
        `uncapped: ${inertUncapped.links} ribbons; smallest ribbon ${inert.smallest.toFixed(2)}px`,
    });
    app.drillUp(0);
    noCap.drillUp(0);

    // THE SENTENCE UNDER THE CHART IS MEASURED ON BOTH YEARS. Step 0's
    // description says the General Fund is "half the fund column" and the
    // smallest fund "less than a thirty-thousandth of it", and it is shown
    // under whichever year is on screen. The exact figures are the column's
    // -- fund/550 at $5,000 is 1/31,575 of fund/100 in FY2025-26, and fund/202
    // at $3,000 is 1/54,786 in FY2026-27 -- so the sentence carries the bound
    // both clear and this pins the two numbers it rounds.
    const raw = col.golden();
    const inflow = new Map();
    for (const l of raw.links) {
      if (l.target.startsWith("fund/")) inflow.set(l.target, (inflow.get(l.target) || 0) + l.value_cents);
    }
    const column = [...inflow.values()].reduce((a, b) => a + b, 0);
    const general = inflow.get("fund/100") || 0;
    const [smallestFund, smallestIn] = [...inflow.entries()].reduce((a, b) => (b[1] < a[1] ? b : a));
    const share = (100 * general) / column;
    const ratio = general / smallestIn;
    out.push({
      name: `${col.label}: the step's description rounds figures this column still supports`,
      ok: share.toFixed(2) === col.share && Math.round(ratio) === col.ratio &&
          share > 45 && share < 55 && ratio > 30000 &&
          PAGE.steps[0].description.includes("is half the fund column") &&
          PAGE.steps[0].description.includes("less than a thirty-thousandth of it"),
      detail: `fund/100 takes ${share.toFixed(2)}% of the fund column's inflow (want ${col.share}), ` +
        `and ${smallestFund} at ${smallestIn / 100} dollars is 1/${Math.round(ratio)} of it ` +
        `(want 1/${col.ratio})`,
    });
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
  for (const col of COLUMNS) {
    const { app } = await opened(null, null, col);
    const parentOf = new Map();
    for (const n of [...goldenGraph().nodes, ...col.golden().nodes]) parentOf.set(n.id, n.parent);
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
      name: `${col.label}: every mark on the fund side knows its group, on the overview and in every opened view`,
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
  //
  // THE TAIL'S SIZE IS THE COLUMN'S: 24 smaller funds in FY2025-26 and 23 in
  // FY2026-27, where fund/207 prints a dash and is not there to fold.
  for (const col of COLUMNS) {
    const { app } = await opened(null, null, col);
    await at(app, PAGE.worst);
    const agg = app.projection.nodes.find((n) => n.id === app.aggregateID(3));
    out.push({
      name: `${col.label}: the capped tail is marked as ours, not as something the city printed`,
      // THE COUNT IS ASSERTED AT TWO OR MORE, not just matched as digits.
      // capColumn engaged at cap + 1, so a column of 9 against a cap of 8 folded
      // ONE city-printed fund into a derived node labelled "1 smaller funds" --
      // and this regex accepted it. fund-group/enterprise has exactly 9.
      ok: Boolean(agg) && agg.derived === true && agg.rationale !== "" &&
          agg.source_note !== "" && /^(\d+) smaller funds$/.test(agg.label) &&
          Number(agg.label.split(" ")[0]) >= 2 && agg.label === col.tail,
      detail: agg
        ? `"${agg.label}" (want "${col.tail}") derived=${agg.derived}, rationale ` +
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
      name: `${col.label}: no opened view folds a single printed figure into an aggregate of one`,
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
  for (const col of COLUMNS) {
    const { app } = await opened(null, null, col);
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
      name: `${col.label}: a caveat about one node reaches that node at depth 0 and, over the other document, at depth 1`,
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
  // THE GENERAL FUND'S COLUMN DIVIDES SINCE THE RESIDUAL: fund/100 is no
  // longer alone in it -- "Not broken down by fund" stands beside it -- so
  // its share is expected there too, and it is a share of what is drawn.
  for (const where of [
    { open: [], sharesColumn: true, stem: "sankey" },
    { open: ["fund-group/general"], sharesColumn: true, stem: "fund-flows" },
  ]) {
    const { app } = await opened();
    await at(app, ...where.open);
    const name = where.open.length ? "opened into " + where.open.join(" > ") : "the overview";
    app.layOut(app.projection);
    // A DRAWN MARK, EXPLICITLY. Since fisc-bccu a carried mark carries its own
    // document's caveats too, and it sorts into this list -- so `find` without
    // the filter would silently start measuring a node whose right anchor is
    // the SPINE's, and this arm's wantHref is the step document's. The carried
    // side is the arm below, which is the one that would go red.
    const marked = app.projection.nodes.find(
      (n) => !n.carried_from && app.caveatsFor(n.id).length > 0);
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
      // is fund/100, which shared its column with nothing until the residual
      // stood beside it -- and a share of a column of one would read "our
      // 100.0% of this column", a derived chip carrying a figure that is
      // 100% by construction, which columnShare suppresses.
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

  // A CARRIED MARK KEEPS THE CAVEAT OF THE DOCUMENT IT CAME FROM.
  //
  // carryResidual copies the spine's endpoints onto the rung, and the rung's
  // document has never heard of them -- so caveatsFor, filtering the DRAWN
  // document's metadata.caveats, returned [] for a figure the spine qualifies.
  // Measured before the fix, both columns: transfers/in and transfers/out carry
  // transfer-legs-unpaired at depth 0 and arrived at depth 1 with nothing.
  // fisc-bccu.
  //
  // THE ANCHOR IS THE OTHER HALF. A caveat that resolves but links nowhere is
  // fisc-ko1j.13's symptom by the other route, so this pins the href too -- and
  // pins it per column, because the packager composes one anchor per (document,
  // caveat) and the two years are two documents.
  for (const column of COLUMNS) {
    const { app } = await opened(null, null, column);
    await at(app, "fund-group/general");
    const carried = app.projection.nodes.filter((n) => n.carried_from);
    const withCaveat = carried.filter((n) => app.caveatsFor(n.id).length > 0);
    const ids = withCaveat.map((n) => n.id).sort();

    // THE PANEL, NOT caveatHref. Reading the anchor off caveatHref(id, true)
    // hands the function the very argument the defect is about, so the arm
    // passed with the call site's `Boolean(n.carried_from)` deleted -- the
    // check tested the function and nobody tested the caller. Driving pin()
    // and reading the rendered panel is what a reader actually gets, and it is
    // the only route that goes red on that deletion. Found by pass two of
    // /code-review over its own pass-one fix.
    const laid = app.layOut(app.projection);
    const textOf = (/** @type {any} */ el) => {
      const parts = [];
      const walk = (/** @type {any} */ n) => {
        if (n.textContent) parts.push(n.textContent);
        for (const c of n.children || []) walk(c);
      };
      walk(el);
      return parts.join(" ");
    };
    const panelFor = (/** @type {string} */ id) => {
      app.pin(laid.nodes.find((n) => n.id === id));
      return textOf(app.dom.byId.get("detail"));
    };
    const hrefsIn = (/** @type {any} */ el) => {
      const found = [];
      const walk = (/** @type {any} */ n) => {
        // link() in app.js assigns a.href as a PROPERTY, so the stub's
        // getAttribute("href") answers null and a walker reading attributes
        // finds nothing -- which is a check that would pass on a panel with no
        // link at all. Read what app.js actually sets.
        if (n.href) found.push(n.href);
        for (const c of n.children || []) walk(c);
      };
      walk(el);
      return found;
    };
    const first = withCaveat[0];
    const caveat = first ? app.caveatsFor(first.id)[0].id : "";
    const wantHref = `caveats.html#caveat-${column.stem}--${caveat}`;
    let carriedPanel = "";
    let carriedHrefs = [];
    if (first) {
      carriedPanel = panelFor(first.id);
      carriedHrefs = hrefsIn(app.dom.byId.get("detail"));
    }
    // The drawn mark beside it must STILL reach the step document's anchor by
    // the same route: the fix must not have moved every caveat onto the spine.
    const drawnCaveat = app.caveatsFor("fund/100")[0];
    const drawnPanel = panelFor("fund/100");
    const drawnHrefs = hrefsIn(app.dom.byId.get("detail"));

    // AND THE PAGES THE PANEL SENDS A READER TO. A carried mark falls back to
    // its document's sources, not the drawn document's -- the residual's own
    // source_note names p.66 and the Sources row cited pp.127-140 beneath it.
    // Read as page numbers off whatever anchor shapes citations() emits, so
    // this does not pin the anchor format as well.
    const pagesIn = (/** @type {string[]} */ hs) => [...new Set(hs
      .filter((h) => !h.startsWith("caveats"))
      .map((h) => (h.match(/p(?:age=)?0*(\d+)/) || [])[1])
      .filter(Boolean))].map(Number).sort((a, b) => a - b);
    panelFor(first.id);
    const carriedPages = pagesIn(hrefsIn(app.dom.byId.get("detail")));
    panelFor("fund/100");
    const drawnPages = pagesIn(hrefsIn(app.dom.byId.get("detail")));
    const wantDrawn = `caveats.html#caveat-${column.step}--${drawnCaveat ? drawnCaveat.id : ""}`;
    out.push({
      name: `${column.label}: a carried mark's panel links to the spine's copy of the caveat, and the drawn mark beside it still links to the step document's`,
      ok: carried.length > 0 && ids.length === 2 &&
          ids[0] === "transfers/in" && ids[1] === "transfers/out" &&
          caveat === "transfer-legs-unpaired" &&
          carriedPanel.includes("Read it in full") && carriedHrefs.includes(wantHref) &&
          drawnPanel.includes("Read it in full") && drawnHrefs.includes(wantDrawn) &&
          carriedPages.join() === "66,67" && drawnPages.includes(127) && !drawnPages.includes(66),
      detail: `${carried.length} carried mark(s), of which ${ids.length} carry a caveat ` +
        `(${ids.join(", ")}); ${first ? first.id : "none"}'s panel ` +
        `${carriedPanel.includes("Read it in full") ? "links" : "does NOT link"} to ` +
        `${JSON.stringify(carriedHrefs.find((h) => h.startsWith("caveats.html")) || "")} ` +
        `(want "${wantHref}"); fund/100's panel ` +
        `${drawnPanel.includes("Read it in full") ? "links" : "does NOT link"} to ` +
        `${JSON.stringify(drawnHrefs.find((h) => h.startsWith("caveats.html")) || "")} ` +
        `(want "${wantDrawn}"), so the carried case did not drag the drawn one with it; ` +
        `the carried mark's Sources cite pp.${carriedPages.join(",")} (the chart above) and ` +
        `fund/100's cite ${drawnPages.length} pages starting p.${drawnPages[0]} (the drawn document)`,
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
  // THE OVERVIEW AND EVERY OPENED VIEW OF BOTH COLUMNS, because the shape
  // occurs on one column of one year and a sample would miss it.
  for (const col of COLUMNS) {
    const hundreds = [];
    const { app } = await opened(null, null, col);
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
    // AND THE CASE NEITHER SHIPPED FIXTURE CAN REACH. The rounding happens on
    // fund-flows-2024-actual, opened on debt-service, and these checks fetch
    // the two adopted columns' captures -- the actual column is not committed
    // and has no spine year to open it from (fundFlowsNoSpineColumn). Removing
    // the ceiling left the scan above green for that reason alone, which is a
    // check passing because its fixture is the wrong year. So the split is
    // built: 99.9943% of a two-node column, the real proportion, laid out by
    // the real layOut.
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
      name: `${col.label}: no share on any view claims 100% of a column that has more than one mark`,
      ok: walk.refused === "" && hundreds.length === 0 && !rounded.includes("100"),
      detail: hundreds.length
        ? hundreds.slice(0, 3).join("; ")
        : `every share across the overview and all ${walk.visited} opened views is under 100%; ` +
          `a 99.9943% mark of a two-node column reads "${rounded}"`,
    });
  }

  // ------------------------------------------------------------ the residual
  //
  // THE RESIDUAL, MEASURED OVER BOTH COLUMNS. Every figure below is pinned in
  // COLUMNS[].residual off fisc export's own documents, independently of
  // app.js; the arms add that the marks reach the DOM, that each carried link
  // is the spine's own byte for byte, and that the set drawn is the declared
  // one read off the step and nothing this file or app.js spelled.
  for (const col of COLUMNS) {
    const declared = PAGE.steps[0].residual;
    const spine = col.spine();
    const stepDoc = col.golden();
    const stepHas = new Set(stepDoc.nodes.map((n) => n.id));
    const spineLink = new Map(spine.links.map((l) => [l.source + "|" + l.target, l]));
    const groups = spine.nodes.filter((n) => n.tier === PAGE.steps[0].from).map((n) => n.id).sort();
    const general = "fund-group/general";
    const want = col.residual;
    const { app } = await opened(null, null, col);
    const residualOf = (/** @type {string} */ g) => {
      const id = app.residualID(g);
      const node = app.projection.nodes.find((n) => n.id === id);
      const links = app.projection.links.filter((l) => l.source === id || l.target === id);
      return { id, node, links,
        in: links.filter((l) => l.target === id).reduce((sum, l) => sum + l.value_cents, 0),
        out: links.filter((l) => l.source === id).reduce((sum, l) => sum + l.value_cents, 0) };
    };

    // WHERE IT IS DRAWN AND WHERE IT IS NOT, over all six groups. A group the
    // pins name draws one node with exactly the pinned sums; a group they do
    // not draws no residual node AND no endpoint copied in beside its funds
    // -- transfers/in drawn under enterprise is the step document's own node,
    // carrying the decomposed flows, and is told apart by the step golden.
    const seen = {};
    const stray = [];
    for (const g of groups) {
      await at(app, g);
      const r = residualOf(g);
      if (r.node || r.links.length) seen[g] = { in: r.in, out: r.out, carried: r.links.length };
      for (const n of app.projection.nodes) {
        if (Object.hasOwn(declared, n.id) && !stepHas.has(n.id) && !r.node) stray.push(g + ": " + n.id);
      }
      if (r.node && !r.links.length) stray.push(g + ": a residual node with no flow");
    }
    const asSeen = JSON.stringify(seen, Object.keys(seen).sort());
    const asWant = JSON.stringify(want, Object.keys(want).sort());
    // AND WITH THE DECLARATION REMOVED FROM THE STEP, NOTHING IS DRAWN: the
    // client spells no endpoint of its own, so the shipped set is the only
    // source of the marks.
    const { app: undeclared } = await opened(null, (c) => {
      c.steps = PAGE.steps.map((st) => { const t = Object.assign({}, st); delete t.residual; return t; });
    }, col);
    await at(undeclared, general);
    const none = residualOf.call(null, general);
    const noneDrawn = !undeclared.projection.nodes.some((n) => app.isResidual(n.id)) &&
      undeclared.projection.links.length === col.general.links - want[general].carried &&
      !undeclared.projection.nodes.some((n) => Object.hasOwn(declared, n.id) && !stepHas.has(n.id));
    void none;
    out.push({
      name: `${col.label}: the residual is drawn beside the funds of exactly the groups whose flows the fund-level document does not decompose, and only from the declared set`,
      ok: asSeen === asWant && stray.length === 0 && noneDrawn,
      detail: (asSeen === asWant
        ? `drawn on ${Object.keys(seen).map((g) => g.replace("fund-group/", "")).join(", ")} with the ` +
          `pinned sums, and on no other group`
        : `drawn ${asSeen}, want ${asWant}`) +
        (stray.length ? `; stray carried marks: ${stray.join("; ")}` : "") +
        `; with residual deleted from the step, the General Fund draws ` +
        `${noneDrawn ? "no carried mark" : "CARRIED MARKS FROM NOWHERE"}`,
    });

    // CARRIED, NOT COMPUTED. Each link on a residual node is the spine's link
    // between that endpoint and the group with its value_cents, fact_ids,
    // locators, kind and derived flag byte-equal -- three named fields, three
    // mutations -- and its far end is a declared endpoint, never a fund.
    const mismatches = [];
    let compared = 0;
    for (const g of Object.keys(want)) {
      await at(app, g);
      const r = residualOf(g);
      for (const l of r.links) {
        const arrives = l.target === r.id;
        const e = arrives ? l.source : l.target;
        const original = spineLink.get(arrives ? e + "|" + g : g + "|" + e);
        compared++;
        if (!Object.hasOwn(declared, e)) { mismatches.push(`${g}: ${e} is not a declared endpoint`); continue; }
        if (!original) { mismatches.push(`${g}: the spine has no link ${arrives ? e + " -> " + g : g + " -> " + e}`); continue; }
        for (const field of ["value_cents", "fact_ids", "locators", "kind", "derived"]) {
          if (JSON.stringify(l[field]) !== JSON.stringify(original[field])) {
            mismatches.push(`${g}: ${e} ${field} ${JSON.stringify(l[field])} != ${JSON.stringify(original[field])}`);
          }
        }
        const end = app.projection.nodes.find((n) => n.id === e);
        if (!end || !PAGE.steps[0].tiers.includes(end.tier)) {
          mismatches.push(`${g}: endpoint ${e} is ${end ? "at undrawn tier " + end.tier : "not drawn"}`);
        }
      }
    }
    out.push({
      name: `${col.label}: every carried flow is the spine's own link byte for byte, and ends at a declared endpoint rather than a fund`,
      ok: compared > 0 && mismatches.length === 0,
      detail: mismatches.length
        ? mismatches.slice(0, 4).join("; ")
        : `${compared} carried flows over ${Object.keys(want).length} groups, each equal to its spine ` +
          `link in value_cents, fact_ids, locators, kind and derived`,
    });

    // THE IMBALANCE IS DRAWN. The General Fund's residual takes in the draw
    // and the transfer in and pays out the transfer out and the reserve
    // increase, and the two sums differ; d3-sankey sizes the node at the
    // larger, in the fund column, with its endpoints in the first and last.
    await at(app, general);
    const r = residualOf(general);
    const laid = app.layOut(app.projection);
    const laidNode = laid.nodes.find((n) => n.id === r.id);
    const tiers = PAGE.steps[0].tiers;
    const layers = r.links.map((l) => {
      const e = l.target === r.id ? l.source : l.target;
      const n = laid.nodes.find((x) => x.id === e);
      return e + "@" + (n ? n.layer : "?") + (l.target === r.id ? " in" : " out");
    });
    const endsRight = layers.every((x) => (x.endsWith(" in") ? x.includes("@0 ") : x.includes("@" + (tiers.length - 1) + " ")));
    out.push({
      name: `${col.label}: the General Fund's residual takes in less than it pays out, and both are drawn rather than balanced`,
      ok: Boolean(r.node) && r.in === want[general].in && r.out === want[general].out && r.in !== r.out &&
          Boolean(laidNode) && laidNode.value === Math.max(r.in, r.out) &&
          laidNode.layer === tiers.indexOf(r.node.tier) && r.node.tier === 3 &&
          endsRight && app.fundGroupOf(r.node) === general,
      detail: r.node
        ? `in ${r.in} out ${r.out} cents (want ${want[general].in} / ${want[general].out}); laid at ` +
          `${laidNode ? laidNode.value : "nowhere"} in column ${laidNode ? laidNode.layer : "?"} of tier ` +
          `${r.node.tier}; ends ${layers.join(", ")}; hue from ${app.fundGroupOf(r.node) || "no group"}`
        : "no residual node on the General Fund",
    });

    // MARKED AS OURS AND REACHING THE READER: derived, a rationale carrying
    // every reason the check declares for the endpoints it carries, a source
    // note naming the pages the carried links cite, an entry in "What we
    // inferred", and the derived chip in the tooltip and the panel. Read
    // back from the DOM, because a field set and a renderer that ignores it
    // are indistinguishable by any other route.
    const text = (/** @type {any} */ el) => {
      const parts = [];
      const walk = (/** @type {any} */ n) => {
        if (n.textContent) parts.push(n.textContent);
        for (const c of n.children || []) walk(c);
      };
      walk(el);
      return parts.join(" ");
    };
    const carriedEnds = r.links.map((l) => (l.target === r.id ? l.source : l.target));
    // THE PAGES ARE THE CARRIED LINKS' OWN, read off their locators rather
    // than typed: the General Fund's four spine links all cite p.66, which
    // testdata/README.md's rule for the OTHER groups (p.67) would not predict.
    const citedPages = [...new Set(r.links.flatMap((l) => l.locators.flatMap((s) => s.pages)))];
    const missingReasons = carriedEnds.filter((e) => !r.node || !r.node.rationale.includes(declared[e]));
    const listed = text(app.dom.byId.get("derived-list"));
    app.showTip({ target: app.dom.byId.get("chart"), clientX: 0, clientY: 0 }, laidNode);
    const tip = text(app.dom.byId.get("tooltip"));
    app.pin(laidNode);
    const panel = text(app.dom.byId.get("detail"));
    const opens = app.projection.nodes.filter((n) => app.isCarried(n.id) && app.drillable(n)).map((n) => n.id);
    out.push({
      name: `${col.label}: the residual is marked as ours, says why in the check's words, and reaches the inferred list, the tooltip and the panel; nothing carried opens`,
      ok: Boolean(r.node) && r.node.derived === true && r.node.label === "Not broken down by fund" &&
          r.node.rationale !== "" && missingReasons.length === 0 &&
          r.node.source_note.includes("Carried, not computed") &&
          citedPages.every((pg) => r.node.source_note.includes(String(pg))) &&
          listed.includes("Not broken down by fund") && listed.includes(r.node.rationale) &&
          tip.includes("◇ inferred") && tip.includes(r.node.rationale) &&
          panel.includes("◇ our inference") && panel.includes(r.node.rationale) &&
          panel.includes(r.node.source_note) && opens.length === 0,
      detail: r.node
        ? `derived=${r.node.derived}, label "${r.node.label}"; rationale carries ` +
          `${carriedEnds.length - missingReasons.length} of ${carriedEnds.length} declared reasons` +
          (missingReasons.length ? ` (missing ${missingReasons.join(", ")})` : "") +
          `; source note names ${citedPages.every((pg) => r.node.source_note.includes(String(pg))) ? "" : "NOT "}` +
          `every cited page (${citedPages.join(", ")}); inferred list ` +
          `${listed.includes("Not broken down by fund") ? "lists it" : "OMITS it"}; tooltip ` +
          `${tip.includes("◇ inferred") ? "chips it inferred" : "chips it PRINTED"}; panel ` +
          `${panel.includes("◇ our inference") ? "chips it ours" : "chips it PRINTED"}; ` +
          `${opens.length ? opens.join(", ") + " WRONGLY open" : "no carried mark opens"}`
        : "no residual node on the General Fund",
    });

    // WHOLE OR NOTHING, ON THE GENERAL FUND, WHICH THE CORPUS CANNOT SHOW: the
    // three groups whose transfers in are decomposed witness the "carries
    // any, copies nothing" branch, and general witnesses the other. This
    // builds the step document that decomposes general's transfer in whole
    // -- one link, transfers/in -> fund/100, at the spine's own figure -- and
    // expects the residual to drop exactly that link and keep the rest.
    const transferIn = spineLink.get("transfers/in|" + general);
    const decomposedIn = JSON.parse(JSON.stringify(stepDoc));
    decomposedIn.links.push(Object.assign({}, transferIn, { target: "fund/100" }));
    const { app: split } = await opened({ [`data/${col.step}.json`]: { doc: decomposedIn } }, null, col);
    await at(split, general);
    const sr = (() => {
      const id = split.residualID(general);
      const links = split.projection.links.filter((l) => l.source === id || l.target === id);
      return { node: split.projection.nodes.find((n) => n.id === id), links,
        in: links.filter((l) => l.target === id).reduce((sum, l) => sum + l.value_cents, 0),
        out: links.filter((l) => l.source === id).reduce((sum, l) => sum + l.value_cents, 0) };
    })();
    const stillCarried = sr.links.some((l) => l.source === "transfers/in");
    out.push({
      name: `${col.label}: an endpoint the fund-level document decomposes whole is not carried, even on the General Fund`,
      ok: Boolean(sr.node) && !stillCarried && sr.in === want[general].in - transferIn.value_cents &&
          sr.out === want[general].out && sr.links.length === want[general].carried - 1,
      detail: sr.node
        ? `with transfers/in -> fund/100 at ${transferIn.value_cents} in the step document, the residual ` +
          `${stillCarried ? "STILL carries transfers/in" : "drops transfers/in"} and reads in ${sr.in} ` +
          `(want ${want[general].in - transferIn.value_cents}) out ${sr.out} over ${sr.links.length} flows`
        : "no residual node on the General Fund",
    });
  }

  // ---------------------------------------------------------------- the chain
  //
  // 0 -> 1 -> 2 -> 1 -> 0 through the real entry points, over the two
  // committed documents at once, each depth read back from the DOM: the
  // stack, the fetches, the counts line, the chart's name and description,
  // the breadcrumb, the hint, the legend, the flow table and where focus went.
  // ONCE PER COLUMN: the words carry the year, and the General Fund's depth-1
  // tuple happens to be the same in both, which is asserted rather than
  // assumed.
  for (const col of COLUMNS) out.push(...(await walkChain(col)));

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
async function walkChain(col) {
  const out = [];
  const { app, fetch, body } = await opened(null, null, col);
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
  // A CARRIED ENDPOINT BELONGS TO NO GROUP, exactly as it does on the spine
  // it was copied from: a transfer out is money leaving, not money held.
  const muted1 = app.projection.nodes
    .filter((n) => !n.id.startsWith("revenue/") && !app.isCarried(n.id) && app.fundGroupOf(n) === "")
    .map((n) => n.id);
  // THE DIVISION CAP IS INERT ON THE CORPUS AND PINNED INERT: 23 divisions
  // under a cap of 24, so no aggregate at tier 4 -- the day a 24th division
  // appears the column starts folding, and this is what says so.
  const divisions1 = app.projection.nodes.filter((n) => n.id.startsWith("dept/")).length;
  const foldedDivisions1 = app.projection.nodes.some((n) => n.id === app.aggregateID(4));
  // THE DOCUMENT'S OWN FACTS, which is what the counts line claims a share
  // of: a carried flow cites the spine, and its facts are counted apart.
  const cited1 = new Set();
  let carried1 = 0;
  for (const l of app.projection.links) {
    if (app.isResidual(l.source) || app.isResidual(l.target)) { carried1++; continue; }
    for (const id of l.fact_ids) cited1.add(id);
  }

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

  const stepFile = `data/${col.step}.json`;
  out.push({
    name: `${col.label} chain: the year's own step document is fetched on the first drill and not before`,
    ok: asked0.length === 1 && asked0[0] === `data/${col.stem}.json` &&
        asked1.length === 2 && asked1[1] === stepFile,
    detail: `main() asked for ${JSON.stringify(asked0)}; the first drill added ` +
      `${JSON.stringify(asked1.slice(asked0.length))}`,
  });
  out.push({
    name: `${col.label} chain: the overview's hint names the column that opens, which is the spine's middle one`,
    // "MIDDLE", READ OFF THE CHART. The spine draws tiers 0, 2 and 5 and its
    // fund groups are tier 2; a hint saying "right-hand column" here would
    // send the reader to the uses, which do not open.
    ok: at0.hint === "Click a node in the middle column to open it into its parts, or tab to " +
        "one and press Enter. A fund swatch follows one group's money without opening anything." &&
        at0.legend === 6 && at0.desc === served,
    detail: `hint "${at0.hint}"; legend ${at0.legend} swatches`,
  });
  out.push({
    name: `${col.label} chain: depth 1 draws the General Fund at {0,3,4} from the other document, and every sentence says so`,
    // 34 nodes, 33 links and 2 sub-pixel ribbons is fisc-ko1j's own
    // measurement of this view, reproduced here through the shipped functions
    // -- and measured the same in FY2026-27, whose General Fund has the same
    // one fund, ten sources and 23 divisions. The residual adds five marks
    // and four ribbons to both, and one hairline to FY2026-27 alone; the
    // counts line names the carried flows apart from the document's facts.
    ok: open1 === "drew" && at1.depth === 1 && !at1.drawnIsYears &&
        Boolean(m1) && m1.nodes === col.general.nodes && m1.links === col.general.links &&
        m1.hairlines === col.general.hairlines &&
        divisions1 === 23 && !foldedDivisions1 && carried1 === col.residual["fund-group/general"].carried &&
        at1.counts === `${col.general.links} flows between ${col.general.nodes} nodes, from ` +
          `${cited1.size} of the document's 280 facts, and ${carried1} flows carried unchanged ` +
          "from the chart above" &&
        rows1 === col.general.links &&
        at1.title === `Sankey diagram of the ${col.label} adopted budget, opened into General Fund group` &&
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
    name: `${col.label} chain: depth 2 draws Patrol at {4,5}, names both rungs, keeps the table pointer, and opens nothing further`,
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
    name: `${col.label} chain: Escape closes one rung at a time, and each depth comes back as it was`,
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
    name: `${col.label} chain: reopening fetches nothing, and each breadcrumb control closes to its own depth`,
    ok: asked4.length === asked1.length &&
        inner.depth === 1 && inner.counts === at1.counts &&
        back0b.depth === 0 && back0b.counts === at0.counts && back0b.crumbHidden,
    detail: `${asked4.length} fetch(es) after 0->1->2->1->0->1->2, want ${asked1.length}; the inner ` +
      `control left depth ${inner.depth} reading "${inner.counts}", and the outermost left depth ` +
      `${back0b.depth} reading "${back0b.counts}"`,
  });
  out.push({
    name: `${col.label} chain: a group with no divisions says nothing opens further, rather than naming a column that is not there`,
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

