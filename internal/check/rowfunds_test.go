package check

import (
	"strings"
	"testing"
)

// TestRowFundsCatchesTheTwinSwapNothingElseSees is this check's whole reason for
// existing, stated as the mutation the rest of the suite is blind to.
//
// Budget Book p76's payer side is hand-typed 44 times and the DOCUMENT cannot
// check it: a counterpart is resolved downstream of every comparison against the
// city's own arithmetic, so redeclaring a payer changes no printed total.
// Measured elsewhere in this repository and relied on here.
//
// Five of p76's payer labels match an operating fund AND its CIP twin, and every
// twin is type: capital. So:
//
//   - transfers-detail-ties-to-spine cannot see it: the swap moves the leg
//     inside the collapsed {capital, special-revenue, permanent} cell and the
//     joint sum is unchanged.
//   - factFundsResolve cannot see three of the five (510, 550, 560), because
//     those are already capital and the fund group does not change.
//
// This test runs the two hardest cases: the one that changes fund group and the
// one that does not.
func TestRowFundsCatchesTheTwinSwapNothingElseSees(t *testing.T) {
	for _, tt := range []struct {
		name          string
		anchor        string
		from, to      int
		alsoSetGroup  string
		printedFundNo int
	}{{
		name: "an operating fund swapped for its CIP twin, same type",
		// Traffic Imp Fee 510 -> CIP Traffic Impact Fee 823. Both capital, so
		// the fund GROUP does not change and factFundsResolve stays green.
		anchor: "Transfer From Traffic Imp Fee", from: 510, to: 823, printedFundNo: 510,
	}, {
		name: "an operating fund swapped for its CIP twin, across types",
		// Low Income Hsng 200 (special-revenue) -> 812 (capital). The group has
		// to move with it or factFundsResolve reddens first, which would make
		// this test prove the wrong thing.
		anchor: "Transfer From Low Income Hsng", from: 200, to: 812,
		alsoSetGroup: "capital", printedFundNo: 200,
	}} {
		t.Run(tt.name, func(t *testing.T) {
			s, err := Load(LoadOptions{Root: repoRoot(t), Version: testVersion})
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			swapped := 0
			for _, f := range s.Files {
				for i := range f.Rules {
					for j := range f.Rules[i].Rows {
						row := &f.Rules[i].Rows[j]
						if row.Counterpart == nil || row.Counterpart.Fund != tt.from ||
							row.Label != tt.anchor {
							continue
						}
						row.Counterpart.Fund = tt.to
						if tt.alsoSetGroup != "" {
							row.Counterpart.FundGroup = tt.alsoSetGroup
						}
						swapped++
					}
				}
			}
			if swapped != 1 {
				t.Fatalf("swapped %d rows, want 1; the mutation missed its target", swapped)
			}

			res, err := (&rowFundsMatchTheirAnchors{}).Run(t.Context(), s)
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			if res.Status != StatusFail {
				t.Fatalf("swapping fund %d for its twin %d reported %s: %s",
					tt.from, tt.to, res.Status, res.Summary)
			}
			var b strings.Builder
			for _, f := range res.Findings {
				b.WriteString(f.Subject + ": " + f.Detail + "\n")
			}
			for _, want := range []string{tt.anchor, "fund", "no other check sees it"} {
				if !strings.Contains(b.String(), want) {
					t.Errorf("no finding mentions %q:\n%s", want, b.String())
				}
			}
		})
	}
}

// TestTheCommittedCorpusRowAnchorsHold pins what the check covers today,
// including what it does NOT, because the summary is printed on every run and a
// reader counting rows would otherwise have to guess.
func TestTheCommittedCorpusRowAnchorsHold(t *testing.T) {
	s, err := Load(LoadOptions{Root: repoRoot(t), Version: testVersion})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	res, err := (&rowFundsMatchTheirAnchors{}).Run(t.Context(), s)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != StatusPass {
		t.Fatalf("%s: %s", res.Status, res.Summary)
	}

	// THE THREE ROWS THE PAGE LEAVES UNANCHORED, named rather than counted.
	// p76 prints three continuation rows whose "Transfer From" carries over
	// from the row above, so their declared payer has no printed label on its
	// own line (fisc-30j). Three of 22 rows are unguarded on the side where the
	// operating/CIP ambiguity lives, and the summary has to say so.
	for _, want := range []string{
		`"to Wastewater Replacement" (the fund that pays, 620)`,
		`"to 2022 COPS" (the fund that pays, 100)`,
		`"to Downtown LMD" (the fund that pays, 100)`,
	} {
		if !strings.Contains(res.Summary, want) {
			t.Errorf("the summary does not name the unanchored row %s:\n%s", want, res.Summary)
		}
	}
	if !strings.Contains(res.Summary, "3 declared fund(s) have no printed anchor") {
		t.Errorf("the summary does not count the unanchored declarations:\n%s", res.Summary)
	}
}

// TestRowFundsIsVacuousWithoutAPerRowSchedule is the arm that lets it be
// registered: the shared fixture is a miniature of the spine, whose fund
// dimension is per column.
func TestRowFundsIsVacuousWithoutAPerRowSchedule(t *testing.T) {
	res, err := (&rowFundsMatchTheirAnchors{}).Run(t.Context(), testSubject(t))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != StatusVacuous {
		t.Errorf("over a corpus whose funds are all per column the check reported %s: %s",
			res.Status, res.Summary)
	}
	if !strings.Contains(res.Summary, "rule-funds-match-their-headings") {
		t.Errorf("the vacuous summary does not name the check that covers the "+
			"per-column case instead: %s", res.Summary)
	}
}

// TestRowFundsCatchesASameGroupEndSwap is the direction half, and it exists
// because the argument for NOT checking direction was measured and found false.
//
// The first version of this check asserted only that each anchor's fund was one
// of the two the row declared, on the reasoning that swapping a row's ends moves
// money between the in and out sides of a fund group and
// transfers-detail-ties-to-spine reddens on it. That holds only when the two
// ends are in DIFFERENT groups. "Transfer From Water  to Water Replacement" is
// 640 and 642, both enterprise: swap them and the payer and payee are genuinely
// inverted in the published facts while every group sum is unchanged and
// `fisc verify` reports zero failures. "Transfer From Wastewater  to
// Stormwater" (620/610) is the same shape.
func TestRowFundsCatchesASameGroupEndSwap(t *testing.T) {
	s, err := Load(LoadOptions{Root: repoRoot(t), Version: testVersion})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	swapped := 0
	for _, f := range s.Files {
		for i := range f.Rules {
			for j := range f.Rules[i].Rows {
				row := &f.Rules[i].Rows[j]
				if row.Label != "Transfer From Water" || row.Counterpart == nil {
					continue
				}
				// Both ends are enterprise, so no fund-group sum moves.
				row.Fund, row.Counterpart.Fund = row.Counterpart.Fund, row.Fund
				swapped++
			}
		}
	}
	if swapped != 1 {
		t.Fatalf("swapped %d rows, want 1", swapped)
	}

	res, err := (&rowFundsMatchTheirAnchors{}).Run(t.Context(), s)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != StatusFail {
		t.Fatalf("inverting a transfer's two ends within one fund group reported %s: %s",
			res.Status, res.Summary)
	}
	// Both anchors are now wrong, and each finding says which end it is about.
	if len(res.Findings) != 2 {
		t.Errorf("got %d findings, want 2 (the payer anchor and the payee anchor)",
			len(res.Findings))
	}
	var b strings.Builder
	for _, f := range res.Findings {
		b.WriteString(f.Detail + "\n")
	}
	for _, want := range []string{"the fund which pays", "the fund which receives"} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("no finding says %q; a direction failure that does not name the "+
				"direction is not actionable:\n%s", want, b.String())
		}
	}
}
