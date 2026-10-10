package export

import "github.com/jcrussell/livermore-budget/internal/project"

// wording is every sentence site/app.js composes about the chart on screen,
// as templates the client fills in: the counts line, the chart hint, the way
// back and the breadcrumb's control. `{name}` is a variable's value and
// `{name:one|many}` is the value followed by the singular or the plural word,
// by whether the value is 1.
//
// Declared here and formatted there, so the words are the packager's and the
// numbers the chart's.
type wording struct {
	Counts            string `json:"counts"`
	CountsPartial     string `json:"counts_partial"`
	CountsCarried     string `json:"counts_carried"`
	CountsCarriedFrom string `json:"counts_carried_from"`
	OpenedHint        string `json:"opened_hint"`
	OpenFurther       string `json:"open_further"`
	OpenInto          string `json:"open_into"`
	NothingFurther    string `json:"nothing_further"`
	NothingOpens      string `json:"nothing_opens"`
	Follow            string `json:"follow"`
	Expand            string `json:"expand"`
	Swatch            string `json:"swatch"`
	InColumn          string `json:"in_column"`
	ColumnLeft        string `json:"column_left"`
	ColumnMiddle      string `json:"column_middle"`
	ColumnSecond      string `json:"column_second"`
	ColumnThird       string `json:"column_third"`
	ColumnRight       string `json:"column_right"`
	GoBack            string `json:"go_back"`
	BackControl       string `json:"back_control"`
	PrintedByCity     string `json:"printed_by_city"`
	InferredByUs      string `json:"inferred_by_us"`
	OurInference      string `json:"our_inference"`
	InferredChip      string `json:"inferred_chip"`
	PrintedChip       string `json:"printed_chip"`
	CarriedNote       string `json:"carried_note"`
	CarriedChip       string `json:"carried_chip"`
	DescOpens         string `json:"desc_opens"`
	DescExpands       string `json:"desc_expands"`
	DescFollows       string `json:"desc_follows"`
	FlowInferred      string `json:"flow_inferred"`
	NoneInferred      string `json:"none_inferred"`
	TablePointer      string `json:"table_pointer"`
	// The aggregate, residual and gap marks, schema/mark.schema.json: their
	// labels, rationales and source notes are templates the constructors
	// fill, so no sentence about what a mark IS is spelled in JavaScript.
	AggregateLabel       string `json:"aggregate_label"`
	AggregateRationale   string `json:"aggregate_rationale"`
	AggregateNote        string `json:"aggregate_note"`
	AggregateTogether    string `json:"aggregate_together"`
	ResidualLabel        string `json:"residual_label"`
	ResidualRationale    string `json:"residual_rationale"`
	ResidualNote         string `json:"residual_note"`
	ResidualWithheldOne  string `json:"residual_withheld_one"`
	ResidualWithheldMany string `json:"residual_withheld_many"`
	// ResidualHeldBack is said beside the chart where a residual has flows
	// and this width draws none of them, so no mark carries its note.
	ResidualHeldBack string `json:"residual_held_back"`
	ResidualFlows    string `json:"residual_flows"`
	GapLabel         string `json:"gap_label"`
	GapLeadShort     string `json:"gap_lead_short"`
	GapLeadOver      string `json:"gap_lead_over"`
	GapRationale     string `json:"gap_rationale"`
	GapNote          string `json:"gap_note"`
	// GapKind is the Kind a gap's ribbon is said to be, which project.LinkKinds
	// has no member for: the difference between two schedules crosses no
	// boundary either of them printed.
	GapKind string `json:"gap_kind"`
	// ContraOrphan is project.ContraOrphan, the sentence a reduction carries
	// when no printed line explains it, shipped so a fold that nets a ribbon
	// negative names it in the producer's words.
	ContraOrphan string `json:"contra_orphan"`
	// A fund's printed balances, schema/column.schema.json's balances, each
	// said beside the node's figure with the fact and pages it cites.
	BalanceBeginning string `json:"balance_beginning"`
	BalanceEnding    string `json:"balance_ending"`
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
	// The full-screen control on the chart's card, in each of its two states.
	FullScreen     string `json:"full_screen"`
	FullScreenExit string `json:"full_screen_exit"`
	// ZoomReset is the control that puts a zoomed or panned chart back whole.
	ZoomReset string `json:"zoom_reset"`
	// BarsShare is a bar's share of the opened node the bars form lists the
	// parts of: a ratio of two client sums over cited cents, which no page
	// prints, so it is said as the page's.
	BarsShare string `json:"bars_share"`
	// Hairline is said beside a chart drawing ribbons under a pixel of its
	// canvas: they are drawn a screen pixel wide, which overstates them, and
	// the sentence says so.
	Hairline string `json:"hairline"`
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
		Counts:            "{links:flow|flows} between {nodes:node|nodes}, from {facts:fact|facts}",
		CountsPartial:     "{links:flow|flows} between {nodes:node|nodes}, from {cited} of the document's {facts:fact|facts}",
		CountsCarried:     "{links:flow|flows} between {nodes:node|nodes}: {own} citing {cited} of the document's {facts:fact|facts}, and {carried} carried unchanged from the chart above",
		CountsCarriedFrom: ", citing {above} of its {theirs:fact|facts}",
		OpenedHint:        "This is {label}, broken into its parts.",
		OpenFurther:       "Double click a node{where} to open it further, or tab to one and press Enter.",
		OpenInto:          "Double click a node{where} to open it into its parts, or tab to one and press Enter.",
		NothingFurther:    "Nothing here opens further; go back to open another.",
		NothingOpens:      "Nothing on this chart opens.",
		Follow:            "A single click, or Space, follows one node's money.",
		Expand:            "The folded mark is several of them drawn as one; double click it, or tab to it and press Enter, to draw them separately.",
		Swatch:            "A fund swatch follows one group's money without opening anything.",
		InColumn:          " in the {columns} column",
		ColumnLeft:        "left-hand",
		ColumnMiddle:      "middle",
		ColumnSecond:      "second",
		ColumnThird:       "third",
		ColumnRight:       "right-hand",
		GoBack:            "Use the breadcrumb above the chart, or press Escape, to go back.",
		BackControl:       "\u2190 {back}",
		PrintedByCity:     "printed by the city",
		InferredByUs:      "inferred by us",
		OurInference:      "◇ our inference",
		InferredChip:      "◇ inferred",
		PrintedChip:       "printed",
		CarriedNote:       "figure printed by the city, re-pointed onto a mark of ours",
		CarriedChip:       "◇ re-pointed by us",
		DescOpens:         ", opens into its parts on a double click or Enter; a single click or Space follows this money",
		DescExpands:       ", draws all of them separately on a double click or Enter; a single click or Space follows this money",
		DescFollows:       ", follow this money",
		FlowInferred:      "This flow is inferred; both endpoints are printed by the city.",
		NoneInferred:      "Nothing on this chart is inferred: every node and flow is printed by the city.",
		TablePointer:      "The same figures are in the flow table below, which opens from the \"" + flowTableHeading + "\" heading.",
		AggregateLabel:    "{folded} smaller {word}",
		AggregateRationale: "Our grouping, not a line the city printed: the {folded} smallest {word} in this column are drawn as one " +
			"mark because they cannot be drawn separately. Every figure inside it is printed; the box around them is ours.",
		AggregateNote:     "The {folded} smallest of {total} by value, at this page's cap of {cap}",
		AggregateTogether: ", together {figure}.",
		ResidualLabel:     "Not split by {grain} here",
		ResidualRationale: "Money the chart above prints for {opened} as a whole and that the schedule this chart is drawn from " +
			"does not split by {grain}, so no {grain} here receives or pays it. It is drawn beside the opened node's parts " +
			"rather than attributed to one of them, and what flows in and what flows out need not balance: the difference " +
			"is what that schedule does not break down. {reasons}",
		ResidualNote:         "Carried, not computed: {flows:flow|flows} of the chart above with figures and citations unchanged \u2014 {where}.{withheld}",
		ResidualWithheldOne:  "The flow leaving it is drawn where there is room for a further column.",
		ResidualWithheldMany: "The {n} flows leaving it are drawn where there is room for a further column.",
		ResidualHeldBack: "{cents} leaves {opened} for {ends}, in {flows:flow|flows} drawn where there is room " +
			"for a further column.",
		ResidualFlows: "{in} in, {out} out",
		GapLabel:      "Difference between the two schedules",
		GapLeadShort: "In {column}, the chart above puts {into} through {centre} and the schedule this chart is drawn from " +
			"accounts for {out} of it, {gap} less.",
		GapLeadOver: "In {column}, the schedule this chart is drawn from accounts for {out} through {centre}, {gap} more " +
			"than the {into} the chart above puts through it.",
		GapRationale: "{lead} {reason} This mark is that {gap}, drawn so that the ribbons and the node agree; no page prints " +
			"it as a figure of its own.",
		GapNote: "Derived, not published: one document's total for this cell less the other's. Each total is built from " +
			"figures `fisc verify` ties to the pages the city printed, and the difference is the one declared for this " +
			"column; no page prints it as a figure of its own.",
		GapKind:          "none: crosses no printed boundary",
		ContraOrphan:     project.ContraOrphan,
		BalanceBeginning: "Balance at the start of the year, as printed: {figure}",
		BalanceEnding:    "Balance at the end of the year, as printed: {figure}",
		FilterHidden:     "Hidden by your filters: {cents} in {flows:flow|flows}. Marks are sized to what is drawn, each mark's figure is still this view's, and the flow table still lists every flow of this view.",
		FilterFolded:     "{flows:flow|flows} under {threshold} are drawn merged by this page into {ribbons:ribbon|ribbons} of smaller flows.",
		FilterHiddenAt:   "Your filters hide {cents} of this mark's flows, in {flows:flow|flows}.",
		FilterDrawnAt:    "{cents} of it drawn under your filters",
		SmallestAll:      "every flow",
		SmallerLabel:     "{flows:flow|flows} under {threshold}",
		SmallerRationale: "Our grouping, not a line the city printed: {flows:flow|flows} each under {threshold}, the smallest flow your filters draw as itself, are drawn as one mark. Every figure inside it is printed; the merge is this page's.",
		SmallerNote:      "Merged by this page from {flows:flow|flows} the city printed, together {figure}; its ribbons cite every fact and page they merge.",
		SmallerFlow:      "{flows:flow|flows} under {threshold}, each printed by the city, merged by this page",
		SearchLabel:      "Find a mark on this chart",
		SearchCount:      "{n:mark matches|marks match}; the arrow keys move between them, and Enter goes to one.",
		SearchNone:       "No mark drawn on this chart has “{query}” in its name.",
		SearchOpens:      "Enter opens it",
		SearchExpands:    "Enter draws out every one it folds",
		FullScreen:       "Full screen",
		FullScreenExit:   "Exit full screen",
		ZoomReset:        "Reset view",
		BarsShare:        "\u25c7 our {share}% of {whole}",
		Hairline: "{n:flow is|flows are} under a pixel wide on this chart's canvas, and each is drawn as a " +
			"hairline one screen pixel wide, which is wider than its share; every other flow is drawn to scale. " +
			"Zoom in, go full screen, search, or open the table below to read them.",
	}
}
