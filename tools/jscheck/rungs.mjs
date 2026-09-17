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
// WHAT IS COMPARED, PER RUNG AND PER BUDGET: the tiers the window draws, and
// for every cap the step declares on a drawn tier, how many document nodes the
// column draws as themselves and how many its folded tail stands for -- read
// off the tail's own label, "24 smaller funds". The uncapped columns and the
// derived marks the client adds (a residual, a gap, a carried endpoint) are not
// Go's to count and are not compared; a derived mark competing with a cap is
// the day this goes red by design.
//
// MUTATION: perturb one cap in testdata/rungs.json by 1. The Go test goes red
// because the artifact is no longer what Go computes, and this arm goes red
// naming the rung and the budget where the client drew a different column.
//
// COMPLETENESS BOTH WAYS. A rung the walk visits that the artifact does not
// answer is red unless the step that opened it is one the artifact declares
// skipped, and a rung the artifact answers that the walk never visits is red
// too -- so neither side can be a silent subset of the other.
import { readFileSync } from "node:fs";
import { join } from "node:path";

import { repoRoot, settle } from "./harness.mjs";
import { COLUMNS, openedWide } from "./drill.mjs";

const ARTIFACT = JSON.parse(readFileSync(join(repoRoot, "testdata", "rungs.json"), "utf8"));
if (ARTIFACT.schema_version !== 1) {
  throw new Error(`testdata/rungs.json declares schema_version ${ARTIFACT.schema_version}; this module reads 1`);
}
const SKIPPED = new Set(ARTIFACT.skipped.map((s) => s.step));

/** @param {string[]} path */
const keyOf = (path) => path.join(" > ");

/**
 * What the client drew for the rung on screen: its tiers, and for every tier
 * how many document nodes it draws as themselves and how many its folded tail
 * says it stands for.
 * @param {any} app
 */
function drawn(app) {
  const nodes = app.projection.nodes;
  /** @type {Record<number, {real: number, hidden: number}>} */
  const byTier = {};
  for (const t of app.activeTiers()) {
    const at = nodes.filter((n) => n.tier === t);
    const real = at.filter((n) => !n.derived && !app.isAggregate(n.id) && !app.isResidual(n.id) &&
      !app.isGap(n.id) && !app.isCarried(n.id)).length;
    const tails = at.filter((n) => app.isAggregate(n.id));
    if (tails.length > 1) throw new Error(`tier ${t} draws ${tails.length} folded tails`);
    let hidden = 0;
    if (tails.length === 1) {
      const m = /^(\d+) smaller /.exec(tails[0].label);
      if (!m) throw new Error(`the folded tail at tier ${t} is labelled ${JSON.stringify(tails[0].label)}, which does not say how many it stands for`);
      hidden = Number(m[1]);
    }
    byTier[t] = { real, hidden };
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
        if (want.step !== step) {
          wrong.push(`${key} at ${width} columns opened under step ${step}, Go says ${want.step}`);
        }
        if (want.tiers.join(",") !== got.tiers.join(",")) {
          wrong.push(`${key} at ${width} columns draws tiers ${got.tiers.join(",")}, Go says ${want.tiers.join(",")}`);
        }
        for (const cap of want.caps) {
          compared++;
          if (cap.hidden > 0) engaged++;
          const mine = got.byTier[cap.tier];
          if (!mine) {
            wrong.push(`${key} at ${width} columns: tier ${cap.tier} is capped at ${cap.cap} and not drawn`);
            continue;
          }
          if (mine.real !== cap.drawn || mine.hidden !== cap.hidden) {
            wrong.push(`${key} at ${width} columns, tier ${cap.tier} under cap ${cap.cap}: app.js draws ` +
              `${mine.real} and hides ${mine.hidden}; Go says ${cap.drawn} drawn and ${cap.hidden} hidden of ${cap.candidates}`);
          }
          // THE CAP ITSELF, NOT ONLY WHAT GO SAYS IT DID. A column the client
          // folded is drawn at exactly its cap, and an unfolded one holds at
          // most cap+1 -- capColumn's own threshold, a tail of one being no
          // fold. Without this, a cap perturbed alone in the artifact, drawn
          // and hidden left as Go computed them, is seen by the Go test and by
          // nothing here.
          if (mine.real > cap.cap + 1 || (mine.hidden > 0 && mine.real !== cap.cap)) {
            wrong.push(`${key} at ${width} columns, tier ${cap.tier}: app.js draws ${mine.real} and hides ` +
              `${mine.hidden} under a cap Go declares as ${cap.cap}`);
          }
        }
      }
      const unvisited = [...expected.keys()];
      const rungs = column.rungs.filter((r) => r.width === width).length;

      out.push({
        name: `${col.label} at ${width} columns: every capped column the client draws is the one Go computed`,
        ok: wrong.length === 0 && compared > 0 && engaged > 0,
        detail: wrong.length === 0
          ? `${compared} capped column(s) over ${rungs} rung(s) agree with testdata/rungs.json, ` +
            `${engaged} of them with the fold engaged`
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
