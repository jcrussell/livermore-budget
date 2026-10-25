// Package fact turns resolved figures into the published fact record.
//
// A fact is one number the city printed, plus everything needed to find it
// again in the source document. Every field appears on every line, nothing
// omitted when empty, because facts.jsonl is read by diffing it.
package fact

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/jcrussell/livermore-budget/internal/amount"
	"github.com/jcrussell/livermore-budget/internal/hint"
	"github.com/jcrussell/livermore-budget/internal/mapping"
	"github.com/jcrussell/livermore-budget/internal/vocab"
)

// IDPrefix marks a fact id, so one is recognizable on sight.
const IDPrefix = "fisc-f-"

// idHexLen is how much of the sha256 an id carries: 48 bits, and a collision
// is a hard error (CheckUniqueIDs), not silent corruption.
const idHexLen = 12

// Fact is one published figure. Field order is the JSON key order and is
// part of the format.
type Fact struct {
	// ID is content-addressed over identity, never over the amount. See MakeID.
	ID    string `json:"id"`
	DocID string `json:"doc_id"`
	// Page and Offset locate the figure in the extracted page text, so a
	// citation can point at the number rather than at the document.
	Page   int `json:"page"`
	Offset int `json:"offset"`
	// Token is the text that was parsed, kept verbatim.
	Token  string `json:"token"`
	RuleID string `json:"rule_id"`

	Kind  vocab.Kind  `json:"kind"`
	Basis vocab.Basis `json:"basis"`
	Scope string      `json:"scope"`

	FiscalYear int `json:"fiscal_year"`
	// FiscalYearThrough is the last fiscal year of a figure printed for
	// several years at once (Budget Book p80's 2031-2035 debt service), and
	// null for a one-year figure. A reader taking fiscal_year alone takes a
	// band for a year: project.SelectFacts refuses one asked for as a year,
	// and readers that bypass it see no band only because the one scope
	// printing bands is a declared residue no cut admits. Hashed into the id
	// where set (MakeIDThrough).
	FiscalYearThrough *int `json:"fiscal_year_through"`

	RowPath    string `json:"row_path"`
	RowLabel   string `json:"row_label"`
	Category   string `json:"category"`
	Department string `json:"department"`

	ColumnPath string `json:"column_path"`
	FundGroup  string `json:"fund_group"`
	// Fund is the fund the column names, and nil -- published as null -- where
	// it names none: a schedule with no fund axis, or a row whose fund resolves
	// to nothing (p76's LAVWMA row). Nil is not 0; fact-funds-resolve refuses a
	// 0 that reaches the store.
	Fund *int `json:"fund"`

	// Sign says how this row relates to its category; it is NOT an instruction
	// to negate. AmountCents is the figure as printed, so a parenthesized contra
	// row arrives already negative (Budget Book p127's ERAF). A consumer sums
	// amount_cents directly and uses sign only for presentation.
	//
	// SignNetted marks a row printed against its KIND's direction (ACFR p41's
	// parenthesised Transfers (out), against Budget Book p66's positive TRANSFER
	// OUT); a consumer summing one kind across scopes must read it. Nothing
	// re-signs a value between the parser and this record.
	Sign  vocab.Sign   `json:"sign"`
	Units amount.Units `json:"units"`
	// AmountCents is integer cents whatever scale the table was printed in;
	// Units records that scale for provenance only.
	AmountCents int64 `json:"amount_cents"`

	// Derived marks a figure inferred rather than read. Every fact this
	// package produces is read from a page, so it is false; the key is stated
	// rather than left to be inferred from its absence.
	Derived bool `json:"derived"`
}

// ByID keys facts by id, which is unique by construction and proved so by the
// fact-ids-unique check.
func ByID(facts []Fact) map[string]Fact {
	out := make(map[string]Fact, len(facts))
	for _, f := range facts {
		out[f.ID] = f
	}
	return out
}

// FundNumber is the fund coordinate naming fund n.
func FundNumber(n int) *int { return &n }

// fundOfColumn is the one place mapping.Column's absent fund, spelled 0, becomes
// nil; the mapping side keeps 0 because ColumnPath hashes it (fisc-12jt).
func fundOfColumn(n int) *int {
	if n == 0 {
		return nil
	}
	return &n
}

// FundString renders a fund coordinate for a message: the number, or
// "(absent)" for none.
func FundString(fund *int) string {
	if fund == nil {
		return "(absent)"
	}
	return strconv.Itoa(*fund)
}

// ColumnLabel names a (fiscal year, basis) column the way every report does.
func ColumnLabel[B ~string](year int, basis B) string {
	return fmt.Sprintf("FY%d %s", year, basis)
}

// BandLabel names a (fiscal year, band end, basis) column: ColumnLabel for a
// one-year column, and the band spelled whole for a band, so no report prints
// a band as its first year.
func BandLabel[B ~string](year, through int, basis B) string {
	if through == 0 {
		return ColumnLabel(year, basis)
	}
	return fmt.Sprintf("FY%d-FY%d %s", year, through, basis)
}

// SameFund reports whether two fund coordinates agree: both absent, or both
// the same number (not the same address).
func SameFund(a, b *int) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// MakeID returns the content-addressed id for a fact's identity: a hash of
// (doc_id, rule_id, row_path, row_label, column_path, fiscal_year, basis).
// It leaves out the amount, so a corrected figure is a modified line, and any
// row or column index, so an inserted line renumbers nothing. The row label is
// needed because a path is a classification, not unique within a rule (Budget
// Book p127 prints thirteen taxes/property lines).
//
// THE LABEL IS mapping.Row.PrintedLabel(), the published row_label -- not
// Row.Label or Row.Identity() -- so an id is recomputable from its own line.
// PrintedLabel() is NOT injective (it joins label and label_tail on a space);
// the parser's printed-label uniqueness rule is what keeps ids apart, and
// CheckUniqueIDs is the backstop. Re-wording a row changes its id.
func MakeID(docID, ruleID, rowPath, rowLabel, columnPath string, fiscalYear int, basis vocab.Basis) string {
	return MakeIDThrough(docID, ruleID, rowPath, rowLabel, columnPath, fiscalYear, nil, basis)
}

// MakeIDThrough is [MakeID] for a figure that may be of several years: a
// band's last year joins the tuple, and a one-year figure's tuple is MakeID's
// unchanged, so no id outside a band moves. It is hashed so that
// fact-ids-recompute refuses a facts.jsonl line whose band was edited.
func MakeIDThrough(docID, ruleID, rowPath, rowLabel, columnPath string, fiscalYear int,
	through *int, basis vocab.Basis) string {
	// \x1f, because every component can contain "/" and some "-": a separator
	// occurring in the data would let two tuples hash identically.
	parts := []string{docID, ruleID, rowPath, rowLabel, columnPath, strconv.Itoa(fiscalYear), string(basis)}
	if through != nil {
		parts = append(parts, "through", strconv.Itoa(*through))
	}
	h := sha256.Sum256([]byte(strings.Join(parts, "\x1f")))
	return IDPrefix + hex.EncodeToString(h[:])[:idHexLen]
}

// BandOf is the last year of a fact's band, or zero for a one-year figure:
// the one place a reader turns FiscalYearThrough into a value it can compare.
func BandOf(f *Fact) int {
	if f.FiscalYearThrough == nil {
		return 0
	}
	return *f.FiscalYearThrough
}

// seriesIDPrefix marks a series id, distinct from [IDPrefix].
const seriesIDPrefix = "fisc-s-"

// makeSeriesID identifies one printed ROW across the columns it appears in:
// [MakeID]'s tuple MINUS the rule id, the fiscal year and the basis, since a
// schedule mapped one rule per year (Budget Book pp.186-209) prints a fund's
// line under four rules. TestASeriesIsOneCellPerYearAndBasis holds that no two
// committed facts share a series, a year and a basis. It cannot collide with a
// fact id: arity and prefix differ. rowLabel is mapping.Row.PrintedLabel(), as
// in MakeID.
func makeSeriesID(docID, rowPath, rowLabel, columnPath string) string {
	h := sha256.Sum256([]byte(strings.Join([]string{
		docID, rowPath, rowLabel, columnPath,
	}, "\x1f")))
	return seriesIDPrefix + hex.EncodeToString(h[:])[:idHexLen]
}

// SeriesID is the series this fact is one point of.
func (f Fact) SeriesID() string {
	return makeSeriesID(f.DocID, f.RowPath, f.RowLabel, f.ColumnPath)
}

// RowPath is the classification a row asserts, as a path: department then
// category where both are present (two axes, not two levels).
func RowPath(r mapping.Row) string {
	switch {
	case r.Department != "" && r.Category != "":
		return r.Department + "/" + r.Category
	case r.Department != "":
		return r.Department
	default:
		return r.Category
	}
}

// ColumnPath names the fund dimension of a column. The fiscal year is not
// part of it: it is its own field and id component.
func ColumnPath(c mapping.Column, scope string) string {
	var parts []string
	if c.FundGroup != "" {
		parts = append(parts, c.FundGroup)
	}
	if c.Fund != 0 {
		parts = append(parts, "fund/"+strconv.Itoa(c.Fund))
	}
	if len(parts) == 0 {
		// A single-fund schedule's column inherits the rule's scope.
		return scope
	}
	return strings.Join(parts, "/")
}

// FromValues builds the facts for one part's resolved figures: one fact per
// figure, and two where a row declares a counterpart. This is the pipeline's
// only fan-out, after CheckTotals has summed Values, so no printed total can
// see the second leg -- and none can catch a counterpart declared wrongly, so
// a schedule using them owes a reconciliation against something outside its
// own page. A row the page does not print is absent, never a zero fact.
func FromValues(f *mapping.File, rule *mapping.Rule, values []mapping.Value) ([]Fact, error) {
	out := make([]Fact, 0, len(values))
	for _, v := range values {
		basis := v.Column.EffectiveBasis(rule)
		sign := v.Row.Sign
		if sign == "" {
			sign = vocab.SignPositive
		}

		// row is the printed row classified as this figure is (a column may
		// carry the category and kind); every read below goes through it.
		row := v.Row
		row.Category, row.Kind = v.Category(), v.Kind(rule)
		col := row.EffectiveColumn(v.Column)
		rowPath := RowPath(row)
		columnPath := ColumnPath(col, rule.Scope)
		missing := ""
		switch {
		// THE CATEGORY, NOT THE ROW PATH: RowPath returns the bare department
		// when Category is empty, and both vocabulary checks skip Category "".
		// mapping.Parse refuses that row; this holds a Value built without it.
		case row.Category == "":
			missing = "category (a department is a second axis, not a substitute)"
		case rowPath == "":
			missing = "row path (the row has no category)"
		case columnPath == "":
			missing = "column path (the column has no fund and the rule no scope)"
		}
		if missing != "" {
			return nil, hint.With(
				fmt.Errorf("%s: rule %q p%d: row %q has no addressable %s",
					f.Path, rule.ID, v.Page, v.Row.PrintedLabel(), missing),
				"a fact's id is built from its row and column paths; without "+
					"both it cannot be addressed, cited, or diffed")
		}

		through := yearsThrough(col)
		out = append(out, Fact{
			// PrintedLabel(), not Label and not Identity(): see MakeID.
			ID: MakeIDThrough(f.DocID, rule.ID, rowPath, v.Row.PrintedLabel(),
				columnPath, col.FiscalYear, through, basis),
			DocID:             f.DocID,
			Page:              v.Page,
			Offset:            v.Offset,
			Token:             v.Token,
			RuleID:            rule.ID,
			Kind:              row.Kind,
			Basis:             basis,
			Scope:             rule.Scope,
			FiscalYear:        col.FiscalYear,
			FiscalYearThrough: through,
			RowPath:           rowPath,
			RowLabel:          v.Row.PrintedLabel(),
			Category:          row.Category,
			Department:        v.Row.Department,
			ColumnPath:        columnPath,
			FundGroup:         col.FundGroup,
			Fund:              fundOfColumn(col.Fund),
			Sign:              sign,
			Units:             rule.Units,
			AmountCents:       int64(v.Cents),
			Derived:           false,
		})

		if v.Row.Counterpart == nil {
			continue
		}
		cp := *v.Row.Counterpart
		cpCol := cp.Column(v.Column)
		cpRow := row
		cpRow.Category, cpRow.Kind = cp.Category, cp.Kind
		// THE FAR LEG IS GUARDED ON BOTH PATHS, as the near leg is: its row
		// path can be empty. And the scope fallback must not apply to its
		// column: a counterpart under the rule's scope names no payer and no
		// fund-group check reaches it.
		cpRowPath := RowPath(cpRow)
		cpMissing, cpHint := "", ""
		switch {
		case cpRowPath == "":
			cpMissing = "row path (the counterpart has no category and the row no department)"
			cpHint = "a fact's id is built from its row and column paths; without " +
				"both it cannot be addressed, cited, or diffed"
		case cpCol.FundGroup == "" && cpCol.Fund == 0:
			cpMissing = "column path (the counterpart declares neither fund nor fund group)"
			cpHint = "a counterpart declares the fund at its own end of the movement; " +
				"without one its fact falls back to the rule's scope, where it " +
				"names no payer and no check can reach it"
		}
		if cpMissing != "" {
			return nil, hint.With(
				fmt.Errorf("%s: rule %q p%d: row %q counterpart has no addressable %s",
					f.Path, rule.ID, v.Page, v.Row.PrintedLabel(), cpMissing),
				cpHint)
		}
		cpPath := ColumnPath(cpCol, rule.Scope)
		out = append(out, Fact{
			ID: MakeIDThrough(f.DocID, rule.ID, cpRowPath, cpRow.PrintedLabel(),
				cpPath, col.FiscalYear, through, basis),
			DocID: f.DocID,
			// THE SAME PROVENANCE: both legs cite one printed figure, and
			// fact-offset-points-at-token holds for both.
			Page:              v.Page,
			Offset:            v.Offset,
			Token:             v.Token,
			RuleID:            rule.ID,
			Kind:              cp.Kind,
			Basis:             basis,
			Scope:             rule.Scope,
			FiscalYear:        col.FiscalYear,
			FiscalYearThrough: through,
			RowPath:           cpRowPath,
			RowLabel:          cpRow.PrintedLabel(),
			Category:          cp.Category,
			Department:        cpRow.Department,
			ColumnPath:        cpPath,
			FundGroup:         cpCol.FundGroup,
			Fund:              fundOfColumn(cpCol.Fund),
			Sign:              sign,
			Units:             rule.Units,
			AmountCents:       int64(v.Cents),
			Derived:           false,
		})
	}
	return out, nil
}

// yearsThrough is a column's band end as the record publishes it: nil for a
// one-year figure, which is every figure but a band's.
func yearsThrough(c mapping.Column) *int {
	if c.FiscalYearThrough == 0 {
		return nil
	}
	n := c.FiscalYearThrough
	return &n
}

// sortKey is the specified total order: (doc_id, page, row_path, column_path,
// rule_id), then fiscal year, band, basis and id, so the order is total and
// two correct builds cannot differ in sequence.
func less(a, b Fact) bool {
	switch {
	case a.DocID != b.DocID:
		return a.DocID < b.DocID
	case a.Page != b.Page:
		return a.Page < b.Page
	case a.RowPath != b.RowPath:
		return a.RowPath < b.RowPath
	case a.ColumnPath != b.ColumnPath:
		return a.ColumnPath < b.ColumnPath
	case a.RuleID != b.RuleID:
		return a.RuleID < b.RuleID
	case a.FiscalYear != b.FiscalYear:
		return a.FiscalYear < b.FiscalYear
	case BandOf(&a) != BandOf(&b):
		return BandOf(&a) < BandOf(&b)
	case a.Basis != b.Basis:
		return a.Basis < b.Basis
	default:
		return a.ID < b.ID
	}
}

// Sort puts facts into the canonical order, in place.
func Sort(facts []Fact) {
	sort.SliceStable(facts, func(i, j int) bool { return less(facts[i], facts[j]) })
}

// CheckSorted reports the first pair of facts that are out of canonical order,
// so a nondeterministic build fails rather than producing a whole-file diff.
func CheckSorted(facts []Fact) error {
	for i := 1; i < len(facts); i++ {
		if less(facts[i], facts[i-1]) {
			return hint.With(
				fmt.Errorf("facts are out of order at line %d: %s (%s p%d %s) sorts before "+
					"line %d: %s (%s p%d %s)",
					i+1, facts[i].ID, facts[i].RowPath, facts[i].Page, facts[i].ColumnPath,
					i, facts[i-1].ID, facts[i-1].RowPath, facts[i-1].Page, facts[i-1].ColumnPath),
				"facts.jsonl is written in a canonical order so a rebuild diffs "+
					"cleanly; run fisc build to regenerate it")
		}
	}
	return nil
}

// CheckUniqueIDs reports two facts claiming the same identity: two rules
// asserting a figure for one cell, which is a bug and never worked around.
func CheckUniqueIDs(facts []Fact) error {
	seen := make(map[string]Fact, len(facts))
	for _, f := range facts {
		prev, dup := seen[f.ID]
		if !dup {
			seen[f.ID] = f
			continue
		}
		return hint.With(
			fmt.Errorf("id %s is claimed twice: rule %q p%d says %s, rule %q p%d says %s "+
				"(both are %s %s %s %s)",
				f.ID, prev.RuleID, prev.Page, amount.Cents(prev.AmountCents),
				f.RuleID, f.Page, amount.Cents(f.AmountCents),
				f.DocID, f.RowPath, f.ColumnPath, BandLabel(f.FiscalYear, BandOf(&f), f.Basis)),
			"two rules assert a figure for the same cell; one is reading the "+
				"wrong column or double-counting a subtotal, so check both "+
				"before changing either")
	}
	return nil
}

// Write emits facts as JSON Lines in the order given, without sorting them,
// so a caller that forgot to sort produces a file CheckSorted rejects.
// HTML escaping is off ("Fines & Forfeitures" stays readable), each record
// ends in exactly one "\n", and no field is omitempty, so every line has the
// same keys in the same order.
func Write(w io.Writer, facts []Fact) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	for _, f := range facts {
		if err := enc.Encode(f); err != nil {
			return fmt.Errorf("write fact %s: %w", f.ID, err)
		}
	}
	return nil
}

// Read parses a facts.jsonl stream.
func Read(r io.Reader) ([]Fact, error) {
	dec := json.NewDecoder(r)
	// An unknown key means a stale file, which must be loud rather than pass
	// a rebuild-and-diff on the fields that survived.
	dec.DisallowUnknownFields()

	var out []Fact
	for {
		var f Fact
		err := dec.Decode(&f)
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return nil, fmt.Errorf("read fact %d: %w", len(out)+1, err)
		}
		out = append(out, f)
	}
}
