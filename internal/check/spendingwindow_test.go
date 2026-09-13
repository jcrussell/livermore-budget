package check

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/jcrussell/livermore-budget/internal/amount"
	"github.com/jcrussell/livermore-budget/internal/mapping"
	"github.com/jcrussell/livermore-budget/internal/project"
)

// spendingSubject is a spine and a departmentwide cross-tab of one column,
// shaped like the committed pair at a size a reader can add up.
//
//	wages-and-benefits    spine in 1,000 = two groups' cells, 700 + 300
//	                      divisions out 1,000 = 600 + 400
//	services-and-supplies spine in   500
//	                      divisions out 250, and 250 is the DECLARED gap
//	transfers/out         spine in   100, and it is not an object category:
//	                      pp.85-125 print that row as zero, so the cross-tab
//	                      draws nothing and this must not be a subject
//
// THE FIGURES DIFFER FROM EACH OTHER ON PURPOSE. A gap of 250 against a
// divisions total of 250 would let a check that swapped the two sides pass, and
// a spine cell equal to another category's would let one that keyed on the wrong
// node pass.
func spendingSubject() *Subject {
	col := project.Column{FiscalYear: 2027, Basis: mapping.BasisAdopted}
	link := func(src, dst string, dollars int64) project.Link {
		return project.Link{Source: src, Target: dst, ValueCents: dollars * 100}
	}
	object := func(id string) project.Node {
		return project.Node{ID: id, Tier: 5, Role: roleObjectCategory}
	}
	spine := &project.Graph{
		Nodes: []project.Node{
			{ID: "fund-group/general", Tier: 2, Role: "fund_group"},
			{ID: "fund-group/internal-service", Tier: 2, Role: "fund_group"},
			object("expenditure/wages-and-benefits"),
			object("expenditure/services-and-supplies"),
			// A FLOW ENDPOINT AT TIER 5, and the reason the restriction is the
			// ROLE: it sits in the same column as the object categories and no
			// division decomposes it.
			{ID: "transfers/out", Tier: 5, Role: "transfer_out"},
		},
		Links: []project.Link{
			link("fund-group/general", "expenditure/wages-and-benefits", 700),
			link("fund-group/internal-service", "expenditure/wages-and-benefits", 300),
			link("fund-group/general", "expenditure/services-and-supplies", 500),
			link("fund-group/general", "transfers/out", 100),
		},
	}
	window := &project.DepartmentSpendingDocument{
		Nodes: []project.Node{
			object("expenditure/wages-and-benefits"),
			object("expenditure/services-and-supplies"),
			{ID: "dept/patrol", Tier: 4, Role: "department"},
			{ID: "dept/maintenance", Tier: 4, Role: "department"},
		},
		Links: []project.Link{
			link("expenditure/wages-and-benefits", "dept/patrol", 600),
			link("expenditure/wages-and-benefits", "dept/maintenance", 400),
			link("expenditure/services-and-supplies", "dept/patrol", 250),
		},
	}
	return &Subject{Projections: []projection{
		{
			Name:    project.PublishedProjection,
			Options: project.Options{Columns: []project.Column{col}, Scopes: []string{spineScope}},
			Graph:   spine,
		},
		{
			Name: project.DepartmentSpendingProjection,
			Options: project.Options{Columns: []project.Column{col},
				Scopes: project.DepartmentSpendingScopes()},
			DepartmentSpending: window,
		},
	}}
}

// fixtureGaps is the declaration the fixture is built around: the same shape
// departmentwideExceptions has, at the fixture's figures.
func fixtureGaps() []departmentwideException {
	return []departmentwideException{{
		category: "services-and-supplies", year: 2027, basis: mapping.BasisAdopted,
		spineCents:  amount.Cents(50000),
		detailCents: amount.Cents(25000),
		bead:        "fisc-av0w",
		reason:      "the fixture's declared gap",
	}}
}

func TestTheSpendingWindowReconcilesOverTheFixture(t *testing.T) {
	res, err := runSpendingWindow(spendingSubject(), fixtureGaps())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	type verdict struct {
		Status   Status
		Subjects int
	}
	// TWO SUBJECTS AND NOT THREE. transfers/out is at tier 5 and carries the
	// spine's transfer_out role, so it is not an object category and pp.85-125
	// decompose nothing into it.
	want := verdict{StatusPass, 2}
	if diff := cmp.Diff(want, verdict{res.Status, res.Subjects}); diff != "" {
		t.Errorf("verdict mismatch (-want +got):\n%s\nfindings: %v", diff, res.Findings)
	}
	if !strings.Contains(res.Summary, "held apart by the declared $250.00") {
		t.Errorf("summary does not name the declared gap: %q", res.Summary)
	}
}

// TestTheDeclaredGapIsReadAndNeverComputed is the arm that makes this check
// worth having, and it is a mutation rather than an assertion about code.
//
// DELETE THE ENTRY AND THE CELL MUST GO RED AT THE FULL AMOUNT. A check that
// took the gap as "whatever the difference is" would pass on both runs, which is
// the fundingSourcesException rule stated as a test: a declared figure fails the
// moment either side moves by anything other than exactly it, and a computed one
// absorbs the next dropped division in silence.
func TestTheDeclaredGapIsReadAndNeverComputed(t *testing.T) {
	res, err := runSpendingWindow(spendingSubject(), nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != StatusFail {
		t.Fatalf("status = %s with the gap deleted, want FAIL: %s", res.Status, res.Summary)
	}
	if !someFinding(res, "$250.00 unaccounted") {
		t.Errorf("no finding reports the full gap as unaccounted; got %v",
			findingLines(res))
	}

	// AND MOVING EITHER SIDE BY ANYTHING ELSE IS STILL RED WITH THE ENTRY IN
	// PLACE. A declaration that absorbed whatever arrived would go quiet here.
	s := spendingSubject()
	s.Projections[1].DepartmentSpending.Links[0].ValueCents -= 100
	moved, err := runSpendingWindow(s, fixtureGaps())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if moved.Status != StatusFail {
		t.Errorf("a division short by $1.00 is %s, want FAIL: %s", moved.Status, moved.Summary)
	}
}

// TestTheWindowRefusesACategoryTheSpineDoesNotDraw is the other direction, and
// it is the claim departmentwide-ties-to-spine makes about facts, made here
// about graphs: this document must DECOMPOSE the spine's right-hand column and
// may never extend it.
func TestTheWindowRefusesACategoryTheSpineDoesNotDraw(t *testing.T) {
	s := spendingSubject()
	w := s.Projections[1].DepartmentSpending
	w.Links = append(w.Links, project.Link{
		Source: "expenditure/grants", Target: "dept/patrol", ValueCents: 12345,
	})
	res, err := runSpendingWindow(s, fixtureGaps())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != StatusFail {
		t.Fatalf("status = %s, want FAIL", res.Status)
	}
	if !someFinding(res, "never extend it") {
		t.Errorf("no finding says the cross-tab may not extend the spine; got %v",
			findingLines(res))
	}
}

// TestASpineCategoryWithNoDivisionBehindItIsAFinding covers the absent-is-not-
// zero edge: a category the spine draws and the cross-tab has lost entirely.
//
// The whole-document arm would not see it -- the totals still balance among the
// categories that remain -- and a loop over the CROSS-TAB's keys would not
// either, because the key is gone from the side it is looping over.
func TestASpineCategoryWithNoDivisionBehindItIsAFinding(t *testing.T) {
	s := spendingSubject()
	w := s.Projections[1].DepartmentSpending
	w.Links = w.Links[:2] // drop services-and-supplies entirely
	res, err := runSpendingWindow(s, fixtureGaps())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != StatusFail {
		t.Fatalf("status = %s, want FAIL", res.Status)
	}
	if !someFinding(res, "pp.85-125's divisions take $0.00 out of it") {
		t.Errorf("no finding reports the category as empty; got %v", findingLines(res))
	}
}

// TestAGapNamingNoReconciledCellIsReported is the self-retiring half: an entry
// that holds nothing apart while the summary reports the column reconciled.
func TestAGapNamingNoReconciledCellIsReported(t *testing.T) {
	gaps := fixtureGaps()
	gaps[0].category = "grants"
	res, err := runSpendingWindow(spendingSubject(), gaps)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != StatusFail {
		t.Fatalf("status = %s, want FAIL", res.Status)
	}
	if !someFinding(res, "held apart from nothing") {
		t.Errorf("no finding reports the inert entry; got %v", findingLines(res))
	}

	// A GAP IN A COLUMN THE SPINE DOES NOT PUBLISH IS NOT STALE. The loop never
	// reaches its cell, so demanding its deletion would be demanding it on the
	// strength of a comparison that was never made.
	gaps = fixtureGaps()
	gaps[0].year = 2024
	gaps[0].basis = mapping.BasisActual
	res, err = runSpendingWindow(spendingSubject(), gaps)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !someFinding(res, "leaving $250.00 unaccounted") {
		t.Errorf("the FY2027 cell should now be unreconciled; got %v", findingLines(res))
	}
	if someFinding(res, "held apart from nothing") {
		t.Errorf("a gap in an unpublished column was reported stale; got %v", findingLines(res))
	}
}

// TestSpendingGapsIsTheSameDeclarationTheCheckReads is the seam Lane F's drill
// step reads, held against the table it is derived from.
//
// TWO SPELLINGS OF A SET THAT MUST AGREE DRIFT. This is the test that there is
// one: every entry of departmentwideExceptions reaches the exported map under
// the node id its category is drawn at, carrying its own figures and its bead.
func TestSpendingGapsIsTheSameDeclarationTheCheckReads(t *testing.T) {
	got := SpendingGaps()
	if len(got) == 0 {
		t.Fatal("SpendingGaps is empty, so nothing below asserts anything")
	}
	for _, e := range departmentwideExceptions {
		id := "expenditure/" + e.category
		reason, ok := got[id]
		if !ok {
			t.Fatalf("SpendingGaps has no entry for %q, which is the node %s is drawn at",
				id, e.key())
		}
		for _, want := range []string{
			e.bead,
			e.discrepancy().String(),
			e.spineCents.String(),
			e.detailCents.String(),
		} {
			if !strings.Contains(reason, want) {
				t.Errorf("SpendingGaps[%q] does not carry %q: %s", id, want, reason)
			}
		}
	}
	// A COPY, so no caller can widen the published set by writing to it.
	got["expenditure/grants"] = "invented"
	if _, ok := SpendingGaps()["expenditure/grants"]; ok {
		t.Error("SpendingGaps hands back a map a caller can extend")
	}
}

// someFinding reports whether any finding's detail carries want.
func someFinding(res Result, want string) bool {
	for _, line := range findingLines(res) {
		if strings.Contains(line, want) {
			return true
		}
	}
	return false
}
