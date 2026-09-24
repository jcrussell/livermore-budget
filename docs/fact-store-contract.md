# The published fact store

`facts/facts.jsonl` is this project's audit trail: every figure it publishes,
content-addressed, carrying the document, page and byte offset it was read from.
This document is the contract for **shipping** it — what lands in `dist/`, how a
citation resolves to it, and what it does and does not promise.

The store itself is defined by `internal/fact`. This describes only its published
form, and adds nothing to it: every artifact below is the committed file
re-encoded or split, never recomputed.

## Where it goes

```
<output>/facts/<doc-id>/pages/pNNNN.jsonl   the records for one page
<output>/facts/facts.csv                    every record, one row each
<output>/facts/index.json                   what is where, and how big
```

**Top-level `facts/`, not under `data/`.** `data/` means one file per projection
— [`sankey-contract.md`](sankey-contract.md) promises it — and
`export.assetPath` refuses any asset landing there. The tree under `facts/`
deliberately mirrors `<output>/extracted/<doc-id>/pages/pNNNN.txt`, so the two
halves of one citation come from parallel paths and a reader who knows one layout
knows the other.

## Resolving a citation

A citation here is a **locator** — `(doc_id, page, offset)` — and never a fact
id. Given one, the records are at

```
facts/<doc_id>/pages/p<page padded to 4>.jsonl
```

**Computed, in one fetch, with no lookup.** `index.json` is not on this path; see
below.

**Why not the fact id.** `fact.MakeID` hashes `(doc_id, rule_id, row_path,
row_label, column_path, fiscal_year, basis)`. `rule_id` is in that tuple, so
revising or splitting a rule moves every id on the pages it covers — a citation
by id would 404 after a change that altered no figure. `(doc_id, page)` cannot
move: it is a fact about the city's document, not about this repository's
mappings.

**Append-stable.** A coverage lane adds shards; it does not renumber existing
ones. A browser cache stays warm across a release, and a link published today
resolves after the store grows.

### Who holds a locator

The URL is composed from two halves, so nothing has to parse a path to take it
apart:

| half | who states it | where the client reads it |
|---|---|---|
| `facts/<doc_id>/pages/` | `pkg/cmd/export`'s `shardBase` | `CONFIG.docs[<doc_id>].records_base` |
| `p<page padded to 4>.jsonl` | `pkg/cmd/export`'s `shardFile` | composed by `citations()` in `site/app.js` |

Both are `shardPath` split at exactly the point a client has to compose it, so
"the rule is spelled once" survives the split — and
`TestTheRecordsBaseComposesBackToTheShardPath` asserts the halves still make
the whole for every page published, rather than leaving it to the comment.

`internal/export` publishes `records_base` and **does not know how it is
built**. That is the same rule `PageIndexEntry.Data` follows and it is stated
in `buildProvenancePage`: the locator-to-URL rule belongs to whoever produced
the records.

`records_base` is always site-relative, unlike `page_text_base` beside it in
the same object. Shards are written into the output tree on every export; page
text is not, and goes absolute under `--source-browse-url`.

### What a chart cites

A projection's links publish `locators` — the `(doc_id, page)` pairs of the
facts each link sums — so a mark on the chart resolves to the records behind
it. This is a **page** locator: `offset` addresses one printed figure and a
link is an aggregate. See
[`sankey-contract.md`](sankey-contract.md#a-link-cites-its-facts-twice-and-the-two-citations-are-not-redundant)
for why a link publishes `fact_ids` as well and why neither replaces the other.

## The shards

One file per `(doc_id, page)`, JSONL, **byte-identical to the corresponding run
of `facts.jsonl`**:

```
LC_ALL=C bash -c 'cat dist/facts/*/pages/*.jsonl' | cmp - facts/facts.jsonl
```

That equality is the whole reconciliation — no record lost, none invented, none
rewritten — and it holds because of a property of the sort rather than care taken
in the packager. `fact.less` orders on `DocID`, then `Page`, then the rest, so
`(doc_id, page)` is a strict **prefix** of the total order and every page's
records are a contiguous run of the file. Splitting on that boundary cannot
reorder anything.

`fisc export` asserts it on every run, in `buildFactAssets`, not only in a test.
That is a deliberate exception to "the packager runs no checks", and the same one
`buildTrendsPage` already makes: a published artifact that silently disagrees
with the file it claims to be is the failure this project exists to refuse.

`LC_ALL=C` in the command above is not decoration. With more than one document
id, a UTF-8 locale's collation ignores punctuation on its first pass, so the
shell could glob in an order `fact.less` does not produce. **There are now two
documents and 36 pages, and the orders still coincide** — `livermore-acfr-*`
sorts before `livermore-budget-*` under C and under en_US.UTF-8 alike, and both
forms of the command reconcile today. So this is a latent hazard rather than an
active one, and the reason to keep the variable is the third document.

NOTE THE `bash -c`, WHICH THIS COMMAND DID NOT HAVE UNTIL 2026-08-30 AND NEEDED
FROM THE START. `LC_ALL=C cat ...` sets the variable for `cat`, which does not
sort anything; the **shell** expands the glob, under its own locale, before
`cat` is executed. The variable was decorating the wrong process. It went
unnoticed because the store held one document, which is exactly the condition
under which the sentence above says the ordering cannot matter.

## The CSV

`facts/facts.csv` is the store in a second encoding — not a report. Its header is
every `fact.Fact` JSON key, **in declaration order**, taken by reflection from
the struct. Every row is one record.

**Amounts are integer cents.** There is no `amount_dollars` column: it would be
either a float, which [`AGENTS.md`](../AGENTS.md) forbids, or a second decimal
spelling of one integer — a second answer to what the amount is, and the one a
spreadsheet silently reformats. Divide by 100.

The CSV is produced by **transcoding** the JSONL, value for value, with
`json.Decoder.Token()` and `UseNumber`. Each number is written as the literal
text the encoder wrote, so no `float64` is constructed anywhere on the money
path — the rule is structural rather than promised. Token streaming rather than
decoding to a map is also what preserves the key order: decoding a JSON object
into a Go map loses it.

Quoting is RFC 4180 via `encoding/csv`, LF line endings. 1,110 of the store's
fields need quoting today — every grouped number in `token` carries a comma — so
this is not a file a `Sprintf` join would produce correctly.

## The index

`facts/index.json` carries the store's totals and, per page, the record count,
byte size, SHA-256, the rules that read it, the fiscal years they read, and the
path to its records.

**It does not name the page's extracted text**, though that is the obvious
companion field. `fisc export` chooses between shipping the text under
`extracted/` and citing a remote browsable copy (`--source-browse-url`), and it
makes that choice after this index is built — so the packager cannot state the
path, and an index that guessed would be wrong on every `--source-browse-url`
deploy. The provenance page's own rows carry the correct link, because they are
composed where the decision lives. A machine consumer can compose it from the
`doc_id` and `page` plus whichever base the deploy uses.

**It is not on the resolution path.** A locator resolves by computing the shard
path. The index exists to build the provenance page, to enumerate the store
without a directory listing a static host may not serve, and to check a download
against a hash. Nothing breaks if it is absent, and a test asserts every path it
publishes equals what the path rule computes — so the two cannot drift into the
index becoming load-bearing.

## What this does not promise

**The store is what the mappings cover, not what the documents contain.** 36
pages of 786 are mapped, across two of the three registered documents. A figure absent from the store is a page nobody has
mapped yet, not a figure the city did not print — the store is not a claim about
the corpus.

**A shard is not a schedule.** It holds the records read from one printed page,
which may be part of a schedule spanning several pages, and may hold records from
more than one rule. `index.json` names the rules per page.

**Sums are not published, and summing shards is not safe in general.** The same
money appears at more than one grain across scopes: `revenue-by-fund` and
`transfers-by-fund` overlap by $21,045,597 in FY2026. Facts carry a `scope`, and
figures may only be added within one. `fisc verify` is what asserts the
relationships between scopes; this file publishes the inputs.

**`fisc export --output .` would put `dist/facts/` beside the repository's real
`facts/`.** The whole site would land in the tree in that case, so it is not a
new hazard, but the collision of names is worth knowing before you try it.

## What checks it

- `buildFactAssets` — the `cat`-equality, on every export, plus a refusal for an
  unsorted store and for a page whose records are not contiguous.
- `TestFactShardsReconstituteTheCommittedStore` — the same equality over the
  committed file.
- `export.Options.validate` — every published locator names a file that is being
  written, at the size the page prints.
- `TestTheSiteLinksEveryShardItShips` — nothing ships that no page links, which
  is `unviewedDocuments`' standard applied to assets.
- `TestEveryShardedPageHasItsExtractedText` and the citation seeding in
  `buildSite` — every published locator's page text is shipped, so both halves of
  a citation resolve.
- `link-locators-match-their-facts` (`fisc verify`, tier 1) — every link's
  `locators` are exactly the pages its `fact_ids` were read from. Nothing else
  compares the two citations a link publishes, and a locator naming the wrong
  page sends a reader to a well-formed shard that does not hold their figure.
- `TestTheRecordsBaseComposesBackToTheShardPath` and
  `TestEveryShippedDocumentCanResolveItsRecords` — the base a client composes
  with rebuilds the path the shard was written at, and every document
  publishing records has one. The second is the fail-closed arm for a field
  whose absence is legal: a caller publishing no records is not a caller
  publishing a base pointing nowhere.
- the client's tests — the fold unions locators with de-duplication, and a
  document missing `links[].locators` is refused before the page repaints.
