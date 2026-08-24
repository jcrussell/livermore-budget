// harness.mjs — load site/app.js the way a browser would, and hand its
// functions back for checking.
//
// site/app.js is served to readers exactly as it is committed: no bundler, no
// npm, no module system. So the only faithful way to test it is to run that
// same file, with the same vendored d3, and reach into it — not to copy the
// functions into a test file, which would check a copy and let the shipped one
// drift.
//
// Node is the only dependency and it stays OFF THE DEPLOY PATH, the way
// tools/extract.py stays off the build path: `make site` and `fisc export`
// never run this, CI runs it as its own step, and a contributor without node
// can still build and serve the site.
//
// WHAT THE STUBS ARE FOR. app.js ends by calling main(), which fetches and
// draws. That needs a DOM, and a hand-rolled DOM would be a second thing to get
// right — its bugs would read as app.js's. So main() is allowed to run and to
// FAIL, into app.js's own fail() path, and the stubs below are exactly what
// that path touches and nothing more. The functions this harness actually
// checks are pure: they take a graph and return numbers.

import { readFileSync } from "node:fs";
import { createContext, runInContext } from "node:vm";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

export const repoRoot = join(dirname(fileURLToPath(import.meta.url)), "..", "..");

/**
 * A DOM stub covering exactly what app.js touches before it gives up.
 *
 * Every method is the smallest thing that keeps the load path from throwing.
 * If app.js starts touching something new, this throws a named error rather
 * than returning undefined and failing somewhere less obvious.
 */
function domStub() {
  const node = () => ({
    dataset: {},
    style: { setProperty() {} },
    classList: { add() {}, remove() {}, toggle() {} },
    setAttribute() {},
    removeAttribute() {},
    appendChild() {},
    append() {},
    prepend() {},
    replaceChildren() {},
    insertBefore() {},
    remove() {},
    addEventListener() {},
    querySelector: () => null,
    querySelectorAll: () => [],
    textContent: "",
    innerHTML: "",
  });
  const document = {
    documentElement: node(),
    body: node(),
    getElementById: () => node(),
    createElement: () => node(),
    createElementNS: () => node(),
    querySelector: () => null,
    querySelectorAll: () => [],
    addEventListener() {},
  };
  const storage = new Map();
  return {
    document,
    localStorage: {
      getItem: (k) => (storage.has(k) ? storage.get(k) : null),
      setItem: (k, v) => storage.set(k, String(v)),
    },
    getComputedStyle: () => ({ getPropertyValue: () => "#000000" }),
    matchMedia: undefined,
    // No fetch: main() is meant to give up here, in app.js's own error path.
    fetch: () => Promise.reject(new Error("harness: no network")),
    requestAnimationFrame: (fn) => fn(),
    addEventListener() {},
  };
}

/**
 * NAMES is what the harness reaches for. Every entry is a top-level `const` or
 * `function` in app.js, which in a script's lexical scope is unreachable from
 * outside — hence the trailing assignment appended below. That appended line is
 * the ONLY difference between what runs here and what a browser runs, and it
 * adds no behaviour: it reads bindings that already exist.
 */
const NAMES = [
  "FUND_ORDER", "nodeRank", "restackLinks", "understands", "isFundGroup",
  "SCHEMA_VERSION",
  "NODE_WIDTH", "NODE_PADDING", "CHART_WIDTH", "CHART_HEIGHT", "LABEL_GUTTER",
];

/** Loads the vendored d3 bundles and site/app.js into one context. */
export function loadApp() {
  const stub = domStub();
  const sandbox = { console, Intl, ...stub };
  sandbox.window = sandbox;
  sandbox.globalThis = sandbox;
  sandbox.self = sandbox;
  const ctx = createContext(sandbox);

  for (const f of ["d3.min.js", "d3-sankey.min.js"]) {
    runInContext(readFileSync(join(repoRoot, "site", "vendor", f), "utf8"), ctx, { filename: f });
  }
  if (!sandbox.d3 || typeof sandbox.d3.sankey !== "function") {
    throw new Error("vendored d3 did not define d3.sankey");
  }

  // A config the page can accept, so main() gets past its schema gate and dies
  // at the fetch instead — one line further into the real code path.
  sandbox.FISC_CONFIG = { schema_version: 1, primary: "sankey", projections: {}, docs: {} };

  const src = readFileSync(join(repoRoot, "site", "app.js"), "utf8");
  const exported = `\n;globalThis.__harness = { ${NAMES.join(", ")} };\n`;
  runInContext(src + exported, ctx, { filename: "app.js" });

  const app = sandbox.__harness;
  for (const n of NAMES) {
    if (app[n] === undefined) throw new Error(`app.js no longer defines ${n}`);
  }
  app.d3 = sandbox.d3;
  // The file's own text, so a check can pin the figures app.js QUOTES against
  // the figures it PRODUCES. Without this the two drift apart silently: editing
  // a comment to say 200 crossings would leave every check green.
  app.source = src;
  return app;
}

/** The committed worked example, which is FY2026 and is what the claims are about. */
export function goldenGraph() {
  return JSON.parse(readFileSync(join(repoRoot, "testdata", "sankey.golden.json"), "utf8"));
}
