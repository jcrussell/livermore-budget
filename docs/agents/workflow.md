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

## Review cadence

Commit after review at logical points — not continuously, and not never.

- **Run `/code-review` before any commit that lands a new package**, and at
  each epic boundary.
- **Skip it for mechanical commits** — a pinned dependency, a `.gitignore`
  fix, a docs typo. Review has a real cost and those have no design surface.
- **Fix what the review finds before committing**, so the code and its review
  land together rather than as a fix-up commit.

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

## Commits

- Conventional Commits: `feat(mapping): ...`, `fix(extract): ...`, `docs: ...`.
- Reference beads as `Refs <id>` in the message body.
- Explain *why* in the body, especially when the reason is a document quirk
  that will not be obvious from the diff.
- Regenerated artifacts belong in the same commit as the change that caused
  them.
- `make pre-commit` runs fmt, vet, test **and lint**, and the symlink into
  `.git/hooks/pre-commit` is what makes it the contract. It warns rather than
  fails when `golangci-lint` is not on PATH, so a contributor with only Go can
  still commit; `make lint` alone still fails, because that is CI's required
  check. Before lint was in this target, CI was the first place a violation
  showed and `main` carried a red lint across three commits.
