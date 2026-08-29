# Workflow

## Remote sync — the human pushes and pulls, never the agent

**The repository owner runs `git push`, `git pull`, `bd dolt push`, and
`bd dolt pull`. Agents never do, in any circumstance, including at session
end.**

Finish the work, commit it locally, report what landed, and stop. Do not
offer to push, do not ask whether to push, and do not treat unpushed work as
incomplete — local commits are the expected end state of an agent session.

This overrides the generated beads block in `AGENTS.md` and `CLAUDE.md`, which
asserts that work is not complete until `git push` succeeds. That assertion is
wrong for this repository. The override is kept inline in both root files
rather than only here, because the text it contradicts lives in those same
files and a pointer would not defeat it. Expect the generated block to reassert
itself on any `bd` upgrade.

## Issue tracking

Beads (`bd`) is the tracker. Do not use TodoWrite, TaskCreate, or markdown
TODO lists.

```bash
bd ready --exclude-type=byob,epic  # claimable work; an epic is never claimable
bd show <id>                       # detail, dependencies, and blockers
bd update <id> --claim             # claim before starting
bd close <id> --reason "..."       # close with what actually happened
```

The roadmap is eight epics, `E1 Foundations` through `E8 Further projections`,
with dependencies wired so `bd ready` surfaces only genuinely unblocked work.
Each task cites the byob decision it follows.

Use `bd remember` for cross-session knowledge rather than MEMORY.md files. When
a finding is durable and specific — an arithmetic proof, a document quirk — put
it in the relevant bead's description or in `docs/`, where the next session
will actually encounter it.

### A bead records what was true when it was written

**Re-derive a bead's premise against the tree before working it.** This is not
hygiene; it changes what the work is. Two from one session: `fisc-9nw` asked
for a check guarding an empty `kind`, which the parser had since made
unreachable — the right answer was to write no check at all. `fisc-5hxr` was
built on a cost trade-off between a cheap option and a fuller one; measured,
every link resolved to one or two pages, so the fuller option was the small one
and the trade-off did not exist.

The corpus knows this about itself. **Five** of the injected memories carry a
line whose only job is to say earlier text has gone stale — *"the bead text
describing it as blocked is historical"*, *"text on those beads describing work
as pending is historical"*, *"same stale-premise class as p76: claims written
against the old extractor outlived it"* — and **eight** commits in the log have
correcting stale text as their whole purpose.

So: **correct the bead in the same session you find it stale**, in its notes,
saying what was measured. And do not write "filed as a bead" in a comment or a
commit message without filing it — a pointer to nothing is worse than no
pointer, because it reads as though the work is tracked.

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
  [A bead records what was true when it was written](#a-bead-records-what-was-true-when-it-was-written).

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
only the positive reading reconciles to the printed total. The test carries the
arithmetic. That is the level of proof a claim about these documents needs.

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
project treats an unchecked one as a defect. Two traps:

- **`make pre-commit` does not run `fisc verify`.** Rebuild `bin/fisc` before
  quoting a check count. One session reported "38 passed" after landing a check
  that made it 39, from a binary built before the check existed.
- **Rebuild-and-`cmp`, don't rebuild in place.** `./bin/fisc build --output
  bin/facts-rebuilt.jsonl` then `cmp` against `facts/facts.jsonl`; the
  committed file is the audit trail and CI compares it byte for byte.
- **A commit message describing a fix is a claim about the tree.** Grep for it
  before writing the sentence. `261c78f` said it had reworded a citation and
  spelled out a README cell; the fifth review pass found both unchanged. The
  cause was mechanical and will recur: a batch of scripted edits with an
  assertion in the middle aborted at the second, the later edits never ran, and
  the message had already been drafted from the plan rather than from the diff.
  This is the same class as writing "filed as a bead" without filing one, and
  it is worse in one way — the claim reads as *done* rather than as *tracked*,
  so nobody goes looking.

The gate line most commits here end with is those two together: *"facts.jsonl
unmoved; fisc verify 39 passed, 0 failed."*

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
  A number in a commit message is a claim like any other.
- **The mutation stated**, per [Prove it can fail](#prove-it-can-fail): what
  was reverted, and what went red.
- **The gate line**, near-formulaic: *"facts.jsonl unmoved; fisc verify 39
  passed, 0 failed."*
- **Credit where a finding came from**: *"Found by /code-review over this
  range."* It tells the next reader whether a fix was designed or discovered.
- `make pre-commit` runs fmt, vet, test, lint **and the app.js checks**. It
  warns rather than fails when `golangci-lint` or node is absent, so a
  contributor with only Go can still commit; `make lint` alone still fails,
  because that is CI's required check. Before lint was in this target, CI was
  the first place a violation showed and `main` carried a red lint across three
  commits.
- **`make hooks` installs the local pre-commit hook, and CI is still the gate.**
  `.git/hooks` is not tracked, so the hook is per-checkout setup that nobody
  may have run — this text used to say the symlink "is what makes it the
  contract" while no checkout anyone looked at had one, which is a workflow
  document asserting a guard that did not exist. Run `make pre-commit`
  yourself; do not assume a hook ran. Note it does **not** run `fisc verify`.
