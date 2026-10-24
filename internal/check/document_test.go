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

func (*seriesOnly) Name() string { return "series-only" }

func (*seriesOnly) Build(_ []fact.Fact, _ project.Options) ([]byte, error) {
	return []byte("{}\n"), nil
}

func (*seriesOnly) Slices(_ []fact.Fact, version string) []project.Options {
	return []project.Options{{
		Columns: []project.Column{{FiscalYear: 2026, Basis: "adopted"}},
		Scopes:  []string{"revenue-by-fund"},
		Version: version,
	}}
}

func spineFacts() []fact.Fact {
	return []fact.Fact{{FiscalYear: 2026, Basis: "adopted", Scope: spineScope}}
}

// TestANonGraphProjectionDoesNotKillTheRun: a projection whose document is not
// a graph is built and recorded, not refused, so `fisc verify` still reports
// every finding of the run.
func TestANonGraphProjectionDoesNotKillTheRun(t *testing.T) {
	t.Parallel()
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

// TestSpineIsSelectedByPublication is the other half: the headline checks
// read the documents the site publishes as the spine, and the structural
// checks every graph, so a projection with no document reaches neither -- and
// a graph of another projection carrying a headline block reaches only the
// structural checks, because the spine is a declaration and not a pointer.
func TestSpineIsSelectedByPublication(t *testing.T) {
	t.Parallel()
	column := project.Options{
		Columns: []project.Column{{FiscalYear: 2026, Basis: project.PublishedBasis}},
		Scopes:  []string{project.PublishedScope},
	}
	s := &Subject{
		Published: spineDocuments(2026),
		Projections: []projection{
			{Name: project.PublishedProjection, Options: column,
				Graph: &project.Document{Metadata: project.Metadata{Headline: &project.Headline{}}}},
			{Name: "fund-flows", Options: column,
				Graph: &project.Document{Metadata: project.Metadata{Headline: &project.Headline{}}}},
			{Name: "series-only"},
		},
	}
	docs, findings := s.spine()
	if len(findings) != 0 {
		t.Fatalf("spine() findings = %+v, want none over a spine carrying its headline", findings)
	}
	if len(docs) != 1 || docs[0].Name != project.PublishedProjection {
		t.Fatalf("spine() = %v, want the one projection the site publishes as the spine", docs)
	}
	linked := s.linkedDocuments()
	if len(linked) != 2 || linked[0].Name != project.PublishedProjection || linked[1].Name != "fund-flows" {
		t.Fatalf("linkedDocuments() = %v, want both graphs and not the series", linked)
	}

	// A spine document the publication names and the pointer would have
	// dropped: it is a finding and not an absence.
	s.Projections[0].Graph.Metadata.Headline = nil
	docs, findings = s.spine()
	if len(docs) != 0 {
		t.Errorf("spine() = %v, want no readable document once the spine's headline is gone", docs)
	}
	if len(findings) != 1 || findings[0].Subject != s.Projections[0].String() ||
		!strings.Contains(findings[0].Detail, "carries no headline") {
		t.Errorf("spine() findings = %+v, want one naming the spine document without a headline", findings)
	}
}

// TestAnUncheckedDocumentIsReported is what stops the relaxation above from
// becoming a hole: a document no structural check reads must be reported, not
// published quietly.
func TestAnUncheckedDocumentIsReported(t *testing.T) {
	t.Parallel()
	s := &Subject{Projections: []projection{
		{Name: "sankey", Graph: &project.Document{}},
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

// TestEachProjectionIsBuiltOverItsOwnSlices: a projection of a different
// schedule is built over the slices it declares, never over the spine's.
func TestEachProjectionIsBuiltOverItsOwnSlices(t *testing.T) {
	t.Parallel()
	built, _, err := buildProjections(
		[]project.Projection{&seriesOnly{}}, spineFacts(), "test")
	if err != nil {
		t.Fatalf("buildProjections: %v", err)
	}
	if len(built) != 1 {
		t.Fatalf("built %d projections, want the 1 slice seriesOnly asked for", len(built))
	}
	if got := built[0].Options.ScopeList(); got != "revenue-by-fund" {
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

// TestAProjectionThatProducesNothingIsNotADefect: the export command builds the
// slices each projection declares, so a projection declaring none writes no
// file and there is no document to be wrong about. A fixture of the spine
// alone declares no trends slice, and that is not a corpus finding. A
// published document that stops being built is publishedProjectionBuilt's.
func TestAProjectionThatProducesNothingIsNotADefect(t *testing.T) {
	t.Parallel()
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

// TestAPublishedYearNothingBuiltIsReported: published-projection-built iterates
// every published year, so a second year that was not built is a finding
// rather than a pass over the first.
func TestAPublishedYearNothingBuiltIsReported(t *testing.T) {
	t.Parallel()
	s := &Subject{
		Published: spineDocuments(2026, 2027),
		Projections: []projection{{
			Name:  project.PublishedProjection,
			Graph: &project.Document{},
			Options: project.Options{
				Columns: []project.Column{{FiscalYear: 2026, Basis: project.PublishedBasis}},
				Scopes:  []string{project.PublishedScope},
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
	t.Parallel()
	s := &Subject{
		Published: spineDocuments(2026),
		Projections: []projection{{
			Name: "something-else",
			Options: project.Options{
				Columns: []project.Column{{FiscalYear: 2026, Basis: project.PublishedBasis}},
				Scopes:  []string{project.PublishedScope},
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
	t.Parallel()
	if len(uncheckedDocuments) != 0 {
		t.Errorf("uncheckedDocuments carries %d entries: %v",
			len(uncheckedDocuments), uncheckedDocuments)
	}
}

// TestADeclaredDocumentIsNotCountedAsExamined: the count is over what was
// examined, so a run in which every projection is declared unchecked is
// vacuous rather than a pass over projections it did not look at.
func TestADeclaredDocumentIsNotCountedAsExamined(t *testing.T) {
	t.Parallel()
	s := &Subject{
		Projections:        []projection{{Name: "blob"}},
		UncheckedDocuments: map[string]string{"blob": "no checks yet (bead id goes here)"},
	}

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
	t.Parallel()
	s := &Subject{Projections: []projection{{Name: "blob"}, {Name: "blob-two"}}}

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
	t.Parallel()
	s := &Subject{
		Projections: []projection{
			{Name: "sankey", Graph: &project.Document{}},
			{Name: "blob"},
		},
		UncheckedDocuments: map[string]string{"blob": "no checks yet (bead id goes here)"},
	}

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

// TestAStaleUncheckedDocumentDeclarationIsCaught pins the expiry branch, and
// the second subtest that the finding names the shape the projection carries.
func TestAStaleUncheckedDocumentDeclarationIsCaught(t *testing.T) {
	t.Parallel()
	t.Run("no projection of that name", func(t *testing.T) {
		s := &Subject{
			Projections:        []projection{{Name: "sankey", Graph: &project.Document{}}},
			UncheckedDocuments: map[string]string{"gone": "removed or typo'd"},
		}

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
		s := &Subject{
			Projections: []projection{
				{Name: "revenue-trends", Trends: &project.TrendsDocument{}},
			},
			UncheckedDocuments: map[string]string{"revenue-trends": "no checks yet"},
		}

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
