// run.mjs — `make js`.
//
// Runs every check over site/app.js and exits non-zero if any fails. Node and
// the standard library only: no npm, no package.json, no node_modules, and
// nothing here is served to a reader.
//
// IT AWAITS, AND THAT IS LOAD-BEARING RATHER THAN COSMETIC. Until 2026-08-26
// this file was fully synchronous: a check's `ok` was an already-evaluated
// boolean, so a Promise assigned to it was TRUTHY and reported PASS whatever it
// resolved to. Every check about app.js's fetch path is inherently async, so
// under the old runner the first one written would have been a check that could
// not fail — the defect this whole directory exists to catch, in the runner
// itself. See fisc-dn9.
//
// A check that THROWS is a FAILED check and not a crashed run. An async check
// drives real app.js code, so it can reject for the same reasons the page can,
// and a rejection that took the process down would report nothing about the
// other checks and read as a broken harness rather than a broken page.

import { settleCheck } from "./harness.mjs";
import { checks as layoutChecks } from "./layout.mjs";
import { checks as yearChecks } from "./year.mjs";
import { checks as seamChecks } from "./seam.mjs";
import { checks as lifecycleChecks } from "./lifecycle.mjs";

let failed = 0;
// Sequentially, not Promise.all: each module loads app.js into its own vm
// context and the output is meant to read in a fixed order.
for (const module of [layoutChecks, yearChecks, seamChecks, lifecycleChecks]) {
  let produced;
  try {
    produced = await module();
  } catch (e) {
    failed++;
    console.log("FAIL  a whole check module threw before producing any check");
    console.log(`      ${e && e.stack ? e.stack : String(e)}`);
    continue;
  }
  for (const raw of produced) {
    const c = await settleCheck(raw);
    if (!c.ok) failed++;
    console.log(`${c.ok ? "PASS" : "FAIL"}  ${c.name}`);
    console.log(`      ${c.detail}`);
  }
}
console.log(failed === 0
  ? `\nsite/app.js: every published layout claim holds.`
  : `\nsite/app.js: ${failed} claim${failed === 1 ? "" : "s"} the page makes about itself no longer hold.`);
process.exit(failed === 0 ? 0 : 1);
