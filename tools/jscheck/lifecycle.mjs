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
  // AND A <tbody>, WITHOUT WHICH buildTable RETURNS AT ITS FIRST LINE. Every
  // check in this file drives a full draw, and buildTable is the LAST and
  // largest step of one -- so while el("flow-table").querySelector("tbody")
  // answered null here, the step carrying the most work executed in none of the
  // checks that assert the page is left consistent.
  //
  // That gap hid a live defect for exactly one commit: buildTable reached
  // citations(projection.metadata.sources), which throws on a document whose
  // metadata carries no sources -- AFTER paintYearWords, buildLegend and
  // buildDerivedList have repainted. The fisc-bsg split, one function past the
  // fix for it, with `make js` green over the whole thing.
  //
  // Past tense: fisc-5hxr moved the flow table onto each LINK's own locators,
  // so buildTable now reaches citations(l.locators) instead. The gap this
  // <tbody> closes is the same one and the key it exposes has changed.
  const body = app.dom.document.node();
  app.dom.document.getElementById("flow-table").selectable = { tbody: body };
  return { app, main, body };
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
    const fetch = plannedFetch({
      "data/sankey.json": { hang: true },
      "data/sankey-2027.json": { doc },
    });
    const { app } = page({ config, fetch });
    await settle();
    clickYear(app, "sankey-2027");
    await settle();

    // THE CHECK IS ABOUT A CLICK DURING THE OPENING FETCH, so it has to know the
    // opening fetch happened. Nothing here forces it: if main() stopped issuing
    // it, nothing would hang, the clicked year would still draw, both counts
    // would still be 1, and this would report PASS over a state it never
    // entered. Verified by gating `await showYear(years[0])` off in main() --
    // seam.mjs goes red, but this check went on passing, which is a claim about
    // a race that did not occur (fisc-ty6).
    const opened = fetch.asked.includes("data/sankey.json");
    const escape = (app.dom.documentListeners.keydown || []).length;
    const theme = (app.dom.mediaListeners.change || []).length;
    // The clicked year DID draw. Without this the check would pass over a page
    // that failed outright, which is a different bug with the same counts.
    const drew = app.dom.byId.get("lede-year");
    out.push({
      name: "a click during the opening fetch leaves the page's keyboard and theme wiring intact",
      ok: opened && escape === 1 && theme === 1 && Boolean(drew && drew.textContent),
      detail: !opened
        ? `the opening fetch was never issued (asked for ${JSON.stringify(fetch.asked)}), ` +
          `so there was no unsettled fetch to click during and this check reached ` +
          `none of the state it is named for`
        : `${escape} Escape handler(s) and ${theme} prefers-color-scheme listener(s) ` +
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
      ok: opened && escape === 1 && threw === "",
      detail: !opened
        ? "the opening fetch was never issued, so nothing was superseded"
        : escape !== 1
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
    // THE DEFAULT IS A NO-OP AND THAT IS WHY `opened` IS ASSERTED BELOW.
    // refuseOpening is only replaced inside plannedFetch's settle callback for
    // data/sankey.json, so if that fetch is never issued the callback never
    // runs, the call below rejects nothing, and this check reports PASS with the
    // rejection it exists to test never having happened (fisc-ty6).
    //
    // MAKING THE DEFAULT THROW WAS THE OTHER FIX AND IS WORSE HERE. run.mjs
    // turns a throw out of checks() into one "a whole check module threw before
    // producing any check" and moves on, discarding every other check in this
    // file -- the cost run.mjs's own comment says not to pay. An assertion
    // reddens one check by name and leaves the rest reporting.
    let refuseOpening = () => {};
    const fetch = plannedFetch({
      "data/sankey.json": { settle: ({ reject }) => { refuseOpening = reject; } },
      "data/sankey-2027.json": { doc },
    });
    const { app, main } = page({ config, fetch });
    await settle();
    clickYear(app, "sankey-2027");
    await settle();
    refuseOpening(new TypeError("Failed to fetch"));
    await settle();

    const opened = fetch.asked.includes("data/sankey.json");
    const banners = refusals(main);
    const drew = app.dom.byId.get("lede-year");
    out.push({
      name: "a superseded fetch rejection paints no banner over the year that drew",
      ok: opened && banners.length === 0 && Boolean(drew && drew.textContent),
      detail: !opened
        ? `the opening fetch was never issued (asked for ${JSON.stringify(fetch.asked)}), ` +
          `so refuseOpening rejected nothing and no banner could have been painted ` +
          `over anything -- this check asserted the absence of a thing it never caused`
        : `${banners.length} refusal banner(s) after switching away from a fetch that ` +
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
            // sources: [] IS LOAD-BEARING, not tidiness. Without it
            // drawableSankey refuses this document at its metadata.sources arm
            // two gates before layOut runs, so this block -- whose entire
            // purpose is to give the change handler's .catch a reachable case
            // -- duplicated the metadata.sources check instead, and the banner
            // it observed said so. Deleting that .catch from app.js left the
            // whole suite green. The document must be SHAPED right and wrong
            // only in its CONTENT, which is what a truncated file looks like.
            metadata: { fiscal_year: 2027, sources: [] },
            nodes: [{ id: "a", label: "A", value_cents: 1 }],
            // locators IS LOAD-BEARING FOR THE SAME REASON sources: [] IS, one
            // key later. drawableSankey refuses a document missing
            // links[].locators before layOut runs, so leaving it off here
            // would silently turn this block back into a duplicate of that
            // gate -- still one banner, still the first year, still PASSING,
            // with the .catch it exists to reach no longer reached.
            links: [{
              source: "a", target: "not-a-node", value_cents: 1, fact_ids: [],
              locators: [], kind: "revenue",
            }],
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

  // --------------------------------------------- a 200 whose body is not a doc
  //
  // `null` is valid JSON, so response.json() resolves it and showYear proceeds.
  // While the "is this a document at all" test lived INSIDE drawableSankey it
  // could never run, because understands(doc.schema_version, ...) dereferenced
  // doc one line earlier -- so the reader got "TypeError: Cannot read properties
  // of null (reading 'schema_version')" for what is almost always an error page
  // served with a success status.
  {
    const { app, main } = page({
      config,
      fetch: plannedFetch({
        "data/sankey.json": { doc },
        "data/sankey-2027.json": { doc: null },
      }),
    });
    await settle();
    clickYear(app, "sankey-2027");
    await settle();

    const banners = refusals(main);
    const text = banners.length ? banners[0].textContent : "";
    out.push({
      name: "a 200 whose body is not a document is refused in words, not with a TypeError",
      ok: banners.length === 1 && text.includes("not a document at all") &&
        !text.includes("TypeError"),
      detail: `${banners.length} banner(s)` + (banners.length ? `: "${text}"` : "") +
        `; the guard runs BEFORE understands(), which would otherwise dereference the null first`,
    });
  }

  // ------------------------------- the draw's LAST step, which nothing watched
  //
  // A document with a graph but no metadata.sources. It passes drawableSankey,
  // it lays out, and then buildTable -- the last and largest step of the
  // repaint -- calls citations(projection.metadata.sources) and throws on
  // `for (const source of undefined)`.
  //
  // IT IS THE SAME DEFECT AS fisc-bsg: paintYearWords, buildLegend and
  // buildDerivedList have already run, so the page is left reading FY 2026-27
  // over FY2025-26's chart. The fix that closed fisc-bsg claimed "everything in
  // the draw that can throw is in layOut" and this is the counter-example.
  //
  // It hid because page() planted no <tbody>, so buildTable returned at its
  // first line in every check here. A gate is only worth what the checks behind
  // it can reach.
  {
    const { app, main, body } = page({
      config,
      fetch: plannedFetch({
        "data/sankey.json": { doc },
        "data/sankey-2027.json": {
          doc: { schema_version: 1, metadata: {}, nodes: doc.nodes, links: doc.links },
        },
      }),
    });
    await settle();
    const rowsFirst = body.children.length;
    clickYear(app, "sankey-2027");
    await settle();

    const banners = refusals(main);
    const lede = app.dom.byId.get("lede-year");
    const stillFirst = lede && lede.textContent === "FY 2025-26 adopted";
    out.push({
      name: "a document whose metadata carries no sources is refused before the page repaints",
      ok: banners.length === 1 && Boolean(stillFirst) && body.children.length === rowsFirst,
      detail: `${banners.length} banner(s)` + (banners.length ? `: "${banners[0].textContent}"` : "") +
        `; the page reads "${lede ? lede.textContent : "(no lede)"}" and the flow table holds ` +
        `${body.children.length} rows against ${rowsFirst} before the click -- buildTable is the ` +
        `LAST step of the repaint, so a throw there is the fisc-bsg split reached one function later`,
    });
  }

  // --------------------------- the same last step, two keys further in (fisc-60r)
  //
  // The block above closed metadata.sources. buildTable dereferences two MORE
  // keys the gate did not name, and both are one element deeper than anything a
  // top-level Array.isArray can see:
  //
  //   links[].fact_ids          `l.fact_ids.join(" ")`
  //   links[].locators          citations(l.locators), one call per row
  //   metadata.sources[].pages  citations(), `for (const page of source.pages)`
  //
  // The last of those is now reached from pin() rather than from buildTable
  // (fisc-5hxr), so it is no longer a half-repaint case; the first two are,
  // and links[].locators is dereferenced on every row of every repaint.
  //
  // Each is the fisc-bsg split repaint reached one function later, by the same
  // route and with the same consequence: paintYearWords, buildLegend and
  // buildDerivedList have run, so the reader is left with one year's words over
  // another year's chart. Both documents below are shaped correctly at the top
  // level -- they pass every arm that existed before -- which is exactly why
  // drawableSankey's stated rule ("every key the draw DEREFERENCES before it
  // could report a failure") did not meet itself.
  //
  // WHAT IS ASSERTED IS THAT NOTHING MOVED, not merely that a banner appeared.
  // A half-repainted page also shows a banner.
  for (const bad of [{
    key: "links[].fact_ids",
    doc: {
      schema_version: 1,
      metadata: { fiscal_year: 2027, sources: [{ doc_id: "livermore-budget-fy2026-2027", pages: [66] }] },
      nodes: doc.nodes,
      links: doc.links.map((l) => ({ ...l, fact_ids: undefined })),
    },
  }, {
    // ONE DEEPER, and the arm fisc-5hxr shipped without: a locator carrying a
    // doc_id and no pages passes every top-level check, and citations() does
    // `for (const page of source.pages)`. On the spine nothing folds and
    // layOut never reads locators, so it reaches buildTable and throws there.
    key: "links[].locators[].pages",
    doc: {
      schema_version: 1,
      metadata: { fiscal_year: 2027, sources: [{ doc_id: "livermore-budget-fy2026-2027", pages: [66] }] },
      nodes: doc.nodes,
      // The doc id MUST be one CONFIG.docs carries, or citations() returns at
      // `if (!doc) continue` and never reaches source.pages -- and this check
      // would pass because the gate fired rather than because a throw was
      // prevented.
      links: doc.links.map((l) => ({
        ...l, locators: [{ doc_id: "livermore-budget-fy2026-2027" }],
      })),
    },
  }, {
    // The key the pin panel and the flow table now dereference to build a
    // per-mark source link. A document without it draws a chart whose every
    // citation throws at the reader.
    key: "links[].locators",
    doc: {
      schema_version: 1,
      metadata: { fiscal_year: 2027, sources: [{ doc_id: "livermore-budget-fy2026-2027", pages: [66] }] },
      nodes: doc.nodes,
      links: doc.links.map((l) => ({ ...l, locators: undefined })),
    },
  }, {
    // WEAKER THAN ITS SIBLINGS SINCE fisc-5hxr, and recorded so a later reader
    // does not assume otherwise. buildTable no longer calls
    // citations(projection.metadata.sources) -- it reads each link's own
    // locators -- so deleting this arm no longer produces a half-repaint here;
    // the only remaining reader is pin(), on a click. The arm stays because a
    // detail panel that throws at a reader is still worth naming in words, and
    // this case still proves the gate fires.
    key: "metadata.sources[].pages",
    doc: {
      schema_version: 1,
      metadata: { fiscal_year: 2027, sources: [{ doc_id: "livermore-budget-fy2026-2027" }] },
      nodes: doc.nodes,
      links: doc.links,
    },
  }]) {
    const { app, main, body } = page({
      config,
      fetch: plannedFetch({
        "data/sankey.json": { doc },
        "data/sankey-2027.json": { doc: bad.doc },
      }),
    });
    await settle();
    const rowsFirst = body.children.length;
    clickYear(app, "sankey-2027");
    await settle();

    const banners = refusals(main);
    const lede = app.dom.byId.get("lede-year");
    const stillFirst = lede && lede.textContent === "FY 2025-26 adopted";
    const named = banners.length === 1 && banners[0].textContent.includes(bad.key);
    out.push({
      name: `a document missing ${bad.key} is refused before the page repaints`,
      ok: named && Boolean(stillFirst) && body.children.length === rowsFirst,
      detail: `${banners.length} banner(s)` + (banners.length ? `: "${banners[0].textContent}"` : "") +
        `; the page still reads "${lede ? lede.textContent : "(no lede)"}" and the flow table holds ` +
        `${body.children.length} rows against ${rowsFirst} before the click, so nothing was ` +
        `half-repainted -- and the banner NAMES ${bad.key} rather than reporting a generic throw`,
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
