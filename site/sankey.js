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
  FoldRefusal, capColumn, carriedResidual, fmt, fmtShortSigned, foldDocument, foldTarget, isAggregate, isExpansionSchedule, isFundGroup, isGap, isLink, isResidual, licensedGap, openSplitOf, printedNet, reducedOf, say, scoped, splitTailID, splitsOf, tailFigure, tierFrontier, withinNode,
} from "./core.js";

export const NODE_WIDTH = 14;

export const NODE_PADDING = 14;

/** The surface gap that separates stacked ribbons, in px (1px each side). */
export const RIBBON_GAP = 2;

export const CHART_HEIGHT = 820;

/**
 * The canvas a chart is laid out on, in px: under a declared sankey.grow,
 * node_px for each node of the busiest slot of `nodes`, held between its
 * floor and its ceiling; else its declared sankey.height, or CHART_HEIGHT
 * where it declares neither.
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
  return (hints && hints.height) || CHART_HEIGHT;
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
 * declares its canvas (sankey.height or sankey.grow), the one kind of chart
 * whose thin ribbons are accepted rather than folded away. On the form's own
 * canvas a ribbon keeps the stroke floor every chart has.
 * @param {FiscChart | FiscDrillStep | null | undefined} chart
 * @param {LaidLink} d
 * @returns {boolean}
 */
export function isHairline(chart, d) {
  const hints = chart && chart.sankey;
  return Boolean(hints && (hints.height || hints.grow)) && d.width < HAIRLINE;
}

/**
 * The document less the tiers a chart declares as dropped (sankey.drop): their
 * nodes and every link touching one. Go refuses a drop whose links cite a
 * fact no kept link cites, so what leaves is a restatement.
 * @param {FiscProjection} doc
 * @param {FiscChart | FiscDrillStep | null | undefined} chart
 * @returns {FiscProjection} doc itself where nothing is dropped
 */
export function dropTiers(doc, chart) {
  const drop = (chart && chart.sankey && chart.sankey.drop) || [];
  if (!drop.length) return doc;
  const gone = new Set(doc.nodes.filter((n) => drop.indexOf(n.tier) >= 0).map((n) => n.id));
  return Object.assign({}, doc, {
    nodes: doc.nodes.filter((n) => !gone.has(n.id)),
    links: doc.links.filter((l) => !gone.has(l.source) && !gone.has(l.target)),
  });
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

/** SankeyHints.Side for a step opening the node its chart's links come FROM. */
export const SIDE_SOURCE = "source";

/**
 * SankeyHints.Side for a step drawing the opened node between what its document
 * sends into it and what it sends out: the TARGET of one half and the SOURCE
 * of the other, both off the step's own document.
 */
export const SIDE_BOTH = "both";

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
 * The declared columns a rung's drawn chart must hold a node in: every one
 * but a widened one, which columnsAt leaves out where it comes out empty.
 * d3-sankey counts columns from depth while alignFor places nodes by
 * declared index, so it throws on an empty leading or interior column and
 * stretches the others across an empty trailing one. The kept
 * ones are the flank's to fill (flankHolds); the rest, the fresh half's at
 * every column set a budget draws (decomposable). The one answer columnsAt
 * and decomposable read, so what is offered draws at every width.
 * @param {FiscDrillStep} step
 * @returns {number[]}
 */
export function promisedTiers(step) {
  const widen = new Set(step.sankey.widen || []);
  return step.sankey.tiers.filter((t) => !widen.has(t));
}

/**
 * The halves a step draws out of its own document, each a run of columns and
 * whether the opened node is the end its ribbons come FROM: one for a window
 * (the kept flank is not the document's) and for a source or target step, and
 * two for a step opening both sides, which meet on the opened column. The one
 * answer freshHalf, decomposable and SANKEY.shape read.
 * @param {FiscDrillStep} step
 * @param {number[]} [tiers] the fresh columns on screen; the declared ones by default
 * @returns {{tiers: number[], nearIsSource: boolean}[]}
 */
export function freshHalves(step, tiers = freshTiers(step)) {
  if (step.sankey.keep && step.sankey.keep.length) return [{ tiers, nearIsSource: flankIsLeft(step) }];
  if (step.sankey.side !== SIDE_BOTH) return [{ tiers, nearIsSource: step.sankey.side === SIDE_SOURCE }];
  const at = tiers.indexOf(step.from);
  if (at <= 0 || at >= tiers.length - 1) {
    throw new Error("cannot draw step " + step.key + ": it opens tier " + step.from + " on both sides " +
      "and the columns on screen are " + tiers.join(", ") + ", which put nothing on one side of it");
  }
  return [
    { tiers: tiers.slice(0, at + 1), nearIsSource: false },
    { tiers: tiers.slice(at), nearIsSource: true },
  ];
}

/**
 * Two charts joined on the nodes they share: the first's nodes in its order,
 * the second's it lacks after them, and both sets of ribbons. The second is
 * the document the result is OF. A node both hold is drawn once, so the
 * opened node a window or a two-sided step meets on is one mark.
 * @param {FiscProjection} first
 * @param {FiscProjection} second
 * @returns {FiscProjection}
 */
export function splice(first, second) {
  const have = new Set(first.nodes.map((n) => n.id));
  return Object.assign({}, second, {
    nodes: first.nodes.concat(second.nodes.filter((n) => !have.has(n.id))),
    links: first.links.concat(second.links),
  });
}

/**
 * What a step's document draws for one node, unfolded, every half spliced:
 * the reach the chart is drawn with (reaching), at the fresh columns given.
 * At every declared column by default, which is the answer to whether a
 * residual's leaving legs exist (carryResidual) and must not move with the
 * viewport; at the columns a budget draws, the answer heldAt reads.
 * @param {FiscDrillStep} step
 * @param {FiscProjection} doc
 * @param {string} id
 * @param {number[]} [tiers] the fresh columns to read at; every declared one by default
 * @returns {FiscProjection}
 */
export function freshHalf(step, doc, id, tiers = freshTiers(step)) {
  return freshHalves(step, tiers)
    .map((h) => filterLinks(doc, id, h.tiers, reaching(doc, id, h.nearIsSource, h.tiers)))
    .reduce(splice);
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
 * @returns {(src: FiscNode, dst: FiscNode, byID: Map<string,FiscNode>, drawn: ReadonlySet<string>) => boolean}
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
 * @param {(src: FiscNode, dst: FiscNode, byID: Map<string,FiscNode>, drawn: ReadonlySet<string>) => boolean} holds
 * @returns {FiscProjection}
 */
export function filterLinks(doc, id, tiers, holds) {
  const byID = new Map(doc.nodes.map((n) => [n.id, n]));
  const drawn = tierFrontier(doc, tiers);
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
        if (drawn.has(at.id)) break;
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
 * keeps no flank and opens one side, and one half of a window or of a step
 * opening both sides.
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
  const drawn = foldDocument(shaped, tierFrontier(shaped, tiers));

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
  const [half] = freshHalves(step, tiers.filter((t) => !keep.has(t)));
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
  const fresh = sideOf(stepDoc, rung, half.tiers, half.nearIsSource);
  // THE KEPT CENTRE MUST HOLD THE OPENED NODE: a window whose flank sends
  // nothing into it is refused rather than drawn as its fresh half alone.
  if (!kept.nodes.some((n) => n.id === rung.id)) {
    throw new Error("cannot draw " + stepDoc.projection + ": the chart on screen sends nothing " +
      "between tiers " + keptTiersOf(step).join(", ") + " and " + rung.id + ", so there is no flank to keep");
  }

  // carried_from IS SET WHERE ABSENT AND NEVER CLEARED: a node keeps the stem
  // whose figure and caveats it carries, however many rungs down.
  const stem = onScreen.projection || "";
  const carried = Object.assign({}, kept, {
    nodes: kept.nodes.map((n) => (n.id === rung.id || n.carried_from
      ? n
      : Object.assign({}, n, { carried_from: stem }))),
  });
  // The spliced document is the step document's; the kept flank is a guest.
  return splice(carried, fresh);
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
 * carries no residual or gap, and folds: each is a refusal of windowFor's,
 * so the offer and the draw are one rule and no mark is classed as opening
 * whose window would banner. Asked per mark per paint, which is why the
 * fold's refusal is an answer here rather than a throw; any other error is a
 * defect and is thrown.
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
      if (!(e instanceof FoldRefusal)) throw e;
      holds = false;
    }
    byStep.set(at, holds);
  }
  return holds;
}

/**
 * heldAt's answers per document, per step, node and fresh column set:
 * activeTiers asks per paint, nextBudget per width and decomposable per node
 * per budget. Keyed on the frozen document, so nothing here is page state.
 * @type {WeakMap<FiscProjection, Map<string, {links: number, tiers: Set<number>}>>}
 */
const helds = new WeakMap();

/**
 * Which of the fresh columns `tiers` the opened node's fresh half holds a
 * node in, and how many ribbons it draws there. A node at a drawn tier is in
 * the half only as some ribbon end's nearest drawn ancestor-or-self -- the
 * column the fold puts it in -- and neither the cap nor the fold empties a
 * column, so this is what the drawn half holds. The one reading columnsAt
 * drops an empty widened column by and decomposable offers by.
 * @param {FiscDrillStep} step
 * @param {FiscProjection} doc
 * @param {string} id
 * @param {number[]} tiers the fresh columns, in order
 * @returns {{links: number, tiers: Set<number>}}
 */
export function heldAt(step, doc, id, tiers) {
  let byKey = helds.get(doc);
  if (!byKey) {
    byKey = new Map();
    helds.set(doc, byKey);
  }
  const at = step.key + "\u001f" + id + "\u001f" + tiers.join(",");
  let held = byKey.get(at);
  if (!held) {
    const half = freshHalf(step, doc, id, tiers);
    held = { links: half.links.length, tiers: new Set(half.nodes.map((n) => n.tier)) };
    byKey.set(at, held);
  }
  return held;
}

/**
 * The columns a rung draws at a budget: the step's tiers, less the widened
 * columns the budget drops (from the END of `widen`, so a narrowed window has
 * no hole) and the widened columns the opened node's fresh half leaves empty
 * at what is left (heldAt). A column dropped as empty frees a place the
 * budget gives the next widened column back, so the set is asked again until
 * it holds every column it keeps; each pass drops one more widened tier, so
 * it ends. Asked afresh at every budget and kept nowhere: whether a widened
 * column is empty depends on the tier set, since a ribbon into it folds onto
 * an ancestor at a narrower one, which can be its own source or behind it.
 * Without a rung, the budget's cut alone.
 * @param {FiscDrillStep} step
 * @param {{id: string, doc: FiscProjection} | null} rung
 * @param {number} budget
 * @returns {number[]}
 */
export function columnsAt(step, rung, budget) {
  const tiers = step.sankey.tiers;
  const widen = step.sankey.widen || [];
  const keep = new Set(step.sankey.keep || []);
  const promised = new Set(promisedTiers(step));
  const empty = new Set();
  for (;;) {
    const drop = new Set(empty);
    for (let k = widen.length - 1; k >= 0 && tiers.length - drop.size > budget; k--) drop.add(widen[k]);
    const drawn = drop.size ? tiers.filter((t) => !drop.has(t)) : tiers;
    if (!rung || !widen.length) return drawn;
    const held = heldAt(step, rung.doc, rung.id, drawn.filter((t) => !keep.has(t)));
    const gone = drawn.filter((t) => !promised.has(t) && !held.tiers.has(t));
    if (!gone.length) return drawn;
    for (const t of gone) empty.add(t);
  }
}

/**
 * The ids at a step's opened tier that its document decomposes, computed once
 * per (document, step, budgets) because nodeClass asks per mark per paint:
 * those whose fresh half holds a node in each promised column the flank does
 * not keep at the columns EVERY budget draws (columnsAt). A node whose
 * decomposition misses one at any width is not offered, since columnsAt
 * drops only a widened column and the chart could not be laid out at the
 * columns that width draws.
 * @type {WeakMap<FiscProjection, Map<string, Set<string>>>}
 */
const decomposed = new WeakMap();

/**
 * @param {FiscDrillStep} step
 * @param {FiscProjection} doc
 * @param {number[]} budgets every budget the page can draw at
 * @returns {Set<string>}
 */
export function decomposable(step, doc, budgets) {
  let byStep = decomposed.get(doc);
  if (!byStep) {
    byStep = new Map();
    decomposed.set(doc, byStep);
  }
  const key = step.key + "\u001f" + budgets.join(",");
  const have = byStep.get(key);
  if (have) return have;
  const keep = new Set(step.sankey.keep || []);
  const fill = promisedTiers(step).filter((t) => !keep.has(t));
  const out = new Set();
  for (const n of doc.nodes) {
    if (n.tier !== step.from || (step.role && n.role !== step.role)) continue;
    const rung = { id: n.id, doc: doc };
    const draws = budgets.every((b) => {
      const held = heldAt(step, doc, n.id, columnsAt(step, rung, b).filter((t) => !keep.has(t)));
      return held.links > 0 && fill.every((t) => held.tiers.has(t));
    });
    if (draws) out.add(n.id);
  }
  byStep.set(key, out);
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
 * Draws the opened node at the figure it prints where its drawn ribbons do
 * not add up to it: a node a schedule prints a reduction under, the reduction
 * drawn forward at its magnitude by markContra. The figure is printedNet
 * over what the step's document sends into the node -- the same one the
 * citywide chart labels the node with -- read before any mark is added or
 * any sign is flipped.
 *
 * READ AT EVERY DECLARED COLUMN, NOT AT THE COLUMNS ON SCREEN: a reduction
 * whose line stands at a widened tier folds onto the node it reduces when
 * the budget drops that tier, and the drawn chart then nets to the gross. A
 * step's document does not change with the viewport, so neither does this
 * figure.
 *
 * d3-sankey would otherwise size the node at the gross its forward-drawn
 * ribbons add to, a figure no page prints. Setting the value here means every
 * surface reads one figure.
 *
 * @param {FiscProjection} drawn  shaped and folded, before any mark
 * @param {Rung} rung
 * @returns {FiscProjection} drawn itself when no ribbon into the node is a reduction
 */
export function markAmounts(drawn, rung) {
  const step = rung.step;
  const half = freshHalf(step, rung.doc, rung.id);
  // Nothing to net where no ribbon is printed negative, so the half is
  // folded only then: a document is read whole at a width that draws part
  // of it only where a reduction is in it.
  if (!half.links.some((l) => l.value_cents < 0)) return drawn;
  const printed = printedNet(foldDocument(half, tierFrontier(half, freshTiers(step))), rung.id);
  if (!printed.reduced) return drawn;
  return Object.assign({}, drawn, {
    nodes: drawn.nodes.map((n) => (n.id === rung.id ? Object.assign({}, n, { fixedValue: printed.net }) : n)),
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
 * carriedResidual over this form's answers: whether the step's document
 * decomposes the opened node, read at every declared column so the figures
 * do not move with the viewport; whether this width draws the column a
 * leaving endpoint stands in; and where the mark and a carried endpoint
 * stand.
 *
 * DECOMPOSED IS THE STEP'S DECLARATION, NOT THIS WIDTH: whether the document
 * sends anything out of a part of the opened node at a tier the step
 * declares, each part read at the column it folds to there. Asked of the
 * drawn window instead, a budget that drops the parts' outward column would
 * drop the leaving legs from the mark's figure as well as from the chart.
 *
 * A LEAVING LEG IS DROPPED WHERE THE BUDGET DROPS ITS COLUMN, as every other
 * ribbon of that column is; placed in the last column the chart has, it
 * would draw a ribbon of no length.
 *
 * THE MARK STANDS AT THE SHALLOWEST DECLARED TIER OF ANY PART OF THE OPENED
 * NODE, read off the step's unfolded document; a node with no part at a
 * declared tier has nowhere to stand it, and neither has one whose tier this
 * width does not draw, by the budget or by columnsAt: d3 would clamp the mark
 * into the first column. ENDPOINTS STAND AT THE FIRST DRAWN
 * TIER WHEN THEIR FLOW ARRIVES AND THE LAST WHEN IT LEAVES -- drawn, not
 * declared: an undrawn declared tier is clamped to the first column and the
 * ribbon runs backwards. Filtered in the step's own order, which is a column
 * order and not a sorted set.
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
  if (!step.residual || !from || !step.projection) return drawn;
  const opened = rung.id;
  const doc = rung.doc;
  const inside = withinNode(doc, opened);
  const declared = freshHalf(step, doc, opened);
  const byDocID = new Map(doc.nodes.map((n) => [n.id, n]));
  const declaredFrontier = tierFrontier(doc, freshTiers(step));
  const decomposed = declared.links.some((l) => {
    const part = foldTarget(byDocID, /** @type {FiscNode} */ (byDocID.get(l.source)), declaredFrontier);
    return part !== "" && part !== opened && inside.has(part);
  });
  let mark = -1;
  for (const n of doc.nodes) {
    if (n.id !== opened && inside.has(n.id) && step.sankey.tiers.includes(n.tier) && (mark < 0 || n.tier < mark)) {
      mark = n.tier;
    }
  }
  if (!onScreen.includes(mark)) mark = -1;
  const tiers = step.sankey.tiers.filter((t) => drawn.nodes.some((n) => n.tier === t));
  return carriedResidual(drawn, from, rung, decomposed, leavingLegDrawn(step, onScreen),
    { mark: mark, arriving: tiers[0], leaving: tiers[tiers.length - 1] });
}

/**
 * licensedGap with the mark stood where its figure is: too little leaving
 * at the last declared tier, too little arriving at the first.
 * @param {FiscProjection} drawn  the rung's chart, shaped, folded and spliced
 * @param {FiscProjection | null} from  the document of the chart the rung was
 *   opened from
 * @param {Rung} rung
 * @returns {FiscProjection}
 */
export function markGap(drawn, from, rung) {
  // The hint is read only once there is a mark to stand: a balanced chart
  // with no licence is returned as drawn, whatever the step declares.
  return licensedGap(drawn, from, rung, (gap) => {
    const tiers = rung.step.sankey.tiers;
    return gap > 0 ? tiers[tiers.length - 1] : tiers[0];
  });
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
  let drawn = foldDocument(expansion, open, { mergeInferred: true });
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
  if (tails.size) drawn = foldDocument(drawn, drawn.nodes.map((n) => n.id), { mergeInferred: true });
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
    // The neighbour's fund group, not the neighbour: on the drill-down the
    // ends are funds and departments.
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
 * The overview as drawn: its dropped tiers gone, each capped tier the reader
 * has not drawn out folded into its tail, in the order the chart draws its
 * tiers as a rung's are (sideOf), then the fold. Each tail spans the whole
 * column, so its parent is "" and its note is finished with the figure only
 * the folded document knows.
 * @param {FiscProjection} doc
 * @param {number[]} tiers
 * @param {FiscChart} chart
 * @param {Set<number>} expanded the tiers drawn whole rather than capped
 * @returns {FiscProjection}
 */
export function overviewOf(doc, tiers, chart, expanded) {
  const caps = (chart.sankey && chart.sankey.caps) || [];
  let shaped = dropTiers(doc, chart);
  for (const tier of tiers) {
    const cap = caps.find((c) => c.tier === tier);
    if (!cap || expanded.has(tier)) continue;
    shaped = capColumn(shaped, tier, cap.cap, "", cap.tail);
  }
  const drawn = foldDocument(shaped, tierFrontier(shaped, tiers));
  if (!caps.length) return drawn;
  return Object.assign({}, drawn, {
    nodes: drawn.nodes.map((n) => (isAggregate(n.id)
      ? Object.assign({}, n, {
        parent: "",
        source_note: n.source_note + say("aggregate_together", { figure: fmt(tailFigure(drawn, n.id)) }),
      })
      : n)),
  });
}

/**
 * The Sankey form's renderer, as app.js's FORMS holds it under "sankey".
 * Every method takes what it needs: the chart's declaration, the frontier,
 * the document on screen. It reads of a chart `chart.sankey` and nothing of
 * any other form's hints; site/layout.test.mjs and site/whole.test.mjs wrap
 * the hints in a recording Proxy and hold every read to this file.
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
   * (expansionOf), any other document folded to the chart's tier set with its
   * capped tiers folded (overviewOf).
   */
  shape(doc, view) {
    if (isExpansionSchedule(doc)) return markContra(expansionOf(doc, view.frontier, view.chart, view.whole));
    return markContra(overviewOf(doc, SANKEY.tiersOf(view.chart), view.chart, view.expanded));
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
