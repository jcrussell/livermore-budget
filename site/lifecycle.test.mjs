// lifecycle.test.mjs — what happens while a year is IN FLIGHT: the fetch, the
// switch token, and what main() does with each answer.

import { describe, test } from "node:test";
import assert from "node:assert/strict";
import {
  loadApp, settle, refusals, pageFixture, columnFixture, clickYear, keydownListeners, listenerErrors, split,
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

describe("the form handshake", () => {
  test("a chart whose form this script holds no renderer for is refused before anything is fetched", async (t) => {
    const asked = async (breaks) => {
      const config = structuredClone(pageFixture().config);
      breaks(config);
      const { app, document, fetch } = await loadApp({ config });
      await app.boot();
      await settle();
      return { fetched: fetch.asked.length, banners: said(document), forms: Array.from(app.FORMS.keys()) };
    };
    const overview = await asked((c) => { c.overview.form = "ledger"; });
    const good = await asked(() => {});
    // THE REFUSED PAGE STILL HAS A THEME TOGGLE, wired before the refusal; its
    // repaint must not reach for the renderer the page has none of.
    const toggled = await (async () => {
      const config = structuredClone(pageFixture().config);
      config.overview.form = "ledger";
      const loaded = await loadApp({ config });
      const errors = listenerErrors(loaded.window);
      await loaded.app.boot();
      await settle();
      const was = loaded.document.documentElement.dataset.theme;
      loaded.document.getElementById("theme-toggle").click();
      await settle();
      return { errors, was, now: loaded.document.documentElement.dataset.theme };
    })();
    t.diagnostic(`this script draws ${overview.forms.join(", ")}; a ledger overview fetches ${overview.fetched} file(s) ` +
      `and says ${JSON.stringify(overview.banners)}; ` +
      `the theme toggle on the refused page went ${toggled.was} -> ${toggled.now} with ${toggled.errors.length} listener error(s)`);
    assert.deepEqual(toggled.errors, []);
    assert.notEqual(toggled.now, toggled.was);
    assert.ok(good.fetched > 0 && good.banners.length === 0);
    assert.equal(overview.fetched, 0);
    assert.equal(overview.banners.length, 1);
    assert.match(overview.banners[0], /declares a ledger chart/);
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
    // theme.js's, which follows the button, and app.js's, which repaints.
    assert.equal(theme, 2);
    assert.ok(drew, "the clicked year did not draw");
  });

  test("the Escape handler that survives a superseded open actually runs", async (t) => {
    const { app, document, window, keydown, errors, opened } = await clickedAway();
    assert.ok(opened, "the opening fetch was never issued, so nothing was superseded");
    assert.equal(keydown.length, 1, "there is no Escape handler to dispatch to");
    // Something for Escape to undo, so "ran" is observable and not just "did
    // not throw".
    app.setIsolated(app.projection.nodes[0].id);
    assert.notEqual(app.filter.isolated, "");
    document.dispatchEvent(new window.KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
    t.diagnostic(`dispatching Escape ${errors.length ? "threw: " + errors.join("; ") : "threw nothing"}; ` +
      `isolation is ${JSON.stringify(app.filter.isolated)}, pin is ${app.pinned}`);
    assert.deepEqual(errors, []);
    assert.equal(app.filter.isolated, "");
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
    // The drawn schedule's nodes and links nulled: a document that parses and
    // carries no graph, which schema/column.schema.json refuses and nothing on
    // this side re-checks.
    const col = secondColumn((c) => { c.schedules.expansion.nodes = null; c.schedules.expansion.links = null; });
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
    const col = secondColumn((c) => { c.schedules.expansion.links[0].to = c.nodes.length; });
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
    // Whatever the fault, the page is never left reading one year's words
    // over another year's chart.
    const DOC = "livermore-budget-fy2026-2027";
    const fixtures = [
      { key: "schedules.expansion.sources", mutate: (c) => { delete c.schedules.expansion.sources; } },
      { key: "links[].fact_ids", mutate: (c) => { for (const l of c.schedules.expansion.links) delete l.fact_ids; } },
      // One deeper: a locator carrying a doc_id and no pages. The doc id must
      // be one CONFIG.docs carries, or citations() never reaches pages.
      { key: "links[].locators[].pages",
        mutate: (c) => { for (const l of c.schedules.expansion.links) l.locators = [{ doc_id: DOC }]; } },
      { key: "links[].locators", mutate: (c) => { for (const l of c.schedules.expansion.links) delete l.locators; } },
      { key: "sources[].pages", mutate: (c) => { c.schedules.expansion.sources = [{ doc_id: DOC }]; } },
    ];
    const halves = [];
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
        halves.push(`${bad.key} (${banners.length} banner(s), lede "${lede(document)}", ` +
          `${rows(document)} rows against ${rowsFirst})`);
      }
    }
    t.diagnostic(halves.length
      ? `${halves.length} fixture(s) half-repainted: ${halves.join("; ")}`
      : `${refused.length} refused with the page untouched [${refused}]; ` +
        `${drewAnyway.length} drew [${drewAnyway}] -- ` +
        "schema/column.schema.json requiring them is what stops such a file being written");
    assert.deepEqual(halves, []);
    // THE FOLD DEFAULTS NEITHER KEY. It reads fact_ids and locators off every
    // link it merges, so a column lacking either fails the draw and the page
    // says so with the first year still on it.
    assert.deepEqual(refused, ["links[].fact_ids", "links[].locators[].pages", "links[].locators"]);
    // The sources are first read at the pin, so a column lacking them draws.
    // Accepted: the client adds no shape check for a file the export cannot
    // write, and the pin is where the absence is met.
    assert.deepEqual(drewAnyway, ["schedules.expansion.sources", "sources[].pages"]);
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

describe("a gesture that throws", () => {
  test("a gesture that throws leaves a refusal a reader can read, not a half-built panel", async (t) => {
    // The shape is served rather than simulated: a schedule whose every node
    // prints a balance on a page the export built no links for draws, since
    // only the drawer cites a balance, and the first mark a reader pins
    // throws out of citations().
    const broken = structuredClone(columnFixture("fy2026-adopted"));
    const doc = broken.schedules.expansion.sources[0].doc_id;
    for (const n of broken.schedules.expansion.nodes) {
      n.balances = { beginning: { value_cents: 1, fact_id: "fisc-f-000000000000", locators: [{ doc_id: doc, pages: [999999] }] } };
    }
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
      ? "the document with an uncitable balance drew nothing, so no gesture could reach the throw"
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
  // Each is a branch of app.js no other test reaches, fired by a one-key
  // change to the served bytes.
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

describe("the view in the URL", () => {
  const YEARS = pageFixture().config.years;
  const NEWEST = YEARS[YEARS.length - 1];
  const OLDEST = YEARS[0];

  /** A fresh page whose URL carries `hash`, booted to rest. */
  async function bootAt(hash, o = {}) {
    const loaded = await loadApp(o);
    loaded.window.history.replaceState(null, "", hash);
    await loaded.app.boot();
    await settle();
    return loaded;
  }
  /** What a booted page shows, comparable across two pages. */
  const view = (app) => ({
    year: app.shownYear ? app.shownYear.stem : "",
    groups: [...app.filter.groups].sort(),
    kinds: [...app.filter.kinds].sort(),
    smallest: app.filter.smallest,
    isolated: app.filter.isolated,
  });
  /** The page's opening view: the newest year's root, nothing filtered. */
  const opening = { year: NEWEST.stem, groups: [], kinds: [], smallest: 0, isolated: "" };

  test("the filters and the isolation round-trip with the year", async (t) => {
    const { app, window } = await loadApp({ checkedStem: OLDEST.stem });
    await app.boot();
    await settle();
    app.setFilter({ kinds: new Set(["internal_transfer"]), smallest: 1000000, groups: new Set(["fund-group/permanent"]) });
    const follow = app.laidNodes.find((n) => !app.isSmaller(n.id) && n.id !== "fund-group/general").id;
    app.setIsolated(follow);
    const hash = window.location.hash;
    const want = view(app);
    const again = await bootAt(hash);
    t.diagnostic(`${hash}`);
    assert.equal(hash, app.hashOf(app.viewState()));
    assert.deepEqual(view(again.app), want);
    assert.deepEqual(said(again.document), []);
    // The hash names the year by its column file and the smallest flow in cents.
    assert.ok(/[#&]min=1000000(&|$)/.test(hash) && hash.includes("y=" + OLDEST.path.replace(/\.json$/, "")), hash);
  });

  test("Back and Forward walk the year; a filter change adds no step", async (t) => {
    const { app, document, window } = await loadApp();
    await app.boot();
    await settle();
    const popped = () => new Promise((resolve) => window.addEventListener("popstate", () => resolve(), { once: true }));
    const start = window.history.length;
    app.setFilter({ kinds: new Set(["internal_transfer"]) });
    assert.equal(window.history.length, start, "a filter pushed an entry");
    clickYear(document, OLDEST.stem);
    await settle();
    const switched = view(app);
    const wait = popped();
    window.history.back();
    await wait;
    await settle();
    const unswitched = view(app);
    t.diagnostic(`year ${switched.year}; back ${unswitched.year}`);
    assert.equal(switched.year, OLDEST.stem);
    assert.equal(unswitched.year, NEWEST.stem);
    assert.equal(document.querySelector("#year-toggle input:checked").value, NEWEST.stem);
    assert.deepEqual(said(document), []);
  });

  test("a hash naming a year, a node, a kind or a group the page does not publish or offer is refused with the banner, and the page shows its opening view", async (t) => {
    const key = NEWEST.path.replace(/\.json$/, "");
    const cases = [
      ["#y=fy1999-adopted", "fy1999-adopted"],
      [`#y=${key}&open=fund-group/general`, "open"],
      [`#y=${key}&hidekind=no_such_kind`, "no_such_kind"],
      [`#y=${key}&hide=fund-group/no-such`, "fund-group/no-such"],
      [`#y=${key}&iso=dept/patrol`, "dept/patrol"],
      [`#y=${key}&min=1.5`, "1.5"],
      [`#hide=fund-group/general`, "no year"],
      [`#y=${key}&zoom=2`, "zoom"],
      // A split of a node the root does not draw, and one the node lacks: no
      // guessing at the parent that would draw it, or at another split.
      [`#y=${key}&x=fund/100`, "fund/100"],
      [`#y=${key}&x=fund-group/general~no-such`, "no-such"],
    ];
    for (const [hash, names] of cases) {
      const { app, document, window } = await bootAt(hash);
      const banners = said(document);
      t.diagnostic(`${hash}: ${banners[0] || "(no banner)"}`);
      assert.equal(banners.length, 1, `${hash} drew without a banner`);
      assert.ok(banners[0].includes(names), banners[0]);
      assert.deepEqual(view(app), opening, `${hash} did not show the opening view`);
      assert.ok(document.querySelectorAll("#chart g.node").length > 0, `${hash} drew no chart`);
      assert.equal(window.location.hash, "#y=" + key);
    }
  });

  test("the splits round-trip with the year, in the order they were made, a tail and an alternative included", async (t) => {
    const { app, window } = await loadApp({ checkedStem: OLDEST.stem });
    await app.boot();
    await settle();
    await split(app, "fund-group/general", "fund-group/special-revenue", "expenditure/capital-outlay~divisions");
    const tail = app.projection.nodes.find((n) => n.id === app.splitTailID("fund-group/special-revenue", "funds"));
    assert.ok(tail, "the special-revenue split folds no tail");
    assert.ok(app.drawOutTail(tail), "the tail would not draw out");
    await settle();
    const member = app.splitsHere({ id: "fund-group/special-revenue" })[0].members.find((m) => app.laidNodes.some((n) => n.id === m));
    app.setIsolated(member);
    const hash = window.location.hash;
    const want = { x: app.expansions.slice(), view: view(app) };
    const again = await bootAt(hash);
    t.diagnostic(hash);
    assert.equal(hash, app.hashOf(app.viewState()));
    assert.deepEqual(want.x, ["fund-group/general", "fund-group/special-revenue",
      "expenditure/capital-outlay~divisions", tail.id]);
    assert.deepEqual({ x: again.app.expansions.slice(), view: view(again.app) }, want);
    assert.deepEqual(said(again.document), []);
  });

  test("Back and Forward walk the splits", async (t) => {
    const { app, document, window } = await loadApp();
    await app.boot();
    await settle();
    const popped = () => new Promise((resolve) => window.addEventListener("popstate", () => resolve(), { once: true }));
    const walk = async (go) => {
      const wait = popped();
      go();
      await wait;
      await settle();
      return app.expansions.join(",");
    };
    const divisions = new Set(app.splitsHere({ id: "fund/100" })[0].members);
    const drawn = () => app.laidNodes.filter((n) => divisions.has(n.id)).length;
    await split(app, "fund-group/general", "fund-group/capital");
    const opened = drawn();
    const back1 = await walk(() => window.history.back());
    // The group opened straight through its one fund in one step, and one
    // Back folds both.
    const back2 = await walk(() => window.history.back());
    const atRoot = drawn();
    const forward = await walk(() => window.history.forward());
    t.diagnostic(`split to fund-group/capital with ${opened} of fund/100's divisions drawn; back ${back1}; ` +
      `back ${back2 || "(the root)"} with ${atRoot}; forward ${forward} with ${drawn()}`);
    assert.ok(opened > 0, "opening the General Fund group drew none of fund/100's divisions");
    assert.equal(back1, "fund-group/general");
    assert.equal(back2, "");
    assert.equal(atRoot, 0);
    assert.equal(forward, "fund-group/general");
    assert.equal(drawn(), opened);
    assert.deepEqual(said(document), []);
  });

  test("a year switch keeps every split the new year can make, and says which it folded back", async (t) => {
    // PLANTED: every node either pinned year's expansion splits is in both,
    // so the oldest year's capital group is renamed, in its node table and
    // in every parent edge naming it.
    const planted = structuredClone(columnFixture(OLDEST.path.replace(/\.json$/, "")));
    const capital = planted.nodes.findIndex((n) => n.id === "fund-group/capital");
    planted.nodes[capital].id = "fund-group/capital-planted";
    for (const sched of Object.values(planted.schedules)) {
      for (const n of sched.nodes) if (n.parent === "fund-group/capital") n.parent = "fund-group/capital-planted";
    }
    const { app, document } = await loadApp({ plan: { [OLDEST.path]: { doc: planted } } });
    await app.boot();
    await settle();
    assert.equal(app.shownYear.stem, NEWEST.stem);
    const line = document.getElementById("splits-line");
    assert.ok(line.hidden, "the line is shown before any switch");
    await split(app, "fund-group/general", "fund-group/capital");
    clickYear(document, OLDEST.stem);
    await settle();
    const there = { year: app.shownYear.stem, x: app.expansions.slice(), words: line.textContent, hidden: line.hidden };
    clickYear(document, NEWEST.stem);
    await settle();
    const back = { x: app.expansions.slice(), hidden: line.hidden };
    t.diagnostic(`on ${there.year}: kept [${there.x.join(", ")}]; "${there.words}"; back on ${NEWEST.label}: [${back.x.join(", ")}]`);
    assert.equal(there.year, OLDEST.stem);
    assert.deepEqual(there.x, ["fund-group/general"]);
    assert.equal(there.hidden, false);
    assert.equal(there.words, app.say("splits_dropped", { year: OLDEST.label, nodes: planted.nodes[capital].label, n: 1 }));
    assert.deepEqual(back, { x: ["fund-group/general"], hidden: true });
    assert.deepEqual(said(document), []);
  });

  test("a group or kind the view does not draw is inert, not refused, and an in-page anchor names no view", async () => {
    const key = NEWEST.path.replace(/\.json$/, "");
    const inert = await bootAt(`#y=${key}&hide=fund-group/permanent`);
    assert.deepEqual(said(inert.document), []);
    assert.deepEqual([...inert.app.filter.groups], ["fund-group/permanent"]);
    assert.equal(inert.app.shown.hidden.length, 0);
    const anchor = await bootAt("#flows");
    assert.deepEqual(said(anchor.document), []);
    assert.deepEqual(view(anchor.app), opening);
    assert.equal(anchor.window.location.hash, "#flows");
  });
});
