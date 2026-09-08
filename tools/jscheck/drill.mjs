// drill.mjs — the chart opening one node, measured rather than assumed.
//
// WHY THIS FILE EXISTS. The drill shipped with `go build`, `go test ./...`,
// `make js` and `fisc verify` all green and NOT ONE CHECK touching it: not
// drillTo, not filterToNode, not capColumn, not drillable, not paintBreadcrumb,
// and neither of the two tier sets the site actually declares. Grepping for the
// names is what turned that up, which is the cheapest way and the one that
// should not have been necessary.
//
// Two defects in the same range were reachable only by measurement: layOut
// aligned columns on RENDER_TIERS while a drilled document is
// folded to DRILL.tiers, so d3-sankey died inside its own ordering pass; and
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

import { loadApp, goldenFundFlows, plannedFetch, settle, repoRoot } from "./harness.mjs";

/**
 * The two views the site ships, verbatim from pkg/cmd/export/data.go's views().
 *
 * COPIED RATHER THAN IMPORTED because there is no seam: views() is Go and this
 * is node. So the copy is a claim, and TestViewsOpensOnTheSpineAndGivesYears
 * ToItAlone is what keeps it honest from the other side -- it asserts these
 * exact tier sets and these exact Drill values off the real view list, field by
 * field.
 *
 * IT DID NOT USED TO. That test asserted only that RenderTiers was non-empty
 * and Drill non-nil, which left this file free to measure a configuration no
 * page ships: change a cap or a tier set in data.go and every gate stayed green
 * while these checks went on pinning the old one. If you change either, both
 * sides go red and that is the point.
 */
const PAGES = [
  {
    name: "revenue",
    render_tiers: [0, 2],
    root: "",
    drill: { from: 2, tiers: [0, 3], back: "All fund groups", tail: "funds", cap: 8 },
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
    // Its marked node is one of six fund groups, so a share of the column is
    // a real quantity.
    sharesColumn: true,
  },
  {
    name: "spending",
    render_tiers: [3, 4],
    // WITHOUT THIS THE PAGE DRAWS NOTHING. See the root check below.
    root: "fund/100",
    drill: { from: 4, tiers: [4, 5], back: "All divisions", tail: "categories", cap: 8 },
    // Measured: the General Fund into its 23 divisions.
    overview: { nodes: 24, links: 23, facts: 49 },
    // NO DIVISION SPENDS ON MORE THAN A HANDFUL OF OBJECT CATEGORIES, so the
    // cap never fires here. Pinned so that stops being true loudly.
    worst: "dept/patrol",
    capEngages: false,
    // Its marked node is fund/100, alone in its column: a share there is 100%
    // by construction and is suppressed.
    sharesColumn: false,
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
      drill: page.drill,
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
    const openable = raw.nodes.filter((n) => n.tier === page.drill.from);
    const drawn = [];
    let refused = "";
    for (const n of openable) {
      try {
        // THE REAL ENTRY POINT, not a hook. drillTo is what a click calls, so
        // driving it exercises the repaint as well as the shaping -- and a
        // test-only setter in app.js would be behaviour the reader never runs.
        app.drillTo(n.id);
        // === app.drilledInto === n.id, NOT merely truthy. drillTo swallows its
        // own throw and restores drilledInto to the PREVIOUS node id, so a
        // truthiness test passes from the second node onward and measure()
        // silently records the chart that was already there. Proved: injecting
        // a throw for one fund group into filterToNode left the whole make js
        // run PASS. A loop over 23 nodes whose guard only works on the first is
        // worse than a loop over one.
        if (app.drilledInto !== n.id) {
          throw new Error("drillTo refused and left the chart on " +
            (app.drilledInto || "the overview"));
        }
        drawn.push(Object.assign({ id: n.id }, measure(app, app.projection)));
      } catch (e) {
        refused = n.id + ": " + (e && e.message ? e.message : String(e));
        break;
      }
    }
    app.drillTo("");

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
        drill: Object.assign({}, page.drill, { cap: 1000 }),
      });
      const { app: noCap } = await opened(wide, { drill: wide.drill });
      noCap.drillTo(page.worst);
      // THE SAME GUARD THE MAIN LOOP HAS, and it was missing here. drillTo
      // swallows its own throw, so a refusal would leave this measuring the
      // OVERVIEW -- and Revenue's engaged test would still pass, because 22
      // sub-pixel ribbons is fewer than 29 links. A baseline that can silently
      // become a different chart is not a baseline.
      if (noCap.drilledInto !== page.worst) {
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
    const openable = raw.nodes.filter((n) => n.tier === page.drill.from);
    for (const n of openable) {
      app.drillTo(n.id);
      bad.push(...unresolved(app.projection).map((x) => n.id + ">" + x.id));
    }
    app.drillTo("");
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
  capApp.drillTo(revenue.worst);
  const agg = capApp.projection.nodes.find((n) => n.id === "aggregate/tail");
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
  capApp.drillTo("");

  // NO AGGREGATE ANYWHERE COVERS FEWER THAN TWO. The check above looks at one
  // group on one page; this looks at every opened view on both, because the
  // shape that shipped -- a column of exactly cap + 1 -- occurs on precisely
  // one of the 29 and would be invisible to a sample.
  const ones = [];
  for (const page of PAGES) {
    const { app } = await opened(page);
    for (const n of raw.nodes.filter((x) => x.tier === page.drill.from)) {
      app.drillTo(n.id);
      if (app.drilledInto !== n.id) continue;
      const a = app.projection.nodes.find((x) => x.id === "aggregate/tail");
      if (a && Number(a.label.split(" ")[0]) < 2) ones.push(n.id + ": " + a.label);
    }
    app.drillTo("");
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
  for (const page of PAGES) {
    const { app } = await opened(page);
    app.layOut(app.projection);
    const marked = app.projection.nodes.find((n) => app.caveatsFor(n.id).length > 0);
    if (!marked) {
      out.push({
        name: `${page.name}: a marked node reaches the tooltip and the panel`,
        ok: false,
        detail: "no node on this page carries a caveat, so this asserts nothing",
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
      name: `${page.name}: a marked node reaches the tooltip and the panel`,
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
          (page.sharesColumn
            ? tip.includes("\u25c7 our ") && tip.includes("of this column")
            : !tip.includes("of this column")) &&
          panel.includes("Read it in full") && href.startsWith("caveats.html#caveat-"),
      detail: `tooltip mentions ${tip.includes("caveat") ? "a caveat" : "NO caveat"} and ` +
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
    app.drillTo(page.worst);
    const agg = app.projection.nodes.find((n) => n.id === "aggregate/tail");
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
    const found = app.caveatsFor("aggregate/tail").map((c) => c.id);
    // And a node the aggregate did NOT swallow must not match, or "resolves"
    // would mean "matches everything".
    app.projection.metadata.caveats = [{
      id: "probe", summary: "s", text: "t", applies_to: ["revenue/taxes/property"],
    }];
    const spurious = app.caveatsFor("aggregate/tail").map((c) => c.id);
    app.projection.metadata.caveats = before;

    app.drillTo("");
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
    const capped = app.capColumn(doc, 3, 1);
    const agg = capped.nodes.find((n) => n.id === "aggregate/tail");
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
      for (const n of raw.nodes.filter((x) => x.tier === page.drill.from)) {
        app.drillTo(n.id);
        if (app.drilledInto !== n.id) continue;
        scan(page.name + " opened " + n.id);
      }
      app.drillTo("");
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
  revApp.drillTo("fund-group/general");
  const opened1 = shown(revApp, revBody);
  const stillThere = revApp.projection.nodes.some((n) => n.id === "fund-group/general");
  revApp.drillTo("");
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
  descApp.drillTo("fund-group/general");
  const drilledDesc = descOf();
  descApp.drillTo("");
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

  return out;
}

/**
 * A stand-in for the ChartDescription a view supplies. Terminated, because
 * export.View.validate refuses one that is not -- app.js separates this from
 * the template's own sentence after it, and an unterminated description runs
 * into it.
 */
const DESCRIPTION = "A chart of something.";

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
