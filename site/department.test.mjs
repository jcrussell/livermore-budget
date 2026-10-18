// What site/department.js does for department.html, over the page Go pins
// (testdata/department.golden.html) and the columns it fetches
// (testdata/<stem>.column.json). The module is imported with nothing on the
// global, so one that read a global or held state at import would fail first.
import { test, describe } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";
import { columnFixture, installBrowser, plannedFetch, repoRoot, settle } from "./testlib.mjs";

const HERE = path.dirname(fileURLToPath(import.meta.url));

delete globalThis.FISC_CONFIG;
delete globalThis.d3;
const domless = typeof globalThis.document === "undefined";
const mod = await import(pathToFileURL(path.join(HERE, "department.js")).href);

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

/** Boots the pinned page at a hash, every pinned column planned. */
async function booted(hash) {
  const { html, config } = departmentPage();
  const browser = installBrowser({ html });
  // A fragment navigation fires popstate in its own task; settled before
  // boot, so the page's own listener never hears the test's navigation.
  if (hash) browser.window.location.hash = hash;
  await settle();
  const plan = {};
  for (const stem of PINNED) plan[stem + ".json"] = { doc: columnFixture(stem) };
  browser.window.fetch = plannedFetch(plan, config.exported_by);
  await mod.boot(browser.window);
  await settle();
  return { ...browser, config };
}

/** The text of every cell of one table body, row by row. */
function rowsOf(document, id) {
  return [...document.querySelectorAll(`#${id} tbody tr`)].map((tr) => [...tr.children].map((c) => c.textContent));
}

test("department.js imports with no page config, no d3 and no DOM, and exports only its functions", () => {
  assert.ok(domless, "nothing installed a DOM before the import");
  assert.deepEqual(Object.keys(mod).sort(),
    ["boot", "departmentBars", "departmentFigures", "dollars", "drawBars", "hashOf", "openBar", "parseHash",
      "readConfig", "render", "share"]);
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
        assert.equal(f.spending.cells.reduce((a, c) => a + c.cents, 0), f.spending.sum.cents, `${stem} ${d.id} cells`);
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
    const heads = rowsOf(document, "spending-table").map((r) => r[0]);
    assert.ok(heads.includes("Wages & Benefits (not an expenditure)"), `no labelled row: ${heads.join(", ")}`);
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
});

describe("the page over the pinned columns", () => {
  test("the hash opens one department of one column, and the selects say which", async () => {
    const { document, window } = await booted("#y=fy2024-actual&d=department/public-works");
    assert.equal(document.getElementById("department-select").value, "department/public-works");
    assert.equal(document.getElementById("year-select").value, "fy2024-actual.json");
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
    assert.equal(document.getElementById("year-select").value, config.years.at(-1).path);
    const longest = mod.departmentBars(columnFixture(config.years.at(-1).path.replace(/\.json$/, "")), config).bars[0].id;
    assert.equal(document.getElementById("department-select").value, longest);
  });

  test("every amount links to the text of a page its figure cites, and carries the fact ids it shows", async () => {
    const { document, config } = await booted("#y=fy2026-adopted&d=department/administrative-services");
    const texts = new Set(Object.values(config.docs).flatMap((d) => Object.values(d.pages).map((p) => p.text)));
    const cells = [...document.querySelectorAll(".department-table td.num[data-cents]")];
    assert.ok(cells.length > 0);
    for (const td of cells) {
      assert.ok(td.dataset.factIds.length > 0, `an amount cell ${td.textContent} cites no fact`);
      if (td.closest("tr").dataset.sum) continue;
      const a = td.querySelector("a");
      assert.ok(a && texts.has(a.getAttribute("href")), `${td.textContent} links to no cited page's text`);
      assert.match(a.title, /^Read on page \d+$/);
    }
  });

  test("a side's closing row is our sum, and nothing on either side is called a total", async () => {
    const { document } = await booted("#y=fy2027-adopted&d=department/police-department");
    for (const id of ["funding-table", "spending-table"]) {
      const rows = [...document.querySelectorAll(`#${id} tbody tr`)];
      const last = rows.at(-1);
      assert.equal(last.dataset.sum, "ours", `${id} closes on no sum row`);
      assert.equal(last.querySelector("th").textContent, "Our sum of the rows above");
      const summed = rows.slice(0, -1).reduce((a, tr) => a + Number(tr.querySelector("td.num[data-cents]").dataset.cents), 0);
      assert.equal(Number(last.querySelector("td.num").dataset.cents), summed, `${id}'s sum is not its rows'`);
    }
    for (const id of ["funding-table", "spending-table", "division-table"]) {
      for (const row of rowsOf(document, id)) {
        assert.ok(!row.some((c) => /total/i.test(c)), `${id} calls something a total: ${row.join(" | ")}`);
      }
    }
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

  test("Transfers Out is drawn as not an expenditure, inside the sum, with its own sentence", async () => {
    const { document } = await booted("#y=fy2024-actual&d=department/public-works");
    const rows = rowsOf(document, "spending-table");
    assert.ok(rows.some((r) => r[0] === "Transfers Out (not an expenditure)"), "no Transfers Out row");
    assert.equal(document.getElementById("transfers-says").hidden, false);

    const later = await booted("#y=fy2026-adopted&d=department/public-works");
    assert.equal(later.document.getElementById("transfers-says").hidden, true,
      "the Transfers Out sentence shows in a column with no such row");
  });

  test("a fund the schedule calls internal service is marked so, beside the sentences that say what each row is", async () => {
    const { document } = await booted("#y=fy2026-adopted&d=department/administrative-services");
    const heads = [...document.querySelectorAll("#funding-table tbody th")].map((th) => th.textContent);
    assert.ok(heads.some((h) => h.endsWith(" (internal service)")), `no row marked internal service: ${heads.join(", ")}`);
    assert.match(document.getElementById("internal-service-says").textContent, /charging its own\s+departments/);
    assert.match(document.getElementById("funding-says").textContent, /General Fund row is the General Fund's share alone/);
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
      // The legend: groups by their sum over every department, the five
      // largest wearing a hue and the rest none.
      for (let i = 1; i < groups.length; i++) assert.ok(groups[i - 1].cents >= groups[i].cents || !groups[i].id, `${stem}: legend order`);
      assert.deepEqual(groups.map((g) => g.rank), groups.map((g, i) => (g.id && i < 5 ? i + 1 : 0)));
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

  test("the drawn bars carry the figures, their tracks scaled to the longest, and every share is marked ours", async () => {
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
    assert.equal(tracks.at(-1), Number((100 * bars.at(-1).cents / bars[0].cents).toFixed(2)));
    const legend = [...document.querySelectorAll("#department-legend li")].map((li) => li.dataset.rank);
    assert.deepEqual(legend, groups.map((g) => String(g.rank)));
    assert.match(document.getElementById("department-bars-sum").textContent, /◇ A bar's figure is our sum/);
  });
});

// One view: the hash's department is the open bar and the tables' department.
//
// Mutation: drop openBar's call from show in boot and every test here goes
// red; drop the toggle branch from activate and a second click selects
// rather than closes; drop the keydown listener and the keyboard arm does;
// redraw the bars only on the first column and the year switch does; open
// the config's first department where the hash names none and the opening
// arm does.
describe("opening a bar chooses its department for the tables and the hash", () => {
  const rowOf = (document, id) => document.querySelector(`#department-list > li[data-department="${id}"] .department-row`);
  const openIds = (document) => barsOf(document).filter((b) => b.expanded === "true").map((b) => b.id);

  test("the opening view opens the longest bar, the first, and names it in the hash and the tables", async () => {
    const { document, window, config } = await booted("");
    const first = barsOf(document)[0].id;
    assert.notEqual(first, config.departments[0].id, "the config's first department is the longest, so the arm is never drawn");
    assert.deepEqual(openIds(document), [first]);
    assert.equal(window.location.hash, `#y=${config.years.at(-1).path.replace(/\.json$/, "")}&d=${first}`);
    assert.equal(document.getElementById("department-select").value, first);
    const label = config.departments.find((d) => d.id === first).label;
    assert.ok(document.getElementById("department-heading").textContent.startsWith(label + ", "), "the tables are not the open bar's");
    const li = document.querySelector(`#department-list > li[data-department="${first}"]`);
    assert.equal(li.querySelector(".department-detail a.department-to-tables").getAttribute("href"), "#department-heading");
    assert.equal(barsOf(document).filter((b) => !b.hidden).length, 1);
  });

  test("a click on a bar opens it, closes the other, and names it in the hash and the select", async () => {
    const { document, window } = await booted("#y=fy2026-adopted&d=department/public-works");
    assert.deepEqual(openIds(document), ["department/public-works"]);
    rowOf(document, "department/police-department").dispatchEvent(new window.MouseEvent("click", { bubbles: true }));
    await settle();
    assert.deepEqual(openIds(document), ["department/police-department"]);
    assert.equal(window.location.hash, "#y=fy2026-adopted&d=department/police-department");
    assert.equal(document.getElementById("department-select").value, "department/police-department");
    assert.match(document.getElementById("department-heading").textContent, /^Police Department, /);
    assert.ok(document.querySelector('#funding-table tbody tr[data-id="fund/100"]'), "the tables are not police's");

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
    year.value = "fy2024-actual.json";
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
  });
});

// Mutation: list.addEventListener("click", activate) in place of the
// toTables branch and the link is followed as a fragment, whose popstate
// reopens the newest year's longest bar.
describe("the open bar's link to its tables keeps the view", () => {
  test("from an older year and a chosen department, the link changes neither the year, the department nor the hash", async () => {
    const { document, window } = await booted("#y=fy2024-actual&d=department/public-works");
    const hash = window.location.hash;
    const link = document.querySelector('#department-list > li[data-department="department/public-works"] a.department-to-tables');
    assert.ok(link !== null, "the open bar carries no link to its tables");
    const e = new window.MouseEvent("click", { bubbles: true, cancelable: true });
    link.dispatchEvent(e);
    await settle();
    assert.ok(e.defaultPrevented, "the link was followed as a fragment");
    assert.equal(window.location.hash, hash);
    assert.equal(document.getElementById("year-select").value, "fy2024-actual.json");
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
