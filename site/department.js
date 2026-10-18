// @ts-check
/**
 * fisc — the client module department.html loads. Importing it does nothing;
 * the template calls boot(). It reads the page's department config, fetches
 * the column the year control names, and draws every department as a bar of
 * its funding rows, split by the fund group each row's fund hangs from, and
 * the chosen department's two blocks of pp.85-125 as tables under the bars:
 * the funds that pay it, and what it spends under each object heading over
 * every division the config files under it.
 *
 * Its arithmetic is sums of cited cents, unions of cited ids, and one row's
 * share of its side's sum. A fund's group is the funding schedule's parent
 * edge, which Go vets; the groups are ranked and folded by glance.js's rule.
 * Whether a department's two sides tie is the config's declaration, printed
 * and never computed here.
 */

import { NAMED, rank } from "./glance.js";

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
 * names, or the newest year and no department where it names none, with
 * `refused` saying what it named that the config does not offer. No
 * department opens the column's longest bar, which only its column can say.
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
  const out = { year: years[years.length - 1], department: "", refused: "" };
  const y = fields.get("y");
  if (y !== undefined) {
    if (years.includes(y + ".json")) out.year = y + ".json";
    else out.refused = `No budget column is called ${y}, so this shows the newest.`;
  }
  const d = fields.get("d");
  if (d !== undefined) {
    if (departments.includes(d)) out.department = d;
    else out.refused = (out.refused ? out.refused + " " : "") + `No department is called ${d}, so this shows the longest.`;
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
 * @typedef {{id: string, label: string, cents: number, members: string[], rank: number}} Group
 *   One entry of the legend: a fund group, or the tail folding several, with
 *   its sum over every department; rank is 1..NAMED for a hue, 0 for none.
 * @typedef {{key: number, label: string, members: string[], rank: number, cents: number, factIds: string[]}} Segment
 *   One department's part paid by one legend entry; key is the entry's index.
 * @typedef {{id: string, label: string, cents: number, factIds: string[], segments: Segment[]}} Bar
 */

/**
 * Every department of one column as a bar: the funding schedule's links into
 * it, summed per fund group and the groups ranked by their sum over every
 * department, largest first, past NAMED folded into one tail as the glance
 * page folds. A fund the schedule hangs from no fund group it draws, and a
 * cell that is not a positive integer of cents, is refused rather than drawn
 * in no segment.
 * @param {any} column
 * @param {any} config
 * @returns {{groups: Group[], bars: Bar[]}}
 */
export function departmentBars(column, config) {
  const funding = column.schedules[config.funding];
  if (!funding) throw new Error(`the column carries no ${config.funding} schedule`);
  const nodes = column.nodes;
  const drawn = new Map(funding.nodes.map((/** @type {any} */ n) => [nodes[n.node].id, n.node]));
  /** @type {Map<number, number>} */
  const groupOf = new Map();
  for (const n of funding.nodes) {
    const g = n.parent === undefined ? undefined : drawn.get(n.parent);
    if (g !== undefined && nodes[g].role === "fund_group") groupOf.set(n.node, g);
  }
  /** @type {Map<number, {id: string, label: string, cents: number, members: string[]}>} */
  const sums = new Map();
  for (const l of funding.links) {
    const g = groupOf.get(l.from);
    if (g === undefined) {
      throw new Error(`${config.funding} draws ${nodes[l.from].id} under no fund group it draws`);
    }
    if (!Number.isSafeInteger(l.value_cents) || l.value_cents <= 0) {
      throw new Error(`${config.funding} draws ${nodes[l.from].id} -> ${nodes[l.to].id} as ${l.value_cents} cents`);
    }
    const s = sums.get(g);
    if (s) s.cents += l.value_cents;
    else sums.set(g, { id: String(nodes[g].id), label: String(nodes[g].label), cents: l.value_cents, members: [String(nodes[g].label)] });
  }
  const { segments } = rank([...sums.values()]);
  /** @type {Group[]} */
  const groups = segments.map((s, i) => ({ ...s, rank: s.id && i < NAMED ? i + 1 : 0 }));
  const named = new Map(groups.map((g, i) => [g.id, i]));
  const tail = groups.findIndex((g) => !g.id);
  /** @param {number} g */
  const keyOf = (g) => named.get(String(nodes[g].id)) ?? tail;

  /** @type {Bar[]} */
  const bars = config.departments.map((/** @type {any} */ d) => {
    /** @type {Map<number, Segment>} */
    const by = new Map();
    for (const l of funding.links) {
      if (nodes[l.to].id !== d.id) continue;
      const key = keyOf(/** @type {number} */ (groupOf.get(l.from)));
      let s = by.get(key);
      if (!s) {
        const g = groups[key];
        s = { key, label: g.label, members: [], rank: g.rank, cents: 0, factIds: [] };
        by.set(key, s);
      }
      const member = String(nodes[/** @type {number} */ (groupOf.get(l.from))].label);
      if (!s.members.includes(member)) s.members.push(member);
      s.cents += l.value_cents;
      for (const id of l.fact_ids) if (!s.factIds.includes(id)) s.factIds.push(id);
    }
    const segs = [...by.values()].sort((a, b) => a.key - b.key);
    const sum = sumOf(segs);
    return { id: String(d.id), label: String(d.label), cents: sum.cents, factIds: sum.factIds, segments: segs };
  });
  bars.sort((/** @type {Bar} */ a, /** @type {Bar} */ b) => b.cents - a.cents || a.label.localeCompare(b.label));
  return { groups, bars };
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
 * Clones one of the page's templates.
 * @param {Document} doc
 * @param {string} id
 * @returns {HTMLElement}
 */
function clone(doc, id) {
  const tmpl = doc.getElementById(id);
  const el = tmpl && "content" in tmpl ? /** @type {HTMLTemplateElement} */ (tmpl).content.firstElementChild : null;
  if (!el) throw new Error(`the page carries no <template id="${id}"> to clone`);
  return /** @type {HTMLElement} */ (el.cloneNode(true));
}

/**
 * One part of a cloned template, refused where the template lacks it.
 * @param {HTMLElement} el
 * @param {string} sel
 * @returns {HTMLElement}
 */
function part(el, sel) {
  const p = el.querySelector(sel);
  if (!p) throw new Error(`a department template has no ${sel}`);
  return /** @type {HTMLElement} */ (p);
}

/**
 * Draws one column's bars and legend into the page, every bar closed, and
 * returns what it drew. A bar's track is as long against the list as its
 * sum is against the longest's.
 * @param {Document} doc
 * @param {any} config
 * @param {any} column
 * @param {string} yearPath
 */
export function drawBars(doc, config, column, yearPath) {
  const drawn = departmentBars(column, config);
  const year = config.years.find((/** @type {any} */ y) => y.path === yearPath);
  const heading = doc.getElementById("bars-heading");
  if (heading) heading.textContent = `Every department, ${year ? year.lede : yearPath}`;
  const legend = doc.getElementById("department-legend");
  const list = doc.getElementById("department-list");
  if (!legend || !list) throw new Error("the page carries no department legend or list to draw into");
  legend.replaceChildren();
  for (const g of drawn.groups) {
    const li = clone(doc, "department-key");
    li.dataset.rank = String(g.rank);
    li.dataset.cents = String(g.cents);
    part(li, ".department-key-name").textContent = g.id ? g.label : `${g.label}: ${g.members.join(", ")}`;
    li.title = `${g.label}: ${dollars(g.cents)} across every department`;
    legend.append(li);
  }
  list.replaceChildren();
  const longest = drawn.bars.length ? drawn.bars[0].cents : 0;
  drawn.bars.forEach((b, i) => {
    const li = clone(doc, "department-bar");
    li.dataset.department = b.id;
    const row = part(li, ".department-row");
    const detail = part(li, ".department-detail");
    detail.id = `department-detail-${i}`;
    row.setAttribute("aria-controls", detail.id);
    part(li, ".department-name").textContent = b.label;
    const value = part(li, ".department-value");
    value.textContent = dollars(b.cents);
    value.dataset.cents = String(b.cents);
    value.dataset.factIds = b.factIds.join(" ");
    const track = part(li, ".department-track");
    track.style.width = `${(100 * b.cents / longest).toFixed(2)}%`;
    const groups = part(li, ".department-groups");
    groups.setAttribute("aria-label", `Who pays for ${b.label}, by fund group`);
    for (const s of b.segments) {
      const named = drawn.groups[s.key].id ? s.label : `${s.label}: ${s.members.join(", ")}`;
      const seg = clone(doc, "department-seg");
      seg.dataset.rank = String(s.rank);
      seg.dataset.cents = String(s.cents);
      seg.dataset.factIds = s.factIds.join(" ");
      seg.style.flexGrow = String(s.cents);
      seg.title = `${named}: ${dollars(s.cents)}, ◇ our ${share(s.cents, b.cents)} of ${b.label}`;
      track.append(seg);

      const g = clone(doc, "department-group");
      g.dataset.rank = String(s.rank);
      part(g, ".department-group-name").textContent = named;
      const v = part(g, ".department-group-value");
      v.textContent = dollars(s.cents);
      v.dataset.cents = String(s.cents);
      v.dataset.factIds = s.factIds.join(" ");
      const sh = part(g, ".department-group-share");
      sh.textContent = `◇ ${share(s.cents, b.cents)}`;
      sh.title = `Our share: ${named}'s part of our sum for ${b.label}`;
      groups.append(g);
    }
    list.append(li);
  });
  return drawn;
}

/**
 * Opens one department's bar, or closes it where `open` is false, and closes
 * every other.
 * @param {Document} doc
 * @param {string} department
 * @param {boolean} open
 */
export function openBar(doc, department, open) {
  for (const li of Array.from(doc.querySelectorAll("#department-list > li"))) {
    const el = /** @type {HTMLElement} */ (li);
    const mine = el.dataset.department === department;
    const row = part(el, ".department-row");
    const detail = part(el, ".department-detail");
    row.setAttribute("aria-expanded", String(mine && open));
    detail.hidden = !(mine && open);
    if (mine) el.dataset.selected = "true";
    else delete el.dataset.selected;
  }
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

  // The page's one view: the year and the department the hash names, which
  // the selects and the open bar show. Closing the open bar hides its detail
  // and keeps the department; any other choice opens what it chooses.
  let shown = 0;
  let current = { year: "", department: "" };
  let barsOf = "";
  let longest = "";
  let open = true;
  /**
   * Shows a view, no department being the column's longest bar's, and
   * resolves to the view shown, or undefined where it was refused or overtaken.
   * @param {{year: string, department: string}} st
   */
  const show = async (st) => {
    const ticket = ++shown;
    current = { year: st.year, department: st.department };
    open = true;
    if (deptSelect && st.department) deptSelect.value = st.department;
    if (yearSelect) yearSelect.value = st.year;
    try {
      const col = await column(st.year);
      if (ticket !== shown) return undefined;
      if (barsOf !== st.year) {
        barsOf = "";
        const { bars } = drawBars(doc, config, col, st.year);
        longest = bars.length ? bars[0].id : config.departments[0].id;
        barsOf = st.year;
      }
      const view = { year: st.year, department: st.department || longest };
      current = view;
      if (deptSelect) deptSelect.value = view.department;
      render(doc, config, col, view);
      openBar(doc, view.department, open);
      return view;
    } catch (e) {
      if (ticket === shown) say(doc, "This department could not be drawn: " + String(e));
      return undefined;
    }
  };
  /** @param {{year: string, department: string}} st */
  const choose = (st) => {
    say(doc, "");
    const hash = hashOf(st);
    if (win.location.hash !== hash) win.history.pushState(null, "", hash);
    void show(st);
  };
  const list = doc.getElementById("department-list");
  /** @param {Event} e */
  const activate = (e) => {
    const target = /** @type {any} */ (e.target);
    const row = target && target.closest ? target.closest(".department-row") : null;
    const li = row && row.parentElement;
    if (!li || !li.dataset.department || li.parentElement !== list) return;
    if (li.dataset.department === current.department && barsOf === current.year) {
      open = !open;
      openBar(doc, current.department, open);
      return;
    }
    choose({ year: current.year, department: li.dataset.department });
  };
  if (list) {
    list.addEventListener("click", activate);
    list.addEventListener("keydown", (e) => {
      const ke = /** @type {KeyboardEvent} */ (e);
      if (ke.key !== "Enter" && ke.key !== " ") return;
      const target = /** @type {any} */ (ke.target);
      if (!target || !target.classList || !target.classList.contains("department-row")) return;
      ke.preventDefault();
      activate(ke);
    });
  }
  const fromControls = () => ({
    year: yearSelect ? yearSelect.value : config.years[config.years.length - 1].path,
    department: deptSelect ? deptSelect.value : config.departments[0].id,
  });
  const restore = async () => {
    const st = parseHash(win.location.hash, config);
    say(doc, st.refused);
    const view = await show(st);
    const hash = hashOf(view || { year: st.year, department: st.department || config.departments[0].id });
    if (win.location.hash !== hash) win.history.replaceState(null, "", hash);
  };
  for (const el of [deptSelect, yearSelect]) {
    if (!el) continue;
    el.addEventListener("change", () => choose(fromControls()));
  }
  win.addEventListener("popstate", () => void restore());
  await restore();
}
