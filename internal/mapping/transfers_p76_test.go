package mapping

import (
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/amount"
	"github.com/jcrussell/livermore-budget/internal/registry"
	"github.com/jcrussell/livermore-budget/internal/vocab"
)

// p76PublishedScope is the scope the published p76 rules carry, decided by
// fisc-aes: p76 and pp.66-67 are the same money, so a rule at the spine's scope
// would double the city's transfers with every check still green.
const p76PublishedScope = "transfers-by-fund"

// realDataDir is data/, holding funds.yaml and the other two registries.
// internal/registry's own tests read the same tree, for the same reason: an
// alias is a claim about a printed page, and checking it against the real file
// is what keeps it one.
const realDataDir = "../../data"

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
			if s, ok := transferSource(v.Row); ok {
				source = s
			}
			if source == "" {
				t.Fatalf("rule %s row %q: no payer named by it or any row above "+
					"it in this block, so the source side cannot be read",
					rule.ID, v.Row.PrintedLabel())
			}
			out.rowLabels[v.Row.PrintedLabel()] = true
			out.byGroupYear[groupYear{v.Column.FundGroup, v.Column.FiscalYear}] += v.Cents
			out.bySourceYear[sourceYear{source, v.Column.FiscalYear}] += v.Cents
			out.byColumn[v.ColumnIndex] += v.Cents
		}
	}
	return out
}

// transferSource returns the fund named after "Transfer From" in a p76 row
// label, and false for the three rows that omit it.
// transferSource reads the paying fund off a row's FIRST anchor.
//
// It used to cut the label on a run of two spaces, because the label was the
// whole printed line and the two fields were separated by the page's own
// kerning. That made this helper a second copy of the defect fisc-ffy was
// filed for -- it would have broken on a re-layout exactly as the rule file
// would. With Row.LabelTail the source IS Row.Label and the destination is
// Row.LabelTail, so there is nothing left to split.
func transferSource(row Row) (string, bool) {
	const prefix = "Transfer From "
	if !strings.HasPrefix(row.Label, prefix) || row.LabelTail == "" {
		return "", false
	}
	return strings.TrimSpace(row.Label[len(prefix):]), true
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

		// Capital Funds and Internal Service Funds RECEIVE nothing -- p76
		// prints no section for either -- so most of what they send is
		// unexplained by this page. Not all of it: p76 lists three
		// capital-sourced payers (Traffic Impact Fee, County Measure D, State
		// - Gas Tax), 211,150 in FY2026. This bound is therefore loose on
		// purpose, and TestP76SourcesDecomposeTheResidualByFundType below is
		// the exact statement.
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
		// THE SCOPE, NOT THE KIND. This read is of pp.66-67 and the doc built
		// above holds only those two pages, so a transfer rule from any other
		// schedule fails here with "not listed in the extraction manifest"
		// rather than being quietly summed. It stopped being hypothetical when
		// fisc-5gk.1 published four transfers-only fund rules on p134.
		if rule.Scope != publishedSpineScope {
			continue
		}
		var dst map[groupYear]amount.Cents
		switch rule.Kind {
		case vocab.KindTransferIn:
			dst = out.in
		case vocab.KindTransferOut:
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

// TestP76SourcesDecomposeTheResidualByFundType is the arithmetic that was
// missing, and its absence is why a wrong table shipped.
//
// TestP76AccountsForTheInSideAndNoneOfTheResidual asserts only that capital
// plus internal-service transfers out do not EXCEED the residual. That bound is
// satisfied by a wide range of wrong decompositions, and one of them was
// published: docs/sankey-contract.md carried "Capital 28,584,740 / Enterprise
// 9,353,147 / Special Revenue 108,850 / ISF 40,000" and the sentence "Capital
// and Internal Service pay nothing p76 lists". p76 lists 211,150 of
// capital-sourced payments in FY2026 -- Traffic Impact Fee (510), County
// Measure D (550) and State - Gas Tax (560) are type: capital in
// data/funds.yaml -- and that money was attributed to Special Revenue instead.
// Both rows were wrong and every published figure still added up to
// 38,086,737, because the error moved money between two rows of the same total.
//
// WHAT MAKES THIS EXPRESSIBLE NOW. Attributing a payment to a fund GROUP needs
// payer-name -> fund -> type, and fisc-8dz landed the alias channel that does
// the first hop: p76 prints "Low Income Hsng" and FundByLabel resolves it to
// fund 200 exactly, refusing rather than guessing between the operating fund
// and its CIP twin. Before that this test could not have been written, which
// is the honest reason it did not exist rather than an oversight.
//
// THE IDENTITY. For every fund group, the spine's TRANSFER OUT is what p76
// shows that group paying plus what the city routes to CIP:
//
//	spine_TRANSFER_OUT[group] == p76_paid[group] + to_CIP[group]
//
// and the to-CIP terms sum to headline.transfer_residual_cents. The right-hand
// side per group is DERIVED here (pp.72-75 print to-CIP per major fund and one
// aggregate for all non-major funds), so what this test pins is the derivation,
// against figures the city does print: p0073.txt:58 and p0075.txt:58.
func TestP76SourcesDecomposeTheResidualByFundType(t *testing.T) {
	p76 := readP76(t)
	spine := readSpineTransfers(t)

	reg, err := registry.Load(os.DirFS(realDataDir))
	if err != nil {
		t.Fatalf("registry.Load(%s): %v", realDataDir, err)
	}

	// Every payer p76 names, bucketed by the fund type data/funds.yaml gives
	// it. An unresolvable payer is fatal, not skipped: a silently dropped
	// payer moves its money out of paid and into to-CIP, which is precisely
	// the defect this test exists to catch.
	paid := map[groupYear]amount.Cents{}
	for sy, cents := range p76.bySourceYear {
		fund, err := reg.FundByLabel(sy.source)
		if err != nil {
			t.Fatalf("p76 payer %q does not resolve to a fund: %v", sy.source, err)
		}
		paid[groupYear{fund.Type, sy.year}] += cents
	}

	// The city's own Transfers Out to CIP column, per group, by difference.
	// Derived, so it is named as such and checked against what IS printed.
	want := map[groupYear]amount.Cents{
		{"general", 2026}: 0, {"general", 2027}: 0,
		{"enterprise", 2026}: 935314700, {"enterprise", 2027}: 1422000000,
		{"internal-service", 2026}: 4000000, {"internal-service", 2027}: 61200000,
		{"debt-service", 2026}: 0, {"debt-service", 2027}: 0,
		{"capital", 2026}: 2837359000, {"capital", 2027}: 3583025100,
		{"special-revenue", 2026}: 32000000, {"special-revenue", 2027}: 10000000,
	}

	// p0073.txt:58 and p0075.txt:58, the "Transfers Out to CIP" grand total.
	// These the city prints; everything above is the split it does not.
	residual := map[int]amount.Cents{2026: 3808673700, 2027: 5076225100}

	for _, year := range []int{2026, 2027} {
		var total amount.Cents
		for _, group := range []string{"general", "enterprise", "internal-service",
			"debt-service", "capital", "special-revenue"} {
			key := groupYear{group, year}
			toCIP := spine.out[key] - paid[key]
			if toCIP != want[key] {
				t.Errorf("FY%d %s: spine transfers out %s less p76 payments %s "+
					"= %s, want to-CIP %s", year, group, spine.out[key],
					paid[key], toCIP, want[key])
			}
			if toCIP < 0 {
				t.Errorf("FY%d %s: p76 shows this group paying %s against a spine "+
					"TRANSFER OUT of %s, so a negative to-CIP of %s would be "+
					"required and the attribution is wrong", year, group,
					paid[key], spine.out[key], toCIP)
			}
			total += toCIP
		}
		if total != residual[year] {
			t.Errorf("FY%d: the to-CIP terms sum to %s, want the printed %s",
				year, total, residual[year])
		}
	}

	// And the claim the whole page exists to make: what p76 DOES itemise is
	// the in side, entire. Stated here beside the out side so the asymmetry
	// is on one screen.
	for _, year := range []int{2026, 2027} {
		var paidTotal amount.Cents
		for _, group := range []string{"general", "enterprise", "internal-service",
			"debt-service", "capital", "special-revenue"} {
			paidTotal += paid[groupYear{group, year}]
		}
		if got := spine.totalIn(year); got != paidTotal {
			t.Errorf("FY%d: p76 payments total %s against a spine transfers in of %s; "+
				"the page itemises every transfer received, so these must be equal",
				year, paidTotal, got)
		}
	}
}

// TestP76ReadsUnderTheColumnGuardWithAHeaderlessColumn is fisc-wfi's evidence,
// and it is on the page rather than on a fixture of my own construction.
//
// p76 is the first part in the corpus with a column the page prints no header
// over, and until this it had no column_headers at all -- so the guard had
// never run on this page in either direction. Three things are asserted, and
// the third is the one that makes the first two mean anything.
func TestP76ReadsUnderTheColumnGuardWithAHeaderlessColumn(t *testing.T) {
	load := func(t *testing.T) *File {
		t.Helper()
		f, err := Load("testdata/transfers-p76.yaml")
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		return f
	}
	readAll := func(t *testing.T, f *File) error {
		t.Helper()
		r, err := NewResolver(budgetDoc(t, p76Page), f)
		if err != nil {
			return err
		}
		for i := range f.Rules {
			rule := &f.Rules[i]
			if _, _, err := r.Values(rule, &rule.Parts[0]); err != nil {
				return err
			}
		}
		return nil
	}

	// (1) The guard is DECLARED. Every part carries five entries for five
	// columns, the last of them null.
	f := load(t)
	for i := range f.Rules {
		p := &f.Rules[i].Parts[0]
		if len(p.ColumnHeaders) != len(p.Columns) {
			t.Fatalf("rule %s: %d headers for %d columns", f.Rules[i].ID,
				len(p.ColumnHeaders), len(p.Columns))
		}
		last := p.ColumnHeaders[len(p.ColumnHeaders)-1]
		if !last.Unheaded {
			t.Errorf("rule %s: last header is %s, want null -- the page prints "+
				"nothing over the footnote-marker column", f.Rules[i].ID, last)
		}
		if !p.Columns[len(p.Columns)-1].Skip {
			t.Errorf("rule %s: the headerless column is not skipped", f.Rules[i].ID)
		}
	}

	// (2) And the whole page still reads. All 22 rows place their four figures
	// in the four bands the four printed headers describe; the marker, which
	// has no band, is carried by the column count and the same-line check.
	if err := readAll(t, f); err != nil {
		t.Fatalf("p76 does not read under its own column guard: %v", err)
	}

	// (3) EXACTLY ONE COLUMN IS UNBANDED, and it is the skipped one. This is
	// the claim the null entry is really making, and a passing read alone does
	// not distinguish it from a guard that quietly checks nothing: a band map
	// of all -1 would also let p76 through.
	r, err := NewResolver(budgetDoc(t, p76Page), f)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	g, err := r.guard(&f.Rules[0], &f.Rules[0].Parts[0])
	if err != nil {
		t.Fatalf("guard: %v", err)
	}
	if g == nil {
		t.Fatal("no guard was built for a part declaring column_headers")
	}
	if want := []int{0, 1, 2, 3, -1}; !slices.Equal(g.band, want) {
		t.Errorf("band map = %v, want %v: the four printed headers give four bands "+
			"and only the footnote column is unplaceable", g.band, want)
	}

	// (4) THE HEADER LIST IS STILL A LIVE CLAIM ABOUT THE PAGE. Swapping two
	// entries describes p76's columns in an order it does not print them in,
	// and the read must refuse rather than shift every figure one band. Note
	// what this catches is the ORDER claim in matchHeaders; the placement claim
	// is (3)'s band map plus the read in (2).
	swapped := load(t)
	for i := range swapped.Rules {
		h := swapped.Rules[i].Parts[0].ColumnHeaders
		h[2], h[3] = h[3], h[2]
	}
	if err := readAll(t, swapped); err == nil {
		t.Fatal("swapping two column headers changed nothing; the guard is not running " +
			"on this page and the null entry silently disabled it")
	} else if !strings.Contains(err.Error(), "column_headers") {
		t.Errorf("error %q does not point at column_headers", err)
	}

	// (5) And a null in the MIDDLE is refused rather than quietly declining a
	// check the grid could have made. Asserted here as well as in parse_test
	// because this is the page the rule was written for.
	mid := load(t)
	for i := range mid.Rules {
		p := &mid.Rules[i].Parts[0]
		p.Columns[1], p.Columns[4] = p.Columns[4], p.Columns[1]
		p.ColumnHeaders[1], p.ColumnHeaders[4] = p.ColumnHeaders[4], p.ColumnHeaders[1]
	}
	if err := mid.Validate(); err == nil {
		t.Error("a null between two headers was accepted; it sits in a gap with known " +
			"bounds that the grid would have checked")
	}
}

// TestPublishedP76SumsToItsPrintedGrandTotal is fisc-9m7's answer, and the
// reason it is a Go test rather than a rollup is a property of the page.
//
// p76 prints ONE total for the whole schedule, beneath its last section, and
// prints no label over it: p0076.txt:64 is a bare "$29,947,677  $27,679,706
// $21,525,997  $21,624,633". So no rule can carry it as total_row -- there is
// no anchor -- and a Rollup is refused twice over besides: a covered rule must
// print its own total, and covered rules must declare the same columns, which
// these five do not because each names its own receiving fund_group. An
// `unassertable` rollup would still have to name a total_row it would be
// inventing.
//
// WHAT MAKES THIS DIFFERENT FROM THE FIXTURE'S VERSION is which rules it reads.
// TestP76ColumnsAgainstItsPrintedTotal runs over testdata/transfers-p76.yaml
// and checks a file nothing publishes; this runs over mappings/, so the page's
// own arithmetic is a check on the rules that produce facts. Both are wanted:
// the fixture covers all four printed columns and pins the two historical
// deltas, which the published rules skip and could not otherwise state.
//
// The delta here is ZERO in both columns, so this is an equality and not a
// tolerance -- there is no rounding on this page's budget years to absorb.
func TestPublishedP76SumsToItsPrintedGrandTotal(t *testing.T) {
	f, err := Load(publishedSpine)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	r, err := NewResolver(budgetDoc(t, p76Page), f)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}

	byYear := map[int]amount.Cents{}
	rules, rows := 0, 0
	for i := range f.Rules {
		rule := &f.Rules[i]
		if rule.Scope != p76PublishedScope {
			continue
		}
		rules++
		// One part each, and the doc above holds only p76, so a rule of this
		// scope on any other page would fail here rather than be summed.
		vals, _, err := r.Values(rule, &rule.Parts[0])
		if err != nil {
			t.Fatalf("rule %s: %v", rule.ID, err)
		}
		rows += len(rule.ActiveRows(&rule.Parts[0]))
		for _, v := range vals {
			byYear[v.Column.FiscalYear] += v.Cents
		}
	}

	// The shape first, so a sum that ties because half the page went missing
	// is not read as agreement.
	if rules != 5 {
		t.Errorf("read %d published p76 rules, want 5 -- one per printed section", rules)
	}
	if rows != 22 {
		t.Errorf("read %d rows, want the 22 transfers the page prints", rows)
	}

	for _, tc := range []struct {
		year   int
		stated amount.Cents
	}{
		{2026, p76StatedFY2026},
		{2027, p76StatedFY2027},
	} {
		if got := byYear[tc.year]; got != tc.stated {
			t.Errorf("FY%d: the five published rules sum to %s, and the page prints %s "+
				"as its grand total (a difference of %s); this is the only arithmetic "+
				"p76 offers against itself",
				tc.year, got, tc.stated, tc.stated-got)
		}
	}
	// And nothing else was published: the two historical columns are skipped,
	// because they miss this same printed total by millions and pp.66-67 print
	// no column to tie them to either.
	if len(byYear) != 2 {
		t.Errorf("the published rules produced %d fiscal years, want just the two "+
			"budget columns: %v", len(byYear), byYear)
	}
}
