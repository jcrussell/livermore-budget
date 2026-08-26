package check

import (
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/project"
)

// seriesOnly is a projection whose document is not a graph, standing in for the
// trends document that fisc-4ua.3 will add.
//
// It exists to pin the shape rather than to model that document: what these
// tests are about is what THIS package does with a projection it cannot read
// structurally, and that answer must be settled before such a projection is
// written, not discovered by it.
type seriesOnly struct{}

var _ project.Projection = (*seriesOnly)(nil)
var _ project.Sliced = (*seriesOnly)(nil)

func (*seriesOnly) Name() string { return "series-only" }

func (*seriesOnly) Build(_ []fact.Fact, _ project.Options) ([]byte, error) {
	return []byte("{}\n"), nil
}

func (*seriesOnly) Slices(_ []fact.Fact, version string) []project.Options {
	return []project.Options{{
		Columns: []project.Column{{FiscalYear: 2026, Basis: "adopted"}},
		Scope:   "revenue-by-fund",
		Version: version,
	}}
}

func spineFacts() []fact.Fact {
	return []fact.Fact{{FiscalYear: 2026, Basis: "adopted", Scope: spineScope}}
}

// TestANonGraphProjectionDoesNotKillTheRun is the regression this whole change
// exists for.
//
// buildProjections used to type-assert graphBuilder and RETURN AN ERROR when it
// failed, so registering the first projection whose document is not a graph made
// `fisc verify` exit non-zero having printed no report at all — every finding in
// the run lost to a document that was merely of a different shape. That is the
// failure ProjectionFailure's doc comment says was fixed once already for a
// refusing projection, wearing a second face.
func TestANonGraphProjectionDoesNotKillTheRun(t *testing.T) {
	built, failed, err := buildProjections(
		[]project.Projection{&seriesOnly{}}, spineFacts(), "test")
	if err != nil {
		t.Fatalf("buildProjections rejected a non-graph projection: %v", err)
	}
	if len(failed) != 0 {
		t.Errorf("a non-graph projection is not a build failure, got %d", len(failed))
	}
	if len(built) != 1 {
		t.Fatalf("built %d projections, want 1", len(built))
	}
	if built[0].Graph != nil {
		t.Error("a projection that builds no graph carries one")
	}
}

// TestGraphsExcludesWhatHasNoGraph is the other half: the checks that mean
// "every graph" must not be reachable by a projection that has none.
func TestGraphsExcludesWhatHasNoGraph(t *testing.T) {
	s := &Subject{Projections: []Projection{
		{Name: "sankey", Graph: &project.Graph{}},
		{Name: "series-only"},
	}}
	got := s.Graphs()
	if len(got) != 1 || got[0].Name != "sankey" {
		t.Fatalf("Graphs() = %v, want the one projection carrying a graph", got)
	}
}

// TestAnUncheckedDocumentIsReported is what stops the relaxation above from
// becoming a hole: a document no structural check reads must be reported, not
// published quietly.
func TestAnUncheckedDocumentIsReported(t *testing.T) {
	s := &Subject{Projections: []Projection{
		{Name: "sankey", Graph: &project.Graph{}},
		{Name: "series-only"},
	}}
	res, err := (&documentsAreChecked{}).Run(t.Context(), s)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != StatusFail {
		t.Fatalf("documents-are-checked = %s over an unchecked document, want fail", res.Status)
	}
	if len(res.Findings) != 1 ||
		!strings.Contains(res.Findings[0].Detail, "no document of any shape the structural checks read") {
		t.Errorf("findings = %+v, want one naming the unread projection", res.Findings)
	}
}

// TestEachProjectionIsBuiltOverItsOwnSlices pins the other half of the split:
// buildProjections used to build the cartesian product of every projection and
// every slice, so a projection of a different schedule was handed slices
// containing none of its facts.
func TestEachProjectionIsBuiltOverItsOwnSlices(t *testing.T) {
	built, _, err := buildProjections(
		[]project.Projection{&seriesOnly{}}, spineFacts(), "test")
	if err != nil {
		t.Fatalf("buildProjections: %v", err)
	}
	if len(built) != 1 {
		t.Fatalf("built %d projections, want the 1 slice seriesOnly asked for", len(built))
	}
	if got := built[0].Options.Scope; got != "revenue-by-fund" {
		t.Errorf("scope = %q, want the projection's own %q and not the spine's",
			got, "revenue-by-fund")
	}
}

// silent is a projection that declares no slices at all. It produces no
// document and no failure, so it appears in neither Subject.Projections nor
// Subject.ProjectionFailures.
type silent struct{}

func (*silent) Name() string { return "silent" }
func (*silent) Build(_ []fact.Fact, _ project.Options) ([]byte, error) {
	return []byte("{}\n"), nil
}
func (*silent) Slices(_ []fact.Fact, _ string) []project.Options { return nil }

// TestAProjectionThatProducesNothingIsNotADefect records a reversal, and the
// measurement behind it.
//
// This used to assert the opposite. documentsAreChecked carried an arm reporting
// a registered projection that declared no slices, on the stated grounds that
// "`fisc export` publishes its document regardless" -- true of the export
// command's old build loop, which ran the cartesian product of the published
// years and the registry and called Build whatever the projection said it was
// of. fisc-neh inverted that loop: the command now iterates the slices each
// projection declares, so a projection declaring none writes no file and there
// is no document to be wrong about.
//
// Keeping the arm would have turned every projection of a schedule a FIXTURE
// does not carry into a corpus defect: internal/check/fixture_test.go is a
// miniature of the spine, the trends projection is of nothing there, and ~70
// runChecks call sites would have gone red reporting on a test's own scope.
// project.Sliced's doc comment already settles what the state means -- "an empty
// result means the store carries nothing this projection is of, which is a
// statement about the corpus and not an error".
//
// WHAT THE ARM DID COVER, AND WHERE IT WENT: a document the site PUBLISHES that
// silently stops being built. Only the spine is guarded against that today, by
// publishedProjectionBuilt, which compares against one projection name. That is
// filed as fisc-w7d rather than left implied by a test that no longer exists.
func TestAProjectionThatProducesNothingIsNotADefect(t *testing.T) {
	built, failed, err := buildProjections(
		[]project.Projection{&silent{}}, spineFacts(), "test")
	if err != nil {
		t.Fatalf("buildProjections: %v", err)
	}
	if len(built) != 0 || len(failed) != 0 {
		t.Fatalf("built=%d failed=%d; a projection declaring no slices should produce neither",
			len(built), len(failed))
	}

	s := &Subject{}
	res, err := (&documentsAreChecked{}).Run(t.Context(), s)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	// Vacuous rather than pass: nothing was built, so nothing was examined, and
	// a pass over zero subjects is the one thing this package exists to prevent.
	if res.Status != StatusVacuous {
		t.Fatalf("documents-are-checked = %s over a projection that built nothing, want vacuous",
			res.Status)
	}
	if len(res.Findings) != 0 {
		t.Errorf("findings = %+v, want none: declaring no slices is a statement about the "+
			"corpus, not a defect", res.Findings)
	}
}

// TestAPublishedYearNothingBuiltIsReported is the FY2027 case.
//
// published-projection-built used to pin ONE (year, basis, scope) triple, so a
// second published year could ship to readers with no check having looked at it
// — and the check would go on passing, over the first year, reporting a number
// that looked like coverage. It iterates the published set now.
func TestAPublishedYearNothingBuiltIsReported(t *testing.T) {
	s := &Subject{
		Published: spineDocuments(2026, 2027),
		Projections: []Projection{{
			Name:  project.PublishedProjection,
			Graph: &project.Graph{},
			Options: project.Options{
				Columns: []project.Column{{FiscalYear: 2026, Basis: project.PublishedBasis}},
				Scope:   project.PublishedScope,
			},
		}},
	}
	res, err := (&publishedProjectionBuilt{}).Run(t.Context(), s)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != StatusFail {
		t.Fatalf("published-projection-built = %s with FY2027 published and unbuilt, want fail",
			res.Status)
	}
	if len(res.Findings) != 1 || !strings.Contains(res.Findings[0].Subject, "FY2027") {
		t.Errorf("findings = %+v, want one naming FY2027", res.Findings)
	}
}

// TestAPublishedYearBuiltByAnotherProjectionIsNotEnough guards the narrowing
// that came with per-projection slices: matching the triple is no longer the
// same as having built the document the page renders.
func TestAPublishedYearBuiltByAnotherProjectionIsNotEnough(t *testing.T) {
	s := &Subject{
		Published: spineDocuments(2026),
		Projections: []Projection{{
			Name: "something-else",
			Options: project.Options{
				Columns: []project.Column{{FiscalYear: 2026, Basis: project.PublishedBasis}},
				Scope:   project.PublishedScope,
			},
		}},
	}
	res, err := (&publishedProjectionBuilt{}).Run(t.Context(), s)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != StatusFail {
		t.Errorf("published-projection-built = %s when only another projection covered the "+
			"published slice, want fail", res.Status)
	}
}

// TestUncheckedDocumentsIsEmpty guards the state the map should stay in, in the
// shape incompleteSeries already has. An entry is a promise that someone is
// coming back; the committed corpus needs none, and a future entry should be a
// deliberate act rather than something that accumulated.
func TestUncheckedDocumentsIsEmpty(t *testing.T) {
	if len(uncheckedDocuments) != 0 {
		t.Errorf("uncheckedDocuments carries %d entries: %v",
			len(uncheckedDocuments), uncheckedDocuments)
	}
}

// withUncheckedDocuments swaps the declaration map for one test and restores it,
// so a table of cases cannot leak into the next.
func withUncheckedDocuments(t *testing.T, m map[string]string) {
	t.Helper()
	prev := uncheckedDocuments
	uncheckedDocuments = m
	t.Cleanup(func() { uncheckedDocuments = prev })
}

// TestADeclaredDocumentIsNotCountedAsExamined is the defect the missing test
// let stand (fisc-rwo).
//
// The count was len(s.Projections), which counts the projections the declaration
// EXEMPTS. So a run in which every projection is declared unchecked reported a
// PASS -- over a denominator it had not looked at, which is the state this
// check's own doc comment calls the one thing this package exists to prevent.
// The `nothing:` branch was unreachable while any declaration was live.
func TestADeclaredDocumentIsNotCountedAsExamined(t *testing.T) {
	withUncheckedDocuments(t, map[string]string{"blob": "no checks yet (fisc-000)"})
	s := &Subject{Projections: []Projection{{Name: "blob"}}}

	res, err := (&documentsAreChecked{}).Run(t.Context(), s)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != StatusVacuous {
		t.Fatalf("documents-are-checked = %s with every projection declared unchecked, "+
			"want vacuous: nothing was examined", res.Status)
	}
	// Vacuous says WHAT is absent, or the verdict is a shrug. The old pass
	// summary here read "0 projections structurally checked ()" -- a count of
	// nothing beside an empty shape list -- which the corrected denominator
	// makes unreachable rather than merely better worded.
	if !strings.Contains(res.Summary, "no projection built a document") {
		t.Errorf("summary %q does not say what is absent", res.Summary)
	}
}

// TestAnUnreadProjectionIsInItsOwnDenominator is the other side of narrowing
// the count, and narrowing it too far is a defect of the same class.
//
// A projection that built no shape any check reads WAS examined -- this check
// looked at it and reported it. Counting only the ones that reached a shape
// would report "2 findings over 0 projections": a numerator with no denominator
// under it, which is as unreadable as the pass over zero the narrowing fixed.
func TestAnUnreadProjectionIsInItsOwnDenominator(t *testing.T) {
	s := &Subject{Projections: []Projection{{Name: "blob"}, {Name: "blob-two"}}}

	res, err := (&documentsAreChecked{}).Run(t.Context(), s)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != StatusFail {
		t.Fatalf("documents-are-checked = %s over two unread projections, want fail",
			res.Status)
	}
	if !strings.Contains(res.Summary, "over 2 projections") {
		t.Errorf("summary %q does not count the two projections it examined", res.Summary)
	}
}

// TestADeclaredDocumentDoesNotHideAnExaminedOne is the other side of the count:
// narrowing the denominator to what was examined must not make the declaration
// invisible, because a growing exemption read as a shrinking one is how a
// document stays unchecked forever.
func TestADeclaredDocumentDoesNotHideAnExaminedOne(t *testing.T) {
	withUncheckedDocuments(t, map[string]string{"blob": "no checks yet (fisc-000)"})
	s := &Subject{Projections: []Projection{
		{Name: "sankey", Graph: &project.Graph{}},
		{Name: "blob"},
	}}

	res, err := (&documentsAreChecked{}).Run(t.Context(), s)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != StatusPass {
		t.Fatalf("documents-are-checked = %s, want pass: one projection was examined",
			res.Status)
	}
	if !strings.Contains(res.Summary, "1 projection structurally checked") {
		t.Errorf("summary %q does not count the one projection it examined", res.Summary)
	}
	if !strings.Contains(res.Summary, `1 of "blob"`) {
		t.Errorf("summary %q does not name the declared projection", res.Summary)
	}
}

// TestAStaleUncheckedDocumentDeclarationIsCaught pins the expiry branch, and the
// second subtest pins the wording fisc-rwo is about: the finding used to say
// "now carries a graph" whatever the projection carried, in the check that was
// widened precisely so a document need not be a graph. A reader would go looking
// for a graph that does not exist.
func TestAStaleUncheckedDocumentDeclarationIsCaught(t *testing.T) {
	t.Run("no projection of that name", func(t *testing.T) {
		withUncheckedDocuments(t, map[string]string{"gone": "removed or typo'd"})
		s := &Subject{Projections: []Projection{{Name: "sankey", Graph: &project.Graph{}}}}

		res, err := (&documentsAreChecked{}).Run(t.Context(), s)
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		if res.Status != StatusFail ||
			!strings.Contains(findingDetails(res), "no projection of that name was built") {
			t.Errorf("a declaration naming nothing reported %s: %s",
				res.Status, findingDetails(res))
		}
	})

	t.Run("the projection now carries a series", func(t *testing.T) {
		withUncheckedDocuments(t, map[string]string{"revenue-trends": "no checks yet"})
		s := &Subject{Projections: []Projection{
			{Name: "revenue-trends", Trends: &project.TrendsDocument{}},
		}}

		res, err := (&documentsAreChecked{}).Run(t.Context(), s)
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		if res.Status != StatusFail {
			t.Fatalf("a declaration over a checked projection reported %s", res.Status)
		}
		got := findingDetails(res)
		if !strings.Contains(got, "series") {
			t.Errorf("the finding does not name the shape the projection carries: %s", got)
		}
		if strings.Contains(got, "graph") {
			t.Errorf("the finding says graph about a projection that carries a series: %s", got)
		}
	})
}
