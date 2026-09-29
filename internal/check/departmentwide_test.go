package check

import (
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/amount"
	"github.com/jcrussell/livermore-budget/internal/structure"
)

// This file is the arithmetic behind pp.85-125's UPPER block, and like
// fundingsources_test.go it is deliberately NOT a second call into the machinery
// that publishes it: check.go's doctrine is that two functions over identical
// input inside one process cannot witness a wrong amount. The first test below
// reads the pages with a scanner of its own and compares what the city printed
// against figures typed out of pp.66-67.
//
// The second guards the one exception structure.BudgetBookExceptions declares
// on these pages, which is the part a reader has to take on trust.

// departmentwidePages are the ELEVEN pages that carry an Expenditures by
// Category block, in printed order. Fourteen pages print the DEPARTMENTWIDE
// EXPENDITURES WITH FUNDING SOURCES heading; p102, p120 and p125 are
// continuation pages carrying funding-source rows only, which is why they are
// in fundingPages and not here.
var departmentwidePages = []int{85, 89, 93, 97, 101, 105, 108, 111, 115, 119, 124}

// objectRow matches a printed object line: an optional division label, an object
// category, and the four figures. A bare "-" is a printed zero.
//
// THE LABEL IS SEPARATED BY \s+ AND NOT BY \s{2,}, which is the whole of what
// makes this match p101. A long division label runs right up to the object
// column: p101:11 prints "Community Development Wages & Benefits" with a SINGLE
// space, where every other page leaves a wide run. A two-space rule reads that
// line as no match at all and silently drops the whole division -- the first
// draft of this test did exactly that and lost Community Development Admin's
// four figures out of a sum it then reported as short.
//
// The figures are still separated by \s{2,}, which is what keeps the object name
// from swallowing the first column.
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

// THE ARITHMETIC, read off the pages rather than out of the fact store.
//
// It asserts two things a mis-read column would break: every division's object
// rows sum to its own printed Division Total, and the eleven pages' object rows
// sum, per object category, to the four figures pp.66-67 print citywide.
//
// The FY2026-27 services-and-supplies figure typed below is the SPINE's, and the
// pages come to 250,000 less -- fisc-av0w. That is asserted as a difference
// here, so this test states the discrepancy independently of the check that
// declares it.
func TestThePrintedDepartmentwideRowsSumToTheSpineByObjectCategory(t *testing.T) {
	s, err := Load(LoadOptions{Root: repoRoot(t), Version: testVersion})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
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
				// Five divisions miss their own printed total by exactly one
				// dollar, every one in the FY2023-24 Actual column, and those
				// five are declared as stated_total_deltas in the rule file.
				//
				// THE TOLERATED CELLS ARE COUNTED, NOT WAIVED. An earlier draft
				// allowed a $1 difference in column 0 of every division and said
				// in its comment that "a sixth goes red". It did not: a sixth
				// rounding division would have passed silently in the lane's one
				// independent witness. The count is asserted against the five
				// declarations below.
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
	s, err := Load(LoadOptions{Root: repoRoot(t), Version: testVersion})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
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

	// THE SCAN COUNTS THE PAGES IT READ. An earlier draft looped to a hardcoded
	// 268 and skipped a page it could not open, so the absence assertion below
	// was vacuously true against an unreadable corpus.
	//
	// IT READS EVERY DOCUMENT THE SUBJECT LOADS AND NOT JUST THE BUDGET BOOK. The
	// string it substantiates says this corpus prints no citywide
	// object-category total, and a scan of one document cannot say that.
	//
	// "Every document the subject loads" is TWO of the three that are extracted:
	// the budget book at 268 pages and the ACFR at 195. The CIP is extracted and
	// is not loaded here, because no rule file maps it, so this test cannot
	// speak for its 323 pages. That is a real limit on the claim rather than a
	// gap in the loop, and it is why the assertion below counts pages rather
	// than naming a corpus size.
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
				e.Name, structure.Cents(e.Residual), g.Name, structure.Cents(g.Residual))
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
