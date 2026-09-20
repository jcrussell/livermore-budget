// rungs.mjs — what the client still works out for itself, held to Go's answer.
//
// GO'S ANSWER IS A COMMITTED ARTIFACT WITH ITS OWN GO TEST, testdata/rungs.json,
// and this file reads that and nothing else of Go's. It is NOT re-derived from
// pkg/cmd/export/data.go by regex the way harness.mjs reads the step shapes:
// that would compare app.js against a reading of the file that generated the
// config app.js was handed, and report every rung PASS whatever either side
// did. The artifact is computed by walking each document's ribbons in Go, and
// pkg/cmd/export's TestTheRungArtifactIsWhatGoComputes refuses one that is not
// what Go computes.
//
// EVERY ARM HERE IS TWO PARTIES, which is a claim about the seam and not about
// this file's diligence. Go's answer says what a column HOLDS, unfolded, and
// app.js fits what it was given to a viewport Go cannot see. So what is
// compared below is a decision the client still makes -- which step opened a
// rung, which columns it lays out and in what order, whether a column folds,
// how many its tail stands for, and every cent on a mark of its own -- against
// a figure or a declaration from Go's side.
//
// THE MEMBERSHIP ARM IS GONE WITH ITS SUBJECT, and that is the boundary
// arriving rather than a guarantee lost. It compared the ids app.js's own
// filter reached against Go's `ids`, and app.js has no filter of its own to
// compare: heldFor reads `draws[].ids` and the page draws the ribbons between
// the nodes it names. Left in place the arm would have compared the artifact
// against itself and reported PASS on every run -- the "green because the gate
// fired" shape AGENTS.md, "Prove it can fail" says to hunt for. The same goes
// for the split between a column's parts and the marks carried beside them:
// both lists come off the answer now. Whether Go's answer is RIGHT is
// pkg/cmd/export's TestTheRungArtifactIsWhatGoComputes and
// TestTheRungArtifactIsWhatTheReachPrimitivesAnswer, in Go, where it belongs
// (AGENTS.md, "Go vets, JavaScript renders").
//
// THE COLUMN IS READ EXPANDED, AND THAT IS WHAT MAKES A CAPPED ONE COMPARABLE.
// A fold puts the ids it drops behind a single tail mark, so a folded chart
// cannot be asked which nodes its column holds -- ask it and the answer is the
// cap. expandTier puts them back, so the walk expands every tail at every rung
// before it reads a column or takes its offers, and the set compared is the
// one app.js's own filter reached.
//
// THE FIT IS COMPARED TOO, AGAINST THE SIZE GO REPORTS AND NOT AGAINST A
// SECOND COPY OF THE RANKING. Which eight a column keeps is this page's, so
// nothing here re-ranks it; what is checked is that the fold engages exactly
// where Go's column holds more than cap + 1, that the fitted column is then
// the cap, that its tail stands for exactly the rest, and that every mark left
// on screen is one the expanded column holds.
//
// SO, PER CHECK, WHAT EACH ARM WITNESSES:
//
// THE COLUMNS AND THEIR ORDER. activeTiers reads the step's own declaration,
// trims it to the column budget and drops a widened column the drawn document
// left empty; none of that comes from the artifact, so a client drawing a
// column Go does not answer, or drawing them in another order, is red. A
// column Go answers that the client does not draw has to be one the step
// declares in `widen`, read out of data.go rather than off the config app.js
// was handed.
//
// WHICH STEP OPENED THE RUNG. stepFor answers from CONFIG.steps and the node's
// own tier and role.
//
// THE CAP AND THE FOLD. cap + 1 is capColumn's threshold and the cap is the
// step's declaration; both are measured against the size of the column Go
// answers, so neither side of that comparison is the other's.
//
// AND THE MARKS THE CLIENT ADDS OF ITS OWN: the residual carryResidual stands
// beside the opened node's parts and the gap markGap states, each by id, tier
// and the cents that arrive at it and leave it, against the rung's `marks`,
// complete both ways -- a mark Go answers that the client does not draw and
// one the client draws that Go does not answer are the same disagreement. Read
// on the fitted chart AND on the expanded one, which is where the client's
// half of "a mark is fold-invariant" is witnessed: Go answers a rung's marks
// once for every way it can be fitted. The marks' prose is not compared here;
// drill.mjs pins it. Every cent on this chart is the client's arithmetic, so
// this arm is a cross-language comparison like the rest.
//
// MUTATION: perturb a mark's in_cents by 1, or delete the mark, and the marks
// arm is red naming the rung and the mark. Narrow a step's declared cap in
// data.go and the fold arm is red on the column that folds on the wrong side
// of cap + 1. Blind walkAt -- return an empty map -- and every counter goes to
// zero and every arm is red on its own counter rather than green with nothing
// compared.
//
// AND THE MUTATION THIS FILE NO LONGER CATCHES, which is where a removed id is
// now seen instead: taking one out of a column's `ids` in testdata/rungs.json
// takes the mark off the chart, and the pinned mark counts in chart.mjs and
// drill.mjs go red rather than anything here.
//
// COMPLETENESS BOTH WAYS, OVER THE EXPAND-THEN-OPEN SPACE. The space is named
// because a count over the wrong one reads as coverage it is not (fisc-22qj):
// the walk expands every folded column before taking a rung's offers, so a
// node the cap hid is a node this walk opens. A rung the walk visits that the
// artifact does not answer is red, and a rung the artifact answers that the
// walk never visits is red too. Within a rung the same holds per column, up to
// the columns the budget drops.
import { readFileSync } from "node:fs";
import { join } from "node:path";

import { repoRoot, settle, stepShapes } from "./harness.mjs";
import { COLUMNS, expandAll, openedWide } from "./drill.mjs";

const ARTIFACT = JSON.parse(readFileSync(join(repoRoot, "testdata", "rungs.json"), "utf8"));
if (ARTIFACT.schema_version !== 5) {
  throw new Error(`testdata/rungs.json declares schema_version ${ARTIFACT.schema_version}; this module reads 5`);
}

// THE DECLARATIONS COME FROM GO'S SOURCE AND NOT FROM THE CONFIG THE PAGE WAS
// HANDED. What may fold and which column a fourth buys are DrillStep.Caps and
// DrillStep.Widen; reading them off CONFIG.steps would be reading the value
// app.js is already acting on, and every arm below that leans on one would
// pass whatever app.js did with it.
const STEPS = new Map(stepShapes().map((s) => [s.key, s]));

/** @param {string[]} path */
const keyOf = (path) => path.join(" > ");

// A unit separator rather than a comma or a space, because a node id carries
// both and a list joined on one of those compares equal to a different list.
const SEP = "\u001f";

/** The cap a step declares for one tier, or 0 where it declares none. */
function capFor(step, tier) {
  const shape = STEPS.get(step);
  if (!shape) throw new Error(`data.go declares no step keyed ${JSON.stringify(step)}`);
  const cap = (shape.caps || []).find((c) => c.tier === tier);
  return cap ? cap.cap : 0;
}

/** The tiers a step declares a fourth column buys, which are the ones a narrow budget drops. */
const widenOf = (step) => (STEPS.get(step) || {}).widen || [];

/**
 * The gap is drawn on one year only -- FY2026-27's services-and-supplies is
 * the one cell the two schedules print apart -- so a column pinned to draw
 * none must compare none, and one pinned to draw a gap must compare one.
 * drill.mjs pins which column that is, as gapCents.
 */
const drawsAGapIn = (col) => Boolean(col.object.gapCents);

/**
 * The client's own marks on the chart on screen, spelled the way the
 * artifact spells them: id, role, tier, and the cents that arrive at each
 * and leave it, summed over the chart's ribbons. Every node of the chart is
 * read and not only those at an active tier, because both marks index the
 * step's declared tiers and a mark at a column the budget dropped is a
 * placement to compare, not one to overlook.
 * @param {any} app
 */
function marksOn(app) {
  /** @type {{id: string, role: string, tier: number, in_cents: number, out_cents: number}[]} */
  const marks = [];
  for (const n of app.projection.nodes) {
    if (!app.isResidual(n.id) && !app.isGap(n.id)) continue;
    let inCents = 0;
    let outCents = 0;
    for (const l of app.projection.links) {
      if (l.target === n.id) inCents += l.value_cents;
      if (l.source === n.id) outCents += l.value_cents;
    }
    marks.push({ id: n.id, role: n.role, tier: n.tier, in_cents: inCents, out_cents: outCents });
  }
  return marks.sort((a, b) => (a.id < b.id ? -1 : a.id > b.id ? 1 : 0));
}

/** @param {{id: string, role: string, tier: number, in_cents?: number, out_cents?: number}} m */
const spellMark = (m) => `${m.id} (${m.role}) at tier ${m.tier}, in ${m.in_cents || 0} out ${m.out_cents || 0}`;

/**
 * What the client drew for the rung on screen: its tiers, and for every tier
 * which document nodes it draws as the opened node's own parts, which it draws
 * but does not count as one, and how many its folded tail says it stands for;
 * and the marks the client added of its own.
 * @param {any} app
 */
function drawn(app) {
  const nodes = app.projection.nodes;
  /** @type {Record<number, {ids: string[], carried: string[], hidden: number}>} */
  const byTier = {};
  for (const t of app.activeTiers()) {
    const at = nodes.filter((n) => n.tier === t);
    // A DOCUMENT NODE AND NOTHING THE CLIENT ADDED, split the way the client
    // splits it: a node the document marks derived or isCarried names is
    // drawn but is not one of the opened node's parts. isCarried answers yes
    // for the residual and the gap too, which are the client's own marks and
    // are taken out first.
    const own = at.filter((n) => !app.isAggregate(n.id) && !app.isResidual(n.id) && !app.isGap(n.id));
    const ids = own.filter((n) => !n.derived && !app.isCarried(n.id)).map((n) => n.id).sort();
    const carried = own.filter((n) => n.derived || app.isCarried(n.id)).map((n) => n.id).sort();
    const tails = at.filter((n) => app.isAggregate(n.id));
    if (tails.length > 1) throw new Error(`tier ${t} draws ${tails.length} folded tails`);
    let hidden = 0;
    if (tails.length === 1) {
      const m = /^(\d+) smaller /.exec(tails[0].label);
      if (!m) throw new Error(`the folded tail at tier ${t} is labelled ${JSON.stringify(tails[0].label)}, which does not say how many it stands for`);
      hidden = Number(m[1]);
    }
    byTier[t] = { ids, carried, hidden };
  }
  return { tiers: app.activeTiers().slice(), byTier, marks: marksOn(app) };
}

/**
 * The whole tree at one budget, every offered node opened and measured twice
 * -- once as the reader is first shown it, once with every folded column
 * expanded -- reopened from the overview per offer the way drill.mjs's
 * everyOpenedView does. Written here rather than reusing that walk because
 * this one needs the key of the step each offer opens under, which the visitor
 * there is not given.
 *
 * EVERY REACHED PATH IS EXPANDED ON THE WAY IN, not only at its end. A node
 * two rungs down that an outer chart's cap was hiding is reachable only if
 * that column was expanded before it was clicked, so walking back to a path
 * means re-expanding at every step of it. That is the difference between this
 * walk and the space the artifact used to be cut to (fisc-qics).
 * @param {any} col
 * @param {number} width
 */
async function walkAt(col, width) {
  const app = await openedWide(width, [], col);
  /** @type {Map<string, {step: string, fitted: ReturnType<typeof drawn>, whole: ReturnType<typeof drawn>}>} */
  const seen = new Map();
  let expansions = 0;
  const open = async (/** @type {string} */ id) => {
    const outcome = await app.drillDown(id);
    await settle();
    if (outcome !== "drew") throw new Error(`drillDown(${id}) ${outcome}`);
  };
  const at = async (/** @type {string[]} */ path) => {
    app.drillUp(0);
    for (const id of path) {
      expansions += expandAll(app);
      await open(id);
    }
  };
  const walk = async (/** @type {string[]} */ path) => {
    await at(path);
    expansions += expandAll(app);
    const offers = app.projection.nodes.filter((n) => app.drillable(n))
      .map((n) => ({ id: n.id, step: app.stepFor(n).key }));
    for (const o of offers) {
      await at(path);
      expansions += expandAll(app);
      await open(o.id);
      const next = path.concat([o.id]);
      if (seen.has(keyOf(next))) continue;
      const fitted = drawn(app);
      expansions += expandAll(app);
      seen.set(keyOf(next), { step: o.step, fitted, whole: drawn(app) });
      await walk(next);
    }
  };
  await walk([]);
  app.drillUp(0);
  return { seen, expansions };
}

/**
 * @returns {Promise<{name: string, ok: boolean, detail: string}[]>}
 */
export async function checks() {
  const out = [];
  for (const col of COLUMNS) {
    const column = ARTIFACT.columns.find((c) => c.stem === col.stem);
    if (!column) throw new Error(`testdata/rungs.json answers for no column with stem ${col.stem}`);
    for (const width of [3, 4]) {
      const expected = new Map(column.rungs.map((r) => [keyOf(r.path), r]));
      const { seen, expansions } = await walkAt(col, width);

      /** @type {string[]} */
      const wrong = [];
      /** @type {string[]} */
      const unanswered = [];
      // EVERY COUNTER IS A COMPARISON THAT CAN FAIL, and there is no longer
      // one for a comparison that cannot: nothing below reads a set off the
      // artifact and then compares the artifact against it.
      let flankDrawn = 0;
      let tiersCompared = 0;
      let budgetDropped = 0;
      let capHeld = 0;
      let foldsEngaged = 0;
      let marksCompared = 0;
      let residualsCompared = 0;
      let gapsCompared = 0;
      for (const [key, { step, fitted, whole }] of seen) {
        const want = expected.get(key);
        if (!want) {
          unanswered.push(key);
          continue;
        }
        expected.delete(key);
        const where = `${key} at ${width} columns`;
        if (want.step !== step) {
          wrong.push(`${where} opened under step ${step}, Go says ${want.step}`);
        }
        // THE COLUMN ORDER IS GO'S, UP TO THE COLUMNS A NARROW BUDGET DROPS. A
        // tier drawn that Go does not answer and two tiers drawn in the other
        // order are the same disagreement; a tier answered that is not drawn
        // is one only where the step declares that column a widening, which is
        // read from Go's source and not from the config app.js holds.
        const wantTiers = want.draws.map((/** @type {any} */ d) => d.tier);
        const order = wantTiers.filter((/** @type {number} */ t) => fitted.tiers.includes(t));
        const dropped = wantTiers.filter((/** @type {number} */ t) => !fitted.tiers.includes(t));
        const stray = dropped.filter((/** @type {number} */ t) => !widenOf(want.step).includes(t));
        tiersCompared += fitted.tiers.length;
        budgetDropped += dropped.length;
        if (order.join(",") !== fitted.tiers.join(",") || stray.length) {
          wrong.push(`${where} draws tiers ${fitted.tiers.join(",")}, Go answers ${wantTiers.join(",")}` +
            (stray.length ? `, and ${stray.join(",")} is not a column step ${want.step} declares a widening` : ""));
        }
        // THE MARKS ARE ONE LIST ON BOTH SIDES, BY ID, TIER AND CENTS, so a
        // mark Go answers that the client does not draw, one the client draws
        // that Go does not answer, and one placed or sized differently are all
        // the same disagreement. Read on the fitted chart and on the expanded
        // one, because Go answers a rung's marks ONCE however it is fitted.
        const wantMarks = (want.marks || []).slice().sort((/** @type {any} */ a, /** @type {any} */ b) => (a.id < b.id ? -1 : a.id > b.id ? 1 : 0));
        const wantSpelled = wantMarks.map(spellMark);
        for (const [how, got] of [["fitted", fitted.marks], ["expanded", whole.marks]]) {
          const gotSpelled = got.map(spellMark);
          if (wantSpelled.join(SEP) === gotSpelled.join(SEP)) continue;
          const missing = wantSpelled.filter((m) => !gotSpelled.includes(m));
          const extra = gotSpelled.filter((m) => !wantSpelled.includes(m));
          wrong.push(`${where}, ${how}: app.js draws ${got.length} mark(s) of its own and Go answers ${wantMarks.length}` +
            (missing.length ? `; Go answers and app.js does not draw: ${missing.join("; ")}` : "") +
            (extra.length ? `; app.js draws and Go does not answer: ${extra.join("; ")}` : ""));
        }
        marksCompared += wantMarks.length * 2;
        residualsCompared += wantMarks.filter((/** @type {any} */ m) => m.role === "residual").length;
        gapsCompared += wantMarks.filter((/** @type {any} */ m) => m.role === "gap").length;
        for (const d of want.draws) {
          const mine = whole.byTier[d.tier];
          if (!mine) continue; // a column this budget dropped, reported above
          // IDS IS A LIST OR THE ENTRY IS MALFORMED: Go writes it even when
          // empty, so that a flank answered with nothing is not read as a
          // column left out.
          if (!Array.isArray(d.ids)) {
            throw new Error(`${where}, tier ${d.tier}: Go's entry carries no ids list`);
          }
          if (d.role === "flank" && mine.ids.length + mine.carried.length) flankDrawn++;
          // OMITTED IS EMPTY: Go writes an empty carried list as no field at
          // all, and a reader that compared undefined would fail every column
          // with no mark beside its parts.
          const ids = d.ids.slice().sort();
          const carried = (d.carried || []).slice().sort();
          // THE FIT, AGAINST THE SIZE GO REPORTS. `holds` is Go's count for
          // the column, `cap` the step's declaration and cap + 1 capColumn's
          // threshold, so the only figure in this arm from the client's side
          // is what it drew.
          const fit = fitted.byTier[d.tier];
          if (!fit) continue;
          const holds = ids.length + carried.length;
          const shown = fit.ids.length + fit.carried.length;
          const cap = capFor(want.step, d.tier);
          if (!cap) {
            if (fit.hidden) {
              wrong.push(`${where}, tier ${d.tier}: app.js folds ${fit.hidden} node(s) away at a ` +
                `tier step ${want.step} declares no cap for`);
            }
            continue;
          }
          capHeld++;
          if ((holds > cap + 1) !== (fit.hidden > 0)) {
            wrong.push(`${where}, tier ${d.tier}: Go answers a column of ${holds} under a cap of ` +
              `${cap} and app.js ${fit.hidden ? `folds ${fit.hidden} away` : "draws it whole"}, ` +
              `which is the wrong side of this page's cap + 1`);
          }
          if (!fit.hidden) continue;
          foldsEngaged++;
          // AND THE TAIL STANDS FOR EXACTLY THE REST, which is what says a
          // reader is not shown a column with a figure missing from both the
          // marks and the count on the tail. The count on the tail is the
          // client's own arithmetic -- capColumn composes those words from what
          // it folded -- and `holds` is Go's figure for the column, so this is
          // a comparison and not a restatement.
          if (shown !== cap || shown + fit.hidden !== holds) {
            wrong.push(`${where}, tier ${d.tier}: app.js draws ${shown} and hides ${fit.hidden} of a ` +
              `column Go answers as ${holds}, under a cap of ${cap}`);
          }
        }
      }
      const unvisited = [...expected.keys()];

      // THE GATES COUNT WHAT CAN STILL FAIL. budgetDropped is reported and
      // gates nothing: it is zero at the wide budget by construction, and a
      // gate on it would fail every run at four columns for being right.
      out.push({
        name: `${col.label} at ${width} columns: what the client still works out for itself is what Go computed`,
        ok: wrong.length === 0 && flankDrawn > 0 &&
          tiersCompared > 0 && capHeld > 0 && foldsEngaged > 0 && marksCompared > 0 &&
          residualsCompared > 0 && expansions > 0 && gapsCompared > 0 === drawsAGapIn(col),
        detail: wrong.length === 0
          ? `over ${column.rungs.length} rung(s), read with every fold expanded ` +
            `(${expansions} expansion(s) over the walk): ${tiersCompared} column(s) drawn in Go's ` +
            `order, ${budgetDropped} dropped as a widening this budget cannot afford, ` +
            `${flankDrawn} of them a kept flank the page drew something in; ${capHeld} column(s) ` +
            `held to a declared cap, ${foldsEngaged} of them folded to it with a tail standing for ` +
            `exactly the rest; ${marksCompared} mark comparison(s) of the client's own, ` +
            `${residualsCompared} of them a residual and ${gapsCompared} a gap (the column ` +
            `${drawsAGapIn(col) ? "draws one" : "draws none"})`
          : `${wrong.length} disagreement(s):\n      ${wrong.join("\n      ")}`,
      });
      out.push({
        name: `${col.label} at ${width} columns: Go answers every rung the walk reaches, and no other`,
        ok: unanswered.length === 0 && unvisited.length === 0 && seen.size === col.expandedViews,
        detail: unanswered.length === 0 && unvisited.length === 0
          ? `${seen.size} rung(s) visited over the expand-then-open space -- every folded column ` +
            `expanded before a rung's offers are taken, so a node a cap hid is one this walk ` +
            `opens (the column pins ${col.expandedViews}) -- and ${column.rungs.length} answered`
          : `${unanswered.length} rung(s) the walk reached that Go does not answer: ${unanswered.join("; ") || "none"}` +
            `; ${unvisited.length} rung(s) Go answers that the walk never reached: ${unvisited.join("; ") || "none"}`,
      });
    }
  }
  return out;
}
