// @ts-check
/**
 * fisc — the Sankey form: the expansion folded, capped and slotted, the
 * reductions flipped for d3-sankey, and layout helpers. Every figure is
 * core.js's; it reads only `chart.sankey`. Stateless as core.js is: d3 is
 * read through globalThis when asked, and app.js passes in its state.
 */

import {
  capColumn, fmt, fmtShortSigned, foldDocument, isExpansionSchedule, isFundGroup, isLink, labelledAsParent, openSplitOf, reducedOf, say, splitTailID, splitsOf, tailFigure, tierFrontier,
} from "./core.js";
import { share } from "./format.js";

/**
 * app.js's typedefs, each the shape it names by its schema/ path there.
 * @typedef {import("./app.js").FiscChart} FiscChart
 * @typedef {import("./app.js").FiscNode} FiscNode
 * @typedef {import("./app.js").FiscProjection} FiscProjection
 * @typedef {import("./app.js").FormRenderer} FormRenderer
 * @typedef {import("./app.js").LaidLink} LaidLink
 * @typedef {import("./app.js").LaidNode} LaidNode
 */

export const NODE_WIDTH = 14;

export const NODE_PADDING = 14;

/** The surface gap that separates stacked ribbons, in px (1px each side). */
export const RIBBON_GAP = 2;

export const CHART_HEIGHT = 820;

/**
 * Under sankey.grow, node_px per node of the busiest slot, clamped to the
 * floor, `room` and the ceiling; else CHART_HEIGHT, which no room moves.
 * @param {FiscChart | null | undefined} chart
 * @param {{id: string, tier: number, slot?: number}[]} [nodes]
 * @param {number} [room] chart units; 0 or absent where unmeasured
 * @returns {number}
 */
export function canvasHeight(chart, nodes, room) {
  const hints = chart && chart.sankey;
  if (hints && hints.grow) {
    const g = hints.grow;
    return Math.min(g.ceiling, Math.max(g.floor, room || 0, g.node_px * busiestSlot(nodes || [], hints.tiers || [])));
  }
  return CHART_HEIGHT;
}

/**
 * @param {{id: string, tier: number, slot?: number}[]} nodes
 * @param {number[]} tiers
 * @returns {number}
 */
export function busiestSlot(nodes, tiers) {
  /** @type {Map<number, number>} */
  const held = new Map();
  for (const at of slotsOf(nodes, tiers).values()) held.set(at, (held.get(at) || 0) + 1);
  return Math.max(0, ...held.values());
}

/**
 * px under which a ribbon on a declared canvas is a one-screen-pixel
 * hairline (style.css) under a wider hit target.
 */
export const HAIRLINE = 1;

/**
 * Only under sankey.grow, where thin ribbons are accepted rather than
 * folded; elsewhere a ribbon keeps the stroke floor.
 * @param {FiscChart | null | undefined} chart
 * @param {LaidLink} d
 * @returns {boolean}
 */
export function isHairline(chart, d) {
  const hints = chart && chart.sankey;
  return Boolean(hints && hints.grow) && d.width < HAIRLINE;
}

/**
 * Label room either side, px. Only end columns label outward, so it does not
 * grow with columns; site/layout.test.mjs pins what labelWords fits to it.
 */
export const LABEL_GUTTER = 320;

/** px between columns, up to HELD_COLUMNS. */
export const BAND = 319;

/** Past this many columns the bands narrow (bandOf) instead of widening. */
export const HELD_COLUMNS = 4;

/** Narrowest band, px; past it the chart widens. site/layout.test.mjs prints what a label keeps here. */
export const BAND_FLOOR = 160;

/**
 * Fixed widths, since labels do not reflow; style.css draws every width at
 * the opening chart's scale.
 * @param {number} n
 * @returns {number}
 */
export function chartWidth(n) {
  // d3-sankey divides by (columns - 1); clamp so the width stays finite.
  const columns = Math.max(1, n);
  /** @param {number} c @param {number} band */
  const at = (c, band) => 2 * LABEL_GUTTER + band * (c - 1) + NODE_WIDTH * c;
  if (columns <= HELD_COLUMNS) return at(columns, BAND);
  return Math.max(at(HELD_COLUMNS, BAND), at(columns, BAND_FLOOR));
}

/**
 * @param {number} n
 * @returns {number}
 */
export function bandOf(n) {
  const columns = Math.max(2, n);
  return (chartWidth(columns) - 2 * LABEL_GUTTER - NODE_WIDTH * columns) / (columns - 1);
}

/**
 * Draws printed reductions forward at magnitude with the document's words.
 * Never reversed: a backward ribbon adds a column d3-sankey's layering
 * throws on. A fold can net a ribbon negative with no printed reduction, or
 * positive despite one; both are handled here.
 * @param {FiscProjection} drawn shaped and folded
 * @returns {FiscProjection} drawn itself when nothing is negative
 */
export function markContra(drawn) {
  if (!drawn.links.some((l) => l.value_cents < 0 || l.contra)) return drawn;
  return Object.assign({}, drawn, {
    links: drawn.links.map((l) => {
      if (l.value_cents < 0) {
        return Object.assign({}, l, {
          value_cents: -l.value_cents,
          contra: l.contra || say("contra_orphan"),
        });
      }
      return l.contra ? Object.assign({}, l, { contra: "" }) : l;
    }),
  });
}

/**
 * @param {LaidNode} d
 * @returns {boolean}
 */
export function isContraNode(d) {
  const links = d.sourceLinks.concat(d.targetLinks);
  return links.length > 0 && links.every((l) => Boolean(l.contra));
}

/**
 * All or nothing, as isContraNode: the whole figure is then a cross-tab total.
 * @param {LaidNode} d
 * @returns {boolean}
 */
export function isPartitionNode(d) {
  const links = d.sourceLinks.concat(d.targetLinks);
  return links.length > 0 && links.every((l) => Boolean(l.partition));
}

/**
 * Negative for a contra ribbon or an all-contra node, else d3's value.
 * @param {LaidLink | LaidNode} d
 * @returns {number}
 */
export function markCents(d) {
  if (isLink(d)) {
    const l = /** @type {LaidLink} */ (d);
    return l.contra ? -l.value_cents : l.value_cents;
  }
  const n = /** @type {LaidNode} */ (d);
  return isContraNode(n) ? -n.value : n.value;
}

/**
 * The overhang of gross arriving ribbons past a net box, or null. Drawing it
 * avoids a box sized to an unprinted figure; pixels only.
 * @param {LaidNode} d
 * @returns {{y:number, height:number} | null}
 */
export function contraBand(d) {
  if (!d.targetLinks.some((l) => l.contra)) return null;
  const arriving = d.targetLinks.reduce((sum, l) => sum + endWidth(l, "target"), 0);
  const excess = arriving - (d.y1 - d.y0);
  // Half a pixel, not zero: no hairline band for reductions that round away.
  return excess > 0.5 ? { y: d.y1, height: excess } : null;
}

/**
 * For a node with contra ribbons among others; "" otherwise, including a
 * contra line, whose figure is already signed.
 * @param {LaidNode} d
 * @returns {string}
 */
export function contraNote(d) {
  if (isContraNode(d)) return "";
  const reduced = reducedOf(d.targetLinks, d.sourceLinks);
  if (!reduced) return "";
  return "\u25c7 our reading: " + fmt(reduced) + " of this category is printed as reductions, " +
    "drawn here at their size";
}

/**
 * d3-sankey sorts before its last relaxation, so ribbons cross at the face
 * for no reason in the data. Widths untouched.
 * @param {{nodes:LaidNode[], links:LaidLink[]}} graph
 */
export function restackLinks(graph) {
  /** @param {(l:LaidLink) => LaidNode} end */
  const byOtherEnd = (end) =>
    /** @param {LaidLink} a @param {LaidLink} b */ (a, b) =>
      end(a).y0 - end(b).y0 || a.index - b.index;

  for (const node of graph.nodes) {
    node.sourceLinks.sort(byOtherEnd((l) => l.target));
    // Contra last on the arriving side, inside the band; a tiebreak only, as
    // nodeRank's crossing count relies on.
    node.targetLinks.sort((a, b) =>
      Number(Boolean(a.contra)) - Number(Boolean(b.contra)) ||
      byOtherEnd((l) => l.source)(a, b));
  }
  for (const node of graph.nodes) {
    let leaving = node.y0;
    for (const l of node.sourceLinks) {
      const w = endWidth(l, "source");
      l.y0 = leaving + w / 2;
      leaving += w;
    }
    let arriving = node.y0;
    for (const l of node.targetLinks) {
      const w = endWidth(l, "target");
      l.y1 = arriving + w / 2;
      arriving += w;
    }
  }
}

/** A fund mark's tallest share of the canvas: about the screen, so not a Go hint. */
export const FUND_MAX_SHARE = 0.35;

/**
 * @param {FiscNode | LaidNode} d
 * @returns {boolean}
 */
export function isFundMark(d) {
  return isFundGroup(d) || (d.tier === 3 && (d.role === "fund" || d.role === "general_fund"));
}

/**
 * @param {LaidLink} l
 * @param {"source" | "target"} end
 * @returns {number}
 */
export function endWidth(l, end) {
  const w = end === "source" ? l.w0 : l.w1;
  return typeof w === "number" ? w : l.width;
}

/**
 * Cuts a fund mark taller than FUND_MAX_SHARE, so one fund cannot swamp its
 * column. It keeps its top; marks below move up and the column recentres.
 * Ribbons keep d3's width at the far end and narrow at the cut face (w0/w1);
 * the mark carries the ratio as `clamped`. Pixels only. A canvas upside
 * down cuts to nothing, never negative.
 * @param {{nodes: LaidNode[], links: LaidLink[]}} graph
 * @param {number} top
 * @param {number} bottom
 */
export function clampFunds(graph, top, bottom) {
  const most = Math.max(0, Math.round(FUND_MAX_SHARE * (bottom - top)));
  for (const l of graph.links) {
    l.w0 = l.width;
    l.w1 = l.width;
  }
  for (const n of graph.nodes) {
    const h = n.y1 - n.y0;
    if (!isFundMark(n) || !(h > most)) continue;
    const column = graph.nodes.filter((m) => m.layer === n.layer);
    const centre = (Math.min(...column.map((m) => m.y0)) + Math.max(...column.map((m) => m.y1))) / 2;
    const freed = h - most;
    for (const m of column) {
      if (m !== n && m.y0 >= n.y1 - 1e-9) {
        m.y0 -= freed;
        m.y1 -= freed;
      }
    }
    n.y1 = n.y0 + most;
    const k = most / h;
    n.clamped = k;
    const shift = centre - (Math.min(...column.map((m) => m.y0)) + Math.max(...column.map((m) => m.y1))) / 2;
    for (const m of column) {
      m.y0 += shift;
      m.y1 += shift;
    }
    for (const l of n.sourceLinks) l.w0 = l.width * k;
    for (const l of n.targetLinks) l.w1 = l.width * k;
  }
}

/**
 * A ratio of 0 is still a cut.
 * @param {LaidNode} d
 * @returns {boolean}
 */
export function isClamped(d) {
  return typeof d.clamped === "number";
}

/** Px two ribbon ends may differ by and be one width: float noise, not a taper. */
export const TAPER_EPSILON = 1e-6;

/**
 * Ends differing by float noise are one width: a fund group and its one fund
 * cut by ratios a few ulps apart draw no taper between them.
 * @param {LaidLink} d
 * @returns {boolean}
 */
export function isTapered(d) {
  return Math.abs(endWidth(d, "source") - endWidth(d, "target")) > TAPER_EPSILON;
}

/**
 * Each edge on d3.sankeyLinkHorizontal's curve, less RIBBON_GAP, floored at 1px.
 * @param {LaidLink} d
 * @returns {string}
 */
export function taperPath(d) {
  const x0 = d.source.x1;
  const x1 = d.target.x0;
  const xm = (x0 + x1) / 2;
  const a = Math.max(1, endWidth(d, "source") - RIBBON_GAP) / 2;
  const b = Math.max(1, endWidth(d, "target") - RIBBON_GAP) / 2;
  return "M" + x0 + "," + (d.y0 - a) + "C" + xm + "," + (d.y0 - a) + " " + xm + "," + (d.y1 - b) + " " + x1 + "," + (d.y1 - b) +
    "L" + x1 + "," + (d.y1 + b) + "C" + xm + "," + (d.y1 + b) + " " + xm + "," + (d.y0 + a) + " " + x0 + "," + (d.y0 + a) + "Z";
}

/**
 * @param {LaidNode} d
 * @returns {string}
 */
export function cutMark(d) {
  if (!isClamped(d)) return "";
  const y = d.y1 - 8;
  return "M" + (d.x0 - 3) + "," + (y + 3) + "L" + (d.x1 + 3) + "," + (y - 3) +
    "M" + (d.x0 - 3) + "," + (y + 7) + "L" + (d.x1 + 3) + "," + (y + 1);
}

/**
 * @param {LaidNode} d
 * @returns {string}
 */
export function clampNote(d) {
  if (!isClamped(d)) return "";
  const pct = /** @type {number} */ (d.clamped) * 100;
  const shown = pct >= 99.5 ? ">99" : pct < 1 ? "<1" : String(Math.round(pct));
  return "\u25c7 our drawing: this mark is cut to " + shown + "% of its full height to keep it from " +
    "swamping the chart, so it is not drawn to scale; its ribbons narrow into the cut, and its figure is unchanged";
}

/**
 * Slots per spine column: key = column * SLOT_DEPTH + ROOT_SLOT ± depth.
 */
export const SLOT_DEPTH = 10;

/** A chain may stand ROOT_SLOT slots before its root and SLOT_DEPTH - ROOT_SLOT - 1 after. */
const ROOT_SLOT = SLOT_DEPTH / 2;

/**
 * The placement: each node's column from 0, none empty. The key is `slot`
 * where stamped (expansionOf), else the tier's place in `tiers`; keys are
 * ranked, since d3-sankey throws on an empty column. An unplaced tier is
 * left out; with no tiers or slots the map is empty and d3 justifies.
 * @param {{id: string, tier: number, slot?: number}[]} nodes
 * @param {number[]} tiers the columns on screen, in order
 * @returns {Map<string, number>}
 */
export function slotsOf(nodes, tiers) {
  /** @param {{tier: number, slot?: number}} n */
  const key = (n) => (typeof n.slot === "number" ? n.slot : tiers.indexOf(n.tier));
  const keys = Array.from(new Set(nodes.map(key).filter((k) => k >= 0))).sort((a, b) => a - b);
  const at = new Map(keys.map((k, i) => [k, i]));
  /** @type {Map<string, number>} */
  const out = new Map();
  for (const n of nodes) {
    const k = key(n);
    if (k >= 0) out.set(n.id, /** @type {number} */ (at.get(k)));
  }
  return out;
}

/**
 * Slot keys from the unfolded parent chains: a root at its spine column, a
 * member one slot after or before its parent per its split's side (before a
 * source, so opening moves nothing right). No link is read for a side.
 * Fails closed on an undrawn tier, an undeclared or non-extend split, no
 * side, a member opening back into its grandparent's slot, and a chain past
 * ROOT_SLOT.
 * @param {FiscProjection} expansion
 * @param {number[]} tiers the spine's column order
 * @returns {Map<string, number>}
 */
export function expansionSlots(expansion, tiers) {
  const byID = new Map(expansion.nodes.map((n) => [n.id, n]));
  /** @type {Map<string, string>} */
  const formOf = new Map();
  /**
   * By node\u001fkey: +1 after, -1 before, 0 where no side is declared.
   * @type {Map<string, number>}
   */
  const stepOf = new Map();
  /** @type {Map<string, string>} */
  const primary = new Map();
  for (const sp of /** @type {any[]} */ ((/** @type {any} */ (expansion)).splits || [])) {
    formOf.set(sp.node + "\u001f" + sp.key, sp.form);
    stepOf.set(sp.node + "\u001f" + sp.key, sp.side === "after" ? 1 : sp.side === "before" ? -1 : 0);
    if (!sp.alternative) primary.set(sp.node, sp.key);
  }
  /** @type {Map<string, number>} */
  const out = new Map();
  /**
   * @param {FiscNode} n
   * @param {number} hops
   * @returns {number}
   */
  const slotOf = (n, hops) => {
    const have = out.get(n.id);
    if (have !== undefined) return have;
    let slot;
    if (!n.parent) {
      const column = tiers.indexOf(n.tier);
      if (column < 0) {
        throw new Error("cannot place " + n.id + ": it is a root of " + expansion.projection +
          " at tier " + n.tier + ", which the spine's columns " + tiers.join(", ") + " do not draw");
      }
      slot = column * SLOT_DEPTH + ROOT_SLOT;
    } else {
      const up = byID.get(n.parent);
      if (!up || hops > 8) throw new Error("cannot place " + n.id + ": its parent chain does not reach a root");
      const key = n.alternative || primary.get(up.id) || "";
      const form = formOf.get(up.id + "\u001f" + key);
      const above = slotOf(up, hops + 1);
      const step = stepOf.get(up.id + "\u001f" + key) ?? 0;
      const within = (above % SLOT_DEPTH) + step;
      // Opening back the way its parent opened would land in the grandparent's slot.
      const back = step !== 0 && Math.sign((above % SLOT_DEPTH) - ROOT_SLOT) === -step;
      if (form === "extend" && step !== 0 && !back && within >= 0 && within < SLOT_DEPTH) slot = above + step;
      else {
        throw new Error("cannot place " + n.id + ": it is a member of " + up.id + "'s split " +
          JSON.stringify(key) + ", " + (!form ? "which the schedule does not declare"
            : form !== "extend" ? "declared " + JSON.stringify(form) + ", and this page draws a split only as extend"
              : step === 0 ? "which declares no side of " + up.id + " its members stand on"
                : back ? "opening back toward " + up.parent + ", whose slot it would stand in"
                  : "extended past the " + (step > 0 ? SLOT_DEPTH - ROOT_SLOT - 1 + " slots after" : ROOT_SLOT + " slots before") +
                    " its root"));
      }
    }
    out.set(n.id, slot);
    return slot;
  };
  for (const n of expansion.nodes) slotOf(n, 0);
  return out;
}

/**
 * The schedule at a frontier: one fold, each capped split not in `whole`
 * folded to its largest members and a tail, folded again to merge the
 * tails' ribbons, and slots stamped. A tail sits in its members' slot under
 * their node.
 * @param {FiscProjection} expansion
 * @param {Iterable<string> | null} frontier
 * @param {FiscChart} chart the overview
 * @param {ReadonlySet<string>} whole node\u001fkey of splits drawn uncapped
 * @returns {FiscProjection}
 */
export function expansionOf(expansion, frontier, chart, whole) {
  const hints = chart.sankey || {};
  const caps = /** @type {{split: string, cap: number, tail: string}[]} */ (hints.split_caps || []);
  const keys = expansionSlots(expansion, hints.tiers || []);
  const open = new Set(frontier);
  let drawn = foldDocument(expansion, open);
  /** @type {Map<string, {node: string, slot: number}>} */
  const tails = new Map();
  for (const sp of /** @type {any[]} */ ((/** @type {any} */ (expansion)).splits || [])) {
    const cap = caps.find((c) => c.split === sp.key);
    if (!cap || whole.has(sp.node + "\u001f" + sp.key) || openSplitOf(expansion, open, sp.node) !== sp.key) continue;
    const split = splitsOf(expansion, sp.node).find((s) => s.key === sp.key);
    const members = new Set((split ? split.members : []).filter((m) => open.has(m)));
    if (!members.size) continue;
    const id = splitTailID(sp.node, sp.key);
    const tier = /** @type {FiscNode} */ (expansion.nodes.find((n) => members.has(n.id))).tier;
    const capped = capColumn(drawn, tier, cap.cap, sp.node, cap.tail, { members, id });
    if (capped === drawn) continue;
    drawn = capped;
    tails.set(id, { node: sp.node, slot: /** @type {number} */ (keys.get(Array.from(members)[0])) });
  }
  if (tails.size) drawn = foldDocument(drawn, drawn.nodes.map((n) => n.id));
  const folded = drawn;
  return Object.assign({}, folded, {
    nodes: folded.nodes.map((n) => {
      const tail = tails.get(n.id);
      if (!tail) return Object.assign({}, n, { slot: keys.get(n.id) });
      return Object.assign({}, n, {
        slot: tail.slot,
        parent: tail.node,
        source_note: n.source_note + say("aggregate_together", { figure: fmt(tailFigure(folded, n.id)) }),
      });
    }),
  });
}

/**
 * Justify puts a link-less sink last, wrong once columns are skipped; an
 * unplaced node gets -1, which d3 clamps to the first column.
 * @param {Map<string, number>} slots
 * @returns {(d: LaidNode) => number}
 */
export function alignFor(slots) {
  return slots.size
    ? /** @param {LaidNode} d */ (d) => { const at = slots.get(d.id); return at === undefined ? -1 : at; }
    : /** @type {any} */ (globalThis).d3.sankeyJustify;
}

/**
 * Not d.depth (the longest path), and d.layer only where nothing is placed.
 * @param {Map<string, number>} slots
 * @param {LaidNode} d
 * @returns {number}
 */
export function columnOf(slots, d) {
  // d3 clamps an unplaced node into column 0, so it is labelled as one.
  return slots.size ? Math.max(0, slots.get(d.id) === undefined ? 0 : /** @type {number} */ (slots.get(d.id))) : d.layer;
}

/**
 * Only end columns have a gutter to label outward into; an interior label
 * centres above its rect in the NODE_PADDING gap.
 * @param {Map<string, number>} slots
 * @param {LaidNode} d
 * @param {number} last the largest column index drawn
 * @returns {{anchor: string, x: number, y: number, dy: string|null}}
 */
export function labelPlacement(slots, d, last) {
  const col = columnOf(slots, d);
  const middle = (d.y0 + d.y1) / 2;
  if (col === 0) return { anchor: "end", x: d.x0 - 10, y: middle, dy: "0.35em" };
  if (col >= last) return { anchor: "start", x: d.x1 + 10, y: middle, dy: "0.35em" };
  // No dy: a half-em shift down would drop the glyphs onto the rect.
  return { anchor: "middle", x: (d.x0 + d.x1) / 2, y: d.y0 - 5, dy: null };
}

/** The radius of the ⊕ or ⊖ beside a node that splits or folds, in px. */
export const GLYPH_R = 6;

/** The gap between a node's rect and its glyph, in px. */
export const GLYPH_GAP = 3;

/**
 * Beside the rect on the ribbons' side (left in the last column); a rect too
 * short to hold it puts it after its label.
 * @param {Map<string, number>} slots
 * @param {LaidNode} d
 * @param {number} last the largest column index drawn
 * @param {number} width labelAdvance
 * @returns {{x: number, y: number}}
 */
export function glyphPlacement(slots, d, last, width) {
  const col = columnOf(slots, d);
  const y = (d.y0 + d.y1) / 2;
  if (col !== 0 && col >= last) return { x: d.x0 - GLYPH_GAP - GLYPH_R, y };
  if (col !== 0 && d.y1 - d.y0 < 2 * GLYPH_R) {
    const place = labelPlacement(slots, d, last);
    return { x: place.x + width / 2 + GLYPH_GAP + GLYPH_R, y: place.y - GLYPH_R };
  }
  return { x: d.x1 + GLYPH_GAP + GLYPH_R, y };
}

/**
 * Parent labels on marks whose label repeats in their column: ambiguity is
 * the column's. The parent comes from `doc`, since the fold blanks a parent
 * split open; a tail keeps its laid parent; unnameable gets "", not invented.
 * @param {LaidNode[]} nodes
 * @param {{nodes: FiscNode[]} | null} doc the unfolded nodes
 * @param {Map<string, number>} slots
 * @returns {Map<string, string>}
 */
export function labelQualifiers(doc, slots, nodes) {
  const byID = new Map((doc ? doc.nodes : []).map((n) => [n.id, n]));
  /** @type {Map<string, LaidNode[]>} */
  const sharing = new Map();
  for (const n of nodes) {
    const key = columnOf(slots, n) + "\u0000" + n.label;
    const seen = sharing.get(key);
    if (seen) seen.push(n);
    else sharing.set(key, [n]);
  }
  /** @type {Map<string, string>} */
  const out = new Map();
  for (const shared of sharing.values()) {
    if (shared.length < 2) continue;
    for (const n of shared) {
      const own = byID.get(n.id) || n;
      const parent = own.parent ? byID.get(own.parent) : undefined;
      out.set(n.id, parent ? parent.label : "");
    }
  }
  return out;
}

/**
 * Member nouns for marks printed in their parent's words: an extend split
 * draws both side by side, so it would read as one thing twice.
 * @param {{nodes: FiscNode[]} | null} doc the unfolded nodes
 * @param {LaidNode[]} nodes
 * @param {(id: string) => string} nounOf
 * @returns {Map<string, string>}
 */
export function labelNouns(doc, nodes, nounOf) {
  const byID = new Map((doc ? doc.nodes : []).map((n) => [n.id, n]));
  /** @type {Map<string, string>} */
  const out = new Map();
  for (const n of nodes) {
    if (!labelledAsParent(byID, n.id)) continue;
    const noun = nounOf(n.id);
    if (noun) out.set(n.id, noun);
  }
  return out;
}

/** em per character, over-estimated so the error cuts a label that would have fitted. */
export const ADVANCE_EM = 0.6;

/** The px style.css sets a label's words and its flags at. */
export const LABEL_PX = 12;

/** The px style.css sets a label's value at. */
export const VALUE_PX = 11;

/**
 * @param {string} text
 * @param {number} px
 * @returns {number}
 */
export function textAdvance(text, px) {
  return text.length * px * ADVANCE_EM;
}

/**
 * @param {{qualifier: string, label: string, noun?: string}} words
 * @param {string} value
 * @param {string} flags
 * @returns {number}
 */
export function labelAdvance(words, value, flags) {
  return textAdvance(words.qualifier + words.label + (words.noun || ""), LABEL_PX) +
    textAdvance(value, VALUE_PX) + textAdvance(flags, LABEL_PX);
}

/**
 * To the chart edge outward; interior, its rect and half a band either side.
 * @param {{anchor: string, x: number}} place labelPlacement's answer
 * @param {number} columns
 * @returns {number}
 */
export function labelRoom(place, columns) {
  if (place.anchor === "end") return place.x;
  if (place.anchor === "start") return chartWidth(columns) - place.x;
  return bandOf(columns) + NODE_WIDTH;
}

/**
 * Fits a label to `room`. One line always: d3 holds only NODE_PADDING
 * between marks and canvasHeight counts marks. An overlong label has its
 * owner's middle cut (the words before the last ": ", else the qualifier,
 * else the label), keeping its first word and end, since funds sharing a
 * prefix differ at the end ("County Meas BB-Bike/Pedestrian"). If two
 * characters still overflow, the whole line is cut as one owner. Go's text,
 * only fitted.
 * @param {number} room labelRoom
 * @param {string} qualifier or ""
 * @param {string} label
 * @param {(words: {qualifier: string, label: string, noun: string}) => number} measure
 * @param {string} [noun] or ""
 * @returns {{qualifier: string, label: string, noun: string, cut: boolean}}
 */
export function labelWords(room, qualifier, label, measure, noun = "") {
  const lead = qualifier ? qualifier + ": " : "";
  const after = noun ? " (" + noun + ")" : "";
  if (measure({ qualifier: lead, label, noun: after }) <= room) return { qualifier: lead, label, noun: after, cut: false };
  const split = label.lastIndexOf(": ");
  const fitted = split > 0
    ? cutOwner(label.slice(0, split), (owner) => ({ qualifier: lead, label: owner + label.slice(split), noun: after }), room, measure)
    : qualifier
      ? cutOwner(qualifier, (owner) => ({ qualifier: owner + ": ", label, noun: after }), room, measure)
      : noun
        ? cutOwner(label, (owner) => ({ qualifier: "", label: owner, noun: after }), room, measure)
        : null;
  if (fitted && measure(fitted) <= room) return fitted;
  const parts = [lead, label, after];
  return cutOwner(parts.join(""), (owner, head, tail) => {
    const [q, l, n] = keptOf(parts, head, tail);
    return { qualifier: q, label: l, noun: n };
  }, room, measure);
}

/**
 * Each part's share of a join cut to `head`…`tail`; whole where tail is null.
 * @param {string[]} parts
 * @param {string} head
 * @param {string | null} tail
 * @returns {string[]}
 */
function keptOf(parts, head, tail) {
  if (tail === null) return parts.slice();
  const whole = parts.join("");
  const from = whole.length - tail.length;
  let at = 0;
  return parts.map((p) => {
    const a = at;
    const b = at + p.length;
    at = b;
    const front = whole.slice(a, Math.max(a, Math.min(b, head.length)));
    const back = whole.slice(Math.min(b, Math.max(a, from)), b);
    return front + (head.length >= a && head.length < b ? "\u2026" : "") + back;
  });
}

/**
 * The most of `owner` (two characters at least) that fits: first word, then
 * the end, since names share prefixes ("Grant - ") more than ends.
 * @param {string} owner
 * @param {(owner: string, head: string, tail: string | null) => {qualifier: string, label: string, noun: string}} words
 * @param {number} room
 * @param {(words: {qualifier: string, label: string, noun: string}) => number} measure
 * @returns {{qualifier: string, label: string, noun: string, cut: boolean}}
 */
function cutOwner(owner, words, room, measure) {
  const word = owner.search(/[\s-]/);
  /** @param {number} keep how many of the owner's characters stay */
  const cutTo = (keep) => {
    const front = Math.min(word > 0 ? word : keep, Math.ceil(keep / 2));
    const head = owner.slice(0, front).trimEnd();
    const tail = owner.slice(owner.length - (keep - front)).trimStart();
    return words(head + "\u2026" + tail, head, tail);
  };
  // The most of the owner's characters that fit, by halving.
  let lo = 2;
  let hi = owner.length - 1;
  if (hi < lo) return Object.assign(words(owner, owner, null), { cut: false });
  while (lo < hi) {
    const mid = Math.ceil((lo + hi) / 2);
    if (measure(cutTo(mid)) <= room) lo = mid;
    else hi = mid - 1;
  }
  return Object.assign(cutTo(lo), { cut: true });
}

/**
 * A mark's share of its column's drawn money. Arithmetic, and says so. A
 * printed reduction has no share and is in no total.
 * @param {LaidNode} d
 * @param {LaidNode[]} laid
 * @returns {string}
 */
export function columnShare(laid, d) {
  if (!d.value || isContraNode(d)) return "";
  let total = 0;
  let siblings = 0;
  for (const other of laid) {
    if (other.layer === d.layer && !isContraNode(other)) {
      total += other.value;
      siblings++;
    }
  }
  // No share of a column of one: it is 100% by construction.
  if (!total || siblings < 2) return "";
  // "our" is what survives the diamond being read aloud.
  return "\u25c7 our " + share(d.value, total) + " of this column";
}

/**
 * Sort key in a column: a fund group's own place, else the value-weighted
 * mean place of the groups it touches, else its own group's. A nodeSort is
 * what pins the fund column to the palette's order.
 * @param {LaidNode} node
 * @param {(node: FiscNode | LaidNode) => string} groupOf
 * @param {(id: string) => number} placeOf
 * @returns {number}
 */
export function nodeRank(groupOf, placeOf, node) {
  if (isFundGroup(node)) return placeOf(node.id);

  let weight = 0;
  let place = 0;
  for (const l of node.sourceLinks.concat(node.targetLinks)) {
    const other = l.source === node ? l.target : l.source;
    // The neighbour's group: once a group splits, ends are funds and divisions.
    const group = groupOf(other);
    // A node in no fund group is ignored rather than counted as position zero.
    if (!group) continue;
    const at = placeOf(group);
    place += at * l.value;
    weight += l.value;
  }
  if (weight) return place / weight;
  // No grouped neighbour: its own group's place; groupless sorts top, value breaks ties.
  const own = groupOf(node);
  return own ? placeOf(own) : 0;
}

/**
 * Zero-value links so a path spans every slot: d3-sankey adds a column only
 * per step of the longest path, and two open extend splits can declare a
 * slot no ribbon reaches. None where drawn paths already span (a scaffold
 * can nudge ribbon aims), and none where a ribbon runs backward in the slots:
 * a scaffold could close a cycle, so the clamp check refuses instead.
 * @param {FiscProjection} drawn
 * @param {Map<string, number>} slots
 * @returns {{source: string, target: string, value: number, scaffold: true}[]}
 */
export function scaffoldOf(drawn, slots) {
  const ranks = new Set(slots.values()).size;
  if (ranks < 2 || drawn.links.some((l) => !(/** @type {number} */ (slots.get(l.source)) < /** @type {number} */ (slots.get(l.target))))) return [];
  /** @type {{source: string, target: string, value: number, scaffold: true}[]} */
  const out = [];
  const placed = drawn.nodes.filter((n) => slots.has(n.id))
    .sort((a, b) => /** @type {number} */ (slots.get(a.id)) - /** @type {number} */ (slots.get(b.id)));
  // Links run forward in the slots, so one pass in slot order suffices.
  const depths = () => {
    /** @type {Map<string, number>} */
    const depth = new Map();
    /** @type {Map<string, string[]>} */
    const into = new Map();
    for (const l of [...drawn.links, ...out]) into.set(l.target, [...(into.get(l.target) || []), l.source]);
    for (const n of placed) {
      depth.set(n.id, Math.max(0, ...(into.get(n.id) || []).map((s) => /** @type {number} */ (depth.get(s)) + 1)));
    }
    return depth;
  };
  for (let r = 1; r < ranks; r++) {
    const depth = depths();
    const at = (/** @type {number} */ rank) => placed.filter((n) => slots.get(n.id) === rank);
    if (at(r).some((n) => depth.get(n.id) === r)) continue;
    const from = /** @type {FiscNode} */ (at(r - 1).find((n) => depth.get(n.id) === r - 1));
    const to = at(r).reduce((a, b) => (/** @type {number} */ (depth.get(b.id)) > /** @type {number} */ (depth.get(a.id)) ? b : a));
    out.push({ source: from.id, target: to.id, value: 0, scaffold: true });
  }
  return out;
}

/**
 * Vertical places from the replaced chart: a kept node's centre, a new one's
 * nearest laid ancestor's, so members take their parent's place. Only fully
 * placed columns are seated, so a fresh chart lays out as by nodeRank alone.
 * @param {{id: string, tier: number, slot?: number}[]} nodes
 * @param {Map<string, number>} slots
 * @param {Map<string, {y0: number, y1: number}>} [before]
 * @param {(id: string) => string} [parentOf]
 * @returns {Map<string, number>} seated columns only
 */
export function seatsOf(nodes, slots, before, parentOf) {
  /** @type {Map<string, number>} */
  const out = new Map();
  if (!before || !before.size) return out;
  /** @type {Map<number, {id: string, at: number | undefined}[]>} */
  const columns = new Map();
  for (const n of nodes) {
    let at = n.id;
    for (let hops = 0; at && !before.has(at) && hops < 9; hops++) at = parentOf ? parentOf(at) : "";
    const was = at ? before.get(at) : undefined;
    const col = /** @type {number} */ (slots.get(n.id));
    columns.set(col, (columns.get(col) || []).concat([{ id: n.id, at: was ? (was.y0 + was.y1) / 2 : undefined }]));
  }
  for (const seated of columns.values()) {
    if (seated.some((x) => x.at === undefined)) continue;
    for (const x of seated) out.set(x.id, /** @type {number} */ (x.at));
  }
  return out;
}

/**
 * SANKEY.layOut at a stated height. Fails closed on a slotted node d3 laid
 * elsewhere: d3 silently clamps a node beyond its last column. Only a
 * backward ribbon, given no scaffold, leaves that reachable.
 * @param {FiscProjection} drawn
 * @param {{tiers: number[], columns: number, groupOf: (n: any) => string, placeOf: (id: string) => number,
 *   before?: Map<string, {y0: number, y1: number}>, parentOf?: (id: string) => string}} ctx
 * @param {number} height
 * @returns {{nodes: LaidNode[], links: LaidLink[]}}
 */
export function layOutAt(drawn, ctx, height) {
  const d3 = /** @type {any} */ (globalThis).d3;
  const slots = slotsOf(drawn.nodes, ctx.tiers);
  const seats = seatsOf(drawn.nodes, slots, ctx.before, ctx.parentOf);
  const rank = (/** @type {LaidNode} */ a, /** @type {LaidNode} */ b) =>
    nodeRank(ctx.groupOf, ctx.placeOf, a) - nodeRank(ctx.groupOf, ctx.placeOf, b) || b.value - a.value;
  const sankey = d3.sankey()
    .nodeId(/** @param {LaidNode} d */ (d) => d.id)
    .nodeWidth(NODE_WIDTH)
    .nodePadding(NODE_PADDING)
    .nodeAlign(alignFor(slots))
    // Supplying this switches d3's own ordering pass off.
    .nodeSort(/** @param {LaidNode} a @param {LaidNode} b */ (a, b) => {
      const sa = seats.get(a.id);
      const sb = seats.get(b.id);
      return (sa !== undefined && sb !== undefined ? sa - sb : 0) || rank(a, b);
    })
    // The same column count render() sizes the viewBox from.
    .extent([[LABEL_GUTTER, 12],
      [chartWidth(ctx.columns) - LABEL_GUTTER, height - 12]]);
  const graph = sankey({
    nodes: drawn.nodes.map((n) => Object.assign({}, n)),
    links: drawn.links.map((l) => Object.assign({}, l, { value: l.value_cents }))
      .concat(/** @type {any[]} */ (scaffoldOf(drawn, slots))),
  });
  // The scaffold leaves before anything reads a ribbon: never drawn, stacked or counted.
  /** @param {any} l */
  const drawnLink = (l) => !l.scaffold;
  graph.links = graph.links.filter(drawnLink);
  for (const n of graph.nodes) {
    n.sourceLinks = n.sourceLinks.filter(drawnLink);
    n.targetLinks = n.targetLinks.filter(drawnLink);
  }
  for (const n of graph.nodes) {
    const at = slots.get(n.id);
    if (typeof n.slot === "number" && n.layer !== at) {
      throw new Error("cannot lay out " + drawn.projection + ": " + n.id + " belongs in column " + at +
        " and no ribbon drawn reaches that far, so d3-sankey would draw it in column " + n.layer);
    }
  }
  clampFunds(graph, 12, height - 12);
  restackLinks(graph);
  return graph;
}

/** The transition a redraw moves its marks in, and the one its labels fade in after. */
const MOVE = "chart-move";
const FADE = "label-fade";

/** A label's tspans in order, by class: qualifier, label, noun, value, flag. */
const LABEL_TSPANS = ["qualifier", "", "qualifier", "value", "flag"];

/**
 * Ends, kind and cross-tab flag: what one shaped ribbon is drawn per.
 * @param {{source: any, target: any, kind: string, partition?: boolean}} d
 * @returns {string}
 */
export function linkKey(d) {
  const end = (/** @type {any} */ x) => (x && typeof x === "object" ? x.id : x);
  return end(d.source) + "→" + end(d.target) + "\u001f" + d.kind + (d.partition ? "\u001fpartition" : "");
}

/**
 * A repeated id is numbered, so no two ribbons share an element.
 * @param {any[]} links
 * @returns {Map<any, string>}
 */
function linkKeys(links) {
  /** @type {Map<string, number>} */
  const seen = new Map();
  /** @type {Map<any, string>} */
  const out = new Map();
  for (const l of links) {
    const k = linkKey(l);
    const n = seen.get(k) || 0;
    seen.set(k, n + 1);
    out.set(l, n ? k + "#" + n : k);
  }
  return out;
}

/**
 * Reads only `chart.sankey`; site/layout.test.mjs holds every hint read to
 * this file through a recording Proxy.
 * @type {FormRenderer}
 */
export const SANKEY = Object.freeze({
  form: "sankey",

  /** The spine's columns, left to right. */
  tiersOf(chart) {
    return (chart.sankey && chart.sankey.tiers) || [];
  },

  /** How wide a chart of this many columns is laid out, in px. */
  width(columns) {
    return chartWidth(columns);
  },

  /** The figure a laid mark prints: markCents. */
  cents(d) {
    return markCents(d);
  },

  /** A laid node's share of its column, as a sentence: columnShare. */
  share(d, laid) {
    return columnShare(laid, d);
  },

  /** Whether a laid node's every ribbon is a printed reduction: isContraNode. */
  reduction(d) {
    return isContraNode(d);
  },

  /** Whether a laid node's whole figure is a cross-tab total: isPartitionNode. */
  crossTab(d) {
    return isPartitionNode(d);
  },

  /** How much of a laid node's figure is printed as reductions, in words: contraNote. */
  reductionNote(d) {
    return contraNote(d);
  },

  /** What the drawing does to a laid node's height, in words: clampNote. */
  drawnNote(d) {
    return clampNote(d);
  },

  /** Distinct slots, at least one. */
  columnsOf(drawn, tiers) {
    return Math.max(1, new Set(slotsOf(drawn.nodes, tiers).values()).size);
  },

  /** The expansion via expansionOf, else a tier-set fold; then markContra. */
  shape(doc, view) {
    if (isExpansionSchedule(doc)) return markContra(expansionOf(doc, view.frontier, view.chart, view.whole));
    return markContra(foldDocument(doc, tierFrontier(doc, SANKEY.tiersOf(view.chart))));
  },

  /**
   * Pure of the page. d3-sankey mutates its input, so it gets a copy and the
   * document stays what the detail panel reads.
   */
  layOut(drawn, ctx) {
    return layOutAt(drawn, ctx, canvasHeight(ctx.chart, drawn.nodes, ctx.room));
  },

  /** The canvas at ctx.room with nothing held, so the page can tell if a room moves it. */
  canvas(nodes, ctx) {
    return canvasHeight(ctx.chart, nodes, ctx.room);
  },

  /** How many laid ribbons of this chart are drawn as hairlines: isHairline. */
  hairlines(graph, chart) {
    return graph.links.filter((l) => isHairline(chart, l)).length;
  },

  /**
   * Draws into ctx.layer; paints nothing, so a theme change repaints without
   * redrawing. Keyed by node id and linkKey so a mark keeps its element:
   * survivors glide, new marks grow from and leaving marks shrink into their
   * nearest ancestor's facing edge (edgeOf); labels fade in after. The
   * viewBox is never shorter than ctx.held. ctx.named gets every uncut name
   * before any description is asked.
   * @returns {() => void} puts every mark at its end now
   */
  render(graph, ctx) {
    const d3 = /** @type {any} */ (globalThis).d3;
    const svg = ctx.svg;
    const width = chartWidth(ctx.columns);
    const height = Math.max(canvasHeight(ctx.chart, graph.nodes, ctx.room), ctx.held || 0);
    const on = ctx.on;
    const motion = ctx.motion > 0 ? ctx.motion : 0;
    /** @type {Map<string, {x0: number, x1: number, y0: number, y1: number}>} */
    const before = ctx.before || new Map();
    /** @type {(id: string) => string} */
    const parentOf = ctx.parentOf || (() => "");

    // No width or height attributes: the viewBox makes the drawing scale.
    svg.attr("viewBox", "0 0 " + width + " " + height);

    const layer = ctx.layer;
    // What a replaced chart left moving is cut off where it stands.
    layer.selectAll("*").interrupt(MOVE).interrupt(FADE);
    layer.selectAll("[data-leaving]").remove();
    /** @param {string} name */
    const groupNamed = (name) => {
      const g = layer.select(":scope > g." + name);
      return g.empty() ? layer.append("g").attr("class", name) : g;
    };
    const gLinks = groupNamed("links");
    const gHits = groupNamed("link-hits");
    const gNodes = groupNamed("nodes");
    const gGlyphs = groupNamed("glyphs");
    /** @param {LaidLink} d */
    const hairline = (d) => isHairline(ctx.chart, d);
    // A hairline is never a band.
    /** @param {LaidLink} d */
    const taper = (d) => !hairline(d) && isTapered(d);
    const keys = linkKeys(graph.links);
    /** @param {LaidLink} d */
    const keyOf = (d) => keys.get(d) || linkKey(d);
    const path = d3.sankeyLinkHorizontal();

    const now = new Map(graph.nodes.map((n) => [n.id, n]));

    const slots = slotsOf(graph.nodes, ctx.tiers);
    const qualified = labelQualifiers(ctx.doc, slots, graph.nodes);
    /** @param {LaidNode} d */
    const qualifierOf = (d) => qualified.get(d.id) || "";
    const nouns = labelNouns(ctx.doc, graph.nodes, ctx.noun);
    /** @param {LaidNode} d */
    const nounOf = (d) => nouns.get(d.id) || "";
    // A name is the drawn words unfitted.
    /** @param {LaidNode} d */
    const nameOf = (d) => (qualifierOf(d) ? qualifierOf(d) + ": " : "") + d.label + (nounOf(d) ? " (" + nounOf(d) + ")" : "");
    if (ctx.named) ctx.named(new Map(graph.nodes.map((d) => [d.id, nameOf(d)])));
    /**
     * @param {string} id
     * @param {Map<string, {x0: number, x1: number, y0: number, y1: number}>} boxes
     */
    const above = (id, boxes) => {
      let at = parentOf(id);
      for (let hops = 0; at && hops < 9; hops++) {
        const box = boxes.get(at);
        if (box) return box;
        at = parentOf(at);
      }
      return null;
    };
    /** Where a mark that enters grows from. @param {string} id */
    const grownFrom = (id) => above(id, before) || above(id, now);
    /**
     * Left where the mark stands left of its ancestor (a source's member),
     * else right; read against `box` where `boxes` lacks the ancestor.
     * @param {string} id
     * @param {Map<string, {x0: number, x1: number, y0: number, y1: number}>} boxes
     * @param {{x0: number, x1: number}} box
     */
    const edgeOf = (id, boxes, box) => {
      const mark = boxes.get(id);
      const up = above(id, boxes) || box;
      return mark && mark.x1 <= up.x0 ? box.x0 : box.x1;
    };
    /** A ribbon at no width at x, mid-height of a box. */
    const flatAt = (/** @type {{y0: number, y1: number}} */ box, /** @type {number} */ x) => {
      const y = (box.y0 + box.y1) / 2;
      return path({ source: { x1: x }, target: { x0: x }, y0: y, y1: y });
    };
    const t = motion ? d3.transition(MOVE).duration(motion) : null;
    /** @param {any} sel */
    const moving = (sel) => (t ? sel.transition(t) : sel);

    /** @type {any} */
    let enteredLinks = null;
    /** @type {any} */
    let leavingLinks = null;
    const ribbons = gLinks.selectAll("path")
      .data(graph.links, keyOf)
      .join(
        (/** @type {any} */ enter) => (enteredLinks = enter.append("path")),
        (/** @type {any} */ update) => update,
        (/** @type {any} */ exit) => (leavingLinks = exit))
      .attr("class", /** @param {LaidLink} d */ (d) => ctx.classes.link(d) + (hairline(d) ? " hairline" : taper(d) ? " taper" : ""))
      .attr("tabindex", 0)
      .attr("role", "button")
      .attr("aria-hidden", null)
      .attr("aria-label", /** @param {LaidLink} d */ (d) => ctx.describe.link(d))
      .on("pointerenter", /** @param {PointerEvent} e @param {LaidLink} d */ (e, d) => { on.tip(e, d); on.trace(d); })
      .on("pointermove", /** @param {PointerEvent} e @param {LaidLink} d */ (e, d) => on.tip(e, d))
      .on("pointerleave", () => { on.hide(); on.untrace(); })
      .on("focus", /** @param {FocusEvent} e @param {LaidLink} d */ (e, d) => on.guarded("show this flow", () => { if (on.unasked()) return; on.tip(e, d); on.pin(d); }))
      .on("blur", on.hide)
      .on("click", /** @param {MouseEvent} e @param {LaidLink} d */ (e, d) => on.guarded("pin this flow", () => { e.stopPropagation(); on.pin(d); }));
    /** @param {any} sel */
    const placeLinks = (sel) => sel
      .attr("d", /** @param {LaidLink} d */ (d) => (taper(d) ? taperPath(d) : path(d)))
      // The surface gap separates ribbons; a band's stroke is only style.css's dashed outline.
      .attr("stroke-width", /** @param {LaidLink} d */ (d) => (taper(d) ? 1 : Math.max(1, endWidth(d, "source") - RIBBON_GAP)));

      // A hairline's wider transparent hit target takes the pointer; the
      // ribbon keeps focus and description.
    gHits.selectAll("path")
      .data(graph.links.filter(hairline), keyOf)
      .join("path")
      .attr("class", "link-hit")
      .attr("d", path)
      .attr("aria-hidden", "true")
      .on("pointerenter", /** @param {PointerEvent} e @param {LaidLink} d */ (e, d) => { on.tip(e, d); on.trace(d); })
      .on("pointermove", /** @param {PointerEvent} e @param {LaidLink} d */ (e, d) => on.tip(e, d))
      .on("pointerleave", () => { on.hide(); on.untrace(); })
      .on("click", /** @param {MouseEvent} e @param {LaidLink} d */ (e, d) => on.guarded("pin this flow", () => { e.stopPropagation(); on.pin(d); }));

    /** @type {any} */
    let enteredNodes = null;
    /** @type {any} */
    let leavingNodes = null;
    /** The mark a menu key went down on, whose contextmenu is the key's. @type {Element | null} */
    let keyedMenu = null;
    const node = gNodes.selectAll("g")
      .data(graph.nodes, /** @param {LaidNode} d */ (d) => d.id)
      .join(
        (/** @type {any} */ enter) => {
          const g = enter.append("g");
          g.append("rect").attr("rx", 2);
          // On every mark: the vendored d3 has no selection.filter().
          g.append("rect").attr("class", "contra-band").attr("pointer-events", "none");
          // The cut a clamped mark is drawn with (cutMark), likewise on every mark.
          g.append("path").attr("class", "cut").attr("pointer-events", "none").attr("aria-hidden", "true");
          const text = g.append("text").attr("class", "halo");
          for (const cls of LABEL_TSPANS) text.append("tspan").attr("class", cls || null);
          return (enteredNodes = g);
        },
        (/** @type {any} */ update) => update,
        (/** @type {any} */ exit) => (leavingNodes = exit))
      .attr("class", /** @param {LaidNode} d */ (d) => ctx.classes.node(d))
      .attr("tabindex", 0)
      .attr("role", "button")
      .attr("aria-hidden", null)
      // aria-pressed is the isolation. Splitting is not that toggle and must
      // never be announced as it.
      .attr("aria-pressed", "false")
      .attr("aria-label", /** @param {LaidNode} d */ (d) => ctx.describe.node(d))
      // M opens the menu, as Shift+F10, the ContextMenu key and right-click do.
      .attr("aria-keyshortcuts", "Enter Space M Shift+F10")
      .on("pointerenter", /** @param {PointerEvent} e @param {LaidNode} d */ (e, d) => { on.tip(e, d); on.trace(d); })
      .on("pointermove", /** @param {PointerEvent} e @param {LaidNode} d */ (e, d) => on.tip(e, d))
      .on("pointerleave", () => { on.hide(); on.untrace(); })
      .on("focus", /** @param {FocusEvent} e @param {LaidNode} d */ (e, d) => on.guarded("show this mark", () => { if (on.unasked()) return; on.tip(e, d); on.pin(d); }))
      .on("blur", on.hide)
      // Click and keydown both: an SVG g[role=button] synthesises no click
      // from Enter. A held key is ignored.
      .on("click", /** @param {MouseEvent} e @param {LaidNode} d */ (e, d) => {
        on.guarded("open this mark", () => {
          e.stopPropagation();
          on.click(d, e.timeStamp);
        });
      })
      // Shift+F10 and the ContextMenu key reach the menu only as this event,
      // never a keydown arm: on Windows the key's contextmenu fires on keyup,
      // after a keydown arm moved focus, and the browser's menu opens over
      // it. The keydown marks the mark (keyedMenu), since Chromium gives that
      // event the mark's coordinates.
      .on("contextmenu", /** @this {SVGGElement} @param {MouseEvent} e @param {LaidNode} d */ function (e, d) {
        const keyed = keyedMenu === this;
        keyedMenu = null;
        if (!on.menu) return;
        e.preventDefault();
        e.stopPropagation();
        const el = this;
        on.guarded("open this mark's menu", () => on.menu(d, keyed ? null : { left: e.clientX, top: e.clientY }, el));
      })
      .on("pointerdown.menu", () => { keyedMenu = null; })
      .on("keydown", /** @this {SVGGElement} @param {KeyboardEvent} e @param {LaidNode} d */ function (e, d) {
        if (e.key === "ContextMenu" || (e.key === "F10" && e.shiftKey)) {
          keyedMenu = this;
          return;
        }
        // M opens on keydown: macOS Safari fires no contextmenu for either key.
        if ((e.key === "m" || e.key === "M") && !e.ctrlKey && !e.metaKey && !e.altKey) {
          if (!on.menu || e.repeat) return;
          e.preventDefault();
          const el = this;
          on.guarded("open this mark's menu", () => on.menu(d, null, el));
          return;
        }
        if (e.key !== "Enter" && e.key !== " ") return;
        if (e.repeat) return;
        e.preventDefault();
        on.guarded("open this mark", () => on.key(d, e.timeStamp));
      });
    /** @param {any} sel the marks, or a transition of them */
    const placeNodes = (sel) => {
      sel.select("rect")
        .attr("x", /** @param {LaidNode} d */ (d) => d.x0)
        .attr("y", /** @param {LaidNode} d */ (d) => d.y0)
        .attr("width", /** @param {LaidNode} d */ (d) => d.x1 - d.x0)
        .attr("height", /** @param {LaidNode} d */ (d) => Math.max(2, d.y1 - d.y0));
      sel.select("rect.contra-band")
        .attr("x", /** @param {LaidNode} d */ (d) => d.x0)
        .attr("y", /** @param {LaidNode} d */ (d) => (contraBand(d) || { y: 0 }).y)
        .attr("width", /** @param {LaidNode} d */ (d) => d.x1 - d.x0)
        .attr("height", /** @param {LaidNode} d */ (d) => (contraBand(d) || { height: 0 }).height);
      sel.select("path.cut").attr("d", /** @param {LaidNode} d */ (d) => cutMark(d) || "M0,0");
    };
    node.select("rect.contra-band")
      .attr("display", /** @param {LaidNode} d */ (d) => (contraBand(d) ? null : "none"));
    node.select("path.cut")
      .attr("display", /** @param {LaidNode} d */ (d) => (isClamped(d) ? null : "none"));

    // Every node is labelled: the relief the palette's contrast check requires.
    const place = (/** @type {LaidNode} */ d) => labelPlacement(slots, d, lastColumn);
    const lastColumn = Math.max(...graph.nodes.map((d) => columnOf(slots, d)));
    /** @param {LaidNode} d */
    const value = (d) => "  " + fmtShortSigned(ctx.figure(d));
    /** @type {Map<string, {qualifier: string, label: string, noun: string, cut: boolean}>} */
    const fitted = new Map();
    // Fitted at ADVANCE_EM, not measured: site/layout.test.mjs holds the
    // drawn tspans to these words.
    /** @param {LaidNode} d */
    const words = (d) => {
      let w = fitted.get(d.id);
      if (!w) {
        const p = place(d);
        w = labelWords(labelRoom(p, ctx.columns), qualifierOf(d), d.label,
          (ws) => labelAdvance(ws, value(d), ctx.classes.flags(d)), nounOf(d));
        fitted.set(d.id, w);
      }
      return w;
    };
    const label = node.select("text")
      .attr("y", /** @param {LaidNode} d */ (d) => place(d).y)
      .attr("dy", /** @param {LaidNode} d */ (d) => place(d).dy)
      .attr("x", /** @param {LaidNode} d */ (d) => place(d).x)
      .attr("text-anchor", /** @param {LaidNode} d */ (d) => place(d).anchor);
    // The noun is the page's words, so drawn as the qualifier is.
    label.each(/** @this {SVGTextElement} @param {LaidNode} d */ function (d) {
      const w = words(d);
      const said = [w.qualifier, w.label, w.noun, value(d), ctx.classes.flags(d)];
      this.querySelectorAll(":scope > tspan").forEach((span, i) => {
        d3.select(span).datum(d).text(said[i]);
      });
    });
    // A cut label keeps its full words in a title.
    node.each(/** @this {SVGGElement} @param {LaidNode} d */ function (d) {
      d3.select(this).select(":scope > title").remove();
      if (!words(d).cut) return;
      d3.select(this).insert("title", ":first-child")
        .text(nameOf(d) + value(d));
    });

    // Glyphs are controls beside the marks, not inside, so no button nests.
    /** @type {Map<string, {kind: string, label: string, menu: boolean}>} */
    const glyphWords = new Map();
    for (const d of graph.nodes) {
      const w = ctx.glyph ? ctx.glyph(d) : null;
      if (w) glyphWords.set(d.id, w);
    }
    const glyphs = gGlyphs.selectAll(":scope > g")
      .data(graph.nodes.filter((d) => glyphWords.has(d.id)), /** @param {LaidNode} d */ (d) => d.id)
      .join((/** @type {any} */ enter) => {
        const g = enter.append("g");
        g.append("circle").attr("r", GLYPH_R);
        g.append("path");
        return g;
      })
      .attr("class", /** @param {LaidNode} d */ (d) => "glyph glyph-" + /** @type {any} */ (glyphWords.get(d.id)).kind)
      .attr("role", "button")
      .attr("tabindex", 0)
      .attr("aria-label", /** @param {LaidNode} d */ (d) => /** @type {any} */ (glyphWords.get(d.id)).label)
      .attr("aria-haspopup", /** @param {LaidNode} d */ (d) => (/** @type {any} */ (glyphWords.get(d.id)).menu ? "true" : null))
      .attr("aria-keyshortcuts", "Enter Space")
      .attr("transform", /** @param {LaidNode} d */ (d) => {
        const w = words(d);
        const at = glyphPlacement(slots, d, lastColumn, labelAdvance(w, value(d), ctx.classes.flags(d)));
        return "translate(" + at.x + "," + at.y + ")";
      })
      .on("click", /** @this {SVGGElement} @param {MouseEvent} e @param {LaidNode} d */ function (e, d) {
        const el = this;
        on.guarded("open or fold this mark", () => {
          e.stopPropagation();
          on.glyph(d, /** @type {any} */ (glyphWords.get(d.id)).kind, e.timeStamp, false, el);
        });
      })
      .on("keydown", /** @this {SVGGElement} @param {KeyboardEvent} e @param {LaidNode} d */ function (e, d) {
        if (e.key !== "Enter" && e.key !== " ") return;
        if (e.repeat) return;
        e.preventDefault();
        const el = this;
        on.guarded("open or fold this mark", () =>
          on.glyph(d, /** @type {any} */ (glyphWords.get(d.id)).kind, e.timeStamp, true, el));
      });
    glyphs.select("path").attr("d", /** @param {LaidNode} d */ (d) => {
      const arm = GLYPH_R - 2.5;
      const across = "M" + -arm + ",0H" + arm;
      return /** @type {any} */ (glyphWords.get(d.id)).kind === "open" ? across + "M0," + -arm + "V" + arm : across;
    });

    /** Marks leaving the chart: inert at once, gone at the end. @param {any} sel */
    const inert = (sel) => sel.attr("data-leaving", "").attr("aria-hidden", "true").attr("pointer-events", "none")
      .attr("tabindex", null).attr("role", null).on("click", null).on("keydown", null).on("contextmenu", null)
      .on("focus", null).on("pointerenter", null).on("pointermove", null);
    const finish = () => {
      layer.selectAll("*").interrupt(MOVE).interrupt(FADE);
      placeLinks(ribbons);
      placeNodes(node);
      label.attr("opacity", null);
      glyphs.attr("opacity", null);
      layer.selectAll("[data-leaving]").remove();
    };
    if (!t) {
      if (leavingLinks) leavingLinks.remove();
      if (leavingNodes) leavingNodes.remove();
      finish();
      return finish;
    }

    // Start: new marks at their ancestor's edge, labels hidden till the end.
    if (enteredNodes) {
      enteredNodes.each(/** @this {SVGGElement} @param {LaidNode} d */ function (d) {
        const from = grownFrom(d.id) || { x0: d.x0, x1: d.x0, y0: d.y0, y1: d.y1 };
        const x = edgeOf(d.id, now, from);
        const g = d3.select(this);
        g.select("rect").attr("x", x).attr("y", from.y0).attr("width", 0).attr("height", Math.max(2, from.y1 - from.y0));
        g.select("rect.contra-band").attr("x", x).attr("y", from.y0).attr("width", 0).attr("height", 0);
      });
    }
    if (enteredLinks) {
      enteredLinks.each(/** @this {SVGPathElement} @param {LaidLink} d */ function (d) {
        const id = !before.has(d.target.id) ? d.target.id : !before.has(d.source.id) ? d.source.id : "";
        const grown = id ? grownFrom(id) : null;
        d3.select(this).attr("d", grown ? flatAt(grown, edgeOf(id, now, grown)) : taper(d) ? taperPath(d) : path(d)).attr("stroke-width", 0);
      });
    }
    label.attr("opacity", 0);
    glyphs.attr("opacity", 0);
    placeLinks(moving(ribbons));
    placeNodes(moving(node));
    label.transition(FADE).delay(motion).duration(motion / 2).attr("opacity", 1)
      .on("end", /** @this {SVGTextElement} */ function () { this.removeAttribute("opacity"); });
    glyphs.transition(FADE).delay(motion).duration(motion / 2).attr("opacity", 1)
      .on("end", /** @this {SVGGElement} */ function () { this.removeAttribute("opacity"); });

    // Leaving marks end at their ancestor's edge.
    if (leavingNodes) {
      inert(leavingNodes);
      leavingNodes.select("text").attr("opacity", 0);
      leavingNodes.each(/** @this {SVGGElement} @param {LaidNode} d */ function (d) {
        const into = above(d.id, now) || d;
        const x = edgeOf(d.id, before, into);
        const g = d3.select(this).transition(t);
        g.select("rect").attr("x", x).attr("y", into.y0).attr("width", 0).attr("height", Math.max(2, into.y1 - into.y0));
        g.select("rect.contra-band").attr("x", x).attr("width", 0).attr("height", 0);
        g.remove();
      });
    }
    if (leavingLinks) {
      inert(leavingLinks);
      leavingLinks.each(/** @this {SVGPathElement} @param {LaidLink} d */ function (d) {
        const ends = [d.target && d.target.id, d.source && d.source.id];
        const gone = ends.find((id) => id && !now.has(id));
        const into = gone ? above(gone, now) : null;
        const p = d3.select(this).transition(t).attr("stroke-width", 0);
        if (gone && into) p.attr("d", flatAt(into, edgeOf(gone, before, into)));
        p.remove();
      });
    }
    return finish;
  },

  /** Colours the drawn ribbons and marks; the colours are the page's. */
  paint(ctx) {
    ctx.svg.selectAll("path.link")
      .attr("stroke", /** @param {LaidLink} d */ (d) => ctx.colour.link(d))
      // style.css fills bands from --ribbon: a fill attribute loses to the
      // unfilled-ribbon rule.
      .style("--ribbon", /** @param {LaidLink} d */ (d) => ctx.colour.link(d));
    ctx.svg.selectAll("g.node rect")
      .attr("fill", /** @param {LaidNode} d */ (d) => ctx.colour.node(d));
  },

});
