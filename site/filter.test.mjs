// The filter panel and state: hidden ribbons are removed before layout and
// stated as a client sum over cited cents; smallest-flow merges keep every
// cent, fact and page. Over the pinned columns, through the shipped app.js.

import { describe, test } from "node:test";
import assert from "node:assert/strict";

import { bootedApp, expansionStates, shapedAt, split, settle, clickYear, fire, pageFixture, refusals, dollars } from "./testlib.mjs";
import * as core from "./core.js";

const CONFIG = pageFixture().config;
const YEARS = CONFIG.years;

/** A ribbon's figure as the page states it: a reduction negative. */
const stated = (app, l) => app.formOnScreen().cents(l);

const sum = (app, links) => links.reduce((s, l) => s + stated(app, l), 0);

const factsOf = (links) => {
  const out = new Set();
  for (const l of links) for (const id of l.fact_ids) out.add(id);
  return out;
};

/** None; each group alone, all but the first, all; each kind; each stop; one of each; an undrawn group; a node out, alone, and alone with a smallest flow. */
function samples(app, doc) {
  const groups = [...new Set(doc.nodes.map((n) => app.fundGroupOf(n)).filter(Boolean))];
  const kinds = [...new Set(doc.links.map((l) => l.kind))].filter((k) => k !== "");
  const absent = app.fundGroups().map((g) => g.id).filter((g) => !groups.includes(g));
  const of = (o) => Object.assign({ groups: new Set(), kinds: new Set(), nodes: new Set(), only: "", smallest: 0 }, o);
  const out = [of({})];
  for (const g of groups) out.push(of({ groups: new Set([g]) }));
  if (groups.length > 1) out.push(of({ groups: new Set(groups.slice(1)) }));
  if (groups.length) out.push(of({ groups: new Set(groups) }));
  for (const k of kinds) out.push(of({ kinds: new Set([k]) }));
  for (const stop of app.SMALLEST_STOPS.slice(1)) out.push(of({ smallest: stop }));
  out.push(of({ groups: new Set(groups.slice(0, 1)), kinds: new Set(kinds.slice(0, 1)), smallest: app.SMALLEST_STOPS[2] }));
  if (absent.length) out.push(of({ groups: new Set(absent) }));
  const node = plainOf(app, doc);
  if (node) {
    out.push(of({ nodes: new Set([node]) }));
    out.push(of({ only: node }));
    out.push(of({ only: node, smallest: app.SMALLEST_STOPS[2] }));
  }
  return out;
}

/** The first node of a chart by id that a ribbon touches and is no fund group or mark of ours, or "". */
function plainOf(app, doc) {
  const touched = new Set(doc.links.flatMap((l) => [l.source, l.target]));
  const ids = doc.nodes.map((n) => n.id).filter((id) => touched.has(id) && !app.isGroupID(id) && !app.isSmaller(id) &&
    !app.isAggregate(id)).sort();
  return ids.length ? ids[0] : "";
}

/** Fault lines for each way a filtered chart loses, doubles or invents a ribbon or cent. */
function faultsOf(app, doc, state, where) {
  const faults = [];
  const r = app.filterFor(doc, state);
  const tally = app.hiddenTally(r);
  const seen = new Map(doc.links.map((l) => [l, 0]));
  // A NODE FILTER CUTS A RIBBON ALONG ITS CELLS: a ribbon a hub draws a
  // node's leaves on is drawn without them and they are hidden, each part at
  // its ribbon's ends and kind, which one ribbon of the chart holds.
  const ribbonKey = (l) => `${l.source}\u001f${l.target}\u001f${l.kind}`;
  const byKey = new Map(doc.links.map((l) => [ribbonKey(l), l]));
  if (byKey.size !== doc.links.length) faults.push(`${where}: two ribbons of the chart share their ends and kind`);
  const parts = new Map();
  const bump = (l, how) => {
    if (seen.has(l)) {
      seen.set(l, seen.get(l) + 1);
      return;
    }
    const whole = byKey.get(ribbonKey(l));
    if (!whole || !app.cellsOf(whole)) {
      faults.push(`${where}: ${how} ${l.source} -> ${l.target} is no ribbon of the chart`);
      return;
    }
    if (!parts.has(whole)) parts.set(whole, []);
    parts.get(whole).push(l);
  };
  let merged = 0;
  for (const l of r.doc.links) {
    if (!Array.isArray(l.legs)) {
      bump(l, "drawn");
      continue;
    }
    merged++;
    for (const leg of l.legs) bump(leg, "merged");
    const legsCents = l.legs.reduce((s, x) => s + x.value_cents, 0);
    if (l.legs.length < 2) faults.push(`${where}: a merged ribbon of ${l.legs.length} leg`);
    if (l.value_cents !== legsCents) faults.push(`${where}: a merged ribbon carries ${l.value_cents}, its legs ${legsCents}`);
    if (JSON.stringify(l.fact_ids) !== JSON.stringify([...factsOf(l.legs)].sort())) faults.push(`${where}: a merged ribbon's facts are not its legs' union`);
    if (!app.isSmaller(l.source) && !app.isSmaller(l.target)) faults.push(`${where}: a merged ribbon ends at no smaller-flows mark`);
    for (const leg of l.legs) {
      if (Math.abs(leg.value_cents) >= state.smallest || leg.derived || leg.contra) {
        faults.push(`${where}: ${leg.source} -> ${leg.target} (${leg.value_cents}) is merged and should not fold`);
      }
    }
  }
  for (const l of r.hidden) bump(l, "hidden");
  // A ribbon cut in parts: every cell in exactly one part at its cents, and
  // the parts' figures its figure.
  for (const [whole, ps] of parts) {
    const at = `${where}: ${whole.source} -> ${whole.target}`;
    if (seen.get(whole) !== 0) faults.push(`${at} is drawn whole and in parts`);
    seen.set(whole, ps.length > 1 ? 1 : 0);
    const cells = app.cellsOf(whole);
    const got = {};
    for (const p of ps) {
      for (const [k, c] of Object.entries(app.cellsOf(p) || {})) {
        if (k in got) faults.push(`${at}: the cell ${k} is in two parts`);
        got[k] = c;
      }
    }
    if (JSON.stringify(Object.entries(got).sort()) !== JSON.stringify(Object.entries(cells).sort())) faults.push(`${at}: its parts' cells are not its cells`);
    if (ps.reduce((s, p) => s + p.value_cents, 0) !== whole.value_cents) faults.push(`${at}: its parts do not sum to it`);
  }
  for (const [l, n] of seen) if (n !== 1) faults.push(`${where}: ${l.source} -> ${l.target} is accounted ${n} times`);
  // Drawn plus hidden is the unfiltered chart, ribbon by ribbon and by fact
  // ids. The ribbons' own figures, not the tally: a hub draws a leaf twice,
  // and the tally states each once ("the tally states a leaf once" below).
  if (sum(app, r.doc.links) + sum(app, r.hidden) !== sum(app, doc.links)) {
    faults.push(`${where}: drawn ${sum(app, r.doc.links)} + hidden ${sum(app, r.hidden)} is not the chart's ${sum(app, doc.links)}`);
  }
  if (tally.flows !== r.hidden.length) faults.push(`${where}: the tally counts ${tally.flows} of ${r.hidden.length} hidden ribbons`);
  const union = new Set([...factsOf(r.doc.links), ...tally.facts]);
  const whole = factsOf(doc.links);
  if (union.size !== whole.size || [...whole].some((id) => !union.has(id))) faults.push(`${where}: drawn and hidden cite ${union.size} facts, the chart ${whole.size}`);
  // Every end drawn, every node drawn touched.
  const ids = new Set(r.doc.nodes.map((n) => n.id));
  const touched = new Set(r.doc.links.flatMap((l) => [l.source, l.target]));
  for (const id of touched) if (!ids.has(id)) faults.push(`${where}: a ribbon ends at ${id}, which is not drawn`);
  for (const id of ids) if (!touched.has(id)) faults.push(`${where}: ${id} is drawn and no ribbon touches it`);
  // A FILTER THAT CUTS EVERY RIBBON INTO A SLOT STILL LAYS OUT: layOutAt's
  // scaffold runs a path through every slot. Any throw is a fault.
  try {
    app.layOut(r.doc);
  } catch (e) {
    faults.push(`${where}: the filtered chart does not lay out: ${e.message}`);
  }
  return { faults, merged, hidden: r.hidden.length, cut: parts.size };
}

// The property on representative charts: the root, everything open, and
// everything open with every tail drawn out, which between them hide, merge
// and cut along cells. Walking every split adds charts, not arms.
describe("drawn plus hidden is the chart", () => {
  for (const year of YEARS.slice(0, 1)) {
    test(`${year.label}: at a sample of filters, no ribbon, cent or fact is lost, duplicated or invented`, async (t) => {
      const { app } = await bootedApp({ checkedStem: year.stem });
      const faults = [];
      let states = 0;
      let merged = 0;
      let hidden = 0;
      let cut = 0;
      // A NODE FILTER CAN HIDE PART OF THE LEAVES A TOTAL RESTATES beside
      // those leaves, which the tally refuses to state (core.distinctCents)
      // and filterFor refuses to draw; a group or kind filter never does.
      const refusedNode = [];
      let drawnNode = 0;
      // EACH CHART SHAPED AS THE PAGE SHAPES IT, with no DOM: the claim is
      // the filter's arithmetic over the shaped document, which is what
      // app.projection holds after a redraw.
      const visit = (where, doc) => {
        // Laid out first, as a redraw lays it out: app.fundGroupOf, which
        // samples and the ribbons' ends read, then knows this chart's own
        // marks, which hueIndex takes from the document laid.
        app.layOut(doc);
        for (const state of samples(app, doc)) {
          const named = where +
            ` [${[...state.groups, ...state.kinds, ...state.nodes].join(", ")}${state.only ? " only " + state.only : ""}` +
            `${state.smallest ? " <" + state.smallest : ""}]`;
          const byNode = state.nodes.size > 0 || state.only !== "";
          let got;
          try {
            got = faultsOf(app, doc, state, named);
          } catch (e) {
            if (!byNode || !/cannot state these ribbons' money once/.test(e.message)) throw e;
            refusedNode.push(`${named}: ${e.message}`);
            continue;
          }
          if (byNode) drawnNode++;
          faults.push(...got.faults);
          merged += got.merged;
          hidden += got.hidden;
          cut += got.cut;
          states++;
        }
      };
      visit("(the root)", shapedAt(app, app.rootFrontier(app.fetched)));
      const charts = expansionStates(app, app.fetched).filter((st) => st.where.startsWith("everything"));
      for (const st of charts) visit(st.where, shapedAt(app, st.frontier, st.whole));
      t.diagnostic(`${year.label}: ${charts.length + 1} chart(s), ${states} filter state(s); ${hidden} ribbon(s) hidden ` +
        `and ${merged} merged ribbon(s) drawn across them; ${faults.length} fault(s)`);
      t.diagnostic(`${year.label}: ${drawnNode} node filter state(s) drawn, ${cut} ribbon(s) cut along their cells, ${refusedNode.length} refused` +
        (refusedNode.length ? `, the first ${refusedNode[0]}` : ""));
      assert.ok(hidden > 0 && merged > 0, "no state hid or merged a ribbon, so nothing here is tested");
      assert.ok(drawnNode > 0, "no node filter state was drawn, so the node filters are not tested");
      assert.ok(cut > 0, "no node filter cut a ribbon along its cells, so the parts are not tested");
      assert.deepEqual(faults.slice(0, 5), []);
    });
  }
});

// A hub draws a leaf twice, and totals restate leaves; the tally states
// each leaf once. With every primary split open, or every alternative,
// hiding the same group or kind states the root's cents. An object
// category's divisions restate other kinds' leaves, so that chart is held
// under a group or every kind at once.
describe("the tally states a leaf once, however many ribbons draw it", () => {
  for (const year of YEARS) {
    test(`${year.label}: hiding a group or a kind with every split open tallies what it tallies at the root`, async (t) => {
      const { app } = await bootedApp({ checkedStem: year.stem });
      const doc = app.fetched;
      const openAll = (alternative) => {
        let open = app.rootFrontier(doc);
        for (let changed = true; changed;) {
          changed = false;
          for (const sp of doc.splits) {
            const others = app.splitsOf(doc, sp.node).some((x) => x.alternative);
            if (sp.alternative !== (alternative && others)) continue;
            if (!open.has(sp.node) || app.openSplitOf(doc, open, sp.node) !== "") continue;
            open = app.expand(doc, open, sp.node, sp.key);
            changed = true;
          }
        }
        return open;
      };
      const shape = (frontier) => app.formOnScreen().shape(doc, { chart: CONFIG.overview, frontier, whole: new Set() });
      const root = shape(app.rootFrontier(doc));
      const kinds = [...new Set(root.links.map((l) => l.kind))].filter((k) => k !== "");
      const states = samples(app, root).filter((st) => !(st.smallest > 0) && (st.groups.size || st.kinds.size))
        .concat([{ groups: new Set(), kinds: new Set(kinds), smallest: 0 }]);
      const wrong = [];
      let repeated = 0;
      let restated = 0;
      for (const [name, open] of [["every primary split", openAll(false)], ["every alternative", openAll(true)]]) {
        const drawn = shape(open);
        const restating = drawn.links.filter((l) => Object.keys(app.cellsOf(l) || {}).some((k) => k.includes("restates")));
        if (!restating.length) wrong.push(`${name} open draws no total restating leaves`);
        for (const st of states) {
          if (name === "every alternative" && st.kinds.size && st.kinds.size !== kinds.length) continue;
          const where = `${name} open, [${[...st.groups, ...st.kinds].join(", ")}]`;
          const r = app.filterFor(root, st);
          const atRoot = app.hiddenTally(r).cents;
          const h = app.filterFor(drawn, st);
          const cents = app.hiddenTally(h).cents;
          if (atRoot !== sum(app, r.hidden)) wrong.push(`${where}: at the root the tally is ${atRoot}, its ribbons ${sum(app, r.hidden)}`);
          if (cents !== atRoot) wrong.push(`${where}: tallies ${cents}, the root ${atRoot}`);
          if (sum(app, h.hidden) !== atRoot) repeated++;
          if (h.hidden.some((l) => restating.includes(l))) restated++;
        }
        t.diagnostic(`${year.label}: ${open.size} nodes drawn with ${name} open, ${restating.length} ribbon(s) restating leaves`);
      }
      t.diagnostic(`${year.label}: ${states.length} filter state(s) per chart; ${repeated} hiding a leaf and a ribbon ` +
        `drawing it again, ${restated} hiding a total restating leaves`);
      assert.deepEqual(wrong.slice(0, 5), []);
      assert.ok(repeated > 0, "no state hides a leaf and a ribbon drawing it again, so nothing here is tested");
      assert.ok(restated > 0, "no state hides a total restating leaves, so the totals are not tested");
    });
  }
});

// Which leaves one division's total carries is unprinted, so part of the
// list beside one of its leaves states no money once: refused. No real
// filter reaches it; a planted hidden set does.
describe("the tally refuses totals restating leaves in part beside those leaves", () => {
  for (const year of YEARS) {
    test(`${year.label}: with every alternative open no kind or group filter is refused, and one division's total beside its category's leaves is`, async (t) => {
      const { app } = await bootedApp({ checkedStem: year.stem });
      const doc = app.fetched;
      let open = app.rootFrontier(doc);
      for (let changed = true; changed;) {
        changed = false;
        for (const sp of doc.splits) {
          if (sp.alternative !== app.splitsOf(doc, sp.node).some((x) => x.alternative)) continue;
          if (!open.has(sp.node) || app.openSplitOf(doc, open, sp.node) !== "") continue;
          open = app.expand(doc, open, sp.node, sp.key);
          changed = true;
        }
      }
      const drawn = app.formOnScreen().shape(doc, { chart: CONFIG.overview, frontier: open, whole: new Set() });
      const kinds = [...new Set(drawn.links.map((l) => l.kind))].filter((k) => k !== "");
      const groups = [...new Set(drawn.nodes.map((n) => app.fundGroupOf(n)).filter(Boolean))];
      const states = [];
      for (const k of kinds) states.push({ kinds: new Set([k]), groups: new Set() });
      for (const g of groups) {
        states.push({ kinds: new Set(), groups: new Set([g]) });
        for (const k of kinds) states.push({ kinds: new Set([k]), groups: new Set([g]) });
      }
      const refused = [];
      for (const st of states) {
        try {
          app.filterFor(drawn, Object.assign({ smallest: 0 }, st));
        } catch (e) {
          refused.push(`[${[...st.kinds, ...st.groups].join(", ")}]: ${e.message}`);
        }
      }
      assert.deepEqual(refused.slice(0, 5), []);
      // The plant: one division's total and every ribbon drawing a leaf of the
      // list it restates.
      const restating = (l) => Object.keys(app.cellsOf(l) || {}).find((k) => k.includes("restates"));
      const total = drawn.links.find((l) => restating(l) && drawn.nodes.find((n) => n.id === l.source).role === "object_category");
      assert.ok(total !== undefined, "no object category's division is drawn, so nothing here is tested");
      const list = new Set(restating(total).split("\u001f").pop().split("\u001e"));
      const leaves = drawn.links.filter((l) => Object.keys(app.cellsOf(l) || {}).some((k) => list.has(k)));
      const hidden = [total, ...leaves];
      const sum = hidden.reduce((s, l) => s + app.formOnScreen().cents(l), 0);
      const whole = [...list].reduce((s, k) => s + app.leafCents(doc).get(k), 0);
      t.diagnostic(`${year.label}: ${states.length} filter state(s), none refused; ${total.source} -> ${total.target} carries ` +
        `${total.value_cents} of the ${whole} its ${list.size} leaves carry, beside ${leaves.length} ribbon(s) drawing them; ` +
        `their ribbons sum to ${sum}`);
      assert.ok(leaves.length > 0 && total.value_cents < whole, "the planted total carries all of its list, or no leaf of it is drawn");
      t.diagnostic(`the tally says ${(() => { try { return app.hiddenTally({ hidden }).cents; } catch (e) { return e.message; } })()}`);
      assert.throws(() => app.hiddenTally({ hidden }), /cannot state these ribbons' money once/);
    });
  }
});

describe("the filter panel", () => {
  test("it lists Go's fund groups in order with the legend's hues, the hueless one last and muted, and every kind Go names", async (t) => {
    const { app, document } = await bootedApp({ checkedStem: YEARS[0].stem });
    const panel = document.getElementById("filters");
    const boxes = [...document.querySelectorAll("#filter-group-list input")];
    const kinds = [...document.querySelectorAll("#filter-kind-list input")];
    const vars = boxes.map((b) => b.parentElement.querySelector(".key").dataset.var);
    t.diagnostic(`groups ${boxes.map((b, i) => b.value.replace("fund-group/", "") + " " + vars[i]).join(", ")}; kinds ${kinds.map((k) => k.value).join(", ")}`);
    assert.equal(panel.hasAttribute("hidden"), false);
    assert.deepEqual(boxes.map((b) => b.value), app.fundGroups().map((g) => g.id));
    assert.deepEqual(vars, boxes.map((b) => app.fundColorVar(b.value)));
    assert.equal(boxes.at(-1).value, "fund-group/permanent");
    assert.equal(vars.at(-1), "--muted");
    assert.ok(vars.slice(0, -1).every((v) => v !== "--muted"));
    assert.deepEqual(kinds.map((k) => k.value), Object.keys(CONFIG.kind_labels));
    assert.deepEqual(kinds.map((k) => k.parentElement.textContent), Object.keys(CONFIG.kind_labels).map((k) => app.kindLabel(k)));
    assert.ok([...boxes, ...kinds].every((b) => b.checked), "a box starts unchecked, so something starts hidden");
  });

  test("every control has an accessible name, and none is out of the tab order", async () => {
    const { document } = await bootedApp();
    const controls = [...document.querySelectorAll("#filters input, #filters button, #filter-line button")];
    assert.ok(controls.length > 6, "the panel holds too few controls to be the panel");
    for (const c of controls) {
      const label = c.tagName === "INPUT"
        ? (c.closest("label") || document.querySelector(`label[for="${c.id}"]`) || { textContent: "" }).textContent
        : c.getAttribute("aria-label") || c.textContent;
      assert.ok(String(label).trim(), `${c.outerHTML.slice(0, 60)} has no name`);
      assert.notEqual(c.getAttribute("tabindex"), "-1");
      assert.equal(c.hasAttribute("disabled"), false, c.id);
    }
    assert.ok(document.querySelector("#filters > summary"), "the panel has no summary to open it by keyboard");
  });

  test("it opens by default where the rail sits beside the chart, and not on a narrow viewport", async () => {
    const wide = await bootedApp({ viewport: 2400 });
    const narrow = await bootedApp({ viewport: 900 });
    assert.equal(wide.document.getElementById("filters").open, true);
    assert.equal(narrow.document.getElementById("filters").open, false);
  });

  test("unchecking a group removes its ribbons, re-lays the chart out, says what it hid in the page's words, and Reset puts it back", async (t) => {
    const { app, document } = await bootedApp({ checkedStem: YEARS[0].stem });
    const chart = document.getElementById("chart");
    const ribbons = () => [...chart.querySelectorAll("path.link")];
    const line = document.getElementById("filter-line");
    const before = ribbons().length;
    const g = "fund-group/general";
    const touches = (l) => app.fundGroupOf(l.source) === g || app.fundGroupOf(l.target) === g;
    const box = [...document.querySelectorAll("#filter-group-list input")].find((b) => b.value === g);
    assert.equal(line.hasAttribute("hidden"), true);
    box.checked = false;
    fire(box, "change");
    const after = ribbons();
    const tally = app.hiddenTally();
    // AN INDEPENDENT SUM over the shaped chart's own ribbons: the line's
    // figure is the ribbons it removed, cited cents and nothing else.
    const expected = app.projection.links.filter((l) => {
      const at = (id) => app.fundGroupOf(app.projection.nodes.find((n) => n.id === id) || { id });
      return at(l.source) === g || at(l.target) === g;
    });
    const said = document.getElementById("filter-said").textContent;
    t.diagnostic(`${before} ribbon(s) before, ${after.length} after; the line says: ${said}`);
    assert.ok(app.filter.groups.has(g));
    assert.ok(!after.some((r) => touches(r.__data__)), "a ribbon of the hidden group is still drawn");
    assert.equal(after.length, app.shown.doc.links.length);
    assert.ok(!app.laidNodes.some((n) => n.id === g), "the hidden group's node is still laid out");
    assert.equal(tally.flows, expected.length);
    assert.equal(tally.cents, sum(app, expected));
    assert.equal(line.hasAttribute("hidden"), false);
    assert.equal(said, app.say("filter_hidden", { cents: dollars(tally.cents), flows: tally.flows }));
    assert.equal(refusals(document).length, 0);
    fire(document.getElementById("filter-reset"), "click");
    assert.equal(ribbons().length, before);
    assert.equal(line.hasAttribute("hidden"), true);
    assert.equal(box.checked, true);
    assert.equal(app.filter.groups.size, 0);
  });

  // Mutation: drop setFilter's clearing of an isolation the filtered chart
  // draws no mark of, and this goes red: the hash says iso= a group the
  // chart does not draw, which moveTo refuses on reload.
  test("hiding the fund group followed lets its isolation go, so the link says no group the chart does not draw", async () => {
    const { app, window } = await bootedApp({ checkedStem: YEARS[0].stem });
    const g = "fund-group/general";
    app.setIsolated(g);
    assert.ok(window.location.hash.includes("iso="), window.location.hash);
    app.setFilter({ groups: new Set([g]) });
    const got = { hidden: [...app.filter.groups], isolated: app.filter.isolated, drawn: app.laidNodes.some((n) => app.isolates(g, n)),
      hash: window.location.hash.includes("iso=") };
    assert.deepEqual(got, { hidden: [g], isolated: "", drawn: false, hash: false });
  });

  // Mutation: let setFilter's isolate after the redraw put back an
  // isolation of a group the redraw draws nothing of, and this goes red: the
  // page follows the group it has just hidden.
  test("hiding a fund group and isolating it in one state isolates nothing, so the page follows no group it does not draw", async () => {
    const { app, window } = await bootedApp({ checkedStem: YEARS[0].stem });
    const g = "fund-group/general";
    const ok = app.setFilter({ groups: new Set([g]), isolated: g });
    const got = { ok, hidden: [...app.filter.groups], isolated: app.filter.isolated, drawn: app.laidNodes.some((n) => app.isolates(g, n)),
      hash: window.location.hash.includes("iso=") };
    assert.deepEqual(got, { ok: true, hidden: [g], isolated: "", drawn: false, hash: false });
  });

  test("a hidden kind's ribbons leave the chart, and None then All hides every group's and puts them back without a refusal", async (t) => {
    const { app, document } = await bootedApp({ checkedStem: YEARS[0].stem });
    const chart = document.getElementById("chart");
    const ribbons = () => [...chart.querySelectorAll("path.link")].map((r) => r.__data__);
    const before = ribbons().length;
    const kind = ribbons().map((l) => l.kind).find((k) => k === "internal_transfer") || ribbons()[0].kind;
    const box = [...document.querySelectorAll("#filter-kind-list input")].find((b) => b.value === kind);
    box.checked = false;
    fire(box, "change");
    const withoutKind = ribbons();
    assert.ok(app.filter.kinds.has(kind));
    assert.ok(withoutKind.length < before, `hiding ${kind} removed nothing`);
    assert.ok(!withoutKind.some((l) => l.kind === kind));
    fire(document.getElementById("filter-none"), "click");
    const none = ribbons().length;
    const tally = app.hiddenTally();
    t.diagnostic(`${before} ribbon(s); ${withoutKind.length} without ${kind}; ${none} with every group hidden too, ` +
      `${tally.flows} hidden, ${dollars(tally.cents)}`);
    assert.equal(none, 0);
    assert.equal(tally.flows, app.projection.links.length);
    assert.equal(refusals(document).length, 0);
    // The hint is about the chart drawn: with nothing drawn, nothing opens.
    const hint = document.getElementById("chart-hint").textContent;
    assert.ok(hint.startsWith(app.say("nothing_opens")), hint);
    fire(document.getElementById("filter-all"), "click");
    assert.equal(ribbons().length, withoutKind.length);
  });

  test("the smallest-flow control merges small printed ribbons into smaller-flows marks whose ribbons say so and cite every leg", async (t) => {
    const { app, document } = await bootedApp({ checkedStem: YEARS[0].stem });
    const chart = document.getElementById("chart");
    const range = document.getElementById("filter-smallest");
    range.value = "3";
    fire(range, "input");
    const stop = app.SMALLEST_STOPS[3];
    const marks = [...chart.querySelectorAll("g.node")].map((m) => m.__data__).filter((n) => app.isSmaller(n.id));
    const merged = [...chart.querySelectorAll("path.link")].filter((r) => Array.isArray(r.__data__.legs));
    const said = document.getElementById("filter-said").textContent;
    t.diagnostic(`at ${dollars(stop)}: ${marks.length} smaller-flows mark(s), ${merged.length} merged ribbon(s); the line says: ${said}`);
    assert.equal(app.filter.smallest, stop);
    assert.equal(document.getElementById("filter-smallest-value").textContent, dollars(stop));
    assert.equal(range.getAttribute("aria-valuetext"), dollars(stop));
    assert.ok(marks.length > 0 && merged.length > 0, "the spine at this stop merges nothing, so nothing is tested");
    const folded = app.foldedTally();
    assert.equal(said, app.say("filter_folded", { flows: folded.flows, ribbons: folded.ribbons, threshold: dollars(stop) }));
    const r = merged[0];
    const d = r.__data__;
    const words = app.say("smaller_flow", { flows: d.legs.length, threshold: dollars(stop) });
    assert.ok(r.getAttribute("aria-label").includes(words), r.getAttribute("aria-label"));
    fire(r, "click");
    const panel = document.getElementById("detail").textContent;
    assert.ok(panel.includes(words), "the pinned ribbon does not say it is the page's merge");
    for (const leg of d.legs) for (const id of leg.fact_ids) assert.ok(panel.includes(id), `the pinned ribbon drops ${id}`);
    for (const m of marks) assert.ok(!app.splittable(m) && !app.expandable(m), `${m.id} offers to split`);
  });

  test("a filter survives a split and a year change, and one naming a group the view does not draw is inert", async (t) => {
    const { app, document } = await bootedApp({ checkedStem: YEARS[0].stem });
    const chart = document.getElementById("chart");
    const g = "fund-group/debt-service";
    const drawsGroup = () => [...chart.querySelectorAll("path.link")].some((r) =>
      app.fundGroupOf(r.__data__.source) === g || app.fundGroupOf(r.__data__.target) === g);
    // A group the spine does not draw: nothing hidden, the chart as shaped.
    app.setFilter({ groups: new Set(["fund-group/permanent"]) });
    assert.equal(app.shown.hidden.length, 0);
    assert.ok(app.shown.doc === app.projection, "the filter reshaped a chart it hides nothing of");
    assert.equal(document.getElementById("filter-line").hasAttribute("hidden"), true);
    app.setFilter({ groups: new Set([g]) });
    assert.equal(drawsGroup(), false);
    await split(app, "fund-group/general");
    const opened1 = { groups: [...app.filter.groups], refused: refusals(document).length };
    clickYear(document, YEARS[1].stem);
    await settle();
    const box = [...document.querySelectorAll("#filter-group-list input")].find((b) => b.value === g);
    t.diagnostic(`with the General Fund group split the filter holds ${JSON.stringify(opened1.groups)}; ` +
      `on ${YEARS[1].label} it holds ${JSON.stringify([...app.filter.groups])} and the box is ${box.checked ? "checked" : "unchecked"}`);
    assert.equal(opened1.refused, 0);
    assert.ok(opened1.groups.includes(g));
    assert.ok(app.filter.groups.has(g));
    assert.equal(app.shownYear.stem, YEARS[1].stem);
    assert.equal(box.checked, false);
    assert.equal(drawsGroup(), false);
    assert.ok(app.hiddenTally().flows > 0);
  });
});

describe("a chart that draws no ribbon into an extend split's slot", () => {
  // A division stands a slot after its category; with no ribbon into the
  // category d3 alone would draw it a column early. The chart is the split
  // minus every ribbon into the category, as no filter reaches it here.
  test("lays out, each node in its slot's column", async (t) => {
    const { app } = await bootedApp({ checkedStem: YEARS[0].stem });
    const C = "expenditure/capital-outlay";
    await split(app, C + "~divisions");
    const divisions = new Set(app.splitsHere({ id: C }).find((sp) => sp.key === "divisions").members);
    const columns = new Set(app.slotsOf(app.laidNodes, app.RENDER_TIERS).values()).size;
    const doc = structuredClone(app.shown.doc);
    doc.links = doc.links.filter((l) => l.target !== C);
    const touched = new Set(doc.links.flatMap((l) => [l.source, l.target]));
    doc.nodes = doc.nodes.filter((n) => touched.has(n.id));
    doc.filtered = true;
    const laid = app.layOut(doc);
    const slots = app.slotsOf(laid.nodes, app.RENDER_TIERS);
    const misplaced = laid.nodes.filter((n) => n.layer !== slots.get(n.id)).map((n) => n.id);
    const last = laid.nodes.filter((n) => divisions.has(n.id) && slots.get(n.id) === columns - 1).map((n) => n.id);
    t.diagnostic(`${last.length} division(s) drawn in the last of ${columns} column(s)`);
    assert.deepEqual(misplaced, []);
    assert.ok(last.length > 0, "no division is drawn in the last column, so nothing here is tested");
    // The case the clamp refused: the longest drawn path is too short to
    // reach the last column, so d3 counts fewer columns than the slots hold.
    const steps = new Map();
    for (const n of [...laid.nodes].sort((a, b) => slots.get(a.id) - slots.get(b.id))) {
      steps.set(n.id, Math.max(0, ...n.targetLinks.map((l) => steps.get(l.source.id) + 1)));
    }
    const longest = Math.max(...steps.values());
    t.diagnostic(`the longest drawn path is ${longest} step(s) over ${columns} column(s)`);
    assert.ok(longest + 1 < columns, "a drawn path spans every column, so the scaffold is not tested");
  });
});

describe("a filter never restates a mark's figure", () => {
  for (const year of YEARS) {
    test(`${year.label}: with a fund group hidden, a node's tooltip, panel, label and accessible name headline the unfiltered figure, and say the drawn part beside it`, async (t) => {
      const { app, document } = await bootedApp({ checkedStem: year.stem });
      const chart = document.getElementById("chart");
      const marks = () => new Map([...chart.querySelectorAll("g.node")].map((g) => [g.__data__.id, g]));
      const read = (g) => {
        const d = g.__data__;
        app.showTip({ target: chart, clientX: 0, clientY: 0 }, d);
        const tip = document.getElementById("tooltip");
        app.pin(d);
        return {
          cents: app.formOnScreen().cents(d),
          tip: tip.querySelector(".tip-value").textContent,
          tipText: tip.textContent,
          tipDrawn: tip.querySelector(".tip-drawn") !== null,
          panel: document.querySelector("#detail .amount").textContent,
          panelText: document.getElementById("detail").textContent,
          aria: g.getAttribute("aria-label"),
          value: g.querySelector("tspan.value").textContent,
        };
      };
      // THE UNFILTERED VIEW, read off the page before any filter: what each
      // mark says when nothing is hidden.
      const before = new Map([...marks()].map(([id, g]) => [id, read(g)]));
      app.setFilter({ groups: new Set(["fund-group/general"]) });
      const after = marks();
      let changed = 0;
      for (const [id, g] of after) {
        const was = before.get(id);
        if (!was) continue;
        const now = read(g);
        assert.equal(now.tip, was.tip, `${id}'s tooltip headlines ${now.tip}, the unfiltered view ${was.tip}`);
        assert.equal(now.panel, was.panel, `${id}'s panel headlines ${now.panel}, the unfiltered view ${was.panel}`);
        assert.equal(now.value, was.value, `${id}'s label says ${now.value}, the unfiltered view ${was.value}`);
        const drawn = app.say("filter_drawn_at", { cents: dollars(now.cents) });
        if (now.cents === was.cents) {
          assert.equal(now.tipDrawn, false, `${id} says a drawn part where all of it is drawn`);
          continue;
        }
        changed++;
        const label = g.__data__.label;
        assert.ok(now.aria.startsWith(`${label}, total ${dollars(was.cents)}, ${drawn},`), now.aria);
        assert.ok(now.tipDrawn && now.tipText.includes(drawn), `${id}'s tooltip does not say the drawn part: ${now.tipText}`);
        assert.ok(now.panelText.includes(drawn), `${id}'s panel does not say the drawn part`);
      }
      t.diagnostic(`${year.label}: ${after.size} mark(s) drawn with the General Fund group hidden, ${changed} of them drawn at less than their figure`);
      assert.ok(changed > 0, "no mark lost part of its figure to the filter, so nothing here is tested");
      assert.equal(refusals(document).length, 0);
    });
  }
});

// A PART HIDDEN IS ONLY TALLIED: core.partOf holds a part drawn to its
// ribbon's sign, and the part a node filter hides is read by its cells at
// their printed signs (core.distinctCents), so it may net the other way.
describe("a node filter hiding a contra cell of a ribbon", () => {
  // Mutation: pass partOf no `false` for the hidden part in
  // core.filterDocument and this goes red: the filter is refused, "the part
  // nets the other way".
  test("draws the rest and tallies the contra at its printed sign", async (t) => {
    const { app } = await bootedApp({ checkedStem: YEARS[0].stem });
    await split(app, "fund-group/enterprise");
    const doc = structuredClone(app.shown.doc);
    const l = doc.links.find((x) => x.value_cents > 0 && x.cells && Object.keys(x.cells).length > 0);
    assert.ok(l, "no ribbon carries cells, so nothing here is cut");
    // The plant: one cell more on the ribbon, a contra from a node of its
    // own netting -30 cents, the ribbon's figure its cells' sum still.
    const contra = "\u001fplanted/contra\u001f" + l.target + "\u001fexternal\u001f";
    const first = Object.keys(l.cells)[0];
    l.cells[first] += 30;
    l.cells[contra] = -30;
    l.cell_sources[contra] = { fact_ids: l.cell_sources[first].fact_ids.slice(), pages: [], derived: false };
    const state = { groups: new Set(), kinds: new Set(), nodes: new Set(["planted/contra"]), smallest: 0 };
    const r = app.filterDocument(doc, () => "", state, null, { out: new Set(["planted/contra"]), only: null });
    const drawnPart = r.doc.links.find((x) => x.source === l.source && x.target === l.target && x.kind === l.kind);
    const hiddenPart = r.hidden.find((x) => x.source === l.source && x.target === l.target && x.kind === l.kind);
    const tally = app.distinctCents(r.hidden, (x) => x.value_cents);
    t.diagnostic(`${l.source} -> ${l.target} ${l.value_cents}: drawn ${drawnPart && drawnPart.value_cents}, ` +
      `hidden ${hiddenPart && hiddenPart.value_cents}, tallied ${tally}`);
    assert.deepEqual({ drawn: drawnPart && drawnPart.value_cents, hidden: hiddenPart && hiddenPart.value_cents, tally },
      { drawn: l.value_cents + 30, hidden: -30, tally: -30 });
  });
});

// On a chart small enough to name every ribbon; the walks above would pass
// a filter hiding the wrong ones. Mutation: test only a ribbon's source
// against hidden groups, or its kind against the group set, and an arm goes red.
describe("a group or kind filter on a planted chart", () => {
  test("hides exactly the ribbons it names", () => {
    globalThis.FISC_CONFIG = pageFixture().config;
    const app = core;
    const node = (id) => ({ id, label: id, kind: "x", tier: 0 });
    const link = (source, target, kind, value_cents) => ({ source, target, kind, value_cents, fact_ids: [`f-${source}-${target}`] });
    const doc = {
      nodes: ["a1", "a2", "b1", "b2"].map(node),
      links: [link("a1", "a2", "revenue", 100), link("a1", "b2", "transfer", 200), link("b1", "b2", "revenue", 300),
        link("b1", "a2", "revenue", 400)],
    };
    const groupOf = (n) => n.id[0];
    const ends = (ls) => ls.map((l) => `${l.source}->${l.target}`).sort();
    const none = { groups: new Set(), kinds: new Set(), nodes: new Set(), smallest: 0 };
    const byGroup = app.filterDocument(structuredClone(doc), groupOf, { ...none, groups: new Set(["a"]) }, null, null);
    assert.deepEqual({ drawn: ends(byGroup.doc.links), hidden: ends(byGroup.hidden) },
      { drawn: ["b1->b2"], hidden: ["a1->a2", "a1->b2", "b1->a2"] });
    const byKind = app.filterDocument(structuredClone(doc), groupOf, { ...none, kinds: new Set(["transfer"]) }, null, null);
    assert.deepEqual({ drawn: ends(byKind.doc.links), hidden: ends(byKind.hidden) },
      { drawn: ["a1->a2", "b1->a2", "b1->b2"], hidden: ["a1->b2"] });
  });
});

describe("a node filter and a ribbon carrying no cells", () => {
  // Mutation: keep a cell-less ribbon whatever leaves.out holds (no
  // leaves.out arm in core.filterDocument) and this goes red: the ribbon at
  // a drawn member of the node filtered out stays drawn.
  test("removes one with an end within the node filtered out", async (t) => {
    const { app } = await bootedApp();
    await split(app, "fund-group/enterprise");
    const doc = structuredClone(app.shown.doc);
    const [l, other] = doc.links.filter((x) => x.value_cents > 0);
    assert.ok(l && other, "the chart draws fewer than two ribbons");
    // The plant: the ribbon carries no cells, and its target is a drawn
    // member of a node filtered out that the chart does not draw.
    delete l.cells;
    delete l.cell_sources;
    const state = { groups: new Set(), kinds: new Set(), nodes: new Set(["planted/parent"]), smallest: 0 };
    const out = new Set(["planted/parent", l.target].filter((id) => id !== other.source && id !== other.target));
    assert.ok(out.has(l.target), `${l.target} ends the other ribbon too, so nothing here is told apart`);
    const r = app.filterDocument(doc, () => "", state, null, { out: out, only: null });
    const drawn = (x) => r.doc.links.some((y) => y.source === x.source && y.target === x.target && y.kind === x.kind);
    t.diagnostic(`${l.source} -> ${l.target} cell-less, ${l.target} within the node filtered out`);
    assert.deepEqual({ planted: drawn(l), hidden: r.hidden.includes(l) }, { planted: false, hidden: true });
  });
});
