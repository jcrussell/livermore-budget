// year.test.mjs — the FY2026/FY2027 toggle.
//
// The packager builds every year's words in Go and the client only chooses
// between them, so the one thing worth testing above all others is that the
// client reads the fields the packager writes: a wrong field name renders an
// empty tile and throws nothing. Go pins its half of that contract
// (TestTheSchemaStatesWhatThePageConfigCarries); this pins the client's, over
// the pinned page and the pinned columns.

import { describe, test } from "node:test";
import assert from "node:assert/strict";

import {
  loadApp, bootedApp, settle, opened, pageFixture, columnFixture, refusals, clickYear,
} from "./testlib.mjs";

/** The pinned page's newer year, which the tests below paint and repaint. */
function shippedYear(overrides) {
  const years = pageFixture().config.years;
  return Object.assign(structuredClone(years[years.length - 1]), overrides);
}

/** n distinct caveat refs, whose ids differ so the anchors do too. */
function fixtureCaveats(n) {
  return Array.from({ length: n }, (_, i) => ({
    id: "c" + i, summary: "summary " + i, href: "caveats.html#caveat-x--c" + i,
  }));
}

/** Everything the page shows after paintYearWords, read off the real DOM. */
function painted(app, document, year) {
  app.paintYearWords(year);
  const el = (id) => document.getElementById(id);
  return {
    hero: el("hero") ? [...el("hero").children] : [],
    tiles: el("figures") ? [...el("figures").children] : [],
    caveats: el("caveats") ? [...el("caveats").children].map((li) => {
      const a = li.querySelector("a");
      return a ? { text: a.textContent, href: a.getAttribute("href") || "" } : { text: li.textContent, href: "" };
    }) : [],
    caveatsCount: el("caveats-count") ? el("caveats-count").textContent : "",
    lede: el("lede-year") ? el("lede-year").textContent : "",
    counts: el("counts-line") ? el("counts-line").textContent : "",
    basis: el("page-basis") ? el("page-basis").textContent : "",
    chartTitle: el("chart-title") ? el("chart-title").textContent : "",
    title: document.title,
    dataPath: document.querySelector("[data-year-path] a"),
  };
}


/** The counts sentence the SERVED page carries for its opening year, as a shape. */
function servedCountsHead(document) {
  return document.getElementById("counts-line").textContent;
}

describe("a year's words are the packager's, painted whole", () => {
  test("a year switch paints one tile per figure and no more", async (t) => {
    const { app, document } = await loadApp();
    const year = shippedYear();
    const got = painted(app, document, year);
    t.diagnostic(`${got.tiles.length} tiles in #figures for ${year.figures.length} figures`);
    assert.equal(got.tiles.length, year.figures.length);
  });
  test("the hero is painted into #hero, alone, and not into the tile row", async (t) => {
    const { app, document } = await loadApp();
    const year = shippedYear();
    const got = painted(app, document, year);
    const text = (e) => [...e.children].map((c) => c.textContent).join("|");
    t.diagnostic(`#hero holds ${got.hero.length} tile(s): ${got.hero.map(text).join(" / ") || "nothing"}`);
    assert.equal(got.hero.length, 1);
    assert.ok(text(got.hero[0]).startsWith(year.hero.label));
    assert.ok(got.tiles.every((e) => !text(e).startsWith(year.hero.label)));
  });
  test("every tile carries the label, value and note the packager wrote", async (t) => {
    const { app, document } = await loadApp();
    const got = painted(app, document, shippedYear());
    const text = (e) => [...e.children].map((c) => c.textContent);
    const all = got.hero.concat(got.tiles).map(text);
    t.diagnostic(all[0] ? all[0].join("|") : "no tiles were painted");
    assert.ok(all.length > 0);
    for (const parts of all) {
      assert.equal(parts.length, 3);
      for (const x of parts) assert.ok(x !== "" && x !== "undefined", parts.join("|"));
    }
  });
  test("the caveats are the year's own summaries, linked to the year's own anchors", async (t) => {
    const { app, document } = await loadApp();
    const year = shippedYear();
    const got = painted(app, document, year);
    t.diagnostic(`${got.caveats.length} caveats; first links to "${got.caveats[0]?.href}"`);
    assert.equal(got.caveats.length, year.caveats.length);
    got.caveats.forEach((c, i) => {
      assert.equal(c.text, year.caveats[i].summary);
      assert.equal(c.href, year.caveats[i].href);
    });
  });
  test("the caveat count in the summary follows the year, and agrees with the list", async (t) => {
    const { app, document } = await loadApp();
    painted(app, document, shippedYear());
    const after = painted(app, document, shippedYear({ caveats: fixtureCaveats(5) }));
    t.diagnostic(`after repainting with five caveats the summary reads "${after.caveatsCount}" over a list of ${after.caveats.length}`);
    assert.equal(after.caveatsCount, "5");
    assert.equal(after.caveatsCount, String(after.caveats.length));
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
  test("the footer's basis follows the year", async (t) => {
    const { app, document } = await loadApp();
    const got = painted(app, document, shippedYear({ basis: "proposed" }));
    t.diagnostic(`#page-basis reads "${got.basis}"`);
    assert.equal(got.basis, "proposed");
  });
  test("a caller's own title is not overwritten by the year switch", async () => {
    const { app, document } = await loadApp();
    assert.equal(painted(app, document, shippedYear({ title: "A title the caller chose" })).title, "A title the caller chose");
  });
  test("a single-year build has no year control, and that is not an error", async () => {
    const { app, document } = await loadApp();
    document.getElementById("year-toggle").remove();
    app.wireYears([shippedYear()]);
    assert.equal(app.maybeEl("year-toggle"), null);
  });
  test("the footer's data-file citation follows the year", async (t) => {
    const { app, document } = await loadApp();
    const year = shippedYear();
    const got = painted(app, document, year);
    t.diagnostic(`href ${got.dataPath?.getAttribute("href")}, text "${got.dataPath?.textContent}"`);
    assert.equal(got.dataPath.getAttribute("href"), year.path);
    assert.equal(got.dataPath.textContent, year.path);
  });
  test("painting a second year replaces the first year's words", async (t) => {
    const { app, document } = await loadApp();
    const years = pageFixture().config.years;
    const first = painted(app, document, Object.assign(structuredClone(years[0]), { caveats: fixtureCaveats(2) }));
    const year = shippedYear();
    const second = painted(app, document, year);
    t.diagnostic(`${first.tiles.length} then ${second.tiles.length} tiles; ${first.caveats.length} then ${second.caveats.length} caveats`);
    assert.equal(second.tiles.length, first.tiles.length);
    assert.equal(second.hero.length, 1);
    assert.equal(first.hero.length, 1);
    assert.equal(first.caveats.length, 2);
    assert.deepEqual(second.caveats.map((c) => c.text), year.caveats.map((c) => c.summary));
    assert.equal(second.lede, year.label + " " + year.basis);
    assert.ok(second.title.includes(year.label));
  });
});

describe("a year switch repaints the drawn page", () => {
  test("a year switch replaces the flow table's rows rather than appending them", async (t) => {
    const { app, document } = await bootedApp({ checkedStem: "sankey" });
    const body = document.querySelector("#flow-table tbody");
    const links = app.projection.links.length;
    const afterFirst = body.children.length;
    clickYear(document, "sankey-2027");
    await settle();
    const afterSecond = body.children.length;
    t.diagnostic(`${afterFirst} rows for ${links} links, ${afterSecond} after switching year`);
    assert.equal(afterFirst, links);
    assert.equal(afterSecond, app.projection.links.length);
    assert.ok(afterSecond < afterFirst * 2);
  });
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
    assert.equal(before, "Dark mode / aria-pressed=false");
    assert.equal(after, "Light mode / aria-pressed=true");
  });
  test("a click on the theme button switches the page and stores the choice", async (t) => {
    const { document, window } = await bootedApp();
    const button = document.getElementById("theme-toggle");
    const before = document.documentElement.dataset.theme || "";
    button.click();
    await settle();
    const after = document.documentElement.dataset.theme || "";
    const stored = window.localStorage.getItem("fisc-theme");
    t.diagnostic(`data-theme "${before}" -> "${after}", stored "${stored}", button reads "${button.textContent}"`);
    assert.notEqual(after, before);
    assert.equal(stored, after);
    assert.equal(button.getAttribute("aria-pressed"), after === "dark" ? "true" : "false");
  });
});

describe("a year switch and an open drill", () => {
  test("the year on screen opens into its own step document, and a drill asks for nothing", async (t) => {
    const other = columnFixture("fy2027-adopted");
    other.nodes.find((n) => n.id === "fund/100").label = "General Fund, the other year";
    const { app, document, fetch } = await bootedApp({ checkedStem: "sankey", plan: { "fy2027-adopted.json": { doc: other } } });
    await opened(app, "fund-group/general");
    clickYear(document, "sankey-2027");
    await settle();
    await opened(app, "fund-group/general");
    const label = app.projection.nodes.find((n) => n.id === "fund/100")?.label;
    const unique = [...new Set(fetch.asked)].sort();
    t.diagnostic(`two drills and a year switch asked for [${unique}] and drew fund/100 labelled "${label}"`);
    assert.deepEqual(unique, ["fy2026-adopted.json", "fy2027-adopted.json", "rungs.json"]);
    assert.equal(label, "General Fund, the other year");
  });
  test("a year switch closes every rung, and the next drill draws the new year's schedule", async (t) => {
    const { app, document } = await bootedApp({ checkedStem: "sankey" });
    await opened(app, "fund-group/general");
    const depthBefore = app.drilled.length;
    clickYear(document, "sankey-2027");
    await settle();
    const crumbHidden = document.getElementById("breadcrumb").hasAttribute("hidden");
    const depthAfter = app.drilled.length;
    const lede = document.getElementById("lede-year").textContent;
    await opened(app, "fund-group/general");
    t.diagnostic(`opened to depth ${depthBefore}, switched year and read "${lede}" at depth ${depthAfter} with the breadcrumb ${crumbHidden ? "hidden" : "SHOWING"}`);
    assert.equal(depthBefore, 1);
    assert.equal(depthAfter, 0);
    assert.ok(crumbHidden);
    assert.equal(lede, "FY 2026-27 adopted");
    assert.equal(refusals(document).length, 0);
  });
  test("a drill overtaken by a year switch stands down rather than landing on the new year", async (t) => {
    let release = null;
    const { app, document } = await bootedApp({ checkedStem: "sankey", plan: { "fy2027-adopted.json": { settle: (pair) => { release = pair; } } } });
    const inFlight = app.drillDown("fund-group/general");
    clickYear(document, "sankey-2027");
    await settle();
    assert.ok(release, "the year fetch was never asked for");
    release.resolve(columnFixture("fy2027-adopted"));
    const outcome = await inFlight;
    await settle();
    const lede = document.getElementById("lede-year").textContent;
    t.diagnostic(`the year switched to "${lede}"; the drill came to "${outcome}" and left ${app.drilled.length} rung(s)`);
    assert.equal(lede, "FY 2026-27 adopted");
    assert.equal(outcome, "superseded");
    assert.equal(app.drilled.length, 0);
  });
  test("a drill begun after a year switch draws on the column it was begun on, and the switch replaces it whole", async (t) => {
    let column = null;
    const { app, document } = await bootedApp({ checkedStem: "sankey", plan: { "fy2027-adopted.json": { settle: (pair) => { column = pair; } } } });
    clickYear(document, "sankey-2027");
    await settle();
    assert.ok(column, "the year fetch was never asked for");
    const inFlight = app.drillDown("fund-group/general");
    await settle();
    column.resolve(columnFixture("fy2027-adopted"));
    const outcome = await inFlight;
    await settle();
    const lede = document.getElementById("lede-year").textContent;
    t.diagnostic(`the drill came to "${outcome}" and the landing year left ${app.drilled.length} rung(s), with the page reading "${lede}"`);
    assert.equal(lede, "FY 2026-27 adopted");
    assert.equal(outcome, "drew");
    assert.equal(app.drilled.length, 0);
  });
  test("a year switch from a four-column rung says the width the overview draws", async (t) => {
    const { app, document } = await bootedApp({ checkedStem: "sankey", viewport: 2000 });
    const control = () => ["column-count", "column-more", "column-fewer"].map((id) => {
      const el = document.getElementById(id);
      return id === "column-count" ? el.textContent : (el.hasAttribute("disabled") ? "disabled" : "live");
    }).join(" / ");
    const overview = control();
    await opened(app, "fund-group/general", "fund/100");
    const drilled = control();
    clickYear(document, "sankey-2027");
    await settle();
    const switched = control();
    t.diagnostic(`count / more / fewer: overview "${overview}", fund/100 "${drilled}", after the switch "${switched}" over ${app.drawnColumns()} drawn column(s)`);
    assert.equal(drilled, "4 columns / disabled / live");
    assert.equal(app.drilled.length, 0);
    assert.equal(switched, overview);
    assert.equal(switched, app.drawnColumns() + " columns / disabled / disabled");
  });
  test("a year whose chart will not lay out leaves the page on the year it was, stack and all", async (t) => {
    // A link whose target is past the end of the node table: selectSchedule
    // takes it, and the lay-out throws on it.
    const broken = structuredClone(columnFixture("fy2027-adopted"));
    broken.schedules.sankey.links[0].to = broken.nodes.length;
    const { app, document } = await bootedApp({ checkedStem: "sankey", plan: { "fy2027-adopted.json": { doc: broken } } });
    const cents = () => app.projection.links.reduce((a, l) => a + l.value_cents, 0);
    const overview = cents();
    await opened(app, "fund-group/general");
    const title = document.title;
    clickYear(document, "sankey-2027");
    await settle();
    const after = {
      path: app.drilled.map((r) => r.id).join(" > "), title: document.title,
      year: app.column.column.fiscal_year, crumb: !document.getElementById("breadcrumb").hasAttribute("hidden"),
      banners: refusals(document).map((b) => b.textContent),
    };
    document.querySelector("#breadcrumb button").click();
    await settle();
    t.diagnostic(`after the refused switch: stack "${after.path}", title "${after.title}", column FY${after.year}, breadcrumb ${after.crumb ? "shown" : "hidden"}, banners ${JSON.stringify(after.banners)}; back to the overview drew ${cents()} cents against ${overview} at boot`);
    assert.equal(after.banners.length, 1);
    assert.equal(after.path, "fund-group/general");
    assert.equal(after.title, title);
    assert.equal(after.year, 2026);
    assert.ok(after.crumb);
    assert.equal(app.drilled.length, 0);
    assert.equal(cents(), overview);
  });
  test("a year whose table will not build leaves the lay-out of the year it was", async (t) => {
    // Only citations reads CONFIG.docs unguarded, so the throw is the table's.
    const config = structuredClone(pageFixture().config);
    const { app, document } = await bootedApp({ checkedStem: "sankey", config });
    const g = [...document.querySelectorAll("#chart g.node")].find((m) => m.__data__.id === "fund-group/general");
    app.pin(g.__data__);
    const read = () => ({
      share: app.columnShare(g.__data__), laid: app.laidNodes, groups: app.groupIndex,
      table: document.querySelector("#flow-table tbody").innerHTML, projection: app.projection,
      year: app.column.column.fiscal_year,
    });
    const before = read();
    delete config.docs;
    clickYear(document, "sankey-2027");
    await settle();
    const after = read();
    const banners = refusals(document).map((b) => b.textContent);
    t.diagnostic(`after the refused switch: FY${after.year}, share "${after.share}" against "${before.share}", banners ${JSON.stringify(banners)}`);
    assert.equal(banners.length, 1);
    assert.ok(before.share !== "");
    assert.equal(after.year, before.year);
    assert.equal(after.share, before.share);
    assert.equal(after.laid, before.laid);
    assert.equal(after.groups, before.groups);
    assert.equal(after.table, before.table);
    assert.equal(after.projection, before.projection);
  });
  test("a year switch drops the expansion with the rung it was made on", async (t) => {
    const { app, document } = await bootedApp({ checkedStem: "sankey" });
    const group = "fund-group/special-revenue";
    const funds = () => app.projection.nodes.filter((n) => n.tier === 3).length;
    const tail = () => app.projection.nodes.find((n) => app.isAggregate(n.id));
    await opened(app, group);
    const folded = { funds: funds(), tail: tail() ? tail().label : "" };
    app.expandTier(tail());
    await settle();
    const expanded = { funds: funds(), tail: tail() ? tail().label : "" };
    clickYear(document, "sankey-2027");
    await settle();
    const between = app.drilled.length;
    await opened(app, group);
    const after = { funds: funds(), tail: tail() ? tail().label : "" };
    t.diagnostic(`FY 2025-26 drew ${folded.funds} fund mark(s) with a tail "${folded.tail}"; expanded, ${expanded.funds} and ${expanded.tail ? "a tail" : "no tail"}; the switch left ${between} rung(s); FY 2026-27 drew ${after.funds} with a tail "${after.tail}"`);
    assert.ok(folded.tail !== "" && expanded.tail === "");
    assert.ok(expanded.funds > folded.funds);
    assert.equal(between, 0);
    assert.ok(after.tail !== "");
    assert.notEqual(after.tail, folded.tail);
  });
});

// fisc-7477: the arm a single-view export reaches, where there is no
// caveats.html for a caveat to link into.
describe("a caveat with no page to link to", () => {
  test("a caveat without an href is painted as plain text and not as an anchor into a file never written", async (t) => {
    const { app, document } = await loadApp();
    const year = shippedYear({ caveats: [{ id: "plain", summary: "plain summary", href: "" }, ...fixtureCaveats(1)] });
    const got = painted(app, document, year);
    t.diagnostic(got.caveats.map((c) => `"${c.text}" -> ${c.href || "(no link)"}`).join("; "));
    assert.equal(got.caveats.length, 2);
    assert.deepEqual(got.caveats[0], { text: "plain summary", href: "" });
    assert.equal(got.caveats[1].href, year.caveats[1].href);
    assert.equal(document.querySelectorAll("#caveats li a").length, 1);
  });
});
