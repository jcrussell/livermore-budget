package mapping

import (
	"fmt"
	"slices"
	"strings"

	"github.com/jcrussell/livermore-budget/internal/geom"
	"github.com/jcrussell/livermore-budget/pkg/cmdutil"
)

// This file is the column-position guard: the second witness that says a figure
// really is printed in the column the rule filed it under.
//
// It never SELECTS which tokens a row consumes. The -layout read is unchanged
// and remains the only source of a value, its offset and its token, because
// those three are published provenance and `fisc verify` re-derives them against
// the page text. Geometry supplies a column index and nothing else, so turning
// the guard on cannot move a byte of facts.jsonl -- it can only refuse.

// placed is one -layout token's geometry: the word it pairs with, and which
// printed line the two of them sit on.
type placed struct {
	word geom.Word
	// line indexes the page's non-blank lines, which is what makes "these two
	// tokens are on the same line" a comparison.
	line int
	// num is that line's number in the page text, which is what a person needs
	// in a message. The two differ by every blank line above: p67 has 15 blank
	// lines among its 48, so its tenth non-blank line is line 23.
	num int
}

// pairing is one page's two substrates reconciled against each other.
//
// The line index is carried per token and not derived from the word's y0,
// because words on one printed line do NOT share a y0 -- the worst spread
// measured on the mapped pages is 0.96pt, which is the entire reason the line
// grouping has a tolerance. Comparing coordinates here would reject legitimate
// rows.
type pairing struct {
	// words is keyed by absolute byte offset into the page text, which is the
	// offset tokens() already records and Value already publishes.
	words map[int]placed
	// lines is the geometry's own view of the page, for the header search.
	lines []geom.Line
	// nums gives each of those lines its number in the page text.
	nums []int
}

// buildPairing reconciles the page text with the word geometry, token by token.
//
// Both substrates describe the same printed page, so they must agree about it.
// Where they do, each -layout token has exactly one geometry word and the pairing
// is an offset lookup; where they do not, this fails and the page cannot carry
// the guard at all. That is a real limit rather than a theoretical one: measured
// at b62a6c8, on 124 of the corpus's 786 pages the two disagree about how many
// lines the page has, and on 99 more they disagree about the tokens on a line.
//
// ONE OF THEM IS MAPPED: ACFR p41, which has 52 non-blank text lines against 51
// geometry lines. It declares no column_headers, so this never runs for it. The
// claim worth checking is therefore not that every mapped page pairs -- one does
// not -- but that every part ASKING for the guard can have it, which
// TestEveryMappedPartWithHeadersCanCarryTheColumnGuard pins over the committed
// corpus. Every page that fails refuses here rather than being read
// approximately.
func buildPairing(text string, g *geom.Page) (*pairing, error) {
	lines := g.Lines()

	type textLine struct {
		body string
		base int
		num  int
	}
	var printed []textLine
	base := 0
	for i, body := range strings.Split(text, "\n") {
		if strings.TrimSpace(body) != "" {
			printed = append(printed, textLine{body: body, base: base, num: i + 1})
		}
		base += len(body) + 1 // the newline strings.Split consumed
	}

	if len(printed) != len(lines) {
		return nil, fmt.Errorf(
			"the page text has %d non-blank lines but the geometry has %d; "+
				"the two substrates do not describe the same page",
			len(printed), len(lines))
	}

	words := make(map[int]placed, len(g.Words))
	for i, pl := range printed {
		toks := tokens(pl.body, pl.base)
		got := lines[i].Words
		if len(toks) != len(got) {
			return nil, fmt.Errorf(
				"line %d has %d tokens in the page text but %d words in the geometry: %s",
				pl.num, len(toks), len(got), disagreement(toks, got))
		}
		for j, tk := range toks {
			if tk.text != got[j].Text {
				return nil, fmt.Errorf(
					"line %d token %d is %q in the page text but %q in the geometry",
					pl.num, j+1, tk.text, got[j].Text)
			}
			words[tk.off] = placed{word: got[j], line: i, num: pl.num}
		}
	}
	nums := make([]int, len(printed))
	for i, pl := range printed {
		nums[i] = pl.num
	}
	return &pairing{words: words, lines: lines, nums: nums}, nil
}

// disagreement renders both substrates' view of one line, which is what makes a
// substrate disagreement diagnosable without opening two files.
func disagreement(toks []token, words []geom.Word) string {
	text := make([]string, len(toks))
	for i, tk := range toks {
		text[i] = tk.text
	}
	geo := make([]string, len(words))
	for i, w := range words {
		geo[i] = w.Text
	}
	return fmt.Sprintf("page text %q, geometry %q", strings.Join(text, " "),
		strings.Join(geo, " "))
}

// pairing returns the reconciled substrates for page n, doing the work at most
// once.
//
// A failure is NOT cached, which follows page rather than resolvedPart: this
// reads two artifacts off a filesystem, so it is I/O-shaped, and the asymmetry
// between the two existing memos is drawn on exactly that line. The lock is not
// held across the work, because building it calls page, which takes the same
// non-reentrant mutex.
func (r *Resolver) pairing(n int) (*pairing, error) {
	r.mu.Lock()
	pr, ok := r.pairings[n]
	r.mu.Unlock()
	if ok {
		return pr, nil
	}

	text, err := r.page(n)
	if err != nil {
		return nil, err
	}
	g, err := r.doc.Geometry(n)
	if err != nil {
		return nil, err
	}
	built, err := buildPairing(text, g)
	if err != nil {
		return nil, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if first, ok := r.pairings[n]; ok {
		return first, nil
	}
	r.pairings[n] = built
	return built, nil
}

// columnGuard is the geometry cross-check for one part. It is nil for a part
// that declares no column_headers, which reads exactly as it did before geometry
// existed.
type columnGuard struct {
	grid *geom.Grid
	pair *pairing
	// band maps a column's index to its index in grid, and -1 for a column the
	// page prints no header over -- which therefore has no band and can carry
	// no placement claim. Only the last column may be unbanded (see
	// ColumnHeader), so this is in practice the identity with at most a
	// trailing -1; it is a map rather than a count so that a mistake in the
	// parse rule cannot silently shift every band by one.
	band []int
}

// guard builds the column guard for a part, or returns nil if the part did not
// ask for one.
func (r *Resolver) guard(rule *Rule, p *Part) (*columnGuard, error) {
	if len(p.ColumnHeaders) == 0 {
		return nil, nil
	}
	fail := func(err error, msg, hint string) (*columnGuard, error) {
		return nil, cmdutil.WithHint(&resolveError{DocID: r.file.DocID, RuleID: rule.ID,
			Page: p.Page, Field: "column_headers", Msg: msg, Err: err}, hint)
	}

	pr, err := r.pairing(p.Page)
	if err != nil {
		return nil, cmdutil.WithHint(&resolveError{DocID: r.file.DocID, RuleID: rule.ID,
			Page: p.Page, Field: "geometry", Msg: err.Error(), Err: err},
			"this part declares column_headers, so it is read against "+
				"geometry/pNNNN.json as well as the page text; a page whose two "+
				"substrates disagree cannot carry the column guard")
	}

	// The header line is searched over the WHOLE page, not within the block.
	// Three of the spine's p66 parts anchor after "EXPENDITURES:", which is
	// below the header line, and p67's own section anchor is the end of that
	// line. The grid is a property of the page; the rule only says which page
	// and which columns.
	// A headerless column is matched against nothing, because the page prints
	// nothing to match. The band map below is what keeps the remaining headers
	// aligned with the columns they belong to.
	headed := make([]string, 0, len(p.ColumnHeaders))
	band := make([]int, len(p.ColumnHeaders))
	for i, h := range p.ColumnHeaders {
		if h.Unheaded {
			band[i] = -1
			continue
		}
		band[i] = len(headed)
		headed = append(headed, h.Text)
	}

	var matched []headerMatch
	var best headerMatch
	for i, line := range pr.lines {
		m := matchHeaders(line, headed)
		m.line = i
		if m.ok {
			matched = append(matched, m)
		} else if m.reached > best.reached {
			best = m
		}
	}

	switch {
	case len(matched) == 0:
		return fail(ErrNotFound, describeUnmatched(headed, best),
			"column_headers names the printed header of every column, left to "+
				"right; check the page text under data/extracted/")
	case len(matched) > 1:
		return fail(ErrAmbiguous,
			fmt.Sprintf("the %d headers match %d lines of the page: %s",
				len(headed), len(matched), describeLines(pr, matched)),
			"there is no ordinal for this key: name more of the printed header "+
				"so the sequence occurs once, and file a bead if a page really "+
				"does print its column headers twice")
	}

	m := matched[0]
	// Anything printed on the header line right of the first column header and
	// not part of a declared header is a column the rule does not know about.
	// In-order matching catches a page that DROPPED a column -- an entry simply
	// stops matching -- but not one that gained a column, where every entry
	// still matches and the new column's figures join the last band.
	if stray, ok := m.strayRightOfFirst(pr.lines[m.line]); ok {
		return fail(ErrNotFound,
			fmt.Sprintf("%q is printed on the header line at x %.2f-%.2f but is "+
				"not one of this part's columns", stray.Text, stray.X0, stray.X1),
			"the page prints a column this part does not declare; add it to "+
				"columns with skip: true, or name it in column_headers")
	}

	// The gutter is the first column header's left edge. Column 0 has no
	// natural left bound and these pages print no header over the row labels,
	// so without one every label would file into the first column.
	grid, err := geom.NewGrid(m.spans, m.spans[0].Lo)
	if err != nil {
		return fail(ErrNotFound, err.Error(),
			"the column headers this part names are not laid out left to right "+
				"on the page as the rule lists them")
	}
	return &columnGuard{grid: grid, pair: pr, band: band}, nil
}

// headerMatch is the result of looking for a part's column headers on one line.
type headerMatch struct {
	line  int
	spans []geom.Span
	// consumed marks the words on the line that a header matched.
	consumed map[int]bool
	// first is the index of the word the first header matched at.
	first int
	// reached is how many headers matched before the search failed, which is
	// what makes an unmatched header diagnosable.
	reached int
	ok      bool
}

// matchHeaders walks a line left to right looking for each header in turn,
// joining consecutive words with one space.
//
// Each header must occur AFTER the one before it, because the list is a claim
// about column ORDER: a page that dropped a column has to fail rather than
// shift every figure one place, and no count can detect that.
func matchHeaders(line geom.Line, headers []string) headerMatch {
	m := headerMatch{consumed: map[int]bool{}, first: -1}
	at := 0
	for _, want := range headers {
		start, end, found := findHeader(line.Words, at, want)
		if !found {
			return m
		}
		if m.first < 0 {
			m.first = start
		}
		for i := start; i <= end; i++ {
			m.consumed[i] = true
		}
		m.spans = append(m.spans, geom.Span{Lo: line.Words[start].X0, Hi: line.Words[end].X1})
		m.reached++
		at = end + 1
	}
	m.ok = true
	return m
}

// findHeader returns the first run of consecutive words at or after `from` whose
// texts joined by one space equal want.
func findHeader(words []geom.Word, from int, want string) (start, end int, ok bool) {
	for i := from; i < len(words); i++ {
		joined := ""
		for j := i; j < len(words); j++ {
			if j > i {
				joined += " "
			}
			joined += words[j].Text
			if joined == want {
				return i, j, true
			}
			if len(joined) >= len(want) {
				break
			}
		}
	}
	return 0, 0, false
}

// strayRightOfFirst returns a word on the header line that sits right of the
// first matched header and belongs to no header.
//
// Words LEFT of the first header are the row-label area's own heading -- CIP p40
// prints "PROJECT NUMBER PROJECT NAME" there -- and are expected.
func (m headerMatch) strayRightOfFirst(line geom.Line) (geom.Word, bool) {
	for i := m.first + 1; i < len(line.Words); i++ {
		if !m.consumed[i] {
			return line.Words[i], true
		}
	}
	return geom.Word{}, false
}

func describeUnmatched(headers []string, best headerMatch) string {
	if best.reached == 0 {
		return fmt.Sprintf("the first column header %q is not printed on any line of the page",
			headers[0])
	}
	return fmt.Sprintf("column header %d (%q) does not occur after header %d (%q) "+
		"on any line of the page",
		best.reached+1, headers[best.reached], best.reached, headers[best.reached-1])
}

func describeLines(pr *pairing, matched []headerMatch) string {
	const max = 3
	var b strings.Builder
	for i, m := range matched {
		if i == max {
			fmt.Fprintf(&b, "; and %d more", len(matched)-max)
			break
		}
		if i > 0 {
			b.WriteString("; ")
		}
		var words []string
		for _, w := range pr.lines[m.line].Words {
			words = append(words, w.Text)
		}
		fmt.Fprintf(&b, "#%d line %d: %q", i+1, pr.nums[m.line], strings.Join(words, " "))
	}
	return b.String()
}

// checkRow is the payoff: every figure this row yields must be printed in the
// band of the column the rule filed it under, and all of them must come off one
// printed line.
//
// The placement check subsumes two others worth naming, so that nobody adds them
// back. A token whose right edge lands left of the gutter is in the row-label
// area, and Index reports -1, which is not any column. Two tokens landing in one
// band fails too, because the second one is not in the band its own column
// claims -- which is what catches a printed figure that extraction split in
// half.
//
// toks[k] is filed under column cols[k]. A column missing from cols is one the
// rule declares blank on this row, and a word printed in its band refuses the
// row: that is the check that a declared blank is blank, and it runs before
// placement so a figure under it is named as such rather than as a figure
// filed one column over.
func (g *columnGuard) checkRow(r *Resolver, rule *Rule, p *Part, row Row, toks []token,
	cols []int) error {
	fail := func(field, msg, hint string) error {
		return cmdutil.WithHint(&resolveError{DocID: r.file.DocID, RuleID: rule.ID,
			Page: p.Page, Field: field, Msg: msg, Err: ErrNotFound}, hint)
	}

	if len(cols) < len(p.Columns) && len(toks) > 0 {
		if pl, ok := g.pair.words[toks[0].off]; ok {
			for _, w := range g.pair.lines[pl.line].Words {
				// A detached "$" belongs to the figure after it, and may sit
				// in the band to that figure's left.
				if isCurrencyMark(w.Text) {
					continue
				}
				c := g.column(g.grid.Index(w.Right()))
				if c >= 0 && !slices.Contains(cols, c) {
					return fail(fmt.Sprintf("row %q column %d", row.PrintedLabel(), c+1),
						fmt.Sprintf("%q is printed under column %d (%s), which the rule "+
							"declares blank on this row", w.Text, c+1, p.ColumnHeaders[c].Text),
						"omitted_cells names a cell the page leaves blank; remove the "+
							"declaration, or move it to the cell that is")
				}
			}
		}
	}

	line, num := -1, 0
	for k, tk := range toks {
		c := cols[k]
		where := fmt.Sprintf("row %q column %d", row.PrintedLabel(), c+1)
		pl, ok := g.pair.words[tk.off]
		if !ok {
			// The substrates were already proved to agree token for token, so
			// the only way to hold an offset no word starts at is to have begun
			// reading inside a printed word -- which means a row label matched
			// part of one.
			return fail(where, fmt.Sprintf(
				"token %q starts inside a printed word rather than at one", tk.text),
				"a row label matched part of a longer word on the page; give "+
					"the label as the page prints it")
		}
		if line < 0 {
			line, num = pl.line, pl.num
		} else if pl.line != line {
			return fail(where, fmt.Sprintf(
				"token %q is printed on line %d but this row's earlier figures are on line %d",
				tk.text, pl.num, num),
				"the row prints fewer figures than the rule declares columns, so "+
					"the read has run on to the next printed line")
		}
		// A column the page prints no header over has no band, so there is no
		// placement claim to make about its token -- only the same-line check
		// above, the row's token count, and checkGap's refusal of anything
		// unexplained between rows. What is given up is bounded by
		// ColumnHeader's rule that only the LAST column may be unbanded: past
		// the last header there is nothing to check against anyway.
		want := g.band[c]
		if want < 0 {
			continue
		}
		if got := g.grid.Index(pl.word.Right()); got != want {
			return fail(where, placementMessage(p, c, g.column(got), tk.text, pl.word),
				"the page prints this figure under a different column than the "+
					"rule declares; check the column order in columns and "+
					"column_headers against the page")
		}
	}
	return nil
}

// column is the reverse of band: which COLUMN a grid band belongs to, so a
// misplacement is reported against the column an author declared rather than
// against a band index they never wrote. -1 for the row-label area, which
// grid.Index also reports as -1 and which belongs to no column.
func (g *columnGuard) column(bandIdx int) int {
	if bandIdx < 0 {
		return -1
	}
	for c, b := range g.band {
		if b == bandIdx {
			return c
		}
	}
	return -1
}

// placementMessage describes a misplacement as a sentence about the DOCUMENT,
// because that is what a reader has to act on: the city moved a column, or the
// rule names them in the wrong order.
func placementMessage(p *Part, want, got int, text string, w geom.Word) string {
	where := "the row-label area, left of the first column"
	if got >= 0 {
		where = fmt.Sprintf("column %d (%s)", got+1, describeColumn(p.Columns[got]))
	}
	return fmt.Sprintf("%q ends at x %.2f, which is %s, but the rule reads it "+
		"as column %d (%s)", text, w.Right(), where, want+1,
		describeColumn(p.Columns[want]))
}

func describeColumn(c Column) string {
	if c.Skip {
		return "skipped"
	}
	// A non-amount column may declare no fiscal year at all, and "FY0" would
	// name it worse than its grammar does.
	if c.Quantity != "" && c.FiscalYear == 0 {
		return string(c.Quantity)
	}
	if c.FundGroup == "" {
		return fmt.Sprintf("FY%d", c.FiscalYear)
	}
	return fmt.Sprintf("%s FY%d", c.FundGroup, c.FiscalYear)
}

// checkLineAccounting is the structural half of the guard, and it is what a
// label-less part has instead of row labels. It reads no geometry itself -- the
// per-row line identity checkRow established is what makes counting printed
// lines meaningful -- so it hangs off the resolver rather than off the guard,
// and runs only for a part that asked to be guarded.
//
// Every row consumes exactly one printed line and every printed line inside the
// block is consumed by exactly one row. checkRow gives each row a single line;
// this adds that there are exactly as many lines as rows, and together with the
// value count that is the bijection.
//
// It catches what the value count cannot. A part whose columns cannot tell two
// figures apart -- a single-column part is the clearest case, since its one band
// claims everything right of the gutter -- has nothing to say when two printed
// rows are fused onto one line, which is a corruption the previous extractor
// produced routinely (fisc-gxt). The count of lines does.
//
// There is deliberately no separate check that no two rows read the SAME line.
// While positionalValues requires the value count to match exactly, a line that
// no row consumed would take its tokens with it and fail that count first, so
// such a check could not fire. It becomes reachable only when the exact count is
// relaxed for sparse pages, and belongs in that change rather than sitting here
// unexercised.
func (r *Resolver) checkLineAccounting(rule *Rule, p *Part, blk *block, rows []Row) error {
	// A block may begin or end mid-line, which is normal -- p67's section anchor
	// is the end of the header line, and its stop_at is the "$" of the totals
	// row. A partial line still counts if it carries anything printed, because
	// what is being counted is the printed lines the block covers.
	fail := func(msg, hint string) error {
		return cmdutil.WithHint(&resolveError{DocID: r.file.DocID, RuleID: rule.ID,
			Page: p.Page, Field: "parts", Msg: msg, Err: ErrNotFound}, hint)
	}

	printed := 0
	for _, l := range strings.Split(blk.Text, "\n") {
		if strings.TrimSpace(l) != "" {
			printed++
		}
	}
	if printed != len(rows) {
		return fail(fmt.Sprintf(
			"the block covers %d printed %s but the rule has %d %s here; "+
				"rows declared absent from this page: %s",
			printed, cmdutil.Plural(printed, "line", "lines"),
			len(rows), cmdutil.Plural(len(rows), "row", "rows"), declaredOmissions(p)),
			"a label-less page is read positionally, one row per printed line, "+
				"so a line the rule has no row for would mismap every row below it")
	}
	return nil
}
