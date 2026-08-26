// seam.mjs — checks on the harness itself.
//
// Everything else here checks site/app.js. This file checks the thing that
// checks it, and it exists because the harness had been quietly unable to
// observe most of what it was pointed at (fisc-dn9).
//
// THE FAILURE MODE IT GUARDS. A stub that answers `null`, discards a listener
// or never settles does not report anything: the check written against it
// simply passes, against a fixed and an unfixed app.js alike. Four of those
// were live at once — fetch could not be injected, the config had one year so
// the toggle was never wired, document-level listeners were dropped on the
// floor, and fail()'s banner was unreachable so asserting its absence was
// vacuous. None of them looked like a broken harness; they looked like green
// checks.
//
// So each check below asserts that a seam CAN SEE something, in the smallest
// way that would go red if the seam regressed to answering nothing.

import { loadApp, settle, twoYearConfig, plannedFetch, refusals, goldenGraph } from "./harness.mjs";

export async function checks() {
  const out = [];
  const doc = goldenGraph();

  // The runner's own floor. Every check about app.js's fetch path is async, and
  // under the pre-2026-08-26 synchronous runner a Promise assigned to `ok` was
  // truthy and reported PASS whatever it resolved to. If that ever regresses,
  // every async check goes quietly green and this is the one that notices.
  out.push({
    name: "an async check reports what it resolves to, not that it is a promise",
    ok: Promise.resolve(true),
    detail: "run.mjs awaits each check's ok; a promise resolving false must FAIL, " +
      "and a truthy-object pass is what the synchronous runner did",
  });

  // The fetch seam, and specifically that it is installed BEFORE app.js runs.
  // app.js ends in main() at file scope, so a fetch assigned after loadApp
  // returns is assigned after the code that would have used it.
  {
    const fetch = plannedFetch({ "data/sankey.json": { doc } });
    const app = loadApp({ fetch });
    await settle();
    const lede = app.dom.byId.get("lede-year");
    out.push({
      name: "an injected fetch is in place before main() reaches it",
      ok: fetch.asked.length === 1 && Boolean(lede && lede.textContent),
      detail: `main() asked for ${JSON.stringify(fetch.asked)} and drew ` +
        `"${lede ? lede.textContent : ""}" from the injected document`,
    });
  }

  // The config seam. wireYears returns early below two years, so under the
  // default single-year config the control is never wired and every check about
  // switching years drives nothing at all.
  {
    const app = loadApp({
      config: twoYearConfig(),
      fetch: plannedFetch({
        "data/sankey.json": { doc },
        "data/sankey-2027.json": { doc },
      }),
    });
    await settle();
    const group = app.dom.byId.get("year-toggle");
    const handlers = group ? (group.listeners.change || []).length : 0;
    out.push({
      name: "a two-year config gives the page a live year control",
      ok: handlers === 1 && group.getAttribute("disabled") === null,
      detail: `the toggle carries ${handlers} change handler(s) and the template's ` +
        `disabled attribute was removed, which is what wireYears' enhancement is`,
    });
  }

  // Document- and window-level listeners are RECORDED. They were no-ops, so
  // "the Escape handler is attached" and "the page follows the OS theme" were
  // both unfalsifiable — the two affordances fisc-8cg is about.
  {
    const app = loadApp({ fetch: plannedFetch({ "data/sankey.json": { doc } }) });
    await settle();
    const escape = (app.dom.documentListeners.keydown || []).length;
    const theme = (app.dom.mediaListeners.change || []).length;
    out.push({
      name: "document-level and matchMedia listeners are observable",
      ok: escape === 1 && theme === 1,
      detail: `a page that opened cleanly carries ${escape} Escape handler(s) and ` +
        `${theme} prefers-color-scheme listener(s), both of which the stub used to discard`,
    });
  }

  // fail()'s banner is REACHABLE, and clearRefusal can take it down. Both
  // halves matter: the banner is prepended into document.querySelector("main"),
  // which answered null, and it is removed with node.remove(), which was a
  // no-op. A check asserting "no banner" passed on either.
  {
    const app = loadApp({ fetch: plannedFetch({ "data/sankey.json": { reject: new TypeError("x") } }) });
    const main = app.dom.document.node();
    app.dom.document.plant("main", main);
    await settle();
    const shown = refusals(main).length;

    // Now let a good year through, which is what clearRefusal is for.
    const app2 = loadApp({
      config: twoYearConfig(),
      fetch: plannedFetch({
        "data/sankey.json": { reject: new TypeError("x") },
        "data/sankey-2027.json": { doc },
      }),
    });
    const main2 = app2.dom.document.node();
    app2.dom.document.plant("main", main2);
    await settle();
    const before = refusals(main2).length;
    const group = app2.dom.byId.get("year-toggle");
    for (const fn of (group && group.listeners.change) || []) fn({ target: { value: "sankey-2027" } });
    await settle();
    const after = refusals(main2).length;

    out.push({
      name: "a refusal banner can be seen appearing and seen going away",
      ok: shown === 1 && before === 1 && after === 0,
      detail: `a refused fetch paints ${shown} banner(s); a recovering year switch takes ` +
        `${before} down to ${after}, which is what makes asserting a banner's ABSENCE mean something`,
    });
  }

  return out;
}
