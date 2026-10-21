package project

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/amount"
	"github.com/jcrussell/livermore-budget/internal/structure"
	"github.com/jcrussell/livermore-budget/internal/vocab"
)

// cipProjectsOf builds the committed store's cip-projects document.
func cipProjectsOf(t *testing.T) *TrendsDocument {
	t.Helper()
	store, err := committedStore()
	if err != nil {
		t.Fatal(err)
	}
	p := &CIPProjects{Labels: store.reg}
	decl := p.Slices(store.facts, "cip-projects test")
	if len(decl) != 1 {
		t.Fatalf("cip-projects declares %d documents over the committed store, want one", len(decl))
	}
	if !slices.Equal(decl[0].Columns, CIPProjectsColumns()) {
		t.Fatalf("cip-projects is built over %v, want %v", decl[0].Columns, CIPProjectsColumns())
	}
	doc, err := p.Document(store.facts, decl[0])
	if err != nil {
		t.Fatalf("Document: %v", err)
	}
	if _, err := p.Build(store.facts, decl[0]); err != nil {
		t.Fatalf("Build: %v", err)
	}
	return doc
}

// TestCIPProjectsTieToTheListingsPrintedTotals holds the document's rows to
// what pp.224-235 print, through cipListingColumns, the one declaration the
// caveats are written from: in every column the rows sum to the printed
// grand total less fund 611's two blocks, plus what the total prints under
// its rows.
//
// Mutation: change FederalStorm for FY2025-26 to 2_246_251 and this test goes
// red, here and in the page-text arm below, while the caveat reads the
// altered figure.
func TestCIPProjectsTieToTheListingsPrintedTotals(t *testing.T) {
	doc := cipProjectsOf(t)
	sums := map[Column]int64{}
	for _, s := range doc.Series {
		for _, p := range s.Points {
			sums[Column{FiscalYear: p.FiscalYear, Basis: vocab.Basis(p.Basis)}] += p.AmountCents
		}
	}
	cols := cipListingColumns()
	if len(cols) != len(CIPProjectsColumns()) {
		t.Fatalf("cipListingColumns declares %d columns and the document publishes %d",
			len(cols), len(CIPProjectsColumns()))
	}
	for i, c := range cols {
		if c.Column != CIPProjectsColumns()[i] {
			t.Errorf("cipListingColumns[%d] is %s, want %s", i, c.Column, CIPProjectsColumns()[i])
		}
		if got, want := sums[c.Column], c.RowsPublished()*100; got != want {
			t.Errorf("%s: the rows sum to %s; p235's total %s less fund 611's %s and %s, plus %s, is %s",
				c.Column, amount.Cents(got).Dollars(), dollars(c.GrandTotal), dollars(c.FederalStorm),
				dollars(c.StateStorm), dollars(c.TotalUnder), amount.Cents(want).Dollars())
		}
	}
}

// TestCIPListingColumnsArePrinted holds each figure cipListingColumns
// declares to the page text it is read from: the grand totals on p235's last
// figure line, fund 611's blocks on p231's lines below 4,070,000's fund
// total, and p222's Totals line. p222's are also the sum of the facts p222
// publishes, its transfers in, grants and balance draw.
//
// Mutation: change GrandTotal for FY2027-28 to 69_034_501 and this test goes
// red, as does TestCIPProjectsTieToTheListingsPrintedTotals.
func TestCIPListingColumnsArePrinted(t *testing.T) {
	page := func(n int) string {
		b, err := os.ReadFile(filepath.Join("..", "..", "data", "extracted",
			"livermore-budget-fy2026-2027", "pages", fmt.Sprintf("p%04d.txt", n)))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	figures := regexp.MustCompile(`[\d,]+|-`)
	lineOf := func(text string, line int) []string {
		lines := strings.Split(text, "\n")
		if line >= len(lines) {
			t.Fatalf("no line %d", line+1)
		}
		return figures.FindAllString(lines[line], -1)
	}
	printed := func(d int64) string {
		if d == 0 {
			return "-"
		}
		return strings.TrimPrefix(dollars(d), "$")
	}
	cols := cipListingColumns()
	// p235's last line of figures is the grand TOTAL; p231 lines 48 and 53
	// are the federal and state storm blocks' totals; each opens on FY2025-26.
	grand := lastFigures(t, page(235), figures)
	federal := lineOf(page(231), 47)
	state := lineOf(page(231), 52)
	for i, c := range cols {
		for _, w := range []struct {
			what string
			line []string
			want int64
		}{
			{"p235 grand total", grand, c.GrandTotal},
			{"p231 federal storm", federal, c.FederalStorm},
			{"p231 state storm", state, c.StateStorm},
		} {
			if i >= len(w.line) || w.line[i] != printed(w.want) {
				t.Errorf("%s: %s prints %v, and cipListingColumns declares %s",
					c.Column, w.what, w.line, printed(w.want))
			}
		}
	}

	store, err := committedStore()
	if err != nil {
		t.Fatal(err)
	}
	p222 := page(222)
	for _, c := range cols {
		if c.NewAppropriations == 0 {
			continue
		}
		if !strings.Contains(p222, printed(c.NewAppropriations)) {
			t.Errorf("%s: p222 does not print %s", c.Column, printed(c.NewAppropriations))
		}
		var sum int64
		for _, f := range store.facts {
			if f.Scope == structure.ScopeCIPFundingSources && f.Kind != vocab.KindTransferOut &&
				f.FiscalYear == c.Column.FiscalYear && f.Basis == c.Column.Basis {
				sum += f.AmountCents
			}
		}
		if sum != c.NewAppropriations*100 {
			t.Errorf("%s: p222's facts sum to %s, and cipListingColumns declares %s",
				c.Column, amount.Cents(sum).Dollars(), dollars(c.NewAppropriations))
		}
	}
}

// lastFigures is the figures on a page's last line that prints any.
func lastFigures(t *testing.T, text string, figures *regexp.Regexp) []string {
	t.Helper()
	lines := strings.Split(text, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.Contains(lines[i], "BUDGET") {
			continue
		}
		if got := figures.FindAllString(lines[i], -1); len(got) > 0 {
			return got
		}
	}
	t.Fatal("the page prints no figure")
	return nil
}

// TestCIPProjectsCaveatsStateTheListingsFigures holds the caveats to
// cipListingColumns: each printed figure appears in the text, so a caveat
// cannot drift from the declaration the tie test reads.
//
// Mutation: drop the fund-611 caveat's clause naming the state block's
// figure and this test goes red for FY2025-26 to FY2027-28.
func TestCIPProjectsCaveatsStateTheListingsFigures(t *testing.T) {
	var text string
	for _, c := range cipProjectsCaveats() {
		text += c.Summary + " " + c.Text + "\n"
	}
	for _, c := range cipListingColumns() {
		for _, d := range []int64{c.GrandTotal, c.RowsPublished(), c.FederalStorm, c.StateStorm, c.NewAppropriations} {
			if d != 0 && !strings.Contains(text, dollars(d)) {
				t.Errorf("%s: no caveat states %s", c.Column, dollars(d))
			}
		}
	}
	for _, want := range []string{"carried forward", "projected", "neither block is published", "FY 2024-25", "FY 2030-45", "TOTAL"} {
		if !strings.Contains(text, want) {
			t.Errorf("no caveat says %q", want)
		}
	}
}
