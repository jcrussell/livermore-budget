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

import {
  loadApp, settle, twoYearConfig, plannedFetch, refusals, goldenGraph,
  RUNGS_PATH, rungsAnswer, columnOf,
} from "./harness.mjs";
// THE DRILL-CONFIGURED PAGE IS BUILT IN drill.mjs AND IS NOT REBUILT HERE. The
// column budget only changes a chart in a window -- the overview is drawn at
// RENDER_TIERS whatever the budget -- so the arm that says a budget change
// redraws the rung needs a real window, and a second copy of that config here
// is the thing drill.mjs's PAGE exists to avoid.
import { openedChain } from "./drill.mjs";

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
  // The reader is on the FIRST year unless an arm says otherwise. The page's
  // own default is the newest, and every arm here plans one document's fetch;
  // leaving the year to the default makes each a check about which year opens,
  // which is year.mjs's subject.
  const app = loadApp({ checkedStem: (opts.config?.years || [])[0]?.stem, ...opts });
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

  // THE ONE VERSION GATE LEFT, and it is about the page and this script rather
  // than about a fetched document. understands() took a version and had three
  // callers, all of them documents; those went because Go validates every
  // artifact against schema/ before writing it, and which COPY arrived is
  // answered by comparing generated_by with exported_by. This pair has no
  // second file to compare -- app.js carries no stamp of its own -- so a
  // constant is the only handshake available, and it is why SCHEMA_VERSION
  // survives the deletion.
  //
  // ASSERTED ON THE FETCH AND NOT ON THE BANNER, which is not a weaker claim
  // but a different one. The gate is the first statement in main() and runs
  // during load, before a harness can plant the <main> a banner lands in --
  // so the banner is genuinely unobservable here while the REFUSAL is exactly
  // observable: a page that will not draw asks for nothing. "Refused before
  // anything is fetched" is what the check is named for.
  {
    const asked = async (v) => {
      const config = twoYearConfig();
      config.schema_version = v;
      const fetch = plannedFetch({ "data/sankey.json": { doc: goldenGraph() } });
      page({ config, fetch });
      await settle();
      return fetch.asked.length;
    };
    const ok = loadApp().SCHEMA_VERSION;
    const good = await asked(ok);
    const newer = await asked(ok + 1);
    const older = await asked(ok - 1);
    out.push({
      name: "a page packaged for another schema_version is refused before anything is fetched",
      ok: good > 0 && newer === 0 && older === 0,
      detail: `schema_version ${ok} fetches ${good} file(s); ${ok + 1} and ${ok - 1} ` +
        `fetch ${newer} and ${older} -- the page and this script are separately ` +
        `cached files, which is the skew no stamp comparison can see`,
    });
  }

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
    const opened = fetch.asked.includes(config.years[0].path);
    const escape = (app.dom.documentListeners.keydown || []).length;
    const theme = app.dom.followers("(prefers-color-scheme: dark)").length;
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

    const opened = fetch.asked.includes(config.years[0].path);
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
  // So this check asserts both halves: a banner appears AND the page is still
  // wholly the year it was already showing. Asserting the banner alone is what
  // let the split state ship -- `make js` was green over it. What keeps the
  // page whole is the ordering, not a gate: showYear shapes, lays out and
  // builds the table's rows before it writes a word.
  {
    const { app, main } = page({
      config,
      fetch: plannedFetch({
        "data/sankey.json": { doc },
        // Right version, no graph -- the shape schema/column.schema.json
        // refuses and no gate on this side does any more.
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
    // THE SENTENCE IS WHAT WAS LOST, AND IT IS THE ONLY THING THAT WAS.
    // Measured, and it was measured when the gate still existed: laying out
    // before repainting ALREADY leaves this page whole, because layOut is
    // where `nodes.map` throws. What drawableSankey bought was the WORDING --
    // "the file is truncated" rather than "TypeError: Cannot read properties
    // of null (reading 'map')". That gate has gone, because Go refuses to
    // write a column with no graph (schema/column.schema.json requires nodes
    // and links) and re-checking it here was a second implementation.
    //
    // So this asserts what survives and reports what does not: the reader is
    // told, and the page is untouched, but the words are the last resort's.
    // Nothing on this side covers the wording, and the file that would need it
    // is one this export cannot have written.
    const text = banners.length ? banners[0].textContent : "";
    out.push({
      name: "a year document with no graph leaves the page whole and the reader told",
      ok: banners.length === 1 && Boolean(stillFirst),
      detail: `${banners.length} refusal banner(s) after a well-formed document with no graph` +
        (banners.length ? `: "${text}"` : "") +
        `; the page still reads "${lede ? lede.textContent : "(no lede)"}" and its title with it. ` +
        `The words are the last-resort catch's, not a sentence about the file: that was ` +
        `drawableSankey's and went with it`,
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

  // ---------------------- a malformed document, now that nothing re-checks it
  //
  // THESE FIVE FIXTURES USED TO BE FIVE ARMS, each asserting that
  // drawableSankey named the key it lacked. That gate is gone: every one of
  // these keys is `required` in schema/column.schema.json and encodeColumn
  // refuses to write bytes that fail it, so the client re-checking them was a
  // second implementation of a check Go already makes. tools/jscheck/contract
  // .mjs is where that claim lives now, and it reddens if the schema is
  // relaxed -- which is what makes this deletion safe rather than argued.
  //
  // WHAT IS ASSERTED HERE IS THE PROPERTY THAT SURVIVED, and it is fisc-bsg's:
  // whatever the fault, the page is never left reading one year's words over
  // another year's chart. showYear shapes and lays out before it writes a
  // word, so a throw from either lands with the page wholly the year it was
  // on; a throw from buildTable -- the LAST step of the repaint -- would not,
  // which is the split fisc-bsg is about and the reason this block exists at
  // all rather than being deleted with the gate.
  //
  // TWO OF THE FIVE NOW DRAW RATHER THAN REFUSE, and that is a real loss,
  // recorded here rather than discovered later: a schedule with no `sources`,
  // and a locator with no `pages`, are both guarded by citations(), so the
  // chart draws with provenance missing and no banner. The page is consistent
  // -- the words and the chart are the same year -- but a reader is not told.
  // Nothing on this side covers it; schema/column.schema.json requiring both
  // is what stops such a file being written, and a hand-edited one is the
  // residual.
  {
    const sources = [{ doc_id: "livermore-budget-fy2026-2027", pages: [66] }];
    const fixtures = [
      // No metadata at all beyond the column: the schedule states no sources.
      { key: "metadata.sources",
        doc: { schema_version: 1, metadata: {}, nodes: doc.nodes, links: doc.links } },
      { key: "links[].fact_ids",
        doc: { schema_version: 1, metadata: { fiscal_year: 2027, sources },
          nodes: doc.nodes, links: doc.links.map((l) => ({ ...l, fact_ids: undefined })) } },
      // ONE DEEPER. A locator carrying a doc_id and no pages passes anything a
      // top-level check could ask, and citations() does `for (const page of
      // source.pages)`. The doc id MUST be one CONFIG.docs carries, or
      // citations() returns at `if (!doc) continue` and never reaches pages --
      // and this fixture would be green because the gate fired earlier rather
      // than because anything was prevented.
      { key: "links[].locators[].pages",
        doc: { schema_version: 1, metadata: { fiscal_year: 2027, sources },
          nodes: doc.nodes,
          links: doc.links.map((l) => ({
            ...l, locators: [{ doc_id: "livermore-budget-fy2026-2027" }],
          })) } },
      { key: "links[].locators",
        doc: { schema_version: 1, metadata: { fiscal_year: 2027, sources },
          nodes: doc.nodes, links: doc.links.map((l) => ({ ...l, locators: undefined })) } },
      { key: "metadata.sources[].pages",
        doc: { schema_version: 1,
          metadata: { fiscal_year: 2027, sources: [{ doc_id: "livermore-budget-fy2026-2027" }] },
          nodes: doc.nodes, links: doc.links } },
    ];
    const split = [];
    const refused = [];
    const drewAnyway = [];
    for (const bad of fixtures) {
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
      const onFirst = Boolean(lede) && lede.textContent === "FY 2025-26 adopted";
      const tableFrozen = body.children.length === rowsFirst;
      // TWO CONSISTENT OUTCOMES AND ONE THAT IS NOT. Refused: a banner, the
      // old year's words, the old year's table. Drew: no banner, the new
      // year's words. Anything else is the words and the chart disagreeing,
      // which is the defect.
      if (banners.length && onFirst && tableFrozen) refused.push(bad.key);
      else if (!banners.length && !onFirst) drewAnyway.push(bad.key);
      else {
        split.push(`${bad.key} (${banners.length} banner(s), lede "${lede ? lede.textContent : "(none)"}", ` +
          `${body.children.length} rows against ${rowsFirst})`);
      }
    }
    out.push({
      name: "a malformed year document never leaves the words and the chart on different years",
      ok: split.length === 0,
      detail: split.length
        ? `${split.length} fixture(s) half-repainted: ${split.join("; ")}`
        : `${refused.length} refused with the page untouched [${refused}]; ` +
          `${drewAnyway.length} drew with provenance missing and no banner [${drewAnyway}] -- ` +
          `citations() guards both, so nothing here tells the reader; ` +
          `schema/column.schema.json requiring them is what stops such a file being written`,
    });
  }

  // ------------------------------------------------------ the column budget
  //
  // TWO CLAIMS, AND THE SECOND IS THE ONE WITH A DEFECT BEHIND IT. That a
  // viewport wide enough for a fourth column opens the page at four is the easy
  // half. That a reader who then steps DOWN is not silently overruled by the
  // next media change is the half that has to be built for: setting the budget
  // and leaving columnOverride null looks identical on screen, until the reader
  // drags the window across the threshold and their choice evaporates.
  //
  // THE MEDIA CHANGE IS DRIVEN BY CROSSING THE THRESHOLD IN BOTH DIRECTIONS.
  // Narrowing alone proves nothing here -- the viewport's own answer below the
  // threshold is three, which is what the reader chose -- so the page has to be
  // widened BACK for the two behaviours to differ at all.
  {
    const { app } = page({
      config,
      fetch: plannedFetch({ "data/sankey.json": { doc } }),
      // Above COLUMN_QUERIES' only threshold, which is chartWidth(4) plus the
      // stylesheet's own cushion. seam.mjs is what says the stub answers this.
      viewport: 2000,
    });
    await settle();
    const opened = app.columnBudget;
    const more = app.dom.byId.get("column-more");
    const fewer = app.dom.byId.get("column-fewer");
    const disabled = (/** @type {any} */ b) => b.getAttribute("disabled") !== null;
    // Both steppers are dead: this config declares no steps, so the page has
    // one width. So this pair no longer witnesses that wireColumns ran, and the
    // arm below drives stepColumns' plumbing rather than a gesture a reader
    // could make here. drill.mjs's "a chart with a second width offers it" is
    // where the control is driven as a reader drives it.
    const atCeiling = [disabled(more), disabled(fewer)].join("/");

    fewer.listeners.click[0]();
    const chosen = app.columnBudget;
    const saved = app.dom.storage.get("fisc-columns");
    const atFloor = [disabled(more), disabled(fewer)].join("/");

    app.dom.setViewport(800);
    const narrowed = app.columnBudget;
    app.dom.setViewport(2000);
    const held = app.columnBudget;

    // AND THE WAY BACK. Stepping to the count the viewport itself would give is
    // how a reader hands the decision back; without it the first press of
    // either button deafens the page to the window for the rest of the visit,
    // and "wins until cleared" would name a state with no exit.
    more.listeners.click[0]();
    const released = app.columnOverride;
    const cleared = app.dom.storage.has("fisc-columns");

    out.push({
      name: "a viewport that can carry four columns gets four, and a reader's step down holds against it",
      ok: opened === 4 && atCeiling === "true/true" &&
          chosen === 3 && saved === "3" && atFloor === "true/true" &&
          narrowed === 3 && held === 3 &&
          released === null && cleared === false,
      detail: `a 2000px window opens at ${opened} columns with (more/fewer) disabled ${atCeiling}; ` +
        `one press of the minus gives ${chosen}, stored as ${JSON.stringify(saved)}, with ` +
        `disabled ${atFloor}; narrowing to 800px leaves ${narrowed} and widening back to 2000px ` +
        `leaves ${held} -- the reader's choice outranks the window rather than being replaced by ` +
        `it -- and stepping back up to the window's own answer clears the override to ` +
        `${released} and removes the key (${cleared ? "still stored" : "gone"})`,
    });
  }

  // A SAVED CHOICE IS READ BACK, AND ONE THIS BUILD CANNOT HONOUR IS NOT.
  //
  // savedColumns is reached exactly once, during wireColumns, so without a
  // seeded localStorage nothing drives it and the whole of "your choice
  // survives a reload" is a path no check enters. The out-of-range case is the
  // arm with the judgement in it: a 5 left by a build with a higher ceiling is
  // DISCARDED rather than clamped, because clamping would put the page in the
  // overridden state -- deaf to the viewport -- on behalf of a reader who never
  // chose 4.
  {
    const fetch = () => plannedFetch({ "data/sankey.json": { doc } });
    const saved = page({ config, fetch: fetch(), storage: { "fisc-columns": "4" } });
    const stale = page({ config, fetch: fetch(), storage: { "fisc-columns": "9" } });
    await settle();
    out.push({
      name: "a saved column count is honoured on the next visit, and one out of range is discarded",
      ok: saved.app.columnBudget === 4 && saved.app.columnOverride === 4 &&
          stale.app.columnBudget === 3 && stale.app.columnOverride === null,
      detail: `a stored "4" opens a page with no viewport at ${saved.app.columnBudget} columns ` +
        `(override ${saved.app.columnOverride}), where the window alone would give 3; a stored ` +
        `"9" opens at ${stale.app.columnBudget} with override ${stale.app.columnOverride}, so the ` +
        `page follows the window rather than honouring a choice it cannot offer`,
    });
  }

  // A BUDGET CHANGE REDRAWS THE RUNG THE READER IS ON, AND DOES NOTHING ELSE TO
  // THE STACK.
  //
  // Four things have to be true at once and each has its own way of being
  // false: the chart gains a column (the change took effect), the stack keeps
  // both its rungs (it redrew rather than popped), nothing is fetched a second
  // time (rung.doc is already recorded), and focus is still on the control for
  // the rung the reader is on rather than the one above it. The last doubles as
  // the pop detector: a popped stack restores focus to the OUTER rung's return
  // control, which reads differently.
  {
    // The fund window, which is the only shape this site ships that has a
    // fourth column to gain: the fund step declares tiers {2,3,4,5} widening
    // by {5}. On the overview a budget change is correctly a no-op.
    const { app, fetch, main } = await openedChain(["fund-group/general", "fund/100"]);
    const crumb = app.dom.byId.get("breadcrumb");
    const innermostControl = () => {
      const buttons = (crumb.children || [])
        .filter((c) => String(c.tagName || "").toLowerCase() === "button");
      return buttons.length ? buttons[buttons.length - 1].textContent : "";
    };
    const wasOn = innermostControl();
    // Focus in the chart, which is the state restoreFocus exists for: without
    // it redrawStack has nothing to restore and "focus did not move" would be
    // true because nothing had it.
    app.dom.document.activeElement = app.dom.document.getElementById("chart");

    const before = {
      depth: app.drilled.length,
      path: app.drilled.map((/** @type {any} */ r) => r.id).join(" > "),
      columns: app.drawnColumns(),
      asked: fetch.asked.length,
    };
    app.dom.byId.get("column-more").listeners.click[0]();
    await settle();
    const after = {
      depth: app.drilled.length,
      path: app.drilled.map((/** @type {any} */ r) => r.id).join(" > "),
      columns: app.drawnColumns(),
      asked: fetch.asked.length,
    };
    const focused = app.dom.focused ? app.dom.focused.textContent : "";
    const banners = refusals(main).length;

    out.push({
      name: "a budget change redraws the rung the reader is on without popping it or refetching",
      ok: before.columns === 3 && after.columns === 4 &&
          before.depth === 2 && after.depth === 2 && after.path === before.path &&
          after.asked === before.asked && banners === 0 &&
          wasOn !== "" && focused === wasOn,
      detail: `the fund window went from ${before.columns} to ${after.columns} columns; the stack ` +
        `reads "${after.path}" at depth ${after.depth} (was "${before.path}" at ${before.depth}); ` +
        `${after.asked} fetch(es) against ${before.asked} before the press, so rung.doc was reused; ` +
        `${banners} banner(s); focus is on "${focused}" (was "${wasOn}"), which is the control for ` +
        `the rung the reader is on and not the one above it`,
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

  // ------------------------------------------- a column from another build
  //
  // THE ONE CHECK THAT REPLACED EIGHT, and the only one on this side that Go
  // cannot make. encodeColumn refuses to write a column that fails
  // schema/column.schema.json, so no file this export produced is the wrong
  // shape; what no schema can express is that the reader's copy of the column
  // and their copy of the page came out of different runs. The site publishes
  // no cache-busting, so that is not hypothetical -- it is how every key
  // drawableSankey ever gained reached a reader for the first time.
  //
  // THE BODY IS WELL-FORMED ON PURPOSE. A malformed one would be refused by
  // something else and this arm would be green for the wrong reason.
  {
    const { app, main } = page({
      config,
      fetch: plannedFetch({
        "data/sankey.json": { doc },
        "data/sankey-2027.json": { doc },
        "fy2027-adopted.json": {
          doc: Object.assign(columnOf({ sankey: doc }, { fiscal_year: 2027, basis: "adopted" }),
            { generated_by: "fisc other" }),
        },
      }),
    });
    await settle();
    const rowsBefore = refusals(main).length;
    clickYear(app, "sankey-2027");
    await settle();

    const banners = refusals(main);
    const text = banners.length ? banners[0].textContent : "";
    const lede = app.dom.byId.get("lede-year");
    const onFirst = Boolean(lede) && lede.textContent === "FY 2025-26 adopted";
    out.push({
      name: "a column from another build is refused, and the page stays on the year it drew",
      ok: rowsBefore === 0 && banners.length === 1 && Boolean(onFirst) &&
        text.includes("fisc other") && text.includes("holding a copy"),
      detail: `${banners.length} banner(s)` + (banners.length ? `: "${text}"` : "") +
        `; the page still reads "${lede ? lede.textContent : "(no lede)"}". The document is ` +
        `well-formed -- only its stamp differs -- so nothing but this comparison could ` +
        `have refused it`,
    });
  }

  // ------------------------------------------------- the rung answer's fetch
  //
  // WHICH NODES A COLUMN DRAWS IS GO'S ANSWER, FETCHED. So the page has a file
  // it cannot draw a rung without, and every way that file can fail to arrive
  // is a way the page could draw one anyway from whatever it had -- which is a
  // chart of public money composed from a half-read answer, refusing nothing.
  //
  // THE SHAPE ASSERTED IS "BANNER, AND NOTHING DRAWN". Not "banner" alone: a
  // refusal over a chart that drew is the fisc-bsg split, and here it would be
  // worse than that, because the chart it sat over would be the one nobody
  // vetted. The year's own document must not even be ASKED FOR, which is what
  // says the refusal happened before the draw rather than during it.
  {
    const rungConfig = Object.assign({}, config, { rungs: RUNGS_PATH });
    const answer = rungsAnswer();
    const truncated = JSON.parse(JSON.stringify(answer));
    delete truncated.columns[0].rungs[0].draws[0].ids;
    for (const tc of [
      {
        name: "a rung answer the server will not serve",
        plan: { [RUNGS_PATH]: { ok: false, status: 404 } },
        says: "HTTP 404",
      },
      // THE SHAPE ROWS ARE GONE AND THE COPY ROW REPLACES THEM. A rung answer
      // of another schema_version, and one missing draws[].ids, were two rows
      // here; both are states schema/rungs.schema.json refuses and encodeRungs
      // will not write, so the client re-checking them was a second
      // implementation (tools/jscheck/contract.mjs holds that claim now).
      //
      // What Go cannot see is which COPY the browser has -- rungs.json and the
      // page are separate files with no cache-busting between them, and each
      // is valid on its own. `truncated` above is kept as the BODY of this
      // row on purpose: it proves the refusal is about the stamp and not about
      // the shape, because a correctly-shaped answer from another build is
      // refused just the same.
      {
        name: "a rung answer from another build",
        plan: { [RUNGS_PATH]: { doc: Object.assign(truncated, { generated_by: "fisc other" }) } },
        says: "fisc other",
      },
    ]) {
      const fetch = plannedFetch(Object.assign({ "data/sankey.json": { doc } }, tc.plan));
      const { app, main } = page({ config: rungConfig, fetch });
      await settle();
      const banners = refusals(main);
      const names = banners.length ? banners[0].textContent : "";
      const drew = app.dom.byId.get("lede-year");
      const askedYear = fetch.asked.includes(config.years[0].path);
      out.push({
        name: `${tc.name} refuses the page in words and draws nothing`,
        ok: fetch.asked[0] === RUNGS_PATH && banners.length === 1 &&
          names.includes(RUNGS_PATH) && names.includes(tc.says) &&
          !askedYear && !(drew && drew.textContent),
        detail: fetch.asked[0] !== RUNGS_PATH
          ? `main() asked for ${JSON.stringify(fetch.asked)} first, so the rung answer was ` +
            `not what failed and this check reached none of the state it is named for`
          : `${banners.length} banner(s) naming ${RUNGS_PATH} and "${tc.says}" ` +
            `(${banners.length ? JSON.stringify(names) : "none"}); the year document was ` +
            `${askedYear ? "FETCHED ANYWAY" : "never asked for"} and the lede reads ` +
            `"${drew ? drew.textContent : ""}"`,
      });
    }
  }

  return out;
}
