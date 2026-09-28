package check

import (
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/google/go-cmp/cmp"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/mapping"
	"github.com/jcrussell/livermore-budget/internal/project"
	"github.com/jcrussell/livermore-budget/internal/registry"
	"github.com/jcrussell/livermore-budget/internal/structure"
)

// lineTieTaxonomyYAML puts lines under two categories of one fund group, so
// re-pointing a line onto its neighbour lands on a cell the spine publishes
// rather than failing for the wrong reason.
const lineTieTaxonomyYAML = linesTaxonomyYAML + `  - slug: taxes/property/current-secured
    label: Current Year - Secured
    document_term: Current Year - Secured
    parent: taxes/property
    kinds: [revenue]
    pages: [127]
  - slug: taxes/property/eraf
    label: ERAF
    document_term: ERAF
    parent: taxes/property
    kinds: [revenue]
    pages: [127]
`

// lineTieCells is the fixture spine with two cells added: a General Fund
// charges-for-services row, so that fund group carries two categories, and an
// enterprise transfer in, so a transfers/in flow can be drawn and reconciled
// without touching the General Fund one the shared exception holds apart.
var lineTieCells = append(slices.Clone(fixtureCells),
	testCell{mapping.KindRevenue, "charges-for-services", "general", 40_000},
	testCell{mapping.KindTransferIn, "transfers/in", "enterprise", 20_000},
)

const (
	securedLine = "revenue-line/taxes/property/current-secured"
	erafLine    = "revenue-line/taxes/property/eraf"
	libraryLine = "revenue-line/charges-for-services/library-fees"
)

// lineTieSubject is that spine and a drill-down that decomposes it, plus a
// second drill-down of a column the spine does not publish, as the corpus has.
// The ERAF contra row makes General Fund property tax tie only if the negative
// is summed with its sign.
//
//	general      revenue taxes/property        100,000  120,000 secured + (20,000) ERAF
//	             revenue charges-for-services   40,000  drawn as one line into fund/100
//	             transfer_in transfers/in       10,000  exempt: drawn by nothing
//	enterprise   revenue taxes/property              0  printed zero, no link
//	             revenue charges-for-services   50,000  drawn as one line into fund/500
//	             transfer_in transfers/in       20,000  drawn from the tier-0 endpoint
func lineTieSubject(t *testing.T) *Subject {
	t.Helper()
	reg, err := registry.Load(fstest.MapFS{
		registry.FundsFile:       &fstest.MapFile{Data: []byte(testFundsYAML)},
		registry.TaxonomyFile:    &fstest.MapFile{Data: []byte(lineTieTaxonomyYAML)},
		registry.DepartmentsFile: &fstest.MapFile{Data: []byte(testDepartmentsYAML)},
	})
	if err != nil {
		t.Fatalf("load the fixture registries: %v", err)
	}

	node := func(id, parent string, tier int) project.Node {
		return project.Node{ID: id, Parent: parent, Tier: tier}
	}
	link := func(src, dst string, cents int64) project.Link {
		return project.Link{Source: src, Target: dst, ValueCents: cents}
	}
	nodes := []project.Node{
		node("revenue/taxes/property", "", 0),
		node("revenue/charges-for-services", "", 0),
		node(transfersInNode, "", 0),
		node(securedLine, "revenue/taxes/property", 1),
		node(erafLine, "revenue/taxes/property", 1),
		node(libraryLine, "revenue/charges-for-services", 1),
		node("fund/100", "fund-group/general", 3),
		node("fund/500", "fund-group/enterprise", 3),
	}
	links := []project.Link{
		link(securedLine, "fund/100", 120_000),
		link(erafLine, "fund/100", -20_000),
		link(libraryLine, "fund/100", 40_000),
		link(libraryLine, "fund/500", 50_000),
		link(transfersInNode, "fund/500", 20_000),
	}
	drill := func(col project.Column) projection {
		return projection{
			Name:    project.FundFlowsProjection,
			Options: project.Options{Columns: []project.Column{col}, Scopes: project.FundFlowsScopes()},
			Graph: &project.Document{
				Nodes: slices.Clone(nodes), Links: slices.Clone(links),
			},
		}
	}
	return &Subject{
		Facts:      testFacts(lineTieCells...),
		Vocabulary: reg,
		Projections: []projection{
			drill(project.Column{FiscalYear: testYear, Basis: testBasis}),
			drill(project.Column{FiscalYear: 2024, Basis: mapping.BasisActual}),
		},
	}
}

// findingLines renders findings one per line, subject then detail, for a
// failure message a reader can grep.
func findingLines(res Result) []string {
	out := make([]string, 0, len(res.Findings))
	for _, f := range res.Findings {
		out = append(out, f.Subject+": "+f.Detail)
	}
	return out
}

// generalTransferInException is the declared exception for the General Fund
// transfer in pp.127-130 do not print, in one budget year, read off the
// structure rather than restated here.
func generalTransferInException(t *testing.T, year int) structure.Exception {
	t.Helper()
	for _, e := range structure.BudgetBookExceptions() {
		if e.Cut != revenueDetailCut || e.Against != spineCut {
			continue
		}
		for _, p := range e.Cells {
			if p.Year == year && !p.Cut.Present &&
				p.Coords[structure.AxisFundGroup] == "general" &&
				p.Coords[structure.AxisCategory] == "transfers/in" {
				return e
			}
		}
	}
	t.Fatalf("no exception holds the General Fund transfer in apart for FY%d", year)
	return structure.Exception{}
}

// lineNode is the first drill-down's node with this id, which every arm-two
// mutation edits. Addressed by id rather than by index: a fixture that gains a
// row must not silently re-aim a mutation at a different node.
func lineNode(t *testing.T, s *Subject, id string) *project.Node {
	t.Helper()
	nodes := s.Projections[0].Graph.Nodes
	for i := range nodes {
		if nodes[i].ID == id {
			return &nodes[i]
		}
	}
	t.Fatalf("the fixture carries no node %q", id)
	return nil
}

// lineLink is the first drill-down's link between these two ends.
func lineLink(t *testing.T, s *Subject, source, target string) *project.Link {
	t.Helper()
	links := s.Projections[0].Graph.Links
	for i := range links {
		if links[i].Source == source && links[i].Target == target {
			return &links[i]
		}
	}
	t.Fatalf("the fixture carries no link %q -> %q", source, target)
	return nil
}

func TestRevenueLinesTieOverTheFixture(t *testing.T) {
	res, err := (&revenueLinesTieToTheirCategories{}).Run(t.Context(), lineTieSubject(t))
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
		// Five cells — the six the two scopes key between them, less the
		// exempted General Fund transfer in — and six line nodes, three in each
		// document.
		Subjects: 11,
		Summary: "5 cells over 1 (fiscal year, basis) pair(s), summed from 10 flows into " +
			"funds, 1 of them a printed zero no link is drawn for; 6 revenue-line nodes " +
			"under their data/taxonomy.yaml category; 1 cell(s) held apart, not among the 5: " +
			generalTransferInException(t, 2026).Name + " (FY2026 adopted fund-group-by-category[category=transfers/in fund_group=general], " +
			"$100.00); 1 pair(s) with no spine column: FY2024 actual",
	}
	got := verdict{res.Status, res.Subjects, res.Summary}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("verdict mismatch (-want +got):\n%s\nfindings: %v", diff, res.Findings)
	}
}

// TestRevenueLinesTieIsFailable is the mutation table. The first row is green
// under every other check in this package.
func TestRevenueLinesTieIsFailable(t *testing.T) {
	cases := []struct {
		name         string
		damage       func(t *testing.T, s *Subject)
		wantStatus   Status
		wantSubjects int
		// want is every expected finding, in report order.
		want []string
	}{
		{
			name: "a line re-pointed onto its neighbour reddens both categories, equal and opposite",
			damage: func(t *testing.T, s *Subject) {
				lineNode(t, s, securedLine).Parent = "revenue/charges-for-services"
			},
			wantStatus: StatusFail, wantSubjects: 11,
			want: []string{
				`node "revenue-line/taxes/property/current-secured" is parented to ` +
					`"revenue/charges-for-services" and data/taxonomy.yaml declares it under ` +
					`"taxes/property", so it wants "revenue/taxes/property"`,
				"FY2026 adopted fund-group-by-category[category=charges-for-services fund_group=general]: the detail sums to $1600.00 " +
					"and the spine publishes $400.00, a difference of $1200.00",
				"FY2026 adopted fund-group-by-category[category=taxes/property fund_group=general]: the detail sums to -$200.00 and the " +
					"spine publishes $1000.00, a difference of -$1200.00",
			},
		},
		{
			// Invisible to node-hierarchy-well-formed; the money lands in a
			// cell whose category is empty.
			name:       "a line's parent blanked",
			damage:     func(t *testing.T, s *Subject) { lineNode(t, s, securedLine).Parent = "" },
			wantStatus: StatusFail, wantSubjects: 12,
			want: []string{
				`node "revenue-line/taxes/property/current-secured" is parented to "" and ` +
					`data/taxonomy.yaml declares it under "taxes/property"`,
				"FY2026 adopted fund-group-by-category[category=(absent) fund_group=general]: the detail publishes $1200.00 here and the spine " +
					"has no such cell",
				"FY2026 adopted fund-group-by-category[category=taxes/property fund_group=general]: the detail sums to -$200.00 and the " +
					"spine publishes $1000.00, a difference of -$1200.00",
			},
		},
		{
			name:       "a line parented to a category that does not exist",
			damage:     func(t *testing.T, s *Subject) { lineNode(t, s, securedLine).Parent = "revenue/typo" },
			wantStatus: StatusFail, wantSubjects: 12,
			want: []string{
				`node "revenue-line/taxes/property/current-secured" is parented to ` +
					`"revenue/typo" and data/taxonomy.yaml declares it under "taxes/property"`,
				"FY2026 adopted fund-group-by-category[category=taxes/property fund_group=general]: the detail sums to -$200.00 and the " +
					"spine publishes $1000.00, a difference of -$1200.00",
				"FY2026 adopted fund-group-by-category[category=typo fund_group=general]: the detail publishes $1200.00 here and the " +
					"spine has no such cell",
			},
		},
		{
			// Arm one is untouched: the money still lands in the right cell.
			name: "the category node removed from the document",
			damage: func(_ *testing.T, s *Subject) {
				d := s.Projections[0].Graph
				d.Nodes = slices.DeleteFunc(slices.Clone(d.Nodes), func(n project.Node) bool {
					return n.ID == "revenue/taxes/property"
				})
			},
			wantStatus: StatusFail, wantSubjects: 11,
			want: []string{
				`node "revenue-line/taxes/property/current-secured" is parented to ` +
					`"revenue/taxes/property", which data/taxonomy.yaml declares and this ` +
					`document does not carry`,
				`node "revenue-line/taxes/property/eraf" is parented to ` +
					`"revenue/taxes/property", which data/taxonomy.yaml declares and this ` +
					`document does not carry`,
			},
		},
		{
			// A renamed taxonomy slug: the document still adds up.
			name: "a line the taxonomy declares nothing about",
			damage: func(t *testing.T, s *Subject) {
				lineLink(t, s, securedLine, "fund/100").Source = "revenue-line/taxes/property/invented"
				lineNode(t, s, securedLine).ID = "revenue-line/taxes/property/invented"
			},
			wantStatus: StatusFail, wantSubjects: 11,
			want: []string{
				`node "revenue-line/taxes/property/invented" names no data/taxonomy.yaml entry`,
			},
		},
		{
			// The fund group is the registry's, not the fund node's parent.
			name:       "a link re-targeted at a fund of another group",
			damage:     func(t *testing.T, s *Subject) { lineLink(t, s, libraryLine, "fund/500").Target = "fund/700" },
			wantStatus: StatusFail, wantSubjects: 12,
			want: []string{
				"FY2026 adopted fund-group-by-category[category=charges-for-services fund_group=enterprise]: the spine publishes $500.00 " +
					"here and the detail has no such row at all",
				"FY2026 adopted fund-group-by-category[category=charges-for-services fund_group=internal-service]: the detail publishes " +
					"$500.00 here and the spine has no such cell",
			},
		},
		{
			// A mis-parented fund cannot reconcile against the group it claims.
			name: "a fund re-parented in the document changes nothing",
			damage: func(t *testing.T, s *Subject) {
				lineNode(t, s, "fund/500").Parent = "fund-group/general"
			},
			wantStatus: StatusPass, wantSubjects: 11,
		},
		{
			name:       "a category the drill-down stopped drawing",
			damage:     func(t *testing.T, s *Subject) { lineLink(t, s, securedLine, "fund/100").ValueCents = 0 },
			wantStatus: StatusFail, wantSubjects: 11,
			want: []string{
				"FY2026 adopted fund-group-by-category[category=taxes/property fund_group=general]: the detail sums to -$200.00 and the " +
					"spine publishes $1000.00, a difference of -$1200.00",
			},
		},
		{
			// Taking a contra's absolute value puts the cell out by twice the
			// row; the base fixture passing is the other half of the proof.
			name: "a contra row drawn positive",
			damage: func(t *testing.T, s *Subject) {
				lineLink(t, s, erafLine, "fund/100").ValueCents = 20_000
			},
			wantStatus: StatusFail, wantSubjects: 11,
			want: []string{
				"FY2026 adopted fund-group-by-category[category=taxes/property fund_group=general]: the detail sums to $1400.00 and the " +
					"spine publishes $1000.00, a difference of $400.00",
			},
		},
		{
			// Neither a line nor the transfer endpoint: money summed into no cell.
			name:       "a third shape of flow into a fund",
			damage:     func(t *testing.T, s *Subject) { lineLink(t, s, libraryLine, "fund/100").Source = "fund-balance/draw" },
			wantStatus: StatusFail, wantSubjects: 11,
			want: []string{
				`link "fund-balance/draw" -> "fund/100" reaches a fund from a node that is ` +
					`neither a revenue-line/ line this document carries nor "transfers/in"`,
				"FY2026 adopted fund-group-by-category[category=charges-for-services fund_group=general]: the spine publishes $400.00 " +
					"here and the detail has no such row at all",
			},
		},
		{
			// The exemption must not outlive its claim about the document.
			name: "the exempted cell drawn after all",
			damage: func(_ *testing.T, s *Subject) {
				d := s.Projections[0].Graph
				d.Links = append(slices.Clone(d.Links),
					project.Link{Source: transfersInNode, Target: "fund/100", ValueCents: 10_000})
			},
			wantStatus: StatusFail, wantSubjects: 12,
			want: []string{
				"pp.127-130-print-no-general-fund-transfer-in-2026: the drill-down draws a flow " +
					"into FY2026 adopted fund-group-by-category[category=transfers/in fund_group=general], so the exception it is exempted " +
					"by has stopped describing the document",
			},
		},
		{
			// Reachable only through a hand-built document; the group must not
			// be guessed from the node's parent.
			name:       "a link into a fund data/funds.yaml does not list",
			damage:     func(t *testing.T, s *Subject) { lineLink(t, s, transfersInNode, "fund/500").Target = "fund/999" },
			wantStatus: StatusFail, wantSubjects: 11,
			want: []string{
				`link "transfers/in" -> "fund/999" names a fund data/funds.yaml does not list`,
				"FY2026 adopted fund-group-by-category[category=transfers/in fund_group=enterprise]: the spine publishes $200.00 here " +
					"and the detail has no such row at all",
			},
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			s := lineTieSubject(t)
			tt.damage(t, s)
			res, err := (&revenueLinesTieToTheirCategories{}).Run(t.Context(), s)
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			if res.Status != tt.wantStatus || res.Subjects != tt.wantSubjects {
				t.Errorf("status %s over %d subjects, want %s over %d",
					res.Status, res.Subjects, tt.wantStatus, tt.wantSubjects)
			}
			got := findingLines(res)
			if len(got) != len(tt.want) {
				t.Fatalf("%d findings, want %d:\n%s", len(got), len(tt.want),
					strings.Join(got, "\n"))
			}
			for i, want := range tt.want {
				if !strings.Contains(got[i], want) {
					t.Errorf("finding %d is %q, want it to contain %q", i, got[i], want)
				}
			}
		})
	}
}

// TestRevenueLinesTieIsVacuousWithoutADrillDown pins that no drill-down is
// vacuous, not a pass.
func TestRevenueLinesTieIsVacuousWithoutADrillDown(t *testing.T) {
	s := lineTieSubject(t)
	s.Projections = nil
	res, err := (&revenueLinesTieToTheirCategories{}).Run(t.Context(), s)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != StatusVacuous || res.Subjects != 0 {
		t.Errorf("status %s over %d subjects, want vacuous over 0", res.Status, res.Subjects)
	}
	if res.Findings == nil {
		t.Error("findings are nil, want an empty slice")
	}
}

// TestRevenueLinesTieRefusesATwoColumnDrillDown covers a shape internal/project
// never builds: links that cannot be keyed to a fiscal year.
func TestRevenueLinesTieRefusesATwoColumnDrillDown(t *testing.T) {
	s := lineTieSubject(t)
	s.Projections[0].Options.Columns = []project.Column{
		{FiscalYear: testYear, Basis: testBasis},
		{FiscalYear: 2027, Basis: testBasis},
	}
	res, err := (&revenueLinesTieToTheirCategories{}).Run(t.Context(), s)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	want := "this document is of 2 columns and a flow diagram is of one"
	got := findingLines(res)
	if res.Status != StatusFail || len(got) == 0 || !strings.Contains(got[0], want) {
		t.Errorf("status %s with findings %v, want fail whose first finding contains %q",
			res.Status, got, want)
	}
}

// TestRevenueLinesTieReadsTheFactsItIsGiven: the spine side is the fact store,
// not the spine graph.
func TestRevenueLinesTieReadsTheFactsItIsGiven(t *testing.T) {
	s := lineTieSubject(t)
	s.Facts = slices.DeleteFunc(s.Facts, func(f fact.Fact) bool {
		return f.Kind == mapping.KindRevenue && f.Category == "taxes/property" &&
			f.FundGroup == "general"
	})
	res, err := (&revenueLinesTieToTheirCategories{}).Run(t.Context(), s)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != StatusFail {
		t.Fatalf("status %s, want fail", res.Status)
	}
	want := "FY2026 adopted fund-group-by-category[category=taxes/property fund_group=general]: the detail publishes $1000.00 here and " +
		"the spine has no such cell"
	if got := findingLines(res); len(got) != 1 || !strings.Contains(got[0], want) {
		t.Errorf("findings are %v, want one containing %q", got, want)
	}
}
