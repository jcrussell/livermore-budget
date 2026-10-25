# The published fact store

`facts/facts.jsonl` is the audit trail: every figure published, content-addressed,
carrying the document, page and byte offset it was read from. `internal/fact`
defines the store; this page is its published form, which is the committed file
re-encoded or split, never recomputed.

## Where it goes

```
<output>/facts/<doc-id>/pages/pNNNN.jsonl   the records for one page
<output>/facts/facts.csv                    every record, one row each
<output>/facts/index.json                   what is where, and how big
```

Top-level `facts/`, not under `data/`, which holds one file per projection
(`export.assetPath` refuses an asset there). The tree mirrors
`<output>/extracted/<doc-id>/pages/pNNNN.txt`, so the two halves of a citation
come from parallel paths.

## Resolving a citation

A citation is a **locator** — `(doc_id, page, offset)` — never a fact id. The
records are at

```
facts/<doc_id>/pages/p<page padded to 4>.jsonl
```

computed, in one fetch, with no lookup. `fact.MakeID` hashes `rule_id`, so a
rule revision moves every id on its pages; `(doc_id, page)` is a fact about the
city's document and cannot move. A coverage lane adds shards and renumbers none.

The client reads the link at `CONFIG.docs[<doc_id>].pages[<page>].records`, Go
composing it from `pkg/cmd/export`'s `shardBase` (passed as
`Options.RecordsBase`) and `internal/export`'s `RecordsFile`;
`TestTheRecordsBaseComposesBackToTheShardPath` holds that the halves make the
whole for every page published. A `records` link is always site-relative; the
page's `text` link goes absolute under `--source-browse-url`.

A projection's links publish `locators`, page locators of the facts each link
sums ([`sankey-contract.md`](sankey-contract.md#a-link-cites-its-facts-twice)).

## The shards

One file per `(doc_id, page)`, **byte-identical to the corresponding run of
`facts.jsonl`**:

```
LC_ALL=C bash -c 'cat dist/facts/*/pages/*.jsonl' | cmp - facts/facts.jsonl
```

It holds because `fact.less` orders on `DocID`, then `Page`, then the rest, so
every page's records are a contiguous run of the file. `fisc export` asserts it
on every run, in `buildFactAssets`.

`LC_ALL=C` must reach the shell that expands the glob, hence `bash -c`: a UTF-8
locale's collation ignores punctuation on its first pass. With the two documents
the store holds, `livermore-acfr-*` sorts before `livermore-budget-*` under C and
en_US.UTF-8 alike, so the hazard waits for a third document.

## The CSV

`facts/facts.csv` is the store in a second encoding. Its header is every
`fact.Fact` JSON key in declaration order, by reflection; every row is one
record. Amounts are integer cents, with no `amount_dollars` column — divide by
100. It is transcoded from the JSONL with `json.Decoder.Token()` and `UseNumber`,
so each number is the literal text the encoder wrote and no `float64` exists on
the path. RFC 4180 via `encoding/csv`, LF line endings; every grouped number in
`token` carries a comma.

## The index

`facts/index.json` carries the store's totals and, per page, the record count,
byte size, SHA-256, the rules that read it, the fiscal years they read, and the
path to its records. It does not name the page's extracted text: `fisc export`
chooses between shipping it and `--source-browse-url` after the index is built.
It is not on the resolution path; a test holds every path it publishes to the
path rule.

## What this does not promise

- **The store is what the mappings cover**, not what the documents contain. A
  figure absent from it is a page nobody has mapped.
- **A shard is not a schedule.** It holds one printed page's records, possibly
  from several rules; `index.json` names them.
- **Summing shards is not safe.** The same money appears at more than one grain
  across scopes: `revenue-by-fund` and `transfers-by-fund` overlap by $21,045,597
  in FY2026. Add figures within one `scope`.
- `fisc export --output .` is refused, as is any non-empty directory no earlier
  export marked, so the site's `facts/` and the repository's cannot share a tree.

## What checks it

- `buildFactAssets` — the `cat`-equality on every export, and a refusal for an
  unsorted store or a page whose records are not contiguous.
- `TestFactShardsReconstituteTheCommittedStore` — the same equality over the
  committed file.
- `export.Options.validate` — every published locator names a file being written.
- `TestTheSiteLinksEveryShardItShips` — nothing ships that no page links.
- `TestEveryShardedPageHasItsExtractedText` — every published locator's page text
  ships.
- `link-locators-match-their-facts` — every link's `locators` are exactly the
  pages its `fact_ids` were read from.
- `TestEveryShippedDocumentCanResolveItsRecords` — every document publishing
  records has a base.
- the client's tests — the fold unions locators with de-duplication, and a
  document missing `links[].locators` is refused.
