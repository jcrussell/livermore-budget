// @ts-check
/**
 * fisc — the text the page modules read and write alike: a figure in
 * dollars, a part's share of a whole, a page's JSON config, and the fields
 * of its hash. Every export is a function of its arguments; importing it
 * runs nothing.
 */

/**
 * Whole dollars with thousands separators, signed, cents only where a
 * figure has them.
 * @param {number} cents
 */
export function dollars(cents) {
  const whole = cents % 100 === 0;
  return (cents < 0 ? "-$" : "$") + (Math.abs(cents) / 100).toLocaleString("en-US",
    { minimumFractionDigits: whole ? 0 : 2, maximumFractionDigits: 2 });
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
