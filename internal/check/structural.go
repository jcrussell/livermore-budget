package check

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"path"
	"slices"
	"sort"
	"strings"

	"github.com/jcrussell/livermore-budget/internal/corpus"
	"github.com/jcrussell/livermore-budget/internal/english"
	"github.com/jcrussell/livermore-budget/internal/registry"
)

// The structural checks: tier 0, not about arithmetic. They say whether the
// committed extraction is still the one the rules were written against.
// Read them as a set: the artifact sweep is a bijection that says nothing
// about size, and page_count, itself held to data/sources.yaml, is what does.

// artifactsMatchManifest asserts the committed extraction is the one its manifest
// describes, in both directions: a changed or missing file, and a file present
// that the manifest does not list.
//
// It witnesses drift since extraction and nothing else; a consistent edit to a
// page and its manifest entry passes. manifest.json itself is not swept; its
// source fields, versions and size are held by the other structural checks.
// `fisc reanchor` is not built (fisc-mq4.6).
type artifactsMatchManifest struct{}

func (*artifactsMatchManifest) meta() meta {
	return meta{id: "artifacts-match-manifest", tier: 0,
		description: "every file under " + extractedDir + " hashes to the sha256 and size its " +
			"manifest records, and the manifest lists every file that is there"}
}

// Run hashes every artifact of every extraction. An unreadable file is a
// finding; an unreadable extraction directory is an error, since the walk
// cannot say what is there.
func (*artifactsMatchManifest) Run(_ context.Context, s *Subject) (Result, error) {
	var findings []Finding
	subjects := 0

	for _, docID := range slices.Sorted(maps.Keys(s.Extractions)) {
		doc := s.Extractions[docID]
		listed := doc.Artifacts()
		present, err := extractionFiles(doc.Tree())
		if err != nil {
			return Result{}, fmt.Errorf("read the extraction of %s under %s: %w",
				docID, extractedDir, err)
		}

		for _, rel := range slices.Sorted(maps.Keys(listed)) {
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
				// Unreachable unless the manifest was edited to disagree with
				// itself.
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
			subjects, len(s.Extractions), english.Plural(len(s.Extractions), "extraction", "extractions")),
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
// sha256, streamed.
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

// artifactPath is an artifact's path from the repository root. It concatenates
// rather than path.Join-ing, because Join cleans: a manifest key of
// "pages/../geometry/p0001.json" must be reported as spelled, not as an
// existing readable path.
func artifactPath(docID, rel string) string { return extractedDir + "/" + docID + "/" + rel }

// short abbreviates a sha256 to twelve hex digits for a report line.
func short(sha string) string {
	if len(sha) <= 12 {
		return sha
	}
	return sha[:12] + "…"
}

// extractionEmittedEveryPage asserts each extraction is COMPLETE: two artifacts
// for every page of the document. The artifact sweep is a consistency claim:
// delete pages from both directory and manifest and it still passes.
// page_count, counted by pdfinfo and held to data/sources.yaml by
// manifest-matches-source-registry, is the witness. It counts documents, not
// artifacts, so an emptied extraction is not hidden in an aggregate.
//
// A page poppler returned no text for still has both artifacts; the blank
// count is printed for that reason.
type extractionEmittedEveryPage struct{}

func (*extractionEmittedEveryPage) meta() meta {
	return meta{id: "extraction-emitted-every-page", tier: 0,
		description: "every extraction lists a page text and a word geometry artifact for each of the " +
			"pages its manifest counted, and nothing else"}
}

// Run compares the artifact map against page_count in both directions: a page with
// no artifact, and an artifact for a page the document does not have.
func (*extractionEmittedEveryPage) Run(_ context.Context, s *Subject) (Result, error) {
	var findings []Finding
	var summaries []string

	for _, docID := range slices.Sorted(maps.Keys(s.Extractions)) {
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
		for _, rel := range slices.Sorted(maps.Keys(listed)) {
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
				len(unexpected), english.Plural(len(unexpected), "artifact", "artifacts"), pages,
				strings.Join(capped(unexpected, 5), ", ")))
		}
		if len(says) > 0 {
			findings = append(findings, finding(docID,
				"the manifest counts %d pages and lists %d artifacts: %s. An extraction "+
					"missing pages is one whose manifest agrees with its own directory, so "+
					"nothing else in this report can see it",
				pages, len(listed), strings.Join(says, ", ")))
			continue
		}
		summaries = append(summaries, describeExtraction(docID, pages, doc.BlankPageCount()))
	}

	return conclusion{
		subjects: len(s.Extractions),
		unit:     "extractions",
		held: fmt.Sprintf("%d extractions, each with a page text and a word geometry "+
			"artifact for every page counted: %s",
			len(s.Extractions), strings.Join(summaries, ", ")),
		nothing:  "no extraction is committed under " + extractedDir,
		findings: findings,
	}.result(), nil
}

// describeExtraction is one document's size, for the passing summary, with a
// blank count only when there is one.
func describeExtraction(docID string, pages, blank int) string {
	if blank == 0 {
		return fmt.Sprintf("%s %d pages", docID, pages)
	}
	return fmt.Sprintf("%s %d pages (%d blank)", docID, pages, blank)
}

// describePages renders a list of page numbers for a finding, capped.
func describePages(pages []int, of int) string {
	if len(pages) == of {
		return fmt.Sprintf("any of the %d pages", of)
	}
	out := make([]string, 0, len(pages))
	for _, n := range pages {
		out = append(out, fmt.Sprintf("p%d", n))
	}
	return fmt.Sprintf("%d of %d pages (%s)", len(pages), of, strings.Join(capped(out, 8), ", "))
}

// capped truncates a list for a report line, saying how much it left out.
func capped(items []string, n int) []string {
	if len(items) <= n {
		return items
	}
	return append(items[:n:n], fmt.Sprintf("and %d more", len(items)-n))
}

// extractorReportedNoErrors asserts the manifest records no extraction error.
// It is a second witness beside extraction-emitted-every-page, naming the stage
// and message; that one fires when the extractor's record is silent.
//
// Warnings are not failures: poppler writes free-form stderr and exits 0. They
// are counted into the passing summary.
type extractorReportedNoErrors struct{}

func (*extractorReportedNoErrors) meta() meta {
	return meta{id: "extractor-reported-no-errors", tier: 0,
		description: "no extraction manifest records an extraction error, which is the extractor's own " +
			"account of a page it could not read and did not write"}
}

func (*extractorReportedNoErrors) Run(_ context.Context, s *Subject) (Result, error) {
	var findings []Finding
	warnings, blank := 0, 0

	for _, docID := range slices.Sorted(maps.Keys(s.Extractions)) {
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
			english.Plural(len(errs), "failure", "failures"), strings.Join(capped(says, 5), ", ")))
	}

	return conclusion{
		subjects: len(s.Extractions),
		unit:     "extractions",
		held: fmt.Sprintf("%d extractions, none recording a failure; %d poppler %s and %d "+
			"blank %s across them, which poppler did not treat as fatal and which are "+
			"recorded in each manifest.json for a human to read",
			len(s.Extractions), warnings, english.Plural(warnings, "warning", "warnings"),
			blank, english.Plural(blank, "page", "pages")),
		nothing:  "no extraction is committed under " + extractedDir,
		findings: findings,
	}.result(), nil
}

// manifestMatchesSourceRegistry asserts the extraction and the registry agree
// about the document each extraction came from, and that both name the same
// set of document ids. The agreement is evidence because tools/extract.py
// reads no YAML, so the two records are independent.
type manifestMatchesSourceRegistry struct{}

func (*manifestMatchesSourceRegistry) meta() meta {
	return meta{id: "manifest-matches-source-registry", tier: 0,
		description: "every document is both listed in " + sourcesFile + " and extracted, and each " +
			"manifest records the same source file, size, sha256 and page count the registry does"}
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
	// pdfinfo's page count against the one written by hand in the registry.
	if doc.PageCount() != src.Pages {
		says = append(says, fmt.Sprintf("the extractor found %d pages and the registry "+
			"records %d", doc.PageCount(), src.Pages))
	}
	if len(says) == 0 {
		return nil
	}
	return []Finding{finding(docID, "%s; the extraction and %s disagree about which "+
		"document this is, and one of them is describing a file nobody read",
		strings.Join(says, ", "), sourcesFile)}
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
	return slices.Sorted(maps.Keys(seen))
}

// extractionToolchainPinned asserts every extraction was produced by the
// toolchain this fisc is pinned to. The versions are provenance, not a read
// contract: another poppler build reads fine and is not what was reviewed.
// schema_version is not checked here because corpus.Open refuses an unknown
// one.
type extractionToolchainPinned struct{}

func (*extractionToolchainPinned) meta() meta {
	return meta{id: "extraction-toolchain-pinned", tier: 0,
		description: fmt.Sprintf("every extraction records extractor_version %d and poppler_version %q, "+
			"the toolchain the committed artifacts were reviewed under",
			corpus.PinnedExtractorVersion, corpus.PinnedPopplerVersion)}
}

func (*extractionToolchainPinned) Run(_ context.Context, s *Subject) (Result, error) {
	var findings []Finding
	for _, docID := range slices.Sorted(maps.Keys(s.Extractions)) {
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
					"the re-extraction workflow)", strings.Join(says, ", ")))
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
// two independent records (data/sources.yaml and each manifest) say they are;
// the only check that reads the bytes.
//
// It does not establish that the extraction came from the document on disk:
// correct both records to a different file and it passes (repoWithPDFs shows
// it). Re-resolving each fact against the pages is fisc-1wr.5.1.
//
// Unavailable bytes are an error and wrong bytes a failure, since data/pdf/ is
// absent from CI. An unavailable document masks another's mismatch until the
// next run.
type sourcePDFsMatchBothRecords struct{}

func (*sourcePDFsMatchBothRecords) meta() meta {
	return meta{id: "source-pdfs-match-both-records", tier: 0, full: true,
		description: "each source document on disk hashes to the sha256 both " + sourcesFile +
			" and its extraction manifest recorded for it"}
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
		case sourceMissing:
			unavailable = append(unavailable, fmt.Sprintf("%s is not there", pdf.Path))
		case sourcePointer:
			unavailable = append(unavailable, describePointer(pdf, src))
		case sourcePresent:
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
				src.File, src.ID, pdf.State, sourcePresent, sourceMissing, sourcePointer)
		}
	}

	if len(unavailable) > 0 {
		// The remediation is in the message rather than in a hint: a check's
		// error becomes this result's Summary, and ErrHint.Error() does not carry the
		// hint, so a hint here would be printed by nobody.
		return Result{}, fmt.Errorf("%d of %d source %s could not be read, so there were no "+
			"bytes to hash: %s. The source PDFs are the only thing in this repository held "+
			"in Git LFS — run `git lfs pull` to fetch them, or drop --full, which every "+
			"other check runs without",
			len(unavailable), len(s.Sources),
			english.Plural(len(s.Sources), "document", "documents"), strings.Join(unavailable, ", "))
	}
	return conclusion{
		subjects: registryRecords + manifestRecords,
		unit:     "records",
		held: fmt.Sprintf("%d records over %d source %s hashed: %d %s %s and %d extraction "+
			"%s, every one of them the hash of the bytes on disk",
			registryRecords+manifestRecords, hashed, english.Plural(hashed, "document", "documents"),
			registryRecords, sourcesFile, english.Plural(registryRecords, "entry", "entries"),
			manifestRecords, english.Plural(manifestRecords, "manifest", "manifests")),
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
func describePointer(pdf sourcePDF, src registry.Source) string {
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
func comparePDF(pdf sourcePDF, src registry.Source, doc *corpus.Doc) []Finding {
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
			"different file", pdf.Path, pdf.Bytes, short(pdf.SHA256), strings.Join(says, ", "))}
}
