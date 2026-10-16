package export

import "github.com/jcrussell/livermore-budget/internal/project"

// wording is every sentence site/app.js composes about the chart on screen,
// as templates the client fills in: the counts line, the chart hint and the
// chip row above the chart. `{name}` is a variable's value and
// `{name:one|many}` is the value followed by the singular or the plural word,
// by whether the value is 1.
//
// A sentence carrying a count the client takes carries the mark ◇ with it,
// or says the count is the reader's ("your filters"): a count of what is
// drawn is the page's sentence, never a figure the city printed.
//
// Declared here and formatted there, so the words are the packager's and the
// numbers the chart's.
type wording struct {
	Counts        string `json:"counts"`
	CountsPartial string `json:"counts_partial"`
	OpenInto      string `json:"open_into"`
	NothingOpens  string `json:"nothing_opens"`
	// FoldBack is said beside a chart with a node split open: the ways to
	// fold one back.
	FoldBack string `json:"fold_back"`
	// Follow is said beside every chart: what the pointer's trace, a click on
	// a node that does not open, the drawer's two buttons and Escape do.
	Follow string `json:"follow"`
	Expand string `json:"expand"`
	Swatch string `json:"swatch"`
	// The glyph beside a node: GlyphOpen names the ⊕ that splits a node one
	// way, GlyphChoose the ⊕ that offers a node's ways, and GlyphFold the ⊖
	// that folds a split node back.
	GlyphOpen   string `json:"glyph_open"`
	GlyphChoose string `json:"glyph_choose"`
	GlyphFold   string `json:"glyph_fold"`
	// The chip row above the chart: one chip per node split open, the chooser
	// beside a node that splits more than one way, a tail drawn out, and the
	// two controls over every split at once.
	ChipFold      string `json:"chip_fold"`
	ChipFoldLabel string `json:"chip_fold_label"`
	SplitChooser  string `json:"split_chooser"`
	TailChip      string `json:"tail_chip"`
	TailChipLabel string `json:"tail_chip_label"`
	ExpandAll     string `json:"expand_all"`
	CollapseAll   string `json:"collapse_all"`
	PrintedByCity string `json:"printed_by_city"`
	InferredByUs  string `json:"inferred_by_us"`
	OurInference  string `json:"our_inference"`
	InferredChip  string `json:"inferred_chip"`
	PrintedChip   string `json:"printed_chip"`
	CarriedNote   string `json:"carried_note"`
	CarriedChip   string `json:"carried_chip"`
	// RepeatsNote and RepeatsChip are said of a link drawing a leaf another
	// drawn link draws too: a figure the city printed between other ends,
	// repeated through a hub a split opened.
	RepeatsNote string `json:"repeats_note"`
	RepeatsChip string `json:"repeats_chip"`
	// CaveatChip is said beside a mark's figure: how many caveats are about
	// the mark.
	CaveatChip string `json:"caveat_chip"`
	// InferredFlows is said under a mark of ours in the list of what we
	// inferred: the inferred flows it is listed with, their sum and their ends.
	InferredFlows string `json:"inferred_flows"`
	DescOpens     string `json:"desc_opens"`
	// DescFolds is said of an open hub: the activation that opened it folds
	// it back.
	DescFolds    string `json:"desc_folds"`
	DescExpands  string `json:"desc_expands"`
	DescFollows  string `json:"desc_follows"`
	FlowInferred string `json:"flow_inferred"`
	NoneInferred string `json:"none_inferred"`
	TablePointer string `json:"table_pointer"`
	// The aggregate marks, schema/mark.schema.json: their
	// labels, rationales and source notes are templates the constructors
	// fill, so no sentence about what a mark IS is spelled in JavaScript.
	AggregateLabel     string `json:"aggregate_label"`
	AggregateRationale string `json:"aggregate_rationale"`
	AggregateNote      string `json:"aggregate_note"`
	AggregateTogether  string `json:"aggregate_together"`
	// ContraOrphan is project.ContraOrphan, the sentence a reduction carries
	// when no printed line explains it, shipped so a fold that nets a ribbon
	// negative names it in the producer's words.
	ContraOrphan string `json:"contra_orphan"`
	// A fund's printed balances, schema/column.schema.json's balances, each
	// said beside the node's figure with the fact and pages it cites.
	BalanceBeginning string `json:"balance_beginning"`
	BalanceEnding    string `json:"balance_ending"`
	// BalanceChange says Go's derived change beside the two it is taken from.
	BalanceChange string `json:"balance_change"`
	// The filter panel's sentences: what the filters leave out of the chart,
	// said beside it as a client sum over cited cents, and the smaller-flows
	// mark and merged ribbon the smallest-flow filter draws.
	FilterHidden   string `json:"filter_hidden"`
	FilterFolded   string `json:"filter_folded"`
	FilterHiddenAt string `json:"filter_hidden_at"`
	// FilterDrawnAt is said beside a node's figure, which is the view's
	// whatever the filters leave drawn, where they leave part of it.
	FilterDrawnAt    string `json:"filter_drawn_at"`
	SmallestAll      string `json:"smallest_all"`
	SmallerLabel     string `json:"smaller_label"`
	SmallerRationale string `json:"smaller_rationale"`
	SmallerNote      string `json:"smaller_note"`
	SmallerFlow      string `json:"smaller_flow"`
	// The search box over the marks of the chart drawn: its label, what it
	// found, and what Enter does to an option beyond following it.
	SearchLabel   string `json:"search_label"`
	SearchCount   string `json:"search_count"`
	SearchNone    string `json:"search_none"`
	SearchOpens   string `json:"search_opens"`
	SearchExpands string `json:"search_expands"`
	// SearchReveals is what Enter does on a search option the chart does not
	// draw: it splits the nodes above it until it is drawn.
	SearchReveals string `json:"search_reveals"`
	// The full-screen control on the chart's card, in each of its two states.
	FullScreen     string `json:"full_screen"`
	FullScreenExit string `json:"full_screen_exit"`
	// ZoomReset is the control that puts a zoomed or panned chart back whole.
	ZoomReset string `json:"zoom_reset"`
	// Rail names the toggle that collapses and expands the rail beside the
	// chart holding the legend, the filters and the key.
	Rail string `json:"rail"`
	// Drawer names the provenance drawer below the chart; DetailEmpty is
	// what it says with nothing previewed or pinned, and Pin its toggle
	// that pins what it shows or lets it go.
	Drawer      string `json:"drawer"`
	DetailEmpty string `json:"detail_empty"`
	Pin         string `json:"pin"`
	// FollowMoney names the drawer's toggle that follows the money of the
	// node it shows, and puts the rest back.
	FollowMoney string `json:"follow_money"`
	// SourcesChart heads the drawer's sources for a node that draws no
	// ribbon: the pages of the whole chart drawn, not pages of its own.
	SourcesChart string `json:"sources_chart"`
	// Hairline is said beside a chart drawing ribbons under a pixel of its
	// canvas: they are drawn a screen pixel wide, which overstates them, and
	// the sentence says so.
	Hairline string `json:"hairline"`
	// SplitsDropped is said when a year switch folds back the splits of
	// nodes the new year's expansion schedule does not carry.
	SplitsDropped string `json:"splits_dropped"`
}

// splitLabels is the words for what a node splits into, keyed by every split
// key the expansion schedule declares ([project.SplitKeys]): what the chip row
// and the chooser call a node's members.
func splitLabels() map[string]string {
	return map[string]string{
		"lines":       "its lines",
		"payers":      "the funds that pay it",
		"funds":       "its funds",
		"divisions":   "its divisions",
		"object-rows": "its object rows",
		"receivers":   "the funds it pays",
	}
}

// memberNouns is what one member of each split key is called, keyed by every
// split key the expansion schedule declares ([project.SplitKeys]): the noun a
// member drawn beside a parent the city prints in the same words is told
// apart by. `{fund}` is the member's column node's fund number.
func memberNouns() map[string]string {
	return map[string]string{
		"lines":       "line",
		"payers":      "fund {fund}",
		"funds":       "fund {fund}",
		"divisions":   "division",
		"object-rows": "object row",
		"receivers":   "fund {fund}",
	}
}

// kindLabels is project's words for every link kind, keyed as a link names it.
func kindLabels() map[string]string {
	out := make(map[string]string, len(project.LinkKinds()))
	for _, k := range project.LinkKinds() {
		out[string(k)] = project.LinkKindLabel(k)
	}
	return out
}

// flowTableHeading is the flow table's heading. The template renders it and
// TablePointer quotes it, so a chart's description cannot point a screen
// reader at a heading the page no longer prints.
const flowTableHeading = "Every flow, as a table"

// defaultWording is the site's English. The counts sentence's head is also
// rendered server-side by site/index.html.tmpl for the page before app.js
// runs, and a test holds that line to Counts.
func defaultWording() wording {
	return wording{
		Counts:        "\u25c7 our count: {links:flow|flows} between {nodes:node|nodes}, from {facts:fact|facts}",
		CountsPartial: "\u25c7 our count: {links:flow|flows} between {nodes:node|nodes}, from {cited} of the document's {facts:fact|facts}",
		OpenInto: "Click a node marked \u2295, or tab to it and press Enter, to split it in place into its parts; " +
			"every other node stays where it is.",
		NothingOpens: "Nothing on this chart opens.",
		FoldBack:     "A second click on a split node, \u2296 beside it, or its chip above the chart folds it back, and Back undoes the last step.",
		Follow: "Hover a node or a flow to trace its path; a click on one that does not split pins it in the drawer, " +
			"whose Pin lets it go and whose Follow this money draws only its money. Escape unpins and puts the rest back.",
		Expand:         "The folded mark is several of them drawn as one; click it, or tab to it and press Enter, to draw them separately.",
		Swatch:         "A fund swatch follows one group's money without opening anything.",
		GlyphOpen:      "Split {label} into {split}",
		GlyphChoose:    "Choose what to split {label} into",
		GlyphFold:      "Fold {label} back into one mark",
		ChipFold:       "{label} \u25be",
		ChipFoldLabel:  "{label}, split into {split}: fold it back into one mark",
		SplitChooser:   "Split {label} into",
		TailChip:       "{label}: showing all \u25c7 {n} {word} \u00d7",
		TailChipLabel:  "{label}: showing all {n} {word}, by our count; fold the smallest back into one mark",
		ExpandAll:      "Expand all",
		CollapseAll:    "Collapse all",
		PrintedByCity:  "printed by the city",
		InferredByUs:   "inferred by us",
		OurInference:   "◇ our inference",
		InferredChip:   "◇ inferred",
		PrintedChip:    "printed",
		CarriedNote:    "figure printed by the city, re-pointed onto a mark of ours",
		CarriedChip:    "◇ re-pointed by us",
		RepeatsNote:    "figure printed by the city between other ends, repeated by us through a split",
		RepeatsChip:    "◇ a printed figure, repeated through a split",
		CaveatChip:     "\u26a0 \u25c7 {n:caveat|caveats}",
		InferredFlows:  "\u25c7 {flows:inferred flow|inferred flows} totalling {figure}: {ends}",
		DescOpens:      ", splits in place into its parts on a click, Enter or Space, and is pinned in the drawer",
		DescFolds:      ", folds back into one mark on a click, Enter or Space, and stays pinned in the drawer",
		DescExpands:    ", draws all of them separately on a click, Enter or Space",
		DescFollows:    ", a click, Enter or Space pins it in the drawer",
		FlowInferred:   "This flow is inferred; both endpoints are printed by the city.",
		NoneInferred:   "Nothing on this chart is inferred: every node and flow is printed by the city.",
		TablePointer:   "The same figures are in the flow table below, which opens from the \"" + flowTableHeading + "\" heading.",
		AggregateLabel: "{folded} smaller {word}",
		AggregateRationale: "Our grouping, not a line the city printed: the {folded} smallest {word} in this column are drawn as one " +
			"mark because they cannot be drawn separately. Every figure inside it is printed; the box around them is ours.",
		AggregateNote:     "The {folded} smallest printed rows of {total} by value, at this page's cap of {cap}",
		AggregateTogether: ", together {figure}.",
		ContraOrphan:      project.ContraOrphan,
		BalanceBeginning:  "Balance at the start of the year, as printed: {figure}",
		BalanceEnding:     "Balance at the end of the year, as printed: {figure}",
		BalanceChange:     "Change in balance, the end of the year's less the start's: {figure}",
		FilterHidden:      "Hidden by your filters: {cents} in {flows:flow|flows}. Marks are sized to what is drawn, each mark's figure is still this view's, and the flow table still lists every flow of this view.",
		FilterFolded:      "\u25c7 {flows:flow|flows} under {threshold} are drawn merged by this page into {ribbons:ribbon|ribbons} of smaller flows.",
		FilterHiddenAt:    "Your filters hide {cents} of this mark's flows, in {flows:flow|flows}.",
		FilterDrawnAt:     "{cents} of it drawn under your filters",
		SmallestAll:       "every flow",
		SmallerLabel:      "{flows:flow|flows} under {threshold}",
		SmallerRationale:  "Our grouping, not a line the city printed: {flows:flow|flows} each under {threshold}, the smallest flow your filters draw as itself, are drawn as one mark. Every figure inside it is printed; the merge is this page's.",
		SmallerNote:       "Merged by this page from {flows:flow|flows} the city printed, together {figure}; its ribbons cite every fact and page they merge.",
		SmallerFlow:       "{flows:flow|flows} under {threshold}, each printed by the city, merged by this page",
		SearchLabel:       "Find a mark on this chart",
		SearchCount:       "\u25c7 {n:mark matches|marks match}; the arrow keys move between them, and Enter goes to one.",
		SearchNone:        "No mark drawn on this chart has “{query}” in its name.",
		SearchOpens:       "Enter splits it in place",
		SearchExpands:     "Enter draws out every one it folds",
		SearchReveals:     "Enter splits the chart until it is drawn",
		FullScreen:        "Full screen",
		FullScreenExit:    "Exit full screen",
		ZoomReset:         "Reset view",
		Rail:              "Legend and filters",
		Drawer:            "Where a figure comes from",
		DetailEmpty:       "Hover a flow or a node to trace its path and preview where its figure comes from; click it to pin it here.",
		Pin:               "Pin",
		FollowMoney:       "Follow this money",
		SourcesChart:      "This mark draws no flow, so these are the sources of the whole chart, not of it:",
		Hairline: "\u25c7 {n:flow is|flows are} under a pixel wide on this chart's canvas, and each is drawn as a " +
			"hairline one screen pixel wide, which is wider than its share; every other flow is drawn to scale. " +
			"Zoom in, go full screen, search, or open the table below to read them.",
		SplitsDropped: "{year} has no {nodes} to split, so \u25c7 {n:split was|splits were} folded back.",
	}
}
