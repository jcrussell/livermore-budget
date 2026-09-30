// figures.test.mjs — the dollar figures and citations a reader is shown, held
// against the cents and locators Go pinned in testdata/ and formatted by the
// test, never by app.js.
//
// A few figures are also written out literally, pinning the test's own
// formatter: ERAF's as the city printed it, Property Taxes' as the sum of rows it printed.

import { describe, test } from "node:test";
import assert from "node:assert/strict";
import {
  bootedApp, opened, settle, clickYear, columnFixture, pageFixture, dollars, shortDollars,
} from "./testlib.mjs";

const PAGE = pageFixture();
const PROPERTY = "revenue/taxes/property";
const ERAF = "revenue-line/taxes/property/eraf";

const YEARS = [
  { stem: "sankey", column: "fy2026-adopted", eraf: "−$15,175,000", property: "$69,459,414" },
  { stem: "sankey-2027", column: "fy2027-adopted", eraf: "−$15,857,875", property: "$72,891,844" },
];

/** The links of one schedule of a pinned column whose ends pass `match`. */
function columnLinks(stem, schedule, match) {
  const col = columnFixture(stem);
  const id = (i) => col.nodes[i].id;
  return col.schedules[schedule].links.filter((l) => match(id(l.from), id(l.to)));
}

async function onYear(stem) {
  const loaded = await bootedApp();
  if (loaded.app.shownYear.stem !== stem) {
    clickYear(loaded.document, stem);
    await settle();
  }
  assert.equal(loaded.app.shownYear.stem, stem);
  return loaded;
}

const markOf = (chart, id) => [...chart.querySelectorAll("g.node")].find((g) => g.__data__.id === id);
const ribbonOf = (chart, from, to) => [...chart.querySelectorAll("path.link")]
  .find((p) => p.__data__.source.id === from && p.__data__.target.id === to);

function shown(app, document, d) {
  const chart = document.getElementById("chart");
  app.showTip({ target: chart, clientX: 0, clientY: 0 }, d);
  const tip = document.querySelector("#tooltip .tip-value").textContent;
  app.pin(d);
  const panel = document.querySelector("#detail .amount").textContent;
  return { tip, panel };
}

for (const year of YEARS) {
  describe(`${year.column}: the figures a reader is shown`, () => {
    test(`${year.column}: a printed node's total is the city's, in the tooltip, the panel, its label and its aria`, async (t) => {
      const { app, document } = await onYear(year.stem);
      const leaving = columnLinks(year.column, "sankey", (from) => from === PROPERTY);
      const cents = leaving.reduce((s, l) => s + l.value_cents, 0);
      t.diagnostic(`${PROPERTY} leaves by ${leaving.length} link(s) in the pinned column, ${cents} cents`);
      assert.ok(leaving.length > 0);
      assert.equal(dollars(cents), year.property);
      const m = markOf(document.getElementById("chart"), PROPERTY);
      assert.ok(m, `${PROPERTY} is not drawn`);
      const { tip, panel } = shown(app, document, m.__data__);
      assert.equal(tip, year.property);
      assert.equal(panel, year.property);
      assert.ok(m.getAttribute("aria-label").startsWith(`Property Taxes, total ${year.property},`),
        m.getAttribute("aria-label"));
      const tspans = [...m.querySelectorAll("tspan")].map((s) => s.textContent);
      assert.ok(tspans.includes("  " + shortDollars(cents)), JSON.stringify(tspans));
    });

    test(`${year.column}: a reduction reads as a negative figure wherever it is shown`, async (t) => {
      const { app, document } = await onYear(year.stem);
      await opened(app, PROPERTY);
      const [printed] = columnLinks(year.column, "fund-flows", (from, to) => from === ERAF && to === PROPERTY);
      t.diagnostic(`ERAF into ${PROPERTY} is ${printed.value_cents} cents in the pinned column`);
      assert.ok(printed.value_cents < 0);
      assert.equal(dollars(printed.value_cents), year.eraf);
      const chart = document.getElementById("chart");
      const ribbon = ribbonOf(chart, ERAF, PROPERTY);
      assert.ok(ribbon, "ERAF is not drawn as a ribbon");
      const onRibbon = shown(app, document, ribbon.__data__);
      assert.equal(onRibbon.tip, year.eraf);
      assert.equal(onRibbon.panel, year.eraf);
      assert.ok(ribbon.getAttribute("aria-label").includes(`, ${year.eraf},`), ribbon.getAttribute("aria-label"));
      const mark = markOf(chart, ERAF);
      assert.ok(mark.getAttribute("aria-label").startsWith(`ERAF, total ${year.eraf},`), mark.getAttribute("aria-label"));
      assert.equal(shown(app, document, mark.__data__).tip, year.eraf);
      const tspans = [...mark.querySelectorAll("tspan")].map((s) => s.textContent);
      assert.ok(tspans.includes("  " + shortDollars(printed.value_cents)), JSON.stringify(tspans));
      const row = [...document.querySelectorAll("#flow-table tbody tr")]
        .find((tr) => tr.children[0].textContent === "ERAF");
      assert.ok(row, "the flow table has no ERAF row");
      assert.equal(row.children[2].textContent, year.eraf);
    });

    test(`${year.column}: a node's share of its column is its drawn figure over the column's`, async (t) => {
      const { app, document } = await onYear(year.stem);
      const chart = document.getElementById("chart");
      const d = markOf(chart, PROPERTY).__data__;
      const column = [...chart.querySelectorAll("g.node")].map((g) => g.__data__).filter((n) => n.layer === d.layer);
      assert.ok(column.length >= 2);
      assert.ok(!column.some((n) => app.isResidual(n.id)), "a residual is sized by its own figure");
      const total = column.reduce((s, n) => s + n.value, 0);
      const want = `◇ our ${((100 * d.value) / total).toFixed(1)}% of this column`;
      t.diagnostic(`${PROPERTY} is ${d.value} of ${total} over ${column.length} drawn marks: "${want}"`);
      app.pin(d);
      const chips = [...document.querySelectorAll("#detail .chip.derived")].map((c) => c.textContent);
      assert.ok(chips.includes(want), JSON.stringify(chips));
    });

    test(`${year.column}: the inferred list totals every inferred flow it lists`, async (t) => {
      const { document } = await onYear(year.stem);
      const items = [...document.querySelectorAll("#derived-list li")];
      let checked = 0;
      for (const [id, label] of [["fund-balance/draw", "Fund Balance Draw"], ["fund-balance/contribution", "Fund Balance Contribution"]]) {
        const flows = columnLinks(year.column, "sankey", (from, to) => from === id || to === id).filter((l) => l.derived);
        const cents = flows.reduce((s, l) => s + l.value_cents, 0);
        const item = items.find((li) => li.querySelector(".what").textContent === "◇ " + label);
        assert.ok(item, `the inferred list has no entry for ${label}`);
        const want = `${flows.length} inferred flow${flows.length === 1 ? "" : "s"} totalling ${dollars(cents)}:`;
        t.diagnostic(`${label}: ${want}`);
        assert.ok(item.textContent.includes(want), item.textContent);
        if (flows.length > 1) checked++;
      }
      assert.ok(checked > 0, "no entry lists more than one flow, so a total of the first alone would pass");
    });

    test(`${year.column}: a ribbon's Sources are its own pages, at the file names Go wrote`, async (t) => {
      const { app, document, window } = await onYear(year.stem);
      await opened(app, PROPERTY);
      const [printed] = columnLinks(year.column, "fund-flows", (from, to) => from === ERAF && to === PROPERTY);
      const doc = PAGE.config.docs[printed.locators[0].doc_id];
      // The anchors Go rendered into the served page: a page's text link in
      // the config is the one the page's own footer carries.
      const served = new Set();
      for (const a of new window.DOMParser().parseFromString(PAGE.html, "text/html").querySelectorAll("a[href]")) {
        served.add(a.getAttribute("href"));
      }
      const want = [];
      for (const loc of printed.locators) {
        for (const page of loc.pages) {
          const links = doc.pages[String(page)];
          assert.ok(links, `the config carries no links for p${page}`);
          assert.ok(served.has(links.text), `p${page}'s text link ${links.text} is not the served page's`);
          want.push(links.pdf, links.text, links.records);
        }
      }
      app.pin(ribbonOf(document.getElementById("chart"), ERAF, PROPERTY).__data__);
      const got = [...document.querySelectorAll("#detail .prov a")].map((a) => a.getAttribute("href"));
      t.diagnostic(`ERAF cites ${JSON.stringify(printed.locators)}: ${got.join(" ")}`);
      assert.deepEqual(got, want);
    });
  });
}

describe("a figure whose words or pages the page does not carry", () => {
  test("a cited page the export built no links for is refused, not dropped from the Sources", async (t) => {
    const config = structuredClone(PAGE.config);
    const { app } = await bootedApp({ checkedStem: "sankey", config });
    const [printed] = columnLinks("fy2026-adopted", "fund-flows", (from, to) => from === ERAF && to === PROPERTY);
    const loc = printed.locators[0];
    const page = String(loc.pages[0]);
    assert.ok(app.citations(printed.locators).length > 0);
    delete config.docs[loc.doc_id].pages[page];
    t.diagnostic(`ERAF cites ${loc.doc_id} p${page}; with its entry deleted, citations() is asked again`);
    assert.throws(() => app.citations(printed.locators), new RegExp(`cannot cite ${loc.doc_id} p${page}`));
  });

  test("a link kind with no label is refused, and a gap's empty kind has no words", async () => {
    const config = structuredClone(PAGE.config);
    const { app } = await bootedApp({ checkedStem: "sankey", config });
    const [kind] = Object.keys(config.kind_labels);
    assert.ok(app.kindLabel(kind));
    assert.equal(app.kindLabel(""), "");
    delete config.kind_labels[kind];
    assert.throws(() => app.kindLabel(kind), new RegExp(`cannot name link kind ${kind}`));
  });
});
