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

test("format.js imports with no DOM and no global, and exports only functions and the tail's two constants", async () => {
  delete globalThis.FISC_CONFIG;
  delete globalThis.d3;
  assert.equal(typeof globalThis.document, "undefined", "nothing installed a DOM before the import");
  const mod = await import(MODULE);
  assert.deepEqual(Object.keys(mod).sort(), ["NAMED", "TAIL_LABEL", "carryYear", "citedPages", "clone", "configOf", "dollars", "hashFields", "hashValue", "hiddenEdges", "linkedPage", "pageLinks", "pageOf", "pinnable", "placeTip", "plus", "refuseFigure", "rowCents", "sayRefusal", "scrollCues", "selectYear", "share", "tipHint", "tipper", "wireYearSelect", "yearKey"]);
  assert.ok(Object.entries(mod).every(([k, v]) => typeof v === "function" || k === "NAMED" || k === "TAIL_LABEL"));
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

// Mutation: drop the zero guard and share(1, 0) reads "Infinity%"; drop the
// ceiling and share(9995, 10000) reads "100.0%".
test("share is a percentage to a tenth that never rounds a part to nothing or to the whole", async () => {
  const { share } = await import(MODULE);
  assert.equal(share(1, 3), "33.3%");
  assert.equal(share(2, 3), "66.7%");
  assert.equal(share(3, 3), "100.0%");
  assert.equal(share(-1, 4), "25.0%");
  assert.equal(share(0, 4), "0.0%");
  assert.equal(share(1, 10000), "<0.1%");
  assert.equal(share(9995, 10000), ">99.9%");
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

// A cited page the config built no links for is named, marked unlinked, and
// does not throw out of the hover, focus or click handler filling the tip.
// Two documents' unlinked pages of one number are two pages.
// Mutation: read each page through pageOf in citedPages and the call
// throws; drop the data-unlinked mark and the unlinked arm goes red; key an
// unlinked page by its number alone and gone's p66 folds into d's.
test("a tip names a cited page the config publishes no links for, unlinked, beside the linked ones", async () => {
  const { citedPages, pageLinks } = await import(MODULE);
  const doc = new JSDOM("").window.document;
  const docs = { d: { pages: { "66": { pdf: "https://x/d.pdf#page=66", text: "extracted/d/pages/p0066.txt", records: "" } } } };
  const pages = citedPages(docs, [{ doc_id: "d", pages: [66, 67] }, { doc_id: "gone", pages: [5, 67, 67] }]);
  assert.deepEqual(pages, [
    { doc: "d", page: 66, text: "extracted/d/pages/p0066.txt", pdf: "https://x/d.pdf#page=66" },
    { doc: "d", page: 67, text: "", pdf: "" },
    { doc: "gone", page: 5, text: "", pdf: "" },
    { doc: "gone", page: 67, text: "", pdf: "" },
    { doc: "gone", page: 67, text: "", pdf: "" },
  ]);
  const line = pageLinks(doc, pages);
  assert.equal(line.textContent, "p66 PDF, p67, p5, p67");
  assert.deepEqual([...line.querySelectorAll("a")].map((a) => a.getAttribute("href")),
    ["extracted/d/pages/p0066.txt", "https://x/d.pdf#page=66"]);
  assert.deepEqual([...line.querySelectorAll("[data-unlinked]")].map((x) => x.textContent), ["p67", "p5", "p67"]);
});

// A table's own page link is read as the page it names, its text and its
// PDF, and a link carrying no PDF reads "" for it.
// Mutation: read the PDF from href in linkedPage -- the first arm goes red;
// drop its `|| ""` -- the second does.
test("linkedPage reads the page, text and PDF a table's page link names", async () => {
  const { linkedPage } = await import(MODULE);
  const doc = new JSDOM(`<a data-page="97" href="extracted/d/pages/p0097.txt" data-pdf="d.pdf#page=97">p97</a><a data-page="98" href="extracted/d/pages/p0098.txt">p98</a>`).window.document;
  const [both, bare] = Array.from(doc.querySelectorAll("a[data-page]")).map(linkedPage);
  assert.deepEqual(both, { page: 97, text: "extracted/d/pages/p0097.txt", pdf: "d.pdf#page=97" });
  assert.deepEqual(bare, { page: 98, text: "extracted/d/pages/p0098.txt", pdf: "" });
});

// A sideways scroller says which edges hide more, kept current as it scrolls.
// jsdom lays nothing out, so the element's widths are stubbed. Mutation: drop
// the scroll listener in scrollCues and the second read stays "right".
test("hiddenEdges and scrollCues mark the edges a scroller hides", async () => {
  const mod = await import(MODULE);
  const at = (scrollLeft) => ({ scrollLeft, scrollWidth: 1320, clientWidth: 1176 });
  assert.deepEqual([at(0), at(70), at(144), { scrollLeft: 0, scrollWidth: 500, clientWidth: 500 }].map(mod.hiddenEdges),
    ["right", "both", "left", ""]);
  const { window } = new JSDOM('<div class="table-scroll"><table></table></div><div class="other"></div>');
  const box = window.document.querySelector(".table-scroll");
  let left = 0;
  Object.defineProperty(box, "scrollWidth", { value: 1320 });
  Object.defineProperty(box, "clientWidth", { value: 1176 });
  Object.defineProperty(box, "scrollLeft", { get: () => left });
  mod.scrollCues(window.document);
  assert.equal(box.dataset.more, "right");
  left = 144;
  box.dispatchEvent(new window.Event("scroll"));
  assert.equal(box.dataset.more, "left");
  assert.equal(window.document.querySelector(".other").dataset.more, undefined);
});

// The cue follows the content as well as the box: a table narrowed under a
// box of the same size moves no edge of the box. Mutation: drop the
// firstElementChild observe in scrollCues, and the table is not observed.
test("scrollCues observes each scroller's content as well as the scroller", async () => {
  const mod = await import(MODULE);
  const { window } = new JSDOM('<div class="table-scroll"><table></table></div>');
  const observed = [];
  window.ResizeObserver = class { observe(el) { observed.push(el); } };
  mod.scrollCues(window.document);
  const box = window.document.querySelector(".table-scroll");
  assert.ok(observed.includes(box) && observed.includes(box.querySelector("table")), observed.map((e) => e.tagName).join());
});

// A message's own closing period is not doubled by the sentence around it.
// Mutation: drop the trailing-period replace in refuseFigure, and it reads
// "data..".
test("refuseFigure states a message ending in a period once", async () => {
  const { refuseFigure } = await import(MODULE);
  const { window } = new JSDOM("<section><h2>x</h2></section>");
  const section = window.document.querySelector("section");
  refuseFigure(section, new Error("no such data."));
  assert.equal(section.querySelector("p.refusal").textContent,
    "This figure could not be drawn: no such data. The table below is what the page carries.");
});

// One clone for every page module. Mutation: return the template itself
// rather than a copy and the second clone is the first.
test("clone copies a template's first element, and refuses a missing or empty template", async () => {
  const { clone } = await import(MODULE);
  const { window } = new JSDOM(`<template id="t"><li class="x">a</li></template><template id="empty"></template><div id="d"></div>`);
  const doc = window.document;
  const a = clone(doc, "t");
  const b = clone(doc, "t");
  assert.equal(a.className, "x");
  assert.ok(a !== b && !a.isConnected, "clone handed back the template's node, not a copy");
  assert.throws(() => clone(doc, "nope"), /no <template id="nope">/);
  assert.throws(() => clone(doc, "d"), /no <template id="d">/);
  assert.throws(() => clone(doc, "empty"), /is empty/);
});

// One refusal per page, replaced rather than stacked, and cleared by an empty
// sentence. Mutation: drop the getElementById reuse and a second say stacks.
test("sayRefusal prepends one alert to the page's main, replaces its words, and clears it", async () => {
  const { sayRefusal } = await import(MODULE);
  const { window } = new JSDOM(`<main id="sides-main"><p>chart</p></main>`);
  const doc = window.document;
  sayRefusal(doc, "sides", "first");
  sayRefusal(doc, "sides", "second");
  const alerts = doc.querySelectorAll("#sides-main > .refusal[role=alert]");
  assert.equal(alerts.length, 1);
  assert.equal(alerts[0].textContent, "second");
  assert.equal(doc.getElementById("sides-main").firstElementChild.id, "sides-refusal");
  sayRefusal(doc, "sides", "");
  assert.equal(doc.getElementById("sides-refusal"), null);
});

// One reader for every page that sums Go's table. Mutation: read the cell
// with Number(raw) and an empty data-cents becomes a printed 0 rather than a
// refusal.
test("rowCents reads a row's figures, absent where none is printed, and refuses an empty or fractional one", async () => {
  const { rowCents, plus } = await import(MODULE);
  const { window } = new JSDOM(`<table>
    <tr id="ok"><td data-column="0" data-cents="-150"></td><td data-column="2" data-cents="0"></td><td data-column="1"></td></tr>
    <tr id="empty"><td data-column="0" data-cents=""></td></tr>
    <tr id="frac"><td data-column="0" data-cents="1.5"></td></tr>
    <tr id="out"><td data-column="3" data-cents="1"></td></tr></table>`);
  const row = (id) => window.document.getElementById(id);
  assert.deepEqual(rowCents(row("ok"), 3, "ok"), [-150, null, 0]);
  assert.throws(() => rowCents(row("empty"), 3, "empty"), /empty carries "" cents/);
  assert.throws(() => rowCents(row("frac"), 3, "frac"), /frac carries "1.5" cents/);
  assert.throws(() => rowCents(row("out"), 3, "out"), /in column 3/);
  assert.equal(plus(null, null), null);
  assert.equal(plus(null, 0), 0);
  assert.equal(plus(5, null), 5);
  assert.equal(plus(5, -2), 3);
});
