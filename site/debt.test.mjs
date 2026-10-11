// What site/debt.js does for debt.html. The module is imported with nothing on
// the global, as modules.test.mjs imports core.js, so a module that read a
// global or held state at import would fail here first.
import { test } from "node:test";
import assert from "node:assert/strict";
import path from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const HERE = path.dirname(fileURLToPath(import.meta.url));

test("debt.js imports with no page config, no d3 and no DOM, and exports nothing yet", async () => {
  delete globalThis.FISC_CONFIG;
  delete globalThis.d3;
  assert.equal(typeof globalThis.document, "undefined", "nothing installed a DOM before the import");
  const mod = await import(pathToFileURL(path.join(HERE, "debt.js")).href);
  assert.deepEqual(Object.keys(mod), []);
});
