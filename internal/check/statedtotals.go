package check

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/jcrussell/livermore-budget/internal/mapping"
)

// factOffsetIsNotAStatedTotal asserts that no fact cites a figure the document
// prints as a total -- on a rule's stated-total line or on a rollup's.
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
// same printed string earlier on the page is a different line: of the 149
// committed parts whose stated total resolves, 50 print their own total_row
// string more than once. [mapping.Resolver.TotalRowSpan] applies the block
// narrowing.
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
	return "no fact cites a figure printed on a rule's or a rollup's stated-total line"
}

// totalSpan is one resolved stated-total line: the byte range from just past its
// LABEL to the end of the printed line, and the rule or rollup that declared it.
//
// The run of spaces between the label and the first figure is inside the range,
// which is the fail-closed direction and costs nothing: a fact's offset points
// at its token, so no fact can sit in the whitespace.
type totalSpan struct {
	lo, hi int
	// ruleID is a rule id or a rollup id, and kind says which. The published
	// finding names one of them, so a rollup violation reported as `rule "x"`
	// sends a reader grepping mappings/ for a rule that is not there.
	ruleID string
	kind   string
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
		// EVERY RULE AND EVERY PART, not only the rules declaring a total_row,
		// because totalAnchor tests LabelsFrom BEFORE it tests TotalRow: a
		// label-less part's stated total is wherever its block ends, via
		// stop_at, whether or not its rule declares one.
		//
		// NO COMMITTED PART EXERCISES THIS TODAY, and the honest thing is to say
		// so rather than claim a guard. Measured: five parts are label-less; the
		// two that resolve a real stated total (spine-revenues and
		// spine-expenditures on p67) also DECLARE a total_row, so the narrow
		// filter would have reached them anyway, and the three that declare none
		// anchor on a block terminator that prints no totals run at all. The
		// widening is kept because it mirrors totalAnchor rather than
		// second-guessing it; a witness for it needs a synthetic rule, which is
		// fisc-loxx.
		for i := range f.Rules {
			rule := &f.Rules[i]
			failures := []string{}
			for j := range rule.Parts {
				p := &rule.Parts[j]
				lo, hi, err := r.TotalRowSpan(rule, p)
				if errors.Is(err, mapping.ErrNoStatedTotals) {
					continue
				}
				if err != nil {
					// The two ordinary shapes, per the arm below: a spanning
					// rule's total prints on the block's last page, and a
					// label-less part anchors on the block terminator rather
					// than on a totals row.
					if !rule.TotalSpansParts && p.LabelsFrom == 0 && rule.TotalRow != "" {
						failures = append(failures, fmt.Sprintf("p%d: %v", p.Page, err))
					}
					continue
				}
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
					totalSpan{lo: lo, hi: hi, ruleID: rule.ID, kind: "rule", label: label})
			}
			// PER PART, NOT PER RULE, AND `resolved == 0` WAS FAIL-OPEN.
			// Gating on "the rule resolved nothing" means a MULTI-PART rule that
			// loses one part's span still passes on the strength of the others.
			// Reproduced end to end: break spine-revenues' total_row and
			// fisc verify reports PASS over 161 lines instead of 162, with no
			// finding, while a fact planted on p66's real "TOTAL REVENUES:" line
			// goes unrefused. Every span this check loses is a line it can no
			// longer refuse a fact on, so every lost span has to be said.
			//
			// TWO KINDS OF FAILURE ARE ORDINARY AND ARE NOT FINDINGS, which is
			// why `failures` alone is the wrong gate and was tried:
			//
			//   - A total_spans_parts rule prints its total on the block's LAST
			//     page, so the earlier parts cannot resolve it. All eleven
			//     committed failures of that shape are exactly this.
			//   - A LABEL-LESS part anchors on the block TERMINATOR rather than
			//     a totals row, and three committed parts land on text that is
			//     no total at all -- spine-transfers-in on p67's fund-group
			//     header, spine-transfers-out and spine-fund-balance on the
			//     running footer. Resolving nothing there is the corpus's normal
			//     shape and pkg/cmd/build already reports those parts unchecked.
			//
			// So what must resolve is a LABELLED part of a rule that DECLARES a
			// total_row and does not spread it across parts.
			if len(failures) > 0 {
				findings = append(findings, finding(rule.ID,
					"declares total_row %q and fails to resolve it on %d of its %d "+
						"part(s), so this check contributes no span there and cannot "+
						"refuse a fact published on that total's line: %v",
					rule.TotalRow, len(failures), len(rule.Parts), failures))
			}
		}

		// ROLLUPS TOO, AND THEY ARE THE WIDEST TOTALS IN THE CORPUS. A rollup is
		// a printed total covering several RULES -- pp.167-170's ELEVEN
		// "<DEPARTMENT> TOTAL" rows over their divisions -- so its figure is a
		// total by exactly the argument a rule's total_row is, and republishing
		// one doubles a whole department rather than one block. This check
		// landed covering rule totals only, which left every rollup open, and
		// dept-city-council's printed $149,198 on p167 was the worked example.
		//
		// FOURTEEN IS THE COUNT OF ALL ROLLUPS IN THE FILE and eleven is the
		// department totals; the other three are gf-total-expenses on p170,
		// gf-total-revenues on p130 and other-funds-total-sources on p140. The
		// first draft of this comment said "pp.167-170's fourteen", which
		// contradicted resolve.go's own "six of pp.167-170's eleven" a few
		// hundred lines away.
		for j := range f.Rollups {
			ro := &f.Rollups[j]
			lo, hi, err := r.RollupTotalSpan(ro)
			if errors.Is(err, mapping.ErrNoStatedTotals) {
				continue
			}
			if err != nil {
				findings = append(findings, finding(ro.ID,
					"prints a rollup total this check cannot locate, so a fact published "+
						"on that line cannot be refused: %v", err))
				continue
			}
			lines++
			if spans[f.DocID] == nil {
				spans[f.DocID] = map[int][]totalSpan{}
			}
			spans[f.DocID][ro.Page] = append(spans[f.DocID][ro.Page],
				totalSpan{lo: lo, hi: hi, ruleID: ro.ID, kind: "rollup", label: ro.TotalRow})
		}
	}

	// NO SORT IS NEEDED AND NONE IS DONE. Every loop above and below ranges a
	// SLICE -- Files, Rules, Parts, then Facts in the store's own order -- and
	// the span maps are only ever looked up, never ranged. So a run names its
	// findings in the same order twice by construction, which is the property
	// factCitationsAreDeclared keeps an explicit `order` slice to get.
	// EITHER WAY OF EXAMINING NOTHING IS VACUOUS AND THEY ARE DIFFERENT, so the
	// reason is chosen rather than assumed. A single `nothing` string asserting
	// only the first would publish a false reason on a run with resolved lines
	// and an empty store. Found by /code-review.
	examined := len(s.Facts)
	nothing := "the fact store is empty, so nothing could cite a printed total"
	if lines == 0 {
		examined = 0
		nothing = "no rule and no rollup resolves a stated-total line, so no printed " +
			"total could be republished"
	}
	for _, fa := range s.Facts {
		for _, sp := range spans[fa.DocID][fa.Page] {
			if fa.Offset < sp.lo || fa.Offset >= sp.hi {
				continue
			}
			findings = append(findings, finding(
				fmt.Sprintf("%s p%d offset %d", fa.DocID, fa.Page, fa.Offset),
				"rule %q publishes token %q as row %q, but that figure is printed on "+
					"%s %q's stated-total line %q, so a total the document uses to "+
					"check a block is republished as one of the block's own rows",
				fa.RuleID, fa.Token, fa.RowLabel, sp.kind, sp.ruleID, sp.label))
		}
	}
	return conclusion{
		subjects: examined,
		unit:     "facts",
		held: fmt.Sprintf("%d facts, none citing a figure printed on any of the %d resolved stated-total %s (rule totals and rollups)",
			examined, lines, plural(lines, "line", "lines")),
		nothing:  nothing,
		findings: findings,
	}.result(), nil
}
