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

// SchemaVersion is the only rule-file version this package understands. A file
// declaring anything else is refused rather than guessed at.
//
// Adding a field is free only while nothing is published. Once a rule file
// ships, KnownFields(true) means an older binary treats a newly added key as a
// fatal parse error rather than reaching the "upgrade the binary" hint in
// validate, so the first field added after publication must bump this.
const SchemaVersion = 1

// Substrate is where a rule reads its values from.
type Substrate string

const (
	// SubstrateText reads the page markdown. This is the primary path and
	// what most rules use, but it is not a default: substrate must be stated
	// explicitly, because table extraction misses 46% of the Budget Book's
	// money-bearing pages, including the citywide spine and the transfer
	// schedule.
	SubstrateText Substrate = "text"
	// SubstrateTable reads a detected table's cell grid, where one exists and
	// is faithful.
	SubstrateTable Substrate = "table"
	// SubstrateManual carries figures transcribed by hand, for pages no
	// parser can read — the transfer schedule at Budget Book p76 extracts as
	// four disjoint text blocks with mismatched cardinalities. Manual rules
	// must declare checksums so the transcription is machine-checkable.
	SubstrateManual Substrate = "manual"
)

func (s Substrate) valid() bool {
	switch s {
	case SubstrateText, SubstrateTable, SubstrateManual:
		return true
	}
	return false
}

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
	ID        string       `yaml:"id"`
	Substrate Substrate    `yaml:"substrate"`
	Kind      Kind         `yaml:"kind"`
	Basis     Basis        `yaml:"basis"`
	Scope     string       `yaml:"scope"`
	Units     amount.Units `yaml:"units"`

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
	// unique-match rule cannot resolve the citywide spine at all. Nor can a
	// line anchor: the whole schedule, both sections and both totals rows, is
	// on one line of extracted text. Declaring the ordinal states which one the
	// rule's author looked at, which is the same discipline OmittedRows uses.
	SectionOrdinal int `yaml:"section_ordinal"`

	// LabelsFrom names an earlier part's page when this part carries no row
	// labels of its own. Budget Book p67 is exactly this: it continues p66's
	// schedule for four more fund groups with nothing but numbers, so row
	// identity is positional against p66's order.
	LabelsFrom int `yaml:"labels_from"`

	// OmittedRows lists rows absent from THIS part although present in the
	// rule's row order. Extraction drops rows that are entirely blank, and
	// p67 omits "Licenses & Permits" because it is zero outside the General
	// Fund. Declaring them is mandatory: without it a positional read shifts
	// every label after the gap, which is a silent mismapping rather than an
	// error. The parser cannot detect this for you — the count assertion at
	// apply time can, and only if the declaration is here to check against.
	OmittedRows []string `yaml:"omitted_rows"`

	// Columns describe the value columns, left to right.
	Columns []Column `yaml:"columns"`

	// Table locates the grid this part reads, for a table rule. Required on a
	// table rule and refused on any other, because a text rule that carried a
	// table locator would silently ignore it.
	Table *TableLocator `yaml:"table"`

	// ExpectedContentHash is the integrity hash of the extracted artifact this
	// part was written against. It is to be checked after the part resolves,
	// so a mismatch reads as "the content changed" — a different failure from
	// "not found", deserving a diff rather than a candidate list.
	//
	// Nothing checks it yet. Resolution landed with fisc-mq4.2; the integrity
	// check is fisc-mq4.3. A rule that sets this field today is recording an
	// intent that is not yet enforced, so do not read a passing verify as
	// evidence that the content still matches.
	ExpectedContentHash string `yaml:"expected_content_hash"`
}

// TableLocator identifies one detected table. Identity is compound and
// content-independent on purpose: no single field is enough.
//
// Ordinal alone moves when the extractor finds one more table on the page.
// LabelFingerprint alone is not unique — 12 tables in the ACFR share one, and
// the largest fingerprint group in the corpus spans two documents, because a
// running page header extracts as a table. ContentHash cannot be an identity
// at all, which is why it lives on Part as ExpectedContentHash and answers a
// different question ("did it change?").
//
// Do not give this type an UnmarshalYAML method. yaml.v3 does not propagate
// KnownFields into a type's own unmarshaler, so a misspelled key here would be
// silently dropped — and a locator that quietly lost its fingerprint is exactly
// the failure this schema exists to prevent.
type TableLocator struct {
	// Ordinal is the table's 1-based position on the page, in the order
	// extract.py sorts them: by (page, y0, x0).
	Ordinal int `yaml:"ordinal"`

	// LabelFingerprint hashes only the non-numeric tokens of the grid, so it
	// survives a new fiscal year changing every figure.
	LabelFingerprint string `yaml:"label_fingerprint"`

	// BBoxCentroid is the midpoint of the table's bounding box, [x, y] in
	// points. It disambiguates the tables a fingerprint cannot: three tables on
	// ACFR p34 share the empty fingerprint, and nothing in the corpus collides
	// within a page once position is considered.
	BBoxCentroid []float64 `yaml:"bbox_centroid"`
}

// Column identifies one value column. Column identity is compound because a
// single header string cannot express it: these schedules stack a fund-group
// header row above a fiscal-year header row, and the extractor's Table.columns
// is empty for every table in the corpus.
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
