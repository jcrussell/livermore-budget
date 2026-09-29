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

describe("a node's annotations are the column's, its parent the schedule's", () => {
  test("a fund's constraint tier reads the same out of every schedule that draws it, and a division hangs under the General Fund in one schedule and under nothing in another", async (t) => {
    const app = await module();
    const column = columnFixture("fy2026-adopted");
    const funds = column.nodes.filter((n) => n.id.startsWith("fund/") && n.constraint_tier);
    assert.ok(funds.length > 0, "the column's table carries a fund with a constraint tier");
    let compared = 0;
    for (const key of ["fund-flows", "department-funding", "transfers-by-fund", "transfers-out"]) {
      const doc = app.scheduleOf(column, key);
      for (const n of doc.nodes.filter((x) => x.id.startsWith("fund/"))) {
        const table = column.nodes.find((x) => x.id === n.id);
        assert.equal(n.constraint_tier, table.constraint_tier || "", `${key} ${n.id} tier`);
        assert.equal(n.rationale, table.rationale || "", `${key} ${n.id} rationale`);
        assert.equal(n.source_note, table.source_note || "", `${key} ${n.id} source note`);
        compared++;
      }
    }
    t.diagnostic(`${compared} fund nodes across four schedules read their annotations off the table`);
    assert.ok(compared > 0);
    const division = (key) => app.scheduleOf(column, key).nodes.find((n) => n.id.startsWith("dept/"));
    const drill = division("fund-flows");
    const spending = app.scheduleOf(column, "department-spending").nodes.find((n) => n.id === drill.id);
    assert.ok(spending, `${drill.id} is drawn by both schedules`);
    assert.equal(drill.parent, "fund/100", "pp.167-170 hang a division under the General Fund");
    assert.equal(spending.parent, "", "pp.85-125 carry no fund, so the same division hangs under nothing");
  });
});

describe("a column's schedules are assembled once", () => {
  // stepDecomposes reads a step's schedule per mark per paint, and decomposable
  // caches on the document object, so one column's schedule is one object --
  // frozen, so no reader can change it under the others.
  test("scheduleOf returns one frozen document per column and schedule", async () => {
    const app = (await loadApp()).app;
    const column = columnFixture("fy2026-adopted");
    const first = app.scheduleOf(column, "fund-flows");
    assert.ok(first, "the pinned column carries no fund-flows schedule");
    assert.equal(app.scheduleOf(column, "fund-flows"), first, "a second read assembled a second document");
    assert.ok(Object.isFrozen(first) && Object.isFrozen(first.links[0]), "the shared document can be changed by a reader");
    assert.notEqual(app.scheduleOf(structuredClone(column), "fund-flows"), first, "another column's schedule is this one's");
  });
});
