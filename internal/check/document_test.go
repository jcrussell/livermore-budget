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
		FiscalYear: 2026, Basis: "adopted", Scope: "revenue-by-fund", Version: version,
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
	if len(res.Findings) != 1 || !strings.Contains(res.Findings[0].Detail, "no structural check reads it") {
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

// TestAProjectionThatProducesNothingIsReported is the case the obvious
// implementation misses.
//
// A registered projection returning zero slices lands in neither list on the
// Subject, so a check reading only those two cannot see it — while `fisc export`
// publishes its document regardless. Silence here would be the whole defect this
// check exists for.
func TestAProjectionThatProducesNothingIsReported(t *testing.T) {
	built, failed, err := buildProjections(
		[]project.Projection{&silent{}}, spineFacts(), "test")
	if err != nil {
		t.Fatalf("buildProjections: %v", err)
	}
	if len(built) != 0 || len(failed) != 0 {
		t.Fatalf("built=%d failed=%d; a projection declaring no slices should produce neither",
			len(built), len(failed))
	}

	s := &Subject{Registered: []string{"silent"}}
	res, err := (&documentsAreChecked{}).Run(t.Context(), s)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != StatusFail {
		t.Fatalf("documents-are-checked = %s over a projection that built nothing, want fail",
			res.Status)
	}
	if len(res.Findings) != 1 ||
		!strings.Contains(res.Findings[0].Detail, "produced no document at all") {
		t.Errorf("findings = %+v, want one naming the silent projection", res.Findings)
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
		PublishedYears: []int{2026, 2027},
		Projections: []Projection{{
			Name:  project.PublishedProjection,
			Graph: &project.Graph{},
			Options: project.Options{
				FiscalYear: 2026,
				Basis:      project.PublishedBasis,
				Scope:      project.PublishedScope,
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
		PublishedYears: []int{2026},
		Projections: []Projection{{
			Name: "something-else",
			Options: project.Options{
				FiscalYear: 2026,
				Basis:      project.PublishedBasis,
				Scope:      project.PublishedScope,
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
