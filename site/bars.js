// @ts-check
/**
 * fisc — the bars form: the parts of an opened node, one ranked bar each.
 * Which of the step document's links join the opened node to its parts at
 * the listed tier, the order they rank in, and where each bar and each of
 * its pieces stands. Every figure it draws is a sum over cited links. It
 * reads of a step the generic fields and `step.bars`, and nothing of any
 * other form's.
 *
 * A BAR IS A NODE AND ITS PIECES ARE LINKS: each link joining a part to the
 * opened node is drawn as one piece of that part's bar, a `path.link` bound
 * to the link, so a piece is cited, followed and pinned exactly as a ribbon
 * is, and the bar's row is a `g.node` bound to the part.
 *
 * NO CAP AND NO FOLD: a ranked bar is legible at any share, which is what
 * this form is for. Nothing here keeps a flank, carries a residual or draws a
 * gap; Go refuses a bars step declaring one.
 *
 * STATELESS BY RULE, as core.js is: no module-level `let`, no global read at
 * import. d3 is read through globalThis when render asks for it. A type
 * named here is one of app.js's typedefs.
 */

import { fmtShortSigned, isLink, say, withinNode } from "./core.js";

/** How tall one bar is drawn, in px. */
export const BAR_HEIGHT = 16;

/** One row of the list, bar and the space under it, in px. */
export const BAR_ROW = 24;

/** The margin above the first row and below the last, in px. */
export const BAR_MARGIN = 12;

/** Room left of the bars for the labels, in px. */
export const BAR_LABELS = 300;

/** The length the largest bar is drawn at, in px. */
export const BAR_SPAN = 480;

/** Room right of the longest bar for its figure and share, in px. */
export const BAR_FIGURES = 200;

/**
 * How wide a bar list is laid out, in px, at any column count: one list is
 * one column wide however wide the reader's window is.
 */
export const BARS_WIDTH = BAR_LABELS + BAR_SPAN + BAR_FIGURES;

/** The surface gap between two pieces of one bar, in px. */
export const PIECE_GAP = 1;

/**
 * The opened node and its parts at `tier` in `doc`, with every link joining a
 * part to the opened node: what a bars rung draws, unfolded. A part no link
 * joins to the opened node has no figure in this document and is not listed.
 *
 * REFUSED, NOT DRAWN: an opened node the document does not carry, a part
 * joined in both directions, whose bar would add money in to money out, and a
 * negative link, which a bar's length cannot say.
 *
 * @param {FiscProjection} doc
 * @param {string} opened
 * @param {number} tier
 * @returns {FiscProjection}
 */
export function barsOf(doc, opened, tier) {
  const byID = new Map(doc.nodes.map((n) => [n.id, n]));
  const head = byID.get(opened);
  if (!head) throw new Error("cannot list the parts of " + opened + ": " + doc.projection + " carries no such node");
  const inside = withinNode(doc, opened);
  const part = (/** @type {string} */ id) => id !== opened && inside.has(id) && (byID.get(id) || { tier: -1 }).tier === tier;
  /** @type {Map<string, boolean>} whether each part's links leave the opened node */
  const out = new Map();
  const links = doc.links.filter((l) => {
    const leaves = l.source === opened && part(l.target);
    const arrives = l.target === opened && part(l.source);
    if (!leaves && !arrives) return false;
    const id = leaves ? l.target : l.source;
    if (out.has(id) && out.get(id) !== leaves) {
      throw new Error("cannot list " + id + " as a bar: " + doc.projection + " joins it to " + opened +
        " in both directions, and one bar cannot add what enters to what leaves");
    }
    if (l.value_cents < 0) {
      throw new Error("cannot list " + id + " as a bar: its link to " + opened + " is " + l.value_cents +
        " cents, and a bar's length cannot say a negative figure");
    }
    out.set(id, leaves);
    return true;
  });
  return Object.assign({}, doc, {
    nodes: [head].concat(doc.nodes.filter((n) => out.has(n.id))),
    links: links,
  });
}

/**
 * A laid mark's figure: a link's own cents, and a node's the sum of every
 * link it is drawn with, which for a part is its bar and for the opened node
 * the whole list.
 * @param {LaidLink | LaidNode} d
 * @returns {number}
 */
export function barCents(d) {
  if (isLink(d)) return /** @type {LaidLink} */ (d).value_cents;
  const n = /** @type {LaidNode} */ (d);
  let cents = 0;
  for (const l of n.sourceLinks.concat(n.targetLinks)) cents += l.value_cents;
  return cents;
}

/**
 * A bar's share of every bar laid with it, as a percentage formatted, or ""
 * for the opened node's own row and for a list of one, which is 100% by
 * construction.
 *
 * IT IS ARITHMETIC: no page prints it. A ceiling as well as a floor, so the
 * largest of two bars never rounds to the whole its sibling denies.
 * @param {LaidNode} d
 * @param {LaidNode[]} laid
 * @returns {string}
 */
export function barPercent(d, laid) {
  if (d.layer !== 1) return "";
  const bars = laid.filter((n) => n.layer === 1);
  if (bars.length < 2) return "";
  const total = bars.reduce((sum, n) => sum + barCents(n), 0);
  if (!total) return "";
  const pct = (100 * barCents(d)) / total;
  return pct < 0.1 ? "<0.1" : pct > 99.9 ? ">99.9" : pct.toFixed(1);
}

/**
 * Lays a bars rung out: the opened node's row first, then one row per part
 * ranked by its figure, largest first, ties by label and then id so the
 * order is stable. The largest bar is drawn BAR_SPAN long and every other at
 * its figure's share of that; each bar's pieces are its links, largest
 * first, end to end. A node is laid with depth and layer 0 for the opened
 * node and 1 for a part.
 * @param {FiscProjection} drawn
 * @param {number} openedTier
 * @returns {{nodes: LaidNode[], links: LaidLink[]}}
 */
export function layOutBars(drawn, openedTier) {
  const nodes = drawn.nodes.map((n) => /** @type {LaidNode} */ (Object.assign({}, n, {
    x0: 0, x1: 0, y0: 0, y1: 0, value: 0, sourceLinks: [], targetLinks: [], depth: 0, layer: 0,
  })));
  const byID = new Map(nodes.map((n) => [n.id, n]));
  const links = drawn.links.map((l, i) => {
    const source = byID.get(l.source);
    const target = byID.get(l.target);
    if (!source || !target) throw new Error("cannot lay out the link " + l.source + " to " + l.target + ": an end is not drawn");
    const laid = /** @type {LaidLink} */ (/** @type {unknown} */ (Object.assign({}, l, {
      source, target, value: l.value_cents, width: BAR_HEIGHT, y0: 0, y1: 0, index: i, x0: 0, x1: 0,
    })));
    source.sourceLinks.push(laid);
    target.targetLinks.push(laid);
    return laid;
  });
  const heads = nodes.filter((n) => n.tier === openedTier);
  if (heads.length !== 1) {
    throw new Error("cannot lay out a bar list with " + heads.length + " opened nodes at tier " + openedTier);
  }
  const head = heads[0];
  const bars = nodes.filter((n) => n !== head);
  for (const n of nodes) n.value = barCents(n);
  bars.sort((a, b) => b.value - a.value || String(a.label).localeCompare(String(b.label)) ||
    (a.id < b.id ? -1 : a.id > b.id ? 1 : 0));
  const largest = bars.reduce((most, n) => Math.max(most, n.value), 0);
  const scale = largest > 0 ? BAR_SPAN / largest : 0;
  head.x0 = BAR_LABELS;
  head.x1 = BAR_LABELS + BAR_SPAN;
  head.y0 = BAR_MARGIN;
  head.y1 = BAR_MARGIN + BAR_HEIGHT;
  bars.forEach((n, k) => {
    n.depth = 1;
    n.layer = 1;
    n.x0 = BAR_LABELS;
    n.x1 = BAR_LABELS + n.value * scale;
    n.y0 = BAR_MARGIN + BAR_ROW * (k + 1);
    n.y1 = n.y0 + BAR_HEIGHT;
    let at = n.x0;
    const pieces = n.sourceLinks.concat(n.targetLinks).sort((a, b) => b.value - a.value || a.index - b.index);
    for (const l of pieces) {
      const piece = /** @type {any} */ (l);
      piece.x0 = at;
      piece.x1 = at + l.value * scale;
      piece.y0 = piece.y1 = (n.y0 + n.y1) / 2;
      at = piece.x1;
    }
  });
  return { nodes: [head].concat(bars), links: links };
}

/**
 * The bars form's renderer, as app.js's FORMS holds it under "bars". It
 * reads of a step the generic fields and `step.bars`, and nothing of any
 * other form's hints; site/bars.test.mjs wraps the hints in a recording Proxy
 * and holds every read to this file.
 * @type {FormRenderer}
 */
export const BARS = Object.freeze({
  form: "bars",

  /** The opened tier and the tier listed, which are the two rows a list draws. */
  tiersOf(chart) {
    const step = /** @type {FiscDrillStep} */ (chart);
    return step.bars ? [step.from, step.bars.tier] : [];
  },

  /** No cap: a ranked bar is legible at any share. */
  caps() {
    return [];
  },

  /** The same two tiers at every budget. */
  columns(chart) {
    return BARS.tiersOf(chart);
  },

  /** One width at every column count. */
  width() {
    return BARS_WIDTH;
  },

  /** The figure a laid mark prints: barCents. */
  cents(d) {
    return barCents(d);
  },

  /** A bar's share of the list, as a sentence in the page's words. */
  share(d, laid) {
    const pct = barPercent(d, laid);
    if (!pct) return "";
    const head = laid.find((n) => n.layer === 0);
    return say("bars_share", { share: pct, whole: head ? head.label : "" });
  },

  /** barsOf refuses a negative link, so no bar is a reduction. */
  reduction() {
    return false;
  },

  /** A bar is no cross-tab total. */
  crossTab() {
    return false;
  },

  /** No bar carries a reduction to say. */
  reductionNote() {
    return "";
  },

  /** Whether some link of the document joins `id` to a part at the listed tier. */
  offers(step, doc, onScreen, id) {
    const inside = withinNode(doc, id);
    const tierOf = new Map(doc.nodes.map((n) => [n.id, n.tier]));
    const part = (/** @type {string} */ end) => end !== id && inside.has(end) && tierOf.get(end) === step.bars.tier;
    return doc.links.some((l) => (l.source === id && part(l.target)) || (l.target === id && part(l.source)));
  },

  /**
   * The opened node and its listed parts (barsOf). Refuses the overview,
   * which has opened nothing, and a node with nothing to list.
   */
  shape(doc, rung) {
    if (!rung) throw new Error("the bars form lists the parts of an opened node, and the overview has opened none");
    const listed = barsOf(doc, rung.id, rung.step.bars.tier);
    if (!listed.links.length) {
      throw new Error("cannot list the parts of " + rung.id + ": " + doc.projection +
        " joins it to nothing at tier " + rung.step.bars.tier);
    }
    return listed;
  },

  /** A bar's pieces are drawn at the bar's thickness, so none is a hairline. */
  hairlines() {
    return 0;
  },

  /** layOutBars at the opened tier, the first of the columns on screen. */
  layOut(drawn, ctx) {
    return layOutBars(drawn, ctx.tiers[0]);
  },

  /**
   * Draws a laid list into ctx.layer: a row per mark, the opened node's
   * first, each a focusable `g.node` with its label left of the bars and its
   * figure and share right of its bar, then every bar's pieces as
   * `path.link`s over the rows. The classes, descriptions, gestures and the
   * figure each row says (ctx.figure) are the page's. Paints nothing.
   */
  render(graph, ctx) {
    const svg = ctx.svg;
    const on = ctx.on;
    const bottom = graph.nodes.reduce((most, n) => Math.max(most, n.y1), 0);
    svg.attr("viewBox", "0 0 " + BARS_WIDTH + " " + (bottom + BAR_MARGIN));

    // ROWS BEFORE PIECES, so Tab walks the list before the pieces of it.
    const gNodes = ctx.layer.append("g").attr("class", "nodes");
    const gLinks = ctx.layer.append("g").attr("class", "links");

    const node = gNodes.selectAll("g")
      .data(graph.nodes)
      .join("g")
      .attr("class", /** @param {LaidNode} d */ (d) => ctx.classes.node(d))
      .attr("tabindex", 0)
      .attr("role", "button")
      // aria-pressed is the isolation, as on the Sankey's marks.
      .attr("aria-pressed", "false")
      .attr("aria-label", /** @param {LaidNode} d */ (d) => ctx.describe.node(d))
      .attr("aria-keyshortcuts", "Enter Space")
      .on("pointerenter", /** @param {PointerEvent} e @param {LaidNode} d */ (e, d) => on.tip(e, d))
      .on("pointermove", /** @param {PointerEvent} e @param {LaidNode} d */ (e, d) => on.tip(e, d))
      .on("pointerleave", on.hide)
      .on("focus", /** @param {FocusEvent} e @param {LaidNode} d */ (e, d) => on.guarded("show this mark", () => { if (on.restoring()) return; on.tip(e, d); on.pin(d); }))
      .on("blur", on.hide)
      .on("click", /** @param {MouseEvent} e @param {LaidNode} d */ (e, d) => {
        on.guarded("pin this mark", () => {
          e.stopPropagation();
          on.click(d, e.timeStamp);
        });
      })
      .on("dblclick", /** @param {MouseEvent} e @param {LaidNode} d */ (e, d) => {
        on.guarded("open this mark", () => {
          e.stopPropagation();
          e.preventDefault();
          on.dblclick(d, e.timeStamp);
        });
      })
      .on("keydown", /** @param {KeyboardEvent} e @param {LaidNode} d */ (e, d) => {
        if (e.key !== "Enter" && e.key !== " ") return;
        if (e.repeat) return;
        e.preventDefault();
        on.guarded("act on this mark", () => on.key(d, e.key, e.timeStamp));
      });

    // The row's own rect is the whole row, unfilled: it takes the pointer
    // between the label and the figure, and the focus ring and the
    // isolation's stroke go round the row.
    node.append("rect")
      .attr("x", 4)
      .attr("y", /** @param {LaidNode} d */ (d) => d.y0 - 3)
      .attr("width", BARS_WIDTH - 8)
      .attr("height", BAR_HEIGHT + 6)
      .attr("rx", 2)
      .attr("fill", "none")
      .attr("pointer-events", "all");

    // ONE LABEL PER ROW, as a Sankey mark has: the name ends left of the
    // bars and the figure, the share and the flags start right of the bar,
    // each tspan placed by its own x, so every row's words are one halo.
    const middle = (/** @type {LaidNode} */ d) => (d.y0 + d.y1) / 2;
    const label = node.append("text")
      .attr("class", "halo")
      .attr("y", middle)
      .attr("dy", "0.35em");
    label.append("tspan")
      .attr("class", /** @param {LaidNode} d */ (d) => (d.layer === 0 ? "total" : null))
      .attr("x", BAR_LABELS - 8)
      .attr("text-anchor", "end")
      .text(/** @param {LaidNode} d */ (d) => d.label);
    label.append("tspan")
      .attr("class", "value")
      .attr("x", /** @param {LaidNode} d */ (d) => (d.layer === 0 ? BAR_LABELS : Math.max(d.x1, d.x0 + 1)) + 6)
      .attr("text-anchor", "start")
      .text(/** @param {LaidNode} d */ (d) => fmtShortSigned(ctx.figure(d)));
    label.append("tspan")
      .attr("class", "share")
      .text(/** @param {LaidNode} d */ (d) => {
        const pct = barPercent(d, graph.nodes);
        return pct ? "  \u25c7 " + pct + "%" : "";
      });
    label.append("tspan")
      .attr("class", "flag")
      .text(/** @param {LaidNode} d */ (d) => ctx.classes.flags(d));

    gLinks.selectAll("path")
      .data(graph.links)
      .join("path")
      .attr("class", /** @param {LaidLink} d */ (d) => ctx.classes.link(d))
      // A piece is a stroke along the bar's middle, as a ribbon is a stroke:
      // the page's link rules colour, dim and ring it alike. Never under a
      // pixel, so every piece is a target.
      .attr("d", /** @param {any} d */ (d) => "M" + d.x0 + "," + d.y0 + "H" + Math.max(d.x1 - PIECE_GAP, d.x0 + 1))
      .attr("stroke-width", BAR_HEIGHT)
      .attr("tabindex", 0)
      .attr("role", "button")
      .attr("aria-label", /** @param {LaidLink} d */ (d) => ctx.describe.link(d))
      .on("pointerenter", /** @param {PointerEvent} e @param {LaidLink} d */ (e, d) => on.tip(e, d))
      .on("pointermove", /** @param {PointerEvent} e @param {LaidLink} d */ (e, d) => on.tip(e, d))
      .on("pointerleave", on.hide)
      .on("focus", /** @param {FocusEvent} e @param {LaidLink} d */ (e, d) => on.guarded("show this flow", () => { if (on.restoring()) return; on.tip(e, d); on.pin(d); }))
      .on("blur", on.hide)
      .on("click", /** @param {MouseEvent} e @param {LaidLink} d */ (e, d) => on.guarded("pin this flow", () => { e.stopPropagation(); on.pin(d); }));
  },

  /** Colours every piece; a row's rect is unfilled, so it takes no colour. */
  paint(ctx) {
    ctx.svg.selectAll("path.link")
      .attr("stroke", /** @param {LaidLink} d */ (d) => ctx.colour.link(d));
  },

  /** A list is two tiers wide whichever step declares it. */
  widest(steps) {
    return steps.length ? 2 : 0;
  },
});
