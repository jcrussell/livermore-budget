# What still witnesses the rung walk once the client stops deriving it

> Evidence for AGENTS.md, "Prove it can fail" and for AGENTS.md, "Go vets,
> JavaScript renders". This file states no rule. It is one measurement, taken
> once, of what a battery of mutations against Go's rung walk is caught by after
> `tools/jscheck/rungs.mjs`'s membership arm stops being a second party — with
> four of its rows re-run when the guards three of them asked for landed.

## The question

`fisc-phtp.2`'s remaining work moves the window walk to Go: `site/app.js` reads
`draws[].ids` out of the rung answer instead of deriving membership for itself.
`rungs.mjs` compares the client's derivation against Go's; when there is no
client derivation left, that arm compares the artifact against itself.

So: after the client's derivation is gone, what still witnesses that Go's walk
is RIGHT rather than merely STABLE?

## The method

Taken on `e13-sankey-merge` at `e577338`.

1. The suite was run from an untracked copy of `tools/jscheck/run.mjs` importing
   the other seven modules and not `rungs.mjs`, which is how the suite behaves
   once that arm goes vacuous. `chart.mjs` stayed live. Blinded and unmutated,
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
three membership guards. The matrix carries those runs' answers and the rows
say which.

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
| 9b | one id dropped from a column's PUBLISHED `ids` only, the chart left whole (fund-departments tier 4) | **GREEN** | — |
| 9c | the same at fund-group tier 3, a column whose members further rungs open | CAUGHT | `rungs_test.go` — *the rung at "fund-group/special-revenue" neither counts nor carries it in any column*; and `chart.mjs` arm (f), on which see below |
| 10 | the kept flank read off the document instead of the chart on screen | CAUGHT-CLOSED | *the chart on screen sends nothing between tiers [2 3] and it, so there is no flank to keep* |
| 11a | a mark moved to a tier the step does not declare | CAUGHT | `rungs_test.go` — *stands at tier 99, which the step does not declare* |
| 11b | a mark moved to a DIFFERENT tier the step does declare | **GREEN** | — |
| 12 | every derived mark's `in_cents` and `out_cents` perturbed by one cent | **GREEN** | — |
| 13 | one endpoint dropped from every residual mark's `ends` | **GREEN** | — |
| 14 | every rung under one step left out of the artifact entirely | CAUGHT | `rungs_test.go` — *draws "fund/510" at tier 3, which step "fund-departments" opens, and this column answers no rung of that step below it* |

Twenty mutations: **fourteen caught, two inert, four green**. Eleven of the
catches are the measurement's own; the other three are the guards `fisc-u8di`
and this file's last section asked for, and the rows name them.

## The seven green ones, and what they have in common

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
ones the step declares. Nothing holds **which** declared tier, **how many** of
the declared endpoints, or **what figure**. A one-cent perturbation of a
residual's amount is the mutation that matters, because a residual's cents is
the one figure on this artifact that a reader is shown and that `fisc verify`
does not reach — which is `fisc-4lsx`'s subject, now with a number against it.

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
was re-run with all three in place and is **still green**: the artifact
regenerates with seven perturbed cents per column and `go test -race ./...`
passes. **9b, 11b, 12 and 13 remain**, and `fisc-r0t6` is where the choice about
them lives.

## Two results that were not what the question assumed

**`chart.mjs` is a second party about drawing, not about membership.** The brief
for this measurement assumed `chart.mjs` carries the weight once `rungs.mjs`'s
arm goes. It does, for everything it was built for: that a node laid out reaches
a mark, that a mark carries the attributes and the words its rules say, that a
gesture does what the page says it does, that a state drew a chart rather than a
refusal. But its arms (a) and (f) compare the DOM against `answeredIDs(answer)`,
and `answer` is the artifact. Today that is a genuine comparison because the
client's own filter produced the DOM — which is exactly why (f) went red on
mutation 9c, saying the client *drew* an id Go accounts for nowhere. Once the
client draws the column Go hands it, both sides of that comparison come from one
file. **The membership arms of `chart.mjs` are equivalence arms too, and they
retire with `rungs.mjs`'s.** What survives in `chart.mjs` is every arm about the
drawing.

So after the move, the artifact's membership has no second party at all: what is
left is `TestTheRungArtifactIsWhatGoComputes`'s structural guards, which are
invariants over the artifact against the step declarations, and
`internal/export`'s unit tests over hand-written graphs.

**The ancestry bound is nine and the corpus needs three.** Cutting `maxHops`
from 9 to 3 leaves `testdata/rungs.json` byte-identical. At 2 the corpus
artifact is *still* byte-identical and only `internal/export`'s hand-written
fixtures go red. Nothing wrong ships at any of those values, so this is not a
defect; it is a bound no committed document exercises, recorded so that a future
change to it is known not to be covered by the corpus.

## What the eleven catches are, and are not

Six of the eleven are CAUGHT-CLOSED: the walk refused to answer rather than
answering wrongly. That is the good half of this measurement, and it is worth
naming what those refusals have in common. Every one of them is a place where
two readings of the same documents have to agree —
`export.Openable` against `ReachOf`, the fresh half against the kept half, a
ribbon's ends against the spliced window, a fold's ancestors against its drawn
tiers. The walk is dense with internal cross-checks, and a mutation that
perturbs *how the walk reads* trips one of them almost every time.

The seven green ones are all perturbations of **what the walk writes down**,
applied after every one of those cross-checks has run, and nothing downstream of
the write disagreed with them. Three now do, and what they disagree with is the
documents and the declarations rather than the walk; the four that are left are
the ones a figure would have to be read to see.

## What would witness the rest, costed

The witness that covers 9b, 12 and 13 is arithmetic, not a re-derivation:
what arrives at the opened node against the sum of what its column's ids carry,
with the derived marks' cents as the declared remainder. Every summand is a
published figure `link-values-tie-to-facts` already witnesses, so the sum is a
claim about the documents that the walk did not compute — which is what makes it
independent under the argument in `internal/check/check.go`'s *"What these
checks can witness, and what they cannot"*. Two functions over identical input
in one process cannot witness a wrong answer; a reconciliation against printed
totals can.

Its cost is that its subject is a file that exists only after export, which no
check in `internal/check` has today. That is the decision `fisc-4lsx` carries,
and this measurement is what it was waiting for.

Mutation 14 wanted something cheaper and different, and has it: a
parent-to-child completeness invariant on the artifact, the mirror of the
parentage guard beside it. It witnesses nothing about any figure, which is why
it closes 14 and reaches none of the rest.

Mutation 11b is covered by neither, and is the smallest of the four that are
left.
