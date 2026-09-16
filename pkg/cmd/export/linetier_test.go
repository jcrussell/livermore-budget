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

// The two tiers the line tier is folded back to: the revenue categories the
// drill-down published before it existed, and the funds they flow into.
//
// FOLDING THE WHOLE DOCUMENT TO {0,3} LEAVES THE REVENUE SIDE ALONE, which is
// what makes this one comparison rather than a filtered one. Both expenditure
// links fold to fund/100 -> fund/100 -- the tier-4-to-5 one through dept, the
// tier-3-to-4 one directly -- and a link whose ends fold together is a flow
// inside one box and is dropped, by this fold and by site/app.js's alike.
const (
	foldToRevenueCategory = 0
	foldToFund            = 3
)

// TestTheLineTierFoldsToTheCategoryLinks is the proof that publishing pp.127-140
// as rows REFINED the drill-down rather than changing it.
//
// A LINE TIER THAT DOES NOT FOLD BACK TO WHAT IT REPLACED HAS CHANGED THE
// DOCUMENT, and no other check in this tree can tell the two apart. The amounts
// still tie to the spine either way (revenue-detail-ties-to-spine sums facts and
// never reads a link), every link still cites facts that sum to it
// (link-values-tie-to-facts is per link), and the counts still reconcile against
// the document's own arrays. What none of them holds is that the SAME money
// still runs between the SAME pairs of nodes.
//
// So each document is folded to {0,3} and diffed against an INDEPENDENT
// re-netting of its own facts by (kind, category, fund) -- the key revKey held
// before the line was added to it. The oracle below reads no registry, calls
// nothing in internal/project, and reaches the category off the fact's own
// field rather than off a node's parent, which is what makes a re-pointed line
// visible here: the fold carries the money to whatever category the document
// says, and the oracle carries it to the one the fact says.
//
// EVERY COLUMN, not the two the site draws. The projection declares four and
// they are not the same shape -- FY2023-24 carries a seventh fund group -- so a
// fold proved over one is a fold proved over one.
//
// THE ONE THING THE FOLD DOES NOT REPRODUCE IS A PRINTED ZERO'S CITATION, and
// it is the refinement rather than a gap in it. A cell that nets to zero earns
// no link, and the cell is now the printed ROW: measured over the committed
// store, 6 rows in each adopted column, 3 in FY2023-24 actual and 8 in FY2024-25
// revised print a dash inside a category cell that is not zero, so the category
// link used to cite them and no line link does. They are not lost -- they are
// counted in counts.facts_uncited, which is what that number is for, and
// TestThePrintedZeroRowsAreNotNodes holds them there. The oracle applies the
// same rule at the same grain and says so.
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
			// WHAT THE FOLD IS ALLOWED TO LOSE IS NAMED, PER PAIR. The (2,3)
			// rollup runs from a parentless tier-2 group, so {0,3} can place
			// neither it nor any other link out of one; asserting the pairs
			// rather than a count is what stops a REVENUE link quietly joining
			// the set the comparison below never sees.
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

// TestThePrintedZeroRowsAreNotNodes is the other half of the fold, and the half
// a green cmp.Diff would hide.
//
// A PRINTED DASH IS A FACT AND NOT A FLOW. The city prints a zero in a revenue
// row to say the line exists and was nil this year; drawing it would put a
// d3-sankey node of zero height beside the lines that did happen, and it would
// assert that a row the city printed as nothing is a thing that happens.
//
// Every such row must therefore be UNCITED and absent from the node list, and
// the two are asserted together because either alone passes on the wrong
// document: a zero-valued link would leave the fact cited and the node drawn,
// and a fact dropped before netting would leave it uncited with nothing saying
// so.
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
			for _, f := range revenueRows(facts, doc.Metadata.FiscalYear, doc.Metadata.Basis) {
				if f.AmountCents != 0 {
					continue
				}
				zeros++
				if cited[f.ID] {
					t.Errorf("fact %s (%s %q, fund %s) prints a dash and some link cites it",
						f.ID, f.Category, f.RowLabel, fact.FundString(f.Fund))
				}
			}
			// The identity the document publishes, restated as the claim this
			// test is about: the uncited facts are the printed zeros and
			// nothing else.
			if got := doc.Metadata.Counts.FactsUncited; got != zeros {
				t.Errorf("counts.facts_uncited is %d and %d revenue rows print a dash; a "+
					"gap either way is money that reached no link for some other reason",
					got, zeros)
			}
		})
	}
}

// TestEveryRevenueLineIsParentedToItsPrintedCategory is the edge the fold walks,
// asserted at the producer.
//
// The fold above cannot distinguish a line parented to the wrong category from
// one whose facts were netted into the wrong cell -- both move the same money to
// the same wrong place -- so it reports either as a diff. This says which,
// against the taxonomy's own grammar: a line slug is its category plus one
// segment, so the parent a node publishes must be the id form of the slug minus
// that segment.
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
//
// Selected by the projection each document names rather than by a list of
// stems, so a fifth published column is covered the day it is declared.
func builtFundFlows(t *testing.T) map[string]*project.FundFlowsDocument {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	built, err := buildProjections(root)
	if err != nil {
		t.Fatalf("buildProjections: %v", err)
	}
	out := map[string]*project.FundFlowsDocument{}
	for stem, raw := range built {
		var doc project.FundFlowsDocument
		if err := json.Unmarshal(raw, &doc); err != nil {
			t.Fatalf("decode %s: %v", stem, err)
		}
		if doc.Projection == project.FundFlowsProjection {
			out[stem] = &doc
		}
	}
	return out
}

// foldTo is site/app.js's foldDocument over a Go document, kept to the two
// clauses this comparison needs: each node folds to its nearest ancestor at a
// drawn tier, links fold with their ends and merge on the folded pair, and a
// link whose ends fold together is dropped.
//
// IT RETURNS WHAT IT COULD NOT PLACE RATHER THAN REFUSING IT, and that is
// stricter than the refusal it replaces rather than weaker. A fund group is
// parentless at tier 2, so the (2,3) rollup out of one can be placed by no fold
// of this document to {0,3}; refusing it would make this comparison
// unstateable, and dropping it in silence would lose a column. Returning it
// lets the caller name the links a fold is allowed to lose and go red on any
// other -- which is what the caller does, per pair.
//
// IT PLACES THE ENDS OF LINKS AND NOT EVERY NODE, which {0,3} needs and the
// client's own fold does not. site/app.js reaches the same document through
// filterToNode, whose quiet branch has already dropped the end this cannot
// place.
func foldTo(doc *project.FundFlowsDocument, tiers ...int) ([]project.Link, []project.Link, error) {
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

// netByCategory re-nets the revenue facts of one column at the grain the
// drill-down published before the line tier existed: one link per (kind,
// category, fund) cell that is not a printed zero.
//
// INDEPENDENT OF THE PROJECTION, which is the whole of its value. It reads the
// fact fields and nothing else: no registry, no revKey, no revenueEndpoint, no
// Node.Parent. The one rule it shares with the projection is the one this
// document has always had -- a cell that nets to zero earns no link -- and it is
// applied at the ROW, because the row is the cell the schedule prints and a
// category cell was only ever a sum of rows.
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
		// The source end is the category itself for revenue and the flow
		// endpoint for a transfer, which is where a transfer's row stays: it is
		// not a line of anything.
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

// TestAContraRowIsANegativeLinkOnItsOwnLine pins where the sign goes, which the
// diff of a regenerated golden will not show.
//
// THE CHILD MODEL FORCES THE PLACEMENT. pp.127-140 print ERAF and RPTTF
// Reduction in parentheses inside the Property Taxes subtotal -- they are
// shift-aways of property tax to the county, and the printed total of the
// thirteen detail rows is 64,143,762 only if they are read as negative. While
// the category was the node, they netted inside its cell and no link was ever
// negative. Now that the row IS the node, the only placement that folds back to
// that cell is a negative value on the line's own link: a reversed positive link
// would fold to a fund -> category flow that does not exist, and a
// sign-decomposed endpoint -- the spine's answer for CHANGE IN WORKING CAPITAL --
// would fold to a link from a node the category grain never had.
//
// TWO LINKS CARRY THE SIGN NOW, not one, and the second is checked against the
// same printed figures rather than against the first.
//
// The figures are the ones p127 prints, so this is a comparison against the
// document rather than against the pipeline.
func TestAContraRowIsANegativeLinkOnItsOwnLine(t *testing.T) {
	want := map[string]map[string]int64{
		// p0127 "ERAF (14,086,438) (14,661,836) (15,175,000) (15,857,875)" and
		// "RPTTF Reduction (1,749,120) (1,704,595) (1,810,339) (1,891,804)".
		"fund-flows":              {"eraf": -1517500000, "rpttf-reduction": -181033900},
		"fund-flows-2027":         {"eraf": -1585787500, "rpttf-reduction": -189180400},
		"fund-flows-2025-revised": {"eraf": -1466183600, "rpttf-reduction": -170459500},
		// THE THIRD ONE IS NOT A CONTRA ROW. p0127 "Prior Year - Unsecured"
		// prints (20,033) in the FY2023-24 actual column alone and the taxonomy
		// declares it `sign: positive`, so it reaches the document as an
		// ordinary row that was negative that year (fisc-9psv). It is here
		// because a test listing only the declared contra rows would go red on
		// this column for a reason that reads as a defect in the sign rule.
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
				// THE ROLLUP CARRIES THE SIGN THROUGH, for the reason the flow
				// into the fund does: a reduction of Property Taxes reduces the
				// category, so the only placement that adds the rows back up to
				// p127's printed 64,143,762 is a negative value on the line's
				// own rollup.
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
			// THE SAME FIGURES AGAIN, AND THAT IS A CLAIM ABOUT THE PAGE: p127
			// prints each of these rows in the General Fund column alone, so
			// the line's whole sum IS its one cell. A rollup that had gathered
			// a second fund's money, or one cell short of the line, would
			// differ here while the column above stayed green.
			if diff := cmp.Diff(want[stem], rolled); diff != "" {
				t.Errorf("the rollups of %s's contra lines into Property Taxes are not the "+
					"rows p127 prints in parentheses (-printed +published):\n%s", stem, diff)
			}
		})
	}
}
