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
  capColumn, fmt, fmtShortSigned, foldDocument, isExpansionSchedule, isFundGroup, isLink, openSplitOf, reducedOf, say, splitTailID, splitsOf, tailFigure, tierFrontier,
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
 * @param {FiscChart | FiscDrillStep | null | undefined} chart
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
 * @param {FiscChart | FiscDrillStep | null | undefined} chart
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
 */
export const LABEL_GUTTER = 250;

/**
 * The clear run between one column's rects and the next's, in px. Held fixed
 * as columns are added; 319 makes chartWidth(3) main's 1180px.
 */
export const BAND = 319;

/**
 * How wide a chart of `n` columns is laid out, in px. Labels do not reflow, so
 * the chart is laid out at a fixed width and scaled by the viewBox; the column
 * budget is asked of the viewport (COLUMN_QUERIES) for that reason.
 * @param {number} n
 * @returns {number}
 */
export function chartWidth(n) {
  // d3-sankey divides by (columns - 1); clamp so the width stays finite.
  const columns = Math.max(1, n);
  return 2 * LABEL_GUTTER + BAND * (columns - 1) + NODE_WIDTH * columns;
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
 * parent chain in the UNFOLDED schedule (the fold blanks a refine member's
 * parent, which is opened and not drawn): a root at its tier's place in the
 * spine's column order, a refine member in its parent's slot, and an extend
 * member in the slot directly after its parent's. Every extend from one slot
 * opens into the same next slot.
 *
 * FAILS CLOSED on a root at a tier the spine does not draw, a member of no
 * split the schedule declares, and an extend chain deeper than SLOT_DEPTH.
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
      if (form === "refine") slot = above;
      else if (form === "extend" && (above % SLOT_DEPTH) + 1 < SLOT_DEPTH) slot = above + 1;
      else {
        throw new Error("cannot place " + n.id + ": it is a member of " + up.id + "'s split " +
          JSON.stringify(key) + ", " + (form ? "extended past " + SLOT_DEPTH + " slots" : "which the schedule does not declare"));
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
 * The dy each of a qualified label's two lines is drawn at, relative to the
 * line before it. The qualifier goes above: an outward pair straddles the
 * rect's middle; an interior label has nowhere below to go, so the qualifier
 * is lifted a whole line and the label stays put.
 *
 * NO COMMITTED VIEW DRAWS THE INTERIOR BRANCH, so no check sees it; fisc-xhqt.
 *
 * @param {string} anchor
 * @returns {{qualifier: string, label: string}}
 */
export function labelLineShift(anchor) {
  return anchor === "middle"
    ? { qualifier: "-1.15em", label: "1.15em" }
    : { qualifier: "-0.6em", label: "1.15em" };
}

/**
 * The share one mark is of the money in its column, as a percentage.
 *
 * IT IS ARITHMETIC AND SAYS SO: no page prints it. Computed from the DRAWN
 * values, a residual's included. A printed reduction (isContraNode) is drawn
 * at its size but is no part of the column's money, so it has no share and
 * is left out of every other mark's total.
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
  // NO NEIGHBOUR IN A GROUP: the residual, whose neighbours are all
  // endpoints, rests at its own group's place, which its parent names. A node
  // in no group either sorts to the top, and its own value breaks the tie.
  const own = groupOf(node);
  return own ? placeOf(own) : 0;
}

/**
 * SANKEY.layOut at a stated canvas height, in px: what the layout test
 * measures a chart at heights the chart does not declare with.
 *
 * FAILS CLOSED on a node with a stamped slot (expansionOf) that d3 laid in
 * another column: d3 counts columns from the longest path and clamps a node
 * placed beyond it, which would draw it a column early in silence. A filter
 * that cuts every ribbon into an inserted slot can do that, and the page says
 * so in its refusal banner.
 * @param {FiscProjection} drawn
 * @param {{tiers: number[], columns: number, groupOf: (n: any) => string, placeOf: (id: string) => number}} ctx
 * @param {number} height
 * @returns {{nodes: LaidNode[], links: LaidLink[]}}
 */
export function layOutAt(drawn, ctx, height) {
  const d3 = /** @type {any} */ (globalThis).d3;
  const slots = slotsOf(drawn.nodes, ctx.tiers);
  const sankey = d3.sankey()
    .nodeId(/** @param {LaidNode} d */ (d) => d.id)
    .nodeWidth(NODE_WIDTH)
    .nodePadding(NODE_PADDING)
    .nodeAlign(alignFor(slots))
    // Supplying this switches d3's own ordering pass off.
    .nodeSort(/** @param {LaidNode} a @param {LaidNode} b */ (a, b) =>
      nodeRank(ctx.groupOf, ctx.placeOf, a) - nodeRank(ctx.groupOf, ctx.placeOf, b) || b.value - a.value)
    // The same column count render() sizes the viewBox from.
    .extent([[LABEL_GUTTER, 12],
      [chartWidth(ctx.columns) - LABEL_GUTTER, height - 12]]);
  const graph = sankey({
    nodes: drawn.nodes.map((n) => Object.assign({}, n)),
    links: drawn.links.map((l) => Object.assign({}, l, { value: l.value_cents })),
  });
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

  /** The declared tiers of an overview or a step, left to right. */
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

  /** The split caps a chart declares, one per split key that needs one. */
  splitCaps(chart) {
    return (chart.sankey && chart.sankey.split_caps) || [];
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
   * Draws a laid graph into ctx.layer, an empty group the page made inside
   * ctx.svg and moves as the reader zooms: ribbons, marks, bands and labels, with
   * the classes, descriptions, gestures and the figure each label says
   * (ctx.figure) the page hands it. Paints nothing: the page paints after, so
   * a theme change repaints without redrawing.
   */
  render(graph, ctx) {
    const d3 = /** @type {any} */ (globalThis).d3;
    const svg = ctx.svg;
    const width = chartWidth(ctx.columns);
    const height = canvasHeight(ctx.chart, graph.nodes);
    const on = ctx.on;

    // No width or height attributes: the viewBox makes the drawing scale.
    svg.attr("viewBox", "0 0 " + width + " " + height);

    const gLinks = ctx.layer.append("g").attr("class", "links");
    const gHits = ctx.layer.append("g").attr("class", "link-hits");
    const gNodes = ctx.layer.append("g").attr("class", "nodes");
    /** @param {LaidLink} d */
    const hairline = (d) => isHairline(ctx.chart, d);

    gLinks.selectAll("path")
      .data(graph.links)
      .join("path")
      .attr("class", /** @param {LaidLink} d */ (d) => ctx.classes.link(d) + (hairline(d) ? " hairline" : ""))
      .attr("d", d3.sankeyLinkHorizontal())
      // The surface gap, not a stroke, separates stacked ribbons.
      .attr("stroke-width", /** @param {LaidLink} d */ (d) => Math.max(1, d.width - RIBBON_GAP))
      .attr("tabindex", 0)
      .attr("role", "button")
      .attr("aria-label", /** @param {LaidLink} d */ (d) => ctx.describe.link(d))
      .on("pointerenter", /** @param {PointerEvent} e @param {LaidLink} d */ (e, d) => on.tip(e, d))
      .on("pointermove", /** @param {PointerEvent} e @param {LaidLink} d */ (e, d) => on.tip(e, d))
      .on("pointerleave", on.hide)
      .on("focus", /** @param {FocusEvent} e @param {LaidLink} d */ (e, d) => on.guarded("show this flow", () => { if (on.restoring()) return; on.tip(e, d); on.pin(d); }))
      .on("blur", on.hide)
      .on("click", /** @param {MouseEvent} e @param {LaidLink} d */ (e, d) => on.guarded("pin this flow", () => { e.stopPropagation(); on.pin(d); }));

    // A HAIRLINE'S HIT TARGET IS WIDER THAN ITS STROKE: the ribbon keeps its
    // focus and its description, and this transparent stroke above it takes
    // the pointer's hover and click.
    gHits.selectAll("path")
      .data(graph.links.filter(hairline))
      .join("path")
      .attr("class", "link-hit")
      .attr("d", d3.sankeyLinkHorizontal())
      .attr("aria-hidden", "true")
      .on("pointerenter", /** @param {PointerEvent} e @param {LaidLink} d */ (e, d) => on.tip(e, d))
      .on("pointermove", /** @param {PointerEvent} e @param {LaidLink} d */ (e, d) => on.tip(e, d))
      .on("pointerleave", on.hide)
      .on("click", /** @param {MouseEvent} e @param {LaidLink} d */ (e, d) => on.guarded("pin this flow", () => { e.stopPropagation(); on.pin(d); }));

    const node = gNodes.selectAll("g")
      .data(graph.nodes)
      .join("g")
      .attr("class", /** @param {LaidNode} d */ (d) => ctx.classes.node(d))
      .attr("tabindex", 0)
      .attr("role", "button")
      // aria-pressed is the isolation, on every node. SPLITTING IS NOT THE
      // TOGGLE and must never be announced as one: it replaces the node with
      // its parts, leaving no pressed state to return to. The label announces
      // what a node splits into.
      .attr("aria-pressed", "false")
      .attr("aria-label", /** @param {LaidNode} d */ (d) => ctx.describe.node(d))
      // Both keys activate every node; which one opens is the description's to say.
      .attr("aria-keyshortcuts", "Enter Space")
      .on("pointerenter", /** @param {PointerEvent} e @param {LaidNode} d */ (e, d) => on.tip(e, d))
      .on("pointermove", /** @param {PointerEvent} e @param {LaidNode} d */ (e, d) => on.tip(e, d))
      .on("pointerleave", on.hide)
      .on("focus", /** @param {FocusEvent} e @param {LaidNode} d */ (e, d) => on.guarded("show this mark", () => { if (on.restoring()) return; on.tip(e, d); on.pin(d); }))
      .on("blur", on.hide)
      // TWO GESTURES, ONE MEANING EACH: a single click and Space isolate on every
      // node; a double click and Enter split the nodes that split, and a double
      // click with Alt or Shift folds back the node a mark was split out of.
      //
      // Both click and keydown, because an SVG g[role=button] synthesises no click
      // from Enter, and some screen readers send both -- which would toggle twice.
      // So the guard is on the activation: a click on the node a key just
      // activated is that key's own click. Focus must not isolate, and a held key
      // is ignored, or tabbing would strobe the chart.
      .on("click", /** @param {MouseEvent} e @param {LaidNode} d */ (e, d) => {
        on.guarded("pin this mark", () => {
          e.stopPropagation();
          on.click(d, e.timeStamp);
        });
      })
      // ON EVERY NODE, not only one that splits: its two clicks have already
      // toggled the isolation twice, and this restores what was isolated before.
      // preventDefault stops the double click selecting the label.
      .on("dblclick", /** @param {MouseEvent} e @param {LaidNode} d */ (e, d) => {
        on.guarded("split this mark", () => {
          e.stopPropagation();
          e.preventDefault();
          on.dblclick(d, e.timeStamp, Boolean(e.altKey || e.shiftKey));
        });
      })
      .on("keydown", /** @param {KeyboardEvent} e @param {LaidNode} d */ (e, d) => {
        if (e.key !== "Enter" && e.key !== " ") return;
        if (e.repeat) return;
        e.preventDefault();
        on.guarded("act on this mark", () => on.key(d, e.key, e.timeStamp));
      });

    node.append("rect")
      .attr("x", /** @param {LaidNode} d */ (d) => d.x0)
      .attr("y", /** @param {LaidNode} d */ (d) => d.y0)
      .attr("width", /** @param {LaidNode} d */ (d) => d.x1 - d.x0)
      .attr("height", /** @param {LaidNode} d */ (d) => Math.max(2, d.y1 - d.y0))
      .attr("rx", 2);

    // On every mark, displayed on the ones that hang: the vendored d3 selection
    // has no filter(). pointer-events none, because the band is not the mark.
    node.append("rect")
      .attr("class", "contra-band")
      .attr("display", /** @param {LaidNode} d */ (d) => (contraBand(d) ? null : "none"))
      .attr("x", /** @param {LaidNode} d */ (d) => d.x0)
      .attr("y", /** @param {LaidNode} d */ (d) => (contraBand(d) || { y: 0 }).y)
      .attr("width", /** @param {LaidNode} d */ (d) => d.x1 - d.x0)
      .attr("height", /** @param {LaidNode} d */ (d) => (contraBand(d) || { height: 0 }).height)
      .attr("pointer-events", "none");

    // Every node is directly labelled: the relief the palette's contrast check
    // requires.
    const slots = slotsOf(graph.nodes, ctx.tiers);
    const place = (/** @type {LaidNode} */ d) => labelPlacement(slots, d, lastColumn);
    const lastColumn = Math.max(...graph.nodes.map((d) => columnOf(slots, d)));
    const qualified = labelQualifiers(ctx.doc, slots, graph.nodes);
    /** @param {LaidNode} d */
    const qualifierOf = (d) => qualified.get(d.id) || "";
    const label = node.append("text")
      .attr("class", "halo")
      .attr("y", /** @param {LaidNode} d */ (d) => place(d).y)
      .attr("dy", /** @param {LaidNode} d */ (d) => place(d).dy)
      .attr("x", /** @param {LaidNode} d */ (d) => place(d).x)
      .attr("text-anchor", /** @param {LaidNode} d */ (d) => place(d).anchor);

    // On an unqualified mark the qualifier tspan is empty with no x or dy, so it
    // starts no line.
    label.append("tspan")
      .attr("class", "qualifier")
      .attr("x", /** @param {LaidNode} d */ (d) => (qualifierOf(d) ? place(d).x : null))
      .attr("dy", /** @param {LaidNode} d */ (d) => (qualifierOf(d) ? labelLineShift(place(d).anchor).qualifier : null))
      .text(/** @param {LaidNode} d */ (d) => qualifierOf(d));
    label.append("tspan")
      .attr("x", /** @param {LaidNode} d */ (d) => (qualifierOf(d) ? place(d).x : null))
      .attr("dy", /** @param {LaidNode} d */ (d) => (qualifierOf(d) ? labelLineShift(place(d).anchor).label : null))
      .text(/** @param {LaidNode} d */ (d) => d.label);
    label.append("tspan")
      .attr("class", "value")
      .text(/** @param {LaidNode} d */ (d) => "  " + fmtShortSigned(ctx.figure(d)));
    label.append("tspan")
      .attr("class", "flag")
      .text(/** @param {LaidNode} d */ (d) => ctx.classes.flags(d));
  },

  /** Colours the drawn ribbons and marks; the colours are the page's. */
  paint(ctx) {
    ctx.svg.selectAll("path.link")
      .attr("stroke", /** @param {LaidLink} d */ (d) => ctx.colour.link(d));
    ctx.svg.selectAll("g.node rect")
      .attr("fill", /** @param {LaidNode} d */ (d) => ctx.colour.node(d));
  },

});
