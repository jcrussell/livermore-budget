# Why AGENTS.md is short, and what may go in it

> Evidence for AGENTS.md, "Where writing goes". This file states no rule. The
> rules are in AGENTS.md; what is here is the argument for how it is organised.

## Three homes, three kinds of writing

- A **rule** — an imperative an agent must follow — is stated in AGENTS.md and
  nowhere else.
- **Evidence** for a rule lives in `docs/`, in a file like this one, and never
  says what to do.
- A **contract** — a specification of an artifact — lives in `docs/` until the
  package that produces it exists, then moves to that package's doc comment.
  `docs/sankey-contract.md` says so of itself.

Two files that both state a rule can disagree about the rule. A file that states
no rule can only go out of date about the world, which is ordinary staleness and
is what `docs/` already carries.

## Why the rules cannot move to docs/

`CLAUDE.md` is a one-line `@AGENTS.md` import, so AGENTS.md arrives in every
session whether or not anyone opens it. A `docs/` file does not. That is the
whole reason the split is by *kind* rather than by topic: a rule an agent must
follow without being told to look it up cannot live in `docs/`, and evidence a
reader goes looking for should not sit in the file everyone pays for.

The import direction cannot be reversed. Codex, Cursor and Copilot read
AGENTS.md as-is and do not process `@` imports, so AGENTS.md must be the full
file and `CLAUDE.md` the pointer. Before that, the two carried 110 lines of
byte-identical hand-synced text and AGENTS.md was missing the build commands and
the four invariants entirely.

## Why this is not a second `docs/agents/`

`5794a60` (2026-08-29) deleted `docs/agents/` and merged it into AGENTS.md,
having measured three kinds of duplication: 110 byte-identical hand-synced lines
between the root files; 59 lines `CLAUDE.md` had that AGENTS.md lacked; and
three sections of `workflow.md`, then under `docs/agents/`, restated in the
root files, of which the review-loop justification checked out false. Its message accepted the cost:
*"AGENTS.md is 36 KB and is now in context every session, where workflow.md and
conventions.md were read on demand."*

That failure was **three files each stating the same rule**. This split moves
only evidence, and no evidence file states a rule — which is the property that
makes it a different arrangement rather than the same one under new names.

## What it cost to leave it alone

AGENTS.md was **693** lines at `5794a60` — its own commit message says 690, which
is why a count is read off the tree and not off a message. Seven days later it
was 1,126, **+63%**, and the only commit in that window that shrank it was
`d6ff0c7`, by 32 lines. A five-pass review lane over the file itself *added* 22
net lines: the range is written `6761254^..f1d0815` because `a..b` excludes `a`,
and the lane's first commit is one of the five. Nothing stopped the growth, and nothing yet stops it: a line-budget check
is specified in `fisc-ak39` and has not been written, so what holds the size down
today is only the habit this file is arguing for.

## The two sections that must precede the generated block

`bd` regenerates a "Beads Issue Tracker" block in both root files, and that block
asserts work is incomplete until `git push` succeeds, which is wrong here. Two
sections are ordered for **position** rather than for reading, and both must sit
above the `BEGIN BEADS INTEGRATION` marker:

- **The human pushes and pulls, never the agent**, which the generated block
  directly contradicts.
- **A bead's text is a claim, not a fact**, which `bd prime`'s injected bead text
  undermines by arriving in context automatically.

They are budgeted floors rather than targets: the generated block is 47 lines
however short the rest becomes, so its share of the file rises as AGENTS.md
shrinks, and the overrides have to stay legible against it. Re-check after any
`bd` upgrade that the block has not been inserted above them.

## The claim this refactor makes, and how it will be falsified

The benefit is asserted and not measured. Nobody has shown that a 1,126-line
AGENTS.md causes a defect a 300-line one would not, while this repository's own
audit says PROSE-NEW plus PROSE-PRE-EXISTING are 32 of 115 findings — so the
refactor rewrites the most defect-dense artifact class it has, to buy an
unmeasured gain.

The honest objection is that the evidence is not decoration around a rule; it is
what makes an agent comply with a rule that costs it something. A bullet reading
*"never copy a count from this file"* has less purchase than the paragraph
explaining that three consecutive passes corrected that exact count and each
correction was wrong in a new way.

`fisc-6lfh` is the falsification: after two full post-refactor lanes, re-run the
per-finding classification and compare the PROSE-NEW and FALSE-CLAIM shares
against the 17% and 32-of-115 baselines. **If they rose, the cut was wrong and
the amputated arguments go back inline.**
