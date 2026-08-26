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

import { loadApp } from "./harness.mjs";

/** A year as the packager emits it, with every key spelled the way Go tags it. */
function fixtureYear(overrides) {
  return Object.assign({
    year: 2027,
    label: "FY 2026-27",
    stem: "sankey-2027",
    path: "data/sankey-2027.json",
    basis: "adopted",
    hero: { label: "What the city actually spends", value: "$252,854,896", note: "note", kind: "hero" },
    figures: [
      { label: "Naive column total", value: "$325,241,780", note: "n", kind: "error" },
      { label: "Unmatched transfers", value: "$50,762,251", note: "n", kind: "" },
    ],
    caveats: ["one", "two", "three"],
    counts: { facts: 120, nodes: 25, links: 58 },
  }, overrides);
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
    caveats: el("caveats") ? [...el("caveats").children] : [],
    lede: el("lede-year") ? el("lede-year").textContent : "",
    counts: el("counts-line") ? el("counts-line").textContent : "",
    title: app.dom.document.title,
  };
}

export function checks() {
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
      name: "the caveats are the year's own, not the year the page opened on",
      ok: got.caveats.length === year.caveats.length &&
          got.caveats.every((c, i) => c.textContent === year.caveats[i]),
      detail: `${got.caveats.length} caveats, matching the ${year.caveats.length} supplied`,
    },
    {
      name: "the lede, the flow count and the document title follow the year",
      ok: got.lede === year.label + " " + year.basis &&
          got.counts === `${year.counts.links} flows between ${year.counts.nodes} nodes, from ${year.counts.facts} facts` &&
          got.title.includes(year.label),
      detail: `lede "${got.lede}", counts "${got.counts}", title "${got.title}"`,
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
        const first = painted(app, fixtureYear({ year: 2026, label: "FY 2025-26" }));
        const second = painted(app, year);
        return second.tiles.length === first.tiles.length &&
               second.lede === year.label + " " + year.basis &&
               second.title.includes(year.label);
      })(),
      detail: "no tile, caveat or figure from the previous year survives the switch",
    },
  ];
}
