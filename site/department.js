// @ts-check
/**
 * fisc — the client module department.html loads. Importing it does nothing;
 * the template calls boot(). It reads the page's department config, fetches
 * the column the year control names, and fits one department's two blocks of
 * pp.85-125 to tables: the funds that pay it, and what it spends under each
 * object heading over every division the config files under it.
 *
 * Its arithmetic is sums of cited cents, unions of cited ids, and one row's
 * share of its side's sum. Whether a department's two sides tie is the
 * config's declaration, printed and never computed here.
 */

/** @typedef {{doc_id: string, pages: number[]}} Locator */
/** @typedef {{id: string, label: string, cents: number, factIds: string[], locators: Locator[]}} Row */

/**
 * The link kinds a spending row may declare, and whether each is a transfer
 * drawn as not an expenditure.
 * @type {Record<string, boolean>}
 */
const SPENDING_TRANSFER = { external: false, internal_transfer: true };

/** The hash's fields. */
const HASH_FIELDS = ["y", "d"];

/**
 * The page's config, read from its JSON script element.
 * @param {Document} doc
 */
export function readConfig(doc) {
  const el = doc.getElementById("department-config");
  if (!el) throw new Error("the page carries no department config");
  return JSON.parse(el.textContent || "");
}

/**
 * A view as a hash: `#y=<column file less .json>&d=<department id>`.
 * @param {{year: string, department: string}} st
 */
export function hashOf(st) {
  const esc = (/** @type {string} */ v) => encodeURIComponent(v).replace(/%2F/g, "/");
  return "#y=" + esc(st.year.replace(/\.json$/, "")) + "&d=" + esc(st.department);
}

/**
 * A hash read back into a view over the config: the year and department it
 * names, or the opening ones where it names none, with `refused` saying what
 * it named that the config does not offer.
 * @param {string} hash
 * @param {any} config
 */
export function parseHash(hash, config) {
  /** @type {Map<string, string>} */
  const fields = new Map();
  for (const part of String(hash || "").replace(/^#/, "").split("&")) {
    const eq = part.indexOf("=");
    if (eq < 0) continue;
    const key = part.slice(0, eq);
    if (!HASH_FIELDS.includes(key)) continue;
    try {
      fields.set(key, decodeURIComponent(part.slice(eq + 1)));
    } catch {
      fields.set(key, part.slice(eq + 1));
    }
  }
  const years = config.years.map((/** @type {any} */ y) => y.path);
  const departments = config.departments.map((/** @type {any} */ d) => d.id);
  const out = { year: years[years.length - 1], department: departments[0], refused: "" };
  const y = fields.get("y");
  if (y !== undefined) {
    if (years.includes(y + ".json")) out.year = y + ".json";
    else out.refused = `No budget column is called ${y}, so this shows the newest.`;
  }
  const d = fields.get("d");
  if (d !== undefined) {
    if (departments.includes(d)) out.department = d;
    else out.refused = (out.refused ? out.refused + " " : "") + `No department is called ${d}, so this shows the first.`;
  }
  return out;
}

/**
 * @param {Locator[]} a
 * @param {Locator[]} b
 * @returns {Locator[]}
 */
function unionLocators(a, b) {
  const out = a.map((l) => ({ doc_id: l.doc_id, pages: [...l.pages] }));
  for (const l of b) {
    const had = out.find((o) => o.doc_id === l.doc_id);
    if (!had) out.push({ doc_id: l.doc_id, pages: [...l.pages] });
    else for (const p of l.pages) if (!had.pages.includes(p)) had.pages.push(p);
  }
  for (const l of out) l.pages.sort((x, y) => x - y);
  return out;
}

/**
 * One row per distinct node at one end of the links, its cents summed and
 * its fact ids and locators unioned, largest first.
 * @param {any[]} links
 * @param {any[]} nodes
 * @param {"from" | "to"} end
 * @returns {Row[]}
 */
function rowsBy(links, nodes, end) {
  /** @type {Map<number, Row>} */
  const by = new Map();
  for (const l of links) {
    const i = l[end];
    let r = by.get(i);
    if (!r) {
      r = { id: nodes[i].id, label: nodes[i].label, cents: 0, factIds: [], locators: [] };
      by.set(i, r);
    }
    r.cents += l.value_cents;
    for (const id of l.fact_ids) if (!r.factIds.includes(id)) r.factIds.push(id);
    r.locators = unionLocators(r.locators, l.locators);
  }
  return [...by.values()].sort((a, b) => b.cents - a.cents || a.label.localeCompare(b.label));
}

/**
 * The sum of rows: their cents added and their ids unioned.
 * @param {{cents: number, factIds: string[]}[]} rows
 */
function sumOf(rows) {
  /** @type {string[]} */
  const factIds = [];
  let cents = 0;
  for (const r of rows) {
    cents += r.cents;
    for (const id of r.factIds) if (!factIds.includes(id)) factIds.push(id);
  }
  return { cents, factIds };
}

/**
 * One department's figures out of one column: the funding schedule's links
 * into the department, and the spending schedule's into each division the
 * config files under it, as rows of each side and each side's sum.
 * @param {any} column
 * @param {any} config
 * @param {string} departmentId
 */
export function departmentFigures(column, config, departmentId) {
  const dept = config.departments.find((/** @type {any} */ d) => d.id === departmentId);
  if (!dept) throw new Error(`the config offers no department ${departmentId}`);
  const funding = column.schedules[config.funding];
  const spending = column.schedules[config.spending];
  if (!funding || !spending) {
    throw new Error(`the column carries no ${funding ? config.spending : config.funding} schedule`);
  }
  const nodes = column.nodes;
  const fundLinks = funding.links.filter((/** @type {any} */ l) => nodes[l.to].id === dept.id);
  const divisions = dept.divisions.map((/** @type {any} */ d) => d.id);
  const spendLinks = spending.links.filter((/** @type {any} */ l) => divisions.includes(nodes[l.to].id));

  // A fund's group is the funding schedule's parent edge; its kind, the link's.
  /** @type {Map<number, string>} */
  const parentOf = new Map();
  for (const n of funding.nodes) if (n.parent) parentOf.set(n.node, n.parent);
  const labelOf = new Map(nodes.map((/** @type {any} */ n) => [n.id, n.label]));
  const fundRows = rowsBy(fundLinks, nodes, "from").map((r) => {
    const l = fundLinks.find((/** @type {any} */ x) => nodes[x.from].id === r.id);
    const group = parentOf.get(l.from) || "";
    return { ...r, kind: String(l.kind), group: String(labelOf.get(group) || group) };
  });
  // Whether a spending row is an expenditure is its links' kind, as Go
  // declares it; a kind the page does not name, or two on one row, is refused.
  const objectRows = rowsBy(spendLinks, nodes, "from").map((r) => {
    const kinds = new Set(spendLinks.filter((/** @type {any} */ l) => nodes[l.from].id === r.id)
      .map((/** @type {any} */ l) => l.kind));
    const [kind] = kinds;
    if (kinds.size !== 1 || !Object.hasOwn(SPENDING_TRANSFER, kind)) {
      throw new Error(`${config.spending} draws ${r.id} as ${[...kinds].map(String).join(" and ")}, ` +
        `and the page says a spending row is ${Object.keys(SPENDING_TRANSFER).join(" or ")}`);
    }
    return { ...r, transfer: SPENDING_TRANSFER[kind] };
  });
  const cells = spendLinks.map((/** @type {any} */ l) => ({
    division: String(nodes[l.to].id), divisionLabel: String(nodes[l.to].label),
    id: String(nodes[l.from].id), label: String(nodes[l.from].label),
    cents: Number(l.value_cents), factIds: [...l.fact_ids], locators: unionLocators([], l.locators),
  })).sort((/** @type {any} */ a, /** @type {any} */ b) =>
    divisions.indexOf(a.division) - divisions.indexOf(b.division) || b.cents - a.cents);
  return {
    department: dept,
    funding: { rows: fundRows, sum: sumOf(fundRows) },
    spending: { rows: objectRows, sum: sumOf(objectRows), cells },
  };
}

/**
 * Whole dollars, as the pages print them; cents only where a figure has them.
 * @param {number} cents
 */
export function dollars(cents) {
  const whole = cents % 100 === 0;
  return (cents < 0 ? "-$" : "$") + (Math.abs(cents) / 100).toLocaleString("en-US",
    { minimumFractionDigits: whole ? 0 : 2, maximumFractionDigits: 2 });
}

/**
 * One row's share of its side's sum, as our percentage.
 * @param {number} part
 * @param {number} whole
 */
export function share(part, whole) {
  return whole === 0 ? "" : (100 * part / whole).toFixed(1) + "%";
}

/**
 * The page-text link of one cited page, or undefined where the config has none.
 * @param {any} config
 * @param {string} docId
 * @param {number} page
 */
function pageText(config, docId, page) {
  const d = config.docs[docId];
  const p = d && d.pages[String(page)];
  return p && p.text ? String(p.text) : undefined;
}

/**
 * A link per page a row cites, each to that page's extracted text.
 * @param {Document} doc
 * @param {Locator[]} locators
 * @param {any} config
 */
function pageLinks(doc, locators, config) {
  const td = doc.createElement("td");
  for (const loc of locators) {
    for (const p of loc.pages) {
      if (td.childNodes.length) td.append(", ");
      const href = pageText(config, loc.doc_id, p);
      if (!href) {
        td.append(`p${p}`);
        continue;
      }
      const a = doc.createElement("a");
      a.href = href;
      a.title = `Read on page ${p}`;
      a.textContent = `p${p}`;
      td.append(a);
    }
  }
  return td;
}

/**
 * An amount cell carrying the cents and fact ids it shows, linked to the page
 * text of its first cited page.
 * @param {Document} doc
 * @param {{cents: number, factIds: string[], locators: Locator[]}} r
 * @param {any} config
 */
function amountCell(doc, r, config) {
  const td = doc.createElement("td");
  td.className = "num";
  td.dataset.cents = String(r.cents);
  td.dataset.factIds = r.factIds.join(" ");
  const first = r.locators[0];
  const href = first && pageText(config, first.doc_id, first.pages[0]);
  if (href) {
    const a = doc.createElement("a");
    a.href = href;
    a.title = `Read on page ${first.pages[0]}`;
    a.textContent = dollars(r.cents);
    td.append(a);
  } else {
    td.textContent = dollars(r.cents);
  }
  return td;
}

/**
 * @param {Document} doc
 * @param {string} text
 * @param {string} [className]
 */
function cell(doc, text, className) {
  const td = doc.createElement("td");
  if (className) td.className = className;
  td.textContent = text;
  return td;
}

/**
 * A row's heading cell.
 * @param {Document} doc
 * @param {string} text
 */
function rowHead(doc, text) {
  const th = doc.createElement("th");
  th.scope = "row";
  th.textContent = text;
  return th;
}

/**
 * The row closing a side: our sum of the rows above, never the department's
 * total.
 * @param {Document} doc
 * @param {{cents: number, factIds: string[]}} sum
 * @param {number} before cells before the amount, the heading among them
 * @param {number} after cells after it
 */
function sumRow(doc, sum, before, after) {
  const tr = doc.createElement("tr");
  tr.dataset.sum = "ours";
  tr.append(rowHead(doc, "Our sum of the rows above"));
  for (let i = 1; i < before; i++) tr.append(cell(doc, ""));
  const td = cell(doc, dollars(sum.cents), "num");
  td.dataset.cents = String(sum.cents);
  td.dataset.factIds = sum.factIds.join(" ");
  tr.append(td);
  for (let i = 0; i < after; i++) tr.append(cell(doc, ""));
  return tr;
}

/**
 * Draws one department of one column into the page, and returns its figures.
 * @param {Document} doc
 * @param {any} config
 * @param {any} column
 * @param {{year: string, department: string}} st
 */
export function render(doc, config, column, st) {
  const f = departmentFigures(column, config, st.department);
  const year = config.years.find((/** @type {any} */ y) => y.path === st.year);
  const heading = doc.getElementById("department-heading");
  if (heading) heading.textContent = `${f.department.label}, ${year ? year.lede : st.year}`;

  const tie = doc.getElementById("department-tie");
  const side = year && year.sides.find((/** @type {any} */ s) => s.department === st.department);
  if (tie) tie.textContent = !side ? "" : side.ties ? tie.dataset.ties || "" : (tie.dataset.held || "") + side.reason;

  const funding = doc.querySelector("#funding-table tbody");
  if (funding) {
    funding.replaceChildren();
    for (const r of f.funding.rows) {
      const tr = doc.createElement("tr");
      tr.dataset.id = r.id;
      const th = rowHead(doc, r.label);
      if (r.kind === "internal_service") {
        const em = doc.createElement("em");
        em.textContent = " (internal service)";
        th.append(em);
      }
      tr.append(th, cell(doc, r.group), amountCell(doc, r, config),
        cell(doc, share(r.cents, f.funding.sum.cents), "num"), pageLinks(doc, r.locators, config));
      funding.append(tr);
    }
    funding.append(sumRow(doc, f.funding.sum, 2, 2));
  }

  const spending = doc.querySelector("#spending-table tbody");
  if (spending) {
    spending.replaceChildren();
    for (const r of f.spending.rows) {
      const tr = doc.createElement("tr");
      tr.dataset.id = r.id;
      tr.append(rowHead(doc, r.label + (r.transfer ? " (not an expenditure)" : "")), amountCell(doc, r, config),
        cell(doc, share(r.cents, f.spending.sum.cents), "num"), pageLinks(doc, r.locators, config));
      spending.append(tr);
    }
    spending.append(sumRow(doc, f.spending.sum, 1, 2));
  }
  const transfers = doc.getElementById("transfers-says");
  if (transfers) transfers.hidden = !f.spending.rows.some((r) => r.transfer);

  const divisions = doc.querySelector("#division-table tbody");
  if (divisions) {
    divisions.replaceChildren();
    for (const c of f.spending.cells) {
      const tr = doc.createElement("tr");
      tr.dataset.id = c.division + " " + c.id;
      tr.append(rowHead(doc, c.divisionLabel), cell(doc, c.label), amountCell(doc, c, config),
        pageLinks(doc, c.locators, config));
      divisions.append(tr);
    }
  }

  const data = doc.getElementById("department-data-path");
  if (data) {
    data.setAttribute("href", st.year);
    data.textContent = st.year;
  }
  return f;
}

/**
 * Says a refusal at the top of the page, or clears it.
 * @param {Document} doc
 * @param {string} text
 */
function say(doc, text) {
  let el = doc.getElementById("department-refusal");
  if (!text) {
    if (el) el.remove();
    return;
  }
  if (!el) {
    el = doc.createElement("p");
    el.id = "department-refusal";
    el.className = "refusal";
    el.setAttribute("role", "alert");
    const main = doc.getElementById("department-main");
    if (main) main.prepend(el);
  }
  el.textContent = text;
}

/**
 * Boots the page: reads the config, opens the view the hash names, and wires
 * the two selects and Back and Forward. The template calls this; it resolves
 * once the opening view is drawn or refused.
 * @param {any} [win] the window, passed in so a test can hand it its own
 */
export async function boot(win = globalThis.window) {
  const doc = win.document;
  /** @type {any} */
  let config;
  try {
    config = readConfig(doc);
  } catch (e) {
    say(doc, "This page could not read its config: " + String(e));
    return;
  }
  const deptSelect = /** @type {HTMLSelectElement | null} */ (doc.getElementById("department-select"));
  const yearSelect = /** @type {HTMLSelectElement | null} */ (doc.getElementById("year-select"));
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

  let shown = 0;
  /** @param {{year: string, department: string}} st */
  const show = async (st) => {
    const ticket = ++shown;
    if (deptSelect) deptSelect.value = st.department;
    if (yearSelect) yearSelect.value = st.year;
    try {
      const col = await column(st.year);
      if (ticket === shown) render(doc, config, col, st);
    } catch (e) {
      if (ticket === shown) say(doc, "This department could not be drawn: " + String(e));
    }
  };
  const fromControls = () => ({
    year: yearSelect ? yearSelect.value : config.years[config.years.length - 1].path,
    department: deptSelect ? deptSelect.value : config.departments[0].id,
  });
  const restore = async () => {
    const st = parseHash(win.location.hash, config);
    say(doc, st.refused);
    await show(st);
    const hash = hashOf(st);
    if (win.location.hash !== hash) win.history.replaceState(null, "", hash);
  };
  for (const el of [deptSelect, yearSelect]) {
    if (!el) continue;
    el.addEventListener("change", () => {
      say(doc, "");
      const st = fromControls();
      const hash = hashOf(st);
      if (win.location.hash !== hash) win.history.pushState(null, "", hash);
      void show(st);
    });
  }
  win.addEventListener("popstate", () => void restore());
  await restore();
}
