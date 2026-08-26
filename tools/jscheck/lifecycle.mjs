// lifecycle.mjs — the checks for what happens while a year is IN FLIGHT.
//
// year.mjs exercises the toggle's PAINTING: given a year, does the page show
// that year's words. Everything here is about the other half — the fetch, the
// switch token, and what main() does with the answer. That half shipped with
// three defects (fisc-8cg), and until fisc-dn9 the harness could express none of
// them, so none was caught by anything but a human reading the file.
//
// WHAT THE THREE HAVE IN COMMON, and it is why they went unnoticed: each is a
// state a reader reaches with two clicks that leaves the page LOOKING correct.
// Nothing shows an error, so nothing gets reported.
//
// Every check here drives the real main(), at file scope, exactly as a browser
// does. Nothing calls main() directly — see harness.mjs on why exporting it
// would boot a second page.

import { loadApp, settle, twoYearConfig, plannedFetch, refusals, goldenGraph } from "./harness.mjs";

/**
 * Loads the page with a planted <main>, which is what makes a banner visible to
 * a check at all.
 *
 * fail() prepends its role="alert" banner into document.querySelector("main"),
 * and the stub answers selectors only from what a check has planted. Without
 * this the banner is never created and "assert no banner" is vacuously true —
 * the exact shape fisc-dn9 is about, arriving in the checks that most need it
 * not to.
 *
 * The plant lands immediately after loadApp rather than before, and that is in
 * time: every banner these checks care about is painted from a promise
 * continuation, not synchronously during load.
 */
function page(opts) {
  const app = loadApp(opts);
  const main = app.dom.document.node();
  app.dom.document.plant("main", main);
  return { app, main };
}

/** Fires the year control's change handler for one stem, as a click would. */
function clickYear(app, stem) {
  const group = app.dom.byId.get("year-toggle");
  if (!group) throw new Error("the page rendered no year-toggle to click");
  const handlers = group.listeners.change || [];
  if (!handlers.length) throw new Error("the year control has no change handler");
  for (const fn of handlers) fn({ target: { value: stem } });
}

export async function checks() {
  const out = [];
  const config = twoYearConfig();
  const doc = goldenGraph();

  // ---------------------------------------------------------------- defect 1
  //
  // The opening fetch never settles and the reader clicks the other year. Not a
  // contrived race: wireYears removes the control's `disabled` BEFORE main()
  // awaits, so the control is live for the whole of the first fetch — and on a
  // cold cache that is exactly when a reader clicks.
  //
  // The old main() returned the moment the opening showYear reported anything
  // but success, and everything it wires comes after that line. The clicked year
  // still drew, from its own showYear, so the page looked entirely healthy with
  // Escape and OS-theme-following dead for the rest of the visit.
  {
    const { app } = page({
      config,
      fetch: plannedFetch({
        "data/sankey.json": { hang: true },
        "data/sankey-2027.json": { doc },
      }),
    });
    await settle();
    clickYear(app, "sankey-2027");
    await settle();

    const escape = (app.dom.documentListeners.keydown || []).length;
    const theme = (app.dom.mediaListeners.change || []).length;
    // The clicked year DID draw. Without this the check would pass over a page
    // that failed outright, which is a different bug with the same counts.
    const drew = app.dom.byId.get("lede-year");
    out.push({
      name: "a click during the opening fetch leaves the page's keyboard and theme wiring intact",
      ok: escape === 1 && theme === 1 && Boolean(drew && drew.textContent),
      detail: `${escape} Escape handler(s) and ${theme} prefers-color-scheme listener(s) ` +
        `after switching away from a fetch that never settled, with the clicked year drawn ` +
        `("${drew ? drew.textContent : ""}")`,
    });

    // Attached is not the same as working. Dispatching is what says the handler
    // counted above is the real one and not something else keyed on "keydown".
    let threw = "";
    try {
      for (const fn of app.dom.documentListeners.keydown || []) fn({ key: "Escape" });
    } catch (e) {
      threw = String(e);
    }
    out.push({
      name: "the Escape handler that survives a superseded open actually runs",
      ok: escape === 1 && threw === "",
      detail: escape !== 1
        ? `there is no Escape handler to dispatch to (${escape} attached)`
        : threw === ""
          ? "dispatching Escape clears the pin, the panel and the isolation without throwing"
          : `dispatching Escape threw: ${threw}`,
    });
  }

  // ---------------------------------------------------------------- defect 2
  //
  // The opening fetch is REFUSED — file://, a dropped connection — and the
  // reader switches away before it lands. Every other exit in showYear checks
  // the switch token first; the catch around the fetch called fail()
  // unconditionally, so the failing year's role="alert" banner was pasted over
  // the year that drew correctly.
  //
  // The refusal is deferred past the click on purpose: an immediate rejection is
  // handled in the next microtask, before anything could be clicked, and the
  // ordering is the whole defect.
  {
    let refuseOpening = () => {};
    const { app, main } = page({
      config,
      fetch: plannedFetch({
        "data/sankey.json": { settle: ({ reject }) => { refuseOpening = reject; } },
        "data/sankey-2027.json": { doc },
      }),
    });
    await settle();
    clickYear(app, "sankey-2027");
    await settle();
    refuseOpening(new TypeError("Failed to fetch"));
    await settle();

    const banners = refusals(main);
    const drew = app.dom.byId.get("lede-year");
    out.push({
      name: "a superseded fetch rejection paints no banner over the year that drew",
      ok: banners.length === 0 && Boolean(drew && drew.textContent),
      detail: `${banners.length} refusal banner(s) after switching away from a fetch that ` +
        `then failed` + (banners.length ? `: "${banners[0].textContent}"` : "") +
        `, with the clicked year drawn ("${drew ? drew.textContent : ""}")`,
    });
  }

  // ---------------------------------------------------------------- defect 3
  //
  // A year the server answers 200 for, with a body that does not parse. The
  // change handler was `void showYear(year)` with no .catch, and
  // `await response.json()` sat outside any try — so the promise rejected into
  // nothing: no banner, and a page half-repainted between two years.
  {
    const { app, main } = page({
      config,
      fetch: plannedFetch({
        "data/sankey.json": { doc },
        "data/sankey-2027.json": { badBody: true },
      }),
    });
    await settle();

    const rejections = [];
    const onRejection = (e) => rejections.push(e);
    process.on("unhandledRejection", onRejection);
    clickYear(app, "sankey-2027");
    await settle();
    process.off("unhandledRejection", onRejection);

    // The assertion is the OBSERVABLE half — the reader is told. Unhandled
    // rejections are reported beside it and not asserted on, because detecting
    // one across a vm boundary is best-effort and a check that depended on it
    // would be flaky rather than strict.
    const banners = refusals(main);
    // AND IT SAYS WHICH FAULT IT WAS. A body that will not parse is not a
    // blocked request, and "serve it over HTTP instead" is useless advice to
    // someone already doing that. Asserting the wording is also what keeps the
    // branch reachable: it is selected on e.name, because instanceof compares
    // against this realm's SyntaxError and the body was parsed in another.
    const parseWorded = banners.length === 1 && banners[0].textContent.includes("not valid JSON");
    out.push({
      name: "a malformed year document surfaces as a banner naming the parse failure",
      ok: banners.length === 1 && parseWorded,
      detail: `${banners.length} refusal banner(s) after a 200 with an unparseable body` +
        (banners.length ? `: "${banners[0].textContent}"` : "") +
        `; ${rejections.length} unhandled rejection(s) observed`,
    });
  }

  // ------------------------------------------------- defect 3, the other half
  //
  // A document that parses, passes the schema gate and is then structurally
  // wrong. This used to walk into buildLegend and throw MID-REPAINT: the .catch
  // painted a banner, and left the new year's tiles, caveats, lede and <title>
  // over the OLD year's chart, with a citation link naming the year that was
  // not drawn (fisc-bsg). A page whose words and whose figures are about
  // different fiscal years is a worse outcome than a page that refuses.
  //
  // So it is now refused BEFORE anything repaints, by drawableSankey, and this
  // check asserts both halves: a banner appears AND the page is still wholly
  // the year it was already showing. Asserting the banner alone is what let the
  // split state ship -- `make js` was green over it.
  {
    const { app, main } = page({
      config,
      fetch: plannedFetch({
        "data/sankey.json": { doc },
        // Right version, no graph. understands() lets it through, because the
        // version really is one this page renders.
        "data/sankey-2027.json": { doc: { schema_version: 1, metadata: {}, nodes: null, links: null } },
      }),
    });
    await settle();
    clickYear(app, "sankey-2027");
    await settle();

    const banners = refusals(main);
    const lede = app.dom.byId.get("lede-year");
    const stillFirst = lede && lede.textContent === "FY 2025-26 adopted" &&
      app.dom.document.title.includes("FY 2025-26");
    // THE MESSAGE IS THE ASSERTION, and that is not fussiness about wording.
    // Measured: laying out before repainting ALREADY leaves this page whole,
    // because layOut is where `nodes.map` throws -- so "a banner appeared and
    // the page is on one year" passes with the gate deleted. What the gate buys
    // is the SENTENCE: a reader gets "the file is truncated" instead of
    // "TypeError: Cannot read properties of null (reading 'map')", which is an
    // internal error shown to a reader for what is really a bad file. Assert
    // the thing the gate actually changes, or the check does not cover it.
    const text = banners.length ? banners[0].textContent : "";
    const explains = text.includes("truncated") && !text.includes("TypeError");
    out.push({
      name: "a year document with no graph is refused, in words a reader can act on",
      ok: banners.length === 1 && Boolean(stillFirst) && explains,
      detail: `${banners.length} refusal banner(s) after a well-formed document with no graph` +
        (banners.length ? `: "${text}"` : "") +
        `; the page still reads "${lede ? lede.textContent : "(no lede)"}", and the banner names a ` +
        `truncated FILE rather than reporting a TypeError at the reader`,
    });
  }

  // ------------------------------------------- the last resort, kept reachable
  //
  // THE .catch ON THE CHANGE HANDLER NEEDS A REACHABLE CASE OR IT IS AN
  // UNFALSIFIABLE CLAIM SITTING IN THE FILE, and the gate above took its only
  // one away. This supplies another, and a more honest one: a document whose
  // shape is entirely correct and whose CONTENT is not -- a link naming a node
  // the document does not carry, which is what a truncated or mis-joined file
  // actually looks like. d3-sankey throws "missing: <id>" on it.
  //
  // The gate deliberately does not catch this. Re-validating the graph in the
  // client would be a second implementation of `fisc verify`, and there will
  // always be a throw nobody anticipated -- which is the whole point of having
  // a last resort. What matters is that reaching it still leaves the page
  // consistent, and laying out BEFORE repainting is what buys that.
  {
    const { app, main } = page({
      config,
      fetch: plannedFetch({
        "data/sankey.json": { doc },
        "data/sankey-2027.json": {
          doc: {
            schema_version: 1,
            metadata: { fiscal_year: 2027 },
            nodes: [{ id: "a", label: "A", value_cents: 1 }],
            links: [{ source: "a", target: "not-a-node", value_cents: 1, fact_ids: [], kind: "revenue" }],
          },
        },
      }),
    });
    await settle();
    clickYear(app, "sankey-2027");
    await settle();

    const banners = refusals(main);
    const lede = app.dom.byId.get("lede-year");
    const stillFirst = lede && lede.textContent === "FY 2025-26 adopted";
    out.push({
      name: "a document that lays out badly reaches the last-resort catch, and repaints nothing",
      ok: banners.length === 1 && Boolean(stillFirst),
      detail: `${banners.length} banner(s) for a link naming a node the document does not carry` +
        (banners.length ? `: "${banners[0].textContent}"` : "") +
        `; the page still reads "${lede ? lede.textContent : "(no lede)"}", because layOut() runs ` +
        `before the first repaint rather than after the last`,
    });
  }

  // ------------------------------------------------- the outcome protocol
  //
  // showYear's three states are the fisc-8cg fix, and until this check they
  // were returned by every exit and READ BY NOBODY: wireYears voids the value
  // and main() discards it. A protocol nothing observes is a comment, and
  // collapsing it back to the boolean it replaced -- the regression its doc
  // comment exists to prevent -- would have been caught by nothing.
  //
  // The three are driven directly rather than through main(), because SUPERSEDED
  // needs two attempts in flight at once and that is not a state a page reaches
  // by itself on demand.
  {
    const { app } = page({
      config,
      fetch: plannedFetch({
        "data/sankey.json": { doc },
        "data/sankey-2027.json": { reject: new TypeError("Failed to fetch") },
      }),
    });
    await settle();

    const drew = await app.showYear(config.years[0]);
    const failed = await app.showYear(config.years[1]);
    // Two attempts started back to back: the first is overtaken by the second
    // before its fetch resolves, which is the state the token guard exists for.
    const first = app.showYear(config.years[0]);
    const second = app.showYear(config.years[0]);
    const [a, b] = [await first, await second];

    const distinct = new Set([drew, failed, a]).size === 3;
    out.push({
      name: "showYear tells drawn, superseded and failed apart",
      ok: distinct && drew === b,
      detail: `a good year is "${drew}", a refused one is "${failed}", an overtaken one is ` +
        `"${a}" and the attempt that overtook it is "${b}" — three distinct outcomes, ` +
        `where one boolean for "did not draw" is what made a superseded opening fetch ` +
        `read as a page that had given up`,
    });
  }

  return out;
}
