package check

import (
	"context"
	"fmt"
	"sort"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/mapping"
	"github.com/jcrussell/livermore-budget/internal/project"
)

// scopePair is two scopes as one comparable value, always in sorted order so a
// pair has one spelling however it is asked for.
type scopePair struct{ a, b string }

func pairOf(x, y string) scopePair {
	if x > y {
		x, y = y, x
	}
	return scopePair{x, y}
}

func (p scopePair) String() string { return p.a + " + " + p.b }

// reconciledScopes are the pairs a reconciliation check relates, and the check
// that relates them.
//
// A RECONCILIATION IDENTITY IS THE STATEMENT THAT TWO SCOPES ARE THE SAME MONEY.
// That is fisc-gkv point B, and it is the half of this check that no measurement
// can supply. Measured on the committed store, all-funds-gross and
// revenue-by-fund share ZERO keys of (kind, category, fund_group, fund, year,
// basis) -- the spine carries no fund and the detail carries fund numbers -- and
// they are nevertheless the same $299,969,007, cell for cell, which
// revenue-detail-ties-to-spine proves at zero tolerance. A key comparison alone
// would call that pair disjoint and let one projection publish both.
//
// The value is the check id, and it must name a check that exists: a table of
// prose about checks drifts the moment one is renamed, so the first arm of the
// run below resolves every id against All().
var reconciledScopes = map[scopePair]string{
	pairOf(project.PublishedScope, revenueDetailScope):     "revenue-detail-ties-to-spine",
	pairOf(project.PublishedScope, expenditureDetailScope): "expenditure-detail-ties-to-spine",
	pairOf(project.PublishedScope, transfersDetailScope):   "transfers-detail-ties-to-spine",
	pairOf(project.PublishedScope, fundingSourcesScope):    "funding-sources-tie-to-spine",
	pairOf(project.PublishedScope, departmentwideScope):    "departmentwide-ties-to-spine",
}

// disjointScopes are the pairs a projection MAY select together, each with the
// reason they cannot collide.
//
// DECLARED RATHER THAN INFERRED, because the absence of evidence is not
// evidence: a pair sharing no key today might still restate the same money, and
// the entry above proves that is not hypothetical. So a pair is safe to
// co-select only if it is named HERE, and an undeclared pair is a finding
// (fisc-gkv point B: "fail closed on ambiguity -- an undeclared pair is an
// error, not a pass"). The reason is required and is checked against the corpus
// by the measured arm below, so a declaration that stops being true goes red.
var disjointScopes = map[scopePair]string{
	pairOf(project.PublishedScope, acfrFundBalancesScope): "FY2016-FY2025 audited against " +
		"FY2026-27 adopted: selectFacts admits one (fiscal_year, basis) per projection " +
		"column, so no column holds both. And unlike acfr-general-fund-summary " +
		"(fisc-tlbp), the pair shares zero keys even at netCells' own three-field grain " +
		"(kind, category, fund_group), measured over the committed store: the GASB 54 " +
		"component categories are ones the spine never writes",

	pairOf(project.PublishedScope, acfrChangesScope): "FY2016-FY2025 audited against " +
		"FY2026-27 adopted, and zero shared keys at netCells' three-field grain here " +
		"too, for a different reason than its sibling: every fact of this scope carries " +
		"no fund_group (ACFR pp.168-169 print all governmental funds combined), which " +
		"netCells refuses outright, so no projection can merge one with a spine cell " +
		"in any column",

	pairOf(revenueDetailScope, expenditureDetailScope): "disjoint by KIND, and the disjointness " +
		"is structural rather than lucky: pp.127-140 publish revenue and transfer_in and " +
		"pp.167-170 publish expenditure, and every cell key internal/project builds carries " +
		"kind, so a revenue cell and an expenditure cell cannot land on one address however " +
		"the rest of the key is spelled",
}

// projectionScopesAreDisjoint asserts no projection selects two scopes that
// restate the same money.
//
// THE HAZARD IS THE ONE fisc-gkv MEASURED AND IT IS SILENT. Merging two scopes
// that publish the same figures doubles them while every downstream check
// agrees: headline-ties-to-facts re-sums the same facts the headline was
// accumulated from, so it reports GREEN on the doubled number, and the node and
// link counts are unchanged because the merged graph is structurally
// indistinguishable from the correct one. Options.Scopes made that expressible;
// this is what keeps it from being reachable.
//
// TWO SOURCES OF RELATEDNESS, AND EACH CATCHES WHAT THE OTHER CANNOT.
//
//   - The DECLARED source is the reconciliation certificates (reconciledScopes).
//     It is the only thing that sees the spine-against-detail case, where the
//     two scopes are the same money at different grains and therefore share no
//     key at all.
//   - The MEASURED source is a shared (kind, category, fund_group, fund, year,
//     basis) key. It is the only thing that sees the detail-against-detail case,
//     which no ties-to-spine check relates: measured on the committed store,
//     revenue-by-fund and transfers-by-fund share 22 such keys carrying
//     $42,183,495 of transfer_in across the four columns, because pp.127-140
//     print a fund's transfers in and p76 prints the same movement from the
//     other end. Nothing declares that pair, and a projection selecting both
//     would double it.
//
// Do not fold the two into one rule. Each is blind exactly where the other sees.
type projectionScopesAreDisjoint struct{}

var _ Check = (*projectionScopesAreDisjoint)(nil)

func (*projectionScopesAreDisjoint) ID() string { return "projection-scopes-are-disjoint" }
func (*projectionScopesAreDisjoint) Tier() int  { return 1 }
func (*projectionScopesAreDisjoint) Full() bool { return false }
func (*projectionScopesAreDisjoint) Description() string {
	return "no projection selects two scopes that restate the same money, and every pair one " +
		"does select is declared disjoint with a reason the corpus agrees with"
}

func (*projectionScopesAreDisjoint) Run(_ context.Context, s *Subject) (Result, error) {
	var findings []Finding

	// ARM 1: the declared table names checks that exist. A reconciliation id
	// that no check answers to is a table describing a guarantee nothing makes.
	ids := map[string]bool{}
	for _, ch := range All() {
		ids[ch.ID()] = true
	}
	for _, p := range sortedPairs(reconciledScopes) {
		if id := reconciledScopes[p]; !ids[id] {
			findings = append(findings, finding(p.String(),
				"this pair is declared reconciled by %q, and no check has that id. The "+
					"declaration is the only thing that sees a spine-against-detail pair, "+
					"so a stale one silently permits the merge it exists to refuse", id))
		}
	}

	// ARM 2: the measured relation, over every pair of scopes the corpus
	// carries. This is the subject count: each pair is a thing examined.
	shared := sharedKeys(s.Facts)
	// A DECLARATION MUST HOLD UNDER THE COARSEST GRAIN A PROJECTION CAN REACH,
	// not only the finest. That grain is cellAddress minus fund, because the
	// spine carries no fund where the detail carries fund numbers, so measuring
	// only cellAddress let a declaration assert a safety that did not hold
	// (fisc-tlbp). It is NOT netCells' own three fields: see mergeableAddress.
	sharedNet := sharedMergeableKeys(s.Facts)
	pairs := allScopePairs(s.Facts)
	for _, p := range pairs {
		reason, declared := disjointScopes[p]
		if !declared {
			continue
		}
		if len(shared[p]) > 0 {
			findings = append(findings, finding(p.String(),
				"declared disjoint (%s) and the corpus disagrees: these scopes share %d "+
					"key(s) of (kind, category, fund_group, fund, fiscal year, basis), so "+
					"a projection selecting both would publish that money twice",
				reason, len(shared[p])))
			continue
		}
		if len(sharedNet[p]) > 0 {
			findings = append(findings, finding(p.String(),
				"declared disjoint (%s) and the corpus disagrees at the grain a projection "+
					"can reach: these scopes share no (kind, category, fund_group, fund, "+
					"fiscal year, basis) address, but %d of (kind, category, fund_group, "+
					"fiscal year, basis) -- the same key without the FUND, which is what a "+
					"projection merges on because the spine carries no fund where the detail "+
					"carries fund numbers. A projection selecting both would merge those cells",
				reason, len(sharedNet[p])))
		}
	}

	// ARM 3: no projection selects a related pair, and every pair it does
	// select is positively declared disjoint.
	//
	// CO-SELECTED PAIRS ARE COUNTED AND REPORTED ONCE, not once per document
	// that selects them. A projection publishing four documents selects the same
	// pair four times, and reporting that as "4 pairs" beside a total of 6 sends
	// a reader looking for four entries in disjointScopes when there is one --
	// the same multiplicity would also emit four byte-identical findings for a
	// single declaration defect.
	coSelected := map[scopePair]bool{}
	reported := map[scopePair]bool{}
	for _, pr := range s.Projections {
		scopes := pr.Options.Scopes
		if len(scopes) < 2 {
			continue
		}
		for i := 0; i < len(scopes); i++ {
			for j := i + 1; j < len(scopes); j++ {
				p := pairOf(scopes[i], scopes[j])
				coSelected[p] = true
				if reported[p] {
					continue
				}
				reported[p] = true
				switch {
				case reconciledScopes[p] != "":
					findings = append(findings, finding(pr.String(),
						"selects %s, and %q reconciles one to the other. A reconciliation "+
							"identity IS the statement that two scopes are the same money, "+
							"so one document holding both publishes it twice -- and every "+
							"check downstream agrees, because they re-sum the same facts",
						p, reconciledScopes[p]))
				case len(shared[p]) > 0:
					findings = append(findings, finding(pr.String(),
						"selects %s, which share %d key(s) of (kind, category, fund_group, "+
							"fund, fiscal year, basis). Nothing reconciles them to each "+
							"other, so no other check would see the doubling", p, len(shared[p])))
				case disjointScopes[p] == "":
					findings = append(findings, finding(pr.String(),
						"selects %s and nothing declares that pair disjoint. Sharing no key "+
							"today is not evidence: all-funds-gross and revenue-by-fund share "+
							"none and are the same money at two grains. Add the pair to "+
							"disjointScopes with the reason it cannot collide, or do not "+
							"select both", p))
				}
			}
		}
	}

	// COUNTED BY ROUTE AND NOT UNIONED INTO ONE NUMBER, because the two routes
	// are this check's whole argument and a reader has to see that neither is
	// redundant. On the committed store one pair is related by BOTH, so the
	// counts deliberately do not add up: reporting "4 of them, 3 by a check and
	// the rest by a key" would imply the routes partition the pairs, and the day
	// one route stopped finding anything the arithmetic would still look right.
	var byCheck, byKey, byBoth int
	for _, p := range pairs {
		rec, key := reconciledScopes[p] != "", len(shared[p]) > 0
		switch {
		case rec && key:
			byBoth++
			byCheck++
			byKey++
		case rec:
			byCheck++
		case key:
			byKey++
		}
	}
	related := byCheck + byKey - byBoth
	return conclusion{
		subjects: len(pairs),
		unit:     "scope pairs",
		held: fmt.Sprintf("%d pair(s) over the %d scope(s) the corpus carries, of which %d "+
			"restate the same money: %d related by a reconciliation check, %d by a shared "+
			"(kind, category, fund_group, fund, fiscal year, basis) key, %d by both. %d "+
			"distinct pair(s) are selected together by a projection, each declared disjoint",
			len(pairs), countScopes(s.Facts), related, byCheck, byKey, byBoth, len(coSelected)),
		nothing:  "the corpus carries fewer than two scopes, so no pair could restate another",
		findings: findings,
	}.result(), nil
}

// cellAddress is the finest place a fact could land in a projection's cell map.
//
// SPELLED HERE RATHER THAN IMPORTED, and deliberately: it is the widest address
// a document could key on, so two scopes sharing one would collide in a
// projection keyed at ANY grain.
//
// IT IS NOT SUFFICIENT ON ITS OWN. A coarser key collides MORE, so "could these
// two scopes collide" is answered by the coarsest grain a projection can reach,
// not by the finest one a document could key on. That is mergeableAddress below
// -- this address without the fund -- and ARM 2 measures both (fisc-tlbp).
type cellAddress struct {
	Kind      mapping.Kind
	Category  string
	FundGroup string
	// Fund is fact.FundString's rendering: a pointer would key by address.
	Fund       string
	FiscalYear int
	Basis      mapping.Basis
}

// mergeableAddress is the coarsest address at which a projection can actually
// merge two facts, which is NOT the same as the key netCells writes.
//
// THE KEY project.netCells WRITES IS THREE FIELDS AND THAT IS NOT THE REACHABLE
// GRAIN. It is qualified because this package declares a netCells of its own.
// project.netCells has one caller, sankey, and it is handed selectFacts' output
// -- which admits a fact only if its (fiscal_year, basis) is one of the columns
// selected -- and sankey refuses any Options carrying more than one column. So
// every fact that reaches netCells already agrees on year and basis, and two
// facts differing in either can never meet in a cell. Adding them back gives the
// grain a projection can realize.
//
// It is therefore cellAddress MINUS FUND, and fund is the whole of the
// difference. That is not a small distinction: the spine carries no fund and the
// detail schedules carry fund numbers, so a spine cell and a detail cell for the
// same category in the same year are a collision no cellAddress comparison can
// see.
//
// MEASURED on the committed store, over facts netCells would accept, as shared
// (fine, mergeable) keys:
//
//	all-funds-gross + revenue-by-fund         0, 74
//	all-funds-gross + transfers-by-fund       2, 16
//	revenue-by-fund + transfers-by-fund      22,  8
//
// and every ACFR pair is (0, 0), because that scope is FY2025 audited and
// everything else is FY2026-27 adopted. An earlier version of this type carried
// only netCells' three fields; under that key the ACFR pairs read 15, 10 and 2,
// and a TRUE disjointness declaration for them was refused -- which would have
// blocked E10's ACFR-beside-the-spine document except by writing a false reason.
// fisc-tlbp's own description had warned of exactly that.
type mergeableAddress struct {
	Kind       mapping.Kind
	Category   string
	FundGroup  string
	FiscalYear int
	Basis      mapping.Basis
}

// sharedMergeableKeys is, for every pair of scopes, the addresses both publish a
// fact at that a projection could actually merge.
//
// It counts only facts [canMerge] admits, for the reason given there.
func sharedMergeableKeys(facts []fact.Fact) map[scopePair]map[mergeableAddress]bool {
	byScope := map[string]map[mergeableAddress]bool{}
	for i := range facts {
		f := &facts[i]
		if !canMerge(f) {
			continue
		}
		a := mergeableAddress{
			Kind: f.Kind, Category: f.Category, FundGroup: f.FundGroup,
			FiscalYear: f.FiscalYear, Basis: f.Basis,
		}
		if byScope[f.Scope] == nil {
			byScope[f.Scope] = map[mergeableAddress]bool{}
		}
		byScope[f.Scope][a] = true
	}
	scopes := make([]string, 0, len(byScope))
	for s := range byScope {
		scopes = append(scopes, s)
	}
	sort.Strings(scopes)

	out := map[scopePair]map[mergeableAddress]bool{}
	for i := 0; i < len(scopes); i++ {
		for j := i + 1; j < len(scopes); j++ {
			p := pairOf(scopes[i], scopes[j])
			for a := range byScope[scopes[i]] {
				if byScope[scopes[j]][a] {
					if out[p] == nil {
						out[p] = map[mergeableAddress]bool{}
					}
					out[p][a] = true
				}
			}
		}
	}
	return out
}

// canMerge reports whether a fact could take part in a collision at all.
//
// project.netCells is the only place in this repository where two facts are ever
// summed into one cell, and it ERRORS rather than merging on a fact carrying a
// department, or missing a category or a fund group. Such a fact stops a build;
// it cannot be double-published by one. Counting it overstates the hazard, and
// the overstatement has teeth: every expenditure-by-department fact carries a
// department, so without this that scope reports collisions it cannot have and a
// TRUE disjointness declaration for it would be refused.
//
// BOTH ARMS APPLY IT, and they must. cellAddress models a document keyed more
// finely than any that exists, but even a finer-grained document would have to
// merge somewhere, and project.netCells is the only somewhere. Measured at the
// commit that introduced this: filtering changes no pair's FINE count today and
// takes all-funds-gross + expenditure-by-department from 8 mergeable keys to 0,
// so it is latent on one arm and load-bearing on the other.
//
// If a second merge point is ever added, this reasoning is what has to be
// revisited -- not the field lists.
func canMerge(f *fact.Fact) bool {
	return f.Department == "" && f.Category != "" && f.FundGroup != ""
}

// sharedKeys is, for every pair of scopes, the addresses both publish a fact at.
//
// THE KEY IS THE ONE A PROJECTION WOULD COLLIDE ON, not a convenient subset.
// kind, category and fund_group are what internal/project keys cells on today;
// fund and the column are what a finer-grained document adds. A pair sharing an
// address is a pair a projection could net into one cell.
func sharedKeys(facts []fact.Fact) map[scopePair]map[cellAddress]bool {
	type addr = cellAddress
	byScope := map[string]map[addr]bool{}
	for i := range facts {
		f := &facts[i]
		if !canMerge(f) {
			continue
		}
		a := addr{
			Kind: f.Kind, Category: f.Category, FundGroup: f.FundGroup,
			Fund: fact.FundString(f.Fund), FiscalYear: f.FiscalYear, Basis: f.Basis,
		}
		if byScope[f.Scope] == nil {
			byScope[f.Scope] = map[addr]bool{}
		}
		byScope[f.Scope][a] = true
	}
	scopes := make([]string, 0, len(byScope))
	for s := range byScope {
		scopes = append(scopes, s)
	}
	sort.Strings(scopes)

	out := map[scopePair]map[addr]bool{}
	for i := 0; i < len(scopes); i++ {
		for j := i + 1; j < len(scopes); j++ {
			p := pairOf(scopes[i], scopes[j])
			for a := range byScope[scopes[i]] {
				if byScope[scopes[j]][a] {
					if out[p] == nil {
						out[p] = map[addr]bool{}
					}
					out[p][a] = true
				}
			}
		}
	}
	return out
}

// allScopePairs is every pair of scopes the fact store carries, sorted so the
// report reads the same on every run.
func allScopePairs(facts []fact.Fact) []scopePair {
	seen := map[string]bool{}
	for i := range facts {
		seen[facts[i].Scope] = true
	}
	scopes := make([]string, 0, len(seen))
	for s := range seen {
		scopes = append(scopes, s)
	}
	sort.Strings(scopes)
	out := make([]scopePair, 0, len(scopes)*(len(scopes)-1)/2)
	for i := 0; i < len(scopes); i++ {
		for j := i + 1; j < len(scopes); j++ {
			out = append(out, pairOf(scopes[i], scopes[j]))
		}
	}
	return out
}

func countScopes(facts []fact.Fact) int {
	seen := map[string]bool{}
	for i := range facts {
		seen[facts[i].Scope] = true
	}
	return len(seen)
}

// sortedPairs orders a pair-keyed map so findings come out in one order.
func sortedPairs[V any](m map[scopePair]V) []scopePair {
	out := make([]scopePair, 0, len(m))
	for p := range m {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].a != out[j].a {
			return out[i].a < out[j].a
		}
		return out[i].b < out[j].b
	})
	return out
}
