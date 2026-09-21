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
import { loadApp, repoRoot } from "./harness.mjs";

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
    const shape = /(?:!Array\.isArray\(\s*(?:doc|sched|col)\b|\btypeof\s+(?:doc|sched|col)\.[\w.]+\s*!==)/g;
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
