// @ts-check
/**
 * fisc — the Sankey form: what depends on the screen and on ribbons. The
 * expansion folded to a frontier, capped per split and stamped with its
 * slots, the reductions flipped so d3-sankey can draw them, and the layout
 * helpers. Every figure it draws is core.js's: a fold, a cap, a sum over
 * cited ribbons. It reads of a chart `chart.sankey`, and nothing of any
 * other form's.
 *
 * STATELESS BY RULE, as core.js is: no module-level `let`, no global read at
 * import. d3 is read through globalThis when a layout asks for it; the
 * frontier and the splits drawn whole are passed in by app.js, which holds
 * them. A type named here is one of app.js's typedefs.
 */

import {
  capColumn, fmt, fmtShortSigned, foldDocument, isExpansionSchedule, isFundGroup, isLink, labelledAsParent, openSplitOf, reducedOf, say, splitTailID, splitsOf, tailFigure, tierFrontier,
} from "./core.js";

export const NODE_WIDTH = 14;

export const NODE_PADDING = 14;

/** The surface gap that separates stacked ribbons, in px (1px each side). */
export const RIBBON_GAP = 2;

export const CHART_HEIGHT = 820;

/**
 * The canvas a chart is laid out on, in px: under a declared sankey.grow,
 * node_px for each node of the busiest slot of `nodes`, held between its
 * floor and its ceiling; else CHART_HEIGHT.
 * @param {FiscChart | null | undefined} chart
 * @param {{id: string, tier: number, slot?: number}[]} [nodes] what is drawn on it
 * @returns {number}
 */
export function canvasHeight(chart, nodes) {
  const hints = chart && chart.sankey;
  if (hints && hints.grow) {
    const g = hints.grow;
    return Math.min(g.ceiling, Math.max(g.floor, g.node_px * busiestSlot(nodes || [], hints.tiers || [])));
  }
  return CHART_HEIGHT;
}

/**
 * How many nodes the fullest column of `nodes` holds, placed by slotsOf.
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
 * A ribbon under this width on a declared canvas, in px, is drawn as a
 * hairline: a stroke one screen pixel wide whatever the zoom, which style.css
 * sets, under a wider transparent hit target the pointer can find.
 */
export const HAIRLINE = 1;

/**
 * Whether a ribbon is drawn as a hairline: under HAIRLINE on a chart that
 * declares its canvas (sankey.grow), the one kind of chart
 * whose thin ribbons are accepted rather than folded away. On the form's own
 * canvas a ribbon keeps the stroke floor every chart has.
 * @param {FiscChart | null | undefined} chart
 * @param {LaidLink} d
 * @returns {boolean}
 */
export function isHairline(chart, d) {
  const hints = chart && chart.sankey;
  return Boolean(hints && hints.grow) && d.width < HAIRLINE;
}

/**
 * Room either side of the plot for node labels, in px. Only the two end
 * columns anchor labels outward, so it does not grow with the column count.
 * labelWords fits an outward label to it; site/layout.test.mjs pins what
 * that fit draws.
 */
export const LABEL_GUTTER = 320;

/**
 * The clear run between one column's rects and the next's, in px, up to
 * HELD_COLUMNS columns.
 */
export const BAND = 319;

/**
 * The column count whose width a wider chart is held to: past it the bands
 * narrow (bandOf) rather than the chart widening.
 */
export const HELD_COLUMNS = 4;

/**
 * The narrowest band, in px. Past it the chart widens again. site/layout.test.mjs prints what an interior label keeps of its
 * name at this floor.
 */
export const BAND_FLOOR = 160;

/**
 * How wide a chart of `n` columns is laid out, in px: at BAND up to
 * HELD_COLUMNS, then held at that width while the bands narrow to
 * BAND_FLOOR, then at the floor. Labels do not reflow, so the chart is laid
 * out at a fixed width, and style.css draws every width at the one scale the
 * opening chart fits its card at.
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
 * The clear run between one column's rects and the next's in a chart of `n`
 * columns, in px, as chartWidth lays them out.
 * @param {number} n
 * @returns {number}
 */
export function bandOf(n) {
  const columns = Math.max(2, n);
  return (chartWidth(columns) - 2 * LABEL_GUTTER - NODE_WIDTH * columns) / (columns - 1);
}

/**
 * Draws every ribbon the schedule prints as a reduction forward, at its
 * magnitude, carrying the sentence the document put on it.
 *
 * A RIBBON IS NEVER REVERSED: a backward ribbon adds a column d3-sankey's
 * layering pass throws on. The contra words are the document's, from Go.
 *
 * The two arms here are about the drawn chart only: a fold sums ribbons, so a
 * merged ribbon can go negative with no printed reduction (named for what it
 * is) or positive despite one (loses the sentence).
 *
 * @param {FiscProjection} drawn  shaped and folded
 * @returns {FiscProjection} drawn itself when nothing in it is negative
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
 * The chart as its documents print it, markContra undone: a link carrying a
 * contra sentence after markContra was printed negative, so it goes back to
 * its own sign, so a chart as drawn can be summed at the signs printed.
 * @param {FiscProjection} drawn
 * @returns {FiscProjection}
 */
export function unmarkContra(drawn) {
  if (!drawn.links.some((l) => l.contra && l.value_cents > 0)) return drawn;
  return Object.assign({}, drawn, {
    links: drawn.links.map((l) => l.contra && l.value_cents > 0
      ? Object.assign({}, l, { value_cents: -l.value_cents })
      : l),
  });
}

/**
 * Whether every ribbon on a laid node is a contra one: a line the schedule
 * prints as a reduction, whose own figure is therefore negative.
 * @param {LaidNode} d
 * @returns {boolean}
 */
export function isContraNode(d) {
  const links = d.sourceLinks.concat(d.targetLinks);
  return links.length > 0 && links.every((l) => Boolean(l.contra));
}

/**
 * Whether every ribbon on a laid node is a partition one, so the mark's whole
 * figure is a cross-tab total rather than money that moved through it. All or
 * nothing, as isContraNode is.
 * @param {LaidNode} d
 * @returns {boolean}
 */
export function isPartitionNode(d) {
  const links = d.sourceLinks.concat(d.targetLinks);
  return links.length > 0 && links.every((l) => Boolean(l.partition));
}

/**
 * The figure a laid mark prints: negative for a contra ribbon and for a line
 * whose every ribbon is one, and d3's value otherwise.
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
 * The region a mark's arriving ribbons hang into, for a mark printed net of
 * reductions drawn forward, and null for every other mark.
 *
 * The box is the published net figure, so the gross arriving stack overhangs
 * it; drawing the overhang avoids a box sized to a figure no page prints. It
 * is pixels and carries no cents.
 *
 * @param {LaidNode} d
 * @returns {{y:number, height:number} | null}
 */
export function contraBand(d) {
  if (!d.targetLinks.some((l) => l.contra)) return null;
  const arriving = d.targetLinks.reduce((sum, l) => sum + l.width, 0);
  const excess = arriving - (d.y1 - d.y0);
  // Half a pixel, not zero: no hairline band for reductions that round away.
  return excess > 0.5 ? { y: d.y1, height: excess } : null;
}

/**
 * How much of a mark's figure is printed as reductions, for a node with contra
 * ribbons among others, and "" for every other node -- a contra line's own
 * mark included, whose figure is the reduction and is already signed.
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
 * Re-stacks each node's ribbons in the order of the ends they run to.
 *
 * d3-sankey sorts before its last relaxation move, so ribbons can cross at the
 * node face for no reason in the data. Widths are untouched.
 * @param {{nodes:LaidNode[], links:LaidLink[]}} graph
 */
export function restackLinks(graph) {
  /** @param {(l:LaidLink) => LaidNode} end */
  const byOtherEnd = (end) =>
    /** @param {LaidLink} a @param {LaidLink} b */ (a, b) =>
      end(a).y0 - end(b).y0 || a.index - b.index;

  for (const node of graph.nodes) {
    node.sourceLinks.sort(byOtherEnd((l) => l.target));
    // CONTRA LAST ON THE ARRIVING SIDE, so reductions stack inside the band.
    // A tiebreak, not a reorder: nodeRank's crossing count rests on that.
    node.targetLinks.sort((a, b) =>
      Number(Boolean(a.contra)) - Number(Boolean(b.contra)) ||
      byOtherEnd((l) => l.source)(a, b));
  }
  for (const node of graph.nodes) {
    let leaving = node.y0;
    for (const l of node.sourceLinks) {
      l.y0 = leaving + l.width / 2;
      leaving += l.width;
    }
    let arriving = node.y0;
    for (const l of node.targetLinks) {
      l.y1 = arriving + l.width / 2;
      arriving += l.width;
    }
  }
}

/**
 * How many extend splits deep one spine column's slots may run: a slot's
 * order key is its spine column times this, plus its depth.
 */
export const SLOT_DEPTH = 10;

/**
 * THE PLACEMENT: the column each node is drawn in, by id, numbered from 0 with
 * no column empty. A node's order key is its `slot` where its shaping stamped
 * one (expansionOf), and otherwise its tier's place in `tiers`, the tier set
 * on screen being one slot assignment among others. The keys present are
 * ranked, so a column nothing is placed in is not one, and d3-sankey, which
 * throws on an empty leading or interior column, is never handed one.
 *
 * A node at a tier `tiers` does not hold, with no slot, is left out; with no
 * tiers and no slots the map is empty and d3 justifies.
 *
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
 * The slot order key of every node of the expansion schedule, read off its
 * parent chain in the UNFOLDED schedule: a root at its tier's place in the
 * spine's column order, and a member in the slot directly after its
 * parent's. Every split from one slot opens into the same next slot.
 *
 * FAILS CLOSED on a root at a tier the spine does not draw, a member of no
 * split the schedule declares, a member of a split of any form but extend,
 * and an extend chain deeper than SLOT_DEPTH.
 *
 * @param {FiscProjection} expansion
 * @param {number[]} tiers the spine's column order
 * @returns {Map<string, number>}
 */
export function expansionSlots(expansion, tiers) {
  const byID = new Map(expansion.nodes.map((n) => [n.id, n]));
  /** @type {Map<string, string>} */
  const formOf = new Map();
  /** @type {Map<string, string>} */
  const primary = new Map();
  for (const sp of /** @type {any[]} */ ((/** @type {any} */ (expansion)).splits || [])) {
    formOf.set(sp.node + "\u001f" + sp.key, sp.form);
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
      slot = column * SLOT_DEPTH;
    } else {
      const up = byID.get(n.parent);
      if (!up || hops > 8) throw new Error("cannot place " + n.id + ": its parent chain does not reach a root");
      const key = n.alternative || primary.get(up.id) || "";
      const form = formOf.get(up.id + "\u001f" + key);
      const above = slotOf(up, hops + 1);
      if (form === "extend" && (above % SLOT_DEPTH) + 1 < SLOT_DEPTH) slot = above + 1;
      else {
        throw new Error("cannot place " + n.id + ": it is a member of " + up.id + "'s split " +
          JSON.stringify(key) + ", " + (!form ? "which the schedule does not declare"
            : form !== "extend" ? "declared " + JSON.stringify(form) + ", and this page draws a split only as extend"
              : "extended past " + SLOT_DEPTH + " slots"));
      }
    }
    out.set(n.id, slot);
    return slot;
  };
  for (const n of expansion.nodes) slotOf(n, 0);
  return out;
}

/**
 * The expansion schedule as drawn at a frontier: the one fold, each open
 * split whose key sankey.split_caps caps and that is not in `whole` folded
 * to its largest members and a tail (capColumn over that split's drawn
 * members alone), the tails' parallel ribbons merged by folding again onto
 * every node now drawn, and every node stamped with its slot. A tail sits in
 * its members' slot, hangs from the node they split, and its note is finished
 * with the figure only the folded document knows.
 *
 * @param {FiscProjection} expansion the expansion schedule
 * @param {Iterable<string>} frontier
 * @param {FiscChart} chart the overview, whose tiers are the spine's columns
 * @param {ReadonlySet<string>} whole the splits drawn whole rather than
 *   capped, each node\u001fkey
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
 * Which column d3-sankey puts a node in: its slot (slotsOf), or d3's own
 * justify when nothing places any node.
 *
 * Justify puts a link-less sink in the last column, wrong once columns can be
 * skipped. A node the slots do not place is answered -1, which d3 clamps
 * into the first column.
 *
 * @param {Map<string, number>} slots
 * @returns {(d: LaidNode) => number}
 */
export function alignFor(slots) {
  return slots.size
    ? /** @param {LaidNode} d */ (d) => { const at = slots.get(d.id); return at === undefined ? -1 : at; }
    : /** @type {any} */ (globalThis).d3.sankeyJustify;
}

/**
 * The column a node was drawn in: its slot.
 *
 * NOT d.depth, the longest path to the node, which differs wherever no ribbon
 * reaches a node from the column before it. Not d.layer either, except in a
 * view that places nothing, where sankeyJustify chose the columns.
 *
 * @param {Map<string, number>} slots
 * @param {LaidNode} d
 * @returns {number}
 */
export function columnOf(slots, d) {
  // d3 clamps an unplaced node into column 0, so it is labelled as one.
  return slots.size ? Math.max(0, slots.get(d.id) === undefined ? 0 : /** @type {number} */ (slots.get(d.id))) : d.layer;
}

/**
 * Where a node's label goes: the side it is anchored on, and the point.
 *
 * A LABEL MAY RUN OUTWARD ONLY INTO A GUTTER, and only the first and last
 * columns have one. An interior label is centred above its rect, in the
 * NODE_PADDING gap, rather than across the ribbons the next column receives.
 *
 * @param {Map<string, number>} slots the placement (slotsOf)
 * @param {LaidNode} d
 * @param {number} last the largest column index this chart drew
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
 * Where a node's ⊕ or ⊖ is centred: beside its rect, mid-height, on the side
 * its label is not -- the ribbons' side, right of the rect, and left of it in
 * the last column, whose labels run right into the gutter. An interior rect
 * too short to hold it between its own label above and the next label below
 * has it at the end of its label's line instead, past the label's width.
 *
 * @param {Map<string, number>} slots the placement (slotsOf)
 * @param {LaidNode} d
 * @param {number} last the largest column index this chart drew
 * @param {number} width how wide the node's label draws (labelAdvance)
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
 * Each mark's parent label, on every mark whose label another mark in the same
 * column also carries; nothing elsewhere.
 *
 * AN AMBIGUITY IS A PROPERTY OF THE COLUMN, NOT THE NODE, so it is decided here
 * rather than in the document. The parent is the one `doc` gives the mark, and
 * is looked up there, not in the laid graph: the fold blanks the parent of a
 * node whose parent is split open, and the chart does not draw it. A mark
 * `doc` does not carry, a tail, keeps the parent it was laid with. A duplicate
 * whose parent is unnameable gets "" and stays ambiguous rather than invented.
 *
 * @param {LaidNode[]} nodes
 * @param {{nodes: FiscNode[]} | null} doc the unfolded nodes the chart was
 *   folded from
 * @param {Map<string, number>} slots the placement (slotsOf)
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
 * The noun each mark is told apart from its parent by, on every mark the
 * document prints in its parent's words; nothing elsewhere. An extended
 * split draws its node and its members side by side, so a member labelled as
 * its parent would read as the same thing twice.
 *
 * The parent is the one `doc` gives the mark, as labelQualifiers reads it.
 * The noun is `nounOf`'s, the page's words for one member of the split the
 * mark belongs to; a mark it names nothing for keeps its label alone.
 *
 * @param {{nodes: FiscNode[]} | null} doc the unfolded nodes the chart was
 *   folded from
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

/**
 * The advance a label's words are fitted at, in em: an over-estimate on
 * purpose, wider than any usual face sets `system-ui`, so the error runs
 * toward cutting a label that would have fitted.
 */
export const ADVANCE_EM = 0.6;

/** The px style.css sets a label's words and its flags at. */
export const LABEL_PX = 12;

/** The px style.css sets a label's value at. */
export const VALUE_PX = 11;

/**
 * How wide `text` draws at `px` at most, by ADVANCE_EM.
 * @param {string} text
 * @param {number} px
 * @returns {number}
 */
export function textAdvance(text, px) {
  return text.length * px * ADVANCE_EM;
}

/**
 * How wide a mark's label draws at most, by ADVANCE_EM: the qualifier, label
 * and noun tspans at LABEL_PX on one line, and the value and flags after them.
 * @param {{qualifier: string, label: string, noun?: string}} words what labelWords draws
 * @param {string} value the value tspan's words
 * @param {string} flags the flag tspan's words
 * @returns {number}
 */
export function labelAdvance(words, value, flags) {
  return textAdvance(words.qualifier + words.label + (words.noun || ""), LABEL_PX) +
    textAdvance(value, VALUE_PX) + textAdvance(flags, LABEL_PX);
}

/**
 * The run a label may draw over, in px: from an outward label's anchor to
 * the edge of the chart, and for an interior label, centred on its rect, its
 * column's rect and half the band either side (bandOf).
 * @param {{anchor: string, x: number}} place labelPlacement's answer
 * @param {number} columns how many columns the chart is laid out in
 * @returns {number}
 */
export function labelRoom(place, columns) {
  if (place.anchor === "end") return place.x;
  if (place.anchor === "start") return chartWidth(columns) - place.x;
  return bandOf(columns) + NODE_WIDTH;
}

/**
 * The words a mark's qualifier, label and noun tspans draw, fitted to `room`.
 *
 * EVERY LABEL IS ONE LINE, its qualifier inline as "Qualifier: " and its noun
 * after it as " (noun)", because d3 holds only NODE_PADDING between two marks
 * and canvasHeight counts marks, not lines. Where it is wider than its room
 * by `measure`, the middle of its OWNER is cut for an ellipsis: the label's
 * words before its last ": ", else the qualifier, else, beside a noun, the
 * label. The ": Category" after it, the noun and the value are kept, and so
 * are the owner's first word and as much of its end as fits, since funds that
 * share a prefix differ at the end ("County Meas BB-Bike/Pedestrian", "County
 * Meas BB-Local St & Rd"). Where the owner cut to two characters still does
 * not fit, the whole line is cut as one owner, each tspan keeping what is
 * left of its own words; a label too wide even then is drawn so, and the room
 * checks under site/ see it.
 *
 * The text stays Go's; this only fits it to the screen.
 *
 * @param {number} room labelRoom
 * @param {string} qualifier labelQualifiers' answer for the mark, or ""
 * @param {string} label
 * @param {(words: {qualifier: string, label: string, noun: string}) => number} measure
 *   how wide the whole label draws in these words, value and flags included
 * @param {string} [noun] labelNouns' answer for the mark, or ""
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
 * What each of `parts` keeps of its own words where their join is cut to
 * `head`, an ellipsis and `tail` (cutOwner), the ellipsis in the part the
 * cut opens in; every part whole where `tail` is null.
 * @param {string[]} parts
 * @param {string} head a prefix of the join
 * @param {string | null} tail a suffix of the join, or null where nothing is cut
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
 * The most of `owner`'s characters, at least two, that `words` draws within
 * `room`: its first word leads and its end takes the rest, since names here
 * share a prefix ("Grant - ", "County Meas BB-") more than an end.
 * `words` is handed the owner as drawn, and what of its front and its end it
 * keeps either side of the ellipsis; its end is null where nothing is cut.
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
 * The share one mark is of the money in its column, as a percentage.
 *
 * IT IS ARITHMETIC AND SAYS SO: no page prints it. Computed from the DRAWN
 * values. A printed reduction (isContraNode) is drawn at its size but is no
 * part of the column's money, so it has no share and is left out of every
 * other mark's total.
 *
 * @param {LaidNode} d
 * @param {LaidNode[]} laid every laid node of the chart
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
  const pct = (100 * d.value) / total;
  // A CEILING AS WELL AS A FLOOR: toFixed(1) would round a divided column's
  // largest share to "100.0", a whole its sibling denies.
  const shown = pct < 0.1 ? "<0.1" : pct > 99.9 ? ">99.9" : pct.toFixed(1);
  // "our" is what survives the diamond being read aloud.
  return "\u25c7 our " + shown + "% of this column";
}

/**
 * Sort key inside a column. A fund group is its own place; anything else is
 * the value-weighted mean place of the fund groups it touches (a barycentre),
 * or, touching none, its own group's place.
 * Supplying a nodeSort is what pins the fund column to the palette's order.
 * The layout tests under site/ measure the alternatives and pin this one.
 * Ties fall to the caller's tie-break on value.
 * @param {LaidNode} node
 * @param {(node: FiscNode | LaidNode) => string} groupOf a node's fund group
 * @param {(id: string) => number} placeOf a fund group's place in the drawn order
 * @returns {number}
 */
export function nodeRank(groupOf, placeOf, node) {
  if (isFundGroup(node)) return placeOf(node.id);

  let weight = 0;
  let place = 0;
  for (const l of node.sourceLinks.concat(node.targetLinks)) {
    const other = l.source === node ? l.target : l.source;
    // The neighbour's fund group, not the neighbour: once a group splits, the
    // ends are funds and divisions.
    const group = groupOf(other);
    // A node in no fund group is ignored rather than counted as position zero.
    if (!group) continue;
    const at = placeOf(group);
    place += at * l.value;
    weight += l.value;
  }
  if (weight) return place / weight;
  // NO NEIGHBOUR IN A GROUP: a node whose every neighbour is in no fund
  // group rests at its own group's place, which its parent chain names. A
  // node in no group either sorts to the top, and its own value breaks the tie.
  const own = groupOf(node);
  return own ? placeOf(own) : 0;
}

/**
 * The links layOutAt hands d3-sankey beside the drawn ones so a path runs
 * through every column the slots declare: none where the drawn ribbons
 * already reach the last, and otherwise one zero-value link at each column
 * no drawn path reaches, from a node the path does reach in the column before.
 *
 * d3-sankey draws one column per step of the longest path, plus one, and
 * nodeAlign can pick among those columns but never add one; two extend splits
 * open together can declare a slot further right than any ribbon reaches.
 * A zero-value link adds no height and pulls no node in d3's relaxation, but
 * d3 still spaces it among its ends' ribbons, so it can nudge where their
 * other ribbons aim: none is added where a drawn path already spans the slots.
 *
 * NONE where a drawn ribbon does not run forward in the slots: a scaffold
 * there could close a cycle, which d3 throws on, so the clamp check is left
 * to refuse the layout.
 *
 * @param {FiscProjection} drawn
 * @param {Map<string, number>} slots slotsOf
 * @returns {{source: string, target: string, value: number, scaffold: true}[]}
 */
export function scaffoldOf(drawn, slots) {
  const ranks = new Set(slots.values()).size;
  if (ranks < 2 || drawn.links.some((l) => !(/** @type {number} */ (slots.get(l.source)) < /** @type {number} */ (slots.get(l.target))))) return [];
  /** @type {{source: string, target: string, value: number, scaffold: true}[]} */
  const out = [];
  const placed = drawn.nodes.filter((n) => slots.has(n.id))
    .sort((a, b) => /** @type {number} */ (slots.get(a.id)) - /** @type {number} */ (slots.get(b.id)));
  // The longest path into each node, over every link so far: each link runs
  // forward in the slots, so one pass in slot order is the whole answer.
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
 * Where each node of a redrawn chart sat in the chart it replaces, as a
 * vertical place to order its column by: a node laid before at its own
 * centre, a new one at the centre of its nearest ancestor laid before
 * (parentOf, one step up), so a split's members take their parent's place.
 * Only a column every node of which has a place is seated; any other, and
 * every column of a chart with nothing before it, is ordered by nodeRank
 * alone, so a fresh chart is laid exactly as it always was.
 * @param {{id: string, tier: number, slot?: number}[]} nodes
 * @param {Map<string, number>} slots slotsOf
 * @param {Map<string, {y0: number, y1: number}>} [before] the replaced chart's laid nodes, by id
 * @param {(id: string) => string} [parentOf]
 * @returns {Map<string, number>} id to place, seated columns only
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
 * SANKEY.layOut at a stated canvas height, in px: what the layout test
 * measures a chart at heights the chart does not declare with.
 *
 * d3 counts columns from the longest path, so the scaffold (scaffoldOf)
 * runs a path through every slot first.
 *
 * FAILS CLOSED on a node with a stamped slot (expansionOf) that d3 laid in
 * another column: d3 clamps a node placed beyond its last column, which would
 * draw it a column early in silence. Only a ribbon that does not run forward
 * in the slots, which no scaffold is added beside, leaves that reachable.
 * @param {FiscProjection} drawn
 * @param {{tiers: number[], columns: number, groupOf: (n: any) => string, placeOf: (id: string) => number,
 *   before?: Map<string, {y0: number, y1: number}>, parentOf?: (id: string) => string}} ctx before and
 *   parentOf seat a redrawn chart's columns (seatsOf)
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
  // THE SCAFFOLD LEAVES BEFORE ANYTHING READS A RIBBON: it is never drawn,
  // stacked or counted.
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
  restackLinks(graph);
  return graph;
}

/** The transition a redraw moves its marks in, and the one its labels fade in after. */
const MOVE = "chart-move";
const FADE = "label-fade";

/** A label's tspans in order, by class: qualifier, label, noun, value, flag. */
const LABEL_TSPANS = ["qualifier", "", "qualifier", "value", "flag"];

/**
 * A laid ribbon's id: its ends, its kind, and whether it is a cross-tab
 * slice, which is what one ribbon of a shaped chart is drawn per.
 * @param {{source: any, target: any, kind: string, partition?: boolean}} d
 * @returns {string}
 */
export function linkKey(d) {
  const end = (/** @type {any} */ x) => (x && typeof x === "object" ? x.id : x);
  return end(d.source) + "→" + end(d.target) + "\u001f" + d.kind + (d.partition ? "\u001fpartition" : "");
}

/**
 * Each ribbon's id (linkKey), a second ribbon of one id numbered after it,
 * so no two ribbons of a chart share an element.
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
 * The Sankey form's renderer, as app.js's FORMS holds it under "sankey".
 * Every method takes what it needs: the chart's declaration, the frontier,
 * the document on screen. It reads of a chart `chart.sankey` and nothing of
 * any other form's hints; site/layout.test.mjs wraps
 * the hints in a recording Proxy and holds every read to this file.
 * @type {FormRenderer}
 */
export const SANKEY = Object.freeze({
  form: "sankey",

  /**
   * The overview's declared tiers, left to right: the spine's columns, which
   * a root of the expansion is placed by.
   */
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

  /**
   * The words a split's tail counts its members in, as the chart's
   * sankey.split_caps declares them for that split key, or null where it
   * caps no such split.
   */
  tailWord(chart, key) {
    const caps = (chart.sankey && chart.sankey.split_caps) || [];
    const cap = caps.find((/** @type {{split: string, tail: string}} */ c) => c.split === key);
    return cap ? cap.tail : null;
  },

  /**
   * How many columns a shaped chart is laid out in: its distinct slots
   * (slotsOf), at least one.
   */
  columnsOf(drawn, tiers) {
    return Math.max(1, new Set(slotsOf(drawn.nodes, tiers).values()).size);
  },

  /**
   * Fits a document to the screen, then flips the reductions (markContra): an
   * expansion schedule folded to the frontier and its open splits capped
   * (expansionOf), any other document folded to the chart's tier set.
   */
  shape(doc, view) {
    if (isExpansionSchedule(doc)) return markContra(expansionOf(doc, view.frontier, view.chart, view.whole));
    return markContra(foldDocument(doc, tierFrontier(doc, SANKEY.tiersOf(view.chart))));
  },

  /**
   * Lays a shaped document out with d3-sankey, pure of the page: the columns
   * on screen, the group and place lookups and the constants are all it
   * reads. d3-sankey mutates its input, so it gets a copy and the document
   * stays the thing the table and the detail panel read from.
   */
  layOut(drawn, ctx) {
    return layOutAt(drawn, ctx, canvasHeight(ctx.chart, drawn.nodes));
  },

  /** How many laid ribbons of this chart are drawn as hairlines: isHairline. */
  hairlines(graph, chart) {
    return graph.links.filter((l) => isHairline(chart, l)).length;
  },

  /**
   * Draws a laid graph into ctx.layer, the group the page keeps inside
   * ctx.svg and moves as the reader zooms: ribbons, marks, bands and labels, with
   * the classes, descriptions, gestures and the figure each label says
   * (ctx.figure) the page hands it. Paints nothing: the page paints after, so
   * a theme change repaints without redrawing.
   *
   * KEYED BY NODE AND RIBBON ID (linkKey), so a mark the chart it replaces
   * drew keeps its element. Over ctx.motion ms, from ctx.before (the replaced
   * chart's laid nodes by id), a surviving mark glides to its place, a new
   * one grows out of the right edge of its nearest ancestor drawn before
   * (parentOf), one leaving shrinks into the right edge of its nearest
   * ancestor drawn now, and every label fades in after the move. With no
   * motion everything is at its end at once. The viewBox is never shorter
   * than ctx.held. Returns what puts every mark at its end now.
   * @returns {() => void}
   */
  render(graph, ctx) {
    const d3 = /** @type {any} */ (globalThis).d3;
    const svg = ctx.svg;
    const width = chartWidth(ctx.columns);
    const height = Math.max(canvasHeight(ctx.chart, graph.nodes), ctx.held || 0);
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
    const keys = linkKeys(graph.links);
    /** @param {LaidLink} d */
    const keyOf = (d) => keys.get(d) || linkKey(d);
    const path = d3.sankeyLinkHorizontal();

    const now = new Map(graph.nodes.map((n) => [n.id, n]));
    /**
     * The box of id's nearest ancestor among `boxes`, or null.
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
    /** A ribbon at no width at a box's right edge, mid-height. */
    const flatAt = (/** @type {{x1: number, y0: number, y1: number}} */ box) => {
      const y = (box.y0 + box.y1) / 2;
      return path({ source: { x1: box.x1 }, target: { x0: box.x1 }, y0: y, y1: y });
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
      .attr("class", /** @param {LaidLink} d */ (d) => ctx.classes.link(d) + (hairline(d) ? " hairline" : ""))
      .attr("tabindex", 0)
      .attr("role", "button")
      .attr("aria-hidden", null)
      .attr("aria-label", /** @param {LaidLink} d */ (d) => ctx.describe.link(d))
      .on("pointerenter", /** @param {PointerEvent} e @param {LaidLink} d */ (e, d) => { on.tip(e, d); on.trace(d); })
      .on("pointermove", /** @param {PointerEvent} e @param {LaidLink} d */ (e, d) => on.tip(e, d))
      .on("pointerleave", () => { on.hide(); on.untrace(); })
      .on("focus", /** @param {FocusEvent} e @param {LaidLink} d */ (e, d) => on.guarded("show this flow", () => { if (on.restoring()) return; on.tip(e, d); on.pin(d); }))
      .on("blur", on.hide)
      .on("click", /** @param {MouseEvent} e @param {LaidLink} d */ (e, d) => on.guarded("pin this flow", () => { e.stopPropagation(); on.pin(d); }));
    /** @param {any} sel */
    const placeLinks = (sel) => sel
      .attr("d", path)
      // The surface gap, not a stroke, separates stacked ribbons.
      .attr("stroke-width", /** @param {LaidLink} d */ (d) => Math.max(1, d.width - RIBBON_GAP));

    // A HAIRLINE'S HIT TARGET IS WIDER THAN ITS STROKE: the ribbon keeps its
    // focus and its description, and this transparent stroke above it takes
    // the pointer's hover and click. Drawn at its end: it is never seen.
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
    const node = gNodes.selectAll("g")
      .data(graph.nodes, /** @param {LaidNode} d */ (d) => d.id)
      .join(
        (/** @type {any} */ enter) => {
          const g = enter.append("g");
          g.append("rect").attr("rx", 2);
          // On every mark, displayed on the ones that hang: the vendored d3
          // selection has no filter(). pointer-events none, because the band
          // is not the mark.
          g.append("rect").attr("class", "contra-band").attr("pointer-events", "none");
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
      // aria-pressed is the isolation, on every node. SPLITTING IS NOT THE
      // TOGGLE and must never be announced as one: the label announces what
      // a node splits into, and ⊖ beside it, not the node, folds it back.
      .attr("aria-pressed", "false")
      .attr("aria-label", /** @param {LaidNode} d */ (d) => ctx.describe.node(d))
      // Both keys are the one activation a click is.
      .attr("aria-keyshortcuts", "Enter Space")
      .on("pointerenter", /** @param {PointerEvent} e @param {LaidNode} d */ (e, d) => { on.tip(e, d); on.trace(d); })
      .on("pointermove", /** @param {PointerEvent} e @param {LaidNode} d */ (e, d) => on.tip(e, d))
      .on("pointerleave", () => { on.hide(); on.untrace(); })
      .on("focus", /** @param {FocusEvent} e @param {LaidNode} d */ (e, d) => on.guarded("show this mark", () => { if (on.restoring()) return; on.tip(e, d); on.pin(d); }))
      .on("blur", on.hide)
      // ONE ACTIVATION, by click, Enter or Space: it opens a node that opens
      // and pins one that does not.
      //
      // Both click and keydown, because an SVG g[role=button] synthesises no
      // click from Enter. A held key is ignored.
      .on("click", /** @param {MouseEvent} e @param {LaidNode} d */ (e, d) => {
        on.guarded("open this mark", () => {
          e.stopPropagation();
          on.click(d);
        });
      })
      .on("keydown", /** @param {KeyboardEvent} e @param {LaidNode} d */ (e, d) => {
        if (e.key !== "Enter" && e.key !== " ") return;
        if (e.repeat) return;
        e.preventDefault();
        on.guarded("open this mark", () => on.key(d));
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
    };
    node.select("rect.contra-band")
      .attr("display", /** @param {LaidNode} d */ (d) => (contraBand(d) ? null : "none"));

    // Every node is directly labelled: the relief the palette's contrast check
    // requires.
    const slots = slotsOf(graph.nodes, ctx.tiers);
    const place = (/** @type {LaidNode} */ d) => labelPlacement(slots, d, lastColumn);
    const lastColumn = Math.max(...graph.nodes.map((d) => columnOf(slots, d)));
    const qualified = labelQualifiers(ctx.doc, slots, graph.nodes);
    /** @param {LaidNode} d */
    const qualifierOf = (d) => qualified.get(d.id) || "";
    const nouns = labelNouns(ctx.doc, graph.nodes, ctx.noun);
    /** @param {LaidNode} d */
    const nounOf = (d) => nouns.get(d.id) || "";
    /** @param {LaidNode} d */
    const value = (d) => "  " + fmtShortSigned(ctx.figure(d));
    /** @type {Map<string, {qualifier: string, label: string, noun: string, cut: boolean}>} */
    const fitted = new Map();
    // Fitted at ADVANCE_EM, not read off the browser: the words drawn are the
    // words site/layout.test.mjs measures against their room.
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
    // The noun is the page's words, not the city's, so it is drawn as the
    // qualifier is.
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
        .text((qualifierOf(d) ? qualifierOf(d) + ": " : "") + d.label + (nounOf(d) ? " (" + nounOf(d) + ")" : "") + value(d));
    });

    // THE GLYPHS ARE CONTROLS OF THEIR OWN, beside the marks and not inside
    // them, so no button is nested in another: ⊕ opens, ⊖ folds back, each
    // in the words ctx.glyph gives it.
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
      .attr("tabindex", null).attr("role", null).on("click", null).on("keydown", null)
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

    // THE START: a new mark at its ancestor's right edge, a surviving one
    // where it stands, every label hidden until the move is over.
    if (enteredNodes) {
      enteredNodes.each(/** @this {SVGGElement} @param {LaidNode} d */ function (d) {
        const from = grownFrom(d.id) || { x1: d.x0, y0: d.y0, y1: d.y1 };
        const g = d3.select(this);
        g.select("rect").attr("x", from.x1).attr("y", from.y0).attr("width", 0).attr("height", Math.max(2, from.y1 - from.y0));
        g.select("rect.contra-band").attr("x", from.x1).attr("y", from.y0).attr("width", 0).attr("height", 0);
      });
    }
    if (enteredLinks) {
      enteredLinks.each(/** @this {SVGPathElement} @param {LaidLink} d */ function (d) {
        const grown = !before.has(d.target.id) ? grownFrom(d.target.id) : !before.has(d.source.id) ? grownFrom(d.source.id) : null;
        d3.select(this).attr("d", grown ? flatAt(grown) : path(d)).attr("stroke-width", 0);
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

    // THE END OF A MARK LEAVING: into the right edge of its ancestor drawn now.
    if (leavingNodes) {
      inert(leavingNodes);
      leavingNodes.select("text").attr("opacity", 0);
      leavingNodes.each(/** @this {SVGGElement} @param {LaidNode} d */ function (d) {
        const into = above(d.id, now) || d;
        const g = d3.select(this).transition(t);
        g.select("rect").attr("x", into.x1).attr("y", into.y0).attr("width", 0).attr("height", Math.max(2, into.y1 - into.y0));
        g.select("rect.contra-band").attr("x", into.x1).attr("width", 0).attr("height", 0);
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
        if (into) p.attr("d", flatAt(into));
        p.remove();
      });
    }
    return finish;
  },

  /** Colours the drawn ribbons and marks; the colours are the page's. */
  paint(ctx) {
    ctx.svg.selectAll("path.link")
      .attr("stroke", /** @param {LaidLink} d */ (d) => ctx.colour.link(d));
    ctx.svg.selectAll("g.node rect")
      .attr("fill", /** @param {LaidNode} d */ (d) => ctx.colour.node(d));
  },

});
