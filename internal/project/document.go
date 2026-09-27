package project

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/schema"
)

// This file holds the parts of a published document that are NOT the Sankey's.
//
// Source, Counts and the encoder below were declared in sankey.go, beside the
// only projection that existed. They are here because the next projection needs
// them and inheriting them from a graph would be the wrong relationship: a
// document that cites pages and accounts for facts is every projection, and a
// document with nodes and links is one of them.
//
// WHAT DELIBERATELY DID NOT MOVE, because it is not shared and pretending
// otherwise would cost more than it saves:
//
//   - Metadata carries FiscalYear, FiscalYearLabel and Basis, all singular, and
//     a Headline. A trends document spans four fiscal years and four bases, so
//     it can fill none of them; hoisting Metadata would hand every future
//     document a fiscal_year it has to leave wrong or blank. Absent is not zero
//     here either.
//   - Headline's keys are all_funds_gross_revenue_cents and
//     naive_expenditure_cents. They are the spine's, and they mean nothing
//     anywhere else.
//
// This move changes no bytes. encoding/json emits a struct's fields in
// declaration order, and which FILE a type is declared in is not part of that,
// so testdata/sankey.golden.json is the proof the extraction was faithful --
// internal/project/sankey_test.go compares it with bytes.Equal.

// counts is how much of the corpus this document accounts for.
type counts struct {
	// Facts is how many facts matched the options, which is NOT how many links
	// were drawn: stocks get no link, and neither do zero-valued cells. The
	// gap between Facts and Links is the part of the schedule the chart cannot
	// show, and stating both is what makes it visible.
	Facts int `json:"facts"`
	// FactsCited is how many of those facts a link actually carries. Facts
	// minus FactsCited is exactly the zero-valued cells plus the stock rows,
	// which makes the gap a quantity a check can assert rather than a
	// discrepancy a reader has to explain to themselves.
	FactsCited int `json:"facts_cited"`
	Nodes      int `json:"nodes"`
	Links      int `json:"links"`
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

// Envelope is what every document of this project carries, and it is the
// leading block of a non-spine document's metadata.
//
// THE SANKEY DOES NOT EMBED IT, and the reason is bytes rather than taste.
// encoding/json emits fields in declaration order, and Metadata's order is
// generated_by, fiscal_year, fiscal_year_label, basis, scope, currency, units,
// sources, headline, counts, caveats -- the shared fields are INTERLEAVED with
// the spine's own. Embedding this type there would move scope, currency and
// units up beside generated_by and change testdata/sankey.golden.json, which is
// the frozen contract. So the two structs share field names without sharing a
// declaration, which is fisc-2u4's option (a), and
// TestSharedMetadataTagsHaveNotDrifted is what couples them instead.
//
// It is a type rather than four repeated fields because the next document after
// the trends should not have to re-derive which four are common.
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

// envelope fills the block from the options a document was built under.
//
// IT RETURNS AN ERROR BECAUSE Scope IS SINGULAR AND [Options.Scopes] IS NOT.
// Envelope.Scope's own doc comment says every document carrying one is of
// exactly one schedule, and that stays true -- but the options handed here can
// now name two, and writing Scopes[0] into a singular key would publish one
// schedule as the whole of a document built over both. A multi-schedule
// document needs a metadata block that says so; it does not get to borrow this
// one and lose half the claim on the way.
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
