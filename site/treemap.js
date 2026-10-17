// @ts-check
/**
 * fisc — the treemap form: each side of the expansion schedule as nested
 * tiles, a tile's area its figure's size (core.sideTree), an opened tile a
 * box of its members. Importing it runs nothing; the template calls boot(),
 * which hands sides.js this renderer. d3 is read through globalThis when a
 * layout asks for it. Reads of the config only its own hints, through
 * sides.js.
 */

import * as core from "./core.js";
import { boot as bootSides, dollars } from "./sides.js";

/** How tall one side's canvas is, in px. */
export const HEIGHT = 420;

/** The band above an opened tile's members where its label stands, in px. */
export const HEAD = 20;

/** The surface gap between tiles, in px. */
export const GAP = 2;

/** How many roots wear a hue of their own before the rest wear the tail's gray. */
export const RANKED = 5;

/**
 * Lays one side out: d3.hierarchy over the side's roots, every tile's box
 * its own size, then d3.treemap at the width given. A tile's area is its
 * share of the side whether or not it is open: its members divide ITS box
 * by their sizes, so a reduction among them takes room from its siblings
 * and never grows the parent past its own figure. Summing the leaves
 * instead would give an opened tile its members' gross, which is larger
 * than the tile wherever a member counts against it. The layout is pixels;
 * every figure stays the tree's.
 * @param {ReturnType<typeof core.sideTree>} tree
 * @param {number} width
 * @param {any} [d3lib] d3, or the global
 */
export function layOut(tree, width, d3lib = /** @type {any} */ (globalThis).d3) {
  if (!d3lib || typeof d3lib.treemap !== "function") throw new Error("d3.treemap is not loaded");
  const root = d3lib.hierarchy({ id: "", label: "", children: tree.roots, cents: tree.cents, size: tree.size }, (/** @type {any} */ t) => t.children);
  root.value = tree.roots.reduce((a, r) => a + r.size, 0);
  root.eachBefore((/** @type {any} */ d) => {
    if (!d.children) return;
    const gross = d.children.reduce((/** @type {number} */ a, /** @type {any} */ c) => a + c.data.size, 0);
    for (const c of d.children) c.value = gross ? (d.value * c.data.size) / gross : 0;
  });
  root.sort((/** @type {any} */ a, /** @type {any} */ b) => b.value - a.value);
  d3lib.treemap()
    .size([Math.max(width, 200), HEIGHT])
    .paddingInner(GAP)
    .paddingOuter(GAP)
    .paddingTop((/** @type {any} */ d) => (d.depth ? HEAD : GAP))
    .round(true)(root);
  return root;
}

/**
 * Whether a label fits a tile, by a plain measure: 7px a character at the
 * tile's font and 6px of padding either side. A label that does not fit is
 * not drawn in the tile; the tooltip and the table carry it.
 * @param {string} text
 * @param {number} width
 * @param {number} height
 */
export function fits(text, width, height) {
  return height >= 16 && text.length * 7 + 12 <= width;
}

/**
 * The treemap renderer sides.js draws each side through.
 * @type {import("./sides.js").SidesRenderer}
 */
export const TREEMAP = Object.freeze({
  form: "treemap",

  /**
   * Draws a side's tree into its section's figure: one `g.tile` per laid
   * node, roots and members alike, each focusable, with its label and
   * figure inside where they fit. A tile that opens takes a click or Enter;
   * Shift opens its next alternative. Rank colours the roots and every
   * member wears its root's.
   */
  draw(section, side, ctx) {
    const doc = ctx.doc;
    const figure = section.querySelector("figure.sides-figure");
    if (!figure) throw new Error("the side has no figure to draw into");
    const width = Math.max(figure.clientWidth || section.clientWidth || 920, 200);
    const root = layOut(side.tree, width);
    const svg = /** @type {SVGElement} */ (ctx.clone("treemap-canvas"));
    svg.setAttribute("viewBox", `0 0 ${width} ${HEIGHT}`);
    svg.setAttribute("width", String(width));
    svg.setAttribute("height", String(HEIGHT));
    svg.setAttribute("aria-label", `${side.heading}: ${side.tree.roots.length} tiles, ${dollars(side.tree.cents)} in all`);
    const rankOf = new Map(side.tree.roots.map((r, i) => [r.id, i < RANKED ? i + 1 : 0]));
    for (const d of root.descendants()) {
      if (!d.depth) continue;
      /** @type {core.SideTile} */
      const tile = d.data;
      let top = d;
      while (top.depth > 1) top = top.parent;
      // The template wraps the tile in an <svg> so the parser reads it as
      // SVG; the tile is the group inside.
      const wrap = ctx.clone("treemap-tile");
      const g = /** @type {SVGElement} */ (wrap.tagName.toLowerCase() === "svg" ? wrap.firstElementChild : wrap);
      if (!g) throw new Error("the tile template is empty");
      g.dataset.id = tile.id;
      g.dataset.rank = String(rankOf.get(top.data.id) || 0);
      g.dataset.depth = String(d.depth);
      g.dataset.cents = String(tile.cents);
      if (tile.contra) g.classList.add("reduction");
      if (tile.derived) g.classList.add("derived");
      if (tile.children) g.classList.add("opened");
      if (tile.splits.length) g.classList.add("can-open");
      g.setAttribute("transform", `translate(${d.x0},${d.y0})`);
      g.setAttribute("aria-label", `${tile.label}: ${dollars(tile.cents)}, ◇ ${ctx.share(tile, side)} of ${side.heading}` +
        (tile.splits.length ? (tile.children ? "; opened" : "; opens") : ""));
      const rect = g.querySelector("rect.tile-face");
      if (!rect) throw new Error("the tile template has no face");
      const w = Math.max(d.x1 - d.x0, 0);
      const h = Math.max(d.y1 - d.y0, 0);
      rect.setAttribute("width", String(w));
      rect.setAttribute("height", String(tile.children ? HEAD - GAP : h));
      if (tile.children) {
        const box = g.querySelector("rect.tile-box");
        if (box) {
          box.setAttribute("width", String(w));
          box.setAttribute("height", String(h));
        }
      } else {
        const box = g.querySelector("rect.tile-box");
        if (box) box.remove();
      }
      const label = g.querySelector("text.tile-label");
      const value = g.querySelector("text.tile-value");
      const words = (tile.contra ? "− " : "") + tile.label;
      const money = core.fmtShortSigned(tile.cents);
      const bandH = tile.children ? HEAD - GAP : h;
      if (label) {
        if (fits(words, w, bandH)) {
          label.textContent = words;
          label.setAttribute("x", "6");
          label.setAttribute("y", String(Math.min(14, bandH - 4)));
        } else label.remove();
      }
      if (value) {
        if (!tile.children && fits(money, w, h - 16) && fits(words, w, h)) {
          value.textContent = money;
          value.setAttribute("x", "6");
          value.setAttribute("y", "30");
        } else value.remove();
      }
      g.addEventListener("pointerenter", (e) => ctx.tip(e, tile, side));
      g.addEventListener("pointermove", (e) => ctx.tip(e, tile, side));
      g.addEventListener("pointerleave", () => ctx.hide());
      g.addEventListener("focus", (e) => ctx.tip(e, tile, side));
      g.addEventListener("blur", () => ctx.hide());
      g.addEventListener("click", (e) => {
        e.stopPropagation();
        if (!tile.splits.length) return;
        const alt = tile.splits.find((sp) => sp.alternative);
        ctx.toggle(tile, e.shiftKey && alt ? alt.key : undefined);
      });
      g.addEventListener("keydown", (e) => {
        if (e.key !== "Enter" && e.key !== " ") return;
        e.preventDefault();
        if (!tile.splits.length) return;
        const alt = tile.splits.find((sp) => sp.alternative);
        ctx.toggle(tile, e.shiftKey && alt ? alt.key : undefined);
      });
      svg.appendChild(g);
    }
    figure.replaceChildren(svg);
    figure.hidden = false;
  },
});

/**
 * Boots treemap.html through sides.js with this renderer.
 * @param {Window} [win]
 */
export function boot(win = globalThis.window) {
  return bootSides(win, TREEMAP);
}
