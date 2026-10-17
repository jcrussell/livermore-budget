// @ts-check
/**
 * fisc — the client module change.html loads. Each figure's table is
 * server-rendered with every printed point Go cited; this module groups the
 * rows by category, sums each category per column, and stacks the sums as
 * one column per printed year, every column on the figure's one scale. Every
 * sum is ours, marked ◇; what a column holds is in its table. Importing it
 * runs nothing; the template calls boot().
 */

/** How many categories a figure names before the rest fold into one tail. */
export const NAMED = 5;

/** What the folded tail is called. */
export const TAIL_LABEL = "Everything else";

/** How tall the tallest column is drawn, in px. */
export const HEIGHT = 240;

/**
 * @typedef {{category: string, label: string, cents: (number | null)[]}} Row
 *   cents is one entry per column, null where the row prints no figure.
 * @typedef {{id: string, label: string, cents: number[], members: string[], rank: number}} Band
 *   One drawn category, or the tail: its cents per column, the categories
 *   folded into it (itself alone where named), and its rank by the last
 *   column, 1..NAMED, or 0 for the tail.
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
        const v = Number(raw);
        if (!Number.isSafeInteger(v) || !(at >= 0 && at < columns)) {
          throw new Error(`${label} carries ${raw} cents in column ${at}`);
        }
        cents[at] = v;
      }
      rows.push({ category, label, cents });
    }
  }
  return { rows, columns };
}

/**
 * Sums the rows by category per column, in order of first appearance.
 * @param {Row[]} rows
 * @param {number} columns
 * @returns {{id: string, cents: number[], members: string[]}[]}
 */
export function groupRows(rows, columns) {
  /** @type {Map<string, {id: string, cents: number[], members: string[]}>} */
  const by = new Map();
  for (const r of rows) {
    let g = by.get(r.category);
    if (!g) {
      g = { id: r.category, cents: new Array(columns).fill(0), members: [r.category] };
      by.set(r.category, g);
    }
    r.cents.forEach((c, i) => { if (c !== null) g.cents[i] += c; });
  }
  return [...by.values()];
}

/**
 * Ranks the categories by their size in the last column, largest first, and
 * folds every one past the NAMED largest into one tail, unless that tail
 * would hold a single category. The rank is the category's and holds in
 * every column, so a colour follows the category and never its place in a
 * column.
 * @param {{id: string, cents: number[], members: string[]}[]} groups
 * @returns {Band[]}
 */
export function rank(groups) {
  const last = (/** @type {{cents: number[]}} */ g) => Math.abs(g.cents[g.cents.length - 1] || 0);
  const sorted = [...groups].sort((a, b) => last(b) - last(a) || a.id.localeCompare(b.id));
  if (sorted.length <= NAMED + 1) {
    return sorted.map((g, i) => ({ id: g.id, label: g.id, cents: g.cents, members: g.members, rank: i + 1 }));
  }
  const named = sorted.slice(0, NAMED).map((g, i) => ({ id: g.id, label: g.id, cents: g.cents, members: g.members, rank: i + 1 }));
  const rest = sorted.slice(NAMED);
  const columns = sorted[0].cents.length;
  const tail = new Array(columns).fill(0);
  for (const g of rest) g.cents.forEach((c, i) => { tail[i] += c; });
  named.push({ id: "", label: TAIL_LABEL, cents: tail, members: rest.map((g) => g.id), rank: 0 });
  return named;
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
 * A figure in millions to one place, for a column's cap: "$149.5M".
 * @param {number} cents
 */
export function millions(cents) {
  return (cents < 0 ? "-$" : "$") + (Math.abs(cents) / 100_000_000).toFixed(1) + "M";
}

/**
 * The column totals of the bands, signed, and the tallest stack: the largest
 * over any column of the positive bands' sum and the negative bands' sum.
 * @param {Band[]} bands
 * @param {number} columns
 */
export function totals(bands, columns) {
  const sums = new Array(columns).fill(0);
  let tallest = 0;
  for (let i = 0; i < columns; i++) {
    let above = 0;
    let below = 0;
    for (const b of bands) {
      const c = b.cents[i];
      sums[i] += c;
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
 * legend naming each band, and the sentence stating the sums as ours.
 * @param {HTMLElement} section
 * @returns {{bands: Band[], sums: number[]}}
 */
export function draw(section) {
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
  const px = (/** @type {number} */ cents) => (tallest ? (HEIGHT * Math.abs(cents)) / tallest : 0);

  holder.replaceChildren();
  heads.forEach((th, i) => {
    const col = clone("change-column");
    col.dataset.column = String(i);
    col.dataset.cents = String(sums[i]);
    if (th.classList.contains("group-start")) col.dataset.new = "true";
    const above = col.querySelector(".change-above");
    const below = col.querySelector(".change-below");
    const total = col.querySelector(".change-total");
    const year = col.querySelector(".change-year");
    if (!above || !below || !total || !year) throw new Error("the column template lacks a stack, a total or a year");
    // Largest at the bottom, so the baseline band is the same category in
    // every column and the eye reads one band across them.
    for (const b of [...bands].reverse()) {
      const c = b.cents[i];
      if (!c) continue;
      const seg = clone("change-segment");
      seg.dataset.rank = String(b.rank);
      seg.dataset.cents = String(c);
      seg.style.height = px(c).toFixed(2) + "px";
      seg.title = `${b.label}, ${(th.textContent || "").trim().replace(/\s+/g, " ")}: ${dollars(c)}`;
      if (c < 0) {
        seg.classList.add("negative");
        below.appendChild(seg);
      } else {
        above.prepend(seg);
      }
    }
    total.textContent = "◇ " + millions(sums[i]);
    year.textContent = (th.textContent || "").trim().replace(/\s+/g, " ");
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
    detail.textContent = (b.rank ? "" : b.members.join(", ") + "; ") + `${dollars(lastCents)} in the last column`;
    legend.appendChild(li);
  }
  caption.textContent = `◇ Each column is our sum of the ${rows.length} printed rows below it, by kind, ` +
    "and its cap says that sum in millions; a band below the baseline is a kind whose rows sum below zero " +
    "in that column, and it counts against the column.";
  figure.hidden = false;
  return { bands, sums };
}

/**
 * Draws every charted figure on the page.
 * @param {Document} doc
 */
export function boot(doc) {
  for (const section of Array.from(doc.querySelectorAll('section.change-figure[data-chart="true"]'))) {
    draw(/** @type {HTMLElement} */ (section));
  }
}
