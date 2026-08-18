package check

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"path"
	"sort"

	"github.com/jcrussell/livermore-budget/internal/corpus"
	"github.com/jcrussell/livermore-budget/internal/registry"
)

// The structural checks: tier 0, because they are not about arithmetic.
//
// Everything else in this package reasons about figures — that an amount matches
// its token, that a link's value ties to the facts under it, that a total sums.
// All of it reads the committed extraction, and none of it can tell whether that
// extraction is still the one the rules were written against. A page whose bytes
// changed under a mapping produces facts whose offsets land on whatever is there
// now; a manifest edited to match a changed page makes the extraction agree with
// itself; an extra file nobody vouches for is one a future rule can read. These
// are the checks that make those events visible, and they come first in the
// report because a drifted artifact invalidates every arithmetic result below it.
//
// Five of them need no PDF and no Python: they compare the artifacts against the
// manifest that lists them, the manifest's size and error record against what it
// should contain, and the manifest against the registry — all committed. The sixth
// needs the source documents and is the reason --full exists.
//
// Read them as a set, because two of them exist to close the others' holes. The
// artifact hashes are a bijection between the manifest's map and the directory, and
// a bijection says nothing about SIZE: delete a page from both sides and the two
// agree. page_count is the independent number that catches that, and it is itself
// checked against data/sources.yaml, which was written by hand from the document.

// artifactsMatchManifest asserts the committed extraction is the one its manifest
// describes, in both directions.
//
// Forwards is the obvious half: a file whose bytes changed, or that is gone. The
// other half is the one that matters and is easy to leave out — a file present
// under data/extracted/ that the manifest does not list. Nothing vouches for such
// a file, the extractor did not write it in the run the manifest records, and a
// rule that reads it (or a reader who follows a provenance link to it) is reading
// bytes with no recorded origin. tools/extract.py clears pages/ and geometry/
// before a run precisely so a shorter re-extraction cannot leave one behind; this
// is the check that notices when something does.
//
// # What a matching hash does and does not witness
//
// It witnesses drift SINCE EXTRACTION and nothing else. It does not say the
// extraction was right, and it cannot: a one-byte edit to a page plus its manifest
// entry passes here, and so does any change made consistently on both sides. That
// is not a gap to be closed by hashing harder — the manifest is a record of what
// the extractor wrote, so the only witness to whether it wrote the right thing is
// the source document, and only for the pages something reads. Two facts about the
// corpus follow, and are worth stating rather than implying: no check in this
// project reads the bytes of a page no fact cites, or of any geometry artifact
// other than those the mapped pages carry -- and those are read by `fisc build`
// rather than by a check, so it is the rebuild-and-diff in CI, not this sweep,
// that would notice a corruption in one. For every other file "unchanged since
// extraction" is the whole of what is known. Re-extracting is what re-establishes
// the rest, and reviewing the artifact diff by hand is the step that cannot be
// automated away (`fisc reanchor`,
// which would make that reviewable in one commit, is fisc-mq4.6 and not built).
//
// manifest.json itself is not swept: it is not one of its own artifacts, it records
// a hash for everything the extractor emitted and cannot record its own. Nor is it
// vouched for by a hash anywhere. What is checked about it is narrower and worth
// being exact about: its source_* fields and its versions are compared against a
// second, independent record (manifest-matches-source-registry,
// extraction-toolchain-pinned), and its size claim against its own contents
// (extraction-emitted-every-page). Its artifact MAP is compared only against the
// directory, which is the limit described above.
type artifactsMatchManifest struct{}

var _ Check = (*artifactsMatchManifest)(nil)

func (*artifactsMatchManifest) ID() string { return "artifacts-match-manifest" }
func (*artifactsMatchManifest) Tier() int  { return 0 }
func (*artifactsMatchManifest) Full() bool { return false }
func (*artifactsMatchManifest) Description() string {
	return "every file under " + extractedDir + " hashes to the sha256 and size its " +
		"manifest records, and the manifest lists every file that is there"
}

// Run hashes every artifact of every extraction: about 1,570 files and 15 MB
// today, which is a fraction of a second and the only way this claim can be
// made at all.
//
// A file it cannot read is a finding rather than an error, because that is a
// state of the repository — a deleted page, a directory made unreadable — and
// the report has to be able to say which files it was. An unreadable extraction
// DIRECTORY is different: the walk itself failed, so this check cannot say what
// is or is not there, and that is returned as an error.
func (*artifactsMatchManifest) Run(_ context.Context, s *Subject) (Result, error) {
	var findings []Finding
	subjects := 0

	for _, docID := range sortedStrings(s.Extractions) {
		doc := s.Extractions[docID]
		listed := doc.Artifacts()
		present, err := extractionFiles(doc.Tree())
		if err != nil {
			return Result{}, fmt.Errorf("read the extraction of %s under %s: %w",
				docID, extractedDir, err)
		}

		for _, rel := range sortedStrings(listed) {
			subjects++
			want := listed[rel]
			gotBytes, gotSHA, err := hashArtifact(doc.Tree(), rel)
			switch {
			case err != nil:
				findings = append(findings, finding(artifactPath(docID, rel),
					"the manifest lists this artifact (%d bytes, sha256 %s) but it cannot be "+
						"read: %v", want.Bytes, short(want.SHA256), err))
			case gotSHA != want.SHA256:
				findings = append(findings, finding(artifactPath(docID, rel),
					"hashes to %s (%d bytes) but the manifest records %s (%d bytes); the "+
						"artifact has changed since it was extracted",
					short(gotSHA), gotBytes, short(want.SHA256), want.Bytes))
			case gotBytes != want.Bytes:
				// Unreachable in practice — two byte strings of different lengths
				// do not share a sha256 — and reported rather than ignored anyway:
				// a manifest whose two records of one file disagree is a manifest
				// that was edited, and that is worth saying out loud.
				findings = append(findings, finding(artifactPath(docID, rel),
					"is %d bytes but the manifest records %d, though the sha256 %s matches; "+
						"the manifest disagrees with itself",
					gotBytes, want.Bytes, short(want.SHA256)))
			}
		}

		for _, rel := range present {
			if _, ok := listed[rel]; ok {
				continue
			}
			subjects++
			findings = append(findings, finding(artifactPath(docID, rel),
				"is present but the manifest does not list it, so nothing records where it "+
					"came from; re-run `make extract`, which clears the emitted directories "+
					"before writing, and commit the result"))
		}
	}

	return conclusion{
		subjects: subjects,
		unit:     "artifacts",
		held: fmt.Sprintf("%d artifacts across %d %s, each hashing to the sha256 its "+
			"manifest records, and nothing present that is unlisted",
			subjects, len(s.Extractions), plural(len(s.Extractions), "extraction", "extractions")),
		nothing:  "no extraction is committed under " + extractedDir,
		findings: findings,
	}.result(), nil
}

// extractionFiles is every regular file in one extraction directory, as
// slash-separated paths relative to it, sorted, and without the manifest.
func extractionFiles(fsys fs.FS) ([]string, error) {
	var out []string
	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		switch {
		case err != nil:
			return err
		case d.IsDir(), p == corpus.ManifestFile:
			return nil
		}
		out = append(out, p)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(out)
	return out, nil
}

// hashArtifact hashes one artifact, returning its size and lower-case hex
// sha256 — the two things the manifest records about it.
//
// It streams rather than reading the file whole: the largest geometry artifact is
// modest, but a checker that loads whatever it is pointed at is a checker with a
// size limit nobody wrote down.
func hashArtifact(fsys fs.FS, name string) (int64, string, error) {
	f, err := fsys.Open(name)
	if err != nil {
		return 0, "", err
	}
	defer f.Close() //nolint:errcheck // read-only file; nothing to flush
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return 0, "", err
	}
	return n, hex.EncodeToString(h.Sum(nil)), nil
}

// artifactPath names an artifact the way a reader can act on: the path from the
// repository root, which is what a finding has to print for `git log` or an editor
// to be any use.
//
// It concatenates rather than path.Join-ing, because Join CLEANS, and rel may be a
// manifest key — which corpus.Artifacts hands back exactly as the file spells it,
// so that a report cites a path the file actually contains. A key of
// "pages/../geometry/p0001.json" is unreadable through an fs.FS and must be
// reported under the name that makes that make sense; cleaning it to
// "geometry/p0001.json" would print an existing, readable path and call it
// unreadable.
func artifactPath(docID, rel string) string { return extractedDir + "/" + docID + "/" + rel }

// short abbreviates a sha256 for a report line. Twelve hex digits is 48 bits,
// which is plenty to tell two hashes apart by eye and short enough that the
// interesting half of the sentence is still on the line. The full value is in the
// manifest, which the finding names.
func short(sha string) string {
	if len(sha) <= 12 {
		return sha
	}
	return sha[:12] + "…"
}

// extractionEmittedEveryPage asserts each extraction is COMPLETE: two artifacts
// for every page of the document.
//
// This is the check that makes the artifact sweep mean something, and it is
// separate from it deliberately. artifacts-match-manifest is a bijection between
// the manifest's map and the directory, and a bijection is a consistency claim, not
// a completeness one: delete every page of an extraction from the directory AND
// from its manifest and the two agree perfectly. The report would go from 1,572
// artifacts to 1,182 and still say PASS — a number a reader would have to diff two
// reports to notice, over an extraction that is now empty.
//
// page_count is the witness, because it is the one number in the manifest that is
// not derived from the artifact map: pdfinfo counted the pages. It is not
// self-certifying either, so it is checked twice — manifest-matches-source-registry
// compares it against the `pages:` data/sources.yaml records by hand from the
// document, and registry.LoadSources refuses a source with no page count — which is
// what stops the same deletion being laundered by zeroing page_count as well.
//
// It is its own check rather than a clause of the sweep because the two work in
// different units, and the units are the substance here. The sweep counts
// artifacts, in the thousands, aggregated across documents; an emptied extraction
// is invisible in an aggregate. This counts DOCUMENTS, and prints each one's page
// count in its own summary, so the passing report states the size of every
// extraction on the line a reader actually reads.
//
// What it does not witness: a page poppler returned no text for still has both
// artifacts, so a run where fifty pages came out blank is complete by this
// definition. The blank count is printed for that reason.
type extractionEmittedEveryPage struct{}

var _ Check = (*extractionEmittedEveryPage)(nil)

func (*extractionEmittedEveryPage) ID() string { return "extraction-emitted-every-page" }
func (*extractionEmittedEveryPage) Tier() int  { return 0 }
func (*extractionEmittedEveryPage) Full() bool { return false }
func (*extractionEmittedEveryPage) Description() string {
	return "every extraction lists a page text and a word geometry artifact for each of the " +
		"pages its manifest counted, and nothing else"
}

// Run compares the artifact map against page_count in both directions: a page with
// no artifact, and an artifact for a page the document does not have.
//
// Both directions matter for the same reason they do in the sweep. A missing page is
// a partial extraction; an artifact numbered past the end is a manifest describing a
// document other than the one it claims, and it is what a re-extraction of a
// SHORTER document would leave behind if extract.py's clean_output ever stopped
// clearing the directories.
func (*extractionEmittedEveryPage) Run(_ context.Context, s *Subject) (Result, error) {
	var findings []Finding
	var summaries []string

	for _, docID := range sortedStrings(s.Extractions) {
		doc := s.Extractions[docID]
		listed := doc.Artifacts()
		pages := doc.PageCount()

		expected := make(map[string]bool, 2*pages)
		var missingText, missingGeometry []int
		for n := 1; n <= pages; n++ {
			text, geometry := corpus.PagePath(n), corpus.GeometryPath(n)
			expected[text], expected[geometry] = true, true
			if _, ok := listed[text]; !ok {
				missingText = append(missingText, n)
			}
			if _, ok := listed[geometry]; !ok {
				missingGeometry = append(missingGeometry, n)
			}
		}
		var unexpected []string
		for _, rel := range sortedStrings(listed) {
			if !expected[rel] {
				unexpected = append(unexpected, rel)
			}
		}

		var says []string
		if len(missingText) > 0 {
			says = append(says, fmt.Sprintf("no page text for %s",
				describePages(missingText, pages)))
		}
		if len(missingGeometry) > 0 {
			says = append(says, fmt.Sprintf("no word geometry for %s",
				describePages(missingGeometry, pages)))
		}
		if len(unexpected) > 0 {
			says = append(says, fmt.Sprintf("%d %s for no page of a %d-page document (%s)",
				len(unexpected), plural(len(unexpected), "artifact", "artifacts"), pages,
				joinComma(capped(unexpected, 5))))
		}
		if len(says) > 0 {
			findings = append(findings, finding(docID,
				"the manifest counts %d pages and lists %d artifacts: %s. An extraction "+
					"missing pages is one whose manifest agrees with its own directory, so "+
					"nothing else in this report can see it",
				pages, len(listed), joinComma(says)))
			continue
		}
		summaries = append(summaries, describeExtraction(docID, pages, doc.BlankPageCount()))
	}

	return conclusion{
		subjects: len(s.Extractions),
		unit:     "extractions",
		held: fmt.Sprintf("%d extractions, each with a page text and a word geometry "+
			"artifact for every page counted: %s",
			len(s.Extractions), joinComma(summaries)),
		nothing:  "no extraction is committed under " + extractedDir,
		findings: findings,
	}.result(), nil
}

// describeExtraction is one document's size, for the passing summary. The blank
// count appears only when there is one, because a zero would read as a caveat where
// there is none — and where there is one, it is the caveat on this check's claim.
func describeExtraction(docID string, pages, blank int) string {
	if blank == 0 {
		return fmt.Sprintf("%s %d pages", docID, pages)
	}
	return fmt.Sprintf("%s %d pages (%d blank)", docID, pages, blank)
}

// describePages renders a list of page numbers for a finding, capped. The total is
// carried so that "195 of 195" reads as an emptied extraction rather than as a long
// list.
func describePages(pages []int, of int) string {
	if len(pages) == of {
		return fmt.Sprintf("any of the %d pages", of)
	}
	out := make([]string, 0, len(pages))
	for _, n := range pages {
		out = append(out, fmt.Sprintf("p%d", n))
	}
	return fmt.Sprintf("%d of %d pages (%s)", len(pages), of, joinComma(capped(out, 8)))
}

// capped truncates a list for a report line, saying how much it left out. A finding
// that prints 195 page numbers is a finding nobody reads to the end of, and the
// manifest holds the whole of it.
func capped(items []string, n int) []string {
	if len(items) <= n {
		return items
	}
	return append(items[:n:n], fmt.Sprintf("and %d more", len(items)-n))
}

// extractorReportedNoErrors asserts the extractor did not tell us it failed.
//
// tools/extract.py records every failure in the manifest rather than only logging
// it, on the stated principle that "a run that failed on some pages must be visible
// to fisc verify rather than looking clean", and exits non-zero. This is the
// consumer that promise was made to: until it existed, a poppler run that failed on
// fifty pages wrote a manifest naming all fifty failures, emitted fifty fewer
// artifacts, exited 1 — and `fisc verify` said PASS, because the missing pages were
// missing from the manifest too and corpus.Doc documents an unlisted page as the
// NORMAL "no such page" case (fisc-6r5).
//
// It is a second, independent witness to the same event
// extraction-emitted-every-page catches, and neither subsumes the other. This one is
// the extractor's own confession and names the stage and the message, which a count
// cannot; the count fires when the confession is absent, which is the case
// extract.py itself warns about — "a run whose `errors` is empty is NOT a promise
// that every page came out whole".
//
// Warnings are not failures and are not treated as any: all three committed
// extractions carry some, poppler writes free-form English to stderr and exits 0 for
// a damaged xref and an unread metadata key alike, and a check that failed on them
// would be red today and permanently. They are counted into the passing summary
// instead, because the alternative — decoding them and saying nothing — is how
// "reading the warnings is the human's job" becomes nobody's.
type extractorReportedNoErrors struct{}

var _ Check = (*extractorReportedNoErrors)(nil)

func (*extractorReportedNoErrors) ID() string { return "extractor-reported-no-errors" }
func (*extractorReportedNoErrors) Tier() int  { return 0 }
func (*extractorReportedNoErrors) Full() bool { return false }
func (*extractorReportedNoErrors) Description() string {
	return "no extraction manifest records an extraction error, which is the extractor's own " +
		"account of a page it could not read and did not write"
}

func (*extractorReportedNoErrors) Run(_ context.Context, s *Subject) (Result, error) {
	var findings []Finding
	warnings, blank := 0, 0

	for _, docID := range sortedStrings(s.Extractions) {
		doc := s.Extractions[docID]
		for _, w := range doc.Warnings() {
			warnings += w.Count
		}
		blank += doc.BlankPageCount()

		errs := doc.Errors()
		if len(errs) == 0 {
			continue
		}
		says := make([]string, 0, len(errs))
		for _, e := range errs {
			where := fmt.Sprintf("p%d", e.Page)
			if e.Page == 0 {
				where = "the document"
			}
			says = append(says, fmt.Sprintf("%s %s: %s", where, e.Stage, e.Message))
		}
		findings = append(findings, finding(docID,
			"the extractor recorded %d %s and wrote no artifact for what failed, so this "+
				"extraction is incomplete: %s. Re-run `make extract` for this document; it "+
				"exits non-zero when this happens", len(errs),
			plural(len(errs), "failure", "failures"), joinComma(capped(says, 5))))
	}

	return conclusion{
		subjects: len(s.Extractions),
		unit:     "extractions",
		held: fmt.Sprintf("%d extractions, none recording a failure; %d poppler %s and %d "+
			"blank %s across them, which poppler did not treat as fatal and which are "+
			"recorded in each manifest.json for a human to read",
			len(s.Extractions), warnings, plural(warnings, "warning", "warnings"),
			blank, plural(blank, "page", "pages")),
		nothing:  "no extraction is committed under " + extractedDir,
		findings: findings,
	}.result(), nil
}

// manifestMatchesSourceRegistry asserts the extraction and the registry agree
// about the document each extraction came from.
//
// The agreement is evidence because the two records are independent.
// tools/extract.py discovers its work from data/pdf/<doc-id>.pdf, computes the
// hash and size itself, and deliberately has no YAML parser — so it cannot copy
// either number from data/sources.yaml, and data/sources.yaml's numbers were
// verified against a re-download. Two parties that never read each other agreeing
// on a sha256 means the extraction really was made from the document the registry
// says it was. This check needs no PDF for exactly that reason: it compares two
// written claims, not bytes.
//
// It also checks that the two sets of document ids are the same set. A source
// nobody has extracted is a document the site can cite and no fact can come from;
// an extraction no source claims is 15 MB of pages with no recorded origin, no URL
// and no retrieval date, which is the provenance chain broken at its first link.
type manifestMatchesSourceRegistry struct{}

var _ Check = (*manifestMatchesSourceRegistry)(nil)

func (*manifestMatchesSourceRegistry) ID() string { return "manifest-matches-source-registry" }
func (*manifestMatchesSourceRegistry) Tier() int  { return 0 }
func (*manifestMatchesSourceRegistry) Full() bool { return false }
func (*manifestMatchesSourceRegistry) Description() string {
	return "every document is both listed in " + sourcesFile + " and extracted, and each " +
		"manifest records the same source file, size, sha256 and page count the registry does"
}

func (*manifestMatchesSourceRegistry) Run(_ context.Context, s *Subject) (Result, error) {
	sources := make(map[string]registry.Source, len(s.Sources))
	for _, src := range s.Sources {
		sources[src.ID] = src
	}

	var findings []Finding
	documents := union(sources, s.Extractions)
	for _, docID := range documents {
		src, listed := sources[docID]
		doc, extracted := s.Extractions[docID]
		switch {
		case !extracted:
			findings = append(findings, finding(docID,
				"%s lists this document (%s) but nothing is extracted under %s, so no rule "+
					"can map it and no fact can cite it",
				sourcesFile, src.File, path.Join(extractedDir, docID)))
		case !listed:
			findings = append(findings, finding(docID,
				"is extracted under %s but no source in %s claims it, so the pages have no "+
					"recorded origin, URL or retrieval date",
				path.Join(extractedDir, docID), sourcesFile))
		default:
			findings = append(findings, compareManifest(docID, src, doc)...)
		}
	}

	return conclusion{
		subjects: len(documents),
		unit:     "documents",
		held: fmt.Sprintf("%d documents, each manifest's source_sha256, source_bytes, "+
			"source_file and page_count agreeing with the %s entry recorded independently "+
			"of it", len(s.Sources), sourcesFile),
		nothing:  "no document is listed in " + sourcesFile + " and none is extracted",
		findings: findings,
	}.result(), nil
}

// compareManifest returns at most one finding, naming every disagreement it
// found: the subject at fault is the document, and three findings about one
// document would read as three documents in the count.
func compareManifest(docID string, src registry.Source, doc *corpus.Doc) []Finding {
	var says []string
	if doc.SourceSHA256() != src.SHA256 {
		says = append(says, fmt.Sprintf("the manifest was made from sha256 %s and the "+
			"registry records %s", short(doc.SourceSHA256()), short(src.SHA256)))
	}
	if doc.SourceBytes() != src.Bytes {
		says = append(says, fmt.Sprintf("the manifest records %d source bytes and the "+
			"registry records %d", doc.SourceBytes(), src.Bytes))
	}
	if doc.SourceFile() != src.File {
		says = append(says, fmt.Sprintf("the manifest was made from %q and the registry "+
			"names %q", doc.SourceFile(), src.File))
	}
	// The page count is a fourth independently recorded claim: extract.py got it
	// from pdfinfo, and the registry's was written down by hand from the document.
	// It is the one of the four that a human could get wrong without any file
	// changing, which is exactly why it is worth comparing.
	if doc.PageCount() != src.Pages {
		says = append(says, fmt.Sprintf("the extractor found %d pages and the registry "+
			"records %d", doc.PageCount(), src.Pages))
	}
	if len(says) == 0 {
		return nil
	}
	return []Finding{finding(docID, "%s; the extraction and %s disagree about which "+
		"document this is, and one of them is describing a file nobody read",
		joinComma(says), sourcesFile)}
}

// union is every key of either map, sorted, so a report over two vocabularies of
// document id covers both and reads in one order.
func union[A, B any](a map[string]A, b map[string]B) []string {
	seen := make(map[string]bool, len(a)+len(b))
	for k := range a {
		seen[k] = true
	}
	for k := range b {
		seen[k] = true
	}
	return sortedStrings(seen)
}

// extractionToolchainPinned asserts every extraction was produced by the
// toolchain this fisc is pinned to.
//
// The versions are provenance, not a read contract. An extraction from another
// poppler build parses fine and reads fine; it simply is not the extraction that
// was reviewed, and poppler's output is not stable across versions — which is why
// requirements.txt's re-extraction workflow is "make extract, review the artifact
// diff, then fisc reanchor --accept" rather than a bump in place. A manifest from
// extractor_version 2 is stronger still: that was xberg, whose table substrate no
// longer exists.
//
// This is why the pin is compared against and mutual agreement between the three
// documents is NOT separately asserted: each manifest is compared to one
// constant, so agreement between them follows, and a check that also reported
// "the documents disagree" would be reporting a consequence of its own first
// finding as a second one.
//
// The manifest's schema_version is not here either, and for the opposite reason:
// corpus.Open refuses a version it does not understand, so an extraction with a
// bumped schema never reaches a check at all. It is stated in the passing summary,
// where it belongs — a reader of the report learns which contract the artifacts
// were read under.
type extractionToolchainPinned struct{}

var _ Check = (*extractionToolchainPinned)(nil)

func (*extractionToolchainPinned) ID() string { return "extraction-toolchain-pinned" }
func (*extractionToolchainPinned) Tier() int  { return 0 }
func (*extractionToolchainPinned) Full() bool { return false }
func (*extractionToolchainPinned) Description() string {
	return fmt.Sprintf("every extraction records extractor_version %d and poppler_version %q, "+
		"the toolchain the committed artifacts were reviewed under",
		corpus.PinnedExtractorVersion, corpus.PinnedPopplerVersion)
}

func (*extractionToolchainPinned) Run(_ context.Context, s *Subject) (Result, error) {
	var findings []Finding
	for _, docID := range sortedStrings(s.Extractions) {
		doc := s.Extractions[docID]
		var says []string
		if doc.ExtractorVersion() != corpus.PinnedExtractorVersion {
			says = append(says, fmt.Sprintf("extractor_version %d, want %d",
				doc.ExtractorVersion(), corpus.PinnedExtractorVersion))
		}
		if doc.PopplerVersion() != corpus.PinnedPopplerVersion {
			says = append(says, fmt.Sprintf("poppler_version %q, want %q",
				doc.PopplerVersion(), corpus.PinnedPopplerVersion))
		}
		if len(says) > 0 {
			findings = append(findings, finding(docID,
				"%s: this extraction was not produced by the pinned toolchain, so its "+
					"artifacts are not the ones that were reviewed (see requirements.txt for "+
					"the re-extraction workflow)", joinComma(says)))
		}
	}

	return conclusion{
		subjects: len(s.Extractions),
		unit:     "extractions",
		held: fmt.Sprintf("%d extractions, all from extractor_version %d with poppler %s, "+
			"read under manifest schema_version %d",
			len(s.Extractions), corpus.PinnedExtractorVersion, corpus.PinnedPopplerVersion,
			corpus.SchemaVersion),
		nothing:  "no extraction is committed under " + extractedDir,
		findings: findings,
	}.result(), nil
}

// sourcePDFsMatchBothRecords asserts the source documents on disk are the ones
// two independent records say they are.
//
// This is the check --full exists for, and the first in this package to need an
// input a plain clone does not have. Three parties are involved: the bytes under
// data/pdf/, the sha256 and size data/sources.yaml records, and the sha256 and size
// each extraction manifest records. The other structural checks compare the two
// records against each other; this one is the only thing in the project that asks
// the bytes.
//
// # Exactly what this adds, and what it does not
//
// It catches a source document whose bytes no longer match what was written down:
// corruption in transit or on disk, a truncated download, a file replaced or swapped
// without either record being updated. That is a real event and nothing else in the
// project can see it — every other check reads the extraction, which was made from
// the old bytes and remains self-consistent with them.
//
// It does NOT establish that the extraction came from the document on disk. Correct
// both records TO the file — the two-line edit a "the city re-uploaded it" cleanup
// would naturally make — and this check passes while every page under
// data/extracted/ is still the old document's. That is demonstrated rather than
// argued, by this package's own tests: repoWithPDFs stands 70-byte stand-in "PDFs"
// beside the real extraction of the real documents, corrects both records to them,
// and this check passes over the result. The bytes are
// hashed but nothing here ties them to the pages: no check in this project derives
// anything from a PDF, because that would mean running the extractor, which is
// deliberately not a Go responsibility. The nearest thing to that witness is
// re-resolving each fact's cell against the pages (fisc-1wr.5.1), and the real one
// is re-extracting and reviewing the artifact diff.
//
// Unavailable bytes are an ERROR and wrong bytes are a FAILURE, which is this
// package's line between a claim about the checker and a claim about the corpus.
// data/pdf/ is Git LFS and is deliberately absent from CI, so "the file is not
// here" is a normal state of a checkout and must not be reported as a corrupted
// document. The consequence, stated because it is a real limitation: in a run where
// one document is unavailable and another's bytes are wrong, the error is what the
// report carries, and the mismatch surfaces on the next run once the fetch is done.
type sourcePDFsMatchBothRecords struct{}

var _ Check = (*sourcePDFsMatchBothRecords)(nil)

func (*sourcePDFsMatchBothRecords) ID() string { return "source-pdfs-match-both-records" }
func (*sourcePDFsMatchBothRecords) Tier() int  { return 0 }
func (*sourcePDFsMatchBothRecords) Full() bool { return true }
func (*sourcePDFsMatchBothRecords) Description() string {
	return "each source document on disk hashes to the sha256 both " + sourcesFile +
		" and its extraction manifest recorded for it"
}

// Run compares what Load hashed. It opens nothing itself: data/pdf/ is read in
// exactly one place (see LoadOptions.Full), so what this check reads is three
// recorded values, like every other check here.
func (*sourcePDFsMatchBothRecords) Run(_ context.Context, s *Subject) (Result, error) {
	var findings []Finding
	var unavailable []string
	// The two records are counted separately, and the check's own name is why. It
	// claims agreement with BOTH, and a document whose extraction is missing has only
	// one record to agree with -- so counting one subject per document would report
	// "3 documents, each matching both" over a document that was compared once. This
	// is factVocabulary's shape: one subject per assertion made, and the summary says
	// how many of each.
	hashed, registryRecords, manifestRecords := 0, 0, 0

	for _, src := range s.Sources {
		pdf, ok := s.SourcePDFs[src.ID]
		if !ok {
			// Load populates one entry per source whenever Full is set, so a gap
			// here is a wiring bug in this package and not a state of the tree.
			return Result{}, fmt.Errorf("no source document was looked for at %s (%s), "+
				"though the run was --full", src.File, src.ID)
		}
		switch pdf.State {
		case SourceMissing:
			unavailable = append(unavailable, fmt.Sprintf("%s is not there", pdf.Path))
		case SourcePointer:
			unavailable = append(unavailable, describePointer(pdf, src))
		case SourcePresent:
			hashed++
			registryRecords++
			doc := s.Extractions[src.ID]
			if doc != nil {
				manifestRecords++
			}
			findings = append(findings, comparePDF(pdf, src, doc)...)
		default:
			// A state this check does not know is a bug in this package, and of the
			// two ways to report one, the way that does not pass silently is the
			// safe one: the alternative is a source document counted as neither
			// checked nor unavailable, which is a vacuous pass.
			return Result{}, fmt.Errorf("the source document at %s (%s) is in state %q, "+
				"which is not one of %q, %q or %q",
				src.File, src.ID, pdf.State, SourcePresent, SourceMissing, SourcePointer)
		}
	}

	if len(unavailable) > 0 {
		// The remediation is in the message rather than in a cmdutil hint: a check's
		// error becomes this result's Summary, and ErrHint.Error() does not carry the
		// hint, so a hint here would be printed by nobody.
		return Result{}, fmt.Errorf("%d of %d source %s could not be read, so there were no "+
			"bytes to hash: %s. The source PDFs are the only thing in this repository held "+
			"in Git LFS — run `git lfs pull` to fetch them, or drop --full, which every "+
			"other check runs without",
			len(unavailable), len(s.Sources),
			plural(len(s.Sources), "document", "documents"), joinComma(unavailable))
	}
	return conclusion{
		subjects: registryRecords + manifestRecords,
		unit:     "records",
		held: fmt.Sprintf("%d records over %d source %s hashed: %d %s %s and %d extraction "+
			"%s, every one of them the hash of the bytes on disk",
			registryRecords+manifestRecords, hashed, plural(hashed, "document", "documents"),
			registryRecords, sourcesFile, plural(registryRecords, "entry", "entries"),
			manifestRecords, plural(manifestRecords, "manifest", "manifests")),
		nothing:  "no source document is listed in " + sourcesFile,
		findings: findings,
	}.result(), nil
}

// describePointer says what an unfetched pointer is, and whether fetching it
// would even produce the recorded document.
//
// The oid comparison is not decoration. The pointer is git's own record of the
// same sha256 the registry claims, so it answers a question the missing bytes
// otherwise leave open: whether `git lfs pull` will reconcile the tree, or
// whether the registry and the committed pointer already disagree and no fetch
// can.
func describePointer(pdf SourcePDF, src registry.Source) string {
	agrees := "which is the sha256 the registry records, so `git lfs pull` fetches the " +
		"recorded document"
	if pdf.PointerOID != src.SHA256 {
		agrees = fmt.Sprintf("which is NOT the sha256 %s records (%s): the committed pointer "+
			"and the registry already disagree, and fetching cannot reconcile them",
			sourcesFile, short(src.SHA256))
	}
	return fmt.Sprintf("%s is an unsmudged Git LFS pointer to %s (%d bytes), %s",
		pdf.Path, short(pdf.PointerOID), pdf.PointerBytes, agrees)
}

// comparePDF returns at most one finding per document, naming every record the
// bytes on disk disagree with.
//
// A document with no extraction is compared against the registry alone, and the
// missing extraction is manifestMatchesSourceRegistry's finding rather than a
// second copy of it here. The caller counts the comparisons this actually makes, so
// a document compared once is not reported as a document compared twice.
func comparePDF(pdf SourcePDF, src registry.Source, doc *corpus.Doc) []Finding {
	var says []string
	if pdf.SHA256 != src.SHA256 {
		says = append(says, fmt.Sprintf("%s records sha256 %s", sourcesFile, short(src.SHA256)))
	}
	if pdf.Bytes != src.Bytes {
		says = append(says, fmt.Sprintf("%s records %d bytes", sourcesFile, src.Bytes))
	}
	if doc != nil {
		if pdf.SHA256 != doc.SourceSHA256() {
			says = append(says, fmt.Sprintf("the extraction was made from sha256 %s",
				short(doc.SourceSHA256())))
		}
		if pdf.Bytes != doc.SourceBytes() {
			says = append(says, fmt.Sprintf("the extraction was made from %d bytes",
				doc.SourceBytes()))
		}
	}
	if len(says) == 0 {
		return nil
	}
	return []Finding{finding(src.ID,
		"%s is %d bytes hashing to %s, but %s; the document on disk is not the one this "+
			"repository describes, and every figure read from it is a figure from a "+
			"different file", pdf.Path, pdf.Bytes, short(pdf.SHA256), joinComma(says))}
}
