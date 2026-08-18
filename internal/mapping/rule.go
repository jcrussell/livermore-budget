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

import "github.com/jcrussell/livermore-budget/internal/amount"

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

func (k Kind) valid() bool {
	switch k {
	case KindRevenue, KindExpenditure, KindTransferIn, KindTransferOut, KindFundBalance:
		return true
	}
	return false
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

	// Path is the file this was read from, for error messages. Not serialized.
	Path string `yaml:"-"`
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
	OmittedRows []string `yaml:"omitted_rows"`

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
	ColumnHeaders []string `yaml:"column_headers"`
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

// Row is one labelled line and the classification it maps to.
type Row struct {
	Label      string `yaml:"label"`
	Category   string `yaml:"category"`
	Department string `yaml:"department"`
	Sign       Sign   `yaml:"sign"`

	// Skip marks a row that occupies a position but produces no facts, such
	// as a subtotal that would double-count.
	Skip bool `yaml:"skip"`
}

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
	omitted := make(map[string]bool, len(p.OmittedRows))
	for _, l := range p.OmittedRows {
		omitted[l] = true
	}
	out := make([]Row, 0, len(r.Rows))
	for _, row := range r.Rows {
		if !omitted[row.Label] {
			out = append(out, row)
		}
	}
	return out
}

// ExpectedValues is how many numbers a positional read of this part must find:
// one per active row per column. A mismatch means the page's shape has changed
// and the rule can no longer be trusted.
func (r *Rule) ExpectedValues(p *Part) int {
	return len(r.ActiveRows(p)) * len(p.Columns)
}
