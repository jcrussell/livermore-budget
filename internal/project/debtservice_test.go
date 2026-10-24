package project

import (
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/structure"
)

// debtStore is the committed store and the one Options the debt document
// declares over it.
func debtStore(t *testing.T) ([]fact.Fact, Options) {
	t.Helper()
	store, err := committedStore()
	if err != nil {
		t.Fatal(err)
	}
	opts := (&DebtService{}).Slices(store.facts, "debt test")
	if len(opts) != 1 {
		t.Fatalf("Slices = %d options, want 1", len(opts))
	}
	return store.facts, opts[0]
}

// figures is every figure a document carries, by fact id, with how many
// cells carry it.
func figures(d *DebtServiceDocument) map[string]int {
	out := map[string]int{}
	add := func(p DebtPair) {
		for _, f := range []*DebtFigure{p.Principal, p.Interest} {
			if f != nil {
				out[f.FactID]++
			}
		}
	}
	for _, is := range d.Issues {
		for _, c := range is.Cells {
			add(c)
		}
	}
	for _, p := range d.Requirements.Periods {
		for _, c := range p.Cells {
			add(c)
		}
	}
	return out
}

// TestTheDebtServiceDocumentCitesEveryFigureOnce: every fact of pp.80-81 is
// one cell of the document and no cell is not a fact; a band is a band; a
// cell the page leaves blank is null and a printed dash is a figure of zero.
func TestTheDebtServiceDocumentCitesEveryFigureOnce(t *testing.T) {
	facts, o := debtStore(t)
	d, err := (&DebtService{}).Document(facts, o)
	if err != nil {
		t.Fatalf("Document: %v", err)
	}
	if _, err := (&DebtService{}).Build(facts, o); err != nil {
		t.Fatalf("Build: %v", err)
	}
	byID := map[string]fact.Fact{}
	want := 0
	for _, f := range facts {
		if slices.Contains(DebtServiceScopes(), f.Scope) {
			byID[f.ID] = f
			want++
		}
	}
	got := figures(d)
	if len(got) != want || d.Metadata.Counts.Facts != want || d.Metadata.Counts.Figures != want {
		t.Errorf("the document carries %d distinct figures and counts %+v; the store prints %d",
			len(got), d.Metadata.Counts, want)
	}
	for id, n := range got {
		if n != 1 {
			t.Errorf("fact %s is carried by %d cells", id, n)
		}
		if _, ok := byID[id]; !ok {
			t.Errorf("fact %s is not one of pp.80-81's", id)
		}
	}

	var labels []string
	for _, p := range d.Requirements.Periods {
		labels = append(labels, p.Label)
		band := p.FiscalYearThrough != nil
		if band != strings.Contains(p.Label, "-") || (band && p.Years != 5) || (!band && p.Years != 1) {
			t.Errorf("period %q is %d through %v over %d years", p.Label, p.FiscalYear, p.FiscalYearThrough, p.Years)
		}
	}
	if diff := cmp.Diff([]string{"2026", "2027", "2028", "2029", "2030", "2031-2035",
		"2036-2040", "2041-2045", "2046-2050", "2051-2055"}, labels); diff != "" {
		t.Errorf("periods (-want +got):\n%s", diff)
	}
	// Internal Service prints nothing from 2030: absent, so null.
	is := slices.IndexFunc(d.Requirements.FundTypes, func(f DebtFundType) bool { return f.Key == fundTypeInternalService })
	if c := d.Requirements.Periods[4].Cells[is]; c.Principal != nil || c.Interest != nil {
		t.Errorf("Internal Service 2030 = %+v, want null: the page prints no cell", c)
	}
	// The Interfund Loan prints a dash for FY 2025-26: a figure of zero.
	for _, row := range d.Issues {
		if row.RowLabel == "Interfund Loan" {
			if c := row.Cells[1]; c.Principal == nil || c.Principal.AmountCents != 0 || c.Principal.Token != "-" {
				t.Errorf("Interfund Loan FY 2025-26 = %+v, want the printed dash", c.Principal)
			}
		}
	}

	// Each total is the sum of the facts it cites.
	for _, tot := range d.Requirements.Totals {
		var p, i int64
		for _, id := range tot.FactIDs {
			f := byID[id]
			if f.Category == debtPrincipal {
				p += f.AmountCents
			} else {
				i += f.AmountCents
			}
		}
		if p != tot.PrincipalCents || i != tot.InterestCents {
			t.Errorf("%s totals %d and %d, and its facts sum to %d and %d",
				tot.FundType, tot.PrincipalCents, tot.InterestCents, p, i)
		}
	}
}

// TestTheDebtCaveatsSayWhatTheStoreSays re-measures every figure the caveats
// print off the committed store: the by-issue shortfall against the funds'
// Debt Services, and every requirements cell of 2026 and 2027 that differs
// from its fund type's issues.
//
// Mutation: change debtShortfall's 2026 to 11957700 and the first arm goes
// red; delete a debtRequirementDrift entry and the second does.
func TestTheDebtCaveatsSayWhatTheStoreSays(t *testing.T) {
	store, err := committedStore()
	if err != nil {
		t.Fatal(err)
	}
	issueTotal := map[int]int64{}
	fundsTotal := map[int]int64{}
	type cell struct {
		year           int
		fundType, side string
	}
	budget := map[cell]int64{}
	reqs := map[cell]int64{}
	fundTypeOfIssue := map[string]string{}
	for _, is := range debtIssues {
		fundTypeOfIssue[is.printed] = is.fundType
	}
	for i := range store.facts {
		f := &store.facts[i]
		switch {
		case f.Scope == structure.ScopeDebtServiceByIssue && f.Basis == "adopted":
			issueTotal[f.FiscalYear] += f.AmountCents
			budget[cell{f.FiscalYear, fundTypeOfIssue[f.RowLabel], f.Category}] += f.AmountCents
		case f.Scope == structure.ScopeAllFundsGross && f.Category == "debt-services":
			fundsTotal[f.FiscalYear] += f.AmountCents
		case f.Scope == structure.ScopeDebtServiceRequirements && f.FiscalYear <= 2027 && fact.BandOf(f) == 0:
			reqs[cell{f.FiscalYear, fundTypeOf(f), f.Category}] += f.AmountCents
		}
	}
	var short []yearCents
	for _, y := range []int{2026, 2027} {
		short = append(short, yearCents{y, fundsTotal[y] - issueTotal[y]})
	}
	if diff := cmp.Diff(debtShortfall, short, cmp.AllowUnexported(yearCents{})); diff != "" {
		t.Errorf("debtShortfall against the store (-declared +measured):\n%s", diff)
	}
	if len(reqs) != 16 {
		t.Fatalf("read %d requirement cells for 2026-27, want 16", len(reqs))
	}
	var drift []debtDrift
	for _, y := range []int{2026, 2027} {
		for _, ft := range debtFundTypes {
			for _, side := range []string{debtPrincipal, debtInterest} {
				k := cell{y, ft.key, side}
				if d := reqs[k] - budget[k]; d != 0 {
					drift = append(drift, debtDrift{y, ft.key, side, d})
				}
			}
		}
	}
	if diff := cmp.Diff(debtRequirementDrift, drift, cmp.AllowUnexported(debtDrift{})); diff != "" {
		t.Errorf("debtRequirementDrift against the store (-declared +measured):\n%s", diff)
	}
	t.Logf("measured: shortfall %v; drift %v", short, drift)
}

// TestTheDebtDocumentRefusesARowItCannotPlace holds the issue and fund-type
// tables to the facts in both directions.
//
// Mutation: drop the unknown-label refusal in debtIssueRows and the first
// case builds a row under no activity; spell yearLabel's band with an en dash
// and every band row is refused, so the year-label case's message changes.
func TestTheDebtDocumentRefusesARowItCannotPlace(t *testing.T) {
	facts, o := debtStore(t)
	edit := func(match func(*fact.Fact) bool, change func(*fact.Fact)) []fact.Fact {
		out := slices.Clone(facts)
		for i := range out {
			if match(&out[i]) {
				change(&out[i])
			}
		}
		return out
	}
	lease := func(f *fact.Fact) bool {
		return f.Scope == structure.ScopeDebtServiceByIssue && f.RowLabel == "Lease/Purchase Loan"
	}
	for _, tt := range []struct {
		name  string
		facts []fact.Fact
		want  string
	}{
		{"an issue no entry names",
			edit(lease, func(f *fact.Fact) { f.RowLabel = "Lease/Purchase Loan (restated)" }),
			`prints issue "Lease/Purchase Loan (restated)", which debtIssues does not name`},
		{"an entry no issue prints",
			slices.DeleteFunc(slices.Clone(facts), func(f fact.Fact) bool { return lease(&f) }),
			`debtIssues names "Lease/Purchase Loan" and no fact prints it`},
		{"a fund dimension no fund type names",
			edit(func(f *fact.Fact) bool {
				return f.Scope == structure.ScopeDebtServiceRequirements && f.FundGroup == "internal-service"
			}, func(f *fact.Fact) { f.FundGroup = "general" }),
			`which no requirements fund type names`},
		{"a requirements row whose label is not its column's years",
			edit(func(f *fact.Fact) bool {
				return f.Scope == structure.ScopeDebtServiceRequirements && f.FiscalYearThrough == nil && f.RowLabel == "2027"
			}, func(f *fact.Fact) { f.RowLabel = "2028" }),
			`prints "2028" and is`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := (&DebtService{}).Document(tt.facts, o); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("got %v, want an error containing %q", err, tt.want)
			}
		})
	}
}
