package check

import (
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/corpus"
)

// TestFundNameInReadsBothPrintedShapes pins the two forms this corpus prints and
// the exactness that separates them from a prefix match.
//
// WHY THE SECOND SHAPE EXISTS. A leading `Total ` cut answered every anchor that
// existed while the fund-bearing rules were the revenue schedule's: pp.131-140
// print `Total <fund>` and pp.127-130's rollup prints `Total General Fund`.
// pp.167-170 put the fund first -- p0170:23 is `General Fund Total Expenses`,
// mapped as the rollup gf-total-expenses over all 23 division rules -- so
// declaring `fund: 100` on those rules produced 23 findings reading "no printed
// total governing it names that fund: the totals governing it are ...,
// "General Fund Total Expenses", ...". The anchor was in the list it was
// reported as missing from.
//
// WHAT THIS IS NOT is a prefix match. data/funds.yaml refuses one in writing and
// the exactness is what keeps the five operating/CIP twins apart. The cases
// below assert that what is cut is the word Total and the schedule's own
// trailing noun, never a fragment of a fund name: the caller then resolves the
// whole candidate through FundByLabel, which answers or does not.
func TestFundNameInReadsBothPrintedShapes(t *testing.T) {
	cases := []struct {
		name  string
		total string
		want  string
		ok    bool
	}{
		{"leading, pp.131-140", "Total Airport", "Airport", true},
		{"leading, p130's rollup", "Total General Fund", "General Fund", true},
		{"trailing, p170's rollup", "General Fund Total Expenses", "General Fund", true},
		{"trailing, no noun after Total", "General Fund Total", "General Fund", true},

		// The whole candidate, never a fragment. "Water" and "Water
		// Replacement" are different funds (640, 642) and a prefix match is
		// what would confuse them.
		{"a twin keeps its whole name", "Total Water Replacement", "Water Replacement", true},
		{"a twin keeps its whole name, trailing", "Water Replacement Total Expenses",
			"Water Replacement", true},

		// pp.167-170's own division totals: the printed label is the bare word,
		// so there is no name to read and the anchor is skipped rather than
		// guessed at. This is why unclaimedFundTotals does not report them.
		{"a bare Total names nothing", "Total", "", false},

		// A category total is not a defect either -- pp.127-130 print ten.
		// It resolves to a candidate that FundByLabel will reject.
		{"a category total still yields a candidate", "Total Property Taxes",
			"Property Taxes", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := fundNameIn(c.total)
			if got != c.want || ok != c.ok {
				t.Errorf("fundNameIn(%q) = %q, %v, want %q, %v",
					c.total, got, ok, c.want, c.ok)
			}
		})
	}
}

// TestUnclaimedFundTotalsSeesBothPrintedShapes is clause 2's half of the same
// generalisation clause 1 got, and it was nearly missed.
//
// fundNameIn taught clause 1 to read `<fund> Total <noun>`; clause 2 kept its own
// leading-`Total ` regex and a claimed lookup that rebuilt "Total "+name. So on
// pp.167-170 -- swept for the first time when those rules declared fund 100 --
// `General Fund Total Expenses` could neither be matched nor claimed, and the
// check's PASS line said "every printed fund total on those pages is claimed"
// about a total it could not see. An overstatement in a summary is published
// verbatim on every run.
//
// The two subtests are the two halves that must both hold: the trailing shape is
// SEEN (so a real unclaimed one is reported) and it is CLAIMABLE under the label
// as printed (so the mapped one does not become a false finding).
func TestUnclaimedFundTotalsSeesBothPrintedShapes(t *testing.T) {
	const docID = "livermore-budget-fy2026-2027"
	// Two printed totals naming one fund, one in each shape, plus a bare
	// `Total` of the kind pp.167-170 actually print.
	const page = "" +
		"                          General Fund Expenditures by Major Category\n" +
		"Wages & Benefits                    $1,000         $2,000\n" +
		"                      Total          $1,000         $2,000\n" +
		"General Fund Total Expenses         $1,000         $2,000\n" +
		"Total General Fund                  $1,000         $2,000\n"

	pages := map[string]map[int]bool{docID: {170: true}}
	s := &Subject{
		Vocabulary: testVocabulary(t),
		Docs:       map[string]*corpus.Doc{docID: inlinePageDoc(t, docID, 170, page)},
	}

	t.Run("an unclaimed trailing-shape total is reported", func(t *testing.T) {
		// "Total General Fund" is claimed; the trailing one is not.
		claimed := map[string]map[claimKey]bool{docID: {{page: 170, label: "Total General Fund"}: true}}
		got := unclaimedFundTotals(s, pages, claimed)
		if len(got) != 1 {
			t.Fatalf("findings = %d, want exactly the unclaimed trailing total: %v",
				len(got), got)
		}
		if !strings.Contains(got[0].Detail, "General Fund Total Expenses") {
			t.Errorf("the finding does not name the trailing-shape total: %q", got[0].Detail)
		}
	})

	t.Run("a claimed trailing-shape total is not a finding", func(t *testing.T) {
		claimed := map[string]map[claimKey]bool{docID: {
			{page: 170, label: "Total General Fund"}:          true,
			{page: 170, label: "General Fund Total Expenses"}: true,
		}}
		if got := unclaimedFundTotals(s, pages, claimed); len(got) != 0 {
			t.Errorf("findings = %v, want none: both printed totals are claimed", got)
		}
	})

	t.Run("a bare Total names no fund and is skipped", func(t *testing.T) {
		// pp.167-170's division totals print the word alone. They must not be
		// reported, and they must not be guessed at either.
		if _, ok := fundNameIn("Total"); ok {
			t.Error(`fundNameIn("Total") resolved a name; the line names no fund`)
		}
		if _, ok := printedTotalLabel("                      Total          $1,000         $2,000"); !ok {
			t.Error("printedTotalLabel skipped a real total line; it is fundNameIn's job to " +
				"decide the line names no fund, not this one's")
		}
	})
}

// TestPrintedTotalLabelSplitsOnTheColumnGrid pins what makes a line a row.
func TestPrintedTotalLabelSplitsOnTheColumnGrid(t *testing.T) {
	cases := []struct {
		line string
		want string
		ok   bool
	}{
		{"Total Airport                    $4,739,507   $1", "Total Airport", true},
		{"General Fund Total Expenses      $144,650,802 $1", "General Fund Total Expenses", true},
		// A fund name with a single internal space survives: the split is on
		// TWO or more, which is the grid pdftotext -layout encodes.
		{"Total Water Replacement          $1  $2", "Total Water Replacement", true},
		// Prose, not a row: no figures after the label.
		{"Total Airport", "", false},
		{"        General Fund Expenditures by Major Category", "", false},
		// A row that is not a total at all.
		{"Wages & Benefits                 $1  $2", "", false},
	}
	for _, c := range cases {
		got, ok := printedTotalLabel(c.line)
		if got != c.want || ok != c.ok {
			t.Errorf("printedTotalLabel(%q) = %q, %v; want %q, %v",
				c.line, got, ok, c.want, c.ok)
		}
	}
}

// TestAClaimOnOnePageDoesNotSilenceAnotherPage is fisc-lkx.
//
// The `claimed` set was keyed on (doc_id, printed label) with NO page dimension,
// while this check's own stated claim is per page: "on the pages a schedule
// maps, the schedule maps all of it". So a total mapped on one page silenced an
// identically-worded total on every other swept page of the same document -- an
// entirely unmapped fund section on a swept page went invisible, which is the
// hole clause 2 exists to close.
//
// The corpus had no collision when the bead was filed and still has none, so
// this cannot be shown against committed pages: it is shown against two inline
// pages printing the same line, one of them claimed.
func TestAClaimOnOnePageDoesNotSilenceAnotherPage(t *testing.T) {
	const docID = "livermore-budget-fy2026-2027"
	const printed = "Total General Fund                  $1,000         $2,000\n"

	s := &Subject{
		Vocabulary: testVocabulary(t),
		Docs: map[string]*corpus.Doc{docID: inlinePagesDoc(t, docID, map[int]string{
			130: printed,
			166: printed,
		})},
	}
	pages := map[string]map[int]bool{docID: {130: true, 166: true}}

	// Claimed on p130 only -- which is where the real gf-total-revenues rollup
	// claims it.
	claimed := map[string]map[claimKey]bool{docID: {
		{page: 130, label: "Total General Fund"}: true,
	}}

	got := unclaimedFundTotals(s, pages, claimed)
	if len(got) != 1 {
		t.Fatalf("findings = %d, want exactly the unclaimed p166 total: %v", len(got), got)
	}
	if !strings.Contains(got[0].Subject, "p166") {
		t.Errorf("finding names %q, want the page the claim does not cover", got[0].Subject)
	}
	if strings.Contains(got[0].Subject, "p130") {
		t.Errorf("finding names p130, which IS claimed: %q", got[0].Subject)
	}

	// And claiming it on both pages silences both, so the fix rejects the
	// unclaimed page rather than the shared label.
	claimed[docID][claimKey{page: 166, label: "Total General Fund"}] = true
	if got := unclaimedFundTotals(s, pages, claimed); len(got) != 0 {
		t.Errorf("findings = %v, want none once both pages claim the total", got)
	}
}

// TestAWrappedTotalNamesItsFundWithItsTail runs the check over the committed
// corpus with one rule's total_row_tail removed. p0175 prints "Total County
// Meas BB-" and wraps "Bike/Pedestrian" onto the next line, so the truncated
// label names no fund and the rule is reported as anchored to nothing.
func TestAWrappedTotalNamesItsFundWithItsTail(t *testing.T) {
	s, err := Load(LoadOptions{Root: repoRoot(t), Version: testVersion})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	const id = "fund-exp-county-meas-bb-bike-pedestrian"
	res := resultFor(t, runOne(t, s, &ruleFundsMatchTheirHeadings{}), "rule-funds-match-their-headings")
	if res.Status != StatusPass {
		t.Fatalf("status = %s over the committed corpus, findings:\n  %v", res.Status, res.Findings)
	}
	found := false
	for _, f := range s.Files {
		for i := range f.Rules {
			if f.Rules[i].ID == id {
				if f.Rules[i].TotalRowTail == "" {
					t.Fatalf("%s declares no total_row_tail, so this mutation changes nothing", id)
				}
				f.Rules[i].TotalRowTail = ""
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("no rule %s in the committed mappings", id)
	}
	res = resultFor(t, runOne(t, s, &ruleFundsMatchTheirHeadings{}), "rule-funds-match-their-headings")
	if len(res.Findings) != 1 || res.Findings[0].Subject != id {
		t.Fatalf("findings = %v, want exactly %s's", res.Findings, id)
	}
	if want := `"Total County Meas BB-"`; !strings.Contains(res.Findings[0].Detail, want) {
		t.Errorf("finding does not quote the truncated label %s: %v", want, res.Findings[0])
	}
}

// TestAWrappedTotalNobodyMapsIsReported removes one wrapped fund's rule from
// the committed corpus. p181 prints "Total Open Space Acquisition &" and wraps
// "Mgmt" beneath it, so clause 2 finds the fund only by rejoining the two.
func TestAWrappedTotalNobodyMapsIsReported(t *testing.T) {
	s, err := Load(LoadOptions{Root: repoRoot(t), Version: testVersion})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	const id = "fund-exp-open-space-acquisition-mgmt"
	removed := false
	for _, f := range s.Files {
		for i := range f.Rules {
			if f.Rules[i].ID == id {
				f.Rules = append(f.Rules[:i], f.Rules[i+1:]...)
				removed = true
				break
			}
		}
	}
	if !removed {
		t.Fatalf("no rule %s in the committed mappings", id)
	}
	res := resultFor(t, runOne(t, s, &ruleFundsMatchTheirHeadings{}), "rule-funds-match-their-headings")
	if len(res.Findings) != 1 {
		t.Fatalf("findings = %v, want exactly p181's unclaimed wrapped total", res.Findings)
	}
	f := res.Findings[0]
	if !strings.Contains(f.Subject, "p181") || !strings.Contains(f.Detail, "fund 300") {
		t.Errorf("finding %v does not name p181 and fund 300", f)
	}
}
