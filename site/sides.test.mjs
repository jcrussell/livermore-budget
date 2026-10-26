// site/sides.js and site/ledger.js over testdata/client/ledger.html and its
// columns. Figures are core.sideTree's; tested: the fold keeps every sum,
// the form draws every tile once, and state, gestures and tips say what is
// drawn.
import { test, describe } from "node:test";
import assert from "node:assert/strict";
import path from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";
import { bootedAt, columnFixture, configuredPage, installBrowser, plannedFetch, settle, sideTiles } from "./testlib.mjs";

const HERE = path.dirname(fileURLToPath(import.meta.url));

delete globalThis.FISC_CONFIG;
delete globalThis.d3;
const core = await import(pathToFileURL(path.join(HERE, "core.js")).href);
const sides = await import(pathToFileURL(path.join(HERE, "sides.js")).href);
const format = await import(pathToFileURL(path.join(HERE, "format.js")).href);
const forms = {
  ledger: await import(pathToFileURL(path.join(HERE, "ledger.js")).href),
};

/** The pinned page of one form and the config cut out of it. */
const sidesPage = (form) => configuredPage(form, "sides-config");

/** The pinned columns the pages' years fetch. */
const PINNED = ["fy2026-adopted", "fy2027-adopted"];

/** The mark selector of each form. */
const MARK = { ledger: "li.bar-row" };

/** Boots one form's pinned page at a hash, every pinned column planned or as `plan` says. */
async function booted(form, hash, plan = {}) {
  const { html, config } = sidesPage(form);
  return { ...await bootedAt(html, forms[form].boot, hash, plan, config.exported_by), config };
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

// The ledger reads its own hints and refuses a chart that declares no sides.
// Mutation: drop the length check and an empty sides list is accepted.
test("the ledger reads its sides off its own hints and refuses a chart without them", () => {
  const { config } = sidesPage("ledger");
  const { LEDGER } = forms.ledger;
  assert.deepEqual(LEDGER.sidesOf(config.overview), config.overview.ledger.sides);
  for (const chart of [{ form: "ledger" }, { form: "ledger", ledger: { sides: [] } },
    { form: "ledger", sankey: { sides: config.overview.ledger.sides } }]) {
    assert.throws(() => LEDGER.sidesOf(chart), /declares no sides for its ledger chart/, JSON.stringify(Object.keys(chart)));
  }
});

// At any frontier the roots, and an opened node's members, sum to the same
// signed cents. Mutation: drop core.sideTree's `source === n.id || target ===
// n.id` member filter and a member is credited every ribbon at it.
describe("core.sideTree keeps every sum through every split", () => {
  const { config } = sidesPage("ledger");
  for (const stem of PINNED) {
    test(stem, (t) => {
      const doc = expansionOf(stem, config);
      const roots = core.rootFrontier(doc);
      const tiers = forms.ledger.LEDGER.sidesOf(config.overview).map((s) => s.tier);
      const totals = tiers.map((tier) => core.sideTree(doc, roots, tier).cents);
      assert.ok(totals.every((c) => c === totals[0]), `the sides sum differently at the root: ${totals.join(", ")}`);
      let splits = 0;
      let reductions = 0;
      let deepest = 0;
      const walk = (frontier, tier, depth) => {
        const tree = core.sideTree(doc, frontier, tier);
        assert.equal(tree.cents, totals[0], `${stem} tier ${tier}: the side's sum moved at depth ${depth}`);
        for (const { tile } of sideTiles(tree.roots)) {
          if (tile.children) {
            assert.equal(tile.children.reduce((a, c) => a + c.cents, 0), tile.cents, `${tile.id}'s members do not sum to it`);
            assert.equal(new Set(tile.children.map((c) => c.id)).size, tile.children.length, `${tile.id} draws a member twice`);
            reductions += tile.children.filter((c) => c.cents < 0).length;
          }
          assert.equal(tile.size, Math.abs(tile.cents));
        }
        if (depth >= 2) return;
        for (const { tile } of sideTiles(tree.roots)) {
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
    const opened = sideTiles(core.sideTree(doc, core.expand(doc, roots, parent, key), 0).roots)
      .find((x) => x.tile.id === parent).tile;
    const [marked, other] = opened.children.filter((c) => !c.derived);
    for (const l of doc.links) if (l.source === marked.id || l.target === marked.id) l.derived = true;
    const again = sideTiles(core.sideTree(doc, core.expand(doc, roots, parent, key), 0).roots);
    assert.ok(again.find((x) => x.tile.id === marked.id).tile.derived, `${marked.id} reads as printed`);
    assert.ok(!again.find((x) => x.tile.id === other.id).tile.derived, `${other.id} reads as inferred`);
  });
  // The any-derived rule core.sumReading holds for both pages. Mutation:
  // read a tile inferred only where every ribbon is and the root below,
  // one of whose ribbons is marked inferred, reads as printed.
  test("a tile summing one inferred ribbon among printed ones is drawn inferred", () => {
    const doc = structuredClone(expansionOf("fy2026-adopted", config));
    const roots = core.rootFrontier(doc);
    const folded = core.foldDocument(doc, new Set(roots));
    // A ribbon that is one link of the document and completes no set, so
    // the fold keeps the mark this test puts on it.
    const lone = (/** @type {any} */ r) => doc.links.find((l) => !l.derived && !l.completes &&
      folded.links.some((f) => f.source === l.source && f.target === l.target && f.kind === l.kind &&
        f.fact_ids.length === l.fact_ids.length) && (l.source === r.id || l.target === r.id));
    const printed = core.sideTree(doc, roots, 0).roots.find((r) => !r.derived && lone(r) &&
      folded.links.filter((l) => l.source === r.id || l.target === r.id).length > 1);
    assert.ok(printed !== undefined, "no printed root sums two ribbons, one of them a lone link");
    lone(printed).derived = true;
    const again = core.sideTree(doc, roots, 0).roots.find((r) => r.id === printed.id);
    assert.ok(again.derived, `${printed.id} sums an inferred ribbon and reads as printed`);
  });
  test("a sum of links that disagree on their reduction is refused", () => {
    assert.throws(() => core.sumReading({}, [{ contra: "ERAF" }, {}], "x"), /x with links that disagree on the reduction/);
    const r = core.sumReading({ derived: true }, [{ contra: "ERAF" }, { contra: "ERAF" }], "x");
    assert.ok(r.derived && r.contra === "ERAF", JSON.stringify(r));
  });
  test("a tier whose roots both take in and pay out is refused", () => {
    const doc = expansionOf("fy2026-adopted", config);
    assert.throws(() => core.sideTree(doc, core.rootFrontier(doc), 2), /both takes in and pays out/);
  });
});

// Mutation: skip members in LEDGER's row and roots' marks go missing; drop
// fillTip's pageLinks line and the pages arm goes red.
describe("each form draws every tile once and its tip names its pages", () => {
  for (const form of Object.keys(forms)) {
    test(form, async (t) => {
      const { document, config } = await booted(form);
      assert.equal(refusal(document), "");
      const doc = expansionOf("fy2027-adopted", config);
      const roots = core.rootFrontier(doc);
      let expected = 0;
      for (const side of forms.ledger.LEDGER.sidesOf(config.overview)) {
        const tree = core.sideTree(doc, roots, side.tier);
        const section = document.querySelector(`section.sides-side[data-tier="${side.tier}"]`);
        const marks = [...section.querySelectorAll(MARK[form])];
        // The list's count and sum are the client's and say so. Mutation:
        // drop the ◇ from ledger.js's aria-label.
        if (form === "ledger") {
          const label = section.querySelector("[aria-label]").getAttribute("aria-label");
          assert.match(label, /: ◇ (?:1 bar by our count(?: that is not a printed reduction)?|(?:0|[2-9]|\d\d+) bars by our count(?: that are not printed reductions)?), .* in all by our sum$/, label);
        }
        const ids = marks.map((m) => m.dataset.id).sort();
        assert.deepEqual(ids, sideTiles(tree.roots).map((x) => x.tile.id).sort());
        for (const m of marks) {
          const tile = sideTiles(tree.roots).find((x) => x.tile.id === m.dataset.id).tile;
          assert.equal(Number(m.dataset.cents), tile.cents, `${m.dataset.id} carries other cents`);
          assert.equal(m.getAttribute("tabindex"), "0");
          assert.equal(m.getAttribute("role"), "button");
          // An img's children are presentational, so a button inside one
          // is no button. Mutation: role="img" on ledger.html.tmpl's
          // ledger-list.
          assert.ok(!m.parentElement.closest('[role="img"]'), `${m.dataset.id} is a button inside an image`);
        }
        const m = marks[0];
        m.dispatchEvent(new document.defaultView.Event("pointerenter", { bubbles: true }));
        const pages = sideTiles(tree.roots).find((x) => x.tile.id === m.dataset.id).tile.locators.flatMap((l) => l.pages.map((p) => `p${p}`));
        const facts = document.querySelector("#tooltip .facts").textContent;
        assert.ok(pages.length > 0, `${m.dataset.id} cites no page`);
        for (const p of pages) assert.ok(facts.split(/[,;] /).map((x) => x.replace(/ PDF$/, "")).includes(p), `${m.dataset.id}'s tip does not name ${p}`);
        assert.ok(section.querySelector(".sides-total").textContent.includes(format.dollars(tree.cents)));
        expected += marks.length;
      }
      t.diagnostic(`${form}: ${expected} marks across ${forms.ledger.LEDGER.sidesOf(config.overview).length} sides at the root`);
    });
  }
});

// Mutation: drop `tile.id` from toggle's opened filter and a second click
// reopens the node.
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

// A left-behind entry would be replayed on a frontier not drawing it and
// refused. Mutation: filter `opened` by the tile's id and `id~` prefix
// instead of closing() and both cases fail.
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

// Mutation: keep refused entries in frontierOf's `kept` and the address
// names an undrawn node.
test("a hash naming the other year and an opened node is restored, and a bad entry is refused by name", async () => {
  const browser = await booted("ledger", "#y=fy2026-adopted&o=revenue/taxes/property,no/such/node");
  const { document, window } = browser;
  assert.match(refusal(document), /no\/such\/node/);
  assert.equal(window.location.hash, "#y=fy2026-adopted&o=revenue/taxes/property");
  assert.equal(document.getElementById("year-select").value, "fy2026-adopted");
  const opened = document.querySelector('li.bar-row[data-id="revenue/taxes/property"]');
  assert.ok(opened.classList.contains("opened"));
  const reductions = [...document.querySelectorAll("li.bar-row.reduction")];
  assert.equal(reductions.length, 2, "the two printed reductions under Property Taxes are not drawn as reductions");
  // A printed reduction takes no share: not on its bar, in its label or in
  // its tooltip. Mutation: drop the contra test in
  // tileShare and each reads "◇ x%".
  for (const r of reductions) {
    assert.ok(Number(r.dataset.cents) < 0);
    assert.ok(r.querySelector(".bar-label").textContent.startsWith("− "));
    assert.equal(r.querySelector(".bar-share").textContent, "", `${r.dataset.id} states a share`);
    assert.ok(!r.getAttribute("aria-label").includes("◇"), `${r.dataset.id}'s label states a share`);
    r.dispatchEvent(new window.Event("pointerenter", { bubbles: true }));
    assert.ok(!document.querySelector("#tooltip .tip-meta").textContent.includes("◇"), `${r.dataset.id}'s tooltip states a share`);
  }
  // Nor is a reduction root in any share's total. Mutation: divide by
  // side.tree.cents and this reads 62.5%.
  const side = { tree: { cents: 80, roots: [{ contra: "", cents: 100 }, { contra: "a reduction", cents: -20 }] } };
  assert.equal(sides.tileShare(/** @type {any} */ ({ contra: "", cents: 50 }), /** @type {any} */ (side)), "◇ 50.0%");
  // And the list's label states the visible total's count and sum, not the
  // tree's. Mutation: state side.tree.roots.length and side.tree.cents in
  // ledger.js listLabel, and this reads "2 bars ... $0.80".
  assert.equal(forms.ledger.listLabel(/** @type {any} */ ({ ...side, heading: "In" })),
    "In: ◇ 1 bar by our count that is not a printed reduction, $1 in all by our sum");
  // One spelling of the count for every total and label, agreeing in
  // number. Mutation: always "bars", or always "that are not", in
  // countedBars, and a row goes red.
  assert.deepEqual([[1, 0], [1, 1], [2, 0], [2, 3]].map(([roots, reductions]) => sides.countedBars({ roots, reductions }, " here")),
    ["1 bar here", "1 bar here that is not a printed reduction", "2 bars here", "2 bars here that are not printed reductions"]);
  const two = { heading: "In", tree: { cents: 130, roots: [...side.tree.roots, { contra: "", cents: 50 }] } };
  assert.equal(forms.ledger.listLabel(/** @type {any} */ (two)),
    "In: ◇ 2 bars by our count that are not printed reductions, $1.50 in all by our sum");
  const plain = document.querySelector('li.bar-row[data-id="revenue/taxes/property"]');
  assert.match(plain.querySelector(".bar-share").textContent, /^◇ \d+\.\d%$/);
  assert.deepEqual(sides.parseHash("#y=nope&o=a,b~k", browser.config),
    { year: "fy2027-adopted.json", opened: ["a", "b~k"], refused: "No budget column is called nope, so this shows the newest." });
  assert.equal(sides.hashOf({ year: "fy2026-adopted.json", opened: ["a", "b~k"] }), "#y=fy2026-adopted&o=a,b~k");
});

// Mutation: pass `opened: []` on boot's year change and the node shuts; drop
// selectYear's carryYear call and the links keep the newest year.
test("a year switch keeps the opened node and redraws the other year's figures", async () => {
  const browser = await booted("ledger", "#o=revenue/taxes/sales");
  const { document, window, config } = browser;
  const was = document.querySelector('li.bar-row[data-id="revenue/taxes/sales"]');
  assert.ok(was.classList.contains("opened"));
  const select = document.getElementById("year-select");
  const flows = () => document.querySelector('nav.site-nav a[href^="flows.html"]').getAttribute("href");
  assert.equal(select.disabled, false, "the select ships disabled and the page never enabled it");
  assert.equal(flows(), "flows.html#y=fy2027-adopted");
  select.value = "fy2026-adopted";
  select.dispatchEvent(new window.Event("change"));
  await settle();
  assert.equal(window.location.hash, "#y=fy2026-adopted&o=revenue/taxes/sales");
  assert.equal(flows(), "flows.html#y=fy2026-adopted");
  const now = document.querySelector('li.bar-row[data-id="revenue/taxes/sales"]');
  assert.ok(now.classList.contains("opened"));
  const tree = core.sideTree(expansionOf("fy2026-adopted", config), core.expand(expansionOf("fy2026-adopted", config),
    core.rootFrontier(expansionOf("fy2026-adopted", config)), "revenue/taxes/sales", "lines"), 0);
  const tile = sideTiles(tree.roots).find((x) => x.tile.id === "revenue/taxes/sales").tile;
  assert.equal(Number(now.dataset.cents), tile.cents);
  assert.notEqual(Number(now.dataset.cents), Number(was.dataset.cents), "the two years print the same sales tax");
  assert.equal(document.getElementById("lede-year").textContent, "FY 2025-26 adopted");
});

// A switch to a column that fails leaves the select, the top bar's links
// and the hash on the column still drawn. Mutation: drop the selectYear in
// show's catch and the select names the year that failed.
test("a year switch whose column fails puts the select back on the year still drawn", async () => {
  const { document, window } = await booted("ledger", "", { "fy2026-adopted.json": null });
  const select = document.getElementById("year-select");
  select.value = "fy2026-adopted";
  select.dispatchEvent(new window.Event("change"));
  await settle();
  const flows = document.querySelector('nav.site-nav a[href^="flows.html"]').getAttribute("href");
  assert.deepEqual({ value: select.value, flows, hash: window.location.hash, refused: /could not be drawn/.test(refusal(document)) },
    { value: "fy2027-adopted", flows: "flows.html#y=fy2027-adopted", hash: "#y=fy2027-adopted", refused: true });
});

// Shift opens a node its alternative way where one is declared.
test("shift-click opens the alternative split, and the hash names it", async () => {
  const browser = await booted("ledger");
  const { document, window, config } = browser;
  const doc = expansionOf("fy2027-adopted", config);
  const two = sideTiles(core.sideTree(doc, core.rootFrontier(doc), 5).roots)
    .find((x) => x.tile.splits.some((sp) => sp.alternative)).tile;
  const alt = two.splits.find((sp) => sp.alternative);
  const mark = document.querySelector(`li.bar-row[data-id="${two.id}"]`);
  await click(browser, mark, { shiftKey: true });
  assert.equal(window.location.hash, `#y=fy2027-adopted&o=${two.id}~${alt.key}`);
  const members = [...document.querySelectorAll('li.bar-row[data-depth="1"]')];
  assert.equal(members.length, alt.members.length);
  assert.equal(members.reduce((a, m) => a + Number(m.dataset.cents), 0), two.cents);
  // Mutation: compare tile.open to the primary key on a plain click and the
  // first click below reopens instead of closing.
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
// Mutation: drop the getBoundingClientRect branch in format.js's placeTip
// and the tip stands at the viewport's corner.
test("the tooltip stands under a focused mark", async () => {
  const { document } = await booted("ledger");
  const tip = document.getElementById("tooltip");
  const target = { getBoundingClientRect: () => ({ left: 120, bottom: 340, top: 320, right: 400 }) };
  format.placeTip(document, tip, /** @type {any} */ ({ target }));
  assert.equal(tip.style.left, "134px");
  assert.equal(tip.style.top, "354px");
  format.placeTip(document, tip, /** @type {any} */ ({ clientX: 10, clientY: 20, target }));
  assert.equal(tip.style.left, "24px");
  assert.equal(tip.style.top, "34px");
});

// Mutation: drop the ◇ and "by our count" from opensAs and the meta arm goes
// red.
test("hover shows a tile's figure, share and pages, and leaving hides it", async () => {
  const browser = await booted("ledger");
  const { document, window } = browser;
  const mark = document.querySelector('li.bar-row[data-id="revenue/taxes/property"]');
  mark.dispatchEvent(new window.Event("pointerenter", { bubbles: true }));
  const tip = document.getElementById("tooltip");
  assert.equal(tip.hidden, false);
  assert.equal(tip.querySelector(".tip-label").textContent, "Property Taxes");
  assert.equal(tip.querySelector(".tip-value").textContent, format.dollars(Number(mark.dataset.cents)));
  assert.match(tip.querySelector(".tip-meta").textContent,
    /^◇ \d+\.\d% of where the money comes from\. Click to open: ◇ \d+ lines(; \d+ [a-z-]+ \(alternative\))*, by our count$/);
  // The root's ribbons are the fund-flows leaves folded, so it cites the
  // revenue-by-fund pages, pp.127-140, and not p66.
  assert.match(tip.querySelector(".facts").textContent, /^p1[2-4]\d PDF(, p1[2-4]\d PDF)*$/);
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
  // Mutation: drop boot's renderer.sidesOf and a chart with no sides fetches
  // a column before it refuses.
  const sideless = installBrowser({ html: html.replace('"ledger":{"sides"', '"ledger":{"none"') });
  const asked = plannedFetch({}, config.exported_by);
  sideless.window.fetch = asked;
  await forms.ledger.boot(sideless.window);
  await settle();
  assert.match(refusal(sideless.document), /could not read its config: .*declares no sides for its ledger chart/);
  assert.equal(asked.asked.length, 0, "a chart with no sides still fetched a column");
});

/** The page-text links the config's docs name for a tile's locators, in order and once each. */
function textsOf(config, tile) {
  return [...new Set(tile.locators.flatMap((l) => l.pages.map((p) => config.docs[l.doc_id].pages[String(p)].text)))];
}

// Mutation: drop fillTip's pageLinks and every tip's links go; drop
// pageLinks' PDF link and the PDF arm goes red; pin the toggled tile's tip
// after redraw and the opener arm goes red.
test("a click pins a tile's tip, linking the page text and the PDF page of every page it cites", async (t) => {
  const { pinGestures, tipState, pageTextExists, pdfFaults, fire } = await import("./testlib.mjs");
  const browser = await booted("ledger");
  const { window, document, config } = browser;
  const doc = expansionOf("fy2027-adopted", config);
  const tiles = new Map();
  for (const side of forms.ledger.LEDGER.sidesOf(config.overview)) {
    for (const x of sideTiles(core.sideTree(doc, core.rootFrontier(doc), side.tier).roots)) tiles.set(x.tile.id, x.tile);
  }
  const opener = [...document.querySelectorAll(MARK.ledger)].find((m) => m.classList.contains("can-open"));
  const id = opener.dataset.id;
  await click(browser, opener);
  assert.ok(!tipState(document).pinned, `${id}'s click pinned a tip over the members it opened`);
  assert.ok(document.querySelector(`${MARK.ledger}[data-id="${id}"]`).classList.contains("opened"), `${id} did not open`);
  // Every leaf now drawn, the opened tile's members among them.
  const frontier = core.expand(doc, core.rootFrontier(doc), id, core.splitsOf(doc, id).find((sp) => !sp.alternative).key);
  for (const side of forms.ledger.LEDGER.sidesOf(config.overview)) {
    for (const x of sideTiles(core.sideTree(doc, frontier, side.tier).roots)) tiles.set(x.tile.id, x.tile);
  }
  const leaves = [...document.querySelectorAll(MARK.ledger)].filter((m) => !m.classList.contains("can-open"));
  assert.ok(leaves.length >= 2, "fewer than two tiles that do not open");
  for (const m of leaves.slice(0, 1)) {
    await click(browser, m);
    const tip = tipState(document);
    assert.ok(tip.pinned && tip.shown, `${m.dataset.id}: no pinned tip`);
    const want = textsOf(config, tiles.get(m.dataset.id));
    assert.ok(want.length > 0, `${m.dataset.id} cites no page`);
    assert.deepEqual(tip.hrefs, want, `${m.dataset.id}: the tip links other pages`);
    for (const href of tip.hrefs) assert.ok(pageTextExists(href), `${href} names no committed page text`);
    assert.deepEqual(pdfFaults(tip), [], `${m.dataset.id}: the tip's PDF links are not its pages'`);
  }
  fire(document.body, "click");
  pinGestures(window, leaves[0], leaves[1]);
  t.diagnostic(`${id} opened with no tip pinned; ${leaves.length} tiles that do not open`);
});

// Mutation: pin the toggled tile's tip after redraw and the first Enter pins;
// drop show's focusDrawn and focus falls off the row; use a bare focus() not
// tips.focus and its listener shows a tip.
test("Enter opens a row and Enter again closes it, the focus kept on the row", async () => {
  const { tipState, fire } = await import("./testlib.mjs");
  const browser = await booted("ledger");
  const { document } = browser;
  const opener = [...document.querySelectorAll(MARK.ledger)].find((m) => m.classList.contains("can-open"));
  const id = opener.dataset.id;
  const row = () => document.querySelector(`${MARK.ledger}[data-id="${id}"]`);
  opener.focus();
  for (const want of [true, false]) {
    fire(document.activeElement, "keydown", { key: "Enter" });
    await settle();
    assert.equal(row().classList.contains("opened"), want, `${id}: Enter left it ${want ? "closed" : "open"}`);
    assert.ok(!tipState(document).pinned, `${id}: Enter pinned a tip`);
    assert.ok(!tipState(document).shown, `${id}: Enter left a tip shown over the row's members`);
    assert.ok(document.activeElement === row(), `${id}: the focus left the row`);
  }
});

// Mutation: drop ledger.js's P keydown arm and an opening row cannot pin from
// the keyboard; drop the figure's click listener and the click opens the
// row, pinning nothing.
test("a row on Bars pins a tip linking its pages, by P or, where it opens, by a click on its figure", async () => {
  const { tipState, fire } = await import("./testlib.mjs");
  const browser = await booted("ledger");
  const { document, config } = browser;
  const doc = expansionOf("fy2027-adopted", config);
  const opener = [...document.querySelectorAll(MARK.ledger)].find((m) => m.classList.contains("can-open"));
  const id = opener.dataset.id;
  await click(browser, opener);
  const frontier = core.expand(doc, core.rootFrontier(doc), id, core.splitsOf(doc, id).find((sp) => !sp.alternative).key);
  const tiles = new Map();
  for (const side of forms.ledger.LEDGER.sidesOf(config.overview)) {
    for (const x of sideTiles(core.sideTree(doc, frontier, side.tier).roots)) tiles.set(x.tile.id, x.tile);
  }
  const all = [...document.querySelectorAll(MARK.ledger)];
  assert.deepEqual(all.map((m) => m.dataset.id).sort(), [...tiles.keys()].sort());
  const rows = [all.find((m) => !m.classList.contains("can-open")), all.find((m) => m.classList.contains("can-open"))];
  assert.ok(rows[0] && rows[1], "the Bars fixture lacks a row that opens or one that does not, so an arm covers nothing");
  let openers = 0;
  for (const m of rows) {
    const want = textsOf(config, tiles.get(m.dataset.id));
    assert.ok(want.length > 0, `${m.dataset.id} cites no page`);
    m.focus();
    fire(m, "keydown", { key: "p" });
    await settle();
    let tip = tipState(document);
    assert.ok(tip.pinned && tip.shown, `${m.dataset.id}: P pinned no tip`);
    assert.deepEqual(tip.hrefs, want, `${m.dataset.id}: P's tip links other pages`);
    fire(document.body, "click");
    if (!m.classList.contains("can-open")) continue;
    openers++;
    const was = m.classList.contains("opened");
    await click(browser, m.querySelector(".bar-value"));
    tip = tipState(document);
    assert.ok(tip.pinned && tip.shown, `${m.dataset.id}: a click on its figure pinned no tip`);
    assert.deepEqual(tip.hrefs, want, `${m.dataset.id}: the figure's tip links other pages`);
    const now = document.querySelector(`${MARK.ledger}[data-id="${m.dataset.id}"]`);
    assert.ok(now === m && now.classList.contains("opened") === was, `${m.dataset.id}: a click on its figure opened or closed it`);
    fire(document.body, "click");
  }
  assert.equal(openers, 1, "no row that opens, so the figure arm covers nothing");
});
