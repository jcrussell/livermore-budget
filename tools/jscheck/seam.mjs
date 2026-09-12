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

import {
  loadApp, settle, settleCheck, twoYearConfig, plannedFetch, refusals, goldenGraph,
  KNOWN_SELECTORS, selectorsIn, parseResidualLiteral, parseStepShapes,
} from "./harness.mjs";

export async function checks() {
  const out = [];
  const doc = goldenGraph();

  // THE STUB'S REACH, PINNED AGAINST THE FILE IT IS POINTED AT.
  //
  // The stub answers one parsed shape and whatever a check plants, so what it
  // can SEE is not visible from its code. Two of app.js's selectors were
  // answered by nothing and the code behind them ran in no check: buildTable()
  // returned at `if (!body) return` every time and paint()'s legend loop
  // iterated an empty list, both silently green (fisc-wcy).
  //
  // This is deliberately STATIC. The obvious alternative -- have the stub throw
  // on a selector it cannot answer -- was measured and is worse: fail() and
  // clearRefusal() both CALL querySelector, so the throw lands inside app.js's
  // own error path and recurses into an unhandled rejection that takes the run
  // down. harness.mjs already settles that case: a stub incomplete in a way
  // app.js turns into its own error path is worse than one obviously
  // incomplete. A scan cannot be swallowed by a banner.
  {
    const app = loadApp();
    const found = selectorsIn(app.source);
    const declared = new Set(Object.keys(KNOWN_SELECTORS));
    const undeclared = [...found].filter((sel) => !declared.has(sel)).sort();
    const stale = [...declared].filter((sel) => !found.has(sel)).sort();
    const byHow = {};
    for (const sel of found) {
      const how = (KNOWN_SELECTORS[sel] || {}).how || "undeclared";
      (byHow[how] ||= []).push(sel);
    }
    const summary = Object.keys(byHow).sort()
      .map((how) => `${byHow[how].length} ${how}`).join(", ");
    out.push({
      name: "every selector app.js uses is one the stub declares it can answer",
      ok: undeclared.length === 0 && stale.length === 0,
      detail: undeclared.length || stale.length
        ? `undeclared in app.js: ${JSON.stringify(undeclared)}; ` +
          `declared but no longer in app.js: ${JSON.stringify(stale)}`
        : `${found.size} selectors, all declared (${summary}) -- and "unanswered" ` +
          `names code that runs in no check, which is printed here rather than implied`,
    });
  }

  // The runner's own floor. Every check about app.js's fetch path is async, and
  // under the pre-2026-08-26 synchronous runner a Promise assigned to `ok` was
  // truthy and reported PASS whatever it resolved to. If that ever regresses,
  // every async check goes quietly green and this is the one that notices.
  // IT ASSERTS ON settleCheck AND NOT ON ITS OWN `ok`, and the first attempt at
  // this check shows why. `ok: Promise.resolve(true)` looks like a tripwire and
  // is none: Boolean(await p) and Boolean(p) are BOTH true for any promise, so
  // it passed under exactly the synchronous runner it claimed to detect.
  // Neither does resolving false help -- un-awaited that is still a truthy
  // object. NO promise-valued ok can ever report false under a synchronous
  // runner, so the only falsifiable form is to call the runner's own resolver
  // and look at what comes back.
  {
    const falsey = await settleCheck({ name: "probe", ok: Promise.resolve(false), detail: "d" });
    const thrower = await settleCheck({
      name: "probe", detail: "d",
      get ok() { throw new Error("a check that throws"); },
    });
    out.push({
      name: "the runner resolves a check's promise and survives one that throws",
      ok: falsey.ok === false && thrower.ok === false &&
        thrower.detail.includes("the check itself threw"),
      detail: `a promise resolving false is reported as ${falsey.ok}, and a check that ` +
        `throws is reported as ${thrower.ok} rather than taking the run down; without ` +
        `the await every async check in this directory reports PASS whatever it finds`,
    });
  }

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


  // THE RESIDUAL PARSE COUNTS TWICE, AND BOTH COUNTS MUST BE ABLE TO DISAGREE.
  //
  // parseResidualLiteral returns the set every residual check drives the client
  // under, so a PARTIAL set would measure a client the site does not run -- and
  // silently, since each entry it did parse is well-formed. The guard against
  // that is counting the literal's entry lines and comparing.
  //
  // It counted `^\t"` until fisc-0flg: the same assumption the entry pattern
  // makes, spelled a second way. A key neither pattern followed was skipped by
  // BOTH, so got === keys held and the partial set was returned. The mutation
  // is the third case below -- an unquoted key, which is what a Go const key
  // looks like -- and it parses 1 of 2 entries.
  {
    const lit = (body) => `var residualNodes = map[string]string{\n${body}\n}\n`;
    const good = lit('\t"a/b": "one",\n\n\t"c/d": "two",');
    const unquoted = lit('\t"a/b": "one",\n\n\tconstKey: "two",');
    const emptyReason = lit('\t"a/b": "",');
    const threw = (src) => {
      try {
        parseResidualLiteral(src);
        return false;
      } catch {
        return true;
      }
    };
    const parsed = Object.keys(parseResidualLiteral(good)).length;
    out.push({
      name: "the residual literal's entry count refuses a key the entry pattern cannot follow",
      ok: parsed === 2 && threw(unquoted) && threw(emptyReason) && !threw(good),
      detail: `a well-formed literal parses ${parsed} of 2 entries and does not throw; an ` +
        `unquoted key throws rather than returning 1 of 2, which is what the old ` +
        `count could not see; an empty reason throws`,
    });
  }


  // THE STEP PARSE COUNTS TWO DIFFERENT MARKERS, for the reason the residual
  // parse does -- and it did not, at first. Both its entry pattern and its
  // count were `^\t{4}From:`, so a step whose From sat anywhere else was
  // skipped by BOTH and the counts agreed with each other while the set was
  // short. The second case below is the measured shape: reflowing a step's
  // opener to `{From: 4,`, which gofmt accepts, dropped that step and moved its
  // cap onto the one before it. fisc-0flg is the same defect in the residual
  // parse; this is it repeated in the fix that cited it.
  {
    const lit = (body) => `\t\tspine.Steps = []export.DrillStep{\n${body}\n\t\t}\n`;
    const step = (key, after, from, tiers, cap) =>
      `\t\t\t{\n\t\t\t\tKey:   ${JSON.stringify(key)},\n` +
      `\t\t\t\tAfter: ${JSON.stringify(after)},\n` +
      `\t\t\t\tFrom:  ${from},\n\t\t\t\tTiers: []int{${tiers}},\n` +
      `\t\t\t\tCaps:  []export.TierCap{{Tier: ${cap[0]}, Cap: ${cap[1]}}},\n\t\t\t},`;
    const two = lit(step("group", "", 2, "0, 3, 4", [3, 8]) + "\n" +
      step("division", "group", 4, "4, 5", [5, 8]));
    const reflowed = two.replace("\t\t\t{\n\t\t\t\tKey:   \"division\",", "\t\t\t{Key: \"division\",");
    const noFrom = two.replace("\t\t\t\tFrom:  4,\n", "");
    // A STEP WITH NO KEY IS A STEP THIS PARSE DID NOT READ. The Go type
    // requires one on every step, so the shape cannot reach data.go -- which
    // is exactly why the parse has to refuse it rather than return a list one
    // field short and let a check measure it.
    const noKey = two.replace("\t\t\t\tKey:   \"division\",\n", "");
    // A NESTED STEP IS THE SHAPE THIS PARSE MUST NOT HALF-READ. Parentage is
    // declared by key and the list stays flat for exactly this reason: an
    // entry indented as a child is invisible to the brace pattern, so the
    // parse would return the outer step alone and every check would measure a
    // chain the site does not ship. What catches it is the entry count
    // disagreeing with the field counts, which is why those are counted at
    // all.
    const nested = lit(step("group", "", 2, "0, 3, 4", [3, 8]) + "\n" +
      step("division", "group", 4, "4, 5", [5, 8]).replace(/^\t{3}/gm, "\t\t\t\t"));
    // THE REFUSAL IS READ, NOT COUNTED. `did it throw` is satisfied by a
    // TypeError off an unguarded dereference, which is green for a reason that
    // has nothing to do with the guard -- measured: deleting the `if (!key)`
    // throw left a bare `key[1]` that threw anyway and the arm stayed green.
    // Matching the message is what tells a refusal from a crash.
    const refusal = (/** @type {string} */ src) => {
      try {
        parseStepShapes(src);
        return "";
      } catch (e) {
        return String((e && e.message) || e);
      }
    };
    // THE CONTROL IS GUARDED TOO. An unguarded call here threw out of checks()
    // and was reported as "a whole check module threw", taking every other arm
    // in this file out of the output with it -- the shape drill.mjs's carried
    // arm was fixed for one commit ago, reintroduced in that same commit. A
    // control that cannot parse is this arm's own failure and says so.
    const read = (/** @type {string} */ src) => {
      try {
        return { steps: parseStepShapes(src), threw: false };
      } catch {
        return { steps: [], threw: true };
      }
    };
    const caps = (/** @type {any[]} */ steps, /** @type {number} */ i) =>
      steps[i] && steps[i].caps ? steps[i].caps.length : -1;
    const control = read(two);
    const base = control.steps;
    const reflowedRead = read(reflowed);
    const reflowedSteps = reflowedRead.steps;
    const reflowedThrew = reflowedRead.threw;
    // A reflow must not lose a step OR a step's caps. Either reading it whole
    // or throwing is acceptable; returning one step with two steps' caps is
    // not, and neither is returning two steps with the last one's caps gone --
    // a slice-end regression drops the final entry's fields, so both entries'
    // caps are asserted rather than only the first's.
    const survived = reflowedThrew || (reflowedSteps.length === 2 &&
      reflowedSteps[1].from === 4 && reflowedSteps[1].after === "group" &&
      reflowedSteps[0].caps.length === 1 && reflowedSteps[1].caps.length === 1);
    out.push({
      name: "a reflowed step is read or refused, never dropped with its caps moved onto the step before it",
      ok: !control.threw && base.length === 2 && base[1].from === 4 &&
          base[0].key === "group" && base[1].after === "group" && base[0].after === "" &&
          caps(base, 0) === 1 && caps(base, 1) === 1 &&
          survived && /From/.test(refusal(noFrom)) && /Key/.test(refusal(noKey)) &&
          /Key/.test(refusal(nested)),
      // THE DETAIL DEREFERENCES NOTHING EITHER. Guarding only `ok` left this
      // string reading base[0].caps on a control that threw, so the arm still
      // took the module down -- the same defect one line lower than where it
      // was fixed. Every index here goes through caps().
      detail: `${control.threw ? "THE CONTROL LITERAL DID NOT PARSE, so nothing below is evidence"
        : `two well-formed steps parse as ${base.length} with ${caps(base, 0)} and ` +
          `${caps(base, 1)} cap(s), the second declared after ` +
          `${JSON.stringify(base[1] ? base[1].after : null)}`}; the gofmt-legal reflow ` +
        `${reflowedThrew ? "throws" : `parses ${reflowedSteps.length} step(s) with ` +
          `${caps(reflowedSteps, 0)} and ${caps(reflowedSteps, 1)} cap(s)`}; ` +
        `a step with no From is refused with ${JSON.stringify(refusal(noFrom))}, one ` +
        `with no Key with ${JSON.stringify(refusal(noKey))}, and a step indented as a ` +
        `nested literal with ${JSON.stringify(refusal(nested))}`,
    });
  }

  return out;
}
