package project

import (
	"testing"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/mapping"
	"github.com/jcrussell/livermore-budget/internal/project/projecttest"
	"github.com/jcrussell/livermore-budget/internal/vocab"
)

// The fixture is the citywide spine, pp.66-67, as facts (projecttest), and the
// cell builders a test states the rest of its input with.
const (
	testDoc   = projecttest.Doc
	testScope = projecttest.Scope
	testYear  = projecttest.Year
	testBasis = projecttest.Basis
)

// testColumn is the one column every single-column fixture prints.
var testColumn = Column{FiscalYear: testYear, Basis: testBasis}

// testOptions are the options the spine document is built under.
func testOptions() Options {
	return Options{
		Columns: []Column{{FiscalYear: testYear, Basis: testBasis}},
		Scopes:  []string{testScope},
		Version: projecttest.Version,
	}
}

// spineFacts renders the fixture as facts for one fiscal year.
func spineFacts(t *testing.T, year int) []fact.Fact {
	t.Helper()
	return projecttest.SpineFacts(year)
}

// cellSpec is one cell of a schedule, with everything that does not matter to
// this package defaulted. Zero year means testYear and zero sign means
// positive, so a test states only what it is about.
type cellSpec struct {
	kind     vocab.Kind
	rule     string
	category string
	label    string
	group    string
	cents    int64
	year     int
	sign     vocab.Sign
	page     int
}

func (c cellSpec) fact(t *testing.T) fact.Fact {
	t.Helper()
	if c.year == 0 {
		c.year = testYear
	}
	if c.sign == "" {
		c.sign = vocab.SignPositive
	}
	if c.rule == "" {
		c.rule = ruleFor(t, c.kind)
	}
	if c.label == "" {
		c.label = c.category
	}
	if c.page == 0 {
		c.page = projecttest.SpinePage(c.group)
	}
	f := printedFact(testDoc, c.rule, mapping.Row{Category: c.category}, c.label,
		mapping.Column{FundGroup: c.group}, testScope, c.kind, Column{FiscalYear: c.year, Basis: testBasis},
		c.cents)
	f.Page, f.Sign = c.page, c.sign
	return f
}

// printedFact is projecttest.PrintedFact at column c.
func printedFact(doc, rule string, row mapping.Row, label string, col mapping.Column, scope string,
	kind vocab.Kind, c Column, cents int64) fact.Fact {
	return projecttest.PrintedFact(doc, rule, row, label, col, scope, kind, c.FiscalYear, c.Basis, cents)
}

// ruleFor is the rule id the spine uses for each kind. It is part of the fact
// id, so the golden file's ids only reproduce with these exact strings.
func ruleFor(t *testing.T, k vocab.Kind) string {
	t.Helper()
	switch k {
	case vocab.KindRevenue:
		return "spine-revenues"
	case vocab.KindExpenditure:
		return "spine-expenditures"
	case vocab.KindTransferIn:
		return "spine-transfers-in"
	case vocab.KindTransferOut:
		return "spine-transfers-out"
	case vocab.KindFundBalance:
		return "spine-fund-balance"
	default:
		t.Fatalf("no fixture rule for kind %q", k)
		return ""
	}
}

// facts is shorthand for a handful of cells.
func facts(t *testing.T, specs ...cellSpec) []fact.Fact {
	t.Helper()
	out := make([]fact.Fact, 0, len(specs))
	for _, s := range specs {
		out = append(out, s.fact(t))
	}
	return out
}

// stubLabels stands in for internal/registry, keyed by data/taxonomy.yaml
// category slug; fund names live in a second map.
type stubLabels = projecttest.Labels

// stubFunds is stubLabels with fund names attached, for the documents that key
// on funds rather than on categories.
type stubFunds struct {
	stubLabels
	funds map[int]string
}

func (s stubFunds) FundName(number int) (string, bool) {
	n, ok := s.funds[number]
	return n, ok
}

// goldenLabels is the spine's labels, transcribed.
var goldenLabels = projecttest.SpineLabels

// buildGraph builds with the golden labels and fails the test on error.
func buildGraph(t *testing.T, fs []fact.Fact, o Options) *Document {
	t.Helper()
	g, err := (&sankey{Labels: goldenLabels}).Document(fs, o)
	if err != nil {
		t.Fatalf("Graph: %v", err)
	}
	return g
}

// linkBetween returns the one link from source to target, or fails.
func linkBetween(t *testing.T, g *Document, source, target string) Link {
	t.Helper()
	for _, l := range g.Links {
		if l.Source == source && l.Target == target {
			return l
		}
	}
	t.Fatalf("no link %s -> %s in %d links", source, target, len(g.Links))
	return Link{}
}

// hasLink reports whether any link joins the two nodes.
func hasLink(g *Document, source, target string) bool {
	for _, l := range g.Links {
		if l.Source == source && l.Target == target {
			return true
		}
	}
	return false
}

// nodeByID returns the node with an id, or fails.
func nodeByID(t *testing.T, g *Document, id string) Node {
	t.Helper()
	for _, n := range g.Nodes {
		if n.ID == id {
			return n
		}
	}
	t.Fatalf("no node %q in %d nodes", id, len(g.Nodes))
	return Node{}
}
