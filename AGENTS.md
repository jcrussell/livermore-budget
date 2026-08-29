# Agent Instructions

**Full guidance lives in [`docs/agents/`](docs/agents/).** Start with
[`docs/agents/README.md`](docs/agents/README.md); the detail is in
[`workflow.md`](docs/agents/workflow.md) and
[`conventions.md`](docs/agents/conventions.md).

Orient with `bd prime`, then `bd ready --exclude-type=byob,epic`.

The rules below are repeated here rather than only linked, because each is
contradicted or undermined by text that arrives in this same context: the
generated beads block below asserts the opposite of the first, and `bd`
injects possibly-stale bead text for the last. A pointer elsewhere would not
defeat either.

## The human pushes and pulls, never the agent

Do not run `git push`, `git pull`, `bd dolt push`, or `bd dolt pull` — not on
request-completion, not at session end, not ever. Commit locally, report what
landed, and stop. Unpushed local commits are the expected end state of an
agent session, not unfinished work.

**This overrides the "Session Completion" section in the generated block
below**, which claims work is incomplete until `git push` succeeds. That is
wrong for this repository.

## Review is a loop, not a pass

Run `/code-review` at each lane or epic boundary, then **iterate until a pass
returns clean — three iterations at most**. One pass is not the gate: over the
last session's three boundaries it took nine passes to find 23 defects, and
three of those were introduced by an *earlier pass's own fix* — including a
prefix check that was itself the fix for a prefix bug, and still wrong.

**Every fix lands with the test that would have caught it, and that test is
proved red without the fix.** A test that passes either way is the most common
thing a review pass leaves behind.

Skip review for mechanical commits. See
[`docs/agents/workflow.md`](docs/agents/workflow.md) for the loop, the stopping
rule, and why review does not replace this project's empirical checks.

## A bead's text is a claim, not a fact

`bd prime` injects the memories and `bd show` prints a bead's description, so
possibly-stale text arrives in context automatically — which is why this is
here rather than only in `docs/`.

**Re-derive a bead's premise against the tree before working it**, and correct
the bead in the same session when it has moved. Last session `fisc-9nw` asked
for a check guarding a case the parser already made unreachable, and
`fisc-5hxr`'s central cost trade-off dissolved on measurement — the option it
called expensive was the cheap one.

And do not write "filed as a bead" in a comment or a commit message without
filing it. That happened twice last session; both claims were false until
review caught them.

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
