// @ts-check
/**
 * fisc — the client module the glance page, index.html, loads. Each bar's table is
 * server-rendered with every cell Go cited; this module groups the rows,
 * sums each group, states each group's share of the sum as dollars of every
 * $100, and draws one bar per table. Every sum and share is ours, marked ◇;
 * the projection's total over every cell of the kind is beside the bar,
 * written by Go. Importing it runs nothing; the template calls boot().
 */

import { dollars, pageLinks, pinnable, share, tipHint, tipper } from "./format.js";

/** How many groups a bar names before the rest fold into one tail. */
export const NAMED = 5;

/** What the folded tail is called. */
export const TAIL_LABEL = "Everything else";

/**
 * @typedef {{ group: string, label: string, cents: number }} Row
 * @typedef {{ id: string, label: string, cents: number, members: string[] }} Segment
 *   members lists the labels folded into a tail; a named segment lists itself.
 */

/**
 * Reads the cells Go rendered into rows. Refuses a row with no group, no
 * label or a figure that is not an integer: a cell the page cannot name is
 * not drawn as part of a bar.
 * @param {HTMLTableElement} table
 * @returns {Row[]}
 */
export function readRows(table) {
  /** @type {Row[]} */
  const rows = [];
  for (const body of Array.from(table.tBodies)) {
    for (const tr of Array.from(body.rows)) {
      const group = tr.dataset.group || "";
      const head = tr.querySelector("th[scope=row]");
      const label = head ? (head.textContent || "").trim() : "";
      const cell = tr.querySelector("td[data-cents]");
      const raw = cell ? cell.getAttribute("data-cents") : null;
      const cents = raw === null ? NaN : Number(raw);
      if (!group || !label || !Number.isSafeInteger(cents) || cents <= 0) {
        throw new Error(`a row of ${group || "no group"} (${label || "no label"}) carries ${raw} cents`);
      }
      rows.push({ group, label, cents });
    }
  }
  return rows;
}

/**
 * Sums the rows by group, in order of first appearance.
 * @param {Row[]} rows
 * @returns {Segment[]}
 */
export function groupRows(rows) {
  /** @type {Map<string, Segment>} */
  const by = new Map();
  for (const r of rows) {
    const s = by.get(r.group);
    if (s) {
      if (s.label !== r.label) throw new Error(`${r.group} is labelled ${s.label} and ${r.label}`);
      s.cents += r.cents;
    } else {
      by.set(r.group, { id: r.group, label: r.label, cents: r.cents, members: [r.label] });
    }
  }
  return [...by.values()];
}

/**
 * Ranks the groups largest first and folds every group past the NAMED
 * largest into one tail, unless that tail would hold a single group, which
 * is named instead. The whole is the sum of every group, folded or not.
 * @param {Segment[]} groups
 * @returns {{ segments: Segment[], whole: number }}
 */
export function rank(groups) {
  const sorted = [...groups].sort((a, b) => b.cents - a.cents || a.label.localeCompare(b.label));
  const whole = sorted.reduce((sum, s) => sum + s.cents, 0);
  if (sorted.length <= NAMED + 1) return { segments: sorted, whole };
  const named = sorted.slice(0, NAMED);
  const rest = sorted.slice(NAMED);
  named.push({
    id: "",
    label: TAIL_LABEL,
    cents: rest.reduce((sum, s) => sum + s.cents, 0),
    members: rest.map((s) => s.label),
  });
  return { segments: named, whole };
}

/**
 * A part's share of the whole as dollars of every $100, to the cent.
 * @param {number} part
 * @param {number} whole
 */
export function perHundred(part, whole) {
  return "$" + (100 * part / whole).toFixed(2);
}

/**
 * Clones the page's template for one piece of the figure.
 * @param {Document} doc
 * @param {string} id
 * @returns {HTMLElement}
 */
function clone(doc, id) {
  const tmpl = doc.getElementById(id);
  if (!(tmpl instanceof doc.defaultView.HTMLTemplateElement)) {
    throw new Error(`the page carries no <template id="${id}"> to clone`);
  }
  const el = tmpl.content.firstElementChild;
  if (!el) throw new Error(`<template id="${id}"> is empty`);
  return /** @type {HTMLElement} */ (el.cloneNode(true));
}

/**
 * Fills one bar's figure from its table: the track of segments, the legend
 * naming each with its dollars of every $100, and the sentence stating the
 * sum as ours. The figure's markup is the template's; this fills it in.
 * Each segment's tip pins on a click and links the pages its rows cite,
 * each to the page text the table's own link names (`a[data-page]`).
 * Returns what it drew.
 * @param {HTMLElement} section
 * @param {ReturnType<typeof tipper>} [tips] the page's tooltip
 * @returns {{ segments: Segment[], whole: number, rows: number }}
 */
export function draw(section, tips = tipper(section.ownerDocument)) {
  const doc = section.ownerDocument;
  const table = section.querySelector("table");
  const figure = section.querySelector("figure.glance-figure");
  const track = figure && figure.querySelector(".glance-track");
  const legend = figure && figure.querySelector(".glance-legend");
  const caption = figure && figure.querySelector(".glance-sum");
  if (!(table instanceof doc.defaultView.HTMLTableElement) || !figure || !track || !legend || !caption) {
    throw new Error("a glance bar has no table, or no figure with a track, a legend and a caption to fill");
  }
  const rows = readRows(table);
  const groups = groupRows(rows);
  const { segments, whole } = rank(groups);
  const named = new Set(segments.filter((s) => s.id).map((s) => s.id));
  /** @param {Segment} s @returns {{page: number, href: string}[]} */
  const pagesOf = (s) => Array.from(table.tBodies).flatMap((b) => Array.from(b.rows))
    .filter((tr) => (s.id ? tr.dataset.group === s.id : !named.has(tr.dataset.group || "")))
    .flatMap((tr) => Array.from(tr.querySelectorAll("a[data-page]")).map((a) => ({
      page: Number(a.getAttribute("data-page")), href: a.getAttribute("href") || "",
    })));
  const heading = section.querySelector("h3");
  const title = heading ? (heading.textContent || "").trim() : "";

  track.setAttribute("aria-label", `${title}: ${segments.map((s) => `${s.label} ${perHundred(s.cents, whole)}`).join(", ")} of every $100`);
  track.replaceChildren();
  legend.replaceChildren();
  segments.forEach((s, i) => {
    const rank = String(s.id ? i + 1 : 0);
    const seg = clone(doc, "glance-segment");
    seg.dataset.rank = rank;
    seg.style.flexGrow = String(s.cents);
    seg.setAttribute("aria-label", `${s.label}: ${dollars(s.cents)}, ${share(s.cents, whole)}`);
    pinnable(seg, tips, (tip, pinned) => {
      const label = doc.createElement("div");
      label.className = "tip-label";
      label.textContent = s.label;
      const value = doc.createElement("div");
      value.className = "tip-value";
      value.textContent = dollars(s.cents);
      const meta = doc.createElement("div");
      meta.className = "tip-meta";
      meta.textContent = `◇ ${perHundred(s.cents, whole)} of every $100 of ${title}, our share` +
        (s.id ? "" : `: ${s.members.join(", ")}`);
      tip.append(label, value, meta, pageLinks(doc, pagesOf(s)), tipHint(doc, pinned));
    });
    track.appendChild(seg);

    const li = clone(doc, "glance-entry");
    li.dataset.rank = rank;
    li.dataset.cents = String(s.cents);
    /** @type {(sel: string) => HTMLElement} */
    const part = (sel) => {
      const el = li.querySelector(sel);
      if (!el) throw new Error(`the legend entry template has no ${sel}`);
      return /** @type {HTMLElement} */ (el);
    };
    part(".glance-per").textContent = perHundred(s.cents, whole);
    part(".glance-name").textContent = s.label;
    part(".glance-detail").textContent = s.id
      ? `${dollars(s.cents)}, ${share(s.cents, whole)}`
      : `${dollars(s.cents)}, ${share(s.cents, whole)}: ${s.members.join(", ")}`;
    legend.appendChild(li);
  });

  caption.textContent = `◇ Our sum of the ${rows.length} printed cells: ${dollars(whole)}. ` +
    "The shares are ours too, rounded to the cent, so they may not add to $100.00 exactly.";
  figure.hidden = false;
  // The table was the bar's data; the figure now says what it held.
  const cells = section.querySelector(".glance-cells");
  if (cells instanceof doc.defaultView.HTMLElement) cells.hidden = true;
  return { segments, whole, rows: rows.length };
}

/**
 * Draws every bar on the page.
 * @param {Document} doc
 */
export function boot(doc) {
  const tips = tipper(doc);
  for (const section of Array.from(doc.querySelectorAll("section.glance-bar"))) {
    draw(/** @type {HTMLElement} */ (section), tips);
  }
}
