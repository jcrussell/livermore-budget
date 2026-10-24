// @ts-check
/**
 * fisc — the ledger form: each side of the expansion schedule as ranked
 * bars, one scale for every side on the page, an opened bar followed by its
 * members' bars indented under it (core.sideTree). Importing it runs
 * nothing; the template calls boot(), which hands sides.js this renderer.
 * Reads of the config only its own hints, in sidesOf. No d3: a bar is
 * a span whose width is its figure's share of the largest root drawn.
 */

import * as core from "./core.js";
import { dollars } from "./format.js";
import { boot as bootSides, countedBars, shareTotal } from "./sides.js";

/** How many roots wear a hue of their own before the rest wear the tail's gray. */
export const RANKED = 5;

/**
 * The width of a bar as a percentage of the track, from the page's one
 * scale: the largest root drawn on any side fills its track.
 * @param {number} size
 * @param {number} scale px per cent, as sides.js hands it: 1 / the largest size
 */
export function widthOf(size, scale) {
  const pct = 100 * size * scale;
  return pct <= 0 ? 0 : Math.max(pct, 0.25);
}

/**
 * A side's list label: the count and sum sides.js shareTotal states in the
 * side's visible total, so a printed reduction root is in neither, as it is
 * in no share's total.
 * @param {import("./sides.js").Side} side
 */
export function listLabel(side) {
  const whole = shareTotal(side);
  return `${side.heading}: ◇ ${countedBars(whole, " by our count")}, ${dollars(whole.cents)} in all by our sum`;
}

/**
 * The ledger renderer sides.js draws each side through.
 * @type {import("./sides.js").SidesRenderer}
 */
export const LEDGER = Object.freeze({
  form: "ledger",

  /**
   * The sides a ledger chart declares, its own hints and no other form's,
   * refused where it declares none.
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
   * Draws a side's tree into its section's figure: an ordered list, one
   * `li.bar-row` per tile, roots ranked largest first and each opened
   * root's members indented under it, every bar on the page's one scale. A
   * row that opens takes a click or Enter, which opens or closes it and pins
   * nothing; Shift opens its next alternative. A row that does not open pins
   * its tip on a click or Enter. Every row pins its tip on P, and a row that
   * opens on a click on its figure.
   */
  draw(section, side, ctx) {
    const figure = section.querySelector("figure.sides-figure");
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
      // A row that opens is opened or closed, and sides.js hands it the
      // focus once it is drawn again; any other row pins its tip here.
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

/**
 * Boots ledger.html through sides.js with this renderer.
 * @param {Window} [win]
 */
export function boot(win = globalThis.window) {
  return bootSides(win, LEDGER);
}
