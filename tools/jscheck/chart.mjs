// The rendering arm: what render() actually put in the SVG, read back off the
// DOM the client drew it into.
//
// WHAT THIS WITNESSES THAT rungs.mjs CANNOT. That file is an EQUIVALENCE arm:
// it holds the client's shaping -- which nodes a column holds, which it
// carries, how many its fit leaves room for -- against Go's committed answer
// in testdata/rungs.json. Both sides of that comparison are computations.
// Neither is a mark on a page. Every class, attribute, child element and event
// handler render() writes was, until this file, written by the page and read
// by nothing: the stub answered no "#chart" selector, so d3 laid render()'s
// whole selection over a null node -- D3.select("#chart").size() was 0,
// .append("g") returned a size-0 selection, every .attr() accessor was invoked
// zero times and .on() registered no handler.
//
// AND WHY THAT MATTERS. A chart agreeing with Go about its own nodes is not a
// chart: the shaping can be right in every figure rungs.mjs reads and reach no
// reader at all, and it did -- every mark, every ribbon and every handler
// below was unobserved until this module drew them. What it reads is the
// DRAWING and not the derivation, so a client that shaped the column Go
// answers and then drew none of it, drew it twice, or drew it with no handler
// on it, is red here and green there.
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
// return [] -- blind the reader -- and all six arms go red on their own
// counters rather than green with nothing compared. Take the "#chart" answer
// out of harness.mjs's document.querySelector and they go red saying the chart
// drew nothing. Remove one id from a column of a driven rung that the fold
// does NOT engage on -- expenditure/capital-outlay's tier 4 -- and (a) goes
// red naming the mark Go accounts for no longer: "the chart draws 9" against 8
// accounted for. Remove one from behind a TAIL -- which (a) cannot see, because
// a folded column's hidden ids are not on the page for it to read -- and (f)
// goes red naming it: that arm presses Enter on the tail the way a reader does
// and reads the column the page then draws out.
//
// AND (f) COUNTS A TAIL IT WAS SHOWN AND COULD NOT EXPAND rather than walking
// past it. expandable() offers the gesture only where THIS rung's step caps
// the tail's own tier, so an aggregate that arrived inside a kept flank was
// folded by the chart above, is drawn to the reader, and is refused -- and
// every id behind it leaves this arm's reach with nothing said. No such tail is
// drawn on the committed corpus, so the red path is forced from the checker
// side: wrapping the harness's expandable so it answers false at one tier takes
// (f) red naming the rung, the tail and the ids left behind it. fisc-2wsw.
import { readFileSync } from "node:fs";
import { join } from "node:path";

import { repoRoot, settle } from "./harness.mjs";
import { COLUMNS, expandAll, openedWide } from "./drill.mjs";

const ARTIFACT = JSON.parse(readFileSync(join(repoRoot, "testdata", "rungs.json"), "utf8"));
if (ARTIFACT.schema_version !== 5) {
  throw new Error(`testdata/rungs.json declares schema_version ${ARTIFACT.schema_version}; this module reads 5`);
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
 * Whether a drawn element carries a class token.
 *
 * READ OFF THE class ATTRIBUTE, because that is where it lands: applyEmphasis
 * goes through d3's .classed(), which has no classList to use on these nodes
 * and so adds and removes through getAttribute/setAttribute -- the same
 * attribute nodeClass writes once at render.
 * @param {any} element
 * @param {string} token
 */
function carries(element, token) {
  return (element.getAttribute("class") || "").split(/\s+/).includes(token);
}

/**
 * The number a folded tail promises the reader, read off the words drawn in it.
 *
 * OFF THE TSPANS AND NOT OFF THE COUNT capColumn COMPOSED THE LABEL FROM. "21
 * smaller departments" is a claim to a reader about how many marks expanding
 * that mark draws, and the claim is only made where the words reached the page.
 * @param {any} mark a drawn g.node standing for a folded column
 * @returns {number} -1 when nothing drawn in it reads as a count
 */
function tailPromise(mark) {
  const text = mark.children.find((/** @type {any} */ c) =>
    c.tagName === "text" && c.className === "halo");
  const words = text ? text.children.map((/** @type {any} */ c) => c.textContent) : [];
  for (const w of words) {
    const found = /^(\d+) smaller /.exec(w);
    if (found) return Number(found[1]);
  }
  return -1;
}

/**
 * The ids Go's answer accounts for at one rung, each with the column it stands
 * in: the opened node's own parts, the marks carried beside them, and the
 * marks the client adds of its own.
 *
 * THE TAIL IS NOT ONE OF THEM, AND THAT IS THE WHOLE OF WHAT A FOLD COSTS THIS
 * ARM. Go answers what a column HOLDS and the client decides how much fits, so
 * neither the tail's existence nor its id is in this file: what (a) can still
 * say is that an id Go accounts for is either a mark on the chart or behind
 * the tail of ITS OWN column, and that nothing else is drawn at all.
 *
 * A MARK OF THE CLIENT'S OWN IS GIVEN NO COLUMN, deliberately, so that it can
 * never be excused as folded away. Measured over the whole walk in rungs.mjs:
 * the residual and the gap are on the chart at every rung whether its columns
 * are fitted or expanded.
 *
 * @param {any} rung one entry of testdata/rungs.json's columns[].rungs
 * @returns {Map<string, number>} every accounted id, by the tier it stands at,
 *   with -1 for a mark that stands outside the fold
 */
function answeredIDs(rung) {
  /** @type {Map<string, number>} */
  const ids = new Map();
  for (const d of rung.draws) {
    for (const id of d.ids) ids.set(id, d.tier);
    for (const id of d.carried || []) ids.set(id, d.tier);
  }
  for (const m of rung.marks || []) ids.set(m.id, -1);
  return ids;
}

/** How many nodes the widest column of one rung holds, unfolded. */
const widest = (rung) => Math.max(...rung.draws.map((/** @type {any} */ d) =>
  d.ids.length + (d.carried || []).length));

/**
 * The rungs this arm drives in one column, chosen by what each one exercises
 * rather than by name.
 *
 * CHOSEN AND NOT ENUMERATED, because driving all of them is rungs.mjs's job and
 * costs a render per rung; what this arm needs is every SHAPE a drawing can
 * take. The overview is the state no rung answers for; the first depth-1 rung
 * is the plain one; the rung holding the widest column is where a fold is
 * engaged if it is engaged anywhere; the first with a residual and the first
 * with a gap are the two marks the client adds of its own; the deepest is the
 * one whose window is three rungs from the overview. Selected off the artifact
 * in its own order, so the set is a function of Go's answer and not of a list
 * here.
 *
 * THE FOLDED STATE IS CHOSEN BY SIZE AND NOT BY A FOLD, which is what the
 * ruling costs this selection: the artifact says what a column holds and no
 * longer which columns fit, so it cannot be asked where a tail is. The widest
 * column is where one is if there is one, and `seen.tails` is what says a tail
 * was actually drawn there -- a run in which the fold stops engaging takes that
 * counter to zero and (a) red, rather than quietly driving five plain charts.
 *
 * @param {any[]} rungs the column's rungs
 */
function statesIn(rungs) {
  /** @type {string[][]} */
  const chosen = [[]];
  const deepest = Math.max(...rungs.map((r) => r.path.length));
  const wanted = [
    rungs.find((r) => r.path.length === 1),
    // Ties broken by the path, so the state driven is the same on two runs
    // over the same artifact.
    rungs.reduce((a, b) => (widest(b) > widest(a) ||
      (widest(b) === widest(a) && keyOf(b.path) < keyOf(a.path)) ? b : a)),
    rungs.find((r) => (r.marks || []).some((/** @type {any} */ m) => m.role === "residual")),
    rungs.find((r) => (r.marks || []).some((/** @type {any} */ m) => m.role === "gap")),
    rungs.find((r) => r.path.length === deepest),
  ];
  for (const r of wanted) {
    if (r && !chosen.some((p) => keyOf(p) === keyOf(r.path))) chosen.push(r.path);
  }
  return chosen;
}

/**
 * Puts the app on the chart one path names, from the overview.
 *
 * EXPANDED ON THE WAY IN AND NOT AT THE END. A node an outer chart's cap
 * folded away cannot be clicked until that column is expanded, so a path Go
 * answers is not always reachable without the gesture; the last open is not
 * followed by one, so the state measured is the chart as a reader is first
 * shown it, fold and all.
 */
async function goTo(app, path) {
  app.drillUp(0);
  for (const id of path) {
    expandAll(app);
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
    const wrong = { reach: [], ribbons: [], written: [], gestures: [], banner: [], tails: [] };
    const seen = {
      states: 0, answered: 0, marks: 0, attributes: 0, ribbons: 0, gestures: 0,
      tails: 0, ownMarks: 0, offscreen: 0, expansions: 0, revealed: 0, refused: 0,
      emphasis: 0, focus: 0,
    };

    for (const width of [3, 4]) {
      const rungs = column.rungs;
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
          seen.ownMarks += (answer.marks || []).length;
          // THE TAILS ARE READ OFF THE CHART, because whether a column folded
          // is the client's answer now. Their columns are what excuses an
          // accounted id from being drawn, and nothing else does.
          const tails = marks.filter((/** @type {any} */ m) => app.isAggregate(m.__data__.id));
          const folded = new Set(tails.map((/** @type {any} */ m) => m.__data__.tier));
          seen.tails += tails.length;
          // AND THE COLUMNS THIS BUDGET DOES NOT BUY. Go answers every tier
          // the step declares; a narrow window draws fewer, so an id at a
          // column that is not on screen at all is neither drawn nor folded.
          // Excused here and held elsewhere: rungs.mjs is what refuses a
          // dropped column the step does not declare a widening.
          const columns = new Set(app.activeTiers());
          const want = answeredIDs(answer);
          const tailIDs = [...folded].map((/** @type {number} */ t) => app.aggregateID(t));
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
            role: "button",
            tabindex: "0",
            "aria-keyshortcuts": "Enter Space",
          };
          const bad = Object.keys(want).filter((k) => m.getAttribute(k) !== want[k]);
          // THE class ATTRIBUTE CARRIES TWO CLAIMS AND THEY ARE CHECKED APART.
          // nodeClass composes what the mark IS -- published or derived, opens
          // or not -- once, at render, and never changes. applyEmphasis adds
          // and removes `dim` and `hot` through the same attribute as the
          // reader isolates and releases, so a plain equality would be a claim
          // about WHEN this arm happened to look. What holds at every moment is
          // that every token nodeClass composes is there, and that anything
          // else is one of the two the emphasis owns.
          const drawnClass = (m.getAttribute("class") || "").split(/\s+/).filter(Boolean);
          const rule = app.nodeClass(d).split(/\s+/).filter(Boolean);
          const missing = rule.filter((/** @type {string} */ c) => !drawnClass.includes(c));
          const extra = drawnClass.filter((/** @type {string} */ c) =>
            !rule.includes(c) && c !== "dim" && c !== "hot");
          if (missing.length) bad.push(`class is missing ${missing.join(" ")}`);
          if (extra.length) bad.push(`class carries ${extra.join(" ")}, which is neither nodeClass's nor the emphasis's`);
          seen.attributes += rule.length;
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
          // WHAT THE ISOLATION DOES FOR A READER WHO CAN SEE THE PAGE, which
          // app.isolated and the legend's aria-pressed do not witness: the
          // dimming. Stated as invariants rather than by recomputing
          // applyEmphasis's predicate here -- a copy would agree with itself.
          // The touching half is the one that matters: a page that dimmed
          // EVERYTHING would satisfy "something dimmed" and show the reader a
          // uniformly grey chart.
          const lit = ribbonsIn(chart).filter((/** @type {any} */ p) =>
            p.__data__.source.id === id || p.__data__.target.id === id);
          const dimmedRibbons = ribbonsIn(chart).filter((/** @type {any} */ p) => carries(p, "dim"));
          const dimmedMarks = marksIn(chart).filter((/** @type {any} */ m) => carries(m, "dim"));
          seen.emphasis += dimmedRibbons.length + dimmedMarks.length;
          if (dimmedRibbons.length === 0) note("isolates and dims no ribbon at all");
          if (dimmedMarks.length === 0) note("isolates and dims no other mark at all");
          if (lit.some((/** @type {any} */ p) => carries(p, "dim"))) {
            note(`dims ${lit.filter((/** @type {any} */ p) => carries(p, "dim")).length} of its own ${lit.length} ribbon(s)`);
          }
          if (carries(subject, "dim")) note("dims itself while isolated");

          seen.gestures += dispatch(subject, "click", { timeStamp: 2000 });
          if (app.isolated !== "") note(`does not release on a second click (isolated is ${JSON.stringify(app.isolated)})`);
          const stuck = ribbonsIn(chart).filter((/** @type {any} */ p) => carries(p, "dim")).length +
            marksIn(chart).filter((/** @type {any} */ m) => carries(m, "dim")).length;
          if (stuck > 0) note(`leaves ${stuck} mark(s) and ribbon(s) dimmed after the isolation is released`);

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
            // FOCUS ON A MARK IS THE STATE A READER IS ACTUALLY IN, and it is
            // the state focusInChart() could not recognise until the stub
            // answered contains(): a reader's focus is on a g.node, never on
            // #chart itself, so every focus arm in this directory drove the one
            // branch nobody uses and the branch everybody uses answered false.
            // A drill REPLACES the chart, so the element focus was on is gone
            // and restoreFocus has to put it somewhere -- which is the whole of
            // what hadFocus decides.
            app.dom.document.activeElement = subject;
            seen.gestures += dispatch(subject, "dblclick", { timeStamp: 10000 });
            await settle();
            const landed = app.dom.document.activeElement;
            seen.focus++;
            if (!landed || landed === subject) {
              note(`left focus ${landed ? "on the mark the drill replaced" : "nowhere"} after opening`);
            } else if (!carries(landed, "crumb-back") && !carries(landed, "node")) {
              note(`put focus on ${JSON.stringify(landed.getAttribute("class"))} after opening, ` +
                `which is neither the rung's own return control nor a mark of the chart it drew`);
            }
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

        // ------------------------------- (f) what a folded tail stands for
        //
        // THE GESTURE, AND THEN THE DOM AGAIN. (a) can say that an id Go
        // accounts for is either a mark or behind the tail of its own column
        // and cannot tell those two apart, so an id that leaves the answer
        // from behind a tail is invisible to it. A reader's own way of asking
        // what a tail stands for is Enter on the tail, and what comes back is
        // marks to read rather than a list to be trusted -- reading the
        // tail's own `folds` would be reading what the client meant to stand
        // for instead of what it drew, which is the distinction this whole
        // module exists to make.
        //
        // THE LABEL IS PART OF THE ANSWER, and it is the part addressed to the
        // reader: "21 smaller departments" is a promise about a number that
        // only the expansion can keep.
        if (answer) {
          // (d) leaves the chart on the rung it opened, so the state this arm
          // measures is established again rather than assumed.
          await goTo(app, path);
          const want = answeredIDs(answer);
          // MET BY NAME, EXPANDED BY PREDICATE, and the gap between the two is
          // what this arm has to account for. isAggregate finds every tail the
          // chart DREW; expandable answers for the ones the page will draw out,
          // and it offers the gesture only where this rung's step caps that
          // column -- so a tail folded into a kept flank by the chart above is
          // shown to the reader and refused.
          /** @type {any[]} */
          let refused = [];
          // THE CAP IS A BOUND ON THE LOOP AND NOT AN ANSWER, so a state that
          // reaches it is reported rather than left: `refused` is only filled
          // by the break, and a run that stopped counting mid-column would say
          // "no tail was left folded" about a chart it never finished reading.
          // An expansion removes the tail it draws out, so 32 is far past what
          // any state here needs.
          const LIMIT = 32;
          let done = 0;
          for (; done < LIMIT; done++) {
            const before = marksIn(chart);
            const shown = before.filter((/** @type {any} */ m) => app.isAggregate(m.__data__.id));
            const tail = shown.find((/** @type {any} */ m) => app.expandable(m.__data__));
            // A TAIL THE PAGE REFUSES IS NOT FORCED THROUGH expandTier, which
            // would measure a state no reader can reach. It leaves the loop to
            // be counted below instead.
            if (!tail) { refused = shown; break; }
            const tier = tail.__data__.tier;
            const drawn = before.map((/** @type {any} */ m) => m.__data__.id);
            const hidden = [...want.keys()].filter((id) =>
              want.get(id) === tier && !drawn.includes(id));
            const promised = tailPromise(tail);
            seen.expansions++;
            const note = (/** @type {string} */ what) =>
              wrong.tails.push(`${where}: the tail at column ${tier} ${what}`);
            try {
              dispatch(tail, "keydown", { key: "Enter", timeStamp: 20000 + done });
              await settle();
            } catch (e) {
              note(`did not expand -- ${e && e.message ? e.message : String(e)}`);
              break;
            }
            const out = marksIn(chart)
              .filter((/** @type {any} */ m) => m.__data__.tier === tier && !app.isAggregate(m.__data__.id))
              .map((/** @type {any} */ m) => m.__data__.id);
            const revealed = out.filter((id) => !drawn.includes(id));
            seen.revealed += revealed.length;
            const stillHidden = hidden.filter((id) => !out.includes(id));
            const strangers = out.filter((id) => !want.has(id));
            if (stillHidden.length) {
              note(`stood for ${hidden.length} id(s) Go accounts for and drew no mark for ` +
                `${stillHidden.length} of them: ${stillHidden.join(", ")}`);
            }
            if (strangers.length) {
              note(`drew ${strangers.join(", ")}, which Go accounts for nowhere at this rung`);
            }
            if (promised !== revealed.length) {
              note(`says "${promised}" in its own words and drew ${revealed.length} mark(s) ` +
                `the chart did not already carry`);
            }
          }
          if (done === LIMIT) {
            wrong.tails.push(`${where}: the chart still offered a tail to expand after ` +
              `${LIMIT} expansions, so what it was left holding was never read`);
          }
          // SHOWN AND COULD NOT IS NOT THE SAME AS NOT SHOWN, and the two must
          // not leave by the same door. The arm's ok still needs an expansion
          // and a revealed mark, so a run that met no tail at all is red on
          // those counters; this is the other half -- a tail the reader can see
          // whose members no gesture on this chart draws out, and which this
          // arm therefore cannot hold to Go's answer at all.
          const drawnNow = marksIn(chart).map((/** @type {any} */ m) => m.__data__.id);
          for (const m of refused) {
            seen.refused++;
            const tier = m.__data__.tier;
            const behind = [...want.keys()].filter((id) =>
              want.get(id) === tier && !drawnNow.includes(id));
            wrong.tails.push(`${where}: the tail at column ${tier} (${m.__data__.id}, ` +
              `${JSON.stringify(m.__data__.label)}) is drawn and the page offers no gesture ` +
              `that draws it out, so what it stands for is unaskable here -- ` +
              `${behind.length} id(s) Go accounts for at that column reach no mark: ` +
              `${behind.join(", ") || "(none)"}`);
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
    //
    // `refused` IS THE ONE COUNTER THAT IS NOT AN ok CLAUSE, because it counts
    // holes rather than comparisons: zero is its passing value, and what stops
    // that zero from meaning "no tail was met" is `expansions`, which is one.
    const drove = seen.states > 0 && seen.marks > 0;
    out.push({
      name: `${col.label}: every node the chart lays out reaches a mark, and every mark is one Go accounts for`,
      ok: wrong.reach.length === 0 && drove && seen.answered > 0 && seen.tails > 0 && seen.ownMarks > 0,
      detail: wrong.reach.length
        ? `${wrong.reach.length} disagreement(s), ${firstOf(wrong.reach)}`
        : `${seen.marks} mark(s) over ${seen.states} state(s), ${seen.answered} of them a rung ` +
          `testdata/rungs.json answers -- every id it accounts for drawn, behind one of ` +
          `${seen.tails} folded tail(s), or at one of ${seen.offscreen} places in a column this ` +
          `budget drops -- and ${seen.ownMarks} mark(s) the client adds of its own, each of ` +
          `which is an id no document names`,
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
      ok: wrong.gestures.length === 0 && drove && seen.gestures > 0 && seen.emphasis > 0 &&
        seen.focus > 0,
      detail: wrong.gestures.length
        ? `${wrong.gestures.length} gesture(s) that did something else, ${firstOf(wrong.gestures)}`
        : `${seen.gestures} handler(s) fired over ${seen.states} state(s): a click isolates and ` +
          `a second releases, Space isolates, the click that Space synthesises does not undo it, ` +
          `a held key does nothing, and a double click and Enter each open the node Go answers a ` +
          `rung for; and the isolation DIMS -- ${seen.emphasis} mark(s) and ribbon(s) read back ` +
          `dimmed, none of them the isolated node or a flow of its own, and none left dimmed ` +
          `once it is released; and from focus on a MARK -- the state a reader is in -- a drill ` +
          `moved it to the rung's own return control ${seen.focus} time(s) rather than leaving ` +
          `it on the element the redraw destroyed`,
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
    out.push({
      name: `${col.label}: a folded tail expands into the ids Go accounts for and no others, ` +
        `and no tail is drawn that the page will not draw out`,
      ok: wrong.tails.length === 0 && drove && seen.expansions > 0 && seen.revealed > 0,
      detail: wrong.tails.length
        ? `${wrong.tails.length} tail(s) standing for something else or left folded, ${firstOf(wrong.tails)}`
        : `${seen.expansions} tail(s) expanded by Enter on the mark itself, drawing ` +
          `${seen.revealed} mark(s) that were behind one: every id Go accounts for at that ` +
          `column reached a mark, no mark it accounts for nowhere was drawn beside them, and ` +
          `each tail drew as many as the words in it promised -- and ${seen.refused} tail(s) ` +
          `were drawn that the page offered no way to draw out`,
    });
  }
  return out;
}
