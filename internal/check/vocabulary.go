package check

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/jcrussell/livermore-budget/internal/registry"
	"github.com/jcrussell/livermore-budget/internal/vocab"
)

// factVocabulary asserts every category and fund group a fact carries resolves in
// the curated registries.
//
// This is the check data/taxonomy.yaml and data/funds.yaml exist for. Until
// something reads them a rule's `category:` and a column's `fund_group:` are free
// strings, and a rule that writes `tax/property` where the taxonomy says
// `taxes/property` produces a fact that is confidently wrong and joins to nothing.
//
// Departments and fund numbers are two more axes a fact can carry and they are
// NOT here: each has its own check, because each is answered by a different file —
// or, for departments, by no file yet — and one check reporting a single verdict
// over all four would mean a real category typo and an unbuilt registry sharing an
// id. Each is separately greppable in the report, and separately declarable in
// vacuity.go, which is what having its own id buys.
type factVocabulary struct{}

var _ Check = (*factVocabulary)(nil)

func (*factVocabulary) ID() string { return "fact-vocabulary" }
func (*factVocabulary) Tier() int  { return 1 }
func (*factVocabulary) Full() bool { return false }
func (*factVocabulary) Description() string {
	return "every category a fact carries is assignable in data/taxonomy.yaml, and every fund " +
		"group is a type data/funds.yaml uses"
}

// Run counts one subject per classification it resolves rather than one per fact,
// because the two axes are two claims and a fact carries one or both.
//
// An empty category is not a finding. A fact with no classification at all cannot
// be projected — internal/project refuses one — but this check is about the values
// that ARE asserted resolving, and an absent value is the mapping's business.
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

// unassignable says why a category may not be written, which is two different
// problems with two different fixes. Registry.Assignable's doc comment draws the
// distinction and this is the caller it draws it for: a rollup exists so a view
// can name the group, and a rule may never assign one; a slug the taxonomy does
// not define at all is a typo or a category nobody has curated yet.
func unassignable(v Vocabulary, slug string) string {
	if c, ok := v.Category(slug); ok {
		return fmt.Sprintf("category %q is the rollup %q, which data/taxonomy.yaml declares "+
			"assignable: false — it exists so a view can group, and a rule may not assign it",
			slug, c.Label)
	}
	return fmt.Sprintf("category %q is not defined in data/taxonomy.yaml", slug)
}

// factKindMatchesCategory asserts every fact's kind is one its category declares.
//
// data/taxonomy.yaml carries a `kinds:` list on every category and registry.Load
// requires it to be non-empty, and until this check nothing read it. So the two
// halves of a fact's classification were each valid alone and never valid
// TOGETHER:
//
//	category: transfers/in  +  kind: revenue
//
// resolves clean against factVocabulary, which asks only whether the slug is
// assignable.
//
// That pair is not hypothetical. Budget Book pp.131-140 print eleven funds whose
// single `Total <fund>` covers revenue rows AND a Transfers In row — p131's
// Stormwater is Charges for Services 1,169,000 + Transfers In 3,247,000 =
// 4,416,000, and the taxonomy's `transfers` entry records the trap in as many
// words. A rule author reaching for that printed total as a total_row, without a
// per-row kind, makes the Transfers In row revenue to make the fund tie. Enterprise
// revenue then reads 80,761,261 against the 67,514,261 pp.66-67 print (fisc-u2v,
// failure route 3).
//
// A fund-group reconciliation would catch that too, and later. This one fails at
// the fact, naming the row and the two strings that disagree, which is the
// difference between a finding an author can act on and an aggregate they have to
// bisect.
//
// DIRECTION is caught here too, by the same literal comparison and not by a second
// check: transfers/in declares transfer_in and transfers/out declares
// transfer_out, so a transfer filed under the wrong one carries a kind its
// category does not declare. That was fisc-ttq, and it was open only for as long
// as all four transfer categories declared a `transfer` family — a string that is
// not one of the five vocab.Kind values and never was, which is why correcting
// the file closed the hole rather than widening the check.
//
// A category factVocabulary ALREADY NAMES is not counted here, and that is two
// cases rather than one. A category the taxonomy does not define has no kinds
// to be among; a category it defines as `assignable: false` is a rollup no rule
// may write, which factVocabulary reports precisely, with the fix. Both are
// skipped so one typo does not redden two checks of this family with two
// different fixes.
//
// SCOPED TO THIS FAMILY DELIBERATELY. It is not a claim that a typo reddens
// exactly one check overall -- `category: taxes` on an expenditure fact also
// reddens cuts-tie-along-the-lattice, because money really has left the
// category the spine expects. That is a different fact about the corpus, not a
// duplicate report of this one.
//
// The unassignable arm was tested for `!ok` alone until 2026-08-29, so
// `category: taxes` + `kind: expenditure` reddened BOTH vocabulary checks --
// the exact case this comment said it avoided (fisc-9nw part 3). The condition
// was wrong, not the comment.
type factKindMatchesCategory struct{}

var _ Check = (*factKindMatchesCategory)(nil)

func (*factKindMatchesCategory) ID() string { return "fact-kind-matches-category" }
func (*factKindMatchesCategory) Tier() int  { return 1 }
func (*factKindMatchesCategory) Full() bool { return false }
func (*factKindMatchesCategory) Description() string {
	return "every fact's kind is one the category it carries declares in data/taxonomy.yaml"
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
		// "assignable" and not merely "defines": the skip above now also
		// passes over a rollup, so a store of nothing but `taxes` facts would
		// otherwise be told the taxonomy defines no such category when it
		// defines it precisely and forbids writing it.
		nothing: "no fact carries both a kind and an assignable category " +
			"data/taxonomy.yaml defines",
		findings: findings,
	}.result(), nil
}

// declaresKind reports whether c permits a fact of kind k.
//
// The comparison is literal: the `kinds:` list holds vocab.Kind values and a
// fact's kind is one, so nothing here resolves a family or widens a match. A
// category that means to admit two kinds says both — the `transfers` rollup
// declares transfer_in and transfer_out — because a list that had to be
// interpreted could not be read as the file's own answer.
func declaresKind(c registry.Category, k vocab.Kind) bool {
	return slices.Contains(c.Kinds, string(k))
}

// factDepartmentsResolve asserts the department axis is a controlled vocabulary.
//
// It can now do that. data/taxonomy.yaml is explicit that departments are
// deliberately absent from it — a fact is simultaneously "Police" and "Wages &
// Benefits", two orthogonal axes, and folding one into the other would make
// department rows and category rows sum into one total — and that they belong in
// a registry of their own. data/departments.yaml is that registry, and this check
// is what reads it. Four claims, and the first is the one the other three used to
// stand in for:
//
//   - the slug names a division OR a department data/departments.yaml lists, so
//     a fact whose `department` joins to nothing cannot be published;
//   - the slug is well formed, so `Police` and `police_dept` are caught, and are
//     reported as the shape problem they are rather than as a missing entry;
//   - the same department is spelled one way across the whole store;
//   - no department collides with a data/taxonomy.yaml category slug, which is
//     the near miss the taxonomy warns about.
//
// THE FIRST CLAIM SPANS TWO TIERS, and this check cannot tell which a schedule
// needs: that is its cut's DepartmentTier, held by cuts-tie-along-the-lattice.
//
// THE LAST TWO ARE NOW BELT AND BRACES, and they stay. The registry refuses a
// slug that is a category slug on both tiers, so a resolving department cannot
// collide — but this check is written against the [Vocabulary] interface rather
// than against that one implementation, and an assertion that is currently
// implied by a loader is not the same thing as one nobody makes.
//
// Until this bead (fisc-o15) the check ERRORED as soon as any fact carried a
// department, because "resolves" could not be established against a file that did
// not exist. That was the honest interim, and it was deliberately loud: an error
// makes Report.Failed() true without --strict, so the registry had to land in the
// same change as the first department-bearing fact. It has.
type factDepartmentsResolve struct{}

var _ Check = (*factDepartmentsResolve)(nil)

func (*factDepartmentsResolve) ID() string { return "fact-departments-resolve" }
func (*factDepartmentsResolve) Tier() int  { return 1 }
func (*factDepartmentsResolve) Full() bool { return false }
func (*factDepartmentsResolve) Description() string {
	return "every department a fact carries is a division or a department " +
		"data/departments.yaml lists: a well-formed slug, spelled one way, that no " +
		"category slug collides with"
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
	for _, key := range sortedStrings(spellings) {
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

var _ Check = (*factFundsResolve)(nil)

func (*factFundsResolve) ID() string { return "fact-funds-resolve" }
func (*factFundsResolve) Tier() int  { return 1 }
func (*factFundsResolve) Full() bool { return false }
func (*factFundsResolve) Description() string {
	return "every fund number a fact carries is a fund data/funds.yaml lists, with the fund " +
		"group the fact claims"
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
