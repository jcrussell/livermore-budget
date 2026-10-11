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
    ["boot", "departmentFigures", "dollars", "hashOf", "parseHash", "readConfig", "render", "share"]);
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
    assert.equal(document.getElementById("department-select").value, config.departments[0].id);
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
