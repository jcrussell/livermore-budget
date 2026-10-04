package check

import (
	"fmt"
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
	t.Parallel()
	s := committed(t)
	res := runStatedTotals(t, s)
	if res.Status != StatusPass {
		t.Fatalf("fact-offset-is-not-a-stated-total = %s: %s (%v)",
			res.Status, res.Summary, res.Findings)
	}
	// THE SUBJECT-COUNT ASSERTION BELOW CANNOT FAIL TODAY AND IS KEPT ON
	// PURPOSE. `examined` is len(s.Facts) unless lines == 0, and lines == 0
	// zeroes it, which makes the result VACUOUS and the status assertion above
	// has already fired. It is the check's published subject count, it costs
	// nothing, and it is what would catch `examined` being narrowed to some
	// subset of the store.
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
			"this check shipped covering rule totals only, and the file's fourteen " +
			"rollups -- eleven of them pp.167-170's department totals -- were the hole")
	}
}

// TestARepublishedTotalIsCaught is fisc-eaic: a figure the document prints as a
// block's total, republished as one of that block's rows.
//
// The fact is planted at a REAL resolved total offset rather than a made-up one,
// so the test fails the same way the corpus would.
func TestARepublishedTotalIsCaught(t *testing.T) {
	t.Parallel()
	s := mutable(t)
	doc, page, off, ruleID := aStatedTotalLine(t, s, func(r *mapping.Rule, _ *mapping.Part) bool {
		return r.TotalRow != ""
	})
	// PLANTED AT THE FIRST FIGURE, NOT AT THE SPAN'S FIRST BYTE. `lo` is the
	// byte after the LABEL, which is whitespace on 147 of the 149 rule spans,
	// and fact-offset-points-at-token guarantees no fact sits in whitespace --
	// so planting there exercises the span's start and never its EXTENT.
	// Collapsing the check to `hi := lo+1` must redden this test; it does not
	// if the fact is planted at `lo`. A real republished total sits where its
	// token does, which is what this plants.
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

// TestARepublishedRollupTotalIsCaught is the same defect one level up.
//
// A rollup is a printed total covering several RULES -- pp.167-170's eleven
// "<DEPARTMENT> TOTAL" rows over their divisions, plus three more elsewhere in
// the file. dept-city-council's p167 line prints $149,198 and covers
// div-city-council's own Total, so republishing it as a row doubles a whole
// department. Delete the rollup loop from the check and this is the only test
// that reddens.
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

	res := runStatedTotals(t, s)
	if res.Status != StatusFail {
		t.Fatalf("a fact citing rollup %q's printed total reported %s: %s",
			id, res.Status, res.Summary)
	}
	got := findingDetails(res)
	if !strings.Contains(got, id) {
		t.Errorf("the finding does not name the rollup %q: %s", id, got)
	}
	// IT MUST SAY "rollup", NOT "rule". Rollup ids are not rule ids, so a
	// violation published as `rule "dept-city-council"` sends its reader to grep
	// mappings/ for a rule that is not there.
	if !strings.Contains(got, `rollup "`+id+`"`) {
		t.Errorf("the finding calls the rollup a rule: %s", got)
	}
}

// TestLosingASpanningRulesTotalIsCaught holds the spanning arm: a
// total_spans_parts rule has ONE stated-total line, on the page
// mapping.Resolver.TotalBearingPart finds, and losing it is a finding.
//
// EVERY committed spanning rule is bent in turn rather than one of them,
// because the shape that goes wrong here is selective: a predicate that
// excuses a spanning rule's non-resolution forgives all of them except the
// ones a rollup happens to cover, which produce a finding by RollupTotalSpan's
// route instead. Restore `if rule.TotalSpansParts { continue }` ahead of the
// bearer lookup and this names every spanning rule no rollup covers.
func TestLosingASpanningRulesTotalIsCaught(t *testing.T) {
	t.Parallel()
	s := isolated(t)
	if res := runStatedTotals(t, s); res.Status != StatusPass {
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
			res := runStatedTotals(t, s)
			rule.TotalRow = was
			if res.Status != StatusFail {
				t.Errorf("spanning rule %q lost its only stated-total line and the check "+
					"reported %s: %s", rule.ID, res.Status, res.Summary)
				continue
			}
			got := findingDetails(res)
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

// TestALabelLessPartThatLosesItsTotalLineIsCaught holds the stop_at arm for a
// part that DECLARES a total: a label-less part of a total_row rule reads its
// stated total at the block terminator, so a terminator that is no longer a
// totals line is a lost span, exactly as a moved total_row is.
//
// The bend is to the LINE and not to the block. stop_at is moved onto the
// page's running footer, which the block still resolves to -- the finding must
// carry statedTotalLine's "no run of" message, which is the arm that says the
// anchor was found and the line under it prints no totals. Setting stop_at to
// text the page does not print would redden the test too, but through the
// block failing, which is what every check over the store refuses and not what
// this one is for.
//
// Excuse label-less parts from the per-part arm -- `if mapping.AnchorOf(p) ==
// mapping.AnchorStopAt { continue }` after the error -- and this goes red.
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
				// The page's last printed line. On every page of the Budget Book
				// that is the running footer, and a footer prints no amounts.
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
	if res := runStatedTotals(t, s); res.Status != StatusPass {
		t.Fatalf("the unbent corpus reported %s: %s", res.Status, res.Summary)
	}

	bentPart.StopAt, bentPart.StopAtOrdinal = footer, 0
	res := runStatedTotals(t, s)
	if res.Status != StatusFail {
		t.Fatalf("rule %q's p%d part stops at %q, a line printing no totals, and the check "+
			"reported %s: %s", bentRule.ID, bentPart.Page, footer, res.Status, res.Summary)
	}
	got := findingDetails(res)
	for _, want := range []string{bentRule.ID, fmt.Sprintf("p%d", bentPart.Page), "no run of"} {
		if !strings.Contains(got, want) {
			t.Errorf("the finding does not name %q: %s", want, got)
		}
	}
}

// TestOneLostPartIsCaughtEvenWhenOthersResolve holds the arm to per-PART.
//
// Gate it on `resolved == 0` instead and a rule that loses ONE part's
// stated-total span passes on the strength of its other parts: fisc verify
// reports PASS over one line fewer, with no finding, while a fact planted on
// p66's real "TOTAL REVENUES:" line goes unrefused. Every span this check
// loses is a line it can no longer refuse a fact on.
//
// The rule bent here is chosen for the shape the arm must NOT excuse: labelled
// parts, a declared total_row, and not total_spans_parts. The spanning shape,
// whose head parts print no total, has its own test above.
func TestOneLostPartIsCaughtEvenWhenOthersResolve(t *testing.T) {
	t.Parallel()
	s := isolated(t)
	// The shape wanted is a rule that keeps SOME span after its total_row is
	// broken, because that is exactly what `resolved == 0` forgave. It is found
	// by simulation rather than by naming a rule, so the test follows the corpus
	// instead of pinning it. spine-revenues is today's answer: its p66 part is
	// labelled and resolves through total_row, and its p67 part is label-less
	// and resolves through stop_at, so breaking total_row costs one of the two.
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

	if res := runStatedTotals(t, s); res.Status != StatusPass {
		t.Fatalf("the unbent corpus reported %s: %s", res.Status, res.Summary)
	}

	bent.TotalRow = "No Page Prints This Row"
	res := runStatedTotals(t, s)
	if res.Status != StatusFail {
		t.Fatalf("rule %q went from %d resolved spans to %d and the check reported %s: %s",
			id, before, after, res.Status, res.Summary)
	}
	got := findingDetails(res)
	for _, want := range []string{id, "fails to resolve it on"} {
		if !strings.Contains(got, want) {
			t.Errorf("the finding does not name %q: %s", want, got)
		}
	}
}

// TestNoTotalLurksAtAnUndeclaredLabelLessTerminator is the page claim behind
// the check's first exit: a rule with no total_row is not asked for a span, and
// that passes over no printed total only if the block terminators of such
// rules' label-less parts -- the one place a label-less part reads a total from
// -- print no totals run.
//
// Held against the pages rather than assumed: spine-transfers-in's p67 part
// ends on the fund-group header `Capital Funds  Debt Service Funds ...`,
// spine-transfers-out's and spine-fund-balance's on the running footer "BUDGET
// FY 2025-27 Page 63", pp.225-235's CIP continuations and pp.187-209's
// fund-balance continuations on the same footer or on a blank run. A part that
// stops failing here has a stated total its rule does not declare, and the fix
// is a total_row on the rule, which is what makes the build tie it and this
// check guard it.
//
// Delete the amountRun test from statedTotalLine and this goes red, naming
// every terminator line verbatim.
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

// TestARuleWhoseTotalResolvesNowhereIsCaught covers the per-part arm's plainest
// case, which is the one way the check could quietly examine nothing: a declared
// total_row that resolves on no part contributes no span, so nothing on that
// block's total line can ever be refused. The spanning shape has its own test
// above and its own finding.
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

	res := runStatedTotals(t, s)
	if res.Status != StatusFail {
		t.Fatalf("a rule whose total_row resolves nowhere reported %s: %s", res.Status, res.Summary)
	}
	got := findingDetails(res)
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
// Mutation: drop the ordinal from the label, and the finding names stop_at
// "$" alone.
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
	res := runStatedTotals(t, s)
	if res.Status != StatusFail {
		t.Fatalf("a fact on rule %q's ordinal-anchored total reported %s", ruleID, res.Status)
	}
	if got := findingDetails(res); !strings.Contains(got, "occurrence 19 after the block's start") {
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
	res := runStatedTotals(t, s)
	if got := findingDetails(res); !strings.Contains(got, "occurrence 19 on the page") {
		t.Errorf("the finding does not count the ordinal from the page: %s", got)
	}
}
