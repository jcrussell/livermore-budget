// gestures.test.mjs -- the SEQUENCES a reader's gestures make on the drawn
// chart: which key opens and which follows the money, an isolation surviving
// the two clicks under a double click, Escape closing one rung at a time and
// each breadcrumb control closing to its own depth, an expansion staying on
// the chart it was made on, a kept flank that nothing opens, and where focus
// lands after a keyboard drill has replaced the element it was on.
//
// Every gesture is a dispatched event with a planted timeStamp, because the
// activation guard compares a click's stamp against the key that may have
// synthesised it.

import { describe, test } from "node:test";
import assert from "node:assert/strict";
import {
  bootedApp, opened, settle, fire, topOf, pageFixture, clickYear,
} from "./testlib.mjs";

const PAGE = pageFixture().config;
/** The published years, newest last as the page lists them. */
const YEARS = PAGE.years;

/** The step the pinned config declares under `key`. */
function stepByKey(config, key) {
  const s = config.steps.find((x) => x.key === key);
  if (!s) throw new Error("the pinned config declares no step " + key);
  return s;
}

/**
 * The tier the fund window folds its object categories at, read off the step
 * the packager ships rather than spelled here.
 */
const CATEGORY_TIER = stepByKey(PAGE, "fund").sankey.caps.find((c) => c.tail === "object rows").tier;

/**
 * A mark that opens on the spine; the rung whose marks open into nothing,
 * since every mark the spine draws opens; and two of that rung's marks.
 */
const OPENS = "fund-group/general";
const SHUT = "transfers/in";
const CLOSED = "fund/100";
const SEED = "fund/610";
/** The group whose window draws a folded tail, and the division that opens nothing. */
const WORST = "fund-group/special-revenue";
const INERT = "dept/patrol";

/**
 * A page booted and switched to `stem`. The page opens on the newest year, so
 * every other year is reached the way a reader reaches it, through the control.
 */
async function onYear(stem, o = {}) {
  const loaded = await bootedApp(o);
  if (loaded.app.shownYear.stem !== stem) {
    clickYear(loaded.document, stem);
    await settle();
  }
  assert.equal(loaded.app.shownYear.stem, stem);
  return loaded;
}

/** The drawn mark for one node id, or a throw naming the chart it is not on. */
function markOf(app, document, id) {
  const m = [...document.querySelectorAll("#chart g.node")]
    .find((g) => g.__data__ && g.__data__.id === id);
  if (!m) throw new Error(`${id} is not drawn on ${topOf(app) || "the overview"}`);
  return m;
}

/** The breadcrumb's return controls, outermost first. */
function backControls(document) {
  return [...document.querySelectorAll("#breadcrumb button.crumb-back")];
}

/** The breadcrumb's expansion chips. */
function chips(document) {
  return [...document.querySelectorAll("#breadcrumb button.crumb-expanded")];
}

/** Escape, dispatched on the document the way a keyboard delivers it. */
function escape(document) {
  fire(document, "keydown", { key: "Escape" });
}

/**
 * Opens `id` the way a keyboard reader does: focus lands on the mark, then
 * Enter. Focusing pins the mark, which is what a reader's focus does too.
 */
async function keyOpen(app, document, id, at) {
  const m = markOf(app, document, id);
  m.focus();
  fire(m, "keydown", { key: "Enter", timeStamp: at });
  await settle();
  assert.equal(topOf(app), id, `Enter on ${id} left the chart on ${topOf(app) || "the overview"}`);
}

/** The page's words about the chart on screen, read off the real DOM. */
function words(app, document) {
  const text = (id) => {
    const e = document.getElementById(id);
    return e ? String(e.textContent).replace(/\s+/g, " ").trim() : "";
  };
  const crumb = document.getElementById("breadcrumb");
  const kids = crumb ? [...crumb.children] : [];
  const legend = document.getElementById("legend");
  return {
    counts: text("counts-line"),
    title: text("chart-title"),
    desc: text("chart-desc"),
    hint: text("chart-hint"),
    legend: legend ? legend.children.length : -1,
    crumbControls: kids.filter((c) => c.tagName === "BUTTON" && c.className === "crumb-back")
      .map((c) => c.textContent),
    crumbHere: kids.filter((c) => c.tagName === "SPAN").map((c) => c.textContent).join(""),
    crumbHidden: crumb ? crumb.hasAttribute("hidden") : true,
    depth: app.drilled.length,
    drawnIsYears: app.drawnDoc() === app.fetched,
  };
}

/** The whole of a drawn chart as two sorted lists, so two charts can be compared mark for mark. */
function wholeOf(doc) {
  return {
    nodes: doc.nodes.map((n) => n.id).sort().join(","),
    links: doc.links.map((l) => `${l.source}>${l.target}:${l.kind}=${l.value_cents}`).sort().join(","),
  };
}

/** How many folded tails the chart on screen draws. */
function tails(app) {
  return app.projection.nodes.filter((n) => app.isAggregate(n.id)).length;
}

describe("the two keys", () => {
  test("Enter opens and Space isolates, and neither does the other", async (t) => {
    const { app, document } = await bootedApp();
    const state = () => ({ depth: app.drilled.length, top: topOf(app), isolated: app.isolated });

    fire(markOf(app, document, OPENS), "keydown", { key: "Enter", timeStamp: 1000 });
    await settle();
    const enter = state();
    app.drillUp(0);
    await settle();
    fire(markOf(app, document, OPENS), "keydown", { key: " ", timeStamp: 5000 });
    await settle();
    const space = state();
    app.drillUp(0);
    await settle();
    await opened(app, SHUT);
    const shut = app.projection.nodes.find((n) => n.id === CLOSED);
    // Enter still activates a mark with nothing to open: a role="button"
    // whose Enter does nothing is worse than one whose two keys agree.
    fire(markOf(app, document, CLOSED), "keydown", { key: "Enter", timeStamp: 9000 });
    await settle();
    const closedEnter = state();

    assert.deepEqual(enter, { depth: 1, top: OPENS, isolated: "" });
    assert.deepEqual(space, { depth: 0, top: "", isolated: OPENS });
    assert.equal(app.drillable(shut), false, `${CLOSED} opens on ${SHUT}'s rung`);
    assert.deepEqual(closedEnter, { depth: 1, top: SHUT, isolated: CLOSED });
    t.diagnostic(`Enter on ${OPENS} opened it; Space on it followed it; Enter on ${CLOSED} under ${SHUT} followed it`);
  });

  test("a double click opens, and leaves the isolation the reader had", async (t) => {
    // THE SEED IS THE WHOLE TEST. A double click delivers click, click,
    // dblclick, and on a chart following nothing those two clicks toggle an
    // isolation on and off and land back on "" without any help. It starts
    // from a mark the reader was already following, the only state in which
    // the two answers differ.
    const seed = SEED;
    const { app, document } = await bootedApp();
    await opened(app, SHUT);
    fire(markOf(app, document, seed), "click", { timeStamp: 0 });
    const seeded = app.isolated;
    fire(markOf(app, document, CLOSED), "click", { timeStamp: 1000 });
    const firstClick = app.isolated;
    fire(markOf(app, document, CLOSED), "click", { timeStamp: 1050 });
    const secondClick = app.isolated;
    fire(markOf(app, document, CLOSED), "dblclick", { timeStamp: 1060 });
    await settle();
    const closedDouble = { depth: app.drilled.length, isolated: app.isolated };

    // A SECOND PAGE AND NOT THE SAME ONE WOUND BACK: seeding the same mark
    // again on a chart still following it clears the seed.
    const second = await bootedApp();
    const m2 = (id) => markOf(second.app, second.document, id);
    const spineSeed = "revenue/taxes/sales";
    fire(m2(spineSeed), "click", { timeStamp: 0 });
    const reseeded = second.app.isolated;
    fire(m2(OPENS), "click", { timeStamp: 1000 });
    fire(m2(OPENS), "click", { timeStamp: 1050 });
    fire(m2(OPENS), "dblclick", { timeStamp: 1060 });
    await settle();
    const opensDouble = { depth: second.app.drilled.length, top: topOf(second.app), isolated: second.app.isolated };

    assert.equal(seeded, seed);
    assert.equal(firstClick, CLOSED);
    assert.equal(secondClick, "");
    assert.deepEqual(closedDouble, { depth: 1, isolated: seed });
    assert.equal(reseeded, spineSeed);
    assert.deepEqual(opensDouble, { depth: 1, top: OPENS, isolated: "" });
    t.diagnostic(`following "${seed}", a double click on ${CLOSED} went "${firstClick}" then ` +
      `"${secondClick}" under the reader and came back to "${closedDouble.isolated}"; the same ` +
      `gesture on ${OPENS}, following "${spineSeed}", opened it, and the redraw cleared the isolation on the way`);
  });
});

for (const year of YEARS) {
  describe(`${year.label}: the folded tail`, () => {
    test(`${year.label}: Enter on the folded tail draws its column uncapped, and Space still follows its money`, async (t) => {
      const { app, document } = await onYear(year.stem);
      await opened(app, WORST);
      const tail = app.projection.nodes.find((n) => app.isAggregate(n.id));
      assert.ok(tail, `${WORST} drew no folded tail`);
      const offered = app.projection.nodes.filter((n) => app.expandable(n)).map((n) => n.id);
      assert.deepEqual(offered, [tail.id], "the tail is the one mark the chart offers to expand");
      const capped = app.projection.links.length;

      // SPACE GOES FIRST AND MUST NOT EXPAND: it is the key that follows the
      // money on every other mark.
      fire(markOf(app, document, tail.id), "keydown", { key: " ", timeStamp: 0 });
      await settle();
      const spaceFollowed = app.isolated;
      const spaceLeft = tails(app);
      fire(markOf(app, document, tail.id), "keydown", { key: "Enter", timeStamp: app.ACTIVATION_WINDOW * 10 });
      await settle();
      const left = tails(app);
      const expanded = app.projection.links.length;
      // THE TWO MARKS THIS CHART WOULD REFUSE, asked of the predicate directly
      // because no committed document draws either: a tail at a tier this
      // rung declares no cap for, and the tier already expanded.
      const group = stepByKey(PAGE, "fund-group");
      const uncapped = group.sankey.tiers.find((tier) => !group.sankey.caps.some((c) => c.tier === tier));
      const foreign = app.expandable({ id: app.aggregateID(uncapped), tier: uncapped });
      const twice = app.expandable({ id: tail.id, tier: tail.tier });

      assert.equal(spaceFollowed, tail.id, "Space follows the tail's money");
      assert.equal(spaceLeft, 1, "Space leaves the tail drawn");
      assert.equal(app.drilled.length, 1, "expanding opens no rung");
      assert.equal(left, 0, "Enter leaves no tail drawn");
      assert.equal(chips(document).length, 1, "the breadcrumb carries one chip for the expansion");
      assert.equal(foreign, false);
      assert.equal(twice, false);
      t.diagnostic(`${WORST} capped lays ${capped} ribbon(s); Enter on "${tail.label}" expanded it to ${expanded}`);
    });

    test(`${year.label}: an expansion is a property of the chart it was made on, not of the page`, async (t) => {
      const { app, document } = await onYear(year.stem);
      const fundStep = stepByKey(PAGE, "fund");
      app.setColumnBudget(4);
      await opened(app, OPENS, "fund/100");
      const narrow = app.projection.links.length;
      const categoryTail = app.projection.nodes
        .find((n) => app.isAggregate(n.id) && n.tier === CATEGORY_TIER);
      assert.ok(categoryTail, "the fund window at four columns draws a folded category tail");
      fire(markOf(app, document, categoryTail.id), "dblclick", { timeStamp: 0 });
      await settle();
      const wide = app.projection.links.length;
      const wideWhole = wholeOf(app.projection);
      const categories = app.projection.nodes.filter((n) => n.tier === CATEGORY_TIER).length;
      assert.equal(chips(document).length, 1, "expanded, the breadcrumb carries one chip");
      assert.equal(tails(app), 0);

      // THE RUNG BELOW IT IS A FRESH ONE: opening a division from an expanded
      // chart must not carry "draw every category" into a chart whose
      // categories are a different column of a different step. What the new
      // rung RECORDED is asserted too, because a kept flank is a filter of it.
      await keyOpen(app, document, INERT, 1000);
      const inner = { chip: chips(document).length, depth: app.drilled.length, tails: tails(app),
        recorded: wholeOf(app.drilled[2].chart) };
      assert.deepEqual(inner, { chip: 0, depth: 3, tails: 0, recorded: wideWhole });

      // AND THE WAY BACK LANDS ON THE EXPANDED CHART, with focus on the return
      // control rather than the chip.
      assert.ok(app.focusInChart(), "a keyboard drill leaves focus in the chart");
      escape(document);
      await settle();
      const back = app.projection.links.length;
      const controls = backControls(document);
      assert.equal(app.drilled.length, 2);
      assert.equal(back, wide, "Escape comes back to the expanded chart");
      // ON THE MARK THAT OPENED THE RUNG JUST CLOSED, not on the chip and not
      // on a return control, and without pinning it.
      assert.ok(controls.length > 0);
      assert.equal(document.activeElement.__data__ ? document.activeElement.__data__.id : "", INERT,
        `focus is on ${document.activeElement.tagName}.${document.activeElement.getAttribute("class")}`);
      assert.ok(app.pinned === null, "restoring focus pinned the mark");

      // AND A RUNG OPENED AFRESH IS CAPPED AFRESH.
      escape(document);
      await settle();
      assert.equal(app.drilled.length, 1);
      await opened(app, "fund/100");
      assert.equal(app.projection.links.length, narrow, "reopening the fund draws it capped again");
      t.diagnostic(`the fund window at four columns lays ${narrow} ribbon(s) capped, ${wide} with its ` +
        `${categories} categories drawn out; opened from there, ${INERT} recorded that expanded chart`);
    });
  });
}

describe("the kept flank", () => {
  test("nothing on a kept flank opens, and a residual's declared endpoint does not either", async (t) => {
    const { app, document } = await bootedApp();
    await opened(app, OPENS);
    const drawn = app.projection;
    const category = drawn.nodes.find((n) => n.tier === 0 && n.role === "revenue_source");
    const endpoint = drawn.nodes.find((n) => n.carried_from && app.isCarried(n.id));
    const fund = drawn.nodes.find((n) => n.id === "fund/100");
    assert.ok(category, "the window keeps a revenue category on its flank");
    assert.ok(endpoint, "the window draws a carried endpoint");
    assert.ok(fund, "the window draws fund/100");

    // NO STEP NAMES EITHER on the shipped config; the carried gate is the
    // next test's.
    assert.equal(category.carried_from, "sankey");
    assert.equal(app.isCarried(category.id), false);
    assert.equal(app.stepFor(category), null);
    assert.equal(app.drillable(category), false);
    assert.equal(app.isCarried(endpoint.id), true);
    assert.equal(app.stepFor(endpoint), null);
    assert.equal(app.drillable(endpoint), false);
    assert.equal(Boolean(fund.carried_from), false);
    assert.equal(app.drillable(fund), true);

    // AND THE GESTURES AGREE WITH THE PREDICATES: a double click and Enter on
    // either refused mark open nothing, and on the fund the double click opens.
    let at = 1000;
    for (const id of [category.id, endpoint.id]) {
      fire(markOf(app, document, id), "dblclick", { timeStamp: (at += 1000) });
      await settle();
      fire(markOf(app, document, id), "keydown", { key: "Enter", timeStamp: (at += 1000) });
      await settle();
      assert.equal(topOf(app), OPENS, `a gesture on ${id} opened something`);
    }
    fire(markOf(app, document, fund.id), "dblclick", { timeStamp: (at += 1000) });
    await settle();
    assert.equal(topOf(app), fund.id);
    assert.equal(app.drilled.length, 2);
    t.diagnostic(`${category.id} is kept from "${category.carried_from}" and no step names its chart; ` +
      `${endpoint.id} is carried; the one mark that opens is ${fund.id}`);
  });
});

describe("a carried mark under a step that would open it", () => {
  test("a residual stays closed under a step at its tier with neither role nor flank: no document carries it, and it is carried", async (t) => {
    // The fund step stripped to the most permissive declaration the packager
    // could ship, no role and no kept flank: only the decomposition rule and
    // the carried gate then stand between the residual beside fund/100 and
    // the step at its tier.
    const config = structuredClone(PAGE);
    const at = config.steps.findIndex((s) => s.key === "fund");
    delete config.steps[at].role;
    delete config.steps[at].sankey.keep;
    const { app, document } = await bootedApp({ config });
    await opened(app, OPENS);
    const residual = app.projection.nodes.find((n) => app.isResidual(n.id));
    assert.ok(residual, `${OPENS} draws no residual`);
    const step = config.steps[at];
    assert.equal(residual.tier, step.from, "the residual does not stand at the fund step's tier");
    // TWO GATES, EACH REFUSING ON ITS OWN. The mark is a node of no document,
    // so the step decomposes it into nothing and no step answers for it; and
    // it is carried, so drillable would refuse it under a step that did.
    const decomposes = app.stepDecomposes(step, residual.id);
    const answered = app.stepFor(residual);
    t.diagnostic(`${residual.id} at tier ${residual.tier}: the ${step.key} step decomposes it ${decomposes}, ` +
      `stepFor answers ${answered ? answered.key : "nothing"}, carried ${app.isCarried(residual.id)}, drillable ${app.drillable(residual)}`);
    assert.equal(decomposes, false, "a step decomposes a mark no document carries");
    assert.ok(answered === null, "a step answers for the residual");
    assert.equal(app.isCarried(residual.id), true);
    assert.equal(app.drillable(residual), false);
    fire(markOf(app, document, residual.id), "dblclick", { timeStamp: 1000 });
    await settle();
    assert.equal(topOf(app), OPENS);
  });
});

for (const year of YEARS) {
  describe(`${year.label}: the way back`, () => {
    test(`${year.label} transfers: Escape leaves the rung and the spine is drawn whole again`, async (t) => {
      const { app, document } = await onYear(year.stem);
      const at0 = words(app, document);
      await keyOpen(app, document, "transfers/in", 1000);
      assert.equal(app.drilled.length, 1);
      // THE ONLY ROUTE OFF A RUNG THAT OPENS NOTHING.
      escape(document);
      await settle();
      const back0 = words(app, document);
      assert.equal(back0.depth, 0);
      assert.equal(back0.counts, at0.counts);
      assert.equal(back0.crumbHidden, true, "the breadcrumb is hidden on the overview");
      t.diagnostic(`Escape from transfers/in came back to depth ${back0.depth} reading "${back0.counts}"`);
    });

    test(`${year.label} chain: Escape closes one rung at a time, and each depth comes back as it was`, async (t) => {
      const { app, document, fetch } = await onYear(year.stem);
      const fundStep = stepByKey(PAGE, "fund");
      const at0 = words(app, document);
      await keyOpen(app, document, OPENS, 1000);
      const at1 = words(app, document);
      const asked1 = fetch.asked.length;
      await keyOpen(app, document, "fund/100", 2000);
      const at2 = words(app, document);
      await keyOpen(app, document, INERT, 3000);
      assert.equal(app.drilled.length, 3);

      // ESCAPE, ONE RUNG AT A TIME, through the handler main() attached: the
      // route whose "innermost first" rule is the claim.
      escape(document);
      await settle();
      const back2 = words(app, document);
      const controls2 = backControls(document);
      const focusBack2 = document.activeElement;
      escape(document);
      await settle();
      const back1 = words(app, document);
      escape(document);
      await settle();
      const back0 = words(app, document);

      assert.equal(back2.depth, 2);
      assert.equal(back2.counts, at2.counts);
      assert.equal(back2.title, at2.title);
      // ON THE MARK THAT OPENED THE RUNG JUST CLOSED, which is drawn again
      // on the chart returned to; the return control would undo a different
      // rung.
      assert.ok(controls2.length > 0);
      assert.equal(focusBack2 && focusBack2.__data__ ? focusBack2.__data__.id : "", INERT,
        `focus is on ${focusBack2 ? focusBack2.tagName + "." + focusBack2.getAttribute("class") : "nothing"}`);
      assert.equal(back1.depth, 1);
      assert.deepEqual(
        { counts: back1.counts, title: back1.title, controls: back1.crumbControls, hint: back1.hint, desc: back1.desc },
        { counts: at1.counts, title: at1.title, controls: at1.crumbControls, hint: at1.hint, desc: at1.desc });
      assert.equal(back0.depth, 0);
      assert.deepEqual(
        { counts: back0.counts, title: back0.title, hidden: back0.crumbHidden, legend: back0.legend,
          hint: back0.hint, desc: back0.desc, drawnIsYears: back0.drawnIsYears },
        { counts: at0.counts, title: at0.title, hidden: true, legend: at0.legend,
          hint: at0.hint, desc: at0.desc, drawnIsYears: true });
      assert.equal(fetch.asked.length, asked1, "the way down and back added no fetch");
      t.diagnostic(`after one Escape: depth ${back2.depth}, counts "${back2.counts}", focus on ` +
        `"${focusBack2.textContent}"; after two: depth ${back1.depth}; after three: depth ${back0.depth}, ` +
        `legend ${back0.legend}, breadcrumb hidden`);
    });

    test(`${year.label} chain: each breadcrumb control closes to its own depth`, async (t) => {
      const { app, document } = await onYear(year.stem);
      const at0 = words(app, document);
      await opened(app, OPENS, "fund/100");
      const at2 = words(app, document);
      await opened(app, INERT);
      // ONE CONTROL PER RUNG, EACH SAYING WHAT THE CHART IT CLOSES TO IS, in
      // the step's own `back`.
      const controls = backControls(document);
      assert.equal(controls.length, 3);
      controls.forEach((c, k) => assert.ok(c.textContent.endsWith(app.drilled[k].step.back),
        `control ${k} reads "${c.textContent}"`));
      // FROM DEPTH 3 THE INNERMOST CONTROL LANDS ON DEPTH 2 AND THE OUTERMOST
      // ON THE OVERVIEW: a bar whose every control went to the overview would
      // pass the Escape test unnoticed.
      fire(controls[2], "click");
      await settle();
      const inner = words(app, document);
      assert.equal(inner.depth, 2);
      assert.equal(inner.counts, at2.counts);
      await opened(app, INERT);
      fire(backControls(document)[0], "click");
      await settle();
      const back0 = words(app, document);
      assert.equal(back0.depth, 0);
      assert.equal(back0.counts, at0.counts);
      assert.equal(back0.crumbHidden, true);
      t.diagnostic(`the innermost control left depth ${inner.depth} reading "${inner.counts}"; ` +
        `the outermost left depth ${back0.depth} reading "${back0.counts}"`);
    });

    test(`${year.label}: closing the only rung by keyboard puts focus back on the mark that opened it`, async (t) => {
      const { app, document } = await onYear(year.stem);
      const first = document.querySelector("#chart g.node").__data__.id;
      assert.notEqual(first, OPENS, "the mark opened is the first in document order, so the test cannot tell the two apart");
      await keyOpen(app, document, OPENS, 1000);
      assert.equal(app.drilled.length, 1);
      escape(document);
      await settle();
      const active = document.activeElement;
      const on = active && active.__data__ ? active.__data__.id : (active ? active.tagName : "nothing");
      t.diagnostic(`after Escape focus is on ${on}; the first mark in document order is ${first}`);
      assert.equal(app.drilled.length, 0);
      assert.equal(on, OPENS);
      // AND IT IS NOT PINNED: the reader did not tab onto it.
      assert.ok(app.pinned === null, "restoring focus pinned the mark");
    });

    test(`${year.label} chain: a keyboard drill lands focus on the rung's return control, at every depth`, async (t) => {
      const { app, document } = await onYear(year.stem);
      const landed = [];
      for (const [k, id] of [OPENS, "fund/100", INERT].entries()) {
        await keyOpen(app, document, id, 1000 * (k + 1));
        const controls = backControls(document);
        const active = document.activeElement;
        assert.equal(active, controls[controls.length - 1],
          `after opening ${id} focus is on ${active ? active.tagName + "." + active.getAttribute("class") : "nothing"}`);
        assert.ok(active.textContent.endsWith(app.drilled[k].step.back));
        landed.push(active.textContent);
      }
      t.diagnostic(`focus landed on ${JSON.stringify(landed)}`);
    });
  });
}
