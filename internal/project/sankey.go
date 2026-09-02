package project

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/jcrussell/livermore-budget/internal/amount"
	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/mapping"
	"github.com/jcrussell/livermore-budget/pkg/cmdutil"
)

// LinkKind says what a link is, which is what decides whether it belongs in
// the headline. Getting this wrong is the error the whole projection exists to
// prevent: naively summing the expenditure column of Budget Book pp.66-67
// gives $313,708,146 where the city spends $254,095,412, a 23% inflation from
// counting every transfer twice.
type LinkKind string

// The kinds of link a Sankey carries.
const (
	// KindExternal is money crossing the city's boundary. The only kind in the
	// external headline figures.
	KindExternal LinkKind = "external"
	// KindInternalTransfer is money moving between the city's own funds.
	KindInternalTransfer LinkKind = "internal_transfer"
	// KindInternalService is an Internal Service Fund charge, billed by one
	// city department to another. Real money in a real fund, but counting it
	// as revenue AND as the paying department's expenditure double-counts it
	// — $18,969,834 in and $25,077,367 out in FY2026, 6.3% of the citywide
	// figure. Same class of error as the transfer double-count, one order of
	// magnitude smaller, which is why it gets its own kind rather than being
	// folded into external.
	KindInternalService LinkKind = "internal_service"
	// KindFundBalance is a draw on or contribution to accumulated balance, and
	// additions to reserves. Not external money, and not a transfer either,
	// because nothing moves between funds.
	KindFundBalance LinkKind = "fund_balance"
)

// Node tiers. The hierarchy has six levels; this schedule publishes two of
// them, and the flow endpoints below sit at the ends rather than inside it.
const (
	tierRevenueSource = 0
	// Tier 1 is UNUSED. It was the constraint tier and the layer cannot exist:
	// a constraint tier is a property of a fund and the fund groups do not
	// partition along it (data/funds.yaml has capital = 3 committed + 43
	// restricted-by-law). It rides as Node.ConstraintTier instead. The number
	// is left unassigned rather than renumbering, because tiers 2-5 are
	// published in node.tier and shifting them would change every document
	// already written. See docs/sankey-contract.md.
	tierFundGroup      = 2
	tierFund           = 3
	tierDepartment     = 4
	tierObjectCategory = 5
)

// Node roles, which say what a node is for without the client parsing its id.
const (
	roleRevenueSource           = "revenue_source"
	roleFundGroup               = "fund_group"
	roleFund                    = "fund"
	roleDepartment              = "department"
	roleObjectCategory          = "object_category"
	roleTransferIn              = "transfer_in"
	roleTransferOut             = "transfer_out"
	roleReserveIncrease         = "reserve_increase"
	roleFundBalanceDraw         = "fund_balance_draw"
	roleFundBalanceContribution = "fund_balance_contribution"
)

// Node id prefixes. A node id is a data/taxonomy.yaml or data/funds.yaml slug
// under the prefix that says which axis it is on, because the same word can be
// a category and a fund type: "debt-service" is the fund group and
// "debt-services" the object category, and the taxonomy is explicit that the
// near-miss is deliberate.
const (
	prefixRevenue     = "revenue/"
	prefixExpenditure = "expenditure/"
	prefixFundGroup   = "fund-group/"
	prefixFund        = "fund/"
	prefixDept        = "dept/"
	// prefixTransfers is the flow endpoints outside the hierarchy. Nothing is
	// parented to them; the prefix exists so transferEndpoints can recognise a
	// transfer node without a list of ids to keep in step.
	prefixTransfers = "transfers/"
)

// nodeTransfersIn is the flow endpoint every transfer arrives from. It sits
// OUTSIDE the hierarchy -- nothing is parented to it and it aggregates nothing --
// and carries tier 0 only so the diagram lays out left to right.
const nodeTransfersIn = "transfers/in"

// nodeTransfersOut is its mirror, and the two are not symmetric in use.
//
// nodeTransfersIn IS ON THE PRODUCTION PATH -- revenueEndpoint
// returns it for every KindTransferIn cell -- while nodeTransfersOut is read
// only by tests. That asymmetry is the document's, not an oversight: the
// drill-down has a revenue side and no transfers-out end to name.
//
// This constant was added to spell [Caveat.AppliesTo]'s two legs, and that is
// no longer what reads it: the field is built by transferEndpoints from the
// links a graph actually draws. What it is for now is letting a test name the
// endpoint it expects without retyping the string the taxonomy produces.
//
// (Two earlier versions of this comment were wrong in opposite directions --
// one said this constant was on the production path, the next said neither was.
// The second was written in the commit that fixed the first, which is the shape
// AGENTS.md's review-loop table collects.)
//
// builtinLabels below spells both keys as bare literals, and an earlier version
// of this comment defended that with two wrong facts -- that the table has
// eleven other rows and that they are literal. Measured: twelve rows, of which
// nine already key off a constant and three do not. So the table is mostly
// constants, and these two are part of the minority rather than the norm.
// Converting them is a tidy-up nothing here needs; it is not a decision this
// comment should keep pretending was made.
const nodeTransfersOut = "transfers/out"

// The slugs this projection has to recognize by name rather than by shape.
const (
	fundGroupInternalService = "internal-service"
	// The three fund-balance rows pp.66-67 print. Change is decomposed by
	// sign; beginning and ending are stocks and get no link at all.
	categoryFundBalanceChange    = "fund-balance/change"
	categoryFundBalanceBeginning = "fund-balance/beginning"
	categoryFundBalanceEnding    = "fund-balance/ending"
)

// NodeFundBalanceDraw and NodeFundBalanceContribution are the two nodes this
// projection infers rather than reads, exported so a check can name them. See
// derivedNodes for why they exist.
const (
	NodeFundBalanceDraw         = "fund-balance/draw"
	NodeFundBalanceContribution = "fund-balance/contribution"
)

// Labels is the view of the label registry (internal/registry, fisc-6ns) this
// projection needs, declared here in the consumer and kept to the one method
// actually used (byob-interfaces.2, as internal/mapping/resolve.go does with
// its doc interface).
//
// The key is a data/taxonomy.yaml category slug, never a node id, because a
// node id is prefixed by the axis it sits on and the taxonomy has no such
// prefixes. Nodes with no category behind them — the six fund groups, whose
// names are data/funds.yaml fund types, and the two nodes we inferred — are
// never looked up here at all, so the two vocabularies never share a key
// space. That matters more than it looks: "debt-service" is a fund type and
// "debt-services" an object category, and data/taxonomy.yaml is explicit that
// one string spanning two axes is the near-miss it exists to prevent.
type Labels interface {
	// Label returns the city's own words for a category slug, and whether the
	// registry knows the slug at all. A miss is not an error: an unlabelled
	// node falls back to a slug-derived label so a newly mapped category
	// renders as something readable instead of failing the build.
	Label(slug string) (string, bool)
	// FundName returns the city's own name for a fund number.
	//
	// It is here because a document keyed on FUNDS needs it and the Sankey does
	// not: the spine's finest fund axis is the fund GROUP, whose names come from
	// the pp.66-67 column headers and are built in. What needs it is the revenue
	// trends (docs/revenue-trends-contract.md), where a printed row label is not
	// unique across funds -- "Property Taxes" is printed by four of them and
	// "Use of Money & Prop" by thirty-eight -- so a series labelled by its row
	// alone is ambiguous on sight.
	//
	// A miss is not an error, as with Label: the number is shown instead.
	FundName(number int) (string, bool)
	// FundType is the fund type data/funds.yaml records, and whether the
	// registry knows the fund at all.
	//
	// IT IS THE PARENT EDGE OF THE TIER HIERARCHY. A tier-3 fund node's parent
	// is fund-group/<type>, and the type comes from HERE and never from the
	// fact's own fund_group: funds.yaml's `type:` is transcribed from the
	// appendix pp.253-257 while a fact's fund_group is read off the section
	// header its row sits under, and comparing two independent records is the
	// only version of that comparison that says anything.
	//
	// A miss IS an error to the caller, unlike Label and FundName: a node
	// parented to `fund-group/` is parented to nothing.
	FundType(number int) (string, bool)
	// ConstraintTier is how tightly a fund's money is tied down, and it is
	// DERIVED -- our reading of the Description of Funds narrative, pp.258-261.
	// A node publishing one must publish a source note and a rationale beside
	// it; see docs/sankey-contract.md's constraint_tier section.
	//
	// "" for a fund the registry does not list is a DIFFERENT answer from the
	// tier "unknown", which is a real classification meaning the document does
	// not establish a restriction. Do not collapse them.
	ConstraintTier(fund int) string
	// RestrictionNote is the sentence a constraint tier was read from, which is
	// what a node carrying one publishes as its rationale.
	RestrictionNote(fund int) string
	// DivisionLabel is the city's own words for a division slug, which is what
	// a fact's `department` field holds. A miss is not an error: the slug is
	// shown instead.
	DivisionLabel(slug string) (string, bool)
}

// derived carries the two fields fisc verify requires on anything we inferred.
type derived struct {
	rationale  string
	sourceNote string
}

// derivedNodes is the complete list of nodes this projection infers rather
// than reads off a page, and it has exactly two entries on purpose.
//
// The city prints CHANGE IN WORKING CAPITAL once per column, signed — the
// General Fund's FY2026 figure is "(1,034,154)". A Sankey cannot draw a
// negative link, so the sign is decomposed into two nodes. That decomposition
// is ours, not the city's, and the distinction between published and derived
// is the site's entire premise, so both nodes carry a rationale and a source
// note and fisc verify fails on a derived node that does not.
var derivedNodes = map[string]derived{
	NodeFundBalanceDraw: {
		rationale: "Our sign-decomposition of the published CHANGE IN WORKING CAPITAL row: " +
			"a negative change is a draw on accumulated balance. The city prints one signed " +
			"row; a Sankey cannot render a negative link.",
		sourceNote: "Budget Book PDF pp.66-67, CHANGE IN WORKING CAPITAL",
	},
	NodeFundBalanceContribution: {
		rationale: "Our sign-decomposition of the published CHANGE IN WORKING CAPITAL row: " +
			"a positive change is a contribution to accumulated balance.",
		sourceNote: "Budget Book PDF pp.66-67, CHANGE IN WORKING CAPITAL",
	},
}

// builtinLabels covers the nodes no category registry answers for.
//
// The six fund groups are here because data/funds.yaml binds a fund to a type
// and stops: it records no label for the type itself. These are the column
// headers Budget Book pp.66-67 print over the columns this projection reads,
// so they are the city's words rather than ours, and hard-coding them here
// beats leaving six of the diagram's central boxes labelled "General" and
// "Internal Service".
//
// The last two exist in no data file at all, because we inferred them.
//
// A registry label wins over any of these, which can only happen for a node
// that has a category slug.
var builtinLabels = map[string]string{
	prefixFundGroup + "general":          "General Fund",
	prefixFundGroup + "special-revenue":  "Special Revenue Funds",
	prefixFundGroup + "capital":          "Capital Funds",
	prefixFundGroup + "debt-service":     "Debt Service Funds",
	prefixFundGroup + "enterprise":       "Enterprise Funds",
	prefixFundGroup + "internal-service": "Internal Service Funds",
	prefixFundGroup + "permanent":        "Permanent Funds",

	NodeFundBalanceDraw:         "Fund Balance Draw",
	NodeFundBalanceContribution: "Fund Balance Contribution",

	// These three have taxonomy entries, and the entries agree with the words
	// below for the first two. They are repeated here so that a build with no
	// registry attached still labels the flow endpoints rather than rendering
	// the last segment of their ids as "In", "Out" and "Reserve Increase".
	//
	// The third disagrees: p66 prints ADDITION TO RESERVES over the figures
	// this projection reads, while data/taxonomy.yaml labels the category
	// "Reserve Increase / (Use)" — the p75 column header it was merged with.
	// testdata/sankey.golden.json shows p66's words; a build with a registry
	// attached shows the taxonomy's.
	"transfers/in":                  "Transfers In",
	"transfers/out":                 "Transfers Out",
	"fund-balance/reserve-increase": "Addition to Reserves",
}

// Graph is one sankey.json document.
//
// Field order is the JSON key order — encoding/json emits struct fields in
// declaration order — and that order is part of the frozen contract, not an
// accident of how this struct grew.
type Graph struct {
	SchemaVersion int      `json:"schema_version"`
	Projection    string   `json:"projection"`
	Metadata      Metadata `json:"metadata"`
	Nodes         []Node   `json:"nodes"`
	Links         []Link   `json:"links"`
}

// Metadata is everything a reader needs to know what slice of the budget the
// graph below it covers, and what it deliberately leaves out.
type Metadata struct {
	GeneratedBy string `json:"generated_by"`
	FiscalYear  int    `json:"fiscal_year"`
	// FiscalYearLabel is the city's own way of writing the year: FY2026 is
	// "FY 2025-26" on every page of the budget book, and a chart captioned
	// "2026" would not match anything the reader is holding.
	FiscalYearLabel string   `json:"fiscal_year_label"`
	Basis           string   `json:"basis"`
	Scope           string   `json:"scope"`
	Currency        string   `json:"currency"`
	Units           string   `json:"units"`
	Sources         []Source `json:"sources"`
	Headline        Headline `json:"headline"`
	Counts          Counts   `json:"counts"`
	// Caveats are the things this chart cannot show, in the chart's own file.
	// A caveat that lives only in a design document is a caveat nobody reads.
	Caveats []Caveat `json:"caveats"`
}

// Headline is the set of figures a reader quotes without reading the chart.
//
// Both a gross and an external number are published for revenue and for
// expenditure, under names that say which is which. An earlier draft called
// the gross figure external_revenue_cents; it is not external, because the
// scope is literally all-funds-gross and that figure includes internal service
// charges. Never name a key for a concept the code does not compute.
type Headline struct {
	AllFundsGrossRevenueCents     int64 `json:"all_funds_gross_revenue_cents"`
	AllFundsGrossExpenditureCents int64 `json:"all_funds_gross_expenditure_cents"`
	ExternalRevenueCents          int64 `json:"external_revenue_cents"`
	ExternalExpenditureCents      int64 `json:"external_expenditure_cents"`
	InternalTransferInCents       int64 `json:"internal_transfer_in_cents"`
	InternalTransferOutCents      int64 `json:"internal_transfer_out_cents"`
	// NaiveExpenditureCents is deliberately the wrong answer: expenditures
	// plus transfers out, which is what summing the printed expenditure column
	// gives. It ships so the page can show the error it is avoiding and so a
	// check can assert the real figure is not equal to it.
	NaiveExpenditureCents int64 `json:"naive_expenditure_cents"`
	// TransferResidualCents is transfers out minus transfers in. It is not
	// zero, and the caveats say why. A figure can go stale-red the day the
	// transfer schedule is mapped; a prose caveat cannot.
	TransferResidualCents int64 `json:"transfer_residual_cents"`
}

// Node is one box in the diagram.
type Node struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Tier  int    `json:"tier"`
	// Parent and ConstraintTier are "" on every node THE SPINE produces, and
	// that is correct rather than lazy: data/funds.yaml records a constraint
	// tier per fund, and pp.66-67 publish only fund groups, whose columns hold
	// funds of several tiers, so filling one in would present an editorial
	// classification as published data.
	//
	// THIS TYPE HAS MORE THAN ONE PRODUCER, so the sentence above is about the
	// schedule and not about the field. A document drawing funds populates both.
	// When it populates ConstraintTier it must also set SourceNote and
	// Rationale, because the node is published while the tier is our reading of
	// pp.258-261 — see docs/sankey-contract.md's constraint_tier section, which
	// carries the argument and the "" / "unknown" distinction.
	Parent         string `json:"parent"`
	ConstraintTier string `json:"constraint_tier"`
	Role           string `json:"role"`
	// Derived is true when we inferred the node and false when the city
	// printed it. It is stated on every node rather than left to be read off a
	// missing key, because the distinction is the site's whole premise.
	Derived    bool   `json:"derived"`
	Rationale  string `json:"rationale"`
	SourceNote string `json:"source_note"`
}

// Link is one flow.
type Link struct {
	Source     string   `json:"source"`
	Target     string   `json:"target"`
	ValueCents int64    `json:"value_cents"`
	Kind       LinkKind `json:"kind"`
	// TransferID pairs the two legs of one transfer. It is "" on every link
	// today -- NOT because the p76 transfer schedule is unmapped. p76 is mapped
	// and published at scope transfers-by-fund, and no projection selects that
	// scope, so its legs are in no graph. Populating
	// this needs a document of p76's own plus an id derived from the two legs'
	// shared (doc_id, page, offset): fisc-9gh, not fisc-5gk.3. Until then any
	// check of the form "every transfer_id has two equal legs" is vacuous and
	// must report itself as vacuous rather than as a pass.
	TransferID string `json:"transfer_id"`
	// FactIDs cite every fact this link sums, ascending. More than one means
	// contra rows netted into their parent category.
	FactIDs []string `json:"fact_ids"`
	// Locators cite the same facts BY PAGE: the (doc_id, page) pairs they were
	// read from, documents ascending, pages ascending within each, each page
	// once, never nil.
	//
	// IT IS NOT A REDUNDANT SPELLING OF FactIDs, and the difference is the
	// whole reason this field exists. fact.MakeID hashes rule_id, so splitting
	// or revising a rule moves every id on the pages it covers -- a citation
	// by id would 404 after a change that altered no figure. (doc_id, page)
	// cannot move, and the fact store's shard path is COMPUTED from it
	// (docs/fact-store-contract.md), so a client holding this can resolve the
	// records behind one mark in a single fetch with no lookup. Holding
	// FactIDs it can resolve nothing: it has no index and must not be given
	// one.
	//
	// Both are published because neither substitutes for the other. A page
	// holds facts from several rules, so a locator cannot say WHICH facts this
	// link summed -- which is what link-values-tie-to-facts, counts-reconcile
	// and citedFacts all need.
	//
	// This is a PAGE locator, not the (doc_id, page, offset) triple that
	// document names: offset addresses one printed figure and a link is an
	// aggregate.
	Locators []Source `json:"locators"`
	Derived  bool     `json:"derived"`
}

// Sankey projects the citywide spine as a flow diagram.
type Sankey struct {
	// Labels supplies the city's own words for a category slug. It is optional
	// and a field rather than a constructor argument because Registry hands
	// back projections before the composition root has loaded data/: a nil
	// Labels degrades to a readable slug rather than to no document.
	Labels Labels
}

var (
	_ Projection = (*Sankey)(nil)
	// Sliced is asserted here and not only relied on. It is consumed through a
	// runtime type assertion in both callers -- pkg/cmd/export's data.go and
	// internal/check's subject.go -- so a drift in Slices' signature would
	// compile clean, make both assertions return false, and silently fall back
	// to PublishedFiscalYears() x PublishedBasis. The spine would then stop
	// being built over the years it declares with no error anywhere.
	_ Sliced = (*Sankey)(nil)
)

// Name is the file stem: sankey.json.
func (*Sankey) Name() string { return "sankey" }

// Slices is one graph per (fiscal year, basis) the spine schedule carries.
//
// The years are read off the facts rather than hard-coded, so mapping a revised
// column or a third budget year puts that graph under verify's checks without
// anyone remembering to add it here.
//
// THAT SENTENCE WAS FALSE UNTIL fisc-rmx and is worth the note, because it is
// the kind of claim a reader acts on. This method keys on (fiscal year, BASIS)
// and the file stem was a function of the fiscal year ALONE, so mapping a
// revised column beside the adopted one declared two slices, computed one stem
// for both, and `fisc export` refused the entire run -- no file written,
// including the ones that were fine. [Stem] reads the whole column list now, so
// the two agree and the claim holds again.
//
// ONE SLICE PER YEAR, NOT ONE SLICE FOR ALL OF THEM, and that is the whole
// content of this method: every fiscal year the city publishes lives in the same
// facts.jsonl, and a graph built over two of them doubles every figure while
// still balancing perfectly. No internal consistency check can catch that,
// because two years of a balanced schedule are also balanced. See [Options].
//
// The scope is fixed rather than derived. It selects the SCHEDULE, and the
// schedule is what makes this projection a Sankey of the citywide spine rather
// than of whatever else the store happens to carry: a department-by-category
// page is a different scope and its rows would be added on top of the spine's.
func (*Sankey) Slices(facts []fact.Fact, version string) []Options {
	type key struct {
		year  int
		basis mapping.Basis
	}
	seen := map[key]bool{}
	for _, f := range facts {
		if f.Scope == PublishedScope {
			seen[key{f.FiscalYear, f.Basis}] = true
		}
	}
	keys := make([]key, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].year != keys[j].year {
			return keys[i].year < keys[j].year
		}
		return keys[i].basis < keys[j].basis
	})

	// ONE OPTIONS PER COLUMN, each carrying a single column. That is this
	// projection's answer to "what is a document of", and it is the opposite
	// of the trends projection's: a Sankey of two budgets is not a chart of
	// anything, so two columns are two documents, not one with two columns.
	out := make([]Options, 0, len(keys))
	for _, k := range keys {
		out = append(out, Options{
			Columns: []Column{{FiscalYear: k.year, Basis: k.basis}},
			Scopes:  []string{PublishedScope},
			Version: version,
		})
	}
	return out
}

// Build renders the graph as canonical JSON.
func (s *Sankey) Build(facts []fact.Fact, o Options) ([]byte, error) {
	g, err := s.Graph(facts, o)
	if err != nil {
		return nil, err
	}
	return encode(g, s.Name())
}

// cellKey addresses one printed cell of the schedule: a row's classification
// crossed with a column's fund group.
type cellKey struct {
	kind      mapping.Kind
	category  string
	fundGroup string
}

// cell is the netted value of one printed cell and the facts behind it.
type cell struct {
	cents   int64
	factIDs []string
	locs    locatorSet
}

// Graph builds the graph without encoding it, so fisc verify can check the
// structure without parsing back the JSON it is trying to validate.
//
// The switch below is the row-to-slug table of docs/sankey-contract.md written
// out as code, one case per printed row family, and it is kept in one function
// so that correspondence stays visible.
func (s *Sankey) Graph(facts []fact.Fact, o Options) (*Graph, error) {
	if err := o.Validate(); err != nil {
		return nil, fmt.Errorf("sankey options: %w", err)
	}
	// The scope selects the SCHEDULE, and refusing a foreign one is what stops
	// this projection publishing another schedule's rows under the spine's
	// contract. Trends.Document makes the same refusal in the same shape; the
	// asymmetry was that the one document the site publishes as its headline
	// was the one that did not make it, while the newer and smaller document
	// did. Unreachable through Slices, which pins PublishedScope -- but Graph
	// is exported and Options.Scopes' own doc comment says the field exists to
	// stop exactly the doubling this would produce.
	//
	// TWO REFUSALS, NOT ONE, and OnlyScope makes the first of them. "How many
	// schedules" and "which schedule" are different mistakes with different
	// remedies: a set of two here means someone pointed a single-grain document
	// at a drill-down's options, and a set of one that is not the spine means
	// they pointed it at the wrong schedule. Reporting either as the other
	// sends the reader to the wrong declaration.
	scope, err := o.OnlyScope()
	if err != nil {
		return nil, fmt.Errorf("sankey: %w", err)
	}
	if scope != PublishedScope {
		return nil, cmdutil.WithHint(
			fmt.Errorf("sankey: scope is %q, want %q", scope, PublishedScope),
			"this document is of one schedule; a projection built over another "+
				"schedule's facts would publish them under this one's contract")
	}
	// A Sankey is of ONE column, and the refusal is here rather than in
	// Validate because it is this projection's rule and not the type's: a
	// trends document over the same Options is of four. Two years of a
	// balanced schedule are also balanced, so nothing downstream would notice
	// -- every figure would simply be twice what the city printed.
	if len(o.Columns) != 1 {
		return nil, cmdutil.WithHint(
			fmt.Errorf("sankey: a graph is of one column, got %d (%s)",
				len(o.Columns), Describe(o.Columns)),
			"build one document per column; a Sankey of two budgets sums them and "+
				"still balances, which is why nothing below would catch it")
	}
	col := o.Columns[0]

	selected := selectFacts(facts, o)

	// Net first, link second. A contra row (Budget Book p127 prints ERAF as
	// "(15,857,875)") carries a negative amount under its parent's category,
	// so summing the cell before deciding anything about it is what "contra
	// nets into its parent" means in code. It also matters for
	// fund-balance/change, whose sign decides which way its link points: that
	// decision has to be made on the netted figure, not per fact.
	cells, err := netCells(selected)
	if err != nil {
		return nil, err
	}

	nodes := make(map[string]Node)
	links := make([]Link, 0, len(cells))
	var h Headline

	for _, k := range sortedCellKeys(cells) {
		c := cells[k]
		group := endpoint{id: prefixFundGroup + k.fundGroup, tier: tierFundGroup, role: roleFundGroup}

		var src, dst endpoint
		var kind LinkKind
		value := c.cents
		isDerived := false

		switch k.kind {
		case mapping.KindRevenue:
			src = endpoint{prefixRevenue + k.category, k.category, tierRevenueSource, roleRevenueSource}
			dst = group
			kind = boundaryKind(k.fundGroup)
			h.AllFundsGrossRevenueCents += value
			if kind == KindExternal {
				h.ExternalRevenueCents += value
			}

		case mapping.KindExpenditure:
			src = group
			dst = endpoint{prefixExpenditure + k.category, k.category, tierObjectCategory, roleObjectCategory}
			kind = boundaryKind(k.fundGroup)
			h.AllFundsGrossExpenditureCents += value
			if kind == KindExternal {
				h.ExternalExpenditureCents += value
			}

		case mapping.KindTransferIn:
			src = endpoint{k.category, k.category, tierRevenueSource, roleTransferIn}
			dst = group
			kind = KindInternalTransfer
			h.InternalTransferInCents += value

		case mapping.KindTransferOut:
			src = group
			dst = endpoint{k.category, k.category, tierObjectCategory, roleTransferOut}
			kind = KindInternalTransfer
			h.InternalTransferOutCents += value

		case mapping.KindFundBalance:
			switch k.category {
			case categoryFundBalanceBeginning, categoryFundBalanceEnding:
				// A stock, not a flow. The fact stays; the link never exists.
				continue
			case categoryFundBalanceChange:
				// The decomposition, and the only place this projection
				// infers rather than reads. Neither node carries the
				// fund-balance/change slug: they are not that category, they
				// are the two halves we split it into.
				isDerived = true
				kind = KindFundBalance
				if c.cents < 0 {
					src = endpoint{id: NodeFundBalanceDraw, tier: tierRevenueSource, role: roleFundBalanceDraw}
					dst = group
					value = -c.cents
				} else {
					src = group
					dst = endpoint{id: NodeFundBalanceContribution, tier: tierObjectCategory, role: roleFundBalanceContribution}
				}
			default:
				src = group
				dst = endpoint{k.category, k.category, tierObjectCategory, roleReserveIncrease}
				kind = KindFundBalance
			}

		default:
			return nil, fmt.Errorf("sankey: fact %s has unknown kind %q", c.factIDs[0], k.kind)
		}

		// Zero-valued links are dropped, and only the links. Most of the
		// revenue grid is zero — a category only the General Fund collects is
		// a dash in the other five columns — and d3-sankey draws zero-height
		// paths that churn node order. The facts still exist and are still
		// counted; a zero the city printed is a fact.
		if value == 0 {
			continue
		}

		s.addNode(nodes, src)
		s.addNode(nodes, dst)
		links = append(links, Link{
			Source:     src.id,
			Target:     dst.id,
			ValueCents: value,
			Kind:       kind,
			TransferID: "",
			FactIDs:    c.factIDs,
			Locators:   c.locs.sources(),
			Derived:    isDerived,
		})
	}

	h.NaiveExpenditureCents = h.AllFundsGrossExpenditureCents + h.InternalTransferOutCents
	h.TransferResidualCents = h.InternalTransferOutCents - h.InternalTransferInCents

	sortLinks(links)
	if err := checkDistinctLinks(links); err != nil {
		return nil, err
	}

	// THE CAVEATS ARE VALIDATED AGAINST THE NODES THIS DOCUMENT ACTUALLY DREW,
	// which is the only place that pairing is available. caveats() is
	// conditional on the graph -- the internal-service and contested-total
	// entries are emitted only when the links justify them -- so a caveat can
	// name a node a DIFFERENT column carries and this one does not, and that is
	// exactly the mistake AppliesTo makes silent.
	drawn := sortedNodes(nodes)
	cavs := caveats(h, col, links)
	if err := ValidateCaveats(cavs, nodeIDs(drawn)); err != nil {
		return nil, fmt.Errorf("%s: %w", col, err)
	}

	return &Graph{
		SchemaVersion: SchemaVersion,
		Projection:    s.Name(),
		Metadata: Metadata{
			GeneratedBy:     o.Version,
			FiscalYear:      col.FiscalYear,
			FiscalYearLabel: fiscalYearLabel(col.FiscalYear),
			Basis:           string(col.Basis),
			Scope:           scope,
			Currency:        "USD",
			Units:           "cents",
			Sources:         sourcesOf(selected),
			Headline:        h,
			Counts: Counts{
				Facts:      len(selected),
				FactsCited: citedFacts(links),
				Nodes:      len(nodes),
				Links:      len(links),
			},
			Caveats: cavs,
		},
		Nodes: drawn,
		Links: links,
	}, nil
}

// selectFacts keeps the facts this projection is of: the scope, and any one of
// the columns.
//
// Both selectors are applied. Filtering on the columns alone would let a
// department schedule's rows land on top of the citywide spine's, and filtering
// on the scope alone would fold every year the store carries into one document
// — both of which produce a graph that balances and is wrong.
//
// The column test is membership rather than equality because a document may be
// of several columns. What it is not is a wildcard: an Options with no columns
// selects NOTHING here, and [Options.Validate] refuses one before it can.
func selectFacts(facts []fact.Fact, o Options) []fact.Fact {
	out := make([]fact.Fact, 0, len(facts))
	for _, f := range facts {
		if !o.HasScope(f.Scope) {
			continue
		}
		if !slices.Contains(o.Columns, Column{FiscalYear: f.FiscalYear, Basis: f.Basis}) {
			continue
		}
		out = append(out, f)
	}
	return out
}

// netCells sums the facts of each printed cell and collects their ids.
func netCells(facts []fact.Fact) (map[cellKey]*cell, error) {
	cells := make(map[cellKey]*cell)
	for i := range facts {
		f := facts[i]
		if f.Category == "" {
			return nil, cmdutil.WithHint(
				fmt.Errorf("sankey: fact %s (%s p%d %q) has no category", f.ID, f.DocID, f.Page, f.RowLabel),
				"every node id in this projection is a data/taxonomy.yaml slug, so a "+
					"fact with no category has nowhere to go")
		}
		if f.Department != "" {
			return nil, cmdutil.WithHint(
				fmt.Errorf("sankey: fact %s (%s p%d %q) carries department %q",
					f.ID, f.DocID, f.Page, f.RowLabel, f.Department),
				"this projection has no department tier yet (fisc-gxa.2), and adding "+
					"department rows to the citywide spine's would double-count them; "+
					"check the scope the projection was asked for")
		}
		if f.FundGroup == "" {
			return nil, cmdutil.WithHint(
				fmt.Errorf("sankey: fact %s (%s p%d %q) has no fund group", f.ID, f.DocID, f.Page, f.RowLabel),
				"the spine is a fund-group graph: every flow has to start or end at one")
		}
		k := cellKey{kind: f.Kind, category: f.Category, fundGroup: f.FundGroup}
		c := cells[k]
		if c == nil {
			c = &cell{}
			cells[k] = c
		}
		// The amount is added as the document printed it. A contra row arrives
		// already negative and nothing here re-signs it; sign says how the row
		// relates to its category, it is not an instruction to negate.
		c.cents += f.AmountCents
		c.factIDs = append(c.factIDs, f.ID)
		c.locs.add(&facts[i])
	}
	for _, c := range cells {
		sort.Strings(c.factIDs)
	}
	return cells, nil
}

// sortedCellKeys puts cells in a total order so the build does not depend on
// map iteration. Links are sorted again at the end; this is what makes the
// intermediate steps — headline accumulation, node creation — deterministic
// too.
func sortedCellKeys(cells map[cellKey]*cell) []cellKey {
	keys := make([]cellKey, 0, len(cells))
	for k := range cells {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		a, b := keys[i], keys[j]
		switch {
		case a.kind != b.kind:
			return a.kind < b.kind
		case a.category != b.category:
			return a.category < b.category
		default:
			return a.fundGroup < b.fundGroup
		}
	})
	return keys
}

// boundaryKind classifies a revenue or expenditure link by the fund group it
// touches. An Internal Service Fund is inside the city, so its charges are not
// external money however much they look like revenue.
func boundaryKind(fundGroup string) LinkKind {
	if fundGroup == fundGroupInternalService {
		return KindInternalService
	}
	return KindExternal
}

// endpoint is one end of a link before it becomes a node: the id it will
// carry, the data/taxonomy.yaml category it stands for (empty when the
// taxonomy does not answer for it), and where it sits in the diagram.
type endpoint struct {
	id   string
	slug string
	tier int
	role string
}

// addNode records a node the first time a link touches it. Nodes exist because
// links do: an unreferenced node is a box with nothing flowing through it.
func (s *Sankey) addNode(nodes map[string]Node, e endpoint) {
	if _, ok := nodes[e.id]; ok {
		return
	}
	n := Node{ID: e.id, Label: s.label(e.id, e.slug), Tier: e.tier, Role: e.role}
	if d, ok := derivedNodes[e.id]; ok {
		n.Derived = true
		n.Rationale = d.rationale
		n.SourceNote = d.sourceNote
	}
	nodes[e.id] = n
}

// label resolves a node to the words a reader sees: the registry's if it has
// any for the node's category, this package's if the node is one of the fixed
// boxes in builtinLabels, and otherwise a readable transform of the id. The
// last case is a placeholder, not a translation — the point of the registry is
// that the site shows the city's words rather than ours.
func (s *Sankey) label(id, slug string) string {
	// builtinLabels wins over the registry, which is the opposite of what you
	// would expect from a curated data file and is deliberate.
	//
	// A taxonomy label names a CATEGORY, which may span several schedules; the
	// words on a node have to be the words on the page this projection read.
	// fund-balance/reserve-increase is the case: the taxonomy calls it "Reserve
	// Increase / (Use)", after the p75 column header the category was merged
	// with, while p66 — the page these figures come from — prints ADDITION TO
	// RESERVES. Letting the registry win would also mean the rendered site and
	// testdata/sankey.golden.json disagreed for any node listed both places,
	// which would quietly retire the golden file as a contract test.
	if l, ok := builtinLabels[id]; ok {
		return l
	}
	if s.Labels != nil && slug != "" {
		if l, ok := s.Labels.Label(slug); ok && l != "" {
			return l
		}
	}
	return slugLabel(id)
}

// slugLabel turns the last segment of a node id into title case.
func slugLabel(id string) string {
	seg := id
	if i := strings.LastIndex(seg, "/"); i >= 0 {
		seg = seg[i+1:]
	}
	words := strings.Split(strings.ReplaceAll(seg, "-", " "), " ")
	for i, w := range words {
		if w == "" {
			continue
		}
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(words, " ")
}

// sortedNodes orders nodes by (tier, id): tier so the diagram's columns come
// out left to right, id so two builds of the same facts agree.
func sortedNodes(nodes map[string]Node) []Node {
	out := make([]Node, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, n)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Tier != out[j].Tier {
			return out[i].Tier < out[j].Tier
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// sortLinks orders links by (source, target).
func sortLinks(links []Link) {
	sort.Slice(links, func(i, j int) bool {
		if links[i].Source != links[j].Source {
			return links[i].Source < links[j].Source
		}
		return links[i].Target < links[j].Target
	})
}

// checkDistinctLinks refuses two links between the same pair of nodes.
//
// d3-sankey draws them stacked and indistinguishable, and a reader hovering
// one would see a value that is not the flow between those two boxes. It can
// only happen if two cells collapse onto one pair, which means a classification
// is wrong upstream, so it is an error rather than a silent merge.
func checkDistinctLinks(links []Link) error {
	for i := 1; i < len(links); i++ {
		if links[i].Source == links[i-1].Source && links[i].Target == links[i-1].Target {
			return fmt.Errorf("sankey: two links from %q to %q (%s and %s)",
				links[i].Source, links[i].Target,
				strings.Join(links[i-1].FactIDs, ","), strings.Join(links[i].FactIDs, ","))
		}
	}
	return nil
}

// sourcesOf lists the documents and pages the facts were read from, so the
// citation on the page names pages rather than a document.
func sourcesOf(facts []fact.Fact) []Source {
	var l locatorSet
	for i := range facts {
		l.add(&facts[i])
	}
	return l.sources()
}

// fiscalYearLabel writes a fiscal year the way the budget book does: FY2026 is
// the year ending in 2026, printed "FY 2025-26".
func fiscalYearLabel(year int) string {
	return fmt.Sprintf("FY %d-%02d", year-1, year%100)
}

// Caveats that hold whatever the facts say. The first two are statements about
// what the model does, not about which numbers arrived, so neither is
// conditional; the third is emitted only by a document that draws such a link.
//
// THE TEXTS ARE UNCHANGED FROM WHEN THESE WERE BARE STRINGS. Only ids and
// summaries were added, so the diff a reader of the golden file has to check is
// "every text byte-identical, nothing else in the old field".
//
// AppliesTo IS EMPTY ON THE FIRST TWO AND NOT ON THE THIRD, and that is the
// distinction the field exists for. A stock that carries no link and a fund type
// with no column are properties of the SCHEDULE -- no node is responsible for
// either, and naming one would be inventing a culprit. Internal service charges
// are a property of one drawn node, so the chart can mark it.
var (
	caveatStocks = Caveat{
		ID:      "working-capital-is-a-stock",
		Summary: "Beginning and ending working capital are stocks, not flows, so the chart carries no link for them.",
		Text: "The Sankey shows flows only. BEGINNING and ENDING WORKING CAPITAL are " +
			"stocks and are recorded as facts but carry no link, so the chart will not " +
			"reconcile row-for-row against pp.66-67.",
		AppliesTo: []string{},
	}
	caveatPermanentFunds = Caveat{
		ID:      "permanent-funds-have-no-column",
		Summary: "Permanent Funds are a seventh fund type, and this schedule prints no column for them.",
		Text: "Permanent Funds are a seventh fund type in data/funds.yaml " +
			"(2 funds) with no column on this schedule.",
		AppliesTo: []string{},
	}
	caveatInternalService = Caveat{
		ID:      "internal-service-is-outside-the-external-headline",
		Summary: "Internal Service charges are billed between city departments, so they are outside the external headline.",
		Text: "Internal Service Fund charges are billed to other City " +
			"departments, so they are classified internal_service and excluded from the " +
			"external headline. The all_funds_gross figures include them and match the " +
			"city's own citywide totals.",
		AppliesTo: []string{prefixFundGroup + "internal-service"},
	}
)

// ContestedTotal is a fund group's total that the SPINE PAGE PRINTS and other
// schedules of the same document contradict, together with what they say
// instead.
//
// IT IS A DECLARATION, NOT A CORRECTION. This projection publishes what pp.66-67
// print, because that is where the spine's facts are read from and "published
// and derived are different things" is the invariant the whole project rests on.
// What a reader is owed is not our arithmetic but the knowledge that the city's
// own book disagrees with itself here.
//
// PrintedBy AND ImpliedBy ARE SEPARATE FIELDS AND THAT IS THE POINT. The first
// draft of the caveat said "six other schedules PRINT" a figure that four of
// them print and two of them imply -- p0061 prints that figure plus a transfer
// it also prints, and pp.85-125 sum to it across 78 rows. A derived figure cited
// as a printed one is the defect this project exists to refuse, and it had got
// into reader-facing text.
//
// IT RETIRES ITSELF. The caveat is emitted only when the graph actually draws
// Published; correct the fact and the condition stops matching and the sentence
// stops being printed. What that alone would NOT catch is the entry going dead
// while still sitting here, so TestContestedTotalsAreStillContested asserts over
// the committed corpus that every entry still describes what is drawn AND still
// agrees with internal/check's fundingSourcesExceptions, which verifies both
// figures against the corpus on every run. Between them, neither a stale caveat
// nor a stale declaration can survive.
type ContestedTotal struct {
	Column    Column
	FundGroup string
	// Published is the spine's figure for the group, in cents, and is what this
	// chart draws. Elsewhere is what the other schedules make it.
	Published, Elsewhere int64
	// SpinePages is where Published is printed, and Row names the single line
	// the difference sits in.
	SpinePages, Row string
	// PrintedBy names the schedules that PRINT Elsewhere. ImpliedBy names the
	// ones that only imply it, and says how -- a schedule that sums to a figure
	// has not printed it, and the sentence must not say it has.
	PrintedBy, ImpliedBy string
	// Bead is the work that owns the decision. A contested figure with no bead
	// is one nobody is deciding.
	Bead string
}

// contestedTotals is the whole list. One entry, and it should stay short: an
// entry here is a place the city's book contradicts itself that we publish
// anyway, and a long list would mean the corpus had stopped being reconcilable
// rather than that this mechanism had become useful.
var contestedTotals = []ContestedTotal{{
	Column:     Column{FiscalYear: 2027, Basis: mapping.BasisAdopted},
	FundGroup:  "internal-service",
	Published:  2654451500,
	Elsewhere:  2629451500,
	SpinePages: "pp.66-67",
	Row:        "Services & Supplies",
	PrintedBy:  "p0183, p0075, p0205 and p0209",
	ImpliedBy: "p0061 prints $26,906,515, which is that figure plus the $612,000 " +
		"transfer to the CIP it prints beside it, and the 78 rows of pp.85-125 sum to it",
	Bead: "fisc-av0w",
}}

// ContestedTotals is the declared list, exported so a test over the COMMITTED
// corpus can assert every entry still describes what that corpus draws.
//
// A fresh slice per call, as Registry does, so no caller can append to the
// published set. Exported for the same reason ConstraintTierCaveat is: an
// assertion should compare against THIS declaration rather than against a
// second copy of the same figures, because two copies agreeing is not the claim
// worth making.
func ContestedTotals() []ContestedTotal { return slices.Clone(contestedTotals) }

// GroupExpenditure is what a graph draws as one fund group's expenditure,
// exported alongside ContestedTotals because an assertion about an entry needs
// the same sum the caveat is conditional on -- computed once, here, rather than
// re-derived in a test where it could drift.
func GroupExpenditure(links []Link, fundGroup string) int64 {
	return groupExpenditure(links, fundGroup)
}

// caveats states what the chart cannot show, in the chart's own file.
//
// Three of them are conditional on the data because a caveat about internal
// service charges in a document that has none would be misdirection, the
// transfer caveat quotes figures it can only get from the graph, and a contested
// total that this document does not draw is not this document's problem.
func caveats(h Headline, col Column, links []Link) []Caveat {
	out := make([]Caveat, 0, 5)

	if h.InternalTransferInCents != 0 || h.InternalTransferOutCents != 0 {
		out = append(out, transferCaveat(h, col, links))
	}
	for _, l := range links {
		if l.Kind == KindInternalService {
			out = append(out, caveatInternalService)
			break
		}
	}
	for _, c := range contestedTotals {
		if cav, ok := contestedCaveat(c, col, links); ok {
			out = append(out, cav)
		}
	}
	out = append(out, caveatStocks, caveatPermanentFunds)
	return out
}

// groupExpenditure is what this graph draws as one fund group's expenditure:
// the links running from that group to an object category.
//
// It is summed from the LINKS rather than taken from the headline because the
// headline is a citywide figure and the claim here is about one group.
func groupExpenditure(links []Link, fundGroup string) int64 {
	var total int64
	source := prefixFundGroup + fundGroup
	for _, l := range links {
		if l.Source == source && strings.HasPrefix(l.Target, prefixExpenditure) {
			total += l.ValueCents
		}
	}
	return total
}

// transferEndpoints is the transfer nodes this graph actually draws, in the
// order the diagram reads: the source of every internal-transfer link, then the
// target. Ids rather than a hard-coded pair, because the caveat has to name
// what the document carries -- see transferCaveat.
//
// It returns [] and not nil for a document with no transfer links: the field is
// published with no omitempty, and nil would write JSON null where every other
// empty list writes [].
func transferEndpoints(links []Link) []string {
	seen := map[string]bool{}
	var in, out []string
	for _, l := range links {
		if l.Kind != KindInternalTransfer {
			continue
		}
		if strings.HasPrefix(l.Source, prefixTransfers) && !seen[l.Source] {
			seen[l.Source] = true
			in = append(in, l.Source)
		}
		if strings.HasPrefix(l.Target, prefixTransfers) && !seen[l.Target] {
			seen[l.Target] = true
			out = append(out, l.Target)
		}
	}
	slices.Sort(in)
	slices.Sort(out)
	return append(append(make([]string, 0, len(in)+len(out)), in...), out...)
}

// contestedCaveat is the caveat for one contested total, and false when this
// document does not draw it.
//
// THE VALUE IS RE-READ FROM THE GRAPH AND COMPARED, rather than the entry's
// column being trusted on its own. A caveat naming a figure the chart does not
// draw is worse than no caveat: it tells a reader to distrust a number that is
// not there, and it would go on saying so after the figure was corrected.
func contestedCaveat(c ContestedTotal, col Column, links []Link) (Caveat, bool) {
	if col != c.Column {
		return Caveat{}, false
	}
	if groupExpenditure(links, c.FundGroup) != c.Published {
		return Caveat{}, false
	}
	label := builtinLabels[prefixFundGroup+c.FundGroup]
	if label == "" {
		label = c.FundGroup
	}
	// THE ID EMBEDS THE FUND GROUP, and it is not decoration. contestedTotals
	// is a list; two entries for two groups in one column would otherwise both
	// publish the anchor "contested-total", ValidateCaveats would refuse the
	// build, and the shortest way out of that refusal is the wrong one --
	// dropping a caveat rather than naming it. One entry today; the list is
	// meant to stay short, not to stay length one.
	return Caveat{
		ID: "contested-total-" + c.FundGroup,
		Summary: fmt.Sprintf(
			"The city's own book prints two different figures for %s expenditure, %s apart; this chart draws the one on %s.",
			label, dollars(c.Published-c.Elsewhere), c.SpinePages),
		Text: fmt.Sprintf(
			"THE CITY'S OWN BOOK DISAGREES WITH ITSELF ABOUT THIS ONE FIGURE. %s "+
				"expenditure is drawn at %s, which is what %s print for %s. Other "+
				"schedules in the same document make it %s: %s print that figure, and "+
				"%s. The difference is %s, in a single %s row. This chart draws the "+
				"spine's figure because every figure here is one the city printed on the "+
				"page it is cited from, and substituting a number from elsewhere would "+
				"make this one an exception to that. Which figure the corpus should "+
				"publish is open (%s).",
			label, dollars(c.Published), c.SpinePages, col.String(), dollars(c.Elsewhere),
			c.PrintedBy, c.ImpliedBy, dollars(c.Published-c.Elsewhere), c.Row, c.Bead),
		AppliesTo: []string{prefixFundGroup + c.FundGroup},
	}, true
}

// transfersOutToCIP is the "Transfers Out to CIP" column the city prints on its
// sources-and-uses schedule, per spine column, read off the page.
//
// KEYED ON THE WHOLE COLUMN AND NOT ON THE YEAR, because pp.72-75 print ONE
// budget column each and no actual or revised figure. A revised spine column
// would find no entry here and fall back to the wording that claims nothing
// printed -- which is the point: naming the column for a basis the city prints
// no figure for would publish an identity that does not exist.
//
// EVERY FIGURE IS READ OFF A PAGE. p0073.txt:58 and p0075.txt:58, under the
// header at p0073.txt:9. They are hand-typed here in the same way, and for the
// same reason, as internal/check/transfersdetail.go's toCIP table: pp.72-75 are
// not fixtures, so nothing in this tree parses them. What IS machine-checked is
// that the figure below equals this document's own residual -- see
// transferCaveat, which declines to name the column when it does not.
var transfersOutToCIP = map[Column]struct {
	Cents amount.Cents
	// Page is the PDF page index, which is what the site's citations label
	// "PDF p" and deep-link with #page=N. The printed folio on that sheet reads
	// 69, four lower; a reader given a bare "p73" and a paper copy looks at the
	// wrong table.
	Page int
}{
	{FiscalYear: 2026, Basis: mapping.BasisAdopted}: {Cents: 3808673700, Page: 73},
	{FiscalYear: 2027, Basis: mapping.BasisAdopted}: {Cents: 5076225100, Page: 75},
}

// transferCaveat says the transfer legs do not pair up, and by how much.
//
// ITS TWO CLAIMS ARE KEPT APART ON PURPOSE, and neither is the one this
// function used to make. That no link carries a transfer_id is NOT caused by
// p76 being unmapped -- p76 has been mapped and published since ced45b4, and
// the legs are still unpaired, because its facts are at scope
// transfers-by-fund and this document is of all-funds-gross. The residual is
// not caused by it either, and mapping the page DEMONSTRATED that rather than
// predicting it: p76's grand total is the transfers-in side to the cent, so the
// city itemises every transfer received and none of the difference.
//
// THE RESIDUAL IS A PRINTED COLUMN, which is a stronger claim than the
// decomposition this comment used to point at. fisc-5gk.3's per-fund-group
// table was corrected in 19bb265 and must not be cited; what is published is
// the citywide figure, and only that. Splitting it by fund group is derived,
// because pp.72-75 print a to-CIP figure per major fund and one aggregate for
// every non-major one -- this project's own published-is-not-derived rule, made
// in full at internal/check's collapseNonMajor.
func transferCaveat(h Headline, col Column, links []Link) Caveat {
	// ONE ID OVER THREE TEXTS, deliberately. Which of the three sentences a
	// document gets is a fact about that document's own arithmetic -- whether
	// the legs balance, and whether the residual meets the printed to-CIP
	// column -- and not three different caveats. A reader following the anchor
	// wants "the transfer legs do not pair"; the paragraph they land on is the
	// one their document earned. What that costs is that a page listing more
	// than one document's caveats has to key on (id, document) rather than on
	// id alone, and cannot assume one text per id.
	const id = "transfer-legs-unpaired"
	const unpaired = "Transfer legs are unpaired: no link carries a transfer_id. Budget " +
		"Book p76's transfer schedule is mapped and published, but at scope " +
		"transfers-by-fund, and this document is of all-funds-gross -- so none of its " +
		"facts is in this graph. They cannot simply be added to it: transfers-by-fund " +
		"and revenue-by-fund both publish transfer_in over the same money, which the " +
		"projection-scopes-are-disjoint check refuses. Pairing the legs needs a document " +
		"of p76's own and a transfer_id derived from the two legs' shared page and " +
		"offset (fisc-9gh). "
	in, out := h.InternalTransferInCents, h.InternalTransferOutCents

	// AppliesTo IS READ OFF THE LINKS, not inferred from the totals above.
	//
	// Two wrong versions preceded this one and the second is the instructive
	// one. Naming both legs unconditionally was plainly wrong -- caveats()
	// emits this caveat when EITHER side is non-zero, so a transfers-in-only
	// document pointed at a node it does not carry, and ValidateCaveats caught
	// it via TestLabelFallback. Gating each leg on its own headline total fixed
	// that case and was still a PROXY: a non-zero transfers-out total says some
	// transfer_out fact was summed, not that the node it produced is spelled
	// "transfers/out". data/taxonomy.yaml admits categories under that kind
	// which would produce another id, and the failure mode is an aborted build
	// naming a node nobody wrote -- fail-closed, but from a message that points
	// at the wrong thing.
	//
	// The endpoints are in hand, so use them. A transfer link's own endpoint IS
	// the node, which is exact rather than a proxy for it, and it degrades the
	// right way: a document whose transfer endpoints are named something else
	// marks those, instead of asserting about ids it guessed.
	targets := transferEndpoints(links)

	if out == in {
		// NO RESIDUAL, SO NO PRINTED COLUMN TO NAME. The city prints a to-CIP
		// figure whatever the legs do, but a caveat that pointed at it here
		// would be explaining a difference this document does not have.
		return Caveat{
			ID: id,
			Summary: fmt.Sprintf(
				"No link pairs a transfer's two legs; that both sides total %s is not evidence they match.",
				dollars(out)),
			Text: unpaired + fmt.Sprintf(
				"That transfers out and transfers in both total %s is not evidence the "+
					"legs pair up; nothing has checked them against each other.", dollars(out)),
			AppliesTo: targets,
		}
	}
	verb := "exceed"
	// signed is out - in, kept alongside the magnitude the prose prints,
	// because the printed column below is transfers OUT to the CIP. A document
	// whose transfers IN exceeded its out by exactly the tabled figure would
	// otherwise be handed an outflow column as the explanation for an inflow
	// surplus -- the arithmetic would match and the sentence would be nonsense.
	signed := out - in
	residual := signed
	if residual < 0 {
		verb = "fall short of"
		residual = -residual
	}
	const stated = " It is stated as headline.transfer_residual_cents rather than netted " +
		"away or padded with an invented link."

	// THE PRINTED CLAIM IS MADE ONLY WHEN THIS DOCUMENT'S OWN ARITHMETIC MEETS
	// THE PAGE. A hand-typed constant that has drifted from the graph it is
	// published beside would be exactly the kind of plausible wrong figure this
	// project exists to refuse, and the reader has no way to see the drift. So
	// the stronger sentence is earned per build rather than asserted once.
	if cip, ok := transfersOutToCIP[col]; ok && amount.Cents(signed) == cip.Cents {
		return Caveat{
			ID: id,
			Summary: fmt.Sprintf(
				"No link pairs a transfer's two legs; transfers out %s transfers in by %s, which the city prints as its own column.",
				verb, dollars(residual)),
			Text: unpaired + fmt.Sprintf(
				"Transfers out (%s) %s transfers in (%s), and the %s difference is not an "+
					"unexplained gap: the city prints it as a column of its own, \"Transfers "+
					"Out to CIP\", on the sources-and-uses schedule at PDF p%d -- with p76's "+
					"grand total printed beside it as the transfers-out figure that excludes "+
					"the CIP. Only the citywide total is published: splitting it by fund group "+
					"is our arithmetic, because that schedule prints one aggregate for every "+
					"non-major fund.%s",
				dollars(out), verb, dollars(in), dollars(residual), cip.Page, stated),
			AppliesTo: targets,
		}
	}
	return Caveat{
		ID: id,
		Summary: fmt.Sprintf(
			"No link pairs a transfer's two legs; transfers out %s transfers in by %s, and mapping p76 did not close it.",
			verb, dollars(residual)),
		Text: unpaired + fmt.Sprintf(
			"Transfers out (%s) %s transfers in (%s), and mapping p76 did not close the %s "+
				"difference: that schedule's own grand total is the transfers-in side, so the "+
				"gap is the city's rather than this project's.%s",
			dollars(out), verb, dollars(in), dollars(residual), stated),
		AppliesTo: targets,
	}
}

// dollars renders cents for prose. The exact ".00" is dropped because these
// schedules print whole dollars, and a caveat reading "$59,612,734.00" invites
// the reader to look for a precision the source does not have.
func dollars(cents int64) string {
	return strings.TrimSuffix(amount.Cents(cents).String(), ".00")
}

// citedFacts counts the distinct facts the links carry. A fact can be cited by
// only one link here, but counting distinctly rather than summing lengths keeps
// the figure honest if aggregation ever puts one fact behind two links.
func citedFacts(links []Link) int {
	seen := make(map[string]struct{})
	for _, l := range links {
		for _, id := range l.FactIDs {
			seen[id] = struct{}{}
		}
	}
	return len(seen)
}
