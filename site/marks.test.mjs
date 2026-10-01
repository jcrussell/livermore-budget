// Every mark the client makes -- the aggregate a cap folds a tail into, the
// residual carrying what a document does not decompose, the gap holding a
// licensed difference -- is held to schema/mark.schema.json. Go writes no mark
// and validates nothing against this schema, so this test reads it (a test
// may; the client never does) and checks the subset tools/extract.py checks:
// required keys, types, enums, patterns, consts, and that nothing is outside
// the declared properties.
import { describe, test } from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { bootedApp, everyOffer, settle, columnFixture, pageFixture } from "./testlib.mjs";

const SCHEMA_DIR = path.join(path.dirname(fileURLToPath(import.meta.url)), "..", "schema");
const read = (name) => JSON.parse(fs.readFileSync(path.join(SCHEMA_DIR, name), "utf8"));
const MARK = read("mark.schema.json");

/** Resolves a $ref by its file's basename and fragment. */
function resolve(ref) {
  const [url, fragment] = ref.split("#");
  let node = read(path.basename(url));
  for (const key of (fragment || "").split("/").filter(Boolean)) node = node[key];
  return node;
}

/** The subset check: every violation as "path: what", none for a conforming value. */
function check(schema, value, at, out) {
  if (schema.$ref) return check(resolve(schema.$ref), value, at, out);
  const type = Array.isArray(value) ? "array" : value === null ? "null" : typeof value;
  const want = schema.type === "integer" ? (Number.isInteger(value) ? "integer" : type) : type;
  if (schema.type && want !== schema.type) out.push(`${at}: is ${want}, not ${schema.type}`);
  if ("const" in schema && value !== schema.const) out.push(`${at}: is ${JSON.stringify(value)}, not ${JSON.stringify(schema.const)}`);
  if (schema.enum && !schema.enum.includes(value)) out.push(`${at}: ${JSON.stringify(value)} is not one of ${schema.enum.join(", ")}`);
  if (schema.minLength !== undefined && typeof value === "string" && value.length < schema.minLength) out.push(`${at}: shorter than ${schema.minLength}`);
  if (schema.pattern && typeof value === "string" && !new RegExp(schema.pattern).test(value)) out.push(`${at}: "${value}" does not match ${schema.pattern}`);
  if (schema.minimum !== undefined && typeof value === "number" && value < schema.minimum) out.push(`${at}: ${value} is below ${schema.minimum}`);
  if (schema.minItems !== undefined && Array.isArray(value) && value.length < schema.minItems) out.push(`${at}: fewer than ${schema.minItems} items`);
  if (schema.items && Array.isArray(value)) value.forEach((v, i) => check(schema.items, v, `${at}[${i}]`, out));
  if (value && typeof value === "object" && !Array.isArray(value)) {
    for (const key of schema.required || []) if (!(key in value)) out.push(`${at}: missing ${key}`);
    const props = schema.properties || {};
    for (const [key, v] of Object.entries(value)) {
      if (key in props) check(props[key], v, `${at}.${key}`, out);
      else if (schema.additionalProperties === false) out.push(`${at}: ${key} is not a declared property`);
    }
  }
  for (const clause of schema.allOf || []) {
    if (clause.if) {
      const applies = check(clause.if, value, at, []).length === 0;
      if (applies && clause.then) check(clause.then, value, at, out);
      if (!applies && clause.else) check(clause.else, value, at, out);
    } else check(clause, value, at, out);
  }
  return out;
}

describe("the marks this client makes", () => {
  test("every mark on every rung the page offers matches schema/mark.schema.json, and all three roles are drawn", async (t) => {
    const { app } = await bootedApp();
    /** @type {Map<string, {role: string, where: string, faults: string[]}>} */
    const seen = new Map();
    const walked = await everyOffer(app, async (path) => {
      for (const n of app.projection.nodes) {
        if (!app.isMark(n.id)) continue;
        const key = path.join(" > ") + " :: " + n.id;
        seen.set(key, { role: n.role, where: path.join(" > "), faults: check(MARK, n, n.id, []) });
      }
    });
    assert.equal(walked.refused, "", walked.refused);
    const roles = new Map();
    for (const m of seen.values()) roles.set(m.role, (roles.get(m.role) || 0) + 1);
    const faults = [...seen.values()].flatMap((m) => m.faults.map((f) => `${m.where}: ${f}`));
    t.diagnostic(`${walked.visited} rung(s) walked; marks by role: ${[...roles].map(([r, n]) => `${r} ${n}`).join(", ")}; faults: ${faults.length}`);
    assert.deepEqual(faults, []);
    for (const role of resolve(MARK.properties.role.$ref).enum) {
      assert.ok(roles.has(role), `no rung drew a ${role}, so the schema's ${role} clause was never exercised`);
    }
  });

  test("a mark built outside the drawn page matches too, and a stray key is caught", async () => {
    const { app } = await bootedApp();
    const gap = app.gapMark("expenditure/x", 5, 25000000, "Why.", "Where from.", [{ doc_id: "d", pages: [1] }]);
    assert.deepEqual(check(MARK, gap, "gap", []), []);
    assert.deepEqual(check(MARK, Object.assign({}, gap, { extra: 1 }), "gap", []), ["gap: extra is not a declared property"]);
    const residual = app.residualMark("fund-group/general", 3, "fund", 10, 20, "Why.", "Where from.");
    assert.deepEqual(check(MARK, residual, "residual", []), []);
    const one = app.aggregateMark(3, "fund-group/general", ["fund/1"], 9, 8, "funds");
    assert.deepEqual(check(MARK, one, "aggregate", []), ["aggregate.folds: fewer than 2 items"]);
  });

  test("a window refuses a kept flank carrying a residual or a gap from the rung above", async (t) => {
    const { app } = await bootedApp();
    const config = pageFixture().config;
    const fund = config.steps.find((s) => s.key === "fund");
    const column = columnFixture("fy2027-adopted");
    const stepDoc = app.scheduleOf(column, "fund-flows");
    // A plausible on-screen chart: the fund-group rung's document, with a
    // residual planted on the tier the fund step keeps, feeding the fund.
    const onScreen = app.scheduleOf(column, "fund-flows");
    const kept = fund.sankey.keep[0];
    const planted = Object.assign({}, onScreen, {
      nodes: onScreen.nodes.concat([app.residualMark("fund-group/general", kept, "fund", 1, 0, "Why.", "From.")]),
      links: onScreen.links.concat([{ source: app.residualID("fund-group/general"), target: "fund/100", value_cents: 1,
        kind: "external", transfer_id: "", fact_ids: [], locators: [], derived: true, partition: false, contra: "" }]),
    });
    const rung = { id: "fund/100", step: fund, doc: stepDoc };
    let message = "";
    try { app.windowFor(planted, stepDoc, rung); } catch (e) { message = e.message; }
    t.diagnostic(`the window said: ${message}`);
    assert.match(message, /kept flank carries residual\//);
    await settle();
  });
});
