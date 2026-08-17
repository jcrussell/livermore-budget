package check

import (
	"context"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
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
// id. That id is what a future data/reconciliations.yaml names.
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

// departmentSlug is the shape a department identifier must have: the same
// lowercase, single-segment kebab-case data/taxonomy.yaml's slug rule produces.
// It is one level deep because a department is not a hierarchy — pp.165-166 print
// a flat list — and because the axis-crossing this project is most likely to
// commit is a department slug that looks like a category one.
var departmentSlug = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// factDepartmentsResolve asserts the department axis is a controlled vocabulary.
//
// It cannot fully do that yet, and says so rather than passing. data/taxonomy.yaml
// is explicit that departments are deliberately absent from it — a fact is
// simultaneously "Police" and "Wages & Benefits", two orthogonal axes, and folding
// one into the other would make department rows and category rows sum into one
// total — and that they belong in a registry of their own. That registry does not
// exist. So while no fact carries a department this check is vacuous, and when one
// does it reports what IS assertable without a registry and then refuses to call
// the run a pass:
//
//   - the slug is well formed, so `Police` and `police_dept` are caught;
//   - the same department is spelled one way across the whole store;
//   - no department collides with a data/taxonomy.yaml category slug, which is the
//     near miss the taxonomy warns about.
//
// The verdict is then an ERROR, not a failure: the corpus may be perfectly
// correct, and what is missing is a file nobody has written. pp.167-170
// (fisc-5gk.2) is in this project's scope, so this arrives with the mapping rather
// than never, and an error is what makes the registry land in the same change.
type factDepartmentsResolve struct{}

var _ Check = (*factDepartmentsResolve)(nil)

func (*factDepartmentsResolve) ID() string { return "fact-departments-resolve" }
func (*factDepartmentsResolve) Tier() int  { return 1 }
func (*factDepartmentsResolve) Full() bool { return false }
func (*factDepartmentsResolve) Description() string {
	return "every department a fact carries is a well-formed slug, spelled one way, that no " +
		"category slug collides with — and resolves in a department registry, which data/ has " +
		"none of yet"
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
		if !departmentSlug.MatchString(f.Department) {
			findings = append(findings, finding(f.ID,
				"%s p%d %q: department %q is not a slug: lower case, digits and single hyphens, "+
					"one segment", f.DocID, f.Page, f.RowLabel, f.Department))
		}
		key := strings.ToLower(strings.NewReplacer(" ", "", "_", "", "-", "").Replace(f.Department))
		if !slices.Contains(spellings[key], f.Department) {
			spellings[key] = append(spellings[key], f.Department)
		}
		if _, isCategory := s.Vocabulary.Category(f.Department); isCategory {
			findings = append(findings, finding(f.ID,
				"%s p%d %q: department %q is also a data/taxonomy.yaml category slug; the two "+
					"axes must not share a string", f.DocID, f.Page, f.RowLabel, f.Department))
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

	if subjects == 0 {
		return Result{
			Status: StatusVacuous,
			Summary: "no fact carries a department, and data/ has no department registry to " +
				"resolve one against (pp.167-170 are unmapped: fisc-5gk.2)",
			Findings: []Finding{},
		}, nil
	}
	if len(findings) > 0 {
		return conclusion{
			subjects: subjects,
			unit:     "departments",
			findings: findings,
		}.result(), nil
	}
	// Well formed, consistent, and not colliding — and still not resolved, because
	// there is nothing to resolve against. Reporting a pass here would say the
	// department axis is a controlled vocabulary when it is a set of free strings
	// this check happens to like the shape of.
	names := slices.Sorted(maps.Keys(departments))
	return Result{}, fmt.Errorf("%d facts carry a department (%s) and every one is a well-formed "+
		"slug that no category collides with, but data/ has no department registry, so "+
		"\"resolves\" cannot be established; a department registry has to land with the "+
		"pp.167-170 mapping (fisc-5gk.2)", subjects, strings.Join(names, ", "))
}

// factFundsResolve asserts every fund number a fact carries is a fund
// data/funds.yaml lists.
//
// Every fact carries fund 0 today: the citywide spine's columns are fund GROUPS,
// and pp.66-67 print no per-fund column. So this is vacuous, and vacuous is the
// honest report rather than absence — registry.Fund exists, nothing called it
// before this check, and fisc-5gk.1 starts emitting facts with a real fund number,
// at which point the join key every fact carries has to resolve or the fact joins
// to nothing.
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
		if f.Fund == 0 {
			continue
		}
		subjects++
		entry, ok := s.Vocabulary.Fund(f.Fund)
		if !ok {
			findings = append(findings, finding(f.ID,
				"%s p%d %q: fund %d is not in data/funds.yaml, so the fact joins to nothing",
				f.DocID, f.Page, f.RowLabel, f.Fund))
			continue
		}
		if f.FundGroup != "" && entry.Type != f.FundGroup {
			findings = append(findings, finding(f.ID,
				"%s p%d %q: fund %d (%s) is type %q in data/funds.yaml but the fact carries "+
					"fund group %q", f.DocID, f.Page, f.RowLabel, f.Fund, entry.Name,
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
