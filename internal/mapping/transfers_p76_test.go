package mapping

import (
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/amount"
)

// Budget Book p76, "SUMMARY OF TRANSFERS", reconciled against the citywide
// spine on pp.66-67. Both sides come out of the resolver: nothing below types
// in a figure that the engine could have read for it, except the three totals
// the page prints for itself, which are cited where they are used.
//
// This is a RECONCILIATION and not a mapping. testdata/transfers-p76.yaml is
// not under mappings/, no fact it produces is exported, and facts.jsonl does
// not move. What it buys is an answer to a question the shipped page was
// getting wrong: mapping p76 would NOT close the transfer residual, because
// p76's own grand total is the transfers-in side to the cent. See fisc-5gk.3.
const p76Page = 76

// The four figures the page prints for itself, PDF p76, the "$" row beneath the
// last section. The first two are the ones this reconciliation cannot
// reproduce; see TestP76ColumnsAgainstItsPrintedTotal.
const (
	p76StatedFY2024 amount.Cents = 2994767700
	p76StatedFY2025 amount.Cents = 2767970600
	p76StatedFY2026 amount.Cents = 2152599700
	p76StatedFY2027 amount.Cents = 2162463300
)

// p76Read is every value p76 produces, keyed the two ways the page can be read:
// by the fund group RECEIVING the money (which is how the page groups its
// sections) and by the fund PAYING it (which is only in the row label).
type p76Read struct {
	// byGroupYear sums the section a value came from, per fiscal year.
	byGroupYear map[groupYear]amount.Cents
	// bySourceYear sums the fund named after "Transfer From", per fiscal year.
	// The fact model cannot carry this -- fact.Fact has one `fund` field and a
	// transfer has two ends (fisc-4rh) -- so the label is split here to show
	// what the missing field would have to hold.
	bySourceYear map[sourceYear]amount.Cents
	// byColumn sums each of the four printed columns across all 22 rows.
	byColumn map[int]amount.Cents
	// rowLabels is every row the resolver actually READ, which is a claim about
	// the page and not about the rule file: a label must be found in the order
	// the rule lists it, anything between two rows fails checkGap, and anything
	// after the last row fails the unmapped-rest check. Counting len(rule.Rows)
	// instead would assert the YAML against itself.
	rowLabels map[string]bool
	values    int
}

type groupYear struct {
	group string
	year  int
}

type sourceYear struct {
	source string
	year   int
}

// readP76 resolves all six rules of the reconciliation fixture.
func readP76(t *testing.T) p76Read {
	t.Helper()

	f, err := Load("testdata/transfers-p76.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	r, err := NewResolver(budgetDoc(t, p76Page), f)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}

	out := p76Read{
		byGroupYear:  map[groupYear]amount.Cents{},
		bySourceYear: map[sourceYear]amount.Cents{},
		byColumn:     map[int]amount.Cents{},
		rowLabels:    map[string]bool{},
	}
	for i := range f.Rules {
		rule := &f.Rules[i]
		part := &rule.Parts[0]
		vals, _, err := r.Values(rule, part)
		if err != nil {
			t.Fatalf("rule %s: %v", rule.ID, err)
		}
		out.values += len(vals)

		// A source omitted from a row label continues from the row above: the
		// document's own shorthand, and the reason a transfer's payer cannot be
		// read off one row in isolation.
		// The source carries forward WITHIN a rule, and starts empty in each
		// one, because a rule is a block of the page and its first row must
		// name its own payer. If the fisc-0cs split (enterprise-a/-b) ever
		// moves onto one of the three continuation rows, that row's money would
		// otherwise be filed under no payer at all and every total below would
		// still add up.
		source := ""
		for _, v := range vals {
			if s, ok := transferSource(v.Row.Label); ok {
				source = s
			}
			if source == "" {
				t.Fatalf("rule %s row %q: no payer named by it or any row above "+
					"it in this block, so the source side cannot be read",
					rule.ID, v.Row.Label)
			}
			out.rowLabels[v.Row.Label] = true
			out.byGroupYear[groupYear{v.Column.FundGroup, v.Column.FiscalYear}] += v.Cents
			out.bySourceYear[sourceYear{source, v.Column.FiscalYear}] += v.Cents
			out.byColumn[v.ColumnIndex] += v.Cents
		}
	}
	return out
}

// transferSource returns the fund named after "Transfer From" in a p76 row
// label, and false for the three rows that omit it.
func transferSource(label string) (string, bool) {
	const prefix = "Transfer From "
	if !strings.HasPrefix(label, prefix) {
		return "", false
	}
	name, _, ok := strings.Cut(label[len(prefix):], "  ")
	if !ok {
		return "", false
	}
	return strings.TrimSpace(name), true
}

// TestP76IsAnOrdinaryLabelledRead is the claim that retired this page's
// "unextractable, hand-transcribe it" bead. The premise was xberg-era: the
// poppler migration (fisc-yqv) turned four disjoint blocks with mismatched
// cardinalities into 22 printed lines, and nobody went back to re-read it.
//
// 22 rows x 4 columns, no omissions and no positional guessing.
func TestP76IsAnOrdinaryLabelledRead(t *testing.T) {
	got := readP76(t)

	if len(got.rowLabels) != 22 {
		t.Errorf("got %d rows read, want 22", len(got.rowLabels))
	}
	if got.values != 88 {
		t.Errorf("got %d values, want 88 (22 rows x 4 columns)", got.values)
	}
	if len(got.byColumn) != 4 {
		t.Errorf("got %d columns, want 4", len(got.byColumn))
	}
}

// TestP76ColumnsAgainstItsPrintedTotal is the page checking our work, and it
// only half agrees.
//
// Both BUDGET columns tie exactly. Both HISTORICAL columns miss by millions --
// one of them by exactly $5,000,000 -- which is nothing like the <= $5 rounding
// declared through stated_total_deltas for p127 (fisc-2sd). Whatever those two
// columns are, they are not the city's rounding, and declaring them away would
// be a false claim about the document. The reconciliation therefore stands on
// the budget years, which are the only years this project publishes.
func TestP76ColumnsAgainstItsPrintedTotal(t *testing.T) {
	got := readP76(t)

	for _, tc := range []struct {
		name   string
		column int
		stated amount.Cents
		delta  amount.Cents
	}{
		{"FY2023-24 actual", 0, p76StatedFY2024, 685805100},
		{"FY2024-25 revised", 1, p76StatedFY2025, 500000000},
		{"FY2025-26 budget", 2, p76StatedFY2026, 0},
		{"FY2026-27 budget", 3, p76StatedFY2027, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mapped := got.byColumn[tc.column]
			if diff := tc.stated - mapped; diff != tc.delta {
				t.Errorf("got stated %s minus mapped %s = %s, want %s",
					tc.stated, mapped, diff, tc.delta)
			}
		})
	}
}

// TestP76TiesToTheSpinePerFundGroup is the strongest statement this fixture
// makes, and the one that decides how p76 may ever be published: every section
// total equals the matching TRANSFER IN cell on pp.66-67 EXACTLY. p76 detail
// and the spine are the same money, so a rule that published these rows at
// scope all-funds-gross would double the city's transfers with every check
// still green -- fisc-u2v's mechanism, one lane over (fisc-5gk.3.1).
func TestP76TiesToTheSpinePerFundGroup(t *testing.T) {
	p76 := readP76(t)
	spine := readSpineTransfers(t)

	for _, year := range []int{2026, 2027} {
		for _, group := range []string{"general", "enterprise", "debt-service", "special-revenue"} {
			key := groupYear{group, year}
			if got, want := p76.byGroupYear[key], spine.in[key]; got != want {
				t.Errorf("FY%d %s: p76 sections total %s, spine TRANSFER IN prints %s",
					year, group, got, want)
			}
		}

		// Permanent Funds is the seventh fund type and pp.66-67 print no column
		// for it (fisc-u8o). p76 does print a Permanent section, so the two
		// schedules can only agree while it is zero -- which it is in both
		// budget years, and is not in FY2023-24.
		if got := p76.byGroupYear[groupYear{"permanent", year}]; got != 0 {
			t.Errorf("FY%d permanent: got %s, want 0; the spine has no column to hold it", year, got)
		}
	}
	if got := p76.byGroupYear[groupYear{"permanent", 2024}]; got == 0 {
		t.Error("FY2024 permanent is zero, so this page no longer evidences fisc-u8o")
	}
}

// TestP76AccountsForTheInSideAndNoneOfTheResidual is why the caveat on the
// shipped page had to change. It used to read "the p76 transfer schedule is not
// yet mapped, SO transfers out exceed transfers in", which tells a reader the
// gap is this project's backlog. It is not: p76's grand total IS the
// transfers-in side, so the city itemises every transfer received and none of
// the difference.
func TestP76AccountsForTheInSideAndNoneOfTheResidual(t *testing.T) {
	p76 := readP76(t)
	spine := readSpineTransfers(t)

	for _, tc := range []struct {
		year   int
		column int
	}{{2026, 2}, {2027, 3}} {
		in, out := spine.totalIn(tc.year), spine.totalOut(tc.year)
		if got := p76.byColumn[tc.column]; got != in {
			t.Errorf("FY%d: p76's 22 rows total %s, spine TRANSFER IN totals %s",
				tc.year, got, in)
		}
		if out <= in {
			t.Fatalf("FY%d: transfers out %s does not exceed transfers in %s, "+
				"so this test no longer describes the document", tc.year, out, in)
		}

		// The whole of Capital Funds and Internal Service Funds transfers out is
		// unexplained: p76 prints no section for either, because neither
		// RECEIVES a transfer. That alone is most of the residual, and no amount
		// of reading p76 more carefully will find it.
		unexplained := spine.out[groupYear{"capital", tc.year}] +
			spine.out[groupYear{"internal-service", tc.year}]
		if unexplained == 0 {
			t.Fatalf("FY%d: no capital or internal-service transfers out, "+
				"so the residual decomposition below means nothing", tc.year)
		}
		if unexplained > out-in {
			t.Errorf("FY%d: capital + internal-service out %s exceeds the whole residual %s",
				tc.year, unexplained, out-in)
		}
	}

	// FY2026 in full, because it is the projection the site publishes and the
	// figure it states as headline.transfer_residual_cents.
	const residual amount.Cents = 3808673700
	if got := spine.totalOut(2026) - spine.totalIn(2026); got != residual {
		t.Errorf("got FY2026 residual %s, want %s", got, residual)
	}
}

// TestP76NamesThePayerThatTheFactModelCannot reads the source side, which is
// the half of a transfer that has nowhere to live: fact.Fact carries one `fund`
// field and every one of these rows has two ends (fisc-4rh). The General Fund's
// out-flows tie to the spine exactly, so the pairing this page publishes is
// real and checkable -- it just cannot be expressed as facts yet.
func TestP76NamesThePayerThatTheFactModelCannot(t *testing.T) {
	p76 := readP76(t)
	spine := readSpineTransfers(t)

	for _, year := range []int{2026, 2027} {
		got := p76.bySourceYear[sourceYear{"General Fund", year}]
		want := spine.out[groupYear{"general", year}]
		if got != want {
			t.Errorf("FY%d: p76 transfers paid by the General Fund total %s, "+
				"spine TRANSFER OUT prints %s", year, got, want)
		}
	}
}

// spineTransfers is the TRANSFER IN and TRANSFER OUT rows of pp.66-67, per
// fund group and fiscal year, read from the PUBLISHED mapping rather than from
// figures typed into this file.
type spineTransfers struct {
	in  map[groupYear]amount.Cents
	out map[groupYear]amount.Cents
}

func (s spineTransfers) totalIn(year int) amount.Cents  { return totalFor(s.in, year) }
func (s spineTransfers) totalOut(year int) amount.Cents { return totalFor(s.out, year) }

func totalFor(m map[groupYear]amount.Cents, year int) amount.Cents {
	var sum amount.Cents
	for k, v := range m {
		if k.year == year {
			sum += v
		}
	}
	return sum
}

func readSpineTransfers(t *testing.T) spineTransfers {
	t.Helper()

	f, err := Load(publishedSpine)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	r, err := NewResolver(budgetDoc(t, 66, 67), f)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}

	out := spineTransfers{in: map[groupYear]amount.Cents{}, out: map[groupYear]amount.Cents{}}
	for i := range f.Rules {
		rule := &f.Rules[i]
		var dst map[groupYear]amount.Cents
		switch rule.Kind {
		case KindTransferIn:
			dst = out.in
		case KindTransferOut:
			dst = out.out
		default:
			continue
		}
		for j := range rule.Parts {
			part := &rule.Parts[j]
			vals, _, err := r.Values(rule, part)
			if err != nil {
				t.Fatalf("rule %s p%d: %v", rule.ID, part.Page, err)
			}
			for _, v := range vals {
				dst[groupYear{v.Column.FundGroup, v.Column.FiscalYear}] += v.Cents
			}
		}
	}
	if len(out.in) == 0 || len(out.out) == 0 {
		t.Fatal("the published spine produced no transfer rows, so every " +
			"comparison against it below is vacuous")
	}
	return out
}
