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

`<output>/fy<year>-<basis>.json`, one document per published column, so
`fisc export -o dist` writes `dist/fy2026-adopted.json` and `fisc export -o site`
writes `site/fy2026-adopted.json`. One code path, one layout.

A projection stating a fiscal year and a basis is **a schedule inside its
column**, keyed by the stem with its year suffix stripped, and is not published
under a name of its own. One that states neither — `revenue-trends` and the two
balance documents carry a series and no column — ships at
`<output>/data/<stem>.json` as itself.

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

`schema/projection.schema.json` holds it, and `internal/project`'s encoder
validates every document against it before returning the bytes. What follows is
what the schema cannot say.

Every key is present on every object, in declaration order. **No `omitempty`,
no `null`** — the same discipline as `fact.Fact`, and for the same reason: a
key that vanishes when it is empty makes a diff between two releases read as a
structural change. Absent strings are `""`; the client writes `node.parent || null`
if it wants nullish semantics. The one key a document may omit is `headline`,
which the spine alone carries: eight zeros in its place on a document with no
total to name would be absent-is-not-zero at document level.

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
one anchor. An anchor collision fails silently — the page renders, the anchor
resolves, and the reader is shown a sentence about something else — which is why
it is refused rather than left to a reader to notice.

Note what that arm can and cannot reach: stems are map keys, so **two different
documents cannot collide on one**. It catches a repeated id inside a document,
and a stem/id pair whose `--` composes ambiguously (`a--b` + `c` against `a` +
`b--c`), because the separator is not an escape. Neither shape occurs today.

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

Determinism: nodes sorted by `(tier, id)`, links by `(source, target, kind)`,
`fact_ids` ascending, `locators` by `doc_id` with `pages` ascending inside each
and every page once. Two builds of the same facts are byte-identical.

**A pair may carry one ribbon per kind**: a revenue row reaching the five
Internal Service Funds takes its money as an internal service charge and the rest
of the city's as external revenue, and one ribbon for both would publish the
first as money crossing the city's boundary. Two links of the SAME kind on one
pair is a cell key that lost an axis and is refused.

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

### `partition`

**`partition: true` means the ribbon divides one printed table along a second
axis rather than following money the schedule prints as moving that way.** The
departmentwide cross-tab is the only document that sets it: Budget Book
pp.85-125 print one matrix, divisions down and object categories across, and a
chart can read it either way round without either reading being money moving.

It is **the projection's flag and not the client's**: nothing in a graph
distinguishes a cross-tab from a chain by looking. `site/app.js` carries the
words in one constant, `PARTITION_NOTE`.

`node-tiers-are-declared` reads it as the **second** exception to "a link runs
from a coarser tier to a finer one", beside a revenue line rolled up into its own
category. A partition link may descend — the cross-tab's run `expenditure/<object>`
(tier 5) into `dept/<division>` (tier 4) — and a descending link that is neither
a rollup nor a declared partition is still refused by name.

`schema_version` stays at 1, the project being pre-release, and the key is
present and `false` on every link of every document.

## Tiers and node ids

| tier | meaning | id form |
|---|---|---|
| 0 | revenue source | `revenue/<slug>` |
| 1 | revenue line — one printed row of a category | `revenue-line/<slug>` |
| 2 | fund group | `fund-group/<type>` |
| 2 | the paying end of one printed transfer | `transfer-from/<number>` |
| 3 | fund | `fund/<number>` |
| 4 | division — one of `data/departments.yaml`'s mixed-case row groups | `dept/<slug>` |
| 4 | department — one of its ALL-CAPS headings | `department/<slug>` |
| 5 | object category | `expenditure/<slug>` |
| 5 | the receiving end of one printed transfer | `transfer-to/<number>` |

Plus the flow endpoints that are not part of that hierarchy: `transfers/in`
(tier 0), `transfers/out` (tier 5), `fund-balance/reserve-increase` (tier 5),
`fund-balance/draw` (tier 0), `fund-balance/contribution` (tier 5).

**`department/` and `dept/` are two id forms at ONE tier, and they have to be.**
`data/departments.yaml` keeps two namespaces because the pages do: pp.167-170
print the General Fund by DIVISION and pp.85-125's funding schedule prints one
block per DEPARTMENT. Five slugs are in both populations — `city-council`,
`city-manager`, `city-attorney`, `general-services`, `administrative-services` —
each naming a department and the sole division beneath it, and the city prints
no other name for either. An id form is read by cutting at the FIRST slash, so
one prefix over both would make `dept/city-council` mean the department in one
document and the division in another.

**`transfer-from/` and `transfer-to/` are ENDS of a movement and not the funds
themselves**, which is why they are id forms rather than a second use of
`fund/<number>`. Budget Book p76 prints money moving between the city's own
funds, so the natural link is `fund/<a>` to `fund/<b>` — tier 3 to tier 3, which
the ordering rule refuses and which `d3-sankey` cannot lay out, both ends taking
the same column index. A rollup would claim the payer folds into the receiver,
and `partition: true` would claim p76 is one table read along a second axis when
it is money moving.

They take the spine's own tiers for the same end of the chart — a payer's end at
2 with the fund groups, a receiver's at 5 with the object categories — so a link
runs tier 2 to tier 3, coarse to fine, and needs no exception anywhere. Only
`transfers-by-fund` and `transfers-out` publish them.

**A flow endpoint is a container in the two transfer networks.** `transfers/in`
carries no link in `transfers-by-fund` and every `transfer-from/` node is
parented to it, so there it is a fold root rather than an end; `transfers/out`
is the same in `transfers-out`, holding every `transfer-to/` node at its own
tier 5. `node-hierarchy-well-formed` allows that exactly where the endpoint draws
no flow of its own, and a container alone may hold a node at its own tier: on
the spine the same ids are the ends of the city's transfers, and a node parented
to one there is still refused.

**Tier 1 is a printed row, not a constraint layer.** A constraint tier is a
property of a **fund**, and the fund groups do not partition along it. Counting `data/funds.yaml` by `type` x `constraint_tier`, `capital`
holds 3 committed funds and 43 restricted-by-law; `special-revenue` holds 37
restricted-by-law, 2 unknown and 1 committed. So `fund-group/<type>.parent =
constraint/<tier>` has no single answer, and a layer whose parent edge is
undefined is not a layer. The constraint tier rides as the `constraint_tier`
**field** on a tier-3 node instead — see below.

A `revenue-line/` node's parent edge **is** defined: the category the row is
printed under, one string, on every line.

**`revenue-line/` is a prefix of its own and not a deeper `revenue/` id.** An id
form is read by cutting at the **first** slash, and a category slug may itself
carry one: `revenue/taxes/property` is a tier-0 category. A line nested under
`revenue/` would be indistinguishable from its own parent's form.

**The documents that use the other tiers are elsewhere.** The drill-down
(`docs/general-fund-drilldown-contract.md`) publishes tiers 0, 1, 2, 3, 4 and 5
over Budget Book pp.127-140 and pp.167-170, and states its own counts there.

**How many nodes a tier holds is a property of the DOCUMENT, not of the
hierarchy.** A second document at another scope draws a different set: the
citywide spine (pp.66-67) prints six fund groups, while revenue-by-fund
(pp.131-140) prints seven, carrying Permanent Funds that pp.66-67 give no column
at all (fisc-u8o). There is no tier count that holds across all documents, so
each states its own below rather than inheriting the spine's.

**On the spine** (`sankey.json`): tier 0 = 12 nodes, tier 2 = 6, tier 5 = 7,
which is the whole of its 25. Tiers 3 and 4 are empty, because pp.66-67 publish
neither a fund nor a department axis.

**All three of the spine's drawn columns open, and every rung but one is a
WINDOW** — the node the reader clicked in the middle, with three columns the
narrowest such window; what each rung draws at what width is stated in
`docs/general-fund-drilldown-contract.md`. The steps form a tree, not a chain:
the six tier-2 fund groups
open into `fund-flows` for the same fiscal year, keeping this document's own
revenue categories on the left and drawing the group's funds on the right; the
ten tier-0 revenue categories open into a window of the same document — the
lines it prints under the category, the category itself, and the spine's own
fund groups for it; and the four tier-5 `expenditure/` nodes open into
`department-spending` — the fund groups that fund the category, the category
itself, and the divisions that spend it. From an opened fund group the General
Fund opens into its divisions and a division into its object categories, three
rungs deep.

**`transfers/in` opens too, and it is the one rung that is not a window.** It is
a flow END rather than a container on the spine, so it has no parts to put a
window around; the step keeps no flank and draws `transfers-by-fund`'s two
columns alone — the funds that pay each transfer on the left, the funds that
receive them on the right; the only flank it could keep is tier 2, which its own
left-hand column already draws. It is also the only rung that opens a SOURCE, and
`DrillStep.Side` says so rather than letting a client infer it from the tier
numbers.

The remaining tier-0 node and the three tier-5 nodes that are flow ENDS rather
than containers do not open, and the `Role` on each step is what closes them. The
sixty funds pp.167-170 do not decompose are closed to the DIVISION step by a
`Role` in the same way and opened by a step of their own into pp.85-125's
departments; which of them that step can open is read off each year's document
by the client (`decomposable` in `site/app.js`), stated in
`docs/general-fund-drilldown-contract.md`. Depth 0 is this document drawn whole.

Those counts are per TIER and include the flow endpoints, which is why they are
larger than the id-form counts a reader might tally from the table above: tier 0
is 10 `revenue/` nodes plus `transfers/in` and `fund-balance/draw`, and tier 5 is
4 `expenditure/` nodes plus `transfers/out`, `fund-balance/reserve-increase` and
`fund-balance/contribution`.

`counts.facts` is the filtered input count — every fact matching the fiscal
year, basis and scope. `counts.facts_cited` is how many of those a link
actually carries, and `counts.facts_uncited` the rest: on this document exactly
the zero-valued cells (a dash is a printed fact but earns no link) plus the two
stock rows, so it is a quantity `uncited-facts-are-printed-zeros` asserts rather
than a discrepancy a reader has to explain away. For FY2026: 120 = 58 cited +
62 uncited, the 62 being 50 zero cells and 12 stock rows. `facts_cited_twice`
is 0: no fact here is behind two links.

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

**On the spine, tier 1 is empty**, and that is the schedule rather than an
omission: pp.66-67 print each revenue category as one row and no detail beneath
it. The rows are printed on pp.127-140, which is a different schedule and a
different document.

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

**Every document that draws a fund node publishes its constraint tier**, through
one constructor in `internal/project`: fund-flows, department-funding and the
two transfer networks, in every column each publishes. The tier is a property
of the fund and not of the schedule, so a column's node table states it once;
measured on `testdata/fy2026-adopted.column.json`, 86 of its 430 nodes carry
one (71 `restricted-by-law`, 12 `committed`, 2 `unknown`, 1 `discretionary`),
and 83 of 422 on FY2027's. `sankey` and `sankey-2027` publish none: the spine
classifies no node, which is `""`'s meaning below. Read *non-empty* rather than
*present*: there is no `omitempty` here, so every node in every document
carries the key.

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

`transfer_id` is `""` on every spine link. Only `transfers-by-fund` and
`transfers-out` draw each end of a movement as its own link, and there the two
legs share a `transfer_id` derived from their shared `(doc_id, page, offset)`.
`transfers-out` is p76's legs and p222's transfers to the CIP, the spine's
TRANSFER OUT by fund group, and the spine's Transfers Out opens into it.

p76's own grand total *is* the transfers-in side, to the cent: each of its
destination sections equals the matching `TRANSFER IN:` cell on pp.66-67, and
the General Fund's out-flows equal `TRANSFER OUT:`.

**The difference is a column the city prints.** pp.72-75 are the same
sources-and-uses schedule carrying a `Transfers Out to CIP` heading, which
pp.66-67 fold into `TRANSFER OUT:` and omit from `TRANSFER IN:` entirely:

```
p0073.txt:58   Transfers Out $21,525,997   Transfers Out to CIP $38,086,737
p0075.txt:58   Transfers Out $21,624,633   Transfers Out to CIP $50,762,251
```

$21,525,997 is p76's grand total to the cent, and $38,086,737 is
`headline.transfer_residual_cents` to the cent. The residual is therefore not a
discrepancy at all — it is transfers to capital projects, printed under their
own heading.

| transfers out to CIP, FY2026 | | source |
|---|---|---|
| Capital Funds | 28,373,590 | `p0199.txt:13` |
| Enterprise Funds | 9,353,147 | `p0199.txt:14` |
| Special Revenue Funds | 320,000 | `p0199.txt:10` |
| Internal Service Funds | 40,000 | `p0073.txt:53`, `p0199.txt:17` |
| **total** | **38,086,737** | `p0073.txt:58`, `p0199.txt:21` |

**pp.72-75 print to-CIP per major fund and one aggregate line for all non-major
funds** — $28,693,590 in FY2026 (`p0073.txt:56`) — so they cannot split Capital
from Special Revenue. pp.198-209 print the column per fund and per fund type:
FY2026's summary is p199 above, FY2027's is p205 (Capital 35,830,251,
`p0205.txt:13`; Special Revenue 100,000, `p0205.txt:10`; together p75's
non-major 35,930,251, `p0075.txt:56`).

Each row also equals the group's `TRANSFER OUT:` less the transfers p76 shows
that group **paying** — attributed by the payer named in each row label, not by
`out − in` per group. The General Fund's $10,037,797 is itemised in full and
contributes zero. Capital pays $211,150 of its $28,584,740: Traffic Impact Fee
(510), County Measure D (550) and State − Gas Tax (560) are `type: capital` in
`data/funds.yaml`, and p76 lists all three as payers.

`TestP76SourcesDecomposeTheResidualByFundType` resolves every payer through
`registry.FundByLabel` and asserts `spine_TRANSFER_OUT == p76_paid + to_CIP` for
all six groups in both budget years; `cuts-tie-along-the-lattice` makes the
same claim over the published facts through the split
`a-transfer-out-is-p76-or-to-the-cip`, which sums p76's transfers out and
p222's transfers to the CIP by fund group and holds them to the spine. p222
names each transferring fund, so the sum splits Capital from Special Revenue
exactly as p199 and p205 do.

`fisc-1wr.4` asks for a residual node. This contract states the residual as
`headline.transfer_residual_cents` instead, and has `verify` assert it equals
out minus in. A synthetic link into `transfers/in` or out of `transfers/out`
would unbalance that node, and splitting the $59.6M into matched and unmatched
portions would publish a division the city never printed.

## The contested total

The spine publishes one figure the same book contradicts four pages over, and it
keeps publishing it deliberately. Budget Book p67's Internal Service Funds column
prints Services & Supplies `16,796,010` for FY2026-27, giving TOTAL EXPENDITURES
`26,544,515`. Four other pages print `26,294,515` -- `p0183:64`, `p0075:53`,
`p0205:17` and `p0209:20`, the last of which corroborates twice because its five
per-fund figures also sum to it -- and `p0061:39` implies it, printing
`26,906,515`, which is that figure plus the `612,000` to-CIP transfer. Of the 48
`(fund group x object category x budget year)` cells between pp.172-183 and
pp.66-67, **47 agree to the dollar** and this is the 48th -- with the same
qualifier the `p0067-internal-service-is-250000-high-by-fund-group` exception
attaches to that figure wherever it prints it, and which matters more here than anywhere because this section is the
published-is-not-derived argument: **that 48-cell grid is our arithmetic and not
the city's.** No page prints a group-by-object subtotal. What pp.172-183 print
are the per-fund object rows and one Total per fund group; the 48 cells are our
sums of those rows, and only the Totals are printed figures.

**We keep p67's figure, and the reason is not deference to the spine.** The
figure that would replace the Services & Supplies row, `16,546,010`, is printed
on **no page of the corpus** -- grep all 786 extracted pages and it does not
occur. It is the sum of five per-fund cells on p0183. Publishing it would put a
value we computed into `facts/facts.jsonl`, which is the one thing the fact store
is defined not to hold: a fact is one figure the city printed, at the page and
offset cited.

The store enforces that rather than trusting it, and the two routes fail on two
different checks. Measured by mutating the fact and running `fisc verify`:

| mutation | the fact check that goes red |
|---|---|
| `amount_cents` corrected, token left as p67 prints it | `fact-token-reparses` -- token `"16,796,010"` is $16,796,010.00 but the fact carries $16,546,010.00 |
| `amount_cents` and token both corrected | `fact-offset-points-at-token` -- p67 at offset 2548 is `"16,796,010"`, but the fact cites `"16,546,010"` |

Each route yields five findings, not one: the fact check above,
`fund-group-sources-equal-uses` on the internal service column, and
`cuts-tie-along-the-lattice` three times, whose
`p0067-internal-service-is-250000-high-by-fund-group`, `-by-fund` and
`-by-object` exceptions pin the spine's present figure and go red the moment it
moves. The table names only the fact check because that is the arm a synthetic
figure cannot get past -- the lattice arms would fall silent again if the
exceptions were re-pointed, and the fact checks would not.

So there is no edit to this fact that keeps its p67 citation, and re-citing it to
p0183 would make the spine no longer a read of pp.66-67.

**What the reader gets instead is disclosure.** `project.ContestedTotals()` declares
the column, the spine's figure, what the rest of the book makes it, and the bead;
`sankey-2027.json` carries it as a caveat and `sankey.json` does not, FY2026
tying everywhere. The declaration is *conditional on the graph actually drawing
`26,544,515`*, so correcting the fact retires the sentence with nobody having to
remember it.

Three exceptions carry this $250,000, and **not for the same cell** -- they
reach it on different axes, which is what makes them independent witnesses rather
than copies of one claim. `p0067-internal-service-is-250000-high-by-fund-group`
holds out `(FY2027, adopted, internal-service)`, a FUND GROUP, against two
figures both of which pages print. `p0067-internal-service-is-250000-high-by-object`
holds out `(FY2027, adopted, services-and-supplies)`, an OBJECT CATEGORY, where
its own `Printed` is careful to say neither figure is printed anywhere and all
three are arithmetic; `SameResidualAs` is what grounds the second in the first. pp.85-125 decompose the money by department, division and object with
no fund dimension at all, and still put the difference in this category and this
year and in none of the other seven cells.
`p0067-internal-service-is-250000-high-by-fund` holds out the 48th cell itself,
`(FY2027, adopted, internal-service, services-and-supplies)`, where the
`fund-expenditures` cut sums pp.172-183's per-fund rows to the spine's grain; it
too is arithmetic on one side and grounded in the fund-group entry.

The failure message `internal/structure`'s exception machinery prints already
says what to do if the city reissues the page: *delete the exception rather than
re-pointing it: the cell then ties on its own*. `fisc-av0w`.

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
