// The cross-language equivalence arm: for every rung the drill walks, at every
// column budget, the client's answer is held to Go's.
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
// WHAT IS COMPARED, PER RUNG AND PER BUDGET: the columns the window draws, in
// order, and for every one of them -- the kept flank, the centre and every
// tier the step opens the node into, capped or not -- WHICH document nodes it
// draws as the opened node's own parts, as a set of ids; WHICH it draws but
// does not count as one, as a second set, `carried`: a node the document
// marks derived, or one the step's residual declaration names, which is
// isCarried's rule; and how many its folded tail stands for, read off the
// tail's own label, "24 smaller funds". The carried set is compared rather
// than subtracted because on the fund-group rungs it is the flank's two spine
// feeders that the residual re-points, and on four of those rungs it is the
// flank's only mark -- an id set subtracted on both sides would compare empty
// against empty there and see nothing. A folded tail is not Go's to count
// and is excluded from both sets; one competing with a cap is the day this
// goes red by design.
//
// AND THE MARKS THE CLIENT ADDS OF ITS OWN, AS A THIRD COLLECTION: the gap
// markGap states, by id, tier and the cents that arrive at it and leave it,
// against the rung's `marks` in the artifact, complete both ways -- a mark
// Go answers that the client does not draw and one the client draws that Go
// does not answer are the same disagreement. The residual is not yet Go's
// to answer and is still excluded from every set here.
//
// MUTATION: rename one id, or perturb one candidates or cap figure by 1, in
// testdata/rungs.json. The Go test goes red because the artifact is no longer
// what Go computes, and this arm goes red naming the rung, the tier and the
// budget where the client drew a different column. Delete an id from a
// flank's carried list and it is red the same way, naming the carried mark.
// Perturb a mark's in_cents by 1, or delete the mark, and it is red naming
// the rung, the budget and the mark.
//
// COMPLETENESS BOTH WAYS, AT TWO GRAINS. A rung the walk visits that the
// artifact does not answer is red, and a rung the artifact answers that the
// walk never visits is red too -- so neither side can be a silent subset of
// the other. Within a rung the same holds per column: every tier the client
// draws needs a compared entry, and an entry for a tier the client does not
// draw is red. There is no declined column and no skipped step: Go answers
// everything the walk reaches, or the artifact does not build.
import { readFileSync } from "node:fs";
import { join } from "node:path";

import { repoRoot, settle } from "./harness.mjs";
import { COLUMNS, openedWide } from "./drill.mjs";

const ARTIFACT = JSON.parse(readFileSync(join(repoRoot, "testdata", "rungs.json"), "utf8"));
if (ARTIFACT.schema_version !== 4) {
  throw new Error(`testdata/rungs.json declares schema_version ${ARTIFACT.schema_version}; this module reads 4`);
}

/** @param {string[]} path */
const keyOf = (path) => path.join(" > ");

/**
 * The client's own marks on the chart on screen, spelled the way the
 * artifact spells them: id, role, tier, and the cents that arrive at each
 * and leave it, summed over the chart's ribbons. Every node of the chart is
 * read and not only those at an active tier, because both marks index the
 * step's declared tiers and a mark at a column the budget dropped is a
 * placement to compare, not one to overlook.
 * @param {any} app
 * @param {(id: string) => boolean} isMark
 */
function marksOn(app, isMark) {
  /** @type {{id: string, role: string, tier: number, in_cents: number, out_cents: number}[]} */
  const marks = [];
  for (const n of app.projection.nodes) {
    if (!isMark(n.id)) continue;
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
 * which document nodes it draws as the opened node's own parts, which it
 * draws but does not count as one, and how many its folded tail says it
 * stands for; and the marks the client added of its own.
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
  return { tiers: app.activeTiers().slice(), byTier, marks: marksOn(app, (id) => app.isGap(id)) };
}

/**
 * The whole tree at one budget, every offered node opened and measured,
 * reopened from the overview per offer the way drill.mjs's everyOpenedView
 * does. Written here rather than reusing that walk because this one needs the
 * key of the step each offer opens under, which the visitor there is not
 * given; visited is pinned against the column's own count so the two walks
 * cannot disagree about the tree in silence.
 * @param {any} col
 * @param {number} width
 */
async function walkAt(col, width) {
  const app = await openedWide(width, [], col);
  /** @type {Map<string, {step: string, got: ReturnType<typeof drawn>}>} */
  const seen = new Map();
  const open = async (/** @type {string} */ id) => {
    const outcome = await app.drillDown(id);
    await settle();
    if (outcome !== "drew") throw new Error(`drillDown(${id}) ${outcome}`);
  };
  const at = async (/** @type {string[]} */ path) => {
    app.drillUp(0);
    for (const id of path) await open(id);
  };
  const walk = async (/** @type {string[]} */ path) => {
    await at(path);
    const offers = app.projection.nodes.filter((n) => app.drillable(n))
      .map((n) => ({ id: n.id, step: app.stepFor(n).key }));
    for (const o of offers) {
      await at(path);
      await open(o.id);
      const next = path.concat([o.id]);
      seen.set(keyOf(next), { step: o.step, got: drawn(app) });
      await walk(next);
    }
  };
  await walk([]);
  app.drillUp(0);
  return seen;
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
      const expected = new Map(column.rungs.filter((r) => r.width === width).map((r) => [keyOf(r.path), r]));
      const seen = await walkAt(col, width);

      /** @type {string[]} */
      const wrong = [];
      /** @type {string[]} */
      const unanswered = [];
      let compared = 0;
      let engaged = 0;
      let idsCompared = 0;
      let flankCompared = 0;
      let carriedCompared = 0;
      let marksCompared = 0;
      let gapsCompared = 0;
      for (const [key, { step, got }] of seen) {
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
        // THE COLUMN ORDER IS ONE LIST ON BOTH SIDES, so a tier drawn that Go
        // does not answer, a tier Go answers that is not drawn, and the same
        // tiers in another order are all the same disagreement.
        const wantTiers = want.draws.map((/** @type {any} */ d) => d.tier);
        if (wantTiers.join(",") !== got.tiers.join(",")) {
          wrong.push(`${where} draws tiers ${got.tiers.join(",")}, Go says ${wantTiers.join(",")}`);
        }
        // THE MARKS ARE ONE LIST ON BOTH SIDES, BY ID, TIER AND CENTS, so a
        // mark Go answers that the client does not draw, one the client draws
        // that Go does not answer, and one placed or sized differently are all
        // the same disagreement. Omitted is zero here as it is for hidden.
        const wantMarks = (want.marks || []).slice().sort((/** @type {any} */ a, /** @type {any} */ b) => (a.id < b.id ? -1 : a.id > b.id ? 1 : 0));
        const wantSpelled = wantMarks.map(spellMark);
        const gotSpelled = got.marks.map(spellMark);
        if (wantSpelled.join("\u001f") !== gotSpelled.join("\u001f")) {
          const missing = wantSpelled.filter((m) => !gotSpelled.includes(m));
          const extra = gotSpelled.filter((m) => !wantSpelled.includes(m));
          wrong.push(`${where}: app.js draws ${got.marks.length} mark(s) of its own and Go answers ${wantMarks.length}` +
            (missing.length ? `; Go answers and app.js does not draw: ${missing.join("; ")}` : "") +
            (extra.length ? `; app.js draws and Go does not answer: ${extra.join("; ")}` : ""));
        }
        marksCompared += wantMarks.length;
        gapsCompared += wantMarks.filter((/** @type {any} */ m) => m.role === "gap").length;
        for (const d of want.draws) {
          const mine = got.byTier[d.tier];
          if (!mine) continue; // reported above as a tier disagreement
          // IDS IS A LIST OR THE ENTRY IS MALFORMED: Go writes it even when
          // empty, so that a flank answered with nothing is not read as a
          // column left out.
          if (!Array.isArray(d.ids)) {
            throw new Error(`${where}, tier ${d.tier}: Go's entry carries no ids list`);
          }
          // THE IDS AS A SET, which is what the column holds; the carried
          // marks as a second set; and the reach as a count: what the client
          // draws, counted or carried, plus what its tail hides is how many
          // document nodes the window reached, which is Go's candidates, and
          // the one figure of an unfolded column the two sets do not already
          // say.
          idsCompared++;
          if (d.role === "flank") flankCompared++;
          // OMITTED IS ZERO HERE AND NOWHERE ELSE: Go writes a 0 hidden or cap
          // and an empty carried as no field at all, and a reader that
          // compared undefined would fail every unfolded column.
          const hidden = d.hidden || 0;
          const ids = d.ids.slice().sort();
          const carried = (d.carried || []).slice().sort();
          if (carried.length) carriedCompared++;
          if (ids.join("\u001f") !== mine.ids.join("\u001f") || mine.hidden !== hidden) {
            const missing = ids.filter((id) => !mine.ids.includes(id));
            const extra = mine.ids.filter((id) => !ids.includes(id));
            wrong.push(`${where}, tier ${d.tier} (${d.role}${d.cap ? ` under cap ${d.cap}` : ""}): app.js draws ` +
              `${mine.ids.length} and hides ${mine.hidden}; Go says ${ids.length} drawn and ${hidden} hidden of ` +
              `${d.candidates}` + (missing.length ? `; Go draws and app.js does not: ${missing.join(", ")}` : "") +
              (extra.length ? `; app.js draws and Go does not: ${extra.join(", ")}` : ""));
          }
          if (carried.join("\u001f") !== mine.carried.join("\u001f")) {
            const missing = carried.filter((id) => !mine.carried.includes(id));
            const extra = mine.carried.filter((id) => !carried.includes(id));
            wrong.push(`${where}, tier ${d.tier} (${d.role}): app.js carries ${mine.carried.length} mark(s) it does ` +
              `not count and Go lists ${carried.length}` +
              (missing.length ? `; Go lists and app.js does not carry: ${missing.join(", ")}` : "") +
              (extra.length ? `; app.js carries and Go does not list: ${extra.join(", ")}` : ""));
          }
          const reached = mine.ids.length + mine.carried.length + mine.hidden;
          if (reached !== (d.candidates || 0)) {
            wrong.push(`${where}, tier ${d.tier}: app.js reaches ${reached} document node(s) ` +
              `and Go says ${d.candidates || 0}`);
          }
          if (!(d.cap > 0)) continue;
          compared++;
          if (hidden > 0) engaged++;
          // THE CAP ITSELF, NOT ONLY WHAT GO SAYS IT DID. A column the client
          // folded is drawn at exactly its cap, and an unfolded one holds at
          // most cap+1 -- capColumn's own threshold, a tail of one being no
          // fold. Without this, a cap perturbed alone in the artifact, ids
          // and hidden left as Go computed them, is seen by the Go test and by
          // nothing here. The cap ranks counted and carried alike, because
          // capColumn runs before either is told apart.
          const drawnHere = mine.ids.length + mine.carried.length;
          if (drawnHere > d.cap + 1 || (mine.hidden > 0 && drawnHere !== d.cap)) {
            wrong.push(`${where}, tier ${d.tier}: app.js draws ${drawnHere} and hides ` +
              `${mine.hidden} under a cap Go declares as ${d.cap}`);
          }
        }
      }
      const unvisited = [...expected.keys()];
      const rungs = column.rungs.filter((r) => r.width === width).length;

      // THE GATES COUNT CAPPED COLUMNS, ID SETS, FLANKS, CARRIED MARKS AND
      // THE CLIENT'S OWN MARKS SEPARATELY. Counting every drawn column as
      // "compared" would turn "a cap was checked" into "a column exists";
      // idsCompared is the gate the uncapped columns add, flankCompared the
      // one the kept flank adds, carriedCompared the one a residual's
      // endpoint on a flank adds, and marksCompared the one the client's own
      // marks add -- each a comparison that could vanish with the others
      // still green.
      //
      // THE GAP HAS ITS OWN COUNTER, HELD TO THE COLUMN'S PIN, because it is
      // drawn on one year only: FY2026-27's services-and-supplies is the one
      // cell the two schedules print apart, and folding the gap into
      // marksCompared would let that one path vanish while a residual kept
      // the counter green. drill.mjs pins which column draws one as
      // gapCents, and a column pinned to draw none must compare none.
      const drawsAGap = Boolean(col.object.gapCents);
      out.push({
        name: `${col.label} at ${width} columns: every column the client draws is the one Go computed`,
        ok: wrong.length === 0 && compared > 0 && engaged > 0 && idsCompared > 0 && flankCompared > 0 &&
          carriedCompared > 0 && marksCompared > 0 === drawsAGap && gapsCompared > 0 === drawsAGap,
        detail: wrong.length === 0
          ? `${idsCompared} column(s) over ${rungs} rung(s) draw the ids testdata/rungs.json says, ` +
            `${compared} of them under a cap and ${engaged} with the fold engaged; ${flankCompared} kept flank(s) ` +
            `among them, ${carriedCompared} column(s) carrying a mark the client does not count; ` +
            `${marksCompared} mark(s) of the client's own compared, ${gapsCompared} of them a gap ` +
            `(the column ${drawsAGap ? "draws one" : "draws none"})`
          : `${wrong.length} disagreement(s):\n      ${wrong.join("\n      ")}`,
      });
      out.push({
        name: `${col.label} at ${width} columns: Go answers every rung the walk reaches, and no other`,
        ok: unanswered.length === 0 && unvisited.length === 0 && seen.size === col.openedViews,
        detail: unanswered.length === 0 && unvisited.length === 0
          ? `${seen.size} rung(s) visited (the column pins ${col.openedViews}), ${rungs} answered`
          : `${unanswered.length} rung(s) the walk reached that Go does not answer: ${unanswered.join("; ") || "none"}` +
            `; ${unvisited.length} rung(s) Go answers that the walk never reached: ${unvisited.join("; ") || "none"}`,
      });
    }
  }
  return out;
}
