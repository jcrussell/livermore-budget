// The client's layout over the columns Go pins: crossings, restackLinks,
// label anchoring and room at every split, the expansion's slots, and the
// stylesheet rules jsdom cannot evaluate. Measured post-restack; every test
// diagnoses what it measured.

import { before, describe, test } from "node:test";
import assert from "node:assert/strict";

import { bootedApp, pageFixture, stylesheet, shapedAt, expansionStates, layOutState,
  settle, columnFixture, loadD3 } from "./testlib.mjs";
// core.js and sankey.js themselves for the expansion's layout at frontiers
// no gesture is needed to reach.
import * as core from "./core.js";
import * as sankey from "./sankey.js";

/** Links per adjacent column pair. Fails closed on a link spanning more: tangle() would undercount. */
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

/** Two same-band ribbons cross when their end orders disagree; overlap is charged at the narrower. */
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

/** Worst ribbon end outside its node face, at sankey.endWidth (narrower at a clamped mark). */
function extentOverflow(graph) {
  let worst = 0;
  for (const node of graph.nodes) {
    for (const [links, y, end] of [[node.sourceLinks, (l) => l.y0, "source"], [node.targetLinks, (l) => l.y1, "target"]]) {
      for (const l of links) {
        const w = sankey.endWidth(l, end);
        worst = Math.max(worst, node.y0 - (y(l) - w / 2), (y(l) + w / 2) - node.y1);
      }
    }
  }
  return worst;
}

// The synthetic city's spine as the page lays it out, PINNED, so a change to
// nodeRank, the seats or restackLinks that moves a ribbon shows here as a
// number. It moves when synthcity.Default does, and with nothing else.
const DRAWN = {
  crossings: 55,
  overlapDollars: 73910000,
};

const usd = (n) => "$" + Math.round(n).toLocaleString();

/**
 * A label's words and width at ADVANCE_EM, fitted by labelWords. The words
 * come off the page (markCents, fmtShortSigned, nodeFlags): a copied marker
 * list would under-measure when one is added.
 */
function labelWidth(app, d, place, room, qualifier, noun = "") {
  const value = "  " + app.fmtShortSigned(app.markCents(d));
  const flags = app.nodeFlags(d);
  const words = app.labelWords(room, qualifier, d.label, (w) => app.labelAdvance(w, value, flags), noun);
  return { words, width: app.labelAdvance(words, value, flags) };
}

const LABEL_PX = sankey.LABEL_PX;

// THE INK ONE LINE CLAIMS ABOVE AND BELOW ITS BASELINE, over-estimated like
// ADVANCE_EM: they sum to the full 12px font size.
const ASCENT_PX = 9;
const DESCENT_PX = 3;

/** The vertical run one mark's label draws over: one line, at its dy. */
function labelBlock(place) {
  const baseline = place.y + (place.dy ? parseFloat(place.dy) * LABEL_PX : 0);
  return { top: baseline - ASCENT_PX, bottom: baseline + DESCENT_PX };
}

/** The x-range a label of this width occupies, anchored this way at this x. */
function boxAt(anchor, x, width) {
  const left = anchor === "end" ? x - width : anchor === "middle" ? x - width / 2 : x;
  return { left, right: left + width };
}

/** Every column the graph drew, as an x-range, keyed by the form's own index. */
function columnsOf(app, graph) {
  const out = new Map();
  const slots = app.slotsOf(graph.nodes, app.RENDER_TIERS);
  for (const n of graph.nodes) {
    const c = app.columnOf(slots, n);
    if (!out.has(c)) out.set(c, { x0: n.x0, x1: n.x1 });
  }
  return out;
}

/** Half a band either side, the gutter for the end columns. Not read off labelPlacement, so the two can disagree. */
function roomFor(app, columns, col) {
  const last = Math.max(...columns.keys());
  const me = columns.get(col);
  const before = columns.get(col - 1);
  const after = columns.get(col + 1);
  return {
    from: col === 0 || !before ? 0 : (before.x1 + me.x0) / 2,
    // The chart's own right edge, at its own column count.
    to: col >= last || !after
      ? app.chartWidth(columns.size) : (me.x1 + after.x0) / 2,
  };
}

/** Every label against its room, measured with the view on screen (flags and qualifiers are its). */
function labelFit(app, graph) {
  const columns = columnsOf(app, graph);
  const last = Math.max(...columns.keys());
  // The words the page would draw, qualifier included.
  const slots = app.slotsOf(graph.nodes, app.RENDER_TIERS);
  // The document render() hands labelQualifiers: the unfolded nodes.
  const qualifiers = app.labelQualifiers({ nodes: [...app.groupIndex.values()] }, slots, graph.nodes);
  const nouns = app.labelNouns({ nodes: [...app.groupIndex.values()] }, graph.nodes, app.memberNounOf);
  return graph.nodes.map((n) => {
    const place = app.labelPlacement(slots, n, last);
    const qualifier = qualifiers.get(n.id) || "";
    const col = app.columnOf(slots, n);
    const room = roomFor(app, columns, col);
    const noun = nouns.get(n.id) || "";
    const { words, width } = labelWidth(app, n, place, app.labelRoom(place, columns.size), qualifier, noun);
    const box = boxAt(place.anchor, place.x, width);
    return { node: n, id: n.id, col, last, place, width, box, room, qualifier, noun,
             words: words.qualifier + words.label + (words.noun || ""),
             block: labelBlock(place),
             clearance: Math.min(box.left - room.from, room.to - box.right) };
  });
}

/** Glyphs overlapping a label, as "glyph's node on label's node". */
function glyphsOnLabels(app, fits, glyph = (d) => app.glyphOf(d)) {
  const slots = app.slotsOf(fits.map((f) => f.node), app.RENDER_TIERS);
  const last = fits.length ? fits[0].last : 0;
  const out = [];
  for (const f of fits) {
    if (!glyph(f.node)) continue;
    const at = app.glyphPlacement(slots, f.node, last, f.width);
    const r = app.GLYPH_R;
    for (const g of fits) {
      const apart = at.x + r <= g.box.left || at.x - r >= g.box.right ||
        at.y + r <= g.block.top || at.y - r >= g.block.bottom;
      if (!apart) out.push(`${f.id} on ${g.id}`);
    }
  }
  return out.sort();
}

/** Every pair of adjacent label blocks of one column that touch or overlap, as "above over below". */
function touchingStacks(fits) {
  const out = [];
  for (const col of new Set(fits.map((f) => f.col))) {
    const stacked = fits.filter((f) => f.col === col).sort((a, b) => a.block.top - b.block.top);
    for (let i = 1; i < stacked.length; i++) {
      if (stacked[i].block.top - stacked[i - 1].block.bottom <= 0) out.push(`${stacked[i - 1].id} over ${stacked[i].id}`);
    }
  }
  return out.sort();
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
 * The px cap on the chart figure: which property the card's cap comes
 * through, valued at what app.js hands it. Pins the declaration's shape, so
 * an equivalent clamp() goes red, the safe direction. Only a rule whose
 * whole selector is #chart-card or .chart-wrap counts; two is a refusal.
 * @param {string} css
 * @param {number | null} handed the px app.js set --chart-max to, or null
 */
function chartAllowance(css, handed) {
  // A rule boundary before the selector: start of text, a brace, or a comment end.
  const rulesOf = (sel) => [...css.matchAll(new RegExp(String.raw`(?:^|[{}]|\*\/)\s*${sel}\s*\{([^}]*)\}`, "g"))];
  const rules = rulesOf("#chart-card");
  if (rules.length === 0) return { px: null, why: "style.css declares no #chart-card rule" };
  const capped = rules.filter((r) => /--chart-room:/.test(r[1]));
  if (capped.length > 1) {
    return { px: null, why: `${capped.length} #chart-card rules declare --chart-room, and this file ` +
      `cannot say which applies at which width` };
  }
  const rule = capped[0] || rules[0];
  if (!/--chart-room:\s*min\(\s*var\(--chart-max[,)]/.test(rule[1])) {
    return { px: null, why: `#chart-card does not cap --chart-room with --chart-max: ${rule[1].trim()}` };
  }
  if (!rulesOf(String.raw`\.chart-wrap`).some((r) => /(^|[\s;])width:\s*var\(--chart-room\)/.test(r[1]))) {
    return { px: null, why: "#chart-card caps --chart-room but .chart-wrap does not take its width from it" };
  }
  if (handed === null) {
    return { px: null, why: "app.js set no --chart-max, so the stylesheet's cap falls back to 100%" };
  }
  // The cushion is read off its own declaration: the min()'s second operand is a var() too.
  const cushion = /--chart-cushion:\s*(\d+)px/.exec(rule[1]);
  if (!/calc\(100vw\s*-\s*var\(--chart-cushion\)\s*-\s*var\(--chart-panels\)\)/.test(rule[1])) {
    return { px: null, why: "#chart-card does not take its sub-cap width from 100vw less --chart-cushion and --chart-panels" };
  }
  return {
    px: handed,
    // Below the cap the chart gets 100vw minus this, and minus the panels beside it.
    cushion: cushion ? Number(cushion[1]) : null,
    why: "",
  };
}

/** Read off an outward label, not copied as a literal. */
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

/** Booted and drawn first: before the column, nodeRank has no fund order and ties to size. */
function spineApp() {
  return bootedApp({ checkedStem: "sankey" });
}

/** The spine laid out by the page's own layOut, as it draws it. */
function laidSpine(app) {
  return app.layOut(app.shown.doc);
}

describe("the spine's layout", () => {
  let app, graph;
  before(async () => {
    ({ app } = await spineApp());
    graph = laidSpine(app);
  });

  test("the chart draws at its pinned crossings and overlap", (t) => {
    const got = tangle(graph);
    const detail = `${got.crossings} crossings, ${usd(got.dollars)} overlapped ` +
      `(pinned ${DRAWN.crossings}, ${usd(DRAWN.overlapDollars)})`;
    t.diagnostic(detail);
    assert.deepEqual({ crossings: got.crossings, dollars: Math.round(got.dollars) },
      { crossings: DRAWN.crossings, dollars: DRAWN.overlapDollars }, detail);
  });

  // Every stack is reversed and mirrored; restacking restores each end
  // where the page placed it, no ribbon resized. Mutation: return early from
  // restackLinks and the crossings stay; drop its sorts and the ends stack
  // upside down.
  test("restackLinks stacks scrambled ribbons back to the chart's", (t) => {
    const own = laidSpine(app);
    const placed = own.links.map((l) => [l.y0, l.y1, l.width]);
    const invert = (links, y) => {
      const places = links.map((l) => l[y]);
      links.forEach((l, i) => { l[y] = places[places.length - 1 - i]; });
    };
    for (const n of own.nodes) {
      invert(n.sourceLinks.slice().sort((a, b) => a.y0 - b.y0), "y0");
      invert(n.targetLinks.slice().sort((a, b) => a.y1 - b.y1), "y1");
      n.sourceLinks.reverse();
      n.targetLinks.reverse();
    }
    const scrambled = tangle(own).crossings;
    app.restackLinks(own);
    const got = tangle(own);
    t.diagnostic(`${scrambled} crossings with every stack inverted, ${got.crossings} after restacking`);
    assert.notEqual(scrambled, DRAWN.crossings, "inverting the stacks changed nothing, so restacking is not tested");
    assert.equal(got.crossings, DRAWN.crossings);
    // Back where the page placed every end, and no ribbon resized.
    assert.deepEqual(own.links.map((l) => [l.y0, l.y1, l.width]), placed);
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

// The one width this file pins, the THREE-column one every figure was
// measured at. No wider width may be pinned: chartWidth(n) derives them.
const NARROW_DESIGN_WIDTH = 1320;

/**
 * Read off render()'s marks, so this measures labelPlacement's use. `class`
 * as an attribute (SVG className is no string); a null dy must be absent.
 * @param {any} app a page with a chart drawn
 * @param {Document} document
 * @param {any} laid the same chart's laid nodes
 */
function drawnPlacement(app, document, laid) {
  const slots = app.slotsOf(laid.nodes, app.RENDER_TIERS);
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

/** The spine's middle column, and the expansion at its finest grain, whose interior columns test the rules. */
describe("where a label is anchored", () => {
  let app, middle, crowded, drawn, allowance, columns;
  before(async () => {
    let document;
    ({ app, document } = await spineApp());
    const spine = laidSpine(app);
    const onSpine = labelFit(app, spine);
    middle = onSpine.filter((f) => f.col > 0 && f.col < f.last);
    const gap = outwardGap(onSpine);
    crowded = middle.filter((f) => outward(f, gap).right > f.room.to);
    app.expandAll();
    await settle();
    drawn = drawnPlacement(app, document, { nodes: app.laidNodes });
    columns = app.layoutColumns(app.shown.doc);
    const handed = document.documentElement.style.getPropertyValue("--chart-max");
    allowance = chartAllowance(stylesheet(), handed ? parseFloat(handed) : null);
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
      ? "the chart drew no marks, so nothing here read a placement off the page at all"
      : `${drawn.read} attribute(s) read off ${drawn.marks} drawn label(s) over ` +
        `${drawn.last + 1} column(s), each equal to labelPlacement's own answer` +
        (drawn.wrong.length ? `; ${drawn.wrong.length} disagree: ${drawn.wrong[0]}` : "");
    t.diagnostic(detail);
    assert.deepEqual({ drewNothing: drawn.marks === 0, read: drawn.read, wrong: drawn.wrong, interior: drawn.last > 2 },
      { drewNothing: false, read: drawn.marks * 4, wrong: [], interior: true }, detail);
  });

  // An equality, not a floor: card cap and svg stretch derive from the same
  // declarations, so slack is the defect.
  test("the stylesheet caps the chart's card at the opening chart's width every chart is scaled from", (t) => {
    const detail = allowance.px === null
      ? allowance.why
      : `style.css caps the chart's card through --chart-max, which app.js set to ${allowance.px}px, ` +
        `the opening chart's width, for the ${columns} columns it lays out at ${app.widthOf(columns)}px`;
    t.diagnostic(detail);
    assert.ok(columns > 3, "the chart split to its finest grain is no wider than the spine");
    assert.equal(app.chartWidth(3), NARROW_DESIGN_WIDTH);
    assert.equal(allowance.px, app.widthOf(app.openingColumns()), detail);
  });
});

// Mutation: restore chartWidth's single BAND formula and this goes red at
// five columns, the chart wider than the four-column one.
describe("the chart's width as columns open", () => {
  test("the width is held at HELD_COLUMNS' while the bands narrow to BAND_FLOOR, then grows at the floor", (t) => {
    const at = (n, band) => 2 * sankey.LABEL_GUTTER + band * (n - 1) + sankey.NODE_WIDTH * n;
    const held = at(sankey.HELD_COLUMNS, sankey.BAND);
    const rows = [];
    for (let n = 1; n <= 10; n++) rows.push({ n, width: sankey.chartWidth(n), band: n > 1 ? sankey.bandOf(n) : null });
    const detail = rows.map((r) => `${r.n}: ${r.width}px${r.band === null ? "" : ` band ${r.band.toFixed(1)}`}`).join(", ");
    t.diagnostic(detail);
    const wrong = rows.filter((r) => r.n > 1 && (
      r.n <= sankey.HELD_COLUMNS ? r.width !== at(r.n, sankey.BAND) || r.band !== sankey.BAND
        : r.band < sankey.BAND_FLOOR || r.width !== Math.max(held, at(r.n, sankey.BAND_FLOOR)) ||
          Math.abs(at(r.n, r.band) - r.width) > 1e-9)).map((r) => r.n);
    assert.deepEqual({ three: sankey.chartWidth(3), wrong, fiveHeld: sankey.chartWidth(5) === held },
      { three: NARROW_DESIGN_WIDTH, wrong: [], fiveHeld: true }, detail);
  });
});

/**
 * Same-word marks in one column, pinned per chart: the expansion hangs each
 * owner's row of a category under it, and a qualifier cannot tell them apart.
 * The synthetic city's two Grant funds and Horizons truncate alike
 * (fisc-yh3q, which empties this).
 */
const ALIKE = {
  "FY 2025-26": {
    "everything by primary splits, drawn out, cold": '3 group(s), 6 mark(s): "Grant…nt Fund: Services & Supplies", "Grant…rant Fund", "Services…zons: Services & Supplies"',
    "everything by primary splits, drawn out, from the root": '3 group(s), 6 mark(s): "Grant…nt Fund: Services & Supplies", "Grant…rant Fund", "Services…zons: Services & Supplies"',
  },
  "FY 2026-27": {
    "everything by primary splits, drawn out, cold": '3 group(s), 6 mark(s): "Grant…nt Fund: Services & Supplies", "Grant…rant Fund", "Services…zons: Services & Supplies"',
    "everything by primary splits, drawn out, from the root": '3 group(s), 6 mark(s): "Grant…nt Fund: Services & Supplies", "Grant…rant Fund", "Services…zons: Services & Supplies"',
  },
};

/**
 * Glyphs drawn over a neighbour's label, pinned per chart: the synthetic
 * city's short General Fund marks beside the special revenue group's label
 * (fisc-dnz0, which empties this).
 */
const GLYPHED = {
  "FY 2025-26": {
    "everything by primary splits, cold": ["revenue/use-of-money-and-property on fund-group/special-revenue"],
    "everything by primary splits, drawn out, cold": ["revenue/contributions-outsourced on fund-group/special-revenue"],
  },
  "FY 2026-27": {
    "everything by primary splits, cold": ["revenue/use-of-money-and-property on fund-group/special-revenue"],
    "everything by primary splits, drawn out, cold": ["revenue/contributions-outsourced on fund-group/special-revenue"],
  },
};

/** Stacked labels per chart, pinned not bounded: sankey.grow counts marks, not lines. */
const OVERLAPPED = {
  "FY 2025-26": {},
  "FY 2026-27": {},
};

/** Fewest name characters a cut interior label keeps at BAND_FLOOR (a member's noun counts). */
const FLOOR_KEEPS = 10;

/** Interior labels at BAND_FLOOR: how many cut, and the worst kept. */
function atFloor(app, fits) {
  const room = app.BAND_FLOOR + app.NODE_WIDTH;
  let cut = 0;
  let fewest = null;
  const inner = fits.filter((f) => f.place.anchor === "middle");
  for (const f of inner) {
    const value = "  " + app.fmtShortSigned(app.markCents(f.node));
    const flags = app.nodeFlags(f.node);
    const w = app.labelWords(room, f.qualifier, f.node.label, (ws) => app.labelAdvance(ws, value, flags), f.noun);
    if (!w.cut) continue;
    cut++;
    const kept = (w.qualifier + w.label).length - 1 + f.noun.length;
    if (!fewest || kept < fewest.kept) fewest = { kept, words: w.qualifier + w.label + w.noun, id: f.id };
  }
  return { inner: inner.length, cut, fewest };
}

for (const label of Object.keys(ALIKE)) {
  describe(`${label}: the root and every split the page offers, measured for what a label does to its neighbours`, () => {
    let views, overlapped, alike, wider, floor, glyphed, glyphs, drawnApart, drawnCut;
    before(async () => {
      // Found by label, so a year the page no longer ships fails by name.
      const year = pageFixture().config.years.find((y) => y.label === label);
      if (!year) throw new Error(`the pinned page ships no year labelled ${JSON.stringify(label)}`);
      const { app } = await bootedApp({ checkedStem: year.stem });
      overlapped = {};
      alike = {};
      wider = {};
      floor = {};
      glyphed = {};
      glyphs = 0;
      views = 0;
      // A state's glyphs are its own frontier's, not the chart on screen's.
      const visit = (where, graph, glyph = (d) => app.glyphOf(d)) => {
        views++;
        const fits = labelFit(app, graph);
        if (where.startsWith("everything")) floor[where] = atFloor(app, fits);
        const touching = touchingStacks(fits);
        if (touching.length) overlapped[where] = touching;
        glyphs += fits.filter((f) => glyph(f.node)).length;
        const onLabels = glyphsOnLabels(app, fits, glyph);
        if (onLabels.length) glyphed[where] = onLabels;
        const over = fits.filter((f) => f.clearance < 0);
        if (over.length) {
          const worst = tightest(over);
          wider[where] = `${over.length}, worst ${worst.id} by ${px(worst.clearance)} in ${JSON.stringify(worst.words)}`;
        }
        const groups = new Map();
        for (const f of fits) groups.set(f.col + "\0" + f.words, (groups.get(f.col + "\0" + f.words) || 0) + 1);
        const same = [...groups.entries()].filter((e) => e[1] > 1);
        if (same.length) {
          alike[where] = `${same.length} group(s), ${same.reduce((n, e) => n + e[1], 0)} mark(s): ` +
            same.map((e) => JSON.stringify(e[0].split("\0")[1])).sort().join(", ");
        }
      };
      visit("the root", { nodes: app.laidNodes });
      // Each split alone, Expand all, and drawn out, laid cold (a deep link)
      // and against the root's boxes (a click); other routes lie between.
      const fromRoot = app.boxesOf(app.laidNodes);
      for (const st of expansionStates(app, app.fetched)) {
        if (!st.whole.size || st.where.startsWith("everything")) {
          const glyph = (d) => app.glyphAt(app.fetched, st.frontier, d);
          visit(st.where + ", cold", layOutState(app, st, null), glyph);
          visit(st.where + ", from the root", layOutState(app, st, fromRoot), glyph);
        }
      }
      // render()'s tspans against labelFit's words, on a fresh page, since
      // layOutState leaves this one holding its last chart.
      const { app: fresh, document } = await bootedApp({ checkedStem: year.stem });
      if (!fresh.expandAll()) throw new Error("Expand all split nothing");
      await settle();
      const fitted = new Map(labelFit(fresh, { nodes: fresh.laidNodes }).map((f) => [f.id, f.words]));
      drawnApart = [];
      drawnCut = 0;
      for (const m of document.querySelectorAll("#chart g.node")) {
        const text = [...m.children].find((c) => c.tagName === "text" && c.getAttribute("class") === "halo");
        const said = text ? [...text.children].slice(0, 3).map((c) => c.textContent).join("") : "";
        const id = m.__data__.id;
        if (said !== fitted.get(id)) drawnApart.push(`${id}: drew ${JSON.stringify(said)}, measured ${JSON.stringify(fitted.get(id))}`);
        if (said !== m.__data__.label && [...m.children].some((c) => c.tagName === "title")) drawnCut++;
      }
    });

    // Mutation: fit sankey.js's drawn words at a wider room than labelRoom,
    // and the drawn words part from the measured ones.
    test("the words render() draws are the words this walk measures", (t) => {
      t.diagnostic(`${drawnCut} drawn label(s) cut after Expand all`);
      assert.ok(drawnCut > 0, "Expand all drew no cut label, so this compares nothing that fitting changes");
      assert.deepEqual(drawnApart, []);
    });

    // fisc-8cx0: every split's labels against their room, not four frontiers'.
    test("no label is drawn wider than its room", (t) => {
      const detail = `${views} chart(s) walked; ` + JSON.stringify(wider);
      t.diagnostic(detail);
      assert.deepEqual({ walkedNothing: views === 0, wider }, { walkedNothing: false, wider: {} }, detail);
    });

    // Mutation: set BAND_FLOOR to 0 and this goes red, every cut label
    // keeping two characters of its name.
    test(`at BAND_FLOOR every cut interior label keeps ${FLOOR_KEEPS} characters of its name, with every split open`, (t) => {
      const short = Object.entries(floor).filter(([, m]) => m.fewest && m.fewest.kept < FLOOR_KEEPS).map(([w]) => w);
      const detail = `BAND_FLOOR ${sankey.BAND_FLOOR}px: ` + Object.entries(floor).map(([w, m]) =>
        `${w}: ${m.cut} of ${m.inner} interior labels cut` +
        (m.fewest ? `, fewest kept ${m.fewest.kept} in ${JSON.stringify(m.fewest.words)} (${m.fewest.id})` : "")).join("; ");
      t.diagnostic(detail);
      assert.deepEqual({ walked: Object.keys(floor).length, short }, { walked: 4, short: [] }, detail);
    });

    // Mutation: drop glyphPlacement's arm for a short interior rect and this
    // goes red, expenditure/capital-outlay's ⊖ on its own label.
    test("the ⊕ and ⊖ drawn over a label are the ones pinned", (t) => {
      const detail = `${views} chart(s) walked, ${glyphs} glyph(s) drawn; ` + JSON.stringify(glyphed);
      t.diagnostic(detail);
      assert.deepEqual({ drewGlyphs: glyphs > 0, glyphed }, { drewGlyphs: true, glyphed: GLYPHED[label] }, detail);
    });

    test("the labels stacked on one another are the ones pinned", (t) => {
      const detail = `${views} chart(s) walked; ` + JSON.stringify(overlapped);
      t.diagnostic(detail);
      assert.deepEqual({ walkedNothing: views === 0, overlapped },
        { walkedNothing: false, overlapped: OVERLAPPED[label] }, detail);
    });

    test("the charts that draw two marks of one column in the same words are the ones pinned", (t) => {
      t.diagnostic(JSON.stringify(alike));
      assert.deepEqual(alike, ALIKE[label]);
    });
  });
}

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

  // Mutation: drop the node rule's outline: none, or the focus rule's
  // stroke-dasharray: none, and the matching arm goes red.
  test("a focused mark is ringed on its bar, not its <g>'s box, and undashed though derived", () => {
    assert.match(ruleFor("svg.sankey .node:focus-visible"), /outline:\s*none/,
      "a focused node takes the global outline, which rings its <g>'s union box with the label in the gutter");
    assert.match(ruleFor("svg.sankey .node:focus-visible rect"), /stroke-dasharray:\s*none/,
      "the focus rule keeps the derived rule's dashes, so a focused derived mark rings dashed");
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

// The expansion laid out from the schedule, so every frontier is reached
// without a gesture: foldDocument, expansionOf, then layOutAt at the index
// overview's sankey.grow. Every assert is over keys and numbers: a failing
// assert over a laid d3 graph formats all of it, which this host cannot hold.

/**
 * Per year and frontier: slots, nodes and ribbons, the canvas and its
 * sub-pixel ribbons, and labels wider than their room (value in, flags
 * out), over the synthetic city. Pinned, not bounded, so a change to the
 * slots, the fold or the canvas shows here as a number.
 */
const EXPANSION_CLAIMED = {
  "FY 2025-26": {
    "root": { slots: "12/6/6", figures: "24 nodes, 36 ribbons; 820px canvas, 2 under 1px",
      labels: "0 wider than their room" },
    "fund-group/general by funds": { slots: "12/6/1/25/6", figures: "50 nodes, 108 ribbons; 1000px canvas, 2 under 1px",
      labels: "0 wider than their room" },
    "everything by primary splits": { slots: "37/12/6/17/25/6/31",
      figures: "134 nodes, 206 ribbons; 1480px canvas, 15 under 1px",
      labels: "0 wider than their room" },
    "everything by primary splits, every tail drawn out": { slots: "47/12/6/37/29/6/98",
      figures: "235 nodes, 338 ribbons; 3920px canvas, 13 under 1px",
      labels: "0 wider than their room" },
  },
  "FY 2026-27": {
    "root": { slots: "12/6/6", figures: "24 nodes, 36 ribbons; 820px canvas, 2 under 1px",
      labels: "0 wider than their room" },
    "fund-group/general by funds": { slots: "12/6/1/25/6", figures: "50 nodes, 108 ribbons; 1000px canvas, 2 under 1px",
      labels: "0 wider than their room" },
    "everything by primary splits": { slots: "37/12/6/18/25/6/31",
      figures: "135 nodes, 208 ribbons; 1480px canvas, 15 under 1px",
      labels: "0 wider than their room" },
    "everything by primary splits, every tail drawn out": { slots: "47/12/6/37/29/6/98",
      figures: "235 nodes, 338 ribbons; 3920px canvas, 13 under 1px",
      labels: "0 wider than their room" },
  },
};

/** The expansion's frontiers, each from the schedule's own helpers. */
function expansionFrontiers(doc) {
  const root = core.rootFrontier(doc);
  const all = core.expandAllFrom(doc, root).frontier;
  const none = new Set();
  const every = new Set(doc.splits.map((sp) => sp.node + "\u001f" + sp.key));
  return [
    { name: "root", frontier: root, whole: none },
    { name: "fund-group/general by funds", frontier: core.expand(doc, root, "fund-group/general", "funds"), whole: none },
    { name: "everything by primary splits", frontier: all, whole: none },
    { name: "everything by primary splits, every tail drawn out", frontier: all, whole: every },
  ];
}

/** As labelFit, over the unfolded `doc`'s qualifiers and nouns. */
function expansionLabels(doc, laid, slots, columns) {
  const xs = new Map();
  for (const n of laid.nodes) if (!xs.has(slots.get(n.id))) xs.set(slots.get(n.id), { x0: n.x0, x1: n.x1 });
  const last = Math.max(...xs.keys());
  const qualifiers = sankey.labelQualifiers(doc, slots, laid.nodes);
  const nouns = sankey.labelNouns(doc, laid.nodes, (id) => core.memberNoun(doc, id));
  const width = sankey.chartWidth(columns);
  return laid.nodes.map((n) => {
    const col = sankey.columnOf(slots, n);
    const place = sankey.labelPlacement(slots, n, last);
    const value = "  " + core.fmtShortSigned(sankey.markCents(n));
    const measure = (w) => sankey.labelAdvance(w, value, "");
    const words = sankey.labelWords(sankey.labelRoom(place, columns), qualifiers.get(n.id) || "", n.label, measure,
      nouns.get(n.id) || "");
    const box = boxAt(place.anchor, place.x, measure(words));
    const me = xs.get(col);
    const before = xs.get(col - 1);
    const after = xs.get(col + 1);
    const room = {
      from: col === 0 || !before ? 0 : (before.x1 + me.x0) / 2,
      to: col >= last || !after ? width : (me.x1 + after.x0) / 2,
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
        // At no room: the canvas the hint alone sizes, which grow is read off.
        const height = sankey.canvasHeight(chart, drawn.nodes, 0);
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
        const over = expansionLabels(doc, laid, slots, held.length).filter((l) => l.clearance < 0)
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
        // The General Fund's division-by-object cells: each a ribbon of its
        // own at its cents and facts, repeated by its row's hub leg.
        if (f.whole.size || f.name === "everything by primary splits") {
          const divisions = new Set(core.splitsOf(doc, "fund/100").find((sp) => sp.key === "divisions").members);
          const cells = doc.links.filter((l) => divisions.has(l.source) && !index.get(l.target).alternative &&
            index.get(l.target).role === "object_category");
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

  // An extend member is drawn in the slot after its parent's: the group
  // stays where it stood and its one fund stands after it.
  test("the General Fund group opened by funds draws fund/100 in the slot after the group's", (t) => {
    for (const [label, m] of measured) {
      t.diagnostic(`${label}: at the root ${m.root.general}; opened ${m["fund-group/general by funds"].general}`);
      assert.equal(m.root.general, "fund-group/general in 1, fund/100 not drawn", label);
      assert.equal(m["fund-group/general by funds"].general, "fund-group/general in 1, fund/100 in 2", label);
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

  // FY 2026-27's services and supplies ranks a remainder past the cap in
  // both its splits, so this goes red where a tail folds one.
  test("no ribbon into or out of a tail is drawn inferred, since a tail folds no remainder", (t) => {
    const want = { "FY 2025-26": [], "FY 2026-27": [] };
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

// d3-sankey draws a column per step of the longest path; here the longest
// is two steps, so d3 alone draws three columns for five slots.
describe("a slot no drawn path reaches is laid out in its own column", () => {
  before(() => loadD3());
  const node = (id, slot) => ({ id, label: id, tier: 0, slot, role: "", source_note: "" });
  const link = (source, target) => ({ source, target, value_cents: 100, kind: "external", fact_ids: [] });
  const ctx = { tiers: [], columns: 5, groupOf: () => "", placeOf: () => 0 };

  test("five slots over a longest path of two steps draw five columns, each node in its slot", (t) => {
    const drawn = {
      projection: "hand-built",
      nodes: [node("A", 0), node("B", 10), node("C", 11), node("D", 20), node("E", 21)],
      links: [link("A", "B"), link("B", "C"), link("A", "D"), link("D", "E")],
    };
    const laid = sankey.layOutAt(drawn, ctx, 400);
    const layers = laid.nodes.map((n) => `${n.id}:${n.layer}`).join(" ");
    t.diagnostic(`laid ${layers}; ${laid.links.length} ribbon(s)`);
    assert.equal(layers, "A:0 B:1 C:2 D:3 E:4");
    // The scaffold is gone once d3 has placed the nodes: no ribbon is drawn
    // that the document does not hold, and no node keeps one in its lists.
    assert.equal(laid.links.map((l) => `${l.source.id}>${l.target.id}`).join(" "), "A>B B>C A>D D>E");
    assert.ok(laid.nodes.every((n) => n.sourceLinks.every((l) => laid.links.includes(l)) &&
      n.targetLinks.every((l) => laid.links.includes(l))), "a node still holds a scaffold link");
    assert.ok(laid.links.every((l) => l.width > 0), "a drawn ribbon has no width");
  });

  // B > C closes a cycle with C > B: no forward path to scaffold, so the
  // clamp check refuses.
  test("a ribbon running backward against the slots is still refused, by the clamp check", (t) => {
    const drawn = {
      projection: "hand-built",
      nodes: [node("A", 0), node("B", 10), node("C", 20)],
      links: [link("A", "B"), link("C", "B")],
    };
    let refused = "";
    try {
      sankey.layOutAt(drawn, ctx, 400);
    } catch (e) {
      refused = e.message;
    }
    t.diagnostic(refused);
    assert.ok(refused.includes("C belongs in column 2 and no ribbon drawn reaches that far"), refused);
  });
});

// labelWords over hand-built words, at the room an end column has: what
// the walks above measure over the pinned columns, said of one label each.
describe("an outward label fitted to its gutter", () => {
  const room = sankey.LABEL_GUTTER - 10;
  const fit = (qualifier, label) => sankey.labelWords(room, qualifier, label,
    (w) => sankey.labelAdvance(w, "  $1.2M", ""));
  const drawn = (w) => w.qualifier + w.label;

  test("two owners that share a prefix are cut apart, each keeping its category", (t) => {
    const a = fit("", "County Meas BB-Bike/Pedestrian: Services & Supplies");
    const b = fit("", "County Meas BB-Local St & Rd: Services & Supplies");
    t.diagnostic(`${JSON.stringify(drawn(a))} and ${JSON.stringify(drawn(b))}`);
    assert.deepEqual({ cut: [a.cut, b.cut], apart: drawn(a) !== drawn(b),
      kept: [drawn(a), drawn(b)].every((w) => w.endsWith(": Services & Supplies") && w.includes("…")) },
    { cut: [true, true], apart: true, kept: true });
  });

  test("a qualifier is drawn inline, and is what is cut where the label has no owner of its own", (t) => {
    const roomy = fit("Licenses and Permits", "Lines");
    const tight = sankey.labelWords(240, "Licenses and Permits", "6 smaller lines",
      (w) => sankey.labelAdvance(w, "  $1.2M", ""));
    t.diagnostic(`${JSON.stringify(roomy)} and ${JSON.stringify(tight)}`);
    assert.deepEqual({ roomy, cut: tight.cut, cutQualifier: tight.qualifier.endsWith(": ") && tight.qualifier.includes("\u2026"),
      label: tight.label }, {
      roomy: { qualifier: "Licenses and Permits: ", label: "Lines", noun: "", cut: false },
      cut: true, cutQualifier: true, label: "6 smaller lines",
    });
  });

  // Mutation: draw labelWords' noun as "" and this goes red.
  test("a noun is drawn after the label, kept whole while the label is cut before it", (t) => {
    const roomy = sankey.labelWords(room, "", "Intergovernmental", (w) => sankey.labelAdvance(w, "  $1.2M", ""), "line");
    const tight = sankey.labelWords(150, "", "Intergovernmental", (w) => sankey.labelAdvance(w, "  $1.2M", ""), "line");
    t.diagnostic(`${JSON.stringify(roomy)} and ${JSON.stringify(tight)}`);
    assert.deepEqual({ roomy, noun: tight.noun, cut: tight.cut, ellipsis: tight.label.includes("\u2026") },
      { roomy: { qualifier: "", label: "Intergovernmental", noun: " (line)", cut: false },
        noun: " (line)", cut: true, ellipsis: true });
  });

  // fisc-ylxh.6. Mutation: hand the whole line to the label tspan, as one
  // owner, and this goes red with the qualifier and the noun empty.
  test("a label cut whole keeps each tspan's own words", (t) => {
    const lead = "Licenses and Permits: ";
    const label = "An Unusually Long Revenue Line Without Any Owner Named Before It";
    const w = sankey.labelWords(200, "Licenses and Permits", label, (ws) => sankey.labelAdvance(ws, "  $1.2M", ""), "line");
    const whole = lead + label + " (line)";
    t.diagnostic(JSON.stringify(w));
    const joined = w.qualifier + w.label + w.noun;
    const at = joined.indexOf("\u2026");
    assert.deepEqual({
      cut: w.cut,
      qualifier: w.qualifier !== "" && lead.startsWith(w.qualifier.replace("\u2026", "")),
      noun: w.noun !== "" && " (line)".endsWith(w.noun.replace("\u2026", "")),
      joined: whole.startsWith(joined.slice(0, at)) && whole.endsWith(joined.slice(at + 1)),
    }, { cut: true, qualifier: true, noun: true, joined: true });
  });

  test("a label with no owner is cut in its middle and fits", (t) => {
    const label = "An Unusually Long Revenue Line Without Any Owner Named Before It";
    const w = fit("", label);
    t.diagnostic(JSON.stringify(w));
    assert.deepEqual({ cut: w.cut, starts: w.label.startsWith("An"), ends: w.label.endsWith("It"),
      fits: sankey.labelAdvance(w, "  $1.2M", "") <= room }, { cut: true, starts: true, ends: true, fits: true });
  });
});

// Every pair of two nodes' extend splits, revealed and opened as a reader
// would, shaped, filtered and laid out by the page's calls, each laid node
// in its slot's column. Nothing is rendered (that is render.test.mjs's).
//
// Mutation: drop scaffoldOf's result in layOutAt and pairs refuse with
// "belongs in column N and no ribbon drawn reaches that far".
describe("every pair of extend splits the schedule declares, laid out", () => {
  for (const year of pageFixture().config.years) {
    test(`${year.label}: every pair lays out, each node in its slot's column`, async (t) => {
      const { app } = await bootedApp({ checkedStem: year.stem });
      const doc = app.fetched;
      const root = app.rootFrontier(doc);
      const extend = doc.splits.filter((sp) => sp.form === "extend");
      const faults = [];
      let pairs = 0;
      let apart = 0;
      for (let i = 0; i < extend.length; i++) {
        for (let j = i + 1; j < extend.length; j++) {
          const [a, b] = [extend[i], extend[j]];
          // Two splits of one node are alternatives: a node splits one way
          // at a time, so they are never open together.
          if (a.node === b.node) continue;
          const where = `${a.node} by ${a.key} with ${b.node} by ${b.key}`;
          let frontier;
          try {
            const first = app.expand(doc, app.reveal(doc, root, a.node), a.node, a.key);
            frontier = app.expand(doc, app.reveal(doc, first, b.node), b.node, b.key);
          } catch (e) {
            faults.push(`${where}: cannot be opened: ${e.message}`);
            continue;
          }
          // Revealing b can split a's ancestor another way and take a's node
          // off; one still drawn but split another way is a fault.
          if (!frontier.has(a.node)) {
            apart++;
            continue;
          }
          pairs++;
          const open = [a, b].filter((sp) => app.openSplitOf(doc, frontier, sp.node) !== sp.key);
          if (open.length) {
            faults.push(`${where}: opening both leaves ${open.map((sp) => sp.node).join(", ")} split another way`);
            continue;
          }
          let graph;
          try {
            const shaped = shapedAt(app, frontier);
            graph = app.layOut(app.filterFor(shaped).doc);
          } catch (e) {
            faults.push(`${where}: ${e.message}`);
            continue;
          }
          const slots = app.slotsOf(graph.nodes, app.RENDER_TIERS);
          for (const n of graph.nodes) {
            if (n.layer !== slots.get(n.id)) faults.push(`${where}: ${n.id} in ${n.layer}, slot ${slots.get(n.id)}`);
          }
        }
      }
      t.diagnostic(`${year.label}: ${extend.length} extend splits, ${pairs} pair(s), ${apart} never drawn together, ` +
        `${faults.length} fault(s)`);
      assert.ok(pairs > 0, "the schedule declares no pair of extend splits, so nothing here is tested");
      // Pinned: no pair takes the other's node off. One that does is a new
      // schedule to pin, or reveal folding what it should not.
      assert.equal(apart, 0, "pairs skipped as never drawn together");
      assert.deepEqual(faults.slice(0, 5), [], `${faults.length} fault(s)`);
    });
  }
});
