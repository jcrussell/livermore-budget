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
		"p0067-internal-service-is-250000-high-by-fund-total": {
			[]printed{{67, 34, "26,544,515"}, {209, 20, "26,294,515"}}, func(v []int64) int64 { return v[0] - v[1] }},
		"p0067-internal-service-ending-is-250000-low": {
			[]printed{{67, 42, "8,529,087"}, {209, 20, "8,779,087"}}, func(v []int64) int64 { return v[0] - v[1] }},
		"pp.127-130-print-no-general-fund-transfer-in-2024": {
			[]printed{{186, 10, "737,455"}}, func(v []int64) int64 { return v[0] }},
		"pp.127-130-print-no-general-fund-transfer-in-2025": {
			[]printed{{192, 10, "914,206"}}, func(v []int64) int64 { return v[0] }},
		"pp.127-130-print-no-general-fund-transfer-in-pp.186-209-2026": {
			[]printed{{198, 10, "480,400"}}, func(v []int64) int64 { return v[0] }},
		"pp.127-130-print-no-general-fund-transfer-in-pp.186-209-2027": {
			[]printed{{204, 10, "486,735"}}, func(v []int64) int64 { return v[0] }},
		"pp.127-130-print-no-general-fund-transfer-in-p76-2026": {
			[]printed{{76, 19, "19,250"}, {76, 23, "250,000"}, {76, 25, "77,250"}, {76, 27, "133,900"}},
			func(v []int64) int64 { return v[0] + v[1] + v[2] + v[3] }},
		"pp.127-130-print-no-general-fund-transfer-in-p76-2027": {
			[]printed{{76, 19, "19,250"}, {76, 23, "250,000"}, {76, 25, "79,568"}, {76, 27, "137,917"}},
			func(v []int64) int64 { return v[0] + v[1] + v[2] + v[3] }},
		"pp.131-140-print-no-general-fund-cip-reserves-2025": {
			[]printed{{194, 29, "4,125,627"}}, func(v []int64) int64 { return v[0] }},
		"pp.186-209-carry-no-police-donations-other-financing-2025": {
			[]printed{{139, 56, "500"}, {139, 58, "5,500"}, {194, 10, "5,000"}}, func(v []int64) int64 { return v[2] - v[1] }},
		"pp.85-125-fund-maintenances-transfer-out-2024": {
			[]printed{{124, 25, "266,798"}, {124, 61, "694,574"}, {187, 60, "694,574"}, {124, 63, "2,965,602"},
				{187, 61, "2,965,602"}}, func(v []int64) int64 { return -v[0] }},
	}
	// pp.66-67's CHANGE IN WORKING CAPITAL, which pp.186-209 print no line for.
	for _, c := range []struct {
		name  string
		page  int
		token string
	}{
		{"general-2026", 66, "(1,034,154)"}, {"enterprise-2026", 66, "3,894,384"},
		{"capital-2026", 67, "(2,500,213)"}, {"internal-service-2026", 67, "(6,147,533)"},
		{"special-revenue-2026", 67, "8,874,949"},
		{"general-2027", 66, "2,351,098"}, {"enterprise-2027", 66, "1,410,280"},
		{"capital-2027", 67, "(10,129,416)"}, {"internal-service-2027", 67, "(7,160,645)"},
		{"special-revenue-2027", 67, "9,021,972"},
	} {
		n := map[int]int{66: 39, 67: 41}[c.page]
		table["pp.186-209-print-no-change-line-"+c.name] = struct {
			figures  []printed
			residual func(v []int64) int64
		}{[]printed{{c.page, n, c.token}}, func(v []int64) int64 { return v[0] }}
	}

	// Exceptions where one schedule prints a total and the other divides it
	// among several cells: the parts sum to the total, and the residual is how
	// the first schedule's rows round under it, which no page prints.
	divided := map[string]struct {
		total printed
		parts []printed
	}{
		"pp.127-140-print-three-funds-revenue-as-the-state-grant-funds-2024": {
			printed{137, 30, "1,473,006"}, []printed{{186, 31, "208,540"}, {186, 32, "761"}, {186, 44, "1,263,705"}}},
		"pp.173-183-print-four-funds-spending-as-the-state-grant-funds-2024": {
			printed{179, 68, "1,135,142"}, []printed{{187, 30, "465,115"}, {187, 31, "778"}, {187, 32, "544"}, {187, 43, "668,705"}}},
		"pp.85-125-fund-four-funds-spending-from-the-state-grant-fund-2024": {
			printed{179, 68, "1,135,142"}, []printed{{187, 30, "465,115"}, {187, 31, "778"}, {187, 32, "544"}, {187, 43, "668,705"}}},
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
		"general-fund-departments-rounds-services-and-supplies-2024":   {{170, 23, "123,228,190"}, {172, 22, "123,228,190"}},
		"pp.186-209-carry-a-dollar-of-capital-beginning-2026":          {{67, 40, "110,713,545"}, {200, 44, "110,713,545"}},
		"pp.186-209-carry-a-dollar-of-capital-ending-2026":             {{67, 42, "108,213,332"}, {201, 46, "108,213,332"}},
		"pp.186-209-carry-a-dollar-of-capital-beginning-2027":          {{67, 40, "108,213,332"}, {206, 44, "108,213,332"}},
		"pp.186-209-carry-a-dollar-of-capital-ending-2027":             {{67, 42, "98,083,916"}, {207, 46, "98,083,916"}},
		"pp.186-209-carry-a-dollar-of-special-revenue-beginning-2026":  {{67, 40, "77,290,073"}, {200, 11, "77,290,073"}},
		"pp.186-209-carry-a-dollar-of-special-revenue-ending-2026":     {{67, 42, "86,165,022"}, {201, 10, "86,165,022"}},
		"pp.186-209-carry-a-dollar-of-special-revenue-beginning-2027":  {{67, 40, "86,165,022"}, {206, 11, "86,165,022"}},
		"pp.186-209-carry-a-dollar-of-special-revenue-ending-2027":     {{67, 42, "95,186,994"}, {207, 10, "95,186,994"}},
		"pp.127-140-round-low-income-housing-revenue-2024":             {{135, 18, "5,852,912"}, {186, 27, "5,852,912"}},
		"pp.127-140-round-airport-revenue-2024":                        {{131, 20, "4,886,525"}, {188, 48, "4,886,525"}},
		"p0172-rounds-general-fund-expenses-2024":                      {{172, 24, "123,228,190"}, {187, 9, "123,228,190"}},
		"pp.173-183-round-downtown-lmd-expenses-2024":                  {{179, 11, "694,564"}, {187, 60, "694,564"}},
		"pp.173-183-round-other-maintenance-cfds-expenses-2024":        {{181, 52, "176,138"}, {187, 63, "176,138"}},
		"pp.173-183-round-airport-expenses-2024":                       {{173, 20, "3,000,829"}, {189, 51, "3,000,829"}},
		"pp.173-183-round-water-expenses-2024":                         {{174, 13, "16,935,869"}, {189, 57, "16,935,869"}},
		"pp.173-183-round-facilities-rehab-expenses-2024":              {{183, 22, "2,487,238"}, {191, 9, "2,487,238"}},
	}

	exceptions := structure.BudgetBookExceptions()
	byName := map[string]structure.Exception{}
	for _, e := range exceptions {
		byName[e.Name] = e
	}
	seen := map[string]bool{}
	for _, e := range exceptions {
		if d, ok := divided[e.Name]; ok {
			seen[e.Name] = true
			total, sum := cents(d.total), int64(0)
			for _, p := range append([]printed{d.total}, d.parts...) {
				if !strings.Contains(e.Printed, p.token) {
					t.Errorf("exception %s rests on %s and its Printed string does not cite it: %q",
						e.Name, p.token, e.Printed)
				}
			}
			for _, p := range d.parts {
				sum += cents(p)
			}
			if sum != total {
				t.Errorf("exception %s: the parts it cites sum to %s and the total it cites is %s",
					e.Name, amount.Cents(sum), amount.Cents(total))
			}
			if len(d.parts) != len(e.Cells) {
				t.Errorf("exception %s pins %d cells and cites %d parts", e.Name, len(e.Cells), len(d.parts))
				continue
			}
			// Each part is the cell it is cited for, and the residual is
			// what the total holds over the rows the other side sums.
			var rows int64
			for i, p := range d.parts {
				if got := cents(p); got != e.Cells[i].Against.Cents {
					t.Errorf("exception %s cites %s for its cell %d and pins %s", e.Name, p.token, i,
						amount.Cents(e.Cells[i].Against.Cents))
				}
				rows += e.Cells[i].Cut.Cents
			}
			if got := total - rows; got != e.Residual {
				t.Errorf("exception %s declares a residual of %s and its cited total holds %s over its rows",
					e.Name, amount.Cents(e.Residual), amount.Cents(got))
			}
			continue
		}
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
				e.Name, amount.Cents(e.Residual), amount.Cents(got))
		}
	}
	for name := range table {
		if !seen[name] {
			t.Errorf("this test reads pages for %q and no exception of that name is declared", name)
		}
	}
	for name := range divided {
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
