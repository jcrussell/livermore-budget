# Agent documentation

Detailed guidance for AI coding agents working on this project. `AGENTS.md`
and `CLAUDE.md` at the repository root are thin pointers to these files, and
carry inline only the rules that must not be missed.

| Document | Covers |
|---|---|
| [status.md](status.md) | **Start here.** Where the project stands, what is next, known traps |
| [workflow.md](workflow.md) | Beads, remote sync, review cadence, commits |
| [conventions.md](conventions.md) | Code conventions, provenance invariants, shell safety |

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
bd prime                          # workflow context and command reference
bd ready --exclude-type=byob      # available work, byob decisions hidden
```

## Read before changing anything non-trivial

- **[`docs/m0-spike.md`](../m0-spike.md)** — what the three documents actually
  contain, which substrate is usable and which is a trap, and how to
  re-download the PDFs past the city site's bot protection. Written from a
  validation run against the real files; several intuitions it corrects are
  ones a reasonable person would otherwise hold.
- **[`testdata/README.md`](../../testdata/README.md)** — every fixture exists
  for a specific named failure mode. Read it before writing a parser.
- **`bd list --type=byob --no-parent`** — the architectural decisions this
  project follows, imported from byob-go-cli. **Never claim or close a
  `byob-*` bead**; they are reference material, not work, and closing one
  hides it from future sessions.
