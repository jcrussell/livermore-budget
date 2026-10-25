// @ts-check
/**
 * fisc — capital.html's module; the template calls boot(). Filters the
 * server-rendered rows by fund, name and year and sums those shown: ours,
 * not the listing's printed TOTAL, and the footer says so.
 */

import { dollars, hashFields, hashValue, plus, rowCents, selectYear, wireYearSelect } from "./format.js";

/**
 * @typedef {{ fund: string, column: string, query: string }} Filter
 *   "" fund or column means every one.
 * @typedef {{ el: HTMLTableRowElement, fund: string, project: string,
 *   cents: (number | null)[] }} Row
 *   cents: one per column, null where the row prints none.
 */

/**
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
      rows.push({
        el,
        fund: el.dataset.fund || "",
        project: head ? (head.textContent || "").trim() : "",
        cents: rowCents(el, columns, `a row of fund ${el.dataset.fund}`),
      });
    }
  }
  return { rows, columns };
}

/**
 * The year filter hides columns, never rows.
 * @param {{ fund: string, project: string }} row
 * @param {Filter} filter
 */
export function isShown(row, filter) {
  if (filter.fund !== "" && row.fund !== filter.fund) return false;
  const q = filter.query.trim().toLowerCase();
  return q === "" || row.project.toLowerCase().includes(q);
}

/**
 * Null, not zero, where no shown row prints in a column.
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
      out[i] = plus(out[i], c);
    });
  }
  return out;
}

/**
 * @param {HTMLTableElement} table
 * @param {HTMLElement | null} status
 * @param {Filter} filter
 */
export function apply(table, status, filter) {
  const { rows, columns } = readTable(table);
  let shown = 0;
  // data-fund-repeat (blanked by style.css) names a fund once per run of rows.
  let above = "";
  for (const row of rows) {
    const on = isShown(row, filter);
    row.el.hidden = !on;
    const fund = row.el.getAttribute("data-fund") || "";
    row.el.toggleAttribute("data-fund-repeat", on && fund === above);
    if (on) {
      shown++;
      above = fund;
    }
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
 * "" for every column, including for an unknown key.
 * @param {HTMLTableElement} table
 * @param {string} key
 */
export function columnOf(table, key) {
  if (!key || !table.tHead) return "";
  for (const th of Array.from(table.tHead.querySelectorAll("th[data-year]"))) {
    if (th.getAttribute("data-year") === key) return th.getAttribute("data-column") || "";
  }
  return "";
}

/**
 * The year opens on `#y=` and a choice is written back.
 * @param {Document} doc
 */
export function boot(doc) {
  const table = /** @type {HTMLTableElement | null} */ (doc.getElementById("capital-table"));
  const form = doc.getElementById("capital-filters");
  const fund = /** @type {HTMLSelectElement | null} */ (doc.getElementById("capital-fund"));
  const search = /** @type {HTMLInputElement | null} */ (doc.getElementById("capital-search"));
  if (!table || !form || !fund || !search) {
    throw new Error("capital.html is missing its table or one of its controls");
  }
  const status = doc.getElementById("capital-shown");
  let year = "";
  const update = () => apply(table, status, { fund: fund.value, column: columnOf(table, year), query: search.value });
  /** @param {string} key */
  const choose = (key) => {
    year = columnOf(table, key) === "" ? "" : key;
    selectYear(doc, year);
    update();
    const win = doc.defaultView;
    const hash = year ? "#y=" + hashValue(year) : "";
    if (win && win.location.hash !== hash) {
      try {
        win.history.replaceState(null, "", hash || win.location.pathname + win.location.search);
      } catch { /* a sandboxed frame refuses history; the table still narrows */ }
    }
  };
  form.addEventListener("submit", (e) => e.preventDefault());
  fund.addEventListener("change", update);
  search.addEventListener("input", update);
  const restored = wireYearSelect(doc, choose);
  form.hidden = false;
  choose(hashFields(doc.location.hash, ["y"]).get("y") ?? restored ?? "");
}
