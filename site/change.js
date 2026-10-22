// @ts-check
/**
 * fisc — the client module change.html loads. Each figure's table is
 * server-rendered with every printed point Go cited; this module groups the
 * rows by category, sums each category per column, and stacks the sums as
 * one column per printed year, every column on the figure's one scale. Every
 * sum is ours, marked ◇; what a column holds is in its table. Importing it
 * runs nothing; the template calls boot().
 */

import { dollars, linkedPage, pageLinks, pinnable, tipHint, tipper } from "./format.js";

/** How many categories a figure names before the rest fold into one tail. */
export const NAMED = 5;

/** What the folded tail is called. */
export const TAIL_LABEL = "Everything else";

/** How tall the tallest column's bands are drawn, in px, gaps aside. */
export const HEIGHT = 240;

/** The px between two stacked bands, and the least a band is drawn. */
export const GAP = 2;
export const LEAST = 1;

/**
 * @typedef {{category: string, label: string, cents: (number | null)[]}} Row
 *   cents is one entry per column, null where the row prints no figure.
 * @typedef {{id: string, label: string, cents: (number | null)[], members: string[], rank: number}} Band
 *   One drawn category, or the tail: its cents per column, null where none
 *   of its rows prints a figure there, the categories
 *   folded into it (itself alone where named), and its rank by the last
 *   column, 1..NAMED+1 (rank names a sixth rather than fold it alone), or 0
 *   for the tail.
 */

/**
 * Reads the rows Go rendered. Refuses a row with no category or a figure
 * that is not an integer: a cell the page cannot place is not stacked.
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
      /** @type {(number | null)[]} */
      const cents = new Array(columns).fill(null);
      for (const td of Array.from(tr.querySelectorAll("td[data-column]"))) {
        const at = Number(td.getAttribute("data-column"));
        const raw = td.getAttribute("data-cents");
        if (raw === null) continue;
        // Number("") is 0, and an empty attribute is not a printed zero.
        const v = /^-?\d+$/.test(raw) ? Number(raw) : NaN;
        if (!Number.isSafeInteger(v) || !(at >= 0 && at < columns)) {
          throw new Error(`${label} carries ${JSON.stringify(raw)} cents in column ${at}`);
        }
        cents[at] = v;
      }
      rows.push({ category, label, cents });
    }
  }
  return { rows, columns };
}

/**
 * Adds a cell into a running sum, absent into absent staying absent: a sum
 * is null until a printed figure, a printed zero included, reaches it.
 * @param {number | null} sum
 * @param {number | null} c
 * @returns {number | null}
 */
function plus(sum, c) {
  return c === null ? sum : (sum ?? 0) + c;
}

/**
 * Sums the rows by category per column, in order of first appearance; a
 * category none of whose rows prints a figure in a column sums to null there.
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
 * Ranks the categories by their size in the last column, largest first, and
 * folds every one past the NAMED largest into one tail, unless that tail
 * would hold a single category. The rank is the category's and holds in
 * every column, so a colour follows the category and never its place in a
 * column.
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
 * A column header's words, apart: the year label Go printed as the header's
 * text, and the basis it printed in the header's `span.basis`, or "" where
 * it printed none. The two are typeset separately, so they are never joined
 * into one word.
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
 * A figure in millions to one place, for a column's cap: "$149.5M".
 * @param {number} cents
 */
export function millions(cents) {
  return (cents < 0 ? "-$" : "$") + (Math.abs(cents) / 100_000_000).toFixed(1) + "M";
}

/**
 * The column totals of the bands, signed, null for a column no band prints
 * in, and the tallest stack: the largest over any column of the positive
 * bands' sum and the negative bands' sum.
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
 * Fills one figure's chart from its table: one column per printed column,
 * positive bands stacked above a baseline and negative ones hung below, the
 * legend naming each band, and the sentence stating the sums as ours. Each
 * band's tip pins on a click and links the pages its cells in that column
 * cite, each to the page text the table's own link names (`a[data-page]`)
 * and to the PDF page its `data-pdf` names.
 * @param {HTMLElement} section
 * @param {ReturnType<typeof tipper>} [tips] the page's tooltip
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
  const clone = (/** @type {string} */ id) => {
    const tmpl = doc.getElementById(id);
    if (!(tmpl instanceof doc.defaultView.HTMLTemplateElement) || !tmpl.content.firstElementChild) {
      throw new Error(`the page carries no <template id="${id}"> to clone`);
    }
    return /** @type {HTMLElement} */ (tmpl.content.firstElementChild.cloneNode(true));
  };
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
  // The stack above the baseline is as tall as the tallest one drawn there,
  // gaps and least heights included, in every column: bands that overflow it
  // are shrunk by the flexbox and leave the one scale.
  let room = 0;
  for (let i = 0; i < heads.length; i++) {
    const up = /** @type {number[]} */ (bands.map((b) => b.cents[i]).filter((c) => c !== null && c > 0));
    room = Math.max(room, up.reduce((a, c) => a + px(c), 0) + GAP * Math.max(up.length - 1, 0));
  }

  holder.replaceChildren();
  heads.forEach((th, i) => {
    const col = clone("change-column");
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
    // Largest nearest the baseline on either side, so the baseline band is
    // the same category in every column and the eye reads one band across
    // them: bands are ranked largest first, prepended above the baseline
    // and appended below it.
    const head = headWords(th);
    for (const b of bands) {
      const c = b.cents[i];
      if (!c) continue;
      const seg = clone("change-segment");
      seg.dataset.rank = String(b.rank);
      seg.dataset.cents = String(c);
      seg.style.height = px(c) + "px";
      const named = `${b.label}, ${head.label}${head.basis ? " " + head.basis : ""}`;
      seg.setAttribute("aria-label", `${named}: ${dollars(c)}`);
      pinnable(seg, tips, (tip, pinned) => {
        const label = doc.createElement("div");
        label.className = "tip-label";
        label.textContent = named;
        const value = doc.createElement("div");
        value.className = "tip-value";
        value.textContent = dollars(c);
        const meta = doc.createElement("div");
        meta.className = "tip-meta";
        meta.textContent = "◇ our sum of its printed rows" + (b.rank ? "" : `: ${b.members.join(", ")}`);
        tip.append(label, value, meta, pageLinks(doc, pagesOf(b, i)), tipHint(doc, pinned));
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
    const li = clone("change-entry");
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

/**
 * Draws every charted figure on the page.
 * @param {Document} doc
 */
export function boot(doc) {
  const tips = tipper(doc);
  for (const section of Array.from(doc.querySelectorAll('section.change-figure[data-chart="true"]'))) {
    draw(/** @type {HTMLElement} */ (section), tips);
  }
}
