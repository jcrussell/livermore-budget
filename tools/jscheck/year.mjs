// year.mjs — the checks for the FY2026/FY2027 toggle.
//
// The toggle's whole design is that the PACKAGER builds every year's words in Go
// and the client only chooses between them. That makes one thing worth checking
// above all others: that the client reads the fields the packager writes. It
// shipped once reading `f.label` off a struct serialising `Label`, which renders
// a page of empty tiles and is silent — the template never sees the JSON, so
// nothing else notices.
//
// internal/export pins the Go side of that contract (TestYearViewKeysAreTheOnes
// TheClientReads). This pins the client side, against the same shipped app.js.

import { loadApp, settle, twoYearConfig, plannedFetch, goldenGraph } from "./harness.mjs";

/** A year as the packager emits it, with every key spelled the way Go tags it. */
function fixtureYear(overrides) {
  return Object.assign({
    year: 2027,
    label: "FY 2026-27",
    stem: "sankey-2027",
    path: "data/sankey-2027.json",
    basis: "adopted",
    title: "City of Livermore budget flows \u2014 FY 2026-27",
    hero: { label: "What the city actually spends", value: "$252,854,896", note: "note", kind: "hero" },
    figures: [
      { label: "Naive column total", value: "$325,241,780", note: "n", kind: "error" },
      { label: "Unmatched transfers", value: "$50,762,251", note: "n", kind: "" },
    ],
    // A CAVEAT IS AN OBJECT, not a string, since it gained an id a page can
    // link to. The fixture carries the whole shape rather than just `text`,
    // because a stub that supplies only the field under test cannot catch a
    // client reading the wrong one.
    // A YEAR'S CAVEATS ARE REFS, NOT WHOLE CAVEATS. The client is handed
    // id/summary/href and never the text -- see FiscCaveatRef in app.js -- so a
    // fixture carrying `text` would let a check pass on a client reading a
    // field the packager does not send.
    caveats: [
      { id: "one", summary: "s-one", href: "caveats.html#caveat-sankey-2027--one" },
      { id: "two", summary: "s-two", href: "caveats.html#caveat-sankey-2027--two" },
      { id: "three", summary: "s-three", href: "caveats.html#caveat-sankey-2027--three" },
    ],
    counts: { facts: 120, nodes: 25, links: 58 },
  }, overrides);
}

/** n distinct caveat refs, whose ids differ so the anchors do too. */
function fixtureCaveats(n) {
  return Array.from({ length: n }, (_, i) => ({
    id: "c" + i, summary: "summary " + i, href: "caveats.html#caveat-x--c" + i,
  }));
}

/**
 * Everything the recording DOM was told to show, after a paint.
 *
 * THE CHILD LISTS ARE COPIED, and that is the difference between a check and a
 * decoration. The stub hands back its LIVE children array, so two calls to this
 * function against an APPENDING paintYearWords return the same array object
 * twice and `second.tiles.length === first.tiles.length` is `n === n` -- green
 * against the exact defect the check below is named for (fisc-kwq: FY2026's
 * residual left on screen beside FY2027's chart). Found by /code-review,
 * 2026-08-26, and mutation-verified: reverting paintYearWords to append() left
 * every check in this file passing.
 */
function painted(app, year) {
  app.paintYearWords(year);
  const el = (id) => app.dom.byId.get(id);
  return {
    tiles: el("figures") ? [...el("figures").children] : [],
    // READ THROUGH THE ANCHOR, because a caveat is now <li><a>summary</a></li>
    // and the stub does NOT derive textContent from children (harness.mjs's
    // node()). Reading li.textContent here returns "" for every row, so a
    // check comparing it against an expected string fails for the wrong reason
    // -- and a check "corrected" to expect "" would pass whether the client
    // rendered the summary, the wrong field, or nothing at all. The href comes
    // out too: a link nobody asserts is a link that can silently stop being one.
    caveats: el("caveats") ? [...el("caveats").children].map((li) => {
      const a = li.children[0];
      return a ? { text: a.textContent, href: a.href || "" }
               : { text: li.textContent, href: "" };
    }) : [],
    caveatsCount: el("caveats-count") ? el("caveats-count").textContent : "",
    lede: el("lede-year") ? el("lede-year").textContent : "",
    counts: el("counts-line") ? el("counts-line").textContent : "",
    basis: el("page-basis") ? el("page-basis").textContent : "",
    title: app.dom.document.title,
  };
}

/**
 * A two-year page that has drawn one year, with the flow table reachable.
 *
 * THE tbody IS PLANTED BEFORE THE DRAW and that is the whole trick. buildTable
 * does `el("flow-table").querySelector("tbody")` and returns at `if (!body)
 * return` when that is null -- which it was in every check, because the stub's
 * flow-table node has no children for any descendant walk to find. No selector
 * grammar fixes that; the element has to be supplied. selectable is the seam
 * that supplies it, and it is the same idiom the footer citation check below
 * already uses.
 */
async function drewWithTable(plan) {
  const fetch = plannedFetch(plan);
  const app = loadApp({ config: twoYearConfig(), fetch });
  const body = app.dom.document.node();
  app.dom.document.getElementById("flow-table").selectable = { tbody: body };
  await settle();
  return { app, body, fetch };
}

/**
 * Fires the year control's change handler for one stem, as a click would.
 *
 * THE GUARDS ARE THE POINT. Without them a page whose toggle has no handler
 * makes this a no-op, and a check comparing before-and-after degenerates to
 * `n === n` -- green over a page that cannot switch year at all. Measured:
 * dropping wireYears' addEventListener left the flow-table check below PASSING.
 * lifecycle.mjs's copy of this helper has always thrown here; this one was
 * written without them.
 */
function clickYear(app, stem) {
  const group = app.dom.byId.get("year-toggle");
  if (!group) throw new Error("the page rendered no year-toggle to click");
  const handlers = group.listeners.change || [];
  if (!handlers.length) throw new Error("the year control has no change handler");
  for (const fn of handlers) fn({ target: { value: stem } });
}

/**
 * The page opens on the year the CONTROL is showing, after a soft reload.
 *
 * THE BROWSER RESTORES THE RADIO AND THE SERVER-RENDERED PAGE KNOWS NOTHING
 * ABOUT IT. index.html.tmpl hard-codes `checked` on the first year and the
 * inputs carry no autocomplete="off", so F5 or a Back navigation brings the
 * reader's own selection back. main() used to call showYear(years[0])
 * unconditionally, which left the toggle reading FY 2026-27 over FY 2025-26's
 * lede, tiles, chart, title and citation -- and no change event fires on a
 * restore, so it never self-corrected. The template's own comment calls that
 * state worse than having no control at all.
 *
 * checkedStem is the harness modelling exactly that restore.
 * @returns {Promise<{name: string, ok: boolean, detail: string}[]>}
 */
async function restoredSelection() {
  const config = twoYearConfig();
  const doc = goldenGraph();
  const second = config.years[1];

  const fetch = plannedFetch({
    "data/sankey.json": { doc },
    "data/sankey-2027.json": { doc },
  });
  const app = loadApp({ config, checkedStem: second.stem, fetch });
  await settle();

  const lede = app.dom.byId.get("lede-year");
  const drew = lede ? lede.textContent : "(no lede)";
  const want = `${second.label} ${second.basis}`;
  const asked = fetch.asked || [];

  return [{
    name: "a restored year selection is the year the page opens on",
    ok: drew === want,
    detail: `the control came back checked on ${second.stem} and the page drew ` +
      `"${drew}", want "${want}" -- opening on years[0] regardless is the state ` +
      `index.html.tmpl's own comment calls worse than no control`,
  }, {
    name: "and it fetched that year's document rather than the first one's",
    ok: asked.includes(second.path),
    detail: `main() asked for ${JSON.stringify(asked)}, which must include ` +
      `${second.path}; a page that fetches sankey.json and labels it FY 2026-27 ` +
      `is the same lie one layer down`,
  }];
}

/**
 * The theme button still describes the page after the OS switches under it.
 *
 * THE CONTROL INVERTED, which is worse than merely going stale. wireTheme's
 * sync was a closure and the prefers-color-scheme listener was wired to paint
 * alone, so a reader with no stored theme who opens in light gets a button
 * reading "Dark mode" / aria-pressed="false"; when the OS flips to dark the
 * page darkens and the ribbons repaint, but the button keeps that label -- and
 * prefersDark() now answers true, so clicking the control labelled "Dark mode"
 * makes the page LIGHT.
 * @returns {Promise<{name: string, ok: boolean, detail: string}[]>}
 */
async function themeFollowsTheOS() {
  const app = loadApp({ fetch: plannedFetch({ "data/sankey.json": { doc: goldenGraph() } }) });
  await settle();
  const button = app.dom.byId.get("theme-toggle");
  const before = `${button.textContent} / aria-pressed=${button.getAttribute("aria-pressed")}`;

  app.dom.setOSDark(true);
  await settle();
  const after = `${button.textContent} / aria-pressed=${button.getAttribute("aria-pressed")}`;

  return [{
    name: "the theme button follows an OS theme change",
    ok: before === "Dark mode / aria-pressed=false" &&
      after === "Light mode / aria-pressed=true",
    detail: `the button read "${before}" on a light page and "${after}" after the OS ` +
      `switched to dark; leaving it on "Dark mode" makes the control INVERT, because ` +
      `prefersDark() now answers true and the click handler negates it`,
  }];
}

export async function checks() {
  const app = loadApp();
  const year = fixtureYear();
  const got = painted(app, year);

  // Flattened text of every tile, which is where a wrong field name shows up as
  // emptiness rather than as an error.
  const tileText = got.tiles.map((t) => t.children.map((c) => c.textContent).join("|"));

  return [
    {
      name: "a year switch paints one tile per figure, plus the hero",
      ok: got.tiles.length === year.figures.length + 1,
      detail: `${got.tiles.length} tiles for ${year.figures.length} figures and a hero`,
    },
    {
      // THE CHECK THIS FILE EXISTS FOR. Reading a field the packager does not
      // write yields undefined, which renders as an empty tile and throws
      // nothing.
      name: "every tile carries the label, value and note the packager wrote",
      ok: tileText.length > 0 && tileText.every((t) => {
        const parts = t.split("|");
        return parts.length === 3 && parts.every((x) => x !== "" && x !== "undefined");
      }),
      detail: tileText.length ? tileText[0] : "no tiles were painted",
    },
    {
      name: "the hero is painted first and is the hero figure",
      ok: tileText.length > 0 && tileText[0].startsWith(year.hero.label),
      detail: tileText.length ? tileText[0].split("|")[0] : "nothing",
    },
    {
      // THE SUMMARY IS SHOWN AND THE HREF IS THE YEAR'S OWN. Both halves
      // matter and they fail differently: showing the wrong field puts a
      // 250-word paragraph back under the chart, while a stale href sends a
      // reader who switched to FY2026-27 to FY2025-26's copy of the sentence --
      // which is possible because one caveat id carries different text in
      // different documents, and is invisible unless the href is read.
      name: "the caveats are the year's own summaries, linked to the year's own anchors",
      ok: got.caveats.length === year.caveats.length &&
          got.caveats.every((c, i) => c.text === year.caveats[i].summary &&
                                      c.href === year.caveats[i].href),
      detail: `${got.caveats.length} caveats, matching the ${year.caveats.length} supplied` +
        (got.caveats.length ? `; first links to "${got.caveats[0].href}"` : ""),
    },
    {
      // THE COUNT IS IN THE <summary> A READER USES TO DECIDE WHETHER TO OPEN
      // THE LIST, so a stale one is a disclosure that under-reports itself --
      // "4 reasons" over a list of five. FY2025-26 really does carry four
      // caveats and FY2026-27 five, so this is not a hypothetical.
      //
      // TWO PAINTS, NOT ONE, and that is the whole check. Asserting the count
      // against a single paint passes on a write that runs once and never
      // again, which is exactly the shape being guarded: the count is
      // server-rendered correct for the OPENING year, so the defect only ever
      // appears after a switch. The second paint supplies a different number of
      // caveats and the count has to have followed it.
      //
      // It is also compared against the LIST BESIDE IT rather than against the
      // fixture's length alone, so a summary and a list that disagree is a
      // finding even if both are wrong in the same direction.
      name: "the caveat count in the summary follows the year, and agrees with the list",
      ok: (() => {
        const five = fixtureYear({ caveats: fixtureCaveats(5) });
        const after = painted(app, five);
        return after.caveatsCount === "5" &&
               after.caveatsCount === String(after.caveats.length);
      })(),
      detail: (() => {
        const after = painted(app, fixtureYear({ caveats: fixtureCaveats(5) }));
        return `after repainting with five caveats the summary reads "${after.caveatsCount}" ` +
          `over a list of ${after.caveats.length}`;
      })(),
    },
    {
      // THE TITLE IS COMPARED WHOLE. It was `got.title.includes(year.label)`,
      // which passes for any string merely CONTAINING "FY 2026-27".
      //
      // MEASURED: whole-string equality does not on its own catch app.js
      // re-composing the literal, because fixtureYear's title is by construction
      // the same string sankeyTitle composes -- reverting app.js to
      // "City of Livermore budget flows \u2014 " + year.label leaves this check
      // GREEN. The check below is the one that goes red, because a caller's
      // title is the one string no composition can reproduce. Both are kept:
      // this one pins that the client writes what it was handed, that one pins
      // that it was handed something. Neither covers fisc-rn0 alone.
      name: "the lede, the flow count and the document title follow the year",
      ok: got.lede === year.label + " " + year.basis &&
          got.counts === `${year.counts.links} flows between ${year.counts.nodes} nodes, from ${year.counts.facts} facts` &&
          got.title === year.title,
      detail: `lede "${got.lede}", counts "${got.counts}", title "${got.title}"`,
    },
    {
      // The footer's "Scope X, basis Y" sentence is a claim about the document
      // on screen. Left unpainted, switching to a year on another basis leaves
      // the lede saying "FY 2026-27 proposed" and the footer three screens down
      // saying "basis adopted" -- one page stating two things about one
      // document. Latent while both published years are adopted, so the fixture
      // supplies a basis neither of them uses.
      name: "the footer's basis follows the year",
      ok: painted(app, fixtureYear({ basis: "proposed" })).basis === "proposed",
      detail: `#page-basis reads "${painted(app, fixtureYear({ basis: "proposed" })).basis}" ` +
        `after painting a year published on a proposed basis`,
    },
    {
      // A title the CALLER configured is the caller's words and must survive
      // the switch; only the composed fallback carries a year. app.js used to
      // overwrite it during the opening showYear, before the reader had touched
      // anything, so a View that named its own page never kept the name.
      name: "a caller's own title is not overwritten by the year switch",
      ok: painted(app, fixtureYear({ title: "A title the caller chose" })).title ===
        "A title the caller chose",
      detail: `document.title is ` +
        `"${painted(app, fixtureYear({ title: "A title the caller chose" })).title}"`,
    },
    {
      // THE BUG THIS CHECK WAS ADDED FOR, and the reason the stub now carries a
      // years config: with one published year the template omits the fieldset,
      // and reaching for it with el() -- which THROWS on a missing id rather
      // than returning null -- took the entire chart down. `if (!group)` reads
      // like a guard and is an exception one line earlier.
      name: "a single-year build has no year control, and that is not an error",
      ok: (() => {
        // A page loaded as the template renders it for ONE published year: no
        // year-toggle at all.
        const single = loadApp(new Set(["figures", "caveats", "lede-year",
          "counts-line", "chart-title", "detail", "chart", "legend",
          "derived-list", "flow-table", "table-view", "tooltip", "theme-toggle"]));
        try {
          single.wireYears([fixtureYear()]);
          return single.maybeEl("year-toggle") === null;
        } catch (e) {
          return false;
        }
      })(),
      detail: "wireYears returns quietly when the template rendered no toggle",
    },
    {
      // The footer's "drawn from" link names a file that IS year-specific:
      // data/sankey.json and data/sankey-2027.json are different documents. Left
      // unpainted it cited the opening year's file for a chart drawn from
      // another one -- a provenance link disagreeing with the figures beside it,
      // which is the defect the citations exist to prevent, reached through the
      // toggle rather than through the packager.
      name: "the footer's data-file citation follows the year",
      ok: (() => {
        const app2 = loadApp();
        const anchor = app2.dom.document.node();
        const wrapper = app2.dom.document.node();
        wrapper.selectable = { a: anchor };
        app2.dom.document.plant("[data-year-path]", wrapper);
        app2.paintYearWords(fixtureYear());
        return anchor.getAttribute("href") === "data/sankey-2027.json" &&
               anchor.textContent === "data/sankey-2027.json";
      })(),
      detail: "the href and the text both name the year's own document",
    },
    {
      // The specific bug fisc-kwq names: FY2026's residual left on screen beside
      // FY2027's chart. Painting a second year must replace, not append.
      name: "painting a second year replaces the first year's words",
      ok: (() => {
        const first = painted(app, fixtureYear({
          year: 2026, label: "FY 2025-26", caveats: fixtureCaveats(2),
        }));
        const second = painted(app, year);
        // THE CAVEATS ARE ASSERTED, not merely collected. painted() has always
        // returned them and this check compared only tiles, lede and title, so
        // changing caveats.replaceChildren to append -- FY2026's caveats left on
        // screen beside FY2027's chart, the exact fisc-kwq defect this check is
        // named for -- left every check in the file green. The detail string
        // below claimed otherwise. Measured, then fixed.
        return second.tiles.length === first.tiles.length &&
               first.caveats.length === 2 &&
               second.caveats.length === year.caveats.length &&
               second.caveats.map((c) => c.text).join("|") === year.caveats.map((c) => c.summary).join("|") &&
               second.lede === year.label + " " + year.basis &&
               second.title.includes(year.label);
      })(),
      detail: "no tile, caveat or figure from the previous year survives the switch",
    },
    // ---------------------------------------------------------------- fisc-wcy
    //
    // THE FLOW TABLE HAD NEVER BEEN OBSERVED BY ANY CHECK. It is the largest
    // thing a year switch repaints, and buildTable returned at its `if (!body)
    // return` guard in every run because the stub could not answer "tbody".
    //
    // WHAT THIS PINS IS CORRECT-AND-UNOBSERVED BEHAVIOUR, NOT A LIVE BUG.
    // buildTable already calls body.replaceChildren() -- checked, at app.js:898
    // -- so the table does not append today. That is precisely why it is worth
    // an assertion: the fisc-kwq defect (FY2026's rows left on screen beside
    // FY2027's chart) is one edit away in a code path nothing looks at.
    await (async () => {
      const doc = goldenGraph();
      const { app: a, body } = await drewWithTable({
        "data/sankey.json": { doc },
        "data/sankey-2027.json": { doc },
      });
      const afterFirst = body.children.length;
      clickYear(a, "sankey-2027");
      await settle();
      const afterSecond = body.children.length;
      return {
        name: "a year switch replaces the flow table's rows rather than appending them",
        ok: afterFirst === doc.links.length && afterSecond === afterFirst,
        detail: `${afterFirst} rows for ${doc.links.length} links, still ${afterSecond} ` +
          `after switching year -- appending would read ${afterFirst * 2}`,
      };
    })(),
    // The other half of fisc-wcy. paint()'s legend loop queries
    // "#legend button .key", and until now that returned [] under every check,
    // so the swatch-recolouring half of theme-following executed nowhere.
    //
    // The swatches cannot be planted before the draw: buildLegend CREATES them.
    // So the check draws, reads back what buildLegend actually built, plants
    // exactly those, and calls paint() -- which is why paint is exported.
    // render() calls paint() itself (app.js:624), so this pins behaviour that
    // is already correct on the shipped page and was simply invisible here.
    await (async () => {
      const doc = goldenGraph();
      const { app: a } = await drewWithTable({
        "data/sankey.json": { doc },
        "data/sankey-2027.json": { doc },
      });
      const buttons = a.dom.byId.get("legend").children;
      const swatches = buttons.map((b) => b.children[0]);
      a.dom.document.plant("#legend button .key", ...swatches);
      for (const sw of swatches) sw.style.background = "";
      a.paint();
      const painted = swatches.filter((sw) => sw.style.background);
      return {
        name: "paint() rewrites every legend swatch's background",
        ok: swatches.length > 0 && painted.length === swatches.length,
        detail: `${painted.length} of ${swatches.length} swatch(es) recoloured from the ` +
          `palette; while this selector answered [] the loop ran zero times and said nothing`,
      };
    })(),
    ...(await restoredSelection()),
    ...(await themeFollowsTheOS()),
  ];
}
