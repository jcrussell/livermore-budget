// layout.mjs — the checks for site/app.js's layout claims.
//
// app.js asserts, in prose, numbers that only a layout can produce: that
// nodeRank plus restackLinks() bring this chart to 195 ribbon crossings and
// $457M of overlapping ribbon, and that restackLinks removes 14 stale-stacked
// pairs on FY2026. Until this file existed nothing in the tree could confirm any
// of it — the numbers were measured once, by hand, out of tree, and quoted
// forever after (fisc-gxa.7).
//
// They are checked against testdata/sankey.golden.json, which is FY2026 — the
// year the comments are about — and is committed, so a number here moves only
// when the graph does.
//
// WHAT CANNOT BE CHECKED, AND IS NOT PRETENDED OTHERWISE. app.js used to quote a
// BEFORE for each figure — "394 ribbon crossings and $1,372M ... become 195 and
// $457M", and "108 under the order this file used to sort by". Those baselines
// describe A SORT ORDER THAT IS NO LONGER IN THE TREE, so no harness can
// reproduce them: the code that produced them was replaced by the code they
// justify. Every ordering that IS still reachable is measured below and pinned,
// and none of them is 394 on either side of the restack. The before-figures in
// app.js were corrected to what reproduces; this file is why that was possible.
//
// EVERY NUMBER THIS FILE PINS IS MEASURED POST-RESTACK, because that is the
// chart a reader sees. Pre-restack figures appear only where a check is about
// what restackLinks itself does.

import { loadApp, goldenGraph, spineConfig, steppedSpineConfig, stylesheet,
  plannedFetch, settle } from "./harness.mjs";
import { openedWindow, openedWide, openedExpanded, openedAsShipped,
  everyOpenedView, COLUMNS } from "./drill.mjs";

/**
 * Lays the golden graph out exactly as render() does, under a given node sort
 * and a given aligner.
 *
 * The constants come out of app.js rather than being repeated here: a chart laid
 * out at a different width has different crossings, so a harness carrying its
 * own copy would drift into checking a chart the page does not draw.
 *
 * THE ALIGNER COMES OFF THE PAGE FOR THAT SAME REASON, and it is the one
 * constant here that used to be hard-coded. app.js hands a graph to d3's
 * sankeyJustify only while no tier set is declared and aligns on the declared
 * order otherwise (alignFor), and index.html declares one
 * (pkg/cmd/export/data.go). A literal `.nodeAlign(sankeyJustify)` would measure
 * an aligner the page does not use and would be green because the two happen to
 * agree -- which is the copy-checks-the-copy shape this file exists to refuse.
 * The default is the page's; a caller passes one only to compare the two, which
 * is what makes that agreement a measurement rather than an assumption.
 */
function layout(app, nodeSort, align) {
  const doc = goldenGraph();
  const sankey = app.d3.sankey()
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
 * confident, wrong number. The contract has six tiers and the detail schedules
 * are not drawn yet, so this is a real future shape and not a hypothetical.
 */
function bands(graph) {
  const out = new Map();
  for (const l of graph.links) {
    const span = l.target.depth - l.source.depth;
    if (span !== 1) {
      throw new Error(
        `link ${l.source.id} -> ${l.target.id} spans ${span} columns; this ` +
        `harness counts crossings band by band and would undercount it`);
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

// The figures app.js publishes about itself, and the alternatives it publishes
// them against. Every one is PINNED, not bounded: a `>` comparison would let
// the alternatives drift while the check stayed green, which is the exact
// defect this harness exists to remove.
//
// They are also checked against app.js's OWN TEXT below, so editing the comment
// to say something else fails here rather than passing quietly. That is the
// pattern internal/export/export_test.go:623 already uses to pin the client's
// SCHEMA_VERSION literal to the producer's.
//
// THE VERSION GATE ITSELF IS NOT CHECKED HERE ANY MORE. It was, through
// understands(), which took a number and answered a boolean; the gate is now
// one comparison inside main() and is reached the way a reader reaches it, in
// lifecycle.mjs.
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
// no harness can know the true advance -- 0.6em is wider than mixed-case
// English sets in any of the usual system faces, so a label this says fits fits
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

/** Every label of a laid graph, boxed where app.js puts it, against its room. */
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
    const key = f.col + "\u0000" + f.words;
    seen.set(key, (seen.get(key) || []).concat([f.id]));
  }
  return [...seen.entries()]
    .filter((e) => e[1].length > 1)
    .map((e) => `column ${e[0].split("\u0000")[0]} draws ${e[1].length} marks ` +
                `reading "${e[0].split("\u0000")[1]}" (${e[1].join(", ")})`);
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
 * A DECLARATION, NOT A RENDERING, and the comment on TestTheStylesheetHasOneTextMeasure
 * is the same warning: nothing here parses CSS, so this reads .chart-wrap's
 * allowance as text and can say only what style.css says.
 *
 * THE CAP IS NO LONGER A NUMBER IN THE STYLESHEET, so what this reads is which
 * PROPERTY the cap comes through, and the px comes from the value app.js hands
 * that property. The pair is still two files that nothing else compares -- the
 * difference is that a disagreement is now a wiring mistake rather than a
 * number somebody forgot to raise in one of four places.
 *
 * IT PINS THE SHAPE OF THE DECLARATION AND NOT ONLY ITS NUMBER. An equivalent
 * rule written with clamp() would go red here; that is the cost of reading a
 * stylesheet with a regex, and the direction it fails in is the safe one.
 *
 * @param {string} css
 * @param {number | null} handed the px app.js set --chart-max to, or null
 */
function chartAllowance(css, handed) {
  const rule = /\.chart-wrap\s*\{([^}]*)\}/.exec(css);
  if (!rule) return { px: null, why: "style.css declares no .chart-wrap rule" };
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
  // the second operand is now a var() too. Both are in this rule, so a cushion
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
    // breakpoint picked for looking like one puts the reader back where
    // fisc-5e2b found them, one column narrower.
    cushion: cushion ? Number(cushion[1]) : null,
    why: "",
  };
}

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

export async function checks() {
  // THE PAGE index.html SHIPS, NOT loadApp'S BARE DEFAULT: the config carries
  // the column order data.go declares, which is what alignFor reads.
  //
  // AND IT DRAWS BEFORE ANYTHING IS MEASURED. nodeRank orders the fund column
  // by the place the COLUMN gives each group, so an app that laid a document
  // out without fetching one is an app ordering by a list it was never served
  // -- every group ties and the column degenerates to size-descending, which
  // is one of the alternatives measured below and would be reported as
  // nodeRank's own number.
  const app = loadApp({ config: spineConfig(), checkedStem: "sankey",
    fetch: plannedFetch({ "data/sankey.json": { doc: goldenGraph() } }) });
  await settle();
  const byRank = (a, b) => app.nodeRank(a) - app.nodeRank(b) || b.value - a.value;

  const graph = layout(app, byRank);
  const justified = layout(app, byRank, app.d3.sankeyJustify);
  app.restackLinks(justified);
  const before = { tangle: tangle(graph), stale: staleStacked(graph) };
  const widths = graph.links.map((l) => l.width);
  app.restackLinks(graph);
  const after = { tangle: tangle(graph), stale: staleStacked(graph) };

  const measured = Object.entries(ALTERNATIVES).map(([label, want]) => {
    const g = layout(app, want.sort);
    app.restackLinks(g);
    const t = tangle(g);
    return { label, want, got: t };
  });

  return [
    {
      name: "the chart draws at the 195 crossings app.js claims",
      ok: after.tangle.crossings === CLAIMED.crossings,
      detail: `${after.tangle.crossings} crossings under nodeRank + restackLinks ` +
              `(app.js claims ${CLAIMED.crossings})`,
    },
    {
      name: "the overlapping ribbon is the $457,434,169 app.js claims",
      ok: Math.round(after.tangle.dollars) === CLAIMED.overlapDollars,
      detail: `${usd(after.tangle.dollars)} overlapped (app.js claims ${usd(CLAIMED.overlapDollars)})`,
    },
    {
      name: "restackLinks finds the 14 stale-stacked pairs app.js claims",
      ok: before.stale === CLAIMED.stalePairs,
      detail: `${before.stale} pairs stacked in an order their neighbours no longer ` +
              `sit in (app.js claims ${CLAIMED.stalePairs})`,
    },
    {
      // NOT "after.stale === 0", which is a tautology: staleStacked re-derives
      // the same comparator restackLinks just sorted by, so it cannot fail.
      // What is independent is the CROSSINGS those pairs cost, counted by a
      // different function against ribbon endpoints rather than node order.
      name: "removing those 14 pairs removes exactly 14 crossings",
      ok: before.tangle.crossings - after.tangle.crossings === CLAIMED.stalePairs,
      detail: `${before.tangle.crossings} crossings before restacking, ` +
              `${after.tangle.crossings} after: ` +
              `${before.tangle.crossings - after.tangle.crossings} removed`,
    },
    {
      name: "every ordering still in the tree measures where app.js says it does",
      ok: measured.every((m) => m.got.crossings === m.want.crossings &&
                                Math.round(m.got.dollars) === m.want.dollars),
      detail: measured.map((m) => `${m.label} ${m.got.crossings}/${usd(m.got.dollars)}`).join(", ") +
              `, nodeRank ${after.tangle.crossings}/${usd(after.tangle.dollars)}`,
    },
    {
      name: "nodeRank beats every one of them",
      ok: measured.every((m) => m.got.crossings > after.tangle.crossings &&
                                m.got.dollars > after.tangle.dollars),
      detail: `best alternative ${Math.min(...measured.map((m) => m.got.crossings))} crossings, ` +
              `nodeRank ${after.tangle.crossings}`,
    },
    {
      // Matched as WHOLE QUOTED PHRASES, not as bare substrings. A bare
      // `source.includes("14")` matches `const NODE_WIDTH = 14;` and a bare
      // `includes("195")` is satisfied by any one of the several places 195
      // appears, so both let the sentence they were meant to pin drift to a
      // wrong number while staying green. Each phrase below is the figure
      // together with enough of its own sentence to be unique.
      name: "app.js quotes the figures it actually produces",
      ok: (() => {
        const phrases = [
          `comes to ${CLAIMED.crossings} ribbon`,
          `$${CLAIMED.overlapDollars.toLocaleString("en-US")} of overlapping ribbon`,
          `down to ${CLAIMED.crossings} ribbon crossings`,
          `${CLAIMED.stalePairs} of them on FY2026`,
          `${ALTERNATIVES["size descending"].crossings} and`,
          `${ALTERNATIVES["input order"].crossings} under the input order`,
          `${ALTERNATIVES["d3's own pass"].crossings} under`,
          `$${ALTERNATIVES["size descending"].dollars.toLocaleString("en-US")} under a sort by size`,
        ];
        const missing = phrases.filter((q) => !app.source.includes(q));
        return missing.length === 0;
      })(),
      detail: "every figure pinned here appears in site/app.js's own comments in " +
              "its own sentence, so editing one without re-measuring fails",
    },
    {
      // THE MEASUREMENT THAT LET THE SPINE DECLARE A COLUMN ORDER AT ALL, kept
      // in the tree instead of in a commit message. Every figure above is
      // measured under the declared order; all of them were measured under
      // sankeyJustify before there was one, and the reason they did not move is
      // structural rather than lucky -- tier 0 is pure source, tier 5 pure
      // sink, and justify's own rule puts a link-less sink in the last column,
      // which is where indexOf puts tier 5. This says so where it can go red:
      // the day the spine grows a tier, or declares its columns in an order
      // topology disagrees with, this fails and the figures above are a new
      // measurement rather than the old one.
      name: "the spine lays out identically under its declared column order and under d3's justify",
      ok: app.RENDER_TIERS.length > 0 && samePlaces(graph, justified),
      detail: app.RENDER_TIERS.length
        ? `every node and ribbon in the same place under nodeAlign indexOf ` +
          `[${app.RENDER_TIERS.join(", ")}] and under sankeyJustify`
        : "the config this ran under declares no column order, so nothing here " +
          "measures the aligner the page uses",
    },
    {
      name: "the fund column is pinned to the order the column shipped",
      ok: (() => {
        const drawn = graph.nodes.filter(app.isFundGroup)
          .sort((a, b) => a.y0 - b.y0).map((n) => n.id);
        // THE SERVED LIST FILTERED BY WHAT IS DRAWN, and the filter is what
        // makes this an equality rather than a subset test: every group the
        // chart lays out has to appear, in the served order, with nothing
        // between them. A group the packager's sequence does not name is at
        // the END of that list, so this fails if the page draws it anywhere
        // else -- which is the direction an indexOf that answers -1 breaks in.
        const served = app.fundGroups().map((g) => g.id);
        return served.length > 0 && JSON.stringify(drawn) ===
               JSON.stringify(served.filter((f) => drawn.includes(f)));
      })(),
      detail: "fund groups run top to bottom in the order internal/export's " +
              "fundGroupsOf shipped, which is what supplying .nodeSort() at all is for",
    },
    {
      name: "restackLinks moves ribbons without resizing them",
      ok: graph.links.every((l, i) => l.width === widths[i]),
      detail: "every ribbon keeps the width the layout gave it",
    },
    {
      name: "no ribbon overflows the node face it meets",
      ok: extentOverflow(graph) <= 1e-9,
      detail: `worst overflow ${extentOverflow(graph).toFixed(6)}px`,
    },
  ].concat(await labelChecks(app, graph)).concat(await wideChecks(app))
    .concat(await expandedChecks()).concat(await everyViewChecks());
}

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
 * nothing, and this file's own header is about a check that was green because
 * it measured a helper rather than its use. So the sizes are summed against the
 * graph's own link count and the columns against its own deepest node.
 */
async function wideChecks(app) {
  const wide = await openedWide(4, WIDE_PATH);
  const laid = wide.layOut(wide.projection);
  const columns = Math.max(...laid.nodes.map((n) => n.depth)) + 1;
  let keys = "";
  let sizes = "";
  let counted = 0;
  let threw = "";
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
  const fits = labelFit(wide, laid);
  // AND THE WINDOW ONE RUNG FURTHER IN, which is where the other half of the
  // labelling decision is measured: a division's own window draws that
  // division's two cells side by side, so labelling a cell by its division --
  // the short label that would have fitted the fourth column -- draws "Patrol"
  // twice. Both windows have to come out unambiguous or neither rule is right.
  const inner = await openedWide(4, WIDE_PATH.concat([DIVISION_WINDOW]));
  const innerFits = labelFit(inner, inner.layOut(inner.projection));
  const ambiguous = sameWords(fits).concat(sameWords(innerFits));
  const stack = tightestStack(fits);
  // THE CEILING IS ASKED OF A PAGE THAT DECLARES THE SITE'S STEPS, because it
  // is derived from them: the `app` these arms are otherwise measured on
  // carries no steps, so its ceiling is the floor and every claim below would
  // be about a three-column page. fisc-clbw is why that config is not simply
  // fixed in place.
  const offered = loadApp({ config: steppedSpineConfig() });
  // THE PX IS app.js's OWN CONSTANT, because the stylesheet no longer carries
  // one: wireColumns hands --chart-max the value of CHART_MAX, and what this
  // reads off the stylesheet is that the cap comes through that property at all.
  // That the hand-off actually happens at boot is lifecycle.mjs's arm, driven
  // through main() where this file never runs one.
  const allowance = chartAllowance(stylesheet(), offered.CHART_MAX);
  // What each responsive threshold actually buys the chart, at the threshold.
  // Math.min with the cap because a query above it is bounded by the cap, which
  // is the arm above's subject rather than this one's.
  const queryRoom = (offered.COLUMN_QUERIES || []).map((/** @type {any} */ q) => {
    const at = Number(/\(min-width:\s*(\d+)px\)/.exec(q.query)[1]);
    return {
      query: q.query,
      columns: q.columns,
      room: Math.min(allowance.px, at - allowance.cushion),
      wants: offered.chartWidth(q.columns),
    };
  });
  return [
    {
      // THE GUTTER DECISION, MEASURED. LABEL_GUTTER does not grow with the
      // column count because only the two end columns anchor outward; the two
      // interior ones are centred over their own rects, in the NODE_PADDING gap
      // above them. This is what says a fourth column did not take a gutter
      // with it.
      name: "every label in the four-column window has room where it was anchored",
      ok: fits.length > 0 && fits.every((f) => f.clearance >= 0) &&
          new Set(fits.map((f) => f.col)).size === 4,
      detail: fits.length === 0
        ? "the window drew no nodes, so nothing here measured a label at all"
        : `${fits.length} labels over ${new Set(fits.map((f) => f.col)).size} columns; ` +
          `tightest ${tightest(fits).id} anchored ${tightest(fits).place.anchor} ` +
          `with ${px(tightest(fits).clearance)} to spare`,
    },
    {
      name: "a four-column window lays out in three bands, each between adjacent columns",
      ok: threw === "" && columns === 4 && keys === WIDE_BANDS.keys &&
          sizes === WIDE_BANDS.sizes && counted === laid.links.length && counted > 0,
      detail: threw !== ""
        ? `bands() refused the four-column window: ${threw}`
        : `${columns} columns and ${keys.split(" ").length} band(s) [${keys}] holding ` +
          `${sizes} of the chart's ${laid.links.length} ribbons (want ${WIDE_BANDS.keys}, ` +
          `${WIDE_BANDS.sizes}); every one of them spans exactly one column, which is ` +
          `what bands() throws on`,
    },
    {
      name: "the four-column window is laid out at its own width, not at the three-column one",
      ok: app.chartWidth(3) === NARROW_DESIGN_WIDTH &&
          app.chartWidth(columns) > NARROW_DESIGN_WIDTH &&
          Math.max(...laid.nodes.map((n) => n.x1)) === app.chartWidth(columns) - app.LABEL_GUTTER &&
          Math.min(...laid.nodes.map((n) => n.x0)) === app.LABEL_GUTTER &&
          bandWidth(laid) === app.BAND,
      detail: `chartWidth(3) is ${app.chartWidth(3)}px (want ${NARROW_DESIGN_WIDTH}, which is ` +
        `what every figure in this file was measured at) and this window's ${columns} columns ` +
        `lay out at ${app.chartWidth(columns)}px; it draws from ` +
        `${Math.min(...laid.nodes.map((n) => n.x0))}px to ` +
        `${Math.max(...laid.nodes.map((n) => n.x1))}px with ${bandWidth(laid)}px of clear ` +
        `run between columns (want ${app.LABEL_GUTTER}px, ` +
        `${app.chartWidth(columns) - app.LABEL_GUTTER}px and ${app.BAND}px)`,
    },
    {
      // THE DEFECT AS A PROPERTY OF WHAT IS DRAWN. Not "tier 5 carries its
      // division": that is one fix's shape, and two of them were refused by
      // measurement before this one was written. What a reader needs is that no
      // two marks standing in one column say the same thing, which is asked of
      // the words here and is false under either of the two labels the document
      // could have carried on its own.
      name: "no column of the fund window or of a division's own draws two marks a reader would read the same",
      ok: ambiguous.length === 0 && fits.length > 0 && innerFits.length > 0,
      detail: ambiguous.length === 0
        ? `${fits.length} marks over ${new Set(fits.map((f) => f.col)).size} columns and ` +
          `${innerFits.length} over ${new Set(innerFits.map((f) => f.col)).size}, ` +
          `${fits.filter((f) => f.qualifier).length + innerFits.filter((f) => f.qualifier).length} ` +
          `of them qualified, and every one reads differently from its neighbours`
        : ambiguous.join("; "),
    },
    {
      // WHAT THE SECOND LINE COSTS, AND IT IS PAID IN THE ONE DIRECTION THE
      // GUTTER CANNOT HELP WITH. A qualified label is two lines tall where its
      // neighbours are one, and the fourth column stacks nine of them; the
      // check is that the block still misses the block below it. MEASURED, and
      // the figure is the argument for the shape: the tightest pair here clears
      // by more than the tightest pair of SINGLE-line labels in the same
      // window's third column, so the qualifier costs a reader nothing the
      // chart was not already spending.
      name: "a qualifier's second line still clears the label stacked below it",
      ok: Boolean(stack) && stack.gap >= 0,
      detail: stack
        ? `tightest stack in the four-column window is column ${stack.col}, ` +
          `${stack.above} over ${stack.below}, clearing ${px(stack.gap)}`
        : "the window stacked no two labels in one column, so nothing here measured a gap",
    },
    {
      // THE CONTAINER AGAINST THE DRAWING, which is the pair nothing else in
      // the tree compares. The chart is an <svg> with a viewBox, so a container
      // narrower than the width app.js lays the chart out at does not clip it
      // and does not reflow it -- it draws the same picture smaller, and a
      // reader who asks for a fourth column gets a fifth less chart (fisc-5e2b).
      // AND IT IS AN EQUALITY NOW, NOT A FLOOR. While the cap was a literal in
      // the stylesheet, `>=` was all this could ask: a cap wider than the
      // ceiling was slack rather than a defect. The cap is derived from the
      // same declarations the ceiling is, so slack IS the defect -- it means
      // one of them was computed from something else.
      //
      // OFFERED_COLUMNS IS READ RATHER THAN SPELLED, for the reason the literal
      // 4 was refused here before: asking about a number would leave the
      // ceiling moving without this arm noticing, which is the whole failure
      // this arm is for.
      name: "the stylesheet lets the widest chart app.js will draw draw at the width it lays it out at",
      ok: allowance.px !== null && allowance.px === offered.chartWidth(offered.OFFERED_COLUMNS) &&
          allowance.cushion === offered.CHART_CUSHION,
      detail: allowance.px === null
        ? allowance.why
        : `style.css caps the chart through --chart-max, which app.js sets to ${allowance.px}px, ` +
          `and app.js lays its offered ${offered.OFFERED_COLUMNS} columns out at ` +
          `${offered.chartWidth(offered.OFFERED_COLUMNS)}px` +
          (allowance.px === offered.chartWidth(offered.OFFERED_COLUMNS)
            ? ""
            : ` -- the two disagree, so one of them is not chartWidth(OFFERED_COLUMNS)`) +
          `; the cushion is ${allowance.cushion}px in the stylesheet against app.js's ` +
          `${offered.CHART_CUSHION}px` +
          (allowance.cushion === offered.CHART_CUSHION ? "" : " -- SKEWED"),
    },
    {
      // THE BREAKPOINTS AGAINST THE SAME DECLARATION, WHICH IS THE OTHER WAY
      // THE PAIR CAN DISAGREE. The arm above says the CAP is wide enough for
      // the ceiling; this one says every viewport at which the page ADDS a
      // column has room for that column at the width it will be laid out at.
      // A query threshold of 1500px passes the arm above untouched and still
      // hands a reader a four-column chart drawn at 95% of itself, because
      // below the cap the room is 100vw minus the cushion. Re-derived from the
      // two shipped files rather than pinned as a number here.
      //
      // EVERY ENTRY, not the widest: a list is the shape of the responsive
      // rule, and the list is now composed rather than typed, so this arm is
      // what says the composition is right at every column it offers.
      name: "every viewport the page adds a column at has room for that column",
      ok: allowance.px !== null && allowance.cushion !== null &&
          queryRoom.length > 0 && queryRoom.every((q) => q.room >= q.wants),
      detail: allowance.cushion === null
        ? `.chart-wrap's --chart-room declares no calc(100vw - Npx) arm, so there is nothing to ` +
          `derive a breakpoint from: ${allowance.why || "the cap parsed but the cushion did not"}`
        : queryRoom.map((q) => `${q.query} buys ${q.columns} columns: at that width the ` +
            `stylesheet gives the chart ${q.room}px and app.js lays ${q.columns} out at ` +
            `${q.wants}px${q.room >= q.wants ? "" : " -- SHORT"}`).join("; ") +
          ` (cushion ${allowance.cushion}px, read off the same declaration)`,
    },
  ];
}

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

// What the spine's middle column escapes by being centred: anchored outward
// from its rects instead, 4 of its 6 labels reach past the midline into the
// half of the band belonging to the column they run at. PINNED, not bounded,
// because the interesting direction is DOWN -- a chart whose middle labels all
// fitted outward would make the centring unnecessary, and a `>= 1` would go on
// passing while that was true.
const MIDDLE_OUTWARD = { crowded: 4, of: 6 };

// The four-column chart the band and geometry arms run over: the fund window
// one rung inside the General Fund group, which pkg/cmd/export/data.go widens
// by tier 5. Its bands are [the group into the fund | the fund into its 23
// divisions | those divisions into the object cells the tier-5 cap leaves],
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
// the stylesheet's cap are all that same function now. The arm below asks what
// a window was laid out at against chartWidth(its own column count), which is
// the claim wanted and holds at any count.
const NARROW_DESIGN_WIDTH = 1180;

// The label selection itself, pinned as whole lines.
//
// THE ARMS BELOW MEASURE labelPlacement AND NOT WHAT render() DOES WITH IT. The
// stub answers no "#chart" selector, so no check here can read an attribute
// back off a drawn <text>; the rule and its use are two claims and only one of
// them is reachable. The mutation says how much that matters: with these lines
// reverted to `d.depth === 0` and labelPlacement left untouched in the file,
// every other arm here stayed green. This is the one that goes red.
//
// Whole lines, the way the figure phrases below are whole sentences: a
// substring of one of these matches the comment that argues for it.
const LABEL_SELECTION = [
  "const lastColumn = Math.max(...graph.nodes.map(columnOf));",
  '.attr("y", /** @param {LaidNode} d */ (d) => labelPlacement(d, lastColumn).y)',
  '.attr("dy", /** @param {LaidNode} d */ (d) => labelPlacement(d, lastColumn).dy)',
  '.attr("x", /** @param {LaidNode} d */ (d) => labelPlacement(d, lastColumn).x)',
  '.attr("text-anchor", /** @param {LaidNode} d */ (d) => labelPlacement(d, lastColumn).anchor)',
];

/**
 * Where every label lands, on the spine and inside a window.
 *
 * WHY A WINDOW AND NOT THE SPINE ALONE. Every path through the spine is the
 * same length, so d3's depth and the column the view declares agree on all 25
 * of its nodes and a rule keyed on either reads the same. A check written there
 * cannot tell the two apart and would be green because the gate fired rather
 * than because the rule is right. The windows are where they come apart.
 */
async function labelChecks(app, spine) {
  const object = await openedWindow(OBJECT_WINDOW);
  const group = await openedWindow(FUND_GROUP_WINDOW);
  const inWindow = labelFit(object, object.layOut(object.projection));
  const onSpine = labelFit(app, spine);
  const middle = onSpine.filter((f) => f.col > 0 && f.col < f.last);
  const gap = outwardGap(onSpine);
  const crowded = middle.filter((f) => outward(f, gap).right > f.room.to);
  const groupLaid = group.layOut(group.projection);
  const disagree = groupLaid.nodes.filter((n) => group.columnOf(n) !== n.depth).map((n) => n.id);

  return [
    {
      // THE MUTATION THIS ARM IS FOR: restore `d.depth === 0 ? ... : ...` on
      // the label selection and the window's middle column is anchored outward
      // again, past the midline, and this goes red naming the node.
      name: "every label in the object-category window is anchored on a side it has room on",
      ok: inWindow.length > 0 && inWindow.every((f) => f.clearance >= 0) &&
          new Set(inWindow.map((f) => f.col)).size === 3,
      detail: inWindow.length === 0
        ? "the window drew no nodes, so nothing here measured a label at all"
        : `${inWindow.length} labels over ${new Set(inWindow.map((f) => f.col)).size} columns; ` +
          `tightest ${tightest(inWindow).id} anchored ${tightest(inWindow).place.anchor} ` +
          `with ${px(tightest(inWindow).clearance)} to spare`,
    },
    {
      // The same shape as "nodeRank beats every one of them": the rule is
      // asserted AND the alternative it was chosen over is measured, so the day
      // the alternative starts fitting this says so instead of staying quiet.
      name: "the spine's middle column is centred because anchoring it outward does not fit",
      ok: middle.length === MIDDLE_OUTWARD.of &&
          middle.every((f) => f.clearance >= 0 && f.place.anchor === "middle") &&
          crowded.length === MIDDLE_OUTWARD.crowded,
      detail: `${middle.length} middle labels anchored ` +
              `${[...new Set(middle.map((f) => f.place.anchor))].join("/")}, tightest ` +
              `${px(tightest(middle).clearance)} clear; anchored outward instead, ` +
              `${crowded.length} of ${middle.length} cross into the next column's half of the ` +
              `band (app.js's rule is chosen against ${MIDDLE_OUTWARD.crowded} of ` +
              `${MIDDLE_OUTWARD.of})`,
    },
    {
      name: "the label selection draws with the rule these arms measure",
      ok: LABEL_SELECTION.every((q) => app.source.includes(q)),
      detail: (() => {
        const missing = LABEL_SELECTION.filter((q) => !app.source.includes(q));
        return missing.length === 0
          ? `all ${LABEL_SELECTION.length} lines of render()'s label selection read ` +
            `labelPlacement, so the placement measured above is the placement drawn`
          : `render()'s label selection no longer reads labelPlacement: ${missing.length} of ` +
            `${LABEL_SELECTION.length} lines are gone, starting "${missing[0]}"`;
      })(),
    },
    {
      name: "a label keys on the column the view declares, not on d3's longest path",
      ok: JSON.stringify(disagree) === JSON.stringify(DEPTH_DISAGREES),
      detail: `of the fund-group window's ${groupLaid.nodes.length} nodes, ` +
              `${disagree.length} sit${disagree.length === 1 ? "s" : ""} in a column ` +
              `d3's depth does not name: ` +
              (disagree.join(", ") || "none, so this chart cannot tell the two rules apart"),
    },
  ];
}

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
 * stated as a number rather than asserted away.
 *
 * `over` WENT FROM FIVE TO TWELVE WHEN THE FUNDS BECAME OPENABLE, and the cause
 * is in labelWidth rather than in the layout: it measures the words a reader
 * sees, nodeFlags included, and every fund pp.85-125 print a funding row for now
 * draws the open marker beside its figure. Measured on this column, worst first:
 * fund/221 goes from 36.0px past the gutter to 55.8px, which is the marker's
 * three value-width characters and nothing else. The vertical answer did not
 * move at all -- the stack is still 2.1px over 32 marks -- because a marker
 * widens a label and does not add a line.
 */
const EXPANDED = { marks: 41, column: 32, stack: "2.1px", over: 12,
  worst: "fund/221", worstBy: "-55.8px" };

/**
 * What a reader gets when they draw a folded column out: the shape the cap
 * exists to prevent, now reachable on purpose.
 *
 * THE CAP'S OWN EVIDENCE SAYS THIS IS A BAD CHART -- 42 ribbons of which 9 lay
 * out under a pixel -- and the reader has asked for it anyway, which is the
 * whole of the feature. What this measures is what the words do, because a
 * denser chart whose labels overlap is not denser, it is unreadable, and while
 * the shape could be reached only by editing a step nothing measured it at all.
 *
 * THE VERTICAL ANSWER IS THE GOOD ONE AND THE HORIZONTAL ANSWER IS NOT. No two
 * labels touch and no two draw the same words; twelve run past the gutter, the
 * worst by 56px of a 306px estimate. WHICH OF THOSE TWELVE ACTUALLY OVERFLOWS IS
 * A BROWSER QUESTION -- ADVANCE_EM is chosen to be wider than any system face
 * sets, so the direction this can be wrong in is calling a label too wide that
 * fits -- and the walk in fisc-rl4j is where it is settled. fisc-mvrt.
 */
/**
 * The marks whose words run past the room they have, over every view the drill
 * opens -- named, and not counted.
 *
 * WHICH VIEWS ARE MEASURED IS NOT A CHOICE THIS FILE MAKES. The three arms
 * above it measure windows a caller picked, and a step added anywhere leaves
 * them measuring the shape somebody chose last time; drill.mjs's
 * everyOpenedView re-opens from the overview at every rung and reads its
 * children off drillable on the DRAWN chart, so a column this file never heard
 * of arrives on its own. The view count is that walker's own pin and is not
 * restated here.
 *
 * OVER THE DOCUMENTS' OWN WORDS. Every other builder in drill.mjs relabels
 * three families of spine nodes so an arm can tell which document a label was
 * read from, and a label check run on those would measure words no reader is
 * shown -- four of them a whole " category" wider than the page draws.
 *
 * THE SET AND NOT ITS SIZE. A count is what let this file's expanded arm go on
 * saying five while its own pin said twelve: nothing in a number says which
 * mark joined or left. Each id below is a mark whose label is wider than the
 * 240px it has at ADVANCE_EM, which is deliberately wider than any system face
 * sets -- so this is an upper bound on a real defect rather than a count of it,
 * and fisc-rl4j's browser walk is what settles which of them a rendered face
 * actually overflows.
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
 * Every view the drill tree opens, measured for the two things a label may do
 * to another and the one thing it may do to the chart's edge.
 */
async function everyViewChecks() {
  const out = [];
  for (const col of COLUMNS) {
    const app = await openedAsShipped([], col);
    const over = new Set();
    const overlapped = [];
    const alike = [];
    let views = 0;
    const walk = await everyOpenedView(app, () => {
      views++;
      const fits = labelFit(app, app.layOut(app.projection));
      for (const f of fits) if (f.clearance < 0) over.add(f.id);
      const stack = tightestStack(fits);
      if (stack && stack.gap <= 0) overlapped.push(`${stack.above} over ${stack.below}`);
      const same = sameWords(fits);
      if (same.length) alike.push(same[0]);
    });
    const ids = [...over].sort();
    const want = OVER_GUTTER[col.label];
    out.push({
      name: `${col.label}: no view the drill opens stacks one label on another, or draws two marks a reader would read the same`,
      // THE RULE, AND IT HAS NO LITERAL. Zero is the only value either of these
      // may take, on any view, whatever the tree grows -- unlike the gutter
      // below, which states a cost the chart's width makes real.
      ok: walk.refused === "" && views === col.openedViews &&
          overlapped.length === 0 && alike.length === 0,
      detail: `${views} view(s) walked (want ${col.openedViews}${walk.refused ? `, refused at ${walk.refused}` : ""}); ` +
        `${overlapped.length ? overlapped[0] : "no two labels touch"}; ` +
        `${alike.length ? alike[0] : "no two marks draw the same words"}`,
    });
    out.push({
      name: `${col.label}: the marks whose words run past the gutter are the ones this file names, and no others`,
      ok: ids.join("|") === want.join("|"),
      detail: ids.join("|") === want.join("|")
        ? `${ids.length} mark(s) wider than the ${app.LABEL_GUTTER - 10}px they have, across ${views} view(s), each named`
        : `arrived ${JSON.stringify(ids.filter((id) => !want.includes(id)))}, ` +
          `left ${JSON.stringify(want.filter((id) => !ids.includes(id)))}`,
    });
  }
  return out;
}

async function expandedChecks() {
  const app = await openedExpanded();
  const laid = app.layOut(app.projection);
  const fits = labelFit(app, laid);
  const column = fits.filter((f) => f.col === 2);
  const stack = tightestStack(fits);
  const ambiguous = sameWords(fits);
  const over = fits.filter((f) => f.clearance < 0);
  const worst = tightest(fits);
  return [{
    name: "a column drawn out to every mark it holds draws no two labels over each other, and states what the gesture costs the gutter",
    ok: fits.length === EXPANDED.marks && column.length === EXPANDED.column &&
        ambiguous.length === 0 && Boolean(stack) && stack.gap > 0 &&
        px(stack.gap) === EXPANDED.stack && over.length === EXPANDED.over &&
        over.every((f) => f.col === 2) &&
        worst.id === EXPANDED.worst && px(worst.clearance) === EXPANDED.worstBy,
    detail: `${fits.length} label(s) (want ${EXPANDED.marks}), ${column.length} of them in ` +
      `the expanded column (want ${EXPANDED.column}); tightest vertically is ` +
      `${stack ? `${stack.above} over ${stack.below} with ${px(stack.gap)}` : "nothing"} ` +
      `(want ${EXPANDED.stack}, against the 12px one line of label claims) and ` +
      `${ambiguous.length ? ambiguous[0] : "no two marks draw the same words"}; ` +
      `${over.length} label(s) are wider than the gutter (want ${EXPANDED.over}, all in the ` +
      `expanded column), worst ${worst.id} at ${px(worst.clearance)} (want ${EXPANDED.worst} ` +
      `at ${EXPANDED.worstBy}) -- measured at this file's 0.6em advance, which is wider than ` +
      `any system face sets`,
  }];
}
