// What render() put in the SVG, read back off the DOM after a state is
// driven, held against the chart the client laid. What a mark should read is
// asked of the shipped nodeClass and nodeFlags; which states to drive is read
// off the pinned columns and the steps the page declares.
//
// The first block drives a handful of states per published column at two
// column budgets; the second is the drill's own rendering claims.

import { describe, test, before } from "node:test";
import assert from "node:assert/strict";
import { readFileSync, readdirSync } from "node:fs";
import { join } from "node:path";

import {
  bootedApp, opened, expandAll, everyOffer, settle, refusals, columnFixture, pageFixture, fire, repoRoot,
  dollars, shortDollars,
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
 * arm wrong on every mark does not bury the others.
 * @param {string[]} found
 */
const firstOf = (found) =>
  `${found.length} of them:\n      ${found.slice(0, 3).join("\n      ")}` +
  (found.length > 3 ? `\n      ... and ${found.length - 3} more` : "");

/**
 * The marks the client drew. THE ONE DOM READER: blinding it takes the whole
 * file red on its counters.
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
 * The fund group whose fund column is widest in the fund-flows schedule of
 * the column on screen: the view the fund cap is for. Read off the unfolded
 * schedule, by the group's own ribbons into the fund tier.
 * @param {any} app
 */
function worstOf(app) {
  const doc = app.scheduleOf(app.column, stepByKey("fund-group").projection);
  if (!doc) throw new Error("the column on screen carries no fund-flows schedule");
  const tierOf = new Map(doc.nodes.map((n) => [n.id, n.tier]));
  const funds = new Map();
  for (const l of doc.links) {
    if (tierOf.get(l.source) === 2 && tierOf.get(l.target) === 3) {
      (funds.get(l.source) || funds.set(l.source, new Set()).get(l.source)).add(l.target);
    }
  }
  return [...funds.entries()].reduce((a, b) => (b[1].size > a[1].size ? b : a))[0];
}

/**
 * The paths a step's declared gaps name for one year: every opened node
 * licensed to differ in this year's column.
 * @param {{year: number, basis: string}} year
 * @returns {string[][]}
 */
function gapPaths(year) {
  const out = [];
  for (const s of CONFIG.steps) {
    for (const [id, licences] of Object.entries(s.gaps || {})) {
      if (licences.some((g) => g.fiscal_year === year.year && g.basis === year.basis)) out.push([id]);
    }
  }
  return out;
}

/**
 * The states driven in one column: the overview, the first node the overview
 * offers, the group whose fund column is widest, the General Fund (which
 * carries a residual), each node a step licenses a gap on in this year, and
 * the chain three rungs deep.
 * @param {any} app  booted and on the year
 * @returns {Promise<string[][]>}
 */
async function statesFor(app) {
  const chosen = [[]];
  const first = app.projection.nodes.filter((n) => app.drillable(n)).map((n) => n.id).sort()[0];
  const add = (path) => { if (path && !chosen.some((p) => keyOf(p) === keyOf(path))) chosen.push(path); };
  add([first]);
  add([worstOf(app)]);
  add(["fund-group/general"]);
  for (const path of gapPaths(app.shownYear)) add(path);
  await opened(app, "fund-group/general", "fund/100");
  const division = app.projection.nodes.filter((n) => app.drillable(n)).map((n) => n.id).sort()[0];
  app.drillUp(0);
  await settle();
  add(["fund-group/general", "fund/100", division]);
  return chosen;
}

/**
 * Puts the app on the chart one path names, expanding on the way in (a folded
 * node cannot be clicked) and not at the end, so the state is fold and all.
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
  const wrong = { reach: [], ribbons: [], written: [], gestures: [], banner: [], tails: [] };
  const seen = {
    states: 0, marks: 0, attributes: 0, ribbons: 0, gestures: 0,
    tails: 0, ownMarks: 0, expansions: 0, revealed: 0, refused: 0,
    emphasis: 0, focus: 0,
  };
  /** @type {string[][] | null} */
  let states = null;

  for (const width of [3, 4]) {
    const { app, document } = await bootedApp({ checkedStem: year.stem });
    app.setColumnBudget(width);
    await settle();
    const chart = document.getElementById("chart");
    const content = document.querySelector("main");
    if (!content) throw new Error("the page has no <main>, so a refusal banner would have nowhere to be");

    if (!states) states = await statesFor(app);
    for (const path of states) {
      const where = `${year.label} at ${width} columns, ${keyOf(path)}`;
      seen.states++;
      // THE PLANTED CLOCK ONLY MOVES FORWARD across states, as a real one does:
      // the activation guard compares timestamps, and one earlier than the last
      // state's Enter on the same node would read as the click it synthesised.
      const t0 = seen.states * 100000;
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

      // (a) every node reaches a mark: DOM against the laid graph.
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
      seen.tails += marks.filter((m) => app.isAggregate(m.__data__.id)).length;
      seen.ownMarks += marks.filter((m) => app.isResidual(m.__data__.id) || app.isGap(m.__data__.id)).length;
      // EVERY DRAWN MARK STANDS AT A COLUMN THIS WIDTH DRAWS, or is a mark the
      // rung added beside them.
      const columns = new Set(app.activeTiers());
      const astray = marks.map((m) => m.__data__).filter((d) => path.length && !columns.has(d.tier))
        .map((d) => `${d.id}@${d.tier}`);
      if (astray.length) wrong.reach.push(`${where}: drawn at a column this budget does not lay out: ${astray.join(", ")}`);

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
        // Read off the tspans, not the composing functions, or d.label would
        // be compared with itself.
        if (!tspans.includes(d.label)) bad.push(`no tspan reading ${JSON.stringify(d.label)}`);
        const amount = "  " + shortDollars(app.markCents(d));
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
      const opener = marks.find((m) => app.drillable(m.__data__));
      const subject = opener || marks[0];
      if (subject) {
        const id = subject.__data__.id;
        const note = (what) => wrong.gestures.push(`${where}: ${id} ${what}`);
        seen.gestures++; fire(subject, "click", { timeStamp: t0 + 1000 });
        if (app.isolated !== id) note(`does not isolate on a click (isolated is ${JSON.stringify(app.isolated)})`);
        // The dimming, stated as invariants rather than recomputing the predicate.
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

        seen.gestures++; fire(subject, "click", { timeStamp: t0 + 2000 });
        if (app.isolated !== "") note(`does not release on a second click (isolated is ${JSON.stringify(app.isolated)})`);
        const stuck = ribbonsIn(chart).filter((p) => p.classList.contains("dim")).length +
          marksIn(chart).filter((m) => m.classList.contains("dim")).length;
        if (stuck > 0) note(`leaves ${stuck} mark(s) and ribbon(s) dimmed after the isolation is released`);

        seen.gestures++; fire(subject, "keydown", { key: " ", timeStamp: t0 + 3000 });
        if (app.isolated !== id) note(`does not isolate on Space (isolated is ${JSON.stringify(app.isolated)})`);
        seen.gestures++; fire(subject, "click", { timeStamp: t0 + 3000 });
        if (app.isolated !== id) note("is un-isolated by the click its own Space synthesised");
        seen.gestures++; fire(subject, "keydown", { key: " ", timeStamp: t0 + 9000 });
        if (app.isolated !== "") note(`does not release on a second Space (isolated is ${JSON.stringify(app.isolated)})`);

        const before = app.drilled.length;
        seen.gestures++; fire(subject, "keydown", { key: "Enter", timeStamp: t0 + 9500, repeat: true });
        await settle();
        if (app.isolated !== "" || app.drilled.length !== before) {
          note(`acts on a held key (isolated ${JSON.stringify(app.isolated)}, ${app.drilled.length} rung(s))`);
        }

        if (opener) {
          // A drill replaces the chart, so restoreFocus must land focus somewhere real.
          subject.focus();
          seen.gestures++; fire(subject, "dblclick", { timeStamp: t0 + 10000 });
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
            seen.gestures++; fire(again, "keydown", { key: "Enter", timeStamp: t0 + 11000 });
            await settle();
            if (app.drilled.length !== before + 1) {
              note(`does not open on Enter (${app.drilled.length} rung(s), was ${before})`);
            }
          }
        }
      }

      // (f) what a folded tail stands for: Enter on the tail the way a reader
      // asks, and then the DOM again. The label is part of the claim, and so
      // is the tail's own `folds`: what it says it stands for is what drawing
      // it out reveals, no more and no fewer.
      if (path.length) {
        await goTo(app, path);
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
          const hidden = (tail.__data__.folds || []).filter((id) => !drawnBefore.includes(id));
          const promised = tailPromise(tail);
          seen.expansions++;
          const note = (what) => wrong.tails.push(`${where}: the tail at column ${tier} ${what}`);
          fire(tail, "keydown", { key: "Enter", timeStamp: t0 + 20000 + done });
          await settle();
          const out = marksIn(chart)
            .filter((m) => m.__data__.tier === tier && !app.isAggregate(m.__data__.id))
            .map((m) => m.__data__.id);
          const revealed = out.filter((id) => !drawnBefore.includes(id));
          seen.revealed += revealed.length;
          const stillHidden = hidden.filter((id) => !out.includes(id));
          const strangers = revealed.filter((id) => !hidden.includes(id));
          if (stillHidden.length) {
            note(`stood for ${hidden.length} id(s) and drew no mark for ` +
              `${stillHidden.length} of them: ${stillHidden.join(", ")}`);
          }
          if (strangers.length) note(`drew ${strangers.join(", ")}, which it did not say it stood for`);
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
          const behind = (m.__data__.folds || []).filter((id) => !drawnNow.includes(id));
          wrong.tails.push(`${where}: the tail at column ${tier} (${m.__data__.id}, ` +
            `${JSON.stringify(m.__data__.label)}) is drawn and the page offers no gesture ` +
            `that draws it out, so what it stands for is unaskable here -- ` +
            `${behind.length} id(s) it stands for reach no mark: ${behind.join(", ") || "(none)"}`);
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

    test(`${year.label}: every node the chart lays out reaches a mark, at a column the width draws`, (t) => {
      const detail = `${seen.marks} mark(s) over ${seen.states} state(s), ${seen.tails} of them folded ` +
        `tail(s) and ${seen.ownMarks} mark(s) the rung adds of its own`;
      t.diagnostic(detail);
      assert.equal(wrong.reach.length, 0, `${wrong.reach.length} disagreement(s), ${firstOf(wrong.reach)}`);
      assert.ok(drove() && seen.tails > 0 && seen.ownMarks > 0, detail);
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
        "a held key does nothing, and a double click and Enter each open a node that opens a " +
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
    // The case no pinned view reaches: a column divided where toFixed(1)
    // would round to 100, by scaling one printed link a millionfold.
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

  test("on a chart of four or five columns the hint names an inner column by its place, not as the middle", async (t) => {
    const w = CONFIG.wording;
    const said = [];
    // The General Fund's group window is five columns and its fund window four.
    for (const [path, width] of [[["fund-group/general"], 5], [["fund-group/general", "fund/100"], 4]]) {
      const { app, document } = await bootedApp({ checkedStem: YEARS[0].stem, viewport: 2000 });
      await opened(app, ...path);
      const tiers = app.activeTiers();
      const opening = new Set(app.projection.nodes.filter(app.drillable).map((n) => n.tier));
      const hint = document.getElementById("chart-hint").textContent;
      said.push(`${path[path.length - 1]} at {${tiers}} opens from {${[...opening]}}: "${hint}"`);
      assert.equal(tiers.length, width);
      assert.deepEqual([...opening].map((tier) => tiers.indexOf(tier)), [2]);
      assert.deepEqual(app.openableColumns(), [w.column_third]);
      assert.ok(hint.includes(app.say("in_column", { columns: w.column_third })), hint);
    }
    t.diagnostic(said.join("; "));
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
      // A drawn mark, not the centre: the opened node is alone in its column,
      // so its share is suppressed by construction.
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
      // The rendered panel, not caveatHref: the only route red on the caller.
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
      // THE WORDS ARE THE DECLARATION'S: the mark is named for the step's grain
      // and carries the step's reason for every endpoint it took a flow from.
      const grain = stepByKey("fund-group").residual_grain;
      assert.ok(grain, "the fund-group step declares no residual_grain");
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
      assert.ok(node.label.includes(grain), node.label);
      assert.notEqual(node.rationale, "");
      assert.deepEqual(missingReasons, []);
      assert.ok(node.source_note.includes("Carried, not computed"), node.source_note);
      assert.ok(citedPages.length > 0 && citedPages.every((pg) => node.source_note.includes(String(pg))), node.source_note);
      assert.ok(listed.includes(node.label) && listed.includes(node.rationale), listed);
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
      const worst = worstOf(app);
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
      // At two or more: a tail of one printed fund labelled "1 smaller funds" is the defect.
      assert.ok(count >= 2, agg.label);
      // Its note carries its figure, since the tail gets no inferred-list line (fisc-hrfd).
      const value = app.layOut(app.projection).nodes.find((n) => n.id === agg.id).value;
      assert.ok(value > 0);
      assert.ok(agg.source_note.includes("together " + dollars(value)), `${agg.source_note} does not carry ${dollars(value)}`);
    });

    test(`${year.label}: the breadcrumb says how many marks the expansion drew, and folds them back`, async (t) => {
      const { app, document } = await bootedApp({ checkedStem: year.stem });
      const chart = document.getElementById("chart");
      await opened(app, worstOf(app));
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
      // Which row the cap folds is the client's answer: read off a plain page,
      // then a caveat naming that row is planted on a copy of the column.
      const plain = await bootedApp({ checkedStem: year.stem });
      const worst = worstOf(plain.app);
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

describe("a gap mark", () => {
  // Every gap a step licenses, in whichever column licenses one.
  const gaps = YEARS.flatMap((year) => gapPaths(year.config).map((path) => ({ year, path })));
  test("the pinned page licenses a gap to drive", () => assert.ok(gaps.length > 0));
  for (const { year, path } of gaps) {
    test(`${year.stem} ${path.join(" > ")}: the gap and its ribbon cite the pages of both totals and say what they are, with no empty label`, async (t) => {
      const { app, document } = await bootedApp({ checkedStem: year.stem });
      const step = app.stepFor(app.projection.nodes.find((n) => n.id === path[0]));
      const licence = step.gaps[path[0]].find((g) => g.fiscal_year === year.config.year && g.basis === year.config.basis);
      // THE PAGES BOTH TOTALS WERE READ FROM: every page a ribbon touching the
      // opened node cites, in the chart above's document and in the drawn one.
      const above = app.docAt(0);
      await opened(app, ...path);
      const chart = document.getElementById("chart");
      const detail = document.getElementById("detail");
      const id = app.gapID(path[path.length - 1]);
      const node = marksIn(chart).find((m) => m.__data__.id === id);
      const ribbon = [...chart.querySelectorAll("path")].find((p) => p.__data__ && p.__data__.source &&
        (p.__data__.source.id === id || p.__data__.target.id === id));
      assert.ok(node && ribbon, `${id} or its ribbon is not drawn`);
      const touching = (doc) => {
        const inside = app.withinNode(doc, path[path.length - 1]);
        return doc.links.filter((l) => inside.has(l.source) || inside.has(l.target))
          .flatMap((l) => l.locators.flatMap((s) => s.pages));
      };
      const want = [...new Set(touching(above).concat(touching(app.drawnDoc())))].sort((a, b) => a - b);
      const pdfPages = () => [...detail.querySelectorAll("a")].map((a) => a.getAttribute("href") || "")
        .filter((href) => href.includes("#page=")).map((href) => Number(href.split("#page=")[1]));
      app.pin(node.__data__);
      const nodeCites = pdfPages();
      const nodePanel = detail.textContent;
      app.pin(ribbon.__data__);
      const ribbonCites = pdfPages();
      const panel = detail.textContent;
      const chips = [...detail.querySelectorAll(".chip")].map((c) => c.textContent);
      const aria = ribbon.getAttribute("aria-label") || "";
      const figure = Math.max(node.__data__.in_cents || 0, node.__data__.out_cents || 0);
      t.diagnostic(`the two documents cite pp.${want.join(",")}; the node's panel pp.${nodeCites.join(",")}, ` +
        `the ribbon's pp.${ribbonCites.join(",")}; the mark stands for ${figure} cents against a licence of ` +
        `${licence.cents}; chips ${JSON.stringify(chips)}; aria "${aria}"`);
      assert.deepEqual(nodeCites, want);
      assert.deepEqual(ribbonCites, want);
      assert.equal(figure, Math.abs(licence.cents));
      assert.ok(!panel.includes("Facts:"), panel);
      assert.ok(panel.includes(node.__data__.source_note), panel);
      assert.ok(nodePanel.includes(licence.reason), nodePanel);
      assert.ok(chips.every((c) => c !== ""), JSON.stringify(chips));
      assert.doesNotMatch(aria, /, ,/);
    });
  }
});

// Arms the committed columns never reach (fisc-7477), driven over a clone of
// a pinned column with one stated change, or a bare call.
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
  test("a derived flow between two published nodes wears the inference chip in its panel, and a published one does not", async (t) => {
    const col = structuredClone(columnFixture(newest.fixture));
    const printed = (l) => !l.derived && !col.nodes[l.from].derived && !col.nodes[l.to].derived;
    const [link, other] = col.schedules.sankey.links.filter(printed);
    assert.ok(link && other, "the pinned column has no two printed flows between published nodes");
    link.derived = true;
    const ids = (l) => [col.nodes[l.from].id, col.nodes[l.to].id].join(">");
    const { app, document } = await bootedApp({ checkedStem: newest.stem, plan: { [newest.path]: { doc: col } } });
    const ribbon = (l) => [...document.querySelectorAll("#chart path")].find((p) => p.__data__ && p.__data__.source &&
      p.__data__.source.id + ">" + p.__data__.target.id === ids(l));
    const chipsOf = (l) => {
      const p = ribbon(l);
      assert.ok(p, `${ids(l)} is not drawn`);
      app.pin(p.__data__);
      return [...document.querySelectorAll("#detail .chip")].map((c) => c.className + ":" + c.textContent);
    };
    const derived = chipsOf(link);
    const published = chipsOf(other);
    t.diagnostic(`derived ${ids(link)}: ${JSON.stringify(derived)}; printed ${ids(other)}: ${JSON.stringify(published)}`);
    assert.ok(derived.includes("chip derived:\u25c7 our inference"), JSON.stringify(derived));
    assert.ok(published.includes("chip:printed by the city"), JSON.stringify(published));
  });
  test("a derived node wears the inference chip in its panel, and a published one does not", async (t) => {
    const { app, document } = await bootedApp({ checkedStem: newest.stem });
    const nodes = marksIn(document.getElementById("chart")).map((m) => m.__data__);
    const derived = nodes.find((n) => n.derived);
    const published = nodes.find((n) => !n.derived);
    assert.ok(derived && published, "the overview draws no derived node beside a published one");
    const chipsOf = (n) => {
      app.pin(n);
      return [...document.querySelectorAll("#detail .chip")].map((c) => c.className + ":" + c.textContent);
    };
    const d = chipsOf(derived);
    const p = chipsOf(published);
    t.diagnostic(`${derived.id}: ${JSON.stringify(d)}; ${published.id}: ${JSON.stringify(p)}`);
    assert.ok(d.includes("chip derived:\u25c7 our inference"), JSON.stringify(d));
    assert.ok(p.includes("chip:printed by the city"), JSON.stringify(p));
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
    // The guard on a source with no caveats list at all.
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
// no page wears is dead ink (fisc-a0fv). The other templates' classes are
// theirs, not the client's.
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

// Every sentence the client composes is a config wording template that say()
// fills, so a reworded config is followed on the page.
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
