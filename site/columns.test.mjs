// columns.test.mjs — the column budget: what the viewport gives, what the
// reader chooses, what the control describes, and what a widened step draws.
//
// The fold is the client's fitting step (AGENTS.md, "Go vets, JavaScript
// renders"): DrillStep.Widen says which columns a fourth buys and the client
// decides, from the viewport and the reader's choice, whether to draw it.

import { describe, test } from "node:test";
import assert from "node:assert/strict";

import {
  bootedApp, opened, settle, pageFixture, rungsFixture, refusals,
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
    for (const path of [[], ["fund-group/general"]]) {
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
  /** The fund-group step widened by `into`, over a copy of the pinned config. */
  function widenFundGroup(into = [4]) {
    const config = structuredClone(pageFixture().config);
    config.steps = config.steps.map((s) => (s.key === "fund-group" ? Object.assign({}, s, { tiers: s.tiers.concat(into), widen: into.slice() }) : s));
    return config;
  }
  /** The same widening in Go's answer: the General Fund's own nodes at each widened tier on its group's rung, read off the fund step's answer, and nothing on the other groups'. */
  function widenFundGroupAnswer(into = [4]) {
    const answer = rungsFixture();
    for (const column of answer.columns) {
      const fund = column.rungs.find((r) => r.path.join("|") === "fund-group/general|fund/100");
      assert.ok(fund, `${column.stem} answers no rung for fund-group/general > fund/100`);
      for (const rung of column.rungs) {
        if (rung.step !== "fund-group") continue;
        for (const tier of into) {
          const at = fund.draws.find((d) => d.tier === tier);
          assert.ok(at, `${column.stem}'s fund/100 rung draws no tier ${tier}`);
          rung.draws.push({ tier: tier, role: "outward", ids: rung.path[0] === "fund-group/general" ? at.ids.slice() : [] });
        }
        delete rung.marks;
      }
    }
    return answer;
  }
  test("a widened column the document leaves empty is dropped, and the chart is re-laid at the columns it has", async (t) => {
    const worst = "fund-group/special-revenue";
    const empty = await windowAt("sankey", 4, [worst], widenFundGroup(), widenFundGroupAnswer());
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
    const filled = await windowAt("sankey", 4, ["fund-group/general"], widenFundGroup(), widenFundGroupAnswer());
    t.diagnostic(`fund-group/general widened to tier 4 draws ${filled.columns} column(s) at {${filled.tiers}}: ${filled.nodes} nodes, ${filled.links} links, bands ${filled.bands}, ${filled.banners} banner(s)`);
    assert.equal(filled.columns, 4);
    assert.equal(filled.tiers, "0,2,3,4");
    assert.equal(filled.banners, 0);
    assert.equal(filled.bands.split("/").length, 3);
    assert.ok(filled.bands.split("/").every((b) => Number(b) > 0));
  });
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
