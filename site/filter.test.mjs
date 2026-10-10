// The filter panel and the filter state behind it: what the reader hides is
// removed before layout and said beside the chart as a client sum over cited
// cents, and what the smallest-flow filter merges keeps every cent, fact and
// page it merged. Every check runs over the pinned columns, through the
// shipped app.js.

import { describe, test } from "node:test";
import assert from "node:assert/strict";

import { bootedApp, everySplit, split, settle, clickYear, fire, pageFixture, refusals, dollars } from "./testlib.mjs";

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

/** The fund group of one end of a shaped chart's ribbon. */
const groupAt = (app, doc, id) => {
  const n = doc.nodes.find((x) => x.id === id);
  return n ? app.fundGroupOf(n) : "";
};

/**
 * The filter states sampled on one chart: none; each group present alone,
 * every one but the first, and all; each kind present; each smallest-flow
 * stop; one of each together; and a group the chart does not draw.
 */
function samples(app, doc) {
  const groups = [...new Set(doc.nodes.map((n) => app.fundGroupOf(n)).filter(Boolean))];
  const kinds = [...new Set(doc.links.map((l) => l.kind))].filter((k) => k !== "");
  const absent = app.fundGroups().map((g) => g.id).filter((g) => !groups.includes(g));
  const of = (o) => Object.assign({ groups: new Set(), kinds: new Set(), smallest: 0 }, o);
  const out = [of({})];
  for (const g of groups) out.push(of({ groups: new Set([g]) }));
  if (groups.length > 1) out.push(of({ groups: new Set(groups.slice(1)) }));
  if (groups.length) out.push(of({ groups: new Set(groups) }));
  for (const k of kinds) out.push(of({ kinds: new Set([k]) }));
  for (const stop of app.SMALLEST_STOPS.slice(1)) out.push(of({ smallest: stop }));
  out.push(of({ groups: new Set(groups.slice(0, 1)), kinds: new Set(kinds.slice(0, 1)), smallest: app.SMALLEST_STOPS[2] }));
  if (absent.length) out.push(of({ groups: new Set(absent) }));
  return out;
}

/**
 * Every way one filtered chart can lose, duplicate or invent a ribbon or a
 * cent, as fault lines; none for a chart that holds.
 */
function faultsOf(app, doc, state, where) {
  const faults = [];
  const r = app.filterFor(doc, state);
  const tally = app.hiddenTally(r);
  const seen = new Map(doc.links.map((l) => [l, 0]));
  const bump = (l, how) => {
    if (!seen.has(l)) faults.push(`${where}: ${how} ${l.source} -> ${l.target} is no ribbon of the chart`);
    else seen.set(l, seen.get(l) + 1);
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
  for (const [l, n] of seen) if (n !== 1) faults.push(`${where}: ${l.source} -> ${l.target} is accounted ${n} times`);
  // Removed exactly what the state names: nothing drawn that it hides, and
  // nothing hidden that it does not.
  const hides = (l) => state.kinds.has(l.kind) || state.groups.has(groupAt(app, doc, l.source)) ||
    state.groups.has(groupAt(app, doc, l.target));
  for (const l of r.hidden) if (!hides(l)) faults.push(`${where}: ${l.source} -> ${l.target} is hidden and the state names nothing of it`);
  for (const l of r.doc.links) {
    for (const leg of l.legs || [l]) if (hides(leg)) faults.push(`${where}: ${leg.source} -> ${leg.target} is drawn and the state hides it`);
  }
  // Drawn plus hidden is the unfiltered chart, by cents and by fact ids.
  if (sum(app, r.doc.links) + tally.cents !== sum(app, doc.links)) {
    faults.push(`${where}: drawn ${sum(app, r.doc.links)} + hidden ${tally.cents} is not the chart's ${sum(app, doc.links)}`);
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
  // A FILTER THAT CUTS EVERY RIBBON INTO A SLOT IS REFUSED, not laid out a
  // column early: layOutAt throws, and setFilter says so in its banner. Any
  // other throw is a fault.
  let refused = "";
  try {
    app.layOut(r.doc);
  } catch (e) {
    if (/no ribbon drawn reaches that far/.test(e.message)) refused = `${where}: ${e.message}`;
    else faults.push(`${where}: the filtered chart does not lay out: ${e.message}`);
  }
  return { faults, merged, hidden: r.hidden.length, refused };
}

describe("drawn plus hidden is the chart", () => {
  for (const year of YEARS) {
    test(`${year.label}: on every split the page offers, at a sample of filters, no ribbon, cent or fact is lost, duplicated or invented`, async (t) => {
      const { app } = await bootedApp({ checkedStem: year.stem });
      const faults = [];
      let states = 0;
      let merged = 0;
      let hidden = 0;
      const refused = [];
      const visit = (where) => {
        const doc = app.projection;
        for (const state of samples(app, doc)) {
          const got = faultsOf(app, doc, state, where +
            ` [${[...state.groups, ...state.kinds].join(", ")}${state.smallest ? " <" + state.smallest : ""}]`);
          faults.push(...got.faults);
          merged += got.merged;
          hidden += got.hidden;
          if (got.refused) refused.push(got.refused);
          states++;
        }
      };
      visit("(the root)");
      const walked = await everySplit(app, async (where) => visit(where));
      assert.equal(walked.refused, "", walked.refused);
      t.diagnostic(`${year.label}: ${walked.visited + 1} chart(s), ${states} filter state(s); ${hidden} ribbon(s) hidden ` +
        `and ${merged} merged ribbon(s) drawn across them; ${faults.length} fault(s); ${refused.length} state(s) ` +
        `refused because no ribbon drawn reaches a slot:\n  ${refused.join("\n  ")}`);
      assert.ok(refused.length < states / 10, `${refused.length} of ${states} states refused`);
      assert.ok(hidden > 0 && merged > 0, "no state hid or merged a ribbon, so nothing here is tested");
      assert.deepEqual(faults.slice(0, 5), []);
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

  test("it opens by default where the viewport has room for a chart one column wider than the spine, and not on a narrow one", async () => {
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
    assert.equal(app.shown.doc, app.projection);
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

describe("a filter the expansion's slots cannot lay out", () => {
  // THE CLAMP (layOutAt): a division an object category is split into stands
  // in a slot after the category's, and hiding every group that pays for the
  // category leaves no ribbon reaching that far, so d3-sankey would draw the
  // divisions a column early. The page refuses the filter instead.
  test("is refused with the banner, and the chart, the filter and the panel stay as they were", async (t) => {
    const { app, document } = await bootedApp({ checkedStem: YEARS[0].stem });
    await split(app, "expenditure/capital-outlay~divisions");
    const laid = app.laidNodes.map((n) => n.id).sort().join(",");
    const hide = new Set(app.fundGroups().map((g) => g.id).filter((g) => g !== "fund-group/capital" && g !== "fund-group/permanent"));
    const drew = app.setFilter({ groups: hide });
    const banners = refusals(document).map((b) => b.textContent);
    t.diagnostic(`hiding ${[...hide].join(", ")}: setFilter ${drew}; ${banners.join(" | ")}`);
    assert.equal(drew, false);
    assert.equal(banners.length, 1);
    assert.ok(banners[0].startsWith("The chart could not be filtered: ") && banners[0].includes("no ribbon drawn reaches that far"), banners[0]);
    assert.equal(app.filter.groups.size, 0);
    assert.equal(app.laidNodes.map((n) => n.id).sort().join(","), laid);
    const boxes = [...document.querySelectorAll("#filter-group-list input")];
    assert.ok(boxes.every((b) => b.checked), "a box was left unchecked for a filter the page refused");
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
