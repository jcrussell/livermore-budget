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

## One schema, three readers, one dependency

Go validates fully, with `github.com/google/jsonschema-go`. `tools/jscheck`
compares the schema's `required` arrays against `app.js`'s own refusals.
`tools/extract.py` checks what it wrote against the same arrays.

Only Go takes the dependency. The client ships without npm and the extractor
runs where PyPI is unreachable, so a JSON Schema *implementation* on those sides
is out — but reading the committed file and checking required keys and types is
thirty lines of standard library, and it is the schema being read rather than
restated. A required-key list copied into either would be the second spelling the
schema exists to remove.

Hand-rolling the Go validator was the first plan and was rejected: it is the
second implementation of a standard thing, which is the argument `tools/jscheck`
already makes about not writing a selector engine.

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
measured. `role` is a declared vocabulary — `internal/project` composes the
values, `pkg/cmd/export` re-spells five of them in its step declarations, and
`internal/export` selects fund groups by a sixth copy — so an enum in
`column.schema.json` is what makes those copies one claim, held in both
directions by a test in each package.

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
`fisc export` refuses with `unexpected additional properties ["grain"]`,
`tools/jscheck` refuses by name at the parse, and the two struct-to-schema parity
tests go red. Before, all three were green.

`manifest.schema.json` does not. `internal/corpus`'s own type documents that
unknown fields are tolerated because `tools/extract.py` may add reporting keys
and `schema_version` is the guard that matters. A schema closing that object
would refuse a manifest the Go reader accepts, which is a contract disagreeing
with itself.

## What has no schema, and why that is not an oversight yet

The projection documents `internal/project` builds — `sankey.json` and its
siblings — have none. Their shape is stated by the Go structs and by the fenced
block in [`sankey-contract.md`](sankey-contract.md), which is TWO spellings and
not three: there is no schema for that block to duplicate.

That is worth saying because the block looks like the one deleted from
`general-fund-drilldown-contract.md` and is not. The drilldown block described an
artifact a schema had just been written for, so it was a third spelling that
could never be compared against bytes. This one is the only prose statement of a
shape nothing else states, and deleting it would remove information rather than
duplication. `fisc-1wmy` is where the projection schema would go.

`data/revenue-trends.json` and the two fund-balance documents have none either.
They ship as themselves, byte for byte, held by a copy test and goldens rather
than by a shape contract.

## Why a column's schedules are not merged

They share node ids — `fund-group/capital` is in three of them — but their links
differ, because each is a different printed schedule with its own provenance.
Merging them into one graph would silently reconcile the cells `DrillStep.Gaps`
exists to keep visibly unreconciled.

What *is* shared was measured rather than assumed: across every published
column, two schedules naming one mark never disagree about `id`, `label`,
`tier`, `role` or `derived`, and do disagree about `parent` 156 times and
`constraint_tier`, `rationale` and `source_note` 64 each. A node's identity is
shared; where it hangs belongs to the schedule that draws it. The first emitter
put `parent` in the shared table and refused its own output on `fy2024-actual`,
which is how this was found.
