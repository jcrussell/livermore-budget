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

**Thirty-eight pages of 786, across two of the three documents** (measured at
`6da1c44`). This is still a proof of concept, and saying so plainly is part of
the point.

| document | pages | extracted | mapped |
|---|---:|---:|---:|
| FY 2025-2027 Budget Book | 268 | all | **35** (pp. 66–67, 76, 85–125 in part, 127–140, 167–170) |
| 2025-2030 Capital Improvement Plan | 323 | all | 0 |
| FY 2024-25 Annual Comprehensive Financial Report | 195 | all | **3** (pp. 41, 167–168, in part) |

Those pages carry nine schedules, each of which reconciles against something the
city itself printed:

| schedule | what it is | fiscal years |
|---|---|---|
| pp. 66–67 | the citywide spine, all funds gross | 2026, 2027 adopted |
| pp. 127–140 | revenue by fund and line item | 2024 actual, 2025 revised, 2026 + 2027 adopted |
| pp. 167–170 | General Fund department × object category | the same four |
| p. 76 | the transfer schedule, both legs of every transfer | 2026, 2027 adopted |
| pp. 85–125 | which funds pay for each department | 2024 actual, 2025 revised, 2026 + 2027 adopted |
| pp. 85–124 | departmentwide expenditure by object category | the same four |
| ACFR p. 41 | the General Fund's revenues, transfers, General Government divisions and fund balances | 2025 audited |
| ACFR p. 167 | fund balances, ten years | 2016–2025 audited |
| ACFR p. 168 | changes in fund balances, ten years | 2016–2025 audited |

The spine gives the all-funds picture the chart draws:

- $299,969,007 gross revenue, $254,095,412 gross expenditure (FY2026)
- six fund groups, whose printed `TOTAL SOURCES` and `TOTAL USES` the chart
  reconciles against exactly

The site also publishes **$313,708,146** — the wrong answer — beside the right
one, because naively summing the expenditure column double-counts transfers by
23%, and showing the error you are avoiding is more useful than quietly avoiding
it.

The site publishes **six pages**. `index.html` is the fund-group spine as a
Sankey, with a toggle between the two adopted years, and it is the one chart
page: **click a fund group and it opens** into that group's own funds from
pp.127-140 and pp.167-170, drawn for the same fiscal year; click a General Fund
division and it opens again into what that division spends on — each rescaled
to the opened node's own total, because the citywide chart cannot show them at
all. `trends.html` draws all
**924** of pp.127-140 — 231 printed rows across four columns, every figure a
link to the extracted text of the page it was read from, and a per-row mark
whose scale is that row's own. `history.html` and `balances.html` are the ACFR's
two ten-year schedules — p168's changes in fund balances and p167's balances
themselves — a decade a column, in the section the ACFR heads "(Unaudited)" and
each page says so. `provenance.html` is the record store itself: every fact, its
page, and the text it was read from. `caveats.html` is every published
document's caveats in full — the other pages show each as one line and link
here, so a reader meets the chart before the apparatus rather than scrolling
past 254 words of it.

One schedule is mapped and checked but **drawn by no chart**: ACFR p.41. Its
facts appear only in `provenance.html`. The CIP is extracted and entirely
unmapped. See `bd ready`.

## Build and look at it

Requires Go. Neither the PDFs nor Python are needed to build or verify — those
are for re-extraction only.

```bash
make build                          # build bin/fisc
make test                           # tests, always with -race
make site                           # build the static site into dist/
python3 -m http.server -d dist 8000 # then open http://localhost:8000
```

The page fetches `fy2026-adopted.json` — one document per published column,
carrying every schedule that column prints — rather than inlining it, so it
needs a server; a provenance file you cannot `curl` on its own is not much of an
audit trail.

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
facts/facts.jsonl                  content-addressed facts, each carrying
                                   doc_id / page / offset / token
  │  internal/project              projections over (columns, scope): one column
  │                                 per Sankey year, four for the revenue trends
  ▼
dist/                              static site: d3-sankey, no bundler, no build step
```

Four invariants hold throughout and most of the code exists to enforce them:
amounts are integer cents, absent is not zero, ambiguity fails closed, and
published is not derived. They are stated in
AGENTS.md, "Provenance invariants".

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
guidance is in [`AGENTS.md`](AGENTS.md). Each published document has a
frozen contract of its own: [`docs/sankey-contract.md`](docs/sankey-contract.md)
for the citywide spine,
[`docs/revenue-trends-contract.md`](docs/revenue-trends-contract.md) for the
per-fund revenue series, and
[`docs/general-fund-drilldown-contract.md`](docs/general-fund-drilldown-contract.md)
for the fund-and-division drill-down, and
[`docs/fact-store-contract.md`](docs/fact-store-contract.md) for the record
store the three of them are drawn from.

One thing a newcomer should know. The site publishes from the `pages-build` and
`pages-deploy` jobs at the foot of
[`.github/workflows/ci.yml`](.github/workflows/ci.yml) — in that file rather
than a workflow of their own so they can `needs:` the checks, on a push to
`main` that has already gone green, once Pages is enabled for the repository
(Settings → Pages → Source → *GitHub Actions*). `dist/` is gitignored and is
built locally by `make site`.

An agent working here never pushes: the repository owner runs `git push` and
`git pull`, so unpushed local commits are the expected end of a session rather
than unfinished work. See [`AGENTS.md`](AGENTS.md).
