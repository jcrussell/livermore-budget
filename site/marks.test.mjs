// Every mark the client makes -- the tail a cap folds a split's or a tier's
// smallest members into, and the smaller-flows mark the smallest-flow filter
// folds small ribbons onto -- is held to schema/mark.schema.json. Go writes no mark
// and validates nothing against this schema, so this test reads it (a test
// may; the client never does) and checks the subset tools/extract.py checks:
// required keys, types, enums, patterns, consts, and that nothing is outside
// the declared properties.
import { describe, test } from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { bootedApp, expansionStates, columnFixture, pageFixture } from "./testlib.mjs";

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

/**
 * Every mark a chart of the expansion draws, once, with no DOM: the chart as
 * the page shapes it (what app.projection holds after a redraw), and the
 * marks the page's filter draws on top of it (what chartDrawn holds), at one
 * of expansionStates' frontiers.
 */
function marksAt(app, st) {
  const shaped = app.formOnScreen().shape(app.fetched, { chart: app.chartOnScreen(), frontier: st.frontier, whole: st.whole });
  const byID = new Map();
  for (const n of shaped.nodes.concat(app.filterFor(shaped).doc.nodes)) if (app.isMark(n.id)) byID.set(n.id, n);
  return [...byID.values()];
}

describe("the marks this client makes", () => {
  test("every mark on every split the page offers matches schema/mark.schema.json, and every role the page draws is drawn", async (t) => {
    const { app } = await bootedApp();
    // A smallest flow, so the smaller-flows marks are drawn beside the rest.
    app.setFilter({ smallest: app.SMALLEST_STOPS[3] });
    /** @type {Map<string, {role: string, where: string, faults: string[]}>} */
    const seen = new Map();
    const charts = expansionStates(app, app.fetched);
    for (const st of charts) {
      for (const n of marksAt(app, st)) {
        const key = st.where + " :: " + n.id;
        seen.set(key, { role: n.role, where: st.where, faults: check(MARK, n, n.id, []) });
      }
    }
    const roles = new Map();
    for (const m of seen.values()) roles.set(m.role, (roles.get(m.role) || 0) + 1);
    const faults = [...seen.values()].flatMap((m) => m.faults.map((f) => `${m.where}: ${f}`));
    t.diagnostic(`${charts.length} chart(s) walked; marks by role: ${[...roles].map(([r, n]) => `${r} ${n}`).join(", ")}; faults: ${faults.length}`);
    assert.deepEqual(faults, []);
    // The page builds no other kind of mark.
    assert.deepEqual([...roles.keys()].sort(), ["aggregate", "smaller"]);
  });

  test("every tail the expansion's split caps fold, everything opened by its primary splits, matches too", async (t) => {
    const { app } = await bootedApp();
    const faults = [];
    let tails = 0;
    for (const year of pageFixture().config.years) {
      const doc = app.scheduleOf(columnFixture(year.path.replace(/\.json$/, "")), "expansion");
      let all = app.rootFrontier(doc);
      for (let changed = true; changed;) {
        changed = false;
        for (const id of [...all]) {
          const splits = app.splitsOf(doc, id);
          if (!splits.length || app.openSplitOf(doc, all, id) !== "") continue;
          all = app.expand(doc, all, id, splits.find((sp) => !sp.alternative).key);
          changed = true;
        }
      }
      for (const n of app.expansionOf(doc, all, app.CONFIG.overview, new Set()).nodes) {
        if (!app.isMark(n.id)) continue;
        tails++;
        faults.push(...check(MARK, n, `${year.label} ${n.id}`, []));
      }
    }
    t.diagnostic(`${tails} tail(s) over the pinned years; faults: ${faults.length}`);
    assert.ok(tails > 0, "no split's tail was folded, so nothing here was checked");
    assert.deepEqual(faults, []);
  });

  test("a mark built outside the drawn page matches too, and a stray key is caught", async () => {
    const { app } = await bootedApp();
    const smaller = app.smallerMark(app.SMALLER_TO, 5, 3, 1000000, 120000, [{ doc_id: "d", pages: [1] }]);
    assert.deepEqual(check(MARK, smaller, "smaller", []), []);
    assert.deepEqual(check(MARK, Object.assign({}, smaller, { extra: 1 }), "smaller", []), ["smaller: extra is not a declared property"]);
    const one = app.aggregateMark(3, "fund-group/general", ["fund/1"], 9, 8, "funds");
    assert.deepEqual(check(MARK, one, "aggregate", []), ["aggregate.folds: fewer than 2 items"]);
  });

  test("every mark's words are the page's wording templates filled, on every split the page offers", async (t) => {
    // Every template prefixed with its own key: a word the client spelled
    // itself carries no prefix, and a placeholder it never filled stays as
    // braces.
    const config = structuredClone(pageFixture().config);
    for (const key of Object.keys(config.wording)) config.wording[key] = `«${key}» ` + config.wording[key];
    const { app } = await bootedApp({ config });
    app.setFilter({ smallest: app.SMALLEST_STOPS[3] });
    const wordsOf = { aggregate: ["aggregate_label", "aggregate_rationale", "aggregate_note"],
      smaller: ["smaller_label", "smaller_rationale", "smaller_note"] };
    /** @type {string[]} */
    const faults = [];
    const roles = new Map();
    let together = 0;
    const charts = expansionStates(app, app.fetched);
    for (const { where, ...st } of charts) {
      for (const n of marksAt(app, st)) {
        roles.set(n.role, (roles.get(n.role) || 0) + 1);
        const [label, rationale, note] = wordsOf[n.role];
        const at = where + " :: " + n.id;
        for (const [field, key] of [["label", label], ["rationale", rationale], ["source_note", note]]) {
          const text = String(n[field]);
          if (!text.startsWith(`«${key}» `)) faults.push(`${at}: ${field} is not ${key}: ${JSON.stringify(text.slice(0, 60))}`);
          if (/\{\w+(:[^|}]*\|[^}]*)?\}/.test(text)) faults.push(`${at}: ${field} keeps a placeholder: ${JSON.stringify(text)}`);
        }
        if (n.role === "aggregate" && n.source_note.includes("«aggregate_together» ")) together++;
      }
    }
    t.diagnostic(`${charts.length} chart(s); marks by role: ${[...roles].map(([r, n]) => `${r} ${n}`).join(", ")}; ` +
      `${together} aggregate note(s) carry the figure clause`);
    assert.deepEqual(faults, []);
    for (const role of Object.keys(wordsOf)) assert.ok(roles.has(role), `no chart drew a ${role}`);
    assert.ok(together > 0, "no aggregate note carried aggregate_together, so that template was never exercised");
  });
});
