# The revenue-trends.json contract

Frozen before implementation, for the same reason `docs/sankey-contract.md` was
and on that file's stated precedent: a published document shape is a decision in
this repository, not a bullet inside the bead that implements it (`fisc-2n5`,
`fisc-oxf`, and before them `fisc-j8f` and `fisc-zna`).

This is the **second** document the site publishes, and the first that is not a
graph. Where the Sankey is one fiscal year of the citywide spine, this is four
printed columns of one detail schedule: Budget Book pp.127-140, *Revenue Sources
by Fund*. Every figure in it is already in `facts/facts.jsonl` — 924 facts, no
new extraction, no new mapping.

## Where it goes

`<output>/data/revenue-trends.json`. Same layout, same relative paths, same `extracted/<doc-id>/pages/pNNNN.txt` provenance tree
(`export.PageTextDir`), which for this document is fourteen pages rather than
the spine's two.

It ships under a name of its own rather than inside a column, and that follows
from the same fact the next paragraph turns into a rule: a document stating no
fiscal year and no basis names no column to be a schedule of.

**The stem carries no fiscal year, and that is a rule rather than a spelling.**
`project.PublishedStem(name, year)` suffixes every year but the opening one, so
the Sankey ships as `sankey.json` and `sankey-2027.json`. It applies only to a
projection that publishes **one document per year**. This projection publishes
one document spanning four columns, so its stem is its `Name()` verbatim.
Putting it through `PublishedStem` would write two byte-identical files at
`revenue-trends` and `revenue-trends-2027`, one of which would be a lie about
which year it covers.

## Shape

```jsonc
{
  "schema_version": 1,
  "projection": "revenue-trends",
  "metadata": {
    "generated_by": "fisc ...",
    "scope": "revenue-by-fund",
    "currency": "USD",
    "units": "cents",
    "columns": [
      {"fiscal_year": 2024, "fiscal_year_label": "FY 2023-24", "basis": "actual",
       "comparable_group": "actual"},
      {"fiscal_year": 2025, "fiscal_year_label": "FY 2024-25", "basis": "revised",
       "comparable_group": "revised"},
      {"fiscal_year": 2026, "fiscal_year_label": "FY 2025-26", "basis": "adopted",
       "comparable_group": "adopted"},
      {"fiscal_year": 2027, "fiscal_year_label": "FY 2026-27", "basis": "adopted",
       "comparable_group": "adopted"}
    ],
    "sources": [{"doc_id": "livermore-budget-fy2026-2027",
                 "pages": [127, 128, 129, 130, 131, 132, 133, 134, 135, 136, 137, 138, 139, 140]}],
    "counts": {"facts": 924, "series": 231, "points": 924},
    "caveats": [
      {"id": "...", "summary": "...", "text": "...", "applies_to": []}
    ]
  },
  "series": [{
    "series_id": "fisc-s-ce9117881328",
    "label": "Industrial Construction Tax",
    "fund": 100, "fund_name": "General Fund", "fund_group": "general",
    "kind": "revenue", "category": "taxes/other", "category_label": "Other Taxes",
    "points": [
      {"fiscal_year": 2024, "basis": "actual", "amount_cents": 0,
       "fact_id": "fisc-f-0ba7b00dfaf7", "doc_id": "livermore-budget-fy2026-2027",
       "page": 127, "offset": 3579, "token": "-", "derived": false},
      {"fiscal_year": 2025, "basis": "revised", "amount_cents": 20000000,
       "fact_id": "fisc-f-977588b137ae", "doc_id": "livermore-budget-fy2026-2027",
       "page": 127, "offset": 3588, "token": "200,000", "derived": false}
      /* ... 2026 and 2027 ... */
    ]
  }]
}
```

That worked example is real: `fisc-s-ce9117881328` and its four fact ids are
computed off the committed store, not invented for the document.

Every key is present on every object, in declaration order. **No `omitempty`** —
`fact.Fact`'s discipline and `internal/project`'s, for the reason stated in both:
a key that vanishes when it is empty makes a diff between two releases read as a
structural change. **And no `null`**, though that is this document's own
property and not the store's: `fund` is the one key the store publishes `null`
in, for an absent fund, and every series here sits under a numbered fund. The
ACFR fund-balance documents publish `null` there on every series (see
`docs/acfr-history-contract.md`).

Money is an integer `amount_cents`, and it is **signed**. **Nine** of the 924
points are negative: General Fund ERAF and RPTTF Reduction, four columns each,
plus Prior Year - Unsecured in the FY2023-24 actual column alone, all printed in
parentheses. Find them with `amount_cents < 0` and not with a sign field — this
document publishes no `sign` key at any level, so the filter an earlier draft of
this paragraph named matches nothing. The Sankey nets contra rows into their parent
category before building links; this document does not, because a series is a
**printed row** and those two rows are printed. Summing a fund's series
therefore works arithmetically without a special case.

## The envelope, and what is not in it

`schema_version`, `projection`, and within `metadata` — `generated_by`, `scope`,
`currency`, `units`, `sources`, `counts`, `caveats` — are the **envelope**: what
any document of this project carries. `internal/project/document.go` already
holds the shared half (`Source`, `Counts`, `encode`) and already records why
`Metadata` and `Headline` did not move with them.

Four things the Sankey's metadata carries are **absent here, deliberately**:

- `fiscal_year` and `fiscal_year_label` — singular. This document spans four.
- `basis` — singular. This document spans three.
- `headline` — its keys are `all_funds_gross_revenue_cents` and
  `naive_expenditure_cents`. They are the spine's and mean nothing here.

Absent is not zero ([`AGENTS.md`](../AGENTS.md)): a trends document is not
defective for lacking a fiscal year, so it must not publish `"fiscal_year": 0`
or `"basis": ""` to keep a shape it is not of. Consumers key on `projection`.

**The two metadata structs share field names but are not embedded**, and the
byte constraint is why. `encoding/json` emits fields in declaration order, and
the Sankey's order is `generated_by, fiscal_year, fiscal_year_label, basis,
scope, currency, units, sources, headline, counts, caveats` — the shared fields
are *interleaved* with the private ones, so hoisting them into an embedded
`Envelope` would reorder the Sankey's keys and change
`testdata/sankey.golden.json`. That is `fisc-2u4`'s option (a), taken for a
measured reason rather than a stylistic one. A test asserts the shared JSON tags
have not drifted apart, since nothing else couples them.

## series_id

```
sha256(doc_id \x1f rule_id \x1f row_path \x1f row_label \x1f column_path)[:12], prefixed "fisc-s-"
```

That is `fact.MakeID`'s tuple **minus** `fiscal_year` and `basis`, and the
relationship is not a coincidence: two facts are the same printed row at
different times exactly when they agree on everything the fact id hashes except
the year and the basis. So the series identity is derivable by anyone holding
`facts.jsonl`, and it is stable when a fifth year is mapped.

The five-component join cannot collide with a fact id: the arity differs, the
prefix differs (`fisc-s-` against `fisc-f-`), and `\x1f` is the separator in both
precisely because every component can contain `/` and some can contain `-`.

`row_label` is `mapping.Row.PrintedLabel()`, the same string the fact record
publishes — not `Row.Label` and not `Row.Identity()`, for the reasons
`fact.MakeID`'s doc comment gives.

**Measured**: 231 series, every one with exactly four points, no duplicates.

## The four columns are not one measurement

| column | basis | what it is |
|---|---|---|
| FY 2023-24 | `actual` | money that moved |
| FY 2024-25 | `revised` | a mid-year re-forecast |
| FY 2025-26 | `adopted` | an intention |
| FY 2026-27 | `adopted` | an intention |

`comparable_group` ships **machine-readable** so a consumer can enforce the
distinction rather than read a caveat about it. The two adopted years share a
group because they are two years of one adopted two-year budget; the other two
stand alone.

**This document publishes no `growth_cents`, no `cagr`, and no inter-column
arithmetic of any kind.** Never name a key for a concept the code does not
compute — and growth from an actual to an adopted figure is not a concept this
project can compute. `derived: true` is the wrong instrument for the same
problem: all 924 figures are printed, so marking them derived would be a
category error. The dishonesty in a trend of these four columns is not in the
numbers; it is in the connecting line, which is a **mark and not a datum**, and
it therefore belongs to whatever renders this document rather than to the
document.

**FY2024 `actual` here is the Budget Book's own restatement, not the ACFR's
audited figure.** `mapping.BasisAudited` exists and is a different basis. This
document must never label a column `audited`; `fisc-4ua.4` is where audited
actuals come from.

## What summing the series does and does not reproduce

This is the section a reader checks the document against, and it is the reason
this contract carries caveats at all. Sum every series into its fund group and
compare with p63 Table 2, *Total Sources – All Funds* (which is revenues plus
transfers in, the same two kinds this scope carries). Twenty of the twenty-eight
cells tie **exactly**:

| fund group | FY2023-24 | FY2024-25 | FY2025-26 | FY2026-27 |
|---|---:|---:|---:|---:|
| general | −737,455 | −914,206 | −480,400 | −486,735 |
| enterprise | 0 | 0 | 0 | 0 |
| debt-service | 0 | 0 | 0 | 0 |
| permanent | 0 | 0 | 0 | 0 |
| capital | −1 | −4,125,627 | 0 | 0 |
| special-revenue | +1 | +500 | 0 | 0 |
| internal-service | 0 | 0 | 0 | 0 |

Every one of the eight non-zero cells is a **named, filed thing**, and none of
them is a defect in this document:

- **general, all four columns** — pp.127-130 print no *General Fund Transfers In*
  row at all, so the schedule cannot carry it. In the adopted years the gap is
  480,400 and 486,735 exactly, which is the spine's own `TRANSFER IN:` General
  Fund cell and is published under scope `transfers-by-fund` off p76. It is
  already `revenue-detail-ties-to-spine`'s one declared exception. The
  historical columns are the same missing row; pp.66-67 print no actual or
  revised column, so nothing on the spine reconciles them.
- **capital FY2024-25, −4,125,627** — `fisc-zl9`. General Fund CIP Reserves has
  **no section** on pp.131-140, so no series is short; the row simply does not
  exist in this schedule. p63 prints 34,839,275 and the schedule's own funds sum
  to 30,713,648.
- **special-revenue FY2024-25, +500** — `fisc-94b`, and this contract's
  derivation is independent of that bead's. p0139.txt:56 prints
  `Oth Financing Source - 500 - -` in the Police Donations fund; p63 and p192
  are two roll-ups produced differently that both print 19,666,119, exactly $500
  less than the 38 printed `Total <fund>` lines. The citywide schedules exclude
  the line; this one includes it, because the city printed it on the page this
  document reads.
- **capital −1 and special-revenue +1, FY2023-24 only** — the document's own
  rounding. `totals-tie-exactly-only-on-the-spine`: eleven blocks are off by ≤ $5
  and every one is in the FY2023-24 Actual column, p131 and p135 among them.
  These are `stated_total_deltas` material (`fisc-2sd`), not tolerance material.

`applies_to` is empty on every one of them, and by construction: this document
publishes series rather than a graph, so there is no node for a caveat to name.
`ValidateCaveats` is passed a nil node set here and skips that arm rather than
failing every entry against an empty one.

**So `metadata.caveats` states the capital FY2024-25 hole and the General Fund
Transfers In gap, with figures.** Not because a *series* is wrong — none is —
but because a reader who sums this document into fund groups and compares it
with the city's own summary table will find those differences, and a document
that leaves them to be rediscovered is a document that reads as wrong. The two
$1 rounding cells and the $500 are on the page and in this contract, which is
where a reader who gets that far will look.

**A fourth caveat is about neither.** `the-revenue-schedule-is-published-twice`
is carried by this document and by every `fund-flows` document, because the site
publishes pp.127-140 in two places: a chart of one adopted column at a time, and
these tables of all four. That asymmetry is what makes both honest — a row found
in both is one printed figure shown once in each, never a second measurement —
and the sentence is shared rather than written twice, so the two pages cannot be
found disagreeing about it.

**One fund group is not on the spine at all.** `permanent` carries a single
series — fund 470, *Transfers In*, 19,533 in FY2023-24 and zero in the other
three — and pp.66-67 print no Permanent column (`fisc-u8o`). It ties to p63
exactly and to the spine not at all.

## counts

```
facts    924   every fact matching the scope and one of the four columns
series   231
points   924
```

**There is no `facts_cited` here.** The Sankey publishes one because there it is
genuinely smaller than `facts` — a zero-valued cell earns no link and a stock row
earns none either — so the gap is a quantity a check can assert. In this document
every fact in the slice becomes a point: no netting, nothing dropped for being
zero, no stocks. A `facts_cited` would be a third name for a number already
published twice, which is *never name a key for a concept the code does not
compute* failing from the other direction.

`facts` and `points` are equal today and both ship, because they are computed
independently — `facts` off the selection, `points` off the series actually
built. A document that dropped a series publishes `points` below `facts`. The
same divergence is caught directly, and with the row named, by
`trend-points-tie-to-facts`; this is the form of it a reader can see in the file
without running anything.

**No point is dropped for being zero.** The Sankey omits zero-valued *links*
because d3-sankey draws zero-height paths that churn node order; a trend point
has no such problem, and a printed `-` is a published zero ([`AGENTS.md`](../AGENTS.md)).
Dropping it would make a series with a gap indistinguishable from a series the
city stopped printing — which is the exact case `trend-series-are-complete`
exists to catch.

## derived

`derived: false` on all 924 points, and the key ships anyway. The distinction
between what the city printed and what we inferred is the site's whole premise,
so it is stated on every point rather than left to be inferred from a missing
key — the same rule the Sankey applies to every node and link.

Nothing in this document is derived. Unlike the spine, which has to decompose a
signed `CHANGE IN WORKING CAPITAL` into two nodes because a Sankey cannot draw a
negative link, a trend of printed rows infers nothing: `fund_name` and
`category_label` are looked up, not computed, and `series_id` is an identity
rather than a claim.

## Labels

`label` is the row's printed label — the city's words, off the page.

`fund_name` and `category_label` are **registry lookups**, and they are
published rather than left to the client because the client would otherwise need
`data/funds.yaml` and `data/taxonomy.yaml` shipped beside the document to render
a legend.

`fund_name` is why this document widens `project.Labels`. That interface is one
method over *category* slugs today, and it is not enough here: these are 231
series across **70 funds**, and `row_label` is not unique across them —
"Property Taxes" is printed by four different funds and "Use of Money & Prop" by
thirty-eight. A series labelled only by its printed row would be ambiguous on
sight. So `Labels` gains

```go
FundName(number int) (string, bool)
```

kept to the one method actually used (byob-interfaces.2), with `registry.Registry`
growing the matching method so `var _ Labels = (*registry.Registry)(nil)` still
holds. `registry.Fund(number) (registry.Fund, bool)` is not usable directly: it
returns a registry type, and `internal/project` reaches the registry through a
narrow interface precisely so it does not import it.

A miss is not an error: an unknown fund falls back to `""` and the client shows
the number, the same way an unlabelled category falls back to a slug-derived
label. Every one of the 70 resolves today — `fact-funds-resolve` already asserts
it over all 1010 fund-bearing facts.

## schema_version

**One `schema_version` spans both contracts, and it versions the envelope.**

`internal/export`'s copy is pinned to `project.SchemaVersion` by
`TestSchemaVersionIsPinnedToTheProducer` and the client's `site/app.js` copy by
`TestClientSchemaVersionIsPinnedToTheProducer`; all three read `1` and a trends
document at `1` satisfies them. `checkSchemaVersion` is already generic on the
document stem — `fisc-oxf` made its refusal name the document it read rather
than the primary one, "a wrong signpost the moment there are two".

The cost, stated rather than discovered: a change to *either* body bumps the
number for *both*, so the Sankey's consumers are told to re-read a contract that
did not move. That is accepted because the alternative is two version axes and
therefore two refusals, and because the packager refuses on this number before
reading anything — a version it cannot check against the right body is worse
than a version that is occasionally conservative. `fisc-oxf`'s precedent stands:
a bump is a decision, recorded in a bead, not a number someone increments.

## What checks this

Nothing in `internal/check/graph.go` does, and that is the point: those are all
the structural checks there are, they read `Subject.Graphs()`, and this document
has no nodes and no links. Three checks cover it instead.

- **`trend-points-tie-to-facts`** (`fisc-3vz`) — every published point equals its
  fact's `amount_cents` and carries that fact's `doc_id`, `page`, `offset` and
  `token`; a point citing a fact id not in the store is a finding, and so is a
  fact in the slice that no point publishes. It is the trends analogue of
  `link-values-tie-to-facts`, and stricter, because a point is **one** fact
  rather than a netted sum.
- **`trend-series-are-complete`** (`fisc-d5n`) — every series has a point in
  every column, or a declared exception. Separate from the above on that bead's
  own argument: one check is about values, one about shape, and a combined check
  would report "N of M" over two different units. Green the day it lands, which
  is the point — it exists for the day the city drops a printed row from one
  column or adds a line mid-book that only the later years carry. What it does
  **not** catch is a whole column vanishing from the corpus: every series loses
  it together, the projection's declared columns shrink with them, and every
  series is complete over what remains. Nothing here compares the corpus against
  what it used to hold. Filed as `fisc-7dt`.
- **`documents-are-checked`** (`fisc-5ep`) — the check that refuses a document no
  check reads. It grew the arm this document needed: `documentShape` names
  `series` for a projection carrying a `TrendsDocument` and cites the two checks
  above as what reads it. Each arm names its checks, so adding a shape without
  adding checks fails there rather than widening the exemption silently.

`revenue-detail-ties-to-spine` continues to reconcile this schedule against the
spine, unchanged. It reads the fact store directly through `detailSums` and has
never consulted `unprojectedScopes`, so publishing this document costs it
nothing — which is what makes retiring that scope's `unprojectedScopes` entry
safe rather than a loss of coverage.

## What this document is not

Not a chart. It publishes series and leaves the mark to the view, and the
`fisc-4ua.3` argument says why that separation is load-bearing rather than tidy:
grouped bars assert "four measurements" where a line asserts "one quantity over
time", and only one of those is true of these columns. A renderer that draws a
line must break it at the actual-to-budget boundary with a labelled rule. This
contract's job is to make that possible — via `comparable_group` — and to make
the alternative visible rather than to forbid it in prose.

Not the whole of pp.127-140 either, in one respect worth naming: the schedule
prints 79 `Total <fund>` and section lines that this document does not publish
as series, because a total is not a row. They are asserted at **build** time by
the mapping engine's `CheckTotals`, per rule and per part, at zero tolerance.
