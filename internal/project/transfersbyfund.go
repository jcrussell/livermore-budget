package project

import (
	"fmt"
	"slices"
	"sort"
	"strconv"

	"github.com/jcrussell/livermore-budget/internal/amount"
	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/hint"
	"github.com/jcrussell/livermore-budget/internal/structure"
	"github.com/jcrussell/livermore-budget/internal/vocab"
	"github.com/jcrussell/livermore-budget/schema"
)

// TransfersByFundProjection is this document's name and file stem.
const TransfersByFundProjection = "transfers-by-fund"

// TransfersByFundScope is Budget Book p76, Summary of Transfers. It is its own
// scope because pp.127-140 print the same transfers in from the receiving end,
// so a document holding both would double them.
const TransfersByFundScope = structure.ScopeTransfersByFund

// TransfersByFundScopes is the schedule set, as [Options.Scopes] holds it.
func TransfersByFundScopes() []string { return []string{TransfersByFundScope} }

// TransfersOutProjection is the network of every transfer out the city makes,
// and its file stem.
const TransfersOutProjection = "transfers-out"

// CIPFundingScope is Budget Book p222, the CIP's funding sources.
const CIPFundingScope = structure.ScopeCIPFundingSources

// TransfersOutScopes is p76 and p222 together: pp.66-67's TRANSFER OUT is what
// p76 lists plus what p222 lists going to the CIP, per fund group
// (structure.BudgetBookSplits).
func TransfersOutScopes() []string { return []string{TransfersByFundScope, CIPFundingScope} }

// transferKinds is the transfers-out network's kind set: p222 also prints the
// CIP funds' grants and a balance draw, which are not transfers.
var transferKinds = []vocab.Kind{vocab.KindTransferIn, vocab.KindTransferOut}

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
// ONE PRINTED FIGURE IS TWO LINKS. The store carries a receiving and a paying
// leg per figure at the same doc_id/page/offset (fisc-4rh); one ribbon citing
// both would publish twice the printed value, so each leg is its own link and
// the two share a [Link.TransferID].
//
// The payer is `transfer-from/<n>` at tier 2 because fund/<a> -> fund/<b> is
// tier 3 to tier 3, which node-tiers-are-declared and d3-sankey both refuse.
// The paying legs (`transfer-to/<n>`) are drawn by the transfers-out network
// below, which holds them beside p222's.
//
// Every payer is parented to transfers/in, whose fold equals p76's printed
// grand total. A printed dash draws no ribbon and stays in facts_uncited.
//
// # Transfers out
//
// With Out set it is [TransfersOutProjection]: p76's legs and p222's
// transfers to the CIP, the network pp.66-67's TRANSFER OUT totals. Every
// receiver's end is parented to transfers/out, which touches no link, so the
// spine's Transfers Out opens into the paying legs; nothing is parented to
// transfers/in, which p222's receipts at the CIP funds are not part of.
type transfersByFund struct {
	// Labels is optional: a nil registry degrades to a slug-derived label.
	Labels labels
	Out    bool
}

var (
	_ Projection = (*transfersByFund)(nil)
	_ Sliced     = (*transfersByFund)(nil)
)

// Name is [Projection]'s, and it is this document's file stem.
func (t *transfersByFund) Name() string {
	if t.Out {
		return TransfersOutProjection
	}
	return TransfersByFundProjection
}

// scopes is the schedule set this network is of.
func (t *transfersByFund) scopes() []string {
	if t.Out {
		return TransfersOutScopes()
	}
	return TransfersByFundScopes()
}

// kinds is the kind set this network selects, empty for every kind.
func (t *transfersByFund) kinds() []vocab.Kind {
	if t.Out {
		return transferKinds
	}
	return nil
}

// Slices is one Options per column every one of its schedules prints. p76
// prints four columns and the mapping skips the two historical ones, which miss
// the page's own grand total by millions; p222's FY2024-25 revised column has
// no p76 column beside it, so the transfers-out network has none either.
func (t *transfersByFund) Slices(facts []fact.Fact, version string) []Options {
	return columnsCarrying(facts, t.scopes(), t.scopes(), t.kinds(), version)
}

// Build is [Projection]'s entry point.
func (t *transfersByFund) Build(facts []fact.Fact, o Options) ([]byte, error) {
	doc, err := t.Document(facts, o)
	if err != nil {
		return nil, err
	}
	return encode(doc, t.Name(), schema.Projection)
}

// Document builds the transfer network and returns it, so `fisc verify` reads
// the same structure `fisc export` writes rather than re-parsing the JSON.
func (t *transfersByFund) Document(facts []fact.Fact, o Options) (*Document, error) {
	if err := o.validate(); err != nil {
		return nil, fmt.Errorf("transfers-by-fund options: %w", err)
	}
	if !sameScopes(o.Scopes, t.scopes()) || !slices.Equal(o.Kinds, t.kinds()) {
		return nil, hint.With(
			fmt.Errorf("%s: scopes are %q and kinds %v, want %q and %v", t.Name(), o.ScopeList(),
				o.Kinds, Options{Scopes: t.scopes()}.ScopeList(), t.kinds()),
			"p76 restates money pp.66-67 and pp.127-140 already publish, so a document "+
				"holding this schedule beside either of them doubles the city's transfers")
	}
	if len(o.Columns) != 1 {
		return nil, hint.With(
			fmt.Errorf("transfers-by-fund: a transfer network is of one column, got %d",
				len(o.Columns)),
			"two budget years in one network add every movement to its own successor")
	}
	col := o.Columns[0]
	selected, err := SelectFacts(facts, o)
	if err != nil {
		return nil, fmt.Errorf("transfers-by-fund: %w", err)
	}

	rows, err := pairTransferLegs(selected, o)
	if err != nil {
		return nil, err
	}

	nodes := map[string]Node{}
	links := make([]Link, 0, 2*len(rows))
	for _, k := range sortedTransferKeys(rows) {
		r := rows[k]
		if r.in.AmountCents == 0 {
			// A printed dash is a fact and is not a flow.
			continue
		}
		payer, receiver, err := r.endpoints()
		if err != nil {
			return nil, err
		}
		if t.Out {
			payer.from.parent = ""
			receiver.to.parent = NodeTransfersOut
			t.addNode(nodes, transfersOutEndpoint())
		} else {
			t.addNode(nodes, transfersInEndpoint())
		}
		t.addNode(nodes, payer.from)
		t.addNode(nodes, payer.fund)
		t.addNode(nodes, receiver.fund)
		t.addNode(nodes, receiver.to)
		id := transferID(k)
		// The receiving leg cites the receiving fact; its payer is read off
		// the counterpart, which shares the same printed figure.
		links = append(links, Link{
			Source: payer.from.id, Target: receiver.fund.id, ValueCents: r.in.AmountCents,
			Kind: KindInternalTransfer, TransferID: id, FactIDs: []string{r.in.ID},
			Locators: SourcesOf([]fact.Fact{r.in}),
		})
		links = append(links, Link{
			Source: payer.fund.id, Target: receiver.to.id, ValueCents: r.out.AmountCents,
			Kind: KindInternalTransfer, TransferID: id, FactIDs: []string{r.out.ID},
			Locators: SourcesOf([]fact.Fact{r.out}),
		})
	}

	sortLinks(links)
	if err := checkDistinctLinks(links); err != nil {
		return nil, err
	}
	out := sortedNodes(nodes)

	c, uncited := tally(selected, links, out)
	if err := refuseUncited(t.Name(), uncited, nil); err != nil {
		return nil, err
	}

	cavs := transfersByFundCaveats()
	if t.Out {
		cavs = transfersOutCaveats(rows)
	}
	if err := validateCaveats(cavs, nodeIDs(out)); err != nil {
		return nil, fmt.Errorf("%s: %w", col, err)
	}

	return &Document{
		SchemaVersion: SchemaVersion,
		Projection:    t.Name(),
		Metadata: Metadata{
			GeneratedBy:     o.Version,
			FiscalYear:      col.FiscalYear,
			FiscalYearLabel: fiscalYearLabel(col.FiscalYear),
			Basis:           string(col.Basis),
			Scopes:          t.scopes(),
			Currency:        "USD",
			Units:           "cents",
			Sources:         SourcesOf(selected),
			Counts:          c,
			Caveats:         cavs,
		},
		Nodes: out,
		Links: links,
	}, nil
}

// transferKey addresses one printed figure of p76 by position, which pairs its
// two legs. Keying on the row label could not: three of p76's rows omit their
// payer because it continues from the row above.
type transferKey struct {
	docID  string
	page   int
	offset int
}

// transferID is [Link.TransferID]: the printed figure both legs were read from,
// spelled legibly rather than hashed, e.g. `livermore-budget-fy2026-2027/p76/3136`.
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

// endpoints resolves the nodes a movement touches, refusing a leg that names no
// fund. p76's LAVWMA row carries no single fund; it prints dashes today, so the
// zero-row skip keeps it from reaching here.
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
		return transferEnds{}, hint.With(
			fmt.Errorf("transfers-by-fund: fact %s (%s p%d %q) is a %s leg naming no fund",
				fa.ID, fa.DocID, fa.Page, fa.RowLabel, fa.Kind),
			"both ends of a movement are fund nodes here, and a leg naming none has no "+
				"node to be")
	}
	n := strconv.Itoa(*fa.Fund)
	return transferEnds{
		from: endpoint{id: PrefixTransferFrom + n, role: RoleTransferSource,
			parent: NodeTransfersIn},
		fund: endpoint{id: PrefixFund + n, role: transferFundRole(*fa.Fund)},
		to:   endpoint{id: PrefixTransferTo + n, role: RoleTransferSink},
	}, nil
}

// transferFundRole marks fund 100 as the General Fund.
func transferFundRole(number int) string {
	if number == generalFund {
		return RoleGeneralFund
	}
	return RoleFund
}

// transfersOutEndpoint is the spine's transfers/out, at the spine's id: the
// node a reader clicks to open the transfers-out network, and the container
// its receivers' ends fold into.
func transfersOutEndpoint() endpoint {
	return endpoint{id: NodeTransfersOut, slug: NodeTransfersOut,
		role: RoleTransferOut}
}

// transfersInEndpoint is the spine's transfers/in, at the spine's id: it is the
// node a reader clicks to open this document.
func transfersInEndpoint() endpoint {
	return endpoint{id: NodeTransfersIn, slug: NodeTransfersIn,
		role: RoleTransferIn}
}

// pairTransferLegs groups the selected facts into printed figures, refusing any
// shape that is not exactly one receiving and one paying leg of equal amount.
func pairTransferLegs(facts []fact.Fact, o Options) (map[transferKey]transferRow, error) {
	legs := map[transferKey][]fact.Fact{}
	var order []transferKey
	for i := range facts {
		fa := facts[i]
		if !o.HasScope(fa.Scope) || !o.HasKind(fa.Kind) {
			return nil, fmt.Errorf("transfers-by-fund: fact %s is in scope %q and of kind %s, "+
				"which this document does not select", fa.ID, fa.Scope, fa.Kind)
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
			return nil, hint.With(
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
			case vocab.KindTransferIn:
				in++
				row.in = fa
			case vocab.KindTransferOut:
				outs++
				row.out = fa
			default:
				return nil, hint.With(
					fmt.Errorf("transfers-by-fund: fact %s at %s is kind %q",
						fa.ID, transferID(k), fa.Kind),
					"this schedule prints transfers and nothing else; a third kind means "+
						"the rules changed under this projection")
			}
		}
		if in != 1 || outs != 1 {
			return nil, hint.With(
				fmt.Errorf("transfers-by-fund: %s has %d receiving and %d paying leg(s), "+
					"want one of each", transferID(k), in, outs),
				"a movement is drawn from both ends, so two legs in the same direction "+
					"would draw one end twice and the other not at all")
		}
		if row.in.AmountCents != row.out.AmountCents {
			return nil, hint.With(
				fmt.Errorf("transfers-by-fund: %s has legs of %d and %d cents",
					transferID(k), row.in.AmountCents, row.out.AmountCents),
				"one printed figure is the evidence for both directions, so two legs "+
					"citing the same token cannot differ in value")
		}
		out[k] = row
	}
	return out, nil
}

// sortedTransferKeys is a total order over the printed figures, so a node's
// first touch does not depend on map iteration order.
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

// addNode records a node the first time a link touches it, and hangs the
// constraint tier on a fund's own node through annotateFund. The two ends of
// a movement are ends and not the fund, and carry none.
func (t *transfersByFund) addNode(nodes map[string]Node, e endpoint) {
	if _, ok := nodes[e.id]; ok {
		return
	}
	n := Node{ID: e.id, Label: t.label(e), Tier: e.tier(), Role: e.role, Parent: e.parent}
	if e.tier() == tierFund {
		if number, err := strconv.Atoi(e.id[len(PrefixFund):]); err == nil {
			annotateFund(&n, t.Labels, number)
		}
	}
	nodes[e.id] = n
}

// label is nodeLabel over this document's registry. The three fund-bearing
// forms take the same words; the a-fund-is-drawn-once-per-end caveat tells the
// reader why.
func (t *transfersByFund) label(e endpoint) string { return nodeLabel(t.Labels, e.id, e.slug) }

// transferFundNumber is the fund a node id names, for the three forms that name
// one. It returns false for transfers/in, which names no fund.
func transferFundNumber(id string) (int, bool) {
	for _, prefix := range []string{PrefixFund, PrefixTransferFrom, PrefixTransferTo} {
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
// Each is about the whole schedule, so none names a node; the first is the
// disclosure every document drawing a fund node carries.
func transfersByFundCaveats() []Caveat {
	return []Caveat{
		ConstraintTierCaveat(),
		{
			ID: "one-figure-is-two-ribbons",
			Summary: "Every printed transfer is drawn twice, once from each end; summing " +
				"every ribbon here comes to twice what the city transfers.",
			Text: "Budget Book p76 names both ends of every movement it prints, and this " +
				"document publishes both: a receiving leg from the payer's end into the " +
				"fund that gets the money, and a paying leg out of the fund that sends it. " +
				"The two carry the same transfer_id and the same value, because one " +
				"printed figure is the evidence for both directions. So the number of " +
				"MOVEMENTS is half metadata.counts.links, and a reader adding every " +
				"value_cents in this file gets twice p76's grand total: $21,525,997 in " +
				"FY 2025-26 and $21,624,633 in FY 2026-27.",
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
				"makes to the Capital Improvement Program, which Budget Book p222 lists " +
				"and the transfers-out document draws beside these. Do not read a fund's " +
				"paying leg here as the whole of what it transfers out.",
			AppliesTo: []string{},
		},
	}
}

// transfersOutCaveats are the transfers-out network's disclosures. Every figure
// in them is summed from the printed figures the network draws.
func transfersOutCaveats(rows map[transferKey]transferRow) []Caveat {
	var p76, cip int64
	for _, r := range rows {
		switch r.out.Scope {
		case TransfersByFundScope:
			p76 += r.out.AmountCents
		case CIPFundingScope:
			cip += r.out.AmountCents
		}
	}
	return []Caveat{
		ConstraintTierCaveat(),
		{
			ID: "one-figure-is-two-ribbons",
			Summary: "Every printed transfer is drawn twice, once from each end; summing " +
				"every ribbon here comes to twice what the city transfers out.",
			Text: fmt.Sprintf("Budget Book p76 and p222 name both ends of every movement they "+
				"print, and this document publishes both: a receiving leg into the fund that "+
				"gets the money and a paying leg out of the fund that sends it, under one "+
				"transfer_id and at one value. A reader adding every value_cents in this "+
				"file gets twice the %s the city transfers out.", amount.Cents(p76+cip).Dollars()),
			AppliesTo: []string{},
		},
		{
			ID: "transfers-out-are-two-schedules",
			Summary: "The city's transfers out are two printed lists: p76's transfers between " +
				"operating funds, and p222's transfers to the Capital Improvement Program.",
			Text: fmt.Sprintf("Budget Book p76 lists the transfers the operating funds make to "+
				"one another, %s here, and p222 lists what each operating fund transfers to a "+
				"Capital Improvement Program fund, %s. pp.66-67 print TRANSFER OUT as the two "+
				"together, and fisc verify holds that sum to the spine by fund group to the "+
				"cent. A fund that appears on both lists pays both.", amount.Cents(p76).Dollars(), amount.Cents(cip).Dollars()),
			AppliesTo: []string{},
		},
		{
			ID: "the-cip-funds-are-outside-the-operating-budget",
			Summary: "The Capital Improvement Program funds that receive p222's transfers are " +
				"not on the citywide chart; their receipts are not part of its Transfers In.",
			Text: "Budget Book p204 prints the Capital Improvement Program funds on a line of " +
				"their own, beside the operating budget, and pp.66-67 total the operating " +
				"budget alone. So money an operating fund transfers to a CIP fund leaves the " +
				"citywide chart as Transfers Out and the chart draws no CIP fund to receive it. " +
				"This document draws each CIP fund under the name data/funds.yaml gives it; " +
				"what the CIP funds spend it on is not drawn.",
			AppliesTo: []string{},
		},
		{
			ID: "a-fund-is-drawn-once-per-end",
			Summary: "A fund that both pays and receives appears in more than one column, " +
				"under the same name.",
			Text: "This is money moving between the city's own funds, and a flow diagram " +
				"cannot draw a box pointing at itself. So a fund's paying end and its " +
				"receiving end are separate marks, each with the words the city prints for " +
				"it. Two marks with one name on this chart are one fund seen from two ends, " +
				"not two funds.",
			AppliesTo: []string{},
		},
	}
}
