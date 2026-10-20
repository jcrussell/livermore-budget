// @ts-check
/**
 * fisc — the text the page modules read and write alike: a figure in
 * dollars, a part's share of a whole, a page's JSON config, the fields of
 * its hash, and the top bar's year select. Every export is a function of its
 * arguments; importing it runs nothing.
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
