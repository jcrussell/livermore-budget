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
	// NEITHER OF THE TWO ASSERTIONS THIS REPLACED COULD FAIL. `examined` is
	// len(s.Facts) unless lines == 0, and lines == 0 zeroes it, which makes the
	// result VACUOUS and the status assertion above has already fired. So
	// "Subjects == len(Facts)" and "the summary does not say 0 lines" were both
	// implied by reaching this line. Found by /code-review.
	//
	// What is NOT implied is how many lines the check found, and that is the
	// number a silent regression would move: the whole check degrades quietly if
	// TotalRowSpan starts refusing lines it used to resolve. The count itself is
	// deliberately not pinned -- it moves with every page that gets mapped -- so
	// what is asserted is that both KINDS of total are represented, which is the
	// durable claim and the one this file's own history says goes wrong.
	if res.Subjects != len(s.Facts) {
		t.Errorf("examined %d facts, want the whole store's %d", res.Subjects, len(s.Facts))
	}
	rules, rollups := 0, 0
	for _, f := range s.Files {
		r := s.Resolvers[f.Path]
		for i := range f.Rules {
			for j := range f.Rules[i].Parts {
				if _, _, err := r.TotalRowSpan(&f.Rules[i], &f.Rules[i].Parts[j]); err == nil {
					rules++
				}
			}
		}
		for j := range f.Rollups {
			if _, _, err := r.RollupTotalSpan(&f.Rollups[j]); err == nil {
				rollups++
			}
		}
	}
	if rules == 0 {
		t.Error("no rule part resolves a stated total, so the check could not fail on one")
	}
	if rollups == 0 {
		t.Error("no rollup resolves its printed total, so the check could not fail on one -- " +
			"this check shipped covering rule totals only, and pp.167-170's fourteen " +
			"department totals were the hole")
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
	// PLANTED AT THE FIRST FIGURE, NOT AT THE SPAN'S FIRST BYTE. Every fail-path
	// test in this file used to plant at exactly `lo`, which is the byte after
	// the LABEL -- whitespace on 147 of the 149 rule spans, where
	// fact-offset-points-at-token guarantees no fact can sit. So the span's
	// extent was never exercised: narrowing the check to `Offset >= lo+1` left
	// the whole package green. A real republished total sits where its token
	// does, which is what this now plants. Found by /code-review.
	at := off
	for at < len(pageOf(t, s, doc, page)) && pageOf(t, s, doc, page)[at] == ' ' {
		at++
	}
	if at == off {
		t.Fatalf("the span at %d starts on a non-space, so this test no longer plants "+
			"past the label as it claims", off)
	}
	s.Facts = append(s.Facts, fact.Fact{
		DocID: doc, Page: page, Offset: at, Token: "1,234",
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

// pageOf is the extracted text of one page, for tests that need to look at the
// line they are planting a fact on.
func pageOf(t *testing.T, s *Subject, docID string, page int) string {
	t.Helper()
	text, err := s.Docs[docID].Page(page)
	if err != nil {
		t.Fatalf("%s p%d: %v", docID, page, err)
	}
	return text
}

// TestARepublishedRollupTotalIsCaught is the same defect one level up, and the
// check shipped without it.
//
// A rollup is a printed total covering several RULES -- pp.167-170's fourteen
// "<DEPARTMENT> TOTAL" rows over their divisions. dept-city-council's p167 line
// prints $149,198 and covers div-city-council's own Total, so republishing it as
// a row doubles a department. The first three review passes over this lane all
// read a check that located rule totals and no rollup at all.
func TestARepublishedRollupTotalIsCaught(t *testing.T) {
	s, err := Load(LoadOptions{Root: repoRoot(t), Version: testVersion})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	var docID, id string
	var page, off int
	for _, f := range s.Files {
		r := s.Resolvers[f.Path]
		for j := range f.Rollups {
			lo, _, err := r.RollupTotalSpan(&f.Rollups[j])
			if err != nil {
				continue
			}
			docID, page, off, id = f.DocID, f.Rollups[j].Page, lo, f.Rollups[j].ID
			break
		}
		if id != "" {
			break
		}
	}
	if id == "" {
		t.Fatal("no committed rollup resolves its printed total, so this test asserts nothing")
	}
	text := pageOf(t, s, docID, page)
	at := off
	for at < len(text) && text[at] == ' ' {
		at++
	}
	s.Facts = append(s.Facts, fact.Fact{
		DocID: docID, Page: page, Offset: at, Token: "1,234",
		RuleID: "some-other-rule", RowLabel: "A Row That Is Really A Rollup Total",
	})

	res := runStatedTotals(t, s)
	if res.Status != StatusFail {
		t.Fatalf("a fact citing rollup %q's printed total reported %s: %s",
			id, res.Status, res.Summary)
	}
	if got := findingDetails(res); !strings.Contains(got, id) {
		t.Errorf("the finding does not name the rollup %q: %s", id, got)
	}
}

// TestALabelLessPartWithNoTotalsRunContributesNoSpan is what replaced this
// file's original regression guard, and the replacement is the finding.
//
// THE GUARD IT REPLACES WAS GREEN BECAUSE THE GATE FIRED. It planted a fact on
// "a label-less part with no total_row", found one, and passed -- and the line
// it planted on was p67 offset 2204, `Capital Funds  Debt Service Funds ...`,
// a COLUMN HEADER. It proved the check flags a header line, which is a defect
// rather than the property it was named for. Its two siblings landed on the
// running footer "BUDGET FY 2025-27 Page 63". All three came from TotalRowSpan
// returning an anchored position without asking whether that line prints a
// total at all.
//
// So the property now asserted is the one that was actually wrong: those three
// parts must contribute NO span, because the terminator they anchor on prints
// no totals run. Delete the amountRun test from statedTotalLine and this goes
// red -- the three lines come back as stated totals and the check's own summary
// over-counts them.
func TestALabelLessPartWithNoTotalsRunContributesNoSpan(t *testing.T) {
	s, err := Load(LoadOptions{Root: repoRoot(t), Version: testVersion})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	seen := 0
	for _, f := range s.Files {
		r := s.Resolvers[f.Path]
		for i := range f.Rules {
			rule := &f.Rules[i]
			if rule.TotalRow != "" {
				continue
			}
			for j := range rule.Parts {
				p := &rule.Parts[j]
				if p.LabelsFrom == 0 {
					continue
				}
				seen++
				lo, _, err := r.TotalRowSpan(rule, p)
				if err == nil {
					text, perr := s.Docs[f.DocID].Page(p.Page)
					if perr != nil {
						t.Fatalf("page %d: %v", p.Page, perr)
					}
					at := strings.LastIndexByte(text[:lo], '\n') + 1
					line := strings.TrimSpace(text[at:])
					if k := strings.IndexByte(line, '\n'); k >= 0 {
						line = line[:k]
					}
					t.Errorf("rule %q p%d resolves a stated-total span at %d, and the line "+
						"it names is %q -- if that is a printed totals row this test is "+
						"stale; if it is a header or a footer the check will report a "+
						"defect against a line the page never totalled",
						rule.ID, p.Page, lo, line)
				}
			}
		}
	}
	if seen == 0 {
		t.Fatal("no committed part is label-less with no total_row, so this test asserts nothing")
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
