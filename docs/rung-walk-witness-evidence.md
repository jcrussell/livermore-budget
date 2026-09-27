# What still witnesses the rung walk once the client stops deriving it

> Evidence for AGENTS.md, "Prove it can fail" and for AGENTS.md, "Go vets,
> JavaScript renders". This file states no rule. It is one measurement of what a
> battery of mutations against Go's rung walk is caught by once the client's
> membership check is not a second party, with seven rows re-run in two later
> rounds against the Go-side guards they asked for.

## The question

`site/app.js` reads `draws[].ids` out of the rung answer rather than deriving
membership for itself (`fisc-phtp.2`). At the measured commit the client's
check module `rungs.mjs` compared the client's derivation against Go's; with no
client derivation, that arm compares the artifact against itself.

So: with no client derivation, what witnesses that Go's walk is RIGHT rather
than merely STABLE?

## The method

Taken on `e13-sankey-merge` at `e577338`, whose client checks were the
`tools/jscheck` modules named below.

1. The client's suite was run from an untracked copy of its runner importing
   the other seven modules and not `rungs.mjs`, which is how the suite behaves
   with that arm vacuous. `chart.mjs` stayed live. Blinded and unmutated,
   that suite is green.
2. Each mutation was applied to `pkg/cmd/export/rungs.go` or
   `internal/export/reach.go`, and **`testdata/rungs.json` was regenerated from
   the mutated walk before anything was run**. That is the whole point: a
   mutation that only reddens the byte pin is one a real defect would ship past,
   because a real defect regenerates its own golden.
3. Recorded per mutation: whether the artifact could be regenerated at all,
   `go test ./pkg/cmd/export ./internal/export`, and the blinded node suite.

Every mutated file was restored from a copy taken before the run and its sha256
compared; `testdata/rungs.json` and `site/app.js` end byte-identical to their
committed bytes.

Mutations 4, 5a, 14 and 12 were re-run by this same method at the commit that
closed `fisc-u8di`, once `TestTheRungArtifactIsWhatGoComputes` had grown the
three membership guards. Mutations 9b, 11b, 12 and 13 — the four still green
after that round — were re-run by it again at the commit that closed
`fisc-r0t6`, against `TestTheRungArtifactIsWhatTheReachPrimitivesAnswer`. The
matrix carries the latest run's answer for each row and the rows say which
round it came from.

In the `fisc-r0t6` round the whole Go suite was run rather than two packages
(`go test ./...`), so each row's catcher is the only test in the tree that goes
red and not merely the first one looked at.

## The matrix

CAUGHT-CLOSED means the mutated walk could not produce an artifact at all: a
refusal inside `rungsOf` or `Fold` fired and the build stopped. CAUGHT means the
artifact regenerated and something still went red. GREEN means the artifact
regenerated, carried the defect, and every gate passed.

| # | mutation | result | caught by |
|---|---|---|---|
| 1 | `nearIsSource` flipped on the transfers step, which keeps no flank | CAUGHT-CLOSED | `rungsOf`: *opens "transfers/in" into tiers [2 3] and the document draws nothing there, which export.Openable said it would* |
| 2 | `nearIsSource` flipped on the fund-departments step, which keeps a flank | CAUGHT-CLOSED | the same refusal, at `fund/510` |
| 3 | one tier dropped from a rung's `draws` (fund-departments tier 4) | CAUGHT | `rungs_test.go` — *draws tiers [2 3] and the step declares [2 3 4]* |
| 4 | an id listed at a tier the document does not draw it at (the opened fund added to its own tier-4 column) | CAUGHT | `rungs_test.go` — *tier 4 draws "fund/510", and "department-funding" holds no node of that id at tier 4* |
| 5a | a printed row moved from `ids` into `carried` (every `department/*`) | CAUGHT | `rungs_test.go` — *tier 4 carries "department/community-development", which no document this column reads marks derived and the step declares no endpoint for* |
| 5b | a declared residual endpoint counted as one of the opened node's parts | CAUGHT | `rungs_test.go` — *counts "transfers/in" as a part of the opened node, and the step declares it a residual endpoint* |
| 6a | `maxHops` 9 → 8 | INERT | artifact byte-identical |
| 6b | `maxHops` 9 → 3 | INERT | artifact byte-identical |
| 6c | `maxHops` 9 → 2 | CAUGHT | artifact still byte-identical; `internal/export`'s hand-written fixtures red — `TestReachOfCarriesTheFoldedChart`, `TestFoldIsTheClientsFoldDocument`, `TestChartSplicesAsWindowForDoes` |
| 7 | `ReachOf` admits a ribbon whose end has no ancestor at a drawn tier | CAUGHT-CLOSED | `Fold`: *"transfer-to/210" has no ancestor at a drawn tier [2 3]* |
| 8 | `ReachOf` keeps a ribbon whose near end is a SIBLING of the opened node | CAUGHT-CLOSED | `answer`'s centre-alone refusal: *the chart on screen draws [fund/511 …] beside it at tier 3* |
| 9 | one id dropped from an outward column and from the chart with it | CAUGHT-CLOSED | the ribbon splice: *names "department/library-department", which the chart does not hold* |
| 9b | one id dropped from a column's PUBLISHED `ids` only, the chart left whole (fund-departments tier 4) | CAUGHT | `rungs_test.go` — *the columns export.ReachOf answers over the documents are not the ones the artifact writes down*, at `fund-group/special-revenue > fund/240` tier 4 |
| 9c | the same at fund-group tier 3, a column whose members further rungs open | CAUGHT | `rungs_test.go` — *the rung at "fund-group/special-revenue" neither counts nor carries it in any column*; and `chart.mjs` arm (f), on which see below |
| 10 | the kept flank read off the document instead of the chart on screen | CAUGHT-CLOSED | *the chart on screen sends nothing between tiers [2 3] and it, so there is no flank to keep* |
| 11a | a mark moved to a tier the step does not declare | CAUGHT | `rungs_test.go` — *stands at tier 99, which the step does not declare* |
| 11b | a mark moved to a DIFFERENT tier the step does declare | CAUGHT | `rungs_test.go` — *the marks export.ResidualOf and export.GapOf answer are not the ones the artifact writes down*: `residual/fund-group/capital` at tier 2 against the recomputed 3 |
| 12 | every derived mark's `in_cents` and `out_cents` perturbed by one cent | CAUGHT | the same arm — `residual/fund-group/capital` at 250021301 against the recomputed 250021300 |
| 13 | one endpoint dropped from every residual mark's `ends` | CAUGHT | the same arm — `residual/fund-group/general` naming `transfers/in` alone against the recomputed pair |
| 14 | every rung under one step left out of the artifact entirely | CAUGHT | `rungs_test.go` — *draws "fund/510" at tier 3, which step "fund-departments" opens, and this column answers no rung of that step below it* |

Twenty mutations: **eighteen caught, two inert, none green**. Eleven of the
catches are the measurement's own; three are the guards `fisc-u8di` and this
file's last section asked for, and four are the replay `fisc-r0t6` asked for.
The rows name them.

## The seven green ones, and what they had in common

Mutation 14 is the one to read first. Leaving out every rung the
fund-departments step opens takes the two columns from 99 and 97 rungs to 45 and
45 — **more than half the artifact gone** — and at the measurement every gate
was green. The parentage guard runs child-to-parent (a rung's opened node must
appear in the rung above it) and nothing ran the other way, so a column could
list ids that no rung ever answered for.

Mutations 4, 5a and 9b are the same shape one column down: **a column's
membership was guarded only where its members are themselves opened.** 9c is 9b
moved one tier up onto a column the walk descends, and there the parentage guard
fires. The last column of every walk was unguarded, and on the committed spine
that is where most of the ids are.

Mutations 11b, 12 and 13 are the derived marks. The structural guards hold a
mark's id to its role's prefix, its role to one of two names, its tier to the
step's declared set, a gap to exactly one side, and a residual's endpoints to
ones the step declares. None of them holds **which** declared tier, **how many**
of the declared endpoints, or **what figure**. A one-cent perturbation of a
residual's amount is the mutation that matters, because a residual's cents is
the one figure on this artifact that a reader is shown and that `fisc verify`
does not reach — which is `fisc-4lsx`'s subject.

Three were closed by reading the documents, four by recomputing the answer; the
two sections below are those two rounds.

## Three of the seven, closed

`fisc-u8di`'s guard and two beside it are in
`TestTheRungArtifactIsWhatGoComputes`, each re-run by the method above —
mutation applied, `testdata/rungs.json` regenerated from the mutated walk,
`rungs.mjs`'s membership arm blinded.

- **14**, the mirror of the parentage guard: every id a rung draws at a tier a
  later step opens, with the role that step declares and where that step's
  document decomposes it, is answered by a rung one path longer. 106 findings.
  It asks `export.Openable`, which the walk asks too, so it is completeness read
  from the parent and not a second reading of the documents — it cannot witness
  the openable set itself being wrong, only the walk answering fewer rungs than
  that set offers.
- **4**, the documents read directly: every id an outward column draws is a node
  that step's own document holds at that tier, and every id the centre or a kept
  flank draws is one some document the column reads holds there. 106 findings.
  The kept half is the weaker of the two claims because its ids came off the
  chart on screen and not off this step's document, and that is measured rather
  than feared: of the artifact's 499 centre and flank ids — 252 in one column
  and 247 in the other — 39 are at no tier of their own step's document at all,
  34 of them fund groups on an object-category rung, which
  `department-spending` does not draw, and 5 on a fund-group rung. Holding the
  kept half to the step's own document would fail on the committed corpus.
- **5a**, what makes a node carried: a carried id is one a document marks
  derived or the step declares a residual endpoint, and a counted id is neither.
  110 findings.

None of the three reads a figure, and that is the limit of what they close.
Mutation 12 — every derived mark's `in_cents` and `out_cents` perturbed by one —
was re-run with all three in place and was **still green**: the artifact
regenerates with every mark's cents off by one and `go test -race ./...` passes.
That left **9b, 11b, 12 and 13**, which are the next section's.

## The last four, closed

`TestTheRungArtifactIsWhatTheReachPrimitivesAnswer`, in
`pkg/cmd/export/rungs_test.go`, takes each rung's path and step as the artifact
states them and recomputes what that step's columns hold and what marks stand
beside them from `export.ReachOf`, `export.ResidualOf` and `export.GapOf`
**called by the test**. The window a mark is arithmetic over is rebuilt there
too, each rung's from its parent's, because a replay that asked the artifact for
that chart would be asking the mutated side for the answer.

This is not the re-derivation `internal/check/check.go` refuses, and the
distinction is which layer is spelled twice. The primitives are held by their
own fixture tests over hand-written graphs in `internal/export`, the layer
below; what this test spells a second time is the **assembly** — which column is
asked of which document on which side, which record answers for each id, which
figures are written down. So a perturbation of what `rungsOf` wrote down moves
one side and not the other, which is the whole of what these four mutations are.

All four go red under it, each with the artifact regenerated from the mutated
walk, and in this round `go test ./...` was run: the replay is the **only** test
in the tree that reddens for any of them.

Two findings of that round:

- **The first spelling of 11b was inert.** Moving a mark one column *outward* —
  to the next entry of `DrillStep.Tiers` — left `testdata/rungs.json`
  byte-identical, because every mark in this artifact already stands at the last
  declared tier or the first. The live mutation moves it *inward*, and
  `residual/fund-group/capital` then reads tier 2 against the recomputed 3. A
  mutation that regenerates an identical artifact witnesses nothing about the
  guard it was aimed at, which is the shape AGENTS.md, "Prove it can fail" says
  to hunt for in a fixture and is just as available in a mutation.
- **Exactly one residual in the corpus names more than one endpoint.** Dropping
  an end from every residual's `ends` moves one line of the artifact and no
  more, so mutation 13's whole subject is one mark on one column. The test
  counts that shape and fails when it reaches zero, rather than leaving the
  arm's only subject to a corpus that could stop supplying it.

What it does not witness, since the replay takes them as given: which rungs
exist at all — the paths and the steps are the artifact's, and their
completeness is the parentage guards above — and the primitives themselves being
wrong, which is `internal/export`'s.

## Two results that were not what the question assumed

**`chart.mjs` is a second party about drawing, not about membership.** It
carries the weight for everything it was built for: that a node laid out reaches
a mark, that a mark carries the attributes and the words its rules say, that a
gesture does what the page says it does, that a state drew a chart rather than a
refusal. But its arms (a) and (f) compare the DOM against `answeredIDs(answer)`,
and `answer` is the artifact. At the measured commit that was a genuine
comparison because the client's own filter produced the DOM — which is why (f)
went red on mutation 9c. When the client draws the column Go hands it, both
sides of that comparison come from one file: **the membership arms of
`chart.mjs` are equivalence arms too.** Its arms about the drawing are not.

So the artifact's membership has no second party across the language boundary,
and by the owner's ruling of 2026-09-20 it is not owed one: Go owns the
correctness of what it emits and JavaScript is tested to render it without
errors. What holds it is all Go — `TestTheRungArtifactIsWhatGoComputes`'s
structural and membership guards, the replay in
`TestTheRungArtifactIsWhatTheReachPrimitivesAnswer`, and `internal/export`'s
unit tests over hand-written graphs.

**The ancestry bound is nine and the corpus needs three.** Cutting `maxHops`
from 9 to 3 leaves `testdata/rungs.json` byte-identical. At 2 the corpus
artifact is *still* byte-identical and only `internal/export`'s hand-written
fixtures go red. Nothing wrong ships at any of those values, so this is not a
defect; it is a bound no committed document exercises, recorded so that a future
change to it is known not to be covered by the corpus.

## What the catches are, and are not

Six are CAUGHT-CLOSED: the walk refused to answer rather than answering
wrongly. Every one of them is a place where
two readings of the same documents have to agree —
`export.Openable` against `ReachOf`, the fresh half against the kept half, a
ribbon's ends against the spliced window, a fold's ancestors against its drawn
tiers. The walk is dense with internal cross-checks, and a mutation that
perturbs *how the walk reads* trips one of them almost every time.

The seven green ones were all perturbations of **what the walk writes down**,
applied after every one of those cross-checks had run, and nothing downstream of
the write disagreed with them. Three are caught by the documents and the
declarations, and four by the answer recomputed beside them.

## What is still not witnessed, costed

The replay closes these mutations as **emitter fidelity**: the artifact says what
the primitives answer. It does not make the primitives' answer right. A figure
the walk computes wrongly and writes down faithfully passes it, and so does one
whose document was mapped wrongly upstream.

The witness for that is arithmetic and not a recomputation: what arrives at the
opened node against the sum of what its column's ids carry, with the derived
marks' cents as the declared remainder. Every summand is a published figure
`link-values-tie-to-facts` already witnesses, so the sum is a claim about the
documents that the walk did not compute — which is what makes it independent
under the argument in `internal/check/check.go`'s *"What these checks can
witness, and what they cannot"*. Two functions over identical input in one
process cannot witness a wrong answer; a reconciliation against printed totals
can.

Its cost is that its subject is a file that exists only after export, which no
check in `internal/check` has. That is the decision `fisc-4lsx` carries.

Mutation 14 wanted something cheaper and different, and has it: a
parent-to-child completeness invariant on the artifact, the mirror of the
parentage guard beside it. It witnesses nothing about any figure, which is why
it closes 14 and reaches none of the rest.
