// What site/department.js does for department.html, over the page Go pins
// (testdata/department.golden.html) and the columns it fetches
// (testdata/<stem>.column.json). The module is imported with nothing on the
// global, so one that read a global or held state at import would fail first.
import { test, describe } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";
import { columnFixture, installBrowser, plannedFetch, repoRoot, settle, stylesheet } from "./testlib.mjs";

const HERE = path.dirname(fileURLToPath(import.meta.url));

delete globalThis.FISC_CONFIG;
delete globalThis.d3;
const domless = typeof globalThis.document === "undefined";
const mod = await import(pathToFileURL(path.join(HERE, "department.js")).href);
const { dollars } = await import(pathToFileURL(path.join(HERE, "format.js")).href);

/** The pinned page and the config cut out of it. */
function departmentPage() {
  const html = readFileSync(path.join(repoRoot, "testdata", "department.golden.html"), "utf8");
  const open = '<script type="application/json" id="department-config">';
  const i = html.indexOf(open);
  if (i < 0) throw new Error("the pinned page carries no department config");
  const rest = html.slice(i + open.length);
  return { html, config: JSON.parse(rest.slice(0, rest.indexOf("</script>"))) };
}

/** The columns pinned under testdata/, by served path less .json. */
const PINNED = ["fy2024-actual", "fy2026-adopted", "fy2027-adopted"];

/** Boots the pinned page at a hash, every pinned column planned, `over` replacing a stem's plan entry. */
async function booted(hash, over = {}) {
  const { html, config } = departmentPage();
  const browser = installBrowser({ html });
  // A fragment navigation fires popstate in its own task; settled before
  // boot, so the page's own listener never hears the test's navigation.
  if (hash) browser.window.location.hash = hash;
  await settle();
  const plan = {};
  for (const stem of PINNED) plan[stem + ".json"] = stem in over ? over[stem] : { doc: columnFixture(stem) };
  browser.window.fetch = plannedFetch(plan, config.exported_by);
  await mod.boot(browser.window);
  await settle();
  return { ...browser, config };
}

/** The bars drawn on one side, `funding` or `spending`, as plain values. */
function drawnOf(document, side) {
  return [...document.querySelectorAll(`section.department-side[data-side="${side}"] li.bar-row`)].map((li) => ({
    id: li.dataset.id, cents: Number(li.dataset.cents), label: li.querySelector(".bar-label").textContent,
    width: parseFloat(li.querySelector(".bar-fill").style.width), rank: li.dataset.rank,
    transfer: li.dataset.transfer === "true",
  }));
}

test("department.js imports with no page config, no d3 and no DOM, and exports only its functions", () => {
  assert.ok(domless, "nothing installed a DOM before the import");
  assert.deepEqual(Object.keys(mod).sort(),
    ["boot", "departmentBars", "departmentFigures", "departmentSides", "drawBars", "hashOf", "openBar", "parseHash",
      "readConfig", "render"]);
});

// The client's arithmetic is sums: every link of a department's two sides
// lands in exactly one row of one department, and every sum is its rows'.
//
// Mutation: keep only `expenditure/` links on the spending side in
// departmentFigures, and FY2023-24 public works' Transfers Out lands in no
// department.
describe("a department's two sides are sums of the links the column draws into it", () => {
  const { config } = departmentPage();
  for (const stem of PINNED) {
    test(`${stem}: every link lands in one department once, and each sum is its rows'`, (t) => {
      const column = columnFixture(stem);
      const nodes = column.nodes;
      const seen = { funding: new Map(), spending: new Map() };
      let transfers = 0;
      for (const d of config.departments) {
        const f = mod.departmentFigures(column, config, d.id);
        for (const [side, part] of [["funding", f.funding], ["spending", f.spending]]) {
          assert.equal(part.rows.reduce((a, r) => a + r.cents, 0), part.sum.cents, `${stem} ${d.id} ${side}`);
          assert.equal(new Set(part.rows.flatMap((r) => r.factIds)).size, part.sum.factIds.length, `${stem} ${d.id} ${side}`);
          for (const id of part.sum.factIds) seen[side].set(id, (seen[side].get(id) || 0) + 1);
        }
        transfers += f.spending.rows.filter((r) => r.transfer).length;
      }
      for (const side of ["funding", "spending"]) {
        const links = column.schedules[config[side]].links;
        const want = links.flatMap((l) => l.fact_ids);
        assert.equal(seen[side].size, new Set(want).size, `${stem}: ${side} facts reached`);
        for (const id of want) assert.equal(seen[side].get(id), 1, `${stem}: ${side} ${id} lands in one department`);
        assert.ok(links.every((l) => config.departments.some((d) => side === "funding"
          ? nodes[l.to].id === d.id : d.divisions.some((v) => v.id === nodes[l.to].id))),
        `${stem}: a ${side} link falls outside every department`);
      }
      t.diagnostic(`${stem}: ${seen.funding.size} funding and ${seen.spending.size} spending facts, each in one department; ${transfers} Transfers Out row(s)`);
      if (stem === "fy2024-actual") assert.ok(transfers > 0, "FY2023-24 prints a Transfers Out row and none was drawn");
    });
  }
});

// Whether a spending row is an expenditure is the kind Go declares on its
// links, never its id: each case is a pinned column less or altering `kind`.
//
// Mutation: restore `transfer: !r.id.startsWith("expenditure/")` in
// departmentFigures and the first two go red; drop the refusal with it and
// all five do.
describe("a spending row is a transfer because its links say so", () => {
  const { config } = departmentPage();
  const works = "department/public-works";
  /** A pinned column with every spending link from `id` edited. */
  const edited = (stem, id, edit) => {
    const column = structuredClone(columnFixture(stem));
    let n = 0;
    for (const l of column.schedules[config.spending].links) {
      if (column.nodes[l.from].id === id) { edit(l); n++; }
    }
    assert.ok(n > 0, `${stem} draws no spending link from ${id}`);
    return column;
  };
  const rowOf = (column, dept, id) => mod.departmentFigures(column, config, dept).spending.rows.find((r) => r.id === id);

  test("an expenditure/ row declared a transfer is drawn as not an expenditure", () => {
    const id = "expenditure/wages-and-benefits";
    assert.equal(rowOf(columnFixture("fy2026-adopted"), works, id).transfer, false);
    const column = edited("fy2026-adopted", id, (l) => { l.kind = "internal_transfer"; });
    assert.equal(rowOf(column, works, id).transfer, true, "the row read its id, not its kind");
    const { document } = installBrowser({ html: departmentPage().html });
    mod.render(document, config, column, { year: "fy2026-adopted.json", department: works });
    const heads = drawnOf(document, "spending").filter((b) => b.transfer).map((b) => b.label);
    assert.deepEqual(heads, ["Wages & Benefits (not an expenditure)"]);
    assert.equal(document.getElementById("transfers-says").hidden, false);
  });

  test("a transfers/ row declared external is drawn as an expenditure", () => {
    assert.equal(rowOf(columnFixture("fy2024-actual"), works, "transfers/out").transfer, true);
    const column = edited("fy2024-actual", "transfers/out", (l) => { l.kind = "external"; });
    assert.equal(rowOf(column, works, "transfers/out").transfer, false, "the row read its id, not its kind");
  });

  test("a spending row with no kind, or a kind the page does not name, is refused", () => {
    for (const [what, edit] of [
      ["no kind", (l) => { delete l.kind; }],
      ["a fund balance kind", (l) => { l.kind = "fund_balance"; }],
      ["an internal service kind", (l) => { l.kind = "internal_service"; }],
    ]) {
      const column = edited("fy2024-actual", "transfers/out", edit);
      assert.throws(() => mod.departmentFigures(column, config, works), /draws transfers\/out as /, what);
    }
  });

  test("a spending row whose links declare two kinds is refused", () => {
    const column = structuredClone(columnFixture("fy2026-adopted"));
    const links = column.schedules[config.spending].links;
    const id = "expenditure/wages-and-benefits";
    const divisions = config.departments.find((d) => d.id === works).divisions.map((d) => d.id);
    const mine = links.filter((l) => column.nodes[l.from].id === id && divisions.includes(column.nodes[l.to].id));
    assert.ok(mine.length > 1, "public works draws wages in fewer than two divisions");
    mine[0].kind = "internal_transfer";
    assert.throws(() => mod.departmentFigures(column, config, works), /draws expenditure\/wages-and-benefits as (external and internal_transfer|internal_transfer and external), /);
  });

  // Mutation: drop the sides check from activate's toggle, and the second
  // and third clicks on the refused department close and then open its bar
  // over police's sides.
  test("a department whose sides are refused opens no bar over another's sides, however often it is clicked", async () => {
    const { html } = departmentPage();
    const browser = installBrowser({ html });
    browser.window.location.hash = "#y=fy2024-actual&d=department/police-department";
    await settle();
    const plan = {};
    for (const stem of PINNED) plan[stem + ".json"] = { doc: columnFixture(stem) };
    plan["fy2024-actual.json"] = { doc: edited("fy2024-actual", "transfers/out", (l) => { delete l.kind; }) };
    browser.window.fetch = plannedFetch(plan, config.exported_by);
    await mod.boot(browser.window);
    await settle();
    const doc = browser.document;
    assert.match(doc.getElementById("department-heading").textContent, /^Police Department, /);
    const row = () => doc.querySelector(`#department-list > li[data-department="${works}"] .department-row`);
    const seen = [];
    for (let i = 0; i < 3; i++) {
      row().dispatchEvent(new browser.window.MouseEvent("click", { bubbles: true }));
      await settle();
      const open = row().getAttribute("aria-expanded") === "true";
      seen.push({ open, heading: doc.getElementById("department-heading").textContent.split(",")[0] });
    }
    assert.ok(doc.getElementById("department-refusal"), "the refused department was never said refused");
    assert.deepEqual(seen.filter((s) => s.open && s.heading !== "Public Works"), [], JSON.stringify(seen));
  });

  test("the reader is told the department could not be drawn", async () => {
    const { html } = departmentPage();
    const browser = installBrowser({ html });
    browser.window.location.hash = "#y=fy2024-actual&d=department/public-works";
    await settle();
    const plan = {};
    for (const stem of PINNED) plan[stem + ".json"] = { doc: columnFixture(stem) };
    plan["fy2024-actual.json"] = { doc: edited("fy2024-actual", "transfers/out", (l) => { delete l.kind; }) };
    browser.window.fetch = plannedFetch(plan, config.exported_by);
    await mod.boot(browser.window);
    await settle();
    const refusal = browser.document.getElementById("department-refusal");
    assert.ok(refusal, "no refusal was said");
    assert.match(refusal.textContent, /could not be drawn: .*transfers\/out/);
  });

  // Mutation: drop the catch's move of the view to barsOf's year in show,
  // and the click on a bar drawn from FY 2023-24 opens it in FY 2026-27;
  // drop clearSides from that catch and FY 2026-27's bars stay drawn.
  test("after a year switch draws the bars and refuses the sides, a click on a bar opens it in the bars' year", async () => {
    const { document, window } = await booted("#y=fy2027-adopted&d=" + works,
      { "fy2024-actual": { doc: edited("fy2024-actual", "transfers/out", (l) => { delete l.kind; }) } });
    const year = document.getElementById("year-select");
    year.value = "fy2024-actual";
    year.dispatchEvent(new window.Event("change"));
    await settle();
    const refused = document.getElementById("department-refusal") !== null;
    // The refused view's sides are cleared, not left as FY 2026-27's under a
    // year select naming FY 2023-24.
    assert.deepEqual({ funding: drawnOf(document, "funding").length, spending: drawnOf(document, "spending").length,
      heading: document.getElementById("department-heading").textContent },
    { funding: 0, spending: 0, heading: "This department could not be drawn" });
    document.querySelector('#department-list > li[data-department="department/police-department"] .department-row')
      .dispatchEvent(new window.MouseEvent("click", { bubbles: true }));
    await settle();
    assert.deepEqual({ refused, hash: window.location.hash, heading: document.getElementById("department-heading").textContent },
      { refused: true, hash: "#y=fy2024-actual&d=department/police-department", heading: "Police Department, FY 2023-24 actual" });
  });
});

describe("the page over the pinned columns", () => {
  test("the hash opens one department of one column, and the selects say which", async () => {
    const { document, window } = await booted("#y=fy2024-actual&d=department/public-works");
    assert.equal(document.getElementById("department-select").value, "department/public-works");
    assert.equal(document.getElementById("year-select").value, "fy2024-actual");
    assert.equal(window.location.hash, "#y=fy2024-actual&d=department/public-works");
    assert.equal(document.getElementById("department-heading").textContent, "Public Works, FY 2023-24 actual");
    assert.equal(document.getElementById("department-refusal"), null);
  });

  test("choosing a department records it in the hash", async () => {
    const { document, window, config } = await booted("#y=fy2026-adopted&d=department/public-works");
    const select = document.getElementById("department-select");
    select.value = "department/police-department";
    select.dispatchEvent(new window.Event("change"));
    await settle();
    assert.equal(window.location.hash, "#y=fy2026-adopted&d=department/police-department");
    assert.match(document.getElementById("department-heading").textContent, /^Police Department, /);
    assert.equal(mod.hashOf(mod.parseHash(window.location.hash, config)), window.location.hash);
  });

  test("a hash naming no department or column is refused in words and the opening view shown", async () => {
    const { document, config } = await booted("#y=fy1999-adopted&d=department/nope");
    const refusal = document.getElementById("department-refusal");
    assert.ok(refusal, "no refusal was said");
    assert.match(refusal.textContent, /fy1999-adopted/);
    assert.match(refusal.textContent, /department\/nope/);
    assert.equal(document.getElementById("year-select").value, config.years.at(-1).path.replace(/\.json$/, ""));
    const longest = mod.departmentBars(columnFixture(config.years.at(-1).path.replace(/\.json$/, "")), config).bars[0].id;
    assert.equal(document.getElementById("department-select").value, longest);
  });

  // Mutation: drop one row's tile in departmentSides, and each side's drawn
  // sum stops being the figures' sum: red, naming the side.
  for (const [stem, dept] of [["fy2024-actual", "department/public-works"], ["fy2026-adopted", "department/administrative-services"],
    ["fy2027-adopted", "department/police-department"]]) {
    test(`${stem} ${dept}: each side's drawn bars sum to the side, under a ◇ caption, on one scale, and nothing is called a total`, async (t) => {
      const { document, config } = await booted(`#y=${stem}&d=${dept}`);
      const f = mod.departmentFigures(columnFixture(stem), config, dept);
      const widest = [];
      for (const side of ["funding", "spending"]) {
        const bars = drawnOf(document, side);
        assert.ok(bars.length > 0, `${side}: no bar drawn`);
        assert.equal(bars.reduce((a, x) => a + x.cents, 0), f[side].sum.cents, `${side}: the drawn bars do not sum to the side`);
        assert.deepEqual(bars.map((x) => x.id), f[side].rows.map((r) => r.id), `${side}: a row drawn twice, or none`);
        const caption = document.querySelector(`section.department-side[data-side="${side}"] .sides-total`).textContent;
        assert.ok(caption.startsWith("◇ our sum of the ") && caption.endsWith(dollars(f[side].sum.cents)), `${side}: ${caption}`);
        assert.ok(!bars.some((x) => /total/i.test(x.label)), `${side} calls a bar a total`);
        widest.push(Math.max(...bars.map((x) => x.width)));
        t.diagnostic(`${side}: ${bars.length} bars summing to ${f[side].sum.cents} cents, widest ${widest.at(-1)}%`);
      }
      // One scale: the longest bar of either side fills its track, and the
      // other side's longest is drawn against it.
      assert.equal(Math.max(...widest), 100);
      const big = Math.max(...["funding", "spending"].flatMap((side) => f[side].rows.map((r) => Math.abs(r.cents))));
      for (const side of ["funding", "spending"]) {
        for (const x of drawnOf(document, side)) {
          assert.ok(Math.abs(x.width - Math.max(100 * Math.abs(x.cents) / big, 0.25)) < 1e-3, `${side} ${x.id}: ${x.width}% off the shared scale`);
        }
      }
    });
  }

  // Mutation: return an empty locators list from tileOf, and every tip's
  // pages are empty: red, naming the bar.
  test("every drawn bar carries the locators it sums, and its tip names their pages", async () => {
    const { document, window, config } = await booted("#y=fy2026-adopted&d=department/administrative-services");
    const f = mod.departmentFigures(columnFixture("fy2026-adopted"), config, "department/administrative-services");
    const want = new Map([...f.funding.rows, ...f.spending.rows].map((r) => [r.id, r.locators.flatMap((l) => l.pages.map((p) => `p${p}`))]));
    const rows = [...document.querySelectorAll("section.department-side li.bar-row")];
    assert.ok(rows.length > 0);
    for (const li of rows) {
      li.dispatchEvent(new window.Event("pointerenter"));
      const tip = document.getElementById("tooltip");
      const pages = tip.querySelector(".facts").textContent;
      const id = li.dataset.id;
      assert.ok(want.get(id).length > 0, `${id} cites no page`);
      assert.deepEqual(pages.split(/[,;] /), want.get(id), `${id}: the tip names other pages`);
      assert.equal(tip.hidden, false);
      li.dispatchEvent(new window.Event("pointerleave"));
    }
    // A fund's tip names its group.
    const fund = document.querySelector('section.department-side[data-side="funding"] li.bar-row');
    fund.dispatchEvent(new window.Event("pointerenter"));
    assert.match(document.getElementById("tooltip").textContent, /Fund group: /);
  });

  // A click on a bar pins its tip, whose page numbers link the page text
  // the config's docs name for the row's locators. Mutation: drop pageLinks
  // from sides.js's fillTip, and every bar's links are missing; drop
  // fillTip's call from department.js's fill, and the tip is empty.
  test("a click pins a bar's tip, linking the page text of every page its row cites", async (t) => {
    const { pinGestures, tipState, pageTextExists, fire } = await import("./testlib.mjs");
    const { document, window, config } = await booted("#y=fy2026-adopted&d=department/administrative-services");
    const f = mod.departmentFigures(columnFixture("fy2026-adopted"), config, "department/administrative-services");
    const want = new Map([...f.funding.rows, ...f.spending.rows].map((r) => [r.id,
      [...new Set(r.locators.flatMap((l) => l.pages.map((p) => config.docs[l.doc_id].pages[String(p)].text)))]]));
    const rows = [...document.querySelectorAll("section.department-side li.bar-row")];
    assert.ok(rows.length >= 2);
    for (const li of rows) {
      fire(li, "click");
      const tip = tipState(document);
      assert.ok(tip.pinned && tip.shown && tip.focused, `${li.dataset.id}: no pinned tip with the focus in it`);
      assert.ok(want.get(li.dataset.id).length > 0, `${li.dataset.id} cites no page`);
      assert.deepEqual(tip.hrefs, want.get(li.dataset.id), `${li.dataset.id}: the tip links other pages`);
      for (const href of tip.hrefs) assert.ok(pageTextExists(href), `${href} names no committed page text`);
    }
    fire(document.body, "click");
    pinGestures(window, rows[0], rows[1]);
    t.diagnostic(`${rows.length} bars pinned`);
  });

  test("whether the two sides tie is the config's declaration, printed and not computed", async () => {
    const held = await booted("#y=fy2024-actual&d=department/public-works");
    const tie = held.document.getElementById("department-tie");
    const year = held.config.years.find((y) => y.path === "fy2024-actual.json");
    const side = year.sides.find((s) => s.department === "department/public-works");
    assert.equal(side.ties, false, "the pinned config no longer holds FY2023-24 public works apart");
    assert.equal(tie.textContent, tie.dataset.held + side.reason);

    const ties = await booted("#y=fy2026-adopted&d=department/public-works");
    const tie2 = ties.document.getElementById("department-tie");
    assert.equal(tie2.textContent, tie2.dataset.ties);
  });

  // Mutation: drop the transfer mark in render, and the hatched arm goes
  // red; drop the label's suffix in departmentSides and the label arm does.
  test("Transfers Out is drawn as not an expenditure, inside the sum, with its own sentence", async () => {
    const { document, window } = await booted("#y=fy2024-actual&d=department/public-works");
    const bars = drawnOf(document, "spending");
    assert.deepEqual(bars.filter((x) => x.transfer).map((x) => [x.id, x.label, x.rank]),
      [["transfers/out", "Transfers Out (not an expenditure)", "0"]]);
    assert.ok(bars.filter((x) => !x.transfer).every((x) => !/not an expenditure/.test(x.label)));
    assert.equal(document.getElementById("transfers-says").hidden, false);
    const li = document.querySelector('section.department-side[data-side="spending"] li.bar-row[data-id="transfers/out"]');
    li.dispatchEvent(new window.Event("pointerenter"));
    assert.match(document.getElementById("tooltip").textContent, /Not an expenditure: money moved to another of the city's own funds/);

    const later = await booted("#y=fy2026-adopted&d=department/public-works");
    assert.equal(later.document.getElementById("transfers-says").hidden, true,
      "the Transfers Out sentence shows in a column with no such row");
    assert.deepEqual(drawnOf(later.document, "spending").filter((x) => x.transfer), []);
  });

  // Mutation: give every fund bar rank 0 in render, and the hue arm goes red.
  test("a fund the schedule calls internal service is marked so, and each fund's bar wears its group's hue", async () => {
    const { document, config } = await booted("#y=fy2026-adopted&d=department/administrative-services");
    const bars = drawnOf(document, "funding");
    assert.ok(bars.some((x) => x.label.endsWith(" (internal service)")), `no bar marked internal service: ${bars.map((x) => x.label).join(", ")}`);
    const { groups } = mod.departmentBars(columnFixture("fy2026-adopted"), config);
    const rows = mod.departmentFigures(columnFixture("fy2026-adopted"), config, "department/administrative-services").funding.rows;
    for (const x of bars) {
      const r = rows.find((y) => y.id === x.id);
      const g = groups.find((y) => (y.id ? y.id === r.groupId : y.members.includes(r.group)));
      assert.equal(x.rank, String(g.rank), `${x.id} wears another group's hue`);
    }
    assert.ok(bars.some((x) => x.rank !== "0"), "no fund bar wears a hue");
    assert.ok(drawnOf(document, "spending").every((x) => x.rank === "0"), "a spending bar wears a fund group's hue");
    assert.match(document.getElementById("internal-service-says").textContent, /charging its own\s+departments/);
    assert.match(document.getElementById("funding-says").textContent, /General Fund bar is the General Fund's share alone/);
    assert.match(document.getElementById("spending-says").textContent, /whole department's\s+cost/);
  });
});

/** The drawn bars, in page order, as ids, cents and their segments. */
function barsOf(document) {
  return [...document.querySelectorAll("#department-list > li")].map((li) => ({
    id: li.dataset.department,
    cents: Number(li.querySelector(".department-value").dataset.cents),
    expanded: li.querySelector(".department-row").getAttribute("aria-expanded"),
    hidden: li.querySelector(".department-detail").hidden,
    segs: [...li.querySelectorAll(".department-seg")].map((s) => ({ rank: s.dataset.rank, cents: Number(s.dataset.cents), title: s.title })),
    shares: [...li.querySelectorAll(".department-group-share")].map((s) => s.textContent),
  }));
}

// A department's bar is its funding side: one segment per legend entry it
// draws, their cents its sum, the bars longest first, each segment in the
// legend's order and wearing its rank.
//
// Mutation: drop the bars' sort in departmentBars and the descending arm
// goes red; reverse the segments' sort and the legend-order arm does; file a
// fund under whatever its parent names, fund group or not, and the refusal
// goes red; drop the ◇ from a drawn share and the drawn-bars test does.
describe("every department is a bar of the funds that pay for it", () => {
  const { config } = departmentPage();
  for (const stem of PINNED) {
    test(`${stem}: one bar per department, longest first, its segments summing to its funding side`, (t) => {
      const column = columnFixture(stem);
      const { groups, bars } = mod.departmentBars(column, config);
      assert.deepEqual(bars.map((b) => b.id).sort(), config.departments.map((d) => d.id).sort());
      for (let i = 1; i < bars.length; i++) assert.ok(bars[i - 1].cents >= bars[i].cents, `${stem}: ${bars[i].id} outranks ${bars[i - 1].id}`);
      for (const b of bars) {
        const f = mod.departmentFigures(column, config, b.id);
        assert.equal(b.cents, f.funding.sum.cents, `${stem} ${b.id}: the bar is not its funding side's sum`);
        assert.equal(b.segments.reduce((a, s) => a + s.cents, 0), b.cents, `${stem} ${b.id}: segments`);
        assert.deepEqual([...b.factIds].sort(), [...f.funding.sum.factIds].sort(), `${stem} ${b.id}: fact ids`);
        const keys = b.segments.map((s) => s.key);
        assert.deepEqual(keys, [...new Set(keys)].sort((x, y) => x - y), `${stem} ${b.id}: segments out of the legend's order`);
        for (const s of b.segments) assert.equal(s.rank, groups[s.key].rank, `${stem} ${b.id}: ${s.label}'s rank`);
      }
      // The legend: groups by their sum over every department, each group it
      // names wearing a hue of its own and only a tail none.
      for (let i = 1; i < groups.length; i++) assert.ok(groups[i - 1].cents >= groups[i].cents || !groups[i].id, `${stem}: legend order`);
      assert.deepEqual(groups.map((g) => g.rank), groups.map((g, i) => (g.id ? i + 1 : 0)));
      assert.equal(groups.reduce((a, g) => a + g.cents, 0), bars.reduce((a, b) => a + b.cents, 0), `${stem}: the legend's sum`);
      // Each group's sum is its funds' links, as the schedule's parent edges file them.
      const sched = column.schedules[config.funding];
      const parent = new Map(sched.nodes.map((n) => [n.node, n.parent]));
      for (const g of groups.filter((x) => x.id)) {
        const want = sched.links.filter((l) => parent.get(l.from) === g.id).reduce((a, l) => a + l.value_cents, 0);
        assert.equal(g.cents, want, `${stem}: ${g.id}`);
      }
      t.diagnostic(`${stem}: ${bars.length} bars, longest ${bars[0].id} at ${bars[0].cents} cents; legend ` +
        groups.map((g) => `${g.rank}:${g.label}`).join(", "));
    });
  }

  // Every pinned column pays from six groups, which rank() names rather than
  // fold the sixth alone. Mutation: give a group past NAMED rank 0 again, and
  // the sixth group's legend entry is drawn in the tail's grey: red, naming it.
  test("in every pinned column each legend entry naming a group wears a hue of its own", async (t) => {
    for (const stem of PINNED) {
      const { document } = await booted(`#y=${stem}`);
      const entries = [...document.querySelectorAll("#department-legend li")]
        .map((li) => ({ rank: li.dataset.rank, label: li.textContent.trim() }));
      const { groups } = mod.departmentBars(columnFixture(stem), departmentPage().config);
      const named = entries.filter((_, i) => groups[i].id);
      t.diagnostic(`${stem}: ${named.length} named groups, ranks ${named.map((e) => e.rank).join(", ")}`);
      assert.ok(named.length > 5, `${stem} names ${named.length} groups, so no sixth is ranked and this proves less than it says`);
      assert.deepEqual(named.filter((e) => e.rank === "0").map((e) => e.label), [], `${stem}: a named group in the tail's grey`);
      assert.equal(new Set(named.map((e) => e.rank)).size, named.length, `${stem}: two named groups share a hue`);
    }
  });

  // Mutation: name every group in departmentBars, folding none, and the
  // tail's members are drawn under hues the legend has five of.
  test("past five fund groups the rest fold into one tail wearing no hue, still summing to each bar", () => {
    const column = structuredClone(columnFixture("fy2027-adopted"));
    const sched = column.schedules[config.funding];
    const paying = [...new Set(sched.links.map((l) => l.from))];
    // Two more groups, each given one paying fund: eight in all, so three fold.
    const moved = [];
    for (const [i, from] of [[0, paying.at(-1)], [1, paying.at(-2)]]) {
      const id = `fund-group/probe-${i}`;
      column.nodes.push({ id, label: `Probe ${i}`, tier: 2, role: "fund_group" });
      sched.nodes.push({ node: column.nodes.length - 1 });
      sched.nodes.find((n) => n.node === from).parent = id;
      moved.push(id);
    }
    const { groups, bars } = mod.departmentBars(column, config);
    assert.equal(groups.length, 6, groups.map((g) => g.label).join(", "));
    const tail = groups.at(-1);
    assert.equal(tail.id, "");
    assert.equal(tail.rank, 0);
    assert.equal(tail.members.length, 3, tail.members.join(", "));
    assert.deepEqual(groups.slice(0, 5).map((g) => g.rank), [1, 2, 3, 4, 5]);
    for (const b of bars) {
      assert.equal(b.segments.reduce((a, x) => a + x.cents, 0), mod.departmentFigures(column, config, b.id).funding.sum.cents, b.id);
      for (const x of b.segments.filter((y) => y.key === 5)) assert.equal(x.rank, 0, `${b.id}: a folded segment wears a hue`);
    }
    assert.equal(groups.reduce((a, g) => a + g.cents, 0), sched.links.reduce((a, l) => a + l.value_cents, 0));
  });

  test("a fund the schedule hangs from no fund group is refused, not drawn in no segment", () => {
    const column = structuredClone(columnFixture("fy2027-adopted"));
    const sched = column.schedules[config.funding];
    const paying = new Set(sched.links.map((l) => l.from));
    const fund = sched.nodes.find((n) => paying.has(n.node) && n.parent);
    for (const [what, edit] of [
      ["no parent", (n) => { delete n.parent; }],
      ["a parent the schedule does not draw", (n) => { n.parent = "fund-group/nowhere"; }],
      ["a parent that is not a fund group", (n) => { n.parent = "department/public-works"; }],
    ]) {
      const c = structuredClone(column);
      edit(c.schedules[config.funding].nodes.find((n) => n.node === fund.node));
      assert.throws(() => mod.departmentBars(c, config), /under no fund group it draws/, what);
    }
  });

  // Mutations: grow each segment by its cents again (style.flexGrow) and the
  // widths arm goes red; put a gap back in .department-track, or a min-width
  // on .department-seg, and the stylesheet arm does.
  test("the drawn bars carry the figures, their tracks scaled to the longest, each segment its cents' share of its track, and every share is marked ours", async () => {
    const { document } = await booted("#y=fy2027-adopted");
    const { groups, bars } = mod.departmentBars(columnFixture("fy2027-adopted"), departmentPage().config);
    const drawn = barsOf(document);
    assert.deepEqual(drawn.map((b) => b.id), bars.map((b) => b.id));
    assert.deepEqual(drawn.map((b) => b.cents), bars.map((b) => b.cents));
    for (const b of drawn) {
      assert.equal(b.segs.reduce((a, s) => a + s.cents, 0), b.cents, `${b.id}: drawn segments`);
      assert.ok(b.segs.every((s) => s.title.includes("◇ our ")), `${b.id}: a segment's title names no share as ours`);
      assert.ok(b.shares.length === b.segs.length && b.shares.every((s) => /^◇ \d+\.\d%$/.test(s)), `${b.id}: ${b.shares.join(", ")}`);
    }
    const tracks = [...document.querySelectorAll("#department-list .department-track")].map((el) => parseFloat(el.style.width));
    assert.equal(tracks[0], 100);
    // Each segment's width is its cents' part of its track, so the segments
    // fill the track and none takes a px the others lose.
    const off = [...document.querySelectorAll("#department-list .department-track")].flatMap((track) => {
      const segs = [...track.querySelectorAll(".department-seg")];
      const sum = segs.reduce((a, s) => a + Number(s.dataset.cents), 0);
      const widths = segs.map((s) => (s.style.width.endsWith("%") ? parseFloat(s.style.width) : NaN));
      const total = widths.reduce((a, w) => a + w, 0);
      return [
        ...(Math.abs(total - 100) < 1e-9 ? [] : [`${track.parentElement.parentElement.dataset.department}: widths sum to ${total}%`]),
        ...segs.flatMap((s, i) => Math.abs(widths[i] - 100 * Number(s.dataset.cents) / sum) < 1e-9 ? [] :
          [`${track.parentElement.parentElement.dataset.department} segment ${i}: ${s.style.width} for ${s.dataset.cents} of ${sum}`]),
      ];
    });
    assert.deepEqual(off, []);
    const css = stylesheet().replace(/\/\*[\s\S]*?\*\//g, "");
    const rule = (sel) => {
      const m = css.match(new RegExp(`(?:^|\\})\\s*${sel.replace(".", "\\.")}\\s*\\{([^}]*)\\}`));
      return m ? m[1] : null;
    };
    const track = rule(".department-track");
    const seg = rule(".department-seg");
    assert.ok(track !== null && seg !== null, "style.css has no .department-track or .department-seg rule to read");
    assert.deepEqual({ gap: /\bgap\s*:/.test(track), minWidth: /min-width/.test(seg), flex: /\bflex\s*:\s*none\s*;/.test(seg) },
      { gap: false, minWidth: false, flex: true }, "a gap, a min-width or a flex grow takes px no segment's cents were given");
    assert.equal(tracks.at(-1), Number((100 * bars.at(-1).cents / bars[0].cents).toFixed(2)));
    const legend = [...document.querySelectorAll("#department-legend li")].map((li) => li.dataset.rank);
    assert.deepEqual(legend, groups.map((g) => String(g.rank)));
    assert.match(document.getElementById("department-bars-sum").textContent, /◇ A bar's figure is our sum/);
  });
});

// One view: the hash's department is the open bar and the sides' department.
//
// Mutation: drop openBar's call from show in boot and every test here goes
// red; drop the toggle branch from activate and a second click selects
// rather than closes; drop the keydown listener and the keyboard arm does;
// redraw the bars only on the first column and the year switch does; open
// the config's first department where the hash names none and the opening
// arm does.
describe("opening a bar chooses its department for the sides and the hash", () => {
  const rowOf = (document, id) => document.querySelector(`#department-list > li[data-department="${id}"] .department-row`);
  const openIds = (document) => barsOf(document).filter((b) => b.expanded === "true").map((b) => b.id);

  test("the opening view opens the longest bar, the first, and names it in the hash and the sides", async () => {
    const { document, window, config } = await booted("");
    const first = barsOf(document)[0].id;
    assert.notEqual(first, config.departments[0].id, "the config's first department is the longest, so the arm is never drawn");
    assert.deepEqual(openIds(document), [first]);
    assert.equal(window.location.hash, `#y=${config.years.at(-1).path.replace(/\.json$/, "")}&d=${first}`);
    assert.equal(document.getElementById("department-select").value, first);
    const label = config.departments.find((d) => d.id === first).label;
    assert.ok(document.getElementById("department-heading").textContent.startsWith(label + ", "), "the sides are not the open bar's");
    const li = document.querySelector(`#department-list > li[data-department="${first}"]`);
    assert.equal(li.querySelector(".department-detail a.department-to-sides").getAttribute("href"), "#department-heading");
    assert.equal(barsOf(document).filter((b) => !b.hidden).length, 1);
  });

  test("a click on a bar opens it, closes the other, and names it in the hash and the select", async () => {
    const { document, window, config } = await booted("#y=fy2026-adopted&d=department/public-works");
    assert.deepEqual(openIds(document), ["department/public-works"]);
    rowOf(document, "department/police-department").dispatchEvent(new window.MouseEvent("click", { bubbles: true }));
    await settle();
    assert.deepEqual(openIds(document), ["department/police-department"]);
    assert.equal(window.location.hash, "#y=fy2026-adopted&d=department/police-department");
    assert.equal(document.getElementById("department-select").value, "department/police-department");
    assert.match(document.getElementById("department-heading").textContent, /^Police Department, /);
    const police = mod.departmentFigures(columnFixture("fy2026-adopted"), config, "department/police-department");
    assert.deepEqual(drawnOf(document, "funding").map((x) => x.id), police.funding.rows.map((r) => r.id), "the funding bars are not police's");
    assert.deepEqual(drawnOf(document, "spending").map((x) => x.id), police.spending.rows.map((r) => r.id), "the spending bars are not police's");

    // A second click closes it and keeps the department.
    rowOf(document, "department/police-department").dispatchEvent(new window.MouseEvent("click", { bubbles: true }));
    await settle();
    assert.deepEqual(openIds(document), []);
    assert.equal(window.location.hash, "#y=fy2026-adopted&d=department/police-department");
    assert.match(document.getElementById("department-heading").textContent, /^Police Department, /);
    assert.equal(barsOf(document).filter((b) => !b.hidden).length, 0);
  });

  test("Enter and Space on a focused bar do what a click does", async () => {
    const { document, window } = await booted("#y=fy2027-adopted&d=department/public-works");
    const row = rowOf(document, "department/fire-department");
    assert.equal(row.getAttribute("tabindex"), "0");
    assert.equal(row.getAttribute("role"), "button");
    const key = (el, k) => {
      const e = new window.KeyboardEvent("keydown", { key: k, bubbles: true, cancelable: true });
      el.dispatchEvent(e);
      return e.defaultPrevented;
    };
    assert.ok(key(row, "Enter"), "Enter was not taken");
    await settle();
    assert.deepEqual(openIds(document), ["department/fire-department"]);
    assert.equal(window.location.hash, "#y=fy2027-adopted&d=department/fire-department");
    assert.ok(key(rowOf(document, "department/fire-department"), " "), "Space was not taken");
    await settle();
    assert.deepEqual(openIds(document), []);
    assert.ok(!key(rowOf(document, "department/fire-department"), "a"), "a key other than Enter or Space was taken");
  });

  test("choosing a department in the select opens its bar", async () => {
    const { document, window } = await booted("#y=fy2027-adopted&d=department/public-works");
    const select = document.getElementById("department-select");
    select.value = "department/city-council";
    select.dispatchEvent(new window.Event("change"));
    await settle();
    assert.deepEqual(openIds(document), ["department/city-council"]);
  });

  test("a year switch redraws the bars from that column and keeps the department open", async () => {
    const { document, window, config } = await booted("#y=fy2027-adopted&d=department/library-department");
    const before = barsOf(document).map((b) => b.cents);
    const year = document.getElementById("year-select");
    year.value = "fy2024-actual";
    year.dispatchEvent(new window.Event("change"));
    await settle();
    const want = mod.departmentBars(columnFixture("fy2024-actual"), config).bars;
    const after = barsOf(document);
    assert.deepEqual(after.map((b) => b.cents), want.map((b) => b.cents));
    assert.notDeepEqual(after.map((b) => b.cents), before, "the bars did not change with the column");
    assert.deepEqual(openIds(document), ["department/library-department"]);
    assert.equal(window.location.hash, "#y=fy2024-actual&d=department/library-department");
    assert.match(document.getElementById("bars-heading").textContent, /FY 2023-24 actual$/);
    assert.equal(document.getElementById("department-heading").textContent, "Library Department, FY 2023-24 actual");
    // A year the other views do not publish is carried to none of them, so
    // no link opens on a refusal; one they publish is carried to each.
    const href = (path) => document.querySelector(`nav.site-nav a[href^="${path}"]`).getAttribute("href");
    assert.deepEqual([href("flows.html"), href("ledger.html"), href("capital.html")], ["flows.html", "ledger.html", "capital.html"]);
    year.value = "fy2026-adopted";
    year.dispatchEvent(new window.Event("change"));
    await settle();
    assert.deepEqual([href("flows.html"), href("ledger.html"), href("capital.html")],
      ["flows.html#y=fy2026-adopted", "ledger.html#y=fy2026-adopted", "capital.html#y=fy2026-adopted"]);
  });

  // Mutation: name the asked-for view current before its column resolves,
  // and the click on a bar drawn from FY 2026-27 asks for the failed year
  // again: the refusal again, and no bar opens.
  test("after a year switch fails, a click on a bar on screen opens it in the year on screen", async () => {
    const { document, window } = await booted("#y=fy2027-adopted&d=department/library-department", { "fy2024-actual": null });
    const year = document.getElementById("year-select");
    year.value = "fy2024-actual";
    year.dispatchEvent(new window.Event("change"));
    await settle();
    const refused = (document.getElementById("department-refusal") || {}).textContent || "";
    rowOf(document, "department/police-department").dispatchEvent(new window.MouseEvent("click", { bubbles: true }));
    await settle();
    assert.deepEqual({
      refused: /could not be drawn: .*fy2024-actual/.test(refused), open: openIds(document), year: year.value,
      hash: window.location.hash, heading: document.getElementById("department-heading").textContent,
      still: document.getElementById("department-refusal") !== null,
    }, {
      refused: true, open: ["department/police-department"], year: "fy2027-adopted",
      hash: "#y=fy2027-adopted&d=department/police-department", heading: "Police Department, FY 2026-27 adopted", still: false,
    });
  });
});

// Mutation: list.addEventListener("click", activate) in place of the
// toTables branch and the link is followed as a fragment, whose popstate
// reopens the newest year's longest bar.
describe("the open bar's link to its sides keeps the view", () => {
  test("from an older year and a chosen department, the link changes neither the year, the department nor the hash", async () => {
    const { document, window } = await booted("#y=fy2024-actual&d=department/public-works");
    const hash = window.location.hash;
    const link = document.querySelector('#department-list > li[data-department="department/public-works"] a.department-to-sides');
    assert.ok(link !== null, "the open bar carries no link to its sides");
    const e = new window.MouseEvent("click", { bubbles: true, cancelable: true });
    link.dispatchEvent(e);
    await settle();
    assert.ok(e.defaultPrevented, "the link was followed as a fragment");
    assert.equal(window.location.hash, hash);
    assert.equal(document.getElementById("year-select").value, "fy2024-actual");
    assert.equal(document.getElementById("department-select").value, "department/public-works");
    assert.equal(document.getElementById("department-heading").textContent, "Public Works, FY 2023-24 actual");
    const open = [...document.querySelectorAll('#department-list > li .department-row[aria-expanded="true"]')]
      .map((r) => r.parentElement.dataset.department);
    assert.deepEqual(open, ["department/public-works"]);
  });
});

// Mutation: write hashOf(view || {year, department: config.departments[0].id})
// after show() resolves, as though a refused or overtaken view were shown,
// and both arms go red naming the config's first department.
describe("the opening view writes the hash only for a view it shows", () => {
  test("a choice made while the opening column is fetched keeps its own hash", async () => {
    const { html, config } = departmentPage();
    const browser = installBrowser({ html });
    await settle();
    const newest = config.years.at(-1).path;
    let release;
    const plan = {};
    for (const stem of PINNED) plan[stem + ".json"] = { doc: columnFixture(stem) };
    const doc = plan[newest].doc;
    plan[newest] = { settle: (h) => { release = () => h.resolve(doc); } };
    browser.window.fetch = plannedFetch(plan, config.exported_by);
    doc.generated_by = config.exported_by;
    const booting = mod.boot(browser.window);
    await settle();
    assert.equal(typeof release, "function", "the opening column was never fetched");
    const select = browser.document.getElementById("department-select");
    const chosen = "department/police-department";
    assert.notEqual(chosen, config.departments[0].id, "the chosen department is the config's first, so the arm is never drawn");
    select.value = chosen;
    select.dispatchEvent(new browser.window.Event("change"));
    release();
    await booting;
    await settle();
    assert.equal(browser.window.location.hash, `#y=${newest.replace(/\.json$/, "")}&d=${chosen}`);
    assert.equal(select.value, chosen);
    assert.match(browser.document.getElementById("department-heading").textContent, /^Police Department, /);
  });

  // Mutation: read the department select in a year's change, as one
  // fromControls did for both selects, and the year chosen during the first
  // fetch opens the config's first department.
  test("a year chosen while the opening column is fetched opens that year's longest bar", async () => {
    const { html, config } = departmentPage();
    const browser = installBrowser({ html });
    await settle();
    const newest = config.years.at(-1).path;
    let release;
    const plan = {};
    for (const stem of PINNED) plan[stem + ".json"] = { doc: columnFixture(stem) };
    const doc = plan[newest].doc;
    plan[newest] = { settle: (h) => { release = () => h.resolve(doc); } };
    browser.window.fetch = plannedFetch(plan, config.exported_by);
    doc.generated_by = config.exported_by;
    const booting = mod.boot(browser.window);
    await settle();
    assert.equal(typeof release, "function", "the opening column was never fetched");
    const year = browser.document.getElementById("year-select");
    year.value = "fy2024-actual";
    year.dispatchEvent(new browser.window.Event("change"));
    await settle();
    release();
    await booting;
    await settle();
    const longest = mod.departmentBars(columnFixture("fy2024-actual"), config).bars[0].id;
    assert.notEqual(longest, config.departments[0].id, "the config's first department is the longest, so the arm is never drawn");
    const open = barsOf(browser.document).filter((b) => b.expanded === "true").map((b) => b.id);
    assert.deepEqual({ open, hash: browser.window.location.hash },
      { open: [longest], hash: `#y=fy2024-actual&d=${longest}` });
  });

  test("a refused opening column leaves the hash as the reader wrote it", async () => {
    const { html, config } = departmentPage();
    for (const hash of ["", "#y=fy2024-actual"]) {
      const browser = installBrowser({ html });
      if (hash) browser.window.location.hash = hash;
      await settle();
      const plan = {};
      for (const stem of PINNED) plan[stem + ".json"] = null;
      browser.window.fetch = plannedFetch(plan, config.exported_by);
      await mod.boot(browser.window);
      await settle();
      const refusal = browser.document.getElementById("department-refusal");
      assert.ok(refusal, `${hash || "no hash"}: no refusal was said`);
      assert.match(refusal.textContent, /could not be drawn/);
      assert.equal(browser.window.location.hash, hash, `${hash || "no hash"}: the hash was rewritten`);
    }
  });
});
