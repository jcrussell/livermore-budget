package check

import (
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/amount"
	"github.com/jcrussell/livermore-budget/internal/structure"
)

// TestEveryExceptionResidualIsPrintedWhereItSaysItIs holds each exception's
// Printed claim to the page line it cites and recomputes its residual from
// those figures. cuts-tie-along-the-lattice holds exceptions only to the facts,
// so a residual typed from memory would agree with itself there. The by-object
// entry prints on no page and is held by
// TestDepartmentwideExceptionFiguresAreNotPrintedAndTheirDifferenceIs. An
// exception this test does not know is a failure.
func TestEveryExceptionResidualIsPrintedWhereItSaysItIs(t *testing.T) {
	s, err := Load(LoadOptions{Root: repoRoot(t), Version: testVersion})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	doc, ok := s.Docs[budgetDoc]
	if !ok {
		t.Fatalf("no extraction for %s", budgetDoc)
	}
	line := func(page, n int) string {
		t.Helper()
		text, err := doc.Page(page)
		if err != nil {
			t.Fatalf("page %d: %v", page, err)
		}
		lines := strings.Split(text, "\n")
		if n < 1 || n > len(lines) {
			t.Fatalf("p%04d has %d lines and the citation names line %d", page, len(lines), n)
		}
		return lines[n-1]
	}
	// One printed figure: the page and line the exception cites, and the token
	// as the page prints it.
	type printed struct {
		page, line int
		token      string
	}
	cents := func(p printed) int64 {
		t.Helper()
		if !strings.Contains(line(p.page, p.line), p.token) {
			t.Errorf("p%04d.txt:%d no longer prints %s", p.page, p.line, p.token)
		}
		c, err := amount.Parse(p.token, amount.Dollars)
		if err != nil {
			t.Fatalf("Parse(%q): %v", p.token, err)
		}
		return int64(c)
	}
	// The exception name, the figures its Printed string cites, and how the
	// residual is read off them.
	table := map[string]struct {
		figures  []printed
		residual func(v []int64) int64
	}{
		"pp.127-130-print-no-general-fund-transfer-in-2026": {
			[]printed{{66, 24, "480,400"}}, func(v []int64) int64 { return v[0] }},
		"pp.127-130-print-no-general-fund-transfer-in-2027": {
			[]printed{{66, 24, "486,735"}}, func(v []int64) int64 { return v[0] }},
		"p0067-internal-service-is-250000-high-by-fund-group": {
			[]printed{{67, 34, "26,544,515"}, {183, 64, "26,294,515"}}, func(v []int64) int64 { return v[0] - v[1] }},
	}

	// Exceptions between two schedules that print one total: each cited line
	// prints the same token, and the residual is how differently their rows
	// round to it, which no page prints.
	oneTotal := map[string][]printed{
		"departmentwide-rounds-administrative-services-2024":             {{97, 43, "12,785,955"}, {97, 51, "12,785,955"}},
		"departmentwide-rounds-innovation-and-economic-development-2024": {{111, 27, "5,680,590"}, {111, 37, "5,680,590"}},
		"departmentwide-rounds-library-department-2024":                  {{115, 19, "6,583,809"}, {115, 31, "6,583,809"}},
		"departmentwide-rounds-police-department-2024":                   {{119, 47, "43,463,240"}, {120, 13, "43,463,240"}},
		"departmentwide-rounds-public-works-2024": {{124, 45, "62,853,536"}, {125, 33, "62,853,536"},
			{124, 25, "266,798"}},
		"general-fund-departments-rounds-administrative-services-2024": {{168, 35, "6,311,564"}, {97, 47, "6,311,564"}},
		"general-fund-departments-rounds-community-development-2024":   {{169, 45, "15,925,170"}, {101, 51, "15,925,170"}},
	}

	exceptions := structure.BudgetBookExceptions()
	byName := map[string]structure.Exception{}
	for _, e := range exceptions {
		byName[e.Name] = e
	}
	seen := map[string]bool{}
	for _, e := range exceptions {
		if figures, ok := oneTotal[e.Name]; ok {
			seen[e.Name] = true
			for _, p := range figures {
				cents(p)
				if !strings.Contains(e.Printed, p.token) {
					t.Errorf("exception %s rests on %s and its Printed string does not cite it: %q",
						e.Name, p.token, e.Printed)
				}
			}
			if figures[0].token != figures[1].token {
				t.Errorf("exception %s cites two totals that differ: %s and %s", e.Name,
					figures[0].token, figures[1].token)
			}
			continue
		}
		row, ok := table[e.Name]
		if !ok {
			if g, grounded := byName[e.SameResidualAs]; grounded && table[g.Name].figures != nil {
				continue
			}
			t.Errorf("exception %s cites %q and this test reads no page for it; add its printed "+
				"figures here or ground it in an exception that has them", e.Name, e.Printed)
			continue
		}
		seen[e.Name] = true
		var v []int64
		for _, p := range row.figures {
			v = append(v, cents(p))
			if !strings.Contains(e.Printed, p.token) {
				t.Errorf("exception %s rests on %s and its Printed string does not cite it: %q",
					e.Name, p.token, e.Printed)
			}
		}
		if got := row.residual(v); got != e.Residual {
			t.Errorf("exception %s declares a residual of %s and the pages it cites give %s",
				e.Name, structure.Cents(e.Residual), structure.Cents(got))
		}
	}
	for name := range table {
		if !seen[name] {
			t.Errorf("this test reads pages for %q and no exception of that name is declared", name)
		}
	}
	for name := range oneTotal {
		if !seen[name] {
			t.Errorf("this test reads pages for %q and no exception of that name is declared", name)
		}
	}
}
