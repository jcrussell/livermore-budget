# Agent Instructions

**This file is the whole of it.** `CLAUDE.md` imports it and adds nothing;
`docs/agents/` no longer exists. One file, because three files carrying
overlapping copies of the same rules is how the copies drift — the review-loop
rule lived in three places and had to be edited in all three by hand, which is
the failure this project spends most of its review budget on everywhere else.

## What this project is

`fisc` turns the City of Livermore's published budget PDFs into a verified
fact store, and that fact store into a static site whose headline view is a
Sankey of where city money comes from and where it goes.

The distinguishing constraint: **every figure published carries a provenance
pointer back to a page and cell of a source PDF**, and `fisc verify` fails if
any link in that chain breaks. That is the feature. Anything that weakens it —
a number that cannot be traced, a check that cannot fail, an inferred value
presented as a published one — is a defect regardless of how good the chart
looks.

## Where to start
```bash
bd prime                          # workflow context, commands, and memories
bd ready --exclude-type=byob,epic # claimable work; byob decisions and epics hidden
```

`bd prime` injects the project's memories, which carry the things a new
session would otherwise re-derive — why page text rather than tables is the
mapping substrate, why the amount parser is strict, and the environment traps.
Status and next steps live in the bead graph, not in a status document.

## Read before changing anything non-trivial

- **[`docs/m0-spike.md`](docs/m0-spike.md)** — what the three documents actually
  contain, which substrate is usable and which is a trap, and how to
  re-download the PDFs past the city site's bot protection. Written from a
  validation run against the real files; several intuitions it corrects are
  ones a reasonable person would otherwise hold.
- **[`testdata/README.md`](testdata/README.md)** — every fixture exists
  for a specific named failure mode. Read it before writing a parser.
- **`bd list --type=byob --no-parent`** — the architectural decisions this
  project follows, imported from byob-go-cli. **Never claim or close a
  `byob-*` bead**; they are reference material, not work, and closing one
  hides it from future sessions.

## Two rules that must precede the generated block

Everything else in this file is ordered for reading. These two are ordered for
**position**: each is contradicted or undermined by text that arrives in this
same context, and both must sit above the `BEGIN BEADS INTEGRATION` marker
below. The generated block asserts the opposite of the first, and `bd prime`
injects possibly-stale bead text that undermines the second.

An earlier version of this note claimed three such rules and named the review
loop as one of them. Nothing contradicts the review loop; it was inline because
it is important, which is a different reason. It now lives once, below, with the
rest of the workflow.

## The human pushes and pulls, never the agent

**The repository owner runs `git push`, `git pull`, `bd dolt push`, and
`bd dolt pull`. Agents never do, in any circumstance, including at session
end.**

Finish the work, commit it locally, report what landed, and stop. Do not offer
to push, do not ask whether to push, and do not treat unpushed work as
incomplete — local commits are the expected end state of an agent session.

**This overrides the "Session Completion" section in the generated block
below**, which claims work is incomplete until `git push` succeeds. That is
wrong for this repository. The override is kept in this file, above that block,
rather than only linked from it: the text it contradicts arrives in the same
context, and a pointer would not defeat it. An `@` import would — which is why
`CLAUDE.md` may be a one-line import of this file and could not be a link to
it. Expect the generated block to reassert itself on any `bd` upgrade, and
check that it has not been inserted above this section.

## A bead's text is a claim, not a fact

`bd prime` injects the memories and `bd show` prints a bead's description, so
possibly-stale text arrives in context automatically. That is why this sits
here rather than further down.

**Re-derive a bead's premise against the tree before working it.** This is not
hygiene; it changes what the work is. Three worked examples: `fisc-9nw` asked
for a check guarding a case the parser had already made unreachable — the right
answer was to write no check at all; `fisc-5hxr`'s central cost trade-off
dissolved on measurement, because the option it called expensive was the cheap
one; and `fisc-71j` asserted that `row-funds-match-their-anchors` guarded the
78 fund numbers it was about to publish, which it does not and cannot, because
those row labels carry no verb phrase for it to read.

The corpus knows this about itself. Measured at `44be60d`, when there were 52
injected memories: **ten** of them carried a line whose only job is to say
earlier text has gone stale — *"the bead text describing it as blocked is
historical"*, *"text on the lane's beads describing work as pending is
historical"*, *"same stale-premise class as p76 (see
p76-is-extractable-and-ties): claims written against the old extractor outlived
it"*. The criterion is that reading, applied by hand; there is no
command that re-measures a judgement over prose, which is why the count names
the commit it was taken at rather than being left live — the population moves,
and the session that landed this sentence went on to take it to 48. The pin
dates the claim; it does not make it reproducible, because memories live in the
Dolt DB and in no git artifact, so the only record of a superseded one is the
commit message of the session that changed it. The log carries the same habit,
and there the count *can* be re-measured rather than pinned:
`git log --format=%s | grep -ci stale` counts the commits that say so in the
subject line alone — six at
`44be60d`. That is a floor and not the answer: the grep misses others of the
same class whose subjects never use the word — `a76a8dc`'s "corrects six beads"
and `f6e4d00`'s "correct two wrong premises" among them — so a plain-meaning
count is more like 13-15. It is quoted because it is
reproducible, not because it is complete.

So: **correct the bead in the same session you find it stale**, in its notes,
saying what was measured. And do not write "filed as a bead" in a comment or a
commit message without filing it — a pointer to nothing is worse than no
pointer, because it reads as though the work is tracked. That has happened
twice, and both claims were false until review caught them.

<!-- BEGIN BEADS INTEGRATION v:1 profile:minimal hash:7510c1e2 -->
## Beads Issue Tracker

This project uses **bd (beads)** for issue tracking. Run `bd prime` to see full workflow context and commands.

### Quick Reference

```bash
bd ready              # Find available work
bd show <id>          # View issue details
bd update <id> --claim  # Claim work
bd close <id>         # Complete work
```

### Rules

- Use `bd` for ALL task tracking — do NOT use TodoWrite, TaskCreate, or markdown TODO lists
- Run `bd prime` for detailed command reference and session close protocol
- Use `bd remember` for persistent knowledge — do NOT use MEMORY.md files

**Architecture in one line:** issues live in a local Dolt DB; sync uses `refs/dolt/data` on your git remote; `.beads/issues.jsonl` is a passive export. See https://github.com/gastownhall/beads/blob/main/docs/SYNC_CONCEPTS.md for details and anti-patterns.

## Session Completion

**When ending a work session**, you MUST complete ALL steps below. Work is NOT complete until `git push` succeeds.

**MANDATORY WORKFLOW:**

1. **File issues for remaining work** - Create issues for anything that needs follow-up
2. **Run quality gates** (if code changed) - Tests, linters, builds
3. **Update issue status** - Close finished work, update in-progress items
4. **PUSH TO REMOTE** - This is MANDATORY:
   ```bash
   git pull --rebase
   git push
   git status  # MUST show "up to date with origin"
   ```
5. **Clean up** - Clear stashes, prune remote branches
6. **Verify** - All changes committed AND pushed
7. **Hand off** - Provide context for next session

**CRITICAL RULES:**
- Work is NOT complete until `git push` succeeds
- NEVER stop before pushing - that leaves work stranded locally
- NEVER say "ready to push when you are" - YOU must push
- If push fails, resolve and retry until it succeeds
<!-- END BEADS INTEGRATION -->

## Build & Test

Go only — neither the PDFs nor Python are needed to build, test, or verify.

```bash
make build        # bin/fisc
make test         # always -race
make pre-commit   # fmt, vet, narration, beadrefs, test, lint, js
make narration    # refuse history in Go comments and in the injected memories
make beadrefs     # refuse a fisc-* id that names no bead
make hooks        # install the local pre-commit hook, once per checkout
make site         # static site into dist/ (gitignored)
make extract      # re-extract from PDFs; needs poppler-utils and git lfs pull
```

**Two of those targets are rules this file used to state only in prose**, and
both were broken repeatedly while the prose sat in context: `narration` refuses
history in a comment, and `beadrefs` refuses an id that names no bead. Read them
where they are enforced rather than here — the Makefile comments carry the
argument, and each failure message names the section of this file it comes from.
`narration`'s memory arm fails `pre-commit` like any other arm **when `bd` can
answer**, and warns and continues when it cannot — bd absent, bd erroring on an
unreachable database, or a database holding no memories are all skips. It can
never be a CI gate at all, because the memories live in the Dolt DB and in no git
artifact. So a memory erratum is caught on a machine whose bd is working, or not
at all.

`make pre-commit` lints, but warns and continues when `golangci-lint` is not on
PATH — the linter is not needed to build or test this project, so its absence
must not stop a commit. `make lint` on its own still fails, because that target
is CI's required check. Lint was red on `main` across three commits before this
was wired up, which is the gap it closes.

`make hooks` installs the local hook. It is per-checkout setup nobody may have
run — `.git/hooks` is not tracked — so **CI is the gate and the hook is a
convenience**. Do not assume the gate ran; run `make pre-commit` yourself. Note
it does *not* run `fisc verify`.

`./bin/fisc verify` is the gate. `--full` adds the PDF hash check and needs the
LFS files. Run `./bin/fisc build --output bin/facts-rebuilt.jsonl` and `cmp`
against the committed `facts/facts.jsonl` rather than rebuilding in place — the
committed file is the audit trail, and CI compares byte for byte.

## Architecture Overview

`data/pdf` → `tools/extract.py` (poppler) → `data/extracted` (786 pages of
`-layout` text plus `-bbox` geometry, hashed in a manifest) → `mappings/*.yaml`
resolved by `internal/mapping` → `facts/facts.jsonl` (content-addressed facts
carrying `doc_id / page / offset / token`) → `internal/project` → `dist/`.

Extraction is deliberately outside the Go binary: `fisc` reads only the
committed artifacts. See `README.md` for the diagram and `docs/sankey-contract.md`
for the output contract.

## Issue tracking

Beads (`bd`) is the tracker. Do not use TodoWrite, TaskCreate, or markdown
TODO lists.

```bash
bd ready --exclude-type=byob,epic  # claimable work; an epic is never claimable
bd show <id>                       # detail, dependencies, and blockers
bd update <id> --claim             # claim before starting
bd close <id> --reason "..."       # close with what actually happened
```

**Do not read the roadmap off this file.** `bd list --type=epic --status=all` is
the roadmap, and every enumeration of it written here has gone stale: one said
eleven epics and four open, its replacement said twelve and five and named an
epic that had since closed while never having heard of the one that replaced it.
The epics are also not in one numbered series — `E0`, `E9`, `E10`, `E11` and
`E12` were each added after the original plan — so a count taken from any
narrative here is a claim about the plan rather than about the tracker.

**A closed epic here means its end condition was met**, not that every bead filed
under it is done. `E2 Extraction` and `E4 Mapping engine` were closed on that
basis while still sitting at `P0`: all three documents extracted, and
`facts.jsonl` generated from the p66-67 spine. Their few remaining children are
past those end conditions and are open at top level, and the dotted ids still
record where they came from.

**An epic closing does not make its dependents workable, and the check is worth
doing before planning around one.** `fisc-9hf` closed on 2026-08-30 and took
`fisc-1wr.2` with it, and `E5` did not become workable: measured 2026-09-03, its
ladder was downstream of two things and only one of them had moved.
`fisc-1wr.3`, `fisc-1wr.4` and `fisc-1wr.5.1` are deferred to 2026-11-10 by
decision, and `fisc-1wr.7` sits behind the `fisc-mq4.6` → `fisc-mq4.3` chain —
a different reason with the same effect, which is why the count is two. Read the
current state off `bd`; the point that survives is the shape, not the statuses.

Dependencies are wired so `bd ready` surfaces only genuinely unblocked work, and
that held when it was checked. **Priority is the part that drifts.** The failure
mode to look for is a bead whose `P1` contradicts its own note — `fisc-bau` sat
near the top of `bd ready` from 18 to 30 August while its own note said *"NOT ON
THE CRITICAL PATH"*, and a memory had already recorded the contradiction. Each task cites the
byob decision it follows.

Use `bd remember` for cross-session knowledge rather than MEMORY.md files. When
a finding is durable and specific — an arithmetic proof, a document quirk — put
it in the relevant bead's description, in this file, or in the contract doc it
belongs to under `docs/`. `bd prime` injects the memories and `CLAUDE.md`
imports this file, so both arrive in context without anyone choosing to open
them; a contract doc does not, and is the right home only for detail a reader
goes looking for.

## Review is a loop, not a pass

Commit after review at logical points — not continuously, and not never. Skip
review entirely for mechanical commits: a pinned dependency, a `.gitignore`
fix, a docs typo. Review has a real cost and those have no design surface.

Everywhere else, **a single pass is not the gate**, and the reason is counted
rather than asserted: in
[What fifteen passes measured](#what-fifteen-passes-measured), an earlier pass's
own fix is the second-largest source of findings in the whole audit. Four shapes
it takes, each found by the pass after the one that wrote it —

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
and one of those cited a **derived** figure as a printed one — the "published and
derived are different things" invariant broken in published text, three commits
after the test beside it said in as many words that the figure appears on no page
in the corpus.

### The loop

1. **`/code-review` over the range, not the last commit.** A lane's goldens,
   its check, its export seam and its client are one claim; reviewing the first
   commit alone cannot see whether the client renders what the projection
   publishes.
2. **Triage before you fix, and fix only two of the five classes.** Every
   finding is one of: **WRONG-OUTPUT** (a wrong number, a wrong link or a
   broken page could reach a reader), **FAIL-OPEN** (a guard or a check that
   cannot fail, or that accepts what it should reject), **FALSE-CLAIM**
   (committed text saying something untrue about the code), **DESIGN** (built
   the wrong shape), **HYGIENE**. **Fix the first two in the pass; file the
   rest**, unless the fix is a single line in a file the pass is already
   touching. The reason is measured and it is the whole of
   [What fifteen passes measured](#what-fifteen-passes-measured): a pass that
   fixes everything it finds writes 150-450 unreviewed lines, and those lines
   are where the next pass's findings come from.
3. **Fix the findings you kept.** Each fix lands with the test that would have
   caught it — see [Prove it can fail](#prove-it-can-fail) — and the code and
   its review land together rather than as a fix-up commit.
4. **`make pre-commit` and the lane's mutation proofs green** before
   re-reviewing. A red gate means step 3 is not finished.
5. **Re-review the range including the fixes.** This is the step that matters:
   it is where the table above comes from.
6. **Stop when a pass returns no WRONG-OUTPUT and no FAIL-OPEN finding** — not
   when it returns nothing. It will not return nothing. Over every pass the log
   records the smallest yield is **two** and zero has never once occurred, so
   the rule this replaces named an outcome the project has never observed and
   the cap was doing all the stopping. Findings you decline for a stated reason
   count as stopped too — `fisc-i38`, `fisc-oz4` and `fisc-8fr` all exist
   because a review pass surfaced something real that was not worth taking
   then.
7. **File whatever is still open when you stop**, whether you are stopping
   because you declined it or because you hit the cap. See
   [Nothing leaves a pass unfiled](#nothing-leaves-a-pass-unfiled).
8. **The last pass's own fix ships unreviewed unless you do something about
   it.** Nothing in steps 1-7 reads it: it is committed after the final review
   and the lane closes. Either give that diff alone a narrow extra pass, or
   keep it small enough that you can say why it needs none. On 2026-08-31 the
   three lanes' fifth-pass fixes were 191, 135 and 74 insertions — 400 lines,
   43 of them in `site/app.js`, which readers are served verbatim — and nothing
   read any of them. `fisc-i92i`.

### Three passes at least, five at most

**Three is the floor.** Two passes is one pass plus a check that the fixes
parse. The table above says why: the defects an earlier fix introduces are only
visible to a pass that runs after it, and the second pass is usually still
finding original defects rather than introduced ones.

**Five is the ceiling**, raised from three because three was demonstrably too
low on a lane whose third pass found five real defects, all of them in the
original commit. The ceiling is not a target — stop the moment step 6's
condition is met.

**No lane has ever stopped because a pass came back clean.** A pass has never
returned clean here, and lanes routinely run the full five —
`git log --format=%s | grep -ci fifth` counts them. Both commands below are
live, so re-run them rather than quoting this paragraph; every figure it has
ever carried has gone stale, most recently because the session that wrote this
sentence went on to add eight review-fix commits of its own. So a stopping rule
that waits for silence is a rule that always defers to the cap.

**A late pass still finding real defects is a signal about the change, not about
the review.** At the cap, report what the last pass found and let the owner
decide whether to narrow the commit rather than keep patching it. Every boundary
in the measured session hit its cap with findings still outstanding, and that is
information the owner should have rather than something to absorb silently.

**A fix that propagates a NUMBER without its DEFINITION is a shape to look for.**
On the ACFR tolerance lane a second-pass reviewer read two files that disagreed
about a figure and said to make them agree; the wrong one was corrected, because
an intermediate draft had quietly redefined the term both were counting ("the
page's four blocks" → "the four blocks this file maps"). The fourth pass found
it.

**The passes that feel least productive are the ones worth having.** That same
lane ran the full five and went 11, 8, 6, 6, 4, and exactly one of its 35
findings was fail-open — an unguarded `abs(diff)*2` in the new tolerance that
wrapped negative and accepted any discrepancy. It survived four passes. The other
34 were false text, missing tests, or refusals that were too strict.

### What fifteen passes measured

Everything above is anecdote sharpened by re-reading. This is the one place the
findings were counted rather than remembered: all **115** findings of the three
2026-08-31 lanes, classified one at a time from the commit bodies, with
provenance settled by `git log -S` wherever a message does not say where a
defect came from.

**Start with the number the session got wrong about itself.** `fisc-yj4w`'s
notes report *"DEFECTS INTRODUCED BY AN EARLIER PASS'S OWN FIX, by lane: caveats
2 of 33, drill 3 of 48, badge 3 of 29"* — eight. Per finding it is **27**:

| lane | as recorded | measured per finding |
|---|---|---|
| caveats | 2 of 33 | **6** of 37 |
| drill | 3 of 48 | **12** of 49 |
| badge | 3 of 29 | **9** of 29 |

The recorded figures count *passes that contained* such a finding. And the
undercount was not fixable by reading harder: **only one of the caveats lane's
six is labelled as introduced-by-fix in its own commit message**; the rest are
recoverable only from the diffs. That is
[Before you quote a number](#before-you-quote-a-number) applied to the
attribution field, and it matters more than the other instances of that defect
because this is the number the loop is judged by.

**Where the 115 lived**

| | count | share |
|---|---|---|
| NEW-CODE — the lane's own feature commits | 39 | 34% |
| **INTRODUCED-BY-FIX — an earlier pass in the same lane** | **27** | **23%** |
| PROSE-NEW — a false claim in text this lane wrote | 20 | 17% |
| TEST-GAP — no guard, or a guard that could not fail | 15 | 13% |
| PROSE-PRE-EXISTING — older text the lane silently falsified | 12 | 10% |
| **PRE-EXISTING code** | **1** | **<1%** |
| classified in two rows at once | 1 | |

**What kind they were**

| | caveats | drill | badge | total |
|---|---|---|---|---|
| WRONG-OUTPUT | 4 | 22 | 14 | 40 |
| FAIL-OPEN | 11 | 12 | 6 | 29 |
| FALSE-CLAIM | 19 | 8 | 5 | 32 |
| DESIGN | 2 | 4 | 3 | 9 |
| HYGIENE | 1 | 3 | 1 | 5 |

Five things follow. Three are the reason for steps 2, 6 and 8 above; the other
two are `fisc-rx1d` and `fisc-xbd4`.

**The loop is its own second-largest defect source.** 27 against 39 from the
feature commits, and against **one** pre-existing code defect in 115. Whatever
else these passes were doing, auditing the tree was not it — they reviewed the
change, and then reviewed themselves. The share also **rises with pass number**:
in the drill lane, findings attributed to an earlier pass's own fix run 1, 3, 3,
**5** across passes two to five; three of the badge lane's fifth-pass five are
the loop's own work. By the fourth pass the review is mostly reading its own
output, which is what a non-decaying count looks like from the inside.

**A pass has never returned nothing.** `git log --format='%s' | grep -oiE
'[a-z]+ findings'` over the whole log. Measured at `ab71b7a`, 92 passes and 472
findings — and note the grep over-collects, catching two subjects that carry the
word without a yield. The distribution, at that commit:

| yield | 2 | 3 | 4 | 5 | 6 | 7 | 8 | 9 | 11 | 12 | 13 |
|---|---|---|---|---|---|---|---|---|---|---|---|
| passes | 8 | 10 | 27 | 15 | 7 | 14 | 5 | 3 | 1 | 1 | 1 |

Zero has never occurred and neither has one, over four months and every lane the
log records. Yield is also close to independent
of how much code the pass is reading: the caveats lane's first pass read 1,812
insertions and returned 9, its fourth read a 69-line fix and returned 5. A rule
that waits for silence from a detector with a floor of two is a rule that always
defers to the cap.

**The fixes are the size of the feature.** Insertions, `.beads` excluded:

| lane | feature | five fix passes | ratio | yields |
|---|---|---|---|---|
| caveats | 1,812 | 729 | 40% | 9, 5, 7, 5, 7 |
| drill | 1,069 | 1,246 | **117%** | 12, 13, 7, 7, 9 |
| badge | **200** | 896 | **448%** | 8, 7, 5, 4, 5 |

Against the lanes that *did* decay, whose late fixes shrank to nothing:
fund-balance's fifth was 16 insertions and yielded 2, the guards lane's fourth
was 26 and yielded 2. The three lanes above never shrank — their fifth-pass
fixes were 74, 135 and 191. **Decay is what a shrinking fix commit looks like**,
which is why step 2 files rather than fixes the three classes that are not the
stopping condition.

**Planning is not where any of this comes from.** DESIGN is 9 of 115. `fisc-yj4w.7`
declines two alternative tier sets with measured layout numbers, names the file
and line of every seam it will touch, and records in its close reason the two
premises that turned out wrong — both of which were caught while implementing,
not by any review pass. What the passes found instead was code shipped without
its guard: `bd0a098` added 492 lines of `site/app.js` and 86 lines of
`tools/jscheck`, **none of them covering the drill it had just written**, and the
drill's whole check module was written *inside* the review passes
(`git log --diff-filter=A -- tools/jscheck/drill.mjs` → `a3fe8e8`). Across the
session's sixteen review-fix commits `drill.mjs` took **633** insertions against
`app.js`'s 503 — the largest single sink of fix churn there was. A finding that
reads *"this has no test"* is the test-writing step, deferred, relabelled, and
paid for at the price of a whole pass. `fisc-rx1d`.

**One fact in three files is three findings.** The badge lane found the same wrong
count — *"the other six fund groups"* — three times, in `fundflows.go`, then
`data.go`'s lede, then `docs/general-fund-drilldown-contract.md`, across three
consecutive passes. The caveats lane's *"250 words"* was false in six places at
once. So: correcting a fact in prose means grepping for its copies in the same
commit. `fisc-xbd4`, and see
[Before you quote a number](#before-you-quote-a-number).

### Nothing leaves a pass unfiled

**Every finding you do not fix becomes a bead, in the same session, before you
report.** Declined, deferred, out of scope, too small to bother with, or simply
past the cap — all the same rule. Priority is where the judgement goes: P3 and
P4 exist so that "not worth doing now" has somewhere to live that is not a
paragraph in a session summary nobody reads again.

The small ones are the point. A finding worth a P1 will be re-found; a
one-sentence P4 about a misleading count in a doc comment will not, and it is
precisely the class this project keeps discovering years-stale. `fisc-i38`,
`fisc-oz4` and `fisc-8fr` are the good case — all three were declined on purpose
and all three are still findable. The bad case is a finding that was reported to
the owner, agreed with, and never written down.

Two failure modes to refuse by name:

- **"I will fix it in the next commit."** File it anyway. If the next commit
  fixes it, close the bead in that commit; a bead that lived twenty minutes
  costs nothing.
- **"Filed as a bead"** written without filing one. This has happened twice and
  both claims were false until a later pass caught them. See
  [A bead's text is a claim, not a fact](#a-beads-text-is-a-claim-not-a-fact).

Reporting to the owner is not a substitute and neither is the commit message.
The report is for the person reading it now; the bead is for whoever opens
`bd ready` next month.

### A finding that touches a golden gets one extra step

Regenerate, then **read the diff against a rule cheap enough to check by eye**,
before re-reviewing. There is deliberately no `-update` flag anywhere in this
repo. `testdata/sankey.golden.json` was regenerated once, to add a key, and the
diff was checkable because `fixture_test.go`'s `spinePage` is a two-branch rule
— general and enterprise on p66, everything else on p67 — so all 58 added
blocks could be verified by hand. A regenerated golden nobody read is how a
review pass launders a bug into the audit trail.

### Review does not cover this project's main risks

The highest-risk claims here are empirical, not structural:

- the amount parser rejects the right tokens and accepts the right ones
- extraction is byte-deterministic across runs
- mapped rows sum to the totals the documents themselves print
- a mapping locator still resolves to the row it was written against

Reading a diff cannot confirm any of these. They are guarded by corpus scans
over `data/extracted/`, arithmetic tests against published figures, and
re-run-and-compare checks — and those stay mandatory regardless of whether a
review ran. Treat review as a complement to that evidence, never a substitute.

A worked example of the standard: rejecting a leading minus sign in
`internal/amount` is justified by summing ACFR p177 row 2017 and showing that
only the positive reading reconciles to the printed total. The test is
`TestLeadingMinusIsReallyPositive`, in `internal/amount/amount_test.go`, and it
carries the arithmetic; `internal/mapping/acfr_p177_test.go` reads the same row
off the committed fixture and reconciles the schedule against the city's own
printed totals — nine of its ten rows tie exactly and FY2024 is short by
$176,292, which is exactly that row's own Financed Purchases column, so the test
is named `TestACFRDebtScheduleTiesExceptOneRow` rather than pretending
otherwise. That is the level of proof a claim about these documents needs: they
are proved with arithmetic, not intuition, and review does not substitute for
it.

## Prove it can fail

`README.md` already says a check that cannot fail is a defect. This is how you
show one can.

The doctrine is written down; read it rather than re-deriving it.
`internal/check/vacuity.go` is the strongest statement — *a vacuous check is
declared or `--strict` fails on it*, and a declaration that has gone stale
fails either way. `internal/check/check.go` sets out what these checks can and
cannot witness: two functions over identical input inside one process cannot
witness a wrong amount, and a run of 61 perturbed amounts once passed every one
of them. `tools/jscheck/seam.mjs` is the file that checks the thing that checks
it, for the same reason.

**The mutation is the proof.** Reproduce the defect green, then show it red.
Both belong in the commit message, which is what the house style already does:
*"reverting `if !ok || !c.Assignable` to `if !ok` makes it fail on the status
assertion directly."*

### The failure mode to look for: green because the gate fired

Not green because the defect was prevented. The two are indistinguishable from
the test's exit code and completely different in what they guarantee.

The worked example, because recognising the shape is cheaper than re-deriving
it. `tools/jscheck`'s `twoYearConfig` carried `docs: {}`. `citations()` opens
`const doc = CONFIG.docs[source.doc_id]; if (!doc) continue` — so it returned
before ever reaching `source.pages`, and **every** required-key check for a
`[].pages` key was passing because `drawableSankey` rejected the document,
never because a throw had been prevented. With `docs` populated, deleting the
guard gives `TypeError: source.pages is not iterable` and a flow table at 0
rows against 58 — a half-repainted page at the reader.

This is at least the third occurrence. `harness.mjs` records two more in its
own comments: modelling ids and not attributes made a check unfalsifiable, and
*"deleting `group.removeAttribute("disabled")` from app.js left the whole suite
green."* When one turns up, look for its siblings — the fixture that hid one
usually hides several.

So the question is not "does the test pass". It is **"what is this fixture
hiding, and what would have to break for this to go red?"**

## Before you quote a number

**First ask whether to write it at all. Do not report a count of the tree's own
contents.** How many facts, nodes, links, subjects, checks or rules there are
goes stale on the next commit and tells a reader nothing they could not get by
looking. A count earns its place only when it is **extrinsic** — when it says
something about a world outside the code that the code cannot report. *"We
handle 18 of the 20 status codes"* is worth writing; *"this map holds nine entries"*
is not.

Two kinds qualify and nothing else does:

- **A count against the documents.** *"pp.85-125's 78 rows"*, *"23 divisions
  under 11 departments"*, *"the corpus is 786 pages"*. The
  city printed those, or they are our coverage of what the city printed. This is
  the project's whole subject and it stays.
- **A count that is the evidence for a decision** — the reconciliation
  arithmetic, and `site/app.js`'s crossings against every other ordering still
  reachable. The number *is* the argument. It stays, and it must be pinned by
  something that re-measures it, which is what `tools/jscheck/layout.mjs` exists
  for; where nothing can, name the commit it was measured at, as this file does.

**The tests here already apply this and the prose never got the memo.**
`TestTheCommittedCorpusVacuitySplit` says *"It asserts the words and not the
subject counts. The counts move with every page that gets mapped, and pinning
them here would make this a test of the mapping's size."*
`TestUnprojectedScopesAreDeclarations` says *"The count is deliberately NOT
pinned."* Both refuse a count that comment phrases and printed strings elsewhere
in the same packages go on to state anyway. `fisc-pm8f`.

Everything below is for the numbers that survive that question. Every claim in a
comment, a commit message or a bead is checkable, and this project treats an
unchecked one as a defect. Three traps:

- **`make pre-commit` does not run `fisc verify`.** Rebuild `bin/fisc` before
  quoting a check count. One session reported "38 passed" after landing a check
  that made it 39, from a binary built before the check existed.
- **Rebuild-and-`cmp`, don't rebuild in place.** `./bin/fisc build --output
  bin/facts-rebuilt.jsonl` then `cmp` against `facts/facts.jsonl`; the
  committed file is the audit trail and CI compares it byte for byte.
- **A commit message describing a fix is a claim about the tree.** Grep for the
  fix before writing the sentence. `261c78f` said it had reworded a citation and
  spelled out a README cell; the fifth review pass found both unchanged. The
  cause was mechanical and will recur: a batch of scripted edits with an
  assertion in the middle aborted at the second, the later edits never ran, and
  the message had been drafted from the plan rather than from the diff.

  **So: one edit per script, or check each edit's exit status.** A heredoc that
  raises halfway leaves a tree that still builds and still passes every test,
  so no gate goes red. The range audit below is the systematic version of this.

  An unmade edit is the class of writing "filed as a bead" without filing one,
  and both are worse than a missing note in the same way: the claim reads as
  *done*, or as *tracked*, so nobody goes looking.

- **An id in prose or a comment is checked by `make beadrefs`**, over the path
  list in the Makefile, against `.beads/issues.jsonl` and then against `bd` for
  anything the export has not caught up with. It is a CI job, where only the
  export answers. This used to be a paragraph here and was broken twice anyway;
  the id `bd create` prints is not guessable, so writing one from memory is easy
  and reads as tracked.

  **What it does not cover.** A commit MESSAGE is not scanned before it lands, so
  read the id back there yourself. And its reach is two hand-maintained lists —
  the paths, in the Makefile, and the file extensions picked up while walking one
  of them, in `scannable`. A listed path that has gone from the TREE fails the
  run; a path DELETED FROM THE LIST just makes the scan quietly smaller, and
  neither list can know about an entry that was never added. That is how
  `site/app.js` and then `site/style.css` were each unchecked for a while after
  this landed.

- **When you correct a figure in prose, grep the tree for its copies in the
  same commit.** A fact restated in four files is not one defect; it is four
  findings arriving one per review pass. Measured on the badge lane: *"the
  other six fund groups"* was wrong in `internal/project/fundflows.go`, in
  `pkg/cmd/export/data.go`'s lede and in
  `docs/general-fund-drilldown-contract.md`, and was found and half-fixed in
  three *consecutive* passes, each of which believed it had finished. The
  caveats lane's *"250 words"* was false in six places at once.

  It is grep-then-**read**, not grep-then-replace: `grep -rn "six fund groups"`
  also returns ten hits that encode a different and largely true proposition —
  how many groups the spine prints, not how many stop at their funds. The same
  numeral standing for two claims is why this cannot be a check. `fisc-xbd4`.

The gate line most commits here end with is those two together: *"facts.jsonl
unmoved; fisc verify 40 passed, 0 failed."*

**The count in that template is itself the trap, and this sentence is where to
learn it.** Three consecutive review passes corrected it and each correction was
wrong in a new way: the template shipped stale; the fix that replaced it quoted
`check.All()`'s total instead, which is a different quantity, because a vacuous
check and a `--full`-only check are both in that total and neither is in the
"passed" line; and the fix that replaced THAT
asserted an exhaustive list of every count the log has carried, measured with a
grep narrow enough to miss several spellings of the same line. So no list is
given here. Rebuild `bin/fisc`, run it, and read the number off the run you are
about to describe. A count copied from anywhere — this file, another commit,
memory — is the defect this section exists to name.

**At a lane boundary, audit the whole range's claims at once.** Every "this
commit fixes X" in the range, checked against the tree in one script. Over
`cd1192c^..4f3c3ab` — eight commits: the pp.85-125 lane, its five review-fix
passes, and two of beads and docs — that was 23 claims and 2 were false, both of
them fixes asserted in a message that never landed, and both found by review
rather than by the author. The range is pinned to two commit ids and not written
as `..HEAD`, because a doc that names a moving range stops naming what it
measured the moment anything else lands; and it starts at `cd1192c^`, because
`a..b` excludes `a` and the lane's own commit is the one carrying most of the
claims.

## Commits

- Conventional Commits: `feat(mapping): ...`, `fix(extract): ...`, `docs: ...`.
  **Compound the scope when a change spans packages** — `fix(check) +
  fix(mapping): ...` — rather than picking one and hiding the other.
- Reference beads as `Refs <id>` in a trailer paragraph, or `Closes <id>` when
  the commit finishes one. Both are used here; `Refs` alone is not the rule.
- Explain *why* in the body, especially when the reason is a document quirk
  that will not be obvious from the diff.
- Regenerated artifacts belong in the same commit as the change that caused
  them.

The house style is denser than that list suggests, and the density is doing
work. From the log:

- **Numbered findings** — `(1)`, `(2)` — when one commit fixes several, each
  opening with a capitalised lead clause that names the defect rather than the
  change: *"THE BACKSTOP I CLAIMED COULD NOT FIRE"*, *"THE `data/pdf` ARM
  COMPARED BASENAMES"*.
- **Measurements inline, labelled as measured.** *"Measured -- dropping
  data/reconciliations.yaml ... leaves fisc verify at 39 passed, 0 failed."*
  A number in a commit message is a claim like any other. (That 39 is quoted
  from the log, so it is what was true then rather than a live count.)
- **The mutation stated**, per [Prove it can fail](#prove-it-can-fail): what
  was reverted, and what went red.
- **The gate line**, near-formulaic: *"facts.jsonl unmoved; fisc verify 39
  passed, 0 failed."*
- **Credit where a finding came from**: *"Found by /code-review over this
  range."* It tells the next reader whether a fix was designed or discovered.
- **Run `make pre-commit` yourself before committing**, and see "Build & Test"
  above for what it does and does not cover. The two things worth repeating
  here because they bite at commit time: it does **not** run `fisc verify`, and
  the local hook is per-checkout setup nobody may have run, so it is a
  convenience and CI is the gate. This paragraph used to restate the whole of
  that section and the two copies had already drifted apart on whether node is
  mentioned — which is the duplication this file was merged to end, reappearing
  inside it.

## Provenance invariants

These are the rules the project exists to uphold. Breaking one is a defect
even when tests pass.

- **Amounts are integer cents.** No float anywhere on the path from an
  extracted cell to a published total. These values are summed and compared
  against printed figures; float drift makes those comparisons meaningless.
- **Absent is not zero.** In these documents `-` means the line exists and is
  zero, while an empty cell means the line does not apply. Conflating them
  invents rows. `amount.Parse` returns `ErrAbsent` for the latter;
  `amount.ParseOrZero` opts out only where a rule has declared that blanks
  mean zero for that table.
- **Fail closed on ambiguity.** PDF extraction corrupts numbers into plausible
  wrong values rather than errors. Every shape not positively recognized is an
  error. Never guess at a value to keep a pipeline green.
- **Published and derived are different things.** A figure the city printed and
  a classification we inferred must not be presented alike. Derived nodes carry
  `derived: true` with a `rationale` and `source_note`, and `verify` fails
  without them.
- **Identity and integrity are separate.** A locator says *which* row this is
  and must be content-independent; a content hash says *whether it changed*.
  One value cannot do both. Budget Book p67 prints four byte-identical
  all-dash lines; content alone cannot say which row any of them is, so
  identity has to come from position. (This invariant was originally argued
  from 16 byte-identical ACFR *tables*. The table substrate no longer exists —
  see the extraction boundary below — but duplicate content does, and the
  argument is the same.)

### A forecast is not a fact

Nothing computed goes into `facts/facts.jsonl`. A fact is one figure the city
printed, and three checks make that a guarantee rather than a claim — none of
which consults `Fact.Derived`:

- `fact-token-reparses` re-parses each fact's own token and fails an empty one.
  Its doc comment settles the case in advance: *"a fact with no token has no
  printed figure behind it, cannot be re-derived, and cannot be cited."*
- `fact-offset-points-at-token` requires the extracted page text at the fact's
  offset to *be* that token. **This is the arm that matters**, because it is the
  one a synthetic token cannot get past: a made-up figure can be made to
  re-parse, but no page prints it.
- `fact-ids-recompute` needs a `rule_id` and a `doc_id` in the hashed tuple.

So `Derived: true` on a fact means a re-reading or re-classification of a figure
the city printed at a page and an offset — never a computed value. The field
exists to *state* the published/derived distinction, not to exempt anything from
the checks above, and it must not become that exemption. An exemption arm is the
worse failure: it does not weaken the store visibly, it weakens it for a set
whose membership is a boolean somebody sets.

A projection or scenario may derive figures, and does so under its own rules —
`derived-nodes-justified` requires a rationale and a source note on every derived
node. The rule here is only about the store. See `fisc-nvw` for where a forecast
is allowed to live and what would have to be true for `fisc verify` to police one.

## Go

Layout and idioms follow the byob decisions; read them with
`bd show byob-<id>` rather than inferring from the code.

- Three-tier layout (`byob-layout.1`): `cmd/fisc` → `internal/fisccmd` →
  `pkg/cmd/root` → `pkg/cmd/<feature>`.
- Each command is `Options` + `NewCmdXxx(f, runF)` + private `xxxRun`
  (`byob-command-shape.1`), with `Options.Validate()` first, failing via
  `cmdutil.FlagErrorf` before any side effects (`byob-input-validation.5`).
- Commands return errors and never call `os.Exit` (`byob-errors.1`). The
  runner owns the error→exit-code mapping.
- One `Factory` built in `main`, threaded everywhere; expensive dependencies
  are lazy closures so `--help` touches no filesystem (`byob-factory-di.1`).
  There is a test asserting exactly that; keep it passing.
- Data goes to `Out`, everything else to `ErrOut` (`byob-iostreams.3`).
- `CGO_ENABLED=0`, pure Go, assets via `go:embed` (`byob-release.8`).
- Stdlib first (`byob-release.10`). A dependency outside cobra, go-cmp,
  modernc sqlite and goreleaser needs its own decision bead in the same
  change — see `fisc-j8f` for the YAML one.

### Comments are checkable claims

The node-boundary section below says *"quote what the current code does"* about
numbers. The same holds for comments that restate a **rule**: one that outlived
its rule is a defect, not cosmetics, because the next reader acts on it. One
session found four — a required-key list naming keys the caller no longer
dereferenced, a parse error reading "row %q has neither category nor
department" that would fire on a row which *had* a department, a guard message
describing a check its condition could not perform, and a summary sentence that
went false one commit after it was pinned.

**A comment names a symbol. It does not say what the symbol does, and it does
not say where the symbol lives.** Both are second sources that nothing keeps in
step: the edit that falsifies a restatement is in the other file, so its author
never sees the comment, and a path goes stale the moment the symbol moves. Go
tooling finds a symbol from its bare name — qualify with a package only where
the name is ambiguous.

**The exception is surprise.** Document another API only where it does not make
sense on its face, or has to be used oddly for this code to work — and then
write down *what surprised you*, not what the API does. That is the one thing
the other file cannot tell the reader, and the one thing a reader loses by
looking it up.

**Data and artifact paths are not symbols and stay.** `data/funds.yaml`,
`testdata/pages/p0067.txt`, `mappings/*.yaml`, `site/app.js` — there is nothing
to look up by name, so the path is the reference rather than a second copy of
one. It is only a symbol's `.go` path that goes.

**History's home is git.** No `Found by /code-review` credit line in source:
[Commits](#commits) already puts that credit in the commit body, where it tells
the next reader whether a fix was designed or discovered. No errata either —
*"an earlier version of this comment said X"* adds a second claim, about the
past, that nothing can check. Where an erratum carries a **rule**, keep the rule
and drop the history that argued for it.

**`make narration` is the enforced half of this**, over Go sources and over the
injected memories, which are worse placed for an erratum because `bd prime`
delivers one whether or not anyone opens the file it is about. Both arms are
deliberately narrow rather than prose detectors — the Go arm refuses two literal
phrases, and the memory arm five patterns, three naming the memory as their
subject and two matching a house-style shape — so the rest of the rule is still
read by hand. `make beadrefs` is the
sibling that refuses a bead id naming no bead.

**This file is governed by the rule too.** It carried two errata blocks about
wrong commit messages, each keeping the rule and the history that argued for it;
the rules are now stated where they belong — in
[Before you quote a number](#before-you-quote-a-number) — and the history is in
the log.

**Narration already in the tree is dropped in the file you were editing
anyway, never in a sweep.** When you touch a file for any other reason, drop the
history from the blocks you touched — and *drop* it, do not rewrite the comment
around it. `fisc-pm8f` priced the rewrite: 42.5 minutes and 411k tokens for 148
lines of `internal/geom`, which extrapolates to ~90 hours over the tree and grew
the file it touched by 18%. Verification cost attaches to claims written or
kept, so a deletion is the cheap half and a restatement re-imports the whole
cost.

Two measurements decide this, both taken at `a14b6c2`. First, **a sweep would
reach nothing an ordinary lane does not**: all 62 files carrying narration were
touched in the last 100 commits — no cold files — and 40 consecutive commits
reach 71% of the narrated lines. Second, **the quantity is small and the usual
figure is the wrong one**: 185 of 19,137 comment lines carry a history marker,
about 1%. `fisc-a3rp`'s ~3,000 is a count of every line of every *block* one of
those sentences sits in, which is what you must READ, not what you would delete;
deleting the block takes the rule with the history, which is what the paragraph
above forbids. Sizing the work off it overstates it by an order of magnitude.

The reason this is a standing habit rather than a lane is in the numbers below:
history is the smallest of the four causes. Reading a comment warm, with the
code it describes already open, is also the only way to tell a rule from the
history that argued for it — the hard case is a paragraph whose history IS the
evidence for the rule it ends with, and no filter separates those.

The evidence is `fisc-pm8f`, over the findings counted in
[What fifteen passes measured](#what-fifteen-passes-measured). Reclassifying the
false-text ones by *cause* gives restating-a-rule-that-changed-elsewhere 35%,
never-true-when-written 23%, quoted-number-drifted 22%, history 15% — so the
narrative register is the carrier and not the cause, and the hypothesis that
prompted the audit came back mostly wrong: measured at `823de73`, history-marker
comment lines are 247 of 18,746.

**Do not insert code between a doc comment and its declaration.** It happened
twice in one commit: a new helper orphaned `foldDocument`'s JSDoc, costing that
function its `@param` under `// @ts-check` and leaving its body unchecked, and
a new test orphaned the rationale belonging to the test below it. Both read as
correct in the diff and were wrong in the file.

### Testing

- Tests ship in the same commit as the code they cover (`byob-testing.4`).
- `google/go-cmp`, not testify (`byob-testing.2`). `cmp.Diff(want, got)`.
- Assert on behavior, not call counts (`byob-testing.3`).
- **Go tests never require Python, the source PDFs, or the network.** Use the
  fixtures in `testdata/`, which are real artifacts copied from
  `data/extracted/`. They are *copies*, and `make extract` does not touch them:
  the page fixtures under `testdata/pages/` and
  `pkg/cmd/build/testdata/pages/` have to be re-copied by hand when the
  extraction changes, and their sha256s must equal the ones the source
  document's `manifest.json` records. A fixture that has drifted is the bad
  case — the tests reading it stay green against a substrate that no longer
  exists, which is exactly what happened across the xberg → poppler migration
  (`fisc-yqv.5`). Re-copy; never edit an assertion to fit a stale fixture.
- Smoke-test the built binary, not only the packages. The unknown-command bug
  — a typo'd command exited 0 and printed help — passed every unit test and
  was caught by running `./bin/fisc biuld`.

## The node boundary

`site/app.js` is served to readers exactly as it is committed — no bundler, no
npm, no module system — and that is a property to keep. But the figures it quotes
about itself (195 ribbon crossings, $457,434,169 of overlapping ribbon, 14
stale-stacked pairs) can only be produced by laying the graph out, so for a while
nothing in the tree could confirm any of them.

`make js` runs `tools/jscheck`, which loads **the shipped `app.js`** and the
vendored d3 into a node `vm` and re-measures them. It reaches into the file
rather than copying functions out of it, because a copy would check the copy and
let the original drift.

Node stays **off the deploy path**, the way `tools/extract.py` stays off the
build path: `make build`, `make site` and `fisc export` never run it, there is no
`package.json` and no `node_modules`, and `pre-commit` warns and continues when
node is absent — CI runs it as its own job. A contributor with only Go can still
build, test and land a change.

One consequence worth knowing before quoting a number in a comment: **a claim
whose baseline no longer exists in the tree cannot be checked.** `app.js` used to
say its crossings fell "from 394", a figure produced by a sort order that has
since been replaced. Measured against every ordering still reachable — 285 under
a sort by size, 297 under the input order, 246 under d3's own pass — none is 394,
and the number could not be reproduced from anything in the tree. Those
before-figures were corrected to what does reproduce, and every one of them is
now pinned rather than bounded, so a comment edited without re-measuring fails
`make js`. Quote what the current code does.

### A change to `app.js` ships its check in the same commit

`site/app.js` has no compiler, no types and no test framework. `tools/jscheck`
is the only thing standing behind a file readers are served verbatim, and it is
hand-written per feature, so a client change with no jscheck beside it
is a change nothing can see go wrong. **Do not open review on one.**

Measured, because the cost is not obvious: `bd0a098` added 492 lines of
`site/app.js` and 86 lines of `tools/jscheck` — all of it in `fold.mjs` and
`harness.mjs`, none of it reaching `drillTo`, `filterToNode`, `capColumn` or
either declared tier set. `tools/jscheck/drill.mjs` was created by that lane's
*first review pass* and reached 375 insertions across the five; over the
session's sixteen review-fix commits it took 633, against `app.js`'s 503, which
made it the largest single sink of fix churn there was. Worse, checks written
under review pressure to close the previous pass's finding were the weakest ones
in the lane: a drill guard that measured the chart already on screen from the
second node on, a baseline that never asserted the drill happened, a share check
that fetched the wrong fiscal year's golden, and a selector entry malformed so
that it *"printed as declared and could not fail"*.
Four checks written to close findings, four that could not fail — which is
[green because the gate fired](#the-failure-mode-to-look-for-green-because-the-gate-fired)
arriving through the review loop rather than through a fixture.

State the mutation, per [Prove it can fail](#prove-it-can-fail). `fisc-rx1d`
carries the audit of which of `app.js`'s current paths no module drives.

## The extraction boundary

Extraction is **not** a Go responsibility and must not become one.

`make extract` runs `tools/extract.py`, which is the only Python in the
project. Go never shells out to it. `fisc` reads only the committed artifacts
under `data/extracted/` and needs neither Python nor the PDFs — which is what
lets CI run `verify` without a venv and without an LFS checkout.

The extractor is poppler (`pdftotext`), a system package; the script itself is
standard library only and PyPI is not reachable from the extraction
environment. It emits two substrates per page: `pages/pNNNN.txt` from
`-layout`, which reproduces the printed column grid in runs of spaces, and
`geometry/pNNNN.json` from `-bbox`, which carries per-word bounding boxes.
Both are needed. On a sparse grid `-layout` emits only the tokens that were
printed and nothing that says which column each belongs to, so a positional
read files them left to right and can land a figure under the wrong year;
geometry gives the x-position that settles it. Page text is `.txt` and not
`.md` because GitHub's blob view renders markdown and collapses the spaces
that *are* the grid.

Geometry gives **column identity for tokens that are present**. It does not
recover a value the PDF never put in its text layer, and it does not settle
"absent is not zero" on its own: CIP p40 rows PB200654 and PB202617 print `-`
in their intervening FY columns and those dashes appear in neither substrate,
because they are drawn as non-text. Row PB200429 on the same page does carry
its dashes, so this is per-row and not a flag chosen wrong. A rule that needs
to tell an absent cell from a zero one must say so itself, and **must not
resolve it by defaulting a missing token to zero** — that turns an absent cell
into a printed one and invents a row. Tracked as `fisc-8ln`.

Where this lives in the code is `(*Resolver).labelledValues` in
`internal/mapping/resolve.go`, and **read it before assuming a sparse row fails
closed, because mostly it does not.** It tokenises `blk.Text[after:]` — the
whole remainder of the *block*, not of the row — so a row missing a cell simply
borrows the next row's leading token and reaches `ncols` anyway. The
`len(toks) < ncols` guard, *"row %q is followed by %d values, want %d (one per
column)"*, therefore fires only when the shortfall runs off the end of the
block. `toks = toks[:ncols]` then discards the overflow — and note that
discarding is *not* a way to absorb a trailing footnote marker, because the
truncated marker is left sitting in the gap where `checkGap` refuses it as
unexplained text. That is precisely why p76's headerless marker column has to be
*declared* rather than ignored (`fisc-wfi`, and `internal/mapping/rule.go`'s
`ColumnHeader` comment).

**What catches the borrowing is mostly downstream of the read, not in it.**
`cursor` advances to the end of the last token consumed, so a row that borrows
its neighbour's figures also steps the cursor past that neighbour's *label*, and
the next iteration refuses the part with `row %q does not occur after %s`.
`checkGap` refuses text between rows unless it is a declared `wrapped_labels`
entry — a real exception, not a formality: CIP p40 needs three of them before a
read gets through at all. Between them a labelled block is well defended, and
the `len(toks) < ncols` message is the rarest of the three rather than the first
line of defence.

**The designed answer for what those miss is the geometry column guard**, and it
is the reason `-bbox` is extracted at all. With `column_headers` declared —
which every part of `mappings/livermore-budget-fy2026-2027.yaml` that reads rows
does —
`placementMessage` (`internal/mapping/geometry.go`) reports which column a
token's x actually lands in against the one the rule reads it as, and it fires
before `amount.Parse` ever reaches a neighbour's label word. `geometry_test.go`
pins the message shape.

Be careful with CIP p40 as the worked example, though: `mappings/` holds two
files, mapping the Budget Book and one page of the ACFR, so **no production rule
reads the CIP at all**. (That premise used to read "holds one file and it maps
the Budget Book only"; the conclusion survived the second document, the premise
did not.) The refusal above is reproducible for a CIP part you write yourself,
and that is the evidence — not a guard standing over committed facts.

Which is why the honest statement is that **nobody has enumerated what is left**
once all four are in play — a block-final row backed by an unlabelled total line
is the obvious candidate. `fisc-8ln` owns that residue.

`TestCIPp40SparseRowFailsClosedButDoesNotRead` DOES now reach the row read
(`fisc-i0d9`, closed). It used to anchor its block on the column-header line, so
`checkGap`'s leading-gap arm refused before any row was read and its one
assertion passed on that unrelated message quoting the first row's anchor name.
It now starts past the headers and asserts the value-count refusal by name.

**And the guard that fires there is not the geometry one.** The row yields 2
tokens against 8 columns, so `len(toks) < ncols` refuses first and geometry never
places anything — measured identically with `column_headers` declared and
without. That guard is also load-bearing against more than a wrong read:
neutering it panics on the `toks[:ncols]` two lines below.

Three successive attempts in this file to summarise this function were wrong in
three different ways; if you need the behaviour, read `labelledValues` and write
a probe, and do not trust this paragraph over the code.

poppler has no structured error channel: it writes free-form English to stderr
and exits 0. The manifest records every stderr line under `warnings`, and
reserves `errors` for non-zero exits and unparseable output. An empty `errors`
is not a promise that every page came out whole.

`tools/extract.py` deliberately does **not** read `data/sources.yaml`. It
discovers work from `data/pdf/<doc-id>.pdf` and records the source sha256 it
computed; `fisc verify` cross-checks that against the registry. Two parties
recording the hash independently is a real check — both reading the same file
would not be. Do not "simplify" this by giving Python a YAML parser.

Extraction output must stay byte-stable across runs: words sorted by position
rather than by the tool's emission order, canonical JSON with sorted keys,
bboxes rounded to 2dp, atomic writes. Changing any of that changes every
committed artifact, so it belongs in its own reviewed commit with
`extractor_version` bumped.

## Non-interactive shell commands

Use non-interactive flags; `cp`, `mv`, and `rm` may be aliased to `-i` and
will hang waiting for input that never comes.

```bash
cp -f source dest      # NOT: cp source dest
mv -f source dest      # NOT: mv source dest
rm -f file             # NOT: rm file
rm -rf directory       # NOT: rm -r directory
```

Others that prompt: `scp`/`ssh` need `-o BatchMode=yes`, `apt-get` needs `-y`,
`brew` needs `HOMEBREW_NO_AUTO_UPDATE=1`.

## Fetching the source PDFs

`www.livermoreca.gov` sits behind Akamai bot protection and returns HTTP 403
to a bare `curl` — an error page, not a network failure, so it is easy to
misdiagnose as a proxy problem. The full browser header set that works is in
[`docs/m0-spike.md`](docs/m0-spike.md).
