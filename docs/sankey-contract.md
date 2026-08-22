# The sankey.json contract

Frozen before implementation so `internal/project` (the producer),
`internal/export` (the packager) and `site/app.js` (the consumer) could be
written at the same time by people who could not see each other's code.

`testdata/sankey.golden.json` is the worked example: real FY2026 figures read
out of the committed pp.66-67 artifacts, with real fact ids. Read it alongside
this document — where the two disagree, the golden file is a bug.

**This file is the interim home.** It moves to the package doc comment on
`internal/project` once that package exists (`fisc-gxa.1`); a contract that
lives only in prose drifts from the code that implements it.

## Where it goes

`<output>/data/<projection>.json`, so `fisc export -o dist` writes
`dist/data/sankey.json` and `fisc export -o site` writes `site/data/sankey.json`.
One code path, one layout. The browser `fetch`es it; it is never inlined into
the page, because a provenance file you cannot curl on its own is not much of
an audit trail.

Beside it, `<output>/extracted/<doc-id>/pages/pNNNN.txt` carries the committed
extraction of every page `metadata.sources` cites, copied out of
`data/extracted/` and only for the cited pages. That is what a citation on the
page points at, so both classes — the city's PDF at `#page=N` and the extracted
text — resolve with no third party involved; `fisc export --source-browse-url`
cites a browsable copy of the repository instead. Anything else the site has to
ship travels the same channel (`export.Options.Files`).

## Shape

```jsonc
{
  "schema_version": 1,
  "projection": "sankey",
  "metadata": {
    "generated_by": "...", "fiscal_year": 2026, "fiscal_year_label": "FY 2025-26",
    "basis": "adopted", "scope": "all-funds-gross",
    "currency": "USD", "units": "cents",
    "sources": [{"doc_id": "livermore-budget-fy2026-2027", "pages": [66, 67]}],
    "headline": { /* see below */ },
    "counts": {"facts": 120, "facts_cited": 58, "nodes": 25, "links": 58},
    "caveats": ["..."]
  },
  "nodes": [{
    "id": "revenue/taxes/property", "label": "Property Taxes",
    "tier": 0, "parent": "", "constraint_tier": "", "role": "revenue_source",
    "derived": false, "rationale": "", "source_note": ""
  }],
  "links": [{
    "source": "revenue/taxes/property", "target": "fund-group/general",
    "value_cents": 6414376200, "kind": "external", "transfer_id": "",
    "fact_ids": ["fisc-f-..."], "derived": false
  }]
}
```

Every key is present on every object, in declaration order. **No `omitempty`,
no `null`** — the same discipline as `fact.Fact`, and for the same reason: a
key that vanishes when it is empty makes a diff between two releases read as a
structural change. Absent strings are `""`; the client writes `node.parent || null`
if it wants nullish semantics.

Money is always an integer `value_cents`. Never a float, never a string. The
largest figure here is 2.99e10 cents, exact in float64 and safe in JS.

**Zero-valued links are omitted.** Most of the revenue grid is zero — a
category that only the General Fund collects is a dash in the other five
columns — and d3-sankey draws zero-height paths that churn node order. The
*facts* still exist; a zero the city printed is a fact. Only the link is
dropped.

Determinism: nodes sorted by `(tier, id)`, links by `(source, target)`,
`fact_ids` ascending. Two builds of the same facts are byte-identical.

## Tiers and node ids

| tier | meaning | id form | on the spine |
|---|---|---|---|
| 0 | revenue source | `revenue/<slug>` | 10 |
| 1 | constraint tier | `constraint/<tier>` | — |
| 2 | fund group | `fund-group/<type>` | 6 |
| 3 | fund | `fund/<number>` | — |
| 4 | department | `dept/<slug>` | — |
| 5 | object category | `expenditure/<slug>` | 4 |

Plus the flow endpoints that are not part of that hierarchy: `transfers/in`
(tier 0), `transfers/out` (tier 5), `fund-balance/reserve-increase` (tier 5),
`fund-balance/draw` (tier 0), `fund-balance/contribution` (tier 5).

`counts.facts` is the filtered input count — every fact matching the fiscal
year, basis and scope. `counts.facts_cited` is how many of those a link
actually carries. The gap is exactly the zero-valued cells (a dash is a printed
fact but earns no link) plus the two stock rows, so it is a quantity a check can
assert rather than a discrepancy a reader has to explain away. For FY2026:
120 = 58 cited + 50 zero + 12 stock.

**Node labels: a built-in wins over the registry.** That inverts what you would
expect from a curated data file, and it is deliberate. A taxonomy `label` names
a *category*, which may span several schedules; the words on a node have to be
the words on the page this projection read.
`fund-balance/reserve-increase` is the case — the taxonomy calls it
"Reserve Increase / (Use)", after the p75 column header the category was merged
with, while p66 prints ADDITION TO RESERVES over the figures actually read here.
The six fund-group labels are built-in for a different reason: `funds.yaml`
binds a fund to a type and records no words for the type itself, so the labels
come from the pp.66-67 column headers. Letting the registry win would also mean
the rendered site and `testdata/sankey.golden.json` disagreed on any node listed
in both, quietly retiring the golden as a contract test.

The slug in every id is a `data/taxonomy.yaml` slug. Do not coin new ones — in
particular `ADDITION TO RESERVES` is `fund-balance/reserve-increase`, whose
`document_term` is that exact printed string.

`constraint_tier` is `""` on every spine node, and that is correct rather than
lazy: `funds.yaml` records a constraint tier per **fund**, and this schedule
publishes only fund **groups**. Inventing one would be an editorial
classification presented as published data.

## link.kind

`external | internal_transfer | internal_service | fund_balance`

This is `fisc-gxa.1`'s set plus `fund_balance`; the bead carries the amendment.

- `external` — money crossing the city's boundary. The only kind in the
  headline.
- `internal_service` — Internal Service Fund charges, billed by one city
  department to another. $18,969,834 in, $25,077,367 out in FY2026. Real money
  in a real fund, but counting it as revenue *and* as the paying department's
  expenditure double-counts it.
- `internal_transfer` — transfers between funds.
- `fund_balance` — a draw on or contribution to accumulated balance, and
  additions to reserves. Not external money; not a transfer either, because
  nothing moves between funds.

## headline

```
all_funds_gross_revenue_cents      299,969,007   ties to the printed schedule
all_funds_gross_expenditure_cents  254,095,412   ties to the printed schedule
external_revenue_cents             280,999,173   gross, net of internal_service
external_expenditure_cents         229,018,045   gross, net of internal_service
internal_transfer_in_cents          21,525,997
internal_transfer_out_cents         59,612,734
naive_expenditure_cents            313,708,146   the WRONG number, published on purpose
transfer_residual_cents             38,086,737   out minus in; see caveats
```

Two rules here are worth more than the numbers.

**Never name a key for a concept the code does not compute.** An earlier draft
called $299,969,007 `external_revenue_cents`. It is not external — the scope is
literally `all-funds-**gross**` and that figure includes internal service
charges. Both are published, under names that say which is which.

**`naive_expenditure_cents` is deliberately the wrong answer.** Summing the
expenditure column gives $313,708,146 where the city spends $254,095,412, a 23%
inflation from counting transfers twice. It ships so the page can show the
error it is avoiding, and so a check can assert the headline is not equal to it.

## derived

`derived: true` means *we* inferred it; `false` means the city printed it. The
distinction is the site's whole premise, so it is stated on every node and link
rather than left to be inferred from a missing key.

On the spine, exactly two things are derived, and both come from one row.
`CHANGE IN WORKING CAPITAL` is printed once per column with a sign
(`(1,034,154)` for the General Fund). A Sankey cannot draw a negative link, so a
negative change becomes a `fund-balance/draw` → fund-group link and a positive
change a fund-group → `fund-balance/contribution` link. That decomposition is
ours. Both nodes carry a `rationale` and a `source_note`, and `fisc verify`
fails on any `derived: true` node that does not.

`BEGINNING` and `ENDING WORKING CAPITAL` are recorded as facts — they are cells
the city printed — but they are **stocks, not flows**, and get no link. A reader
reconciling the chart against p66 will find three rows unaccounted for, so
`metadata.caveats` says so explicitly.

## The transfer residual

Transfers out ($59,612,734) exceed transfers in ($21,525,997) by $38,086,737.
Two separate facts sit behind that sentence and an earlier draft of this section
ran them together.

`transfer_id` is `""` on every link **because the p76 transfer schedule is not
yet mapped** (`fisc-5gk.3`), so `fisc-1wr.1`'s "every transfer_id has two equal
legs" check is **vacuous** — it must report as such, not as a pass. Mapping p76
fixes this.

The residual is **not** waiting on that mapping, and mapping p76 will not close
it. p76's own grand total *is* the transfers-in side, to the cent: each of its
destination sections equals the matching `TRANSFER IN:` cell on pp.66-67, and
the General Fund's out-flows equal `TRANSFER OUT:`. The city itemises every
transfer received and none of the difference, which sits where its own schedule
never goes:

| unexplained transfers out, FY2026 | |
|---|---|
| Capital Funds | 28,584,740 |
| Enterprise Funds | 9,353,147 |
| Special Revenue Funds | 108,850 |
| Internal Service Funds | 40,000 |
| **total** | **38,086,737** |

Read those rows as *`TRANSFER OUT:` for the group, minus the transfers p76 shows
that group **paying*** — attributed by the payer named in each row label, not by
`out − in` per group. The two differ: Capital and Internal Service pay nothing
p76 lists and receive nothing, so their whole `TRANSFER OUT:` is unexplained,
while the General Fund's $10,037,797 is itemised in full and contributes zero.
Only the first and last rows are pinned by a test today
(`TestP76AccountsForTheInSideAndNoneOfTheResidual`), because attributing the
other two needs a payer-to-fund-group lookup the fact model cannot yet carry
(`fisc-4rh`).

`fisc-1wr.4` asks for a residual node. This contract states the residual as
`headline.transfer_residual_cents` instead, and has `verify` assert it equals
out minus in. A synthetic link into `transfers/in` or out of `transfers/out`
would unbalance that node, and splitting the $59.6M into matched and unmatched
portions would publish a division the city never printed. What goes stale-red
the day p76 lands is `transfer_id` and the vacuous check, not this figure.

## What the fund groups must satisfy

For every fund group and fiscal year:

```
revenue + transfers_in + fund_balance_draw
    == expenditure + transfers_out + reserve_increase + fund_balance_contribution
```

FY2026, all six exact:

| fund group | in | out |
|---|---:|---:|
| general | 159,388,024 | 159,388,024 |
| enterprise | 80,761,261 | 80,761,261 |
| capital | 29,646,095 | 29,646,095 |
| debt-service | 6,984,597 | 6,984,597 |
| special-revenue | 29,279,560 | 29,279,560 |
| internal-service | 25,117,367 | 25,117,367 |

These are the city's own `TOTAL SOURCES` and `TOTAL USES` rows, which is what
makes the identity a cross-check rather than a restatement of itself.

## Row-to-slug map

The seam between the rule file (`fisc-mq4.7`) and the projection
(`fisc-gxa.1`). Both sides code against this table.

| printed row | `category` | `mapping.Kind` | projection |
|---|---|---|---|
| Property Taxes | `taxes/property` | revenue | `revenue/taxes/property` → group |
| Other Taxes | `taxes/other` | revenue | ” |
| Intergovernmental | `intergovernmental` | revenue | ” |
| Charges for Services | `charges-for-services` | revenue | ” |
| Use of Money And Property | `use-of-money-and-property` | revenue | ” |
| Contributions Outsourced | `contributions-outsourced` | revenue | ” |
| Miscellaneous Revenue | `miscellaneous-revenue` | revenue | ” |
| Sales Taxes | `taxes/sales` | revenue | ” |
| Fines & Forfeitures | `fines-and-forfeitures` | revenue | ” |
| Licenses & Permits | `licenses-and-permits` | revenue | ” |
| Wages & Benefits | `wages-and-benefits` | expenditure | group → `expenditure/…` |
| Services & Supplies | `services-and-supplies` | expenditure | ” |
| Capital Outlay | `capital-outlay` | expenditure | ” |
| Debt Services | `debt-services` | expenditure | ” |
| TRANSFER IN: | `transfers/in` | transfer_in | `transfers/in` → group, `internal_transfer` |
| TRANSFER OUT: | `transfers/out` | transfer_out | group → `transfers/out`, `internal_transfer` |
| ADDITION TO RESERVES | `fund-balance/reserve-increase` | fund_balance | group → node, `fund_balance` |
| CHANGE IN WORKING CAPITAL | `fund-balance/change` | fund_balance | `< 0` draw → group; `> 0` group → contribution; `== 0` no link. `derived: true` |
| BEGINNING WORKING CAPITAL | `fund-balance/beginning` | fund_balance | **excluded — stock** |
| ENDING WORKING CAPITAL | `fund-balance/ending` | fund_balance | **excluded — stock** |

A revenue or expenditure link whose fund group is `internal-service` takes
`kind: internal_service`; every other one takes `external`.

`sign: contra` rows net into their parent category before links are built.
pp.66-67 print none — the contra rows are on p127 (ERAF, RPTTF) — so that path
ships untested unless the projection's own tests synthesize one.
