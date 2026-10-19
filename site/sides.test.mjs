// What site/sides.js and site/ledger.js do for ledger.html, over the page Go
// pins (testdata/ledger.golden.html) and the columns it fetches
// (testdata/<stem>.column.json). Every figure is core.sideTree's, a sum of
// cited cents at a membership Go declared; what is tested is that the fold
// keeps every sum, that the form draws every tile of it once, and that the
// page's state, gestures and table say what is drawn.
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
const core = await import(pathToFileURL(path.join(HERE, "core.js")).href);
const sides = await import(pathToFileURL(path.join(HERE, "sides.js")).href);
const format = await import(pathToFileURL(path.join(HERE, "format.js")).href);
const forms = {
  ledger: await import(pathToFileURL(path.join(HERE, "ledger.js")).href),
};

/** The pinned page of one form and the config cut out of it. */
function sidesPage(form) {
  const html = readFileSync(path.join(repoRoot, "testdata", form + ".golden.html"), "utf8");
  const open = '<script type="application/json" id="sides-config">';
  const i = html.indexOf(open);
  if (i < 0) throw new Error(`the pinned ${form} page carries no sides config`);
  const rest = html.slice(i + open.length);
  return { html, config: JSON.parse(rest.slice(0, rest.indexOf("</script>"))) };
}

/** The pinned columns the pages' years fetch. */
const PINNED = ["fy2026-adopted", "fy2027-adopted"];

/** The mark selector of each form. */
const MARK = { ledger: "li.bar-row" };

/** Boots one form's pinned page at a hash, every pinned column planned. */
async function booted(form, hash) {
  const { html, config } = sidesPage(form);
  const browser = installBrowser({ html });
  if (hash) browser.window.location.hash = hash;
  await settle();
  browser.window.fetch = plannedFetch({}, config.exported_by);
  await forms[form].boot(browser.window);
  await settle();
  return { ...browser, config };
}

/** The expansion schedule of a pinned column. */
function expansionOf(stem, config) {
  return core.scheduleOf(columnFixture(stem), config.primary);
}

/** Clicks a mark and settles. */
async function click(browser, el, extra = {}) {
  el.dispatchEvent(new browser.window.MouseEvent("click", { bubbles: true, ...extra }));
  await settle();
}

/** The refusal on the page, or "". */
function refusal(document) {
  const el = document.getElementById("sides-refusal");
  return el ? el.textContent : "";
}

test("the modules import with no page config, no d3 and no DOM, and run nothing", () => {
  assert.ok(domless, "nothing installed a DOM before the import");
  assert.deepEqual(Object.keys(sides).sort(), ["KEY_MARK", "boot", "citationsOf", "clone", "closing", "fillTable",
    "frontierOf", "hashOf", "hideTip", "opensAs", "outside", "parseHash", "readConfig", "render", "say", "share", "showTip", "sidesAt", "sidesOf", "tileShare"]);
  assert.deepEqual(Object.keys(forms.ledger).sort(), ["LEDGER", "RANKED", "boot", "widthOf"]);
});

// The fold keeps every sum: at any frontier, a side's roots sum to the same
// cents, and an opened node's members sum to it, signed, with the printed
// reductions counting against their parent. Mutation: drop the `source ===
// n.id || target === n.id` filter in core.sideTree's member join and a member
// is credited with every ribbon at it.
describe("core.sideTree keeps every sum through every split", () => {
  const { config } = sidesPage("ledger");
  for (const stem of PINNED) {
    test(stem, (t) => {
      const doc = expansionOf(stem, config);
      const roots = core.rootFrontier(doc);
      const tiers = sides.sidesOf(config).map((s) => s.tier);
      const totals = tiers.map((tier) => core.sideTree(doc, roots, tier).cents);
      assert.ok(totals.every((c) => c === totals[0]), `the sides sum differently at the root: ${totals.join(", ")}`);
      let splits = 0;
      let reductions = 0;
      let deepest = 0;
      const walk = (frontier, tier, depth) => {
        const tree = core.sideTree(doc, frontier, tier);
        assert.equal(tree.cents, totals[0], `${stem} tier ${tier}: the side's sum moved at depth ${depth}`);
        for (const { tile } of core.sideTiles(tree.roots)) {
          if (tile.children) {
            assert.equal(tile.children.reduce((a, c) => a + c.cents, 0), tile.cents, `${tile.id}'s members do not sum to it`);
            assert.equal(new Set(tile.children.map((c) => c.id)).size, tile.children.length, `${tile.id} draws a member twice`);
            reductions += tile.children.filter((c) => c.cents < 0).length;
          }
          assert.equal(tile.size, Math.abs(tile.cents));
        }
        if (depth >= 2) return;
        for (const { tile } of core.sideTiles(tree.roots)) {
          if (tile.children) continue;
          for (const sp of tile.splits) {
            splits++;
            deepest = Math.max(deepest, depth + 1);
            walk(core.expand(doc, frontier, tile.id, sp.key), tier, depth + 1);
          }
        }
      };
      for (const tier of tiers) walk(roots, tier, 0);
      assert.ok(splits > 20, `only ${splits} splits walked`);
      assert.ok(reductions >= 2, `the Property Taxes reductions were not drawn as negative members (${reductions})`);
      t.diagnostic(`${stem}: ${splits} splits opened to depth ${deepest}, every member sum its parent's; ${reductions} reductions counted against their parent`);
    });
  }
  // Mutation: set a tile's derived from its node alone and the member whose
  // every ribbon is inferred reads as printed.
  test("a member whose every ribbon is inferred is drawn inferred", () => {
    const doc = structuredClone(expansionOf("fy2026-adopted", config));
    const roots = core.rootFrontier(doc);
    const parent = "revenue/taxes/property";
    const key = core.splitsOf(doc, parent)[0].key;
    const opened = core.sideTiles(core.sideTree(doc, core.expand(doc, roots, parent, key), 0).roots)
      .find((x) => x.tile.id === parent).tile;
    const [marked, other] = opened.children.filter((c) => !c.derived);
    for (const l of doc.links) if (l.source === marked.id || l.target === marked.id) l.derived = true;
    const again = core.sideTiles(core.sideTree(doc, core.expand(doc, roots, parent, key), 0).roots);
    assert.ok(again.find((x) => x.tile.id === marked.id).tile.derived, `${marked.id} reads as printed`);
    assert.ok(!again.find((x) => x.tile.id === other.id).tile.derived, `${other.id} reads as inferred`);
  });
  test("a tier whose roots both take in and pay out is refused", () => {
    const doc = expansionOf("fy2026-adopted", config);
    assert.throws(() => core.sideTree(doc, core.rootFrontier(doc), 2), /both takes in and pays out/);
  });
});

// Each form draws every tile of every side once, with the tree's cents on it,
// and the table lists the same. Mutation: skip members in LEDGER's row, and
// the roots' marks go missing.
describe("each form draws every tile once and the table lists them", () => {
  for (const form of Object.keys(forms)) {
    test(form, async (t) => {
      const { document, config } = await booted(form);
      assert.equal(refusal(document), "");
      const doc = expansionOf("fy2027-adopted", config);
      const roots = core.rootFrontier(doc);
      let expected = 0;
      for (const side of sides.sidesOf(config)) {
        const tree = core.sideTree(doc, roots, side.tier);
        const section = document.querySelector(`section.sides-side[data-tier="${side.tier}"]`);
        const marks = [...section.querySelectorAll(MARK[form])];
        const ids = marks.map((m) => m.dataset.id).sort();
        assert.deepEqual(ids, core.sideTiles(tree.roots).map((x) => x.tile.id).sort());
        for (const m of marks) {
          const tile = core.sideTiles(tree.roots).find((x) => x.tile.id === m.dataset.id).tile;
          assert.equal(Number(m.dataset.cents), tile.cents, `${m.dataset.id} carries other cents`);
          assert.equal(m.getAttribute("tabindex"), "0");
          assert.equal(m.getAttribute("role"), "button");
          // An img's children are presentational, so a button inside one
          // is no button. Mutation: role="img" on ledger.html.tmpl's
          // ledger-list.
          assert.ok(!m.parentElement.closest('[role="img"]'), `${m.dataset.id} is a button inside an image`);
        }
        assert.ok(section.querySelector(".sides-total").textContent.includes(format.dollars(tree.cents)));
        expected += marks.length;
      }
      const rows = [...document.querySelectorAll("#sides-table tbody tr")];
      assert.equal(rows.length, expected);
      assert.ok(document.getElementById("sides-count").textContent.startsWith("◇ our count: " + expected));
      t.diagnostic(`${form}: ${expected} marks across ${sides.sidesOf(config).length} sides at the root`);
    });
  }
});

// A click opens a tile in place and its members are drawn under it, each at
// its share of the one scale; a second click closes it; the hash carries the
// opened entries; Back restores. Mutation: drop `tile.id` from the opened
// filter in sides.js's toggle and a second click opens the node again.
describe("a click opens a tile, a click closes it, and the hash says which", () => {
  for (const form of Object.keys(forms)) {
    test(form, async (t) => {
      const browser = await booted(form);
      const { document, window, config } = browser;
      const first = [...document.querySelectorAll(MARK[form])].find((m) => m.classList.contains("can-open"));
      assert.ok(first, "no mark opens");
      const id = first.dataset.id;
      const before = document.querySelectorAll(MARK[form]).length;
      await click(browser, first);
      assert.equal(window.location.hash, `#y=fy2027-adopted&o=${id}`);
      const doc = expansionOf("fy2027-adopted", config);
      const split = core.splitsOf(doc, id).find((sp) => !sp.alternative);
      const after = document.querySelectorAll(MARK[form]).length;
      assert.equal(after, before + split.members.length, "the members are not all drawn");
      const opened = document.querySelector(`${MARK[form]}[data-id="${id}"]`);
      assert.ok(opened.classList.contains("opened"));
      const members = [...document.querySelectorAll(`${MARK[form]}[data-depth="1"]`)];
      assert.equal(members.length, split.members.length);
      const sum = members.reduce((a, m) => a + Number(m.dataset.cents), 0);
      assert.equal(sum, Number(opened.dataset.cents), "the drawn members do not sum to the opened tile");
      if (form === "ledger") {
        // One scale: the widest root fills its track and a member's width is its size's share of that.
        const widest = Math.max(...[...document.querySelectorAll(`${MARK[form]}[data-depth="0"]`)].map((m) => Math.abs(Number(m.dataset.cents))));
        for (const m of members) {
          const want = (100 * Math.abs(Number(m.dataset.cents))) / widest;
          const got = parseFloat(m.querySelector(".bar-fill").style.width);
          assert.ok(Math.abs(got - Math.max(want, 0.25)) < 0.01, `${m.dataset.id}: width ${got}% for ${want}%`);
        }
      }
      await click(browser, opened);
      assert.equal(window.location.hash, "#y=fy2027-adopted");
      assert.equal(document.querySelectorAll(MARK[form]).length, before);
      // Back reopens it.
      window.history.back();
      await settle();
      await settle();
      assert.equal(document.querySelectorAll(MARK[form]).length, after, "Back did not restore the opened tile");
      t.diagnostic(`${form}: ${id} opened into ${split.members.length} members and closed`);
    });
  }
});

// Closing a tile drops every entry inside it, from the node whose opening
// opened it: an entry left behind is replayed on a frontier that no longer
// draws it and refused, so an ordinary close would say the address could not
// be drawn. Over the pinned schedule, which nests no opener under a drawn
// root; the shapes are the schedule's own. Mutation: filter `opened` by the
// tile's own id and `id~` prefix instead of closing() and both cases fail.
test("closing a tile drops the entries inside it, and a member opened straight through closes with its parent", () => {
  const { config } = sidesPage("ledger");
  const doc = expansionOf("fy2027-adopted", config);
  // fund-group/general's one split has one member, fund/100, which
  // core.expand opens straight through; the schedule has no other.
  const [one] = doc.splits.filter((sp) => core.splitsOf(doc, sp.node).find((x) => x.key === sp.key).members.length === 1);
  assert.ok(one, "the pinned schedule declares no one-member split");
  const [member] = core.splitsOf(doc, one.node).find((x) => x.key === one.key).members;
  assert.deepEqual(sides.closing(doc, [one.node], member), [], "closing the member did not close the parent that opened it");
  // Opening the member another way keeps the parent that opened it; the
  // member's own entries go. Mutation: filter from straightFrom in
  // outside() and the parent's entry is dropped.
  assert.deepEqual(sides.outside(doc, [one.node, member + sides.KEY_MARK + "x"], member), [one.node]);
  // A parent with several members: its own entry and a member's go; a
  // sibling root's stays.
  const wide = doc.splits.find((sp) => core.splitsOf(doc, sp.node).find((x) => x.key === sp.key).members.length > 1);
  const wideMembers = core.splitsOf(doc, wide.node).find((x) => x.key === wide.key).members;
  const other = doc.nodes.find((n) => !n.parent && n.id !== wide.node && n.id !== one.node).id;
  assert.deepEqual(
    sides.closing(doc, [other, wide.node, wideMembers[0], wideMembers[1] + sides.KEY_MARK + "x"], wide.node),
    [other]);
  assert.deepEqual(sides.closing(doc, [other, wide.node, wideMembers[0]], wideMembers[0]), [other, wide.node]);
});

// The hash is a view: a column and opened entries, replayed through
// core.expand, with what the schedule refuses named and dropped from the
// address. Mutation: keep refused entries in `kept` in frontierOf and the
// address keeps naming a node that is not drawn.
test("a hash naming the other year and an opened node is restored, and a bad entry is refused by name", async () => {
  const browser = await booted("ledger", "#y=fy2026-adopted&o=revenue/taxes/property,no/such/node");
  const { document, window } = browser;
  assert.match(refusal(document), /no\/such\/node/);
  assert.equal(window.location.hash, "#y=fy2026-adopted&o=revenue/taxes/property");
  assert.equal(document.getElementById("year-select").value, "fy2026-adopted.json");
  const opened = document.querySelector('li.bar-row[data-id="revenue/taxes/property"]');
  assert.ok(opened.classList.contains("opened"));
  const reductions = [...document.querySelectorAll("li.bar-row.reduction")];
  assert.equal(reductions.length, 2, "the two printed reductions under Property Taxes are not drawn as reductions");
  // A printed reduction takes no share: not on its bar, in its label, in its
  // table row or in its tooltip. Mutation: drop the contra test in
  // tileShare and each reads "◇ x%".
  for (const r of reductions) {
    assert.ok(Number(r.dataset.cents) < 0);
    assert.ok(r.querySelector(".bar-label").textContent.startsWith("− "));
    assert.equal(r.querySelector(".bar-share").textContent, "", `${r.dataset.id} states a share`);
    assert.ok(!r.getAttribute("aria-label").includes("◇"), `${r.dataset.id}'s label states a share`);
    const row = document.querySelector(`#sides-table tr[data-id="${r.dataset.id}"]`);
    assert.ok(!row.textContent.includes("◇"), `${r.dataset.id}'s table row states a share`);
    r.dispatchEvent(new window.Event("pointerenter", { bubbles: true }));
    assert.ok(!document.querySelector("#tooltip .tip-meta").textContent.includes("◇"), `${r.dataset.id}'s tooltip states a share`);
  }
  // Nor is a reduction root in any share's total. Mutation: divide by
  // side.tree.cents and this reads 62.5%.
  const side = { tree: { cents: 80, roots: [{ contra: "", cents: 100 }, { contra: "a reduction", cents: -20 }] } };
  assert.equal(sides.tileShare(/** @type {any} */ ({ contra: "", cents: 50 }), /** @type {any} */ (side)), "◇ 50.0%");
  const plain = document.querySelector('li.bar-row[data-id="revenue/taxes/property"]');
  assert.match(plain.querySelector(".bar-share").textContent, /^◇ \d+\.\d%$/);
  assert.deepEqual(sides.parseHash("#y=nope&o=a,b~k", browser.config),
    { year: "fy2027-adopted.json", opened: ["a", "b~k"], refused: "No budget column is called nope, so this shows the newest." });
  assert.equal(sides.hashOf({ year: "fy2026-adopted.json", opened: ["a", "b~k"] }), "#y=fy2026-adopted&o=a,b~k");
});

// A year switch keeps what is open and redraws it over the other column's
// figures. Mutation: pass `opened: []` on the year change in boot and the
// opened node shuts on a switch.
test("a year switch keeps the opened node and redraws the other year's figures", async () => {
  const browser = await booted("ledger", "#o=revenue/taxes/sales");
  const { document, window, config } = browser;
  const was = document.querySelector('li.bar-row[data-id="revenue/taxes/sales"]');
  assert.ok(was.classList.contains("opened"));
  const select = document.getElementById("year-select");
  select.value = "fy2026-adopted.json";
  select.dispatchEvent(new window.Event("change"));
  await settle();
  assert.equal(window.location.hash, "#y=fy2026-adopted&o=revenue/taxes/sales");
  const now = document.querySelector('li.bar-row[data-id="revenue/taxes/sales"]');
  assert.ok(now.classList.contains("opened"));
  const tree = core.sideTree(expansionOf("fy2026-adopted", config), core.expand(expansionOf("fy2026-adopted", config),
    core.rootFrontier(expansionOf("fy2026-adopted", config)), "revenue/taxes/sales", "lines"), 0);
  const tile = core.sideTiles(tree.roots).find((x) => x.tile.id === "revenue/taxes/sales").tile;
  assert.equal(Number(now.dataset.cents), tile.cents);
  assert.notEqual(Number(now.dataset.cents), Number(was.dataset.cents), "the two years print the same sales tax");
  assert.equal(document.getElementById("lede-year").textContent, "FY 2025-26 adopted");
});

// Shift opens a node its alternative way where one is declared.
test("shift-click opens the alternative split, and the hash names it", async () => {
  const browser = await booted("ledger");
  const { document, window, config } = browser;
  const doc = expansionOf("fy2027-adopted", config);
  const two = core.sideTiles(core.sideTree(doc, core.rootFrontier(doc), 5).roots)
    .find((x) => x.tile.splits.some((sp) => sp.alternative)).tile;
  const alt = two.splits.find((sp) => sp.alternative);
  const mark = document.querySelector(`li.bar-row[data-id="${two.id}"]`);
  await click(browser, mark, { shiftKey: true });
  assert.equal(window.location.hash, `#y=fy2027-adopted&o=${two.id}~${alt.key}`);
  const members = [...document.querySelectorAll('li.bar-row[data-depth="1"]')];
  assert.equal(members.length, alt.members.length);
  assert.equal(members.reduce((a, m) => a + Number(m.dataset.cents), 0), two.cents);
  // A plain click on a tile open its other way closes it, as the key says,
  // rather than reopening it the primary way; Shift-click on a tile open
  // the primary way switches it. Mutation: compare tile.open to the primary
  // key on a plain click in toggle and the first click below reopens.
  const rows = () => document.querySelectorAll("li.bar-row").length;
  const before = rows();
  await click(browser, document.querySelector(`li.bar-row[data-id="${two.id}"]`));
  assert.equal(window.location.hash, "#y=fy2027-adopted");
  assert.ok(rows() < before, "the plain click did not close the tile");
  await click(browser, document.querySelector(`li.bar-row[data-id="${two.id}"]`));
  assert.equal(window.location.hash, `#y=fy2027-adopted&o=${two.id}`);
  await click(browser, document.querySelector(`li.bar-row[data-id="${two.id}"]`), { shiftKey: true });
  assert.equal(window.location.hash, `#y=fy2027-adopted&o=${two.id}~${alt.key}`);
  await click(browser, document.querySelector(`li.bar-row[data-id="${two.id}"]`), { shiftKey: true });
  assert.equal(window.location.hash, "#y=fy2027-adopted");
});

// A hash wrong in its year and in an entry is answered for both. Mutation:
// say the entry refusal alone in show() and the year's sentence is lost.
test("a hash naming no such year and no such node is refused for both", async () => {
  const browser = await booted("ledger", "#y=nope&o=no/such/node");
  const text = refusal(browser.document);
  assert.match(text, /No budget column is called nope/);
  assert.match(text, /no\/such\/node/);
  assert.equal(browser.window.location.hash, "#y=fy2027-adopted");
});

// A focus event carries no pointer; the tip goes under the mark reached.
// Mutation: drop the getBoundingClientRect branch in showTip and the tip
// stands at the viewport's corner.
test("the tooltip stands under a focused mark", async () => {
  const browser = await booted("ledger");
  const { document, config } = browser;
  const doc = expansionOf("fy2027-adopted", config);
  const side = core.sideTree(doc, core.rootFrontier(doc), 5);
  const tile = side.roots[0];
  const target = { getBoundingClientRect: () => ({ left: 120, bottom: 340, top: 320, right: 400 }) };
  sides.showTip(document, /** @type {any} */ ({ target }), tile, { heading: "Where the money goes", tree: side, tier: 5 });
  const tip = document.getElementById("tooltip");
  assert.equal(tip.hidden, false);
  assert.equal(tip.style.left, "134px");
  assert.equal(tip.style.top, "354px");
  sides.showTip(document, /** @type {any} */ ({ clientX: 10, clientY: 20, target }), tile, { heading: "Where the money goes", tree: side, tier: 5 });
  assert.equal(tip.style.left, "24px");
  assert.equal(tip.style.top, "34px");
});

// The tooltip says the tile's figure, its share of its side marked ours, and
// the pages it cites; it hides on leave.
test("hover shows a tile's figure, share and pages, and leaving hides it", async () => {
  const browser = await booted("ledger");
  const { document, window } = browser;
  const mark = document.querySelector('li.bar-row[data-id="revenue/taxes/property"]');
  mark.dispatchEvent(new window.Event("pointerenter", { bubbles: true }));
  const tip = document.getElementById("tooltip");
  assert.equal(tip.hidden, false);
  assert.equal(tip.querySelector(".tip-label").textContent, "Property Taxes");
  assert.equal(tip.querySelector(".tip-value").textContent, format.dollars(Number(mark.dataset.cents)));
  assert.match(tip.querySelector(".tip-meta").textContent, /^◇ \d+\.\d% of where the money comes from\. Click to open: \d+ lines/);
  // The root's ribbons are the fund-flows leaves folded, so it cites the
  // revenue-by-fund pages, pp.127-140, and not p66.
  assert.match(tip.querySelector(".facts").textContent, /^p1[2-4]\d(, p1[2-4]\d)*$/);
  mark.dispatchEvent(new window.Event("pointerleave", { bubbles: true }));
  assert.equal(tip.hidden, true);
});

// A page declaring a form its script does not draw is refused before a fetch.
test("a config declaring another form is refused, and a missing config is said", async () => {
  const { html, config } = sidesPage("ledger");
  const browser = installBrowser({ html: html.replace('"form":"ledger"', '"form":"sankey"').replace('"ledger":{"sides"', '"sankey":{"sides"') });
  const fetch = plannedFetch({}, config.exported_by);
  browser.window.fetch = fetch;
  await forms.ledger.boot(browser.window);
  await settle();
  assert.match(refusal(browser.document), /declares a sankey chart and this script draws a ledger/);
  assert.equal(fetch.asked.length, 0, "a refused page still fetched a column");
  const bare = installBrowser({ html: html.replace('id="sides-config"', 'id="nothing"') });
  await forms.ledger.boot(bare.window);
  assert.match(refusal(bare.document), /carries no sides config/);
});
