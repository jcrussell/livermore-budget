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

	// THE SEVENTY-EIGHT ROWS THE CHECK DOES NOT READ, and they are a different
	// statement about the page from the three above. pp.85-125's Department
	// Funding Sources rows print a bare fund name -- which names a fund and no
	// direction, so rowAnchorPrefixes matches nothing. Counting them under "no
	// printed anchor on their own line" would have been false about the page,
	// and this check's summary is printed verbatim on every fisc verify run.
	// The hand-typed fund on all 78 is guarded by nothing (fisc-90fp).
	if !strings.Contains(res.Summary, "a further 78 declared fund(s) sit on rows whose "+
		"printed label names a fund but no direction") {
		t.Errorf("the summary does not count the rows whose label carries no verb phrase "+
			"separately from the rows the page leaves blank:\n%s", res.Summary)
	}
	// ATTRIBUTED TO ELEVEN RULES AND NOT LISTED AS 78 ROWS. Naming each row put
	// this check's PASS line at 11 KB, against roughly 450 bytes before, which
	// is detailtie.go's "a count is the honest middle" argument arriving one
	// check late. The rule id is what a reader opens.
	if !strings.Contains(res.Summary, "funding-city-council") ||
		!strings.Contains(res.Summary, "funding-public-works") {
		t.Errorf("the summary does not attribute the unread declarations to their "+
			"rules:\n%s", res.Summary)
	}
	if strings.Contains(res.Summary, `funding-public-works "Water"`) {
		t.Errorf("the summary names the unread rows one by one again; 78 of them is a "+
			"count, not a list:\n%s", res.Summary)
	}
	if n := len(res.Summary); n > 2000 {
		t.Errorf("the summary is %d bytes; it is printed on every fisc verify run", n)
	}
}

// TestAVacuousRowFundsSummaryCannotDenyTheRowsItSaw is the arm the first pass of
// /code-review over this lane found missing.
//
// The vacuous reason is printed by --strict as the justification for a check
// having nothing to look at, so a false one is worse than no reason at all --
// which is what the comment beside `nothing` already said. It tested only the
// unanchored counter, so a corpus of nothing but pp.85-125 reported "no row
// declares a fund" while 78 did. Reverting the guard to `unanchored > 0` makes
// this red.
func TestAVacuousRowFundsSummaryCannotDenyTheRowsItSaw(t *testing.T) {
	base, err := Load(LoadOptions{Root: repoRoot(t), Version: testVersion})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// Only the funding-source rules, whose row labels carry no verb phrase, so
	// every declared fund is unphrased and none is unanchored.
	s := *base
	s.Files = nil
	for _, f := range base.Files {
		cut := *f
		cut.Rules = nil
		for i := range f.Rules {
			if strings.HasPrefix(f.Rules[i].ID, "funding-") {
				cut.Rules = append(cut.Rules, f.Rules[i])
			}
		}
		s.Files = append(s.Files, &cut)
	}

	res, err := (&rowFundsMatchTheirAnchors{}).Run(t.Context(), &s)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != StatusVacuous {
		t.Fatalf("over a corpus of bare-label rows the check reported %s: %s",
			res.Status, res.Summary)
	}
	if strings.Contains(res.Summary, "no row declares a fund") {
		t.Errorf("the vacuous reason denies the 78 rows that do declare one:\n%s",
			res.Summary)
	}
	if !strings.Contains(res.Summary, "78 declared fund(s)") {
		t.Errorf("the vacuous reason does not say what it saw:\n%s", res.Summary)
	}
	// AND IT HAS TO READ. The assertion above passed on "; a further 78
	// declared fund(s)..." -- a clause with no antecedent, because the note was
	// seeded in the unanchored arm and appended to in the unphrased one. Found
	// by the third review pass; the defect was introduced by the second pass's
	// own fix, which is the failure mode AGENTS.md tabulates under "Review is a
	// loop, not a pass".
	if strings.Contains(res.Summary, "a further") {
		t.Errorf("the summary says \"a further\" with nothing before it:\n%s", res.Summary)
	}
	if !strings.Contains(res.Summary, "resolve; 78 declared fund(s)") {
		t.Errorf("the one clause does not follow the reason directly:\n%s", res.Summary)
	}
}

// TestABareFundLabelIsNotReportedAsAnUnprintedAnchor is the property the split
// above exists for, stated so it can fail on its own.
//
// Before pp.85-125 landed, every declared fund with no resolved anchor was p76's
// continuation-row case and the one sentence was true. Adding 78 rows whose
// label IS printed made it false 78 times over, in a string fisc verify prints
// verbatim. Reverting anchorHasVerbPhrase to a constant true collapses the two
// lists back into one and makes this test red on the count.
func TestABareFundLabelIsNotReportedAsAnUnprintedAnchor(t *testing.T) {
	if anchorHasVerbPhrase("General Fund") {
		t.Error(`"General Fund" carries no verb phrase, so it cannot be evidence of direction`)
	}
	// "to " is a prefix of nothing in the funding-source label set, and the
	// nearest thing to a trap -- a fund whose name begins with a lowercase
	// preposition -- does not exist in data/funds.yaml.
	if !anchorHasVerbPhrase("to Downtown LMD") || !anchorHasVerbPhrase("Transfer From CASP Fee") {
		t.Error("a p76 anchor stopped being recognised as carrying a verb phrase")
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
