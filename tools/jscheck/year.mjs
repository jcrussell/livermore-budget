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

import { readFileSync } from "node:fs";
import { join } from "node:path";

import {
  loadApp, settle, twoYearConfig, plannedFetch, goldenGraph, goldenGraph2027, goldenFundFlows,
  goldenFundFlows2027, repoRoot,
} from "./harness.mjs";

/**
 * The counts sentence exactly as a shipped template renders it, with `c` in
 * its slots.
 *
 * READ FROM THE TEMPLATE, NOT RE-COMPOSED. paintCounts writes this element
 * over the server-rendered sentence, so its words must be the template's own
 * -- and an expectation spelled as a literal here is a third copy of the
 * sentence, pinning the client to the checker while the template drifts free.
 * Substituting the numbers into the template's span makes an edit to either
 * wording go red. Verified by mutation: with the literal expectation,
 * rewording index.html.tmpl's span left the whole suite at 0 FAILs.
 *
 * THE PLURAL SHAPE IS THE SHARED ONE. Neither template renders a singular --
 * their numbers are a whole document's counts -- and paintCounts singularises
 * only figures a drilled chart can reach, which no template renders. A field
 * the template renames survives substitution as literal braces, so the
 * comparison fails closed rather than matching an empty slot.
 */
function templateCounts(file, c) {
  const src = readFileSync(join(repoRoot, "site", file), "utf8");
  const m = src.match(/<span id="counts-line">([\s\S]*?)<\/span>/);
  if (!m) throw new Error(file + " renders no #counts-line span to pin against");
  return m[1].replace(/\s+/g, " ").trim()
    .replaceAll("{{.Links}}", String(c.links))
    .replaceAll("{{.Nodes}}", String(c.nodes))
    .replaceAll("{{.Facts}}", String(c.facts));
}

/** A year as the packager emits it, with every key spelled the way Go tags it. */
function fixtureYear(overrides) {
  return Object.assign({
    year: 2027,
    label: "FY 2026-27",
    stem: "sankey-2027",
    path: "data/sankey-2027.json",
    basis: "adopted",
    title: "City of Livermore budget flows \u2014 FY 2026-27",
    // A STRING NO CLIENT COMPOSITION REPRODUCES, for the reason the title is
    // one: with the composed spine sentence here, a client rebuilding
    // "Sankey diagram of the " + label + " " + basis + " budget" from a
    // literal stays green against its own copy. The suffix is the part only
    // the packager knows -- the chart pages' views really do carry one.
    chart_title: "Sankey diagram of the FY 2026-27 adopted budget by a subject only the packager knows",
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
 * residual left on screen beside FY2027's chart). Mutation-verified: reverting
 * paintYearWords to append() left every check in this file passing.
 */
function painted(app, year) {
  app.paintYearWords(year);
  const el = (id) => app.dom.byId.get(id);
  return {
    // TWO CONTAINERS SINCE THE HERO ROSE ABOVE THE CHART, and collecting only
    // one of them is how a client that paints the headline into the collapsed
    // tile row stays green: #figures would hold one extra tile and every arm
    // that counts by figures.length would still be arithmetic about the wrong
    // element.
    hero: el("hero") ? [...el("hero").children] : [],
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
    chartTitle: el("chart-title") ? el("chart-title").textContent : "",
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
  const flatten = (t) => t.children.map((c) => c.textContent).join("|");
  const tileText = got.tiles.map(flatten);
  const heroText = got.hero.map(flatten);

  return [
    {
      name: "a year switch paints one tile per figure and no more",
      ok: got.tiles.length === year.figures.length,
      detail: `${got.tiles.length} tiles in #figures for ${year.figures.length} figures`,
    },
    {
      // THE ARM THAT NAMES THE SPLIT. index.html.tmpl puts #hero above the
      // chart and #figures inside a closed <details> below it, so a client
      // painting both into one container hides the headline behind a
      // disclosure -- which looks like nothing at all until somebody opens it.
      name: "the hero is painted into #hero, alone, and not into the tile row",
      ok: got.hero.length === 1 &&
          heroText[0].startsWith(year.hero.label) &&
          tileText.every((t) => !t.startsWith(year.hero.label)),
      detail: `#hero holds ${got.hero.length} tile(s): ${heroText.join(" / ") || "nothing"}`,
    },
    {
      // THE CHECK THIS FILE EXISTS FOR. Reading a field the packager does not
      // write yields undefined, which renders as an empty tile and throws
      // nothing. BOTH containers, since the hero is now painted through a
      // second call that could read a different set of field names.
      name: "every tile carries the label, value and note the packager wrote",
      ok: heroText.length + tileText.length > 0 && heroText.concat(tileText).every((t) => {
        const parts = t.split("|");
        return parts.length === 3 && parts.every((x) => x !== "" && x !== "undefined");
      }),
      detail: heroText.concat(tileText)[0] || "no tiles were painted",
    },
    {
      // THE SUMMARY IS SHOWN AND THE HREF IS THE YEAR'S OWN. Both halves
      // matter and they fail differently: showing the wrong field puts a
      // full paragraph back under the chart, while a stale href sends a
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
      // THE COUNTS EXPECTATION IS THE TEMPLATE'S SENTENCE, rendered by
      // templateCounts above -- see its comment for why a literal here is the
      // copy-checks-copy defect this suite exists to refuse.
      ok: got.lede === year.label + " " + year.basis &&
          got.counts === templateCounts("index.html.tmpl", year.counts) &&
          got.title === year.title,
      detail: `lede "${got.lede}", counts "${got.counts}", title "${got.title}"`,
    },
    {
      // BOTH TEMPLATES RENDER THIS ELEMENT AND THE CLIENT REPAINTS IT ON BOTH
      // PAGES. This arm and the index.html.tmpl comparison above witness only
      // the state BEFORE a document arrives: painted() loads none, so
      // paintCounts takes its undrawn branch -- the sentence a reader sees
      // while the fetch is in flight, and the one a reader with JavaScript off
      // keeps. The drawn page's sentence is a different one by design, and the
      // check after this one is the arm that holds it.
      name: "before a document arrives, chart.html.tmpl's counts sentence is the one the client paints",
      ok: got.counts === templateCounts("chart.html.tmpl", year.counts),
      detail: `the client paints "${got.counts}" and chart.html.tmpl renders ` +
        `"${templateCounts("chart.html.tmpl", year.counts)}"`,
    },
    await (async () => {
      // THE DRAWN SENTENCE, WHICH IS THE ONE EVERY SHIPPED PAGE SHOWS after
      // its first draw and the state no counts check here reached: with
      // ribbons on screen, paintCounts writes the templates' head with the
      // DRAWN numbers, then "N of the document's M facts" whenever the ribbons
      // cite fewer facts than the document holds -- true of both fund-flows
      // pages. The undrawn comparisons above never leave that branch's other
      // side, so a repaint drifting from the template post-draw is invisible
      // to them.
      //
      // THE HEAD IS READ FROM THE TEMPLATES, NOT SPELLED HERE: rendering each
      // span with a sentinel in its facts slot and cutting where the sentinel
      // lands yields everything up to the tail without this file carrying a
      // copy of the sentence. Rewording either template's head goes red in
      // THIS state, the one readers are in.
      //
      // THE TAIL IS A PINNED LITERAL, AND THAT IS THE HONEST LIMIT: no
      // template renders the drawn tail -- its words exist only in app.js --
      // so there is nothing shipped to read it from. A literal for it is a
      // pin against the client, not a copy of a template; fold.mjs pins both
      // fund-flows pages' whole drawn sentences the same way.
      const doc = goldenGraph();
      const { app: a } = await drewWithTable({
        "data/sankey.json": { doc },
        "data/sankey-2027.json": { doc },
      });
      const el = a.dom.byId.get("counts-line");
      const drawn = el ? el.textContent : "(no counts-line)";
      const headOf = (file) => {
        const t = templateCounts(file, {
          links: doc.links.length, nodes: doc.nodes.length, facts: "\u0000",
        });
        const cut = t.indexOf("\u0000");
        if (cut < 0) throw new Error(file + "'s counts sentence renders no facts slot to cut at");
        return t.slice(0, cut);
      };
      const cited = new Set();
      for (const l of doc.links) for (const id of l.fact_ids) cited.add(id);
      const total = twoYearConfig().years[0].counts.facts;
      if (cited.size === total) {
        throw new Error("the fixture's fact total equals the cited count, so the drawn tail cannot be witnessed");
      }
      const want = headOf("index.html.tmpl") +
        cited.size + " of the document's " + total + " fact" + (total === 1 ? "" : "s");
      return {
        name: "after a draw, the repainted counts sentence still opens with the templates' own head",
        ok: drawn === want && headOf("chart.html.tmpl") === headOf("index.html.tmpl"),
        detail: `the drawn page's counts-line reads "${drawn}", want "${want}"; ` +
          `chart.html.tmpl's head is "${headOf("chart.html.tmpl")}"`,
      };
    })(),
    {
      // THE CHART'S ACCESSIBLE NAME, WRITTEN WHOLE FROM THE PACKAGER'S FIELD.
      // It is a <title> inside the SVG, so drift here is invisible to every
      // sighted reader: a client composing its own sentence repaints the
      // template's server-rendered name with different words on the first
      // year toggle, announced only through a screen reader (fisc-rn0, and
      // fisc-m1uu for this element). The fixture's chart_title carries a
      // suffix no label-and-basis composition reproduces -- measured: with
      // the composed spine sentence in the fixture, reverting paintChartName
      // to compose from year.label and year.basis leaves this check green.
      name: "the chart's accessible name is the packager's chart_title, written whole",
      ok: got.chartTitle === year.chart_title,
      detail: `#chart-title reads "${got.chartTitle}", ` +
        `want the packager's "${year.chart_title}"`,
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
               // THE HERO IS A SECOND CONTAINER AND CAN APPEND ON ITS OWN.
               // Without this the arm below is about #figures only, and a hero
               // that grew a tile per year switch would ship green.
               second.hero.length === 1 && first.hero.length === 1 &&
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
    ...(await yearSwitchClosesTheDrill()),
    ...(await yearSwitchDropsTheExpansion()),
  ];
}

/**
 * A two-year spine whose tier 2 opens into fund-flows, with the flow table
 * reachable and every path planned -- `plan` overrides any of them.
 *
 * THE STEP DOCUMENT IS THE YEAR'S, as the packager ships it: each year's
 * config entry carries a `steps` list naming the file its rung draws, and
 * app.js joins nothing. `paths` is that file per spine stem; by default BOTH
 * years name the same file, which is the only shape under which a step cache
 * surviving a year switch can be seen at all -- with the shipped per-year
 * paths a stale cache would simply miss.
 */
async function chainedYears(plan, paths) {
  const config = twoYearConfig();
  config.projections["fund-flows"] = "data/fund-flows.json";
  config.steps = [{
    key: "group", after: [""], from: 2, projection: "fund-flows", tiers: [0, 3, 4],
    caps: [{ tier: 3, cap: 8 }, { tier: 4, cap: 24 }], back: "All fund groups", tail: "funds",
    description: "Opened.",
  }];
  const stepPaths = Object.assign(
    { sankey: "data/fund-flows.json", "sankey-2027": "data/fund-flows.json" }, paths || {});
  config.years = config.years.map((y) => Object.assign({}, y, {
    steps: [{ stem: stepPaths[y.stem].replace(/^data\/|\.json$/g, ""), path: stepPaths[y.stem], caveats: [] }],
  }));
  const doc = goldenGraph();
  const fetch = plannedFetch(Object.assign({
    "data/sankey.json": { doc },
    "data/sankey-2027.json": { doc },
    "data/fund-flows.json": { doc: goldenFundFlows() },
  }, plan || {}));
  const app = loadApp({ config, fetch });
  app.dom.document.getElementById("flow-table").selectable = { tbody: app.dom.document.node() };
  app.dom.document.plant("main", app.dom.document.node());
  await settle();
  return { app, fetch };
}

/**
 * A year switch closes every rung and forgets the step document, a drill
 * still in flight when the year changes stands down, and the year on screen
 * opens into ITS OWN step document.
 *
 * THREE DIFFERENT DEFECTS. Keeping the stack would draw the new year folded to
 * a node that may not exist in it -- FY2023-24 carries a seventh fund group
 * (fisc-zojk) -- and keeping the step document would hand the next rung a
 * cached file rather than the one the year names. The second arm is the race:
 * the step fetch is held open, the year is switched under it, and only then
 * is it allowed to resolve. Without the token check between the fetch and
 * the repaint the drill lands on the new year's empty stack and the page
 * shows FY 2026-27 in the control over FY 2025-26's General Fund. The third
 * arm is the per-year join on the wire: FY 2026-27's fund groups open into
 * fund-flows-2027, the file the packager put in that year's entry, and not
 * into the one file CONFIG.projections maps the stem to -- which is what the
 * client used to read, and drew FY2025-26's funds under FY2026-27's chart.
 * @returns {Promise<{name: string, ok: boolean, detail: string}[]>}
 */
async function yearSwitchClosesTheDrill() {
  const out = [];
  {
    const other = goldenFundFlows();
    other.nodes.find((n) => n.id === "fund/100").label = "General Fund, the other year";
    const { app, fetch } = await chainedYears(
      { "data/fund-flows-2027.json": { doc: other } },
      { "sankey-2027": "data/fund-flows-2027.json" },
    );
    const first = await app.drillDown("fund-group/general");
    await settle();
    clickYear(app, "sankey-2027");
    await settle();
    const second = await app.drillDown("fund-group/general");
    await settle();
    const asked = (p) => fetch.asked.filter((x) => x === p).length;
    const drawn = app.projection.nodes.find((n) => n.id === "fund/100");
    const label = drawn ? drawn.label : "";
    out.push({
      name: "the year on screen opens into its own step document, not the one the stem maps to",
      ok: first === "drew" && second === "drew" &&
          asked("data/fund-flows.json") === 1 && asked("data/fund-flows-2027.json") === 1 &&
          label === "General Fund, the other year",
      detail: `FY 2025-26 opened from data/fund-flows.json (${asked("data/fund-flows.json")} fetch); ` +
        `after the switch FY 2026-27 opened from data/fund-flows-2027.json ` +
        `(${asked("data/fund-flows-2027.json")} fetch) and drew fund/100 labelled "${label}" -- ` +
        `a client joining on CONFIG.projections would fetch the first file twice and draw ` +
        `"General Fund" under the wrong year`,
    });
  }
  {
    const { app, fetch } = await chainedYears();
    const drew = await app.drillDown("fund-group/general");
    await settle();
    const depthBefore = app.drilled.length;
    const askedBefore = fetch.asked.filter((p) => p === "data/fund-flows.json").length;
    clickYear(app, "sankey-2027");
    await settle();
    const crumb = app.dom.byId.get("breadcrumb");
    // READ NOW, before the second drill below shows the breadcrumb again.
    const crumbHidden = crumb.getAttribute("hidden") !== null;
    const depthAfter = app.drilled.length;
    const lede = app.dom.byId.get("lede-year").textContent;
    const again = await app.drillDown("fund-group/general");
    await settle();
    const askedAfter = fetch.asked.filter((p) => p === "data/fund-flows.json").length;
    out.push({
      name: "a year switch closes every rung and drops the step document",
      ok: drew === "drew" && depthBefore === 1 && depthAfter === 0 &&
          crumbHidden && lede === "FY 2026-27 adopted" &&
          askedBefore === 1 && again === "drew" && askedAfter === 2,
      detail: `opened to depth ${depthBefore}, switched year and read "${lede}" at depth ` +
        `${depthAfter} with the breadcrumb ${crumbHidden ? "hidden" : "SHOWING"}; ` +
        `the step document was fetched ${askedBefore} time(s) before the switch and ${askedAfter} ` +
        `after the next drill -- a cache that survived the year would read ${askedBefore}`,
    });
  }
  {
    let release = null;
    const { app } = await chainedYears({
      "data/fund-flows.json": { settle: (pair) => { release = pair; } },
    });
    const inFlight = app.drillDown("fund-group/general");
    await settle();
    clickYear(app, "sankey-2027");
    await settle();
    const lede = app.dom.byId.get("lede-year").textContent;
    if (!release) throw new Error("the step fetch was never asked for");
    release.resolve(goldenFundFlows());
    const outcome = await inFlight;
    await settle();
    out.push({
      name: "a drill overtaken by a year switch stands down rather than landing on the new year",
      ok: lede === "FY 2026-27 adopted" && outcome === "superseded" && app.drilled.length === 0,
      detail: `the year switched to "${lede}" while the step fetch was open; when it resolved the ` +
        `drill came to "${outcome}" and left ${app.drilled.length} rung(s) -- a drill that ` +
        `landed would read "drew" and 1`,
    });
  }

  // THE OTHER ORDERING, WHICH THE ARM ABOVE DOES NOT REACH. There the drill
  // starts first and the switch overtakes it, so `switching` moves after the
  // drill captured it and the token comparison fires. Here the SWITCH starts
  // first: showYear bumps `switching` before drillDown ever reads it, so the
  // token compares equal when the spine lands and no token can see the
  // collision. What lands is the old year's document under the new year's
  // heading -- measured before the fix as an FY2026-27 title and residual over
  // FY2025-26 fund figures in one chart.
  //
  // It is also the LIKELIER ordering at a reader's hands: the step document is
  // larger than the spine, so a click during a year switch usually resolves in
  // exactly this order.
  {
    let spine = null;
    let step = null;
    const { app } = await chainedYears({
      "data/sankey-2027.json": { settle: (pair) => { spine = pair; } },
      "data/fund-flows.json": { settle: (pair) => { step = pair; } },
    });
    clickYear(app, "sankey-2027");
    await settle();
    if (!spine) throw new Error("the year fetch was never asked for");
    // The reader clicks a group on the chart still in front of them, so BOTH
    // are open at once. The spine lands first -- it is the smaller file, which
    // is why this is the ordering a reader meets.
    const inFlight = app.drillDown("fund-group/general");
    await settle();
    if (!step) throw new Error("the step fetch was never asked for");
    spine.resolve(goldenGraph2027());
    await settle();
    step.resolve(goldenFundFlows());
    const outcome = await inFlight;
    await settle();
    const lede = app.dom.byId.get("lede-year").textContent;
    out.push({
      name: "a drill begun while a year switch is already in flight stands down rather than drawing two years at once",
      ok: lede === "FY 2026-27 adopted" && outcome === "superseded" && app.drilled.length === 0,
      detail: `the drill was begun on the old chart while the new year's spine was open; it came ` +
        `to "${outcome}" leaving ${app.drilled.length} rung(s), with the page reading "${lede}" -- ` +
        `a drill that landed would read "drew" and 1, and the chart would carry the new year's ` +
        `title over the old year's figures`,
    });
  }
  return out;
}

/**
 * A year switch takes the expansion with the rung it was made on.
 *
 * IT IS A DECISION AND NOT A CONSEQUENCE, which is why it is pinned in this
 * file rather than in drill.mjs. "Show me all 32 funds" names a column of one
 * year's document, and the other year's column is not that column -- fund/207
 * prints a dash in FY2026-27, so special-revenue has 31 funds there. A page
 * that remembered the expansion would answer a request the reader made about a
 * chart they are no longer looking at, and it would answer it with a different
 * number.
 *
 * WHAT MAKES IT TRUE is that the expansion is on the rung and showYear empties
 * the stack -- which it already did, for the reason the arm above states. So
 * this arm's subject is the choice of where the set LIVES: with a page-level
 * set, both halves below stay green and this one turns red.
 *
 * THE TWO COMMITTED COLUMNS AND NOT ONE DOCUMENT SERVED TWICE, because the
 * whole claim is about a column that differs between them.
 * @returns {Promise<{name: string, ok: boolean, detail: string}[]>}
 */
async function yearSwitchDropsTheExpansion() {
  const { app } = await chainedYears(
    { "data/fund-flows-2027.json": { doc: goldenFundFlows2027() } },
    { "sankey-2027": "data/fund-flows-2027.json" },
  );
  const group = "fund-group/special-revenue";
  const funds = () => app.projection.nodes.filter((n) => n.tier === 3).length;
  const tail = () => app.projection.nodes.find((n) => app.isAggregate(n.id));
  await app.drillDown(group);
  await settle();
  const folded = { funds: funds(), tail: tail() ? tail().label : "" };
  app.expandTier(tail());
  await settle();
  const opened = { funds: funds(), tail: tail() ? tail().label : "" };
  clickYear(app, "sankey-2027");
  await settle();
  const between = app.drilled.length;
  await app.drillDown(group);
  await settle();
  const after = { funds: funds(), tail: tail() ? tail().label : "" };
  return [{
    name: "a year switch drops the expansion with the rung it was made on",
    ok: folded.funds === 9 && folded.tail === "24 smaller funds" &&
        opened.funds === 32 && opened.tail === "" && between === 0 &&
        after.funds === 9 && after.tail === "23 smaller funds",
    detail: `FY 2025-26 drew ${folded.funds} fund mark(s) with a tail reading ` +
      `"${folded.tail}"; expanded, ${opened.funds} mark(s) and ${opened.tail ? "a tail" : "no tail"}; ` +
      `the switch left ${between} rung(s), and reopening the same group in FY 2026-27 drew ` +
      `${after.funds} mark(s) with a tail reading "${after.tail}" -- that year's column is 31 ` +
      `funds and not 32, so a remembered expansion would answer the reader's question about ` +
      `a column they are no longer looking at`,
  }];
}
