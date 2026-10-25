package check

import (
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/amount"
	"github.com/jcrussell/livermore-budget/internal/structure"
)

// This file reads pp.85-125 with a scanner of its own, independent of the
// machinery that publishes them, and compares the printed figures to pp.66-67.

// departmentwidePages are the eleven pages carrying an Expenditures by Category
// block; p102, p120 and p125 carry funding-source rows only.
var departmentwidePages = []int{85, 89, 93, 97, 101, 105, 108, 111, 115, 119, 124}

// objectRow matches a printed object line: an optional division label, an
// object category, and four figures; a bare "-" is a printed zero. The label is
// split by \s+ because p101:11 prints "Community Development Wages & Benefits"
// with one space; the figures are split by \s{2,}.
var objectRow = regexp.MustCompile(
	`^(.*?)\s+(Wages & Benefits|Services & Supplies|Capital Outlay|Debt Services|` +
		`Transfers Out|Division Total)\s{2,}(.*)$`)

func departmentwideCents(t *testing.T, tok string) amount.Cents {
	t.Helper()
	if tok == "-" {
		return 0
	}
	c, err := amount.Parse(tok, amount.Dollars)
	if err != nil {
		t.Fatalf("amount.Parse(%q): %v", tok, err)
	}
	return c
}

// readObjectFigures pulls the four column figures off one printed line.
func readObjectFigures(t *testing.T, tail string) []amount.Cents {
	t.Helper()
	var out []amount.Cents
	for _, f := range strings.Fields(tail) {
		out = append(out, departmentwideCents(t, f))
	}
	return out
}

// TestThePrintedDepartmentwideRowsSumToTheSpineByObjectCategory holds every
// division's object rows to its printed Division Total, and the pages' rows per
// category to pp.66-67's citywide figures; FY2026-27 services and supplies come
// to 250,000 less than the spine (fisc-av0w), asserted as a difference.
func TestThePrintedDepartmentwideRowsSumToTheSpineByObjectCategory(t *testing.T) {
	t.Parallel()
	s := committed(t)
	doc, ok := s.Docs[budgetDoc]
	if !ok {
		t.Fatalf("no extraction for %s", budgetDoc)
	}

	// byCategory is the FY2025-26 and FY2026-27 columns per object category;
	// divisions counts the printed Division Total rows.
	byCat := map[string][2]amount.Cents{}
	divisions, transfers, rounded := 0, 0, 0
	for _, page := range departmentwidePages {
		text, err := doc.Page(page)
		if err != nil {
			t.Fatalf("page %d: %v", page, err)
		}
		block, _, cut := strings.Cut(text, "Total Department Expenditures")
		if !cut {
			t.Fatalf("p%d prints no Total Department Expenditures row", page)
		}
		running := [4]amount.Cents{}
		for _, line := range strings.Split(block, "\n") {
			m := objectRow.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			v := readObjectFigures(t, m[3])
			if len(v) != 4 {
				t.Fatalf("p%d: %q is followed by %d figures, want 4", page, m[2], len(v))
			}
			if m[2] == "Division Total" {
				divisions++
				// Five divisions miss their printed total by exactly $1 in the
				// FY2023-24 Actual column, declared as stated_total_deltas; the
				// tolerated cells are counted against those five, not waived.
				for c := 0; c < 4; c++ {
					d := v[c] - running[c]
					if d == 0 {
						continue
					}
					if c == 0 && d == amount.Cents(100) {
						rounded++
						continue
					}
					t.Errorf("p%d Division Total column %d: rows sum to %s, the page "+
						"prints %s, a difference of %s", page, c, running[c], v[c], d)
				}
				running = [4]amount.Cents{}
				continue
			}
			if m[2] == "Transfers Out" {
				// The one row on these pages that is not an expenditure. It is
				// inside the printed Division Total, so it stays in `running`,
				// and it is OUT of the object-category sums below.
				transfers++
				for c := 0; c < 4; c++ {
					running[c] += v[c]
				}
				continue
			}
			for c := 0; c < 4; c++ {
				running[c] += v[c]
			}
			acc := byCat[m[2]]
			acc[0] += v[2]
			acc[1] += v[3]
			byCat[m[2]] = acc
		}
	}

	if divisions != 29 || transfers != 1 {
		t.Fatalf("read %d division blocks and %d Transfers Out rows, want 29 and 1; the "+
			"schedule's shape changed and every figure below is about different rows",
			divisions, transfers)
	}
	// EXACTLY the five the rule file declares, and no more. A sixth is either a
	// mis-read column or a rounding the rules have not declared, and both must
	// be seen rather than absorbed.
	if rounded != 5 {
		t.Errorf("%d division columns miss their printed total by $1, want 5; the rule file "+
			"declares five stated_total_deltas and a sixth would be tolerated here while "+
			"failing the build", rounded)
	}

	// Typed off pp.66-67, summed over the six fund groups. The FY2026-27
	// services-and-supplies entry is the spine's own figure; the pages come to
	// 250,000 less and that is asserted as such below.
	want := map[string][2]amount.Cents{
		"Wages & Benefits":    {amount.Cents(10774112300), amount.Cents(11029702000)},
		"Services & Supplies": {amount.Cents(13352230900), amount.Cents(13050208700)},
		"Capital Outlay":      {amount.Cents(384692400), amount.Cents(311683500)},
		"Debt Services":       {amount.Cents(898505600), amount.Cents(893895400)},
	}
	for cat, w := range want {
		got := byCat[cat]
		if got[0] != w[0] {
			t.Errorf("%s FY2025-26: the pages sum to %s, the spine publishes %s",
				cat, got[0], w[0])
		}
		short := w[1] - got[1]
		if cat == "Services & Supplies" {
			// fisc-av0w, stated as a difference so this test carries the
			// discrepancy rather than inheriting it from the check.
			if short != amount.Cents(25000000) {
				t.Errorf("%s FY2026-27: the pages are %s under the spine, want exactly "+
					"$250,000 (fisc-av0w)", cat, short)
			}
			continue
		}
		if short != 0 {
			t.Errorf("%s FY2026-27: the pages sum to %s, the spine publishes %s",
				cat, got[1], w[1])
		}
	}
	if len(byCat) != len(want) {
		t.Errorf("the pages print %d object categories, want %d", len(byCat), len(want))
	}
}

// NEITHER FIGURE IN THE BY-OBJECT EXCEPTION IS PRINTED: this corpus prints no
// citywide object-category total. Both halves are asserted: the figures are
// absent from the extracted pages, and the entry is grounded (SameResidualAs)
// in an exception whose figures ARE printed and hold the same residual apart.
func TestDepartmentwideExceptionFiguresAreNotPrintedAndTheirDifferenceIs(t *testing.T) {
	t.Parallel()
	s := committed(t)
	docIDs := make([]string, 0, len(s.Docs))
	for id := range s.Docs {
		docIDs = append(docIDs, id)
	}
	sort.Strings(docIDs)
	if len(docIDs) < 2 {
		t.Fatalf("the corpus holds %d extracted document(s); a claim about what THIS CORPUS "+
			"prints cannot be made from one", len(docIDs))
	}

	byName := map[string]structure.Exception{}
	var byObject []structure.Exception
	for _, e := range structure.BudgetBookExceptions() {
		byName[e.Name] = e
		if e.Cut == structure.CutDepartmentwide && e.Against == structure.CutSpine {
			byObject = append(byObject, e)
		}
	}
	if len(byObject) == 0 {
		t.Fatal("no exception is declared on the departmentwide cut, so this test asserts nothing")
	}

	// The scan counts the pages it read so the absence assertion cannot pass
	// over an unreadable corpus, and reads every document the subject loads
	// (the budget book and the ACFR; the CIP is not loaded, so is not covered).
	scanned, figures := 0, 0
	for _, e := range byObject {
		for _, p := range e.Cells {
			for _, c := range []amount.Cents{amount.Cents(p.Against.Cents), amount.Cents(p.Cut.Cents)} {
				figures++
				// %s renders cents as a grouped dollar figure, which is how the
				// pages print money.
				needle := strings.TrimSuffix(strings.TrimPrefix(c.String(), "$"), ".00")
				for _, id := range docIDs {
					d := s.Docs[id]
					for page := 1; page <= d.PageCount(); page++ {
						text, err := d.Page(page)
						if err != nil {
							t.Fatalf("page %d of %s: %v", page, id, err)
						}
						scanned++
						if strings.Contains(text, needle) {
							t.Errorf("exception %s carries %s and %s p%04d PRINTS it; if a page "+
								"really does publish this figure the entry should assert it "+
								"directly, and the comment saying no page does is now wrong",
								e.Name, c, id, page)
							break
						}
					}
				}
			}
		}

		g, ok := byName[e.SameResidualAs]
		if !ok {
			t.Errorf("exception %s is grounded in %q, which is not declared; neither of its "+
				"figures is printed, so that grounding is the whole of it", e.Name, e.SameResidualAs)
			continue
		}
		if g.Residual != e.Residual {
			t.Errorf("exception %s holds %s apart and %s holds %s; they are the same "+
				"discrepancy on two axes and must agree",
				e.Name, amount.Cents(e.Residual), g.Name, amount.Cents(g.Residual))
		}
		// And the ground's own figures ARE printed, which is what makes it a
		// ground: TestP0067IsTheOutlierAndFivePagesDisagree reads them off the
		// pages, and the cut it names is the by-fund-group one.
		if g.Cut != "funding-sources" || g.Against != structure.CutSpine {
			t.Errorf("exception %s is grounded in %s, which compares %s against %s; the "+
				"printed figures are on the funding-sources side", e.Name, g.Name, g.Cut, g.Against)
		}
	}

	// EXACT RATHER THAN A FLOOR, and computed from the documents rather than
	// typed: two figures per pinned cell over every page of every document
	// loaded. A floor would let a scan that stopped early pass.
	want := 0
	for _, id := range docIDs {
		want += s.Docs[id].PageCount()
	}
	want *= figures
	if scanned != want {
		t.Errorf("the absence scan read %d pages, want %d -- %d figures over every page of %d "+
			"document(s); a scan that stopped early proves less than the string it "+
			"substantiates claims", scanned, want, figures, len(docIDs))
	}
}
