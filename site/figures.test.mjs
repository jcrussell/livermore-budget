// figures.test.mjs — the dollar figures and citations a reader is shown, held
// against the cents and locators Go pinned in testdata/ and formatted by the
// test, never by app.js.
//
// A few figures are also written out literally, pinning the test's own
// formatter: ERAF's as the city printed it, Property Taxes' as the sum of rows it printed.

import { describe, test } from "node:test";
import assert from "node:assert/strict";
import {
  bootedApp, split, settle, clickYear, columnFixture, pageFixture, dollars, shortDollars, everySplit, revealed,
} from "./testlib.mjs";

const PAGE = pageFixture();
const PROPERTY = "revenue/taxes/property";
/** A line of Property Taxes printed as a reduction: split, its ribbon runs into its category. */
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
      await split(app, PROPERTY);
      const [printed] = columnLinks(year.column, "expansion", (from) => from === ERAF);
      t.diagnostic(`ERAF out of ${PROPERTY} is ${printed.value_cents} cents in the pinned column`);
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
    });

    test(`${year.column}: a node's share of its column is its cited cents over the column's, and each mark is laid at its cents`, async (t) => {
      const { app, document } = await onYear(year.stem);
      const chart = document.getElementById("chart");
      const d = markOf(chart, PROPERTY).__data__;
      const column = [...chart.querySelectorAll("g.node")].map((g) => g.__data__).filter((n) => n.layer === d.layer);
      assert.ok(column.length >= 2);
      // The share is defined over the drawn values, so the expectation is
      // taken from the pinned column's cents and each drawn value is held to
      // its mark's: the larger side of its ribbons, which is how a mark is
      // sized.
      const centsOf = (id) => {
        const side = (end) => columnLinks(year.column, "sankey", (from, to) => (end === "from" ? from : to) === id)
          .reduce((s, l) => s + l.value_cents, 0);
        return Math.max(side("from"), side("to"));
      };
      for (const n of column) {
        assert.equal(n.value, centsOf(n.id), `${n.id} is laid at ${n.value}, not the cents its ribbons add to`);
      }
      const cents = centsOf(PROPERTY);
      const total = column.reduce((s, n) => s + centsOf(n.id), 0);
      const want = `◇ our ${((100 * cents) / total).toFixed(1)}% of this column`;
      t.diagnostic(`${PROPERTY} is ${cents} of ${total} cents over ${column.length} drawn marks: "${want}"`);
      app.pin(d);
      const chips = [...document.querySelectorAll("#detail .chip.derived")].map((c) => c.textContent);
      assert.ok(chips.includes(want), JSON.stringify(chips));
    });

    test(`${year.column}: a printed reduction states no share of its column and is in no other mark's total`, async (t) => {
      const { app, document } = await onYear(year.stem);
      await split(app, PROPERTY);
      const chart = document.getElementById("chart");
      const eraf = markOf(chart, ERAF).__data__;
      // A reduction is read off the pinned column: every ribbon it leaves by is printed negative.
      const reduces = (id) => {
        const out = columnLinks(year.column, "expansion", (from) => from === id);
        return out.length > 0 && out.every((l) => l.value_cents < 0);
      };
      assert.ok(reduces(ERAF));
      const column = [...chart.querySelectorAll("g.node")].map((g) => g.__data__).filter((n) => n.layer === eraf.layer);
      const reductions = column.filter((n) => reduces(n.id));
      const parts = column.filter((n) => !reduces(n.id));
      const total = parts.reduce((s, n) => s + n.value, 0);
      t.diagnostic(`${reductions.length} reduction(s) and ${parts.length} part(s) in ERAF's column; the parts total ${total} cents`);
      assert.ok(parts.length >= 2);
      for (const n of reductions) {
        assert.equal(app.shareOf(n), "", `${n.id} states a share`);
        app.showTip({ target: chart, clientX: 0, clientY: 0 }, n);
        const tip = document.getElementById("tooltip").textContent;
        app.pin(n);
        const panel = document.getElementById("detail").textContent;
        assert.ok(!tip.includes("of this column") && !panel.includes("of this column"), `${n.id}: ${tip} | ${panel}`);
      }
      for (const n of parts) {
        const pct = (100 * n.value) / total;
        const shown = pct < 0.1 ? "<0.1" : pct > 99.9 ? ">99.9" : pct.toFixed(1);
        assert.equal(app.shareOf(n), `◇ our ${shown}% of this column`, n.id);
      }
    });

    test(`${year.column}: a ribbon's Sources are its own pages, at the file names Go wrote`, async (t) => {
      const { app, document } = await onYear(year.stem);
      await split(app, PROPERTY);
      const [printed] = columnLinks(year.column, "expansion", (from) => from === ERAF);
      const doc = PAGE.config.docs[printed.locators[0].doc_id];
      const want = [];
      for (const loc of printed.locators) {
        for (const page of loc.pages) {
          const links = doc.pages[String(page)];
          assert.ok(links, `the config carries no links for p${page}`);
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

  test("a link kind with no label is refused, the empty kind included", async () => {
    const config = structuredClone(PAGE.config);
    const { app } = await bootedApp({ checkedStem: "sankey", config });
    const [kind] = Object.keys(config.kind_labels);
    assert.ok(app.kindLabel(kind));
    assert.throws(() => app.kindLabel(""), /cannot name link kind/);
    delete config.kind_labels[kind];
    assert.throws(() => app.kindLabel(kind), new RegExp(`cannot name link kind ${kind}`));
  });
});

