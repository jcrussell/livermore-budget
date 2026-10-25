// @ts-check
/**
 * fisc — the glance page's module; the template calls boot(). Groups each
 * server-rendered table's rows, sums and shares each group, and draws one
 * bar per table. Sums and shares are ours, marked ◇; Go writes the
 * projection's total beside the bar.
 */

import {
  NAMED, TAIL_LABEL, clone, dollars, hashFields, hashValue, linkedPage, pageLinks, pinnable, refuseFigure, selectYear, share, tipHead, tipHint, tipper,
  wireYearSelect,
} from "./format.js";

/**
 * @typedef {{ group: string, label: string, cents: number }} Row
 * @typedef {{ id: string, label: string, cents: number, members: string[] }} Segment
 *   members lists the labels folded into a tail; a named segment lists itself.
 */

/**
 * Refuses a row with no group, no label or a non-integer figure: a cell the
 * page cannot name is not drawn.
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
      // Number() would read "0x10", "1e3" or " 12 " as a figure.
      const cents = raw !== null && /^\d+$/.test(raw) ? Number(raw) : NaN;
      if (!group || !label || !Number.isSafeInteger(cents) || cents <= 0) {
        throw new Error(`a row of ${group || "no group"} (${label || "no label"}) carries ${raw} cents`);
      }
      rows.push({ group, label, cents });
    }
  }
  return rows;
}

/**
 * In first-appearance order.
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
 * Largest first, past NAMED folded into a tail unless it would hold one.
 * The whole sums every group.
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
 * Fills the template's bar: segments, legend with shares, the ◇ sentence. A
 * segment's tip links its rows' pages via `a[data-page]` and `data-pdf`.
 * @param {HTMLElement} section
 * @param {ReturnType<typeof tipper>} [tips]
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
  /** @param {Segment} s @returns {import("./format.js").CitedPage[]} */
  const pagesOf = (s) => Array.from(table.tBodies).flatMap((b) => Array.from(b.rows))
    .filter((tr) => (s.id ? tr.dataset.group === s.id : !named.has(tr.dataset.group || "")))
    .flatMap((tr) => Array.from(tr.querySelectorAll("a[data-page]")).map(linkedPage));
  const heading = section.querySelector("h3");
  const title = heading ? (heading.textContent || "").trim() : "";

  track.setAttribute("aria-label", `${title}, our shares: ${segments.map((s) => `${s.label} ◇ ${share(s.cents, whole)}`).join(", ")}`);
  track.replaceChildren();
  legend.replaceChildren();
  segments.forEach((s, i) => {
    const rank = String(s.id ? i + 1 : 0);
    const seg = clone(doc, "glance-segment");
    seg.dataset.rank = rank;
    seg.style.flexGrow = String(s.cents);
    seg.setAttribute("aria-label", `${s.label}: ${dollars(s.cents)}, ◇ ${share(s.cents, whole)}, our share`);
    pinnable(seg, tips, (tip, pinned) => {
      const meta = `◇ ${share(s.cents, whole)} of ${title}, our share` + (s.id ? "" : `: ${s.members.join(", ")}`);
      tip.append(...tipHead(doc, s.label, dollars(s.cents), meta), pageLinks(doc, pagesOf(s)), tipHint(doc, pinned));
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
    part(".glance-per").textContent = "◇ " + share(s.cents, whole);
    part(".glance-name").textContent = s.label;
    part(".glance-detail").textContent = s.id ? dollars(s.cents) : `${dollars(s.cents)}: ${s.members.join(", ")}`;
    legend.appendChild(li);
  });

  // Where Go declares the cells tie to the total printed under the figure,
  // that line already states the sum, so the caption does not say it twice.
  const shares = "each rounded to a tenth of a percent, so they may not add to 100% exactly.";
  caption.textContent = section.dataset.ties === "true"
    ? `◇ The shares are ours: each a part of the ${rows.length} printed cells, which sum to the total below, ${shares}`
    : `◇ Our sum of the ${rows.length} printed cells: ${dollars(whole)}. The shares are ours too, ${shares}`;
  figure.hidden = false;
  // The table was the bar's data; the figure now says what it held.
  const cells = section.querySelector(".glance-cells");
  if (cells instanceof doc.defaultView.HTMLElement) cells.hidden = true;
  return { segments, whole, rows: rows.length };
}

/**
 * An unknown key shows the select's year; with no select every year shows.
 * @param {Document} doc
 * @param {string} key
 * @returns {string} the key shown, or "" with no select
 */
export function showYear(doc, key) {
  const sections = Array.from(doc.querySelectorAll("section.glance-year[data-year]"));
  const select = /** @type {HTMLSelectElement | null} */ (doc.getElementById("year-select"));
  if (!select || sections.length < 2) return "";
  const shown = sections.some((s) => /** @type {HTMLElement} */ (s).dataset.year === key)
    ? key
    : select.value || select.options[0].value;
  for (const s of sections) /** @type {HTMLElement} */ (s).hidden = /** @type {HTMLElement} */ (s).dataset.year !== shown;
  selectYear(doc, shown);
  return shown;
}

/** @param {Document} doc */
export function boot(doc) {
  const tips = tipper(doc);
  for (const section of Array.from(doc.querySelectorAll("section.glance-bar"))) {
    // One figure that cannot draw says so in its own section, and the
    // others still draw.
    try {
      draw(/** @type {HTMLElement} */ (section), tips);
    } catch (err) {
      refuseFigure(/** @type {HTMLElement} */ (section), err);
    }
  }
  const win = doc.defaultView;
  const asked = hashFields(win ? win.location.hash : "", ["y"]).get("y") || "";
  showYear(doc, asked);
  // A tip pinned or shown over a year being hidden would state that year's
  // figure beside the other's bars.
  const turn = (/** @type {string} */ key) => {
    tips.unpin(false);
    tips.hide();
    return showYear(doc, key);
  };
  wireYearSelect(doc, (key) => {
    const shown = turn(key);
    if (win && shown) win.history.replaceState(null, "", "#y=" + hashValue(shown));
  });
  if (win) win.addEventListener("hashchange", () => turn(hashFields(win.location.hash, ["y"]).get("y") || ""));
}
