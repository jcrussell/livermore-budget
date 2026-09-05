package check

import (
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/mapping"
)

func runStatedTotals(t *testing.T, s *Subject) Result {
	t.Helper()
	res, err := (&factOffsetIsNotAStatedTotal{}).Run(t.Context(), s)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	return res
}

// aStatedTotalLine returns one resolved stated-total span from the committed
// rules, chosen by a predicate, so a test can plant a fact on a real printed
// total instead of inventing an offset that no page backs.
func aStatedTotalLine(t *testing.T, s *Subject, want func(*mapping.Rule, *mapping.Part) bool) (
	docID string, page, offset int, ruleID string,
) {
	t.Helper()
	for _, f := range s.Files {
		r := s.Resolvers[f.Path]
		for i := range f.Rules {
			rule := &f.Rules[i]
			for j := range rule.Parts {
				p := &rule.Parts[j]
				if !want(rule, p) {
					continue
				}
				lo, _, err := r.TotalRowSpan(rule, p)
				if err != nil {
					continue
				}
				return f.DocID, p.Page, lo, rule.ID
			}
		}
	}
	t.Fatal("no committed part matches the predicate, so this test asserts nothing")
	return "", 0, 0, ""
}

// TestNoCommittedFactCitesAStatedTotal is the baseline, and it asserts the
// check had something to look at as well as that it held. A pass over zero
// stated-total lines would be the vacuous result wearing a pass's summary.
func TestNoCommittedFactCitesAStatedTotal(t *testing.T) {
	s, err := Load(LoadOptions{Root: repoRoot(t), Version: testVersion})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	res := runStatedTotals(t, s)
	if res.Status != StatusPass {
		t.Fatalf("fact-offset-is-not-a-stated-total = %s: %s (%v)",
			res.Status, res.Summary, res.Findings)
	}
	if res.Subjects != len(s.Facts) {
		t.Errorf("examined %d facts, want the whole store's %d", res.Subjects, len(s.Facts))
	}
	// The count of lines is deliberately NOT pinned -- it moves with every page
	// that gets mapped. That it is not zero is the durable claim.
	if strings.Contains(res.Summary, " 0 resolved stated-total") {
		t.Errorf("the check resolved no stated-total line, so it could not have failed: %q",
			res.Summary)
	}
}

// TestARepublishedTotalIsCaught is fisc-eaic: a figure the document prints as a
// block's total, republished as one of that block's rows.
//
// The fact is planted at a REAL resolved total offset rather than a made-up one,
// so the test fails the same way the corpus would.
func TestARepublishedTotalIsCaught(t *testing.T) {
	s, err := Load(LoadOptions{Root: repoRoot(t), Version: testVersion})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	doc, page, off, ruleID := aStatedTotalLine(t, s, func(r *mapping.Rule, _ *mapping.Part) bool {
		return r.TotalRow != ""
	})
	s.Facts = append(s.Facts, fact.Fact{
		DocID: doc, Page: page, Offset: off, Token: "1,234",
		RuleID: "some-other-rule", RowLabel: "A Row That Is Really A Total",
	})

	res := runStatedTotals(t, s)
	if res.Status != StatusFail {
		t.Fatalf("a fact citing rule %q's stated total reported %s: %s",
			ruleID, res.Status, res.Summary)
	}
	got := findingDetails(res)
	for _, want := range []string{ruleID, "some-other-rule", "stated-total line"} {
		if !strings.Contains(got, want) {
			t.Errorf("the finding does not name %q: %s", want, got)
		}
	}
}

// TestALabelLessPartsTotalIsCovered is the regression guard for a fail-open this
// check SHIPPED WITH and a probe caught before it landed.
//
// A label-less part has a stated total wherever its block ends, via stop_at,
// whether or not its rule declares a total_row: totalAnchor tests LabelsFrom
// BEFORE it tests TotalRow. The first draft of this check iterated only rules
// with a total_row, which left unguarded the three label-less parts that
// declare none -- and every other test here stayed green, because none of
// them planted a fact on one. (Five parts are label-less in all; the other
// two are TestALabelLessPartWithATotalRowNamesItsRealAnchor's subject.)
//
// So this test picks a part of exactly that shape and asserts the check sees it.
// Restore `if rule.TotalRow == "" { continue }` to the rule loop and it goes red
// while TestARepublishedTotalIsCaught still passes.
func TestALabelLessPartsTotalIsCovered(t *testing.T) {
	s, err := Load(LoadOptions{Root: repoRoot(t), Version: testVersion})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	doc, page, off, ruleID := aStatedTotalLine(t, s, func(r *mapping.Rule, p *mapping.Part) bool {
		return r.TotalRow == "" && p.LabelsFrom != 0
	})
	s.Facts = append(s.Facts, fact.Fact{
		DocID: doc, Page: page, Offset: off, Token: "1,234",
		RuleID: "some-other-rule", RowLabel: "A Row That Is Really A Total",
	})

	res := runStatedTotals(t, s)
	if res.Status != StatusFail {
		t.Fatalf("a fact citing label-less rule %q's stop_at total reported %s: %s",
			ruleID, res.Status, res.Summary)
	}
	if got := findingDetails(res); !strings.Contains(got, "stop_at") {
		t.Errorf("the finding does not say the total came from stop_at: %s", got)
	}
}

// TestALabelLessPartWithATotalRowNamesItsRealAnchor is the SECOND half of the
// LabelsFrom-before-TotalRow trap, and the first fix landed without it.
//
// Two committed parts are label-less AND declare a total_row: spine-revenues and
// spine-expenditures on p67. totalAnchor tests LabelsFrom first, so their anchor
// is stop_at "$" and their total_row is never searched for -- and p67 prints
// neither "TOTAL REVENUES:" nor "TOTAL EXPENDITURES:" anywhere (it is p66 that
// prints them). Choosing the finding's label on TotalRow alone therefore made
// this check report a real defect by pointing at a printed line that does not
// exist, which is worse than not reporting it: a reader goes to p67, greps, and
// concludes the check is broken.
//
// Restore `label := rule.TotalRow` ahead of the LabelsFrom test and this goes
// red while every other test in this file still passes.
func TestALabelLessPartWithATotalRowNamesItsRealAnchor(t *testing.T) {
	s, err := Load(LoadOptions{Root: repoRoot(t), Version: testVersion})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	doc, page, off, ruleID := aStatedTotalLine(t, s, func(r *mapping.Rule, p *mapping.Part) bool {
		return r.TotalRow != "" && p.LabelsFrom != 0
	})
	// The premise, asserted rather than assumed: the declared total_row is not
	// on this page, so naming it would name nothing.
	var declared string
	for _, f := range s.Files {
		for i := range f.Rules {
			if f.Rules[i].ID == ruleID {
				declared = f.Rules[i].TotalRow
			}
		}
	}
	text, err := s.Docs[doc].Page(page)
	if err != nil {
		t.Fatalf("page %d: %v", page, err)
	}
	if strings.Contains(text, declared) {
		t.Fatalf("p%d does print %q, so this test no longer covers the case it was written for",
			page, declared)
	}

	s.Facts = append(s.Facts, fact.Fact{
		DocID: doc, Page: page, Offset: off, Token: "1,234",
		RuleID: "some-other-rule", RowLabel: "A Row That Is Really A Total",
	})
	res := runStatedTotals(t, s)
	if res.Status != StatusFail {
		t.Fatalf("a fact citing %q's stop_at total reported %s: %s", ruleID, res.Status, res.Summary)
	}
	got := findingDetails(res)
	if strings.Contains(got, declared) {
		t.Errorf("the finding names %q as the printed stated-total line, and p%d does not print it: %s",
			declared, page, got)
	}
	if !strings.Contains(got, "stop_at") {
		t.Errorf("the finding does not name the stop_at anchor it actually resolved: %s", got)
	}
}

// TestARuleWhoseTotalResolvesNowhereIsCaught covers the check's other arm, which
// is the one way it could quietly examine nothing: a declared total_row that
// resolves on no part contributes no span, so nothing on that block's total line
// can ever be refused.
//
// A SPANNING BLOCK IS NOT AN EXCEPTION and that is why the arm is per RULE. Eleven
// committed parts fail to resolve their rule's total_row because the total is
// printed on the block's LAST page; each of those rules resolves it there, so a
// per-part arm would report eleven findings over a corpus with no defect in it.
func TestARuleWhoseTotalResolvesNowhereIsCaught(t *testing.T) {
	s, err := Load(LoadOptions{Root: repoRoot(t), Version: testVersion})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	var bent *mapping.Rule
	for _, f := range s.Files {
		for i := range f.Rules {
			if f.Rules[i].TotalRow != "" && len(f.Rules[i].Parts) > 0 {
				bent = &f.Rules[i]
				break
			}
		}
		if bent != nil {
			break
		}
	}
	if bent == nil {
		t.Fatal("no committed rule declares a total_row, so this test asserts nothing")
	}
	id := bent.ID
	bent.TotalRow = "No Page Prints This Row"

	res := runStatedTotals(t, s)
	if res.Status != StatusFail {
		t.Fatalf("a rule whose total_row resolves nowhere reported %s: %s", res.Status, res.Summary)
	}
	got := findingDetails(res)
	for _, want := range []string{id, "resolves it on none", "No Page Prints This Row"} {
		if !strings.Contains(got, want) {
			t.Errorf("the finding does not name %q: %s", want, got)
		}
	}
}
