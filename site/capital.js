// @ts-check
/**
 * fisc — the client module capital.html loads. The table is server-rendered
 * with every figure Go cited; this module filters its rows by fund and by
 * project name, narrows it to one year, and sums the rows left shown. The sum
 * is ours, not the listing's printed TOTAL, and the template's footer labels
 * it so. Importing it runs nothing; the template calls boot().
 */

import { dollars } from "./format.js";

/**
 * @typedef {{ fund: string, column: string, query: string }} Filter
 *   fund is a fund number or "" for every fund, column a column index or ""
 *   for every year, query the text a project's name must contain.
 * @typedef {{ el: HTMLTableRowElement, fund: string, project: string,
 *   cents: (number | null)[] }} Row
 *   cents is one entry per column, null where the row prints no figure.
 */

/**
 * Reads the table Go rendered into rows.
 * @param {HTMLTableElement} table
 * @returns {{ rows: Row[], columns: number }}
 */
export function readTable(table) {
  const columns = table.tHead ? table.tHead.querySelectorAll("th[data-column]").length : 0;
  /** @type {Row[]} */
  const rows = [];
  for (const body of Array.from(table.tBodies)) {
    for (const el of Array.from(body.rows)) {
      const head = el.querySelector("th[scope=row]");
      /** @type {(number | null)[]} */
      const cents = new Array(columns).fill(null);
      for (const td of Array.from(el.querySelectorAll("td[data-column]"))) {
        const at = Number(td.getAttribute("data-column"));
        const raw = td.getAttribute("data-cents");
        if (raw === null) continue;
        const v = Number(raw);
        if (!Number.isSafeInteger(v) || !(at >= 0 && at < columns)) {
          throw new Error(`a cell of fund ${el.dataset.fund} carries ${raw} cents in column ${at}`);
        }
        cents[at] = v;
      }
      rows.push({
        el,
        fund: el.dataset.fund || "",
        project: head ? (head.textContent || "").trim() : "",
        cents,
      });
    }
  }
  return { rows, columns };
}

/**
 * Whether a row passes the fund and name filters. The year filter hides
 * columns, never rows.
 * @param {{ fund: string, project: string }} row
 * @param {Filter} filter
 */
export function isShown(row, filter) {
  if (filter.fund !== "" && row.fund !== filter.fund) return false;
  const q = filter.query.trim().toLowerCase();
  return q === "" || row.project.toLowerCase().includes(q);
}

/**
 * The cents of the shown rows, summed per column. A column no shown row
 * prints a figure in sums to null rather than to zero.
 * @param {Row[]} rows
 * @param {number} columns
 * @param {Filter} filter
 * @returns {(number | null)[]}
 */
export function sumShown(rows, columns, filter) {
  /** @type {(number | null)[]} */
  const out = new Array(columns).fill(null);
  for (const row of rows) {
    if (!isShown(row, filter)) continue;
    row.cents.forEach((c, i) => {
      if (c !== null) out[i] = (out[i] ?? 0) + c;
    });
  }
  return out;
}

/**
 * Applies a filter to the table: hides the rows and columns it excludes,
 * writes the sum of the rows shown into the footer and the count into status.
 * @param {HTMLTableElement} table
 * @param {HTMLElement | null} status
 * @param {Filter} filter
 */
export function apply(table, status, filter) {
  const { rows, columns } = readTable(table);
  let shown = 0;
  for (const row of rows) {
    const on = isShown(row, filter);
    row.el.hidden = !on;
    if (on) shown++;
  }
  for (const cell of Array.from(table.querySelectorAll("[data-column]"))) {
    const el = /** @type {HTMLElement} */ (cell);
    el.hidden = filter.column !== "" && el.getAttribute("data-column") !== filter.column;
  }
  const sums = sumShown(rows, columns, filter);
  const foot = table.tFoot;
  if (foot) {
    for (const td of Array.from(foot.querySelectorAll("td[data-column]"))) {
      const s = sums[Number(td.getAttribute("data-column"))];
      td.textContent = s === null || s === undefined ? "—" : dollars(s);
    }
    foot.hidden = shown === 0;
  }
  if (status) {
    status.textContent = `◇ ${shown} of ${rows.length} project lines shown, by our count`;
  }
}

/**
 * Binds the page's controls. The template calls this with its document.
 * @param {Document} doc
 */
export function boot(doc) {
  const table = /** @type {HTMLTableElement | null} */ (doc.getElementById("capital-table"));
  const form = doc.getElementById("capital-filters");
  const fund = /** @type {HTMLSelectElement | null} */ (doc.getElementById("capital-fund"));
  const year = /** @type {HTMLSelectElement | null} */ (doc.getElementById("capital-year"));
  const search = /** @type {HTMLInputElement | null} */ (doc.getElementById("capital-search"));
  if (!table || !form || !fund || !year || !search) {
    throw new Error("capital.html is missing its table or one of its controls");
  }
  const status = doc.getElementById("capital-shown");
  const update = () => apply(table, status, { fund: fund.value, column: year.value, query: search.value });
  form.addEventListener("submit", (e) => e.preventDefault());
  fund.addEventListener("change", update);
  year.addEventListener("change", update);
  search.addEventListener("input", update);
  form.hidden = false;
  update();
}
