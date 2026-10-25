// @ts-check
/**
 * fisc — shared chrome: theme toggle, menu button, scroll cues. A switch
 * dispatches THEME_EVENT so app.js repaints without a second click handler.
 */

import { scrollCues } from "./format.js";

/** The localStorage key the head's theme-stamp script reads. */
export const THEME_KEY = "fisc-theme";

/** Dispatched on the document after the toggle switches the page. */
export const THEME_EVENT = "fisc-theme";

const OS_DARK = "(prefers-color-scheme: dark)";

/**
 * Stamped theme, then the OS; matchMedia is missing in some headless renderers.
 * @param {Window & typeof globalThis} win
 * @returns {boolean}
 */
export function prefersDark(win) {
  const stamped = win.document.documentElement.dataset.theme;
  if (stamped === "dark") return true;
  if (stamped === "light") return false;
  return typeof win.matchMedia === "function" && win.matchMedia(OS_DARK).matches;
}

/**
 * The label is the click's action, so no aria-pressed: "Light mode,
 * pressed" on a dark page would announce the wrong state.
 * @param {Window & typeof globalThis} win
 */
export function syncTheme(win) {
  const button = win.document.getElementById("theme-toggle");
  if (!button) return;
  const dark = prefersDark(win);
  button.textContent = dark ? "Light mode" : "Dark mode";
}

/**
 * The narrow-screen menu: data-menu lets style.css fold the list, data-open
 * while open; Escape closes and refocuses the button. No script, no fold.
 * @param {Window & typeof globalThis} win
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

/** Where how-to-read prose folds; style.css's media query says the same. */
const NARROW = "(max-width: 760px)";

/**
 * Folds details.how-to-read narrow, opens it wide: there the summary is
 * hidden, so closed would hide the prose for good. No matchMedia: as shipped.
 * @param {Window & typeof globalThis} win
 */
export function wireHowTo(win) {
  if (typeof win.matchMedia !== "function") return;
  const blocks = /** @type {HTMLDetailsElement[]} */ (Array.from(win.document.querySelectorAll("details.how-to-read")));
  if (!blocks.length) return;
  const mq = win.matchMedia(NARROW);
  const apply = () => {
    for (const d of blocks) d.open = !mq.matches;
  };
  apply();
  if (typeof mq.addEventListener === "function") mq.addEventListener("change", apply);
  else if (typeof mq.addListener === "function") mq.addListener(apply);
}

/**
 * "More" under a lede the narrow clamp cuts; a press drops the clamp
 * (data-full) for good.
 * @param {Window & typeof globalThis} win
 */
export function wireLede(win) {
  const doc = win.document;
  const lede = /** @type {HTMLElement | null} */ (doc.querySelector(".page-head .lede"));
  const tmpl = /** @type {HTMLTemplateElement | null} */ (doc.getElementById("lede-more"));
  const cloned = tmpl && tmpl.content.firstElementChild;
  if (!lede || !cloned) return;
  lede.setAttribute("data-clamp", "");
  const more = /** @type {HTMLElement} */ (cloned.cloneNode(true));
  more.addEventListener("click", () => {
    lede.setAttribute("data-full", "");
    more.remove();
  });
  const check = () => {
    const cut = !lede.hasAttribute("data-full") && lede.scrollHeight > lede.clientHeight + 1;
    if (cut && !more.isConnected) lede.after(more);
    else if (!cut && more.isConnected) more.remove();
  };
  check();
  win.addEventListener("resize", check);
  // A page repaints the lede's year words after boot, which can cut it.
  if (typeof win.MutationObserver === "function") {
    new win.MutationObserver(check).observe(lede, { childList: true, characterData: true, subtree: true });
  }
}

/**
 * Opens the <details> the fragment names or is inside: browsers scroll to a
 * fragment but do not reliably open it.
 * @param {Window & typeof globalThis} win
 */
function openTarget(win) {
  let id = "";
  try {
    id = decodeURIComponent(win.location.hash.slice(1));
  } catch (e) { /* a malformed fragment names nothing */ }
  const el = id && win.document.getElementById(id);
  if (!el) return;
  let opened = false;
  for (let d = el.closest("details"); d; d = d.parentElement && d.parentElement.closest("details")) {
    if (!d.open) {
      d.open = true;
      opened = true;
    }
  }
  if (opened && typeof el.scrollIntoView === "function") el.scrollIntoView();
}

/**
 * Also on a click of the fragment already in the URL, which fires no
 * hashchange: a reader who closed it and follows the link again sees it open.
 * @param {Window & typeof globalThis} win
 */
export function wireTargets(win) {
  openTarget(win);
  win.addEventListener("hashchange", () => openTarget(win));
  win.document.addEventListener("click", (e) => {
    // A click that does not navigate this page -- to a new tab, or one a
    // handler took -- opens nothing here.
    if (e.defaultPrevented || e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return;
    const a = e.target instanceof win.Element ? e.target.closest("a[href^='#']") : null;
    if (a && /** @type {HTMLAnchorElement} */ (a).hash === win.location.hash) openTarget(win);
  });
}

/**
 * The toggle follows the OS so the button never announces the opposite of
 * what it does.
 * @param {Window & typeof globalThis} [win]
 */
export function boot(win = globalThis.window) {
  wireMenu(win);
  wireHowTo(win);
  wireLede(win);
  wireTargets(win);
  scrollCues(win.document);
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
