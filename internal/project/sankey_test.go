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

	gotBytes, err := (&sankey{Labels: goldenLabels}).Build(spineFacts(t, testYear), testOptions())
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
	s := &sankey{Labels: goldenLabels}

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
	out, err := (&sankey{}).Build(nil, testOptions())
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
	graph, err := (&sankey{}).Graph(spine(t), testOptions())
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
	withLabels := &sankey{Labels: stubLabels{
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
	var _ labels = (*registry.Registry)(nil)
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

			_, err := (&sankey{}).Graph(fs, testOptions())
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
	_, err := (&sankey{}).Graph(spineFacts(t, testYear), Options{})
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

	_, err := (&sankey{}).Graph(spineFacts(t, testYear), o)
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
// THE STALE REASON, AND IT HAS NOW GONE STALE TWICE IN THE SAME PLACE. The
// caveat first said the legs were unpaired "because the p76 transfer schedule
// is not yet mapped (fisc-5gk.3)"; the page was mapped and the legs stayed
// unpaired, because its facts are at scope transfers-by-fund and no projection
// selected it. It then said pairing them "needs a document of p76's own and a
// transfer_id derived from the two legs' shared page and offset (fisc-9gh)";
// that document exists, and this chart's Transfers In opens into it. Both
// sentences were true when written and neither was revisited, which is the
// failure mode this test exists to make loud -- so the stale list below names a
// promise of work rather than one wording, and grows by one entry each time.
//
// WHAT IS STILL TRUE IS SCOPED TO THIS DOCUMENT. The spine's own links carry no
// transfer_id and cannot: internal/project nets p76's rows into fund-group
// cells before a pairing could attach to anything. The caveat has to say that
// about THIS graph without telling a reader the pairing does not exist.
//
// THE PRINTED COLUMN. The residual is not a discrepancy: the city prints it
// under a heading of its own. The figure asserted here is the one
// internal/check/transfersdetail.go's toCIP table sums to, and the caveat is
// built to name the column ONLY when this document's own residual meets the
// hand-typed page figure -- so this test also covers that gate being open.
func TestTransferCaveatNamesThePrintedColumn(t *testing.T) {
	g := buildGraph(t, spineFacts(t, testYear), testOptions())
	text := caveatText(g)

	for _, stale := range []string{
		"not yet mapped", "not mapped yet", "fisc-5gk.3",
		// The pairing is done. A caveat naming the bead for it, or saying what
		// it would take, tells a reader the chart cannot do what it does.
		"fisc-9gh", "Pairing the legs needs",
	} {
		if strings.Contains(text, stale) {
			t.Errorf("a caveat still promises work that has landed (%q); p76 is mapped, "+
				"published AND drawn with its legs paired:\n%s", stale, text)
		}
	}
	for _, want := range []string{
		"Transfers Out to CIP", // the printed heading
		"$38,086,737",          // p0073.txt:58, and this document's own residual
		"PDF p73",              // the site labels citations "PDF p" + the PDF page index
		// The document that DOES pair them, which is what a reader who has just
		// been told this graph does not needs pointed at. It is also where p76's
		// facts are, which is why they are not in this graph.
		"transfers-by-fund",
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

	if got := transferCaveat(h, col, nil).Text; strings.Contains(got, "Transfers Out to CIP") {
		t.Errorf("a $4,000.00 residual claimed the printed column:\n%s", got)
	}

	// And a basis pp.72-75 print no figure for gets no claim either, whatever
	// the arithmetic does: those pages carry one budget column per year.
	revised := Column{FiscalYear: testYear, Basis: mapping.BasisRevised}
	real := Headline{InternalTransferInCents: 2152599700, InternalTransferOutCents: 5961273400}
	if got := transferCaveat(real, revised, nil).Text; strings.Contains(got, "Transfers Out to CIP") {
		t.Errorf("a revised column claimed a schedule that prints only an adopted one:\n%s", got)
	}
	if got := transferCaveat(real, col, nil).Text; !strings.Contains(got, "Transfers Out to CIP") {
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
	if got := transferCaveat(inverted, col, nil).Text; strings.Contains(got, "Transfers Out to CIP") {
		t.Errorf("transfers in exceeding out by the tabled figure claimed an OUT column:\n%s", got)
	}
}

// TestTheTransferCaveatMarksTheLegsTheGraphDRAWS is the arm that stops
// AppliesTo being a guess.
//
// TWO WRONG VERSIONS PRECEDED THE ONE THIS PINS. Naming both legs
// unconditionally pointed at a node a transfers-in-only document does not
// carry; gating each on its own headline total fixed that and was still a
// PROXY, because a non-zero total says a transfer_out fact was summed, not that
// the node it produced is spelled "transfers/out". This asserts the endpoints
// come off the LINKS, which is the thing itself.
//
// The third case is the one no headline can answer: same totals, different
// node id. It is not a shape the corpus produces today -- which is exactly why
// it needs a test rather than a reader's confidence.
func TestTheTransferCaveatMarksTheLegsTheGraphDRAWS(t *testing.T) {
	col := Column{FiscalYear: testYear, Basis: testBasis}
	h := Headline{InternalTransferInCents: 100, InternalTransferOutCents: 500}

	both := []Link{
		{Source: nodeTransfersIn, Target: prefixFundGroup + "general",
			ValueCents: 100, Kind: KindInternalTransfer},
		{Source: prefixFundGroup + "general", Target: nodeTransfersOut,
			ValueCents: 500, Kind: KindInternalTransfer},
	}
	if diff := cmp.Diff([]string{nodeTransfersIn, nodeTransfersOut},
		transferCaveat(h, col, both).AppliesTo); diff != "" {
		t.Errorf("both legs drawn (-want +got):\n%s", diff)
	}

	// One leg drawn, the other only in the totals. The caveat still fires --
	// caveats() gates on either side being non-zero -- and must mark only what
	// is there, or ValidateCaveats aborts the build.
	inOnly := both[:1]
	if diff := cmp.Diff([]string{nodeTransfersIn},
		transferCaveat(h, col, inOnly).AppliesTo); diff != "" {
		t.Errorf("only the in leg drawn (-want +got):\n%s", diff)
	}

	// A transfer endpoint the corpus does not use today. The headline is
	// identical to the first case and the answer must not be.
	odd := []Link{
		{Source: prefixFundGroup + "general", Target: prefixTransfers + "out-to-cip",
			ValueCents: 500, Kind: KindInternalTransfer},
	}
	if diff := cmp.Diff([]string{prefixTransfers + "out-to-cip"},
		transferCaveat(h, col, odd).AppliesTo); diff != "" {
		t.Errorf("an endpoint named otherwise (-want +got):\n%s", diff)
	}

	// A link that is not a transfer contributes nothing, so a fund-balance
	// endpoint at tier 0 is not mistaken for a leg.
	none := []Link{
		{Source: "fund-balance/draw", Target: prefixFundGroup + "general",
			ValueCents: 100, Kind: KindFundBalance},
	}
	if got := transferCaveat(h, col, none).AppliesTo; len(got) != 0 {
		t.Errorf("a graph with no transfer link marked %v", got)
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
		got := transferCaveat(h, col, nil).Text
		if !strings.Contains(got, page) {
			t.Errorf("%s cites no %s:\n%s", col, page, got)
		}
	}
}

// hasCaveat reports whether any caveat mentions a word.
func hasCaveat(g *Graph, substr string) bool {
	return strings.Contains(caveatText(g), substr)
}

// caveatText joins the caveats' TEXT, which is the field that used to be the
// whole caveat. Summaries are deliberately not searched: a substring assertion
// that matched either would pass on a document whose text had been emptied.
func caveatText(g *Graph) string {
	texts := make([]string, 0, len(g.Metadata.Caveats))
	for _, c := range g.Metadata.Caveats {
		texts = append(texts, c.Text)
	}
	return strings.Join(texts, "\n")
}

func sortedStrings(s []string) bool {
	for i := 1; i < len(s); i++ {
		if s[i-1] > s[i] {
			return false
		}
	}
	return true
}

// The contested-total caveat (fisc-av0w).
//
// WHAT IT IS FOR. Budget Book p0067 prints an Internal Service Funds Services &
// Supplies figure $250,000 higher than six other schedules of the same document,
// so the FY2027 spine draws a total the city's own book contradicts. The chart
// publishes p0067's figure, because every figure here is one the city printed on
// the page it is cited from and substituting one from elsewhere would make this
// an exception to that. What the reader is owed is the disclosure.
//
// EVERY TEST HERE MUTATES THE CONDITION rather than asserting the happy path.
// The caveat is emitted only when the graph actually draws the declared figure,
// which is what makes it retire itself; a test that only checked it appears
// would pass against a version that always appended it.

// contestedFY2027 is the one declared entry, fetched rather than respelled so a
// change to the declaration reaches these tests.
func contestedFY2027(t *testing.T) contestedTotal {
	t.Helper()
	all := ContestedTotals()
	if len(all) != 1 {
		t.Fatalf("got %d contested totals, want 1; this file is written about the "+
			"single fisc-av0w entry and a second one needs its own tests", len(all))
	}
	return all[0]
}

func TestContestedCaveatIsEmittedOnlyForTheColumnThatDrawsIt(t *testing.T) {
	c := contestedFY2027(t)
	links := []Link{
		{Source: prefixFundGroup + c.FundGroup, Target: prefixExpenditure + "services-and-supplies",
			ValueCents: c.Published},
	}

	got, ok := contestedCaveat(c, c.Column, links)
	if !ok {
		t.Fatal("no caveat for the column and figure the entry declares")
	}
	// The sentence must carry BOTH figures and the bead. A caveat naming only
	// the drawn figure tells a reader nothing they could act on.
	for _, want := range []string{dollars(c.Published), dollars(c.Elsewhere), c.Bead,
		dollars(c.Published - c.Elsewhere)} {
		if !strings.Contains(got.Text, want) {
			t.Errorf("caveat does not mention %q:\n%s", want, got.Text)
		}
	}
	// THE ID CARRIES THE FUND GROUP, because contestedTotals is a list and two
	// entries in one column would otherwise share an anchor -- which
	// ValidateCaveats refuses, so the failure would arrive as a build error a
	// long way from its cause.
	if want := "contested-total-" + c.FundGroup; got.ID != want {
		t.Errorf("caveat id is %q, want %q", got.ID, want)
	}
	// The summary is what a reader sees in a list of its peers, so it has to
	// name the group and the size of the disagreement on its own.
	for _, want := range []string{dollars(c.Published - c.Elsewhere)} {
		if !strings.Contains(got.Summary, want) {
			t.Errorf("summary does not mention %q:\n%s", want, got.Summary)
		}
	}
	// And it marks the group it is about, so a chart can flag that node.
	if len(got.AppliesTo) != 1 || got.AppliesTo[0] != prefixFundGroup+c.FundGroup {
		t.Errorf("caveat applies to %v, want just the %s group", got.AppliesTo, c.FundGroup)
	}

	// A different column draws a different year's figures and is not this
	// entry's problem.
	other := Column{FiscalYear: c.Column.FiscalYear - 1, Basis: c.Column.Basis}
	if cav, ok := contestedCaveat(c, other, links); ok {
		t.Errorf("caveat emitted for %s, which the entry does not name:\n%s", other, cav.Text)
	}
	// So is a different basis on the same year.
	if cav, ok := contestedCaveat(c, Column{FiscalYear: c.Column.FiscalYear, Basis: mapping.BasisActual}, links); ok {
		t.Errorf("caveat emitted for a basis the entry does not name:\n%s", cav.Text)
	}
}

// TestContestedCaveatRetiresItselfWhenTheFigureIsCorrected is the property that
// makes this a declaration rather than a note.
//
// The day the corpus stops publishing p0067's figure -- whichever way fisc-av0w
// is decided -- the graph stops drawing Published, the condition stops matching,
// and the sentence stops being printed. Nobody has to remember to delete it.
func TestContestedCaveatRetiresItselfWhenTheFigureIsCorrected(t *testing.T) {
	c := contestedFY2027(t)
	corrected := []Link{
		{Source: prefixFundGroup + c.FundGroup, Target: prefixExpenditure + "services-and-supplies",
			ValueCents: c.Elsewhere},
	}
	if cav, ok := contestedCaveat(c, c.Column, corrected); ok {
		t.Errorf("the caveat survived the figure being corrected to %s:\n%s",
			dollars(c.Elsewhere), cav.Text)
	}
	// And a third value -- neither the spine's nor the other schedules' -- also
	// silences it. That is correct and is why the corpus-level assertion in
	// internal/check exists: silence here must not be the only signal, or an
	// entry could go dead unnoticed.
	third := []Link{
		{Source: prefixFundGroup + c.FundGroup, Target: prefixExpenditure + "services-and-supplies",
			ValueCents: c.Published + 1},
	}
	if cav, ok := contestedCaveat(c, c.Column, third); ok {
		t.Errorf("the caveat survived a figure that is neither declared value:\n%s", cav.Text)
	}
}

// TestGroupExpenditureSumsOnlyThatGroupsObjectLinks is what the condition above
// rests on, and it is the arm a wrong sum would break silently.
func TestGroupExpenditureSumsOnlyThatGroupsObjectLinks(t *testing.T) {
	links := []Link{
		{Source: prefixFundGroup + "internal-service", Target: prefixExpenditure + "wages-and-benefits", ValueCents: 100},
		{Source: prefixFundGroup + "internal-service", Target: prefixExpenditure + "services-and-supplies", ValueCents: 20},
		// Another group's expenditure.
		{Source: prefixFundGroup + "general", Target: prefixExpenditure + "wages-and-benefits", ValueCents: 7},
		// The same group's non-expenditure flows.
		{Source: prefixFundGroup + "internal-service", Target: "transfers/out", ValueCents: 5},
		// And a link INTO the group, which is revenue rather than spending.
		{Source: prefixRevenue + "intergovernmental", Target: prefixFundGroup + "internal-service", ValueCents: 900},
	}
	if got, want := GroupExpenditure(links, "internal-service"), int64(120); got != want {
		t.Errorf("GroupExpenditure = %d, want %d", got, want)
	}
	if got, want := GroupExpenditure(links, "permanent"), int64(0); got != want {
		t.Errorf("GroupExpenditure for a group with no links = %d, want %d", got, want)
	}
}

// TestTheHeadlineIsANamedCut is fisc-w11l's acceptance arithmetic on the new
// path: the four published figures, produced as a sum over the view the
// document names rather than accumulated as its cells are drawn, and unchanged.
func TestTheHeadlineIsANamedCut(t *testing.T) {
	g := buildGraph(t, spineFacts(t, testYear), testOptions())
	want := Headline{
		AllFundsGrossRevenueCents:     29_996_900_700,
		AllFundsGrossExpenditureCents: 25_409_541_200,
		InternalTransferInCents:       2_152_599_700,
		InternalTransferOutCents:      5_961_273_400,
	}
	got := g.Metadata.Headline
	got.ExternalRevenueCents, got.ExternalExpenditureCents = 0, 0
	got.NaiveExpenditureCents, got.TransferResidualCents = 0, 0
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("the four published figures (-want +got):\n%s", diff)
	}
}
