package check

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/jcrussell/livermore-budget/internal/mapping"
)

// ruleFundsMatchTheirHeadings asserts every hand-typed `fund:` agrees with the
// fund name the page prints beside the figures it reads.
//
// THE HOLE IT CLOSES IS THE ONE fisc-u2v's DESIGN PASS COULD NOT. A rule author
// writes `fund:` by hand and nothing derives it from the heading the rule
// anchors on, so a fund mis-assigned WITHIN ITS OWN FUND TYPE survives every
// other check at once:
//
//   - revenue-detail-ties-to-spine passes, because both funds are in the same
//     fund group and the column sums are unchanged;
//   - fact-funds-resolve passes, because it asserts the fund EXISTS and that its
//     type matches the fact's fund group, and both are true of the wrong fund;
//   - CheckTotals passes, because the printed `Total <fund>` line never mentions
//     a number.
//
// Every check green and the fact filed under the wrong fund, which is the
// plausible-wrong-value class this project exists to refuse. It is not
// hypothetical: p134 prints `Doolan Canyon Preserve Endow` and p76 prints
// `Doolan Canyon Open Space`, funds 470 and 471, both permanent, and pp.131-140
// name about seventy funds of which several pairs differ only in a trailing word.
//
// TWO CLAUSES, AND THEY CATCH DIFFERENT MISTAKES.
//
// (1) THE NUMBER AGAINST THE NAME. registry.FundByLabel resolves the printed
// name exactly — never a prefix, never case-folded, ambiguity refused when
// data/funds.yaml loads — so a heading that resolves to no fund is an error
// rather than a guess, and one that resolves to a different number than the rule
// declares is the mis-assignment above.
//
// It reads PRINTED TOTALS and not section anchors, and that is not
// interchangeable. Six parts of the revenue schedule anchor on a column-header
// fragment because their rows begin at the top of a page — the leading gap may
// not contain a digit, so the heading on the previous page cannot be the anchor
// — and resolving `"FY 2026-27"` as a fund name would produce six findings whose
// cheapest escape is inventing a fund alias for a column header.
//
// AND THE PRINTED TOTAL THAT NAMES THE FUND IS NOT ALWAYS THE RULE'S OWN. Two
// schedules sit under this check and they are shaped differently. pp.131-140 are
// one fund per rule and each prints `Total <fund>`, so the rule's own total names
// its number. pp.127-130 are ONE fund decomposed by CATEGORY across ten rules,
// whose printed totals are `Total Property Taxes` and the like — category names,
// not fund names — and the fund is named exactly once, by the rollup
// `Total General Fund` that covers all ten. So the check asks whether SOME
// printed total that governs the rule names the fund it declares, its own or a
// rollup's, and reports a rule that no printed line anchors at all.
//
// (2) THE PAGES AGAINST THE RULES. Clause 1 only inspects rules that exist, so
// it cannot see a fund section nobody mapped. That gap is real and the
// arithmetic does not close it: NINE of the schedule's 69 funds print zero in
// BOTH budget years — 2022 COP Construction Fund, Transferable Development Cred,
// Doolan Canyon Preserve Endow, four grant funds, Import Mitigation Fee, Human
// Services Facility Fee — so dropping any one of them leaves every FY2026 and
// FY2027 sum unchanged and revenue-detail-ties-to-spine green, while up to
// $4.7M of FY2024 revenue vanishes. So the check reads the pages those rules
// touch and requires every printed fund total on them to be claimed.
//
// WHAT IT STILL DOES NOT CATCH, stated rather than glossed: a total_row and a
// fund number that are BOTH wrong in the same direction. That is narrower than
// fisc-u54 feared, because the section anchor decides which rows are read — a
// rule anchored on the wrong block maps different figures and its printed total
// stops tying — but it is not nothing, and no check here sees it.
type ruleFundsMatchTheirHeadings struct{}

var _ Check = (*ruleFundsMatchTheirHeadings)(nil)

func (*ruleFundsMatchTheirHeadings) ID() string { return "rule-funds-match-their-headings" }
func (*ruleFundsMatchTheirHeadings) Tier() int  { return 1 }
func (*ruleFundsMatchTheirHeadings) Full() bool { return false }
func (*ruleFundsMatchTheirHeadings) Description() string {
	return "every fund number a rule declares is the fund named by the total its pages print, " +
		"and every printed fund total on those pages is claimed by a rule"
}

// printedTotalLabel is the label half of a printed total line, and whether the
// line prints one at all.
//
// THE SPLIT IS THE COLUMN GRID. A label is separated from its figures by a run
// of two or more spaces, which is what `pdftotext -layout` encodes and what
// makes a single space inside a fund name safe. A line with no such run, or with
// nothing after it, is prose rather than a row.
//
// IT RETURNS THE WHOLE LABEL AND DOES NOT LOOK FOR THE FUND NAME. That is
// fundNameIn's job, and keeping them apart is what let clause 2 learn the second
// printed shape without a second regex: this answers "is this a total line",
// fundNameIn answers "which fund does it name", and the shapes live in exactly
// one of the two.
func printedTotalLabel(line string) (string, bool) {
	label, figures, ok := strings.Cut(strings.TrimLeft(line, " \t"), "  ")
	if !ok || strings.TrimSpace(figures) == "" {
		return "", false
	}
	label = strings.TrimSpace(label)
	if label == "" || !strings.Contains(label, "Total") {
		return "", false
	}
	return label, true
}

func (*ruleFundsMatchTheirHeadings) Run(_ context.Context, s *Subject) (Result, error) {
	var findings []Finding
	subjects := 0

	// The pages a fund-bearing rule reads, and the printed totals already
	// spoken for. Both are per document: two documents may print the same
	// heading and mean different funds.
	pages := map[string]map[int]bool{}
	// CLAIMED IS KEYED ON THE PAGE AS WELL AS THE DOCUMENT (fisc-lkx). It used
	// to be (doc_id, printed label), with no page dimension at all, while the
	// claim this check makes is explicitly per page -- "on the pages a schedule
	// maps, the schedule maps all of it". So one mapped total silenced an
	// identically-worded total on every OTHER swept page of the same document,
	// which is exactly the hole clause 2 exists to close.
	//
	// Measured before the fix: rollup gf-total-revenues claims "Total General
	// Fund" from p130 for the whole document, and p0166 and p0172 print the same
	// line. Neither is swept today, so nothing was wrong in the committed
	// corpus -- but both adjoin pp.167-170, whose 23 division rules declare
	// fund 100, so any rule whose parts reached p166 or p172 opened it.
	claimed := map[string]map[claimKey]bool{}

	for _, f := range s.Files {
		for _, ro := range f.Rollups {
			// A rollup's total is a printed line too, and p130's names a fund.
			// A rollup declares exactly one page and its total is printed there,
			// so this claim is exact rather than approximate.
			set(claimed, f.DocID, ro.Page, ro.TotalRow)
		}
		for i := range f.Rules {
			ru := &f.Rules[i]

			// THE TOTAL IS CLAIMED BEFORE EITHER CONTINUE BELOW, and that
			// ordering is the fix (fisc-948). Claiming a total is the statement
			// that SOME rule reads that printed heading, which is true whether
			// or not the rule declares one fund. Claiming it after the guards
			// meant a rule mapping a fund section with fund_group-only columns
			// -- or one whose columns declare two funds, already reported for
			// that -- never claimed its own printed total, and clause 2's
			// unclaimedFundTotals then reported a MAPPED section as one no rule
			// or rollup declares: two findings from one cause, the second false.
			//
			// Latent on the committed corpus, which declares a fund on every
			// column of every fund-bearing rule. It goes live the moment a
			// schedule declares its fund per ROW, which is what p76 does.
			// ON EVERY PAGE OF THE RULE'S PARTS, which is an OVER-claim and is
			// still strictly narrower than the document-wide claim it replaces.
			// A rule's total is printed on ONE of its pages -- for a
			// total_spans_parts rule, the one totalBearingPart finds -- and
			// establishing which needs the pages, i.e. a resolver this function
			// does not have. Narrowing it to the bearing page is filed
			// separately; it is not folded in here because reaching for
			// s.Resolvers to answer it would make a vocabulary check depend on
			// the corpus being readable.
			for j := range ru.Parts {
				set(claimed, f.DocID, ru.Parts[j].Page, ru.TotalRow)
			}

			fund, mixed := declaredFund(ru)
			if mixed {
				findings = append(findings, finding(ru.ID,
					"this rule's columns declare more than one fund, so no single heading "+
						"can name it and the figures beneath one label belong to two funds"))
				continue
			}
			if fund == 0 {
				continue
			}
			for j := range ru.Parts {
				setPage(pages, f.DocID, ru.Parts[j].Page)
			}
			subjects++

			// The printed totals that govern this rule: its own, and every
			// rollup that covers it.
			anchors := []string{}
			if ru.TotalRow != "" {
				anchors = append(anchors, ru.TotalRow)
			}
			for _, ro := range f.Rollups {
				for _, id := range ro.Covers {
					if id == ru.ID {
						anchors = append(anchors, ro.TotalRow)
					}
				}
			}

			// `named` and `reported` are two different facts and the check
			// needs both. `named` is "a printed total governing this rule names
			// the fund it declares"; `reported` is "this rule already has a
			// finding". The mismatch arm below sets only the second, because
			// setting `named` would make the variable lie -- and NOT setting
			// something meant the !named arm fired too, producing a second
			// finding that said the rule "is anchored to nothing on the page"
			// while listing the total that anchors it (fisc-948).
			named, reported := false, false
			for _, a := range anchors {
				label, ok := fundNameIn(a)
				if !ok {
					continue
				}
				entry, err := s.Vocabulary.FundByLabel(label)
				if err != nil {
					// A total naming something other than a fund is not a
					// defect: pp.127-130 print ten category totals. It simply
					// does not anchor the number, so keep looking.
					continue
				}
				if entry.Number != fund {
					findings = append(findings, finding(ru.ID,
						"this rule declares fund %d and %q governs it, which is fund %d "+
							"(%s). A fund mis-assigned inside its own type changes no "+
							"column sum and no other check sees it",
						fund, a, entry.Number, entry.Name))
					reported = true
					continue
				}
				named = true
			}
			if !named && !reported {
				findings = append(findings, finding(ru.ID,
					"this rule declares fund %d and no printed total governing it names "+
						"that fund: %s. The number is hand-typed and anchored to nothing "+
						"on the page", fund, describeAnchors(anchors)))
			}
		}
	}

	findings = append(findings, unclaimedFundTotals(s, pages, claimed)...)

	return conclusion{
		subjects: subjects,
		unit:     "fund-bearing rules",
		held: fmt.Sprintf("%d rules declare a fund, each the fund named by the total its "+
			"pages print, and every printed fund total on those pages is claimed",
			subjects),
		nothing: "no rule declares a fund: the citywide spine's columns are fund groups, and " +
			"pp.66-67 print no per-fund column",
		findings: findings,
	}.result(), nil
}

// fundNameIn is the fund name a printed total line carries, in either of the two
// shapes this corpus prints, and whether it carries one at all.
//
// TWO SHAPES, BECAUSE THE CORPUS PRINTS TWO, and reading only the first was a
// live gap rather than a tidiness point. pp.131-140 print `Total <fund>` and
// pp.127-130's rollup prints `Total General Fund`, so a leading cut answered
// every anchor that existed while the fund-bearing rules were the revenue
// schedule's. pp.167-170 put the fund FIRST: p0170:23 is
// `General Fund Total Expenses`, mapped as the rollup gf-total-expenses, which
// covers all 23 division rules on those pages. That anchor names the fund, in
// print, and governs every rule there -- and a leading cut alone rejects it, so
// declaring `fund: 100` on those rules produced 23 findings against a heading
// sitting right there in the mapping.
//
// BOTH SHAPES RESOLVE EXACTLY AND NEITHER IS A PREFIX MATCH. data/funds.yaml
// spends twenty lines forbidding one, and the exactness is what keeps the five
// operating/CIP twins apart -- Water 640 from Water Replacement 642. What is cut
// here is the word `Total` and the schedule's own trailing noun, never a
// fragment of the fund name: `General Fund Total Expenses` yields the whole of
// `General Fund` and nothing shorter. FundByLabel then answers or it does not.
func fundNameIn(total string) (string, bool) {
	if label, ok := strings.CutPrefix(total, "Total "); ok {
		return label, true
	}
	// `<fund> Total <noun>`: everything before the first " Total" is the
	// candidate name. Cutting at the FIRST occurrence matters -- a fund whose
	// own name contained "Total" would otherwise lose part of itself, and a
	// short wrong answer that resolves to nothing is a skipped anchor rather
	// than a wrong fund.
	if label, _, ok := strings.Cut(total, " Total"); ok && label != "" {
		return label, true
	}
	return "", false
}

// declaredFund is the fund every column of a rule declares, and whether they
// disagree. Zero means no column names one, which is the spine.
func declaredFund(ru *mapping.Rule) (fund int, mixed bool) {
	seen := map[int]bool{}
	for i := range ru.Parts {
		for _, c := range ru.Parts[i].Columns {
			if c.Skip {
				continue
			}
			seen[c.Fund] = true
		}
	}
	if len(seen) != 1 {
		// A rule with no parts, or one whose columns disagree. Zero and one are
		// the same answer here only when the one is zero.
		if len(seen) == 0 {
			return 0, false
		}
		return 0, true
	}
	for k := range seen {
		return k, false
	}
	return 0, false
}

// unclaimedFundTotals reads the pages the fund-bearing rules touch and reports
// every printed `Total <fund>` that no rule or rollup declares.
//
// It reads ONLY those pages, deliberately. Sweeping every page of every document
// would report the appendix schedules and the fund-balance pages, which print
// fund totals nobody has mapped and nobody claimed to have — hundreds of
// findings about work that was never started, which is how a check gets muted.
// The claim this makes is bounded and true: on the pages a schedule maps, the
// schedule maps all of it.
func unclaimedFundTotals(s *Subject, pages map[string]map[int]bool,
	claimed map[string]map[claimKey]bool) []Finding {

	var findings []Finding
	for _, docID := range sortedPageSets(pages) {
		doc, ok := s.Docs[docID]
		if !ok {
			continue
		}
		for _, n := range sortedPages(pages[docID]) {
			text, err := doc.Page(n)
			if err != nil {
				findings = append(findings, finding(fmt.Sprintf("%s p%d", docID, n),
					"a rule reads this page and it cannot be read: %v", err))
				continue
			}
			for _, line := range strings.Split(text, "\n") {
				label, ok := printedTotalLabel(line)
				if !ok {
					continue
				}
				// BOTH PRINTED SHAPES, through the same helper clause 1 uses.
				// While this clause carried its own leading-`Total ` regex it
				// could not see `General Fund Total Expenses` at all, so on
				// pp.167-170 -- swept for the first time when those rules
				// declared fund 100 -- it reported coverage it was not
				// providing.
				name, ok := fundNameIn(label)
				if !ok {
					continue
				}
				entry, err := s.Vocabulary.FundByLabel(name)
				if err != nil {
					// Not a fund total. pp.127-130 print ten CATEGORY totals
					// and this is how they are told apart -- by the registry,
					// not by a list in this file.
					continue
				}
				// Claimed under the label AS PRINTED, which is what a rule's
				// total_row holds. Rebuilding "Total "+name would only ever
				// match the leading shape and would report a claimed trailing
				// one as unclaimed.
				if claimed[docID][claimKey{page: n, label: label}] {
					continue
				}
				findings = append(findings, finding(fmt.Sprintf("%s p%d", docID, n),
					"the page prints %q, which is fund %d (%s), and no rule or rollup "+
						"declares that total. A fund section nobody mapped is invisible to "+
						"the reconciliation whenever it is zero in the years the spine "+
						"publishes", label, entry.Number, entry.Name))
			}
		}
	}
	return findings
}

// describeAnchors renders the printed totals that govern a rule, so a finding
// says what the check DID look at rather than only that it found nothing.
func describeAnchors(anchors []string) string {
	if len(anchors) == 0 {
		return "no total_row and no rollup covers it"
	}
	out := make([]string, 0, len(anchors))
	for _, a := range anchors {
		out = append(out, fmt.Sprintf("%q", a))
	}
	sort.Strings(out)
	return "the totals governing it are " + joinComma(out)
}

// claimKey is one printed total on one page.
type claimKey struct {
	page  int
	label string
}

func set(m map[string]map[claimKey]bool, doc string, page int, key string) {
	if key == "" {
		return
	}
	if m[doc] == nil {
		m[doc] = map[claimKey]bool{}
	}
	m[doc][claimKey{page: page, label: key}] = true
}

func setPage(m map[string]map[int]bool, doc string, page int) {
	if m[doc] == nil {
		m[doc] = map[int]bool{}
	}
	m[doc][page] = true
}

func sortedPageSets(m map[string]map[int]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedPages(m map[int]bool) []int {
	out := make([]int, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Ints(out)
	return out
}
