# The published fact store

`facts/facts.jsonl` is the audit trail: every figure the site publishes, with
the document, page and byte offset it was read from. The site republishes it
under `facts/`, re-encoded or split and never recomputed. A record's shape is
`schema/fact.schema.json`; how the export builds and checks these files is in
`go doc ./pkg/cmd/export`.

## Files

| path | what it holds |
|---|---|
| `facts/<doc_id>/pages/pNNNN.jsonl` | the records read from one printed page |
| `facts/facts.csv` | every record, one row each |
| `facts/index.json` | per page: record count, bytes, SHA-256, the rules and fiscal years that read it, and its path |

The shards mirror `extracted/<doc_id>/pages/pNNNN.txt`, the page text a record's
offset points into, so both halves of a citation sit at parallel paths.

## Resolving a citation

A citation is a locator, `(doc_id, page, offset)`, never a fact id. A fact id
hashes the rule that read it, so it moves when a rule is revised; the page it
was printed on does not. The records for a locator are at
`facts/<doc_id>/pages/p<page, zero-padded to 4>.jsonl`, computed without a
lookup. `index.json` is for listing, not for resolution.

Concatenated in path order, the shards are `facts.jsonl` byte for byte:

```bash
LC_ALL=C bash -c 'cat dist/facts/*/pages/*.jsonl' | cmp - facts/facts.jsonl
```

`LC_ALL=C` has to reach the shell that expands the glob. A UTF-8 locale's
collation ignores punctuation on its first pass; with today's document ids the
order happens to agree, and a third document could change that.

## The CSV

The header is every record key in the JSONL's order. Amounts are integer cents
(divide by 100); there is no dollars column. RFC 4180, LF line endings; a
grouped number in `token` carries its comma, so it is quoted.

## What not to do with it

- **Do not read it as the documents' contents.** It holds what the mappings
  cover; a figure absent from it is on a page nobody has mapped.
- **Do not treat a shard as a schedule.** It is one printed page's records,
  possibly from several rules; `index.json` names them.
- **Do not sum across scopes.** The same money appears at more than one grain:
  `revenue-by-fund` and `transfers-by-fund` overlap by $21,045,597 in FY2026.
  Add figures within one `scope`.
