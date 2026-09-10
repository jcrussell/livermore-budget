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
// Spending's {3,4} could not draw at all, because the document's tier-0 revenue
// nodes have no ancestor at tier 3 or 4. Both are shapes a Go test cannot see
// and a reader meets on the first click.
//
// EVERY FIGURE HERE IS PINNED, NOT BOUNDED, for layout.mjs's reason: a bound
// that holds is not evidence a number is still the number, and these are the
// numbers pkg/cmd/export/data.go's comments quote to justify the tier sets and
// the cap.

import { readFileSync } from "node:fs";
import { join } from "node:path";

import {
  loadApp, goldenFundFlows, goldenGraph, plannedFetch, settle, refusals, twoYearConfig, repoRoot,
} from "./harness.mjs";

/**
 * The two views the site ships, verbatim from pkg/cmd/export/data.go's views().
 *
 * COPIED RATHER THAN IMPORTED because there is no seam: views() is Go and this
 * is node. So the copy is a claim, and TestViewsOpensOnTheSpineAndGivesYears
 * ToItAlone is what keeps it honest from the other side -- it asserts these
 * exact tier sets and these exact step values off the real view list, field by
 * field.
 *
 * IT DID NOT USED TO. That test asserted only that RenderTiers was non-empty
 * and the drill non-nil, which left this file free to measure a configuration no
 * page ships: change a cap or a tier set in data.go and every gate stayed green
 * while these checks went on pinning the old one. If you change either, both
 * sides go red and that is the point.
 */
const PAGES = [
  {
    name: "revenue",
    render_tiers: [0, 2],
    root: "",
    steps: [{ from: 2, tiers: [0, 3], caps: [{ tier: 3, cap: 8 }], back: "All fund groups", tail: "funds" }],
    // Measured: 11 revenue categories into 6 fund groups.
    // facts IS THE COUNT ITS OWN RIBBONS CITE, not the document's 280. The two
    // pages partition the document's 239 cited facts exactly, 190 and 49, which
    // is what two pages splitting one document should do -- and the counts line
    // says both numbers so a reader can see the gap rather than infer it.
    overview: { nodes: 17, links: 29, facts: 190 },
    // The node whose open view the cap is FOR. Its 32 funds are the shape
    // fisc-ppkq said rescaling would fix and measurement said it would not.
    worst: "fund-group/special-revenue",
    capEngages: true,
    // WHERE A CAVEAT-MARKED NODE IS, AND WHETHER ITS COLUMN DIVIDES, PER
    // DEPTH. The flag was per page, and a depth is what decides it: the
    // overview's marked node is one of six fund groups, a real share; opened
    // into general, the same caveat marks fund/100, alone in its fund column,
    // where a share would be 100% by construction and is suppressed.
    marked: [
      { open: "", sharesColumn: true },
      { open: "fund-group/general", sharesColumn: false },
    ],
  },
  {
    name: "spending",
    render_tiers: [3, 4],
    // WITHOUT THIS THE PAGE DRAWS NOTHING. See the root check below.
    root: "fund/100",
    steps: [{ from: 4, tiers: [4, 5], caps: [{ tier: 5, cap: 8 }], back: "All divisions", tail: "categories" }],
    // Measured: the General Fund into its 23 divisions.
    overview: { nodes: 24, links: 23, facts: 49 },
    // NO DIVISION SPENDS ON MORE THAN A HANDFUL OF OBJECT CATEGORIES, so the
    // cap never fires here. Pinned so that stops being true loudly.
    worst: "dept/patrol",
    capEngages: false,
    // Its marked node is fund/100, alone in its column: a share there is 100%
    // by construction and is suppressed. No opened depth is listed because the
    // one caveat naming a node names fund/100, which {4,5} does not draw.
    marked: [{ open: "", sharesColumn: false }],
  },
];

/** An app configured as one of those pages, with the committed document to fetch. */
function appFor(page, overrides) {
  return loadApp({
    fetch: plannedFetch({ "data/fund-flows.json": { doc: goldenFundFlows() } }),
    config: Object.assign({
      schema_version: 1,
      primary: "fund-flows",
      projections: { "fund-flows": "data/fund-flows.json" },
      render_tiers: page.render_tiers,
      root: page.root,
      steps: page.steps,
      years: [{
        year: 2026, label: "FY 2025-26", stem: "fund-flows",
        path: "data/fund-flows.json", basis: "adopted",
        hero: { label: "l", value: "v", note: "n", kind: "hero" },
        figures: [],
        // THE YEAR'S CAVEAT REFS, which is what caveatHref looks an id up in --
        // the anchor is per (document, caveat) and the packager is the only
        // party that knows which stem the year came from, so the client
        // composes nothing. Ids taken from testdata/fund-flows.golden.json, the
        // document these checks fetch.
        caveats: [
          { id: "constraint-tier-is-our-reading", summary: "s", href: "caveats.html#caveat-fund-flows--constraint-tier-is-our-reading" },
          { id: "mixed-grain-double-counts", summary: "s", href: "caveats.html#caveat-fund-flows--mixed-grain-double-counts" },
          { id: "only-the-general-fund-is-decomposed", summary: "s", href: "caveats.html#caveat-fund-flows--only-the-general-fund-is-decomposed" },
        ],
        counts: { facts: 280, nodes: 145, links: 175 },
        chart_title: "Sankey diagram of the FY 2025-26 adopted budget",
      }],
      docs: {},
    }, overrides || {}),
  });
}

/** A page opened through main(), with the DOM seams the repaint needs. */
async function opened(page, overrides) {
  const app = appFor(page, overrides);
  const body = app.dom.document.node();
  app.dom.document.getElementById("flow-table").selectable = { tbody: body };
  const main = app.dom.document.node();
  app.dom.document.plant("main", main);
  await settle();
  return { app, body, main };
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
 * that was already there -- the defect the `=== n.id` guard below was written
 * for, which a three-outcome return now states in words.
 */
async function openInto(app, id) {
  const outcome = await app.drillDown(id);
  await settle();
  return outcome;
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

export async function checks() {
  const out = [];
  const raw = goldenFundFlows();

  for (const page of PAGES) {
    const { app, body } = await opened(page);
    const before = shown(app, body);

    out.push({
      name: `${page.name}: the overview draws, and its counts line describes it`,
      ok: before.counts === `${page.overview.links} flows between ${page.overview.nodes} nodes, from ${page.overview.facts} of the document's 280 facts` &&
          before.rows === page.overview.links &&
          before.crumbHidden,
      detail: `counts "${before.counts}", ${before.rows} table rows, breadcrumb ` +
              (before.crumbHidden ? "hidden" : "SHOWING with nothing opened"),
    });

    // EVERY DRILLABLE NODE, not a sample. The reason is the two defects above:
    // one showed up on every drill and one on none of them, and a sample would
    // have caught the first and missed the second.
    const openable = raw.nodes.filter((n) => n.tier === page.steps[0].from);
    const drawn = [];
    let refused = "";
    for (const n of openable) {
      try {
        // THE REAL ENTRY POINT, not a hook. drillDown is what a click calls,
        // so driving it exercises the repaint as well as the shaping -- and a
        // test-only setter in app.js would be behaviour the reader never runs.
        // FROM THE OVERVIEW EACH TIME: a rung is closed before the next is
        // opened, or the second node would be looked for in the first's chart.
        app.drillUp(0);
        const outcome = await openInto(app, n.id);
        // topOf(app) === n.id, NOT merely a truthy stack. drillDown swallows its
        // own throw and restores the PREVIOUS stack, so a truthiness test
        // passes from the second node onward and measure() silently records
        // the chart that was already there. Proved: injecting a throw for one
        // fund group into filterToNode left the whole make js run PASS. A loop
        // over 23 nodes whose guard only works on the first is worse than a
        // loop over one.
        if (outcome !== "drew" || topOf(app) !== n.id) {
          throw new Error("drillDown " + outcome + " and left the chart on " +
            (topOf(app) || "the overview"));
        }
        drawn.push(Object.assign({ id: n.id }, measure(app, app.projection)));
      } catch (e) {
        refused = n.id + ": " + (e && e.message ? e.message : String(e));
        break;
      }
    }
    app.drillUp(0);

    out.push({
      name: `${page.name}: every node the page offers to open draws when opened`,
      ok: refused === "" && drawn.length === openable.length && drawn.length > 0,
      detail: refused
        ? `refused ${refused}`
        : `${drawn.length} of ${openable.length} opened; smallest ribbon over all of them ` +
          `${Math.min(...drawn.map((d) => d.smallest)).toFixed(3)}px`,
    });

    // THE CAP IS THE POINT OF THIS FILE, and it does not engage on both pages.
    // fisc-ppkq says rescaling to a group's own total is what makes its funds
    // legible; measured, it is not, and the cap is what is -- but only where a
    // column is wide enough to need one. Revenue's special-revenue group has 32
    // funds and the cap folds 24 of them; Spending's widest division spends on
    // two object categories and the cap never fires at all.
    //
    // BOTH FACTS ARE PINNED, not just the first. A check that asserted the cap
    // engages everywhere would fail on Spending for being right, and one that
    // asserted it nowhere would go quiet the day a division gains a ninth
    // category and the column starts folding without anyone deciding to.
    const worst = drawn.find((d) => d.id === page.worst);
    const uncapped = await (async () => {
      const wide = Object.assign({}, page, {
        steps: [Object.assign({}, page.steps[0], {
          caps: page.steps[0].caps.map((c) => ({ tier: c.tier, cap: 1000 })),
        })],
      });
      const { app: noCap } = await opened(wide, { steps: wide.steps });
      await openInto(noCap, page.worst);
      // THE SAME GUARD THE MAIN LOOP HAS, and it was missing here. drillDown
      // swallows its own throw, so a refusal would leave this measuring the
      // OVERVIEW -- and Revenue's engaged test would still pass, because 22
      // sub-pixel ribbons is fewer than 29 links. A baseline that can silently
      // become a different chart is not a baseline.
      if (topOf(noCap) !== page.worst) {
        throw new Error("the uncapped baseline could not open " + page.worst);
      }
      return measure(noCap, noCap.projection);
    })();
    const engaged = Boolean(worst) && worst.links < uncapped.links;
    out.push({
      name: `${page.name}: the cap ${page.capEngages ? "is what makes the worst column drawable" : "is inert, because no column is wide enough to need it"}`,
      ok: Boolean(worst) && engaged === page.capEngages &&
          (page.capEngages
            ? worst.hairlines < uncapped.hairlines && worst.hairlines <= 2
            : worst.hairlines === uncapped.hairlines),
      detail: worst
        ? `${page.worst} capped: ${worst.links} ribbons, ${worst.hairlines} under 1px; ` +
          `uncapped: ${uncapped.links} ribbons, ${uncapped.hairlines} under 1px; the cap ` +
          `${engaged ? "folded a tail" : "folded nothing"}`
        : `${page.worst} is not a node this page opens`,
    });
  }

  // THE CAP IS READ OFF THE TIER IT NAMES, NOT OFF ITS POSITION. The packager
  // ships a step's caps as a list in declaration order, and a step may cap a
  // coarse tier before its fine one. Two decoys, each refuting one wrong
  // reading: caps listed coarse-first must draw exactly the declared page, so
  // a client taking caps[0] folds Revenue's fund column at the wrong number
  // and fails here; and a step capping ONLY a coarse tier must draw its fine
  // column whole, so a client taking any cap it finds folds when nothing
  // asked it to.
  {
    const page = PAGES[0];
    const step = page.steps[0];
    const fine = step.tiers[step.tiers.length - 1];
    const declared = step.caps.find((c) => c.tier === fine);
    const opensWorst = async (caps) => {
      const steps = [Object.assign({}, step, { caps })];
      const { app } = await opened(Object.assign({}, page, { steps }), { steps });
      await openInto(app, page.worst);
      if (topOf(app) !== page.worst) {
        throw new Error("the cap-order check could not open " + page.worst);
      }
      return measure(app, app.projection);
    };
    const asDeclared = await opensWorst(step.caps);
    const coarseFirst = await opensWorst([{ tier: step.tiers[0], cap: 1000 }, declared]);
    const coarseOnly = await opensWorst([{ tier: step.tiers[0], cap: declared.cap }]);
    const uncapped = await opensWorst([]);
    out.push({
      name: `${page.name}: a step's cap is looked up by the tier it names, not by its position`,
      ok: Boolean(declared) && page.capEngages &&
          coarseFirst.links === asDeclared.links &&
          coarseOnly.links === uncapped.links &&
          uncapped.links > asDeclared.links,
      detail: `${page.worst} at the declared caps: ${asDeclared.links} ribbons; coarse tier ` +
        `listed first: ${coarseFirst.links}; only the coarse tier capped: ${coarseOnly.links}; ` +
        `no caps: ${uncapped.links}`,
    });
  }

  // EVERY MARK KNOWS ITS FUND GROUP, which is what colours it. Built from the
  // DRAWN nodes alone this returned "" for every node on Spending's overview
  // and on all six opened Revenue views -- filterToNode keeps only what the
  // drawn tiers need, so a fund's fund-group ancestor is absent and the walk
  // stops at the first parent it cannot resolve. The whole of spending.html and
  // every drilled chart rendered in --muted, and nothing caught it: fold.mjs's
  // palette check runs at {0,2,4,5}, a tier set no view declares.
  // A REVENUE SOURCE BELONGS TO NO FUND GROUP and correctly resolves to "" --
  // it is money arriving, not money held. What must resolve is anything on the
  // fund side of the hierarchy, which is what carries a hue.
  const onTheFundSide = (/** @type {{id: string}} */ n) =>
    ["fund-group/", "fund/", "dept/", "expenditure/", "aggregate/"]
      .some((p) => n.id.startsWith(p));

  for (const page of PAGES) {
    const { app } = await opened(page);
    const unresolved = (/** @type {{nodes: any[]}} */ d) => {
      // layOut assigns the index fundGroupOf walks, so it has to have run over
      // this document before the question can be asked at all.
      app.layOut(d);
      return d.nodes.filter((n) => onTheFundSide(n) && app.fundGroupOf(n) === "");
    };
    const bad = unresolved(app.projection).map((n) => n.id);
    const openable = raw.nodes.filter((n) => n.tier === page.steps[0].from);
    for (const n of openable) {
      app.drillUp(0);
      await openInto(app, n.id);
      bad.push(...unresolved(app.projection).map((x) => n.id + ">" + x.id));
    }
    app.drillUp(0);
    const groups = [...new Set(app.projection.nodes.map((n) => app.fundGroupOf(n)))]
      .filter(Boolean).sort();
    out.push({
      name: `${page.name}: every mark on the fund side knows its group, opened or not`,
      ok: bad.length === 0 && groups.length > 0,
      detail: bad.length
        ? `${bad.length} mark(s) resolve to no group: ${bad.slice(0, 4).join(", ")}`
        : `overview draws ${JSON.stringify(groups)}, and all ${openable.length} opened ` +
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
  const revenue = PAGES[0];
  const { app: capApp } = await opened(revenue);
  await openInto(capApp, revenue.worst);
  const fineTier = revenue.steps[0].tiers[revenue.steps[0].tiers.length - 1];
  const agg = capApp.projection.nodes.find((n) => n.id === capApp.aggregateID(fineTier));
  out.push({
    name: "revenue: the capped tail is marked as ours, not as something the city printed",
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
      : "no aggregate node: the cap folded nothing on the page it is needed for",
  });
  capApp.drillUp(0);

  // NO AGGREGATE ANYWHERE COVERS FEWER THAN TWO. The check above looks at one
  // group on one page; this looks at every opened view on both, because the
  // shape that shipped -- a column of exactly cap + 1 -- occurs on precisely
  // one of the 29 and would be invisible to a sample.
  const ones = [];
  for (const page of PAGES) {
    const { app } = await opened(page);
    for (const n of raw.nodes.filter((x) => x.tier === page.steps[0].from)) {
      app.drillUp(0);
      await openInto(app, n.id);
      if (topOf(app) !== n.id) continue;
      for (const a of app.projection.nodes.filter((x) => app.isAggregate(x.id))) {
        if (Number(a.label.split(" ")[0]) < 2) ones.push(n.id + ": " + a.label);
      }
    }
    app.drillUp(0);
  }
  out.push({
    name: "no opened view folds a single printed figure into an aggregate of one",
    ok: ones.length === 0,
    detail: ones.length
      ? ones.join("; ")
      : "every aggregate across all 29 opened views covers two or more",
  });

  // A CAVEAT ABOUT A NODE REACHES THAT NODE, THROUGH THE FOLD. applies_to names
  // ids in the FILE and a drawn mark is often a fold of several of them, so a
  // direct id match would leave the badge silent on every page that folds --
  // and a check asserting "no badge" would pass whether the caveat does not
  // apply or the resolution is broken. fund-flows' only-the-general-fund
  // caveat names fund/100, which spending.html draws directly and revenue.html
  // folds into fund-group/general.
  const seen = [];
  for (const page of PAGES) {
    const { app } = await opened(page);
    app.layOut(app.projection);
    const marked = app.projection.nodes.filter((n) => app.caveatsFor(n.id).length > 0);
    seen.push({ page: page.name, ids: marked.map((n) => n.id) });
  }
  const revenueMarks = seen[0].ids;
  const spendingMarks = seen[1].ids;
  out.push({
    name: "a caveat about one node reaches that node on both pages, folded or not",
    ok: revenueMarks.includes("fund-group/general") && spendingMarks.includes("fund/100") &&
        revenueMarks.length > 0 && spendingMarks.length > 0,
    detail: `revenue marks ${JSON.stringify(revenueMarks)}; ` +
            `spending marks ${JSON.stringify(spendingMarks)}`,
  });

  // THE BADGE HAS TO REACH THE READER, not merely be computable. caveatsFor
  // resolving correctly and showTip/pin never calling it are indistinguishable
  // from every other check here -- proved by stubbing caveatsFor out of both,
  // which left make js at 77 of 77. So these drive the two renderers and read
  // back what the DOM was told to show.
  for (const page of PAGES) for (const at of page.marked) {
    const { app } = await opened(page);
    if (at.open) await openInto(app, at.open);
    if (topOf(app) !== at.open) throw new Error("could not open " + at.open + " on " + page.name);
    const where = `${page.name}${at.open ? " opened into " + at.open : ""}`;
    app.layOut(app.projection);
    const marked = app.projection.nodes.find((n) => app.caveatsFor(n.id).length > 0);
    if (!marked) {
      out.push({
        name: `${where}: a marked node reaches the tooltip and the panel`,
        ok: false,
        detail: "no node at this depth carries a caveat, so this asserts nothing",
      });
      continue;
    }
    // showTip needs a laid node -- it reads .value and positions from a box --
    // so it gets the one layOut produced rather than the folded one.
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
    const href = app.caveatHref(app.caveatsFor(marked.id)[0].id);
    out.push({
      name: `${where}: a marked node reaches the tooltip and the panel`,
      // THE SHARE IS ASSERTED WITH ITS DERIVED MARKING, not merely present. A
      // share is arithmetic over two printed figures and sits in a chip row
      // beside "printed by the city", which is the one adjacency this project's
      // premise is about -- so "of this column" alone would pass on a chip that
      // had quietly lost the thing that says whose number it is.
      // THE SHARE IS EXPECTED WHERE THE COLUMN DIVIDES AND NOWHERE ELSE.
      // revenue.html's marked node is one of six fund groups, so it gets one;
      // spending.html's is fund/100, alone in its column, where a share would
      // read "our 100.0% of this column" -- a derived chip carrying a figure
      // that is 100% by construction. Asserting "a share appears" would have
      // demanded the second, and asserting nothing would have missed the first.
      //
      // WHERE IT DOES APPEAR IT CARRIES ITS DERIVED MARKING, not merely the
      // words "of this column": a share is arithmetic over two printed figures
      // and sits beside "printed by the city", which is the one adjacency this
      // project's premise is about.
      ok: tip.includes("caveat") &&
          (at.sharesColumn
            ? tip.includes("\u25c7 our ") && tip.includes("of this column")
            : !tip.includes("of this column")) &&
          panel.includes("Read it in full") && href.startsWith("caveats.html#caveat-"),
      detail: `${marked.id}: tooltip mentions ${tip.includes("caveat") ? "a caveat" : "NO caveat"} and ` +
              `${tip.includes("of this column")
                ? (tip.includes("\u25c7 our ") ? "a share marked as ours" : "an UNMARKED share")
                : "no share, which is right for a column of one"}; panel ` +
              `${panel.includes("Read it in full") ? "links to the full text" : "does NOT link"}; ` +
              `href "${href}"`,
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
    const page = PAGES[0];
    const { app } = await opened(page);
    await openInto(app, page.worst);
    const aggID = app.aggregateID(page.steps[0].tiers[page.steps[0].tiers.length - 1]);
    const agg = app.projection.nodes.find((n) => n.id === aggID);
    const drawn = new Set(app.projection.nodes.map((n) => n.id));
    // Every id the aggregate claims is one the drawn document does NOT carry --
    // that is what "swallowed" means -- and caveatsFor resolves each to the
    // aggregate.
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
    // And a node the aggregate did NOT swallow must not match, or "resolves"
    // would mean "matches everything".
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
          `${spurious.length === 0 ? "does not" : "WRONGLY DOES"}`        : "no aggregate on the page whose column the cap is for",
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
    const { app } = await opened(PAGES[0]);
    const node = (/** @type {string} */ id, /** @type {number} */ tier,
      /** @type {string} */ parent) =>
      ({ id, label: id, tier, parent, constraint_tier: "", role: "", derived: false,
        rationale: "", source_note: "" });
    const link = (/** @type {string} */ a, /** @type {string} */ b,
      /** @type {number} */ v) =>
      ({ source: a, target: b, value_cents: v, kind: "external", transfer_id: "",
        fact_ids: [], locators: [], derived: false });
    // Three funds against a cap of 1, so two are folded -- and the smallest
    // carries a child, which is the shape the shipped columns never produce.
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
  // EVERY OPENED VIEW OF BOTH PAGES, and both overviews, because the shape
  // occurs on one column of one year and a sample would miss it.
  {
    const hundreds = [];
    for (const page of PAGES) {
      const { app } = await opened(page);
      const scan = (/** @type {string} */ where) => {
        for (const n of app.layOut(app.projection).nodes) {
          const share = app.columnShare(n);
          if (share.includes("100.0%") || share.includes("100%")) {
            hundreds.push(where + " " + n.id + ": " + share);
          }
        }
      };
      scan(page.name + " overview");
      for (const n of raw.nodes.filter((x) => x.tier === page.steps[0].from)) {
        app.drillUp(0);
        await openInto(app, n.id);
        if (topOf(app) !== n.id) continue;
        scan(page.name + " opened " + n.id);
      }
      app.drillUp(0);
    }
    // AND THE CASE THE SHIPPED FIXTURE CANNOT REACH. The rounding happens on
    // fund-flows-2024-actual, opened on debt-service, and these checks fetch the
    // FY2025-26 golden -- the only fund-flows document committed. Removing the
    // ceiling left the scan above green for that reason alone, which is a check
    // passing because its fixture is the wrong year. So the split is built:
    // 99.9943% of a two-node column, the real proportion, laid out by the real
    // layOut.
    const app = appFor(PAGES[0]);
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
      ok: hundreds.length === 0 && !rounded.includes("100"),
      detail: hundreds.length
        ? hundreds.slice(0, 3).join("; ")
        : `every share across both overviews and all 29 opened views is under 100%; ` +
          `a 99.9943% mark of a two-node column reads "${rounded}"`,
    });
  }

  // THE ROOT, WHICH IS NOT A NARROWING BUT THE THING THAT DRAWS AT ALL. Spending
  // draws tiers {3,4} of a document carrying eleven tier-0 revenue nodes, and
  // foldDocument refuses a node it cannot place. Without a root the page is a
  // banner, not a smaller chart.
  const spending = PAGES[1];
  const rootless = appFor(Object.assign({}, spending, { root: "" }), { root: "" });
  let rootlessErr = "";
  try {
    rootless.shapeFor(raw);
  } catch (e) {
    rootlessErr = e && e.message ? e.message : String(e);
  }
  out.push({
    name: "spending: without its root the page refuses rather than drawing half a chart",
    ok: rootlessErr.includes("no ancestor of it is a tier this page draws"),
    detail: rootlessErr || "NO REFUSAL: {3,4} over the whole document drew something",
  });

  // THE BREADCRUMB IS THE ONLY ALWAYS-VISIBLE WAY BACK, because opening a node
  // removes it from the chart -- there is nothing left to click again.
  const { app: revApp, body: revBody } = await opened(PAGES[0]);
  await openInto(revApp, "fund-group/general");
  const opened1 = shown(revApp, revBody);
  const stillThere = revApp.projection.nodes.some((n) => n.id === "fund-group/general");
  revApp.drillUp(0);
  const closed = shown(revApp, revBody);
  out.push({
    name: "revenue: opening a node shows the way back, and closing it puts the overview back",
    ok: !opened1.crumbHidden && opened1.crumbText.includes("All fund groups") &&
        opened1.crumbText.includes("General Fund") &&
        !stillThere &&
        closed.crumbHidden && closed.counts === before0(PAGES[0]),
    detail: `opened: breadcrumb "${opened1.crumbText}", counts "${opened1.counts}", ` +
            `the opened node is ${stillThere ? "STILL DRAWN" : "gone from the chart"}; ` +
            `closed: counts "${closed.counts}"`,
  });

  // THE FLOW TABLE SHIPS CLOSED, so the chart's <desc> is the only route to it
  // a reader who cannot see the page has -- a closed <details> is out of the
  // accessibility tree until it is opened. Before that fold the table shipped
  // open on the chart pages and this sentence was a convenience; it is now the
  // pointer, and a drill used to replace the whole description with one naming
  // no table.
  //
  // THE EXPECTATION IS THE SERVED SENTENCE, not a literal. The check reads the
  // description the page shipped, takes its last sentence the way app.js does,
  // and asserts the drilled description still ends with THAT -- so a reworded
  // template moves both sides together and this cannot pin the client to the
  // checker.
  // THE SERVED SENTENCE IS READ FROM THE TEMPLATE, not typed here, for the
  // reason year.mjs reads the counts span the same way: an expectation spelled
  // as a literal is a third copy of a sentence the template owns, and it pins
  // the client to the checker while the template drifts free. The stub ships an
  // empty <desc>, so what the browser would have been served is planted.
  const descApp = appFor(PAGES[0]);
  const descEl = descApp.dom.document.getElementById("chart-desc");
  const served = templateDesc("chart.html.tmpl", DESCRIPTION);
  descEl.textContent = served;
  const descBody = descApp.dom.document.node();
  descApp.dom.document.getElementById("flow-table").selectable = { tbody: descBody };
  descApp.dom.document.plant("main", descApp.dom.document.node());
  await settle();

  const descOf = () => String(descEl.textContent).replace(/\s+/g, " ").trim();
  // THE POINTER IS THE TEMPLATE'S SUFFIX, taken by subtracting the description
  // the packager supplied -- not by re-splitting the sentence the way app.js
  // does. A second copy of that split here IS the client's implementation
  // asserted against itself: measured, a description closing with "!" made this
  // check fail while app.js was doing the right thing. Whether the template's
  // pointer is one sentence, and the last, is the Go side's claim
  // (TestAClosedFlowTableIsNotDescribedAsListedBelow).
  const pointer = served.slice(DESCRIPTION.length).trim();
  await openInto(descApp, "fund-group/general");
  const drilledDesc = descOf();
  descApp.drillUp(0);
  const restored = descOf();
  out.push({
    name: "revenue: opening a node keeps the chart description's pointer to the flow table",
    ok: pointer !== "" && pointer.toLowerCase().includes("table") &&
        drilledDesc.endsWith(pointer) && drilledDesc !== served &&
        restored === served,
    detail: `the served description ends "${pointer}"; drilled it reads ` +
            `"${drilledDesc}"; closing the drill ` +
            `${restored === served ? "restores it" : "does NOT restore it"}`,
  });
  void descBody;

  // ---------------------------------------------------------------- the chain
  //
  // THE MERGED CHAIN, DRIVEN BEFORE ANY PAGE SHIPS IT. Every arm above runs a
  // one-step chain over one document, which is what the site declares today.
  // The stack exists for fisc-ko1j.4's chain -- the spine opening into
  // fund-flows, and that into divisions -- and the machinery that pays for it
  // is only witnessed by walking it: 0 -> 1 -> 2 -> 1 -> 0 through the real
  // entry points, over the two committed documents at once.
  //
  // THE SHAPE IS fisc-ko1j.4's DECLARATION, copied for drill.mjs's reason: no
  // seam joins a Go view to a node check, so the copy is the claim. When the
  // merge lands, PAGES and this become the same list.
  for (const walk of [await walkChain()]) out.push(...walk);

  // FOUR REFUSAL PATHS, EACH WITH ITS NEW CALLER. isDocument, understands,
  // drawableSankey and the fetch's own two failures had exactly one caller --
  // showYear -- and drillDown is the second. A click that reached a guard
  // showYear did not, or skipped one it did, would draw at depth 1 a file the
  // year control refuses at depth 0, and none of the year arms could tell.
  // Each arm here plans one failure for the step document and asserts the
  // drill FAILED, the reader was told in the words that name the fault, the
  // stack is still empty, and the spine's own sentence is still on screen.
  for (const refusal of [
    { name: "a 404", plan: { ok: false, status: 404 }, says: "HTTP 404" },
    { name: "a body that is not JSON", plan: { badBody: true }, says: "not valid JSON" },
    {
      name: "a document at the wrong schema_version",
      plan: { doc: Object.assign({}, goldenFundFlows(), { schema_version: 2 }) },
      says: "declares schema_version 2",
    },
    { name: "a null body", plan: { doc: null }, says: "not a document at all" },
  ]) {
    const { app, fetch, main, body } = await chainOpened({ "data/fund-flows.json": refusal.plan });
    const before = shown(app, body);
    const outcome = await openInto(app, "fund-group/general");
    const after = shown(app, body);
    const banners = refusals(main).map((b) => b.textContent);
    out.push({
      name: `the drill refuses ${refusal.name} for its step document, in words that name the fault`,
      ok: outcome === "failed" && app.drilled.length === 0 &&
          banners.length === 1 && banners[0].includes(refusal.says) &&
          after.counts === before.counts && after.crumbHidden &&
          fetch.asked.includes("data/fund-flows.json"),
      detail: `drillDown came to "${outcome}" with ${app.drilled.length} rung(s) on the stack; ` +
        `${banners.length} banner(s)${banners.length ? `, reading "${banners[0].slice(0, 90)}..."` : ""}; ` +
        `the counts line still reads "${after.counts}"`,
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

/**
 * fisc-ko1j.4's chain, copied: the spine's tier 2 opens into fund-flows at
 * {0,3,4}, and a division there opens into {4,5}.
 */
const CHAIN = [
  { from: 2, projection: "fund-flows", tiers: [0, 3, 4], caps: [{ tier: 3, cap: 8 }, { tier: 4, cap: 24 }],
    back: "All fund groups", tail: "funds" },
  { from: 4, tiers: [4, 5], caps: [{ tier: 5, cap: 8 }], back: "All divisions", tail: "categories" },
];

/**
 * A spine page carrying the chain, opened through main() over the two
 * committed documents -- with `plan` overriding what any path answers.
 */
async function chainOpened(plan) {
  const config = twoYearConfig();
  config.projections["fund-flows"] = "data/fund-flows.json";
  config.years = [Object.assign({}, config.years[0], {
    counts: { facts: 120, nodes: 25, links: 58 },
    chart_title: "Sankey diagram of the FY 2025-26 adopted budget",
  })];
  config.steps = CHAIN;
  // THE SPINE'S LABEL FOR THE GROUP IS MADE DISTINCT, because both committed
  // documents print "General Fund" for fund-group/general and a rung named
  // from the wrong document would be invisible. The breadcrumb, the chart
  // name and the hint name the node the reader clicked in the words of the
  // chart they clicked it on -- the spine's -- and not the step document's.
  const spine = goldenGraph();
  const group = spine.nodes.find((n) => n.id === "fund-group/general");
  group.label = "General Fund group";
  const fetch = plannedFetch(Object.assign({
    "data/sankey.json": { doc: spine },
    "data/fund-flows.json": { doc: goldenFundFlows() },
  }, plan || {}));
  const app = loadApp({ config, fetch });
  const body = app.dom.document.node();
  app.dom.document.getElementById("flow-table").selectable = { tbody: body };
  const main = app.dom.document.node();
  app.dom.document.plant("main", main);
  await settle();
  return { app, fetch, main, body };
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
 * THE SERVED DESCRIPTION IS PLANTED so the table pointer exists to be kept:
 * the stub ships an empty <desc>, and an empty pointer makes "ends with the
 * pointer" true of any string.
 */
async function walkChain() {
  const out = [];
  const { app, fetch, body } = await chainOpened();
  const desc = app.dom.document.getElementById("chart-desc");
  const served = "A chart of everything. The flows are listed in a table below.";
  desc.textContent = served;
  const pointer = "The flows are listed in a table below.";

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
  // EVERY FUND-SIDE MARK RESOLVES ITS GROUP AGAINST THE DRAWN DOCUMENT. layOut
  // merges the fetched hierarchy over the drawn nodes for the hue walk, and
  // across a document switch "fetched" has to mean the rung's file: merged
  // from the year's spine instead, fund/100 is a drawn node whose parent the
  // fold blanked, the spine has no such node to override it, and the walk
  // stops at "" -- every mark in --muted, the failure fundGroupOf's comment
  // records, reached from the other document.
  const muted1 = app.projection.nodes
    .filter((n) => !n.id.startsWith("revenue/") && app.fundGroupOf(n) === "").map((n) => n.id);
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

  out.push({
    name: "chain: the step document is fetched on the first drill and not before",
    ok: asked0.length === 1 && !asked0.includes("data/fund-flows.json") &&
        asked1.length === 2 && asked1[1] === "data/fund-flows.json",
    detail: `main() asked for ${JSON.stringify(asked0)}; the first drill added ` +
      `${JSON.stringify(asked1.slice(asked0.length))}`,
  });
  out.push({
    name: "chain: depth 1 draws the General Fund at {0,3,4} from the other document, and every sentence says so",
    // 34 nodes and 33 links is fisc-ko1j's own measurement of this view,
    // reproduced here through the shipped functions.
    ok: open1 === "drew" && at1.depth === 1 && !at1.drawnIsYears &&
        Boolean(m1) && m1.nodes === 34 && m1.links === 33 &&
        at1.counts === `33 flows between 34 nodes, from ${cited1.size} of the document's 280 facts` &&
        rows1 === 33 &&
        at1.title === "Sankey diagram of the FY 2025-26 adopted budget, opened into General Fund group" &&
        at1.crumbControls.join("|") === "← All fund groups" && at1.crumbHere === "General Fund group" &&
        at1.hint.includes("open it further") && !at1.hint.includes("Nothing here opens") &&
        at1.legend === 0 && at0.legend === 6 &&
        at1.desc.endsWith(pointer) && at1.desc.startsWith("General Fund group on the left") &&
        at1.hint.startsWith("This is General Fund group, ") &&
        patrolOpens && Boolean(fund100) && !fund100Opens && muted1.length === 0 &&
        focus1 === "← All fund groups",
    detail: open1 === "drew"
      ? `${m1.nodes} nodes, ${m1.links} links; counts "${at1.counts}"; title "${at1.title}"; ` +
        `breadcrumb ${JSON.stringify(at1.crumbControls)} + "${at1.crumbHere}"; hint "${at1.hint}"; ` +
        `legend ${at0.legend} -> ${at1.legend} swatches; a division ` +
        `${patrolOpens ? "opens" : "does NOT open"} and a fund ` +
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
        at2.hint.startsWith("This is Patrol, broken into its parts. Nothing here opens further") &&
        at2.desc.endsWith(pointer) && !anyOpens2 && at2.legend === 0 &&
        focus2 === "← All divisions",
    detail: open2 === "drew"
      ? `${m2.nodes} nodes, ${m2.links} links, smallest ribbon ${m2.smallest.toFixed(2)}px; title ` +
        `"${at2.title}"; breadcrumb ${JSON.stringify(at2.crumbControls)} + "${at2.crumbHere}"; ` +
        `hint "${at2.hint}"; ${anyOpens2 ? "SOMETHING still opens" : "nothing opens"}; ` +
        `no second fetch; focus on "${focus2}"`
      : `opening Patrol came to "${open2}"`,
  });
  out.push({
    name: "chain: Escape closes one rung at a time, and each depth comes back as it was",
    ok: back1.depth === 1 && back1.counts === at1.counts && back1.title === at1.title &&
        back1.crumbControls.join("|") === at1.crumbControls.join("|") &&
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
  return out;
}

/**
 * A stand-in for the ChartDescription a view supplies.
 *
 * TERMINATED, because export.View.validate refuses one that is not: app.js
 * separates this from the template's own sentence after it, and an unterminated
 * description runs into it.
 *
 * AND TERMINATED WITH "!", NOT ".", WHICH IS THE POINT. validate accepts three
 * terminators and lastSentence splits on all three; with a period here both the
 * broadened split and two of the three accepted terminators are unwitnessed --
 * measured, reverting lastSentence to ". " left this whole file green. A
 * fixture that uses the COMMON shape cannot see a check written for the
 * uncommon one, which is the shape that hid the defect this fixture is for.
 */
const DESCRIPTION = "A chart of something!";

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

/** The counts line a page's overview shows, composed the way paintCounts does. */
function before0(page) {
  return `${page.overview.links} flows between ${page.overview.nodes} nodes, ` +
    `from ${page.overview.facts} of the document's 280 facts`;
}
