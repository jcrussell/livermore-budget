package check

import (
	"context"
	"fmt"

	"github.com/jcrussell/livermore-budget/internal/mapping"
	"github.com/jcrussell/livermore-budget/internal/project"
)

// unprojectedScopes are the scopes deliberately not drawn by any projection, each
// with the reason it is not.
//
// An entry is a declaration rather than a note: it says a human decided those
// facts belong in the store and not in a chart. Without the declaration, a fact no
// projection covers is a failure — because the alternative is what this map exists
// to stop. Moving twenty facts to a mistyped `all-funds-gross-detail` took over
// half the city's revenue out of the published chart, and every check still
// passed: the graph checks only ever see the facts a projection selected, so facts
// that fall outside every slice are not checked, they are ignored.
//
// A scope listed here is still checked by everything above the projection line —
// sorted, unique ids, token re-parse, offsets, vocabulary — because those read the
// fact store directly. For the entry below that is not a consolation but most of
// the coverage: expenditure-detail-ties-to-spine ties the 98 facts pp.66-67 print
// a column for, cell by cell, to spine facts that ARE graph-checked, so the
// declaration hands them to a stronger check rather than excusing them from one.
// The other 98 — the FY2024 actual and FY2025 revised columns — reconcile against
// the schedule's OWN printed totals at build time and against nothing on the
// spine, because the spine prints no such column. fisc-brx's reason text said
// "all 196"; it is corrected here rather than repeated, because verify prints
// this string verbatim.
//
// AN ENTRY THAT HAS STOPPED BEING TRUE MUST GO RED, NOT QUIET. factsAreProjected
// consults this map only for UNPROJECTED facts, so the moment a projection starts
// drawing a declared scope the entry goes silent while remaining a false claim
// about the corpus. staleDeclarations is the branch that refuses that, and it is
// what retires an entry automatically instead of leaving an exemption for whoever
// forgets.
var unprojectedScopes = map[string]string{
	expenditureDetailScope: "Budget Book pp.167-170, General Fund Expenditures by Major " +
		"Category: the department x object decomposition of p66's General Fund expenditure " +
		"block, not additional money. Its 49 object rows total 144,650,802 in FY2025-26 and " +
		"149,014,579 in FY2026-27 -- p66's TOTAL EXPENDITURES to the cent -- so drawing them " +
		"into the fund-group spine doubles General Fund spending. " +
		"expenditure-detail-ties-to-spine reconciles the 98 of its 196 facts that the spine " +
		"prints a column for -- the two adopted years -- against spine facts that ARE " +
		"graph-checked; pp.66-67 print no actual or revised column, so the FY2024 and FY2025 " +
		"halves are published with nothing to tie to. A department tier in the graph is " +
		"fisc-gxa.2 / fisc-oxf.",
	revenueDetailScope: "Budget Book pp.127-140, Revenue Sources by Fund: the line-item and " +
		"per-fund decomposition of pp.66-67's REVENUE rows, not additional money. Its 231 " +
		"rows reproduce all six fund groups' TOTAL REVENUES exactly in both budget years " +
		"-- general 157,873,470 / 164,358,147 and the other five to the cent besides -- so " +
		"drawing them into the fund-group spine doubles the city's revenue, and unlike the " +
		"department schedule nothing else would stop it: netCells refuses a fact carrying " +
		"a department and has no such refusal for a fund. revenue-detail-ties-to-spine " +
		"reconciles the 462 of its 924 facts the spine prints a column for -- the two " +
		"adopted years -- against spine facts that ARE graph-checked. It is NOT the whole " +
		"of the spine's inflow: pp.127-130 print no General Fund Transfers In row, so " +
		"480,400 in FY2026 and 486,735 in FY2027 are on the spine with no counterpart " +
		"here, declared as that check's one exception and owed to transfers-by-fund " +
		"(fisc-5gk.3.1), which prints them. And pp.66-67 print no actual or revised " +
		"column, so this schedule's FY2024 and FY2025 halves tie to its own 79 printed " +
		"totals at build time and to nothing on the spine. A fund tier in the graph is " +
		"fisc-gxa.2 / fisc-oxf.",
	transfersDetailScope: "Budget Book p76, Summary of Transfers: the per-fund decomposition " +
		"of pp.66-67's TRANSFER IN and TRANSFER OUT rows, not additional money. Its 22 " +
		"printed rows sum, per receiving fund group, to those pages' TRANSFER IN cells " +
		"exactly in both budget years -- general 480,400 / 486,735, enterprise 13,247,000 / " +
		"13,330,000, debt-service 6,984,597 / 6,969,898, special-revenue 814,000 / 838,000 " +
		"-- so drawing them into the fund-group spine doubles the city's transfers. Each " +
		"row publishes TWO facts from one printed figure, the receiving leg and the paying " +
		"one, so 88 in all. WHAT RECONCILES THEM IS NOT UNIFORM AND THE DIFFERENCE MATTERS: " +
		"the IN side ties to the spine exactly, cell for cell, with no constant and no " +
		"exception. The OUT side does not and cannot -- pp.66-67's TRANSFER OUT includes " +
		"transfers to the CIP, which p76 does not list -- so it ties only after adding a " +
		"figure read off pp.72-75, which are neither mapped nor fixtures and are therefore " +
		"hand-typed into the check. And the permanent in-leg is a published zero reconciled " +
		"against an absent spine column, because pp.66-67 print no Permanent group at all " +
		"(fisc-u8o); it is covered by no arithmetic here. The two historical columns are " +
		"not published: they miss p76's own printed grand total by 6,858,051 and by exactly " +
		"5,000,000, and the spine prints no actual or revised column to tie them to. " +
		"WHAT IS STILL UNGUARDED, because the check above cannot see it: the payer at " +
		"each row's far end is hand-typed, the page cannot check it (a counterpart is " +
		"resolved downstream of every comparison against the city's own arithmetic), " +
		"and rule-funds-match-their-headings reads column funds only, so it does not " +
		"reach these rows at all. Five of p76's payer labels match an operating fund " +
		"AND its CIP twin, and every twin is type: capital, so a leg under the wrong " +
		"twin moves inside the collapsed non-major cell and ties anyway. That is " +
		"fisc-bhe. A fund tier in the graph is fisc-gxa.2 / fisc-oxf; leg-level links " +
		"carrying a transfer_id are fisc-9gh.",
}

// projectionsBuild asserts every slice of the fact store that a projection was
// asked for actually produced a graph.
//
// THIS CHECK EXISTS BECAUSE ITS ABSENCE WAS SILENT IN BOTH DIRECTIONS. A
// projection that refuses to build used to abort check.Load, so `fisc verify`
// exited 1 having printed no report — every other finding in the run lost, and
// the reason wrapped inside a load error. Recording the refusal instead fixes
// that, and opens the opposite hole: a slice missing from Subject.Projections is
// not a failure anywhere, it is silence, and ten checks below read nothing but
// the graph. So this one must FAIL on a recorded refusal and must never be
// vacuous while one is recorded.
//
// The refusal it exists for today is internal/project's: netCells rejects a fact
// carrying a department, because the citywide spine crosses category against
// fund group and has no department tier (fisc-gxa.2). A correctly scoped
// pp.167-170 fact never reaches it — selectFacts filters on scope first — so on
// a healthy corpus this is vacuous, and what makes it reachable is a rule whose
// `scope:` says all-funds-gross when it means expenditure-by-department.
type projectionsBuild struct{}

var _ Check = (*projectionsBuild)(nil)

func (*projectionsBuild) ID() string { return "projections-build" }
func (*projectionsBuild) Tier() int  { return 1 }
func (*projectionsBuild) Full() bool { return false }
func (*projectionsBuild) Description() string {
	return "every projection the fact store's slices asked for produced a graph, rather than " +
		"refusing and taking the whole report down with it"
}

// Run counts one subject per slice a projection was asked for — the built ones
// and the failed ones together — because the claim is about all of them and a
// count of only the failures would read as "1 of 1" on a corpus where nine
// slices built and one did not.
func (*projectionsBuild) Run(_ context.Context, s *Subject) (Result, error) {
	findings := make([]Finding, 0, len(s.ProjectionFailures))
	for _, f := range s.ProjectionFailures {
		findings = append(findings, finding(f.String(),
			"this projection refused to build: %v. Every check that reads a graph is "+
				"silent about this slice — they are not failing it, they cannot see it",
			f.Err))
	}
	return conclusion{
		subjects: len(s.Projections) + len(s.ProjectionFailures),
		unit:     "projection slices",
		held: fmt.Sprintf("%d projection slices built, none refused",
			len(s.Projections)),
		nothing:  "no projection was asked for at all",
		findings: findings,
	}.result(), nil
}

// sliceKey identifies one projection slice. It is the triple selectFacts
// filters on (internal/project), so a fact and the projection that would have
// drawn it are compared on the same three fields rather than on two of them.
type sliceKey struct {
	year  int
	basis mapping.Basis
	scope string
}

func keyOf(o project.Options) sliceKey {
	return sliceKey{o.FiscalYear, o.Basis, o.Scope}
}

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
	// A refused projection makes every fact of ITS slice unprojected, and each
	// one would otherwise arrive below as its own finding: on the committed
	// corpus that is 120 findings restating one cause. Those facts are collected
	// against the refusal and reported once.
	//
	// Only that slice's facts, though. Suppressing the whole check while any
	// projection failed would hide a genuinely undeclared scope elsewhere in the
	// store until the refusal was fixed and verify re-run — one real finding
	// costing two rounds, which is the thing a report exists not to do.
	refused := map[sliceKey]int{}
	for _, f := range s.ProjectionFailures {
		refused[keyOf(f.Options)] = 0
	}

	var findings []Finding
	declared := map[string]int{}
	for _, f := range s.Facts {
		if projected[f.ID] {
			continue
		}
		k := sliceKey{f.FiscalYear, f.Basis, f.Scope}
		if _, isRefused := refused[k]; isRefused {
			refused[k]++
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
	for _, f := range s.ProjectionFailures {
		if n := refused[keyOf(f.Options)]; n > 0 {
			findings = append(findings, finding(f.String(),
				"%d facts in this slice are unprojected because the projection refused to "+
					"build; see projections-build. Whether their scopes are declared cannot "+
					"be established until it does", n))
		}
	}

	findings = append(findings, staleDeclarations(s, declared)...)

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

// staleDeclarations reports every unprojectedScopes entry that exempted nothing.
//
// The entry is a claim about the corpus — "these facts exist and no projection
// draws them" — and it can stop being true in two ways that need different
// fixes, so it says which:
//
//   - the scope carries facts and something now PROJECTS them. The exemption is
//     doing nothing and must be deleted; leaving it means the next reader
//     believes a schedule is out of the chart when it is in it. This is the arm
//     fisc-u2v (4) asks for, and it is what retires an entry automatically
//     instead of leaving a 196-fact exemption for whoever forgets.
//
//     IT CANNOT FIRE THROUGH check.Load TODAY, and saying so is the difference
//     between a guard and a promise: buildProjections builds slices in
//     spineScope only, so no declared scope can become projected until it learns
//     to build another (fisc-gxa.2). The arm is written now because that change
//     is where the entry silently stops being true, and a branch added at the
//     same time as the thing it guards is a branch nobody has to remember.
//
//   - no rule writes the scope and no fact carries it. Either the rules were
//     removed and the entry outlived them, or this string and the one in
//     mappings/ have drifted apart — which is the mistyped-scope incident this
//     map exists for, wearing its other face: the facts would be unprojected AND
//     undeclared, and this entry is not the declaration anyone thinks it is.
//
// THE SECOND ARM ASKS THE RULE FILES, not the fact store, and that is the
// difference between "this schedule is not mapped here" and "this declaration is
// wrong". A Subject with no rule files — a hand-built fixture, a miniature of
// one schedule — is not a repository in which a declaration for another schedule
// has gone stale, and reporting one would make every fixture carry every
// schedule to stay green.
//
// Same direction as fisc-2sd's third failure mode, where a declared per-part
// delta that now ties exactly must fail rather than pass quietly. A declaration
// nobody can see expiring is a declaration that outlives its reason.
func staleDeclarations(s *Subject, declared map[string]int) []Finding {
	written := map[string]bool{}
	for _, f := range s.Files {
		for i := range f.Rules {
			written[f.Rules[i].Scope] = true
		}
	}

	var out []Finding
	for _, scope := range sortedStrings(unprojectedScopes) {
		if declared[scope] > 0 {
			continue
		}
		carried := 0
		for _, f := range s.Facts {
			if f.Scope == scope {
				carried++
			}
		}
		if carried > 0 {
			out = append(out, finding(scope,
				"unprojectedScopes declares this scope unprojected, but all %d of its facts "+
					"are in some projection's slice; the declaration is exempting nothing "+
					"and must be removed", carried))
			continue
		}
		if len(s.Files) > 0 && !written[scope] {
			out = append(out, finding(scope,
				"unprojectedScopes declares this scope unprojected, and no rule in %s writes "+
					"it and no fact carries it; the rules that wrote it are gone, or this "+
					"string and the one they write have drifted apart", mappingsDir))
		}
	}
	return out
}
