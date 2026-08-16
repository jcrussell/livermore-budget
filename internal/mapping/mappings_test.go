package mapping

import (
	"errors"
	"fmt"
	"sort"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/amount"
)

// These tests read the PUBLISHED rule file — mappings/, not testdata/ — against
// the committed page fixtures, which are byte-identical copies of
// data/extracted/. Everything downstream (facts, the Sankey projection, the
// site) is built from that file, so a defect in it is a defect in a published
// artifact rather than in a test fixture.
const publishedSpine = "../../mappings/livermore-budget-fy2026-2027.yaml"

// The spine's headline figures, in dollars exactly as Budget Book pp.66-67
// print them, so the expectation reads the way the document does.
// resolve_test.go already states two of them (generalFundFY2026Expenditures,
// allFundsFY2026Revenues) and they are reused rather than restated.
const (
	generalFundFY2026Revenues  = 157_873_470
	enterpriseFY2026Revenues   = 67_514_261
	allFundsFY2026Expenditures = 254_095_412
	transfersInFY2026          = 21_525_997
	transfersOutFY2026         = 59_612_734
)

// spineFacts is how many figures the whole file yields: 120 revenue (10 rows
// over 12 columns), 48 expenditure (4 × 12), 12 transfers in, 12 transfers out
// and 48 fund balance (4 rows × 12). The three padded rules read far more cells
// than that — every row of their block is parsed, because a skipped row still
// has to consume its position — and yield facts for one row each. This count is
// the difference between those two things.
const spineFacts = 240

// spineTiedColumns is how many columns CheckTotals reconciles to a total the
// document itself prints: the four parts carrying a total_row, eight columns
// on each p67 part and four on each p66 part. The three padded rules print no
// total of their own and are tied by the balance identity instead.
const spineTiedColumns = 24

// cell identifies one aggregated figure. Category is carried alongside kind
// because the fund-balance rule maps four categories under one kind and the
// balance identity needs them apart: a reserve increase is a use of money in
// the year, while a beginning balance is a stock that must never be summed
// with a flow at all.
type cell struct {
	kind     Kind
	category string
	group    string
	year     int
}

// spineTotals is every figure the rule file yields, summed by cell.
type spineTotals map[cell]amount.Cents

func (s spineTotals) kind(k Kind, group string, year int) amount.Cents {
	var sum amount.Cents
	for c, v := range s {
		if c.kind == k && c.group == group && c.year == year {
			sum += v
		}
	}
	return sum
}

func (s spineTotals) allFunds(k Kind, year int) amount.Cents {
	var sum amount.Cents
	for c, v := range s {
		if c.kind == k && c.year == year {
			sum += v
		}
	}
	return sum
}

func (s spineTotals) category(category, group string, year int) amount.Cents {
	var sum amount.Cents
	for c, v := range s {
		if c.category == category && c.group == group && c.year == year {
			sum += v
		}
	}
	return sum
}

// dollars converts a figure written the way the document prints it into the
// integer cents a fact carries.
func dollars(d int64) amount.Cents { return amount.Cents(d) * 100 }

// spineRead is one full pass over the rule file.
type spineRead struct {
	totals spineTotals
	facts  int
	tied   int
	// unchecked names the parts that declare a total_row but whose stated
	// totals could not be read, as "<rule> p<page>". A part the document prints
	// a total for and we cannot check is not a pass, so it is reported rather
	// than dropped.
	unchecked []string
}

// readSpine resolves every part of every rule in the published file.
func readSpine(t *testing.T) spineRead {
	t.Helper()

	f, err := Load(publishedSpine)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	r, err := NewResolver(budgetDoc(t, 66, 67), f)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}

	out := spineRead{totals: spineTotals{}}
	for i := range f.Rules {
		ru := &f.Rules[i]
		for j := range ru.Parts {
			p := &ru.Parts[j]

			// pp.66-67 print every row of every block. A declaration here would
			// be a claim about what the city printed, and the one time this
			// project made it, it was laundering an extractor defect (fisc-c00).
			if len(p.OmittedRows) != 0 {
				t.Errorf("rule %s p%d declares omitted_rows %q, want none: "+
					"pp.66-67 print every row", ru.ID, p.Page, p.OmittedRows)
			}

			values, omitted, err := r.Values(ru, p)
			if err != nil {
				t.Fatalf("Values(%s, p%d): %v", ru.ID, p.Page, err)
			}
			if len(omitted) != 0 {
				t.Errorf("Values(%s, p%d) reported %d omissions, want 0",
					ru.ID, p.Page, len(omitted))
			}
			out.facts += len(values)
			for _, v := range values {
				out.totals[cell{kind: ru.Kind, category: v.Row.Category,
					group: v.Column.FundGroup, year: v.Column.FiscalYear}] += v.Cents
			}

			if ru.TotalRow == "" {
				continue
			}
			switch err := r.CheckTotals(ru, p); {
			case err == nil:
				out.tied += len(p.Columns)
			case errors.Is(err, ErrNotFound):
				out.unchecked = append(out.unchecked, fmt.Sprintf("%s p%d", ru.ID, p.Page))
			default:
				// A column that does not tie is the failure this file exists to
				// prevent: a row missed, double counted, or read from the wrong
				// column.
				t.Errorf("CheckTotals(%s, p%d): %v", ru.ID, p.Page, err)
			}
		}
	}
	sort.Strings(out.unchecked)
	return out
}

// TestPublishedSpineResolves is the check the rule file exists to pass: every
// part of every rule resolves against the real pages, and every column whose
// printed total can be read reconciles to it exactly. That is the document
// checking our work rather than us checking our own.
func TestPublishedSpineResolves(t *testing.T) {
	got := readSpine(t)

	if want := spineFacts; got.facts != want {
		t.Errorf("read %d values, want %d", got.facts, want)
	}
	if want := spineTiedColumns; got.tied != want {
		t.Errorf("tied %d columns to a stated total, want %d", got.tied, want)
	}
	// Reported, not ignored. A part whose printed total cannot be read is a
	// silently unchecked column, which is the failure this list exists to make
	// visible. It is empty today and must stay that way.
	if len(got.unchecked) != 0 {
		t.Errorf("parts with an unreadable stated total = %q, want none", got.unchecked)
	}
}

func TestPublishedSpineHeadlineFigures(t *testing.T) {
	totals := readSpine(t).totals

	tests := []struct {
		name string
		got  amount.Cents
		want int64
	}{
		{"General Fund FY2025-26 revenues",
			totals.kind(KindRevenue, "general", 2026), generalFundFY2026Revenues},
		{"General Fund FY2025-26 expenditures",
			totals.kind(KindExpenditure, "general", 2026), generalFundFY2026Expenditures},
		{"enterprise FY2025-26 revenues",
			totals.kind(KindRevenue, "enterprise", 2026), enterpriseFY2026Revenues},
		{"all-funds FY2025-26 revenues",
			totals.allFunds(KindRevenue, 2026), allFundsFY2026Revenues},
		{"all-funds FY2025-26 expenditures",
			totals.allFunds(KindExpenditure, 2026), allFundsFY2026Expenditures},
		{"all-funds FY2025-26 transfers in",
			totals.allFunds(KindTransferIn, 2026), transfersInFY2026},
		{"all-funds FY2025-26 transfers out",
			totals.allFunds(KindTransferOut, 2026), transfersOutFY2026},
	}
	for _, tt := range tests {
		if want := dollars(tt.want); tt.got != want {
			t.Errorf("%s = %s, want %s", tt.name, tt.got, want)
		}
	}
}

// TestPublishedSpineBalancesPerFundGroup is the cross-check for everything
// CheckTotals cannot reach: no fund group may spend, transfer out or reserve
// more than it takes in, once transfers in and draws on accumulated balance are
// counted.
//
//	revenue + transfers_in + draw
//	    == expenditure + transfers_out + reserve_increase + contribution
//
// The draw and the contribution are the two halves of the signed
// "CHANGE IN WORKING CAPITAL" row: a negative change is money coming out of
// accumulated balance to fund the year, a positive one is money going into it.
//
// Both sides land on a control total the city itself prints — TOTAL USES for a
// group that draws on balance, TOTAL SOURCES for one that adds to it, and they
// are equal where the change is zero. Those rows are mapped by no rule, which
// is what makes this a cross-check rather than a restatement of the figures
// being checked. It covers all six fund groups in both budget years, so it
// reaches the eight p67 expenditure columns CheckTotals cannot.
func TestPublishedSpineBalancesPerFundGroup(t *testing.T) {
	totals := readSpine(t).totals

	tests := []struct {
		group string
		// The greater of the group's printed TOTAL SOURCES and TOTAL USES, in
		// dollars, for FY2025-26 and FY2026-27.
		fy2026, fy2027 int64
	}{
		{"general", 159_388_024, 164_844_882},
		{"enterprise", 80_761_261, 83_638_250},
		{"capital", 29_646_095, 37_017_670},
		{"debt-service", 6_984_597, 6_969_898},
		{"special-revenue", 29_279_560, 21_730_522},
		{"internal-service", 25_117_367, 27_156_515},
	}
	for _, tt := range tests {
		for _, y := range []struct {
			year int
			want int64
		}{{2026, tt.fy2026}, {2027, tt.fy2027}} {
			year, want := y.year, y.want
			t.Run(fmt.Sprintf("%s/%d", tt.group, year), func(t *testing.T) {
				change := totals.category("fund-balance/change", tt.group, year)
				sources := totals.kind(KindRevenue, tt.group, year) +
					totals.kind(KindTransferIn, tt.group, year) +
					max(0, -change)
				uses := totals.kind(KindExpenditure, tt.group, year) +
					totals.kind(KindTransferOut, tt.group, year) +
					totals.category("fund-balance/reserve-increase", tt.group, year) +
					max(0, change)

				if sources != uses {
					t.Errorf("sources %s, uses %s: off by %s", sources, uses, sources-uses)
				}
				if got, w := sources, dollars(want); got != w {
					t.Errorf("sources = %s, want %s (the printed control total)", got, w)
				}
				if got, w := uses, dollars(want); got != w {
					t.Errorf("uses = %s, want %s (the printed control total)", got, w)
				}
			})
		}
	}
}

// TestPublishedSpineDoesNotDoubleCount guards the four subtotals the schedule
// prints and no rule may map. They are the sums the mapped rows are checked
// against; mapping one alongside its own components would double every dollar
// in the column while every stated total still tied.
func TestPublishedSpineDoesNotDoubleCount(t *testing.T) {
	f, err := Load(publishedSpine)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	subtotals := map[string]bool{
		"TOTAL REVENUES:": true, "TOTAL EXPENDITURES:": true,
		"TOTAL SOURCES": true, "TOTAL USES:": true,
	}
	for i := range f.Rules {
		ru := &f.Rules[i]
		for _, row := range ru.Rows {
			if subtotals[row.Label] && !row.Skip {
				t.Errorf("rule %s maps the subtotal row %q to category %q, want skip: true",
					ru.ID, row.Label, row.Category)
			}
		}
	}
}

// TestPublishedSpineMatchesTheSankeyContract checks the seam with
// docs/sankey-contract.md. Its "Row-to-slug map" is the interface between this
// file and internal/project, which were written against the table and not
// against each other, so a slug renamed on one side has to fail somewhere.
func TestPublishedSpineMatchesTheSankeyContract(t *testing.T) {
	totals := readSpine(t).totals

	want := map[string]Kind{
		"taxes/property":                KindRevenue,
		"taxes/other":                   KindRevenue,
		"taxes/sales":                   KindRevenue,
		"intergovernmental":             KindRevenue,
		"charges-for-services":          KindRevenue,
		"use-of-money-and-property":     KindRevenue,
		"contributions-outsourced":      KindRevenue,
		"miscellaneous-revenue":         KindRevenue,
		"fines-and-forfeitures":         KindRevenue,
		"licenses-and-permits":          KindRevenue,
		"wages-and-benefits":            KindExpenditure,
		"services-and-supplies":         KindExpenditure,
		"capital-outlay":                KindExpenditure,
		"debt-services":                 KindExpenditure,
		"transfers/in":                  KindTransferIn,
		"transfers/out":                 KindTransferOut,
		"fund-balance/reserve-increase": KindFundBalance,
		"fund-balance/change":           KindFundBalance,
		"fund-balance/beginning":        KindFundBalance,
		"fund-balance/ending":           KindFundBalance,
	}

	got := map[string]Kind{}
	for c := range totals {
		if prev, seen := got[c.category]; seen && prev != c.kind {
			t.Errorf("category %q is mapped as both %q and %q", c.category, prev, c.kind)
		}
		got[c.category] = c.kind
	}
	for category, k := range want {
		switch gotKind, ok := got[category]; {
		case !ok:
			t.Errorf("no rule produces category %q, which the sankey contract expects", category)
		case gotKind != k:
			t.Errorf("category %q has kind %q, want %q", category, gotKind, k)
		}
	}
	for category := range got {
		if _, ok := want[category]; !ok {
			t.Errorf("category %q is not in the sankey contract's row-to-slug map", category)
		}
	}
}
