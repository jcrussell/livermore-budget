// pins.test.mjs — every literal the client shares with Go or with the
// stylesheet is held to the other party's copy, so neither can move alone:
// the side a step opens from, the fund-group role, the viewport cushion, and
// the hue slots a column's fund groups are numbered into.

import { describe, test } from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import { loadApp, stylesheet, columnFixture, publishedColumns, repoRoot } from "./testlib.mjs";

const read = (name) => JSON.parse(fs.readFileSync(path.join(repoRoot, "schema", name), "utf8"));

describe("the literals the client shares", () => {
  test("SIDE_SOURCE is one of page.schema.json's sides", async (t) => {
    const { app } = await loadApp();
    const sides = read("page.schema.json").$defs.sankey_hints.properties.side.enum;
    t.diagnostic(`the schema's sides are ${JSON.stringify(sides)}; the client's is ${JSON.stringify(app.SIDE_SOURCE)}`);
    assert.ok(sides.includes(app.SIDE_SOURCE));
  });

  test("isFundGroup answers to exactly the fund_group role of enums.schema.json", async (t) => {
    const { app } = await loadApp();
    const roles = read("enums.schema.json").$defs.role.enum;
    const groups = roles.filter((role) => app.isFundGroup({ role }));
    t.diagnostic(`of ${roles.length} roles, the client treats ${JSON.stringify(groups)} as a fund group`);
    assert.deepEqual(groups, ["fund_group"]);
  });

  test("CHART_CUSHION is the stylesheet's --chart-cushion", async (t) => {
    const { app } = await loadApp();
    const found = [...stylesheet().matchAll(/--chart-cushion:\s*(\d+)px/g)].map((m) => Number(m[1]));
    t.diagnostic(`style.css sets --chart-cushion ${found.join(", ")}px; the client's CHART_CUSHION is ${app.CHART_CUSHION}`);
    assert.deepEqual(found, [app.CHART_CUSHION]);
  });

  test("every hue slot a published column numbers its fund groups into has a --fund-slot in the stylesheet", (t) => {
    const slots = new Set([...stylesheet().matchAll(/--fund-slot-(\d+):/g)].map((m) => Number(m[1])));
    const needed = new Map();
    for (const stem of publishedColumns()) {
      for (const g of columnFixture(stem).fund_groups) {
        if (g.slot) needed.set(g.slot, (needed.get(g.slot) || []).concat(stem + " " + g.id));
      }
    }
    t.diagnostic(`style.css defines slots ${[...slots].sort((a, b) => a - b).join(", ")}; the published columns use ${[...needed.keys()].sort((a, b) => a - b).join(", ")}`);
    assert.ok(needed.size > 0, "no published column numbers a fund group, so this proves nothing");
    assert.deepEqual([...needed.keys()].filter((n) => !slots.has(n)), []);
  });
});
