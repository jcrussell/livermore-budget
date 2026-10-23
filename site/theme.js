// @ts-check
/**
 * fisc — the controls in every page's top bar: the theme toggle, and the menu
 * button the view list folds behind on a narrow screen. Importing this
 * module does nothing; boot() wires the page it is handed.
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
 * Shows and wires the menu button the view list folds behind on a narrow
 * screen. It marks the nav data-menu, which lets style.css fold the list
 * there, and data-open while the reader has it open; Escape closes it and
 * puts focus back on the button. A page with no button is left alone, and
 * with no script the list is never folded.
 * @param {Window} win
 */
export function wireMenu(win) {
  const doc = win.document;
  const button = doc.getElementById("nav-toggle");
  const nav = button && button.closest("nav");
  if (!button || !nav) return;
  /** @param {boolean} open */
  const set = (open) => {
    nav.toggleAttribute("data-open", open);
    button.setAttribute("aria-expanded", String(open));
  };
  button.addEventListener("click", () => set(!nav.hasAttribute("data-open")));
  nav.addEventListener("keydown", (e) => {
    if (e.key === "Escape" && nav.hasAttribute("data-open")) {
      set(false);
      button.focus();
    }
  });
  nav.setAttribute("data-menu", "");
  set(false);
  button.removeAttribute("hidden");
}

/**
 * Wires the top bar: the menu (wireMenu), and the page's theme toggle,
 * shown and following the OS setting so the button never announces the
 * opposite of what it does. A page with no toggle keeps no theme control.
 * @param {Window} [win]
 */
export function boot(win = globalThis.window) {
  wireMenu(win);
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
  syncTheme(win);
  button.removeAttribute("hidden");
  if (typeof win.matchMedia === "function") {
    // Safari before 14 offers only addListener.
    const os = win.matchMedia(OS_DARK);
    const follow = () => syncTheme(win);
    if (typeof os.addEventListener === "function") os.addEventListener("change", follow);
    else if (typeof os.addListener === "function") os.addListener(follow);
  }
}
