// What site/capital.js does for capital.html, over testdata/capital.golden.html:
// the page Go writes over the client subset, which keeps the whole project
// listing. Every figure here is one Go cited; what is tested is the client's
// filtering and its sum of the rows left shown.
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";
import { JSDOM } from "jsdom";

const HERE = path.dirname(fileURLToPath(import.meta.url));
const MODULE = pathToFileURL(path.join(HERE, "capital.js")).href;
const PAGE = readFileSync(path.join(HERE, "..", "testdata", "capital.golden.html"), "utf8");

/** A fresh document over the pinned page, booted. */
async function booted() {
  const dom = new JSDOM(PAGE);
  const mod = await import(MODULE);
  const doc = dom.window.document;
  mod.boot(doc);
  return { mod, doc };
}

/** Sets a control's value and fires the event capital.js listens for. */
function set(doc, id, value, event) {
  const el = doc.getElementById(id);
  el.value = value;
  el.dispatchEvent(new doc.defaultView.Event(event));
}

/** The body rows. */
function bodyRows(doc) {
  return [...doc.querySelectorAll("#capital-table tbody tr")];
}

/** The cents of the rows the page shows, read off the DOM, per column. */
function visibleSums(doc) {
  const sums = new Map();
  for (const tr of bodyRows(doc)) {
    if (tr.hidden) continue;
    for (const td of tr.querySelectorAll("td[data-cents]")) {
      const c = td.getAttribute("data-column");
      sums.set(c, (sums.get(c) ?? 0) + Number(td.getAttribute("data-cents")));
    }
  }
  return sums;
}

/** The footer's text per column. */
function footer(doc) {
  const out = new Map();
  for (const td of doc.querySelectorAll("#capital-table tfoot td[data-column]")) {
    out.set(td.getAttribute("data-column"), td.textContent);
  }
  return out;
}

test("capital.js imports with no page config, no d3 and no DOM, and runs nothing", async () => {
  delete globalThis.FISC_CONFIG;
  delete globalThis.d3;
  assert.equal(typeof globalThis.document, "undefined", "nothing installed a DOM before the import");
  const mod = await import(MODULE);
  assert.deepEqual(Object.keys(mod).sort(), ["apply", "boot", "formatCents", "isShown", "readTable", "sumShown"]);
});

test("boot offers the controls and labels the footer as our sum, never as a Total", async () => {
  const { doc } = await booted();
  assert.equal(doc.getElementById("capital-filters").hidden, false);
  const rows = bodyRows(doc).length;
  assert.ok(rows > 0, "the fixture carries no project row");
  assert.equal(doc.getElementById("capital-shown").textContent,
    `◇ ${rows} of ${rows} project lines shown, by our count`);
  const foot = doc.querySelector("#capital-table tfoot");
  assert.equal(foot.hidden, false);
  const label = foot.querySelector("th").textContent;
  assert.ok(label.includes("◇") && label.includes("sum of the rows shown"), label);
  assert.ok(!/\btotal\b/i.test(label), label);
});

// Mutation: make sumShown add every row rather than the shown ones, and the
// fund-filtered footer no longer equals the visible rows' cents.
test("the footer is the sum of the rows shown, whatever the fund", async () => {
  const { mod, doc } = await booted();
  const funds = [...doc.querySelectorAll("#capital-fund option")].map((o) => o.value);
  for (const fund of funds) {
    set(doc, "capital-fund", fund, "change");
    const want = visibleSums(doc);
    for (const [col, text] of footer(doc)) {
      const cents = want.get(col);
      assert.equal(text, cents === undefined ? "—" : mod.formatCents(cents),
        `fund ${fund || "every"} column ${col}`);
    }
  }
});

test("the funds partition the rows, and their sums add up to every fund's", async (t) => {
  const { mod, doc } = await booted();
  const { rows, columns } = mod.readTable(doc.getElementById("capital-table"));
  const all = { fund: "", column: "", query: "" };
  const whole = mod.sumShown(rows, columns, all).map((s) => s ?? 0);
  const funds = [...doc.querySelectorAll("#capital-fund option")].map((o) => o.value).filter((v) => v !== "");
  const seen = new Array(rows.length).fill(0);
  const added = new Array(columns).fill(0);
  for (const fund of funds) {
    const f = { ...all, fund };
    rows.forEach((r, i) => {
      if (mod.isShown(r, f)) seen[i]++;
    });
    mod.sumShown(rows, columns, f).forEach((s, i) => {
      added[i] += s ?? 0;
    });
  }
  const lost = seen.findIndex((n) => n !== 1);
  assert.ok(lost < 0, lost < 0 ? "" : `row ${lost} (fund ${rows[lost].fund}) is under ${seen[lost]} funds`);
  assert.deepEqual(added, whole);
  t.diagnostic(`${funds.length} fund options partition ${rows.length} project lines`);
});

test("search narrows to the names that contain it, ignoring case and edge spaces", async () => {
  const { doc } = await booted();
  set(doc, "capital-search", "  STREET resurfacing ", "input");
  const has = (tr) => tr.querySelector("th").textContent.toLowerCase().includes("street resurfacing");
  const shown = bodyRows(doc).filter((tr) => !tr.hidden);
  assert.ok(shown.length > 0, "no project matches street resurfacing");
  assert.ok(shown.every(has), "a row not naming street resurfacing is shown");
  assert.ok(bodyRows(doc).filter((tr) => tr.hidden).every((tr) => !has(tr)), "a matching row is hidden");
  set(doc, "capital-search", "", "input");
  assert.ok(bodyRows(doc).every((tr) => !tr.hidden), "clearing the search left a row hidden");
});

// Mutation: narrow apply's column hiding to the body and footer, and the
// other years' headers stay shown.
test("one year shows only its column, and a projected year still says so", async () => {
  const { doc } = await booted();
  set(doc, "capital-year", "2", "change");
  for (const el of doc.querySelectorAll("#capital-table [data-column]")) {
    assert.equal(el.hidden, el.getAttribute("data-column") !== "2", el.getAttribute("data-column"));
  }
  const head = doc.querySelector('#capital-table thead th[data-column="2"]');
  assert.equal(head.hidden, false);
  assert.ok(head.textContent.includes("projected"), head.textContent);
  assert.ok(bodyRows(doc).every((tr) => !tr.hidden), "a year filter hid a row");
});

test("formatCents prints whole dollars as the table does", async () => {
  const mod = await import(MODULE);
  assert.equal(mod.formatCents(7076545000), "$70,765,450");
  assert.equal(mod.formatCents(0), "$0");
  assert.equal(mod.formatCents(-150), "-$1.50");
});
