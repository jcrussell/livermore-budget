// run.mjs — `make js`.
//
// Runs every check over site/app.js and exits non-zero on the first that fails.
// Node and the standard library only: no npm, no package.json, no node_modules,
// and nothing here is served to a reader.

import { checks as layoutChecks } from "./layout.mjs";
import { checks as yearChecks } from "./year.mjs";

let failed = 0;
for (const c of [...layoutChecks(), ...yearChecks()]) {
  const status = c.ok ? "PASS" : "FAIL";
  if (!c.ok) failed++;
  console.log(`${status}  ${c.name}`);
  console.log(`      ${c.detail}`);
}
console.log(failed === 0
  ? `\nsite/app.js: every published layout claim holds.`
  : `\nsite/app.js: ${failed} claim${failed === 1 ? "" : "s"} the page makes about itself no longer hold.`);
process.exit(failed === 0 ? 0 : 1);
