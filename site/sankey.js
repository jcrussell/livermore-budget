// @ts-check
/**
 * fisc — the Sankey form: what depends on the screen and on ribbons. Which
 * tiers fit a budget, which ribbons a window holds and in what column order,
 * the reductions flipped so d3-sankey can draw them, where a residual or a
 * gap stands, and the layout helpers. Every figure it draws is core.js's: a
 * fold, a cap, a sum over cited ribbons. It reads of a step the generic
 * fields and `step.sankey`, and nothing of any other form's.
 *
 * STATELESS BY RULE, as core.js is: no module-level `let`, no global read at
 * import. d3 is read through globalThis when a layout asks for it; the
 * columns on screen and the rung stack are passed in by app.js, which holds
 * them. A type named here is one of app.js's typedefs.
 */

import {
  FOLD_REFUSES_MIXED, capColumn, citations, config, fmt, fmtShortSigned, foldDocument, foldTarget, gapID, gapMark, isAggregate, isFundGroup, isGap, isResidual, ledeOf, link, regroupLocators, residualID, residualMark, say, scoped, tailFigure, withinNode,
} from "./core.js";

export const NODE_WIDTH = 14;

export const NODE_PADDING = 14;

/** The surface gap that separates stacked ribbons, in px (1px each side). */
export const RIBBON_GAP = 2;

export const CHART_HEIGHT = 820;

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
 * The px `100vw` counts that the window does not: body padding plus a classic
 * scrollbar. style.css records it independently as --chart-cushion.
 */
export const CHART_CUSHION = 56;

/** DrillStep.Side for a step opening the node its chart's links come FROM. */
export const SIDE_SOURCE = "source";

/**
 * Whether a window step keeps its flank at the LEFT end of its columns: the
 * kept tier is drawn before the opened one. Declared by position, as
 * validateSteps holds it, so the two halves' sides are read off one list.
 * @param {FiscDrillStep} step
 * @returns {boolean}
 */
export function flankIsLeft(step) {
  return step.sankey.tiers.indexOf(step.sankey.keep[0]) < step.sankey.tiers.indexOf(step.from);
}

/**
 * The tiers a step opens into, in its column order: every declared tier but
 * the kept flank, widened ones included. Widen fits a viewport and does not
 * change what the document draws.
 * @param {FiscDrillStep} step
 * @returns {number[]}
 */
export function freshTiers(step) {
  const keep = new Set(step.sankey.keep || []);
  return step.sankey.tiers.filter((t) => !keep.has(t));
}

/**
 * What a step's document draws for one node at the tiers the step declares,
 * unfolded: the reach the chart is drawn with (reaching), asked at every
 * declared column rather than at the columns a budget draws. The answer to
 * whether a node opens (decomposable) and to whether its residual's leaving
 * legs exist (carryResidual), which must not move with the viewport.
 * @param {FiscDrillStep} step
 * @param {FiscProjection} doc
 * @param {string} id
 * @returns {FiscProjection}
 */
export function freshHalf(step, doc, id) {
  const window = Boolean(step.sankey.keep && step.sankey.keep.length);
  const tiers = freshTiers(step);
  const nearIsSource = window ? flankIsLeft(step) : step.sankey.side === SIDE_SOURCE;
  return filterLinks(doc, id, tiers, reaching(doc, id, nearIsSource, tiers));
}

/**
 * The filter every rung uses: a ribbon of `doc` is drawn when its near end --
 * the target, or the source where the opened node is the end links come FROM
 * -- is the opened node or one of its parts by parent chain, AND its folded
 * ends run forward in the drawn column order. Membership alone admits cycles:
 * a fund window's flank parents a department's rows under the fund, so
 * department -> row folds to department -> fund, which d3-sankey refuses as a
 * circular link. The rule is the hierarchy's, not the ribbons': a walk along
 * ribbons would miss transfers/in, which no ribbon touches and whose payers
 * are its children.
 *
 * An end that folds to nothing is passed through; placing it is scoped()'s
 * question.
 *
 * @param {FiscProjection} doc
 * @param {string} opened
 * @param {boolean} nearIsSource
 * @param {number[]} tiers  the columns drawn, in order
 * @returns {(src: FiscNode, dst: FiscNode, byID: Map<string,FiscNode>, drawn: Set<number>) => boolean}
 */
export function reaching(doc, opened, nearIsSource, tiers) {
  const inside = withinNode(doc, opened);
  const at = new Map(tiers.map((t, i) => [t, i]));
  return (src, dst, byID, drawn) => {
    if (!inside.has(nearIsSource ? src.id : dst.id)) return false;
    const from = foldTarget(byID, src, drawn);
    const to = foldTarget(byID, dst, drawn);
    if (from === "" || to === "") return true;
    const a = at.get((byID.get(from) || src).tier);
    const b = at.get((byID.get(to) || dst).tier);
    return a !== undefined && b !== undefined && a < b;
  };
}

/**
 * The links `holds` admits whose ends this tier set can place, and the nodes
 * those links need. Refuses nothing: an id nothing flows for is an empty
 * answer, which decomposable reads as "does not open" and windowFor refuses.
 *
 * @param {FiscProjection} doc
 * @param {string} id
 * @param {number[]} tiers
 * @param {(src: FiscNode, dst: FiscNode, byID: Map<string,FiscNode>, drawn: Set<number>) => boolean} holds
 * @returns {FiscProjection}
 */
export function filterLinks(doc, id, tiers, holds) {
  const byID = new Map(doc.nodes.map((n) => [n.id, n]));
  const drawn = new Set(tiers);
  const placeable = scoped(doc, tiers);
  const links = doc.links.filter((l) => {
    const src = byID.get(l.source);
    const dst = byID.get(l.target);
    if (!src || !dst) return false;
    if (!holds(src, dst, byID, drawn)) return false;
    return placeable(src) && placeable(dst);
  });

  // Only the nodes those links touch, up to the drawn tiers: any other node
  // would make foldDocument refuse the document.
  const keep = new Set();
  for (const l of links) {
    for (const end of [l.source, l.target]) {
      let at = byID.get(end);
      for (let hops = 0; at && hops < 9; hops++) {
        keep.add(at.id);
        if (drawn.has(at.tier)) break;
        at = at.parent ? byID.get(at.parent) : undefined;
      }
    }
  }
  return Object.assign({}, doc, {
    nodes: doc.nodes.filter((n) => keep.has(n.id)),
    links: links,
  });
}

/**
 * One filtered, capped and folded chart of a node: a whole rung on a step that
 * keeps no flank, and one half of a window on a step that does.
 *
 * THE ORDER IS filter, cap, fold, AND IT IS NOT INTERCHANGEABLE: the cap must
 * rank sizes inside the opened node, and the fold is what merges the cap's
 * parallel ribbons and unions their citations.
 *
 * @param {FiscProjection} doc
 * @param {Rung} rung
 * @param {number[]} tiers the columns this chart draws, in order
 * @param {boolean} nearIsSource whether the opened node is the end the drawn
 *   ribbons come FROM
 * @returns {FiscProjection}
 */
export function sideOf(doc, rung, tiers, nearIsSource) {
  const step = rung.step;
  let shaped = filterLinks(doc, rung.id, tiers, reaching(doc, rung.id, nearIsSource, tiers));
  const inside = withinNode(doc, rung.id);
  // An aggregate the chart above drew and this side reaches is finished
  // already: its parent and note are left as they came.
  const had = new Set(doc.nodes.filter((n) => isAggregate(n.id)).map((n) => n.id));
  // CAPS RUN IN THE TIER ORDER THIS CHART DRAWS: folding a coarse node
  // removes descendants a finer cap would otherwise rank.
  //
  // THE TAIL'S PARENT IS THE OPENED NODE ONLY WHEN THE WHOLE COLUMN IS INSIDE
  // IT, asked of the document; otherwise "" (--muted).
  /** @type {Map<number, string>} */
  const parentOf = new Map();
  for (const tier of tiers) {
    const cap = (step.sankey.caps || []).find((c) => c.tier === tier);
    // An expanded tier skips the cap and only the cap.
    if (!cap || (rung.expanded && rung.expanded.has(tier))) continue;
    const column = shaped.nodes.filter((n) => n.tier === tier);
    const parent = column.every((n) => inside.has(n.id)) ? rung.id : "";
    parentOf.set(tier, parent);
    shaped = capColumn(shaped, tier, cap.cap, parent, cap.tail || step.tail);
  }
  const drawn = foldDocument(shaped, tiers);

  // EVERY AGGREGATE THIS CALL MADE HAS ITS PARENT PUT BACK AFTER THE FOLD,
  // which blanks it because the opened node was filtered away. Its note is
  // finished here with the figure only the folded document knows.
  return Object.assign({}, drawn, {
    nodes: drawn.nodes.map((n) => (isAggregate(n.id) && !had.has(n.id)
      ? Object.assign({}, n, {
        parent: parentOf.get(n.tier) || "",
        source_note: n.source_note + say("aggregate_together", { figure: fmt(tailFigure(drawn, n.id)) }),
      })
      : n)),
  });
}

/**
 * The tiers a window keeps from the chart on screen: its flank and its centre.
 * Never widened, so every viewport keeps the same ones.
 * @param {FiscDrillStep} step
 * @returns {number[]}
 */
export function keptTiersOf(step) {
  const keep = new Set(step.sankey.keep || []);
  return step.sankey.tiers.filter((t) => keep.has(t) || t === step.from);
}

/**
 * A window's kept half: the chart on screen, read at its printed signs, at the
 * kept tiers, reaching the opened node. The one reading windowFor draws and
 * stepDecomposes offers by, so what is offered is what draws.
 * @param {FiscProjection} onScreen
 * @param {{id: string, step: FiscDrillStep, expanded?: Set<number>}} rung
 * @returns {FiscProjection}
 */
export function keptFlank(onScreen, rung) {
  return sideOf(unmarkContra(onScreen), /** @type {any} */ (rung), keptTiersOf(rung.step), !flankIsLeft(rung.step));
}

/**
 * A window on the node the reader clicked: the flank they came from on one
 * side, the step document's decomposition on the other, and that node between
 * them.
 *
 * TWO DOCUMENTS, SO TWO HALVES, spliced on the centre: folding them together
 * would hand foldDocument two parent chains. Which side is which is read off
 * the step: the kept tiers are the flank, the opened tier the centre, and the
 * rest is what the node opens into.
 *
 * THE KEPT FLANK COMES OFF THE CHART ON SCREEN, NOT OFF A FILE: its nodes need
 * not exist in the step's document at all. The centre's record is the
 * on-screen chart's too, and is not marked carried.
 *
 * @param {FiscProjection} onScreen the drawn chart the rung was opened from
 * @param {FiscProjection} stepDoc the document the step draws
 * @param {Rung} rung
 * @param {number[]} tiers the columns on screen, not the columns declared
 * @returns {FiscProjection}
 */
export function windowFor(onScreen, stepDoc, rung, tiers) {
  const step = rung.step;
  const keep = new Set(step.sankey.keep);
  // THE CENTRE IS THE ONLY COLUMN BOTH HALVES HOLD, so a ribbon of one cannot
  // land in a column of the other. With the flank on the left the opened node
  // is the TARGET of the kept half and the SOURCE of the fresh one; on the
  // right, the reverse.
  const keptLeft = flankIsLeft(step);
  const freshOnScreen = tiers.filter((t) => !keep.has(t));
  const kept = keptFlank(onScreen, rung);
  // A RESIDUAL OR A GAP ON THE FLANK IS REFUSED: it is a mark this page drew
  // and no schedule prints, so the fresh half never computed its figure and
  // the window would carry it as though the new document had.
  const mark = kept.nodes.find((n) => isResidual(n.id) || isGap(n.id));
  if (mark) {
    throw new Error("cannot draw " + stepDoc.projection + ": the kept flank carries " + mark.id +
      ", a mark this page drew and no schedule prints, so the window would show a figure " +
      "the document it draws never computed");
  }
  const fresh = sideOf(stepDoc, rung, freshOnScreen, keptLeft);
  // THE KEPT CENTRE MUST HOLD THE OPENED NODE: a window whose flank sends
  // nothing into it is refused rather than drawn as its fresh half alone.
  if (!kept.nodes.some((n) => n.id === rung.id)) {
    throw new Error("cannot draw " + stepDoc.projection + ": the chart on screen sends nothing " +
      "between tiers " + keptTiersOf(step).join(", ") + " and " + rung.id + ", so there is no flank to keep");
  }

  // carried_from IS SET WHERE ABSENT AND NEVER CLEARED: a node keeps the stem
  // whose figure and caveats it carries, however many rungs down.
  const stem = onScreen.projection || "";
  /** @type {Set<string>} */
  const have = new Set();
  /** @type {FiscNode[]} */
  const nodes = [];
  for (const n of kept.nodes) {
    have.add(n.id);
    nodes.push(n.id === rung.id || n.carried_from
      ? n
      : Object.assign({}, n, { carried_from: stem }));
  }
  for (const n of fresh.nodes) {
    if (have.has(n.id)) continue;
    have.add(n.id);
    nodes.push(n);
  }
  // The spliced document is the step document's; the kept flank is a guest.
  return Object.assign({}, fresh, {
    nodes: nodes,
    links: kept.links.concat(fresh.links),
  });
}

/**
 * flankHolds' answers per chart on screen, per step and node: nodeClass asks
 * per mark per paint. Keyed on the frozen document, so nothing here is page
 * state.
 * @type {WeakMap<FiscProjection, Map<string, boolean>>}
 */
const flanks = new WeakMap();

/**
 * Whether a window step's kept flank off the chart on screen holds the node,
 * and carries no residual or gap, which windowFor refuses: the offer and the
 * draw are one rule. A flank the fold refuses is offered rather than thrown
 * here: this is asked per mark per paint, and the drill that opens it says
 * why it cannot draw.
 * @param {FiscProjection} onScreen
 * @param {FiscDrillStep} step
 * @param {string} id
 * @returns {boolean}
 */
export function flankHolds(onScreen, step, id) {
  let byStep = flanks.get(onScreen);
  if (!byStep) {
    byStep = new Map();
    flanks.set(onScreen, byStep);
  }
  const at = step.key + "\u001f" + id;
  let holds = byStep.get(at);
  if (holds === undefined) {
    try {
      const kept = keptFlank(onScreen, { id: id, step: step }).nodes;
      holds = kept.some((n) => n.id === id) && !kept.some((n) => isResidual(n.id) || isGap(n.id));
    } catch (e) {
      // Only the fold's refusal; anything else is a defect and is thrown.
      if (!(e instanceof Error && e.message.includes(FOLD_REFUSES_MIXED))) throw e;
      holds = true;
    }
    byStep.set(at, holds);
  }
  return holds;
}

/**
 * The ids at a step's opened tier that its document decomposes, computed once
 * per (document, step) because nodeClass asks per mark per paint. The fresh
 * half at every declared tier, widened ones included: the step promised them.
 * @type {WeakMap<FiscProjection, Map<string, Set<string>>>}
 */
const decomposed = new WeakMap();

/**
 * @param {FiscDrillStep} step
 * @param {FiscProjection} doc
 * @returns {Set<string>}
 */
export function decomposable(step, doc) {
  let byStep = decomposed.get(doc);
  if (!byStep) {
    byStep = new Map();
    decomposed.set(doc, byStep);
  }
  const have = byStep.get(step.key);
  if (have) return have;
  const out = new Set();
  for (const n of doc.nodes) {
    if (n.tier !== step.from || (step.role && n.role !== step.role)) continue;
    if (freshHalf(step, doc, n.id).links.length) out.add(n.id);
  }
  byStep.set(step.key, out);
  return out;
}

/**
 * Whether a residual's leaving leg is drawn at these columns: a leaving
 * endpoint stands at the step's last declared tier (carryResidual).
 *
 * @param {FiscDrillStep} step
 * @param {number[]} tiers the columns the chart draws
 * @returns {boolean}
 */
export function leavingLegDrawn(step, tiers) {
  return tiers.includes(step.sankey.tiers[step.sankey.tiers.length - 1]);
}

/**
 * Drops a widened column the drawn document left empty, so the chart is laid
 * out at the columns it has.
 *
 * d3-sankey takes its column count from topology, so an empty declared column
 * would stretch the others rather than narrow the chart. DROPPED AND NOT
 * REFUSED, and only a widened column: the step promised the others.
 *
 * @param {FiscProjection} drawn
 * @param {Rung | null} rung the innermost rung, or null on the overview
 * @param {number[]} tiers the columns on screen
 * @returns {boolean} whether anything was dropped
 */
export function dropEmptyColumns(drawn, rung, tiers) {
  const widen = rung && rung.step.sankey.widen ? rung.step.sankey.widen : [];
  if (!widen.length) return false;
  const has = new Set(drawn.nodes.map((n) => n.tier));
  const gone = tiers.filter((t) => widen.indexOf(t) >= 0 && !has.has(t));
  if (!gone.length) return false;
  rung.dropped = (rung.dropped || []).concat(gone);
  return true;
}

/**
 * Draws the opened node at the figure it prints where its drawn ribbons do
 * not add up to it: a node a schedule prints a reduction under, the reduction
 * drawn forward at its magnitude by markContra. The figure is the signed sum
 * of the ribbons arriving at it from the columns the step opens into --
 * the same one the citywide chart labels the node with -- and is read before
 * any mark is added or any sign is flipped.
 *
 * d3-sankey would otherwise size the node at the gross its forward-drawn
 * ribbons add to, a figure no page prints. Setting the value here means every
 * surface reads one figure. A sum at or below zero would rescale its whole
 * column silently, so it is refused.
 *
 * @param {FiscProjection} drawn  shaped and folded, before any mark
 * @param {Rung} rung
 * @returns {FiscProjection} drawn itself when no arriving ribbon is a reduction
 */
export function markAmounts(drawn, rung) {
  const keep = new Set(rung.step.sankey.keep || []);
  const tierOf = new Map(drawn.nodes.map((n) => [n.id, n.tier]));
  let sum = 0;
  let contra = false;
  for (const l of drawn.links) {
    if (l.target !== rung.id || keep.has(tierOf.get(l.source))) continue;
    sum += l.value_cents;
    if (l.value_cents < 0) contra = true;
  }
  if (!contra) return drawn;
  if (sum <= 0) {
    throw new Error("cannot draw " + drawn.projection + ": the ribbons into " + rung.id +
      " net to " + sum + " cents, which is no height to draw it at");
  }
  return Object.assign({}, drawn, {
    nodes: drawn.nodes.map((n) => (n.id === rung.id ? Object.assign({}, n, { fixedValue: sum }) : n)),
  });
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
 * its own sign. A kept flank is read off the chart on screen, which is marked,
 * and its reductions are summed and folded at the sign they were printed at.
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
 * @param {LaidLink | LaidNode} d
 * @returns {boolean}
 */
export function isLink(d) {
  return Object.prototype.hasOwnProperty.call(d, "fact_ids");
}

/**
 * The figure a laid mark prints: negative for a contra ribbon and for a line
 * whose every ribbon is one, the larger of a residual's two sides, and d3's
 * value otherwise. A residual's is carryResidual's sum even where the width
 * holds legs back.
 * @param {LaidLink | LaidNode} d
 * @returns {number}
 */
export function markCents(d) {
  if (isLink(d)) {
    const l = /** @type {LaidLink} */ (d);
    return l.contra ? -l.value_cents : l.value_cents;
  }
  const n = /** @type {LaidNode} */ (d);
  if (isResidual(n.id)) return Math.max(n.in_cents || 0, n.out_cents || 0);
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
 * A residual's two figures, where money both enters and leaves it; "" for
 * every other mark.
 * @param {LaidNode} d
 * @returns {string}
 */
export function residualFlows(d) {
  if (!isResidual(d.id) || !d.in_cents || !d.out_cents) return "";
  return say("residual_flows", { in: fmt(d.in_cents), out: fmt(d.out_cents) });
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
  const arriving = d.targetLinks.reduce((sum, l) => sum + l.value, 0);
  const leaving = d.sourceLinks.reduce((sum, l) => sum + l.value, 0);
  const side = arriving >= leaving ? d.targetLinks : d.sourceLinks;
  const reduced = side.filter((l) => l.contra).reduce((sum, l) => sum + l.value_cents, 0);
  if (!reduced) return "";
  return "\u25c7 our reading: " + fmt(reduced) + " of this category is printed as reductions, " +
    "drawn here at their size";
}

/**
 * Adds to a rung's drawn document the flows the chart it was opened from
 * prints for the opened node and the document it draws does not decompose,
 * copied verbatim onto one derived node beside the node's parts.
 *
 * CARRIED, NOT COMPUTED: every link added is a link of the chart above with
 * only its group end re-pointed, so its figure and provenance are untouched.
 * The endpoints are the step's declaration, whole or nothing each, in sorted
 * id order: an endpoint's inflow is carried only where the drawn document
 * carries nothing from it into the opened node, its outflow only where the
 * step's document decomposes the node at the tiers the step declares and
 * carries nothing from inside to it; where both, the outflow's placement
 * wins. An endpoint's ribbons come off
 * the chart on screen where it draws any, else off the file, never both: the
 * window's flank is already on screen, and taking both would count
 * transfers/in twice against what p0067 prints. The mark's two figures are the sums of the
 * ribbons carried each way, the leaving ones counted whether or not this width
 * draws their column, so the figures do not move with the viewport. Its
 * inflow and outflow differ by construction, and d3-sankey shows that on the
 * mark.
 *
 * Only across a document switch: a step that draws the document before it has
 * no second grain to be residual by.
 *
 * @param {FiscProjection} drawn  the rung's document, shaped and folded
 * @param {FiscProjection | null} from  the document of the chart the rung was
 *   opened from
 * @param {Rung} rung
 * @param {number[]} onScreen the columns on screen
 * @returns {FiscProjection}
 */
export function carryResidual(drawn, from, rung, onScreen) {
  const step = rung.step;
  const residual = step.residual;
  if (!residual || !from || !step.projection) return drawn;
  const opened = rung.id;
  const doc = rung.doc;
  const id = residualID(opened);
  const inside = withinNode(doc, opened);
  if (!inside.has(opened)) {
    throw new Error("cannot draw " + doc.projection + ": it does not carry " + opened +
      ", so nothing can be residual beside its parts");
  }
  // DECOMPOSED IS THE STEP'S DECLARATION, NOT THIS WIDTH: whether the document
  // sends anything out of a part of the opened node at a tier the step
  // declares, each part read at the column it folds to there. Asked of the
  // drawn window instead, a budget that drops the parts' outward column would
  // drop the leaving legs from the mark's figure as well as from the chart.
  const declared = freshHalf(step, doc, opened);
  const byDocID = new Map(doc.nodes.map((n) => [n.id, n]));
  const declaredTiers = new Set(freshTiers(step));
  const decomposed = declared.links.some((l) => {
    const part = foldTarget(byDocID, /** @type {FiscNode} */ (byDocID.get(l.source)), declaredTiers);
    return part !== "" && part !== opened && inside.has(part);
  });
  const carriesFrom = (/** @type {string} */ e) => doc.links.some((l) => l.source === e && inside.has(l.target));
  const carriesTo = (/** @type {string} */ e) => doc.links.some((l) => l.target === e && inside.has(l.source));
  /** @type {FiscLink[]} */
  const links = [];
  /**
   * The chart's own copy of each link re-pointed below, dropped: a window's
   * flank already draws them, and keeping both would double-count.
   * @type {Set<FiscLink>}
   */
  const spliced = new Set();
  /**
   * One endpoint's links off the chart above: this rung's own where it draws
   * them, else the file's. Never both.
   * @param {(l: FiscLink) => boolean} want
   * @returns {FiscLink[]}
   */
  const above = (want) => {
    const here = drawn.links.filter(want);
    return here.length ? here : from.links.filter(want);
  };
  /** @type {Map<string, boolean>} endpoint id to whether its flow arrives */
  const ends = new Map();
  // A LEAVING LEG IS DROPPED WHERE THE BUDGET DROPS ITS COLUMN, as every other
  // ribbon of that column is; placed in the last column the chart has, it
  // would draw a ribbon of no length.
  const leaves = leavingLegDrawn(step, onScreen);
  // Counted, not inferred from the endpoints: a mark with no leaving flow
  // holds nothing back and its note must not say otherwise.
  let withheld = 0;
  let inCents = 0;
  let outCents = 0;
  // The endpoints whose flow the mark carries, drawn or withheld: the ones
  // its rationale gives a reason for.
  /** @type {Set<string>} */
  const touched = new Set();
  for (const e of Object.keys(residual).sort()) {
    if (!carriesFrom(e)) {
      for (const l of above((l) => l.source === e && l.target === opened)) {
        links.push(Object.assign({}, l, { target: id }));
        inCents += l.value_cents;
        ends.set(e, true);
        touched.add(e);
        spliced.add(l);
      }
    }
    if (!decomposed || carriesTo(e)) continue;
    const leaving = above((l) => l.source === opened && l.target === e);
    if (leaving.length) touched.add(e);
    for (const l of leaving) outCents += l.value_cents;
    if (!leaves) {
      withheld += leaving.length;
      continue;
    }
    for (const l of leaving) {
      links.push(Object.assign({}, l, { source: id }));
      ends.set(e, false);
      spliced.add(l);
    }
  }

  // A MARK NONE OF WHOSE FLOWS THIS WIDTH DRAWS IS NOT DRAWN: every flow of an
  // enterprise or special revenue residual leaves through the widened column,
  // and at three columns it would be a box of no height citing nothing.
  if (!links.length) return drawn;

  // THE MARK STANDS AT THE SHALLOWEST DECLARED TIER OF ANY PART OF THE OPENED
  // NODE, read off the step's unfolded document; a node with no part at a
  // declared tier has nowhere to stand it.
  let tier = -1;
  for (const n of doc.nodes) {
    if (n.id !== opened && inside.has(n.id) && step.sankey.tiers.includes(n.tier) && (tier < 0 || n.tier < tier)) {
      tier = n.tier;
    }
  }
  if (tier < 0) {
    throw new Error("cannot draw " + doc.projection + ": " + opened +
      " has no part at a tier this step draws to stand the residual beside");
  }

  // ENDPOINTS STAND AT THE FIRST DRAWN TIER WHEN THEIR FLOW ARRIVES AND THE
  // LAST WHEN IT LEAVES -- drawn, not declared: an undrawn declared tier is
  // clamped to the first column and the ribbon runs backwards. Filtered in the
  // step's own order, which is a column order and not a sorted set.
  const tiers = step.sankey.tiers.filter((t) => drawn.nodes.some((n) => n.tier === t));
  const have = new Set(drawn.nodes.map((n) => n.id));
  const fromByID = new Map(from.nodes.map((n) => [n.id, n]));
  /** @type {FiscNode[]} */
  const added = [];
  for (const [e, arrives] of ends) {
    const node = fromByID.get(e);
    if (!node || have.has(e)) continue;
    added.push(Object.assign({}, node, {
      tier: arrives ? tiers[0] : tiers[tiers.length - 1], parent: "",
      // Its caveats belong to the chart it was carried from; caveatsFor,
      // caveatHref and carriedSource resolve this stem against the stack.
      carried_from: from.projection || "",
    }));
  }

  /** @type {Map<string, Set<number>>} */
  const cited = new Map();
  for (const l of links) {
    for (const s of l.locators || []) {
      const pages = cited.get(s.doc_id) || new Set();
      for (const p of s.pages) pages.add(p);
      cited.set(s.doc_id, pages);
    }
  }
  const where = Array.from(cited.keys()).sort().map((docID) => {
    const d = config() && config().docs ? config().docs[docID] : undefined;
    const pages = Array.from(cited.get(docID) || []).sort((a, b) => a - b);
    return (d && d.title ? d.title : docID) + " " +
      (pages.length === 1 ? "p." : "pp.") + pages.join(", ");
  }).join("; ");

  // The words: the label names the grain, the rationale carries the step's
  // reason for every endpoint in sorted order, whichever legs this width draws.
  const label = (/** @type {string} */ n) => {
    const own = doc.nodes.find((g) => g.id === n && g.label);
    if (own) return own.label;
    const theirs = from.nodes.find((g) => g.id === n && g.label);
    return theirs ? theirs.label : n;
  };
  const grain = step.residual_grain || "";
  const reasons = Object.keys(residual).sort().filter((e) => touched.has(e))
    .map((e) => label(e) + ": " + residual[e] + ".");

  // No plural is formed from the grain. The note cites the ribbons carried
  // at this width.
  const node = residualMark(opened, tier, grain, inCents, outCents,
    say("residual_rationale", { opened: label(opened), grain: grain, reasons: reasons.join(" ") }),
    say("residual_note", {
      flows: links.length, where: where,
      withheld: !withheld ? ""
        : " " + (withheld === 1 ? say("residual_withheld_one") : say("residual_withheld_many", { n: withheld })),
    }));
  return Object.assign({}, drawn, {
    nodes: drawn.nodes.concat(added, [node]),
    links: drawn.links.filter((l) => !spliced.has(l)).concat(links),
  });
}

/**
 * States as a mark of its own the difference between what the chart above
 * sends into the opened node and what the document this rung draws breaks that
 * node into, where the step licenses exactly that difference in this column.
 *
 * Without it d3-sankey absorbs the difference into node height with no ribbon
 * against it, and the chart looks balanced. A gap has no published link to
 * copy, so unlike carryResidual's mark this one is derived. The sums are
 * signed, reductions negative, as they stand before markContra; after that
 * pass a centre would read short by twice the reductions. Too little leaving
 * stands the mark at the last declared tier, too little arriving at the
 * first. It carries no kind: the difference crosses no printed boundary.
 *
 * FAILS CLOSED: a difference no licence accounts for, one licensed in another
 * column or at another figure, and a licence for a centre that balances are
 * each a throw, so the two documents drifting apart is a banner and not a
 * chart. The licence's cents is structure's, shipped on the step.
 *
 * @param {FiscProjection} drawn  the rung's chart, shaped, folded and spliced
 * @param {FiscProjection | null} from  the document of the chart the rung was
 *   opened from, which the mark cites alongside the drawn one
 * @param {Rung} rung
 * @returns {FiscProjection} drawn itself where the step declares no gap at all,
 *   or the node it opened balances unlicensed
 */
export function markGap(drawn, from, rung) {
  const step = rung.step;
  const gaps = step.gaps;
  if (!gaps || typeof gaps !== "object") return drawn;
  const opened = rung.id;
  if (!drawn.nodes.some((n) => n.id === opened)) {
    throw new Error("cannot draw " + drawn.projection + ": " + opened + " is not a mark of the " +
      "drawn chart, so the gap this step declares has nothing to be stated against");
  }
  let into = 0;
  let outOf = 0;
  for (const l of drawn.links) {
    if (l.target === opened) into += l.value_cents;
    if (l.source === opened) outOf += l.value_cents;
  }
  const gap = into - outOf;
  const meta = drawn.metadata || /** @type {any} */ ({});
  const licence = (gaps[opened] || [])
    .find((g) => g.fiscal_year === meta.fiscal_year && g.basis === meta.basis);
  if (gap === 0 && !licence) return drawn;
  // Named only once there is something to say about the column.
  const where = ledeOf(meta);
  if (gap === 0) {
    throw new Error("cannot draw " + drawn.projection + ": the step declares a gap of " + licence.cents +
      " cents on " + opened + " in " + where + " and the chart balances there");
  }
  if (!licence) {
    throw new Error("cannot draw " + drawn.projection + ": the chart above sends " + into + " into " +
      opened + " and this one draws " + outOf + " of it, a difference of " + Math.abs(gap) +
      " cents that no declaration on this step accounts for in " + where +
      "; the two documents have drifted apart");
  }
  if (licence.cents !== gap) {
    throw new Error("cannot draw " + drawn.projection + ": the step declares a gap of " + licence.cents +
      " cents on " + opened + " in " + where + " and the charts differ there by " + gap);
  }
  const tiers = step.sankey.tiers;
  const locators = citedAround(opened, from ? [from, rung.doc] : [rung.doc]);
  if (!locators.length) {
    throw new Error("cannot draw " + drawn.projection + ": no ribbon touching " + opened +
      " cites a page, so the gap on it could cite none");
  }
  const centreNode = drawn.nodes.find((n) => n.id === opened && n.label);
  const centre = centreNode ? centreNode.label : opened;
  const column = where;
  const lead = gap > 0
    ? say("gap_lead_short", { column: column, into: fmt(into), centre: centre, out: fmt(outOf), gap: fmt(gap) })
    : say("gap_lead_over", { column: column, out: fmt(outOf), centre: centre, gap: fmt(-gap), into: fmt(into) });
  const id = gapID(opened);
  // THE SIDE THE FIGURE IS ON IS THE SIDE THE MARK STANDS ON: too little
  // leaving arrives at the mark, too little arriving leaves it.
  const node = gapMark(opened, gap > 0 ? tiers[tiers.length - 1] : tiers[0], gap,
    say("gap_rationale", { lead: lead, reason: licence.reason, gap: fmt(Math.abs(gap)) }),
    say("gap_note"),
    locators);
  const link = gap > 0
    ? { source: opened, target: id, value_cents: gap }
    : { source: id, target: opened, value_cents: -gap };
  return Object.assign({}, drawn, {
    nodes: drawn.nodes.concat([node]),
    links: drawn.links.concat([Object.assign({
      kind: "", transfer_id: "", fact_ids: [], locators: locators, derived: true,
    }, link)]),
  });
}

/**
 * Every page cited by a ribbon of any of `docs` with an end at `opened` or
 * inside it, merged per document, documents and pages sorted: the pages of
 * the two totals a gap subtracts.
 * @param {string} opened
 * @param {FiscProjection[]} docs
 * @returns {FiscSource[]}
 */
export function citedAround(opened, docs) {
  const keys = new Set();
  for (const doc of docs) {
    const inside = withinNode(doc, opened);
    for (const l of doc.links) {
      if (!inside.has(l.source) && !inside.has(l.target)) continue;
      for (const s of l.locators || []) {
        for (const p of s.pages) keys.add(s.doc_id + "\u001f" + p);
      }
    }
  }
  return regroupLocators(keys);
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
 * Which column d3-sankey puts a node in: the position of its tier in the
 * declared order, or d3's own justify when nothing was declared.
 *
 * Justify puts a link-less sink in the last column, wrong once tiers can be
 * skipped. The order may be non-monotonic, so it is indexOf, not a sort. The
 * tier set is the caller's: aligning a drilled document on the page's set
 * gives indexOf -1, and d3-sankey dies in its ordering pass.
 *
 * @param {number[]} tiers
 * @returns {(d: LaidNode) => number}
 */
export function alignFor(tiers) {
  return tiers.length
    ? /** @param {LaidNode} d */ (d) => tiers.indexOf(d.tier)
    : /** @type {any} */ (globalThis).d3.sankeyJustify;
}

/**
 * The column a node was drawn in, as an index into the declared order.
 *
 * NOT d.depth, the longest path to the node, which differs wherever no ribbon
 * reaches a node from the column before it. Not d.layer either, except in a
 * view that declares no column order, where sankeyJustify chose the columns.
 *
 * @param {LaidNode} d
 * @param {number[]} tiers the columns on screen
 * @returns {number}
 */
export function columnOf(tiers, d) {
  // d3 clamps an undeclared tier into column 0, so it is labelled as one.
  return tiers.length ? Math.max(0, tiers.indexOf(d.tier)) : d.layer;
}

/**
 * Where a node's label goes: the side it is anchored on, and the point.
 *
 * A LABEL MAY RUN OUTWARD ONLY INTO A GUTTER, and only the first and last
 * columns have one. An interior label is centred above its rect, in the
 * NODE_PADDING gap, rather than across the ribbons the next column receives.
 *
 * @param {LaidNode} d
 * @param {number} last the largest column index this chart drew
 * @param {number[]} tiers the columns on screen
 * @returns {{anchor: string, x: number, y: number, dy: string|null}}
 */
export function labelPlacement(tiers, d, last) {
  const col = columnOf(tiers, d);
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
 * rather than in the document. The parent is looked up in the document, not the
 * laid graph, because a window routinely does not draw it. A duplicate whose
 * parent is unnameable gets "" and stays ambiguous rather than invented.
 *
 * @param {LaidNode[]} nodes
 * @param {FiscProjection | null} doc the document the nodes were laid from
 * @param {number[]} tiers the columns on screen
 * @returns {Map<string, string>}
 */
export function labelQualifiers(doc, tiers, nodes) {
  /** @type {Map<string, LaidNode[]>} */
  const sharing = new Map();
  for (const n of nodes) {
    const key = columnOf(tiers, n) + "\u0000" + n.label;
    const seen = sharing.get(key);
    if (seen) seen.push(n);
    else sharing.set(key, [n]);
  }
  /** @type {Map<string, string>} */
  const out = new Map();
  for (const shared of sharing.values()) {
    if (shared.length < 2) continue;
    for (const n of shared) {
      const parent = doc
        ? doc.nodes.find((p) => p.id === n.parent)
        : null;
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
 * values, a residual's included.
 *
 * @param {LaidNode} d
 * @param {LaidNode[]} laid every laid node of the chart
 * @returns {string}
 */
export function columnShare(laid, d) {
  if (!d.value) return "";
  let total = 0;
  let siblings = 0;
  for (const other of laid) {
    if (other.layer === d.layer) {
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
 * the value-weighted mean place of the fund groups it touches (a barycentre).
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
    // The neighbour's fund group, not the neighbour: on the drill-down the
    // ends are funds and departments.
    const group = groupOf(other);
    // A node in no fund group is ignored rather than counted as position zero.
    if (!group) continue;
    const at = placeOf(group);
    place += at * l.value;
    weight += l.value;
  }
  // A node whose links were all dropped as zero-valued has no position to
  // average. It sorts to the top and its own value breaks the tie.
  return weight === 0 ? 0 : place / weight;
}

/**
 * The Sankey form's renderer, as app.js's FORMS holds it under "sankey".
 * Every method takes what it needs: the chart's declaration, the rung, the
 * columns on screen, the document on screen. It reads of a step the generic
 * fields and `step.sankey`, and nothing of any other form's hints; the test
 * in site/drill.test.mjs wraps the hints in a recording Proxy and holds every
 * read to this file.
 * @type {FormRenderer}
 */
export const SANKEY = Object.freeze({
  form: "sankey",

  /** The declared tiers of an overview or a step, left to right. */
  tiersOf(chart) {
    return (chart.sankey && chart.sankey.tiers) || [];
  },

  /** The caps a chart declares, one per tier that needs one. */
  caps(chart) {
    return (chart.sankey && chart.sankey.caps) || [];
  },

  /**
   * The tier set a rung draws at a budget: the step's tiers, less the widened
   * columns the budget drops (from the END of `widen`, so a narrowed window
   * has no hole) and the columns the rung dropped as empty. Re-asked after
   * every drop: a tier already dropped as empty is an entry of this same
   * order, and a fixed shortfall would drop it twice.
   */
  columns(chart, rung, budget) {
    const tiers = SANKEY.tiersOf(chart);
    const widen = (chart.sankey && chart.sankey.widen) || [];
    const drop = new Set((rung && rung.dropped) || []);
    for (let k = widen.length - 1; k >= 0 && tiers.length - drop.size > budget; k--) {
      drop.add(widen[k]);
    }
    return drop.size ? tiers.filter((t) => !drop.has(t)) : tiers;
  },

  /**
   * Whether this form can draw `id` opened on `step` out of `doc`: the step's
   * document decomposes it, and on a window step the kept flank of the chart
   * on screen holds it.
   */
  offers(step, doc, onScreen, id) {
    if (!decomposable(step, doc).has(id)) return false;
    if (!step.sankey.keep || !step.sankey.keep.length || !onScreen) return true;
    return flankHolds(onScreen, step, id);
  },

  /**
   * Fits a document to the columns on screen. On the overview, the fold and
   * the reductions flipped. On a rung, the window or the side, then THE
   * MARKS AFTER THE CAP AND THE FOLD, which must not touch them, in this
   * order: the amount a node prints net of reductions is read off the fresh
   * ribbons before any mark is added, then the residual, then the gap over
   * what the residual left, then markContra over what the fold left negative.
   */
  shape(doc, rung, from, tiers) {
    if (!rung) return markContra(foldDocument(doc, tiers));
    const step = rung.step;
    const drawn = (step.sankey.keep && step.sankey.keep.length)
      ? windowFor(rung.chart, doc, rung, tiers)
      : sideOf(doc, rung, tiers, step.sankey.side === SIDE_SOURCE);
    return markContra(markGap(carryResidual(markAmounts(drawn, rung), from, rung, tiers), from, rung));
  },

  /** Whether the drawn chart must be shaped again at fewer columns. */
  refit(drawn, rung, tiers) {
    return rung ? dropEmptyColumns(drawn, rung, tiers) : false;
  },

  /**
   * Lays a shaped document out with d3-sankey, pure of the page: the columns
   * on screen, the group and place lookups and the constants are all it
   * reads. d3-sankey mutates its input, so it gets a copy and the document
   * stays the thing the table and the detail panel read from.
   */
  layOut(drawn, ctx) {
    const d3 = /** @type {any} */ (globalThis).d3;
    const sankey = d3.sankey()
      .nodeId(/** @param {LaidNode} d */ (d) => d.id)
      .nodeWidth(NODE_WIDTH)
      .nodePadding(NODE_PADDING)
      .nodeAlign(alignFor(ctx.tiers))
      // Supplying this switches d3's own ordering pass off.
      .nodeSort(/** @param {LaidNode} a @param {LaidNode} b */ (a, b) =>
        nodeRank(ctx.groupOf, ctx.placeOf, a) - nodeRank(ctx.groupOf, ctx.placeOf, b) || b.value - a.value)
      // The same column count render() sizes the viewBox from.
      .extent([[LABEL_GUTTER, 12],
        [chartWidth(ctx.columns) - LABEL_GUTTER, CHART_HEIGHT - 12]]);
    const graph = sankey({
      nodes: drawn.nodes.map((n) => Object.assign({}, n)),
      links: drawn.links.map((l) => Object.assign({}, l, { value: l.value_cents })),
    });
    restackLinks(graph);
    return graph;
  },

  /**
   * Draws a laid graph into ctx.svg: ribbons, marks, bands and labels, with
   * the classes, descriptions and gestures the page hands it. Paints nothing:
   * the page paints after, so a theme change repaints without redrawing.
   */
  render(graph, ctx) {
    const d3 = /** @type {any} */ (globalThis).d3;
    const svg = ctx.svg;
    const width = chartWidth(ctx.columns);
    const height = CHART_HEIGHT;
    const on = ctx.on;

    // No width or height attributes: the viewBox makes the drawing scale.
    svg.attr("viewBox", "0 0 " + width + " " + height);
    svg.selectAll("g").remove();

    const gLinks = svg.append("g").attr("class", "links");
    const gNodes = svg.append("g").attr("class", "nodes");

    gLinks.selectAll("path")
      .data(graph.links)
      .join("path")
      .attr("class", /** @param {LaidLink} d */ (d) => ctx.classes.link(d))
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

    const node = gNodes.selectAll("g")
      .data(graph.nodes)
      .join("g")
      .attr("class", /** @param {LaidNode} d */ (d) => ctx.classes.node(d))
      .attr("tabindex", 0)
      .attr("role", "button")
      // aria-pressed is the isolation, on every node. OPENING IS NOT THE TOGGLE
      // and must never be announced as one: it replaces the chart, leaving no
      // pressed state to return to. The label announces what a node opens into.
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
      // node; a double click and Enter open the nodes that open.
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
      // ON EVERY NODE, not only one that opens: its two clicks have already
      // toggled the isolation twice, and this restores what was isolated before.
      // preventDefault stops the double click selecting the label.
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
    const place = (/** @type {LaidNode} */ d) => labelPlacement(ctx.tiers, d, lastColumn);
    const lastColumn = Math.max(...graph.nodes.map((d) => columnOf(ctx.tiers, d)));
    const qualified = labelQualifiers(ctx.doc, ctx.tiers, graph.nodes);
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
      .text(/** @param {LaidNode} d */ (d) => "  " + fmtShortSigned(markCents(d)));
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

  /** The most columns any of these steps declares. */
  widest(steps) {
    return steps.reduce((most, s) => Math.max(most, SANKEY.tiersOf(s).length), 0);
  },
});
