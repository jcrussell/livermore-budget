// @ts-check
/**
 * fisc — the chrome every page shares: the theme toggle and the menu button
 * in its top bar, and the cue on every box that scrolls sideways. Importing
 * this module does nothing; boot() wires the page it is handed.
 *
 * A switch dispatches THEME_EVENT on the document, so a page that writes
 * resolved colours onto its marks (app.js) repaints without wiring a second
 * click handler onto the same button.
 */

import { scrollCues } from "./format.js";

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

/** Where how-to-read prose folds; style.css's media query says the same. */
const NARROW = "(max-width: 760px)";

/**
 * Folds every details.how-to-read on a narrow screen and opens it on a wide
 * one, following the window across the breakpoint. Where the summary is
 * hidden (wide) a closed block would hide its prose with no way to open it,
 * so wide always opens. Without matchMedia every block stays as shipped,
 * open.
 * @param {Window} win
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
 * Offers "More" (the top bar's "lede-more" template) under the page head's
 * lede wherever style.css's narrow clamp cuts it short, kept current as the window resizes, and drops the clamp
 * (data-full) for good on a press. A lede the clamp does not cut has no
 * button.
 * @param {Window} win
 */
export function wireLede(win) {
  const doc = win.document;
  const lede = /** @type {HTMLElement | null} */ (doc.querySelector(".page-head .lede"));
  const tmpl = /** @type {HTMLTemplateElement | null} */ (doc.getElementById("lede-more"));
  const cloned = tmpl && tmpl.content.firstElementChild;
  if (!lede || !cloned) return;
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
}

/**
 * Opens every closed <details> the element the URL's fragment names is, or
 * is inside, and scrolls it into view. A browser scrolls to a fragment but
 * does not reliably open the <details> it names, which on the caveats page
 * would land a reader on a folded summary, or on nothing where the target is
 * inside one. A fragment naming nothing, or nothing folded, is left alone.
 * @param {Window} win
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
 * Opens the fragment's target now (openTarget), on every later change of
 * fragment, and on a click of a link to the fragment already in the URL,
 * which changes nothing and so fires no hashchange: a reader who closed the
 * caveat they arrived at and follows its link again would otherwise see
 * nothing happen.
 * @param {Window} win
 */
export function wireTargets(win) {
  openTarget(win);
  win.addEventListener("hashchange", () => openTarget(win));
  win.document.addEventListener("click", (e) => {
    const a = e.target instanceof win.Element ? e.target.closest("a[href^='#']") : null;
    if (a && /** @type {HTMLAnchorElement} */ (a).hash === win.location.hash) openTarget(win);
  });
}

/**
 * Wires the page's chrome: the menu (wireMenu), the folding prose
 * (wireHowTo), the clamped page lede (wireLede), a folded fragment target
 * (wireTargets), the cue on every box that
 * scrolls sideways (format.js scrollCues), and the page's theme toggle, shown
 * and following the OS setting so the button never announces the opposite of
 * what it does. A page with no toggle keeps no theme control.
 * @param {Window} [win]
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
