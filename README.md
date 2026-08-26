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

**Twenty-one pages of 786.** This is still a proof of concept, and saying so
plainly is part of the point.

| document | pages | extracted | mapped |
|---|---:|---:|---:|
| FY 2025-2027 Budget Book | 268 | all | **21** (pp. 66–67, 76, 127–140, 167–170) |
| 2025-2030 Capital Improvement Plan | 323 | all | 0 |
| FY 2024-25 Annual Comprehensive Financial Report | 195 | all | 0 |

Those pages yield **1,448 facts** across four schedules, each of which reconciles
against something the city itself printed:

| schedule | facts | what it is | fiscal years |
|---|---:|---|---|
| pp. 66–67 | 240 | the citywide spine, all funds gross | 2026, 2027 adopted |
| pp. 127–140 | 924 | revenue by fund and line item | 2024 actual, 2025 revised, 2026 + 2027 adopted |
| pp. 167–170 | 196 | General Fund department × object category | the same four |
| p. 76 | 88 | the transfer schedule, both legs of every transfer | 2026, 2027 adopted |

The spine gives the all-funds picture the chart draws:

- $299,969,007 gross revenue, $254,095,412 gross expenditure (FY2026)
- six fund groups, whose printed `TOTAL SOURCES` and `TOTAL USES` the chart
  reconciles against exactly

The site also publishes **$313,708,146** — the wrong answer — beside the right
one, because naively summing the expenditure column double-counts transfers by
23%, and showing the error you are avoiding is more useful than quietly avoiding
it.

The site publishes **two pages**. `index.html` is the fund-group spine as a
Sankey, with a toggle between the two adopted years. `revenue.html` draws all
**924** of pp.127-140 — 231 printed rows across four columns, every figure a link
to the extracted text of the page it was read from, and a per-row mark whose
scale is that row's own.

The other two detail schedules are published and checked but **not yet drawn**:
pp.167-170 (196 facts) and p.76 (88). `fisc verify` declares each undrawn
schedule with the reason it is undrawn rather than leaving it unsaid. The CIP and
the ACFR are extracted and entirely unmapped. See `bd ready`.

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
./bin/fisc verify     # every claim the fact store and the graph rest on
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
facts/facts.jsonl                  1,448 content-addressed facts, each carrying
                                   doc_id / page / offset / token
  │  internal/project              projections over (columns, scope): one column
  │                                 per Sankey year, four for the revenue trends
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
guidance is in [`docs/agents/`](docs/agents/). Each published document has a
frozen contract of its own: [`docs/sankey-contract.md`](docs/sankey-contract.md)
for the citywide spine and
[`docs/revenue-trends-contract.md`](docs/revenue-trends-contract.md) for the
per-fund revenue series.

One thing a newcomer should know. The site publishes from the `pages-build` and
`pages-deploy` jobs at the foot of
[`.github/workflows/ci.yml`](.github/workflows/ci.yml) — in that file rather
than a workflow of their own so they can `needs:` the checks, on a push to
`main` that has already gone green, once Pages is enabled for the repository
(Settings → Pages → Source → *GitHub Actions*). `dist/` is gitignored and is
built locally by `make site`.

An agent working here never pushes: the repository owner runs `git push` and
`git pull`, so unpushed local commits are the expected end of a session rather
than unfinished work. See [`docs/agents/workflow.md`](docs/agents/workflow.md).
