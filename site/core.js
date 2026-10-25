// @ts-check
/**
 * fisc — the client's form-agnostic core: schedule assembly, the hierarchy,
 * the fold, the cap, mark ids, provenance and wording, each a sum or lookup
 * over figures Go cited.
 *
 * Stateless by rule: no module-level `let`, no global read at import. A
 * module app.js imports is shared by every test in the process, so state
 * here would leak between them. The one store, `schedules`, memoises a pure
 * function of its key. Types are app.js's typedefs.
 */

import { pageOf } from "./format.js";

/** @returns {FiscConfig} */
export function config() {
  return /** @type {any} */ (globalThis).FISC_CONFIG;
}

/**
 * The page config version this client draws; internal/export pins it to
 * export.PageConfigVersion. Another version would draw wrong, so it is refused.
 */
export const SCHEMA_VERSION = 2;

/** Pinned by internal/export to project.SchemaVersion. */
export const PROJECTION_VERSION = 1;

/**
 * `{name:one|many}` picks singular or plural; an unfilled placeholder stays.
 * @param {string} key
 * @param {Record<string, string | number>} [vars]
 * @returns {string}
 */
export function say(key, vars) {
  const template = config().wording[key];
  return template.replace(/\{(\w+)(?::([^|}]*)\|([^}]*))?\}/g, (whole, name, one, many) => {
    if (!vars || !(name in vars)) return whole;
    const v = vars[name];
    return one === undefined ? String(v) : v + " " + (v === 1 ? one : many);
  });
}

/**
 * @param {string} kind
 * @returns {string}
 */
export function kindLabel(kind) {
  const label = config().kind_labels[kind];
  if (!label) throw new Error("cannot name link kind " + kind + ": the page publishes no label for it");
  return label;
}

/**
 * Whether `id` is printed in its parent's words, so needs a memberNoun.
 * @param {Map<string, {label: string, parent?: string}>} byID the unfolded nodes
 * @param {string} id
 * @returns {boolean}
 */
export function labelledAsParent(byID, id) {
  const own = byID.get(id);
  const parent = own && own.parent ? byID.get(own.parent) : undefined;
  return Boolean(own && parent && parent.label === own.label);
}

/**
 * A split member's noun from member_nouns, `{fund}` filled; "" for no
 * member. Refused, not guessed, for a key with no noun or a `{fund}` on a
 * node with no fund number.
 * @param {FiscProjection} doc the expansion schedule
 * @param {string} id
 * @returns {string}
 */
export function memberNoun(doc, id) {
  const n = doc.nodes.find((x) => x.id === id);
  if (!n || !n.parent) return "";
  const split = (doc.splits || []).find((/** @type {any} */ sp) =>
    sp.node === n.parent && (n.alternative ? sp.alternative && sp.key === n.alternative : !sp.alternative));
  if (!split) return "";
  const noun = (config().member_nouns || {})[split.key];
  if (!noun) throw new Error("cannot qualify " + id + ": the page publishes no noun for a member of split " + split.key);
  return noun.replace(/\{fund\}/g, () => {
    if (!n.fund) throw new Error("cannot qualify " + id + ": its noun names a fund and the column gives it no fund number");
    return String(n.fund);
  });
}

export const money = new Intl.NumberFormat("en-US", {
  style: "currency",
  currency: "USD",
  maximumFractionDigits: 0,
});

export const moneyCompact = new Intl.NumberFormat("en-US", {
  style: "currency",
  currency: "USD",
  notation: "compact",
  maximumFractionDigits: 1,
});

/** @param {number} cents */
export function fmt(cents) {
  return money.format(cents / 100);
}

/** @param {number} cents */
export function fmtShort(cents) {
  return moneyCompact.format(cents / 100);
}

/**
 * U+2212, not parentheses: screen readers do not announce parentheses.
 * @param {number} cents
 * @returns {string}
 */
export function fmtSigned(cents) {
  return (cents < 0 ? "\u2212" : "") + fmt(Math.abs(cents));
}

/** @param {number} cents */
export function fmtShortSigned(cents) {
  return (cents < 0 ? "\u2212" : "") + fmtShort(Math.abs(cents));
}

/**
 * @param {string[]} parts
 * @returns {string}
 */
export function joinAnd(parts) {
  if (parts.length < 3) return parts.join(" and ");
  return parts.slice(0, -1).join(", ") + " and " + parts[parts.length - 1];
}

/**
 * @param {string} id
 * @returns {HTMLElement}
 */
export function el(id) {
  const found = document.getElementById(id);
  if (!found) throw new Error("missing element #" + id);
  return found;
}

/**
 * For conditional elements: el() throws, so `if (!el(id))` is no guard.
 * @param {string} id
 * @returns {HTMLElement | null}
 */
export function maybeEl(id) {
  return document.getElementById(id);
}

/**
 * @param {string} name CSS custom property, with leading dashes
 * @returns {string}
 */
export function cssVar(name) {
  return getComputedStyle(document.documentElement).getPropertyValue(name).trim();
}

/**
 * @param {string} tag
 * @param {string} [className]
 * @param {string} [text]
 * @returns {HTMLElement}
 */
export function h(tag, className, text) {
  const node = document.createElement(tag);
  if (className) node.className = className;
  // Labels are data: inserted as text, never markup.
  if (text !== undefined) node.textContent = text;
  return node;
}

/**
 * @param {string} label
 * @param {string} href
 * @returns {HTMLAnchorElement}
 */
export function link(label, href) {
  const a = document.createElement("a");
  a.textContent = label;
  a.href = href;
  a.rel = "noopener";
  return a;
}

/* ---- State ---- */

/**
 * PDF page, page text and fact shard links the export built (pageOf refuses
 * an unbuilt page); an empty link is not rendered.
 * @param {FiscSource[]} sources
 * @returns {{label:string, href:string}[]}
 */
export function citations(sources) {
  /** @type {{label:string, href:string}[]} */
  const out = [];
  for (const source of sources) {
    for (const page of source.pages) {
      const links = pageOf(config().docs, source.doc_id, page);
      if (links.pdf) out.push({ label: "PDF p" + page, href: links.pdf });
      if (links.text) out.push({ label: "extracted p" + page, href: links.text });
      if (links.records) out.push({ label: "records p" + page, href: links.records });
    }
  }
  return out;
}

/**
 * Beginning and ending balances, then Go's derived change. A blank balance
 * is absent, not zero, and yields nothing.
 * @param {FiscNode} node
 * @returns {{words: string, fact_ids: string[], locators: FiscSource[], derived: boolean, rationale: string}[]}
 */
export function balancesOf(node) {
  const b = node.balances;
  if (!b) return [];
  return ["beginning", "ending", "change"].filter((which) => b[which]).map((which) => ({
    words: say("balance_" + which, { figure: fmtSigned(b[which].value_cents) }),
    fact_ids: b[which].fact_ids || [b[which].fact_id],
    locators: b[which].locators,
    derived: Boolean(b[which].derived),
    rationale: b[which].rationale || "",
  }));
}

/** The direction drawn is not printed; the ribbon is never reversed, so this says it. */
export const PARTITION_NOTE = "a cross-tab: one printed table read along a second axis, " +
  "not money moving in the direction drawn";

/**
 * @param {FiscNode | LaidNode} node
 * @returns {boolean}
 */
export function isFundGroup(node) {
  // The role, not an id prefix: column.schema.json holds the role to an enum.
  return node.role === "fund_group";
}

/**
 * Walks node.parent through `index`; "" for none (drawn --muted).
 * @param {Map<string, FiscNode>} index
 * @param {FiscNode | LaidNode} node
 * @returns {string}
 */
export function fundGroupOf(index, node) {
  // From the index, not the laid copy: the fold blanks a parent whose
  // ancestor was filtered away.
  let at = index.get(node.id) || node;
  // Bounded against a parent cycle in a malformed document.
  for (let hops = 0; hops < 8; hops++) {
    if (isFundGroup(at)) return at.id;
    if (!at.parent) return "";
    const up = index.get(at.parent);
    if (!up) return "";
    at = up;
  }
  return "";
}

/**
 * One object per column schedule, frozen so a mutating caller throws.
 * @type {WeakMap<object, Map<string, FiscProjection | null>>}
 */
const schedules = new WeakMap();

/**
 * @template T
 * @param {T} v
 * @returns {T}
 */
function deepFreeze(v) {
  if (v && typeof v === "object" && !Object.isFrozen(v)) {
    Object.freeze(v);
    for (const k of Object.keys(v)) deepFreeze(/** @type {any} */ (v)[k]);
  }
  return v;
}

/**
 * @param {any} column
 * @param {string} key
 * @returns {FiscProjection | null}
 */
function assembleSchedule(column, key) {
  const sched = column && column.schedules ? column.schedules[key] : null;
  if (!sched) return null;
  const table = Array.isArray(column.nodes) ? column.nodes : [];

  // A malformed schedule throws below, before showYear writes a word.

  const nodes = sched.nodes.map((n) => {
    const base = table[n.node] || {};
    const node = {
      id: base.id, label: base.label, tier: base.tier,
      role: base.role || "", derived: Boolean(base.derived),
      constraint_tier: base.constraint_tier || "",
      rationale: base.rationale || "", source_note: base.source_note || "",
      // What a schedule states for itself: its parent and a fund's balances.
      parent: n.parent || "",
    };
    // Not defaulted: absent balance is blank, not zero; absent alternative
    // is the primary split.
    if (n.alternative) Object.assign(node, { alternative: n.alternative });
    // Not defaulted: a node naming no fund has no number.
    if (base.fund) Object.assign(node, { fund: base.fund });
    return n.balances ? Object.assign(node, { balances: n.balances }) : node;
  });
  const links = sched.links.map((l) => {
    const from = table[l.from] || {};
    const to = table[l.to] || {};
    const link = {
      source: from.id, target: to.id,
      value_cents: l.value_cents, kind: l.kind,
      transfer_id: l.transfer_id || "",
      // Not defaulted: a default would invent provenance.
      fact_ids: l.fact_ids, locators: l.locators,
      derived: Boolean(l.derived), partition: Boolean(l.partition),
      // Defaulted: the schema carries contra only on a link printed negative.
      contra: l.contra || "",
    };
    // Not defaulted: absent alternative is primary; absent completes is no set.
    if (l.completes) Object.assign(link, { completes: l.completes });
    // Not defaulted: absent repeats is drawn once; absent restates restates nothing.
    if (l.repeats) Object.assign(link, { repeats: l.repeats });
    if (Array.isArray(l.restates)) Object.assign(link, { restates: l.restates.slice() });
    return l.alternative ? Object.assign(link, { alternative: l.alternative }) : link;
  });
  const splits = Array.isArray(sched.splits)
    ? sched.splits.map((sp) => ({
      node: (table[sp.node] || {}).id, key: sp.key, form: sp.form, alternative: Boolean(sp.alternative), side: sp.side,
    }))
    : null;
  const col = column.column || {};
  const doc = {
    schema_version: PROJECTION_VERSION,
    projection: key,
    nodes, links,
    metadata: {
      generated_by: column.generated_by || "",
      fiscal_year: col.fiscal_year, fiscal_year_label: col.label,
      basis: col.basis,
      scopes: sched.scopes,
      currency: "USD", units: "cents",
      sources: sched.sources,
      counts: sched.counts || {},
      caveats: sched.caveats || [],
    },
  };
  // The expansion schedule's alone, and absent rather than empty elsewhere.
  return /** @type {any} */ (splits ? Object.assign(doc, { splits }) : doc);
}

/**
 * A column schedule with node indices resolved and omitted keys filled.
 * @param {any} column
 * @param {string} key the page's `primary`
 * @returns {FiscProjection | null} null when the column has no such schedule
 */
export function scheduleOf(column, key) {
  if (!column || typeof column !== "object") return assembleSchedule(column, key);
  let byKey = schedules.get(column);
  if (!byKey) {
    byKey = new Map();
    schedules.set(column, byKey);
  }
  if (!byKey.has(key)) byKey.set(key, deepFreeze(assembleSchedule(column, key)));
  return /** @type {FiscProjection | null} */ (byKey.get(key));
}

/**
 * The node and every id whose parent chain reaches it; nothing names its children.
 * @param {FiscProjection} doc
 * @param {string} id
 * @returns {Set<string>}
 */
export function withinNode(doc, id) {
  const byID = new Map(doc.nodes.map((n) => [n.id, n]));
  const inside = new Set();
  for (const n of doc.nodes) {
    let at = n;
    for (let hops = 0; at && hops < 9; hops++) {
      if (at.id === id) {
        inside.add(n.id);
        break;
      }
      at = at.parent ? byID.get(at.parent) : undefined;
    }
  }
  return inside;
}

/**
 * Every node at one of `tiers`; empty is refused by foldDocument.
 * @param {FiscProjection} doc
 * @param {Iterable<number>} tiers
 * @returns {Set<string>}
 */
export function tierFrontier(doc, tiers) {
  const drawn = new Set(tiers);
  return new Set(doc.nodes.filter((n) => drawn.has(n.tier)).map((n) => n.id));
}

/**
 * Itself or its nearest frontier ancestor, else "" (refused unless opened).
 * @param {Map<string,FiscNode>} byID
 * @param {FiscNode} n
 * @param {ReadonlySet<string>} frontier
 * @returns {string}
 */
export function foldTarget(byID, n, frontier) {
  let at = n;
  for (let hops = 0; !frontier.has(at.id); hops++) {
    const up = at.parent ? byID.get(at.parent) : undefined;
    if (!up || hops > 8) return "";
    at = up;
  }
  return at.id;
}

/**
 * In the packager's order (docs, then pages, ascending, once): any other
 * order renders one page as different citations on two charts.
 * @param {Set<string>|undefined} keys
 * @returns {FiscSource[]}
 */
export function regroupLocators(keys) {
  /** @type {Map<string, number[]>} */
  const byDoc = new Map();
  for (const k of keys || []) {
    const cut = k.indexOf("\u001f");
    const doc = k.slice(0, cut);
    const page = Number(k.slice(cut + 1));
    const pages = byDoc.get(doc);
    if (pages) pages.push(page);
    else byDoc.set(doc, [page]);
  }
  return Array.from(byDoc.keys()).sort().map((doc) => ({
    doc_id: doc,
    pages: (byDoc.get(doc) || []).sort((a, b) => a - b),
  }));
}

/**
 * @param {FiscSource[]} ss
 * @returns {string[]}
 */
export function locatorKeys(ss) {
  const out = [];
  for (const s of ss) {
    for (const p of s.pages) out.push(s.doc_id + "\u001f" + p);
  }
  return out;
}

/**
 * A union, so a leaf two ribbons draw cites its pages once.
 * @param {{locators?: FiscSource[]}[]} links
 * @returns {FiscSource[]}
 */
export function pagesCitedBy(links) {
  const keys = new Set();
  for (const l of links) for (const k of locatorKeys(l.locators || [])) keys.add(k);
  return regroupLocators(keys);
}

/**
 * The fold's refusal to merge a cross-tab slice with a non-slice. A type, so
 * a caller matches the refusal, never its words.
 */
export class FoldRefusal extends Error {}

/**
 * A cut through part of a ribbon whose restated leaves the filter names only
 * some of. Carries the ends, so a caller never reads the message.
 */
export class PartialCut extends Error {
  /**
   * @param {string} message
   * @param {string} source
   * @param {string} target
   */
  constructor(message, source, target) {
    super(message);
    this.source = source;
    this.target = target;
  }
}

/**
 * Alternatives open (a member or something inside it is in the frontier)
 * and nodes opened (something strictly inside is).
 * @param {Map<string,FiscNode>} byID
 * @param {ReadonlySet<string>} frontier
 * @returns {{alternatives: Set<string>, opened: Set<string>}} alternatives keyed node\u001fkey
 */
function openedBy(byID, frontier) {
  const alternatives = new Set();
  const opened = new Set();
  for (const id of frontier) {
    let at = byID.get(id);
    for (let hops = 0; at && hops < 9; hops++) {
      if (at.alternative) alternatives.add(at.parent + "\u001f" + at.alternative);
      if (at.parent) opened.add(at.parent);
      at = at.parent ? byID.get(at.parent) : undefined;
    }
  }
  return { alternatives, opened };
}

/**
 * Every alternative on its chain is open; under a shut one the parent's other
 * split carries the same money.
 * @param {Map<string,FiscNode>} byID
 * @param {FiscNode} n
 * @param {ReadonlySet<string>} alternatives node\u001fkey
 * @returns {boolean}
 */
function readUnder(byID, n, alternatives) {
  /** @type {FiscNode | undefined} */
  let at = n;
  for (let hops = 0; at && hops < 9; hops++) {
    if (at.alternative && !alternatives.has(at.parent + "\u001f" + at.alternative)) return false;
    at = at.parent ? byID.get(at.parent) : undefined;
  }
  return true;
}

/** The prefix of a cell standing for the leaves a total restates. */
const RESTATING = "\u001frestates\u001f";

/**
 * A ribbon's cells at printed signs, or null if no leg repeats or restates:
 * a leaf at its `repeats` key (the same key wherever drawn), a total at one
 * key per restated list (lists are ascending; expansion-splits-tie refuses
 * otherwise), any other leg at its own ends. Cells sum to value_cents.
 * @param {FiscLink} l
 * @returns {Record<string, number> | null}
 */
export function cellsOf(l) {
  if (l.cells) return l.cells;
  if (l.repeats) return { [l.repeats]: l.value_cents };
  if (l.restates) return { [RESTATING + l.restates.join("\u001e")]: l.value_cents };
  return null;
}

/**
 * Links drawing each leaf, by `repeats` key. Other keys open with the
 * separator, which no node id does.
 * @param {FiscProjection | null} doc
 * @returns {Map<string, number>}
 */
export function leafDraws(doc) {
  /** @type {Map<string, number>} */
  const out = new Map();
  for (const l of doc ? doc.links : []) {
    for (const k of Object.keys(cellsOf(l) || {})) if (!k.startsWith("\u001f")) out.set(k, (out.get(k) || 0) + 1);
  }
  return out;
}

/**
 * A leaf printed once, drawn again through a split's hub: no page prints it
 * at either pair of ends.
 * @param {FiscLink} l
 * @param {Map<string, number>} draws leafDraws of its document
 * @returns {boolean}
 */
export function drawsAgain(l, draws) {
  return Object.keys(cellsOf(l) || {}).some((k) => (draws.get(k) || 0) > 1);
}

/**
 * @param {FiscProjection | null} doc the schedule before any fold
 * @returns {Map<string, number>}
 */
export function leafCents(doc) {
  /** @type {Map<string, number>} */
  const out = new Map();
  for (const l of doc ? doc.links : []) if (l.repeats) out.set(l.repeats, l.value_cents);
  return out;
}

/** @typedef {{fact_ids: string[], pages: string[], derived: boolean}} CellSource */

/**
 * @param {FiscLink} l
 * @returns {Record<string, CellSource> | null}
 */
export function cellSourcesOf(l) {
  if (l.cell_sources) return l.cell_sources;
  const cells = cellsOf(l);
  if (!cells) return null;
  /** @type {Record<string, CellSource>} */
  const out = {};
  for (const k of Object.keys(cells)) {
    out[k] = { fact_ids: l.fact_ids.slice(), pages: locatorKeys(l.locators || []), derived: Boolean(l.derived) };
  }
  return out;
}

/**
 * A merged ribbon with its legs' cells and their cell_sources, so a node
 * filter can draw part of it at that part's own citations; no `repeats`,
 * which names one leaf.
 * @param {FiscLink} ribbon
 * @param {FiscLink[]} legs
 * @returns {FiscLink}
 */
function withCells(ribbon, legs) {
  delete ribbon.repeats;
  delete ribbon.restates;
  delete ribbon.cells;
  delete ribbon.cell_sources;
  if (!legs.some((l) => cellsOf(l))) return ribbon;
  /** @type {Record<string, number>} */
  const cells = {};
  /** @type {Record<string, {facts: Set<string>, pages: Set<string>, derived: boolean}>} */
  const from = {};
  for (const l of legs) {
    const ownKey = "\u001f" + l.source + "\u001f" + l.target + "\u001f" + l.kind + "\u001f" + (l.alternative || "");
    const own = cellsOf(l) || { [ownKey]: l.value_cents };
    const sources = cellSourcesOf(l) ||
      { [ownKey]: { fact_ids: l.fact_ids, pages: locatorKeys(l.locators || []), derived: Boolean(l.derived) } };
    for (const [k, c] of Object.entries(own)) {
      cells[k] = (cells[k] || 0) + c;
      const at = from[k] || (from[k] = { facts: new Set(), pages: new Set(), derived: false });
      const src = sources[k];
      for (const id of src.fact_ids) at.facts.add(id);
      for (const p of src.pages) at.pages.add(p);
      at.derived = at.derived || src.derived;
    }
  }
  /** @type {Record<string, CellSource>} */
  const cellSources = {};
  for (const [k, at] of Object.entries(from)) {
    cellSources[k] = { fact_ids: Array.from(at.facts).sort(), pages: Array.from(at.pages).sort(), derived: at.derived };
  }
  return Object.assign(ribbon, { cells, cell_sources: cellSources });
}

/**
 * "part" for totals restating leaves only some of which are inside: no
 * ribbon can be cut along that.
 * @param {string} key a key of cellsOf
 * @param {ReadonlySet<string>} inside
 * @returns {"all" | "none" | "part"}
 */
export function cellIn(key, inside) {
  /** @param {string} leaf a repeats key, "source -> target kind" */
  const named = (leaf) => {
    const cut = leaf.indexOf(" -> ");
    if (cut < 0) return false;
    const rest = leaf.slice(cut + 4);
    const sp = rest.lastIndexOf(" ");
    return inside.has(leaf.slice(0, cut)) || (sp > 0 && inside.has(rest.slice(0, sp)));
  };
  if (key.startsWith(RESTATING)) {
    const leaves = key.slice(RESTATING.length).split("\u001e");
    const hit = leaves.filter(named).length;
    return hit === 0 ? "none" : hit === leaves.length ? "all" : "part";
  }
  if (key.startsWith("\u001f")) {
    const [, source, target] = key.split("\u001f");
    return inside.has(source) || inside.has(target) ? "all" : "none";
  }
  return named(key) ? "all" : "none";
}

/**
 * @param {string[]} keys
 * @param {number} c
 * @param {Map<string, number>} leaves
 * @returns {boolean}
 */
function restatesWhole(keys, c, leaves) {
  return keys.every((key) => leaves.has(key)) &&
    keys.reduce((s, key) => s + /** @type {number} */ (leaves.get(key)), 0) === c;
}

/**
 * A sum over ribbons, each cell once: a leaf drawn again through a hub, and
 * a total restating all of its leaves, are one figure. Cell-less ribbons
 * read whole. Fails closed on a total restating part of a list beside any of
 * its leaves: which leaves it restates is unprinted (no one division of
 * pp.85-125 is one leaf).
 * @param {FiscLink[]} links
 * @param {(l: FiscLink) => number} cents
 * @param {Map<string, number>} [leaves] leafCents of the schedule folded
 * @returns {number}
 */
export function distinctCents(links, cents, leaves = new Map()) {
  const seen = new Set();
  /** @type {Map<string, number>} */
  const restating = new Map();
  let sum = 0;
  const once = (/** @type {string} */ k, /** @type {number} */ c) => {
    if (seen.has(k)) return;
    seen.add(k);
    sum += c;
  };
  for (const l of links) {
    const cells = cellsOf(l);
    if (!cells) {
      sum += cents(l);
      continue;
    }
    for (const [k, c] of Object.entries(cells)) {
      if (k.startsWith(RESTATING)) restating.set(k, (restating.get(k) || 0) + c);
      else once(k, c);
    }
  }
  /** @type {string[][]} */
  const partial = [];
  for (const [k, c] of restating) {
    const keys = k.slice(RESTATING.length).split("\u001e");
    if (!restatesWhole(keys, c, leaves)) {
      partial.push(keys);
      sum += c;
      continue;
    }
    for (const key of keys) once(key, /** @type {number} */ (leaves.get(key)));
  }
  for (const keys of partial) {
    const twice = keys.find((key) => seen.has(key));
    if (twice !== undefined) {
      throw new Error("cannot state these ribbons' money once: totals restating " + keys.length +
        " leaves carry part of them, beside the leaf " + twice + " they may restate");
    }
    for (const key of keys) seen.add(key);
  }
  return sum;
}

/**
 * Folds each node to its nearest frontier ancestor-or-self, merging links
 * per folded pair and kind (values summed, facts and locators unioned); a
 * link onto one node is dropped.
 *
 * Expansion schedule: an alternative's links and nodes are read only while
 * it is open; an extended node stays beside its members; a node the
 * frontier opens folds to nothing.
 *
 * A ribbon whose legs are exactly one `completes` set is that printed cell;
 * any other merge of printed and inferred legs is inferred.
 *
 * Fails closed on a read node that neither folds nor opens, and a link end
 * that does not fold: dropping loses money silently.
 * @param {FiscProjection} doc
 * @param {Iterable<string>} frontier
 * @returns {FiscProjection}
 */
export function foldDocument(doc, frontier) {
  const drawn = new Set(frontier);
  const byID = new Map(doc.nodes.map((n) => [n.id, n]));
  const { alternatives, opened } = openedBy(byID, drawn);
  /** @type {Map<string, number>} each completing set's links */
  const sets = new Map();
  for (const l of doc.links) if (l.completes) sets.set(l.completes, (sets.get(l.completes) || 0) + 1);

  /** @type {Map<string,string>} */
  const foldsTo = new Map();
  for (const n of doc.nodes) {
    if (!readUnder(byID, n, alternatives)) continue;
    const to = foldTarget(byID, n, drawn);
    if (!to) {
      if (opened.has(n.id)) continue;
      throw new Error("cannot draw " + doc.projection + ": node " + n.id +
        " is tier " + n.tier + " and neither it nor an ancestor of it is among the " +
        drawn.size + " nodes this page draws");
    }
    foldsTo.set(n.id, to);
  }

  /** @type {Map<string, FiscLink>} */
  const merged = new Map();
  /** @type {Map<string, Set<string>>} */
  const cited = new Map();
  // Without this a merged ribbon cites only its first leg's pages.
  /** @type {Map<string, Set<string>>} */
  const located = new Map();
  /** @type {Map<string, {completes: string, legs: number}>} */
  const legsOf = new Map();
  /** @type {Map<string, FiscLink[]>} */
  const ribbonLegs = new Map();
  for (const l of doc.links) {
    if (l.alternative && !alternatives.has(l.source + "\u001f" + l.alternative)) continue;
    const source = foldsTo.get(l.source);
    const target = foldsTo.get(l.target);
    if (!source || !target) {
      throw new Error("cannot draw " + doc.projection + ": the link from " + l.source + " to " + l.target +
        " (" + l.kind + ") has an end this frontier draws no node for");
    }
    if (source === target) continue;
    // One ribbon per kind per folded pair, so a ribbon's kind is true of all of it.
    const key = source + "\u001f" + target + "\u001f" + l.kind;
    const at = merged.get(key);
    const ids = cited.get(key);
    if (!at || !ids) {
      merged.set(key, Object.assign({}, l, { source: source, target: target }));
      // Iterated, not new Set(l.fact_ids): absent fact_ids must throw.
      const first = new Set();
      for (const id of l.fact_ids) first.add(id);
      cited.set(key, first);
      located.set(key, new Set(locatorKeys(l.locators)));
      legsOf.set(key, { completes: l.completes || "", legs: 1 });
      ribbonLegs.set(key, [l]);
      continue;
    }
    // A partition leg and a non-partition leg are never one ribbon.
    if (Boolean(at.partition) !== Boolean(l.partition)) {
      throw new FoldRefusal("cannot draw " + doc.projection + ": folding merges ribbons one mark cannot draw as one, " +
        "a cross-tab slice and a flow that is not one, from " + source + " to " + target + " (" + l.kind + ")");
    }
    const legs = /** @type {{completes: string, legs: number}} */ (legsOf.get(key));
    legs.legs++;
    /** @type {FiscLink[]} */ (ribbonLegs.get(key)).push(l);
    if (legs.completes !== (l.completes || "")) legs.completes = "";
    at.derived = Boolean(at.derived) || Boolean(l.derived);
    // One leg's reduction words cannot name the sum.
    if ((at.contra || "") !== (l.contra || "")) at.contra = "";
    at.value_cents += l.value_cents;
    // A transfer id names one leg of one transfer and cannot survive a merge.
    if (at.transfer_id !== l.transfer_id) at.transfer_id = "";
    for (const id of l.fact_ids) ids.add(id);
    const locs = located.get(key);
    if (locs) for (const k of locatorKeys(l.locators)) locs.add(k);
  }

  // A set is broken where any leg is not in a ribbon of that set alone; no
  // later fold mends it, so none of its ribbons keeps the name, or a lone
  // derived leg would read as the whole cell.
  /** @type {Map<string, number>} */
  const landed = new Map();
  for (const legs of legsOf.values()) {
    if (legs.completes) landed.set(legs.completes, (landed.get(legs.completes) || 0) + legs.legs);
  }
  // Printed plus inferred is printed only as a completing set.
  for (const [key, l] of merged) {
    const legs = /** @type {{completes: string, legs: number}} */ (legsOf.get(key));
    const whole = legs.completes !== "" && landed.get(legs.completes) === sets.get(legs.completes);
    if (whole && legs.legs === sets.get(legs.completes)) {
      l.derived = false;
    }
    if (whole) l.completes = legs.completes;
    else delete l.completes;
  }

  const links = Array.from(merged.entries())
    .map(([key, l]) => withCells(Object.assign(l, {
      fact_ids: Array.from(cited.get(key) || []).sort(),
      // The shape Go publishes, so citations() cannot tell a folded link.
      locators: regroupLocators(located.get(key)),
    }), ribbonLegs.get(key) || []))
    .sort((a, b) => (a.source < b.source ? -1 : a.source > b.source ? 1
      : a.target < b.target ? -1 : a.target > b.target ? 1
      : a.kind < b.kind ? -1 : a.kind > b.kind ? 1 : 0));

  // d3-sankey lands an untouched node in column one at zero height.
  const touched = new Set();
  for (const l of links) {
    touched.add(l.source);
    touched.add(l.target);
  }
  // Parents re-point at the folded ancestor ("" if folded into it). A loop,
  // since the walk passes ancestors the filter dropped.
  const nodes = doc.nodes.filter((n) => touched.has(n.id)).map((n) => {
    let up = n.parent ? foldsTo.get(n.parent) : "";
    for (let hops = 0; up && up !== n.id && !touched.has(up); hops++) {
      const above = byID.get(up);
      up = above && above.parent && hops < 8 ? foldsTo.get(above.parent) : "";
    }
    return Object.assign({}, n, { parent: up && up !== n.id ? up : "" });
  });

  return Object.assign({}, doc, { nodes: nodes, links: links });
}

/**
 * The schedule's roots: the spine's nodes.
 * @param {FiscProjection} doc the expansion schedule
 * @returns {Set<string>}
 */
export function rootFrontier(doc) {
  return new Set(doc.nodes.filter((n) => !n.parent).map((n) => n.id));
}

/**
 * A node's declared splits with members: children whose alternative is the
 * key, or absent for the primary split.
 * @param {FiscProjection} doc the expansion schedule
 * @param {string} id
 * @returns {{key: string, form: string, alternative: boolean, members: string[]}[]}
 */
export function splitsOf(doc, id) {
  return (doc.splits || []).filter((/** @type {any} */ sp) => sp.node === id).map((/** @type {any} */ sp) => ({
    key: sp.key, form: sp.form, alternative: sp.alternative,
    members: doc.nodes
      .filter((n) => n.parent === id && (sp.alternative ? n.alternative === sp.key : !n.alternative))
      .map((n) => n.id),
  }));
}

/**
 * The open split's key, or "". Fails closed on members of two splits drawn
 * at once: the same money twice.
 * @param {FiscProjection} doc the expansion schedule
 * @param {Iterable<string>} frontier
 * @param {string} id
 * @returns {string}
 */
export function openSplitOf(doc, frontier, id) {
  const byID = new Map(doc.nodes.map((n) => [n.id, n]));
  const splits = splitsOf(doc, id);
  const primary = splits.find((sp) => !sp.alternative);
  const keys = new Set();
  for (const f of frontier) {
    let at = byID.get(f);
    for (let hops = 0; at && at.parent && hops < 9; hops++) {
      if (at.parent === id) {
        keys.add(at.alternative || (primary ? primary.key : ""));
        break;
      }
      at = byID.get(at.parent);
    }
  }
  if (keys.size > 1) {
    throw new Error("cannot draw " + doc.projection + ": the frontier opens " + id + " two ways (" +
      Array.from(keys).sort().join(", ") + ")");
  }
  return keys.size ? String(Array.from(keys)[0]) : "";
}

/**
 * Everything strictly inside `id` out, `id` back in.
 * @param {FiscProjection} doc the expansion schedule
 * @param {Iterable<string>} frontier
 * @param {string} id
 * @returns {Set<string>} a new set
 */
export function collapse(doc, frontier, id) {
  const inside = withinNode(doc, id);
  const out = new Set();
  let shut = false;
  for (const f of frontier) {
    if (f !== id && inside.has(f)) shut = true;
    else out.add(f);
  }
  if (shut) out.add(id);
  return out;
}

/**
 * Opens one split: members beside the node, kept as hub; another open split
 * is shut first. Fails closed on an undeclared split, a non-extend form, or
 * an undrawn node.
 * @param {FiscProjection} doc the expansion schedule
 * @param {Iterable<string>} frontier
 * @param {string} id
 * @param {string} key one of splitsOf(doc, id)
 * @returns {Set<string>} a new set
 */
export function expand(doc, frontier, id, key) {
  const split = splitsOf(doc, id).find((sp) => sp.key === key);
  if (!split) throw new Error("cannot open " + id + ": it has no split " + key);
  if (split.form !== "extend") {
    throw new Error("cannot open " + id + ": its split " + key + " is declared " + JSON.stringify(split.form) +
      ", and this page draws a split only as extend");
  }
  const open = openSplitOf(doc, frontier, id);
  if (open === key) return new Set(frontier);
  const out = open ? collapse(doc, frontier, id) : new Set(frontier);
  if (!out.has(id)) throw new Error("cannot open " + id + ": the frontier does not draw it");
  for (const m of split.members) out.add(m);
  // A one-member split opens straight through: alone it draws nothing new.
  if (split.members.length !== 1) return out;
  const only = splitsOf(doc, split.members[0]).find((sp) => !sp.alternative);
  return only ? expand(doc, out, split.members[0], only.key) : out;
}

/**
 * The ancestor whose opening opened `id` straight through (one-member
 * primary splits), else `id`: what folding `id` folds.
 * @param {FiscProjection} doc the expansion schedule
 * @param {string} id
 * @returns {string}
 */
export function straightFrom(doc, id) {
  const byID = new Map(doc.nodes.map((n) => [n.id, n]));
  let at = id;
  // Nine hops bounds a parent cycle; no schedule nests near so deep.
  for (let hops = 0; hops < 9; hops++) {
    const n = byID.get(at);
    if (!n || !n.parent) break;
    const split = splitsOf(doc, n.parent).find((sp) => sp.members.includes(at));
    if (!split || split.members.length !== 1) break;
    at = n.parent;
    // An alternative is the reader's choice, not expand's: the parent folds alone.
    if (n.alternative) break;
  }
  return at;
}

/**
 * The splits that draw a node, root first: each ancestor and the split the
 * next node down belongs to. A parent with no such split is refused:
 * nothing could draw the child.
 * @param {FiscProjection} doc the expansion schedule
 * @param {string} id
 * @returns {{node: string, key: string}[]}
 */
export function revealChain(doc, id) {
  const byID = new Map(doc.nodes.map((n) => [n.id, n]));
  const chain = [];
  for (let at = byID.get(id), hops = 0; at && at.parent && hops < 9; at = byID.get(at.parent), hops++) {
    const splits = splitsOf(doc, at.parent);
    const primary = splits.find((sp) => !sp.alternative);
    const key = at.alternative || (primary ? primary.key : "");
    if (!key || !splits.some((sp) => sp.key === key)) {
      throw new Error(`cannot reveal ${id}: ${at.parent} declares no split ${at.id} belongs to`);
    }
    chain.unshift({ node: at.parent, key });
  }
  return chain;
}

/**
 * @param {FiscProjection} doc the expansion schedule
 * @param {Iterable<string>} frontier
 * @param {string} id
 * @returns {Set<string>} a new set
 */
export function reveal(doc, frontier, id) {
  let out = new Set(frontier);
  for (const { node, key } of revealChain(doc, id)) {
    if (openSplitOf(doc, out, node) !== key) out = expand(doc, out, node, key);
  }
  return out;
}

/**
 * Splits every unsplit node by its first split, repeatedly, up to 16 passes.
 * @param {FiscProjection} doc the expansion schedule
 * @param {Set<string>} frontier
 * @returns {{splits: {node: string, key: string}[], frontier: Set<string>}}
 */
export function expandAllFrom(doc, frontier) {
  let open = frontier;
  const splits = [];
  for (let pass = 0; pass < 16; pass++) {
    const shut = doc.nodes.filter((n) => open.has(n.id) && splitsOf(doc, n.id).length &&
      openSplitOf(doc, open, n.id) === "");
    if (!shut.length) break;
    for (const n of shut) {
      const key = splitsOf(doc, n.id)[0].key;
      splits.push({ node: n.id, key });
      open = expand(doc, open, n.id, key);
    }
  }
  return { splits, frontier: open };
}

/**
 * Folds all but `cap` nodes of a tier into one tail: remainders first, then
 * the largest printed rows. A cap, not a rescale; values sum and citations
 * union, so the tail stays citable. What may fold is Go's (split_caps);
 * what is kept is the page's.
 * @param {FiscProjection} doc
 * @param {number} tier
 * @param {number} cap
 * @param {string} opened the tail's parent, "" across fund groups
 * @param {string} noun the plural noun for the tier's rows
 * @param {{members: ReadonlySet<string>, id: string}} [among] one split's
 *   members and its tail id, instead of the whole tier
 * @returns {FiscProjection} doc itself when the column fits whole
 */
export function capColumn(doc, tier, cap, opened, noun, among) {
  const inColumn = among
    ? (/** @type {FiscNode} */ n) => among.members.has(n.id)
    : (/** @type {FiscNode} */ n) => n.tier === tier;
  const tailID = among ? among.id : aggregateID(tier);
  const atTier = doc.nodes.filter(inColumn);
  // A tail of one is worse than none: cap + 1 draws whole.
  if (atTier.length <= cap + 1) return doc;

  // Ranked by max(inflow, outflow) at magnitude, d3-sankey's node value: a
  // line takes in nothing, and a contra row is as large as its figure.
  /** @type {Map<string, number>} */
  const inflow = new Map();
  /** @type {Map<string, number>} */
  const outflow = new Map();
  for (const l of doc.links) {
    inflow.set(l.target, (inflow.get(l.target) || 0) + Math.abs(l.value_cents));
    outflow.set(l.source, (outflow.get(l.source) || 0) + Math.abs(l.value_cents));
  }
  const size = (/** @type {string} */ id) => Math.max(inflow.get(id) || 0, outflow.get(id) || 0);
  // Ties broken by id, so the kept set is stable across builds.
  const ranked = atTier.slice().sort((a, b) =>
    size(b.id) - size(a.id) || (a.id < b.id ? -1 : 1));
  // A remainder is never folded: the tail's words say all inside is printed.
  const remainders = ranked.filter((n) => n.derived);
  const printed = ranked.filter((n) => !n.derived);
  const kept = new Set(remainders.concat(printed.slice(0, Math.max(0, cap - remainders.length))).map((n) => n.id));
  const folded = ranked.filter((n) => !kept.has(n.id));
  if (folded.length < 2) return doc;

  // The noun is the view's, plural: the threshold above folds two or more.
  const aggregate = aggregateMark(tier, opened, folded.map((n) => n.id), atTier.length, cap, noun, tailID);
  const tail = new Set(folded.map((n) => n.id));
  const remap = (/** @type {string} */ id) => (tail.has(id) ? tailID : id);

  // A folded node's children hang from the tail: foldDocument refuses an
  // unknown parent, and dropping them drops their money.
  const nodes = doc.nodes
    .filter((n) => !inColumn(n) || kept.has(n.id))
    .map((n) => (tail.has(n.parent) ? Object.assign({}, n, { parent: tailID }) : n))
    .concat([aggregate]);
  // And the links that named them, for the same refusal.
  const present = new Set(nodes.map((n) => n.id));
  const links = doc.links
    .map((l) => Object.assign({}, l, { source: remap(l.source), target: remap(l.target) }))
    .filter((l) => present.has(l.source) && present.has(l.target));

  // tailFigure finishes the note after the fold and any later cap.
  return Object.assign({}, doc, { nodes: nodes, links: links });
}

/**
 * max(arriving, leaving) at magnitude, as d3-sankey draws it. Read off the
 * folded document, since the fold nets, re-points and drops ribbons.
 * @param {FiscProjection} doc a folded document
 * @param {string} id the tail's id
 * @returns {number} cents
 */
export function tailFigure(doc, id) {
  let arriving = 0;
  let leaving = 0;
  for (const l of doc.links) {
    if (l.target === id) arriving += Math.abs(l.value_cents);
    if (l.source === id) leaving += Math.abs(l.value_cents);
  }
  return Math.max(arriving, leaving);
}

/** Every capped tail's id prefix. */
export const AGGREGATE_PREFIX = "aggregate/tail/";

/**
 * Per tier: one chart can cap two columns, and everything keys by id.
 * @param {number} tier
 * @returns {string}
 */
export function aggregateID(tier) {
  return AGGREGATE_PREFIX + tier;
}

/**
 * Per split, not tier: one slot holds many splits' members, and each tail
 * stays inside its node.
 * @param {string} node
 * @param {string} key the split's key
 * @returns {string}
 */
export function splitTailID(node, key) {
  return AGGREGATE_PREFIX + key + "/" + node;
}

/**
 * Null for a tier's tail or any other id.
 * @param {string} id
 * @returns {{node: string, key: string} | null}
 */
export function splitTailOf(id) {
  if (!id.startsWith(AGGREGATE_PREFIX)) return null;
  const rest = id.slice(AGGREGATE_PREFIX.length);
  const cut = rest.indexOf("/");
  if (cut <= 0 || cut === rest.length - 1) return null;
  return { node: rest.slice(cut + 1), key: rest.slice(0, cut) };
}

/**
 * Declares splits; scheduleOf carries them on that schedule alone.
 * @param {FiscProjection | null | undefined} doc
 * @returns {boolean}
 */
export function isExpansionSchedule(doc) {
  return Boolean(doc) && Array.isArray(/** @type {any} */ (doc).splits);
}

/**
 * @param {string} id
 * @returns {boolean}
 */
export function isAggregate(id) {
  return id.startsWith(AGGREGATE_PREFIX);
}

/** Followed by side and tier. */
export const SMALLER_PREFIX = "smaller/";

/** smallerMark's side for the mark the folded ribbons run TO. */
export const SMALLER_TO = "to";

/** smallerMark's side for the mark the folded ribbons run FROM. */
export const SMALLER_FROM = "from";

/**
 * Per side, so a mark only receives or sends and never reads as a pass-through;
 * per tier and slot, since one tier can stand in several columns.
 * @param {string} side SMALLER_TO or SMALLER_FROM
 * @param {number} tier
 * @param {number} [slot]
 * @returns {string}
 */
export function smallerID(side, tier, slot) {
  return SMALLER_PREFIX + side + "/" + tier + (slot === undefined ? "" : "/" + slot);
}

/**
 * @param {string} id
 * @returns {boolean}
 */
export function isSmaller(id) {
  return id.startsWith(SMALLER_PREFIX);
}

/**
 * The declared set is export.MarkPrefixes.
 * @param {string} id
 * @returns {boolean}
 */
export function isMark(id) {
  return isAggregate(id) || isSmaller(id);
}

/**
 * A ribbon cites facts and a node does not: the schema's shape, no form's.
 * @param {LaidLink | LaidNode | FiscLink | FiscNode} d
 * @returns {boolean}
 */
export function isLink(d) {
  return Object.prototype.hasOwnProperty.call(d, "fact_ids");
}

/**
 * A cap's tail mark. derived: true is the invariant: no city line is called
 * "N smaller funds". The folded ids are kept so caveatsFor still reaches
 * them, since no parent chain records a fold by value.
 * @param {number} tier
 * @param {string} opened the parent, "" across groups
 * @param {string[]} folded the ids folded, two or more
 * @param {number} total nodes before the cap
 * @param {number} cap
 * @param {string} word plural noun
 * @param {string} [id] aggregateID(tier) by default
 * @returns {FiscMark}
 */
export function aggregateMark(tier, opened, folded, total, cap, word, id) {
  return {
    id: id || aggregateID(tier),
    label: say("aggregate_label", { folded: folded.length, word: word }),
    tier: tier,
    parent: opened,
    constraint_tier: "",
    role: "aggregate",
    derived: true,
    folds: folded.slice(),
    rationale: say("aggregate_rationale", { folded: folded.length, word: word }),
    source_note: say("aggregate_note", { folded: folded.length, total: total, cap: cap }),
  };
}

/**
 * The smallest-flow filter's mark. derived: true, as no city line is called
 * "flows under $10,000"; its ribbons cite every fact and page they merge.
 * @param {string} side SMALLER_TO or SMALLER_FROM
 * @param {number} tier
 * @param {number} flows printed ribbons it stands for
 * @param {number} smallest the threshold, cents
 * @param {number} cents their sum
 * @param {FiscSource[]} locators every page they cite
 * @param {number} [slot]
 * @returns {FiscMark}
 */
export function smallerMark(side, tier, flows, smallest, cents, locators, slot) {
  const mark = {
    id: smallerID(side, tier, slot),
    label: say("smaller_label", { flows: flows, threshold: fmt(smallest) }),
    tier: tier,
    parent: "",
    constraint_tier: "",
    role: "smaller",
    derived: true,
    rationale: say("smaller_rationale", { flows: flows, threshold: fmt(smallest) }),
    source_note: say("smaller_note", { flows: flows, figure: fmt(cents) }),
    locators: locators,
  };
  return slot === undefined ? mark : Object.assign(mark, { slot });
}

/**
 * A fund group among `nodes` hides as one among `groups` does.
 * @typedef {{groups: ReadonlySet<string>, kinds: ReadonlySet<string>, nodes?: ReadonlySet<string>, smallest: number}} Removals
 */

/**
 * Schedule ids (withinNode), not the chart's: a hub draws a member's leaves
 * on ribbons with no end at the member.
 * @typedef {{out: ReadonlySet<string>, groups?: ReadonlySet<string>, only: ReadonlySet<string> | null}} LeafCut
 */

/**
 * Applies removals: ribbons in a hidden group (either end), kind or node,
 * or not kept by `keeps`, are removed; printed ribbons under `smallest` fold
 * onto a smaller-flows mark; untouched nodes drop. Each ribbon is drawn,
 * merged or hidden, exactly one, or under `leaves` cut in two along its
 * cells. A node keeps fixedValue only while all its arriving ribbons are
 * drawn.
 *
 * Under `leaves` a node's money is its leaves, so a hub loses a member's
 * money with it. Fails closed on a cell restating leaves only some of which
 * are named, except by group: a division's total spans groups and is drawn
 * whole, as no page prints one group's part.
 * @param {FiscProjection} doc a shaped chart, reductions drawn forward
 * @param {(n: FiscNode) => string} groupOf "" for none
 * @param {Removals} state
 * @param {((l: FiscLink) => boolean) | null} [keeps] what `only` draws whole
 * @param {LeafCut | null} [leaves]
 * @returns {{doc: FiscProjection, hidden: FiscLink[]}} doc itself where nothing changes
 */
export function filterDocument(doc, groupOf, state, keeps = null, leaves = null) {
  const byID = new Map(doc.nodes.map((n) => [n.id, n]));
  const nodes = state.nodes || new Set();
  const inHidden = (/** @type {string} */ id) => {
    const n = byID.get(id);
    const g = n ? groupOf(n) : "";
    return (g !== "" && (state.groups.has(g) || nodes.has(g))) || nodes.has(id);
  };
  /** @type {FiscLink[]} */
  const hidden = [];
  /** @type {FiscLink[]} */
  const kept = [];
  for (const l of doc.links) {
    const whole = !keeps || keeps(l);
    if (state.kinds.has(l.kind) || inHidden(l.source) || inHidden(l.target) || (!whole && !(leaves && leaves.only))) {
      hidden.push(l);
      continue;
    }
    const cells = leaves ? cellsOf(l) : null;
    if (!leaves || !cells) {
      if (whole && !(leaves && (leaves.out.has(l.source) || leaves.out.has(l.target)))) kept.push(l);
      else hidden.push(l);
      continue;
    }
    /** @type {string[]} */
    const drawn = [];
    /** @type {string[]} */
    const gone = [];
    for (const k of Object.keys(cells)) {
      const out = leaves.out.size ? cellIn(k, leaves.out) : "none";
      const grouped = leaves.groups && leaves.groups.size ? cellIn(k, leaves.groups) : "none";
      const alone = whole || !leaves.only ? "all" : cellIn(k, leaves.only);
      if (out === "part" || alone === "part") {
        throw new PartialCut("cannot draw part of the ribbon from " + l.source + " to " + l.target +
          ": it carries totals the city printed for several leaves together, and the node filtered names only some of them",
        l.source, l.target);
      }
      if (out === "none" && grouped !== "all" && alone === "all") drawn.push(k);
      else gone.push(k);
    }
    if (!gone.length) kept.push(l);
    else if (!drawn.length) hidden.push(l);
    else {
      kept.push(partOf(l, drawn));
      hidden.push(partOf(l, gone, false));
    }
  }
  if (!hidden.length && !(state.smallest > 0)) return { doc: doc, hidden: hidden };
  const folded = foldSmaller(kept, byID, state.smallest, keeps);
  /** @type {Set<string>} the nodes a ribbon that arrived at them no longer reaches as itself */
  const arrivingChanged = new Set(hidden.map((l) => l.target).concat(folded.cut));
  const touched = new Set();
  for (const l of folded.links) {
    touched.add(l.source);
    touched.add(l.target);
  }
  const drawnNodes = doc.nodes.filter((n) => touched.has(n.id)).map((n) => {
    if (n.fixedValue === undefined || !arrivingChanged.has(n.id)) return n;
    const out = Object.assign({}, n);
    delete out.fixedValue;
    return out;
  });
  return {
    // filtered: a column the shaped chart filled may be empty now.
    doc: Object.assign({}, doc, { nodes: drawnNodes.concat(folded.marks), links: folded.links, filtered: true }),
    hidden: hidden,
  };
}

/**
 * The cells `keys` names, drawn as the ribbon is; claims no completing set.
 * Fails closed on a drawn part netting against its ribbon's direction; a
 * hidden part is only tallied (distinctCents).
 * @param {FiscLink} l
 * @param {string[]} keys
 * @param {boolean} [drawn] false for a hidden part
 * @returns {FiscLink}
 */
export function partOf(l, keys, drawn = true) {
  const cells = /** @type {Record<string, number>} */ (cellsOf(l));
  const sources = /** @type {Record<string, CellSource>} */ (cellSourcesOf(l));
  const printed = Object.values(cells).reduce((s, c) => s + c, 0);
  // markContra drew a ribbon printed negative forward, at its magnitude.
  const sign = l.value_cents === printed ? 1 : l.value_cents === -printed ? -1 : 0;
  if (!sign) {
    throw new Error("cannot draw part of the ribbon from " + l.source + " to " + l.target +
      ": it is drawn at neither the sum of its cells at their printed signs nor that sum reversed");
  }
  /** @type {Record<string, number>} */
  const sub = {};
  /** @type {Record<string, CellSource>} */
  const from = {};
  const facts = new Set();
  const pages = new Set();
  let cents = 0;
  let derived = false;
  for (const k of keys) {
    sub[k] = cells[k];
    from[k] = sources[k];
    cents += cells[k];
    for (const id of sources[k].fact_ids) facts.add(id);
    for (const p of sources[k].pages) pages.add(p);
    derived = derived || sources[k].derived;
  }
  if (drawn && sign * cents < 0) {
    throw new Error("cannot draw part of the ribbon from " + l.source + " to " + l.target + ": the part nets the other way");
  }
  const part = Object.assign({}, l, {
    value_cents: sign * cents, fact_ids: Array.from(facts).sort(), locators: regroupLocators(pages),
    derived: derived, cells: sub, cell_sources: from,
  });
  delete part.completes;
  return part;
}

/**
 * Folds printed ribbons under `smallest` onto one mark per column and side,
 * keeping each at its larger end; a merge of one is drawn as itself. Only
 * printed ribbons between printed nodes fold, and none `keep` keeps: one
 * merged ribbon cannot be both printed and inferred.
 * @param {FiscLink[]} links
 * @param {Map<string, FiscNode>} byID
 * @param {number} smallest cents; 0 folds nothing
 * @param {((l: FiscLink) => boolean) | null} [keep]
 * @returns {{links: FiscLink[], marks: FiscMark[], cut: string[]}} cut: ids no longer arrived at
 */
export function foldSmaller(links, byID, smallest, keep = null) {
  if (!(smallest > 0)) return { links: links, marks: [], cut: [] };
  /** @type {Map<string, number>} */
  const inflow = new Map();
  /** @type {Map<string, number>} */
  const outflow = new Map();
  for (const l of links) {
    inflow.set(l.target, (inflow.get(l.target) || 0) + Math.abs(l.value_cents));
    outflow.set(l.source, (outflow.get(l.source) || 0) + Math.abs(l.value_cents));
  }
  const size = (/** @type {string} */ id) => Math.max(inflow.get(id) || 0, outflow.get(id) || 0);
  const folds = (/** @type {FiscLink} */ l) => Math.abs(l.value_cents) < smallest && !l.derived && !l.contra &&
    !isMark(l.source) && !isMark(l.target) && byID.has(l.source) && byID.has(l.target) && !(keep && keep(l));
  /** @type {Map<string, {source: string, target: string, far: string, legs: FiscLink[]}>} */
  const merges = new Map();
  /** @type {(FiscLink | string)[]} a link drawn as itself, or the key of a merge, in first-seen order */
  const order = [];
  for (const l of links) {
    if (!folds(l)) {
      order.push(l);
      continue;
    }
    // THE LARGER END KEEPS THE RIBBON; ties by id, so the fold is stable.
    const keepSource = size(l.source) > size(l.target) ||
      (size(l.source) === size(l.target) && l.source < l.target);
    const far = keepSource ? l.target : l.source;
    const farNode = /** @type {FiscNode} */ (byID.get(far));
    const tier = farNode.tier;
    const slot = typeof farNode.slot === "number" ? farNode.slot : undefined;
    const source = keepSource ? l.source : smallerID(SMALLER_FROM, tier, slot);
    const target = keepSource ? smallerID(SMALLER_TO, tier, slot) : l.target;
    const key = source + "\u001f" + target + "\u001f" + l.kind + "\u001f" + Boolean(l.partition);
    const at = merges.get(key);
    if (at) {
      at.legs.push(l);
      continue;
    }
    merges.set(key, { source, target, far, legs: [l] });
    order.push(key);
  }
  /** @type {Map<string, {flows: number, cents: number, pages: Set<string>, tier: number, slot: number | undefined, side: string}>} */
  const marks = new Map();
  /** @type {string[]} */
  const cut = [];
  const out = order.map((entry) => {
    if (typeof entry !== "string") return entry;
    const m = /** @type {{source: string, target: string, far: string, legs: FiscLink[]}} */ (merges.get(entry));
    if (m.legs.length < 2) return m.legs[0];
    const ids = new Set();
    const locators = pagesCitedBy(m.legs);
    let cents = 0;
    let transfer = m.legs[0].transfer_id;
    for (const l of m.legs) {
      cents += l.value_cents;
      for (const id of l.fact_ids) ids.add(id);
      if (l.transfer_id !== transfer) transfer = "";
      // Kept at its source, a leg no longer arrives at its own target.
      if (!isSmaller(m.target)) continue;
      cut.push(l.target);
    }
    const markID = isSmaller(m.source) ? m.source : m.target;
    const side = isSmaller(m.source) ? SMALLER_FROM : SMALLER_TO;
    const farNode = /** @type {FiscNode} */ (byID.get(m.far));
    const mark = marks.get(markID) || { flows: 0, cents: 0, pages: new Set(), tier: farNode.tier,
      slot: typeof farNode.slot === "number" ? farNode.slot : undefined, side };
    mark.flows += m.legs.length;
    mark.cents += cents;
    for (const k of locatorKeys(locators)) mark.pages.add(k);
    marks.set(markID, mark);
    return withCells(Object.assign({}, m.legs[0], {
      source: m.source, target: m.target, value_cents: cents, transfer_id: transfer,
      fact_ids: Array.from(ids).sort(), locators,
      merged: say("smaller_flow", { flows: m.legs.length, threshold: fmt(smallest) }),
      legs: m.legs.slice(),
    }), m.legs);
  });
  return {
    links: out,
    marks: Array.from(marks.values()).map((m) =>
      smallerMark(m.side, m.tier, m.flows, smallest, m.cents, regroupLocators(m.pages), m.slot)),
    cut: cut,
  };
}

/* ---- A mark's figures: sums over cited ribbons ---- */

/**
 * Reductions on the larger side, at magnitude.
 * @param {{value_cents: number, contra?: string}[]} arriving
 * @param {{value_cents: number, contra?: string}[]} leaving
 * @returns {number} cents
 */
export function reducedOf(arriving, leaving) {
  const sum = (/** @type {{value_cents: number}[]} */ ls) => ls.reduce((s, l) => s + Math.abs(l.value_cents), 0);
  const side = sum(arriving) >= sum(leaving) ? arriving : leaving;
  return sum(side.filter((l) => l.contra));
}

/**
 * One drawn node of a side's tree: its signed figure in cents, the size it
 * is laid out at, what it cites, and the members a frontier draws under it.
 * @typedef {{
 *   id: string, label: string, tier: number, parent: string, derived: boolean, contra: string,
 *   cents: number, size: number, fact_ids: string[], locators: FiscSource[],
 *   splits: {key: string, form: string, alternative: boolean, members: string[]}[], open: string,
 *   children: SideTile[] | null,
 * }} SideTile
 */

/**
 * Inferred if the node or any link is; the shared reduction words. Fails
 * closed on mixed reduction, which would misplace the sum in a share.
 * @param {{derived?: boolean}} node
 * @param {{derived?: boolean, contra?: string}[]} links
 * @param {string} what names the sum, for the refusal
 * @returns {{derived: boolean, contra: string}}
 */
export function sumReading(node, links, what) {
  /** @type {string | null} */
  let contra = null;
  for (const l of links) {
    const c = String(l.contra || "");
    if (contra === null) contra = c;
    else if (contra !== c) {
      throw new Error(what + " with links that disagree on the reduction they are printed as (" +
        JSON.stringify(contra) + " and " + JSON.stringify(c) + "), so its sum is neither");
    }
  }
  return { derived: Boolean(node.derived) || links.some((l) => Boolean(l.derived)), contra: contra || "" };
}

/**
 * One side's tree at a frontier: roots at `tier`, opened members beneath. A
 * root's figure is its folded outside ribbons, signed; a member's is its
 * ribbons to its hub. Fails closed on a root both receiving and sending
 * outside, and a member with no ribbon to its parent.
 * @param {FiscProjection} doc the expansion schedule
 * @param {Iterable<string>} frontier hubs included
 * @param {number} tier a spine column
 * @returns {{roots: SideTile[], cents: number, size: number}}
 */
export function sideTree(doc, frontier, tier) {
  const open = new Set(frontier);
  const folded = foldDocument(doc, open);
  const byID = new Map(folded.nodes.map((n) => [n.id, n]));
  const raw = new Map(doc.nodes.map((n) => [n.id, n]));
  /** @type {Map<string, FiscLink[]>} ribbons at each drawn node */
  const at = new Map();
  for (const l of folded.links) {
    for (const end of [l.source, l.target]) {
      const list = at.get(end) || [];
      list.push(l);
      at.set(end, list);
    }
  }
  /** @param {FiscLink[]} ribbons */
  const figure = (ribbons) => {
    const ids = new Set();
    let cents = 0;
    for (const l of ribbons) {
      cents += l.value_cents;
      for (const id of l.fact_ids) ids.add(id);
    }
    return { cents, fact_ids: Array.from(ids).sort(), locators: pagesCitedBy(ribbons) };
  };
  /**
   * @param {FiscNode} n
   * @param {FiscLink[]} ribbons
   * @returns {SideTile}
   */
  const tile = (n, ribbons) => {
    const f = figure(ribbons);
    const splits = splitsOf(doc, n.id);
    const members = folded.nodes.filter((m) => m.parent === n.id && (raw.get(m.id) || { parent: "" }).parent === n.id);
    const children = members.length ? members.map((m) => {
      const joining = (at.get(m.id) || []).filter((l) => l.source === n.id || l.target === n.id);
      if (!joining.length) {
        throw new Error("cannot draw " + doc.projection + ": " + m.id + " is drawn under " + n.id +
          " and no ribbon joins the two, so it has no figure here");
      }
      return tile(m, joining);
    }) : null;
    const reading = sumReading(n, ribbons, "cannot draw " + doc.projection + ": " + n.id);
    return {
      id: n.id, label: String(n.label), tier: n.tier, parent: n.parent || "",
      derived: reading.derived, contra: reading.contra,
      cents: f.cents, size: Math.abs(f.cents), fact_ids: f.fact_ids, locators: f.locators,
      splits, open: splits.length ? openSplitOf(doc, open, n.id) : "",
      children,
    };
  };
  const roots = [];
  let cents = 0;
  let size = 0;
  for (const n of folded.nodes) {
    if (n.parent || n.tier !== tier) continue;
    const inside = withinNode(doc, n.id);
    const outside = (at.get(n.id) || []).filter((l) => !inside.has(l.source === n.id ? l.target : l.source));
    const leaving = outside.some((l) => l.source === n.id);
    const arriving = outside.some((l) => l.target === n.id);
    if (leaving && arriving) {
      throw new Error("cannot draw tier " + tier + " as a side: " + n.id + " both takes in and pays out, " +
        "so no one figure is its size");
    }
    const t = tile(n, outside);
    roots.push(t);
    cents += t.cents;
    size += t.size;
  }
  if (!roots.length) throw new Error("cannot draw tier " + tier + " as a side: " + doc.projection + " draws no root at it");
  roots.sort((a, b) => b.size - a.size || a.label.localeCompare(b.label));
  return { roots, cents, size };
}
