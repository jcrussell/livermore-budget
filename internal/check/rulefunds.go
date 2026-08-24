package check

import (
	"context"
	"fmt"
	"regexp"
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

// printedTotal matches a printed `Total <name>` line and captures the name. The
// figures are separated from the label by a run of two or more spaces, which is
// the column grid `pdftotext -layout` encodes; a single space inside a fund name
// is therefore safe.
var printedTotal = regexp.MustCompile(`(?m)^[ \t]*Total ([^\n]*?)[ \t]{2,}`)

func (*ruleFundsMatchTheirHeadings) Run(_ context.Context, s *Subject) (Result, error) {
	var findings []Finding
	subjects := 0

	// The pages a fund-bearing rule reads, and the printed totals already
	// spoken for. Both are per document: two documents may print the same
	// heading and mean different funds.
	pages := map[string]map[int]bool{}
	claimed := map[string]map[string]bool{}

	for _, f := range s.Files {
		for _, ro := range f.Rollups {
			// A rollup's total is a printed line too, and p130's names a fund.
			set(claimed, f.DocID, ro.TotalRow)
		}
		for i := range f.Rules {
			ru := &f.Rules[i]
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
			set(claimed, f.DocID, ru.TotalRow)
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

			named := false
			for _, a := range anchors {
				label, ok := strings.CutPrefix(a, "Total ")
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
					continue
				}
				named = true
			}
			if !named {
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
	claimed map[string]map[string]bool) []Finding {

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
			for _, m := range printedTotal.FindAllStringSubmatch(text, -1) {
				label := strings.TrimSpace(m[1])
				entry, err := s.Vocabulary.FundByLabel(label)
				if err != nil {
					// Not a fund total. pp.127-130 print ten CATEGORY totals
					// and this is how they are told apart -- by the registry,
					// not by a list in this file.
					continue
				}
				if claimed[docID]["Total "+label] {
					continue
				}
				findings = append(findings, finding(fmt.Sprintf("%s p%d", docID, n),
					"the page prints %q, which is fund %d (%s), and no rule or rollup "+
						"declares that total. A fund section nobody mapped is invisible to "+
						"the reconciliation whenever it is zero in the years the spine "+
						"publishes", "Total "+label, entry.Number, entry.Name))
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

func set(m map[string]map[string]bool, doc, key string) {
	if key == "" {
		return
	}
	if m[doc] == nil {
		m[doc] = map[string]bool{}
	}
	m[doc][key] = true
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
