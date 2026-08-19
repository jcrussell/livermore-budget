#!/usr/bin/env python3
"""Compute the Sankey's vertical node order by exact crossing minimisation.

Prints the SOURCE_ORDER and USE_ORDER constants that site/app.js commits, and
the crossing table quoted in the comment above them. Run it after a data
change; if its output and the committed constants disagree, the constants are
stale.

    python3 tools/sankey_order.py dist/data/sankey.json

Stdlib only, and outside the Go build path on purpose: `make build`, `make
test` and `fisc verify` must not acquire a Python dependency. This is not a
build step, it is the derivation of a constant -- the same standing as the
colour-adjacency measurement that produced FUND_ORDER.

WHY THE ORDER IS A CONSTANT AT ALL. site/app.js supplies d3-sankey a
.nodeSort(), and in v0.12.3 that gates off the re-sort which would otherwise
reorder a column during relaxation (`void 0===e && i.sort(u)` in the bundle).
The barycentre sweeps still nudge nodes vertically; they can never swap two of
them. So the app owns the order outright, and it may as well own a good one.
It cannot simply drop .nodeSort() and let d3 choose, because the fund column's
order is a palette decision: which two hues touch at a node is decided by which
two funds are adjacent in that column.

WHAT IS OPTIMISED. With the fund column held fixed, each outer column is an
independent one-sided crossing minimisation -- exactly solvable by a Held-Karp
DP over subsets, and with n <= 12 that is 4096 states. These are optima, not
heuristic orderings. Two quantities are minimised, in that order:

    crossings   how many pairs of ribbons cross
    ink         the cents of overlap, sum of min(value_a, value_b) over them

Crossings lead because a crossing is what the eye reports; ink breaks ties
because it is what the eye reports it as, and two ribbons worth tens of
millions colliding is not the same event as two hairlines doing it. The
tie-break is not decoration: on FY2026 many orders reach the minimum 177
crossings, and the one that ignores ink spills $667M of overlap where the one
that breaks on it spills $491M. Minimising ink alone instead gets to $455M but
at 193 crossings, which is the worse of the two trades. All three numbers are
in the table this prints.

WHAT IS NOT OPTIMISED. FUND_ORDER, the middle column, is an input. Freeing it
too is worth 9 crossings out of 177 -- run with --free-funds to confirm --
which is not a trade worth making against a measured colour result.

WHAT THE COUNTS ASSUME. That each node hands its ribbons out in the order of
the ends they run to, so that two links sharing a node never cross. d3-sankey
sorts them that way and then moves nodes once more without re-sorting, which
on FY2026 leaves 19 pairs crossing at a node face for no reason in the data;
site/app.js undoes that in restackLinks() before drawing. This script counts
the crossings that ordering is responsible for, and the browser only agrees
with it because of that function.
"""

from __future__ import annotations

import argparse
import itertools
import json
import sys

# The fund column, top to bottom. site/app.js owns this ordering -- it is a
# colour-adjacency result, not a layout one -- and this copy exists only so the
# search has a fixed middle layer to solve against. Keep the two in step.
FUND_ORDER = [
    "fund-group/internal-service",
    "fund-group/capital",
    "fund-group/general",
    "fund-group/special-revenue",
    "fund-group/enterprise",
    "fund-group/debt-service",
]

# One crossing costs COUNT_WEIGHT + the cents of ink it spills, which makes a
# single integer behave lexicographically: no reachable amount of ink can pay
# for one extra crossing. The ceiling that has to clear is every pair of links
# overlapping at its thinner end, well under $10^13 for a graph this size, and
# assert_lexicographic() checks the margin rather than trusting the arithmetic.
COUNT_WEIGHT = 10**18


def load(path):
    with open(path, encoding="utf-8") as handle:
        doc = json.load(handle)
    nodes = {n["id"]: n for n in doc["nodes"]}
    links = [(l["source"], l["target"], l["value_cents"]) for l in doc["links"]]
    return nodes, links


def columns(nodes, links, funds):
    """Split the graph into (sources, uses) by adjacency to the fund column.

    Derived from the links rather than from the JSON's `tier` field: tier is a
    place in the city's hierarchy, and the projection emits 0, 2 and 5 for what
    are drawn as three adjacent columns. What a layout needs is the columns.
    """
    fund_set = set(funds)
    sources, uses = set(), set()
    for source, target, _ in links:
        if source in fund_set and target in fund_set:
            # Checked before the two branches below, which would otherwise
            # file a fund group as a source and hand app.js a SOURCE_ORDER
            # with a fund-column node in it.
            sys.exit("link %s -> %s joins two fund groups, which is no column" % (source, target))
        if target in fund_set:
            sources.add(source)
        elif source in fund_set:
            uses.add(target)
        else:
            sys.exit("link %s -> %s touches no fund group" % (source, target))
    both = sources & uses
    if both:
        sys.exit("node(s) in two columns, which this DP cannot order: %s" % sorted(both))
    stranded = set(nodes) - sources - uses - fund_set
    if stranded:
        sys.exit("node(s) in no column: %s" % sorted(stranded))
    return sorted(sources), sorted(uses)


def neighbours(nodes, links, funds, side):
    """For each node, the fixed-layer positions it reaches, with their values."""
    at = {fund: i for i, fund in enumerate(funds)}
    out = {node: [] for node in nodes}
    for source, target, value in links:
        if side == "source" and source in out and target in at:
            out[source].append((at[target], value))
        if side == "use" and target in out and source in at:
            out[target].append((at[source], value))
    return out


def solve(nodes, links, funds, side, count_weight=COUNT_WEIGHT, ink_weight=1):
    """Exact one-sided crossing minimisation. Returns the ordered node ids.

    cost[i][j] is what it costs to draw i above j: every pair of their links
    whose fund-column endpoints run the other way is a crossing. That matrix
    makes the ordering a shortest Hamiltonian path, and the subset DP finds it.
    """
    adjacent = neighbours(nodes, links, funds, side)
    n = len(nodes)
    cost = [[0] * n for _ in range(n)]
    for i, above in enumerate(nodes):
        for j, below in enumerate(nodes):
            if i == j:
                continue
            cost[i][j] = sum(
                count_weight + ink_weight * min(va, vb)
                for pa, va in adjacent[above]
                for pb, vb in adjacent[below]
                if pa > pb
            )

    best = [None] * (1 << n)
    came_from = [-1] * (1 << n)
    best[0] = 0
    for placed in range(1 << n):
        if best[placed] is None:
            continue
        for j in range(n):
            if placed >> j & 1:
                continue
            # j is drawn below everything already placed, so it pays each.
            total = best[placed] + sum(cost[i][j] for i in range(n) if placed >> i & 1)
            below = placed | (1 << j)
            if best[below] is None or total < best[below]:
                best[below] = total
                came_from[below] = j

    order, placed = [], (1 << n) - 1
    while placed:
        j = came_from[placed]
        order.append(nodes[j])
        placed ^= 1 << j
    order.reverse()
    return order


def score(source_order, use_order, links, funds):
    """(crossings, cents of ink) for a drawn order. Ink is the overlap."""
    at = {fund: i for i, fund in enumerate(funds)}
    at.update({node: i for i, node in enumerate(source_order)})
    at.update({node: i for i, node in enumerate(use_order)})
    sources = set(source_order)
    left = [(at[s], at[t], v) for s, t, v in links if s in sources]
    right = [(at[s], at[t], v) for s, t, v in links if s not in sources]
    crossings = ink = 0
    for band in (left, right):
        for (a0, a1, va), (b0, b1, vb) in itertools.combinations(band, 2):
            if (a0 - b0) * (a1 - b1) < 0:
                crossings += 1
                ink += min(va, vb)
    return crossings, ink


def assert_lexicographic(links):
    """Every crossing at once still cannot outbid one crossing's COUNT_WEIGHT."""
    values = sorted((value for _, _, value in links), reverse=True)
    ceiling = sum(value * i for i, value in enumerate(values))
    if ceiling >= COUNT_WEIGHT:
        sys.exit(
            "COUNT_WEIGHT %d no longer dominates this graph's ink ceiling %d"
            % (COUNT_WEIGHT, ceiling)
        )


def order_before_this_change(nodes, links, nodes_by_id):
    """The rule site/app.js used before the measured constants replaced it.

    Kept so that the 286 crossings the app's comment cites stays derivable
    rather than remembered: rank transfer endpoints below the categories and
    fund-balance endpoints below those, then descending value.
    """
    value = {node: 0 for node in nodes}
    for source, target, cents in links:
        if source in value:
            value[source] += cents
        if target in value:
            value[target] += cents

    def rank(node):
        role = nodes_by_id[node]["role"]
        if role in ("transfer_in", "transfer_out"):
            return 100
        if role.startswith("fund_balance") or role == "reserve_increase":
            return 200
        return 0

    return sorted(nodes, key=lambda node: (rank(node), -value[node], node))


def as_js(name, order, nodes_by_id):
    column = max(len(node) for node in order) + 3
    lines = ["const %s = [" % name]
    for node in order:
        pad = " " * (column - len(node))
        lines.append('  "%s",%s// %s' % (node, pad, nodes_by_id[node]["label"]))
    lines.append("];")
    return "\n".join(lines)


def main():
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("projection", nargs="?", default="dist/data/sankey.json")
    parser.add_argument(
        "--free-funds",
        action="store_true",
        help="also search the fund column, to price FUND_ORDER against layout",
    )
    args = parser.parse_args()

    nodes_by_id, links = load(args.projection)
    assert_lexicographic(links)
    funds = [fund for fund in FUND_ORDER if fund in nodes_by_id]
    if len(funds) != len(FUND_ORDER):
        sys.exit("FUND_ORDER names a fund group the projection does not have")
    sources, uses = columns(nodes_by_id, links, funds)

    print(as_js("SOURCE_ORDER", solve(sources, links, funds, "source"), nodes_by_id))
    print()
    print(as_js("USE_ORDER", solve(uses, links, funds, "use"), nodes_by_id))
    print()

    # The same search, with one of the two terms switched off in each case.
    rows = [
        (
            "before this change (rank, value)",
            order_before_this_change(sources, links, nodes_by_id),
            order_before_this_change(uses, links, nodes_by_id),
        ),
        (
            "fewest crossings only",
            solve(sources, links, funds, "source", count_weight=1, ink_weight=0),
            solve(uses, links, funds, "use", count_weight=1, ink_weight=0),
        ),
        (
            "least ink only",
            solve(sources, links, funds, "source", count_weight=0, ink_weight=1),
            solve(uses, links, funds, "use", count_weight=0, ink_weight=1),
        ),
        (
            "crossings, ties on ink (committed)",
            solve(sources, links, funds, "source"),
            solve(uses, links, funds, "use"),
        ),
    ]
    print("%-38s %9s %9s" % ("objective", "crossings", "ink"))
    for name, source_order, use_order in rows:
        crossings, ink = score(source_order, use_order, links, funds)
        print("%-38s %9d %9s" % (name, crossings, "$%.0fM" % (ink / 100 / 1e6)))

    if args.free_funds:
        best = None
        for permutation in itertools.permutations(funds):
            column = list(permutation)
            result = score(
                solve(sources, links, column, "source"),
                solve(uses, links, column, "use"),
                links,
                column,
            )
            if best is None or result < best[0]:
                best = (result, column)
        crossings, ink = best[0]
        print()
        print("fund column freed: %d crossings, $%.0fM ink, in the order" % (crossings, ink / 100 / 1e6))
        print("  " + " | ".join(nodes_by_id[fund]["label"] for fund in best[1]))


if __name__ == "__main__":
    main()
