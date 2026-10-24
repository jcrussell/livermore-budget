// year.test.mjs — the year select: that the client reads the fields the
// packager writes, since a wrong field name renders an empty headline and
// throws nothing.

import { describe, test } from "node:test";
import assert from "node:assert/strict";

import {
  loadApp, bootedApp, settle, split, pageFixture, columnFixture, refusals, clickYear, keyOfStem,
} from "./testlib.mjs";

/** The pinned page's newer year, which the tests below paint and repaint. */
function shippedYear(overrides) {
  const years = pageFixture().config.years;
  return Object.assign(structuredClone(years[years.length - 1]), overrides);
}

/** Everything the page shows after paintYearWords, read off the real DOM. */
function painted(app, document, year) {
  app.paintYearWords(year);
  const el = (id) => document.getElementById(id);
  return {
    hero: el("hero") ? el("hero").textContent : "",
    lede: el("lede-year") ? el("lede-year").textContent : "",
    counts: el("counts-line") ? el("counts-line").textContent : "",
    chartTitle: el("chart-title") ? el("chart-title").textContent : "",
    title: document.title,
  };
}


/** The counts sentence the SERVED page carries for its opening year, as a shape. */
function servedCountsHead(document) {
  return document.getElementById("counts-line").textContent;
}

describe("a year's words are the packager's, painted whole", () => {
  // Mutation: paint year.hero.note in place of its value -- red.
  test("the headline is the year's label and value, painted into #hero", async (t) => {
    const { app, document } = await loadApp();
    const year = shippedYear({ hero: Object.assign(structuredClone(shippedYear().hero), { value: "$1,234" }) });
    const got = painted(app, document, year);
    t.diagnostic(`#hero reads "${got.hero}"`);
    assert.equal(got.hero, year.hero.label + ": " + year.hero.value);
  });
  test("the page carries no tile row and no caveat list for a year switch to leave stale", async () => {
    const { document } = await loadApp();
    for (const id of ["figures", "caveats", "caveats-count"]) {
      assert.equal(document.getElementById(id), null, `#${id} is on the page`);
    }
  });
  test("the lede, the flow count and the document title follow the year", async (t) => {
    const { app, document } = await loadApp();
    const year = shippedYear();
    // THE COUNTS SENTENCE IS THE TEMPLATE'S: the served page renders it for the
    // opening year, which is this year, so the repaint must write the very
    // sentence the server wrote.
    const served = servedCountsHead(document);
    const got = painted(app, document, year);
    t.diagnostic(`lede "${got.lede}", counts "${got.counts}", title "${got.title}"`);
    assert.equal(got.lede, year.label + " " + year.basis);
    assert.equal(got.counts, served);
    assert.equal(got.title, year.title);
  });
  test("after a draw, the repainted counts sentence still opens with the template's own head", async (t) => {
    const { document } = await bootedApp({ checkedStem: "sankey" });
    // The served page's own sentence for its opening year; the drawn one for
    // the year that booted must open with the same words up to the numbers.
    const head = (s) => s.replace(/\d[\d,]*/g, "N");
    const drawn = servedCountsHead(document);
    const template = pageFixture().html.match(/<span id="counts-line">([\s\S]*?)<\/span>/)[1].replace(/\s+/g, " ").trim();
    t.diagnostic(`the drawn page's counts-line reads "${drawn}"`);
    assert.ok(drawn.startsWith(head(template).split("N")[0]), `"${drawn}" does not open with the template's head "${template}"`);
    // The tail is the wording's counts_partial past its {cited} placeholder,
    // with the numbers and the plural left open.
    const tail = pageFixture().config.wording.counts_partial.split("{cited}")[1];
    const pattern = tail.replace(/[.*+?^$()[\]\\]/g, "\\$&")
      .replace(/\{(\w+):([^|}]*)\|([^}]*)\}/g, "\\d+ (?:$2|$3)").replace(/\{\w+\}/g, "\\d+");
    assert.match(drawn, new RegExp(pattern + "$"));
  });
  test("the chart's accessible name is the packager's chart_title, written whole", async (t) => {
    const { app, document } = await loadApp();
    const year = shippedYear({ chart_title: "Sankey diagram by a subject only the packager knows" });
    const got = painted(app, document, year);
    t.diagnostic(`#chart-title reads "${got.chartTitle}"`);
    assert.equal(got.chartTitle, year.chart_title);
  });
  test("a caller's own title is not overwritten by the year switch", async () => {
    const { app, document } = await loadApp();
    assert.equal(painted(app, document, shippedYear({ title: "A title the caller chose" })).title, "A title the caller chose");
  });
  test("a single-year build has no year control, and that is not an error", async () => {
    const { app, document } = await loadApp();
    document.getElementById("year-select").closest("label").remove();
    app.wireYears([shippedYear()]);
    assert.equal(app.maybeEl("year-select"), null);
  });
  test("painting a second year replaces the first year's words", async (t) => {
    const { app, document } = await loadApp();
    const years = pageFixture().config.years;
    const first = painted(app, document, structuredClone(years[0]));
    const year = shippedYear();
    const second = painted(app, document, year);
    t.diagnostic(`"${first.hero}" then "${second.hero}"`);
    assert.equal(first.hero, years[0].hero.label + ": " + years[0].hero.value);
    assert.equal(second.hero, year.hero.label + ": " + year.hero.value);
    assert.equal(second.lede, year.label + " " + year.basis);
    assert.ok(second.title.includes(year.label));
  });
});

describe("a year switch repaints the drawn page", () => {

  test("paint() rewrites every legend swatch's background", async (t) => {
    const { app, document } = await bootedApp({ checkedStem: "sankey" });
    const swatches = [...document.querySelectorAll("#legend button .key")];
    for (const sw of swatches) sw.style.background = "";
    app.paint();
    const repainted = swatches.filter((sw) => sw.style.background);
    t.diagnostic(`${repainted.length} of ${swatches.length} swatch(es) recoloured from the palette`);
    assert.ok(swatches.length > 0);
    assert.equal(repainted.length, swatches.length);
  });
});

describe("which year the page opens on", () => {
  const lede = (document) => document.getElementById("lede-year").textContent;
  test("the page opens on the newest year, not the first one listed", async (t) => {
    const { document, config } = await bootedApp();
    const years = config.years;
    const newest = years[years.length - 1];
    t.diagnostic(`years are listed ${JSON.stringify(years.map((y) => y.label))}; the page drew "${lede(document)}"`);
    assert.notEqual(years[0].stem, newest.stem);
    assert.equal(lede(document), `${newest.label} ${newest.basis}`);
  });
  test("a restore naming a year this config no longer publishes falls back to the newest", async (t) => {
    const { document, config } = await bootedApp({ checkedStem: "sankey-2019" });
    const newest = config.years[config.years.length - 1];
    t.diagnostic(`the page drew "${lede(document)}"`);
    assert.equal(lede(document), `${newest.label} ${newest.basis}`);
  });
  test("a reader's own selection still outranks the newest", async (t) => {
    const { document, config, fetch } = await bootedApp({ checkedStem: "sankey" });
    const first = config.years[0];
    t.diagnostic(`checked on ${first.stem}, the page drew "${lede(document)}" and asked for ${JSON.stringify(fetch.asked)}`);
    assert.equal(lede(document), `${first.label} ${first.basis}`);
    assert.ok(fetch.asked.includes(first.path));
  });
  test("a restored year selection is the year the page opens on, and its document is the one fetched", async (t) => {
    const { document, config, fetch } = await bootedApp({ checkedStem: "sankey-2027" });
    const second = config.years[1];
    t.diagnostic(`the page drew "${lede(document)}"; main() asked for ${JSON.stringify(fetch.asked)}`);
    assert.equal(lede(document), `${second.label} ${second.basis}`);
    assert.ok(fetch.asked.includes(second.path));
  });
});

describe("the theme control", () => {
  test("the theme button follows an OS theme change", async (t) => {
    const { document, media } = await bootedApp();
    const button = document.getElementById("theme-toggle");
    const read = () => `${button.textContent} / aria-pressed=${button.getAttribute("aria-pressed")}`;
    const before = read();
    media.setOSDark(true);
    await settle();
    const after = read();
    t.diagnostic(`the button read "${before}" on a light page and "${after}" after the OS switched to dark`);
    assert.equal(before, "Dark mode / aria-pressed=null");
    assert.equal(after, "Light mode / aria-pressed=null");
  });
  test("a click on the theme button switches the page, stores the choice and repaints the chart", async (t) => {
    const { document, window } = await bootedApp();
    const button = document.getElementById("theme-toggle");
    // d3 wrote resolved hues onto the ribbons, so a theme that only moved the
    // stylesheet would leave them in the old one.
    const hues = () => Array.from(document.querySelectorAll("#chart path.link"), (p) => p.getAttribute("stroke")).join(" ");
    const before = document.documentElement.dataset.theme || "";
    const painted = hues();
    button.click();
    await settle();
    const after = document.documentElement.dataset.theme || "";
    const stored = window.localStorage.getItem("fisc-theme");
    const repainted = hues();
    t.diagnostic(`data-theme "${before}" -> "${after}", stored "${stored}", button reads "${button.textContent}"; ` +
      `first ribbon stroke ${painted.split(" ")[0]} -> ${repainted.split(" ")[0]}`);
    assert.notEqual(after, before);
    assert.equal(stored, after);
    assert.equal(button.textContent, after === "dark" ? "Light mode" : "Dark mode");
    assert.equal(button.getAttribute("aria-pressed"), null);
    assert.ok(painted !== "", "the chart drew no ribbon to repaint");
    assert.ok(repainted !== painted, "the ribbons kept the hues of the theme the page left");
  });
});

describe("a year switch and the splits on screen", () => {
  test("the year on screen splits its own expansion, and a split asks for nothing", async (t) => {
    const other = columnFixture("fy2027-adopted");
    other.nodes.find((n) => n.id === "fund/100").label = "General Fund, the other year";
    const { app, document, fetch } = await bootedApp({ checkedStem: "sankey", plan: { "fy2027-adopted.json": { doc: other } } });
    await split(app, "fund-group/general");
    clickYear(document, "sankey-2027");
    await settle();
    await split(app, "fund-group/general");
    const label = app.projection.nodes.find((n) => n.id === "fund/100")?.label;
    const unique = [...new Set(fetch.asked)].sort();
    t.diagnostic(`two splits and a year switch asked for [${unique}] and drew fund/100 labelled "${label}"`);
    assert.deepEqual(unique, ["fy2026-adopted.json", "fy2027-adopted.json"]);
    assert.equal(label, "General Fund, the other year");
  });
  test("a year switch keeps every split the new year draws, drawn from the new year's schedule", async (t) => {
    const { app, document } = await bootedApp({ checkedStem: "sankey" });
    await split(app, "fund-group/general");
    const before = app.expansions.slice();
    clickYear(document, "sankey-2027");
    await settle();
    const collapsible = !document.getElementById("chart-collapse-all").disabled;
    const after = app.expansions.slice();
    const lede = document.getElementById("lede-year").textContent;
    await split(app, "fund/100");
    t.diagnostic(`split [${before.join(", ")}], switched year and read "${lede}" with [${after.join(", ")}] split; Collapse all live: ${collapsible}`);
    assert.deepEqual(before, ["fund-group/general"]);
    assert.deepEqual(after, ["fund-group/general"]);
    assert.equal(collapsible, true);
    assert.equal(lede, "FY 2026-27 adopted");
    assert.equal(app.fetched.metadata.fiscal_year, 2027);
    assert.ok(document.getElementById("splits-line").hidden, "a switch that dropped nothing says it did");
    assert.equal(refusals(document).length, 0);
  });
  test("a split made while a year is fetched is replaced whole when the year lands", async (t) => {
    let column = null;
    const { app, document } = await bootedApp({ checkedStem: "sankey", plan: { "fy2027-adopted.json": { settle: (pair) => { column = pair; } } } });
    clickYear(document, "sankey-2027");
    await settle();
    assert.ok(column, "the year fetch was never asked for");
    await split(app, "fund-group/general");
    const during = { year: app.fetched.metadata.fiscal_year, splits: app.expansions.length };
    column.resolve(columnFixture("fy2027-adopted"));
    await settle();
    const lede = document.getElementById("lede-year").textContent;
    t.diagnostic(`split on FY${during.year} while the next year was fetched; it landed reading "${lede}" with ${app.expansions.length} split(s)`);
    assert.deepEqual(during, { year: 2026, splits: 1 });
    assert.equal(lede, "FY 2026-27 adopted");
    assert.equal(app.fetched.metadata.fiscal_year, 2027);
    assert.equal(app.expansions.length, 0);
  });
  test("a year whose chart will not lay out leaves the page on the year it was, splits and all", async (t) => {
    // A link whose target is past the end of the node table: selectSchedule
    // takes it, and the lay-out throws on it.
    const broken = structuredClone(columnFixture("fy2027-adopted"));
    broken.schedules.expansion.links[0].to = broken.nodes.length;
    const { app, document } = await bootedApp({ checkedStem: "sankey", plan: { "fy2027-adopted.json": { doc: broken } } });
    const cents = () => app.projection.links.reduce((a, l) => a + l.value_cents, 0);
    const root = cents();
    await split(app, "fund-group/general");
    const title = document.title;
    clickYear(document, "sankey-2027");
    await settle();
    const after = {
      splits: app.expansions.join(" > "), title: document.title,
      year: app.column.column.fiscal_year, collapsible: !document.getElementById("chart-collapse-all").disabled,
      banners: refusals(document).map((b) => b.textContent),
    };
    document.getElementById("chart-collapse-all").click();
    await settle();
    t.diagnostic(`after the refused switch: splits "${after.splits}", title "${after.title}", column FY${after.year}, Collapse all live: ${after.collapsible}, banners ${JSON.stringify(after.banners)}; folded back, the chart draws ${cents()} cents against ${root} at boot`);
    assert.equal(after.banners.length, 1);
    assert.equal(after.splits, "fund-group/general");
    assert.equal(after.title, title);
    assert.equal(after.year, 2026);
    assert.equal(after.collapsible, true);
    assert.equal(app.expansions.length, 0);
    assert.equal(cents(), root);
  });
  test("a refused year switch puts the year control back on the year still shown", async (t) => {
    const broken = structuredClone(columnFixture("fy2027-adopted"));
    broken.schedules.expansion.links[0].to = broken.nodes.length;
    const checked = (document) => document.getElementById("year-select").value;
    for (const [how, entry] of [["a 404", null], ["a lay-out that throws", { doc: broken }]]) {
      const { document } = await bootedApp({ checkedStem: "sankey", plan: { "fy2027-adopted.json": entry } });
      clickYear(document, "sankey-2027");
      await settle();
      t.diagnostic(`after ${how}: the control reads "${checked(document)}", ${refusals(document).length} banner(s)`);
      assert.equal(checked(document), keyOfStem("sankey"), how);
      assert.equal(refusals(document).length, 1, how);
    }
  });

  test("a year drawn is carried onto the top bar's links whose view offers it, and onto no other", async (t) => {
    const { document } = await bootedApp({ checkedStem: "sankey" });
    const href = (path) => document.querySelector(`nav.site-nav a[href^="${path}"]`).getAttribute("href");
    const select = document.getElementById("year-select");
    const opened = { ledger: href("ledger.html"), capital: href("capital.html"), change: href("change.html"), disabled: select.disabled };
    clickYear(document, "sankey-2027");
    await settle();
    const switched = { ledger: href("ledger.html"), department: href("department.html"), change: href("change.html") };
    t.diagnostic(`opened: ${JSON.stringify(opened)}; switched: ${JSON.stringify(switched)}`);
    assert.equal(opened.disabled, false, "the select ships disabled and the page never enabled it");
    assert.equal(opened.ledger, "ledger.html#y=" + keyOfStem("sankey"));
    assert.equal(opened.capital, "capital.html#y=" + keyOfStem("sankey"));
    assert.equal(opened.change, "change.html", "a view drawing every year was handed one");
    assert.equal(switched.ledger, "ledger.html#y=" + keyOfStem("sankey-2027"));
    assert.equal(switched.department, "department.html#y=" + keyOfStem("sankey-2027"));
    assert.equal(switched.change, "change.html");
  });

  test("a year switch keeps a tail drawn out, drawing the new year's members whole", async (t) => {
    const { app, document } = await bootedApp({ checkedStem: "sankey" });
    const group = "fund-group/special-revenue";
    const funds = () => app.projection.nodes.filter((n) => n.tier === 3).length;
    const tail = () => app.projection.nodes.find((n) => app.isAggregate(n.id));
    await split(app, group);
    const folded = { funds: funds(), tail: tail() ? tail().label : "" };
    app.drawOutTail(tail());
    await settle();
    const expanded = { funds: funds(), tail: tail() ? tail().label : "" };
    clickYear(document, "sankey-2027");
    await settle();
    const after = { funds: funds(), tail: tail() ? tail().label : "", x: app.expansions.length };
    const members = app.splitsHere({ id: group })[0].members.length;
    t.diagnostic(`FY 2025-26 drew ${folded.funds} fund mark(s) with a tail "${folded.tail}"; drawn out, ${expanded.funds}; ` +
      `FY 2026-27 kept ${after.x} expansion(s) and drew ${after.funds} of the group's ${members} funds with ${after.tail ? "a tail" : "no tail"}`);
    assert.ok(folded.tail !== "" && expanded.tail === "");
    assert.ok(expanded.funds > folded.funds);
    assert.equal(after.x, 2);
    assert.equal(after.tail, "");
    assert.equal(after.funds, members);
  });
});
