// drill.test.mjs — the drill's WALK: that every node the chart offers opens,
// that every rung Go answers draws the columns and the ids it answers, and the
// shape, flank, words and carried document of each window the shipped steps
// declare, opened through the real gesture path over the pinned artifacts.
//
// What it deliberately does not test: gesture sequences (Enter, Space, double
// click, Escape, focus restoration), which have their own suite; anything a
// figure of the answer or of a document could be re-derived from -- a cent sum,
// a balance, a residual set, a gap -- which is Go's to vet and `fisc verify`'s
// to hold; and any sentence that exists only in app.js. Words asserted here are
// read from CONFIG (a step's description, back and projection) or from the
// document on screen (a node's label, a ribbon's contra note).

import { describe, test } from "node:test";
import assert from "node:assert/strict";
import {
  bootedApp, opened, settle, expandAll, everyOffer, rungsFixture, pageFixture,
  columnFixture, clickYear, refusals, topOf,
} from "./testlib.mjs";

const PAGE = pageFixture().config;
/** The published years, newest last as the page lists them. */
const YEARS = PAGE.years;
const RUNGS = rungsFixture();

/** The step the pinned config declares under `key`. */
function stepByKey(config, key) {
  const s = config.steps.find((x) => x.key === key);
  if (!s) throw new Error("the pinned config declares no step " + key);
  return s;
}

/** Go's answer for one rung of one year. */
function answerFor(stem, path) {
  const col = RUNGS.columns.find((c) => c.stem === stem);
  if (!col) throw new Error("rungs.json answers no column " + stem);
  const r = col.rungs.find((x) => x.path.join("") === path.join(""));
  if (!r) throw new Error(`rungs.json answers no rung ${path.join(" > ")} of ${stem}`);
  return r;
}

/** The year's own `steps[]` entry (caveats and `opens`) for a step of the config. */
function yearStep(year, config, key) {
  return year.steps[config.steps.findIndex((s) => s.key === key)];
}

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

/**
 * Opens a path whose members may be inside a folded tail: a node the chart
 * does not draw is reached by expanding every column first, the way a reader
 * does, and then through the real entry point.
 */
async function openThrough(app, ...path) {
  let expanded = 0;
  for (const id of path) {
    if (!app.projection.nodes.some((n) => n.id === id)) {
      expandAll(app);
      expanded++;
    }
    await opened(app, id);
  }
  return expanded;
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

/** The columns as d3 placed them, left to right, read back as tiers. */
function placedTiers(app) {
  const laid = app.layOut(app.projection);
  return [...new Set(laid.nodes.slice().sort((a, b) => a.x0 - b.x0).map((n) => n.tier))];
}

/** The ids drawn at one tier of the chart on screen, in document order. */
function atTier(app, tier) {
  return app.projection.nodes.filter((n) => n.tier === tier).map((n) => n.id);
}

/** Whether the chart on screen offers `id` to open. */
function offers(app, id) {
  const n = app.projection.nodes.find((x) => x.id === id);
  return Boolean(n) && app.drillable(n);
}

/** The ids the answer says one rung draws at the columns this viewport lays out. */
function answeredIDs(answer, tiers, app) {
  const ids = new Set(answer.draws.filter((d) => tiers.includes(d.tier))
    .flatMap((d) => d.ids.concat(d.carried || []))
    .concat((answer.marks || []).map((m) => m.id)));
  // A RESIDUAL'S LEAVING ENDPOINTS ARE NAMED ON THE MARK AND NOT IN A COLUMN:
  // Go answers a column's ids off the documents, and an endpoint the spine
  // draws at tier 5 is no document's node at the step's last tier. Whether
  // the client draws them at these columns is the shipped rule's answer.
  const step = stepByKey(PAGE, answer.step);
  const answered = new Set(answer.draws.flatMap((d) => d.ids.concat(d.carried || [])));
  for (const m of answer.marks || []) {
    if (m.role !== "residual") continue;
    for (const e of m.ends || []) {
      if (!answered.has(e) && app.leavingLegDrawn(step, tiers)) ids.add(e);
    }
  }
  return ids;
}

/** The table pointer: everything after the served description's first sentence. */
function pointerOf(served) {
  return served.slice(served.indexOf(". ") + 2);
}

for (const year of YEARS) {
  describe(`${year.label}: every rung, opened through the real gesture path`, () => {
    test(`${year.label}: every node the tree offers to open draws when opened, at every depth`, async (t) => {
      const { app, fetch } = await onYear(year.stem);
      const asked = fetch.asked.length;
      const short = [];
      let smallest = Infinity;
      const walk = await everyOffer(app, (where) => {
        const has = new Set(app.projection.nodes.map((n) => n.tier));
        const missing = app.activeTiers().filter((tier) => !has.has(tier));
        if (missing.length) short.push(`${where.join(" > ")} draws no tier ${missing.join(", ")}`);
        for (const l of app.layOut(app.projection).links) smallest = Math.min(smallest, l.width);
      });
      t.diagnostic(`${year.label}: ${walk.visited} views opened; smallest ribbon over all of them ` +
        `${smallest.toFixed(3)}px; ${fetch.asked.length - asked} fetch(es) added by the walk`);
      assert.equal(walk.refused, "");
      assert.ok(walk.visited > 0, "the chart offered nothing to open");
      assert.deepEqual(short, []);
      assert.equal(fetch.asked.length, asked, "a drill fetched");
    });

    test(`${year.label}: every rung the answer names opens, draws the columns it answers and the ids it answers, and no other`, async (t) => {
      const { app } = await onYear(year.stem);
      const answers = RUNGS.columns.find((c) => c.stem === year.stem).rungs;
      assert.ok(answers.length > 0, "rungs.json answers nothing for " + year.stem);
      let throughTail = 0;
      let narrowed = 0;
      let marks = 0;
      const wrong = [];
      for (const r of answers) {
        app.drillUp(0);
        await settle();
        // A RUNG THAT WILL NOT OPEN IS RECORDED AND THE WALK GOES ON, so the
        // failure names every rung that came out wrong rather than the first.
        try {
          if (await openThrough(app, ...r.path)) throughTail++;
        } catch (e) {
          wrong.push(`${r.path.join(" > ")}: ${e.message}`);
          continue;
        }
        const tiers = app.activeTiers();
        const answered = r.draws.filter((d) => tiers.includes(d.tier));
        if (answered.length < r.draws.length) narrowed++;
        marks += (r.marks || []).length;
        const want = answeredIDs(r, tiers, app);
        // FOLDED, THE CHART DRAWS A SUBSET AND THE TAIL; EXPANDED, THE SET.
        const folded = app.projection.nodes.map((n) => n.id);
        const strays = folded.filter((id) => !want.has(id) && !app.isAggregate(id));
        expandAll(app);
        const drawn = app.projection.nodes.map((n) => n.id);
        const columns = new Set(app.projection.nodes.map((n) => n.tier)).size;
        const missing = [...want].filter((id) => !drawn.includes(id));
        const extra = drawn.filter((id) => !want.has(id));
        if (strays.length || missing.length || extra.length || columns !== answered.length) {
          wrong.push(`${r.path.join(" > ")}: ${columns} column(s) for ${answered.length} answered; ` +
            (strays.length ? `drawn folded but not answered ${JSON.stringify(strays)}; ` : "") +
            (missing.length ? `answered but not drawn ${JSON.stringify(missing)}; ` : "") +
            (extra.length ? `drawn but not answered ${JSON.stringify(extra)}` : ""));
        }
      }
      app.drillUp(0);
      t.diagnostic(`${year.label}: ${answers.length} rungs answered, ${throughTail} reached through a ` +
        `folded tail, ${narrowed} narrowed by the column budget of ${app.columnBudget}, ` +
        `${marks} mark(s) of Go's beside them`);
      assert.deepEqual(wrong, []);
    });
  });

  describe(`${year.label}: the transfers window`, () => {
    test(`${year.label} transfers: Transfers In opens into p76 and draws its receiving legs`, async (t) => {
      const { app, config, fetch } = await onYear(year.stem);
      const step = stepByKey(config, "transfers");
      const asked = fetch.asked.length;
      await opened(app, "transfers/in");
      const answer = answerFor(year.stem, ["transfers/in"]);
      const drawn = app.projection.nodes.map((n) => n.id).sort();
      const want = [...answeredIDs(answer, app.activeTiers(), app)].sort();
      t.diagnostic(`${year.label} transfers: ${app.projection.nodes.length} nodes, ` +
        `${app.projection.links.length} links in columns ${JSON.stringify(placedTiers(app))}`);
      assert.equal(app.drilled.length, 1);
      assert.equal(app.projection.projection, step.projection);
      assert.equal(fetch.asked.length, asked, "the drill fetched");
      assert.deepEqual(placedTiers(app), step.tiers);
      assert.deepEqual(drawn, want);
      assert.ok(app.projection.links.length > 0, "no receiving leg drawn");
      assert.ok(!app.projection.nodes.some((n) => app.drillable(n)), "something on it opens further");
    });
  });
}

for (const year of YEARS) {
  describe(`${year.label}: the department windows`, () => {
    test(`${year.label} departments: the fund column carries two steps, told apart by role alone`, (t) => {
      const step = stepByKey(PAGE, "fund-departments");
      const fundStep = stepByKey(PAGE, "fund");
      t.diagnostic(`both open tier ${step.from} of ${JSON.stringify(step.after)}; roles ` +
        `"${fundStep.role}" and "${step.role}"; this one draws ${JSON.stringify(step.tiers)} keeping ` +
        `${JSON.stringify(step.keep)} of ${step.projection} with ${(step.caps || []).length} cap(s)`);
      assert.deepEqual(step.after, fundStep.after);
      assert.equal(step.from, fundStep.from);
      assert.equal(step.role, "fund");
      assert.equal(fundStep.role, "general_fund");
      assert.deepEqual(step.tiers, [2, 3, 4]);
      assert.deepEqual(step.keep, [2]);
      assert.equal(step.projection, "department-funding");
      assert.equal((step.caps || []).length, 0);
    });

    test(`${year.label} departments: a fund the group's window draws opens into its own funding rows, with the group kept beside it`, async (t) => {
      const { app, document, config } = await onYear(year.stem);
      const step = stepByKey(config, "fund-departments");
      const groupStep = stepByKey(config, "fund-group");
      await opened(app, "fund-group/special-revenue");
      const fund = app.projection.nodes
        .filter((n) => n.tier === 3 && app.drillable(n)).map((n) => n.id).sort()[0];
      assert.ok(fund, "the group's window draws no fund that opens");
      await opened(app, fund);
      const answer = answerFor(year.stem, ["fund-group/special-revenue", fund]);
      const ribbons = app.projection.links.filter((l) => l.source === fund);
      const departments = answer.draws.find((d) => d.tier === 4).ids;
      const at = words(app, document);
      t.diagnostic(`${year.label} departments: opened ${fund} at depth ${app.drilled.length} into ` +
        `${app.projection.projection}, columns ${JSON.stringify(placedTiers(app))}; ` +
        `${ribbons.length} department ribbon(s) against ` +
        `${app.projection.links.filter((l) => l.target === fund).length} kept from the chart above`);
      assert.equal(app.drilled.length, 2);
      assert.equal(app.projection.projection, step.projection);
      assert.deepEqual(placedTiers(app), step.tiers);
      assert.ok(ribbons.length > 0);
      assert.deepEqual(ribbons.map((l) => l.target).sort(), departments.slice().sort());
      assert.deepEqual(at.crumbControls, [app.say("back_control", { back: groupStep.back }), app.say("back_control", { back: step.back })]);
    });

    test(`${year.label} departments: a fund the cap folds away is reachable by expanding the column, and opens into every department it pays`, async (t) => {
      const { app, config } = await onYear(year.stem);
      const step = stepByKey(config, "fund-departments");
      await opened(app, "fund-group/special-revenue");
      const foldedBefore = app.projection.nodes.some((n) => n.id === "fund/240");
      const tail = app.projection.nodes.find((n) => app.isAggregate(n.id));
      assert.ok(tail, "the group's fund column is not folded");
      app.expandTier(tail);
      await settle();
      const drawnAfter = app.projection.nodes.find((n) => n.id === "fund/240");
      const opensAfter = Boolean(drawnAfter) && app.drillable(drawnAfter);
      await opened(app, "fund/240");
      const answer = answerFor(year.stem, ["fund-group/special-revenue", "fund/240"]);
      const ribbons = app.projection.links.filter((l) => l.source === "fund/240");
      t.diagnostic(`${year.label} departments: fund/240 is ${foldedBefore ? "DRAWN" : "inside the tail"} ` +
        `on the capped column; opened, it draws ${ribbons.length} department ribbon(s) ` +
        `${JSON.stringify(ribbons.map((l) => l.target))}`);
      assert.equal(foldedBefore, false);
      assert.ok(drawnAfter, "fund/240 is still not drawn once the column is expanded");
      assert.equal(opensAfter, true);
      assert.equal(app.drilled.length, 2);
      assert.equal(app.projection.projection, step.projection);
      assert.deepEqual(ribbons.map((l) => l.target).sort(),
        answer.draws.find((d) => d.tier === 4).ids.slice().sort());
    });
  });

  describe(`${year.label}: the category windows`, () => {
    const PROPERTY = "revenue/taxes/property";

    test(`${year.label} category: every revenue category opens from the spine, and of the two tier-0 flow ends only transfers/in does`, async (t) => {
      const { app } = await onYear(year.stem);
      const categories = app.projection.nodes
        .filter((n) => n.tier === 0 && n.role === "revenue_source").map((n) => n.id);
      const opensAt0 = {
        categories: categories.filter((id) => offers(app, id)).length,
        transfersIn: offers(app, "transfers/in"),
        draw: offers(app, "fund-balance/draw"),
        general: offers(app, "fund-group/general"),
      };
      await opened(app, "fund-group/general");
      const underGroup = offers(app, PROPERTY);
      app.drillUp(0);
      t.diagnostic(`${year.label} category: ${opensAt0.categories} of ${categories.length} categories open`);
      assert.ok(categories.length > 0);
      assert.equal(opensAt0.categories, categories.length);
      assert.equal(opensAt0.transfersIn, true, "transfers/in does not open");
      assert.equal(opensAt0.draw, false, "fund-balance/draw opens");
      assert.equal(opensAt0.general, true);
      assert.equal(underGroup, false, "under an opened group the category opens");
    });

    test(`${year.label} category: Property Taxes opens into a window whose centre it is, and every sentence says so`, async (t) => {
      const { app, document, fetch, config } = await onYear(year.stem);
      const step = stepByKey(config, "revenue-category");
      const served = words(app, document).desc;
      const pointer = pointerOf(served);
      const label = app.projection.nodes.find((n) => n.id === PROPERTY).label;
      const groups = columnFixture(year.path.replace(/\.json$/, "")).fund_groups.length;
      const asked = fetch.asked.length;
      await opened(app, PROPERTY);
      const at = words(app, document);
      const answer = answerFor(year.stem, [PROPERTY]);
      const flank = atTier(app, 2);
      t.diagnostic(`${year.label} category: ${app.projection.nodes.length} nodes, ` +
        `${app.projection.links.length} links in columns ${JSON.stringify(placedTiers(app))}, ` +
        `the middle one ${JSON.stringify(atTier(app, 0))} beside ${flank.length} kept fund group(s); ` +
        `title "${at.title}"; breadcrumb ${JSON.stringify(at.crumbControls)} + "${at.crumbHere}"`);
      assert.equal(at.depth, 1);
      assert.equal(at.drawnIsYears, false);
      assert.deepEqual(placedTiers(app), step.tiers);
      assert.deepEqual(atTier(app, 0), [PROPERTY]);
      assert.deepEqual(flank.slice().sort(), answer.draws.find((d) => d.tier === 2).ids.slice().sort());
      assert.equal(fetch.asked.length, asked);
      assert.ok(at.title.startsWith(year.chart_title), at.title);
      assert.ok(at.title.endsWith(label), at.title);
      assert.deepEqual(at.crumbControls, [app.say("back_control", { back: step.back })]);
      assert.ok(at.crumbHere.includes(label), at.crumbHere);
      assert.equal(at.legend, 0);
      assert.ok(!app.projection.nodes.some((n) => app.drillable(n)), "something opens further");
      assert.ok(at.desc.includes(label), at.desc);
      assert.ok(at.desc.includes(step.description), "the description lacks the step's own");
      assert.ok(at.desc.endsWith(pointer), "the table pointer is dropped");
      app.drillUp(0);
      await settle();
      const back = words(app, document);
      assert.equal(back.depth, 0);
      assert.equal(back.legend, groups);
      assert.equal(back.crumbHidden, true);
      assert.equal(back.drawnIsYears, true);
      assert.equal(back.desc, served);
    });

    test(`${year.label} category: the two contra rows run into the centre, in the schedule's own words`, async (t) => {
      const { app, document } = await onYear(year.stem);
      await opened(app, PROPERTY);
      const contra = app.projection.links.filter((l) => l.contra);
      const laid = app.layOut(app.projection);
      const eraf = laid.links.find((l) => l.source.id === "revenue-line/taxes/property/eraf");
      const rows = [...document.querySelectorAll("#flow-table tbody tr")];
      const erafRow = rows.find((tr) => tr.className === "contra" &&
        tr.children[0].textContent === laid.nodes.find((n) => n.id === eraf.source.id).label);
      t.diagnostic(`${year.label} category: ${contra.length} contra ribbon(s): ` +
        contra.map((l) => `${l.source.split("/").pop()} into ${l.target} (${l.contra})`).join(", "));
      assert.equal(contra.length, 2);
      for (const l of contra) {
        assert.equal(l.target, PROPERTY);
        assert.ok(l.value_cents > 0, "a contra ribbon is drawn at a negative width");
        assert.equal(typeof l.contra, "string");
        assert.ok(l.contra.length > 0);
      }
      assert.ok(eraf, "ERAF is not drawn as a ribbon");
      assert.equal(app.linkClass(eraf), "link contra");
      assert.ok(erafRow, "the flow table has no contra row for ERAF");
      assert.equal(erafRow.children[4].textContent, eraf.contra);
    });
  });

  describe(`${year.label}: the object-category windows`, () => {
    const ENDS = ["transfers/out", "fund-balance/contribution", "fund-balance/reserve-increase"];

    test(`${year.label} object: the spine's four object categories open and its three flow ends do not`, async (t) => {
      const { app, config } = await onYear(year.stem);
      const ids = yearStep(year, config, "object-category").opens;
      const at5 = atTier(app, 5);
      t.diagnostic(`${year.label} object: the spine draws ${at5.length} node(s) in its right-hand column; ` +
        `${ids.filter((id) => offers(app, id)).length} of ${ids.length} object categories open`);
      assert.equal(ids.length, 4);
      assert.equal(at5.length, ids.length + ENDS.length);
      for (const id of ids) assert.equal(offers(app, id), true, id + " does not open");
      for (const id of ENDS) assert.equal(offers(app, id), false, id + " opens");
    });

    test(`${year.label} object: all four categories draw as three columns -- the groups that fund it, the category, the divisions that spend it`, async (t) => {
      const { app, config } = await onYear(year.stem);
      const step = stepByKey(config, "object-category");
      const ids = yearStep(year, config, "object-category").opens;
      const shapes = [];
      for (const id of ids) {
        app.drillUp(0);
        await settle();
        await opened(app, id);
        const d = app.projection;
        shapes.push(`${id}: ${d.nodes.length} nodes, ${d.links.length} links, ` +
          `${atTier(app, 2).length} funding group(s), tail "${(d.nodes.find((n) => app.isAggregate(n.id)) || {}).label || ""}"`);
        assert.deepEqual(placedTiers(app), step.tiers, id);
        assert.deepEqual(atTier(app, 5), [id]);
        assert.deepEqual(d.nodes.filter((n) => n.tier === 4 && n.parent).map((n) => n.id), [],
          id + " draws a division carrying a fund group");
      }
      t.diagnostic(`${year.label} object: ${shapes.join("; ")}`);
    });
  });
}

describe("a step's `after`", () => {
  test("a step opens from every chart its `after` names, and from no other -- membership, so one view can be reached from several", async (t) => {
    const read = async (after) => {
      const config = structuredClone(PAGE);
      const i = config.steps.findIndex((s) => s.key === "fund");
      config.steps[i].after = after;
      const { app } = await bootedApp({ config });
      await opened(app, "fund-group/general");
      return { after, steps: app.STEPS.length, opens: offers(app, "fund/100") };
    };
    const [shipped, member, stranger] = [
      await read(["fund-group"]), await read(["nope", "fund-group"]), await read(["nope"]),
    ];
    t.diagnostic([shipped, member, stranger].map((r) =>
      `${JSON.stringify(r.after)}: ${r.steps} step(s) read, fund/100 ${r.opens ? "opens" : "does not open"}`)
      .join("; "));
    for (const r of [shipped, member, stranger]) assert.equal(r.steps, PAGE.steps.length);
    assert.equal(shipped.opens, true);
    assert.equal(member.opens, true);
    assert.equal(stranger.opens, false);
  });
});

describe("the window: three columns spliced on the node the reader clicked", () => {
  const CENTRE = "fund-group/general";

  test("a window is three columns spliced on the node the reader clicked, its centre alone in the middle", async (t) => {
    const { app, config } = await bootedApp();
    const step = stepByKey(config, "fund-group");
    const spine = app.docAt(0);
    const kept = spine.links.filter((l) => l.target === CENTRE);
    await opened(app, CENTRE);
    const answer = answerFor(app.shownYear.stem, [CENTRE]);
    const drawn = app.projection;
    const tiers = [...new Set(drawn.nodes.map((n) => n.tier))].sort((a, b) => a - b);
    const fromKept = drawn.links.filter((l) => l.target === CENTRE);
    const toFunds = drawn.links.filter((l) => l.source === CENTRE);
    const residualEnds = drawn.links.filter((l) => app.isResidual(l.target)).map((l) => l.source);
    t.diagnostic(`${app.shownYear.label}: tiers ${JSON.stringify(tiers)} drawn left to right as ` +
      `${JSON.stringify(placedTiers(app))}; kept column ${atTier(app, 0).length} node(s) sending ` +
      `${fromKept.length} ribbon(s) into the centre and ${residualEnds.length} past it; opened column ` +
      `${atTier(app, 3).length} node(s) taking ${toFunds.length} from it`);
    // AT THE BUDGET EVERY READER GETS, which does not buy the step's widening.
    const own = step.tiers.filter((tier) => !(step.widen || []).includes(tier));
    assert.deepEqual(tiers, own.slice().sort((a, b) => a - b));
    assert.deepEqual(placedTiers(app), own);
    assert.deepEqual(atTier(app, 2), [CENTRE]);
    assert.equal(atTier(app, 0).length, kept.length);
    assert.equal(fromKept.length, kept.length - residualEnds.length);
    assert.equal(toFunds.length, answer.draws.find((d) => d.tier === 3).ids.length);
    assert.equal(atTier(app, 3).length, toFunds.length + (answer.marks || []).length);
  });

  test("the kept flank and the centre come off the chart on screen, in its words and at its figures", async (t) => {
    const { app } = await bootedApp();
    const spine = app.docAt(0);
    const kept = spine.links.filter((l) => l.target === CENTRE);
    await opened(app, CENTRE);
    const drawn = app.projection;
    const labels = new Map(drawn.nodes.map((n) => [n.id, n.label]));
    const spineLabels = new Map(spine.nodes.map((n) => [n.id, n.label]));
    const stepDoc = app.scheduleOf(app.column, stepByKey(app.CONFIG, "fund-group").projection);
    const stepLabels = new Map(stepDoc.nodes.map((n) => [n.id, n.label]));
    const named = atTier(app, 0).concat([CENTRE]);
    const tellsApart = named.filter((id) => stepLabels.has(id) && spineLabels.get(id) !== stepLabels.get(id));
    const notInStep = named.filter((id) => !stepLabels.has(id));
    t.diagnostic(`${app.shownYear.label}: centre drawn as "${labels.get(CENTRE)}" against the step ` +
      `document's "${stepLabels.get(CENTRE)}"; ${tellsApart.length} of ${named.length} drawn marks are ` +
      `named differently by the two documents, and ${notInStep.length} the step document does not carry`);
    for (const id of named) assert.equal(labels.get(id), spineLabels.get(id), id);
    for (const l of kept) {
      const drew = drawn.links.find((d) => d.source === l.source &&
        (d.target === CENTRE || app.isResidual(d.target)));
      assert.ok(drew, l.source + " is not drawn into the window");
      assert.equal(drew.value_cents, l.value_cents, l.source);
    }
  });

  test("a window's kept flank survives being drilled through and popped back to", async (t) => {
    const { app, config } = await bootedApp();
    await opened(app, CENTRE);
    const shape = (d) => JSON.stringify(d.nodes.map((n) => n.id + "@" + n.tier)) +
      JSON.stringify(d.links.map((l) => l.source + ">" + l.target + "=" + l.value_cents));
    const before = shape(app.projection);
    await opened(app, "fund/100");
    const deepTiers = [...new Set(app.projection.nodes.map((n) => n.tier))].sort((a, b) => a - b);
    app.drillUp(1);
    await settle();
    t.diagnostic(`${app.shownYear.label}: fund/100 opened at tiers ${JSON.stringify(deepTiers)}; ` +
      `popping back left ${app.drilled.length} rung`);
    assert.deepEqual(deepTiers, app.activeTiers(app.columnBudget).length
      ? stepByKey(config, "fund").tiers.filter((x) => deepTiers.includes(x)) : deepTiers);
    assert.equal(app.drilled.length, 1);
    assert.equal(shape(app.projection), before);
  });

  test("a carried mark's document is found by the stem it records, at the depth that document is on the stack", async (t) => {
    const { app, config } = await bootedApp();
    const spineStem = app.shownYear.stem;
    const stepStem = stepByKey(config, "fund-group").projection;
    const caveat = app.shownYear.caveats[0].id;
    await opened(app, CENTRE, "fund/100");
    const depths = {
      spine: app.depthOfDocument("sankey"),
      step: app.depthOfDocument(stepStem),
      absent: app.depthOfDocument("no-such-document"),
    };
    const carried = app.projection.nodes.filter((n) => n.carried_from).map((n) => n.carried_from);
    t.diagnostic(`${app.shownYear.label}: two rungs deep the stack answers sankey at depth ${depths.spine}, ` +
      `${stepStem} at ${depths.step} and an unknown stem at ${depths.absent}; the ${carried.length} ` +
      `carried mark(s) here record ${JSON.stringify([...new Set(carried)])}; year stem ${spineStem}`);
    assert.equal(app.drilled.length, 2);
    assert.equal(depths.spine, 0);
    assert.equal(depths.step, 2);
    assert.equal(depths.absent, -1);
    assert.ok(carried.length > 0, "nothing on the fund's window is carried");
    for (const s of carried) assert.equal(s, stepStem);
    assert.equal(app.carriedSource("sankey").projection, "sankey");
    assert.equal(app.carriedSource(stepStem).projection, stepStem);
    assert.equal(app.carriedSource("no-such-document"), null);
    assert.equal(app.caveatHref(caveat, "no-such-document"), "");
    assert.equal(app.caveatHref(caveat, "sankey"), app.shownYear.caveats[0].href);
  });
});

for (const year of YEARS) {
  describe(`${year.label}: the chain`, () => {
    const CHAIN = ["fund-group/general", "fund/100", "dept/patrol"];

    test(`${year.label} chain: the drill adds no fetch, and draws the schedule the step names`, async (t) => {
      const { app, fetch, config } = await onYear(year.stem);
      const asked0 = fetch.asked.slice();
      await opened(app, ...CHAIN);
      const asked1 = fetch.asked.slice();
      app.drillUp(0);
      await settle();
      await opened(app, ...CHAIN);
      t.diagnostic(`${year.label} chain: main() asked for ${JSON.stringify(asked0)}; the drills added ` +
        `${JSON.stringify(fetch.asked.slice(asked0.length))} and drew ` +
        `"${app.docAt(1).projection}" at depth 1`);
      assert.equal(asked0[0], config.rungs);
      assert.ok(asked0.includes(year.path));
      assert.deepEqual(asked1, asked0);
      assert.deepEqual(fetch.asked, asked0);
      assert.equal(app.docAt(1).projection, stepByKey(config, "fund-group").projection);
    });

    test(`${year.label} chain: depth 1 draws the General Fund at {0,2,3} as a window at the budget every reader gets, and every sentence says so`, async (t) => {
      const { app, document, config } = await onYear(year.stem);
      const step = stepByKey(config, "fund-group");
      const served = words(app, document).desc;
      const label = app.projection.nodes.find((n) => n.id === CHAIN[0]).label;
      await opened(app, CHAIN[0]);
      const at = words(app, document);
      const opens = app.projection.nodes.filter((n) => app.drillable(n)).map((n) => n.id);
      const muted = app.projection.nodes.filter((n) => n.tier === 3 && !app.isCarried(n.id) &&
        !app.isResidual(n.id) && !app.fundGroupOf(n)).map((n) => n.id);
      t.diagnostic(`${year.label} chain: ${app.projection.nodes.length} nodes, ${app.projection.links.length} ` +
        `links; counts "${at.counts}"; title "${at.title}"; breadcrumb ${JSON.stringify(at.crumbControls)} + ` +
        `"${at.crumbHere}"; ${JSON.stringify(opens)} open`);
      assert.equal(at.depth, 1);
      assert.equal(at.drawnIsYears, false);
      // THE STEP DECLARES A FOURTH COLUMN AND THE DEFAULT BUDGET DOES NOT BUY
      // IT: the placed tiers are the step's less its widening.
      assert.deepEqual(placedTiers(app), step.tiers.filter((tier) => !step.widen.includes(tier)));
      assert.deepEqual(placedTiers(app), [0, 2, 3]);
      assert.deepEqual(atTier(app, 4), []);
      assert.ok(at.title.startsWith(year.chart_title) && at.title.endsWith(label), at.title);
      assert.deepEqual(at.crumbControls, [app.say("back_control", { back: step.back })]);
      assert.ok(at.crumbHere.includes(label), at.crumbHere);
      assert.equal(at.legend, 0);
      assert.ok(at.desc.includes(label) && at.desc.includes(step.description), at.desc);
      assert.ok(at.desc.endsWith(pointerOf(served)), "the table pointer is dropped");
      assert.deepEqual(opens, ["fund/100"]);
      assert.deepEqual(muted, []);
    });

    test(`${year.label} chain: depth 2 draws fund/100 at {2,3,4}, whose two sides are different quantities and whose description says which`, async (t) => {
      const { app, document, fetch, config } = await onYear(year.stem);
      const groupStep = stepByKey(config, "fund-group");
      const fundStep = stepByKey(config, "fund");
      await opened(app, CHAIN[0]);
      const asked = fetch.asked.length;
      const label = app.projection.nodes.find((n) => n.id === CHAIN[1]).label;
      await opened(app, CHAIN[1]);
      const at = words(app, document);
      const divisions = atTier(app, 4);
      const folded = divisions.filter((id) => app.isAggregate(id));
      t.diagnostic(`${year.label} chain: ${app.projection.nodes.length} nodes, ${app.projection.links.length} ` +
        `links; ${divisions.length} divisions drawn, ${folded.length} folded; counts "${at.counts}"`);
      assert.equal(at.depth, 2);
      assert.equal(fetch.asked.length, asked);
      assert.deepEqual(placedTiers(app), fundStep.tiers.filter((x) => app.activeTiers().includes(x)));
      assert.deepEqual(placedTiers(app), [2, 3, 4]);
      assert.deepEqual(folded, []);
      assert.equal(offers(app, CHAIN[2]), true, "a division does not open");
      assert.deepEqual(at.crumbControls, [app.say("back_control", { back: groupStep.back }), app.say("back_control", { back: fundStep.back })]);
      assert.ok(at.desc.includes(label) && at.desc.includes(fundStep.description), at.desc);
      assert.ok(fundStep.description.includes("what it takes in is its revenue"));
      assert.ok(fundStep.description.includes("less the money the group takes in that no fund receives"));
    });

    test(`${year.label} chain: depth 3 draws Patrol at {3,4,5}, names all three rungs, keeps the table pointer, and opens nothing further`, async (t) => {
      const { app, document, fetch, config } = await onYear(year.stem);
      const steps = ["fund-group", "fund", "division"].map((k) => stepByKey(config, k));
      const served = words(app, document).desc;
      const asked = fetch.asked.length;
      const labels = [];
      for (const id of CHAIN) {
        labels.push(app.projection.nodes.find((n) => n.id === id).label);
        await opened(app, id);
      }
      const at = words(app, document);
      t.diagnostic(`${year.label} chain: ${app.projection.nodes.length} nodes, ${app.projection.links.length} ` +
        `links; title "${at.title}"; breadcrumb ${JSON.stringify(at.crumbControls)} + "${at.crumbHere}"`);
      assert.equal(at.depth, 3);
      assert.ok(app.projection.links.length > 0);
      assert.equal(fetch.asked.length, asked);
      assert.deepEqual(placedTiers(app), steps[2].tiers);
      for (const l of labels) assert.ok(at.title.includes(l), `the title lacks ${l}: ${at.title}`);
      assert.ok(at.title.endsWith(labels[2]), at.title);
      assert.deepEqual(at.crumbControls, steps.map((s) => app.say("back_control", { back: s.back })));
      assert.ok(at.crumbHere.includes(labels[2]), at.crumbHere);
      assert.ok(at.desc.includes(steps[2].description), at.desc);
      assert.ok(at.desc.endsWith(pointerOf(served)), "the table pointer is dropped");
      assert.ok(!app.projection.nodes.some((n) => app.drillable(n)), "something still opens");
      assert.equal(at.legend, 0);
    });

    test(`${year.label} chain: a non-General group's funds open exactly where pp.85-125 print a row for them, and the rest are drawn as ends`, async (t) => {
      const { app, document, config } = await onYear(year.stem);
      const groupStep = stepByKey(config, "fund-group");
      const decomposed = yearStep(year, config, "fund-departments").opens;
      await opened(app, "fund-group/capital");
      const at = words(app, document);
      const capped = atTier(app, 3).filter((id) => id.startsWith("fund/")).length;
      // THE WHOLE COLUMN, DRAWN OUT: the cap folds the smallest funds into a
      // tail, and the split is a claim about every fund of the group.
      expandAll(app);
      const funds = atTier(app, 3).filter((id) => id.startsWith("fund/"));
      const opens = funds.filter((id) => offers(app, id)).sort();
      const shut = funds.filter((id) => !offers(app, id)).sort();
      const answered = answerFor(year.stem, ["fund-group/capital"]).draws.find((d) => d.tier === 3).ids;
      t.diagnostic(`${year.label} chain: opened into capital: ${capped} fund mark(s) drawn under the cap, ` +
        `${funds.length} drawn out, ${opens.length} open and ${shut.length} drawn as ends -- ${JSON.stringify(shut)}`);
      assert.equal(at.depth, 1);
      assert.ok(capped <= funds.length);
      assert.deepEqual(funds.slice().sort(), answered.slice().sort());
      assert.ok(opens.length > 0 && shut.length > 0, "the split is not a split");
      assert.deepEqual(opens, funds.filter((id) => decomposed.includes(id)).sort());
      assert.deepEqual(shut, funds.filter((id) => !decomposed.includes(id)).sort());
      assert.ok(at.desc.includes(groupStep.description), at.desc);
    });
  });
}

describe("the refusal a drill can still meet", () => {
  test("the drill refuses a column carrying no schedule the step names, in words that name it", async (t) => {
    const newest = YEARS[YEARS.length - 1];
    const schedule = stepByKey(PAGE, "fund-group").projection;
    const column = structuredClone(columnFixture(newest.path.replace(/\.json$/, "")));
    assert.ok(Object.hasOwn(column.schedules, schedule));
    delete column.schedules[schedule];
    const { app, document, fetch } = await bootedApp({ plan: { [newest.path]: { doc: column } } });
    assert.equal(app.shownYear.stem, newest.stem);
    const before = words(app, document);
    assert.equal(refusals(document).length, 0);
    const asked = fetch.asked.length;
    const outcome = await app.drillDown("fund-group/general");
    await settle();
    const after = words(app, document);
    const banners = refusals(document).map((b) => b.textContent);
    t.diagnostic(`drillDown came to "${outcome}" with ${app.drilled.length} rung(s); ${banners.length} ` +
      `banner(s)${banners.length ? `, reading "${banners[0].slice(0, 90)}..."` : ""}`);
    assert.equal(outcome, "failed");
    assert.equal(app.drilled.length, 0);
    assert.equal(topOf(app), "");
    assert.equal(banners.length, 1);
    assert.ok(banners[0].includes(schedule), banners[0]);
    assert.equal(after.counts, before.counts);
    assert.equal(after.title, before.title);
    assert.equal(after.crumbHidden, true);
    assert.equal(after.drawnIsYears, true);
    assert.equal(fetch.asked.length, asked);
  });

  // THE GESTURES ASK drillable FIRST, so a reader meets these two only when
  // the chart changed under the gesture; the page's own callers meet them by
  // name. Each is a FAILED that used to say nothing (fisc-51qf).
  for (const [what, id, names] of [
    ["an id the chart does not draw", "fund/999", "fund/999"],
    ["a drawn mark no step opens", "fund-balance/draw", "Fund Balance Draw"],
  ]) {
    test(`the drill refuses ${what}, in words that name it, and leaves the chart alone`, async (t) => {
      const { app, document } = await bootedApp({});
      const drawn = app.projection.nodes.some((n) => n.id === id);
      assert.equal(drawn, id !== "fund/999");
      if (drawn) assert.equal(app.drillable(app.projection.nodes.find((n) => n.id === id)), false);
      const before = words(app, document);
      assert.equal(refusals(document).length, 0);
      const outcome = await app.drillDown(id);
      await settle();
      const banners = refusals(document).map((b) => b.textContent);
      t.diagnostic(`drillDown(${id}) came to "${outcome}" with ${banners.length} banner(s)` +
        `${banners.length ? `, reading "${banners[0]}"` : ""}`);
      assert.equal(outcome, "failed");
      assert.equal(app.drilled.length, 0);
      assert.equal(topOf(app), "");
      assert.equal(banners.length, 1);
      assert.ok(banners[0].includes(names), banners[0]);
      assert.deepEqual(words(app, document), before);
      // AND THE BANNER DOES NOT OUTLIVE THE NEXT OPEN THAT DRAWS.
      await opened(app, "fund-group/general");
      assert.equal(refusals(document).length, 0);
    });
  }
});
