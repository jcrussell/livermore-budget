package project

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/mapping"
	"github.com/jcrussell/livermore-budget/internal/registry"
)

// goldenPath is the hand-derived worked example: real FY2026 figures read out
// of the committed pp.66-67 artifacts, with real content-addressed fact ids.
const goldenPath = "../../testdata/sankey.golden.json"

// TestSankeyReproducesGoldenFile is the test this package exists to pass.
//
// It compares the decoded graph first, because a field-level diff says what is
// wrong, and the bytes second, because only the bytes catch key order, indent
// and HTML escaping — the parts of the contract a decoded comparison cannot
// see.
func TestSankeyReproducesGoldenFile(t *testing.T) {
	wantBytes, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}

	var want Graph
	dec := json.NewDecoder(bytes.NewReader(wantBytes))
	// A key in the golden file this package has no field for is a contract we
	// are not implementing, not a key to skip.
	dec.DisallowUnknownFields()
	if err = dec.Decode(&want); err != nil {
		t.Fatalf("decode golden: %v", err)
	}

	got := buildGraph(t, spineFacts(t, testYear), testOptions())
	if diff := cmp.Diff(want, *got); diff != "" {
		t.Errorf("graph mismatch (-want +got):\n%s", diff)
	}

	gotBytes, err := (&Sankey{Labels: goldenLabels}).Build(spineFacts(t, testYear), testOptions())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if !bytes.Equal(gotBytes, wantBytes) {
		t.Errorf("encoded bytes differ from %s (got %d bytes, want %d)",
			goldenPath, len(gotBytes), len(wantBytes))
	}
}

// TestFiscalYearFilter is the trap that no consistency check can catch: both
// budget years live in one facts.jsonl, and a projection that forgets to
// filter doubles every figure while still balancing perfectly.
func TestFiscalYearFilter(t *testing.T) {
	both := append(spineFacts(t, testYear), spineFacts(t, testYear+1)...)

	got := buildGraph(t, both, testOptions())

	if got.Metadata.Counts.Facts != 120 {
		t.Errorf("got %d facts, want 120 (the FY%d half of %d)", got.Metadata.Counts.Facts, testYear, len(both))
	}
	if got.Metadata.Counts.FactsCited != 58 {
		t.Errorf("got %d cited facts, want 58", got.Metadata.Counts.FactsCited)
	}
	if got.Metadata.Counts.Links != 58 {
		t.Errorf("got %d links, want 58", got.Metadata.Counts.Links)
	}
	if got, want := got.Metadata.Headline.AllFundsGrossRevenueCents, int64(29996900700); got != want {
		t.Errorf("got %d gross revenue cents, want %d", got, want)
	}
	if got, want := got.Metadata.Headline.AllFundsGrossExpenditureCents, int64(25409541200); got != want {
		t.Errorf("got %d gross expenditure cents, want %d", got, want)
	}
}

// TestBasisAndScopeFilter checks the other two selectors. A revised column
// beside an adopted one, or a department schedule on top of the citywide
// spine, would both produce a graph that balances and is wrong.
func TestBasisAndScopeFilter(t *testing.T) {
	spine := spineFacts(t, testYear)

	other := make([]fact.Fact, 0, len(spine)*2)
	for _, f := range spine {
		revised := f
		revised.Basis = mapping.BasisRevised
		revised.ID += "-revised"
		elsewhere := f
		elsewhere.Scope = "general-fund"
		elsewhere.ID += "-elsewhere"
		other = append(other, revised, elsewhere)
	}

	got := buildGraph(t, append(spine, other...), testOptions())
	if got.Metadata.Counts.Facts != len(spine) {
		t.Errorf("got %d facts, want %d", got.Metadata.Counts.Facts, len(spine))
	}
	if got, want := got.Metadata.Headline.AllFundsGrossRevenueCents, int64(29996900700); got != want {
		t.Errorf("got %d gross revenue cents, want %d", got, want)
	}
}

// TestChangeInWorkingCapitalSign covers the one row this projection infers
// from. The city prints it once, signed; a Sankey cannot draw a negative link.
func TestChangeInWorkingCapitalSign(t *testing.T) {
	change := func(group string, cents int64) cellSpec {
		return cellSpec{
			kind:     mapping.KindFundBalance,
			category: categoryFundBalanceChange,
			label:    "CHANGE IN WORKING CAPITAL",
			group:    group,
			cents:    cents,
		}
	}

	g := buildGraph(t, facts(t,
		change("general", -103415400),
		change("enterprise", 389438400),
		change("capital", 0),
	), testOptions())

	draw := linkBetween(t, g, NodeFundBalanceDraw, "fund-group/general")
	if draw.ValueCents != 103415400 {
		t.Errorf("got draw of %d cents, want 103415400 (the negation of the printed figure)", draw.ValueCents)
	}
	if draw.Kind != KindFundBalance {
		t.Errorf("got kind %q, want %q", draw.Kind, KindFundBalance)
	}
	if !draw.Derived {
		t.Error("got derived false on the draw link, want true")
	}

	contribution := linkBetween(t, g, "fund-group/enterprise", NodeFundBalanceContribution)
	if contribution.ValueCents != 389438400 {
		t.Errorf("got contribution of %d cents, want 389438400", contribution.ValueCents)
	}
	if !contribution.Derived {
		t.Error("got derived false on the contribution link, want true")
	}

	// A zero change is neither a draw nor a contribution, and the fact behind
	// it is still counted.
	if hasLink(g, NodeFundBalanceDraw, "fund-group/capital") ||
		hasLink(g, "fund-group/capital", NodeFundBalanceContribution) {
		t.Error("got a link for a zero CHANGE IN WORKING CAPITAL, want none")
	}
	if g.Metadata.Counts.Facts != 3 || g.Metadata.Counts.Links != 2 {
		t.Errorf("got %d facts and %d links, want 3 and 2",
			g.Metadata.Counts.Facts, g.Metadata.Counts.Links)
	}

	// Neither side of the decomposition is in the headline: a fund drawing on
	// its own balance is not revenue.
	if h := g.Metadata.Headline; h.AllFundsGrossRevenueCents != 0 || h.ExternalRevenueCents != 0 {
		t.Errorf("got %+v, want no revenue from a fund-balance row", h)
	}
}

// TestDerivedNodesCarryProvenance guards the rule fisc verify enforces:
// derived: true without a rationale and a source note is an unsupported claim.
func TestDerivedNodesCarryProvenance(t *testing.T) {
	g := buildGraph(t, spineFacts(t, testYear), testOptions())

	derivedIDs := make(map[string]bool)
	for _, n := range g.Nodes {
		if !n.Derived {
			continue
		}
		derivedIDs[n.ID] = true
		if n.Rationale == "" {
			t.Errorf("node %q is derived with no rationale", n.ID)
		}
		if n.SourceNote == "" {
			t.Errorf("node %q is derived with no source note", n.ID)
		}
	}

	want := map[string]bool{NodeFundBalanceDraw: true, NodeFundBalanceContribution: true}
	if diff := cmp.Diff(want, derivedIDs); diff != "" {
		t.Errorf("derived nodes (-want +got):\n%s", diff)
	}

	// The rest of the spine is the city's own arithmetic and must not claim
	// otherwise.
	for _, n := range g.Nodes {
		if !n.Derived && (n.Rationale != "" || n.SourceNote != "") {
			t.Errorf("node %q is not derived but carries a rationale or source note", n.ID)
		}
	}
}

// TestStocksAreExcluded keeps BEGINNING and ENDING WORKING CAPITAL out of the
// graph. They are cells the city printed, so they are facts; they are stocks,
// so they are not flows.
func TestStocksAreExcluded(t *testing.T) {
	g := buildGraph(t, facts(t,
		cellSpec{kind: mapping.KindRevenue, category: "taxes/property", label: "Property Taxes",
			group: "general", cents: 6414376200},
		cellSpec{kind: mapping.KindFundBalance, category: categoryFundBalanceBeginning,
			label: "BEGINNING WORKING CAPITAL", group: "general", cents: 176661300},
		cellSpec{kind: mapping.KindFundBalance, category: categoryFundBalanceEnding,
			label: "ENDING WORKING CAPITAL", group: "general", cents: 73245900},
	), testOptions())

	if g.Metadata.Counts.Facts != 3 {
		t.Errorf("got %d facts, want 3: a stock the city printed is still a fact", g.Metadata.Counts.Facts)
	}
	if g.Metadata.Counts.Links != 1 {
		t.Errorf("got %d links, want 1", g.Metadata.Counts.Links)
	}
	for _, n := range g.Nodes {
		if strings.HasPrefix(n.ID, categoryFundBalanceBeginning) || strings.HasPrefix(n.ID, categoryFundBalanceEnding) {
			t.Errorf("got a node %q for a stock, want none", n.ID)
		}
	}
	if !hasCaveat(g, "stocks") {
		t.Error("no caveat mentions stocks; a reader reconciling against p66 finds three rows missing")
	}
}

// TestZeroValuedCellKeepsFactAndDropsLink covers the dashes. Most of the
// revenue grid is zero, d3-sankey draws zero-height paths that churn node
// order, and a zero the city printed is still a fact.
func TestZeroValuedCellKeepsFactAndDropsLink(t *testing.T) {
	g := buildGraph(t, facts(t,
		cellSpec{kind: mapping.KindRevenue, category: "taxes/property", label: "Property Taxes",
			group: "general", cents: 6414376200},
		cellSpec{kind: mapping.KindRevenue, category: "taxes/property", label: "Property Taxes",
			group: "enterprise", cents: 0},
	), testOptions())

	if g.Metadata.Counts.Facts != 2 {
		t.Errorf("got %d facts, want 2", g.Metadata.Counts.Facts)
	}
	if g.Metadata.Counts.Links != 1 {
		t.Errorf("got %d links, want 1", g.Metadata.Counts.Links)
	}
	if g.Metadata.Counts.Nodes != 2 {
		t.Errorf("got %d nodes, want 2: the enterprise column has no flow to draw", g.Metadata.Counts.Nodes)
	}
}

// TestInternalServiceClassification is the 6.3% version of the error this
// projection exists to prevent. Internal Service Fund charges are real money
// in a real fund, and counting them as revenue and as the paying department's
// expenditure double-counts them.
func TestInternalServiceClassification(t *testing.T) {
	g := buildGraph(t, facts(t,
		cellSpec{kind: mapping.KindRevenue, category: "intergovernmental", label: "Intergovernmental",
			group: "internal-service", cents: 1884483400},
		cellSpec{kind: mapping.KindRevenue, category: "intergovernmental", label: "Intergovernmental",
			group: "general", cents: 432816400},
		cellSpec{kind: mapping.KindExpenditure, category: "wages-and-benefits", label: "Wages & Benefits",
			group: "internal-service", cents: 597065400},
		cellSpec{kind: mapping.KindExpenditure, category: "wages-and-benefits", label: "Wages & Benefits",
			group: "general", cents: 8180101100},
	), testOptions())

	if got := linkBetween(t, g, "revenue/intergovernmental", "fund-group/internal-service").Kind; got != KindInternalService {
		t.Errorf("got kind %q on an internal service revenue link, want %q", got, KindInternalService)
	}
	if got := linkBetween(t, g, "fund-group/internal-service", "expenditure/wages-and-benefits").Kind; got != KindInternalService {
		t.Errorf("got kind %q on an internal service expenditure link, want %q", got, KindInternalService)
	}
	if got := linkBetween(t, g, "revenue/intergovernmental", "fund-group/general").Kind; got != KindExternal {
		t.Errorf("got kind %q on a general fund revenue link, want %q", got, KindExternal)
	}

	h := g.Metadata.Headline
	if h.AllFundsGrossRevenueCents != 1884483400+432816400 {
		t.Errorf("got %d gross revenue cents, want the internal service charges included", h.AllFundsGrossRevenueCents)
	}
	if h.ExternalRevenueCents != 432816400 {
		t.Errorf("got %d external revenue cents, want %d: net of internal service",
			h.ExternalRevenueCents, 432816400)
	}
	if h.AllFundsGrossExpenditureCents != 597065400+8180101100 {
		t.Errorf("got %d gross expenditure cents, want the internal service spending included",
			h.AllFundsGrossExpenditureCents)
	}
	if h.ExternalExpenditureCents != 8180101100 {
		t.Errorf("got %d external expenditure cents, want %d", h.ExternalExpenditureCents, 8180101100)
	}
	if !hasCaveat(g, "internal_service") {
		t.Error("no caveat explains the internal service classification")
	}
}

// TestHeadlineNaiveAndResidual checks the two figures that exist to be wrong
// on purpose and to go stale-red.
func TestHeadlineNaiveAndResidual(t *testing.T) {
	g := buildGraph(t, spineFacts(t, testYear), testOptions())
	h := g.Metadata.Headline

	if h.NaiveExpenditureCents != 31370814600 {
		t.Errorf("got %d naive expenditure cents, want 31370814600", h.NaiveExpenditureCents)
	}
	if h.NaiveExpenditureCents == h.AllFundsGrossExpenditureCents {
		t.Error("the naive figure equals the real one; it exists to differ from it")
	}
	if h.TransferResidualCents != h.InternalTransferOutCents-h.InternalTransferInCents {
		t.Errorf("got residual %d, want out %d minus in %d",
			h.TransferResidualCents, h.InternalTransferOutCents, h.InternalTransferInCents)
	}
	if !hasCaveat(g, "transfer_id") {
		t.Error("no caveat says the transfer legs are unpaired")
	}
	if !strings.Contains(caveatText(g), "$38,086,737") {
		t.Errorf("the transfer caveat does not quote the residual:\n%s", caveatText(g))
	}
}

// TestFundGroupsBalance is the city checking our work rather than us checking
// our own: these are its own TOTAL SOURCES and TOTAL USES rows.
func TestFundGroupsBalance(t *testing.T) {
	g := buildGraph(t, spineFacts(t, testYear), testOptions())

	in := make(map[string]int64)
	out := make(map[string]int64)
	for _, l := range g.Links {
		if strings.HasPrefix(l.Target, prefixFundGroup) {
			in[l.Target] += l.ValueCents
		}
		if strings.HasPrefix(l.Source, prefixFundGroup) {
			out[l.Source] += l.ValueCents
		}
	}

	want := map[string]int64{
		"fund-group/general":          15938802400,
		"fund-group/enterprise":       8076126100,
		"fund-group/capital":          2964609500,
		"fund-group/debt-service":     698459700,
		"fund-group/special-revenue":  2927956000,
		"fund-group/internal-service": 2511736700,
	}
	if diff := cmp.Diff(want, in); diff != "" {
		t.Errorf("sources into fund groups (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(want, out); diff != "" {
		t.Errorf("uses out of fund groups (-want +got):\n%s", diff)
	}
}

// TestContraRowNetsIntoParent synthesizes the case pp.66-67 do not print. The
// contra rows are the p127 ERAF and RPTTF property-tax shifts, ~26% of gross
// property tax, and a Sankey cannot render them as their own inbound flow.
func TestContraRowNetsIntoParent(t *testing.T) {
	gross := int64(9821444000)
	eraf := int64(-1585787500)
	rpttf := int64(-189180400)

	g := buildGraph(t, facts(t,
		cellSpec{kind: mapping.KindRevenue, category: "taxes/property",
			label: "Current Year - Secured", group: "general", cents: gross},
		cellSpec{kind: mapping.KindRevenue, category: "taxes/property",
			label: "ERAF", group: "general", cents: eraf, sign: mapping.SignContra},
		cellSpec{kind: mapping.KindRevenue, category: "taxes/property",
			label: "RPTTF Reduction", group: "general", cents: rpttf, sign: mapping.SignContra},
	), testOptions())

	l := linkBetween(t, g, "revenue/taxes/property", "fund-group/general")
	if want := gross + eraf + rpttf; l.ValueCents != want {
		t.Errorf("got %d cents, want %d: the contra rows net into their parent", l.ValueCents, want)
	}
	if len(l.FactIDs) != 3 {
		t.Errorf("got %d fact ids, want 3: a netted link cites every row behind it", len(l.FactIDs))
	}
	if !sortedStrings(l.FactIDs) {
		t.Errorf("got fact ids %v, want them ascending", l.FactIDs)
	}
	if l.Derived {
		t.Error("got derived true on a netted link; netting is arithmetic the city's own subtotal does")
	}
	if g.Metadata.Counts.Facts != 3 || g.Metadata.Counts.Links != 1 {
		t.Errorf("got %d facts and %d links, want 3 and 1",
			g.Metadata.Counts.Facts, g.Metadata.Counts.Links)
	}
}

// TestDeterminism is what makes a rebuild-and-diff check meaningful.
func TestDeterminism(t *testing.T) {
	s := &Sankey{Labels: goldenLabels}

	first, err := s.Build(spineFacts(t, testYear), testOptions())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	second, err := s.Build(spineFacts(t, testYear), testOptions())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Error("two builds of the same facts differ")
	}

	// Fact order must not matter either: the fact file's canonical order is
	// not this projection's, and a map iteration leaking into the output would
	// show up here rather than as a mystery diff in CI.
	shuffled := spineFacts(t, testYear)
	for i, j := 0, len(shuffled)-1; i < j; i, j = i+1, j-1 {
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	}
	reversed, err := s.Build(shuffled, testOptions())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if !bytes.Equal(first, reversed) {
		t.Error("reversing the input facts changed the output")
	}
}

// TestNoNullsOrMissingKeys enforces the contract's shape rule directly on the
// bytes: every key present on every object, no null anywhere.
func TestNoNullsOrMissingKeys(t *testing.T) {
	out, err := (&Sankey{}).Build(nil, testOptions())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if bytes.Contains(out, []byte("null")) {
		t.Errorf("an empty projection contains null:\n%s", out)
	}

	var doc map[string]json.RawMessage
	if err := json.Unmarshal(out, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, key := range []string{"schema_version", "projection", "metadata", "nodes", "links"} {
		if _, ok := doc[key]; !ok {
			t.Errorf("key %q is missing from an empty projection", key)
		}
	}
	if !bytes.Contains(out, []byte(`"nodes": []`)) || !bytes.Contains(out, []byte(`"links": []`)) {
		t.Errorf("empty nodes and links must be [] rather than null:\n%s", out)
	}
}

// TestLabelFallback covers the three ways a node gets its words. The registry
// is optional on purpose: this package has to build without data/taxonomy.yaml
// so that a missing registry degrades to an ugly label rather than to no site.
func TestLabelFallback(t *testing.T) {
	spine := func(t *testing.T) []fact.Fact {
		t.Helper()
		return facts(t,
			cellSpec{kind: mapping.KindRevenue, category: "taxes/property",
				label: "Property Taxes", group: "general", cents: 100},
			cellSpec{kind: mapping.KindTransferIn, category: "transfers/in",
				label: "TRANSFER IN:", group: "general", cents: 100})
	}

	// No registry: the nodes in builtinLabels keep their words, and a category
	// falls back to its slug.
	graph, err := (&Sankey{}).Graph(spine(t), testOptions())
	if err != nil {
		t.Fatalf("Graph: %v", err)
	}
	if got := nodeByID(t, graph, "revenue/taxes/property").Label; got != "Property" {
		t.Errorf("got %q, want the slug-derived %q", got, "Property")
	}
	if got := nodeByID(t, graph, "transfers/in").Label; got != "Transfers In" {
		t.Errorf("got %q, want the built-in %q", got, "Transfers In")
	}
	if got := nodeByID(t, graph, "fund-group/general").Label; got != "General Fund" {
		t.Errorf("got %q, want the built-in %q", got, "General Fund")
	}

	// A registry wins for any node that has a category slug, including over a
	// built-in label. It is never consulted for a fund group, which has no
	// category and whose name is a data/funds.yaml fund type.
	withLabels := &Sankey{Labels: stubLabels{
		"taxes/property": "Property Taxes",
		"transfers/in":   "Transfers In (p76)",
		"general":        "General (from the wrong vocabulary)",
	}}
	// transfers/in has a built-in label, and a built-in wins: the words on a
	// node must be the words on the page this projection read, while a
	// taxonomy label names a category that may span several schedules. See
	// (*Sankey).label.
	graph, err = withLabels.Graph(spine(t), testOptions())
	if err != nil {
		t.Fatalf("Graph: %v", err)
	}
	if got := nodeByID(t, graph, "revenue/taxes/property").Label; got != "Property Taxes" {
		t.Errorf("got %q, want %q", got, "Property Taxes")
	}
	if got := nodeByID(t, graph, "transfers/in").Label; got != "Transfers In" {
		t.Errorf("got %q, want the built-in %q: a built-in label wins over the registry", got, "Transfers In")
	}
	// A category with no built-in does take the registry's words.
	if got := nodeByID(t, graph, "revenue/taxes/property").Label; got != "Property Taxes" {
		t.Errorf("got %q, want %q", got, "Property Taxes")
	}
	if got := nodeByID(t, graph, "fund-group/general").Label; got != "General Fund" {
		t.Errorf("got %q, want the built-in %q: a fund group is not a category", got, "General Fund")
	}
}

// TestRegistrySatisfiesLabels is the compile-time check that the interface
// this package declares is the one internal/registry implements. It is a test
// rather than a package-scope assertion so that internal/project does not
// import the registry it was deliberately decoupled from.
func TestRegistrySatisfiesLabels(t *testing.T) {
	var _ Labels = (*registry.Registry)(nil)
}

// TestSources reports the pages actually read, so the citation on the page
// names pages rather than a document.
func TestSources(t *testing.T) {
	g := buildGraph(t, spineFacts(t, testYear), testOptions())
	want := []Source{{DocID: testDoc, Pages: []int{66, 67}}}
	if diff := cmp.Diff(want, g.Metadata.Sources); diff != "" {
		t.Errorf("sources (-want +got):\n%s", diff)
	}
}

// TestGraphRejects covers the facts this projection refuses rather than
// silently mislays. Each of them would otherwise publish a wrong number with a
// working provenance link.
func TestGraphRejects(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(f *fact.Fact)
		want   string
	}{
		{"no category", func(f *fact.Fact) { f.Category = "" }, "has no category"},
		{"no fund group", func(f *fact.Fact) { f.FundGroup = "" }, "has no fund group"},
		{"a department", func(f *fact.Fact) { f.Department = "police" }, "carries department"},
		{"unknown kind", func(f *fact.Fact) { f.Kind = "grant" }, "unknown kind"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fs := facts(t, cellSpec{kind: mapping.KindRevenue, category: "taxes/property",
				label: "Property Taxes", group: "general", cents: 100})
			c.mutate(&fs[0])

			_, err := (&Sankey{}).Graph(fs, testOptions())
			if err == nil {
				t.Fatalf("got no error, want one containing %q", c.want)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("got %q, want it to contain %q", err.Error(), c.want)
			}
		})
	}
}

// TestGraphRejectsBadOptions makes sure the seam validates before it filters:
// an unvalidated projection of year zero is an empty document, not an error,
// and an empty document is the failure that ships.
func TestGraphRejectsBadOptions(t *testing.T) {
	_, err := (&Sankey{}).Graph(spineFacts(t, testYear), Options{})
	if err == nil {
		t.Fatal("got no error, want one")
	}
	if !strings.Contains(err.Error(), "sankey options") {
		t.Errorf("got %q, want it to name the projection", err.Error())
	}
}

// TestGraphRefusesAForeignSchedule calls Graph directly with another
// schedule's scope, which is the only way to reach the refusal: Slices pins
// PublishedScope, so nothing in the tree asks for this today.
//
// It is worth a test anyway, and the reason is what the facts below do without
// it. pp.127-140's revenue-by-fund rows net perfectly well on
// (kind, category, fund_group) -- they carry all three -- so before this
// refusal Graph returned a FULLY FORMED graph over them, with
// metadata.scope "revenue-by-fund" and a headline that is a different total
// from the spine's. A wrong document rather than an error, which is the shape
// Options.Scope exists to prevent.
func TestGraphRefusesAForeignSchedule(t *testing.T) {
	o := testOptions()
	o.Scopes = []string{TrendsScope}

	_, err := (&Sankey{}).Graph(spineFacts(t, testYear), o)
	if err == nil {
		t.Fatal("got no error, want one")
	}
	for _, want := range []string{"sankey:", TrendsScope, PublishedScope} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("got %q, want it to contain %q", err.Error(), want)
		}
	}
}

// TestNodeTiersAndRoles pins the two hierarchy levels this schedule publishes
// and the flow endpoints that sit outside it.
func TestNodeTiersAndRoles(t *testing.T) {
	g := buildGraph(t, spineFacts(t, testYear), testOptions())

	want := map[string][2]any{
		"revenue/taxes/property":         {tierRevenueSource, roleRevenueSource},
		"transfers/in":                   {tierRevenueSource, roleTransferIn},
		NodeFundBalanceDraw:              {tierRevenueSource, roleFundBalanceDraw},
		"fund-group/general":             {tierFundGroup, roleFundGroup},
		"expenditure/wages-and-benefits": {tierObjectCategory, roleObjectCategory},
		"transfers/out":                  {tierObjectCategory, roleTransferOut},
		"fund-balance/reserve-increase":  {tierObjectCategory, roleReserveIncrease},
		NodeFundBalanceContribution:      {tierObjectCategory, roleFundBalanceContribution},
	}
	for id, w := range want {
		n := nodeByID(t, g, id)
		if n.Tier != w[0].(int) || n.Role != w[1].(string) {
			t.Errorf("node %q: got tier %d role %q, want tier %d role %q", id, n.Tier, n.Role, w[0], w[1])
		}
		if n.Parent != "" || n.ConstraintTier != "" {
			t.Errorf("node %q: got parent %q constraint tier %q, want both empty on a fund-group schedule",
				id, n.Parent, n.ConstraintTier)
		}
	}
}

// TestNodesAndLinksAreSorted states the ordering the contract fixes, on the
// real fixture rather than on a constructed one.
func TestNodesAndLinksAreSorted(t *testing.T) {
	g := buildGraph(t, spineFacts(t, testYear), testOptions())

	for i := 1; i < len(g.Nodes); i++ {
		a, b := g.Nodes[i-1], g.Nodes[i]
		if a.Tier > b.Tier || (a.Tier == b.Tier && a.ID >= b.ID) {
			t.Errorf("nodes out of (tier, id) order at %d: %d %q then %d %q", i, a.Tier, a.ID, b.Tier, b.ID)
		}
	}
	for i := 1; i < len(g.Links); i++ {
		a, b := g.Links[i-1], g.Links[i]
		if a.Source > b.Source || (a.Source == b.Source && a.Target >= b.Target) {
			t.Errorf("links out of (source, target) order at %d: %q->%q then %q->%q",
				i, a.Source, a.Target, b.Source, b.Target)
		}
	}
}

// TestTransferCaveatWhenLegsMatch covers the phrasing the golden file cannot:
// equal totals are not evidence that the legs pair up.
func TestTransferCaveatWhenLegsMatch(t *testing.T) {
	g := buildGraph(t, facts(t,
		cellSpec{kind: mapping.KindTransferIn, category: "transfers/in", label: "TRANSFER IN:",
			group: "general", cents: 100000},
		cellSpec{kind: mapping.KindTransferOut, category: "transfers/out", label: "TRANSFER OUT:",
			group: "enterprise", cents: 100000},
	), testOptions())

	if g.Metadata.Headline.TransferResidualCents != 0 {
		t.Fatalf("got residual %d, want 0", g.Metadata.Headline.TransferResidualCents)
	}
	text := caveatText(g)
	if !strings.Contains(text, "is not evidence the legs pair up") {
		t.Errorf("got caveats:\n%s\nwant one saying matching totals prove nothing", text)
	}
}

// TestTransferCaveatNamesThePrintedColumn is the evidence for the correction
// this caveat carries, and it asserts the two halves separately because they
// went stale for different reasons.
//
// THE STALE REASON. Until ced45b4 the caveat said the legs were unpaired
// "because the p76 transfer schedule is not yet mapped (fisc-5gk.3)". p76 was
// mapped and published in that commit and the legs stayed unpaired, because its
// facts are at scope transfers-by-fund and no projection selects it. The
// sentence was true when written in 45235d3 and nobody went back, which is the
// failure mode this test exists to make loud.
//
// THE PRINTED COLUMN. The residual is not a discrepancy: the city prints it
// under a heading of its own. The figure asserted here is the one
// internal/check/transfersdetail.go's toCIP table sums to, and the caveat is
// built to name the column ONLY when this document's own residual meets the
// hand-typed page figure -- so this test also covers that gate being open.
func TestTransferCaveatNamesThePrintedColumn(t *testing.T) {
	g := buildGraph(t, spineFacts(t, testYear), testOptions())
	text := caveatText(g)

	for _, stale := range []string{"not yet mapped", "not mapped yet", "fisc-5gk.3"} {
		if strings.Contains(text, stale) {
			t.Errorf("a caveat still says %q; p76 has been mapped and published since ced45b4:\n%s",
				stale, text)
		}
	}
	for _, want := range []string{
		"Transfers Out to CIP", // the printed heading
		"$38,086,737",          // p0073.txt:58, and this document's own residual
		"PDF p73",              // the site labels citations "PDF p" + the PDF page index
		"fisc-9gh",             // the bead that would actually pair the legs
		"transfers-by-fund",    // where p76's facts are, which is why they are not here
	} {
		if !strings.Contains(text, want) {
			t.Errorf("no caveat says %q:\n%s", want, text)
		}
	}
}

// TestTransferCaveatDeclinesTheColumnItCannotVouchFor pins the gate rather than
// the happy path: transfersOutToCIP is hand-typed off a page no fixture carries,
// so a figure that has drifted from the graph beside it must lose the caveat its
// stronger sentence, not publish a mismatch.
func TestTransferCaveatDeclinesTheColumnItCannotVouchFor(t *testing.T) {
	col := Column{FiscalYear: testYear, Basis: testBasis}
	h := Headline{InternalTransferInCents: 100000, InternalTransferOutCents: 500000}

	if got := transferCaveat(h, col); strings.Contains(got, "Transfers Out to CIP") {
		t.Errorf("a $4,000.00 residual claimed the printed column:\n%s", got)
	}

	// And a basis pp.72-75 print no figure for gets no claim either, whatever
	// the arithmetic does: those pages carry one budget column per year.
	revised := Column{FiscalYear: testYear, Basis: mapping.BasisRevised}
	real := Headline{InternalTransferInCents: 2152599700, InternalTransferOutCents: 5961273400}
	if got := transferCaveat(real, revised); strings.Contains(got, "Transfers Out to CIP") {
		t.Errorf("a revised column claimed a schedule that prints only an adopted one:\n%s", got)
	}
	if got := transferCaveat(real, col); !strings.Contains(got, "Transfers Out to CIP") {
		t.Errorf("the adopted column did not name the printed column:\n%s", got)
	}

	// AND THE SIGN IS PART OF THE MATCH, not just the magnitude. The printed
	// column is transfers OUT to the CIP; a document whose transfers IN exceeded
	// its out by exactly that figure must not be handed an outflow column as the
	// explanation for an inflow surplus.
	inverted := Headline{
		InternalTransferInCents:  real.InternalTransferOutCents,
		InternalTransferOutCents: real.InternalTransferInCents,
	}
	if got := transferCaveat(inverted, col); strings.Contains(got, "Transfers Out to CIP") {
		t.Errorf("transfers in exceeding out by the tabled figure claimed an OUT column:\n%s", got)
	}
}

// TestEveryPrintedToCIPColumnIsCited pins the page number as well as the figure.
//
// WHY IT IS SEPARATE. transferCaveat gates the printed-column sentence on the
// Cents field meeting the document's own residual, so a wrong AMOUNT cannot
// ship. Nothing gates the Page field: pp.72-75 are not fixtures, so no test can
// read the number off the sheet, and transposing 73 and 75 would publish a
// citation pointing a reader at the other year's schedule with every check
// green. This asserts each published column names its own page, which is the
// most a tree without those fixtures can say.
func TestEveryPrintedToCIPColumnIsCited(t *testing.T) {
	want := map[Column]string{
		{FiscalYear: 2026, Basis: mapping.BasisAdopted}: "PDF p73",
		{FiscalYear: 2027, Basis: mapping.BasisAdopted}: "PDF p75",
	}
	if len(want) != len(transfersOutToCIP) {
		t.Fatalf("transfersOutToCIP has %d entries and this test knows %d; a new "+
			"printed column needs its page asserted here", len(transfersOutToCIP), len(want))
	}
	for col, page := range want {
		cip, ok := transfersOutToCIP[col]
		if !ok {
			t.Errorf("no printed to-CIP column declared for %s", col)
			continue
		}
		// Drive the real function rather than reading the field, so this fails
		// if the citation stops reaching the prose as well as if it changes.
		h := Headline{InternalTransferOutCents: int64(cip.Cents), InternalTransferInCents: 0}
		got := transferCaveat(h, col)
		if !strings.Contains(got, page) {
			t.Errorf("%s cites no %s:\n%s", col, page, got)
		}
	}
}

// hasCaveat reports whether any caveat mentions a word.
func hasCaveat(g *Graph, substr string) bool {
	return strings.Contains(caveatText(g), substr)
}

func caveatText(g *Graph) string {
	return strings.Join(g.Metadata.Caveats, "\n")
}

func sortedStrings(s []string) bool {
	for i := 1; i < len(s); i++ {
		if s[i-1] > s[i] {
			return false
		}
	}
	return true
}
