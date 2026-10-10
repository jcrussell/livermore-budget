// The client's layout over the columns Go pins: ribbon crossings under each
// node ordering, restackLinks, label anchoring and room, the four-column
// window, the contra band, and two stylesheet rules jsdom cannot evaluate.
//
// Every number is measured post-restack, which is the chart a reader sees.
// Every test diagnoses what it measured, pass or fail.

import { before, describe, test } from "node:test";
import assert from "node:assert/strict";

import { bootedApp, goldenGraph, pageFixture, wholeFixture, stylesheet, opened, expandAll, everyOffer, clickYear,
  dollars, settle, columnFixture, loadD3 } from "./testlib.mjs";
// core.js and sankey.js themselves for the expansion's layout, which no page
// draws yet: app.js binds its own foldDocument to the page's tier set.
import * as core from "./core.js";
import * as sankey from "./sankey.js";

/**
 * Lays the golden graph out as layOut() does, under a given node sort.
 *
 * The constants and the aligner come off the page: a copy here would measure a
 * chart the page does not draw. `align` is passed only to compare aligners.
 */
function layout(app, nodeSort, align) {
  const doc = goldenGraph();
  const sankey = globalThis.d3.sankey()
    .nodeId((d) => d.id)
    .nodeWidth(app.NODE_WIDTH)
    .nodePadding(app.NODE_PADDING)
    .nodeAlign(align || app.alignFor(app.slotsOf(doc.nodes, app.RENDER_TIERS)))
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
 * FAILS CLOSED ON A LINK SPANNING MORE THAN ONE COLUMN: it would pass over
 * ribbons nothing compares it against, and tangle() would undercount.
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
 * Crossings and the ribbon value they overlap: two same-band ribbons cross when
 * their left-end and right-end orders disagree. Overlap is charged at the
 * narrower of the two.
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

/** Pairs of a node's ribbons stacked in an order its neighbours no longer sit in. */
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

// The figures nodeRank was chosen on. PINNED, not bounded: a `>` would let
// the alternatives drift while the check stayed green.
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

// THE ADVANCE A LABEL IS MEASURED AT, an over-estimate on purpose: `system-ui`
// differs per platform, so 0.6em is wider than any usual face sets, and the
// error runs toward refusing a label that would have fitted.
const ADVANCE_EM = 0.6;
const LABEL_PX = 12;
const VALUE_PX = 11;

/**
 * How wide the three tspans of one node's label draw.
 *
 * THE WORDS COME OFF THE PAGE: markCents, fmtShortSigned and nodeFlags are what
 * render() draws. A copy of nodeFlags' markers here would under-measure every
 * label the day a marker is added.
 */
function labelWidth(app, d, qualifier) {
  const value = "  " + app.fmtShortSigned(app.markCents(d)) + app.nodeFlags(d);
  const label = d.label.length * LABEL_PX * ADVANCE_EM + value.length * VALUE_PX * ADVANCE_EM;
  // THE WIDEST LINE IS THE BOX, not the sum: a qualifier draws on its own line.
  return Math.max(label, (qualifier || "").length * LABEL_PX * ADVANCE_EM);
}

// THE INK ONE LINE CLAIMS ABOVE AND BELOW ITS BASELINE, over-estimated like
// ADVANCE_EM: they sum to the full 12px font size.
const ASCENT_PX = 9;
const DESCENT_PX = 3;

/**
 * The vertical run one mark's label draws over, one line or two. The shifts
 * come off the page's labelLineShift.
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

/** Every column the graph drew, as an x-range, keyed by the form's own index. */
function columnsOf(app, graph) {
  const out = new Map();
  const slots = app.slotsOf(graph.nodes, app.activeTiers());
  for (const n of graph.nodes) {
    const c = app.columnOf(slots, n);
    if (!out.has(c)) out.set(c, { x0: n.x0, x1: n.x1 });
  }
  return out;
}

/**
 * The x-range a column's labels may claim: half the band on either side, and
 * the gutter out to the edge for the outermost two.
 *
 * NOT READ OFF labelPlacement: this says what room exists, so the two can
 * disagree.
 */
function roomFor(app, columns, col) {
  const last = Math.max(...columns.keys());
  const me = columns.get(col);
  const before = columns.get(col - 1);
  const after = columns.get(col + 1);
  return {
    from: col === 0 || !before ? 0 : (before.x1 + me.x0) / 2,
    // The chart's own right edge, at its own column count.
    to: col >= last || !after
      ? app.chartWidth(app.drawnColumns()) : (me.x1 + after.x0) / 2,
  };
}

/**
 * Every label of a laid graph, boxed where app.js puts it, against its room.
 * MEASURED WITH THE VIEW ON SCREEN: the columns handed to columnOf and the
 * flags nodeFlags reads are the drill state's.
 */
function labelFit(app, graph) {
  const columns = columnsOf(app, graph);
  const last = Math.max(...columns.keys());
  // The words the page would draw, qualifier included.
  const slots = app.slotsOf(graph.nodes, app.activeTiers());
  const qualifiers = app.labelQualifiers(app.projection, slots, graph.nodes);
  return graph.nodes.map((n) => {
    const place = app.labelPlacement(slots, n, last);
    const qualifier = qualifiers.get(n.id) || "";
    const width = labelWidth(app, n, qualifier);
    const box = boxAt(place.anchor, place.x, width);
    const col = app.columnOf(slots, n);
    const room = roomFor(app, columns, col);
    return { node: n, id: n.id, col, last, place, width, box, room, qualifier,
             words: (qualifier ? qualifier + " / " : "") + n.label,
             block: labelBlock(app, place, qualifier),
             clearance: Math.min(box.left - room.from, room.to - box.right) };
  });
}

/**
 * Every pair of marks a reader would see the same words on: same column, same
 * drawn label, qualifier and all. Asked of the words rather than of a tier.
 */
function sameWords(fits) {
  const seen = new Map();
  for (const f of fits) {
    const key = f.col + "\0" + f.words;
    seen.set(key, (seen.get(key) || []).concat([f.id]));
  }
  return [...seen.entries()]
    .filter((e) => e[1].length > 1)
    .map((e) => `column ${e[0].split("\0")[0]} draws ${e[1].length} marks ` +
                `reading "${e[0].split("\0")[1]}" (${e[1].join(", ")})`);
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
 * Nothing here parses CSS: this reads which PROPERTY .chart-wrap's cap comes
 * through, and the px is the value app.js hands that property. It pins the
 * declaration's shape, so an equivalent clamp() goes red -- the safe direction.
 *
 * ONLY A RULE WHOSE WHOLE SELECTOR IS .chart-wrap COUNTS, and two of them is a
 * refusal: `.card:fullscreen .chart-wrap {` contains `.chart-wrap {`, and a
 * second rule under @media is a cap this cannot place.
 *
 * @param {string} css
 * @param {number | null} handed the px app.js set --chart-max to, or null
 */
function chartAllowance(css, handed) {
  // A rule boundary before the selector: start of text, a brace, or a comment end.
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
  // The cushion is read off its own declaration: the min()'s second operand is a var() too.
  const cushion = /--chart-cushion:\s*(\d+)px/.exec(rule[1]);
  if (!/calc\(100vw\s*-\s*var\(--chart-cushion\)\)/.test(rule[1])) {
    return { px: null, why: ".chart-wrap does not take its sub-cap width from 100vw less --chart-cushion" };
  }
  return {
    px: handed,
    // Below the cap the chart gets 100vw minus this, so n columns fit at
    // chartWidth(n) + cushion, which COLUMN_QUERIES has to agree with.
    cushion: cushion ? Number(cushion[1]) : null,
    why: "",
  };
}

// The one query grammar this file reads a breakpoint out of; any other entry
// of COLUMN_QUERIES is a named failure.
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
 * The page as shipped, booted on its first published column. It DRAWS BEFORE
 * ANYTHING IS MEASURED: unfetched, nodeRank's fund order ties everywhere and
 * degenerates to size-descending, one of the alternatives measured below.
 */
function spineApp() {
  return bootedApp({ checkedStem: "sankey" });
}

/** The node sort app.js draws the spine under. */
const byRank = (app) => {
  const rank = (n) => app.nodeRank(app.fundGroupOf, app.fundGroupPlace, n);
  return (a, b) => rank(a) - rank(b) || b.value - a.value;
};

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

  // Not "after.stale === 0", a tautology: staleStacked uses restackLinks'
  // comparator. The crossings removed are counted independently.
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

  // The spine's figures hold under sankeyJustify structurally, not by luck:
  // tier 0 is pure source, tier 5 pure sink. This goes red the day that stops.
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
    // The served list filtered by what is drawn, so this is an equality:
    // a group the sequence does not name sorts last, and must draw last.
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

// Nodes of the fund-group window whose declared column is not d3's longest
// path to them: the residual has no ribbon from the middle column.
const DEPTH_DISAGREES = ["residual/fund-group/general"];

// The four-column chart the band and geometry tests run over, measured over
// the committed FY 2025-26 capture.
const WIDE_PATH = ["fund-group/general", "fund/100"];

// A division's own window, where the SHORT label is the right one.
const DIVISION_WINDOW = "dept/patrol";
const WIDE_BANDS = { keys: "0:1 1:2 2:3", sizes: "1/23/30" };

// The one width this file pins, the THREE-column one every figure was
// measured at. No wider width may be pinned: chartWidth(n) derives them.
const NARROW_DESIGN_WIDTH = 1180;

/**
 * The bands of a four-column chart: every measurement here is counted band by
 * band, and a widened tier the document could not fill would leave a hole.
 * Sizes are summed against the graph's own link count, so a chart with no
 * links at all fails rather than counting nothing.
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
    // A division's own window: labelling cells by division would draw "Patrol" twice.
    app.drillUp(0);
    await settle();
    await opened(app, ...WIDE_PATH, DIVISION_WINDOW);
    innerFits = labelFit(app, app.layOut(app.projection));
    ambiguous = sameWords(fits).concat(sameWords(innerFits));
    stack = tightestStack(fits);
    // The px is app.js's CHART_MAX; the stylesheet carries none.
    allowance = chartAllowance(stylesheet(), app.CHART_MAX);
    // Capped by the cap. A query this file cannot read is set aside and
    // named: a throw here would cancel the whole suite.
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

  // LABEL_GUTTER does not grow with the column count: only the end columns
  // anchor outward.
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

  // Asked of the words, not of the tier: no two marks in one column read the same.
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

  // A qualified label is two lines tall; its block must still miss the one below.
  test("a qualifier's second line still clears the label stacked below it", (t) => {
    const detail = stack
      ? `tightest stack in the four-column window is column ${stack.col}, ` +
        `${stack.above} over ${stack.below}, clearing ${px(stack.gap)}`
      : "the window stacked no two labels in one column, so nothing here measured a gap";
    t.diagnostic(detail);
    assert.ok(stack, detail);
    assert.ok(stack.gap >= 0, detail);
  });

  // THE CONTAINER AGAINST THE DRAWING: a viewBox'd <svg> in a narrower
  // container draws smaller, not clipped. AN EQUALITY, NOT A FLOOR: both are
  // derived from the same declarations, so slack is the defect.
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

  // A query this file cannot derive a viewport from is refused by name.
  test("every viewport query the page declares is a min-width in px this file can read", (t) => {
    const detail = unreadable.length === 0
      ? `${queryRoom.length} column quer${queryRoom.length === 1 ? "y" : "ies"} read, ` +
        `each (min-width: Npx)`
      : `${unreadable.length} of ${unreadable.length + queryRoom.length} column queries ` +
        `cannot be read as (min-width: Npx): ${unreadable.map((q) => JSON.stringify(q)).join(", ")}`;
    t.diagnostic(detail);
    assert.deepEqual(unreadable, [], detail);
  });

  // Every viewport at which the page ADDS a column has room for it: below the
  // cap the room is 100vw minus the cushion, so a threshold can pass the cap
  // test and still shrink the chart. Every entry, not the widest.
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
 * Where a drawn label actually sits, read off the mark render() appended, so
 * this measures labelPlacement's USE. `class` is read as an attribute because
 * an SVG className is not a string; a null dy must be ABSENT, not "null".
 *
 * @param {any} app a page with a chart already drawn
 * @param {Document} document the page it drew on
 * @param {any} laid the same chart's nodes, as laid out
 */
function drawnPlacement(app, document, laid) {
  const slots = app.slotsOf(laid.nodes, app.activeTiers());
  const last = Math.max(...laid.nodes.map((n) => app.columnOf(slots, n)));
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
    const want = app.labelPlacement(slots, d, last);
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
 * Where every label lands, on the spine and inside a window. On the spine
 * d3's depth and the declared column agree everywhere, so only a window can
 * tell the two rules apart.
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
    // The object-category window has an interior column; on the spine every
    // placement rule agrees.
    drawn = drawnPlacement(app, document, objectLaid);
    app.drillUp(0);
    await settle();
    await opened(app, FUND_GROUP_WINDOW);
    groupLaid = app.layOut(app.projection);
    const slots = app.slotsOf(groupLaid.nodes, app.activeTiers());
    disagree = groupLaid.nodes.filter((n) => app.columnOf(slots, n) !== n.depth).map((n) => n.id);
  });

  // Mutation: anchor labelPlacement's interior branch outward and this goes
  // red naming the node.
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

  // The alternative is measured too; its count is printed, not pinned.
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

  // Mutation: revert render()'s label .attr lines to `d.depth === 0 ? ...`
  // and this alone goes red, naming every interior mark.
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
 * The marks whose words run past their room, over every view everyOffer
 * opens -- named, not counted. An upper bound at ADVANCE_EM; fisc-rl4j's
 * browser walk settles which a rendered face actually overflows.
 */
const OVER_GUTTER = {
  "FY 2025-26": [
    "department/innovation-and-economic-development",
    "dept/administrative-services", "dept/community-development-admin",
    "dept/innovation-and-economic-devel", "dept/public-works-administration",
    "fund-balance/contribution", "fund/101",
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
    "transfer-to/611",
  ],
  // gap/expenditure/services-and-supplies is p0067's FY2026-27 shortfall,
  // the widest label drawn anywhere.
  "FY 2026-27": [
    "department/innovation-and-economic-development",
    "dept/administrative-services", "dept/community-development-admin",
    "dept/innovation-and-economic-devel", "dept/public-works-administration",
    "fund-balance/contribution", "fund/101",
    "fund/280", "fund/282", "fund/283", "fund/320", "fund/513", "fund/551",
    "fund/552", "fund/623", "fund/730",
    "gap/expenditure/services-and-supplies",
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
    "transfer-to/731", "transfer-to/826", "transfer-to/830",
  ],
};

/** Every view the drill opens, in each of the page's years, measured for label collisions and overflow. */
for (const label of Object.keys(OVER_GUTTER)) {
  describe(`${label}: every view the drill opens, measured for what a label does to its neighbours`, () => {
    let app, walk, views, over, overlapped, alike, ids, want;
    before(async () => {
      // Found by label, so a year the page no longer ships fails by name.
      const year = pageFixture().config.years.find((y) => y.label === label);
      if (!year) throw new Error(`the pinned page ships no year labelled ${JSON.stringify(label)}`);
      ({ app } = await bootedApp({ checkedStem: year.stem }));
      over = new Set();
      overlapped = [];
      alike = [];
      views = 0;
      walk = await everyOffer(app, () => {
        // The gutter is the Sankey's; a bar list's labels are site/bars.test.mjs's.
        if (app.formOnScreen().form !== "sankey") return;
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
      // Zero is the only value either may take, on any view.
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
 * The pins the expanded column has to hold. PINNED AND NOT BOUNDED: the stack
 * gap is the air over the expanded column, and `over` states what the gesture
 * costs the gutter under this file's pessimistic advance rather than asserting
 * it away. Which of those actually overflow is fisc-rl4j's browser question.
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

// The one mark on the site SHORTER than the ribbons arriving at it; the band
// is what makes that legible.
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

    // The excess is exactly twice the reductions: each is drawn forward at its
    // magnitude where the schedule subtracts it.
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
    // An SVG rect with no fill paints SOLID BLACK, so a renamed class would
    // cover the ribbons the band reveals. The class is read off the drawn band.
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

    // And they land inside the band: the ordering alone would pass with no band.
    const band = app.contraBand(centre);
    const strays = arriving.filter((l) => l.contra)
      .filter((l) => l.y1 - l.width / 2 < band.y - 1e-9);
    assert.deepEqual(strays.map((l) => l.source.id), [], "a reduction starts above the band");
    t.diagnostic(`${arriving.length} arriving ribbons, the ${arriving.length - firstContra} ` +
      `reduction(s) last and all inside the band`);
  });
});

// Nothing in this tree renders CSS, so rule blocks are extracted by selector
// string and one property compared (fisc-6at).
describe("the focus indicator is painted as focus and not as text", () => {
  let css;
  // Comments are stripped first, so prose about a property cannot satisfy or break the check.
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

    // AND --focus IS DECLARED WHEREVER --ink-1 IS, or a palette drops the indicator.
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

  test("an isolated mark has an indicator of its own, distinct from focus, and focus wins where both apply", (t) => {
    const pressed = "svg.sankey .node[aria-pressed=\"true\"] rect";
    const rule = ruleFor(pressed);
    assert.match(rule, /stroke:\s*var\(--[a-z0-9-]+\)/, "the isolated mark's stroke is not a palette token");
    assert.doesNotMatch(rule, /var\(--focus\)/, "the isolated mark is painted as focus, so a reader cannot tell held from focused");
    assert.doesNotMatch(rule, /stroke-dasharray/, "the rule resets the dash, so an isolated derived mark stops reading as derived");
    // At equal specificity source order decides: after derived, before focus.
    const at = (sel) => css.indexOf(sel + " {");
    assert.ok(at("svg.sankey .node.derived rect") < at(pressed), "the isolation rule precedes the derived rule, which would override it");
    assert.ok(at(pressed) < at("svg.sankey .node:focus-visible rect"), "the isolation rule follows the focus rule, which it would override");
    t.diagnostic(`the isolated mark's rule is ${rule.trim().replace(/\s+/g, " ")}`);
  });

  test("a ribbon gets a drawn echo and not a box around its bounding rectangle", (t) => {
    const rule = ruleFor("svg.sankey .link:focus-visible");
    // A <path>'s outline boxes its whole bezier, so the echo is a drop-shadow.
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

describe("the residual's place in its column", () => {
  test("a residual, whose neighbours are all endpoints, ranks at its own group's place and sorts by value among that group's funds", async (t) => {
    const { app } = await bootedApp();
    app.setColumnBudget(app.OFFERED_COLUMNS);
    const rank = (n) => app.nodeRank(app.fundGroupOf, app.fundGroupPlace, n);
    const groups = app.projection.nodes.filter((n) => app.isFundGroup(n) && app.drillable(n)).map((n) => n.id);
    const seen = [];
    for (const g of groups) {
      await opened(app, g);
      const laid = app.layOut(app.projection);
      const residual = laid.nodes.find((n) => app.isResidual(n.id));
      if (residual) {
        const column = laid.nodes.filter((n) => n.layer === residual.layer).sort((a, b) => a.y0 - b.y0);
        const at = column.indexOf(residual);
        seen.push(`${g.replace("fund-group/", "")}: rank ${rank(residual)}, ${at + 1} of ${column.length}`);
        assert.ok(residual.sourceLinks.concat(residual.targetLinks).every((l) =>
          !app.fundGroupOf(l.source === residual ? l.target : l.source)), `${residual.id} has a neighbour in a group`);
        assert.equal(rank(residual), app.fundGroupPlace(g), `${residual.id} does not rank at ${g}'s place`);
        // Every mark beside it in the column is the group's, so equal rank
        // leaves the order to value.
        for (let k = 1; k < column.length; k++) {
          if (rank(column[k - 1]) !== rank(column[k])) continue;
          assert.ok(column[k - 1].value >= column[k].value, `${g}: ${column[k - 1].id} sits above the larger ${column[k].id}`);
        }
      }
      app.drillUp(0);
      await settle();
    }
    t.diagnostic(seen.join("; "));
    assert.ok(seen.length > 0, "no fund group's window draws a residual, so its rank is never asked");
    assert.ok(seen.some((s) => !/rank 0,/.test(s)), "every residual's group is at place 0, which the old rank also gave");
  });
});

// THE WHOLE CITY IN ONE SANKEY (fisc-yj4w.23): the whole-budget page,
// testdata/whole.golden.html, shaped by the page's own shapeFor and laid out
// by the form's own layOutAt at each candidate canvas height, folded as it
// opens and with every tail drawn out. The declared height is read off the
// page's own layOut, which is what the hint reaches.
const WHOLE_HEIGHTS = [820, 1600, 2400, 3200, 4800];

/**
 * Each pinned year's whole-budget chart at every candidate height, folded and
 * drawn out: nodes, links, ribbons under a pixel and the narrowest ribbon;
 * then the chart the page lays out at its declared canvas, folded.
 * PINNED, not bounded: the height in data/views.yaml is read off these.
 */
const WHOLE_CLAIMED = {
  "FY 2025-26": {
    folded: [
      "820: 52 nodes, 96 links, 15 under 1px, narrowest 0.0112px",
      "1600: 52 nodes, 96 links, 8 under 1px, narrowest 0.0246px",
      "2400: 52 nodes, 96 links, 6 under 1px, narrowest 0.0383px",
      "3200: 52 nodes, 96 links, 5 under 1px, narrowest 0.0520px",
      "4800: 52 nodes, 96 links, 2 under 1px, narrowest 0.0793px",
    ],
    drawnOut: [
      "820: 229 nodes, 262 links, 262 under 1px, narrowest 0.0000px",
      "1600: 229 nodes, 262 links, 262 under 1px, narrowest 0.0000px",
      "2400: 229 nodes, 262 links, 123 under 1px, narrowest 0.0000px",
      "3200: 229 nodes, 262 links, 82 under 1px, narrowest 0.0001px",
      "4800: 229 nodes, 262 links, 51 under 1px, narrowest 0.0002px",
    ],
    declared: "52 nodes, 96 links, 2 under 1px, narrowest 0.0793px",
  },
  "FY 2026-27": {
    folded: [
      "820: 52 nodes, 95 links, 16 under 1px, narrowest 0.0112px",
      "1600: 52 nodes, 95 links, 8 under 1px, narrowest 0.0244px",
      "2400: 52 nodes, 95 links, 7 under 1px, narrowest 0.0380px",
      "3200: 52 nodes, 95 links, 4 under 1px, narrowest 0.0516px",
      "4800: 52 nodes, 95 links, 1 under 1px, narrowest 0.0786px",
    ],
    drawnOut: [
      "820: 224 nodes, 257 links, 257 under 1px, narrowest 0.0000px",
      "1600: 224 nodes, 257 links, 257 under 1px, narrowest 0.0000px",
      "2400: 224 nodes, 257 links, 119 under 1px, narrowest 0.0000px",
      "3200: 224 nodes, 257 links, 83 under 1px, narrowest 0.0001px",
      "4800: 224 nodes, 257 links, 50 under 1px, narrowest 0.0002px",
    ],
    declared: "52 nodes, 95 links, 1 under 1px, narrowest 0.0786px",
  },
};

/**
 * One laid chart in the pins' words.
 * @param {{nodes: any[], links: any[]}} laid
 */
function laidFigures(laid) {
  const widths = laid.links.map((l) => l.width);
  return `${laid.nodes.length} nodes, ${laid.links.length} links, ` +
    `${widths.filter((w) => w < 1).length} under 1px, narrowest ${Math.min(...widths).toFixed(4)}px`;
}

describe("the whole city in one Sankey, folded as it opens and drawn out", () => {
  /** @type {Map<string, {folded: string[], drawnOut: string[], declared: string, doubled: string}>} */
  const measured = new Map();

  before(async () => {
    const page = wholeFixture();
    const { app, document } = await bootedApp({ html: page.html, config: page.config });
    const at = (doc, height) => app.layOutAt(doc, {
      tiers: app.layoutTiers(doc),
      columns: app.layoutColumns(doc),
      groupOf: app.fundGroupOf,
      placeOf: app.fundGroupPlace,
    }, height);
    for (const year of page.config.years) {
      clickYear(document, year.stem);
      await settle();
      assert.equal(app.shownYear.stem, year.stem, `the page did not switch to ${year.label}`);
      const folded = WHOLE_HEIGHTS.map((h) => `${h}: ${laidFigures(at(app.shown.doc, h))}`);
      const declared = laidFigures(app.layOut(app.shown.doc));
      expandAll(app);
      await settle();
      const drawnOut = WHOLE_HEIGHTS.map((h) => `${h}: ${laidFigures(at(app.shown.doc, h))}`);
      // The tier set the group tier is dropped from, drawn with it: each
      // fund's inflow arrives once from its revenue lines and once from its
      // group, so the fund's mark is drawn at the sum of the two.
      const withGroups = app.foldDocument(app.fetched, [0, 2, 3, 4, 5]);
      const into = new Map();
      for (const l of withGroups.links) {
        const n = withGroups.nodes.find((x) => x.id === l.target);
        if (n && n.tier === 3) into.set(l.target, (into.get(l.target) || 0) + l.value_cents);
      }
      const own = new Map();
      for (const l of app.fetched.links) {
        if (app.fetched.nodes.some((x) => x.id === l.source && x.tier === 2)) own.set(l.target, (own.get(l.target) || 0) + l.value_cents);
      }
      const twice = [...own].filter(([id, c]) => into.get(id) === 2 * c).length;
      measured.set(year.label, { folded, drawnOut, declared, doubled: `${twice} of ${own.size} funds drawn at twice their inflow` });
      app.collapseTier(3); app.collapseTier(4); app.collapseTier(5);
      await settle();
    }
  });

  test("folded as it opens, no candidate height draws every printed ribbon at a pixel, and the tallest leaves the fewest under one", (t) => {
    for (const [label, m] of measured) t.diagnostic(`${label} folded by height: ${m.folded.join("; ")}`);
    assert.deepEqual(Object.fromEntries([...measured].map(([k, m]) => [k, m.folded])),
      Object.fromEntries(Object.entries(WHOLE_CLAIMED).map(([k, m]) => [k, m.folded])));
  });

  test("with every tail drawn out, every candidate height leaves ribbons under a pixel, the hairlines the page says it draws", (t) => {
    for (const [label, m] of measured) t.diagnostic(`${label} drawn out by height: ${m.drawnOut.join("; ")}`);
    assert.deepEqual(Object.fromEntries([...measured].map(([k, m]) => [k, m.drawnOut])),
      Object.fromEntries(Object.entries(WHOLE_CLAIMED).map(([k, m]) => [k, m.drawnOut])));
  });

  test("the page lays its chart out on the canvas its sankey hint declares", (t) => {
    for (const [label, m] of measured) t.diagnostic(`${label} at the declared canvas, folded: ${m.declared}`);
    assert.deepEqual(Object.fromEntries([...measured].map(([k, m]) => [k, m.declared])),
      Object.fromEntries(Object.entries(WHOLE_CLAIMED).map(([k, m]) => [k, m.declared])));
  });

  test("drawn with its group tier, every fund's inflow would be drawn twice, which is why the page drops it", (t) => {
    for (const [label, m] of measured) {
      t.diagnostic(`${label}: ${m.doubled}`);
      const [twice, , all] = m.doubled.split(" ");
      assert.equal(twice, all, `${label}: ${m.doubled}`);
    }
  });
});

// THE EXPANSION, LAID OUT (fisc-yj4w.34.3): each pinned column's expansion
// schedule folded to a frontier by core.foldDocument, capped per split and
// stamped with its slots by sankey.js's expansionOf, and laid out by the
// form's own layOutAt on the canvas the index overview's sankey.grow sizes.
// No page draws it yet, so the page's state is not read: the hints are the
// pinned index page's, the order of fund groups is the column's.
//
// EVERY ASSERT IS OVER KEYS AND NUMBERS. A failing assert over a laid d3
// graph formats the whole of it, which this host cannot hold.

/**
 * Per year and frontier: the nodes in each slot, left to right; nodes and
 * ribbons; the canvas and the ribbons under a pixel on it; the labels wider
 * than their room (half the band either side, or the gutter at either end)
 * at this file's ADVANCE_EM, value included and flags not (the expansion's
 * flags are the interaction's); and the worst of them.
 * PINNED, not bounded: data/views.yaml's grow is read off these.
 */
const EXPANSION_CLAIMED = {
  "FY 2025-26": {
    "root": { slots: "12/6/7", figures: "25 nodes, 58 ribbons; 820px canvas, 9 under 1px",
      labels: "0 wider than their room" },
    "fund-group/general by funds": { slots: "12/6/7", figures: "25 nodes, 58 ribbons; 820px canvas, 9 under 1px",
      labels: "0 wider than their room" },
    "everything by primary splits": { slots: "74/36/23/45",
      figures: "178 nodes, 290 ribbons; 2960px canvas, 54 under 1px",
      labels: "27 wider than their room: 10 in the first column, 0 inside, 17 in the last; " +
        "worst expenditure/fund/720/services-and-supplies by -115.8px" },
    "everything by primary splits, every tail drawn out": { slots: "102/69/23/162",
      figures: "356 nodes, 389 ribbons; 4800px canvas, 116 under 1px",
      labels: "99 wider than their room: 13 in the first column, 0 inside, 86 in the last; " +
        "worst expenditure/fund/221/services-and-supplies by -187.2px" },
  },
  "FY 2026-27": {
    "root": { slots: "12/6/7", figures: "25 nodes, 58 ribbons; 820px canvas, 11 under 1px",
      labels: "0 wider than their room" },
    "fund-group/general by funds": { slots: "12/6/7", figures: "25 nodes, 58 ribbons; 820px canvas, 11 under 1px",
      labels: "0 wider than their room" },
    "everything by primary splits": { slots: "74/37/23/44",
      figures: "178 nodes, 286 ribbons; 2960px canvas, 55 under 1px",
      labels: "27 wider than their room: 10 in the first column, 0 inside, 17 in the last; " +
        "worst expenditure/fund/720/services-and-supplies by -115.8px" },
    "everything by primary splits, every tail drawn out": { slots: "102/68/23/159",
      figures: "352 nodes, 385 ribbons; 4800px canvas, 118 under 1px",
      labels: "95 wider than their room: 13 in the first column, 0 inside, 82 in the last; " +
        "worst expenditure/fund/221/services-and-supplies by -187.2px" },
  },
};

/** The expansion's frontiers, each from the schedule's own helpers. */
function expansionFrontiers(doc) {
  const root = core.rootFrontier(doc);
  let all = root;
  for (let changed = true; changed;) {
    changed = false;
    for (const id of [...all]) {
      const splits = core.splitsOf(doc, id);
      if (!splits.length || core.openSplitOf(doc, all, id) !== "") continue;
      all = core.expand(doc, all, id, splits.find((sp) => !sp.alternative).key);
      changed = true;
    }
  }
  const none = new Set();
  const every = new Set(doc.splits.map((sp) => sp.node + "\u001f" + sp.key));
  return [
    { name: "root", frontier: root, whole: none },
    { name: "fund-group/general by funds", frontier: core.expand(doc, root, "fund-group/general", "funds"), whole: none },
    { name: "everything by primary splits", frontier: all, whole: none },
    { name: "everything by primary splits, every tail drawn out", frontier: all, whole: every },
  ];
}

/**
 * Each label of a laid expansion against its room, as labelFit measures the
 * page's: placed by labelPlacement over the same slots, anchored as drawn.
 */
function expansionLabels(laid, slots, columns) {
  const xs = new Map();
  for (const n of laid.nodes) if (!xs.has(slots.get(n.id))) xs.set(slots.get(n.id), { x0: n.x0, x1: n.x1 });
  const last = Math.max(...xs.keys());
  return laid.nodes.map((n) => {
    const col = sankey.columnOf(slots, n);
    const place = sankey.labelPlacement(slots, n, last);
    const value = "  " + core.fmtShortSigned(sankey.markCents(n));
    const box = boxAt(place.anchor, place.x, n.label.length * LABEL_PX * ADVANCE_EM + value.length * VALUE_PX * ADVANCE_EM);
    const me = xs.get(col);
    const before = xs.get(col - 1);
    const after = xs.get(col + 1);
    const room = {
      from: col === 0 || !before ? 0 : (before.x1 + me.x0) / 2,
      to: col >= last || !after ? sankey.chartWidth(columns) : (me.x1 + after.x0) / 2,
    };
    return { id: n.id, col, last, clearance: Math.min(box.left - room.from, room.to - box.right) };
  });
}

describe("the expansion schedule laid out in slots, at the root, one group open and everything open", () => {
  /** @type {Map<string, Record<string, any>>} */
  const measured = new Map();
  /** @type {Map<string, {cells: number, ribbons: number, folded: number, missing: string[]}>} */
  const divisionCells = new Map();
  before(() => {
    loadD3();
    const page = pageFixture();
    globalThis.FISC_CONFIG = page.config;
    const chart = page.config.overview;
    for (const year of page.config.years) {
      const column = columnFixture(year.path.replace(/\.json$/, ""));
      const doc = core.scheduleOf(column, "expansion");
      const index = new Map(doc.nodes.map((n) => [n.id, n]));
      const groups = column.fund_groups.map((g) => g.id);
      const out = {};
      for (const f of expansionFrontiers(doc)) {
        const drawn = sankey.markContra(sankey.expansionOf(doc, f.frontier, chart, f.whole));
        const slots = sankey.slotsOf(drawn.nodes, chart.sankey.tiers);
        const held = [];
        for (const at of slots.values()) held[at] = (held[at] || 0) + 1;
        const height = sankey.canvasHeight(chart, drawn.nodes);
        // A layout d3 refuses is a measurement here, so the slots are still
        // read against their pin rather than lost to the hook.
        let laid = { nodes: [], links: [] };
        let refused = "";
        try {
          laid = sankey.layOutAt(drawn, {
            tiers: chart.sankey.tiers, columns: held.length,
            groupOf: (n) => core.fundGroupOf(index, n), placeOf: (id) => groups.indexOf(id),
          }, height);
        } catch (e) {
          refused = `d3-sankey refused the layout: ${e.message}`;
        }
        const over = expansionLabels(laid, slots, held.length).filter((l) => l.clearance < 0)
          .sort((a, b) => a.clearance - b.clearance);
        out[f.name] = {
          slots: Array.from(held, (n) => n || 0).join("/"),
          figures: refused || `${laid.nodes.length} nodes, ${laid.links.length} ribbons; ${height}px canvas, ` +
            `${laid.links.filter((l) => l.width < 1).length} under 1px`,
          labels: refused || `${over.length} wider than their room` + (over.length
            ? `: ${over.filter((l) => l.col === 0).length} in the first column, ` +
              `${over.filter((l) => l.col > 0 && l.col < l.last).length} inside, ` +
              `${over.filter((l) => l.col >= l.last).length} in the last; worst ${over[0].id} by ${px(over[0].clearance)}`
            : ""),
          backward: drawn.links.filter((l) => !(slots.get(l.source) < slots.get(l.target)))
            .map((l) => `${l.source} > ${l.target}`),
          layers: laid.nodes.filter((n) => n.layer !== slots.get(n.id)).map((n) => n.id),
          unplaced: drawn.nodes.filter((n) => !slots.has(n.id)).map((n) => n.id),
          general: ["fund-group/general", "fund/100"].map((id) => `${id} ${slots.has(id) ? "in " + slots.get(id) : "not drawn"}`).join(", "),
        };
        // THE GENERAL FUND'S DIVISION-BY-OBJECT CELLS (fisc-yj4w.32): every
        // link from one of fund/100's divisions to an object row, each a
        // ribbon of its own at its cents and facts.
        if (f.whole.size || f.name === "everything by primary splits") {
          const divisions = new Set(core.splitsOf(doc, "fund/100").find((sp) => sp.key === "divisions").members);
          const cells = doc.links.filter((l) => divisions.has(l.source) && !index.get(l.target).alternative &&
            index.get(index.get(l.target).parent || "") && index.get(index.get(l.target).parent).role === "object_category");
          const ribbons = new Map(drawn.links.map((l) => [`${l.source} > ${l.target} (${l.kind})`, l]));
          const missing = cells.filter((l) => {
            const r = ribbons.get(`${l.source} > ${l.target} (${l.kind})`);
            return !r || r.value_cents !== l.value_cents || r.fact_ids.join() !== [...l.fact_ids].sort().join();
          }).map((l) => `${l.source} > ${l.target}`);
          divisionCells.set(`${year.label}, ${f.name}`, { cells: cells.length, ribbons: cells.length - missing.length, folded: missing.length, missing });
        }
      }
      measured.set(year.label, out);
    }
  });

  test("every node is placed, in a slot no ribbon runs backward into, and d3 lays each in its slot", (t) => {
    for (const [label, m] of measured) {
      for (const [name, f] of Object.entries(m)) {
        t.diagnostic(`${label}, ${name}: slots ${f.slots}; ${f.backward.length} ribbon(s) not running forward`);
        assert.deepEqual({ unplaced: f.unplaced, backward: f.backward, layers: f.layers, empty: f.slots.split("/").includes("0") },
          { unplaced: [], backward: [], layers: [], empty: false }, `${label}, ${name}`);
      }
    }
  });

  // A refine member is drawn in its parent's slot: the group's one fund
  // stands where the group stood.
  test("the General Fund group opened by funds draws fund/100 in the group's slot, and the group not at all", (t) => {
    for (const [label, m] of measured) {
      t.diagnostic(`${label}: at the root ${m.root.general}; opened ${m["fund-group/general by funds"].general}`);
      assert.equal(m.root.general, "fund-group/general in 1, fund/100 not drawn", label);
      assert.equal(m["fund-group/general by funds"].general, "fund-group/general not drawn, fund/100 in 1", label);
    }
  });

  test("only sankey.js reads the overview's canvas growth and split caps", async (t) => {
    const config = structuredClone(pageFixture().config);
    /** @type {{prop: string, frame: string}[]} */
    const reads = [];
    config.overview.sankey = new Proxy(config.overview.sankey, {
      get(target, prop) {
        const frame = ((new Error().stack || "").split("\n")[2] || "").trim();
        reads.push({ prop: String(prop), frame });
        return target[prop];
      },
    });
    const { app } = await bootedApp({ config });
    const doc = core.scheduleOf(columnFixture(config.years[0].path.replace(/\.json$/, "")), "expansion");
    sankey.expansionOf(doc, expansionFrontiers(doc)[2].frontier, app.CONFIG.overview, new Set());
    const props = new Set(reads.map((r) => r.prop));
    t.diagnostic(`${reads.length} reads of ${[...props].join(", ")}`);
    for (const p of ["grow", "split_caps"]) assert.ok(props.has(p), `nothing read ${p}`);
    const outside = reads.filter((r) => (r.prop === "grow" || r.prop === "split_caps") && !r.frame.includes("site/sankey.js"));
    assert.deepEqual([...new Set(outside.map((r) => `${r.prop} read at ${r.frame}`))], []);
  });

  test("each frontier draws the slots, nodes, ribbons and canvas pinned", (t) => {
    const got = {};
    for (const [label, m] of measured) {
      got[label] = {};
      for (const [name, f] of Object.entries(m)) {
        t.diagnostic(`${label}, ${name}: slots ${f.slots}; ${f.figures}`);
        got[label][name] = `slots ${f.slots}; ${f.figures}`;
      }
    }
    const want = Object.fromEntries(Object.entries(EXPANSION_CLAIMED).map(([label, m]) =>
      [label, Object.fromEntries(Object.entries(m).map(([name, f]) => [name, `slots ${f.slots}; ${f.figures}`]))]));
    assert.deepEqual(got, want);
  });

  // fisc-2gyh: the labels a frontier draws wider than the room they have.
  test("the labels wider than their room are the ones pinned", (t) => {
    const got = {};
    for (const [label, m] of measured) {
      got[label] = {};
      for (const [name, f] of Object.entries(m)) {
        t.diagnostic(`${label}, ${name}: ${f.labels}`);
        got[label][name] = f.labels;
      }
    }
    const want = Object.fromEntries(Object.entries(EXPANSION_CLAIMED).map(([label, m]) =>
      [label, Object.fromEntries(Object.entries(m).map(([name, f]) => [name, f.labels]))]));
    assert.deepEqual(got, want);
  });

  // WHY THE EXPANSION FOLDS WITH mergeInferred: a capped split's tail can
  // hold a remainder beside printed members, a part of no completing set, so
  // the ribbon into it is drawn inferred. Pinned per year over every split
  // opened alone from the root and capped.
  test("the inferred ribbons into or out of a tail are the ones a remainder shares with printed members", (t) => {
    const want = {
      "FY 2025-26": [],
      "FY 2026-27": [
        "expenditure/services-and-supplies by divisions: expenditure/services-and-supplies > " +
          "aggregate/tail/divisions/expenditure/services-and-supplies (external)",
        "expenditure/services-and-supplies by object-rows: fund-group/internal-service > " +
          "aggregate/tail/object-rows/expenditure/services-and-supplies (internal_service)",
      ],
    };
    const got = {};
    const { config } = pageFixture();
    for (const year of config.years) {
      const doc = core.scheduleOf(columnFixture(year.path.replace(/\.json$/, "")), "expansion");
      const root = core.rootFrontier(doc);
      const found = new Set();
      for (const sp of doc.splits) {
        const open = core.expand(doc, core.reveal(doc, root, sp.node), sp.node, sp.key);
        for (const l of sankey.expansionOf(doc, open, config.overview, new Set()).links) {
          if (l.derived && (core.isAggregate(l.source) || core.isAggregate(l.target))) {
            found.add(`${sp.node} by ${sp.key}: ${l.source} > ${l.target} (${l.kind})`);
          }
        }
      }
      got[year.label] = [...found].sort();
      t.diagnostic(`${year.label}: ${got[year.label].length} inferred tail ribbon(s)`);
    }
    assert.deepEqual(got, want);
  });

  test("everything open with every tail drawn out draws every General Fund division-by-object cell as a ribbon of its own", (t) => {
    for (const [at, c] of divisionCells) {
      t.diagnostic(`${at}: ${c.ribbons} of ${c.cells} division-by-object cells drawn as their own ribbon, ${c.folded} folded into a tail or a merge`);
    }
    for (const [at, c] of divisionCells) {
      assert.ok(c.cells > 0, `${at}: no division-by-object cell to draw`);
      if (at.endsWith("every tail drawn out")) assert.deepEqual(c.missing, [], at);
    }
  });
});
