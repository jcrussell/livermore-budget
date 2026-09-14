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
  goldenSpending, goldenSpending2027, spendingGapDeclaration,
  stepDescriptions, stepShapes, spineRenderTiers,
  settle, refusals, twoYearConfig, repoRoot, residualDeclaration,
} from "./harness.mjs";

/**
 * The one chart page the site ships, verbatim from pkg/cmd/export/data.go's
 * views(): the spine, drawn whole, whose fund groups open into fund-flows and
 * whose divisions open into their object categories.
 *
 * NOTHING HERE THAT views() DECLARES IS SPELLED TWICE. The tier sets, the caps
 * and each step's `from` are read out of the Go source (stepShapes), the
 * descriptions likewise (stepDescriptions), and the residual set out of the
 * check that declares it (residualDeclaration).
 *
 * A COPY WOULD BE HELD BY NOTHING, and was. TestViewsOpensOnTheSpineAndGives
 * YearsToItAlone pins views() against a literal in the Go TEST file and reads
 * nothing in this directory, so a change made to data.go and to that literal
 * together -- which is how a real change is made -- left this file measuring
 * the client under a shape the site had stopped shipping. Measured twice, one
 * field set at a time: a reworded description (fisc-vsu8), and `{Tier: 3, Cap:
 * 8}` changed to 9, which left `go test` and `make js` green while jscheck went
 * on folding special-revenue under the old cap.
 *
 * WHAT IS STILL SPELLED HERE is what views() does NOT declare -- `back` and
 * `tail` are the packager's, but the stems and the per-year join are resolved
 * at build time and stepDocsFor below stands in for them.
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
const STEP_SHAPES = stepShapes();
// THE SPINE'S COLUMN ORDER, READ OFF data.go FOR THE SAME REASON THE SHAPES
// ARE. app.js aligns every column on this list and openableColumns names the
// columns in it, so a copy here would measure a chart the site does not draw
// -- and would place a kept flank on whichever side the copy happened to say.
const RENDER_TIERS = spineRenderTiers();

/**
 * One step as the packager ships it: the parsed shape -- key, after, side,
 * role, from, tiers, caps, keep, widen -- under the words views() declares
 * beside it.
 *
 * THE OPTIONAL FIELDS ARE COPIED ONLY WHERE THE LITERAL DECLARES THEM, which is
 * what the wire does: all four carry `omitempty`, so a config built here with
 * `keep: []` on every step would hand the client a shape data.go does not ship.
 * @param {number} i
 * @param {Record<string, any>} words
 */
function stepAs(i, words) {
  const shape = STEP_SHAPES[i];
  const step = { key: shape.key, after: shape.after, from: shape.from, tiers: shape.tiers,
    caps: shape.caps, description: STEP_DESCRIPTIONS[i] };
  if (shape.side) step.side = shape.side;
  if (shape.role) step.role = shape.role;
  if (shape.keep) step.keep = shape.keep;
  if (shape.widen) step.widen = shape.widen;
  return Object.assign(step, words);
}

const PAGE = {
  // FIVE STEPS AND A TREE, NOT A CHAIN OF FIVE: three of them are one chain --
  // a fund group opens into its funds, a fund into its divisions, a division
  // into what it spends on -- and two more open from the spine's own chart
  // (after ""), one tier each, which stepFor tells apart by tier and role.
  steps: [
    stepAs(0, { projection: "fund-flows", back: "All fund groups", tail: "funds",
      residual: residualDeclaration() }),
    stepAs(1, { back: "All funds", tail: "divisions" }),
    stepAs(2, { back: "All divisions", tail: "categories" }),
    stepAs(3, { projection: "fund-flows", back: "All revenue categories", tail: "lines" }),
    // THE LAST STEP OPENS THE SPINE'S RIGHT-HAND COLUMN, and its gap set is
    // read off internal/check the way the fund-group step's residual is: one
    // declaration, two readers, and no third spelling to drift.
    stepAs(4, { projection: "department-spending", back: "All object categories",
      tail: "divisions", gaps: spendingGapDeclaration() }),
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
  // HOW MANY VIEWS THE TREE OPENS, counted by walking it rather than by adding
  // the declarations up: ten revenue categories, six fund groups and four
  // object categories at depth 1; fund/100 at depth 2, ALONE, because the fund
  // step's role opens the General Fund and leaves the other sixty funds as the
  // ends of the chain; and its 23 divisions at depth 3. Neither a revenue
  // category's window nor an object category's offers anything at all, and
  // neither does a kept flank -- no step names those charts. Measured
  // 2026-09-13 over both published columns.
  openedViews: 44,
  // HOW MANY OF THEM DRAW A SECOND DOCUMENT'S RIBBONS: the ten revenue
  // categories, the six fund groups and the four object categories, all at
  // depth 1, each keeping a flank of the spine it was opened from. The fund
  // and division windows keep a flank too and it is NOT counted here, because
  // the document it comes off is the one they draw. This is the evidence for
  // splitting the counts line in two: nearly half the views the page opens
  // state both documents' figures, so folding one into a trailing clause built
  // for a residual would be a footnote over anything from a tenth of the
  // ribbons (Licenses & Permits) to twelve of thirteen (the General Fund).
  carryingViews: 20,
  // THE COLUMN ORDER THE SPINE DECLARES, off data.go rather than typed. Its
  // three tiers are the ones the document carries, so the fold it asks for
  // changes no mark; what it decides is which column is "left-hand" and which
  // tier a window could keep beside which.
  renderTiers: RENDER_TIERS,
};

/**
 * The columns of the spine's own chart that hold a node which opens, in the
 * order it declares them, as openableColumns names them.
 *
 * ALL THREE SINCE THE OBJECT-CATEGORY STEP, and it was "left-hand|middle" for
 * as long as the right-hand column opened nothing -- which is the owner's third
 * finding stated as a pin. One constant because three checks assert it and a
 * spine that stopped offering one of its columns should turn all three red at
 * once rather than one at a time.
 */
const OPENABLE_COLUMNS = "left-hand|middle|right-hand";

/**
 * The fund-group window of FY 2025-26's worst group, drawn under a step widened
 * to a tier that group's document has no node at: what it comes out as once the
 * empty column is dropped.
 *
 * THE SAME TUPLE THE THREE-COLUMN WINDOW DRAWS, which is the claim. A dropped
 * column is not a narrower version of the wide chart -- it is the chart the
 * narrow budget draws, reshaped at the tier set that survived.
 */
const EMPTY_DROP = { nodes: 18, links: 19 };

/**
 * The same widened fund-group step on the one group whose document fills the
 * fourth column: [the spine's revenue categories | the General Fund group | its
 * funds | the divisions pp.167-170 decompose fund/100 into].
 */
const FILLED_WIDE = { nodes: 40, links: 38, bands: "10/1/25" };

/**
 * Every fact id a committed document publishes, read off its own links.
 * @param {{links: {fact_ids: string[]}[]}} doc
 * @returns {Set<string>}
 */
function factIDsOf(doc) {
  const ids = new Set();
  for (const l of doc.links) for (const id of l.fact_ids) ids.add(id);
  return ids;
}

/**
 * The counts line a drawn view must read, composed from the COMMITTED GOLDENS.
 *
 * THE PARTITION IS BY MEMBERSHIP AND NOT BY carried_from, which is what makes
 * this evidence rather than a restatement: a drawn ribbon's facts are the drawn
 * document's or they are not, and `mine` answers that from the file the
 * packager wrote. app.js reaches the same two numbers through the stem a
 * carried mark records, so a client that counted the spine's ribbons into the
 * step document's share -- or stopped counting a flank kept off the document it
 * draws -- parts company with this here.
 *
 * A LINK IS THE OTHER DOCUMENT'S WHEN ANY OF ITS FACTS IS, rather than when all
 * of them are. The two readings agree on the committed corpus, where no drawn
 * ribbon cites both documents at once, and the strict one is the one that
 * cannot call a half-foreign ribbon native.
 *
 * @param {any} drawn the projection on screen
 * @param {Set<string>} mine the drawn document's own fact ids
 * @param {number} total the drawn document's fact total
 * @param {number} aboveTotal the fact total of the chart a flank is kept from
 */
function countsLineFor(drawn, mine, total, aboveTotal) {
  const plural = (/** @type {number} */ n, /** @type {string} */ word) =>
    n + " " + word + (n === 1 ? "" : "s");
  const foreign = drawn.links.filter((l) => l.fact_ids.some((/** @type {string} */ id) => !mine.has(id)));
  const own = drawn.links.filter((l) => !l.fact_ids.some((/** @type {string} */ id) => !mine.has(id)));
  const factsIn = (/** @type {any[]} */ links) => {
    const ids = new Set();
    for (const l of links) for (const id of l.fact_ids) ids.add(id);
    return ids;
  };
  const head = plural(drawn.links.length, "flow") + " between " + plural(drawn.nodes.length, "node");
  const cited = factsIn(own).size;
  if (!foreign.length) {
    return { carried: 0, cited: cited, want: head + (cited === total
      ? ", from " + plural(cited, "fact")
      : ", from " + cited + " of the document's " + plural(total, "fact")) };
  }
  return {
    carried: foreign.length,
    cited: cited,
    want: head + ": " + own.length + " citing " + cited + " of the document's " +
      plural(total, "fact") + ", and " + foreign.length +
      " carried unchanged from the chart above, citing " + factsIn(foreign).size +
      " of its " + plural(aboveTotal, "fact"),
  };
}

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
 * General Fund's depth-1 tuple is 39 nodes and 37 links citing 135 of 280 in
 * both years, and the sub-pixel count is NOT the same in both -- 2 in
 * FY2025-26 and 3 in FY2026-27 -- which is why COLUMNS carries it per column
 * and walkChain pins it once per column rather than once.
 *
 * THESE ARE PINS ON THE FOLD AND NOT ON THE DOCUMENT'S SIZE, which is this
 * file's half of the line tier's proof. A rung folds the tier-1 lines back
 * through node.parent before it draws, so an opened view whose shape depended
 * on how finely the document decomposes would be the fold failing rather than
 * a pin going stale. The citation is the one figure that does not survive that
 * argument: 135 and not the 141 rows a category-grain link would carry, the
 * six being rows that print a dash inside a category cell that is not zero.
 */
const COLUMNS = [
  {
    stem: "sankey", label: "FY 2025-26", step: "fund-flows", golden: goldenFundFlows,
    spine: goldenGraph,
    spendingStem: "department-spending", spending: goldenSpending,
    // The narrowest depth-2 ribbon, which is Patrol's in both years.
    worstDeep: "51.38",
    // PAGE.worst at the declared caps and uncapped: ribbons, and how many of
    // them lay out under a pixel.
    capped: { links: 19, hairlines: 0 }, uncapped: { links: 42, hairlines: 9 },
    tail: "24 smaller funds",
    // fund/100's share of the fund column's inflow, and how many times the
    // smallest fund's inflow it is: the two figures step 0's description
    // rounds to "half" and "less than a thirty-thousandth".
    share: "49.18", ratio: 31575,
    // The General Fund's depth-1 tuple at {0,2,3} with the residual drawn:
    // nodes, links and sub-pixel ribbons. The ONE mark the residual adds is
    // its own node -- its two endpoints are already on screen, in the kept
    // flank -- and it adds no ribbon at all, because the two it carries are
    // the kept flank's own, re-pointed past the group rather than copied.
    general: { nodes: 15, links: 13, hairlines: 1,
      // TWELVE OF THE THIRTEEN RIBBONS ARE THE SPINE'S. The kept flank is the
      // spine's ten revenue categories reaching this group, and the residual
      // carries two more; the one ribbon pp.127-140 and 167-170 draw here is
      // the group's whole inflow into fund/100, which alone cites 86 of the
      // 280. Measured by hand, 2026-09-13.
      counts: "13 flows between 15 nodes: 1 citing 86 of the document's 280 facts, " +
        "and 12 carried unchanged from the chart above, citing 12 of its 120 facts" },
    // The fund window one rung further in: [the group | fund/100 | its 23
    // divisions]. Its centre is the one node on the page whose two sides are
    // different quantities, and the step's description is what says so --
    // `fund` is [what it takes in, what its divisions spend].
    fund: { nodes: 25, links: 24, hairlines: 1 },
    // THE SAME WINDOW WITH ROOM FOR A FOURTH COLUMN: [the group | fund/100 |
    // its 23 divisions | the object-category cells they spend on]. `bands` is
    // the ribbons crossing each pair of adjacent columns, left to right, which
    // is what tools/jscheck/layout.mjs counts crossings inside and throws on a
    // link that spans two of. `right` is where d3 put the last column's rect,
    // which is chartWidth(4) - LABEL_GUTTER when the width was believed.
    fundWide: { nodes: 34, links: 54, hairlines: 2, tail: "36 smaller categories",
      bands: "1/23/30", right: 1263 },
    fundCentre: [15787347000, 14465080200],
    // THE RESIDUAL PER GROUP, IN CENTS, MEASURED OFF fisc export's OWN
    // sankey.json AND fund-flows.json (2026-09-11) under the check's
    // whole-or-nothing rule and independently of app.js: a declared
    // endpoint's spine link is residual where the fund-level document
    // carries nothing from it into the group's funds. A group absent here
    // draws no residual node at all: special-revenue, enterprise and
    // debt-service have their transfers in decomposed to the cent and draw no
    // fund-balance row.
    //
    // `out` IS 0 ON EVERY GROUP AND THAT IS THE TIER SET'S DOING, not the
    // documents'. {0,2,3} draws a group's funds and no division, so its funds
    // publish no outflow on this chart; carrying general's transfers out and
    // reserve increase against an outflow of nothing would state an identity
    // the drawn chart does not hold, and there is no column right of the
    // funds to draw those endpoints in. It is the rule this file already
    // applies to the five groups pp.167-170 do not decompose, reached through
    // the columns instead of through the file.
    residual: {
      // 1,034,154 draw + 480,400 transfers in.
      "fund-group/general": { in: 151455400, out: 0, carried: 2 },
      "fund-group/capital": { in: 250021300, out: 0, carried: 1 },
      "fund-group/internal-service": { in: 614753300, out: 0, carried: 1 },
    },
    // THE FOUR OBJECT-CATEGORY WINDOWS, MEASURED BY HAND ON 2026-09-13 through
    // drillDown over this column's two committed captures.
    //
    // FOUR ENTRIES AND THREE ABSENCES. The spine puts seven nodes in its
    // right-hand column; transfers/out, fund-balance/contribution and
    // fund-balance/reserve-increase are flow ends rather than object
    // categories and the step's role leaves them closed, which is the mirror
    // of the revenue step's gate at the other end of the chart.
    //
    // `views` is [nodes, links, sub-pixel ribbons, fund groups in the kept
    // flank] and `uncapped` is [links, sub-pixel ribbons] with the tier-4 cap
    // taken off -- which is what says the cap is doing the work here, unlike
    // the division cap one step over that has never engaged.
    object: {
      views: {
        "expenditure/capital-outlay": [9, 8, 0, 3],
        "expenditure/debt-services": [10, 9, 0, 4],
        "expenditure/services-and-supplies": [15, 14, 0, 5],
        "expenditure/wages-and-benefits": [15, 14, 0, 5],
      },
      uncapped: {
        "expenditure/capital-outlay": [8, 0],
        "expenditure/debt-services": [9, 0],
        "expenditure/services-and-supplies": [34, 4],
        "expenditure/wages-and-benefits": [31, 2],
      },
      tails: {
        "expenditure/services-and-supplies": "21 smaller divisions",
        "expenditure/wages-and-benefits": "18 smaller divisions",
      },
      // pp.85-125's division rows come to p0067's cell to the cent in every
      // one of this column's four categories, so no gap mark is drawn at all.
      gapCents: 0,
      // MEASURED BY HAND, 2026-09-13, over this column's committed captures:
      // 9 of the 14 ribbons are pp.85-125's and cite 29 of its 73 facts; the
      // other 5 are the spine's own, kept beside the centre, and cite 5 of
      // p0067's 120.
      counts: "14 flows between 15 nodes: 9 citing 29 of the document's 73 facts, " +
        "and 5 carried unchanged from the chart above, citing 5 of its 120 facts",
    },
    // THE (1,0) ROLLUPS THE DOCUMENT CARRIES, one per (printed row, kind) over
    // 93 lines, two of which reach their funds under both kinds. The same
    // number in both years -- fund/207's dash in FY2026-27 moves a fund and
    // not a row. The ten category windows below are where they are DRAWN, and
    // every other view the page opens drops them.
    rollups: 95,
    // THE TEN CATEGORY WINDOWS, MEASURED BY HAND ON 2026-09-13 through
    // drillDown over this column's two committed captures: the printed lines
    // on the left, the category the reader clicked in the middle, and the fund
    // groups the spine draws it reaching on the right.
    //
    // THE CENTRE IS THE NODE THE READER CLICKED, which is the owner's second
    // finding stated as a pin: the category is drawn, once, between its lines
    // and the groups its money reaches, and a shape that lost it is a red row.
    //
    // `views` is [nodes, links, sub-pixel ribbons, fund groups reached]. THE
    // TWO COLUMNS DRAW THE SAME TEN SHAPES, which is a measurement and not a
    // rule: fund/207 prints a dash in FY2026-27, which takes a fund out of
    // that column and no fund GROUP, and fund groups are what the right-hand
    // column draws. Stated as one table per column even so.
    category: {
      nodes: 12, links: 11, hairlines: 0,
      // MEASURED BY HAND, 2026-09-13, and the same sentence in both columns:
      // 9 of the 11 ribbons are pp.127-140's and cite 17 of its 280 facts, and
      // the 2 fund groups kept beside the centre reach it over 2 spine ribbons
      // citing 2 of p0067's 120.
      counts: "11 flows between 12 nodes: 9 citing 17 of the document's 280 facts, " +
        "and 2 carried unchanged from the chart above, citing 2 of its 120 facts",
      chargesLineTail: "11 smaller lines", chargesLinks: 13, moneyLinks: 13,
      // THE CENTRE OF THE PROPERTY TAXES WINDOW as d3 sizes it, what the two
      // reductions contribute to that figure, and what p127 prints net of
      // them -- contraNote's three figures, read off this column's golden
      // rather than typed, and asserted against the comment that quotes them.
      gross: 10343009200, reduced: 1698533900, net: 6945941400,
      views: {
        "revenue/charges-for-services": [14, 13, 0, 4], "revenue/contributions-outsourced": [5, 4, 0, 2],
        "revenue/fines-and-forfeitures": [4, 3, 0, 1], "revenue/intergovernmental": [12, 12, 1, 4],
        "revenue/licenses-and-permits": [11, 10, 0, 1], "revenue/miscellaneous-revenue": [14, 13, 3, 4],
        "revenue/taxes/other": [12, 11, 0, 2], "revenue/taxes/property": [12, 11, 0, 2],
        "revenue/taxes/sales": [4, 3, 0, 1], "revenue/use-of-money-and-property": [13, 13, 0, 5],
      },
    },
  },
  {
    stem: "sankey-2027", label: "FY 2026-27", step: "fund-flows-2027", golden: goldenFundFlows2027,
    spine: goldenGraph2027,
    spendingStem: "department-spending-2027", spending: goldenSpending2027,
    worstDeep: "67.02",
    capped: { links: 19, hairlines: 0 }, uncapped: { links: 41, hairlines: 7 },
    tail: "23 smaller funds",
    share: "50.79", ratio: 54786,
    // ONE MARK AND ONE RIBBON FEWER THAN FY2025-26, and the difference is the
    // endpoint set: general's change in working capital turns positive this
    // year, so there is no fund-balance draw to carry and the residual stands
    // on the transfer in alone.
    general: { nodes: 14, links: 12, hairlines: 1,
      // ONE CARRIED RIBBON FEWER, for the reason the residual block below
      // gives: there is no fund-balance draw to carry this year.
      counts: "12 flows between 14 nodes: 1 citing 86 of the document's 280 facts, " +
        "and 11 carried unchanged from the chart above, citing 11 of its 120 facts" },
    fund: { nodes: 25, links: 24, hairlines: 1 },
    // THE SAME TUPLE AS FY 2025-26, WHICH IS A MEASUREMENT AND NOT A COPY.
    // Both columns print 23 divisions for fund/100 and 44 object cells under
    // them, and the tier-5 cap folds the same 36; the year's figures differ and
    // the shape does not.
    fundWide: { nodes: 34, links: 54, hairlines: 2, tail: "36 smaller categories",
      bands: "1/23/30", right: 1263 },
    fundCentre: [16435814700, 14901457900],
    residual: {
      // 486,735 transfers in and NO draw -- general's change in working
      // capital turns positive this year, so it leaves the group as
      // fund-balance/contribution rather than arriving as a draw, and this
      // chart draws no column for it to leave into.
      "fund-group/general": { in: 48673500, out: 0, carried: 1 },
      "fund-group/capital": { in: 1012941600, out: 0, carried: 1 },
      "fund-group/internal-service": { in: 716064500, out: 0, carried: 1 },
    },
    // THE SAME FOUR, ON THE COLUMN THAT DOES NOT TIE. capital-outlay reaches
    // four divisions here against five, and services-and-supplies carries the
    // declared 250,000 between p0067 and pp.85-125 -- the one cell of the eight
    // that does not close, and the reason this window needed a gap mark at all.
    object: {
      views: {
        "expenditure/capital-outlay": [8, 7, 0, 3],
        "expenditure/debt-services": [10, 9, 0, 4],
        "expenditure/services-and-supplies": [16, 15, 0, 5],
        "expenditure/wages-and-benefits": [15, 14, 0, 5],
      },
      uncapped: {
        "expenditure/capital-outlay": [7, 0],
        "expenditure/debt-services": [9, 0],
        "expenditure/services-and-supplies": [35, 5],
        "expenditure/wages-and-benefits": [31, 2],
      },
      tails: {
        "expenditure/services-and-supplies": "21 smaller divisions",
        "expenditure/wages-and-benefits": "18 smaller divisions",
      },
      // $250,000.00, drawn as one derived mark rather than left as node height
      // with no ribbon under it.
      gapCents: 25000000,
      // ONE MORE OWN RIBBON THAN FY2025-26 AND NO MORE FACTS: the extra one is
      // the gap mark's, which is derived and cites nothing, so 10 ribbons cite
      // the same 29.
      counts: "15 flows between 16 nodes: 10 citing 29 of the document's 73 facts, " +
        "and 5 carried unchanged from the chart above, citing 5 of its 120 facts",
    },
    rollups: 95,
    category: {
      nodes: 12, links: 11, hairlines: 0,
      // MEASURED BY HAND, 2026-09-13, and the same sentence in both columns:
      // 9 of the 11 ribbons are pp.127-140's and cite 17 of its 280 facts, and
      // the 2 fund groups kept beside the centre reach it over 2 spine ribbons
      // citing 2 of p0067's 120.
      counts: "11 flows between 12 nodes: 9 citing 17 of the document's 280 facts, " +
        "and 2 carried unchanged from the chart above, citing 2 of its 120 facts",
      chargesLineTail: "11 smaller lines", chargesLinks: 13, moneyLinks: 13,
      gross: 10839120200, reduced: 1774967900, net: 7289184400,
      views: {
        "revenue/charges-for-services": [14, 13, 0, 4], "revenue/contributions-outsourced": [5, 4, 0, 2],
        "revenue/fines-and-forfeitures": [4, 3, 0, 1], "revenue/intergovernmental": [12, 12, 1, 4],
        "revenue/licenses-and-permits": [11, 10, 0, 1], "revenue/miscellaneous-revenue": [14, 13, 3, 4],
        "revenue/taxes/other": [12, 11, 0, 2], "revenue/taxes/property": [12, 11, 0, 2],
        "revenue/taxes/sales": [4, 3, 0, 1], "revenue/use-of-money-and-property": [13, 13, 0, 5],
      },
    },
  },
];

/** The caveat ids each committed golden carries, for the refs a year ships. */
const SPINE_CAVEATS = [
  "transfer-legs-unpaired", "internal-service-is-outside-the-external-headline",
  "working-capital-is-a-stock", "permanent-funds-have-no-column",
];
const FUND_FLOWS_CAVEATS = [
  "constraint-tier-is-our-reading", "the-revenue-schedule-is-published-twice",
  "mixed-grain-double-counts", "only-the-general-fund-is-decomposed",
];
const SPENDING_CAVEATS = [
  "no-fund-axis-on-these-pages", "the-ribbons-are-a-cross-tab",
  "the-boundary-is-not-classified-here",
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
 *
 * TWO PROJECTIONS, RESOLVED OFF EACH STEP'S OWN DECLARATION rather than by
 * index. The spine opens into pp.127-140 and into pp.85-125, and a list keyed
 * by position would go on handing the object-category rung a fund-flows file
 * the day a step is inserted before it -- which is the packager's own rule
 * (export.stepDocuments) reached from this side.
 */
function stepDocsFor(flows, spending) {
  return PAGE.steps.map((s) => {
    const stem = s.projection === "department-spending" ? spending : flows;
    const caveats = stem.startsWith("department-spending") ? SPENDING_CAVEATS : FUND_FLOWS_CAVEATS;
    return { stem, path: `data/${stem}.json`, caveats: refsFor(stem, caveats) };
  });
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
async function opened(plan, tweak, column = COLUMNS[0], extra) {
  const config = twoYearConfig();
  config.projections["fund-flows"] = "data/fund-flows.json";
  config.projections["fund-flows-2027"] = "data/fund-flows-2027.json";
  config.projections["department-spending"] = "data/department-spending.json";
  config.projections["department-spending-2027"] = "data/department-spending-2027.json";
  config.render_tiers = PAGE.renderTiers;
  config.steps = PAGE.steps;
  config.years = config.years.map((y, i) => Object.assign({}, y, {
    counts: { facts: 120, nodes: 25, links: 58 },
    chart_title: `Sankey diagram of the ${y.label} adopted budget`,
    caveats: refsFor(y.stem, SPINE_CAVEATS),
    steps: stepDocsFor(i === 0 ? "fund-flows" : "fund-flows-2027",
      i === 0 ? "department-spending" : "department-spending-2027"),
  }));
  if (tweak) tweak(config);
  const spineOf = (/** @type {() => any} */ load) => {
    const spine = load();
    spine.nodes.find((n) => n.id === "fund-group/general").label = "General Fund group";
    // THE SAME FOR THE CATEGORY, for the same reason: both documents print
    // "Property Taxes" for revenue/taxes/property, and the rung, the title and
    // the hint must name it in the spine's words. markContra names the
    // category in the STEP DOCUMENT's words, so the contra sentence stays
    // "Property Taxes" and the two are told apart.
    spine.nodes.find((n) => n.id === "revenue/taxes/property").label = "Property Taxes category";
    // AND THE SAME FOR THE FOUR OBJECT CATEGORIES, which the cross-tab prints
    // in the same words as the spine: the object-category window's centre is
    // the node the reader clicked, so it must be namable apart from the copy
    // the step document carries under the same id.
    for (const n of spine.nodes) {
      if (n.id.startsWith("expenditure/")) n.label = n.label + " category";
    }
    return spine;
  };
  const fetch = plannedFetch(Object.assign({
    "data/sankey.json": { doc: spineOf(goldenGraph) },
    "data/sankey-2027.json": { doc: spineOf(goldenGraph2027) },
    "data/fund-flows.json": { doc: goldenFundFlows() },
    "data/fund-flows-2027.json": { doc: goldenFundFlows2027() },
    "data/department-spending.json": { doc: goldenSpending() },
    "data/department-spending-2027.json": { doc: goldenSpending2027() },
  }, plan || {}));
  const app = loadApp(Object.assign({ config, fetch, checkedStem: column.stem }, extra || {}));
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

/**
 * The rollup links a drawn chart carries: a printed row added back into the
 * category it is printed under, which internal/project publishes at the (1,0)
 * pair.
 *
 * A CATEGORY'S WINDOW DRAWS THEM AND NO OTHER VIEW MAY, which is why the two
 * arms that call this are a pair rather than one assertion. The rollup was
 * published for this chart: it is the window's whole left half, the ribbons
 * that give the centre something flowing into it. Everywhere else the answer
 * is zero for a reason of its own -- at {0,3,4} both ends fold to the category
 * and foldDocument drops the self-loop, and at {2,5,4} and {4,5} neither end
 * is placeable, so scoped's quiet branch drops it before the fold. The
 * per-view node and link pins say the shapes did not move; this says WHERE the
 * rollups went, and a pin cannot tell a rollup that was dropped from one that
 * was never published.
 *
 * @param {{links: FiscLink[]}} doc
 */
function rollupsIn(doc) {
  return doc.links.filter((l) => l.source.startsWith("revenue-line/") &&
    l.target.startsWith("revenue/")).length;
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
  let visited = 0;
  // THE WHOLE TREE, NOT ONE EDGE OF IT, and that is what the rewrite buys.
  // This read the drawn chart for its children and the GOLDEN SPINE for its
  // roots, at `PAGE.steps[0].from` -- so the day the spine grew a second and a
  // third edge out of its own chart, the walk went on visiting the fund groups
  // alone and PAGE.openedViews agreed with it. Every node the chart on screen
  // OFFERS is opened now, at every depth, which is the same question asked once
  // instead of once per declared step.
  const walk = async (/** @type {string[]} */ path) => {
    if (path.length > PAGE.steps.length) {
      throw new Error(`the drill went ${path.length} rungs deep on ${PAGE.steps.length} ` +
        `declared step(s), at ${path.join(" > ")}; a step is opening its own chart`);
    }
    await at(app, ...path);
    const offers = app.projection.nodes.filter((n) => app.drillable(n)).map((n) => n.id);
    for (const id of offers) {
      // REOPENED FROM THE OVERVIEW FOR EACH OFFER rather than popped one rung,
      // because a rung records the chart it was opened from and popping back
      // into a sibling would reuse it. `at` is cheap: every document is fetched
      // once and cached for the year.
      await at(app, ...path);
      await mustOpen(app, id);
      visited++;
      const next = path.concat([id]);
      await visit(next.join(" > "), next.length);
      await walk(next);
    }
  };
  try {
    await walk([]);
  } catch (e) {
    app.drillUp(0);
    return { visited, refused: e && e.message ? e.message : String(e) };
  }
  app.drillUp(0);
  return { visited, refused: "" };
}

/**
 * The spine's first published column, opened into one node, for a caller in
 * another module.
 *
 * layout.mjs NEEDS A WINDOW AND THIS FILE IS WHERE ONE IS BUILT. The label rule
 * it measures keys on the column a view DECLARES, and the spine cannot tell
 * that rule from one keyed on d3's longest path -- every path through the spine
 * is the same length, so the two agree on all 25 of its nodes. A check written
 * over the spine alone is green because the gate fired. Rebuilding the page
 * config there instead would be a second copy of PAGE, which is the thing this
 * file's steps are read off Go to avoid.
 *
 * @param {string} id the node to open
 */
export async function openedWindow(id) {
  const { app } = await opened();
  await mustOpen(app, id);
  return app;
}

/**
 * The same page opened down a path at a stated column budget, for a caller in
 * another module.
 *
 * layout.mjs NEEDS A CHART OF FOUR COLUMNS AND THIS FILE IS WHERE ONE IS BUILT,
 * which is openedWindow's argument at one more column: the band count and the
 * label room are claims about a shape a reader reaches only on a wide window,
 * and rebuilding the page config there would be a second copy of PAGE.
 *
 * IT STATES THE BUDGET RATHER THAN LETTING THE PAGE DECIDE, which is what keeps
 * these measurements measurements: what the control gives a reader depends on
 * their viewport, and openedChain below is the entry point for a check that
 * wants that instead.
 *
 * @param {number} budget
 * @param {string[]} path the nodes to open, outermost first
 */
export async function openedWide(budget, path) {
  const { app } = await opened();
  app.setColumnBudget(budget);
  for (const id of path) await mustOpen(app, id);
  return app;
}

/**
 * The page opened down a path at the budget the PAGE decided, with the fetch
 * and the planted <main> beside it.
 *
 * openedWide's sibling, and the difference between them is the whole of what
 * this is for. That one moves the budget through setColumnBudget, which is how
 * a check MEASURES a chart no shipped viewport reaches; this one leaves the
 * budget where wireColumns put it, which is how a check DRIVES the control that
 * moves it. Asserting "the reader's + redrew the rung" against a budget the
 * check had already set would assert nothing.
 *
 * @param {string[]} path the nodes to open, outermost first
 * @param {object} [extra] loadApp options -- a viewport, a seeded localStorage
 */
export async function openedChain(path, extra) {
  const { app, fetch, main } = await opened(null, null, COLUMNS[0], extra);
  for (const id of path) await mustOpen(app, id);
  return { app, fetch, main };
}

/** A spine opened into one node, or into a node and then one beneath it. */
async function at(app, ...ids) {
  app.drillUp(0);
  for (const id of ids) await mustOpen(app, id);
}

/**
 * What a node is drawn as, over the overview and every view the chain opens.
 *
 * PINNED AND NOT BOUNDED, for this file's reason, and one of these figures is
 * load-bearing in a way the others are not. The arm below asserts that a mark
 * carries `opens` exactly when drillable says it opens -- and that sentence is
 * TRUE OF A PAGE WHERE NOTHING OPENS AT ALL, both sides being false everywhere.
 * `opens` is what stops the arm passing that way: it says the corpus actually
 * puts marks on both sides of the question.
 *
 * `both` IS ZERO AND IS PINNED BECAUSE IT IS ZERO. No mark on either committed
 * capture is an inference that also opens, so the composed marker nodeFlags
 * draws for that case is latent -- and a latent path with no figure beside it
 * is one a later document reaches with nothing to notice.
 */
const MARKS = { nodes: 368, opens: 44, derivedOnly: 17, both: 0 };

// The lines of render() that hang the affordance on the mark, pinned whole.
//
// THESE ARMS MEASURE THE RULES AND NOT WHAT render() DOES WITH THEM, which is
// layout.mjs's LABEL_SELECTION problem and the same cause: the stub answers no
// "#chart" selector, so d3 lays every selection render() builds over a null
// node. No <g> is created, no attribute is written, and no handler is
// registered -- measured directly: D3.select("#chart").size() is 0 and an
// .attr() accessor is invoked zero times. So the class, the marker and the
// three gestures are reachable here only because they are NAMED in app.js, and
// that they are the ones the chart is drawn and wired with is this pin's claim
// rather than any arm's.
//
// WHICH IS ALSO WHY THE GESTURES ARE FUNCTIONS AND NOT CLOSURES. A body written
// inline on that selection executes in no check in this directory, however many
// checks it grows.
const GESTURE_WIRING = [
  '.attr("class", /** @param {LaidNode} d */ (d) => nodeClass(d))',
  '.text(/** @param {LaidNode} d */ (d) => nodeFlags(d))',
  '.attr("aria-keyshortcuts", "Enter Space")',
  "clickNode(d, e.timeStamp);",
  "doubleClickNode(d, e.timeStamp);",
  "keyNode(d, e.key, e.timeStamp);",
  '.on("dblclick", /** @param {MouseEvent} e @param {LaidNode} d */ (e, d) => {',
];

/**
 * The affordance and the two gestures behind it.
 *
 * ONE COLUMN AND NOT BOTH. Every arm here is about what a mark offers and what
 * an activation does, neither of which reads a figure off the document -- and
 * the walk in the first arm visits both captures' shapes through the same
 * entry points the per-column suites already pin.
 */
async function gestureChecks() {
  const out = [];

  // ---------------------------------------------- a node that opens says so
  const { app } = await opened();
  /** Every laid node of a chart on screen, against what it is drawn as. */
  const tally = { nodes: 0, opens: 0, derivedOnly: 0, both: 0 };
  const wrong = [];
  const look = (where) => {
    for (const n of app.layOut(app.projection).nodes) {
      const classes = app.nodeClass(n).split(" ");
      const flags = app.nodeFlags(n);
      const opens = app.drillable(n);
      tally.nodes++;
      if (opens) tally.opens++;
      if (n.derived && !opens) tally.derivedOnly++;
      if (n.derived && opens) tally.both++;
      // THE CLASS AND THE MARKER ARE ONE CLAIM AND ARE ASSERTED AS ONE. A
      // marker with no class is an affordance the stylesheet cannot reach; a
      // class with no marker is one nothing renders. Either alone is the half
      // contract this wave exists to refuse.
      if (classes.includes("opens") !== opens ||
          flags.includes("▸") !== opens ||
          flags.includes("◇") !== Boolean(n.derived)) {
        wrong.push(`${where} > ${n.id} is drawn "${app.nodeClass(n)}" / "${flags}" but ` +
          `${opens ? "opens" : "does not open"}`);
      }
    }
  };
  look("the overview");
  const walk = await everyOpenedView(app, (where) => look(where));
  out.push({
    name: "a node that opens is drawn as one, and a node that does not is not",
    ok: walk.refused === "" && walk.visited === PAGE.openedViews && wrong.length === 0 &&
        tally.nodes === MARKS.nodes && tally.opens === MARKS.opens &&
        tally.derivedOnly === MARKS.derivedOnly && tally.both === MARKS.both,
    detail: wrong.length
      ? `${wrong.length} of ${tally.nodes} mark(s) are drawn as something they are not: ` +
        wrong.slice(0, 3).join("; ")
      : `over the overview and ${walk.visited} opened view(s), ${tally.nodes} mark(s) ` +
        `(want ${MARKS.nodes}): ${tally.opens} carry "opens" and the triangle (want ` +
        `${MARKS.opens}), ${tally.derivedOnly} the diamond alone (want ${MARKS.derivedOnly}), ` +
        `${tally.both} both (want ${MARKS.both}, so the composed marker is latent); every ` +
        `one of them agrees with drillable`,
  });

  out.push({
    name: "the chart is drawn and wired with the rules these arms measure",
    ok: GESTURE_WIRING.every((q) => app.source.includes(q)),
    detail: (() => {
      const gone = GESTURE_WIRING.filter((q) => !app.source.includes(q));
      return gone.length === 0
        ? `all ${GESTURE_WIRING.length} lines of render()'s node selection read nodeClass, ` +
          `nodeFlags and the three gesture functions, so the affordance measured above is ` +
          `the affordance drawn and the gestures driven below are the gestures bound`
        : `render()'s node selection no longer reads them: ${gone.length} of ` +
          `${GESTURE_WIRING.length} lines are gone, starting "${gone[0]}"`;
    })(),
  });

  // ------------------------------------------ Enter opens and Space isolates
  //
  // DRIVEN THROUGH keyNode AND NOT THROUGH A SYNTHESISED KeyboardEvent, for
  // GESTURE_WIRING's reason: there is no element to dispatch one at. The key
  // filter, e.repeat and preventDefault stay in the closure and are pinned as
  // text; what a key MEANS is here.
  const { app: keys } = await opened();
  const opensID = "fund-group/general";
  const closedID = "transfers/in";
  const nodeAt = (a, id) => a.layOut(a.projection).nodes.find((n) => n.id === id);
  const after = async (a, run) => { run(); await settle(); 
    return { depth: a.drilled.length, top: topOf(a), isolated: a.isolated }; };

  const enter = await after(keys, () => keys.keyNode(nodeAt(keys, opensID), "Enter", 1000));
  keys.drillUp(0);
  const space = await after(keys, () => keys.keyNode(nodeAt(keys, opensID), " ", 5000));
  keys.drillUp(0);
  // AND ENTER STILL ACTIVATES A MARK WITH NOTHING TO OPEN, which is the half of
  // the split that is not a split: a role="button" whose Enter does nothing is
  // worse than one whose two keys agree, and on these marks they always did.
  const closedEnter = await after(keys, () => keys.keyNode(nodeAt(keys, closedID), "Enter", 9000));
  out.push({
    name: "Enter opens and Space isolates, and neither does the other",
    ok: enter.depth === 1 && enter.top === opensID && enter.isolated === "" &&
        space.depth === 0 && space.isolated === opensID &&
        closedEnter.depth === 0 && closedEnter.isolated === closedID,
    detail: `Enter on ${opensID} left the chart at depth ${enter.depth} on ` +
      `"${enter.top || "the overview"}" following "${enter.isolated}" (a drill clears the ` +
      `isolation, so "" is the whole of what Enter may leave); Space on the same mark left ` +
      `depth ${space.depth} following "${space.isolated}"; Enter on ${closedID}, which opens ` +
      `into nothing, left depth ${closedEnter.depth} following "${closedEnter.isolated}"`,
  });

  // --------------------------- a double click opens, and puts the isolate back
  //
  // THE SEED IS THE WHOLE ARM. A double click delivers click, click, dblclick,
  // and on a chart with nothing isolated those two clicks toggle an isolation
  // on and off again and land back on "" without any help -- so an arm run from
  // the empty state is green whether or not the restore exists. It starts from
  // a mark the reader was already following instead, which is the only state
  // the two answers differ in.
  const { app: clicks } = await opened();
  const seed = "revenue/taxes/sales";
  const marks = (id) => nodeAt(clicks, id);
  clicks.clickNode(marks(seed), 0);
  const seeded = clicks.isolated;
  clicks.clickNode(marks(closedID), 1000);
  const firstClick = clicks.isolated;
  clicks.clickNode(marks(closedID), 1050);
  const secondClick = clicks.isolated;
  const closedDouble = await after(clicks, () => clicks.doubleClickNode(marks(closedID), 1060));

  // A SECOND PAGE AND NOT THE SAME ONE WOUND BACK. setIsolated is a toggle, so
  // seeding the same mark again on a chart still following it CLEARS the seed
  // -- the arm would then start from the empty state its own comment above says
  // it must not.
  const { app: opensClicks } = await opened();
  const openMarks = (id) => nodeAt(opensClicks, id);
  opensClicks.clickNode(openMarks(seed), 0);
  const reseeded = opensClicks.isolated;
  opensClicks.clickNode(openMarks(opensID), 1000);
  opensClicks.clickNode(openMarks(opensID), 1050);
  const opensDouble = await after(opensClicks,
    () => opensClicks.doubleClickNode(openMarks(opensID), 1060));
  out.push({
    name: "a double click opens, and leaves the isolation the reader had",
    ok: seeded === seed && firstClick === closedID && secondClick === "" &&
        closedDouble.depth === 0 && closedDouble.isolated === seed &&
        reseeded === seed && opensDouble.depth === 1 && opensDouble.top === opensID &&
        opensDouble.isolated === "",
    detail: `following "${seeded}", a double click on ${closedID} went "${firstClick}" then ` +
      `"${secondClick}" under the reader and came back to "${closedDouble.isolated}" at depth ` +
      `${closedDouble.depth} (want "${seed}", nothing opened); the same gesture on ${opensID} ` +
      `opened it to depth ${opensDouble.depth} on "${opensDouble.top}" following ` +
      `"${opensDouble.isolated}", which drawChart cleared on the way -- so what the restore is ` +
      `witnessed by is the mark that does not open`,
  });

  return out;
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
    /** Each opened view's counts line, beside the one the goldens say it must be. */
    const said = [];
    // AND WHETHER EACH VIEW FILLS THE COLUMNS ITS STEP DECLARES. openableColumns
    // narrows to the tiers actually drawn, and its comment says that narrowing
    // changes nothing on the committed corpus -- which is a measurement, so it
    // is measured here rather than asserted there. A view that comes out short
    // is not a defect; a comment claiming none does while one has is.
    const short = [];
    const walk = await everyOpenedView(app, (where, depth) => {
      drawn.push(Object.assign({ where, depth, rollups: rollupsIn(app.projection) },
        measure(app, app.projection)));
      // THE DRAWN DOCUMENT IS ASKED FOR BY THE NAME THE FILE CARRIES, not by
      // the year's stem: the packager writes `projection: "fund-flows"` into
      // fund-flows-2027.json too, which is what lets one carried_from resolve
      // under either column.
      const stem = app.projection.projection;
      const golden = stem === "department-spending" ? col.spending() : col.golden();
      said.push(Object.assign({ where, depth, got: app.dom.byId.get("counts-line").textContent },
        countsLineFor(app.projection, factIDsOf(golden),
          golden.metadata.counts.facts, col.spine().metadata.counts.facts)));
      // THE COLUMNS THE CHART ASKED FOR, NOT THE ONES THE STEP OFFERS. A step
      // may declare more columns than the budget draws (activeTiers), and a
      // widened column this reader never asked for is not a column that came
      // out short.
      const has = new Set(app.projection.nodes.map((n) => n.tier));
      const missing = app.activeTiers().filter((t) => !has.has(t));
      if (missing.length) short.push(`${where} draws no tier ${missing.join(", ")}`);
    });
    // THE WORST DIVISION IS PINNED BY NAME AND BY WIDTH: Patrol's window at
    // 51.38px, and PAGE.inert is that node because it is the worst, not a
    // division picked at random. It is DEPTH 3 since the fund became a rung of
    // its own -- group, fund, division -- which is one more rung than
    // fisc-ko1j measured and the same chart.
    const deep = drawn.filter((d) => d.depth === 3);
    const worstDeep = deep.length ? deep.reduce((a, b) => (b.smallest < a.smallest ? b : a)) : null;
    // THE STEP FILE IS THE COLUMN'S OWN, asserted on the wire: a walk that
    // drew every view from the other year's file would pin that year twice.
    const stepAsked = fetch.asked.filter((p) => p.startsWith("data/fund-flows"));
    out.push({
      name: `${col.label}: every node the tree offers to open draws when opened, at every depth`,
      ok: walk.refused === "" && walk.visited === PAGE.openedViews && drawn.length === walk.visited &&
          Boolean(worstDeep) && worstDeep.where.endsWith(" > " + PAGE.inert) &&
          worstDeep.smallest.toFixed(2) === col.worstDeep && short.length === 0 &&
          stepAsked.join() === `data/${col.step}.json`,
      detail: walk.refused
        ? `after ${walk.visited} view(s), refused: ${walk.refused}`
        : `${walk.visited} views opened (want ${PAGE.openedViews}) from ${JSON.stringify(stepAsked)}; ` +
          `smallest ribbon over all of them ${Math.min(...drawn.map((d) => d.smallest)).toFixed(3)}px; ` +
          `the narrowest depth-3 ribbon is ` +
          `${worstDeep ? `${worstDeep.where} at ${worstDeep.smallest.toFixed(2)}px` : "nowhere"} ` +
          `(want ${col.worstDeep}); ` +
          (short.length ? `SHORT OF A DECLARED COLUMN: ${short.slice(0, 3).join("; ")}`
            : "every one of them draws every column its step declares"),
    });

    // EVERY COUNTS LINE, AGAINST THE DOCUMENT EACH OF ITS NUMBERS IS OF.
    //
    // A WINDOW DRAWS TWO DOCUMENTS. Its kept flank is a flank of the chart it
    // was opened from, so those ribbons cite that document's facts; reported as
    // a share of the drawn document's total they are one document's figure over
    // another's denominator, which is a published number that is wrong. The
    // partition here is membership of the drawn document's own fact ids, read
    // off the committed golden, so it agrees with the client only if the client
    // is telling the two apart.
    //
    // AND WHERE A SECOND DOCUMENT MAY APPEAR AT ALL. Only a window opened off
    // the spine keeps a flank of another file; the fund and division windows
    // keep fund-flows on a fund-flows chart, whose facts ARE the drawn
    // document's and are counted as such. So a carried count below depth 1 is
    // this check going red, not a shape it tolerates.
    const carrying = said.filter((c) => c.carried > 0);
    const wrong = said.filter((c) => c.got !== c.want);
    out.push({
      name: `${col.label}: every opened view's counts line weighs each document's ribbons against that document's own total`,
      ok: said.length === PAGE.openedViews && wrong.length === 0 &&
          carrying.length === PAGE.carryingViews &&
          carrying.every((c) => c.depth === 1),
      detail: wrong.length
        ? `${wrong.length} of ${said.length} view(s) misreport: ${wrong.slice(0, 2)
            .map((c) => `${c.where} says "${c.got}" for "${c.want}"`).join("; ")}`
        : `${said.length} view(s) each read what the goldens say they must; ` +
          `${carrying.length} of them carry another document's ribbons (want ` +
          `${PAGE.carryingViews}), at depth(s) ` +
          `${JSON.stringify([...new Set(carrying.map((c) => c.depth))])}; the widest flank is ` +
          `${carrying.reduce((a, b) => (b.carried > a.carried ? b : a)).where} at ` +
          `${Math.max(...carrying.map((c) => c.carried))} carried ribbon(s)`,
    });

    // THE ROLLUP IS DRAWN IN THE TEN CATEGORY WINDOWS AND IN NO OTHER VIEW,
    // asserted over the whole walk rather than left to the per-view pins. Both
    // halves have to hold -- the document publishes 95 and exactly the ten
    // windows draw one -- because a filter that started keeping a rollup
    // somewhere else would move no node and no ribbon count a shape pin can
    // see.
    const drewRollup = drawn.filter((d) => d.rollups > 0);
    const windows = drawn.filter((d) => d.depth === 1 && d.where.startsWith("revenue/"));
    out.push({
      name: `${col.label}: the document's line-to-category rollups are drawn in the ten category windows and in no other view`,
      ok: rollupsIn(col.golden()) === col.rollups && drawn.length > 0 && windows.length === 10 &&
          drewRollup.length === windows.length &&
          drewRollup.every((d) => d.depth === 1 && d.where.startsWith("revenue/")),
      detail: `${col.step} carries ${rollupsIn(col.golden())} rollup link(s) (want ${col.rollups}); ` +
        `of ${drawn.length} view(s) walked, ${windows.length} are category windows (want 10) and ` +
        `${drewRollup.length} draw a rollup` +
        (drewRollup.some((d) => !d.where.startsWith("revenue/"))
          ? `, including ${drewRollup.find((d) => !d.where.startsWith("revenue/")).where}`
          : ""),
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
    await at(app, "fund-group/general", "fund/100", PAGE.inert);
    const inert = measure(app, app.projection);
    await at(noCap, "fund-group/general", "fund/100", PAGE.inert);
    const inertUncapped = measure(noCap, noCap.projection);
    out.push({
      name: `${col.label}: the category cap is inert three rungs deep, because no division is wide enough to need it`,
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
  // cap made beneath one, or any node whose parent chain in the FETCHED
  // documents REACHES a fund group. Read off the goldens' parent field rather
  // than an id prefix, because the spine's uses share the expenditure/ prefix
  // with fund-flows' object cells and belong to no group.
  //
  // THE CHAIN AND NOT THE FIRST LINK, and "has a parent at all" is what it said
  // until the walk reached the revenue categories' views. A printed revenue
  // line is parented to its category, so it HAS a parent -- and it is money
  // arriving, exactly like the category above it, so it correctly resolves to
  // no group. Measured: 89 such marks in FY2025-26 and 88 in FY2026-27, every
  // one of them a revenue-line node, reported as unresolved by a predicate
  // whose own comment says a revenue source is not.
  //
  // AND THE HIERARCHY IS THE DRAWN DOCUMENT'S, NOT ONE PAIR OF FILES'. A
  // division is parented to its fund on pp.167-170 and to NOTHING on pp.85-125,
  // which print what it spends whatever pays for it -- so the same dept/ id is
  // on the fund side in one window and correctly group-less in the other, and a
  // map built from the two revenue documents called 18 of FY2025-26's marks
  // unresolved for being drawn from the third. The spine's own parents sit
  // underneath because a window splices its kept flank in, and every one of
  // them is "".
  for (const col of COLUMNS) {
    const { app } = await opened(null, null, col);
    const files = new Map([[col.stem, col.spine()], [col.step, col.golden()],
      [col.spendingStem, col.spending()]]);
    const parentsFor = (/** @type {string} */ stem) => {
      const m = new Map();
      for (const n of col.spine().nodes) m.set(n.id, n.parent);
      for (const n of (files.get(stem) || { nodes: [] }).nodes) m.set(n.id, n.parent);
      return m;
    };
    let parentOf = parentsFor(col.stem);
    const underAGroup = (/** @type {string} */ id) => {
      for (let up = parentOf.get(id), hops = 0; up && hops < 9; hops++) {
        if (app.isFundGroup({ id: up })) return true;
        up = parentOf.get(up);
      }
      return false;
    };
    // AN AGGREGATE IS ON THE FUND SIDE WHEN THE COLUMN IT FOLDED IS, and
    // shapeFor is what decides that: a tail wholly inside the opened node is
    // parented to it and inherits its hue, and one spanning fund groups gets
    // "" and draws --muted on purpose. A category's line tail is parented to
    // the CATEGORY, and a category has no group -- so "any aggregate" reported
    // 29 marks unresolved in FY2025-26 and 28 in FY2026-27, every one a tail
    // the cap folded on the revenue side.
    const onTheFundSide = (/** @type {{id: string, parent?: string}} */ n) => {
      if (app.isFundGroup(n)) return true;
      if (app.isAggregate(n.id)) {
        return Boolean(n.parent) &&
          (app.isFundGroup({ id: n.parent }) || underAGroup(n.parent));
      }
      return underAGroup(n.id);
    };
    const unresolved = (/** @type {{nodes: any[], projection?: string}} */ d) => {
      app.layOut(d);
      parentOf = parentsFor(d.projection || col.stem);
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
    // AND NOT THE CENTRE EITHER, for a reason the window model makes
    // structural: the node the reader opened is alone in the middle column of
    // every window, so its share is suppressed by construction and an arm
    // expecting one would be red for being right. fund-group/general carries a
    // caveat and sorts first, so `find` picked it.
    const centre = where.open.length ? where.open[where.open.length - 1] : "";
    const marked = app.projection.nodes.find(
      (n) => !n.carried_from && n.id !== centre && app.caveatsFor(n.id).length > 0);
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
    // GUARDED, SO A REGRESSION IS A RED ROW AND NOT A CRASH. `first` is
    // undefined when no carried mark carries a caveat -- which is exactly the
    // regression this arm exists to catch -- and an unguarded pin() on it threw
    // a TypeError out of checks(), reported as "a whole check module threw"
    // rather than as this arm failing. Found by pass three of /code-review.
    const pagesIn = (/** @type {string[]} */ hs) => [...new Set(hs
      .filter((h) => !h.startsWith("caveats"))
      .map((h) => (h.match(/p(?:age=)?0*(\d+)/) || [])[1])
      .filter(Boolean))].map(Number).sort((a, b) => a - b);
    let carriedPages = [];
    if (first) {
      panelFor(first.id);
      carriedPages = pagesIn(hrefsIn(app.dom.byId.get("detail")));
    }
    panelFor("fund/100");
    const drawnPages = pagesIn(hrefsIn(app.dom.byId.get("detail")));
    const wantDrawn = `caveats.html#caveat-${column.step}--${drawnCaveat ? drawnCaveat.id : ""}`;
    out.push({
      name: `${column.label}: a carried mark's panel links to the spine's copy of the caveat, and the drawn mark beside it still links to the step document's`,
      // ONE CARRIED MARK WITH A CAVEAT AND NOT TWO. transfers/out was the
      // other, and {0,2,3} draws no column for a fund's outflow, so the
      // residual states the inflow side alone -- which is why this counts the
      // set rather than asserting "at least one".
      ok: carried.length > 0 && ids.length === 1 &&
          ids[0] === "transfers/in" &&
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
    // AND WITH THE DECLARATION REMOVED FROM THE STEP, NOTHING IS RE-POINTED.
    // The client spells no endpoint of its own, so the shipped set is the only
    // source of the mark -- and what the declaration decides at {0,2,3} is
    // WHERE two ribbons land, not whether their ends are drawn: the endpoints
    // are marks of the chart above and the window keeps that flank either way.
    // Undeclared, they run into the group, which then takes in more than it
    // sends on by exactly the residual, with nothing on the page saying so;
    // declared, they run past it onto a node of their own beside the funds and
    // the group's two sides agree. The link COUNT is identical in both, which
    // is why this measures the shortfall instead.
    const { app: undeclared } = await opened(null, (c) => {
      c.steps = PAGE.steps.map((st) => { const t = Object.assign({}, st); delete t.residual; return t; });
    }, col);
    await at(undeclared, general);
    const sumAt = (/** @type {any} */ a, /** @type {"source"|"target"} */ end) =>
      a.projection.links.filter((/** @type {any} */ l) => l[end] === general)
        .reduce((/** @type {number} */ sum, /** @type {any} */ l) => sum + l.value_cents, 0);
    const absorbed = sumAt(undeclared, "target") - sumAt(undeclared, "source");
    const noneDrawn = !undeclared.projection.nodes.some((n) => app.isResidual(n.id)) &&
      undeclared.projection.links.length === col.general.links &&
      absorbed === want[general].in;
    out.push({
      name: `${col.label}: the residual is drawn beside the funds of exactly the groups whose flows the fund-level document does not decompose, and only from the declared set`,
      ok: asSeen === asWant && stray.length === 0 && noneDrawn,
      detail: (asSeen === asWant
        ? `drawn on ${Object.keys(seen).map((g) => g.replace("fund-group/", "")).join(", ")} with the ` +
          `pinned sums, and on no other group`
        : `drawn ${asSeen}, want ${asWant}`) +
        (stray.length ? `; stray carried marks: ${stray.join("; ")}` : "") +
        `; with residual deleted from the step the same ${undeclared.projection.links.length} ` +
        `ribbon(s) are drawn and ${undeclared.projection.nodes.some((n) => app.isResidual(n.id))
          ? "a residual node appears FROM NOWHERE"
          : `no residual node is, leaving the group ${absorbed} cent(s) of unaccounted node ` +
            `height (want ${want[general].in})`}`,
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

    // THE RESIDUAL TAKES THE MONEY PAST THE GROUP, AND THAT IS WHAT MAKES THE
    // CENTRE BALANCE. The endpoints' ribbons are the kept flank's own, drawn
    // once each with the group end re-pointed onto a node beside the funds, so
    // the group takes in exactly what its funds take in. The three claims are
    // one check because the wrong repair passes two of them: copying the
    // chart above's links instead of re-pointing the drawn ones leaves the
    // residual right, the endpoints drawn at twice their printed figure, and
    // the centre short by the residual -- measured before the fix, transfers/in
    // left tier 0 at 960,800 against the 480,400 p0067 prints.
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
    const centreIn = app.projection.links.filter((l) => l.target === general)
      .reduce((sum, l) => sum + l.value_cents, 0);
    const centreOut = app.projection.links.filter((l) => l.source === general)
      .reduce((sum, l) => sum + l.value_cents, 0);
    // EACH ENDPOINT SENDS ITS PRINTED FIGURE ONCE, read off the drawn chart:
    // the spine's link for it, and no second ribbon anywhere.
    const doubled = r.links.filter((l) => {
      const e = l.target === r.id ? l.source : l.target;
      const sent = app.projection.links.filter((x) => x.source === e)
        .reduce((sum, x) => sum + x.value_cents, 0);
      const printed = spineLink.get(e + "|" + general);
      return !printed || sent !== printed.value_cents;
    }).map((l) => (l.target === r.id ? l.source : l.target));
    out.push({
      name: `${col.label}: the residual carries the money that reaches no fund past the group, which is left taking in exactly what its funds take in`,
      ok: Boolean(r.node) && r.in === want[general].in && r.out === want[general].out &&
          Boolean(laidNode) && laidNode.value === Math.max(r.in, r.out) &&
          laidNode.layer === tiers.indexOf(r.node.tier) && r.node.tier === 3 &&
          endsRight && app.fundGroupOf(r.node) === general &&
          centreIn === centreOut && centreIn > 0 && doubled.length === 0,
      detail: r.node
        ? `in ${r.in} out ${r.out} cents (want ${want[general].in} / ${want[general].out}); laid at ` +
          `${laidNode ? laidNode.value : "nowhere"} in column ${laidNode ? laidNode.layer : "?"} of tier ` +
          `${r.node.tier}; ends ${layers.join(", ")}; hue from ${app.fundGroupOf(r.node) || "no group"}; ` +
          `the group takes in ${centreIn} and sends on ${centreOut}` +
          (doubled.length ? `; DRAWN TWICE: ${doubled.join(", ")}` : "; no endpoint drawn twice")
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
    // AND WHERE IT WAS THE LAST ONE, THE MARK GOES. FY2026-27's general has no
    // fund-balance draw -- its change in working capital is a contribution --
    // so the transfer in is its whole residual, and decomposing that whole
    // leaves nothing for a node to stand for. "A group with nothing to carry
    // draws nothing" is carryResidual's own rule and this is the only place
    // the General Fund reaches it; the other column keeps its draw and its
    // node, so the two branches are both witnessed.
    const wantLinks = want[general].carried - 1;
    const wantIn = want[general].in - transferIn.value_cents;
    out.push({
      name: `${col.label}: an endpoint the fund-level document decomposes whole is not carried, even on the General Fund`,
      ok: wantLinks === 0
        ? !sr.node && sr.links.length === 0 && wantIn === 0
        : Boolean(sr.node) && !stillCarried && sr.in === wantIn &&
          sr.out === want[general].out && sr.links.length === wantLinks,
      detail: `with transfers/in -> fund/100 at ${transferIn.value_cents} in the step document, the ` +
        (sr.node
          ? `residual ${stillCarried ? "STILL carries transfers/in" : "drops transfers/in"} and reads ` +
            `in ${sr.in} (want ${wantIn}) out ${sr.out} over ${sr.links.length} flow(s), want ${wantLinks}`
          : `residual node is gone, which is ${wantLinks === 0 ? "right: it had nothing else to carry" :
            `WRONG: ${wantLinks} flow(s) should still be on it`}`),
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

  // ------------------------------------------------------ a revenue category
  //
  // THE SECOND EDGE OUT OF THE SPINE'S CHART, over both columns: a category
  // opened into a window of the lines pp.127-140 print under it, the category
  // itself, and the fund groups the spine draws it reaching -- through the
  // same entry point a click and Enter call. Every figure is pinned in
  // COLUMNS[].category off the committed goldens.
  for (const col of COLUMNS) out.push(...(await walkCategory(col)));
  // EACH GROUP BELOW REPORTS ITS OWN THROW, which is run.mjs's rule about a
  // check applied one level down. These groups drive the shipped declaration,
  // so a step that stops being a window takes mustOpen's throw out of the
  // first of them -- and one throw out of checks() reports a single line about
  // a hundred arms that were never reached. Measured: with Keep deleted from
  // the revenue-category step, the whole module printed one FAIL and said
  // nothing about the ten views whose shapes had moved.
  const group = async (/** @type {() => Promise<any[]>} */ fn) => {
    try {
      return await fn();
    } catch (e) {
      return [{ name: `${fn.name}: the group threw before producing any check`, ok: false,
        detail: String((e && e.stack) || e) }];
    }
  };
  for (const fn of [gapAtTheCentre, categoryProbes, keylessSteps, severalParents, windowChecks,
    objectCategoryChecks, columnAndPartitionChecks, foreignFlankProbe, widenedColumns,
    gestureChecks]) {
    out.push(...(await group(fn)));
  }

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
      { key: "g", after: [""], from: 2, tiers: [0, 3, 4], caps: [{ tier: 3, cap: 2 }, { tier: 4, cap: 2 }], back: "Back", tail: "funds" },
      { key: "d", after: ["g"], from: 4, tiers: [4, 5], caps: [{ tier: 5, cap: 1 }], back: "Up", tail: "categories" },
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
        // THE STEP OPENS FROM A TIER THE OVERVIEW DRAWS. drillDown looks the
        // activated node up in the CHART ON SCREEN, so a step opening from a
        // tier the overview folds away is a step no reader could reach -- which
        // is the configuration export.View refuses by placing every root step's
        // From against RenderTiers. The group is what the overview draws here:
        // fund/100 folds into it at {0,2}.
        render_tiers: [0, 2],
        steps: [{ key: "fund", after: [""], from: 2, tiers: [2, 4], back: "Back", tail: "divisions" }],
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
    const outcome = await openInto(app, "fund-group/general");
    const groupDrawn = app.projection.nodes.some((n) => n.id === "fund-group/general");
    const swatches1 = legend.children.length;
    out.push({
      name: "an opened view draws no legend even where a fund-group node survives the filter",
      ok: outcome === "drew" && swatches0 === 1 && groupDrawn && swatches1 === 0,
      detail: `overview ${swatches0} swatch(es); opened into the group, its node is ` +
        `${groupDrawn ? "drawn" : "NOT drawn, so this asserts nothing"} and the legend holds ` +
        `${swatches1} -- one would be a key to a chart of one hue, toggling the only group on it`,
    });
  }

  return out;
}

/**
 * A revenue category opened from the spine into its own window, each depth
 * read back from the DOM as walkChain reads the chain: the stack, the fetch,
 * the words, the cap on its line column, the contra rows against its centre,
 * and the way back.
 *
 * THE CENTRE IS THE NODE THE READER CLICKED, and that is the subject of this
 * whole function rather than one arm of it: a window's middle column is the
 * mark that was opened, so the arm asserting the category is drawn at all --
 * and drawn in the middle -- is the one that answers the owner's second
 * finding. Every figure below is measured against {1,0,2}.
 *
 * THE FIGURES ARE THE COLUMN'S OWN, pinned in COLUMNS[].category and, where
 * the golden can state them independently of app.js -- the signed sum over the
 * drawn rollups, the top eight by outflow, the spine's own cell for the
 * category -- computed off the golden here and compared against what the
 * client drew.
 */
async function walkCategory(col) {
  const out = [];
  const { app, fetch, body } = await opened(null, null, col);
  const step = PAGE.steps[3];
  const want = col.category;
  const golden = col.golden();
  const spine = col.spine();
  const property = "revenue/taxes/property";
  const asked0 = fetch.asked.slice();
  const desc = app.dom.document.getElementById("chart-desc");
  const served = templateDesc("index.html.tmpl", "");
  desc.textContent = served;
  const pointer = served.slice(served.indexOf(". ") + 2);
  const text = (/** @type {any} */ el) => {
    const parts = [];
    const walk = (/** @type {any} */ n) => {
      if (n.textContent) parts.push(n.textContent);
      for (const c of n.children || []) walk(c);
    };
    walk(el);
    return parts.join(" ");
  };

  // WHAT OPENS AT DEPTH 0, asked of the drawn spine: every category, every
  // fund group, and neither endpoint that shares tier 0 with the categories.
  const at0 = app.projection.nodes;
  const opens = (/** @type {string} */ id) => {
    const n = at0.find((x) => x.id === id);
    return Boolean(n) && app.drillable(n);
  };
  const categories = at0.filter((n) => n.id.startsWith("revenue/")).map((n) => n.id);
  const columns0 = app.openableColumns();

  const outcome = await openInto(app, property);
  // REPORTED, NOT THROWN. Every arm below stands on this view being open;
  // measured with the side ignored, the first mustOpen threw out of checks()
  // and the module reported nothing about why.
  if (outcome !== "drew") {
    app.drillUp(0);
    out.push({
      name: `${col.label} category: Property Taxes opens into a window whose centre it is, and every sentence says so`,
      ok: false,
      detail: `opening Property Taxes came to "${outcome}"; ${refusals(app.dom.byId.get("main") ||
        app.dom.document.node()).map((b) => b.textContent).join(" | ")}`,
    });
    return out;
  }
  const at1 = words(app);
  const m1 = measure(app, app.projection);
  const asked1 = fetch.asked.slice();
  const rows1 = body.children.length;
  const anyOpens1 = app.projection.nodes.some((n) => app.drillable(n));
  const laid1 = app.layOut(app.projection);
  // THE COLUMNS AS d3 PLACED THEM, left to right, read back as tiers -- not
  // the tier numbers sorted. {1,0,2} is non-monotonic on purpose, and a sorted
  // read would call the same chart correct whichever order it came out in.
  const placed = [...new Set(laid1.nodes.slice().sort((a, b) => a.x0 - b.x0).map((n) => n.tier))];
  const middle = app.projection.nodes.filter((n) => n.tier === 0).map((n) => n.id);
  const flank = app.projection.nodes.filter((n) => n.tier === 2);

  // THE CONTRA ROWS: two, both ERAF and the RPTTF reduction, and they now run
  // into the CENTRE rather than into a fund. pp.127-140 print each as a
  // reduction of Property Taxes, and the link the window draws for a line is
  // its rollup back into the category it is printed under -- so the mark the
  // reduction is netted out of is the mark it is drawn against. Neither is
  // folded into the tail: by magnitude they are the second and sixth largest
  // of the fourteen.
  const contra = app.projection.links.filter((l) => l.contra);
  const contraOK = contra.length === 2 &&
    contra.every((l) => l.target === property && l.value_cents > 0 &&
      l.contra === "printed as a reduction of Property Taxes") &&
    contra.map((l) => l.source).sort().join() ===
      "revenue-line/taxes/property/eraf,revenue-line/taxes/property/rpttf-reduction";
  // THE SIGNED SUM OVER THE DRAWN ROLLUPS IS THE SPINE'S CELL. The spine
  // prints the category once per fund group, netted; the rollups are that
  // money before netting, and a contra ribbon subtracts what it draws.
  const intoCentre = app.projection.links.filter((l) => l.target === property);
  const drawnSum = intoCentre.reduce((sum, l) => sum + (l.contra ? -l.value_cents : l.value_cents), 0);
  const outOfCentre = app.projection.links.filter((l) => l.source === property);
  const leavingSum = outOfCentre.reduce((sum, l) => sum + l.value_cents, 0);
  const spineSum = spine.links.filter((l) => l.source === property).reduce((sum, l) => sum + l.value_cents, 0);
  // AND THE CENTRE'S MARK SAYS WHAT IT IS GROSS OF. Read off the golden: every
  // rollup of the category, signed.
  const rollupsOf = golden.links.filter((l) => l.target === property &&
    l.source.startsWith("revenue-line/taxes/property/"));
  const centreNet = rollupsOf.reduce((sum, l) => sum + l.value_cents, 0);
  const centreReduced = rollupsOf.filter((l) => l.value_cents < 0).reduce((sum, l) => sum - l.value_cents, 0);
  // THE SIZE OF THE MARK ITSELF, not only what its note says about it. A
  // contra ribbon is drawn at its magnitude and so ENTERS the category, which
  // puts the gross ABOVE the additions rather than at them: on FY2025-26 the
  // centre is $103,430,092, the $86,444,753 of additions plus the $16,985,339
  // of reductions, and the note subtracts the reductions twice to reach p127's
  // $69,459,414 -- which is the figure the two ribbons LEAVING the centre come
  // to. The three are pinned in contraNote's own comment and checked against
  // it below, so quoting the wrong one of them fails here.
  const centreGross = rollupsOf.reduce((sum, l) => sum + Math.abs(l.value_cents), 0);
  const centre = laid1.nodes.find((n) => n.id === property);
  const eraf = laid1.links.find((l) => l.source.id === "revenue-line/taxes/property/eraf");
  // ERAF'S FIGURE OFF THE GOLDEN, NOT TYPED: it is $15,175,000 in FY2025-26
  // and $15,857,875 in FY2026-27, and a figure typed for one column would pin
  // the other's arm to the wrong year.
  const erafPrinted = -golden.links.find((l) => l.source === "revenue-line/taxes/property/eraf" &&
    l.target === property).value_cents;
  // EACH MARK IS LOOKED UP BEFORE IT IS ASKED FOR A TOOLTIP, because a
  // declaration that stopped drawing one would otherwise throw out of the
  // module and report nothing about the other arms -- measured, dropping Keep
  // from the step took the whole file's output down to one line.
  const tipOn = (/** @type {any} */ mark) => {
    if (!mark) return "";
    app.showTip({ target: app.dom.byId.get("chart"), clientX: 0, clientY: 0 }, mark);
    return text(app.dom.byId.get("tooltip"));
  };
  const centreTip = tipOn(centre);
  // AND NO SUCH NOTE SITS ON THE KEPT FUND GROUP. The right-hand column is the
  // spine's, whose cells for this category are already net of the two
  // reductions, so a gross-of sentence there would be arithmetic about a
  // figure nothing on this chart takes anything off.
  const keptGeneral = laid1.nodes.find((n) => n.id === "fund-group/general");
  const keptTip = tipOn(keptGeneral);
  const erafTip = tipOn(eraf);
  if (eraf) app.pin(eraf);
  const erafPanel = eraf ? text(app.dom.byId.get("detail")) : "";
  const erafRow = body.children.find((tr) => tr.className === "contra" &&
    tr.children[0].textContent === "ERAF");
  const erafCells = erafRow ? erafRow.children.map((td) => td.textContent) : [];
  const erafLine = laid1.nodes.find((n) => n.id === "revenue-line/taxes/property/eraf");

  // ESCAPE CLOSES IT, INNERMOST FIRST: the pin the panel check left is what
  // the first press clears, and the rung is what the second closes.
  app.dom.document.activeElement = app.dom.document.getElementById("chart");
  const escape = () => { for (const fn of app.dom.documentListeners.keydown || []) fn({ key: "Escape" }); };
  escape();
  const unpinned = words(app);
  escape();
  const back0 = words(app);

  // DEPTH 0 ONLY: under an opened fund group the categories are context, and
  // the step's `after` is the spine's chart and not the group's.
  await mustOpen(app, "fund-group/general");
  const categoryUnderGroup = app.projection.nodes.find((n) => n.id === property);
  const opensUnderGroup = Boolean(categoryUnderGroup) && app.drillable(categoryUnderGroup);
  app.drillUp(0);

  out.push({
    name: `${col.label} category: every revenue category opens from the spine and neither tier-0 endpoint does`,
    ok: categories.length === 10 && categories.every(opens) && !opens("transfers/in") &&
        !opens("fund-balance/draw") && opens("fund-group/general") &&
        columns0.join("|") === OPENABLE_COLUMNS && !opensUnderGroup,
    detail: `${categories.filter(opens).length} of ${categories.length} categories open; transfers/in ` +
      `${opens("transfers/in") ? "WRONGLY opens" : "does not open"}, fund-balance/draw ` +
      `${opens("fund-balance/draw") ? "WRONGLY opens" : "does not open"}; openable columns ` +
      `${JSON.stringify(columns0)}; under an opened group the category ` +
      `${opensUnderGroup ? "WRONGLY opens" : "does not open"}`,
  });
  out.push({
    name: `${col.label} category: Property Taxes opens into a window whose centre it is, and every sentence says so`,
    ok: at1.depth === 1 && !at1.drawnIsYears &&
        JSON.stringify(placed) === JSON.stringify(step.tiers) &&
        JSON.stringify(middle) === JSON.stringify([property]) &&
        flank.length === want.views[property][3] &&
        asked1.length === asked0.length + 1 && asked1[asked1.length - 1] === `data/${col.step}.json` &&
        m1.nodes === want.nodes && m1.links === want.links && m1.hairlines === want.hairlines &&
        rows1 === want.links && at1.counts === want.counts &&
        at1.title === `Sankey diagram of the ${col.label} adopted budget, opened into Property Taxes category` &&
        at1.crumbControls.join("|") === "← All revenue categories" && at1.crumbHere === "Property Taxes category" &&
        at1.hint === "This is Property Taxes category, broken into its parts. Nothing here " +
          "opens further; go back to open another. A single click, or Space, follows one node's money." &&
        at1.legend === 0 && !anyOpens1 &&
        at1.desc === "Opened into Property Taxes category. " + step.description +
          " Use the breadcrumb above the chart, or press Escape, to go back. " + pointer &&
        unpinned.depth === 1 && back0.depth === 0 && back0.legend === 6 && back0.crumbHidden && back0.drawnIsYears,
    detail: `${m1.nodes} nodes, ${m1.links} links, ${m1.hairlines} under 1px (want ${want.nodes}/${want.links}/` +
      `${want.hairlines}) in columns ${JSON.stringify(placed)} left to right (want ` +
      `${JSON.stringify(step.tiers)}), the middle one ${JSON.stringify(middle)} beside ${flank.length} ` +
      `kept fund group(s); counts "${at1.counts}" (want "${want.counts}"); ` +
      `title "${at1.title}"; breadcrumb ${JSON.stringify(at1.crumbControls)} + "${at1.crumbHere}"; hint ` +
      `"${at1.hint}"; legend ${at1.legend}; desc ${at1.desc.startsWith("Opened into Property Taxes category. " +
        step.description) ? "carries" : "LACKS"} the step's description; fetched ` +
      `${JSON.stringify(asked1.slice(asked0.length))}; Escape once (pinned) leaves depth ${unpinned.depth}, twice ` +
      `depth ${back0.depth} with legend ${back0.legend}`,
  });
  out.push({
    name: `${col.label} category: the two contra rows run into the centre, whose mark is gross of them and says what it nets to`,
    ok: contraOK && drawnSum === spineSum && leavingSum === spineSum && Boolean(centre) && Boolean(eraf) &&
        centreGross === want.gross && centreReduced === want.reduced && centreNet === want.net &&
        centre.value === centreGross && centreTip.includes(fmtDollars(centreGross)) &&
        centreTip.includes(fmtDollars(centreReduced)) && centreTip.includes(fmtDollars(centreNet)) &&
        centreTip.includes("◇ our reading") && quotesFigures(app, col, centreGross, centreReduced, centreNet) &&
        Boolean(keptGeneral) && !keptTip.includes("printed as reductions") &&
        erafPrinted > 0 && erafTip.includes("−" + fmtDollars(erafPrinted)) &&
        erafTip.includes("reduction") && erafTip.includes("printed as a reduction of Property Taxes") &&
        erafPanel.includes("printed as a reduction of Property Taxes") &&
        erafCells.length > 0 && erafCells[2] === "−" + fmtDollars(erafPrinted) &&
        erafCells[4] === "printed as a reduction of Property Taxes" &&
        app.linkClass(eraf) === "link contra" && Boolean(erafLine) && erafLine.value === erafPrinted,
    detail: `${contra.length} contra ribbon(s): ${contra.map((l) => l.source.split("/").pop() + " " +
        l.value_cents + " into " + l.target + " (" + l.contra + ")").join(", ")}; signed sum into the centre ` +
      `${drawnSum} and out of it ${leavingSum}, the spine's cell ${spineSum}; the centre is sized at ` +
      `${centre ? centre.value : "nothing"} (want ${want.gross}) and app.js ` +
      `${quotesFigures(app, col, centreGross, centreReduced, centreNet) ? "quotes" : "MISQUOTES"} that; ` +
      `its tooltip ${centreTip.includes(fmtDollars(centreNet)) ?
        "names" : "DOES NOT name"} ${fmtDollars(centreNet)} net of ${fmtDollars(centreReduced)}, and the kept ` +
      `General Fund's ${keptTip.includes("printed as reductions") ? "WRONGLY carries" : "carries no"} ` +
      `reductions note; ERAF's tooltip ` +
      `${erafTip.includes("−") ? "carries the sign" : "LACKS the sign"} and ` +
      `${erafTip.includes("printed as a reduction of Property Taxes") ? "the sentence" : "NOT the sentence"}; ` +
      `its table row reads ${JSON.stringify(erafCells.slice(0, 5))}; class "${eraf ? app.linkClass(eraf) : ""}"`,
  });

  // THE CAP ON THE CATEGORY'S ONE FOLDABLE COLUMN. Charges for Services prints
  // 19 lines, so the line column folds; Use of Money and Property prints 7
  // into 5 fund groups and Fines & Forfeitures 2 into 1, so neither folds
  // anything -- the fund column that used to fold for both of them is gone,
  // and the right-hand column is the spine's six groups.
  const capsOf = async (/** @type {string} */ id) => {
    await at(app, id);
    const aggs = app.projection.nodes.filter((n) => app.isAggregate(n.id));
    const byTier = Object.fromEntries(aggs.map((a) => [a.tier, a]));
    return { aggs, byTier, m: measure(app, app.projection), laid: app.layOut(app.projection) };
  };
  const charges = await capsOf("revenue/charges-for-services");
  // TOP EIGHT BY OUTFLOW OVER THE LINKS THIS VIEW DRAWS, off the golden: a
  // line's rollups into its own category, which is its whole outflow here.
  // What inflow would have kept is the contrast that makes the arm mean
  // something -- a line takes in nothing, so under inflow every line ties at
  // zero and the eight kept are the first eight ids.
  const chargeLines = golden.nodes.filter((n) => n.parent === "revenue/charges-for-services").map((n) => n.id);
  const outflow = new Map(chargeLines.map((id) => [id, 0]));
  for (const l of golden.links) {
    if (outflow.has(l.source) && l.target === "revenue/charges-for-services") {
      outflow.set(l.source, outflow.get(l.source) + Math.abs(l.value_cents));
    }
  }
  const byOutflow = chargeLines.slice().sort((a, b) => outflow.get(b) - outflow.get(a) || (a < b ? -1 : 1)).slice(0, 8);
  const byInflow = chargeLines.slice().sort().slice(0, 8);
  const keptLines = app.projection.nodes.filter((n) => n.tier === 1 && !app.isAggregate(n.id)).map((n) => n.id);
  const lineAgg = charges.byTier[1];
  out.push({
    name: `${col.label} category: Charges for Services folds its line column alone, the tail in the step's own noun and parented to the category`,
    ok: charges.aggs.length === 1 && Boolean(lineAgg) && lineAgg.id === app.aggregateID(1) &&
        lineAgg.label === want.chargesLineTail &&
        lineAgg.parent === "revenue/charges-for-services" &&
        lineAgg.derived === true && lineAgg.rationale !== "" && lineAgg.source_note !== "" &&
        Array.isArray(lineAgg.folds) && lineAgg.folds.length === Number(lineAgg.label.split(" ")[0]) &&
        keptLines.slice().sort().join() === byOutflow.slice().sort().join() &&
        byInflow.slice().sort().join() !== byOutflow.slice().sort().join() &&
        charges.m.links === want.chargesLinks,
    detail: charges.aggs.length === 1
      ? `tier 1 "${lineAgg.label}" (want "${want.chargesLineTail}") parent ${JSON.stringify(lineAgg.parent)}; ` +
        `${charges.m.links} ribbons (want ${want.chargesLinks}); the eight lines kept are the top eight by ` +
        `outflow ${keptLines.slice().sort().join() === byOutflow.slice().sort().join() ? "yes" : "NO"}, and ` +
        `inflow would have kept ${byInflow.filter((id) => !byOutflow.includes(id)).length} other(s)`
      : `${charges.aggs.length} aggregate(s): ${JSON.stringify(charges.aggs.map((a) => a.label))}`,
  });
  const money = await capsOf("revenue/use-of-money-and-property");
  const fines = await capsOf("revenue/fines-and-forfeitures");
  const finesFlank = fines.laid.nodes.find((n) => n.tier === 2);
  const finesCentre = fines.laid.nodes.find((n) => n.tier === 0);
  const finesLine = fines.laid.nodes.find((n) => n.tier === 1);
  out.push({
    name: `${col.label} category: neither cap engages on Use of Money and Property or Fines & Forfeitures, and no column of one claims a share`,
    ok: money.aggs.length === 0 && money.m.links === want.moneyLinks &&
        fines.aggs.length === 0 && fines.m.links === 3 && fines.m.nodes === 4 &&
        Boolean(finesFlank) && app.columnShare(finesFlank) === "" &&
        Boolean(finesCentre) && app.columnShare(finesCentre) === "" &&
        Boolean(finesLine) && app.columnShare(finesLine).startsWith("◇ our "),
    detail: `Use of Money: ${money.aggs.length} aggregate(s), ${money.m.links} ribbons (want ` +
      `${want.moneyLinks}); Fines: ${fines.aggs.length} aggregate(s), ${fines.m.links} ribbons over ` +
      `${fines.m.nodes} nodes; the one kept fund group's share reads ` +
      `${JSON.stringify(finesFlank ? app.columnShare(finesFlank) : "")}, the centre's ` +
      `${JSON.stringify(finesCentre ? app.columnShare(finesCentre) : "")} and a line's ` +
      `${JSON.stringify(finesLine ? app.columnShare(finesLine) : "")}`,
  });

  // EVERY CATEGORY OPENS, IN BOTH YEARS, at the pinned shape -- and at the
  // SAME shape in both, which is a property of the right-hand column being
  // fund GROUPS: fund/207 prints a dash in FY2026-27 and that moves a fund
  // without moving the group it belongs to.
  const shapes = {};
  const refused = [];
  const balances = [];
  const ownRollups = [];
  for (const id of Object.keys(want.views)) {
    app.drillUp(0);
    const o = await openInto(app, id);
    if (o !== "drew") { refused.push(id + ": " + o); continue; }
    const drawn = app.projection;
    const m = measure(app, drawn);
    shapes[id] = [m.nodes, m.links, m.hairlines, drawn.nodes.filter((n) => n.tier === 2).length];
    // THE CENTRE BALANCES, SIGNED, AND THAT IS THE IDENTITY THE WINDOW DRAWS.
    // The left half is pp.127-140's rollups into the category and the right is
    // the spine's own cells for it; markGap reads exactly this difference, so
    // a category that stopped tying would be a gap mark on a step that
    // declares none -- which throws rather than drawing.
    const into = drawn.links.filter((l) => l.target === id)
      .reduce((sum, l) => sum + (l.contra ? -l.value_cents : l.value_cents), 0);
    const outOf = drawn.links.filter((l) => l.source === id).reduce((sum, l) => sum + l.value_cents, 0);
    const cell = spine.links.filter((l) => l.source === id).reduce((sum, l) => sum + l.value_cents, 0);
    if (into !== cell || outOf !== cell) balances.push(`${id}: ${into} in, ${outOf} out, spine ${cell}`);
    // AND EVERY ROLLUP IT DRAWS IS ITS OWN CATEGORY'S. A window that kept
    // another category's rollup would draw a ribbon into a mark that is not on
    // the chart, which rollupsIn alone cannot see.
    if (drawn.links.some((l) => l.source.startsWith("revenue-line/") && l.target !== id)) ownRollups.push(id);
  }
  const asDrawn = JSON.stringify(shapes, Object.keys(shapes).sort());
  const asPinned = JSON.stringify(want.views, Object.keys(want.views).sort());
  // AND THE TWO-KIND LINE IS TWO RIBBONS FROM ONE ROW, each its own kind.
  // pp.127-140 print 2 of their 93 rows reaching the five Internal Service
  // Funds as an internal service charge and the rest of the city as external
  // revenue, and internal/project publishes a rollup per (row, kind) for
  // exactly that reason, so the pair carrying both kinds is the printed ROW
  // itself, which is where the document puts it.
  await at(app, "revenue/use-of-money-and-property");
  const twoKind = app.projection.links.filter((l) =>
    l.source === "revenue-line/use-of-money-and-property/use-of-money-and-prop" &&
    l.target === "revenue/use-of-money-and-property");
  const tailKinds = twoKind.map((l) => l.kind).sort();
  out.push({
    name: `${col.label} category: all ten categories open at their pinned shapes, and a row spanning two kinds draws one ribbon per kind`,
    ok: refused.length === 0 && asDrawn === asPinned && tailKinds.join() === "external,internal_service",
    detail: (refused.length ? `refused: ${refused.join("; ")}; ` : "") +
      (asDrawn === asPinned ? `all ${Object.keys(shapes).length} at the pinned shapes` : `drawn ${asDrawn}, want ${asPinned}`) +
      `; the Use of Money row reaches its category ${twoKind.length} time(s), kinds ${JSON.stringify(tailKinds)}`,
  });
  out.push({
    name: `${col.label} category: every window's centre takes in what it sends out, and that is the spine's own cell for it`,
    ok: refused.length === 0 && balances.length === 0 && ownRollups.length === 0 &&
        Object.keys(shapes).length === Object.keys(want.views).length,
    detail: (balances.length
      ? `${balances.length} centre(s) do not tie: ${balances.join("; ")}`
      : `all ${Object.keys(shapes).length} centres tie to the spine's cell to the cent, signed`) +
      (ownRollups.length ? `; ${ownRollups.join(", ")} draw a rollup into another category` : ""),
  });
  app.drillUp(0);
  return out;
}

/**
 * markGap's own arithmetic over the category window, which declares no gap:
 * the centre balances, so nothing is drawn, and a centre that did not would
 * throw rather than leave the difference as node height.
 *
 * THE STEP DECLARES NO GAP AND SO markGap RETURNS AT ITS FIRST LINE -- which
 * is exactly why the claim "the centre balances" needs checking somewhere the
 * gap reader can see. This splices an empty gap map onto the shipped step,
 * which is the declaration a step would carry the day one of these cells
 * drifted, and then asks markGap the question it would ask.
 *
 * THE SIDES IT COMPARES ARE SIGNED, AND THAT IS THE WHOLE OF WHY THIS PASSES.
 * A reduction is drawn forward at its magnitude, so a Property Taxes centre
 * measured after markContra takes $103,430,092 and sends $69,459,414 -- the
 * shape markGap exists to refuse. shapeFor runs it BEFORE markContra, on the
 * figures pp.127-140 print, and the mutation below is that ordering: with the
 * spine's cell for the category moved by a dollar, the same call refuses by
 * name.
 */
async function gapAtTheCentre() {
  const out = [];
  const CENTRE = "revenue/taxes/property";
  const withGaps = (/** @type {any} */ config) => {
    config.steps = config.steps.map((s) => (s.key === "revenue-category"
      ? Object.assign({}, s, { gaps: {} }) : s));
  };
  for (const col of COLUMNS) {
    const { app } = await opened(null, withGaps, col);
    const drew = await openInto(app, CENTRE);
    const marks = drew === "drew" ? app.projection.nodes.filter((n) => app.isGap(n.id)) : [];
    // THE MUTATION, RUN RATHER THAN DESCRIBED: the spine's own cell for the
    // category moved by one dollar makes the kept flank disagree with the
    // rollups, and the step names no reason for it.
    const { app: bentApp, main: bentMain } = await opened({
      [`data/${col.stem}.json`]: {
        doc: (() => {
          const doc = col.spine();
          doc.nodes.find((n) => n.id === "fund-group/general").label = "General Fund group";
          doc.nodes.find((n) => n.id === CENTRE).label = "Property Taxes category";
          for (const n of doc.nodes) if (n.id.startsWith("expenditure/")) n.label = n.label + " category";
          const link = doc.links.find((l) => l.source === CENTRE);
          link.value_cents += 100;
          return doc;
        })(),
      },
    }, withGaps, col);
    const refused = await openInto(bentApp, CENTRE);
    const said = refusals(bentMain).map((b) => b.textContent).join(" | ");
    out.push({
      name: `${col.label} category: the window's centre balances under markGap's own arithmetic, and a cent of drift is refused by name`,
      ok: drew === "drew" && marks.length === 0 && refused === "failed" &&
        said.includes("that no declaration on this step accounts for"),
      detail: `with an empty gap map declared the click came to "${drew}" and the centre drew ` +
        `${marks.length} gap mark(s); with the spine's cell for it moved by $1.00 it came to ` +
        `"${refused}"` + (said ? `, saying "${said}"` : " and said nothing"),
    });
  }
  return out;
}

/**
 * Whether contraNote's own comment in site/app.js quotes the three figures
 * this column measures off the golden.
 *
 * MATCHED AS WHOLE QUOTED PHRASES, layout.mjs's rule: a bare includes() of a
 * figure is satisfied by that figure appearing anywhere in a 3,700-line file,
 * so each phrase carries enough of its own sentence to be unique.
 *
 * ON THE YEAR THE COMMENT NAMES AND NO OTHER. The sentence says FY2025-26 and
 * FY2026-27's gross is a different number, so the second column asserts that
 * the comment does NOT claim to be about it rather than asserting nothing.
 */
function quotesFigures(app, col, gross, reduced, net) {
  const year = col.label.replace("FY ", "FY");
  if (col.stem !== "sankey") return !app.source.includes("Measured on " + year + ":");
  return [
    "Measured on " + year + ":",
    `sized at ${fmtDollars(gross)}`,
    `${fmtDollars(reduced)} of reductions among ${fmtDollars(gross - reduced)}`,
    `p127 prints ${fmtDollars(net)}`,
  ].every((q) => app.source.includes(q));
}

/**
 * What the client does with a step whose `after` names SEVERAL charts: opens
 * from each of them, by membership rather than by equality.
 *
 * THE CLIENT WALKS THE LIST BEFORE ANYTHING ASKS IT TO DRAW ONE. export
 * .DrillStep.After is []string and reaches app.js through its JSON tag, so the
 * wire carries `["fund-group"]` where it carried `"fund-group"`; read with
 * `!==` every step would have failed to match and nothing on the page would
 * have opened, with every Go test green. No step the site ships names two
 * charts yet -- this is measured on the shipped shape with one field replaced.
 *
 * FOUR READS AND NOT ONE, because "the fund opens" is satisfied by a client
 * that opens everything: the same step reached through a list that does NOT
 * name the rung on screen must NOT open, and a bare string -- the shape the
 * wire carried before this -- must be dropped by STEPS rather than quietly
 * matched by `.includes` on a string, which would be true of "fund-group" and
 * of "und-grou" alike.
 *
 * THE SUBJECT IS THE FUND STEP, which is the one hanging off the fund group's
 * chart: its `after` is what is replaced, and fund/100 is the mark that opens
 * or does not.
 */
async function severalParents() {
  const read = async (/** @type {any} */ after) => {
    const { app } = await opened(null, (config) => {
      config.steps = config.steps.map((s, i) => (i === 1 ? Object.assign({}, s, { after }) : s));
    });
    const outcome = await openInto(app, "fund-group/general");
    const laid = outcome === "drew" ? app.layOut(app.projection) : null;
    const fund = laid ? laid.nodes.find((/** @type {any} */ n) => n.id === "fund/100") : null;
    return {
      after, steps: app.STEPS.length, outcome,
      opens: Boolean(fund) && app.drillable(fund),
    };
  };
  const got = [
    await read(["fund-group"]),
    await read(["nope", "fund-group"]),
    await read(["nope"]),
    await read("fund-group"),
  ];
  const [shipped, member, stranger, asString] = got;
  return [{
    name: "a step opens from every chart its `after` names, and from no other -- membership, so one view can be reached from several",
    ok: got.every((r) => r.outcome === "drew") &&
      shipped.steps === PAGE.steps.length && shipped.opens &&
      member.steps === PAGE.steps.length && member.opens &&
      stranger.steps === PAGE.steps.length && !stranger.opens &&
      asString.steps === PAGE.steps.length - 1 && !asString.opens,
    detail: got.map((r) => `${JSON.stringify(r.after)}: ${r.steps} step(s) read, the spine ` +
      `${r.outcome}, fund/100 ${r.opens ? "opens" : "does not open"}`).join("; "),
  }];
}

/**
 * What the client does with a step the wire declares without a key or without
 * an `after`: drops it, so nothing opens, rather than reading either as "".
 *
 * THE CONSEQUENCE IS THE WHOLE DRILL, WHICH IS WHY IT IS MEASURED HERE AND NOT
 * ONLY GO-SIDE. export.DrillStep's `after` reaches the client only through its
 * JSON tag; dropped back to `json:"-"` the config carries no `after` at all,
 * every step falls out of STEPS, and the page a reader gets isolates on a
 * click with no sign that it ever opened anything.
 *
 * AND READ AS A ROOT INSTEAD, A KEYLESS STEP MATCHES ITSELF. A list carrying
 * "" is the root marker, so a step with no key becomes its own parent --
 * measured under a config whose steps carried none, Patrol opened into Patrol
 * without end. Dropping is what makes that unrepresentable, and it is why the
 * client tests for an ARRAY rather than for a truthy value: a dropped field is
 * `undefined`, which is not one. The packager is what refuses the same thing
 * one side over (export.validateSteps, and seam.mjs's `noKey` arm on the
 * parse).
 */
async function keylessSteps() {
  const without = async (/** @type {string} */ field) => {
    const { app, main } = await opened(null, (config) => {
      config.steps = config.steps.map((s) => {
        const copy = Object.assign({}, s);
        delete copy[field];
        return copy;
      });
    });
    const nodeAt = (/** @type {string} */ id) => app.projection.nodes.find((n) => n.id === id);
    const outcome = await openInto(app, "fund-group/general");
    return {
      field, steps: app.STEPS.length,
      groupOpens: app.drillable(nodeAt("fund-group/general")),
      categoryOpens: app.drillable(nodeAt("revenue/taxes/property")),
      columns: app.openableColumns(), outcome, depth: app.drilled.length,
      banners: refusals(main).length,
    };
  };
  // THE CONTROL FIRST, so this arm cannot be green because the fixture stopped
  // drawing: the same page with both fields present opens two things.
  const { app: control } = await opened(null, null);
  const controlOpens = control.STEPS.length === PAGE.steps.length &&
    control.openableColumns().join("|") === OPENABLE_COLUMNS;
  const got = [await without("key"), await without("after")];
  return [{
    name: "a step the wire declares with no key, or with no `after`, is dropped rather than read as a root -- so the drill vanishes instead of opening a node into itself",
    ok: controlOpens && got.every((r) => r.steps === 0 && !r.groupOpens && !r.categoryOpens &&
      r.columns.length === 0 && r.outcome === "failed" && r.depth === 0 && r.banners === 0),
    detail: `with both fields the page reads ${control.STEPS.length} step(s) and offers ` +
      `${JSON.stringify(control.openableColumns())}; ` +
      got.map((r) => `without \`${r.field}\`: ${r.steps} step(s), fund group ` +
        `${r.groupOpens ? "STILL opens" : "does not open"}, category ` +
        `${r.categoryOpens ? "STILL opens" : "does not open"}, columns ` +
        `${JSON.stringify(r.columns)}, a click came to "${r.outcome}" at depth ${r.depth} ` +
        `with ${r.banners} banner(s)`).join("; "),
  }];
}

/* ------------------------------------------------------------------ *
 * The window: three columns spliced on the node the reader clicked
 * ------------------------------------------------------------------ */

/** The ids drawn at one tier of the chart on screen, in document order. */
function atTier(app, tier) {
  return app.projection.nodes.filter((n) => n.tier === tier).map((n) => n.id);
}

/**
 * Everything the window shaping claims, measured through drillDown over the
 * committed goldens and over the SHIPPED steps alone.
 *
 * NO FIXTURE STEP AT ALL SINCE LANE G, which is what the site growing a real
 * chain of windows buys. This drove a copy of the fund-group step declaring
 * `after: ["", "revenue-category"]`, because nothing shipped slid twice; the
 * spine's fund group now opens into a window and its FUND opens into another,
 * so the rung's recorded chart, carriedSource by stem and the gates two rungs
 * down are all reached through data.go's own declarations.
 *
 * AND THE SECOND EDGE THAT FIXTURE MODELLED IS NOW UNDECLARABLE. A fund group
 * kept on a revenue category's flank is drawn at THAT category's share of it,
 * so a step opening it would draw one figure in and the group's whole
 * decomposition out -- measured over both columns at up to 157,797,110 of node
 * height with no ribbon under it. export.validateSteps refuses the declaration
 * by name, and Go's TestWriteRefusesAnUnrenderableViewSet is where that is
 * held; here the consequence is asserted instead, on the chart: a kept mark
 * opens nothing.
 */
async function windowChecks() {
  const out = [];
  const CENTRE = "fund-group/general";
  const { app } = await opened();
  const step = PAGE.steps[0];
  const spine = goldenGraph();
  const kept = spine.links.filter((l) => l.target === CENTRE);

  const outcome = await openInto(app, CENTRE);
  const drawn = outcome === "drew" ? app.projection : { nodes: [], links: [] };
  const tiers = [...new Set(drawn.nodes.map((n) => n.tier))].sort((a, b) => a - b);
  const centreColumn = atTier(app, 2);
  const keptColumn = atTier(app, 0);
  const freshColumn = atTier(app, 3);
  const fromKept = drawn.links.filter((l) => l.target === CENTRE);
  const toFunds = drawn.links.filter((l) => l.source === CENTRE);
  const laid = outcome === "drew" ? app.layOut(drawn) : null;
  // The columns left to right, as d3 placed them, read back as tiers.
  const placed = laid
    ? [...new Set(laid.nodes.slice().sort((a, b) => a.x0 - b.x0).map((n) => n.tier))]
    : [];
  // THE KEPT COLUMN IS THE SPINE'S OWN, LESS WHAT THE RESIDUAL TOOK PAST THE
  // CENTRE: every tier-0 mark the spine draws into the group is still on
  // screen, and two of their ribbons end on the residual instead of on the
  // group. So the column's WIDTH is the spine's and the centre's inflow is
  // not, which is the one place those two numbers part company.
  const residualEnds = drawn.links
    .filter((l) => app.isResidual(l.target)).map((l) => l.source);
  out.push({
    name: "a window is three columns spliced on the node the reader clicked, its centre alone in the middle",
    ok: outcome === "drew" && JSON.stringify(tiers) === "[0,2,3]" &&
      JSON.stringify(centreColumn) === JSON.stringify([CENTRE]) &&
      keptColumn.length === kept.length && freshColumn.length === 2 &&
      fromKept.length === kept.length - residualEnds.length && toFunds.length === 1 &&
      JSON.stringify(placed) === JSON.stringify(step.tiers),
    detail: outcome === "drew"
      ? `tiers ${JSON.stringify(tiers)} drawn left to right as ${JSON.stringify(placed)}; ` +
        `centre column ${JSON.stringify(centreColumn)}; kept column ${keptColumn.length} ` +
        `node(s) sending ${fromKept.length} ribbon(s) into the centre and ` +
        `${residualEnds.length} past it; opened column ${freshColumn.length} node(s) taking ` +
        `${toFunds.length} from it`
      : `the fund group would not open: ${outcome}`,
  });

  // THE KEPT FLANK'S RECORDS ARE THE SPINE'S, WHICH IS WHAT PROVES THE SOURCE.
  // Both committed documents carry fund-group/general and print the same label
  // for it, so opened() relabels the SPINE's copy -- a flank read off the step
  // document would name it "General Fund" and this would say so. The figures
  // are the spine's cells to the cent, which no filter of fund-flows produces.
  const labels = new Map(drawn.nodes.map((n) => [n.id, n.label]));
  // THE SPINE AS THE PAGE HOLDS IT, which is the fixture's relabelled copy and
  // not goldenGraph()'s: opened() renames exactly the ids both documents print
  // the same words for, so "off the chart on screen" and "off the file" give
  // different answers here and this can tell them apart.
  const onScreen = app.docAt(0);
  const spineLabels = new Map(onScreen.nodes.map((n) => [n.id, n.label]));
  const stepLabels = new Map(goldenFundFlows().nodes.map((n) => [n.id, n.label]));
  const named = keptColumn.concat([CENTRE]);
  const sameLabels = named.every((id) => labels.get(id) === spineLabels.get(id));
  const sameValues = kept.every((l) => {
    const drew = drawn.links.find((d) => d.source === l.source &&
      (d.target === CENTRE || app.isResidual(d.target)));
    return Boolean(drew) && drew.value_cents === l.value_cents;
  });
  // AND THE TWO SOURCES DISAGREE, or the arm above is green either way. Over
  // the ids BOTH documents carry: the kept column holds fund-balance/draw,
  // which fund-flows has no node for at all, and "the step document does not
  // name it" is a weaker thing than "the two name it differently".
  const tellsApart = named.filter((id) => stepLabels.has(id) &&
    spineLabels.get(id) !== stepLabels.get(id));
  out.push({
    name: "the kept flank and the centre come off the chart on screen, in its words and at its figures",
    ok: outcome === "drew" && sameLabels && sameValues && tellsApart.length === 2,
    detail: outcome === "drew"
      ? `centre drawn as "${labels.get(CENTRE)}" against the step document's ` +
        `"${stepLabels.get(CENTRE)}"; kept column carries the spine's own cells; ` +
        `${tellsApart.length} of ${named.length} drawn marks are named differently by the ` +
        `two documents, so the source is distinguishable`
      : `the fund group would not open: ${outcome}`,
  });

  // NOTHING ON A KEPT FLANK OPENS, AND THE TWO REASONS ARE SEPARATED. A kept
  // revenue category is carried in the sense carried_from records -- its figure
  // and its caveats are the chart above's -- and it is NOT what isCarried
  // means, which is a declared residual endpoint. Both are shut here, and by
  // different gates: no step names the chart a kept mark is on, so stepFor
  // answers null before isCarried is consulted at all.
  //
  // WHICH MAKES isCarried's ARM OF drillable INERT ON EVERY VIEW THE PAGE
  // OPENS, and that is measured rather than assumed: over the whole walk, both
  // columns, not one carried mark is at a tier a step could open from. It is
  // kept as the second gate and this says so, so that a check reading "nothing
  // carried opens" is not mistaken for evidence that the gate fired.
  const category = drawn.nodes.find((n) => n.tier === 0 && n.role === "revenue_source");
  const endpoint = drawn.nodes.find((n) => n.carried_from && app.isCarried(n.id));
  const fund = drawn.nodes.find((n) => n.id === "fund/100");
  out.push({
    name: "nothing on a kept flank opens, and a residual's declared endpoint does not either -- by two different gates",
    ok: Boolean(category) && category.carried_from === "sankey" && !app.isCarried(category.id) &&
      !app.drillable(category) && app.stepFor(category) === null &&
      Boolean(endpoint) && app.isCarried(endpoint.id) && !app.drillable(endpoint) &&
      Boolean(fund) && !fund.carried_from && app.drillable(fund),
    detail: (category
      ? `${category.id} carried from "${category.carried_from}", isCarried ` +
        `${app.isCarried(category.id)}, a step ${app.stepFor(category) ? "NAMES" : "names"} its ` +
        `chart, drillable ${app.drillable(category)}`
      : "no kept revenue category was drawn, so this asserts nothing") + "; " +
      (endpoint
        ? `${endpoint.id} carried from "${endpoint.carried_from}", isCarried ` +
          `${app.isCarried(endpoint.id)}, drillable ${app.drillable(endpoint)}`
        : "no carried endpoint was drawn, so half of this asserts nothing") +
      `; the one mark that opens is ${fund ? fund.id : "NONE"}`,
  });

  // A WINDOW CHAINS, AND COMES BACK. The kept flank is a filter of the chart
  // that was ON SCREEN, and that chart is gone by the time Escape reshapes the
  // rung -- so the rung records it. Opening the fund and popping back has to
  // land on the same window, mark for mark, or the flank is being recomputed
  // from whatever happens to be drawn.
  const before = JSON.stringify(drawn.nodes.map((n) => n.id + "@" + n.tier)) +
    JSON.stringify(drawn.links.map((l) => l.source + ">" + l.target + "=" + l.value_cents));
  const deeper = await openInto(app, "fund/100");
  const deepTiers = deeper === "drew"
    ? [...new Set(app.projection.nodes.map((n) => n.tier))].sort((a, b) => a - b) : [];
  app.drillUp(1);
  await settle();
  const after = JSON.stringify(app.projection.nodes.map((n) => n.id + "@" + n.tier)) +
    JSON.stringify(app.projection.links.map((l) => l.source + ">" + l.target + "=" + l.value_cents));
  out.push({
    name: "a window's kept flank survives being drilled through and popped back to",
    ok: deeper === "drew" && JSON.stringify(deepTiers) === "[2,3,4]" &&
      app.drilled.length === 1 && after === before,
    detail: `fund/100 opened to "${deeper}" at tiers ${JSON.stringify(deepTiers)}; ` +
      `popping back left ${app.drilled.length} rung drawing a chart that is ` +
      `${after === before ? "identical to" : "DIFFERENT from"} the one it was opened from`,
  });

  // THE STEM IS RESOLVED AGAINST THE STACK, which is what makes a carried mark
  // deeper than one rung resolvable at all. docAt(0) answers "the spine" for
  // every mark on the page, and every window carries a flank, so a chain two
  // deep has carried marks whose chart above is not the spine.
  await at(app, CENTRE, "fund/100");
  const depths = {
    spine: app.depthOfDocument("sankey"),
    step: app.depthOfDocument("fund-flows"),
    absent: app.depthOfDocument("no-such-document"),
  };
  const deepCarried = app.projection.nodes.filter((n) => n.carried_from).map((n) => n.carried_from);
  out.push({
    name: "a carried mark's document is found by the stem it records, at the depth that document is on the stack",
    ok: app.drilled.length === 2 && depths.spine === 0 && depths.step === 2 &&
      depths.absent === -1 && deepCarried.length > 0 &&
      deepCarried.every((s) => s === "fund-flows") &&
      app.carriedSource("sankey").projection === "sankey" &&
      app.carriedSource("fund-flows").projection === "fund-flows" &&
      app.carriedSource("no-such-document") === null &&
      app.caveatHref(SPINE_CAVEATS[0], "no-such-document") === "" &&
      app.caveatHref(SPINE_CAVEATS[0], "sankey") ===
        `caveats.html#caveat-sankey--${SPINE_CAVEATS[0]}`,
    detail: `two rungs deep the stack answers sankey at depth ${depths.spine}, fund-flows at ` +
      `${depths.step} and an unknown stem at ${depths.absent}; the ${deepCarried.length} carried ` +
      `mark(s) here record ${JSON.stringify([...new Set(deepCarried)])} rather than the spine; ` +
      `the spine's caveat anchors to "${app.caveatHref(SPINE_CAVEATS[0], "sankey")}" and an ` +
      `unknown stem to "${app.caveatHref(SPINE_CAVEATS[0], "no-such-document")}"`,
  });

  // FAIL CLOSED ON A DECLARATION THAT IS NOT A WINDOW. Every one of these is
  // refused by export.validateSteps before it could ship, and the client is
  // handed a config rather than a View: a window drawn the wrong way round
  // lays out and means something else, which is the one failure a reader
  // cannot see. The message names the document and says what a window is.
  const rung = { id: CENTRE, doc: goldenFundFlows(), step: step, chart: goldenGraph() };
  const refusedBy = (bad, chart) => {
    try {
      app.windowFor(chart === undefined ? goldenGraph() : chart, goldenFundFlows(),
        Object.assign({}, rung, { step: Object.assign({}, step, bad) }));
      return "";
    } catch (e) {
      return String(e.message);
    }
  };
  const bad = [
    ["no chart on screen to keep a flank of", {}, null],
    ["two columns", { tiers: [0, 2] }, undefined],
    ["the centre is not the opened tier", { tiers: [2, 3, 0] }, undefined],
    ["the kept tier is in the middle", { keep: [2], tiers: [0, 2, 3] }, undefined],
    ["two kept flanks", { keep: [0, 3] }, undefined],
  ].map(([why, step, chart]) => ({ why, said: refusedBy(step, chart) }));
  out.push({
    name: "a declaration that is not a window is refused by name rather than drawn",
    ok: bad.every((b) => b.said.includes("which is not a window")),
    detail: bad.map((b) => `${b.why}: ${b.said ? "refused" : "DREW ANYWAY"}`).join("; "),
  });

  return out;
}

/**
 * The step that answers the owner's third finding: the spine's right-hand
 * column, opened into the divisions that spend it.
 *
 * THE GATE IS THE ROLE AND NOT THE TIER, which is the mirror of the revenue
 * step's at the other end of the chart. Four of the spine's seven tier-5 nodes
 * are object categories pp.85-125 decompose; the other three -- transfers/out
 * and the two fund-balance rows -- are flow ends, and the Transfers Out row
 * those pages do print is a dash in both budget columns. Deleting Role from
 * the step opens all seven: measured, the three then draw a window whose
 * opened column is empty and filterLinks refuses each BY NAME.
 *
 * EVERY FIGURE IS MEASURED BY HAND PER PUBLISHED YEAR (COLUMNS[*].object) and
 * not derived from the documents here, for the file's reason: a pin computed
 * the way the code computes it agrees with the code by construction.
 */
async function objectCategoryChecks() {
  const out = [];
  const ENDS = ["transfers/out", "fund-balance/contribution", "fund-balance/reserve-increase"];
  for (const col of COLUMNS) {
    const { app, body } = await opened(null, null, col);
    const pins = col.object;
    const ids = Object.keys(pins.views);

    // WHAT OPENS IN THE RIGHT-HAND COLUMN, asked of the drawn spine.
    const at5 = app.projection.nodes.filter((n) => n.tier === 5);
    const opens = (/** @type {string} */ id) => {
      const n = at5.find((x) => x.id === id);
      return Boolean(n) && app.drillable(n);
    };
    out.push({
      name: `${col.label} object: the spine's four object categories open and its three flow ends do not`,
      ok: at5.length === ids.length + ENDS.length && ids.every(opens) &&
        ENDS.every((id) => !opens(id)) &&
        app.openableColumns().join("|") === OPENABLE_COLUMNS,
      detail: `the spine draws ${at5.length} node(s) in its right-hand column; ` +
        `${ids.filter(opens).length} of ${ids.length} object categories open and ` +
        `${ENDS.filter(opens).length} of ${ENDS.length} flow ends do; openable columns ` +
        `${JSON.stringify(app.openableColumns())}`,
    });

    // EVERY ONE OF THE FOUR, NOT A SAMPLE, and the shape is the whole window:
    // the fund groups that pay for it on the left, the category in the middle
    // alone, the divisions on the right, laid out in the order the step
    // declares rather than in tier order.
    /** @type {Record<string, any>} */
    const got = {};
    for (const id of ids) {
      await at(app);
      const outcome = await openInto(app, id);
      if (outcome !== "drew") {
        got[id] = { outcome };
        continue;
      }
      const d = app.projection;
      const laid = app.layOut(d);
      const widths = laid.links.map((l) => l.width);
      got[id] = {
        outcome,
        shape: [d.nodes.length, d.links.length, widths.filter((w) => w < 1).length,
          d.nodes.filter((n) => n.tier === 2).length],
        placed: [...new Set(laid.nodes.slice().sort((a, b) => a.x0 - b.x0).map((n) => n.tier))],
        centre: d.nodes.filter((n) => n.tier === 5).map((n) => n.id),
        // THE DESCRIPTION'S OWN CLAIM, CHECKED: "no division here takes the
        // colour of a fund group". pp.85-125 print what a division spends
        // whatever pays for it, so every tier-4 mark is parentless and draws
        // --muted, and the sentence under the chart says so.
        grouped: d.nodes.filter((n) => n.tier === 4 && n.parent).map((n) => n.id),
        tail: (d.nodes.find((n) => app.isAggregate(n.id)) || {}).label || "",
      };
      await at(app);
    }
    const shapeOf = (/** @type {string} */ id) => (got[id].shape || []).join(",");
    out.push({
      name: `${col.label} object: all four categories draw as three columns -- the groups that fund it, the category, the divisions that spend it`,
      ok: ids.every((id) => got[id].outcome === "drew" &&
        shapeOf(id) === pins.views[id].join(",") &&
        got[id].placed.join(",") === "2,5,4" &&
        got[id].centre.join(",") === id &&
        got[id].grouped.length === 0 &&
        got[id].tail === (pins.tails[id] || "")),
      detail: ids.map((id) => `${id}: ${got[id].outcome === "drew"
        ? `[${shapeOf(id)}] want [${pins.views[id].join(",")}], columns ` +
          `${JSON.stringify(got[id].placed)}, centre ${JSON.stringify(got[id].centre)}, ` +
          `${got[id].grouped.length} division(s) carrying a fund group, tail ` +
          `"${got[id].tail}"`
        : `would not open: ${got[id].outcome}`}`).join("; "),
    });

    // THE CAP IS DOING THE WORK, unlike the division cap one step over. Two of
    // the four columns fold and two are drawn whole, and both facts are pinned:
    // a check asserting the cap engages everywhere would fail on debt-services
    // for being right, and one asserting it nowhere would go quiet the day a
    // fifth division appears under capital outlay.
    const { app: noCap } = await opened(null, (c) => {
      c.steps = c.steps.map((st) => (st.key === "object-category"
        ? Object.assign({}, st, { caps: [] }) : st));
    }, col);
    /** @type {Record<string, number[]>} */
    const bare = {};
    for (const id of ids) {
      await at(noCap);
      const outcome = await openInto(noCap, id);
      if (outcome !== "drew") { bare[id] = [-1, -1]; continue; }
      const widths = noCap.layOut(noCap.projection).links.map((l) => l.width);
      bare[id] = [widths.length, widths.filter((w) => w < 1).length];
      await at(noCap);
    }
    out.push({
      name: `${col.label} object: the division cap is what keeps the two wide categories drawable, and is inert on the two narrow ones`,
      ok: ids.every((id) => bare[id].join(",") === pins.uncapped[id].join(",")) &&
        ids.every((id) => (pins.tails[id]
          ? bare[id][0] > pins.views[id][1] && bare[id][1] > 0
          : bare[id][0] === pins.views[id][1])) &&
        ids.every((id) => pins.views[id][2] === 0),
      detail: ids.map((id) => `${id}: capped ${pins.views[id][1]} ribbon(s), ` +
        `${pins.views[id][2]} under 1px; uncapped ${bare[id][0]} and ${bare[id][1]} ` +
        `(want ${pins.uncapped[id].join(", ")})`).join("; "),
    });

    // THE GAP, AND THE YEAR THAT DOES NOT HAVE ONE. check.SpendingGaps has no
    // year axis -- a step is declared once for every year the view lists -- so
    // the declaration names services-and-supplies under BOTH columns and only
    // one of them draws a mark. That asymmetry is the check: a gap node drawn
    // on a chart that ties to the cent would be a figure the site invented.
    const gapID = "expenditure/services-and-supplies";
    await at(app, gapID);
    const drawn = app.projection;
    const mark = drawn.nodes.find((n) => app.isGap(n.id));
    const flow = mark ? drawn.links.find((l) => l.target === mark.id) : null;
    const inferred = app.dom.byId.get("derived-list").children
      .map((li) => (li.children[0] ? li.children[0].textContent : li.textContent));
    const named = inferred.filter((t) => t.includes("Difference between the two schedules"));
    const rows = body.children.length;
    const counts = app.dom.byId.get("counts-line").textContent;
    const into = drawn.links.filter((l) => l.target === gapID)
      .reduce((a, b) => a + b.value_cents, 0);
    const outOf = drawn.links.filter((l) => l.source === gapID)
      .reduce((a, b) => a + b.value_cents, 0);
    out.push({
      name: pins.gapCents
        ? `${col.label} object: the 250,000 p0067 and pp.85-125 disagree by is drawn, named and listed under what we inferred`
        : `${col.label} object: services-and-supplies ties to the cent, so no gap mark is drawn on a chart that balances`,
      ok: Boolean(pins.gapCents) === Boolean(mark) &&
        (pins.gapCents
          ? Boolean(flow) && flow.value_cents === pins.gapCents && flow.derived &&
            flow.fact_ids.length === 0 && flow.source === gapID &&
            mark.tier === 4 && mark.derived && mark.parent === "" &&
            !app.drillable(mark) && app.isCarried(mark.id) &&
            mark.rationale.includes(spendingGapDeclaration()[gapID]) &&
            named.length === 1
          : named.length === 0) &&
        into === outOf && counts === pins.counts && rows === pins.views[gapID][1],
      detail: `${mark ? `one gap mark, "${mark.label}", at tier ${mark.tier} taking ` +
        `${flow ? flow.value_cents : "no"} cents (want ${pins.gapCents})` : "no gap mark"}; ` +
        `${named.length} entry(ies) under what we inferred name it, of ${inferred.length}; ` +
        `the centre takes ${into} and sends ${outOf}; the counts line reads "${counts}" and ` +
        `the flow table holds ${rows} row(s)`,
    });
    await at(app);
  }

  // FAIL CLOSED ON A DIFFERENCE NOTHING DECLARES, which is the only thing that
  // makes "this step's opened nodes balance" a claim rather than a hope. Both
  // refusals are reached directly: neither is producible by a click while the
  // committed corpus ties, which is the point -- they exist for the day it
  // stops.
  const { app } = await opened();
  const spine = goldenGraph();
  const step = PAGE.steps[PAGE.steps.length - 1];
  const centre = "expenditure/services-and-supplies";
  const chart = (/** @type {any[]} */ links) => ({
    projection: "department-spending",
    nodes: [{ id: centre, label: "Services & Supplies", tier: 5, parent: "" },
      { id: "dept/patrol", label: "Patrol", tier: 4, parent: "" },
      { id: "fund-group/general", label: "General Fund", tier: 2, parent: "" }],
    links: links,
  });
  const link = (/** @type {string} */ a, /** @type {string} */ b, /** @type {number} */ v) =>
    ({ source: a, target: b, value_cents: v, kind: "external", fact_ids: [], locators: [] });
  const refusedBy = (/** @type {any} */ drawnDoc, /** @type {any} */ gaps) => {
    try {
      return { threw: "", got: app.markGap(drawnDoc, { id: centre, step: Object.assign({}, step, { gaps }) }) };
    } catch (e) {
      return { threw: String((e && e.message) || e), got: null };
    }
  };
  const balanced = chart([link("fund-group/general", centre, 10000),
    link(centre, "dept/patrol", 10000)]);
  const short = chart([link("fund-group/general", centre, 10000),
    link(centre, "dept/patrol", 6000)]);
  const declared = spendingGapDeclaration();
  const undeclared = refusedBy(short, { "expenditure/wages-and-benefits": "elsewhere" });
  // THE CENTRE MISSING FROM THE CHART IS ITS OWN REFUSAL, and it is not the
  // same state as a chart that ties: `into` and `outOf` are both zero either
  // way, so a function that only compared them would call a rung that drew
  // nothing at all a rung that reconciles.
  const withoutCentre = { projection: "department-spending",
    nodes: balanced.nodes.filter((n) => n.id !== centre),
    links: [link("fund-group/general", "dept/patrol", 10000)] };
  const absent = refusedBy(withoutCentre, declared);
  const withReason = refusedBy(short, declared);
  const ties = refusedBy(balanced, declared);
  const none = refusedBy(short, undefined);
  out.push({
    name: "a shortfall the step declares no reason for is refused by name; a declared one is drawn, and a step declaring no gap at all is left alone",
    ok: /no declaration on this step accounts for/.test(undeclared.threw) &&
      /\$40/.test(undeclared.threw) &&
      /nothing to be stated against/.test(absent.threw) &&
      withReason.threw === "" && withReason.got.nodes.length === 4 &&
      withReason.got.links.length === 3 &&
      withReason.got.links[2].value_cents === 4000 &&
      ties.threw === "" && ties.got.nodes.length === 3 && ties.got.links.length === 2 &&
      none.threw === "" && none.got.links.length === 2 &&
      spine.nodes.some((n) => n.id === centre),
    detail: `an undeclared $40 shortfall throws ${JSON.stringify(undeclared.threw)}; a declared ` +
      `one draws ${withReason.threw ? `A THROW (${withReason.threw})` :
        `${withReason.got.nodes.length} node(s) and ${withReason.got.links.length} link(s)`}; ` +
      `a centre the chart does not draw throws ${JSON.stringify(absent.threw)}; a chart that ` +
      `ties draws ${ties.threw ? "A THROW" : `${ties.got.links.length} link(s)`}; a step with ` +
      `no gaps at all draws ${none.threw ? "A THROW" : `${none.got.links.length} link(s)`}`,
  });
  return out;
}

/**
 * A chart whose columns are declared in an order a sort by tier disagrees
 * with, and whose ribbons are partitions rather than flows.
 *
 * BOTH SHAPES ARE THE WINDOW'S AND NEITHER IS ON THE CORPUS YET. {2,5,4} is
 * the object-category window's column order -- fund groups, the category they
 * pay for, the divisions that spend it -- and its ribbons are Budget Book
 * pp.85-125's one matrix read along a second axis. Written here because the
 * shaping and the wording ship before the projection that produces them, and
 * a class nothing renders is a class nothing can see go wrong.
 *
 * @param {any[]} steps
 */
function crossTabProbe(steps) {
  const node = (id, tier, parent, label) =>
    ({ id, label, tier, parent, constraint_tier: "", role: "", derived: false,
      rationale: "", source_note: "" });
  const link = (a, b, v) =>
    ({ source: a, target: b, value_cents: v, kind: "external", transfer_id: "",
      fact_ids: ["f1"], locators: [], derived: false, partition: true });
  const doc = {
    schema_version: 1, projection: "probe",
    metadata: { sources: [], caveats: [], counts: { facts: 1, nodes: 3, links: 2 } },
    nodes: [
      node("fund-group/general", 2, "", "General Fund"),
      node("expenditure/wages", 5, "", "Wages & Benefits"),
      node("dept/patrol", 4, "", "Patrol"),
    ],
    links: [link("fund-group/general", "expenditure/wages", 100),
      link("expenditure/wages", "dept/patrol", 100)],
  };
  return loadApp({
    fetch: plannedFetch({ "data/probe.json": { doc } }),
    config: {
      schema_version: 1, primary: "probe", projections: { probe: "data/probe.json" },
      render_tiers: [2, 5, 4],
      steps,
      years: [{
        year: 2026, label: "FY", stem: "probe", path: "data/probe.json", basis: "adopted",
        hero: { label: "l", value: "v", note: "n", kind: "hero" }, figures: [], caveats: [],
        counts: { facts: 1, nodes: 3, links: 2 }, chart_title: "t",
      }],
      docs: {},
    },
  });
}

/** Opens the probe through main() and hands back the app and its table body. */
async function drawnProbe(steps) {
  const app = crossTabProbe(steps);
  const body = app.dom.document.node();
  app.dom.document.getElementById("flow-table").selectable = { tbody: body };
  app.dom.document.plant("main", app.dom.document.node());
  await settle();
  return { app, body };
}

/**
 * The columns a reader is told to click, and the words a partition ribbon
 * carries.
 */
async function columnAndPartitionChecks() {
  const out = [];
  const step = (key, from, tiers) =>
    ({ key, after: [""], from, tiers, back: "Back", tail: "things" });

  // ONE OPENABLE COLUMN, AND IT IS THE ONE THE TWO ORDERS DISAGREE ABOUT.
  // Declared {2,5,4}, tier 4 is the LAST column; sorted ascending it is the
  // MIDDLE one. With every column openable the two orders produce the same
  // three words and the check would pass under either, which is why exactly
  // one opens here.
  const { app: one } = await drawnProbe([step("d", 4, [4, 5])]);
  const declared = one.openableColumns();
  out.push({
    name: "the openable column is named by the order the page declares, not by tier number",
    ok: JSON.stringify(declared) === JSON.stringify(["right-hand"]),
    detail: `tiers [2,5,4] declared, tier 4 opens, and the hint calls it ` +
      `${JSON.stringify(declared)} -- sorted ascending the same tier is the middle column`,
  });

  // THREE COLUMNS READ AS A LIST. `join(" or ")` gives "the left-hand or
  // middle or right-hand column", which is a sentence nobody writes; the spine
  // reaches three the day its right-hand column opens.
  const { app: three } = await drawnProbe(
    [step("a", 2, [2, 5]), step("b", 5, [5, 4]), step("c", 4, [4, 5])]);
  const hint = three.maybeEl("chart-hint").textContent;
  out.push({
    name: "three openable columns are joined as a list, and joinOr is that rule at every length",
    ok: hint.includes("Double click a node in the left-hand, middle or right-hand column") &&
      three.joinOr([]) === "" && three.joinOr(["a"]) === "a" &&
      three.joinOr(["a", "b"]) === "a or b" &&
      three.joinOr(["a", "b", "c"]) === "a, b or c",
    detail: `hint "${hint}"; joinOr over one, two and three reads "${three.joinOr(["a"])}", ` +
      `"${three.joinOr(["a", "b"])}", "${three.joinOr(["a", "b", "c"])}"`,
  });

  // THE CROSS-TAB SENTENCE, IN ALL FOUR PLACES ONE MARK CAN CARRY IT, and the
  // same sentence in each: the class the stylesheet paints, the label a screen
  // reader hears for the ribbon and for a node whose every ribbon is one, and
  // the flow table's own column, which is where a reader who cannot use the
  // chart at all reads it.
  const { app, body } = await drawnProbe([step("d", 4, [4, 5])]);
  const laid = app.layOut(app.projection);
  const ribbon = laid.links[0];
  const category = laid.nodes.find((n) => n.id === "expenditure/wages");
  const cell = body.children.length ? body.children[0].children[4].textContent : "";
  out.push({
    name: "a partition ribbon says it is a cross-tab in its class, its two labels and the flow table",
    ok: app.linkClass(ribbon).split(" ").includes("partition") &&
      app.linkDescription(ribbon).includes(app.PARTITION_NOTE) &&
      app.isPartitionNode(category) &&
      app.nodeDescription(category).includes(app.PARTITION_NOTE) &&
      cell === app.PARTITION_NOTE &&
      !app.linkClass(ribbon).split(" ").includes("derived"),
    detail: `class "${app.linkClass(ribbon)}"; ribbon label ends ` +
      `"${app.linkDescription(ribbon)}"; the category's label carries the sentence ` +
      `${app.nodeDescription(category).includes(app.PARTITION_NOTE)}; the flow table's ` +
      `provenance column reads "${cell}" where a plain published ribbon reads "printed"`,
  });

  return out;
}

/**
 * A window whose kept flank is a node the step's document has never heard of.
 *
 * THE CASE THAT DECIDES WHERE drillDown LOOKS THE NODE UP. A departmentwide
 * document has no fund axis, so an object category's window keeps fund groups
 * drawn from the spine in a chart whose own file carries none -- and the step
 * that opens one of them draws a THIRD document. Asked of the rung's file, that
 * fund group is a node the document does not carry and the click returns FAILED
 * with nothing said; asked of the chart on screen, it is the mark the reader
 * activated and it opens. Both documents here are three nodes wide because the
 * shape is the subject, not the figures.
 */
async function foreignFlankProbe() {
  const node = (id, tier, parent, label) =>
    ({ id, label, tier, parent, constraint_tier: "", role: "", derived: false,
      rationale: "", source_note: "" });
  const link = (a, b, v) =>
    ({ source: a, target: b, value_cents: v, kind: "external", transfer_id: "",
      fact_ids: ["f"], locators: [], derived: false });
  const doc = (stem, nodes, links) =>
    ({ schema_version: 1, projection: stem,
      metadata: { sources: [], caveats: [],
        counts: { facts: 1, nodes: nodes.length, links: links.length } },
      nodes, links });
  // The spine: a category paying a group.
  const spine = doc("spine", [node("cat", 0, "", "Category"), node("grp", 2, "", "Group")],
    [link("cat", "grp", 100)]);
  // What the window opens INTO, which carries the category and no group at all.
  const lines = doc("lines", [node("line", 1, "cat", "A line"), node("cat", 0, "", "Category")],
    [link("line", "cat", 100)]);
  // What the kept group opens into: a third document, reached from a chart
  // whose own file does not carry the node that was clicked.
  const funds = doc("funds", [node("grp", 2, "", "Group"), node("fund", 3, "grp", "A fund")],
    [link("grp", "fund", 100)]);
  const app = loadApp({
    fetch: plannedFetch({
      "data/spine.json": { doc: spine },
      "data/lines.json": { doc: lines },
      "data/funds.json": { doc: funds },
    }),
    config: {
      schema_version: 1, primary: "spine",
      projections: { spine: "data/spine.json", lines: "data/lines.json", funds: "data/funds.json" },
      render_tiers: [0, 2],
      steps: [
        { key: "win", after: [""], from: 0, projection: "lines", keep: [2], tiers: [1, 0, 2],
          back: "Back", tail: "lines" },
        { key: "grp", after: ["win"], from: 2, projection: "funds", tiers: [2, 3],
          back: "Back", tail: "funds" },
      ],
      years: [{
        year: 2026, label: "FY", stem: "spine", path: "data/spine.json", basis: "adopted",
        hero: { label: "l", value: "v", note: "n", kind: "hero" }, figures: [], caveats: [],
        counts: { facts: 1, nodes: 2, links: 1 }, chart_title: "t",
        steps: [{ stem: "lines", path: "data/lines.json", caveats: [] },
          { stem: "funds", path: "data/funds.json", caveats: [] }],
      }],
      docs: {},
    },
  });
  app.dom.document.getElementById("flow-table").selectable = { tbody: app.dom.document.node() };
  app.dom.document.plant("main", app.dom.document.node());
  await settle();

  const opened = await openInto(app, "cat");
  const drawn = app.projection.nodes.map((n) => n.id).sort();
  const inRungFile = app.docAt(1).nodes.some((n) => n.id === "grp");
  const group = app.projection.nodes.find((n) => n.id === "grp");
  // ASKED WHILE THE WINDOW IS STILL ON SCREEN. drillable is a question about
  // the chart the mark is drawn on, and the answer changes the moment the mark
  // has been opened.
  const offers = Boolean(group) && app.drillable(group);
  const deeper = await openInto(app, "grp");
  const deepIDs = app.projection.nodes.map((n) => n.id).sort();
  return [{
    name: "a kept flank the step's own document does not carry is still the mark the reader clicked, and still opens",
    ok: opened === "drew" && JSON.stringify(drawn) === '["cat","grp","line"]' &&
      !inRungFile && offers &&
      deeper === "drew" && JSON.stringify(deepIDs) === '["fund","grp"]' &&
      app.drilled.length === 2,
    detail: opened === "drew"
      ? `the window drew ${JSON.stringify(drawn)} from a step document that ` +
        `${inRungFile ? "DOES carry" : "does not carry"} the kept group, which the window ` +
        `${offers ? "offers to open" : "does NOT offer to open"}; clicking it came to ` +
        `"${deeper}" drawing ${JSON.stringify(deepIDs)} of a third document`
      : `the window would not open: ${opened}`,
  }];
}

/** A cents figure as fmt() prints it, for a pin typed as dollars and cents. */
function fmtDollars(cents) {
  return new Intl.NumberFormat("en-US", { style: "currency", currency: "USD", maximumFractionDigits: 0 })
    .format(cents / 100);
}

/**
 * The shapes the committed columns cannot reach: a category of one line, a
 * category with none, and fisc-ng17's broken parent chains.
 */
async function categoryProbes() {
  const out = [];
  const col = COLUMNS[0];
  const node = (id, tier, parent, role) =>
    ({ id, label: id, tier, parent, constraint_tier: "", role: role || "", derived: false,
      rationale: "", source_note: "" });
  const link = (a, b, v) =>
    ({ source: a, target: b, value_cents: v, kind: "external", transfer_id: "",
      fact_ids: [], locators: [], derived: false });

  // A CATEGORY OF ONE LINE STILL OPENS -- it shows which groups carry it --
  // and TWO of that window's three columns hold one mark: the line and the
  // centre get no share chip while the two fund groups beside them do. Built
  // rather than found: no adopted column prints a category with a single
  // distinct label.
  {
    const doc = Object.assign({}, goldenFundFlows());
    doc.nodes = doc.nodes.concat([
      node("revenue/probe", 0, "", "revenue_source"),
      node("revenue-line/probe/only", 1, "revenue/probe", "revenue_line"),
      node("revenue/empty", 0, "", "revenue_source"),
    ]);
    doc.links = doc.links.concat([
      link("revenue-line/probe/only", "fund/100", 700), link("revenue-line/probe/only", "fund/200", 300),
      // THE ROLLUP IS WHAT THE WINDOW DRAWS. A category is the target of its
      // lines' (1,0) links and the source of nothing this document prints, so
      // a probe category without one opens into an empty left half.
      link("revenue-line/probe/only", "revenue/probe", 1000),
    ]);
    const spine = goldenGraph();
    spine.nodes = spine.nodes.concat([
      node("revenue/probe", 0, "", "revenue_source"), node("revenue/empty", 0, "", "revenue_source")]);
    spine.links = spine.links.concat([
      link("revenue/probe", "fund-group/general", 700), link("revenue/probe", "fund-group/special-revenue", 300),
      link("revenue/empty", "fund-group/general", 1)]);
    const { app, main } = await opened({
      "data/sankey.json": { doc: spine }, "data/fund-flows.json": { doc } }, null, col);
    const one = await openInto(app, "revenue/probe");
    const laid = one === "drew" ? app.layOut(app.projection) : null;
    const only = laid ? laid.nodes.find((n) => n.id === "revenue-line/probe/only") : null;
    const groups = laid ? laid.nodes.filter((n) => n.tier === 2) : [];
    const probeCentre = laid ? laid.nodes.find((n) => n.id === "revenue/probe") : null;
    // READ WHILE THIS VIEW IS THE ONE LAID OUT. columnShare totals the column
    // of the last layOut, and after drillUp below that is the spine's --
    // measured: asked afterwards, the line read "<0.1% of this column",
    // its 1000 cents against the spine's revenue column.
    const onlyShare = only ? app.columnShare(only) : "";
    const centreShare = probeCentre ? app.columnShare(probeCentre) : "";
    const oneShares = groups.map((f) => app.columnShare(f));
    app.drillUp(0);
    const none = await openInto(app, "revenue/empty");
    const banners = refusals(main).map((b) => b.textContent);
    let direct = "";
    try {
      app.filterToNode(doc, "revenue/empty", [1, 0]);
    } catch (e) {
      direct = String((e && e.message) || e);
    }
    out.push({
      name: "a category of one line opens with no share on the line, and one with none is refused by name rather than handed to d3-sankey empty",
      ok: one === "drew" && Boolean(only) && onlyShare === "" &&
          Boolean(probeCentre) && centreShare === "" && groups.length === 2 &&
          oneShares.every((sh) => sh.startsWith("\u25c7 our ")) &&
          none === "failed" && app.drilled.length === 0 && banners.length === 1 &&
          banners[0].includes("nothing flows between tiers 1, 0 for node revenue/empty") &&
          direct.includes("nothing flows between tiers 1, 0 for node revenue/empty"),
      detail: `one line: ${one}, the line's share reads ${JSON.stringify(onlyShare)}, the centre's ` +
        `${JSON.stringify(centreShare)} and ` +
        `its ${groups.length} fund groups' ${JSON.stringify(oneShares)}; no lines: ${none} with ${banners.length} ` +
        `banner(s)${banners.length ? ` reading "${banners[0]}"` : ""}; filterFromNode directly: ` +
        `${JSON.stringify(direct)}`,
    });
  }

  // fisc-ng17. A LINE WITH A BROKEN PARENT CHAIN STOPS THE DRILL, in both
  // shapes: a rung filters before the fold can refuse, and the filter's
  // placeability test dropped the link as if the view had declared its tier
  // away. Measured before the fix, over this golden: ERAF's parent blanked drew
  // the General Fund's Property Taxes at $79,318,762 against p127's
  // $64,143,762, with node and link counts unchanged and no banner.
  //
  // DRIVEN THROUGH A DECLARATION THE SITE NO LONGER SHIPS, and that is stated
  // rather than hidden. The guard fires where a view FOLDS tier 1 into tier 0,
  // and no shipped tier set does any more: {0,2,3} takes its categories off the
  // spine, which has no lines at all, and {1,0,2} draws each line in a column of
  // its own. Measured over both columns after the tier sets moved -- ERAF's
  // parent blanked or pointed at a node the document does not carry changes not
  // one node, link or figure of any view the page opens. So the branch is
  // reached through the fixture below, which is the {0,3,4} step this page
  // shipped until Lane G, and the check goes on holding app.js's own refusal
  // rather than being deleted with the declaration that used to reach it.
  {
    const folding = [Object.assign({}, PAGE.steps[0], { tiers: [0, 3, 4],
      caps: [{ tier: 3, cap: 8 }, { tier: 4, cap: 24, tail: "divisions" }] })];
    delete folding[0].keep;
    const withFolding = (/** @type {any} */ plan) =>
      opened(plan, (c) => { c.steps = folding; }, col);
    const control = await withFolding(null);
    await mustOpen(control.app, "fund-group/general");
    const asPublished = control.app.projection.links.find((l) =>
      l.source === "revenue/taxes/property" && l.target === "fund/100");
    const shapes = [
      { name: "blanked", parent: "", says: "revenue-line/taxes/property/eraf is tier 1 and reaches no tier this page draws (0, 3, 4), while other tier-1 nodes do; its parent chain is broken" },
      { name: "pointing at a node the document does not carry", parent: "revenue/no-such-category",
        says: "revenue-line/taxes/property/eraf names parent revenue/no-such-category, which the document does not carry" },
    ];
    const results = [];
    for (const shape of shapes) {
      const doc = goldenFundFlows();
      doc.nodes.find((n) => n.id === "revenue-line/taxes/property/eraf").parent = shape.parent;
      const { app, main, body } = await withFolding({ "data/fund-flows.json": { doc } });
      const before = shown(app, body);
      const outcome = await openInto(app, "fund-group/general");
      const after = shown(app, body);
      const banners = refusals(main).map((b) => b.textContent);
      results.push({ shape: shape.name, outcome, depth: app.drilled.length, banners,
        named: banners.length === 1 && banners[0].includes(shape.says),
        unchanged: after.counts === before.counts && after.crumbHidden });
    }
    out.push({
      name: "a tier-1 line whose parent chain is broken stops a drill that folds it by name, instead of folding the group's Property Taxes without it",
      ok: Boolean(asPublished) && asPublished.value_cents === 6414376200 &&
          results.every((r) => r.outcome === "failed" && r.depth === 0 && r.named && r.unchanged),
      detail: `as published the opened General Fund draws Property Taxes at ${asPublished ? asPublished.value_cents : "nothing"} ` +
        `(p127: 6414376200); ` + results.map((r) => `ERAF's parent ${r.shape}: ${r.outcome} at depth ${r.depth}, ` +
          `${r.banners.length} banner(s) ${r.named ? "naming the node and the fault" : "NOT naming it"}` +
          (r.banners.length && !r.named ? ` ("${r.banners[0].slice(0, 120)}")` : "")).join("; "),
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
  const [groupStep, fundStep, divisionStep] = PAGE.steps;

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
  //
  // THE FUND IS WHAT OPENS HERE NOW AND THE DIVISION IS NOT DRAWN AT ALL:
  // {0,2,3} stops at the funds, and the divisions are a rung further in. The
  // fund column's OTHER marks are the counter-case -- the capped tail and the
  // residual, neither of which is a fund the city printed -- and they must
  // stay shut.
  const fund100 = app.projection.nodes.find((n) => n.id === "fund/100");
  const fund100Opens = Boolean(fund100) && app.drillable(fund100);
  const divisions1 = app.projection.nodes.filter((n) => n.id.startsWith("dept/")).length;
  const opensAt1 = app.projection.nodes.filter((n) => app.drillable(n)).map((n) => n.id);
  // A CARRIED ENDPOINT BELONGS TO NO GROUP, exactly as it does on the spine
  // it was copied from: a transfer out is money leaving, not money held.
  const muted1 = app.projection.nodes
    .filter((n) => !n.id.startsWith("revenue/") && !app.isCarried(n.id) && app.fundGroupOf(n) === "")
    .map((n) => n.id);
  // THE DOCUMENT'S OWN FACTS, which is what the counts line claims a share of.
  // The residual count stays a number of its own: the residual is some of the
  // ribbons this window carries off the spine and the kept flank is the rest,
  // and the residual block is what pins which are which.
  const resid1 = app.projection.links.filter((l) =>
    app.isResidual(l.source) || app.isResidual(l.target)).length;
  // THE CENTRE BALANCES, ASSERTED WHERE THE READER MEETS IT. The residual is
  // what makes it true and the residual block above is what proves the
  // residual; this is the one figure a reader could check by eye, so the walk
  // reads it too.
  const centre1 = {
    in: app.projection.links.filter((l) => l.target === "fund-group/general")
      .reduce((sum, l) => sum + l.value_cents, 0),
    out: app.projection.links.filter((l) => l.source === "fund-group/general")
      .reduce((sum, l) => sum + l.value_cents, 0),
  };

  withFocus();
  const open2 = await openInto(app, "fund/100");
  const at2 = words(app);
  const rows2 = body.children.length;
  const focus2 = app.dom.focused ? app.dom.focused.textContent : "";
  const asked2 = fetch.asked.slice();
  const m2 = open2 === "drew" ? measure(app, app.projection) : null;
  const patrol = app.projection.nodes.find((n) => n.id === "dept/patrol");
  const patrolOpens = Boolean(patrol) && app.drillable(patrol);
  // THE DIVISION CAP IS INERT ON THE CORPUS AND PINNED INERT: 23 divisions
  // under a cap of 24, so no aggregate at tier 4 -- the day a 24th division
  // appears the column starts folding, and this is what says so.
  const divisions2 = app.projection.nodes.filter((n) => n.id.startsWith("dept/")).length;
  const foldedDivisions2 = app.projection.nodes.some((n) => n.id === app.aggregateID(4));
  const cited2 = new Set();
  for (const l of app.projection.links) for (const id of l.fact_ids) cited2.add(id);
  // THE ONE CENTRE ON THE PAGE WHOSE TWO SIDES ARE DIFFERENT QUANTITIES, and
  // the step's description is what says so. A fund's revenue and its
  // divisions' spending are two schedules; what is left is what the city
  // transfers out and adds to reserves, which pp.66-67 print for the GROUP.
  // Pinned per column so that difference cannot move unremarked, and the
  // sentence that explains it is asserted beside the figures.
  const centre2 = {
    in: app.projection.links.filter((l) => l.target === "fund/100")
      .reduce((sum, l) => sum + l.value_cents, 0),
    out: app.projection.links.filter((l) => l.source === "fund/100")
      .reduce((sum, l) => sum + l.value_cents, 0),
  };

  withFocus();
  const open3 = await openInto(app, "dept/patrol");
  const at3 = words(app);
  const focus3 = app.dom.focused ? app.dom.focused.textContent : "";
  const asked3 = fetch.asked.slice();
  const m3 = open3 === "drew" ? measure(app, app.projection) : null;
  const anyOpens3 = app.projection.nodes.some((n) => app.drillable(n));

  // ESCAPE, ONE RUNG AT A TIME, through the handler main() attached. The
  // breadcrumb's controls call drillUp(k) directly; Escape is the other route
  // and the one whose "innermost first" rule the bead names.
  const escape = () => {
    for (const fn of app.dom.documentListeners.keydown || []) fn({ key: "Escape" });
  };
  withFocus();
  escape();
  const back2 = words(app);
  const focusBack2 = app.dom.focused ? app.dom.focused.textContent : "";
  escape();
  const back1 = words(app);
  escape();
  const back0 = words(app);
  const asked4 = fetch.asked.slice();

  // AND THE CACHE: opening again fetches nothing. Then the breadcrumb's own
  // controls, each closing TO ITS OWN DEPTH: from depth 3 the innermost
  // control lands on depth 2 and the outermost on the overview -- a bar whose
  // every control went to the overview would pass the Escape arm above
  // unnoticed.
  await at(app, "fund-group/general", "fund/100", "dept/patrol");
  const asked5 = fetch.asked.slice();
  const click = (control) => { for (const fn of (control && control.listeners.click) || []) fn({}); };
  const controlsAt = () =>
    app.dom.byId.get("breadcrumb").children.filter((c) => c.tagName === "button");
  click(controlsAt()[2]);
  const inner = words(app);
  await openInto(app, "dept/patrol");
  click(controlsAt()[0]);
  const back0b = words(app);

  // A GROUP WHOSE FUNDS NOTHING DECOMPOSES SAYS SO. Five of the six groups
  // draw only funds the fund step's role leaves shut, so the depth-1 hint over
  // them must say nothing opens further rather than point at a column whose
  // marks are ends of the chain.
  await openInto(app, "fund-group/capital");
  const capital = words(app);
  const capitalOpens = app.projection.nodes.filter((n) => app.drillable(n)).map((n) => n.id);
  const capitalFunds = app.projection.nodes.filter((n) => n.id.startsWith("fund/")).length;
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
    name: `${col.label} chain: the overview's hint names all three columns that open`,
    // ALL THREE, READ OFF THE CHART. The spine draws tiers 0, 2 and 5 in that
    // declared order, and something opens in each: the revenue categories, the
    // fund groups, and -- since the object-category step -- four of the seven
    // nodes in the right-hand column. A hint naming the middle column alone
    // would leave two gestures nothing on the page invites, and this is the
    // sentence a reader who cannot see the marks is given.
    ok: at0.hint === "Double click a node in the left-hand, middle or right-hand column to open " +
        "it into its parts, or tab to one and press Enter. A single click, or Space, follows one node's money. A fund swatch follows " +
        "one group's money without opening anything." &&
        at0.legend === 6 && at0.desc === served,
    detail: `hint "${at0.hint}"; legend ${at0.legend} swatches`,
  });
  out.push({
    name: `${col.label} chain: depth 1 draws the General Fund at {0,2,3} as a window whose centre balances, and every sentence says so`,
    // THE SPINE'S OWN REVENUE CATEGORIES ON THE LEFT, the group the reader
    // clicked in the middle and its funds on the right, with the money no
    // fund receives carried past the centre onto the residual. What makes the
    // shape checkable rather than merely drawn is the equality: the centre's
    // two sides agree to the cent, in both columns, which is what the residual
    // is for.
    ok: open1 === "drew" && at1.depth === 1 && !at1.drawnIsYears &&
        Boolean(m1) && m1.nodes === col.general.nodes && m1.links === col.general.links &&
        m1.hairlines === col.general.hairlines &&
        divisions1 === 0 && resid1 === col.residual["fund-group/general"].carried &&
        centre1.in === centre1.out && centre1.in > 0 &&
        at1.counts === col.general.counts &&
        rows1 === col.general.links &&
        at1.title === `Sankey diagram of the ${col.label} adopted budget, opened into General Fund group` &&
        at1.crumbControls.join("|") === "← All fund groups" && at1.crumbHere === "General Fund group" &&
        at1.hint === "This is General Fund group, broken into its parts. Double click a node " +
          "in the right-hand column to open it further, or tab to one and press Enter. A single click, or Space, follows one node's money." &&
        at1.legend === 0 &&
        at1.desc === "Opened into General Fund group. " + groupStep.description +
          " Use the breadcrumb above the chart, or press Escape, to go back. " + pointer &&
        opensAt1.join() === "fund/100" && fund100Opens && muted1.length === 0 &&
        focus1 === "← All fund groups",
    detail: open1 === "drew"
      ? `${m1.nodes} nodes, ${m1.links} links, ${m1.hairlines} under 1px; counts "${at1.counts}"; ` +
        `title "${at1.title}"; breadcrumb ${JSON.stringify(at1.crumbControls)} + "${at1.crumbHere}"; ` +
        `hint "${at1.hint}"; legend ${at0.legend} -> ${at1.legend} swatches; desc ` +
        `${at1.desc.startsWith("Opened into General Fund group. " + groupStep.description) ? "carries" : "LACKS"} ` +
        `the step's description and ${at1.desc.endsWith(pointer) ? "keeps" : "DROPS"} the table pointer; ` +
        `the centre takes ${centre1.in} and sends ${centre1.out}; ${resid1} residual ribbon(s); ` +
        `${divisions1} division(s) drawn; ` +
        `${JSON.stringify(opensAt1)} open; ${muted1.length} fund-side mark(s) ` +
        `resolve to no group${muted1.length ? " (" + muted1.slice(0, 3).join(", ") + ")" : ""}; ` +
        `focus on "${focus1}"`
      : `opening the General Fund came to "${open1}"`,
  });
  out.push({
    name: `${col.label} chain: depth 2 draws fund/100 at {2,3,4}, whose two sides are different quantities and whose description says which`,
    ok: open2 === "drew" && at2.depth === 2 && Boolean(m2) &&
        m2.nodes === col.fund.nodes && m2.links === col.fund.links &&
        m2.hairlines === col.fund.hairlines &&
        divisions2 === 23 && !foldedDivisions2 && patrolOpens &&
        asked2.length === asked1.length &&
        centre2.in === col.fundCentre[0] && centre2.out === col.fundCentre[1] &&
        at2.counts === `${col.fund.links} flows between ${col.fund.nodes} nodes, from ` +
          `${cited2.size} of the document's 280 facts` &&
        rows2 === col.fund.links &&
        at2.crumbControls.join("|") === "← All fund groups|← All funds" &&
        at2.desc === "Opened into General Fund group, then General Fund. " + fundStep.description +
          " Use the breadcrumb above the chart, or press Escape, to go back. " + pointer &&
        fundStep.description.includes("what it takes in is its revenue") &&
        fundStep.description.includes("transfers out of the fund and adds to its reserves") &&
        focus2 === "← All funds",
    detail: open2 === "drew"
      ? `${m2.nodes} nodes, ${m2.links} links, ${m2.hairlines} under 1px; counts "${at2.counts}"; ` +
        `breadcrumb ${JSON.stringify(at2.crumbControls)} + "${at2.crumbHere}"; ` +
        `the fund takes in ${centre2.in} and pays its divisions ${centre2.out} ` +
        `(want ${col.fundCentre.join(" / ")}), a difference of ${centre2.in - centre2.out} that the ` +
        `step's sentence ${fundStep.description.includes("transfers out of the fund") ? "names" : "does NOT name"}; ` +
        `${divisions2} divisions drawn${foldedDivisions2 ? " AND a tier-4 aggregate" : ", none folded"}; ` +
        `a division ${patrolOpens ? "opens" : "does NOT open"}; no second fetch; focus on "${focus2}"`
      : `opening fund/100 came to "${open2}"`,
  });
  out.push({
    name: `${col.label} chain: depth 3 draws Patrol at {3,4,5}, names all three rungs, keeps the table pointer, and opens nothing further`,
    ok: open3 === "drew" && at3.depth === 3 && Boolean(m3) && m3.links > 0 &&
        asked3.length === asked1.length &&
        at3.title.endsWith(", opened into General Fund group, then General Fund, then Patrol") &&
        at3.crumbControls.join("|") === "← All fund groups|← All funds|← All divisions" &&
        at3.crumbHere === "Patrol" &&
        at3.hint === "This is Patrol, broken into its parts. Nothing here opens further; go " +
          "back to open another. A single click, or Space, follows one node's money." &&
        at3.desc === "Opened into General Fund group, then General Fund, then Patrol. " +
          divisionStep.description +
          " Use the breadcrumb above the chart, or press Escape, to go back. " + pointer &&
        !anyOpens3 && at3.legend === 0 &&
        focus3 === "← All divisions",
    detail: open3 === "drew"
      ? `${m3.nodes} nodes, ${m3.links} links, smallest ribbon ${m3.smallest.toFixed(2)}px; title ` +
        `"${at3.title}"; breadcrumb ${JSON.stringify(at3.crumbControls)} + "${at3.crumbHere}"; ` +
        `hint "${at3.hint}"; desc "${at3.desc.slice(0, 60)}..."; ` +
        `${anyOpens3 ? "SOMETHING still opens" : "nothing opens"}; no second fetch; focus on "${focus3}"`
      : `opening Patrol came to "${open3}"`,
  });
  out.push({
    name: `${col.label} chain: Escape closes one rung at a time, and each depth comes back as it was`,
    ok: back2.depth === 2 && back2.counts === at2.counts && back2.title === at2.title &&
        focusBack2 === "← All funds" &&
        back1.depth === 1 && back1.counts === at1.counts && back1.title === at1.title &&
        back1.crumbControls.join("|") === at1.crumbControls.join("|") &&
        back1.hint === at1.hint && back1.desc === at1.desc &&
        back0.depth === 0 && back0.counts === at0.counts && back0.title === at0.title &&
        back0.crumbHidden && back0.legend === 6 && back0.hint === at0.hint &&
        back0.desc === served && back0.drawnIsYears &&
        asked4.length === asked1.length,
    detail: `after one Escape: depth ${back2.depth}, counts "${back2.counts}", focus on "${focusBack2}"; ` +
      `after two: depth ${back1.depth}, counts "${back1.counts}"; ` +
      `after three: depth ${back0.depth}, legend ${back0.legend}, ` +
      `breadcrumb ${back0.crumbHidden ? "hidden" : "SHOWING"}`,
  });
  out.push({
    name: `${col.label} chain: reopening fetches nothing, and each breadcrumb control closes to its own depth`,
    ok: asked5.length === asked1.length &&
        inner.depth === 2 && inner.counts === at2.counts &&
        back0b.depth === 0 && back0b.counts === at0.counts && back0b.crumbHidden,
    detail: `${asked5.length} fetch(es) after 0->1->2->3->2->1->0->1->2->3, want ${asked1.length}; the ` +
      `innermost control left depth ${inner.depth} reading "${inner.counts}", and the outermost left ` +
      `depth ${back0b.depth} reading "${back0b.counts}"`,
  });
  out.push({
    name: `${col.label} chain: a group whose funds no schedule decomposes says nothing opens further, rather than naming a column that is not there`,
    // THE FUNDS ARE DRAWN AND NONE OF THEM OPENS, which is a stronger claim
    // than "the column is empty": pp.167-170 are the General Fund's schedule,
    // so capital's ten marks are ends of the chain rather than rungs, and the
    // step's role is what says so. Before that role every one of them offered
    // a click that banners.
    ok: capital.depth === 1 && capitalOpens.length === 0 && capitalFunds > 0 &&
        capital.hint === "This is Capital Funds, broken into its parts. Nothing here opens " +
          "further; go back to open another. A single click, or Space, follows one node's money." &&
        capital.desc.startsWith("Opened into Capital Funds. " + groupStep.description),
    detail: `opened into capital: ${capitalFunds} fund mark(s) drawn and ` +
      `${capitalOpens.length ? capitalOpens.join(", ") + " WRONGLY open" : "none opens"}; ` +
      `hint "${capital.hint}"`,
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


/**
 * One window opened at a stated column budget, measured off the chart the page
 * drew rather than off the declaration it drew it from.
 *
 * THE BUDGET IS SET BEFORE THE DRILL, because activeTiers is read while the
 * rung is SHAPED -- the filter, the caps and the fold all take the tier set --
 * so a budget raised afterwards would leave a chart on screen shaped at the old
 * one and measured as though it were not.
 *
 * A FRESH PAGE PER MEASUREMENT. The budget is page state, and a check that
 * raised it and left it raised would hand every arm after it a chart no reader
 * is shown.
 *
 * @param {any} col the published column to open over
 * @param {number} budget
 * @param {string[]} path the nodes to open, outermost first
 * @param {(c: any) => void} [tweak] the config edit, if this is a shape the
 *   site does not ship
 */
async function windowAt(col, budget, path, tweak) {
  const { app, main } = await opened(null, tweak, col);
  app.setColumnBudget(budget);
  for (const id of path) await mustOpen(app, id);
  const laid = app.layOut(app.projection);
  const widths = laid.links.map((l) => l.width);
  const tail = app.projection.nodes.find((n) => app.isAggregate(n.id) && n.tier === 5);
  return {
    tiers: app.activeTiers().join(","),
    columns: app.drawnColumns(),
    // THE RIGHT-HAND EDGE OF THE DRAWING, WHICH IS WHAT SAYS THE WIDTH WAS
    // BELIEVED. d3-sankey takes its column COUNT from topology and spreads it
    // over whatever extent it is given, so a chart laid out at a width it
    // cannot fill draws the columns it has further apart -- and the last
    // column's rect is the one place that shows.
    right: Math.max(...laid.nodes.map((n) => n.x1)),
    banners: refusals(main).length,
    nodes: app.projection.nodes.length,
    links: app.projection.links.length,
    hairlines: widths.filter((w) => w < 1).length,
    tail: tail ? tail.label : "",
    // Every adjacent pair of drawn columns, as the count of ribbons crossing
    // it: layout.mjs's bands() throws on a link that spans more than one, so a
    // band with nothing in it is a column no ribbon reaches.
    bands: app.activeTiers().slice(1).map((t, k) => app.projection.links.filter((l) => {
      const at = (/** @type {string} */ id) =>
        (app.projection.nodes.find((n) => n.id === id) || { tier: -1 }).tier;
      return at(l.source) === app.activeTiers()[k] && at(l.target) === t;
    }).length).join("/"),
  };
}

/**
 * The fund window at four columns, and a widened column the document cannot
 * fill.
 *
 * WHAT THIS IS EVIDENCE FOR. pkg/cmd/export/data.go declares the fund step at
 * tiers {2,3,4,5} widening by {5}, and a reader sees the fourth column only on
 * a window wide enough to buy it or after asking for it: the harness has no
 * viewport, so every page loaded here opens at NARROW_COLUMNS. These arms are
 * what exercise the widened declaration at the shape it was declared for;
 * lifecycle.mjs is where the control that gets a reader there is driven.
 *
 * THE NARROW WINDOW IS PINNED BESIDE IT, AND THAT PAIRING IS THE POINT. With
 * the widening taken back off the step, the wide arm goes red and the narrow
 * one stays green: that is what tells "the fourth column was drawn" from "the
 * fourth column was never asked for", which one arm over the shipped budget
 * cannot say at all.
 *
 * THE EMPTY-COLUMN DROP IS DRIVEN ON A STEP THE SITE DOES NOT SHIP, because
 * none that it ships has one: the fund step's tier 5 is drawn under every
 * division fund/100 has. A fund-group step widened to tier 4 is a declaration
 * export.validateSteps ACCEPTS -- the flank is at the left end, the widening at
 * the other -- and five of the six groups have no tier-4 node, which is the
 * state this drop exists for.
 */
async function widenedColumns() {
  const out = [];
  // THE WIDENING AS A STEP DECLARATION, OVER A COPY. config.steps is the shared
  // STEP_SHAPES array, so a tweak that edited a step in place would hand every
  // later check a page the packager does not ship.
  const widenFundGroup = (/** @type {any} */ c) => {
    c.steps = c.steps.map((/** @type {any} */ s) => (s.key === "fund-group"
      ? Object.assign({}, s, { tiers: s.tiers.concat([4]), widen: [4] })
      : s));
  };
  for (const col of COLUMNS) {
    const path = ["fund-group/general", "fund/100"];
    const narrow = await windowAt(col, 3, path);
    const wide = await windowAt(col, 4, path);
    const want = col.fundWide;
    out.push({
      name: `${col.label}: the fund window is three columns at the budget every reader gets`,
      ok: narrow.columns === 3 && narrow.tiers === "2,3,4" && narrow.banners === 0 &&
          narrow.nodes === col.fund.nodes && narrow.links === col.fund.links &&
          narrow.hairlines === col.fund.hairlines && narrow.right === 1180 - 250,
      detail: `${narrow.columns} column(s) at tiers {${narrow.tiers}}: ${narrow.nodes} nodes, ` +
        `${narrow.links} links, ${narrow.hairlines} sub-pixel ribbon(s), bands ${narrow.bands}, ` +
        `right edge ${narrow.right}px, ${narrow.banners} banner(s) ` +
        `(want tiers {2,3,4}, ${col.fund.nodes}/${col.fund.links}/${col.fund.hairlines}, 930px)`,
    });
    out.push({
      name: `${col.label}: the fund window draws its fourth column where there is room for one`,
      ok: wide.columns === 4 && wide.tiers === "2,3,4,5" && wide.banners === 0 &&
          wide.nodes === want.nodes && wide.links === want.links &&
          wide.hairlines === want.hairlines && wide.tail === want.tail &&
          wide.bands === want.bands && wide.right === want.right,
      detail: `${wide.columns} column(s) at tiers {${wide.tiers}}: ${wide.nodes} nodes ` +
        `(want ${want.nodes}), ${wide.links} links (want ${want.links}), ${wide.hairlines} ` +
        `sub-pixel ribbon(s) (want ${want.hairlines}), bands ${wide.bands} (want ${want.bands}), ` +
        `tail "${wide.tail}" (want "${want.tail}"), right edge ${wide.right}px ` +
        `(want ${want.right}px), ${wide.banners} banner(s)`,
    });
  }
  // THE DROP, AND THE SAME STEP ON THE ONE GROUP THAT FILLS THE COLUMN. Both
  // are needed: the first says an empty widened column is dropped rather than
  // banner or stretch the chart, and the second says the drop is a measurement
  // of the document and not a widening that never worked.
  const col = COLUMNS[0];
  const empty = await windowAt(col, 4, [PAGE.worst], widenFundGroup);
  // THE SHIPPED STEP ON THE SAME GROUP, so the arm can say the dropped chart IS
  // the narrow one rather than only that it has three columns.
  const asShipped = await windowAt(col, 3, [PAGE.worst]);
  const filled = await windowAt(col, 4, ["fund-group/general"], widenFundGroup);
  out.push({
    name: "a widened column the document leaves empty is dropped, and the chart is re-laid at the columns it has",
    ok: empty.columns === 3 && empty.tiers === "0,2,3" && empty.banners === 0 &&
        empty.right === 1180 - 250 && empty.nodes === EMPTY_DROP.nodes &&
        empty.links === EMPTY_DROP.links &&
        empty.nodes === asShipped.nodes && empty.links === asShipped.links &&
        empty.bands === asShipped.bands,
    detail: `${PAGE.worst} widened to tier 4 draws ${empty.columns} column(s) at ` +
      `{${empty.tiers}}: ${empty.nodes} nodes, ${empty.links} links, bands ` +
      `${empty.bands}, right edge ${empty.right}px, ${empty.banners} banner(s) ` +
      `(want 3 columns at {0,2,3}, ${EMPTY_DROP.nodes}/${EMPTY_DROP.links}, 930px, no ` +
      `banner); the shipped three-column step draws ${asShipped.nodes} nodes, ` +
      `${asShipped.links} links, bands ${asShipped.bands}`,
  });
  out.push({
    name: "the same widened step keeps its fourth column on the one group whose document fills it",
    ok: filled.columns === 4 && filled.tiers === "0,2,3,4" && filled.banners === 0 &&
        filled.nodes === FILLED_WIDE.nodes && filled.links === FILLED_WIDE.links &&
        filled.bands === FILLED_WIDE.bands,
    detail: `fund-group/general widened to tier 4 draws ${filled.columns} column(s) at ` +
      `{${filled.tiers}}: ${filled.nodes} nodes, ${filled.links} links, bands ` +
      `${filled.bands}, right edge ${filled.right}px, ${filled.banners} banner(s) ` +
      `(want 4 columns at {0,2,3,4}, ${FILLED_WIDE.nodes}/${FILLED_WIDE.links}, ` +
      `bands ${FILLED_WIDE.bands})`,
  });
  return out;
}
