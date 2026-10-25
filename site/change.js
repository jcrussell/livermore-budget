// @ts-check
/**
 * fisc — change.html's module; the template calls boot(). Groups each
 * server-rendered table's rows by category and stacks the per-column sums
 * on one scale per figure. Every sum is ours, marked ◇.
 */

import { NAMED, TAIL_LABEL, clone, dollars, linkedPage, pageLinks, pinnable, plus, refuseFigure, rowCents, tipHead, tipHint, tipper } from "./format.js";

/** How tall the tallest column's bands are drawn, in px, gaps aside. */
export const HEIGHT = 240;

/** The px between two stacked bands, and the least a band is drawn. */
export const GAP = 2;
export const LEAST = 1;

/**
 * @typedef {{category: string, label: string, cents: (number | null)[]}} Row
 *   cents: one per column, null where the row prints none.
 * @typedef {{id: string, label: string, cents: (number | null)[], members: string[], rank: number}} Band
 *   A category or the tail. rank is by the last column, 1..NAMED+1 (a sixth
 *   is named rather than folded alone), 0 for the tail.
 */

/**
 * Refuses a row with no category or a non-integer figure: a cell the page
 * cannot place is not stacked.
 * @param {HTMLTableElement} table
 * @returns {{rows: Row[], columns: number}}
 */
export function readRows(table) {
  const columns = table.tHead ? table.tHead.querySelectorAll("th[data-column]").length : 0;
  /** @type {Row[]} */
  const rows = [];
  for (const body of Array.from(table.tBodies)) {
    for (const tr of Array.from(body.rows)) {
      const category = tr.dataset.category || "";
      const head = tr.querySelector("th[scope=row]");
      const label = head ? (head.textContent || "").trim() : "";
      if (!category || !label) throw new Error(`a row (${label || "no label"}) carries no category`);
      rows.push({ category, label, cents: rowCents(tr, columns, label) });
    }
  }
  return { rows, columns };
}

/**
 * In first-appearance order; null where no row prints in a column.
 * @param {Row[]} rows
 * @param {number} columns
 * @returns {{id: string, cents: (number | null)[], members: string[]}[]}
 */
export function groupRows(rows, columns) {
  /** @type {Map<string, {id: string, cents: (number | null)[], members: string[]}>} */
  const by = new Map();
  for (const r of rows) {
    let g = by.get(r.category);
    if (!g) {
      g = { id: r.category, cents: new Array(columns).fill(null), members: [r.category] };
      by.set(r.category, g);
    }
    const into = g;
    r.cents.forEach((c, i) => { into.cents[i] = plus(into.cents[i], c); });
  }
  return [...by.values()];
}

/**
 * Ranks by the last column, folding past NAMED unless the tail would hold
 * one. The rank holds in every column, so a colour follows the category.
 * @param {{id: string, cents: (number | null)[], members: string[]}[]} groups
 * @returns {Band[]}
 */
export function rank(groups) {
  // An absent last cell ranks with a printed zero, at the bottom; the order
  // is ours and states no figure.
  const last = (/** @type {{cents: (number | null)[]}} */ g) => Math.abs(g.cents[g.cents.length - 1] ?? 0);
  const sorted = [...groups].sort((a, b) => last(b) - last(a) || a.id.localeCompare(b.id));
  if (sorted.length <= NAMED + 1) {
    return sorted.map((g, i) => ({ id: g.id, label: g.id, cents: g.cents, members: g.members, rank: i + 1 }));
  }
  const named = sorted.slice(0, NAMED).map((g, i) => ({ id: g.id, label: g.id, cents: g.cents, members: g.members, rank: i + 1 }));
  const rest = sorted.slice(NAMED);
  const columns = sorted[0].cents.length;
  /** @type {(number | null)[]} */
  const tail = new Array(columns).fill(null);
  for (const g of rest) g.cents.forEach((c, i) => { tail[i] = plus(tail[i], c); });
  named.push({ id: "", label: TAIL_LABEL, cents: tail, members: rest.map((g) => g.id), rank: 0 });
  return named;
}

/**
 * Label and `span.basis` apart: typeset separately, never joined into one word.
 * @param {Element} th
 * @returns {{label: string, basis: string}}
 */
export function headWords(th) {
  const basis = th.querySelector(".basis");
  const label = Array.from(th.childNodes)
    .filter((n) => n.nodeType === 3)
    .map((n) => n.textContent || "")
    .join(" ").trim().replace(/\s+/g, " ");
  return { label, basis: basis ? (basis.textContent || "").trim() : "" };
}

/**
 * "$149.5M".
 * @param {number} cents
 */
export function millions(cents) {
  return (cents < 0 ? "-$" : "$") + (Math.abs(cents) / 100_000_000).toFixed(1) + "M";
}

/**
 * Signed column totals (null where no band prints) and the tallest positive
 * or negative stack.
 * @param {Band[]} bands
 * @param {number} columns
 */
export function totals(bands, columns) {
  /** @type {(number | null)[]} */
  const sums = new Array(columns).fill(null);
  let tallest = 0;
  for (let i = 0; i < columns; i++) {
    let above = 0;
    let below = 0;
    for (const b of bands) {
      const c = b.cents[i];
      if (c === null) continue;
      sums[i] = plus(sums[i], c);
      if (c >= 0) above += c;
      else below -= c;
    }
    tallest = Math.max(tallest, above + below);
  }
  return { sums, tallest };
}

/**
 * Positive bands above a baseline, negative below. A band's tip links its
 * cells' pages via the table's `a[data-page]` and `data-pdf`.
 * @param {HTMLElement} section
 * @param {ReturnType<typeof tipper>} [tips]
 * @returns {{bands: Band[], sums: (number | null)[]}}
 */
export function draw(section, tips = tipper(section.ownerDocument)) {
  const doc = section.ownerDocument;
  const table = section.querySelector("table");
  const figure = section.querySelector("figure.change-chart");
  const holder = figure && figure.querySelector(".change-columns");
  const legend = figure && figure.querySelector(".change-legend");
  const caption = figure && figure.querySelector(".change-sum");
  if (!(table instanceof doc.defaultView.HTMLTableElement) || !figure || !holder || !legend || !caption) {
    throw new Error("a change figure has no table, or no chart with columns, a legend and a caption to fill");
  }
  const { rows, columns } = readRows(table);
  const bands = rank(groupRows(rows, columns));
  const { sums, tallest } = totals(bands, columns);
  const heads = Array.from(table.querySelectorAll("thead th[data-column]"));
  /** @param {Band} b @param {number} i @returns {import("./format.js").CitedPage[]} */
  const pagesOf = (b, i) => Array.from(table.tBodies).flatMap((body) => Array.from(body.rows))
    .filter((tr) => b.members.includes(tr.dataset.category || ""))
    .flatMap((tr) => Array.from(tr.querySelectorAll(`td[data-column="${i}"] a[data-page]`)).map(linkedPage));
  // Rounded once, here, so the stack's height is the sum of the heights drawn.
  const px = (/** @type {number} */ cents) =>
    Math.round(100 * Math.max(LEAST, tallest ? (HEIGHT * Math.abs(cents)) / tallest : 0)) / 100;
  // As tall as the tallest stack in any column, so flexbox shrinks no band
  // off the one scale.
  let room = 0;
  for (let i = 0; i < heads.length; i++) {
    const up = /** @type {number[]} */ (bands.map((b) => b.cents[i]).filter((c) => c !== null && c > 0));
    room = Math.max(room, up.reduce((a, c) => a + px(c), 0) + GAP * Math.max(up.length - 1, 0));
  }

  holder.replaceChildren();
  heads.forEach((th, i) => {
    const col = clone(doc, "change-column");
    col.dataset.column = String(i);
    const sum = sums[i];
    if (sum !== null) col.dataset.cents = String(sum);
    if (th.classList.contains("group-start")) col.dataset.new = "true";
    const above = col.querySelector(".change-above");
    const below = col.querySelector(".change-below");
    const total = col.querySelector(".change-total");
    const year = col.querySelector(".change-year");
    if (!above || !below || !total || !year) throw new Error("the column template lacks a stack, a total or a year");
    // One baseline across the columns: the stack above it is room tall in
    // every column, whatever hangs below, and the columns align at the top.
    above.style.height = (Math.ceil(100 * room) / 100) + "px";
    above.style.gap = below.style.gap = GAP + "px";
    // Largest nearest the baseline, so the baseline band is one category
    // across columns.
    const head = headWords(th);
    for (const b of bands) {
      const c = b.cents[i];
      if (!c) continue;
      const seg = clone(doc, "change-segment");
      seg.dataset.rank = String(b.rank);
      seg.dataset.cents = String(c);
      seg.style.height = px(c) + "px";
      const named = `${b.label}, ${head.label}${head.basis ? " " + head.basis : ""}`;
      seg.setAttribute("aria-label", `${named}: ${dollars(c)}`);
      pinnable(seg, tips, (tip, pinned) => {
        const meta = "◇ our sum of its printed rows" + (b.rank ? "" : `: ${b.members.join(", ")}`);
        tip.append(...tipHead(doc, named, dollars(c), meta), pageLinks(doc, pagesOf(b, i)), tipHint(doc, pinned));
      });
      if (c < 0) {
        seg.classList.add("negative");
        below.appendChild(seg);
      } else {
        above.prepend(seg);
      }
    }
    total.textContent = sum === null ? "prints nothing" : "◇ " + millions(sum);
    year.textContent = head.label;
    if (head.basis) {
      const basis = doc.createElement("span");
      basis.className = "basis";
      basis.textContent = head.basis;
      year.appendChild(basis);
    }
    holder.appendChild(col);
  });

  legend.replaceChildren();
  for (const b of bands) {
    const li = clone(doc, "change-entry");
    li.dataset.rank = String(b.rank);
    const name = li.querySelector(".change-name");
    const detail = li.querySelector(".change-detail");
    if (!name || !detail) throw new Error("the legend entry template lacks a name or a detail");
    name.textContent = b.label;
    const lastCents = b.cents[b.cents.length - 1];
    detail.textContent = (b.rank ? "" : b.members.join(", ") + "; ") +
      (lastCents === null ? "prints nothing in the last column" : `${dollars(lastCents)} in the last column`);
    legend.appendChild(li);
  }
  // A column none of the rows prints in has no sum, and its cap says so.
  const blank = heads.filter((_, i) => sums[i] === null).map((th) => {
    const head = headWords(th);
    return head.label + (head.basis ? " " + head.basis : "");
  });
  caption.textContent = `◇ Each column is our sum of what the ${rows.length} printed rows print in it, by kind, ` +
    "and its cap says that sum in millions" +
    (blank.length ? `, or says the column prints nothing where none of the rows prints in it (${blank.join(", ")})` : "") +
    "; a band below the baseline is a kind whose rows sum below zero " +
    "in that column, and it counts against the column.";
  figure.hidden = false;
  // The table was the chart's data; the chart now says what it held.
  const cells = section.querySelector(".change-cells");
  if (cells instanceof doc.defaultView.HTMLElement) cells.hidden = true;
  return { bands, sums };
}

/** @param {Document} doc */
export function boot(doc) {
  const tips = tipper(doc);
  for (const section of Array.from(doc.querySelectorAll('section.change-figure[data-chart="true"]'))) {
    // One figure that cannot draw says so in its own section, and the
    // others still draw.
    try {
      draw(/** @type {HTMLElement} */ (section), tips);
    } catch (err) {
      refuseFigure(/** @type {HTMLElement} */ (section), err);
    }
  }
}
