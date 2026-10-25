package check

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/jcrussell/livermore-budget/internal/mapping"
	"github.com/jcrussell/livermore-budget/internal/registry"
)

// rowAnchorPrefix is a verb phrase a printed row anchor carries in front of the
// fund it names, and which end of the movement that fund is at. Written out,
// not inferred, so the check reads the same printed words the author did.
//
// far is what makes this a direction check: swapping a row's ends within one
// group (Water to Water Replacement, 640/642) moves no group sum. An anchor
// resolving to nothing after stripping is not a defect.
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
// page names in a row rather than in a heading, as Budget Book p76 does; such
// a rule declares fund 0 and never enters rule-funds-match-their-headings.
//
// The page's own totals cannot check a counterpart's fund
// (TestTheDocumentCannotCheckACounterpart), and a leg under an operating
// fund's CIP twin moves inside the cell cuts-tie-along-the-lattice compares,
// so this is the only guard on it.
//
// The fund each printed anchor names is the fund the row declares at that
// end: "Transfer From X" the counterpart's, "to Y" the row's own.
//
// Two kinds of row it cannot cover are counted and reported separately:
//
//   - a continuation row with no anchor on its own line, told apart because
//     its other end is anchored.
//   - a bare fund-name label with no verb phrase, read only where its rule
//     declares row_labels_name_funds (the bare-label arm, which runs before
//     the prefix arm). A bare name says which fund, never which end, so that
//     arm asserts only the row's own fund.
type rowFundsMatchTheirAnchors struct{}

func (*rowFundsMatchTheirAnchors) meta() meta {
	return meta{id: "row-funds-match-their-anchors", tier: 1,
		description: "every fund a row's printed anchors name is one that row declares, for the " +
			"schedules whose fund dimension is per row rather than per column"}
}

func (*rowFundsMatchTheirAnchors) Run(_ context.Context, s *Subject) (Result, error) {
	var findings []Finding
	subjects, unanchored, unphrased := 0, 0, 0
	// bare counts the subjects the bare-label arm resolved, kept apart in the
	// summary.
	bare := 0
	// numbered counts the ends the fund-number arm resolved.
	numbered := 0
	var unanchoredRows []string
	// The rules an unphrased declaration belongs to, deduplicated.
	var unphrasedRules []string
	seenRule := map[string]bool{}

	for _, f := range s.Files {
		for i := range f.Rules {
			ru := &f.Rules[i]
			// Once per row, not per part: parts share rows, so iterating
			// parts would count a multi-part schedule's rows twice.
			for _, row := range activeInAnyPart(ru) {
				// A skipped row is not read from the page, so no arm speaks
				// of it; at the top of the loop so it stays out of the
				// omission counters too.
				if row.Skip {
					continue
				}
				near, far := row.Fund, 0
				if row.Counterpart != nil {
					far = row.Counterpart.Fund
				}
				// A fund-number rule declares every row's label a fund, so a row
				// declaring none is read, not skipped.
				if near == 0 && far == 0 && !ru.RowLabelsAreFundNumbers {
					continue
				}

				namedNear, namedFar, phrased := false, false, false

				// THE FUND-NUMBER ARM. The label is the row's own fund as the
				// page prints it, and on a transfer the tail's first field is
				// the payer's, so both ends are held, direction included. An
				// end the row declares and the page does not number is a
				// finding, never a skip.
				if ru.RowLabelsAreFundNumbers {
					ends := []numberedEnd{{row.Label, near, "receives"}}
					// Split as the resolver's findFields splits it.
					tail := ""
					if f := strings.Fields(row.LabelTail); len(f) > 0 {
						tail = f[0]
					}
					if far != 0 {
						ends = append(ends, numberedEnd{tail, far, "pays"})
					} else if _, err := strconv.Atoi(tail); err == nil {
						// The page numbers a paying fund and the row declares none:
						// a transfer read as something else.
						subjects++
						numbered++
						findings = append(findings, finding(
							fmt.Sprintf("%s %q", ru.ID, row.PrintedLabel()),
							"this rule declares that its rows print fund numbers, and the "+
								"page prints %q as the fund which pays where this row declares "+
								"no counterpart", tail))
					}
					for _, e := range ends {
						subjects++
						numbered++
						if n, err := strconv.Atoi(e.printed); err != nil || n != e.want {
							findings = append(findings, finding(
								fmt.Sprintf("%s %q", ru.ID, row.PrintedLabel()),
								"this rule declares that its rows print fund numbers, and the "+
									"page prints %q where this row declares that the fund which "+
									"%s is %d", e.printed, e.end, e.want))
						}
					}
					continue
				}

				// The bare-label arm resolves the whole label and asserts
				// only the row's own fund. Fail-closed: an unresolved label
				// is a finding, not a skip, and namedNear is set on both
				// paths so the row never lands in the omission counters.
				//
				// A verb-phrased label under row_labels_name_funds is
				// refused. Resolving it whole finds nothing; deferring to the
				// prefix arm resolves it as the far end, want == far == 0,
				// and skips it silently.
				phrasedLabel := anchorHasVerbPhrase(row.Label) ||
					anchorHasVerbPhrase(row.LabelTail)
				if ru.RowLabelsNameFunds && phrasedLabel {
					findings = append(findings, finding(
						fmt.Sprintf("%s %q", ru.ID, row.PrintedLabel()),
						"this rule declares that its row labels are printed fund names "+
							"and %q opens with a verb phrase, which names a movement "+
							"rather than a fund. A schedule whose rows name two ends is "+
							"read by the anchor arm and must not declare this",
						row.PrintedLabel()))
				}
				if ru.RowLabelsNameFunds && near != 0 && !phrasedLabel {
					subjects++
					bare++
					namedNear = true
					label := row.PrintedLabel()
					switch entry, err := s.Vocabulary.FundByLabel(label); {
					case err != nil:
						findings = append(findings, finding(
							fmt.Sprintf("%s %q", ru.ID, label),
							"this rule declares that its row labels are printed fund "+
								"names and %q is not one data/funds.yaml records, so "+
								"the fund %d typed on this row is checked against "+
								"nothing: %v", label, near, err))
					case entry.Number != near:
						findings = append(findings, finding(
							fmt.Sprintf("%s %q", ru.ID, label),
							"the page prints %q, which is fund %d (%s), and this row "+
								"declares fund %d. A fund mis-typed inside its own "+
								"type changes no column sum and no other check sees it",
							label, entry.Number, entry.Name, near))
					}
				}

				for _, a := range []string{row.Label, row.LabelTail} {
					entry, isFar, matched, lookupErr := resolveAnchorFund(s, a)
					if !matched {
						continue
					}
					phrased = true
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
					// A verb phrase names the fund at this end, so a name
					// the registry does not resolve leaves the declared fund
					// checked against nothing: a finding, as in the
					// bare-label arm, and not a row with no printed anchor.
					if lookupErr != nil {
						findings = append(findings, finding(
							fmt.Sprintf("%s %q", ru.ID, row.PrintedLabel()),
							"the page prints %q, which names the fund that %s, and the "+
								"name is not one data/funds.yaml records, so the fund %d "+
								"this row declares there is checked against nothing: %v",
							a, end, want, lookupErr))
						continue
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
					// The page decides the list: a row whose other end
					// resolved is a continuation row; one with no verb phrase
					// has a label this check does not read.
					where := fmt.Sprintf("%s %q (the fund that %s, %d)",
						ru.ID, row.PrintedLabel(), d.end, d.fund)
					if phrased {
						unanchored++
						unanchoredRows = append(unanchoredRows, where)
						continue
					}
					unphrased++
					if !seenRule[ru.ID] {
						seenRule[ru.ID] = true
						unphrasedRules = append(unphrasedRules, ru.ID)
					}
				}
			}
		}
	}

	// Each clause reads on its own, because either arm can fire without the
	// other.
	var clauses []string
	if unanchored > 0 {
		clauses = append(clauses, fmt.Sprintf("%d declared fund(s) have no printed anchor "+
			"on their own line and are checked by nothing here: %s", unanchored,
			strings.Join(unanchoredRows, ", ")))
	}
	// Counted and attributed to rules, not listed row by row: these come a
	// whole schedule at a time. Empty over the committed corpus (fisc-90fp);
	// it says out loud that a rule declining the declaration is unread.
	// TestAVacuousRowFundsSummaryCannotDenyTheRowsItSaw exercises it.
	if unphrased > 0 {
		lead := ""
		if unanchored > 0 {
			lead = "a further "
		}
		clauses = append(clauses, fmt.Sprintf("%s%d declared fund(s) sit at an end this "+
			"check does not read: the row's label carries no verb phrase, so "+
			"rowAnchorPrefixes matches nothing in it, and its rule does not declare "+
			"row_labels_name_funds. The label may well be printed -- pp.85-125's rows "+
			"are bare fund names -- but nothing here reads it, so no claim is made "+
			"about them: %s",
			lead, unphrased, strings.Join(unphrasedRules, ", ")))
	}
	unanchoredNote := ""
	if len(clauses) > 0 {
		unanchoredNote = "; " + strings.Join(clauses, "; ")
	}
	// THE VACUOUS SUMMARY HAS TO STAY TRUE OF THE CORPUS IT RAN OVER. If rows
	// declare funds and none of them is anchored, "no row declares a fund" is a
	// false statement -- and under --strict it would surface as an undeclared
	// vacancy carrying a misleading reason, which is worse than no reason.
	//
	// BOTH COUNTERS GUARD IT: over a corpus of nothing but pp.85-125 every
	// declared fund is unphrased rather than unanchored, and the check must not
	// report vacuous while announcing that no row declares a fund.
	nothing := "no row declares a fund: every mapped schedule takes its fund from a " +
		"column, where rule-funds-match-their-headings checks it"
	if unanchored > 0 || unphrased > 0 {
		nothing = "every fund declared on a row is at an end this check can make no claim " +
			"about, so there is nothing to resolve" + unanchoredNote
	}
	return conclusion{
		subjects: subjects,
		unit:     "row anchors",
		// TWO CLAIMS AND NOT ONE, because the arms assert different things and a
		// summary saying "at that end of the movement" about all of them was
		// false for the bare-label rows the moment they became subjects. A verb
		// phrase carries direction and this check reads it; a bare fund name
		// carries none and it must not claim to.
		held:     heldLines(subjects, bare, numbered) + unanchoredNote,
		nothing:  nothing,
		findings: findings,
	}.result(), nil
}

// heldLine states what the check resolved, keeping the two arms' claims apart.
//
// A verb phrase carries direction and this check reads it; a bare fund name
// carries none and must not claim to. One sentence covering both was false for
// the bare-label rows the moment they became subjects, in a string fisc verify
// prints on every run.
//
// The second clause is omitted rather than printed as a zero, because a corpus
// where no rule declares row_labels_name_funds is the ordinary case and a
// summary announcing "and 0 a bare fund name" reports the absence of a feature
// as though it were a result.
func heldLine(subjects, bare int) string {
	phrased := subjects - bare
	// BOTH SIDES ARE GUARDED, which is the second version. The first guarded
	// only the bare side, so a corpus of nothing but declaring rules printed
	// "0 printed with a verb phrase, each the fund its row declares at that end
	// of the movement" -- the zero-clause-as-a-result shape this function's own
	// comment says it exists to avoid, reintroduced on the other axis by the fix
	// that introduced the comment.
	switch {
	case bare == 0:
		return fmt.Sprintf("%d row anchors name a fund, each one the fund its row "+
			"declares at that end of the movement", subjects)
	case phrased == 0:
		return fmt.Sprintf("%d row anchors name a fund, each a bare fund name under "+
			"row_labels_name_funds and each the fund its row declares -- a bare label "+
			"names no direction, so none is checked", subjects)
	default:
		return fmt.Sprintf("%d row anchors name a fund: %d printed with a verb phrase, "+
			"each the fund its row declares at that end of the movement, and %d a bare "+
			"fund name under row_labels_name_funds, each the fund its row declares -- a "+
			"bare label names no direction, so none is checked", subjects, phrased, bare)
	}
}

// numberedEnd is one end of a row the fund-number arm reads: the number the
// page prints, the fund the row declares there, and which end it is.
type numberedEnd struct {
	printed string
	want    int
	end     string
}

// heldLines is heldLine with the fund-number arm's claim beside it, each
// clause omitted where its arm resolved nothing.
func heldLines(subjects, bare, numbered int) string {
	numberedClause := fmt.Sprintf("%d printed fund numbers under row_labels_are_fund_numbers, "+
		"each the fund its row declares at the end whose column prints it", numbered)
	switch {
	case numbered == 0:
		return heldLine(subjects, bare)
	case subjects == numbered:
		return numberedClause
	}
	return heldLine(subjects-numbered, bare) + "; and " + numberedClause
}

// anchorHasVerbPhrase reports whether a printed anchor opens with one of the
// verb phrases this check reads a direction from.
//
// It is the difference between "the page prints nothing here" and "the page
// prints a label this check does not read", and those are the two lists the
// summary keeps apart.
func anchorHasVerbPhrase(anchor string) bool {
	for _, p := range rowAnchorPrefixes {
		if strings.HasPrefix(anchor, p.prefix) {
			return true
		}
	}
	return false
}

// resolveAnchorFund strips a known verb phrase off a printed anchor and looks
// the remainder up. No verb phrase is the common case and not an error: most
// rows in this corpus name a category rather than a fund. A verb phrase whose
// remainder the registry does not resolve is matched with the lookup's error.
func resolveAnchorFund(s *Subject, anchor string) (entry registry.Fund, far, matched bool, err error) {
	if anchor == "" {
		return registry.Fund{}, false, false, nil
	}
	label := anchor
	for _, p := range rowAnchorPrefixes {
		if rest, cut := strings.CutPrefix(label, p.prefix); cut {
			label, far, matched = rest, p.far, true
			break
		}
	}
	if !matched {
		// No declared verb phrase, so the anchor says nothing about direction
		// and this check has no claim to make about it.
		return registry.Fund{}, false, false, nil
	}
	e, err := s.Vocabulary.FundByLabel(label)
	if err != nil {
		return registry.Fund{}, far, true, err
	}
	return e, far, true, nil
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
