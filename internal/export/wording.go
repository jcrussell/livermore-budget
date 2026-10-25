package export

import "github.com/jcrussell/livermore-budget/internal/project"

// wording is every sentence site/app.js composes about the chart, as
// templates the client fills: `{name}` is a value and `{name:one|many}` the
// value followed by the singular or plural word. A sentence carrying a count
// the client takes carries ◇: it is the page's sentence, never a printed
// figure. The words are the packager's and the numbers the chart's.
type wording struct {
	Counts        string `json:"counts"`
	CountsPartial string `json:"counts_partial"`
	OpenInto      string `json:"open_into"`
	NothingOpens  string `json:"nothing_opens"`
	FoldBack      string `json:"fold_back"`
	Follow        string `json:"follow"`
	Expand        string `json:"expand"`
	Swatch        string `json:"swatch"`
	// GlyphOpen and GlyphChoose name a node's ⊕ (one way, or a choice of
	// ways, in the SplitChooser menu); GlyphFold its ⊖.
	GlyphOpen    string `json:"glyph_open"`
	GlyphChoose  string `json:"glyph_choose"`
	GlyphFold    string `json:"glyph_fold"`
	SplitChooser string `json:"split_chooser"`
	// NodeMenu names a node's context menu; FollowMoney, FilterOutNode and
	// ShowOnlyNode are its choices.
	NodeMenu      string `json:"node_menu"`
	NodeMenuMore  string `json:"node_menu_more"`
	FilterOutNode string `json:"filter_out_node"`
	ShowOnlyNode  string `json:"show_only_node"`
	// RefusedPart, RefusedAloneEmpty and RefusedOther are said under a node
	// menu choice the redraw would refuse; RedrawPart and RedrawAloneEmpty
	// follow any redraw's refusal for the same reasons.
	RefusedPart       string `json:"refused_part"`
	RefusedAloneEmpty string `json:"refused_alone_empty"`
	RefusedOther      string `json:"refused_other"`
	RedrawPart        string `json:"redraw_part"`
	RedrawAloneEmpty  string `json:"redraw_alone_empty"`
	ExpandAll         string `json:"expand_all"`
	CollapseAll       string `json:"collapse_all"`
	PrintedByCity     string `json:"printed_by_city"`
	InferredByUs      string `json:"inferred_by_us"`
	OurInference      string `json:"our_inference"`
	InferredChip      string `json:"inferred_chip"`
	PrintedChip       string `json:"printed_chip"`
	CarriedNote       string `json:"carried_note"`
	CarriedChip       string `json:"carried_chip"`
	// RepeatsNote and RepeatsChip are said of a printed figure repeated
	// through a hub a split opened.
	RepeatsNote  string `json:"repeats_note"`
	RepeatsChip  string `json:"repeats_chip"`
	CaveatChip   string `json:"caveat_chip"`
	DescOpens    string `json:"desc_opens"`
	DescFolds    string `json:"desc_folds"`
	DescExpands  string `json:"desc_expands"`
	DescFollows  string `json:"desc_follows"`
	FlowInferred string `json:"flow_inferred"`
	// The aggregate marks' words (schema/mark.schema.json), so no sentence
	// about what a mark IS is spelled in JavaScript.
	AggregateLabel     string `json:"aggregate_label"`
	AggregateRationale string `json:"aggregate_rationale"`
	AggregateNote      string `json:"aggregate_note"`
	AggregateTogether  string `json:"aggregate_together"`
	// ContraOrphan is project.ContraOrphan, for a fold that nets a ribbon
	// negative.
	ContraOrphan     string `json:"contra_orphan"`
	BalanceBeginning string `json:"balance_beginning"`
	BalanceEnding    string `json:"balance_ending"`
	// BalanceChange says Go's derived change beside the two it is taken from.
	BalanceChange string `json:"balance_change"`
	// The filter panel's sentences, each a client sum over cited cents.
	FilterHidden     string `json:"filter_hidden"`
	FilterFolded     string `json:"filter_folded"`
	FilterHiddenAt   string `json:"filter_hidden_at"`
	FilterOnly       string `json:"filter_only"`
	FilterOut        string `json:"filter_out"`
	FilterDrawnAt    string `json:"filter_drawn_at"`
	SmallestAll      string `json:"smallest_all"`
	SmallerLabel     string `json:"smaller_label"`
	SmallerRationale string `json:"smaller_rationale"`
	SmallerNote      string `json:"smaller_note"`
	SmallerFlow      string `json:"smaller_flow"`
	SearchLabel      string `json:"search_label"`
	SearchCount      string `json:"search_count"`
	SearchNone       string `json:"search_none"`
	SearchOpens      string `json:"search_opens"`
	SearchExpands    string `json:"search_expands"`
	SearchReveals    string `json:"search_reveals"`
	FullScreen       string `json:"full_screen"`
	FullScreenExit   string `json:"full_screen_exit"`
	// ZoomReset puts the chart back as its year opens.
	ZoomReset   string `json:"zoom_reset"`
	Rail        string `json:"rail"`
	Drawer      string `json:"drawer"`
	DetailEmpty string `json:"detail_empty"`
	Pin         string `json:"pin"`
	FollowMoney string `json:"follow_money"`
	// SourcesChart heads the whole chart's sources, for a node drawing no
	// ribbon.
	SourcesChart string `json:"sources_chart"`
	// Hairline says ribbons under a pixel are drawn wider than their share.
	Hairline string `json:"hairline"`
	// SplitsDropped is said when a year switch folds back splits the new
	// year's expansion schedule does not carry.
	SplitsDropped string `json:"splits_dropped"`
}

// splitLabels is what the glyph and the chooser call a node's members, keyed
// by every [project.SplitKeys].
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

// memberNouns is what one member of each [project.SplitKeys] is called, to
// tell it apart from a parent printed in the same words. `{fund}` is the
// member's fund number.
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

// defaultWording is the site's English. The counts sentence's head is also
// rendered server-side by site/flows.html.tmpl for the page before app.js
// runs, and a test holds that line to Counts.
func defaultWording() wording {
	return wording{
		Counts:        "\u25c7 our count: {links:flow|flows} between {nodes:node|nodes}, from {facts:fact|facts}",
		CountsPartial: "\u25c7 our count: {links:flow|flows} between {nodes:node|nodes}, from {cited} of the document's {facts:fact|facts}",
		OpenInto: "Click a node marked \u2295, or tab to it and press Enter, to split it in place into its parts; " +
			"every other node stays where it is.",
		NothingOpens:   "Nothing on this chart opens.",
		FoldBack:       "A second click on a split node, or \u2296 beside it, folds it back; Collapse all folds every split, and Back undoes the last step.",
		Follow:         "Escape unpins and puts the rest back. Right-click a node, press M on it, or choose More\u2026 here, to follow its money, filter it out or show only it.",
		Expand:         "The folded mark is several of them drawn as one; click it, or tab to it and press Enter, to draw them separately.",
		Swatch:         "A fund swatch follows one group's money without opening anything.",
		GlyphOpen:      "Split {label} into {split}",
		GlyphChoose:    "Choose what to split {label} into",
		GlyphFold:      "Fold {label} back into one mark",
		SplitChooser:   "Split {label} into",
		NodeMenu:       "What to do with {label}",
		NodeMenuMore:   "More\u2026",
		FilterOutNode:  "Filter this out",
		ShowOnlyNode:   "Show only this",
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
		DescOpens:      ", splits in place into its parts on a click, Enter or Space, and is pinned in the drawer",
		DescFolds:      ", folds back into one mark on a click, Enter or Space, and stays pinned in the drawer",
		DescExpands:    ", draws all of them separately on a click, Enter or Space",
		DescFollows:    ", a click, Enter or Space pins it in the drawer",
		FlowInferred:   "This flow is inferred; both endpoints are printed by the city.",
		AggregateLabel: "{folded} smaller {word}",
		AggregateRationale: "Our grouping, not a line the city printed: the {folded} smallest {word} in this column are drawn as one " +
			"mark because they cannot be drawn separately. Every figure inside it is printed; the box around them is ours.",
		AggregateNote:     "The {folded} smallest printed rows of {total} by value, at this page's cap of {cap}",
		AggregateTogether: ", together {figure}.",
		ContraOrphan:      project.ContraOrphan,
		BalanceBeginning:  "Balance at the start of the year, as printed: {figure}",
		BalanceEnding:     "Balance at the end of the year, as printed: {figure}",
		BalanceChange:     "Change in balance, the end of the year's less the start's: {figure}",
		FilterHidden:      "Hidden by your filters: {cents} in \u25c7 {flows:flow|flows}. Marks are sized to what is drawn, and each mark's figure is still this view's.",
		FilterFolded:      "\u25c7 {flows:flow|flows} under {threshold} are drawn merged by this page into {ribbons:ribbon|ribbons} of smaller flows.",
		FilterHiddenAt:    "Your filters hide {cents} of this mark's flows, in \u25c7 {flows:flow|flows}.",
		FilterOnly:        "Showing only the money through {label}.",
		FilterOut:         "Filtered out: {names}.",
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
			"Zoom in, go full screen, or search to read them.",
		SplitsDropped: "{year} has no {nodes} to split, so \u25c7 {n:split was|splits were} folded back.",
		RefusedPart: "Not offered with the chart open as it is: the city printed the money from {source} to {target} " +
			"as one total over several parts together, and this is only some of them.",
		RefusedAloneEmpty: "Not offered: the other filters would leave none of the money through {label} to show.",
		RefusedOther:      "Not offered: the chart cannot be drawn that way.",
		RedrawPart: "the filters would draw part of the money from {source} to {target}, which the city printed " +
			"as one total over several parts together.",
		RedrawAloneEmpty: "the other filters would leave none of the money through {label} to show.",
	}
}
