// @ts-check
/**
 * fisc — the ledger form: each side as ranked bars on one page-wide scale,
 * opened members indented beneath (core.sideTree). No d3: a bar is a span.
 * Reads only its own hints, in sidesOf.
 */

import * as core from "./core.js";
import { dollars } from "./format.js";
import { boot as bootSides, countedBars, shareTotal } from "./sides.js";

/** How many roots wear a hue of their own before the rest wear the tail's gray. */
export const RANKED = 5;

/**
 * @param {number} size
 * @param {number} scale px per cent: 1 / the largest size
 */
export function widthOf(size, scale) {
  const pct = 100 * size * scale;
  return pct <= 0 ? 0 : Math.max(pct, 0.25);
}

/**
 * shareTotal's count and sum, so a reduction root is in neither.
 * @param {import("./sides.js").Side} side
 */
export function listLabel(side) {
  const whole = shareTotal(side);
  return `${side.heading}: ◇ ${countedBars(whole, " by our count")}, ${dollars(whole.cents)} in all by our sum`;
}

/** @type {import("./sides.js").SidesRenderer} */
export const LEDGER = Object.freeze({
  form: "ledger",

  /**
   * Refused where the chart declares none.
   * @param {any} chart
   * @returns {import("./sides.js").SideHint[]}
   */
  sidesOf(chart) {
    const hints = chart && chart.ledger;
    if (!hints || !Array.isArray(hints.sides) || !hints.sides.length) {
      throw new Error("the config declares no sides for its ledger chart");
    }
    return hints.sides;
  },

  /**
   * One `li.bar-row` per tile. Click or Enter opens or closes a row that
   * opens (Shift: next alternative), else pins its tip; P always pins.
   */
  draw(section, side, ctx) {
    const figure = /** @type {HTMLElement | null} */ (section.querySelector("figure.sides-figure"));
    if (!figure) throw new Error("the side has no figure to draw into");
    const list = /** @type {HTMLElement} */ (ctx.clone("ledger-list"));
    list.setAttribute("aria-label", listLabel(side));
    const rankOf = new Map(side.tree.roots.map((r, i) => [r.id, i < RANKED ? i + 1 : 0]));
    /** @param {core.SideTile} tile @param {number} depth @param {string} root */
    const row = (tile, depth, root) => {
      const li = /** @type {HTMLElement} */ (ctx.clone("ledger-row"));
      li.dataset.id = tile.id;
      li.dataset.rank = String(rankOf.get(root) || 0);
      li.dataset.depth = String(depth);
      li.dataset.cents = String(tile.cents);
      li.style.setProperty("--depth", String(depth));
      if (tile.contra) li.classList.add("reduction");
      if (tile.derived) li.classList.add("derived");
      if (tile.children) li.classList.add("opened");
      if (tile.splits.length) li.classList.add("can-open");
      li.setAttribute("aria-label", `${tile.label}: ${dollars(tile.cents)}` + (ctx.share(tile, side) ? `, ${ctx.share(tile, side)} of ${side.heading}` : "") +
        (tile.splits.length ? (tile.children ? "; opened" : "; opens") : ""));
      const label = li.querySelector(".bar-label");
      const fill = /** @type {HTMLElement | null} */ (li.querySelector(".bar-fill"));
      const value = li.querySelector(".bar-value");
      const shareEl = li.querySelector(".bar-share");
      if (!label || !fill || !value || !shareEl) throw new Error("the row template lacks a label, fill, value or share");
      label.textContent = (tile.contra ? "− " : "") + tile.label + (tile.derived ? " (inferred)" : "");
      fill.style.width = widthOf(tile.size, ctx.scale).toFixed(3) + "%";
      value.textContent = core.fmtShortSigned(tile.cents);
      shareEl.textContent = ctx.share(tile, side);
      li.addEventListener("pointerenter", (e) => ctx.tip(e, tile, side));
      li.addEventListener("pointermove", (e) => ctx.tip(e, tile, side));
      li.addEventListener("pointerleave", () => ctx.hide());
      li.addEventListener("focus", (e) => ctx.tip(e, tile, side));
      li.addEventListener("blur", () => ctx.hide());
      // sides.js refocuses an opened row after redraw; others pin here.
      /** @param {MouseEvent | KeyboardEvent} e */
      const press = (e) => {
        if (!tile.splits.length) {
          ctx.pin(li, tile, side, e);
          return;
        }
        const alt = tile.splits.find((sp) => sp.alternative);
        ctx.toggle(tile, e.shiftKey && alt ? alt.key : undefined);
      };
      li.dataset.pins = "";
      li.addEventListener("click", (e) => {
        e.stopPropagation();
        press(e);
      });
      li.addEventListener("keydown", (e) => {
        if ((e.key === "p" || e.key === "P") && !e.ctrlKey && !e.metaKey && !e.altKey) {
          e.preventDefault();
          ctx.pin(li, tile, side, e);
          return;
        }
        if (e.key !== "Enter" && e.key !== " ") return;
        e.preventDefault();
        press(e);
      });
      if (tile.splits.length) {
        value.addEventListener("click", (e) => {
          e.stopPropagation();
          ctx.pin(li, tile, side, e);
        });
      }
      list.appendChild(li);
      if (tile.children) {
        const members = [...tile.children].sort((a, b) => b.size - a.size || a.label.localeCompare(b.label));
        for (const m of members) row(m, depth + 1, root);
      }
    };
    for (const r of side.tree.roots) row(r, 0, r.id);
    figure.replaceChildren(list);
    figure.hidden = false;
  },
});

/** @param {Window & typeof globalThis} [win] */
export function boot(win = globalThis.window) {
  return bootSides(win, LEDGER);
}
