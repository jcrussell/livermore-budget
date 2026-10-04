// Package fact turns resolved figures into the published fact record.
//
// A fact is one number the city printed, plus everything needed to find that
// number again in the source document. The record is deliberately wide and
// deliberately repetitive — every field appears on every line, nothing is
// omitted when empty — because facts.jsonl is read by diffing it. A key that
// vanishes when a value is empty turns "this fact's department became blank"
// into a structural change, and a reviewer comparing two builds cannot tell
// the two apart.
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
	"github.com/jcrussell/livermore-budget/internal/mapping"
	"github.com/jcrussell/livermore-budget/internal/vocab"
	"github.com/jcrussell/livermore-budget/pkg/cmdutil"
)

// IDPrefix marks a fact id. It exists so an id is recognizable on sight in a
// log line or a URL, and so a future identifier of another kind cannot be
// mistaken for one.
const IDPrefix = "fisc-f-"

// idHexLen is how much of the sha256 an id carries. Twelve hex digits is 48
// bits: with the low tens of thousands of facts this corpus can produce, the
// chance of an accidental collision is on the order of 1 in 10^6, and a
// collision is a hard error rather than silent corruption, so the tradeoff is
// legibility against a failure that announces itself.
const idHexLen = 12

// Fact is one published figure.
//
// Field order is the JSON key order — encoding/json emits struct fields in
// declaration order — and the order is part of the format, not an accident of
// how this struct grew.
type Fact struct {
	// ID is content-addressed over identity, never over the amount. See MakeID.
	ID    string `json:"id"`
	DocID string `json:"doc_id"`
	// Page and Offset locate the figure in the extracted page text, so a
	// citation can point at the number rather than at the document.
	Page   int `json:"page"`
	Offset int `json:"offset"`
	// Token is the text that was parsed, kept verbatim. An auditor comparing
	// amount_cents against the PDF needs to see what the parser actually read,
	// not a re-rendering of what it concluded.
	Token  string `json:"token"`
	RuleID string `json:"rule_id"`

	Kind  vocab.Kind  `json:"kind"`
	Basis vocab.Basis `json:"basis"`
	Scope string      `json:"scope"`

	FiscalYear int `json:"fiscal_year"`

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
	// to negate. AmountCents is always the figure as the document printed it,
	// so a contra row the city parenthesizes arrives here already negative
	// (Budget Book p127 prints ERAF as "(14,086,438)"). A consumer sums
	// amount_cents directly and uses sign only to decide how to present the
	// row — a flow diagram cannot draw a negative link, so contra rows net
	// into their parent instead of getting one.
	//
	// A THIRD VALUE SAYS SOMETHING ELSE AGAIN. SignNetted marks a row printed
	// against its KIND's direction rather than a deduction inside its category:
	// ACFR p41 parenthesises Transfers (out) because its block sums to a net
	// figure, while Budget Book p66 prints TRANSFER OUT as a positive magnitude.
	// A consumer summing one kind across scopes must read this or it will cancel
	// where it meant to accumulate.
	//
	// Nothing re-signs a value between the parser and this record. That last
	// value is what the paragraph this one replaced anticipated when it said a
	// new convention "needs its own field rather than an overload of this one".
	Sign  vocab.Sign   `json:"sign"`
	Units amount.Units `json:"units"`
	// AmountCents is integer cents, always, whatever scale the source table was
	// printed in. Units records that scale for provenance; it is not a
	// conversion the reader has to apply.
	AmountCents int64 `json:"amount_cents"`

	// Derived marks a figure this project inferred rather than read. Every
	// fact this package produces is read from a page, so it is false on every
	// line today. The key is present anyway because "published and derived are
	// different things" is the invariant the site is built on, and a consumer
	// must be able to rely on the distinction being stated rather than
	// inferred from a key's absence.
	Derived bool `json:"derived"`
}

// FundNumber is the fund coordinate naming fund n.
func FundNumber(n int) *int { return &n }

// fundOfColumn is the one place mapping.Column's absent fund, spelled 0, becomes
// nil. The mapping side keeps 0 because ColumnPath hashes it into every fact id
// (fisc-12jt).
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

// SameFund reports whether two fund coordinates agree: both absent, or both
// the same number (not the same address).
func SameFund(a, b *int) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// MakeID returns the content-addressed id for a fact's identity.
//
// The hashed tuple is (doc_id, rule_id, row_path, row_label, column_path,
// fiscal_year, basis). What it leaves out matters more than what it includes:
//
//   - The amount. A corrected figure must be a modified line in facts.jsonl,
//     not a delete plus an add, or every republication of a revised number
//     reads as though a fact vanished and an unrelated one appeared.
//   - Any row or column index. Positions shift when the city inserts a line;
//     hashing one would renumber every fact below the insertion and turn a
//     one-row change into a whole-file diff.
//
// The row label is in the tuple and the row path is not enough on its own,
// because a path is a classification and a classification is not unique within
// a rule. Budget Book p127 prints thirteen detail lines that are all
// taxes/property — data/taxonomy.yaml is explicit that those lines are a rule's
// label, not a category — so hashing the path alone gives all thirteen the same
// id, and the p167 department-by-category schedule has the same shape.
//
// THE LABEL PASSED HERE IS mapping.Row.PrintedLabel(), the same string the
// record publishes as row_label — not Row.Label, and not Row.Identity(). A row
// may be named by two printed anchors (Row.LabelTail), and the three differ for
// one: "Transfer From Wastewater", "Transfer From Wastewater to Stormwater",
// and the same pair joined by "\x1f". Hashing Row.Label collided the two rows
// Budget Book p76 prints as "Transfer From Wastewater to Stormwater" and
// "Transfer From Wastewater to LAVWMA / Wastewater" (fisc-28h), so the real
// choice was between the published label and the resolver's identity. Two
// reasons settle it for the published one:
//
//   - An id must be recomputable from the record it names. facts.jsonl is the
//     audit trail; a reader holding a line must be able to re-derive its id
//     from the fields printed on that line. Identity() is not among them, and
//     it cannot be recovered from row_label either — "A B" could be one anchor
//     or two.
//   - A re-spelling is not a re-wording. A row shipped as {label: "A B"} that
//     later needs two-anchor matching and is re-spelt {label: "A", label_tail:
//     "B"} keeps its id here, and would change it under Identity() — though the
//     page, the printed row and every published field are unchanged. The
//     doctrine below is that re-WORDING a row changes its id, because the
//     resolver anchors on the label; splitting one anchor into two is not that.
//
// PrintedLabel() is NOT injective, and it is worth being plain about it: it
// joins on a space, so {label: "Transfer From Water to Water"} and {label:
// "Transfer From Water", label_tail: "to Water"} print the same string and hash
// to the same id. What separates them is the parser's printed-label uniqueness
// rule, not this encoding — so id uniqueness here rests on a validation, where
// under Identity() it would have rested on the encoding alone. That is the real
// cost of the choice, accepted because the first reason above is decisive and
// because the collision needs two rows whose anchor text is the same word
// sequence, which the parser refuses outright.
//
// The two notions of uniqueness this leaves apart are both enforced, in the
// place each belongs. The parser enforces that no two rows of a rule share an
// Identity(), which is what a positional read of a page needs, AND that no two
// share a PrintedLabel(), which is what this id needs — two rows publishing one
// row_label would be indistinguishable to every reader of facts.jsonl whatever
// their ids were, so that is a defect to refuse and not to route around.
// CheckUniqueIDs is the backstop if a rule ever reaches here unparsed.
//
// The cost is that re-wording a row in the source changes its id. That is the
// right trade: the label is also what the resolver anchors on, so a re-wording
// already forces a rule change, and a rule change is the reviewable event where
// an id shift belongs.
func MakeID(docID, ruleID, rowPath, rowLabel, columnPath string, fiscalYear int, basis vocab.Basis) string {
	// The separator is a unit separator rather than a printable character
	// because every component here can contain "/" and some can contain "-".
	// Joining on a character that occurs in the data would let two different
	// tuples hash identically, which is the one thing an identity must not do.
	h := sha256.Sum256([]byte(strings.Join([]string{
		docID, ruleID, rowPath, rowLabel, columnPath, strconv.Itoa(fiscalYear), string(basis),
	}, "\x1f")))
	return IDPrefix + hex.EncodeToString(h[:])[:idHexLen]
}

// seriesIDPrefix marks a series id. It is deliberately not [IDPrefix]: a series
// and a fact are different things and an id that could be either is an id a
// reader has to look up to understand.
const seriesIDPrefix = "fisc-s-"

// makeSeriesID identifies one printed ROW across the columns it appears in.
//
// It is [MakeID]'s tuple MINUS the rule id, the fiscal year and the basis. Two
// facts are the same printed row at different times exactly when one document
// prints them on one line of one schedule, and the rule is not part of that: a
// schedule mapped one rule per year, as Budget Book pp.186-209 are, prints a
// fund's line once a year under four rules. So a series identity is derivable
// by anyone holding facts.jsonl, and it is stable when a fifth year is mapped
// -- which a positional or ordinal id would not be.
//
// What keeps two rows of one document apart is the row path, the printed label
// and the column path together; TestASeriesIsOneCellPerYearAndBasis holds that
// no two committed facts share a series, a year and a basis.
//
// IT CANNOT COLLIDE WITH A FACT ID. The arity differs (four components against
// seven), the prefix differs, and \x1f is the separator in both for MakeID's
// reason: every component can contain "/" and some can contain "-", so joining
// on a character that occurs in the data would let two different tuples hash
// alike.
//
// rowLabel is mapping.Row.PrintedLabel(), the same string the fact record
// publishes, for every reason MakeID's doc comment gives about it.
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

// RowPath is the classification a row asserts, as a path.
//
// A department and a category are two axes, not two levels, so where both are
// present the department leads and the category qualifies it: a fact is
// "Police, wages and benefits". Where only one is present it stands alone.
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

// ColumnPath names the fund dimension of a column.
//
// The fiscal year is deliberately not part of it. A column in these schedules
// is a (fund, year) pair, but the year is carried as its own field and its own
// id component, and folding it in here would make "the same fund, next year"
// look like a different place rather than the same place at a different time.
func ColumnPath(c mapping.Column, scope string) string {
	var parts []string
	if c.FundGroup != "" {
		parts = append(parts, c.FundGroup)
	}
	if c.Fund != 0 {
		parts = append(parts, "fund/"+strconv.Itoa(c.Fund))
	}
	if len(parts) == 0 {
		// A column with no fund dimension of its own inherits the rule's
		// scope, which is what a single-fund schedule relies on.
		return scope
	}
	return strings.Join(parts, "/")
}

// FromValues builds the facts for one part's resolved figures.
//
// USUALLY ONE FACT PER FIGURE, AND TWO WHERE A ROW DECLARES A COUNTERPART.
// This is the only fan-out in the pipeline: mapping.Resolver yields one Value
// per printed token and everything upstream of here counts figures, not facts.
// That placement is load-bearing rather than convenient. CheckTotals sums
// Values against the total the page prints, and pkg/cmd/build calls Values
// before it calls this, so no printed total, stated_total_delta or rollup can
// see the second leg -- a counterpart cannot make a schedule stop tying to its
// own document. The cost is the other side of the same coin: those checks
// cannot catch a counterpart declared wrongly either, which is why a schedule
// using them owes a reconciliation against something outside its own page.
//
// Declared omissions are not among them. A row the page does not print is
// absent, not zero, and inventing a zero fact for it would put a provenance
// pointer on a line the document has no line for. The column total tying is
// what establishes the column is complete; a consumer reading a missing
// (row, column) pair as "no flow" is reading it correctly.
func FromValues(f *mapping.File, rule *mapping.Rule, values []mapping.Value) ([]Fact, error) {
	out := make([]Fact, 0, len(values))
	for _, v := range values {
		basis := v.Column.EffectiveBasis(rule)
		sign := v.Row.Sign
		if sign == "" {
			sign = vocab.SignPositive
		}

		// row is the printed row classified as this figure is: on a rule whose
		// columns carry the category, the column's category and kind. Every
		// read below of a fact's category or kind goes through it.
		row := v.Row
		row.Category, row.Kind = v.Category(), v.Kind(rule)
		col := row.EffectiveColumn(v.Column)
		rowPath := RowPath(row)
		columnPath := ColumnPath(col, rule.Scope)
		missing := ""
		switch {
		// THE CATEGORY, NOT THE ROW PATH, because the row path does not
		// witness it. RowPath returns the bare department when Category is
		// empty, so a Value carrying `department:` and no category yields a
		// non-empty path and would publish a fact with Category: "" -- which
		// both vocabulary checks SKIP, and which in an unprojected scope is
		// named by nothing at all. mapping.Parse refuses that row; this is the
		// same rule at the other constructor, so a Value built without the
		// parser cannot reopen it.
		case row.Category == "":
			missing = "category (a department is a second axis, not a substitute)"
		case rowPath == "":
			missing = "row path (the row has no category)"
		case columnPath == "":
			missing = "column path (the column has no fund and the rule no scope)"
		}
		if missing != "" {
			return nil, cmdutil.WithHint(
				fmt.Errorf("%s: rule %q p%d: row %q has no addressable %s",
					f.Path, rule.ID, v.Page, v.Row.PrintedLabel(), missing),
				"a fact's id is built from its row and column paths; without "+
					"both it cannot be addressed, cited, or diffed")
		}

		out = append(out, Fact{
			// PrintedLabel(), not Label and not Identity(): see MakeID.
			ID: MakeID(f.DocID, rule.ID, rowPath, v.Row.PrintedLabel(),
				columnPath, v.Column.FiscalYear, basis),
			DocID:       f.DocID,
			Page:        v.Page,
			Offset:      v.Offset,
			Token:       v.Token,
			RuleID:      rule.ID,
			Kind:        row.Kind,
			Basis:       basis,
			Scope:       rule.Scope,
			FiscalYear:  v.Column.FiscalYear,
			RowPath:     rowPath,
			RowLabel:    v.Row.PrintedLabel(),
			Category:    row.Category,
			Department:  v.Row.Department,
			ColumnPath:  columnPath,
			FundGroup:   col.FundGroup,
			Fund:        fundOfColumn(col.Fund),
			Sign:        sign,
			Units:       rule.Units,
			AmountCents: int64(v.Cents),
			Derived:     false,
		})

		if v.Row.Counterpart == nil {
			continue
		}
		cp := *v.Row.Counterpart
		cpCol := cp.Column(v.Column)
		cpRow := row
		cpRow.Category, cpRow.Kind = cp.Category, cp.Kind
		// THE FAR LEG IS GUARDED ON BOTH PATHS, exactly as the near leg is
		// twenty lines above. cpRow takes its category from the counterpart and
		// its department from the row it fans out of, so both can be empty at
		// once -- and the far leg would then publish row_path "" and an id
		// hashed over it, an unaddressable fact rather than an error.
		//
		// AND THE SCOPE FALLBACK MUST NOT APPLY TO THE COLUMN. ColumnPath
		// returns the rule's scope for a column with no fund dimension, which
		// is what a single-fund schedule relies on. A counterpart taking it
		// would publish a real-looking fact under "transfers-by-fund" rather
		// than under a fund, where no fund-group check ever compares it against
		// the spine -- and the payer, which is the only thing the far leg
		// exists to name, would be absent from the fact that names it.
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
			return nil, cmdutil.WithHint(
				fmt.Errorf("%s: rule %q p%d: row %q counterpart has no addressable %s",
					f.Path, rule.ID, v.Page, v.Row.PrintedLabel(), cpMissing),
				cpHint)
		}
		cpPath := ColumnPath(cpCol, rule.Scope)
		out = append(out, Fact{
			ID: MakeID(f.DocID, rule.ID, cpRowPath, cpRow.PrintedLabel(),
				cpPath, v.Column.FiscalYear, basis),
			DocID: f.DocID,
			// THE SAME PROVENANCE, DELIBERATELY. Both legs cite one printed
			// figure, because that figure IS the evidence for both directions
			// of one movement. fact-offset-points-at-token holds for both.
			Page:        v.Page,
			Offset:      v.Offset,
			Token:       v.Token,
			RuleID:      rule.ID,
			Kind:        cp.Kind,
			Basis:       basis,
			Scope:       rule.Scope,
			FiscalYear:  v.Column.FiscalYear,
			RowPath:     cpRowPath,
			RowLabel:    cpRow.PrintedLabel(),
			Category:    cp.Category,
			Department:  cpRow.Department,
			ColumnPath:  cpPath,
			FundGroup:   cpCol.FundGroup,
			Fund:        fundOfColumn(cpCol.Fund),
			Sign:        sign,
			Units:       rule.Units,
			AmountCents: int64(v.Cents),
			Derived:     false,
		})
	}
	return out, nil
}

// sortKey is the specified total order: (doc_id, page, row_path, column_path,
// rule_id), then fiscal year, basis and id.
//
// The bead specifies the first five. The rest are needed for the order to be
// total: two columns of one schedule differ only by fiscal year, so the first
// five tie for them, and an order with ties is not a canonical order — two
// correct builds could emit the same facts in different sequences, which is
// exactly the nondeterminism the sortedness check exists to catch. The id
// breaks any remaining tie absolutely, because ids are unique by construction
// and CheckUniqueIDs has already proved it.
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

// CheckSorted reports the first pair of facts that are out of canonical order.
//
// This is what makes nondeterminism visible in CI for free: a build that emits
// the same facts in a different sequence fails here rather than producing a
// whole-file diff that a reviewer has to read to discover says nothing.
func CheckSorted(facts []Fact) error {
	for i := 1; i < len(facts); i++ {
		if less(facts[i], facts[i-1]) {
			return cmdutil.WithHint(
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

// CheckUniqueIDs reports two facts claiming the same identity.
//
// A collision is a real bug and never a hash accident to be worked around: it
// means two rules assert a figure for the same cell of the same document, so
// one of them is reading the wrong page, the wrong column, or double-counting
// a subtotal. Whichever it is, publishing either number would be a guess.
func CheckUniqueIDs(facts []Fact) error {
	seen := make(map[string]Fact, len(facts))
	for _, f := range facts {
		prev, dup := seen[f.ID]
		if !dup {
			seen[f.ID] = f
			continue
		}
		return cmdutil.WithHint(
			fmt.Errorf("id %s is claimed twice: rule %q p%d says %s, rule %q p%d says %s "+
				"(both are %s %s %s %s)",
				f.ID, prev.RuleID, prev.Page, amount.Cents(prev.AmountCents),
				f.RuleID, f.Page, amount.Cents(f.AmountCents),
				f.DocID, f.RowPath, f.ColumnPath, ColumnLabel(f.FiscalYear, f.Basis)),
			"two rules assert a figure for the same cell; one is reading the "+
				"wrong column or double-counting a subtotal, so check both "+
				"before changing either")
	}
	return nil
}

// Write emits facts as JSON Lines in the order given, without sorting them:
// the caller decides the order, and Write refuses to quietly impose one so a
// caller that forgot to sort produces a file CheckSorted will reject rather
// than a file that silently differs from the one it meant to write.
//
// Encoding is deliberate. HTML escaping is off, so "Fines & Forfeitures" stays
// readable instead of becoming "&"; the encoder writes exactly one "\n"
// after each record, giving LF endings and a trailing newline; and no field is
// omitempty, so every line has the same keys in the same order.
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

// Read parses a facts.jsonl stream. It exists so verify can check a committed
// file against a fresh build without re-deriving it.
func Read(r io.Reader) ([]Fact, error) {
	dec := json.NewDecoder(r)
	// A key the model no longer has means the file was written by a different
	// version of fisc, which must be loud: silently dropping it would let a
	// stale file pass a rebuild-and-diff check on the fields that survived.
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
