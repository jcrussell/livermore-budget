# What the review loop measured

> Evidence for AGENTS.md, "Review is a loop, not a pass". This file states no
> rule. The rules are in AGENTS.md; what is here is what was counted, and when.

## The loop is its own second-largest defect source

All **115** findings of the three 2026-08-31 lanes, classified one at a time
from the commit bodies, with provenance settled by `git log -S` wherever a
message does not say where a defect came from.

**Start with the number the session got wrong about itself.** `fisc-yj4w`'s
notes report *"DEFECTS INTRODUCED BY AN EARLIER PASS'S OWN FIX, by lane:
caveats 2 of 33, drill 3 of 48, badge 3 of 29"* — eight. Per finding it is
**27**:

| lane | as recorded | measured per finding |
|---|---|---|
| caveats | 2 of 33 | **6** of 37 |
| drill | 3 of 48 | **12** of 49 |
| badge | 3 of 29 | **9** of 29 |

The recorded figures count *passes that contained* such a finding. The
undercount was not fixable by reading harder: **only one of the caveats lane's
six is labelled as introduced-by-fix in its own commit message**; the rest are
recoverable only from the diffs.

### Where the 115 lived

| | count | share |
|---|---|---|
| NEW-CODE — the lane's own feature commits | 39 | 34% |
| **INTRODUCED-BY-FIX — an earlier pass in the same lane** | **27** | **23%** |
| PROSE-NEW — a false claim in text this lane wrote | 20 | 17% |
| TEST-GAP — no guard, or a guard that could not fail | 15 | 13% |
| PROSE-PRE-EXISTING — older text the lane silently falsified | 12 | 10% |
| **PRE-EXISTING code** | **1** | **<1%** |
| classified in two rows at once | 1 | |

### What kind they were

| | caveats | drill | badge | total |
|---|---|---|---|---|
| WRONG-OUTPUT | 4 | 22 | 14 | 40 |
| FAIL-OPEN | 11 | 12 | 6 | 29 |
| FALSE-CLAIM | 19 | 8 | 5 | 32 |
| DESIGN | 2 | 4 | 3 | 9 |
| HYGIENE | 1 | 3 | 1 | 5 |

27 against 39 from the feature commits, and against **one** pre-existing code
defect in 115. Whatever else these passes were doing, auditing the tree was not
it — they reviewed the change, and then reviewed themselves. The share also
**rises with pass number**: in the drill lane, findings attributed to an earlier
pass's own fix run 1, 3, 3, **5** across passes two to five; three of the badge
lane's fifth-pass five are the loop's own work.

## A pass has never returned nothing

`git log --format='%s' | grep -oiE '[a-z]+ findings'` over the whole log. The
grep over-collects, catching subjects that carry the word without a yield.

**The command and the number it was published with do not match, and that is
worth more than either.** AGENTS.md carried "92 passes and 472 findings, measured
at `ab71b7a`"; running the line above *at* `ab71b7a` returns **94**. Whatever
produced 92 was not this command, and nothing recorded the difference — which is
the quoted-number-drift this file is partly about. Re-run it and read what it
says; do not quote this paragraph.

| yield | 2 | 3 | 4 | 5 | 6 | 7 | 8 | 9 | 11 | 12 | 13 |
|---|---|---|---|---|---|---|---|---|---|---|---|
| passes | 8 | 10 | 27 | 15 | 7 | 14 | 5 | 3 | 1 | 1 | 1 |

Zero has never occurred and neither has one, over four months and every lane the
log records. Yield is close to independent of how much code the pass is reading:
the caveats lane's first pass read 1,812 insertions and returned 9, its fourth
read a 69-line fix and returned 5.

The distribution table above stays pinned at `ab71b7a`, because it is a
snapshot; the totals move with every lane.

## The fixes are the size of the feature

Insertions, `.beads` excluded:

| lane | feature | five fix passes | ratio | yields |
|---|---|---|---|---|
| caveats | 1,812 | 729 | 40% | 9, 5, 7, 5, 7 |
| drill | 1,069 | 1,246 | **117%** | 12, 13, 7, 7, 9 |
| badge | **200** | 896 | **448%** | 8, 7, 5, 4, 5 |

Against the lanes that *did* decay: fund-balance's fifth was 16 insertions and
yielded 2, the guards lane's fourth was 26 and yielded 2. The three lanes above
never shrank — their fifth-pass fixes were 74, 135 and 191, 400 lines in all, 43
of them in `site/app.js`, which readers are served verbatim, and nothing read any
of them (`fisc-i92i`).

## Four shapes an earlier pass's own fix takes

Each found by the pass after the one that wrote it.

| the fix | what the next pass found in it |
|---|---|
| `HasSuffix(base, "/")`, itself the fix for a string-prefix bug | still accepted an ancestor base: `facts/` passed, and the client composed `facts/p0066.jsonl` |
| a reworded `fact.FromValues` comment, claiming a backstop | the condition it tested could not reach the shape it claimed to catch |
| two commits saying "filed as a bead" | neither bead existed |
| a test written for a summary defect, asserting `Contains("78 declared fund(s)")` | the malformed string it was written for satisfies that substring, so it passed on the bug |

Three more shapes, these in the original work rather than in a fix: a guard
written as fail-closed that shipped **fail-open**; a test that passed identically
with and without the fix it was named for; and a map read with no ok-check, which
left the one test guarding against a wrong-column read unable to fail on it.

**A false claim is worse when the program prints it.** On one lane, two of the
false statements a late pass found were strings `fisc verify` emits on every run,
and one of those cited a **derived** figure as a printed one — three commits
after the test beside it said in as many words that the figure appears on no page
in the corpus.

## Planning is not where this comes from

DESIGN is 9 of 115. `fisc-yj4w.7` declines two alternative tier sets with
measured layout numbers, names the file and line of every seam it will touch,
and records in its close reason the two premises that turned out wrong — both
caught while implementing, not by any review pass.

What the passes found instead was code shipped without its guard: `bd0a098`
added 492 lines of `site/app.js` and 86 lines of `tools/jscheck`, **none of them
covering the drill it had just written**, and the drill's whole check module was
written *inside* the review passes (`git log --diff-filter=A --
tools/jscheck/drill.mjs` → `a3fe8e8`). Across the session's sixteen review-fix
commits `drill.mjs` took **633** insertions against `app.js`'s 503 — the largest
single sink of fix churn there was.

Worse, checks written under review pressure to close the previous pass's finding
were the weakest in the lane: a drill guard that measured the chart already on
screen from the second node on, a baseline that never asserted the drill
happened, a share check that fetched the wrong fiscal year's golden, and a
selector entry malformed so that it *"printed as declared and could not fail"*.
Four checks written to close findings, four that could not fail. `fisc-rx1d`.

## The late passes are the ones worth having

On the ACFR tolerance lane a second-pass reviewer read two files that disagreed
about a figure and said to make them agree; the wrong one was corrected, because
an intermediate draft had quietly redefined the term both were counting ("the
page's four blocks" → "the four blocks this file maps"). The fourth pass found
it.

That lane ran the full five and went 11, 8, 6, 6, 4, and exactly one of its 35
findings was fail-open — an unguarded `abs(diff)*2` in the new tolerance that
wrapped negative and accepted any discrepancy. It survived four passes. The other
34 were false text, missing tests, or refusals that were too strict.

## What review cannot see

The highest-risk claims here are empirical, not structural: that the amount
parser rejects the right tokens, that extraction is byte-deterministic, that
mapped rows sum to the totals the documents print, that a locator still resolves
to the row it was written against. Reading a diff confirms none of them.

The worked example of the standard those need instead: rejecting a leading minus
sign in `internal/amount` is justified by summing ACFR p177 row 2017 and showing
that only the positive reading reconciles to the printed total.
`TestLeadingMinusIsReallyPositive` carries that arithmetic;
`internal/mapping/acfr_p177_test.go` reconciles the schedule against the city's
own printed totals — nine of its ten rows tie exactly and FY2024 is short by
$176,292, which is exactly that row's own Financed Purchases column, so the test
is named `TestACFRDebtScheduleTiesExceptOneRow` rather than pretending otherwise.

## Regenerating a golden

`testdata/sankey.golden.json` was regenerated once, to add a key, and the diff
was checkable because `fixture_test.go`'s `spinePage` is a two-branch rule —
general and enterprise on p66, everything else on p67 — so all 58 added blocks
could be verified by hand.

## Green because the gate fired, not because the defect was prevented

`tools/jscheck`'s `twoYearConfig` carried `docs: {}`. `citations()` opens
`const doc = CONFIG.docs[source.doc_id]; if (!doc) continue` — so it returned
before ever reaching `source.pages`, and **every** required-key check for a
`[].pages` key was passing because `drawableSankey` rejected the document, never
because a throw had been prevented. With `docs` populated, deleting the guard
gives `TypeError: source.pages is not iterable` and a flow table at 0 rows
against 58 — a half-repainted page at the reader.

At least the third occurrence. `harness.mjs` records two more in its own
comments: modelling ids and not attributes made a check unfalsifiable, and
*"deleting `group.removeAttribute("disabled")` from app.js left the whole suite
green."*
