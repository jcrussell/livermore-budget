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
// fund it names, and WHICH END OF THE MOVEMENT that fund is at.
//
// The list is written out rather than inferred, for the reason
// rule-funds-match-their-headings writes out "Total ": a check that guessed
// where the fund name starts would resolve a different string than the author
// read, and the whole value here is that both are reading the same printed
// words. Budget Book p76 phrases its rows "Transfer From <payer>  to <payee>".
//
// far IS WHAT MAKES THIS A DIRECTION CHECK. A swap of a row's ends moves no
// group sum when both ends are in one group: swapping "Transfer From Water  to
// Water Replacement" (640/642, both enterprise) rebuilds with every other check
// green.
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
// cuts-tie-along-the-lattice compares and that check stays green. Three of
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
//
//   - NO VERB PHRASE, AND NO DECLARATION. A row labelled with a bare fund name
//     -- "General Fund", "Cal Home Reuse" -- names a fund unambiguously and
//     names no direction, so rowAnchorPrefixes matches nothing. Reporting it as
//     "no printed anchor" would be false about the page: the label is right
//     there. Such a row is read only where its rule declares
//     row_labels_name_funds, which is what the bare-label arm of Run is for --
//     it runs FIRST, ahead of the prefix arm, and the ordering matters because
//     each reads the same string for a different purpose. Where the rule does
//     not declare it, this check still makes no claim and the summary says so
//     per rule. pp.85-125's 78 rows all declare it (fisc-90fp).
//
//     THE DIRECTION HALF IS NOT RECOVERED BY THAT DECLARATION and must not be.
//     A bare fund name says which fund, never which end, so the bare-label arm
//     asserts the row's own fund and never the counterpart's. What the verb
//     phrase buys over the declaration is exactly the `side` field above.
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
	// bare counts the subjects the bare-label arm resolved, so the summary can keep
	// the two claims apart rather than averaging them into one false sentence.
	bare := 0
	// numbered counts the ends the fund-number arm resolved.
	numbered := 0
	var unanchoredRows []string
	// The rules an unphrased declaration belongs to, deduplicated: 78 rows over
	// eleven rules is a list of eleven, not of 78. See the note where it is
	// formatted.
	var unphrasedRules []string
	seenRule := map[string]bool{}

	for _, f := range s.Files {
		for i := range f.Rules {
			ru := &f.Rules[i]
			// ONCE PER ROW, NOT ONCE PER PART. A rule's rows are shared by
			// every part it declares -- that is what labels_from is for -- so
			// iterating parts would report a multi-part schedule's every row
			// twice and inflate the subject count with it. Omissions are still
			// per part, so the union of what any part reads is the right set.
			for _, row := range activeInAnyPart(ru) {
				// A SKIPPED ROW IS NOT READ FROM THE PAGE, so this check has
				// nothing to say about it in ANY arm -- not as a subject, not as
				// a fund the page prints no anchor for. ActiveRows filters
				// omitted rows and not skipped ones, so without this they arrive
				// here.
				//
				// IT SITS AT THE TOP OF THE LOOP AND NOT INSIDE ONE ARM, which
				// is the second attempt. The first put `!row.Skip` on the
				// bare-label arm alone, which excluded such a row from being
				// CHECKED and left it in the omission counters -- so the summary
				// then said "no row_labels_name_funds declaration covers that
				// end" about a rule that declares it. That is the same false
				// sentence the counterpart refusal had just closed at the root,
				// reintroduced one commit later by the fix for a different
				// finding. Both passes found it; the second found it in the
				// first's fix.
				if row.Skip {
					continue
				}
				near, far := row.Fund, 0
				if row.Counterpart != nil {
					far = row.Counterpart.Fund
				}
				if near == 0 && far == 0 {
					continue
				}

				namedNear, namedFar, phrased := false, false, false

				// THE FUND-NUMBER ARM. The label is the row's own fund as the
				// page prints it, and on a transfer the tail's first field is
				// the payer's, so both ends are held, direction included. An
				// end the row declares and the page does not number is a
				// finding, never a skip.
				if ru.RowLabelsAreFundNumbers {
					ends := []struct {
						printed string
						want    int
						end     string
					}{{row.Label, near, "receives"}}
					tail, _, _ := strings.Cut(row.LabelTail, " ")
					if far != 0 {
						ends = append(ends, struct {
							printed string
							want    int
							end     string
						}{tail, far, "pays"})
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

				// THE BARE-LABEL ARM, and it runs before the prefix arm
				// because the two read the same string for different
				// purposes. A rule that declares row_labels_name_funds says
				// its labels ARE printed fund names, so the label is resolved
				// whole rather than after stripping a verb phrase off it --
				// "Cal Home Reuse" names a fund unambiguously and names no
				// direction, so this asserts the row's OWN fund and never the
				// counterpart's.
				//
				// IT IS FAIL-CLOSED, and that is the whole reason it is worth
				// writing rather than skipping the unresolvable case. If a
				// label that resolves to nothing were a `continue`, a rule
				// could declare this over labels the registry has never heard
				// of and the check would report exactly nothing -- vacuously
				// green, with a declaration standing over it that reads like a
				// guarantee. So an unresolved label is a FINDING, and
				// namedNear is set on both paths so the row never also lands
				// in the omission counters below as a fund the page prints no
				// anchor for. The page prints the anchor; that is what the
				// declaration asserts.
				// A VERB-PHRASED LABEL UNDER THIS DECLARATION IS REFUSED, not
				// deferred and not resolved whole. Three attempts, and the two
				// that failed are worth recording because each looked right.
				//
				// RESOLVING IT WHOLE was the original commit. "Transfer From
				// General Fund  to Horizons" is not the printed name of a fund,
				// so FundByLabel finds nothing: measured, flagging
				// p76-transfers-in-special-revenue took the check to FAIL over
				// 121 subjects with three spurious findings, each row also
				// counted twice because the prefix arm read it as well.
				//
				// DEFERRING TO THE PREFIX ARM was the fix for that, and it left
				// a hole. The parser forbids a counterpart on a declaring rule,
				// so far == 0; a row labelled "Transfer From X" resolves as the
				// FAR end, want == far == 0, and the arm skips it without
				// comparing anything. Measured: injecting one drops subjects
				// 118 -> 117, the check stays PASS, and the row is reported
				// under "no printed anchor on their own line" -- false, the
				// label is right there. So the deferral turned a noisy defect
				// into a silent one.
				//
				// REFUSING IS WHAT BOTH OF THOSE WERE REACHING FOR. The
				// declaration says the label IS a fund name; a label carrying a
				// verb phrase is a different shape and the rule author has said
				// something untrue about their own page. Failing closed here
				// also does what fisc-mjdw was filed to do -- a declaration that
				// asserts nothing can no longer be written -- so that bead is
				// retired by this rather than left open on a wrong premise.
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

	// EACH CLAUSE HAS TO READ ON ITS OWN, because either arm can fire without
	// the other. Seeding the note in the first arm and appending "a further" in
	// the second left a corpus of nothing but bare-label rows announcing "a
	// further 78" with no prior count -- and the test written for that corpus
	// asserted Contains("78 declared fund(s)"), which the malformed string
	// satisfies.
	var clauses []string
	if unanchored > 0 {
		clauses = append(clauses, fmt.Sprintf("%d declared fund(s) have no printed anchor "+
			"on their own line and are checked by nothing here: %s", unanchored,
			joinComma(unanchoredRows)))
	}
	// COUNTED AND ATTRIBUTED TO ITS RULES, NOT LISTED ROW BY ROW, which is the
	// opposite of the arm above and deliberately so. detailtie.go's argument
	// applies here: a count is the honest middle, loud without being a wall. The
	// three rows above are three, are p76's own, and each needs naming because a
	// reader cannot otherwise find which line the page leaves blank. These are
	// a whole schedule at a time -- naming pp.85-125's 78 individually turned
	// this check's PASS line into 11 KB on every run, which is a list nobody
	// reads reported as a summary. The rules are what a reader acts on.
	//
	// THIS ARM IS EMPTY OVER THE COMMITTED CORPUS as of fisc-90fp, because the
	// eleven rules it was written for now declare row_labels_name_funds and are
	// read by the arm above. It is kept rather than deleted: declining the
	// declaration is a legitimate state -- every schedule mapped before those
	// eleven is in it -- and this is the sentence that says so out loud instead
	// of letting a rule be silently unread. TestAVacuousRowFundsSummaryCannot-
	// DenyTheRowsItSaw exercises it by clearing the flag.
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
			lead, unphrased, joinComma(unphrasedRules)))
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
	// BOTH COUNTERS GUARD IT, and testing only the first was a defect this check
	// shipped with for one commit: over a corpus of nothing but pp.85-125 every
	// declared fund is unphrased rather than unanchored, so the check reported
	// vacuous while announcing that no row declares a fund and 78 did.
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
		held:     heldLine(subjects-numbered, bare) + numberedClause(numbered) + unanchoredNote,
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

// numberedClause states the fund-number arm's claim, or nothing where no rule
// declares row_labels_are_fund_numbers.
func numberedClause(numbered int) string {
	if numbered == 0 {
		return ""
	}
	return fmt.Sprintf("; and %d printed fund numbers under row_labels_are_fund_numbers, each "+
		"the fund its row declares at the end whose column prints it", numbered)
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
