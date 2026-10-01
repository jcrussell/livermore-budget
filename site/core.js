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
 * A link kind in the page's words, as the export ships them. The empty kind
 * is a gap's, which crosses no printed boundary, and has no words.
 * @param {string} kind
 * @returns {string}
 */
export function kindLabel(kind) {
  if (kind === "") return "";
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

/**
 * A list of phrases as English: "a", "a or b", "a, b or c" (no serial comma).
 * @param {string[]} parts
 * @returns {string}
 */
export function joinOr(parts) {
  if (parts.length < 3) return parts.join(" or ");
  return parts.slice(0, -1).join(", ") + " or " + parts[parts.length - 1];
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
 * Each column's schedules, assembled once: a document is read per mark per
 * paint (stepDecomposes), and caches keyed on it (decomposable) only hit if
 * one column's schedule is one object. Frozen, so a caller that would change
 * it throws rather than changing every later reader's copy.
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
    return {
      id: base.id, label: base.label, tier: base.tier,
      role: base.role || "", derived: Boolean(base.derived),
      constraint_tier: base.constraint_tier || "",
      rationale: base.rationale || "", source_note: base.source_note || "",
      // The one field a schedule states for itself: where the node hangs in
      // its own hierarchy.
      parent: n.parent || "",
    };
  });
  const links = sched.links.map((l) => {
    const from = table[l.from] || {};
    const to = table[l.to] || {};
    return {
      source: from.id, target: to.id,
      value_cents: l.value_cents, kind: l.kind,
      transfer_id: l.transfer_id || "",
      // Not defaulted: a default would invent provenance.
      fact_ids: l.fact_ids, locators: l.locators,
      derived: Boolean(l.derived), partition: Boolean(l.partition),
      // Defaulted: the schema carries contra only on a link printed negative.
      contra: l.contra || "",
    };
  });
  const col = column.column || {};
  return /** @type {any} */ ({
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
  });
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
  const drawn = new Set(tiers);
  return (n) => foldTarget(byID, n, drawn) !== "";
}

/**
 * The id a node folds to under a tier set, or "" when it has no drawn
 * ancestor. The non-throwing half of foldDocument's first loop: to filterLinks
 * an unplaceable end is an ordinary answer, not a fault.
 * @param {Map<string,FiscNode>} byID
 * @param {FiscNode} n
 * @param {Set<number>} drawn
 * @returns {string}
 */
export function foldTarget(byID, n, drawn) {
  let at = n;
  for (let hops = 0; !drawn.has(at.tier); hops++) {
    const up = at.parent ? byID.get(at.parent) : undefined;
    if (!up || hops > 8) return "";
    at = up;
  }
  return at.id;
}

/**
 * Rebuild doc\u001fpage keys as FiscSource[] in the packager's order:
 * documents ascending, pages ascending within each, each once. Any other order
 * would render one page as a different citation on the spine and in a drill.
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

/** The fold's refusal to merge two ribbons one mark cannot draw as one. */
export const FOLD_REFUSES_MIXED = "folding merges ribbons one mark cannot draw as one";

/**
 * Folds a document to the tiers this page draws: each node to its nearest
 * drawn ancestor, links merged on the folded pair and kind with values summed
 * and fact ids unioned. A link folding onto one node is dropped; the link
 * that survives carries the same money and facts.
 *
 * FAILS CLOSED on a node with no drawn ancestor: dropping it loses a column
 * silently, keeping it leaves it nowhere to draw.
 *
 * @param {FiscProjection} doc
 * @param {number[]} tiers the tier set to fold to: the overview's own on the
 *   page's chart, a step's once a node has opened.
 * @returns {FiscProjection} doc itself when the tier set draws every tier.
 */
export function foldDocument(doc, tiers) {
  const wanted = tiers;
  if (!wanted.length) return doc;
  const byID = new Map(doc.nodes.map((n) => [n.id, n]));
  const drawn = new Set(wanted);

  /** @type {Map<string,string>} */
  const foldsTo = new Map();
  for (const n of doc.nodes) {
    const to = foldTarget(byID, n, drawn);
    if (!to) {
      throw new Error("cannot draw " + doc.projection + ": node " + n.id +
        " is tier " + n.tier + " and no ancestor of it is a tier this page draws (" +
        wanted.join(", ") + ")");
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
  /** @param {FiscSource[]} ss @returns {string[]} */
  const locatorKeys = (ss) => {
    const out = [];
    for (const s of ss) {
      for (const p of s.pages) out.push(s.doc_id + "\u001f" + p);
    }
    return out;
  };
  for (const l of doc.links) {
    const source = foldsTo.get(l.source);
    const target = foldsTo.get(l.target);
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
      continue;
    }
    // A PRINTED LEG AND AN INFERRED ONE, OR A PARTITION LEG AND ONE THAT IS
    // NOT, ARE NEVER ONE RIBBON: one mark cannot be drawn as both, so the fold
    // refuses rather than letting the first leg's flag speak for the other.
    const mixed = Boolean(at.derived) !== Boolean(l.derived) ? "a printed flow and an inferred one"
      : Boolean(at.partition) !== Boolean(l.partition) ? "a cross-tab slice and a flow that is not one"
        : "";
    if (mixed) {
      throw new Error("cannot draw " + doc.projection + ": " + FOLD_REFUSES_MIXED + ", " + mixed +
        ", from " + source + " to " + target + " (" + l.kind + ")");
    }
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
 * @returns {FiscProjection} doc itself when the column fits whole
 */
export function capColumn(doc, tier, cap, opened, noun) {
  const atTier = doc.nodes.filter((n) => n.tier === tier);
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
  const word = noun || "items";
  const label = folded.length + " smaller " + word;
  // derived: true IS THE INVARIANT: the city printed no line called "N
  // smaller funds". Every cent inside is printed; the grouping is inferred.
  const aggregate = {
    id: aggregateID(tier), label: label, tier: tier,
    parent: opened,
    constraint_tier: "",
    // An empty role renders as an empty .chip.
    role: "aggregate",
    // THE IDS IT SWALLOWED, so caveatsFor still reaches them: the tail is
    // folded by value, which no parent chain records.
    folds: [],
    derived: true,
    rationale: "Our grouping, not a line the city printed: the " + folded.length +
      " smallest " + word + " in this column are drawn as one " +
      "mark because they cannot be drawn separately. Every figure inside it is printed; " +
      "the box around them is ours.",
    source_note: "",
  };
  const tail = new Set(folded.map((n) => n.id));
  const remap = (/** @type {string} */ id) => (tail.has(id) ? aggregateID(tier) : id);

  aggregate.folds = folded.map((n) => n.id);

  // A FOLDED NODE'S CHILDREN HANG FROM THE TAIL: foldDocument refuses a child
  // naming a parent the document does not carry, and dropping the child would
  // drop the money it carries onward -- a folded fund's object categories, in
  // a fund group's window.
  const nodes = doc.nodes
    .filter((n) => n.tier !== tier || kept.has(n.id))
    .map((n) => (tail.has(n.parent) ? Object.assign({}, n, { parent: aggregateID(tier) }) : n))
    .concat([aggregate]);
  // And the links that named them, for the same refusal.
  const present = new Set(nodes.map((n) => n.id));
  const links = doc.links
    .map((l) => Object.assign({}, l, { source: remap(l.source), target: remap(l.target) }))
    .filter((l) => present.has(l.source) && present.has(l.target));

  // THE FIGURE IS NOT HERE: tailFigure finishes the note after the fold and
  // any later cap have re-pointed these ribbons.
  aggregate.source_note = "The " + folded.length + " smallest of " + atTier.length +
    " by value, at this page's cap of " + cap;

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
 * Whether an id names a capped tail of any tier.
 * @param {string} id
 * @returns {boolean}
 */
export function isAggregate(id) {
  return id.startsWith(AGGREGATE_PREFIX);
}

/**
 * The prefix the residual node's id carries, followed by the opened node's id.
 */
export const RESIDUAL_PREFIX = "residual/";

/**
 * The id of the node an opened node's undecomposed flows are carried onto.
 * @param {string} opened
 * @returns {string}
 */
export function residualID(opened) {
  return RESIDUAL_PREFIX + opened;
}

/**
 * Whether an id names a residual node.
 * @param {string} id
 * @returns {boolean}
 */
export function isResidual(id) {
  return id.startsWith(RESIDUAL_PREFIX);
}

/**
 * The prefix a gap node's id carries, followed by the opened node's id. Not
 * the residual's: a residual is carried, a gap is derived.
 */
export const GAP_PREFIX = "gap/";

/**
 * The id of the node an opened node's gap is drawn under.
 * @param {string} opened
 * @returns {string}
 */
export function gapID(opened) {
  return GAP_PREFIX + opened;
}

/**
 * Whether an id names a gap node.
 * @param {string} id
 * @returns {boolean}
 */
export function isGap(id) {
  return id.startsWith(GAP_PREFIX);
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
