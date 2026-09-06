package check

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/jcrussell/livermore-budget/internal/mapping"
	"github.com/jcrussell/livermore-budget/internal/project"
)

// acfrGeneralFundScope is the scope ACFR MD&A p41's General Fund statement is
// mapped at. It is declared here rather than beside a reconciliation check
// because there is no such check and there cannot be one: see its entry in
// unprojectedScopes below.
//
// IT SITS ABOVE unprojectedScopes' DOC COMMENT AND NOT BETWEEN IT AND THE var,
// which is where it was first written. A declaration inserted into that gap
// orphans the comment: go doc prints 45 lines about what an unprojected scope
// means under a string constant, and the map itself documents nothing. AGENTS.md
// names this defect class and it had already happened twice in one commit
// elsewhere in this tree.
const acfrGeneralFundScope = "acfr-general-fund-summary"

// acfrFundBalancesScope is the scope ACFR p167's Fund Balances of Governmental
// Funds schedule is mapped at. Its sibling schedule's scope, acfrChangesScope,
// is declared in excessidentity.go beside the check that reads it.
const acfrFundBalancesScope = "acfr-fund-balances"

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
// A RETIREMENT IS THE MECHANISM WORKING RATHER THAN A LOSS.
// revenue-by-fund (Budget Book pp.127-140) was declared here with a
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
		"resolves the printed row anchors against data/funds.yaml instead -- its own " +
		"summary line carries the count, which is why one is not repeated here. Three " +
		"declared payers it cannot reach: p76 prints three continuation rows whose " +
		"\"Transfer From\" carries over from the row above, so they have no printed " +
		"anchor on their own line. A fund tier in the graph is fisc-gxa.2 / fisc-oxf; " +
		"leg-level links carrying a transfer_id are fisc-9gh.",

	departmentwideScope: "Budget Book pp.85-125, the Expenditures by Category block: the same " +
		"eleven pages' UPPER block, decomposing each department by division and object " +
		"category where the lower block decomposes it by paying fund. 29 division blocks, " +
		"73 object rows, 292 facts -- 288 expenditure and four the Transfers Out row on " +
		"p124 produces. These rows are the same money pp.66-67 publish, decomposed a third " +
		"way, so drawing them into the fund-group spine doubles the city's expenditure. " +
		"THEY CARRY NO FUND AND NO FUND GROUP, which is the page rather than an omission: " +
		"this block prints what a department spends whatever pays for it. So they cannot " +
		"tie to any one of the spine's six group cells and tie instead to its object " +
		"categories summed over all six. WHAT IS RECONCILED IS THE TWO BUDGET COLUMNS AND " +
		"NOTHING ELSE: departmentwide-ties-to-spine ties 8 cells, four object categories " +
		"over FY2026 adopted and FY2027 adopted, of which seven tie to the cent -- FY2026 " +
		"totalling 254,095,412, which IS all_funds_gross_expenditure_cents -- and one, " +
		"services-and-supplies FY2027, ties instead against 130,252,087 where the spine " +
		"publishes 130,502,087. That is the same 250,000 funding-sources-tie-to-spine " +
		"declares, ARRIVING ON A DIFFERENT AXIS: that check lands it on the " +
		"internal-service GROUP and this one on the SERVICES AND SUPPLIES category, and " +
		"between them they place p0067's error at one cell of a grid neither could locate " +
		"alone (fisc-av0w). The two historical columns, 146 of the 292 facts, tie to each " +
		"division's own printed Division Total at build time and to nothing on the spine, " +
		"because pp.66-67 print no actual or revised column. Five of the 29 divisions miss " +
		"that printed total by exactly one dollar, every one in the FY2023-24 Actual " +
		"column, declared as stated_total_deltas. TOTAL DEPARTMENT EXPENDITURES DOES NOT " +
		"HOLD THEM and it looks as though it should: no rule or rollup reads that row, " +
		"and all 29 rules stop at Division Total. Its printed value does equal the sum of " +
		"its page's printed Division Totals in all 44 (page, column) cells, but that was " +
		"measured by hand while writing this lane and nothing re-checks it. WHICH DIVISION SPENT THE MONEY IS NOT COVERED BY THE " +
		"ARITHMETIC AT ALL: the check sums the divisions away, so a dollar moved from " +
		"Patrol to Horizons inside one object category and year leaves every cell " +
		"unchanged. What holds it is fact-departments-resolve against " +
		"data/departments.yaml, whose six pp.85-125-only divisions this lane added, plus " +
		"each division's own printed Division Total",

	acfrGeneralFundScope: "ACFR p41, the General Fund's Statement of Revenues, Expenditures " +
		"and Changes in Fund Balances: 20 audited FY2024-25 figures, and the only facts in " +
		"this store from a document other than the Budget Book. The page prints FOUR blocks " +
		"and three of them are mapped WHOLE -- ten revenue rows tying to the printed Total " +
		"Revenues of 157.20 exactly; two transfer legs, 0.53 in and (25.72) out, tying to the " +
		"printed Total Other Financing Sources (Uses) of (25.19) exactly; and the three " +
		"fund-balance lines that close the statement, beginning 92.10, change (5.00), ending " +
		"87.10. The fourth, the expenditure block, is mapped IN PART: the five divisions of " +
		"the General Government function and none of its other rows. All in millions to two " +
		"decimals, so the least significant printed digit is $10,000. " +
		"IT IS UNPROJECTED BECAUSE IT IS A DIFFERENT YEAR ON A DIFFERENT BASIS, not because " +
		"it restates money some other scope already publishes -- which is the opposite of " +
		"every other entry in this map. The Budget Book spine is FY2026 and FY2027 adopted; " +
		"this is FY2025 audited, and the spine prints no audited column, so these facts share " +
		"no (kind, category, fund_group, fund, fiscal year, basis) key with anything published " +
		"and no doubling is possible -- measured again when the ten-year documents landed: " +
		"zero shared keys with either, at the full grain and at the mergeable one. The " +
		"history pages draw ten audited years of the two statistical-section scopes, and " +
		"this page can join neither table: it is the General Fund alone where pp.168-169 " +
		"combine all governmental funds, and its ending balance does not tie to p167's " +
		"components -- $56,424 apart in FY2025, 5.6 units of p41's own printed precision " +
		"(fisc-y242). " +
		"AND SO THIS SCOPE CARRIES NO DETAIL-TIES-TO-SPINE CHECK, which breaks the rule every " +
		"other non-spine scope here follows. There is nothing on the spine to tie it to. What " +
		"asserts these facts instead is fund-balance-identity, which reaches three of the 20 " +
		"-- beginning + change == ending, the same identity it asserts over the spine's twelve " +
		"cells -- plus CheckTotals at build time on the other 17. " +
		"THE GENERAL GOVERNMENT BLOCK IS THE ONE PLACE IN THIS CORPUS THAT TIES ONLY WITHIN A " +
		"TOLERANCE, and the build report says so on every run rather than counting it as a " +
		"clean tie: its five divisions print 18.44 against a printed subtotal of 18.45, one " +
		"printed unit over five terms, so the bound derived from the page is half a unit per " +
		"row -- $25,000, which is what this column's $10,000 is admitted by and is also the " +
		"most any column of this rule could be out by and still tie (fisc-1wr.2). It is also " +
		"the only rule reading a total the document prints ABOVE its own rows, which is bounded " +
		"by pinning that total to the section anchor's own line (fisc-h96o). " +
		"The remaining exposure is stated rather than absorbed: this page declares no " +
		"column_headers, because its printed headers are the bare years 2025 and 2024 and the " +
		"parser refuses a header amount.Parse accepts, and because the geometry line pairing " +
		"fails on the page anyway (a stray \"0.0\" with no row label sits inside the line " +
		"tolerance of the row above, giving 51 geometry lines against 52 text lines). So no " +
		"geometry column guard stands over any of these figures -- the widest tolerance in the " +
		"corpus over the fewest guards, which is why the bound is half a unit per row and not " +
		"the worst case that would also add the printed total's own half unit. " +
		"WHAT IS STILL NOT MAPPED is the rest of the expenditure block: SIX of its nine other " +
		"top-level rows are the ACFR's remaining functions -- Fire, Police, Public Works, " +
		"Community Development, Economic Development, Library -- and unlike General " +
		"Government's five divisions they are departments fact-departments-resolve cannot " +
		"accept (fisc-xudn). The other three, Capital Outlay, Principal and Interest and " +
		"fiscal charges, are not departments at all and are blocked by nothing; they are " +
		"simply unmapped. No row of " +
		"the mapped block carries a department either, for a reason of its own -- two of the " +
		"five name departments covering several divisions -- see data/taxonomy.yaml's " +
		"general-government entry. The FY2024 column of every block is present and skipped: " +
		"three of the four PRINTED blocks miss in it -- revenue by $200,000, expenditure by " +
		"$100,000 and the fund balances by $100,000, twenty, ten and ten printed units, with " +
		"only Other Financing Sources tying. The General Government sub-block misses by " +
		"$170,000 on top of that, seventeen units, which is why the tolerance that admits its " +
		"$10,000 in FY2025 comes nowhere near admitting FY2024. The four blocks are the ones the " +
		"PAGE prints, and General Government is inside one of them.",

	fundingSourcesScope: "Budget Book pp.85-125, Department Funding Sources: the per-fund " +
		"decomposition of pp.66-67's TOTAL EXPENDITURES rows, not additional money. Its 78 " +
		"printed rows sum, per fund group, to those pages' expenditure cells -- capital " +
		"1,061,355 / 969,934, debt-service 6,984,597 / 6,969,898, enterprise 57,053,730 / " +
		"57,547,970, general 144,650,802 / 149,014,579, internal-service 25,077,367 / " +
		"26,294,515, special-revenue 19,267,561 / 11,808,000, total 254,095,412 / " +
		"252,604,896 -- so drawing them into the fund-group spine doubles the city's " +
		"expenditure. ALL SEVEN GROUPS ARE LISTED ON PURPOSE: an earlier draft omitted " +
		"internal-service, and the five that were left sum to 229,018,045, which is a " +
		"DIFFERENT published headline -- the sankey's external expenditure -- so a reader " +
		"adding the list up landed on a real figure that was not this schedule's total. WHAT IS RECONCILED IS THE TWO BUDGET COLUMNS AND NOTHING ELSE: " +
		"funding-sources-tie-to-spine ties 14 cells, seven fund groups over FY2026 adopted " +
		"and FY2027 adopted, of which eleven tie to the cent against the spine, two are the " +
		"permanent group agreeing at zero against a spine that prints no Permanent column " +
		"at all (fisc-u8o), and one -- internal-service FY2027 -- ties instead against " +
		"26,294,515, the figure five other schedules print where p0067 prints 26,544,515 " +
		"(fisc-av0w). The two historical columns, 156 of the 312 facts, tie to each " +
		"department's own printed Total Department Funding Sources at build time and to " +
		"nothing on the spine, because pp.66-67 print no actual or revised column; five of " +
		"the eleven departments miss that printed total by exactly one dollar, every one in " +
		"the FY2023-24 Actual column, declared as stated_total_deltas. THE FUND NUMBER ON " +
		"EACH ROW IS NOT COVERED BY THE ARITHMETIC AT ALL -- it is hand-typed 78 times, " +
		"and what the arithmetic catches is a fund of the wrong TYPE, because that moves " +
		"money between groups and breaks a sum, while a same-type substitution such as " +
		"Water 640 for CIP Water 641 moves nothing. It is checked instead by " +
		"row-funds-match-their-anchors, because these eleven rules declare " +
		"row_labels_name_funds: their row labels are printed fund names, so the number " +
		"typed beside each one is read against the label the page prints (fisc-90fp). " +
		"rule-funds-match-their-headings still never enters, because it reads column " +
		"funds and these rules declare none. Drawing this schedule as department pages is fisc-4ua.2, " +
		"which also needs the Expenditures by Category block above it (fisc-7q6).",
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

// Run counts one subject per SLICE a projection was asked for — the built ones
// and the failed ones together — because the claim is about all of them and a
// count of only the failures would read as "1 of 1" on a corpus where nine
// slices built and one did not.
//
// THE UNIT IS SLICES AND SO IS THE COUNT, which took a fix (fisc-rwo). It
// counted PROJECTIONS under that unit, and one projection stopped being one
// slice when the first multi-column document landed: keysOf was added to this
// file for exactly that reason -- its own doc comment says "anything keying a
// map on a projection has to iterate this rather than call it once" -- and this
// Run did not. Measured on the committed corpus, it reported "3 projection
// slices built" over six: the two sankey columns plus revenue-trends' four.
func (*projectionsBuild) Run(_ context.Context, s *Subject) (Result, error) {
	findings := make([]Finding, 0, len(s.ProjectionFailures))
	for _, f := range s.ProjectionFailures {
		findings = append(findings, finding(f.String(),
			"this projection refused to build: %v. Every check that reads a graph is "+
				"silent about this slice — they are not failing it, they cannot see it",
			f.Err))
	}
	slicesBuilt := 0
	for _, p := range s.Projections {
		slicesBuilt += len(keysOf(p.Options))
	}
	slicesRefused := 0
	for _, f := range s.ProjectionFailures {
		slicesRefused += len(keysOf(f.Options))
	}
	return conclusion{
		subjects: slicesBuilt + slicesRefused,
		unit:     "projection slices",
		held: fmt.Sprintf("%d %s built, none refused",
			slicesBuilt, plural(slicesBuilt, "projection slice", "projection slices")),
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

// keysOf is every slice one Options covers: one per column PER SCOPE.
//
// IT RETURNS A LIST BECAUSE ONE DOCUMENT NEED NOT BE ONE SLICE. While every
// projection was of a single (year, basis) this was a one-to-one map and was
// spelled as one; a trends document is of four columns of one schedule, so an
// Options that maps to a single key can no longer be assumed. Anything keying a
// map on a projection has to iterate this rather than call it once.
//
// THE PRODUCT OVER SCOPES IS LOAD-BEARING AND NOT TIDINESS. A sliceKey is the
// address a FACT has -- (year, basis, scope) -- so a document of two schedules
// occupies two addresses per column and must say so. factsAreProjected builds
// its `drawn` set from these keys and staleDeclarations decides from that set
// whether an unprojectedScopes entry has stopped being true. Emitting one key
// per column with a single scope would leave a drawn schedule looking undrawn:
// the entry declaring it unprojected would stay quiet while the document
// publishing it shipped, which is the precise failure "an entry that has
// stopped being true must go RED, not quiet" is written against.
func keysOf(o project.Options) []sliceKey {
	out := make([]sliceKey, 0, len(o.Columns)*len(o.Scopes))
	for _, c := range o.Columns {
		for _, scope := range o.Scopes {
			out = append(out, sliceKey{c.FiscalYear, c.Basis, scope})
		}
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
	// Every SLICE built, grouped by the projection and scope that built it. A
	// published document must be covered by ONE of them.
	//
	// IT USED TO BE THE UNION OF THEM, and that is fisc-b8o. The union was
	// justified as not caring what shape a document is -- the spine publishes
	// one document per year and builds one Options per year, the trends publish
	// one document over four columns built as one Options -- but the shape is
	// exactly what decides whether the published FILE exists. Measured: a
	// four-column document is satisfied by four single-column slices, and
	// `fisc export` then writes four files of which the one at the declared
	// stem covers a quarter of what it declares. Verify green, site wrong. One
	// document is one file is one slice, and today every published document is
	// covered by exactly one.
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
	type source struct{ name, scopes string }
	built := map[source][]project.Options{}
	names := make([]string, 0, len(s.Projections))
	for _, p := range s.Projections {
		k := source{p.Name, scopeKey(p.Options.Scopes)}
		built[k] = append(built[k], p.Options)
		names = append(names, p.String())
	}

	var findings []Finding
	// The slices some published document claims, so the sweep below can report
	// the ones none does.
	claimed := map[string]bool{}
	for _, d := range s.Published {
		// project.MissingColumns rather than a comparison written here: `fisc
		// export` asks the same question of the documents it wrote, and two
		// spellings of "does this cover what we said" is the divergence the
		// shared declaration exists to remove. It compares the scope too, so a
		// projection of the right name built over a different SCHEDULE does not
		// satisfy a published document by accident.
		// The BEST-covering slice, not the first, and it is marked claimed even
		// when it covers the document only partly. A slice short a column is
		// already reported here, by name and by which column; letting the sweep
		// below report it a second time as "no document covers this" would give
		// one defect two findings that read as two, and send a reader looking
		// for a stray document that is really the declared one gone short.
		absent := d.Columns
		for _, o := range built[source{d.Projection, scopeKey(d.Scopes)}] {
			missing := project.MissingColumns(d, o)
			if len(missing) < len(absent) {
				absent = missing
				claimed[sliceID(d.Projection, o)] = true
			}
			if len(absent) == 0 {
				break
			}
		}
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

	// THE OTHER DIRECTION, and it is the half fisc-b8o's acceptance criterion
	// asks for. A slice that built and that no published document claims is a
	// file `fisc export` writes and the site never declares -- and the way it
	// bites is not the stray file: it is that the site would serve a document it
	// never declared, under a name nothing else expects.
	//
	// It asks project.Stem rather than pkg/cmd/export's stemFor, and the
	// distinction is the decision fisc-b8o left open. Stem is the shared
	// declaration -- one place, read by the packager that writes the files, by
	// project.PublishedDocuments that declares them, and by this check -- so
	// reading it is this package doing its job, where reaching into a command's
	// naming helper would have been the coupling internal/check avoids
	// everywhere else.
	at := map[string]string{}
	for _, d := range s.Published {
		at[d.Stem] = d.String()
	}
	// THE DECLARED SLICES OF EACH PROJECTION, BY NAME, built and refused
	// together -- which is the list `fisc export` names its files from, and the
	// reason it is assembled here rather than reusing `built` above. `built` is
	// keyed on (name, scope) and holds only what BUILT, so a projection that
	// declared three slices and refused one would be named against a list of
	// two, and this check would report a stem export never writes. The one
	// spelling of the rule is only worth having if its argument is one thing
	// too.
	declared := map[string][]project.Options{}
	for _, p := range s.Projections {
		declared[p.Name] = append(declared[p.Name], p.Options)
	}
	for _, f := range s.ProjectionFailures {
		declared[f.Name] = append(declared[f.Name], f.Options)
	}
	for _, p := range s.Projections {
		if claimed[sliceID(p.Name, p.Options)] {
			continue
		}
		stem, err := project.Stem(p.Name, p.Options, declared[p.Name])
		if err != nil {
			// An Options with no columns. It built, so it is not
			// projectionsBuild's subject; it cannot be named, so it is not this
			// sweep's either, and the graph checks read it like any other.
			continue
		}
		detail := "the site declares no document at that stem, so `fisc export` would " +
			"write a file nothing published claims"
		if other, ok := at[stem]; ok {
			detail = fmt.Sprintf("the site already publishes %s at that stem, so `fisc "+
				"export` computes one stem for two documents and its duplicate-stem guard "+
				"refuses the ENTIRE export -- no file written, including the ones that "+
				"were fine", other)
		}
		findings = append(findings, finding(p.String(),
			"this slice was built and no document the site publishes covers it. It would "+
				"be written at stem %q, and %s", stem, detail))
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
// sliceID names one built slice, for telling a claimed slice from an unclaimed
// one. Name, scope and every column, so two slices of one projection that
// differ only in basis are two ids and not one.
func sliceID(name string, o project.Options) string {
	return name + "\x1f" + scopeKey(o.Scopes) + "\x1f" + project.Describe(o.Columns)
}

// scopeKey is a scope SET as one comparable string.
//
// Sorted, unlike [project.Options.ScopeList], and the difference is the point:
// ScopeList is published text and keeps the order a projection declared, while
// this is a map key and two declarations of the same schedules must land on it
// however they were written. A sorted copy rather than a sort in place, because
// the slice it is handed is a projection's own declaration.
func scopeKey(scopes []string) string {
	sorted := slices.Clone(scopes)
	slices.Sort(sorted)
	return strings.Join(sorted, "\x1f")
}

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
	// ONE FINDING PER REFUSED SLICE, NOT ONE PER FAILURE, and the difference is
	// the number a reader triages on.
	//
	// It used to sum refused[k] over each failure's OWN columns, independently.
	// That is right for one document of four columns -- all four are stranded,
	// and reporting one column's worth would understate it -- and wrong the
	// moment two projections refuse the SAME slice: `refused` is keyed on
	// (year, basis, scope), so both claimed the full count and a reader adding
	// the findings up got twice the facts that exist.
	//
	// The slice is what the facts are stranded IN, so it is what the finding is
	// about. Every failure that refused it is still named, so no refusal goes
	// unreported here; projections-build reports them a second time as refusals
	// in their own right.
	refusers := map[sliceKey][]string{}
	var stranded []sliceKey
	for _, f := range s.ProjectionFailures {
		for _, k := range keysOf(f.Options) {
			if len(refusers[k]) == 0 {
				stranded = append(stranded, k)
			}
			refusers[k] = append(refusers[k], f.String())
		}
	}
	for _, k := range stranded {
		if refused[k] == 0 {
			continue
		}
		findings = append(findings, finding(fmt.Sprintf("FY%d %s %s", k.year, k.basis, k.scope),
			"%d facts in this slice are unprojected because %s refused to build: %s. See "+
				"projections-build. Whether their scopes are declared cannot be "+
				"established until it does",
			refused[k], plural(len(refusers[k]), "a projection", "projections"),
			joinComma(refusers[k])))
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

		// THE NO-FACTS ARM IS TESTED FIRST, and the order is the fix rather
		// than an aesthetic (fisc-rwo). Arm 2 below tests only drawnSlices and
		// liveSlices, never `carried`, so a scope no rule writes at all reported
		// "all 0 of its facts are in some projection's slice" whenever any
		// projection merely DECLARED that scope -- and the mistyped-scope
		// diagnosis this arm exists for was unreachable. `carried == 0` is the
		// stronger and more specific claim, so it is asked first.
		//
		// Reordering rather than gating arm 2 on `carried > 0`: gating would
		// leave carried == 0, written[scope], drawnSlices > 0 matching no arm at
		// all, and a declaration exempting nothing would go unreported. Silence
		// is the failure this whole check is about.
		switch {
		case carried == 0 && len(s.Files) > 0 && !written[scope]:
			out = append(out, finding(scope,
				"unprojectedScopes declares this scope unprojected, and no rule in %s writes "+
					"it and no fact carries it; the rules that wrote it are gone, or this "+
					"string and the one they write have drifted apart", mappingsDir))
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
		}
	}
	return out
}
