# M0 spike findings

> **Historical record, 2026-08-15. Read the notes marked SUPERSEDED.**
>
> This is the spike that chose the substrate, and [`AGENTS.md`](../AGENTS.md) still
> names it as required reading — so the parts of it that are no longer true have
> to say so rather than be quietly deleted. The extraction pipeline was replaced
> wholesale on 2026-08-17 (`fisc-yqv`): **xberg is gone, `pdftotext` from poppler
> 24.02.0 writes `-layout` text plus `-bbox` geometry, the table substrate and
> the markdown output no longer exist, and `extractor_version` is 3.**
>
> Findings 2, 3 and 4 survive the replacement intact, and finding 4 is the reason
> this document is kept at all. Findings 1 and 5 and the Cost section describe a
> pipeline that no longer exists; each is annotated below. Numbers here are the
> spike's own (136 facts, 16 columns); the real implementation publishes 240
> facts over 24 columns for two budget years.

Throwaway validation run before any schema was frozen. The question it had to
answer: **can we get labelled, checkable budget figures out of these PDFs, and
from which substrate?**

Answer: **yes, from page text.** The spike mapped 136 facts from Budget Book
p66–67 and all 16 column totals tied *exactly* to the totals the document itself
prints.

## Verified numbers

| Figure | Value |
|---|---|
| General Fund FY2025-26 revenues | $157,873,470 |
| General Fund FY2025-26 expenditures | $144,650,802 |
| All-funds FY2025-26 revenues | $299,969,007 |

Every fund-group column (General, Enterprise, Capital, Debt Service, Special
Revenue, Internal Service) reconciles against its printed `TOTAL REVENUES:` /
`TOTAL EXPENDITURES:` line for both budget years.

## Findings that shaped the design

**1. Page text is the primary substrate, not tables.** Table extraction reaches
only 46% of the Budget Book's money-bearing pages (62% ACFR, 52% CIP), and the
misses are concentrated on the pages that matter most: the citywide spine
(p66–67), the transfer schedule (p76), and revenue-by-fund (p127–140) all yield
*zero* table objects. Page text carries the same content and is complete.

> **SUPERSEDED in form, upheld in conclusion.** The recall percentages compare
> against xberg's table objects, a substrate that no longer exists — `fisc-yqv.6`
> deleted it and the markdown residue with it. The conclusion is now structural
> rather than empirical: page text is the *only* text substrate. A second
> substrate did arrive, but it is word geometry (`-bbox`), and it does not carry
> content — it supplies a column index the text read is checked against
> (`fisc-yqv.1`, `fisc-yqv.2`). See the `substrate-is-page-text` memory.

**2. The spine's grammar is "label followed by N numbers."** p66 carries the row
labels plus the General Fund and Enterprise columns — 4 values per row, being
2 fund groups × 2 fiscal years.

**3. p67 has no row labels at all.** It is a positional continuation of p66 for
the other four fund groups (8 values per row). Row identity comes from p66's
order, which means a logical table can span pages with the label page as the
anchor.

**4. The row-count assertion caught an extractor bug — not a document quirk.**
The spike read 72 values on p67 where 10 rows × 8 columns requires 80, and
failed loudly:

```
p67 row-count mismatch: 72 values is not 10 rows x 8 columns.
Declared omissions: []
```

This was originally written up as "p67 omits rows that are all-zero across its
four fund groups," and `Licenses & Permits` was declared absent from p67 to
make the count work. **That reading was wrong.** `pdftotext -bbox` puts ten
revenue rows on p67, at the same ten y-positions as p66's ten labelled rows —
the document omits nothing. Rows 8–10 (`Sales Taxes`, `Fines & Forfeitures`,
`Licenses & Permits`) are three consecutive all-dash lines, and xberg's
`content_filter.strip_repeating_text` — a header/footer dedup heuristic that
defaults to on — deleted the third as boilerplate. It was disabled in
`tools/extract.py` at `EXTRACTOR_VERSION = 2`; see bead `fisc-c00`.

> **The remedy is superseded; the finding is not.** xberg and its
> `strip_repeating_text` are gone — `pdftotext -layout` has no such heuristic and
> reproduces the printed page. The failure class it belongs to is very much
> alive, and it is why the replacement happened at all: `fisc-yqv` was opened
> because the *same* pipeline was found to be dropping 33 of 40 rows on p167 and
> all 13 on p130. Read the paragraph below as the standing lesson, not as
> history.

The lesson is stronger than the original one. A positional read of a label-less
continuation page identifies rows by order, so a row silently removed *by our
own pipeline* mismaps every row beneath it. The count assertion is the only
thing standing between that and 16 published facts carrying the wrong
`row_label` — which is part of the fact id. Fail-closed paid for itself here.

**5. `\-` is a seventh amount-corruption mode.** The extracted markdown escapes
some dashes, so a backslash-dash appears as a distinct zero token alongside the
six modes already catalogued (whitespace-split digits, em-dash-as-zero glued to
the next column, `$` bleeding one cell right, parenthesis negatives, `-` vs `''`
meaning zero vs absent, and varying units).

> **SUPERSEDED — this mode no longer exists.** It was an artefact of the markdown
> output, and `tools/extract.py` now writes raw `pdftotext -layout` bytes with no
> markdown and no cell normalisation. The other six modes are real and are what
> `internal/amount` is built against; see the `amounts-fail-closed` memory. Two
> corruption shapes found *since* and not in this list: a figure split across a
> token boundary (`$21130 083`, CIP p29) and printed dashes that appear in no
> substrate at all because they are drawn as non-text (CIP p40, `fisc-8ln`).

## Cost

The whole corpus — 786 pages across three PDFs — extracts in **10 seconds**
with `disable_ocr=True, layout=None`, fully offline, no model downloads.

> **SUPERSEDED.** Those are xberg parameters. Extraction is now
> `pdftotext -layout` and `pdftotext -bbox` per page via `make extract`; it is
> still fully offline and needs no model downloads, but the timing and the flags
> above no longer describe anything. The pinned toolchain is poppler 24.02.0,
> asserted by `fisc verify`'s `extraction-toolchain-pinned` check against
> `corpus.PinnedPopplerVersion`.

## Retrieving the source PDFs

`www.livermoreca.gov` is behind Akamai bot protection and returns **HTTP 403 to
a bare `curl`** (an `AkamaiGHost` error page, not a network failure). A full
browser header set gets through:

```bash
curl -sSL --compressed \
  -A "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36" \
  -H "Accept: text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8" \
  -H "Accept-Language: en-US,en;q=0.9" \
  -H "Sec-Fetch-Dest: document" -H "Sec-Fetch-Mode: navigate" \
  -H "Sec-Fetch-Site: none" -H "Sec-Fetch-User: ?1" \
  -H "Upgrade-Insecure-Requests: 1" \
  "https://www.livermoreca.gov/home/showpublisheddocument/12813" -o budget.pdf
```

Document permalinks take the form `/home/showpublisheddocument/<id>`. The
longer form with a trailing .NET-ticks value is a cache-buster; both serve the
same bytes, and `data/sources.yaml` records each.

All three committed PDFs were re-downloaded and verified **byte-for-byte
identical** to the city's published files on 2026-08-15. Because Git LFS uses
sha256 as its object ID, the LFS OIDs double as a second copy of that check.

## Status

The reference implementation is **not production code**. It exists to prove the
approach and to give the real implementation (`internal/mapping`, bead
`fisc-mq4.7`) a known-good target to reproduce.

`fisc-mq4.7` closed: `internal/mapping` reproduces the spine and goes past it,
publishing 240 facts over 24 columns for both budget years where the spike read
136 over 16. So the target has been hit and this section is settled.

`mappings/*.yaml` plus `fisc build` is the live path.
