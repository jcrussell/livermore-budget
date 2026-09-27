package export

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/mapping"
	"github.com/jcrussell/livermore-budget/internal/project"
)

// The revenue categories and the funds they flow into. Folding to {0,3}
// leaves only the revenue side, since expenditure links fold inside one box
// and are dropped.
const (
	foldToRevenueCategory = 0
	foldToFund            = 3
)

// TestTheLineTierFoldsToTheCategoryLinks holds that the pp.127-140 revenue rows
// fold back to the same money between the same (category, fund) pairs, which
// no other check sees. Every column is folded to {0,3} and diffed against an
// independent re-netting of its facts that reads the category off the fact,
// so a re-pointed line shows. A printed zero row earns no link; those are
// TestThePrintedZeroRowsAreNotNodes'.
func TestTheLineTierFoldsToTheCategoryLinks(t *testing.T) {
	_, facts := committedStore(t)
	docs := builtFundFlows(t)
	if len(docs) == 0 {
		t.Fatal("no fund-flows document was built, so this test asserts nothing")
	}

	for stem, doc := range docs {
		t.Run(stem, func(t *testing.T) {
			folded, unplaceable, err := foldTo(doc, foldToRevenueCategory, foldToFund)
			if err != nil {
				t.Fatalf("folding %s to {0,3}: %v", stem, err)
			}
			// What the fold may lose is named per pair, so a revenue link
			// cannot quietly join it.
			gotDropped := make([]string, 0, len(unplaceable))
			for _, l := range unplaceable {
				gotDropped = append(gotDropped, l.Source+" -> "+l.Target)
			}
			wantDropped := make([]string, 0, len(unplaceable))
			for _, l := range doc.Links {
				if strings.HasPrefix(l.Source, "fund-group/") {
					wantDropped = append(wantDropped, l.Source+" -> "+l.Target)
				}
			}
			sort.Strings(wantDropped)
			if diff := cmp.Diff(wantDropped, gotDropped); diff != "" {
				t.Errorf("%s folded to {%d,%d} lost links that are not the group's own "+
					"rollups (-the document's (2,3) links +what the fold could not place):"+
					"\n%s", stem, foldToRevenueCategory, foldToFund, diff)
			}
			want := netByCategory(facts, doc.Metadata.FiscalYear, doc.Metadata.Basis)
			if diff := cmp.Diff(want, folded); diff != "" {
				t.Errorf("%s folded to {%d,%d} is not the category-grain document it "+
					"replaced (-independently re-netted +folded):\n%s",
					stem, foldToRevenueCategory, foldToFund, diff)
			}
		})
	}
}

// TestThePrintedZeroRowsAreNotNodes: a revenue row printed as a dash is a fact,
// not a flow, so it is both uncited and absent from the nodes. Either alone
// passes on the wrong document. A pp.172-183 object row printed as a dash is
// uncited for the same reason; a pp.167-170 one is cited by its division's
// total and is not counted here.
func TestThePrintedZeroRowsAreNotNodes(t *testing.T) {
	_, facts := committedStore(t)
	for stem, doc := range builtFundFlows(t) {
		t.Run(stem, func(t *testing.T) {
			cited := map[string]bool{}
			for _, l := range doc.Links {
				for _, id := range l.FactIDs {
					cited[id] = true
				}
			}
			ids := map[string]bool{}
			for _, n := range doc.Nodes {
				ids[n.ID] = true
			}

			zeros := 0
			rows := revenueRows(facts, doc.Metadata.FiscalYear, doc.Metadata.Basis)
			for _, f := range facts {
				if f.Scope == "expenditure-by-fund" && f.FiscalYear == doc.Metadata.FiscalYear &&
					string(f.Basis) == doc.Metadata.Basis {
					rows = append(rows, f)
				}
			}
			for _, f := range rows {
				if f.AmountCents != 0 {
					continue
				}
				zeros++
				if cited[f.ID] {
					t.Errorf("fact %s (%s %q, fund %s) prints a dash and some link cites it",
						f.ID, f.Category, f.RowLabel, fact.FundString(f.Fund))
				}
			}
			if got := doc.Metadata.Counts.FactsUncited; got != zeros {
				t.Errorf("counts.facts_uncited is %d and %d revenue and fund object rows print a dash; a "+
					"gap either way is money that reached no link for some other reason",
					got, zeros)
			}
		})
	}
}

// TestEveryRevenueLineIsParentedToItsPrintedCategory: a line's parent is its
// slug minus the last segment. The fold reports a wrong parent and a wrong
// cell alike; this says which.
func TestEveryRevenueLineIsParentedToItsPrintedCategory(t *testing.T) {
	lines := 0
	for stem, doc := range builtFundFlows(t) {
		byID := map[string]project.Node{}
		for _, n := range doc.Nodes {
			byID[n.ID] = n
		}
		for _, n := range doc.Nodes {
			if !strings.HasPrefix(n.ID, "revenue-line/") {
				continue
			}
			lines++
			slug := strings.TrimPrefix(n.ID, "revenue-line/")
			i := strings.LastIndex(slug, "/")
			if i < 0 {
				t.Errorf("%s: line %q is one segment, so it names no category", stem, n.ID)
				continue
			}
			want := "revenue/" + slug[:i]
			if n.Parent != want {
				t.Errorf("%s: line %q is parented to %q, want %q", stem, n.ID, n.Parent, want)
			}
			if p, ok := byID[want]; !ok || p.Tier != foldToRevenueCategory {
				t.Errorf("%s: line %q folds into %q, which this document does not carry at "+
					"tier %d", stem, n.ID, want, foldToRevenueCategory)
			}
		}
	}
	if lines == 0 {
		t.Error("no document carries a revenue line, so this test asserts nothing")
	}
}

// builtFundFlows is every fund-flows column `fisc export` writes, by stem.
func builtFundFlows(t *testing.T) map[string]*project.Document {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	built, err := buildProjections(root)
	if err != nil {
		t.Fatalf("buildProjections: %v", err)
	}
	out := map[string]*project.Document{}
	for stem, raw := range built {
		var doc project.Document
		if err := json.Unmarshal(raw, &doc); err != nil {
			t.Fatalf("decode %s: %v", stem, err)
		}
		if doc.Projection == project.FundFlowsProjection {
			out[stem] = &doc
		}
	}
	return out
}

// foldTo folds each link's ends to their nearest ancestor at a drawn tier,
// merging on the folded pair and dropping a link folded inside one box. It
// returns the links it could not place so the caller can name them.
func foldTo(doc *project.Document, tiers ...int) ([]project.Link, []project.Link, error) {
	drawn := map[int]bool{}
	for _, t := range tiers {
		drawn[t] = true
	}
	byID := map[string]project.Node{}
	for _, n := range doc.Nodes {
		byID[n.ID] = n
	}

	foldsTo := map[string]string{}
	placed := map[string]bool{}
	place := func(id string) error {
		if _, done := placed[id]; done {
			return nil
		}
		placed[id] = true
		at, ok := byID[id]
		if !ok {
			return fmt.Errorf("link names %s, which is not a node of this document", id)
		}
		for hops := 0; !drawn[at.Tier]; hops++ {
			parent, ok := byID[at.Parent]
			if !ok || hops > len(doc.Nodes) {
				return nil
			}
			at = parent
		}
		foldsTo[id] = at.ID
		return nil
	}
	var unplaceable []project.Link
	for _, l := range doc.Links {
		if err := place(l.Source); err != nil {
			return nil, nil, err
		}
		if err := place(l.Target); err != nil {
			return nil, nil, err
		}
	}

	merged := map[string]*project.Link{}
	var order []string
	cited := map[string]map[string]bool{}
	pages := map[string]map[string]map[int]bool{}
	for _, l := range doc.Links {
		source, target := foldsTo[l.Source], foldsTo[l.Target]
		if source == "" || target == "" {
			unplaceable = append(unplaceable, l)
			continue
		}
		if source == target {
			continue
		}
		key := source + "\x1f" + target
		at := merged[key]
		if at == nil {
			at = &project.Link{Source: source, Target: target, Kind: l.Kind,
				TransferID: l.TransferID, Derived: l.Derived}
			merged[key] = at
			cited[key] = map[string]bool{}
			pages[key] = map[string]map[int]bool{}
			order = append(order, key)
		}
		if at.Kind != l.Kind || at.Derived != l.Derived {
			return nil, nil, fmt.Errorf("%s -> %s folds together a %s flow and a %s one",
				source, target, at.Kind, l.Kind)
		}
		at.ValueCents += l.ValueCents
		for _, id := range l.FactIDs {
			cited[key][id] = true
		}
		for _, s := range l.Locators {
			if pages[key][s.DocID] == nil {
				pages[key][s.DocID] = map[int]bool{}
			}
			for _, p := range s.Pages {
				pages[key][s.DocID][p] = true
			}
		}
	}

	out := make([]project.Link, 0, len(order))
	for _, key := range order {
		l := *merged[key]
		l.FactIDs = sortedSet(cited[key])
		l.Locators = locatorsOf(pages[key])
		out = append(out, l)
	}
	sortLinksByEnds(out)
	sortLinksByEnds(unplaceable)
	return out, unplaceable, nil
}

// netByCategory re-nets one column's revenue facts to one link per (kind,
// category, fund), reading fact fields only. Like the projection, a row that
// nets to zero earns no link.
func netByCategory(facts []fact.Fact, year int, basis string) []project.Link {
	type key struct {
		kind     mapping.Kind
		category string
		group    string
		fund     string
	}
	byRow := map[string]int64{}
	selected := revenueRows(facts, year, basis)
	for _, f := range selected {
		byRow[rowOf(f)] += f.AmountCents
	}

	cents := map[key]int64{}
	ids := map[key]map[string]bool{}
	pages := map[key]map[string]map[int]bool{}
	var order []key
	for _, f := range selected {
		if byRow[rowOf(f)] == 0 {
			continue
		}
		k := key{f.Kind, f.Category, f.FundGroup, fact.FundString(f.Fund)}
		if ids[k] == nil {
			ids[k] = map[string]bool{}
			pages[k] = map[string]map[int]bool{}
			order = append(order, k)
		}
		cents[k] += f.AmountCents
		ids[k][f.ID] = true
		if pages[k][f.DocID] == nil {
			pages[k][f.DocID] = map[int]bool{}
		}
		pages[k][f.DocID][f.Page] = true
	}
	out := make([]project.Link, 0, len(order))
	for _, k := range order {
		source := "revenue/" + k.category
		kind := project.KindExternal
		if k.kind == mapping.KindTransferIn {
			source = k.category
			kind = project.KindInternalTransfer
		} else if k.group == "internal-service" {
			kind = project.KindInternalService
		}
		out = append(out, project.Link{
			Source: source, Target: "fund/" + k.fund,
			ValueCents: cents[k], Kind: kind,
			FactIDs: sortedSet(ids[k]), Locators: locatorsOf(pages[k]),
		})
	}
	sortLinksByEnds(out)
	return out
}

// revenueRows is one column's revenue-by-fund facts, in the order the store
// carries them.
func revenueRows(facts []fact.Fact, year int, basis string) []fact.Fact {
	var out []fact.Fact
	for _, f := range facts {
		if f.Scope == project.ScopeRevenueByFund && f.FiscalYear == year &&
			string(f.Basis) == basis {
			out = append(out, f)
		}
	}
	return out
}

// rowOf addresses one printed row of one fund: the finest cell pp.127-140 print.
func rowOf(f fact.Fact) string {
	return fmt.Sprintf("%s\x1f%s\x1f%s", f.Category, f.RowLabel, fact.FundString(f.Fund))
}

func sortedSet(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// locatorsOf renders a doc -> pages index the way a published link carries it:
// documents ascending, pages ascending within each, never nil.
func locatorsOf(m map[string]map[int]bool) []project.Source {
	out := make([]project.Source, 0, len(m))
	for doc, ps := range m {
		nums := make([]int, 0, len(ps))
		for p := range ps {
			nums = append(nums, p)
		}
		sort.Ints(nums)
		out = append(out, project.Source{DocID: doc, Pages: nums})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].DocID < out[j].DocID })
	return out
}

func sortLinksByEnds(links []project.Link) {
	sort.Slice(links, func(i, j int) bool {
		if links[i].Source != links[j].Source {
			return links[i].Source < links[j].Source
		}
		return links[i].Target < links[j].Target
	})
}

// TestAContraRowIsANegativeLinkOnItsOwnLine pins p127's parenthesised ERAF and
// RPTTF Reduction rows as negative values on the line's link to the fund and
// on its rollup, both against the printed figures: the only placement that
// folds back to the category cell.
func TestAContraRowIsANegativeLinkOnItsOwnLine(t *testing.T) {
	want := map[string]map[string]int64{
		// p0127 "ERAF (14,086,438) (14,661,836) (15,175,000) (15,857,875)" and
		// "RPTTF Reduction (1,749,120) (1,704,595) (1,810,339) (1,891,804)".
		"fund-flows":              {"eraf": -1517500000, "rpttf-reduction": -181033900},
		"fund-flows-2027":         {"eraf": -1585787500, "rpttf-reduction": -189180400},
		"fund-flows-2025-revised": {"eraf": -1466183600, "rpttf-reduction": -170459500},
		// Not a contra row: p0127 "Prior Year - Unsecured" is `sign: positive`
		// and printed (20,033) in FY2023-24 actual alone (fisc-9psv).
		"fund-flows-2024-actual": {"eraf": -1408643800, "rpttf-reduction": -174912000,
			"prior-year-unsecured": -2003300},
	}

	for stem, doc := range builtFundFlows(t) {
		t.Run(stem, func(t *testing.T) {
			got := map[string]int64{}
			rolled := map[string]int64{}
			for _, l := range doc.Links {
				if l.ValueCents >= 0 {
					continue
				}
				line := strings.TrimPrefix(l.Source, "revenue-line/taxes/property/")
				switch {
				case l.Target == "fund/100":
					got[line] = l.ValueCents
				case l.Target == "revenue/taxes/property" &&
					strings.HasPrefix(l.Source, "revenue-line/taxes/property/"):
					rolled[line] = l.ValueCents
				default:
					t.Errorf("%s -> %s is negative at %d, and every negative row pp.127-140 "+
						"print is a General Fund property-tax line", l.Source, l.Target,
						l.ValueCents)
				}
			}
			if diff := cmp.Diff(want[stem], got); diff != "" {
				t.Errorf("the negative links of %s are not the rows p127 prints in "+
					"parentheses (-printed +published):\n%s", stem, diff)
			}
			// p127 prints each row in the General Fund column alone, so the
			// rollup is the same figure.
			if diff := cmp.Diff(want[stem], rolled); diff != "" {
				t.Errorf("the rollups of %s's contra lines into Property Taxes are not the "+
					"rows p127 prints in parentheses (-printed +published):\n%s", stem, diff)
			}
		})
	}
}
