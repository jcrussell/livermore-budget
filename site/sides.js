// @ts-check
/**
 * fisc — the page plumbing treemap.html and ledger.html share. Importing it
 * runs nothing; a form module's boot() hands this its renderer. It reads the
 * page's sides config, fetches the column the year control names, folds the
 * expansion schedule to the frontier the hash opens (core.sideTree), and
 * hands each declared side's tree to the renderer, which draws it. The
 * tooltip, the table view, the hash and the gestures that open and shut a
 * tile are here, so the two forms differ in what they draw and nothing else.
 *
 * Every figure here is core.js's: a sum of cited cents at a membership Go
 * declared. A share is this page's sentence about what is drawn, marked ◇.
 */

import * as core from "./core.js";

/** The hash's fields: the column and the opened nodes, in order. */
const HASH_FIELDS = ["y", "o"];

/** Separates a node id from the key of the split it is opened by in the hash. */
export const KEY_MARK = "~";

/**
 * @typedef {{tier: number, heading: string}} SideHint
 * @typedef {{tier: number, heading: string, tree: ReturnType<typeof core.sideTree>}} Side
 * @typedef {{year: string, opened: string[]}} SidesState
 *   year is the column file; opened lists nodes as `id` or `id~key`.
 * @typedef {{
 *   doc: Document,
 *   scale: number,
 *   toggle: (tile: core.SideTile, key?: string) => void,
 *   tip: (e: {clientX?: number, clientY?: number, target?: any}, tile: core.SideTile, side: Side) => void,
 *   hide: () => void,
 *   share: (tile: core.SideTile, side: Side) => string,
 *   clone: (id: string) => HTMLElement | SVGElement,
 * }} DrawContext
 *   scale is px per cent, one for every side on the page, from the largest
 *   root drawn.
 * @typedef {{form: string, draw: (section: HTMLElement, side: Side, ctx: DrawContext) => void}} SidesRenderer
 */

/**
 * The page's config, read from its JSON script element.
 * @param {Document} doc
 */
export function readConfig(doc) {
  const el = doc.getElementById("sides-config");
  if (!el) throw new Error("the page carries no sides config");
  const config = JSON.parse(el.textContent || "");
  const hints = config.overview && config.overview[config.overview.form];
  if (!hints || !Array.isArray(hints.sides) || !hints.sides.length) {
    throw new Error("the config declares no sides for its " + (config.overview ? config.overview.form : "") + " chart");
  }
  return config;
}

/**
 * The declared sides of the config's chart, its own form's hints and no other's.
 * @param {any} config
 * @returns {SideHint[]}
 */
export function sidesOf(config) {
  return config.overview[config.overview.form].sides;
}

/**
 * A view as a hash: `#y=<column file less .json>&o=<id>[~key],...`.
 * @param {SidesState} st
 */
export function hashOf(st) {
  const esc = (/** @type {string} */ v) => encodeURIComponent(v).replace(/%2F/g, "/");
  const o = st.opened.map(esc).join(",");
  return "#y=" + esc(st.year.replace(/\.json$/, "")) + (o ? "&o=" + o : "");
}

/**
 * A hash read back into a view over the config: the column it names or the
 * opening one, and the nodes it opens, with `refused` saying what it named
 * that the config does not offer.
 * @param {string} hash
 * @param {any} config
 * @returns {SidesState & {refused: string}}
 */
export function parseHash(hash, config) {
  /** @type {Map<string, string>} */
  const fields = new Map();
  for (const part of String(hash || "").replace(/^#/, "").split("&")) {
    const eq = part.indexOf("=");
    if (eq < 0) continue;
    const key = part.slice(0, eq);
    if (!HASH_FIELDS.includes(key)) continue;
    try {
      fields.set(key, decodeURIComponent(part.slice(eq + 1)));
    } catch {
      fields.set(key, part.slice(eq + 1));
    }
  }
  const years = config.years.map((/** @type {any} */ y) => y.path);
  const out = { year: years[years.length - 1], opened: /** @type {string[]} */ ([]), refused: "" };
  const y = fields.get("y");
  if (y !== undefined) {
    if (years.includes(y + ".json")) out.year = y + ".json";
    else out.refused = `No budget column is called ${y}, so this shows the newest.`;
  }
  const o = fields.get("o");
  if (o) out.opened = o.split(",").filter(Boolean);
  return out;
}

/**
 * The frontier a list of opened entries comes to, replayed in order through
 * core.expand from the schedule's roots, with every entry the schedule
 * refuses left out and named. An entry is `id` for a node's primary split or
 * `id~key` for one of its others.
 * @param {FiscProjection} doc the expansion schedule
 * @param {string[]} opened
 * @returns {{frontier: Set<string>, kept: string[], refused: string[]}}
 */
export function frontierOf(doc, opened) {
  let frontier = core.rootFrontier(doc);
  /** @type {string[]} */
  const kept = [];
  /** @type {string[]} */
  const refused = [];
  for (const entry of opened) {
    const at = entry.indexOf(KEY_MARK);
    const id = at < 0 ? entry : entry.slice(0, at);
    const splits = core.splitsOf(doc, id);
    const primary = splits.find((sp) => !sp.alternative);
    const key = at < 0 ? (primary ? primary.key : "") : entry.slice(at + 1);
    try {
      if (!key) throw new Error(id + " has no split to open");
      frontier = core.expand(doc, frontier, id, key);
      kept.push(entry);
    } catch (e) {
      refused.push(`${entry}: ${e instanceof Error ? e.message : String(e)}`);
    }
  }
  return { frontier, kept, refused };
}

/**
 * The opened entries with a tile shut: its own entry and every entry inside
 * it taken off, from the node whose opening opened it (core.straightFrom), so
 * a member opened straight through its parent's one-member split closes with
 * its parent. A member's own entry left behind would be replayed on a
 * frontier that no longer draws it, and refused.
 * @param {FiscProjection} doc the expansion schedule
 * @param {string[]} opened
 * @param {string} id
 * @returns {string[]}
 */
export function closing(doc, opened, id) {
  return outside(doc, opened, core.straightFrom(doc, id));
}

/**
 * The opened entries not inside one node: what survives opening or closing
 * it. Opening filters from the tile itself, so a tile opened straight
 * through its parent's one-member split can be switched to its other split
 * with the parent's entry kept, where closing() would fold the parent too.
 * @param {FiscProjection} doc the expansion schedule
 * @param {string[]} opened
 * @param {string} id
 * @returns {string[]}
 */
export function outside(doc, opened, id) {
  const inside = core.withinNode(doc, id);
  return opened.filter((entry) => {
    const at = entry.indexOf(KEY_MARK);
    return !inside.has(at < 0 ? entry : entry.slice(0, at));
  });
}

/**
 * Whole dollars, signed, cents only where a figure has them.
 * @param {number} cents
 */
export function dollars(cents) {
  const whole = cents % 100 === 0;
  return (cents < 0 ? "-$" : "$") + (Math.abs(cents) / 100).toLocaleString("en-US",
    { minimumFractionDigits: whole ? 0 : 2, maximumFractionDigits: 2 });
}

/**
 * A tile's share of its side's drawn total, as this page's percentage.
 * @param {number} part
 * @param {number} whole
 */
export function share(part, whole) {
  if (!whole) return "";
  const pct = (100 * Math.abs(part)) / Math.abs(whole);
  return pct < 0.1 ? "<0.1%" : pct.toFixed(1) + "%";
}

/**
 * The words a tile's tooltip and table row say of how it opens: its primary
 * split's member count, and each alternative's.
 * @param {core.SideTile} tile
 */
export function opensAs(tile) {
  return tile.splits.map((sp) => `${sp.members.length} ${sp.key}${sp.alternative ? " (alternative)" : ""}`).join("; ");
}

/**
 * Clones the page's template for one piece of a figure.
 * @param {Document} doc
 * @param {string} id
 * @returns {HTMLElement | SVGElement}
 */
export function clone(doc, id) {
  const tmpl = doc.getElementById(id);
  if (!(tmpl instanceof doc.defaultView.HTMLTemplateElement)) {
    throw new Error(`the page carries no <template id="${id}"> to clone`);
  }
  const el = tmpl.content.firstElementChild;
  if (!el) throw new Error(`<template id="${id}"> is empty`);
  return /** @type {HTMLElement | SVGElement} */ (el.cloneNode(true));
}

/**
 * One link per page a tile cites, each to that page's text as the config
 * names it, or the page number alone where the config names none. Read off
 * this page's config, never a global: these pages ship no FISC_CONFIG.
 * @param {any} config
 * @param {FiscSource[]} locators
 * @returns {{label: string, href: string}[]}
 */
export function citationsOf(config, locators) {
  /** @type {{label: string, href: string}[]} */
  const out = [];
  for (const loc of locators) {
    const d = config.docs && config.docs[loc.doc_id];
    for (const p of loc.pages) {
      const page = d && d.pages && d.pages[String(p)];
      out.push({ label: `p${p}`, href: page && page.text ? String(page.text) : "" });
    }
  }
  return out;
}

/**
 * The sides of one column at one frontier, each its declared tier's tree.
 * @param {any} config
 * @param {any} column
 * @param {Set<string>} frontier
 * @returns {{doc: FiscProjection, sides: Side[]}}
 */
export function sidesAt(config, column, frontier) {
  const doc = core.scheduleOf(column, config.primary);
  if (!doc) throw new Error(`the column carries no ${config.primary} schedule`);
  if (!core.isExpansionSchedule(doc)) throw new Error(`${config.primary} declares no splits, so nothing here opens`);
  const sides = sidesOf(config).map((s) => ({ tier: s.tier, heading: s.heading, tree: core.sideTree(doc, frontier, s.tier) }));
  return { doc, sides };
}

/**
 * Fills the table view: one row per drawn tile of every side, parents before
 * their members, with the tile's share of its side marked ours.
 * @param {Document} doc
 * @param {Side[]} sides
 * @param {any} config
 */
export function fillTable(doc, sides, config) {
  const body = doc.querySelector("#sides-table tbody");
  if (!body) return;
  body.replaceChildren();
  for (const side of sides) {
    for (const { tile, depth } of core.sideTiles(side.tree.roots)) {
      const tr = doc.createElement("tr");
      tr.dataset.side = String(side.tier);
      tr.dataset.id = tile.id;
      tr.dataset.cents = String(tile.cents);
      tr.dataset.depth = String(depth);
      if (tile.contra) tr.className = "contra";
      const cells = [side.heading, tile.label, tile.parent ? (labelOf(side, tile.parent) || tile.parent) : "",
        dollars(tile.cents), "◇ " + share(tile.cents, side.tree.cents)];
      cells.forEach((text, i) => {
        const td = doc.createElement(i === 1 ? "th" : "td");
        if (i === 1) td.setAttribute("scope", "row");
        if (i >= 3) td.className = "num";
        td.textContent = text;
        tr.append(td);
      });
      const pages = doc.createElement("td");
      for (const c of citationsOf(config, tile.locators)) {
        if (c.href) {
          const a = doc.createElement("a");
          a.href = c.href;
          a.textContent = c.label;
          pages.append(a);
        } else pages.append(c.label);
        pages.append(doc.createTextNode(" "));
      }
      tr.append(pages);
      const note = doc.createElement("td");
      note.textContent = tile.derived ? "inferred" : tile.contra ? tile.contra : "";
      tr.append(note);
      body.append(tr);
    }
  }
  const count = doc.getElementById("sides-count");
  if (count) {
    const n = sides.reduce((a, s) => a + core.sideTiles(s.tree.roots).length, 0);
    count.textContent = `◇ our count: ${n} tiles drawn across ${sides.length} sides`;
  }
}

/**
 * The label of a drawn tile of a side, or "" where none has that id.
 * @param {Side} side
 * @param {string} id
 */
function labelOf(side, id) {
  const found = core.sideTiles(side.tree.roots).find((t) => t.tile.id === id);
  return found ? found.tile.label : "";
}

/**
 * Shows the tooltip for a tile beside the pointer, or under a focused mark.
 * @param {Document} doc
 * @param {{clientX?: number, clientY?: number, target?: any}} e
 * @param {core.SideTile} tile
 * @param {Side} side
 */
export function showTip(doc, e, tile, side) {
  const tip = doc.getElementById("tooltip");
  if (!tip) return;
  tip.replaceChildren();
  const label = doc.createElement("div");
  label.className = "tip-label";
  label.textContent = tile.label + (tile.derived ? " (inferred)" : "");
  const value = doc.createElement("div");
  value.className = "tip-value";
  value.textContent = dollars(tile.cents);
  const meta = doc.createElement("div");
  meta.className = "tip-meta";
  const parts = [`◇ ${share(tile.cents, side.tree.cents)} of ${side.heading.toLowerCase()}`];
  if (tile.contra) parts.push(tile.contra);
  if (tile.splits.length) parts.push((tile.children ? "Opened; click to close. " : "Click to open: ") + opensAs(tile));
  meta.textContent = parts.join(". ");
  const facts = doc.createElement("div");
  facts.className = "facts";
  facts.textContent = tile.locators.map((l) => l.pages.map((p) => `p${p}`).join(", ")).join("; ");
  tip.append(label, value, meta, facts);
  tip.hidden = false;
  const win = doc.defaultView;
  // Beside the pointer, or under the mark itself on a focus event, which
  // carries no pointer: a keyboard user's tip must be seen beside the bar
  // they reached.
  let x = 0;
  let y = 0;
  if (typeof e.clientX === "number" && typeof e.clientY === "number") {
    x = e.clientX;
    y = e.clientY;
  } else if (e.target && typeof (/** @type {any} */ (e.target)).getBoundingClientRect === "function") {
    const r = (/** @type {Element} */ (e.target)).getBoundingClientRect();
    x = r.left;
    y = r.bottom;
  }
  tip.style.left = (x + (win ? win.scrollX : 0) + 14) + "px";
  tip.style.top = (y + (win ? win.scrollY : 0) + 14) + "px";
}

/**
 * Hides the tooltip.
 * @param {Document} doc
 */
export function hideTip(doc) {
  const tip = doc.getElementById("tooltip");
  if (tip) tip.hidden = true;
}

/**
 * Says a refusal at the top of the page, or clears it.
 * @param {Document} doc
 * @param {string} text
 */
export function say(doc, text) {
  let el = doc.getElementById("sides-refusal");
  if (!text) {
    if (el) el.remove();
    return;
  }
  if (!el) {
    el = doc.createElement("p");
    el.id = "sides-refusal";
    el.className = "refusal";
    el.setAttribute("role", "alert");
    const main = doc.getElementById("sides-main");
    if (main) main.prepend(el);
  }
  el.textContent = text;
}

/**
 * Draws every side of a column at a state through the renderer, fills the
 * table, and returns what was drawn with the entries the schedule refused.
 * @param {Window} win
 * @param {any} config
 * @param {any} column
 * @param {SidesState} st
 * @param {SidesRenderer} renderer
 * @param {(tile: core.SideTile, key?: string) => void} toggle
 */
export function render(win, config, column, st, renderer, toggle) {
  const doc = win.document;
  const { frontier, refused } = frontierOf(core.scheduleOf(column, config.primary) || { nodes: [], links: [], projection: config.primary }, st.opened);
  const { sides } = sidesAt(config, column, frontier);
  const largest = Math.max(1, ...sides.flatMap((s) => s.tree.roots.map((r) => r.size)));
  /** @type {DrawContext} */
  const ctx = {
    doc,
    scale: 1 / largest,
    toggle,
    tip: (e, tile, side) => showTip(doc, e, tile, side),
    hide: () => hideTip(doc),
    share: (tile, side) => share(tile.cents, side.tree.cents),
    clone: (id) => clone(doc, id),
  };
  for (const side of sides) {
    const section = /** @type {HTMLElement | null} */ (doc.querySelector(`section.sides-side[data-tier="${side.tier}"]`));
    if (!section) throw new Error(`the page has no section for tier ${side.tier}`);
    const total = section.querySelector(".sides-total");
    if (total) total.textContent = `◇ our sum of the ${side.tree.roots.length} drawn at this tier: ${dollars(side.tree.cents)}`;
    renderer.draw(section, side, ctx);
  }
  fillTable(doc, sides, config);
  const data = doc.getElementById("sides-data-path");
  if (data) {
    data.setAttribute("href", st.year);
    data.textContent = st.year;
  }
  const year = config.years.find((/** @type {any} */ y) => y.path === st.year);
  const lede = doc.getElementById("lede-year");
  if (lede && year) lede.textContent = year.lede;
  return { sides, frontier, refused };
}

/**
 * Boots a sides page with its renderer: reads the config, opens the view the
 * hash names, and wires the year select, the gestures and Back and Forward.
 * Resolves once the opening view is drawn or refused.
 * @param {Window} win
 * @param {SidesRenderer} renderer
 */
export async function boot(win, renderer) {
  const doc = win.document;
  /** @type {any} */
  let config;
  try {
    config = readConfig(doc);
    if (config.overview.form !== renderer.form) {
      throw new Error(`the page declares a ${config.overview.form} chart and this script draws a ${renderer.form}`);
    }
  } catch (e) {
    say(doc, "This page could not read its config: " + String(e));
    return;
  }
  const yearSelect = /** @type {HTMLSelectElement | null} */ (doc.getElementById("year-select"));
  /** @type {Map<string, Promise<any>>} */
  const columns = new Map();
  const column = (/** @type {string} */ path) => {
    let c = columns.get(path);
    if (!c) {
      c = Promise.resolve(win.fetch(path)).then((/** @type {any} */ r) => {
        if (!r.ok) throw new Error(`${path} answered ${r.status}`);
        return r.json();
      }).then((/** @type {any} */ col) => {
        if (col.generated_by !== config.exported_by) {
          throw new Error(`${path} was written by ${col.generated_by} and this page by ${config.exported_by}; reload the page`);
        }
        return col;
      });
      c.catch(() => columns.delete(path));
      columns.set(path, c);
    }
    return c;
  };

  /** @type {SidesState} */
  let state = { year: config.years[config.years.length - 1].path, opened: [] };
  /** The schedule of the column drawn, which a gesture closes against. */
  /** @type {FiscProjection | undefined} */
  let schedule;
  let shown = 0;
  /** @param {SidesState} st @param {boolean} push */
  const show = async (st, push) => {
    const ticket = ++shown;
    if (yearSelect) yearSelect.value = st.year;
    try {
      const col = await column(st.year);
      if (ticket !== shown) return;
      schedule = core.scheduleOf(col, config.primary);
      const drawn = render(win, config, col, st, renderer, toggle);
      // The state keeps what the schedule drew, so the hash never names an
      // entry the next load would refuse again.
      const kept = frontierOf(schedule, st.opened).kept;
      state = { year: st.year, opened: kept };
      if (drawn.refused.length) {
        // After what restore() said of the year, so a hash wrong in both is
        // answered for both.
        const prior = doc.getElementById("sides-refusal");
        say(doc, (prior ? prior.textContent + " " : "") + "Some of what the address opens could not be drawn: " + drawn.refused.join("; "));
      }
      const hash = hashOf(state);
      if (win.location.hash !== hash) {
        if (push) win.history.pushState(null, "", hash);
        else win.history.replaceState(null, "", hash);
      }
    } catch (e) {
      if (ticket === shown) say(doc, "The chart could not be drawn: " + String(e));
    }
  };
  /** @type {(tile: core.SideTile, key?: string) => void} */
  const toggle = (tile, key) => {
    say(doc, "");
    hideTip(doc);
    if (!schedule) return;
    const primary = tile.splits.find((sp) => !sp.alternative);
    const want = key || (primary ? primary.key : "");
    // A click on an opened tile closes it, whichever way it is open, as the
    // key and the tooltip say; a keyed gesture on a tile open another way
    // switches it, and on a tile open that way closes it.
    const open = tile.children ? tile.open : "";
    const opens = !open || (key && open !== key);
    const opened = opens ? outside(schedule, state.opened, tile.id) : closing(schedule, state.opened, tile.id);
    if (opens) {
      if (!want) return;
      opened.push(want === (primary ? primary.key : "") ? tile.id : tile.id + KEY_MARK + want);
    }
    void show({ year: state.year, opened }, true);
  };
  const restore = async () => {
    const st = parseHash(win.location.hash, config);
    say(doc, st.refused);
    await show({ year: st.year, opened: st.opened }, false);
  };
  if (yearSelect) {
    yearSelect.addEventListener("change", () => {
      say(doc, "");
      void show({ year: yearSelect.value, opened: state.opened }, true);
    });
  }
  win.addEventListener("popstate", () => void restore());
  await restore();
}
