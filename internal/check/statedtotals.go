package check

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/jcrussell/livermore-budget/internal/english"
	"github.com/jcrussell/livermore-budget/internal/mapping"
)

// factOffsetIsNotAStatedTotal asserts that no fact cites a figure the document
// prints as a total -- on a rule's stated-total line or on a rollup's -- so a
// block's sum is never published beside the block.
//
// fact-citations-are-declared cannot catch this: a republished total collides
// with no other fact. Un-skipping ACFR p41's "Total Other Financing Sources
// (Uses)" is the case only this check fails.
//
// The position is resolved, not searched for: [mapping.Resolver.TotalRowSpan]
// narrows to after the rule's block. The lines resolved are the build's own
// selection:
//
//   - a rule with no total_row is not asked (pkg/cmd/build.reasonNoTotalRow).
//   - a total_spans_parts rule's only span is on the page
//     [mapping.Resolver.TotalBearingPart] finds.
//   - every part of any other total_row rule resolves its own line, a
//     label-less one through its stop_at terminator ([mapping.AnchorOf]).
//
// A line in that selection that does not resolve is a finding, per part,
// except [mapping.ErrNoStatedTotals], which the build reports unchecked.
type factOffsetIsNotAStatedTotal struct{}

func (*factOffsetIsNotAStatedTotal) meta() meta {
	return meta{id: "fact-offset-is-not-a-stated-total", tier: 1,
		description: "no fact cites a figure printed on a rule's or a rollup's stated-total line"}
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
			examined, lines, english.Plural(lines, "line", "lines")),
		nothing:  nothing,
		findings: findings,
	}.result(), nil
}
