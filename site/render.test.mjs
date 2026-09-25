// render.test.mjs — what render() put in the SVG, read back off the DOM.
//
// Every claim here is about the DRAWING: the marks, ribbons, attributes, words
// and handlers the page holds after a state is driven, held against the chart
// the client laid and against Go's rung answer for which ids a column accounts
// for. It re-derives no figure Go emitted -- no cent sum, no membership -- and
// holds no copy of a function of app.js: what a mark should read is asked of
// the shipped nodeClass and nodeFlags and compared with what the mark reads.
//
// The tests in the first block drive a handful of states per published column
// at two column budgets and read six things off each; the second block is the
// drill's own rendering claims, over the same pinned page.

import { describe, test, before } from "node:test";
import assert from "node:assert/strict";
import { readFileSync, readdirSync } from "node:fs";
import { join } from "node:path";

import {
  bootedApp, opened, expandAll, everyOffer, settle, refusals, rungsFixture, columnFixture, pageFixture, fire, repoRoot,
} from "./testlib.mjs";

const CONFIG = pageFixture().config;
/** The two published columns, as the pinned page declares them. */
const YEARS = CONFIG.years.map((y) => ({
  stem: y.stem, label: y.label, path: y.path, chartTitle: y.chart_title,
  fixture: y.path.replace(/\.json$/, ""), config: y,
}));

/** @param {string[]} path */
const keyOf = (path) => (path.length ? path.join(" > ") : "(the overview)");

/**
 * A failure list as a detail line: the count whole and the list cut, so one
 * deleted `.attr()` line -- wrong on every mark of every state -- does not
 * bury the other arms' output.
 * @param {string[]} found
 */
const firstOf = (found) =>
  `${found.length} of them:\n      ${found.slice(0, 3).join("\n      ")}` +
  (found.length > 3 ? `\n      ... and ${found.length - 3} more` : "");

/**
 * The marks the client drew, read off the SVG.
 *
 * THE ONE DOM READER, and every arm goes through it: blinding it must take
 * the whole file red on its counters, which is what stops any arm passing
 * with nothing compared.
 * @param {Element} chart
 */
const marksIn = (chart) => [...chart.querySelectorAll("g.node")];

/** @param {Element} chart */
const ribbonsIn = (chart) => [...chart.querySelectorAll("path.link")];

/** @param {Element} m a drawn g.node */
const haloOf = (m) => [...m.children].find((c) =>
  c.tagName === "text" && c.getAttribute("class") === "halo");

/** The words drawn in a mark's label, tspan by tspan. */
const tspansOf = (m) => {
  const text = haloOf(m);
  return text ? [...text.children].map((c) => c.textContent) : [];
};

/** The flag tspan a mark draws, or null where render() drew none. */
const flagOf = (m) => {
  const text = haloOf(m);
  return text ? [...text.children].find((c) => c.getAttribute("class") === "flag") : null;
};

/**
 * The number a folded tail promises the reader, read off the words drawn in
 * it; -1 when nothing drawn in it reads as a count.
 * @param {Element} mark
 */
function tailPromise(mark) {
  for (const w of tspansOf(mark)) {
    const found = /^(\d+) smaller /.exec(w);
    if (found) return Number(found[1]);
  }
  return -1;
}

/**
 * The ids Go's answer accounts for at one rung, each with the tier it stands
 * at: the opened node's parts, the marks carried beside them, and the marks
 * the client adds of its own, which get -1 so a fold can never excuse them.
 * @param {any} rung
 * @returns {Map<string, number>}
 */
function answeredIDs(rung, step) {
  const ids = new Map();
  for (const d of rung.draws) {
    for (const id of d.ids) ids.set(id, d.tier);
    for (const id of d.carried || []) ids.set(id, d.tier);
  }
  for (const m of rung.marks || []) {
    ids.set(m.id, -1);
    // A RESIDUAL'S LEAVING ENDPOINTS stand at the step's last declared tier
    // (export.ResidualOf), and are answered on the mark rather than in a
    // column; carryResidual draws them there and drops the leg with the
    // column where the budget does not buy it, so they are read here as ids
    // of that column.
    if (m.role !== "residual") continue;
    for (const e of m.ends || []) {
      if (!ids.has(e)) ids.set(e, step.tiers[step.tiers.length - 1]);
    }
  }
  return ids;
}

/** How many nodes the widest column of one rung holds, unfolded. */
const widest = (rung) => Math.max(...rung.draws.map((d) => d.ids.length + (d.carried || []).length));

/**
 * The rungs driven in one column, chosen by what each exercises: the
 * overview, the first depth-1 rung, the rung holding the widest column (where
 * a fold engages if anywhere), the first with a residual, the first with a
 * gap, and the deepest. Selected off the artifact in its own order.
 * @param {any[]} rungs
 * @returns {string[][]}
 */
function statesIn(rungs) {
  const chosen = [[]];
  const deepest = Math.max(...rungs.map((r) => r.path.length));
  const wanted = [
    rungs.find((r) => r.path.length === 1),
    rungs.reduce((a, b) => (widest(b) > widest(a) ||
      (widest(b) === widest(a) && keyOf(b.path) < keyOf(a.path)) ? b : a)),
    rungs.find((r) => (r.marks || []).some((m) => m.role === "residual")),
    rungs.find((r) => (r.marks || []).some((m) => m.role === "gap")),
    rungs.find((r) => r.path.length === deepest),
  ];
  for (const r of wanted) {
    if (r && !chosen.some((p) => keyOf(p) === keyOf(r.path))) chosen.push(r.path);
  }
  return chosen;
}

/**
 * The depth-1 rung of the fund-group step whose column is widest: the view
 * the fund cap is for.
 * @param {any[]} rungs
 */
function worstOf(rungs) {
  const groups = rungs.filter((r) => r.path.length === 1 && r.step === "fund-group");
  return groups.reduce((a, b) => (widest(b) > widest(a) ? b : a)).path[0];
}

/** Go's rungs for one column. */
function rungsOf(stem) {
  const artifact = rungsFixture();
  if (artifact.schema_version !== 5) {
    throw new Error(`testdata/rungs.json declares schema_version ${artifact.schema_version}; this file reads 5`);
  }
  const column = artifact.columns.find((c) => c.stem === stem);
  if (!column) throw new Error(`testdata/rungs.json answers for no column with stem ${stem}`);
  return column.rungs;
}

/**
 * Puts the app on the chart one path names, from the overview, expanding on
 * the way in and not at the end: a node an outer chart's cap folded away
 * cannot be clicked until that column is expanded, and the state measured is
 * the chart as a reader is first shown it, fold and all.
 */
async function goTo(app, path) {
  app.drillUp(0);
  await settle();
  for (const id of path) {
    expandAll(app);
    await opened(app, id);
  }
}


/** The step the pinned page declares under one key. */
function stepByKey(key) {
  const found = CONFIG.steps.find((s) => s.key === key);
  if (!found) throw new Error(`the pinned page declares no step keyed ${key}`);
  return found;
}

/** The breadcrumb's children as class:text. */
const crumbs = (document) =>
  [...document.getElementById("breadcrumb").children].map((c) => `${c.className}:${c.textContent}`);

/**
 * Drives every chosen state of one column at two budgets and reads six things
 * off each, returning what disagreed and what was counted.
 */
async function drive(year) {
  const rungs = rungsOf(year.stem);
  const answers = new Map(rungs.map((r) => [keyOf(r.path), r]));
  const wrong = { reach: [], ribbons: [], written: [], gestures: [], banner: [], tails: [] };
  const seen = {
    states: 0, answered: 0, marks: 0, attributes: 0, ribbons: 0, gestures: 0,
    tails: 0, ownMarks: 0, offscreen: 0, expansions: 0, revealed: 0, refused: 0,
    emphasis: 0, focus: 0,
  };

  for (const width of [3, 4]) {
    const { app, document } = await bootedApp({ checkedStem: year.stem });
    app.setColumnBudget(width);
    await settle();
    const chart = document.getElementById("chart");
    const content = document.querySelector("main");
    if (!content) throw new Error("the page has no <main>, so a refusal banner would have nowhere to be");

    for (const path of statesIn(rungs)) {
      const where = `${year.label} at ${width} columns, ${keyOf(path)}`;
      seen.states++;
      // A STATE THAT THROWS IS A STATE THAT DID NOT DRAW, reported as one
      // rather than left to take the file down.
      try {
        await goTo(app, path);
      } catch (e) {
        wrong.banner.push(`${where}: redrawing it threw -- ${e && e.message ? e.message : String(e)}`);
        try { app.drillUp(0); } catch { /* reported above */ }
        continue;
      }

      const marks = marksIn(chart);
      const ribbons = ribbonsIn(chart);

      // (e) no banner -- first, and on every state: every arm below reads a
      // DOM that app.js only fills when render() ran to the end.
      const banners = refusals(document);
      const groups = [...chart.children].map((c) => c.getAttribute("class"));
      if (banners.length) {
        wrong.banner.push(`${where}: the page drew ${banners.length} refusal banner(s): ${banners[0].textContent}`);
      }
      if (!groups.includes("links") || !groups.includes("nodes") || !marks.length) {
        wrong.banner.push(`${where}: #chart holds [${groups.join(", ")}] and ` +
          `${marks.length} mark(s), which is not a chart that drew`);
      }

      // (a) every node reaches a mark: three sets and not two. The DOM
      // against the graph the client laid says every node it decided on got
      // drawn; the graph against Go's answer says the decision was Go's.
      const drawn = marks.map((m) => m.__data__.id);
      const laid = app.projection.nodes.map((n) => n.id);
      const missing = laid.filter((id) => !drawn.includes(id));
      const extra = drawn.filter((id) => !laid.includes(id));
      const twice = drawn.length !== new Set(drawn).size;
      if (missing.length || extra.length || twice) {
        wrong.reach.push(`${where}: ${laid.length} node(s) laid and ${drawn.length} drawn` +
          (missing.length ? `; laid and never drawn: ${missing.join(", ")}` : "") +
          (extra.length ? `; drawn and never laid: ${extra.join(", ")}` : "") +
          (twice ? `; only ${new Set(drawn).size} of the marks are distinct` : ""));
      }
      seen.marks += marks.length;

      const answer = answers.get(keyOf(path));
      if (answer) {
        seen.answered++;
        seen.ownMarks += (answer.marks || []).length;
        const tails = marks.filter((m) => app.isAggregate(m.__data__.id));
        const folded = new Set(tails.map((m) => m.__data__.tier));
        seen.tails += tails.length;
        const columns = new Set(app.activeTiers());
        const want = answeredIDs(answer, app.CONFIG.steps.find((s) => s.key === answer.step));
        const tailIDs = [...folded].map((t) => app.aggregateID(t));
        const unanswered = drawn.filter((id) => !want.has(id) && !tailIDs.includes(id));
        const offscreen = [...want.keys()].filter((id) => want.get(id) !== -1 && !columns.has(want.get(id)));
        const undrawn = [...want.keys()].filter((id) => !drawn.includes(id) &&
          !folded.has(want.get(id)) && !offscreen.includes(id));
        seen.offscreen += offscreen.length;
        if (unanswered.length || undrawn.length) {
          wrong.reach.push(`${where}: Go accounts for ${want.size} id(s), the chart draws ` +
            `${drawn.length} of which ${tails.length} stand(s) for a folded column, and ` +
            `${offscreen.length} stand(s) at a column this budget drops` +
            (undrawn.length ? `; Go answers, no mark carries and no tail stands for: ${undrawn.join(", ")}` : "") +
            (unanswered.length ? `; drawn and Go accounts for none: ${unanswered.join(", ")}` : ""));
        }
      } else if (path.length) {
        wrong.reach.push(`${where}: testdata/rungs.json answers for no such rung`);
      }

      // (b) every link reaches a ribbon, and both its ends are marks here.
      const laidLinks = app.projection.links.length;
      if (ribbons.length !== laidLinks) {
        wrong.ribbons.push(`${where}: ${laidLinks} link(s) laid, ${ribbons.length} ribbon(s) drawn`);
      }
      for (const p of ribbons) {
        seen.ribbons++;
        const d = p.getAttribute("d");
        const stroke = Number(p.getAttribute("stroke-width"));
        const label = p.getAttribute("aria-label") || "";
        const ends = [p.__data__.source.id, p.__data__.target.id];
        const orphan = ends.filter((id) => !drawn.includes(id));
        if (!d || !(stroke > 0) || !label || orphan.length) {
          wrong.ribbons.push(`${where}: the ribbon ${ends.join(" -> ")} is drawn ` +
            `d=${JSON.stringify(d)} stroke-width=${JSON.stringify(p.getAttribute("stroke-width"))} ` +
            `aria-label=${JSON.stringify(label)}` +
            (orphan.length ? `, between ${orphan.join(" and ")}, which no mark draws` : ""));
        }
      }

      // (c) what is written on a mark. The class attribute carries two
      // claims checked apart: every token nodeClass composes is there, and
      // anything else is one of the two the emphasis owns.
      for (const m of marks) {
        const d = m.__data__;
        const rect = [...m.children].find((c) => c.tagName === "rect");
        const tspans = tspansOf(m);
        const want = { role: "button", tabindex: "0", "aria-keyshortcuts": "Enter Space" };
        const bad = Object.keys(want).filter((k) => m.getAttribute(k) !== want[k]);
        const drawnClass = (m.getAttribute("class") || "").split(/\s+/).filter(Boolean);
        const rule = app.nodeClass(d).split(/\s+/).filter(Boolean);
        const missingClass = rule.filter((c) => !drawnClass.includes(c));
        const extraClass = drawnClass.filter((c) => !rule.includes(c) && c !== "dim" && c !== "hot");
        if (missingClass.length) bad.push(`class is missing ${missingClass.join(" ")}`);
        if (extraClass.length) bad.push(`class carries ${extraClass.join(" ")}, which is neither nodeClass's nor the emphasis's`);
        seen.attributes += rule.length;
        if (m.getAttribute("aria-pressed") === null) bad.push("aria-pressed");
        if (!(m.getAttribute("aria-label") || "")) bad.push("aria-label");
        seen.attributes += Object.keys(want).length + 2;
        const w = rect ? Number(rect.getAttribute("width")) : 0;
        const h = rect ? Number(rect.getAttribute("height")) : 0;
        if (!(w > 0) || !(h > 0)) bad.push(`rect ${w}x${h}`);
        // THE WORDS ARE READ OFF THE TSPANS and not off the functions that
        // compose them: comparing d.label against d.label would pass on a
        // label render() never wrote.
        if (!tspans.includes(d.label)) bad.push(`no tspan reading ${JSON.stringify(d.label)}`);
        const amount = "  " + app.fmtShortSigned(app.markCents(d));
        if (!tspans.includes(amount)) bad.push(`no tspan reading ${JSON.stringify(amount)}`);
        seen.attributes += 3;
        if (bad.length) {
          const attrs = [...m.attributes].map((a) => `${a.name}=${JSON.stringify(a.value)}`).join(" ");
          wrong.written.push(`${where}: ${d.id} is drawn class=${JSON.stringify(m.getAttribute("class"))} ` +
            `with ${attrs} and tspans ${JSON.stringify(tspans)}; wrong or missing: ${bad.join(", ")}`);
        }
      }

      // (d) what a gesture DOES, by dispatch: a click isolates and a second
      // releases; Space isolates and the click it synthesises does not undo
      // it; a held key does nothing; a double click and Enter each open.
      const opener = marks.find((m) => answers.has(keyOf(path.concat([m.__data__.id]))));
      const subject = opener || marks[0];
      if (subject) {
        const id = subject.__data__.id;
        const note = (what) => wrong.gestures.push(`${where}: ${id} ${what}`);
        seen.gestures++; fire(subject, "click", { timeStamp: 1000 });
        if (app.isolated !== id) note(`does not isolate on a click (isolated is ${JSON.stringify(app.isolated)})`);
        // The dimming is what the isolation does for a reader who can see the
        // page; stated as invariants rather than by recomputing the predicate.
        const lit = ribbonsIn(chart).filter((p) => p.__data__.source.id === id || p.__data__.target.id === id);
        const dimmedRibbons = ribbonsIn(chart).filter((p) => p.classList.contains("dim"));
        const dimmedMarks = marksIn(chart).filter((m) => m.classList.contains("dim"));
        seen.emphasis += dimmedRibbons.length + dimmedMarks.length;
        if (dimmedRibbons.length === 0) note("isolates and dims no ribbon at all");
        if (dimmedMarks.length === 0) note("isolates and dims no other mark at all");
        if (lit.some((p) => p.classList.contains("dim"))) {
          note(`dims ${lit.filter((p) => p.classList.contains("dim")).length} of its own ${lit.length} ribbon(s)`);
        }
        if (subject.classList.contains("dim")) note("dims itself while isolated");

        seen.gestures++; fire(subject, "click", { timeStamp: 2000 });
        if (app.isolated !== "") note(`does not release on a second click (isolated is ${JSON.stringify(app.isolated)})`);
        const stuck = ribbonsIn(chart).filter((p) => p.classList.contains("dim")).length +
          marksIn(chart).filter((m) => m.classList.contains("dim")).length;
        if (stuck > 0) note(`leaves ${stuck} mark(s) and ribbon(s) dimmed after the isolation is released`);

        seen.gestures++; fire(subject, "keydown", { key: " ", timeStamp: 3000 });
        if (app.isolated !== id) note(`does not isolate on Space (isolated is ${JSON.stringify(app.isolated)})`);
        seen.gestures++; fire(subject, "click", { timeStamp: 3000 });
        if (app.isolated !== id) note("is un-isolated by the click its own Space synthesised");
        seen.gestures++; fire(subject, "keydown", { key: " ", timeStamp: 9000 });
        if (app.isolated !== "") note(`does not release on a second Space (isolated is ${JSON.stringify(app.isolated)})`);

        const before = app.drilled.length;
        seen.gestures++; fire(subject, "keydown", { key: "Enter", timeStamp: 9500, repeat: true });
        await settle();
        if (app.isolated !== "" || app.drilled.length !== before) {
          note(`acts on a held key (isolated ${JSON.stringify(app.isolated)}, ${app.drilled.length} rung(s))`);
        }

        if (opener) {
          // FOCUS ON A MARK IS THE STATE A READER IS IN. A drill replaces the
          // chart, so the element focus was on is gone and restoreFocus has
          // to put it somewhere real.
          subject.focus();
          seen.gestures++; fire(subject, "dblclick", { timeStamp: 10000 });
          await settle();
          const landed = document.activeElement;
          seen.focus++;
          if (!landed || landed === subject || landed === document.body) {
            note(`left focus ${landed && landed !== document.body ? "on the mark the drill replaced" : "nowhere"} after opening`);
          } else if (!landed.classList.contains("crumb-back") && !landed.classList.contains("node")) {
            note(`put focus on ${JSON.stringify(landed.getAttribute("class"))} after opening, ` +
              "which is neither the rung's own return control nor a mark of the chart it drew");
          }
          if (app.drilled.length !== before + 1) {
            note(`does not open on a double click (${app.drilled.length} rung(s), was ${before})`);
          }
          await goTo(app, path);
          const again = marksIn(chart).find((m) => m.__data__.id === id);
          if (!again) note("is gone from the chart it was just opened from");
          else {
            seen.gestures++; fire(again, "keydown", { key: "Enter", timeStamp: 11000 });
            await settle();
            if (app.drilled.length !== before + 1) {
              note(`does not open on Enter (${app.drilled.length} rung(s), was ${before})`);
            }
          }
        }
      }

      // (f) what a folded tail stands for: Enter on the tail the way a reader
      // asks, and then the DOM again. The label is part of the answer.
      if (answer) {
        await goTo(app, path);
        const want = answeredIDs(answer);
        let refused = [];
        const LIMIT = 32;
        let done = 0;
        for (; done < LIMIT; done++) {
          const before = marksIn(chart);
          const shown = before.filter((m) => app.isAggregate(m.__data__.id));
          const tail = shown.find((m) => app.expandable(m.__data__));
          if (!tail) { refused = shown; break; }
          const tier = tail.__data__.tier;
          const drawnBefore = before.map((m) => m.__data__.id);
          const hidden = [...want.keys()].filter((id) => want.get(id) === tier && !drawnBefore.includes(id));
          const promised = tailPromise(tail);
          seen.expansions++;
          const note = (what) => wrong.tails.push(`${where}: the tail at column ${tier} ${what}`);
          fire(tail, "keydown", { key: "Enter", timeStamp: 20000 + done });
          await settle();
          const out = marksIn(chart)
            .filter((m) => m.__data__.tier === tier && !app.isAggregate(m.__data__.id))
            .map((m) => m.__data__.id);
          const revealed = out.filter((id) => !drawnBefore.includes(id));
          seen.revealed += revealed.length;
          const stillHidden = hidden.filter((id) => !out.includes(id));
          const strangers = out.filter((id) => !want.has(id));
          if (stillHidden.length) {
            note(`stood for ${hidden.length} id(s) Go accounts for and drew no mark for ` +
              `${stillHidden.length} of them: ${stillHidden.join(", ")}`);
          }
          if (strangers.length) note(`drew ${strangers.join(", ")}, which Go accounts for nowhere at this rung`);
          if (promised !== revealed.length) {
            note(`says "${promised}" in its own words and drew ${revealed.length} mark(s) the chart did not already carry`);
          }
        }
        if (done === LIMIT) {
          wrong.tails.push(`${where}: the chart still offered a tail to expand after ${LIMIT} expansions, ` +
            "so what it was left holding was never read");
        }
        // SHOWN AND COULD NOT IS NOT THE SAME AS NOT SHOWN: a tail the reader
        // can see whose members no gesture on this chart draws out.
        const drawnNow = marksIn(chart).map((m) => m.__data__.id);
        for (const m of refused) {
          seen.refused++;
          const tier = m.__data__.tier;
          const behind = [...want.keys()].filter((id) => want.get(id) === tier && !drawnNow.includes(id));
          wrong.tails.push(`${where}: the tail at column ${tier} (${m.__data__.id}, ` +
            `${JSON.stringify(m.__data__.label)}) is drawn and the page offers no gesture ` +
            `that draws it out, so what it stands for is unaskable here -- ` +
            `${behind.length} id(s) Go accounts for at that column reach no mark: ${behind.join(", ") || "(none)"}`);
        }
      }
    }
    try { app.drillUp(0); } catch { /* a repaint that throws is reported by the state that hit it */ }
  }
  return { wrong, seen };
}

for (const year of YEARS) {
  describe(`${year.label}: the drawing`, () => {
    let wrong;
    let seen;
    before(async () => { ({ wrong, seen } = await drive(year)); });

    // Each arm carries the counter that could vanish under it alone.
    const drove = () => seen.states > 0 && seen.marks > 0;

    test(`${year.label}: every node the chart lays out reaches a mark, and every mark is one Go accounts for`, (t) => {
      const detail = `${seen.marks} mark(s) over ${seen.states} state(s), ${seen.answered} of them a rung ` +
        `testdata/rungs.json answers -- every id it accounts for drawn, behind one of ` +
        `${seen.tails} folded tail(s), or at one of ${seen.offscreen} places in a column this ` +
        `budget drops -- and ${seen.ownMarks} mark(s) the client adds of its own`;
      t.diagnostic(detail);
      assert.equal(wrong.reach.length, 0, `${wrong.reach.length} disagreement(s), ${firstOf(wrong.reach)}`);
      assert.ok(drove() && seen.answered > 0 && seen.tails > 0 && seen.ownMarks > 0, detail);
    });

    test(`${year.label}: every link the chart lays out reaches a ribbon with a path, a width and a name`, (t) => {
      const detail = `${seen.ribbons} ribbon(s) over ${seen.states} state(s) carry a d, a stroke-width ` +
        "above zero and an aria-label, and both ends of every one of them are marks on the same chart";
      t.diagnostic(detail);
      assert.equal(wrong.ribbons.length, 0, `${wrong.ribbons.length} disagreement(s), ${firstOf(wrong.ribbons)}`);
      assert.ok(drove() && seen.ribbons > 0, detail);
    });

    test(`${year.label}: what render() wrote on a mark is what the mark's own rules say`, (t) => {
      const detail = `${seen.attributes} attribute(s) and word(s) read off ${seen.marks} drawn mark(s): the ` +
        "class nodeClass composes, role, tabindex, aria-pressed, aria-keyshortcuts, a non-empty " +
        "aria-label, a rect with a positive box, and tspans carrying the label and the short signed amount";
      t.diagnostic(detail);
      assert.equal(wrong.written.length, 0, `${wrong.written.length} mark(s) drawn as something else, ${firstOf(wrong.written)}`);
      assert.ok(drove() && seen.attributes > 0, detail);
    });

    test(`${year.label}: a gesture on a drawn mark does what the page says it does`, (t) => {
      const detail = `${seen.gestures} event(s) fired over ${seen.states} state(s): a click isolates and ` +
        "a second releases, Space isolates, the click that Space synthesises does not undo it, " +
        "a held key does nothing, and a double click and Enter each open the node Go answers a " +
        `rung for; the isolation dims ${seen.emphasis} mark(s) and ribbon(s), none of them the ` +
        "isolated node or a flow of its own, and none left dimmed once released; and from focus " +
        `on a mark a drill moved it to the rung's own return control ${seen.focus} time(s)`;
      t.diagnostic(detail);
      assert.equal(wrong.gestures.length, 0, `${wrong.gestures.length} gesture(s) that did something else, ${firstOf(wrong.gestures)}`);
      assert.ok(drove() && seen.gestures > 0 && seen.emphasis > 0 && seen.focus > 0, detail);
    });

    test(`${year.label}: every state drew a chart rather than a refusal`, (t) => {
      const detail = `${seen.states} state(s): no .refusal anywhere, #chart holds a g.links and a g.nodes, ` +
        `and ${seen.marks} mark(s) are in them`;
      t.diagnostic(detail);
      assert.equal(wrong.banner.length, 0, `${wrong.banner.length} state(s) that did not draw, ${firstOf(wrong.banner)}`);
      assert.ok(drove(), detail);
    });

    test(`${year.label}: a folded tail expands into the ids Go accounts for and no others, and no tail is drawn that the page will not draw out`, (t) => {
      const detail = `${seen.expansions} tail(s) expanded by Enter on the mark itself, drawing ` +
        `${seen.revealed} mark(s) that were behind one, each tail drawing as many as the words in ` +
        `it promised; ${seen.refused} tail(s) were drawn that the page offered no way to draw out`;
      t.diagnostic(detail);
      assert.equal(wrong.tails.length, 0, `${wrong.tails.length} tail(s) standing for something else or left folded, ${firstOf(wrong.tails)}`);
      assert.ok(drove() && seen.expansions > 0 && seen.revealed > 0, detail);
    });
  });
}

describe("the drill's drawing", () => {
  /** Every drawn mark of the overview and every offered view, both columns. */
  const records = [];
  const walks = {};
  const hundreds = [];
  const tally = { nodes: 0, opens: 0, derivedOnly: 0, both: 0, expands: 0 };

  before(async () => {
    for (const year of YEARS) {
      const { app, document } = await bootedApp({ checkedStem: year.stem });
      const chart = document.getElementById("chart");
      const look = (where) => {
        for (const m of marksIn(chart)) {
          const d = m.__data__;
          const flag = flagOf(m);
          const opens = app.drillable(d);
          const expands = app.expandable(d);
          tally.nodes++;
          if (opens) tally.opens++;
          if (d.derived && !opens) tally.derivedOnly++;
          if (d.derived && opens) tally.both++;
          if (expands) tally.expands++;
          records.push({
            where: `${year.label} ${where}`, id: d.id,
            classes: (m.getAttribute("class") || "").split(/\s+/).filter(Boolean),
            flag: flag ? flag.textContent : null,
            rule: app.nodeClass(d), flags: app.nodeFlags(d),
            opens, expands, derived: Boolean(d.derived),
          });
          const share = app.columnShare(d);
          if (share.includes("100.0%") || share.includes("100%")) hundreds.push(`${year.label} ${where} ${d.id}: ${share}`);
        }
      };
      look("the overview");
      walks[year.stem] = await everyOffer(app, (path) => look(keyOf(path)));
    }
  });

  test("a node that opens is drawn as one, a node that expands is drawn as one, and a node that does neither is drawn as neither", (t) => {
    const wrong = records.filter((r) =>
      r.classes.includes("opens") !== r.opens ||
      r.classes.includes("expands") !== r.expands ||
      (r.flag || "").includes("⊞") !== r.expands ||
      (r.opens && r.expands) ||
      (r.flag || "").includes("▸") !== r.opens ||
      (r.flag || "").includes("◇") !== r.derived)
      .map((r) => `${r.where} > ${r.id} is drawn "${r.classes.join(" ")}" / "${r.flag}" but ` +
        `${r.opens ? "opens" : "does not open"}`);
    const visited = Object.values(walks).map((w) => w.visited).reduce((a, b) => a + b, 0);
    t.diagnostic(`over the overview and ${visited} opened view(s) of both columns, ${tally.nodes} mark(s): ` +
      `${tally.opens} carry "opens" and the triangle, ${tally.derivedOnly} the diamond alone, ` +
      `${tally.both} both (the diamond-and-triangle pair is latent while this is 0), ` +
      `${tally.expands} carry "expands" and the plus`);
    for (const w of Object.values(walks)) assert.equal(w.refused, "");
    assert.deepEqual(wrong.slice(0, 5), [], `${wrong.length} of ${tally.nodes} mark(s) are drawn as something they are not`);
    assert.ok(tally.opens > 0 && tally.expands > 0 && tally.derivedOnly > 0, "the corpus puts marks on both sides of each question");
    assert.equal(tally.both, 0, "a mark that is an inference and also opens: the composed marker is no longer latent");
  });

  test("the flag tspan a mark draws carries exactly nodeFlags", (t) => {
    const wrong = records.filter((r) => r.flag !== r.flags)
      .map((r) => `${r.where} > ${r.id} draws ${JSON.stringify(r.flag)} where nodeFlags says ${JSON.stringify(r.flags)}`);
    const flagged = records.filter((r) => r.flag).length;
    t.diagnostic(`${records.length} mark(s) read, ${flagged} of them drawing a glyph`);
    assert.deepEqual(wrong.slice(0, 5), [], `${wrong.length} mark(s) draw a flag that is not nodeFlags's`);
    assert.ok(flagged > 0);
  });

  test("no share on any view claims 100% of a column that has more than one mark", async (t) => {
    // AND THE CASE NO PINNED VIEW REACHES: a column that IS divided where
    // toFixed(1) would round to 100. One printed link of the pinned column
    // is scaled up a millionfold, so its source is all but the whole of its
    // column, and the share is read off the mark the page then draws.
    const doc = columnFixture(YEARS[0].fixture);
    const l = doc.schedules.sankey.links.find((x) => doc.nodes[x.from].tier === 0 && x.kind === "external");
    l.value_cents = l.value_cents * 1000000;
    const { app, document } = await bootedApp({ checkedStem: YEARS[0].stem, plan: { [YEARS[0].path]: { doc } } });
    const big = marksIn(document.getElementById("chart")).find((m) => m.__data__.id === doc.nodes[l.from].id);
    const rounded = big ? app.columnShare(big.__data__) : "(the mark is not drawn)";
    t.diagnostic(`every share across the overview and ${Object.values(walks).map((w) => w.visited).join(" + ")} ` +
      `opened views is under 100%; ${doc.nodes[l.from].id} scaled to all but the whole of its column reads "${rounded}"`);
    assert.deepEqual(hundreds.slice(0, 3), [], `${hundreds.length} share(s) read 100%`);
    assert.ok(big, "the scaled mark is drawn");
    assert.ok(rounded.includes("of this column") && !rounded.includes("100"), rounded);
  });

  test("every declared step names the noun a rung of its own is qualified with", (t) => {
    t.diagnostic(CONFIG.steps.map((st) => `${st.key}=${JSON.stringify(st.noun)}`).join(", "));
    for (const st of CONFIG.steps) assert.ok(typeof st.noun === "string" && st.noun !== "", `step ${st.key} declares no noun`);
  });

  test("three openable columns are joined as a list, and joinOr is that rule at every length", async (t) => {
    const { app, document } = await bootedApp({ checkedStem: YEARS[0].stem });
    const hint = document.getElementById("chart-hint").textContent;
    t.diagnostic(`hint "${hint}"; joinOr over one, two and three reads "${app.joinOr(["a"])}", ` +
      `"${app.joinOr(["a", "b"])}", "${app.joinOr(["a", "b", "c"])}"`);
    const w = CONFIG.wording;
    assert.deepEqual(app.openableColumns(), [w.column_left, w.column_middle, w.column_right]);
    const where = app.say("in_column", { columns: app.joinOr([w.column_left, w.column_middle, w.column_right]) });
    assert.ok(hint.startsWith(app.say("open_into", { where })), hint);
    assert.equal(app.joinOr([]), "");
    assert.equal(app.joinOr(["a"]), "a");
    assert.equal(app.joinOr(["a", "b"]), "a or b");
    assert.equal(app.joinOr(["a", "b", "c"]), "a, b or c");
  });

  test("a partition ribbon says it is a cross-tab in its class, its two labels and the flow table", async (t) => {
    const step = stepByKey("object-category");
    const { app, document } = await bootedApp({ checkedStem: YEARS[0].stem });
    const chart = document.getElementById("chart");
    const category = marksIn(chart).find((m) => m.__data__.tier === step.from && app.drillable(m.__data__));
    assert.ok(category, `no mark at tier ${step.from} opens`);
    await opened(app, category.__data__.id);
    const ribbons = ribbonsIn(chart).filter((p) => p.classList.contains("partition"));
    const ribbon = ribbons[0];
    const marks = marksIn(chart);
    const noted = marks.filter((m) => (m.getAttribute("aria-label") || "").includes(app.PARTITION_NOTE));
    const misnoted = marks.filter((m) => app.isPartitionNode(m.__data__) !==
      (m.getAttribute("aria-label") || "").includes(app.PARTITION_NOTE)).map((m) => m.__data__.id);
    const rows = [...document.getElementById("flow-table").querySelector("tbody").children];
    const cells = rows.filter((tr) => tr.children[4].textContent === app.PARTITION_NOTE).length;
    t.diagnostic(`opened into ${category.__data__.id}: ${ribbons.length} of ${ribbonsIn(chart).length} ribbon(s) ` +
      `carry "partition"; ${noted.length} of ${marks.length} mark(s) carry the sentence in their label; ` +
      `${cells} of ${rows.length} table row(s) read it in the provenance column`);
    assert.ok(ribbon, "no drawn ribbon carries the partition class");
    assert.ok(ribbon.getAttribute("aria-label").includes(app.PARTITION_NOTE), ribbon.getAttribute("aria-label"));
    assert.ok(!ribbon.classList.contains("derived"));
    assert.ok(noted.length > 0, "no mark's label carries the sentence");
    assert.deepEqual(misnoted, []);
    assert.equal(cells, ribbons.length);
  });

  test("an opened view draws no legend even where a fund-group node survives the filter", async (t) => {
    const { app, document } = await bootedApp({ checkedStem: YEARS[0].stem });
    const legend = document.getElementById("legend");
    const chart = document.getElementById("chart");
    const swatches0 = legend.children.length;
    await opened(app, "fund-group/general");
    const groupDrawn = marksIn(chart).some((m) => m.__data__.id === "fund-group/general");
    const swatches1 = legend.children.length;
    t.diagnostic(`overview ${swatches0} swatch(es), one per fund group of the column; opened into the group, ` +
      `its node is ${groupDrawn ? "drawn" : "NOT drawn"} and the legend holds ${swatches1}`);
    assert.equal(swatches0, columnFixture(YEARS[0].fixture).fund_groups.length);
    assert.ok(groupDrawn, "the group is not drawn on its own window, so this asserts nothing");
    assert.equal(swatches1, 0);
  });

  test("the opened chart says what it is, in this step's own words", async (t) => {
    const step = stepByKey("transfers");
    const year = YEARS[0];
    const { app, document } = await bootedApp({ checkedStem: year.stem });
    const chart = document.getElementById("chart");
    const mark = marksIn(chart).find((m) => (app.stepFor(m.__data__) || {}).key === step.key);
    assert.ok(mark, `no drawn mark opens under the ${step.key} step`);
    const label = mark.__data__.label;
    await opened(app, mark.__data__.id);
    const desc = document.getElementById("chart-desc").textContent.replace(/\s+/g, " ").trim();
    const title = document.getElementById("chart-title").textContent.replace(/\s+/g, " ").trim();
    t.diagnostic(`title "${title}"; desc opens "${desc.slice(0, 60)}..."`);
    assert.equal(title, `${year.chartTitle}, opened into ${label}`);
    assert.ok(desc.startsWith(`Opened into ${label}. ${step.description}`), desc);
  });

  for (const [open, stem, sharesColumn] of [[[], "sankey", true], [["fund-group/general"], "fund-flows", true]]) {
    const name = open.length ? "opened into " + open.join(" > ") : "the overview";
    test(`${name}: a marked node reaches the tooltip and the panel, and links to its own document's caveat`, async (t) => {
      const { app, document } = await bootedApp({ checkedStem: YEARS[0].stem });
      const chart = document.getElementById("chart");
      await opened(app, ...open);
      // A DRAWN MARK AND NOT THE CENTRE: a carried mark's anchor is the chart
      // above's, and the opened node is alone in its column, so its share is
      // suppressed by construction.
      const centre = open.length ? open[open.length - 1] : "";
      const marked = marksIn(chart).find((m) =>
        !m.__data__.carried_from && m.__data__.id !== centre && app.caveatsFor(m.__data__.id).length > 0);
      assert.ok(marked, "no drawn node at this depth carries a caveat, so this asserts nothing");
      const laid = marked.__data__;
      app.showTip({ target: chart, clientX: 0, clientY: 0 }, laid);
      const tip = document.getElementById("tooltip").textContent;
      app.pin(laid);
      const panel = document.getElementById("detail").textContent;
      const caveat = app.caveatsFor(laid.id)[0].id;
      const href = app.caveatHref(caveat);
      const wantHref = `caveats.html#caveat-${stem}--${caveat}`;
      t.diagnostic(`${laid.id}: tooltip mentions ${tip.includes("caveat") ? "a caveat" : "NO caveat"} and ` +
        `${tip.includes("of this column") ? (tip.includes("◇ our ") ? "a share marked as ours" : "an UNMARKED share") : "no share"}; ` +
        `panel ${panel.includes("Read it in full") ? "links to the full text" : "does NOT link"}; href "${href}"`);
      assert.ok(tip.includes("caveat"), tip);
      if (sharesColumn) assert.ok(tip.includes("◇ our ") && tip.includes("of this column"), tip);
      else assert.ok(!tip.includes("of this column"), tip);
      assert.ok(panel.includes("Read it in full"), panel);
      assert.equal(href, wantHref);
    });
  }

  for (const year of YEARS) {
    const pages = (hs) => [...new Set(hs
      .filter((h) => !h.startsWith("caveats"))
      .map((h) => (h.match(/p(?:age=)?0*(\d+)/) || [])[1])
      .filter(Boolean))].map(Number).sort((a, b) => a - b);

    test(`${year.label}: a carried mark's panel links to the spine's copy of the caveat, and the drawn mark beside it still links to the step document's`, async (t) => {
      const { app, document } = await bootedApp({ checkedStem: year.stem });
      const chart = document.getElementById("chart");
      await opened(app, "fund-group/general");
      const detail = document.getElementById("detail");
      const carried = marksIn(chart).filter((m) => m.__data__.carried_from);
      const withCaveat = carried.filter((m) => app.caveatsFor(m.__data__.id).length > 0);
      const ids = withCaveat.map((m) => m.__data__.id).sort();
      const hrefsIn = () => [...detail.querySelectorAll("a")].map((a) => a.getAttribute("href") || "");
      // THE PANEL, NOT caveatHref: driving pin() and reading the rendered
      // panel is what a reader gets, and the only route red on the caller.
      const panelFor = (id) => {
        const m = marksIn(chart).find((x) => x.__data__.id === id);
        assert.ok(m, `${id} is not drawn`);
        app.pin(m.__data__);
        return detail.textContent;
      };
      const first = withCaveat[0];
      const caveat = first ? app.caveatsFor(first.__data__.id)[0].id : "";
      const wantHref = (year.config.caveats.find((c) => c.id === caveat) || {}).href || "(the year declares no anchor)";
      let carriedPanel = "";
      let carriedHrefs = [];
      if (first) {
        carriedPanel = panelFor(first.__data__.id);
        carriedHrefs = hrefsIn();
      }
      const drawnCaveat = app.caveatsFor("fund/100")[0];
      const stepEntry = year.config.steps[CONFIG.steps.findIndex((s) => s.key === "fund-group")];
      const wantDrawn = (drawnCaveat && (stepEntry.caveats || []).find((c) => c.id === drawnCaveat.id) || {}).href ||
        "(the step declares no anchor)";
      const drawnPanel = panelFor("fund/100");
      const drawnHrefs = hrefsIn();
      const carriedPages = first ? (panelFor(first.__data__.id), pages(hrefsIn())) : [];
      panelFor("fund/100");
      const drawnPages = pages(hrefsIn());
      t.diagnostic(`${carried.length} carried mark(s), ${ids.length} with a caveat (${ids.join(", ")}); ` +
        `the carried panel links to ${JSON.stringify(carriedHrefs.find((h) => h.startsWith("caveats.html")) || "")} ` +
        `and cites pp.${carriedPages.join(",")}; fund/100's links to ` +
        `${JSON.stringify(drawnHrefs.find((h) => h.startsWith("caveats.html")) || "")} and cites ` +
        `${drawnPages.length} page(s) from p.${drawnPages[0]}`);
      assert.ok(carried.length > 0);
      assert.deepEqual(ids, ["transfers/in"]);
      assert.equal(caveat, "transfer-legs-unpaired");
      assert.ok(carriedPanel.includes("Read it in full"), carriedPanel);
      assert.ok(carriedHrefs.includes(wantHref), `want ${wantHref} in ${JSON.stringify(carriedHrefs)}`);
      assert.ok(drawnPanel.includes("Read it in full"), drawnPanel);
      assert.ok(drawnHrefs.includes(wantDrawn), `want ${wantDrawn} in ${JSON.stringify(drawnHrefs)}`);
      assert.notEqual(wantHref, wantDrawn, "the two anchors name one document, so this tells nothing apart");
      assert.deepEqual(carriedPages, [66, 67]);
      assert.ok(drawnPages.includes(127) && !drawnPages.includes(66), JSON.stringify(drawnPages));
    });

    test(`${year.label}: the residual is marked as ours, says why in the check's words, and reaches the inferred list, the tooltip and the panel; nothing carried opens`, async (t) => {
      const { app, document } = await bootedApp({ checkedStem: year.stem });
      const chart = document.getElementById("chart");
      const general = "fund-group/general";
      await opened(app, general);
      const rid = app.residualID(general);
      const mark = marksIn(chart).find((m) => m.__data__.id === rid);
      assert.ok(mark, "no residual mark is drawn on the General Fund");
      const node = mark.__data__;
      const links = app.projection.links.filter((l) => l.source === rid || l.target === rid);
      const carriedEnds = links.map((l) => (l.target === rid ? l.source : l.target));
      const citedPages = [...new Set(links.flatMap((l) => l.locators.flatMap((s) => s.pages)))];
      const declared = stepByKey("fund-group").residual;
      const missingReasons = carriedEnds.filter((e) => !node.rationale.includes(declared[e]));
      const listed = document.getElementById("derived-list").textContent;
      app.showTip({ target: chart, clientX: 0, clientY: 0 }, node);
      const tip = document.getElementById("tooltip").textContent;
      app.pin(node);
      const panel = document.getElementById("detail").textContent;
      const opens = app.projection.nodes.filter((n) => app.isCarried(n.id) && app.drillable(n)).map((n) => n.id);
      // THE WORDS ARE GO'S: the label and the rationale are read off the
      // served answer, not spelled here.
      const answered = (rungsOf(year.stem).find((r) => keyOf(r.path) === general).marks || [])
        .find((m) => m.role === "residual");
      assert.ok(answered, "Go answers no residual mark for the General Fund");
      const carriedPrinted = node.targetLinks.filter((l) => !l.derived);
      const wrongProvenance = carriedPrinted.filter((l) => {
        const said = app.linkDescription(l);
        return !said.includes("re-pointed onto a mark of ours") || said.includes(", printed by the city");
      }).map((l) => l.source.id);
      const rows = [...document.getElementById("flow-table").querySelector("tbody").children];
      const carriedRow = rows.find((tr) => tr.children[1].textContent === node.label && tr.children[0].textContent === "Transfers In");
      const carriedCell = carriedRow ? carriedRow.children[4].textContent : "";
      const derivedLinks = app.projection.links.filter((l) => l.derived);
      const wantFlowLines = new Set(derivedLinks.map((l) => app.homeOf(l)).filter((id) => id !== "")).size;
      const listedFlows = (listed.match(/\d+ inferred flows? totalling/g) || []).length;
      const labelOf = (id) => (app.projection.nodes.find((x) => x.id === id) || { label: id }).label;
      const namedOnce = derivedLinks.every((l) => listed.split(labelOf(l.source) + " → " + labelOf(l.target)).length - 1 === 1);
      const saysCarried = /\d+ flows? of the chart above/.test(listed);
      t.diagnostic(`"${node.label}" derived=${node.derived}, drawn class "${mark.getAttribute("class")}", carries ` +
        `${carriedEnds.length} declared reason(s), cites pp.${citedPages.join(",")}; inferred list has ` +
        `${listedFlows} "N inferred flows" line(s) (want ${wantFlowLines}); ${carriedPrinted.length} printed ` +
        `flow(s) re-pointed onto it, the table reading "${carriedCell}"`);
      assert.equal(node.derived, true);
      assert.ok(mark.classList.contains("derived"), mark.getAttribute("class"));
      assert.equal(node.label, answered.label);
      assert.equal(node.rationale, answered.rationale);
      assert.notEqual(node.rationale, "");
      assert.deepEqual(missingReasons, []);
      assert.ok(node.source_note.includes("Carried, not computed"), node.source_note);
      assert.ok(citedPages.length > 0 && citedPages.every((pg) => node.source_note.includes(String(pg))), node.source_note);
      assert.ok(listed.includes(answered.label) && listed.includes(node.rationale), listed);
      assert.equal(listedFlows, wantFlowLines);
      assert.ok(namedOnce && saysCarried, listed);
      assert.ok(carriedPrinted.length > 0);
      assert.deepEqual(wrongProvenance, []);
      assert.equal(carriedCell, app.CARRIED_CHIP);
      assert.ok(tip.includes("◇ inferred") && tip.includes(node.rationale), tip);
      assert.ok(panel.includes("◇ our inference") && panel.includes(node.rationale) && panel.includes(node.source_note), panel);
      assert.deepEqual(opens, []);
    });

    test(`${year.label}: the capped tail is marked as ours, not as something the city printed`, async (t) => {
      const { app, document } = await bootedApp({ checkedStem: year.stem });
      const chart = document.getElementById("chart");
      const worst = worstOf(rungsOf(year.stem));
      await opened(app, worst);
      const tail = marksIn(chart).find((m) => app.isAggregate(m.__data__.id));
      assert.ok(tail, `no aggregate mark: the cap folded nothing on ${worst}`);
      const agg = tail.__data__;
      const word = app.tailNoun(agg.tier);
      const count = Number((agg.label.match(/^(\d+) smaller /) || [])[1]);
      t.diagnostic(`${worst} draws "${agg.label}" as "${tail.getAttribute("class")}", derived=${agg.derived}, ` +
        `rationale "${agg.rationale.slice(0, 48)}...", source note ${agg.source_note ? "present" : "MISSING"}`);
      assert.equal(agg.derived, true);
      assert.ok(tail.classList.contains("derived"), tail.getAttribute("class"));
      assert.notEqual(agg.rationale, "");
      assert.notEqual(agg.source_note, "");
      assert.equal(agg.label, `${count} smaller ${word}`);
      // AT TWO OR MORE: a column of cap + 1 once folded ONE printed fund
      // into a mark labelled "1 smaller funds".
      assert.ok(count >= 2, agg.label);
      // AND ITS NOTE SAYS WHAT IT CARRIES, at the figure the chart draws it
      // at: the tail is not an inferred flow and gets no line in the inferred
      // list, so this note is where its figure is written (fisc-hrfd).
      const value = app.layOut(app.projection).nodes.find((n) => n.id === agg.id).value;
      assert.ok(value > 0);
      assert.ok(agg.source_note.includes("together " + app.fmt(value)), `${agg.source_note} does not carry ${app.fmt(value)}`);
    });

    test(`${year.label}: the breadcrumb says how many marks the expansion drew, and folds them back`, async (t) => {
      const { app, document } = await bootedApp({ checkedStem: year.stem });
      const chart = document.getElementById("chart");
      await opened(app, worstOf(rungsOf(year.stem)));
      const capped = ribbonsIn(chart).length;
      const tail = marksIn(chart).find((m) => app.expandable(m.__data__));
      assert.ok(tail, "no drawn mark offers to expand");
      const tier = tail.__data__.tier;
      const word = app.tailNoun(tier);
      fire(tail, "keydown", { key: "Enter", timeStamp: 1000 });
      await settle();
      const drawn = marksIn(chart).filter((m) => m.__data__.tier === tier && !app.isAggregate(m.__data__.id)).length;
      const chipCrumbs = crumbs(document);
      const chip = document.getElementById("breadcrumb").querySelector(".crumb-expanded");
      assert.ok(chip, `no chip in ${JSON.stringify(chipCrumbs)}`);
      const chipText = chip.textContent;
      fire(chip, "click");
      await settle();
      const folded = ribbonsIn(chart).length;
      t.diagnostic(`expanded, the column draws ${drawn} ${word} and the breadcrumb reads ${JSON.stringify(chipCrumbs)}; ` +
        `pressing the chip left ${folded} ribbon(s) (capped: ${capped}) under ${JSON.stringify(crumbs(document))}`);
      assert.equal(chipText, `showing all ${drawn} ${word} ×`);
      assert.equal(chipCrumbs.length, 3);
      assert.ok(chipCrumbs[2].startsWith("crumb-expanded"), chipCrumbs[2]);
      assert.equal(folded, capped);
      assert.equal(crumbs(document).length, 2);
    });

    test(`${year.label}: a caveat about a folded row marks the tail while it stands for it, and the row itself once it is drawn`, async (t) => {
      const worst = worstOf(rungsOf(year.stem));
      // Which row the cap folds is the client's answer, so it is read off a
      // plain page first and the caveat is then planted on a copy of the
      // pinned column naming that row.
      const plain = await bootedApp({ checkedStem: year.stem });
      await opened(plain.app, worst);
      const foldedTail = plain.app.projection.nodes.find((n) => plain.app.isAggregate(n.id));
      assert.ok(foldedTail && foldedTail.folds && foldedTail.folds.length, "the cap folded nothing");
      const foldedID = foldedTail.folds[0];
      const CAVEAT = { id: "a-folded-row", summary: "A caveat about a row the cap folds.", text: "", applies_to: [foldedID] };
      const doc = columnFixture(year.fixture);
      doc.schedules[stepByKey("fund-group").projection].caveats.push(CAVEAT);
      const { app, document } = await bootedApp({ checkedStem: year.stem, plan: { [year.path]: { doc } } });
      const chart = document.getElementById("chart");
      await opened(app, worst);
      const marksCarrying = () => marksIn(chart).map((m) => m.__data__.id)
        .filter((id) => app.caveatsFor(id).some((c) => c.id === CAVEAT.id)).sort();
      const cappedMarks = marksCarrying();
      const cappedTail = marksIn(chart).find((m) => app.isAggregate(m.__data__.id));
      assert.ok(cappedTail, "no tail is drawn on the planted page");
      app.expandTier(cappedTail.__data__);
      await settle();
      const expandedMarks = marksCarrying();
      t.diagnostic(`a caveat naming ${foldedID}, which the cap folds, is carried by ${JSON.stringify(cappedMarks)} ` +
        `on the capped chart and by ${JSON.stringify(expandedMarks)} on the expanded one`);
      assert.deepEqual(cappedMarks, [app.aggregateID(cappedTail.__data__.tier), worst].sort());
      assert.deepEqual(expandedMarks, [worst, foldedID].sort());
    });

    test(`${year.label}: no two rungs of one trail draw the same words, over the labels the site ships`, async (t) => {
      const { app, document } = await bootedApp({ checkedStem: year.stem });
      await opened(app, "fund-group/general", "fund/100");
      const words = [app.labelOfRung(0), app.labelOfRung(1)];
      const rungWords = app.trailOfRungs();
      const groupNoun = stepByKey("fund-group").noun;
      const fundNoun = stepByKey("fund").noun;
      const title = document.getElementById("chart-title").textContent.replace(/\s+/g, " ").trim();
      const desc = document.getElementById("chart-desc").textContent.replace(/\s+/g, " ").trim();
      t.diagnostic(`the column labels the two rungs ${JSON.stringify(words)}, and the trail reads ` +
        `${JSON.stringify(rungWords)}; title "${title}"`);
      assert.equal(words[0], words[1], "the two labels no longer collide, so this asserts nothing");
      assert.deepEqual(rungWords, [`${words[0]} (${groupNoun})`, `${words[1]} (${fundNoun})`]);
      assert.equal(new Set(rungWords).size, rungWords.length);
      assert.ok(title.endsWith(`, opened into ${rungWords[0]}, then ${rungWords[1]}`), title);
      assert.ok(desc.startsWith(`Opened into ${rungWords[0]}, then ${rungWords[1]}. `), desc);
    });
  }
});

// Arms the committed columns never reach, driven over a clone of a pinned
// column with one stated change, or a bare call where the arm is a pure
// function. fisc-7477 measured them undriven.
describe("arms no committed column reaches", () => {
  const newest = YEARS[YEARS.length - 1];
  test("a derived flow between two published nodes is listed as inferred on its own, with both ends named", async (t) => {
    const col = structuredClone(columnFixture(newest.fixture));
    const link = col.schedules.sankey.links.find((l) =>
      !l.derived && !col.nodes[l.from].derived && !col.nodes[l.to].derived);
    assert.ok(link, "the pinned column has no printed flow between two published nodes");
    link.derived = true;
    const from = col.nodes[link.from].label;
    const to = col.nodes[link.to].label;
    const { document } = await bootedApp({ checkedStem: newest.stem, plan: { [newest.path]: { doc: col } } });
    assert.equal(refusals(document).length, 0);
    const items = [...document.querySelectorAll("#derived-list li")];
    const orphan = items.find((li) => li.textContent.includes("both endpoints are printed by the city"));
    t.diagnostic(`${items.length} inferred entries; the orphan reads "${orphan ? orphan.textContent : "(none)"}"`);
    assert.ok(orphan, "the inferred list does not mention the flow");
    assert.ok(orphan.textContent.includes(from + " → " + to), orphan.textContent);
  });
  test("a schedule carrying no caveats gives every mark none, and the page still draws", async (t) => {
    const col = structuredClone(columnFixture(newest.fixture));
    delete col.schedules.sankey.caveats;
    const { app, document } = await bootedApp({ checkedStem: newest.stem, plan: { [newest.path]: { doc: col } } });
    assert.equal(refusals(document).length, 0);
    const marks = document.querySelectorAll("#chart g.node").length;
    const ids = app.projection.nodes.map((n) => n.id);
    t.diagnostic(`${marks} marks drawn, ${ids.length} ids asked, all with no caveat`);
    assert.ok(marks > 0);
    assert.ok(ids.every((id) => app.caveatsFor(id).length === 0));
    // The guard on a source with no caveats list at all, which scheduleOf's
    // default keeps off the served path.
    app.projection.metadata.caveats = undefined;
    assert.deepEqual(app.caveatsFor(ids[0]), []);
  });
  test("a label anchored at an end shifts its qualifier less than one anchored in the middle", async () => {
    const { app } = await bootedApp({ checkedStem: newest.stem });
    const middle = app.labelLineShift("middle");
    const end = app.labelLineShift("end");
    assert.deepEqual(app.labelLineShift("start"), end);
    assert.notEqual(end.qualifier, middle.qualifier);
    assert.equal(end.label, middle.label);
  });
});

// A class the client sets that no rule styles is a silent no-op, and a rule
// no page wears is dead ink; neither shows on any other check (fisc-a0fv).
// Rules come from the injected stylesheet as jsdom parsed it, worn classes
// off every element the page holds across the states the client can reach,
// and the classes the other pages' templates set are theirs, not the client's.
describe("the classes the client sets and the rules the stylesheet carries", () => {
  /** Set by the client, styled by nothing, on purpose: hooks the tests and the gestures key on. */
  const HOOKS = {
    links: "the <g> the ribbons are drawn into",
    nodes: "the <g> the marks are drawn into",
    opens: "the drillable mark; its affordance is the flag tspan, not a style",
    expands: "the folded tail, likewise",
  };
  /** Styled, set by nothing anywhere: dead ink, declared with its bead until it goes. */
  const DEAD = {};

  const classesIn = (attr) => (attr || "").split(/\s+/).filter(Boolean);
  const templateClasses = () => {
    const out = new Set();
    for (const f of readdirSync(join(repoRoot, "site")).filter((f) => f.endsWith(".html.tmpl"))) {
      const text = readFileSync(join(repoRoot, "site", f), "utf8");
      // A template writes its class attributes around {{pipes}}; the words
      // either side are the classes and the pipe is not.
      for (const m of text.matchAll(/class="([^"]*)"/g)) {
        for (const c of classesIn(m[1].replace(/\{\{[^}]*\}\}/g, " "))) out.add(c);
      }
    }
    return out;
  };
  const ruleClasses = (document) => {
    const out = new Set();
    const walk = (rules) => {
      for (const r of rules) {
        if (r.selectorText) for (const m of r.selectorText.matchAll(/\.([A-Za-z_][\w-]*)/g)) out.add(m[1]);
        if (r.cssRules) walk(r.cssRules);
      }
    };
    for (const sheet of document.styleSheets) walk(sheet.cssRules);
    return out;
  };

  test("every class the client sets is styled or a declared hook, and every rule is worn on some page or declared dead", async (t) => {
    const { app, document } = await bootedApp({ checkedStem: YEARS[0].stem });
    const chart = document.getElementById("chart");
    const worn = new Set();
    const snap = () => {
      for (const el of document.querySelectorAll("*")) for (const c of classesIn(el.getAttribute("class"))) worn.add(c);
    };
    // The overview, the tooltip on a mark and on a ribbon, the panel on a
    // ribbon, a pin, an isolation, every view the drill offers, an expanded
    // tail, and a refusal.
    snap();
    const mark = marksIn(chart)[0];
    const ribbon = ribbonsIn(chart)[0];
    app.showTip({ target: chart, clientX: 0, clientY: 0 }, mark.__data__); snap();
    app.showTip({ target: chart, clientX: 0, clientY: 0 }, ribbon.__data__); snap();
    app.pin(ribbon.__data__); snap();
    app.pin(mark.__data__); snap();
    fire(mark, "keydown", { key: " " }); await settle(); snap();
    let expandedAt = "";
    const walked = await everyOffer(app, async (path) => {
      snap();
      const laidRibbon = ribbonsIn(chart)[0];
      if (laidRibbon) { app.showTip({ target: chart, clientX: 0, clientY: 0 }, laidRibbon.__data__); app.pin(laidRibbon.__data__); snap(); }
      // The first view that folds a column is expanded once, for the chip.
      if (!expandedAt && app.projection.nodes.some((n) => app.expandable(n))) {
        expandAll(app); await settle(); snap();
        expandedAt = path.join(" > ");
      }
    });
    assert.equal(walked.refused, "");
    assert.ok(expandedAt, "no view offered a column to expand");
    app.fail("probe: a refusal, for its class"); snap();

    const rules = ruleClasses(document);
    const templates = templateClasses();
    const unstyled = [...worn].filter((c) => !rules.has(c) && !(c in HOOKS)).sort();
    const unworn = [...rules].filter((c) => !worn.has(c) && !templates.has(c) && !(c in DEAD)).sort();
    t.diagnostic(`${worn.size} classes worn across ${walked.visited} views and the states above (expanded at ${expandedAt}); ${rules.size} classes styled; ` +
      `${[...rules].filter((c) => !worn.has(c) && templates.has(c)).length} styled classes are other pages' templates'; ` +
      `hooks ${Object.keys(HOOKS).join(", ")}; dead ${Object.keys(DEAD).join(", ")}`);
    assert.deepEqual(unstyled, [], "set by the client and styled by no rule");
    assert.deepEqual(unworn, [], "styled and worn by nothing on any page");
    // The declarations cannot go stale in silence: a hook is worn and
    // unstyled, and a dead rule is styled and set nowhere.
    for (const c of Object.keys(HOOKS)) assert.ok(worn.has(c) && !rules.has(c), `${c} is no longer a hook`);
    for (const c of Object.keys(DEAD)) assert.ok(rules.has(c) && !worn.has(c) && !templates.has(c), `${c} is no longer dead`);
  });
});

// The words on the page are the packager's: every sentence the client
// composes is a wording template in the config that say() fills, so a
// reworded config is followed on the page and no copy of the words survives
// in app.js (fisc-xixn, fisc-jdsb).
describe("the wording is the packager's", () => {
  test("say fills a placeholder with the value, and a {name:one|many} with the value and the word its count picks", async () => {
    const config = structuredClone(pageFixture().config);
    config.wording.counts = "{links:flow|flows} over {nodes:node|nodes}; {unfilled} stays";
    const { app } = await bootedApp({ config });
    assert.equal(app.say("counts", { links: 1, nodes: 2 }), "1 flow over 2 nodes; {unfilled} stays");
    assert.equal(app.say("counts", { links: 3, nodes: 1 }), "3 flows over 1 node; {unfilled} stays");
    assert.equal(app.say("go_back"), config.wording.go_back);
  });
  test("a reworded config is followed by the counts line, the hint, the description and the breadcrumb", async (t) => {
    const config = structuredClone(pageFixture().config);
    for (const key of Object.keys(config.wording)) config.wording[key] = `«${key}» ` + config.wording[key];
    const { app, document } = await bootedApp({ config, checkedStem: YEARS[0].stem });
    const text = (id) => document.getElementById(id).textContent;
    const hintWhole = text("chart-hint");
    await opened(app, "fund-group/general");
    const counts = text("counts-line");
    const hint = text("chart-hint");
    const desc = text("chart-desc");
    const crumbs = [...document.querySelectorAll("#breadcrumb button.crumb-back")].map((b) => b.textContent);
    t.diagnostic(`counts "${counts}"; hint "${hint.slice(0, 80)}…"; crumbs ${JSON.stringify(crumbs)}`);
    assert.ok(hintWhole.startsWith("«open_into» ") && hintWhole.includes("«in_column» ") &&
      hintWhole.includes("«column_left»") && hintWhole.includes(" «follow» "), hintWhole);
    assert.ok(counts.startsWith("«counts_carried» ") || counts.startsWith("«counts_partial» "), counts);
    assert.match(counts, /\d+ (flow|flows) between \d+ (node|nodes)/);
    assert.ok(hint.startsWith("«opened_hint» This is "), hint);
    assert.ok(desc.includes(" «go_back» Use the breadcrumb"), desc);
    assert.deepEqual(crumbs, [`«back_control» \u2190 ${config.steps.find((s) => s.key === app.drilled[0].step.key).back}`]);
  });
});
