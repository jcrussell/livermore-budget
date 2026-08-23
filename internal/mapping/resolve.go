package mapping

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"unicode"

	"github.com/jcrussell/livermore-budget/internal/amount"
	"github.com/jcrussell/livermore-budget/internal/geom"
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

// ResolveError locates a resolution failure in both the rule and the document,
// because either one can be what is wrong.
type ResolveError struct {
	DocID  string
	RuleID string
	Page   int
	Field  string
	Msg    string
	Err    error
}

func (e *ResolveError) Error() string {
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

func (e *ResolveError) Unwrap() error { return e.Err }

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
	values    []Value
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

// Block is a resolved span of page text: the rows one part covers, bounded by
// the part's section and stop_at anchors.
type Block struct {
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
	// part's active rows: an Omission has no active-row position at all, and
	// two indexing bases would collide the moment a part omitted a row from
	// anywhere but the end.
	RowIndex, ColumnIndex int
	Page                  int
	// Offset is the byte offset of the token within the page text, and Token
	// is the text that was parsed. Both exist so a provenance link can point
	// at the figure rather than at the page.
	Offset int
	Token  string
}

// Omission is a row a part does not print, declared by the rule.
//
// Whether these become facts (as zeros) is deliberately not decided here. The
// argument for it is conditional: with one declared omission and a column total
// that ties, the tie proves the missing row is zero — but with two omissions on
// one part it proves nothing, and "absent is not zero" is a project invariant.
// The fact model decides; resolution only reports what was declared.
type Omission struct {
	Row      Row
	RowIndex int
	Page     int
}

// Block resolves the span of page text a part covers.
func (r *Resolver) Block(rule *Rule, p *Part) (*Block, error) {
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
	if p.StopAt != "" {
		i := strings.Index(text[start:], p.StopAt)
		if i < 0 {
			return nil, &ResolveError{DocID: r.file.DocID, RuleID: rule.ID, Page: p.Page,
				Field: "stop_at", Err: ErrNotFound,
				Msg: fmt.Sprintf("%q does not occur after the section anchor", p.StopAt)}
		}
		end = start + i
	}
	return &Block{Page: p.Page, Start: start, End: end, Text: text[start:end]}, nil
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

	fail := func(err error, msg, hint string) (int, error) {
		return 0, cmdutil.WithHint(&ResolveError{DocID: r.file.DocID, RuleID: rule.ID,
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
		return fail(ErrNotFound, fmt.Sprintf("%q does not occur on the page", needle),
			"the page may have been re-extracted or the document revised; "+
				"check the page text under data/extracted/")
	case ordinal == 0 && len(at) > 1:
		return fail(ErrAmbiguous,
			fmt.Sprintf("%q occurs %d times on the page: %s", needle, len(at),
				describeAt(text, at)),
			fmt.Sprintf("set section_ordinal to say which one starts the block "+
				"(1 to %d)", len(at)))
	case ordinal > len(at):
		return fail(ErrNotFound,
			fmt.Sprintf("occurrence %d of %q was requested but it occurs %d times: %s",
				ordinal, needle, len(at), describeAt(text, at)),
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
// rows the rule declared this part omits.
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
// changing what the next caller sees. Value and Omission carry no slices of
// their own — every field is a scalar or a Row or Column, which are scalars
// throughout — so a shallow copy is a whole one, the same argument
// Rule.ActiveRows makes.
func (r *Resolver) Values(rule *Rule, p *Part) ([]Value, []Omission, error) {
	rp := r.resolvePart(rule, p)
	if rp.err != nil {
		return nil, nil, rp.err
	}
	return slices.Clone(rp.values), slices.Clone(rp.omissions), nil
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
	rp.values, rp.omissions, rp.err = r.readPart(rule, p)
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
	blk, err := r.Block(rule, p)
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
	return values, omissions(rule, p), nil
}

// canonicalRows returns the part's active rows paired with each one's index in
// the rule's row list. Values and Omissions both index that list, so a caller
// can lay them out together.
func canonicalRows(rule *Rule, p *Part) ([]Row, []int) {
	active := rule.ActiveRows(p)
	idx := make([]int, len(active))
	at := 0
	for i, row := range rule.Rows {
		// Identity(), like ActiveRows: two rows may share a Label, and pairing
		// on it would give the surviving row the omitted row's index, filing
		// its figures under a position the page does not print.
		if at < len(active) && active[at].Identity() == row.Identity() {
			idx[at] = i
			at++
		}
	}
	return active, idx
}

func omissions(rule *Rule, p *Part) []Omission {
	if len(p.OmittedRows) == 0 {
		return nil
	}
	// Keyed on Identity(), the same key ActiveRows drops on. Matching the bare
	// Label here would report an omission for every row sharing it, so the
	// omissions and the active rows would disagree about which rows the page
	// prints (fisc-gtv).
	declared := omittedSet(p)
	var out []Omission
	for i, row := range rule.Rows {
		if declared[row.Identity()] {
			out = append(out, Omission{Row: row, RowIndex: i, Page: p.Page})
		}
	}
	return out
}

func (r *Resolver) labelledValues(rule *Rule, p *Part, blk *Block, guard *columnGuard) ([]Value, error) {
	rows, rowIndex := canonicalRows(rule, p)
	ncols := len(p.Columns)
	fail := func(field, msg, hint string) error {
		return cmdutil.WithHint(&ResolveError{DocID: r.file.DocID, RuleID: rule.ID,
			Page: p.Page, Field: field, Msg: msg, Err: ErrNotFound}, hint)
	}

	values := make([]Value, 0, len(rows)*ncols)
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
		if err := r.checkGap(rule, p, gap, rows, i, used); err != nil {
			return nil, err
		}

		after := cursor + j + len(row.Label)
		if row.LabelTail != "" {
			k := strings.Index(blk.Text[after:], row.LabelTail)
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
			after += k + len(row.LabelTail)
		}
		toks, err := dropCurrencyMarks(tokens(blk.Text[after:], blk.Start+after))
		if err != nil {
			return nil, fail("rows", fmt.Sprintf("row %q: %s", row.PrintedLabel(), err), currencyHint)
		}
		if len(toks) < ncols {
			return nil, fail("rows", fmt.Sprintf(
				"row %q is followed by %d values, want %d (one per column)",
				row.PrintedLabel(), len(toks), ncols), "check the part's columns against the page")
		}
		toks = toks[:ncols]
		vals, err := r.parseRow(rule, p, row, rowIndex[i], toks, guard)
		if err != nil {
			return nil, err
		}
		values = append(values, vals...)
		last := toks[ncols-1]
		cursor = last.off - blk.Start + len(last.text)
	}

	// Anything after the last row's figures is a row the rule did not map --
	// unless the page wrapped a label there, which is the same shape as a gap
	// between two rows and is declared the same way.
	if rest := strings.TrimSpace(blk.Text[cursor:]); rest != "" {
		if !slices.Contains(p.WrappedLabels, rest) {
			return nil, fail("rows", fmt.Sprintf(
				"%q follows the last mapped row but is not mapped", rest),
				"every row inside the block must be listed in rows, with skip: true "+
					"if it should not produce facts, or in wrapped_labels if the "+
					"page wrapped a label onto its own line")
		}
		used[rest] = true
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
	return values, nil
}

// checkGap enforces what may sit between one row's last figure and the next
// row's label. Before the first row the block may carry column headers, which
// are words; between rows nothing at all may intervene, because anything that
// does is a row the rule has not mapped.
func (r *Resolver) checkGap(rule *Rule, p *Part, gap string, rows []Row, i int,
	used map[string]bool) error {
	if i == 0 {
		if strings.ContainsFunc(gap, unicode.IsDigit) {
			return cmdutil.WithHint(&ResolveError{DocID: r.file.DocID, RuleID: rule.ID,
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
	if slices.Contains(p.WrappedLabels, trimmed) {
		used[trimmed] = true
		return nil
	}
	return cmdutil.WithHint(&ResolveError{DocID: r.file.DocID, RuleID: rule.ID,
		Page: p.Page, Field: "rows", Err: ErrNotFound,
		Msg: fmt.Sprintf("%q sits between rows %q and %q but is not mapped",
			trimmed, rows[i-1].PrintedLabel(), rows[i].PrintedLabel())},
		"add it to rows, with skip: true if it should not produce facts, or to "+
			"wrapped_labels if the page wrapped a label onto its own line; "+
			"leaving it out would publish a breakdown that does not add up")
}

func precedingRow(rows []Row, i int) string {
	if i == 0 {
		return "the section anchor"
	}
	return fmt.Sprintf("row %q", rows[i-1].Label)
}

func (r *Resolver) parseRow(rule *Rule, p *Part, row Row, rowIndex int, toks []token,
	guard *columnGuard) ([]Value, error) {
	// The column check happens here rather than in the two callers because this
	// is the one place that already has a row, its tokens and the columns they
	// were filed under together. Checking in both callers would be two copies of
	// the invariant the whole guard exists for.
	if guard != nil {
		if err := guard.checkRow(r, rule, p, row, toks); err != nil {
			return nil, err
		}
	}
	out := make([]Value, 0, len(toks))
	for c, tk := range toks {
		col := p.Columns[c]
		cents, err := amount.Parse(tk.text, rule.Units)
		if err != nil {
			return nil, &ResolveError{DocID: r.file.DocID, RuleID: rule.ID, Page: p.Page,
				Field: fmt.Sprintf("row %q column %d", row.PrintedLabel(), c+1),
				Msg:   err.Error(), Err: err}
		}
		// A skipped row or column still consumes its position — that is the
		// point of skip — but yields no fact.
		if row.Skip || col.Skip {
			continue
		}
		out = append(out, Value{Cents: cents, Row: row, Column: col,
			RowIndex: rowIndex, ColumnIndex: c, Page: p.Page,
			Offset: tk.off, Token: tk.text})
	}
	return out, nil
}

func (r *Resolver) positionalValues(rule *Rule, p *Part, blk *Block, guard *columnGuard) ([]Value, error) {
	rows, rowIndex := canonicalRows(rule, p)
	ncols := len(p.Columns)
	toks := tokens(blk.Text, blk.Start)
	want := rule.ExpectedValues(p)

	if len(toks) != want {
		return nil, cmdutil.WithHint(&ResolveError{DocID: r.file.DocID, RuleID: rule.ID,
			Page: p.Page, Field: "parts", Err: ErrNotFound,
			Msg: fmt.Sprintf("read %d values, want %d (%d rows × %d columns); "+
				"rows declared absent from this page: %s",
				len(toks), want, len(rows), ncols, declaredOmissions(p))},
			"a label-less page is read positionally, so a count that does not "+
				"match means every row after the gap would be mismapped; add or "+
				"remove an omitted_rows entry only after checking the page")
	}

	values := make([]Value, 0, want)
	for i, row := range rows {
		rowToks := toks[i*ncols : (i+1)*ncols]
		vals, err := r.parseRow(rule, p, row, rowIndex[i], rowToks, guard)
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

func declaredOmissions(p *Part) string {
	if len(p.OmittedRows) == 0 {
		return "none"
	}
	labels := make([]string, len(p.OmittedRows))
	for i, o := range p.OmittedRows {
		labels[i] = o.PrintedLabel()
	}
	return fmt.Sprintf("%q", labels)
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
	blk, err := r.Block(rule, p)
	if err != nil {
		return nil, err
	}
	text, err := r.page(p.Page)
	if err != nil {
		return nil, err
	}

	from := blk.End
	field := "stop_at"
	if p.LabelsFrom == 0 {
		if rule.TotalRow == "" {
			return nil, &ResolveError{DocID: r.file.DocID, RuleID: rule.ID, Page: p.Page,
				Field: "total_row", Msg: "the rule declares none", Err: ErrNoStatedTotals}
		}
		field = "total_row"
		i := strings.Index(text[blk.End:], rule.TotalRow)
		if i < 0 {
			return nil, &ResolveError{DocID: r.file.DocID, RuleID: rule.ID, Page: p.Page,
				Field: field, Err: ErrNotFound,
				Msg: fmt.Sprintf("%q does not occur after the block", rule.TotalRow)}
		}
		from = blk.End + i + len(rule.TotalRow)
	} else if p.StopAt == "" {
		return nil, &ResolveError{DocID: r.file.DocID, RuleID: rule.ID, Page: p.Page,
			Field: "stop_at", Err: ErrNoStatedTotals,
			Msg: "a label-less part needs a stop_at anchor to find its totals"}
	}

	line := text[from:]
	if i := strings.IndexByte(line, '\n'); i >= 0 {
		line = line[:i]
	}
	totals, ok := amountRun(line, len(p.Columns), rule.Units)
	if !ok {
		return nil, cmdutil.WithHint(&ResolveError{DocID: r.file.DocID, RuleID: rule.ID,
			Page: p.Page, Field: field, Err: ErrNotFound,
			Msg: fmt.Sprintf("no run of %d consecutive amounts follows the anchor on %q",
				len(p.Columns), strings.TrimSpace(line))},
			"the totals row must print one figure per column; if the page prints "+
				"them twice on one line, anchor past the first copy")
	}
	return totals, nil
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
func amountRun(s string, n int, u amount.Units) ([]amount.Cents, bool) {
	toks := tokens(s, 0)
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

// TotalsResult is what checking one part's totals established.
type TotalsResult struct {
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
func (r *Resolver) CheckTotals(rule *Rule, p *Part) (*TotalsResult, error) {
	stated, err := r.StatedTotals(rule, p)
	if err != nil {
		return nil, err
	}
	values, _, err := r.Values(rule, p)
	if err != nil {
		return nil, err
	}

	sums := make([]amount.Cents, len(p.Columns))
	for _, v := range values {
		if !rule.totalCovers(v.Row.EffectiveKind(rule)) {
			continue
		}
		sums[v.ColumnIndex] += v.Cents
	}
	return r.compareTotals(rule, p, p, stated, sums)
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
func (r *Resolver) CheckSpanningTotals(rule *Rule) (*TotalsResult, error) {
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
			&ResolveError{DocID: r.file.DocID, RuleID: rule.ID, Page: p.Page,
				Field: "stated_total_deltas", Err: ErrNotFound,
				Msg: fmt.Sprintf("declared here, but page %d is the page that prints %q",
					bearer.Page, rule.TotalRow)},
			"a declared delta describes the difference between a printed total "+
				"and the rows beneath it, so it is declared where that total is printed")
	}

	sums := make([]amount.Cents, len(bearer.Columns))
	for i := range rule.Parts {
		p := &rule.Parts[i]
		values, _, err := r.Values(rule, p)
		if err != nil {
			return nil, err
		}
		for _, v := range values {
			if !rule.totalCovers(v.Row.EffectiveKind(rule)) {
				continue
			}
			sums[v.ColumnIndex] += v.Cents
		}
	}
	return r.compareTotals(rule, bearer, bearer, stated, sums)
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
		blk, err := r.Block(rule, p)
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
			&ResolveError{DocID: r.file.DocID, RuleID: rule.ID, Page: rule.Parts[0].Page,
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
			&ResolveError{DocID: r.file.DocID, RuleID: rule.ID, Page: found[0].Page,
				Field: "total_row", Err: ErrNotFound,
				Msg: fmt.Sprintf("%q occurs after the block on pages %v; it must identify one",
					rule.TotalRow, pages)},
			"lengthen the anchor until it names the one page that prints this "+
				"block's total")
	}
}

// RollupResult is what checking one cross-rule rollup established.
type RollupResult struct {
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
func (r *Resolver) CheckRollup(ro *Rollup) (*RollupResult, error) {
	rules, err := r.coveredRules(ro)
	if err != nil {
		return nil, err
	}

	// The columns come from the part that PRINTS each rule's total, not from
	// its first part. A rule may declare its columns per part, so a stated
	// total read off part 2 is stated over part 2's columns; taking the width
	// from Parts[0] and indexing with it read off the end of the sum. The
	// parser's own guard compares Parts[0] for the same reason and cannot
	// close this, because which part bears the total needs the pages.
	var cols []Column
	var sums []amount.Cents
	for _, rule := range rules {
		stated, bearer, err := r.ruleStatedTotals(rule)
		if err != nil {
			return nil, err
		}
		if cols == nil {
			cols, sums = bearer.Columns, make([]amount.Cents, len(stated))
		}
		if len(stated) != len(sums) {
			return nil, cmdutil.WithHint(
				&ResolveError{DocID: r.file.DocID, Page: ro.Page, RuleID: ro.ID,
					Field: "covers", Err: ErrNotFound,
					Msg: fmt.Sprintf("rule %q states %d columns and rule %q states %d",
						rules[0].ID, len(sums), rule.ID, len(stated))},
				"a rollup adds these totals column by column, so every covered "+
					"rule must state the same number of them")
		}
		for i, c := range stated {
			sums[i] += c
		}
	}

	stated, err := r.rollupStatedTotals(ro, len(cols), rules[0].Units)
	if err != nil {
		return nil, err
	}

	res := &RollupResult{Rules: len(rules)}
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
				i+1, col.FundGroup, col.FiscalYear, len(rules), sums[i], stated[i], sums[i]-stated[i]))
		}
	}
	if len(bad) == 0 {
		return res, nil
	}
	return nil, cmdutil.WithHint(
		&ResolveError{DocID: r.file.DocID, Page: ro.Page, RuleID: ro.ID,
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
			return nil, &ResolveError{DocID: r.file.DocID, Page: ro.Page, RuleID: ro.ID,
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
			&ResolveError{DocID: r.file.DocID, RuleID: rule.ID, Page: rule.Parts[0].Page,
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
func (r *Resolver) rollupStatedTotals(ro *Rollup, n int, units amount.Units) ([]amount.Cents, error) {
	text, err := r.page(ro.Page)
	if err != nil {
		return nil, err
	}
	first := strings.Index(text, ro.TotalRow)
	if first < 0 {
		return nil, &ResolveError{DocID: r.file.DocID, Page: ro.Page, RuleID: ro.ID,
			Field: "total_row", Err: ErrNotFound,
			Msg: fmt.Sprintf("%q does not occur on the page", ro.TotalRow)}
	}
	if strings.Contains(text[first+len(ro.TotalRow):], ro.TotalRow) {
		return nil, cmdutil.WithHint(
			&ResolveError{DocID: r.file.DocID, Page: ro.Page, RuleID: ro.ID,
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
		return nil, cmdutil.WithHint(
			&ResolveError{DocID: r.file.DocID, Page: ro.Page, RuleID: ro.ID,
				Field: "total_row", Err: ErrNotFound,
				Msg: fmt.Sprintf("no run of %d consecutive amounts follows %q on %q",
					n, ro.TotalRow, strings.TrimSpace(line))},
			"the rollup's figures must be on the same printed line as its "+
				"anchor; where the label wraps, anchor on the part that "+
				"carries the figures")
	}
	return totals, nil
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
func (r *Resolver) compareTotals(rule *Rule, cols, dec *Part, stated, sums []amount.Cents) (*TotalsResult, error) {
	p := cols

	// Declared discrepancies, by 1-based column. The parser has already
	// refused a duplicate, an out-of-range column, a skipped column, a zero
	// delta and a missing note, so nothing here needs to re-check any of that.
	declared := make(map[int]amount.Cents, len(dec.StatedTotalDeltas))
	for _, d := range dec.StatedTotalDeltas {
		declared[d.Column] = d.Cents
	}

	res := &TotalsResult{}
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
		switch {
		case !isDeclared:
			bad = append(bad, fmt.Sprintf("column %d (%s FY%d): mapped %s, document states %s, off by %s",
				c+1, col.FundGroup, col.FiscalYear, sums[c], stated[c], sums[c]-stated[c]))
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
	if len(bad) == 0 {
		return res, nil
	}
	return nil, cmdutil.WithHint(&ResolveError{DocID: r.file.DocID, RuleID: rule.ID, Page: p.Page,
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
// the dollar sign detached from its figure; pp.66-67 print none, which is why
// turning this on cannot move a byte of facts.jsonl.
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
