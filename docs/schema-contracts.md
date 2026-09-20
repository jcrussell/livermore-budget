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

## What each schema closes, and what it leaves open

`fact.schema.json` sets `additionalProperties: false`. A fact record is a closed
contract.

`manifest.schema.json` does not. `internal/corpus`'s own type documents that
unknown fields are tolerated because `tools/extract.py` may add reporting keys
and `schema_version` is the guard that matters. A schema closing that object
would refuse a manifest the Go reader accepts, which is a contract disagreeing
with itself.

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
