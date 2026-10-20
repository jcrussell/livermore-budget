// @ts-check
/**
 * fisc — the client module department.html loads. Importing it does nothing;
 * the template calls boot(). It reads the page's department config, fetches
 * the column the year control names, and draws every department as a bar of
 * its funding rows, split by the fund group each row's fund hangs from, and
 * the chosen department's two blocks of pp.85-125 as two sides of bars
 * drawn by the ledger form: the funds that pay it, and what it spends under
 * each object heading over every division the config files under it.
 *
 * Its arithmetic is sums of cited cents, unions of cited ids, and one row's
 * share of its side's sum. A fund's group is the funding schedule's parent
 * edge, which Go vets; the groups are ranked and folded by glance.js's rule.
 * Whether a department's two sides tie is the config's declaration, printed
 * and never computed here.
 */

import * as core from "./core.js";
import { rank } from "./glance.js";
import { configOf, dollars, hashFields, hashValue, selectYear, share, tipper, wireYearSelect, yearKey } from "./format.js";
import { LEDGER } from "./ledger.js";
import { clone, fillTip, tileShare } from "./sides.js";

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
  return configOf(doc, "department-config", "department");
}

/**
 * A view as a hash: `#y=<column file less .json>&d=<department id>`, with no
 * `d` where it names no department, which parseHash reads as the longest bar.
 * @param {{year: string, department: string}} st
 */
export function hashOf(st) {
  return "#y=" + hashValue(yearKey(st.year)) + (st.department ? "&d=" + hashValue(st.department) : "");
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
  const fields = hashFields(hash, HASH_FIELDS);
  const years = config.years.map((/** @type {any} */ y) => y.path);
  const departments = config.departments.map((/** @type {any} */ d) => d.id);
  const out = { year: years[years.length - 1], department: "", refused: "" };
  const y = fields.get("y");
  if (y !== undefined) {
    const named = years.find((/** @type {string} */ p) => yearKey(p) === y);
    if (named) out.year = named;
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
    return { ...r, kind: String(l.kind), groupId: String(group), group: String(labelOf.get(group) || group) };
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
  return {
    department: dept,
    funding: { rows: fundRows, sum: sumOf(fundRows) },
    spending: { rows: objectRows, sum: sumOf(objectRows) },
  };
}

/**
 * @typedef {{id: string, label: string, cents: number, members: string[], memberIds: string[], rank: number}} Group
 *   One entry of the legend: a fund group, or the tail folding several, with
 *   its sum over every department; members are the labels it names and
 *   memberIds the fund-group ids it holds; rank is its place for a hue,
 *   1..NAMED+1 (rank names a sixth group rather than fold it alone), 0 for
 *   the tail.
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
  const namedIds = new Set(segments.map((s) => s.id).filter((id) => id));
  const folded = [...sums.values()].map((s) => s.id).filter((id) => !namedIds.has(id));
  /** @type {Group[]} */
  const groups = segments.map((s, i) => ({ ...s, memberIds: s.id ? [s.id] : folded, rank: s.id ? i + 1 : 0 }));
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
 * @typedef {{tier: number, heading: string, tree: {roots: core.SideTile[], cents: number, size: number}}} DeptSide
 */

/**
 * One row of a side as a flat tile LEDGER draws: no parent, nothing under
 * it and nothing to open, its figure and citations the row's.
 * @param {Row} r
 * @param {string} label
 * @returns {core.SideTile}
 */
function tileOf(r, label) {
  return {
    id: r.id, label, tier: 0, parent: "", derived: false, contra: "",
    cents: r.cents, size: Math.abs(r.cents), fact_ids: [...r.factIds],
    locators: r.locators.map((l) => ({ doc_id: l.doc_id, pages: [...l.pages] })),
    splits: [], open: "", children: null,
  };
}

/**
 * A department's two sides as LEDGER draws them: where its money comes from,
 * a tile per fund, and where it goes, a tile per object heading, each side's
 * figure the sum of its tiles. A fund the schedule calls internal service,
 * and a heading Go declares a transfer, say so in their label.
 * @param {ReturnType<typeof departmentFigures>} f
 * @returns {{funding: DeptSide, spending: DeptSide}}
 */
export function departmentSides(f) {
  /** @param {string} heading @param {core.SideTile[]} roots @returns {DeptSide} */
  const side = (heading, roots) => ({
    tier: 0, heading,
    tree: { roots, cents: roots.reduce((a, t) => a + t.cents, 0), size: roots.reduce((a, t) => a + t.size, 0) },
  });
  return {
    funding: side("Where its money comes from", f.funding.rows.map((r) =>
      tileOf(r, r.label + (r.kind === "internal_service" ? " (internal service)" : "")))),
    spending: side("Where its money goes", f.spending.rows.map((r) =>
      tileOf(r, r.label + (r.transfer ? " (not an expenditure)" : "")))),
  };
}

/**
 * Draws one department of one column into the page: its two sides through
 * LEDGER on one scale, the longest bar of either filling its track, each
 * side's ◇ sum under its heading. A fund's bar wears the hue of the legend
 * entry in `groups` holding its fund group's id, the tail's only where the
 * tail holds it; a bar no entry holds, and every spending bar, wears none,
 * and a transfer's is marked as not an expenditure. Returns the figures and the
 * sides drawn.
 * @param {Document} doc
 * @param {any} config
 * @param {any} column
 * @param {{year: string, department: string}} st
 * @param {Group[]} [groups] the legend departmentBars ranked over the column
 * @param {ReturnType<typeof tipper>} [tips] the page's tooltip
 */
export function render(doc, config, column, st, groups = [], tips = tipper(doc)) {
  const f = departmentFigures(column, config, st.department);
  const sides = departmentSides(f);
  const year = config.years.find((/** @type {any} */ y) => y.path === st.year);
  const heading = doc.getElementById("department-heading");
  if (heading) heading.textContent = `${f.department.label}, ${year ? year.lede : st.year}`;

  const tie = doc.getElementById("department-tie");
  const held = year && year.sides.find((/** @type {any} */ s) => s.department === st.department);
  if (tie) tie.textContent = !held ? "" : held.ties ? tie.dataset.ties || "" : (tie.dataset.held || "") + held.reason;

  /** @type {Map<string, string>} the tip's extra line, by tile id */
  const notes = new Map();
  /** @type {Map<string, number>} a fund bar's hue, by tile id */
  const hue = new Map();
  for (const r of f.funding.rows) {
    if (r.group) notes.set(r.id, `Fund group: ${r.group}`);
    const g = groups.find((x) => x.memberIds.includes(r.groupId));
    if (g) hue.set(r.id, g.rank);
  }
  const transfer = new Set(f.spending.rows.filter((r) => r.transfer).map((r) => r.id));
  for (const id of transfer) {
    notes.set(id, "Not an expenditure: money moved to another of the city's own funds. The page prints it inside the division's sum, so it is inside this side's.");
  }
  /**
   * A bar's tip: sides.js's, with the bar's fund group or transfer note
   * over its pages.
   * @param {core.SideTile} tile
   * @param {any} side
   */
  const fill = (tile, side) => (/** @type {HTMLElement} */ tip, /** @type {boolean} */ pinned) => {
    fillTip(doc, tip, tile, side, config.docs, pinned);
    const note = notes.get(tile.id);
    if (!note) return;
    const el = doc.createElement("div");
    el.className = "tip-meta";
    el.textContent = note;
    tip.insertBefore(el, tip.querySelector(".facts"));
  };
  const largest = Math.max(1, ...[sides.funding, sides.spending].flatMap((s) => s.tree.roots.map((r) => r.size)));
  /** @type {import("./sides.js").DrawContext} */
  const ctx = {
    doc,
    scale: 1 / largest,
    toggle: () => {},
    tip: (e, tile, side) => tips.show(e, fill(tile, side)),
    pin: (mark, tile, side, e) => tips.pin(mark, fill(tile, side), e),
    hide: () => tips.hide(),
    share: tileShare,
    clone: (id) => clone(doc, id),
  };
  for (const [key, side] of /** @type {[string, DeptSide][]} */ (Object.entries(sides))) {
    const section = /** @type {HTMLElement | null} */ (doc.querySelector(`section.department-side[data-side="${key}"]`));
    if (!section) throw new Error(`the page has no section for the ${key} side`);
    const total = section.querySelector(".sides-total");
    if (total) total.textContent = `◇ our sum of the ${side.tree.roots.length} bars on this side: ${dollars(side.tree.cents)}`;
    LEDGER.draw(section, /** @type {any} */ (side), ctx);
    for (const li of Array.from(section.querySelectorAll("li.bar-row"))) {
      const el = /** @type {HTMLElement} */ (li);
      const id = el.dataset.id || "";
      const rank = key === "funding" ? hue.get(id) : undefined;
      if (rank === undefined) delete el.dataset.rank;
      else el.dataset.rank = String(rank);
      if (key === "spending" && transfer.has(id)) el.dataset.transfer = "true";
    }
  }
  const says = doc.getElementById("transfers-says");
  if (says) says.hidden = transfer.size === 0;
  return { ...f, sides };
}

/**
 * Empties both sides, their sums and the heading over them, so no
 * department's bars stand under a year select naming another column.
 * @param {Document} doc
 */
function clearSides(doc) {
  for (const section of Array.from(doc.querySelectorAll("section.department-side"))) {
    const figure = /** @type {HTMLElement | null} */ (section.querySelector("figure.sides-figure"));
    if (figure) {
      figure.replaceChildren();
      figure.hidden = true;
    }
    const total = section.querySelector(".sides-total");
    if (total) total.textContent = "";
  }
  const heading = doc.getElementById("department-heading");
  if (heading) heading.textContent = "This department could not be drawn";
  const tie = doc.getElementById("department-tie");
  if (tie) tie.textContent = "";
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
      seg.style.width = `${100 * s.cents / b.cents}%`;
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
  // The view drawn, and the one asked for and not yet drawn or refused.
  let current = { year: "", department: "" };
  /** @type {{year: string, department: string} | null} */
  let asked = null;
  let barsOf = "";
  // The view whose sides are drawn, as hashOf names it: the open bar toggles
  // over its own department's sides only.
  let sidesOf = "";
  /** @type {Group[]} the legend of barsOf's column, whose hues a fund's bar wears */
  let groups = [];
  let longest = "";
  let open = true;
  const tips = tipper(doc);
  /**
   * Shows a view, no department being the column's longest bar's, and
   * resolves to the view shown, or undefined where it was refused or overtaken.
   * @param {{year: string, department: string}} st
   */
  const show = async (st) => {
    const ticket = ++shown;
    asked = { year: st.year, department: st.department };
    if (deptSelect && st.department) deptSelect.value = st.department;
    selectYear(doc, yearKey(st.year));
    try {
      const col = await column(st.year);
      if (ticket !== shown) return undefined;
      if (barsOf !== st.year) {
        barsOf = "";
        const drawn = drawBars(doc, config, col, st.year);
        const { bars } = drawn;
        groups = drawn.groups;
        longest = bars.length ? bars[0].id : config.departments[0].id;
        barsOf = st.year;
      }
      const view = { year: st.year, department: st.department || longest };
      if (deptSelect) deptSelect.value = view.department;
      sidesOf = "";
      tips.unpin(false);
      render(doc, config, col, view, groups, tips);
      sidesOf = hashOf(view);
      current = view;
      asked = null;
      open = true;
      openBar(doc, view.department, open);
      return view;
    } catch (e) {
      if (ticket !== shown) return undefined;
      asked = null;
      // Sides render refused are not left standing as another view's.
      if (!sidesOf) clearSides(doc);
      // The bars drawn are barsOf's year's, whichever view asked for them.
      if (barsOf) current = { year: barsOf, department: current.department };
      if (current.year) selectYear(doc, yearKey(current.year));
      if (deptSelect && current.department) deptSelect.value = current.department;
      say(doc, "This department could not be drawn: " + String(e));
      return undefined;
    }
  };
  /** @param {{year: string, department: string}} st */
  const choose = (st) => {
    say(doc, "");
    const hash = hashOf(st);
    if (win.location.hash !== hash) win.history.pushState(null, "", hash);
    void show(st).then((view) => {
      // A choice naming no department shows the longest bar, which the hash
      // names once its column has said which; a later choice keeps its own.
      if (view && win.location.hash === hash && hashOf(view) !== hash) win.history.replaceState(null, "", hashOf(view));
    });
  };
  const list = doc.getElementById("department-list");
  // The open bar's link to its sides scrolls by script: followed as a
  // fragment it would fire popstate, and restore() would read a hash naming
  // no view and open the newest year's longest bar in place of this one.
  /** @param {Event} e */
  const toSides = (e) => {
    const target = /** @type {any} */ (e.target);
    const link = target && target.closest ? target.closest(".department-to-sides") : null;
    if (!link || !list || !list.contains(link)) return false;
    e.preventDefault();
    const heading = doc.getElementById("department-heading");
    if (heading && typeof heading.scrollIntoView === "function") heading.scrollIntoView();
    return true;
  };
  /** @param {Event} e */
  const activate = (e) => {
    const target = /** @type {any} */ (e.target);
    const row = target && target.closest ? target.closest(".department-row") : null;
    const li = row && row.parentElement;
    if (!li || !li.dataset.department || li.parentElement !== list) return;
    if (li.dataset.department === current.department && barsOf === current.year && sidesOf === hashOf(current)) {
      open = !open;
      openBar(doc, current.department, open);
      return;
    }
    choose({ year: current.year, department: li.dataset.department });
  };
  if (list) {
    list.addEventListener("click", (e) => {
      if (!toSides(e)) activate(e);
    });
    list.addEventListener("keydown", (e) => {
      const ke = /** @type {KeyboardEvent} */ (e);
      if (ke.key !== "Enter" && ke.key !== " ") return;
      const target = /** @type {any} */ (ke.target);
      if (!target || !target.classList || !target.classList.contains("department-row")) return;
      ke.preventDefault();
      activate(ke);
    });
  }
  // A select names one half of the view and the other half is the view's
  // own: until the opening column resolves the department select shows the
  // config's first department, which nobody chose, so a year chosen then
  // keeps the view's department, "" where none was named: the longest bar.
  const asking = () => asked || current;
  const restore = async () => {
    const st = parseHash(win.location.hash, config);
    say(doc, st.refused);
    // A view refused or overtaken by a later choice is not on screen, so the
    // hash is left naming what the reader or that choice named.
    const view = await show(st);
    if (!view) return;
    const hash = hashOf(view);
    if (win.location.hash !== hash) win.history.replaceState(null, "", hash);
  };
  if (deptSelect) {
    deptSelect.addEventListener("change", () => choose({ year: asking().year, department: deptSelect.value }));
  }
  wireYearSelect(doc, (key) => {
    const year = config.years.find((/** @type {any} */ y) => yearKey(y.path) === key);
    if (year) choose({ year: year.path, department: asking().department });
  });
  win.addEventListener("popstate", () => void restore());
  await restore();
}
