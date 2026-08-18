package check

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/jcrussell/livermore-budget/internal/corpus"
	"github.com/jcrussell/livermore-budget/internal/registry"
)

// The tests in this file are all of one shape: take a copy of the real repository,
// change one thing on disk, load it through Load, and assert the structural check
// that claims the changed thing reports it.
//
// They are written that way for the reason the whole tier exists. A test that hands
// a check a hand-built Subject with a wrong hash in it proves the comparison, and
// proves nothing about whether any repository can reach that state — and a suite
// made of those is how this project once shipped five checks that could not fail.
// Every mutation below is one a `git status` could show.
//
// Nothing here needs Python, the network, or the 67 MB of PDF in Git LFS. The
// --full tests synthesize their own source documents (see repoWithPDFs), which is
// also the only way to test a corrupted one.

// docWithFacts is the document the committed mapping rules read, and docWithout is
// one that nothing maps. Both are extracted and both are swept, which is the
// difference between Subject.Extractions and Subject.Docs; a test that wants a
// mutation the fact checks cannot see uses the second.
const (
	docWithFacts = testDoc
	docWithout   = "livermore-acfr-fy2025"
)

// repoPath joins a repository-relative slash path onto a root.
func repoPath(root, rel string) string { return filepath.Join(root, filepath.FromSlash(rel)) }

// writeRepoFile writes a repository-relative file, creating parents.
func writeRepoFile(t *testing.T, root, rel string, body []byte) {
	t.Helper()
	p := repoPath(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		t.Fatalf("mkdir for %s: %v", rel, err)
	}
	if err := os.WriteFile(p, body, 0o600); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}

// readRepoFile reads a repository-relative file.
func readRepoFile(t *testing.T, root, rel string) []byte {
	t.Helper()
	b, err := os.ReadFile(repoPath(root, rel))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return b
}

// mutateManifest rewrites one extraction's manifest.json.
//
// It decodes with UseNumber so that page_count and every artifact size survive the
// round trip as the integers they are: decoded into `any` they would become
// float64, and 35648752 written back as 3.5648752e+07 would make the mutation
// under test indistinguishable from a mangled file.
func mutateManifest(t *testing.T, root, docID string, mutate func(map[string]any)) {
	t.Helper()
	rel := artifactPath(docID, corpus.ManifestFile)
	dec := json.NewDecoder(bytes.NewReader(readRepoFile(t, root, rel)))
	dec.UseNumber()
	var man map[string]any
	if err := dec.Decode(&man); err != nil {
		t.Fatalf("decode %s: %v", rel, err)
	}
	mutate(man)
	b, err := json.Marshal(man)
	if err != nil {
		t.Fatalf("marshal %s: %v", rel, err)
	}
	writeRepoFile(t, root, rel, b)
}

// manifestArtifacts is one extraction's artifact records, for a test that needs to
// name a real artifact rather than assume one.
func manifestArtifacts(t *testing.T, root, docID string) map[string]corpus.Artifact {
	t.Helper()
	doc, err := corpus.OpenDoc(root, docID)
	if err != nil {
		t.Fatalf("open the extraction of %s: %v", docID, err)
	}
	return doc.Artifacts()
}

// anArtifactOf names one artifact of a document, chosen deterministically by
// sorting so a failure message is reproducible: the first geometry file.
//
// Geometry rather than a page, and deliberately: no fact cites a geometry artifact,
// so a corruption in one is invisible to every check except the drift sweep. It is
// the FIRST geometry file by sort, p0001, which no rule reads -- geometry for a
// mapped page would also break `fisc build`, and the point of choosing this one is
// that these tests are a claim about THIS check rather than about the report or the
// resolver.
func anArtifactOf(t *testing.T, root, docID string) (string, corpus.Artifact) {
	t.Helper()
	artifacts := manifestArtifacts(t, root, docID)
	for _, rel := range sortedStrings(artifacts) {
		if strings.HasPrefix(rel, "geometry/") {
			return rel, artifacts[rel]
		}
	}
	t.Fatalf("%s lists no geometry artifact, so this test has nothing to corrupt", docID)
	return "", corpus.Artifact{}
}

// TestTheStructuralChecksPassOverTheCommittedCorpus is the positive claim, with the
// numbers this file's mutations move.
//
// The subject count is asserted against the tree itself rather than pinned to a
// literal: every file under data/extracted/ except the three manifests is one
// artifact this check looked at. A check reporting a pass over fewer subjects than
// there are files is a check that swept part of the corpus and said it swept all of
// it, which is the specific dishonesty Result.Subjects exists to prevent.
func TestTheStructuralChecksPassOverTheCommittedCorpus(t *testing.T) {
	root := repoRoot(t)
	s, err := Load(LoadOptions{Root: root, Version: testVersion})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	rep := Run(t.Context(), s, All(), ReportOptions{GeneratedBy: testVersion})

	files := extractedFiles(t, root)
	wantArtifacts := len(files) - len(s.Extractions) // every file but the manifests
	got := resultFor(t, rep, "artifacts-match-manifest")
	if got.Status != StatusPass {
		t.Fatalf("artifacts-match-manifest = %s (%s): %v", got.Status, got.Summary, got.Findings)
	}
	if got.Subjects != wantArtifacts {
		t.Errorf("artifacts-match-manifest looked at %d artifacts, but %s holds %d files "+
			"beside its %d manifests", got.Subjects, extractedDir, wantArtifacts,
			len(s.Extractions))
	}
	for _, tt := range []struct{ id, unit string }{
		{"extraction-emitted-every-page", "extractions"},
		{"extractor-reported-no-errors", "extractions"},
		{"manifest-matches-source-registry", "documents"},
		{"extraction-toolchain-pinned", "extractions"},
	} {
		res := resultFor(t, rep, tt.id)
		if res.Status != StatusPass {
			t.Errorf("%s = %s (%s): %v", tt.id, res.Status, res.Summary, res.Findings)
		}
		if res.Subjects != len(s.Extractions) {
			t.Errorf("%s looked at %d %s, want the %d extracted documents",
				tt.id, res.Subjects, tt.unit, len(s.Extractions))
		}
	}
}

// mutateArtifacts rewrites one extraction's artifact map, which is the half of a
// manifest that is otherwise checkable only against the directory beside it.
func mutateArtifacts(t *testing.T, root, docID string, mutate func(map[string]any)) {
	t.Helper()
	mutateManifest(t, root, docID, func(m map[string]any) {
		artifacts, ok := m["artifacts"].(map[string]any)
		if !ok {
			t.Fatalf("manifest artifacts is %T, want an object", m["artifacts"])
		}
		mutate(artifacts)
	})
}

// dropArtifact removes one artifact from BOTH the manifest and the directory, which
// is the mutation the hash sweep cannot see: the two sides still agree with each
// other perfectly, and the extraction is one page shorter than the document.
func dropArtifact(t *testing.T, root, docID, rel string) {
	t.Helper()
	mutateArtifacts(t, root, docID, func(artifacts map[string]any) {
		if _, ok := artifacts[rel]; !ok {
			t.Fatalf("%s does not list %s, so dropping it proves nothing", docID, rel)
		}
		delete(artifacts, rel)
	})
	if err := os.Remove(repoPath(root, artifactPath(docID, rel))); err != nil {
		t.Fatalf("remove %s: %v", rel, err)
	}
}

// TestAnEmptiedExtractionFails is the hole a bijection cannot see, and it is the
// reason extraction-emitted-every-page exists.
//
// Delete every artifact of an extraction from the directory AND from its manifest,
// and the hash sweep passes: the map and the directory agree, because both are
// empty. Its subject count drops from 1,572 to 1,182 — a number nobody notices
// without diffing two reports — and 195 pages of a published document are gone.
// --strict does not help either, because it is a PASS and not a vacuous result.
//
// This is not a contrived state. It is the shape of the extractor's own documented
// failure mode: a poppler run that fails on a page writes no artifact for it, and
// the manifest it writes afterwards does not list one.
func TestAnEmptiedExtractionFails(t *testing.T) {
	root := repoWithoutPDFs(t)
	before := loadAndRun(t, root)
	if before.Failed() {
		t.Fatal("the unmutated copy already fails")
	}
	pages := resultFor(t, before, "extraction-emitted-every-page").Subjects

	artifacts := manifestArtifacts(t, root, docWithout)
	if len(artifacts) == 0 {
		t.Fatalf("%s lists no artifacts, so this test covers nothing", docWithout)
	}
	for rel := range artifacts {
		if err := os.Remove(repoPath(root, artifactPath(docWithout, rel))); err != nil {
			t.Fatalf("remove %s: %v", rel, err)
		}
	}
	mutateArtifacts(t, root, docWithout, func(m map[string]any) {
		for rel := range m {
			delete(m, rel)
		}
	})

	rep := loadAndRun(t, root)
	if !rep.Failed() {
		t.Error("an extraction emptied from both its directory and its manifest did not fail")
	}
	res := resultFor(t, rep, "extraction-emitted-every-page")
	if res.Status != StatusFail {
		t.Fatalf("extraction-emitted-every-page = %s (%s), want fail", res.Status, res.Summary)
	}
	if len(res.Findings) != 1 || res.Findings[0].Subject != docWithout {
		t.Fatalf("findings = %v, want exactly the emptied extraction", res.Findings)
	}
	for _, want := range []string{"counts 195 pages and lists 0 artifacts", "any of the 195 pages"} {
		if !strings.Contains(res.Findings[0].Detail, want) {
			t.Errorf("finding %q does not contain %q", res.Findings[0].Detail, want)
		}
	}
	// Every extraction is still counted, so the report says three documents were
	// examined and one of them is empty -- not that two were examined.
	if res.Subjects != pages {
		t.Errorf("subjects = %d, want the %d extractions it looked at before", res.Subjects, pages)
	}
	// And the hash sweep is happy, which is the whole point of this test: the two
	// sides of the bijection agree, and only an independent count of the document's
	// pages can see that both are wrong.
	sweep := resultFor(t, rep, "artifacts-match-manifest")
	if sweep.Status != StatusPass {
		t.Errorf("artifacts-match-manifest = %s (%s), want pass: the manifest and the "+
			"directory still agree", sweep.Status, sweep.Summary)
	}
}

// TestAMissingPageFails is the same failure at the scale it actually happens: one
// page, dropped from both sides, the way a poppler failure on page 5 leaves it.
func TestAMissingPageFails(t *testing.T) {
	for _, tt := range []struct {
		name, rel, want string
	}{
		{"the page text", corpus.PagePath(5), "no page text for 1 of 195 pages (p5)"},
		{"the word geometry", corpus.GeometryPath(5),
			"no word geometry for 1 of 195 pages (p5)"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := repoWithoutPDFs(t)
			dropArtifact(t, root, docWithout, tt.rel)

			rep := loadAndRun(t, root)
			res := resultFor(t, rep, "extraction-emitted-every-page")
			if res.Status != StatusFail {
				t.Fatalf("extraction-emitted-every-page = %s (%s), want fail",
					res.Status, res.Summary)
			}
			if len(res.Findings) != 1 || res.Findings[0].Subject != docWithout {
				t.Fatalf("findings = %v, want exactly the short extraction", res.Findings)
			}
			if !strings.Contains(res.Findings[0].Detail, tt.want) {
				t.Errorf("finding %q does not contain %q", res.Findings[0].Detail, tt.want)
			}
			if got := resultFor(t, rep, "artifacts-match-manifest").Status; got != StatusPass {
				t.Errorf("artifacts-match-manifest = %s, want pass", got)
			}
		})
	}
}

// TestAnArtifactForNoPageFails is the other direction: a manifest listing a page the
// document does not have, consistent with the directory, so the sweep passes. It is
// what a re-extraction of a shorter document would leave if extract.py stopped
// clearing its output directories.
func TestAnArtifactForNoPageFails(t *testing.T) {
	root := repoWithoutPDFs(t)
	const stray = "pages/p9999.txt"
	body := []byte("a page the document does not have\n")
	writeRepoFile(t, root, artifactPath(docWithout, stray), body)
	sum := sha256.Sum256(body)
	mutateArtifacts(t, root, docWithout, func(artifacts map[string]any) {
		artifacts[stray] = map[string]any{
			"bytes":  json.Number(fmt.Sprint(len(body))),
			"sha256": hex.EncodeToString(sum[:]),
		}
	})

	rep := loadAndRun(t, root)
	// The sweep cannot see this: the file is listed, and it hashes to what the
	// manifest says.
	if got := resultFor(t, rep, "artifacts-match-manifest").Status; got != StatusPass {
		t.Errorf("artifacts-match-manifest = %s, want pass", got)
	}
	res := resultFor(t, rep, "extraction-emitted-every-page")
	if res.Status != StatusFail {
		t.Fatalf("extraction-emitted-every-page = %s (%s), want fail", res.Status, res.Summary)
	}
	if !strings.Contains(findingDetails(res), "1 artifact for no page of a 195-page document") {
		t.Errorf("findings %v do not report the artifact as belonging to no page", res.Findings)
	}
	if !strings.Contains(findingDetails(res), stray) {
		t.Errorf("findings %v do not name the artifact", res.Findings)
	}
}

// TestZeroingThePageCountIsCaughtByTheRegistry closes the laundering path: page_count
// is what extraction-emitted-every-page compares against, so an attacker who empties
// an extraction would zero it too. It is not self-certifying, and this is the check
// that says so — data/sources.yaml records the same count, written by hand from the
// document, and registry.LoadSources refuses a source with no page count at all.
func TestZeroingThePageCountIsCaughtByTheRegistry(t *testing.T) {
	root := repoWithoutPDFs(t)
	for rel := range manifestArtifacts(t, root, docWithout) {
		if err := os.Remove(repoPath(root, artifactPath(docWithout, rel))); err != nil {
			t.Fatalf("remove %s: %v", rel, err)
		}
	}
	mutateManifest(t, root, docWithout, func(m map[string]any) {
		m["page_count"] = json.Number("0")
		m["artifacts"] = map[string]any{}
	})

	rep := loadAndRun(t, root)
	if !rep.Failed() {
		t.Error("an emptied extraction with a zeroed page count did not fail the run")
	}
	// The completeness check is now satisfied -- zero pages, zero artifacts -- and
	// the registry comparison is what fires.
	res := resultFor(t, rep, "manifest-matches-source-registry")
	if res.Status != StatusFail {
		t.Fatalf("manifest-matches-source-registry = %s (%s), want fail", res.Status, res.Summary)
	}
	if !strings.Contains(findingDetails(res), "the extractor found 0 pages and the registry records 195") {
		t.Errorf("findings %v do not report the page count disagreement", res.Findings)
	}
}

// TestARecordedExtractionErrorFails is fisc-6r5: tools/extract.py records every
// failure in the manifest rather than only logging it, precisely so that verify can
// see it, and until this check existed nothing read the field. A run that failed on
// fifty pages exited 1 and still verified clean.
func TestARecordedExtractionErrorFails(t *testing.T) {
	for _, tt := range []struct {
		name  string
		entry map[string]any
		want  string
	}{{
		name:  "a page poppler gave up on",
		entry: map[string]any{"stage": "layout", "page": json.Number("34"), "message": "pdftotext exited 1"},
		want:  "p34 layout: pdftotext exited 1",
	}, {
		name:  "a document-level failure",
		entry: map[string]any{"stage": "pdfinfo", "page": json.Number("0"), "message": "cannot parse page count"},
		want:  "the document pdfinfo: cannot parse page count",
	}} {
		t.Run(tt.name, func(t *testing.T) {
			root := repoWithoutPDFs(t)
			mutateManifest(t, root, docWithout, func(m map[string]any) {
				m["errors"] = []any{tt.entry}
			})

			rep := loadAndRun(t, root)
			if !rep.Failed() {
				t.Error("a manifest recording an extraction failure did not fail the run")
			}
			res := resultFor(t, rep, "extractor-reported-no-errors")
			if res.Status != StatusFail {
				t.Fatalf("extractor-reported-no-errors = %s (%s), want fail",
					res.Status, res.Summary)
			}
			if len(res.Findings) != 1 || res.Findings[0].Subject != docWithout {
				t.Fatalf("findings = %v, want exactly the failed extraction", res.Findings)
			}
			for _, want := range []string{tt.want, "make extract"} {
				if !strings.Contains(res.Findings[0].Detail, want) {
					t.Errorf("finding %q does not contain %q", res.Findings[0].Detail, want)
				}
			}
			if res.Subjects != 3 {
				t.Errorf("subjects = %d, want the 3 committed extractions", res.Subjects)
			}
		})
	}
}

// TestWarningsAreCountedAndNotFailures is the other half of reading the extractor's
// two channels. Warnings are not failures — poppler exits 0 for a damaged xref and an
// unread metadata key alike, and all three committed extractions carry some — but
// decoding them and then saying nothing is how "reading the warnings is the human's
// job" becomes nobody's. They are counted into the passing summary.
func TestWarningsAreCountedAndNotFailures(t *testing.T) {
	root := repoWithoutPDFs(t)
	before := resultFor(t, loadAndRun(t, root), "extractor-reported-no-errors")
	if before.Status != StatusPass {
		t.Fatalf("extractor-reported-no-errors = %s over the committed corpus, which carries "+
			"warnings and no errors", before.Status)
	}
	if !strings.Contains(before.Summary, "8 poppler warnings and 1 blank page") {
		t.Errorf("summary %q does not count the warnings and blank pages the committed "+
			"manifests record", before.Summary)
	}

	mutateManifest(t, root, docWithFacts, func(m map[string]any) {
		m["warnings"] = []any{map[string]any{
			"stage": "layout", "message": "Syntax Warning: Invalid Font Weight",
			"count": json.Number("7"), "pages": []any{json.Number("12")},
		}}
	})
	res := resultFor(t, loadAndRun(t, root), "extractor-reported-no-errors")
	if res.Status != StatusPass {
		t.Fatalf("extractor-reported-no-errors = %s (%s), want pass: a poppler warning is "+
			"not a failure", res.Status, res.Summary)
	}
	if !strings.Contains(res.Summary, "15 poppler warnings") {
		t.Errorf("summary %q does not count the seven added warnings", res.Summary)
	}
}

// TestAManifestKeyThatIsNotACleanPathIsNamedAsWritten guards the report's honesty
// about paths. corpus.Artifacts hands back manifest keys exactly as the file spells
// them, and a finding has to cite what the file contains: cleaning
// "pages/../geometry/p0001.json" would print an existing, readable path and call it
// unreadable.
func TestAManifestKeyThatIsNotACleanPathIsNamedAsWritten(t *testing.T) {
	root := repoWithoutPDFs(t)
	const key = "pages/../geometry/p0001.json"
	mutateArtifacts(t, root, docWithout, func(artifacts map[string]any) {
		artifacts[key] = map[string]any{
			"bytes": json.Number("1"), "sha256": strings.Repeat("c", 64),
		}
	})

	res := resultFor(t, loadAndRun(t, root), "artifacts-match-manifest")
	if res.Status != StatusFail {
		t.Fatalf("artifacts-match-manifest = %s (%s), want fail", res.Status, res.Summary)
	}
	if len(res.Findings) != 1 {
		t.Fatalf("findings = %v, want one", res.Findings)
	}
	if got, want := res.Findings[0].Subject, artifactPath(docWithout, key); got != want {
		t.Errorf("finding names %q, want the key as the manifest spells it, %q", got, want)
	}
	if !strings.Contains(res.Findings[0].Detail, "cannot be read") {
		t.Errorf("finding %q does not say the path could not be read", res.Findings[0].Detail)
	}
}

// TestAnUnreadableExtractionDirectoryIsAnError covers the one path in the sweep that
// returns an error rather than a finding. A file it cannot read is a state of the
// repository and is reported per file; a directory it cannot LIST means it cannot say
// what is or is not there, which is a claim about the machine and not the corpus.
func TestAnUnreadableExtractionDirectoryIsAnError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root, which ignores the permission bits this test sets")
	}
	root := repoWithoutPDFs(t)
	dir := repoPath(root, extractedDir+"/"+docWithout+"/pages")
	if err := os.Chmod(dir, 0o000); err != nil {
		t.Fatalf("chmod %s: %v", dir, err)
	}
	// Restored so the temporary tree can be cleaned up.
	t.Cleanup(func() { _ = os.Chmod(dir, 0o750) })

	rep := loadAndRun(t, root)
	if !rep.Failed() {
		t.Error("an unreadable extraction directory did not fail the run")
	}
	res := resultFor(t, rep, "artifacts-match-manifest")
	if res.Status != StatusError {
		t.Fatalf("artifacts-match-manifest = %s (%s), want error: the sweep could not "+
			"establish what is in the directory", res.Status, res.Summary)
	}
	if !strings.Contains(res.Summary, docWithout) {
		t.Errorf("summary %q does not name the extraction it could not read", res.Summary)
	}
}

// TestTheCommittedExtractionsAreComplete states the invariant the completeness check
// rests on directly against the tree, rather than only through the check: every
// document's page count equals the number of page artifacts AND the number of
// geometry artifacts. 195/195/195, 268/268/268, 323/323/323.
//
// It is worth asserting separately because it is the premise, not the conclusion. If
// this ever stops being true of a freshly extracted corpus — a substrate that is not
// emitted per page, say — the check above becomes wrong rather than merely red, and
// this is the test that says which of the two happened.
func TestTheCommittedExtractionsAreComplete(t *testing.T) {
	root := repoRoot(t)
	s, err := Load(LoadOptions{Root: root, Version: testVersion})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(s.Extractions) != 3 {
		t.Fatalf("%d extractions loaded, want the 3 committed documents", len(s.Extractions))
	}
	for _, docID := range sortedStrings(s.Extractions) {
		doc := s.Extractions[docID]
		text, geometry, other := 0, 0, 0
		for rel := range doc.Artifacts() {
			switch {
			case strings.HasPrefix(rel, "pages/"):
				text++
			case strings.HasPrefix(rel, "geometry/"):
				geometry++
			default:
				other++
			}
		}
		if pages := doc.PageCount(); text != pages || geometry != pages {
			t.Errorf("%s: %d pages, %d page artifacts, %d geometry artifacts", docID,
				pages, text, geometry)
		}
		if other != 0 {
			t.Errorf("%s lists %d artifacts in neither namespace", docID, other)
		}
		if len(doc.Errors()) != 0 {
			t.Errorf("%s records %d extraction errors", docID, len(doc.Errors()))
		}
	}
}

// TestACorruptedArtifactFails is the drift this tier is named for: one byte of one
// committed artifact, changed, with the manifest untouched.
func TestACorruptedArtifactFails(t *testing.T) {
	root := repoWithoutPDFs(t)
	rel, want := anArtifactOf(t, root, docWithFacts)
	body := readRepoFile(t, root, artifactPath(docWithFacts, rel))
	body[len(body)/2]++ // one byte, in the middle, same length
	writeRepoFile(t, root, artifactPath(docWithFacts, rel), body)

	rep := loadAndRun(t, root)
	if !rep.Failed() {
		t.Error("a corrupted extraction artifact did not fail the run")
	}
	res := resultFor(t, rep, "artifacts-match-manifest")
	if res.Status != StatusFail {
		t.Fatalf("artifacts-match-manifest = %s (%s), want fail", res.Status, res.Summary)
	}
	if len(res.Findings) != 1 {
		t.Fatalf("findings = %v, want exactly the corrupted artifact", res.Findings)
	}
	if got := res.Findings[0].Subject; got != artifactPath(docWithFacts, rel) {
		t.Errorf("finding names %q, want the corrupted artifact", got)
	}
	// The finding has to carry both hashes: "something changed" is not actionable,
	// and the manifest's value is what a reader compares against.
	if !strings.Contains(res.Findings[0].Detail, short(want.SHA256)) {
		t.Errorf("finding %q does not name the sha256 the manifest records",
			res.Findings[0].Detail)
	}
	// And no other check noticed, which is the point of the sweep: no fact cites a
	// geometry artifact, so nothing else in the report reads this file at all.
	if got := resultFor(t, rep, "fact-offset-points-at-token").Status; got != StatusPass {
		t.Errorf("fact-offset-points-at-token = %s; this corruption is invisible to it, "+
			"which is why the drift sweep exists", got)
	}
}

// TestAnEditedManifestHashFails is the same mismatch from the other side: the
// artifact is untouched and the record of it is not. It is the likelier mistake of
// the two — a hand-edited manifest is one keystroke, a corrupted page is an event —
// and neither side can be trusted to police itself.
func TestAnEditedManifestHashFails(t *testing.T) {
	root := repoWithoutPDFs(t)
	rel, want := anArtifactOf(t, root, docWithFacts)
	const wrong = "0000000000000000000000000000000000000000000000000000000000000000"
	mutateManifest(t, root, docWithFacts, func(m map[string]any) {
		artifacts, ok := m["artifacts"].(map[string]any)
		if !ok {
			t.Fatalf("manifest artifacts is %T, want an object", m["artifacts"])
		}
		entry, ok := artifacts[rel].(map[string]any)
		if !ok {
			t.Fatalf("manifest entry for %s is %T, want an object", rel, artifacts[rel])
		}
		entry["sha256"] = wrong
	})

	res := resultFor(t, loadAndRun(t, root), "artifacts-match-manifest")
	if res.Status != StatusFail {
		t.Fatalf("artifacts-match-manifest = %s (%s), want fail", res.Status, res.Summary)
	}
	detail := findingDetails(res)
	for _, s := range []string{short(want.SHA256), short(wrong)} {
		if !strings.Contains(detail, s) {
			t.Errorf("findings %v do not name %s", res.Findings, s)
		}
	}
}

// TestAnUnlistedArtifactFails is the direction that is easy to leave out, and it is
// the one that lets a file be read that nothing vouches for. The manifest is the
// authority on what was extracted; a file beside it that it does not name has no
// recorded origin, and a future rule reading it would produce provenance that
// resolves to bytes nobody hashed.
func TestAnUnlistedArtifactFails(t *testing.T) {
	root := repoWithoutPDFs(t)
	before := resultFor(t, loadAndRun(t, root), "artifacts-match-manifest")
	if before.Status != StatusPass {
		t.Fatalf("the unmutated copy is already %s (%s)", before.Status, before.Summary)
	}

	const stray = "pages/p9999.txt"
	writeRepoFile(t, root, artifactPath(docWithout, stray), []byte("a page nothing extracted\n"))

	res := resultFor(t, loadAndRun(t, root), "artifacts-match-manifest")
	if res.Status != StatusFail {
		t.Fatalf("artifacts-match-manifest = %s (%s), want fail", res.Status, res.Summary)
	}
	if len(res.Findings) != 1 || res.Findings[0].Subject != artifactPath(docWithout, stray) {
		t.Fatalf("findings = %v, want exactly the unlisted file", res.Findings)
	}
	if !strings.Contains(res.Findings[0].Detail, "does not list it") {
		t.Errorf("finding %q does not say the manifest never listed the file",
			res.Findings[0].Detail)
	}
	// An extra file is one more thing looked at, not the same sweep with a
	// complaint attached.
	if res.Subjects != before.Subjects+1 {
		t.Errorf("subjects = %d after adding a file, want %d", res.Subjects, before.Subjects+1)
	}
}

// TestADeletedArtifactFails covers the third state: the manifest lists it and it is
// gone. corpus.Doc reports a missing page as a read error at the point a rule asks
// for it, which is a failure of whatever was being built; this is what says so about
// the whole corpus, including the 1,300 artifacts no rule reads yet.
func TestADeletedArtifactFails(t *testing.T) {
	root := repoWithoutPDFs(t)
	rel, want := anArtifactOf(t, root, docWithout)
	if err := os.Remove(repoPath(root, artifactPath(docWithout, rel))); err != nil {
		t.Fatalf("remove %s: %v", rel, err)
	}

	res := resultFor(t, loadAndRun(t, root), "artifacts-match-manifest")
	if res.Status != StatusFail {
		t.Fatalf("artifacts-match-manifest = %s (%s), want fail", res.Status, res.Summary)
	}
	if len(res.Findings) != 1 || res.Findings[0].Subject != artifactPath(docWithout, rel) {
		t.Fatalf("findings = %v, want exactly the deleted artifact", res.Findings)
	}
	for _, s := range []string{"cannot be read", short(want.SHA256)} {
		if !strings.Contains(res.Findings[0].Detail, s) {
			t.Errorf("finding %q does not contain %q", res.Findings[0].Detail, s)
		}
	}
}

// TestAManifestSkewedFromTheRegistryFails is the cross-check that needs no PDF: two
// parties recorded the same three claims about one document, and this is what
// happens when they stop agreeing. Each field is its own case, because each says a
// different thing about what went wrong.
func TestAManifestSkewedFromTheRegistryFails(t *testing.T) {
	const wrongSHA = "1111111111111111111111111111111111111111111111111111111111111111"
	tests := []struct {
		name  string
		key   string
		value any
		want  string
	}{
		{"a skewed source hash", "source_sha256", wrongSHA, short(wrongSHA)},
		{"a skewed source size", "source_bytes", json.Number("123"), "123 source bytes"},
		{"a different source file", "source_file", "data/pdf/somewhere-else.pdf",
			"somewhere-else.pdf"},
		{"a page count the registry disagrees with", "page_count", json.Number("267"),
			"the extractor found 267 pages"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := repoWithoutPDFs(t)
			mutateManifest(t, root, docWithFacts, func(m map[string]any) {
				m[tt.key] = tt.value
			})

			rep := loadAndRun(t, root)
			if !rep.Failed() {
				t.Error("a manifest that disagrees with the source registry did not fail the run")
			}
			res := resultFor(t, rep, "manifest-matches-source-registry")
			if res.Status != StatusFail {
				t.Fatalf("manifest-matches-source-registry = %s (%s), want fail",
					res.Status, res.Summary)
			}
			if len(res.Findings) != 1 || res.Findings[0].Subject != docWithFacts {
				t.Fatalf("findings = %v, want exactly the skewed document", res.Findings)
			}
			if !strings.Contains(res.Findings[0].Detail, tt.want) {
				t.Errorf("finding %q does not contain %q", res.Findings[0].Detail, tt.want)
			}
			// The artifacts still hash correctly: this is a claim about which
			// document the extraction came from, not about its contents.
			if got := resultFor(t, rep, "artifacts-match-manifest").Status; got != StatusPass {
				t.Errorf("artifacts-match-manifest = %s, want pass: no artifact was touched", got)
			}
		})
	}
}

// TestASourceWithNoExtractionFails covers one half of the set comparison. A
// document the registry lists and nobody has extracted is a document the site can
// cite, with a URL and a retrieval date, that no fact can ever come from.
func TestASourceWithNoExtractionFails(t *testing.T) {
	root := repoWithoutPDFs(t)
	if err := os.RemoveAll(repoPath(root, extractedDir+"/"+docWithout)); err != nil {
		t.Fatalf("remove the extraction of %s: %v", docWithout, err)
	}

	rep := loadAndRun(t, root)
	res := resultFor(t, rep, "manifest-matches-source-registry")
	if res.Status != StatusFail {
		t.Fatalf("manifest-matches-source-registry = %s (%s), want fail", res.Status, res.Summary)
	}
	if len(res.Findings) != 1 || res.Findings[0].Subject != docWithout {
		t.Fatalf("findings = %v, want exactly the unextracted document", res.Findings)
	}
	if !strings.Contains(res.Findings[0].Detail, "nothing is extracted") {
		t.Errorf("finding %q does not say the extraction is absent", res.Findings[0].Detail)
	}
	// Two documents remain and both are still swept: the missing one is reported
	// once, by the check that compares the two sets, and does not silently shrink
	// what everything else claims to have looked at.
	if got := resultFor(t, rep, "extraction-toolchain-pinned").Subjects; got != 2 {
		t.Errorf("extraction-toolchain-pinned looked at %d extractions, want the 2 left", got)
	}
}

// TestAnExtractionWithNoSourceFails is the other half, and the more dangerous one:
// 15 MB of pages with no URL, no retrieval date and no recorded hash is the
// provenance chain broken at its first link. A renamed document leaves exactly this
// behind.
func TestAnExtractionWithNoSourceFails(t *testing.T) {
	root := repoWithoutPDFs(t)
	const orphan = "livermore-budget-fy2024-2025"
	writeSyntheticExtraction(t, root, orphan, nil)

	res := resultFor(t, loadAndRun(t, root), "manifest-matches-source-registry")
	if res.Status != StatusFail {
		t.Fatalf("manifest-matches-source-registry = %s (%s), want fail", res.Status, res.Summary)
	}
	if len(res.Findings) != 1 || res.Findings[0].Subject != orphan {
		t.Fatalf("findings = %v, want exactly the unclaimed extraction", res.Findings)
	}
	if !strings.Contains(res.Findings[0].Detail, "no source in "+sourcesFile) {
		t.Errorf("finding %q does not say the registry never claimed it",
			res.Findings[0].Detail)
	}
}

// writeSyntheticExtraction lays down a valid, empty extraction directory whose
// manifest names the directory and the pinned toolchain, so a test can add a
// document to a tree without inventing artifacts. overrides is applied last, which
// is how a test makes exactly one field wrong.
func writeSyntheticExtraction(t *testing.T, root, docID string, overrides map[string]any) {
	t.Helper()
	man := map[string]any{
		"schema_version":    corpus.SchemaVersion,
		"doc_id":            docID,
		"page_count":        0,
		"extractor_version": corpus.PinnedExtractorVersion,
		"poppler_version":   corpus.PinnedPopplerVersion,
		"source_file":       "data/pdf/" + docID + ".pdf",
		"source_sha256":     strings.Repeat("a", 64),
		"source_bytes":      1,
		"artifacts":         map[string]corpus.Artifact{},
	}
	for k, v := range overrides {
		man[k] = v
	}
	b, err := json.Marshal(man)
	if err != nil {
		t.Fatalf("marshal a synthetic manifest: %v", err)
	}
	writeRepoFile(t, root, artifactPath(docID, corpus.ManifestFile), b)
}

// TestAnUnpinnedToolchainFails: the artifacts are intact, the registry agrees, and
// the extraction was made by something else. poppler's output is not stable across
// versions, so this is the difference between the artifacts that were reviewed and
// artifacts that merely parse.
func TestAnUnpinnedToolchainFails(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value any
		want  string
	}{
		{"a bumped extractor", "extractor_version", json.Number("4"), "extractor_version 4"},
		{"the xberg extractor", "extractor_version", json.Number("2"), "extractor_version 2"},
		{"another poppler", "poppler_version", "25.03.1", `poppler_version "25.03.1"`},
		{"no poppler recorded", "poppler_version", "", `poppler_version ""`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := repoWithoutPDFs(t)
			mutateManifest(t, root, docWithout, func(m map[string]any) { m[tt.key] = tt.value })

			rep := loadAndRun(t, root)
			if !rep.Failed() {
				t.Error("an extraction from an unpinned toolchain did not fail the run")
			}
			res := resultFor(t, rep, "extraction-toolchain-pinned")
			if res.Status != StatusFail {
				t.Fatalf("extraction-toolchain-pinned = %s (%s), want fail",
					res.Status, res.Summary)
			}
			if len(res.Findings) != 1 || res.Findings[0].Subject != docWithout {
				t.Fatalf("findings = %v, want exactly the unpinned extraction", res.Findings)
			}
			if !strings.Contains(res.Findings[0].Detail, tt.want) {
				t.Errorf("finding %q does not contain %q", res.Findings[0].Detail, tt.want)
			}
			// Every extraction is still counted: the check looked at three and
			// found one wrong, which is not the same report as looking at one.
			if res.Subjects != 3 {
				t.Errorf("subjects = %d, want the 3 committed extractions", res.Subjects)
			}
		})
	}
}

// TestAnUnreadableManifestSchemaRefusesToLoad records the line between a finding
// and a load failure, which the toolchain check's doc comment depends on.
//
// A schema_version this fisc does not know means the artifact namespace itself may
// be different — at version 1 the pages were .md and there was a tables/ directory —
// so there is no verdict to reach about that extraction, only a statement that the
// reader cannot read it. That is a failure of the harness, and internal/corpus,
// internal/registry and internal/mapping all draw it in the same place.
func TestAnUnreadableManifestSchemaRefusesToLoad(t *testing.T) {
	for _, version := range []json.Number{"1", "3"} {
		t.Run("schema "+string(version), func(t *testing.T) {
			root := repoWithoutPDFs(t)
			mutateManifest(t, root, docWithout, func(m map[string]any) {
				m["schema_version"] = version
			})

			_, err := Load(LoadOptions{Root: root, Version: testVersion})
			if err == nil {
				t.Fatal("Load = nil error over a manifest schema it does not understand")
			}
			for _, want := range []string{"schema_version", docWithout} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not contain %q", err, want)
				}
			}
		})
	}
}

// --------------------------------------------------------------------------
// --full: the source documents
// --------------------------------------------------------------------------

// fakePDF is what these tests put under data/pdf/ in place of a 35 MB document.
//
// It begins "%PDF-" because that is the one thing about a real PDF that matters
// here: it is what an LFS pointer is told apart from. Nothing in fisc parses a PDF —
// the extractor is poppler and it does not run in a test — so the bytes only have to
// be bytes with a recorded hash.
func fakePDF(id string) []byte {
	return []byte("%PDF-1.7\n% a stand-in for " + id + ", so no test needs Git LFS\n%%EOF\n")
}

// repoWithPDFs is a tree --full can pass over: repoWithoutPDFs plus a synthesized
// source document per registry entry, with data/sources.yaml and every manifest
// rewritten to record the hash and size of the bytes actually written.
//
// The real documents are 67 MB in Git LFS. A test that read them would be a test
// that fails on a plain clone, and it could not test a corrupted document at all
// without corrupting the checkout. Synthesizing them instead means the three
// parties this check compares — the bytes, the registry and the manifest — are all
// under the test's control, so any one of them can be made to disagree.
//
// What is NOT rewritten is anything else the registry says about the document: it
// still records 268 pages for a 70-byte file. Nothing reads a PDF's page count —
// the manifest's page_count came from the extractor, and the extraction is the real
// one — so leaving it is honest about what these tests cover and what they do not.
func repoWithPDFs(t *testing.T) string {
	t.Helper()
	root := repoWithoutPDFs(t)
	sources, err := registry.LoadSources(os.DirFS(repoPath(root, dataDir)))
	if err != nil {
		t.Fatalf("load the source registry: %v", err)
	}

	yaml := string(readRepoFile(t, root, sourcesFile))
	for _, src := range sources {
		body := fakePDF(src.ID)
		writeRepoFile(t, root, src.File, body)
		sum := sha256.Sum256(body)
		sha := hex.EncodeToString(sum[:])

		yaml = replaceOnce(t, yaml, src.SHA256, sha)
		yaml = replaceOnce(t, yaml,
			fmt.Sprintf("bytes: %d", src.Bytes), fmt.Sprintf("bytes: %d", len(body)))
		mutateManifest(t, root, src.ID, func(m map[string]any) {
			m["source_sha256"] = sha
			m["source_bytes"] = json.Number(fmt.Sprint(len(body)))
		})
	}
	writeRepoFile(t, root, sourcesFile, []byte(yaml))
	return root
}

// replaceOnce substitutes exactly one occurrence and fails if there was not exactly
// one, so a helper that quietly stopped rewriting the registry cannot leave the
// tests it supports passing for the wrong reason.
func replaceOnce(t *testing.T, s, old, new string) string {
	t.Helper()
	if n := strings.Count(s, old); n != 1 {
		t.Fatalf("%s contains %q %d times, want exactly 1", sourcesFile, old, n)
	}
	return strings.Replace(s, old, new, 1)
}

// loadAndRunFull loads a tree with the --full inputs and runs every check.
func loadAndRunFull(t *testing.T, root string) *Report {
	t.Helper()
	s, err := Load(LoadOptions{Root: root, Version: testVersion, Full: true})
	if err != nil {
		t.Fatalf("Load --full: %v", err)
	}
	if !s.Full {
		t.Fatal("the subject does not report that --full was given")
	}
	return Run(t.Context(), s, All(), ReportOptions{GeneratedBy: testVersion})
}

// TestFullHashesEverySourceDocument is the pass case, and it is also what makes
// SKIPPED reachable: this check is the first in the report to need an input --full
// supplies, so this is the run in which it stops being skipped and does something.
func TestFullHashesEverySourceDocument(t *testing.T) {
	root := repoWithPDFs(t)
	rep := loadAndRunFull(t, root)
	if rep.Counts.Fail > 0 || rep.Counts.Error > 0 {
		t.Errorf("counts = %+v over a consistent tree, want no failure or error:\n%v",
			rep.Counts, rep.Results)
	}
	if rep.Counts.Skipped != 0 {
		t.Errorf("counts = %+v under --full, want nothing skipped", rep.Counts)
	}
	res := resultFor(t, rep, "source-pdfs-match-both-records")
	if res.Status != StatusPass {
		t.Fatalf("source-pdfs-match-both-records = %s (%s): %v",
			res.Status, res.Summary, res.Findings)
	}
	// Six records over three documents: each hashed file is compared against two
	// independently written claims, and both comparisons are counted.
	// TestOnlyTheRecordsThatExistAreCounted is what holds that honest.
	if res.Subjects != 6 {
		t.Errorf("subjects = %d, want 6 records over the 3 source documents", res.Subjects)
	}
}

// TestACorruptedSourceDocumentFails is the check's reason for existing. The two
// written records still agree with each other — so the three checks that need no PDF
// all pass — and the bytes they describe are not the bytes on disk. Nothing but
// hashing the file can see it.
func TestACorruptedSourceDocumentFails(t *testing.T) {
	root := repoWithPDFs(t)
	rel := "data/pdf/" + docWithFacts + ".pdf"
	body := readRepoFile(t, root, rel)
	body[len(body)/2]++
	writeRepoFile(t, root, rel, body)

	rep := loadAndRunFull(t, root)
	if !rep.Failed() {
		t.Error("a corrupted source document did not fail the run")
	}
	res := resultFor(t, rep, "source-pdfs-match-both-records")
	if res.Status != StatusFail {
		t.Fatalf("source-pdfs-match-both-records = %s (%s), want fail", res.Status, res.Summary)
	}
	if len(res.Findings) != 1 || res.Findings[0].Subject != docWithFacts {
		t.Fatalf("findings = %v, want exactly the corrupted document", res.Findings)
	}
	// Both records are named, because both are now wrong about the file and a
	// reader has to know that neither of them is the odd one out.
	for _, want := range []string{sourcesFile, "the extraction was made from sha256"} {
		if !strings.Contains(res.Findings[0].Detail, want) {
			t.Errorf("finding %q does not mention %q", res.Findings[0].Detail, want)
		}
	}
	if got := resultFor(t, rep, "manifest-matches-source-registry").Status; got != StatusPass {
		t.Errorf("manifest-matches-source-registry = %s, want pass: the two written records "+
			"still agree with each other, which is why this check has to read the bytes", got)
	}
}

// TestATruncatedSourceDocumentFails covers comparePDF's size comparison, which a
// corruption test cannot reach: flipping a byte preserves the length, so the finding
// only ever names the hashes. A truncated download is the likelier real event of the
// two, and the size is what says so in one number.
func TestATruncatedSourceDocumentFails(t *testing.T) {
	root := repoWithPDFs(t)
	rel := "data/pdf/" + docWithFacts + ".pdf"
	body := readRepoFile(t, root, rel)
	writeRepoFile(t, root, rel, body[:len(body)/2])

	res := resultFor(t, loadAndRunFull(t, root), "source-pdfs-match-both-records")
	if res.Status != StatusFail {
		t.Fatalf("source-pdfs-match-both-records = %s (%s), want fail", res.Status, res.Summary)
	}
	if len(res.Findings) != 1 || res.Findings[0].Subject != docWithFacts {
		t.Fatalf("findings = %v, want exactly the truncated document", res.Findings)
	}
	// Both size records are named, and both hashes: four disagreements over one file.
	for _, want := range []string{
		fmt.Sprintf("records %d bytes", len(body)),
		fmt.Sprintf("was made from %d bytes", len(body)),
		fmt.Sprintf("is %d bytes hashing to", len(body)/2),
	} {
		if !strings.Contains(res.Findings[0].Detail, want) {
			t.Errorf("finding %q does not contain %q", res.Findings[0].Detail, want)
		}
	}
}

// TestOnlyTheRecordsThatExistAreCounted is must-fix 4 from review: this check claims
// agreement with BOTH records, and a document whose extraction is missing has only
// one. Counting one subject per document would print "3 documents, each matching
// both" over a document that was compared once.
func TestOnlyTheRecordsThatExistAreCounted(t *testing.T) {
	root := repoWithPDFs(t)
	full := resultFor(t, loadAndRunFull(t, root), "source-pdfs-match-both-records")
	if full.Subjects != 6 {
		t.Errorf("subjects = %d over 3 documents with 3 manifests, want 6 records", full.Subjects)
	}
	if !strings.Contains(full.Summary, "3 data/sources.yaml entries and 3 extraction manifests") {
		t.Errorf("summary %q does not split the two records", full.Summary)
	}

	if err := os.RemoveAll(repoPath(root, extractedDir+"/"+docWithout)); err != nil {
		t.Fatalf("remove the extraction of %s: %v", docWithout, err)
	}
	res := resultFor(t, loadAndRunFull(t, root), "source-pdfs-match-both-records")
	if res.Status != StatusPass {
		t.Fatalf("source-pdfs-match-both-records = %s (%s), want pass: every record that "+
			"exists still matches", res.Status, res.Summary)
	}
	if res.Subjects != 5 {
		t.Errorf("subjects = %d with one extraction removed, want 5 records", res.Subjects)
	}
	if !strings.Contains(res.Summary, "3 data/sources.yaml entries and 2 extraction manifests") {
		t.Errorf("summary %q claims a manifest comparison it did not make", res.Summary)
	}
}

// TestAnUnfetchedPointerIsAnErrorAndSaysHowToFetchIt is the case a plain clone is
// in, and the one it would be easiest to get wrong. data/pdf/** is the only thing in
// this repository held in Git LFS, and a checkout without it has POINTER TEXT where
// the document should be — not a missing file. Reporting that as a corrupted
// document would be a false alarm on every developer machine that has not run `git
// lfs pull`, and in CI, which skips the smudge filter deliberately.
//
// So it is an ERROR: the checker could not reach a verdict, which is a different
// column of the report from a corpus that is wrong. And the message carries the
// command, because the fix is one command.
func TestAnUnfetchedPointerIsAnErrorAndSaysHowToFetchIt(t *testing.T) {
	root := repoWithPDFs(t)
	sources, err := registry.LoadSources(os.DirFS(repoPath(root, dataDir)))
	if err != nil {
		t.Fatalf("load the source registry: %v", err)
	}
	var pointerFor registry.Source
	for _, src := range sources {
		if src.ID == docWithFacts {
			pointerFor = src
		}
	}
	writeRepoFile(t, root, pointerFor.File, lfsPointer(pointerFor.SHA256, pointerFor.Bytes))

	rep := loadAndRunFull(t, root)
	if !rep.Failed() {
		t.Error("a run whose --full input was never fetched did not fail")
	}
	res := resultFor(t, rep, "source-pdfs-match-both-records")
	if res.Status != StatusError {
		t.Fatalf("source-pdfs-match-both-records = %s (%s), want error: unavailable bytes "+
			"are a claim about the checkout, not about the corpus", res.Status, res.Summary)
	}
	for _, want := range []string{
		"Git LFS pointer",
		"git lfs pull",
		pointerFor.File,
		// The pointer records the same hash the registry does, so fetching will
		// produce the recorded document — which is worth saying, because the
		// alternative is a tree no fetch can reconcile.
		"which is the sha256 the registry records",
	} {
		if !strings.Contains(res.Summary, want) {
			t.Errorf("summary %q does not contain %q", res.Summary, want)
		}
	}
	// It is an error and not a failure, and nothing else in the report moved: the
	// committed artifacts are all still there and still hash correctly.
	if rep.Counts.Fail != 0 {
		t.Errorf("counts = %+v, want the unfetched pointer in the error column alone",
			rep.Counts)
	}
}

// TestAPointerThatDisagreesWithTheRegistrySaysSo distinguishes two unfetched trees
// that look identical: one where `git lfs pull` produces the recorded document, and
// one where the committed pointer names a different file than the registry does, so
// no fetch can reconcile them. Both are errors; only one of them is fixable by
// fetching.
func TestAPointerThatDisagreesWithTheRegistrySaysSo(t *testing.T) {
	root := repoWithPDFs(t)
	rel := "data/pdf/" + docWithFacts + ".pdf"
	const other = "2222222222222222222222222222222222222222222222222222222222222222"
	writeRepoFile(t, root, rel, lfsPointer(other, 999))

	res := resultFor(t, loadAndRunFull(t, root), "source-pdfs-match-both-records")
	if res.Status != StatusError {
		t.Fatalf("source-pdfs-match-both-records = %s (%s), want error", res.Status, res.Summary)
	}
	for _, want := range []string{short(other), "already disagree"} {
		if !strings.Contains(res.Summary, want) {
			t.Errorf("summary %q does not contain %q", res.Summary, want)
		}
	}
}

// TestAMissingSourceDocumentIsAnError is the third state, and it is a different
// thing to do about it: a pointer needs fetching, an absent file needs restoring.
func TestAMissingSourceDocumentIsAnError(t *testing.T) {
	root := repoWithPDFs(t)
	rel := "data/pdf/" + docWithout + ".pdf"
	if err := os.Remove(repoPath(root, rel)); err != nil {
		t.Fatalf("remove %s: %v", rel, err)
	}

	res := resultFor(t, loadAndRunFull(t, root), "source-pdfs-match-both-records")
	if res.Status != StatusError {
		t.Fatalf("source-pdfs-match-both-records = %s (%s), want error", res.Status, res.Summary)
	}
	if !strings.Contains(res.Summary, rel+" is not there") {
		t.Errorf("summary %q does not say the file is absent", res.Summary)
	}
	if strings.Contains(res.Summary, "pointer") {
		t.Errorf("summary %q calls an absent file a pointer", res.Summary)
	}
}

// lfsPointer renders a Git LFS v1 pointer, in the byte-for-byte shape this
// repository's own committed pointers have — three lines, LF-terminated, oid before
// size, 130-odd bytes total. Taken from `git cat-file -p HEAD:data/pdf/...`, not
// guessed: the sentinel that tells this from a PDF is the first line, and a test
// that invented its own spelling would prove nothing about the file git writes.
func lfsPointer(oid string, size int64) []byte {
	return []byte(fmt.Sprintf("version https://git-lfs.github.com/spec/v1\noid sha256:%s\nsize %d\n",
		oid, size))
}

// TestParseLFSPointer is the sentinel on its own, over the shapes it has to tell
// apart. A false positive here would report a real document as unfetched; a false
// negative would hash 130 bytes of pointer text and report the document as
// corrupted.
func TestParseLFSPointer(t *testing.T) {
	const oid = "1176ab87130ba5ddc73f13bf97e8d9d44f1dc049d16334a124afe00fdfbdd1e8"
	tests := []struct {
		name     string
		body     []byte
		wantOK   bool
		wantOID  string
		wantSize int64
	}{
		{"this repository's own pointer", lfsPointer(oid, 4557225), true, oid, 4557225},
		{"a PDF", fakePDF("x"), false, "", 0},
		{"empty", nil, false, "", 0},
		{"a text file that mentions the spec url", []byte(
			"see version https://git-lfs.github.com/spec/v1 for the format\n"), false, "", 0},
		// A pointer whose keys are unreadable is still a pointer: the first line
		// decides, and reporting empty values beats hashing it as a document.
		{"a truncated pointer", []byte("version https://git-lfs.github.com/spec/v1\noid sha256:"),
			true, "", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oid, size, ok := parseLFSPointer(tt.body)
			if got := []any{ok, oid, size}; !cmp.Equal(got,
				[]any{tt.wantOK, tt.wantOID, tt.wantSize}) {
				t.Errorf("parseLFSPointer = (%q, %d, %v), want (%q, %d, %v)",
					oid, size, ok, tt.wantOID, tt.wantSize, tt.wantOK)
			}
		})
	}
}

// TestReadSourcePDFClassifiesWhatItFinds covers the loader that decides which of the
// three states a file is in, over a tree it can be pointed at directly.
func TestReadSourcePDFClassifiesWhatItFinds(t *testing.T) {
	root := t.TempDir()
	body := fakePDF("probe")
	sum := sha256.Sum256(body)
	writeRepoFile(t, root, "data/pdf/probe.pdf", body)
	writeRepoFile(t, root, "data/pdf/pointer.pdf", lfsPointer(strings.Repeat("b", 64), 12))

	want := map[string]SourcePDF{
		"data/pdf/probe.pdf": {
			Path: "data/pdf/probe.pdf", State: SourcePresent,
			Bytes: int64(len(body)), SHA256: hex.EncodeToString(sum[:]),
		},
		"data/pdf/pointer.pdf": {
			Path: "data/pdf/pointer.pdf", State: SourcePointer,
			PointerOID: strings.Repeat("b", 64), PointerBytes: 12,
		},
		"data/pdf/absent.pdf": {Path: "data/pdf/absent.pdf", State: SourceMissing},
	}
	for name, wantPDF := range want {
		got, err := readSourcePDF(os.DirFS(root), name)
		if err != nil {
			t.Fatalf("readSourcePDF(%s): %v", name, err)
		}
		if diff := cmp.Diff(wantPDF, got); diff != "" {
			t.Errorf("readSourcePDF(%s) (-want +got):\n%s", name, diff)
		}
	}
}
