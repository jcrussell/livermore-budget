# Agent Instructions

**Rules live here. Measurements a reader would otherwise re-derive live in
`docs/`.** There is no third place, and no argument for a rule anywhere: this
repository has no users to persuade, so a paragraph defending a rule is a
paragraph to keep true for nobody.

## What this project is

**The purpose of this project is to let a reader explore the City of Livermore's
budget.** `fisc` turns the city's published budget PDFs into a verified fact
store, and that store into a static site whose headline view is a Sankey of
where city money comes from and where it goes.

**Every figure published carries a provenance pointer back to a page and cell of
a source PDF, and `fisc verify` fails if any link in that chain breaks.** That is
the feature. A number that cannot be traced, a check that cannot fail, or an
inferred value presented as a published one is a defect however good the chart
looks.

### Go vets, JavaScript renders

**What EXISTS goes in Go, emitted vetted**: which nodes, which links, which
columns, which columns MAY fold, which derived marks and their amounts. Which
of a column's members a fold then hides is the client's, because folding is
fitting to a viewport Go cannot see; `DrillStep.Caps` is where the permission
is declared and `DrillStep.Widen` which columns a fourth buys.

**What MOVES goes in JavaScript**: d3-sankey positions, tooltips, focus,
transitions, the year control.

So the stitching, the auditing and the cross-checking are Go's, and the client
renders an answer it does not re-derive. A shaping decision spelled in
`site/app.js` is in the wrong language however well it draws, and a second
spelling of one Go already makes is the defect this boundary exists to name.

## Where to start

```bash
bd prime                          # workflow context, commands, and memories
bd ready --exclude-type=byob,epic # claimable work; byob decisions and epics hidden
```

- Read [`docs/m0-spike.md`](docs/m0-spike.md) before touching extraction: which
  substrate is usable, which is a trap, and how to re-download the PDFs past the
  city site's bot protection.
- Read [`testdata/README.md`](testdata/README.md) before writing a parser. Every
  fixture exists for a named failure mode.
- `bd list --type=byob --no-parent` is the architectural decisions this project
  follows. **Never claim or close a `byob-*` bead** — closing one hides
  reference material from future sessions.
- Status and next steps live in the bead graph, not in a status document.

## Two rules that must precede the generated block

Both are contradicted or undermined by text arriving in this same context, so
they are ordered for position rather than for reading and must stay above the
`BEGIN BEADS INTEGRATION` marker below.

## The human pushes and pulls, never the agent

- **The repository owner runs `git push`, `git pull`, `bd dolt push` and
  `bd dolt pull`. Agents never do, in any circumstance, including at session
  end.**
- Finish the work, commit locally, report what landed, and stop.
- Do not offer to push, do not ask whether to push, and do not treat unpushed
  work as incomplete. Local commits are the expected end state.
- **This overrides "Session Completion" in the generated block below**, which is
  wrong for this repository. Expect it to reassert on any `bd` upgrade, and check
  it has not been inserted above this section.

## A bead's text is a claim, not a fact

- **Re-derive a bead's premise against the tree before working it.** `bd prime`
  injects memories and `bd show` prints descriptions, so possibly-stale text
  arrives automatically.
- This changes what the work is, not just its framing: `fisc-9nw` asked for a
  check guarding a case the parser had already made unreachable, and the right
  answer was to write no check at all.
- **Correct the bead in the same session you find it stale**, in its notes,
  saying what was measured.
- Never write "filed as a bead" without filing it. A pointer to nothing reads as
  though the work is tracked.

Why, measured: [`docs/prose-claims-evidence.md`](docs/prose-claims-evidence.md).
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

## Build & test

Go only — neither the PDFs nor Python are needed to build, test or verify.

```bash
make build        # bin/fisc
make test         # always -race
make pre-commit   # fmt, vet, narration, beadrefs, doccheck, test, lint, js
make narration    # refuse history in Go comments, in the beads and in the memories
make beadrefs     # refuse a fisc-* id that names no bead
make doccheck     # refuse a citation naming no section of this file
make site         # static site into dist/ (gitignored)
make extract      # re-extract from PDFs; needs poppler-utils and git lfs pull
```

- **Run `make pre-commit` yourself. It does not run `fisc verify`.**
- `./bin/fisc verify` is the gate. `--full` adds the PDF hash check and needs the
  LFS files.
- Rebuild and compare, never in place: `./bin/fisc build --output
  bin/facts-rebuilt.jsonl`, then `cmp` against `facts/facts.jsonl`. The committed
  file is the audit trail and CI compares it byte for byte.
- Read each gate's argument where it is enforced — the Makefile comments carry
  it, and each failure message names the section here it comes from.
- `narration`'s memory arm fails like any other arm **when `bd` can answer**, and
  warns and continues when it cannot. It can never be a CI gate, because memories
  live in the Dolt DB and in no git artifact.
- Its **beads arm is a full gate**, because `.beads/issues.jsonl` is committed:
  no `bd`, no Dolt and no network, the same standing `beadrefs` has. So the
  target is two real gates and one advisory arm, and a green run means different
  things for each.
- `pre-commit` warns and continues when `golangci-lint` or node is absent.
  `make lint` and `make js` on their own still fail, because those are CI's
  required checks.
- **CI is the gate; a local hook is a convenience.** `make hooks` installs into
  the directory git actually reads (`core.hooksPath` when set, `.git/hooks`
  otherwise) and **refuses when that directory is tracked**, as `.beads/hooks` is
  here — a committed hook would make `make pre-commit` mandatory for everyone who
  has bd. So in this checkout the fisc gate is not installed at all — bd's own
  tracked hook still runs — and you must run `make pre-commit` yourself.

## Architecture

`data/pdf` → `tools/extract.py` (poppler) → `data/extracted` (786 pages of
`-layout` text plus `-bbox` geometry, hashed in a manifest) → `mappings/*.yaml`
resolved by `internal/mapping` → `facts/facts.jsonl` (content-addressed facts
carrying `doc_id / page / offset / token`) → `internal/project` → `dist/`.

See `README.md` for the diagram and `docs/sankey-contract.md` for the output
contract.

## Issue tracking

Beads (`bd`) is the tracker. Do not use TodoWrite, TaskCreate or markdown TODO
lists.

```bash
bd ready --exclude-type=byob,epic  # claimable work; an epic is never claimable
bd show <id>                       # detail, dependencies, and blockers
bd update <id> --claim             # claim before starting
bd close <id> --reason "..."       # close with what actually happened
```

- **Do not read the roadmap off this file.** `bd list --type=epic --status=all`
  is the roadmap; every enumeration written here has gone stale.
- **A closed epic means its end condition was met**, not that every bead under it
  is done. Remaining children are open at top level, and the dotted ids record
  where they came from.
- **An epic closing does not make its dependents workable.** Check before
  planning around one.
- **Priority drifts.** Look for a bead whose `P1` contradicts its own note.
- **A bead says what needs DOING, not what happened** — *History's home is git*
  applied to the tracker. A bead that reads as a changelog buries the work under
  it. Drop the commit shas, the account of what landed, and the corrections of
  earlier notes; keep a measurement only where it is evidence a reader would
  otherwise re-derive or re-litigate.
- **Correct a stale bead in place rather than annotating it.** A note saying the
  text above is wrong leaves the wrong text as the thing a session reads first.
  Where a closed bead's decision is reversed, `bd supersede` it with the one
  that replaces it — a note cannot reach a close reason.
- Use `bd remember` for cross-session knowledge, not MEMORY.md files. A durable,
  specific finding goes in the relevant bead, in this file, or in the contract
  doc under `docs/` it belongs to.

Why, measured: [`docs/prose-claims-evidence.md`](docs/prose-claims-evidence.md).

## Review is a loop, not a pass

- Skip review entirely for mechanical commits: a pinned dependency, a
  `.gitignore` fix, a docs typo. Everywhere else a single pass is not the gate.
- Run `/code-review` over the **range**, not the last commit. A lane's goldens,
  its check, its export seam and its client are one claim.
- **On a branch of several lanes, run the loop once at the merge**, not once per
  lane. Same triage, same three-to-five passes, over the whole branch. A later
  lane reshapes the declarations an earlier one wrote, so a per-lane pass spends
  itself on shapes that are gone by the time the branch lands.
- **Deferring the review never defers the check.** Each lane still ships its
  jscheck arm and its stated mutation in its own commit (`fisc-rx1d`), and every
  finding a lane does not fix is still a bead in the session that found it. A
  lane that lands with neither is a lane nothing can see go wrong, whenever the
  review happens.
- Triage before you fix. Every finding is WRONG-OUTPUT, FAIL-OPEN, FALSE-CLAIM,
  DESIGN or HYGIENE.
- **Fix WRONG-OUTPUT and FAIL-OPEN in the pass; file the other three**, unless
  the fix is one line in a file the pass is already touching. A pass that fixes
  everything writes 150-450 unreviewed lines, and those lines are where the next
  pass's findings come from.
- Each fix lands with the test that would have caught it, in the same commit as
  its review — not as a fix-up commit.
- `make pre-commit` and the lane's mutation proofs green before re-reviewing.
- Re-review the range **including the fixes**. This is the step that finds most.
- **Stop when a pass returns no WRONG-OUTPUT and no FAIL-OPEN finding** — not
  when it returns nothing. It will not return nothing; the smallest yield the log
  records is two.
- **Three passes minimum, five maximum.** The ceiling is not a target.
- At the cap, report what the last pass found and let the owner decide whether to
  narrow the commit rather than keep patching it.
- **The last pass's own fix ships unreviewed unless you do something about it.**
  Give that diff a narrow extra pass, or keep it small enough to say why it needs
  none. `fisc-i92i`.
- **Every finding you do not fix becomes a bead, in the same session, before you
  report.** Declined, deferred, out of scope or too small — all the same rule.
  P3 and P4 exist so "not worth doing now" has somewhere to live.
- Refuse "I will fix it in the next commit": file it anyway and close it in that
  commit. Reporting to the owner is not filing, and neither is a commit message.
- A finding that touches a golden gets one extra step: regenerate, then read the
  diff against a rule cheap enough to check by eye, before re-reviewing. There is
  deliberately no `-update` flag in this repo.
- Watch for a fix that propagates a **number without its definition**, and for a
  fix that introduces the next pass's finding.
- **Review does not cover this project's main risks.** That the amount parser
  rejects the right tokens, that extraction is byte-deterministic, that mapped
  rows sum to the totals the documents print, and that a locator still resolves
  are guarded by corpus scans, arithmetic against published figures, and
  re-run-and-compare. Those stay mandatory whether or not a review ran.
- Prove a claim about these documents with arithmetic, not intuition.

Why, measured: [`docs/review-loop-evidence.md`](docs/review-loop-evidence.md) —
115 findings classified, and the loop is its own second-largest defect source.

## Prove it can fail

- **The mutation is the proof.** Reproduce the defect green, then show it red,
  and put both in the commit message.
- The doctrine is written down: `internal/check/vacuity.go` — a vacuous check is
  declared or `--strict` fails on it. `internal/check/check.go` sets out what
  these checks can and cannot witness. `tools/jscheck/seam.mjs` checks the thing
  that checks it.
- **Ask what the fixture is hiding, not whether the test passes.** A test that is
  **green because the gate fired** earlier — rejected before it reached the code
  under test — is indistinguishable by exit code from one green because the
  defect was prevented, and the two guarantee completely different things.
- When one of those turns up, look for its siblings — the fixture that hid one
  usually hides several.

Why, measured: [`docs/review-loop-evidence.md`](docs/review-loop-evidence.md).

## Before you quote a number

- **First ask whether to write it at all.** Do not report a count of the tree's
  own contents — facts, nodes, links, checks, rules, map entries. It goes stale
  on the next commit and tells a reader nothing they could not get by looking.
- Two kinds earn their place and nothing else does: **a count against the
  documents** ("pp.85-125's 78 rows", "the corpus is 786 pages"), and **a count
  that is the evidence for a decision**.
- Pin every surviving count to something that re-measures it —
  `tools/jscheck/layout.mjs` is what that looks like. Where nothing can, name the
  commit it was taken at.
- **Rebuild `bin/fisc` and run it before quoting a check count**, and read the
  gate line off the run you are describing. A count copied from anywhere — this
  file, another commit, memory — is the defect this section exists to name.
- **A commit message describing a fix is a claim about the tree.** Grep for the
  fix before writing the sentence.
- **One edit per script, or check each edit's exit status.** A heredoc that
  raises halfway leaves a tree that still builds and passes every test.
- **When you correct a figure in prose, grep for its copies in the same commit —
  then read each hit** rather than replacing it. The same numeral states
  different propositions in different files, which is why this cannot be a check.
  `fisc-xbd4`.
- `make beadrefs` checks ids in tracked prose and comments; `make doccheck`
  checks citations of this file's sections. Neither reads a commit **message**,
  so read those back yourself.
- **At a lane boundary, audit every "this commit fixes X" in the range** against
  the tree in one script. Pin the range to two commit ids and start at `<first>^`.

Why, measured: [`docs/prose-claims-evidence.md`](docs/prose-claims-evidence.md).

## Commits

- Conventional Commits: `feat(mapping): ...`, `fix(extract): ...`, `docs: ...`.
  **Compound the scope when a change spans packages** — `fix(check) +
  fix(mapping): ...` — rather than picking one and hiding the other.
- `Refs <id>` in a trailer paragraph, or `Closes <id>` when the commit finishes
  one. Both are used here.
- Explain *why* in the body, especially when the reason is a document quirk that
  will not be obvious from the diff.
- Regenerated artifacts belong in the same commit as the change that caused them.
- **Numbered findings** — `(1)`, `(2)` — when one commit fixes several, each
  opening with a capitalised lead clause naming the defect rather than the change.
- **Measurements inline, labelled as measured.** A number in a commit message is
  a claim like any other.
- **State the mutation**: what was reverted, and what went red.
- **The gate line**, near-formulaic: *"facts.jsonl unmoved; fisc verify N passed,
  0 failed"* — with N read off the run you just did.
- **Credit where a finding came from**: *"Found by /code-review over this range."*
  It tells the next reader whether a fix was designed or discovered.

## Provenance invariants

Breaking one is a defect even when tests pass.

- **Amounts are integer cents.** No float anywhere from an extracted cell to a
  published total. These values are compared against printed figures; float drift
  makes those comparisons meaningless.
- **Absent is not zero.** `-` means the line exists and is zero; an empty cell
  means the line does not apply. Conflating them invents rows. `amount.Parse`
  returns `ErrAbsent` for the latter, and no rule in the tree opts out. A table
  whose blanks really do mean zero has to say so in its own rule, and adding the
  opt-out is part of that change rather than something already waiting for it.
- **Fail closed on ambiguity.** PDF extraction corrupts numbers into plausible
  wrong values rather than errors. Every shape not positively recognised is an
  error. Never guess a value to keep a pipeline green.
- **Published and derived are different things.** Derived nodes carry
  `derived: true` with a `rationale` and `source_note`, and `verify` fails
  without them.
- **Identity and integrity are separate.** A locator says *which* row this is and
  must be content-independent; a content hash says *whether it changed*. One
  value cannot do both — Budget Book p67 prints four byte-identical all-dash
  lines, so identity has to come from position.

### A forecast is not a fact

- **Nothing computed goes into `facts/facts.jsonl`.** A fact is one figure the
  city printed.
- Three checks make that a guarantee, none of which consults `Fact.Derived`:
  `fact-token-reparses` re-parses each fact's own token and fails an empty one;
  `fact-offset-points-at-token` requires the extracted page text at the fact's
  offset to *be* that token; `fact-ids-recompute` needs a `rule_id` and a
  `doc_id` in the hashed tuple.
- **`fact-offset-points-at-token` is the arm that matters** — a made-up figure
  can be made to re-parse, but no page prints it.
- `Derived: true` on a fact means a re-reading or re-classification of a figure
  the city printed at a page and an offset, never a computed value. **It must
  never become an exemption from the checks above.**
- A projection or scenario may derive figures under its own rules;
  `derived-nodes-justified` requires a rationale and a source note on every
  derived node. See `fisc-nvw` for where a forecast is allowed to live.

## Where writing goes

- **Three homes, and each claim has exactly one.** A RULE goes in this file,
  once. A MEASUREMENT a reader would otherwise re-derive or re-litigate goes in
  `docs/`, and states no rule. What is true of THIS declaration goes in its doc
  comment.
- This file arrives in every session unasked; a `docs/` file does not. So a rule
  an agent must follow without being told to look it up cannot live in `docs/`,
  and a measurement nobody needs in hand should not live here.
- **Do not argue for a rule.** The rule is the decision; the argument is what it
  cost to reach, and it is in git. A `docs/` page earns its place by holding a
  number that is expensive to recover — 115 findings classified, twenty mutations
  and what each caught — and not by explaining why the rule is right.
- **A comment is tactical**: the surprise, the invariant this seam upholds, the
  mutation that proves the guard. If a comment has grown into an essay it is
  documentation in the wrong place — move the argument to `docs/` and leave the
  claim.
- **A comment names a symbol.** It does not restate what the symbol does and does
  not say where the symbol lives. Both are second sources nothing keeps in step.
- **The exception is surprise.** Document another API only where it does not make
  sense on its face, and write what surprised *you* rather than what the API does.
- **Data and artifact paths are not symbols and stay** — `data/funds.yaml`,
  `testdata/pages/budget-p0067.txt`, `mappings/*.yaml`, `site/app.js`. Only a symbol's
  `.go` path goes.
- **History's home is git.** ALL PROSE THIS PROJECT KEEPS STATES WHAT IS TRUE
  NOW — a doc comment, a `docs/` page, this file, a bead's description or notes,
  an injected memory, a README, a mapping's comment. A sentence about what a
  passage, a declaration or a bead USED TO SAY is a second claim, about the past,
  that nothing can check and nothing keeps in step, sitting exactly where a
  reader looks for the present. So no errata — *"an earlier version of this
  comment said X"* — and no `Found by /code-review` credit in source. Where an
  erratum carries a rule, keep the rule and drop the history.
- **So correct prose in place rather than annotating it.** A passage saying the
  text above is wrong leaves the wrong text as the thing a reader meets first,
  which is the whole of what annotating it was meant to fix.
- **What survives the rule is a measurement**, and only where it is evidence a
  reader would otherwise re-derive or re-litigate. That is a claim about the
  present which is expensive to recover, not a record of what happened.
- **The commit message is the exception, because it IS the history.** It states
  what changed, what the mutation was and what went red. It is still a claim
  about the tree rather than a diary: no account of what an earlier commit, bead
  or comment said.
- The rule reaches the injected memories for a reason worth keeping in view:
  `bd prime` delivers them whether or not anyone opens the file they are about,
  so a memory's erratum arrives in every session unasked.
- **Do not insert code between a doc comment and its declaration.**
- **Drop narration in the file you were already editing, never in a sweep** — and
  *drop* it rather than rewriting the comment around it.
- A frozen output **contract** is neither a rule nor evidence. **Its SHAPE lives
  in `schema/`**, as a JSON Schema something can check the bytes against; `docs/`
  keeps the ARGUMENT for it, which is what a schema cannot say. A contract with
  no package yet keeps its prose in `docs/`; one with a package puts the claim in
  that package's doc comment.
- **Prefer the machine-checkable form wherever there is one.** A shape stated in
  prose, again in a struct tag, and a third time in a hand-written key check is
  three spellings none of which can be held against the data. `schema/fact.schema.json`
  is what the alternative looks like: it refused a float `amount_cents`, an empty
  `token` and a string `fund` on the day it landed.

Why, measured: [`docs/prose-claims-evidence.md`](docs/prose-claims-evidence.md).

## Go

Layout and idioms follow the byob decisions; read them with `bd show byob-<id>`
rather than inferring from the code.

- Three-tier layout (`byob-layout.1`): `cmd/fisc` → `internal/fisccmd` →
  `pkg/cmd/root` → `pkg/cmd/<feature>`.
- Each command is `Options` + `NewCmdXxx(f, runF)` + private `xxxRun`
  (`byob-command-shape.1`), with `Options.Validate()` first, failing via
  `cmdutil.FlagErrorf` before any side effects (`byob-input-validation.5`).
- Commands return errors and never call `os.Exit` (`byob-errors.1`). The runner
  owns the error→exit-code mapping.
- One `Factory` built in `main`, threaded everywhere; expensive dependencies are
  lazy closures so `--help` touches no filesystem (`byob-factory-di.1`). A test
  asserts exactly that; keep it passing.
- Data goes to `Out`, everything else to `ErrOut` (`byob-iostreams.3`).
- `CGO_ENABLED=0`, pure Go, assets via `go:embed` (`byob-release.8`).
- Stdlib first (`byob-release.10`). This tree's direct dependencies are cobra,
  go-cmp and `go.yaml.in/yaml/v3` — the last decided by `fisc-j8f` — and anything
  else needs its own decision bead in the same change. **byob's blessed set is
  not this tree's.** It also names modernc sqlite and goreleaser, which this
  project does not use and which are not pre-approved here.

## Testing

- Tests ship in the same commit as the code they cover (`byob-testing.4`).
- `google/go-cmp`, not testify (`byob-testing.2`). `cmp.Diff(want, got)`.
- Assert on behavior, not call counts (`byob-testing.3`).
- **Go tests never require Python, the source PDFs, or the network.** Use the
  fixtures in `testdata/`, which are real artifacts copied from
  `data/extracted/`.
- Those fixtures are *copies* and `make extract` does not touch them: re-copy by
  hand when the extraction changes, and their sha256s must equal the ones the
  source document's `manifest.json` records. **Re-copy; never edit an assertion
  to fit a stale fixture** — a drifted fixture keeps its tests green against a
  substrate that no longer exists (`fisc-yqv.5`).
- Smoke-test the built binary, not only the packages. A typo'd command once
  exited 0 and printed help; every unit test passed and `./bin/fisc biuld` caught
  it.

## The node boundary

- `site/app.js` is served to readers exactly as committed — no bundler, no npm,
  no module system. Keep it that way.
- `make js` runs `tools/jscheck`, which loads **the shipped `app.js`** and the
  vendored d3 into a node `vm` and re-measures the figures it quotes about
  itself. It reaches into the file rather than copying functions out of it,
  because a copy would check the copy and let the original drift.
- **Node stays off the deploy path.** `make build`, `make site` and `fisc export`
  never run it; there is no `package.json` and no `node_modules`. A contributor
  with only Go can still build, test and land a change.
- **A change to `app.js` ships its check in the same commit.** A client change
  with no jscheck beside it is a change nothing can see go wrong — do not open
  review on one.
- **Quote what the current code does.** A claim whose baseline no longer exists
  in the tree cannot be checked; every such figure is now pinned rather than
  bounded, so a comment edited without re-measuring fails `make js`.
- `fisc-rx1d` carries the audit of which of `app.js`'s current paths no module
  drives.

Why, measured: [`docs/review-loop-evidence.md`](docs/review-loop-evidence.md).

## The extraction boundary

- **Extraction is not a Go responsibility and must not become one.** Go never
  shells out to `tools/extract.py`, and `fisc` reads only the committed artifacts
  under `data/extracted/` — which is what lets CI verify without a venv or an LFS
  checkout.
- `tools/extract.py` is the only Python here: standard library only, poppler as a
  system package, PyPI unreachable from the extraction environment.
- **Two substrates per page, both needed**: `-layout` text for the printed column
  grid, `-bbox` geometry for column identity. Page text is `.txt` and not `.md`
  because GitHub's blob view collapses the spaces that *are* the grid.
- **A rule that must tell an absent cell from a zero one has to say so itself,
  and must not default a missing token to zero** — that turns an absent cell into
  a printed one and invents a row. `fisc-8ln`.
- **Do not trust a prose summary of `(*Resolver).labelledValues`.** Read it in
  `internal/mapping/resolve.go` and write a probe; three successive attempts to
  summarise it here were wrong in three different ways.
- **An empty `errors` in the manifest is not a promise every page came out
  whole.** poppler writes free-form English to stderr and exits 0; every stderr
  line is recorded under `warnings`.
- `tools/extract.py` deliberately does **not** read `data/sources.yaml`. It
  records the sha256 it computed and `fisc verify` cross-checks it against the
  registry — two parties recording it independently is a real check. Do not
  "simplify" this by giving Python a YAML parser.
- **Extraction output must stay byte-stable across runs**: words sorted by
  position, canonical JSON with sorted keys, bboxes rounded to 2dp, atomic
  writes. Changing any of it changes every committed artifact, so it belongs in
  its own reviewed commit with `extractor_version` bumped.

Why, measured:
[`docs/extraction-substrate.md`](docs/extraction-substrate.md).

## Non-interactive shell commands

`cp`, `mv` and `rm` may be aliased to `-i` and will hang waiting for input that
never comes.

```bash
cp -f source dest      # NOT: cp source dest
mv -f source dest      # NOT: mv source dest
rm -f file             # NOT: rm file
rm -rf directory       # NOT: rm -r directory
```

`scp`/`ssh` need `-o BatchMode=yes`, `apt-get` needs `-y`, `brew` needs
`HOMEBREW_NO_AUTO_UPDATE=1`.

## Fetching the source PDFs

`www.livermoreca.gov` sits behind Akamai bot protection and returns HTTP 403 to a
bare `curl` — an error page, not a network failure, so it is easy to misdiagnose
as a proxy problem. The full browser header set that works is in
[`docs/m0-spike.md`](docs/m0-spike.md).
