// schedule.test.mjs — scheduleOf: one schedule of a published column,
// rehydrated into the document shape the rest of the client draws from.

import { describe, test } from "node:test";
import assert from "node:assert/strict";

import { loadApp, columnFixture, pageFixture } from "./testlib.mjs";

/** The shipped module with the pinned config, no page booted. */
async function module() {
  return (await loadApp({ config: pageFixture().config })).app;
}

describe("a schedule read out of a column", () => {
  test("carries its scope set as the list the column states, on the spine and on the three-schedule drill-down", async (t) => {
    const app = await module();
    const column = columnFixture("fy2026-adopted");
    for (const key of ["sankey", "fund-flows", "transfers-out"]) {
      const doc = app.scheduleOf(column, key);
      assert.ok(doc, `${key} is a schedule of the column`);
      assert.deepEqual(doc.metadata.scopes, column.schedules[key].scopes,
        `${key}'s metadata.scopes is the schedule's own list`);
      assert.ok(Array.isArray(doc.metadata.scopes) && doc.metadata.scopes.length >= 1,
        `${key} states at least one scope`);
      assert.equal("scope" in doc.metadata, false, `${key} states no singular scope`);
      t.diagnostic(`${key}: scopes ${JSON.stringify(doc.metadata.scopes)}`);
    }
    assert.equal(app.scheduleOf(column, "fund-flows").metadata.scopes.length, 3,
      "the drill-down is of three schedules");
  });

  test("a schedule the column does not carry is null, not a document with nothing in it", async () => {
    const app = await module();
    assert.equal(app.scheduleOf(columnFixture("fy2026-adopted"), "no-such-schedule"), null);
  });
});
