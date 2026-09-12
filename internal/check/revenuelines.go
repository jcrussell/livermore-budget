package check

import (
	"context"
	"fmt"

	"github.com/jcrussell/livermore-budget/internal/mapping"
	"github.com/jcrussell/livermore-budget/internal/registry"
)

// factRevenueLinesResolve asserts that the revenue rows pp.127-140 print and
// the line entries data/taxonomy.yaml declares under the revenue categories are
// the same set, spelled the same way.
//
// A LINE IS A CHILD OF AN ASSIGNABLE CATEGORY that declares the revenue kind.
// That is the whole definition, and it is structural rather than a flag: a fact
// reaches a line through (category, row_label), so a line's parent is a
// category a rule may write, and a child of a rollup — taxes/property under
// `taxes`, which no rule may write — is a category and not a line. The fact
// store carries no line slug; row_label is free text off the page, and this
// check is what binds it to an identity.
//
// Two arms, and either alone is fail-open:
//
//   - every fact of scope revenue-by-fund and kind revenue resolves its
//     (category, row_label) to EXACTLY ONE line, by the line's document_term or
//     one of its aliases, byte for byte. A row that resolves to nothing is a row
//     the projection has no node for and must refuse at build time; this says so
//     before anyone builds. A row that resolves to two is an identity nothing
//     can draw once.
//   - every line is printed by at least one such fact. An entry nothing prints
//     is a node nothing can ever draw, and it is exactly what a re-typeset row
//     leaves behind: the old spelling stays declared while the new one arrives
//     as an unresolved fact. Only both arms together report that drift as one
//     event instead of letting the stale entry stand.
//
// A fact whose category is ITSELF a line — a rule that wrote
// `category: taxes/property/eraf` — fails the first arm, because a line has no
// lines under it. fact-vocabulary does not catch that, since a line is
// assignable; the guard against a rule classifying a fact as a row rather than
// as its category is here.
//
// The label is never a match key. Label is what the site prints and carries no
// pages; a spelling the city prints is document_term or an alias, each of which
// says where to go and look, so the binding stays checkable. A line with no
// document_term and no alias can resolve nothing and is reported by the second
// arm, which is why the loader needs no rule for it.
//
// The "Transfers In" row pp.131-140 print is not a subject: its kind is
// transfer_in and its category transfers/in, and the tier it draws at is the
// category's own.
//
// WHAT IT CANNOT WITNESS. This compares the registry against the fact store's
// labels and nothing else. A fact resolving to the right line with the wrong
// amount passes here; that is fact-offset-points-at-token's claim. Nor does it
// read a page or a rule file — that a document_term is printed where its pages
// say is internal/registry's TestEveryRevenueLineIsPrintedOnItsPages, and that
// every line is a row of the rule file is TestEveryRevenueLineIsARowOfTheMapping.
// Three witnesses, because generated entries are reviewable only against
// something that did not generate them.
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

	// Slug order, because Categories is: two unprinted lines would otherwise
	// swap places between runs, and a report whose findings reorder cannot be
	// diffed across releases.
	lines := map[string][]registry.Category{} // category slug -> the lines under it
	var declared []string
	for _, c := range s.Vocabulary.Categories() {
		if c.Parent == "" || !s.Vocabulary.Assignable(c.Parent) || !declaresKind(c, mapping.KindRevenue) {
			continue
		}
		lines[c.Parent] = append(lines[c.Parent], c)
		declared = append(declared, c.Slug)
	}

	printed := map[string]bool{}
	rows := 0
	for _, f := range s.Facts {
		if f.Scope != revenueDetailScope || f.Kind != mapping.KindRevenue {
			continue
		}
		rows++
		var hits []string
		for _, c := range lines[f.Category] {
			if printsAs(c, f.RowLabel) {
				hits = append(hits, c.Slug)
			}
		}
		// Every hit is printed, even when there are two: the second arm is about
		// a line nothing prints, and an ambiguous row prints both of its lines.
		// The ambiguity is the first arm's finding and is reported once.
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
				f.DocID, f.Page, f.RowLabel, len(hits), f.Category, joinComma(hits)))
		}
	}

	for _, slug := range declared {
		if !printed[slug] {
			findings = append(findings, finding(taxonomyFile,
				"line %q is printed by no fact of scope %s and kind %s; a node nothing can draw",
				slug, revenueDetailScope, mapping.KindRevenue))
		}
	}

	return conclusion{
		subjects: rows + len(declared),
		unit:     "rows and lines",
		held: fmt.Sprintf("%d revenue rows of scope %s each resolve to one of the %d lines %s "+
			"declares under %d categories, and every line is printed",
			rows, revenueDetailScope, len(declared), taxonomyFile, len(lines)),
		nothing: fmt.Sprintf("no fact is a revenue row of scope %s and %s declares no line "+
			"under an assignable revenue category", revenueDetailScope, taxonomyFile),
		findings: findings,
	}.result(), nil
}

// printsAs reports whether label is a spelling c declares the city prints for
// it: its document_term or one of its aliases, exactly. An empty document_term
// matches nothing, so an entry that declares no printed spelling resolves no
// row.
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
