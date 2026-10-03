package fact

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/google/go-cmp/cmp"

	"github.com/jcrussell/livermore-budget/internal/amount"
	"github.com/jcrussell/livermore-budget/internal/corpus"
	"github.com/jcrussell/livermore-budget/internal/mapping"
)

// budgetDoc builds an extraction of the Budget Book holding only the pages a
// test names, from the committed fixtures.
func budgetDoc(t *testing.T, pages ...int) *corpus.Doc {
	t.Helper()
	fsys := fstest.MapFS{}
	artifacts := map[string]corpus.Artifact{}
	for _, p := range pages {
		body, err := os.ReadFile(fmt.Sprintf("../../testdata/pages/budget-p%04d.txt", p))
		if err != nil {
			t.Fatalf("read page fixture: %v", err)
		}
		fsys[corpus.PagePath(p)] = &fstest.MapFile{Data: body}
		artifacts[corpus.PagePath(p)] = corpus.Artifact{Bytes: int64(len(body))}

		// The second substrate, where the fixture tree has it. A part that
		// declares column_headers is read against geometry as well as text, so
		// a document without it cannot carry the column guard -- and p76's
		// fixture declares them since fisc-wfi.
		geo, err := os.ReadFile(fmt.Sprintf("../../testdata/geometry/budget-p%04d.json", p))
		if err != nil {
			continue
		}
		fsys[corpus.GeometryPath(p)] = &fstest.MapFile{Data: geo}
		artifacts[corpus.GeometryPath(p)] = corpus.Artifact{Bytes: int64(len(geo))}
	}
	man, err := json.Marshal(map[string]any{
		"schema_version": corpus.SchemaVersion,
		"doc_id":         "livermore-budget-fy2026-2027",
		"artifacts":      artifacts,
	})
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	fsys["manifest.json"] = &fstest.MapFile{Data: man}

	d, err := corpus.Open(fsys)
	if err != nil {
		t.Fatalf("corpus.Open: %v", err)
	}
	return d
}

// factsFrom builds every fact a rule file yields over the pages named, from
// the committed page fixtures.
func factsFrom(t *testing.T, rules string, pages ...int) []Fact {
	t.Helper()

	d := budgetDoc(t, pages...)
	f, err := mapping.Load(rules)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	r, err := mapping.NewResolver(d, f)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}

	var facts []Fact
	for i := range f.Rules {
		rule := &f.Rules[i]
		for j := range rule.Parts {
			values, _, err := r.Values(rule, &rule.Parts[j])
			if err != nil {
				t.Fatalf("Values(%s, p%d): %v", rule.ID, rule.Parts[j].Page, err)
			}
			got, err := FromValues(f, rule, values)
			if err != nil {
				t.Fatalf("FromValues: %v", err)
			}
			facts = append(facts, got...)
		}
	}
	return facts
}

// spineFacts builds every fact the citywide spine yields. It is the input to
// most of these tests, because a fact model checked only against invented rows
// proves nothing about the corpus.
func spineFacts(t *testing.T) []Fact {
	t.Helper()
	return factsFrom(t, "../mapping/testdata/spine.yaml", 66, 67)
}

// TestSpineFacts checks the model against the real schedule: every figure the
// resolver reads becomes exactly one addressable fact, and the arithmetic
// survives the trip through the fact record.
func TestSpineFacts(t *testing.T) {
	facts := spineFacts(t)

	if want := 136; len(facts) != want {
		t.Errorf("got %d facts, want %d", len(facts), want)
	}
	if err := CheckUniqueIDs(facts); err != nil {
		t.Errorf("CheckUniqueIDs: %v", err)
	}

	// The published figures must survive being turned into facts.
	var gfExpenditure, allFundsRevenue int64
	for _, f := range facts {
		if f.FiscalYear != 2026 {
			continue
		}
		switch {
		case f.Kind == mapping.KindExpenditure && f.FundGroup == "general":
			gfExpenditure += f.AmountCents
		case f.Kind == mapping.KindRevenue:
			allFundsRevenue += f.AmountCents
		}
	}
	if want := int64(144_650_802 * 100); gfExpenditure != want {
		t.Errorf("General Fund FY2025-26 expenditures = %s, want %s",
			amount.Cents(gfExpenditure), amount.Cents(want))
	}
	if want := int64(299_969_007 * 100); allFundsRevenue != want {
		t.Errorf("all-funds FY2025-26 revenues = %s, want %s",
			amount.Cents(allFundsRevenue), amount.Cents(want))
	}

	// Every fact must be addressable and carry its provenance.
	for _, f := range facts {
		if !strings.HasPrefix(f.ID, IDPrefix) || len(f.ID) != len(IDPrefix)+idHexLen {
			t.Errorf("id %q is not a well-formed fact id", f.ID)
		}
		if f.Page == 0 || f.Token == "" || f.RowPath == "" || f.ColumnPath == "" {
			t.Errorf("fact %s is missing provenance: %+v", f.ID, f)
		}
		if f.Sign == "" {
			t.Errorf("fact %s has an empty sign; it should be normalized", f.ID)
		}
		if f.Derived {
			t.Errorf("fact %s is marked derived, but every spine fact is published", f.ID)
		}
	}

	// p67 prints "Licenses & Permits" as a row of dashes in all four of its
	// fund groups, so it must produce eight facts worth zero — not zero facts,
	// and not facts with some other row's label. This assertion used to read
	// the other way round, because the extractor was deleting the row and the
	// rule declared it omitted to compensate (fisc-c00). Zero is a figure the
	// city printed; absent is not zero, and neither is it a licence to guess.
	n := 0
	for _, f := range facts {
		if f.RowLabel == "Licenses & Permits" && f.Page == 67 {
			n++
			if f.AmountCents != 0 {
				t.Errorf("fact %s = %s, want 0", f.ID, amount.Cents(f.AmountCents))
			}
		}
	}
	if want := 8; n != want {
		t.Errorf("got %d p67 Licenses & Permits facts, want %d", n, want)
	}
}

// TestOffsetPointsAtTheToken is what makes a citation worth printing. A page
// number alone sends a reader to a page of numbers; the offset must land on
// the figure itself, and nothing else in the pipeline checks that it does.
func TestOffsetPointsAtTheToken(t *testing.T) {
	pages := map[int]string{}
	for _, p := range []int{66, 67} {
		b, err := os.ReadFile(fmt.Sprintf("../../testdata/pages/budget-p%04d.txt", p))
		if err != nil {
			t.Fatalf("read page fixture: %v", err)
		}
		pages[p] = string(b)
	}

	for _, f := range spineFacts(t) {
		text := pages[f.Page]
		if f.Offset < 0 || f.Offset+len(f.Token) > len(text) {
			t.Fatalf("fact %s offset %d is outside p%d", f.ID, f.Offset, f.Page)
		}
		if got := text[f.Offset : f.Offset+len(f.Token)]; got != f.Token {
			t.Errorf("fact %s: p%d at offset %d is %q, but the fact carries token %q",
				f.ID, f.Page, f.Offset, got, f.Token)
		}
	}
}

// propertyTaxRule maps the General Fund property-tax detail on Budget Book
// p127. It is inline rather than a fixture because it exists to exercise two
// shapes the spine does not have: thirteen rows that all classify as
// taxes/property, and rows the city prints parenthesized.
func propertyTaxRule(t *testing.T) (*mapping.File, *mapping.Rule) {
	t.Helper()
	rows := []mapping.Row{
		{Label: "Current Year - Secured", Category: "taxes/property"},
		{Label: "Prior Year - Secured", Category: "taxes/property"},
		{Label: "ERAF", Category: "taxes/property", Sign: mapping.SignContra},
		{Label: "RPTTF Reduction", Category: "taxes/property", Sign: mapping.SignContra},
		{Label: "Current Year - Unsecured", Category: "taxes/property"},
		{Label: "Prior Year - Unsecured", Category: "taxes/property"},
		{Label: "Supple - Sec Roll Current", Category: "taxes/property"},
		{Label: "VLF Comp Fund", Category: "taxes/property"},
		{Label: "Unitary Utility Tax", Category: "taxes/property"},
		{Label: "Aircraft Taxes", Category: "taxes/property"},
		{Label: "RPTTF Receipts & Other PropTax", Category: "taxes/property"},
		{Label: "St Homeowner Prop Tax Re", Category: "taxes/property"},
		{Label: "Pen & Int On Delinq Tax", Category: "taxes/property"},
	}
	f := &mapping.File{
		SchemaVersion: mapping.SchemaVersion,
		DocID:         "livermore-budget-fy2026-2027",
		Path:          "inline.yaml",
		Rules: []mapping.Rule{{
			ID:   "gf-property-tax-detail",
			Kind: mapping.KindRevenue, Basis: mapping.BasisAdopted,
			Scope: "general-fund", Grain: "fund-group-by-category", Units: amount.Dollars,
			TotalRow: "Total Property Taxes",
			Rows:     rows,
			Parts: []mapping.Part{{
				Page: 127,
				// The trailing newline is load-bearing. "Property Taxes" is
				// also the tail of "Total Property Taxes", which p127 prints
				// below this block; the city prints the section heading alone
				// on its line and the total with its four figures beside it, so
				// requiring the line break is what makes this anchor unique and
				// lets it resolve with no ordinal at all. There are no emphasis
				// markers to anchor on: `pdftotext -layout` reproduces the
				// printed page, and the "**" this rule used to carry was xberg's
				// markdown, not the document's.
				Section: "Property Taxes\n",
				StopAt:  "Total Property Taxes",
				// This schedule prints four years on three bases side by side.
				Columns: []mapping.Column{
					{FundGroup: "general", FiscalYear: 2024, Basis: mapping.BasisActual},
					{FundGroup: "general", FiscalYear: 2025, Basis: mapping.BasisRevised},
					{FundGroup: "general", FiscalYear: 2026, Basis: mapping.BasisAdopted},
					{FundGroup: "general", FiscalYear: 2027, Basis: mapping.BasisAdopted},
				},
			}},
		}},
	}
	if err := f.Validate(); err != nil {
		t.Fatalf("the inline rule is itself invalid: %v", err)
	}
	return f, &f.Rules[0]
}

// TestContraRowsAndSharedCategories is the case the spine cannot exercise.
//
// Thirteen rows here all classify as taxes/property, so an id built from the
// classification alone would give all thirteen the same identity. Two of them
// are printed in parentheses, so they must arrive already negative — the
// arithmetic only closes with the sign kept.
func TestContraRowsAndSharedCategories(t *testing.T) {
	d := budgetDoc(t, 127)
	f, rule := propertyTaxRule(t)
	r, err := mapping.NewResolver(d, f)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	values, _, err := r.Values(rule, &rule.Parts[0])
	if err != nil {
		t.Fatalf("Values: %v", err)
	}
	facts, err := FromValues(f, rule, values)
	if err != nil {
		t.Fatalf("FromValues: %v", err)
	}

	if want := 13 * 4; len(facts) != want {
		t.Fatalf("got %d facts, want %d (13 rows × 4 columns)", len(facts), want)
	}
	// Thirteen rows, one category: the ids must still all differ.
	if dupe := CheckUniqueIDs(facts); dupe != nil {
		t.Errorf("CheckUniqueIDs on rows sharing a category: %v", dupe)
	}
	for _, f := range facts {
		if f.RowPath != "taxes/property" {
			t.Fatalf("fact %s has row_path %q; the fixture puts every row under one category",
				f.ID, f.RowPath)
		}
	}

	// The contra rows are negative as printed, and nothing re-signs them.
	contra := map[string]int64{"ERAF": -14_086_438_00, "RPTTF Reduction": -1_749_120_00}
	for _, f := range facts {
		if f.FiscalYear != 2024 {
			continue
		}
		want, isContra := contra[f.RowLabel]
		if !isContra {
			continue
		}
		if f.Sign != mapping.SignContra {
			t.Errorf("fact %s (%s) has sign %q, want contra", f.ID, f.RowLabel, f.Sign)
		}
		if f.AmountCents != want {
			t.Errorf("%s FY2023-24 = %s, want %s (as printed, parentheses kept)",
				f.RowLabel, amount.Cents(f.AmountCents), amount.Cents(want))
		}
	}

	// Basis comes from the column, not the rule, on this schedule.
	byYear := map[int]mapping.Basis{}
	sums := map[int]int64{}
	for _, f := range facts {
		byYear[f.FiscalYear] = f.Basis
		sums[f.FiscalYear] += f.AmountCents
	}
	for year, want := range map[int]mapping.Basis{
		2024: mapping.BasisActual, 2025: mapping.BasisRevised,
		2026: mapping.BasisAdopted, 2027: mapping.BasisAdopted,
	} {
		if byYear[year] != want {
			t.Errorf("FY%d basis = %q, want %q", year, byYear[year], want)
		}
	}

	// The arithmetic only closes with the parentheses read as negative:
	// ignoring them gives 98,114,440 for FY2025-26 rather than 64,143,762.
	if want := int64(64_143_762_00); sums[2026] != want {
		t.Errorf("FY2025-26 property taxes = %s, want %s (the p66 spine figure)",
			amount.Cents(sums[2026]), amount.Cents(want))
	}

	// And the known document rounding: this column is off by one dollar from
	// the total the city prints, on a correct read. See bead fisc-2sd; the
	// other three columns tie exactly.
	stated, err := r.StatedTotals(rule, &rule.Parts[0])
	if err != nil {
		t.Fatalf("StatedTotals: %v", err)
	}
	if got, want := sums[2024], int64(58_179_467_00); got != want {
		t.Errorf("FY2023-24 property taxes = %s, want %s",
			amount.Cents(got), amount.Cents(want))
	}
	if got, want := int64(stated[0]), int64(58_179_468_00); got != want {
		t.Errorf("p127 states FY2023-24 total %s, want %s", amount.Cents(got), amount.Cents(want))
	}
	if diff := int64(stated[0]) - sums[2024]; diff != 100 {
		t.Errorf("the FY2023-24 discrepancy is %s; it was one dollar of the city's "+
			"own rounding when this was written", amount.Cents(diff))
	}
	for c := 1; c < 4; c++ {
		year := []int{0, 2025, 2026, 2027}[c]
		if int64(stated[c]) != sums[year] {
			t.Errorf("FY%d: mapped %s, document states %s — this column used to tie exactly",
				year, amount.Cents(sums[year]), stated[c])
		}
	}
}

// TestIDIsIdentityNotContent is the property the whole model rests on: a
// corrected figure must be a modified line, and an inserted row must not
// renumber anything.
func TestIDIsIdentityNotContent(t *testing.T) {
	id := MakeID("doc", "rule", "taxes/property", "Property Taxes", "general", 2026, mapping.BasisAdopted)

	t.Run("stable across a corrected amount", func(t *testing.T) {
		// The amount is not an input at all, so this is really a statement
		// about the signature: if a future change adds it, this test is where
		// the intent is written down.
		again := MakeID("doc", "rule", "taxes/property", "Property Taxes", "general", 2026, mapping.BasisAdopted)
		if again != id {
			t.Errorf("MakeID is not deterministic: %s then %s", id, again)
		}
	})

	t.Run("distinct on every identity component", func(t *testing.T) {
		seen := map[string]string{id: "the original"}
		for name, other := range map[string]string{
			"doc":         MakeID("other", "rule", "taxes/property", "Property Taxes", "general", 2026, mapping.BasisAdopted),
			"rule":        MakeID("doc", "other", "taxes/property", "Property Taxes", "general", 2026, mapping.BasisAdopted),
			"row path":    MakeID("doc", "rule", "taxes/other", "Property Taxes", "general", 2026, mapping.BasisAdopted),
			"column path": MakeID("doc", "rule", "taxes/property", "Property Taxes", "enterprise", 2026, mapping.BasisAdopted),
			"fiscal year": MakeID("doc", "rule", "taxes/property", "Property Taxes", "general", 2027, mapping.BasisAdopted),
			"basis":       MakeID("doc", "rule", "taxes/property", "Property Taxes", "general", 2026, mapping.BasisRevised),
			"row label":   MakeID("doc", "rule", "taxes/property", "Prior Year - Secured", "general", 2026, mapping.BasisAdopted),
		} {
			if prev, dup := seen[other]; dup {
				t.Errorf("changing the %s collides with %s", name, prev)
			}
			seen[other] = name
		}
	})

	t.Run("component boundaries cannot be shifted", func(t *testing.T) {
		// Joining on a character that occurs in the data would make these two
		// hash identically. row_path routinely contains "/" and doc ids
		// contain "-", so this is not hypothetical.
		a := MakeID("doc", "rule", "a/b", "L", "c", 2026, mapping.BasisAdopted)
		b := MakeID("doc", "rule", "a", "L", "b/c", 2026, mapping.BasisAdopted)
		if a == b {
			t.Errorf("component boundary is ambiguous: both hash to %s", a)
		}
	})
}

// TestTwoAnchorRowsGetDistinctIDs is fisc-28h, against the page it was found
// on. Budget Book p76 prints two rows that share a source fund and differ only
// in their destination, both in rule p76-transfers-in-enterprise-a:
//
//	Transfer From Wastewater   to Stormwater          ...
//	Transfer From Wastewater   to LAVWMA / Wastewater ...
//
// Hashing the bare Row.Label gave both of them one id, so the id was keyed on
// a string that is not the row_label the fact publishes. The id is built from
// PrintedLabel() for exactly this reason: it is what the record says, and a
// content address a reader cannot recompute from the record is not an
// address.
func TestTwoAnchorRowsGetDistinctIDs(t *testing.T) {
	facts := factsFrom(t, "../mapping/testdata/transfers-p76.yaml", 76)
	if len(facts) == 0 {
		t.Fatal("p76 produced no facts")
	}
	if err := CheckUniqueIDs(facts); err != nil {
		t.Fatalf("p76 facts collide: %v", err)
	}

	// The confirmed pair, both in rule p76-transfers-in-enterprise-a.
	const shared = "Transfer From Wastewater"
	byLabel := map[string][]Fact{}
	for _, f := range facts {
		if f.FiscalYear == 2026 && strings.HasPrefix(f.RowLabel, shared) {
			byLabel[f.RowLabel] = append(byLabel[f.RowLabel], f)
		}
	}
	if len(byLabel) != 2 {
		t.Fatalf("got %d FY2026 row labels beginning %q, want 2: %v",
			len(byLabel), shared, byLabel)
	}
	seen := map[string]string{}
	for label, fs := range byLabel {
		if len(fs) != 1 {
			t.Fatalf("row label %q claims %d FY2026 facts, want 1", label, len(fs))
		}
		if prev, dup := seen[fs[0].ID]; dup {
			t.Errorf("rows %q and %q both hash to %s", prev, label, fs[0].ID)
		}
		seen[fs[0].ID] = label
	}
}

// TestIDIsBuiltFromTheLabelTheFactPublishes states the fisc-28h decision as a
// property rather than a page: whatever the id is hashed from must be
// recoverable from the published record. row_label is that field.
func TestIDIsBuiltFromTheLabelTheFactPublishes(t *testing.T) {
	row := mapping.Row{Label: "Transfer From Wastewater",
		LabelTail: "to Stormwater", Category: "transfers/in"}
	f := &mapping.File{DocID: "doc"}
	rule := &mapping.Rule{ID: "rule", Basis: mapping.BasisAdopted, Scope: "s"}
	got, err := FromValues(f, rule, []mapping.Value{{
		Row: row, Column: mapping.Column{FundGroup: "enterprise", FiscalYear: 2026},
	}})
	if err != nil {
		t.Fatalf("FromValues: %v", err)
	}
	want := MakeID("doc", "rule", RowPath(row), got[0].RowLabel,
		"enterprise", 2026, mapping.BasisAdopted)
	if got[0].ID != want {
		t.Errorf("id = %s, want %s: the id must be a function of the published "+
			"row_label %q", got[0].ID, want, got[0].RowLabel)
	}
	if got[0].RowLabel != row.PrintedLabel() {
		t.Errorf("row_label = %q, want %q", got[0].RowLabel, row.PrintedLabel())
	}
}

func TestRowAndColumnPaths(t *testing.T) {
	tests := []struct {
		name string
		row  mapping.Row
		col  mapping.Column
		// scope is the rule's, used only when the column has no fund dimension.
		scope   string
		wantRow string
		wantCol string
	}{
		{
			name:    "category only",
			row:     mapping.Row{Label: "Property Taxes", Category: "taxes/property"},
			col:     mapping.Column{FundGroup: "general", FiscalYear: 2026},
			wantRow: "taxes/property", wantCol: "general",
		},
		{
			// A fact is both "Police" and "wages and benefits"; the department
			// leads because it is the coarser axis.
			name:    "department and category",
			row:     mapping.Row{Label: "Wages", Category: "wages-and-benefits", Department: "police"},
			col:     mapping.Column{Fund: 601, FiscalYear: 2026},
			wantRow: "police/wages-and-benefits", wantCol: "fund/601",
		},
		{
			name:    "department only",
			row:     mapping.Row{Label: "Police", Department: "police"},
			col:     mapping.Column{FundGroup: "enterprise", Fund: 601, FiscalYear: 2026},
			wantRow: "police", wantCol: "enterprise/fund/601",
		},
		{
			name:    "column with no fund dimension falls back to the rule's scope",
			row:     mapping.Row{Label: "A", Category: "a"},
			col:     mapping.Column{FiscalYear: 2026},
			scope:   "all-funds-gross",
			wantRow: "a", wantCol: "all-funds-gross",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RowPath(tt.row); got != tt.wantRow {
				t.Errorf("RowPath() = %q, want %q", got, tt.wantRow)
			}
			if got := ColumnPath(tt.col, tt.scope); got != tt.wantCol {
				t.Errorf("ColumnPath() = %q, want %q", got, tt.wantCol)
			}
		})
	}
}

func TestSortIsCanonicalAndTotal(t *testing.T) {
	facts := spineFacts(t)

	Sort(facts)
	if err := CheckSorted(facts); err != nil {
		t.Fatalf("CheckSorted after Sort: %v", err)
	}

	// Sorting a shuffled copy must reproduce the same sequence exactly. An
	// order with ties would pass CheckSorted while still emitting two
	// different files, which is the nondeterminism the check exists to catch.
	shuffled := append([]Fact(nil), facts...)
	for i := range shuffled {
		j := (i * 7) % len(shuffled) // a fixed permutation; tests must not be random
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	}
	Sort(shuffled)
	if diff := cmp.Diff(facts, shuffled); diff != "" {
		t.Errorf("sorting a permutation gives a different order (-want +got):\n%s", diff)
	}
}

func TestCheckSortedReportsTheOffendingPair(t *testing.T) {
	facts := spineFacts(t)
	Sort(facts)
	facts[3], facts[9] = facts[9], facts[3]

	err := CheckSorted(facts)
	if err == nil {
		t.Fatal("CheckSorted on out-of-order facts = nil, want an error")
	}
	if !strings.Contains(err.Error(), "out of order") {
		t.Errorf("error = %q, want it to say the facts are out of order", err)
	}
}

func TestCheckUniqueIDsRejectsTwoClaimsOnOneCell(t *testing.T) {
	// Two rules reading the same cell: the second is double-counting.
	a := Fact{ID: "fisc-f-abc", DocID: "d", RuleID: "spine", Page: 66,
		RowPath: "taxes/property", ColumnPath: "general", FiscalYear: 2026,
		Basis: mapping.BasisAdopted, AmountCents: 100}
	b := a
	b.RuleID = "detail"
	b.Page = 127
	b.AmountCents = 200

	err := CheckUniqueIDs([]Fact{a, b})
	if err == nil {
		t.Fatal("CheckUniqueIDs on a collision = nil, want an error")
	}
	for _, want := range []string{"claimed twice", "spine", "detail", "$1.00", "$2.00"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not contain %q", err, want)
		}
	}
}

// TestWriteIsDeterministicJSONL pins the wire format: the same facts must
// produce byte-identical output, every line must carry every key in the
// declared order, and the file must end in a newline.
func TestWriteIsDeterministicJSONL(t *testing.T) {
	facts := spineFacts(t)
	Sort(facts)

	var first, second bytes.Buffer
	if err := Write(&first, facts); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := Write(&second, facts); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if !bytes.Equal(first.Bytes(), second.Bytes()) {
		t.Fatal("two writes of the same facts differ")
	}

	out := first.String()
	if !strings.HasSuffix(out, "\n") {
		t.Error("output does not end in a newline")
	}
	if strings.Contains(out, "\r") {
		t.Error("output contains a carriage return; endings must be LF")
	}
	if strings.Contains(out, `\u0026`) {
		t.Error(`output HTML-escapes "&"; "Fines & Forfeitures" must stay readable`)
	}
	if !strings.Contains(out, "Fines & Forfeitures") {
		t.Error(`no line carries a raw "Fines & Forfeitures"`)
	}

	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) != len(facts) {
		t.Fatalf("wrote %d lines for %d facts", len(lines), len(facts))
	}

	// Every line carries the same keys, in the same order, with none dropped
	// for being empty -- "department" is empty on every spine fact and must
	// still appear.
	wantKeys := jsonKeys(t, lines[0])
	if !contains(wantKeys, "department") {
		t.Errorf("keys %v do not include the empty-valued department", wantKeys)
	}
	for i, line := range lines {
		if diff := cmp.Diff(wantKeys, jsonKeys(t, line)); diff != "" {
			t.Fatalf("line %d has different keys (-want +got):\n%s", i+1, diff)
		}
	}
}

func TestReadRoundTrips(t *testing.T) {
	facts := spineFacts(t)
	Sort(facts)

	var buf bytes.Buffer
	if err := Write(&buf, facts); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got, err := Read(&buf)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if diff := cmp.Diff(facts, got); diff != "" {
		t.Errorf("round trip changed the facts (-want +got):\n%s", diff)
	}
}

func TestReadRejectsAnUnknownKey(t *testing.T) {
	// A file written by a different fisc must not half-load: the fields that
	// happened to survive would pass a rebuild-and-diff check.
	_, err := Read(strings.NewReader(`{"id":"fisc-f-abc","surprise":1}` + "\n"))
	if err == nil {
		t.Fatal("Read with an unknown key = nil error, want a failure")
	}
	if !strings.Contains(err.Error(), "surprise") {
		t.Errorf("error = %q, want it to name the unknown key", err)
	}
}

func jsonKeys(t *testing.T, line string) []string {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(line))
	tok, err := dec.Token()
	if err != nil || tok != json.Delim('{') {
		t.Fatalf("line is not a JSON object: %q", line)
	}
	var keys []string
	for dec.More() {
		k, err := dec.Token()
		if err != nil {
			t.Fatalf("read key: %v", err)
		}
		keys = append(keys, k.(string))
		// Skip the value, whatever shape it is.
		var v any
		if err := dec.Decode(&v); err != nil {
			t.Fatalf("read value for %v: %v", k, err)
		}
	}
	return keys
}

func contains(s []string, want string) bool {
	for _, v := range s {
		if v == want {
			return true
		}
	}
	return false
}

// TestARowKindOverridesTheRulesKind pins mapping.Row.Kind reaching the
// published record, which is the only place the override is observable.
//
// It lives here rather than in internal/mapping because that package cannot
// import this one, and a fact is what the override is FOR: eleven-plus mixed
// fund blocks on Budget Book pp.131-140 print revenue rows and a Transfers In
// row inside one printed total, and the transfer's facts must say transfer_in
// while the rule says revenue.
func TestARowKindOverridesTheRulesKind(t *testing.T) {
	f := &mapping.File{DocID: "doc"}
	rule := &mapping.Rule{ID: "stormwater", Kind: mapping.KindRevenue,
		Basis: mapping.BasisAdopted, Scope: "all-funds-gross"}
	col := mapping.Column{FundGroup: "enterprise", FiscalYear: 2026}

	got, err := FromValues(f, rule, []mapping.Value{
		{Row: mapping.Row{Label: "Charges for Services", Category: "charges-for-services"}, Column: col},
		{Row: mapping.Row{Label: "Transfers In", Category: "transfers/in",
			Kind: mapping.KindTransferIn}, Column: col},
	})
	if err != nil {
		t.Fatalf("FromValues: %v", err)
	}
	if got[0].Kind != mapping.KindRevenue {
		t.Errorf("row with no kind = %q, want the rule's %q", got[0].Kind, mapping.KindRevenue)
	}
	if got[1].Kind != mapping.KindTransferIn {
		t.Errorf("row declaring transfer_in = %q, want its own kind; without this the "+
			"transfer is published as revenue and double-counts the fund's income",
			got[1].Kind)
	}
	// The override must not leak into the id's other fields: two rows of one
	// rule still differ by row_path, not by kind, and kind is not in MakeID.
	if got[0].RuleID != got[1].RuleID {
		t.Errorf("rule_id differs across the override: %q vs %q", got[0].RuleID, got[1].RuleID)
	}
}

// TestACounterpartPublishesTheFarLegFromTheSameFigure is the fan-out: one
// printed figure becoming the two facts a transfer needs.
//
// Budget Book p76 prints "Transfer From Low Income Hsng  to General Fund
// 257,012". That number is 257,012 leaving fund 200 and 257,012 arriving at
// fund 100, and fact.Fact carries one fund field, so one fact cannot hold the
// pair (fisc-4rh). What this pins is that the pair is expressible AND that the
// two legs are told apart on the components that carry the meaning of the
// difference, rather than on a discriminator added to avoid a hash clash.
func TestACounterpartPublishesTheFarLegFromTheSameFigure(t *testing.T) {
	f := &mapping.File{DocID: "doc"}
	rule := &mapping.Rule{ID: "p76-transfers-in-general", Kind: mapping.KindTransferIn,
		Basis: mapping.BasisAdopted, Scope: "transfers-by-fund", Units: "dollars"}

	got, err := FromValues(f, rule, []mapping.Value{{
		Row: mapping.Row{
			Label: "Transfer From Low Income Hsng", LabelTail: "to General Fund",
			Category: "transfers/in", Fund: 100,
			Counterpart: &mapping.Counterpart{
				Category: "transfers/out", Kind: mapping.KindTransferOut,
				Fund: 200, FundGroup: "special-revenue",
			},
		},
		Column: mapping.Column{FundGroup: "general", FiscalYear: 2026},
		Cents:  25701200, Page: 76, Offset: 4211, Token: "257,012",
	}})
	if err != nil {
		t.Fatalf("FromValues: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d facts from one figure, want 2 (the near leg and its counterpart)", len(got))
	}
	near, far := got[0], got[1]

	// ONE FIGURE IS THE PROVENANCE FOR BOTH DIRECTIONS. This is not duplicated
	// evidence: fact-offset-points-at-token has to hold for each leg, and both
	// legs point at the token the city actually printed.
	if near.Page != far.Page || near.Offset != far.Offset || near.Token != far.Token {
		t.Errorf("the legs cite different provenance: p%d@%d %q vs p%d@%d %q; one "+
			"printed figure is the evidence for both",
			near.Page, near.Offset, near.Token, far.Page, far.Offset, far.Token)
	}
	if near.AmountCents != far.AmountCents {
		t.Errorf("legs differ in amount: %d vs %d", near.AmountCents, far.AmountCents)
	}
	if far.RowLabel != near.RowLabel {
		t.Errorf("far leg row_label = %q, want the same printed line %q", far.RowLabel, near.RowLabel)
	}

	// AND THEY DIFFER TWICE OVER, without any change to MakeID. kind is not in
	// the id tuple, so kind alone could not have separated them; row_path and
	// column_path both do, and each says WHY the legs are different.
	if near.RowPath != "transfers/in" || far.RowPath != "transfers/out" {
		t.Errorf("row paths = %q / %q, want transfers/in and transfers/out",
			near.RowPath, far.RowPath)
	}
	if near.ColumnPath != "general/fund/100" || far.ColumnPath != "special-revenue/fund/200" {
		t.Errorf("column paths = %q / %q, want the receiving and the paying fund",
			near.ColumnPath, far.ColumnPath)
	}
	if near.ID == far.ID {
		t.Fatalf("both legs published as %s; one figure would be one fact and the "+
			"payer would be unpublishable", near.ID)
	}
	if far.Kind != mapping.KindTransferOut || far.FundGroup != "special-revenue" ||
		!SameFund(far.Fund, FundNumber(200)) {
		t.Errorf("far leg = %s %s fund %s, want transfer_out special-revenue 200",
			far.Kind, far.FundGroup, FundString(far.Fund))
	}
	// The near leg takes its fund from the ROW where the row declares one, and
	// its group from the column, which is the per-field override p76 needs:
	// the section is the receiving group, the row is the receiving fund.
	if near.FundGroup != "general" || !SameFund(near.Fund, FundNumber(100)) {
		t.Errorf("near leg = %s fund %s, want general 100", near.FundGroup, FundString(near.Fund))
	}
	if far.Derived || near.Derived {
		t.Error("a leg is marked derived; both are read off a printed figure")
	}
}

// TestACounterpartWithNoFundIsRefusedRatherThanFiledUnderTheScope guards the
// one way the far leg can go missing quietly. ColumnPath falls back to the
// rule's scope for a column with no fund dimension, so a counterpart declaring
// neither would publish under "transfers-by-fund" -- a real-looking fact, in a
// place no fund-group check would ever compare it against the spine.
func TestACounterpartWithNoFundIsRefusedRatherThanFiledUnderTheScope(t *testing.T) {
	f := &mapping.File{DocID: "doc"}
	rule := &mapping.Rule{ID: "r", Kind: mapping.KindTransferIn,
		Basis: mapping.BasisAdopted, Scope: "transfers-by-fund"}

	_, err := FromValues(f, rule, []mapping.Value{{
		Row: mapping.Row{Label: "Transfer From X", LabelTail: "to Y", Category: "transfers/in",
			Counterpart: &mapping.Counterpart{Category: "transfers/out", Kind: mapping.KindTransferOut}},
		Column: mapping.Column{FiscalYear: 2026},
		Cents:  100, Page: 76, Offset: 1, Token: "1",
	}})
	if err == nil {
		t.Fatal("no error; a counterpart with no fund was filed under the rule's scope")
	}
	if !strings.Contains(err.Error(), "counterpart has no addressable column path") {
		t.Errorf("error %q does not say the counterpart has no column path", err)
	}
}

// TestACounterpartWithNoRowPathIsRefused is the far leg's other half, and it is
// the one that was missing.
//
// The near leg is guarded on BOTH paths twenty lines above the fan-out; the far
// leg was guarded only on its column. cpRow takes its category from the
// counterpart and its department from the row it fans out of, so both can be
// empty at once -- and the far leg then publishes row_path "" AND AN ID HASHED
// OVER IT, an unaddressable fact rather than an error.
//
// LATENT, NOT LIVE, which is why this test has to build the value by hand:
// mapping's checkCounterpart refuses an empty counterpart category, so nothing
// reachable through the parser can produce one. The hole is open to any direct
// TestARowWithADepartmentAndNoCategoryIsRefused is the near-leg equivalent, and
// it exists because RowPath does not witness the thing that matters.
//
// RowPath returns the bare department when Category is empty, so a Value with
// `department:` and no category produced a NON-empty row path, passed the
// guard, and published a fact carrying Category: "" -- which factVocabulary
// and factKindMatchesCategory both skip, and which in a scope no projection
// selects is named by nothing at all. mapping.Parse closed that on 2026-08-29;
// this is the same rule at the other constructor, because the parser is not
// the only FromValues caller and the guard beside it already says so.
func TestARowWithADepartmentAndNoCategoryIsRefused(t *testing.T) {
	f := &mapping.File{DocID: "doc"}
	rule := &mapping.Rule{ID: "r", Kind: mapping.KindExpenditure,
		Basis: mapping.BasisAdopted, Scope: "expenditure-by-department"}

	_, err := FromValues(f, rule, []mapping.Value{{
		Row:    mapping.Row{Label: "Wages & Benefits", Department: "city-manager"},
		Column: mapping.Column{FundGroup: "general", FiscalYear: 2026},
		Cents:  100, Page: 167, Offset: 1, Token: "1",
	}})
	if err == nil {
		t.Fatal("no error; the row published a fact with category \"\", which every " +
			"vocabulary check skips")
	}
	if !strings.Contains(err.Error(), "no addressable category") {
		t.Errorf("error %q does not say the category is what is missing", err)
	}
	if !strings.Contains(err.Error(), "second axis") {
		t.Errorf("error %q does not say a department is not a substitute", err)
	}
}

// FromValues caller, and the parser is not the only thing that will ever be one.
func TestACounterpartWithNoRowPathIsRefused(t *testing.T) {
	f := &mapping.File{DocID: "doc"}
	rule := &mapping.Rule{ID: "r", Kind: mapping.KindTransferIn,
		Basis: mapping.BasisAdopted, Scope: "transfers-by-fund"}

	_, err := FromValues(f, rule, []mapping.Value{{
		Row: mapping.Row{Label: "Transfer From X", LabelTail: "to Y", Category: "transfers/in",
			Fund: 100,
			// A counterpart naming its fund but no category: the column path
			// resolves, so the existing guard passes it straight through.
			Counterpart: &mapping.Counterpart{Kind: mapping.KindTransferOut,
				Fund: 200, FundGroup: "special-revenue"}},
		Column: mapping.Column{FundGroup: "general", FiscalYear: 2026},
		Cents:  100, Page: 76, Offset: 1, Token: "1",
	}})
	if err == nil {
		t.Fatal("no error; the far leg published row_path \"\" and an id hashed over it")
	}
	if !strings.Contains(err.Error(), "counterpart has no addressable row path") {
		t.Errorf("error %q does not say the counterpart has no row path", err)
	}
	// The near leg's wording, on the far leg. An error that named the missing
	// piece on one side and not the other is how this went unnoticed.
	if !strings.Contains(err.Error(), "no category and the row no department") {
		t.Errorf("error %q does not name what would have to be declared", err)
	}
}

// TestMakeSeriesIDIsTheFactIDWithoutRuleOrColumn is the relationship the
// revenue trends contract publishes, checked rather than asserted in prose: a
// series id is MakeID's tuple minus the rule id, the fiscal year and the basis.
//
// The consequence a reader depends on is the second half: two facts of the SAME
// printed row in different columns share a series id, and two facts of different
// rows never do.
func TestMakeSeriesIDIsTheFactIDWithoutRuleOrColumn(t *testing.T) {
	const (
		doc  = "livermore-budget-fy2026-2027"
		rule = "gf-rev-other-taxes"
		path = "taxes/other"
		col  = "general/fund/100"
	)
	row := "Industrial Construction Tax"

	base := makeSeriesID(doc, path, row, col)
	// Every column of one row agrees.
	for _, c := range []struct {
		year  int
		basis mapping.Basis
	}{
		{2024, mapping.BasisActual},
		{2025, mapping.BasisRevised},
		{2026, mapping.BasisAdopted},
		{2027, mapping.BasisAdopted},
	} {
		f := Fact{DocID: doc, RuleID: rule, RowPath: path, RowLabel: row, ColumnPath: col,
			FiscalYear: c.year, Basis: c.basis}
		if got := f.SeriesID(); got != base {
			t.Errorf("FY%d %s recomputes to %s, want %s", c.year, c.basis, got, base)
		}
		// And the FACT id differs per column, which is what makes the two
		// identities do different jobs.
		if id := MakeID(doc, rule, path, row, col, c.year, c.basis); id == base {
			t.Errorf("fact id and series id collide at %s", id)
		}
	}

	// One row printed under a different rule each year, as pp.186-209 are
	// mapped, is still one series.
	other := Fact{DocID: doc, RuleID: "fund-balances-fy2027-p0204", RowPath: path,
		RowLabel: row, ColumnPath: col, FiscalYear: 2027, Basis: mapping.BasisAdopted}
	if got := other.SeriesID(); got != base {
		t.Errorf("the same row under another rule is series %s, want %s", got, base)
	}

	// A different row of the same fund, and the same row in a different fund,
	// are different series. The second is the case the document exists for:
	// "Property Taxes" is printed by four funds.
	if makeSeriesID(doc, path, "Other Row", col) == base {
		t.Error("two different printed rows share a series id")
	}
	if makeSeriesID(doc, path, row, "special-revenue/fund/310") == base {
		t.Error("one row label in two funds shares a series id")
	}

	// The prefixes must differ, so an id is recognizable on sight without a
	// lookup -- IDPrefix's own doc comment is the rule this extends.
	if strings.HasPrefix(base, IDPrefix) {
		t.Errorf("series id %q carries the fact prefix", base)
	}
	if !strings.HasPrefix(base, seriesIDPrefix) {
		t.Errorf("series id %q does not carry %q", base, seriesIDPrefix)
	}
}

// committedFacts reads the fact store this repository publishes.
func committedFacts(t *testing.T) []Fact {
	t.Helper()
	f, err := os.Open(filepath.Join("..", "..", "facts", "facts.jsonl"))
	if err != nil {
		t.Fatalf("open the committed fact store: %v", err)
	}
	defer f.Close()
	facts, err := Read(f)
	if err != nil {
		t.Fatalf("read the committed fact store: %v", err)
	}
	return facts
}

// TestASeriesIsOneCellPerYearAndBasis is what lets makeSeriesID leave the rule
// out: over the committed store, no two facts share a series, a fiscal year and
// a basis, so no series stacks two cells in one column.
func TestASeriesIsOneCellPerYearAndBasis(t *testing.T) {
	type cell struct {
		series string
		year   int
		basis  mapping.Basis
	}
	seen := map[cell]string{}
	for _, f := range committedFacts(t) {
		c := cell{f.SeriesID(), f.FiscalYear, f.Basis}
		if prev, ok := seen[c]; ok {
			t.Errorf("%s and %s are both series %s in FY%d %s", prev, f.ID, c.series, c.year, c.basis)
			continue
		}
		seen[c] = f.ID
	}
}

// TestAFundBalancesRowIsOneSeriesAcrossItsYears reads the General Fund's
// Revenues on Budget Book pp.186, 192, 198 and 204: four rules, one per year,
// and one series of four points.
func TestAFundBalancesRowIsOneSeriesAcrossItsYears(t *testing.T) {
	series := map[string][]int{}
	rules := map[string]bool{}
	for _, f := range committedFacts(t) {
		if f.Scope != "fund-balances-by-fund" || f.RowPath != "revenues" ||
			f.Fund == nil || *f.Fund != 100 {
			continue
		}
		series[f.SeriesID()] = append(series[f.SeriesID()], f.FiscalYear)
		rules[f.RuleID] = true
	}
	if len(rules) != 4 {
		t.Fatalf("the General Fund's Revenues are read by %d rules, want one per year (4)", len(rules))
	}
	if len(series) != 1 {
		t.Fatalf("the General Fund's Revenues are %d series, want 1: %v", len(series), series)
	}
	for id, years := range series {
		slices.Sort(years)
		if diff := cmp.Diff([]int{2024, 2025, 2026, 2027}, years); diff != "" {
			t.Errorf("series %s years (-want +got):\n%s", id, diff)
		}
	}
}

// TestAColumnCategoryReachesTheFact reads Budget Book pp.186-187's General Fund
// row, whose budget lines are the page's columns. Each fact's row_path is its
// column's category, so the eight cells of one row in one year hash to eight
// ids, and each carries the row's fund: the axes a fund-by-category grain names.
func TestAColumnCategoryReachesTheFact(t *testing.T) {
	facts := factsFrom(t, "../mapping/testdata/fund-balances-p186.yaml", 186, 187)
	if err := CheckUniqueIDs(facts); err != nil {
		t.Fatalf("one row's cells collide: %v", err)
	}
	type cell struct {
		RowPath, Category string
		Kind              mapping.Kind
		FundGroup         string
		Fund              int
		Cents             int64
	}
	var got []cell
	for _, f := range facts {
		if f.Fund == nil {
			t.Fatalf("fact %s carries no fund", f.ID)
		}
		if f.ID != MakeID(f.DocID, f.RuleID, f.Category, f.RowLabel, f.ColumnPath,
			f.FiscalYear, f.Basis) {
			t.Errorf("fact %s is not hashed over its column's category %q", f.ID, f.Category)
		}
		got = append(got, cell{f.RowPath, f.Category, f.Kind, f.FundGroup, *f.Fund, f.AmountCents})
	}
	want := []cell{
		{"fund-balance/beginning", "fund-balance/beginning", mapping.KindFundBalance, "general", 100, 1444069000},
		{"taxes", "taxes", mapping.KindRevenue, "general", 100, 14202800200},
		{"transfers/in", "transfers/in", mapping.KindTransferIn, "general", 100, 73745500},
		{"wages-and-benefits", "wages-and-benefits", mapping.KindExpenditure, "general", 100, 12322819000},
		{"transfers/out", "transfers/out", mapping.KindTransferOut, "general", 100, 1450739800},
		{"transfers/out-to-cip", "transfers/out-to-cip", mapping.KindTransferOut, "general", 100, 44084600},
		{"fund-balance/reserve-increase", "fund-balance/reserve-increase", mapping.KindFundBalance, "general", 100, 428893300},
		{"fund-balance/ending", "fund-balance/ending", mapping.KindFundBalance, "general", 100, 1474078000},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("General Fund's facts (-want +got):\n%s", diff)
	}
}
