# fisc — the Livermore budget, with its sources attached

`fisc` turns the City of Livermore's published budget PDFs into a verified fact
store, and that fact store into a static site whose headline view is a Sankey of
where city money comes from and where it goes.

The distinguishing constraint: **every figure published carries a provenance
pointer back to a page and cell of a source PDF**, and `fisc verify` fails if any
link in that chain breaks. That is the feature. A number that cannot be traced,
a check that cannot fail, or an inferred value presented as a published one is a
defect here regardless of how good the chart looks.

## What is covered today

**Two pages of 786.** This is a proof of concept, and saying so plainly is part
of the point.

| document | pages | extracted | mapped |
|---|---:|---:|---:|
| FY 2025-2027 Budget Book | 268 | all | **2** (pp. 66–67) |
| 2025-2030 Capital Improvement Plan | 323 | all | 0 |
| FY 2024-25 Annual Comprehensive Financial Report | 195 | all | 0 |

Those two pages are the citywide spine — the schedule titled "CITYWIDE REVENUES,
EXPENDITURES, AND FUND BALANCE/WORKING CAPITAL" — which yields **240 facts** and
an all-funds picture of the adopted FY2026 and FY2027 budgets:

- $299,969,007 gross revenue, $254,095,412 gross expenditure (FY2026)
- six fund groups, whose printed `TOTAL SOURCES` and `TOTAL USES` the chart
  reconciles against exactly

The site also publishes **$313,708,146** — the wrong answer — beside the right
one, because naively summing the expenditure column double-counts transfers by
23%, and showing the error you are avoiding is more useful than quietly avoiding
it.

Everything below the fund-group level (departments, revenue line items,
individual funds, the CIP, the ACFR) is **not yet mapped**. See `bd ready`.

## Build and look at it

Requires Go. Neither the PDFs nor Python are needed to build or verify — those
are for re-extraction only.

```bash
make build                          # build bin/fisc
make test                           # tests, always with -race
make site                           # build the static site into dist/
python3 -m http.server -d dist 8000 # then open http://localhost:8000
```

The page fetches `data/sankey.json` rather than inlining it, so it needs a
server; a provenance file you cannot `curl` on its own is not much of an audit
trail.

```bash
./bin/fisc build      # mappings/ + data/extracted/ -> facts/facts.jsonl
./bin/fisc verify     # 26 checks over the fact store and the graph
./bin/fisc verify --full   # also re-hashes the source PDFs (needs git lfs pull)
./bin/fisc export     # facts/facts.jsonl -> dist/
```

`fisc export` never re-runs the mapping engine — it reads the committed facts —
so building the site cannot change a published figure.

To re-extract from the PDFs you also need `poppler-utils` and `git lfs pull`:

```bash
make extract          # or: make extract DOC=livermore-budget-fy2026-2027
```

## How it fits together

```
data/pdf/*.pdf                     source documents, LFS, sha256 in sources.yaml
  │  tools/extract.py (poppler)    deterministic; NOT part of the Go binary
  ▼
data/extracted/<doc>/              786 pages of -layout text + -bbox geometry,
                                   each hashed in a manifest
  │  mappings/*.yaml               the judgment layer: which rows, which columns,
  │                                what they mean. Written to be read.
  ▼
facts/facts.jsonl                  240 content-addressed facts, each carrying
                                   doc_id / page / offset / token
  │  internal/project              one projection per (fiscal_year, basis, scope)
  ▼
dist/                              static site: d3-sankey, no bundler, no build step
```

Four invariants hold throughout, and most of the code exists to enforce them:

- **Amounts are integer cents.** Never float. These PDFs corrupt figures into
  *plausible wrong values* rather than errors, so `internal/amount` recognises a
  closed set of shapes and treats everything else as an error.
- **Absent is not zero.** A printed `-` is a zero the city published; a blank
  cell means the line does not apply.
- **Fail closed on ambiguity.** A rule that no longer resolves is an error, not a
  guess.
- **Published is not derived.** Anything we inferred carries `derived: true` with
  a rationale, and `fisc verify` fails on one that does not.

## Sources

Every URL and retrieval date is asserted in exactly one place,
[`data/sources.yaml`](data/sources.yaml). All three documents were re-downloaded
and compared byte-for-byte against the committed copies; `fisc verify --full`
re-runs that check.

- [FY 2025-2027 Budget Book](https://www.livermoreca.gov/home/showpublisheddocument/12813)
- [2025-2030 Capital Improvement Plan](https://www.livermoreca.gov/home/showpublisheddocument/12793)
- [FY 2024-25 Annual Comprehensive Financial Report](https://www.livermoreca.gov/home/showpublisheddocument/13545)

This is an independent project and is not affiliated with or endorsed by the
City of Livermore.

## Working on it

Issues live in [beads](https://github.com/gastownhall/beads), not in this file —
run `bd ready` for available work and `bd prime` for the workflow. Agent-facing
guidance is in [`docs/agents/`](docs/agents/); the `sankey.json` contract is
[`docs/sankey-contract.md`](docs/sankey-contract.md).

Two things a newcomer should know. A git remote is configured but **nothing has
been pushed to it**, so CI has never run and the site has never deployed —
`dist/` is a local artifact. The Pages workflow
([`.github/workflows/pages.yml`](.github/workflows/pages.yml)) is in place and
publishes on push to `main`, once Pages is enabled for the repository (Settings
→ Pages → Source → *GitHub Actions*). And the extracted page text is cited by a
GitHub URL that does not resolve until that first push lands; that is
`fisc-ze7`.
