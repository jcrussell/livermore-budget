package check

import (
	"fmt"
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/mapping"
)

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

// TestARepublishedTotalIsCaught: a block's printed total republished as one of
// its rows, planted at a real resolved total offset.
func TestARepublishedTotalIsCaught(t *testing.T) {
	t.Parallel()
	s := mutable(t)
	doc, page, off, ruleID := aStatedTotalLine(t, s, func(r *mapping.Rule, _ *mapping.Part) bool {
		return r.TotalRow != ""
	})
	// Planted at the first figure, not at `lo`, which is whitespace on most
	// spans and so exercises only the span's start. Mutation: collapse the
	// check to `hi := lo+1` and this is red.
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

	res := runCheck(t, &factOffsetIsNotAStatedTotal{}, s)
	if res.Status != StatusFail {
		t.Fatalf("a fact citing rule %q's stated total reported %s: %s",
			ruleID, res.Status, res.Summary)
	}
	got := said(res)
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

// TestARepublishedRollupTotalIsCaught: a rollup's printed total (p167's
// dept-city-council $149,198) republished as a row doubles a department.
// Mutation: delete the rollup loop from the check and only this reddens.
func TestARepublishedRollupTotalIsCaught(t *testing.T) {
	t.Parallel()
	s := mutable(t)
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

	res := runCheck(t, &factOffsetIsNotAStatedTotal{}, s)
	if res.Status != StatusFail {
		t.Fatalf("a fact citing rollup %q's printed total reported %s: %s",
			id, res.Status, res.Summary)
	}
	got := said(res)
	if !strings.Contains(got, id) {
		t.Errorf("the finding does not name the rollup %q: %s", id, got)
	}
	// It must say "rollup", not "rule": rollup ids are not rule ids, so
	// `rule "dept-city-council"` names a rule mappings/ does not hold.
	if !strings.Contains(got, `rollup "`+id+`"`) {
		t.Errorf("the finding calls the rollup a rule: %s", got)
	}
}

// TestLosingASpanningRulesTotalIsCaught: losing a total_spans_parts rule's
// one stated-total line is a finding, bent for every committed spanning rule.
// Mutation: restore `if rule.TotalSpansParts { continue }` ahead of the bearer
// lookup and this names every spanning rule no rollup covers.
func TestLosingASpanningRulesTotalIsCaught(t *testing.T) {
	t.Parallel()
	s := isolated(t)
	if res := runCheck(t, &factOffsetIsNotAStatedTotal{}, s); res.Status != StatusPass {
		t.Fatalf("the unbent corpus reported %s: %s", res.Status, res.Summary)
	}
	bent := 0
	for _, f := range s.Files {
		for i := range f.Rules {
			rule := &f.Rules[i]
			if !rule.TotalSpansParts {
				continue
			}
			bent++
			was := rule.TotalRow
			rule.TotalRow = "No Page Prints This Row"
			res := runCheck(t, &factOffsetIsNotAStatedTotal{}, s)
			rule.TotalRow = was
			if res.Status != StatusFail {
				t.Errorf("spanning rule %q lost its only stated-total line and the check "+
					"reported %s: %s", rule.ID, res.Status, res.Summary)
				continue
			}
			got := said(res)
			for _, want := range []string{rule.ID, "resolves it on no page", "No Page Prints This Row"} {
				if !strings.Contains(got, want) {
					t.Errorf("the finding for %q does not name %q: %s", rule.ID, want, got)
				}
			}
		}
	}
	if bent == 0 {
		t.Fatal("no committed rule declares total_spans_parts, so this test asserts nothing")
	}
	t.Logf("measured: %d spanning rules, each a finding when its total_row resolves on no page", bent)
}

// TestALabelLessPartThatLosesItsTotalLineIsCaught: stop_at is moved onto the
// page's running footer, which the block still resolves to, so the finding
// must carry statedTotalLine's "no run of" message. Mutation: excuse
// label-less parts with `if mapping.AnchorOf(p) == mapping.AnchorStopAt
// { continue }` after the error and this goes red.
func TestALabelLessPartThatLosesItsTotalLineIsCaught(t *testing.T) {
	t.Parallel()
	s := isolated(t)
	var bentRule *mapping.Rule
	var bentPart *mapping.Part
	var footer string
	for _, f := range s.Files {
		r := s.Resolvers[f.Path]
		for i := range f.Rules {
			rule := &f.Rules[i]
			if rule.TotalRow == "" || rule.TotalSpansParts {
				continue
			}
			for j := range rule.Parts {
				p := &rule.Parts[j]
				if mapping.AnchorOf(p) != mapping.AnchorStopAt {
					continue
				}
				if _, _, err := r.TotalRowSpan(rule, p); err != nil {
					continue
				}
				// The page's last printed line: the running footer, which
				// prints no amounts.
				text := strings.TrimRight(pageOf(t, s, f.DocID, p.Page), "\n ")
				footer = strings.TrimSpace(text[strings.LastIndexByte(text, '\n')+1:])
				bentRule, bentPart = rule, p
				break
			}
			if bentRule != nil {
				break
			}
		}
		if bentRule != nil {
			break
		}
	}
	if bentRule == nil {
		t.Fatal("no committed label-less part of a total_row rule resolves a stated total, " +
			"so this test asserts nothing")
	}
	if res := runCheck(t, &factOffsetIsNotAStatedTotal{}, s); res.Status != StatusPass {
		t.Fatalf("the unbent corpus reported %s: %s", res.Status, res.Summary)
	}

	bentPart.StopAt, bentPart.StopAtOrdinal = footer, 0
	res := runCheck(t, &factOffsetIsNotAStatedTotal{}, s)
	if res.Status != StatusFail {
		t.Fatalf("rule %q's p%d part stops at %q, a line printing no totals, and the check "+
			"reported %s: %s", bentRule.ID, bentPart.Page, footer, res.Status, res.Summary)
	}
	got := said(res)
	for _, want := range []string{bentRule.ID, fmt.Sprintf("p%d", bentPart.Page), "no run of"} {
		if !strings.Contains(got, want) {
			t.Errorf("the finding does not name %q: %s", want, got)
		}
	}
}

// TestOneLostPartIsCaughtEvenWhenOthersResolve holds the arm to per-part, on a
// labelled total_row rule that is not total_spans_parts. Mutation: gate it on
// `resolved == 0` and a rule losing one part's span passes with no finding.
func TestOneLostPartIsCaughtEvenWhenOthersResolve(t *testing.T) {
	t.Parallel()
	s := isolated(t)
	// A rule that keeps some span after its total_row is broken, found by
	// simulation (spine-revenues: labelled p66, label-less p67).
	var bent *mapping.Rule
	var before, after int
	count := func(r *mapping.Resolver, rule *mapping.Rule) int {
		n := 0
		for j := range rule.Parts {
			if _, _, err := r.TotalRowSpan(rule, &rule.Parts[j]); err == nil {
				n++
			}
		}
		return n
	}
	for _, f := range s.Files {
		r := s.Resolvers[f.Path]
		for i := range f.Rules {
			rule := &f.Rules[i]
			if rule.TotalRow == "" || rule.TotalSpansParts || len(rule.Parts) < 2 {
				continue
			}
			was := rule.TotalRow
			b := count(r, rule)
			rule.TotalRow = "No Page Prints This Row"
			a := count(r, rule)
			rule.TotalRow = was
			if a > 0 && a < b {
				bent, before, after = rule, b, a
				break
			}
		}
		if bent != nil {
			break
		}
	}
	if bent == nil {
		t.Fatal("no committed rule loses SOME but not all of its stated-total spans when " +
			"its total_row is broken, so this test asserts nothing")
	}
	id := bent.ID

	if res := runCheck(t, &factOffsetIsNotAStatedTotal{}, s); res.Status != StatusPass {
		t.Fatalf("the unbent corpus reported %s: %s", res.Status, res.Summary)
	}

	bent.TotalRow = "No Page Prints This Row"
	res := runCheck(t, &factOffsetIsNotAStatedTotal{}, s)
	if res.Status != StatusFail {
		t.Fatalf("rule %q went from %d resolved spans to %d and the check reported %s: %s",
			id, before, after, res.Status, res.Summary)
	}
	got := said(res)
	for _, want := range []string{id, "fails to resolve it on"} {
		if !strings.Contains(got, want) {
			t.Errorf("the finding does not name %q: %s", want, got)
		}
	}
}

// TestNoTotalLurksAtAnUndeclaredLabelLessTerminator: the block terminators of
// label-less parts of rules with no total_row print no totals run, held
// against the pages. A failure means the rule needs a total_row.
// Mutation: delete the amountRun test from statedTotalLine and this goes red,
// naming every terminator line.
func TestNoTotalLurksAtAnUndeclaredLabelLessTerminator(t *testing.T) {
	t.Parallel()
	s := committed(t)
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
				if mapping.AnchorOf(p) != mapping.AnchorStopAt {
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
					t.Errorf("rule %q declares no total_row and its p%d part's terminator "+
						"resolves a stated-total span at %d on the line %q -- a printed "+
						"total the check is passing over; declare it on the rule",
						rule.ID, p.Page, lo, line)
				}
			}
		}
	}
	if seen == 0 {
		t.Fatal("no committed part is label-less with no total_row, so this test asserts nothing")
	}
}

// TestALabelLessPartWithATotalRowNamesItsRealAnchor: p67's label-less
// spine-revenues and spine-expenditures parts anchor on stop_at "$", and p67
// prints neither declared total_row, so the finding must name the stop_at.
// Mutation: restore `label := rule.TotalRow` ahead of the LabelsFrom test and
// only this goes red.
func TestALabelLessPartWithATotalRowNamesItsRealAnchor(t *testing.T) {
	t.Parallel()
	s := mutable(t)
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
	res := runCheck(t, &factOffsetIsNotAStatedTotal{}, s)
	if res.Status != StatusFail {
		t.Fatalf("a fact citing %q's stop_at total reported %s: %s", ruleID, res.Status, res.Summary)
	}
	got := said(res)
	if strings.Contains(got, declared) {
		t.Errorf("the finding names %q as the printed stated-total line, and p%d does not print it: %s",
			declared, page, got)
	}
	if !strings.Contains(got, "stop_at") {
		t.Errorf("the finding does not name the stop_at anchor it actually resolved: %s", got)
	}
}

// TestARuleWhoseTotalResolvesNowhereIsCaught: a declared total_row that
// resolves on no part contributes no span, and is a finding.
func TestARuleWhoseTotalResolvesNowhereIsCaught(t *testing.T) {
	t.Parallel()
	s := isolated(t)
	var bent *mapping.Rule
	for _, f := range s.Files {
		for i := range f.Rules {
			if f.Rules[i].TotalRow != "" && !f.Rules[i].TotalSpansParts && len(f.Rules[i].Parts) > 0 {
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

	res := runCheck(t, &factOffsetIsNotAStatedTotal{}, s)
	if res.Status != StatusFail {
		t.Fatalf("a rule whose total_row resolves nowhere reported %s: %s", res.Status, res.Summary)
	}
	got := said(res)
	for _, want := range []string{id, "fails to resolve it on", "No Page Prints This Row"} {
		if !strings.Contains(got, want) {
			t.Errorf("the finding does not name %q: %s", want, got)
		}
	}
}

// TestALineFoundByOrdinalIsNamedWithIt: p81's total is the 19th "$" after its
// block starts, and the first is a row's. A finding naming the line by stop_at
// "$" alone sends its reader to the first row.
//
// Mutation: drop the ordinal from the label and the finding names stop_at "$"
// alone.
func TestALineFoundByOrdinalIsNamedWithIt(t *testing.T) {
	t.Parallel()
	s := mutable(t)
	doc, page, off, ruleID := aStatedTotalLine(t, s, func(_ *mapping.Rule, p *mapping.Part) bool {
		return p.StopAtOrdinal > 0
	})
	s.Facts = append(s.Facts, fact.Fact{
		DocID: doc, Page: page, Offset: off, Token: "5,548,908.00",
		RuleID: "some-other-rule", RowLabel: "A Row That Is Really A Total",
	})
	res := runCheck(t, &factOffsetIsNotAStatedTotal{}, s)
	if res.Status != StatusFail {
		t.Fatalf("a fact on rule %q's ordinal-anchored total reported %s", ruleID, res.Status)
	}
	if got := said(res); !strings.Contains(got, "occurrence 19 after the block's start") {
		t.Errorf("the finding does not name the line by its ordinal: %s", got)
	}
}

// TestAnOrdinalWithNoSectionCountsFromThePage: with no section the block
// starts at the top of the page, which is where the resolver counts from and
// what its own refusal says, so the label says it too.
//
// Mutation: always say "after the block's start", and this is red.
func TestAnOrdinalWithNoSectionCountsFromThePage(t *testing.T) {
	t.Parallel()
	s := isolated(t)
	cleared := 0
	for _, f := range s.Files {
		for i := range f.Rules {
			for j := range f.Rules[i].Parts {
				if p := &f.Rules[i].Parts[j]; p.StopAtOrdinal > 0 {
					p.Section, p.SectionOrdinal = "", 0
					cleared++
				}
			}
		}
	}
	if cleared == 0 {
		t.Fatal("no committed part declares stop_at_ordinal, so this test asserts nothing")
	}
	doc, page, off, _ := aStatedTotalLine(t, s, func(_ *mapping.Rule, p *mapping.Part) bool {
		return p.StopAtOrdinal > 0
	})
	s.Facts = append(s.Facts, fact.Fact{
		DocID: doc, Page: page, Offset: off, Token: "5,548,908.00",
		RuleID: "some-other-rule", RowLabel: "A Row That Is Really A Total",
	})
	res := runCheck(t, &factOffsetIsNotAStatedTotal{}, s)
	if got := said(res); !strings.Contains(got, "occurrence 19 on the page") {
		t.Errorf("the finding does not count the ordinal from the page: %s", got)
	}
}
