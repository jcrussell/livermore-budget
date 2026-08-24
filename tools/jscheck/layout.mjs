// layout.mjs — the checks for site/app.js's layout claims.
//
// app.js asserts, in prose, numbers that only a layout can produce: that
// nodeRank plus restackLinks() bring this chart to 195 ribbon crossings and
// $457M of overlapping ribbon, and that restackLinks removes 14 stale-stacked
// pairs on FY2026. Until this file existed nothing in the tree could confirm any
// of it — the numbers were measured once, by hand, out of tree, and quoted
// forever after (fisc-gxa.7).
//
// They are checked against testdata/sankey.golden.json, which is FY2026 — the
// year the comments are about — and is committed, so a number here moves only
// when the graph does.
//
// WHAT CANNOT BE CHECKED, AND IS NOT PRETENDED OTHERWISE. app.js used to quote a
// BEFORE for each figure — "394 ribbon crossings and $1,372M ... become 195 and
// $457M", and "108 under the order this file used to sort by". Those baselines
// describe A SORT ORDER THAT IS NO LONGER IN THE TREE, so no harness can
// reproduce them: the code that produced them was replaced by the code they
// justify. Every ordering that IS still reachable is measured below and pinned,
// and none of them is 394 on either side of the restack. The before-figures in
// app.js were corrected to what reproduces; this file is why that was possible.
//
// EVERY NUMBER THIS FILE PINS IS MEASURED POST-RESTACK, because that is the
// chart a reader sees. Pre-restack figures appear only where a check is about
// what restackLinks itself does.

import { loadApp, goldenGraph } from "./harness.mjs";

/**
 * Lays the golden graph out exactly as render() does, under a given node sort.
 *
 * The constants come out of app.js rather than being repeated here: a chart laid
 * out at a different width has different crossings, so a harness carrying its
 * own copy would drift into checking a chart the page does not draw.
 */
function layout(app, nodeSort) {
  const doc = goldenGraph();
  const sankey = app.d3.sankey()
    .nodeId((d) => d.id)
    .nodeWidth(app.NODE_WIDTH)
    .nodePadding(app.NODE_PADDING)
    .nodeAlign(app.d3.sankeyJustify)
    .extent([[app.LABEL_GUTTER, 12],
             [app.CHART_WIDTH - app.LABEL_GUTTER, app.CHART_HEIGHT - 12]]);
  if (nodeSort !== "d3") sankey.nodeSort(nodeSort);
  return sankey({
    nodes: doc.nodes.map((n) => Object.assign({}, n)),
    links: doc.links.map((l) => Object.assign({}, l, { value: l.value_cents })),
  });
}

/**
 * The links of each band: a band is one pair of adjacent columns.
 *
 * FAILS CLOSED ON A LINK THAT SPANS MORE THAN ONE COLUMN. Every link in this
 * graph joins adjacent columns today, and the crossing count below is only
 * correct while that holds: a ribbon that skipped a column would pass OVER the
 * ribbons in the column it skipped, and comparing it only against links of the
 * same span would miss every one of those crossings and report a smaller,
 * confident, wrong number. The contract has six tiers and the detail schedules
 * are not drawn yet, so this is a real future shape and not a hypothetical.
 */
function bands(graph) {
  const out = new Map();
  for (const l of graph.links) {
    const span = l.target.depth - l.source.depth;
    if (span !== 1) {
      throw new Error(
        `link ${l.source.id} -> ${l.target.id} spans ${span} columns; this ` +
        `harness counts crossings band by band and would undercount it`);
    }
    const key = l.source.depth + ":" + l.target.depth;
    if (!out.has(key)) out.set(key, []);
    out.get(key).push(l);
  }
  return out;
}

/**
 * Crossings and the ribbon value they overlap.
 *
 * Two ribbons in a band cross when the order of their left ends and the order of
 * their right ends disagree — a sign flip. Only same-band pairs can cross,
 * because a ribbon does not leave its band. Overlap is charged at the narrower
 * of the two, which is the most either can hide of the other.
 */
function tangle(graph) {
  let crossings = 0;
  let cents = 0;
  for (const band of bands(graph).values()) {
    for (let i = 0; i < band.length; i++) {
      for (let j = i + 1; j < band.length; j++) {
        const a = band[i], b = band[j];
        if ((a.y0 - b.y0) * (a.y1 - b.y1) < 0) {
          crossings++;
          cents += Math.min(a.value, b.value);
        }
      }
    }
  }
  return { crossings, dollars: cents / 100 };
}

/**
 * Pairs of a node's ribbons stacked in an order its neighbours no longer sit in.
 *
 * This is the defect restackLinks() exists to remove, counted the way its doc
 * comment counts it: d3 sorts a node's links by where the other end sits, then
 * moves nodes again and never re-sorts.
 */
function staleStacked(graph) {
  let pairs = 0;
  for (const node of graph.nodes) {
    for (const [links, end] of [[node.sourceLinks, (l) => l.target],
                                [node.targetLinks, (l) => l.source]]) {
      const wanted = links.slice().sort((a, b) => end(a).y0 - end(b).y0 || a.index - b.index);
      for (let i = 0; i < links.length; i++) {
        for (let j = i + 1; j < links.length; j++) {
          if (wanted.indexOf(links[i]) > wanted.indexOf(links[j])) pairs++;
        }
      }
    }
  }
  return pairs;
}

/** The worst distance any ribbon end falls outside the node face it meets. */
function extentOverflow(graph) {
  let worst = 0;
  for (const node of graph.nodes) {
    for (const [links, y] of [[node.sourceLinks, (l) => l.y0], [node.targetLinks, (l) => l.y1]]) {
      for (const l of links) {
        worst = Math.max(worst, node.y0 - (y(l) - l.width / 2), (y(l) + l.width / 2) - node.y1);
      }
    }
  }
  return worst;
}

// The figures app.js publishes about itself, and the alternatives it publishes
// them against. Every one is PINNED, not bounded: a `>` comparison would let
// the alternatives drift while the check stayed green, which is the exact
// defect this harness exists to remove.
//
// They are also checked against app.js's OWN TEXT below, so editing the comment
// to say something else fails here rather than passing quietly. That is the
// pattern internal/export/export_test.go:623 already uses to pin the client's
// SCHEMA_VERSION literal to the producer's.
const CLAIMED = {
  crossings: 195,
  overlapDollars: 457434169,
  stalePairs: 14,
};

/** Every ordering still reachable in the tree, measured post-restack. */
const ALTERNATIVES = {
  "size descending": { sort: (a, b) => b.value - a.value, crossings: 285, dollars: 966956035 },
  "size ascending": { sort: (a, b) => a.value - b.value, crossings: 285, dollars: 966956035 },
  "input order": { sort: null, crossings: 297, dollars: 1136201320 },
  "d3's own pass": { sort: "d3", crossings: 246, dollars: 556944973 },
};

const usd = (n) => "$" + Math.round(n).toLocaleString();

export function checks() {
  const app = loadApp();
  const byRank = (a, b) => app.nodeRank(a) - app.nodeRank(b) || b.value - a.value;

  const graph = layout(app, byRank);
  const before = { tangle: tangle(graph), stale: staleStacked(graph) };
  const widths = graph.links.map((l) => l.width);
  app.restackLinks(graph);
  const after = { tangle: tangle(graph), stale: staleStacked(graph) };

  const measured = Object.entries(ALTERNATIVES).map(([label, want]) => {
    const g = layout(app, want.sort);
    app.restackLinks(g);
    const t = tangle(g);
    return { label, want, got: t };
  });

  return [
    {
      name: "the chart draws at the 195 crossings app.js claims",
      ok: after.tangle.crossings === CLAIMED.crossings,
      detail: `${after.tangle.crossings} crossings under nodeRank + restackLinks ` +
              `(app.js claims ${CLAIMED.crossings})`,
    },
    {
      name: "the overlapping ribbon is the $457,434,169 app.js claims",
      ok: Math.round(after.tangle.dollars) === CLAIMED.overlapDollars,
      detail: `${usd(after.tangle.dollars)} overlapped (app.js claims ${usd(CLAIMED.overlapDollars)})`,
    },
    {
      name: "restackLinks finds the 14 stale-stacked pairs app.js claims",
      ok: before.stale === CLAIMED.stalePairs,
      detail: `${before.stale} pairs stacked in an order their neighbours no longer ` +
              `sit in (app.js claims ${CLAIMED.stalePairs})`,
    },
    {
      // NOT "after.stale === 0", which is a tautology: staleStacked re-derives
      // the same comparator restackLinks just sorted by, so it cannot fail.
      // What is independent is the CROSSINGS those pairs cost, counted by a
      // different function against ribbon endpoints rather than node order.
      name: "removing those 14 pairs removes exactly 14 crossings",
      ok: before.tangle.crossings - after.tangle.crossings === CLAIMED.stalePairs,
      detail: `${before.tangle.crossings} crossings before restacking, ` +
              `${after.tangle.crossings} after: ` +
              `${before.tangle.crossings - after.tangle.crossings} removed`,
    },
    {
      name: "every ordering still in the tree measures where app.js says it does",
      ok: measured.every((m) => m.got.crossings === m.want.crossings &&
                                Math.round(m.got.dollars) === m.want.dollars),
      detail: measured.map((m) => `${m.label} ${m.got.crossings}/${usd(m.got.dollars)}`).join(", ") +
              `, nodeRank ${after.tangle.crossings}/${usd(after.tangle.dollars)}`,
    },
    {
      name: "nodeRank beats every one of them",
      ok: measured.every((m) => m.got.crossings > after.tangle.crossings &&
                                m.got.dollars > after.tangle.dollars),
      detail: `best alternative ${Math.min(...measured.map((m) => m.got.crossings))} crossings, ` +
              `nodeRank ${after.tangle.crossings}`,
    },
    {
      // Matched as WHOLE QUOTED PHRASES, not as bare substrings. A bare
      // `source.includes("14")` matches `const NODE_WIDTH = 14;` and a bare
      // `includes("195")` is satisfied by any one of the several places 195
      // appears, so both let the sentence they were meant to pin drift to a
      // wrong number while staying green. Each phrase below is the figure
      // together with enough of its own sentence to be unique.
      name: "app.js quotes the figures it actually produces",
      ok: (() => {
        const phrases = [
          `comes to ${CLAIMED.crossings} ribbon`,
          `$${CLAIMED.overlapDollars.toLocaleString("en-US")} of overlapping ribbon`,
          `down to ${CLAIMED.crossings} ribbon crossings`,
          `${CLAIMED.stalePairs} of them on FY2026`,
          `${ALTERNATIVES["size descending"].crossings} and`,
          `${ALTERNATIVES["input order"].crossings} under the input order`,
          `${ALTERNATIVES["d3's own pass"].crossings} under`,
          `$${ALTERNATIVES["size descending"].dollars.toLocaleString("en-US")} under a sort by size`,
        ];
        const missing = phrases.filter((q) => !app.source.includes(q));
        return missing.length === 0;
      })(),
      detail: "every figure pinned here appears in site/app.js's own comments in " +
              "its own sentence, so editing one without re-measuring fails",
    },
    {
      name: "the fund column is pinned to the palette's order",
      ok: (() => {
        const drawn = graph.nodes.filter(app.isFundGroup)
          .sort((a, b) => a.y0 - b.y0).map((n) => n.id);
        return JSON.stringify(drawn) ===
               JSON.stringify(app.FUND_ORDER.filter((f) => drawn.includes(f)));
      })(),
      detail: "fund groups run top to bottom in FUND_ORDER, which is what " +
              "supplying .nodeSort() at all is for",
    },
    {
      name: "restackLinks moves ribbons without resizing them",
      ok: graph.links.every((l, i) => l.width === widths[i]),
      detail: "every ribbon keeps the width the layout gave it",
    },
    {
      name: "no ribbon overflows the node face it meets",
      ok: extentOverflow(graph) <= 1e-9,
      detail: `worst overflow ${extentOverflow(graph).toFixed(6)}px`,
    },
    {
      name: "the client refuses a document it does not understand",
      ok: app.understands(app.SCHEMA_VERSION, "x") === true &&
          app.understands(app.SCHEMA_VERSION + 1, "x") === false &&
          app.understands(app.SCHEMA_VERSION - 1, "x") === false,
      detail: `schema_version ${app.SCHEMA_VERSION} is accepted and its neighbours are refused`,
    },
  ];
}
