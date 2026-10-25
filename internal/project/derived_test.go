package project

import (
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/jcrussell/livermore-budget/internal/structure"
)

// TestTheSpineEndpointsAreTheGoldensOwn holds the endpoint table to the
// document it describes: every node of testdata/sankey.golden.json outside
// the revenue and expenditure hierarchy is in the table, and nothing else is.
func TestTheSpineEndpointsAreTheGoldensOwn(t *testing.T) {
	raw, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	var doc Document
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("decode golden: %v", err)
	}
	var drawn []string
	for _, n := range doc.Nodes {
		if strings.HasPrefix(n.ID, PrefixRevenue) || strings.HasPrefix(n.ID, PrefixExpenditure) ||
			strings.HasPrefix(n.ID, PrefixFundGroup) {
			continue
		}
		drawn = append(drawn, n.ID)
	}
	var declared []string
	for _, e := range flowEndpoints {
		if e.spine {
			declared = append(declared, e.id)
		}
	}
	slices.Sort(drawn)
	slices.Sort(declared)
	if diff := cmp.Diff(drawn, declared); diff != "" {
		t.Errorf("the golden's endpoints and the spine's flowEndpoints differ (-golden +declared):\n%s", diff)
	}
}

// TestContestedTotalsReadTheirFiguresOffTheException: the one declared entry
// carries the exception's column, group, both figures and bead, so the caveat
// and cuts-tie-along-the-lattice cannot name two different pairs.
func TestContestedTotalsReadTheirFiguresOffTheException(t *testing.T) {
	all, err := ContestedTotals()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 {
		t.Fatalf("got %d contested totals, want 1", len(all))
	}
	c := all[0]
	var e structure.Exception
	for _, x := range structure.BudgetBookExceptions() {
		if x.Name == c.Exception {
			e = x
		}
	}
	if e.Name == "" {
		t.Fatalf("contested total names exception %q, which is not declared", c.Exception)
	}
	p := e.Cells[0]
	want := contestedTotal{
		Exception: c.Exception, Column: Column{FiscalYear: p.Year, Basis: "adopted"},
		FundGroup: p.Coords[structure.AxisFundGroup],
		Published: p.Against.Cents, Elsewhere: p.Cut.Cents,
		SpinePages: c.SpinePages, Row: c.Row, PrintedBy: c.PrintedBy, ImpliedBy: c.ImpliedBy, Kept: c.Kept,
	}
	if diff := cmp.Diff(want, c, cmp.AllowUnexported(contestedTotal{})); diff != "" {
		t.Errorf("ContestedTotals (-exception +declared):\n%s", diff)
	}
	if c.Published-c.Elsewhere != e.Residual {
		t.Errorf("the caveat's difference is %d and the exception's residual %d", c.Published-c.Elsewhere, e.Residual)
	}
}

// TestAContestedTotalWithAnEmptyClauseIsAnError: an empty prose clause is
// refused where the entries are resolved.
// Mutation: drop the empty-clause loop from resolveContestedTotals and every
// case goes red with "got nil error".
func TestAContestedTotalWithAnEmptyClauseIsAnError(t *testing.T) {
	was := contestedTotals
	t.Cleanup(func() { contestedTotals = was })
	for name, blank := range map[string]func(*contestedTotal){
		"Kept":       func(c *contestedTotal) { c.Kept = "" },
		"PrintedBy":  func(c *contestedTotal) { c.PrintedBy = " " },
		"ImpliedBy":  func(c *contestedTotal) { c.ImpliedBy = "" },
		"Row":        func(c *contestedTotal) { c.Row = "" },
		"SpinePages": func(c *contestedTotal) { c.SpinePages = "" },
	} {
		t.Run(name, func(t *testing.T) {
			contestedTotals = slices.Clone(was)
			blank(&contestedTotals[0])
			_, err := resolveContestedTotals()
			if err == nil {
				t.Fatal("got nil error, want an empty clause refused")
			}
			if !strings.Contains(err.Error(), "declares no "+name) {
				t.Errorf("got %v, want it to name %s", err, name)
			}
		})
	}
}

// Of two blank clauses the refusal names the one declared first (Row before
// Kept), every run.
// Mutation: iterate the clauses as a map and this goes red with "want it to
// name Row" on some of its twenty runs.
func TestAContestedTotalWithTwoEmptyClausesNamesTheFirst(t *testing.T) {
	was := contestedTotals
	t.Cleanup(func() { contestedTotals = was })
	contestedTotals = slices.Clone(was)
	contestedTotals[0].Kept = ""
	contestedTotals[0].Row = ""
	for range 20 {
		_, err := resolveContestedTotals()
		if err == nil {
			t.Fatal("got nil error, want an empty clause refused")
		}
		if !strings.Contains(err.Error(), "declares no Row") {
			t.Fatalf("got %v, want it to name Row", err)
		}
	}
}

// TestAContestedTotalNamingNoExceptionIsAnError pins that a declaration naming
// an exception structure no longer carries refuses with a message, which
// verify records as a projection that did not build, rather than a panic.
func TestAContestedTotalNamingNoExceptionIsAnError(t *testing.T) {
	was := contestedTotals
	t.Cleanup(func() { contestedTotals = was })
	contestedTotals = slices.Clone(was)
	contestedTotals[0].Exception = "renamed-away"
	if _, err := resolveContestedTotals(); err == nil || !strings.Contains(err.Error(), `"renamed-away", which is not declared`) {
		t.Errorf("resolveContestedTotals = %v, want the undeclared exception named", err)
	}
}
