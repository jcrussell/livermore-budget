package check

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/jcrussell/livermore-budget/internal/mapping"
)

// ruleFundsMatchTheirHeadings asserts every hand-typed `fund:` agrees with the
// fund name the page prints beside the figures it reads. A fund mis-assigned
// within its own fund type passes cuts-tie-along-the-lattice,
// fact-funds-resolve and CheckTotals at once.
//
// Two clauses:
//
// (1) The number against the name: some printed total governing the rule, its
// own or a covering rollup's, names the fund it declares, resolved exactly by
// registry.FundByLabel. It reads printed totals, not section anchors, which
// may be column-header fragments.
//
// (2) The pages against the rules: every printed fund total on the pages those
// rules touch is claimed, since a fund printing zero in both budget years can
// go unmapped with every sum unchanged.
//
// It does not catch a total_row and a fund number both wrong in the same
// direction (fisc-u54).
type ruleFundsMatchTheirHeadings struct{}

func (*ruleFundsMatchTheirHeadings) meta() meta {
	return meta{id: "rule-funds-match-their-headings", tier: 1,
		description: "every fund number a rule declares is the fund named by the total its pages print, " +
			"and every printed fund total on those pages is claimed by a rule"}
}

// printedTotalLabel is the label half of a printed total line, and whether the
// line prints one at all. The label ends at the first run of two spaces, the
// `-layout` column grid. The fund name inside it is fundNameIn's.
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
	// claimed is keyed per page, so a total mapped on one page does not
	// silence an identically-worded one on another ("Total General Fund" on
	// p0130, p0166 and p0172).
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

			// The total is claimed before either continue below: a rule
			// declaring its fund per row still reads its printed heading.
			// Claimed on every page of the rule's parts, an over-claim;
			// narrowing it to the bearing page is fisc-id8b.
			for j := range ru.Parts {
				set(claimed, f.DocID, ru.Parts[j].Page, ru.WrappedTotalLabel())
			}

			fund, mixed := declaredFund(ru)
			if mixed {
				subjects++
				findings = append(findings, columnFundsNamed(s, f.DocID, ru)...)
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
				anchors = append(anchors, ru.WrappedTotalLabel())
			}
			for _, ro := range f.Rollups {
				for _, id := range ro.Covers {
					if id == ru.ID {
						anchors = append(anchors, ro.TotalRow)
					}
				}
			}

			// named: a governing total names the declared fund. reported: the
			// rule already has a finding, so the !named arm does not add a
			// second, false one.
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
// shapes this corpus prints, and whether it carries one at all: `Total <fund>`
// (pp.127-140) and `<fund> Total Expenses` (p0170).
//
// Neither is a prefix match: only `Total` and the schedule's trailing noun are
// cut, never a fragment of the fund name, which keeps operating/CIP twins
// (Water 640, Water Replacement 642) apart.
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
			lines := strings.Split(text, "\n")
			for i, line := range lines {
				label, ok := printedTotalLabel(line)
				if !ok {
					continue
				}
				// BOTH PRINTED SHAPES, through the same helper clause 1 uses, so
				// `General Fund Total Expenses` on p170 is read as fund 100.
				name, ok := fundNameIn(label)
				if !ok {
					continue
				}
				entry, err := s.Vocabulary.FundByLabel(name)
				// A `Total <fund>` wrapping onto the next line names its fund
				// rejoined, as a rule's total_row_tail does, and the rejoined
				// label is the printed one: "Total Water" over "Replacement" is
				// 642, and a rule claiming "Total Water" claims 640, not it.
				// The trailing shape ends its name before " Total", so no tail
				// can change it.
				if strings.HasPrefix(label, "Total ") && i+1 < len(lines) {
					if tail := strings.TrimSpace(lines[i+1]); tail != "" && !isFigureRow(tail) {
						whole := mapping.JoinWrapped(label, tail)
						if joined, ok := fundNameIn(whole); ok {
							if e, jerr := s.Vocabulary.FundByLabel(joined); jerr == nil {
								entry, err, label = e, nil, whole
							}
						}
					}
				}
				if err != nil {
					// Not a fund total. pp.127-130 print ten CATEGORY totals
					// and this is how they are told apart -- by the registry,
					// not by a list in this file.
					continue
				}
				// Claimed under the label AS PRINTED, rejoined where it wraps,
				// which is what a rule's WrappedTotalLabel holds. Rebuilding
				// "Total "+name would only ever match the leading shape and would
				// report a claimed trailing one as unclaimed.
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

// isFigureRow says a line is a row of its own rather than a wrapped fragment:
// cells set off by a column gap, with a figure among them. A fragment may hold
// a digit ("2009-1 Maint") or a gap, not both.
func isFigureRow(line string) bool {
	line = strings.TrimSpace(line)
	return strings.Contains(line, "  ") && strings.ContainsAny(line, "0123456789")
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
	return "the totals governing it are " + strings.Join(out, ", ")
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

// columnFundsNamed holds a rule whose columns declare more than one fund to
// the page that prints them: every part reading a fund column prints, on one
// line, the name data/funds.yaml gives each of its funds, left to right in
// the order its columns declare them. Budget Book p81 heads its requirements
// "Airport Fund", "Water Fund", "Grand Total", over columns declaring 600 and
// 640. A rule whose funds no heading names, or names in another order, is two
// funds' figures filed under each other's numbers.
//
// Mutation: swap fund 600 and 640 on p81's columns and this finds the page
// prints them in the other order.
func columnFundsNamed(s *Subject, docID string, ru *mapping.Rule) []Finding {
	doc, ok := s.Docs[docID]
	if !ok {
		return []Finding{finding(ru.ID, "this rule's columns declare more than one fund, and %s "+
			"cannot be read to find the headings naming them", docID)}
	}
	var findings []Finding
	for i := range ru.Parts {
		p := &ru.Parts[i]
		var names []string
		var funds []int
		for _, c := range p.Columns {
			if c.Skip || c.Fund == 0 || (len(funds) > 0 && funds[len(funds)-1] == c.Fund) {
				continue
			}
			entry, ok := s.Vocabulary.Fund(c.Fund)
			name := entry.Name
			if !ok {
				findings = append(findings, finding(ru.ID, "p%d declares fund %d, which data/funds.yaml "+
					"does not list", p.Page, c.Fund))
				return findings
			}
			names = append(names, name)
			funds = append(funds, c.Fund)
		}
		if len(names) == 0 {
			continue
		}
		text, err := doc.Page(p.Page)
		if err != nil {
			return append(findings, finding(ru.ID, "p%d cannot be read: %v", p.Page, err))
		}
		named := false
		for _, line := range strings.Split(text, "\n") {
			at := 0
			inOrder := true
			for _, n := range names {
				if !strings.HasSuffix(n, " Fund") {
					n += " Fund"
				}
				j := strings.Index(line[at:], n)
				if j < 0 {
					inOrder = false
					break
				}
				at += j + len(n)
			}
			if inOrder {
				named = true
				break
			}
		}
		if !named {
			findings = append(findings, finding(ru.ID, "p%d's columns declare more than one fund, %v, "+
				"and no line of the page heads them %q in that order", p.Page, funds, names))
		}
	}
	return findings
}
