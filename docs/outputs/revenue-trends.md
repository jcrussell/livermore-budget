# The revenue-trends.json contract

Four printed columns of one detail schedule: Budget Book pp.127-140, *Revenue
Sources by Fund*. Every figure is already a fact in `facts/facts.jsonl`; the
document is series, not a graph.

## Where it goes

`<output>/data/revenue-trends.json`, with the same
`extracted/<doc-id>/pages/pNNNN.txt` provenance tree as the spine — fourteen
pages here. It states no fiscal year and no basis, so it is a schedule of no
column and ships under its own name. Its stem is its `Name()` verbatim: it
publishes one document spanning four columns, so a year suffix would write two
byte-identical files, one lying about the year it covers.

## Shape

`schema/series.schema.json`, validated on every encode. Each point is one
printed figure and cites its fact by id, page, offset and token. Every key is
present in declaration order, and no key is `null`: every series here sits under
a numbered fund (the ACFR fund-balance documents publish `null` `fund`; see
[`acfr-history.md`](acfr-history.md)).

Money is a **signed** integer `amount_cents`. Nine points are negative: General
Fund ERAF and RPTTF Reduction, four columns each, plus Prior Year - Unsecured in
the FY2023-24 actual column alone, all printed in parentheses. There is no `sign`
key; filter on `amount_cents < 0`. A series is a printed row, so contra rows are
not netted and summing a fund's series needs no special case.

## The envelope

`schema_version`, `projection`, and within `metadata` — `generated_by`, `scope`,
`currency`, `units`, `sources`, `counts`, `caveats` — are the envelope, held as
`Envelope` in `internal/project/document.go`. A graph's `Metadata` spells the same
keys with `scopes` a list, interleaved with its own (`generated_by, fiscal_year,
fiscal_year_label, basis, scopes, currency, units, sources, headline, counts,
caveats`), so the two are not embedded;
`TestTheEnvelopeAndTheGraphMetadataSpellTheirSharedKeysOnce` holds the shared
tags together.

`fiscal_year`, `fiscal_year_label`, `basis` and `headline` are absent: the
document spans four years and three bases, and the headline keys are the
spine's. It publishes no `"fiscal_year": 0` to keep a shape it is not of.

## series_id

```
sha256(doc_id \x1f row_path \x1f row_label \x1f column_path)[:12], prefixed "fisc-s-"
```

`fact.MakeID`'s tuple minus `rule_id`, `fiscal_year` and `basis`: a schedule
mapped one rule per year (Budget Book pp.186-209) prints a row under a different
rule each year, so the rule is no part of a row's identity. Anyone holding
`facts.jsonl` can derive it, and it is stable when a fifth year is mapped.
`fact`'s `TestASeriesIsOneCellPerYearAndBasis` holds that no two committed facts
share a series, a year and a basis. The arity and prefix differ from a fact id's,
so the two cannot collide. `row_label` is `mapping.Row.PrintedLabel()`, the
string the fact record publishes.

## The four columns are not one measurement

| column | basis | what it is |
|---|---|---|
| FY 2023-24 | `actual` | money that moved |
| FY 2024-25 | `revised` | a mid-year re-forecast |
| FY 2025-26 | `adopted` | an intention |
| FY 2026-27 | `adopted` | an intention |

`comparable_group` ships machine-readable: the two adopted years share a group,
being two years of one two-year budget; the other two stand alone. The document
publishes no growth, CAGR or other inter-column arithmetic. A line connecting the
columns is a mark, not a datum, and a renderer drawing one breaks it at the
actual-to-budget boundary.

**FY2024 `actual` is the Budget Book's own restatement, not the ACFR's audited
figure**, and is never labelled `audited` (`vocab.BasisAudited` is a different
basis).

## What summing the series does and does not reproduce

Sum every series into its fund group and compare with p63 Table 2, *Total Sources
– All Funds* (revenues plus transfers in, the two kinds this scope carries).
Twenty of the twenty-eight cells tie exactly:

| fund group | FY2023-24 | FY2024-25 | FY2025-26 | FY2026-27 |
|---|---:|---:|---:|---:|
| general | −737,455 | −914,206 | −480,400 | −486,735 |
| enterprise | 0 | 0 | 0 | 0 |
| debt-service | 0 | 0 | 0 | 0 |
| permanent | 0 | 0 | 0 | 0 |
| capital | −1 | −4,125,627 | 0 | 0 |
| special-revenue | +1 | +500 | 0 | 0 |
| internal-service | 0 | 0 | 0 | 0 |

- **general, all four columns** — pp.127-130 print no *General Fund Transfers In*
  row. In the adopted years the gap is 480,400 and 486,735 exactly, the spine's
  `TRANSFER IN:` General Fund cell, published under scope `transfers-by-fund` off
  p76 and held apart on the revenue-detail cut by
  `structure.BudgetBookExceptions`. pp.66-67 print no actual or revised column,
  so nothing on the spine reconciles the other two.
- **capital FY2024-25, −4,125,627** — `fisc-zl9`. General Fund CIP Reserves has
  no section on pp.131-140. p63 prints 34,839,275; the schedule's funds sum to
  30,713,648.
- **special-revenue FY2024-25, +500** — `fisc-94b`. p0139.txt:56 prints
  `Oth Financing Source - 500 - -` in the Police Donations fund; p63 and p192,
  two roll-ups produced differently, both print 19,666,119, exactly $500 less than
  the 38 printed `Total <fund>` lines. This document includes the line because
  the city printed it on the page it reads.
- **capital −1 and special-revenue +1, FY2023-24 only** — the document's own
  rounding: eleven blocks are off by ≤ $5, every one in the FY2023-24 Actual
  column, p131 and p135 among them (`fisc-2sd`).

`metadata.caveats` states the capital FY2024-25 hole and the General Fund
Transfers In gap, with figures; `applies_to` is empty on each, there being no
node to name. It also carries `the-revenue-schedule-is-published-twice`, shared
with every `fund-flows` document.

**`permanent` is not on the spine.** Its one series — fund 470, *Transfers In*,
19,533 in FY2023-24 and zero in the other three — ties to p63 exactly; pp.66-67
print no Permanent column (`fisc-u8o`).

## counts

`facts` (every fact matching the scope and one of the four columns), `series`
and `points`. There is no `facts_cited`: every fact becomes a point. `facts` and
`points` are computed independently, so a dropped series shows as `points` below
`facts`; `trend-points-tie-to-facts` names the row.

**No point is dropped for being zero.** A printed `-` is a published zero, and a
gap would be indistinguishable from a row the city stopped printing, which
`trend-series-are-complete` catches.

## derived and labels

`derived: false` on every point; nothing here is inferred. `label` is the row's
printed label. `fund_name` and `category_label` are registry lookups published so
the client needs no `data/funds.yaml` or `data/taxonomy.yaml`: `row_label` is not
unique across funds — "Property Taxes" is printed by four funds and "Use of Money
& Prop" by thirty-eight. `project.Labels` carries `FundName(number int) (string,
bool)` for it; a miss falls back to `""` and the client shows the number.
`fact-funds-resolve` asserts every fund resolves.

## schema_version

One `schema_version` spans every document and versions the envelope, so a change
to either body bumps it for both. `TestClientSchemaVersionIsPinnedToTheProducer`
pins the client's copy; `checkSchemaVersion` names the document it read.

## What checks this

The structural checks in `internal/check/graph.go` read nodes and links, which
this document has none of.

- **`trend-points-tie-to-facts`** — every point equals its fact's `amount_cents`
  and carries its `doc_id`, `page`, `offset` and `token`; a point citing no fact
  in the store, or a fact in the slice no point publishes, is a finding.
- **`trend-series-are-complete`** — every series has a point in every column, or a
  declared exception. It does not catch a whole column vanishing from the corpus
  (`fisc-7dt`).
- **`documents-are-checked`** — `documentShape` names `series` for a
  `TrendsDocument` and cites the two checks above.

`cuts-tie-along-the-lattice` reconciles the schedule against the spine over the
fact store. The schedule's 79 `Total <fund>` and section lines are not series;
the mapping engine's `CheckTotals` asserts them at build time, per rule and part,
at zero tolerance.
