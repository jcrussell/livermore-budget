// bars.test.mjs — THE BARS FORM LISTS WHAT THE SANKEY FOLDS. A fund group's
// window draws its funds as a Sankey column, capped, its tail folded into "N
// smaller funds"; the group's own mark opens again into the fund-list step,
// which site/bars.js draws as one ranked bar per fund. Every figure a bar
// shows is a sum of the links its window draws for that fund, so the bars are
// held here to the window's column, every tail drawn out, and to the spine's
// own figure for the group, over both pinned columns.

import { describe, test } from "node:test";
import assert from "node:assert/strict";
import {
  bootedApp, loadApp, settle, opened, expandAll, clickYear, pageFixture, stepByKey, refusals, fire,
  dollars, shortDollars, columnFixture,
} from "./testlib.mjs";

const CONFIG = pageFixture().config;
const STEP = stepByKey(CONFIG, "fund-list");
const GROUP_STEP = stepByKey(CONFIG, STEP.after[0]);
/** The group whose window folds the most funds into its tail. */
const WIDEST = "fund-group/special-revenue";

/** The page booted on one year. */
async function onYear(year, o = {}) {
  const loaded = await bootedApp(o);
  if (loaded.app.shownYear.stem !== year.stem) {
    clickYear(loaded.document, year.stem);
    await settle();
  }
  assert.equal(loaded.app.shownYear.stem, year.stem);
  return loaded;
}

/** Cents and every cited fact over a set of links. */
function tally(links) {
  const facts = new Set();
  let cents = 0;
  for (const l of links) {
    cents += l.value_cents;
    for (const id of l.fact_ids) facts.add(id);
  }
  return { cents, facts: [...facts].sort() };
}

/**
 * What one group's window draws for its funds, every tail drawn out, and
 * what its list draws, both read off the page the reader walks: opened on the
 * overview, drawn out, and opened again.
 */
async function windowAndList(app, group) {
  app.drillUp(0);
  await settle();
  await opened(app, group);
  expandAll(app);
  await settle();
  const win = app.projection;
  const tierOf = new Map(win.nodes.map((n) => [n.id, n.tier]));
  const toFunds = win.links.filter((l) => l.source === group && tierOf.get(l.target) === GROUP_STEP.from + 1);
  const intoGroup = win.links.filter((l) => l.target === group);
  const intoResidual = win.links.filter((l) => app.isResidual(l.target));
  await opened(app, group);
  const laid = app.laidNodes;
  const head = laid.filter((n) => n.layer === 0);
  const bars = laid.filter((n) => n.layer === 1);
  return { win, toFunds, intoGroup, intoResidual, head, bars, list: app.projection };
}

for (const year of CONFIG.years) {
  describe(`${year.label}: every fund group's list`, () => {
    test(`${year.label}: the bars sum to the column the group's window draws and to the spine's figure for the group, and no fund is lost or listed twice`, async (t) => {
      const { app } = await onYear(year);
      const spine = app.docAt(0);
      const groups = app.projection.nodes.filter((n) => app.drillable(n) && n.tier === GROUP_STEP.from).map((n) => n.id);
      const wrong = [];
      const said = [];
      let listed = 0;
      let widest = 0;
      for (const group of groups) {
        const got = await windowAndList(app, group);
        const ids = got.bars.map((n) => n.id);
        const want = [...new Set(got.toFunds.map((l) => l.target))].sort();
        const sum = got.bars.reduce((s, n) => s + app.BARS.cents(n), 0);
        const window = tally(got.toFunds);
        const pieces = tally(got.list.links);
        const intake = tally(got.intoGroup).cents;
        const printed = tally(spine.links.filter((l) => l.target === group)).cents;
        const carried = tally(got.intoResidual).cents;
        listed += ids.length;
        if (group === WIDEST) widest = ids.length;
        said.push(`${group} ${ids.length} bar(s) ${sum}`);
        // NO FUND LOST OR LISTED TWICE, by id, by cents and by fact.
        if (ids.length !== new Set(ids).size) wrong.push(`${group}: a fund is listed twice in ${ids.join(", ")}`);
        if ([...ids].sort().join() !== want.join()) wrong.push(`${group}: lists ${[...ids].sort().join(", ")} where its window draws ${want.join(", ")}`);
        for (const bar of got.bars) {
          const own = tally(got.toFunds.filter((l) => l.target === bar.id));
          const drawn = tally(bar.sourceLinks.concat(bar.targetLinks));
          if (app.BARS.cents(bar) !== own.cents) wrong.push(`${group}: ${bar.id} is ${app.BARS.cents(bar)} cents and its window draws ${own.cents}`);
          if (drawn.facts.join() !== own.facts.join()) wrong.push(`${group}: ${bar.id} cites ${drawn.facts.length} fact(s) and its window ${own.facts.length}`);
        }
        if (pieces.facts.join() !== window.facts.join()) wrong.push(`${group}: the pieces cite ${pieces.facts.length} fact(s) and the window's fund column ${window.facts.length}`);
        // THE SUMS: the bars, the window's fund column, what the window's
        // group takes in, the group's own row, and the spine's printed figure
        // less what the window carries past the funds under the residual.
        if (sum !== window.cents) wrong.push(`${group}: the bars sum to ${sum} and the window's fund column to ${window.cents}`);
        if (sum !== intake) wrong.push(`${group}: the bars sum to ${sum} and the window's group takes in ${intake}`);
        if (got.head.length !== 1 || app.BARS.cents(got.head[0]) !== sum) wrong.push(`${group}: the group's own row is not the bars' sum`);
        if (sum + carried !== printed) wrong.push(`${group}: the bars sum to ${sum}, the residual carries ${carried} and the spine prints ${printed}`);
      }
      t.diagnostic(`${year.label}: ${groups.length} group(s) listed, ${listed} bar(s): ${said.join("; ")}`);
      assert.deepEqual(wrong, []);
      assert.ok(groups.length > 1, "the page offers too few groups to list");
      const cap = GROUP_STEP.sankey.caps.find((c) => c.tier === GROUP_STEP.from + 1);
      assert.ok(widest > cap.cap + 1, `${WIDEST} lists ${widest} funds, which its window's cap of ${cap.cap} folds none of`);
    });

    test(`${year.label}: a list draws each fund as one row and its pieces as the links it sums, ranked, labelled and cited`, async (t) => {
      const { app, document } = await onYear(year);
      await opened(app, WIDEST);
      await opened(app, WIDEST);
      const chart = document.getElementById("chart");
      const layer = chart.querySelector(":scope > g.zoom-layer");
      assert.ok(layer, "the form drew outside the zoom's layer");
      const rows = [...layer.querySelectorAll("g.node")];
      const pieces = [...layer.querySelectorAll("path.link")];
      assert.deepEqual(rows.map((g) => g.__data__.id), app.laidNodes.map((n) => n.id));
      assert.equal(pieces.length, app.projection.links.length);
      assert.deepEqual(refusals(document), []);
      const wrong = [];
      let last = Infinity;
      let widestLabel = 0;
      for (const g of rows) {
        const d = g.__data__;
        const words = [...g.querySelector("text.halo").children].map((s) => s.textContent);
        const pct = app.barPercent(d, app.laidNodes);
        const want = [d.label, shortDollars(app.markFigure(d)), pct ? `  ◇ ${pct}%` : "", app.nodeFlags(d)];
        if (words.join("|") !== want.join("|")) wrong.push(`${d.id} reads ${JSON.stringify(words)}, not ${JSON.stringify(want)}`);
        // A LABEL FITS LEFT OF THE BARS at the layout test's 0.6em advance.
        widestLabel = Math.max(widestLabel, String(d.label).length * 0.6 * 12);
        if (d.layer !== 1) continue;
        if (app.BARS.cents(d) > last) wrong.push(`${d.id} ranks below a smaller bar`);
        last = app.BARS.cents(d);
        // THE PIECES TILE THE BAR, end to end, so the length drawn is the sum.
        const own = d.sourceLinks.concat(d.targetLinks).sort((a, b) => a.x0 - b.x0);
        let at = d.x0;
        for (const l of own) {
          if (Math.abs(l.x0 - at) > 1e-6) wrong.push(`${d.id}'s piece ${l.fact_ids[0]} starts at ${l.x0}, not ${at}`);
          at = l.x1;
        }
        if (Math.abs(at - d.x1) > 1e-6) wrong.push(`${d.id}'s pieces end at ${at} and its bar at ${d.x1}`);
      }
      for (const p of pieces) {
        if (!p.getAttribute("aria-label")) wrong.push("a piece carries no aria-label");
        if (p.getAttribute("tabindex") !== "0") wrong.push("a piece is not focusable");
      }
      t.diagnostic(`${year.label} ${WIDEST}: ${rows.length} row(s), ${pieces.length} piece(s); widest label ` +
        `${widestLabel.toFixed(0)}px of the ${app.BAR_LABELS - 8}px left of the bars`);
      assert.deepEqual(wrong.slice(0, 5), []);
      assert.ok(widestLabel <= app.BAR_LABELS - 8);

      // CITED AS A RIBBON IS: a piece pins its facts and pages; a bar pins
      // its share, in the page's words, and the document's sources.
      const piece = pieces[0].__data__;
      app.pin(piece);
      const panel = document.getElementById("detail");
      assert.equal(panel.querySelector(".amount").textContent, dollars(piece.value_cents));
      assert.ok(panel.textContent.includes("Facts: " + piece.fact_ids.join(" ")), panel.textContent);
      assert.ok(panel.querySelectorAll("a").length >= piece.locators.length);
      const bar = rows[1].__data__;
      app.pin(bar);
      const chips = [...panel.querySelectorAll(".chip.derived")].map((c) => c.textContent);
      const share = app.say("bars_share", { share: app.barPercent(bar, app.laidNodes), whole: rows[0].__data__.label });
      assert.ok(chips.includes(share), JSON.stringify(chips));
      assert.equal(panel.querySelector(".amount").textContent, dollars(app.BARS.cents(bar)));
      app.showTip({ target: chart, clientX: 0, clientY: 0 }, bar);
      assert.ok(document.getElementById("tooltip").textContent.includes(share));
    });
  });
}

describe("a list, read and worked by the reader", () => {
  test("the group's centre offers the list, Back leaves it, and nothing on it opens further", async () => {
    const { app, document } = await bootedApp();
    await opened(app, WIDEST);
    const centre = app.projection.nodes.find((n) => n.id === WIDEST);
    assert.equal((app.stepFor(centre) || {}).key, STEP.key);
    await opened(app, WIDEST);
    assert.equal(app.drilled.length, 2);
    assert.deepEqual(app.activeTiers(), [STEP.from, STEP.bars.tier]);
    assert.deepEqual(app.projection.nodes.filter(app.drillable), []);
    const back = [...document.querySelectorAll("#breadcrumb button.crumb-back")].map((b) => b.textContent);
    assert.deepEqual(back, [GROUP_STEP.back, STEP.back].map((b) => app.say("back_control", { back: b })));
    document.querySelectorAll("#breadcrumb button.crumb-back")[1].click();
    await settle();
    assert.equal(app.drilled.length, 1);
    assert.equal(app.formOnScreen(), app.SANKEY);
  });

  test("a click, Space and Enter on a bar follow its money, and search finds a bar and brings it into a zoomed view", async (t) => {
    const { app, document } = await bootedApp();
    await opened(app, WIDEST);
    await opened(app, WIDEST);
    const chart = document.getElementById("chart");
    const rows = [...chart.querySelectorAll("g.node")];
    const last = rows[rows.length - 1];
    const d = last.__data__;
    fire(last, "click", { timeStamp: 1000 });
    assert.equal(app.filter.isolated, d.id);
    const dimRows = rows.filter((g) => g.classList.contains("dim")).map((g) => g.__data__.id);
    const dimPieces = [...chart.querySelectorAll("path.link")].filter((p) => p.classList.contains("dim"));
    // Every other bar dims; the bar and the group's own row do not.
    assert.deepEqual(dimRows.sort(), rows.slice(1).map((g) => g.__data__.id).filter((id) => id !== d.id).sort());
    assert.equal(dimPieces.length, app.projection.links.length - d.sourceLinks.concat(d.targetLinks).length);
    assert.equal(last.getAttribute("aria-pressed"), "true");
    fire(last, "click", { timeStamp: 5000 });
    assert.equal(app.filter.isolated, "");
    fire(last, "keydown", { key: "Enter", timeStamp: 9000 });
    await settle();
    assert.equal(app.filter.isolated, d.id, "Enter on a bar that opens nothing follows it");
    assert.equal(app.drilled.length, 2);
    app.setIsolated("");

    app.zoomBy(4);
    const before = app.zoomOf();
    assert.ok(app.chooseSearch(d.id, false));
    const after = app.zoomOf();
    const [cx, cy] = after.apply([(d.x0 + d.x1) / 2, (d.y0 + d.y1) / 2]);
    const box = String(chart.getAttribute("viewBox")).split(/\s+/).map(Number);
    t.diagnostic(`the last bar, ${d.id}, at ${before.k}x zoom: centred at (${cx.toFixed(0)}, ${cy.toFixed(0)}) of a ${box[2]}x${box[3]} view`);
    assert.equal(app.filter.isolated, d.id);
    assert.ok(cx >= 0 && cx <= box[2] && cy >= 0 && cy <= box[3], "search left the bar out of sight");
    assert.equal(document.activeElement, chart.querySelector("g.node[aria-pressed=true]"));
  });

  test("the filters remove and merge pieces, and a bar's figure stays the list's", async (t) => {
    const { app, document } = await bootedApp();
    await opened(app, WIDEST);
    await opened(app, WIDEST);
    const total = app.BARS.cents(app.laidNodes[0]);
    assert.ok(app.setFilter({ kinds: new Set(["internal_transfer"]) }));
    const hidden = app.hiddenTally();
    const drawn = app.laidNodes.filter((n) => n.layer === 1).reduce((s, n) => s + app.BARS.cents(n), 0);
    const head = app.laidNodes[0];
    t.diagnostic(`internal transfers hidden: ${hidden.flows} piece(s), ${hidden.cents} cents; ${drawn} drawn of ${total}`);
    assert.ok(hidden.flows > 0, "no piece is an internal transfer, so the filter removed nothing");
    assert.equal(drawn + hidden.cents, total);
    assert.equal(app.markFigure(head), total, "the group's row moved with the filter");
    app.pin(head);
    assert.ok(document.getElementById("detail").textContent.includes(app.say("filter_drawn_at", { cents: app.fmtSigned(drawn) })));
    assert.ok(app.setFilter({ kinds: new Set(), smallest: 100000000 }));
    const smaller = app.laidNodes.filter((n) => n.role === "smaller");
    const merged = app.laidNodes.filter((n) => n.layer === 1).reduce((s, n) => s + app.BARS.cents(n), 0);
    t.diagnostic(`flows under $1,000,000 merged into ${smaller.length} row(s); ${merged} drawn of ${total}`);
    assert.equal(smaller.length, 1);
    assert.equal(merged, total);
    assert.deepEqual(refusals(document), []);
  });

  test("only the bars renderer reads a step's bars hints", async (t) => {
    const config = structuredClone(CONFIG);
    const reads = [];
    const step = stepByKey(config, STEP.key);
    step.bars = new Proxy(step.bars, {
      get(target, prop) {
        reads.push({ prop: String(prop), frame: ((new Error().stack || "").split("\n")[2] || "").trim() });
        return target[prop];
      },
    });
    const { app } = await bootedApp({ config });
    await opened(app, WIDEST);
    await opened(app, WIDEST);
    t.diagnostic(`${reads.length} bars hint read(s)`);
    assert.ok(reads.length > 0, "the list read no hint, so this proves nothing");
    assert.deepEqual([...new Set(reads.filter((r) => !r.frame.includes("site/bars.js")).map((r) => `${r.prop} at ${r.frame}`))], []);
  });
});

describe("what the bars form refuses", () => {
  test("an overview, a node with nothing to list, a negative link and a part joined both ways", async () => {
    const { app } = await loadApp();
    const doc = app.scheduleOf(columnFixture("fy2026-adopted"), "fund-flows");
    const rung = { id: WIDEST, step: STEP, doc, chart: doc };
    assert.throws(() => app.BARS.shape(doc, null, null, []), /the overview has opened none/);
    assert.throws(() => app.BARS.shape(doc, Object.assign({}, rung, { id: "fund/100" }), null, []), /joins it to nothing/);
    const at = doc.links.findIndex((l) => l.source === WIDEST);
    const negative = Object.assign({}, doc, { links: doc.links.map((l, i) => (i === at ? Object.assign({}, l, { value_cents: -l.value_cents }) : l)) });
    assert.throws(() => app.barsOf(negative, WIDEST, STEP.bars.tier), /negative figure/);
    const back = Object.assign({}, doc.links[at], { source: doc.links[at].target, target: WIDEST });
    assert.throws(() => app.barsOf(Object.assign({}, doc, { links: doc.links.concat([back]) }), WIDEST, STEP.bars.tier), /both directions/);
    assert.equal(app.BARS.shape(doc, rung, null, []).links.length > 0, true);
  });

  test("a column whose list the form refuses leaves the window on screen and says why", async () => {
    const column = columnFixture("fy2027-adopted");
    const groupAt = column.nodes.findIndex((n) => n.id === WIDEST);
    const link = column.schedules["fund-flows"].links.find((l) => l.from === groupAt);
    link.value_cents = -link.value_cents;
    const { app, document } = await bootedApp({ plan: { "fy2027-adopted.json": { doc: column } } });
    await opened(app, WIDEST);
    const before = app.projection;
    const got = await app.drillDown(WIDEST);
    await settle();
    assert.equal(got, "failed");
    assert.equal(app.projection, before);
    assert.equal(app.drilled.length, 1);
    assert.match(refusals(document)[0].textContent, /negative figure/);
  });
});
