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
  One value cannot do both — 16 ACFR tables are byte-identical to another, one
  group 12 deep, and the Budget Book has a 20-deep group.

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
  `data/extracted/`.
- Smoke-test the built binary, not only the packages. The unknown-command bug
  — a typo'd command exited 0 and printed help — passed every unit test and
  was caught by running `./bin/fisc biuld`.

## The extraction boundary

Extraction is **not** a Go responsibility and must not become one.

`make extract` runs `tools/extract.py`, which is the only Python in the
project. Go never shells out to it. `fisc` reads only the committed artifacts
under `data/extracted/` and needs neither Python nor the PDFs — which is what
lets CI run `verify` without a venv and without an LFS checkout.

`tools/extract.py` deliberately does **not** read `data/sources.yaml`. It
discovers work from `data/pdf/<doc-id>.pdf` and records the source sha256 it
computed; `fisc verify` cross-checks that against the registry. Two parties
recording the hash independently is a real check — both reading the same file
would not be. Do not "simplify" this by giving Python a YAML parser.

Extraction output must stay byte-stable across runs: tables sorted by
`(page, y0, x0)` rather than xberg's iteration order, canonical JSON with
sorted keys, bboxes rounded to 2dp, atomic writes. Changing any of that
changes every committed artifact, so it belongs in its own reviewed commit
with `extractor_version` or `normalizer_version` bumped.

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
