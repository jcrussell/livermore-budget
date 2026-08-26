package check

import (
	"context"
	"fmt"
	"sort"

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
//
// IT HAS RETIRED ONE ENTRY ALREADY, WHICH IS THE MECHANISM WORKING RATHER THAN A
// LOSS. revenue-by-fund (Budget Book pp.127-140) was declared here with a
// paragraph explaining that drawing its 924 facts into the fund-group spine would
// double the city's revenue. That is still true of the SPINE, and it is no longer
// a reason nothing draws them: the revenue-trends document draws all 924, at their
// own scope, as 231 four-point series. staleDeclarations went red demanding the
// entry be deleted and the entry was deleted. What did NOT change is the
// coverage: revenue-detail-ties-to-spine reads the fact store directly through
// detailSums and never consulted this map, so it still reconciles the same 134
// cells against the spine. Retiring a declaration here costs nothing, which is
// what makes the automatic retirement safe.
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
		"THE PAYER AT EACH ROW'S FAR END IS NOT COVERED BY THE ARITHMETIC AT ALL, and " +
		"is checked separately: it is hand-typed 44 times, the page cannot check it (a " +
		"counterpart is resolved downstream of every comparison against the city's own " +
		"totals), and five of p76's payer labels match an operating fund AND its CIP " +
		"twin -- every twin type: capital, so a leg under the wrong twin moves inside " +
		"the collapsed non-major cell and ties anyway. row-funds-match-their-anchors " +
		"resolves 40 printed row anchors against data/funds.yaml instead. Three " +
		"declared payers it cannot reach: p76 prints three continuation rows whose " +
		"\"Transfer From\" carries over from the row above, so they have no printed " +
		"anchor on their own line. A fund tier in the graph is fisc-gxa.2 / fisc-oxf; " +
		"leg-level links carrying a transfer_id are fisc-9gh.",
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

// keysOf is every slice one Options covers, which is one per column.
//
// IT RETURNS A LIST BECAUSE ONE DOCUMENT NEED NOT BE ONE SLICE. While every
// projection was of a single (year, basis) this was a one-to-one map and was
// spelled as one; a trends document is of four columns of one schedule, so an
// Options that maps to a single key can no longer be assumed. Anything keying a
// map on a projection has to iterate this rather than call it once.
func keysOf(o project.Options) []sliceKey {
	out := make([]sliceKey, 0, len(o.Columns))
	for _, c := range o.Columns {
		out = append(out, sliceKey{c.FiscalYear, c.Basis, o.Scope})
	}
	return out
}

// publishedProjectionBuilt asserts EVERY document the site publishes was built,
// over every column it publishes it over.
//
// Every graph check reads Subject.Projections, so a projection that was not built
// is not a failure anywhere: it is silence. If the fact store no longer carries
// facts for a published slice, export publishes a chart of nothing while verify
// reports on whatever other slices happen to exist.
//
// IT ITERATES THE PUBLISHED DOCUMENTS RATHER THAN PINNING ONE. It began pinned to
// a single triple, which could not see FY2027; it then iterated the published
// YEARS, which could not see a second DOCUMENT. revenue-trends is that document:
// its own scope, four columns, no year at all, published since 2026-08-25 and
// guarded by nothing until this check learned to read
// project.PublishedDocuments. A check that covers less than the site publishes is
// worse than one that covers nothing, because it reports a number that looks like
// coverage.
//
// WHAT THE OLD SHAPE LET THROUGH, and why the columns are checked one at a time.
// Trends.Slices returns nil when the store carries no revenue-by-fund fact -- a
// scope typo in a mapping rule does it -- and then: projections-build counts what
// was ASKED FOR and sees nothing missing; documents-are-checked passes over what
// remains; trend-points-tie-to-facts and trend-series-are-complete both go
// VACUOUS, which fails only under --strict and is silenced outright by adding a
// declaration; and facts-are-projected reddens with 924 findings ABOUT FACTS
// rather than one about a missing document. Per-column is the same argument one
// level down: a document that lost FY2023-24 builds fine, every series is
// complete over the three columns that remain, and trend-series-are-complete
// passes green over 231 subjects while counts.facts falls from 924 to 693
// (fisc-7dt).
//
// This is the check that closes the divergence a shared declaration cannot: the
// two commands reading one list says nothing about whether the facts are there.
type publishedProjectionBuilt struct{}

var _ Check = (*publishedProjectionBuilt)(nil)

func (*publishedProjectionBuilt) ID() string { return "published-projection-built" }
func (*publishedProjectionBuilt) Tier() int  { return 1 }
func (*publishedProjectionBuilt) Full() bool { return false }
func (*publishedProjectionBuilt) Description() string {
	return "every document `fisc export` publishes was built, over every column it " +
		"publishes it over, and is among the projections these checks were run over"
}

// Run has one subject per published document.
//
// It used to have exactly one and to say so — "there is no state of the corpus
// in which this check has nothing to look at, which is why it is not routed
// through conclusion". Both halves stopped being true when the published set
// became a list: a repository publishing nothing has nothing here to look at,
// and that state is what the nothing: string below is for.
func (*publishedProjectionBuilt) Run(_ context.Context, s *Subject) (Result, error) {
	// Every column built, grouped by the projection that built it. A published
	// document is satisfied by the UNION of what that projection built, not by
	// any single slice of it: the spine publishes one document per year and
	// builds one Options per year, while the trends publish one document over
	// four columns built as one Options, and this check must not care which
	// shape it is looking at.
	//
	// It is keyed on the projection NAME, and that is not belt-and-braces.
	// Since each projection is built over the slices it declares
	// (project.Sliced), "some projection was built at the published triple"
	// does not imply the published DOCUMENT was: a second projection whose
	// slices happen to include that triple would satisfy this check while the
	// site's chart was of nothing.
	// Keyed on (name, scope) and not on the name alone, so that a projection
	// which one day builds two schedules cannot have the columns of one satisfy
	// a published document of the other. Nothing does that today; the key costs
	// nothing and the alternative is a silent wrong answer rather than a
	// refusal.
	type source struct{ name, scope string }
	built := map[source]project.Options{}
	names := make([]string, 0, len(s.Projections))
	for _, p := range s.Projections {
		k := source{p.Name, p.Options.Scope}
		o := built[k]
		o.Scope = p.Options.Scope
		o.Columns = append(o.Columns, p.Options.Columns...)
		built[k] = o
		names = append(names, p.String())
	}

	var findings []Finding
	for _, d := range s.Published {
		// project.MissingColumns rather than a comparison written here: `fisc
		// export` asks the same question of the documents it wrote, and two
		// spellings of "does this cover what we said" is the divergence the
		// shared declaration exists to remove. It compares the scope too, so a
		// projection of the right name built over a different SCHEDULE does not
		// satisfy a published document by accident.
		absent := project.MissingColumns(d, built[source{d.Projection, d.Scope}])
		if len(absent) == 0 {
			continue
		}
		detail := "no projection was built at all"
		if len(names) > 0 {
			detail = fmt.Sprintf("the projections built were: %s", joinComma(names))
		}
		// The finding names the DOCUMENT, then which of its columns is missing,
		// because those are two different repairs. A whole document absent is a
		// projection that built nothing -- a scope typo, a registry entry
		// dropped. A column absent from a document that built is the corpus
		// having lost a printed column, and the fix is in the mapping.
		what := "and nothing checked it"
		if len(absent) < len(d.Columns) {
			what = fmt.Sprintf("and it was built without %s", project.Describe(absent))
		}
		findings = append(findings, finding(d.String(),
			"`fisc export` publishes this document %s — %s. Every check below this one "+
				"reads the projections, so a document that was not built is not failed, "+
				"it is unexamined", what, detail))
	}

	return conclusion{
		// One subject per published document, so the summary counts what the
		// site serves rather than what happened to build.
		subjects: len(s.Published),
		unit:     "published documents",
		held: fmt.Sprintf("every document the site publishes was built over every column "+
			"it publishes: %s", joinComma(describeDocuments(s.Published))),
		nothing:  "the site publishes no document, so there is nothing to have built",
		findings: findings,
	}.result(), nil
}

// describeDocuments names the published documents the way the report should: the
// stem a reader can fetch, then the columns and the scope behind it, so one can
// be matched against the projections listed elsewhere in the run.
func describeDocuments(docs []project.PublishedDocument) []string {
	out := make([]string, 0, len(docs))
	for _, d := range docs {
		out = append(out, d.String())
	}
	return out
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
		for _, k := range keysOf(f.Options) {
			refused[k] = 0
		}
	}

	// Which slices a projection IS OF, which is what a declaration of "no
	// projection draws this" is a claim about. It is read off the options rather
	// than off the facts that came back: a projection of a slice the store
	// happens to carry nothing for still draws that slice, and the declaration
	// is still false.
	drawn := map[sliceKey]bool{}
	for _, p := range s.Projections {
		for _, k := range keysOf(p.Options) {
			drawn[k] = true
		}
	}

	var findings []Finding
	declared := map[string]map[sliceKey]int{}
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
			if declared[f.Scope] == nil {
				declared[f.Scope] = map[sliceKey]int{}
			}
			declared[f.Scope][k]++
			continue
		}
		findings = append(findings, finding(f.ID,
			"%s p%d %q is FY%d %s %s, which no projection is of and which no entry in "+
				"unprojectedScopes declares unprojected",
			f.DocID, f.Page, f.RowLabel, f.FiscalYear, f.Basis, f.Scope))
	}
	for _, f := range s.ProjectionFailures {
		// Summed over the failure's own columns, not read from one key: a
		// document of four columns that refused to build stranded the facts of
		// all four, and reporting one column's worth would understate it.
		n := 0
		for _, k := range keysOf(f.Options) {
			n += refused[k]
		}
		if n > 0 {
			findings = append(findings, finding(f.String(),
				"%d facts in this slice are unprojected because the projection refused to "+
					"build; see projections-build. Whether their scopes are declared cannot "+
					"be established until it does", n))
		}
	}

	findings = append(findings, staleDeclarations(s, declared, drawn)...)

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
func describeDeclared(byScope map[string]map[sliceKey]int) string {
	out := make([]string, 0, len(byScope))
	for _, scope := range sortedStrings(byScope) {
		out = append(out, fmt.Sprintf("%d in scope %q (%s)",
			countOf(byScope[scope]), scope, unprojectedScopes[scope]))
	}
	return joinComma(out)
}

// countOf is how many facts a scope's per-slice tally covers.
func countOf(bySlice map[sliceKey]int) int {
	n := 0
	for _, c := range bySlice {
		n += c
	}
	return n
}

// describeSliceKeys names a set of slices the way a finding should, sorted so
// two runs report in the same order.
func describeSliceKeys(keys []sliceKey) string {
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].year != keys[j].year {
			return keys[i].year < keys[j].year
		}
		return keys[i].basis < keys[j].basis
	})
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, fmt.Sprintf("FY%d %s", k.year, k.basis))
	}
	return joinComma(out)
}

// staleDeclarations reports every unprojectedScopes entry that has stopped being
// true, in any of the three ways it can.
//
// The entry is a claim about a SCOPE -- "these facts exist and no projection
// draws them" -- and each way it can fail needs a different fix, so the finding
// says which:
//
//   - THE WHOLE SCOPE IS NOW DRAWN. The exemption is doing nothing and must be
//     deleted; leaving it means the next reader believes a schedule is out of
//     the chart when it is in it. This is the arm fisc-u2v (4) asked for, and it
//     has now fired in anger: the revenue-trends document draws all 924
//     revenue-by-fund facts and this branch is what made the entry go, in the
//     same commit, rather than surviving as a false paragraph verify printed on
//     every run.
//
//   - PART OF THE SCOPE IS DRAWN AND PART IS NOT (fisc-rmw). The entry is not
//     false as a whole, so the arm above stays quiet -- and that silence was the
//     bug. `declared` counts only facts that fell outside EVERY projection, so a
//     projection drawing half a scope leaves the other half counted, the total
//     arm skips, and verify goes on printing the entry's reason VERBATIM beside
//     a smaller number. A reader sees the count fall and the declaration hold,
//     which reads as coverage improving under a stable exemption -- the opposite
//     of what happened. Measured before the fix: a revenue-trends over the two
//     ADOPTED years only would have left declared["revenue-by-fund"] = 462 and
//     said nothing.
//
//   - NO RULE WRITES THE SCOPE AND NO FACT CARRIES IT. Either the rules were
//     removed and the entry outlived them, or this string and the one in
//     mappings/ have drifted apart -- the mistyped-scope incident this map
//     exists for, wearing its other face: the facts would be unprojected AND
//     undeclared, and this entry is not the declaration anyone thinks it is.
//
// THE UNIT OF THE PARTIAL ARM IS THE SLICE, not the fact count, and that is the
// whole of fisc-rmw's argument. describeDeclared prints "%d in scope %q", so a
// partly-drawn scope prints a smaller number beside an unchanged reason; what a
// reader needs instead is WHICH (fiscal year, basis) slices moved. sliceKey is
// the unit factsAreProjected already keys on, so the two halves of this check
// agree about what a slice is rather than each deciding.
//
// THE THIRD ARM ASKS THE RULE FILES, not the fact store, and that is the
// difference between "this schedule is not mapped here" and "this declaration is
// wrong". A Subject with no rule files -- a hand-built fixture, a miniature of
// one schedule -- is not a repository in which a declaration for another schedule
// has gone stale, and reporting one would make every fixture carry every
// schedule to stay green.
//
// Same direction as fisc-2sd's third failure mode, where a declared per-part
// delta that now ties exactly must fail rather than pass quietly. A declaration
// nobody can see expiring is a declaration that outlives its reason.
func staleDeclarations(s *Subject, declared map[string]map[sliceKey]int, drawn map[sliceKey]bool) []Finding {
	written := map[string]bool{}
	for _, f := range s.Files {
		for i := range f.Rules {
			written[f.Rules[i].Scope] = true
		}
	}

	var out []Finding
	for _, scope := range sortedStrings(unprojectedScopes) {
		// The slices of this scope some projection is of, and the slices whose
		// facts still fall outside every one. A scope is stale for the first set
		// and live for the second.
		var drawnSlices []sliceKey
		for k := range drawn {
			if k.scope == scope {
				drawnSlices = append(drawnSlices, k)
			}
		}
		var liveSlices []sliceKey
		for k := range declared[scope] {
			liveSlices = append(liveSlices, k)
		}

		// A scope with a REFUSED slice is in an unknown state, not a clean one.
		// factsAreProjected routes those facts to the refusal rather than to
		// `declared`, so liveSlices can be empty while the scope genuinely still
		// exempts everything the refused slice carried -- and this loop would
		// then demand the declaration be deleted on the strength of a projection
		// that did not build. Whether the entry is stale cannot be established
		// until the refusal is, which is what factsAreProjected already tells the
		// reader about the facts themselves.
		refusedHere := false
		for _, f := range s.ProjectionFailures {
			for _, k := range keysOf(f.Options) {
				if k.scope == scope {
					refusedHere = true
				}
			}
		}
		if refusedHere {
			continue
		}

		carried := 0
		for _, f := range s.Facts {
			if f.Scope == scope {
				carried++
			}
		}

		switch {
		case len(drawnSlices) > 0 && len(liveSlices) == 0:
			out = append(out, finding(scope,
				"unprojectedScopes declares this scope unprojected, but all %d of its facts "+
					"are in some projection's slice (%s); the declaration is exempting "+
					"nothing and must be removed",
				carried, describeSliceKeys(drawnSlices)))
		case len(drawnSlices) > 0 && len(liveSlices) > 0:
			// Both counts are printed because the two numbers are the finding:
			// the entry is a claim about a scope and it is now true of only part
			// of one, so a reader has to be told the shape of the split rather
			// than handed a total that moved.
			out = append(out, finding(scope,
				"unprojectedScopes declares this scope unprojected, and a projection now "+
					"draws %s while %d of its facts are still unprojected in %s. The entry "+
					"is no longer true of the whole scope: either draw it exhaustively so "+
					"this declaration retires, or rewrite the reason to say which slices it "+
					"still covers",
				describeSliceKeys(drawnSlices), countOf(declared[scope]),
				describeSliceKeys(liveSlices)))
		case carried == 0 && len(s.Files) > 0 && !written[scope]:
			out = append(out, finding(scope,
				"unprojectedScopes declares this scope unprojected, and no rule in %s writes "+
					"it and no fact carries it; the rules that wrote it are gone, or this "+
					"string and the one they write have drifted apart", mappingsDir))
		}
	}
	return out
}
