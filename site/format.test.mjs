// What site/format.js does for every page module that imports it: dollars,
// shares, a page's config, its hash's fields and the top bar's year select. Imported with nothing on
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
  assert.deepEqual(Object.keys(mod).sort(), ["carryYear", "configOf", "dollars", "hashFields", "hashValue", "pageLinks", "pageOf", "pageTexts", "pinnable", "placeTip", "selectYear", "share", "tipHint", "tipper", "wireYearSelect", "yearKey"]);
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
    [2 ** 52 - 1, "$45,035,996,273,704.95"],
    // Past 2^53/100 dollars a float divide by 100 misprints the cents
    // (.90, .88, .84); the integer split prints them as cited.
    [2 ** 53 - 1, "$90,071,992,547,409.91"],
    [2 ** 53 - 5, "$90,071,992,547,409.87"],
    [-(2 ** 53 - 7), "-$90,071,992,547,409.85"],
  ];
  assert.deepEqual(cases.map(([c]) => dollars(c)), cases.map(([, want]) => want));
});

// Mutation: drop the safe-integer guard and dollars(1.5) prints a figure
// no page cites.
test("dollars refuses anything but a safe integer of cents", async () => {
  const { dollars } = await import(MODULE);
  for (const bad of [1.5, 0.1 + 0.2, 2 ** 53, NaN, Infinity, "100", undefined]) {
    assert.throws(() => dollars(/** @type {any} */ (bad)), /not a safe integer of cents/, String(bad));
  }
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

// Mutation: drop `offers` from carryYear's test and the link to a view that
// does not publish the year is handed it; drop the strip of the old fragment
// and a second year appends a second `#y=`.
test("the top bar's year select is enabled, says the year, and carries it only to the views that offer it", async () => {
  const { wireYearSelect, selectYear, yearKey } = await import(MODULE);
  const { window } = new JSDOM(`<nav class="site-nav"><ul>
    <li><a href="flows.html" data-years="fy2026-adopted fy2027-adopted">Flows</a></li>
    <li><a href="department.html" data-years="fy2024-actual fy2026-adopted">Departments</a></li>
    <li><a href="change.html">Over time</a></li></ul>
    <select id="year-select" disabled><option value="fy2024-actual">a</option><option value="fy2026-adopted" selected>b</option></select></nav>`);
  const doc = window.document;
  const href = (p) => doc.querySelector(`a[href^="${p}"]`).getAttribute("href");
  const chosen = [];
  const restored = wireYearSelect(doc, (k) => chosen.push(k));
  const select = doc.getElementById("year-select");
  assert.deepEqual({ restored, disabled: select.disabled }, { restored: "fy2026-adopted", disabled: false });
  select.value = "fy2024-actual";
  select.dispatchEvent(new window.Event("change"));
  assert.deepEqual(chosen, ["fy2024-actual"]);
  selectYear(doc, "fy2026-adopted");
  selectYear(doc, "fy2024-actual");
  assert.deepEqual([select.value, href("flows"), href("department"), href("change")],
    ["fy2024-actual", "flows.html", "department.html#y=fy2024-actual", "change.html"]);
  selectYear(doc, "");
  assert.equal(href("department"), "department.html");
  assert.equal(yearKey("fy2026-adopted.json"), "fy2026-adopted");
  assert.equal(wireYearSelect(new JSDOM("").window.document, () => {}), undefined);
});
