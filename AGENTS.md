# Agent Instructions

`fisc` turns the City of Livermore's published budget PDFs into a verified
fact store, and that fact store into a static site. Every figure it publishes
carries a provenance pointer back to a page and cell of a source PDF, and
`fisc verify` fails if any link in that chain breaks.

Start with `bd prime`, then `bd ready --exclude-type=byob` for available work.

## Read these before changing anything non-trivial

- [`docs/m0-spike.md`](docs/m0-spike.md) — what the documents actually contain,
  which substrate is usable, and how to re-download the PDFs past the city
  site's bot protection.
- [`testdata/README.md`](testdata/README.md) — each fixture exists for a named
  failure mode. Read it before writing a parser.
- `bd list --type=byob --no-parent` — the architectural decisions this project
  follows. **Never claim or close a `byob-*` bead**; they are reference
  material, not work.

## Remote sync — agents do NOT push

**The repository owner controls when anything leaves this machine.** Do not run
`git push`, `git pull`, or `bd dolt push/pull` unless explicitly asked. Finish
the work, commit it locally, and stop.

This overrides the "Session Completion" section in the generated beads block
below, which asserts that pushing is mandatory. It is not, here.

## Review cadence

Commit after review at logical points, not continuously and not never:

- **Run `/code-review` before any commit that lands a new package**, and at
  each epic boundary (E1, E2, …).
- **Skip it for mechanical commits** — a pinned dependency, a `.gitignore`
  fix, a docs typo. Review has a cost and those have no design surface.
- Fix what the review finds *before* committing, so the commit and its review
  land together.

Review complements empirical verification; it does not replace it. The
highest-risk claims here are not structural — "the parser rejects the right
tokens", "extraction is byte-deterministic", "the mapped rows sum to the
published total". Reading a diff cannot confirm any of those. Guard them with
corpus scans, arithmetic tests against figures the documents themselves print,
and re-run-and-compare checks, and keep doing that regardless of review.

## Working rules

- Tests ship in the same commit as the code they cover (`byob-testing.4`), and
  use `google/go-cmp`, not testify (`byob-testing.2`).
- Amounts are integer cents. Never float, anywhere on the path from cell to
  published total.
- Extraction is not a Go responsibility. `make extract` runs `tools/extract.py`;
  `fisc` reads only the committed artifacts and needs neither Python nor the
  PDFs. Keep it that way — see bead `fisc-j8f` for why the Python side must not
  learn to read `sources.yaml`.
- Reference bead IDs in commit messages as `Refs <id>`.

## Non-Interactive Shell Commands

**ALWAYS use non-interactive flags** with file operations to avoid hanging on confirmation prompts.

Shell commands like `cp`, `mv`, and `rm` may be aliased to include `-i` (interactive) mode on some systems, causing the agent to hang indefinitely waiting for y/n input.

**Use these forms instead:**
```bash
# Force overwrite without prompting
cp -f source dest           # NOT: cp source dest
mv -f source dest           # NOT: mv source dest
rm -f file                  # NOT: rm file

# For recursive operations
rm -rf directory            # NOT: rm -r directory
cp -rf source dest          # NOT: cp -r source dest
```

**Other commands that may prompt:**
- `scp` - use `-o BatchMode=yes` for non-interactive
- `ssh` - use `-o BatchMode=yes` to fail instead of prompting
- `apt-get` - use `-y` flag
- `brew` - use `HOMEBREW_NO_AUTO_UPDATE=1` env var

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
