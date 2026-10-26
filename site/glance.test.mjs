// What site/glance.js does for the glance page, over testdata/client/index.html:
// a planted page held to schema/bindings.json, its tables shaped for the
// mechanisms below. What is tested is the client's grouping, its sums, its
// shares and its fold of the tail; that each bar's rows reach the whole Go
// prints beside it is Go's, refused at the write (glanceTies).
import { test, describe } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";
import { JSDOM } from "jsdom";
import { NAMED, TAIL_LABEL } from "./format.js";
import { bootedDocument } from "./testlib.mjs";

const HERE = path.dirname(fileURLToPath(import.meta.url));
const MODULE = pathToFileURL(path.join(HERE, "glance.js")).href;
const PAGE = readFileSync(path.join(HERE, "..", "testdata", "client", "index.html"), "utf8");

/** A fresh document over the pinned page, booted. */
const booted = (url) => bootedDocument(PAGE, MODULE, url);

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

test("boot fills every bar's figure", async () => {
  const { doc } = await booted();
  const all = bars(doc);
  assert.ok(all.length > 1, "the planted page carries fewer than two bars");
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
  let folded = 0;
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
      if (tail) folded++;
      assert.deepEqual(members.sort(), [...new Set(rows.map((r) => r.label))].sort(), "a group is lost or doubled");
      if (byGroup.size > NAMED + 1) {
        assert.ok(tail, `${byGroup.size} groups and no tail`);
        assert.equal(named.length, NAMED);
        assert.equal(tail.cents, whole - named.reduce((a, s) => a + s.cents, 0));
        assert.equal(tail.label, TAIL_LABEL);
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
  test("the planted page folds a tail in one bar and names every group in another", () => {
    assert.ok(folded > 0 && folded < bars(doc).length, `${folded} of ${bars(doc).length} bars fold a tail`);
  });
});

// A bar that ties states its total once, in the printed line under it; one
// Go holds apart states the client's own sum too, since that sum is not the
// printed figure. Mutation: say the sum in the caption whatever data-ties
// says, and the tied arm goes red.
test("a tied bar's caption states no sum of ours, and one held apart states it", async () => {
  const { doc } = await booted();
  const seen = new Set();
  for (const b of bars(doc)) {
    const whole = rowsOf(b.section).reduce((a, r) => a + r.cents, 0);
    const caption = b.section.querySelector(".glance-sum").textContent;
    const said = caption.includes(new Intl.NumberFormat("en-US").format(whole / 100));
    const ties = b.section.dataset.ties === "true";
    seen.add(ties);
    if (ties) assert.ok(caption.startsWith("◇ The shares are ours") && !said, caption);
    else assert.ok(caption.startsWith("◇ Our sum of the") && said, caption);
  }
  assert.deepEqual([...seen].sort(), [false, true], "the planted page lacks a tied bar or one held apart");
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
    for (const p of pers) assert.match(p, /^◇ (<0\.1|>99\.9|\d+\.\d)%$/, `${b.label} ${b.id}`);
    const sum = pers.reduce((a, p) => a + (p.includes("<") ? 0.05 : p.includes(">") ? 99.95 : Number(p.slice(2, -1))), 0);
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
// case draws a bar a cell short; read the cell with Number() and "0x10"
// draws as 16 cents.
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
  for (const raw of ["0x10", "1e3", " 12 ", ""]) {
    t = fresh();
    t.querySelector("tbody tr td[data-cents]").setAttribute("data-cents", raw);
    assert.throws(() => mod.readRows(t), /carries .* cents/, JSON.stringify(raw));
  }
  assert.equal(mod.readRows(fresh()).length, rowsOf(bars(doc)[0].section).length);
});

test("a group labelled two ways is refused", async () => {
  const { mod } = await booted();
  assert.throws(() => mod.groupRows([{ group: "a", label: "A", cents: 1 }, { group: "a", label: "B", cents: 1 }]),
    /labelled A and B/);
});

// A click on a segment pins its tip, and the tip links the page text and
// the PDF page of every row the segment sums, the tail's included.
// Mutation: drop pageLinks from the segment's fill in glance.js and every
// segment's links are missing; drop the PDF link from format.js's pageLinks
// and the PDF arm goes red; match the tail's rows by its id in place of its
// members and the tail links no page.
test("a click pins a segment's tip, linking the page text and the PDF page of each of its rows", async () => {
  const { installBrowser, tipState, pdfFaults, fire } = await import("./testlib.mjs");
  const { document } = installBrowser({ html: PAGE });
  const mod = await import(MODULE);
  mod.boot(document);
  // Mutation: set role="img" on a segment before pinnable and this is red.
  assert.deepEqual([...document.querySelectorAll("[data-pins]")].filter((m) => m.getAttribute("role") !== "button").map((m) => m.dataset.rank), []);
  const revenue = bars(document)[0];
  const segs = [...revenue.section.querySelectorAll(".glance-seg")];
  const pages = (seg) => {
    fire(seg, "click");
    const tip = tipState(document);
    assert.ok(tip.pinned && tip.shown, `${tip.label}: no pinned tip`);
    assert.deepEqual(pdfFaults(tip), [], `${tip.label}: the tip's PDF links are not its pages'`);
    fire(document.body, "click");
    return { label: tip.label, pages: tip.hrefs.map((h) => Number(/p(\d{4})\.txt$/.exec(h)[1])) };
  };
  // The planted revenue bar: Taxes is two rows on pp.66 and 67, and the
  // tail folds Interest (p66) and Other (p67).
  assert.deepEqual(pages(segs[0]), { label: "Taxes", pages: [66, 67] });
  assert.deepEqual(pages(segs.at(-1)), { label: TAIL_LABEL, pages: [66, 67] });
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
  const { window, doc } = await booted("http://x/index.html");
  const select = doc.getElementById("year-select");
  assert.ok(select && !select.disabled, "the year select is missing or left disabled");
  assert.deepEqual(shown(doc), [select.value]);
  const other = [...select.options].map((o) => o.value).find((v) => v !== select.value);
  select.value = other;
  select.dispatchEvent(new window.Event("change"));
  assert.deepEqual(shown(doc), [other]);
  assert.equal(window.location.hash, "#y=" + other);
  assert.match(doc.querySelector('nav.site-nav a[href^="flows"]').getAttribute("href"), new RegExp("#y=" + other + "$"));

  // A hash naming no year leaves the year on screen. Mutation: fall back to
  // the option the template selects, and this shows the newest again.
  window.location.hash = "#y=fy2099-adopted";
  window.dispatchEvent(new window.HashChangeEvent("hashchange"));
  assert.deepEqual(shown(doc), [other]);

  const asked = await booted("http://x/index.html#y=" + other);
  assert.deepEqual(shown(asked.doc), [other]);
});

// A year switch unpins a tip over the year it hides, or the tip would state
// that year's figure beside the other year's bars. Mutation: drop the unpin
// in boot's turn, and the tip stays pinned and shown.
test("a year switch unpins a tip pinned over the year it hides", async () => {
  const { installBrowser, tipState, fire } = await import("./testlib.mjs");
  const { window, document } = installBrowser({ html: PAGE });
  (await import(MODULE)).boot(document);
  const select = document.getElementById("year-select");
  const seg = document.querySelector(`section.glance-year[data-year="${select.value}"] .glance-seg`);
  fire(seg, "click");
  assert.ok(tipState(document).pinned, "the click pinned no tip");
  select.value = [...select.options].map((o) => o.value).find((v) => v !== select.value);
  select.dispatchEvent(new window.Event("change"));
  const tip = tipState(document);
  assert.ok(!tip.pinned && !tip.shown, "a tip over the hidden year is still pinned or shown");
});
