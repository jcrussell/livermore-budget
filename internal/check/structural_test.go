package check

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/jcrussell/livermore-budget/internal/corpus"
	"github.com/jcrussell/livermore-budget/internal/mapping"
	"github.com/jcrussell/livermore-budget/internal/registry"
)

// Every test here copies the real repository, changes one thing on disk that
// `git status` could show, loads it, and asserts the check claiming that thing
// reports it. --full tests synthesize their own source documents (repoWithPDFs).

// docWithFacts is the document the committed rules read; docWithout is one no
// rule file maps, so mutating it is not a load error.
// TestDocWithoutIsMappedByNothing holds that.
const (
	docWithFacts = testDoc
	docWithout   = "livermore-cip-fy2026-2030"
)

// pagesOfDocWithout is docWithout's page count, read off data/sources.yaml.
func pagesOfDocWithout(t *testing.T) int {
	t.Helper()
	sources, err := registry.LoadSources(os.DirFS("../../data"))
	if err != nil {
		t.Fatalf("LoadSources: %v", err)
	}
	for _, src := range sources {
		if src.ID == docWithout {
			return src.Pages
		}
	}
	t.Fatalf("data/sources.yaml lists no document %q", docWithout)
	return 0
}

// TestDocWithoutIsMappedByNothing fails with the cause when a mapped document
// would otherwise surface as a load failure in an unrelated manifest test.
func TestDocWithoutIsMappedByNothing(t *testing.T) {
	t.Parallel()
	files, err := mapping.LoadDir(os.DirFS("../.."), "mappings")
	if err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("no rule files loaded, so this test asserts nothing")
	}
	mapped := false
	for _, f := range files {
		if f.DocID == docWithFacts {
			mapped = true
		}
		if f.DocID == docWithout {
			t.Errorf("%s maps %s, which docWithout names as a document nothing maps; "+
				"point docWithout at a document with no rule file, or the mutation "+
				"tests in this file will fail on a missing manifest instead of on "+
				"what they assert", f.Path, docWithout)
		}
	}
	// The other half: docWithFacts must actually be mapped, or the tests that
	// contrast the two are contrasting nothing.
	if !mapped {
		t.Errorf("no rule file maps %s, which docWithFacts names as the document the "+
			"committed rules read", docWithFacts)
	}
}

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

// mutateManifest rewrites one extraction's manifest.json, decoding with
// UseNumber so integers do not round-trip as float64.
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

// anArtifactOf names one artifact deterministically: the first geometry file by
// sort, p0001, which no fact cites and no rule reads, so only the drift sweep
// sees a corruption in it.
func anArtifactOf(t *testing.T, root, docID string) (string, corpus.Artifact) {
	t.Helper()
	artifacts := manifestArtifacts(t, root, docID)
	for _, rel := range slices.Sorted(maps.Keys(artifacts)) {
		if strings.HasPrefix(rel, "geometry/") {
			return rel, artifacts[rel]
		}
	}
	t.Fatalf("%s lists no geometry artifact, so this test has nothing to corrupt", docID)
	return "", corpus.Artifact{}
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

// dropArtifact removes one artifact from both the manifest and the directory,
// which the hash sweep cannot see.
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

// TestAnEmptiedExtractionFails: an extraction emptied from both the directory
// and its manifest passes the hash sweep, and extraction-emitted-every-page
// must fail it. This is the shape a poppler run failing on pages leaves.
func TestAnEmptiedExtractionFails(t *testing.T) {
	t.Parallel()
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
	n := pagesOfDocWithout(t)
	for _, want := range []string{
		fmt.Sprintf("counts %d pages and lists 0 artifacts", n),
		fmt.Sprintf("any of the %d pages", n),
	} {
		if !strings.Contains(res.Findings[0].Detail, want) {
			t.Errorf("finding %q does not contain %q", res.Findings[0].Detail, want)
		}
	}
	// Every extraction is still counted, so the report says three documents were
	// examined and one of them is empty -- not that two were examined.
	if res.Subjects != pages {
		t.Errorf("subjects = %d, want the %d extractions it looked at before", res.Subjects, pages)
	}
	// The hash sweep passes: only an independent page count sees this.
	sweep := resultFor(t, rep, "artifacts-match-manifest")
	if sweep.Status != StatusPass {
		t.Errorf("artifacts-match-manifest = %s (%s), want pass: the manifest and the "+
			"directory still agree", sweep.Status, sweep.Summary)
	}
}

// TestAMissingPageFails: one page dropped from both sides, as a poppler
// failure on one page leaves it.
func TestAMissingPageFails(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name, rel, want string
	}{
		{"the page text", corpus.PagePath(5), "no page text for 1 of %d pages (p5)"},
		{"the word geometry", corpus.GeometryPath(5),
			"no word geometry for 1 of %d pages (p5)"},
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
			want := fmt.Sprintf(tt.want, pagesOfDocWithout(t))
			if !strings.Contains(res.Findings[0].Detail, want) {
				t.Errorf("finding %q does not contain %q", res.Findings[0].Detail, want)
			}
			if got := resultFor(t, rep, "artifacts-match-manifest").Status; got != StatusPass {
				t.Errorf("artifacts-match-manifest = %s, want pass", got)
			}
		})
	}
}

// TestAnArtifactForNoPageFails: a manifest listing a page the document does not
// have, consistent with the directory, so the sweep passes.
func TestAnArtifactForNoPageFails(t *testing.T) {
	t.Parallel()
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
	if !strings.Contains(said(res),
		fmt.Sprintf("1 artifact for no page of a %d-page document", pagesOfDocWithout(t))) {
		t.Errorf("findings %v do not report the artifact as belonging to no page", res.Findings)
	}
	if !hasFinding(res, stray) {
		t.Errorf("findings %v do not name the artifact", res.Findings)
	}
}

// TestZeroingThePageCountIsCaughtByTheRegistry: zeroing page_count alongside an
// emptied extraction satisfies the completeness check, and the hand-written
// count in data/sources.yaml catches it.
func TestZeroingThePageCountIsCaughtByTheRegistry(t *testing.T) {
	t.Parallel()
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
	// The completeness check is satisfied, and the registry comparison fires.
	res := resultFor(t, rep, "manifest-matches-source-registry")
	if res.Status != StatusFail {
		t.Fatalf("manifest-matches-source-registry = %s (%s), want fail", res.Status, res.Summary)
	}
	if !hasFinding(res, fmt.Sprintf(
		"the extractor found 0 pages and the registry records %d", pagesOfDocWithout(t))) {
		t.Errorf("findings %v do not report the page count disagreement", res.Findings)
	}
}

// TestARecordedExtractionErrorFails: an error tools/extract.py records in the
// manifest fails verify.
func TestARecordedExtractionErrorFails(t *testing.T) {
	t.Parallel()
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

// TestWarningsAreCountedAndNotFailures: poppler warnings are not failures, and
// are counted into the passing summary.
func TestWarningsAreCountedAndNotFailures(t *testing.T) {
	t.Parallel()
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

// TestAManifestKeyThatIsNotACleanPathIsNamedAsWritten: a finding cites the
// manifest key as written; cleaning "pages/../geometry/p0001.json" would name
// an existing path as unreadable.
func TestAManifestKeyThatIsNotACleanPathIsNamedAsWritten(t *testing.T) {
	t.Parallel()
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

// TestAnUnreadableExtractionDirectoryIsAnError: a directory the sweep cannot
// list is an error, not a finding; an unreadable file is a per-file finding.
func TestAnUnreadableExtractionDirectoryIsAnError(t *testing.T) {
	t.Parallel()
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

// TestACorruptedArtifactFails is the drift this tier is named for: one byte of one
// committed artifact, changed, with the manifest untouched.
func TestACorruptedArtifactFails(t *testing.T) {
	t.Parallel()
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
	// No other check noticed: nothing else reads a geometry artifact.
	if got := resultFor(t, rep, "fact-offset-points-at-token").Status; got != StatusPass {
		t.Errorf("fact-offset-points-at-token = %s; this corruption is invisible to it, "+
			"which is why the drift sweep exists", got)
	}
}

// TestAnEditedManifestHashFails: the artifact untouched and its manifest
// record edited.
func TestAnEditedManifestHashFails(t *testing.T) {
	t.Parallel()
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
	detail := said(res)
	for _, s := range []string{short(want.SHA256), short(wrong)} {
		if !strings.Contains(detail, s) {
			t.Errorf("findings %v do not name %s", res.Findings, s)
		}
	}
}

// TestAnUnlistedArtifactFails: a file beside the manifest that it does not
// name has no recorded origin.
func TestAnUnlistedArtifactFails(t *testing.T) {
	t.Parallel()
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

// TestADeletedArtifactFails: the manifest lists an artifact that is gone,
// including one no rule reads.
func TestADeletedArtifactFails(t *testing.T) {
	t.Parallel()
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

// TestAManifestSkewedFromTheRegistryFails: manifest and registry disagree on
// one field per case.
func TestAManifestSkewedFromTheRegistryFails(t *testing.T) {
	t.Parallel()
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

// TestASourceWithNoExtractionFails: a registered document nobody extracted.
func TestASourceWithNoExtractionFails(t *testing.T) {
	t.Parallel()
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
	// Two documents remain and both are still swept; the missing one is
	// reported once, by the set comparison.
	if got := resultFor(t, rep, "extraction-toolchain-pinned").Subjects; got != 2 {
		t.Errorf("extraction-toolchain-pinned looked at %d extractions, want the 2 left", got)
	}
}

// TestAnExtractionWithNoSourceFails: an extraction with no registry entry, as
// a renamed document leaves behind.
func TestAnExtractionWithNoSourceFails(t *testing.T) {
	t.Parallel()
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

// writeSyntheticExtraction lays down a valid, empty extraction directory with
// the pinned toolchain; overrides is applied last to make one field wrong.
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

// TestAnUnpinnedToolchainFails: intact artifacts, an agreeing registry, and an
// extraction made by an unpinned poppler.
func TestAnUnpinnedToolchainFails(t *testing.T) {
	t.Parallel()
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

// TestAnUnreadableManifestSchemaRefusesToLoad: an unknown schema_version is a
// load failure, not a finding, since the artifact namespace may differ.
func TestAnUnreadableManifestSchemaRefusesToLoad(t *testing.T) {
	t.Parallel()
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

// fakePDF stands in for a source document under data/pdf/. It begins "%PDF-",
// which is what tells it from an LFS pointer; nothing in fisc parses a PDF.
func fakePDF(id string) []byte {
	return []byte("%PDF-1.7\n% a stand-in for " + id + ", so no test needs Git LFS\n%%EOF\n")
}

// repoWithPDFs is repoWithoutPDFs plus a synthesized source document per
// registry entry, with data/sources.yaml and every manifest rewritten to its
// hash and size. The registry's page counts are left as they are; nothing
// reads a PDF's page count.
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

// replaceOnce substitutes exactly one occurrence and fails otherwise.
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

// TestFullHashesEverySourceDocument is the pass case, the run in which this
// check stops being skipped.
func TestFullHashesEverySourceDocument(t *testing.T) {
	t.Parallel()
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
	// Six records over three documents: each file against two written claims.
	if res.Subjects != 6 {
		t.Errorf("subjects = %d, want 6 records over the 3 source documents", res.Subjects)
	}
}

// TestACorruptedSourceDocumentFails: the two written records agree and the
// bytes on disk differ; only hashing the file sees it.
func TestACorruptedSourceDocumentFails(t *testing.T) {
	t.Parallel()
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
	// Both records are named, so neither reads as the odd one out.
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

// TestATruncatedSourceDocumentFails covers comparePDF's size comparison, which
// a byte flip cannot reach.
func TestATruncatedSourceDocumentFails(t *testing.T) {
	t.Parallel()
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

// TestOnlyTheRecordsThatExistAreCounted: a document whose extraction is missing
// is compared against one record and counted once, not twice.
func TestOnlyTheRecordsThatExistAreCounted(t *testing.T) {
	t.Parallel()
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

// TestAnUnfetchedPointerIsAnErrorAndSaysHowToFetchIt: an LFS pointer where the
// document should be is an ERROR, not a corrupted document, and the message
// carries `git lfs pull`.
func TestAnUnfetchedPointerIsAnErrorAndSaysHowToFetchIt(t *testing.T) {
	t.Parallel()
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
		// The pointer records the registry's hash, so fetching reconciles.
		"which is the sha256 the registry records",
	} {
		if !strings.Contains(res.Summary, want) {
			t.Errorf("summary %q does not contain %q", res.Summary, want)
		}
	}
	// An error, not a failure, and nothing else in the report moved.
	if rep.Counts.Fail != 0 {
		t.Errorf("counts = %+v, want the unfetched pointer in the error column alone",
			rep.Counts)
	}
}

// TestAPointerThatDisagreesWithTheRegistrySaysSo: a pointer naming a different
// file than the registry is an error no fetch can fix, and says so.
func TestAPointerThatDisagreesWithTheRegistrySaysSo(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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

// lfsPointer renders a Git LFS v1 pointer in the shape this repository's
// committed pointers have (`git cat-file -p HEAD:data/pdf/...`).
func lfsPointer(oid string, size int64) []byte {
	return []byte(fmt.Sprintf("version https://git-lfs.github.com/spec/v1\noid sha256:%s\nsize %d\n",
		oid, size))
}

// TestParseLFSPointer: a false positive reports a real document as unfetched;
// a false negative hashes pointer text and reports it corrupted.
func TestParseLFSPointer(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	root := t.TempDir()
	body := fakePDF("probe")
	sum := sha256.Sum256(body)
	writeRepoFile(t, root, "data/pdf/probe.pdf", body)
	writeRepoFile(t, root, "data/pdf/pointer.pdf", lfsPointer(strings.Repeat("b", 64), 12))

	want := map[string]sourcePDF{
		"data/pdf/probe.pdf": {
			Path: "data/pdf/probe.pdf", State: sourcePresent,
			Bytes: int64(len(body)), SHA256: hex.EncodeToString(sum[:]),
		},
		"data/pdf/pointer.pdf": {
			Path: "data/pdf/pointer.pdf", State: sourcePointer,
			PointerOID: strings.Repeat("b", 64), PointerBytes: 12,
		},
		"data/pdf/absent.pdf": {Path: "data/pdf/absent.pdf", State: sourceMissing},
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
