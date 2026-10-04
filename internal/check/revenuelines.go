package check

import (
	"context"
	"fmt"
	"strings"

	"github.com/jcrussell/livermore-budget/internal/project"
	"github.com/jcrussell/livermore-budget/internal/registry"
	"github.com/jcrussell/livermore-budget/internal/vocab"
)

// factRevenueLinesResolve asserts that the revenue rows pp.127-140 print and
// the lines data/taxonomy.yaml declares are the same set, spelled the same way:
// every revenue-by-fund revenue fact resolves its (category, row_label) to
// exactly one line by document_term or alias, byte for byte, and every line is
// printed by some fact. Either arm alone is fail-open: a re-typeset row leaves
// the old spelling declared and the new one unresolved.
//
// A line is a child of an assignable category that declares the revenue kind;
// a child of a rollup (taxes/property under taxes) is a category. A fact whose
// category is itself a line fails the first arm, which fact-vocabulary cannot
// see because a line is assignable. The label is never a match key.
type factRevenueLinesResolve struct{}

var _ Check = (*factRevenueLinesResolve)(nil)

func (*factRevenueLinesResolve) ID() string { return "fact-revenue-lines-resolve" }
func (*factRevenueLinesResolve) Tier() int  { return 1 }
func (*factRevenueLinesResolve) Full() bool { return false }
func (*factRevenueLinesResolve) Description() string {
	return "every revenue row of pp.127-140 resolves to exactly one line data/taxonomy.yaml " +
		"declares under its category, and every declared line is printed by some row"
}

func (*factRevenueLinesResolve) Run(_ context.Context, s *Subject) (Result, error) {
	var findings []Finding

	// Slug order, so findings do not reorder between runs.
	lines := map[string][]registry.Category{} // category slug -> the lines under it
	var declared []string
	for _, c := range s.Vocabulary.Categories() {
		if c.Parent == "" || !s.Vocabulary.Assignable(c.Parent) || !declaresKind(c, vocab.KindRevenue) {
			continue
		}
		lines[c.Parent] = append(lines[c.Parent], c)
		declared = append(declared, c.Slug)
	}

	printed := map[string]bool{}
	rows := 0
	for _, f := range s.Facts {
		if f.Scope != project.ScopeRevenueByFund || f.Kind != vocab.KindRevenue {
			continue
		}
		rows++
		var hits []string
		for _, c := range lines[f.Category] {
			if printsAs(c, f.RowLabel) {
				hits = append(hits, c.Slug)
			}
		}
		// An ambiguous row prints both its lines; the ambiguity is reported once.
		for _, h := range hits {
			printed[h] = true
		}
		switch len(hits) {
		case 1:
		case 0:
			findings = append(findings, finding(f.ID,
				"%s p%d: category %q declares no line printed as %q in %s, so the row has "+
					"no node and a projection must refuse it",
				f.DocID, f.Page, f.Category, f.RowLabel, taxonomyFile))
		default:
			findings = append(findings, finding(f.ID,
				"%s p%d %q: resolves to %d lines under %q, %s; one printed row is one node",
				f.DocID, f.Page, f.RowLabel, len(hits), f.Category, strings.Join(hits, ", ")))
		}
	}

	for _, slug := range declared {
		if !printed[slug] {
			findings = append(findings, finding(taxonomyFile,
				"line %q is printed by no fact of scope %s and kind %s; a node nothing can draw",
				slug, project.ScopeRevenueByFund, vocab.KindRevenue))
		}
	}

	return conclusion{
		subjects: rows + len(declared),
		unit:     "rows and lines",
		held: fmt.Sprintf("%d revenue rows of scope %s each resolve to one of the %d lines %s "+
			"declares under %d categories, and every line is printed",
			rows, project.ScopeRevenueByFund, len(declared), taxonomyFile, len(lines)),
		nothing: fmt.Sprintf("no fact is a revenue row of scope %s and %s declares no line "+
			"under an assignable revenue category", project.ScopeRevenueByFund, taxonomyFile),
		findings: findings,
	}.result(), nil
}

// printsAs reports whether label is c's document_term or one of its aliases,
// exactly. An empty document_term matches nothing.
func printsAs(c registry.Category, label string) bool {
	if c.DocumentTerm != "" && c.DocumentTerm == label {
		return true
	}
	for _, a := range c.Aliases {
		if a.Term == label {
			return true
		}
	}
	return false
}
