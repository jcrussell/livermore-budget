# Project Instructions for AI Agents

@AGENTS.md

<!--
This file is an import and nothing else. All guidance lives in AGENTS.md, which
is the file every other agent tool reads verbatim; `@AGENTS.md` above is Claude
Code's import syntax, so its text arrives in context here rather than as a link
a reader might not follow.

The direction is deliberate and cannot be reversed. Codex, Cursor and Copilot
read AGENTS.md as-is and do not process `@` imports, so AGENTS.md must be the
full file and this one the pointer. Before this, the two carried 110 lines of
byte-identical hand-synced text and AGENTS.md was missing the build commands and
the four invariants entirely.

`bd` regenerates a "Beads Issue Tracker" block in BOTH root files, and the block
asserts that work is incomplete until `git push` succeeds, which is wrong here.
Expect it to reappear below this comment on any `bd` upgrade. That is harmless
as long as it lands BELOW the import: AGENTS.md's override arrives first and
`bd`'s block is read against it. If a `bd` upgrade ever inserts above the
import, move it back.
-->
