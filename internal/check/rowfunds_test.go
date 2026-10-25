package check

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/mapping"
)

// TestRowFundsCatchesTheTwinSwapNothingElseSees: five of p76's payer labels
// match an operating fund and its capital CIP twin, a swap no printed total
// and no group sum sees. Runs one case that changes fund group and one that does not.
func TestRowFundsCatchesTheTwinSwapNothingElseSees(t *testing.T) {
	t.Parallel()
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
			s := isolated(t)
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

			res := runCheck(t, &rowFundsMatchTheirAnchors{}, s)
			wantFail(t, res, tt.anchor, "fund", "no other check sees it")
		})
	}
}

// TestThePassLineKeepsTheTwoArmsApart is heldLine's wiring over p76's verb
// phrases and pp.85-125's bare labels. Mutation: pass 0 for either arm to
// heldLines and the bare labels fold into the verb phrases' direction claim.
func TestThePassLineKeepsTheTwoArmsApart(t *testing.T) {
	t.Parallel()
	base := committed(t)
	s := *base
	s.Files = nil
	for _, f := range base.Files {
		cut := *f
		cut.Rules = nil
		for i := range f.Rules {
			if id := f.Rules[i].ID; strings.HasPrefix(id, "p76-") || strings.HasPrefix(id, "funding-") {
				cut.Rules = append(cut.Rules, f.Rules[i])
			}
		}
		s.Files = append(s.Files, &cut)
	}
	res := runCheck(t, &rowFundsMatchTheirAnchors{}, &s)
	if res.Status != StatusPass {
		t.Fatalf("%s: %s", res.Status, res.Summary)
	}
	for _, want := range []string{"40 printed with a verb phrase", "78 a bare fund name under row_labels_name_funds"} {
		if !strings.Contains(res.Summary, want) {
			t.Errorf("the PASS line does not keep the two arms apart: no %q in\n%s", want, res.Summary)
		}
	}
}

// TestTheBareLabelArmIsWhatTheDeclarationTurnsOn: with row_labels_name_funds
// cleared the check falls back to 522 fewer subjects (pp.85-125's 78,
// pp.186-209's 444) and the unread-rows clause returns.
func TestTheBareLabelArmIsWhatTheDeclarationTurnsOn(t *testing.T) {
	t.Parallel()
	base := committed(t)
	s := withoutRowLabelFunds(base)
	res := runCheck(t, &rowFundsMatchTheirAnchors{}, s)
	if res.Subjects != 312 {
		t.Errorf("with the declaration cleared the check resolves %d row anchors, "+
			"want 312 -- the p76 anchors and the fund numbers of p222 and pp.224-235 "+
			"alone", res.Subjects)
	}
	if !strings.Contains(res.Summary, "a further 522 declared fund(s) sit at an end this "+
		"check does not read") {
		t.Errorf("with the declaration cleared the summary does not report the 522 as "+
			"rows it makes no claim about:\n%s", res.Summary)
	}
	// One name per rule, not per row. Mutation: a per-row append in place of
	// the seenRule dedup leaves the rest of the suite green.
	for _, want := range []string{"funding-city-council", "funding-public-works"} {
		if !strings.Contains(res.Summary, want) {
			t.Errorf("the summary does not attribute the unread declarations to their "+
				"rules: no %q in\n%s", want, res.Summary)
		}
	}
	if n := strings.Count(res.Summary, "funding-"); n != 11 {
		t.Errorf("the summary names funding-* %d times, want 11 -- one per rule. A "+
			"per-row list is 78 and is what put this check's PASS line at 11 KB", n)
	}
	if n := strings.Count(res.Summary, "fund-balances-fy"); n != 16 {
		t.Errorf("the summary names fund-balances-* %d times, want 16 -- one per rule", n)
	}
}

// withoutRowLabelFunds copies s with RowLabelsNameFunds cleared on every rule,
// deep enough that the original is untouched: Subject holds []*mapping.File and
// two tests share one Load.
func withoutRowLabelFunds(base *Subject) *Subject {
	s := *base
	s.Files = nil
	for _, f := range base.Files {
		cut := *f
		cut.Rules = append([]mapping.Rule(nil), f.Rules...)
		for i := range cut.Rules {
			cut.Rules[i].RowLabelsNameFunds = false
		}
		s.Files = append(s.Files, &cut)
	}
	return &s
}

// TestAVacuousRowFundsSummaryCannotDenyTheRowsItSaw: a corpus of only
// pp.85-125 must not report "no row declares a fund" while 78 do.
// Mutation: revert the guard to `unanchored > 0` and this is red.
func TestAVacuousRowFundsSummaryCannotDenyTheRowsItSaw(t *testing.T) {
	t.Parallel()
	base := committed(t)
	// Only the funding-source rules, with row_labels_name_funds cleared: 78
	// declared funds, none unanchored, none resolvable by this check.
	cleared := withoutRowLabelFunds(base)
	s := *cleared
	s.Files = nil
	for _, f := range cleared.Files {
		cut := *f
		cut.Rules = nil
		for i := range f.Rules {
			if strings.HasPrefix(f.Rules[i].ID, "funding-") {
				cut.Rules = append(cut.Rules, f.Rules[i])
			}
		}
		s.Files = append(s.Files, &cut)
	}

	res := runCheck(t, &rowFundsMatchTheirAnchors{}, &s)
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
	// The clause needs its antecedent: Contains("78 declared fund(s)") is also
	// satisfied by "; a further 78 ...", so antecedent and adjacency are pinned.
	if strings.Contains(res.Summary, "a further") {
		t.Errorf("the summary says \"a further\" with nothing before it:\n%s", res.Summary)
	}
	if !strings.Contains(res.Summary, "resolve; 78 declared fund(s)") {
		t.Errorf("the one clause does not follow the reason directly:\n%s", res.Summary)
	}
}

// TestABareFundLabelIsNotReportedAsAnUnprintedAnchor: pp.85-125's 78 rows
// print their label. Mutation: anchorHasVerbPhrase returning true collapses
// the two lists into one and this is red on the count.
func TestABareFundLabelIsNotReportedAsAnUnprintedAnchor(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	res := runCheck(t, &rowFundsMatchTheirAnchors{}, testSubject(t))
	if res.Status != StatusVacuous {
		t.Errorf("over a corpus whose funds are all per column the check reported %s: %s",
			res.Status, res.Summary)
	}
	if !strings.Contains(res.Summary, "rule-funds-match-their-headings") {
		t.Errorf("the vacuous summary does not name the check that covers the "+
			"per-column case instead: %s", res.Summary)
	}
}

// TestRowFundsCatchesASameGroupEndSwap is the direction half: swapping the ends
// of "Transfer From Water  to Water Replacement" (640/642, both enterprise)
// moves no group sum, so cuts-tie-along-the-lattice cannot see it.
func TestRowFundsCatchesASameGroupEndSwap(t *testing.T) {
	t.Parallel()
	s := isolated(t)
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

	res := runCheck(t, &rowFundsMatchTheirAnchors{}, s)
	if res.Status != StatusFail {
		t.Fatalf("inverting a transfer's two ends within one fund group reported %s: %s",
			res.Status, res.Summary)
	}
	// Both anchors are now wrong, and each finding says which end it is about.
	if len(res.Findings) != 2 {
		t.Errorf("got %d findings, want 2 (the payer anchor and the payee anchor)",
			len(res.Findings))
	}
	b := said(res)
	for _, want := range []string{"the fund which pays", "the fund which receives"} {
		if !strings.Contains(b, want) {
			t.Errorf("no finding says %q; a direction failure that does not name the "+
				"direction is not actionable:\n%s", want, b)
		}
	}
}

// TestRowFundsCatchesABareLabelTwin: a bare-label row's fund that disagrees
// with its label fails fisc verify. It asserts on this Result, not the suite,
// because cuts-tie-along-the-lattice also reddens on Water 640 -> 642; the
// last subtest holds that the CIP twin 641 is refused by the cip-funds cut.
// Mutation: delete the bare-label arm from Run (the second gated on
// ru.RowLabelsNameFunds) and the check goes green.
func TestRowFundsCatchesABareLabelTwin(t *testing.T) {
	t.Parallel()
	s := isolated(t)
	// Water 640 -> Water Replacement 642, with the fund group left alone.
	swapped := 0
	for _, f := range s.Files {
		for i := range f.Rules {
			for j := range f.Rules[i].Rows {
				row := &f.Rules[i].Rows[j]
				if f.Rules[i].ID != "funding-public-works" || row.Label != "Water" {
					continue
				}
				if row.Fund != 640 {
					t.Fatalf("row %q declares fund %d, want 640", row.Label, row.Fund)
				}
				row.Fund = 642
				swapped++
			}
		}
	}
	if swapped != 1 {
		t.Fatalf("swapped %d rows, want 1; the mutation missed its target", swapped)
	}

	res := runCheck(t, &rowFundsMatchTheirAnchors{}, s)
	wantFail(t, res, `funding-public-works "Water"`, "fund 640", "fund 642")

	// The other gates: fact-funds-resolve passes this same-type swap and
	// cuts-tie-along-the-lattice reports it per fund against pp.186-209. Both
	// read s.Facts, so the facts are mutated as `fisc build` would emit them.
	original := slices.Clone(s.Facts)
	swappedFacts := 0
	for i := range s.Facts {
		f := &s.Facts[i]
		if f.RuleID == "funding-public-works" && f.Fund != nil && *f.Fund == 640 {
			f.Fund = fact.FundNumber(642)
			swappedFacts++
		}
	}
	if swappedFacts == 0 {
		t.Fatal("no fact carries funding-public-works fund 640; the green half would " +
			"pass over an unmutated store, which is what it exists to refuse")
	}
	got := runCheck(t, &factFundsResolve{}, s)
	if got.Status != StatusPass {
		t.Errorf("fact-funds-resolve reported %s over the same-type swap: %s", got.Status, got.Summary)
	}
	got = runCheck(t, &cutsTieAlongTheLattice{}, s)
	perFund := 0
	for _, f := range got.Findings {
		if f.Subject == "funding-sources -> fund-balance-expenses" &&
			(strings.Contains(f.Detail, "fund=640") || strings.Contains(f.Detail, "fund=642")) {
			perFund++
		}
	}
	if got.Status != StatusFail || perFund != len(got.Findings) || perFund != 8 {
		t.Errorf("cuts-tie-along-the-lattice reported %s with %d findings, want the 8 cells of "+
			"funds 640 and 642 against pp.186-209, four columns each:\n  %v", got.Status, len(got.Findings), got.Findings)
	}

	t.Run("the CIP twin is refused by the outside cut", func(t *testing.T) {
		s.Facts = slices.Clone(original)
		for i := range s.Facts {
			f := &s.Facts[i]
			if f.RuleID == "funding-public-works" && f.Fund != nil && *f.Fund == 640 {
				f.Fund = fact.FundNumber(641)
			}
		}
		got := runCheck(t, &cutsTieAlongTheLattice{}, s)
		b := said(got)
		if got.Status != StatusFail || !strings.Contains(b, "fund 641") ||
			!strings.Contains(b, "funding-sources") {
			t.Fatalf("Water 640 -> CIP Water 641 reported %s, want a finding naming fund 641 "+
				"and the cut that now carries it:\n%s", got.Status, b)
		}
	})
}

// TestTheFundNumberArmHoldsBothEndsOfAP222Row mutates one p222 row's
// receiving fund and, separately, its transferring fund, each to another fund
// of the same type, which moves no sum any check reads.
func TestTheFundNumberArmHoldsBothEndsOfAP222Row(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		label  string
		mutate func(*mapping.Row)
		want   string
	}{
		{"the receiving fund", "811", func(r *mapping.Row) { r.Fund = 813 }, "the fund which receives is 813"},
		{"the transferring fund", "811", func(r *mapping.Row) { r.Counterpart.Fund = 510 }, "the fund which pays is 510"},
		// A transfer read as revenue: the page still numbers its payer.
		{"the counterpart dropped", "811", func(r *mapping.Row) { r.Counterpart = nil }, "declares no counterpart"},
		// A grant row declaring no fund at all is read, not skipped.
		{"a grant row's fund dropped", "816", func(r *mapping.Row) { r.Fund = 0 }, "the fund which receives is 0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := isolated(t)
			hit := 0
			for _, f := range s.Files {
				for i := range f.Rules {
					for j := range f.Rules[i].Rows {
						row := &f.Rules[i].Rows[j]
						if f.Rules[i].ID == "p222-cip-funding-sources" && row.Label == tc.label {
							tc.mutate(row)
							hit++
						}
					}
				}
			}
			if hit != 1 {
				t.Fatalf("mutated %d rows, want 1", hit)
			}
			res := runCheck(t, &rowFundsMatchTheirAnchors{}, s)
			if res.Status != StatusFail || len(res.Findings) != 1 ||
				!strings.Contains(res.Findings[0].Detail, tc.want) {
				t.Fatalf("reported %s with %v, want one finding saying %q", res.Status, res.Findings, tc.want)
			}
		})
	}
}

// TestABareLabelThatResolvesToNoFundIsAFinding: a label data/funds.yaml does
// not resolve is a finding, not a `continue`. Only mutating the loaded rules
// reaches it; renaming a page label breaks the anchor first.
func TestABareLabelThatResolvesToNoFundIsAFinding(t *testing.T) {
	t.Parallel()
	s := isolated(t)
	renamed := 0
	for _, f := range s.Files {
		for i := range f.Rules {
			for j := range f.Rules[i].Rows {
				row := &f.Rules[i].Rows[j]
				if f.Rules[i].ID != "funding-city-attorney" || row.Label != "General Fund" {
					continue
				}
				row.Label = "No Such Fund"
				renamed++
			}
		}
	}
	if renamed != 1 {
		t.Fatalf("renamed %d rows, want 1; the mutation missed its target", renamed)
	}

	res := runCheck(t, &rowFundsMatchTheirAnchors{}, s)
	// The finding is one line, so the assertion is the line.
	wantFail(t, res, "No Such Fund", "checked against nothing")
}

// TestASkippedRowIsInvisibleToThisCheck: a skipped row is neither a subject
// nor counted in the omission counters. No skip: true row in the corpus
// carries a fund, so the row is built here.
func TestASkippedRowIsInvisibleToThisCheck(t *testing.T) {
	t.Parallel()
	base := isolated(t)
	before := runCheck(t, &rowFundsMatchTheirAnchors{}, base)

	s := isolated(t)
	// A skipped row carrying a fund, inside a rule that declares
	// row_labels_name_funds, with a label no fund in the registry answers to.
	// If the guard is missing this is either a finding (the label resolves to
	// nothing) or a subject; either way the numbers below move.
	added := 0
	for _, f := range s.Files {
		for i := range f.Rules {
			if f.Rules[i].ID != "funding-city-attorney" {
				continue
			}
			f.Rules[i].Rows = append(f.Rules[i].Rows, mapping.Row{
				Label: "Not A Fund At All", Skip: true, Fund: 100, FundGroup: "general",
			})
			added++
		}
	}
	if added != 1 {
		t.Fatalf("added %d rows, want 1", added)
	}

	got := runCheck(t, &rowFundsMatchTheirAnchors{}, s)
	wantPass(t, got, before.Subjects)
	if got.Summary != before.Summary {
		t.Errorf("a skipped row changed the summary:\nbefore: %s\nafter:  %s",
			before.Summary, got.Summary)
	}
}

// TestTheHeldLineNeverReportsAnEmptyArm covers all three shapes of the PASS
// line, two over corpora this repo does not have: no clause may report
// "0 printed with a verb phrase" for an absent arm.
func TestTheHeldLineNeverReportsAnEmptyArm(t *testing.T) {
	t.Parallel()
	// wantNot are regexps: the substring "0 printed" matches inside
	// "40 printed", and \b refuses it.
	zeroPhrased := regexp.MustCompile(`\b0 printed with a verb phrase`)
	zeroBare := regexp.MustCompile(`\b0 a bare fund name`)

	for _, tc := range []struct {
		name           string
		subjects, bare int
		wantNot        []*regexp.Regexp
		wantHas        []string
	}{{
		name:     "verb phrases only, as every corpus before pp.85-125",
		subjects: 40, bare: 0,
		wantNot: []*regexp.Regexp{regexp.MustCompile(`bare fund name`),
			regexp.MustCompile(`row_labels_name_funds`)},
		wantHas: []string{"40 row anchors", "at that end of the movement"},
	}, {
		name:     "bare labels only, as a corpus of nothing but funding-* rules",
		subjects: 78, bare: 78,
		wantNot: []*regexp.Regexp{regexp.MustCompile(`printed with a verb phrase`), zeroBare},
		wantHas: []string{"78 row anchors", "names no direction"},
	}, {
		name:     "both, as the committed corpus",
		subjects: 118, bare: 78,
		wantNot: []*regexp.Regexp{zeroPhrased, zeroBare},
		wantHas: []string{"118 row anchors", "40 printed with a verb phrase", "78 a bare"},
	}} {
		t.Run(tc.name, func(t *testing.T) {
			got := heldLine(tc.subjects, tc.bare)
			for _, re := range tc.wantNot {
				if re.MatchString(got) {
					t.Errorf("held line matches %s, which is a clause about no row:\n%s", re, got)
				}
			}
			for _, w := range tc.wantHas {
				if !strings.Contains(got, w) {
					t.Errorf("held line does not contain %q:\n%s", w, got)
				}
			}
		})
	}
}

// TestAVerbPhrasedLabelUnderTheDeclarationIsRefused: under
// row_labels_name_funds, "Transfer From General Fund  to Horizons" names a
// movement, not a fund, and is refused. Deferring it to the prefix arm would
// resolve it as the far end, want == far == 0, and compare nothing.
func TestAVerbPhrasedLabelUnderTheDeclarationIsRefused(t *testing.T) {
	t.Parallel()
	base := isolated(t)
	before := runCheck(t, &rowFundsMatchTheirAnchors{}, base)

	s := isolated(t)
	flagged := 0
	for _, f := range s.Files {
		for i := range f.Rules {
			if f.Rules[i].ID == "p76-transfers-in-special-revenue" {
				f.Rules[i].RowLabelsNameFunds = true
				flagged++
			}
		}
	}
	if flagged != 1 {
		t.Fatalf("flagged %d rules, want 1", flagged)
	}

	got := runCheck(t, &rowFundsMatchTheirAnchors{}, s)
	wantFail(t, got, "opens with a verb phrase", "names a movement")
	// The subject count must not move: reading a row twice or not at all
	// both change it.
	if got.Subjects != before.Subjects {
		t.Errorf("the declaration moved the subject count from %d to %d; these rows "+
			"are read by the anchor arm and by it alone", before.Subjects, got.Subjects)
	}
}

// TestTheRowFundsSummaryStatesNoZeroClause: a corpus whose only fund-declaring
// rule prints fund numbers reports that arm alone.
func TestTheRowFundsSummaryStatesNoZeroClause(t *testing.T) {
	t.Parallel()
	if got := heldLines(62, 0, 62); strings.Contains(got, "0 row anchors") ||
		!strings.HasPrefix(got, "62 printed fund numbers") {
		t.Errorf("heldLines(62, 0, 62) = %q", got)
	}
	if got := heldLines(180, 78, 62); !strings.Contains(got, "118 row anchors") ||
		!strings.Contains(got, "; and 62 printed fund numbers") {
		t.Errorf("heldLines(180, 78, 62) = %q", got)
	}
}

// TestAVerbPhrasedAnchorThatResolvesToNoFundIsAFinding holds the prefix arm to
// what the bare-label arm already does: "Transfer From X" names the fund that
// pays, so an X the registry does not resolve leaves the declared payer checked
// against nothing, and the row is not one whose page prints no anchor.
func TestAVerbPhrasedAnchorThatResolvesToNoFundIsAFinding(t *testing.T) {
	t.Parallel()
	s := isolated(t)
	renamed := 0
	for _, f := range s.Files {
		for i := range f.Rules {
			for j := range f.Rules[i].Rows {
				row := &f.Rules[i].Rows[j]
				if row.Counterpart == nil || row.Counterpart.Fund != 200 ||
					row.Label != "Transfer From Low Income Hsng" {
					continue
				}
				row.Label = "Transfer From No Such Fund"
				renamed++
			}
		}
	}
	if renamed != 1 {
		t.Fatalf("renamed %d rows, want 1; the mutation missed its target", renamed)
	}

	res := runCheck(t, &rowFundsMatchTheirAnchors{}, s)
	wantFail(t, res, "Transfer From No Such Fund", "the fund that pays", "fund 200", "checked against nothing")
	if strings.Contains(res.Summary, "Transfer From No Such Fund") {
		t.Errorf("the row is also listed as having no printed anchor: %s", res.Summary)
	}
}
