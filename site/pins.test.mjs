// pins.test.mjs — every literal the client shares with Go or with the
// stylesheet is held to the other party's copy, so neither can move alone:
// the fund-group role, the mark prefixes, the viewport cushion, the rail's
// and the drawer's widths, and the hue slots a column's fund groups are
// numbered into.

import { describe, test } from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import { loadApp, stylesheet, columnFixture, publishedColumns, repoRoot } from "./testlib.mjs";

const read = (name) => JSON.parse(fs.readFileSync(path.join(repoRoot, "schema", name), "utf8"));

describe("the literals the client shares", () => {
  test("isFundGroup answers to exactly the fund_group role of enums.schema.json", async (t) => {
    const { app } = await loadApp();
    const roles = read("enums.schema.json").$defs.role.enum;
    const groups = roles.filter((role) => app.isFundGroup({ role }));
    t.diagnostic(`of ${roles.length} roles, the client treats ${JSON.stringify(groups)} as a fund group`);
    assert.deepEqual(groups, ["fund_group"]);
  });

  test("the mark prefixes are mark.schema.json's id pattern, in role order", async (t) => {
    const { app } = await loadApp();
    const pattern = read("mark.schema.json").properties.id.pattern;
    const m = /^\^\(([^()|]+(?:\|[^()|]+)*)\)$/.exec(pattern);
    const prefixes = [app.AGGREGATE_PREFIX, app.SMALLER_PREFIX];
    t.diagnostic(`the schema's id pattern is ${pattern}; the client's prefixes are ${JSON.stringify(prefixes)}`);
    assert.ok(m, `the id pattern ${pattern} is not ^(a|b|c)`);
    assert.deepEqual(m[1].split("|"), prefixes);
  });

  test("CHART_CUSHION is the stylesheet's --chart-cushion", async (t) => {
    const { app } = await loadApp();
    const found = [...stylesheet().matchAll(/--chart-cushion:\s*(\d+)px/g)].map((m) => Number(m[1]));
    t.diagnostic(`style.css sets --chart-cushion ${found.join(", ")}px; the client's CHART_CUSHION is ${app.CHART_CUSHION}`);
    assert.deepEqual(found, [app.CHART_CUSHION]);
  });

  test("RAIL_WIDTH and DRAWER_WIDTH are the stylesheet's --rail-width and --drawer-width", async (t) => {
    const { app } = await loadApp();
    const css = stylesheet();
    const found = (name) => [...css.matchAll(new RegExp(`--${name}:\\s*(\\d+)px`, "g"))].map((m) => Number(m[1]));
    const got = { rail: found("rail-width"), drawer: found("drawer-width") };
    t.diagnostic(`style.css sets --rail-width ${got.rail.join(", ")}px and --drawer-width ${got.drawer.join(", ")}px; ` +
      `the client's are ${app.RAIL_WIDTH} and ${app.DRAWER_WIDTH}`);
    assert.deepEqual(got, { rail: [app.RAIL_WIDTH], drawer: [app.DRAWER_WIDTH] });
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
