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
    ["NAMED", "TAIL_LABEL", "boot", "draw", "groupRows", "rank", "readRows", "showYear"]);
});

test("the page carries two years of three bars, and boot fills every figure", async () => {
  const { doc } = await booted();
  const all = bars(doc);
  assert.equal(all.length, 6, all.map((b) => `${b.label} ${b.id}`).join(", "));
  assert.deepEqual([...new Set(all.map((b) => b.id))], ["revenue", "spending", "departments"]);
  for (const b of all) {
    const figure = b.section.querySelector("figure.glance-figure");
    assert.equal(figure.hidden, false, `${b.label} ${b.id} figure stays hidden`);
    // Mutation: drop the hiding in glance.js's draw, and this goes red.
    assert.equal(b.section.querySelector(".glance-cells").hidden, true, `${b.label} ${b.id}'s cells still show under its bar`);
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

// The legend's shares are the client's, each a percentage marked ours and
// rounded to a tenth, so they may miss 100% by rounding and by no more; the
// segment's aria-label and the legend's detail carry no unmarked share.
// Mutation: drop the ◇ from the legend's share, or put the share back in the
// detail unmarked, and this goes red.
test("the legend's shares are marked ours and sum to 100% within their rounding", async () => {
  const { doc } = await booted();
  for (const b of bars(doc)) {
    const pers = [...b.section.querySelectorAll(".glance-per")].map((el) => el.textContent);
    for (const p of pers) assert.match(p, /^◇ (<0\.1|\d+\.\d)%$/, `${b.label} ${b.id}`);
    const sum = pers.reduce((a, p) => a + (p.includes("<") ? 0.05 : Number(p.slice(2, -1))), 0);
    assert.ok(Math.abs(sum - 100) <= 0.05 * pers.length + 1e-9, `${b.label} ${b.id}: ${pers.join(" + ")} = ${sum}`);
    for (const el of b.section.querySelectorAll(".glance-detail")) assert.ok(!el.textContent.includes("%"), el.textContent);
    for (const el of b.section.querySelectorAll(".glance-track > *")) {
      const label = el.getAttribute("aria-label") || "";
      assert.ok(!label.includes("%") || label.includes("◇"), label);
    }
  }
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

/** The page each fact cites, by fact id, read off facts/facts.jsonl. */
function factPages() {
  const out = new Map();
  const text = readFileSync(path.join(HERE, "..", "facts", "facts.jsonl"), "utf8");
  for (const line of text.split("\n")) {
    if (!line) continue;
    const f = JSON.parse(line);
    out.set(f.id, `extracted/${f.doc_id}/pages/p${String(f.page).padStart(4, "0")}.txt`);
  }
  return out;
}

// A click on a segment pins its tip, and the tip links the page text of
// every page its rows' facts were printed on, read off the fact store and
// not off the table the client reads. Mutation: drop pageLinks from the
// segment's fill in glance.js and every segment's links are missing; drop
// the PDF link from format.js's pageLinks and every PDF arm goes red; make
// tipper's show paint over a pinned tip and pinGestures' hover arm goes red.
test("a click pins a segment's tip, linking the page text and the PDF page of every fact its rows cite", async (t) => {
  const { installBrowser, pinGestures, tipState, pageTextExists, pdfFaults, fire } = await import("./testlib.mjs");
  const { window, document } = installBrowser({ html: PAGE });
  const mod = await import(MODULE);
  mod.boot(document);
  const pages = factPages();
  let pinned = 0;
  for (const b of bars(document)) {
    const { segments } = mod.rank(mod.groupRows(mod.readRows(b.section.querySelector("table"))));
    const named = new Set(segments.filter((s) => s.id).map((s) => s.id));
    const segs = [...b.section.querySelectorAll(".glance-seg")];
    assert.equal(segs.length, segments.length);
    segments.forEach((s, i) => {
      const want = new Set();
      for (const tr of b.section.querySelectorAll("tbody tr")) {
        if (s.id ? tr.dataset.group !== s.id : named.has(tr.dataset.group)) continue;
        for (const id of tr.dataset.facts.split(" ").filter(Boolean)) {
          assert.ok(pages.has(id), `${id} is in no fact`);
          want.add(pages.get(id));
        }
      }
      fire(segs[i], "click");
      const tip = tipState(document);
      assert.ok(tip.pinned && tip.shown, `${b.label} ${b.id} ${s.label}: no pinned tip`);
      assert.equal(tip.label, s.label);
      assert.deepEqual([...new Set(tip.hrefs)].sort(), [...want].sort(), `${b.label} ${b.id} ${s.label}: the tip links other pages`);
      for (const href of tip.hrefs) assert.ok(pageTextExists(href), `${href} names no committed page text`);
      assert.deepEqual(pdfFaults(tip), [], `${b.label} ${b.id} ${s.label}: the tip's PDF links are not its pages'`);
      pinned++;
    });
    fire(document.body, "click");
  }
  const [a, b] = bars(document)[0].section.querySelectorAll(".glance-seg");
  pinGestures(window, a, b);
  t.diagnostic(`${pinned} segments pinned, each linking its rows' facts' pages`);
});

// A bar that cannot draw says so in its own section, its table left shown,
// and the bars after it still draw. Mutation: drop boot's catch and the
// first bar's throw stops every later one, with no sentence on the page.
test("a bar whose table is malformed is refused in its section and the others draw", async () => {
  const dom = new JSDOM(PAGE);
  const mod = await import(MODULE);
  const doc = dom.window.document;
  const [first, ...rest] = bars(doc);
  first.section.querySelector("tbody td[data-cents]").removeAttribute("data-cents");
  mod.boot(doc);
  const said = first.section.querySelector("p.refusal");
  assert.ok(said !== null && /could not be drawn: .*carries null cents/.test(said.textContent), said && said.textContent);
  assert.ok(first.section.querySelector("figure.glance-figure").hidden, "the refused bar's figure is shown");
  const cells = first.section.querySelector(".glance-cells");
  assert.ok(!cells || !cells.hidden, "the refused bar's table is hidden");
  for (const b of rest) {
    assert.ok(!b.section.querySelector("figure.glance-figure").hidden, `${b.label} ${b.id} did not draw`);
    assert.ok(b.section.querySelector("p.refusal") === null, `${b.label} ${b.id} was refused`);
  }
});

// One year shows at a time, the one the select opens on, and choosing
// another shows it, writes it to the hash and carries it onto the top bar's
// links; a hash naming a year opens on it. With no script every year shows
// (the template hides none). Mutation: drop showYear's hiding and both years
// stay shown; drop its selectYear and the Flows link carries no year.
test("the year select shows one year at a time and carries it to the other pages", async () => {
  const shown = (doc) => [...doc.querySelectorAll("section.glance-year")].filter((s) => !s.hidden).map((s) => s.dataset.year);
  const bare = new JSDOM(PAGE).window.document;
  assert.equal([...bare.querySelectorAll("section.glance-year[hidden]")].length, 0, "the template hides a year");
  const dom = new JSDOM(PAGE, { url: "http://x/index.html" });
  const { window } = dom;
  const doc = window.document;
  (await import(MODULE)).boot(doc);
  const select = doc.getElementById("year-select");
  assert.ok(select && !select.disabled, "the year select is missing or left disabled");
  assert.deepEqual(shown(doc), [select.value]);
  const other = [...select.options].map((o) => o.value).find((v) => v !== select.value);
  select.value = other;
  select.dispatchEvent(new window.Event("change"));
  assert.deepEqual(shown(doc), [other]);
  assert.equal(window.location.hash, "#y=" + other);
  assert.match(doc.querySelector('nav.site-nav a[href^="flows"]').getAttribute("href"), new RegExp("#y=" + other + "$"));

  const asked = new JSDOM(PAGE, { url: "http://x/index.html#y=" + other });
  (await import(MODULE)).boot(asked.window.document);
  assert.deepEqual(shown(asked.window.document), [other]);
});
