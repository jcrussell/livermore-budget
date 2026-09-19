// The rendering arm: what render() actually put in the SVG, read back off the
// DOM the client drew it into.
//
// WHAT THIS WITNESSES THAT rungs.mjs CANNOT. That file is an EQUIVALENCE arm:
// it holds the client's shaping -- which nodes a column draws, which it
// carries, what its tail hides -- against Go's committed answer in
// testdata/rungs.json. Both sides of that comparison are computations. Neither
// is a mark on a page. Every class, attribute, child element and event handler
// render() writes was, until this file, written by the page and read by
// nothing: the stub answered no "#chart" selector, so d3 laid render()'s whole
// selection over a null node -- D3.select("#chart").size() was 0, .append("g")
// returned a size-0 selection, every .attr() accessor was invoked zero times
// and .on() registered no handler.
//
// AND WHY THAT MATTERS. Where the client READS Go's answer rather than
// recomputing it (fisc-phtp.2), rungs.mjs compares that artifact against
// itself and is green by construction -- checks that cannot fail, in this
// branch's strongest guarantee, arriving disguised as a passing suite. A
// capped column whose fold engages is already such a place, and rungs.mjs's
// own header says which of its arms that costs. This module is what witnesses
// the client there, because what it reads is the drawing and not the
// derivation: a client that got its ids straight from Go and drew none of
// them, or drew them with no handler on them, is red here and green there.
//
// PLANTING A BARE #chart NODE IS THE TRAP AND ARM (e) IS WHY IT IS NAMED. With
// setAttribute and addEventListener answering but ownerDocument and
// namespaceURI absent, d3's creatorInherit throws at svg.append("g") -- and
// app.js turns that throw into its own refusal banner, so every arm below would
// be green because the gate fired rather than because the page drew. (e) is
// asserted on EVERY state driven, not once.
//
// MUTATION: delete one `.attr("aria-keyshortcuts", "Enter Space")` line from
// render() and (c) goes red naming the mark and the attribute. Make marksIn()
// return [] -- blind the reader -- and all five arms go red on their own
// counters rather than green with nothing compared. Take the "#chart" answer
// out of harness.mjs's document.querySelector and they go red saying the chart
// drew nothing. Remove one id from a column of a rung the fold does not engage
// on -- expenditure/capital-outlay's tier 4 at 3 columns -- and (a) goes red
// naming the mark Go answers for no longer: "the chart draws 9" against 8
// accounted for. That is the mutation that proves this arm is what sees
// fisc-phtp.2 land. Remove one from a column the fold DOES engage on and the
// rung refuses instead, because capColumn holds the answer against its own
// `hidden`, so (e) is the arm that names it.
import { readFileSync } from "node:fs";
import { join } from "node:path";

import { repoRoot, settle } from "./harness.mjs";
import { COLUMNS, openedWide } from "./drill.mjs";

const ARTIFACT = JSON.parse(readFileSync(join(repoRoot, "testdata", "rungs.json"), "utf8"));
if (ARTIFACT.schema_version !== 4) {
  throw new Error(`testdata/rungs.json declares schema_version ${ARTIFACT.schema_version}; this module reads 4`);
}

/** @param {string[]} path */
const keyOf = (path) => (path.length ? path.join(" > ") : "(the overview)");

/**
 * A failure list as a detail line.
 *
 * THE COUNT IS WHOLE AND THE LIST IS CUT. One deleted `.attr()` line is wrong
 * on every mark of every state, and printing all of them buries the other four
 * arms' output; the count is what says how far it reached.
 * @param {string[]} found
 */
const firstOf = (found) =>
  `${found.length} of them:\n      ${found.slice(0, 3).join("\n      ")}` +
  (found.length > 3 ? `\n      ... and ${found.length - 3} more` : "");

/**
 * The marks the client drew, read off the SVG.
 *
 * THE ONE DOM READER IN THIS FILE, and every arm below goes through it. That is
 * deliberate: blinding it must take the whole module red on its counters, which
 * is the property that stops any arm here passing with nothing compared.
 * @param {any} chart the #chart element
 */
function marksIn(chart) {
  return chart.querySelectorAll("g.node");
}

/** The ribbons the client drew, read off the same SVG. */
function ribbonsIn(chart) {
  return chart.querySelectorAll("path.link");
}

/**
 * The ids Go's answer accounts for at one rung: the opened node's own parts,
 * the marks carried beside them, the folded tail where a column has one, and
 * the marks the client adds of its own.
 *
 * THE TAIL'S ID IS THE CLIENT'S SPELLING AND ITS EXISTENCE IS GO'S. Go answers
 * how many a column hides and not what the mark standing for them is called, so
 * `hidden > 0` is read out of the artifact and aggregateID says what that mark's
 * id is. A tail Go says nothing is hidden at is an id nothing here adds, and a
 * mark the client draws for it is then an id (a) names as unanswered.
 *
 * @param {any} rung one entry of testdata/rungs.json's columns[].rungs
 * @param {(tier: number) => string} aggregateID
 */
function answeredIDs(rung, aggregateID) {
  const ids = new Set();
  for (const d of rung.draws) {
    for (const id of d.ids) ids.add(id);
    for (const id of d.carried || []) ids.add(id);
    if ((d.hidden || 0) > 0) ids.add(aggregateID(d.tier));
  }
  for (const m of rung.marks || []) ids.add(m.id);
  return ids;
}

/**
 * The rungs this arm drives in one column at one budget, chosen by what each
 * one exercises rather than by name.
 *
 * CHOSEN AND NOT ENUMERATED, because driving all of them is rungs.mjs's job and
 * costs a render per rung; what this arm needs is every SHAPE a drawing can
 * take. The overview is the state no rung answers for; the first depth-1 rung
 * is the plain one; the first rung with a fold engaged is the only state where
 * a tail is a mark; the first with a residual and the first with a gap are the
 * two marks the client adds of its own; the deepest is the one whose window is
 * three rungs from the overview. Selected off the artifact in its own order, so
 * the set is a function of Go's answer and not of a list here.
 *
 * @param {any[]} rungs the column's rungs at one width
 */
function statesIn(rungs) {
  /** @type {string[][]} */
  const chosen = [[]];
  const deepest = Math.max(...rungs.map((r) => r.path.length));
  const wanted = [
    rungs.find((r) => r.path.length === 1),
    rungs.find((r) => r.draws.some((/** @type {any} */ d) => (d.hidden || 0) > 0)),
    rungs.find((r) => (r.marks || []).some((/** @type {any} */ m) => m.role === "residual")),
    rungs.find((r) => (r.marks || []).some((/** @type {any} */ m) => m.role === "gap")),
    rungs.find((r) => r.path.length === deepest),
  ];
  for (const r of wanted) {
    if (r && !chosen.some((p) => keyOf(p) === keyOf(r.path))) chosen.push(r.path);
  }
  return chosen;
}

/** Puts the app on the chart one path names, from the overview. */
async function goTo(app, path) {
  app.drillUp(0);
  for (const id of path) {
    const outcome = await app.drillDown(id);
    await settle();
    if (outcome !== "drew") throw new Error(`drillDown(${id}) ${outcome}`);
  }
  await settle();
}

/** An event with the two methods render()'s handlers call on it. */
const event = (extra) => ({
  stopPropagation() {},
  preventDefault() {},
  timeStamp: 0,
  repeat: false,
  ...extra,
});

/** Fires every listener the element carries for one type, the way a browser would. */
function dispatch(element, type, extra) {
  const listeners = element.listeners[type] || [];
  if (!listeners.length) throw new Error(`the mark carries no ${type} handler`);
  for (const fn of listeners) fn.call(element, event(extra));
  return listeners.length;
}

/**
 * @returns {Promise<{name: string, ok: boolean, detail: string}[]>}
 */
export async function checks() {
  const out = [];
  for (const col of COLUMNS) {
    const column = ARTIFACT.columns.find((c) => c.stem === col.stem);
    if (!column) throw new Error(`testdata/rungs.json answers for no column with stem ${col.stem}`);

    /** @type {Record<string, string[]>} */
    const wrong = { reach: [], ribbons: [], written: [], gestures: [], banner: [] };
    const seen = {
      states: 0, answered: 0, marks: 0, attributes: 0, ribbons: 0, gestures: 0,
      tails: 0, ownMarks: 0,
    };

    for (const width of [3, 4]) {
      const rungs = column.rungs.filter((r) => r.width === width);
      const answers = new Map(rungs.map((r) => [keyOf(r.path), r]));
      const app = await openedWide(width, [], col);
      const chart = app.dom.document.getElementById("chart");
      // fail() prepends its banner to the <main> the walk's builder plants, so
      // without one a refusal has nowhere to be and (e) would be asserting
      // absence against a place nothing writes to.
      const content = app.dom.document.querySelector("main");
      if (!content) throw new Error("no <main> is planted, so a refusal banner would have nowhere to be");

      for (const path of statesIn(rungs)) {
        const where = `${col.label} at ${width} columns, ${keyOf(path)}`;
        seen.states++;
        // A STATE THAT THROWS IS A STATE THAT DID NOT DRAW, and it is reported
        // as one rather than left to take the module down. render() is where a
        // half-modelled DOM fails: drillDown catches the throws it knows --
        // the fetch, the shape, the layout -- and a repaint that threw goes
        // past it. A module that dies there reports nothing about the other
        // states, and reads as a broken harness rather than a chart that did
        // not draw.
        try {
          await goTo(app, path);
        } catch (e) {
          wrong.banner.push(`${where}: redrawing it threw -- ${e && e.message ? e.message : String(e)}`);
          // The way back to the overview is another repaint and can throw for
          // the same reason; there is nothing left to report if it does.
          try { app.drillUp(0); } catch { /* reported above */ }
          continue;
        }

        const marks = marksIn(chart);
        const ribbons = ribbonsIn(chart);

        // --------------------------------------------------- (e) no banner
        //
        // FIRST, AND ON EVERY STATE. Every arm below reads a DOM that app.js
        // only fills when render() ran to the end; a throw anywhere inside it
        // comes back out as this banner with the chart left as it was, and an
        // arm reading that state is green because the gate fired.
        //
        // ASKED THE WAY clearRefusal ASKS, through the same selector on the
        // same element, so what is read is a banner that appeared rather than
        // one this file guessed the home of.
        const banners = content.querySelectorAll(".refusal");
        const groups = chart.children.map((/** @type {any} */ c) => c.className);
        if (banners.length) {
          wrong.banner.push(`${where}: the page drew ${banners.length} refusal banner(s)`);
        }
        if (!groups.includes("links") || !groups.includes("nodes") || !marks.length) {
          wrong.banner.push(`${where}: #chart holds [${groups.join(", ")}] and ` +
            `${marks.length} mark(s), which is not a chart that drew`);
        }

        // ------------------------------------ (a) every node reaches a mark
        //
        // THREE SETS AND NOT TWO. The DOM against the graph the client laid
        // says every node it decided on got drawn; the graph against Go's
        // answer says the decision was Go's. Dropping either leaves the other
        // comparing one side against itself.
        const drawn = marks.map((/** @type {any} */ m) => m.__data__.id);
        const laid = app.projection.nodes.map((/** @type {any} */ n) => n.id);
        const missing = laid.filter((id) => !drawn.includes(id));
        const extra = drawn.filter((id) => !laid.includes(id));
        // COUNTED AS WELL AS MEMBERSHIP-CHECKED, because a mark attached twice
        // passes every set comparison and is a mark the reader can click that
        // is not the one under the pointer. Measured: with the stub's
        // insertBefore ignoring its reference node, d3's order() attached 25
        // marks 49 times, and both id sets still agreed.
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
          seen.tails += answer.draws.filter((/** @type {any} */ d) => (d.hidden || 0) > 0).length;
          seen.ownMarks += (answer.marks || []).length;
          const want = answeredIDs(answer, app.aggregateID);
          const unanswered = drawn.filter((id) => !want.has(id));
          const undrawn = [...want].filter((id) => !drawn.includes(id));
          if (unanswered.length || undrawn.length) {
            wrong.reach.push(`${where}: Go accounts for ${want.size} id(s) and the chart draws ` +
              `${drawn.length}` +
              (undrawn.length ? `; Go answers and no mark carries: ${undrawn.join(", ")}` : "") +
              (unanswered.length ? `; drawn and Go accounts for none: ${unanswered.join(", ")}` : ""));
          }
        } else if (path.length) {
          wrong.reach.push(`${where}: testdata/rungs.json answers for no such rung`);
        }

        // ----------------------------------- (b) every link reaches a ribbon
        const laidLinks = app.projection.links.length;
        if (ribbons.length !== laidLinks) {
          wrong.ribbons.push(`${where}: ${laidLinks} link(s) laid, ${ribbons.length} ribbon(s) drawn`);
        }
        for (const p of ribbons) {
          seen.ribbons++;
          const d = p.getAttribute("d");
          const stroke = Number(p.getAttribute("stroke-width"));
          const label = p.getAttribute("aria-label") || "";
          // AND ITS TWO ENDS ARE MARKS ON THIS CHART. A ribbon drawn between
          // nodes nothing drew is a flow the reader cannot follow to either
          // end, and it is what makes this arm depend on marksIn too.
          const ends = [p.__data__.source.id, p.__data__.target.id];
          const orphan = ends.filter((id) => !drawn.includes(id));
          if (!d || !(stroke > 0) || !label || orphan.length) {
            wrong.ribbons.push(`${where}: the ribbon ${ends.join(" -> ")} is drawn ` +
              `d=${JSON.stringify(d)} stroke-width=${JSON.stringify(p.getAttribute("stroke-width"))} ` +
              `aria-label=${JSON.stringify(label)}` +
              (orphan.length ? `, between ${orphan.join(" and ")}, which no mark draws` : ""));
          }
        }

        // ------------------------------------- (c) what is written on a mark
        for (const m of marks) {
          const d = m.__data__;
          const rect = m.children.find((/** @type {any} */ c) => c.tagName === "rect");
          const text = m.children.find((/** @type {any} */ c) =>
            c.tagName === "text" && c.className === "halo");
          const tspans = text ? text.children.map((/** @type {any} */ c) => c.textContent) : [];
          const want = {
            class: app.nodeClass(d),
            role: "button",
            tabindex: "0",
            "aria-keyshortcuts": "Enter Space",
          };
          const bad = Object.keys(want).filter((k) => m.getAttribute(k) !== want[k]);
          if (m.getAttribute("aria-pressed") === null) bad.push("aria-pressed");
          if (!(m.getAttribute("aria-label") || "")) bad.push("aria-label");
          seen.attributes += Object.keys(want).length + 2;
          const w = rect ? Number(rect.getAttribute("width")) : 0;
          const h = rect ? Number(rect.getAttribute("height")) : 0;
          if (!(w > 0) || !(h > 0)) bad.push(`rect ${w}x${h}`);
          // THE WORDS ARE READ OFF THE TSPANS AND NOT OFF THE FUNCTIONS THAT
          // COMPOSE THEM. Comparing d.label against d.label would pass on a
          // label render() never wrote.
          if (!tspans.includes(d.label)) bad.push(`no tspan reading ${JSON.stringify(d.label)}`);
          const amount = "  " + app.fmtShortSigned(app.markCents(d));
          if (!tspans.includes(amount)) bad.push(`no tspan reading ${JSON.stringify(amount)}`);
          seen.attributes += 3;
          if (bad.length) {
            wrong.written.push(`${where}: ${d.id} is drawn class=${JSON.stringify(m.getAttribute("class"))} ` +
              `with ${JSON.stringify(m.attributes)} and tspans ${JSON.stringify(tspans)}; ` +
              `wrong or missing: ${bad.join(", ")}`);
          }
        }

        // ----------------------------------------- (d) what a gesture DOES
        //
        // BY DISPATCH AND NOT BY SOURCE TEXT. drill.mjs pins these lines of
        // render() as strings, which passes when the line is right and the
        // selection it sits on is empty; what a handler DOES to the page is
        // only answerable by calling the handler the page registered.
        const opener = marks.find((/** @type {any} */ m) =>
          answers.has(keyOf(path.concat([m.__data__.id]))));
        const subject = opener || marks[0];
        if (subject) {
          const id = subject.__data__.id;
          const note = (what) => wrong.gestures.push(`${where}: ${id} ${what}`);
          seen.gestures += dispatch(subject, "click", { timeStamp: 1000 });
          if (app.isolated !== id) note(`does not isolate on a click (isolated is ${JSON.stringify(app.isolated)})`);
          seen.gestures += dispatch(subject, "click", { timeStamp: 2000 });
          if (app.isolated !== "") note(`does not release on a second click (isolated is ${JSON.stringify(app.isolated)})`);

          seen.gestures += dispatch(subject, "keydown", { key: " ", timeStamp: 3000 });
          if (app.isolated !== id) note(`does not isolate on Space (isolated is ${JSON.stringify(app.isolated)})`);
          // THE TIMESTAMP GUARD: the click some assistive tech synthesises from
          // the key that just fired carries that key's own timestamp, and must
          // not undo it.
          seen.gestures += dispatch(subject, "click", { timeStamp: 3000 });
          if (app.isolated !== id) note(`is un-isolated by the click its own Space synthesised`);
          seen.gestures += dispatch(subject, "keydown", { key: " ", timeStamp: 9000 });
          if (app.isolated !== "") note(`does not release on a second Space (isolated is ${JSON.stringify(app.isolated)})`);

          const before = app.drilled.length;
          seen.gestures += dispatch(subject, "keydown", { key: "Enter", timeStamp: 9500, repeat: true });
          await settle();
          if (app.isolated !== "" || app.drilled.length !== before) {
            note(`acts on a held key (isolated ${JSON.stringify(app.isolated)}, ${app.drilled.length} rung(s))`);
          }

          if (opener) {
            seen.gestures += dispatch(subject, "dblclick", { timeStamp: 10000 });
            await settle();
            if (app.drilled.length !== before + 1) {
              note(`does not open on a double click (${app.drilled.length} rung(s), was ${before})`);
            }
            await goTo(app, path);
            const again = marksIn(chart).find((/** @type {any} */ m) => m.__data__.id === id);
            if (!again) note("is gone from the chart it was just opened from");
            else {
              seen.gestures += dispatch(again, "keydown", { key: "Enter", timeStamp: 11000 });
              await settle();
              if (app.drilled.length !== before + 1) {
                note(`does not open on Enter (${app.drilled.length} rung(s), was ${before})`);
              }
            }
          }
        }
      }
      try { app.drillUp(0); } catch { /* a repaint that throws is reported by the state that hit it */ }
    }

    // EACH ARM CARRIES THE COUNTER THAT COULD VANISH UNDER IT ALONE. `marks` is
    // the reader every arm goes through; `answered` is the comparison against
    // Go that fisc-phtp.2 would otherwise silence; `attributes`, `ribbons` and
    // `gestures` are the three that could each go to nothing while the other
    // two stayed green.
    const drove = seen.states > 0 && seen.marks > 0;
    out.push({
      name: `${col.label}: every node the chart lays out reaches a mark, and every mark is one Go accounts for`,
      ok: wrong.reach.length === 0 && drove && seen.answered > 0 && seen.tails > 0 && seen.ownMarks > 0,
      detail: wrong.reach.length
        ? `${wrong.reach.length} disagreement(s), ${firstOf(wrong.reach)}`
        : `${seen.marks} mark(s) over ${seen.states} state(s), ${seen.answered} of them a rung ` +
          `testdata/rungs.json answers -- accounting for ${seen.tails} folded tail(s) and ` +
          `${seen.ownMarks} mark(s) the client adds of its own, each of which is an id no ` +
          `document names`,
    });
    out.push({
      name: `${col.label}: every link the chart lays out reaches a ribbon with a path, a width and a name`,
      ok: wrong.ribbons.length === 0 && drove && seen.ribbons > 0,
      detail: wrong.ribbons.length
        ? `${wrong.ribbons.length} disagreement(s), ${firstOf(wrong.ribbons)}`
        : `${seen.ribbons} ribbon(s) over ${seen.states} state(s) carry a d, a stroke-width ` +
          `above zero and an aria-label, and both ends of every one of them are marks on the ` +
          `same chart`,
    });
    out.push({
      name: `${col.label}: what render() wrote on a mark is what the mark's own rules say`,
      ok: wrong.written.length === 0 && drove && seen.attributes > 0,
      detail: wrong.written.length
        ? `${wrong.written.length} mark(s) drawn as something else, ${firstOf(wrong.written)}`
        : `${seen.attributes} attribute(s) and word(s) read off ${seen.marks} drawn mark(s): the ` +
          `class nodeClass composes, role, tabindex, aria-pressed, aria-keyshortcuts, a ` +
          `non-empty aria-label, a rect with a positive box, and tspans carrying the label and ` +
          `the short signed amount`,
    });
    out.push({
      name: `${col.label}: a gesture on a drawn mark does what the page says it does`,
      ok: wrong.gestures.length === 0 && drove && seen.gestures > 0,
      detail: wrong.gestures.length
        ? `${wrong.gestures.length} gesture(s) that did something else, ${firstOf(wrong.gestures)}`
        : `${seen.gestures} handler(s) fired over ${seen.states} state(s): a click isolates and ` +
          `a second releases, Space isolates, the click that Space synthesises does not undo it, ` +
          `a held key does nothing, and a double click and Enter each open the node Go answers a ` +
          `rung for`,
    });
    out.push({
      name: `${col.label}: every state drew a chart rather than a refusal`,
      ok: wrong.banner.length === 0 && drove,
      detail: wrong.banner.length
        ? `${wrong.banner.length} state(s) that did not draw, ${firstOf(wrong.banner)}`
        : `${seen.states} state(s): no .refusal anywhere, #chart holds a g.links and a g.nodes, ` +
          `and ${seen.marks} mark(s) are in them -- which is what tells a drawing apart from a ` +
          `throw app.js caught and banners`,
    });
  }
  return out;
}
