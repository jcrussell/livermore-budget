// @ts-check
/**
 * fisc — text the page modules share: dollars, shares, the config, the hash,
 * the year select, cited pages and the tip. Importing runs nothing and no
 * export reads a global; tipper, pinnable and wireYearSelect add listeners
 * per call.
 */

/** How many groups a ranked figure names before the rest fold into one tail. */
export const NAMED = 5;

/** What the folded tail is called. */
export const TAIL_LABEL = "Everything else";

/**
 * Signed whole dollars, cents only where present. Integer arithmetic: no
 * float between a cited figure and its digits; a non-safe-integer is refused.
 * @param {number} cents
 */
export function dollars(cents) {
  if (!Number.isSafeInteger(cents)) throw new Error(`dollars: ${cents} is not a safe integer of cents`);
  const abs = Math.abs(cents);
  const rem = abs % 100;
  const whole = (abs - rem) / 100;
  return (cents < 0 ? "-$" : "$") + whole.toLocaleString("en-US", { maximumFractionDigits: 0 }) +
    (rem === 0 ? "" : "." + String(rem).padStart(2, "0"));
}

/**
 * To a tenth, "" of a zero whole. "<0.1%" and ">99.9%" rather than 0.0 or
 * 100.0: a sliver is not nothing, and the largest part is not the whole.
 * @param {number} part
 * @param {number} whole
 */
export function share(part, whole) {
  if (!whole) return "";
  const pct = (100 * Math.abs(part)) / Math.abs(whole);
  if (pct > 0 && pct < 0.1) return "<0.1%";
  if (pct < 100 && pct > 99.9) return ">99.9%";
  return pct.toFixed(1) + "%";
}

/**
 * @param {Document} doc
 * @param {string} id
 * @param {string} name what the refusal calls the config
 */
export function configOf(doc, id, name) {
  const el = doc.getElementById(id);
  if (!el) throw new Error(`the page carries no ${name} config`);
  return JSON.parse(el.textContent || "");
}

/**
 * Escaped but for the slashes ids hold, so the hash stays readable.
 * @param {string} v
 */
export function hashValue(v) {
  return encodeURIComponent(v).replace(/%2F/g, "/");
}

/**
 * Named fields only, decoded (raw where undecodable); the last of a repeat wins.
 * @param {string} hash
 * @param {readonly string[]} names
 * @returns {Map<string, string>}
 */
export function hashFields(hash, names) {
  /** @type {Map<string, string>} */
  const fields = new Map();
  for (const part of String(hash || "").replace(/^#/, "").split("&")) {
    const eq = part.indexOf("=");
    if (eq < 0) continue;
    const key = part.slice(0, eq);
    if (!names.includes(key)) continue;
    try {
      fields.set(key, decodeURIComponent(part.slice(eq + 1)));
    } catch {
      fields.set(key, part.slice(eq + 1));
    }
  }
  return fields;
}

/**
 * The column `y` names, else the newest, with `refused` naming what was not offered.
 * @param {Map<string, string>} fields
 * @param {any} config
 * @returns {{year: string, refused: string}}
 */
export function hashYear(fields, config) {
  const years = config.years.map((/** @type {any} */ y) => y.path);
  const y = fields.get("y");
  if (y === undefined) return { year: years[years.length - 1], refused: "" };
  const named = years.find((/** @type {string} */ p) => yearKey(p) === y);
  return named ? { year: named, refused: "" }
    : { year: years[years.length - 1], refused: `No budget column is called ${y}, so this shows the newest.` };
}

/**
 * Fetches each column once; refuses one not ok or from another export, and
 * forgets a refusal so asking again refetches.
 * @param {any} win
 * @param {string} exportedBy the page config's exported_by
 * @returns {(path: string) => Promise<any>}
 */
export function columnLoader(win, exportedBy) {
  /** @type {Map<string, Promise<any>>} */
  const columns = new Map();
  return (path) => {
    let c = columns.get(path);
    if (!c) {
      c = Promise.resolve(win.fetch(path)).then((/** @type {any} */ r) => {
        if (!r.ok) throw new Error(`${path} answered ${r.status}`);
        return r.json();
      }).then((/** @type {any} */ col) => {
        if (col.generated_by !== exportedBy) {
          throw new Error(`${path} was written by ${col.generated_by} and this page by ${exportedBy}; reload the page`);
        }
        return col;
      });
      c.catch(() => columns.delete(path));
      columns.set(path, c);
    }
    return c;
  };
}

/**
 * The file name less .json: the select's value and `#y=` on every page.
 * @param {string} path
 */
export function yearKey(path) {
  return String(path).replace(/\.json$/, "");
}

/**
 * Points `data-years` links offering the year at `#y=<key>` and the rest at
 * their opening view, so an unpublished year is not carried to be refused.
 * @param {Document} doc
 * @param {string} key "" carries none
 */
export function carryYear(doc, key) {
  for (const a of Array.from(doc.querySelectorAll("a[data-years]"))) {
    const base = (a.getAttribute("href") || "").replace(/#.*$/, "");
    const offers = (a.getAttribute("data-years") || "").split(" ").includes(key);
    a.setAttribute("href", key && offers ? base + "#y=" + hashValue(key) : base);
  }
}

/**
 * A one-year page has no select and still carries.
 * @param {Document} doc
 * @param {string} key
 */
export function selectYear(doc, key) {
  const select = /** @type {HTMLSelectElement | null} */ (doc.getElementById("year-select"));
  if (select) select.value = key;
  carryYear(doc, key);
}

/**
 * Enables the select (shipped disabled) and hands choices to `onChange`.
 * Returns the shown key, maybe browser-restored, or undefined with no select.
 * @param {Document} doc
 * @param {(key: string) => void} onChange
 * @returns {string | undefined}
 */
export function wireYearSelect(doc, onChange) {
  const select = /** @type {HTMLSelectElement | null} */ (doc.getElementById("year-select"));
  if (!select) return undefined;
  select.disabled = false;
  select.addEventListener("change", () => onChange(select.value));
  return select.value;
}

/**
 * Refused where the export built none, so a cited page cannot vanish.
 * @param {Record<string, {pages?: Record<string, {pdf?: string, text?: string, records?: string}>}>} docs
 * @param {string} docId
 * @param {number} page
 * @returns {{pdf?: string, text?: string, records?: string}}
 */
export function pageOf(docs, docId, page) {
  const doc = docs[docId];
  const links = doc && doc.pages && doc.pages[String(page)];
  if (!links) throw new Error("cannot cite " + docId + " p" + page + ": the page publishes no links for it");
  return links;
}

/**
 * Unlike pageOf, keeps an unbuilt page with empty links, so a tip names it
 * rather than throwing inside a pointer or key handler.
 * @param {Parameters<typeof pageOf>[0]} docs
 * @param {{doc_id: string, pages: number[]}[]} locators
 * @returns {CitedPage[]}
 */
export function citedPages(docs, locators) {
  return locators.flatMap((l) => l.pages.map((p) => {
    const d = docs[l.doc_id];
    const links = d && d.pages && d.pages[String(p)];
    return { doc: l.doc_id, page: p, text: (links && links.text) || "", pdf: (links && links.pdf) || "" };
  }));
}

/**
 * Read off `data-page`, href and `data-pdf`; "" where absent.
 * @param {Element} a
 * @returns {CitedPage}
 */
export function linkedPage(a) {
  return { page: Number(a.getAttribute("data-page")), text: a.getAttribute("href") || "", pdf: a.getAttribute("data-pdf") || "" };
}

/** text and pdf are "" where none was built. @typedef {{doc?: string, page: number, text: string, pdf: string}} CitedPage */

/**
 * `p97 PDF, p98 PDF`, once per pair of links; an unlinked page is named once
 * per document and page, marked `data-unlinked`.
 * @param {Document} doc
 * @param {CitedPage[]} pages
 * @returns {HTMLElement}
 */
export function pageLinks(doc, pages) {
  const line = doc.createElement("div");
  line.className = "facts";
  const seen = new Set();
  for (const { doc: id, page, text, pdf } of pages) {
    const key = text || pdf ? text + " " + pdf : (id || "") + "\u001f" + page;
    if (seen.has(key)) continue;
    seen.add(key);
    if (line.childNodes.length) line.append(", ");
    if (text) {
      const a = doc.createElement("a");
      a.className = "page-text";
      a.href = text;
      a.title = `Read on page ${page}`;
      a.textContent = `p${page}`;
      line.append(a);
    } else {
      const span = doc.createElement("span");
      span.textContent = `p${page}`;
      if (!pdf) {
        span.dataset.unlinked = "";
        span.title = `This page publishes no link for page ${page}`;
      }
      line.append(span);
    }
    if (pdf) {
      const a = doc.createElement("a");
      a.className = "page-pdf";
      a.href = pdf;
      a.title = `The city's PDF at page ${page}`;
      a.rel = "noopener";
      a.textContent = "PDF";
      line.append(" ", a);
    }
  }
  return line;
}

/**
 * Beside the pointer, or under the mark for focus or a key, so a keyboard
 * user sees it beside what they reached.
 * @param {Document} doc
 * @param {HTMLElement} tip
 * @param {{clientX?: number, clientY?: number, target?: any}} e
 */
export function placeTip(doc, tip, e) {
  const win = doc.defaultView;
  let x = 0;
  let y = 0;
  if (typeof e.clientX === "number" && typeof e.clientY === "number") {
    x = e.clientX;
    y = e.clientY;
  } else if (e.target && typeof e.target.getBoundingClientRect === "function") {
    const r = e.target.getBoundingClientRect();
    x = r.left;
    y = r.bottom;
  }
  tip.style.left = (x + (win ? win.scrollX : 0) + 14) + "px";
  tip.style.top = (y + (win ? win.scrollY : 0) + 14) + "px";
}

/**
 * The page's `#tooltip`: shown on hover, pinned by click, Enter or Space. A
 * pinned tip takes pointer and focus so its links work; hovering leaves it.
 * Escape, or a click outside it and every `[data-pins]`, unpins; Escape
 * returns focus to the mark. The pinned mark lives in this closure.
 * @param {Document} doc
 */
export function tipper(doc) {
  /** @type {HTMLElement | null} the mark the pinned tip is of */
  let mark = null;
  /** Whether tips.focus is handing a mark the focus, whose focus shows no tip. */
  let refocusing = false;
  const tipEl = () => /** @type {HTMLElement | null} */ (doc.getElementById("tooltip"));
  /**
   * @param {{clientX?: number, clientY?: number, target?: any}} e
   * @param {(tip: HTMLElement, pinned: boolean) => void} fill
   * @param {boolean} pinned
   */
  const paint = (e, fill, pinned) => {
    const tip = tipEl();
    if (!tip) return null;
    tip.replaceChildren();
    fill(tip, pinned);
    placeTip(doc, tip, e);
    tip.hidden = false;
    return tip;
  };
  const tips = {
    /** Whether a tip is pinned. */
    pinned: () => mark !== null,
    /**
     * Unless one is pinned.
     * @param {{clientX?: number, clientY?: number, target?: any}} e
     * @param {(tip: HTMLElement, pinned: boolean) => void} fill
     */
    show(e, fill) {
      if (!mark && !refocusing) paint(e, fill, false);
    },
    /** Hides the tip, unless it is pinned. */
    hide() {
      const tip = tipEl();
      if (!mark && tip) tip.hidden = true;
    },
    /**
     * Pins a mark's tip under the mark and moves the focus into it.
     * @param {HTMLElement} at the mark
     * @param {(tip: HTMLElement, pinned: boolean) => void} fill
     * @param {{clientX?: number, clientY?: number, target?: any}} [e]
     */
    pin(at, fill, e = { target: at }) {
      mark = null;
      const tip = paint(e, fill, true);
      if (!tip) return;
      mark = at;
      tip.dataset.pinned = "true";
      tip.setAttribute("role", "dialog");
      tip.setAttribute("aria-label", at.getAttribute("aria-label") || "Details");
      tip.tabIndex = -1;
      const first = /** @type {HTMLElement | null} */ (tip.querySelector("a[href]"));
      (first || tip).focus();
    },
    /**
     * Refocuses the mark where `refocus` asks and it is still drawn.
     * @param {boolean} refocus
     */
    unpin(refocus) {
      const was = mark;
      if (!was) return;
      mark = null;
      const tip = tipEl();
      if (tip) {
        delete tip.dataset.pinned;
        tip.setAttribute("role", "status");
        tip.removeAttribute("aria-label");
        tip.removeAttribute("tabindex");
        tip.hidden = true;
      }
      if (refocus && was.isConnected) tips.focus(was);
    },
    /**
     * Focuses a mark with no tip shown, as unpin and a redraw do.
     * @param {HTMLElement} at
     */
    focus(at) {
      refocusing = true;
      try {
        at.focus();
      } finally {
        refocusing = false;
      }
    },
  };
  doc.addEventListener("keydown", (e) => {
    if (e.key !== "Escape" || !mark) return;
    e.preventDefault();
    tips.unpin(true);
  });
  doc.addEventListener("click", (e) => {
    if (!mark) return;
    const target = /** @type {any} */ (e.target);
    const tip = tipEl();
    if (tip && target && tip.contains(target)) return;
    if (target && typeof target.closest === "function" && target.closest("[data-pins]")) return;
    tips.unpin(false);
  });
  return tips;
}

/**
 * @param {Document} doc
 * @param {string} className
 * @param {string} text
 */
export function tipLine(doc, className, text) {
  const el = doc.createElement("div");
  el.className = className;
  el.textContent = text;
  return el;
}

/**
 * @param {Document} doc
 * @param {string} label
 * @param {string} value
 * @param {string} meta
 */
export function tipHead(doc, label, value, meta) {
  return [tipLine(doc, "tip-label", label), tipLine(doc, "tip-value", value), tipLine(doc, "tip-meta", meta)];
}

/**
 * @param {Document} doc
 * @param {boolean} pinned
 */
export function tipHint(doc, pinned) {
  return tipLine(doc, "tip-meta", pinned ? "Escape or a click elsewhere closes this." : "Click to keep this open and follow its pages.");
}

/**
 * The source table stays shown beneath; it is hidden only once a figure draws.
 * @param {HTMLElement} section
 * @param {unknown} err
 */
export function refuseFigure(section, err) {
  const doc = section.ownerDocument;
  let el = section.querySelector(":scope > p.refusal");
  if (!el) {
    el = doc.createElement("p");
    el.className = "refusal";
    el.setAttribute("role", "alert");
    const heading = section.querySelector("h2, h3");
    if (heading && heading.parentElement === section) heading.after(el);
    else section.prepend(el);
  }
  const why = (err instanceof Error ? err.message : String(err)).replace(/\.$/, "");
  el.textContent = `This figure could not be drawn: ${why}. The table below is what the page carries.`;
}

/**
 * Hover and focus show, leave and blur hide, click, Enter or Space pin.
 * @param {HTMLElement} mark
 * @param {ReturnType<typeof tipper>} tips
 * @param {(tip: HTMLElement, pinned: boolean) => void} fill
 */
export function pinnable(mark, tips, fill) {
  mark.dataset.pins = "";
  mark.tabIndex = 0;
  if (!mark.getAttribute("role")) mark.setAttribute("role", "button");
  mark.addEventListener("pointerenter", (e) => tips.show(e, fill));
  mark.addEventListener("pointermove", (e) => tips.show(e, fill));
  mark.addEventListener("pointerleave", () => tips.hide());
  mark.addEventListener("focus", (e) => tips.show(e, fill));
  mark.addEventListener("blur", () => tips.hide());
  mark.addEventListener("click", (e) => tips.pin(mark, fill, e));
  mark.addEventListener("keydown", (e) => {
    if (e.key !== "Enter" && e.key !== " ") return;
    e.preventDefault();
    tips.pin(mark, fill);
  });
}

/** The boxes that scroll sideways and say so at the edge they hide. */
const SCROLL_CUED = ".table-scroll, .chart-scroll";

/**
 * "left", "right", "both" or ""; a pixel of slack absorbs sub-pixel widths.
 * @param {{scrollLeft: number, scrollWidth: number, clientWidth: number}} el
 * @returns {string}
 */
export function hiddenEdges(el) {
  const left = el.scrollLeft > 1;
  const right = el.scrollLeft + el.clientWidth < el.scrollWidth - 1;
  return left && right ? "both" : left ? "left" : right ? "right" : "";
}

/**
 * Keeps data-more current on every SCROLL_CUED box so style.css fades the
 * hiding edge: a box cut at the card's edge otherwise reads as broken.
 * @param {Document} doc
 */
export function scrollCues(doc) {
  const win = doc.defaultView;
  const boxes = /** @type {HTMLElement[]} */ (Array.from(doc.querySelectorAll(SCROLL_CUED)));
  const observes = Boolean(win && typeof win.ResizeObserver === "function");
  const mark = (/** @type {HTMLElement} */ el) => {
    const more = hiddenEdges(el);
    if (more) el.dataset.more = more;
    else delete el.dataset.more;
  };
  for (const el of boxes) {
    mark(el);
    el.addEventListener("scroll", () => mark(el), { passive: true });
    if (observes) {
      const ro = new /** @type {Window & typeof globalThis} */ (win).ResizeObserver(() => mark(el));
      ro.observe(el);
      if (el.firstElementChild) ro.observe(el.firstElementChild);
    }
  }
  if (win && !observes) win.addEventListener("resize", () => boxes.forEach(mark));
}

/**
 * @param {Document} doc
 * @param {string} id
 * @returns {HTMLElement | SVGElement}
 */
export function clone(doc, id) {
  const tmpl = doc.getElementById(id);
  if (!(tmpl instanceof /** @type {Window & typeof globalThis} */ (doc.defaultView).HTMLTemplateElement)) {
    throw new Error(`the page carries no <template id="${id}"> to clone`);
  }
  const el = tmpl.content.firstElementChild;
  if (!el) throw new Error(`<template id="${id}"> is empty`);
  return /** @type {HTMLElement | SVGElement} */ (el.cloneNode(true));
}

/**
 * One `<page>-refusal` alert atop `<page>-main`; empty text clears it.
 * @param {Document} doc
 * @param {string} page
 * @param {string} text
 */
export function sayRefusal(doc, page, text) {
  let el = doc.getElementById(`${page}-refusal`);
  if (!text) {
    if (el) el.remove();
    return;
  }
  if (!el) {
    el = doc.createElement("p");
    el.id = `${page}-refusal`;
    el.className = "refusal";
    el.setAttribute("role", "alert");
    const main = doc.getElementById(`${page}-main`);
    if (main) main.prepend(el);
  }
  el.textContent = text;
}

/**
 * Null where the row prints none; refuses a non-integer (empty included) or
 * a cell outside the columns.
 * @param {HTMLTableRowElement} tr
 * @param {number} columns
 * @param {string} who the row, for a refusal
 * @returns {(number | null)[]}
 */
export function rowCents(tr, columns, who) {
  /** @type {(number | null)[]} */
  const cents = new Array(columns).fill(null);
  for (const td of Array.from(tr.querySelectorAll("td[data-column]"))) {
    const at = Number(td.getAttribute("data-column"));
    const raw = td.getAttribute("data-cents");
    if (raw === null) continue;
    // Number("") is 0, and an empty attribute is not a printed zero.
    const v = /^-?\d+$/.test(raw) ? Number(raw) : NaN;
    if (!Number.isSafeInteger(v) || !(at >= 0 && at < columns)) {
      throw new Error(`${who} carries ${JSON.stringify(raw)} cents in column ${at}`);
    }
    cents[at] = v;
  }
  return cents;
}

/**
 * Absent plus absent stays absent: null until a printed figure, zero included.
 * @param {number | null} sum
 * @param {number | null} c
 * @returns {number | null}
 */
export function plus(sum, c) {
  return c === null ? sum : (sum ?? 0) + c;
}
