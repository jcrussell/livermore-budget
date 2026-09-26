// lifecycle.test.mjs — what happens while a year is IN FLIGHT: the fetch, the
// switch token, and what main() does with each answer. Every test here boots
// the real page over Go's pinned artifacts and drives the year control with
// the event a browser fires.
//
// NOT HERE: the column budget (columns.test.mjs) and what a drawn year SAYS
// (year.test.mjs). A served document is never assembled by hand: a malformed
// one is a structuredClone of a pinned column with one stated key changed.

import { describe, test } from "node:test";
import assert from "node:assert/strict";
import {
  loadApp, settle, refusals, pageFixture, columnFixture, rungsFixture, clickYear, keydownListeners, listenerErrors,
} from "./testlib.mjs";

const FIRST = "sankey";
const SECOND = "sankey-2027";
const FIRST_LEDE = "FY 2025-26 adopted";


const lede = (document) => document.getElementById("lede-year").textContent;
const marks = (document) => document.querySelectorAll("#chart g.node").length;
const rows = (document) => document.querySelector("#flow-table tbody").children.length;
const said = (document) => refusals(document).map((b) => b.textContent);

/**
 * Whether `stem` is the year on screen: shownYear names it and marks are
 * drawn. The lede alone cannot say so, because the served page already reads
 * the newest year's words before anything has drawn.
 */
const drawn = (app, document, stem) =>
  Boolean(app.shownYear) && app.shownYear.stem === stem && marks(document) > 0;



/** The page loaded with the reader on the FIRST year and booted to rest. */
async function onFirstYear(o = {}) {
  const loaded = await loadApp({ checkedStem: FIRST, ...o });
  await loaded.app.boot();
  await settle();
  return loaded;
}

/** The pinned second column with `mutate` applied to a clone of it. */
function secondColumn(mutate) {
  const col = structuredClone(columnFixture("fy2027-adopted"));
  mutate(col);
  return col;
}

describe("the version handshake", () => {
  test("a page packaged for another schema_version is refused before anything is fetched", async (t) => {
    const asked = async (v) => {
      const config = structuredClone(pageFixture().config);
      config.schema_version = v;
      const { app, document, fetch } = await loadApp({ config });
      await app.boot();
      await settle();
      return { fetched: fetch.asked.length, banners: said(document) };
    };
    const ok = (await loadApp()).app.SCHEMA_VERSION;
    assert.equal(pageFixture().config.schema_version, ok, "the pinned page carries the script's schema_version");
    const good = await asked(ok);
    const newer = await asked(ok + 1);
    const older = await asked(ok - 1);
    t.diagnostic(`schema_version ${ok} fetches ${good.fetched} file(s); ${ok + 1} and ${ok - 1} ` +
      `fetch ${newer.fetched} and ${older.fetched}; the newer page says ${JSON.stringify(newer.banners)}`);
    assert.ok(good.fetched > 0);
    assert.equal(good.banners.length, 0);
    assert.equal(newer.fetched, 0);
    assert.equal(older.fetched, 0);
    assert.equal(newer.banners.length, 1);
    assert.match(newer.banners[0], new RegExp(`packaged for schema_version ${ok + 1} `));
    assert.match(newer.banners[0], /newer than this script/);
    assert.equal(older.banners.length, 1);
    assert.match(older.banners[0], new RegExp(`packaged for schema_version ${ok - 1} `));
    assert.match(older.banners[0], /older than this script/);
  });
});

describe("a click during the opening fetch", () => {
  // The opening fetch never settles and the reader clicks the other year.
  // wireYears removes the control's `disabled` before main() awaits, so the
  // control is live for the whole of the first fetch.
  async function clickedAway() {
    const loaded = await loadApp({ checkedStem: FIRST, plan: { "fy2026-adopted.json": { hang: true } } });
    const keydown = keydownListeners(loaded.document);
    const errors = listenerErrors(loaded.window);
    // Not awaited: boot() awaits the fetch that never settles.
    void loaded.app.boot();
    await settle();
    clickYear(loaded.document, SECOND);
    await settle();
    const opened = loaded.fetch.asked.includes(loaded.config.years[0].path);
    return { ...loaded, keydown, errors, opened };
  }

  test("a click during the opening fetch leaves the page's keyboard and theme wiring intact", async (t) => {
    const { app, document, media, fetch, keydown, opened } = await clickedAway();
    const theme = media.followers("(prefers-color-scheme: dark)").length;
    const drew = drawn(app, document, SECOND);
    t.diagnostic(`${keydown.length} Escape handler(s) and ${theme} prefers-color-scheme listener(s) ` +
      `after switching away from a fetch that never settled; the clicked year ` +
      `${drew ? "drew" : "did not draw"} ("${lede(document)}", ${marks(document)} marks)`);
    // THE CHECK IS ABOUT A CLICK DURING THE OPENING FETCH, so it has to know
    // the opening fetch happened; otherwise both counts hold over a state the
    // test never entered.
    assert.ok(opened, `the opening fetch was never issued (asked for ${JSON.stringify(fetch.asked)})`);
    assert.equal(keydown.length, 1);
    assert.equal(theme, 1);
    assert.ok(drew, "the clicked year did not draw");
  });

  test("the Escape handler that survives a superseded open actually runs", async (t) => {
    const { app, document, window, keydown, errors, opened } = await clickedAway();
    assert.ok(opened, "the opening fetch was never issued, so nothing was superseded");
    assert.equal(keydown.length, 1, "there is no Escape handler to dispatch to");
    // Something for Escape to undo, so "ran" is observable and not just "did
    // not throw".
    app.setIsolated(app.projection.nodes[0].id);
    assert.notEqual(app.isolated, "");
    document.dispatchEvent(new window.KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
    t.diagnostic(`dispatching Escape ${errors.length ? "threw: " + errors.join("; ") : "threw nothing"}; ` +
      `isolation is ${JSON.stringify(app.isolated)}, pin is ${app.pinned}`);
    assert.deepEqual(errors, []);
    assert.equal(app.isolated, "");
    // A BOOLEAN, NOT THE PIN: a failing assert.equal hands node:test the laid
    // mark as `actual`, whose graph reaches the whole jsdom window, and
    // serializing it for the report takes every byte the machine has.
    assert.ok(app.pinned === null, "Escape left a pin");
  });
});

describe("a switch that fails", () => {
  test("a superseded fetch rejection paints no banner over the year that drew", async (t) => {
    // The refusal is deferred past the click on purpose: an immediate
    // rejection is handled before anything could be clicked, and the ordering
    // is the whole defect.
    let refuseOpening = () => {};
    const { app, document, fetch, config } = await loadApp({
      checkedStem: FIRST,
      plan: { "fy2026-adopted.json": { settle: ({ reject }) => { refuseOpening = reject; } } },
    });
    void app.boot();
    await settle();
    clickYear(document, SECOND);
    await settle();
    refuseOpening(new TypeError("Failed to fetch"));
    await settle();

    const opened = fetch.asked.includes(config.years[0].path);
    const banners = said(document);
    const drew = drawn(app, document, SECOND);
    t.diagnostic(`${banners.length} refusal banner(s) after switching away from a fetch that then ` +
      `failed${banners.length ? ": " + JSON.stringify(banners[0]) : ""}; the clicked year ` +
      `${drew ? "drew" : "did not draw"} ("${lede(document)}", ${marks(document)} marks)`);
    assert.ok(opened, `the opening fetch was never issued (asked for ${JSON.stringify(fetch.asked)}), ` +
      "so refuseOpening rejected nothing");
    assert.deepEqual(banners, []);
    assert.ok(drew, "the clicked year did not draw");
  });

  test("a malformed year document surfaces as a banner naming the parse failure", async (t) => {
    const { document } = await onFirstYear({ plan: { "fy2027-adopted.json": { badBody: true } } });
    const rejections = [];
    const onRejection = (e) => rejections.push(e);
    process.on("unhandledRejection", onRejection);
    clickYear(document, SECOND);
    await settle();
    process.off("unhandledRejection", onRejection);

    const banners = said(document);
    t.diagnostic(`${banners.length} refusal banner(s) after a 200 with an unparseable body` +
      (banners.length ? `: ${JSON.stringify(banners[0])}` : "") +
      `; ${rejections.length} unhandled rejection(s) observed`);
    assert.equal(banners.length, 1);
    assert.match(banners[0], /not valid JSON/);
  });

  test("a year document with no graph leaves the page whole and the reader told", async (t) => {
    // The spine schedule's nodes and links nulled: a document that parses and
    // carries no graph, which schema/column.schema.json refuses and nothing on
    // this side re-checks.
    const col = secondColumn((c) => { c.schedules.sankey.nodes = null; c.schedules.sankey.links = null; });
    const { document } = await onFirstYear({ plan: { "fy2027-adopted.json": { doc: col } } });
    clickYear(document, SECOND);
    await settle();

    const banners = said(document);
    t.diagnostic(`${banners.length} refusal banner(s) after a well-formed document with no graph` +
      (banners.length ? `: ${JSON.stringify(banners[0])}` : "") +
      `; the page still reads "${lede(document)}" under the title "${document.title}". ` +
      "The words are the last-resort catch's, not a sentence about the file");
    assert.equal(banners.length, 1);
    assert.equal(lede(document), FIRST_LEDE);
    assert.match(document.title, /FY 2025-26/);
  });

  test("a document that lays out badly reaches the last-resort catch, and repaints nothing", async (t) => {
    // Shaped right and wrong only in its content: one link's `to` is an index
    // past the node table, which is what a truncated or mis-joined file looks
    // like. Nothing before layOut refuses it.
    const col = secondColumn((c) => { c.schedules.sankey.links[0].to = c.nodes.length; });
    const { document } = await onFirstYear({ plan: { "fy2027-adopted.json": { doc: col } } });
    clickYear(document, SECOND);
    await settle();

    const banners = said(document);
    t.diagnostic(`${banners.length} banner(s) for a link naming a node the document does not carry` +
      (banners.length ? `: ${JSON.stringify(banners[0])}` : "") +
      `; the page still reads "${lede(document)}", because layOut() runs before the first repaint`);
    assert.equal(banners.length, 1);
    assert.equal(lede(document), FIRST_LEDE);
  });

  test("a 200 whose body is not a document is refused in words, not with a TypeError", async (t) => {
    // `null` is valid JSON, so response.json() resolves it.
    const { document } = await onFirstYear({ plan: { "fy2027-adopted.json": { doc: null } } });
    clickYear(document, SECOND);
    await settle();

    const banners = said(document);
    t.diagnostic(`${banners.length} banner(s)` + (banners.length ? `: ${JSON.stringify(banners[0])}` : ""));
    assert.equal(banners.length, 1);
    assert.match(banners[0], /not a document at all/);
    assert.doesNotMatch(banners[0], /TypeError/);
  });

  test("a malformed year document never leaves the words and the chart on different years", async (t) => {
    // Each fixture is the pinned second column with one key removed or broken
    // in its spine schedule. Every one is `required` in
    // schema/column.schema.json, so no file this export wrote lacks it; what
    // is asserted is that whatever the fault, the page is never left reading
    // one year's words over another year's chart.
    const DOC = "livermore-budget-fy2026-2027";
    const fixtures = [
      { key: "schedules.sankey.sources", mutate: (c) => { delete c.schedules.sankey.sources; } },
      { key: "links[].fact_ids", mutate: (c) => { for (const l of c.schedules.sankey.links) delete l.fact_ids; } },
      // One deeper: a locator carrying a doc_id and no pages. The doc id must
      // be one CONFIG.docs carries, or citations() never reaches pages.
      { key: "links[].locators[].pages",
        mutate: (c) => { for (const l of c.schedules.sankey.links) l.locators = [{ doc_id: DOC }]; } },
      { key: "links[].locators", mutate: (c) => { for (const l of c.schedules.sankey.links) delete l.locators; } },
      { key: "sources[].pages", mutate: (c) => { c.schedules.sankey.sources = [{ doc_id: DOC }]; } },
    ];
    const split = [];
    const refused = [];
    const drewAnyway = [];
    for (const bad of fixtures) {
      const { document } = await onFirstYear({ plan: { "fy2027-adopted.json": { doc: secondColumn(bad.mutate) } } });
      const rowsFirst = rows(document);
      // The first year drew, so there IS a year for the words to disagree
      // with; without this a page that never drew at all classifies as
      // "drew anyway" and the test passes over nothing.
      assert.equal(lede(document), FIRST_LEDE, `${bad.key}: the first year did not draw`);
      assert.ok(rowsFirst > 0, `${bad.key}: the first year drew no table rows`);
      clickYear(document, SECOND);
      await settle();

      const banners = said(document);
      const onFirst = lede(document) === FIRST_LEDE;
      const tableFrozen = rows(document) === rowsFirst;
      // Two consistent outcomes and one that is not. Refused: a banner, the
      // old year's words, the old year's table. Drew: no banner, the new
      // year's words. Anything else is the words and the chart disagreeing.
      if (banners.length && onFirst && tableFrozen) refused.push(bad.key);
      else if (!banners.length && !onFirst) drewAnyway.push(bad.key);
      else {
        split.push(`${bad.key} (${banners.length} banner(s), lede "${lede(document)}", ` +
          `${rows(document)} rows against ${rowsFirst})`);
      }
    }
    t.diagnostic(split.length
      ? `${split.length} fixture(s) half-repainted: ${split.join("; ")}`
      : `${refused.length} refused with the page untouched [${refused}]; ` +
        `${drewAnyway.length} drew [${drewAnyway}] -- ` +
        "schema/column.schema.json requiring them is what stops such a file being written");
    assert.deepEqual(split, []);
    // THE FOLD DEFAULTS NEITHER KEY. It reads fact_ids and locators off every
    // link it merges, so a column lacking either fails the draw and the page
    // says so with the first year still on it.
    assert.deepEqual(refused, ["links[].fact_ids", "links[].locators[].pages", "links[].locators"]);
    // The sources are first read at the pin, so a column lacking them draws.
    // Accepted: the client adds no shape check for a file the export cannot
    // write (fisc-wodu), and the pin is where the absence is met.
    assert.deepEqual(drewAnyway, ["schedules.sankey.sources", "sources[].pages"]);
  });
});

describe("the outcome protocol", () => {
  test("showYear tells drawn, superseded and failed apart", async (t) => {
    const { app, config } = await onFirstYear({
      plan: { "fy2027-adopted.json": { reject: new TypeError("Failed to fetch") } },
    });
    const drew = await app.showYear(config.years[0]);
    const failed = await app.showYear(config.years[1]);
    // Two attempts started back to back: the first is overtaken by the second
    // before its fetch resolves, which is the state the token guard exists for.
    const first = app.showYear(config.years[0]);
    const second = app.showYear(config.years[0]);
    const [a, b] = [await first, await second];

    t.diagnostic(`a good year is "${drew}", a refused one is "${failed}", an overtaken one is ` +
      `"${a}" and the attempt that overtook it is "${b}"`);
    assert.equal(new Set([drew, failed, a]).size, 3);
    assert.equal(drew, app.DREW);
    assert.equal(failed, app.FAILED);
    assert.equal(a, app.SUPERSEDED);
    assert.equal(b, drew);
  });
});

describe("a copy from another build", () => {
  test("a column from another build is refused, and the page stays on the year it drew", async (t) => {
    // Well-formed on purpose: only its stamp differs, so nothing but the
    // stamp comparison could refuse it.
    const col = secondColumn((c) => { c.generated_by = "fisc other"; });
    const { document } = await onFirstYear({ plan: { "fy2027-adopted.json": { doc: col } } });
    const before = said(document).length;
    clickYear(document, SECOND);
    await settle();

    const banners = said(document);
    t.diagnostic(`${banners.length} banner(s)` + (banners.length ? `: ${JSON.stringify(banners[0])}` : "") +
      `; the page still reads "${lede(document)}"`);
    assert.equal(before, 0);
    assert.equal(banners.length, 1);
    assert.match(banners[0], /fisc other/);
    assert.match(banners[0], /holding a copy/);
    assert.equal(lede(document), FIRST_LEDE);
  });
});

describe("the rung answer's fetch", () => {
  // Which nodes a column draws is Go's answer, fetched. The shape asserted is
  // "banner, and nothing drawn": the year's own document is never asked for,
  // which is what says the refusal happened before the draw.
  const RUNGS = "rungs.json";
  const truncated = () => {
    const answer = structuredClone(rungsFixture());
    // Kept as the body of the other-build row on purpose: a correctly-shaped
    // answer from another build is refused just the same, so the refusal is
    // about the stamp and not the shape.
    delete answer.columns[0].rungs[0].draws[0].ids;
    return answer;
  };
  for (const tc of [
    { name: "a rung answer the server will not serve", plan: { [RUNGS]: { ok: false, status: 404 } }, says: "HTTP 404" },
    { name: "a rung answer from another build",
      plan: { [RUNGS]: { doc: Object.assign(truncated(), { generated_by: "fisc other" }) } }, says: "fisc other" },
  ]) {
    test(`${tc.name} refuses the page in words and draws nothing`, async (t) => {
      const { app, document, fetch, config } = await onFirstYear({ plan: tc.plan });
      const banners = said(document);
      const askedYear = fetch.asked.includes(config.years[0].path);
      t.diagnostic(`main() asked for ${JSON.stringify(fetch.asked)}; ${banners.length} banner(s) ` +
        `(${banners.length ? JSON.stringify(banners[0]) : "none"}); the year document was ` +
        `${askedYear ? "FETCHED ANYWAY" : "never asked for"}, ${marks(document)} mark(s) drawn ` +
        `and the lede still reads the served "${lede(document)}"`);
      assert.equal(fetch.asked[0], RUNGS, "the rung answer was not what failed");
      assert.equal(banners.length, 1);
      assert.ok(banners[0].includes(RUNGS), "the banner does not name the file");
      assert.ok(banners[0].includes(tc.says), `the banner does not say ${JSON.stringify(tc.says)}`);
      assert.equal(askedYear, false);
      assert.equal(app.shownYear, null);
      assert.equal(app.projection, null);
      assert.equal(marks(document), 0);
    });
  }
});

describe("a gesture that throws", () => {
  test("a gesture that throws leaves a refusal a reader can read, not a half-built panel", async (t) => {
    // The shape is served rather than simulated: a schedule with no sources
    // draws, and the first mark a reader pins throws out of citations().
    const broken = structuredClone(columnFixture("fy2026-adopted"));
    delete broken.schedules.sankey.sources;
    const { app, document, window } = await onFirstYear({ plan: { "fy2026-adopted.json": { doc: broken } } });
    const errors = listenerErrors(window);
    const drewFirst = (app.projection && app.projection.nodes || []).length;
    const before = said(document).length;
    const drawnMarks = document.querySelectorAll("#chart g.node");
    const mark = drawnMarks[0];
    // The gesture and not the function: pin() called directly would walk past
    // the guard, which is on the listener.
    if (mark) mark.dispatchEvent(new window.MouseEvent("click", { bubbles: true }));
    await settle();

    const banners = said(document);
    t.diagnostic(!drewFirst
      ? "the document with no sources drew nothing, so no gesture could reach the throw"
      : `the chart drew ${drewFirst} node(s) as ${drawnMarks.length} mark(s) with ${before} banner(s); ` +
        `clicking the first ${errors.length ? "THREW PAST THE GUARD (" + errors.join("; ") + ")" : "was caught"} ` +
        `and left ${banners.length} banner(s)` + (banners.length ? `: ${JSON.stringify(banners[0])}` : ""));
    assert.ok(drewFirst > 0);
    assert.equal(before, 0);
    assert.ok(drawnMarks.length > 0);
    assert.deepEqual(errors, []);
    assert.equal(banners.length, 1);
    assert.match(banners[0], /could not/);
    assert.match(banners[0], /chart on screen is unchanged/);
  });
});

describe("refusals nothing drove", () => {
  // Each is a branch of app.js no other test reaches (fisc-rx1d), fired by a
  // one-key change to the served bytes.
  test("a column served 404 is refused in words and the page stays whole", async (t) => {
    // The newest year's column: `null` in the plan is a 404.
    const { app, document, config } = await loadApp({ plan: { "fy2027-adopted.json": null } });
    const served = lede(document);
    await app.boot();
    await settle();

    const banners = said(document);
    t.diagnostic(`${banners.length} banner(s)` + (banners.length ? `: ${JSON.stringify(banners[0])}` : "") +
      `; ${marks(document)} mark(s) drawn and the lede still reads the served "${lede(document)}"`);
    assert.equal(banners.length, 1);
    assert.equal(banners[0], `Could not load ${config.years[1].path}: HTTP 404`);
    assert.equal(app.shownYear, null);
    assert.equal(app.projection, null);
    assert.equal(marks(document), 0);
    assert.equal(lede(document), served);
  });

  for (const tc of [
    { name: "a rung answer the network refuses", plan: { "rungs.json": { reject: new TypeError("Failed to fetch") } },
      says: /Could not load rungs\.json\. If you opened this file directly/ },
    { name: "a rung answer that is not JSON", plan: { "rungs.json": { badBody: true } },
      says: /Could not read rungs\.json: the file is not valid JSON/ },
  ]) {
    test(`${tc.name} refuses the page in words and draws nothing`, async (t) => {
      const { app, document, fetch, config } = await onFirstYear({ plan: tc.plan });
      const banners = said(document);
      const askedYear = fetch.asked.includes(config.years[0].path);
      t.diagnostic(`main() asked for ${JSON.stringify(fetch.asked)}; ${banners.length} banner(s) ` +
        `(${banners.length ? JSON.stringify(banners[0]) : "none"}); the year document was ` +
        `${askedYear ? "FETCHED ANYWAY" : "never asked for"} and ${marks(document)} mark(s) drawn`);
      assert.equal(fetch.asked[0], "rungs.json");
      assert.equal(banners.length, 1);
      assert.match(banners[0], tc.says);
      assert.equal(askedYear, false);
      assert.equal(app.shownYear, null);
      assert.equal(marks(document), 0);
    });
  }

  test("a page packaged with no published year says so and fetches nothing", async (t) => {
    const config = structuredClone(pageFixture().config);
    config.years = [];
    const { app, document, fetch } = await loadApp({ config });
    await app.boot();
    await settle();

    const banners = said(document);
    t.diagnostic(`asked for ${JSON.stringify(fetch.asked)}; ${banners.length} banner(s)` +
      (banners.length ? `: ${JSON.stringify(banners[0])}` : ""));
    assert.deepEqual(fetch.asked, []);
    assert.equal(banners.length, 1);
    assert.match(banners[0], /packaged without any published year/);
    assert.equal(app.shownYear, null);
    assert.equal(marks(document), 0);
  });
});
