# Where the project stands

Updated at the end of the session of 2026-08-15. Beads are the source of
truth for *what* is left; this file is the context that would otherwise have
to be re-derived.

Start with `bd ready --exclude-type=byob`.

## Done

**Provenance root is verified, not asserted.** All three PDFs were
re-downloaded from livermoreca.gov and confirmed byte-identical to the
committed copies. `data/sources.yaml` records both the stable canonical URL
and the versioned one each retrieval came from.

**Extraction is reproducible.** `make extract` produces 1,171 artifacts (4.9 MB)
across 786 pages in ~13 s, offline, and byte-identical across runs. Tables are
sorted by `(page, y0, x0)` rather than xberg's iteration order, which is not
document order.

**`fisc` skeleton, `internal/amount`, `internal/mapping` are in**, with
`.golangci.yml` clean at zero issues.

## The single most important thing to know

**Page text is the substrate, not tables.** Table extraction reaches only 46%
of the Budget Book's money-bearing pages and misses every schedule that
matters: the citywide spine (pp. 66–67), the transfer summary (p76), and
revenue-by-fund (pp. 127–140) all yield *zero* table objects. Anyone who
starts from `doc.tables` will conclude the project is impossible.

The corollary: figures in these documents are corrupted in ways that produce
**plausible wrong values rather than errors**. `internal/amount` refuses seven
distinct corruption shapes for that reason, and every relaxation of it needs
arithmetic proof, not intuition. See `TestLeadingMinusIsReallyPositive` for the
standard.

## Next up

The critical path is `E4 → E5 → E6` and it is a chain; fanning out will not
shorten it.

1. **`fisc-mq4.2` locator resolution** — the next real step. Resolve a rule's
   part against committed page text, requiring a unique match and reporting
   candidates on ambiguity. `internal/mapping/testdata/spine.yaml` is a working
   target: the M0 spike proved 136 facts come out of pp. 66–67 with all 16
   column totals tying exactly to the printed totals.
2. `fisc-ft0.1` `funds.yaml` and `fisc-ft0.2` `taxonomy.yaml` — independent
   data entry, safe to run in parallel with the above.
3. `fisc-mq4.4` fact model, then `fisc-mq4.5` `fisc build`.

Nothing is deployable until E6. If a visible artifact matters sooner, the
cheapest reordering is to thin E5 to tier-1 checks and pull E6 forward — the
spine alone is enough to render a real Sankey.

**E7 is the bulk of the remaining labour** and is highly parallel: six tasks of
independent mapping rules over different page ranges, each verifiable against
its own printed totals. E8's projections likewise. That is where subagents pay;
right now there is one genuinely independent task.

## Traps, all confirmed the hard way

- **`git add -A` will sweep in agent worktrees** as embedded git repos.
  `.claude/worktrees/` is now ignored, but prefer adding explicit paths.
- **livermoreca.gov returns 403 to a bare `curl`** — Akamai bot protection, an
  error page rather than a network failure, easy to misdiagnose as a proxy
  problem. The working header set is in `docs/m0-spike.md`.
- **PyPI is blocked** in this sandbox, so `tools/extract.py` has no YAML
  parser. That is now load-bearing by design, not a workaround: extraction and
  the registry record the source sha256 independently, so `fisc verify` cross-
  checking them is a real check. Do not "fix" it.
- **`bd` regenerates a block in AGENTS.md and CLAUDE.md** asserting that agents
  must `git push`. They must not. The override sits above the BEGIN marker in
  both files and will need restoring after a `bd` upgrade.
- **GitHub LFS is 1 GB/month** and each LFS checkout is ~70 MB. Keep LFS out of
  CI. `raw.githubusercontent.com` serves LFS pointer text, not the PDF, so the
  site must link to the city's URL for `#page=N` deep links.
- Release assets need `release-assets.githubusercontent.com` allowlisted;
  `github-cloud.githubusercontent.com` will likely be needed for the first LFS
  push.

## Review cadence

`/code-review` before any commit landing a new package, and at each epic
boundary. It has found real defects every time it has run — including silent
int64 overflow that returned a *negative* dollar figure, and a fixture anchored
on the very total it was meant to verify. See `workflow.md` for why it does not
replace the corpus scans and arithmetic checks.
