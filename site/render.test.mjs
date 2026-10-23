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
  bootedApp, loadApp, split, expandAll, everySplit, everyExtendPair, settle, refusals, columnFixture, pageFixture, fire, repoRoot,
  dollars, shortDollars, clickYear,
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
 * it, or off its title where they are cut; -1 when neither reads as a count.
 * @param {Element} mark
 */
function tailPromise(mark) {
  for (const w of tspansOf(mark)) {
    const found = /^(\d+) smaller /.exec(w);
    if (found) return Number(found[1]);
  }
  // A label cut to its room keeps its full words in the mark's title.
  const title = [...mark.children].find((c) => c.tagName === "title");
  const found = title && /(?:^|: )(\d+) smaller /.exec(title.textContent);
  return found ? Number(found[1]) : -1;
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
 * General Fund group, opened straight through its one fund, an object
 * category by its divisions,
 * and everything by its first split (Expand all).
 * @param {any} app  booted and on the year
 * @returns {string[][]}
 */
function statesFor(app) {
  const chosen = [[]];
  const add = (entries) => { if (!chosen.some((p) => keyOf(p) === keyOf(entries))) chosen.push(entries); };
  add([app.projection.nodes.filter((n) => app.splittable(n)).map((n) => n.id).sort()[0]]);
  add([worstOf(app)]);
  add(["fund-group/general"]);
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
      const want = { role: "button", tabindex: "0", "aria-keyshortcuts": "Enter Space M Shift+F10" };
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
      // be compared with itself. A label cut to its gutter (labelWords)
      // carries an ellipsis and its full words in the mark's title.
      const title = [...m.children].find((c) => c.tagName === "title");
      if (title) {
        if (!title.textContent.includes(d.label)) bad.push(`a title not reading ${JSON.stringify(d.label)}`);
        if (!(tspans[0] + tspans[1]).includes("\u2026")) bad.push("a title on a label drawn whole");
      } else if (!tspans.includes(d.label)) bad.push(`no tspan reading ${JSON.stringify(d.label)}`);
      const amount = "  " + shortDollars(app.markCents(d));
      if (!tspans.includes(amount)) bad.push(`no tspan reading ${JSON.stringify(amount)}`);
      seen.attributes += 3;
      if (bad.length) {
        const attrs = [...m.attributes].map((a) => `${a.name}=${JSON.stringify(a.value)}`).join(" ");
        wrong.written.push(`${where}: ${d.id} is drawn class=${JSON.stringify(m.getAttribute("class"))} ` +
          `with ${attrs} and tspans ${JSON.stringify(tspans)}; wrong or missing: ${bad.join(", ")}`);
      }
    }

    // (d) what a gesture DOES, by dispatch: focus pins a mark, the drawer's
    // Follow isolates it and a second releases it; a held key does nothing;
    // a click pins a mark that does not split, and a click and Enter each
    // split one that does, pinning it.
    const opener = marks.find((m) => app.splittable(m.__data__));
    const subject = opener || marks[0];
    if (subject) {
      const id = subject.__data__.id;
      const note = (what) => wrong.gestures.push(`${where}: ${id} ${what}`);
      const pinnedHere = () => Boolean(app.pinned) && app.pinned.id === id;
      const follow = document.getElementById("detail-follow");
      // Focus already on it (restoreFocus) would fire no focus to pin it.
      subject.blur();
      subject.focus();
      if (!pinnedHere()) note("is not pinned by focus");
      seen.gestures++; fire(follow, "click");
      if (app.filter.isolated !== id) note(`does not isolate on the drawer's Follow (isolated is ${JSON.stringify(app.filter.isolated)})`);
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

      seen.gestures++; fire(follow, "click");
      if (app.filter.isolated !== "") note(`does not release on a second Follow (isolated is ${JSON.stringify(app.filter.isolated)})`);
      const stuck = ribbonsIn(chart).filter((p) => p.classList.contains("dim")).length +
        marksIn(chart).filter((m) => m.classList.contains("dim")).length;
      if (stuck > 0) note(`leaves ${stuck} mark(s) and ribbon(s) dimmed after the isolation is released`);

      const before = app.expansions.length;
      seen.gestures++; fire(subject, "keydown", { key: "Enter", timeStamp: t0 + 9500, repeat: true });
      await settle();
      if (app.filter.isolated !== "" || app.expansions.length !== before) {
        note(`acts on a held key (isolated ${JSON.stringify(app.filter.isolated)}, ${app.expansions.length} split(s))`);
      }

      if (!opener) {
        app.unpin();
        seen.gestures++; fire(subject, "click", { timeStamp: t0 + 9800 });
        await settle();
        if (!pinnedHere() || app.expansions.length !== before) note("is not pinned by a click, or a click split it");
      }

      if (opener) {
        // A split replaces the mark, so restoreFocus must land focus somewhere real.
        subject.focus();
        seen.gestures++; fire(subject, "click", { timeStamp: t0 + 10000 });
        await settle();
        const landed = document.activeElement;
        seen.focus++;
        if (!landed || landed === document.body || !landed.classList.contains("node")) {
          note(`left focus on ${landed && landed !== document.body ? JSON.stringify(landed.getAttribute("class")) : "nothing"} after splitting`);
        }
        if (app.expansions.length !== before + 1) {
          note(`does not split on a click (${app.expansions.length} split(s), was ${before})`);
        }
        if (!pinnedHere()) note("is not pinned once a click split it");
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
        "aria-label, a rect with a positive box, and tspans carrying the label (or, cut to its gutter, an " +
        "ellipsis, with a title carrying the label) and the short signed amount";
      t.diagnostic(detail);
      assert.equal(wrong.written.length, 0, `${wrong.written.length} mark(s) drawn as something else, ${firstOf(wrong.written)}`);
      assert.ok(drove() && seen.attributes > 0, detail);
    });

    test(`${year.label}: a gesture on a drawn mark does what the page says it does`, (t) => {
      const detail = `${seen.gestures} event(s) fired over ${seen.states} state(s): focus pins, the drawer's ` +
        "Follow isolates and a second releases, a held key does nothing, a click pins a node that " +
        "does not split, and a click and Enter each split a node that splits, pinning it; " +
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
  const tally = { nodes: 0, opens: 0, folds: 0, derivedOnly: 0, expands: 0 };

  before(async () => {
    for (const year of YEARS) {
      const { app, document } = await bootedApp({ checkedStem: year.stem });
      const chart = document.getElementById("chart");
      const look = (where) => {
        const glyphs = new Map([...chart.querySelectorAll("g.glyph")].map((g) =>
          [g.__data__.id, g.classList.contains("glyph-open") ? "open" : g.classList.contains("glyph-fold") ? "fold" : "?"]));
        for (const m of marksIn(chart)) {
          const d = m.__data__;
          const flag = flagOf(m);
          const opens = app.splittable(d);
          const folds = app.foldable(d);
          const expands = app.expandable(d);
          tally.nodes++;
          if (opens) tally.opens++;
          if (folds) tally.folds++;
          if (d.derived && !opens) tally.derivedOnly++;
          if (expands) tally.expands++;
          records.push({
            where: `${year.label} ${where}`, id: d.id,
            classes: (m.getAttribute("class") || "").split(/\s+/).filter(Boolean),
            flag: flag ? flag.textContent : null,
            rule: app.nodeClass(d), flags: app.nodeFlags(d),
            opens, folds, expands, derived: Boolean(d.derived), glyph: glyphs.get(d.id) || "",
          });
          const share = app.shareOf(d);
          if (share.includes("100.0%") || share.includes("100%")) hundreds.push(`${year.label} ${where} ${d.id}: ${share}`);
        }
      };
      look("the root");
      walks[year.stem] = await everySplit(app, (where) => look(where));
    }
  });

  // Mutation: draw the glyph beside every node the schedule splits, open or
  // not, and this goes red, ⊕ beside a hub.
  test("a node that splits is drawn as one with ⊕ beside it, a hub with ⊖, a tail that draws out as one, and a node that does none of these as none", (t) => {
    const wrong = records.filter((r) =>
      r.classes.includes("opens") !== r.opens ||
      r.classes.includes("expands") !== r.expands ||
      (r.flag || "").includes("⊞") !== r.expands ||
      (r.opens && r.expands) ||
      (r.glyph === "open") !== r.opens ||
      (r.glyph === "fold") !== r.folds ||
      (r.flag || "").includes("◇") !== r.derived)
      .map((r) => `${r.where} > ${r.id} is drawn "${r.classes.join(" ")}" / "${r.flag}" but ` +
        `${r.opens ? "splits" : "does not split"}`);
    const visited = Object.values(walks).map((w) => w.visited).reduce((a, b) => a + b, 0);
    t.diagnostic(`over the root and ${visited} split chart(s) of both columns, ${tally.nodes} mark(s): ` +
      `${tally.opens} carry "opens" and ⊕, ${tally.folds} ⊖, ${tally.derivedOnly} the diamond alone, ` +
      `${tally.expands} carry "expands" and the plus`);
    for (const w of Object.values(walks)) assert.equal(w.refused, "");
    assert.deepEqual(wrong.slice(0, 5), [], `${wrong.length} of ${tally.nodes} mark(s) are drawn as something they are not`);
    assert.ok(tally.opens > 0 && tally.folds > 0 && tally.expands > 0 && tally.derivedOnly > 0, "the corpus puts marks on both sides of each question");
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

  test("a partition ribbon says it is a cross-tab in its class and its two labels", async (t) => {
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
    t.diagnostic(`${category} split by its divisions: ${ribbons.length} of ${ribbonsIn(chart).length} ribbon(s) ` +
      `carry "partition"; ${noted.length} of ${marks.length} mark(s) carry the sentence in their label`);
    assert.ok(ribbon, "no drawn ribbon carries the partition class");
    assert.ok(ribbon.getAttribute("aria-label").includes(app.PARTITION_NOTE), ribbon.getAttribute("aria-label"));
    assert.ok(!ribbon.classList.contains("derived"));
    assert.ok(noted.length > 0, "no mark's label carries the sentence");
    assert.deepEqual(misnoted, []);
  });

  test("the legend is the fund groups the marks are in, in Go's order, and a swatch follows the group across its funds", async (t) => {
    const { app, document } = await bootedApp({ checkedStem: YEARS[0].stem });
    const chart = document.getElementById("chart");
    const swatches = () => [...document.querySelectorAll("#legend [data-group]")];
    const atRoot = swatches().map((b) => b.dataset.group);
    // The heading over the buttons ships hidden and shows once they are
    // drawn. Mutation: drop buildLegend's unhiding and it stays hidden.
    const head = document.getElementById("legend-head");
    assert.ok(head && !head.hidden && document.getElementById("legend").getAttribute("aria-labelledby") === "legend-head",
      "the legend's heading is missing, hidden or not its label");
    // EVERY GROUP SPLIT INTO ITS FUNDS: funds of every group drawn beside
    // their group's node, which the swatch keys the hues of.
    for (const sp of app.fetched.splits.filter((x) => x.key === "funds")) await split(app, sp.node);
    const marks = marksIn(chart);
    const ribbons = ribbonsIn(chart);
    const legend = swatches();
    const ids = legend.map((b) => b.dataset.group);
    const inMarks = new Set(marks.map((m) => app.fundGroupOf(m.__data__)).filter(Boolean));
    t.diagnostic(`at the root the legend is ${JSON.stringify(atRoot.map((id) => id.replace("fund-group/", "")))}; ` +
      `with every group split it is ${JSON.stringify(ids.map((id) => id.replace("fund-group/", "")))} over ` +
      `${marks.length} mark(s)`);
    assert.deepEqual(atRoot, app.fundGroups().map((g) => g.id).filter((id) => atRoot.includes(id)));
    assert.ok(marks.some((m) => app.isFundGroup(m.__data__)), "no group node is drawn beside its funds");
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

  /** The href Go emitted for a caveat id in one year's refs, or "". */
  const emittedHref = (year, id) => (year.config.caveats.find((c) => c.id === id) || {}).href || "";
  /** Whether every pinned year carrying an id links it to one anchor. */
  const oneAnchor = (id) => new Set(YEARS.map((y) => emittedHref(y, id)).filter(Boolean)).size === 1;
  /** Whether more than one pinned year carries an id. */
  const inSeveralYears = (id) => YEARS.filter((y) => emittedHref(y, id)).length > 1;

  for (const entries of [[], ["fund-group/capital"]]) {
    const name = entries.length ? "with " + entries.join(" > ") + " split" : "at the root";
    test(`${name}: a marked node reaches the tooltip and the panel, and links each caveat where Go linked it`, async (t) => {
      const { app, document } = await bootedApp({ checkedStem: YEARS[0].stem });
      const chart = document.getElementById("chart");
      await split(app, ...entries);
      // A mark one of whose caveats more than one year carries: the caveats
      // page lists an id once, so every year links it to one anchor, and the
      // drawer links what Go emitted. At the root a fund group carries the
      // caveat its own remainder node is named in: the remainder is folded
      // inside it, as a tail's members are inside the tail.
      //
      // Mutation: give each document's entry its own anchor in Go's
      // buildCaveatIndex and oneAnchor fails.
      const marked = marksIn(chart).find((m) => app.caveatsFor(m.__data__.id).some((c) => inSeveralYears(c.id)));
      assert.ok(marked, "no drawn node carries a caveat two years carry, so this asserts nothing");
      const laid = marked.__data__;
      app.showTip({ target: chart, clientX: 0, clientY: 0 }, laid);
      const tip = document.getElementById("tooltip").textContent;
      app.pin(laid);
      const panel = document.getElementById("detail").textContent;
      const ids = app.caveatsFor(laid.id).map((c) => c.id);
      const want = ids.map((id) => emittedHref(YEARS[0], id));
      const drawn = [...document.querySelectorAll("#detail a")]
        .filter((a) => a.textContent === "Read it in full").map((a) => a.getAttribute("href"));
      t.diagnostic(`${laid.id}: tooltip mentions ${tip.includes("caveat") ? "a caveat" : "NO caveat"}; ` +
        `drawer links ${JSON.stringify(drawn)}; ${ids.filter(inSeveralYears).join(", ")} in several years`);
      assert.ok(ids.every(oneAnchor), `one of ${ids.join(", ")} links to two anchors across the years`);
      assert.ok(tip.includes("caveat"), tip);
      assert.ok(panel.includes("Read it in full"), panel);
      assert.ok(want.every(Boolean), `Go emitted no href for one of ${ids.join(", ")}`);
      assert.deepEqual(drawn, want);
    });
  }

  for (const year of YEARS) {
    test(`${year.label}: a fund drawn apart from its own change says so at its pin, with its two printed balances and Go's change marked derived`, async (t) => {
      const { app, document } = await bootedApp({ checkedStem: year.stem });
      const chart = document.getElementById("chart");
      await split(app, "fund-group/capital");
      // The targets are read off the pinned column, not asked of caveatsFor.
      const col = columnFixture(year.fixture);
      const caveat = col.schedules.expansion.caveats.find((c) => c.id === "some-fund-group-money-is-not-by-fund");
      assert.ok(caveat, "the expansion carries no caveat about money no fund carries");
      const apart = new Set(caveat.applies_to.filter((id) => id.startsWith("fund/")));
      const mark = marksIn(chart).find((m) => apart.has(m.__data__.id));
      assert.ok(mark, "the capital split draws no fund the caveat names, so this asserts nothing");
      const node = mark.__data__;
      const b = node.balances;
      assert.ok(b && b.beginning && b.ending && b.change, `${node.id} carries no balances and change`);
      app.showTip({ target: chart, clientX: 0, clientY: 0 }, node);
      const tip = document.getElementById("tooltip").textContent;
      app.pin(node);
      const detail = document.getElementById("detail");
      const panel = detail.textContent;
      const words = CONFIG.wording;
      const said = (key, cents) => words[key].replace("{figure}", dollars(cents));
      const changeLine = [...detail.querySelectorAll("p")].find((p) => p.textContent.startsWith(said("balance_change", b.change.value_cents)));
      t.diagnostic(`${year.label} ${node.id}: ${apart.size} funds named; begins ${b.beginning.value_cents}, ends ${b.ending.value_cents}, ` +
        `Go's change ${b.change.value_cents}; tooltip ${tip.includes(caveat.summary) ? "says" : "does NOT say"} the caveat`);
      assert.ok(tip.includes(caveat.summary), tip);
      assert.ok(panel.includes(caveat.summary), panel);
      assert.ok(tip.includes(said("balance_beginning", b.beginning.value_cents)), tip);
      assert.ok(tip.includes(said("balance_change", b.change.value_cents)), tip);
      for (const key of ["beginning", "ending"]) {
        assert.ok(panel.includes(said("balance_" + key, b[key].value_cents)) && panel.includes(b[key].fact_id), panel);
      }
      assert.ok(changeLine, panel);
      assert.equal(changeLine.querySelector(".chip.derived")?.textContent, words.our_inference);
      assert.ok(changeLine.textContent.includes(b.change.rationale), changeLine.textContent);
      assert.ok(b.change.fact_ids.every((id) => changeLine.textContent.includes(id)), changeLine.textContent);
    });
  }

  for (const year of YEARS) {
    test(`${year.label}: a remainder Go derives is marked as ours, says why in Go's words, and reaches the tooltip and the panel; it splits nothing`, async (t) => {
      const { app, document } = await bootedApp({ checkedStem: year.stem });
      const chart = document.getElementById("chart");
      await split(app, "fund-group/capital");
      const mark = marksIn(chart).find((m) => m.__data__.id === "fund-group-only/capital");
      assert.ok(mark, "the capital group split draws no remainder of its own");
      const node = mark.__data__;
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

    test(`${year.label}: a tail's note counts the rows drawing it out draws`, async (t) => {
      const { app, document } = await bootedApp({ checkedStem: year.stem });
      const chart = document.getElementById("chart");
      const worst = worstOf(app);
      await split(app, worst);
      const tail = marksIn(chart).find((m) => app.expandable(m.__data__));
      assert.ok(tail, "no drawn mark offers to draw out");
      const tier = tail.__data__.tier;
      const word = CONFIG.overview.sankey.split_caps.find((c) => c.split === "funds").tail;
      // A group's derived remainder is one of its rows: the capped column
      // counts it, so the rows drawn are the tail note's total.
      const rows = () => marksIn(chart).filter((m) => m.__data__.tier === tier && !app.isAggregate(m.__data__.id) &&
        app.nodeOf(m.__data__.id).parent === worst).length;
      const cap = CONFIG.overview.sankey.split_caps.find((c) => c.split === "funds").cap;
      const note = tail.__data__.source_note;
      fire(tail, "keydown", { key: "Enter", timeStamp: 1000 });
      await settle();
      const drawn = rows();
      const stated = app.say("aggregate_note", { folded: drawn - cap, total: drawn, cap });
      t.diagnostic(`drawn out, ${worst} draws ${drawn} ${word}; the tail's note read "${note}"`);
      assert.ok(note.startsWith(stated), `${drawn} rows are drawn and the tail's note reads "${note}"`);
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
      app.drawOutTail(cappedTail.__data__);
      await settle();
      const drawnOut = marksCarrying();
      t.diagnostic(`a caveat naming ${foldedID}, which the cap folds, is carried by ${JSON.stringify(cappedMarks)} ` +
        `on the capped chart and by ${JSON.stringify(drawnOut)} drawn out`);
      // The split's node is drawn beside its members and carries every
      // caveat of what it holds.
      const hub = app.nodeOf(foldedID).parent;
      assert.deepEqual(cappedMarks, [cappedTail.__data__.id, hub].sort());
      assert.deepEqual(drawnOut, [foldedID, hub].sort());
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
  // Mutation: give the qualifier tspan of a middle-anchored label a dy of
  // -1.15em again and this goes red, naming the lifted marks.
  test("every qualified label is drawn on one line, an interior one included", async (t) => {
    // Two funds of one group under one name, so a pair the funds' column,
    // an interior one, qualifies is drawn: the columns no source's lines
    // stand in carry no shared label of their own.
    const col = structuredClone(columnFixture(newest.fixture));
    const named = (id) => col.nodes.find((n) => n.id === id);
    named("fund/640").label = named("fund/620").label;
    const { app, document } = await bootedApp({ checkedStem: newest.stem, plan: { [newest.path]: { doc: col } } });
    app.expandAll();
    await settle();
    const slots = app.slotsOf(app.laidNodes, app.RENDER_TIERS);
    const qualified = app.labelQualifiers({ nodes: [...app.groupIndex.values()] }, slots, app.laidNodes);
    const marks = [...document.querySelectorAll("#chart g.node")];
    const text = (m) => [...m.children].find((c) => c.tagName === "text");
    const interior = marks.filter((m) => qualified.has(m.__data__.id) && text(m).getAttribute("text-anchor") === "middle");
    const lifted = marks.filter((m) => [...text(m).children].some((s) => s.hasAttribute("dy") || s.hasAttribute("x")))
      .map((m) => m.__data__.id);
    const detail = `${qualified.size} qualified mark(s), ${interior.length} interior; lifted: ${JSON.stringify(lifted)}`;
    t.diagnostic(detail);
    assert.deepEqual({ interior: interior.length > 0, lifted }, { interior: true, lifted: [] }, detail);
  });
  // Mutation: have nameOf return d.label and this goes red, the tooltip,
  // the drawer and the description naming two same-word marks alike.
  test("a qualified mark is named by its drawn words in the tooltip, the drawer and its description, and so is a ribbon's end", async (t) => {
    const { app, document } = await bootedApp({ checkedStem: newest.stem });
    app.expandAll();
    await settle();
    const doc = { nodes: [...app.groupIndex.values()] };
    const slots = app.slotsOf(app.laidNodes, app.RENDER_TIERS);
    const qualified = app.labelQualifiers(doc, slots, app.laidNodes);
    const nouns = app.labelNouns(doc, app.laidNodes, app.memberNounOf);
    const named = (n) => (qualified.get(n.id) ? qualified.get(n.id) + ": " : "") + n.label + (nouns.has(n.id) ? " (" + nouns.get(n.id) + ")" : "");
    const marks = [...document.querySelectorAll("#chart g.node")];
    const twins = marks.filter((m) => qualified.get(m.__data__.id));
    const [a, b] = twins.filter((m) => twins.some((o) => o !== m && o.__data__.label === m.__data__.label));
    assert.ok(a && b, `no two qualified marks share a label among ${twins.length}`);
    const said = (m) => {
      app.hideTip();
      fire(m, "pointerenter");
      const tip = document.querySelector("#tooltip .tip-label").textContent;
      app.pin(m.__data__);
      const heading = document.querySelector("#detail .amount").nextElementSibling;
      const drawer = (heading && heading.tagName === "P" ? heading.nextElementSibling : heading).textContent;
      return { tip, drawer, described: m.getAttribute("aria-label").startsWith(named(m.__data__) + ", total ") };
    };
    const got = [a, b].map(said);
    const want = [a, b].map((m) => ({ tip: named(m.__data__), drawer: named(m.__data__), described: true }));
    const ribbon = [...document.querySelectorAll("#chart path.link")]
      .find((p) => p.__data__ && (qualified.get(p.__data__.source.id) || qualified.get(p.__data__.target.id)));
    assert.ok(ribbon, "no ribbon touches a qualified mark");
    const l = ribbon.__data__;
    const ends = named(l.source) + " \u2192 " + named(l.target);
    app.hideTip();
    fire(ribbon, "pointerenter");
    const ribbonTip = document.querySelector("#tooltip .tip-label").textContent;
    t.diagnostic(`${a.__data__.id}: ${JSON.stringify(got[0].tip)}; ${b.__data__.id}: ${JSON.stringify(got[1].tip)}; ribbon: ${JSON.stringify(ribbonTip)}`);
    assert.deepEqual({ got, ribbonTip, apart: got[0].tip !== got[1].tip }, { got: want, ribbonTip: ends, apart: true });
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
    glyphs: "the <g> the glyphs beside the marks are drawn into",
    "glyph-open": "the ⊕ beside a mark that splits, drawn as a plus by its path",
    "glyph-fold": "the ⊖ beside a split mark's hub, drawn as a minus by its path",
    opens: "the mark that splits; its affordance is its ⊕, not a style",
    expands: "the folded tail, likewise",
    "tip-drawn": "the tooltip's drawn-part line, worn as a tip-meta",
    label: "the hero's words, which paintYearWords repaints beside the .value #hero styles",
  };

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

  test("every class the client sets is styled or a declared hook, and every rule is worn on some page", async (t) => {
    const { app, document } = await bootedApp({ checkedStem: YEARS[0].stem });
    const chart = document.getElementById("chart");
    const worn = new Set();
    const snap = () => {
      for (const el of document.querySelectorAll("*")) for (const c of classesIn(el.getAttribute("class"))) worn.add(c);
    };
    // The root, the tooltip on a mark and on a ribbon, the panel on a
    // ribbon, a pin, an isolation, a node menu refusing a filter, a zoom,
    // every split the page offers, a tail drawn out, and a refusal.
    snap();
    const mark = marksIn(chart)[0];
    const ribbon = ribbonsIn(chart)[0];
    app.showTip({ target: chart, clientX: 0, clientY: 0 }, mark.__data__); snap();
    app.showTip({ target: chart, clientX: 0, clientY: 0 }, ribbon.__data__); snap();
    app.pin(ribbon.__data__); snap();
    app.pin(mark.__data__); snap();
    // A trace under the pointer, and the menu ⊕ opens on a node that splits two ways.
    fire(mark, "pointerenter"); snap(); fire(mark, "pointerleave");
    const chooser = [...chart.querySelectorAll("g.glyph")].find((g) => g.getAttribute("aria-haspopup") === "true");
    assert.ok(chooser, "no ⊕ offers a menu");
    fire(chooser, "click", { timeStamp: 0 }); snap(); app.closeChartMenu();
    app.setIsolated(mark.__data__.id); await settle(); snap();
    // A search listing its options, and a filter leaving part of a mark drawn.
    const search = document.getElementById("search-input");
    search.value = String(mark.__data__.label).slice(0, 3);
    search.dispatchEvent(new globalThis.Event("input", { bubbles: true })); snap();
    app.setFilter({ groups: new Set(["fund-group/general"]) });
    const part = marksIn(chart).find((m) => app.drawnAt(m.__data__));
    assert.ok(part, "no mark is drawn at less than its figure under the filter");
    app.showTip({ target: chart, clientX: 0, clientY: 0 }, part.__data__); app.pin(part.__data__); snap();
    app.resetFilters();
    // A node menu offering a filter the redraw would refuse, with why.
    app.expandNode("fund-group/general"); await settle();
    const refusing = app.laidNodes.find((n) => n.id === "expenditure/services-and-supplies");
    assert.ok(refusing, "Services & Supplies is not drawn with the General Fund open");
    app.openNodeMenu(refusing, { left: 0, top: 0 }); snap(); app.closeChartMenu();
    assert.ok(worn.has("chart-menu-why"), "no node menu said why a filter is not offered");
    app.collapseAll(); await settle();
    // A zoomed chart, for the class that clips it.
    app.zoomBy(2); snap(); app.resetZoom();
    let expandedAt = "";
    const walked = await everySplit(app, async (where) => {
      snap();
      const laidRibbon = ribbonsIn(chart)[0];
      if (laidRibbon) { app.showTip({ target: chart, clientX: 0, clientY: 0 }, laidRibbon.__data__); app.pin(laidRibbon.__data__); snap(); }
      // The first chart that folds a split is drawn out once, for its marks.
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
    const unworn = [...rules].filter((c) => !worn.has(c) && !templates.has(c)).sort();
    t.diagnostic(`${worn.size} classes worn across ${walked.visited} charts and the states above (drawn out at ${expandedAt}); ${rules.size} classes styled; ` +
      `${[...rules].filter((c) => !worn.has(c) && templates.has(c)).length} styled classes are other pages' templates'; ` +
      `hooks ${Object.keys(HOOKS).join(", ")}`);
    assert.deepEqual(unstyled, [], "set by the client and styled by no rule");
    assert.deepEqual(unworn, [], "styled and worn by nothing on any page");
    // A hook cannot go stale in silence: it is worn and unstyled.
    for (const c of Object.keys(HOOKS)) assert.ok(worn.has(c) && !rules.has(c), `${c} is no longer a hook`);
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
  test("a reworded config is followed by the counts line, the hint, the toolbar and the ⊕ menu", async (t) => {
    const config = structuredClone(pageFixture().config);
    for (const key of Object.keys(config.wording)) config.wording[key] = `«${key}» ` + config.wording[key];
    const { app, document } = await bootedApp({ config, checkedStem: YEARS[0].stem });
    const text = (id) => document.getElementById(id).textContent;
    const hintRoot = text("chart-hint");
    await split(app, "fund-group/general", "expenditure/services-and-supplies");
    const counts = text("counts-line");
    const hint = text("chart-hint");
    const bulk = ["chart-expand-all", "chart-collapse-all"].map((id) => document.getElementById(id).textContent);
    const chooser = [...document.querySelectorAll("#chart g.glyph")].find((g) => g.getAttribute("aria-haspopup") === "true");
    assert.ok(chooser, "no ⊕ offers a menu");
    fire(chooser, "click", { timeStamp: 0 });
    const menu = document.getElementById("split-menu");
    const named = menu ? menu.getAttribute("aria-label") : "";
    t.diagnostic(`counts "${counts}"; hint "${hint.slice(0, 80)}…"; toolbar ${JSON.stringify(bulk)}; menu "${named}"`);
    assert.ok(hintRoot.startsWith("«open_into» ") && hintRoot.includes(" «follow» ") && !hintRoot.includes("«fold_back»"), hintRoot);
    assert.ok(counts.startsWith("«counts_partial» "), counts);
    assert.match(counts, /\d+ (flow|flows) between \d+ (node|nodes)/);
    assert.ok(hint.includes(" «fold_back» "), hint);
    assert.deepEqual(bulk, ["«expand_all» Expand all", "«collapse_all» Collapse all"]);
    assert.ok(named.startsWith("«split_chooser» Split "), named);
  });
});

// Services and supplies' divisions and object rows each hold a derived
// remainder small enough to rank into the tail, in the year that draws one.
describe("a capped tail folds no derived remainder", () => {
  for (const year of YEARS) {
    test(`${year.label}: services and supplies' remainders are drawn beside their tails, not counted in them`, async (t) => {
      const { app } = await bootedApp({ checkedStem: year.stem });
      const node = "expenditure/services-and-supplies";
      const cases = [
        { entry: node, remainder: "expenditure/fund-group/internal-service/services-and-supplies" },
        { entry: node + app.ALTERNATIVE_MARK + "divisions", remainder: "no-division/services-and-supplies" },
      ];
      // FY 2025-26's schedule draws neither remainder; FY 2026-27's both.
      const held = cases.filter((c) => app.nodeOf(c.remainder) !== undefined);
      t.diagnostic(`${held.length} of ${cases.length} remainders in this year's schedule`);
      assert.equal(held.length, year.label === "FY 2026-27" ? 2 : 0);
      for (const c of held) {
        app.collapseAll();
        await split(app, c.entry);
        const tail = app.projection.nodes.find((n) => app.isAggregate(n.id) && n.parent === node);
        assert.ok(tail !== undefined, `${c.entry} draws no tail`);
        t.diagnostic(`${c.entry}: "${tail.label}" folds ${tail.folds.length}`);
        assert.ok(!tail.folds.includes(c.remainder), `${tail.id} folds ${c.remainder}`);
        assert.ok(tail.folds.every((m) => !app.nodeOf(m).derived), `${tail.id} folds a derived node`);
        assert.ok(app.projection.nodes.some((n) => n.id === c.remainder), `${c.remainder} is not drawn`);
      }
    });
  }
});

// AGENTS.md, "Go vets, JavaScript renders": a count the client takes is
// its sentence about what is drawn, marked as its own. Each sentence is read
// off the page where it is painted, after the state that paints it.
describe("a count the client takes is marked as its own", () => {
  const MARK = "\u25c7";
  const OLDEST = YEARS[0];
  const NEWEST = YEARS[YEARS.length - 1];

  test("every painted sentence carrying a count the client took carries the mark", async (t) => {
    // PLANTED: a caveat on a root mark, so its tooltip carries a caveat count.
    const col = structuredClone(columnFixture(OLDEST.fixture));
    col.schedules.expansion.caveats.push({ id: "a-planted-caveat", summary: "A planted caveat.", text: "", applies_to: ["fund-group/general"] });
    const { app, document } = await bootedApp({ checkedStem: OLDEST.stem, plan: { [OLDEST.path]: { doc: col } } });
    const chart = document.getElementById("chart");
    const text = (id) => document.getElementById(id).textContent;
    /** @type {Record<string, string>} */
    const said = {};
    said.counts = text("counts-line");
    said.hairline = text("hairline-line");
    const general = marksIn(chart).find((m) => m.__data__.id === "fund-group/general");
    assert.ok(general, "the root draws no fund-group/general");
    app.showTip({ target: chart, clientX: 0, clientY: 0 }, general.__data__);
    said.caveatChip = [...document.querySelectorAll("#tooltip .chip.caveat")].map((c) => c.textContent).join(" | ");
    const input = document.getElementById("search-input");
    input.value = "general";
    input.dispatchEvent(new globalThis.Event("input", { bubbles: true }));
    said.search = text("search-status");
    const worst = worstOf(app);
    await split(app, worst);
    app.expandAll();
    await settle();
    const range = document.getElementById("filter-smallest");
    range.value = "3";
    fire(range, "input");
    said.filter = text("filter-said");
    for (const [what, words] of Object.entries(said)) t.diagnostic(`${what}: "${words}"`);
    for (const [what, words] of Object.entries(said)) {
      assert.ok(words !== "", `nothing is painted for ${what}, so nothing is tested`);
      assert.ok(words.includes(MARK) || /\byour\b/i.test(words), `${what} counts without the mark: "${words}"`);
    }
  });

  test("the sentence a year switch says about the splits it folded back carries the mark", async (t) => {
    // PLANTED as the year-switch test plants it: the oldest year's capital
    // group renamed, so a split of it cannot be kept.
    const planted = structuredClone(columnFixture(OLDEST.fixture));
    const capital = planted.nodes.findIndex((n) => n.id === "fund-group/capital");
    planted.nodes[capital].id = "fund-group/capital-planted";
    for (const sched of Object.values(planted.schedules)) {
      for (const n of sched.nodes) if (n.parent === "fund-group/capital") n.parent = "fund-group/capital-planted";
    }
    const { app, document } = await loadApp({ plan: { [OLDEST.path]: { doc: planted } } });
    await app.boot();
    await settle();
    await split(app, "fund-group/capital");
    clickYear(document, OLDEST.stem);
    await settle();
    const line = document.getElementById("splits-line");
    t.diagnostic(`"${line.textContent}"`);
    assert.equal(line.hidden, false, "the switch folded nothing back, so nothing is tested");
    assert.ok(line.textContent.includes(MARK), line.textContent);
  });
});

// TWO EXTEND SPLITS DRAWN TOGETHER (fisc-yj4w.34.13): each opens its members
// a slot beside its parent's, so two open together can declare a slot further
// out than any drawn ribbon reaches. Every node is still drawn in its slot.
describe("two extend splits open together", () => {
  /** Each laid node whose d3 layer is not its slot's rank, by id. */
  const misplaced = (app) => {
    const slots = app.slotsOf(app.laidNodes, app.RENDER_TIERS);
    return app.laidNodes.filter((n) => n.layer !== slots.get(n.id)).map((n) => `${n.id} in ${n.layer}, slot ${slots.get(n.id)}`);
  };

  for (const year of YEARS) {
    test(`${year.label}: every pair of extend splits draws, each node in its slot's column`, async (t) => {
      const { app, document } = await bootedApp({ checkedStem: year.stem });
      const faults = [];
      const walked = await everyExtendPair(app, (where) => {
        for (const m of misplaced(app)) faults.push(`${where}: ${m}`);
        if (refusals(document).length) faults.push(`${where}: a refusal banner is up`);
      });
      t.diagnostic(`${year.label}: ${walked.visited} of ${walked.pairs} pair(s) drew; ${walked.refused.length} refused` +
        (walked.refused.length ? ":\n  " + walked.refused.join("\n  ") : ""));
      assert.ok(walked.pairs > 0, "the schedule declares no pair of extend splits, so nothing here is tested");
      assert.deepEqual(walked.refused, []);
      assert.equal(walked.visited, walked.pairs);
      assert.deepEqual(faults.slice(0, 5), [], `${faults.length} fault(s)`);
    });

    // The sequence the bead measured the refusal on.
    test(`${year.label}: the General Fund split by funds, fund/100 by divisions, then capital outlay by divisions draws`, async (t) => {
      const { app, document } = await bootedApp({ checkedStem: year.stem });
      await split(app, "fund-group/general", "fund/100" + app.ALTERNATIVE_MARK + "divisions",
        "expenditure/capital-outlay" + app.ALTERNATIVE_MARK + "divisions");
      const slots = app.slotsOf(app.laidNodes, app.RENDER_TIERS);
      const columns = new Set(slots.values()).size;
      t.diagnostic(`${year.label}: ${app.laidNodes.length} node(s) in ${columns} column(s)`);
      assert.equal(refusals(document).length, 0);
      assert.equal(columns, 6);
      assert.deepEqual(misplaced(app), []);
    });
  }
});

describe("a member printed in its parent's words", () => {
  for (const year of YEARS) {
    test(`${year.label}: is drawn with the page's noun for it in a qualifier tspan, and named with it`, async (t) => {
      const { app, document } = await bootedApp({ checkedStem: year.stem });
      app.expandAll();
      await settle();
      expandAll(app);
      await settle();
      const chart = document.getElementById("chart");
      const byID = new Map(app.fetched.nodes.map((n) => [n.id, n]));
      const want = app.fetched.nodes.filter((n) => app.labelledAsParent(byID, n.id)).map((n) => n.id).sort();
      const drawn = new Map(marksIn(chart).map((m) => [m.__data__.id, m]));
      const wrong = [];
      const said = [];
      for (const id of want) {
        const m = drawn.get(id);
        if (!m) {
          wrong.push(`${id} is not drawn with everything open`);
          continue;
        }
        const noun = haloOf(m).children[2];
        const words = " (" + app.memberNoun(app.fetched, id) + ")";
        const name = m.getAttribute("aria-label");
        if (noun.getAttribute("class") !== "qualifier" || noun.textContent !== words) {
          wrong.push(`${id} draws ${JSON.stringify(tspansOf(m))}, not the qualifier ${JSON.stringify(words)}`);
        }
        if (!name.startsWith(byID.get(id).label + words + ", ")) wrong.push(`${id} is named "${name}"`);
        said.push(tspansOf(m).slice(0, 3).join(""));
      }
      const stray = [...drawn].filter(([id, m]) => !want.includes(id) && haloOf(m).children[2].textContent !== "").map(([id]) => id);
      t.diagnostic(`${year.label}: ${JSON.stringify(said)}`);
      assert.ok(want.length > 0, "no member is printed in its parent's words, so this asserts nothing");
      assert.deepEqual(wrong, []);
      assert.deepEqual(stray, [], "a mark not printed in its parent's words draws a noun");
      assert.equal(app.memberNoun(app.fetched, "fund/100"), "fund 100");
      assert.equal(app.memberNoun(app.fetched, "revenue-line/intergovernmental/intergovernmental"), "line");
    });
  }
});

describe("a link repeating a leaf through a split is not called printed", () => {
  // A leaf drawn twice is one key in two drawn ribbons' cells: once
  // re-pointed onto the hub, once as the hub's leg. Counted here off the
  // cells, not asked of leafDraws.
  // Mutation: label by derived ends only, dropping drawsAgain from the
  // tooltip, the drawer and provenanceOf, and this goes red: the re-pointed
  // leaves and their hub legs read "printed".
  for (const year of YEARS) {
    test(`${year.label}: with the General Fund group open, no tooltip, drawer or description of a repeating link says printed`, async (t) => {
      const { app, document } = await bootedApp({ checkedStem: year.stem });
      const chart = document.getElementById("chart");
      await split(app, "fund-group/general");
      const words = CONFIG.wording;
      const links = app.projection.links;
      const leaves = (l) => Object.keys(l.cells || {}).filter((k) => !k.startsWith("\u001f"));
      const count = new Map();
      for (const l of links) for (const k of leaves(l)) count.set(k, (count.get(k) || 0) + 1);
      const again = (l) => leaves(l).some((k) => count.get(k) > 1);
      const wrong = [];
      let repeating = 0;
      for (const l of links) if (again(l)) repeating++;
      const ribbons = [...chart.querySelectorAll("path.link")].map((p) => p.__data__).filter(again);
      for (const d of ribbons.slice(0, 6)) {
        app.showTip({ target: chart, clientX: 0, clientY: 0 }, d);
        const tip = [...document.querySelectorAll("#tooltip .chip")].map((c) => c.textContent);
        if (tip.includes(words.printed_chip)) wrong.push(`tooltip: ${d.source.id} -> ${d.target.id}`);
        app.pin(d);
        const panel = [...document.querySelectorAll("#detail .chip")].map((c) => c.textContent);
        if (panel.includes(words.printed_by_city)) wrong.push(`drawer: ${d.source.id} -> ${d.target.id}`);
        if (app.linkDescription(d).endsWith(words.printed_by_city)) wrong.push(`description: ${d.source.id} -> ${d.target.id}`);
      }
      t.diagnostic(`${year.label}: ${repeating} link(s) repeat a leaf, ${ribbons.length} drawn ribbon(s); ${wrong.length} called printed`);
      assert.ok(repeating > 0 && ribbons.length > 0, "nothing drawn repeats a leaf, so this asserts nothing");
      assert.deepEqual(wrong, []);
    });
  }
});

// On a narrow screen the chart gives way to a note pointing to the bars of
// the same money: the template ships the note's button hidden, app.js shows
// it, and a press marks the root data-chart-anyway, which unfolds the card
// under style.css's narrow query. The note's link carries the year on
// screen as the top bar's do. Mutation: drop wireNarrow from boot and the
// button stays hidden; narrow carryYear back to the nav and the link keeps
// no year.
test("the narrow screen's note shows its button, which puts the chart back, and its link carries the year", async () => {
  const { app, document } = await bootedApp();
  const button = document.getElementById("narrow-show");
  const link = document.querySelector("#narrow-note a[data-years]");
  assert.ok(button && !button.hidden, "the note's button is missing or hidden");
  assert.ok(link, "the note does not link a view that offers years");
  assert.match(link.getAttribute("href"), new RegExp("#y=" + document.getElementById("year-select").value + "$"));
  assert.ok(!document.documentElement.hasAttribute("data-chart-anyway"));
  fire(button, "click");
  await settle();
  assert.ok(document.documentElement.hasAttribute("data-chart-anyway"), "the press did not unfold the card");
  assert.ok(app.chartDrawn(), "no chart is drawn after the press");
});
