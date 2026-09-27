package check

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/jcrussell/livermore-budget/internal/amount"
	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/mapping"
	"github.com/jcrussell/livermore-budget/internal/project"
)

// The two rows pp.66-67 print that are stocks rather than flows: a balance
// carried into or out of the year is not money moving, so the projection records
// the facts and draws no link.
//
// These strings restate two unexported constants in internal/project. That is
// deliberate rather than an oversight: this check re-derives what the projection
// should have counted, and a check that imported the projection's own answer
// would be asking the thing under test. The restatement is safe in the direction
// that matters — if internal/project changes which rows are stocks, the counts
// identity below stops adding up and the check FAILS, rather than passing
// against a definition that has moved.
const (
	categoryFundBalanceBeginning = "fund-balance/beginning"
	categoryFundBalanceEnding    = "fund-balance/ending"
)

// graphAcyclic asserts no projection contains a cycle.
//
// A cycle in a flow diagram is money that funds itself. d3-sankey will render
// one — it lays the nodes out and draws the paths regardless — so nothing
// downstream of here would notice, and the figure a reader hovers would be part
// of a loop rather than a flow.
type graphAcyclic struct{}

var _ Check = (*graphAcyclic)(nil)

func (*graphAcyclic) ID() string { return "graph-acyclic" }
func (*graphAcyclic) Tier() int  { return 1 }
func (*graphAcyclic) Full() bool { return false }
func (*graphAcyclic) Description() string {
	return "no projection's links form a cycle, which would be money funding itself"
}

func (*graphAcyclic) Run(_ context.Context, s *Subject) (Result, error) {
	var findings []Finding
	links, nodes := 0, 0
	for _, p := range s.linkedDocuments() {
		links += len(p.Links)
		nodes += len(p.Nodes)
		if cycle := findCycle(p.Links); len(cycle) > 0 {
			findings = append(findings, finding(p.String(),
				"these nodes form a cycle: %s", strings.Join(cycle, " -> ")))
		}
	}
	return conclusion{
		subjects: links,
		unit:     "links",
		held: fmt.Sprintf("%d links over %d nodes in %d projections, no cycle",
			links, nodes, len(s.linkedDocuments())),
		nothing:  "no projection carries a link, so there is no path to walk",
		findings: findings,
	}.result(), nil
}

// findCycle returns the nodes of one cycle, first and last the same, or nil.
//
// Links are walked in the order the graph lists them, which internal/project
// sorts, so a graph with two cycles reports the same one on every run.
func findCycle(links []project.Link) []string {
	const (
		unseen = iota
		open   // on the current path
		done
	)
	adj := map[string][]string{}
	for _, l := range links {
		adj[l.Source] = append(adj[l.Source], l.Target)
	}

	state := map[string]int{}
	var path, cycle []string
	var visit func(string) bool
	visit = func(n string) bool {
		state[n] = open
		path = append(path, n)
		for _, m := range adj[n] {
			if state[m] == open {
				cycle = append(append([]string{}, path[slices.Index(path, m):]...), m)
				return true
			}
			if state[m] == unseen && visit(m) {
				return true
			}
		}
		path = path[:len(path)-1]
		state[n] = done
		return false
	}
	for _, l := range links {
		if state[l.Source] == unseen && visit(l.Source) {
			return cycle
		}
	}
	return nil
}

// derivedNodesJustified asserts everything the projection inferred says why.
//
// "Published and derived are different things" is the invariant the site is
// built on: a figure the city printed and a classification we inferred must not
// be presented alike. A derived node with no rationale and no source note is an
// editorial claim about public money with nothing behind it.
type derivedNodesJustified struct{}

var _ Check = (*derivedNodesJustified)(nil)

func (*derivedNodesJustified) ID() string { return "derived-nodes-justified" }
func (*derivedNodesJustified) Tier() int  { return 1 }
func (*derivedNodesJustified) Full() bool { return false }
func (*derivedNodesJustified) Description() string {
	return "every node marked derived carries a rationale and a source note, and the two nodes " +
		"the projection infers are marked derived"
}

// Run checks the claim in both directions.
//
// The forward direction — a derived node has its two fields — is the one the
// invariant states. The reverse matters as much: internal/project infers exactly
// two nodes, and it exports their ids so a check can name them. Without that
// half, a projection that dropped the derived flag would leave this check with
// nothing to look at, and although a vacuous result would say so, naming the two
// nodes turns "nothing to check" into "these two are wrong".
func (*derivedNodesJustified) Run(_ context.Context, s *Subject) (Result, error) {
	inferred := []string{project.NodeFundBalanceDraw, project.NodeFundBalanceContribution}
	var findings []Finding
	subjects := 0

	for _, p := range s.linkedDocuments() {
		for _, n := range p.Nodes {
			isInferred := slices.Contains(inferred, n.ID)
			if !n.Derived && !isInferred {
				continue
			}
			subjects++
			switch {
			case !n.Derived:
				findings = append(findings, finding(p.String()+" "+n.ID,
					"the projection infers this node but it is not marked derived"))
			case n.Rationale == "" && n.SourceNote == "":
				findings = append(findings, finding(p.String()+" "+n.ID,
					"is derived but carries neither a rationale nor a source note"))
			case n.Rationale == "":
				findings = append(findings, finding(p.String()+" "+n.ID,
					"is derived but carries no rationale"))
			case n.SourceNote == "":
				findings = append(findings, finding(p.String()+" "+n.ID,
					"is derived but carries no source note"))
			}
		}
	}
	return conclusion{
		subjects: subjects,
		unit:     "nodes",
		held:     fmt.Sprintf("%d derived nodes, each with a rationale and a source note", subjects),
		nothing:  "no projection carries a derived node, and neither node the projection infers is present",
		findings: findings,
	}.result(), nil
}

// linkValuesTieToFacts asserts every link is the sum of the facts it cites.
//
// It checks the correspondence between a link's value and its citation, which is
// not the same as checking the value. internal/project computes both from the same
// cells, so this cannot witness a wrong amount — fact-token-reparses does that —
// but the two sides are assembled differently: the projection sums a cell and then
// records which facts that cell held, while this sums whatever fact_ids the link
// actually carries. So a link that publishes one cell's total over another cell's
// citation fails here, and that is the failure worth catching: a reader who
// follows a citation lands on real rows of a real page that do not add up to the
// number they were shown.
type linkValuesTieToFacts struct{}

var _ Check = (*linkValuesTieToFacts)(nil)

func (*linkValuesTieToFacts) ID() string { return "link-values-tie-to-facts" }
func (*linkValuesTieToFacts) Tier() int  { return 1 }
func (*linkValuesTieToFacts) Full() bool { return false }
func (*linkValuesTieToFacts) Description() string {
	return "every link's value_cents equals the sum of the amounts of the facts its fact_ids " +
		"name, so its value and its citation are of the same cell"
}

// Run allows exactly one link a value that is not the plain sum: the
// fund-balance draw.
//
// The city prints CHANGE IN WORKING CAPITAL once per column, signed, and a
// Sankey cannot draw a negative link, so internal/project decomposes the sign
// into two nodes and the draw leg carries the negation of its facts. The
// condition is the source node id rather than the sign of the sum, because
// "value equals the absolute sum" would accept a leg pointing the wrong way,
// which is the error the decomposition can actually make.
func (*linkValuesTieToFacts) Run(_ context.Context, s *Subject) (Result, error) {
	// Two indexes, because a link may only cite facts from the slice its own
	// projection is of. A citation that resolves in the fact store but not in
	// that slice is a link drawn from another year's or another basis's figure —
	// the failure project.Options exists to prevent — and looking it up in the
	// whole store would accept it whenever the amount happened to agree.
	all := factIndex(s.Facts)

	var findings []Finding
	links := 0
	for _, p := range s.linkedDocuments() {
		selected := factIndex(factsFor(s.Facts, p.Options))
		for _, l := range p.Links {
			links++
			subject := fmt.Sprintf("%s %s -> %s", p, l.Source, l.Target)
			if len(l.FactIDs) == 0 {
				findings = append(findings, finding(subject,
					"cites no fact, so the %s it draws rests on nothing",
					amount.Cents(l.ValueCents)))
				continue
			}
			var sum int64
			unknown := false
			for _, id := range l.FactIDs {
				f, ok := selected[id]
				if !ok {
					unknown = true
					if other, exists := all[id]; exists {
						findings = append(findings, finding(subject,
							"cites fact %s, which is FY%d %s %s and not this projection's slice",
							id, other.FiscalYear, other.Basis, other.Scope))
					} else {
						findings = append(findings, finding(subject,
							"cites fact %s, which is not in %s", id, factsFile))
					}
					continue
				}
				sum += f.AmountCents
			}
			if unknown {
				continue
			}
			want := sum
			if l.Source == project.NodeFundBalanceDraw {
				want = -sum
			}
			if l.ValueCents != want {
				findings = append(findings, finding(subject,
					"value_cents is %s but its %d facts sum to %s (off by %s)",
					amount.Cents(l.ValueCents), len(l.FactIDs), amount.Cents(want),
					amount.Cents(l.ValueCents-want)))
			}
		}
	}
	return conclusion{
		subjects: links,
		unit:     "links",
		held:     fmt.Sprintf("%d links, each equal to the facts it cites", links),
		nothing:  "no projection carries a link",
		findings: findings,
	}.result(), nil
}

// linkLocatorsMatchTheirFacts asserts every link's locators are exactly the
// pages its facts were read from.
//
// A link publishes its citation twice -- fact_ids, and the (doc_id, page) pairs
// a client resolves the fact store by -- and the second is the one the site
// actually turns into a URL. Nothing else compares them. A locator naming a
// page the link's facts do not come from sends a reader to a shard that does
// not hold the figure they clicked, and every other check would stay green:
// link-values-tie-to-facts reads only fact_ids, and the shard itself is
// well-formed either way.
//
// IT RE-DERIVES THE GROUPING RATHER THAN CALLING project.sourcesOf, for the
// reason this file's header already gives for categoryFundBalanceBeginning: a
// check that asks the producer for the answer is asking the thing under test.
// locatorSet is exactly the code that built the field, so comparing against it
// would pass on any bug inside it.
//
// It resolves ids in the projection's OWN slice, as its neighbour does, so a
// locator derived from another year's fact is caught here rather than accepted
// because the page number happened to agree.
type linkLocatorsMatchTheirFacts struct{}

var _ Check = (*linkLocatorsMatchTheirFacts)(nil)

func (*linkLocatorsMatchTheirFacts) ID() string { return "link-locators-match-their-facts" }
func (*linkLocatorsMatchTheirFacts) Tier() int  { return 1 }
func (*linkLocatorsMatchTheirFacts) Full() bool { return false }
func (*linkLocatorsMatchTheirFacts) Description() string {
	return "every link's locators are exactly the (doc_id, page) pairs of the facts its " +
		"fact_ids name, so a reader following one lands on the page the figure was read from"
}

func (*linkLocatorsMatchTheirFacts) Run(_ context.Context, s *Subject) (Result, error) {
	var findings []Finding
	links := 0
	for _, p := range s.linkedDocuments() {
		selected := factIndex(factsFor(s.Facts, p.Options))
		for _, l := range p.Links {
			links++
			subject := fmt.Sprintf("%s %s -> %s", p, l.Source, l.Target)
			if l.Locators == nil {
				findings = append(findings, finding(subject,
					"has no locators at all, so nothing on the page can resolve the "+
						"%d facts it cites", len(l.FactIDs)))
				continue
			}
			pages := map[string]map[int]bool{}
			unknown := false
			for _, id := range l.FactIDs {
				f, ok := selected[id]
				if !ok {
					// link-values-tie-to-facts already names an unresolvable
					// citation, and with two findings per id a reader would
					// chase the same fix twice.
					unknown = true
					break
				}
				if pages[f.DocID] == nil {
					pages[f.DocID] = map[int]bool{}
				}
				pages[f.DocID][f.Page] = true
			}
			if unknown {
				continue
			}
			want := make([]project.Source, 0, len(pages))
			for _, doc := range slices.Sorted(maps.Keys(pages)) {
				ps := slices.Sorted(maps.Keys(pages[doc]))
				want = append(want, project.Source{DocID: doc, Pages: ps})
			}
			// slices.EqualFunc and not cmp.Diff: this is the fisc binary that
			// IS the gate, and go-cmp is a test library that panics on types
			// it cannot walk. The diff was never shown to anyone anyway --
			// the reader-facing message is describeSources' -- so it bought
			// nothing and put a panic path in the check.
			same := slices.EqualFunc(want, l.Locators,
				func(a, b project.Source) bool {
					return a.DocID == b.DocID && slices.Equal(a.Pages, b.Pages)
				})
			if !same {
				findings = append(findings, finding(subject,
					"locators are %s but its %d facts were read from %s",
					describeSources(l.Locators), len(l.FactIDs), describeSources(want)))
			}
		}
	}
	return conclusion{
		subjects: links,
		unit:     "links",
		held: fmt.Sprintf("%d links, each citing exactly the pages its facts were read from",
			links),
		nothing:  "no projection carries a link",
		findings: findings,
	}.result(), nil
}

// describeSources writes a locator list the way a citation reads, so a finding
// says "p66, p67" rather than printing a Go struct at someone trying to find
// the page.
func describeSources(ss []project.Source) string {
	if len(ss) == 0 {
		return "none"
	}
	parts := make([]string, 0, len(ss))
	for _, s := range ss {
		ps := make([]string, len(s.Pages))
		for i, p := range s.Pages {
			ps[i] = fmt.Sprintf("p%d", p)
		}
		parts = append(parts, fmt.Sprintf("%s %s", s.DocID, strings.Join(ps, ", ")))
	}
	return strings.Join(parts, "; ")
}

// countsReconcile re-derives every count a graph document publishes from the
// document's own links and the facts its options select, never from a second
// run of the producer, so a wrong citation changes the answer.
//
// The identity every document publishes is facts = facts_cited + facts_uncited,
// and it holds whatever an uncited fact is worth; that an uncited fact is a
// printed zero or a stock row is uncited-facts-are-printed-zeros'. counts.facts
// against a fresh selection from the store catches a selector dropped from the
// projection's filter; facts_cited and facts_cited_twice against the union of
// every link's fact_ids catch a citation the document dropped or counted twice;
// nodes and links against the arrays are a ratchet, and labelled as one.
type countsReconcile struct{}

var _ Check = (*countsReconcile)(nil)

func (*countsReconcile) ID() string { return "counts-reconcile" }
func (*countsReconcile) Tier() int  { return 1 }
func (*countsReconcile) Full() bool { return false }
func (*countsReconcile) Description() string {
	return "every graph document's counts -- facts, facts_cited, facts_uncited, " +
		"facts_cited_twice, nodes and links -- re-derived from its own links and the facts " +
		"it was built from, and facts = facts_cited + facts_uncited"
}

func (*countsReconcile) Run(_ context.Context, s *Subject) (Result, error) {
	var findings []Finding
	var summaries []string

	for _, p := range s.linkedDocuments() {
		c := p.Graph.Metadata.Counts
		slice := factsFor(s.Facts, p.Options)
		times := map[string]int{}
		for _, l := range p.Links {
			for _, id := range l.FactIDs {
				times[id]++
			}
		}
		cited, twice := 0, 0
		for _, n := range times {
			cited++
			if n > 1 {
				twice++
			}
		}
		uncited := 0
		for i := range slice {
			if times[slice[i].ID] == 0 {
				uncited++
			}
		}
		for _, cmp := range []struct {
			what      string
			got, want int
			why       string
		}{
			{"counts.facts", c.Facts, len(slice),
				"the document says it drew a different slice than the one it was built over"},
			{"counts.facts_cited", c.FactsCited, cited,
				"a citation the document dropped, or one it counted twice: this is the " +
					"number of DISTINCT facts some link names"},
			{"counts.facts_uncited", c.FactsUncited, uncited,
				"a fact that reached no link, and the document disagrees about how many"},
			{"counts.facts_cited_twice", c.FactsCitedTwice, twice,
				"the facts behind more than one link: on the drill-down a revenue row " +
					"behind both its flow into a fund and its line's rollup, or an " +
					"expenditure row behind both a division's object rows and the " +
					"fund-to-division link that totals them"},
			{"counts.nodes", c.Nodes, len(p.Nodes), "a ratchet on the arrays as published"},
			{"counts.links", c.Links, len(p.Links), "a ratchet on the arrays as published"},
		} {
			if cmp.got != cmp.want {
				findings = append(findings, finding(p.String(),
					"%s is %d and re-derives to %d: %s", cmp.what, cmp.got, cmp.want, cmp.why))
			}
		}
		// THE IDENTITY, asserted against the document's OWN numbers rather than
		// the re-derived ones: whether each is right is the arms above; this is
		// whether they are consistent with each other, which is what the
		// document publishes as a claim.
		if c.FactsCited+c.FactsUncited != c.Facts {
			findings = append(findings, finding(p.String(),
				"counts.facts is %d and facts_cited + facts_uncited is %d + %d = %d; the "+
					"document's published identity is that every fact is either behind a "+
					"link or uncited, and the two numbers it publishes do not add up to the third",
				c.Facts, c.FactsCited, c.FactsUncited, c.FactsCited+c.FactsUncited))
		}
		summaries = append(summaries, fmt.Sprintf("%s (%d = %d cited + %d uncited, %d cited twice)",
			p, c.Facts, c.FactsCited, c.FactsUncited, c.FactsCitedTwice))
	}

	return conclusion{
		subjects: len(summaries),
		unit:     "graph documents",
		held:     strings.Join(summaries, "; "),
		nothing:  "no projection built a graph, so there are no counts to reconcile",
		findings: findings,
	}.result(), nil
}

// fundGroupInternalService is the fund group whose flows are inside the city.
//
// It restates an unexported constant in internal/project, and the restatement is
// the point: the external headline figures are the gross ones minus this group,
// so a check that took the projection's word for which group that is would not be
// checking anything. It fails closed — were the projection to classify a
// different group as internal, the sums below would disagree with what it
// published — and data/funds.yaml records `internal-service` as a fund type, so
// the fact-vocabulary check would already have failed on a group that does not
// exist.
const fundGroupInternalService = "internal-service"

// headlineTiesToFacts asserts the five revenue and expenditure headline figures
// are the sums they claim to be.
//
// These are the figures a reader quotes without reading the chart, and they are
// accumulated cell by cell inside internal/project rather than published as
// anything a consumer can re-add. Before this check, all five could be wrong with
// every link still tying to its facts.
//
// What it witnesses is the CLASSIFICATION, not the amounts: the sums here are over
// the same facts the projection used, so a corrupted amount moves both sides
// together (fact-token-reparses is what catches that). The rule each side applies
// is different, though, and that difference is the point — this one filters facts
// by kind and by fund group, while the projection accumulates per printed cell as
// it decides each link's kind, so an external figure that includes internal service
// charges, or a gross figure that has quietly stopped including them, fails here.
type headlineTiesToFacts struct{}

var _ Check = (*headlineTiesToFacts)(nil)

func (*headlineTiesToFacts) ID() string { return "headline-ties-to-facts" }
func (*headlineTiesToFacts) Tier() int  { return 1 }
func (*headlineTiesToFacts) Full() bool { return false }
func (*headlineTiesToFacts) Description() string {
	return "the gross and external revenue and expenditure figures, and the naive expenditure " +
		"figure, each equal the sum of the facts they are of"
}

// Run sums the facts each figure is of, which for the two external figures means
// every fund group but the internal service one: an Internal Service Fund charge
// is billed by one city department to another, so counting it as revenue AND as the
// paying department's expenditure double-counts it.
//
// The naive figure is here rather than with headline-naive-expenditure because its
// identity — expenditures plus transfers out — is a sum of facts like the other
// four, while that check makes a different kind of claim about it. Keeping the two
// apart is also what lets each report a subject count it can defend.
func (*headlineTiesToFacts) Run(_ context.Context, s *Subject) (Result, error) {
	var findings []Finding
	subjects := 0

	for _, p := range s.graphs() {
		var grossRevenue, grossExpenditure, externalRevenue, externalExpenditure, transfersOut int64
		for _, f := range factsFor(s.Facts, p.Options) {
			external := f.FundGroup != fundGroupInternalService
			switch f.Kind {
			case mapping.KindRevenue:
				subjects++
				grossRevenue += f.AmountCents
				if external {
					externalRevenue += f.AmountCents
				}
			case mapping.KindExpenditure:
				subjects++
				grossExpenditure += f.AmountCents
				if external {
					externalExpenditure += f.AmountCents
				}
			case mapping.KindTransferOut:
				// A subject of this check too: the naive figure is not a sum of
				// expenditures alone, and counting the facts behind a figure is
				// what makes "0 subjects" mean something.
				subjects++
				transfersOut += f.AmountCents
			default:
				// Transfers in and fund balance have their own figures, checked by
				// headline-transfer-residual and carried by their own links.
			}
		}

		h := p.Graph.Metadata.Headline
		for _, f := range []struct {
			key       string
			got, want int64
		}{
			{"all_funds_gross_revenue_cents", h.AllFundsGrossRevenueCents, grossRevenue},
			{"all_funds_gross_expenditure_cents", h.AllFundsGrossExpenditureCents, grossExpenditure},
			{"external_revenue_cents", h.ExternalRevenueCents, externalRevenue},
			{"external_expenditure_cents", h.ExternalExpenditureCents, externalExpenditure},
			{"naive_expenditure_cents", h.NaiveExpenditureCents, grossExpenditure + transfersOut},
		} {
			if f.got != f.want {
				findings = append(findings, finding(p.String(),
					"%s is %s but the facts sum to %s (off by %s)",
					f.key, amount.Cents(f.got), amount.Cents(f.want), amount.Cents(f.got-f.want)))
			}
		}
	}
	return conclusion{
		subjects: subjects,
		unit:     "facts",
		held: fmt.Sprintf("%d revenue, expenditure and transfer-out facts across %d projections, "+
			"each of the five figures the sum of them", subjects, len(s.graphs())),
		nothing:  "no fact is a revenue, an expenditure or a transfer out, so there is no headline to check",
		findings: findings,
	}.result(), nil
}

// headlineTransferResidual asserts the transfer headline is the facts, and the
// residual is the difference between its two halves.
//
// The residual is not zero and the caveats say why. It is NOT waiting on the p76
// transfer schedule being mapped: p76's own grand total is the transfers-in side
// to the cent, so mapping it pairs every leg the city itemises and leaves the
// $38,086,737 exactly where it is (fisc-5gk.3, proved in
// internal/mapping/transfers_p76_test.go). Publishing it as a figure rather
// than as prose is what lets it move when the city's own schedules move
// (fisc-1wr.4), and this check is what makes it a figure that has been checked.
type headlineTransferResidual struct{}

var _ Check = (*headlineTransferResidual)(nil)

func (*headlineTransferResidual) ID() string { return "headline-transfer-residual" }
func (*headlineTransferResidual) Tier() int  { return 1 }
func (*headlineTransferResidual) Full() bool { return false }
func (*headlineTransferResidual) Description() string {
	return "headline.transfer_residual_cents equals internal_transfer_out minus " +
		"internal_transfer_in, and both equal the transfer facts"
}

func (*headlineTransferResidual) Run(_ context.Context, s *Subject) (Result, error) {
	var findings []Finding
	subjects := 0

	for _, p := range s.graphs() {
		var in, out int64
		for _, f := range factsFor(s.Facts, p.Options) {
			switch f.Kind {
			case mapping.KindTransferIn:
				subjects++
				in += f.AmountCents
			case mapping.KindTransferOut:
				subjects++
				out += f.AmountCents
			default:
			}
		}
		h := p.Graph.Metadata.Headline
		if h.InternalTransferInCents != in {
			findings = append(findings, finding(p.String(),
				"internal_transfer_in_cents is %s but the transfer_in facts sum to %s",
				amount.Cents(h.InternalTransferInCents), amount.Cents(in)))
		}
		if h.InternalTransferOutCents != out {
			findings = append(findings, finding(p.String(),
				"internal_transfer_out_cents is %s but the transfer_out facts sum to %s",
				amount.Cents(h.InternalTransferOutCents), amount.Cents(out)))
		}
		if want := h.InternalTransferOutCents - h.InternalTransferInCents; h.TransferResidualCents != want {
			findings = append(findings, finding(p.String(),
				"transfer_residual_cents is %s but out minus in is %s",
				amount.Cents(h.TransferResidualCents), amount.Cents(want)))
		}
	}
	return conclusion{
		subjects: subjects,
		unit:     "transfer facts",
		held: fmt.Sprintf("%d transfer facts across %d projections, each headline the sum of them",
			subjects, len(s.graphs())),
		nothing:  "no fact is a transfer, so there is no residual to state",
		findings: findings,
	}.result(), nil
}

// headlineNaiveExpenditure asserts the deliberately wrong figure still differs
// from the right one.
//
// naive_expenditure_cents is what summing the printed expenditure column gives:
// $313,708,146 where the city spends $254,095,412, a 23% inflation from counting
// every transfer twice. It ships so the page can show the error it is avoiding, and
// it is worth shipping only while it differs from the external figure.
//
// This check establishes the inequality and nothing more. It does not establish
// that either figure is right: driving external expenditure to $0.00 satisfies it,
// because $0.00 is not $59.6M. What the two figures ARE is headline-ties-to-facts'
// claim, and that check is where a wrong external figure fails; the two together are
// the pair, and neither on its own says what the headline means.
type headlineNaiveExpenditure struct{}

var _ Check = (*headlineNaiveExpenditure)(nil)

func (*headlineNaiveExpenditure) ID() string { return "headline-naive-expenditure" }
func (*headlineNaiveExpenditure) Tier() int  { return 1 }
func (*headlineNaiveExpenditure) Full() bool { return false }
func (*headlineNaiveExpenditure) Description() string {
	return "the external expenditure figure is not equal to naive_expenditure_cents, so the " +
		"double count the naive figure exists to illustrate is still being netted out"
}

// Run counts a projection as a subject only where the two figures CAN differ, and
// looks at nothing else: every projection it does not count is one it did not
// examine, which is what Result.Subjects has to mean.
//
// The difference is exactly transfers out plus the internal service charges the
// external figure excludes: naive is gross plus transfers out, and external is
// gross minus internal service expenditure. So a graph with no transfers out and no
// internal service expenditure makes "external is not naive" a claim about nothing,
// and asserting it there would fail a projection that is perfectly correct.
//
// Both halves of that condition are read off the headline rather than out of the
// links, and the second is stated as "gross is not external" rather than as "some
// link is classified internal_service": a projection whose only internal service
// flow is REVENUE has such a link and no expenditure difference at all, and testing
// for the link would fail it.
func (*headlineNaiveExpenditure) Run(_ context.Context, s *Subject) (Result, error) {
	var findings []Finding
	var summaries []string
	subjects := 0

	for _, p := range s.graphs() {
		h := p.Graph.Metadata.Headline
		if h.InternalTransferOutCents == 0 &&
			h.AllFundsGrossExpenditureCents == h.ExternalExpenditureCents {
			continue
		}
		subjects++
		if h.ExternalExpenditureCents == h.NaiveExpenditureCents {
			findings = append(findings, finding(p.String(),
				"external_expenditure_cents equals naive_expenditure_cents (%s), so the "+
					"projection is no longer netting out the double count the naive figure exists "+
					"to illustrate", amount.Cents(h.NaiveExpenditureCents)))
			continue
		}
		summaries = append(summaries, fmt.Sprintf("%s (external %s vs naive %s)",
			p, amount.Cents(h.ExternalExpenditureCents), amount.Cents(h.NaiveExpenditureCents)))
	}
	return conclusion{
		subjects: subjects,
		unit:     "projections",
		held:     strings.Join(summaries, "; "),
		nothing: "no projection publishes a transfer out or an internal service expenditure, so " +
			"the naive figure cannot differ from the external one",
		findings: findings,
	}.result(), nil
}

// transferLegsPair asserts every transfer_id has two equal legs, one receiving
// and one paying.
//
// Only transfers-by-fund draws each end of a movement as its own link, so only
// it gives this a subject, and there a link with no id is a finding: blanking
// one leg's TransferID reports both halves.
//
// That the two legs are the same printed figure is the projection's guarantee;
// a finding means the document drew half a movement, not that the page
// disagrees with itself.
type transferLegsPair struct{}

var _ Check = (*transferLegsPair)(nil)

func (*transferLegsPair) ID() string { return "transfer-legs-pair" }
func (*transferLegsPair) Tier() int  { return 1 }
func (*transferLegsPair) Full() bool { return false }
func (*transferLegsPair) Description() string {
	return "every transfer_id names exactly two links of equal value, one receiving and one paying, " +
		"over the documents that draw each end of a movement as its own link"
}

func (*transferLegsPair) Run(_ context.Context, s *Subject) (Result, error) {
	legs := map[string][]project.Link{}
	roles := map[string]map[string]string{}
	var ids []string
	var findings []Finding
	for _, p := range s.linkedDocuments() {
		role := map[string]string{}
		for _, n := range p.Nodes {
			role[n.ID] = n.Role
		}
		// The two transfer networks draw every link as a leg, so one there with
		// no transfer_id is half a movement the pairing would otherwise skip.
		paired := p.Name == project.TransfersByFundProjection || p.Name == project.TransfersOutProjection
		for _, l := range p.Links {
			if l.TransferID == "" {
				if paired {
					findings = append(findings, finding(p.String(),
						"%s -> %s carries no transfer_id, and every link of this document is a leg", l.Source, l.Target))
				}
				continue
			}
			key := p.String() + " transfer " + l.TransferID
			if _, seen := legs[key]; !seen {
				ids = append(ids, key)
			}
			legs[key] = append(legs[key], l)
			roles[key] = role
		}
	}

	for _, id := range ids {
		pair := legs[id]
		if len(pair) != 2 {
			findings = append(findings, finding(id, "names %d legs, want 2: %s",
				len(pair), describeLegs(pair)))
			continue
		}
		role := roles[id]
		source := func(l project.Link) bool { return role[l.Source] == project.RoleTransferSource }
		sink := func(l project.Link) bool { return role[l.Target] == project.RoleTransferSink }
		// Exclusive, so a leg drawn from payer's end to receiver's end is neither.
		receiving := func(l project.Link) bool { return source(l) && !sink(l) }
		paying := func(l project.Link) bool { return sink(l) && !source(l) }
		if (!receiving(pair[0]) || !paying(pair[1])) && (!paying(pair[0]) || !receiving(pair[1])) {
			findings = append(findings, finding(id, "its two legs are not one receiving and one paying: %s",
				describeLegs(pair)))
			continue
		}
		if pair[0].ValueCents != pair[1].ValueCents {
			findings = append(findings, finding(id, "its two legs are %s and %s (off by %s): %s",
				amount.Cents(pair[0].ValueCents), amount.Cents(pair[1].ValueCents),
				amount.Cents(pair[0].ValueCents-pair[1].ValueCents), describeLegs(pair)))
		}
	}
	return conclusion{
		subjects: len(ids),
		unit:     "transfer ids",
		held:     fmt.Sprintf("%d transfer ids, each with two equal legs", len(ids)),
		nothing:  "no link carries a transfer_id, so no pairing has been checked",
		findings: findings,
	}.result(), nil
}

func describeLegs(legs []project.Link) string {
	out := make([]string, 0, len(legs))
	for _, l := range legs {
		out = append(out, fmt.Sprintf("%s -> %s", l.Source, l.Target))
	}
	return strings.Join(out, ", ")
}

// sameTierContainers is the one container that holds nodes at its own tier,
// and the id form it holds: transfers-out's receivers' ends.
var sameTierContainers = map[string]string{"transfers/out": "transfer-to/"}

// nodeHierarchyWellFormed asserts the tier hierarchy a document publishes can
// actually be folded: every parent resolves, the hierarchy runs coarse to fine,
// and no node is its own ancestor.
//
// THIS CHECK USED TO BE CALLED aggregation-invariance AND IT PROMISED SOMETHING
// IT CANNOT DELIVER. Its old description — "the total at every tier depth equals
// the total at every other depth" — cannot fail on arithmetic within one
// document, because a fold maps each link to exactly one folded link and the sum
// is invariant under relabelling whatever node.parent says. Node.Parent is one
// string, so it cannot even express the double parent the old doc comment named
// as the thing it caught. Written that way the check passes on any input, and
// retiring a DECLARED vacuous check by replacing it with a green tautology is
// worse than the state it replaced: the vacuity was at least visible in the
// report. Check.ID's own doc comment says an id "says what is claimed rather
// than what is done", so the id moved with the claim.
//
// THE INTER-DOCUMENT FOLD IS REAL AND IS ALREADY DISCHARGED ELSEWHERE, which is
// the other half of why this one is structural. Folding this document's fund
// totals to fund groups and comparing them against the SPINE's published
// fund-group figures is a comparison between two schedules, and
// cuts-tie-along-the-lattice makes exactly that comparison, per cell, at zero
// tolerance, over Subject.Facts and without a graph. Rebuilding it here would
// buy nothing.
//
// WHAT IS GENUINELY NEW IS STRUCTURAL, and none of it was asserted anywhere
// before:
//
//   - every node.parent RESOLVES to a node in this document. A dangling parent
//     is a fold that loses money: the client aggregating to a coarser tier finds
//     no box to put the child in.
//   - the parent's tier is strictly COARSER than the child's. An inverted edge
//     makes the fold run the wrong way, and a client walking it renders a
//     hierarchy inside out.
//   - no node is its own ancestor. Parent is one string so a cycle needs at
//     least two nodes, and a client folding one would not terminate.
//   - no node is parented to a FLOW ENDPOINT THAT CARRIES A FLOW in its
//     document: the child's money would be both inside that box and beside it.
//
// An endpoint touching no link is a container, and folding into it is
// ordinary: transfers-by-fund draws transfers/in that way. Give it one link and
// every child is a finding. One container may hold nodes at its own tier, the
// ones sameTierContainers names: transfers-out parents every receiver's end to
// transfers/out, and both are tier 5, where the spine draws the endpoint the
// reader opens.
//
// SUBJECTS ARE THE PARENTED NODES, so this stays honestly vacuous over a
// document with no hierarchy — the spine — and goes live over the first one that
// has a hierarchy to be well-formed.
type nodeHierarchyWellFormed struct{}

var _ Check = (*nodeHierarchyWellFormed)(nil)

func (*nodeHierarchyWellFormed) ID() string { return "node-hierarchy-well-formed" }
func (*nodeHierarchyWellFormed) Tier() int  { return 1 }
func (*nodeHierarchyWellFormed) Full() bool { return false }
func (*nodeHierarchyWellFormed) Description() string {
	return "every node.parent resolves to a node of the same document, at a strictly coarser " +
		"tier or a flow endpoint drawn as a container at its own, with no node its own ancestor " +
		"and none parented to a flow endpoint that carries a flow in that same document"
}

func (*nodeHierarchyWellFormed) Run(_ context.Context, s *Subject) (Result, error) {
	var findings []Finding
	parented, docs, endpointParents := 0, 0, 0

	for _, p := range s.linkedDocuments() {
		byID := make(map[string]project.Node, len(p.Nodes))
		for _, n := range p.Nodes {
			byID[n.ID] = n
		}
		flowing := make(map[string]bool, 2*len(p.Links))
		for _, l := range p.Links {
			flowing[l.Source] = true
			flowing[l.Target] = true
		}
		some := false
		for _, n := range p.Nodes {
			if n.Parent == "" {
				continue
			}
			parented++
			some = true

			parent, ok := byID[n.Parent]
			if !ok {
				findings = append(findings, finding(p.String(),
					"node %q is parented to %q, which is not a node of this document. A "+
						"client folding to a coarser tier has no box to put it in, so the "+
						"money it carries leaves the picture", n.ID, n.Parent))
				continue
			}
			sameTier := false
			if _, isEndpoint := endpointTiers[n.Parent]; isEndpoint {
				if flowing[n.Parent] {
					findings = append(findings, finding(p.String(),
						"node %q is parented to %q, a flow endpoint this document draws a "+
							"flow at. An endpoint carrying its own flow sits outside the "+
							"hierarchy, so a node folding into one is both inside that box "+
							"and beside it",
						n.ID, n.Parent))
					continue
				}
				endpointParents++
				form, ok := sameTierContainers[n.Parent]
				sameTier = ok && strings.HasPrefix(n.ID, form)
			}
			if parent.Tier > n.Tier || (parent.Tier == n.Tier && !sameTier) {
				findings = append(findings, finding(p.String(),
					"node %q is at tier %d and its parent %q is at tier %d. A parent is "+
						"strictly coarser than its child, or the fold runs the wrong way "+
						"and the client renders the hierarchy inside out",
					n.ID, n.Tier, n.Parent, parent.Tier))
				continue
			}
			if cycle := ancestorCycle(byID, n.ID); len(cycle) > 0 {
				findings = append(findings, finding(p.String(),
					"node %q is its own ancestor: %s. A client folding this chain does not "+
						"terminate", n.ID, joinArrow(cycle)))
			}
		}
		if some {
			docs++
		}
	}

	return conclusion{
		subjects: parented,
		unit:     "parented nodes",
		held: fmt.Sprintf("%d parented nodes across %d document(s) with a hierarchy, each "+
			"resolving to a node of its own document at a strictly coarser tier or into a "+
			"container endpoint at its own, none its own ancestor and none folding into a flow endpoint its own document draws a "+
			"flow at; %d fold into an endpoint drawn as a container", parented, docs, endpointParents),
		nothing: "no node carries a parent, so no document publishes a hierarchy to be " +
			"well-formed",
		findings: findings,
	}.result(), nil
}

// ancestorCycle walks a node's parent chain and returns the cycle it closes, or
// nil. The visited set is what makes it terminate on the very input it exists to
// report.
func ancestorCycle(byID map[string]project.Node, start string) []string {
	seen := map[string]bool{start: true}
	path := []string{start}
	for id := byID[start].Parent; id != ""; id = byID[id].Parent {
		path = append(path, id)
		if seen[id] {
			return path
		}
		seen[id] = true
		if _, ok := byID[id]; !ok {
			return nil
		}
	}
	return nil
}

// joinArrow renders a chain the way a reader follows it.
func joinArrow(path []string) string { return strings.Join(path, " -> ") }

// constraintTierVocabulary asserts a node's constraint_tier is one the fund
// registry actually uses.
//
// IT IS NO LONGER VACUOUS, and this comment said it was long after the
// drill-down landed. The four fund-flows documents publish 247 nodes carrying a
// non-empty constraint_tier; the two spine documents publish none, and that
// remains correct rather than lazy — data/funds.yaml records a tier per fund,
// pp.66-67 publish only fund groups, and filling one in there would present an
// editorial classification as published data. The distinction is per document,
// which is what the old wording lost.
type constraintTierVocabulary struct{}

var _ Check = (*constraintTierVocabulary)(nil)

func (*constraintTierVocabulary) ID() string { return "constraint-tier-vocabulary" }
func (*constraintTierVocabulary) Tier() int  { return 1 }
func (*constraintTierVocabulary) Full() bool { return false }
func (*constraintTierVocabulary) Description() string {
	return "every constraint_tier a node carries is one data/funds.yaml uses, with the source " +
		"note and the restriction note it was read from"
}

// Run takes the vocabulary from the loaded registry rather than from a list in
// this file, which is the same reading Registry.FundGroup takes of a fund type: a
// tier no fund is recorded under is not a tier this corpus can classify anything
// as. The closed set registry.go declares is the ceiling; what funds.yaml uses is
// the floor, and a node may only carry the latter.
func (*constraintTierVocabulary) Run(_ context.Context, s *Subject) (Result, error) {
	used := map[string]bool{}
	var known []string
	for _, f := range s.Vocabulary.Funds() {
		if f.ConstraintTier != "" && !used[f.ConstraintTier] {
			used[f.ConstraintTier] = true
			known = append(known, f.ConstraintTier)
		}
	}
	slices.Sort(known)

	var findings []Finding
	subjects := 0
	tiered := map[string]bool{}
	for _, p := range s.linkedDocuments() {
		for _, n := range p.Nodes {
			if n.ConstraintTier == "" {
				continue
			}
			subjects++
			if !used[n.ConstraintTier] {
				findings = append(findings, finding(p.String()+" "+n.ID,
					"constraint_tier %q is not one data/funds.yaml uses (%s)",
					n.ConstraintTier, strings.Join(known, ", ")))
			}
			// THE ARM WITH TEETH. The vocabulary clause above cannot fail short
			// of a corrupted copy; this one asserts the thing the project's
			// premise actually turns on.
			if n.SourceNote == "" {
				findings = append(findings, finding(p.String()+" "+n.ID,
					"carries constraint_tier %q and no source_note. The tier is OUR reading "+
						"of the Description of Funds narrative (pp.258-261) and the node is "+
						"the city's, so node.derived stays false and the disclosure has "+
						"nowhere else to go", n.ConstraintTier))
			}
			if n.Rationale == "" {
				findings = append(findings, finding(p.String()+" "+n.ID,
					"carries constraint_tier %q and no rationale. The rationale is the "+
						"restriction note the tier was read from; a classification "+
						"published without the sentence behind it is the editorial claim "+
						"this project refuses to make unmarked", n.ConstraintTier))
			}
			tiered[p.String()] = true
		}
	}

	// AND THE DOCUMENT ITSELF MUST DISCLOSE, or the per-node notes are a
	// provenance panel a reader has to know to open. docs/sankey-contract.md
	// requires the sentence in metadata.caveats, and it is compared against
	// project's own constant rather than against prose written twice: two
	// authors agreeing is not the same claim as the document saying the thing.
	for _, p := range s.Projections {
		if !tiered[p.String()] {
			continue
		}
		// Every graph carries the same metadata block, so the document that
		// tiers a node is the document whose caveats are read.
		caveats := p.Graph.Metadata.Caveats
		// TWO FINDINGS, NOT ONE, now that a caveat has an id as well as a
		// sentence. A caveat is looked up by ID and then its TEXT is compared,
		// and the pair is the whole point:
		//
		// Matching on the id alone would go green over a document whose
		// disclosure had been reworded into something weaker -- a new fail-open
		// in the exact place the comment above was written to close, and a
		// quieter one, because the anchor would still resolve and the page
		// would still render a paragraph under the right heading.
		//
		// Matching on the text alone works, and was the previous behaviour, but
		// then the finding cannot tell "discloses nothing" from "discloses
		// something else" -- and those have different fixes. The id is free.
		i := slices.IndexFunc(caveats, func(c project.Caveat) bool {
			return c.ID == project.ConstraintTierCaveatID
		})
		switch {
		case i < 0:
			findings = append(findings, finding(p.String(),
				"nodes here carry constraint tiers and metadata.caveats does not carry the "+
					"disclosure sentence. A reader of this file alone would take an "+
					"editorial classification for something the city printed"))
		case caveats[i].Text != project.ConstraintTierCaveat().Text:
			findings = append(findings, finding(p.String(),
				"nodes here carry constraint tiers and metadata.caveats carries %q with text "+
					"that is not the disclosure internal/project declares. The document "+
					"discloses something, under the right anchor, and it is not the sentence "+
					"the contract requires", project.ConstraintTierCaveatID))
		// THE SUMMARY IS CHECKED TOO, because it is the string a reader
		// actually meets. index.html prints summaries and links to the text; a
		// document whose text was word-perfect and whose
		// summary said something else would pass the arm above and still
		// mislead every reader who did not follow the link -- which is most of
		// them, since following it is the extra step the summary exists to
		// save. Nothing else constrains this field beyond non-emptiness.
		case caveats[i].Summary != project.ConstraintTierCaveat().Summary:
			findings = append(findings, finding(p.String(),
				"nodes here carry constraint tiers and metadata.caveats carries %q whose "+
					"SUMMARY is not the one internal/project declares. The summary is what "+
					"the chart pages print in place of the text, so a reader who does not "+
					"follow the link meets this sentence and no other", project.ConstraintTierCaveatID))
		}
	}

	return conclusion{
		subjects: subjects,
		unit:     "nodes",
		held: fmt.Sprintf("%d nodes carry one of %s, each with the source note and the "+
			"restriction note it was read from, in %d document(s) that disclose the "+
			"derivation in metadata.caveats", subjects, strings.Join(known, ", "), len(tiered)),
		nothing:  "no node carries a constraint_tier, so none has been checked against data/funds.yaml",
		findings: findings,
	}.result(), nil
}

// factsFor is the facts one projection is of.
//
// It restates internal/project's own selection on purpose. A check that asked
// the projection which facts it had used would be asking the thing under test;
// re-selecting from the fact store is what makes counts.facts a claim rather
// than a restatement. Every selector is applied, because a projection
// filtered on fiscal year alone doubles every figure and still balances. The
// kinds are the published Options', so a fact of a kind no document selects
// is outside every count here.
func factsFor(facts []fact.Fact, o project.Options) []fact.Fact {
	out := make([]fact.Fact, 0, len(facts))
	for _, f := range facts {
		if !o.HasScope(f.Scope) || !o.HasKind(f.Kind) {
			continue
		}
		if !slices.Contains(o.Columns, project.Column{FiscalYear: f.FiscalYear, Basis: f.Basis}) {
			continue
		}
		out = append(out, f)
	}
	return out
}

// factIndex keys facts by id, which is unique by construction and proved so by
// the fact-ids-unique check.
func factIndex(facts []fact.Fact) map[string]fact.Fact {
	out := make(map[string]fact.Fact, len(facts))
	for _, f := range facts {
		out[f.ID] = f
	}
	return out
}
