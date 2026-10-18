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
  assert.deepEqual(Object.keys(mod).sort(), ["THEME_EVENT", "THEME_KEY", "boot", "prefersDark", "syncTheme"]);
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

/** The toggle's state as a reader meets it, as one string. */
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
      assert.equal(before, "Dark mode / aria-pressed=false / hidden=false");
      assert.equal(theme, "dark");
      assert.equal(stored, "dark");
      assert.equal(after, "Light mode / aria-pressed=true / hidden=false");
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
    assert.equal(before, "Light mode / aria-pressed=true / hidden=false");
    assert.equal(document.documentElement.dataset.theme, "light");
    assert.equal(window.localStorage.getItem("fisc-theme"), "light");
  });

  test("the toggle follows an OS switch while nothing is stored", async () => {
    const { document, media } = await booted("index.golden.html");
    const button = /** @type {HTMLButtonElement} */ (document.getElementById("theme-toggle"));
    media.setOSDark(true);
    assert.equal(read(button), "Light mode / aria-pressed=true / hidden=false");
  });

  test("the toggle ships hidden, for a page with JavaScript off", () => {
    for (const page of PAGES) {
      const html = readFileSync(path.join(TESTDATA, page), "utf8");
      assert.ok(html.includes('<button type="button" id="theme-toggle" aria-pressed="false" hidden>'),
        `${page} ships a theme toggle that is not hidden`);
    }
  });
});
