// @ts-check
/**
 * fisc — the client's form-agnostic core: what every chart form needs and no
 * form owns. Schedule assembly, the hierarchy by parent chain, the fold, the
 * cap, the mark ids, provenance, wording and formatting, each a sum or a
 * lookup over figures Go cited.
 *
 * STATELESS BY RULE: no module-level `let`, and no global read at import.
 * The tests load app.js through a cache-busting query so each gets a fresh
 * instance; a module app.js imports by relative path is instantiated once per
 * process and shared by every test, so anything held here would leak between
 * them. The page's config is read through config() at call time, and the DOM
 * through `document` inside the function that needs it.
 *
 * A type named here is one of app.js's typedefs, generated from schema/.
 */

/**
 * The page's window.FISC_CONFIG, read when asked for and never at import.
 * @returns {FiscConfig}
 */
export function config() {
  return /** @type {any} */ (globalThis).FISC_CONFIG;
}

/**
 * The projection schema this client draws. A test in internal/export pins
 * this literal to the producer's constant. A newer schema would draw a wrong
 * chart rather than fail, so the page refuses it.
 */
export const SCHEMA_VERSION = 1;

/**
 * One sentence of the page's wording, filled in. `{name:one|many}` appends the
 * singular or plural word; an unfilled placeholder is left as written.
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
 * A link kind in the page's words, as the export ships them.
 * @param {string} kind
 * @returns {string}
 */
export function kindLabel(kind) {
  const label = config().kind_labels[kind];
  if (!label) throw new Error("cannot name link kind " + kind + ": the page publishes no label for it");
  return label;
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
 * A figure with its sign. A minus rather than parentheses, because screen
 * readers do not announce parentheses; U+2212 so it reads as a sign.
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

const moneyExact = new Intl.NumberFormat("en-US", {
  style: "currency",
  currency: "USD",
  minimumFractionDigits: 2,
  maximumFractionDigits: 2,
});

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
 * The element with this id, or null. For conditional elements such as the
 * year toggle: el() throws, so `if (!el(id))` is no guard.
 * @param {string} id
 * @returns {HTMLElement | null}
 */
export function maybeEl(id) {
  return document.getElementById(id);
}

/**
 * @param {string} name CSS custom property, including the leading dashes.
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
  // Labels come out of the data file; they are inserted as text, never as
  // markup.
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

/* ------------------------------------------------------------------ *
 * State
 * ------------------------------------------------------------------ */

/**
 * Citations for a set of source documents: the city's PDF at the page, the
 * committed page text, and the fact-store shard for the page, each the link
 * the export built for that page. A link it left empty is not rendered; a
 * page it built no entry for is refused, so a cited page cannot vanish.
 * @param {FiscSource[]} sources
 * @returns {{label:string, href:string}[]}
 */
export function citations(sources) {
  /** @type {{label:string, href:string}[]} */
  const out = [];
  for (const source of sources) {
    const doc = config().docs[source.doc_id];
    for (const page of source.pages) {
      const links = doc && doc.pages && doc.pages[String(page)];
      if (!links) {
        throw new Error("cannot cite " + source.doc_id + " p" + page + ": the page publishes no links for it");
      }
      if (links.pdf) out.push({ label: "PDF p" + page, href: links.pdf });
      if (links.text) out.push({ label: "extracted p" + page, href: links.text });
      if (links.records) out.push({ label: "records p" + page, href: links.records });
    }
  }
  return out;
}

/**
 * A node's printed balances, beginning then ending, each in the page's words
 * with the fact and the pages it cites; none for a node carrying none, and
 * none for a balance the page leaves blank, which is absent and not zero.
 * @param {FiscNode} node
 * @returns {{words: string, fact_id: string, locators: FiscSource[]}[]}
 */
export function balancesOf(node) {
  const b = node.balances;
  if (!b) return [];
  return ["beginning", "ending"].filter((which) => b[which]).map((which) => ({
    words: say("balance_" + which, { figure: fmtSigned(b[which].value_cents) }),
    fact_id: b[which].fact_id,
    locators: b[which].locators,
  }));
}

/**
 * A drawn document's column, named as the page's lede names that year: the
 * published year the export shipped the name for.
 * @param {any} meta
 * @returns {string}
 */
export function ledeOf(meta) {
  const year = config().years.find((y) => y.year === meta.fiscal_year && y.basis === meta.basis);
  if (!year) {
    throw new Error("cannot name FY" + meta.fiscal_year + " " + meta.basis + ": the page publishes no such year");
  }
  return year.lede;
}

/**
 * What a partition ribbon is: the one sentence every mark showing it uses.
 * The direction drawn is not one the city printed; the ribbon is never
 * reversed, and the class and this sentence carry what it means.
 */
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
 * The fund group a node belongs to, walking node.parent through `index`, the
 * drawn document's nodes by id; "" for none, which callers draw as --muted.
 * @param {Map<string, FiscNode>} index
 * @param {FiscNode | LaidNode} node
 * @returns {string}
 */
export function fundGroupOf(index, node) {
  // Start from the index, not the laid copy: the fold blanks a node's parent
  // when its ancestor is filtered away, which would stop the walk on hop one.
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
 * Each column's schedules, assembled once, so one column's schedule is one
 * object however often a gesture reads it. Frozen, so a caller that would
 * change it throws rather than changing every later reader's copy.
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
      // The fields a schedule states for itself: where the node hangs in its
      // own hierarchy, and a fund's printed balances where it cites them.
      parent: n.parent || "",
    };
    // Not defaulted: an absent balance is a blank on the page, not a zero,
    // and an absent alternative is a member of its parent's primary split.
    if (n.alternative) Object.assign(node, { alternative: n.alternative });
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
    // Not defaulted: an absent alternative is a primary link, always read,
    // and an absent completes is a leg of no set Go declares.
    if (l.completes) Object.assign(link, { completes: l.completes });
    return l.alternative ? Object.assign(link, { alternative: l.alternative }) : link;
  });
  const splits = Array.isArray(sched.splits)
    ? sched.splits.map((sp) => ({
      node: (table[sp.node] || {}).id, key: sp.key, form: sp.form, alternative: Boolean(sp.alternative),
    }))
    : null;
  const col = column.column || {};
  const doc = {
    schema_version: SCHEMA_VERSION,
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
 * One schedule of a column document, with the shared node table's indices
 * resolved to ids, a node's identity and annotations taken from the table and
 * its parent from the schedule, and the keys the column omits (empty strings,
 * false booleans) filled in.
 *
 * @param {any} column
 * @param {string} key the schedule to read -- a step's `projection`
 * @returns {FiscProjection | null} null when the column carries no such schedule
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
 * The subtree of one node: its id and every id whose parent chain reaches it.
 * Walked upward per node rather than downward from the root, because a node
 * names its parent and nothing names its children.
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
 * The frontier a tier set draws: every node at one of those tiers, so a fold
 * to it places each node at its nearest ancestor-or-self at a drawn tier.
 * Empty where no node is at any of them, which scoped() answers as "nothing
 * is placed" and foldDocument refuses.
 *
 * @param {FiscProjection} doc
 * @param {Iterable<number>} tiers
 * @returns {Set<string>}
 */
export function tierFrontier(doc, tiers) {
  const drawn = new Set(tiers);
  return new Set(doc.nodes.filter((n) => drawn.has(n.tier)).map((n) => n.id));
}

/**
 * Whether a link end has a column in this tier set. A broken parent chain is
 * Go's to refuse at the write (node-hierarchy-well-formed), so a node with no
 * drawn ancestor is simply not placed.
 *
 * @param {FiscProjection} doc
 * @param {number[]} tiers
 * @returns {(n: FiscNode) => boolean}
 */
export function scoped(doc, tiers) {
  const byID = new Map(doc.nodes.map((n) => [n.id, n]));
  const frontier = tierFrontier(doc, tiers);
  return (n) => foldTarget(byID, n, frontier) !== "";
}

/**
 * The id a node folds to under a frontier: itself or its nearest ancestor in
 * it, or "" when it has neither. The non-throwing half of foldDocument's first
 * loop: to filterLinks an unplaceable end is an ordinary answer, not a fault.
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
 * Rebuild doc\u001fpage keys as FiscSource[] in the packager's order:
 * documents ascending, pages ascending within each, each once. Any other order
 * would render one page as a different citation on two charts.
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
 * One doc\u001fpage key per page a set of locators cites, the form
 * regroupLocators reads back.
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
 * The fold's refusal to merge two ribbons one mark cannot draw as one: a
 * cross-tab slice with a flow that is not one. A type and not a message, so
 * a caller that decides by it matches the refusal itself and never an error
 * that quotes or wraps its words.
 */
export class FoldRefusal extends Error {}

/**
 * The alternatives a frontier opens and the nodes it opens, both read off the
 * parent chains of the nodes in it: an alternative of a node is open while a
 * member of it, or a node inside one, is in the frontier, and a node is open
 * while a node strictly inside it is.
 * @param {Map<string,FiscNode>} byID
 * @param {ReadonlySet<string>} frontier
 * @returns {{alternatives: Set<string>, opened: Set<string>}} alternatives
 *   keyed node\u001fkey
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
 * Whether a node is read under a frontier: every alternative on its parent
 * chain, itself included, is open. A node under a shut alternative is not
 * money this chart draws, since its parent's other split carries the same.
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

/**
 * Folds a document to a frontier of node ids: each node to its nearest
 * ancestor-or-self in the frontier, links merged on the folded pair and kind
 * with values summed and fact ids and locators unioned. A link folding onto
 * one node is dropped; the link that survives carries the same money and
 * facts. A tier set is folded as tierFrontier's frontier of it.
 *
 * On the expansion schedule: a link carrying an alternative is read only
 * while that alternative of the node it runs from is open, and a node under a
 * shut alternative is not read. An extended node stays in the frontier beside
 * its members, so its link to a member is a ribbon while they are drawn and a
 * dropped self-loop while it is shut. A node the frontier opens -- one a node
 * strictly inside it is drawn for -- is drawn as its members and folds to
 * nothing.
 *
 * A ribbon whose legs are exactly the links naming one cell in `completes` is
 * that printed cell, and is drawn printed whichever of its legs Go derived:
 * a membership Go declared. Any part of such a set is not the cell. Any other
 * ribbon merging a printed leg and an inferred one is drawn inferred: a
 * capped split's tail can hold a remainder beside printed members, and the
 * ribbon claims no more than its least-printed leg.
 *
 * FAILS CLOSED on a read node that neither folds nor is opened, and on a read
 * link with an end that does not fold: dropping either loses money silently,
 * keeping it leaves it nowhere to draw.
 *
 * @param {FiscProjection} doc
 * @param {Iterable<string>} frontier the node ids drawn: a tier set's
 *   tierFrontier, or an expansion's
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
  // Locators union as fact_ids do, keyed doc\u001fpage; without this a merged
  // ribbon cites only its first leg's pages.
  /** @type {Map<string, Set<string>>} */
  const located = new Map();
  /** @type {Map<string, {completes: string, legs: number}>} */
  const legsOf = new Map();
  for (const l of doc.links) {
    if (l.alternative && !alternatives.has(l.source + "\u001f" + l.alternative)) continue;
    const source = foldsTo.get(l.source);
    const target = foldsTo.get(l.target);
    if (!source || !target) {
      throw new Error("cannot draw " + doc.projection + ": the link from " + l.source + " to " + l.target +
        " (" + l.kind + ") has an end this frontier draws no node for");
    }
    if (source === target) continue;
    // ONE RIBBON PER KIND BETWEEN A FOLDED PAIR, so a ribbon's kind is true of
    // all of it.
    const key = source + "\u001f" + target + "\u001f" + l.kind;
    const at = merged.get(key);
    const ids = cited.get(key);
    if (!at || !ids) {
      merged.set(key, Object.assign({}, l, { source: source, target: target }));
      // ITERATED, NOT new Set(l.fact_ids): that would turn an absent fact_ids
      // into a ribbon citing nothing instead of a throw.
      const first = new Set();
      for (const id of l.fact_ids) first.add(id);
      cited.set(key, first);
      located.set(key, new Set(locatorKeys(l.locators)));
      legsOf.set(key, { completes: l.completes || "", legs: 1 });
      continue;
    }
    // A PARTITION LEG AND ONE THAT IS NOT ARE NEVER ONE RIBBON: one mark
    // cannot be drawn as both, so the fold refuses rather than letting the
    // first leg's flag speak for the other.
    if (Boolean(at.partition) !== Boolean(l.partition)) {
      throw new FoldRefusal("cannot draw " + doc.projection + ": folding merges ribbons one mark cannot draw as one, " +
        "a cross-tab slice and a flow that is not one, from " + source + " to " + target + " (" + l.kind + ")");
    }
    const legs = /** @type {{completes: string, legs: number}} */ (legsOf.get(key));
    legs.legs++;
    if (legs.completes !== (l.completes || "")) legs.completes = "";
    at.derived = Boolean(at.derived) || Boolean(l.derived);
    // One leg's reduction words cannot name the sum; markContra names what
    // the merged ribbon nets to.
    if ((at.contra || "") !== (l.contra || "")) at.contra = "";
    at.value_cents += l.value_cents;
    // A transfer id names one leg of one transfer and cannot survive a merge.
    if (at.transfer_id !== l.transfer_id) at.transfer_id = "";
    for (const id of l.fact_ids) ids.add(id);
    const locs = located.get(key);
    if (locs) for (const k of locatorKeys(l.locators)) locs.add(k);
  }

  // A PRINTED LEG AND AN INFERRED ONE ARE A PRINTED RIBBON ONLY AS A
  // COMPLETING SET; otherwise the merge is drawn inferred, as at.derived
  // already says.
  for (const [key, l] of merged) {
    const legs = /** @type {{completes: string, legs: number}} */ (legsOf.get(key));
    if (legs.completes && legs.legs === sets.get(legs.completes)) {
      l.derived = false;
    }
    if (legs.completes) l.completes = legs.completes;
    else delete l.completes;
  }

  const links = Array.from(merged.entries())
    .map(([key, l]) => Object.assign(l, {
      fact_ids: Array.from(cited.get(key) || []).sort(),
      // The shape Go publishes, so citations() cannot tell a folded link.
      locators: regroupLocators(located.get(key)),
    }))
    .sort((a, b) => (a.source < b.source ? -1 : a.source > b.source ? 1
      : a.target < b.target ? -1 : a.target > b.target ? 1
      : a.kind < b.kind ? -1 : a.kind > b.kind ? 1 : 0));

  // An untouched node is not drawable: d3-sankey lands it in the first column
  // at zero height.
  const touched = new Set();
  for (const l of links) {
    touched.add(l.source);
    touched.add(l.target);
  }
  // A RETAINED NODE'S parent IS RE-POINTED AT ITS FOLDED ANCESTOR, so every
  // parent resolves in the folded document; "" where its parent folded into
  // it. A loop, because the walk continues past an ancestor the filter above
  // dropped.
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
 * The frontier the expansion schedule opens on: its roots, which are the
 * spine's nodes.
 * @param {FiscProjection} doc the expansion schedule
 * @returns {Set<string>}
 */
export function rootFrontier(doc) {
  return new Set(doc.nodes.filter((n) => !n.parent).map((n) => n.id));
}

/**
 * Every way a node opens, as Go declared it, each with its members: the nodes
 * parented to it whose alternative is the split's key on an alternative
 * split, or absent on its primary one.
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
 * The key of the split of a node a frontier has open, or "" when none is:
 * read off the member of it each drawn node strictly inside it hangs from.
 * FAILS CLOSED on a frontier drawing members of two of its splits, which
 * would draw the same money twice.
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
 * The frontier with a node shut: every node strictly inside it taken out and
 * the node put back. A frontier drawing nothing inside it is returned as it
 * was.
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
 * The frontier with one split of a drawn node open: a refine replaces the
 * node with its members, an extend draws them beside it. A node open another
 * way is shut first, so a frontier opens a node one way at a time.
 * FAILS CLOSED on a split the node does not have and on a node the frontier
 * does not draw.
 * @param {FiscProjection} doc the expansion schedule
 * @param {Iterable<string>} frontier
 * @param {string} id
 * @param {string} key the split to open, one of splitsOf(doc, id)
 * @returns {Set<string>} a new set
 */
export function expand(doc, frontier, id, key) {
  const split = splitsOf(doc, id).find((sp) => sp.key === key);
  if (!split) throw new Error("cannot open " + id + ": it has no split " + key);
  const open = openSplitOf(doc, frontier, id);
  if (open === key) return new Set(frontier);
  const out = open ? collapse(doc, frontier, id) : new Set(frontier);
  if (!out.has(id)) throw new Error("cannot open " + id + ": the frontier does not draw it");
  if (split.form === "refine") out.delete(id);
  for (const m of split.members) out.add(m);
  return out;
}

/**
 * The frontier with a node drawn: each ancestor of it opened, from its root
 * down, by the split the next node on the chain is a member of.
 * @param {FiscProjection} doc the expansion schedule
 * @param {Iterable<string>} frontier
 * @param {string} id
 * @returns {Set<string>} a new set
 */
export function reveal(doc, frontier, id) {
  const byID = new Map(doc.nodes.map((n) => [n.id, n]));
  const chain = [];
  for (let at = byID.get(id), hops = 0; at && hops < 9; hops++) {
    chain.unshift(at);
    at = at.parent ? byID.get(at.parent) : undefined;
  }
  let out = new Set(frontier);
  for (let i = 0; i + 1 < chain.length; i++) {
    const at = chain[i].id;
    const next = chain[i + 1];
    const primary = splitsOf(doc, at).find((sp) => !sp.alternative);
    const key = next.alternative || (primary ? primary.key : "");
    if (openSplitOf(doc, out, at) !== key) out = expand(doc, out, at, key);
  }
  return out;
}

/**
 * Folds all but the largest `cap` nodes of one tier into a single node.
 *
 * A cap, not a rescale: the concentration is within a column, and the fold
 * tests measure the sub-pixel ribbons it removes. Values sum and fact ids and
 * locators union, as in the fold, so the aggregate stays citable. What MAY
 * fold is Go's (DrillStep.Caps); which nodes it keeps is this page's.
 *
 * @param {FiscProjection} doc
 * @param {number} tier
 * @param {number} cap
 * @param {string} opened  the node the aggregate is parented to: the opened
 *   node when the whole column is inside it, "" when it spans fund groups
 * @param {string} noun  the plural noun for the tier's rows
 * @param {{members: ReadonlySet<string>, id: string}} [among] the column is
 *   these ids rather than every node at the tier, and its tail is `id`: one
 *   split's drawn members, which share a slot with other splits' members
 * @returns {FiscProjection} doc itself when the column fits whole
 */
export function capColumn(doc, tier, cap, opened, noun, among) {
  const inColumn = among
    ? (/** @type {FiscNode} */ n) => among.members.has(n.id)
    : (/** @type {FiscNode} */ n) => n.tier === tier;
  const tailID = among ? among.id : aggregateID(tier);
  const atTier = doc.nodes.filter(inColumn);
  // AN AGGREGATE OF ONE IS WORSE THAN NO AGGREGATE, so a column of cap + 1 is
  // drawn whole.
  if (atTier.length <= cap + 1) return doc;

  // RANKED BY THE LARGER OF INFLOW AND OUTFLOW, d3-sankey's node value: a
  // line takes in nothing, so inflow alone ties every line at zero. By
  // magnitude, because a contra row is a printed line as large as its figure.
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
  const kept = new Set(ranked.slice(0, cap).map((n) => n.id));
  const folded = ranked.slice(cap);

  // The noun is the view's, plural: the threshold above folds two or more.
  const aggregate = aggregateMark(tier, opened, folded.map((n) => n.id), atTier.length, cap, noun, tailID);
  const tail = new Set(folded.map((n) => n.id));
  const remap = (/** @type {string} */ id) => (tail.has(id) ? tailID : id);

  // A FOLDED NODE'S CHILDREN HANG FROM THE TAIL: foldDocument refuses a child
  // naming a parent the document does not carry, and dropping the child would
  // drop the money it carries onward -- a folded fund's object categories, in
  // a fund group's window.
  const nodes = doc.nodes
    .filter((n) => !inColumn(n) || kept.has(n.id))
    .map((n) => (tail.has(n.parent) ? Object.assign({}, n, { parent: tailID }) : n))
    .concat([aggregate]);
  // And the links that named them, for the same refusal.
  const present = new Set(nodes.map((n) => n.id));
  const links = doc.links
    .map((l) => Object.assign({}, l, { source: remap(l.source), target: remap(l.target) }))
    .filter((l) => present.has(l.source) && present.has(l.target));

  // THE FIGURE IS NOT ON THE MARK: tailFigure finishes the note after the
  // fold and any later cap have re-pointed these ribbons.
  return Object.assign({}, doc, { nodes: nodes, links: links });
}

/**
 * What a folded tail carries, at the height d3-sankey draws it: the larger of
 * what arrives and what leaves, each ribbon at its magnitude as markContra
 * draws a reduction.
 *
 * READ OFF THE FOLDED DOCUMENT, because the fold nets, re-points and drops
 * ribbons capColumn produced.
 *
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

/**
 * The prefix every capped tail's id carries.
 */
export const AGGREGATE_PREFIX = "aggregate/tail/";

/**
 * The id of the node one tier's capped tail is folded into. Per tier, because
 * a step can cap two columns and d3-sankey and the fold both key by id.
 * @param {number} tier
 * @returns {string}
 */
export function aggregateID(tier) {
  return AGGREGATE_PREFIX + tier;
}

/**
 * The id of the tail one split's capped members fold into. Per split, not per
 * tier: one slot holds the members of many splits, and each tail stays inside
 * the node it splits.
 * @param {string} node
 * @param {string} key the split's key
 * @returns {string}
 */
export function splitTailID(node, key) {
  return AGGREGATE_PREFIX + key + "/" + node;
}

/**
 * The node and split key a split's tail id names (splitTailID), or null for
 * an id that is no split's tail: a tier's tail (aggregateID) included.
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
 * Whether a document is an expansion schedule: one that declares how its
 * nodes split (schema/projection.schema.json's splits), which scheduleOf
 * carries on that schedule alone.
 * @param {FiscProjection | null | undefined} doc
 * @returns {boolean}
 */
export function isExpansionSchedule(doc) {
  return Boolean(doc) && Array.isArray(/** @type {any} */ (doc).splits);
}

/**
 * Whether an id names a capped tail of any tier.
 * @param {string} id
 * @returns {boolean}
 */
export function isAggregate(id) {
  return id.startsWith(AGGREGATE_PREFIX);
}

/**
 * The prefix a smaller-flows mark's id carries, followed by its side and
 * tier.
 */
export const SMALLER_PREFIX = "smaller/";

/** smallerMark's side for the mark the folded ribbons run TO. */
export const SMALLER_TO = "to";

/** smallerMark's side for the mark the folded ribbons run FROM. */
export const SMALLER_FROM = "from";

/**
 * The id of the mark one column's folded ribbons on one side end at. Per
 * side, so a mark either receives or sends and never reads as money passing
 * through it; per tier, and per slot where the shaping placed the far ends in
 * slots (expansionOf), since one tier can stand in several columns.
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
 * Whether an id is a mark this client makes: an aggregate or a smaller-flows
 * mark. The declared set is export.MarkPrefixes.
 * @param {string} id
 * @returns {boolean}
 */
export function isMark(id) {
  return isAggregate(id) || isSmaller(id);
}

/**
 * Whether a mark is a ribbon rather than a node: a ribbon cites facts and a
 * node does not, which is schema/projection.schema.json's shape and no
 * form's.
 * @param {LaidLink | LaidNode | FiscLink | FiscNode} d
 * @returns {boolean}
 */
export function isLink(d) {
  return Object.prototype.hasOwnProperty.call(d, "fact_ids");
}

/**
 * The aggregate a cap folds a column's tail into: schema/mark.schema.json,
 * role aggregate, worded by the page's aggregate_* templates. derived: true
 * IS THE INVARIANT: the city printed no line called "N smaller funds". Every cent inside is printed; the grouping is
 * inferred, and the ids it swallowed are kept so caveatsFor still reaches
 * them, since a tail folded by value is recorded by no parent chain.
 * @param {number} tier
 * @param {string} opened the node the aggregate hangs under, "" across groups
 * @param {string[]} folded the ids folded, two or more
 * @param {number} total how many nodes the column held before the cap
 * @param {number} cap the cap it was folded at
 * @param {string} word the plural noun the tail is counted in
 * @param {string} [id] the tail's id, aggregateID(tier) by default
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
 * The mark the smallest-flow filter folds one column's small ribbons onto:
 * schema/mark.schema.json, role smaller, worded by the page's smaller_*
 * templates. derived: true, because the city printed no line called "flows
 * under $10,000"; every cent inside is printed and the ribbons into or out of
 * it cite every fact and page they merge.
 * @param {string} side SMALLER_TO or SMALLER_FROM
 * @param {number} tier
 * @param {number} flows how many printed ribbons it stands for
 * @param {number} smallest the threshold they are under, in cents
 * @param {number} cents what they add up to
 * @param {FiscSource[]} locators every page those ribbons cite
 * @param {number} [slot] the slot of the column it stands in, where the
 *   shaping placed the far ends in slots
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
 * What the reader has asked the chart to leave out: the fund groups and link
 * kinds to hide, and the smallest flow drawn as itself, in cents. app.js's
 * FilterState is this plus the isolation.
 * @typedef {{groups: ReadonlySet<string>, kinds: ReadonlySet<string>, smallest: number}} Removals
 */

/**
 * A shaped chart with the reader's removals applied: ribbons in a hidden
 * fund group or of a hidden kind REMOVED, then every printed ribbon under
 * the smallest flow folded onto a smaller-flows mark, then every node no
 * ribbon touches dropped. Figures are untouched: a ribbon is drawn as it
 * came, merged with others by summing cents and unioning facts and pages,
 * or listed in `hidden`, and exactly one of the three.
 *
 * A RIBBON IS IN A GROUP WHEN EITHER END IS, so hiding a group hides every
 * ribbon touching it, whatever the other end. A node keeps fixedValue only
 * while every ribbon arriving at it is drawn: the figure it fixes is the net
 * of those ribbons.
 *
 * @param {FiscProjection} doc  a shaped chart, reductions already drawn forward
 * @param {(n: FiscNode) => string} groupOf  a node's fund group, "" for none
 * @param {Removals} state
 * @returns {{doc: FiscProjection, hidden: FiscLink[]}} doc itself where the
 *   state removes and folds nothing; otherwise a new chart marked filtered
 */
export function filterDocument(doc, groupOf, state) {
  const byID = new Map(doc.nodes.map((n) => [n.id, n]));
  const inHidden = (/** @type {string} */ id) => {
    const n = byID.get(id);
    const g = n ? groupOf(n) : "";
    return g !== "" && state.groups.has(g);
  };
  /** @type {FiscLink[]} */
  const hidden = [];
  /** @type {FiscLink[]} */
  const kept = [];
  for (const l of doc.links) {
    if (state.kinds.has(l.kind) || inHidden(l.source) || inHidden(l.target)) hidden.push(l);
    else kept.push(l);
  }
  if (!hidden.length && !(state.smallest > 0)) return { doc: doc, hidden: hidden };
  const folded = foldSmaller(kept, byID, state.smallest);
  /** @type {Set<string>} the nodes a ribbon that arrived at them no longer reaches as itself */
  const arrivingChanged = new Set(hidden.map((l) => l.target).concat(folded.cut));
  const touched = new Set();
  for (const l of folded.links) {
    touched.add(l.source);
    touched.add(l.target);
  }
  const nodes = doc.nodes.filter((n) => touched.has(n.id)).map((n) => {
    if (n.fixedValue === undefined || !arrivingChanged.has(n.id)) return n;
    const out = Object.assign({}, n);
    delete out.fixedValue;
    return out;
  });
  return {
    // filtered: a column the shaped chart filled may be empty now.
    doc: Object.assign({}, doc, { nodes: nodes.concat(folded.marks), links: folded.links, filtered: true }),
    hidden: hidden,
  };
}

/**
 * Folds every printed ribbon under `smallest` cents onto a smaller-flows
 * mark: each is kept at its LARGER end, sized as capColumn sizes a node, and
 * its other end becomes the mark at that end's column, one per column and
 * side, standing in that end's slot where it has one. Ribbons sharing both ends, kind and cross-tab flag after that are one
 * ribbon, summed and unioned as foldDocument merges; one leg alone is drawn
 * as itself, since a merge of one is worse than none.
 *
 * ONLY A PRINTED RIBBON BETWEEN PRINTED NODES FOLDS: an inferred ribbon, a
 * reduction, and a ribbon touching a mark this client made are drawn
 * whatever their size, because one merged ribbon cannot be drawn as both a
 * printed flow and one of those.
 *
 * @param {FiscLink[]} links
 * @param {Map<string, FiscNode>} byID
 * @param {number} smallest cents; 0 folds nothing
 * @returns {{links: FiscLink[], marks: FiscMark[], cut: string[]}} cut: the
 *   ids a folded ribbon no longer arrives at
 */
export function foldSmaller(links, byID, smallest) {
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
    !isMark(l.source) && !isMark(l.target) && byID.has(l.source) && byID.has(l.target);
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
    const pages = new Set();
    let cents = 0;
    let transfer = m.legs[0].transfer_id;
    for (const l of m.legs) {
      cents += l.value_cents;
      for (const id of l.fact_ids) ids.add(id);
      for (const k of locatorKeys(l.locators || [])) pages.add(k);
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
    for (const k of pages) mark.pages.add(k);
    marks.set(markID, mark);
    return Object.assign({}, m.legs[0], {
      source: m.source, target: m.target, value_cents: cents, transfer_id: transfer,
      fact_ids: Array.from(ids).sort(), locators: regroupLocators(pages),
      merged: say("smaller_flow", { flows: m.legs.length, threshold: fmt(smallest) }),
      legs: m.legs.slice(),
    });
  });
  return {
    links: out,
    marks: Array.from(marks.values()).map((m) =>
      smallerMark(m.side, m.tier, m.flows, smallest, m.cents, regroupLocators(m.pages), m.slot)),
    cut: cut,
  };
}

/* ------------------------------------------------------------------ *
 * The figures a mark carries: sums over cited ribbons, and a difference
 * held to a licence Go shipped. A form supplies what only it knows --
 * where a mark stands, and whether this width draws a column -- and
 * computes no figure of its own.
 * ------------------------------------------------------------------ */

/**
 * How much of a mark's figure is printed as reductions: the reductions among
 * the ribbons on the side its figure is read from, the larger of what
 * arrives and what leaves, each ribbon at its magnitude.
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
 * Which derived node of `doc` an inferred flow is listed under, or "" for one
 * whose endpoints are both printed.
 *
 * ONE ENTRY PER FLOW, so a flow with a derived node at both ends is not listed
 * twice. The mark it arrives at wins: an inferred flow is evidence about the
 * mark that receives it.
 *
 * @param {FiscProjection | null} doc
 * @param {FiscLink} l
 * @returns {string}
 */
export function homeOf(doc, l) {
  if (!doc) return "";
  const derived = new Set(doc.nodes.filter((n) => n.derived).map((n) => n.id));
  if (derived.has(l.target)) return l.target;
  if (derived.has(l.source)) return l.source;
  return "";
}

export function tableRows(doc) {
  const out = [];
  if (!doc) return out;
  const labels = new Map(doc.nodes.map((n) => [n.id, n.label]));
  const ours = new Set(doc.nodes.filter((n) => n.derived).map((n) => n.id));

  for (const l of doc.links) {
    const tr = document.createElement("tr");
    // A contra row reads as printed: signed, with the words for what it reduces.
    if (l.contra) tr.className = "contra";
    tr.append(h("td", "", labels.get(l.source) || l.source));
    tr.append(h("td", "", labels.get(l.target) || l.target));
    tr.append(h("td", "num", fmtSigned(l.contra ? -l.value_cents : l.value_cents)));
    tr.append(h("td", "", kindLabel(l.kind)));
    tr.append(h("td", "", l.derived ? say("inferred_chip")
      : l.contra ? l.contra
        : l.partition ? PARTITION_NOTE
          : ours.has(l.source) || ours.has(l.target) ? say("carried_chip") : say("printed_chip")));
    tr.append(h("td", "ids", l.fact_ids.join(" ")));
    // PER ROW, not per document: the pages this row's own figure was read from.
    const td = h("td", "");
    for (const c of citations(l.locators)) {
      td.append(link(c.label, c.href));
      td.append(document.createTextNode(" "));
    }
    tr.append(td);
    out.push(tr);
  }
  return out;
}
