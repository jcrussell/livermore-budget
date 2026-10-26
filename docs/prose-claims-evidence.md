# What false text in this repository has cost

> Evidence for AGENTS.md, "Before you quote a number" and AGENTS.md, "Where
> writing goes". This file states no rule. The rules are in AGENTS.md; what is
> here is what was counted, and when.

A false claim in committed text is one subject whether the text is a comment, a
doc or an injected memory. PROSE-NEW and PROSE-PRE-EXISTING together are 32 of
the 115 findings counted in
[`review-loop-evidence.md`](review-loop-evidence.md).

## One fact in three files is three findings

The badge lane found the same wrong count — *"the other six fund groups"* —
three times, in `internal/project/fundflows.go`, then `pkg/cmd/export/data.go`'s
lede, then `docs/outputs/fund-flows.md`, across three *consecutive*
passes, each of which believed it had finished. The caveats lane's *"250 words"*
was false in six places at once.

It is grep-then-**read**, not grep-then-replace: `grep -rn "six fund groups"`
also returns ten hits that encode a different and largely true proposition — how
many groups the spine prints, not how many stop at their funds. The same numeral
standing for two claims is why this cannot be a check. `fisc-xbd4`.

## The gate line's own count is the trap

Three consecutive review passes corrected it and each correction was wrong in a
new way: the template shipped stale; the fix that replaced it quoted
`check.All()`'s total instead, which is a different quantity, because a vacuous
check and a `--full`-only check are both in that total and neither is in the
"passed" line; and the fix that replaced THAT asserted an exhaustive list of
every count the log has carried, measured with a grep narrow enough to miss
several spellings of the same line.

So no list of past counts is recorded anywhere. One session reported "38 passed"
after landing a check that made it 39, from a binary built before the check
existed.

## A scripted edit that aborts halfway leaves a tree that passes

`261c78f`'s rewording of a citation and a README cell never landed: a batch of
scripted edits with an assertion in the middle aborted at the second, and the
later edits never ran. The tree still built and passed every test, so no gate
went red; the fifth review pass found both unchanged.

An unmade edit is the same class as writing "filed as a bead" without filing one
— which has happened twice, both claims false until a later pass caught them.
Both read as *done*, or as *tracked*, so nobody goes looking.

## What `make beadrefs` does not cover

Its reach is two hand-maintained lists — the paths, in the Makefile, and the file
extensions picked up while walking one of them, in `scannable`. A listed path
that has gone from the tree fails the run; a path **deleted from the list** just
makes the scan quietly smaller, and neither list can know about an entry that was
never added. That is how `site/app.js` and then `site/style.css` were each
unchecked for a while after it landed.

## Comments that outlived their rule

One session found four: a required-key list naming keys the caller no longer
dereferenced; a parse error reading "row %q has neither category nor department"
that would fire on a row which *had* a department; a guard message describing a
check its condition could not perform; and a summary sentence that went false one
commit after it was pinned.

Two doc comments were orphaned in a single commit by code inserted between them
and their declaration: a new helper orphaned `foldDocument`'s JSDoc, costing that
function its `@param` under `// @ts-check` and leaving its body unchecked, and a
new test orphaned the rationale belonging to the test below it. Both read as
correct in the diff and were wrong in the file.

## What narration costs, and why it is not a sweep

`fisc-pm8f` priced the rewrite: **42.5 minutes and 411k tokens for 148 lines** of
`internal/geom`, which extrapolates to ~90 hours over the tree and grew the file
it touched by 18%. Verification cost attaches to claims written or kept, so a
deletion is the cheap half and a restatement re-imports the whole cost.

Two measurements decide the sweep question, both taken at `a14b6c2`. A sweep
would reach nothing an ordinary lane does not: all 62 files carrying narration
were touched in the last 100 commits — no cold files — and 40 consecutive commits
reach 71% of the narrated lines. And the quantity is small: 185 of 19,137 comment
lines carry a history marker, about 1%. `fisc-a3rp`'s ~3,000 is a count of every
line of every *block* one of those sentences sits in, which is what you must
READ, not what you would delete; sizing the work off it overstates it by an order
of magnitude.

Reclassifying the false-text findings by *cause* gives
restating-a-rule-that-changed-elsewhere 35%, never-true-when-written 23%,
quoted-number-drifted 22%, history 15% — so the narrative register is the carrier
and not the cause, and the hypothesis that prompted the audit came back mostly
wrong. Measured at `823de73`, history-marker comment lines are 247 of 18,746.

## The roadmap goes stale faster than anything else

Every enumeration of the epics written into AGENTS.md has gone stale: one said
eleven epics and four open; its replacement said twelve and five, and named an
epic that had since closed while never having heard of the one that replaced it.

`fisc-9hf` closed on 2026-08-30 and took `fisc-1wr.2` with it, and `E5` did not
become workable: measured 2026-09-03, its ladder was downstream of two things and
only one had moved. `fisc-bau` sat near the top of `bd ready` from 18 to 30
August while its own note said *"NOT ON THE CRITICAL PATH"*, and a memory had
already recorded the contradiction.

Measured at `44be60d`, when there were 52 injected memories: **ten** carried a
line whose only job is to say earlier text has gone stale. The log carries the
same habit and there the count can be re-measured:
`git log --format=%s | grep -ci stale` counted six at `44be60d` — a floor, not
the answer, since the grep misses others of the same class whose subjects never
use the word (`a76a8dc`'s "corrects six beads", `f6e4d00`'s "correct two wrong
premises" among them), so a plain-meaning count is more like 13-15.
