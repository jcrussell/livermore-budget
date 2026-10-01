# Why the artifact shapes are schemas

Evidence for the rule in AGENTS.md, "Where writing goes": a frozen contract's
shape lives in `schema/`, and this file holds the argument for it.

## The shape was spelled three times and checked in none of them

A published artifact's shape was stated in `docs/`, again in Go struct tags, and
a third time in `site/app.js`'s hand-written key refusals. None of the three
could be held against the bytes.

Measured when the first schema landed: it refused a float `amount_cents`, an
empty `token` and a string `fund`, and it caught its own author — the first draft
guessed `fund` as string-or-null, and 1,518 of 2,382 records proved it is an
integer.

Writing the enums from the corpus rather than from the declared set would have
omitted `basis: "projected"` and `units: "thousands"`: both declared in
`internal/mapping` and `internal/amount`, neither present in any committed fact.
A schema built from today's data refuses a correct record the first time the
corpus grows one.

## One schema, two readers, one dependency

Go validates fully, with `github.com/google/jsonschema-go`. `tools/extract.py`
checks what it wrote against a subset of the same file: required keys, JSON
types and enums. The client reads no
schema at all: it draws what Go validated before writing, and refuses only a
200 carrying an error page and a copy from another build.

Only Go takes the dependency. The extractor runs where PyPI is unreachable, so
a JSON Schema *implementation* there is out — but reading the committed file and
checking that subset needs only the standard library, and it is the schema being
read rather than restated. A required-key list copied into it
would be the second spelling the schema exists to remove.

## Shape is checked before semantics

A malformed record is not a check failure; it is a store the checks cannot speak
about. Every domain error downstream is an interpretation, and interpreting a
structure that is not the agreed one produces a worse sentence than saying so.

Measured, deleting one record's `token`: before, `fact-token-reparses` reported
one finding over 2,382 facts. After, the load refuses with `line 6 ... missing
properties: ["token"]`.

`internal/check`'s loader validates the raw file rather than the decoded structs,
because a struct has already lost the difference between a key that was absent
and one present and empty — and that difference is what "absent is not zero"
rests on. Validating after decode could not see the defect above at all.

## A shape, yes; an open set, no

A schema here states a record's SHAPE and a DECLARED code set. It never closes
an open data set.

The two look alike and are not, and the fund column is where the difference was
measured. `role` is a declared vocabulary: `project.Roles` is the set, the
steps in `pkg/cmd/export` and `internal/export`'s fund-group selection name its
constants, and `enums.schema.json` states it once for every document that
carries a role, held to `project.Roles` both ways by `schema/enums_test.go`.
The other closed sets (basis, kind, sign, units, link kind) are stated there
too, each held to its Go set the same way.

The fund groups are not. `data/funds.yaml` declares seven fund types and grows
without asking any of these files, so `fund_groups` is an ordered array of
whatever the column holds and the schema enumerates no id. The alternative was
already in the tree and had the defect the rule predicts: `site/app.js` held six
ids as a literal, `fy2024-actual` publishes seven, and the seventh's position
was whatever `indexOf` returned for a miss.

The same distinction is why `docs` in `page.schema.json` is keyed by
`additionalProperties` rather than by doc id. Which documents a page cites is
the corpus's; naming them in the contract would make it a copy of the corpus.

## What each schema closes, and what it leaves open

`fact.schema.json` sets `additionalProperties: false`. A fact record is a closed
contract.

`page.schema.json` closes every object in it, and that is the property with the
most behind it. `window.FISC_CONFIG` is the one artifact that crosses this
boundary without being FETCHED — it is rendered into the page's own `<script>` —
so there is no cached-copy question to ask on arrival and nothing on the client
side would ever have caught a key added or dropped. `additionalProperties: false`
is what makes a field added to one of those structs refused at the export,
naming it, rather than shipped to a client that ignores it.

Measured by adding `Grain []int` to `export.DrillStep` and to one step literal:
`fisc export` refuses with `unexpected additional properties ["grain"]` and
the two struct-to-schema parity tests go red. Before, both were green.

`manifest.schema.json` does not. `internal/corpus`'s own type documents that
unknown fields are tolerated because `tools/extract.py` may add reporting keys
and `schema_version` is the guard that matters. A schema closing that object
would refuse a manifest the Go reader accepts, which is a contract disagreeing
with itself.

## Every artifact has one, and they are not one schema

`internal/project` builds documents of two shapes, and one schema for both would
state the union of two things and refuse neither.

`projection.schema.json` holds the graph documents: one `metadata` block with
`scopes` a list on every document and one `counts` block with one identity,
`facts = facts_cited + facts_uncited`, on every document. They differ in one
way, which it states and a fenced example could not: only `sankey` carries a
`headline`, because a headline is a total over a single-grain view and a
document holding the same money at two grains has none to name.

`series.schema.json` holds the documents built as a series per printed row. Those
are the only projections a reader fetches as themselves, so it is holding served
bytes rather than bytes passed between packages.

The seam they were written for has no compiler behind it: `internal/export`
decodes these documents without importing `internal/project`, so `decoded` and
the decoders in `page.go` are joined to them by json tags alone. A tag renamed
on one side reads as a zero value with no error anywhere -- the page renders,
the figure is absent, nothing is red. `internal/project` is held to the schemas
by equality and `internal/export` by containment, because a decoder may read a
subset and may not read a name no document carries.

Two things the schemas state that were true of the tree and stated nowhere: a
series' `fund` is null rather than zero
where the row sits under no numbered fund, and no fund is numbered 0; and a
projection with nodes must cite a source, while an empty one may cite none.

## Why a column's node is one table entry and its parent is the schedule's

A column's schedules share node ids — `fund-group/capital` is in three of them —
but their links differ, because each is a different printed schedule with its
own provenance. Merging the links into one graph would silently reconcile the
cells `DrillStep.Gaps` exists to keep visibly unreconciled, so a schedule keeps
its own link set.

What a node's table entry carries was measured rather than assumed, over
the `fy2026-adopted.json` and `fy2027-adopted.json` that `fisc export` writes at 1529ae5. Of the
123 and 121 nodes two or more schedules draw, none disagree about `id`,
`label`, `tier`, `role` or `derived`; none disagree about `constraint_tier`,
`rationale` or `source_note`, because every builder annotates a fund node
through one constructor from `data/funds.yaml` — before that constructor, 30
and 29 fund nodes carried a tier in two schedules and none in the transfer
networks, an absence and not a second reading. So the table carries the
identity and the annotations, and `ColumnsOf` refuses a schedule that
disagrees about any table field rather than merging.

`parent` differs on 70 and 69 of those shared nodes, and stays the schedule's:
23 `dept/` nodes hang under `fund/100` in fund-flows (pp.167-170 are the
General Fund's) and under nothing in department-spending (pp.85-125 carry no
fund); 30 and 29 `fund/` nodes hang under their group where a schedule draws
the groups and under nothing in the transfer networks, which draw none; 17
transfer ends hang under `transfers/in` or `transfers/out` in the network that
folds them there and under nothing in the other, since transfers-out's p222
legs are outside `transfers/in`. A parent is a claim about one schedule's
hierarchy, and two schedules can be right about one node.

## A shape Go never emits still has a schema

The client synthesises three nodes no document carries: the aggregate a cap
folds a column's tail into, the residual that carries flow a drawn document
does not decompose, and the gap that holds a licensed difference. Their roles
and id prefixes are a declared set in `enums.schema.json`, held to the Go set
`internal/export` refuses on a producer node, and their shape is
`schema/mark.schema.json`. Go validates nothing against it, because Go never writes
one; `site/marks.test.mjs` reads the schema and holds each mark the client
builds to its required keys, its properties, the enum and the pattern, which
is the subset `tools/extract.py` checks and the same precedent. The client
itself reads no schema.
