// What site/change.js does for change.html, over testdata/change.golden.html:
// the page Go writes over the client subset. Every cell is one printed point
// Go cited; what is tested is the client's grouping by kind, its sums per
// column, its fold of the tail, the rank a kind keeps across columns, and
// that the audited revenue and spending sums differ by the excess the city
// prints.
import { test, describe } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";
import { JSDOM } from "jsdom";
import { NAMED, TAIL_LABEL } from "./format.js";

const HERE = path.dirname(fileURLToPath(import.meta.url));
const MODULE = pathToFileURL(path.join(HERE, "change.js")).href;
const PAGE = readFileSync(path.join(HERE, "..", "testdata", "change.golden.html"), "utf8");

/** A fresh document over the pinned page, booted. */
async function booted() {
  const dom = new JSDOM(PAGE);
  const mod = await import(MODULE);
  const doc = dom.window.document;
  mod.boot(doc);
  return { mod, doc };
}

/** The figure sections, by id. */
function figures(doc) {
  const out = new Map();
  for (const s of doc.querySelectorAll("section.change-figure")) out.set(s.dataset.figure, s);
  return out;
}

/** Each column's sum of the table's cents, by column index. */
function columnSums(section) {
  const sums = new Map();
  for (const td of section.querySelectorAll("tbody td[data-cents]")) {
    const c = Number(td.getAttribute("data-column"));
    sums.set(c, (sums.get(c) || 0) + Number(td.getAttribute("data-cents")));
  }
  return sums;
}

test("the page carries four figures, three charted, and boot fills each chart", async () => {
  const { doc } = await booted();
  const figs = figures(doc);
  assert.deepEqual([...figs.keys()], ["audited-revenue", "audited-spending", "audited-excess", "budget-revenue"]);
  for (const [id, s] of figs) {
    const chart = s.querySelector("figure.change-chart");
    if (s.dataset.chart === "true") {
      assert.ok(chart && !chart.hidden, `${id}'s chart stays hidden`);
      assert.ok(chart.querySelectorAll(".change-column").length > 0, `${id} draws no column`);
      // Mutation: drop the hiding in change.js's draw, and this goes red.
      assert.equal(s.querySelector(".change-cells").hidden, true, `${id}'s table still shows under its chart`);
    } else {
      assert.equal(chart, null, `${id} carries a chart it should not`);
      assert.equal(s.querySelector(".change-cells").hidden, false, "the excess table, its only figure, is hidden");
    }
  }
});

// Each drawn column is the sum of the printed rows under it, band by band
// and in all. Mutation: add a row's first column only in groupRows and every
// later column goes red.
describe("each column's bands sum to its printed rows, and every kind is in one band", async () => {
  const { mod, doc } = await booted();
  for (const [id, s] of figures(doc)) {
    if (s.dataset.chart !== "true") continue;
    test(id, (t) => {
      const sums = columnSums(s);
      const cols = [...s.querySelectorAll(".change-column")];
      assert.equal(cols.length, sums.size);
      for (const col of cols) {
        const i = Number(col.dataset.column);
        const segs = [...col.querySelectorAll(".change-seg")];
        const drawn = segs.reduce((a, x) => a + Number(x.dataset.cents), 0);
        assert.equal(drawn, sums.get(i), `${id} column ${i}: bands sum to ${drawn}, rows to ${sums.get(i)}`);
        assert.equal(Number(col.dataset.cents), sums.get(i));
        for (const x of segs) {
          const below = x.parentElement.classList.contains("change-below");
          assert.equal(Number(x.dataset.cents) < 0, below, `${id} column ${i}: a band is on the wrong side of the baseline`);
        }
      }
      const { rows, columns } = mod.readRows(s.querySelector("table"));
      const bands = mod.rank(mod.groupRows(rows, columns));
      const kinds = new Set(rows.map((r) => r.category));
      assert.deepEqual(bands.flatMap((b) => b.members).sort(), [...kinds].sort(), "a kind is lost or doubled");
      const named = bands.filter((b) => b.rank);
      if (kinds.size > NAMED + 1) {
        assert.equal(named.length, NAMED);
        assert.equal(bands[bands.length - 1].label, TAIL_LABEL);
      } else assert.equal(named.length, kinds.size);
      // A rank is a kind's and holds in every column: the band of rank r
      // in each column names the same kind.
      for (const b of named) {
        for (const col of cols) {
          const seg = col.querySelector(`.change-seg[data-rank="${b.rank}"]`);
          if (seg) assert.ok(seg.getAttribute("aria-label").startsWith(b.label + ","), `${id}: rank ${b.rank} is ${seg.getAttribute("aria-label")} in column ${col.dataset.column}, not ${b.label}`);
        }
      }
      t.diagnostic(`${id}: ${rows.length} rows in ${kinds.size} kinds over ${cols.length} columns, ${named.length} named`);
    });
  }
});

// The audited revenue and spending sums differ, in every year, by exactly the
// excess of revenues over expenditures the report prints: the client's two
// sums against a figure the city printed, with no difference computed on the
// page. Mutation: drop the fund_balance rows from the excess figure and this
// has nothing to hold the sums to.
test("ten years: our revenue sum less our spending sum is the printed excess, to the cent", async (t) => {
  const { doc } = await booted();
  const figs = figures(doc);
  const revenue = columnSums(figs.get("audited-revenue"));
  const spending = columnSums(figs.get("audited-spending"));
  const excess = columnSums(figs.get("audited-excess"));
  assert.equal(excess.size, 10);
  assert.equal(figs.get("audited-excess").querySelectorAll("tbody tr").length, 1);
  for (const [i, printed] of excess) {
    assert.equal(revenue.get(i) - spending.get(i), printed, `column ${i}`);
  }
  const years = [...figs.get("audited-excess").querySelectorAll("thead th[data-column]")].map((th) => th.textContent.trim().split(/\s/)[0]);
  t.diagnostic(`${years[0]}..${years[years.length - 1]}: revenue less spending equals the printed excess in all ${excess.size} years`);
});

// A rule stands between two columns that are not one measurement, and
// nowhere else. Mutation: set data-new on every column and the audited
// figures grow rules between audited years.
test("the budget's columns carry rules at its two measurement boundaries and the audited years carry none", async () => {
  const { doc } = await booted();
  const figs = figures(doc);
  const rules = (id) => [...figs.get(id).querySelectorAll(".change-column")].filter((c) => c.dataset.new).map((c) => c.dataset.column);
  assert.deepEqual(rules("budget-revenue"), ["1", "2"]);
  assert.deepEqual(rules("audited-revenue"), []);
  assert.deepEqual(rules("audited-spending"), []);
});

// The largest band stands nearest the baseline on either side of it, in
// every column, the stack above the baseline is one height in every column
// and holds every band above it at its own height, gaps included, so one
// baseline runs across them whatever hangs below (jsdom lays nothing out, so
// the heights are held here and the alignment is CSS's), and the header's
// year and basis stay two words. Mutation: drop the above.style.height line
// and the first assertion fails in every column; drop the GAP term from
// room and the tallest column's bands overflow its stack;
// iterate the bands reversed in draw and rank 1 lands at the top of every
// column; take th.textContent for the caption and "FY 2015-16unaudited" is
// one word.
test("rank 1 touches the baseline in every column, and a column's year and basis are two words", async () => {
  const { mod, doc } = await booted();
  for (const [id, s] of figures(doc)) {
    if (s.dataset.chart !== "true") continue;
    const heights = new Set();
    let fullest = 0;
    for (const col of s.querySelectorAll(".change-column")) {
      const ranks = (sel) => [...col.querySelectorAll(sel)].map((x) => Number(x.dataset.rank) || Infinity);
      const above = ranks(".change-above .change-seg");
      const below = ranks(".change-below .change-seg");
      assert.deepEqual(above, [...above].sort((a, b) => b - a), `${id} column ${col.dataset.column}: ranks above the baseline run ${above}`);
      assert.deepEqual(below, [...below].sort((a, b) => a - b), `${id} column ${col.dataset.column}: ranks below the baseline run ${below}`);
      const stack = col.querySelector(".change-above");
      assert.match(stack.style.height, /px$/, `${id}: the stack above the baseline has no height`);
      heights.add(stack.style.height);
      const segs = [...stack.querySelectorAll(".change-seg")];
      const need = segs.reduce((a, x) => a + parseFloat(x.style.height), 0) + mod.GAP * Math.max(segs.length - 1, 0);
      assert.ok(need <= parseFloat(stack.style.height) + 1e-9, `${id} column ${col.dataset.column}: ${need}px of bands in a ${stack.style.height} stack`);
      assert.equal(stack.style.gap, `${mod.GAP}px`);
      for (const x of segs) assert.ok(parseFloat(x.style.height) >= mod.LEAST, `${id}: a band under ${mod.LEAST}px`);
      fullest = Math.max(fullest, need);
      const year = col.querySelector(".change-year");
      const basis = year.querySelector(".basis");
      const th = s.querySelector(`thead th[data-column="${col.dataset.column}"]`);
      const thBasis = th.querySelector(".basis");
      assert.match(year.firstChild.textContent, /^FY \d{4}-\d{2}$/, `${id}: the year reads ${year.textContent}`);
      assert.equal(basis ? basis.textContent : "", thBasis ? thBasis.textContent.trim() : "");
      for (const seg of col.querySelectorAll(".change-seg")) {
        assert.ok(seg.getAttribute("aria-label").includes(`, ${year.firstChild.textContent}${basis ? " " + basis.textContent : ""}: `), `${id}: ${seg.getAttribute("aria-label")}`);
      }
    }
    assert.equal(heights.size, 1, `${id}: the stacks above the baseline are ${[...heights]}`);
    assert.ok(Math.abs(fullest - parseFloat([...heights][0])) < 0.011, `${id}: the fullest stack is ${fullest}px in ${[...heights][0]}`);
  }
});

// The one audited year of negative investment income hangs below the baseline.
test("FY2021-22's use of money and property hangs below the baseline, and nothing else does on the audited side", async () => {
  const { doc } = await booted();
  const figs = figures(doc);
  const below = [...figs.get("audited-revenue").querySelectorAll(".change-below .change-seg")];
  assert.equal(below.length, 1);
  assert.match(below[0].getAttribute("aria-label"), /^Use of Money And Property, FY 2021-22/);
  assert.ok(Number(below[0].dataset.cents) < 0);
  assert.equal(figs.get("audited-spending").querySelectorAll(".change-below .change-seg").length, 0);
  // The lede says what that stack is: a total that is the net, with a
  // negative kind below the baseline, not a height that is the sum.
  // Mutation: put back "Each column's height is our sum" in change.go.
  const lede = figs.get("audited-revenue").querySelector(".change-lede").textContent;
  assert.match(lede, /total is our sum of the rows .* hangs below the baseline and counts against that total/);
  assert.ok(!/height is our sum/.test(lede), lede);
});

// A cell the page cannot place is refused, not stacked. Mutation: default a
// missing category to "other" in readRows and the first case draws a band
// for no kind.
test("readRows refuses a row with no category and a cell that is not integer cents", async () => {
  const { mod, doc } = await booted();
  const table = figures(doc).get("audited-revenue").querySelector("table");
  let t = table.cloneNode(true);
  t.querySelector("tbody tr").removeAttribute("data-category");
  assert.throws(() => mod.readRows(t), /carries no category/);
  t = table.cloneNode(true);
  t.querySelector("tbody td[data-cents]").setAttribute("data-cents", "1.5");
  assert.throws(() => mod.readRows(t), /carries "1.5" cents/);
  // Absent is not zero: an empty attribute is refused, not read as 0.
  // Mutation: Number(raw) without the integer test and the row reads 0.
  t = table.cloneNode(true);
  t.querySelector("tbody td[data-cents]").setAttribute("data-cents", "");
  assert.throws(() => mod.readRows(t), /carries "" cents/);
  const { rows, columns } = mod.readRows(table.cloneNode(true));
  assert.equal(columns, 10);
  assert.equal(rows.length, 9);
  assert.equal(mod.millions(14952609600), "$149.5M");
  assert.equal(mod.millions(-126150980), "-$1.3M");
});

// Absent is not zero: a kind or a column none of whose rows prints a figure
// sums to null, and the legend and the cap say it prints nothing rather than
// $0. Each case is the pinned audited-revenue figure with its cells' data-cents
// stripped as stated, which readRows reads as absent.
//
// Mutation: seed groupRows' sums with 0 in place of null and every arm goes
// red; keep the sums null but drop the null arm of the legend's detail, or of
// the column's cap, and that arm does.
describe("a kind or a column that prints nothing is said so, not drawn as $0", async () => {
  const { mod, doc } = await booted();
  const section = figures(doc).get("audited-revenue");
  const strip = (cells) => {
    let n = 0;
    for (const td of cells) { td.removeAttribute("data-cents"); n++; }
    assert.ok(n > 0, "no cell stripped");
  };

  test("a kind with no figure in the last column is legended as printing nothing there", () => {
    const s = section.cloneNode(true);
    // Cut to three kinds so the stripped kind is named, not folded.
    const rows = [...s.querySelectorAll("tbody tr")];
    for (const tr of rows.slice(3)) tr.remove();
    const kind = rows[2].dataset.category;
    const { columns } = mod.readRows(s.querySelector("table"));
    strip(s.querySelectorAll(`tbody tr[data-category="${kind}"] td[data-column="${columns - 1}"]`));
    const { bands } = mod.draw(s);
    const band = bands.find((b) => b.members.includes(kind));
    assert.ok(band && band.rank > 0, `${kind} folded into the tail`);
    assert.equal(band.cents[columns - 1], null, `${kind}'s absent last cell summed to a figure`);
    const entry = s.querySelector(`.change-legend li[data-rank="${band.rank}"] .change-detail`);
    assert.equal(entry.textContent, "prints nothing in the last column");
  });

  test("a column with no figure in it is capped as printing nothing, and carries no sum", () => {
    const s = section.cloneNode(true);
    strip(s.querySelectorAll('tbody td[data-column="0"]'));
    const { sums } = mod.draw(s);
    assert.equal(sums[0], null, "the absent column summed to a figure");
    const col = s.querySelector('.change-column[data-column="0"]');
    assert.deepEqual({ cap: col.querySelector(".change-total").textContent, cents: col.dataset.cents ?? null, bands: col.querySelectorAll(".change-seg").length },
      { cap: "prints nothing", cents: null, bands: 0 });
    // Every other column still sums its printed rows.
    assert.notEqual(sums[1], null);
  });

  // Mutation: drop the blank-column clause from the caption and this goes
  // red; the caption then calls a column with no printed figure a sum.
  test("the caption names a column that prints nothing rather than calling it a sum", () => {
    const s = section.cloneNode(true);
    strip(s.querySelectorAll('tbody td[data-column="0"]'));
    mod.draw(s);
    const head = mod.headWords(s.querySelector('thead th[data-column="0"]'));
    const named = head.label + (head.basis ? " " + head.basis : "");
    const caption = s.querySelector(".change-sum").textContent;
    assert.ok(caption.includes(`prints nothing where none of the rows prints in it (${named})`), caption);
  });

  test("a caption over columns that all print names none as printing nothing", () => {
    const s = section.cloneNode(true);
    mod.draw(s);
    const caption = s.querySelector(".change-sum").textContent;
    assert.equal(caption.includes("prints nothing"), false, caption);
  });
});

// A click on a band pins its tip, and the tip links the page text of each
// of the band's cells in that column, a page that prints the cell's figure.
// Mutation: drop pageLinks from the band's fill in change.js and every
// band's links are missing; drop the PDF link from format.js's pageLinks
// and every PDF arm goes red; match a band's rows by its id in place of its
// members, and the tail links no page.
test("a click pins a band's tip, linking the page text and the PDF page that print each of its cells", async (t) => {
  const { installBrowser, tipState, pageTextExists, pdfFaults, fire, repoRoot } = await import("./testlib.mjs");
  const { document } = installBrowser({ html: PAGE });
  const mod = await import(MODULE);
  mod.boot(document);
  const whole = new Intl.NumberFormat("en-US", { maximumFractionDigits: 0 });
  let pinned = 0;
  let tails = 0;
  for (const [id, s] of [...figures(document)].filter(([, s]) => s.dataset.chart === "true").slice(0, 1)) {
    const bands = mod.rank(mod.groupRows(mod.readRows(s.querySelector("table")).rows, mod.readRows(s.querySelector("table")).columns));
    for (const col of s.querySelectorAll(".change-column")) {
      const i = col.dataset.column;
      for (const seg of col.querySelectorAll(".change-seg")) {
        const band = bands.find((b) => String(b.rank) === seg.dataset.rank);
        const cells = [...s.querySelectorAll("tbody tr")].filter((tr) => band.members.includes(tr.dataset.category))
          .map((tr) => tr.querySelector(`td[data-column="${i}"][data-cents]`)).filter(Boolean);
        fire(seg, "click");
        const tip = tipState(document);
        assert.ok(tip.pinned && tip.shown, `${id} column ${i} rank ${seg.dataset.rank}: no pinned tip`);
        const want = new Set(cells.map((td) => td.querySelector("a").getAttribute("href")));
        assert.deepEqual([...new Set(tip.hrefs)].sort(), [...want].sort(), `${id} column ${i} rank ${seg.dataset.rank}: the tip links other pages`);
        for (const href of tip.hrefs) assert.ok(pageTextExists(href), `${href} names no committed page text`);
        assert.deepEqual(pdfFaults(tip), [], `${id} column ${i} rank ${seg.dataset.rank}: the tip's PDF links are not its pages'`);
        const texts = tip.hrefs.map((h) => readFileSync(path.join(repoRoot, "data", h), "utf8"));
        for (const td of cells) {
          const printed = whole.format(Math.abs(Number(td.getAttribute("data-cents"))) / 100);
          assert.ok(texts.some((x) => x.includes(printed)), `${id} column ${i}: no linked page prints ${printed}`);
        }
        pinned++;
        if (!band.rank) tails++;
      }
    }
  }
  t.diagnostic(`${pinned} bands pinned, ${tails} of them tails, each linking pages that print its cells`);
  assert.ok(tails > 0, "no tail band was pinned");
});

// A chart that cannot draw says so in its own section, its table left
// shown, and the charts after it still draw. Mutation: drop boot's catch
// and the first chart's throw stops every later one, with no sentence on
// the page.
test("a chart whose table is malformed is refused in its section and the others draw", async () => {
  const dom = new JSDOM(PAGE);
  const mod = await import(MODULE);
  const doc = dom.window.document;
  const [first, ...rest] = [...doc.querySelectorAll('section.change-figure[data-chart="true"]')];
  assert.ok(rest.length > 0, "the page draws one chart, so nothing shows the others survive");
  first.querySelector("tbody td[data-cents]").setAttribute("data-cents", "12.5");
  mod.boot(doc);
  const said = first.querySelector("p.refusal");
  assert.ok(said !== null && /could not be drawn: .*carries "12\.5" cents/.test(said.textContent), said && said.textContent);
  assert.ok(first.querySelector("figure.change-chart").hidden, "the refused chart is shown");
  const cells = first.querySelector(".change-cells");
  assert.ok(!cells || !cells.hidden, "the refused chart's table is hidden");
  for (const s of rest) {
    assert.ok(!s.querySelector("figure.change-chart").hidden, `${s.dataset.figure} did not draw`);
  }
});
