package check

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/google/go-cmp/cmp"

	"github.com/jcrussell/livermore-budget/internal/project"
	"github.com/jcrussell/livermore-budget/internal/registry"
	"github.com/jcrussell/livermore-budget/internal/vocab"
)

// linesTaxonomyYAML is the fixture taxonomy with two lines under
// charges-for-services, one re-spelled by an alias. taxes/property is not a
// line, because `taxes` is a rollup.
const linesTaxonomyYAML = testTaxonomyYAML + `  - slug: charges-for-services/plan-check-fees
    label: Plan Check Fees
    document_term: Plan Check Fees
    parent: charges-for-services
    kinds: [revenue]
    pages: [129]
  - slug: charges-for-services/library-fees
    label: Library Fees
    document_term: Library Fees
    parent: charges-for-services
    kinds: [revenue]
    pages: [129]
    aliases:
      - {term: "Library Fees & Fines", pages: [131]}
`

// revenueRow is one printed row of pp.127-140 as the check sees it: the
// category a rule classified it as, and the label the page prints.
type revenueRow struct {
	kind     vocab.Kind
	category string
	label    string
}

// revenueLinesSubject is the fixture facts with the first len(rows) rewritten
// as revenue-by-fund rows, over a taxonomy of the caller's choosing.
func revenueLinesSubject(t *testing.T, taxonomy string, rows ...revenueRow) *Subject {
	t.Helper()
	reg, err := registry.Load(fstest.MapFS{
		registry.FundsFile:       &fstest.MapFile{Data: []byte(testFundsYAML)},
		registry.TaxonomyFile:    &fstest.MapFile{Data: []byte(taxonomy)},
		registry.DepartmentsFile: &fstest.MapFile{Data: []byte(testDepartmentsYAML)},
	})
	if err != nil {
		t.Fatalf("load the fixture registries: %v", err)
	}
	facts := testFacts()
	if len(rows) > len(facts) {
		t.Fatalf("%d rows over %d fixture facts", len(rows), len(facts))
	}
	for i, r := range rows {
		facts[i].Scope = project.ScopeRevenueByFund
		facts[i].Kind = r.kind
		facts[i].Category = r.category
		facts[i].RowLabel = r.label
	}
	return &Subject{Facts: facts[:len(rows)], Vocabulary: reg}
}

func revenue(category, label string) revenueRow {
	return revenueRow{vocab.KindRevenue, category, label}
}

// TestRevenueRowsAndLinesAreOneSet is the check's own table.
func TestRevenueRowsAndLinesAreOneSet(t *testing.T) {
	t.Parallel()
	plan := revenue("charges-for-services", "Plan Check Fees")
	library := revenue("charges-for-services", "Library Fees")

	tests := []struct {
		name     string
		taxonomy string
		rows     []revenueRow
		status   Status
		subjects int
		want     []string // each must occur in the findings, and there are len(want) of them
		summary  string
	}{
		{
			name: "every row resolves and every line is printed", taxonomy: linesTaxonomyYAML,
			rows: []revenueRow{plan, library}, status: StatusPass, subjects: 4,
			summary: "2 revenue rows of scope revenue-by-fund each resolve to one of the 2 lines",
		}, {
			name: "a row resolving through an alias", taxonomy: linesTaxonomyYAML,
			rows:   []revenueRow{plan, revenue("charges-for-services", "Library Fees & Fines")},
			status: StatusPass, subjects: 4,
		}, {
			name: "a row the taxonomy declares no line for", taxonomy: linesTaxonomyYAML,
			rows:   []revenueRow{plan, library, revenue("charges-for-services", "Weed Abatement")},
			status: StatusFail, subjects: 5,
			want: []string{`category "charges-for-services" declares no line printed as "Weed Abatement"`},
		}, {
			name: "a line nothing prints", taxonomy: linesTaxonomyYAML,
			rows: []revenueRow{plan}, status: StatusFail, subjects: 3,
			want: []string{`line "charges-for-services/library-fees" is printed by no fact`},
		}, {
			// One drift, both arms: the row resolves to nothing and its line
			// goes unprinted.
			name: "a row spelled differently from its line", taxonomy: linesTaxonomyYAML,
			rows:   []revenueRow{revenue("charges-for-services", "Plan check fees"), library},
			status: StatusFail, subjects: 4,
			want: []string{
				`declares no line printed as "Plan check fees"`,
				`line "charges-for-services/plan-check-fees" is printed by no fact`,
			},
		}, {
			// A rule that classified the row as the line.
			name: "a row whose category is itself a line", taxonomy: linesTaxonomyYAML,
			rows:   []revenueRow{revenue("charges-for-services/plan-check-fees", "Plan Check Fees"), library},
			status: StatusFail, subjects: 4,
			want: []string{
				`category "charges-for-services/plan-check-fees" declares no line printed as "Plan Check Fees"`,
				`line "charges-for-services/plan-check-fees" is printed by no fact`,
			},
		}, {
			// registry.Load allows two categories one term (fund-balance/beginning
			// and /ending both print "Fund Balance / Working Capital"), so only
			// this arm refuses two sibling lines claiming a row.
			name: "a row resolving to two lines",
			taxonomy: linesTaxonomyYAML + `  - slug: charges-for-services/plan-check
    label: Plan Check
    document_term: Plan Check
    parent: charges-for-services
    kinds: [revenue]
    pages: [129]
    aliases:
      - {term: "Plan Check Fees", pages: [129]}
`,
			rows: []revenueRow{plan, library}, status: StatusFail, subjects: 5,
			want: []string{`"Plan Check Fees": resolves to 2 lines under "charges-for-services", ` +
				`charges-for-services/plan-check, charges-for-services/plan-check-fees`},
		}, {
			// Declared lines and no facts is a failure, not vacuous.
			name: "lines declared and no row printed", taxonomy: linesTaxonomyYAML,
			rows: nil, status: StatusFail, subjects: 2,
			want: []string{
				`line "charges-for-services/library-fees" is printed by no fact`,
				`line "charges-for-services/plan-check-fees" is printed by no fact`,
			},
		}, {
			// Scope and kind are both required.
			name: "rows outside the scope or kind are not subjects", taxonomy: testTaxonomyYAML,
			rows:   []revenueRow{{vocab.KindTransferIn, "transfers/in", "Transfers In"}},
			status: StatusVacuous, subjects: 0,
			summary: "no fact is a revenue row of scope revenue-by-fund and data/taxonomy.yaml declares no line",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := revenueLinesSubject(t, tt.taxonomy, tt.rows...)
			res, err := (&factRevenueLinesResolve{}).Run(t.Context(), s)
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			if res.Status != tt.status {
				t.Fatalf("status = %s (%s), want %s", res.Status, res.Summary, tt.status)
			}
			if res.Subjects != tt.subjects {
				t.Errorf("subjects = %d, want %d (rows plus lines)", res.Subjects, tt.subjects)
			}
			if got := len(res.Findings); got != len(tt.want) {
				t.Errorf("findings = %d, want %d:\n%s", got, len(tt.want), findingDetails(res))
			}
			for _, w := range tt.want {
				if !strings.Contains(findingDetails(res), w) {
					t.Errorf("findings do not contain %q:\n%s", w, findingDetails(res))
				}
			}
			if tt.summary != "" && !strings.Contains(res.Summary, tt.summary) {
				t.Errorf("summary %q does not contain %q", res.Summary, tt.summary)
			}
			// fact-vocabulary is green over the same subject, so no earlier
			// gate hides this one.
			voc, err := (&factVocabulary{}).Run(t.Context(), s)
			if err != nil {
				t.Fatalf("fact-vocabulary: %v", err)
			}
			if voc.Status == StatusFail {
				t.Errorf("fact-vocabulary = fail over the same rows; the finding above is not this check's alone:\n%s",
					findingDetails(voc))
			}
		})
	}
}

// TestEveryRevenueLineIsARowOfTheMapping makes fact-revenue-lines-resolve's
// claim against the rule file rather than the fact store, so a rule row no
// page yields a fact for is caught here.
//
// Mutation: move taxes/property/eraf to taxes/other/eraf; this fails twice,
// naming the line no rule row carries and the rule row no line resolves.
func TestEveryRevenueLineIsARowOfTheMapping(t *testing.T) {
	t.Parallel()
	s := committed(t)

	type row struct{ category, label string }
	rows := map[row]int{} // distinct (category, printed label) over the rule files
	for _, f := range s.Files {
		for i := range f.Rules {
			r := &f.Rules[i]
			if r.Scope != project.ScopeRevenueByFund {
				continue
			}
			for _, rw := range r.Rows {
				if rw.EffectiveKind(r) != vocab.KindRevenue {
					continue
				}
				rows[row{rw.Category, rw.PrintedLabel()}]++
			}
		}
	}

	resolved := map[row][]string{} // rule row -> the lines claiming it
	var lines int
	for _, c := range s.Vocabulary.Categories() {
		if c.Parent == "" || !s.Vocabulary.Assignable(c.Parent) || !declaresKind(c, vocab.KindRevenue) {
			continue
		}
		lines++
		terms := []string{c.DocumentTerm}
		for _, a := range c.Aliases {
			terms = append(terms, a.Term)
		}
		carried := 0
		for _, term := range terms {
			k := row{c.Parent, term}
			if _, ok := rows[k]; ok {
				resolved[k] = append(resolved[k], c.Slug)
				carried++
			}
		}
		if carried == 0 {
			t.Errorf("line %q: no revenue-by-fund rule row is {label: %q, category: %s}; "+
				"the entry names a row no rule reads", c.Slug, c.DocumentTerm, c.Parent)
		}
	}
	for k := range rows {
		if got := resolved[k]; len(got) != 1 {
			t.Errorf("rule row {label: %q, category: %s} resolves to %d lines %v, want exactly 1",
				k.label, k.category, len(got), got)
		}
	}

	// pp.127-140 print 101 distinct (category, row) revenue pairs. Both sides
	// are pinned so a deleted line and a deleted rule row cannot cancel out.
	if got := len(rows); got != 101 {
		t.Errorf("the rule files carry %d distinct revenue rows of scope %s, want 101; "+
			"the pin is a count against pp.127-140 and means nothing if the schedule changed",
			got, project.ScopeRevenueByFund)
	}
	if lines != 101 {
		t.Errorf("the registry declares %d revenue lines, want 101 for the same reason", lines)
	}
}

// TestRevenueLinesAreDistinctFromRollupChildren pins over the committed
// registry that a line is a child of an assignable category, told apart by the
// flag and never by depth.
func TestRevenueLinesAreDistinctFromRollupChildren(t *testing.T) {
	t.Parallel()
	s := committed(t)
	var lines, rollupChildren []string
	for _, c := range s.Vocabulary.Categories() {
		switch {
		case c.Parent == "":
		case s.Vocabulary.Assignable(c.Parent):
			lines = append(lines, c.Slug)
		default:
			rollupChildren = append(rollupChildren, c.Slug)
		}
	}
	wantRollupChildren := []string{
		"fund-balance/assigned", "fund-balance/beginning", "fund-balance/change",
		"fund-balance/committed", "fund-balance/ending", "fund-balance/excess-of-revenues",
		"fund-balance/nonspendable", "fund-balance/reserve-increase", "fund-balance/restricted",
		"fund-balance/unassigned", "fund-balance/use-for-cip", "taxes/other", "taxes/property", "taxes/sales",
		"transfers/in", "transfers/out", "transfers/out-to-cip",
	}
	if diff := cmp.Diff(wantRollupChildren, rollupChildren); diff != "" {
		t.Errorf("children of the three rollups (-want +got):\n%s", diff)
	}
	if len(lines) == 0 {
		t.Fatal("the committed registry declares no revenue line")
	}
	// Every line's parent is its slug minus the last segment.
	for _, slug := range lines {
		c, _ := s.Vocabulary.Category(slug)
		if want := slug[:strings.LastIndexByte(slug, '/')]; c.Parent != want {
			t.Errorf("line %q has parent %q, want %q", slug, c.Parent, want)
		}
	}
}
