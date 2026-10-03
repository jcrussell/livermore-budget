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

// Quantity is the grammar a cell must satisfy, and it is not Kind: a Kind
// classifies a published fact in the flow model, a Quantity says what a CELL
// is before any fact exists. Every quantity but the amount default is read so
// the row stays whole, and publishes nothing (fisc-9tn4).
type Quantity string

// The quantities a cell can be. QuantityAmount is the default and the only
// quantity that publishes; a rule file never writes it.
const (
	QuantityAmount        Quantity = "amount"
	QuantityAmountPerUnit Quantity = "amount_per_unit"
	QuantityPercentage    Quantity = "percentage"
	QuantityNumber        Quantity = "number"
)

// quantities is the closed set, in the order the constants declare it — the
// same one-list discipline as kinds, so a fifth value cannot be added to one
// spelling and missed by the other.
var quantities = []Quantity{
	QuantityAmount,
	QuantityAmountPerUnit,
	QuantityPercentage,
	QuantityNumber,
}

func (q Quantity) valid() bool { return slices.Contains(quantities, q) }

// quantityList spells the declarable values for an error message. It excludes
// QuantityAmount, which the parser refuses to see written: an undeclared cell
// already parses amounts.
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

var bases = []Basis{BasisAdopted, BasisRevised, BasisActual, BasisAudited, BasisProjected}

// Bases is every basis a column may be of.
func Bases() []Basis { return slices.Clone(bases) }

func basisList() string {
	s := make([]string, len(bases))
	for i, b := range bases {
		s[i] = string(b)
	}
	return strings.Join(s, ", ")
}

// Valid reports whether b is one of [Bases].
func (b Basis) Valid() bool { return slices.Contains(bases, b) }

// Sign says how a row relates to its category, and it is never an instruction
// to negate: AmountCents is always the figure as the document printed it.
//
// Two things need saying and they are different. SignContra marks a row that
// REDUCES its category rather than adding to it. SignNetted marks a row the
// document prints against its KIND's direction. Both leave the amount alone.
type Sign string

const (
	// SignPositive is the default.
	SignPositive Sign = "positive"
	// SignContra marks a deduction booked as negative revenue — the ERAF and
	// RPTTF property-tax shifts on Budget Book p127 are ~26% of gross
	// property tax. Where the negative goes is the consumer's: a category-grain
	// view nets it into its parent, a view drawing the printed row carries it
	// as a negative value on that row's own link.
	SignContra Sign = "contra"
	// SignNetted marks a row the document prints with the OPPOSITE ORIENTATION
	// to its kind's convention, because it sits inside a block that sums to a
	// net figure.
	//
	// It is not SignContra one more time, and the difference is the one this
	// field exists to carry. A contra row is a deduction INSIDE its own
	// category: p127's ERAF reduces property tax, and summing it with its
	// siblings is exactly right. A netted row is the SAME quantity pointing the
	// other way: ACFR p41 prints Transfers (out) as (25.72) because its block
	// sums to a net Other Financing Sources (Uses), while Budget Book p66 prints
	// TRANSFER OUT as a positive magnitude in a uses column. Both are the money
	// leaving, both are published exactly as printed, and summing the two
	// together cancels rather than accumulates.
	//
	// SignContra's own comment anticipated this: "If a document ever prints a
	// deduction as a positive number under a 'Less:' heading, that convention
	// needs its own field rather than an overload of this one." This is that
	// sentence's mirror image (fisc-fdxx).
	//
	// IT IS STILL NOT AN INSTRUCTION TO NEGATE. AmountCents remains the figure
	// as the document printed it; this says which way the document was facing.
	SignNetted Sign = "netted"
)

var signs = []Sign{SignPositive, SignContra, SignNetted}

// Signs is every sign a row may declare; a row declaring none is positive.
func Signs() []Sign { return slices.Clone(signs) }

func (s Sign) valid() bool { return s == "" || slices.Contains(signs, s) }

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

	// Kinds names the kinds the covered rules span, and is declared ONLY where
	// they span more than one.
	//
	// WHAT IT CANNOT DO, said first because the name invites the opposite
	// reading: it cannot catch a wrong kind. It is required to equal the set the
	// covered rules already carry, so it can only ever restate them. What it
	// buys is that a mixed-kind rollup becomes a sentence someone wrote down —
	// the difference between a statement and an accident.
	//
	// WHY MIXED IS NOT SIMPLY REFUSED. p140 prints "Total Sources", which is
	// literally revenue plus transfers in — a real printed line over two kinds,
	// declared Unassertable today for an unrelated reason (fisc-wev). Refusing
	// mixed kinds outright would choose a rule the corpus has not asked for, and
	// would have to be unpicked the first time such a total became assertable.
	// Scope is the opposite case and IS refused: see validateRollups.
	//
	// It compares Rule.Kind and not the effective kinds of rows. A rule's kind
	// is the schedule's own claim about what it maps; a Row.Kind override is
	// about one printed line, and the eleven mixed fund blocks on pp.131-140
	// carry a Transfers In row inside a revenue rule without making that rule a
	// transfer schedule. No rollup covers such a rule today, so this is a
	// statement of intent rather than a measurement of the corpus.
	//
	// Like Rule.TotalRowKinds, nothing in mappings/ declares it yet.
	Kinds []Kind `yaml:"kinds"`

	// Note records why this rollup looks the way it does.
	Note string `yaml:"note"`
}

// Rule maps a contiguous block of rows into facts.
type Rule struct {
	ID    string `yaml:"id"`
	Kind  Kind   `yaml:"kind"`
	Basis Basis  `yaml:"basis"`
	Scope string `yaml:"scope"`
	// Grain names the lattice level this rule's figures are totals at: which
	// of fund_group, fund, department and category the table has an axis for.
	// Required on a rule that publishes a fact and refused on one that
	// publishes none, since nothing could check it. The parser checks presence
	// only; internal/structure checks the name against the facts.
	Grain string       `yaml:"grain"`
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

	// TotalRowAbove says the document prints the total_row ABOVE its own rows
	// rather than below them, and that the total_row label is therefore also
	// the part's section anchor.
	//
	// READ THIS AS "the stated totals are on the section anchor's own line".
	// Under this flag total_row does NO independent resolution: totalAnchor
	// returns the block's start, which Block already derived from the section
	// anchor, so total_row's usual "does not occur after the block" path is
	// unreachable for such a rule. The field is still required, and still
	// refused as a row label, because it is what names the printed line for a
	// reader of the rule and what a failure message reports against. A reader
	// who takes it for a SECOND, independent anchor has it wrong.
	//
	// It exists for ACFR p41, which prints "General Government:  18.45" and
	// then the five divisions summing to 18.44 beneath it. Every schedule
	// mapped before it printed its total below its rows, so nothing needed
	// otherwise, and two separate guards stood in the way: totalAnchor
	// searched only text after the block, and checkGap's leading arm refuses
	// any digit before the first row. Both are opened by this flag and by
	// nothing else -- see fisc-h96o.
	//
	// The section-anchor requirement is what keeps it from being a loosening.
	// Without it the flag would mean "look somewhere above", which no guard
	// could bound; with it the total is exactly one line, at a position the
	// resolver already had to find, and checkGap's refusal still stands over
	// every line after that one.
	TotalRowAbove bool `yaml:"total_row_above"`

	// TotalRowTail is the rest of the total's label where the page wraps it
	// onto the next line, verbatim: p0175 prints "Total County Meas BB-" with
	// the figures and "Bike/Pedestrian" alone beneath. total_row still anchors
	// the figures; totalAnchor refuses the rule unless the next line is this
	// text and nothing else, so the tail is read off the page rather than
	// asserted about it. See WrappedTotalLabel.
	TotalRowTail string `yaml:"total_row_tail"`

	// PrintedDecimals is how many decimal places this rule's page PRINTS its
	// figures to, and declaring it is what gives CheckTotals a tolerance
	// derived from the document instead of from an author's judgement.
	//
	// THE UNIT IS THE LEAST SIGNIFICANT PRINTED DIGIT, NOT THE CAPTION'S WORD.
	// ACFR p41 is captioned "(in Millions)" and prints two decimals, so its
	// unit is $10,000; reading the caption instead would hand a five-term sum
	// $2,500,000 of slack rather than $25,000. A column then ties when
	//
	//	2*|stated - mapped| <= DigitCents(units, printed_decimals) * n_terms
	//
	// -- half a unit per row summed, which is the most a correctly-read set of
	// rounded figures can be out by. The printed TOTAL's own half unit is
	// deliberately NOT added: leaving it out makes the bound tighter than the
	// worst case, which fails closed, and a page that misses by (n+1)/2 units
	// should be read again rather than have the bound widened to fit it.
	//
	// IT IS A POINTER SO THAT ABSENT AND ZERO ARE DIFFERENT CLAIMS. Absent
	// means no tolerance at all, which is what all 123 Budget Book rules want
	// and what this project's arithmetic has always assumed. Zero is a real
	// declaration -- a page printing whole millions, whose unit is $1,000,000 --
	// and an int would have silently read it as "no tolerance".
	//
	// FOUR THINGS KEEP IT FROM BECOMING A GLOBAL EPSILON, which is the thing
	// StatedTotalDeltas' doc comment below is right to refuse:
	//
	//   - It is refused on units: dollars. A dollar-precision branch would have
	//     zero consumers in this corpus, and the discrepancies it would appear
	//     to cover are not rounding: p127's Total Property Taxes is $1 over 13
	//     rows and pp.168-170 carry six more, all of which are the city's own
	//     arithmetic and belong in stated_total_deltas by decision (fisc-2sd).
	//   - It is refused on a rule that also declares a stated_total_delta. The
	//     two mechanisms are disjoint by construction rather than by prose:
	//     one names an exact figure and the other bounds an unnamed one, and a
	//     rule reaching for both is asking for a declaration it can hide inside.
	//   - It must describe the page. Every token that is SUMMED INTO A COMPARED
	//     COLUMN is checked against it, and a declaration no such token
	//     justifies is refused. Not every token the rule reads: a skipped row or
	//     column never reaches the witness (Values drops it), and a row the
	//     total_row does not cover is filtered out of the sum by totalCovers.
	//     Both are outside the comparison the tolerance applies to, so
	//     witnessing them could only let a declaration pass on precision the
	//     compared figures do not have.
	//   - It must be NEEDED. A rule whose columns all tie exactly is refused,
	//     the same way a stated_total_delta that now ties exactly is refused --
	//     because a declaration that has stopped doing anything is the one shape
	//     a declaration in this repository must not have.
	//
	// fisc-1wr.2 is the tier this implements.
	PrintedDecimals *int `yaml:"printed_decimals"`

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

	// SubtotalChain names a run of rules whose rows are one printed table for
	// the purpose of its subtotal rows (Row.Subtotal), read in file order. A
	// fund block on Budget Book pp.224-235 can start on one page pair and
	// print its total on the next, and each page pair is its own rule because
	// its continuation page is read positionally against it. Absent, a rule
	// with subtotal rows is a chain of its own.
	SubtotalChain string `yaml:"subtotal_chain"`

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

	// RowLabelsNameFunds declares that every row of this rule is labelled with
	// the printed name of the fund it carries, so row-funds-match-their-anchors
	// can read that name off the page and check the hand-typed number against
	// it.
	//
	// IT IS OPT-IN FOR THE SAME REASON Part.ColumnHeaders IS: declaring it is a
	// claim about a specific schedule that someone has looked at, rather than
	// something a resolver infers.
	//
	// THE OBVIOUS ARGUMENT FOR OPT-IN IS FALSE, and the measurement is kept
	// because it is the opposite of what that argument assumes. The argument runs
	// that a corpus-wide arm "would go from 78 subjects to thousands of which
	// only 78 were ever intended". Measured over the committed corpus: 429 rows
	// declared, 400 active and non-skipped, and exactly 78 of those 400 resolve
	// through registry.FundByLabel — the 78 this declaration covers. ZERO
	// accidental hits, and "thousands" is not reachable from a corpus of 429 rows
	// at all. p127's "Current Year - Secured", p167's object categories and the
	// spine's "Wages & Benefits" all miss.
	//
	// SO THE ARGUMENT IS NOT ABOUT TODAY'S COUNT. It is that an inferred claim
	// does not stay true: the next schedule mapped may print a row label that
	// happens to be a fund name, and a resolver guessing would silently check it
	// against a fund nobody declared, with no line in the rule file to review.
	// A declaration is a statement whose author can be asked. That reasoning
	// holds at 429 rows and at 4,290; the count never was the reason.
	//
	// WHY THE RULE AND NOT THE PART. Rule.Rows is one list shared by every part,
	// and the check that reads this iterates Rule.ActiveRows unioned across
	// parts precisely so a multi-part schedule's rows are visited once rather
	// than once per part — so it holds a rule and no part at the point it needs
	// this, and a per-part spelling would not be reachable there. (Not because
	// of labels_from: no rule carrying this declares one. Budget Book pp.85-125
	// straddles two parts, each row placed on its page by Row.Page.)
	//
	// A BARE FUND NAME NAMES A FUND AND NAMES NO DIRECTION, which is why this is
	// a separate declaration from the "Transfer From X to Y" anchors p76 prints.
	// Those carry a verb phrase, so the check reads which END of the movement
	// each fund sits at; these carry none, so it can assert identity and must
	// not assert direction.
	RowLabelsNameFunds bool `yaml:"row_labels_name_funds"`

	// RowLabelsAreFundNumbers declares that every row's label is the printed
	// number of the fund the row declares, and that on a row with a
	// counterpart the first field of its label_tail is the printed number of
	// the counterpart's fund. Budget Book p222 prints its receiving CIP fund
	// and its transferring operating fund as two number columns in that order,
	// so row-funds-match-their-anchors holds both ends, direction included.
	RowLabelsAreFundNumbers bool `yaml:"row_labels_are_fund_numbers"`

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
	// at, which is the same discipline OmittedCells uses.
	SectionOrdinal int `yaml:"section_ordinal"`

	// StopAtOrdinal picks which occurrence of StopAt, counted from the block's
	// start, ends the block, 1 based. Absent keeps StopAt's first occurrence.
	//
	// It exists for a page with no text between its figures. Budget Book p81
	// continues p80's debt-service rows with nothing but amounts, and its first
	// row is "$"-prefixed exactly as its total is, so the "$" that p67's parts
	// stop at is the wrong line on p81 and no other string marks the total.
	// The ordinal is a count the author made of the page, and a wrong one
	// moves the block's end onto a row line, where the value count or the
	// stated total refuses it.
	StopAtOrdinal int `yaml:"stop_at_ordinal"`

	// LabelsFrom names an earlier part's page when this part carries no row
	// labels of its own. Budget Book p67 is exactly this: it continues p66's
	// schedule for four more fund groups with nothing but numbers, so row
	// identity is positional against p66's order.
	LabelsFrom int `yaml:"labels_from"`

	// OmittedCells lists cells THE DOCUMENT leaves blank in rows it prints.
	// Budget Book p207 prints five figures on each of County Measure D's,
	// Wastewater's and Water's lines and nothing under Reserve
	// Increase/(Use), where every other row prints a figure or a "-". A blank
	// is absent, not zero, so the cell yields no Value and is reported as an
	// Omission.
	//
	// It is about the DOCUMENT, never about our pipeline. A declaration that
	// absorbed an extraction defect would launder a pipeline bug into a
	// permanent published claim about the city's budget -- and RowLabel is
	// part of the fact id, so the mislabelled facts would be citable. If cells
	// go missing between the PDF and the artifact, fix extraction; do not
	// declare them here.
	//
	// A row the page does not print at all is not a blank: it is printed by
	// another page of the same table, and Row.Page says which.
	//
	// The row is read with its tokens filed left to right under the columns
	// it does print, so this part must declare column_headers: the guard is
	// what holds each token to the band of the column it is filed under, and
	// what refuses a figure printed under a column declared blank.
	OmittedCells []omittedCell `yaml:"omitted_cells"`

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
	ColumnHeaders columnHeaders `yaml:"column_headers"`

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
	// A gap may hold several fragments, one to a printed line, each declared
	// on its own: p222 ends one row's description and begins the next row's
	// between their figures. A line in the gap that is not declared refuses
	// the whole gap.
	//
	// A declared fragment the page does not use is an error, for the same
	// reason a stated_total_delta that now ties exactly is one: the
	// declaration is a claim about the document, and a claim that has stopped
	// being true must be removed rather than left to pass silently.
	WrappedLabels []string `yaml:"wrapped_labels"`

	// Headings are the section headings this part's page prints on a line of
	// their own between rows, each written out verbatim. Budget Book p186
	// prints "Special Revenue Funds" alone between "Total City Budget"'s
	// figures and "Low Income Housing Fund", opening the detail that follows.
	//
	// A heading is not a wrapped label: it is no row's label broken onto a
	// second line, and declaring it as one would be a false claim about the
	// page. It is matched exactly as a wrapped label is, one printed line at a
	// time, and a gap may mix both with unmapped_text figures so long as every
	// line is declared -- p190 prints a footnote marker "1" alone on the line
	// above its "Capital Improvement Program Funds" heading. A declared heading
	// the part never uses is refused.
	//
	// A HEADING MAY EQUAL A ROW LABEL OF THE RULE, because p186's does:
	// "Special Revenue Funds" is also the summary row higher up the same block.
	// Refusing the collision would leave that page unmappable. It cannot
	// swallow the row: a gap's lines are matched whole, so a line carrying the
	// row's figures is never a heading.
	Headings []string `yaml:"headings"`

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
	// point: an author writes down one column, one exact figure, and why.
	// CheckTotals then accepts that figure and no other -- a column off by a
	// different amount fails, and so does a column that now ties, because a
	// declaration the document has stopped needing is a stale claim about the
	// city's arithmetic and should surface rather than rot.
	//
	// THERE IS STILL NO GLOBAL EPSILON. What there now is, and what this
	// comment used to deny, is a per-rule tolerance -- Rule.PrintedDecimals,
	// which sizes itself from the page's own printed precision and the number
	// of rows summed (fisc-1wr.2). The sentence here read "no global epsilon
	// and no per-rule fuzz" until that landed; it is corrected rather than
	// deleted because the distinction it was drawing is the one that keeps the
	// two apart, and a reader needs it more now that both exist:
	//
	//   - A delta names the exact figure a document is out by. It fails when
	//     the difference is anything else, including zero.
	//   - A tolerance bounds an unnamed difference, is derived rather than
	//     chosen, and is refused on dollar-precision tables -- which is every
	//     rule the sentence above was written about. p127's $1 over 13 rows is
	//     a delta and could never be a tolerance: ceil(13/2) is seven CENTS.
	//
	// A rule declaring both is refused, so no column is ever compared against
	// a named figure with slack around it. Absent both, exact equality holds.
	StatedTotalDeltas []statedTotalDelta `yaml:"stated_total_deltas"`

	// UnmappedText declares a FIGURE the page prints inside this block that
	// belongs to no row, with the reason it is there.
	//
	// ACFR p41 is the case. Its revenue block prints a bare "0.0" on a line of
	// its own between the Miscellaneous row's figures and the printed Total
	// Revenues -- an artefact of the city's spreadsheet, in the FY2024 column,
	// belonging to no printed label. Every other exit was closed: the trailing
	// arm refuses it, stop_at cannot name it because the parser refuses an
	// anchor amount.Parse accepts, and an eleventh skip: true row fails the
	// value count because the block ends immediately after it.
	//
	// IT IS SPLIT FROM WrappedLabels RATHER THAN FOLDED INTO IT because the two
	// assert different things about the document, and one of them would have
	// been false. A wrapped label says the page broke a row's LABEL onto its
	// own line. This says the page printed a FIGURE that is nobody's. Declaring
	// the second as the first was mechanically accepted until the same change
	// that added this field refused it -- measured, wrapped_labels: ["0.0"]
	// published all ten of p41's revenue rows with nothing objecting. So the
	// parser now requires a wrapped label NOT to parse as an amount and
	// requires this to parse as one, and the two declarations cannot be
	// substituted for each other in either direction.
	//
	// THAT SYMMETRY HAS A COST AND IT IS FILED, NOT HIDDEN. A page that wraps a
	// row LABEL which happens to be a bare number -- a fund number, a year, a
	// footnote index -- can now declare it as neither: wrapped_labels refuses it
	// as an amount, and this would accept it under a claim that is false of a
	// wrapped label. That is fisc-2jk's shape, reopened one case wide. No page
	// in the corpus has it (all five committed wrapped_labels entries are
	// words), and the parser cannot tell the two apart by looking, so it needs a
	// decision rather than a patch: fisc-xmsk.
	//
	// THIS IS THE WEAKEST DECLARATION CLASS IN THIS REPOSITORY, and a reader
	// should know it before reaching for it. A stated_total_delta is ratified
	// by exact arithmetic -- get the figure wrong and the column fails. A
	// wrapped label is bounded downstream, by the cursor advance and by the
	// per-row value count. This one is bounded by nothing but its own staleness
	// arm, and its first consumer has no arithmetic behind it at all: "0.0"
	// parses as zero and sits in a skipped column, so removing it from the read
	// changes no sum and CheckTotals could not redden on a wrong declaration
	// here. What actually keeps it narrow is that the text must match a
	// whole printed line of a gap and every row still has to be found by its own label. The Note is
	// therefore load-bearing rather than decorative.
	//
	// fisc-hcus.
	UnmappedText []unmappedText `yaml:"unmapped_text"`
}

// unmappedText is one figure a page prints inside a block that belongs to no
// row, and why.
type unmappedText struct {
	// Text is the figure exactly as the page prints it, matched against one
	// trimmed printed line of the gap it sits in -- the same matching
	// wrapped_labels and headings use, so the three behave alike where they
	// behave at all.
	Text string `yaml:"text"`

	// Note is required and says why the page prints it. Without one this would
	// be a silent skip wearing a declaration's clothes, which is the whole
	// difference between this and deleting the offending line from the read.
	//
	// Same requirement, and the same reason, as StatedTotalDelta.Note.
	Note string `yaml:"note"`
}

// columnHeader is one entry in a part's ColumnHeaders: the header a page prints
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
type columnHeader struct {
	// Text is the header as the page prints it, empty when Unheaded.
	Text string
	// Unheaded says the page prints no header over this column. It is a
	// separate field rather than an empty Text so that a YAML entry of "" --
	// which is a typo, not a claim -- stays refused.
	Unheaded bool
}

// columnHeaders is a part's header list.
//
// It is a named slice with its own unmarshaler because yaml.v3 DROPS a null
// element from a sequence rather than decoding it: the element unmarshaler is
// never reached, and `["FY 2025-26", ~]` arrives as a one-entry list. Since the
// entry count is the column-count check, a silently shortened list is the exact
// failure this whole guard exists to prevent -- it would have been reported as
// "has 1 entries but the part has 2 columns", blaming the author for the
// decoder.
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

	// Quantity declares that this column's cells are not amounts: they must
	// satisfy the named grammar (internal/quantity), which keeps the row whole
	// for checkGap, and they never publish. A non-amount column therefore
	// needs no fiscal_year and no distinct identity — no fact ever carries it.
	//
	// skip cannot express this, and the ordering is why: parseRow parses a
	// cell BEFORE it consults skip, deliberately, so a skipped "2.5%" still
	// fails the amount grammar. This is the channel fisc-9tn4 settled on.
	Quantity Quantity `yaml:"quantity"`

	// Category and Kind classify every figure in this column, for a page whose
	// columns are its budget LINES and whose rows are its funds: Budget Book
	// pp.186-209 print a fund per row and starting balance, Revenues, Transfers
	// In, Expenses, Transfers Out and the rest across. Kind falls back to the
	// rule's, as Row.Kind does. A rule carries its category on its rows or on
	// its columns and never both; see Value.Category.
	Category string `yaml:"category"`
	Kind     Kind   `yaml:"kind"`
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
	//
	// A tail of several words names several fields: each matches with any run
	// of spaces or tabs before the next, and never across a line break. p222
	// prints "601   600       Airport", so a label of "601" takes the tail
	// "600 Airport" without spelling the kerning.
	LabelTail string `yaml:"label_tail"`

	// Page is the one part that prints this row, for a table whose rows run
	// across a page break: Budget Book p167 prints General Services' Wages &
	// Benefits and p168 its Services & Supplies, Debt Services and Total.
	// Zero means every part prints the row, which is the ordinary case and
	// the only one for a rule with one part. The parser refuses a page that
	// names no part of the rule.
	//
	// It is on the row and not a row range on the part because a part may
	// print no row at all: Budget Book p130 prints Contributions Outsourced's
	// one row on p129 and only its Total on p130.
	//
	// It is about the DOCUMENT, never about our pipeline. Placing a row on
	// one page because extraction lost it from another launders a pipeline
	// bug into a published claim about the city's budget; fix extraction.
	Page int `yaml:"page"`

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

	// Subtotal declares a skipped row to be a subtotal the page prints, and
	// its level. CheckSubtotals holds each of its figures to the sum of the
	// rows above it that are not subtotals, since the last subtotal of the
	// same or a higher level, under the same column header -- so a level-2
	// fund total over level-1 subgroup subtotals sums the subgroups' rows,
	// and a level-1 total after it starts afresh. Every column is compared,
	// skipped ones included: a figure the rule does not publish is still one
	// the page printed and the subtotal adds.
	//
	// It exists for the subtotals a label-less continuation page prints
	// between its blocks, where no rule boundary can fall: Budget Book p225
	// continues p224's projects with nothing but figures. A printed total
	// that ends a block is still the rule's total_row.
	Subtotal int `yaml:"subtotal"`

	// SubtotalDeltas declares where a subtotal row prints a figure its rows do
	// not sum to, and by how much: Rule's stated_total_deltas, for a subtotal.
	// Budget Book p235's grand TOTAL prints FY2025-26 a dollar under its 219
	// rows. A declared column that ties exactly is refused as stale.
	SubtotalDeltas []SubtotalDelta `yaml:"subtotal_deltas"`

	// Quantity overrides every column's quantity for this row — the whole-row
	// arm of fisc-9tn4's decision. In the section's dominant orientation the
	// non-amount is a ROW spanning every year column (p169, p179, p189, p192),
	// which no column declaration can express. A non-amount row is read and
	// publishes nothing, so it needs no category.
	Quantity Quantity `yaml:"quantity"`
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

// WrappedTotalLabel is the whole printed label of the rule's total: total_row,
// and total_row_tail after it where the page wraps one, joined by JoinWrapped.
func (r *Rule) WrappedTotalLabel() string {
	if r.TotalRowTail == "" {
		return r.TotalRow
	}
	return JoinWrapped(r.TotalRow, r.TotalRowTail)
}

// JoinWrapped is a label the page breaks across two lines, rejoined, its
// tail's words single-spaced: the page's layout spacing is not part of a name.
// A label broken at a hyphen joins without a space, as p0175's "County Meas
// BB-" + "Bike/Pedestrian" does; a spaced separator dash ("Asset Seizure -")
// is a word of its own and keeps the space. A wrong join names no fund, which
// the heading check reports.
func JoinWrapped(head, tail string) string {
	tail = strings.Join(strings.Fields(tail), " ")
	if strings.HasSuffix(head, "-") && !strings.HasSuffix(head, " -") {
		return head + tail
	}
	return head + " " + tail
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
// column becomes a fact: the part prints the row (Row.OnPart) and the cell
// (it is not in omitted_cells), and Row.Publishes holds for the column.
// Every decision about the facts a row publishes reads it and nothing
// re-filters after it: kindsOf, and through it a sign, a counterpart, a
// grain and RowPublishes; checkCounterpart's collision loop; and
// Resolver.Values. Three decisions deliberately read something else. A row's
// category requirement reads cellAddressed, since a declared blank is an
// address too. validateOmittedCells counts the columns a row PRINTS, since a
// blank is a claim about the page. And a column's class (validateColumnClass)
// is about the column alone.
func (p *Part) cellPublishes(j int, row Row) bool {
	return p.cellAddressed(j, row) && !blankColumns(p)[row.Identity()][j]
}

// cellAddressed is cellPublishes before omitted_cells: the cell is a fact's
// address, printed or declared blank. A declared blank still needs the row's
// category, since internal/check reads it as the fact the cell would have
// been.
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
func (r *Rule) kindsOf(row Row) []Kind {
	var out []Kind
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
func (c Column) EffectiveKind(rule *Rule) Kind {
	if c.Kind != "" {
		return c.Kind
	}
	return rule.Kind
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
// names its page. Every placement decision calls it; validateRule alone reads
// Row.Page directly, to hold it to one part.
func (r Row) OnPart(p *Part) bool {
	return r.Page == 0 || r.Page == p.Page
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

// omittedCell names one blank cell: a row, by the anchors the row itself is
// written with, and the column_headers entry printed over the cell.
//
//	omitted_cells:
//	  - {label: "County Measure D", column: "Increase/(Use)", note: "..."}
//
// A bare label names a row identified by one anchor; with label_tail it names
// a row identified by two (see Row.LabelTail). The pair is not optional sugar:
// row identity within a rule is Row.Identity(), which joins the two anchors on
// a \x1f no author would type, and spelling the pair as one string would mean
// asserting the run of spaces the page prints between the fields -- the one
// thing Row.LabelTail exists in order not to do. A typo'd key is refused by
// the decoder's KnownFields, which this type keeps by having no unmarshaler.
type omittedCell struct {
	Label     string `yaml:"label"`
	LabelTail string `yaml:"label_tail"`
	Column    string `yaml:"column"`
	Note      string `yaml:"note"`
}

// row is the Row this entry names, so identity and printed form are computed
// by Row's own methods and cannot drift from them.
func (o omittedCell) row() Row { return Row{Label: o.Label, LabelTail: o.LabelTail} }

// describe is how the entry reads in a message.
func (o omittedCell) describe() string {
	return fmt.Sprintf("%q under %q", o.row().PrintedLabel(), o.Column)
}

// blankColumns is the part's declared blank cells as row identity to the
// columns blank on that row, each found by its header.
func blankColumns(p *Part) map[string]map[int]bool {
	if len(p.OmittedCells) == 0 {
		return nil
	}
	out := map[string]map[int]bool{}
	for _, o := range p.OmittedCells {
		for c, h := range p.ColumnHeaders {
			if !h.Unheaded && h.Text == o.Column {
				if out[o.row().Identity()] == nil {
					out[o.row().Identity()] = map[int]bool{}
				}
				out[o.row().Identity()][c] = true
			}
		}
	}
	return out
}

// printedColumns is the columns a row prints on this part, left to right: all
// of them, less any the part declares blank on it.
func printedColumns(p *Part, blank map[string]map[int]bool, row Row) []int {
	cols := make([]int, 0, len(p.Columns))
	for c := range p.Columns {
		if !blank[row.Identity()][c] {
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
// order. This is the sequence a positional read of the part must line up
// against.
//
// The result is always a fresh slice. Returning r.Rows directly when every
// row is on the part would alias the rule, so a caller that wrote through the
// result would mutate the rule on some pages and not on others — a difference
// that only shows up on some pages.
func (r *Rule) ActiveRows(p *Part) []Row {
	out := make([]Row, 0, len(r.Rows))
	for _, row := range r.Rows {
		if row.OnPart(p) {
			out = append(out, row)
		}
	}
	return out
}

// expectedValues is how many numbers a positional read of this part must find:
// one per active row per column it prints. A mismatch means the page's shape
// has changed and the rule can no longer be trusted.
func (r *Rule) expectedValues(p *Part) int {
	blank := blankColumns(p)
	n := 0
	for _, row := range r.ActiveRows(p) {
		n += len(printedColumns(p, blank, row))
	}
	return n
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
