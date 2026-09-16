package check

import (
	"slices"
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/amount"
	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/mapping"
)

// TestEveryToCIPConstantBelongsToExactlyOneClause is the test for a defect this
// check had while it was being written, and it is worth keeping because the
// symptom pointed away from the cause.
//
// addToCIP first added the WHOLE table to both out clauses. A constant under a
// group the clause does not compare produces a detail cell the spine cannot
// have, so capital+special-revenue's 28,693,590 landed in the major clause and
// enterprise's 9,353,147 in the non-major one. The check reported six findings
// -- "the detail publishes X here and the spine has no such cell" -- while
// every real cell tied to the cent, and the findings read as a mapping error.
func TestEveryToCIPConstantBelongsToExactlyOneClause(t *testing.T) {
	claimed := map[string]int{}
	for _, cl := range transfersClauses {
		for _, g := range cl.toCIPGroups {
			claimed[g]++
			// A constant may only be added under a group the clause actually
			// compares -- which for the collapsed clause is the synthetic key,
			// not its two members.
			want := cl.restriction.FundGroups
			if cl.collapse {
				want = []string{nonMajorGroup}
			}
			if !slices.Contains(want, g) {
				t.Errorf("clause %q adds a to-CIP constant for %q, which it does not "+
					"compare; the spine can have no such cell and the finding will "+
					"read as a mapping error", cl.name, g)
			}
		}
	}
	for _, g := range declaredToCIPGroups() {
		switch claimed[g] {
		case 1:
		case 0:
			t.Errorf("toCIP declares %q and no clause adds it back; that group's "+
				"transfers to the CIP are silently missing from the comparison", g)
		default:
			t.Errorf("%d clauses add toCIP[%q]; a constant counted twice ties by "+
				"accident", claimed[g], g)
		}
	}
	// And the two collapsed members are NOT in the table under their own names,
	// because pp.72-75 print no per-group figure for them.
	for _, g := range []string{"capital", "special-revenue"} {
		if _, ok := toCIP[g]; ok {
			t.Errorf("toCIP names %q; pp.72-75 print one aggregate for all non-major "+
				"funds, so a per-group figure here would be derived by difference "+
				"from p67 while the pair is published", g)
		}
	}
}

// TestTransfersDetailIsFailable is the evidence, and half of it is the
// statement of what CANNOT redden this check.
//
// Every mutation below is applied to the committed facts and run through the
// real check.
func TestTransfersDetailIsFailable(t *testing.T) {
	base := func(t *testing.T) *Subject {
		t.Helper()
		s, err := Load(LoadOptions{Root: repoRoot(t), Version: testVersion})
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		return s
	}
	run := func(t *testing.T, s *Subject) Result {
		t.Helper()
		res, err := (&transfersDetailTiesToSpine{}).Run(t.Context(), s)
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		return res
	}

	if got := run(t, base(t)); got.Status != StatusPass {
		t.Fatalf("the committed corpus does not pass: %s %s", got.Status, got.Summary)
	}

	// (1) A LEG MOVED ACROSS A CLAUSE BOUNDARY. The Wastewater -> Stormwater
	// payer redeclared as the General Fund: enterprise loses 460,000 and
	// general gains it, and both clause-(b) cells break.
	t.Run("a payer moved to another fund group", func(t *testing.T) {
		s := base(t)
		moved := 0
		for i := range s.Facts {
			f := &s.Facts[i]
			if f.Scope == transfersDetailScope && f.Kind == mapping.KindTransferOut &&
				f.FundGroup == "enterprise" && f.AmountCents == 46000000 {
				f.FundGroup, f.Fund = "general", fact.FundNumber(100)
				moved++
			}
		}
		if moved != 2 {
			t.Fatalf("moved %d legs, want 2 (one per budget year)", moved)
		}
		got := run(t, s)
		if got.Status != StatusFail {
			t.Fatalf("moving 460,000 between fund groups reported %s: %s",
				got.Status, got.Summary)
		}
		if len(got.Findings) != 4 {
			t.Errorf("got %d findings, want 4 (two groups x two years)", len(got.Findings))
		}
	})

	// (2) A DROPPED SECTION. The union, not the detail's keys, is what makes
	// this visible: deleting every permanent leg would otherwise remove the key
	// from both sides of a detail-keyed loop.
	t.Run("a whole section deleted", func(t *testing.T) {
		s := base(t)
		s.Facts = slices.DeleteFunc(slices.Clone(s.Facts), func(f fact.Fact) bool {
			return f.Scope == transfersDetailScope && f.FundGroup == "debt-service"
		})
		if got := run(t, s); got.Status != StatusFail {
			t.Fatalf("deleting the debt-service in-legs reported %s: %s",
				got.Status, got.Summary)
		}
	})

	// (3) AND WHAT CANNOT REDDEN IT, which is the argument for fisc-bhe. All
	// five of p76's ambiguous payer labels match an operating fund AND its CIP
	// twin, and every twin is type: capital -- so a leg declared under the
	// wrong twin moves INSIDE clause (c)'s collapsed cell, the joint sum is
	// unchanged, and this check stays green. Three of the five (510, 550, 560)
	// do not even change fund group, so factFundsResolve cannot see them
	// either.
	t.Run("a twin swap inside the collapsed cell is invisible", func(t *testing.T) {
		s := base(t)
		swapped := 0
		for i := range s.Facts {
			f := &s.Facts[i]
			// Low Income Hsng 200 (special-revenue) -> its CIP twin 812
			// (capital). Both are inside the collapsed pair.
			if f.Scope == transfersDetailScope && f.Fund != nil && *f.Fund == 200 {
				f.Fund, f.FundGroup = fact.FundNumber(812), "capital"
				swapped++
			}
		}
		if swapped == 0 {
			t.Fatal("no leg carries fund 200; the mutation tested nothing")
		}
		if got := run(t, s); got.Status != StatusPass {
			t.Errorf("a twin swap reddened this check: %s. That would be good news, "+
				"but it contradicts collapseNonMajor's stated cost and fisc-bhe's "+
				"reason for existing -- check which is now wrong", got.Summary)
		}
	})
}

// TestTransfersDetailIsVacuousWithoutTheSchedule is the arm that lets the check
// land at all: internal/check's shared fixture carries a spine with no p76
// behind it, so a literal "a spine key with no detail is a failure" would redden
// every test calling runChecks over a corpus with nothing wrong in it.
func TestTransfersDetailIsVacuousWithoutTheSchedule(t *testing.T) {
	res, err := (&transfersDetailTiesToSpine{}).Run(t.Context(), testSubject(t))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != StatusVacuous {
		t.Errorf("over a corpus with no p76 the check reported %s: %s", res.Status, res.Summary)
	}
	if !strings.Contains(res.Summary, transfersDetailScope) {
		t.Errorf("the vacuous summary does not name the scope it is waiting for: %s", res.Summary)
	}
}

// TestTheToCIPTableIsReadOffThePages holds the constants to the standard the
// file claims for them, because the difference between a check and a tautology
// is exactly here: none of these may be computed as spine minus detail.
//
// The four figures printed verbatim are asserted against the page text; the
// enterprise pair is asserted as the difference of two figures printed on the
// SAME page, which is what its comment says it is.
func TestTheToCIPTableIsReadOffThePages(t *testing.T) {
	for _, tt := range []struct {
		group string
		year  int
		want  amount.Cents
	}{
		{"internal-service", 2026, 4000000},
		{"internal-service", 2027, 61200000},
		{nonMajorGroup, 2026, 2869359000},
		{nonMajorGroup, 2027, 3593025100},
		{"general", 2026, 0},
		{"debt-service", 2027, 0},
	} {
		if got := toCIP[tt.group][tt.year]; got != tt.want {
			t.Errorf("toCIP[%q][%d] = %s, want %s", tt.group, tt.year, got, tt.want)
		}
	}

	// Total Major Funds to-CIP minus the printed internal-service line. Both
	// figures are on the same page as each other, which is what makes this a
	// reading and not a derivation from our own facts.
	for _, tt := range []struct {
		year                 int
		totalMajor, internal amount.Cents
	}{
		{2026, 939314700, 4000000},
		{2027, 1483200000, 61200000},
	} {
		if got, want := toCIP["enterprise"][tt.year], tt.totalMajor-tt.internal; got != want {
			t.Errorf("toCIP[enterprise][%d] = %s, but p007%d.txt:55 prints a Total Major "+
				"Funds to-CIP of %s and :53 an Internal Service total of %s, leaving %s",
				tt.year, got, map[int]int{2026: 3, 2027: 5}[tt.year],
				tt.totalMajor, tt.internal, want)
		}
	}
}

// TestTheTwoOutClausesPartitionEveryFundGroup is the test for the gap that
// mattered most in review, and the reason it is a partition rather than a
// coverage check is what a hole here does.
//
// permanent was in NEITHER out clause. pp.66-67 print no Permanent column, so
// no spine cell would ever have raised it and no finding would ever have named
// it -- a permanent payer would have been compared by nothing at all while the
// check went on reporting PASS and claiming the detail "must decompose the
// spine, never extend it". Measured before the fix: appending a $999,999.99
// permanent out-leg to the committed corpus left the check PASS with zero
// findings. p76 already prints a Permanent section on the receiving side, so
// that leg is one counterpart declaration away.
func TestTheTwoOutClausesPartitionEveryFundGroup(t *testing.T) {
	seen := map[string]int{}
	for _, cl := range transfersClauses {
		if cl.name == "in" {
			// The in clause pins no groups on purpose: p76's sections span five
			// and the spine publishes six.
			if len(cl.restriction.FundGroups) != 0 {
				t.Errorf("the in clause pins %v; it must span every group",
					cl.restriction.FundGroups)
			}
			continue
		}
		for _, g := range cl.restriction.FundGroups {
			seen[g]++
		}
	}
	for _, g := range clausesCoverEveryFundGroup {
		switch seen[g] {
		case 1:
		case 0:
			t.Errorf("no out clause compares fund group %q, so a transfer_out leg in it "+
				"is reconciled by nothing while the check reports PASS", g)
		default:
			t.Errorf("%d out clauses compare %q; a group counted twice is compared "+
				"against the spine twice", seen[g], g)
		}
	}
	for g := range seen {
		if !slices.Contains(clausesCoverEveryFundGroup, g) {
			t.Errorf("an out clause compares %q, which is in no fund-group list this "+
				"corpus uses", g)
		}
	}
}

// TestAPermanentPayerIsCompared is the same gap stated as arithmetic rather
// than as a list, because a partition test passes the moment somebody adds a
// string to two slices.
func TestAPermanentPayerIsCompared(t *testing.T) {
	s, err := Load(LoadOptions{Root: repoRoot(t), Version: testVersion})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// A payer p76 does not have, in the group that had no clause. The spine
	// publishes no permanent TRANSFER OUT cell at all, so this money extends
	// the spine rather than decomposing it -- which is the failure the check's
	// own finding text names.
	s.Facts = append(slices.Clone(s.Facts), fact.Fact{
		ID: "fisc-f-000000000000", DocID: "livermore-budget-fy2026-2027", Page: 76,
		RuleID: "p76-transfers-in-permanent", Kind: mapping.KindTransferOut,
		Basis: mapping.BasisAdopted, Scope: transfersDetailScope, FiscalYear: 2026,
		RowPath: "transfers/out", Category: "transfers/out",
		ColumnPath: "permanent/fund/300", FundGroup: "permanent", Fund: fact.FundNumber(300),
		Units: "dollars", AmountCents: 99999999,
	})

	res, err := (&transfersDetailTiesToSpine{}).Run(t.Context(), s)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != StatusFail {
		t.Fatalf("a permanent payer of $999,999.99 the spine does not publish reported "+
			"%s: %s", res.Status, res.Summary)
	}
	if !strings.Contains(joinFindings(res.Findings), "999,999.99") {
		t.Errorf("no finding names the amount: %v", res.Findings)
	}
}

// TestToCIPIsFoldedIntoTheAdoptedBasisOnly guards the second gap review found,
// and it is unreachable on today's corpus by design.
//
// pp.72-75 print one budget column per year and no revised or actual to-CIP
// figure, so this table holds adopted-column constants and nothing else.
// addToCIP originally matched on YEAR alone, which folded them into every basis
// the spine published for that year. The spine publishes only adopted today, so
// nothing reached it -- but reconciledPairs is deliberately dynamic, and the
// day pp.66-67 gain a revised column the check would have reported "the detail
// sums to $9,353,147" for a basis where p76 publishes nothing, blaming the
// mapping for a figure the check invented.
func TestToCIPIsFoldedIntoTheAdoptedBasisOnly(t *testing.T) {
	reconcile := map[yearBasis]bool{
		{2026, mapping.BasisAdopted}: true,
		{2026, mapping.BasisRevised}: true,
	}
	got := addToCIP(nil, []string{"enterprise"}, reconcile)

	if c := got[detailKey{2026, mapping.BasisAdopted, "enterprise", "transfers/out"}]; !c.present ||
		c.cents != toCIP["enterprise"][2026] {
		t.Errorf("the adopted cell is %v, want the printed constant %s",
			c, toCIP["enterprise"][2026])
	}
	if c := got[detailKey{2026, mapping.BasisRevised, "enterprise", "transfers/out"}]; c.present {
		t.Errorf("a revised cell was invented from an adopted-column constant: %v; "+
			"pp.72-75 print no revised to-CIP figure", c)
	}
	if len(got) != 1 {
		t.Errorf("addToCIP produced %d cells for one group and one printed basis: %v",
			len(got), got)
	}
}

// joinFindings renders findings for a message.
func joinFindings(fs []Finding) string {
	var b strings.Builder
	for _, f := range fs {
		b.WriteString(f.Subject + ": " + f.Detail + "\n")
	}
	return b.String()
}
