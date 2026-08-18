# Project Instructions for AI Agents

**Full guidance lives in [`docs/agents/`](docs/agents/).** Start with
[`docs/agents/README.md`](docs/agents/README.md); the detail is in
[`workflow.md`](docs/agents/workflow.md) and
[`conventions.md`](docs/agents/conventions.md).

Orient with `bd prime`, then `bd ready --exclude-type=byob`.

Two rules are repeated here rather than only linked, because the generated
beads block below contradicts the first of them and a pointer would not
defeat text living in this same file.

## The human pushes and pulls, never the agent

Do not run `git push`, `git pull`, `bd dolt push`, or `bd dolt pull` — not on
request-completion, not at session end, not ever. Commit locally, report what
landed, and stop. Unpushed local commits are the expected end state of an
agent session, not unfinished work.

**This overrides the "Session Completion" section in the generated block
below**, which claims work is incomplete until `git push` succeeds. That is
wrong for this repository.

## Review before committing new packages

Run `/code-review` before any commit that lands a new package, and at each
epic boundary. Skip it for mechanical commits. Fix findings before committing.
See [`docs/agents/workflow.md`](docs/agents/workflow.md) for the full cadence
and for why review does not replace this project's empirical checks.

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
make pre-commit   # fmt, vet, test — symlink it to .git/hooks/pre-commit
make site         # static site into dist/ (gitignored)
make extract      # re-extract from PDFs; needs poppler-utils and git lfs pull
```

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

## Conventions & Patterns

Four invariants, in `docs/agents/conventions.md`:

- **Amounts are integer cents**, never float. The corpus corrupts figures into
  plausible wrong values rather than errors, so `internal/amount` recognises a
  closed set of shapes and errors on everything else.
- **Absent is not zero.** A printed `-` is a published zero; a blank means the
  line does not apply.
- **Fail closed on ambiguity.** A rule that stops resolving is an error.
- **Published is not derived.** Inferences carry `derived: true` plus a
  rationale, or `fisc verify` fails.

A claim about these documents is proved with arithmetic, not intuition — the
worked example is `TestLeadingMinusIsReallyPositive`, where ACFR p177 reconciles
only if `-512,946` reads positive. Review does not substitute for that evidence;
see `docs/agents/workflow.md`.
