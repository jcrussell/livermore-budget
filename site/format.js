// @ts-check
/**
 * fisc — the text the page modules read and write alike: a figure in
 * dollars, a part's share of a whole, a page's JSON config, the fields of
 * its hash, the top bar's year select, cited pages and the tip. Importing it
 * runs nothing, and no export reads a global. Most exports are functions of
 * their arguments; tipper keeps the pinned mark in the object it returns and
 * adds listeners to the document it is handed, and pinnable and
 * wireYearSelect add listeners to the elements they are handed, so each
 * call wires another set.
 */

/**
 * Whole dollars with thousands separators, signed, cents only where a
 * figure has them. Integer arithmetic throughout: the dollars and the cents
 * are split by remainder, so no float stands between a cited figure and its
 * printed digits, and anything but a safe integer of cents is refused.
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
 * A part's share of the whole as a signed percentage to a tenth, or "" of
 * a whole of nothing.
 * @param {number} part
 * @param {number} whole
 */
export function share(part, whole) {
  return whole === 0 ? "" : (100 * part / whole).toFixed(1) + "%";
}

/**
 * The page's config, parsed from the JSON script element of that id.
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
 * One value of a hash, escaped except for the slashes every id holds, so
 * the hash stays readable.
 * @param {string} v
 */
export function hashValue(v) {
  return encodeURIComponent(v).replace(/%2F/g, "/");
}

/**
 * The fields of a hash among those named, each decoded, or kept raw where it
 * does not decode. A part with no `=` or a field not named is skipped, and a
 * field written twice keeps its last value.
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
 * A column's key, the same on every page: its file name less .json. It is
 * the top bar's option value and a page's `#y=`.
 * @param {string} path
 */
export function yearKey(path) {
  return String(path).replace(/\.json$/, "");
}

/**
 * Points each top-bar link whose view offers the year (its `data-years`) at
 * that year, `#y=<key>`, and every other at its view as it opens: a year a
 * view does not publish is not carried there to be refused. "" carries none.
 * @param {Document} doc
 * @param {string} key
 */
export function carryYear(doc, key) {
  for (const a of Array.from(doc.querySelectorAll("nav.site-nav a[data-years]"))) {
    const base = (a.getAttribute("href") || "").replace(/#.*$/, "");
    const offers = (a.getAttribute("data-years") || "").split(" ").includes(key);
    a.setAttribute("href", key && offers ? base + "#y=" + hashValue(key) : base);
  }
}

/**
 * Moves the top bar's year select to the year on screen and carries that year
 * onto the top bar's links (carryYear). A page with one year has no select
 * and still carries.
 * @param {Document} doc
 * @param {string} key
 */
export function selectYear(doc, key) {
  const select = /** @type {HTMLSelectElement | null} */ (doc.getElementById("year-select"));
  if (select) select.value = key;
  carryYear(doc, key);
}

/**
 * Wires the top bar's year select, which the template ships disabled:
 * enables it and hands each key chosen to `onChange`, which shows the year
 * and calls selectYear once it is on screen. Returns the key the select
 * shows, which a browser may have restored, or undefined with no select.
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
 * The links the export built for one cited page of one document, refused
 * where it built none, so a cited page cannot vanish from what cites it.
 * @param {Record<string, {pages?: Record<string, {pdf?: string, text?: string, records?: string}>}>} docs
 *   the page config's `docs`
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
 * The pages a set of locators cites, each with the links the export built
 * for it in the config's `docs`, in the locators' order. A page the config
 * built no links for is kept with both links empty, so pageLinks names it
 * unlinked rather than a tip throwing inside a pointer or key handler.
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
 * The cited page a table's own page link names: its `data-page` number, its
 * href to the extracted text and its `data-pdf` to the city's PDF page,
 * either "" where the link carries none.
 * @param {Element} a
 * @returns {CitedPage}
 */
export function linkedPage(a) {
  return { page: Number(a.getAttribute("data-page")), text: a.getAttribute("href") || "", pdf: a.getAttribute("data-pdf") || "" };
}

/**
 * One cited page as a tip links it: its extracted text and the city's PDF
 * at the page, either "" where none was built, and the document it is a
 * page of where the citation names one.
 * @typedef {{doc?: string, page: number, text: string, pdf: string}} CitedPage
 */

/**
 * A tip's line of cited pages, `p97 PDF, p98 PDF`: each page number links
 * its extracted text and the PDF link beside it the city's document at that
 * page, once per pair of links however many cells cite it. A page with
 * neither link is named, marked `data-unlinked`, and not linked, once per
 * document and page.
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
 * Stands a tip beside the pointer, or under the mark itself on an event
 * carrying no pointer, as a focus or a key does: a keyboard user's tip is
 * seen beside the mark they reached.
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
 * The page's `#tooltip`, shown on hover and pinned by a click, Enter or
 * Space on a mark. A pinned tip takes the pointer and the focus, so its
 * links can be clicked and tabbed to; hovering or leaving a mark leaves it
 * as it is, and Escape or a click outside it and outside every mark
 * (`[data-pins]`) unpins it, Escape handing the focus back to its mark
 * with no tip showing until the next hover or focus.
 * `fill` writes the tip's content into the emptied tip, told whether it is
 * being pinned. The pinned mark is held in this call's closure, one per
 * page booted.
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
     * Shows a mark's tip, unless one is pinned.
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
     * Unpins and hides the tip, handing the focus back to its mark where
     * `refocus` asks and the mark is still drawn.
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
     * Hands the focus to a mark with no tip shown for it, as unpin does and
     * as a redraw does to the row it was asked on.
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
 * The tip's last line: how to keep it, or how to let it go.
 * @param {Document} doc
 * @param {boolean} pinned
 */
export function tipHint(doc, pinned) {
  const hint = doc.createElement("div");
  hint.className = "tip-meta";
  hint.textContent = pinned ? "Escape or a click elsewhere closes this." : "Click to keep this open and follow its pages.";
  return hint;
}

/**
 * Says in a figure's own section that it could not be drawn, and why, under
 * its heading. The table it was to be drawn from is hidden only once a
 * figure draws, so it stays shown beneath.
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
 * Wires a mark to a page's tip: hover and focus show it, leaving and blur
 * hide it, and a click, Enter or Space pins it. `fill` is the mark's
 * tipper fill.
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
