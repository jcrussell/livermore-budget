// Package mapping models the curated rules that say which rows of which
// extracted pages become which facts: the human judgment layer, kept in
// reviewable YAML. See docs/m0-spike.md for the document quirks the schema
// answers.
package mapping

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	yaml "go.yaml.in/yaml/v3"

	"github.com/jcrussell/livermore-budget/internal/amount"
	"github.com/jcrussell/livermore-budget/internal/vocab"
)

// SchemaVersion is the rule-file version this package understands, declared in
// every file and refused if it differs. It bumps when an existing field changes
// meaning; a misspelled key is refused by KnownFields(true) in the parser.
const SchemaVersion = 1

// Quantity is the grammar a cell must satisfy. It is not Kind: a Kind
// classifies a published fact, a Quantity says what a cell is. Every quantity
// but the amount default is read so the row stays whole, and publishes nothing.
type Quantity string

// The quantities a cell can be. QuantityAmount is the default and the only
// quantity that publishes; a rule file never writes it.
const (
	QuantityAmount        Quantity = "amount"
	QuantityAmountPerUnit Quantity = "amount_per_unit"
	QuantityPercentage    Quantity = "percentage"
	QuantityNumber        Quantity = "number"
)

// quantities is the closed set, in declaration order.
var quantities = []Quantity{
	QuantityAmount,
	QuantityAmountPerUnit,
	QuantityPercentage,
	QuantityNumber,
}

func (q Quantity) valid() bool { return slices.Contains(quantities, q) }

// quantityList spells the declarable values for an error message, less
// QuantityAmount, which the parser refuses to see written.
func quantityList() string {
	s := make([]string, 0, len(quantities)-1)
	for _, q := range quantities {
		if q == QuantityAmount {
			continue
		}
		s = append(s, string(q))
	}
	return strings.Join(s, ", ")
}

// File is one rule file, covering one source document.
type File struct {
	SchemaVersion int    `yaml:"schema_version"`
	DocID         string `yaml:"doc_id"`
	Rules         []Rule `yaml:"rules"`

	// Rollups are printed totals that cover several rules, which no single
	// rule's total_row can assert: the eleven <DEPARTMENT> TOTAL rows and General
	// Fund Total Expenses on pp.167-170, Total General Fund on p130, and Total
	// Sources on p140. A department total is the only printed figure that catches
	// a division filed under the wrong department.
	Rollups []Rollup `yaml:"rollups"`

	// Path is the file this was read from, for error messages. Not serialized.
	Path string `yaml:"-"`
}

// Rollup is one printed total that covers several rules.
//
// It sums the covered rules' STATED totals, not their rows. On Budget Book
// p168 ADMINISTRATIVE SERVICES TOTAL is $6,311,564, its three division Totals
// sum to exactly that, and the object rows beneath sum to $6,311,563. Summing
// leaves would tie or fail on whether the city's roundings cancelled. So each
// covered rule must print a total of its own.
type Rollup struct {
	// ID names this rollup for the build report and for error messages.
	ID string `yaml:"id"`

	// Page is the page printing the total, and TotalRow the text anchoring it,
	// which must occur exactly once on the page. p0167.txt:61-62 prints the
	// department name (with the city's typo, DEVELOPEMENT) and its figures on one
	// line and a bare "TOTAL" on the next; the anchor is the name.
	Page     int    `yaml:"page"`
	TotalRow string `yaml:"total_row"`

	// Covers names the rules this total covers, each of which must print a
	// total of its own.
	Covers []string `yaml:"covers"`

	// Unassertable declares that the document prints this rollup and nothing here
	// can assert it, with the reason. An entry carries exactly one of covers and
	// this. p140's "Total Sources" exceeds the pages it closes by ~$57M, differently
	// per column. reasonNoTotalRow means "the rule declares none", a different
	// claim.
	Unassertable string `yaml:"unassertable"`

	// Kinds names the kinds the covered rules span, declared only where they span
	// more than one. It must equal the set the covered rules carry, so it cannot
	// catch a wrong kind; it makes a mixed-kind rollup a written statement. Mixed
	// kinds are not refused (p140's "Total Sources" is revenue plus transfers in);
	// mixed scope is, in validateRollups. It compares Rule.Kind, not a Row.Kind
	// override.
	Kinds []vocab.Kind `yaml:"kinds"`

	// Note records why this rollup looks the way it does.
	Note string `yaml:"note"`
}

// Rule maps a contiguous block of rows into facts.
type Rule struct {
	ID    string      `yaml:"id"`
	Kind  vocab.Kind  `yaml:"kind"`
	Basis vocab.Basis `yaml:"basis"`
	Scope string      `yaml:"scope"`
	// Grain names the lattice level this rule's figures are totals at. Required
	// on a rule that publishes a fact and refused on one that publishes none. The
	// parser checks presence only; internal/structure checks the name.
	Grain string       `yaml:"grain"`
	Units amount.Units `yaml:"units"`

	// Parts are the pages this logical table spans, in document order.
	Parts []Part `yaml:"parts"`

	// Rows are the row labels in the order they appear on the part that
	// carries them, each with the classification it maps to.
	Rows []Row `yaml:"rows"`

	// TotalRow is the label of the row the document prints as the total. When
	// set, verify asserts the mapped rows sum to it.
	TotalRow string `yaml:"total_row"`

	// TotalRowAbove says the document prints the total_row above its rows, and
	// that the total_row label is therefore also the part's section anchor: the
	// stated totals are on the section anchor's own line. total_row does no
	// independent resolution under it (totalAnchor returns the block's start); it
	// names the printed line for a reader and a failure message. It exists for
	// ACFR p41, "General Government:  18.45" over five divisions summing to 18.44,
	// and it alone opens totalAnchor's search above the block and checkGap's
	// leading arm (fisc-h96o). checkGap still refuses every line after the anchor.
	TotalRowAbove bool `yaml:"total_row_above"`

	// TotalRowTail is the rest of the total's label where the page wraps it,
	// verbatim: p0175 prints "Total County Meas BB-" with the figures and
	// "Bike/Pedestrian" alone beneath. totalAnchor refuses the rule unless the next
	// line is this text and nothing else. See WrappedTotalLabel.
	TotalRowTail string `yaml:"total_row_tail"`

	// PrintedDecimals is how many decimal places this rule's page prints its
	// figures to, giving CheckTotals a tolerance derived from the document. The
	// unit is the least significant printed digit, not the caption's word: ACFR
	// p41 is "(in Millions)" with two decimals, so its unit is $10,000. A column
	// ties when
	//
	//	2*|stated - mapped| <= DigitCents(units, printed_decimals) * n_terms
	//
	// The printed total's own half unit is not added, which fails closed. A
	// pointer, because absent (no tolerance, every Budget Book rule) and zero
	// (whole millions) are different claims.
	//
	// It is not a global epsilon: it is refused on units: dollars, refused beside
	// a stated_total_delta, refused unless some token summed into a compared column
	// prints that precision, and refused when every column ties exactly.
	// fisc-1wr.2.
	PrintedDecimals *int `yaml:"printed_decimals"`

	// TotalSpansParts says the printed total_row covers the rows of every part,
	// not just the part that prints it, for a block straddling a page break.
	// Without it the only way to tie such a block is a stated_total_deltas entry
	// sized to the head page's sum, a fabricated claim no staleness check catches.
	// It is refused unless every part declares an identical column list, which
	// keeps it off the p66-67 spine, whose parts partition columns rather than
	// rows.
	TotalSpansParts bool `yaml:"total_spans_parts"`

	// SubtotalChain names a run of rules whose rows are one printed table for
	// the purpose of Row.Subtotal, read in file order: a fund block on Budget Book
	// pp.224-235 can print its total on the next page pair's rule. Absent, a rule
	// is a chain of its own.
	SubtotalChain string `yaml:"subtotal_chain"`

	// TotalRowKinds names the kinds the printed total_row covers, where it covers
	// only some. Absent, it covers every row, the reading the mixed fund blocks on
	// Budget Book pp.131-140 need: their `Total <fund>` includes a Transfers In row.
	// The filter lives on the total row, not in CheckTotals. The p66-67 spine's
	// "TOTAL REVENUES:" wants it (fisc-56f).
	TotalRowKinds []vocab.Kind `yaml:"total_row_kinds"`

	// RowLabelsNameFunds declares that every row of this rule is labelled with
	// the printed name of the fund it carries, so row-funds-match-their-anchors
	// can check the hand-typed number against it. Opt-in, like
	// Part.ColumnHeaders: an inferred claim would silently check the next
	// schedule's row that happens to be a fund name. Measured over the committed
	// corpus: of 400 active non-skipped rows, exactly the 78 declared resolve
	// through registry.FundByLabel. It is on the rule because that check iterates
	// Rule.ActiveRows unioned across parts. A bare fund name asserts identity and
	// no direction, unlike p76's "Transfer From X to Y" anchors.
	RowLabelsNameFunds bool `yaml:"row_labels_name_funds"`

	// RowLabelsAreFundNumbers declares that every row's label is the printed
	// number of the fund the row declares, and that on a row with a
	// counterpart the first field of its label_tail is the printed number of
	// the counterpart's fund. Budget Book p222 prints its receiving CIP fund
	// and its transferring operating fund as two number columns in that order,
	// so row-funds-match-their-anchors holds both ends, direction included.
	RowLabelsAreFundNumbers bool `yaml:"row_labels_are_fund_numbers"`

	// Note records why this rule looks the way it does.
	Note string `yaml:"note"`
}

// Part is one page's worth of a logical table.
type Part struct {
	Page int `yaml:"page"`

	// Section and StopAt bound the block within the page text.
	Section string `yaml:"section"`
	StopAt  string `yaml:"stop_at"`

	// SectionOrdinal picks which occurrence of Section starts the block, 1
	// based. Absent means the anchor must occur exactly once on the page. Budget
	// Book p66 prints "REVENUES:" twice, once inside "TOTAL REVENUES:". An anchor
	// is a plain substring search (Resolver.anchor); a rule that needs it to end a
	// line puts the "\n" in the anchor, as the p67 parts do.
	SectionOrdinal int `yaml:"section_ordinal"`

	// StopAtOrdinal picks which occurrence of StopAt, counted from the block's
	// start, ends the block, 1 based. Absent keeps the first. Budget Book p81 has
	// only amounts and its first row is "$"-prefixed like its total. A wrong
	// ordinal moves the end onto a row line, where the value count or the stated
	// total refuses it.
	StopAtOrdinal int `yaml:"stop_at_ordinal"`

	// LabelsFrom names an earlier part's page when this part carries no row
	// labels of its own: Budget Book p67 continues p66 with numbers only, so row
	// identity is positional against p66's order.
	LabelsFrom int `yaml:"labels_from"`

	// OmittedCells lists cells THE DOCUMENT leaves blank in rows it prints:
	// Budget Book p207 prints nothing under Reserve Increase/(Use) on County
	// Measure D's, Wastewater's and Water's lines. A blank is absent, not zero, so
	// the cell yields no Value and is reported as an Omission. Never declare a cell
	// extraction lost; fix extraction. A row the page does not print at all is
	// placed by Row.Page instead. The part must declare column_headers, which hold
	// each token to its column's band and refuse a figure under a declared blank.
	OmittedCells []omittedCell `yaml:"omitted_cells"`

	// blank is OmittedCells resolved by resolveOmittedCells to row identity and
	// column index: the one place a header is resolved to a column for a blank.
	// A part read before validation declares no blank, and the page's figure count
	// refuses the row.
	blank map[string]map[int]bool

	// Columns describe the value columns, left to right.
	Columns []Column `yaml:"columns"`

	// ColumnHeaders names the header the document prints over each column, left
	// to right, one entry per entry in Columns including skipped ones. Declaring it
	// opts this part into the column-position guard: every figure must fall in the
	// band of its column. Absent, the part is read by text alone.
	//
	// Headers are matched left to right, each after the one before, so a page that
	// dropped a column fails rather than shifting every figure one place. Repeats
	// are allowed (p66 prints "FY 2025-26" twice). An entry that parses as an
	// amount is refused, since it would match a data row: write "FY 2026", not
	// "2026". An entry may be null only for a skipped last column; see
	// columnHeader.
	ColumnHeaders columnHeaders `yaml:"column_headers"`

	// Grid names a header list, for a page printing two schedules under
	// different headers (Budget Book p80). Only checkColumnGrids reads it: parts
	// naming one grid of a page declare one header list, two grids may not declare
	// the same list, and a part naming a grid must declare its headers.
	Grid string `yaml:"grid"`

	// WrappedLabels are the printed fragments this part's page wraps onto a
	// line of their own, each verbatim. The tail lands between one row's last
	// figure and the next row's label, where checkGap refuses anything else:
	//
	//	Innovation & Economic Wages & Benefits   1,028,282  1,068,663 ...
	//	Devel
	//	                      Services & Supplies  1,610,950  2,592,436 ...
	//
	// skip: true cannot express it: a skipped row still consumes a value per
	// column. A fragment is matched whole against one printed line, so this admits
	// only what an author wrote down. It is on the part because a fragment may be
	// no row's own (p167's wrapped department TOTAL). A gap may hold several, one
	// per line; an undeclared line refuses the gap, and a declared fragment the
	// page does not use is an error.
	WrappedLabels []string `yaml:"wrapped_labels"`

	// Headings are the section headings this part's page prints on a line of
	// their own between rows, each verbatim: Budget Book p186's "Special Revenue
	// Funds". Matched like a wrapped label, one line at a time; a gap may mix them
	// with wrapped labels and unmapped_text (p190's footnote "1"). An unused
	// heading is refused. A heading may equal a row label (p186's does); a line
	// carrying figures is never a heading, so it cannot swallow the row.
	Headings []string `yaml:"headings"`

	// StatedTotalDeltas declares columns where the total the DOCUMENT prints
	// is not the sum of its rows, and by how much. p127's thirteen property-tax
	// rows give $58,179,467 against a printed $58,179,468, and ten more blocks are
	// off by <= $5, all in the FY2023-24 Actual column: the city's rounding.
	//
	// A declaration, not a tolerance: CheckTotals accepts that exact figure and no
	// other, and refuses one that now ties as stale. Rule.PrintedDecimals is the
	// only tolerance, and a rule declaring both is refused. Absent both, exact
	// equality holds.
	StatedTotalDeltas []statedTotalDelta `yaml:"stated_total_deltas"`

	// UnmappedText declares a FIGURE the page prints inside this block that
	// belongs to no row, with the reason. ACFR p41 prints a bare "0.0" between
	// the Miscellaneous row's figures and Total Revenues, in no printed row.
	//
	// The parser requires a wrapped label NOT to parse as an amount and this to
	// parse as one, so neither declaration can stand in for the other. A wrapped
	// label that is a bare number therefore has no declaration (fisc-2jk's shape;
	// fisc-xmsk). This is the weakest declaration class here: "0.0" sits in a
	// skipped column, so no sum witnesses it. It is bounded by its staleness arm,
	// by matching a whole printed line, and by every row still being found by its
	// label, so its Note is load-bearing. fisc-hcus.
	UnmappedText []unmappedText `yaml:"unmapped_text"`
}

// unmappedText is one figure a page prints inside a block that belongs to no
// row, and why.
type unmappedText struct {
	// Text is the figure exactly as printed, matched against one trimmed printed
	// line of the gap, as wrapped_labels and headings are.
	Text string `yaml:"text"`

	// Note is required and says why the page prints it, as StatedTotalDelta.Note.
	Note string `yaml:"note"`
}

// columnHeader is one entry in a part's ColumnHeaders: the header a page prints
// over a column, or the statement that it prints none.
//
// Budget Book p76 prints footnote markers "(1)" through "(10)" right of its four
// year headers, which amount.Parse reads as negatives; the column must be
// declared and skipped, and has no header. null is restricted to the last
// column, where there is no band to check; between two headers the grid could
// have checked it. The unbanded token must still be on the row's line
// (columnGuard.checkRow), the row must yield len(Columns) tokens, and checkGap
// still refuses unexplained text.
type columnHeader struct {
	// Text is the header as the page prints it, empty when Unheaded.
	Text string
	// Unheaded is separate from an empty Text so that a YAML "" stays refused.
	Unheaded bool
}

// columnHeaders is a part's header list. It has its own unmarshaler because
// yaml.v3 drops a null element from a sequence, which would silently shorten
// the list the column count is checked against.
type columnHeaders []columnHeader

// UnmarshalYAML reads the sequence element by element, so a null keeps its
// place.
func (h *columnHeaders) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.SequenceNode {
		return fmt.Errorf("line %d: column_headers is a list of the headers the page "+
			"prints over this part's columns, left to right", n.Line)
	}
	out := make(columnHeaders, 0, len(n.Content))
	for _, e := range n.Content {
		var c columnHeader
		if e.Tag == "!!null" {
			c.Unheaded = true
		} else if e.Kind != yaml.ScalarNode {
			return fmt.Errorf("line %d: a column_headers entry is the header the page "+
				"prints over that column, or null where it prints none", e.Line)
		} else if err := e.Decode(&c.Text); err != nil {
			return err
		}
		out = append(out, c)
	}
	*h = out
	return nil
}

// String renders an entry for an error message, so a null reads as a claim
// rather than as an empty pair of quotes.
func (c columnHeader) String() string {
	if c.Unheaded {
		return "null (the page prints no header here)"
	}
	return fmt.Sprintf("%q", c.Text)
}

// statedTotalDelta is one column's declared discrepancy between the document's
// printed total and the rows beneath it.
type statedTotalDelta struct {
	// Column is 1 based, as CheckTotals numbers columns in a mismatch.
	Column int `yaml:"column"`

	// Cents is STATED MINUS MAPPED, always in cents whatever the rule's units:
	// p127's FY2023-24 column prints $1 more than its rows, so it declares 100.
	Cents amount.Cents `yaml:"delta_cents"`

	// Note is required and says why; a bare number would be a tolerance.
	Note string `yaml:"note"`
}

// Column identifies one value column. Identity is compound because these
// schedules stack a fund-group header above a fiscal-year header.
type Column struct {
	FundGroup  string `yaml:"fund_group"`
	Fund       int    `yaml:"fund"`
	FiscalYear int    `yaml:"fiscal_year"`

	// Basis overrides the rule's basis for this column: the four-column revenue
	// schedules carry actual, revised and two adopted years side by side.
	Basis vocab.Basis `yaml:"basis"`

	// FiscalYearThrough is the last fiscal year of a figure printed over
	// several. No column declares it; Row.EffectiveColumn carries a row's here.
	FiscalYearThrough int `yaml:"-"`

	// Skip marks a column that is present in the text but should not produce
	// facts, so column positions still line up.
	Skip bool `yaml:"skip"`

	// Quantity declares that this column's cells are not amounts: they must
	// satisfy the named grammar (internal/quantity) and never publish, so the
	// column needs no fiscal_year. skip cannot express it: parseRow parses a cell
	// before it consults skip, so a skipped "2.5%" still fails the amount grammar.
	// fisc-9tn4.
	Quantity Quantity `yaml:"quantity"`

	// Category and Kind classify every figure in this column, for a page whose
	// columns are its budget lines and whose rows are its funds (Budget Book
	// pp.186-209). Kind falls back to the rule's. A rule carries its category on
	// its rows or its columns, never both; see Value.Category.
	Category string     `yaml:"category"`
	Kind     vocab.Kind `yaml:"kind"`
}

// Counterpart is the far end of a figure that moves money between two funds.
// Every field is the counterpart leg's own: one defaulted from the row would be
// a double-count under a different id. The row's sign is inherited: netted only
// where both ends are transfers (validateNettedSign), and contra never
// (checkCounterpart).
type Counterpart struct {
	Category  string     `yaml:"category"`
	Kind      vocab.Kind `yaml:"kind"`
	Fund      int        `yaml:"fund"`
	FundGroup string     `yaml:"fund_group"`
}

// Column returns the column the counterpart leg's fact is filed under: the
// printed column with the counterpart's fund.
func (c Counterpart) Column(printed Column) Column {
	printed.FundGroup = c.FundGroup
	printed.Fund = c.Fund
	return printed
}

// Row is one labelled line and the classification it maps to.
type Row struct {
	Label string `yaml:"label"`

	// LabelTail is a second published anchor identifying the row together with
	// Label, for a page whose row identity is two printed fields with an arbitrary
	// gap. Budget Book p76 prints
	//
	//	Transfer From Low Income Hsng          to General Fund   257,012 ...
	//
	// where neither field alone is unique. The gap must be whitespace and nothing
	// else, never across a line break, so a rule never spells the page's kerning.
	// A tail of several words names several fields: p222's "601   600       Airport"
	// is label "601", tail "600 Airport".
	LabelTail string `yaml:"label_tail"`

	// Page is the one part that prints this row, for a table whose rows run
	// across a page break: Budget Book p167 prints General Services' Wages &
	// Benefits and p168 the rest. Zero means every part prints it. A part may
	// print no row at all (p130 prints only a Total). Never place a row to cover
	// for extraction; fix extraction.
	Page int `yaml:"page"`

	Category   string     `yaml:"category"`
	Department string     `yaml:"department"`
	Sign       vocab.Sign `yaml:"sign"`

	// Fund and FundGroup override the column's fund dimension for this row.
	// Budget Book p76's columns are years and its sections the receiving groups,
	// while the paying fund varies row by row (three of five sections are mixed);
	// without this, rule boundaries would depend on the city's row order.
	Fund      int    `yaml:"fund"`
	FundGroup string `yaml:"fund_group"`

	// FiscalYear is the year this row's figures are of, for a table whose rows
	// are years: Budget Book p80's row "2026" is fiscal_year 2026. A rule carries
	// its year on its columns or on its publishing rows, never both, and the
	// parser holds the year to the printed label.
	FiscalYear int `yaml:"fiscal_year"`

	// FiscalYearThrough is the last year of a row printing several years as one
	// figure (p80's "2031-2035"), published as fact.Fact.FiscalYearThrough.
	FiscalYearThrough int `yaml:"fiscal_year_through"`

	// Counterpart is the other end of a figure that moves money between two
	// funds, so one printed figure is evidence for two facts. p76's "Transfer From
	// Low Income Hsng  to General Fund  257,012" leaves fund 200 and arrives at
	// fund 100; both facts cite the same doc_id, page, offset and token. Their ids
	// differ through category and fund, not a discriminator. It also lets a rule
	// declare the payer on p76's "to ..." continuation rows rather than infer it.
	Counterpart *Counterpart `yaml:"counterpart"`

	// Kind overrides the rule's kind for this row. Eleven or more fund blocks on
	// Budget Book pp.131-140 print revenue rows and a Transfers In row inside one
	// `Total <fund>` (p131's Stormwater FY2025-26 is 1,169,000 of charges plus
	// 3,247,000 of Transfers In), so CheckTotals stays kind-blind; a total covering
	// only some kinds says so in TotalRowKinds.
	Kind vocab.Kind `yaml:"kind"`

	// Skip marks a row that occupies a position but produces no facts.
	Skip bool `yaml:"skip"`

	// Subtotal declares a skipped row to be a subtotal the page prints, and its
	// level. CheckSubtotals holds each figure to the sum of the non-subtotal rows
	// above it since the last subtotal of the same or higher level, under the same
	// column header, skipped columns included. It is for subtotals a label-less
	// continuation page prints between blocks (Budget Book p225); a total ending a
	// block is still the rule's total_row.
	Subtotal int `yaml:"subtotal"`

	// SubtotalDeltas declares where a subtotal row prints a figure its rows do
	// not sum to, and by how much: Budget Book p235's grand TOTAL prints FY2025-26
	// a dollar under its 219 rows. A declared column that ties is refused.
	SubtotalDeltas []SubtotalDelta `yaml:"subtotal_deltas"`

	// Quantity overrides every column's quantity for this row (fisc-9tn4): the
	// non-amount is usually a row spanning every year column (p169, p179, p189,
	// p192). A non-amount row publishes nothing, so needs no category.
	Quantity Quantity `yaml:"quantity"`
}

// Identity is the row's identity within its rule: both anchors when it has
// two. Uniqueness is enforced on this, not on Label.
func (r Row) Identity() string {
	if r.LabelTail == "" {
		return r.Label
	}
	return r.Label + "\x1f" + r.LabelTail
}

// WrappedTotalLabel is the whole printed label of the rule's total: total_row,
// and total_row_tail after it where the page wraps one, joined by JoinWrapped.
func (r *Rule) WrappedTotalLabel() string {
	if r.TotalRowTail == "" {
		return r.TotalRow
	}
	return JoinWrapped(r.TotalRow, r.TotalRowTail)
}

// JoinWrapped is a label the page breaks across two lines, rejoined with its
// tail's words single-spaced. A label broken at a hyphen joins without a space
// ("County Meas BB-" + "Bike/Pedestrian"); a spaced dash ("Asset Seizure -")
// keeps it.
func JoinWrapped(head, tail string) string {
	tail = strings.Join(strings.Fields(tail), " ")
	if strings.HasSuffix(head, "-") && !strings.HasSuffix(head, " -") {
		return head + tail
	}
	return head + " " + tail
}

// totalCovers says whether a row of this kind is one the printed total_row
// covers; every row is when no TotalRowKinds is declared.
func (r *Rule) totalCovers(k vocab.Kind) bool {
	if len(r.TotalRowKinds) == 0 {
		return true
	}
	return slices.Contains(r.TotalRowKinds, k)
}

// EffectiveKind is the kind this row's facts carry: its own override where it
// declares one, the rule's otherwise.
func (r Row) EffectiveKind(rule *Rule) vocab.Kind {
	if r.Kind != "" {
		return r.Kind
	}
	return rule.Kind
}

// publishes says whether a figure in this column can become a fact: it is
// neither skipped nor a non-amount quantity.
func (c Column) publishes() bool {
	return !c.Skip && c.Quantity == ""
}

// categoryOnColumns says whether this rule's columns carry its category rather
// than its rows: some column that publishes declares one.
func (r *Rule) categoryOnColumns() bool {
	for i := range r.Parts {
		for _, c := range r.Parts[i].Columns {
			if c.publishes() && c.Category != "" {
				return true
			}
		}
	}
	return false
}

// cellPublishes is THE predicate for whether row's cell in this part's j'th
// column becomes a fact, and nothing re-filters after it. Three decisions read
// something else: a row's category requirement reads cellAddressed (a declared
// blank is an address too), validateOmittedCells counts the columns a row
// prints, and validateColumnClass is about the column alone.
func (p *Part) cellPublishes(j int, row Row) bool {
	return p.cellAddressed(j, row) && !p.blank[row.Identity()][j]
}

// cellAddressed is cellPublishes before omitted_cells. A declared blank still
// needs the row's category, since internal/check reads it as the fact the cell
// would have been.
func (p *Part) cellAddressed(j int, row Row) bool {
	return row.OnPart(p) && row.Publishes(p.Columns[j])
}

// rowAddressed says whether any cell of this rule is an address of row.
func (r *Rule) rowAddressed(row Row) bool {
	for i := range r.Parts {
		p := &r.Parts[i]
		for j := range p.Columns {
			if p.cellAddressed(j, row) {
				return true
			}
		}
	}
	return false
}

// RowPublishes says whether any cell of this rule publishes row.
func (r *Rule) RowPublishes(row Row) bool {
	return len(r.kindsOf(row)) > 0
}

// kindsOf is every kind a row's facts carry: one per cell cellPublishes
// admits, read through Value.Kind.
func (r *Rule) kindsOf(row Row) []vocab.Kind {
	var out []vocab.Kind
	for i := range r.Parts {
		p := &r.Parts[i]
		for j, c := range p.Columns {
			if p.cellPublishes(j, row) {
				out = append(out, Value{Row: row, Column: c}.Kind(r))
			}
		}
	}
	return out
}

// EffectiveKind is the kind this column's facts carry on a rule whose columns
// carry the category: its own where it declares one, the rule's otherwise.
func (c Column) EffectiveKind(rule *Rule) vocab.Kind {
	if c.Kind != "" {
		return c.Kind
	}
	return rule.Kind
}

// EffectiveBasis is the basis this column's facts are published on: its own
// where it declares one, the rule's otherwise.
func (c Column) EffectiveBasis(rule *Rule) vocab.Basis {
	if c.Basis != "" {
		return c.Basis
	}
	return rule.Basis
}

// EffectiveQuantity is the grammar this row's cell in c must satisfy: the
// row's override where it declares one, the column's otherwise, amounts by
// default. Only a QuantityAmount cell can become a fact.
func (r Row) EffectiveQuantity(c Column) Quantity {
	if r.Quantity != "" {
		return r.Quantity
	}
	if c.Quantity != "" {
		return c.Quantity
	}
	return QuantityAmount
}

// Publishes says whether this row's cell in c can become a fact: the row is
// not skipped, and c publishes under the row's quantity override.
func (r Row) Publishes(c Column) bool {
	if r.Quantity != "" {
		c.Quantity = r.Quantity
	}
	return !r.Skip && c.publishes()
}

// OnPart says whether p prints this row: every part does unless the row
// names its page. validateRule alone reads Row.Page directly.
func (r Row) OnPart(p *Part) bool {
	return r.Page == 0 || r.Page == p.Page
}

// EffectiveColumn is the column a row's own fact is filed under: the printed
// column with the row's fund and fiscal-year overrides applied, per field.
func (r Row) EffectiveColumn(printed Column) Column {
	if r.FundGroup != "" {
		printed.FundGroup = r.FundGroup
	}
	if r.Fund != 0 {
		printed.Fund = r.Fund
	}
	if r.FiscalYear != 0 {
		printed.FiscalYear = r.FiscalYear
		printed.FiscalYearThrough = r.FiscalYearThrough
	}
	return printed
}

// YearsOnRows says whether this rule's rows carry its fiscal years rather
// than its columns: some row declares one.
func (r *Rule) YearsOnRows() bool {
	for _, row := range r.Rows {
		if row.FiscalYear != 0 || row.FiscalYearThrough != 0 {
			return true
		}
	}
	return false
}

// YearLabel is how a page prints a row's years: "2026" for one fiscal year,
// "2031-2035" for a band.
func YearLabel(year, through int) string {
	if through == 0 {
		return strconv.Itoa(year)
	}
	return strconv.Itoa(year) + "-" + strconv.Itoa(through)
}

// PrintedLabel is what the document printed for this row, as the fact's
// row_label: a two-anchor row's anchors joined by one space, since the page's
// run of spaces is typesetting.
func (r Row) PrintedLabel() string {
	if r.LabelTail == "" {
		return r.Label
	}
	return r.Label + " " + r.LabelTail
}

// omittedCell names one blank cell: a row, by its label and label_tail, and
// the column_headers entry printed over the cell.
//
//	omitted_cells:
//	  - {label: "County Measure D", column: "Increase/(Use)", note: "..."}
//
// It has no unmarshaler so KnownFields still refuses a typo'd key.
type omittedCell struct {
	Label     string `yaml:"label"`
	LabelTail string `yaml:"label_tail"`
	Column    string `yaml:"column"`
	// ColumnOrdinal says which column headed Column is meant, 1 based, where
	// the part prints that header over several (p80). Refused on a header printed
	// once.
	ColumnOrdinal int    `yaml:"column_ordinal"`
	Note          string `yaml:"note"`
}

// row is the Row this entry names, so identity uses Row's own methods.
func (o omittedCell) row() Row { return Row{Label: o.Label, LabelTail: o.LabelTail} }

// describe is how the entry reads in a message.
func (o omittedCell) describe() string {
	if o.ColumnOrdinal > 0 {
		return fmt.Sprintf("%q under %q #%d", o.row().PrintedLabel(), o.Column, o.ColumnOrdinal)
	}
	return fmt.Sprintf("%q under %q", o.row().PrintedLabel(), o.Column)
}

// columnHeaded is the index in Columns of the one column header heads. A
// header no column prints is refused, and so is one printed over two (Budget
// Book p67's "FY 2025-26").
func (p *Part) columnHeaded(header string) (int, error) {
	return p.columnHeadedAt(header, 0)
}

// columnHeadedAt is columnHeaded naming the ordinal-th column header heads,
// for a header printed over several. An ordinal of zero requires the header
// to head one column, and a non-zero one requires it to head several.
func (p *Part) columnHeadedAt(header string, ordinal int) (int, error) {
	var at []int
	for c, h := range p.ColumnHeaders {
		if !h.Unheaded && h.Text == header {
			at = append(at, c)
		}
	}
	switch {
	case len(at) == 0:
		return -1, fmt.Errorf("%q is not one of this part's column_headers: %s",
			header, describeHeaders(p.ColumnHeaders))
	case ordinal < 0:
		return -1, fmt.Errorf("column_ordinal %d for %q; ordinals count from 1", ordinal, header)
	case ordinal == 0 && len(at) > 1:
		return -1, fmt.Errorf("%q heads more than one of this part's columns, so it "+
			"cannot say which cell is blank; set column_ordinal (1 to %d)", header, len(at))
	case ordinal == 0:
		return at[0], nil
	case len(at) == 1:
		return -1, fmt.Errorf("column_ordinal %d for %q, which heads one column; "+
			"remove the ordinal", ordinal, header)
	case ordinal > len(at):
		return -1, fmt.Errorf("column_ordinal %d for %q, which heads %d columns",
			ordinal, header, len(at))
	}
	return at[ordinal-1], nil
}

// printedColumns is the columns a row prints on this part, left to right: all
// of them, less any the part declares blank on it.
func printedColumns(p *Part, row Row) []int {
	cols := make([]int, 0, len(p.Columns))
	for c := range p.Columns {
		if !p.blank[row.Identity()][c] {
			cols = append(cols, c)
		}
	}
	return cols
}

// labelledPart returns the part that carries row labels for p, which is p
// itself unless p declares LabelsFrom.
func (r *Rule) labelledPart(p *Part) *Part {
	if p.LabelsFrom == 0 {
		return p
	}
	for i := range r.Parts {
		if r.Parts[i].Page == p.LabelsFrom {
			return &r.Parts[i]
		}
	}
	return nil
}

// ActiveRows returns the rule's rows this part prints, by Row.OnPart, in
// order. The result is always a fresh slice, so a caller cannot write through
// it into the rule.
func (r *Rule) ActiveRows(p *Part) []Row {
	rows, _ := r.activeRows(p)
	return rows
}

// activeRows is ActiveRows paired with each row's index in r.Rows, the one
// filter on Row.OnPart. It walks by position because two skipped rows may share
// an identity on different pages.
func (r *Rule) activeRows(p *Part) ([]Row, []int) {
	rows := make([]Row, 0, len(r.Rows))
	idx := make([]int, 0, len(r.Rows))
	for i, row := range r.Rows {
		if row.OnPart(p) {
			rows = append(rows, row)
			idx = append(idx, i)
		}
	}
	return rows, idx
}

// expectedValues is how many numbers a positional read of this part must find:
// one per active row per column it prints.
func (r *Rule) expectedValues(p *Part) int {
	n := 0
	for _, row := range r.ActiveRows(p) {
		n += len(printedColumns(p, row))
	}
	return n
}

// SubtotalDeltaAt is the delta this row declares for the column a part
// prints at index ci, matched through columnHeaded. A delta naming a header
// this part does not print belongs to another part.
func (r Row) SubtotalDeltaAt(p *Part, ci int) (amount.Cents, bool) {
	for _, d := range r.SubtotalDeltas {
		if c, err := p.columnHeaded(d.Column); err == nil && c == ci {
			return d.Cents, true
		}
	}
	return 0, false
}

// SubtotalDelta is one column's declared discrepancy on a subtotal row.
type SubtotalDelta struct {
	// Column is the header the page prints over the figure.
	Column string `yaml:"column"`

	// Cents is PRINTED MINUS SUMMED, in cents, as statedTotalDelta's is.
	Cents amount.Cents `yaml:"delta_cents"`

	// Note is required and says why.
	Note string `yaml:"note"`
}
