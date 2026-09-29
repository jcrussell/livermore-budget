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

// publishedSpineScope is the scope pp.66-67's rules carry. Spelled here rather
// than imported: internal/project declares it and imports this package, so the
// dependency cannot run the other way.
//
// A test that means "the citywide spine" must say so with the SCOPE and never
// with the kind. That was indistinguishable while one schedule was mapped and
// stopped being so when fisc-5gk.1 published four transfers-only fund rules on
// p134: a filter on KindTransferIn alone swept them into a read of pp.66-67.
const publishedSpineScope = "all-funds-gross"

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
		// The published file now carries a second schedule (pp.167-170, at
		// scope expenditure-by-department), and every assertion below is about
		// the SPINE: its fact count, its tied columns, its headline figures.
		// Filtering by scope here rather than by rule id keeps this file's
		// claims true as more schedules land, and it is the same string the
		// projection selects on.
		if ru.Scope != publishedSpineScope {
			continue
		}
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
			switch res, err := r.CheckTotals(ru, p); {
			case err == nil:
				// Counted from what the check compared rather than from the
				// part, so this number and the one `fisc build` reports are
				// the same number.
				out.tied += res.Columns
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

// TestPublishedPartsDeclareColumnHeaders is the ratchet on an opt-in guard.
//
// A part with no column_headers is read exactly as it was before geometry
// existed: its figures are placed by position alone, and nothing anywhere says
// so. That is the same silent degradation the resolver refuses when it requires
// its document to answer for geometry rather than discovering the capability by
// type assertion, and without this assertion the next part someone adds inherits
// it by default.
func TestPublishedPartsDeclareColumnHeaders(t *testing.T) {
	f, err := Load(publishedSpine)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	parts := 0
	for i := range f.Rules {
		ru := &f.Rules[i]
		for j := range ru.Parts {
			p := &ru.Parts[j]
			parts++
			if len(p.ColumnHeaders) == 0 {
				t.Errorf("rule %s p%d declares no column_headers, so its figures are "+
					"placed by position alone", ru.ID, p.Page)
				continue
			}
			if got, want := len(p.ColumnHeaders), len(p.Columns); got != want {
				t.Errorf("rule %s p%d declares %d column_headers for %d columns",
					ru.ID, p.Page, got, want)
			}
		}
	}
	// 10 spine parts over pp.66-67; 26 division parts over pp.167-170, of which
	// three divisions declare two because they straddle a page break; 84
	// revenue parts over pp.127-140, of which five declare two because either
	// their rows or their printed total is on the far side of a page break;
	// 5 transfer parts on p76, one per printed section -- the first parts in
	// this file to declare a column the page prints no header over, which they
	// spell as a trailing null (fisc-wfi); and 14 funding-source parts over
	// pp.85-125, one per page, of which three departments declare two because
	// their fund rows and their printed total straddle a page break; and 29
	// departmentwide parts, one per division block, on the ELEVEN of those same
	// fourteen pages that carry an Expenditures by Category block. Those last
	// two groups declare the same four headers over the same pages because
	// checkColumnGrids is per (doc_id, page) and all-or-none, so a division part
	// omitting the list would be a hard parse error rather than a weaker read.
	// And p222, the CIP funding bridge, one part; and 66 fund parts over
	// pp.172-183, one per fund, of which three declare two because their rows
	// straddle a page break.
	if want := 235; parts != want {
		t.Errorf("checked %d parts, want %d; the published file's shape changed",
			parts, want)
	}
}
