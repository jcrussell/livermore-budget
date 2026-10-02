package check

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/jcrussell/livermore-budget/internal/mapping"
	"github.com/jcrussell/livermore-budget/pkg/cmdutil"
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
// Financing Sources (Uses)", is that sibling's total_row. Un-skipped with sign:
// netted, which is CORRECT for that figure, the build publishes the block's own
// printed total as a third transfer_out fact, and this is the only check that
// fails.
//
// THE POSITION IS RESOLVED, NOT SEARCHED FOR. A rule's total_row is anchored
// AFTER that rule's block, so the same printed string earlier on the page is a
// different line, and many parts print theirs more than once.
// [mapping.Resolver.TotalRowSpan] applies the block narrowing.
//
// WHAT IT DOES ABOUT LOSING A SPAN, precisely, because the boundary is narrow
// and easy to read as wider than it is. A resolution failure is the one way this
// check can quietly examine less than it did yesterday, and it cannot be refused
// outright: some parts legitimately resolve no stated-total line at all. So
// there are two mechanisms and only one of them is a finding:
//
//   - THE COUNT IS PUBLISHED UNCONDITIONALLY, exempt or not, which is what makes
//     span loss VISIBLE rather than silent and is the half no exemption
//     predicate can hide.
//   - THE FINDING IS NARROW, and deliberately under-claims. It fires for a
//     LABELLED part of a rule that declares a total_row and does not spread it
//     across parts. Break a spanning rule's total_row or a label-less part's
//     stop_at and this check still reports pass, with the counts moved -- but
//     `fisc build` resolves the same lines and exits non-zero on either break
//     (measured on div-special-operations and on spine-revenues' p67 part),
//     so CI's rebuild refuses it before any fact is published.
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
	lines, unresolved := 0, 0
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
		// NO COMMITTED PART EXERCISES THIS, and the honest thing is to say so
		// rather than claim a guard. The label-less parts that resolve a real
		// stated total -- p67's spine-revenues and spine-expenditures, p81's
		// debt-service-principal and debt-service-interest -- also DECLARE a
		// total_row, so the narrow filter would reach them anyway; the rest
		// anchor on a block terminator that prints no totals run: p67's other
		// three on its fund-group header and running footer, pp.225-235's CIP
		// continuation parts on the running footer. The widening is kept
		// because it follows mapping.AnchorOf rather than second-guessing it; a
		// witness for it needs a synthetic rule, which is fisc-loxx.
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
					// COUNTED WHATEVER THE ARM BELOW DOES WITH IT. The published
					// summary carries this number, so a span that stops
					// resolving moves a figure `fisc verify` prints on every run
					// whether or not any predicate calls it a finding. It is the
					// part of this that an over-broad exemption cannot hide.
					unresolved++
					// The two ordinary shapes, per the arm below: a spanning
					// rule's total prints on the block's last page, and a
					// label-less part anchors on the block terminator rather
					// than on a totals row.
					if !rule.TotalSpansParts && mapping.AnchorOf(p) == mapping.AnchorTotalRow && rule.TotalRow != "" {
						failures = append(failures, fmt.Sprintf("p%d: %v", p.Page, err))
					}
					continue
				}
				lines++
				if spans[f.DocID] == nil {
					spans[f.DocID] = map[int][]totalSpan{}
				}
				// THE LABEL NAMES THE ANCHOR mapping.AnchorOf SAYS the part reads,
				// the branch totalAnchor takes. spine-revenues and spine-expenditures on
				// p67 are label-less AND declare a total_row, and their anchor
				// is still stop_at -- so naming rule.TotalRow there would name
				// "TOTAL REVENUES:" as p67's printed stated-total line, a string
				// p67 does not contain at all (p66 prints it). A check that
				// reports a defect by pointing at a line that is not there sends
				// its reader to grep for nothing.
				label := "the end of the block, via stop_at " + strconv.Quote(p.StopAt)
				if p.StopAtOrdinal > 0 {
					label += fmt.Sprintf(" (occurrence %d after the block's start)", p.StopAtOrdinal)
				}
				if mapping.AnchorOf(p) == mapping.AnchorTotalRow {
					label = rule.TotalRow
				}
				spans[f.DocID][p.Page] = append(spans[f.DocID][p.Page],
					totalSpan{lo: lo, hi: hi, ruleID: rule.ID, kind: "rule", label: label})
			}
			// PER PART, NOT PER RULE. Gating on "the rule resolved nothing"
			// lets a MULTI-PART rule lose one part's span and still pass on the
			// strength of the others, and every span this check loses is a
			// printed total line it can no longer refuse a fact on -- so a lost
			// span has to be said even when its siblings resolve.
			// TestOneLostPartIsCaughtEvenWhenOthersResolve holds this.
			//
			// TWO KINDS OF FAILURE ARE ORDINARY AND ARE NOT FINDINGS, which is
			// why `len(failures) > 0` alone is the wrong gate:
			//
			//   - A total_spans_parts rule prints its total on the block's LAST
			//     page, so the earlier parts cannot resolve it. All eleven
			//     committed failures of that shape are exactly this.
			//   - A LABEL-LESS part anchors on the block TERMINATOR rather than
			//     a totals row, and committed parts land on text that is no
			//     total at all -- spine-transfers-in on p67's fund-group header,
			//     spine-transfers-out, spine-fund-balance and pp.225-235's CIP
			//     continuation parts on the running footer. Resolving nothing
			//     there is the corpus's normal shape: pkg/cmd/build reports p67's
			//     three unchecked, and ties the CIP parts' figures through their
			//     subtotal chain.
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
		// one doubles a whole department rather than one block --
		// dept-city-council's printed $149,198 on p167 is the worked example.
		//
		for j := range f.Rollups {
			ro := &f.Rollups[j]
			lo, hi, err := r.RollupTotalSpan(ro)
			if errors.Is(err, mapping.ErrNoStatedTotals) {
				continue
			}
			if err != nil {
				unresolved++
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
	// and an empty store.
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
		held: fmt.Sprintf("%d facts, none citing a figure printed on any of the %d resolved stated-total %s (rule totals and rollups); %d declared total(s) resolve to no line",
			examined, lines, cmdutil.Plural(lines, "line", "lines"), unresolved),
		nothing:  nothing,
		findings: findings,
	}.result(), nil
}
