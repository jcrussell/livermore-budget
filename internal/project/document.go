package project

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/pkg/cmdutil"
	"github.com/jcrussell/livermore-budget/schema"
)

// Document is every graph this package publishes: the citywide spine and the
// schedules drawn beside it, one type whatever the schedule.
//
// Field order is the JSON key order -- encoding/json emits struct fields in
// declaration order -- and schema/projection.schema.json holds the shape.
type Document struct {
	SchemaVersion int      `json:"schema_version"`
	Projection    string   `json:"projection"`
	Metadata      Metadata `json:"metadata"`
	Nodes         []Node   `json:"nodes"`
	Links         []Link   `json:"links"`
}

// Metadata is what a reader needs to know which slice of the budget the graph
// below it covers, and what it deliberately leaves out.
type Metadata struct {
	// GeneratedBy is build.Get().String(), so a reader can tell which binary
	// wrote the file.
	GeneratedBy string `json:"generated_by"`
	FiscalYear  int    `json:"fiscal_year"`
	// FiscalYearLabel is the city's own way of writing the year: FY2026 is
	// "FY 2025-26" on every page of the budget book, and a chart captioned
	// "2026" would not match anything the reader is holding.
	FiscalYearLabel string `json:"fiscal_year_label"`
	Basis           string `json:"basis"`
	// Scopes is the schedule set the facts came from, in the order the
	// projection declares it. A list on every document, one entry long on
	// most: a document of two schedules writing the first into a singular
	// key would publish one schedule as the whole of it.
	Scopes []string `json:"scopes"`
	// Currency and Units are stated rather than assumed. Money is an integer
	// count of cents everywhere in this project, and a document that does not
	// say so is one a reader has to guess about.
	Currency string   `json:"currency"`
	Units    string   `json:"units"`
	Sources  []Source `json:"sources"`
	// Headline is the spine's alone and is the one key a document may omit:
	// its figures are citywide totals over a single-grain view, and a
	// document holding the same money at two grains has no total to name.
	// Eight zeros in its place would be absent-is-not-zero at document level.
	Headline *Headline `json:"headline,omitempty"`
	Counts   Counts    `json:"counts"`
	// Caveats are the things this chart cannot show, in the chart's own file.
	// A caveat that lives only in a design document is a caveat nobody reads.
	Caveats []Caveat `json:"caveats"`
}

// Counts is how much of the corpus a document accounts for, and the identity
// every document publishes is facts = facts_cited + facts_uncited.
type Counts struct {
	// Facts is how many facts matched the options, which is NOT how many links
	// were drawn: a zero-valued cell earns no link, and neither does a stock
	// row.
	Facts int `json:"facts"`
	// FactsCited is how many DISTINCT facts some link carries.
	FactsCited int `json:"facts_cited"`
	// FactsUncited is the facts no link carries: a printed zero, or on the
	// spine a stock row. uncited-facts-are-printed-zeros holds that to the
	// store; every builder refuses anything else at build.
	FactsUncited int `json:"facts_uncited"`
	// FactsCitedTwice is how many distinct facts are behind MORE THAN ONE
	// link. Zero on a document whose links partition its facts; on the
	// drill-down, where both sides carry a summing link above the cell, it
	// warns that summing every link's value_cents double-counts.
	FactsCitedTwice int `json:"facts_cited_twice"`
	Nodes           int `json:"nodes"`
	Links           int `json:"links"`
}

// tally is how the links account for the selected facts: the counts a
// document publishes, and the facts no link carries, in selection order, for
// the builder to hold to its own rule about what may go uncited.
func tally(selected []fact.Fact, links []Link, nodes int) (Counts, []*fact.Fact) {
	times := map[string]int{}
	for _, l := range links {
		for _, id := range l.FactIDs {
			times[id]++
		}
	}
	c := Counts{Facts: len(selected), Nodes: nodes, Links: len(links)}
	var uncited []*fact.Fact
	for i := range selected {
		switch n := times[selected[i].ID]; {
		case n == 0:
			c.FactsUncited++
			uncited = append(uncited, &selected[i])
		case n > 1:
			c.FactsCited++
			c.FactsCitedTwice++
		default:
			c.FactsCited++
		}
	}
	return c, uncited
}

// refuseUncited is the build-time half of uncited-facts-are-printed-zeros:
// a fact no link carries is a printed zero, or the document has dropped money
// in silence and its own counts cannot show it. allow names the one other
// shape a builder admits, or is nil.
func refuseUncited(name string, uncited []*fact.Fact, allow func(*fact.Fact) bool) error {
	for _, f := range uncited {
		if f.AmountCents == 0 || (allow != nil && allow(f)) {
			continue
		}
		return cmdutil.WithHint(
			fmt.Errorf("%s: fact %s is carried by no link and is not a printed zero", name, f.ID),
			"facts = facts_cited + facts_uncited is every document's published identity "+
				"and every uncited fact is a cell the city printed as nothing; a fact reaching "+
				"no link for another reason is money dropped in silence")
	}
	return nil
}

// Source is one document the facts came from, with the pages actually read.
type Source struct {
	DocID string `json:"doc_id"`
	Pages []int  `json:"pages"`
}

// Caveat is one thing a document cannot show, said in three registers.
//
// IT USED TO BE A BARE STRING, and the reason it is not any more is that a
// reader met four of them at once -- 254 words between them on the FY2025-26
// spine, the longest 174 -- at the same altitude as the chart they qualify. A page can now show [Caveat.Summary] and link to
// [Caveat.Text] somewhere a reader goes when they want it. Nothing was
// shortened to achieve that: Text is the string that used to be the whole
// caveat, verbatim.
//
// THE ID IS A PUBLISHED URL FRAGMENT, which is why [ValidateCaveats] refuses a
// document that repeats one. Two caveats sharing an anchor is a link that lands
// on the wrong paragraph, and it fails silently -- the page renders, the anchor
// resolves, and the reader is shown a sentence about something else.
type Caveat struct {
	// ID is a stable slug. Stable is the load-bearing word: it outlives edits
	// to Summary and Text, because a URL somebody has bookmarked or cited must
	// not stop resolving because a sentence was reworded.
	ID string `json:"id"`
	// Summary is one line, written to be skimmed in a list of its peers. It is
	// authored beside Text rather than derived from it -- a first sentence is
	// not a summary, and truncating a paragraph produces neither.
	Summary string `json:"summary"`
	// Text is the caveat in full, and is the string this field replaced.
	Text string `json:"text"`
	// AppliesTo names the node ids this caveat is about, so a chart can mark
	// them. EMPTY MEANS DOCUMENT-WIDE, not "not filled in yet" -- most of these
	// are statements about what a schedule does not contain, which no single
	// node is responsible for.
	AppliesTo []string `json:"applies_to"`
}

// revenueSchedulePublishedTwiceCaveat is on every document that draws Budget
// Book pp.127-140, shared so the two cannot disagree.
func revenueSchedulePublishedTwiceCaveat() Caveat {
	return Caveat{
		ID: "the-revenue-schedule-is-published-twice",
		Summary: "Budget Book pp.127-140 are published twice on this site: the chart draws their " +
			"adopted columns one year at a time, and the Revenue tables print all four.",
		Text: "Budget Book pp.127-140 are published in two places on this site, and the two " +
			"are the same money rather than two figures. The site's chart draws ONE adopted " +
			"column of that schedule at a time, while the Revenue tables print all four " +
			"columns it carries: FY2023-24 actual, FY2024-25 revised, and both adopted years. " +
			"A row found in both places is one printed figure shown once in each, so neither " +
			"view is a second measurement of it and the two are never to be added.",
		// Document-wide: it is about the schedule, not any node.
		AppliesTo: []string{},
	}
}

// validateCaveats refuses a set no page could render honestly.
//
// IT RUNS AT BUILD TIME, in every document builder, because every failure below
// is invisible downstream: a missing id publishes an anchor of "", a missing
// summary publishes a blank line in a list, a repeated id publishes two
// paragraphs under one anchor of which a reader sees whichever the browser finds
// first, and an [Caveat.AppliesTo] entry naming nothing simply never marks
// anything. None of them stops a page rendering, so none would be found by a
// page that rendered.
//
// nodes IS THE DOCUMENT'S OWN NODE IDS, and passing it is what makes AppliesTo a
// checkable claim rather than a hopeful one. A mistyped id is the worst of the
// failures here precisely because it is the quietest: the chart draws, the
// caveat lists, and the mark it was written to flag is simply never flagged --
// a test asserting "this node carries no caveat" passes whether the id is wrong
// or the caveat genuinely does not apply. A caller with no nodes to offer --
// a document that is not a graph -- passes nil, and the arm is skipped rather
// than being made to fail on every entry.
func validateCaveats(caveats []Caveat, nodes map[string]struct{}) error {
	seen := make(map[string]struct{}, len(caveats))
	for i, c := range caveats {
		switch {
		case c.ID == "":
			return fmt.Errorf("caveat %d has no id, and an id is the anchor a page links to", i)
		case c.Summary == "":
			return fmt.Errorf("caveat %q has no summary, and a summary is what a page shows in place of the text", c.ID)
		case c.Text == "":
			return fmt.Errorf("caveat %q has no text, so its summary summarises nothing", c.ID)
		}
		if _, dup := seen[c.ID]; dup {
			return fmt.Errorf("caveat id %q is used twice in one document; an id is a published URL fragment and two paragraphs cannot share one", c.ID)
		}
		seen[c.ID] = struct{}{}
		if nodes == nil {
			continue
		}
		for _, target := range c.AppliesTo {
			if _, ok := nodes[target]; !ok {
				return fmt.Errorf("caveat %q applies to node %q, which this document does not carry; a caveat that marks nothing is worse than one that marks the wrong thing, because nothing goes red", c.ID, target)
			}
		}
	}
	return nil
}

// nodeIDs is the set [ValidateCaveats] checks AppliesTo against.
func nodeIDs(nodes []Node) map[string]struct{} {
	out := make(map[string]struct{}, len(nodes))
	for _, n := range nodes {
		out[n.ID] = struct{}{}
	}
	return out
}

// locatorSet collects the (doc_id, page) pairs of a set of facts.
//
// IT IS THE ONE GROUPING RULE IN THIS PACKAGE. Both sourcesOf (a whole
// document's citation) and every Link's Locators (one flow's) go through it,
// so the two cannot disagree about ordering, de-duplication or the empty case
// -- which matters because the client renders them with the same function, and
// a link whose list were shaped differently would compose a different URL for
// the same page.
type locatorSet struct {
	pages map[string]map[int]bool
}

// add records the page one fact was read from.
func (l *locatorSet) add(f *fact.Fact) {
	if l.pages == nil {
		l.pages = make(map[string]map[int]bool)
	}
	if l.pages[f.DocID] == nil {
		l.pages[f.DocID] = make(map[int]bool)
	}
	l.pages[f.DocID][f.Page] = true
}

// merge folds another set into this one. It is what a rollup link needs: the
// tier-3-to-4 flow in fundflows cites every fact of every cell beneath it, and
// its locators must be the union of theirs rather than the first one's.
func (l *locatorSet) merge(o *locatorSet) {
	for doc, ps := range o.pages {
		for p := range ps {
			if l.pages == nil {
				l.pages = make(map[string]map[int]bool)
			}
			if l.pages[doc] == nil {
				l.pages[doc] = make(map[int]bool)
			}
			l.pages[doc][p] = true
		}
	}
}

// sources renders the set as the published shape: documents ascending, pages
// ascending within each, each page once.
//
// It never returns nil. docs/sankey-contract.md requires every key present on
// every object with no null, and an empty locator list would otherwise encode
// as null the moment a caller built one from no facts.
func (l *locatorSet) sources() []Source {
	out := make([]Source, 0, len(l.pages))
	for doc, ps := range l.pages {
		nums := make([]int, 0, len(ps))
		for p := range ps {
			nums = append(nums, p)
		}
		sort.Ints(nums)
		out = append(out, Source{DocID: doc, Pages: nums})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].DocID < out[j].DocID })
	return out
}

// encode is [marshal] validated against the schema the caller names. It checks
// the bytes rather than the struct, which has lost the difference between an
// absent key and an empty one; internal/export decodes these documents by json
// tag alone, so a renamed tag would otherwise decode to zero in silence.
func encode(v any, name, schemaName string) ([]byte, error) {
	raw, err := marshal(v, name)
	if err != nil {
		return nil, err
	}
	var doc any
	if err = json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("re-read %s projection: %w", name, err)
	}
	resolved, err := schema.Load(schemaName)
	if err != nil {
		return nil, fmt.Errorf("loading %s: %w", schemaName, err)
	}
	if err = resolved.Validate(doc); err != nil {
		return nil, fmt.Errorf("the %s projection does not match %s: %w", name, schemaName, err)
	}
	return raw, nil
}

// marshal is the canonical JSON every projection publishes: HTML escaping off,
// so "Fines & Forfeitures" stays readable, a two-space indent and one trailing
// newline. name is the projection's, so a failure names the document.
func marshal(v any, name string) ([]byte, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return nil, fmt.Errorf("encode %s projection: %w", name, err)
	}
	return b.Bytes(), nil
}

// Envelope is the leading block of a series document's metadata: the trends
// and the two ACFR histories, each of one schedule. A graph's block is
// [Metadata], which spells the same four keys with scopes as a list.
type Envelope struct {
	// GeneratedBy is build.Get().String(), so a reader can tell which binary
	// wrote the file.
	GeneratedBy string `json:"generated_by"`
	// Scope is the schedule the facts came from. Every document is of exactly
	// one, which is what keeps two schedules' figures out of one total.
	Scope string `json:"scope"`
	// Currency and Units are stated rather than assumed. Money is an integer
	// count of cents everywhere in this project, and a document that does not
	// say so is one a reader has to guess about.
	Currency string `json:"currency"`
	Units    string `json:"units"`
}

// envelope fills the block from the options a series document was built
// under. Scope is singular and [Options.Scopes] is not: writing Scopes[0] into
// it would publish one schedule as the whole of a document built over two, so
// a set of any other size is refused.
func envelope(o Options) (Envelope, error) {
	scope, err := o.onlyScope()
	if err != nil {
		return Envelope{}, err
	}
	return Envelope{
		GeneratedBy: o.Version,
		Scope:       scope,
		Currency:    "USD",
		Units:       "cents",
	}, nil
}
