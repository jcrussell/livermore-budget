// What render() put in the SVG, read back off the DOM after a state is
// driven, held against the chart the client laid. What a mark should read is
// asked of the shipped nodeClass and nodeFlags; which states to drive is read
// off the pinned columns and the splits the expansion declares.
//
// The first block drives a handful of states per published column; the
// second is the expansion's own rendering claims.

import { describe, test, before } from "node:test";
import assert from "node:assert/strict";
import { readFileSync, readdirSync } from "node:fs";
import { join } from "node:path";

import {
  bootedApp, split, expandAll, everySplit, settle, refusals, columnFixture, pageFixture, fire, repoRoot,
  dollars, shortDollars,
} from "./testlib.mjs";

const CONFIG = pageFixture().config;
/** The two published columns, as the pinned page declares them. */
const YEARS = CONFIG.years.map((y) => ({
  stem: y.stem, label: y.label, path: y.path, chartTitle: y.chart_title,
  fixture: y.path.replace(/\.json$/, ""), config: y,
}));

/** @param {readonly string[]} entries a list of expansions */
const keyOf = (entries) => (entries.length ? entries.join(" > ") : "(the root)");

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
 * The fund group that splits into the most funds in the expansion of the
 * column on screen: the split the fund cap is for.
 * @param {any} app
 */
function worstOf(app) {
  const groups = app.fetched.splits.filter((sp) => sp.key === "funds").map((sp) => sp.node);
  return groups.reduce((a, b) => (app.splitsHere({ id: b })[0].members.length > app.splitsHere({ id: a })[0].members.length ? b : a));
}

/**
 * The states driven in one column, each a list of expansions: the root, the
 * first node the root splits, the group whose fund column is widest, the
 * General Fund group and its one fund, an object category by its divisions,
 * and everything by its first split (Expand all).
 * @param {any} app  booted and on the year
 * @returns {string[][]}
 */
function statesFor(app) {
  const chosen = [[]];
  const add = (entries) => { if (!chosen.some((p) => keyOf(p) === keyOf(entries))) chosen.push(entries); };
  add([app.projection.nodes.filter((n) => app.splittable(n)).map((n) => n.id).sort()[0]]);
  add([worstOf(app)]);
  add(["fund-group/general", "fund/100"]);
  add(["expenditure/services-and-supplies" + app.ALTERNATIVE_MARK + "divisions"]);
  app.collapseAll();
  app.expandAll();
  add(app.expansions.slice());
  app.collapseAll();
  return chosen;
}

/** Puts the app on the chart one list of expansions names. */
async function goTo(app, entries) {
  if (!app.redraw(entries)) throw new Error(`the chart at ${keyOf(entries)} did not draw`);
  await settle();
}

/** The chip row's children as class:text. */
const crumbs = (document) =>
  [...document.getElementById("breadcrumb").children].map((c) => `${c.className}:${c.textContent}`);

/**
 * Drives every chosen state of one column and reads six things off each,
 * returning what disagreed and what was counted.
 */
async function drive(year) {
  const wrong = { reach: [], ribbons: [], written: [], gestures: [], banner: [], tails: [] };
  const seen = {
    states: 0, marks: 0, attributes: 0, ribbons: 0, gestures: 0,
    tails: 0, expansions: 0, revealed: 0, refused: 0,
    emphasis: 0, focus: 0,
  };
  const { app, document } = await bootedApp({ checkedStem: year.stem });
  const chart = document.getElementById("chart");
  const content = document.querySelector("main");
  if (!content) throw new Error("the page has no <main>, so a refusal banner would have nowhere to be");

  for (const entries of statesFor(app)) {
    const where = `${year.label}, ${keyOf(entries)}`;
    seen.states++;
    // THE PLANTED CLOCK ONLY MOVES FORWARD across states, as a real one does:
    // the activation guard compares timestamps, and one earlier than the last
    // state's Enter on the same node would read as the click it synthesised.
    const t0 = seen.states * 100000;
    // A STATE THAT THROWS IS A STATE THAT DID NOT DRAW, reported as one
    // rather than left to take the file down.
    try {
      await goTo(app, entries);
    } catch (e) {
      wrong.banner.push(`${where}: redrawing it threw -- ${e && e.message ? e.message : String(e)}`);
      continue;
    }

    const marks = marksIn(chart);
    const ribbons = ribbonsIn(chart);

    // (e) no banner -- first, and on every state: every arm below reads a
    // DOM that app.js only fills when render() ran to the end.
    const banners = refusals(document);
    // The form draws into the zoom's layer, the one group the page makes.
    const layer = chart.querySelector(":scope > g.zoom-layer");
    const groups = layer ? [...layer.children].map((c) => c.getAttribute("class")) : [];
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
    // it; a held key does nothing; a double click and Enter each split.
    const opener = marks.find((m) => app.splittable(m.__data__));
    const subject = opener || marks[0];
    if (subject) {
      const id = subject.__data__.id;
      const note = (what) => wrong.gestures.push(`${where}: ${id} ${what}`);
      seen.gestures++; fire(subject, "click", { timeStamp: t0 + 1000 });
      if (app.filter.isolated !== id) note(`does not isolate on a click (isolated is ${JSON.stringify(app.filter.isolated)})`);
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
      if (app.filter.isolated !== "") note(`does not release on a second click (isolated is ${JSON.stringify(app.filter.isolated)})`);
      const stuck = ribbonsIn(chart).filter((p) => p.classList.contains("dim")).length +
        marksIn(chart).filter((m) => m.classList.contains("dim")).length;
      if (stuck > 0) note(`leaves ${stuck} mark(s) and ribbon(s) dimmed after the isolation is released`);

      seen.gestures++; fire(subject, "keydown", { key: " ", timeStamp: t0 + 3000 });
      if (app.filter.isolated !== id) note(`does not isolate on Space (isolated is ${JSON.stringify(app.filter.isolated)})`);
      seen.gestures++; fire(subject, "click", { timeStamp: t0 + 3000 });
      if (app.filter.isolated !== id) note("is un-isolated by the click its own Space synthesised");
      seen.gestures++; fire(subject, "keydown", { key: " ", timeStamp: t0 + 9000 });
      if (app.filter.isolated !== "") note(`does not release on a second Space (isolated is ${JSON.stringify(app.filter.isolated)})`);

      const before = app.expansions.length;
      seen.gestures++; fire(subject, "keydown", { key: "Enter", timeStamp: t0 + 9500, repeat: true });
      await settle();
      if (app.filter.isolated !== "" || app.expansions.length !== before) {
        note(`acts on a held key (isolated ${JSON.stringify(app.filter.isolated)}, ${app.expansions.length} split(s))`);
      }

      if (opener) {
        // A split replaces the mark, so restoreFocus must land focus somewhere real.
        subject.focus();
        seen.gestures++; fire(subject, "dblclick", { timeStamp: t0 + 10000 });
        await settle();
        const landed = document.activeElement;
        seen.focus++;
        if (!landed || landed === document.body || !landed.classList.contains("node")) {
          note(`left focus on ${landed && landed !== document.body ? JSON.stringify(landed.getAttribute("class")) : "nothing"} after splitting`);
        }
        if (app.expansions.length !== before + 1) {
          note(`does not split on a double click (${app.expansions.length} split(s), was ${before})`);
        }
        await goTo(app, entries);
        const again = marksIn(chart).find((m) => m.__data__.id === id);
        if (!again) note("is gone from the chart it was just split from");
        else {
          seen.gestures++; fire(again, "keydown", { key: "Enter", timeStamp: t0 + 11000 });
          await settle();
          if (app.expansions.length !== before + 1) {
            note(`does not split on Enter (${app.expansions.length} split(s), was ${before})`);
          }
        }
      }
    }

    // (f) what a folded tail stands for: Enter on the tail the way a reader
    // asks, and then the DOM again. The label is part of the claim, and so
    // is the tail's own `folds`: what it says it stands for is what drawing
    // it out reveals, no more and no fewer.
    if (entries.length) {
      await goTo(app, entries);
      let refused = [];
      const LIMIT = 64;
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
        const note = (what) => wrong.tails.push(`${where}: the tail ${tail.__data__.id} ${what}`);
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
        wrong.tails.push(`${where}: the chart still offered a tail to draw out after ${LIMIT} of them, ` +
          "so what it was left holding was never read");
      }
      // SHOWN AND COULD NOT IS NOT THE SAME AS NOT SHOWN: a tail the reader
      // can see whose members no gesture on this chart draws out.
      const drawnNow = marksIn(chart).map((m) => m.__data__.id);
      for (const m of refused) {
        seen.refused++;
        const behind = (m.__data__.folds || []).filter((id) => !drawnNow.includes(id));
        wrong.tails.push(`${where}: the tail ${m.__data__.id} (${JSON.stringify(m.__data__.label)}) is drawn ` +
          `and the page offers no gesture that draws it out -- ${behind.length} id(s) it stands for ` +
          `reach no mark: ${behind.join(", ") || "(none)"}`);
      }
    }
  }
  app.collapseAll();
  return { wrong, seen };
}

for (const year of YEARS) {
  describe(`${year.label}: the drawing`, () => {
    let wrong;
    let seen;
    before(async () => { ({ wrong, seen } = await drive(year)); });

    // Each arm carries the counter that could vanish under it alone.
    const drove = () => seen.states > 0 && seen.marks > 0;

    test(`${year.label}: every node the chart lays out reaches a mark`, (t) => {
      const detail = `${seen.marks} mark(s) over ${seen.states} state(s), ${seen.tails} of them folded tail(s)`;
      t.diagnostic(detail);
      assert.equal(wrong.reach.length, 0, `${wrong.reach.length} disagreement(s), ${firstOf(wrong.reach)}`);
      assert.ok(drove() && seen.tails > 0, detail);
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
        "a held key does nothing, and a double click and Enter each split a node that splits; " +
        `the isolation dims ${seen.emphasis} mark(s) and ribbon(s), none of them the isolated node or ` +
        "a flow of its own, and none left dimmed once released; and from focus on a mark a split " +
        `moved it to a mark of the chart it drew ${seen.focus} time(s)`;
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

    test(`${year.label}: a folded tail draws out into the ids Go accounts for and no others, and no tail is drawn that the page will not draw out`, (t) => {
      const detail = `${seen.expansions} tail(s) drawn out by Enter on the mark itself, drawing ` +
        `${seen.revealed} mark(s) that were behind one, each tail drawing as many as the words in ` +
        `it promised; ${seen.refused} tail(s) were drawn that the page offered no way to draw out`;
      t.diagnostic(detail);
      assert.equal(wrong.tails.length, 0, `${wrong.tails.length} tail(s) standing for something else or left folded, ${firstOf(wrong.tails)}`);
      assert.ok(drove() && seen.expansions > 0 && seen.revealed > 0, detail);
    });
  });
}

describe("the expansion's drawing", () => {
  /** Every drawn mark of the root and every split the page offers, both columns. */
  const records = [];
  const walks = {};
  const hundreds = [];
  const tally = { nodes: 0, opens: 0, derivedOnly: 0, expands: 0 };

  before(async () => {
    for (const year of YEARS) {
      const { app, document } = await bootedApp({ checkedStem: year.stem });
      const chart = document.getElementById("chart");
      const look = (where) => {
        for (const m of marksIn(chart)) {
          const d = m.__data__;
          const flag = flagOf(m);
          const opens = app.splittable(d);
          const expands = app.expandable(d);
          tally.nodes++;
          if (opens) tally.opens++;
          if (d.derived && !opens) tally.derivedOnly++;
          if (expands) tally.expands++;
          records.push({
            where: `${year.label} ${where}`, id: d.id,
            classes: (m.getAttribute("class") || "").split(/\s+/).filter(Boolean),
            flag: flag ? flag.textContent : null,
            rule: app.nodeClass(d), flags: app.nodeFlags(d),
            opens, expands, derived: Boolean(d.derived),
          });
          const share = app.shareOf(d);
          if (share.includes("100.0%") || share.includes("100%")) hundreds.push(`${year.label} ${where} ${d.id}: ${share}`);
        }
      };
      look("the root");
      walks[year.stem] = await everySplit(app, (where) => look(where));
    }
  });

  test("a node that splits is drawn as one, a tail that draws out is drawn as one, and a node that does neither is drawn as neither", (t) => {
    const wrong = records.filter((r) =>
      r.classes.includes("opens") !== r.opens ||
      r.classes.includes("expands") !== r.expands ||
      (r.flag || "").includes("⊞") !== r.expands ||
      (r.opens && r.expands) ||
      (r.flag || "").includes("▸") !== r.opens ||
      (r.flag || "").includes("◇") !== r.derived)
      .map((r) => `${r.where} > ${r.id} is drawn "${r.classes.join(" ")}" / "${r.flag}" but ` +
        `${r.opens ? "splits" : "does not split"}`);
    const visited = Object.values(walks).map((w) => w.visited).reduce((a, b) => a + b, 0);
    t.diagnostic(`over the root and ${visited} split chart(s) of both columns, ${tally.nodes} mark(s): ` +
      `${tally.opens} carry "opens" and the triangle, ${tally.derivedOnly} the diamond alone, ` +
      `${tally.expands} carry "expands" and the plus`);
    for (const w of Object.values(walks)) assert.equal(w.refused, "");
    assert.deepEqual(wrong.slice(0, 5), [], `${wrong.length} of ${tally.nodes} mark(s) are drawn as something they are not`);
    assert.ok(tally.opens > 0 && tally.expands > 0 && tally.derivedOnly > 0, "the corpus puts marks on both sides of each question");
  });

  test("the flag tspan a mark draws carries exactly nodeFlags", (t) => {
    const wrong = records.filter((r) => r.flag !== r.flags)
      .map((r) => `${r.where} > ${r.id} draws ${JSON.stringify(r.flag)} where nodeFlags says ${JSON.stringify(r.flags)}`);
    const flagged = records.filter((r) => r.flag).length;
    t.diagnostic(`${records.length} mark(s) read, ${flagged} of them drawing a glyph`);
    assert.deepEqual(wrong.slice(0, 5), [], `${wrong.length} mark(s) draw a flag that is not nodeFlags's`);
    assert.ok(flagged > 0);
  });

  test("no share on any chart claims 100% of a column that has more than one mark", async (t) => {
    // The case no pinned chart reaches: a column divided where toFixed(1)
    // would round to 100, by scaling one printed line's link a millionfold
    // and so its revenue category's.
    const doc = columnFixture(YEARS[0].fixture);
    const sched = doc.schedules.expansion;
    const parentOf = new Map(sched.nodes.map((n) => [doc.nodes[n.node].id, n.parent]));
    const l = sched.links.find((x) => doc.nodes[x.from].id.startsWith("revenue-line/") && x.kind === "external" && !x.alternative);
    l.value_cents = l.value_cents * 1000000;
    const category = parentOf.get(doc.nodes[l.from].id);
    const { app, document } = await bootedApp({ checkedStem: YEARS[0].stem, plan: { [YEARS[0].path]: { doc } } });
    const big = marksIn(document.getElementById("chart")).find((m) => m.__data__.id === category);
    const rounded = big ? app.shareOf(big.__data__) : "(the mark is not drawn)";
    t.diagnostic(`every share across the root and ${Object.values(walks).map((w) => w.visited).join(" + ")} ` +
      `split charts is under 100%; ${category} scaled to all but the whole of its column reads "${rounded}"`);
    assert.deepEqual(hundreds.slice(0, 3), [], `${hundreds.length} share(s) read 100%`);
    assert.ok(big, "the scaled mark is drawn");
    assert.ok(rounded.includes("of this column") && !rounded.includes("100"), rounded);
  });

  test("a partition ribbon says it is a cross-tab in its class, its two labels and the flow table", async (t) => {
    const { app, document } = await bootedApp({ checkedStem: YEARS[0].stem });
    const chart = document.getElementById("chart");
    const category = app.fetched.splits.find((sp) => sp.key === "divisions" && sp.alternative).node;
    await split(app, category + app.ALTERNATIVE_MARK + "divisions");
    const ribbons = ribbonsIn(chart).filter((p) => p.classList.contains("partition"));
    const ribbon = ribbons[0];
    const marks = marksIn(chart);
    const noted = marks.filter((m) => (m.getAttribute("aria-label") || "").includes(app.PARTITION_NOTE));
    const misnoted = marks.filter((m) => app.isPartitionNode(m.__data__) !==
      (m.getAttribute("aria-label") || "").includes(app.PARTITION_NOTE)).map((m) => m.__data__.id);
    const rows = [...document.getElementById("flow-table").querySelector("tbody").children];
    const cells = rows.filter((tr) => tr.children[4].textContent === app.PARTITION_NOTE).length;
    t.diagnostic(`${category} split by its divisions: ${ribbons.length} of ${ribbonsIn(chart).length} ribbon(s) ` +
      `carry "partition"; ${noted.length} of ${marks.length} mark(s) carry the sentence in their label; ` +
      `${cells} of ${rows.length} table row(s) read it in the provenance column`);
    assert.ok(ribbon, "no drawn ribbon carries the partition class");
    assert.ok(ribbon.getAttribute("aria-label").includes(app.PARTITION_NOTE), ribbon.getAttribute("aria-label"));
    assert.ok(!ribbon.classList.contains("derived"));
    assert.ok(noted.length > 0, "no mark's label carries the sentence");
    assert.deepEqual(misnoted, []);
    assert.equal(cells, ribbons.length);
  });

  test("the legend is the fund groups the marks are in, in Go's order, and a swatch follows the group where no group node is drawn", async (t) => {
    const { app, document } = await bootedApp({ checkedStem: YEARS[0].stem });
    const chart = document.getElementById("chart");
    const swatches = () => [...document.querySelectorAll("#legend [data-group]")];
    const atRoot = swatches().map((b) => b.dataset.group);
    // EVERY GROUP SPLIT INTO ITS FUNDS: funds of every group and no group
    // node on screen, so the swatch is the only key to the hues.
    for (const sp of app.fetched.splits.filter((x) => x.key === "funds")) await split(app, sp.node);
    const marks = marksIn(chart);
    const ribbons = ribbonsIn(chart);
    const legend = swatches();
    const ids = legend.map((b) => b.dataset.group);
    const inMarks = new Set(marks.map((m) => app.fundGroupOf(m.__data__)).filter(Boolean));
    t.diagnostic(`at the root the legend is ${JSON.stringify(atRoot.map((id) => id.replace("fund-group/", "")))}; ` +
      `with every group split it is ${JSON.stringify(ids.map((id) => id.replace("fund-group/", "")))} over ` +
      `${marks.length} mark(s), none of them a group node`);
    assert.deepEqual(atRoot, app.fundGroups().map((g) => g.id).filter((id) => atRoot.includes(id)));
    assert.ok(!marks.some((m) => app.isFundGroup(m.__data__)), "a group node is drawn, so a node toggle would do");
    assert.ok(ids.length >= 2, `the chart spans ${ids.length} group(s), so it tells a group from a node apart nowhere`);
    assert.deepEqual(ids, app.fundGroups().map((g) => g.id).filter((id) => inMarks.has(id)));
    for (const b of legend) {
      assert.equal(b.querySelector(".key").dataset.var, app.fundColorVar(b.dataset.group), b.dataset.group);
    }

    const pick = legend[0];
    const g = pick.dataset.group;
    fire(pick, "click");
    const lit = (els) => els.filter((e) => !e.classList.contains("dim")).map((e) => e.__data__);
    const litMarks = lit(marks);
    const litRibbons = lit(ribbons);
    const ofGroup = (n) => app.fundGroupOf(n) === g;
    t.diagnostic(`following ${g}: ${litMarks.length} of ${marks.length} mark(s) and ${litRibbons.length} of ` +
      `${ribbons.length} ribbon(s) stay lit`);
    assert.equal(pick.getAttribute("aria-pressed"), "true");
    assert.equal(app.filter.isolated, g);
    for (const m of marks) if (ofGroup(m.__data__)) assert.ok(litMarks.includes(m.__data__), `${m.__data__.id} is in ${g} and dimmed`);
    for (const r of ribbons) {
      const d = r.__data__;
      const mine = ofGroup(d.source) || ofGroup(d.target);
      assert.equal(litRibbons.includes(d), mine, `${d.source.id} -> ${d.target.id} is ${mine ? "" : "not "}${g}'s`);
    }
    assert.ok(litMarks.length < marks.length, "following the group dims no mark, so it follows nothing");
    fire(pick, "click");
    assert.equal(pick.getAttribute("aria-pressed"), "false");
    assert.equal(lit(marks).length, marks.length);
  });

  test("a ribbon with one group-less end wears the other end's hue, and one between two groups stays muted", async (t) => {
    const { app, document } = await bootedApp({ checkedStem: YEARS[0].stem });
    const chart = document.getElementById("chart");
    const muted = app.cssVar("--muted");
    let recoloured = 0;
    const charts = new Set();
    const wrong = [];
    const walked = await everySplit(app, async (where) => {
      for (const r of ribbonsIn(chart)) {
        const d = r.__data__;
        const a = app.fundGroupOf(d.source);
        const b = app.fundGroupOf(d.target);
        if (Boolean(a) === Boolean(b) || app.isFundGroup(d.source) || app.isFundGroup(d.target)) continue;
        const g = a || b;
        const want = app.cssVar(app.fundColorVar(g));
        if (r.getAttribute("stroke") !== want) wrong.push(`${where}: ${d.source.id} -> ${d.target.id} is ${r.getAttribute("stroke")}, not ${g}'s ${want}`);
        if (want !== muted) {
          recoloured++;
          charts.add(where);
        }
      }
    });
    assert.equal(walked.refused, "", walked.refused);
    t.diagnostic(`${YEARS[0].label}: ${recoloured} ribbon(s) with one group-less end on ${charts.size} of ` +
      `${walked.visited} chart(s) wear their other end's hue rather than --muted`);
    assert.deepEqual(wrong, [], firstOf(wrong));
    assert.ok(recoloured > 0, "no ribbon has one group-less end, so the arm is never drawn");

    // Two groups, neither of them a group node: picking one would be a guess.
    app.expandAll();
    await settle();
    const funds = marksIn(chart).map((m) => m.__data__).filter((n) => app.fundGroupOf(n) && !app.isFundGroup(n));
    const other = funds.find((n) => app.fundGroupOf(n) !== app.fundGroupOf(funds[0]));
    assert.ok(other, "everything split draws funds of one group only");
    assert.equal(app.linkGroup({ source: funds[0], target: other }), "");
    assert.equal(app.linkColor({ source: funds[0], target: other }), muted);
  });

  for (const [entries, stem] of [[[], "expansion"], [["fund-group/capital"], "expansion"]]) {
    const name = entries.length ? "with " + entries.join(" > ") + " split" : "at the root";
    test(`${name}: a marked node reaches the tooltip and the panel, and links to its own document's caveat`, async (t) => {
      const { app, document } = await bootedApp({ checkedStem: YEARS[0].stem });
      const chart = document.getElementById("chart");
      await split(app, ...entries);
      const marked = marksIn(chart).find((m) => app.caveatsFor(m.__data__.id).length > 0);
      // At the root a fund group carries the caveat its own remainder node
      // is named in: the remainder is folded inside it, as a tail's members
      // are inside the tail.
      assert.ok(marked, "no drawn node carries a caveat, so this asserts nothing");
      const laid = marked.__data__;
      app.showTip({ target: chart, clientX: 0, clientY: 0 }, laid);
      const tip = document.getElementById("tooltip").textContent;
      app.pin(laid);
      const panel = document.getElementById("detail").textContent;
      const caveat = app.caveatsFor(laid.id)[0].id;
      const href = app.caveatHref(caveat);
      const wantHref = `caveats.html#caveat-${stem}--${caveat}`;
      t.diagnostic(`${laid.id}: tooltip mentions ${tip.includes("caveat") ? "a caveat" : "NO caveat"}; ` +
        `panel ${panel.includes("Read it in full") ? "links to the full text" : "does NOT link"}; href "${href}"`);
      assert.ok(tip.includes("caveat"), tip);
      assert.ok(panel.includes("Read it in full"), panel);
      assert.equal(href, wantHref);
    });
  }

  for (const year of YEARS) {
    test(`${year.label}: a remainder Go derives is marked as ours, says why in Go's words, and reaches the inferred list, the tooltip and the panel; it splits nothing`, async (t) => {
      const { app, document } = await bootedApp({ checkedStem: year.stem });
      const chart = document.getElementById("chart");
      await split(app, "fund-group/capital");
      const mark = marksIn(chart).find((m) => m.__data__.id === "fund-group-only/capital");
      assert.ok(mark, "the capital group split draws no remainder of its own");
      const node = mark.__data__;
      const listed = document.getElementById("derived-list").textContent;
      app.showTip({ target: chart, clientX: 0, clientY: 0 }, node);
      const tip = document.getElementById("tooltip").textContent;
      app.pin(node);
      const panel = document.getElementById("detail").textContent;
      t.diagnostic(`"${node.label}" derived=${node.derived}, drawn class "${mark.getAttribute("class")}", ` +
        `rationale "${node.rationale.slice(0, 60)}..."`);
      assert.equal(node.derived, true);
      assert.ok(mark.classList.contains("derived"), mark.getAttribute("class"));
      assert.notEqual(node.rationale, "");
      assert.notEqual(node.source_note, "");
      assert.ok(listed.includes(node.label) && listed.includes(node.rationale), listed);
      assert.ok(tip.includes("◇ inferred") && tip.includes(node.rationale), tip);
      assert.ok(panel.includes("◇ our inference") && panel.includes(node.rationale) && panel.includes(node.source_note), panel);
      assert.equal(app.splittable(node), false);
    });

    test(`${year.label}: the capped tail is marked as ours, not as something the city printed`, async (t) => {
      const { app, document } = await bootedApp({ checkedStem: year.stem });
      const chart = document.getElementById("chart");
      const worst = worstOf(app);
      await split(app, worst);
      const tail = marksIn(chart).find((m) => app.isAggregate(m.__data__.id));
      assert.ok(tail, `no aggregate mark: the cap folded nothing on ${worst}`);
      const agg = tail.__data__;
      const word = CONFIG.overview.sankey.split_caps.find((c) => c.split === "funds").tail;
      const count = Number((agg.label.match(/^(\d+) smaller /) || [])[1]);
      t.diagnostic(`${worst} draws "${agg.label}" as "${tail.getAttribute("class")}", derived=${agg.derived}, ` +
        `rationale "${agg.rationale.slice(0, 48)}...", source note ${agg.source_note ? "present" : "MISSING"}`);
      assert.equal(agg.id, app.splitTailID(worst, "funds"));
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

    test(`${year.label}: the chip row says how many marks a tail drew out, and folds them back`, async (t) => {
      const { app, document } = await bootedApp({ checkedStem: year.stem });
      const chart = document.getElementById("chart");
      const worst = worstOf(app);
      await split(app, worst);
      const capped = ribbonsIn(chart).length;
      const tail = marksIn(chart).find((m) => app.expandable(m.__data__));
      assert.ok(tail, "no drawn mark offers to draw out");
      const tier = tail.__data__.tier;
      const word = CONFIG.overview.sankey.split_caps.find((c) => c.split === "funds").tail;
      fire(tail, "keydown", { key: "Enter", timeStamp: 1000 });
      await settle();
      const drawn = marksIn(chart).filter((m) => m.__data__.tier === tier && !app.isAggregate(m.__data__.id) &&
        app.nodeOf(m.__data__.id).parent === worst).length;
      const chipCrumbs = crumbs(document);
      const chip = [...document.getElementById("breadcrumb").querySelectorAll(".crumb-expanded")].at(-1);
      const chipText = chip.textContent;
      fire(chip, "click");
      await settle();
      const folded = ribbonsIn(chart).length;
      const label = app.nodeOf(worst).label;
      t.diagnostic(`drawn out, ${worst} draws ${drawn} ${word} and the chips read ${JSON.stringify(chipCrumbs)}; ` +
        `pressing the last left ${folded} ribbon(s) (capped: ${capped}) under ${JSON.stringify(crumbs(document))}`);
      assert.equal(chipText, app.say("tail_chip", { label, n: drawn, word }));
      assert.equal(chipCrumbs.length, 4);
      assert.ok(chipCrumbs[3].startsWith("crumb-expanded"), chipCrumbs[3]);
      assert.equal(folded, capped);
      assert.equal(crumbs(document).length, 3);
    });

    test(`${year.label}: a caveat about a folded row marks the tail while it stands for it, and the row itself once it is drawn`, async (t) => {
      // Which row the cap folds is the client's answer: read off a plain page,
      // then a caveat naming that row is planted on a copy of the column.
      const plain = await bootedApp({ checkedStem: year.stem });
      const worst = worstOf(plain.app);
      await split(plain.app, worst);
      const foldedTail = plain.app.projection.nodes.find((n) => plain.app.isAggregate(n.id));
      assert.ok(foldedTail && foldedTail.folds && foldedTail.folds.length, "the cap folded nothing");
      const foldedID = foldedTail.folds[0];
      const CAVEAT = { id: "a-folded-row", summary: "A caveat about a row the cap folds.", text: "", applies_to: [foldedID] };
      const doc = columnFixture(year.fixture);
      doc.schedules.expansion.caveats.push(CAVEAT);
      const { app, document } = await bootedApp({ checkedStem: year.stem, plan: { [year.path]: { doc } } });
      const chart = document.getElementById("chart");
      await split(app, worst);
      const marksCarrying = () => marksIn(chart).map((m) => m.__data__.id)
        .filter((id) => app.caveatsFor(id).some((c) => c.id === CAVEAT.id)).sort();
      const cappedMarks = marksCarrying();
      const cappedTail = marksIn(chart).find((m) => app.isAggregate(m.__data__.id));
      assert.ok(cappedTail, "no tail is drawn on the planted page");
      app.expandTier(cappedTail.__data__);
      await settle();
      const drawnOut = marksCarrying();
      t.diagnostic(`a caveat naming ${foldedID}, which the cap folds, is carried by ${JSON.stringify(cappedMarks)} ` +
        `on the capped chart and by ${JSON.stringify(drawnOut)} drawn out`);
      assert.deepEqual(cappedMarks, [cappedTail.__data__.id]);
      assert.deepEqual(drawnOut, [foldedID]);
    });

    test(`${year.label}: two chips the city prints in the same words are told apart by what each splits into`, async (t) => {
      const { app, document } = await bootedApp({ checkedStem: year.stem });
      await split(app, "fund-group/general", "fund/100");
      const words = [app.nodeOf("fund-group/general").label, app.nodeOf("fund/100").label];
      const chips = [...document.querySelectorAll("#breadcrumb .crumb-expanded")].map((c) => c.textContent);
      t.diagnostic(`the column labels the two ${JSON.stringify(words)}, and the chips read ${JSON.stringify(chips)}`);
      assert.equal(words[0], words[1], "the two labels no longer collide, so this asserts nothing");
      assert.deepEqual(chips, [
        app.say("chip_fold", { label: `${words[0]} (${app.splitWords("funds")})` }),
        app.say("chip_fold", { label: `${words[1]} (${app.splitWords("divisions")})` }),
      ]);
      assert.equal(new Set(chips).size, chips.length);
    });
  }
});

// Arms the committed columns never reach (fisc-7477), driven over a clone of
// a pinned column with one stated change, or a bare call.
describe("arms no committed column reaches", () => {
  const newest = YEARS[YEARS.length - 1];
  /**
   * A clone of the newest column with every leaf of one printed root cell
   * marked derived, so the ribbon the root draws for it is derived and both
   * its ends are printed; and a second printed cell left as it is.
   */
  const derivedCell = () => {
    const col = structuredClone(columnFixture(newest.fixture));
    const sched = col.schedules.expansion;
    const parentOf = new Map(sched.nodes.map((n) => [col.nodes[n.node].id, n.parent]));
    const rootOf = (id) => {
      let at = id;
      for (let hops = 0; parentOf.get(at) && hops < 9; hops++) at = parentOf.get(at);
      return at;
    };
    const printedRoot = (id) => !col.nodes.find((n) => n.id === id).derived;
    const cells = new Map();
    for (const l of sched.links) {
      if (l.alternative) continue;
      const key = rootOf(col.nodes[l.from].id) + ">" + rootOf(col.nodes[l.to].id) + ":" + l.kind;
      (cells.get(key) || cells.set(key, []).get(key)).push(l);
    }
    const printed = [...cells].filter(([key, legs]) => {
      const [ends] = key.split(":");
      const [from, to] = ends.split(">");
      return from !== to && printedRoot(from) && printedRoot(to) && legs.every((l) => !l.derived && !l.completes);
    });
    const [[cell, legs], [other]] = printed;
    for (const l of legs) l.derived = true;
    const [ends] = cell.split(":");
    const [from, to] = ends.split(">");
    const label = (id) => col.nodes.find((n) => n.id === id).label;
    return { col, cell: ends, other: other.split(":")[0], from: label(from), to: label(to) };
  };
  test("a derived flow between two published nodes is listed as inferred on its own, with both ends named", async (t) => {
    const { col, from, to } = derivedCell();
    const { document } = await bootedApp({ checkedStem: newest.stem, plan: { [newest.path]: { doc: col } } });
    assert.equal(refusals(document).length, 0);
    const items = [...document.querySelectorAll("#derived-list li")];
    const orphan = items.find((li) => li.textContent.includes("both endpoints are printed by the city"));
    t.diagnostic(`${items.length} inferred entries; the orphan reads "${orphan ? orphan.textContent : "(none)"}"`);
    assert.ok(orphan, "the inferred list does not mention the flow");
    assert.ok(orphan.textContent.includes(from + " → " + to), orphan.textContent);
  });
  test("a derived flow between two published nodes wears the inference chip in its panel, and a published one does not", async (t) => {
    const { col, cell, other } = derivedCell();
    const { app, document } = await bootedApp({ checkedStem: newest.stem, plan: { [newest.path]: { doc: col } } });
    const ribbon = (ids) => [...document.querySelectorAll("#chart path")].find((p) => p.__data__ && p.__data__.source &&
      p.__data__.source.id + ">" + p.__data__.target.id === ids);
    const chipsOf = (ids) => {
      const p = ribbon(ids);
      assert.ok(p, `${ids} is not drawn`);
      app.pin(p.__data__);
      return [...document.querySelectorAll("#detail .chip")].map((c) => c.className + ":" + c.textContent);
    };
    const derived = chipsOf(cell);
    const published = chipsOf(other);
    t.diagnostic(`derived ${cell}: ${JSON.stringify(derived)}; printed ${other}: ${JSON.stringify(published)}`);
    assert.ok(derived.includes("chip derived:\u25c7 our inference"), JSON.stringify(derived));
    assert.ok(published.includes("chip:printed by the city"), JSON.stringify(published));
  });
  test("a derived node wears the inference chip in its panel, and a published one does not", async (t) => {
    const { app, document } = await bootedApp({ checkedStem: newest.stem });
    const nodes = marksIn(document.getElementById("chart")).map((m) => m.__data__);
    const derived = nodes.find((n) => n.derived);
    const published = nodes.find((n) => !n.derived);
    assert.ok(derived && published, "the root draws no derived node beside a published one");
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
    delete col.schedules.expansion.caveats;
    const { app, document } = await bootedApp({ checkedStem: newest.stem, plan: { [newest.path]: { doc: col } } });
    assert.equal(refusals(document).length, 0);
    const marks = document.querySelectorAll("#chart g.node").length;
    const ids = app.projection.nodes.map((n) => n.id);
    t.diagnostic(`${marks} marks drawn, ${ids.length} ids asked, all with no caveat`);
    assert.ok(marks > 0);
    assert.ok(ids.every((id) => app.caveatsFor(id).length === 0));
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
    "zoom-layer": "the <g> the page makes for the form to draw into, which the zoom transforms",
    "link-hits": "the <g> a hairline's hit targets are drawn into",
    opens: "the mark that splits; its affordance is the flag tspan, not a style",
    expands: "the folded tail, likewise",
    "tip-drawn": "the tooltip's drawn-part line, worn as a tip-meta",
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
    // The root, the tooltip on a mark and on a ribbon, the panel on a
    // ribbon, a pin, an isolation, a zoom, every split the page offers with
    // the chooser and the chips, a tail drawn out, and a refusal.
    snap();
    const mark = marksIn(chart)[0];
    const ribbon = ribbonsIn(chart)[0];
    app.showTip({ target: chart, clientX: 0, clientY: 0 }, mark.__data__); snap();
    app.showTip({ target: chart, clientX: 0, clientY: 0 }, ribbon.__data__); snap();
    app.pin(ribbon.__data__); snap();
    app.pin(mark.__data__); snap();
    fire(mark, "keydown", { key: " " }); await settle(); snap();
    // A search listing its options, and a filter leaving part of a mark drawn.
    const search = document.getElementById("search-input");
    search.value = String(mark.__data__.label).slice(0, 3);
    search.dispatchEvent(new globalThis.Event("input", { bubbles: true })); snap();
    app.setFilter({ groups: new Set(["fund-group/general"]) });
    const part = marksIn(chart).find((m) => app.drawnAt(m.__data__));
    assert.ok(part, "no mark is drawn at less than its figure under the filter");
    app.showTip({ target: chart, clientX: 0, clientY: 0 }, part.__data__); app.pin(part.__data__); snap();
    app.resetFilters();
    // A zoomed chart, for the class that clips it.
    app.zoomBy(2); snap(); app.resetZoom();
    let expandedAt = "";
    const walked = await everySplit(app, async (where) => {
      snap();
      const laidRibbon = ribbonsIn(chart)[0];
      if (laidRibbon) { app.showTip({ target: chart, clientX: 0, clientY: 0 }, laidRibbon.__data__); app.pin(laidRibbon.__data__); snap(); }
      // The first chart that folds a split is drawn out once, for its chip.
      if (!expandedAt && app.projection.nodes.some((n) => app.expandable(n))) {
        expandAll(app); await settle(); snap();
        expandedAt = where;
      }
    });
    assert.equal(walked.refused, "");
    assert.ok(expandedAt, "no chart offered a tail to draw out");
    app.fail("probe: a refusal, for its class"); snap();

    const rules = ruleClasses(document);
    const templates = templateClasses();
    const unstyled = [...worn].filter((c) => !rules.has(c) && !(c in HOOKS)).sort();
    const unworn = [...rules].filter((c) => !worn.has(c) && !templates.has(c) && !(c in DEAD)).sort();
    t.diagnostic(`${worn.size} classes worn across ${walked.visited} charts and the states above (drawn out at ${expandedAt}); ${rules.size} classes styled; ` +
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
    assert.equal(app.say("fold_back"), config.wording.fold_back);
  });
  test("a reworded config is followed by the counts line, the hint, the description and the chip row", async (t) => {
    const config = structuredClone(pageFixture().config);
    for (const key of Object.keys(config.wording)) config.wording[key] = `«${key}» ` + config.wording[key];
    const { app, document } = await bootedApp({ config, checkedStem: YEARS[0].stem });
    const text = (id) => document.getElementById(id).textContent;
    const hintRoot = text("chart-hint");
    await split(app, "fund-group/general", "expenditure/services-and-supplies");
    const counts = text("counts-line");
    const hint = text("chart-hint");
    const chips = [...document.querySelectorAll("#breadcrumb button")].map((b) => b.textContent);
    const named = [...document.querySelectorAll("#breadcrumb [aria-label]")].map((b) => b.getAttribute("aria-label"));
    t.diagnostic(`counts "${counts}"; hint "${hint.slice(0, 80)}…"; chips ${JSON.stringify(chips)}; named ${JSON.stringify(named)}`);
    assert.ok(hintRoot.startsWith("«open_into» ") && hintRoot.includes(" «follow» ") && !hintRoot.includes("«fold_back»"), hintRoot);
    assert.ok(counts.startsWith("«counts_partial» "), counts);
    assert.match(counts, /\d+ (flow|flows) between \d+ (node|nodes)/);
    assert.ok(hint.includes(" «fold_back» "), hint);
    assert.deepEqual(chips.slice(0, 2), ["«expand_all» Expand all", "«collapse_all» Collapse all"]);
    assert.ok(chips[2].startsWith("«chip_fold» General Fund"), chips[2]);
    assert.ok(chips.includes("«split_labels» object rows") || chips.some((c) => c === app.splitWords("object-rows")), JSON.stringify(chips));
    assert.ok(named.some((n) => n.startsWith("«chip_fold_label» General Fund")), JSON.stringify(named));
    assert.ok(named.some((n) => n.startsWith("«split_chooser» Split Services")), JSON.stringify(named));
  });
});
