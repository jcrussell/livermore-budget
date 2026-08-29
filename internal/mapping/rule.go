// Package mapping models the curated rules that say which rows of which
// extracted pages become which facts.
//
// This is the judgment layer. Extraction is mechanical and reproducible;
// deciding that a row labelled "Charges for Services" in the Enterprise column
// of page 66 is FY2026 adopted revenue for the enterprise fund group is a
// human call. Keeping it in reviewable YAML — rather than in Go, or inferred
// by heuristics — is what makes the mapping auditable.
//
// The schema is shaped by what the source documents actually do, which is
// stranger than a table model suggests. See docs/m0-spike.md; the awkward
// parts of this schema exist because of the awkward parts of the PDFs.
package mapping

import (
	"fmt"
	"slices"
	"strings"

	yaml "go.yaml.in/yaml/v3"

	"github.com/jcrussell/livermore-budget/internal/amount"
)

// SchemaVersion is the rule-file version this package understands, declared in
// every file and refused if it differs. It is a compatibility marker between
// the rule files and the reader, not a feature flag: a rule file is only
// meaningful against the resolver that knows how to read its fields, so when
// the meaning of an existing field changes — a renamed key, a new default, an
// anchor that resolves differently — this bumps and the old files are edited
// rather than silently reinterpreted.
//
// Adding a field does not require a bump while nothing is published, because
// there is no older binary anywhere to hand a newer file to. The refusal that
// does the day-to-day work is KnownFields(true) in the parser, which makes a
// misspelled key fatal; this constant only guards a change of meaning.
const SchemaVersion = 1

// Kind is what a fact represents in the flow model.
type Kind string

// The kinds of fact a rule can produce.
const (
	KindRevenue     Kind = "revenue"
	KindExpenditure Kind = "expenditure"
	KindTransferIn  Kind = "transfer_in"
	KindTransferOut Kind = "transfer_out"
	KindFundBalance Kind = "fund_balance"
)

// kinds is the closed set, in the order the constants declare it. It is the one
// list: Kinds and valid both read it, so a sixth kind cannot be added to one
// spelling and missed by the other.
var kinds = []Kind{
	KindRevenue,
	KindExpenditure,
	KindTransferIn,
	KindTransferOut,
	KindFundBalance,
}

// Kinds returns the closed set of kinds a rule can produce, in declaration
// order.
//
// It is exported for readers OUTSIDE this package that must agree with this
// vocabulary -- data/taxonomy.yaml declares kinds per category, and
// internal/registry has to refuse a member that is not one of these. Without
// it that package would re-spell the five values, which is the second copy the
// taxonomy's own header argues against. The returned slice is a copy: a caller
// that sorts or appends must not be able to move the vocabulary.
func Kinds() []Kind { return slices.Clone(kinds) }

func (k Kind) valid() bool { return slices.Contains(kinds, k) }

// kindList spells the closed set for an error message, comma separated in
// declaration order. It exists so no message re-types the five values: one
// such literal had already drifted out of sight in validateRule.
func kindList() string {
	s := make([]string, len(kinds))
	for i, k := range kinds {
		s[i] = string(k)
	}
	return strings.Join(s, ", ")
}

// Basis distinguishes a budgeted figure from an audited one. Mixing them in a
// single view is the most common way a civic budget chart misleads, so it is
// carried on every fact rather than assumed per document.
type Basis string

// The bases a figure can be stated on.
const (
	BasisAdopted   Basis = "adopted"
	BasisRevised   Basis = "revised"
	BasisActual    Basis = "actual"
	BasisAudited   Basis = "audited"
	BasisProjected Basis = "projected"
)

func (b Basis) valid() bool {
	switch b {
	case BasisAdopted, BasisRevised, BasisActual, BasisAudited, BasisProjected:
		return true
	}
	return false
}

// Sign marks a row that reduces its category rather than adding to it.
type Sign string

const (
	// SignPositive is the default.
	SignPositive Sign = "positive"
	// SignContra marks a deduction booked as negative revenue — the ERAF and
	// RPTTF property-tax shifts on Budget Book p127 are ~26% of gross
	// property tax. A Sankey cannot render a negative link, so these net into
	// their parent category and are disclosed in the provenance panel.
	SignContra Sign = "contra"
)

func (s Sign) valid() bool { return s == "" || s == SignPositive || s == SignContra }

// File is one rule file, covering one source document.
type File struct {
	SchemaVersion int    `yaml:"schema_version"`
	DocID         string `yaml:"doc_id"`
	Rules         []Rule `yaml:"rules"`

	// Rollups are printed totals that cover several RULES.
	//
	// TotalRow is a field on Rule, so a total the document prints over more
	// than one rule has no rule to be the total_row OF and cannot be asserted
	// at all. Twelve such totals sit on the pages the coverage lanes map: the
	// eleven <DEPARTMENT> TOTAL rows and General Fund Total Expenses on
	// pp.167-170, Total General Fund on p130, and Total Sources on p140. The
	// rules beneath them are one per fund block or one per division because
	// row identity forces that, not by preference.
	//
	// It matters rather than being a counting nit: the department-level total
	// is the ONLY printed figure that catches a division filed under the wrong
	// department, since every category sum is unchanged by the move.
	Rollups []Rollup `yaml:"rollups"`

	// Path is the file this was read from, for error messages. Not serialized.
	Path string `yaml:"-"`
}

// Rollup is one printed total that covers several rules.
//
// IT SUMS THE COVERED RULES' STATED TOTALS, NOT THEIR ROWS, and that is the
// design decision rather than an implementation detail. Measured on Budget
// Book p168: ADMINISTRATIVE SERVICES TOTAL is $6,311,564, its three printed
// division Totals sum to exactly that, and the six OBJECT rows beneath them
// sum to $6,311,563 -- Finance's own $1 of rounding is absorbed by its printed
// division Total and reappears one level up. General Fund Total Expenses on
// p170 happens to tie against the object rows too, but only because that
// page's four declared deltas net to zero.
//
// So a rollup summing leaves would tie or fail depending on whether the
// document's roundings happened to cancel, and the author's only escape would
// be a delta declaration at the rollup level absorbing a structural artefact
// -- which is the fabricated-declaration hazard Rule.TotalSpansParts exists to
// close, reintroduced one level up.
//
// Summing stated totals instead makes a two-level chain: CheckTotals ties each
// rule's rows to its own printed subtotal, and the rollup ties those subtotals
// to the printed rollup. Each level is the document checking us, and neither
// level absorbs the other's residue. It is also why a covered rule MUST carry
// a printed total of its own -- there would otherwise be nothing to sum.
type Rollup struct {
	// ID names this rollup for the build report and for error messages.
	ID string `yaml:"id"`

	// Page is the page printing the total, and TotalRow the text anchoring it.
	// The anchor must occur exactly once on the page: a rollup checked against
	// whichever occurrence came first would be the confident wrong answer this
	// project exists to refuse.
	//
	// Watch p0167.txt:61-62, which prints a department total as
	// "INNOVATION & ECONOMIC DEVELOPEMENT  $2,639,232 ..." on one line with
	// the city's own typo and a bare "TOTAL" wrapping onto the next. The
	// anchor is the department name, and the figures are on its line.
	Page     int    `yaml:"page"`
	TotalRow string `yaml:"total_row"`

	// Covers names the rules this total covers, each of which must print a
	// total of its own.
	Covers []string `yaml:"covers"`

	// Unassertable declares that the document prints this rollup and that no
	// rule structure here can assert it, with the reason. An entry carries
	// EITHER covers OR this, never both and never neither.
	//
	// It exists because silence is the one unacceptable answer. p140's
	// "Total Sources" exceeds the pages it closes by ~$57M, differently per
	// column (fisc-wev), so it cannot be asserted by this or any other
	// mechanism here -- and mapping those ten pages while declining to mention
	// that the page's own closing total does not reconcile would be publishing
	// the gap as though it were not there. reasonNoTotalRow cannot say this:
	// it means "the rule declares none", which is a different claim.
	Unassertable string `yaml:"unassertable"`

	// Note records why this rollup looks the way it does.
	Note string `yaml:"note"`
}

// Rule maps a contiguous block of rows into facts.
type Rule struct {
	ID    string       `yaml:"id"`
	Kind  Kind         `yaml:"kind"`
	Basis Basis        `yaml:"basis"`
	Scope string       `yaml:"scope"`
	Units amount.Units `yaml:"units"`

	// Parts are the pages this logical table spans, in document order.
	Parts []Part `yaml:"parts"`

	// Rows are the row labels in the order they appear on the part that
	// carries them, each with the classification it maps to.
	Rows []Row `yaml:"rows"`

	// TotalRow is the label of the row the document itself prints as the
	// total. When set, verify asserts the mapped rows sum to it — the single
	// most valuable check available, because it is the document checking our
	// work rather than us checking our own.
	TotalRow string `yaml:"total_row"`

	// TotalSpansParts says the printed total_row covers the rows of EVERY
	// part, not just the rows of the part that prints it.
	//
	// Nine blocks in the two coverage lanes straddle a page break, so their
	// rows land in two parts while their printed total lands in one. Without
	// this, neither exit works: keeping total_row makes the head part fail to
	// find it (ErrNotFound, a hard build failure), and dropping it reports
	// both parts as "the rule declares no total_row", which is a claim about
	// the RULE standing in for a claim about the DOCUMENT — precisely the
	// misreport pkg/cmd/build.reasonNoTotalRow's own comment guards against.
	//
	// The hazard this closes is the third exit, which is worse than either:
	// with both of the others shut, the cheapest thing left for a rule author
	// is a stated_total_deltas entry sized to the head page's sum — a
	// FABRICATED claim about the city's rounding absorbing a structural gap.
	// fisc-2sd catches a declaration that has gone STALE; nothing catches one
	// that was fiction from the start.
	//
	// It is refused unless every part declares an IDENTICAL column list, which
	// is what keeps it off the p66-67 spine: those parts partition COLUMNS
	// (p66 is four wide, p67 eight) rather than ROWS, so summing across them
	// would produce a number that means nothing. A flag whose misuse is caught
	// by the parser is a different thing from one that relies on the author.
	TotalSpansParts bool `yaml:"total_spans_parts"`

	// TotalRowKinds names the kinds the printed total_row covers, where it
	// covers only some of them. Absent, it covers every row the rule maps,
	// which is the reading eleven-plus mixed fund blocks on Budget Book
	// pp.131-140 need: their printed `Total <fund>` includes both the revenue
	// rows and a Transfers In row.
	//
	// This is where a kind filter belongs, rather than in CheckTotals. A total
	// row is the thing that knows which rows it covers, so saying which is a
	// claim the rule author makes and review can check -- the same shape as
	// Column.Basis overriding Rule.Basis on the column. Making CheckTotals
	// globally kind-aware instead would break all eleven of those blocks,
	// which is the opposite of what Row.Kind was promoted to enable.
	//
	// The p66-67 spine is the case that wants it: "TOTAL REVENUES:" covers
	// only the revenue rows of a block it shares with transfers and fund
	// balance. Declaring that is what would let those five rules become three
	// (fisc-56f); nothing does so yet.
	TotalRowKinds []Kind `yaml:"total_row_kinds"`

	// Note records why this rule looks the way it does, for the next reader.
	Note string `yaml:"note"`
}

// Part is one page's worth of a logical table.
type Part struct {
	Page int `yaml:"page"`

	// Section and StopAt bound the block within the page text.
	Section string `yaml:"section"`
	StopAt  string `yaml:"stop_at"`

	// SectionOrdinal picks which occurrence of Section starts the block, 1
	// based. Absent means the anchor must occur exactly once on the page.
	//
	// It is not an optional nicety. Budget Book p66 prints "REVENUES:" twice —
	// once as the section header and once inside "TOTAL REVENUES:" — so a
	// unique-match rule cannot resolve the citywide spine at all. Nor would
	// "the line the anchor is on" settle it, and there is no such concept here
	// in any case: an anchor is a plain substring search over the page text
	// (see Resolver.anchor) and nothing in this package is line-aware. A rule
	// that wants to assert its anchor ends a line says so by putting the "\n"
	// in the anchor, which is how the p67 parts pin their column header.
	// Declaring the ordinal states which occurrence the rule's author looked
	// at, which is the same discipline OmittedRows uses.
	SectionOrdinal int `yaml:"section_ordinal"`

	// LabelsFrom names an earlier part's page when this part carries no row
	// labels of its own. Budget Book p67 is exactly this: it continues p66's
	// schedule for four more fund groups with nothing but numbers, so row
	// identity is positional against p66's order.
	LabelsFrom int `yaml:"labels_from"`

	// OmittedRows lists rows THE DOCUMENT does not print on this part although
	// they are present in the rule's row order. Declaring them is mandatory:
	// without it a positional read shifts every label after the gap, which is a
	// silent mismapping rather than an error. The parser cannot detect this for
	// you — the count assertion at apply time can, and only if the declaration
	// is here to check against.
	//
	// It is about the DOCUMENT, never about our pipeline. This field once
	// carried "Licenses & Permits" for Budget Book p67, on the belief that the
	// page omits an all-zero row; `pdftotext -bbox` showed the page prints all
	// ten rows and that our extractor was deleting one (fisc-c00). Using this
	// field to absorb an extraction defect would launder a pipeline bug into a
	// permanent published claim about the city's budget — and RowLabel is part
	// of the fact id, so the mislabelled facts would be citable. If rows go
	// missing between the PDF and the artifact, fix extraction; do not declare
	// them here.
	//
	// Each entry names exactly one row, as a bare label or as the pair of
	// anchors that identifies it; see OmittedRow. An entry that names no row
	// is refused, and so is one that names more than one.
	OmittedRows []OmittedRow `yaml:"omitted_rows"`

	// Columns describe the value columns, left to right.
	Columns []Column `yaml:"columns"`

	// ColumnHeaders names the header the document prints over each column,
	// left to right, one entry per entry in Columns INCLUDING any marked
	// skip: true. Declaring it opts this part into the column-position guard:
	// the header line gives the page's column geometry, and every figure the
	// part reads must fall in the band of the column the rule assigned it to.
	//
	// Absent, the part is read exactly as it was before geometry existed. The
	// key is per-part rather than per-rule because a page can print more than
	// one schedule, and per-part rather than global because opting a part in
	// is a claim about a specific page that someone has looked at.
	//
	// The list is a claim about column ORDER, not just about column count.
	// Headers are matched left to right and each must occur AFTER the one
	// before it, so a page that dropped a column fails to resolve rather than
	// shifting every figure one place -- which is the failure this whole guard
	// exists to prevent, and one no count can detect.
	//
	// Repeats are expected and are not an error, which is the opposite of the
	// rule for Columns: Budget Book p66 prints "FY 2025-26" over the General
	// Fund and again over Enterprise Funds, so the header alone does not
	// identify a column and was never meant to. Columns are identified by
	// (fund_group, fiscal_year); these strings only say where on the page each
	// one is.
	//
	// An entry that parses as an amount is refused. A header that is a figure
	// would match a DATA row, and a grid built from a data row files that row's
	// own values perfectly and everything else by luck. A schedule whose
	// headers are bare years therefore has to name more of the header --
	// "FY 2026" rather than "2026". ("2024-25" is fine; it is not an amount.)
	//
	// AN ENTRY MAY BE null, AND ONLY FOR THE LAST COLUMN WHEN IT IS SKIPPED.
	// See ColumnHeader.
	ColumnHeaders ColumnHeaders `yaml:"column_headers"`

	// WrappedLabels are the printed fragments this part's page wraps onto a
	// line of their own, each written out verbatim.
	//
	// pdftotext -layout reproduces the printed page, and a long row label
	// wraps. The tail lands BETWEEN one row's last figure and the next row's
	// label, which is where checkGap refuses anything that is not whitespace:
	//
	//	Innovation & Economic Wages & Benefits   1,028,282  1,068,663 ...
	//	Devel
	//	                      Services & Supplies  1,610,950  2,592,436 ...
	//
	// skip: true cannot express it -- a skipped row still consumes one value
	// per column and these fragments carry none.
	//
	// A DECLARATION RATHER THAN A RELAXATION, and that is the whole design.
	// checkGap keeps refusing everything it refused before; the only text it
	// now admits is a fragment an author wrote down after reading the page,
	// matched in full rather than by pattern. Anything else between two rows
	// is still an unmapped row, which is what the guard is for.
	//
	// It is declared on the PART rather than on a Row because a fragment is
	// not reliably a row's own: Budget Book p167 wraps "INNOVATION & ECONOMIC
	// DEVELOPEMENT TOTAL" onto a second line too, and under one-rule-per-
	// division that TOTAL line belongs to no rule's row set.
	//
	// A declared fragment the page does not use is an error, for the same
	// reason a stated_total_delta that now ties exactly is one: the
	// declaration is a claim about the document, and a claim that has stopped
	// being true must be removed rather than left to pass silently.
	WrappedLabels []string `yaml:"wrapped_labels"`

	// StatedTotalDeltas declares columns where the total the DOCUMENT prints
	// is not the sum of the rows it totals, and by how much.
	//
	// "All column totals tie exactly" is a property of Budget Book pp.66-67
	// and NOT of this corpus. A correct label-anchored read of p127's thirteen
	// property-tax rows gives $58,179,467 against a printed $58,179,468, and
	// ten more blocks are off by <= $5 -- pp.111, 128, 129, 131, 135, 168, 172,
	// 173, 181, 183, every one of them in the FY2023-24 Actual column. That is
	// rounding in the city's own arithmetic, not in ours (fisc-2sd).
	//
	// This is a DECLARATION, not a tolerance, and the difference is the whole
	// point. There is no global epsilon and no per-rule fuzz: an author writes
	// down one column, one exact figure, and why. CheckTotals then accepts that
	// figure and no other -- a column off by a different amount fails, and so
	// does a column that now ties, because a declaration the document has
	// stopped needing is a stale claim about the city's arithmetic and should
	// surface rather than rot. Absent a declaration, exact equality still holds.
	StatedTotalDeltas []StatedTotalDelta `yaml:"stated_total_deltas"`
}

// ColumnHeader is one entry in a part's ColumnHeaders: the header a page prints
// over a column, or the explicit statement that it prints none.
//
// THE null ENTRY EXISTS BECAUSE A PAGE CAN PRINT A COLUMN WITH NO HEADER OVER
// IT, and the entry count is a check the guard cannot afford to give up.
// Budget Book p76 prints four year headers and, right of them, a footnote
// marker on every one of its 22 rows -- "(1)" through "(10)", which
// amount.Parse reads as parenthesised NEGATIVES. The marker column has to be
// declared, because labelledValues truncates a row to len(Columns) and at four
// the read stops before the marker, leaving checkGap to refuse "(10)" as
// unexplained text between rows. So the column is real and skipped, and there
// is no header anywhere on the page to name it with (fisc-wfi).
//
// The alternatives were both worse. Dropping the one-entry-per-column rule
// gives up exactly what the rule buys -- a short list builds a grid one band
// too narrow and files every figure right of the gap one place left, silently,
// with the value count still matching. Declaring the markers as
// Part.WrappedLabels would satisfy the guard by saying something untrue about
// the page: a footnote reference is not a wrapped fragment of the label above
// it, and all ten distinct markers would have to be listed.
//
// null IS RESTRICTED TO THE LAST COLUMN, and the restriction is what makes the
// weakened guard honest rather than merely convenient. A headerless column has
// no band, so no placement claim can be made about its token at all; past the
// last header there is simply nothing, whereas a headerless column BETWEEN two
// headers sits in a gap whose bounds are known and which the grid would happily
// have checked. Restricting it to the end means the guard never silently
// declines a check it could have made.
//
// What still guards the unbanded token: it must be printed on the same line as
// the row's other figures (columnGuard.checkRow), the row must yield exactly
// len(Columns) tokens, and checkGap still refuses anything unexplained between
// rows. What is given up is the band check on that one token, and only there.
type ColumnHeader struct {
	// Text is the header as the page prints it, empty when Unheaded.
	Text string
	// Unheaded says the page prints no header over this column. It is a
	// separate field rather than an empty Text so that a YAML entry of "" --
	// which is a typo, not a claim -- stays refused.
	Unheaded bool
}

// ColumnHeaders is a part's header list.
//
// It is a named slice with its own unmarshaler because yaml.v3 DROPS a null
// element from a sequence rather than decoding it: the element unmarshaler is
// never reached, and `["FY 2025-26", ~]` arrives as a one-entry list. Since the
// entry count is the column-count check, a silently shortened list is the exact
// failure this whole guard exists to prevent -- it would have been reported as
// "has 1 entries but the part has 2 columns", blaming the author for the
// decoder.
type ColumnHeaders []ColumnHeader

// UnmarshalYAML reads the sequence element by element, so a null keeps its
// place.
func (h *ColumnHeaders) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.SequenceNode {
		return fmt.Errorf("line %d: column_headers is a list of the headers the page "+
			"prints over this part's columns, left to right", n.Line)
	}
	out := make(ColumnHeaders, 0, len(n.Content))
	for _, e := range n.Content {
		var c ColumnHeader
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
func (c ColumnHeader) String() string {
	if c.Unheaded {
		return "null (the page prints no header here)"
	}
	return fmt.Sprintf("%q", c.Text)
}

// StatedTotalDelta is one column's declared discrepancy between the document's
// printed total and the rows beneath it.
type StatedTotalDelta struct {
	// Column is 1 based, matching how CheckTotals numbers columns when it
	// reports a mismatch, so a failure message can be turned into this
	// declaration without translating an index.
	Column int `yaml:"column"`

	// Cents is STATED MINUS MAPPED: how much the printed total exceeds the sum
	// of the rows. p127's FY2023-24 column prints $1 more than its rows add to,
	// so it declares 100. A document that printed LESS than its rows would
	// declare a negative.
	//
	// In cents, always, like every other amount in this project -- the rule's
	// `units` says how the page prints its figures, not how this is written.
	Cents amount.Cents `yaml:"delta_cents"`

	// Note is required and says why. A bare number here would be a tolerance
	// wearing a declaration's clothes; the reason someone looked at the page
	// and concluded "the city rounded" is the thing worth keeping.
	Note string `yaml:"note"`
}

// Column identifies one value column. Column identity is compound because a
// single header string cannot express it: these schedules stack a fund-group
// header row above a fiscal-year header row, so no one line of the page names
// a column completely.
type Column struct {
	FundGroup  string `yaml:"fund_group"`
	Fund       int    `yaml:"fund"`
	FiscalYear int    `yaml:"fiscal_year"`

	// Basis overrides the rule's basis for this column, which the four-column
	// revenue schedules need: the same row carries FY23-24 actual, FY24-25
	// revised, and two adopted years side by side.
	Basis Basis `yaml:"basis"`

	// Skip marks a column that is present in the text but should not produce
	// facts, so column positions still line up.
	Skip bool `yaml:"skip"`
}

// Counterpart is the far end of a figure that moves money between two funds.
//
// Every field is the counterpart leg's own: the category it classifies as, the
// kind it is, and the fund it belongs to. Nothing is inherited from the row,
// because a counterpart that defaulted to its row's classification would be a
// second fact saying the same thing about the same fund, which is a
// double-count wearing a different id.
type Counterpart struct {
	Category  string `yaml:"category"`
	Kind      Kind   `yaml:"kind"`
	Fund      int    `yaml:"fund"`
	FundGroup string `yaml:"fund_group"`
}

// Column returns the column the counterpart leg's fact is filed under: the
// printed column, with the fund dimension replaced by the counterpart's own.
// The fiscal year and basis are the printed column's, because both legs are
// the same figure in the same year on the same basis.
func (c Counterpart) Column(printed Column) Column {
	printed.FundGroup = c.FundGroup
	printed.Fund = c.Fund
	return printed
}

// Row is one labelled line and the classification it maps to.
type Row struct {
	Label string `yaml:"label"`

	// LabelTail is a SECOND published anchor that identifies the row together
	// with Label, for a page whose row identity is two printed fields with an
	// arbitrary gap between them. Budget Book p76 prints
	//
	//	Transfer From Low Income Hsng          to General Fund   257,012 ...
	//	Transfer From Home Grant               to General Fund     8,932 ...
	//
	// where neither field alone identifies the row: the source is not unique
	// and the destination is not either. A single Label spanning both would
	// have to spell out the run of spaces between them -- ten on one row,
	// fifteen on the next -- which is the kerning of one release written into
	// a rule file, and testdata/spine.yaml's doctrine forbids exactly that.
	//
	// The gap stays CHECKABLE rather than becoming a wildcard: the text
	// between the two anchors must be whitespace and nothing else. That is
	// what makes this a two-anchor match rather than a relaxation of the
	// substring match -- a word appearing between the fields is still a row
	// the rule has not accounted for.
	LabelTail string `yaml:"label_tail"`

	Category   string `yaml:"category"`
	Department string `yaml:"department"`
	Sign       Sign   `yaml:"sign"`

	// Fund and FundGroup override the column's fund dimension for this row,
	// exactly as Kind overrides the rule's kind: the thing that knows says so.
	//
	// The fund is normally a property of the COLUMN, because a schedule's
	// columns are its (fund, year) pairs and its rows are the categories.
	// Budget Book p76 inverts that. Its columns are years and its five
	// sections are the fund groups RECEIVING money, so a row's own end fits
	// the column fine -- but the fund PAYING varies row by row inside a
	// section. Measured: three of the five sections are mixed, Enterprise
	// running general, enterprise x5 (fisc-aes). Without a per-row fund the
	// only way to express that is one rule per contiguous run of same-group
	// rows, which makes rule boundaries depend on the city's row ORDER, so
	// inserting a row re-partitions the rules.
	Fund      int    `yaml:"fund"`
	FundGroup string `yaml:"fund_group"`

	// Counterpart is the other end of a figure that moves money between two
	// funds, and it is what lets ONE printed figure be evidence for TWO facts.
	//
	// p76 prints "Transfer From Low Income Hsng  to General Fund  257,012".
	// That single figure is 257,012 leaving fund 200 and 257,012 arriving at
	// fund 100. fact.Fact carries one fund field, so one fact cannot hold the
	// pair (fisc-4rh); two facts can, and they cite the SAME doc_id, page,
	// offset and token, because one printed figure is the provenance for both
	// directions of one movement. That is not duplicated evidence.
	//
	// The two ids differ without any change to fact.MakeID: the legs carry
	// different categories, so their row_paths differ, and different funds, so
	// their column_paths differ. Both differences carry the MEANING of the
	// difference rather than being a discriminator added to avoid a clash.
	//
	// It also disposes of p76's three continuation rows -- "to Wastewater
	// Replacement", "to 2022 COPS", "to Downtown LMD", whose "Transfer From"
	// carries over from the row above. A rule DECLARES the payer here rather
	// than a reader inferring it from a neighbouring line.
	Counterpart *Counterpart `yaml:"counterpart"`

	// Kind overrides the rule's kind for this row, exactly as Column.Basis
	// overrides Rule.Basis: the thing that knows says so.
	//
	// Eleven or more fund blocks on Budget Book pp.131-140 print revenue rows
	// AND a Transfers In row inside ONE printed `Total <fund>` -- Stormwater,
	// Wastewater, Water and the rest. Kind is per RULE, so without this those
	// printed totals cannot be used as total_row AT ALL: not "are verbose to
	// express", cannot be expressed. The document would stop checking our work
	// on eleven of its own totals, which is the most valuable check this
	// project has.
	//
	// It does NOT make CheckTotals kind-aware, and that is deliberate. Those
	// eleven totals cover BOTH kinds, so kind-BLINDNESS is exactly what they
	// need; a check that summed only the rule's own kind would make p131's
	// Stormwater block stop tying to `Total Stormwater`, whose FY2025-26
	// column is 1,169,000 of charges plus 3,247,000 of Transfers In and is
	// unreachable without the transfer row. (fisc-uli's note names Airport for
	// this; Airport prints no Transfers In at all, and the 3,247,000 is
	// Stormwater's.) Where a printed total covers only SOME of the
	// kinds beneath it, the rule says so on the total_row declaration -- see
	// TotalRowKinds -- because the total row is the thing that knows which
	// rows it covers.
	Kind Kind `yaml:"kind"`

	// Skip marks a row that occupies a position but produces no facts, such
	// as a subtotal that would double-count.
	Skip bool `yaml:"skip"`
}

// Identity is the row's identity within its rule: both anchors when it has
// two, the label alone when it has one. Uniqueness is enforced on this rather
// than on Label, because two rows may legitimately share a source fund and
// differ only in their destination.
func (r Row) Identity() string {
	if r.LabelTail == "" {
		return r.Label
	}
	return r.Label + "\x1f" + r.LabelTail
}

// totalCovers says whether a row of this kind is one the printed total_row
// covers. With no TotalRowKinds declared every row is, which is the kind-blind
// reading the mixed fund blocks on Budget Book pp.131-140 depend on.
func (r *Rule) totalCovers(k Kind) bool {
	if len(r.TotalRowKinds) == 0 {
		return true
	}
	return slices.Contains(r.TotalRowKinds, k)
}

// EffectiveKind is the kind this row's facts carry: its own override where it
// declares one, the rule's otherwise.
func (r Row) EffectiveKind(rule *Rule) Kind {
	if r.Kind != "" {
		return r.Kind
	}
	return rule.Kind
}

// EffectiveColumn is the column a row's own fact is filed under: the printed
// column, with the row's fund overrides applied where it declares them.
//
// The override is per FIELD and not all-or-nothing. A schedule may pin the
// group on the column and vary only the fund number per row, or the reverse,
// and a row that declares neither is the ordinary case that every schedule
// but p76 is.
func (r Row) EffectiveColumn(printed Column) Column {
	if r.FundGroup != "" {
		printed.FundGroup = r.FundGroup
	}
	if r.Fund != 0 {
		printed.Fund = r.Fund
	}
	return printed
}

// PrintedLabel is what the document printed for this row, as the fact's
// row_label. Both anchors are published text, so a two-anchor row publishes
// the pair -- joined by a single space, because the run the page prints
// between them is typesetting rather than content, and the whole point of
// LabelTail is that a rule may not assert it.
func (r Row) PrintedLabel() string {
	if r.LabelTail == "" {
		return r.Label
	}
	return r.Label + " " + r.LabelTail
}

// OmittedRow names one of the rule's rows that a part does not print. It is
// written the way the row itself is written, and for the same reason:
//
//	omitted_rows: ["Licenses & Permits"]
//	omitted_rows:
//	  - {label: "Transfer From Wastewater", label_tail: "to Stormwater"}
//
// A bare string names a row identified by one anchor; the pair names a row
// identified by two (see Row.LabelTail). The pair is not optional sugar. Row
// identity within a rule is Row.Identity(), which joins the two anchors on a
// \x1f no author would type — YAML "\x1f" produces one, and such an entry does
// match, so this is a convention rather than an impossibility — and spelling
// the pair as one string instead would
// mean asserting the run of spaces the page prints between the fields — the
// one thing Row.LabelTail exists in order not to do. Before this form existed,
// a row carrying a label_tail could not be named here at all, while
// OmittedRows' own doc called the declaration mandatory (fisc-gtv).
type OmittedRow struct {
	Label     string `yaml:"label"`
	LabelTail string `yaml:"label_tail"`
}

// UnmarshalYAML accepts either spelling.
func (o *OmittedRow) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.ScalarNode {
		return n.Decode(&o.Label)
	}
	if n.Kind != yaml.MappingNode {
		return fmt.Errorf("line %d: an omitted_rows entry is a label, or a "+
			"{label, label_tail} pair naming a row's two anchors", n.Line)
	}
	// The decoder's KnownFields does not reach a custom unmarshaler, so
	// unknown keys are refused here instead. A typo'd `label_tial` would
	// otherwise be dropped, and the entry would name a different row than the
	// author wrote — silently, since the label alone may well match one.
	for i := 0; i+1 < len(n.Content); i += 2 {
		switch k := n.Content[i].Value; k {
		case "label", "label_tail":
		default:
			return fmt.Errorf("line %d: unknown field %q in an omitted_rows "+
				"entry; it takes label and label_tail", n.Content[i].Line, k)
		}
	}
	// The alias sheds this method, so the struct tags above do the decoding
	// rather than being decoration.
	type entry OmittedRow
	var e entry
	if err := n.Decode(&e); err != nil {
		return err
	}
	*o = OmittedRow(e)
	return nil
}

// row is the Row this entry names, so identity and printed form are computed
// by Row's own methods and cannot drift from them.
func (o OmittedRow) row() Row { return Row{Label: o.Label, LabelTail: o.LabelTail} }

// Identity is the row identity this entry claims, in the same key space the
// rule's rows are indexed by.
func (o OmittedRow) Identity() string { return o.row().Identity() }

// PrintedLabel is how the entry reads in a message, matching what the row
// would have published had the page printed it.
func (o OmittedRow) PrintedLabel() string { return o.row().PrintedLabel() }

// LabelledPart returns the part that carries row labels for p, which is p
// itself unless p declares LabelsFrom.
func (r *Rule) LabelledPart(p *Part) *Part {
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

// ActiveRows returns the rule's rows minus those omitted on this part, in
// order. This is the sequence a positional read of the part must line up
// against.
//
// The result is always a fresh slice. Returning r.Rows directly on the
// no-omissions path would alias the rule, so a caller that wrote through the
// result would mutate the rule on pages without omissions and not on pages
// with them — a difference that only shows up on some pages.
func (r *Rule) ActiveRows(p *Part) []Row {
	if len(p.OmittedRows) == 0 {
		return append([]Row(nil), r.Rows...)
	}
	omitted := omittedSet(p)
	out := make([]Row, 0, len(r.Rows))
	for _, row := range r.Rows {
		if !omitted[row.Identity()] {
			out = append(out, row)
		}
	}
	return out
}

// omittedSet is the part's declared omissions as a set of row identities. It
// is keyed on Identity() and not on Label because a Label is not unique within
// a rule once a row may carry a second anchor: filtering on the bare label
// would drop EVERY row sharing it, so declaring one of p76's two "Transfer
// From Wastewater" rows absent would silently delete the other as well.
func omittedSet(p *Part) map[string]bool {
	set := make(map[string]bool, len(p.OmittedRows))
	for _, o := range p.OmittedRows {
		set[o.Identity()] = true
	}
	return set
}

// ExpectedValues is how many numbers a positional read of this part must find:
// one per active row per column. A mismatch means the page's shape has changed
// and the rule can no longer be trusted.
func (r *Rule) ExpectedValues(p *Part) int {
	return len(r.ActiveRows(p)) * len(p.Columns)
}
