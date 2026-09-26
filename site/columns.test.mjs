// columns.test.mjs — the column budget: what the viewport gives, what the
// reader chooses, what the control describes, and what a widened step draws.
//
// The fold is the client's fitting step (AGENTS.md, "Go vets, JavaScript
// renders"): DrillStep.Widen says which columns a fourth buys and the client
// decides, from the viewport and the reader's choice, whether to draw it.

import { describe, test } from "node:test";
import assert from "node:assert/strict";

import {
  bootedApp, opened, settle, pageFixture, rungsFixture, columnFixture, refusals,
} from "./testlib.mjs";

const disabled = (b) => !b || b.hasAttribute("disabled");

describe("the viewport, the reader and the stored choice", () => {
  test("a viewport that can carry four columns gets four, and a reader's step down holds against it", async (t) => {
    // ON THE FUND WINDOW, where the control is live: on the overview both
    // steppers are disabled and a browser ignores a click on a disabled button.
    const { app, document, window, media } = await bootedApp({ checkedStem: "sankey", viewport: 2000 });
    await opened(app, "fund-group/general", "fund/100");
    const more = document.getElementById("column-more");
    const fewer = document.getElementById("column-fewer");
    const openedAt = app.columnBudget;
    const atCeiling = [disabled(more), disabled(fewer)].join("/");
    fewer.click();
    await settle();
    const chosen = app.columnBudget;
    const saved = window.localStorage.getItem("fisc-columns");
    const atFloor = [disabled(more), disabled(fewer)].join("/");
    media.setViewport(800);
    await settle();
    const narrowed = app.columnBudget;
    media.setViewport(2000);
    await settle();
    const held = app.columnBudget;
    more.click();
    await settle();
    const released = app.columnOverride;
    const cleared = window.localStorage.getItem("fisc-columns");
    t.diagnostic(`a 2000px window opens the fund window at ${openedAt} columns with (more/fewer) disabled ${atCeiling}; one press of the minus gives ${chosen}, stored as ${JSON.stringify(saved)}, with disabled ${atFloor}; narrowing to 800px leaves ${narrowed} and widening back leaves ${held}; stepping back up clears the override to ${released} and the key to ${JSON.stringify(cleared)}`);
    assert.equal(openedAt, 4);
    assert.equal(atCeiling, "true/false");
    assert.equal(chosen, 3);
    assert.equal(saved, "3");
    assert.equal(atFloor, "false/true");
    assert.equal(narrowed, 3);
    assert.equal(held, 3);
    assert.equal(released, null);
    assert.equal(cleared, null);
  });
  test("the page hands the stylesheet the width its widest chart is laid out at", async (t) => {
    const { app, document } = await bootedApp();
    const handed = document.documentElement.style.getPropertyValue("--chart-max");
    t.diagnostic(`boot set --chart-max to ${JSON.stringify(handed)}; this page offers ${app.OFFERED_COLUMNS} columns (floor ${app.NARROW_COLUMNS}) and lays them out at ${app.chartWidth(app.OFFERED_COLUMNS)}px`);
    assert.ok(app.OFFERED_COLUMNS > app.NARROW_COLUMNS);
    assert.equal(handed, app.chartWidth(app.OFFERED_COLUMNS) + "px");
  });
  test("a saved column count is honoured on the next visit, and one out of range is discarded", async (t) => {
    const saved = await bootedApp({ storage: { "fisc-columns": "4" } });
    const stale = await bootedApp({ storage: { "fisc-columns": "9" } });
    t.diagnostic(`a stored "4" opens at ${saved.app.columnBudget} columns (override ${saved.app.columnOverride}); a stored "9" opens at ${stale.app.columnBudget} with override ${stale.app.columnOverride}`);
    assert.equal(saved.app.columnBudget, 4);
    assert.equal(saved.app.columnOverride, 4);
    assert.equal(stale.app.columnBudget, 3);
    assert.equal(stale.app.columnOverride, null);
  });
  test("a budget change redraws the rung the reader is on without popping it or refetching", async (t) => {
    const { app, document, fetch } = await bootedApp({ checkedStem: "sankey" });
    await opened(app, "fund-group/general", "fund/100");
    const innermostControl = () => {
      const buttons = [...document.querySelectorAll("#breadcrumb button")];
      return buttons.length ? buttons[buttons.length - 1].textContent : "";
    };
    const wasOn = innermostControl();
    // Focus on a drawn mark, which is the state restoreFocus exists for.
    document.querySelector("#chart g.node").focus();
    assert.ok(document.getElementById("chart").contains(document.activeElement), "a mark did not take focus");
    const before = { depth: app.drilled.length, path: app.drilled.map((r) => r.id).join(" > "), columns: app.drawnColumns(), asked: fetch.asked.length };
    document.getElementById("column-more").click();
    await settle();
    const after = { depth: app.drilled.length, path: app.drilled.map((r) => r.id).join(" > "), columns: app.drawnColumns(), asked: fetch.asked.length };
    const focused = document.activeElement ? document.activeElement.textContent : "";
    t.diagnostic(`the fund window went from ${before.columns} to ${after.columns} columns; the stack reads "${after.path}" at depth ${after.depth}; ${after.asked} fetch(es) against ${before.asked}; ${refusals(document).length} banner(s); focus is on "${focused}" (was "${wasOn}")`);
    assert.equal(before.columns, 3);
    assert.equal(after.columns, 4);
    assert.equal(after.depth, before.depth);
    assert.equal(after.path, before.path);
    assert.equal(after.asked, before.asked);
    assert.equal(refusals(document).length, 0);
    assert.ok(wasOn !== "");
    assert.equal(focused, wasOn);
  });
});

describe("the column control describes the chart on screen", () => {
  const read = (app, document) => ({
    label: document.getElementById("column-count").textContent,
    drawn: app.drawnColumns(),
    marks: app.projection ? app.projection.nodes.length : 0,
    more: disabled(document.getElementById("column-more")),
    fewer: disabled(document.getElementById("column-fewer")),
  });
  test("a chart with one width offers no step, and says the width it has", async (t) => {
    const quiet = [];
    // THE REVENUE CATEGORY, NOT THE FUND GROUP: the fund-group step widens
    // (fisc-84y5), so its window has a second width and is the other test's.
    for (const path of [[], ["revenue/taxes/property"]]) {
      const { app, document } = await bootedApp({ checkedStem: "sankey", viewport: 2000 });
      await opened(app, ...path);
      quiet.push({ where: path.length ? path[path.length - 1] : "(the overview)", ...read(app, document) });
    }
    t.diagnostic(quiet.map((q) => `${q.where}: "${q.label}" over ${q.drawn} drawn column(s), + ${q.more ? "disabled" : "LIVE"} and - ${q.fewer ? "disabled" : "LIVE"}`).join("; "));
    for (const q of quiet) {
      assert.ok(q.more && q.fewer, q.where);
      assert.equal(q.label, q.drawn + " columns", q.where);
    }
  });
  test("a chart with a second width offers it, takes it, and reports what it drew", async (t) => {
    const { app, document } = await bootedApp({ checkedStem: "sankey", viewport: 1440 });
    await opened(app, "fund-group/general", "fund/100");
    const was = read(app, document);
    document.getElementById("column-more").click();
    await settle();
    const now = read(app, document);
    t.diagnostic(`fund/100 opened at "${was.label}" with + ${was.more ? "disabled" : "live"}; one press drew ${was.drawn} -> ${now.drawn} column(s) and ${was.marks} -> ${now.marks} mark(s); the count now reads "${now.label}" with + ${now.more ? "disabled" : "still live"}`);
    assert.ok(!was.more);
    assert.equal(was.label, was.drawn + " columns");
    assert.equal(now.drawn, was.drawn + 1);
    assert.ok(now.marks > was.marks);
    assert.equal(now.label, now.drawn + " columns");
    assert.ok(now.more);
  });
});

describe("a budget whose chart will not draw", () => {
  test("leaves the budget and the control on the chart that stayed", async (t) => {
    // Links into one fourth-column node with their fact_ids removed: the
    // three-column window never reads them, and the four-column one throws.
    const broken = structuredClone(columnFixture("fy2026-adopted"));
    const into = broken.nodes.findIndex((n) => n.id === "expenditure/engineering/wages-and-benefits");
    const cut = broken.schedules["fund-flows"].links.filter((l) => l.to === into);
    for (const l of cut) delete l.fact_ids;
    const { app, document, window } = await bootedApp({ checkedStem: "sankey", viewport: 800, plan: { "fy2026-adopted.json": { doc: broken } } });
    await opened(app, "fund-group/general", "fund/100");
    const nodes = app.projection.nodes.length;
    const choice = () => ({ override: app.columnOverride, stored: window.localStorage.getItem("fisc-columns") });
    const chosen = choice();
    document.getElementById("column-more").click();
    await settle();
    const label = document.getElementById("column-count").textContent;
    const banners = refusals(document).map((b) => b.textContent);
    t.diagnostic(`${cut.length} link(s) cut; after + the budget is ${app.columnBudget}, the count reads "${label}", ${app.projection.nodes.length} node(s) drawn against ${nodes}, banners ${JSON.stringify(banners)}; the reader's choice ${JSON.stringify(choice())} against ${JSON.stringify(chosen)} before`);
    assert.deepEqual(choice(), chosen);
    assert.ok(cut.length > 0);
    assert.equal(banners.length, 1);
    assert.equal(app.projection.nodes.length, nodes);
    assert.equal(app.columnBudget, 3);
    assert.equal(app.drawnColumns(), 3);
    assert.equal(label, "3 columns");
  });
});

describe("a widened step", () => {
  /** One window at a stated budget: its shape, read off the chart. */
  async function windowAt(stem, budget, path, config, answer) {
    const o = { checkedStem: stem };
    if (config) o.config = config;
    if (answer) o.plan = { "rungs.json": { doc: answer } };
    const { app, document } = await bootedApp(o);
    app.setColumnBudget(budget);
    await opened(app, ...path);
    const laid = app.layOut(app.projection);
    const tiers = app.activeTiers();
    const tierOf = (id) => (app.projection.nodes.find((n) => n.id === id) || { tier: -1 }).tier;
    const tail = app.projection.nodes.find((n) => app.isAggregate(n.id) && n.tier === 5);
    return {
      tiers: tiers.join(","),
      columns: app.drawnColumns(),
      right: Math.max(...laid.nodes.map((n) => n.x1)),
      banners: refusals(document).length,
      nodes: app.projection.nodes.length,
      links: app.projection.links.length,
      hairlines: laid.links.filter((l) => l.width < 1).length,
      tail: tail ? tail.label : "",
      bands: tiers.slice(1).map((to, k) => app.projection.links.filter((l) => tierOf(l.source) === tiers[k] && tierOf(l.target) === to).length).join("/"),
      gutter: app.LABEL_GUTTER, width: (n) => app.chartWidth(n),
    };
  }
  for (const stem of ["sankey", "sankey-2027"]) {
    test(`${stem}: the fund window is three columns at the budget every reader gets, and four where there is room`, async (t) => {
      const path = ["fund-group/general", "fund/100"];
      const narrow = await windowAt(stem, 3, path);
      const wide = await windowAt(stem, 4, path);
      t.diagnostic(`narrow: ${narrow.columns} column(s) at {${narrow.tiers}}, ${narrow.nodes} nodes, ${narrow.links} links, ${narrow.hairlines} sub-pixel, bands ${narrow.bands}, right edge ${narrow.right}px; wide: ${wide.columns} at {${wide.tiers}}, ${wide.nodes}/${wide.links}/${wide.hairlines}, bands ${wide.bands}, tail "${wide.tail}", right edge ${wide.right}px`);
      assert.equal(narrow.columns, 3);
      assert.equal(narrow.tiers, "2,3,4");
      assert.equal(narrow.banners, 0);
      assert.equal(narrow.right, narrow.width(3) - narrow.gutter);
      assert.equal(wide.columns, 4);
      assert.equal(wide.tiers, "2,3,4,5");
      assert.equal(wide.banners, 0);
      assert.ok(wide.nodes > narrow.nodes && wide.links > narrow.links);
      assert.equal(wide.right, wide.width(4) - wide.gutter);
      assert.match(wide.tail, /smaller/);
      assert.equal(wide.bands.split("/").length, 3);
    });
  }
  /**
   * The fund-group step widened to exactly `into` beyond its window's own
   * three columns, over a copy of the pinned config: the shipped widening is
   * replaced, not extended.
   */
  function widenFundGroup(into) {
    const config = structuredClone(pageFixture().config);
    config.steps = config.steps.map((s) => {
      if (s.key !== "fund-group") return s;
      const own = s.tiers.filter((t) => !(s.widen || []).includes(t));
      return Object.assign({}, s, { tiers: own.concat(into), widen: into.slice() });
    });
    return config;
  }
  /**
   * The same widening in Go's answer: the General Fund's own nodes at each
   * widened tier on its group's rung, read off the fund step's answer, and
   * nothing on the other groups'. A tier the shipped answer already draws is
   * left as answered.
   */
  function widenFundGroupAnswer(into) {
    const answer = rungsFixture();
    for (const column of answer.columns) {
      const fund = column.rungs.find((r) => r.path.join("|") === "fund-group/general|fund/100");
      assert.ok(fund, `${column.stem} answers no rung for fund-group/general > fund/100`);
      for (const rung of column.rungs) {
        if (rung.step !== "fund-group") continue;
        for (const tier of into) {
          if (rung.draws.some((d) => d.tier === tier)) continue;
          const at = fund.draws.find((d) => d.tier === tier);
          assert.ok(at, `${column.stem}'s fund/100 rung draws no tier ${tier}`);
          rung.draws.push({ tier: tier, role: "outward", ids: rung.path[0] === "fund-group/general" ? at.ids.slice() : [] });
        }
        delete rung.marks;
      }
    }
    return answer;
  }
  // THE FUND-GROUP STEP SHIPS WIDENED TO TIER 4 (fisc-84y5), so these two
  // drive the pinned config and Go's own answer rather than a synthetic pair.
  test("a widened column the document leaves empty is dropped, and the chart is re-laid at the columns it has", async (t) => {
    const worst = "fund-group/special-revenue";
    const empty = await windowAt("sankey", 4, [worst]);
    const asShipped = await windowAt("sankey", 3, [worst]);
    t.diagnostic(`${worst} widened to tier 4 draws ${empty.columns} column(s) at {${empty.tiers}}: ${empty.nodes} nodes, ${empty.links} links, bands ${empty.bands}, right edge ${empty.right}px, ${empty.banners} banner(s); the shipped step draws ${asShipped.nodes}/${asShipped.links}, bands ${asShipped.bands}`);
    assert.equal(empty.columns, 3);
    assert.equal(empty.tiers, "0,2,3");
    assert.equal(empty.banners, 0);
    assert.equal(empty.right, empty.width(3) - empty.gutter);
    assert.equal(empty.nodes, asShipped.nodes);
    assert.equal(empty.links, asShipped.links);
    assert.equal(empty.bands, asShipped.bands);
  });
  test("the same widened step keeps its fourth column on the one group whose document fills it", async (t) => {
    const filled = await windowAt("sankey", 4, ["fund-group/general"]);
    t.diagnostic(`fund-group/general widened to tier 4 draws ${filled.columns} column(s) at {${filled.tiers}}: ${filled.nodes} nodes, ${filled.links} links, bands ${filled.bands}, ${filled.banners} banner(s)`);
    assert.equal(filled.columns, 4);
    assert.equal(filled.tiers, "0,2,3,4");
    assert.equal(filled.banners, 0);
    assert.equal(filled.bands.split("/").length, 3);
    assert.ok(filled.bands.split("/").every((b) => Number(b) > 0));
  });
  // A LEAVING LEG GOES WITH THE COLUMN ITS ENDPOINT STANDS IN. The General
  // Fund's residual carries flows out of the group as well as into it, and Go
  // stands the leaving endpoints at the step's last tier -- the widened one.
  for (const stem of ["sankey", "sankey-2027"]) {
    test(`${stem}: the residual's leaving leg is drawn in the fourth column and dropped with it`, async (t) => {
      const legs = [];
      /** @type {Record<number, {note: string, drawn: number, leaving: number}>} */
      const at = {};
      for (const budget of [3, 4]) {
        const { app, document } = await bootedApp({ checkedStem: stem });
        app.setColumnBudget(budget);
        await opened(app, "fund-group/general");
        const laid = app.layOut(app.projection);
        const residual = app.projection.nodes.find((n) => app.isResidual(n.id));
        assert.ok(residual, "no residual on the General Fund's window");
        const tierOf = new Map(app.projection.nodes.map((n) => [n.id, n.tier]));
        const arriving = app.projection.links.filter((l) => l.target === residual.id);
        const leaving = app.projection.links.filter((l) => l.source === residual.id);
        const flat = laid.links.filter((l) => l.source.x0 === l.target.x0).length;
        const last = app.activeTiers()[app.activeTiers().length - 1];
        legs.push(`budget ${budget}: {${app.activeTiers()}}, ${arriving.length} arriving, ${leaving.length} leaving to ` +
          `${JSON.stringify(leaving.map((l) => `${l.target}@${tierOf.get(l.target)}`))}, ${flat} ribbon(s) inside one column`);
        assert.ok(arriving.length > 0);
        assert.equal(flat, 0);
        // AND THE NOTE SAYS WHICH OF THE MARK'S FLOWS THIS WIDTH DRAWS, since
        // Go's rationale names every endpoint at every width.
        const drawnFlows = arriving.length + leaving.length;
        at[budget] = { note: residual.source_note, drawn: drawnFlows, leaving: leaving.length };
        assert.ok(residual.source_note.startsWith(
          `Carried, not computed: ${drawnFlows} ${drawnFlows === 1 ? "flow" : "flows"} of the chart above`), residual.source_note);
        if (budget === 3) {
          assert.equal(leaving.length, 0);
          assert.ok(!app.projection.nodes.some((n) => n.id === "transfers/out"));
        } else {
          assert.ok(leaving.length > 0);
          for (const l of leaving) assert.equal(tierOf.get(l.target), last);
          assert.doesNotMatch(residual.source_note, /where there is room/);
        }
      }
      // THE FLOWS HELD BACK AT THREE COLUMNS ARE THE ONES FOUR DRAW, counted,
      // with the verb agreeing with the count.
      const held = at[4].leaving;
      assert.ok(at[3].note.endsWith(
        (held === 1 ? " The flow leaving it is" : ` The ${held} flows leaving it are`) +
        " drawn where there is room for a further column."), at[3].note);
      assert.equal(at[3].drawn + held, at[4].drawn);
      legs.push(`note at 3: "${at[3].note}"`);
      t.diagnostic(legs.join("; "));
    });
  }
  // THE FIGURES ARE GO'S AT EVERY WIDTH, formatted here without the page's
  // formatter, against the cents the answer carries.
  const dollars = (cents) => new Intl.NumberFormat("en-US",
    { style: "currency", currency: "USD", maximumFractionDigits: 0 }).format(cents / 100);
  for (const stem of ["sankey", "sankey-2027"]) {
    test(`${stem}: the residual states Go's figures in and out whether or not its leaving legs are drawn`, async (t) => {
      const mark = rungsFixture().columns.find((c) => c.stem === stem).rungs
        .find((r) => r.path.join("|") === "fund-group/general").marks.find((m) => m.role === "residual");
      assert.ok(mark.in_cents > 0 && mark.out_cents > 0, "the General Fund's residual is not answered both ways");
      const said = {};
      const shares = {};
      for (const budget of [3, 4]) {
        const { app, document } = await bootedApp({ checkedStem: stem });
        app.setColumnBudget(budget);
        await opened(app, "fund-group/general");
        const chart = document.getElementById("chart");
        const g = [...chart.querySelectorAll("g.node")].find((m) => m.__data__.id === mark.id);
        assert.ok(g, `${mark.id} is not drawn at ${budget} columns`);
        app.showTip({ target: chart, clientX: 0, clientY: 0 }, g.__data__);
        const tip = document.getElementById("tooltip");
        app.pin(g.__data__);
        const panel = document.getElementById("detail");
        // THE NOTE AND THE SHARE DIFFER BY WIDTH, saying which legs are held
        // back and how the drawn column divides; the figures do not.
        const words = (e) => e.textContent.replace(g.__data__.source_note, "").replace(app.columnShare(g.__data__), "");
        said[budget] = {
          label: g.textContent, aria: g.getAttribute("aria-label"),
          tipValue: tip.querySelector(".tip-value").textContent, tip: words(tip),
          amount: panel.querySelector(".amount").textContent, panel: words(panel),
        };
        // THE SHARE IS OF THE COLUMN AS DRAWN, read off the marks' heights.
        const height = (d) => d.y1 - d.y0;
        const column = [...chart.querySelectorAll("g.node")].map((m) => m.__data__).filter((d) => d.x0 === g.__data__.x0);
        const pct = (100 * height(g.__data__)) / column.reduce((a, d) => a + height(d), 0);
        shares[budget] = { share: app.columnShare(g.__data__), drawn: "\u25c7 our " + pct.toFixed(1) + "% of this column" };
      }
      const flows = `${dollars(mark.in_cents)} in, ${dollars(mark.out_cents)} out`;
      t.diagnostic(`Go answers ${mark.in_cents} in and ${mark.out_cents} out; at 3 columns "${said[3].aria}", ` +
        `share ${JSON.stringify(shares[3])}; at 4 "${said[4].aria}", share ${JSON.stringify(shares[4])}`);
      assert.deepEqual(said[3], said[4]);
      for (const budget of [3, 4]) assert.equal(shares[budget].share, shares[budget].drawn, `at ${budget} columns`);
      assert.equal(said[3].tipValue, dollars(Math.max(mark.in_cents, mark.out_cents)));
      assert.equal(said[3].amount, dollars(Math.max(mark.in_cents, mark.out_cents)));
      for (const where of ["aria", "tip", "panel"]) assert.ok(said[3][where].includes(flows), said[3][where]);
    });
  }
  // A RESIDUAL WITH NO LEAVING FLOW HOLDS NOTHING BACK AT ANY WIDTH: the
  // capital group's only draws on its balance, so its note says nothing about
  // flows leaving it.
  for (const stem of ["sankey", "sankey-2027"]) {
    test(`${stem}: a residual with no leaving flow says nothing is held back, at either width`, async (t) => {
      const notes = [];
      for (const budget of [3, 4]) {
        const { app } = await bootedApp({ checkedStem: stem });
        app.setColumnBudget(budget);
        await opened(app, "fund-group/capital");
        const residual = app.projection.nodes.find((n) => app.isResidual(n.id));
        assert.ok(residual, "no residual on the capital group's window");
        assert.equal(app.projection.links.filter((l) => l.source === residual.id).length, 0);
        notes.push(`budget ${budget}: "${residual.source_note}"`);
        assert.doesNotMatch(residual.source_note, /leaving it/);
      }
      t.diagnostic(notes.join("; "));
    });
  }

  // A CHILD WHOSE TIERS ARE A STRICT SUBSET OF ITS WIDENED PARENT'S IS DRAWN.
  // export.validateSteps refuses a step whose tiers EQUAL its parent's and
  // accepts a subset (fisc-ke1f); this is the client's half of that decision,
  // over the one pair that makes one: the fund-group step widened to the
  // whole fund-flows chain, under which the fund step's {2,3,4,5} is a subset
  // of the {0,2,3,4,5} on screen. The fund window is a narrower chart of one
  // fund and not a redraw, at every budget.
  test("a step whose tiers are a strict subset of its widened parent's opens at every budget", async (t) => {
    const path = ["fund-group/general", "fund/100"];
    const seen = [];
    for (const budget of [3, 4, 5]) {
      const config = widenFundGroup([4, 5]);
      const group = config.steps.find((s) => s.key === "fund-group");
      const fund = config.steps.find((s) => s.key === "fund");
      assert.ok(fund.tiers.every((tier) => group.tiers.includes(tier)) && fund.tiers.length < group.tiers.length);
      const at = await windowAt("sankey", budget, path, config, widenFundGroupAnswer([4, 5]));
      seen.push(`budget ${budget}: {${at.tiers}}, ${at.nodes} nodes, ${at.links} links, bands ${at.bands}, ${at.banners} banner(s)`);
      assert.equal(at.banners, 0);
      assert.equal(at.columns, Math.min(budget, 4));
      assert.equal(at.tiers, budget === 3 ? "2,3,4" : "2,3,4,5");
      assert.ok(at.bands.split("/").every((b) => Number(b) > 0), at.bands);
    }
    t.diagnostic(seen.join("; "));
  });
});

// fisc-7477: the storage arm, which some privacy modes reach by throwing on
// the read rather than answering null.
describe("a storage the browser refuses", () => {
  test("a localStorage that throws on read is a saved count of none", async () => {
    const { app } = await bootedApp({ storage: { "fisc-columns": "4" } });
    assert.equal(app.savedColumns(), 4);
    const prior = Object.getOwnPropertyDescriptor(globalThis, "localStorage");
    Object.defineProperty(globalThis, "localStorage", {
      configurable: true, writable: true,
      value: { getItem() { throw new Error("denied"); }, setItem() { throw new Error("denied"); } },
    });
    try {
      assert.equal(app.savedColumns(), null);
    } finally {
      Object.defineProperty(globalThis, "localStorage", prior);
    }
  });
});
