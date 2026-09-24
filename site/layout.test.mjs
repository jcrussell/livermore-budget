// layout.test.mjs — what the client's layout does over the columns Go pins:
// ribbon crossings and overlap under each node ordering, restackLinks, where a
// label is anchored and how much room it has, the four-column window, the
// contra band, and the two stylesheet rules jsdom cannot evaluate.
//
// EVERY TEST DIAGNOSES WHAT IT MEASURED, PASS OR FAIL, and a figure asserted
// here is the evidence for a decision the page made (AGENTS.md, "Before you
// quote a number"). A test asserting without a diagnostic has dropped a pin.
//
// EVERY NUMBER HERE IS MEASURED POST-RESTACK, because that is the chart a
// reader sees; pre-restack figures appear only where a test is about what
// restackLinks itself does. Nothing here re-derives a figure Go emitted, holds
// a copy of a function, or reads source text.

import { before, describe, test } from "node:test";
import assert from "node:assert/strict";

import { bootedApp, goldenGraph, pageFixture, stylesheet, opened, expandAll, everyOffer,
  settle } from "./testlib.mjs";

/**
 * Lays the golden graph out exactly as layOut() does, under a given node sort
 * and a given aligner.
 *
 * The constants come out of app.js rather than being repeated here: a chart laid
 * out at a different width has different crossings, so a copy carried here would
 * drift into checking a chart the page does not draw.
 *
 * THE ALIGNER COMES OFF THE PAGE FOR THAT SAME REASON. app.js hands a graph to
 * d3's sankeyJustify only while no tier set is declared and aligns on the
 * declared order otherwise (alignFor), and the pinned page declares one. A
 * literal `.nodeAlign(sankeyJustify)` would measure an aligner the page does not
 * use and would be green because the two happen to agree. The default is the
 * page's; a caller passes one only to compare the two, which is what makes that
 * agreement a measurement rather than an assumption.
 */
function layout(app, nodeSort, align) {
  const doc = goldenGraph();
  const sankey = globalThis.d3.sankey()
    .nodeId((d) => d.id)
    .nodeWidth(app.NODE_WIDTH)
    .nodePadding(app.NODE_PADDING)
    .nodeAlign(align || app.alignFor(app.RENDER_TIERS))
    .extent([[app.LABEL_GUTTER, 12],
             [app.chartWidth(app.drawnColumns()) - app.LABEL_GUTTER,
              app.CHART_HEIGHT - 12]]);
  if (nodeSort !== "d3") sankey.nodeSort(nodeSort);
  return sankey({
    nodes: doc.nodes.map((n) => Object.assign({}, n)),
    links: doc.links.map((l) => Object.assign({}, l, { value: l.value_cents })),
  });
}

/** Whether two layouts put every node and every ribbon in the same place. */
function samePlaces(a, b) {
  return a.nodes.length === b.nodes.length && a.links.length === b.links.length &&
    a.nodes.every((n, i) => n.id === b.nodes[i].id && n.depth === b.nodes[i].depth &&
      Math.abs(n.x0 - b.nodes[i].x0) < 1e-9 && Math.abs(n.y0 - b.nodes[i].y0) < 1e-9 &&
      Math.abs(n.y1 - b.nodes[i].y1) < 1e-9) &&
    a.links.every((l, i) => Math.abs(l.y0 - b.links[i].y0) < 1e-9 &&
      Math.abs(l.y1 - b.links[i].y1) < 1e-9 &&
      Math.abs(l.width - b.links[i].width) < 1e-9);
}

/**
 * The links of each band: a band is one pair of adjacent columns.
 *
 * FAILS CLOSED ON A LINK THAT SPANS MORE THAN ONE COLUMN. Every link in this
 * graph joins adjacent columns today, and the crossing count below is only
 * correct while that holds: a ribbon that skipped a column would pass OVER the
 * ribbons in the column it skipped, and comparing it only against links of the
 * same span would miss every one of those crossings and report a smaller,
 * confident, wrong number.
 */
function bands(graph) {
  const out = new Map();
  for (const l of graph.links) {
    const span = l.target.depth - l.source.depth;
    if (span !== 1) {
      throw new Error(
        `link ${l.source.id} -> ${l.target.id} spans ${span} columns; this ` +
        `file counts crossings band by band and would undercount it`);
    }
    const key = l.source.depth + ":" + l.target.depth;
    if (!out.has(key)) out.set(key, []);
    out.get(key).push(l);
  }
  return out;
}

/**
 * Crossings and the ribbon value they overlap.
 *
 * Two ribbons in a band cross when the order of their left ends and the order of
 * their right ends disagree — a sign flip. Only same-band pairs can cross,
 * because a ribbon does not leave its band. Overlap is charged at the narrower
 * of the two, which is the most either can hide of the other.
 */
function tangle(graph) {
  let crossings = 0;
  let cents = 0;
  for (const band of bands(graph).values()) {
    for (let i = 0; i < band.length; i++) {
      for (let j = i + 1; j < band.length; j++) {
        const a = band[i], b = band[j];
        if ((a.y0 - b.y0) * (a.y1 - b.y1) < 0) {
          crossings++;
          cents += Math.min(a.value, b.value);
        }
      }
    }
  }
  return { crossings, dollars: cents / 100 };
}

/**
 * Pairs of a node's ribbons stacked in an order its neighbours no longer sit in.
 *
 * This is the defect restackLinks() exists to remove, counted the way its doc
 * comment counts it: d3 sorts a node's links by where the other end sits, then
 * moves nodes again and never re-sorts.
 */
function staleStacked(graph) {
  let pairs = 0;
  for (const node of graph.nodes) {
    for (const [links, end] of [[node.sourceLinks, (l) => l.target],
                                [node.targetLinks, (l) => l.source]]) {
      const wanted = links.slice().sort((a, b) => end(a).y0 - end(b).y0 || a.index - b.index);
      for (let i = 0; i < links.length; i++) {
        for (let j = i + 1; j < links.length; j++) {
          if (wanted.indexOf(links[i]) > wanted.indexOf(links[j])) pairs++;
        }
      }
    }
  }
  return pairs;
}

/** The worst distance any ribbon end falls outside the node face it meets. */
function extentOverflow(graph) {
  let worst = 0;
  for (const node of graph.nodes) {
    for (const [links, y] of [[node.sourceLinks, (l) => l.y0], [node.targetLinks, (l) => l.y1]]) {
      for (const l of links) {
        worst = Math.max(worst, node.y0 - (y(l) - l.width / 2), (y(l) + l.width / 2) - node.y1);
      }
    }
  }
  return worst;
}

// The figures nodeRank was chosen on, and the alternatives it was chosen
// against. Every one is PINNED, not bounded: a `>` comparison would let the
// alternatives drift while the check stayed green, which is the exact defect
// this file exists to remove.
const CLAIMED = {
  crossings: 195,
  overlapDollars: 457434169,
  stalePairs: 14,
};

/** Every ordering still reachable in the tree, measured post-restack. */
const ALTERNATIVES = {
  "size descending": { sort: (a, b) => b.value - a.value, crossings: 285, dollars: 966956035 },
  "size ascending": { sort: (a, b) => a.value - b.value, crossings: 285, dollars: 966956035 },
  "input order": { sort: null, crossings: 297, dollars: 1136201320 },
  "d3's own pass": { sort: "d3", crossings: 246, dollars: 556944973 },
};

const usd = (n) => "$" + Math.round(n).toLocaleString();

// THE ADVANCE A LABEL IS MEASURED AT, and an over-estimate on purpose. The
// stylesheet sets `system-ui`, which is a different face on every platform, so
// no test can know the true advance -- 0.6em is wider than mixed-case English
// sets in any of the usual system faces, so a label this says fits fits
// everywhere, and the direction it can be wrong in is refusing a label that
// would have fitted.
const ADVANCE_EM = 0.6;
const LABEL_PX = 12;
const VALUE_PX = 11;

/**
 * How wide the three tspans of one node's label draw.
 *
 * THE WORDS COME OFF THE PAGE. markCents, fmtShortSigned and nodeFlags are the
 * same three functions render() hands the value and flag tspans, so this
 * measures the string a reader sees rather than one this file spelled for
 * itself -- and the flag tspan is counted, because a marked node's label is
 * characters longer than its neighbours' and that is exactly the case a fit
 * check is about.
 *
 * THE MARKERS ARE ASKED FOR AND NOT SPELLED HERE, which is the whole of why
 * nodeFlags is called. A copy of its rule would measure the markers this file
 * knew about, so a marker added to the page would leave every label below
 * measured a glyph narrower than it draws -- silently, and in the one direction
 * these deliberate over-estimates are chosen NOT to be wrong in.
 */
function labelWidth(app, d, qualifier) {
  const value = "  " + app.fmtShortSigned(app.markCents(d)) + app.nodeFlags(d);
  const label = d.label.length * LABEL_PX * ADVANCE_EM + value.length * VALUE_PX * ADVANCE_EM;
  // THE WIDEST LINE IS THE BOX, not the sum. A qualified mark draws its
  // parent's name on a line of its own above the label, so the two do not add
  // up -- which is the whole reason the qualifier is a second line and not a
  // longer first one: the pair on one line wants 348px of a 250px gutter.
  return Math.max(label, (qualifier || "").length * LABEL_PX * ADVANCE_EM);
}

// THE INK ONE LINE CLAIMS ABOVE AND BELOW ITS BASELINE, over-estimated on
// purpose the way ADVANCE_EM is: they sum to the full 12px font size, where a
// real face's cap height and descender leave some of it unused. So a gap this
// file calls clear is clear everywhere, and the direction it can be wrong in is
// refusing a pair of labels that would have missed each other.
const ASCENT_PX = 9;
const DESCENT_PX = 3;

/**
 * The vertical run one mark's label draws over: one line where the mark needs
 * no qualifier, and the qualifier's line plus the label's where it does.
 *
 * THE SHIFTS COME OFF THE PAGE. labelLineShift is what render() hands the two
 * tspans, so this measures where the lines are drawn rather than where this
 * file would have put them.
 */
function labelBlock(app, place, qualifier) {
  const em = (v) => (v ? parseFloat(v) * LABEL_PX : 0);
  const first = place.y + em(place.dy);
  if (!qualifier) return { top: first - ASCENT_PX, bottom: first + DESCENT_PX };
  const shift = app.labelLineShift(place.anchor);
  const top = first + em(shift.qualifier);
  return { top: top - ASCENT_PX, bottom: top + em(shift.label) + DESCENT_PX };
}

/** The x-range a label of this width occupies, anchored this way at this x. */
function boxAt(anchor, x, width) {
  const left = anchor === "end" ? x - width : anchor === "middle" ? x - width / 2 : x;
  return { left, right: left + width };
}

/** Every column the graph drew, as an x-range, keyed by app.js's own index. */
function columnsOf(app, graph) {
  const out = new Map();
  for (const n of graph.nodes) {
    const c = app.columnOf(n);
    if (!out.has(c)) out.set(c, { x0: n.x0, x1: n.x1 });
  }
  return out;
}

/**
 * The x-range a column's labels may claim.
 *
 * EACH COLUMN OWNS HALF THE BAND ON EITHER SIDE OF IT, and the two outermost
 * own the gutter out to the edge of the chart. The half is the line worth
 * drawing because past it a label is nearer the next column's marks than its
 * own, and because the next column's labels are coming the other way.
 *
 * THIS IS NOT READ OFF labelPlacement, which is the point: the rule says where
 * a label goes and this says what room exists, so the two can disagree.
 */
function roomFor(app, columns, col) {
  const last = Math.max(...columns.keys());
  const me = columns.get(col);
  const before = columns.get(col - 1);
  const after = columns.get(col + 1);
  return {
    from: col === 0 || !before ? 0 : (before.x1 + me.x0) / 2,
    // THE CHART'S OWN RIGHT EDGE, AT ITS OWN COLUMN COUNT. A window of four
    // columns is laid out wider than one of three, and the last column's label
    // runs into the gutter that width put there.
    to: col >= last || !after
      ? app.chartWidth(app.drawnColumns()) : (me.x1 + after.x0) / 2,
  };
}

/**
 * Every label of a laid graph, boxed where app.js puts it, against its room.
 *
 * MEASURED WITH THE VIEW ON SCREEN: columnOf and nodeFlags read the drill
 * state, so a fit taken after the chart moved on measures the wrong chart.
 */
function labelFit(app, graph) {
  const columns = columnsOf(app, graph);
  const last = Math.max(...columns.keys());
  // THE WORDS THE PAGE WOULD DRAW, from the page's own rule. A fit measured
  // from d.label alone measures the document; what a reader sees is the
  // document plus whatever labelQualifiers adds to tell two marks apart.
  const qualifiers = app.labelQualifiers(graph.nodes);
  return graph.nodes.map((n) => {
    const place = app.labelPlacement(n, last);
    const qualifier = qualifiers.get(n.id) || "";
    const width = labelWidth(app, n, qualifier);
    const box = boxAt(place.anchor, place.x, width);
    const col = app.columnOf(n);
    const room = roomFor(app, columns, col);
    return { node: n, id: n.id, col, last, place, width, box, room, qualifier,
             words: (qualifier ? qualifier + " / " : "") + n.label,
             block: labelBlock(app, place, qualifier),
             clearance: Math.min(box.left - room.from, room.to - box.right) };
  });
}

/**
 * Every pair of marks a reader would see the same words on: same column, same
 * drawn label, qualifier and all.
 *
 * THE DEFECT STATED AS A PROPERTY. fund-flows labels a tier-5 cell by its object
 * category and carries the division in `parent`, so the fund window's fourth
 * column drew six marks reading "Wages & Benefits" and a reader could tell them
 * apart only by following a ribbon back (fisc-og1n). Two marks a reader cannot
 * tell apart is the thing, at any tier and in any window, so this asks it of
 * the words rather than of the tier.
 */
function sameWords(fits) {
  const seen = new Map();
  for (const f of fits) {
    const key = f.col + " " + f.words;
    seen.set(key, (seen.get(key) || []).concat([f.id]));
  }
  return [...seen.entries()]
    .filter((e) => e[1].length > 1)
    .map((e) => `column ${e[0].split(" ")[0]} draws ${e[1].length} marks ` +
                `reading "${e[0].split(" ")[1]}" (${e[1].join(", ")})`);
}

/** The tightest vertical gap between two label blocks of one column. */
function tightestStack(fits) {
  let worst = null;
  const columns = new Set(fits.map((f) => f.col));
  for (const col of columns) {
    const stacked = fits.filter((f) => f.col === col).sort((a, b) => a.block.top - b.block.top);
    for (let i = 1; i < stacked.length; i++) {
      const gap = stacked[i].block.top - stacked[i - 1].block.bottom;
      if (!worst || gap < worst.gap) {
        worst = { gap, col, above: stacked[i - 1].id, below: stacked[i].id };
      }
    }
  }
  return worst;
}

/**
 * The widest the stylesheet lets the chart figure draw, in px.
 *
 * A DECLARATION, NOT A RENDERING: nothing here parses CSS, so this reads
 * .chart-wrap's allowance as text and can say only what style.css says.
 *
 * THE CAP IS NOT A NUMBER IN THE STYLESHEET, so what this reads is which
 * PROPERTY the cap comes through, and the px comes from the value app.js hands
 * that property. The pair is two files that nothing else compares, and a
 * disagreement is a wiring mistake rather than a number somebody forgot to
 * raise in one of four places.
 *
 * IT PINS THE SHAPE OF THE DECLARATION AND NOT ONLY ITS NUMBER. An equivalent
 * rule written with clamp() would go red here; that is the cost of reading a
 * stylesheet with a regex, and the direction it fails in is the safe one.
 *
 * ONLY A RULE WHOSE WHOLE SELECTOR IS .chart-wrap COUNTS, and it refuses two
 * of them. `.card:fullscreen .chart-wrap {` contains `.chart-wrap {` as a
 * substring, so a first-match read of the text answers from whichever rule
 * comes first in the file; and a second .chart-wrap rule under an @media block
 * is a cap this file cannot say applies at which width. Either is a refusal
 * by name rather than a number.
 *
 * @param {string} css
 * @param {number | null} handed the px app.js set --chart-max to, or null
 */
function chartAllowance(css, handed) {
  // A RULE BOUNDARY BEFORE THE SELECTOR: the start of the text, a brace or
  // the end of a comment. A descendant or compound selector puts a space, a
  // combinator or a colon there instead and does not match.
  const rules = [...css.matchAll(/(?:^|[{}]|\*\/)\s*\.chart-wrap\s*\{([^}]*)\}/g)];
  if (rules.length === 0) return { px: null, why: "style.css declares no .chart-wrap rule" };
  const capped = rules.filter((r) => /--chart-room:/.test(r[1]));
  if (capped.length > 1) {
    return { px: null, why: `${capped.length} .chart-wrap rules declare --chart-room, and this file ` +
      `cannot say which applies at which width` };
  }
  const rule = capped[0] || rules[0];
  if (!/--chart-room:\s*min\(\s*var\(--chart-max[,)]/.test(rule[1])) {
    return { px: null, why: `.chart-wrap does not cap --chart-room with --chart-max: ${rule[1].trim()}` };
  }
  if (!/(^|[\s;])width:\s*var\(--chart-room\)/.test(rule[1])) {
    return { px: null, why: ".chart-wrap caps --chart-room but does not take its width from it" };
  }
  if (handed === null) {
    return { px: null, why: "app.js set no --chart-max, so the stylesheet's cap falls back to 100%" };
  }
  // THE CUSHION IS READ OFF ITS OWN DECLARATION AND NOT OUT OF THE min(), since
  // the second operand is a var() too. Both are in this rule, so a cushion
  // declared nowhere is still caught.
  const cushion = /--chart-cushion:\s*(\d+)px/.exec(rule[1]);
  if (!/calc\(100vw\s*-\s*var\(--chart-cushion\)\)/.test(rule[1])) {
    return { px: null, why: ".chart-wrap does not take its sub-cap width from 100vw less --chart-cushion" };
  }
  return {
    px: handed,
    // WHAT THE CAP COSTS BELOW ITSELF, which is the other half of the same
    // declaration and the half COLUMN_QUERIES has to agree with. Below the cap
    // the chart gets 100vw minus this, so the viewport at which a chart of n
    // columns FITS is chartWidth(n) + cushion -- not a round number, and a
    // breakpoint picked for looking like one puts the reader one column
    // narrower than they asked for (fisc-5e2b).
    cushion: cushion ? Number(cushion[1]) : null,
    why: "",
  };
}

// THE ONE QUERY GRAMMAR THIS FILE CAN READ A BREAKPOINT OUT OF. An entry of
// COLUMN_QUERIES it does not match is a named failure below, carrying the
// query, rather than a throw out of a hook (fisc-bxal).
const MIN_WIDTH_PX = /\(min-width:\s*(\d+)px\)/;

/**
 * The gap app.js leaves between a rect and a label anchored outward from it,
 * read off a label that IS anchored that way rather than copied as a literal.
 */
function outwardGap(fits) {
  const outer = fits.find((f) => f.col >= f.last);
  return outer.place.x - outer.node.x1;
}

/** The same label anchored outward instead -- what the middle column escaped. */
function outward(fit, gap) {
  return boxAt("start", fit.node.x1 + gap, fit.width);
}

const px = (n) => n.toFixed(1) + "px";

/** The tightest fit of a set, for a detail line that names a node. */
function tightest(fits) {
  return fits.slice().sort((a, b) => a.clearance - b.clearance)[0];
}

/**
 * The page as shipped, booted on its first published column.
 *
 * AND IT DRAWS BEFORE ANYTHING IS MEASURED. nodeRank orders the fund column by
 * the place the COLUMN gives each group, so an app that laid a document out
 * without fetching one is an app ordering by a list it was never served --
 * every group ties and the column degenerates to size-descending, which is one
 * of the alternatives measured below and would be reported as nodeRank's own
 * number.
 */
function spineApp() {
  return bootedApp({ checkedStem: "sankey" });
}

/** The node sort app.js draws the spine under. */
const byRank = (app) => (a, b) => app.nodeRank(a) - app.nodeRank(b) || b.value - a.value;

describe("the spine's layout, against the figures nodeRank was chosen on", () => {
  let app, graph, justified, widths, before_, after_, measured;
  before(async () => {
    ({ app } = await spineApp());
    const sort = byRank(app);
    graph = layout(app, sort);
    justified = layout(app, sort, globalThis.d3.sankeyJustify);
    app.restackLinks(justified);
    before_ = { tangle: tangle(graph), stale: staleStacked(graph) };
    widths = graph.links.map((l) => l.width);
    app.restackLinks(graph);
    after_ = { tangle: tangle(graph), stale: staleStacked(graph) };
    measured = Object.entries(ALTERNATIVES).map(([label, want]) => {
      const g = layout(app, want.sort);
      app.restackLinks(g);
      return { label, want, got: tangle(g) };
    });
  });

  test("the chart draws at the 195 crossings measured under nodeRank + restackLinks", (t) => {
    const detail = `${after_.tangle.crossings} crossings under nodeRank + restackLinks ` +
      `(measured ${CLAIMED.crossings})`;
    t.diagnostic(detail);
    assert.equal(after_.tangle.crossings, CLAIMED.crossings, detail);
  });

  test("the overlapping ribbon is the $457,434,169 measured", (t) => {
    const detail = `${usd(after_.tangle.dollars)} overlapped (measured ${usd(CLAIMED.overlapDollars)})`;
    t.diagnostic(detail);
    assert.equal(Math.round(after_.tangle.dollars), CLAIMED.overlapDollars, detail);
  });

  test("restackLinks finds the 14 stale-stacked pairs measured on FY2026", (t) => {
    const detail = `${before_.stale} pairs stacked in an order their neighbours no longer ` +
      `sit in (measured ${CLAIMED.stalePairs})`;
    t.diagnostic(detail);
    assert.equal(before_.stale, CLAIMED.stalePairs, detail);
  });

  // NOT "after.stale === 0", which is a tautology: staleStacked re-derives
  // the same comparator restackLinks just sorted by, so it cannot fail.
  // What is independent is the CROSSINGS those pairs cost, counted by a
  // different function against ribbon endpoints rather than node order.
  test("removing those 14 pairs removes exactly 14 crossings", (t) => {
    const detail = `${before_.tangle.crossings} crossings before restacking, ` +
      `${after_.tangle.crossings} after: ` +
      `${before_.tangle.crossings - after_.tangle.crossings} removed`;
    t.diagnostic(detail);
    assert.equal(before_.tangle.crossings - after_.tangle.crossings, CLAIMED.stalePairs, detail);
  });

  test("every ordering still in the tree measures at its pinned crossings and overlap", (t) => {
    const detail = measured.map((m) => `${m.label} ${m.got.crossings}/${usd(m.got.dollars)}`).join(", ") +
      `, nodeRank ${after_.tangle.crossings}/${usd(after_.tangle.dollars)}`;
    t.diagnostic(detail);
    assert.deepEqual(
      measured.map((m) => ({ label: m.label, crossings: m.got.crossings, dollars: Math.round(m.got.dollars) })),
      measured.map((m) => ({ label: m.label, crossings: m.want.crossings, dollars: m.want.dollars })),
      detail);
  });

  test("nodeRank beats every one of them", (t) => {
    const detail = `best alternative ${Math.min(...measured.map((m) => m.got.crossings))} crossings, ` +
      `nodeRank ${after_.tangle.crossings}`;
    t.diagnostic(detail);
    assert.deepEqual(
      measured.filter((m) => m.got.crossings <= after_.tangle.crossings ||
                             m.got.dollars <= after_.tangle.dollars)
        .map((m) => m.label),
      [], detail);
  });

  // THE MEASUREMENT THAT LET THE SPINE DECLARE A COLUMN ORDER AT ALL. Every
  // figure above is measured under the declared order, and the reason the
  // figures are the same under sankeyJustify is structural rather than lucky
  // -- tier 0 is pure source, tier 5 pure sink, and justify's own rule puts a
  // link-less sink in the last column, which is where indexOf puts tier 5.
  // This says so where it can go red: the day the spine grows a tier, or
  // declares its columns in an order topology disagrees with, this fails and
  // the figures above are a new measurement rather than the old one.
  test("the spine lays out identically under its declared column order and under d3's justify", (t) => {
    const detail = app.RENDER_TIERS.length
      ? `every node and ribbon in the same place under nodeAlign indexOf ` +
        `[${app.RENDER_TIERS.join(", ")}] and under sankeyJustify`
      : "the config this ran under declares no column order, so nothing here " +
        "measures the aligner the page uses";
    t.diagnostic(detail);
    assert.ok(app.RENDER_TIERS.length > 0, detail);
    assert.ok(samePlaces(graph, justified), detail);
  });

  test("the fund column is pinned to the order the column shipped", (t) => {
    const drawn = graph.nodes.filter(app.isFundGroup)
      .sort((a, b) => a.y0 - b.y0).map((n) => n.id);
    // THE SERVED LIST FILTERED BY WHAT IS DRAWN, and the filter is what
    // makes this an equality rather than a subset test: every group the
    // chart lays out has to appear, in the served order, with nothing
    // between them. A group the packager's sequence does not name is at
    // the END of that list, so this fails if the page draws it anywhere
    // else -- which is the direction an indexOf that answers -1 breaks in.
    const served = app.fundGroups().map((g) => g.id);
    const detail = `fund groups run top to bottom in the order the column shipped ` +
      `[${drawn.join(", ")}], which is what supplying .nodeSort() at all is for`;
    t.diagnostic(detail);
    assert.ok(served.length > 0, detail);
    assert.deepEqual(drawn, served.filter((f) => drawn.includes(f)), detail);
  });

  test("restackLinks moves ribbons without resizing them", (t) => {
    const detail = "every ribbon keeps the width the layout gave it";
    t.diagnostic(detail);
    assert.deepEqual(graph.links.map((l) => l.width), widths, detail);
  });

  test("no ribbon overflows the node face it meets", (t) => {
    const detail = `worst overflow ${extentOverflow(graph).toFixed(6)}px`;
    t.diagnostic(detail);
    assert.ok(extentOverflow(graph) <= 1e-9, detail);
  });
});

/** The clear run between one column's rects and the next's, as laid out. */
function bandWidth(laid) {
  const xs = [...new Set(laid.nodes.map((n) => n.x0))].sort((a, b) => a - b);
  return xs[1] - laid.nodes.find((n) => n.x0 === xs[0]).x1;
}

// The two windows the label checks run over, and what each is for. Read off the
// committed corpus by the same drill a reader clicks.
const OBJECT_WINDOW = "expenditure/wages-and-benefits";
const FUND_GROUP_WINDOW = "fund-group/general";

// Nodes of the fund-group window whose declared column is not d3's longest path
// to them, and the one that is. MEASURED, and the evidence that columnOf and
// d.depth are different questions on a chart the site already draws: the
// residual has no ribbon reaching it from the middle column, so its longest
// path is one while the view declares it third.
const DEPTH_DISAGREES = ["residual/fund-group/general"];

// The four-column chart the band and geometry tests run over: the fund window
// one rung inside the General Fund group, which the page's step from tier 3
// widens by tier 5. Its bands are [the group into the fund | the fund into its
// 23 divisions | those divisions into the object cells the tier-5 cap leaves],
// measured over the committed FY 2025-26 capture.
const WIDE_PATH = ["fund-group/general", "fund/100"];

// And the division opened out of it, which is the window where the SHORT label
// is the right one: Patrol's two cells, with Patrol beside them and in the
// breadcrumb. Patrol because it is the largest division of the fund window's
// fourth column and draws both of its cells.
const DIVISION_WINDOW = "dept/patrol";
const WIDE_BANDS = { keys: "0:1 1:2 2:3", sizes: "1/23/30" };

// The one width this file pins, and it is the THREE-column one: every figure
// here was measured at 1180px, and BAND is 319 precisely so that three columns
// come to it, so a band chosen for its own sake would move all of them at once.
//
// NO WIDER WIDTH IS PINNED HERE, AND NONE MAY BE. chartWidth(n) is a function
// of this width's own constants, so a wider one written down is a copy of
// something already derivable -- and the page's ceiling, its media queries and
// the stylesheet's cap are all that same function. The test below asks what a
// window was laid out at against chartWidth(its own column count), which is
// the claim wanted and holds at any count.
const NARROW_DESIGN_WIDTH = 1180;

/**
 * The bands of a four-column chart.
 *
 * WHY THE BAND COUNT IS THE THING TO CHECK. Everything this file measures --
 * the crossings, the value they overlap, the room a label has -- is counted
 * BAND BY BAND, and bands() refuses a ribbon that spans more than one column
 * because a skipping ribbon passes over marks nothing would compare it against.
 * A fourth column is the first shape in this tree with three of them, and it is
 * also the first that could have produced a hole: d3-sankey takes its column
 * count from topology, so a widened tier the document could not fill would draw
 * three columns' worth of ribbons over four columns' worth of extent.
 *
 * IT ASSERTS bands() RAN, and not only that it did not throw. The two are
 * different: a chart drawing no links at all would raise nothing and count
 * nothing. So the sizes are summed against the graph's own link count and the
 * columns against its own deepest node.
 */
describe("the four-column window", () => {
  let app, laid, columns, keys, sizes, counted, threw;
  let fits, innerFits, ambiguous, stack, allowance, queryRoom, unreadable;
  before(async () => {
    ({ app } = await spineApp());
    app.setColumnBudget(4);
    await opened(app, ...WIDE_PATH);
    laid = app.layOut(app.projection);
    columns = Math.max(...laid.nodes.map((n) => n.depth)) + 1;
    keys = "";
    sizes = "";
    counted = 0;
    threw = "";
    try {
      // SORTED BY BAND, because the Map's own order is the order the links
      // happened to arrive in and says nothing about the chart.
      const b = [...bands(laid).entries()].sort((x, y) => x[0].localeCompare(y[0]));
      keys = b.map((e) => e[0]).join(" ");
      sizes = b.map((e) => e[1].length).join("/");
      counted = b.reduce((n, e) => n + e[1].length, 0);
    } catch (e) {
      threw = String((e && e.message) || e);
    }
    fits = labelFit(app, laid);
    // AND THE WINDOW ONE RUNG FURTHER IN, which is where the other half of the
    // labelling decision is measured: a division's own window draws that
    // division's two cells side by side, so labelling a cell by its division --
    // the short label that would have fitted the fourth column -- draws "Patrol"
    // twice. Both windows have to come out unambiguous or neither rule is right.
    app.drillUp(0);
    await settle();
    await opened(app, ...WIDE_PATH, DIVISION_WINDOW);
    innerFits = labelFit(app, app.layOut(app.projection));
    ambiguous = sameWords(fits).concat(sameWords(innerFits));
    stack = tightestStack(fits);
    // THE PX IS app.js's OWN CONSTANT, because the stylesheet carries none:
    // wireColumns hands --chart-max the value of CHART_MAX, and what this reads
    // off the stylesheet is that the cap comes through that property at all.
    allowance = chartAllowance(stylesheet(), app.CHART_MAX);
    // What each responsive threshold actually buys the chart, at the threshold.
    // Math.min with the cap because a query above it is bounded by the cap, which
    // is the test above's subject rather than this one's.
    //
    // A QUERY THIS FILE CANNOT READ IS SET ASIDE AND NAMED, not dereferenced:
    // a throw here cancels every test of this suite, and the reason would be in
    // a stack rather than in the report.
    unreadable = [];
    queryRoom = [];
    for (const q of app.COLUMN_QUERIES || []) {
      const m = MIN_WIDTH_PX.exec(q.query);
      if (!m) {
        unreadable.push(q.query);
        continue;
      }
      queryRoom.push({
        query: q.query,
        columns: q.columns,
        room: Math.min(allowance.px, Number(m[1]) - allowance.cushion),
        wants: app.chartWidth(q.columns),
      });
    }
  });

  // THE GUTTER DECISION, MEASURED. LABEL_GUTTER does not grow with the
  // column count because only the two end columns anchor outward; the two
  // interior ones are centred over their own rects, in the NODE_PADDING gap
  // above them. This is what says a fourth column did not take a gutter
  // with it.
  test("every label in the four-column window has room where it was anchored", (t) => {
    const detail = fits.length === 0
      ? "the window drew no nodes, so nothing here measured a label at all"
      : `${fits.length} labels over ${new Set(fits.map((f) => f.col)).size} columns; ` +
        `tightest ${tightest(fits).id} anchored ${tightest(fits).place.anchor} ` +
        `with ${px(tightest(fits).clearance)} to spare`;
    t.diagnostic(detail);
    assert.deepEqual({
      drewNothing: fits.length === 0,
      columns: new Set(fits.map((f) => f.col)).size,
      tight: fits.filter((f) => f.clearance < 0).map((f) => f.id),
    }, { drewNothing: false, columns: 4, tight: [] }, detail);
  });

  test("a four-column window lays out in three bands, each between adjacent columns", (t) => {
    const detail = threw !== ""
      ? `bands() refused the four-column window: ${threw}`
      : `${columns} columns and ${keys.split(" ").length} band(s) [${keys}] holding ` +
        `${sizes} of the chart's ${laid.links.length} ribbons (want ${WIDE_BANDS.keys}, ` +
        `${WIDE_BANDS.sizes}); every one of them spans exactly one column, which is ` +
        `what bands() throws on`;
    t.diagnostic(detail);
    assert.deepEqual({ threw, columns, keys, sizes, counted, countedNothing: counted === 0 },
      { threw: "", columns: 4, keys: WIDE_BANDS.keys, sizes: WIDE_BANDS.sizes,
        counted: laid.links.length, countedNothing: false }, detail);
  });

  test("the four-column window is laid out at its own width, not at the three-column one", (t) => {
    const detail = `chartWidth(3) is ${app.chartWidth(3)}px (want ${NARROW_DESIGN_WIDTH}, which is ` +
      `what every figure in this file was measured at) and this window's ${columns} columns ` +
      `lay out at ${app.chartWidth(columns)}px; it draws from ` +
      `${Math.min(...laid.nodes.map((n) => n.x0))}px to ` +
      `${Math.max(...laid.nodes.map((n) => n.x1))}px with ${bandWidth(laid)}px of clear ` +
      `run between columns (want ${app.LABEL_GUTTER}px, ` +
      `${app.chartWidth(columns) - app.LABEL_GUTTER}px and ${app.BAND}px)`;
    t.diagnostic(detail);
    assert.deepEqual({
      narrow: app.chartWidth(3),
      widerThanNarrow: app.chartWidth(columns) > NARROW_DESIGN_WIDTH,
      right: Math.max(...laid.nodes.map((n) => n.x1)),
      left: Math.min(...laid.nodes.map((n) => n.x0)),
      band: bandWidth(laid),
    }, {
      narrow: NARROW_DESIGN_WIDTH, widerThanNarrow: true,
      right: app.chartWidth(columns) - app.LABEL_GUTTER, left: app.LABEL_GUTTER, band: app.BAND,
    }, detail);
  });

  // THE DEFECT AS A PROPERTY OF WHAT IS DRAWN. Not "tier 5 carries its
  // division": that is one fix's shape, and two of them were refused by
  // measurement before this one was written. What a reader needs is that no
  // two marks standing in one column say the same thing, which is asked of
  // the words here and is false under either of the two labels the document
  // could have carried on its own.
  test("no column of the fund window or of a division's own draws two marks a reader would read the same", (t) => {
    const detail = ambiguous.length === 0
      ? `${fits.length} marks over ${new Set(fits.map((f) => f.col)).size} columns and ` +
        `${innerFits.length} over ${new Set(innerFits.map((f) => f.col)).size}, ` +
        `${fits.filter((f) => f.qualifier).length + innerFits.filter((f) => f.qualifier).length} ` +
        `of them qualified, and every one reads differently from its neighbours`
      : ambiguous.join("; ");
    t.diagnostic(detail);
    assert.deepEqual({ ambiguous, outer: fits.length > 0, inner: innerFits.length > 0 },
      { ambiguous: [], outer: true, inner: true }, detail);
  });

  // WHAT THE SECOND LINE COSTS, AND IT IS PAID IN THE ONE DIRECTION THE
  // GUTTER CANNOT HELP WITH. A qualified label is two lines tall where its
  // neighbours are one, and the fourth column stacks nine of them; the
  // test is that the block still misses the block below it. MEASURED, and
  // the figure is the argument for the shape: the tightest pair here clears
  // by more than the tightest pair of SINGLE-line labels in the same
  // window's third column, so the qualifier costs a reader nothing the
  // chart was not already spending.
  test("a qualifier's second line still clears the label stacked below it", (t) => {
    const detail = stack
      ? `tightest stack in the four-column window is column ${stack.col}, ` +
        `${stack.above} over ${stack.below}, clearing ${px(stack.gap)}`
      : "the window stacked no two labels in one column, so nothing here measured a gap";
    t.diagnostic(detail);
    assert.ok(stack, detail);
    assert.ok(stack.gap >= 0, detail);
  });

  // THE CONTAINER AGAINST THE DRAWING, which is the pair nothing else in
  // the tree compares. The chart is an <svg> with a viewBox, so a container
  // narrower than the width app.js lays the chart out at does not clip it
  // and does not reflow it -- it draws the same picture smaller, and a
  // reader who asks for a fourth column gets a fifth less chart (fisc-5e2b).
  // AN EQUALITY, NOT A FLOOR: the cap is derived from the same declarations
  // the ceiling is, so slack IS the defect -- it means one of them was
  // computed from something else.
  //
  // OFFERED_COLUMNS IS READ RATHER THAN SPELLED: asking about a number would
  // leave the ceiling moving without this test noticing, which is the whole
  // failure this test is for.
  test("the stylesheet lets the widest chart app.js will draw draw at the width it lays it out at", (t) => {
    const detail = allowance.px === null
      ? allowance.why
      : `style.css caps the chart through --chart-max, which app.js sets to ${allowance.px}px, ` +
        `and app.js lays its offered ${app.OFFERED_COLUMNS} columns out at ` +
        `${app.chartWidth(app.OFFERED_COLUMNS)}px` +
        (allowance.px === app.chartWidth(app.OFFERED_COLUMNS)
          ? ""
          : ` -- the two disagree, so one of them is not chartWidth(OFFERED_COLUMNS)`) +
        `; the cushion is ${allowance.cushion}px in the stylesheet against app.js's ` +
        `${app.CHART_CUSHION}px` +
        (allowance.cushion === app.CHART_CUSHION ? "" : " -- SKEWED");
    t.diagnostic(detail);
    assert.deepEqual({ px: allowance.px, cushion: allowance.cushion },
      { px: app.chartWidth(app.OFFERED_COLUMNS), cushion: app.CHART_CUSHION }, detail);
  });

  // THE REFUSAL BY NAME. A max-width, a range, an orientation or a resolution
  // query is a breakpoint this file has no way to derive a viewport from, and
  // the test below would measure the list with that entry silently missing.
  test("every viewport query the page declares is a min-width in px this file can read", (t) => {
    const detail = unreadable.length === 0
      ? `${queryRoom.length} column quer${queryRoom.length === 1 ? "y" : "ies"} read, ` +
        `each (min-width: Npx)`
      : `${unreadable.length} of ${unreadable.length + queryRoom.length} column queries ` +
        `cannot be read as (min-width: Npx): ${unreadable.map((q) => JSON.stringify(q)).join(", ")}`;
    t.diagnostic(detail);
    assert.deepEqual(unreadable, [], detail);
  });

  // THE BREAKPOINTS AGAINST THE SAME DECLARATION, WHICH IS THE OTHER WAY
  // THE PAIR CAN DISAGREE. The test above says the CAP is wide enough for
  // the ceiling; this one says every viewport at which the page ADDS a
  // column has room for that column at the width it will be laid out at.
  // A query threshold of 1500px passes the test above untouched and still
  // hands a reader a four-column chart drawn at 95% of itself, because
  // below the cap the room is 100vw minus the cushion. Re-derived from the
  // two shipped files rather than pinned as a number here.
  //
  // EVERY ENTRY, not the widest: a list is the shape of the responsive
  // rule, and the list is composed rather than typed, so this test is what
  // says the composition is right at every column it offers.
  test("every viewport the page adds a column at has room for that column", (t) => {
    const detail = allowance.cushion === null
      ? `.chart-wrap's --chart-room declares no calc(100vw - Npx) arm, so there is nothing to ` +
        `derive a breakpoint from: ${allowance.why || "the cap parsed but the cushion did not"}`
      : queryRoom.map((q) => `${q.query} buys ${q.columns} columns: at that width the ` +
          `stylesheet gives the chart ${q.room}px and app.js lays ${q.columns} out at ` +
          `${q.wants}px${q.room >= q.wants ? "" : " -- SHORT"}`).join("; ") +
        ` (cushion ${allowance.cushion}px, read off the same declaration)`;
    t.diagnostic(detail);
    assert.notEqual(allowance.px, null, detail);
    assert.notEqual(allowance.cushion, null, detail);
    assert.ok(queryRoom.length > 0, detail);
    assert.deepEqual(queryRoom.filter((q) => q.room < q.wants), [], detail);
  });
});

/**
 * Where a drawn label actually sits, read off the mark render() appended.
 *
 * THE TESTS BELOW MEASURE labelPlacement AND THIS MEASURES ITS USE, which is
 * the difference between a rule being right and a chart drawing it. The <text>
 * is reached by walking a mark's own children rather than by a selector, and by
 * its class attribute rather than className, which on an SVG element is an
 * SVGAnimatedString and not a string.
 *
 * x, y AND text-anchor ARE COMPARED AS THE STRINGS THE PAGE WROTE, and dy as
 * the ABSENCE d3 leaves when labelPlacement answers null -- an interior column
 * takes no half-em shift, and an attribute set to "null" would be a different
 * drawing from one not set at all.
 *
 * @param {any} app a page with a chart already drawn
 * @param {Document} document the page it drew on
 * @param {any} laid the same chart's nodes, as laid out
 */
function drawnPlacement(app, document, laid) {
  const last = Math.max(...laid.nodes.map((n) => app.columnOf(n)));
  const chart = document.getElementById("chart");
  const marks = chart ? [...chart.querySelectorAll("g.node")] : [];
  const wrong = [];
  let read = 0;
  for (const m of marks) {
    const d = m.__data__;
    const text = [...m.children].find((c) =>
      c.tagName === "text" && c.getAttribute("class") === "halo");
    if (!text) {
      wrong.push(`${d && d.id}: the mark has no text.halo to place`);
      continue;
    }
    const want = app.labelPlacement(d, last);
    const got = {
      x: text.getAttribute("x"), y: text.getAttribute("y"),
      dy: text.getAttribute("dy"), anchor: text.getAttribute("text-anchor"),
    };
    read += 4;
    const bad = [];
    if (got.x !== String(want.x)) bad.push(`x ${got.x} want ${want.x}`);
    if (got.y !== String(want.y)) bad.push(`y ${got.y} want ${want.y}`);
    if (got.dy !== (want.dy === null ? null : String(want.dy))) {
      bad.push(`dy ${JSON.stringify(got.dy)} want ${JSON.stringify(want.dy)}`);
    }
    if (got.anchor !== want.anchor) bad.push(`text-anchor ${got.anchor} want ${want.anchor}`);
    if (bad.length) wrong.push(`${d.id}: ${bad.join(", ")}`);
  }
  return { read, marks: marks.length, wrong, last };
}

/**
 * Where every label lands, on the spine and inside a window.
 *
 * WHY A WINDOW AND NOT THE SPINE ALONE. Every path through the spine is the
 * same length, so d3's depth and the column the view declares agree on all 25
 * of its nodes and a rule keyed on either reads the same. A check written there
 * cannot tell the two apart and would be green because the gate fired rather
 * than because the rule is right. The windows are where they come apart.
 *
 * THE SPINE IS MEASURED ON THE PAGE AS SHIPPED, where four of its marks open:
 * a drillable node's label carries the open marker and is not the width of an
 * inert one (fisc-clbw).
 */
describe("where a label is anchored, on the windows that can tell the rules apart", () => {
  let inWindow, middle, crowded, disagree, groupLaid, drawn;
  before(async () => {
    const { app, document } = await spineApp();
    const spine = layout(app, byRank(app));
    app.restackLinks(spine);
    const onSpine = labelFit(app, spine);
    middle = onSpine.filter((f) => f.col > 0 && f.col < f.last);
    const gap = outwardGap(onSpine);
    crowded = middle.filter((f) => outward(f, gap).right > f.room.to);
    await opened(app, OBJECT_WINDOW);
    const objectLaid = app.layOut(app.projection);
    inWindow = labelFit(app, objectLaid);
    // READ OFF THE OBJECT-CATEGORY WINDOW, because it is the chart the test
    // above measures and it has an INTERIOR column: on the spine every
    // placement rule agrees, so a comparison there would be green whichever
    // rule drew it.
    drawn = drawnPlacement(app, document, objectLaid);
    app.drillUp(0);
    await settle();
    await opened(app, FUND_GROUP_WINDOW);
    groupLaid = app.layOut(app.projection);
    disagree = groupLaid.nodes.filter((n) => app.columnOf(n) !== n.depth).map((n) => n.id);
  });

  // THE MUTATION THIS TEST IS FOR: change labelPlacement's interior branch
  // back to anchoring outward, and the window's middle column runs past the
  // midline and this goes red naming the node. It does NOT see render()
  // reverted to `d.depth === 0`, because it reads labelPlacement and not
  // the page; that is the test below.
  test("every label in the object-category window is anchored on a side it has room on", (t) => {
    const detail = inWindow.length === 0
      ? "the window drew no nodes, so nothing here measured a label at all"
      : `${inWindow.length} labels over ${new Set(inWindow.map((f) => f.col)).size} columns; ` +
        `tightest ${tightest(inWindow).id} anchored ${tightest(inWindow).place.anchor} ` +
        `with ${px(tightest(inWindow).clearance)} to spare`;
    t.diagnostic(detail);
    assert.deepEqual({
      columns: new Set(inWindow.map((f) => f.col)).size,
      tight: inWindow.filter((f) => f.clearance < 0).map((f) => f.id),
      drewNothing: inWindow.length === 0,
    }, { columns: 3, tight: [], drewNothing: false }, detail);
  });

  // The same shape as "nodeRank beats every one of them": the rule is
  // asserted AND the alternative it was chosen over is measured, so the day
  // the alternative starts fitting this says so instead of staying quiet.
  // THE COUNT IS PRINTED AND NOT PINNED: the claim is that anchoring outward
  // does not fit, and how many of the column's labels it fails on is what a
  // marker or a relabel moves without touching the claim.
  test("the spine's middle column is centred because anchoring it outward does not fit", (t) => {
    const detail = `${middle.length} middle labels anchored ` +
      `${[...new Set(middle.map((f) => f.place.anchor))].join("/")}, tightest ` +
      `${middle.length ? px(tightest(middle).clearance) : "n/a"} clear; anchored outward instead, ` +
      `${crowded.length} of ${middle.length} cross into the next column's half of the band`;
    t.diagnostic(detail);
    assert.deepEqual({
      drewNoMiddle: middle.length === 0,
      anchors: [...new Set(middle.map((f) => f.place.anchor))],
      tight: middle.filter((f) => f.clearance < 0).map((f) => f.id),
      outwardFits: crowded.length === 0,
    }, { drewNoMiddle: false, anchors: ["middle"], tight: [], outwardFits: false }, detail);
  });

  // THE MUTATION THIS TEST IS FOR: revert render()'s four label .attr lines
  // to `d.depth === 0 ? ... : ...` and leave labelPlacement untouched, and
  // this goes red naming every interior mark. Every other test in this file
  // stays green under it, including the two above -- they read
  // labelPlacement, which that mutation does not touch.
  test("every label is drawn where labelPlacement puts it", (t) => {
    const detail = drawn.marks === 0
      ? "the window drew no marks, so nothing here read a placement off the page at all"
      : `${drawn.read} attribute(s) read off ${drawn.marks} drawn label(s) over ` +
        `${drawn.last + 1} column(s), each equal to labelPlacement's own answer` +
        (drawn.wrong.length ? `; ${drawn.wrong.length} disagree: ${drawn.wrong[0]}` : "");
    t.diagnostic(detail);
    assert.deepEqual({ drewNothing: drawn.marks === 0, read: drawn.read, wrong: drawn.wrong },
      { drewNothing: false, read: drawn.marks * 4, wrong: [] }, detail);
  });

  test("a label keys on the column the view declares, not on d3's longest path", (t) => {
    const detail = `of the fund-group window's ${groupLaid.nodes.length} nodes, ` +
      `${disagree.length} sit${disagree.length === 1 ? "s" : ""} in a column ` +
      `d3's depth does not name: ` +
      (disagree.join(", ") || "none, so this chart cannot tell the two rules apart");
    t.diagnostic(detail);
    assert.deepEqual(disagree, DEPTH_DISAGREES, detail);
  });
});

/**
 * The marks whose words run past the room they have, over every view the drill
 * opens -- named, and not counted.
 *
 * WHICH VIEWS ARE MEASURED IS NOT A CHOICE THIS FILE MAKES. The suites above
 * measure windows a caller picked, and a step added anywhere leaves them
 * measuring the shape somebody chose last time; everyOffer re-opens from the
 * overview at every rung and reads its children off drillable on the DRAWN
 * chart, so a column this file never heard of arrives on its own. How many
 * views that is gets printed, not pinned here.
 *
 * THE SET AND NOT ITS SIZE. Nothing in a number says which mark joined or
 * left. Each id below is a mark whose label is wider than the 240px it has at
 * ADVANCE_EM, which is deliberately wider than any system face sets -- so this
 * is an upper bound on a real defect rather than a count of it, and fisc-rl4j's
 * browser walk is what settles which of them a rendered face actually
 * overflows.
 */
const OVER_GUTTER = {
  "FY 2025-26": [
    "department/innovation-and-economic-development",
    "dept/administrative-services", "dept/community-development-admin",
    "dept/innovation-and-economic-devel", "dept/public-works-administration",
    "fund/202", "fund/282", "fund/283", "fund/513", "fund/551", "fund/552",
    "fund/623", "fund/730",
    "revenue-line/charges-for-services/administrative-cost-recovery",
    "revenue-line/charges-for-services/engineering-inspection-fees",
    "revenue-line/charges-for-services/fire-plan-check-and-inspct-fee",
    "revenue-line/contributions-outsourced/contribution-outside-services",
    "revenue-line/intergovernmental/state-motor-veh-in-lieu-mvil",
    "revenue-line/taxes/other/real-property-transfer-tax",
    "revenue-line/taxes/other/residential-construction-tax",
    "revenue-line/taxes/property/rpttf-receipts-and-other-proptax",
    "revenue-line/taxes/sales/prop-172-public-sfty-augmnt",
    "revenue-line/use-of-money-and-property/multi-service-center-rentals",
  ],
  // TWO OF THIS COLUMN'S MARKS ARE DERIVED AND NEITHER IS IN THE OTHER'S.
  // gap/expenditure/services-and-supplies is the declared 250,000 shortfall
  // p0067 prints in FY2026-27 and not in FY2025-26, and it is the widest label
  // the tree draws anywhere; residual/fund-group/general carries a different
  // endpoint's words in each column.
  "FY 2026-27": [
    "department/innovation-and-economic-development",
    "dept/administrative-services", "dept/community-development-admin",
    "dept/innovation-and-economic-devel", "dept/public-works-administration",
    "fund/280", "fund/282", "fund/283", "fund/320", "fund/513", "fund/551",
    "fund/552", "fund/623", "fund/730",
    "gap/expenditure/services-and-supplies", "residual/fund-group/general",
    "revenue-line/charges-for-services/administrative-cost-recovery",
    "revenue-line/charges-for-services/engineering-inspection-fees",
    "revenue-line/charges-for-services/fire-plan-check-and-inspct-fee",
    "revenue-line/contributions-outsourced/contribution-outside-services",
    "revenue-line/intergovernmental/state-motor-veh-in-lieu-mvil",
    "revenue-line/taxes/other/real-property-transfer-tax",
    "revenue-line/taxes/other/residential-construction-tax",
    "revenue-line/taxes/property/rpttf-receipts-and-other-proptax",
    "revenue-line/taxes/sales/prop-172-public-sfty-augmnt",
    "revenue-line/use-of-money-and-property/multi-service-center-rentals",
  ],
};

/**
 * Every view the drill tree opens, in each of the page's two years, measured
 * for the two things a label may do to another and the one thing it may do to
 * the chart's edge.
 */
for (const label of Object.keys(OVER_GUTTER)) {
  describe(`${label}: every view the drill opens, measured for what a label does to its neighbours`, () => {
    let app, walk, views, over, overlapped, alike, ids, want;
    before(async () => {
      // THE YEAR IS THE PAGE'S, found by the label this file names, so a year
      // the page no longer ships is a named failure rather than a walk over
      // whichever radio was checked.
      const year = pageFixture().config.years.find((y) => y.label === label);
      if (!year) throw new Error(`the pinned page ships no year labelled ${JSON.stringify(label)}`);
      ({ app } = await bootedApp({ checkedStem: year.stem }));
      over = new Set();
      overlapped = [];
      alike = [];
      views = 0;
      walk = await everyOffer(app, () => {
        views++;
        const fits = labelFit(app, app.layOut(app.projection));
        for (const f of fits) if (f.clearance < 0) over.add(f.id);
        const stack = tightestStack(fits);
        if (stack && stack.gap <= 0) overlapped.push(`${stack.above} over ${stack.below}`);
        const same = sameWords(fits);
        if (same.length) alike.push(same[0]);
      });
      ids = [...over].sort();
      want = OVER_GUTTER[label];
    });

    test("no view stacks one label on another, or draws two marks a reader would read the same", (t) => {
      const detail = `${views} view(s) walked${walk.refused ? `, refused at ${walk.refused}` : ""}; ` +
        `${overlapped.length ? overlapped[0] : "no two labels touch"}; ` +
        `${alike.length ? alike[0] : "no two marks draw the same words"}`;
      t.diagnostic(detail);
      // THE RULE, AND IT HAS NO LITERAL. Zero is the only value either of these
      // may take, on any view, whatever the tree grows -- unlike the gutter
      // below, which states a cost the chart's width makes real.
      assert.deepEqual({ refused: walk.refused, walkedNothing: views === 0, overlapped, alike },
        { refused: "", walkedNothing: false, overlapped: [], alike: [] }, detail);
    });

    test("the marks whose words run past the gutter are the ones this file names, and no others", (t) => {
      const detail = ids.join("|") === want.join("|")
        ? `${ids.length} mark(s) wider than the ${app.LABEL_GUTTER - 10}px they have, across ${views} view(s), each named`
        : `arrived ${JSON.stringify(ids.filter((id) => !want.includes(id)))}, ` +
          `left ${JSON.stringify(want.filter((id) => !ids.includes(id)))}`;
      t.diagnostic(detail);
      assert.deepEqual(ids, want, detail);
    });
  });
}

// The window whose folded tail is drawn out below: the group whose column the
// cap folds hardest.
const WORST_GROUP = "fund-group/special-revenue";

/**
 * The pins the expanded column has to hold.
 *
 * PINNED AND NOT BOUNDED, for this file's reason, and two of these are figures
 * rather than floors. The stack gap is what one line of label has to itself:
 * 12px is what a line claims (ASCENT_PX + DESCENT_PX), so 2.1px is the air
 * over 32 marks, stated so that a rule change halving it is visible here rather
 * than staying green until it crosses. `over` is the other: twelve of the
 * column's labels are wider than the 250px gutter under this file's
 * deliberately pessimistic 0.6em advance, and that is the cost of the gesture
 * stated as a number rather than asserted away. The worst, fund/221, is 55.8px
 * past the gutter, three value-width characters of which are the open marker
 * every fund pp.85-125 print a funding row for draws beside its figure.
 *
 * THE VERTICAL ANSWER IS THE GOOD ONE AND THE HORIZONTAL ANSWER IS NOT. No two
 * labels touch and no two draw the same words; twelve run past the gutter, the
 * worst by 56px of a 306px estimate. WHICH OF THOSE TWELVE ACTUALLY OVERFLOWS IS
 * A BROWSER QUESTION -- ADVANCE_EM is chosen to be wider than any system face
 * sets -- and the walk in fisc-rl4j is where it is settled. fisc-mvrt.
 */
const EXPANDED = { marks: 41, column: 32, stack: "2.1px", over: 12,
  worst: "fund/221", worstBy: "-55.8px" };

describe("a column drawn out to every mark it holds", () => {
  let fits, column, stack, ambiguous, over, worst, expanded;
  before(async () => {
    const { app } = await spineApp();
    await opened(app, WORST_GROUP);
    expanded = expandAll(app);
    await settle();
    fits = labelFit(app, app.layOut(app.projection));
    column = fits.filter((f) => f.col === 2);
    stack = tightestStack(fits);
    ambiguous = sameWords(fits);
    over = fits.filter((f) => f.clearance < 0);
    worst = tightest(fits);
  });

  test("draws no two labels over each other, and states what the gesture costs the gutter", (t) => {
    const detail = `${expanded} column(s) expanded; ${fits.length} label(s) (want ${EXPANDED.marks}), ` +
      `${column.length} of them in the expanded column (want ${EXPANDED.column}); tightest vertically is ` +
      `${stack ? `${stack.above} over ${stack.below} with ${px(stack.gap)}` : "nothing"} ` +
      `(want ${EXPANDED.stack}, against the 12px one line of label claims) and ` +
      `${ambiguous.length ? ambiguous[0] : "no two marks draw the same words"}; ` +
      `${over.length} label(s) are wider than the gutter (want ${EXPANDED.over}, all in the ` +
      `expanded column), worst ${worst.id} at ${px(worst.clearance)} (want ${EXPANDED.worst} ` +
      `at ${EXPANDED.worstBy}) -- measured at this file's 0.6em advance, which is wider than ` +
      `any system face sets`;
    t.diagnostic(detail);
    assert.deepEqual({
      expandedNothing: expanded === 0,
      marks: fits.length, column: column.length, ambiguous, stacked: Boolean(stack) && stack.gap > 0,
      stack: stack && px(stack.gap), over: over.length,
      overOutsideTheColumn: over.filter((f) => f.col !== 2).map((f) => f.id),
      worst: worst.id, worstBy: px(worst.clearance),
    }, {
      expandedNothing: false,
      marks: EXPANDED.marks, column: EXPANDED.column, ambiguous: [], stacked: true,
      stack: EXPANDED.stack, over: EXPANDED.over, overOutsideTheColumn: [],
      worst: EXPANDED.worst, worstBy: EXPANDED.worstBy,
    }, detail);
  });
});

// The category window is the one chart on the site whose mark is SHORTER than
// the ribbons arriving at it, and the band is what makes that legible rather
// than broken. Measured over the committed FY 2025-26 capture at this file's
// one pinned width.
const CONTRA_WINDOW = "revenue/taxes/property";
const CONTRA_BAND = { box: 459.35, arriving: 684.00, excess: 224.65 };

describe("a mark drawn at the figure the city publishes, and the band its ribbons hang into", () => {
  let app, document, centre;
  before(async () => {
    ({ app, document } = await spineApp());
    await opened(app, CONTRA_WINDOW);
    centre = app.layOut(app.projection).nodes.find((n) => n.id === CONTRA_WINDOW);
    assert.ok(centre, `the window draws no ${CONTRA_WINDOW}; this suite cannot pose its question`);
  });

  test("the box is the published figure and the arriving stack exceeds it by twice the reductions", (t) => {
    const box = centre.y1 - centre.y0;
    const arriving = centre.targetLinks.reduce((sum, l) => sum + l.width, 0);
    const reductions = centre.targetLinks
      .filter((l) => l.contra)
      .reduce((sum, l) => sum + l.width, 0);

    // THE BOX IS value x ky AND NOTHING ELSE, which is what says the mark is
    // drawn at the figure rather than at what the layout made of its ribbons.
    const ky = centre.targetLinks[0].width / centre.targetLinks[0].value;
    assert.ok(Math.abs(box - centre.value * ky) < 1e-9,
      `box ${box} is not value x ky (${centre.value * ky})`);

    // AND THE EXCESS IS EXACTLY TWICE THE REDUCTIONS, because each is drawn
    // forward at its magnitude where the schedule subtracts it. This is the
    // arithmetic that makes the band unavoidable in one direction or the
    // other, and it is asserted rather than described.
    assert.ok(Math.abs((arriving - box) - 2 * reductions) < 1e-9,
      `the stack exceeds the box by ${arriving - box}, want 2 x ${reductions}`);

    assert.equal(Number(box.toFixed(2)), CONTRA_BAND.box);
    assert.equal(Number(arriving.toFixed(2)), CONTRA_BAND.arriving);
    t.diagnostic(`box ${box.toFixed(2)}px at value x ky; arriving stack ${arriving.toFixed(2)}px, ` +
      `exceeding it by ${(arriving - box).toFixed(2)}px = 2 x ${reductions.toFixed(2)}px of reductions`);
  });

  test("the band covers the excess, and covers nothing on a mark that does not hang", (t) => {
    const band = app.contraBand(centre);
    assert.ok(band, "the centre hangs and has no band");
    const box = centre.y1 - centre.y0;
    const arriving = centre.targetLinks.reduce((sum, l) => sum + l.width, 0);

    assert.ok(Math.abs(band.height - (arriving - box)) < 1e-9,
      `band is ${band.height}px over ${arriving - box}px of excess`);
    assert.ok(Math.abs(band.y - centre.y1) < 1e-9, "the band does not start at the mark's foot");
    assert.equal(Number(band.height.toFixed(2)), CONTRA_BAND.excess);

    // EVERY OTHER MARK ON THE SAME CHART HAS NONE. A band on a mark whose
    // ribbons do add up would be ruling off money that cancels nothing.
    const others = app.layOut(app.projection).nodes
      .filter((n) => n.id !== CONTRA_WINDOW)
      .filter((n) => app.contraBand(n));
    assert.deepEqual(others.map((n) => n.id), [],
      "a mark whose ribbons add up was given a band");
    t.diagnostic(`band ${band.height.toFixed(2)}px from y ${band.y.toFixed(2)}, covering the ` +
      `excess exactly; 0 of the other marks on this chart carry one`);
  });

  test("the band's class is one the stylesheet paints, and it is painted unfilled", (t) => {
    // A BAND WHOSE RULE DOES NOT MATCH IS NOT AN INVISIBLE BAND, which is why
    // this is here and not filed as tidiness. An SVG rect with no fill declared
    // paints SOLID BLACK, so a renamed class or a dropped rule puts an opaque
    // block over the reduction ribbons the band exists to reveal -- the worst
    // reading of this chart there is, arrived at by a one-word typo, and
    // nothing in this tree parses CSS (fisc-6at).
    //
    // THE CLASS IS READ OFF THE DRAWN BAND: the centre mark's rect that is
    // displayed and is not the mark's own face.
    const mark = [...document.querySelectorAll("#chart g.node")]
      .find((m) => m.__data__ && m.__data__.id === CONTRA_WINDOW);
    assert.ok(mark, `the chart draws no mark for ${CONTRA_WINDOW}`);
    const bands = [...mark.children].filter((c) => c.tagName === "rect" &&
      c.getAttribute("display") !== "none" && c.getAttribute("pointer-events") === "none");
    assert.equal(bands.length, 1, `the centre mark draws ${bands.length} displayed band rect(s), want 1`);
    const drawn = bands[0].getAttribute("class");
    assert.ok(drawn, "the drawn band carries no class for the stylesheet to paint");
    const css = stylesheet();
    const rule = (css.match(new RegExp(`svg\\.sankey \\.${drawn} \\{([^}]*)\\}`)) || [])[1];
    assert.ok(rule, `site/style.css states no rule for svg.sankey .${drawn}`);
    assert.match(rule, /fill:\s*none/,
      "the band declares no fill:none, so it paints solid black over the ribbons it marks off");
    assert.match(rule, /stroke:\s*var\(--critical\)/,
      "the band is not painted in the hue its ribbons and its mark's value take");
    t.diagnostic(`the page draws the band with class "${drawn}" and style.css paints it ` +
      `${rule.trim().replace(/\s+/g, " ")}`);
  });

  test("the reductions stack last, so they fall inside the band rather than through the mark's face", (t) => {
    const arriving = centre.targetLinks;
    const firstContra = arriving.findIndex((l) => l.contra);
    const lastPlain = arriving.reduce((at, l, i) => (l.contra ? at : i), -1);
    assert.ok(firstContra > lastPlain,
      `a reduction stacks at ${firstContra}, before a printed line at ${lastPlain}`);

    // AND THEY LAND WHERE THE BAND IS. Contra-last is only worth asserting if
    // it puts them in the region the band rules off; the ordering alone would
    // pass on a chart drawn with no band at all.
    const band = app.contraBand(centre);
    const strays = arriving.filter((l) => l.contra)
      .filter((l) => l.y1 - l.width / 2 < band.y - 1e-9);
    assert.deepEqual(strays.map((l) => l.source.id), [], "a reduction starts above the band");
    t.diagnostic(`${arriving.length} arriving ribbons, the ${arriving.length - firstContra} ` +
      `reduction(s) last and all inside the band`);
  });
});

// Nothing in this tree renders or parses CSS, and the one Go assertion about
// the stylesheet matches selector strings and never reads a colour -- which is
// how var(--ink-1) could sit on every focus indicator as #ffffff in both dark
// blocks while every gate stayed green. fisc-6at names the cheap partial:
// extract rule blocks by their selector string and compare one property.
describe("the focus indicator is painted as focus and not as text", () => {
  let css;
  // COMMENTS ARE STRIPPED FIRST: the ribbon's own rule EXPLAINS why it avoids
  // stroke-dasharray, so a test asserting the rule does not mention it fails
  // on the prose saying so. A property check that a comment can satisfy -- or
  // break -- is checking the wrong text.
  before(() => { css = stylesheet().replace(/\/\*[\s\S]*?\*\//g, ""); });

  /** The declarations of one rule, by exact selector. */
  const ruleFor = (selector) => {
    const at = css.indexOf(selector + " {");
    assert.ok(at >= 0, `site/style.css states no rule for "${selector}"`);
    return css.slice(at + selector.length + 2, css.indexOf("}", at));
  };

  test("every focus indicator takes --focus, and no palette makes that the text colour", (t) => {
    const indicators = [
      ":focus-visible",
      "svg.sankey .node:focus-visible rect",
      ".year-toggle input:focus-visible + label",
    ];
    for (const sel of indicators) {
      const rule = ruleFor(sel);
      assert.match(rule, /var\(--focus\)/, `${sel} does not take --focus`);
      assert.doesNotMatch(rule, /var\(--ink-1\)/,
        `${sel} still paints itself with the text colour, which is #ffffff in dark`);
    }

    // AND THE TOKEN IS DECLARED WHEREVER --ink-1 IS, or a palette falls back to
    // an unset custom property and the indicator disappears entirely -- worse
    // than the white box, and invisible to a check that only reads the rules.
    const palettes = (css.match(/--ink-1:\s*#[0-9a-f]{6}/gi) || []).length;
    const focuses = (css.match(/--focus:\s*#[0-9a-f]{6}/gi) || []).length;
    assert.equal(focuses, palettes,
      `--ink-1 is declared in ${palettes} palette(s) and --focus in ${focuses}`);

    // THE VALUE IS READ, WHICH IS THE WHOLE POINT. A --focus that resolved to
    // #ffffff would satisfy every line above and be the reported defect exactly.
    const values = (css.match(/--focus:\s*(#[0-9a-f]{6})/gi) || [])
      .map((m) => m.split(":")[1].trim().toLowerCase());
    assert.deepEqual(values.filter((v) => v === "#ffffff"), [],
      "a palette paints focus pure white, which is the defect this test exists for");
    t.diagnostic(`${indicators.length} indicator(s) take --focus; ${focuses} palette(s) ` +
      `declare it as ${[...new Set(values)].join(", ")}`);
  });

  test("a ribbon gets a drawn echo and not a box around its bounding rectangle", (t) => {
    const rule = ruleFor("svg.sankey .link:focus-visible");
    // A <path>'s bounding box is the whole bezier's rectangle, so ANY outline
    // on a ribbon is a box around mostly empty space. Dropping the offset was
    // not enough and is what shipped.
    assert.match(rule, /outline:\s*none/,
      "a focused ribbon still takes the global outline, whose box is its whole bezier");
    assert.match(rule, /drop-shadow\([^)]*var\(--focus\)/,
      "a focused ribbon has no drawn echo, so focus on it is invisible");
    // AND NOT THROUGH A CHANNEL THE CHART IS ALREADY USING: stroke is the
    // ribbon's own value and stroke-dasharray carries derived and partition.
    assert.doesNotMatch(rule, /stroke-dasharray/,
      "the echo reuses the dash, which already distinguishes derived from partition");
    assert.doesNotMatch(rule, /stroke-width/,
      "the echo reuses stroke-width, which IS the ribbon's value");
    t.diagnostic(`the ribbon's focus rule is ${rule.trim().replace(/\s+/g, " ")}`);
  });
});
