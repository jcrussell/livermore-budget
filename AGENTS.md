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

The corpus knows this about itself. **Five** of the injected memories carry a
line whose only job is to say earlier text has gone stale — *"the bead text
describing it as blocked is historical"*, *"text on those beads describing work
as pending is historical"*, *"same stale-premise class as p76: claims written
against the old extractor outlived it"* — and **eight** commits in the log have
correcting stale text as their whole purpose.

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
make pre-commit   # fmt, vet, test, lint, js
make hooks        # install the local pre-commit hook, once per checkout
make site         # static site into dist/ (gitignored)
make extract      # re-extract from PDFs; needs poppler-utils and git lfs pull
```

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

The roadmap is ten epics. `E1 Foundations` through `E8 Further projections` are
the plan as first written; `E0 Extraction replacement` (`fisc-yqv`) and `E9 The
drill-down reaches a reader` (`fisc-5miz`) were added afterwards and are both
closed, so a count of eight is a claim about the plan rather than about the
tracker. Dependencies are wired so `bd ready` surfaces only genuinely unblocked
work.
Each task cites the byob decision it follows.

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

Everywhere else, **a single pass is not the gate**. Measured over one session's
three lane boundaries: nine passes, 23 findings. Three of those were defects
introduced by an *earlier pass's own fix*; a later session added the fourth row
below —

| the fix | what the next pass found in it |
|---|---|
| `HasSuffix(base, "/")`, itself the fix for a string-prefix bug | still accepted an ancestor base: `facts/` passed, and the client composed `facts/p0066.jsonl` |
| a reworded `fact.FromValues` comment, claiming a backstop | the condition it tested could not reach the shape it claimed to catch |
| two commits saying "filed as a bead" | neither bead existed |
| a test written for a summary defect, asserting `Contains("78 declared fund(s)")` | the malformed string it was written for satisfies that substring, so it passed on the bug |

The rest were defects in the original work — but they were found across all
three passes, not the first. The two worth recognising by shape: a guard
written as fail-closed that shipped **fail-open**, and a test that passed
identically with and without the fix it was named for.

Severity decayed across passes and never reached zero. Hence the loop.

**And it does not always decay.** The pp.85-125 lane (`cd1192c..261c78f`) went
5, 4, **5** — the third pass found *more* than the second, and **every one of
its five was a defect in the original commit that two passes had read past.**

Four were false statements in committed text: a page list naming five pages that
carry no such declaration; a README cell whose back-reference pointed at the row
above the one it meant; a summary clause reading `a further 78` with nothing
before it; and a **derived** figure cited as a printed one — three commits after
the test beside it said in as many words that the figure appears on no page in
the corpus. Two of those four are strings `fisc verify` prints on every run, and
the last is the "published is not derived" invariant broken in published text.
The fifth was a map read with no ok-check, which left the one test guarding
against a wrong-column read unable to fail on it.

The table's fourth row comes from the same pass and is the *fix* side rather
than the finding side: the first pass's fix shipped a test for the `a further
78` string asserting a substring that the malformed string satisfies, so it
passed on the bug it was written for.

A three-pass cap would have shipped all five. So the count is a floor as well as
a ceiling — see below.

*(Correction, recorded here rather than by amending, because the log is the
audit trail: `261c78f`'s own message says two of that pass's findings were
"introduced by an earlier pass's own fix" and attributes the malformed summary
clause to the second pass. Checked against `git show cd1192c` — the clause was
in the original commit, and what an earlier fix introduced was the test that
failed to catch it. One, not two, and on the fix side rather than the finding
side. The habit that produced the error is the one
[Before you quote a number](#before-you-quote-a-number) exists for, applied to a
commit message about review rather than about the corpus.)*

### The loop

1. **`/code-review` over the range, not the last commit.** A lane's goldens,
   its check, its export seam and its client are one claim; reviewing the first
   commit alone cannot see whether the client renders what the projection
   publishes.
2. **Fix the findings.** Each fix lands with the test that would have caught it
   — see [Prove it can fail](#prove-it-can-fail) — and the code and its review
   land together rather than as a fix-up commit.
3. **`make pre-commit` and the lane's mutation proofs green** before
   re-reviewing. A red gate means step 2 is not finished.
4. **Re-review the range including the fixes.** This is the step that matters:
   it is where the table above comes from.
5. **Stop** when a pass returns nothing, or when the only findings left are
   ones you can state a reason for declining. **File the declines as beads** —
   `fisc-i38`, `fisc-oz4` and `fisc-8fr` all exist because a review pass
   surfaced something real that was not worth taking then.
6. **File whatever is still open when you stop**, whether you are stopping
   because you declined it or because you hit the cap. See
   [Nothing leaves a pass unfiled](#nothing-leaves-a-pass-unfiled).

### Three passes at least, five at most

**Three is the floor.** Two passes is one pass plus a check that the fixes
parse. The table above and the pp.85-125 counts both say the same thing: the
defects an earlier fix introduces are only visible to a pass that runs after it,
and the second pass is usually still finding original defects rather than
introduced ones.

**Five is the ceiling**, raised from three because three was demonstrably too
low on a lane whose third pass found five real defects, all of them in the
original commit. The ceiling is not a target — stop the moment a pass returns
clean, which is often the third.

**A late pass still finding real defects is a signal about the change, not about
the review.** At the cap, report what the last pass found and let the owner
decide whether to narrow the commit rather than keep patching it. Every boundary
in the measured session hit its cap with findings still outstanding, and that is
information the owner should have rather than something to absorb silently.

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

Every claim in a comment, a commit message or a bead is checkable, and this
project treats an unchecked one as a defect. Three traps:

- **`make pre-commit` does not run `fisc verify`.** Rebuild `bin/fisc` before
  quoting a check count. One session reported "38 passed" after landing a check
  that made it 39, from a binary built before the check existed.
- **Rebuild-and-`cmp`, don't rebuild in place.** `./bin/fisc build --output
  bin/facts-rebuilt.jsonl` then `cmp` against `facts/facts.jsonl`; the
  committed file is the audit trail and CI compares it byte for byte.
- **A commit message describing a fix is a claim about the tree, and so is
  every id in it.** Grep for the fix before writing the sentence, and read the
  id back — `bd create` prints the new bead's id and it is not guessable.

  Both halves have failed here. `261c78f` said it had reworded a citation and
  spelled out a README cell; the fifth review pass found both unchanged. The
  cause was mechanical and will recur: a batch of scripted edits with an
  assertion in the middle aborted at the second, the later edits never ran, and
  the message had been drafted from the plan rather than from the diff. Two
  commits later, a bead correctly filed was cited under an invented id.

  Both are the class of writing "filed as a bead" without filing one, and both
  are worse than a missing note in the same way: the claim reads as *done*, or
  as *tracked*, so nobody goes looking. The invented id is the worst of the
  three, because the work really is tracked and only the pointer is dead.

The gate line most commits here end with is those two together: *"facts.jsonl
unmoved; fisc verify 40 passed, 0 failed."* The count in that template is itself
the trap above — it has been 33, 34, 39, 40 and 41 in this file's lifetime, and
was written as 39 here while the log's last eight commits said 40. Rebuild and
read it; do not copy the template's number.

**At a lane boundary, audit the whole range's claims at once.** Every "this
commit fixes X" in the range, checked against the tree in one script. Over
`cd1192c^..HEAD` that was 23 claims and 2 were false — both of them fixes
asserted in a message that never landed, and both found by review rather than by
the author.

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
  (Quoted from the log, so the 39 is what was true then and is not a live
  count.)
  A number in a commit message is a claim like any other.
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
  the 26 page fixtures under `testdata/pages/` (24) and
  `pkg/cmd/build/testdata/pages/` (2) have to be re-copied by hand when the
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
to tell an absent cell from a zero one must say so itself.

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
