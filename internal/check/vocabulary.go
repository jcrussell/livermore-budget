package check

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/jcrussell/livermore-budget/internal/registry"
	"github.com/jcrussell/livermore-budget/internal/vocab"
)

// factVocabulary asserts every category and fund group a fact carries resolves in
// the curated registries. Departments and fund numbers have checks of their
// own, so each axis is separately greppable and declarable.
type factVocabulary struct{}

func (*factVocabulary) meta() meta {
	return meta{id: "fact-vocabulary", tier: 1,
		description: "every category a fact carries is assignable in data/taxonomy.yaml, and every fund " +
			"group is a type data/funds.yaml uses"}
}

// Run counts one subject per classification, not per fact. An empty category
// is not a finding here; internal/project refuses an unclassified fact.
func (*factVocabulary) Run(_ context.Context, s *Subject) (Result, error) {
	var findings []Finding
	categories, groups := 0, 0

	for _, f := range s.Facts {
		if f.Category != "" {
			categories++
			if !s.Vocabulary.Assignable(f.Category) {
				findings = append(findings, finding(f.ID, "%s p%d %q: %s",
					f.DocID, f.Page, f.RowLabel, unassignable(s.Vocabulary, f.Category)))
			}
		}
		if f.FundGroup != "" {
			groups++
			if !s.Vocabulary.FundGroup(f.FundGroup) {
				findings = append(findings, finding(f.ID, "%s p%d %q: fund group %q is not a "+
					"type any fund in data/funds.yaml is recorded under",
					f.DocID, f.Page, f.RowLabel, f.FundGroup))
			}
		}
	}

	n := categories + groups
	return conclusion{
		subjects: n,
		unit:     "classifications",
		held: fmt.Sprintf("%d classifications over %d facts: %d categories, %d fund groups",
			n, len(s.Facts), categories, groups),
		nothing:  "no fact carries a category or a fund group",
		findings: findings,
	}.result(), nil
}

// unassignable says why a category may not be written: a rollup, or a slug the
// taxonomy does not define.
func unassignable(v Vocabulary, slug string) string {
	if c, ok := v.Category(slug); ok {
		return fmt.Sprintf("category %q is the rollup %q, which data/taxonomy.yaml declares "+
			"assignable: false — it exists so a view can group, and a rule may not assign it",
			slug, c.Label)
	}
	return fmt.Sprintf("category %q is not defined in data/taxonomy.yaml", slug)
}

// factKindMatchesCategory asserts every fact's kind is one its category declares.
// `category: transfers/in` + `kind: revenue` passes factVocabulary; this fails
// it at the fact (fisc-u2v, failure route 3). Direction is caught by the same
// literal comparison: transfers/in declares transfer_in (fisc-ttq).
//
// A category factVocabulary already names, undefined or a rollup, is skipped,
// so one typo does not redden two vocabulary checks (fisc-9nw).
type factKindMatchesCategory struct{}

func (*factKindMatchesCategory) meta() meta {
	return meta{id: "fact-kind-matches-category", tier: 1,
		description: "every fact's kind is one the category it carries declares in data/taxonomy.yaml"}
}

func (*factKindMatchesCategory) Run(_ context.Context, s *Subject) (Result, error) {
	var findings []Finding
	subjects := 0
	pairs := map[string]bool{}

	for _, f := range s.Facts {
		if f.Kind == "" || f.Category == "" {
			continue
		}
		c, ok := s.Vocabulary.Category(f.Category)
		if !ok || !c.Assignable {
			continue
		}
		subjects++
		pairs[string(f.Kind)+" "+f.Category] = true
		if !declaresKind(c, f.Kind) {
			findings = append(findings, finding(f.ID,
				"%s p%d %q: kind %q is not one data/taxonomy.yaml declares for category %q, "+
					"which declares %s", f.DocID, f.Page, f.RowLabel, f.Kind, f.Category,
				strings.Join(c.Kinds, ", ")))
		}
	}

	return conclusion{
		subjects: subjects,
		unit:     "facts",
		held: fmt.Sprintf("%d facts over %d kind/category pairs, each kind one its category "+
			"declares in data/taxonomy.yaml", subjects, len(pairs)),
		// "assignable", since the skip above also passes over a rollup.
		nothing: "no fact carries both a kind and an assignable category " +
			"data/taxonomy.yaml defines",
		findings: findings,
	}.result(), nil
}

// declaresKind reports whether c permits a fact of kind k, literally: no
// family is resolved and no match widened.
func declaresKind(c registry.Category, k vocab.Kind) bool {
	return slices.Contains(c.Kinds, string(k))
}

// factDepartmentsResolve asserts the department axis is a controlled vocabulary,
// against data/departments.yaml:
//
//   - the slug names a division or a department it lists (which tier a
//     schedule needs is its cut's DepartmentTier, held by
//     cuts-tie-along-the-lattice);
//   - the slug is well formed, reported as a shape problem;
//   - the same department is spelled one way across the store;
//   - no department collides with a data/taxonomy.yaml category slug.
//
// The last two hold against the [Vocabulary] interface, not one registry.
type factDepartmentsResolve struct{}

func (*factDepartmentsResolve) meta() meta {
	return meta{id: "fact-departments-resolve", tier: 1,
		description: "every department a fact carries is a division or a department " +
			"data/departments.yaml lists: a well-formed slug, spelled one way, that no " +
			"category slug collides with"}
}

func (*factDepartmentsResolve) Run(_ context.Context, s *Subject) (Result, error) {
	var findings []Finding
	subjects := 0
	spellings := map[string][]string{} // normalized -> spellings as written
	departments := map[string]bool{}

	for _, f := range s.Facts {
		if f.Department == "" {
			continue
		}
		subjects++
		departments[f.Department] = true
		key := strings.ToLower(strings.NewReplacer(" ", "", "_", "", "-", "").Replace(f.Department))
		if !slices.Contains(spellings[key], f.Department) {
			spellings[key] = append(spellings[key], f.Department)
		}
		if _, isCategory := s.Vocabulary.Category(f.Department); isCategory {
			findings = append(findings, finding(f.ID,
				"%s p%d %q: department %q is also a data/taxonomy.yaml category slug; the two "+
					"axes must not share a string", f.DocID, f.Page, f.RowLabel, f.Department))
		}
		// Shape OR resolution, never both. A malformed slug is not in the
		// registry either — departments.yaml applies the same rule when it
		// loads — so reporting both would put two findings and one fix against
		// one fact, and the shape is the more specific diagnosis.
		if !registry.ValidSlug(f.Department) {
			findings = append(findings, finding(f.ID,
				"%s p%d %q: department %q is not a slug: lower case, digits and single hyphens, "+
					"one segment", f.DocID, f.Page, f.RowLabel, f.Department))
			continue
		}
		if _, isDivision := s.Vocabulary.Division(f.Department); !isDivision &&
			!s.Vocabulary.Department(f.Department) {
			findings = append(findings, finding(f.ID,
				"%s p%d %q: department %q is neither a division nor a department %s lists, "+
					"so the fact joins to nothing",
				f.DocID, f.Page, f.RowLabel, f.Department, departmentsFile))
		}
	}

	// Keyed order, not map order: two mis-spelled departments would otherwise
	// swap places between runs, and a report whose findings reorder cannot be
	// diffed across releases.
	for _, key := range slices.Sorted(maps.Keys(spellings)) {
		written := spellings[key]
		if len(written) > 1 {
			slices.Sort(written)
			findings = append(findings, finding(factsFile,
				"one department is spelled %d ways: %s", len(written), strings.Join(written, ", ")))
		}
	}

	return conclusion{
		subjects: subjects,
		unit:     "departments",
		held: fmt.Sprintf("%d facts name one of %d slugs, each a division or a department "+
			"%s lists", subjects, len(departments), departmentsFile),
		nothing: "no fact carries a department: the citywide spine crosses category against " +
			"fund group and has no department axis (pp.167-170 are fisc-5gk.2)",
		findings: findings,
	}.result(), nil
}

// factFundsResolve asserts every fund number a fact carries is a fund
// data/funds.yaml lists.
//
// A fact with no fund (null) is not a subject. A fact carrying 0 is, and is
// refused before the registry is asked: no fund is numbered 0, and 0 as "no
// fund" would put a fund-group total and a printed dash at one address.
type factFundsResolve struct{}

func (*factFundsResolve) meta() meta {
	return meta{id: "fact-funds-resolve", tier: 1,
		description: "every fund number a fact carries is a fund data/funds.yaml lists, with the fund " +
			"group the fact claims"}
}

// Run also checks the fund against the fact's fund group, which is the failure a
// number alone cannot show: fund 100 existing says nothing about a fact filing it
// under `enterprise`, and the group is what the projection draws.
func (*factFundsResolve) Run(_ context.Context, s *Subject) (Result, error) {
	var findings []Finding
	subjects := 0

	for _, f := range s.Facts {
		if f.Fund == nil {
			continue
		}
		subjects++
		fund := *f.Fund
		if fund == 0 {
			findings = append(findings, finding(f.ID,
				"%s p%d %q: fund 0 names no fund; an absent fund is null",
				f.DocID, f.Page, f.RowLabel))
			continue
		}
		entry, ok := s.Vocabulary.Fund(fund)
		if !ok {
			findings = append(findings, finding(f.ID,
				"%s p%d %q: fund %d is not in data/funds.yaml, so the fact joins to nothing",
				f.DocID, f.Page, f.RowLabel, fund))
			continue
		}
		if f.FundGroup != "" && entry.Type != f.FundGroup {
			findings = append(findings, finding(f.ID,
				"%s p%d %q: fund %d (%s) is type %q in data/funds.yaml but the fact carries "+
					"fund group %q", f.DocID, f.Page, f.RowLabel, fund, entry.Name,
				entry.Type, f.FundGroup))
		}
	}
	return conclusion{
		subjects: subjects,
		unit:     "fund numbers",
		held: fmt.Sprintf("%d facts name a fund, each listed in data/funds.yaml under the group "+
			"the fact claims", subjects),
		nothing: "no fact names a fund: the citywide spine's columns are fund groups, and " +
			"pp.66-67 print no per-fund column (fisc-5gk.1)",
		findings: findings,
	}.result(), nil
}
