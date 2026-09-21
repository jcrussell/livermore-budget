// contract.mjs — the boundary between Go's vetting and the client's drawing.
//
// site/app.js used to re-check, key by key, that a fetched document carried
// what the draw dereferences: eight arms over a column and four over the rung
// answer. Every one of those keys is `required` in schema/, and encodeColumn
// and encodeRungs refuse to write bytes that do not match, so the client was a
// second implementation of a check Go already makes -- kept in step by hand and
// by nothing else (AGENTS.md, "Go vets, JavaScript renders").
//
// DELETING A GATE MOVES A CLAIM, IT DOES NOT RETIRE ONE. This module is where
// the moved claims live, and it is the reason the deletion is safe rather than
// merely argued:
//
//   (1) Every key the client stopped refusing is still REQUIRED by the schema.
//       Relaxing the schema reddens this, which is the failure the deletion
//       would otherwise open up silently.
//   (2) app.js carries no per-key shape refusal, so the deletion cannot drift
//       back one arm at a time -- which is how the list got to eight.
//
// It reads the committed schemas with JSON.parse and app.js as source. No npm,
// no validator, no second engine: the same trade tools/extract.py makes on the
// Python side, where only the language that can have a dependency has one.

import { readFileSync } from "node:fs";
import { join } from "node:path";
import { loadApp, repoRoot, goFields, KNOWN_STEP_FIELDS } from "./harness.mjs";

const schema = (name) =>
  JSON.parse(readFileSync(join(repoRoot, "schema", name), "utf8"));

/**
 * Walks a dotted path through a JSON Schema and reports whether the last
 * segment is `required` where it is declared.
 *
 * `*` steps through additionalProperties (the column's schedule map) and array
 * items are stepped through implicitly, because a path names keys and not
 * positions. A path that does not resolve is an ERROR and not a false: a typo
 * here would otherwise read as "the schema does not require it" and send the
 * next reader to fix the wrong file.
 */
function requires(root, path) {
  const parts = path.split(".");
  let node = root;
  for (let i = 0; i < parts.length; i++) {
    while (node && node.items) node = node.items;
    const key = parts[i];
    const holder = node;
    node = key === "*"
      ? holder.additionalProperties
      : (holder.properties || {})[key];
    if (!node) throw new Error(`${path}: no schema declares ${key}`);
    if (i === parts.length - 1 && key !== "*") {
      return Array.isArray(holder.required) && holder.required.includes(key);
    }
  }
  throw new Error(`${path} names no key`);
}

/**
 * The subschema at a dotted path, by the same walk [requires] takes.
 *
 * A path that does not resolve is an ERROR for that function's reason: a typo
 * would otherwise read as "the schema says nothing about it" and send the next
 * reader to fix the wrong file.
 */
function at(root, path) {
  let node = root;
  for (const key of path.split(".")) {
    while (node && node.items) node = node.items;
    node = key === "*"
      ? node.additionalProperties
      : (node.properties || {})[key];
    if (!node) throw new Error(`${path}: no schema declares ${key}`);
  }
  while (node && node.items && node.type !== "array") node = node.items;
  return node;
}

// WHAT drawableSankey REFUSED, one row per arm it had on the day it was
// deleted, against where the same key is required now. scheduleOf rehydrates a
// schedule into {nodes, links, metadata}, so the client's `metadata.sources` is
// the schedule's own `sources` and its `nodes` are the schedule's.
const COLUMN_KEYS = [
  ["nodes", "nodes"],
  ["nodes (this schedule's)", "schedules.*.nodes"],
  ["links", "schedules.*.links"],
  ["links[].fact_ids", "schedules.*.links.fact_ids"],
  ["links[].locators", "schedules.*.links.locators"],
  ["metadata.sources", "schedules.*.sources"],
];

// The same, for readableRungs.
const RUNGS_KEYS = [
  ["columns", "columns"],
  ["columns[].stem", "columns.stem"],
  ["columns[].rungs", "columns.rungs"],
  ["columns[].rungs[].path", "columns.rungs.path"],
  ["columns[].rungs[].draws", "columns.rungs.draws"],
  ["columns[].rungs[].draws[].tier", "columns.rungs.draws.tier"],
  ["columns[].rungs[].draws[].ids", "columns.rungs.draws.ids"],
];

// A LOCATOR IS $ref'd AND SO IS CHECKED ON ITS OWN. Both `links[].locators[]`
// and the schedule's `sources[]` are locators, and `pages` is the key two of
// drawableSankey's arms went one element deeper for.
const LOCATOR_KEYS = [["locators[].pages / sources[].pages", "pages"]];

// WHAT STEPS AND RENDER_TIERS REFUSED, one row per part of the filter they
// carried. The page config is not FETCHED -- it is rendered into the page's own
// script -- so the client had no cache question to ask about it and the filter
// was pure shape.
//
// IT WAS WORSE THAN A REFUSAL AND THAT IS WHY IT HAD TO GO, not merely
// redundant: STEPS DROPPED what it did not recognise. A step whose `key` tag
// went missing simply vanished, so the reader got a chart whose nodes would not
// open, with no banner and nothing in the console. page.schema.json refuses the
// same page at the export instead, naming the key.
const PAGE_KEYS = [
  ["steps[].key", "steps.key"],
  ["steps[].after", "steps.after"],
  ["steps[].tiers", "steps.tiers"],
];

// The same keys' TYPES, which is the other half of what the filter tested:
// `Array.isArray(s.after)` and `s.after.every(a => typeof a === "string")` are
// claims about kind, not about presence, and `requires` cannot see them.
//
// after's minItems IS LOAD-BEARING AND NOT TIDINESS. A root step says so by
// carrying "" IN its list; read as an empty one it becomes its own parent, and
// the chart opens into itself without end -- measured on a config whose steps
// carried no keys, where Patrol opened into Patrol.
const PAGE_TYPES = [
  ["steps", "steps", { type: "array" }],
  ["steps[].key", "steps.key", { type: "string" }],
  ["steps[].after", "steps.after", { type: "array", minItems: 1, of: "string" }],
  ["steps[].tiers", "steps.tiers", { type: "array", minItems: 1, of: "integer" }],
  ["render_tiers", "render_tiers", { type: "array", minItems: 1, of: "integer" }],
];

export function checks() {
  const out = [];

  for (const [name, file, rows] of [
    ["column", "column.schema.json", COLUMN_KEYS],
    ["rung answer", "rungs.schema.json", RUNGS_KEYS],
    ["locator", "locator.schema.json", LOCATOR_KEYS],
  ]) {
    const root = schema(file);
    const loose = [];
    for (const [label, path] of rows) {
      if (!requires(root, path)) loose.push(`${label} (${path})`);
    }
    out.push({
      name: `every key app.js stopped refusing of a ${name} is required by its schema`,
      ok: loose.length === 0,
      detail: loose.length
        ? `${loose.length} key(s) the client no longer checks and ${file} does not ` +
          `require either: ${loose.join(", ")} — nothing would refuse a document missing them`
        : `${rows.length} key(s) moved from a hand-written refusal in site/app.js to ` +
          `${file}, which encodeColumn/encodeRungs holds the emitted bytes to`,
    });
  }

  {
    const root = schema("page.schema.json");
    const loose = PAGE_KEYS.filter(([, path]) => !requires(root, path))
      .map(([label, path]) => `${label} (${path})`);
    out.push({
      name: "every key STEPS stopped filtering is required by the page's schema",
      ok: loose.length === 0,
      detail: loose.length
        ? `${loose.length} key(s) the client no longer drops a step for, and ` +
          `page.schema.json does not require either: ${loose.join(", ")} — nothing would ` +
          `refuse a page whose step is missing them`
        : `${PAGE_KEYS.length} key(s) moved from a silent drop in site/app.js to ` +
          `page.schema.json, which encodeConfig holds the emitted bytes to`,
    });

    const wrong = [];
    for (const [label, path, want] of PAGE_TYPES) {
      const node = at(root, path);
      if (node.type !== want.type) {
        wrong.push(`${label} is ${JSON.stringify(node.type)}, want ${want.type}`);
      }
      if (want.minItems && node.minItems !== want.minItems) {
        wrong.push(`${label} allows ${node.minItems || 0} item(s), want at least ${want.minItems}`);
      }
      if (want.of && (!node.items || node.items.type !== want.of)) {
        wrong.push(`${label} holds ${JSON.stringify(node.items && node.items.type)}, want ${want.of}`);
      }
    }
    out.push({
      name: "every kind STEPS and RENDER_TIERS stopped testing is stated by the page's schema",
      ok: wrong.length === 0,
      detail: wrong.length
        ? `${wrong.length} claim(s) the client no longer makes and page.schema.json does ` +
          `not make either: ${wrong.join("; ")}`
        : `${PAGE_TYPES.length} kind claim(s) held against the bytes encodeConfig writes, ` +
          `including after's minItems -- a root step says so by carrying "" IN its list, ` +
          `and an empty one would make it its own parent`,
    });

    // fisc-mglf: THE THREE PARTIES TO A DrillStep FIELD, held to each other.
    // Adding one to export.DrillStep and to a literal in data.go used to be
    // invisible: page.schema.json did not exist, parseStepShapes matched only
    // the fields someone had hit before, and every drill.mjs check went on
    // measuring a step shape missing the new declaration, green. Now the
    // struct, the schema and the parse must name the same set, and the schema
    // is closed so the export refuses a field it has not been told about.
    const shipped = goFields("internal/export/export.go", "DrillStep");
    const stated = Object.keys(at(root, "steps").items.properties);
    const parsed = shipped.filter((f) => KNOWN_STEP_FIELDS.includes(f.field)).map((f) => f.json);
    const missFromSchema = shipped.map((f) => f.json).filter((k) => !stated.includes(k)).sort();
    const missFromStruct = stated.filter((k) => !shipped.some((f) => f.json === k)).sort();
    const missFromParse = shipped.map((f) => f.json).filter((k) => !parsed.includes(k)).sort();
    const unknown = KNOWN_STEP_FIELDS.filter((f) => !shipped.some((s) => s.field === f)).sort();
    out.push({
      name: "a DrillStep field is named by the struct, the page's schema and harness.mjs's parse alike",
      ok: !missFromSchema.length && !missFromStruct.length && !missFromParse.length && !unknown.length,
      detail: (missFromSchema.length || missFromStruct.length || missFromParse.length || unknown.length)
        ? `shipped and unstated [${missFromSchema}]; stated and unshipped [${missFromStruct}]; ` +
          `shipped and unparsed [${missFromParse}]; parsed and unshipped [${unknown}]`
        : `${shipped.length} field(s), agreed three ways: ${shipped.map((f) => f.json).join(", ")}`,
    });
  }

  // THE BOUNDARY ITSELF. A per-key shape refusal in app.js is a second
  // implementation of the schema, and the list got to eight arms one
  // reasonable-looking addition at a time. This is what stops the eleventh.
  //
  // THE TWO GATES IT PERMITS ARE NOT SHAPE CHECKS. isDocument answers "the
  // server sent an error page with a 200", which is about the transport and no
  // schema can reach it; the generated_by comparison answers "your browser
  // kept a copy from before the last deploy", which is about which FILE
  // arrived rather than what is in it -- each file is valid on its own.
  {
    const app = loadApp();
    // A refusal is fail() reached from a test on a key of a fetched document.
    // Matched on the test rather than on fail(), because fail() is also how the
    // page reports a drill that could not be answered and a year that was
    // packaged with none.
    //
    // CONFIG AND THE STEP BINDING ARE IN THE SET, and they were not: the scan
    // read doc, sched and col, so the filter STEPS carried over CONFIG.steps
    // was invisible to the arm whose whole job is to stop this shape coming
    // back. `s` is the step in that filter's own callback.
    //
    // A POSITIVE Array.isArray COUNTS TOO, which the `!?` is for. The filter
    // was not written as a refusal -- it SELECTED the steps that matched and
    // dropped the rest -- so a scan for `!Array.isArray` would have missed the
    // very code this arm was extended to catch.
    const shape = /(?:!?Array\.isArray\(\s*(?:doc|sched|col|CONFIG|s)\b|\btypeof\s+(?:doc|sched|col|CONFIG|s)\.[\w.]+\s*!==)/g;
    const found = [...app.source.matchAll(shape)].map((m) => m[0]);
    out.push({
      name: "app.js refuses no key of a document Go's schema already vets",
      ok: found.length === 0,
      detail: found.length
        ? `${found.length} per-key refusal(s) are back in site/app.js: ${found.join("; ")} — ` +
          `state the key in schema/ instead, where it holds the emitted bytes`
        : "no per-key shape refusal remains; the two gates left are isDocument " +
          "(a 200 carrying an error page) and generated_by (a cached copy), " +
          "neither of which is a claim about shape",
    });
  }

  return out;
}
