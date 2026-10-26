# The sankey.json contract

`internal/project` produces it, `internal/export` packages it and `site/app.js`
consumes it. `schema/projection.schema.json` holds its shape and every document
is validated against it before its bytes are returned; this page holds what the
schema cannot say.

`testdata/sankey.golden.json` is the worked example: real FY2026 figures read
out of the committed pp.66-67 artifacts, with real fact ids. Where the two
disagree, the golden file is a bug.

## Where it goes

`<output>/fy<year>-<basis>.json`, one document per published column. A
projection stating a fiscal year and a basis is **a schedule inside its
column**, keyed by its stem with the year suffix stripped. One that states
neither — `revenue-trends` and the two balance documents carry a series and no
column — ships at `<output>/data/<stem>.json` as itself. The output directory
must be missing, empty, or carry the manifest an earlier export left in
`.fisc-export`.

`project.Stem` is the one spelling of a stem. A projection publishing one
document takes its name verbatim; among several, the opening published slice
keeps the bare name and the rest are suffixed by every column they carry, with
the basis spelled out whenever it is not the published one: `sankey` and
`sankey-2027`, `fund-flows-2024-actual`. `TestTheCommittedStemsAreUnchanged`
pins every published stem. The browser `fetch`es the document; it is never
inlined.

Beside it, `<output>/extracted/<doc-id>/pages/pNNNN.txt` carries the committed
extraction of every page the site cites — the pages `facts/index.json`
publishes, including pages no chart draws — so a citation resolves with no third
party; `fisc export --source-browse-url` cites a browsable copy of the
repository instead. `<output>/facts/` carries the record store:
[`fact-store.md`](fact-store.md).

## Shape

Every key is present on every object, in declaration order: no `omitempty`, no
`null`, absent strings `""`. The keys a document may omit are `headline`, which
the spine alone carries, and a node's `balances`, which fund-sources-uses and
the expansion's fund nodes carry, holding only the balances its page prints and,
where both are printed, their derived `change`.

Money is an integer `value_cents`. The largest figure is 2.99e10 cents, exact in
float64.

**Zero-valued links are omitted.** Most of the revenue grid is zero — a category
only the General Fund collects is a dash in the other five columns. The *facts*
still exist; only the link is dropped.

**A pair may carry one ribbon per kind**: a revenue row reaching the Internal
Service Funds takes its money as an internal service charge and the rest of the
city's as external revenue. Two links of the same kind on one pair are refused.

Nodes sort by `(tier, id)`, links by `(source, target, kind)`, `fact_ids`
ascending, `locators` by `doc_id` with `pages` ascending. Two builds of the same
facts are byte-identical.

### Caveats

A caveat carries an `id`, a one-line `summary`, its `text`, a `theme` from
`project.CaveatThemes`, and `applies_to`: the node ids it is about, empty for a
caveat about the schedule rather than a mark. Every id in `applies_to` names a
node this document carries (`project`'s `validateCaveats`).

The id is in published URL fragments: `caveats.html#caveat-<id>` on the first
copy, `caveats.html#caveats-on-<view>--caveat-<id>` on each later one, and
`caveats.html#caveats-on-<view>--<theme>` for a theme's list. One id carries one
summary and one theme in every document (`buildCaveatIndex`); a figure that
varies by year or schedule belongs in the text. One id may carry several texts:
`transfer-legs-unpaired` has four, picked by the document's own arithmetic.

### A link cites its facts twice

`fact_ids` names *which* facts a link summed; `locators` names the
`(doc_id, page)` pairs they were printed on, the shape of `metadata.sources`.
`fact.MakeID` hashes `rule_id`, so a rule edit that alters no figure moves every
id on its pages; `(doc_id, page)` cannot move, and the fact store's shard path is
computed from it, so a mark resolves to its records in one fetch. A locator stops
at the page because a link is an aggregate. Every link resolves to 1 or 2 pairs.

`project.SchemaVersion`, `export.SchemaVersion` and `site/core.js`'s
`PROJECTION_VERSION` agree, at 1. The page's `window.FISC_CONFIG` carries
`export.PageConfigVersion`, which `site/core.js`'s `SCHEMA_VERSION` equals.

### `partition`

**`partition: true` means the ribbon divides one printed table along a second
axis rather than following money.** Only the departmentwide cross-tab sets it:
Budget Book pp.85-125 print one matrix, divisions down and object categories
across. `node-tiers-are-declared` admits a partition link that descends — the
cross-tab's `expenditure/<object>` (tier 5) into `dept/<division>` (tier 4) —
beside a revenue line rolled up into its category; any other descending link is
refused. The key is present and `false` on every other link.

## Tiers and node ids

A node's id form fixes its tier: `project.TierOf` is the table, and
`node-tiers-are-declared` holds each published node to it.

**`department/` and `dept/` are two id forms at one tier.** pp.167-170 print the
General Fund by DIVISION and pp.85-125 print one block per DEPARTMENT. Five slugs
are in both populations — `city-council`, `city-manager`, `city-attorney`,
`general-services`, `administrative-services` — each a department and its sole
division, and the city prints no other name for either. An id form is read by
cutting at the first slash, so one prefix would make `dept/city-council` mean
different things in different documents.

**`transfer-from/` and `transfer-to/` are ends of a movement, not the funds.**
p76 prints money moving between the city's own funds; `fund/<a>` to `fund/<b>`
is tier 3 to tier 3, which d3-sankey cannot lay out. A payer's end sits at tier 2
with the fund groups and a receiver's at 5 with the object categories. Only
`transfers-by-fund` and `transfers-out` publish them; in those two networks
`transfers/in` and `transfers/out` are containers holding the ends, and
`node-hierarchy-well-formed` allows that only where the endpoint draws no flow
of its own.

**The expansion schedule adds four forms.** `dept/<division>/<object>` is one
division's spending on one object category across every fund, pp.85-125's cell.
`expenditure/fund-group/<type>/<object>` is an object category's money a fund
group's column of pp.66-67 prints and its funds' rows do not; it is derived.
`fund-group-only/<type>` (tier 3) is money pp.66-67 print in a group's column and
no fund's own page carries, and `no-division/<object>` (tier 4) an object
category's money no division on pp.85-125 prints. A remainder's link cites the
printed cell and the members it is the difference of; `project.LinkCents` reads
it as the first less the second.

**Tier 1 is a printed row, not a constraint layer.** The fund groups do not
partition along constraint tier: `data/funds.yaml` puts both committed and
restricted-by-law funds under `capital` and `special-revenue`, so
`fund-group/<type>.parent = constraint/<tier>` has no single answer. The tier
rides as the `constraint_tier` field on a tier-3 node instead.

**`revenue-line/` is its own prefix.** A category slug may carry a slash —
`revenue/taxes/property` is tier 0 — so a line nested under `revenue/` would be
indistinguishable from its parent's form. Its parent is the category the row is
printed under.

**A tier's node count belongs to the document.** The spine (pp.66-67) prints six
fund groups; revenue-by-fund (pp.131-140) prints seven, carrying Permanent Funds
that pp.66-67 give no column (`fisc-u8o`). On the spine, tier 0 holds 12 nodes
(10 `revenue/` plus `transfers/in` and `fund-balance/draw`), tier 2 holds 6, and
tier 5 holds 7 (4 `expenditure/` plus `transfers/out`,
`fund-balance/reserve-increase` and `fund-balance/contribution`). Tiers 1, 3 and
4 are empty: pp.66-67 print each revenue category as one row and no fund or
department axis. The drill-down's tiers are in
[`fund-flows.md`](fund-flows.md).

The slug in every id is a `data/taxonomy.yaml` slug. Do not coin new ones:
`ADDITION TO RESERVES` is
`fund-balance/reserve-increase`, whose `document_term` is that printed string.

### The expansion

The page's chart is the expansion schedule (`project.expansion`'s doc comment is
its shape). At its root frontier it draws the spine's three columns; `splits`
says how each node opens: a revenue category into its lines, Transfers In into
the funds that pay it, a fund group into its funds, the General Fund into its
divisions, an object category into its object rows or, as the alternative, the
divisions that spend it, and Transfers Out into the funds it pays. The
fund-balance ends declare no split. Every split is EXTEND: the node stays drawn
as a hub and its members stand in a column beside it. The General Fund's link to
each division, and an object category's to each division of the alternative,
cites that member's own total; every other split's links are legs carrying
`repeats`, each repeating one leaf's cents and facts. Go publishes no sum for a
hub ribbon; the client's fold adds the legs it draws.

Every node the expansion draws carries `slot`, its column's order key: a root at
its tier's place in the overview's `tiers`, a member in the slot after its
parent's, or before it where the split declares `side` `before`. The hints
`sankey.grow` (`node_px`, `floor`, `ceiling`) and `sankey.split_caps` (`cap`,
`tail` per split key) fit it to a screen; `validateGrowth` and
`validateSplitCaps` refuse at the write what the client could not honour.

### Counts

`counts.facts` is every fact matching the fiscal year, basis and scope;
`counts.facts_cited` how many a link carries; `counts.facts_uncited` the rest,
which on the spine is exactly the zero-valued cells plus the stock rows. For
FY2026: 120 = 58 cited + 62 uncited, the 62 being 50 zero cells and 12 stock
rows. `facts_cited_twice` is 0.

### Labels

**A built-in label wins over the registry.** A taxonomy `label` names a
category that may span schedules; a node's words must be the page's. The
taxonomy calls `fund-balance/reserve-increase` "Reserve Increase / (Use)", after
p75's column header, while p66 prints ADDITION TO RESERVES. `funds.yaml` records
no words for a fund type, so the six fund-group labels come from the pp.66-67
column headers.

## constraint_tier

`constraint_tier` is `""` on every spine node: pp.66-67 publish fund groups,
whose columns hold funds of several tiers.

**Where a node carries one, the value is ours.** `data/funds.yaml`'s header:
`constraint_tier` and `restriction_note` are *"DERIVED — our reading of the
'Description of Funds' narrative (pp. 258-261) — and must not be presented as
something the city printed."* The fund node itself is printed, so
`node.derived` stays `false`; the disclosure travels on `source_note` (citing
`data/funds.yaml` and pp.258-261) and `rationale` (the fund's
`restriction_note`), and as a caveat in `metadata.caveats`.
`constraint-tier-vocabulary` fails a tier without both.

Every document that draws a fund node publishes its tier, through one
constructor: fund-flows, department-funding and the two transfer networks.

**`""` and `unknown` are different claims.** `""` means this document does not
classify the node. `unknown` means we read pp.258-261 and could not tell;
`data/funds.yaml` uses it for two funds.

## link.kind

`kind` is one of `schema/enums.schema.json`'s `link_kind`; the page's words for
each are `project.LinkKindLabel`'s. Only `external` is in the headline: an
Internal Service Fund charge is billed by one city department to another, so
counting it as revenue and as the payer's expenditure counts it twice.

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

$299,969,007 includes internal service charges — the scope is
`all-funds-gross` — so it is not `external_revenue_cents`.

**`naive_expenditure_cents` is deliberately wrong.** Summing the expenditure
column gives $313,708,146 where the city spends $254,095,412, a 23% inflation
from counting transfers twice. It ships so the page can show the error it avoids
and a check can assert the headline is not equal to it.

## derived

`derived: true` means we inferred it; `false` means the city printed it.

On the spine exactly two things are derived, both from one row. `CHANGE IN
WORKING CAPITAL` is printed once per column with a sign (`(1,034,154)` for the
General Fund). A Sankey cannot draw a negative link, so a negative change becomes
a `fund-balance/draw` → fund-group link and a positive one a fund-group →
`fund-balance/contribution` link. Both nodes carry a `rationale` and a
`source_note`.

`BEGINNING` and `ENDING WORKING CAPITAL` are facts but **stocks, not flows**, and
get no link; `metadata.caveats` says so, since a reader reconciling against p66
finds three rows unaccounted for.

## The transfer residual

Transfers out ($59,612,734) exceed transfers in ($21,525,997) by $38,086,737.

`transfer_id` is `""` on every spine link. Only `transfers-by-fund` and
`transfers-out` draw each end of a movement, and there the two legs share a
`transfer_id` derived from their shared `(doc_id, page, offset)`.
`transfers-out` is p76's legs and p222's transfers to the CIP, and the spine's
Transfers Out opens into it.

p76's grand total *is* the transfers-in side, to the cent: each destination
section equals the matching `TRANSFER IN:` cell on pp.66-67, and the General
Fund's out-flows equal `TRANSFER OUT:`.

**The difference is a column the city prints.** pp.72-75 are the same
sources-and-uses schedule carrying a `Transfers Out to CIP` heading, which
pp.66-67 fold into `TRANSFER OUT:` and omit from `TRANSFER IN:`:

```
p0073.txt:58   Transfers Out $21,525,997   Transfers Out to CIP $38,086,737
p0075.txt:58   Transfers Out $21,624,633   Transfers Out to CIP $50,762,251
```

$21,525,997 is p76's grand total and $38,086,737 is
`headline.transfer_residual_cents`, both to the cent: the residual is transfers to
capital projects, printed under their own heading.

| transfers out to CIP, FY2026 | | source |
|---|---|---|
| Capital Funds | 28,373,590 | `p0199.txt:13` |
| Enterprise Funds | 9,353,147 | `p0199.txt:14` |
| Special Revenue Funds | 320,000 | `p0199.txt:10` |
| Internal Service Funds | 40,000 | `p0073.txt:53`, `p0199.txt:17` |
| **total** | **38,086,737** | `p0073.txt:58`, `p0199.txt:21` |

pp.72-75 print to-CIP per major fund and one line for all non-major funds —
$28,693,590 in FY2026 (`p0073.txt:56`) — so they cannot split Capital from
Special Revenue. pp.198-209 print it per fund and per fund type: FY2026's summary
is p199 above; FY2027's is p205 (Capital 35,830,251, `p0205.txt:13`; Special
Revenue 100,000, `p0205.txt:10`; together p75's non-major 35,930,251,
`p0075.txt:56`).

Each row equals the group's `TRANSFER OUT:` less the transfers p76 shows that
group **paying**, attributed by the payer in each row label. The General Fund's
$10,037,797 is itemised in full and contributes zero. Capital pays $211,150 of
its $28,584,740: Traffic Impact Fee (510), County Measure D (550) and State − Gas
Tax (560) are `type: capital` in `data/funds.yaml`, and p76 lists all three as
payers.

`cuts-tie-along-the-lattice` holds `spine_TRANSFER_OUT == p76_paid + to_CIP` for
all six groups in both budget years through the split
`a-transfer-out-is-p76-or-to-the-cip`; p222 names each transferring fund, so the
sum splits Capital from Special Revenue as p199 and p205 do.

The residual is `headline.transfer_residual_cents`, not a node: a synthetic link
into `transfers/in` or out of `transfers/out` would unbalance that node, and
splitting the $59.6M into matched and unmatched portions would publish a division
the city never printed.

## The contested total

Budget Book p67's Internal Service Funds column prints Services & Supplies
`16,796,010` for FY2026-27, giving TOTAL EXPENDITURES `26,544,515`. Four other
pages print `26,294,515` — `p0183:64`, `p0075:53`, `p0205:17` and `p0209:20`,
whose five per-fund figures also sum to it — and `p0061:39` prints `26,906,515`,
that figure plus the `612,000` to-CIP transfer. Of the 48 `(fund group x object
category x budget year)` cells between pp.172-183 and pp.66-67, **47 agree to the
dollar** and this is the 48th. That grid is our arithmetic: pp.172-183 print
per-fund object rows and one Total per fund group, and no page prints a
group-by-object subtotal.

**We keep p67's figure.** The replacement Services & Supplies figure,
`16,546,010`, is printed on no page of the corpus — it is the sum of five
per-fund cells on p0183 — and a fact is one figure the city printed. Measured by
mutating the fact and running `fisc verify`:

| mutation | the fact check that goes red |
|---|---|
| `amount_cents` corrected, token left as p67 prints it | `fact-token-reparses` -- token `"16,796,010"` is $16,796,010.00 but the fact carries $16,546,010.00 |
| `amount_cents` and token both corrected | `fact-offset-points-at-token` -- p67 at offset 2548 is `"16,796,010"`, but the fact cites `"16,546,010"` |

Each route yields five findings: the fact check above,
`fund-group-sources-equal-uses` on the internal service column, and
`cuts-tie-along-the-lattice` three times, through the exceptions below.

`project.ContestedTotals()` declares the column, the spine's figure, what the
rest of the book makes it, and the bead; `sankey-2027.json` carries it as a
caveat and `sankey.json` does not, FY2026 tying everywhere. The declaration is
conditional on the graph drawing `26,544,515`, so correcting the fact retires it.

Three exceptions carry this $250,000 on different axes, so they are independent
witnesses. `p0067-internal-service-is-250000-high-by-fund-group` holds out
`(FY2027, adopted, internal-service)` against two figures pages print.
`-by-object` holds out `(FY2027, adopted, services-and-supplies)`, where every
figure is arithmetic and `SameResidualAs` grounds it in the first: pp.85-125
decompose the money by department, division and object with no fund dimension
and still put the difference in this category and year and none of the other
seven cells. `-by-fund` holds out the 48th cell itself, `(FY2027, adopted,
internal-service, services-and-supplies)`, where the `fund-expenditures` cut sums
pp.172-183's per-fund rows to the spine's grain. If the city reissues the page,
delete the exceptions; the cell then ties on its own (`fisc-av0w`).

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

These are the city's own `TOTAL SOURCES` and `TOTAL USES` rows, so the identity
is a cross-check rather than a restatement.

## Row-to-slug map

Which printed row of pp.66-67 is which `category` and `kind` is
`mappings/livermore-budget-fy2026-2027.yaml`'s, over `data/taxonomy.yaml`'s
slugs. `sign: contra` rows net into their parent category before links are
built; pp.66-67 print none — the contra rows are on p127 (ERAF, RPTTF).
