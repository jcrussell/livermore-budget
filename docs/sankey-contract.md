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
One code path, one layout.

**The stem is a function of the whole column list**, and `project.Stem` is the
only place that rule is spelled — `fisc export` writes the files, `fisc verify`
says one was not built, and `project.PublishedDocuments` declares them, so a
second spelling is a chance for the site to serve a document under a name
nothing else expects. A projection publishing one document takes its name
verbatim; among several, the opening published slice keeps the bare name and the
rest are suffixed by every column they carry, with the basis spelled out
whenever it is not the published one. Today that is seven: `sankey` and `sankey-2027` for the spine's two published
years, `revenue-trends`, and the drill-down's four — `fund-flows`,
`fund-flows-2024-actual`, `fund-flows-2025-revised` and `fund-flows-2027`. All
seven are pinned as literal strings by `TestTheCommittedStemsAreUnchanged`,
because these are the paths this document promises and the values the year radio
carries. The browser `fetch`es it; it is never inlined into
the page, because a provenance file you cannot curl on its own is not much of
an audit trail.

Beside it, `<output>/extracted/<doc-id>/pages/pNNNN.txt` carries the committed
extraction of every page the site cites, copied out of `data/extracted/`. That
is what a citation on the page points at, so both classes — the city's PDF at
`#page=N` and the extracted text — resolve with no third party involved;
`fisc export --source-browse-url` cites a browsable copy of the repository
instead.

**Cited means cited by the site, not by a chart**, and the distinction is
load-bearing since the fact store began shipping. This sentence used to say
"only for the cited pages", meaning the pages named in some projection's
`metadata.sources`. The published record store covers a page no chart draws —
p76's transfer schedule is in scope `transfers-by-fund`, which no projection
selects — so under the old rule a provenance link resolved to a record file
sitting beside a 404. Worse, the set was unstable: a page entered and left the
published extraction as views were added, with no event anyone could see. The
pages `facts/index.json` publishes are cited, so the extraction covers every
locator the site can resolve. It is still not the whole corpus: 36 pages of 786.

And `<output>/facts/` carries the record store itself — the shards, the CSV and
the index. It has a contract of its own:
[`fact-store-contract.md`](fact-store-contract.md). Anything else the site has
to ship travels the same channel (`export.Options.Files`).

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
    "caveats": [
      {"id": "...", "summary": "...", "text": "...", "applies_to": ["..."]}
    ]
  },
  "nodes": [{
    "id": "revenue/taxes/property", "label": "Property Taxes",
    "tier": 0, "parent": "", "constraint_tier": "", "role": "revenue_source",
    "derived": false, "rationale": "", "source_note": ""
  }],
  "links": [{
    "source": "revenue/taxes/property", "target": "fund-group/general",
    "value_cents": 6414376200, "kind": "external", "transfer_id": "",
    "fact_ids": ["fisc-f-..."],
    "locators": [{"doc_id": "livermore-budget-fy2026-2027", "pages": [66]}],
    "derived": false
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

### A caveat is an object, not a string

It carries an **`id`**, a one-line **`summary`**, the **`text`** that used to be
the whole caveat, and **`applies_to`**: the node ids the caveat is about.

The id goes into a **published URL fragment** and is stable across rewordings of
the other two fields, so a bookmark or a citation survives an edit to a
sentence. A document repeating an id is refused at build time by
`project.ValidateCaveats`.

**The fragment is `caveats.html#caveat-<stem>--<id>`, not `#<id>`**, because the
id alone does not identify a caveat: it identifies a caveat *in a document*, and
the same id carries different text in different documents (see below). The site
has one spelling of that composition, `caveatAnchor` in
`internal/export/page.go`, and `buildCaveatsPage` refuses two entries claiming
one anchor — across documents, which is the case `ValidateCaveats` cannot see.
An anchor collision fails silently: the page renders, the anchor resolves, and
the reader is shown a sentence about something else.

`applies_to` is **empty for a caveat about the schedule rather than about a
mark**, and empty means document-wide rather than not-yet-filled-in. Where it is
non-empty, every id must name a node **this** document carries — `ValidateCaveats`
is given the drawn node set and refuses otherwise, because a caveat pointing at a
node that is not there marks nothing, and marking nothing is indistinguishable
from having nothing to mark.

**One id may carry more than one text.** `transfer-legs-unpaired` has three,
picked by whether the legs balance and whether the residual meets the printed
to-CIP column; which sentence a document gets is a fact about that document's
own arithmetic, not three different caveats. A page listing more than one
document's caveats must therefore key on `(id, document)` and must not assume
one text per id.

Determinism: nodes sorted by `(tier, id)`, links by `(source, target)`,
`fact_ids` ascending, `locators` by `doc_id` with `pages` ascending inside each
and every page once. Two builds of the same facts are byte-identical.

### A link cites its facts twice, and the two citations are not redundant

`fact_ids` names *which* facts a link summed. `locators` says *where they were
printed* — the `(doc_id, page)` pairs, in the same shape as `metadata.sources`,
so a client resolves both with one function.

Only one of them survives a rule change. `fact.MakeID` hashes `rule_id`, so
splitting or revising a rule moves every id on the pages it covers; a citation
by id would 404 after an edit that altered no figure. `(doc_id, page)` cannot
move, and the fact store's shard path is *computed* from it — see
[`fact-store-contract.md`](fact-store-contract.md) — so a mark on the chart
resolves to the records behind it in one fetch with no index. That is what
`locators` is for and it is the only thing it is for.

Neither replaces the other. A page holds facts from several rules, so a locator
cannot say which facts a link summed, which is what `link-values-tie-to-facts`,
`counts-reconcile` and `counts.facts_cited` all need. Both are published on
every link.

It is a **page** locator. `fact-store-contract.md` writes a locator as
`(doc_id, page, offset)`; `offset` addresses one printed figure and a link is
an aggregate, so it stops at the page. Measured over the six published
documents: every link resolves to 1 or 2 pairs.

`locators` was added without bumping `schema_version`. The project is
pre-release, so the number is not yet a promise to any reader outside this
tree, and the three constants that must agree — `project.SchemaVersion`,
`export.SchemaVersion` and `app.js`'s `SCHEMA_VERSION` — stay at 1 together.
Recorded as a decision on fisc-5hxr rather than assumed, because
[`revenue-trends-contract.md`](revenue-trends-contract.md) says a bump is a
decision and not a number someone increments. The next additive key is
re-decided, not waved through on this precedent.

## Tiers and node ids

| tier | meaning | id form |
|---|---|---|
| 0 | revenue source | `revenue/<slug>` |
| 2 | fund group | `fund-group/<type>` |
| 3 | fund | `fund/<number>` |
| 4 | department | `dept/<slug>` |
| 5 | object category | `expenditure/<slug>` |

Plus the flow endpoints that are not part of that hierarchy: `transfers/in`
(tier 0), `transfers/out` (tier 5), `fund-balance/reserve-increase` (tier 5),
`fund-balance/draw` (tier 0), `fund-balance/contribution` (tier 5).

**Tier 1 is not a layer, and this table used to say it was.** Earlier revisions
gave tier 1 as a `constraint/<tier>` node between the revenue source and the fund
group. It cannot be one, and the refutation is arithmetic rather than taste: a
constraint tier is a property of a **fund**, and the fund groups do not partition
along it. Counting `data/funds.yaml` by `type` x `constraint_tier`, `capital`
holds 3 committed funds and 43 restricted-by-law; `special-revenue` holds 37
restricted-by-law, 2 unknown and 1 committed. So `fund-group/<type>.parent =
constraint/<tier>` has no single answer, and a layer whose parent edge is
undefined is not a layer. The constraint tier rides as the `constraint_tier`
**field** on a tier-3 node instead — see below.

The number 1 is left unused rather than renumbering. Tiers 2-5 are published in
`node.tier` today, and shifting them would silently change the meaning of every
document already written.

**The documents that use the other tiers are elsewhere.** The drill-down
(`docs/general-fund-drilldown-contract.md`) publishes tiers 0, 2, 3, 4 and 5 over
Budget Book pp.127-140 and pp.167-170, and states its own counts there.

**How many nodes a tier holds is a property of the DOCUMENT, not of the
hierarchy.** A second document at another scope draws a different set: the
citywide spine (pp.66-67) prints six fund groups, while revenue-by-fund
(pp.131-140) prints seven, carrying Permanent Funds that pp.66-67 give no column
at all (fisc-u8o). There is no tier count that holds across all documents, so
each states its own below rather than inheriting the spine's.

**On the spine** (`sankey.json`): tier 0 = 12 nodes, tier 2 = 6, tier 5 = 7,
which is the whole of its 25. Tiers 3 and 4 are empty, because pp.66-67 publish
neither a fund nor a department axis.

Those counts are per TIER and include the flow endpoints, which is why they are
larger than the id-form counts a reader might tally from the table above: tier 0
is 10 `revenue/` nodes plus `transfers/in` and `fund-balance/draw`, and tier 5 is
4 `expenditure/` nodes plus `transfers/out`, `fund-balance/reserve-increase` and
`fund-balance/contribution`.

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

## constraint_tier

`constraint_tier` is `""` on every **spine** node, and that is correct rather
than lazy: `funds.yaml` records a constraint tier per **fund**, and pp.66-67
publish only fund **groups**, whose columns contain funds of several tiers.
Inventing one there would be an editorial classification presented as published
data.

**Where a node does carry one, the value is ours and the node must say so.**
`data/funds.yaml`'s own header is explicit: `constraint_tier` and
`restriction_note` are *"DERIVED — our reading of the 'Description of Funds'
narrative (pp. 258-261) — and must not be presented as something the city
printed."* A tier-3 fund node is in the opposite position from the two
fund-balance nodes: the **node** is published (the city prints the fund and its
revenue) while the **attribute** is inferred. So `node.derived` stays `false` —
setting it would claim the city did not print the fund, which is the opposite
error — and the disclosure travels on the two fields beside it instead:

- `source_note` cites `data/funds.yaml` and pp.258-261;
- `rationale` carries that fund's `restriction_note`, which is the reading itself.

**Four documents publish a constraint tier**, so the paragraph above is a
description rather than the obligation it was written as. The four `fund-flows`
years carry 247 nodes with a non-empty `constraint_tier` between them — 61, 65,
60 and 61 — of which 204 are `restricted-by-law`, 32 `committed`, 7 `unknown` and
4 `discretionary`. `sankey` and `sankey-2027` publish none: the spine classifies
no node, which is `""`'s meaning below. Read *non-empty* rather than *present*:
there is no `omitempty` here, so every node in every document carries the key.

What those documents owed, and carry: `source_note` and `rationale` on every
tier-bearing node; the disclosure sentence in `metadata.caveats`, held as a
constant in `internal/project` so the caveat and the check cannot drift apart;
and `constraint-tier-vocabulary`, which fails a node carrying a tier without the
source note and the restriction note it was read from.
`derived-nodes-justified` does not cover it — that check inspects only
`derived: true` nodes, and these are deliberately not among them.

**`""` and `unknown` are different claims and must not be read as one.** `""`
means *this document does not classify this node* — the spine's answer, and the
answer for every node that is not a fund. `unknown` means *we read pp.258-261 and
could not tell*; `data/funds.yaml` uses it for exactly two funds. Collapsing them
would be the absent-is-not-zero mistake in a new field.

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

`transfer_id` is `""` on every link, and **mapping p76 does not fix that on its
own**. `fact.Fact` carries no field that could hold a pairing and
`internal/project/sankey.go` writes the empty string unconditionally, so
`fisc-1wr.1`'s "every transfer_id has two equal legs" check is **vacuous** — it
must report as such, not as a pass. Retiring it needs a projection that selects
p76's scope *and* a `Link.TransferID` derived from the two legs' shared
`(doc_id, page, offset)`. That is `fisc-9gh`, not `fisc-5gk.3`, and an earlier
draft of this paragraph said otherwise.

The residual is not waiting on that mapping either, and mapping p76 will not
close it. p76's own grand total *is* the transfers-in side, to the cent: each of
its destination sections equals the matching `TRANSFER IN:` cell on pp.66-67,
and the General Fund's out-flows equal `TRANSFER OUT:`.

**The difference is a column the city prints.** pp.72-75 are the same
sources-and-uses schedule carrying a `Transfers Out to CIP` heading, which
pp.66-67 fold into `TRANSFER OUT:` and omit from `TRANSFER IN:` entirely:

```
p0073.txt:58   Transfers Out $21,525,997   Transfers Out to CIP $38,086,737
p0075.txt:58   Transfers Out $21,624,633   Transfers Out to CIP $50,762,251
```

$21,525,997 is p76's grand total to the cent, and $38,086,737 is
`headline.transfer_residual_cents` to the cent. The residual is therefore not a
discrepancy at all — it is transfers to capital projects, itemised under a
heading, on a schedule this project has not yet mapped.

| transfers out to CIP, FY2026 | | source |
|---|---|---|
| Capital Funds | 28,373,590 | derived |
| Enterprise Funds | 9,353,147 | derived |
| Special Revenue Funds | 320,000 | derived |
| Internal Service Funds | 40,000 | `p0073.txt:53` |
| **total** | **38,086,737** | `p0073.txt:58` |

Read those rows as *`TRANSFER OUT:` for the group, minus the transfers p76 shows
that group **paying*** — attributed by the payer named in each row label, not by
`out − in` per group. The General Fund's $10,037,797 is itemised in full and
contributes zero. Capital pays $211,150 of its $28,584,740, so nearly all of it
is to-CIP: Traffic Impact Fee (510), County Measure D (550) and State − Gas Tax
(560) are `type: capital` in `data/funds.yaml`, and p76 lists all three as
payers. An earlier version of this table said Capital pays nothing p76 lists and
put its whole `TRANSFER OUT:` in the residual, with the $211,150 landing on
Special Revenue instead. That was wrong in both rows.

**Only the total and the Internal Service row above are printed figures.**
pp.72-75 give to-CIP per major fund and then a single aggregate line for all
non-major funds — $28,693,590 in FY2026 (`p0073.txt:56`) — so the split between
Capital and Special Revenue is derived by difference from p67 rather than read.
That is the reason to state the pair rather than the split, and it is *not*
because the split is impossible: an earlier draft claimed FY2026-27 would need a
negative to-CIP of −$117,485, which came from reading p76's $1,018,035 of
non-major sources as special-revenue sources alone. Both years divide cleanly
(FY2027: Capital 35,830,251 + Special Revenue 100,000 = 35,930,251,
`p0075.txt:56`).

Those four rows are pinned twice over now, and neither pin existed when the
wrong figures were written. `TestP76SourcesDecomposeTheResidualByFundType`
(19bb265) resolves every payer through `registry.FundByLabel` and asserts
`spine_TRANSFER_OUT == p76_paid + to_CIP` for all six groups in both budget
years; `transfers-detail-ties-to-spine` then makes the same claim over the
published facts, with the to-CIP figures declared in `internal/check` as
`toCIP`. What both replaced is
`TestP76AccountsForTheInSideAndNoneOfTheResidual`, which asserts only that
Capital and Internal Service together do not exceed the whole residual -- a
bound loose enough that the wrong figures satisfied it. Still open is `fisc-4ac`:
tying the declared constants to `headline.transfer_residual_cents` in both
projected years, which is the one direction neither test covers.

`fisc-1wr.4` asks for a residual node. This contract states the residual as
`headline.transfer_residual_cents` instead, and has `verify` assert it equals
out minus in. A synthetic link into `transfers/in` or out of `transfers/out`
would unbalance that node, and splitting the $59.6M into matched and unmatched
portions would publish a division the city never printed. What goes stale the
day p76 lands is the prose above about the schedule being unmapped, not this
figure.

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
