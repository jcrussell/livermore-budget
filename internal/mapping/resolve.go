package mapping

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"sync"
	"unicode"

	"github.com/jcrussell/livermore-budget/internal/amount"
	"github.com/jcrussell/livermore-budget/internal/geom"
	"github.com/jcrussell/livermore-budget/internal/quantity"
	"github.com/jcrussell/livermore-budget/pkg/cmdutil"
)

// doc is the view of an extracted document a resolver needs. It is declared
// here, in the consumer, and kept to the methods actually used
// (byob-interfaces.1, byob-interfaces.2). That is why this package does not
// import internal/corpus at all: rules are resolved against a document
// identity and the two substrates the extractor emits, and nothing else. The
// geometry type is named through internal/geom, which is a leaf both this
// package and the artifact reader depend on, so naming it here does not put
// the artifact reader above the judgment layer.
//
// Geometry is a required method rather than an optional capability discovered
// by type assertion, and the difference is the whole argument. With an
// assertion, a document that did not implement it would resolve with the
// column guard silently switched off -- and a guard that can be absent without
// anything saying so is the quiet degradation this package exists to refuse. A
// part that does not ask for the guard simply never calls it.
type doc interface {
	DocID() string
	Page(n int) (string, error)
	Geometry(n int) (*geom.Page, error)
}

// Resolution failures, distinguishable with errors.Is because they call for
// different responses: a locator that found nothing may need re-anchoring,
// while one that found several needs the rule to say which.
var (
	// ErrNotFound reports a locator that resolved to nothing.
	ErrNotFound = errors.New("locator did not resolve")
	// ErrAmbiguous reports a locator that resolved to more than one candidate.
	ErrAmbiguous = errors.New("locator resolved to more than one candidate")
	// ErrNoStatedTotals reports a part the document prints no total for. It is
	// not a defect — most schedules print none — but it is not a pass either,
	// so it is a distinct value rather than a silent nil.
	ErrNoStatedTotals = errors.New("part has no stated totals to check against")
)

// resolveError locates a resolution failure in both the rule and the document,
// because either one can be what is wrong.
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
//
// It is constructed per (document, rule file) pair so the doc_id agreement can
// be checked once, at the seam, rather than trusted at every call: resolving
// the ACFR's rules against the Budget Book's pages would not fail loudly, it
// would produce confident wrong figures carrying working-looking provenance.
type Resolver struct {
	doc  doc
	file *File

	// mu guards pages and parts, which are the resolver's two memos.
	mu sync.Mutex
	// pages caches page text. A rule file resolves the same handful of pages
	// many times over — every part reads its page, and checking a part's
	// totals reads it again — and re-decoding a page per call turned one
	// CheckTotals into four reads of the same file.
	pages map[int]string
	// parts caches each part's read, which is the expensive half of the work
	// the page cache does not cover.
	parts map[partKey]*resolvedPart
	// pairings caches each page's two substrates reconciled against each other,
	// which is what the column guard reads. See Resolver.pairing.
	pairings map[int]*pairing
}

// partKey identifies one part of the rule file this resolver reads. The rule
// id and page suffice: a resolver is built per rule file, rule ids are unique
// within one, and the parser rejects a rule that lists a page twice. A rule
// assembled in memory has not been through the parser, so a caller that builds
// one by hand and gives two parts the same page gets the first part's figures
// for both — the same class of caveat the negative-ordinal branch of anchor
// carries.
type partKey struct {
	rule string
	page int
}

// resolvedPart memoizes one part's read, error included. A part that failed
// must fail identically when it is asked again: retrying would make the answer
// depend on how many times a caller happened to ask, and a caller that got a
// different answer the second time could publish either one.
//
// Caching the error deliberately contradicts what page does one field above,
// which returns before storing so that a failed read IS retried. The two are
// not symmetric and should not be read as such: a page read is I/O and may fail
// transiently, while a part read is a pure function of page text and a rule, so
// a second attempt can only differ if the page text did — and for a single-shot
// CLI over local files, a page that changed mid-run is not a case to paper over.
type resolvedPart struct {
	cells     []Value
	omissions []Omission
	err       error
}

// NewResolver pairs a rule file with the document it maps.
func NewResolver(d doc, f *File) (*Resolver, error) {
	if f.DocID != d.DocID() {
		return nil, cmdutil.WithHint(
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
	// Start and End are byte offsets into the page text. End is where the
	// stop_at anchor begins, which is also where a stated totals row starts.
	Start, End int
	Text       string
}

// Value is one figure a part yielded, with enough provenance to cite it
// without resolving the page a second time.
type Value struct {
	Cents  amount.Cents
	Row    Row
	Column Column
	// RowIndex is the row's position in the rule's row list, and ColumnIndex
	// its column's position in the part's columns, so a caller can rebuild the
	// grid. RowIndex deliberately indexes the rule's rows rather than the
	// part's active rows: two parts of one table print different rows, and
	// two indexing bases would collide the moment a row another page prints
	// sat anywhere but at the end.
	RowIndex, ColumnIndex int
	Page                  int
	// Offset is the byte offset of the token within the page text, and Token
	// is the text that was parsed. Both exist so a provenance link can point
	// at the figure rather than at the page.
	Offset int
	Token  string
}

// Category is the category this figure's facts carry: its column's on a rule
// whose columns carry the category, its row's otherwise. Parse refuses a rule
// that puts one on both axes, so the precedence decides nothing it admits.
func (v Value) Category() string {
	if v.Column.Category != "" {
		return v.Column.Category
	}
	return v.Row.Category
}

// Kind is the kind this figure's facts carry, following the same axis as
// Category: the column's or the row's override, the rule's where it states none.
func (v Value) Kind(rule *Rule) Kind {
	if v.Column.Category != "" {
		return v.Column.EffectiveKind(rule)
	}
	return v.Row.EffectiveKind(rule)
}

// Omission is one cell of a row a part prints that the page leaves blank,
// declared by Part.OmittedCells. A row the part does not print is no
// omission: another page prints it, and Row.Page says which.
//
// Whether a blank becomes a fact (as a zero) is deliberately not decided here.
// The argument for it is conditional: with one blank and a column total that
// ties, the tie proves the missing cell is zero — but with two blanks in one
// column it proves nothing, and "absent is not zero" is a project invariant.
// The fact model decides; resolution only reports what was declared.
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

	// A stop_at is counted from the block's start, not the page's, and its
	// message says so: an author recounting from the top gets another number.
	where := "on the page"
	if from > 0 {
		where = "after the block's start"
	}
	fail := func(err error, msg, hint string) (int, error) {
		return 0, cmdutil.WithHint(&resolveError{DocID: r.file.DocID, RuleID: rule.ID,
			Page: p.Page, Field: field, Msg: msg, Err: err}, hint)
	}

	switch {
	case ordinal < 0:
		// The parser rejects this, but a rule built in memory has not been
		// through the parser, and a negative index here would panic rather
		// than report anything.
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

// describeAt renders up to three candidate offsets as "#N line L: <context>",
// which is what makes an ambiguity message actionable: the reader needs to see
// which occurrence is which without opening the page.
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
// Two shapes of page need two reads. Where the part carries its own row labels,
// each label anchors the figures that follow it, and the text between rows must
// be empty — that check is what catches a row the rule failed to map, on the
// majority of pages that print no total to fall back on. Where the part is a
// label-less continuation, there is nothing to anchor on and the read is
// positional, guarded by the value count.
//
// The positional read is confined to label-less parts on purpose. Run it over a
// labelled block and it silently invents rows: on Budget Book p127 it reads 14
// rows where there are 13, because labels there contain "-" (which this corpus
// spells zero) and digits ("Prop 172 - Public Sfty Augmnt").
//
// A part is read at most once per resolver. That is a performance change and
// not a correctness one: the page memo already made a second read deterministic,
// so a build's figures and the figures CheckTotals corroborates agreed before
// this cache existed. What it buys is that they agree without doing the work
// twice, which is what matters as coverage grows past ten parts. The one
// behavioural difference is on the failing path, described on resolvedPart.
//
// The slices returned are copies, so a caller may sort or rewrite them without
// changing what the next caller sees. The copy is shallow: a Value's Row
// shares its Counterpart and SubtotalDeltas with the rule, and a caller must
// not write through them.
//
// The values are the cells cellPublishes admits. The read yields every cell
// the part prints, skipped ones included, because a printed subtotal adds
// them (Cells); the one predicate for which of those become facts is applied
// here and nowhere after.
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

// Cells is every amount the part reads, in Values' order: its values, and the
// figures under a skipped row or column, which publish nothing but which a
// printed subtotal still adds.
func (r *Resolver) Cells(rule *Rule, p *Part) ([]Value, error) {
	rp := r.resolvePart(rule, p)
	if rp.err != nil {
		return nil, rp.err
	}
	return slices.Clone(rp.cells), nil
}

// resolvePart returns the memoized read of one part, performing it on the first
// ask.
//
// The lock is deliberately not held across the read: the read calls page,
// which takes the same lock, and a sync.Mutex is not reentrant. Two callers
// racing on one part therefore both do the work — and the first result stored
// is the one both of them see, so the memo stays single-valued, which is the
// property callers depend on.
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

// storePart records rp under key and returns whichever result is now canonical,
// which is an earlier one if a racing caller got there first.
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

	// Built once per part. A part that declares no column_headers gets a nil
	// guard and is read exactly as it was before geometry existed.
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

// canonicalRows returns the part's active rows paired with each one's index in
// the rule's row list. Values and Omissions both index that list, so a caller
// can lay them out together.
//
// It walks the rule's rows by position rather than pairing ActiveRows back by
// identity: two skipped rows may share an identity on different pages, and
// pairing would give one the other's index.
func canonicalRows(rule *Rule, p *Part) ([]Row, []int) {
	var active []Row
	var idx []int
	for i, row := range rule.Rows {
		if row.OnPart(p) {
			active = append(active, row)
			idx = append(idx, i)
		}
	}
	return active, idx
}

// Omissions is the cells a part declares the page leaves blank, in row order.
// It reads the declaration alone, so a caller with no page in hand gets what
// Values would, and internal/check reads its blanks from here and matches
// nothing of its own.
func Omissions(rule *Rule, p *Part) []Omission {
	if len(p.OmittedCells) == 0 {
		return nil
	}
	// Keyed on Identity(), the key the rule's rows are unique on. Matching
	// the bare Label here would report a blank on every row sharing it
	// (fisc-gtv).
	blank := blankColumns(p)
	var out []Omission
	for i, row := range rule.Rows {
		for c, h := range p.ColumnHeaders {
			if blank[row.Identity()][c] {
				out = append(out, Omission{Row: row, RowIndex: i, Page: p.Page,
					ColumnIndex: c, Header: h.Text})
			}
		}
	}
	return out
}

func (r *Resolver) labelledValues(rule *Rule, p *Part, blk *block, guard *columnGuard) ([]Value, error) {
	rows, rowIndex := canonicalRows(rule, p)
	blank := blankColumns(p)
	fail := func(field, msg, hint string) error {
		return cmdutil.WithHint(&resolveError{DocID: r.file.DocID, RuleID: rule.ID,
			Page: p.Page, Field: field, Msg: msg, Err: ErrNotFound}, hint)
	}

	values := make([]Value, 0, rule.expectedValues(p))
	used := map[string]bool{}
	cursor := 0
	for i, row := range rows {
		j := strings.Index(blk.Text[cursor:], row.Label)
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
		cols := printedColumns(p, blank, row)
		if len(toks) < len(cols) {
			return nil, fail("rows", fmt.Sprintf(
				"row %q is followed by %d values, want %d (one per column it prints)",
				row.PrintedLabel(), len(toks), len(cols)), "check the part's columns against the page")
		}
		toks = toks[:len(cols)]
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

	// Anything after the last row's figures is a row the rule did not map --
	// unless the page wrapped a label there, which is the same shape as a gap
	// between two rows and is declared the same way.
	if rest := strings.TrimSpace(blk.Text[cursor:]); rest != "" && !declaredGap(p, blk.Text[cursor:], used, true, nil) {
		return nil, fail("rows", fmt.Sprintf(
			"%q follows the last mapped row but is not mapped", rest),
			"every row inside the block must be listed in rows, with skip: true "+
				"if it should not produce facts, in wrapped_labels if the page "+
				"wrapped a label onto its own line, in headings if it is a section "+
				"heading, or in unmapped_text if it is a figure belonging to no row")
	}
	// A declared fragment the page did not use is a claim about the document
	// that has stopped being true. Same direction as a stated_total_delta that
	// now ties exactly: remove the declaration rather than let it pass.
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
	// Its own arm, and its own message. Told that a figure "is declared but
	// does not appear", a reader who saw the wrapped_labels wording would go
	// looking for a wrapped label -- and the whole reason these are two
	// declarations rather than one is that they say different things about the
	// page. This is the ONLY thing standing over an unmapped_text entry: see
	// Part.UnmappedText on why the class is the weakest here.
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
// are words; between rows nothing at all may intervene, because anything that
// does is a row the rule has not mapped. raised is declaredGap's, for a gap
// between two rows.
func (r *Resolver) checkGap(rule *Rule, p *Part, gap string, rows []Row, i int,
	used map[string]bool, raised func(off int) bool) error {
	if i == 0 {
		// A LABEL WRAPPED BEFORE THE FIRST MAPPED ROW IS DECLARABLE HERE, and
		// until fisc-2jk it was the one place a fragment was invisible either
		// way: declaring it failed as a stale declaration, because only the
		// between-rows and after-last-row paths marked anything used, and NOT
		// declaring it passed silently. A shape that can be neither stated nor
		// omitted is not a choice the author gets to make.
		//
		// The permissiveness below is kept rather than tightened, and the reason
		// is specific: this gap is also where COLUMN HEADERS live, which are
		// words, are printed on every schedule, and are declared through
		// column_headers rather than here. Requiring a declaration for undeclared
		// leading text would demand one for every header on every part.
		// A total printed ABOVE its rows puts the block's own stated totals in
		// this gap, on the section anchor's line, so the digit refusal below
		// would fire on the very figures the rule went there to read. Skip that
		// ONE line and no more: everything after it is still refused, so "the
		// block starts too early" keeps its whole meaning for a rule that
		// starts two lines early instead of one.
		//
		// IT RUNS BEFORE THE wrapped_labels TEST AND NOT AFTER, which is the
		// order the first draft got wrong. Matching a wrapped label against the
		// whole gap first meant that under total_row_above the label had to be
		// declared WITH the total's figures glued to the front of it, which no
		// author would write -- so a page wrapping a label between the total's
		// line and the first row had a fragment that was refused if undeclared
		// and stale if declared. That is fisc-2jk's failure mode exactly,
		// reintroduced under a new flag. Skipping first leaves every check below
		// looking at the same shape it sees on every other rule, and for a rule
		// without the flag the skip is a no-op, so nothing else changes.
		//
		// IndexByte-guarded, deliberately, and not strings.Cut: Cut returns an
		// empty remainder when there is no newline at all, which would make the
		// refusal below pass VACUOUSLY on a gap that lies entirely on the
		// anchor's line. That gap is not the safe case -- it is the case where
		// the first row's label was found on the same printed line as the
		// total, which means the anchors matched something other than the block
		// this rule describes.
		if rule.TotalRowAbove {
			nl := strings.IndexByte(gap, '\n')
			if nl < 0 {
				return cmdutil.WithHint(&resolveError{DocID: r.file.DocID, RuleID: rule.ID,
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
		// NOTE: unmapped_text is deliberately NOT honoured here. This gap is
		// where column headers live and is already permissive about words, so
		// the digit refusal is the ONLY guard standing over it; letting a
		// declared figure through would retire that guard, and with the
		// total-line skip above there would be nothing left. A stray figure
		// before the first row therefore still cannot be declared -- a real gap,
		// and fisc-0cff rather than an oversight. No page in the corpus needs it.
		if strings.ContainsFunc(gap, unicode.IsDigit) {
			return cmdutil.WithHint(&resolveError{DocID: r.file.DocID, RuleID: rule.ID,
				Page: p.Page, Field: "section", Err: ErrNotFound,
				Msg: fmt.Sprintf("figures appear before the first row %q: %q",
					rows[0].Label, strings.TrimSpace(gap))},
				"the block starts too early — move the section anchor past the "+
					"column headers, or the first row's figures will be read as "+
					"part of the header")
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
	return cmdutil.WithHint(&resolveError{DocID: r.file.DocID, RuleID: rule.ID,
		Page: p.Page, Field: "rows", Err: ErrNotFound,
		Msg: fmt.Sprintf("%q sits between rows %q and %q but is not mapped",
			trimmed, rows[i-1].PrintedLabel(), rows[i].PrintedLabel())},
		"add it to rows, with skip: true if it should not produce facts, to "+
			"wrapped_labels if the page wrapped a label onto its own line, to "+
			"headings if it is a section heading, or to unmapped_text if it is a "+
			"figure belonging to no row; leaving it out would publish a breakdown "+
			"that does not add up")
}

// declaredGap reports whether a trimmed gap is text the part declares, and
// marks what it used. The gap is either one declared wrapped label or several
// declared lines: Budget Book p222 ends one row's description and begins the
// next row's on the two lines between their figures, and p190 prints a
// footnote marker above a heading. Each line is matched whole and trimmed, as
// a wrapped label, a heading, or -- when figures is set -- an unmapped_text
// figure, so the declaration names lines rather than the run of spaces between
// them, and a line that is not declared refuses the whole gap.
//
// A declared figure must be the whole gap, or a footnote marker in a gap of
// headings and such markers alone, on the line immediately above a heading or
// on the gap's last line, keying the row below it (p194). A marker is what
// raised says the page prints as one, given the line's offset in gap; it is
// nil after the last row, where no row follows for a marker to key, and
// before the first. A wrapped label never shares a gap with a figure, so the
// declaration cannot admit a row broken over two lines, nor a two-digit figure
// printed among a label's fragments.
//
// figures is false only before the first row, where unmapped_text is not
// honoured; see checkGap.
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

// findFields finds the first run of s that is tail's whitespace-separated
// fields in order, separated by spaces or tabs and never by a line break, and
// returns its offset and length, or -1. A tail spanning several printed fields
// names each field and not the kerning between them, which is what p222's
// "600       Airport" after a label of "601" needs.
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

// declaresUnmapped reports whether the part declares this exact printed line as
// a figure belonging to no row.
//
// It takes one whole trimmed line, exactly as the wrapped_labels and headings
// tests do, so a declaration covers a line and never part of one; declaredGap
// says which gaps such a line may share.
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
	// The column check happens here rather than in the two callers because this
	// is the one place that already has a row, its tokens and the columns they
	// were filed under together. Checking in both callers would be two copies of
	// the invariant the whole guard exists for.
	if guard != nil {
		if err := guard.checkRow(r, rule, p, row, toks, cols); err != nil {
			return nil, err
		}
	}
	out := make([]Value, 0, len(toks))
	for k, tk := range toks {
		c := cols[k]
		col := p.Columns[c]
		// The quantity picks the grammar BEFORE skip is consulted, for the
		// same reason amounts parse before skip: a skipped cell is still a
		// cell. A recognized non-amount cell is READ — the row stays whole and
		// checkGap stands — and never becomes a Value, so it cannot publish.
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
		// A skipped row or column still consumes its position — that is the
		// point of skip — and is returned here as a cell; Values keeps it out
		// of the part's values, so it yields no fact.
		out = append(out, Value{Cents: cents, Row: row, Column: col,
			RowIndex: rowIndex, ColumnIndex: c, Page: p.Page,
			Offset: tk.off, Token: tk.text})
	}
	return out, nil
}

// recognize dispatches a non-amount cell to its grammar. This switch is the
// one seam between the closed vocabulary and internal/quantity; a Quantity
// with no arm here is a harness error, not a claim about the document.
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
	rows, rowIndex := canonicalRows(rule, p)
	ncols := len(p.Columns)
	blank := blankColumns(p)
	// The marks are dropped here as on a labelled row: p81's first row and its
	// subtotals print "$" detached from each figure, inside the block.
	toks, err := dropCurrencyMarks(tokens(blk.Text, blk.Start))
	if err != nil {
		return nil, cmdutil.WithHint(&resolveError{DocID: r.file.DocID, RuleID: rule.ID,
			Page: p.Page, Field: "parts", Err: ErrNotFound, Msg: err.Error()}, currencyHint)
	}
	want := rule.expectedValues(p)

	if len(toks) != want {
		return nil, cmdutil.WithHint(&resolveError{DocID: r.file.DocID, RuleID: rule.ID,
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
		cols := printedColumns(p, blank, row)
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
// The anchor is the rule's total_row where the part carries labels, and the
// part's stop_at where it does not — Budget Book p67's totals row is positional
// like everything else on that page, and its "$" boundary is both the end of
// the data block and the start of the totals.
//
// From the anchor, the totals are the first run of exactly one-per-column
// consecutively parsable amounts *within the anchor's line*. Three details are
// load-bearing. The run must be maximal, or a totals label containing a bare
// number ("Total 2022 COP Construction Fund") would be read as a figure. It
// must be exactly column-width rather than a prefix, so a line printing the
// totals twice fails instead of guessing. And it must stop at the line end,
// because the next line's figures are whitespace-separated from these and
// would otherwise extend the run.
func (r *Resolver) StatedTotals(rule *Rule, p *Part) ([]amount.Cents, error) {
	_, _, totals, err := r.statedTotalLine(rule, p)
	return totals, err
}

// statedTotalLine resolves this part's stated-total line and reads it: the byte
// range its figures occupy in the page text, and the figures themselves.
//
// ANCHORING IS NOT ENOUGH AND THAT IS THE WHOLE REASON THIS IS ONE FUNCTION.
// totalAnchor returns a position; whether the line at that position PRINTS a
// total is a separate question, and for a label-less part the anchor is the
// block TERMINATOR rather than a totals row. Three committed parts anchor that
// way and land on text that is not a total at all: spine-transfers-in on p67
// lands on the fund-group header line, and spine-transfers-out and
// spine-fund-balance both land on the running footer "BUDGET FY 2025-27 Page
// 63". A caller that took the anchor as a stated total would be pointing at a
// header and a page number.
//
// So the amountRun test below is what MAKES a line a stated total, and every
// caller gets it. A caller that anchors without it reports those three lines as
// stated totals.
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
		return 0, 0, nil, cmdutil.WithHint(&resolveError{DocID: r.file.DocID, RuleID: rule.ID,
			Page: p.Page, Field: field, Err: ErrNotFound,
			Msg: fmt.Sprintf("no run of %d consecutive amounts follows the anchor on %q",
				len(p.Columns), strings.TrimSpace(line))},
			"the totals row must print one figure per column; if the page prints "+
				"them twice on one line, anchor past the first copy")
	}
	return from, to, totals, nil
}

// TotalRowSpan is the byte range, within the part's page text, of the figures
// the document prints on this part's stated-total line.
//
// It is [Resolver.StatedTotals] over the same [Resolver.statedTotalLine], asking
// for the position rather than the figures, so the two agree on which line is a
// stated total and on every error by construction rather than by inspection.
//
// WHY THE POSITION IS WORTH EXPORTING. It is the only thing that can answer "is
// this fact republishing a total", and a caller outside this package cannot
// recompute it: the anchor is narrowed by the block, and a page that prints the
// same total_row string more than once gives a plain search the wrong
// occurrence. See fisc-eaic for the hazard.
//
// THE SPAN IS [anchor, end of line), and the anchor is the byte just past the
// LABEL rather than the first byte of the figures -- the run of spaces between
// them is inside it. Fail-closed and free: a fact's offset points at its token,
// so nothing can sit in the whitespace.
func (r *Resolver) TotalRowSpan(rule *Rule, p *Part) (lo, hi int, err error) {
	lo, hi, _, err = r.statedTotalLine(rule, p)
	return lo, hi, err
}

// RollupTotalSpan is the byte range, within the rollup's page text, of the
// figures the document prints on the rollup's own total line.
//
// IT IS [Resolver.TotalRowSpan] ONE LEVEL UP, AND THE LEVEL MATTERS. A rollup is
// a printed total covering several RULES -- pp.167-170's eleven
// "<DEPARTMENT> TOTAL" rows over their divisions, and three more elsewhere in
// the file -- so its figure is a total by exactly the argument a rule's
// total_row is, and republishing one as a row doubles a department. Locating a
// rule's totals and not a rollup's leaves that hole open for the widest totals
// in the corpus.
//
// It returns ErrNoStatedTotals for a rollup that covers no rule, which is the
// unassertable case: the document prints the total and nothing here can say
// where, so there is no span to hand out.
//
// The width and units come from the FIRST covered rule's bearer part, which is
// what CheckRollup reads them from; the column-consistency refusals CheckRollup
// makes on the rest are its own and are not repeated here, because a caller
// asking where a line is does not need the covered rules to agree with it.
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
	// rollupStatedTotals is what refuses an anchor that is missing, ambiguous,
	// or followed by no run of amounts, so the span this returns is a line the
	// document really totals on -- the property TotalRowSpan had to be taught.
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
// Where a maximal run is the wrong width but begins with a currency-marked
// figure, its currency-marked prefix is considered too. These documents mark a
// totals row with "$" and print the data rows bare, and extraction routinely
// glues the two onto one line: Budget Book p67 runs the eight TOTAL
// EXPENDITURES figures straight into the eight TRANSFER OUT figures, so the
// maximal run there is sixteen and the totals are unreadable without the
// distinction the page itself draws (fisc-gxt).
//
// The maximal run is computed and preferred first, so the prefix is only ever
// consulted where a run is the wrong width. A page that marks only some of its
// totals with "$" therefore still fails closed rather than yielding a short
// run. The preference is per start position, not global: a line whose first
// currency-marked run is n wide now answers ahead of a later bare run of n,
// where before the bare one won. No line in the corpus does that, and reading
// the marked run first is the better answer anyway — but it is a behaviour
// change, not merely an addition.
// A DETACHED mark is dropped before the run is sought, exactly as the labelled
// read drops it: ACFR p167 prints its totals as "$ 47,139,536      $ 54,468,310"
// per column, so without the drop every run is one amount wide and no width can
// match. A mark with no figure after it fails the whole line, closed, through
// dropCurrencyMarks' own refusal.
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
	// Columns is how many non-skip columns were compared, which is the unit
	// coverage is counted in. A skipped column produces no facts, so counting
	// it would claim coverage the check did not earn.
	Columns int

	// Declared is how many of those columns tied only because the rule
	// declared a discrepancy (fisc-2sd). It is reported separately so a part
	// that ties on the document's own rounding cannot read as one that ties
	// exactly -- the caller says so, rather than the number quietly counting
	// as clean coverage.
	Declared int

	// Tolerated is how many columns tied only inside the tolerance the rule's
	// printed_decimals derives from the page (fisc-1wr.2), and Slack is what
	// each of those columns was actually out by, in the order they were
	// compared.
	//
	// SAME REASON AS Declared, and the reason is the whole design: a column
	// that ties within half a printed unit per row has NOT tied exactly, and a
	// report that counted the two together would publish the corpus as tighter
	// than it is. Every caller that adds Columns to a coverage figure must
	// carry these two beside it.
	Tolerated int
	Slack     []amount.Cents
}

// CheckTotals asserts that the figures a part yields sum, per column, to the
// totals the document prints for it.
//
// This is the most valuable check available, because it is the document
// checking our work rather than us checking our own. It returns
// ErrNoStatedTotals where the document prints no total, which is not a pass:
// callers are expected to report those parts as unchecked rather than silent.
//
// The figures checked are the memoized ones Values returned, not a second read
// of the same part: a check that re-read the page would be corroborating a
// different read from the one the caller published.
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

// CheckSpanningTotals asserts that the figures EVERY part of a rule yields sum,
// per column, to the one total the document prints for the block.
//
// This is CheckTotals for a block whose rows straddle a page break. Nine such
// blocks sit in the two coverage lanes: the rows land in two parts and the
// printed total in one, so a per-part check has nothing to compare the head
// part against and would either fail to resolve or report the rule as
// declaring no total at all. See Rule.TotalSpansParts for why both of those
// exits are worse than they look.
//
// The parser has already established that the parts declare identical columns,
// so summing across them is meaningful. What is established here, because it
// needs the pages: that exactly one part prints the total row, and that a
// declared discrepancy sits on that part rather than on one of the others.
func (r *Resolver) CheckSpanningTotals(rule *Rule) (*totalsResult, error) {
	bearer, err := r.totalBearingPart(rule)
	if err != nil {
		return nil, err
	}
	stated, err := r.StatedTotals(rule, bearer)
	if err != nil {
		return nil, err
	}

	// A delta is a claim about the printed total, so it belongs on the part
	// that prints it. The parser refused two parts declaring one; this refuses
	// the one part declaring it in the wrong place, which the parser cannot
	// see because it does not know which page prints the total.
	for i := range rule.Parts {
		p := &rule.Parts[i]
		if len(p.StatedTotalDeltas) == 0 || p.Page == bearer.Page {
			continue
		}
		return nil, cmdutil.WithHint(
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

// totalBearingPart finds the one part of a spanning rule whose page prints the
// total row.
//
// Exactly one, and the count is the point. Zero means the total row has moved
// or was mistyped, which must fail rather than quietly leave the block
// unchecked. Two means the anchor is ambiguous across the rule's pages, and
// taking the first would pick whichever page happens to come first in the part
// list -- a total checked against the wrong page's figures is the confident
// wrong answer this project exists to refuse.
func (r *Resolver) totalBearingPart(rule *Rule) (*Part, error) {
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
		return nil, cmdutil.WithHint(
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
		return nil, cmdutil.WithHint(
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
// to the total the document prints over all of them.
//
// It sums STATED TOTALS rather than mapped rows; see Rollup for the
// measurement that decides it. Each covered rule's own total has already been
// tied to its rows by CheckTotals, so this is the second link of a chain
// rather than a second opinion on the first.
func (r *Resolver) CheckRollup(ro *Rollup) (*rollupResult, error) {
	rules, err := r.coveredRules(ro)
	if err != nil {
		return nil, err
	}
	// A rollup covering nothing indexes rules[0] below and would panic there.
	// Two things keep that unreachable today and NEITHER IS THIS FUNCTION'S:
	// the parser refuses a rollup declaring neither covers nor unassertable,
	// and pkg/cmd/build skips CheckRollup entirely for an unassertable one.
	// CheckRollup is exported, so a second caller -- the verify-side structural
	// sweep reading Subject.Resolvers is the obvious one -- reintroduces the
	// panic by doing nothing wrong. Same class as the Parts[0] panic fisc-3bl
	// already fixed here, one caller further away.
	if len(rules) == 0 {
		return nil, cmdutil.WithHint(
			&resolveError{DocID: r.file.DocID, Page: ro.Page, RuleID: ro.ID,
				Field: "covers", Err: ErrNotFound, Msg: "names no rule to sum"},
			"a rollup either lists the rules its printed total covers or "+
				"declares unassertable with the reason it cannot be checked")
	}

	// The columns come from the part that PRINTS each rule's total, not from
	// its first part. A rule may declare its columns per part, so a stated
	// total read off part 2 is stated over part 2's columns; taking the width
	// from Parts[0] and indexing with it read off the end of the sum. The
	// parser's own guard compares Parts[0] for the same reason and cannot
	// close this, because which part bears the total needs the pages.
	//
	// THE COMPARISON IS THE COLUMNS THEMSELVES AND NOT THEIR COUNT. An earlier
	// version tested only len(stated) != len(sums), which let two rules whose
	// bearer parts declare the same NUMBER of differently-labelled columns be
	// summed against each other: rule A's FY2026 general added to rule B's
	// FY2027 general, reported as a clean tie. The width test is what stops the
	// sum reading off its own end; it is not what makes the sum MEAN anything.
	//
	// Element by element over the whole Column, so fund and skip are compared
	// too. Fund matters as soon as a single-fund schedule is covered, and a
	// bearer carrying skip on a DIFFERENT column silently changes which columns
	// the tie loop below examines, because that loop reads Skip off the first
	// bearer only.
	//
	// THE BASIS COMPARED IS THE EFFECTIVE ONE, not Column.Basis. That field is
	// an OVERRIDE: fact.FromValues falls back to the rule's basis when it is
	// empty, so two rules declaring `basis: adopted` and `basis: revised` with
	// byte-identical column blocks produce facts on different bases while every
	// Column compares equal. Summing an adopted total into a revised one is the
	// same defect as summing FY2026 into FY2027, reached through the other
	// declaration, and a comparison that missed it would close half a hole while
	// claiming the whole one.
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
			return nil, cmdutil.WithHint(
				&resolveError{DocID: r.file.DocID, Page: ro.Page, RuleID: ro.ID,
					Field: "covers", Err: ErrNotFound,
					Msg: fmt.Sprintf("rule %q states %d columns and rule %q states %d",
						first.ID, len(sums), rule.ID, len(stated))},
				"a rollup adds these totals column by column, so every covered "+
					"rule must state the same number of them")
		}
		// The width guard above has already refused a length mismatch, so
		// `comparable` is true here on every reachable path. It is checked
		// rather than discarded because the alternative is indexing two slices
		// on the strength of a comment: the same one-caller-away reasoning that
		// put a panic in this helper in the first place.
		i, comparable := firstDifferingColumn(
			effectiveColumns(first, bearers[0]), effectiveColumns(rule, bearer))
		if !comparable {
			return nil, cmdutil.WithHint(
				&resolveError{DocID: r.file.DocID, Page: ro.Page, RuleID: ro.ID,
					Field: "covers", Err: ErrNotFound,
					Msg: fmt.Sprintf("rule %q states %d columns on p%d and rule %q states %d on p%d",
						first.ID, len(effectiveColumns(first, bearers[0])), bearers[0].Page,
						rule.ID, len(effectiveColumns(rule, bearer)), bearer.Page)},
				"a rollup adds these totals column by column, so every covered "+
					"rule must state the same number of them")
		}
		if i >= 0 {
			return nil, cmdutil.WithHint(
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
	return nil, cmdutil.WithHint(
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

// ruleStatedTotals is the ONE total a covered rule prints.
//
// One, and the count is load-bearing. A rule printing a total on each of two
// pages has two, and adding them would be a guess about which the rollup meant
// -- so this refuses rather than choosing. A rule whose rows straddle a page
// break says so with total_spans_parts and has one total again.
func (r *Resolver) ruleStatedTotals(rule *Rule) ([]amount.Cents, *Part, error) {
	if rule.TotalSpansParts {
		bearer, err := r.totalBearingPart(rule)
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
		return nil, nil, cmdutil.WithHint(
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
// line.
//
// The anchor must occur exactly once on the page. Unlike a rule's total_row,
// which is searched only after that rule's block and so is already narrowed by
// the block's own anchors, a rollup has no block to search after and would
// otherwise take whichever occurrence came first.
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
		return nil, 0, cmdutil.WithHint(
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
		return nil, 0, cmdutil.WithHint(
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
// already one of its covered rules' totals.
//
// THIS IS THE INVARIANT THAT REPLACED "a rollup must cover two rules". That
// count was a proxy, and a wrong one: six of pp.167-170's eleven
// <DEPARTMENT> TOTAL rows sit over a single division and are a second printed
// line, which the proxy refused. What it was protecting against is a rollup
// that asserts a figure against itself -- always green, checking nothing -- and
// that is a claim about the LINE.
//
// The parser refuses the string form (rollup.total_row equal to a covered
// rule's) and cannot do more: which part bears a rule's total, and where on the
// page it lands, needs the pages. So a rollup declaring `total_row: " Total"`
// over one division whose own total_row is "Total" reaches here, resolves to
// that division's own printed Total, and would tie by construction.
//
// A rule whose total is printed on another page cannot collide and is skipped
// rather than resolved a second time.
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
			return cmdutil.WithHint(
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
// are read from, and the field name an error about it should carry.
//
// It is split out of [Resolver.StatedTotals] because CheckRollup needs the same
// offset for a different question -- whether a rollup names the same printed
// line as one of the totals it covers -- and two computations of "where does
// this rule's printed total sit" would be two things to keep in step.
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
	// A total printed ABOVE its rows is on the section anchor's own line, and
	// Block already resolved that anchor: blk.Start is the byte just past the
	// label, which is exactly the shape the below-case returns. There is no
	// search to do and so no ErrNotFound to report -- the parser has already
	// required section == total_row, and anchor() has already refused an
	// ambiguous or missing one. See Rule.TotalRowAbove and fisc-h96o.
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

// abs is |c|.
//
// A tolerance is symmetric: a document that prints one unit LESS than its rows
// add to is rounding just as one that prints one more is.
//
// IT CAN RETURN A NEGATIVE, and every caller must say what it does about that.
// amount.Cents is an int64 and -math.MinInt64 is math.MinInt64, so the one input
// this cannot answer for is the one input that would make a naive comparison
// fail open. withinTolerance is the only caller and handles it.
func abs(c amount.Cents) amount.Cents {
	if c < 0 {
		return -c
	}
	return c
}

// withinTolerance reports whether a column's discrepancy is inside half a
// printed unit per row summed.
//
// THE COMPARISON IS DOUBLED RATHER THAN THE BOUND HALVED so the arithmetic stays
// in whole cents: an odd term count would otherwise round the tolerance, and
// which way it rounded would be the difference between a page passing and not.
//
// THE OVERFLOW GUARDS ARE THE POINT OF THE FUNCTION and are why this is not an
// inline expression any more. It was one, and it could wrap: amount.Parse admits
// a millions token up to roughly 9.2e18 cents, and `abs(diff)*2` on a diff that
// size is NEGATIVE, which compares <= any bound and ties. `diff` at exactly
// math.MinInt64 gives abs() == math.MinInt64 and a doubled value of 0, which
// also ties. Both are fail-OPEN in the one arm that decides whether a column may
// miss the total the document prints for it -- in a package that bounds its
// products explicitly everywhere else (see amount.Parse's own MaxInt64 checks).
//
// Every guard below refuses rather than accepts, because a discrepancy too large
// to compare against a bound built from a printed unit is, self-evidently, not
// within it.
func withinTolerance(diff, unit amount.Cents, terms int) bool {
	if unit <= 0 || terms <= 0 {
		return false
	}
	// The bound itself. unit is a power of ten no larger than 10^8 cents and
	// terms is a row count, so this cannot realistically wrap -- but "cannot
	// realistically" is what the guard above this one was relying on.
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
// rule actually read.
//
// THE DECLARATION IS A CLAIM ABOUT THE PAGE, so the page is what settles it,
// and it is settled in BOTH directions for different reasons. A token printing
// MORE decimals than declared is refused because it would mean the tolerance
// was computed from too coarse a unit and is therefore too wide -- the unsafe
// direction. A declaration NO token justifies is refused because it has gone
// stale, the same standard wrapped_labels and stated_total_deltas are held to.
//
// A dash is skipped rather than counted as zero decimals. These documents spell
// a zero cell "-", which says nothing about how finely the page prints; counting
// it would let one dash in a column of hundredths claim the page prints whole
// units, and that reading gives a hundredfold wider tolerance.
//
// THE PRINTED TOTAL'S OWN TOKENS ARE DELIBERATELY NOT WITNESSED. StatedTotals
// and amountRun hand back []amount.Cents and carry no token, so including them
// would change four signatures for a coarser total that could only WIDEN the
// bound. Leaving it out fails closed, which is the direction to be wrong in.
type decimalsWitness struct {
	exact bool
}

// observe records what one value's token says about the page's precision.
//
// IT DOES NOT TEST v.Column.Skip. Values drops a skipped row or column, so no
// Value reaching here carries one and such a test could not fire. Where
// skipped columns are excluded from the witness is Values, and Cells, which
// keeps them, must not feed it.
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
	// "SUMMED INTO A COMPARED COLUMN", not "reads". Values drops a skipped
	// row or column, so the witness never sees those
	// tokens -- and a rule whose compared column prints integers while its
	// SKIPPED column prints "16.00" would otherwise be told, falsely, that no
	// token it reads prints two decimals. Rule.PrintedDecimals draws exactly
	// this distinction and the message used to ignore it.
	return fmt.Errorf("declares %d printed decimal place(s), but no token summed into a "+
		"compared column prints that many", *rule.PrintedDecimals)
}

// decimalsError wraps a witness complaint as a resolve error.
func (r *Resolver) decimalsError(rule *Rule, p *Part, err error) error {
	return cmdutil.WithHint(&resolveError{DocID: r.file.DocID, RuleID: rule.ID, Page: p.Page,
		Field: "printed_decimals", Err: ErrNotFound, Msg: err.Error()},
		"printed_decimals says how finely THIS page prints, and the tolerance is "+
			"derived from it; a count the page does not bear out would size that "+
			"tolerance from a unit the document never used")
}

// compareTotals is the arithmetic both totals checks share: per column, the
// mapped sum against the stated total, with any declared discrepancy applied.
//
// cols is the part whose column list is being compared and dec the part whose
// stated_total_deltas apply. They are the same part for a per-part check and
// may differ for a spanning one, where the columns are every part's (identical
// by the parser's guard) and the declaration belongs to whichever part prints
// the total. Passing them separately is what stops a spanning rule silently
// reading a delta off the wrong page.
func (r *Resolver) compareTotals(rule *Rule, cols, dec *Part, stated, sums []amount.Cents,
	terms []int) (*totalsResult, error) {
	p := cols

	// The tolerance, derived from the page rather than chosen. Zero unless the
	// rule declares printed_decimals, which is every rule in the corpus but
	// one. The parser has already bounded the count by the units' own
	// precision, so DigitCents cannot fail here; refusing rather than assuming
	// keeps a future caller from getting a silent zero unit.
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

	// Declared discrepancies, by 1-based column. The parser has already
	// refused a duplicate, an out-of-range column, a skipped column, a zero
	// delta and a missing note, so nothing here needs to re-check any of that.
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
		// It is tried only on an UNDECLARED column. A declared delta names an
		// exact figure and must still match exactly, or the two mechanisms
		// would compose into a declaration with slack around it.
		if !isDeclared && withinTolerance(diff, unit, terms[c]) {
			res.Tolerated++
			res.Slack = append(res.Slack, diff)
			continue
		}
		switch {
		case !isDeclared:
			bad = append(bad, fmt.Sprintf("column %d (%s FY%d): mapped %s, document states %s, off by %s",
				c+1, col.FundGroup, col.FiscalYear, sums[c], stated[c], diff))
		case diff == 0:
			// The declaration has outlived the discrepancy. Failing is the
			// point: a stale claim about the city's arithmetic that nothing
			// ever retracts is exactly what a tolerance would have hidden.
			bad = append(bad, fmt.Sprintf("column %d (%s FY%d): declares a delta of %s but now ties exactly; remove the declaration",
				c+1, col.FundGroup, col.FiscalYear, want))
		default:
			bad = append(bad, fmt.Sprintf("column %d (%s FY%d): mapped %s, document states %s, declared delta %s but the difference is %s",
				c+1, col.FundGroup, col.FiscalYear, sums[c], stated[c], want, diff))
		}
	}
	// A declaration that has stopped doing anything is the one shape a
	// declaration in this repository must not have, and this is the exact
	// mirror of the stale-delta arm above: there, a delta the document no
	// longer needs; here, a tolerance no column needed. Both fail rather than
	// pass quietly, because a tolerance nobody notices is how a global epsilon
	// arrives one rule at a time.
	if len(bad) == 0 && rule.PrintedDecimals != nil && res.Tolerated == 0 {
		return nil, cmdutil.WithHint(&resolveError{DocID: r.file.DocID, RuleID: rule.ID, Page: p.Page,
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
	return nil, cmdutil.WithHint(&resolveError{DocID: r.file.DocID, RuleID: rule.ID, Page: p.Page,
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
// base. Splitting on whitespace rather than scanning for a number pattern is
// deliberate: a parenthesised negative, a bled currency symbol and a dash-zero
// are all single tokens, and a pattern scan would silently pick the digits out
// of a token it did not understand.
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
// so a row printed as "$ 1,234  $ 5,678" reads as two figures rather than four
// tokens the amount grammar cannot parse. 236 of the corpus's 786 pages print
// the dollar sign detached from its figure. A positional read drops them too:
// Budget Book p81 and pp.225-235 print a "$" before each figure of a block's
// first row and of its totals.
//
// It DROPS the mark rather than joining it to the figure, and that is the whole
// design. A joined token would be a string this project synthesized: Value
// carries the token text and its offset straight through to the published fact,
// and fisc verify's fact-offset-points-at-token asserts
// text[Offset:Offset+len(Token)] == Token against the page. The page bytes at a
// detached mark are "$      1,2", so a synthesized "$1,234" would fail that
// check on every row it touched. The figure's own token is already exactly what
// the document printed and already points at itself.
//
// A mark with no figure after it is an error, not a silent drop. That is the
// case where the read has run off the end of the row, which is the failure the
// column guard exists to catch and must not be laundered into a shorter token
// list.
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
// digits. The set is closed on purpose: amount.Parse rejects a bare "$" and
// must keep doing so, so this is the one place that knows the mark can stand
// alone, and it recognises only the mark itself rather than any token the
// amount grammar happens to reject.
func isCurrencyMark(s string) bool { return s == "$" }

// firstDifferingColumn is the index where two column runs disagree, or -1, and
// a second value saying whether the two were comparable at all.
//
// It reports the INDEX rather than a bool so the error can name the column that
// differs. Comparing whole Column values is deliberate: every field of it
// changes what a figure in that position MEANS, so there is no subset worth
// exempting.
//
// THE SECOND RETURN IS WHY THIS SIGNATURE CHANGED (fisc-oz4). The length branch
// used to return min(len(a), len(b)), and the only caller indexes BOTH slices at
// the returned value to build its message -- so a=3 columns against b=5 returned
// 3 and panicked on a[3]. A branch added to make the helper safe for its next
// caller crashed instead.
//
// The bead preferred returning -1 there and leaving the width to the caller.
// That is NOT what this does, and the reason is the comment the branch replaced:
// -1 means "no differing column", which is the FALSE AGREEMENT the branch
// existed to prevent -- a run that is a prefix of a longer one would report
// agreement. Choosing between crashing and lying is not the choice; a caller
// that cannot express "these are not comparable" is the defect. So the state is
// returned instead of encoded in a sentinel, and a caller must handle it before
// it can index anything.
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

// effectiveColumns is a rule's bearer-part columns with the basis each figure
// is actually published on filled in, and the kind on a column that carries
// the category.
//
// Column.Basis and Column.Kind are overrides and usually empty; the ones in
// force are the rule's. Comparing the declarations rather than the effective values is what
// let two rules on different bases read as the same columns.
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

// columnIdentity renders a column as the thing it IDENTIFIES, which is not what
// describeColumn renders.
//
// The two answer different questions and neither generalises. describeColumn
// answers "which column is this token in", a positional question where basis and
// fund are noise and a skipped column has no identity worth printing. This
// answers "are these two the same column", where every field is discriminating
// -- including Skip, since a skipped column and a read one in the same position
// are precisely not the same column.
//
// Fund and basis appear only when set, because a schedule with neither would
// otherwise report "fund 0" on every column and bury the fields that do differ.
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
