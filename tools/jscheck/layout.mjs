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

import { loadApp, goldenGraph, spineConfig } from "./harness.mjs";
import { openedWindow } from "./drill.mjs";

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
             [app.CHART_WIDTH - app.LABEL_GUTTER, app.CHART_HEIGHT - 12]]);
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
 * THE WORDS COME OFF THE PAGE. markCents and fmtShortSigned are the same two
 * functions render() hands the value tspan, so this measures the string a
 * reader sees rather than one this file spelled for itself -- and the flag
 * tspan is counted, because a derived node's label is three characters longer
 * than its neighbours' and that is exactly the case a fit check is about.
 */
function labelWidth(app, d) {
  const value = "  " + app.fmtShortSigned(app.markCents(d)) + (d.derived ? "  \u25c7" : "");
  return d.label.length * LABEL_PX * ADVANCE_EM + value.length * VALUE_PX * ADVANCE_EM;
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
    to: col >= last || !after ? app.CHART_WIDTH : (me.x1 + after.x0) / 2,
  };
}

/** Every label of a laid graph, boxed where app.js puts it, against its room. */
function labelFit(app, graph) {
  const columns = columnsOf(app, graph);
  const last = Math.max(...columns.keys());
  return graph.nodes.map((n) => {
    const place = app.labelPlacement(n, last);
    const width = labelWidth(app, n);
    const box = boxAt(place.anchor, place.x, width);
    const col = app.columnOf(n);
    const room = roomFor(app, columns, col);
    return { node: n, id: n.id, col, last, place, width, box, room,
             clearance: Math.min(box.left - room.from, room.to - box.right) };
  });
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
  const app = loadApp({ config: spineConfig() });
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
      name: "the fund column is pinned to the palette's order",
      ok: (() => {
        const drawn = graph.nodes.filter(app.isFundGroup)
          .sort((a, b) => a.y0 - b.y0).map((n) => n.id);
        return JSON.stringify(drawn) ===
               JSON.stringify(app.FUND_ORDER.filter((f) => drawn.includes(f)));
      })(),
      detail: "fund groups run top to bottom in FUND_ORDER, which is what " +
              "supplying .nodeSort() at all is for",
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
    {
      name: "the client refuses a document it does not understand",
      ok: app.understands(app.SCHEMA_VERSION, "x") === true &&
          app.understands(app.SCHEMA_VERSION + 1, "x") === false &&
          app.understands(app.SCHEMA_VERSION - 1, "x") === false,
      detail: `schema_version ${app.SCHEMA_VERSION} is accepted and its neighbours are refused`,
    },
  ].concat(await labelChecks(app, graph));
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
