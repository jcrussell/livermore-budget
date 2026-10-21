// @ts-check
/**
 * fisc — the page plumbing ledger.html runs on. Importing it
 * runs nothing; a form module's boot() hands this its renderer. It reads the
 * page's sides config, fetches the column the year control names, folds the
 * expansion schedule to the frontier the hash opens (core.sideTree), and
 * hands each declared side's tree to the renderer, which draws it. The
 * tooltip, the hash and the gestures that open and shut a
 * tile are here, so a form module holds what it draws and nothing else.
 *
 * Every figure here is core.js's: a sum of cited cents at a membership Go
 * declared. A share is this page's sentence about what is drawn, marked ◇.
 */

import * as core from "./core.js";
import { configOf, dollars, hashFields, hashValue, citedPages, pageLinks, selectYear, tipHint, tipper, wireYearSelect, yearKey } from "./format.js";

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
 *   pin: (mark: HTMLElement, tile: core.SideTile, side: Side, e?: {clientX?: number, clientY?: number, target?: any}) => void,
 *   hide: () => void,
 *   share: (tile: core.SideTile, side: Side) => string,
 *   clone: (id: string) => HTMLElement | SVGElement,
 * }} DrawContext
 *   scale is px per cent, one for every side on the page, from the largest
 *   root drawn.
 *   tip shows a tile's tip unless one is pinned; pin pins it under its mark
 *   (format.js's tipper).
 * @typedef {{form: string, draw: (section: HTMLElement, side: Side, ctx: DrawContext) => void}} SidesRenderer
 */

/**
 * The page's config, read from its JSON script element.
 * @param {Document} doc
 */
export function readConfig(doc) {
  const config = configOf(doc, "sides-config", "sides");
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
  const o = st.opened.map(hashValue).join(",");
  return "#y=" + hashValue(yearKey(st.year)) + (o ? "&o=" + o : "");
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
  const fields = hashFields(hash, HASH_FIELDS);
  const years = config.years.map((/** @type {any} */ y) => y.path);
  const out = { year: years[years.length - 1], opened: /** @type {string[]} */ ([]), refused: "" };
  const y = fields.get("y");
  if (y !== undefined) {
    const named = years.find((/** @type {string} */ p) => yearKey(p) === y);
    if (named) out.year = named;
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
 * A tile's share of its side marked ours, or "" for a printed reduction:
 * a reduction takes no share and its side's total leaves out every root
 * that is one.
 * @param {core.SideTile} tile
 * @param {Side} side
 */
export function tileShare(tile, side) {
  if (tile.contra) return "";
  const whole = side.tree.roots.reduce((a, r) => (r.contra ? a : a + r.cents), 0);
  const pct = share(tile.cents, whole);
  return pct ? "◇ " + pct : "";
}

/**
 * The words a tile's tooltip says of how it opens: its primary
 * split's member count, and each alternative's, marked ours as a count of
 * what is drawn; "" for a tile that does not open.
 * @param {core.SideTile} tile
 */
export function opensAs(tile) {
  if (!tile.splits.length) return "";
  return "◇ " + tile.splits.map((sp) => `${sp.members.length} ${sp.key}${sp.alternative ? " (alternative)" : ""}`).join("; ") +
    ", by our count";
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
 * Writes a tile's tip into the emptied tooltip: its label, figure, share,
 * how it opens, and the pages it cites, each linked to its extracted text
 * and its PDF page by the config's `docs`.
 * @param {Document} doc
 * @param {HTMLElement} tip
 * @param {core.SideTile} tile
 * @param {Side} side
 * @param {Parameters<typeof citedPages>[0]} docs
 * @param {boolean} pinned
 */
export function fillTip(doc, tip, tile, side, docs, pinned) {
  const label = doc.createElement("div");
  label.className = "tip-label";
  label.textContent = tile.label + (tile.derived ? " (inferred)" : "");
  const value = doc.createElement("div");
  value.className = "tip-value";
  value.textContent = dollars(tile.cents);
  const meta = doc.createElement("div");
  meta.className = "tip-meta";
  const own = tileShare(tile, side);
  const parts = own ? [`${own} of ${side.heading.toLowerCase()}`] : [];
  if (tile.contra) parts.push(tile.contra);
  if (tile.splits.length) parts.push((tile.children ? "Opened; click to close. " : "Click to open: ") + opensAs(tile));
  meta.textContent = parts.join(". ");
  tip.append(label, value, meta, pageLinks(doc, citedPages(docs, tile.locators)));
  // A tile that opens says so in its meta, and a click opens it, so its
  // figure or P keeps its tip.
  if (pinned || !tile.splits.length) tip.append(tipHint(doc, pinned));
  else {
    const hint = doc.createElement("div");
    hint.className = "tip-meta";
    hint.textContent = "Click its figure, or press P, to keep this open and follow its pages.";
    tip.append(hint);
  }
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
 * Draws every side of a column at a state through the renderer and returns
 * what was drawn with the entries the schedule refused.
 * @param {Window} win
 * @param {any} config
 * @param {any} column
 * @param {SidesState} st
 * @param {SidesRenderer} renderer
 * @param {(tile: core.SideTile, key?: string) => void} toggle
 * @param {ReturnType<typeof tipper>} tips the page's tooltip
 */
export function render(win, config, column, st, renderer, toggle, tips) {
  const doc = win.document;
  const { frontier, refused } = frontierOf(core.scheduleOf(column, config.primary) || { nodes: [], links: [], projection: config.primary }, st.opened);
  const { sides } = sidesAt(config, column, frontier);
  const largest = Math.max(1, ...sides.flatMap((s) => s.tree.roots.map((r) => r.size)));
  /** @type {DrawContext} */
  const ctx = {
    doc,
    scale: 1 / largest,
    toggle,
    tip: (e, tile, side) => tips.show(e, (tip, pinned) => fillTip(doc, tip, tile, side, config.docs, pinned)),
    pin: (mark, tile, side, e) => tips.pin(mark, (tip, pinned) => fillTip(doc, tip, tile, side, config.docs, pinned), e),
    hide: () => tips.hide(),
    share: tileShare,
    clone: (id) => clone(doc, id),
  };
  for (const side of sides) {
    const section = /** @type {HTMLElement | null} */ (doc.querySelector(`section.sides-side[data-tier="${side.tier}"]`));
    if (!section) throw new Error(`the page has no section for tier ${side.tier}`);
    const total = section.querySelector(".sides-total");
    if (total) total.textContent = `◇ our sum of the ${side.tree.roots.length} drawn at this tier: ${dollars(side.tree.cents)}`;
    renderer.draw(section, side, ctx);
  }
  const year = config.years.find((/** @type {any} */ y) => y.path === st.year);
  const lede = doc.getElementById("lede-year");
  if (lede && year) lede.textContent = year.lede;
  return { sides, frontier, refused };
}

/**
 * Hands the focus back to the mark of the tile of this id where a side draws
 * it again, as an open or close redraws the row it was asked on: the next
 * Enter lands on that row, and no tip is pinned or shown over its members.
 * @param {Document} doc
 * @param {Side[]} sides
 * @param {string} id
 * @param {ReturnType<typeof tipper>} tips
 */
function focusDrawn(doc, sides, id, tips) {
  for (const side of sides) {
    const section = doc.querySelector(`section.sides-side[data-tier="${side.tier}"]`);
    const mark = section && Array.from(section.querySelectorAll("[data-id]")).find((m) => /** @type {HTMLElement} */ (m).dataset.id === id);
    if (!mark) continue;
    tips.focus(/** @type {HTMLElement} */ (mark));
    return;
  }
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

  const tips = tipper(doc);
  /** The tile a toggle hands the focus back to once the view it asked for is drawn, and that view's ticket. */
  let refocus = { id: "", ticket: 0 };
  /** @type {SidesState} */
  let state = { year: config.years[config.years.length - 1].path, opened: [] };
  /** The schedule of the column drawn, which a gesture closes against. */
  /** @type {FiscProjection | undefined} */
  let schedule;
  let shown = 0;
  /** @param {SidesState} st @param {boolean} push */
  const show = async (st, push) => {
    const ticket = ++shown;
    selectYear(doc, yearKey(st.year));
    try {
      const col = await column(st.year);
      if (ticket !== shown) return;
      schedule = core.scheduleOf(col, config.primary);
      tips.unpin(false);
      const drawn = render(win, config, col, st, renderer, toggle, tips);
      if (refocus.ticket === ticket) focusDrawn(doc, drawn.sides, refocus.id, tips);
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
      if (ticket !== shown) return;
      // The control and the top bar's links go back to the column on screen.
      selectYear(doc, yearKey(state.year));
      say(doc, "The chart could not be drawn: " + String(e));
    }
  };
  /** @type {(tile: core.SideTile, key?: string) => void} */
  const toggle = (tile, key) => {
    say(doc, "");
    tips.unpin(false);
    tips.hide();
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
    refocus = { id: tile.id, ticket: shown + 1 };
    void show({ year: state.year, opened }, true);
  };
  const restore = async () => {
    const st = parseHash(win.location.hash, config);
    say(doc, st.refused);
    await show({ year: st.year, opened: st.opened }, false);
  };
  wireYearSelect(doc, (key) => {
    const year = config.years.find((/** @type {any} */ y) => yearKey(y.path) === key);
    if (!year) return;
    say(doc, "");
    void show({ year: year.path, opened: state.opened }, true);
  });
  win.addEventListener("popstate", () => void restore());
  await restore();
}
