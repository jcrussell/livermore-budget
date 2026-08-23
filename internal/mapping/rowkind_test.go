package mapping

import (
	"strings"
	"testing"
)

// stormwaterPage is Budget Book p131's Stormwater block, transcribed with its
// printed figures, spacing collapsed but rows and columns intact.
//
// It is the block fisc-uli exists for: five revenue rows and a Transfers In row
// inside ONE printed `Total Stormwater`. All four columns tie only when the
// transfer row is included -- FY2025-26 is 1,169,000 of Charges for Services
// plus 3,247,000 of Transfers In, and there is no other way to reach the
// printed 4,416,000.
//
// The page is inline rather than a committed fixture because testdata/ carries
// no p131 pair yet; adding one owes both substrates and a README entry, which
// is fisc-5gk.1's own work. The figures are the document's.
const stormwaterPage = `Stormwater
Fines & Forfeitures 1,211 - - -
Intergovernmental 55,380 3,444,000 - -
Charges for Services 1,155,156 1,161,870 1,169,000 1,174,000
ContributionsOutSrce 263,000 - - -
Transfers In 3,470,000 2,740,000 3,247,000 3,330,000
Total Stormwater $4,944,747 $7,345,870 $4,416,000 $4,504,000
`

const stormwaterRule = `schema_version: 1
doc_id: stormwater-doc

rules:
  - id: stormwater
    kind: revenue
    basis: adopted
    scope: all-funds-gross
    units: dollars
    total_row: "Total Stormwater"
    #KINDS
    rows:
      - {label: "Fines & Forfeitures", category: fines-and-forfeitures}
      - {label: "Intergovernmental", category: intergovernmental}
      - {label: "Charges for Services", category: charges-for-services}
      - {label: "ContributionsOutSrce", category: contributions-outsourced}
      - {label: "Transfers In", category: transfers/in, kind: transfer_in}
    parts:
      - page: 131
        section: "Stormwater"
        section_ordinal: 1
        stop_at: "Total Stormwater"
        columns:
          - {fund_group: enterprise, fund: 501, fiscal_year: 2024, basis: actual}
          - {fund_group: enterprise, fund: 501, fiscal_year: 2025, basis: revised}
          - {fund_group: enterprise, fund: 501, fiscal_year: 2026}
          - {fund_group: enterprise, fund: 501, fiscal_year: 2027}
`

func stormwaterResolver(t *testing.T, kinds string) (*Resolver, *Rule) {
	t.Helper()
	src := strings.Replace(stormwaterRule, "    #KINDS", kinds, 1)
	f, err := Parse(strings.NewReader(src), "stormwater.yaml")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	r, err := NewResolver(inlineDoc(t, f.DocID, map[int]string{131: stormwaterPage}), f)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	return r, &f.Rules[0]
}

// TestAMixedBlockTiesBecauseCheckTotalsStaysKindBlind is the regression test
// for the decision at the centre of fisc-uli.
//
// Row.Kind lets one rule carry rows of two kinds. The tempting next step --
// making CheckTotals sum only the rule's own kind -- would break exactly the
// blocks the override was promoted to enable: this printed total covers BOTH
// kinds, so a kind-aware sum reaches 1,169,000 where the document prints
// 4,416,000. Kind-blindness is the behaviour, not an oversight.
func TestAMixedBlockTiesBecauseCheckTotalsStaysKindBlind(t *testing.T) {
	r, rule := stormwaterResolver(t, "    #KINDS")
	res, err := r.CheckTotals(rule, &rule.Parts[0])
	if err != nil {
		t.Fatalf("CheckTotals: %v; the printed total covers the transfer row too", err)
	}
	if res.Columns != 4 {
		t.Errorf("Columns = %d, want 4", res.Columns)
	}
}

// TestTotalRowKindsNarrowsWhatTheTotalCovers is the other half: where a printed
// total covers only some of the kinds beneath it, the rule says so on the
// declaration and CheckTotals honours it.
//
// The assertion is written as a pair on purpose. Declaring `[revenue]` here is
// WRONG about this document -- Total Stormwater does cover the transfer row --
// so the check must FAIL, and fail with the figure it actually summed. A filter
// that could only ever narrow a passing check into another passing check would
// not be a check.
func TestTotalRowKindsNarrowsWhatTheTotalCovers(t *testing.T) {
	r, rule := stormwaterResolver(t, "    total_row_kinds: [revenue]")
	_, err := r.CheckTotals(rule, &rule.Parts[0])
	if err == nil {
		t.Fatal("declaring the total covers only revenue still tied; the transfer " +
			"row is 3,247,000 of the printed 4,416,000")
	}
	// 1,169,000 is the FY2025-26 revenue-only sum, in cents as the message prints it.
	if !strings.Contains(err.Error(), "1,169,000") {
		t.Errorf("error = %v; want it to name the revenue-only sum it compared", err)
	}
}

// TestTotalRowKindsRefusesWhatAssertsNothing states the declarations the parser
// will not accept, all of them claims that cannot fail.
func TestTotalRowKindsRefusesWhatAssertsNothing(t *testing.T) {
	for _, tc := range []struct{ name, kinds, want string }{
		{"a kind no row has", "    total_row_kinds: [fund_balance]",
			`no row of this rule has kind "fund_balance"`},
		{"a kind that is not one of the five", "    total_row_kinds: [banana]",
			`"banana" is not one of the five kinds`},
		{"the same kind twice", "    total_row_kinds: [revenue, revenue]",
			`"revenue" is listed twice`},
		{"every kind the rule maps", "    total_row_kinds: [revenue, transfer_in]",
			"excludes nothing"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := strings.Replace(stormwaterRule, "    #KINDS", tc.kinds, 1)
			if _, err := Parse(strings.NewReader(src), "stormwater.yaml"); err == nil {
				t.Fatal("accepted")
			} else if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %v, want it to contain %q", err, tc.want)
			}
		})
	}
}

// TestARowKindThatIsNotOneOfTheFiveIsRefused keeps the override inside the
// vocabulary. Without this a typo'd kind reaches facts.jsonl, where the only
// thing that would notice is a check over a category that happens to declare
// its kinds.
func TestARowKindThatIsNotOneOfTheFiveIsRefused(t *testing.T) {
	src := strings.Replace(stormwaterRule, "kind: transfer_in}", "kind: transfers}", 1)
	_, err := Parse(strings.NewReader(src), "stormwater.yaml")
	if err == nil {
		t.Fatal("a row declaring kind: transfers was accepted")
	}
	if !strings.Contains(err.Error(), `kind "transfers" is not one of the five`) {
		t.Errorf("error = %v", err)
	}
}

// TestEffectiveKindPrefersTheRow states the override's precedence directly,
// the same shape internal/fact states for Column.Basis over Rule.Basis.
func TestEffectiveKindPrefersTheRow(t *testing.T) {
	rule := &Rule{Kind: KindRevenue}
	if got := (Row{Label: "Charges"}).EffectiveKind(rule); got != KindRevenue {
		t.Errorf("a row declaring no kind = %q, want the rule's %q", got, KindRevenue)
	}
	if got := (Row{Label: "Transfers In", Kind: KindTransferIn}).EffectiveKind(rule); got != KindTransferIn {
		t.Errorf("a row declaring a kind = %q, want its own %q", got, KindTransferIn)
	}
}
