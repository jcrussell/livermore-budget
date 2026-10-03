package mapping

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/jcrussell/livermore-budget/internal/amount"
)

// TestAColumnCarriesTheCategoryAndKind reads Budget Book pp.186-187's first
// summary row, General Fund, whose budget lines are the page's COLUMNS: each
// value takes its category from its column, and its kind from the column or,
// where the column declares none, the rule.
func TestAColumnCarriesTheCategoryAndKind(t *testing.T) {
	f, err := Load("testdata/fund-balances-p186.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	r, err := NewResolver(budgetDoc(t, 186, 187), f)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	rule := &f.Rules[0]

	type cell struct {
		Category string
		Kind     Kind
		Cents    amount.Cents
	}
	var got []cell
	for i := range rule.Parts {
		vals, _, err := r.Values(rule, &rule.Parts[i])
		if err != nil {
			t.Fatalf("Values(p%d): %v", rule.Parts[i].Page, err)
		}
		for _, v := range vals {
			got = append(got, cell{v.Category(), v.Kind(rule), v.Cents})
		}
	}
	want := []cell{
		{"fund-balance/beginning", KindFundBalance, 1444069000},
		{"taxes", KindRevenue, 14202800200},
		{"transfers/in", KindTransferIn, 73745500},
		{"wages-and-benefits", KindExpenditure, 12322819000},
		{"transfers/out", KindTransferOut, 1450739800},
		{"transfers/out-to-cip", KindTransferOut, 44084600},
		{"fund-balance/reserve-increase", KindFundBalance, 428893300},
		{"fund-balance/ending", KindFundBalance, 1474078000},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("General Fund's cells (-want +got):\n%s", diff)
	}
}

// TestValueClassificationFollowsOneAxis pins the accessor every consumer reads
// a value's category and kind through: the row's on a row-category rule, the
// column's on a column-category rule, the rule's kind where neither says.
func TestValueClassificationFollowsOneAxis(t *testing.T) {
	rule := &Rule{Kind: KindRevenue}
	tests := []struct {
		name     string
		v        Value
		category string
		kind     Kind
	}{
		{"row axis, rule kind", Value{Row: Row{Category: "taxes"}}, "taxes", KindRevenue},
		{"row axis, row kind", Value{Row: Row{Category: "transfers/in", Kind: KindTransferIn}},
			"transfers/in", KindTransferIn},
		{"column axis, rule kind", Value{Column: Column{Category: "taxes"}}, "taxes", KindRevenue},
		{"column axis, column kind", Value{Column: Column{Category: "transfers/in",
			Kind: KindTransferIn}}, "transfers/in", KindTransferIn},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.v.Category(); got != tt.category {
				t.Errorf("Category() = %q, want %q", got, tt.category)
			}
			if got := tt.v.Kind(rule); got != tt.kind {
				t.Errorf("Kind() = %q, want %q", got, tt.kind)
			}
		})
	}
}

// columnRule is a two-column rule over one row, with the row and the columns
// spliced in.
func columnRule(row, cols string) string {
	return "schema_version: 1\ndoc_id: d\nrules:\n  - id: r\n" +
		"    kind: fund_balance\n    basis: actual\n    grain: fund-by-category\n" +
		"    units: dollars\n" +
		"    parts: [{page: 1, columns: [" + cols + "]}]\n" +
		"    rows:\n      - " + row + "\n"
}

func TestParseRefusesAMixedCategoryAxis(t *testing.T) {
	const two = "{fiscal_year: 2024, category: a}, {fiscal_year: 2024, category: b}"
	tests := []struct {
		name string
		yaml string
		want string
	}{
		{"a row category beside column categories",
			columnRule(`{label: "A", fund: 100, category: a}`, two),
			`row "A" declares category "a", but this rule's columns carry the category`},
		{"a row kind beside column categories",
			columnRule(`{label: "A", fund: 100, kind: revenue}`, two),
			`row "A" declares kind "revenue", but this rule's columns carry the category`},
		{"a published column with no category beside one with",
			columnRule(`{label: "A", fund: 100}`, "{fiscal_year: 2024, category: a}, {fiscal_year: 2025}"),
			"columns[1] has no category"},
		{"a column kind on a row-category rule",
			columnRule(`{label: "A", category: a}`, "{fiscal_year: 2024, kind: revenue}"),
			`columns[0] declares kind "revenue" and no category`},
		{"a column kind that is not one of the five",
			columnRule(`{label: "A", fund: 100}`, "{fiscal_year: 2024, category: a, kind: incom}"),
			`columns[0]: kind "incom" is not one of the five`},
		{"a category on a skipped column",
			columnRule(`{label: "A", category: a}`, "{fiscal_year: 2024}, {skip: true, category: b}"),
			"columns[1] publishes no fact and declares a category or kind"},
		{"a counterpart on a column-category rule",
			columnRule(`{label: "A", fund: 100, counterpart: {category: transfers/out, `+
				`kind: transfer_out, fund: 200, fund_group: special-revenue}}`, two),
			`row "A" declares a counterpart, but this rule's columns carry the category`},
		{"a netted row over a column kind that is not one of the five",
			columnRule(`{label: "A", fund: 100, sign: netted}`, "{fiscal_year: 2024, category: a, kind: incom}"),
			`columns[0]: kind "incom" is not one of the five`},
		{"two columns differing only in kind",
			columnRule(`{label: "A", fund: 100}`,
				"{fiscal_year: 2024, category: a}, {fiscal_year: 2024, category: a, kind: revenue}"),
			"duplicates an earlier column"},
		{"neither axis",
			columnRule(`{label: "A", fund: 100}`, "{fiscal_year: 2024}"),
			`row "A" has no category`},
		{"sign netted on a row whose column is not a transfer",
			columnRule(`{label: "A", fund: 100, sign: netted}`,
				"{fiscal_year: 2024, category: a, kind: transfer_in}, {fiscal_year: 2024, category: b}"),
			`sign netted on kind "fund_balance"`},
		{"total_row_kinds naming a kind no column carries",
			strings.Replace(columnRule(`{label: "A", fund: 100}`, two),
				"    rows:", "    total_row: T\n    total_row_kinds: [revenue]\n    rows:", 1),
			`no row of this rule has kind "revenue"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parse(strings.NewReader(tt.yaml), "test.yaml")
			if err == nil {
				t.Fatalf("parsed; want an error containing %q", tt.want)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %v\nwant it to contain %q", err, tt.want)
			}
		})
	}
}

func TestParseAcceptsAColumnCategoryAxis(t *testing.T) {
	tests := []struct{ name, yaml string }{
		{"two columns of one year told apart by category",
			columnRule(`{label: "A", fund: 100, fund_group: general}`,
				"{fiscal_year: 2024, category: a}, {fiscal_year: 2024, category: b, kind: revenue}")},
		{"fund rows read as printed fund names",
			strings.Replace(columnRule(`{label: "A", fund: 100, fund_group: general}`,
				"{fiscal_year: 2024, category: a}, {fiscal_year: 2024, category: b}"),
				"    rows:", "    row_labels_name_funds: true\n    rows:", 1)},
		// The skipped column has no category, so a reading of its kind would
		// find the rule's, which has no direction; the netted check reads only
		// the cells Row.Publishes admits.
		{"sign netted where every column is a transfer or skipped",
			columnRule(`{label: "A", fund: 100, sign: netted}`,
				"{fiscal_year: 2024, category: a, kind: transfer_in}, "+
					"{fiscal_year: 2024, category: b, kind: transfer_out}, {skip: true}")},
		{"total_row_kinds read off the columns",
			strings.Replace(columnRule(`{label: "A", fund: 100}`,
				"{fiscal_year: 2024, category: a}, {fiscal_year: 2024, category: b, kind: revenue}"),
				"    rows:", "    total_row: T\n    total_row_kinds: [revenue]\n    rows:", 1)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := parse(strings.NewReader(tt.yaml), "test.yaml"); err != nil {
				t.Errorf("parse: %v", err)
			}
		})
	}
}

// TestColumnIdentityNamesTheCategoryAndKind: two rules whose columns differ only
// in category or kind are not laid out alike, and the message must say which.
func TestColumnIdentityNamesTheCategoryAndKind(t *testing.T) {
	a := Column{FiscalYear: 2024, Category: "transfers/in", Kind: KindTransferIn}
	b := a
	b.Category = "transfers/out"
	if i, ok := firstDifferingColumn([]Column{a}, []Column{b}); !ok || i != 0 {
		t.Errorf("firstDifferingColumn = %d, %v; want 0, true", i, ok)
	}
	for _, want := range []string{"transfers/in", string(KindTransferIn)} {
		if got := columnIdentity(a); !strings.Contains(got, want) {
			t.Errorf("columnIdentity = %q, want it to name %q", got, want)
		}
	}
	// The kind in force is the rule's where the column states none, as the
	// basis is, so a rule that writes it out is laid out like one that does not.
	rule := &Rule{Kind: KindFundBalance, Basis: BasisActual}
	bare := &Part{Columns: []Column{{FiscalYear: 2024, Category: "fund-balance/ending"}}}
	spelt := &Part{Columns: []Column{{FiscalYear: 2024, Category: "fund-balance/ending",
		Kind: KindFundBalance, Basis: BasisActual}}}
	if i, ok := firstDifferingColumn(effectiveColumns(rule, bare), effectiveColumns(rule, spelt)); !ok || i != -1 {
		t.Errorf("firstDifferingColumn over effective columns = %d, %v; want -1, true", i, ok)
	}
}
