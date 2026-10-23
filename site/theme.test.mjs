// What site/theme.js does for the top bar of every page Go pins under
// testdata/: one toggle, in the nav, that switches the page, stores the
// choice and says what it will do.
import { describe, test } from "node:test";
import assert from "node:assert/strict";
import { readdirSync, readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const HERE = path.dirname(fileURLToPath(import.meta.url));
const MODULE = pathToFileURL(path.join(HERE, "theme.js")).href;
const TESTDATA = path.join(HERE, "..", "testdata");
const PAGES = readdirSync(TESTDATA).filter((f) => f.endsWith(".golden.html")).sort();

test("theme.js imports with no DOM and runs nothing", async () => {
  assert.equal(typeof globalThis.document, "undefined", "nothing installed a DOM before the import");
  const mod = await import(MODULE);
  assert.deepEqual(Object.keys(mod).sort(), ["THEME_EVENT", "THEME_KEY", "boot", "prefersDark", "syncTheme", "wireHowTo", "wireLede", "wireMenu"]);
});

/** One pinned page in a browser, theme.js booted against it as its top bar does. */
async function booted(page, o = {}) {
  const { installBrowser } = await import("./testlib.mjs");
  const browser = installBrowser({ html: readFileSync(path.join(TESTDATA, page), "utf8"), osDark: o.osDark });
  const mod = await import(MODULE);
  const events = [];
  browser.document.addEventListener(mod.THEME_EVENT, (e) => events.push(e.detail.dark));
  mod.boot(browser.window);
  return { ...browser, mod, events };
}

/**
 * The toggle's state as a reader meets it, as one string. Its label is the
 * action a click takes, so it carries no aria-pressed, which would announce
 * "Light mode, pressed" on a dark page.
 *
 * Mutation: set aria-pressed in syncTheme again and every arm reading the
 * toggle goes red.
 */
const read = (button) =>
  `${button.textContent} / aria-pressed=${button.getAttribute("aria-pressed")} / hidden=${button.hidden}`;

describe("the theme toggle in every pinned page's top bar", () => {
  test("the pins are every page Go pins", (t) => {
    t.diagnostic(`pinned pages: ${PAGES.join(", ")}`);
    // Anti-vacuity: a glob that matched nothing would pass every test below.
    assert.ok(PAGES.length > 1 && PAGES.includes("index.golden.html"), PAGES.join(", "));
  });

  for (const page of PAGES) {
    test(`${page}: one toggle, in the nav, that switches the page and stores the choice`, async (t) => {
      const { document, window, events } = await booted(page);
      const toggles = document.querySelectorAll("#theme-toggle");
      assert.equal(toggles.length, 1, `${page} carries ${toggles.length} theme toggles`);
      const button = /** @type {HTMLButtonElement} */ (toggles[0]);
      assert.ok(button.closest("nav.site-nav") !== null, `${page}'s theme toggle is outside nav.site-nav`);
      const before = read(button);
      button.click();
      const theme = document.documentElement.dataset.theme || "";
      const stored = window.localStorage.getItem("fisc-theme");
      const after = read(button);
      button.click();
      const back = document.documentElement.dataset.theme || "";
      t.diagnostic(`${page}: "${before}" -> "${after}"; data-theme "${theme}" then "${back}", stored "${stored}", events ${JSON.stringify(events)}`);
      assert.equal(before, "Dark mode / aria-pressed=null / hidden=false");
      assert.equal(theme, "dark");
      assert.equal(stored, "dark");
      assert.equal(after, "Light mode / aria-pressed=null / hidden=false");
      assert.equal(back, "light");
      assert.equal(window.localStorage.getItem("fisc-theme"), "light");
      assert.deepEqual(events, [true, false]);
    });
  }

  test("an OS that prefers dark opens the toggle on Light mode, and a click stores light", async (t) => {
    const { document, window } = await booted("flows.golden.html", { osDark: true });
    const button = /** @type {HTMLButtonElement} */ (document.getElementById("theme-toggle"));
    const before = read(button);
    button.click();
    t.diagnostic(`on a dark OS with nothing stored the toggle read "${before}"; a click stored "${window.localStorage.getItem("fisc-theme")}"`);
    assert.equal(before, "Light mode / aria-pressed=null / hidden=false");
    assert.equal(document.documentElement.dataset.theme, "light");
    assert.equal(window.localStorage.getItem("fisc-theme"), "light");
  });

  test("the toggle follows an OS switch while nothing is stored", async () => {
    const { document, media } = await booted("index.golden.html");
    const button = /** @type {HTMLButtonElement} */ (document.getElementById("theme-toggle"));
    media.setOSDark(true);
    assert.equal(read(button), "Light mode / aria-pressed=null / hidden=false");
  });

  // Mutation: call addEventListener unguarded before the toggle is unhidden,
  // and boot throws: the toggle stays hidden and follows no OS switch.
  test("a MediaQueryList with only addListener still shows a working toggle that follows the OS", async (t) => {
    const { installBrowser } = await import("./testlib.mjs");
    const { window, document, media } = installBrowser({ html: readFileSync(path.join(TESTDATA, "index.golden.html"), "utf8") });
    const modern = window.matchMedia;
    window.matchMedia = (q) => {
      const m = modern(q);
      return { get matches() { return m.matches; }, media: q, addListener: (fn) => m.addEventListener("change", fn) };
    };
    const mod = await import(MODULE);
    let threw = "";
    try {
      mod.boot(window);
    } catch (e) {
      threw = String(e);
    }
    const button = /** @type {HTMLButtonElement} */ (document.getElementById("theme-toggle"));
    const opened = read(button);
    media.setOSDark(true);
    const followed = read(button);
    button.click();
    t.diagnostic(`boot ${threw ? "threw " + threw : "returned"}; "${opened}", then on a dark OS "${followed}"`);
    assert.deepEqual({ threw, opened, followed, theme: document.documentElement.dataset.theme }, {
      threw: "", opened: "Dark mode / aria-pressed=null / hidden=false",
      followed: "Light mode / aria-pressed=null / hidden=false", theme: "light",
    });
  });

  test("the toggle ships hidden, for a page with JavaScript off", () => {
    for (const page of PAGES) {
      const html = readFileSync(path.join(TESTDATA, page), "utf8");
      assert.ok(html.includes('<button type="button" id="theme-toggle" hidden>'),
        `${page} ships a theme toggle that is not hidden`);
    }
  });
});

// On every pinned page the view list folds behind a menu button naming the
// page on: shipped hidden with the list whole (no script, no fold), shown and
// closed once booted, opened by a click, and closed again by Escape with the
// focus back on the button. Mutation: drop wireMenu's data-menu and the
// stylesheet folds nothing; drop its Escape arm and the last arm goes red.
describe("the narrow-screen menu in every pinned page's top bar", () => {
  for (const page of PAGES) {
    test(page, async () => {
      const shipped = readFileSync(path.join(TESTDATA, page), "utf8");
      assert.match(shipped, /<button type="button" id="nav-toggle" aria-expanded="false" aria-controls="nav-views" hidden>/);
      const { document, window } = await booted(page);
      const nav = document.querySelector("nav.site-nav");
      const button = document.getElementById("nav-toggle");
      const current = document.querySelector('#nav-views [aria-current="page"]');
      assert.ok(current !== null, "no view is marked current");
      assert.equal(button.querySelector(".nav-here").textContent, current.textContent);
      const state = () => `${nav.hasAttribute("data-menu")}/${nav.hasAttribute("data-open")}/${button.getAttribute("aria-expanded")}/${button.hidden}`;
      assert.equal(state(), "true/false/false/false");
      button.click();
      assert.equal(state(), "true/true/true/false");
      document.querySelector("#nav-views a").focus();
      nav.dispatchEvent(new window.KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
      assert.equal(state(), "true/false/false/false");
      assert.equal(document.activeElement, button);
    });
  }
});

// How-to-read prose folds on a narrow screen and opens on a wide one,
// following the window across the breakpoint. jsdom has no media queries, so
// matchMedia is stubbed. Mutation: open every block whatever matches says,
// and the narrow arm goes red.
test("wireHowTo folds the how-to-read prose on a narrow screen and opens it on a wide one", async () => {
  const { JSDOM } = await import("jsdom");
  const mod = await import(MODULE);
  const html = readFileSync(path.join(TESTDATA, "ledger.golden.html"), "utf8");
  const { window } = new JSDOM(html);
  const listeners = [];
  const mq = { matches: true, addEventListener: (_, f) => listeners.push(f) };
  window.matchMedia = () => mq;
  mod.wireHowTo(window);
  const blocks = [...window.document.querySelectorAll("details.how-to-read")];
  assert.ok(blocks.length > 0, "the pinned ledger page folds no prose");
  assert.ok(blocks.every((d) => !d.open), "a block stayed open on a narrow screen");
  mq.matches = false;
  listeners.forEach((f) => f());
  assert.ok(blocks.every((d) => d.open), "a block stayed closed on a wide screen");
});

// The page lede gets "More" only where the clamp cuts it, and a press drops
// the clamp for good. jsdom lays nothing out, so the heights are stubbed.
// Mutation: add the button whatever the heights, and the uncut arm goes red.
test("wireLede offers More only where the lede is cut, and a press shows it whole", async () => {
  const { JSDOM } = await import("jsdom");
  const mod = await import(MODULE);
  const html = readFileSync(path.join(TESTDATA, "ledger.golden.html"), "utf8");
  for (const [scroll, client, want] of [[120, 72, true], [72, 72, false]]) {
    const { window } = new JSDOM(html);
    const lede = window.document.querySelector(".page-head .lede");
    Object.defineProperty(lede, "scrollHeight", { value: scroll });
    Object.defineProperty(lede, "clientHeight", { value: client });
    mod.wireLede(window);
    const more = window.document.querySelector(".lede-more");
    assert.equal(Boolean(more), want, `a lede ${scroll}px in ${client}px`);
    if (more) {
      more.click();
      assert.ok(lede.hasAttribute("data-full") && !more.isConnected);
    }
  }
});
