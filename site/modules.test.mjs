// The shipped modules app.js imports are instantiated once per process and
// shared by every test, so they may hold no state and read no global at
// import. This file imports them with nothing on the global, deliberately
// without testlib: loading d3 or a page first would hide the defect.
import { describe, test } from "node:test";
import assert from "node:assert/strict";
import path from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const HERE = path.dirname(fileURLToPath(import.meta.url));

describe("the shipped modules hold no state", () => {
  test("core.js imports with no page config and no d3 on the global, and reads the config at call time", async (t) => {
    delete globalThis.FISC_CONFIG;
    delete globalThis.d3;
    assert.equal(typeof globalThis.document, "undefined", "nothing installed a DOM before the import");
    const core = await import(pathToFileURL(path.join(HERE, "core.js")).href);
    assert.equal(typeof core.say, "function");
    assert.equal(typeof core.foldDocument, "function");
    // Two configs in turn: a module that read the config at import would
    // answer from the first, or from nothing.
    globalThis.FISC_CONFIG = { wording: { count: "first: {n:thing|things}" } };
    const first = core.say("count", { n: 1 });
    globalThis.FISC_CONFIG = { wording: { count: "second: {n:thing|things}" } };
    const second = core.say("count", { n: 2 });
    delete globalThis.FISC_CONFIG;
    t.diagnostic(`say answered "${first}" then "${second}" from two configs set after the import`);
    assert.equal(first, "first: 1 thing");
    assert.equal(second, "second: 2 things");
    // Every export is a function or a primitive, bar the two number
    // formatters, which hold no state of this page's: a new object export is
    // named here or it is a binding every test would share.
    const objects = Object.entries(core).filter(([, v]) => typeof v === "object").map(([k]) => k).sort();
    assert.deepEqual(objects, ["money", "moneyCompact"]);
  });
});
