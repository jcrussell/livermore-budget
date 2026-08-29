package check

import (
	"context"
	"fmt"
	"strings"

	"github.com/jcrussell/livermore-budget/internal/mapping"
	"github.com/jcrussell/livermore-budget/internal/registry"
)

// rowAnchorPrefix is a verb phrase a printed row anchor carries in front of the
// fund it names, and WHICH END OF THE MOVEMENT that fund is at.
//
// The list is written out rather than inferred, for the reason
// rule-funds-match-their-headings writes out "Total ": a check that guessed
// where the fund name starts would resolve a different string than the author
// read, and the whole value here is that both are reading the same printed
// words. Budget Book p76 phrases its rows "Transfer From <payer>  to <payee>".
//
// THE side FIELD IS WHAT MAKES THIS A DIRECTION CHECK and not just a membership
// one. An earlier version asserted only that each anchor's fund was one of the
// two the row declared, on the argument that swapping a row's ends moves money
// between the in and out sides of a fund group and
// transfers-detail-ties-to-spine reddens on that. THAT ARGUMENT IS ONLY TRUE
// WHEN THE TWO ENDS ARE IN DIFFERENT GROUPS. Measured: swapping the ends of
// "Transfer From Water  to Water Replacement" -- 640 and 642, both enterprise
// -- rebuilds cleanly and every check stays green with the payer and payee
// genuinely inverted. "Transfer From Wastewater  to Stormwater" is the same
// shape. The page says which is which, so the check reads it.
//
// An anchor that resolves to nothing after stripping is NOT a defect. Most rows
// in this corpus carry a category label, not a fund name.
type rowAnchorPrefix struct {
	prefix string
	// far is true when the fund this anchor names is the COUNTERPART's rather
	// than the row's own.
	far bool
}

var rowAnchorPrefixes = []rowAnchorPrefix{
	{prefix: "Transfer From ", far: true},
	{prefix: "to "},
}

// rowFundsMatchTheirAnchors is the guard on a fund the rule types out and the
// page names in a ROW rather than in a heading.
//
// WHY IT IS A SEPARATE CHECK. rule-funds-match-their-headings covers a schedule
// whose fund is a property of the whole rule, read off a printed
// "Total <fund>" line -- declaredFund reads Parts[].Columns[].Fund and nothing
// else. Budget Book p76 inverts that: a column is a YEAR, the section is the
// receiving fund GROUP, and the fund is per ROW. Such a rule declares fund 0,
// so it is not merely unchecked by that check, it never enters its sweep at all.
//
// AND IT IS EXACTLY WHERE THE HAZARD LIVES. p76's payer side is hand-typed 44
// times over, and THE DOCUMENT CANNOT CHECK IT: the counterpart fan-out is
// downstream of StatedTotals, CheckTotals, stated_total_deltas, the rollups and
// the gap guard, so redeclaring every payer as fund 999 leaves the page's own
// totals byte for byte unchanged (mapping's TestTheDocumentCannotCheckACounterpart).
// Five of p76's payer labels match an operating fund AND its CIP twin -- Low
// Income Hsng 200/812, Traffic Imp Fee 510/823, Host Comm Impact 282/820,
// Measure D 550/828, State Gas Tax 560/834 -- and every twin is type: capital,
// so a leg under the wrong twin moves INSIDE the collapsed cell
// transfers-detail-ties-to-spine compares and that check stays green. Three of
// the five do not even change fund group, so factFundsResolve cannot see them.
// This is the only thing that does.
//
// WHAT IT ASSERTS. The fund each printed anchor names is the fund the row
// declares AT THAT END: "Transfer From X" is the payer, so it must equal the
// counterpart's fund, and "to Y" is the payee, so it must equal the row's own.
// Both the identity and the direction, because neither is covered elsewhere --
// see rowAnchorPrefix for the measurement that retired the argument that the
// arithmetic covers direction.
//
// TWO KINDS OF ROW IT CANNOT COVER, counted and named SEPARATELY rather than
// skipped, because the two are different claims about the page and the summary
// is printed on every run.
//
//   - NO ANCHOR ON THE LINE. p76 prints three continuation rows whose "Transfer
//     From" carries over from the row above, so their declared payer has no
//     printed anchor of its own to be checked against (fisc-30j). The other end
//     of the same row IS anchored, which is how these are told apart.
//   - NO VERB PHRASE. pp.85-125's Department Funding Sources rows are labelled
//     with a bare fund name -- "General Fund", "Cal Home Reuse" -- which names
//     a fund unambiguously and names no direction, so rowAnchorPrefixes matches
//     nothing and this check makes no claim at all. Seventy-eight rows, every
//     fund number hand-typed. Reporting them as "no printed anchor" would be
//     false about the page: the label is right there. Tracked as fisc-90fp.
type rowFundsMatchTheirAnchors struct{}

var _ Check = (*rowFundsMatchTheirAnchors)(nil)

func (*rowFundsMatchTheirAnchors) ID() string { return "row-funds-match-their-anchors" }
func (*rowFundsMatchTheirAnchors) Tier() int  { return 1 }
func (*rowFundsMatchTheirAnchors) Full() bool { return false }
func (*rowFundsMatchTheirAnchors) Description() string {
	return "every fund a row's printed anchors name is one that row declares, for the " +
		"schedules whose fund dimension is per row rather than per column"
}

func (*rowFundsMatchTheirAnchors) Run(_ context.Context, s *Subject) (Result, error) {
	var findings []Finding
	subjects, unanchored, unphrased := 0, 0, 0
	var unanchoredRows, unphrasedRows []string

	for _, f := range s.Files {
		for i := range f.Rules {
			ru := &f.Rules[i]
			// ONCE PER ROW, NOT ONCE PER PART. A rule's rows are shared by
			// every part it declares -- that is what labels_from is for -- so
			// iterating parts would report a multi-part schedule's every row
			// twice and inflate the subject count with it. Omissions are still
			// per part, so the union of what any part reads is the right set.
			for _, row := range activeInAnyPart(ru) {
				near, far := row.Fund, 0
				if row.Counterpart != nil {
					far = row.Counterpart.Fund
				}
				if near == 0 && far == 0 {
					continue
				}

				namedNear, namedFar, phrased := false, false, false
				for _, a := range []string{row.Label, row.LabelTail} {
					if anchorHasVerbPhrase(a) {
						phrased = true
					}
					entry, isFar, ok := resolveAnchorFund(s, a)
					if !ok {
						continue
					}
					want, end := near, "receives"
					if isFar {
						want, end = far, "pays"
					}
					if want == 0 {
						// The page names a fund at an end the row declares
						// none at. Not a mismatch -- LAVWMA is exactly this,
						// deliberately -- and not a subject either.
						continue
					}
					subjects++
					if isFar {
						namedFar = true
					} else {
						namedNear = true
					}
					if entry.Number == want {
						continue
					}
					findings = append(findings, finding(
						fmt.Sprintf("%s %q", ru.ID, row.PrintedLabel()),
						"the page prints %q, which is fund %d (%s), and this row "+
							"declares that the fund which %s is %d. A fund mis-typed "+
							"inside its own type changes no column sum and no other "+
							"check sees it",
						a, entry.Number, entry.Name, end, want))
				}

				// A declared fund with no printed anchor of its own is the
				// continuation-row case, and it is REPORTED rather than
				// failed: the page really does print no source on that line.
				for _, d := range []struct {
					fund  int
					named bool
					end   string
				}{{near, namedNear, "receives"}, {far, namedFar, "pays"}} {
					if d.fund == 0 || d.named {
						continue
					}
					// WHICH LIST IT GOES IN IS DECIDED BY THE PAGE, not by the
					// rule. A row whose OTHER end resolved is a continuation
					// row: the page really does print nothing here. A row where
					// no anchor carried a verb phrase at all has a printed
					// label this check simply does not read.
					where := fmt.Sprintf("%s %q (the fund that %s, %d)",
						ru.ID, row.PrintedLabel(), d.end, d.fund)
					if phrased {
						unanchored++
						unanchoredRows = append(unanchoredRows, where)
					} else {
						unphrased++
						unphrasedRows = append(unphrasedRows, where)
					}
				}
			}
		}
	}

	unanchoredNote := ""
	if unanchored > 0 {
		unanchoredNote = fmt.Sprintf("; %d declared fund(s) have no printed anchor on "+
			"their own line and are checked by nothing here: %s", unanchored,
			joinComma(unanchoredRows))
	}
	if unphrased > 0 {
		unanchoredNote += fmt.Sprintf("; a further %d declared fund(s) sit on rows whose "+
			"printed label names a fund but no direction, which %s matches nothing in, so "+
			"this check makes no claim about them either (fisc-90fp): %s", unphrased,
			"rowAnchorPrefixes", joinComma(unphrasedRows))
	}
	// THE VACUOUS SUMMARY HAS TO STAY TRUE OF THE CORPUS IT RAN OVER. If rows
	// declare funds and none of them is anchored, "no row declares a fund" is a
	// false statement -- and under --strict it would surface as an undeclared
	// vacancy carrying a misleading reason, which is worse than no reason.
	nothing := "no row declares a fund: every mapped schedule takes its fund from a " +
		"column, where rule-funds-match-their-headings checks it"
	if unanchored > 0 {
		nothing = "every fund declared on a row is at an end the page prints no anchor " +
			"for, so there is nothing to resolve" + unanchoredNote
	}
	return conclusion{
		subjects: subjects,
		unit:     "row anchors",
		held: fmt.Sprintf("%d row anchors name a fund, each one the fund its row declares "+
			"at that end of the movement", subjects) + unanchoredNote,
		nothing:  nothing,
		findings: findings,
	}.result(), nil
}

// anchorHasVerbPhrase reports whether a printed anchor opens with one of the
// verb phrases this check reads a direction from.
//
// It is the difference between "the page prints nothing here" and "the page
// prints a label this check does not read", and those are the two lists the
// summary keeps apart. resolveAnchorFund cannot answer it: it returns the same
// not-ok for an unmatched prefix and for a matched prefix whose remainder is
// not a fund, which are opposite statements about the page.
func anchorHasVerbPhrase(anchor string) bool {
	for _, p := range rowAnchorPrefixes {
		if strings.HasPrefix(anchor, p.prefix) {
			return true
		}
	}
	return false
}

// resolveAnchorFund strips a known verb phrase off a printed anchor and looks
// the remainder up. Not found is the common case and not an error: most rows in
// this corpus name a category rather than a fund.
func resolveAnchorFund(s *Subject, anchor string) (entry registry.Fund, far, ok bool) {
	if anchor == "" {
		return registry.Fund{}, false, false
	}
	label, matched := anchor, false
	for _, p := range rowAnchorPrefixes {
		if rest, cut := strings.CutPrefix(label, p.prefix); cut {
			label, far, matched = rest, p.far, true
			break
		}
	}
	if !matched {
		// No declared verb phrase, so the anchor says nothing about direction
		// and this check has no claim to make about it.
		return registry.Fund{}, false, false
	}
	e, err := s.Vocabulary.FundByLabel(label)
	if err != nil {
		return registry.Fund{}, false, false
	}
	return e, far, true
}

// activeInAnyPart is the union of the rows any part of a rule reads, in
// declaration order and without repeats.
func activeInAnyPart(ru *mapping.Rule) []mapping.Row {
	seen := map[string]bool{}
	var out []mapping.Row
	for j := range ru.Parts {
		for _, row := range ru.ActiveRows(&ru.Parts[j]) {
			if id := row.Identity(); !seen[id] {
				seen[id] = true
				out = append(out, row)
			}
		}
	}
	return out
}
