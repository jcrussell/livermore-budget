package mapping

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/jcrussell/livermore-budget/internal/amount"
	"github.com/jcrussell/livermore-budget/internal/geom"
	"github.com/jcrussell/livermore-budget/internal/hint"
	"github.com/jcrussell/livermore-budget/internal/quantity"
	"github.com/jcrussell/livermore-budget/internal/vocab"
)

// doc is the view of an extracted document a resolver needs, declared in the
// consumer (byob-interfaces.1, byob-interfaces.2), so this package does not
// import internal/corpus. Geometry is a required method rather than a type
// assertion, so the column guard can never be silently absent.
type doc interface {
	DocID() string
	Page(n int) (string, error)
	Geometry(n int) (*geom.Page, error)
}

// Resolution failures, distinguishable with errors.Is: a locator that found
// nothing may need re-anchoring, one that found several needs an ordinal.
var (
	// ErrNotFound reports a locator that resolved to nothing.
	ErrNotFound = errors.New("locator did not resolve")
	// ErrAmbiguous reports a locator that resolved to more than one candidate.
	ErrAmbiguous = errors.New("locator resolved to more than one candidate")
	// ErrNoStatedTotals reports a part the document prints no total for. It is
	// not a defect, but it is not a pass either.
	ErrNoStatedTotals = errors.New("part has no stated totals to check against")
)

// resolveError locates a resolution failure in both the rule and the document.
type resolveError struct {
	DocID  string
	RuleID string
	Page   int
	Field  string
	Msg    string
	Err    error
}

func (e *resolveError) Error() string {
	var b strings.Builder
	if e.DocID != "" {
		b.WriteString(e.DocID + " ")
	}
	fmt.Fprintf(&b, "p%d", e.Page)
	if e.RuleID != "" {
		fmt.Fprintf(&b, ": rule %q", e.RuleID)
	}
	if e.Field != "" {
		fmt.Fprintf(&b, ": %s", e.Field)
	}
	fmt.Fprintf(&b, ": %s", e.Msg)
	return b.String()
}

func (e *resolveError) Unwrap() error { return e.Err }

// Resolver reads the parts of one rule file against one extracted document.
// It is constructed per (document, rule file) pair so doc_id agreement is
// checked once, at the seam.
type Resolver struct {
	doc  doc
	file *File

	// mu guards pages and parts, which are the resolver's two memos.
	mu sync.Mutex
	// pages caches page text.
	pages map[int]string
	// parts caches each part's read.
	parts map[partKey]*resolvedPart
	// pairings caches each page's two substrates reconciled; see Resolver.pairing.
	pairings map[int]*pairing
}

// partKey identifies one part of the rule file. Rule id and page suffice
// because the parser rejects a rule listing a page twice; a rule built in
// memory that does so gets the first part's figures for both.
type partKey struct {
	rule string
	page int
}

// resolvedPart memoizes one part's read, error included, so a part that
// failed fails identically when asked again. page, by contrast, does not cache
// an error: a page read is I/O, a part read is a pure function of page text.
type resolvedPart struct {
	cells     []Value
	omissions []Omission
	err       error
}

// NewResolver pairs a rule file with the document it maps.
func NewResolver(d doc, f *File) (*Resolver, error) {
	if f.DocID != d.DocID() {
		return nil, hint.With(
			fmt.Errorf("%s declares doc_id %q but the extraction is %q",
				f.Path, f.DocID, d.DocID()),
			"a rule file maps exactly one document; check which extraction "+
				"directory was opened")
	}
	return &Resolver{doc: d, file: f,
		pages: map[int]string{}, parts: map[partKey]*resolvedPart{},
		pairings: map[int]*pairing{}}, nil
}

// page returns the text of page n, reading it at most once.
func (r *Resolver) page(n int) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if text, ok := r.pages[n]; ok {
		return text, nil
	}
	text, err := r.doc.Page(n)
	if err != nil {
		return "", err
	}
	r.pages[n] = text
	return text, nil
}

// block is a resolved span of page text: the rows one part covers, bounded by
// the part's section and stop_at anchors.
type block struct {
	Page int
	// End is where the stop_at anchor begins, also where a stated totals row
	// starts.
	Start, End int
	Text       string
}

// Value is one figure a part yielded, with enough provenance to cite it
// without resolving the page a second time.
type Value struct {
	Cents  amount.Cents
	Row    Row
	Column Column
	// RowIndex indexes the rule's rows, not the part's active rows, so two
	// parts printing different rows share one indexing base; ColumnIndex indexes
	// the part's columns.
	RowIndex, ColumnIndex int
	Page                  int
	// Offset is the byte offset of Token within the page text, so provenance
	// points at the figure.
	Offset int
	Token  string
}

// Category is the category this figure's facts carry: its column's on a rule
// whose columns carry the category, its row's otherwise.
func (v Value) Category() string {
	if v.Column.Category != "" {
		return v.Column.Category
	}
	return v.Row.Category
}

// Kind is the kind this figure's facts carry, following the same axis as
// Category: the column's or the row's override, the rule's where it states none.
func (v Value) Kind(rule *Rule) vocab.Kind {
	if v.Column.Category != "" {
		return v.Column.EffectiveKind(rule)
	}
	return v.Row.EffectiveKind(rule)
}

// Omission is one cell of a row a part prints that the page leaves blank,
// declared by Part.OmittedCells. Whether a blank becomes a fact (as a zero) is
// the fact model's decision; with two blanks in one column a tie proves
// nothing, and absent is not zero.
type Omission struct {
	Row      Row
	RowIndex int
	Page     int

	// ColumnIndex is the blank cell's column in the part's columns, and
	// Header the column_headers entry naming it.
	ColumnIndex int
	Header      string
}

// block resolves the span of page text a part covers.
func (r *Resolver) block(rule *Rule, p *Part) (*block, error) {
	text, err := r.page(p.Page)
	if err != nil {
		return nil, err
	}

	start := 0
	if p.Section != "" {
		at, err := r.anchor(rule, p, "section", text, 0, p.Section, p.SectionOrdinal)
		if err != nil {
			return nil, err
		}
		start = at + len(p.Section)
	}

	end := len(text)
	if p.StopAtOrdinal > 0 {
		at, err := r.anchor(rule, p, "stop_at", text, start, p.StopAt, p.StopAtOrdinal)
		if err != nil {
			return nil, err
		}
		end = at
	} else if p.StopAt != "" {
		i := strings.Index(text[start:], p.StopAt)
		if i < 0 {
			return nil, &resolveError{DocID: r.file.DocID, RuleID: rule.ID, Page: p.Page,
				Field: "stop_at", Err: ErrNotFound,
				Msg: fmt.Sprintf("%q does not occur after the section anchor", p.StopAt)}
		}
		end = start + i
	}
	return &block{Page: p.Page, Start: start, End: end, Text: text[start:end]}, nil
}

// anchor returns the byte offset of the ordinal-th occurrence of needle at or
// after from. An ordinal of zero means the anchor must be unique.
func (r *Resolver) anchor(rule *Rule, p *Part, field, text string, from int, needle string, ordinal int) (int, error) {
	var at []int
	for i := from; ; {
		j := strings.Index(text[i:], needle)
		if j < 0 {
			break
		}
		at = append(at, i+j)
		i += j + len(needle)
	}

	// A stop_at is counted from the block's start, and its message says so.
	where := "on the page"
	if from > 0 {
		where = "after the block's start"
	}
	fail := func(err error, msg, remedy string) (int, error) {
		return 0, hint.With(&resolveError{DocID: r.file.DocID, RuleID: rule.ID,
			Page: p.Page, Field: field, Msg: msg, Err: err}, remedy)
	}

	switch {
	case ordinal < 0:
		// The parser rejects this; a rule built in memory would panic.
		return fail(ErrNotFound, fmt.Sprintf("ordinal %d; ordinals count from 1", ordinal),
			"leave the ordinal unset to require a unique match")
	case len(at) == 0:
		return fail(ErrNotFound, fmt.Sprintf("%q does not occur %s", needle, where),
			"the page may have been re-extracted or the document revised; "+
				"check the page text under data/extracted/")
	case ordinal == 0 && len(at) > 1:
		return fail(ErrAmbiguous,
			fmt.Sprintf("%q occurs %d times %s: %s", needle, len(at), where,
				describeAt(text, at)),
			fmt.Sprintf("set section_ordinal to say which one starts the block "+
				"(1 to %d)", len(at)))
	case ordinal > len(at):
		return fail(ErrNotFound,
			fmt.Sprintf("occurrence %d of %q was requested but it occurs %d times %s: %s",
				ordinal, needle, len(at), where, describeAt(text, at)),
			"the page's shape changed; re-count the occurrences before "+
				"adjusting the ordinal, because a wrong one reads a real block "+
				"at the wrong place")
	case ordinal == 0:
		return at[0], nil
	}
	return at[ordinal-1], nil
}

// describeAt renders up to three candidate offsets as "#N line L: <context>".
func describeAt(text string, at []int) string {
	const max = 3
	var b strings.Builder
	for i, off := range at {
		if i == max {
			fmt.Fprintf(&b, "; and %d more", len(at)-max)
			break
		}
		if i > 0 {
			b.WriteString("; ")
		}
		fmt.Fprintf(&b, "#%d line %d: %q", i+1, 1+strings.Count(text[:off], "\n"),
			context(text, off))
	}
	return b.String()
}

// context returns a short window of text around off, for error messages.
func context(text string, off int) string {
	const window = 24
	start := max(0, off-window)
	end := min(len(text), off+window)
	s := strings.ReplaceAll(text[start:end], "\n", " ")
	if start > 0 {
		s = "…" + s
	}
	if end < len(text) {
		s += "…"
	}
	return s
}

// Values reads the figures a part yields, in row-major order, along with the
// cells the part declares the page leaves blank.
//
// A part with its own labels is read by label, and the text between rows must
// be empty; that is what catches an unmapped row on pages with no total. A
// label-less continuation is read positionally, guarded by the value count.
// Positional reading is confined to label-less parts: over Budget Book p127 it
// reads 14 rows where there are 13, because labels there contain "-" and
// digits.
//
// A part is read at most once per resolver. The returned slices are shallow
// copies: a Value's Row shares Counterpart and SubtotalDeltas with the rule.
// The values are the cells cellPublishes admits, applied here and nowhere
// after.
func (r *Resolver) Values(rule *Rule, p *Part) ([]Value, []Omission, error) {
	rp := r.resolvePart(rule, p)
	if rp.err != nil {
		return nil, nil, rp.err
	}
	var values []Value
	for _, v := range rp.cells {
		if p.cellPublishes(v.ColumnIndex, v.Row) {
			values = append(values, v)
		}
	}
	return values, slices.Clone(rp.omissions), nil
}

// Cells is every amount the part reads, in Values' order, including figures
// under a skipped row or column, which a printed subtotal still adds.
func (r *Resolver) Cells(rule *Rule, p *Part) ([]Value, error) {
	rp := r.resolvePart(rule, p)
	if rp.err != nil {
		return nil, rp.err
	}
	return slices.Clone(rp.cells), nil
}

// resolvePart returns the memoized read of one part, performing it on the
// first ask. The lock is not held across the read (page takes it, and
// sync.Mutex is not reentrant); racing callers both read, and the first result
// stored wins.
func (r *Resolver) resolvePart(rule *Rule, p *Part) *resolvedPart {
	key := partKey{rule: rule.ID, page: p.Page}
	if rp, ok := r.cachedPart(key); ok {
		return rp
	}
	rp := &resolvedPart{}
	rp.cells, rp.omissions, rp.err = r.readPart(rule, p)
	return r.storePart(key, rp)
}

func (r *Resolver) cachedPart(key partKey) (*resolvedPart, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rp, ok := r.parts[key]
	return rp, ok
}

// storePart records rp under key and returns whichever result is canonical.
func (r *Resolver) storePart(key partKey, rp *resolvedPart) *resolvedPart {
	r.mu.Lock()
	defer r.mu.Unlock()
	if first, ok := r.parts[key]; ok {
		return first
	}
	r.parts[key] = rp
	return rp
}

func (r *Resolver) readPart(rule *Rule, p *Part) ([]Value, []Omission, error) {
	blk, err := r.block(rule, p)
	if err != nil {
		return nil, nil, err
	}

	// A part with no column_headers gets a nil guard and is read by text alone.
	guard, err := r.guard(rule, p)
	if err != nil {
		return nil, nil, err
	}

	var values []Value
	if p.LabelsFrom == 0 {
		values, err = r.labelledValues(rule, p, blk, guard)
	} else {
		values, err = r.positionalValues(rule, p, blk, guard)
	}
	if err != nil {
		return nil, nil, err
	}
	return values, Omissions(rule, p), nil
}

// Omissions is the cells a part declares the page leaves blank, in row order,
// from the declaration alone; internal/check reads its blanks from here.
func Omissions(rule *Rule, p *Part) []Omission {
	if len(p.OmittedCells) == 0 {
		return nil
	}
	// Keyed on Identity(); the bare Label would mark every row sharing it.
	var out []Omission
	for i, row := range rule.Rows {
		for c := range p.ColumnHeaders {
			if p.blank[row.Identity()][c] {
				out = append(out, Omission{Row: row, RowIndex: i, Page: p.Page,
					ColumnIndex: c, Header: headerAt(p, c)})
			}
		}
	}
	return out
}

// headerAt names column c by its header, with its ordinal ("Principal #2")
// where the part prints that header over several columns.
func headerAt(p *Part, c int) string {
	text := p.ColumnHeaders[c].Text
	n, at := 0, 0
	for j, h := range p.ColumnHeaders {
		if !h.Unheaded && h.Text == text {
			n++
			if j == c {
				at = n
			}
		}
	}
	if n < 2 {
		return text
	}
	return fmt.Sprintf("%s #%d", text, at)
}

func (r *Resolver) labelledValues(rule *Rule, p *Part, blk *block, guard *columnGuard) ([]Value, error) {
	rows, rowIndex := rule.activeRows(p)
	fail := func(field, msg, remedy string) error {
		return hint.With(&resolveError{DocID: r.file.DocID, RuleID: rule.ID,
			Page: p.Page, Field: field, Msg: msg, Err: ErrNotFound}, remedy)
	}

	values := make([]Value, 0, rule.expectedValues(p))
	used := map[string]bool{}
	cursor := 0
	for i, row := range rows {
		j := indexWord(blk.Text, cursor, row.Label)
		if j < 0 {
			return nil, fail("rows",
				fmt.Sprintf("row %q does not occur after %s", row.Label, precedingRow(rows, i)),
				"rows are matched in the order the rule lists them, which must "+
					"be the order the page prints them")
		}
		gap := blk.Text[cursor : cursor+j]
		gapAt := blk.Start + cursor
		raised := func(off int) bool { return guard.raisedMarker(gapAt + off) }
		if err := r.checkGap(rule, p, gap, rows, i, used, raised); err != nil {
			return nil, err
		}

		after := cursor + j + len(row.Label)
		if row.LabelTail != "" {
			k, n := findFields(blk.Text[after:], row.LabelTail)
			if k < 0 {
				return nil, fail("rows", fmt.Sprintf(
					"row %q: second anchor %q does not occur after it",
					row.Label, row.LabelTail),
					"label_tail names the row's second printed field; both must be "+
						"on the row, in the order the page prints them")
			}
			if between := blk.Text[after : after+k]; strings.TrimSpace(between) != "" {
				return nil, fail("rows", fmt.Sprintf(
					"row %q: %q sits between it and %q", row.Label,
					strings.TrimSpace(between), row.LabelTail),
					"the two anchors identify one row, so only whitespace may "+
						"separate them; a word here means the anchors matched "+
						"different rows")
			}
			after += k + n
		}
		toks, err := dropCurrencyMarks(tokens(blk.Text[after:], blk.Start+after))
		if err != nil {
			return nil, fail("rows", fmt.Sprintf("row %q: %s", row.PrintedLabel(), err), currencyHint)
		}
		cols := printedColumns(p, row)
		if len(toks) < len(cols) {
			return nil, fail("rows", fmt.Sprintf(
				"row %q is followed by %d values, want %d (one per column it prints)",
				row.PrintedLabel(), len(toks), len(cols)), "check the part's columns against the page")
		}
		toks = toks[:len(cols)]
		// A row's figures are on its label's line. The column guard enforces this
		// with line numbers; this is the same rule for a part with no guard.
		if guard == nil && len(toks) > 0 {
			if last := toks[len(toks)-1]; strings.Contains(blk.Text[after:last.off-blk.Start], "\n") {
				return nil, fail("rows", fmt.Sprintf(
					"row %q prints fewer than %d values on its own line",
					row.PrintedLabel(), len(cols)),
					"a row's figures are on the line its label is; check the part's "+
						"columns against the page, or declare the row's skipped columns")
			}
		}
		vals, err := r.parseRow(rule, p, row, rowIndex[i], toks, cols, guard)
		if err != nil {
			return nil, err
		}
		values = append(values, vals...)
		cursor = after
		if len(toks) > 0 {
			last := toks[len(toks)-1]
			cursor = last.off - blk.Start + len(last.text)
		}
	}

	// Anything after the last row's figures is an unmapped row, unless it is
	// declared like a gap between rows.
	if rest := strings.TrimSpace(blk.Text[cursor:]); rest != "" && !declaredGap(p, blk.Text[cursor:], used, true, nil) {
		return nil, fail("rows", fmt.Sprintf(
			"%q follows the last mapped row but is not mapped", rest),
			"every row inside the block must be listed in rows, with skip: true "+
				"if it should not produce facts, in wrapped_labels if the page "+
				"wrapped a label onto its own line, in headings if it is a section "+
				"heading, or in unmapped_text if it is a figure belonging to no row")
	}
	// A declared fragment the page did not use is stale.
	for _, w := range p.WrappedLabels {
		if !used[w] {
			return nil, fail("wrapped_labels", fmt.Sprintf(
				"%q is declared but does not appear between this part's rows", w),
				"a wrapped label is a claim about what the page prints; remove "+
					"the declaration when the page stops wrapping there")
		}
	}
	// Its own arm, and its own message, for the same reason as the figure arm
	// below: a heading is not a wrapped label and the reader is told which.
	for _, h := range p.Headings {
		if !used[h] {
			return nil, fail("headings", fmt.Sprintf(
				"%q is declared but does not appear between this part's rows", h),
				"a heading is a claim about what the page prints; remove the "+
					"declaration when the page stops printing it there")
		}
	}
	// Its own arm and message, since a heading is not a wrapped label, and the
	// only guard over an unmapped_text entry (see Part.UnmappedText).
	for _, u := range p.UnmappedText {
		if !used[u.Text] {
			return nil, fail("unmapped_text", fmt.Sprintf(
				"%q is declared but does not appear inside this part's block", u.Text),
				"an unmapped figure is a claim about what the page prints; remove "+
					"the declaration when the page stops printing it there")
		}
	}
	return values, nil
}

// checkGap enforces what may sit between one row's last figure and the next
// row's label. Before the first row the block may carry column headers, which
// are words; between rows only declared text may intervene. raised is
// declaredGap's, for a gap between two rows.
func (r *Resolver) checkGap(rule *Rule, p *Part, gap string, rows []Row, i int,
	used map[string]bool, raised func(off int) bool) error {
	if i == 0 {
		// Before the first row, words are allowed because column headers live here;
		// a wrapped label there is still declarable. Under total_row_above the
		// section anchor's line carries the stated totals, so exactly that one line is
		// skipped, before the wrapped_labels test, and everything after it is still
		// refused. IndexByte rather than strings.Cut: with no newline the first row is
		// on the total's line, which is refused rather than passed vacuously.
		if rule.TotalRowAbove {
			nl := strings.IndexByte(gap, '\n')
			if nl < 0 {
				return hint.With(&resolveError{DocID: r.file.DocID, RuleID: rule.ID,
					Page: p.Page, Field: "section", Err: ErrNotFound,
					Msg: fmt.Sprintf("row %q is on the same printed line as the total %q",
						rows[0].Label, rule.TotalRow)},
					"total_row_above says the stated totals are on the section "+
						"anchor's own line and the rows begin on the next one; "+
						"a row on that same line means the anchor matched elsewhere")
			}
			gap = gap[nl+1:]
		}
		if declaredGap(p, gap, used, false, nil) {
			return nil
		}
		// unmapped_text is not honoured here: the digit refusal is the only guard
		// over this gap (fisc-0cff).
		if strings.ContainsFunc(gap, unicode.IsDigit) {
			return hint.With(&resolveError{DocID: r.file.DocID, RuleID: rule.ID,
				Page: p.Page, Field: "section", Err: ErrNotFound,
				Msg: fmt.Sprintf("figures appear before the first row %q: %q",
					rows[0].Label, strings.TrimSpace(gap))},
				"the block starts too early — move the section anchor past the "+
					"column headers, or the first row's figures will be read as "+
					"part of the header")
		}
		// A LINE ENDING IN A DASH IS A ROW OF PRINTED ZEROS, which the digit
		// refusal above never sees. Words are a header; a dash is a cell.
		for _, l := range strings.Split(gap, "\n") {
			fs := strings.Fields(l)
			if len(fs) == 0 {
				continue
			}
			if _, err := amount.Parse(fs[len(fs)-1], amount.Dollars); err == nil {
				return hint.With(&resolveError{DocID: r.file.DocID, RuleID: rule.ID,
					Page: p.Page, Field: "section", Err: ErrNotFound,
					Msg: fmt.Sprintf("a line of printed zeros appears before the first row %q: %q",
						rows[0].Label, strings.TrimSpace(l))},
					"a dash is a printed zero, so this line is a row the rule has "+
						"not mapped: map it, with skip: true if it should not "+
						"produce facts, or move the section anchor past it")
			}
		}
		return nil
	}
	trimmed := strings.TrimSpace(gap)
	if trimmed == "" {
		return nil
	}
	if declaredGap(p, gap, used, true, raised) {
		return nil
	}
	return hint.With(&resolveError{DocID: r.file.DocID, RuleID: rule.ID,
		Page: p.Page, Field: "rows", Err: ErrNotFound,
		Msg: fmt.Sprintf("%q sits between rows %q and %q but is not mapped",
			trimmed, rows[i-1].PrintedLabel(), rows[i].PrintedLabel())},
		"add it to rows, with skip: true if it should not produce facts, to "+
			"wrapped_labels if the page wrapped a label onto its own line, to "+
			"headings if it is a section heading, or to unmapped_text if it is a "+
			"figure belonging to no row; leaving it out would publish a breakdown "+
			"that does not add up")
}

// declaredGap reports whether a gap is text the part declares, and marks what
// it used: one declared wrapped label, or several declared lines, each matched
// whole and trimmed as a wrapped label, a heading or, when figures is set, an
// unmapped_text figure. An undeclared line refuses the whole gap.
//
// A declared figure must be the whole gap, or a footnote marker (raised says
// the page prints it as one) on the line above a heading or the gap's last
// line (p194). raised is nil before the first row and after the last. A
// wrapped label never shares a gap with a figure.
func declaredGap(p *Part, gap string, used map[string]bool, figures bool, raised func(off int) bool) bool {
	if trimmed := strings.TrimSpace(gap); slices.Contains(p.WrappedLabels, trimmed) {
		used[trimmed] = true
		return true
	}
	type frag struct {
		text string
		at   int
	}
	var frags []frag
	figure := false
	at := 0
	for _, l := range strings.Split(gap, "\n") {
		lineAt := at
		at += len(l) + 1
		t := strings.TrimSpace(l)
		if t == "" {
			continue
		}
		if !slices.Contains(p.WrappedLabels, t) && !slices.Contains(p.Headings, t) {
			if !figures || !declaresUnmapped(p, t) {
				return false
			}
			figure = true
		}
		frags = append(frags, frag{t, lineAt + strings.Index(l, t)})
	}
	if figure && len(frags) > 1 {
		for i, f := range frags {
			heading := slices.Contains(p.Headings, f.text)
			marker := !heading && raised != nil && declaresUnmapped(p, f.text) && raised(f.at) &&
				(i+1 == len(frags) || slices.Contains(p.Headings, frags[i+1].text))
			if !heading && !marker {
				return false
			}
		}
	}
	for _, f := range frags {
		used[f.text] = true
	}
	return true
}

// indexWord is the offset after from at which label occurs in s as whole
// words, or -1: an end of label that is a letter or digit may not abut one in
// s, so "Fund 2" is not found in "Fund 20". The offset is relative to from.
func indexWord(s string, from int, label string) int {
	if label == "" {
		return strings.Index(s[from:], label)
	}
	first, _ := utf8.DecodeRuneInString(label)
	last, _ := utf8.DecodeLastRuneInString(label)
	for at := from; ; {
		i := strings.Index(s[at:], label)
		if i < 0 {
			return -1
		}
		start, end := at+i, at+i+len(label)
		before, _ := utf8.DecodeLastRuneInString(s[:start])
		after, _ := utf8.DecodeRuneInString(s[end:])
		joinsBefore := wordRune(first) && start > 0 && wordRune(before)
		joinsAfter := wordRune(last) && end < len(s) && wordRune(after)
		if !joinsBefore && !joinsAfter {
			return start - from
		}
		at = start + 1
	}
}

func wordRune(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }

// findFields finds the first run of s that is tail's whitespace-separated
// fields in order, separated by spaces or tabs and never by a line break, and
// returns its offset and length, or -1.
func findFields(s, tail string) (int, int) {
	fields := strings.Fields(tail)
	for from := 0; ; {
		i := strings.Index(s[from:], fields[0])
		if i < 0 {
			return -1, 0
		}
		start := from + i
		end := start + len(fields[0])
		ok := true
		for _, f := range fields[1:] {
			j := end
			for j < len(s) && (s[j] == ' ' || s[j] == '\t') {
				j++
			}
			if j == end || !strings.HasPrefix(s[j:], f) {
				ok = false
				break
			}
			end = j + len(f)
		}
		if ok {
			return start, end - start
		}
		from = start + 1
	}
}

// declaresUnmapped reports whether the part declares this exact trimmed line as
// a figure belonging to no row.
func declaresUnmapped(p *Part, trimmed string) bool {
	for _, u := range p.UnmappedText {
		if u.Text == trimmed {
			return true
		}
	}
	return false
}

func precedingRow(rows []Row, i int) string {
	if i == 0 {
		return "the section anchor"
	}
	return fmt.Sprintf("row %q", rows[i-1].Label)
}

// parseRow reads one row's tokens, toks[k] filed under column cols[k].
func (r *Resolver) parseRow(rule *Rule, p *Part, row Row, rowIndex int, toks []token,
	cols []int, guard *columnGuard) ([]Value, error) {
	// The column check is here, the one place holding a row, its tokens and their
	// columns together.
	if guard != nil {
		if err := guard.checkRow(r, rule, p, row, toks, cols); err != nil {
			return nil, err
		}
	}
	out := make([]Value, 0, len(toks))
	for k, tk := range toks {
		c := cols[k]
		col := p.Columns[c]
		// The quantity picks the grammar before skip is consulted. A recognized
		// non-amount cell is read, keeping the row whole, and never becomes a Value.
		if q := row.EffectiveQuantity(col); q != QuantityAmount {
			if err := recognize(q, tk.text); err != nil {
				return nil, &resolveError{DocID: r.file.DocID, RuleID: rule.ID, Page: p.Page,
					Field: fmt.Sprintf("row %q column %d", row.PrintedLabel(), c+1),
					Msg:   err.Error(), Err: err}
			}
			continue
		}
		cents, err := amount.Parse(tk.text, rule.Units)
		if err != nil {
			return nil, &resolveError{DocID: r.file.DocID, RuleID: rule.ID, Page: p.Page,
				Field: fmt.Sprintf("row %q column %d", row.PrintedLabel(), c+1),
				Msg:   err.Error(), Err: err}
		}
		// A skipped row or column still consumes its position and is returned as a
		// cell; Values keeps it out of the part's values.
		out = append(out, Value{Cents: cents, Row: row, Column: col,
			RowIndex: rowIndex, ColumnIndex: c, Page: p.Page,
			Offset: tk.off, Token: tk.text})
	}
	return out, nil
}

// recognize dispatches a non-amount cell to its grammar. A Quantity with no
// arm here is a harness error.
func recognize(q Quantity, tok string) error {
	switch q {
	case QuantityAmountPerUnit:
		return quantity.AmountPerUnit(tok)
	case QuantityPercentage:
		return quantity.Percentage(tok)
	case QuantityNumber:
		return quantity.Number(tok)
	case QuantityAmount:
		// parseRow owns amounts, through amount.Parse at the rule's units;
		// answering for them here would be a second money grammar.
	}
	return fmt.Errorf("no grammar recognizes quantity %q", q)
}

func (r *Resolver) positionalValues(rule *Rule, p *Part, blk *block, guard *columnGuard) ([]Value, error) {
	rows, rowIndex := rule.activeRows(p)
	ncols := len(p.Columns)
	// The marks are dropped here as on a labelled row: p81's first row and its
	// subtotals print "$" detached from each figure, inside the block.
	toks, err := dropCurrencyMarks(tokens(blk.Text, blk.Start))
	if err != nil {
		return nil, hint.With(&resolveError{DocID: r.file.DocID, RuleID: rule.ID,
			Page: p.Page, Field: "parts", Err: ErrNotFound, Msg: err.Error()}, currencyHint)
	}
	want := rule.expectedValues(p)

	if len(toks) != want {
		return nil, hint.With(&resolveError{DocID: r.file.DocID, RuleID: rule.ID,
			Page: p.Page, Field: "parts", Err: ErrNotFound,
			Msg: fmt.Sprintf("read %d values, want %d (%d rows × %d columns, "+
				"less %d declared blank); rows placed on other pages: %s",
				len(toks), want, len(rows), ncols, len(p.OmittedCells), declaredOmissions(rule, p))},
			"a label-less page is read positionally, so a count that does not "+
				"match means every row after the gap would be mismapped; move a "+
				"row's page or add or remove an omitted_cells entry only after "+
				"checking the page")
	}

	values := make([]Value, 0, want)
	at := 0
	for i, row := range rows {
		cols := printedColumns(p, row)
		rowToks := toks[at : at+len(cols)]
		at += len(cols)
		vals, err := r.parseRow(rule, p, row, rowIndex[i], rowToks, cols, guard)
		if err != nil {
			return nil, err
		}
		values = append(values, vals...)
	}
	if guard != nil {
		if err := r.checkLineAccounting(rule, p, blk, rows); err != nil {
			return nil, err
		}
	}
	return values, nil
}

// declaredOmissions renders, for the count message, the rows another page
// prints and the cells this one declares blank.
func declaredOmissions(rule *Rule, p *Part) string {
	rows := "none"
	var elsewhere []string
	for _, row := range rule.Rows {
		if !row.OnPart(p) {
			elsewhere = append(elsewhere, fmt.Sprintf("%s (page %d)", row.PrintedLabel(), row.Page))
		}
	}
	if len(elsewhere) > 0 {
		rows = strings.Join(elsewhere, ", ")
	}
	if len(p.OmittedCells) == 0 {
		return rows
	}
	cells := make([]string, len(p.OmittedCells))
	for i, o := range p.OmittedCells {
		cells[i] = o.describe()
	}
	return fmt.Sprintf("%s; cells declared blank: %s", rows, strings.Join(cells, ", "))
}

// StatedTotals reads the totals the document itself prints for a part.
//
// The anchor is the rule's total_row on a labelled part and its stop_at on a
// label-less one (Budget Book p67's "$" both ends the block and starts the
// totals). The totals are the first maximal run of exactly one amount per
// column within the anchor's line: maximal, so "Total 2022 COP Construction
// Fund" is not read as a figure; exact, so a line printing the totals twice
// fails; and line-bounded, so the next line cannot extend the run.
func (r *Resolver) StatedTotals(rule *Rule, p *Part) ([]amount.Cents, error) {
	_, _, totals, err := r.statedTotalLine(rule, p)
	return totals, err
}

// statedTotalLine resolves this part's stated-total line and reads it: the
// byte range its figures occupy, and the figures. The amountRun test is what
// makes a line a stated total: spine-transfers-in on p67 anchors on a header
// line, and spine-transfers-out and spine-fund-balance on the running footer
// "BUDGET FY 2025-27 Page 63".
func (r *Resolver) statedTotalLine(rule *Rule, p *Part) (lo, hi int, totals []amount.Cents, err error) {
	blk, err := r.block(rule, p)
	if err != nil {
		return 0, 0, nil, err
	}
	text, err := r.page(p.Page)
	if err != nil {
		return 0, 0, nil, err
	}

	from, field, err := r.totalAnchor(rule, p, blk, text)
	if err != nil {
		return 0, 0, nil, err
	}

	to := len(text)
	if i := strings.IndexByte(text[from:], '\n'); i >= 0 {
		to = from + i
	}
	line := text[from:to]
	totals, ok := amountRun(line, len(p.Columns), rule.Units)
	if !ok {
		return 0, 0, nil, hint.With(&resolveError{DocID: r.file.DocID, RuleID: rule.ID,
			Page: p.Page, Field: field, Err: ErrNotFound,
			Msg: fmt.Sprintf("no run of %d consecutive amounts follows the anchor on %q",
				len(p.Columns), strings.TrimSpace(line))},
			"the totals row must print one figure per column; if the page prints "+
				"them twice on one line, anchor past the first copy")
	}
	return from, to, totals, nil
}

// TotalRowSpan is the byte range, within the part's page text, of the figures
// the document prints on this part's stated-total line: statedTotalLine
// asking for the position, so it agrees with StatedTotals on every error. It
// is what answers "is this fact republishing a total" (fisc-eaic). The span is
// [anchor, end of line), the whitespace before the figures included.
func (r *Resolver) TotalRowSpan(rule *Rule, p *Part) (lo, hi int, err error) {
	lo, hi, _, err = r.statedTotalLine(rule, p)
	return lo, hi, err
}

// RollupTotalSpan is the byte range, within the rollup's page text, of the
// figures on the rollup's own total line: TotalRowSpan one level up, since
// republishing a rollup's figure as a row doubles a department. It returns
// ErrNoStatedTotals for a rollup that covers no rule. Width and units come
// from the first covered rule's bearer part, as CheckRollup reads them.
func (r *Resolver) RollupTotalSpan(ro *Rollup) (lo, hi int, err error) {
	rules, err := r.coveredRules(ro)
	if err != nil {
		return 0, 0, err
	}
	if len(rules) == 0 {
		return 0, 0, &resolveError{DocID: r.file.DocID, Page: ro.Page, RuleID: ro.ID,
			Field: "covers", Err: ErrNoStatedTotals,
			Msg: "the rollup covers no rule, so its printed total cannot be located"}
	}
	_, bearer, err := r.ruleStatedTotals(rules[0])
	if err != nil {
		return 0, 0, err
	}
	// rollupStatedTotals refuses an anchor that is missing, ambiguous, or followed
	// by no run of amounts.
	_, at, err := r.rollupStatedTotals(ro, len(bearer.Columns), rules[0].Units)
	if err != nil {
		return 0, 0, err
	}
	text, err := r.page(ro.Page)
	if err != nil {
		return 0, 0, err
	}
	lo = at + len(ro.TotalRow)
	hi = len(text)
	if i := strings.IndexByte(text[lo:], '\n'); i >= 0 {
		hi = lo + i
	}
	return lo, hi, nil
}

// amountRun returns the first maximal run of exactly n parsable amounts in s.
//
// Where a maximal run is the wrong width but begins with a "$"-marked figure,
// its marked prefix is considered too: Budget Book p67 runs the eight TOTAL
// EXPENDITURES figures straight into the eight TRANSFER OUT figures. The
// maximal run is preferred, so a page marking only some totals still fails
// closed. A detached mark is dropped first (ACFR p167 prints
// "$ 47,139,536      $ 54,468,310"); a mark with no figure after it fails the
// line.
func amountRun(s string, n int, u amount.Units) ([]amount.Cents, bool) {
	toks, err := dropCurrencyMarks(tokens(s, 0))
	if err != nil {
		return nil, false
	}
	for i := 0; i < len(toks); {
		var run []amount.Cents
		j := i
		for ; j < len(toks); j++ {
			c, err := amount.Parse(toks[j].text, u)
			if err != nil {
				break
			}
			run = append(run, c)
		}
		if len(run) == n {
			return run, true
		}
		if len(run) > n && strings.HasPrefix(toks[i].text, "$") {
			marked := 0
			for ; marked < len(run); marked++ {
				if !strings.HasPrefix(toks[i+marked].text, "$") {
					break
				}
			}
			if marked == n {
				return run[:n], true
			}
		}
		if j == i {
			j++ // not an amount at all; step past it
		}
		i = j
	}
	return nil, false
}

// totalsResult is what checking one part's totals established.
type totalsResult struct {
	// Columns is how many non-skip columns were compared.
	Columns int

	// Declared is how many of those tied only because the rule declared a
	// discrepancy, reported apart from exact ties.
	Declared int

	// Tolerated is how many columns tied only inside the printed_decimals
	// tolerance, and Slack what each was out by, in comparison order. A caller
	// adding Columns to a coverage figure must carry these two beside it.
	Tolerated int
	Slack     []amount.Cents
}

// CheckTotals asserts that the figures a part yields sum, per column, to the
// totals the document prints for it. It returns ErrNoStatedTotals where the
// document prints none, which callers report as unchecked. It checks the
// memoized read Values returned, not a second one.
func (r *Resolver) CheckTotals(rule *Rule, p *Part) (*totalsResult, error) {
	stated, err := r.StatedTotals(rule, p)
	if err != nil {
		return nil, err
	}
	values, _, err := r.Values(rule, p)
	if err != nil {
		return nil, err
	}

	sums := make([]amount.Cents, len(p.Columns))
	terms := make([]int, len(p.Columns))
	var w decimalsWitness
	for _, v := range values {
		if !rule.totalCovers(v.Kind(rule)) {
			continue
		}
		sums[v.ColumnIndex] += v.Cents
		terms[v.ColumnIndex]++
		if err := w.observe(rule, v); err != nil {
			return nil, r.decimalsError(rule, p, err)
		}
	}
	if err := w.settle(rule); err != nil {
		return nil, r.decimalsError(rule, p, err)
	}
	return r.compareTotals(rule, p, p, stated, sums, terms)
}

// CheckSpanningTotals asserts that the figures every part of a rule yields
// sum, per column, to the one total the document prints for the block: see
// Rule.TotalSpansParts. The parser established identical columns; this
// establishes that exactly one part prints the total row and that any
// declared discrepancy sits on that part.
func (r *Resolver) CheckSpanningTotals(rule *Rule) (*totalsResult, error) {
	bearer, err := r.TotalBearingPart(rule)
	if err != nil {
		return nil, err
	}
	stated, err := r.StatedTotals(rule, bearer)
	if err != nil {
		return nil, err
	}

	// A delta belongs on the part that prints the total, which the parser cannot
	// see.
	for i := range rule.Parts {
		p := &rule.Parts[i]
		if len(p.StatedTotalDeltas) == 0 || p.Page == bearer.Page {
			continue
		}
		return nil, hint.With(
			&resolveError{DocID: r.file.DocID, RuleID: rule.ID, Page: p.Page,
				Field: "stated_total_deltas", Err: ErrNotFound,
				Msg: fmt.Sprintf("declared here, but page %d is the page that prints %q",
					bearer.Page, rule.TotalRow)},
			"a declared delta describes the difference between a printed total "+
				"and the rows beneath it, so it is declared where that total is printed")
	}

	sums := make([]amount.Cents, len(bearer.Columns))
	terms := make([]int, len(bearer.Columns))
	var w decimalsWitness
	for i := range rule.Parts {
		p := &rule.Parts[i]
		values, _, err := r.Values(rule, p)
		if err != nil {
			return nil, err
		}
		for _, v := range values {
			if !rule.totalCovers(v.Kind(rule)) {
				continue
			}
			sums[v.ColumnIndex] += v.Cents
			terms[v.ColumnIndex]++
			if err := w.observe(rule, v); err != nil {
				return nil, r.decimalsError(rule, p, err)
			}
		}
	}
	if err := w.settle(rule); err != nil {
		return nil, r.decimalsError(rule, bearer, err)
	}
	return r.compareTotals(rule, bearer, bearer, stated, sums, terms)
}

// TotalBearingPart finds the one part of a spanning rule whose page prints the
// total row. Zero fails (moved or mistyped), and two fails (ambiguous across
// pages). Exported so a check guarding that total uses the page the build
// compared against.
func (r *Resolver) TotalBearingPart(rule *Rule) (*Part, error) {
	var found []*Part
	for i := range rule.Parts {
		p := &rule.Parts[i]
		blk, err := r.block(rule, p)
		if err != nil {
			return nil, err
		}
		text, err := r.page(p.Page)
		if err != nil {
			return nil, err
		}
		if strings.Contains(text[blk.End:], rule.TotalRow) {
			found = append(found, p)
		}
	}
	switch len(found) {
	case 1:
		return found[0], nil
	case 0:
		pages := make([]int, len(rule.Parts))
		for i, p := range rule.Parts {
			pages[i] = p.Page
		}
		return nil, hint.With(
			&resolveError{DocID: r.file.DocID, RuleID: rule.ID, Page: rule.Parts[0].Page,
				Field: "total_row", Err: ErrNotFound,
				Msg: fmt.Sprintf("%q occurs after the block on none of pages %v",
					rule.TotalRow, pages)},
			"a rule declaring total_spans_parts asserts that one of its pages "+
				"prints the total; if none does, the anchor is wrong or the "+
				"document has changed")
	default:
		pages := make([]int, len(found))
		for i, p := range found {
			pages[i] = p.Page
		}
		return nil, hint.With(
			&resolveError{DocID: r.file.DocID, RuleID: rule.ID, Page: found[0].Page,
				Field: "total_row", Err: ErrNotFound,
				Msg: fmt.Sprintf("%q occurs after the block on pages %v; it must identify one",
					rule.TotalRow, pages)},
			"lengthen the anchor until it names the one page that prints this "+
				"block's total")
	}
}

// rollupResult is what checking one cross-rule rollup established.
type rollupResult struct {
	// Columns is how many non-skip columns were compared, and Rules how many
	// covered rules' stated totals were summed into each of them.
	Columns int
	Rules   int
}

// CheckRollup asserts that the totals the covered rules print sum, per column,
// to the total the document prints over all of them. It sums stated totals,
// not rows; see Rollup.
func (r *Resolver) CheckRollup(ro *Rollup) (*rollupResult, error) {
	rules, err := r.coveredRules(ro)
	if err != nil {
		return nil, err
	}
	// The parser refuses a rollup covering nothing and pkg/cmd/build skips an
	// unassertable one, but CheckRollup is exported, so it refuses here rather
	// than panic on rules[0] (fisc-3bl's class).
	if len(rules) == 0 {
		return nil, hint.With(
			&resolveError{DocID: r.file.DocID, Page: ro.Page, RuleID: ro.ID,
				Field: "covers", Err: ErrNotFound, Msg: "names no rule to sum"},
			"a rollup either lists the rules its printed total covers or "+
				"declares unassertable with the reason it cannot be checked")
	}

	// The columns come from the part that prints each rule's total, since a rule
	// may declare columns per part. They are compared element by element over the
	// whole Column, not by count: two rules with the same number of differently
	// labelled columns would otherwise sum FY2026 into FY2027 and tie. Fund and
	// skip are compared too, and the basis compared is the effective one, since
	// Column.Basis is an override (fact.FromValues falls back to the rule's).
	var cols []Column
	var sums []amount.Cents
	var first *Rule
	bearers := make([]*Part, 0, len(rules))
	for _, rule := range rules {
		stated, bearer, ruleErr := r.ruleStatedTotals(rule)
		if ruleErr != nil {
			return nil, ruleErr
		}
		bearers = append(bearers, bearer)
		if cols == nil {
			cols, sums, first = bearer.Columns, make([]amount.Cents, len(stated)), rule
		}
		if len(stated) != len(sums) {
			return nil, hint.With(
				&resolveError{DocID: r.file.DocID, Page: ro.Page, RuleID: ro.ID,
					Field: "covers", Err: ErrNotFound,
					Msg: fmt.Sprintf("rule %q states %d columns and rule %q states %d",
						first.ID, len(sums), rule.ID, len(stated))},
				"a rollup adds these totals column by column, so every covered "+
					"rule must state the same number of them")
		}
		// The width guard has refused a length mismatch, so comparable is always
		// true here; it is checked rather than trusted.
		i, comparable := firstDifferingColumn(
			effectiveColumns(first, bearers[0]), effectiveColumns(rule, bearer))
		if !comparable {
			return nil, hint.With(
				&resolveError{DocID: r.file.DocID, Page: ro.Page, RuleID: ro.ID,
					Field: "covers", Err: ErrNotFound,
					Msg: fmt.Sprintf("rule %q states %d columns on p%d and rule %q states %d on p%d",
						first.ID, len(effectiveColumns(first, bearers[0])), bearers[0].Page,
						rule.ID, len(effectiveColumns(rule, bearer)), bearer.Page)},
				"a rollup adds these totals column by column, so every covered "+
					"rule must state the same number of them")
		}
		if i >= 0 {
			return nil, hint.With(
				&resolveError{DocID: r.file.DocID, Page: ro.Page, RuleID: ro.ID,
					Field: "covers", Err: ErrNotFound,
					Msg: fmt.Sprintf("rule %q states column %d on p%d as %s and rule %q states it on p%d as %s",
						first.ID, i+1, bearers[0].Page, columnIdentity(effectiveColumns(first, bearers[0])[i]),
						rule.ID, bearer.Page, columnIdentity(effectiveColumns(rule, bearer)[i]))},
				"a rollup adds these totals column by column, so column N of "+
					"every covered rule's total must be the same column; the "+
					"parser compares each rule's FIRST part and cannot see this, "+
					"because which part bears the total needs the pages")
		}
		for i, c := range stated {
			sums[i] += c
		}
	}

	stated, at, err := r.rollupStatedTotals(ro, len(cols), rules[0].Units)
	if err != nil {
		return nil, err
	}
	if err := r.rollupNamesItsOwnLine(ro, rules, bearers, at); err != nil {
		return nil, err
	}

	res := &rollupResult{Rules: len(rules)}
	var bad []string
	for i, col := range cols {
		if col.Skip {
			continue
		}
		res.Columns++
		// STATED MINUS SUMMED, the same direction StatedTotalDelta is written
		// in, so a failure reads like every other totals failure here.
		if diff := stated[i] - sums[i]; diff != 0 {
			bad = append(bad, fmt.Sprintf("column %d (%s FY%d): the %d covered rules state %s, the document states %s, off by %s",
				i+1, col.FundGroup, col.FiscalYear, len(rules), sums[i], stated[i], diff))
		}
	}
	if len(bad) == 0 {
		return res, nil
	}
	return nil, hint.With(
		&resolveError{DocID: r.file.DocID, Page: ro.Page, RuleID: ro.ID,
			Field: "rollups", Msg: strings.Join(bad, "; ")},
		"a rollup sums the totals its covered rules PRINT, each already tied "+
			"to its own rows; a gap here is a rule missing from covers, a rule "+
			"counted twice, or a figure the document does not itemise")
}

// coveredRules resolves a rollup's rule ids in the order it names them.
func (r *Resolver) coveredRules(ro *Rollup) ([]*Rule, error) {
	byID := make(map[string]*Rule, len(r.file.Rules))
	for i := range r.file.Rules {
		byID[r.file.Rules[i].ID] = &r.file.Rules[i]
	}
	out := make([]*Rule, 0, len(ro.Covers))
	for _, id := range ro.Covers {
		rule, ok := byID[id]
		if !ok {
			return nil, &resolveError{DocID: r.file.DocID, Page: ro.Page, RuleID: ro.ID,
				Field: "covers", Err: ErrNotFound,
				Msg: fmt.Sprintf("no rule %q in this file", id)}
		}
		out = append(out, rule)
	}
	return out, nil
}

// ruleStatedTotals is the one total a covered rule prints. A rule printing a
// total on each of two pages is refused rather than summed;
// total_spans_parts gives such a rule one total.
func (r *Resolver) ruleStatedTotals(rule *Rule) ([]amount.Cents, *Part, error) {
	if rule.TotalSpansParts {
		bearer, err := r.TotalBearingPart(rule)
		if err != nil {
			return nil, nil, err
		}
		stated, err := r.StatedTotals(rule, bearer)
		return stated, bearer, err
	}
	var bearing []*Part
	for i := range rule.Parts {
		p := &rule.Parts[i]
		if p.LabelsFrom == 0 && rule.TotalRow != "" {
			bearing = append(bearing, p)
		}
	}
	if len(bearing) != 1 {
		return nil, nil, hint.With(
			&resolveError{DocID: r.file.DocID, RuleID: rule.ID, Page: rule.Parts[0].Page,
				Field: "total_row", Err: ErrNotFound,
				Msg: fmt.Sprintf("%d of this rule's parts print a total, and a rollup needs one",
					len(bearing))},
			"a rule covered by a rollup contributes one printed total; where "+
				"its rows straddle a page break, declare total_spans_parts")
	}
	stated, err := r.StatedTotals(rule, bearing[0])
	return stated, bearing[0], err
}

// rollupStatedTotals reads the figures the document prints on the rollup's own
// line. The anchor must occur exactly once on the page, since a rollup has no
// block to narrow the search.
func (r *Resolver) rollupStatedTotals(ro *Rollup, n int, units amount.Units) ([]amount.Cents, int, error) {
	text, err := r.page(ro.Page)
	if err != nil {
		return nil, 0, err
	}
	first := strings.Index(text, ro.TotalRow)
	if first < 0 {
		return nil, 0, &resolveError{DocID: r.file.DocID, Page: ro.Page, RuleID: ro.ID,
			Field: "total_row", Err: ErrNotFound,
			Msg: fmt.Sprintf("%q does not occur on the page", ro.TotalRow)}
	}
	if strings.Contains(text[first+len(ro.TotalRow):], ro.TotalRow) {
		return nil, 0, hint.With(
			&resolveError{DocID: r.file.DocID, Page: ro.Page, RuleID: ro.ID,
				Field: "total_row", Err: ErrNotFound,
				Msg: fmt.Sprintf("%q occurs more than once on the page", ro.TotalRow)},
			"lengthen the anchor until it names one printed line")
	}
	line := text[first+len(ro.TotalRow):]
	if i := strings.IndexByte(line, '\n'); i >= 0 {
		line = line[:i]
	}
	totals, ok := amountRun(line, n, units)
	if !ok {
		return nil, 0, hint.With(
			&resolveError{DocID: r.file.DocID, Page: ro.Page, RuleID: ro.ID,
				Field: "total_row", Err: ErrNotFound,
				Msg: fmt.Sprintf("no run of %d consecutive amounts follows %q on %q",
					n, ro.TotalRow, strings.TrimSpace(line))},
			"the rollup's figures must be on the same printed line as its "+
				"anchor; where the label wraps, anchor on the part that "+
				"carries the figures")
	}
	return totals, first, nil
}

// rollupNamesItsOwnLine refuses a rollup anchored on a printed line that is
// already one of its covered rules' totals, which would tie by construction.
// Six of pp.167-170's eleven <DEPARTMENT> TOTAL rows sit over a single
// division, so the test is on the line, not a cover count. The parser refuses
// the string form; `total_row: " Total"` over a division totalled "Total"
// reaches here. A rule whose total is on another page cannot collide and is
// skipped.
func (r *Resolver) rollupNamesItsOwnLine(ro *Rollup, rules []*Rule, bearers []*Part, at int) error {
	text, err := r.page(ro.Page)
	if err != nil {
		return err
	}
	want := lineAt(text, at)
	for i, rule := range rules {
		p := bearers[i]
		if p.Page != ro.Page {
			continue
		}
		blk, err := r.block(rule, p)
		if err != nil {
			return err
		}
		from, _, err := r.totalAnchor(rule, p, blk, text)
		if err != nil {
			// StatedTotals already reported anything wrong with this rule's
			// own anchor; this check has nothing to add about it.
			continue
		}
		if lineAt(text, from) == want {
			return hint.With(
				&resolveError{DocID: r.file.DocID, Page: ro.Page, RuleID: ro.ID,
					Field: "total_row", Err: ErrNotFound,
					Msg: fmt.Sprintf("names the same printed line as rule %q's total_row %q",
						rule.ID, rule.TotalRow)},
				"a rollup asserts a total the document prints OVER its covered "+
					"rules; anchored on one of their own totals it would compare "+
					"a figure with itself and tie whatever the rows said")
		}
	}
	return nil
}

// The two anchors a part's stated totals are read from.
const (
	AnchorTotalRow = "total_row"
	AnchorStopAt   = "stop_at"
)

// AnchorOf is which anchor a part's stated totals are read from: a part whose
// labels come from another part has none of its own and ends at its block's
// stop_at; any other part reads its rule's total_row. totalAnchor branches on
// it, and so does any caller naming the line.
func AnchorOf(p *Part) string {
	if p.LabelsFrom != 0 {
		return AnchorStopAt
	}
	return AnchorTotalRow
}

// totalAnchor returns the offset just past the anchor a part's stated totals
// are read from, and the field name an error about it should carry. CheckRollup
// shares it to ask whether a rollup names a covered rule's line.
func (r *Resolver) totalAnchor(rule *Rule, p *Part, blk *block, text string) (int, string, error) {
	if AnchorOf(p) == AnchorStopAt {
		if p.StopAt == "" {
			return 0, "stop_at", &resolveError{DocID: r.file.DocID, RuleID: rule.ID,
				Page: p.Page, Field: "stop_at", Err: ErrNoStatedTotals,
				Msg: "a label-less part needs a stop_at anchor to find its totals"}
		}
		return blk.End, "stop_at", nil
	}
	if rule.TotalRow == "" {
		return 0, "total_row", &resolveError{DocID: r.file.DocID, RuleID: rule.ID,
			Page: p.Page, Field: "total_row", Msg: "the rule declares none",
			Err: ErrNoStatedTotals}
	}
	// A total printed above its rows is on the section anchor's line, already
	// resolved by Block: blk.Start is just past the label. See Rule.TotalRowAbove
	// and fisc-h96o.
	if rule.TotalRowAbove {
		return blk.Start, "total_row", nil
	}
	i := strings.Index(text[blk.End:], rule.TotalRow)
	if i < 0 {
		return 0, "total_row", &resolveError{DocID: r.file.DocID, RuleID: rule.ID,
			Page: p.Page, Field: "total_row", Err: ErrNotFound,
			Msg: fmt.Sprintf("%q does not occur after the block", rule.TotalRow)}
	}
	at := blk.End + i + len(rule.TotalRow)
	if rule.TotalRowTail != "" {
		if got := lineAfter(text, at); got != rule.TotalRowTail {
			return 0, "total_row_tail", &resolveError{DocID: r.file.DocID, RuleID: rule.ID,
				Page: p.Page, Field: "total_row_tail", Err: ErrNotFound,
				Msg: fmt.Sprintf("the line after %q prints %q, not %q",
					rule.TotalRow, got, rule.TotalRowTail)}
		}
	}
	return at, "total_row", nil
}

// lineAfter is the line after the one off falls on, trimmed, or "" on the
// page's last line.
func lineAfter(text string, off int) string {
	nl := strings.IndexByte(text[off:], '\n')
	if nl < 0 {
		return ""
	}
	next := text[off+nl+1:]
	if end := strings.IndexByte(next, '\n'); end >= 0 {
		next = next[:end]
	}
	return strings.TrimSpace(next)
}

// lineAt is the 0-based index of the line offset off falls on. Two anchors that
// answer the same here name one printed line, whatever strings they are.
func lineAt(text string, off int) int { return strings.Count(text[:off], "\n") }

// abs is |c|. It returns a negative for math.MinInt64; withinTolerance, the
// only caller, handles that.
func abs(c amount.Cents) amount.Cents {
	if c < 0 {
		return -c
	}
	return c
}

// withinTolerance reports whether a column's discrepancy is inside half a
// printed unit per row summed. The comparison is doubled rather than the bound
// halved, to stay in whole cents. The overflow guards are the point: a millions
// token admits up to ~9.2e18 cents, and abs(diff)*2 on such a diff, or on
// math.MinInt64, wraps to a value that ties. Every guard refuses.
func withinTolerance(diff, unit amount.Cents, terms int) bool {
	if unit <= 0 || terms <= 0 {
		return false
	}
	// The bound itself, guarded the same way.
	if amount.Cents(terms) > math.MaxInt64/unit {
		return false
	}
	d := abs(diff)
	if d < 0 || d > math.MaxInt64/2 {
		return false
	}
	return d*2 <= unit*amount.Cents(terms)
}

// decimalsWitness checks a printed_decimals declaration against the tokens the
// rule read, in both directions: a token printing more decimals than declared
// is refused (the tolerance would be too wide), and a declaration no token
// justifies is refused as stale. A dash is not counted as zero decimals. The
// printed total's own tokens are not witnessed, which can only fail closed.
type decimalsWitness struct {
	exact bool
}

// observe records what one value's token says about the page's precision. It
// does not test v.Column.Skip: Values has already dropped skipped cells, and
// Cells, which keeps them, must not feed it.
func (w *decimalsWitness) observe(rule *Rule, v Value) error {
	if rule.PrintedDecimals == nil {
		return nil
	}
	d, ok := amount.Decimals(v.Token, rule.Units)
	if !ok {
		return nil
	}
	switch {
	case d > *rule.PrintedDecimals:
		return fmt.Errorf("row %q prints %q, which carries %d decimal place(s), but the rule declares %d",
			v.Row.PrintedLabel(), v.Token, d, *rule.PrintedDecimals)
	case d == *rule.PrintedDecimals:
		w.exact = true
	}
	return nil
}

// settle refuses a declaration the page does not justify.
func (w *decimalsWitness) settle(rule *Rule) error {
	if rule.PrintedDecimals == nil || w.exact {
		return nil
	}
	// "Summed into a compared column", not "reads": the witness never sees
	// skipped tokens.
	return fmt.Errorf("declares %d printed decimal place(s), but no token summed into a "+
		"compared column prints that many", *rule.PrintedDecimals)
}

// decimalsError wraps a witness complaint as a resolve error.
func (r *Resolver) decimalsError(rule *Rule, p *Part, err error) error {
	return hint.With(&resolveError{DocID: r.file.DocID, RuleID: rule.ID, Page: p.Page,
		Field: "printed_decimals", Err: ErrNotFound, Msg: err.Error()},
		"printed_decimals says how finely THIS page prints, and the tolerance is "+
			"derived from it; a count the page does not bear out would size that "+
			"tolerance from a unit the document never used")
}

// compareTotals is the arithmetic both totals checks share: per column, the
// mapped sum against the stated total, with any declared discrepancy applied.
// cols is the part whose columns are compared and dec the part whose
// stated_total_deltas apply; they differ for a spanning rule, whose delta
// belongs to the part printing the total.
func (r *Resolver) compareTotals(rule *Rule, cols, dec *Part, stated, sums []amount.Cents,
	terms []int) (*totalsResult, error) {
	p := cols

	// The tolerance, zero unless the rule declares printed_decimals. The parser
	// bounds the count, but a DigitCents error is still refused.
	var unit amount.Cents
	if rule.PrintedDecimals != nil {
		u, ok := amount.DigitCents(rule.Units, *rule.PrintedDecimals)
		if !ok {
			return nil, &resolveError{DocID: r.file.DocID, RuleID: rule.ID, Page: p.Page,
				Field: "printed_decimals", Err: ErrNotFound,
				Msg: fmt.Sprintf("%d decimal places is not exact at %s",
					*rule.PrintedDecimals, string(rule.Units))}
		}
		unit = u
	}

	// Declared discrepancies, by 1-based column, already validated by the parser.
	declared := make(map[int]amount.Cents, len(dec.StatedTotalDeltas))
	for _, d := range dec.StatedTotalDeltas {
		declared[d.Column] = d.Cents
	}

	res := &totalsResult{}
	var bad []string
	for c, col := range p.Columns {
		if col.Skip {
			continue
		}
		res.Columns++
		// STATED MINUS MAPPED, the same direction StatedTotalDelta.Cents is
		// written in, so a failure message can be pasted into a declaration.
		diff := stated[c] - sums[c]
		want, isDeclared := declared[c+1]
		if isDeclared {
			res.Declared++
		}
		if diff == want {
			continue
		}
		// Tried only on an undeclared column, so a delta never gains slack.
		if !isDeclared && withinTolerance(diff, unit, terms[c]) {
			res.Tolerated++
			res.Slack = append(res.Slack, diff)
			continue
		}
		switch {
		case !isDeclared:
			bad = append(bad, fmt.Sprintf("column %d (%s): mapped %s, document states %s, off by %s",
				c+1, totalsColumn(col), sums[c], stated[c], diff))
		case diff == 0:
			// The declaration has outlived the discrepancy, so it fails as stale.
			bad = append(bad, fmt.Sprintf("column %d (%s): declares a delta of %s but now ties exactly; remove the declaration",
				c+1, totalsColumn(col), want))
		default:
			bad = append(bad, fmt.Sprintf("column %d (%s): mapped %s, document states %s, declared delta %s but the difference is %s",
				c+1, totalsColumn(col), sums[c], stated[c], want, diff))
		}
	}
	// A tolerance no column needed fails, mirroring the stale-delta arm.
	if len(bad) == 0 && rule.PrintedDecimals != nil && res.Tolerated == 0 {
		return nil, hint.With(&resolveError{DocID: r.file.DocID, RuleID: rule.ID, Page: p.Page,
			Field: "printed_decimals",
			Msg: fmt.Sprintf("declares %d printed decimal places, but all %d compared column(s) tie exactly",
				*rule.PrintedDecimals, res.Columns)},
			"remove the declaration; a tolerance is earned by a discrepancy the "+
				"document actually prints, and one that loosens nothing is a claim "+
				"about the page that has stopped being true")
	}
	if len(bad) == 0 {
		return res, nil
	}
	return nil, hint.With(&resolveError{DocID: r.file.DocID, RuleID: rule.ID, Page: p.Page,
		Field: "total_row", Msg: strings.Join(bad, "; ")},
		"a mapped column that does not tie means a row was missed, "+
			"double-counted, or read from the wrong column -- unless the "+
			"document's own arithmetic rounds, which is declared per column "+
			"with stated_total_deltas and never absorbed silently")
}

// token is a whitespace-delimited run of text and where it sits in the page.
type token struct {
	text string
	off  int
}

// tokens splits s on whitespace, recording each token's offset relative to
// base. A pattern scan would silently pick digits out of a token it did not
// understand.
func tokens(s string, base int) []token {
	var out []token
	i := 0
	for i < len(s) {
		for i < len(s) && isSpace(s[i]) {
			i++
		}
		if i >= len(s) {
			break
		}
		j := i
		for j < len(s) && !isSpace(s[j]) {
			j++
		}
		out = append(out, token{text: s[i:j], off: base + i})
		i = j
	}
	return out
}

func isSpace(b byte) bool {
	switch b {
	case ' ', '\t', '\n', '\r', '\v', '\f':
		return true
	}
	return false
}

// currencyHint is the guidance for a standalone currency mark the tokenizer
// could not attach to a figure.
const currencyHint = "a '$' printed as its own token belongs to the figure after " +
	"it; one with no figure after it is not a currency mark and the rule is " +
	"reading past the end of the row"

// dropCurrencyMarks removes tokens that are a currency mark and nothing else,
// so "$ 1,234  $ 5,678" reads as two figures. 236 of the corpus's 786 pages
// print the dollar sign detached. It drops the mark rather than joining it,
// because fact-offset-points-at-token requires the token to be the page's
// bytes at its offset. A mark with no figure after it is an error: the read
// has run off the end of the row.
func dropCurrencyMarks(toks []token) ([]token, error) {
	if !slices.ContainsFunc(toks, func(t token) bool { return isCurrencyMark(t.text) }) {
		return toks, nil
	}
	out := make([]token, 0, len(toks))
	for i, t := range toks {
		if !isCurrencyMark(t.text) {
			out = append(out, t)
			continue
		}
		if i+1 >= len(toks) || isCurrencyMark(toks[i+1].text) {
			return nil, fmt.Errorf("%q is a currency mark with no figure after it", t.text)
		}
	}
	return out, nil
}

// isCurrencyMark reports whether a token is a currency symbol carrying no
// digits. amount.Parse rejects a bare "$" and must keep doing so.
func isCurrencyMark(s string) bool { return s == "$" }

// firstDifferingColumn is the index where two column runs disagree, or -1,
// and whether they were comparable at all: runs of different lengths are not.
// Whole Column values are compared, since every field changes what a figure
// means.
func firstDifferingColumn(a, b []Column) (idx int, comparable bool) {
	if len(a) != len(b) {
		return 0, false
	}
	for i := range a {
		if a[i] != b[i] {
			return i, true
		}
	}
	return -1, true
}

// effectiveColumns is a rule's bearer-part columns with the basis in force
// filled in, and the kind on a column that carries the category.
func effectiveColumns(rule *Rule, p *Part) []Column {
	out := make([]Column, len(p.Columns))
	for i, c := range p.Columns {
		c.Basis = c.EffectiveBasis(rule)
		if c.Category != "" {
			c.Kind = c.EffectiveKind(rule)
		}
		out[i] = c
	}
	return out
}

// columnIdentity renders a column as what it identifies, for "are these the
// same column": every field discriminates, Skip included. describeColumn is
// the positional rendering. Fund and basis appear only when set.
func columnIdentity(c Column) string {
	out := fmt.Sprintf("%s FY%d", c.FundGroup, c.FiscalYear)
	if c.Basis != "" {
		out += " " + string(c.Basis)
	}
	if c.Fund != 0 {
		out += fmt.Sprintf(" fund %d", c.Fund)
	}
	if c.Quantity != "" {
		out += " " + string(c.Quantity)
	}
	if c.Category != "" {
		out += " category " + c.Category
	}
	if c.Kind != "" {
		out += " kind " + string(c.Kind)
	}
	if c.Skip {
		out += " (skipped)"
	}
	return out
}

// totalsColumn names a column in a totals message: its fund group and year,
// or for a column whose years are on its rows, its fund and category.
func totalsColumn(c Column) string {
	if c.FiscalYear != 0 {
		return strings.TrimSpace(fmt.Sprintf("%s FY%d", c.FundGroup, c.FiscalYear))
	}
	var parts []string
	if c.FundGroup != "" {
		parts = append(parts, c.FundGroup)
	}
	if c.Fund != 0 {
		parts = append(parts, fmt.Sprintf("fund %d", c.Fund))
	}
	if c.Category != "" {
		parts = append(parts, c.Category)
	}
	return strings.Join(parts, " ")
}
