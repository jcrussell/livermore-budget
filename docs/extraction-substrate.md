# What the extracted substrate does and does not carry

> Evidence for AGENTS.md, "The extraction boundary". This file states no rule.
> The rules are in AGENTS.md; what is here is what was measured against the
> real documents.

## Two substrates, and why both

`tools/extract.py` emits `pages/pNNNN.txt` from poppler's `-layout`, which
reproduces the printed column grid in runs of spaces, and `geometry/pNNNN.json`
from `-bbox`, which carries per-word bounding boxes.

On a sparse grid `-layout` emits only the tokens that were printed and nothing
that says which column each belongs to, so a positional read files them left to
right and can land a figure under the wrong year. Geometry gives the x-position
that settles it.

Page text is `.txt` and not `.md` because GitHub's blob view renders markdown and
collapses the spaces that *are* the grid.

## Geometry gives column identity, not presence

It does not recover a value the PDF never put in its text layer, and it does not
settle "absent is not zero" on its own.

CIP p40 rows PB200654 and PB202617 print `-` in their intervening FY columns and
those dashes appear in **neither** substrate, because they are drawn as non-text.
Row PB200429 on the same page does carry its dashes, so this is per-row and not a
flag chosen wrong. `fisc-8ln` owns the residue.

## A sparse row mostly does not fail closed

`(*Resolver).labelledValues` in `internal/mapping/resolve.go` tokenises
`blk.Text[after:]` — the whole remainder of the *block*, not of the row — so a
row missing a cell borrows the next row's leading token and reaches `ncols`
anyway. The `len(toks) < ncols` guard fires only when the shortfall runs off the
end of the block.

**Do not trust this paragraph over the code.** Three successive attempts in
AGENTS.md to summarise that function were wrong in three different ways, which is
why AGENTS.md now points at the function instead of describing it. If you need
the behaviour, read it and write a probe.

What catches the borrowing is mostly downstream of the read: `cursor` advances to
the end of the last token consumed, so a row that borrows its neighbour's figures
also steps the cursor past that neighbour's *label*, and the next iteration
refuses with `row %q does not occur after %s`. `checkGap` refuses text between
rows unless it is a declared `wrapped_labels` entry — a real exception, not a
formality: CIP p40 needs three before a read gets through at all.

`toks = toks[:ncols]` discards the overflow, and discarding is *not* a way to
absorb a trailing footnote marker: the truncated marker is left sitting in the
gap where `checkGap` refuses it as unexplained text. That is why p76's headerless
marker column has to be *declared* rather than ignored (`fisc-wfi`, and
`internal/mapping/rule.go`'s `ColumnHeader` comment).

## The geometry column guard

The designed answer for what those miss, and the reason `-bbox` is extracted at
all. With `column_headers` declared — which every part of
`mappings/livermore-budget-fy2026-2027.yaml` that reads rows does —
`placementMessage` (`internal/mapping/geometry.go`) reports which column a
token's x actually lands in against the one the rule reads it as, and it fires
before `amount.Parse` ever reaches a neighbour's label word. `geometry_test.go`
pins the message shape.

## CIP p40 is a probe, not a guard over committed facts

`mappings/` holds two files, mapping the Budget Book and one page of the ACFR, so
**no production rule reads the CIP at all**. The refusal is reproducible for a CIP
part you write yourself, and that is the evidence.

`TestCIPp40SparseRowFailsClosedButDoesNotRead` does now reach the row read
(`fisc-i0d9`, closed). It used to anchor its block on the column-header line, so
`checkGap`'s leading-gap arm refused before any row was read and its one
assertion passed on an unrelated message quoting the first row's anchor name. It
now starts past the headers and asserts the value-count refusal by name.

**The guard that fires there is not the geometry one.** The row yields 2 tokens
against 8 columns, so `len(toks) < ncols` refuses first and geometry never places
anything — measured identically with `column_headers` declared and without. That
guard is also load-bearing against more than a wrong read: neutering it panics on
the `toks[:ncols]` two lines below.

Nobody has enumerated what is left once all four defences are in play. A
block-final row backed by an unlabelled total line is the obvious candidate.

## poppler has no structured error channel

It writes free-form English to stderr and exits 0. The manifest records every
stderr line under `warnings`, and reserves `errors` for non-zero exits and
unparseable output. An empty `errors` is not a promise that every page came out
whole.

## Two parties record the hash

`tools/extract.py` deliberately does not read `data/sources.yaml`. It discovers
work from `data/pdf/<doc-id>.pdf` and records the source sha256 it computed;
`fisc verify` cross-checks that against the registry. Two parties recording the
hash independently is a real check — both reading the same file would not be.
