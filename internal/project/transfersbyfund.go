package project

import (
	"fmt"
	"sort"
	"strconv"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/mapping"
	"github.com/jcrussell/livermore-budget/pkg/cmdutil"
)

// TransfersByFundProjection is this document's name and file stem.
const TransfersByFundProjection = "transfers-by-fund"

// TransfersByFundScope is the schedule this document is of: Budget Book p76,
// Summary of Transfers.
//
// IT IS ITS OWN SCOPE AND IT CANNOT JOIN ANOTHER DOCUMENT'S. Measured on the
// committed store, transfers-by-fund and revenue-by-fund share 22 keys of
// (kind, category, fund_group, fund, year, basis) carrying $42,183,495 of
// transfer_in across the published columns, because pp.127-140 print a fund's
// transfers in and p76 prints the same movement from the payer's end. One
// document holding both would double it, which projection-scopes-are-disjoint
// refuses; and at the spine's scope the same rows double the city's transfers.
const TransfersByFundScope = "transfers-by-fund"

// TransfersByFundScopes is the schedule set, as [Options.Scopes] holds it.
func TransfersByFundScopes() []string { return []string{TransfersByFundScope} }

// transfersByFundCounts is how much of the corpus this document accounts for.
//
// facts = facts_cited + facts_uncited, and here facts_cited is EVERY fact of a
// row the page prints a figure for, because each leg is its own link. That is
// the difference between this document and every other one in the project: a
// cell elsewhere nets several facts into one ribbon, so its citation is a set;
// a leg here is one printed figure read in one direction, so its citation is a
// singleton and the two counts move together.
//
// transfers is the number of printed movements drawn, which is links/2. It is
// published because it is the figure a reader needs to not double the schedule:
// summing every link comes to twice p76's grand total, by construction.
type transfersByFundCounts struct {
	Facts        int `json:"facts"`
	FactsCited   int `json:"facts_cited"`
	FactsUncited int `json:"facts_uncited"`
	Transfers    int `json:"transfers"`
	Nodes        int `json:"nodes"`
	Links        int `json:"links"`
}

// transfersByFundMetadata is this document's metadata block. It embeds
// [Envelope] rather than [MultiScopeEnvelope] for departmentSpendingMetadata's
// reason: this document is of one schedule and one printed column.
type transfersByFundMetadata struct {
	Envelope
	FiscalYear      int                   `json:"fiscal_year"`
	FiscalYearLabel string                `json:"fiscal_year_label"`
	Basis           string                `json:"basis"`
	Sources         []Source              `json:"sources"`
	Counts          transfersByFundCounts `json:"counts"`
	Caveats         []Caveat              `json:"caveats"`
}

// TransfersByFundDocument is the whole published file.
type TransfersByFundDocument struct {
	SchemaVersion int                     `json:"schema_version"`
	Projection    string                  `json:"projection"`
	Metadata      transfersByFundMetadata `json:"metadata"`
	Nodes         []Node                  `json:"nodes"`
	Links         []Link                  `json:"links"`
}

// transfersByFund draws Budget Book p76, Summary of Transfers: which fund pays
// each transfer the city makes and which fund receives it.
//
// # The shape
//
//	tier 0  transfers/in              parent ""
//	          |  the fold, not a flow: no link runs from it
//	tier 2  transfer-from/<fund>      parent "transfers/in"
//	          |  the RECEIVING leg, one link per printed figure
//	tier 3  fund/<fund>               parent ""
//	          |  the PAYING leg, one link per printed figure
//	tier 5  transfer-to/<fund>        parent ""
//
// ONE PRINTED FIGURE IS TWO LINKS, and that is this document's whole structure
// rather than a duplication. p76 names both ends of every movement, so the fact
// store carries two facts per printed figure -- a receiving leg keyed to the
// destination fund and a paying leg keyed to the payer, citing the same
// doc_id/page/offset (fisc-4rh). A link may cite only facts that SUM to its
// value, so one ribbon carrying both legs would publish a figure twice the one
// the page prints. Two links, one per leg, each citing one fact, is the shape
// that keeps link-values-tie-to-facts exact -- and it is what [Link.TransferID]
// was declared for: the two legs carry the same id, and transfer-legs-pair
// asserts they are two and equal.
//
// THE PAYER IS ITS OWN ID FORM BECAUSE fund/<a> -> fund/<b> CANNOT BE DRAWN.
// The natural link runs tier 3 to tier 3: node-tiers-are-declared refuses it,
// since it is neither a rollup into the source's own parent nor a partition --
// p76 IS money moving, not one table read along a second axis, and declaring
// otherwise would be a false claim on the wire -- and d3-sankey cannot lay it
// out either, both ends taking the same column index. `transfer-from/<number>`
// at tier 2 is the payer's end of a movement rather than the payer itself, the
// same construct nodeTransfersIn already is on the spine, and it makes the link
// run coarse to fine with no new exception anywhere.
//
// `transfer-to/<number>` IS THE MIRROR AND IT IS NOT SYMMETRIC IN USE. The
// receiving legs are drawn: `fisc export`'s transfers step opens the spine's
// transfers/in into tiers {2,3}. The paying legs are published and no view
// draws them, because the spine pins transfers/out at tier 5 and a node
// decomposing it would have to be FINER than its own parent, which the tier
// order forbids. That asymmetry is the spine's -- nodeTransfersOut records the
// same thing one document over -- and it is fisc-ko1j.12.10.
//
// THE FOLD IS EXACT AND IS THE REASON transfers/in IS HERE AT ALL. Every payer
// node is parented to it, and the payer legs sum to $21,525,997 in FY2025-26
// and $21,624,633 in FY2026-27, which are p76's own printed grand totals and
// the spine's transfers/in to the cent. The node is also what the client opens:
// the reader clicks transfers/in on sankey.json, and filterFromNode asks this
// document for the subtree of the id they clicked.
//
// A PRINTED DASH IS A FACT AND NOT A FLOW, fundFlows' rule at the same place.
// Nine of p76's 22 rows print a dash in both budget columns; they draw no
// ribbon and survive in facts_uncited.
type transfersByFund struct {
	// Labels supplies the city's words for a fund number. It is optional, as
	// Sankey's is: a nil registry degrades to a slug-derived label.
	Labels labels
}

var (
	_ Projection = (*transfersByFund)(nil)
	_ Sliced     = (*transfersByFund)(nil)
)

// Name is [Projection]'s, and it is this document's file stem.
func (*transfersByFund) Name() string { return TransfersByFundProjection }

// Slices is one Options per column the schedule carries, [Sankey.Slices]'s
// rule: a cross-tab of two budget years adds every movement to its successor.
//
// THE CORPUS CARRIES TWO COLUMNS AND p76 PRINTS FOUR, and the two that are
// missing are missing at the MAPPING rather than here. p76's FY2023-24 and
// FY2024-25 columns miss its own printed grand total by $6,858,051 and by
// exactly $5,000,000 -- millions, nothing like the <=$5 rounding class
// stated_total_deltas is for -- so mappings/ reads and skips them and no fact
// carries them. This therefore draws the schedule exhaustively over what the
// store holds, which is what retires unprojectedScopes' entry for it rather
// than leaving it half true.
func (*transfersByFund) Slices(facts []fact.Fact, version string) []Options {
	seen := map[Column]bool{}
	for i := range facts {
		if facts[i].Scope == TransfersByFundScope {
			seen[Column{FiscalYear: facts[i].FiscalYear, Basis: facts[i].Basis}] = true
		}
	}
	cols := make([]Column, 0, len(seen))
	for c := range seen {
		cols = append(cols, c)
	}
	sortColumns(cols)
	out := make([]Options, 0, len(cols))
	for _, c := range cols {
		out = append(out, Options{
			Columns: []Column{c}, Scopes: TransfersByFundScopes(), Version: version,
		})
	}
	return out
}

// Build is [Projection]'s entry point.
func (t *transfersByFund) Build(facts []fact.Fact, o Options) ([]byte, error) {
	doc, err := t.Document(facts, o)
	if err != nil {
		return nil, err
	}
	return encode(doc, t.Name())
}

// Document builds the transfer network and returns it, so `fisc verify` reads
// the same structure `fisc export` writes rather than re-parsing the JSON.
func (t *transfersByFund) Document(facts []fact.Fact, o Options) (*TransfersByFundDocument, error) {
	if err := o.validate(); err != nil {
		return nil, fmt.Errorf("transfers-by-fund options: %w", err)
	}
	if !sameScopes(o.Scopes, TransfersByFundScopes()) {
		return nil, cmdutil.WithHint(
			fmt.Errorf("transfers-by-fund: scopes are %q, want %q", o.ScopeList(),
				Options{Scopes: TransfersByFundScopes()}.ScopeList()),
			"p76 restates money pp.66-67 and pp.127-140 already publish, so a document "+
				"holding this schedule beside either of them doubles the city's transfers")
	}
	if len(o.Columns) != 1 {
		return nil, cmdutil.WithHint(
			fmt.Errorf("transfers-by-fund: a transfer network is of one column, got %d",
				len(o.Columns)),
			"two budget years in one network add every movement to its own successor")
	}
	col := o.Columns[0]
	selected := selectFacts(facts, o)

	rows, err := pairTransferLegs(selected)
	if err != nil {
		return nil, err
	}

	nodes := map[string]Node{}
	links := make([]Link, 0, 2*len(rows))
	cited := map[string]bool{}
	zero := map[string]bool{}
	transfers := 0
	for _, k := range sortedTransferKeys(rows) {
		r := rows[k]
		if r.in.AmountCents == 0 {
			// A printed dash is a fact and is not a flow.
			zero[r.in.ID] = true
			zero[r.out.ID] = true
			continue
		}
		payer, receiver, err := r.endpoints()
		if err != nil {
			return nil, err
		}
		transfers++
		t.addNode(nodes, transfersInEndpoint())
		t.addNode(nodes, payer.from)
		t.addNode(nodes, payer.fund)
		t.addNode(nodes, receiver.fund)
		t.addNode(nodes, receiver.to)
		cited[r.in.ID] = true
		cited[r.out.ID] = true
		id := transferID(k)
		// THE RECEIVING LEG CITES THE RECEIVING FACT, whose own fund IS this
		// link's target. The payer at the other end is read off the counterpart
		// rather than off this fact, and the citation does not lose it: the two
		// legs share a (doc_id, page, offset), so both ends of the ribbon
		// resolve to the one printed figure this link's locator names.
		links = append(links, Link{
			Source: payer.from.id, Target: receiver.fund.id, ValueCents: r.in.AmountCents,
			Kind: KindInternalTransfer, TransferID: id, FactIDs: []string{r.in.ID},
			Locators: sourcesOf([]fact.Fact{r.in}),
		})
		links = append(links, Link{
			Source: payer.fund.id, Target: receiver.to.id, ValueCents: r.out.AmountCents,
			Kind: KindInternalTransfer, TransferID: id, FactIDs: []string{r.out.ID},
			Locators: sourcesOf([]fact.Fact{r.out}),
		})
	}

	sortLinks(links)
	if err := checkDistinctLinks(links); err != nil {
		return nil, err
	}
	out := sortedNodes(nodes)

	// EVERY UNCITED FACT MUST BE A PRINTED ZERO, refused here rather than
	// asserted downstream, for the reason fundFlows and departmentSpending each
	// give at the same place: a fact that reached no link for any other reason
	// is a transfer this document dropped, and a document publishing the
	// identity while quietly failing it is worse than one publishing no counts.
	uncited := 0
	for i := range selected {
		id := selected[i].ID
		if cited[id] {
			continue
		}
		uncited++
		if !zero[id] {
			return nil, cmdutil.WithHint(
				fmt.Errorf("transfers-by-fund: fact %s is carried by no link and is not a "+
					"printed zero", id),
				"every leg of a printed figure is its own link here, so a leg reaching no "+
					"link is one end of a movement dropped in silence")
		}
	}

	cavs := transfersByFundCaveats()
	if err := validateCaveats(cavs, nodeIDs(out)); err != nil {
		return nil, fmt.Errorf("%s: %w", col, err)
	}

	return &TransfersByFundDocument{
		SchemaVersion: SchemaVersion,
		Projection:    t.Name(),
		Metadata: transfersByFundMetadata{
			Envelope: Envelope{
				GeneratedBy: o.Version,
				Scope:       TransfersByFundScope,
				Currency:    "USD",
				Units:       "cents",
			},
			FiscalYear:      col.FiscalYear,
			FiscalYearLabel: fiscalYearLabel(col.FiscalYear),
			Basis:           string(col.Basis),
			Sources:         sourcesOf(selected),
			Counts: transfersByFundCounts{
				Facts:        len(selected),
				FactsCited:   len(cited),
				FactsUncited: uncited,
				Transfers:    transfers,
				Nodes:        len(out),
				Links:        len(links),
			},
			Caveats: cavs,
		},
		Nodes: out,
		Links: links,
	}, nil
}

// transferKey addresses one printed figure of p76: the page and the offset the
// token was read at.
//
// IT IS THE PAIRING, AND IT IS CONTENT-INDEPENDENT. fisc-4rh settled that the
// two legs of a movement cite the same doc_id, page and offset by construction,
// because one printed figure is the evidence for both directions. Keying on the
// row LABEL could not do this: three of p76's rows omit their payer because it
// continues from the row above, and p67 elsewhere in this corpus prints four
// byte-identical all-dash lines -- identity has to come from position.
type transferKey struct {
	docID  string
	page   int
	offset int
}

// transferID is [Link.TransferID]: the printed figure both legs were read from,
// spelled so a reader can find it.
//
// IT IS NOT A HASH. A fact id is hashed because it has to survive a rule being
// renamed; this names a place in a document, and a place is worth being legible
// -- a reader holding `livermore-budget-fy2026-2027/p76/3136` can open p0076.txt
// and seek to the character the two legs were read from.
func transferID(k transferKey) string {
	return fmt.Sprintf("%s/p%d/%d", k.docID, k.page, k.offset)
}

// transferRow is one printed figure read in both directions.
type transferRow struct {
	in  fact.Fact
	out fact.Fact
}

// transferEnds are the two nodes one leg runs between.
type transferEnds struct {
	// from is the payer's end of the receiving leg, tier 2.
	from endpoint
	// fund is the payer's or the receiver's own fund node, tier 3.
	fund endpoint
	// to is the receiver's end of the paying leg, tier 5.
	to endpoint
}

// endpoints resolves the four nodes a movement touches, refusing a leg that
// names no fund.
//
// A FUND OF 0 IS REFUSED RATHER THAN DEFAULTED, and it is reachable rather than
// hypothetical: p76's LAVWMA row publishes a receiving leg carrying fund 0,
// because the row's destination is not a single fund of data/funds.yaml. It
// prints a dash in both budget columns, so it never reaches here -- the caller
// drops a zero row first -- and the day it prints a figure this refuses to draw
// a movement into a fund it cannot name, rather than coining `fund/0`, which
// node-tiers-are-declared refuses in as many words.
func (r transferRow) endpoints() (transferEnds, transferEnds, error) {
	payer, err := transferFundEnds(&r.out)
	if err != nil {
		return transferEnds{}, transferEnds{}, err
	}
	receiver, err := transferFundEnds(&r.in)
	if err != nil {
		return transferEnds{}, transferEnds{}, err
	}
	return payer, receiver, nil
}

// transferFundEnds is the three id forms one leg's fund takes.
func transferFundEnds(fa *fact.Fact) (transferEnds, error) {
	if fa.Fund == nil {
		return transferEnds{}, cmdutil.WithHint(
			fmt.Errorf("transfers-by-fund: fact %s (%s p%d %q) is a %s leg naming no fund",
				fa.ID, fa.DocID, fa.Page, fa.RowLabel, fa.Kind),
			"both ends of a movement are fund nodes here, and a leg naming none has no "+
				"node to be")
	}
	n := strconv.Itoa(*fa.Fund)
	return transferEnds{
		from: endpoint{id: prefixTransferFrom + n, tier: tierFundGroup, role: roleTransferSource,
			parent: nodeTransfersIn},
		fund: endpoint{id: prefixFund + n, tier: tierFund, role: transferFundRole(*fa.Fund)},
		to:   endpoint{id: prefixTransferTo + n, tier: tierObjectCategory, role: roleTransferSink},
	}, nil
}

// transferFundRole is fundFlows' rule, for its reason: fund 100 is the General
// Fund whatever any column holds, and a reader of the published node learns it
// from the role rather than from the number.
func transferFundRole(number int) string {
	if number == generalFund {
		return roleGeneralFund
	}
	return roleFund
}

// transfersInEndpoint is the spine's own flow endpoint, at the spine's own id.
//
// THE ID IS THE SPINE'S DELIBERATELY, departmentSpending's rule for its tier-5
// nodes: a reader opens this document by clicking transfers/in on sankey.json,
// and a second id form for the same endpoint would make the thing they clicked
// a different box wearing the same words.
func transfersInEndpoint() endpoint {
	return endpoint{id: nodeTransfersIn, slug: nodeTransfersIn,
		tier: tierRevenueSource, role: roleTransferIn}
}

// pairTransferLegs groups the selected facts into printed figures, refusing any
// shape that is not exactly one receiving leg and one paying leg.
//
// EVERY GUARD IS A REFUSAL AND NOT A SKIP, this package's rule. A group that is
// not a pair is a mapping defect -- a counterpart that was not declared, a rule
// writing two rows at one offset -- and drawing what is left would publish half
// a movement with no error anywhere. The equal-amounts guard is the one that
// matters most: transfer-legs-pair asserts the same thing over the LINKS, and a
// projection that let an unequal pair through would be handing that check a
// finding it should never have had to make.
func pairTransferLegs(facts []fact.Fact) (map[transferKey]transferRow, error) {
	legs := map[transferKey][]fact.Fact{}
	var order []transferKey
	for i := range facts {
		fa := facts[i]
		if fa.Scope != TransfersByFundScope {
			return nil, fmt.Errorf("transfers-by-fund: fact %s is in scope %q, which this "+
				"document does not select", fa.ID, fa.Scope)
		}
		k := transferKey{docID: fa.DocID, page: fa.Page, offset: fa.Offset}
		if len(legs[k]) == 0 {
			order = append(order, k)
		}
		legs[k] = append(legs[k], fa)
	}

	out := make(map[transferKey]transferRow, len(order))
	for _, k := range order {
		pair := legs[k]
		if len(pair) != 2 {
			return nil, cmdutil.WithHint(
				fmt.Errorf("transfers-by-fund: %s cites %d facts, want 2",
					transferID(k), len(pair)),
				"p76 prints one figure per movement and the store carries a leg for each "+
					"end of it, so a figure with any other number of legs is a counterpart "+
					"that was not declared or a rule writing two rows at one offset")
		}
		var row transferRow
		var in, outs int
		for _, fa := range pair {
			switch fa.Kind {
			case mapping.KindTransferIn:
				in++
				row.in = fa
			case mapping.KindTransferOut:
				outs++
				row.out = fa
			default:
				return nil, cmdutil.WithHint(
					fmt.Errorf("transfers-by-fund: fact %s at %s is kind %q",
						fa.ID, transferID(k), fa.Kind),
					"this schedule prints transfers and nothing else; a third kind means "+
						"the rules changed under this projection")
			}
		}
		if in != 1 || outs != 1 {
			return nil, cmdutil.WithHint(
				fmt.Errorf("transfers-by-fund: %s has %d receiving and %d paying leg(s), "+
					"want one of each", transferID(k), in, outs),
				"a movement is drawn from both ends, so two legs in the same direction "+
					"would draw one end twice and the other not at all")
		}
		if row.in.AmountCents != row.out.AmountCents {
			return nil, cmdutil.WithHint(
				fmt.Errorf("transfers-by-fund: %s has legs of %d and %d cents",
					transferID(k), row.in.AmountCents, row.out.AmountCents),
				"one printed figure is the evidence for both directions, so two legs "+
					"citing the same token cannot differ in value")
		}
		out[k] = row
	}
	return out, nil
}

// sortedTransferKeys is a total order over the printed figures, so node
// creation does not depend on map iteration order. Links are re-sorted
// afterwards, but a node's first touch is decided here.
func sortedTransferKeys(m map[transferKey]transferRow) []transferKey {
	out := make([]transferKey, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		switch {
		case a.docID != b.docID:
			return a.docID < b.docID
		case a.page != b.page:
			return a.page < b.page
		default:
			return a.offset < b.offset
		}
	})
	return out
}

// addNode records a node the first time a link touches it.
func (t *transfersByFund) addNode(nodes map[string]Node, e endpoint) {
	if _, ok := nodes[e.id]; ok {
		return
	}
	nodes[e.id] = Node{ID: e.id, Label: t.label(e), Tier: e.tier, Role: e.role, Parent: e.parent}
}

// label resolves a node's words: a built-in first, then the registry, then a
// readable transform of the id -- Sankey.label's order, for its reasons.
//
// THE THREE FUND-BEARING FORMS TAKE THE SAME WORDS, and that is the document
// rather than an omission: `transfer-from/620` and `fund/620` are both the
// Wastewater Fund, drawn in two columns because a flow diagram cannot draw a
// fund paying itself into the same box. The caveat below says so to a reader
// who meets one fund twice on one chart; inventing "(paying)" for it would put
// words on a node the city did not print.
func (t *transfersByFund) label(e endpoint) string {
	if l, ok := builtinLabels[e.id]; ok {
		return l
	}
	if t.Labels != nil {
		if n, ok := transferFundNumber(e.id); ok {
			if name, ok := t.Labels.FundName(n); ok && name != "" {
				return name
			}
		}
		if e.slug != "" {
			if l, ok := t.Labels.Label(e.slug); ok && l != "" {
				return l
			}
		}
	}
	return slugLabel(e.id)
}

// transferFundNumber is the fund a node id names, for the three forms that name
// one. It returns false for transfers/in, which names no fund.
func transferFundNumber(id string) (int, bool) {
	for _, prefix := range []string{prefixFund, prefixTransferFrom, prefixTransferTo} {
		if len(id) > len(prefix) && id[:len(prefix)] == prefix {
			n, err := strconv.Atoi(id[len(prefix):])
			if err != nil {
				return 0, false
			}
			return n, true
		}
	}
	return 0, false
}

// transfersByFundCaveats are the things a reader of this file has to be told.
//
// NONE NAMES A NODE, departmentSpending's rule at the same place: each is a
// statement about the SCHEDULE, so marking particular marks would mark every
// one of them, which marks none.
func transfersByFundCaveats() []Caveat {
	return []Caveat{
		{
			ID: "one-figure-is-two-ribbons",
			Summary: "Every printed transfer is drawn twice, once from each end; summing " +
				"every ribbon here comes to twice what the city transfers.",
			Text: "Budget Book p76 names both ends of every movement it prints, and this " +
				"document publishes both: a receiving leg from the payer's end into the " +
				"fund that gets the money, and a paying leg out of the fund that sends it. " +
				"The two carry the same transfer_id and the same value, because one " +
				"printed figure is the evidence for both directions. So metadata.counts " +
				"states the number of MOVEMENTS as well as the number of links, and a " +
				"reader adding every value_cents in this file gets twice p76's grand " +
				"total: $21,525,997 in FY 2025-26 and $21,624,633 in FY 2026-27.",
			AppliesTo: []string{},
		},
		{
			ID: "a-fund-is-drawn-once-per-end",
			Summary: "A fund that both pays and receives appears in more than one column, " +
				"under the same name.",
			Text: "This schedule is money moving between the city's own funds, and a flow " +
				"diagram cannot draw a box pointing at itself. So a fund's paying end and " +
				"its receiving end are separate marks: the General Fund receives four " +
				"transfers and pays five in both budget years, and is drawn once for each " +
				"role, with the words the city prints for it either way. Two marks with " +
				"one name on this chart are one fund seen from two ends, not two funds.",
			AppliesTo: []string{},
		},
		{
			ID: "only-the-budget-columns-are-published",
			Summary: "p76 prints four columns and this draws the two adopted ones; its " +
				"historical columns do not add up to its own printed total.",
			Text: "The page's FY2023-24 and FY2024-25 columns miss p76's own printed grand " +
				"total by $6,858,051 and by exactly $5,000,000 -- millions, and nothing " +
				"like the rounding of a few dollars this corpus declares elsewhere. " +
				"Whatever those two columns are, they are not the city's rounding, so the " +
				"rules read and skip them and no figure from them is published anywhere " +
				"in this project. Budget Book pp.66-67 print no actual and no revised " +
				"column either, so there would be nothing to reconcile them against.",
			AppliesTo: []string{},
		},
		{
			ID: "the-paying-side-is-not-the-whole-of-transfers-out",
			Summary: "p76's grand total is the transfers-IN side; the city's transfers out " +
				"are larger, and the difference is a column this page does not list.",
			Text: "Each receiving section of p76 equals the matching TRANSFER IN cell on " +
				"Budget Book pp.66-67 to the cent, so the paying legs drawn here sum to " +
				"the transfers-in total and not to the city's transfers out. What pp.66-67 " +
				"fold into TRANSFER OUT and p76 does not list is the transfers each fund " +
				"makes to the Capital Improvement Program, which pp.72-75 print under a " +
				"heading of their own: $38,086,737 in FY 2025-26 and $50,762,251 in " +
				"FY 2026-27. Do not read a fund's paying leg here as the whole of what it " +
				"transfers out.",
			AppliesTo: []string{},
		},
	}
}
