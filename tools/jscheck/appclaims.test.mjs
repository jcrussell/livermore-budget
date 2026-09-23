// appclaims.test.mjs — the figures site/app.js quotes about the views the drill
// opens, re-derived from the drill tree.
//
// WHY THESE ARE HERE AND NOT IN drill.mjs. drill.mjs pins what the DRILL does;
// these are sentences app.js writes about that walk, in comments beside the
// gestures they justify -- how many views open, how many draw a folded tail,
// how many columns each draws, and what share of the marks on them open. They
// were quoted and pinned by nothing, so all four drifted: the view count said
// 44 where the tree opens 76 and 74, the node count 343 where it draws 453 and
// 446, the openable count 24 where it is 55 and 53, and the claim that no view
// draws two columns was false of transfers/in in both budgets (fisc-0lnb).
//
// EACH PHRASE HAS TO FIT ON ONE COMMENT LINE, which is a constraint on the
// PROSE rather than on this file: source.includes() cannot see a phrase a line
// break runs through, so a sentence that wraps mid-figure is unpinnable and
// this goes red until it is reflowed. That is the right direction to fail in.
//
// THE SHARE IS ASSERTED AS THE ROUNDED PERCENT APP.JS PRINTS, not as the ratio,
// because the sentence a reader meets is "12% ... and 88%". A ratio that
// rounded differently would leave the sentence wrong and this green.
//
// EVERY FIGURE IS ALSO GREPPED FOR IN app.js's OWN SOURCE, which is what makes
// this a pin rather than a second measurement sitting beside an unchecked
// sentence: re-measuring here and leaving the comment alone would be green.
// The phrases carry enough of their own sentence to be unique, which is
// layout.test.mjs's rule and for its reason -- a bare includes("76") matches
// any of several places 76 appears.

import { describe, test, before } from "node:test";
import assert from "node:assert/strict";

import { here } from "./harness.mjs";
import { openedAsShipped, everyOpenedView, COLUMNS } from "./drill.mjs";

/**
 * What one budget's drill tree draws, walked once.
 *
 * AT THE PAGE'S OWN BUDGET AND AT FOUR, because app.js's sentence quotes both
 * and they differ: a widened step buys one view a fourth column and leaves
 * every other view alone.
 */
async function walk(column, budget) {
  const app = await openedAsShipped([], column);
  app.setColumnBudget(budget);
  let views = 0;
  let nodes = 0;
  let opens = 0;
  let withTail = 0;
  /** @type {Map<number, number>} */
  const columns = new Map();
  const refused = await everyOpenedView(app, () => {
    views++;
    const laid = app.layOut(app.projection);
    nodes += laid.nodes.length;
    opens += laid.nodes.filter((/** @type {any} */ d) => app.drillable(d)).length;
    if (laid.nodes.some((/** @type {any} */ d) => app.expandable(d))) withTail++;
    const c = new Set(laid.nodes.map((/** @type {any} */ d) => app.columnOf(d))).size;
    columns.set(c, (columns.get(c) || 0) + 1);
  });
  return { app, views, nodes, opens, withTail, refused: refused.refused,
    columns: [...columns.entries()].sort((a, b) => a[0] - b[0]) };
}

// What the tree draws, measured 2026-09-23 and quoted in site/app.js's own
// comments beside nodeClass, nodeFlags and the click wiring.
const DRAWN = {
  "FY 2025-26": { views: 76, nodes: 453, opens: 55, withTail: 9,
    atThree: [[2, 1], [3, 75]], atFour: [[2, 1], [3, 74], [4, 1]] },
  "FY 2026-27": { views: 74, nodes: 446, opens: 53, withTail: 9,
    atThree: [[2, 1], [3, 73]], atFour: [[2, 1], [3, 72], [4, 1]] },
};

for (const col of COLUMNS) {
  describe(`${col.label}: the figures app.js quotes about the views the drill opens`, () => {
    /** @type {any} */ let three, four, want;
    before(async () => {
      three = await walk(col, 3);
      four = await walk(col, 4);
      want = DRAWN[col.label];
    });

    test("the view, node and openable counts are the ones app.js prints", (t) => {
      const detail = `${three.views} view(s) drawing ${three.nodes} mark(s), ` +
        `${three.opens} of which open, and ${three.withTail} view(s) draw a folded tail ` +
        `(app.js says ${want.views}/${want.nodes}/${want.opens}/${want.withTail})`;
      t.diagnostic(detail);
      assert.deepEqual(here({ refused: three.refused, views: three.views, nodes: three.nodes,
        opens: three.opens, withTail: three.withTail }),
        { refused: "", views: want.views, nodes: want.nodes, opens: want.opens,
          withTail: want.withTail }, detail);
    });

    // NOT "every view draws three": that is the claim this test exists because
    // app.js made falsely. A step that keeps no flank is a FILTER and draws the
    // opened node's parts alone, so the histogram is the shape to assert --
    // a count of three-column views would go green on a tree that had stopped
    // drawing the two-column one at all.
    test("the column-count histogram is the one app.js describes, at both budgets", (t) => {
      const detail = `at the page's own budget ${JSON.stringify(three.columns)}; ` +
        `at four ${JSON.stringify(four.columns)} ` +
        `(app.js says ${JSON.stringify(want.atThree)} and ${JSON.stringify(want.atFour)})`;
      t.diagnostic(detail);
      assert.deepEqual(here({ three: three.columns, four: four.columns }),
        { three: want.atThree, four: want.atFour }, detail);
    });

    test("app.js's own sentences carry these figures", (t) => {
      const share = Math.round(three.opens / three.nodes * 100);
      const year = col.label.replace(" ", "").replace("FY", "FY");
      const phrases = [
        // Each carries enough of its own sentence to name ONE budget, so a
        // figure edited under the other year does not satisfy it.
        `${three.withTail} of the ${three.views} views ${year} opens`,
        col.label === COLUMNS[0].label
          ? `Of the ${three.nodes} marks those`
          : `on ${year}, ${three.opens} of ${three.nodes}`,
        `${share}% of an opened chart's marks`,
      ];
      const missing = phrases.filter((q) => !three.app.source.includes(q));
      const detail = `${phrases.length} phrase(s) sought in site/app.js's own comments; ` +
        (missing.length ? `MISSING ${JSON.stringify(missing)}` : "each present in its own sentence");
      t.diagnostic(detail);
      assert.deepEqual(here(missing), [], detail);
    });
  });
}
