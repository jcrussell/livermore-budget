// What site/format.js does for every page module that imports it: dollars,
// shares, a page's config and its hash's fields. Imported with nothing on
// the global, so a module that read one or held state at import fails first.
import { test } from "node:test";
import assert from "node:assert/strict";
import path from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";
import { JSDOM } from "jsdom";

const HERE = path.dirname(fileURLToPath(import.meta.url));
const MODULE = pathToFileURL(path.join(HERE, "format.js")).href;

test("format.js imports with no DOM and no global, and exports only functions", async () => {
  delete globalThis.FISC_CONFIG;
  delete globalThis.d3;
  assert.equal(typeof globalThis.document, "undefined", "nothing installed a DOM before the import");
  const mod = await import(MODULE);
  assert.deepEqual(Object.keys(mod).sort(), ["configOf", "dollars", "hashFields", "hashValue", "share"]);
  assert.ok(Object.values(mod).every((v) => typeof v === "function"));
});

// Mutation: drop the sign (always "$") and the negative cases go red; print
// two places always and the whole-dollar cases do.
test("dollars signs, groups and prints cents only where a figure has them", async () => {
  const { dollars } = await import(MODULE);
  const cases = [
    [0, "$0"],
    [-0, "$0"],
    [1, "$0.01"],
    [-5, "-$0.05"],
    [99, "$0.99"],
    [100, "$1"],
    [-100, "-$1"],
    [105, "$1.05"],
    [150, "$1.50"],
    [-150, "-$1.50"],
    [123456, "$1,234.56"],
    [7076545000, "$70,765,450"],
    [25409541200, "$254,095,412"],
    [-126150980, "-$1,261,509.80"],
    // Past every sum a page draws, and still to the cent: the float divide
    // by 100 is within half a cent below 2^52 cents.
    [2 ** 52 - 1, "$45,035,996,273,704.95"],
  ];
  assert.deepEqual(cases.map(([c]) => dollars(c)), cases.map(([, want]) => want));
});

// Mutation: drop the zero guard and share(1, 0) reads "Infinity%".
test("share is a signed percentage to a tenth, and nothing of a whole of nothing", async () => {
  const { share } = await import(MODULE);
  assert.equal(share(1, 3), "33.3%");
  assert.equal(share(2, 3), "66.7%");
  assert.equal(share(3, 3), "100.0%");
  assert.equal(share(-1, 4), "-25.0%");
  assert.equal(share(1, 10000), "0.0%");
  assert.equal(share(1, 0), "");
  assert.equal(share(0, 0), "");
});

test("configOf parses the named script element, and refuses a page without one", async () => {
  const { configOf } = await import(MODULE);
  const doc = new JSDOM('<script type="application/json" id="x-config">{"years":[1]}</script>').window.document;
  assert.deepEqual(configOf(doc, "x-config", "x"), { years: [1] });
  assert.throws(() => configOf(doc, "y-config", "y"), /the page carries no y config/);
});

// Mutation: escape the slash too and the id round trip below reads %2F.
test("hashValue escapes everything but the slashes ids hold", async () => {
  const { hashValue } = await import(MODULE);
  assert.equal(hashValue("department/public-works"), "department/public-works");
  assert.equal(hashValue("a&b=c,d~e #f"), "a%26b%3Dc%2Cd~e%20%23f");
});

// Mutation: drop the catch and "%E0" throws; drop the names test and "z"
// is read.
test("hashFields reads the named fields, decoded, and skips the rest", async () => {
  const { hashFields, hashValue } = await import(MODULE);
  const read = (hash) => Object.fromEntries(hashFields(hash, ["y", "d"]));
  assert.deepEqual(read("#y=fy2026-adopted&d=department/public-works"), { y: "fy2026-adopted", d: "department/public-works" });
  assert.deepEqual(read("y=a"), { y: "a" }, "the # is optional");
  assert.deepEqual(read("#z=1&y=a&bare&=2"), { y: "a" }, "an unnamed field, a bare word and an empty name are skipped");
  assert.deepEqual(read("#y=a&y=b"), { y: "b" }, "a field written twice keeps its last value");
  assert.deepEqual(read("#y=%E0%A4%A&d=a%20b"), { y: "%E0%A4%A", d: "a b" }, "a value that does not decode is kept raw");
  assert.deepEqual(read("#y=a=b"), { y: "a=b" }, "a value may hold =");
  assert.deepEqual(read("#y="), { y: "" });
  assert.deepEqual(read(""), {});
  assert.deepEqual(read(/** @type {any} */ (undefined)), {});
  const id = "a&b/c d";
  assert.deepEqual(read("#d=" + hashValue(id)), { d: id });
});
