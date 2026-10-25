// @ts-check
/**
 * fisc — ledger.html's plumbing: config, column fetch, the hash's frontier
 * (core.sideTree), tooltip and open/shut gestures; the form module draws
 * each side. Every figure is core.js's; a share is ours, marked ◇.
 */

import * as core from "./core.js";
import { clone, columnLoader, configOf, hashYear, sayRefusal, dollars, share, hashFields, hashValue, citedPages, pageLinks, selectYear, tipHead, tipHint, tipLine, tipper, wireYearSelect, yearKey } from "./format.js";

/**
 * app.js's typedefs, each the shape it names by its schema/ path there.
 * @typedef {import("./app.js").FiscProjection} FiscProjection
 */

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
 *   scale: px per cent, one for the page, from the largest root drawn.
 * @typedef {{form: string, sidesOf: (chart: any) => SideHint[], draw: (section: HTMLElement, side: Side, ctx: DrawContext) => void}} SidesRenderer
 */

/** @param {Document} doc */
export function readConfig(doc) {
  const config = configOf(doc, "sides-config", "sides");
  if (!config.overview || !config.overview.form) throw new Error("the config declares no chart");
  return config;
}

/**
 * `#y=<column file less .json>&o=<id>[~key],...`.
 * @param {SidesState} st
 */
export function hashOf(st) {
  const o = st.opened.map(hashValue).join(",");
  return "#y=" + hashValue(yearKey(st.year)) + (o ? "&o=" + o : "");
}

/**
 * @param {string} hash
 * @param {any} config
 * @returns {SidesState & {refused: string}}
 */
export function parseHash(hash, config) {
  const fields = hashFields(hash, HASH_FIELDS);
  const out = { ...hashYear(fields, config), opened: /** @type {string[]} */ ([]) };
  const o = fields.get("o");
  if (o) out.opened = o.split(",").filter(Boolean);
  return out;
}

/**
 * Replays entries through core.expand from the roots; refused ones are
 * dropped and named.
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
 * Drops the tile's entry and all inside it, from core.straightFrom, so a
 * member opened through its parent closes with it; a leftover entry would
 * be replayed on a frontier that no longer draws it, and refused.
 * @param {FiscProjection} doc the expansion schedule
 * @param {string[]} opened
 * @param {string} id
 * @returns {string[]}
 */
export function closing(doc, opened, id) {
  return outside(doc, opened, core.straightFrom(doc, id));
}

/**
 * Like closing() but from the tile itself, so switching a tile opened
 * straight through keeps the parent's entry.
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
 * Roots that are not printed reductions, and how many are left out.
 * @param {Side} side
 * @returns {{roots: number, cents: number, reductions: number}}
 */
export function shareTotal(side) {
  const counted = side.tree.roots.filter((r) => !r.contra);
  return {
    roots: counted.length,
    cents: counted.reduce((a, r) => a + r.cents, 0),
    reductions: side.tree.roots.length - counted.length,
  };
}

/**
 * "1 bar" / "N bars", `where`, and that reductions are left out, if any.
 * @param {{roots: number, reductions: number}} whole
 * @param {string} where
 */
export function countedBars(whole, where) {
  const one = whole.roots === 1;
  return `${whole.roots} ${one ? "bar" : "bars"}${where}` +
    (whole.reductions ? (one ? " that is not a printed reduction" : " that are not printed reductions") : "");
}

/**
 * "" for a printed reduction, which takes no share.
 * @param {core.SideTile} tile
 * @param {Side} side
 */
export function tileShare(tile, side) {
  if (tile.contra) return "";
  const whole = shareTotal(side).cents;
  const pct = share(tile.cents, whole);
  return pct ? "◇ " + pct : "";
}

/**
 * Member counts per split, marked ours. Asked only of a tile that opens.
 * @param {core.SideTile} tile
 */
export function opensAs(tile) {
  return "◇ " + tile.splits.map((sp) => `${sp.members.length} ${sp.key}${sp.alternative ? " (alternative)" : ""}`).join("; ") +
    ", by our count";
}

/**
 * @param {any} config
 * @param {any} column
 * @param {Set<string>} frontier
 * @param {SidesRenderer} renderer
 * @returns {{doc: FiscProjection, sides: Side[]}}
 */
function sidesAt(config, column, frontier, renderer) {
  const doc = core.scheduleOf(column, config.primary);
  if (!doc) throw new Error(`the column carries no ${config.primary} schedule`);
  if (!core.isExpansionSchedule(doc)) throw new Error(`${config.primary} declares no splits, so nothing here opens`);
  const sides = renderer.sidesOf(config.overview).map((s) => ({ tier: s.tier, heading: s.heading, tree: core.sideTree(doc, frontier, s.tier) }));
  return { doc, sides };
}

/**
 * @param {Document} doc
 * @param {HTMLElement} tip
 * @param {core.SideTile} tile
 * @param {Side} side
 * @param {Parameters<typeof citedPages>[0]} docs
 * @param {boolean} pinned
 */
export function fillTip(doc, tip, tile, side, docs, pinned) {
  const own = tileShare(tile, side);
  const parts = own ? [`${own} of ${side.heading.toLowerCase()}`] : [];
  if (tile.contra) parts.push(tile.contra);
  if (tile.splits.length) parts.push((tile.children ? "Opened; click to close. " : "Click to open: ") + opensAs(tile));
  tip.append(...tipHead(doc, tile.label + (tile.derived ? " (inferred)" : ""), dollars(tile.cents), parts.join(". ")),
    pageLinks(doc, citedPages(docs, tile.locators)));
  // A tile that opens says so in its meta, and a click opens it, so its
  // figure or P keeps its tip.
  tip.append(pinned || !tile.splits.length ? tipHint(doc, pinned)
    : tipLine(doc, "tip-meta", "Click its figure, or press P, to keep this open and follow its pages."));
}

/**
 * @param {Window & typeof globalThis} win
 * @param {any} config
 * @param {any} column
 * @param {SidesState} st
 * @param {SidesRenderer} renderer
 * @param {(tile: core.SideTile, key?: string) => void} toggle
 * @param {ReturnType<typeof tipper>} tips
 */
export function render(win, config, column, st, renderer, toggle, tips) {
  const doc = win.document;
  const { frontier, refused } = frontierOf(core.scheduleOf(column, config.primary) || /** @type {any} */ ({ nodes: [], links: [], projection: config.primary }), st.opened);
  const { sides } = sidesAt(config, column, frontier, renderer);
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
    const whole = shareTotal(side);
    if (total) total.textContent = `◇ our sum of the ${countedBars(whole, " drawn at this tier")}: ${dollars(whole.cents)}`;
    renderer.draw(section, side, ctx);
  }
  const year = config.years.find((/** @type {any} */ y) => y.path === st.year);
  const lede = doc.getElementById("lede-year");
  if (lede && year) lede.textContent = year.lede;
  return { sides, frontier, refused };
}

/**
 * After a redraw, focus the row asked on, with no tip over its members.
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
 * Resolves once the opening view is drawn or refused.
 * @param {Window & typeof globalThis} win
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
    renderer.sidesOf(config.overview);
  } catch (e) {
    sayRefusal(doc, "sides", "This page could not read its config: " + String(e));
    return;
  }
  const column = columnLoader(win, config.exported_by);

  const tips = tipper(doc);
  /** The tile a toggle hands the focus back to once the view it asked for is drawn, and that view's ticket. */
  let refocus = { id: "", ticket: 0 };
  /** @type {SidesState} */
  let state = { year: config.years[config.years.length - 1].path, opened: [] };
  /** The schedule of the column drawn, which a gesture closes against. */
  /** @type {FiscProjection | null | undefined} */
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
      const kept = frontierOf(/** @type {FiscProjection} */ (schedule), st.opened).kept;
      state = { year: st.year, opened: kept };
      if (drawn.refused.length) {
        // After what restore() said of the year, so a hash wrong in both is
        // answered for both.
        const prior = doc.getElementById("sides-refusal");
        sayRefusal(doc, "sides", (prior ? prior.textContent + " " : "") + "Some of what the address opens could not be drawn: " + drawn.refused.join("; "));
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
      sayRefusal(doc, "sides", "The chart could not be drawn: " + String(e));
    }
  };
  /** @type {(tile: core.SideTile, key?: string) => void} */
  const toggle = (tile, key) => {
    sayRefusal(doc, "sides", "");
    tips.unpin(false);
    tips.hide();
    if (!schedule) return;
    const primary = tile.splits.find((sp) => !sp.alternative);
    const want = key || (primary ? primary.key : "");
    // A click closes an open tile either way; a key on a tile open another
    // way switches it.
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
    sayRefusal(doc, "sides", st.refused);
    await show({ year: st.year, opened: st.opened }, false);
  };
  wireYearSelect(doc, (key) => {
    const year = config.years.find((/** @type {any} */ y) => yearKey(y.path) === key);
    if (!year) return;
    sayRefusal(doc, "sides", "");
    void show({ year: year.path, opened: state.opened }, true);
  });
  win.addEventListener("popstate", () => void restore());
  await restore();
}
