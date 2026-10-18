// @ts-check
/**
 * fisc — the theme toggle in every page's top bar. Importing this module does
 * nothing; boot() wires the page it is handed.
 *
 * A switch dispatches THEME_EVENT on the document, so a page that writes
 * resolved colours onto its marks (app.js) repaints without wiring a second
 * click handler onto the same button.
 */

/** The localStorage key the head's theme-stamp script reads. */
export const THEME_KEY = "fisc-theme";

/** Dispatched on the document after the toggle switches the page. */
export const THEME_EVENT = "fisc-theme";

const OS_DARK = "(prefers-color-scheme: dark)";

/**
 * Whether the page is dark: the stamped theme wins, then the OS setting.
 * matchMedia is missing in some headless renderers, hence the check.
 * @param {Window} win
 * @returns {boolean}
 */
export function prefersDark(win) {
  const stamped = win.document.documentElement.dataset.theme;
  if (stamped === "dark") return true;
  if (stamped === "light") return false;
  return typeof win.matchMedia === "function" && win.matchMedia(OS_DARK).matches;
}

/**
 * Brings the button's label back into agreement with the page. The label is
 * the action a click takes, so the button carries no aria-pressed: "Light
 * mode, pressed" on a dark page would announce the state it is not in.
 * @param {Window} win
 */
export function syncTheme(win) {
  const button = win.document.getElementById("theme-toggle");
  if (!button) return;
  const dark = prefersDark(win);
  button.textContent = dark ? "Light mode" : "Dark mode";
}

/**
 * Shows and wires the page's theme toggle, and follows the OS setting so the
 * button never announces the opposite of what it does. A page with no toggle
 * is left alone.
 * @param {Window} [win]
 */
export function boot(win = globalThis.window) {
  const doc = win.document;
  const button = doc.getElementById("theme-toggle");
  if (!button) return;
  button.addEventListener("click", () => {
    const dark = !prefersDark(win);
    doc.documentElement.dataset.theme = dark ? "dark" : "light";
    try {
      win.localStorage.setItem(THEME_KEY, dark ? "dark" : "light");
    } catch (e) { /* private mode: the toggle still works for this visit */ }
    syncTheme(win);
    doc.dispatchEvent(new win.CustomEvent(THEME_EVENT, { detail: { dark } }));
  });
  if (typeof win.matchMedia === "function") {
    win.matchMedia(OS_DARK).addEventListener("change", () => syncTheme(win));
  }
  syncTheme(win);
  button.removeAttribute("hidden");
}
