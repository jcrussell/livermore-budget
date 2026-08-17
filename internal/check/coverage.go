package check

import (
	"context"
	"fmt"

	"github.com/jcrussell/livermore-budget/internal/project"
)

// unprojectedScopes are the scopes deliberately not drawn by any projection, each
// with the reason it is not.
//
// It is empty today, and an entry is a declaration rather than a note: it says a
// human decided those facts belong in the store and not in a chart. Without the
// declaration, a fact no projection covers is a failure — because the alternative
// is what this map exists to stop. Moving twenty facts to a mistyped
// `all-funds-gross-detail` took over half the city's revenue out of the published
// chart, and every check still passed: the graph checks only ever see the facts a
// projection selected, so facts that fall outside every slice are not checked, they
// are ignored.
//
// A scope listed here is still checked by everything above the projection line —
// sorted, unique ids, token re-parse, offsets, vocabulary — because those read the
// fact store directly.
var unprojectedScopes = map[string]string{}

// publishedProjectionBuilt asserts the slice the site publishes was one of the
// slices built.
//
// Every graph check reads Subject.Projections, so a projection that was not built
// is not a failure anywhere: it is silence. `fisc export` publishes exactly
// (project.PublishedFiscalYear, PublishedBasis, PublishedScope), and if the fact
// store no longer carries facts for that triple, export publishes a chart of
// nothing while verify reports on whatever other slices happen to exist.
//
// This is the check that closes the divergence a shared constant cannot: the
// spelling being identical everywhere says nothing about whether the facts are
// there.
type publishedProjectionBuilt struct{}

var _ Check = (*publishedProjectionBuilt)(nil)

func (*publishedProjectionBuilt) ID() string { return "published-projection-built" }
func (*publishedProjectionBuilt) Tier() int  { return 1 }
func (*publishedProjectionBuilt) Full() bool { return false }
func (*publishedProjectionBuilt) Description() string {
	return "the fiscal year, basis and scope `fisc export` publishes is among the projections " +
		"these checks were run over"
}

// Run always has exactly one subject: the published triple either was built or was
// not. There is no state of the corpus in which this check has nothing to look at,
// which is why it is not routed through conclusion.
func (*publishedProjectionBuilt) Run(_ context.Context, s *Subject) (Result, error) {
	want := fmt.Sprintf("FY%d %s %s",
		project.PublishedFiscalYear, project.PublishedBasis, project.PublishedScope)

	built := make([]string, 0, len(s.Projections))
	for _, p := range s.Projections {
		o := p.Options
		if o.FiscalYear == project.PublishedFiscalYear &&
			o.Basis == project.PublishedBasis &&
			o.Scope == project.PublishedScope {
			return conclusion{
				subjects: 1,
				unit:     "published slice",
				held:     fmt.Sprintf("the published slice %s was built and checked", want),
			}.result(), nil
		}
		built = append(built, p.String())
	}

	detail := "no projection was built at all"
	if len(built) > 0 {
		detail = fmt.Sprintf("the projections built were: %s", joinComma(built))
	}
	return conclusion{
		subjects: 1,
		unit:     "published slice",
		findings: []Finding{finding(want,
			"`fisc export` publishes this slice and nothing checked it — %s. Every check below "+
				"this one reads the projections, so a slice that was not built is not failed, "+
				"it is unexamined", detail)},
	}.result(), nil
}

// factsAreProjected asserts no fact quietly falls outside every projection.
//
// A fact in no projection's slice is read by none of the graph checks. That is not
// a hole in one check, it is a hole under all of them at once, and it opens on a
// one-word edit: a rule whose `scope:` is mistyped still produces facts, still
// sorts, still resolves in the taxonomy, and still has a token that re-parses — so
// every check that reads the store passes, while the figures leave the chart.
type factsAreProjected struct{}

var _ Check = (*factsAreProjected)(nil)

func (*factsAreProjected) ID() string { return "facts-are-projected" }
func (*factsAreProjected) Tier() int  { return 1 }
func (*factsAreProjected) Full() bool { return false }
func (*factsAreProjected) Description() string {
	return "every fact is in some projection's slice, or its scope is declared as one no " +
		"projection draws"
}

func (*factsAreProjected) Run(_ context.Context, s *Subject) (Result, error) {
	projected := map[string]bool{}
	for _, p := range s.Projections {
		for _, f := range factsFor(s.Facts, p.Options) {
			projected[f.ID] = true
		}
	}

	var findings []Finding
	declared := map[string]int{}
	for _, f := range s.Facts {
		if projected[f.ID] {
			continue
		}
		if _, ok := unprojectedScopes[f.Scope]; ok {
			declared[f.Scope]++
			continue
		}
		findings = append(findings, finding(f.ID,
			"%s p%d %q is FY%d %s %s, which no projection is of and which no entry in "+
				"unprojectedScopes declares unprojected",
			f.DocID, f.Page, f.RowLabel, f.FiscalYear, f.Basis, f.Scope))
	}

	held := fmt.Sprintf("%d facts, all of them in one of %d projections",
		len(s.Facts), len(s.Projections))
	if len(declared) > 0 {
		held = fmt.Sprintf("%d facts in %d projections, plus %s declared unprojected",
			len(projected), len(s.Projections), describeDeclared(declared))
	}
	return conclusion{
		subjects: len(s.Facts),
		unit:     "facts",
		held:     held,
		nothing:  "the fact store is empty",
		findings: findings,
	}.result(), nil
}

// describeDeclared renders the deliberately unprojected facts, by scope, with the
// declared reason: a count alone would let a growing exclusion pass unread.
func describeDeclared(byScope map[string]int) string {
	out := make([]string, 0, len(byScope))
	for _, scope := range sortedStrings(byScope) {
		out = append(out, fmt.Sprintf("%d in scope %q (%s)",
			byScope[scope], scope, unprojectedScopes[scope]))
	}
	return joinComma(out)
}
