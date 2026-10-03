package structure

import (
	"fmt"
	"strconv"

	"github.com/jcrussell/livermore-budget/internal/amount"
	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/mapping"
	"github.com/jcrussell/livermore-budget/internal/registry"
)

// A Line is one row a fund balance prints: a kind, narrowed to a category
// where the kind alone names more than one row.
type Line struct {
	Kind     mapping.Kind
	Category string
}

// Matches says whether a fact is printed on this line.
func (l Line) Matches(f *fact.Fact) bool {
	return f.Kind == l.Kind && (l.Category == "" || f.Category == l.Category)
}

func (l Line) String() string {
	if l.Category == "" {
		return string(l.Kind)
	}
	if l.Kind == mapping.KindFundBalance {
		return l.Category
	}
	return string(l.Kind) + " " + l.Category
}

// The three lines of a balance's own identity.
var (
	LineBeginning = Line{mapping.KindFundBalance, CategoryFundBalanceBeginning}
	LineChange    = Line{mapping.KindFundBalance, CategoryFundBalanceChange}
	LineEnding    = Line{mapping.KindFundBalance, CategoryFundBalanceEnding}
)

// A Balance is a scope that prints a fund balance per fund group, fund and
// column. Every scope carrying a beginning, change or ending line is one.
type Balance struct {
	Scope string
	// Lines are the rows every balance of the scope must carry, or leave blank
	// by a cell its rule declares omitted. A scope printing LineChange holds
	// beginning + change == ending; one that does not holds its change as
	// ending - beginning, a difference of two printed figures.
	Lines []Line
	// SourcesUses says the scope's flows net to its change: revenue +
	// transfers in - expenditure - transfers out - reserve increase.
	SourcesUses bool
}

// PrintsChange says whether the scope prints a change line.
func (b Balance) PrintsChange() bool {
	for _, l := range b.Lines {
		if l == LineChange {
			return true
		}
	}
	return false
}

// FundBalances is every scope printing a fund balance, and how.
//
// The spine and ACFR p41 print a change line, which witnesses their flows, so
// only the three balance lines are required of them. pp.186-209 print no change
// line, so every flow they print is required: a dropped column of dashes would
// leave ending - beginning with nothing to violate.
func FundBalances() []Balance {
	identity := []Line{LineBeginning, LineChange, LineEnding}
	return []Balance{
		{Scope: ScopeAllFundsGross, Lines: identity, SourcesUses: true},
		// p41 prints its Transfers (out) negative, so its flows are not
		// SourcesUses' terms as signed.
		{Scope: ScopeACFRGeneralFundSummary, Lines: identity},
		{
			Scope: ScopeFundBalancesByFund,
			Lines: []Line{
				LineBeginning,
				{Kind: mapping.KindRevenue},
				{Kind: mapping.KindTransferIn, Category: "transfers/in"},
				{Kind: mapping.KindExpenditure},
				{Kind: mapping.KindTransferOut, Category: "transfers/out"},
				{Kind: mapping.KindTransferOut, Category: "transfers/out-to-cip"},
				{Kind: mapping.KindFundBalance, Category: CategoryFundBalanceReserveIncrease},
				LineEnding,
			},
			SourcesUses: true,
		},
	}
}

// FundBalanceGroupRows maps each summary row pp.186-209 print at the head of a
// year to the fund type data/funds.yaml gives its funds. The Capital
// Improvement Program Funds row maps to "": its funds are residue, typed by the
// operating fund they sit in, and held to p222 instead. FY2023-24 prints the
// internal service row as "Internal Service Funds", the later years as
// "Internal Service".
func FundBalanceGroupRows() map[string]string {
	return map[string]string{
		"General Fund":                      registry.FundTypeGeneral,
		"Special Revenue Funds":             registry.FundTypeSpecialRevenue,
		"Debt Service Funds":                registry.FundTypeDebtService,
		"Permanent Funds":                   registry.FundTypePermanent,
		"Capital Funds":                     registry.FundTypeCapital,
		"Enterprise Funds":                  registry.FundTypeEnterprise,
		"Internal Service Funds":            registry.FundTypeInternalService,
		"Internal Service":                  registry.FundTypeInternalService,
		"Capital Improvement Program Funds": "",
	}
}

// FundBalanceGroupTotals maps each group's printed block total on pp.186-209
// to its fund type: the row the summary block's group row repeats, and the
// one whose subtotal_deltas say where the page rounds it off its funds.
func FundBalanceGroupTotals() map[string]string {
	return map[string]string{
		"Total Special Revenue Funds":  registry.FundTypeSpecialRevenue,
		"Total Debt Service Funds":     registry.FundTypeDebtService,
		"Total Permanent Funds":        registry.FundTypePermanent,
		"Total Capital Funds":          registry.FundTypeCapital,
		"Total Enterprise Funds":       registry.FundTypeEnterprise,
		"Total Internal Service Funds": registry.FundTypeInternalService,
	}
}

// BalanceOf is the declaration of a scope, if it prints a fund balance.
func BalanceOf(balances []Balance, scope string) (Balance, bool) {
	for _, b := range balances {
		if b.Scope == scope {
			return b, true
		}
	}
	return Balance{}, false
}

// BalanceAt is one balance: one fund of one fund group of one scope of one
// document, in one (fiscal year, basis) column. Fund is fact.FundString's
// rendering, so two facts naming one fund are one balance.
type BalanceAt struct {
	DocID, Scope, FundGroup, Fund string
	Year                          int
	Basis                         mapping.Basis
}

// BalanceAtOf is the balance a fact is a line of.
func BalanceAtOf(f *fact.Fact) BalanceAt {
	return BalanceAt{DocID: f.DocID, Scope: f.Scope, FundGroup: f.FundGroup,
		Fund: fact.FundString(f.Fund), Year: f.FiscalYear, Basis: f.Basis}
}

// Series is the balance less its column: what carries forward from year to
// year.
func (a BalanceAt) Series() string {
	group := a.FundGroup
	if group == "" {
		group = "(no fund group)"
	}
	if a.Fund != fact.FundString(nil) {
		group = fmt.Sprintf("%s fund %s", group, a.Fund)
	}
	return fmt.Sprintf("%s %s %s", a.DocID, a.Scope, group)
}

func (a BalanceAt) String() string {
	return a.Series() + " " + fact.ColumnLabel(a.Year, a.Basis)
}

// A BalanceIdentity names one identity a balance is held to.
type BalanceIdentity string

const (
	// BalanceCarryForward is ending(y) == beginning(y+1). An exception's At
	// is year y's balance; Left is its ending and Right the next beginning.
	BalanceCarryForward BalanceIdentity = "carry-forward"
	// BalanceSourcesUses is SourcesUses.Net() == the change. Left is the net
	// and Right the change.
	BalanceSourcesUses BalanceIdentity = "sources-uses"
)

// A BalanceException holds one balance apart from one identity, pinned on
// both sides. It is not a tolerance: it fails when either side moves, when
// the balance stops existing, or when the identity holds there.
type BalanceException struct {
	Identity    BalanceIdentity
	At          BalanceAt
	Left, Right int64
	// Printed says where both sides are printed; Reason is why the document
	// breaks the identity there; Bead is the work that retires it or the
	// decision that keeps it.
	Printed, Reason, Bead string
}

// BalanceExceptions are the balances the documents print apart from an
// identity, each pinned on both sides.
func BalanceExceptions() []BalanceException {
	return append([]BalanceException{{
		Identity: BalanceCarryForward,
		At: BalanceAt{DocID: budgetBookDocID, Scope: ScopeFundBalancesByFund, FundGroup: "capital",
			Fund: "101", Year: 2024, Basis: mapping.BasisActual},
		Left:    0,
		Right:   3_495_436_300,
		Printed: "Budget Book p189 (General Fund CIP Reserves ends FY2023-24 at -) and p194 (begins FY2024-25 at 34,954,363)",
		Reason: "p196's footnote 2, keyed to the row on p194, says the CIP reserves fund was created in " +
			"FY 2024-25 and moved out of the General Fund, and p190's footnote 1 that the CIP specific " +
			"funds were created in FY 2024-25 with the new ERP system. Neither says where the 34,954,363 " +
			"came from, and the General Fund begins FY2024-25 at its FY2023-24 ending, 14,740,780, so " +
			"the schedule shows no balance leaving it",
		Bead: "fisc-wb2q",
	}}, fy2024RoundingExceptions()...)
}

// budgetBookDocID is the Budget Book's registered source id.
const budgetBookDocID = "livermore-budget-fy2026-2027"

// fy2024RoundingExceptions are the FY2023-24 actual lines on pp.186-191 whose
// printed figures, each rounded to the dollar on its own, net to a dollar off
// the change between the printed balances. Measured over every published line
// of the four years: these 18, all FY2023-24.
func fy2024RoundingExceptions() []BalanceException {
	var out []BalanceException
	for _, r := range []struct {
		group       string
		fund, page  int
		net, change int64
	}{
		{"special-revenue", 201, 186, 30093, 30094},   // Housing Successor Agency
		{"special-revenue", 210, 186, 103261, 103260}, // Horizons
		{"special-revenue", 220, 186, 44142, 44143},   // Grant - Federal Grant Fund
		{"special-revenue", 224, 186, -94329, -94330}, // Grant - CDBG
		{"special-revenue", 225, 186, 42143, 42142},   // Grant - Home Grant
		{"special-revenue", 240, 186, -76804, -76803}, // Grant - State Grant Fund
		{"special-revenue", 282, 186, -33382, -33383}, // Host Community Impact Fee
		{"special-revenue", 284, 186, 17292, 17293},   // Public Art Fee
		{"special-revenue", 289, 186, -19189, -19190}, // Solid Waste & Recycling Fee
		{"special-revenue", 290, 186, 113266, 113265}, // Human Services Facility Fee
		{"special-revenue", 300, 186, 370738, 370739}, // Open Space Acquisition & Mgmt
		{"special-revenue", 311, 186, 488922, 488923}, // Other LMD
		{"capital", 510, 188, 576336, 576335},         // Traffic Impact Fee (TIF)
		{"capital", 512, 188, 941613, 941614},         // Park Fee - AB 1600
		{"capital", 553, 188, 37842, 37841},           // County Measure F Veh Reg Fee
		{"capital", 560, 188, -6145, -6144},           // State - Gas Tax
		{"capital", 561, 188, 650787, 650786},         // State - SB1
		{"enterprise", 640, 188, 286471, 286472},      // Water
	} {
		out = append(out, BalanceException{
			Identity: BalanceSourcesUses,
			At: BalanceAt{DocID: budgetBookDocID, Scope: ScopeFundBalancesByFund, FundGroup: r.group,
				Fund: strconv.Itoa(r.fund), Year: 2024, Basis: mapping.BasisActual},
			Left:    r.net * 100,
			Right:   r.change * 100,
			Printed: fmt.Sprintf("Budget Book pp.%d-%d, fund %d's line", r.page, r.page+1, r.fund),
			Reason: "FY2023-24 actuals are printed rounded to the dollar figure by figure, so the " +
				"line's printed flows net to a dollar off its printed balances' difference",
			Bead: "fisc-3eh2",
		})
	}
	return out
}

// ValidateBalanceExceptions refuses a declaration that could not hold
// anything apart. It reads the declarations alone; HoldBalances reads the store.
func ValidateBalanceExceptions(balances []Balance, exceptions []BalanceException) error {
	seen := map[[2]string]bool{}
	for _, e := range exceptions {
		b, ok := BalanceOf(balances, e.At.Scope)
		switch {
		case e.Identity != BalanceCarryForward && e.Identity != BalanceSourcesUses:
			return fmt.Errorf("balance exception at %s names identity %q, which is not declared", e.At, e.Identity)
		case !ok:
			return fmt.Errorf("balance exception at %s is on a scope FundBalances does not declare", e.At)
		case e.Identity == BalanceSourcesUses && !b.SourcesUses:
			return fmt.Errorf("balance exception at %s holds sources = uses apart on a scope that does not hold it", e.At)
		case e.Left == e.Right:
			return fmt.Errorf("balance exception at %s pins %s on both sides; a balance that holds needs no exception",
				e.At, amount.Cents(e.Left))
		case e.Printed == "" || e.Reason == "" || e.Bead == "":
			return fmt.Errorf("balance exception at %s does not say where it is printed, why, and which bead keeps it", e.At)
		}
		k := [2]string{string(e.Identity), e.At.String()}
		if seen[k] {
			return fmt.Errorf("balance exception at %s is declared twice for %s", e.At, e.Identity)
		}
		seen[k] = true
	}
	return nil
}

func init() {
	if err := ValidateBalanceExceptions(FundBalances(), BalanceExceptions()); err != nil {
		panic("internal/structure: " + err.Error())
	}
}

// HoldBalances applies one identity's exceptions to the sides it computed,
// keyed by balance as [left, right]. It returns the balances an exception
// holds apart, and a finding for each exception that could not apply: one
// naming a balance the store does not produce, one over a balance that holds,
// and one whose pinned side has moved. A caller over a store that does not
// publish the tree's schedules passes the exceptions it declares, not the tree's.
func HoldBalances(identity BalanceIdentity, sides map[BalanceAt][2]int64,
	exceptions []BalanceException) (held map[BalanceAt]bool, findings []string) {
	held = map[BalanceAt]bool{}
	for _, e := range exceptions {
		if e.Identity != identity {
			continue
		}
		got, ok := sides[e.At]
		switch {
		case !ok:
			findings = append(findings, fmt.Sprintf("balance exception at %s on %s matches no balance, so it "+
				"excuses nothing; remove it rather than leaving a declaration that has stopped describing "+
				"the corpus (%s)", e.At, identity, e.Bead))
		case got[0] == got[1]:
			findings = append(findings, fmt.Sprintf("balance exception at %s on %s names a balance that holds "+
				"at %s, so it would excuse a figure that later moved; remove it (%s)",
				e.At, identity, amount.Cents(got[0]), e.Bead))
		case got != [2]int64{e.Left, e.Right}:
			findings = append(findings, fmt.Sprintf("balance exception at %s on %s pins %s and %s and the "+
				"store says %s and %s. %s (%s)", e.At, identity, amount.Cents(e.Left), amount.Cents(e.Right),
				amount.Cents(got[0]), amount.Cents(got[1]), e.Reason, e.Bead))
		default:
			held[e.At] = true
		}
	}
	return held, findings
}
