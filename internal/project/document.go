package project

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/jcrussell/livermore-budget/internal/beadid"
	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/hint"
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
	// Splits is the expansion schedule's alone: what each node splits into.
	Splits []Split `json:"splits,omitempty"`
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
	// FactsCited is how many DISTINCT facts some link or node balance
	// carries.
	FactsCited int `json:"facts_cited"`
	// FactsUncited is the facts nothing carries: a printed zero, or on the
	// spine a stock row. Every builder refuses anything else at build.
	FactsUncited int `json:"facts_uncited"`
	// FactsCitedTwice is how many distinct facts are behind MORE THAN ONE
	// link. Zero on a document whose links partition its facts; on the
	// drill-down, where both sides carry a summing link above the cell, it
	// warns that summing every link's value_cents double-counts.
	FactsCitedTwice int `json:"facts_cited_twice"`
	Nodes           int `json:"nodes"`
	Links           int `json:"links"`
}

// tally is how the links and the nodes' balances account for the selected
// facts: the counts a document publishes, and the facts neither carries, in
// selection order, for the builder to hold to its own rule about what may go
// uncited. A balance cites its fact as a link does; only links count toward
// facts_cited_twice, which warns about summing value_cents.
func tally(selected []fact.Fact, links []Link, nodes []Node) (Counts, []*fact.Fact) {
	times := map[string]int{}
	for _, l := range links {
		for _, id := range l.FactIDs {
			times[id]++
		}
	}
	onNode := map[string]bool{}
	for _, n := range nodes {
		for _, id := range n.Balances.FactIDs() {
			onNode[id] = true
		}
	}
	c := Counts{Facts: len(selected), Nodes: len(nodes), Links: len(links)}
	var uncited []*fact.Fact
	for i := range selected {
		switch n := times[selected[i].ID]; {
		case n == 0 && onNode[selected[i].ID]:
			c.FactsCited++
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

// refuseUncited holds every document to its uncited facts: a fact no link
// carries is a printed zero, or the document has dropped money
// in silence and its own counts cannot show it. allow names the one other
// shape a builder admits, or is nil.
func refuseUncited(name string, uncited []*fact.Fact, allow func(*fact.Fact) bool) error {
	for _, f := range uncited {
		if f.AmountCents == 0 || (allow != nil && allow(f)) {
			continue
		}
		return hint.With(
			fmt.Errorf("%s: fact %s is carried by no link and is not a printed zero", name, f.ID),
			"facts = facts_cited + facts_uncited is every document's published identity "+
				"and every uncited fact is a cell the city printed as nothing; a fact reaching "+
				"no link for another reason is money dropped in silence")
	}
	return nil
}

// drawGraph sorts links in place and nodes by (tier, id), refusing two links
// of one kind between one pair.
func drawGraph(nodes map[string]Node, links []Link) ([]Node, error) {
	sortLinks(links)
	if err := checkDistinctLinks(links); err != nil {
		return nil, err
	}
	return sortedNodes(nodes), nil
}

// graphDocument is the tail every graph projection shares, over nodes and
// links already sorted: the counts, the refusal of an uncited fact allow does
// not admit, the caveats validated against the nodes this column drew, and
// the metadata of the one column o selects.
func graphDocument(name string, o Options, scopes []string, selected []fact.Fact, nodes []Node,
	links []Link, allow func(*fact.Fact) bool, caveats func(Counts) []Caveat) (*Document, error) {
	c, uncited := tally(selected, links, nodes)
	if err := refuseUncited(name, uncited, allow); err != nil {
		return nil, err
	}
	col := o.Columns[0]
	cavs := caveats(c)
	if err := validateCaveats(cavs, nodeIDs(nodes)); err != nil {
		return nil, fmt.Errorf("%s: %w", col, err)
	}
	return &Document{
		SchemaVersion: SchemaVersion,
		Projection:    name,
		Metadata: Metadata{
			GeneratedBy:     o.Version,
			FiscalYear:      col.FiscalYear,
			FiscalYearLabel: fiscalYearLabel(col.FiscalYear),
			Basis:           string(col.Basis),
			Scopes:          scopes,
			Currency:        "USD",
			Units:           "cents",
			Sources:         SourcesOf(selected),
			Counts:          c,
			Caveats:         cavs,
		},
		Nodes: nodes,
		Links: links,
	}, nil
}

// encodeGraph is a graph projection's Build: its Document, encoded and held to
// the projection schema.
func encodeGraph(p interface {
	Name() string
	Document([]fact.Fact, Options) (*Document, error)
}, facts []fact.Fact, o Options) ([]byte, error) {
	doc, err := p.Document(facts, o)
	if err != nil {
		return nil, err
	}
	return encode(doc, p.Name(), schema.Projection)
}

// Source is one document the facts came from, with the pages actually read.
type Source struct {
	DocID string `json:"doc_id"`
	Pages []int  `json:"pages"`
}

// Caveat is one thing a document cannot show, said in three registers: a
// [Caveat.Summary] a page lists, a [Caveat.Text] the caveats page prints, and
// a [Caveat.Theme] that page groups it under.
//
// The id is a published URL fragment, so a document may not repeat one, and
// one id is one heading across every document: its Summary and Theme must not
// vary by year or schedule. The export refuses an id that breaks this.
type Caveat struct {
	// ID is a stable slug that outlives edits to Summary and Text.
	ID string `json:"id"`
	// Summary is one line, authored beside Text, not derived from it.
	Summary string `json:"summary"`
	// Text is the caveat in full.
	Text string `json:"text"`
	// Theme is which kind of limit this is, one of [CaveatThemes].
	Theme CaveatTheme `json:"theme"`
	// AppliesTo names the node ids this caveat is about, so a chart can mark
	// them. Empty means document-wide.
	AppliesTo []string `json:"applies_to"`
}

// CaveatTheme is the kind of limit a caveat states, so a reader can take in
// the kinds before the instances.
type CaveatTheme string

const (
	// ThemeBooksDisagree is two of the city's own pages printing different
	// figures for one thing.
	ThemeBooksDisagree CaveatTheme = "books-disagree"
	// ThemeLeftOut is money or a column the drawn schedule does not carry.
	ThemeLeftOut CaveatTheme = "left-out"
	// ThemeSameMoneyTwice is one printed figure a reader could add twice.
	ThemeSameMoneyTwice CaveatTheme = "same-money-twice"
	// ThemeKindsOfYear is a column that is not the same kind of number as its
	// neighbours: actual, revised, adopted, projected, unaudited.
	ThemeKindsOfYear CaveatTheme = "kinds-of-year"
	// ThemeOurReading is a classification or label this site chose.
	ThemeOurReading CaveatTheme = "our-reading"
	// ThemeChecked is a check that passed. It qualifies nothing, so a page's
	// caveat line does not count it as a limit, and it may name no node.
	ThemeChecked CaveatTheme = "checked"
	// ThemeDataFiles matters only to someone summing a published file. No
	// page states it and no chart marks it, so it may name no node.
	ThemeDataFiles CaveatTheme = "data-files"
)

// caveatThemeWords is each theme as the caveats page heads and explains it.
var caveatThemeWords = map[CaveatTheme][2]string{
	ThemeBooksDisagree: {"Where the city's own books disagree",
		"Two of the city's pages print different figures or labels for the same thing, by anything from a dollar to hundreds of thousands. This site draws one as printed and says which; it corrects neither."},
	ThemeLeftOut: {"What isn't in the picture",
		"Money or columns a schedule doesn't carry, or that this site reads and doesn't draw. A total here can be smaller than one printed elsewhere for that reason alone."},
	ThemeSameMoneyTwice: {"Don't add or compare these",
		"Figures that look as if they should be added together or set side by side, and should not: the same dollars shown twice, or two different measures under similar names."},
	ThemeKindsOfYear: {"Not every year is the same kind of number",
		"Actual, revised, adopted, projected and unaudited columns measure different things. A change from one to another isn't growth."},
	ThemeOurReading: {"Where this site, not the city, decided",
		"A label, a grouping or a classification this site chose to draw the city's figures. The figures are printed; these choices are ours."},
	ThemeChecked: {"What we checked and found to agree",
		"Not limits: places where one schedule was held against another and they agree."},
	ThemeDataFiles: {"For anyone using the data files",
		"Only matters if you add up the figures in a downloaded file. No page draws them this way."},
}

// CaveatThemes is the closed set of caveat themes, in the order the caveats
// page lists them.
func CaveatThemes() []CaveatTheme {
	return []CaveatTheme{ThemeBooksDisagree, ThemeLeftOut, ThemeSameMoneyTwice,
		ThemeKindsOfYear, ThemeOurReading, ThemeChecked, ThemeDataFiles}
}

// CaveatThemeHeading is a theme as the caveats page heads it, "" for no theme
// of [CaveatThemes].
func CaveatThemeHeading(t CaveatTheme) string { return caveatThemeWords[t][0] }

// CaveatThemeBlurb is a theme in a sentence or two, "" for no theme of
// [CaveatThemes].
func CaveatThemeBlurb(t CaveatTheme) string { return caveatThemeWords[t][1] }

// revenueSchedulePublishedTwiceCaveat is on every document that draws Budget
// Book pp.127-140, shared so the two cannot disagree.
func revenueSchedulePublishedTwiceCaveat() Caveat {
	return Caveat{
		ID:      "the-revenue-schedule-is-published-twice",
		Theme:   ThemeSameMoneyTwice,
		Summary: "Revenue the charts draw is the same money the Over time page prints; don't add the two.",
		Text: "Budget Book pp.127-140 appear in two places on this site: the charts draw one " +
			"adopted year at a time, and the Over time page prints all four columns (FY2023-24 " +
			"actual, FY2024-25 revised, and both adopted years). A row found in both is one " +
			"printed figure shown twice.",
		// Document-wide: it is about the schedule, not any node.
		AppliesTo: []string{},
	}
}

// validateCaveats refuses a set no page could render honestly, at build time,
// since none of its failures stops a page rendering: a missing id or summary,
// a repeated id, a tracker id in the prose, an [Caveat.AppliesTo] entry naming
// no node. nodes is the document's own node ids; a document that is not a
// graph passes nil and that arm is skipped.
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
		case CaveatThemeHeading(c.Theme) == "":
			return fmt.Errorf("caveat %q has theme %q, which is none of CaveatThemes; the caveats page lists a caveat under its theme", c.ID, c.Theme)
		case (c.Theme == ThemeDataFiles || c.Theme == ThemeChecked) && len(c.AppliesTo) > 0:
			return fmt.Errorf("caveat %q is of theme %s and names nodes; no page counts it as a limit, so a chart must not mark one", c.ID, c.Theme)
		}
		for _, prose := range []string{c.Summary, c.Text} {
			if ids := beadid.Cited(prose); len(ids) > 0 {
				return fmt.Errorf("caveat %q prints the tracker id %q; a reader cannot open the tracker, so say what it decided instead", c.ID, ids[0])
			}
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
