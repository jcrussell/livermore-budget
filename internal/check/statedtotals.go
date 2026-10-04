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
// WHICH LINES IT MUST RESOLVE is the build's own selection, read off the same
// declarations the build reads, so that every line the build ties rows to is a
// line this check guards and nothing stands in for a verdict:
//
//   - A rule with no total_row declares no stated total, and no part of it is
//     asked -- the build's pkg/cmd/build.reasonNoTotalRow. A label-less part of
//     such a rule still has a stop_at, but its block terminator is a header, a
//     footer or a blank run, never a totals line; the test that scans those
//     terminators holds that against the pages.
//   - A total_spans_parts rule prints its total on the one page
//     [mapping.Resolver.TotalBearingPart] finds, and that page's line is the
//     rule's only span. The rule's other parts print no total.
//   - Every part of any other total_row rule resolves its own line: a labelled
//     part through total_row after its block, a label-less one through the
//     stop_at terminator, which is where [mapping.AnchorOf] says its total is.
//
// A line in that selection which does not resolve is a FINDING, per part,
// because every span this check loses is a printed total it can no longer
// refuse a fact on. The one failure that is not a finding is
// [mapping.ErrNoStatedTotals], a label-less part with no stop_at, which the
// build reports unchecked for the same reason.
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
	add := func(docID string, page int, sp totalSpan) {
		lines++
		if spans[docID] == nil {
			spans[docID] = map[int][]totalSpan{}
		}
		spans[docID][page] = append(spans[docID][page], sp)
	}
	for _, f := range s.Files {
		r := s.Resolvers[f.Path]
		for i := range f.Rules {
			rule := &f.Rules[i]
			if rule.TotalRow == "" {
				continue
			}
			if rule.TotalSpansParts {
				// ONE SPAN PER SPANNING RULE, on the bearing page. Asking every
				// part would count the head parts, whose pages print no total,
				// as losses -- and gating on "some part resolved" would let the
				// bearing page's loss hide behind nothing at all, since no other
				// part can resolve it.
				bearer, err := r.TotalBearingPart(rule)
				var lo, hi int
				if err == nil {
					lo, hi, err = r.TotalRowSpan(rule, bearer)
				}
				if err != nil {
					findings = append(findings, finding(rule.ID,
						"declares total_row %q across its parts and resolves it on no page, "+
							"so this check contributes no span for the block and cannot "+
							"refuse a fact published on that total's line: %v",
						rule.TotalRow, err))
					continue
				}
				add(f.DocID, bearer.Page,
					totalSpan{lo: lo, hi: hi, ruleID: rule.ID, kind: "rule", label: rule.TotalRow})
				continue
			}
			failures := []string{}
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
					from := "after the block's start"
					if p.Section == "" {
						from = "on the page"
					}
					label += fmt.Sprintf(" (occurrence %d %s)", p.StopAtOrdinal, from)
				}
				if mapping.AnchorOf(p) == mapping.AnchorTotalRow {
					label = rule.TotalRow
				}
				add(f.DocID, p.Page,
					totalSpan{lo: lo, hi: hi, ruleID: rule.ID, kind: "rule", label: label})
			}
			// PER PART, NOT PER RULE. Gating on "the rule resolved nothing"
			// lets a MULTI-PART rule lose one part's span and still pass on the
			// strength of the others, and every span this check loses is a
			// printed total line it can no longer refuse a fact on -- so a lost
			// span has to be said even when its siblings resolve.
			// TestOneLostPartIsCaughtEvenWhenOthersResolve holds this.
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
				findings = append(findings, finding(ro.ID,
					"prints a rollup total this check cannot locate, so a fact published "+
						"on that line cannot be refused: %v", err))
				continue
			}
			add(f.DocID, ro.Page,
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
		held: fmt.Sprintf("%d facts, none citing a figure printed on any of the %d resolved stated-total %s (rule totals and rollups)",
			examined, lines, cmdutil.Plural(lines, "line", "lines")),
		nothing:  nothing,
		findings: findings,
	}.result(), nil
}
