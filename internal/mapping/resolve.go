package mapping

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"unicode"

	"github.com/jcrussell/livermore-budget/internal/amount"
	"github.com/jcrussell/livermore-budget/pkg/cmdutil"
)

// doc is the view of an extracted document a resolver needs. It is declared
// here, in the consumer, and kept to the two methods actually used
// (byob-interfaces.1, byob-interfaces.2). Keeping it to these two is why this
// package does not import internal/corpus at all: rules are resolved against
// page text and a document identity, and nothing else.
type doc interface {
	DocID() string
	Page(n int) (string, error)
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

	// pages caches page text. A rule file resolves the same handful of pages
	// many times over — every part reads its page, and checking a part's
	// totals reads it again — and re-decoding a page per call turned one
	// CheckTotals into four reads of the same file.
	mu    sync.Mutex
	pages map[int]string
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
	return &Resolver{doc: d, file: f, pages: map[int]string{}}, nil
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
func (r *Resolver) Values(rule *Rule, p *Part) ([]Value, []Omission, error) {
	blk, err := r.Block(rule, p)
	if err != nil {
		return nil, nil, err
	}

	var values []Value
	if p.LabelsFrom == 0 {
		values, err = r.labelledValues(rule, p, blk)
	} else {
		values, err = r.positionalValues(rule, p, blk)
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
		if at < len(active) && active[at].Label == row.Label {
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
	declared := make(map[string]bool, len(p.OmittedRows))
	for _, l := range p.OmittedRows {
		declared[l] = true
	}
	var out []Omission
	for i, row := range rule.Rows {
		if declared[row.Label] {
			out = append(out, Omission{Row: row, RowIndex: i, Page: p.Page})
		}
	}
	return out
}

func (r *Resolver) labelledValues(rule *Rule, p *Part, blk *Block) ([]Value, error) {
	rows, rowIndex := canonicalRows(rule, p)
	ncols := len(p.Columns)
	fail := func(field, msg, hint string) error {
		return cmdutil.WithHint(&ResolveError{DocID: r.file.DocID, RuleID: rule.ID,
			Page: p.Page, Field: field, Msg: msg, Err: ErrNotFound}, hint)
	}

	values := make([]Value, 0, len(rows)*ncols)
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
		if err := r.checkGap(rule, p, gap, rows, i); err != nil {
			return nil, err
		}

		after := cursor + j + len(row.Label)
		toks := tokens(blk.Text[after:], blk.Start+after)
		if len(toks) < ncols {
			return nil, fail("rows", fmt.Sprintf(
				"row %q is followed by %d values, want %d (one per column)",
				row.Label, len(toks), ncols), "check the part's columns against the page")
		}
		toks = toks[:ncols]
		vals, err := r.parseRow(rule, p, row, rowIndex[i], toks)
		if err != nil {
			return nil, err
		}
		values = append(values, vals...)
		last := toks[ncols-1]
		cursor = last.off - blk.Start + len(last.text)
	}

	// Anything after the last row's figures is a row the rule did not map.
	if rest := blk.Text[cursor:]; strings.TrimSpace(rest) != "" {
		return nil, fail("rows", fmt.Sprintf(
			"%q follows the last mapped row but is not mapped", strings.TrimSpace(rest)),
			"every row inside the block must be listed in rows, with skip: true "+
				"if it should not produce facts")
	}
	return values, nil
}

// checkGap enforces what may sit between one row's last figure and the next
// row's label. Before the first row the block may carry column headers, which
// are words; between rows nothing at all may intervene, because anything that
// does is a row the rule has not mapped.
func (r *Resolver) checkGap(rule *Rule, p *Part, gap string, rows []Row, i int) error {
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
	if strings.TrimSpace(gap) == "" {
		return nil
	}
	return cmdutil.WithHint(&ResolveError{DocID: r.file.DocID, RuleID: rule.ID,
		Page: p.Page, Field: "rows", Err: ErrNotFound,
		Msg: fmt.Sprintf("%q sits between rows %q and %q but is not mapped",
			strings.TrimSpace(gap), rows[i-1].Label, rows[i].Label)},
		"add it to rows, with skip: true if it should not produce facts; "+
			"leaving it out would publish a breakdown that does not add up")
}

func precedingRow(rows []Row, i int) string {
	if i == 0 {
		return "the section anchor"
	}
	return fmt.Sprintf("row %q", rows[i-1].Label)
}

func (r *Resolver) parseRow(rule *Rule, p *Part, row Row, rowIndex int, toks []token) ([]Value, error) {
	out := make([]Value, 0, len(toks))
	for c, tk := range toks {
		col := p.Columns[c]
		cents, err := amount.Parse(tk.text, rule.Units)
		if err != nil {
			return nil, &ResolveError{DocID: r.file.DocID, RuleID: rule.ID, Page: p.Page,
				Field: fmt.Sprintf("row %q column %d", row.Label, c+1),
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

func (r *Resolver) positionalValues(rule *Rule, p *Part, blk *Block) ([]Value, error) {
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
		vals, err := r.parseRow(rule, p, row, rowIndex[i], toks[i*ncols:(i+1)*ncols])
		if err != nil {
			return nil, err
		}
		values = append(values, vals...)
	}
	return values, nil
}

func declaredOmissions(p *Part) string {
	if len(p.OmittedRows) == 0 {
		return "none"
	}
	return fmt.Sprintf("%q", p.OmittedRows)
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

// CheckTotals asserts that the figures a part yields sum, per column, to the
// totals the document prints for it.
//
// This is the most valuable check available, because it is the document
// checking our work rather than us checking our own. It returns
// ErrNoStatedTotals where the document prints no total, which is not a pass:
// callers are expected to report those parts as unchecked rather than silent.
func (r *Resolver) CheckTotals(rule *Rule, p *Part) error {
	stated, err := r.StatedTotals(rule, p)
	if err != nil {
		return err
	}
	values, _, err := r.Values(rule, p)
	if err != nil {
		return err
	}

	sums := make([]amount.Cents, len(p.Columns))
	for _, v := range values {
		sums[v.ColumnIndex] += v.Cents
	}

	var bad []string
	for c, col := range p.Columns {
		if col.Skip {
			continue
		}
		if sums[c] != stated[c] {
			bad = append(bad, fmt.Sprintf("column %d (%s FY%d): mapped %s, document states %s, off by %s",
				c+1, col.FundGroup, col.FiscalYear, sums[c], stated[c], sums[c]-stated[c]))
		}
	}
	if len(bad) == 0 {
		return nil
	}
	return cmdutil.WithHint(&ResolveError{DocID: r.file.DocID, RuleID: rule.ID, Page: p.Page,
		Field: "total_row", Msg: strings.Join(bad, "; ")},
		"a mapped column that does not tie means a row was missed, "+
			"double-counted, or read from the wrong column")
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
