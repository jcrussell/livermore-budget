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
// order, and for every column Go answers -- the centre and every tier the
// step opens the node into, capped or not -- WHICH document nodes it draws as
// themselves, as a set of ids, and how many its folded tail stands for, read
// off the tail's own label, "24 smaller funds". A column Go declines to answer
// carries a reason code instead, which must name an entry of the artifact's
// reasons table: today that is the kept flank, the half of the chart on screen
// windowFor carries over, which Go's walk of the step's own document cannot
// see. The derived marks the client adds (a residual, a gap, a folded tail) are
// not Go's to count and are excluded from the id set; a derived mark competing
// with a cap is the day this goes red by design.
//
// MUTATION: rename one id, or perturb one candidates or cap figure by 1, in
// testdata/rungs.json. The Go test goes red because the artifact is no longer
// what Go computes, and this arm goes red naming the rung, the tier and the
// budget where the client drew a different column.
//
// COMPLETENESS BOTH WAYS, AT TWO GRAINS. A rung the walk visits that the
// artifact does not answer is red unless the step that opened it is one the
// artifact declares skipped, and a rung the artifact answers that the walk
// never visits is red too -- so neither side can be a silent subset of the
// other. Within a rung the same holds per column: every tier the client draws
// needs a compared entry or a Go-declared reason, and an entry for a tier the
// client does not draw is red.
import { readFileSync } from "node:fs";
import { join } from "node:path";

import { repoRoot, settle } from "./harness.mjs";
import { COLUMNS, openedWide } from "./drill.mjs";

const ARTIFACT = JSON.parse(readFileSync(join(repoRoot, "testdata", "rungs.json"), "utf8"));
if (ARTIFACT.schema_version !== 2) {
  throw new Error(`testdata/rungs.json declares schema_version ${ARTIFACT.schema_version}; this module reads 2`);
}
const SKIPPED = new Set(ARTIFACT.skipped.map((s) => s.step));
const REASONS = new Set(Object.keys(ARTIFACT.reasons || {}));
if (REASONS.size === 0) throw new Error("testdata/rungs.json declares no reasons table");

/** @param {string[]} path */
const keyOf = (path) => path.join(" > ");

/**
 * What the client drew for the rung on screen: its tiers, and for every tier
 * which document nodes it draws as themselves and how many its folded tail
 * says it stands for.
 * @param {any} app
 */
function drawn(app) {
  const nodes = app.projection.nodes;
  /** @type {Record<number, {ids: string[], hidden: number}>} */
  const byTier = {};
  for (const t of app.activeTiers()) {
    const at = nodes.filter((n) => n.tier === t);
    // A DOCUMENT NODE AND NOTHING THE CLIENT ADDED. The aggregate, residual
    // and gap tests are implied by !derived and kept because they say what is
    // being excluded; isCarried is not implied, a residual's endpoint being a
    // printed node the chart above lent this one.
    const ids = at.filter((n) => !n.derived && !app.isAggregate(n.id) && !app.isResidual(n.id) &&
      !app.isGap(n.id) && !app.isCarried(n.id)).map((n) => n.id).sort();
    const tails = at.filter((n) => app.isAggregate(n.id));
    if (tails.length > 1) throw new Error(`tier ${t} draws ${tails.length} folded tails`);
    let hidden = 0;
    if (tails.length === 1) {
      const m = /^(\d+) smaller /.exec(tails[0].label);
      if (!m) throw new Error(`the folded tail at tier ${t} is labelled ${JSON.stringify(tails[0].label)}, which does not say how many it stands for`);
      hidden = Number(m[1]);
    }
    byTier[t] = { ids, hidden };
  }
  return { tiers: app.activeTiers().slice(), byTier };
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
      let skipped = 0;
      let compared = 0;
      let engaged = 0;
      let idsCompared = 0;
      let declined = 0;
      for (const [key, { step, got }] of seen) {
        const want = expected.get(key);
        if (!want) {
          if (SKIPPED.has(step) || key.split(" > ").some((_, i, parts) => {
            const prefix = seen.get(keyOf(parts.slice(0, i + 1)));
            return prefix && SKIPPED.has(prefix.step);
          })) { skipped++; } else { unanswered.push(key); }
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
        for (const d of want.draws) {
          const mine = got.byTier[d.tier];
          if (!mine) continue; // reported above as a tier disagreement
          if (d.unanswered) {
            // A REASON GO GIVES MUST BE ONE THE ARTIFACT DEFINES, and it must
            // stand alone: an answer beside a reason would be compared or
            // not depending on which field a reader looked at first.
            if (!REASONS.has(d.unanswered)) {
              throw new Error(`${where}, tier ${d.tier}: Go declines with reason ${JSON.stringify(d.unanswered)}, which testdata/rungs.json's reasons table does not name`);
            }
            if (d.ids || d.cap || d.candidates || d.hidden) {
              throw new Error(`${where}, tier ${d.tier}: Go both declines (${d.unanswered}) and answers`);
            }
            declined++;
            continue;
          }
          // THE IDS AS A SET, which is what the column holds, and the reach
          // as a count: what the client draws plus what its tail hides is
          // how many document nodes the window reached, which is Go's
          // candidates, and the one figure of an unfolded column the id set
          // does not already say.
          idsCompared++;
          // OMITTED IS ZERO HERE AND NOWHERE ELSE: Go writes a 0 hidden or cap
          // as no field at all, and a reader that compared undefined would
          // fail every unfolded column.
          const hidden = d.hidden || 0;
          const ids = (d.ids || []).slice().sort();
          if (ids.join("\u001f") !== mine.ids.join("\u001f") || mine.hidden !== hidden) {
            const missing = ids.filter((id) => !mine.ids.includes(id));
            const extra = mine.ids.filter((id) => !ids.includes(id));
            wrong.push(`${where}, tier ${d.tier} (${d.role}${d.cap ? ` under cap ${d.cap}` : ""}): app.js draws ` +
              `${mine.ids.length} and hides ${mine.hidden}; Go says ${ids.length} drawn and ${hidden} hidden of ` +
              `${d.candidates}` + (missing.length ? `; Go draws and app.js does not: ${missing.join(", ")}` : "") +
              (extra.length ? `; app.js draws and Go does not: ${extra.join(", ")}` : ""));
          }
          if (mine.ids.length + mine.hidden !== (d.candidates || 0)) {
            wrong.push(`${where}, tier ${d.tier}: app.js reaches ${mine.ids.length + mine.hidden} document node(s) ` +
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
          // nothing here.
          if (mine.ids.length > d.cap + 1 || (mine.hidden > 0 && mine.ids.length !== d.cap)) {
            wrong.push(`${where}, tier ${d.tier}: app.js draws ${mine.ids.length} and hides ` +
              `${mine.hidden} under a cap Go declares as ${d.cap}`);
          }
        }
      }
      const unvisited = [...expected.keys()];
      const rungs = column.rungs.filter((r) => r.width === width).length;

      // THE GATES COUNT CAPPED COLUMNS AND ID SETS SEPARATELY. Counting every
      // drawn column as "compared" would turn "a cap was checked" into "a
      // column exists"; idsCompared is the gate the uncapped columns add.
      out.push({
        name: `${col.label} at ${width} columns: every column the client draws is the one Go computed`,
        ok: wrong.length === 0 && compared > 0 && engaged > 0 && idsCompared > 0,
        detail: wrong.length === 0
          ? `${idsCompared} column(s) over ${rungs} rung(s) draw the ids testdata/rungs.json says, ` +
            `${compared} of them under a cap and ${engaged} with the fold engaged; ${declined} declined with a reason`
          : `${wrong.length} disagreement(s):\n      ${wrong.join("\n      ")}`,
      });
      out.push({
        name: `${col.label} at ${width} columns: Go answers every rung the walk reaches, and no other`,
        ok: unanswered.length === 0 && unvisited.length === 0 && seen.size === col.openedViews,
        detail: unanswered.length === 0 && unvisited.length === 0
          ? `${seen.size} rung(s) visited (the column pins ${col.openedViews}), ${rungs} answered, ` +
            `${skipped} under a step the artifact declares skipped (${[...SKIPPED].join(", ") || "none"})`
          : `${unanswered.length} rung(s) the walk reached that Go does not answer: ${unanswered.join("; ") || "none"}` +
            `; ${unvisited.length} rung(s) Go answers that the walk never reached: ${unvisited.join("; ") || "none"}`,
      });
    }
  }
  return out;
}
