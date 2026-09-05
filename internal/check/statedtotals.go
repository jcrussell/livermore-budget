package check

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/jcrussell/livermore-budget/internal/mapping"
)

// factOffsetIsNotAStatedTotal asserts that no fact cites a figure the document
// prints on some rule's stated-total line.
//
// A TOTAL IS THE DOCUMENT CHECKING OUR WORK, AND A ROW IS OUR WORK. Conflating
// them publishes the sum of a block alongside the block, so the city's own money
// is counted twice and the arithmetic that was supposed to catch it is the very
// figure that got republished. CheckTotals corroborates a part against its
// printed total; nothing until now said that total may not ALSO be sold to a
// reader as one of the rows it totals.
//
// WHAT IT CATCHES, and why fact-citations-are-declared cannot. That check keys on
// a shared (doc_id, page, offset): two facts at one address are a row and its
// declared counterpart or they are a defect. A republished total collides with
// NOTHING, because a total's figure is consumed as a total and cited by no fact,
// so there is no second fact at the address and nothing to compare.
//
// MEASURED on ACFR p41 (fisc-eaic). acfr-p0041-gf-fund-balances declares three
// rows skip: true. Two of them -- "Transfers in" and "Transfers (out)" -- are
// rows the sibling rule PUBLISHES, so un-skipping one puts two facts at one
// address and fact-citations-are-declared refuses it. The third, "Total Other
// Financing Sources (Uses)", is that sibling's total_row, and un-skipping it was
// refused by nothing:
//
//   - un-skipped with kind transfer_out and no sign, `fisc verify` went red on
//     fact-transfer-orientation-is-declared, complaining the row prints "(25.19)"
//     while declaring sign positive. That is the gate firing on an unrelated
//     defect, not the doubling being caught -- see AGENTS.md, "green because the
//     gate fired".
//   - un-skipped and ALSO declared sign: netted, which is CORRECT for that
//     figure, `fisc verify` reported 44 passed, 0 failed while the store
//     published the block's own printed total as a third transfer_out fact:
//     ACFR transfers/out became 25.72 + 25.19.
//
// So the author careless enough to get the sign wrong was caught, and the author
// careful enough to get it right was not.
//
// THE POSITION IS RESOLVED, NOT SEARCHED FOR, and the difference is a third of
// the corpus. A rule's total_row is anchored AFTER that rule's block, so the
// same printed string earlier on the page is a different line: of the 152
// committed parts whose stated total resolves, 50 print their own total_row
// string more than once. [mapping.Resolver.TotalRowSpan] applies the block
// narrowing.
//
// (Both halves of that ratio were wrong when this check landed -- "53 of 150"
// here and "53 of 152" in the resolver, with the 53 counted over the 160 parts
// that DECLARE a total_row and the denominator over the 152 that resolve one.
// A numerator and a denominator from two populations, found by /code-review.)
//
// A RULE WHOSE DECLARED TOTAL RESOLVES ON NO PART IS A FINDING, not a skip. A
// resolution failure is the one way this check could quietly examine nothing,
// and a spanning block is not an exception to it: a rule whose total prints on
// its last page fails to resolve on the earlier ones and resolves there, which
// is why the arm is per RULE and not per part.
type factOffsetIsNotAStatedTotal struct{}

var _ Check = (*factOffsetIsNotAStatedTotal)(nil)

func (*factOffsetIsNotAStatedTotal) ID() string { return "fact-offset-is-not-a-stated-total" }
func (*factOffsetIsNotAStatedTotal) Tier() int  { return 1 }
func (*factOffsetIsNotAStatedTotal) Full() bool { return false }
func (*factOffsetIsNotAStatedTotal) Description() string {
	return "no fact cites a figure printed on a rule's stated-total line"
}

// totalSpan is one resolved stated-total line: the byte range its figures
// occupy in a page's extracted text, and the rule that declared it.
type totalSpan struct {
	lo, hi int
	ruleID string
	label  string
}

func (*factOffsetIsNotAStatedTotal) Run(_ context.Context, s *Subject) (Result, error) {
	var findings []Finding

	// Keyed by document and page, which is how a fact addresses itself.
	spans := map[string]map[int][]totalSpan{}
	lines := 0
	for _, f := range s.Files {
		r, ok := s.Resolvers[f.Path]
		if !ok {
			findings = append(findings, finding(f.Path,
				"no resolver was built for this rule file, so its totals cannot be located"))
			continue
		}
		// EVERY RULE AND EVERY PART, not only the rules declaring a total_row.
		// A LABEL-LESS part has a stated total wherever its block ends, via
		// stop_at, whether or not its rule declares a total_row -- see
		// totalAnchor, which tests LabelsFrom BEFORE it tests TotalRow. Five
		// committed parts are label-less; three of them declare no total_row,
		// and filtering on TotalRow here left this check unable to fail on
		// those three.
		for i := range f.Rules {
			rule := &f.Rules[i]
			resolved, failures := 0, []string{}
			for j := range rule.Parts {
				p := &rule.Parts[j]
				lo, hi, err := r.TotalRowSpan(rule, p)
				if errors.Is(err, mapping.ErrNoStatedTotals) {
					continue
				}
				if err != nil {
					failures = append(failures, fmt.Sprintf("p%d: %v", p.Page, err))
					continue
				}
				resolved++
				lines++
				if spans[f.DocID] == nil {
					spans[f.DocID] = map[int][]totalSpan{}
				}
				// THE LABEL MUST BRANCH THE WAY totalAnchor BRANCHED, and on
				// LabelsFrom FIRST. The OTHER two label-less parts --
				// spine-revenues and spine-expenditures on p67 -- DO declare a
				// total_row, and their anchor was still stop_at. Choosing the
				// label on TotalRow alone made a finding there name "TOTAL
				// REVENUES:" as the printed stated-total line on p67, a string
				// p67 does not contain at all (it is p66 that prints it): a
				// check reporting a defect by pointing at a line that is not
				// there. Found by /code-review, one predicate over from the
				// same trap this loop was widened for.
				label := "the end of the block, via stop_at " + strconv.Quote(p.StopAt)
				if p.LabelsFrom == 0 && rule.TotalRow != "" {
					label = rule.TotalRow
				}
				spans[f.DocID][p.Page] = append(spans[f.DocID][p.Page],
					totalSpan{lo: lo, hi: hi, ruleID: rule.ID, label: label})
			}
			// Restricted to rules that DECLARE one, because a rule with only
			// labelled parts and no total_row resolves nothing and is right to.
			if rule.TotalRow != "" && resolved == 0 {
				findings = append(findings, finding(rule.ID,
					"declares total_row %q and resolves it on none of its %d part(s), so "+
						"this check contributes no span for it and cannot refuse a fact "+
						"published on that total's line: %v",
					rule.TotalRow, len(rule.Parts), failures))
			}
		}
	}

	// NO SORT IS NEEDED AND NONE IS DONE. Every loop above and below ranges a
	// SLICE -- Files, Rules, Parts, then Facts in the store's own order -- and
	// the span maps are only ever looked up, never ranged. So a run names its
	// findings in the same order twice by construction, which is the property
	// factCitationsAreDeclared keeps an explicit `order` slice to get.
	// EITHER WAY OF EXAMINING NOTHING IS VACUOUS AND THEY ARE DIFFERENT, so the
	// reason is chosen rather than assumed. A single `nothing` string asserting
	// only the first would publish a false reason on a run with 152 resolved
	// lines and an empty store. Found by /code-review.
	examined := len(s.Facts)
	nothing := "the fact store is empty, so nothing could cite a printed total"
	if lines == 0 {
		examined = 0
		nothing = "no rule resolves a stated-total line, so no printed total could be republished"
	}
	for _, fa := range s.Facts {
		for _, sp := range spans[fa.DocID][fa.Page] {
			if fa.Offset < sp.lo || fa.Offset >= sp.hi {
				continue
			}
			findings = append(findings, finding(
				fmt.Sprintf("%s p%d offset %d", fa.DocID, fa.Page, fa.Offset),
				"rule %q publishes token %q as row %q, but that figure is printed on "+
					"rule %q's stated-total line %q, so a total the document uses to "+
					"check a block is republished as one of the block's own rows",
				fa.RuleID, fa.Token, fa.RowLabel, sp.ruleID, sp.label))
		}
	}
	return conclusion{
		subjects: examined,
		unit:     "facts",
		held: fmt.Sprintf("%d facts, none citing a figure printed on any of the %d resolved stated-total %s",
			examined, lines, plural(lines, "line", "lines")),
		nothing:  nothing,
		findings: findings,
	}.result(), nil
}
