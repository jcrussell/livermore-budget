// Each skeleton under testdata/client/ carries the DOM schema/bindings.json
// says its page binds, as Go holds every page it writes to the same entry
// (pkg/cmd/export TestEveryPageCarriesTheDOMItsModulesBind). The boot tests
// then run each module over its skeleton, so an id a module reads and the
// bindings omit is a skeleton the boot tests refuse -- except one read
// through maybeEl, which tolerates its absence silently.
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { JSDOM } from "jsdom";
import { repoRoot, withConfig } from "./testlib.mjs";

const BINDINGS = JSON.parse(readFileSync(join(repoRoot, "schema", "bindings.json"), "utf8"));

/**
 * Whether doc carries selector; one beginning template#<id> is matched
 * inside that template's content, where querySelector does not reach.
 */
function carries(doc, selector) {
  const m = /^template#([A-Za-z0-9_-]+)(?:\s+(>)?\s*(.*))?$/.exec(selector);
  if (!m) return doc.querySelector(selector) !== null;
  const tmpl = doc.getElementById(m[1]);
  if (!tmpl || tmpl.localName !== "template") return false;
  if (!m[3]) return true;
  const host = doc.createElement("div");
  host.append(tmpl.content.cloneNode(true));
  return host.querySelector((m[2] ? ":scope > " : ":scope ") + m[3]) !== null;
}

/** Every selector of page's entry and the chrome's that its skeleton lacks. */
function missing(page, html) {
  const doc = new JSDOM(html).window.document;
  const entry = BINDINGS.pages[page];
  return [...BINDINGS.chrome.selectors, ...entry.selectors].filter((s) => !carries(doc, s));
}

const skeleton = (page) => readFileSync(join(repoRoot, BINDINGS.pages[page].skeleton), "utf8");

test("every page schema/bindings.json binds has a skeleton carrying all of it", (t) => {
  const pages = Object.keys(BINDINGS.pages);
  t.diagnostic(`bound pages: ${pages.join(", ")}`);
  assert.ok(pages.length > 0 && BINDINGS.chrome.selectors.length > 0, "the bindings bind nothing");
  const faults = pages.flatMap((p) => missing(p, skeleton(p)).map((s) => `${BINDINGS.pages[p].skeleton} lacks ${s}`));
  assert.deepEqual(faults, []);
});

// Mutation proof: each row breaks one binding the way a template edit would,
// and the check above must name it.
test("the skeleton check names a binding a skeleton lost", () => {
  const flows = skeleton("flows.html");
  const rows = [
    ["an id renamed", 'id="legend"', 'id="legend2"', "#rail-content div.legend#legend"],
    ["an attribute dropped", 'id="filter-line" aria-live="polite" hidden', 'id="filter-line" aria-live="polite"',
      "p#filter-line[hidden] span#filter-said"],
    ["a child wrapped", '<div class="chart-body" id="chart-body">', '<div><div class="chart-body" id="chart-body">',
      "#chart-card > div.chart-body#chart-body"],
  ];
  for (const [name, from, to, want] of rows) {
    assert.ok(flows.includes(from), `${name}: the skeleton carries no ${from}`);
    assert.ok(missing("flows.html", flows.replace(from, to)).includes(want), name);
  }
  const ledger = skeleton("ledger.html");
  const from = '<span class="bar-value"></span>';
  assert.ok(ledger.includes(from));
  assert.ok(missing("ledger.html", ledger.replace(from, "")).includes("template#ledger-row .bar-value"),
    "a template's part dropped");
});

// Mutation: a string replacement in withConfig expands the `$'`, and an
// unescaped `<` ends the script at the `</script>`.
test("a config splices into its skeleton verbatim", () => {
  const config = { wording: "$' and $& and </script><!--" };
  const html = withConfig(`<p>a</p><script type="application/json" id="c"></script><p>b</p>`, "c", config);
  const doc = new JSDOM(html).window.document;
  assert.deepEqual(JSON.parse(doc.getElementById("c").textContent), config);
  assert.equal(doc.querySelectorAll("p").length, 2);
});
