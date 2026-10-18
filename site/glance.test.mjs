// What site/glance.js does for the glance page, over testdata/index.golden.html:
// the page Go writes over the client subset, which keeps every fund group of
// a budget year whole. Every cell here is one Go cited; what is tested is the
// client's grouping, its sums, its shares and its fold of the tail, and that
// the whole it draws is the figure Go printed beside it.
import { test, describe } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";
import { JSDOM } from "jsdom";

const HERE = path.dirname(fileURLToPath(import.meta.url));
const MODULE = pathToFileURL(path.join(HERE, "glance.js")).href;
const PAGE = readFileSync(path.join(HERE, "..", "testdata", "index.golden.html"), "utf8");

/** A fresh document over the pinned page, booted. */
async function booted() {
  const dom = new JSDOM(PAGE);
  const mod = await import(MODULE);
  const doc = dom.window.document;
  mod.boot(doc);
  return { mod, doc, window: dom.window };
}

/** Cents of a figure as Go prints it: "$254,095,412" or "$1,234.56". */
function centsOf(text) {
  const m = /^\$([\d,]+)(?:\.(\d{2}))?$/.exec(text.trim());
  assert.ok(m, `not a dollar figure: ${text}`);
  return Number(m[1].replace(/,/g, "")) * 100 + Number(m[2] || 0);
}

/** The bars of the page, by year label and bar id. */
function bars(doc) {
  const out = [];
  for (const year of doc.querySelectorAll("section.glance-year")) {
    const label = year.querySelector("h2").textContent.trim();
    for (const section of year.querySelectorAll("section.glance-bar")) {
      out.push({ label, id: section.dataset.bar, section });
    }
  }
  return out;
}

/** The rows of a bar's table as the page carries them. */
function rowsOf(section) {
  return [...section.querySelectorAll("tbody tr")].map((tr) => ({
    group: tr.dataset.group,
    label: tr.querySelector("th[scope=row]").textContent.trim(),
    cents: Number(tr.querySelector("td[data-cents]").getAttribute("data-cents")),
  }));
}

test("glance.js imports with no page config, no d3 and no DOM, and runs nothing", async () => {
  delete globalThis.FISC_CONFIG;
  delete globalThis.d3;
  assert.equal(typeof globalThis.document, "undefined", "nothing installed a DOM before the import");
  const mod = await import(MODULE);
  assert.deepEqual(Object.keys(mod).sort(),
    ["NAMED", "TAIL_LABEL", "boot", "dollars", "draw", "groupRows", "perHundred", "rank", "readRows", "share"]);
});

test("the page carries two years of three bars, and boot fills every figure", async () => {
  const { doc } = await booted();
  const all = bars(doc);
  assert.equal(all.length, 6, all.map((b) => `${b.label} ${b.id}`).join(", "));
  assert.deepEqual([...new Set(all.map((b) => b.id))], ["revenue", "spending", "departments"]);
  for (const b of all) {
    const figure = b.section.querySelector("figure.glance-figure");
    assert.equal(figure.hidden, false, `${b.label} ${b.id} figure stays hidden`);
    assert.ok(figure.querySelector(".glance-track"), `${b.label} ${b.id} has no track`);
    assert.ok(figure.querySelector(".glance-legend"), `${b.label} ${b.id} has no legend`);
  }
});

// Each drawn segment is a sum of the printed cells of its group, the tail is
// the rest, and together they are every cell once. Mutation: sum a group's
// first row only in groupRows, and every multi-row group goes red; drop the
// tail's push in rank and the fold loses its members.
describe("a bar's segments are sums of its cells, and no cell is lost or counted twice", async () => {
  const { mod, doc } = await booted();
  for (const b of bars(doc)) {
    test(`${b.label}: ${b.id}`, (t) => {
      const rows = rowsOf(b.section);
      const byGroup = new Map();
      for (const r of rows) byGroup.set(r.group, (byGroup.get(r.group) || 0) + r.cents);
      const whole = rows.reduce((a, r) => a + r.cents, 0);

      const { segments, whole: drawn } = mod.rank(mod.groupRows(mod.readRows(b.section.querySelector("table"))));
      assert.equal(drawn, whole);
      assert.equal(segments.reduce((a, s) => a + s.cents, 0), whole, "the segments do not sum to the whole");
      const named = segments.filter((s) => s.id);
      const tail = segments.find((s) => !s.id);
      for (const s of named) assert.equal(s.cents, byGroup.get(s.id), `${s.id} is not its rows' sum`);
      const members = segments.flatMap((s) => s.members);
      assert.deepEqual(members.sort(), [...new Set(rows.map((r) => r.label))].sort(), "a group is lost or doubled");
      if (byGroup.size > mod.NAMED + 1) {
        assert.ok(tail, `${byGroup.size} groups and no tail`);
        assert.equal(named.length, mod.NAMED);
        assert.equal(tail.cents, whole - named.reduce((a, s) => a + s.cents, 0));
        assert.equal(tail.label, mod.TAIL_LABEL);
      } else {
        assert.equal(tail, undefined, "a tail folded where every group could be named");
        assert.equal(named.length, byGroup.size);
      }
      // Largest first, and the tail last whatever its size.
      for (let i = 1; i < named.length; i++) assert.ok(named[i - 1].cents >= named[i].cents, "not ranked");

      // And the DOM says the same: one legend entry per segment, its cents on it.
      const legend = [...b.section.querySelectorAll(".glance-legend li")];
      assert.deepEqual(legend.map((li) => Number(li.dataset.cents)), segments.map((s) => s.cents));
      assert.equal(b.section.querySelectorAll(".glance-seg").length, segments.length);
      t.diagnostic(`${b.label} ${b.id}: ${rows.length} cells in ${byGroup.size} groups, ${named.length} named` +
        (tail ? `, ${tail.members.length} folded` : ""));
    });
  }
});

// The whole the client sums is the figure Go printed beside the bar, except
// where Go declares the rows apart from it, and there the difference is the
// declared residual. The declaration is Go's; the client only sums.
// Mutation: drop the fund/730 rows from a departments table and the
// FY2025-26 bar no longer reaches its printed whole.
describe("the client's whole is the city's figure, or the declared residual short of it", async () => {
  const { doc } = await booted();
  let apart = 0;
  for (const b of bars(doc)) {
    test(`${b.label}: ${b.id}`, (t) => {
      const printed = centsOf(b.section.querySelector("[data-whole-cents]").textContent);
      const whole = rowsOf(b.section).reduce((a, r) => a + r.cents, 0);
      const caption = b.section.querySelector(".glance-sum").textContent;
      assert.ok(caption.startsWith("◇ Our sum of the"), caption);
      assert.ok(caption.includes(new Intl.NumberFormat("en-US").format(whole / 100)), caption);
      if (b.section.dataset.ties === "true") {
        assert.equal(whole, printed, `${b.label} ${b.id}: rows do not reach the printed whole`);
        assert.equal(b.section.querySelector(".glance-apart"), null);
      } else {
        apart++;
        const line = b.section.querySelector(".glance-apart");
        assert.ok(line, "marked apart with no declaration printed");
        const residual = centsOf(line.querySelector("strong").textContent);
        assert.equal(whole + residual, printed, `${b.label} ${b.id}: rows plus the declared residual miss the printed whole`);
        t.diagnostic(`${b.label} ${b.id}: rows ${whole} + declared ${residual} = printed ${printed}`);
      }
    });
  }
  test("exactly one bar is declared apart: FY2026-27's departments", () => {
    assert.equal(apart, 1);
    const b = bars(doc).find((x) => x.section.dataset.ties === "false");
    assert.equal(b.id, "departments");
    assert.equal(b.label, "FY 2026-27 adopted");
  });
});

// Dollars of every $100 are the client's shares, to the cent, so they may
// miss $100.00 by rounding and by no more. Mutation: round perHundred to
// whole dollars and the ten revenue categories drift past the bound.
test("the legend's dollars of every $100 sum to 100 within the rounding of the segment count", async () => {
  const { mod, doc } = await booted();
  for (const b of bars(doc)) {
    const pers = [...b.section.querySelectorAll(".glance-per")].map((el) => Number(el.textContent.replace("$", "")));
    const sum = pers.reduce((a, x) => a + x, 0);
    assert.ok(Math.abs(sum - 100) <= 0.005 * pers.length + 1e-9, `${b.label} ${b.id}: ${pers.join(" + ")} = ${sum}`);
    for (const p of pers) assert.match(String(p.toFixed(2)), /^\d+\.\d\d$/);
  }
  assert.equal(mod.perHundred(1, 3), "$33.33");
  assert.equal(mod.share(1, 3), "33.3%");
  assert.equal(mod.dollars(25409541200), "$254,095,412");
  assert.equal(mod.dollars(123456), "$1,234.56");
});

// A cell the page cannot name is refused, not drawn as part of a bar.
// Mutation: default a missing data-cents to 0 in readRows and the first
// case draws a bar a cell short.
test("readRows refuses a row with no cents, no group or no label", async () => {
  const { mod, doc } = await booted();
  const table = bars(doc)[0].section.querySelector("table");
  const fresh = () => table.cloneNode(true);
  let t = fresh();
  t.querySelector("tbody tr td[data-cents]").removeAttribute("data-cents");
  assert.throws(() => mod.readRows(t), /carries null cents/);
  t = fresh();
  t.querySelector("tbody tr").removeAttribute("data-group");
  assert.throws(() => mod.readRows(t), /no group/);
  t = fresh();
  t.querySelector("tbody tr th[scope=row]").textContent = "";
  assert.throws(() => mod.readRows(t), /no label/);
  t = fresh();
  t.querySelector("tbody tr td[data-cents]").setAttribute("data-cents", "12.5");
  assert.throws(() => mod.readRows(t), /carries 12.5 cents/);
  assert.equal(mod.readRows(fresh()).length, rowsOf(bars(doc)[0].section).length);
});

test("a group labelled two ways is refused", async () => {
  const { mod } = await booted();
  assert.throws(() => mod.groupRows([{ group: "a", label: "A", cents: 1 }, { group: "a", label: "B", cents: 1 }]),
    /labelled A and B/);
});
