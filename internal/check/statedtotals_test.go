package check

import (
	"strconv"
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
	s, err := Load(LoadOptions{Root: repoRoot(t), Version: testVersion})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
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

// TestLosingAnExemptSpanStillMovesThePublishedCount is what the check has
// instead of a finding for the fourteen parts whose non-resolution is exempt,
// and it is the half that no predicate can hide.
//
// The finding arm under-claims on purpose (fisc-xbvs): ten of the eleven
// total_spans_parts rules can lose their only span with the check still
// reporting pass, because `fisc build` refuses the same lost line first
// (TestTheTotalRowMustIdentifyOnePage's "on no page" case).
// Within verify, what makes the loss VISIBLE is the unresolved count, published
// unconditionally, so breaking an exempt rule moves a number `fisc verify`
// prints on every run.
//
// This asserts that count specifically and not merely that the summary changed
// -- the RESOLVED count moves too, so a laxer assertion passes with the
// unconditional `unresolved++` deleted. Delete it and this is the only test in
// the package that reddens.
func TestLosingAnExemptSpanStillMovesThePublishedCount(t *testing.T) {
	s, err := Load(LoadOptions{Root: repoRoot(t), Version: testVersion})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	before := runStatedTotals(t, s)
	if before.Status != StatusPass {
		t.Fatalf("the unbent corpus reported %s: %s", before.Status, before.Summary)
	}
	// A GENUINELY EXEMPT RULE, found by simulation rather than named. Not every
	// spanning rule is exempt in practice: div-general-services is covered by a
	// rollup, so breaking its total_row breaks RollupTotalSpan and produces a
	// finding by that route instead. The subject wanted here is one whose loss
	// the check really does forgive.
	var bent *mapping.Rule
	var after Result
	for _, f := range s.Files {
		for i := range f.Rules {
			rule := &f.Rules[i]
			if !rule.TotalSpansParts || rule.TotalRow == "" {
				continue
			}
			was := rule.TotalRow
			rule.TotalRow = "No Page Prints This Row"
			res := runStatedTotals(t, s)
			if res.Status == StatusPass {
				bent, after = rule, res
				break
			}
			rule.TotalRow = was
		}
		if bent != nil {
			break
		}
	}
	if bent == nil {
		t.Fatal("no committed spanning rule loses a span without producing a finding, so " +
			"the exemption this test is about no longer bites -- assert the finding instead")
	}
	// THE UNRESOLVED COUNT SPECIFICALLY, not just "the summary changed": the
	// RESOLVED count moves too, so the laxer assertion is green whether or not
	// the unconditional counter is there.
	wasUnresolved, nowUnresolved := unresolvedIn(t, before.Summary), unresolvedIn(t, after.Summary)
	if nowUnresolved != wasUnresolved+1 {
		t.Errorf("rule %q lost its stated-total span and the published unresolved count "+
			"went %d -> %d, want %d: the loss is not visible as a loss.\n  before: %q\n  after:  %q",
			bent.ID, wasUnresolved, nowUnresolved, wasUnresolved+1, before.Summary, after.Summary)
	}
	// It is exempt from the FINDING on purpose: the build refuses a spanning
	// rule whose total no page prints. If this ever starts failing, the
	// exemption has been tightened and this test should assert the finding.
	if after.Status != StatusPass {
		t.Logf("NOTE: breaking a spanning rule now reports %s -- the exemption has been "+
			"tightened", after.Status)
	}
}

// TestOneLostPartIsCaughtEvenWhenOthersResolve holds the arm to per-PART.
//
// Gate it on `resolved == 0` instead and a rule that loses ONE part's
// stated-total span passes on the strength of its other parts: fisc verify
// reports PASS over 161 lines rather than 162, with no finding, while a fact
// planted on p66's real "TOTAL REVENUES:" line goes unrefused. Every span this
// check loses is a line it can no longer refuse a fact on.
//
// The rule bent here is chosen for the shape the arm must NOT excuse: labelled
// parts, a declared total_row, and not total_spans_parts. The two shapes it MUST
// excuse have their own tests above -- the eleven spanning parts whose total
// prints on the block's last page, and the label-less parts anchoring on a block
// terminator.
// unresolvedIn reads the "N declared total(s) resolve to no line" figure out of
// the check's published summary, so a test can assert the number a reader sees
// rather than a number recomputed beside it.
func unresolvedIn(t *testing.T, summary string) int {
	t.Helper()
	const tail = " declared total(s) resolve to no line"
	i := strings.Index(summary, tail)
	if i < 0 {
		t.Fatalf("the summary does not publish an unresolved count: %q", summary)
	}
	j := strings.LastIndexByte(summary[:i], ' ') + 1
	n, err := strconv.Atoi(summary[j:i])
	if err != nil {
		t.Fatalf("unreadable unresolved count in %q: %v", summary, err)
	}
	return n
}

func TestOneLostPartIsCaughtEvenWhenOthersResolve(t *testing.T) {
	s, err := Load(LoadOptions{Root: repoRoot(t), Version: testVersion})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
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

// TestALabelLessPartWithNoTotalsRunContributesNoSpan asserts that a label-less
// part anchoring on a block TERMINATOR contributes no span.
//
// Three committed parts are of that shape and none of the three lines is a
// total: spine-transfers-in lands on p67's column-header line
// `Capital Funds  Debt Service Funds ...`, and spine-transfers-out and
// spine-fund-balance land on the running footer "BUDGET FY 2025-27 Page 63".
// A check that took the anchor for a total would report a defect against a
// header and a page number.
//
// Delete the amountRun test from statedTotalLine and this goes red, naming all
// three lines verbatim -- they come back as stated totals and the check's own
// summary over-counts them.
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
	s, err := Load(LoadOptions{Root: repoRoot(t), Version: testVersion})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
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
