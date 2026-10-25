package mapping

import (
	"fmt"
	"slices"
	"strings"

	"github.com/jcrussell/livermore-budget/internal/english"
	"github.com/jcrussell/livermore-budget/internal/geom"
	"github.com/jcrussell/livermore-budget/internal/hint"
)

// This file is the column-position guard: the second witness that a figure is
// printed in the column the rule filed it under. It never selects which tokens
// a row consumes; the -layout read stays the only source of a value, offset and
// token. Geometry supplies a column index, so the guard can only refuse.

// placed is one -layout token's geometry: the word it pairs with, and which
// printed line the two of them sit on.
type placed struct {
	word geom.Word
	// line indexes the page's non-blank lines, which is what makes "these two
	// tokens are on the same line" a comparison.
	line int
	// num is that line's number in the page text, for messages; p67 has 15 blank
	// lines among its 48, so its tenth non-blank line is line 23.
	num int
}

// pairing is one page's two substrates reconciled against each other. The
// line index is carried per token, not derived from y0: words on one printed
// line spread up to 0.96pt on the mapped pages.
type pairing struct {
	// words is keyed by absolute byte offset into the page text, which is the
	// offset tokens() already records and Value already publishes.
	words map[int]placed
	// lines is the geometry's own view of the page, for the header search.
	lines []geom.Line
	// nums gives each of those lines its number in the page text.
	nums []int
	// raised holds the index in lines of every footnote superscript
	// splitSuperscripts gave back to a line of its own.
	raised map[int]bool
}

// buildPairing reconciles the page text with the word geometry, token by
// token, or fails, and the page cannot carry the guard. Measured at b47f332, on
// 99 of the corpus's 786 pages the two disagree about the line count and on 108
// more about a line's tokens. ACFR p41 (52 text lines, 51 geometry lines) is
// mapped and declares no column_headers; fisc build refuses a part declaring
// column_headers on a page that cannot pair.
func buildPairing(text string, g *geom.Page) (*pairing, error) {
	var printed []textLine
	base := 0
	for i, body := range strings.Split(text, "\n") {
		if strings.TrimSpace(body) != "" {
			printed = append(printed, textLine{body: body, base: base, num: i + 1})
		}
		base += len(body) + 1 // the newline strings.Split consumed
	}
	lines, raised := splitSuperscripts(printed, g.Lines())

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
	return &pairing{words: words, lines: lines, nums: nums, raised: raised}, nil
}

// textLine is one non-blank line of the page text.
type textLine struct {
	body string
	base int
	num  int
}

// splitSuperscripts gives back to its own line every footnote superscript the
// geometry clustered into the printed line below it. The pages that pair only
// because of this are pinned by TestFootnoteSuperscriptsTheCorpusReconciles.
// It splits rather than compares, so buildPairing's comparison still refuses a
// page that disagrees elsewhere. The set it returns indexes the split
// superscripts.
func splitSuperscripts(printed []textLine, lines []geom.Line) ([]geom.Line, map[int]bool) {
	out := make([]geom.Line, 0, len(lines))
	raised := map[int]bool{}
	j := 0
	for i := 0; i < len(printed) && j < len(lines); i++ {
		if i+1 < len(printed) {
			if mark, rest, ok := absorbedSuperscript(printed[i].body, printed[i+1].body, lines[j]); ok {
				raised[len(out)] = true
				out = append(out, mark, rest)
				i++
				j++
				continue
			}
		}
		out = append(out, lines[j])
		j++
	}
	return append(out, lines[j:]...), raised
}

// absorbedSuperscript reports whether geometry line l is the text line next
// plus the lone footnote marker the text line marker prints above it, and if
// so returns the two lines apart. The word must be a footnote marker, shorter
// than every other word on l and raised above all of them; anything else stays
// refused. ACFR p41's "0.0" fails every arm.
func absorbedSuperscript(marker, next string, l geom.Line) (geom.Line, geom.Line, bool) {
	toks := tokens(marker, 0)
	if len(toks) != 1 || !isFootnoteMarker(toks[0].text) {
		return geom.Line{}, geom.Line{}, false
	}
	want := tokens(next, 0)
	if len(l.Words) != len(want)+1 {
		return geom.Line{}, geom.Line{}, false
	}
	found := -1
	for k, w := range l.Words {
		if w.Text != toks[0].text {
			continue
		}
		rest := slices.Delete(slices.Clone(l.Words), k, k+1)
		if !sameTexts(rest, want) || !isSuperscript(w, rest) {
			continue
		}
		if found >= 0 {
			return geom.Line{}, geom.Line{}, false
		}
		found = k
	}
	if found < 0 {
		return geom.Line{}, geom.Line{}, false
	}
	return geom.Line{Words: []geom.Word{l.Words[found]}},
		geom.Line{Words: slices.Delete(slices.Clone(l.Words), found, found+1)}, true
}

// isFootnoteMarker accepts "1", "12" and "(1)" -- the forms the reconciled
// pages print -- and no figure of three digits or more.
func isFootnoteMarker(s string) bool {
	if len(s) > 2 && s[0] == '(' && s[len(s)-1] == ')' {
		s = s[1 : len(s)-1]
	}
	if len(s) == 0 || len(s) > 2 {
		return false
	}
	for i := range len(s) {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// raisedMarker reports whether the token at page offset off is a footnote
// marker absorbedSuperscript split out of the line below it. A marker on a
// geometry line of its own is refused: nothing ties it to the line below. A
// part with no guard prints no marker.
func (g *columnGuard) raisedMarker(off int) bool {
	if g == nil {
		return false
	}
	pl, ok := g.pair.words[off]
	return ok && g.pair.raised[pl.line]
}

func isSuperscript(w geom.Word, line []geom.Word) bool {
	for _, o := range line {
		if w.Y1-w.Y0 >= o.Y1-o.Y0 || w.Y0 >= o.Y0 {
			return false
		}
	}
	return true
}

func sameTexts(words []geom.Word, toks []token) bool {
	if len(words) != len(toks) {
		return false
	}
	for i, w := range words {
		if w.Text != toks[i].text {
			return false
		}
	}
	return true
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

// pairing returns the reconciled substrates for page n, at most once. A
// failure is not cached, since this is I/O, as page. The lock is not held
// across the work, which calls page.
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
// that declares no column_headers.
type columnGuard struct {
	grid *geom.Grid
	pair *pairing
	// band maps a column's index to its index in grid, and -1 for a headerless
	// column, which has no band. Only the last column may be unbanded; a map
	// rather than a count so a parse mistake cannot shift every band.
	band []int
}

// guard builds the column guard for a part, or returns nil if the part did not
// ask for one.
func (r *Resolver) guard(rule *Rule, p *Part) (*columnGuard, error) {
	if len(p.ColumnHeaders) == 0 {
		return nil, nil
	}
	fail := func(err error, msg, remedy string) (*columnGuard, error) {
		return nil, hint.With(&resolveError{DocID: r.file.DocID, RuleID: rule.ID,
			Page: p.Page, Field: "column_headers", Msg: msg, Err: err}, remedy)
	}

	pr, err := r.pairing(p.Page)
	if err != nil {
		return nil, hint.With(&resolveError{DocID: r.file.DocID, RuleID: rule.ID,
			Page: p.Page, Field: "geometry", Msg: err.Error(), Err: err},
			"this part declares column_headers, so it is read against "+
				"geometry/pNNNN.json as well as the page text; a page whose two "+
				"substrates disagree cannot carry the column guard")
	}

	// The header line is searched over the whole page, not the block: three of
	// the spine's p66 parts anchor below it. A headerless column is matched
	// against nothing; the band map keeps the rest aligned.
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
	// A word on the header line right of the first header and in no declared
	// header is a column the rule does not know about. In-order matching catches
	// a dropped column, not a gained one.
	if stray, ok := m.strayRightOfFirst(pr.lines[m.line]); ok {
		return fail(ErrNotFound,
			fmt.Sprintf("%q is printed on the header line at x %.2f-%.2f but is "+
				"not one of this part's columns", stray.Text, stray.X0, stray.X1),
			"the page prints a column this part does not declare; add it to "+
				"columns with skip: true, or name it in column_headers")
	}

	// The gutter is the first column header's left edge; without it every row
	// label would file into the first column.
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
// joining consecutive words with one space. Each header must occur after the
// one before it, so a dropped column fails.
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

// strayRightOfFirst returns a word on the header line right of the first
// matched header that belongs to no header. Words left of it are the label
// area's heading (CIP p40's "PROJECT NUMBER PROJECT NAME").
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

// checkRow holds every figure this row yields to the band of the column the
// rule filed it under, all off one printed line. A token left of the gutter
// (Index -1) or two tokens in one band (a figure extraction split) fail
// placement too. toks[k] is filed under cols[k]; a word in the band of a
// column declared blank refuses the row, checked before placement.
func (g *columnGuard) checkRow(r *Resolver, rule *Rule, p *Part, row Row, toks []token,
	cols []int) error {
	fail := func(field, msg, remedy string) error {
		return hint.With(&resolveError{DocID: r.file.DocID, RuleID: rule.ID,
			Page: p.Page, Field: field, Msg: msg, Err: ErrNotFound}, remedy)
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
			// The substrates agree token for token, so an offset no word starts at means
			// a row label matched part of a printed word.
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
		// A headerless column has no band and no placement claim; only the last
		// column may be one.
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

// column is the reverse of band, so a misplacement names the declared column;
// -1 for the row-label area.
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

// placementMessage describes a misplacement as a sentence about the document.
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

// checkLineAccounting is the structural half of the guard, what a label-less
// part has instead of row labels: every row consumes exactly one printed line
// and every printed line in the block is consumed by exactly one row. It
// catches two rows fused onto one line, which a single-column part's value
// count cannot. No check that two rows read the same line: under
// positionalValues' exact count it could not fire.
func (r *Resolver) checkLineAccounting(rule *Rule, p *Part, blk *block, rows []Row) error {
	// A block may begin or end mid-line (p67); a partial line counts if it
	// carries anything printed.
	fail := func(msg, remedy string) error {
		return hint.With(&resolveError{DocID: r.file.DocID, RuleID: rule.ID,
			Page: p.Page, Field: "parts", Msg: msg, Err: ErrNotFound}, remedy)
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
				"rows placed on other pages: %s",
			printed, english.Plural(printed, "line", "lines"),
			len(rows), english.Plural(len(rows), "row", "rows"), declaredOmissions(rule, p)),
			"a label-less page is read positionally, one row per printed line, "+
				"so a line the rule has no row for would mismap every row below it")
	}
	return nil
}
