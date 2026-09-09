package check

import (
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/jcrussell/livermore-budget/internal/mapping"
	"github.com/jcrussell/livermore-budget/internal/project"
)

// drillSubject is a spine and a drill-down of one column, shaped like the
// committed pair at a size a reader can add up: two fund groups, one of them
// decomposed to a division, and every kind of residual the declaration names
// on at least one side.
//
// THE NUMBERS ARE CHOSEN SO NO MUTATION BELOW LANDS ON A COINCIDENCE. capital's
// transfers in are split 40/20 across two funds and its revenue 300/200 across
// two others, so moving any one fund between groups changes both groups by an
// amount no other flow happens to equal -- a fund carrying exactly general's
// transfer in could be re-parented and leave general green.
//
//	general  spine in  1,075 = revenue 1,000 + draw 50 + transfers in 25
//	         fund  in  1,000                    (draw and transfer in: residual)
//	         spine out 1,075 = expenditure 900 + transfers out 100 + reserve 75
//	         fund  out   900                    (the two endpoints: residual)
//	capital  spine in    600 = revenue 500 + draw 40 + transfers in 60
//	         fund  in    560 = revenue 300 + 200, transfers in 40 + 20
//	                                          (draw residual; transfers in whole)
//	         spine out   600, no fund-level outflow: not a subject
//
// A second drill-down of a column the spine does not publish is carried to
// show the pairing: it has links and contributes nothing.
func drillSubject() *Subject {
	col := project.Column{FiscalYear: 2026, Basis: mapping.BasisAdopted}
	link := func(src, dst string, dollars int64) project.Link {
		return project.Link{Source: src, Target: dst, ValueCents: dollars * 100}
	}
	spine := &project.Graph{Links: []project.Link{
		link("revenue/taxes/property", "fund-group/general", 1000),
		link("fund-balance/draw", "fund-group/general", 50),
		link("transfers/in", "fund-group/general", 25),
		link("fund-group/general", "expenditure/wages-and-benefits", 900),
		link("fund-group/general", "transfers/out", 100),
		link("fund-group/general", "fund-balance/reserve-increase", 75),
		link("revenue/charges-for-services", "fund-group/capital", 500),
		link("fund-balance/draw", "fund-group/capital", 40),
		link("transfers/in", "fund-group/capital", 60),
		link("fund-group/capital", "expenditure/capital-outlay", 600),
	}}
	node := func(id, parent string) project.Node { return project.Node{ID: id, Parent: parent} }
	drill := &project.FundFlowsDocument{
		Nodes: []project.Node{
			node("fund-group/general", ""), node("fund-group/capital", ""),
			node("fund/100", "fund-group/general"),
			node("fund/300", "fund-group/capital"), node("fund/301", "fund-group/capital"),
			node("fund/302", "fund-group/capital"),
			node("dept/patrol", "fund/100"),
			node("expenditure/patrol/wages-and-benefits", "dept/patrol"),
			node("revenue/taxes/property", ""), node("revenue/charges-for-services", ""),
			node("transfers/in", ""),
		},
		Links: []project.Link{
			link("revenue/taxes/property", "fund/100", 1000),
			link("fund/100", "dept/patrol", 900),
			link("dept/patrol", "expenditure/patrol/wages-and-benefits", 900),
			link("revenue/charges-for-services", "fund/300", 300),
			link("revenue/charges-for-services", "fund/302", 200),
			link("transfers/in", "fund/300", 40),
			link("transfers/in", "fund/301", 20),
		},
	}
	unpaired := project.Column{FiscalYear: 2024, Basis: mapping.BasisActual}
	return &Subject{Projections: []projection{
		{
			Name:    project.PublishedProjection,
			Options: project.Options{Columns: []project.Column{col}, Scopes: []string{spineScope}},
			Graph:   spine,
		},
		{
			Name:      project.FundFlowsProjection,
			Options:   project.Options{Columns: []project.Column{col}, Scopes: project.FundFlowsScopes()},
			FundFlows: drill,
		},
		{
			Name:      project.FundFlowsProjection,
			Options:   project.Options{Columns: []project.Column{unpaired}, Scopes: project.FundFlowsScopes()},
			FundFlows: &project.FundFlowsDocument{Nodes: drill.Nodes, Links: drill.Links},
		},
	}}
}

// findingLines renders each finding as "subject: detail", so a test can name
// both the side at fault and the amount in one substring.
func findingLines(res Result) []string {
	out := make([]string, 0, len(res.Findings))
	for _, f := range res.Findings {
		out = append(out, f.Subject+": "+f.Detail)
	}
	return out
}

func TestDrillReconcilesOverTheFixture(t *testing.T) {
	res, err := runDrillReconcile(drillSubject(), residualNodes)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	type verdict struct {
		Status   Status
		Subjects int
		Summary  string
	}
	want := verdict{
		Status: StatusPass,
		// Two groups' inflow and one group's outflow; capital's outflow is not
		// examined and is not counted, and the FY2024 drill-down pairs with
		// nothing.
		Subjects: 3,
		Summary: "FY2026 adopted: 2 fund groups' inflow and 1 decomposed group's outflow " +
			"(fund-group/general) reconcile to the cent; the residual the drill-down " +
			"cannot break down is $115.00 in and $175.00 out",
	}
	got := verdict{res.Status, res.Subjects, res.Summary}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("verdict mismatch (-want +got):\n%s\nfindings: %v", diff, res.Findings)
	}
}

// TestDrillReconcileIsFailable is the mutation table. Every row is a drift
// between the two documents or a hole in the declaration, and every expected
// string carries the UNACCOUNTED amount rather than the raw gap.
func TestDrillReconcileIsFailable(t *testing.T) {
	cases := []struct {
		name string
		// damage mutates the subject, the residual set, or both.
		damage       func(s *Subject, residual map[string]string)
		wantSubjects int
		// want is one substring per expected finding, in report order.
		want []string
	}{
		{
			name:         "the draw dropped from the declaration reddens every group that has one",
			damage:       func(_ *Subject, r map[string]string) { delete(r, "fund-balance/draw") },
			wantSubjects: 3,
			want: []string{
				"fund-group/capital inflow: spine flow into the group is $600.00, fund-level " +
					"flow is $560.00, named residual is $0.00, unaccounted $40.00",
				"fund-group/general inflow: spine flow into the group is $1,075.00, fund-level " +
					"flow is $1,000.00, named residual is $25.00, unaccounted $50.00",
			},
		},
		{
			name:         "transfers out dropped from the declaration reddens the decomposed group's outflow alone",
			damage:       func(_ *Subject, r map[string]string) { delete(r, "transfers/out") },
			wantSubjects: 3,
			want: []string{
				"fund-group/general outflow: spine flow out of the group is $1,075.00, " +
					"fund-level flow is $900.00, named residual is $75.00, unaccounted $100.00",
			},
		},
		{
			// The arm that proves the identity is not vacuous by construction:
			// the mapping error's own shape, and both groups say so with equal
			// and opposite amounts.
			name: "a spine link re-pointed to another group reddens both, equal and opposite",
			damage: func(s *Subject, _ map[string]string) {
				s.Projections[0].Graph.Links[0].Target = "fund-group/capital"
			},
			wantSubjects: 3,
			want: []string{
				"fund-group/capital inflow: spine flow into the group is $1,600.00, fund-level " +
					"flow is $560.00, named residual is $40.00, unaccounted $1,000.00",
				"fund-group/general inflow: spine flow into the group is $75.00, fund-level " +
					"flow is $1,000.00, named residual is $75.00, unaccounted -$1,000.00",
			},
		},
		{
			// data/funds.yaml moving a fund's type: revenue-by-fund's facts
			// follow the fund and the spine's stay where p66 prints them.
			name: "a fund re-parented to another group reddens both, equal and opposite",
			damage: func(s *Subject, _ map[string]string) {
				d := s.Projections[1].FundFlows
				for i := range d.Nodes {
					if d.Nodes[i].ID == "fund/302" {
						d.Nodes[i].Parent = "fund-group/general"
					}
				}
			},
			wantSubjects: 3,
			want: []string{
				"fund-group/capital inflow: spine flow into the group is $600.00, fund-level " +
					"flow is $360.00, named residual is $40.00, unaccounted $200.00",
				"fund-group/general inflow: spine flow into the group is $1,075.00, fund-level " +
					"flow is $1,200.00, named residual is $75.00, unaccounted -$200.00",
			},
		},
		{
			// A declared endpoint carried in part is the drift a subtraction
			// rule would have absorbed as residual.
			name: "one fund's transfer in dropped is a split, not a residual",
			damage: func(s *Subject, _ map[string]string) {
				d := s.Projections[1].FundFlows
				d.Links = slices.DeleteFunc(d.Links, func(l project.Link) bool {
					return l.Target == "fund/301"
				})
			},
			wantSubjects: 3,
			want: []string{
				"fund-group/capital inflow: transfers/in into the group is $60.00 on the spine " +
					"and $40.00 at fund level. A declared residual endpoint is carried whole or " +
					"decomposed whole, never split, so one document holds a row of it the other " +
					"lacks, and $20.00 is unaccounted",
			},
		},
		{
			// The group set is the union: a group one document carries and the
			// other does not is a subject, and it is red.
			name: "a group only the spine carries is unaccounted whole",
			damage: func(s *Subject, _ map[string]string) {
				g := s.Projections[0].Graph
				g.Links = append(g.Links, project.Link{
					Source: "revenue/taxes/property", Target: "fund-group/enterprise", ValueCents: 1000,
				})
			},
			wantSubjects: 4,
			want: []string{
				"fund-group/enterprise inflow: spine flow into the group is $10.00, fund-level " +
					"flow is $0.00, named residual is $0.00, unaccounted $10.00",
			},
		},
		{
			// The outflow identity switches on the moment a group is
			// decomposed, and a decomposition that does not add up is red.
			name: "a division appearing under a capital fund makes capital's outflow a subject",
			damage: func(s *Subject, _ map[string]string) {
				d := s.Projections[1].FundFlows
				d.Links = append(d.Links, project.Link{
					Source: "fund/300", Target: "dept/streets", ValueCents: 300_00,
				})
			},
			wantSubjects: 4,
			want: []string{
				"fund-group/capital outflow: spine flow out of the group is $600.00, fund-level " +
					"flow is $300.00, named residual is $0.00, unaccounted $300.00",
			},
		},
		{
			// An endpoint nothing declares is not skipped: spine_out is every
			// link out of the group, which is why project.GroupExpenditure is
			// not the outflow side.
			name: "a flow to an undeclared endpoint is unaccounted",
			damage: func(s *Subject, _ map[string]string) {
				g := s.Projections[0].Graph
				g.Links = append(g.Links, project.Link{
					Source: "fund-group/general", Target: "fund-balance/something-new", ValueCents: 700,
				})
			},
			wantSubjects: 3,
			want: []string{
				"fund-group/general outflow: spine flow out of the group is $1,082.00, " +
					"fund-level flow is $900.00, named residual is $175.00, unaccounted $7.00",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := drillSubject()
			residual := maps.Clone(residualNodes)
			tc.damage(s, residual)
			res, err := runDrillReconcile(s, residual)
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			if res.Status != StatusFail {
				t.Fatalf("status = %s, want fail: %s", res.Status, res.Summary)
			}
			if res.Subjects != tc.wantSubjects {
				t.Errorf("subjects = %d, want %d", res.Subjects, tc.wantSubjects)
			}
			got := findingLines(res)
			if len(got) != len(tc.want) {
				t.Fatalf("got %d findings, want %d:\n%s", len(got), len(tc.want),
					strings.Join(got, "\n"))
			}
			for i, want := range tc.want {
				if !strings.Contains(got[i], want) {
					t.Errorf("finding %d:\n  got  %s\n  want a line containing %q", i, got[i], want)
				}
			}
		})
	}
}

// TestADecompositionDroppedWholeReadsAsResidual pins the one drift the
// whole-or-nothing rule cannot see, so that the doc comment's claim about it is
// a measured one: capital's two transfer-in rows removed together leave nothing
// at fund level, and nothing at fund level is what "residual" means.
//
// It is GREEN HERE AND RED ELSEWHERE. The rows are facts of revenue-by-fund, and
// revenue-detail-ties-to-spine holds the spine's transfer_in cell against them
// at zero tolerance; the exception it declares is general's, not capital's.
// fisc-6gp2 carries the per-group declaration that would close it here.
func TestADecompositionDroppedWholeReadsAsResidual(t *testing.T) {
	s := drillSubject()
	d := s.Projections[1].FundFlows
	d.Links = slices.DeleteFunc(d.Links, func(l project.Link) bool { return l.Source == "transfers/in" })
	res, err := runDrillReconcile(s, residualNodes)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != StatusPass {
		t.Fatalf("status = %s, want the documented pass: %v", res.Status, res.Findings)
	}
	if !strings.Contains(res.Summary, "$175.00 in") {
		t.Errorf("summary %q does not carry capital's transfers in as residual", res.Summary)
	}
}

func TestDrillReconcileIsVacuousWithoutASharedColumn(t *testing.T) {
	s := drillSubject()
	// Keep the spine and the FY2024 drill-down; drop the one that pairs.
	s.Projections = slices.Delete(s.Projections, 1, 2)
	res, err := runDrillReconcile(s, residualNodes)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != StatusVacuous {
		t.Fatalf("status = %s, want vacuous: %v", res.Status, res.Findings)
	}
	if want := "no fiscal column is published by both the spine and the drill-down"; !strings.Contains(res.Summary, want) {
		t.Errorf("summary %q does not say %q", res.Summary, want)
	}
}

// TestTwoSpinesOfOneColumnIsAHarnessError: a second spine of the same column is
// a state the check cannot make sense of, and it is an error rather than a
// verdict about the corpus.
func TestTwoSpinesOfOneColumnIsAHarnessError(t *testing.T) {
	s := drillSubject()
	s.Projections = append(s.Projections, s.Projections[0])
	if _, err := runDrillReconcile(s, residualNodes); err == nil {
		t.Fatal("two spines of one column produced a verdict, want an error")
	}
}

// TestTheResidualSetIsTheEndpointSet pins the two declarations to each other:
// an endpoint is a flow outside the hierarchy, and that is exactly what a
// schedule of the hierarchy cannot decompose. A node in one table and not the
// other is a flow either drawn at a tier nothing declared or reconciled by
// nothing.
func TestTheResidualSetIsTheEndpointSet(t *testing.T) {
	want := slices.Sorted(maps.Keys(endpointTiers))
	got := slices.Sorted(maps.Keys(residualNodes))
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("residualNodes and endpointTiers name different nodes (-endpoints +residual):\n%s", diff)
	}
	for id, reason := range residualNodes {
		if strings.TrimSpace(reason) == "" {
			t.Errorf("%s is declared residual with no reason", id)
		}
	}
	// The published copy is a copy.
	published := ResidualNodes()
	published["coined/node"] = "widened by a caller"
	if _, leaked := residualNodes["coined/node"]; leaked {
		t.Error("ResidualNodes() handed out the package's own map")
	}
}

// TestTheCommittedDrillReconciles is where the identity meets pp.66-67 against
// pp.127-140 and 167-170.
//
// THE SUBJECT COUNT IS A COUNT AGAINST THE DOCUMENTS, which is why it is pinned:
// the spine publishes two adopted columns and the drill-down publishes those two
// and two more, p66 prints six fund groups, and pp.167-170 decompose one fund.
// Two columns, seven sides each. A change here is a change in what the city
// printed or in what the projections select from it, and either deserves to be
// read rather than absorbed.
func TestTheCommittedDrillReconciles(t *testing.T) {
	s, err := Load(LoadOptions{Root: repoRoot(t), Version: testVersion})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	res, err := runDrillReconcile(s, residualNodes)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != StatusPass {
		t.Fatalf("drill-reconciles-across-documents = %s:\n%s", res.Status,
			strings.Join(findingLines(res), "\n"))
	}
	if res.Subjects != 14 {
		t.Errorf("subjects = %d, want 14: two shared columns, each six groups' inflow and "+
			"the General Fund's outflow", res.Subjects)
	}
	for _, want := range []string{
		"FY2026 adopted: 6 fund groups' inflow and 1 decomposed group's outflow (fund-group/general)",
		"FY2027 adopted: 6 fund groups' inflow and 1 decomposed group's outflow (fund-group/general)",
	} {
		if !strings.Contains(res.Summary, want) {
			t.Errorf("summary does not say %q:\n%s", want, res.Summary)
		}
	}
	t.Logf("%s", res.Summary)
}
