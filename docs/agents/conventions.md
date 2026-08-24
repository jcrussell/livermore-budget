# Conventions

## Provenance invariants

These are the rules the project exists to uphold. Breaking one is a defect
even when tests pass.

- **Amounts are integer cents.** No float anywhere on the path from an
  extracted cell to a published total. These values are summed and compared
  against printed figures; float drift makes those comparisons meaningless.
- **Absent is not zero.** In these documents `-` means the line exists and is
  zero, while an empty cell means the line does not apply. Conflating them
  invents rows. `amount.Parse` returns `ErrAbsent` for the latter;
  `amount.ParseOrZero` opts out only where a rule has declared that blanks
  mean zero for that table.
- **Fail closed on ambiguity.** PDF extraction corrupts numbers into plausible
  wrong values rather than errors. Every shape not positively recognized is an
  error. Never guess at a value to keep a pipeline green.
- **Published and derived are different things.** A figure the city printed and
  a classification we inferred must not be presented alike. Derived nodes carry
  `derived: true` with a `rationale` and `source_note`, and `verify` fails
  without them.
- **Identity and integrity are separate.** A locator says *which* row this is
  and must be content-independent; a content hash says *whether it changed*.
  One value cannot do both. Budget Book p67 prints four byte-identical
  all-dash lines; content alone cannot say which row any of them is, so
  identity has to come from position. (This invariant was originally argued
  from 16 byte-identical ACFR *tables*. The table substrate no longer exists —
  see the extraction boundary below — but duplicate content does, and the
  argument is the same.)

### A forecast is not a fact

Nothing computed goes into `facts/facts.jsonl`. A fact is one figure the city
printed, and three checks make that a guarantee rather than a claim — none of
which consults `Fact.Derived`:

- `fact-token-reparses` re-parses each fact's own token and fails an empty one.
  Its doc comment settles the case in advance: *"a fact with no token has no
  printed figure behind it, cannot be re-derived, and cannot be cited."*
- `fact-offset-points-at-token` requires the extracted page text at the fact's
  offset to *be* that token. **This is the arm that matters**, because it is the
  one a synthetic token cannot get past: a made-up figure can be made to
  re-parse, but no page prints it.
- `fact-ids-recompute` needs a `rule_id` and a `doc_id` in the hashed tuple.

So `Derived: true` on a fact means a re-reading or re-classification of a figure
the city printed at a page and an offset — never a computed value. The field
exists to *state* the published/derived distinction, not to exempt anything from
the checks above, and it must not become that exemption. An exemption arm is the
worse failure: it does not weaken the store visibly, it weakens it for a set
whose membership is a boolean somebody sets.

A projection or scenario may derive figures, and does so under its own rules —
`derived-nodes-justified` requires a rationale and a source note on every derived
node. The rule here is only about the store. See `fisc-nvw` for where a forecast
is allowed to live and what would have to be true for `fisc verify` to police one.

## Go

Layout and idioms follow the byob decisions; read them with
`bd show byob-<id>` rather than inferring from the code.

- Three-tier layout (`byob-layout.1`): `cmd/fisc` → `internal/fisccmd` →
  `pkg/cmd/root` → `pkg/cmd/<feature>`.
- Each command is `Options` + `NewCmdXxx(f, runF)` + private `xxxRun`
  (`byob-command-shape.1`), with `Options.Validate()` first, failing via
  `cmdutil.FlagErrorf` before any side effects (`byob-input-validation.5`).
- Commands return errors and never call `os.Exit` (`byob-errors.1`). The
  runner owns the error→exit-code mapping.
- One `Factory` built in `main`, threaded everywhere; expensive dependencies
  are lazy closures so `--help` touches no filesystem (`byob-factory-di.1`).
  There is a test asserting exactly that; keep it passing.
- Data goes to `Out`, everything else to `ErrOut` (`byob-iostreams.3`).
- `CGO_ENABLED=0`, pure Go, assets via `go:embed` (`byob-release.8`).
- Stdlib first (`byob-release.10`). A dependency outside cobra, go-cmp,
  modernc sqlite and goreleaser needs its own decision bead in the same
  change — see `fisc-j8f` for the YAML one.

### Testing

- Tests ship in the same commit as the code they cover (`byob-testing.4`).
- `google/go-cmp`, not testify (`byob-testing.2`). `cmp.Diff(want, got)`.
- Assert on behavior, not call counts (`byob-testing.3`).
- **Go tests never require Python, the source PDFs, or the network.** Use the
  fixtures in `testdata/`, which are real artifacts copied from
  `data/extracted/`. They are *copies*, and `make extract` does not touch them:
  the five page fixtures under `testdata/pages/` and
  `pkg/cmd/build/testdata/pages/` have to be re-copied by hand when the
  extraction changes, and their sha256s must equal the ones the source
  document's `manifest.json` records. A fixture that has drifted is the bad
  case — the tests reading it stay green against a substrate that no longer
  exists, which is exactly what happened across the xberg → poppler migration
  (`fisc-yqv.5`). Re-copy; never edit an assertion to fit a stale fixture.
- Smoke-test the built binary, not only the packages. The unknown-command bug
  — a typo'd command exited 0 and printed help — passed every unit test and
  was caught by running `./bin/fisc biuld`.

## The extraction boundary

Extraction is **not** a Go responsibility and must not become one.

`make extract` runs `tools/extract.py`, which is the only Python in the
project. Go never shells out to it. `fisc` reads only the committed artifacts
under `data/extracted/` and needs neither Python nor the PDFs — which is what
lets CI run `verify` without a venv and without an LFS checkout.

The extractor is poppler (`pdftotext`), a system package; the script itself is
standard library only and PyPI is not reachable from the extraction
environment. It emits two substrates per page: `pages/pNNNN.txt` from
`-layout`, which reproduces the printed column grid in runs of spaces, and
`geometry/pNNNN.json` from `-bbox`, which carries per-word bounding boxes.
Both are needed. On a sparse grid `-layout` emits only the tokens that were
printed and nothing that says which column each belongs to, so a positional
read files them left to right and can land a figure under the wrong year;
geometry gives the x-position that settles it. Page text is `.txt` and not
`.md` because GitHub's blob view renders markdown and collapses the spaces
that *are* the grid.

Geometry gives **column identity for tokens that are present**. It does not
recover a value the PDF never put in its text layer, and it does not settle
"absent is not zero" on its own: CIP p40 rows PB200654 and PB202617 print `-`
in their intervening FY columns and those dashes appear in neither substrate,
because they are drawn as non-text. Row PB200429 on the same page does carry
its dashes, so this is per-row and not a flag chosen wrong. A rule that needs
to tell an absent cell from a zero one must say so itself.

poppler has no structured error channel: it writes free-form English to stderr
and exits 0. The manifest records every stderr line under `warnings`, and
reserves `errors` for non-zero exits and unparseable output. An empty `errors`
is not a promise that every page came out whole.

`tools/extract.py` deliberately does **not** read `data/sources.yaml`. It
discovers work from `data/pdf/<doc-id>.pdf` and records the source sha256 it
computed; `fisc verify` cross-checks that against the registry. Two parties
recording the hash independently is a real check — both reading the same file
would not be. Do not "simplify" this by giving Python a YAML parser.

Extraction output must stay byte-stable across runs: words sorted by position
rather than by the tool's emission order, canonical JSON with sorted keys,
bboxes rounded to 2dp, atomic writes. Changing any of that changes every
committed artifact, so it belongs in its own reviewed commit with
`extractor_version` bumped.

## Non-interactive shell commands

Use non-interactive flags; `cp`, `mv`, and `rm` may be aliased to `-i` and
will hang waiting for input that never comes.

```bash
cp -f source dest      # NOT: cp source dest
mv -f source dest      # NOT: mv source dest
rm -f file             # NOT: rm file
rm -rf directory       # NOT: rm -r directory
```

Others that prompt: `scp`/`ssh` need `-o BatchMode=yes`, `apt-get` needs `-y`,
`brew` needs `HOMEBREW_NO_AUTO_UPDATE=1`.

## Fetching the source PDFs

`www.livermoreca.gov` sits behind Akamai bot protection and returns HTTP 403
to a bare `curl` — an error page, not a network failure, so it is easy to
misdiagnose as a proxy problem. The full browser header set that works is in
[`docs/m0-spike.md`](../m0-spike.md).
