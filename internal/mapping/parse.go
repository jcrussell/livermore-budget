package mapping

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"slices"
	"sort"
	"strings"

	yaml "go.yaml.in/yaml/v3"

	"github.com/jcrussell/livermore-budget/internal/amount"
	"github.com/jcrussell/livermore-budget/internal/hint"
	"github.com/jcrussell/livermore-budget/internal/vocab"
)

// parseError reports a problem in a rule file, located precisely enough to fix
// without hunting.
type parseError struct {
	Path   string
	RuleID string
	Field  string
	Msg    string
}

func (e *parseError) Error() string {
	var b strings.Builder
	b.WriteString(e.Path)
	if e.RuleID != "" {
		fmt.Fprintf(&b, ": rule %q", e.RuleID)
	}
	if e.Field != "" {
		fmt.Fprintf(&b, ": %s", e.Field)
	}
	fmt.Fprintf(&b, ": %s", e.Msg)
	return b.String()
}

// Load reads and validates one rule file.
func Load(p string) (*File, error) {
	// #nosec G304 -- p is a mapping file path chosen by the operator.
	f, err := os.Open(p)
	if err != nil {
		return nil, fmt.Errorf("open rule file: %w", err)
	}
	defer f.Close()
	return parse(f, p)
}

// LoadDir reads every *.yaml under dir, in sorted order, from an fs.FS
// (byob-interfaces.3).
func LoadDir(fsys fs.FS, dir string) ([]*File, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, fmt.Errorf("read mapping dir: %w", err)
	}
	// An unexpected file is an error rather than a skip, since an unread mapping
	// file produces no facts and says nothing.
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		lower := strings.ToLower(name)
		switch {
		case strings.HasSuffix(lower, ".yaml"), strings.HasSuffix(lower, ".yml"):
			names = append(names, name)
		case strings.HasPrefix(name, "."), name == "README.md":
			// editor swap files and docs are fine to ignore
		default:
			return nil, hint.With(
				fmt.Errorf("%s: unexpected file in the mapping directory", path.Join(dir, name)),
				"mapping files must end in .yaml or .yml, or they are never read")
		}
	}
	sort.Strings(names)

	files := make([]*File, 0, len(names))
	seen := map[string]string{} // rule id -> file that defined it
	for _, name := range names {
		full := path.Join(dir, name)
		r, err := fsys.Open(full)
		if err != nil {
			return nil, fmt.Errorf("open %s: %w", full, err)
		}
		parsed, err := parse(r, full)
		// Read-only close: the read has already succeeded or already failed,
		// so there is nothing a Close error would let the caller do.
		_ = r.Close()
		if err != nil {
			return nil, err
		}
		// Rule IDs anchor fact IDs, so a collision across files would make two
		// different rows produce the same fact.
		for _, rule := range parsed.Rules {
			if prev, dup := seen[rule.ID]; dup {
				return nil, &parseError{Path: full, RuleID: rule.ID,
					Msg: fmt.Sprintf("duplicate rule id, already defined in %s", prev)}
			}
			seen[rule.ID] = full
		}
		files = append(files, parsed)
	}
	// Re-checked across the whole directory, not only within each file: two
	// files may map the same document, and nothing else here compares them.
	if err := checkColumnGrids(files); err != nil {
		return nil, err
	}
	return files, nil
}

// parse decodes and validates a rule file.
func parse(r io.Reader, p string) (*File, error) {
	var f File
	dec := yaml.NewDecoder(r)
	// Unknown fields are errors, so a row that lost its `page` to a typo fails.
	dec.KnownFields(true)
	if err := dec.Decode(&f); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, &parseError{Path: p, Msg: "file is empty"}
		}
		return nil, &parseError{Path: p, Msg: err.Error()}
	}

	// A second `---` document is refused rather than silently discarded.
	var extra File
	if err := dec.Decode(&extra); err == nil {
		return nil, hint.With(
			&parseError{Path: p, Msg: "file contains more than one YAML document"},
			"put each document in its own file; every rule in a file must "+
				"belong to the doc_id declared at the top")
	} else if !errors.Is(err, io.EOF) {
		return nil, &parseError{Path: p, Msg: err.Error()}
	}
	f.Path = p
	if err := f.validate(); err != nil {
		return nil, err
	}
	return &f, nil
}

// Validate checks a rule file's internal consistency. Parse calls it; it is
// exported for callers that build a File in memory.
func (f *File) Validate() error { return f.validate() }

func (f *File) validate() error {
	errf := func(ruleID, field, format string, args ...any) error {
		return &parseError{Path: f.Path, RuleID: ruleID, Field: field,
			Msg: fmt.Sprintf(format, args...)}
	}

	switch {
	case f.SchemaVersion == 0:
		return errf("", "schema_version", "is required (want %d)", SchemaVersion)
	case f.SchemaVersion > SchemaVersion:
		return hint.With(
			errf("", "schema_version", "got %d, want %d", f.SchemaVersion, SchemaVersion),
			"this rule file was written for a newer fisc; upgrade the binary")
	case f.SchemaVersion != SchemaVersion:
		return errf("", "schema_version", "got %d, want %d", f.SchemaVersion, SchemaVersion)
	}
	if f.DocID == "" {
		return errf("", "doc_id", "is required")
	}
	if len(f.Rules) == 0 {
		return errf("", "rules", "is empty")
	}

	seen := map[string]bool{}
	for i := range f.Rules {
		r := &f.Rules[i]
		if r.ID == "" {
			return errf("", "rules", "rule %d has no id", i)
		}
		if seen[r.ID] {
			return errf(r.ID, "id", "duplicate rule id")
		}
		seen[r.ID] = true
		if err := validateRule(r, errf); err != nil {
			return err
		}
	}
	if err := validateRollups(f, errf); err != nil {
		return err
	}
	return checkColumnGrids([]*File{f})
}

// validateRollups checks the rollup declarations that need no pages. That the
// anchor occurs once and the covered totals add up is checked when the rollup
// resolves.
func validateRollups(f *File, errf errFunc) error {
	byID := make(map[string]*Rule, len(f.Rules))
	for i := range f.Rules {
		byID[f.Rules[i].ID] = &f.Rules[i]
	}
	seen := map[string]bool{}
	for i := range f.Rollups {
		ro := &f.Rollups[i]
		field := func(name string) string {
			return fmt.Sprintf("rollups[%s].%s", ro.ID, name)
		}
		if ro.ID == "" {
			return errf("", "rollups", "rollup %d has no id", i)
		}
		if seen[ro.ID] {
			return errf("", "rollups", "duplicate rollup id %q", ro.ID)
		}
		seen[ro.ID] = true
		if byID[ro.ID] != nil {
			return hint.With(
				errf("", "rollups", "%q is also a rule id", ro.ID),
				"a rollup and a rule are reported side by side, so one id must "+
					"not name both")
		}

		// Either it asserts or it says why it cannot, never both or neither. A
		// whitespace-only reason is refused, since the build compares it untrimmed.
		if ro.Unassertable != "" && strings.TrimSpace(ro.Unassertable) == "" {
			return errf("", field("unassertable"), "is whitespace; omit it or give the reason")
		}
		asserts, declines := len(ro.Covers) > 0, ro.Unassertable != ""
		// Checked before the `declines` short-circuit, which validateRollupKinds runs
		// after: an unassertable rollup (p140's) is the likeliest place for `kinds:`.
		if len(ro.Kinds) > 0 && declines {
			return hint.With(
				errf("", field("kinds"), "is declared alongside unassertable"),
				"kinds says what a rollup's covered rules span, and an "+
					"unassertable rollup covers none; the reason text is where "+
					"a spanning total that cannot be checked gets described")
		}
		switch {
		case asserts && declines:
			return errf("", field("unassertable"),
				"is declared alongside covers; a rollup either asserts or says why it cannot")
		case !asserts && !declines:
			return hint.With(
				errf("", field("covers"), "is empty and no reason is declared"),
				"say which rules the printed total covers, or declare "+
					"unassertable with the reason it cannot be checked")
		}
		if ro.Page <= 0 {
			return errf("", field("page"), "is required")
		}
		if strings.TrimSpace(ro.TotalRow) == "" {
			return errf("", field("total_row"), "is required")
		}
		if declines {
			continue
		}

		// A rollup may cover one rule: six of pp.167-170's eleven <DEPARTMENT> TOTAL
		// rows sit over a single division and are a second printed line (CITY COUNCIL
		// TOTAL on p0167:15, three lines below City Council's Total on :13). What
		// holds instead is that a rollup may not name a covered rule's total line: the
		// string half is checked here, the page half in CheckRollup.
		covered := map[string]bool{}
		var first *Rule
		for _, id := range ro.Covers {
			rule := byID[id]
			if rule == nil {
				return errf("", field("covers"), "no rule %q in this file", id)
			}
			if covered[id] {
				return hint.With(
					errf("", field("covers"), "%q is listed twice", id),
					"a rule counted twice inflates the sum by its own total")
			}
			covered[id] = true
			// Without a printed total of its own a covered rule contributes nothing to
			// sum.
			if rule.TotalRow == "" {
				return hint.With(
					errf("", field("covers"), "rule %q declares no total_row", id),
					"a rollup sums the totals its covered rules PRINT, so every "+
						"one of them must print one")
			}
			if rule.TotalRow == ro.TotalRow {
				return hint.With(
					errf("", field("total_row"), "is also rule %q's total_row", id),
					"the rollup's anchor must name the line printing the ROLLUP, "+
						"not one of the totals it covers")
			}
			if first == nil {
				first = rule
				continue
			}
			if first.Units != rule.Units {
				return errf("", field("covers"),
					"rule %q states its figures in %s and rule %q in %s",
					first.ID, first.Units, id, rule.Units)
			}
			// Scope is refused where kind is declared. Two scopes over one total is
			// fisc-u2v's doubling raised to the rollup: pp.85-125 and pp.66-67 are the
			// same expenditure counted two ways.
			if first.Scope != rule.Scope {
				return hint.With(
					errf("", field("covers"),
						"rule %q is scope %q and rule %q is scope %q",
						first.ID, first.Scope, id, rule.Scope),
					"a rollup adds totals that are the same money counted once; "+
						"two scopes over one printed total is the same money "+
						"counted twice, which ties and still misstates the city")
			}
			if !slices.Equal(first.Parts[0].Columns, rule.Parts[0].Columns) {
				return hint.With(
					errf("", field("covers"),
						"rules %q and %q do not declare the same columns", first.ID, id),
					"the rollup adds these totals column by column, which is "+
						"only meaningful where the columns are the same")
			}
		}
		if err := validateRollupKinds(ro, byID, field, errf); err != nil {
			return err
		}
	}
	return nil
}

// validateRollupKinds checks the declaration that a printed total covers more
// than one kind. Mixed kinds are what p140's "Total Sources" prints, so the
// author must say so: a list restating the rules' agreed kind, or one absent
// where they disagree, is refused.
func validateRollupKinds(ro *Rollup, byID map[string]*Rule, field func(string) string,
	errf errFunc) error {

	span := map[vocab.Kind]bool{}
	for _, id := range ro.Covers {
		span[byID[id].Kind] = true
	}
	declared := map[vocab.Kind]bool{}
	for _, k := range ro.Kinds {
		if !k.Valid() {
			return errf("", field("kinds"), "%q is not one of the five kinds", k)
		}
		if declared[k] {
			return errf("", field("kinds"), "%q is listed twice", k)
		}
		declared[k] = true
	}
	if len(span) == 1 {
		if len(ro.Kinds) > 0 {
			return hint.With(
				errf("", field("kinds"), "is declared and every covered rule is kind %q",
					sortedKinds(span)[0]),
				"the list exists to say that a printed total spans more than one "+
					"kind; where they agree it restates them and cannot fail")
		}
		return nil
	}
	if len(ro.Kinds) == 0 {
		return hint.With(
			errf("", field("kinds"),
				"is required: the covered rules span %s", describeKinds(sortedKinds(span))),
			"a printed total over two kinds is a real line -- p140's \"Total "+
				"Sources\" is revenue plus transfers in -- but it must be a "+
				"statement the rule file makes, not something the sum happens to "+
				"allow")
	}
	if !slices.Equal(sortedKinds(declared), sortedKinds(span)) {
		return errf("", field("kinds"),
			"names %s and the covered rules span %s",
			describeKinds(sortedKinds(declared)), describeKinds(sortedKinds(span)))
	}
	return nil
}

func sortedKinds(set map[vocab.Kind]bool) []vocab.Kind {
	out := make([]vocab.Kind, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

func describeKinds(ks []vocab.Kind) string {
	parts := make([]string, len(ks))
	for i, k := range ks {
		parts[i] = fmt.Sprintf("%q", k)
	}
	return strings.Join(parts, ", ")
}

type errFunc func(ruleID, field, format string, args ...any) error

// validateClass checks one declaration of a category and a kind, on a row or
// a column. A declaration that publishes needs a category; internal/check
// does not require one.
func validateClass(r *Rule, field, owner, category string, kind vocab.Kind, publishes bool,
	errf errFunc, remedy string) error {
	if kind != "" && !kind.Valid() {
		return errf(r.ID, field, "%s: kind %q is not one of the five", owner, kind)
	}
	if publishes && category == "" {
		return hint.With(errf(r.ID, field, "%s has no category", owner), remedy)
	}
	return nil
}

// validateRowClass holds a row to the axis its rule carries the category on.
// On the row axis every row carries a category; a department is a second axis
// (pp.167-170), not a substitute. A row rowAddressed finds no cell for is
// exempt. A row blank in every amount cell is not: internal/check reads each
// blank as the fact the cell would have been.
func validateRowClass(r *Rule, row Row, byColumn bool, errf errFunc) error {
	if !byColumn {
		return validateClass(r, "rows", fmt.Sprintf("row %q", row.Label), row.Category,
			row.Kind, r.rowAddressed(row), errf,
			"every row needs one, including a row that declares a department: "+
				"department is a second axis, not a substitute. Use skip: true if "+
				"the row is a subtotal that would double-count. On a page whose "+
				"columns are its budget lines, every column carries one instead")
	}
	for _, d := range []struct{ key, val string }{
		{"category", row.Category}, {"kind", string(row.Kind)},
	} {
		if d.val != "" {
			return hint.With(
				errf(r.ID, "rows", "row %q declares %s %q, but this rule's columns "+
					"carry the category", row.Label, d.key, d.val),
				"exactly one axis classifies a rule's figures, so that no cell "+
					"is named twice; declare the category and kind on the rows or "+
					"on the columns, not on both")
		}
	}
	return nil
}

// validateColumnClass is validateRowClass for the part's j'th column. A column
// that publishes nothing may declare no category or kind.
func validateColumnClass(r *Rule, p *Part, j int, byColumn bool, errf errFunc) error {
	c := p.Columns[j]
	field := fmt.Sprintf("parts[page %d]", p.Page)
	owner := fmt.Sprintf("columns[%d]", j)
	if !c.publishes() {
		if c.Category != "" || c.Kind != "" {
			return hint.With(
				errf(r.ID, field, "%s publishes no fact and declares a category or kind", owner),
				"a skipped or non-amount column publishes no fact, so nothing it "+
					"classifies is ever read; remove the declaration")
		}
		return nil
	}
	if byColumn {
		return validateClass(r, field, owner, c.Category, c.Kind, true, errf,
			"this rule's other columns carry its category, so every column that "+
				"publishes must; skip: true a column the rule does not map")
	}
	if c.Kind != "" {
		return hint.With(
			errf(r.ID, field, "%s declares kind %q and no category", owner, c.Kind),
			"a column's kind follows its category; on a rule whose rows carry the "+
				"category, the row's kind overrides the rule's")
	}
	return nil
}

// validateGapLines refuses the entries of one part's line declaration --
// wrapped_labels or headings -- that could never match a gap's trimmed line,
// that repeat, or that are figures, and refuses the declaration outright on a
// labels_from part, which reads no gaps.
func validateGapLines(r *Rule, p *Part, key string, entries []string, errf errFunc,
	labelsFromHint, amountHint string) error {
	field := fmt.Sprintf("parts[page %d].%s", p.Page, key)
	if p.LabelsFrom != 0 && len(entries) > 0 {
		return hint.With(
			errf(r.ID, field,
				"is declared on a part whose labels_from takes its row labels from page %d",
				p.LabelsFrom),
			labelsFromHint)
	}
	seen := map[string]bool{}
	for _, e := range entries {
		if strings.TrimSpace(e) == "" {
			return errf(r.ID, field, "has a blank entry")
		}
		if strings.TrimSpace(e) != e {
			return errf(r.ID, field,
				"%q has leading or trailing whitespace; it is matched against "+
					"a trimmed line of the gap", e)
		}
		if seen[e] {
			return errf(r.ID, field, "%q is listed twice", e)
		}
		if _, err := amount.Parse(e, r.Units); err == nil {
			return hint.With(errf(r.ID, field, "%q is a currency amount", e), amountHint)
		}
		seen[e] = true
	}
	return nil
}

// validateGrain holds a rule to Rule.Grain's two refusals: a publishing rule
// declares one, a rule that publishes nothing does not.
func validateGrain(r *Rule, errf errFunc) error {
	switch {
	case r.publishes() && r.Grain == "":
		return hint.With(
			errf(r.ID, "grain", "is required on a rule that publishes facts"),
			"name the lattice level the table is printed at -- which of fund_group, "+
				"fund, department and category it has an axis for; internal/structure "+
				"checks the name against the facts the rule publishes")
	case !r.publishes() && r.Grain != "":
		return hint.With(
			errf(r.ID, "grain", "is %q, but every row or every column of this rule is "+
				"skipped, non-amount or declared blank, so it publishes no fact", r.Grain),
			"a grain declared over zero facts cannot be checked against the store; "+
				"remove it until the rule publishes")
	}
	return nil
}

// publishes says whether any cell of this rule becomes a fact, by
// cellPublishes.
func (r *Rule) publishes() bool {
	for _, row := range r.Rows {
		if r.RowPublishes(row) {
			return true
		}
	}
	return false
}

// validatePrintedDecimals checks the preconditions of a document-derived
// tolerance; the resolver checks the claim against the tokens it read. See
// Rule.PrintedDecimals and fisc-1wr.2.
func validatePrintedDecimals(r *Rule, errf errFunc) error {
	if r.PrintedDecimals == nil {
		return nil
	}
	d := *r.PrintedDecimals
	if d < 0 {
		return errf(r.ID, "printed_decimals", "is %d; a count of printed decimal places cannot be negative", d)
	}
	max, ok := amount.MaxDecimals(r.Units)
	if !ok {
		// Unreachable: units are validated above. Refusing rather than
		// assuming keeps the arithmetic below from running on a zero scale.
		return errf(r.ID, "printed_decimals", "cannot be checked against unknown units %q", string(r.Units))
	}
	if d > max {
		return hint.With(
			errf(r.ID, "printed_decimals", "is %d, but %s can represent only %d decimal places exactly",
				d, string(r.Units), max),
			"the tolerance is a whole number of cents derived from this count; "+
				"a finer one would not be")
	}
	if r.Units == amount.Dollars {
		return hint.With(
			errf(r.ID, "printed_decimals", "declared on a rule printed in dollars"),
			"a dollar-precision tolerance has no consumer in this corpus, and the "+
				"dollar-level discrepancies it looks like it would cover are the "+
				"city's own arithmetic rather than rounding -- p127's Total Property "+
				"Taxes is $1 over 13 rows -- which stated_total_deltas names exactly "+
				"and this must not absorb silently (fisc-2sd)")
	}
	if r.TotalRow == "" {
		return hint.With(
			errf(r.ID, "printed_decimals", "declared without a total_row"),
			"the tolerance applies to the comparison between the mapped rows and "+
				"a printed total; with no total there is nothing for it to loosen")
	}
	if len(r.Parts) > 1 && !r.TotalSpansParts {
		return hint.With(
			errf(r.ID, "printed_decimals", "declared on a %d-part rule whose total does not span its parts", len(r.Parts)),
			"such a rule is compared once PER PART, so the refusal of a tolerance "+
				"no column needed would fire on whichever page happens to tie "+
				"exactly; declare total_spans_parts, which makes it one comparison, "+
				"or split the rule")
	}
	for i := range r.Parts {
		p := &r.Parts[i]
		if len(p.StatedTotalDeltas) > 0 {
			return hint.With(
				errf(r.ID, fmt.Sprintf("parts[page %d].stated_total_deltas", p.Page),
					"declared on a rule that also declares printed_decimals"),
				"the two are disjoint on purpose: a delta names one exact figure the "+
					"document is out by, a tolerance bounds an unnamed one, and a rule "+
					"holding both offers a place to hide the difference between them")
		}
		// unmapped_text takes a figure out of the read on the author's word; a
		// tolerance beside it would absorb the shortfall if that word is wrong, so the
		// pair is refused. p41's revenue rule declares the orphan and ties exactly.
		if len(p.UnmappedText) > 0 {
			return hint.With(
				errf(r.ID, fmt.Sprintf("parts[page %d].unmapped_text", p.Page),
					"declared on a rule that also declares printed_decimals"),
				"a figure declared out of the read is checked by nothing but the "+
					"arithmetic of the rows around it, and a tolerance is what "+
					"would absorb the error if the declaration were wrong; split "+
					"the rule so the block with the orphan and the block that "+
					"rounds are checked separately")
		}
	}
	return nil
}

// validateTotalRowTail refuses a tail with nothing to follow: no total_row, a
// total printed above its rows (whose line is the section anchor's, so the
// line after it is the block's first row), a rule a label-less part of which
// totals by stop_at, or a tail that is not one line of text.
func validateTotalRowTail(r *Rule, errf errFunc) error {
	if r.TotalRowTail == "" {
		return nil
	}
	switch {
	case r.TotalRow == "":
		return errf(r.ID, "total_row_tail", "declared without a total_row")
	case r.TotalRowAbove:
		return errf(r.ID, "total_row_tail", "declared with total_row_above")
	case slices.ContainsFunc(r.Parts, func(p Part) bool { return p.LabelsFrom != 0 }):
		// A label-less part finds its totals from stop_at, so the tail would
		// be read off no page.
		return errf(r.ID, "total_row_tail", "declared on a rule with a labels_from part")
	case strings.TrimSpace(r.TotalRowTail) != r.TotalRowTail || strings.Contains(r.TotalRowTail, "\n"):
		return errf(r.ID, "total_row_tail",
			"is %q; it is the one printed line after the total, trimmed", r.TotalRowTail)
	}
	return nil
}

// validateTotalRowAbove checks the preconditions of a total the document prints
// above its own rows. Requiring section == total_row pins the total to the one
// line anchor() already resolved, so checkGap's leading arm need skip only
// that line. See Rule.TotalRowAbove and fisc-h96o.
func validateTotalRowAbove(r *Rule, errf errFunc) error {
	if !r.TotalRowAbove {
		return nil
	}
	if r.TotalRow == "" {
		return hint.With(
			errf(r.ID, "total_row_above", "declared without a total_row"),
			"total_row_above says WHERE the printed total is; total_row says "+
				"which line it is, and there is nothing to find without it")
	}
	if r.TotalSpansParts {
		return hint.With(
			errf(r.ID, "total_row_above", "declared with total_spans_parts"),
			"a total printed above its own rows is on the same page as them by "+
				"construction, so it cannot also be the one total of a block "+
				"that straddles a page break")
	}
	for i := range r.Parts {
		p := &r.Parts[i]
		if p.LabelsFrom != 0 {
			return hint.With(
				errf(r.ID, fmt.Sprintf("parts[page %d].labels_from", p.Page),
					"declared on a rule with total_row_above"),
				"a label-less part finds its totals from stop_at and reads no "+
					"section anchor, so it has nowhere to print a total above")
		}
		if p.Section != r.TotalRow {
			return hint.With(
				errf(r.ID, fmt.Sprintf("parts[page %d].section", p.Page),
					"is %q, but total_row_above requires it to be the total_row %q",
					p.Section, r.TotalRow),
				"total_row_above means the stated totals sit on the section "+
					"anchor's own line; anchoring anywhere else would leave the "+
					"total unbounded above the block")
		}
	}
	return nil
}

// validateRowLabelFunds checks the preconditions of the claim that this rule's
// row labels are printed fund names; row-funds-match-their-anchors checks the
// claim against the registry and the pages. A row the declaration would say
// nothing about, one with no fund, is refused.
func validateRowLabelFunds(r *Rule, errf errFunc) error {
	if !r.RowLabelsNameFunds {
		return nil
	}
	// A rule whose every row is skipped is refused too, since the check drops
	// skipped rows. The count goes through ActiveRows, as the check's
	// activeInAnyPart does.
	covered := 0
	seen := map[string]bool{}
	for i := range r.Parts {
		for _, row := range r.ActiveRows(&r.Parts[i]) {
			if row.Skip || seen[row.Identity()] {
				continue
			}
			seen[row.Identity()] = true
			covered++
		}
	}
	if covered == 0 {
		return hint.With(
			errf(r.ID, "row_labels_name_funds",
				"no part of this rule reads any of its rows, so the declaration "+
					"covers none of them"),
			"a skipped row is never read from the page, so nothing reads its "+
				"label and nothing checks the fund typed on it")
	}
	for _, row := range r.Rows {
		if row.Skip {
			// A skipped row is not read, so the claim is not about it. This is
			// the same reading validateTotalRowKinds takes of `have`.
			continue
		}
		if row.Fund == 0 {
			return hint.With(
				errf(r.ID, "row_labels_name_funds",
					"row %q declares no fund, so the declaration says nothing about it",
					row.PrintedLabel()),
				"the declaration is what lets row-funds-match-their-anchors read "+
					"a fund off the printed label and check the number typed "+
					"beside it; a row with no number to check needs no label read")
		}
		// A counterpart is refused: its fund is at the far end of the movement, and a
		// bare fund name carries no direction to reach it.
		if row.Counterpart != nil {
			return hint.With(
				errf(r.ID, "row_labels_name_funds",
					"row %q declares a counterpart, whose fund the printed label "+
						"cannot name", row.PrintedLabel()),
				"a bare fund name says which fund and never which end, so this "+
					"declaration speaks for the row's own fund only; a schedule "+
					"with two ends per row needs the verb-phrase anchors p76 prints")
		}
	}
	return nil
}

// validateTotalRowKinds checks the claim that the printed total covers only
// some of the kinds beneath it. Every refusal is of a declaration that asserts
// nothing.
func validateTotalRowKinds(r *Rule, errf errFunc) error {
	if len(r.TotalRowKinds) == 0 {
		return nil
	}
	if r.TotalRow == "" {
		return hint.With(
			errf(r.ID, "total_row_kinds", "is declared but the rule has no total_row"),
			"the list says which of the rule's kinds the PRINTED total covers, "+
				"so there must be a printed total for it to describe")
	}
	seen := map[vocab.Kind]bool{}
	for _, k := range r.TotalRowKinds {
		if !k.Valid() {
			return errf(r.ID, "total_row_kinds", "%q is not one of the five kinds", k)
		}
		if seen[k] {
			return errf(r.ID, "total_row_kinds", "%q is listed twice", k)
		}
		seen[k] = true
	}
	// A declared kind no row carries would silently narrow the check.
	have := map[vocab.Kind]bool{}
	for _, row := range r.Rows {
		if row.Skip {
			continue
		}
		for _, k := range r.kindsOf(row) {
			have[k] = true
		}
	}
	for _, k := range r.TotalRowKinds {
		if !have[k] {
			return hint.With(
				errf(r.ID, "total_row_kinds", "no row of this rule has kind %q", k),
				"the list names the kinds the printed total covers, and every "+
					"one of them must be a kind this rule maps")
		}
	}
	if len(seen) == len(have) {
		return hint.With(
			errf(r.ID, "total_row_kinds", "names every kind the rule maps, so it excludes nothing"),
			"a total that covers all of its rows needs no declaration; remove "+
				"it, and the check stays kind-blind as the mixed fund blocks "+
				"on pp.131-140 need")
	}
	return nil
}

// validateTotalSpansParts checks the declaration that a printed total covers
// every part's rows. Which page prints the total, and that a declared delta is
// on it, are checked when the rule resolves.
func validateTotalSpansParts(r *Rule, errf errFunc) error {
	if !r.TotalSpansParts {
		return nil
	}
	if r.TotalRow == "" {
		return hint.With(
			errf(r.ID, "total_spans_parts", "is set but the rule declares no total_row"),
			"the flag says WHICH rows the printed total covers; with no total "+
				"row declared there is nothing for it to say that about")
	}
	// On one part the flag would assert nothing.
	if len(r.Parts) < 2 {
		return hint.With(
			errf(r.ID, "total_spans_parts", "is set on a rule with %d part", len(r.Parts)),
			"the flag exists for a block whose rows straddle a page break; on "+
				"a single part it is the per-part check already, so remove it")
	}
	// Identical columns, field by field: this keeps the flag off the p66-67
	// spine, whose parts partition columns (four and eight) rather than rows.
	first := r.Parts[0]
	for i := 1; i < len(r.Parts); i++ {
		p := r.Parts[i]
		if slices.Equal(first.Columns, p.Columns) {
			continue
		}
		return hint.With(
			errf(r.ID, "total_spans_parts",
				"page %d declares %d columns and page %d declares %d, and they must be identical",
				first.Page, len(first.Columns), p.Page, len(p.Columns)),
			"summing rows across parts is only meaningful where the parts "+
				"split the same table by ROW; parts that split it by COLUMN "+
				"are not what this flag describes")
	}
	// One printed total, so at most one part may declare a delta against it.
	declaring := make([]int, 0, len(r.Parts))
	for _, p := range r.Parts {
		if len(p.StatedTotalDeltas) > 0 {
			declaring = append(declaring, p.Page)
		}
	}
	if len(declaring) > 1 {
		return hint.With(
			errf(r.ID, "stated_total_deltas",
				"pages %v each declare a delta, but this rule has one printed total", declaring),
			"declare the discrepancy on the part whose page prints the total row")
	}
	return nil
}

// validatePartAnchors checks the fields that say which block on the page this
// part reads.
func validatePartAnchors(r *Rule, p *Part, errf errFunc) error {
	field := func(name string) string {
		return fmt.Sprintf("parts[page %d].%s", p.Page, name)
	}

	if p.SectionOrdinal < 0 {
		return errf(r.ID, field("section_ordinal"), "is %d; ordinals count from 1",
			p.SectionOrdinal)
	}
	if p.SectionOrdinal > 0 && p.Section == "" {
		return hint.With(
			errf(r.ID, field("section_ordinal"), "is set but section is empty"),
			"section_ordinal picks which occurrence of section starts the "+
				"block, so it means nothing without one")
	}
	if p.StopAtOrdinal < 0 {
		return errf(r.ID, field("stop_at_ordinal"), "is %d; ordinals count from 1",
			p.StopAtOrdinal)
	}
	if p.StopAtOrdinal > 0 && p.StopAt == "" {
		return hint.With(
			errf(r.ID, field("stop_at_ordinal"), "is set but stop_at is empty"),
			"stop_at_ordinal picks which occurrence of stop_at ends the block, "+
				"so it means nothing without one")
	}
	if len(p.ColumnHeaders) == 0 {
		return nil
	}
	// One entry per column, including skipped ones; a short list would shift
	// every figure right of the gap one place left with the count still matching.
	if len(p.ColumnHeaders) != len(p.Columns) {
		return hint.With(
			errf(r.ID, field("column_headers"), "has %d entries but the part has %d columns",
				len(p.ColumnHeaders), len(p.Columns)),
			"name the printed header of every column left to right, including "+
				"any marked skip: true; the list is what says where each column "+
				"sits on the page")
	}
	// At least one column must be headed, or there is no grid to build.
	if !slices.ContainsFunc(p.ColumnHeaders, func(h columnHeader) bool { return !h.Unheaded }) {
		return hint.With(
			errf(r.ID, field("column_headers"), "every entry is null"),
			"the grid is built from the headers the page prints, so at least one "+
				"column must name one; a part that can name none declares no "+
				"column_headers at all")
	}
	for i, h := range p.ColumnHeaders {
		if h.Unheaded {
			// A null column must be skipped: it has no band to file a figure under.
			if !p.Columns[i].Skip {
				return hint.With(
					errf(r.ID, field("column_headers"), "entry %d is null but column %d "+
						"is not skipped", i+1, i+1),
					"null says the page prints no header over this column, which "+
						"leaves it no band; a column the rule actually reads must "+
						"name its printed header so its figures can be placed")
			}
			// Only at the end: between two headers the grid could check the token.
			if i != len(p.ColumnHeaders)-1 {
				return hint.With(
					errf(r.ID, field("column_headers"), "entry %d is null but is not "+
						"the last of %d", i+1, len(p.ColumnHeaders)),
					"a headerless column is only unplaceable past the last printed "+
						"header; between two headers it sits in a gap the grid can "+
						"check, so name the header on either side and declare the "+
						"skipped column there")
			}
			continue
		}
		if strings.TrimSpace(h.Text) == "" {
			return hint.With(
				errf(r.ID, field("column_headers"), "entry %d is empty", i+1),
				"a header is matched by joining the words printed on the header "+
					"line, so no page can produce an empty one; write null, not "+
					"\"\", for a column the page heads with nothing")
		}
		// A header that is a figure would match a data row.
		if _, err := amount.Parse(h.Text, r.Units); err == nil {
			return hint.With(
				errf(r.ID, field("column_headers"), "entry %d is a currency amount: %q",
					i+1, h.Text),
				"name more of the printed header -- \"FY 2026\" rather than "+
					"\"2026\" -- because a header that is a figure matches a data "+
					"row as readily as the header line")
		}
	}
	return nil
}

// checkColumnGrids checks what no per-part rule can see: every part reading a
// page agrees about that page's column grid, and they all opt into it or none
// do. The spine writes p66's and p67's header lists out five times; a drifted
// copy would build a different grid while still tying to its own totals.
// Grouping is by (doc_id, page), not by file. A page printing two schedules
// names them apart with Part.Grid (Budget Book pp.80-81); two grids of one
// page declaring one list are refused.
func checkColumnGrids(files []*File) error {
	type declaration struct {
		path    string
		rule    string
		headers columnHeaders
	}
	type key struct {
		docID string
		page  int
		grid  string
	}

	declared := map[key]declaration{}
	for _, f := range files {
		for i := range f.Rules {
			r := &f.Rules[i]
			for j := range r.Parts {
				p := &r.Parts[j]
				k := key{docID: f.DocID, page: p.Page, grid: p.Grid}
				cur := declaration{path: f.Path, rule: r.ID, headers: p.ColumnHeaders}
				if len(p.ColumnHeaders) == 0 {
					continue
				}
				prev, ok := declared[k]
				if !ok {
					declared[k] = cur
					continue
				}
				if !slices.Equal(prev.headers, p.ColumnHeaders) {
					return hint.With(
						&parseError{Path: f.Path, RuleID: r.ID,
							Field: fmt.Sprintf("parts[page %d].column_headers", p.Page),
							Msg: fmt.Sprintf("is %s, but rule %q in %s declares %s for the same page",
								describeHeaders(p.ColumnHeaders), prev.rule, prev.path,
								describeHeaders(prev.headers))},
						"a page has one column grid; two parts describing it "+
							"differently would read the same figures into different "+
							"columns and each would still tie against its own total")
				}
			}
		}
	}

	// Two names for one header list on one page are one grid named twice.
	for a, da := range declared {
		for b, db := range declared {
			if a.docID == b.docID && a.page == b.page && a.grid < b.grid &&
				slices.Equal(da.headers, db.headers) {
				return &parseError{Path: db.path, RuleID: db.rule,
					Field: fmt.Sprintf("parts[page %d].grid", b.page),
					Msg: fmt.Sprintf("grid %q declares %s, as grid %q of rule %q does; "+
						"one header list is one grid", b.grid, describeHeaders(db.headers),
						a.grid, da.rule)}
			}
		}
	}

	// Any grid of a page, for the all-or-none half.
	type pageKey struct {
		docID string
		page  int
	}
	// The first grid by name, so the message does not depend on map order.
	anyGrid := map[pageKey]key{}
	for k := range declared {
		pk := pageKey{k.docID, k.page}
		if prev, ok := anyGrid[pk]; !ok || k.grid < prev.grid {
			anyGrid[pk] = k
		}
	}

	// Second pass, because a part with no headers may be read before the part
	// that declares them.
	for _, f := range files {
		for i := range f.Rules {
			r := &f.Rules[i]
			for j := range r.Parts {
				p := &r.Parts[j]
				if len(p.ColumnHeaders) != 0 {
					continue
				}
				if p.Grid != "" {
					return hint.With(
						&parseError{Path: f.Path, RuleID: r.ID,
							Field: fmt.Sprintf("parts[page %d].grid", p.Page),
							Msg:   fmt.Sprintf("names grid %q but declares no column_headers", p.Grid)},
						"a grid is a header list this part reads its columns by; "+
							"name one only on a part that declares the headers")
				}
				k, ok := anyGrid[pageKey{f.DocID, p.Page}]
				if !ok {
					continue
				}
				prev := declared[k]
				return hint.With(
					&parseError{Path: f.Path, RuleID: r.ID,
						Field: fmt.Sprintf("parts[page %d]", p.Page),
						Msg: fmt.Sprintf("declares no column_headers, but rule %q in %s "+
							"declares %s for the same page", prev.rule, prev.path,
							describeHeaders(prev.headers))},
					"every part reading a page opts into the column guard or none "+
						"does; one part left out is a part whose figures are placed "+
						"by position alone, with nothing saying so")
			}
		}
	}
	return nil
}

// unitsValid reports whether u is one this project supports, reusing the
// amount package's own table so the two cannot drift.
func unitsValid(u amount.Units) bool {
	switch u {
	case amount.Dollars, amount.Thousands, amount.Millions:
		// Parse a zero to confirm the unit is wired up on the amount side too.
		_, err := amount.Parse("0", u)
		return err == nil
	}
	return false
}

// checkCounterpart refuses a far leg that could not be told apart from its own
// near leg, or that would be filed nowhere. A counterpart is invisible to
// CheckTotals (fact.FromValues fans out after it), so these arms stand in.
func checkCounterpart(r *Rule, row Row, errf errFunc) error {
	cp := row.Counterpart
	if cp == nil {
		return nil
	}
	if r.categoryOnColumns() {
		return hint.With(
			errf(r.ID, "rows", "row %q declares a counterpart, but this rule's columns "+
				"carry the category", row.Label),
			"a counterpart names ONE far end for every figure of its row, and a row "+
				"of this rule holds a figure per budget line -- its revenue, its "+
				"transfers in, its balances -- which no single far end is the other "+
				"side of")
	}
	if cp.Category == "" {
		return hint.With(
			errf(r.ID, "rows", "row %q: counterpart has no category", row.Label),
			"the far leg classifies on its own account -- a transfer OUT of the "+
				"paying fund, where the near leg is a transfer IN to the receiving "+
				"one -- and nothing is inherited from the row")
	}
	if !cp.Kind.Valid() {
		return errf(r.ID, "rows", "row %q: counterpart kind %q is not one of the five",
			row.Label, cp.Kind)
	}
	if cp.FundGroup == "" {
		return hint.With(
			errf(r.ID, "rows", "row %q: counterpart has no fund_group", row.Label),
			"without one the leg's column_path falls back to the rule's scope, "+
				"where it is indistinguishable from any other scope-filed fact")
	}
	// The far leg needs a fund, not only a group: nothing downstream catches a
	// fund-0 leg (fact.FromValues fires only when both are absent). The near leg
	// may be group-only: p76's LAVWMA row receives into a joint powers authority
	// that is no City fund.
	if cp.Fund == 0 {
		return hint.With(
			errf(r.ID, "rows", "row %q: counterpart declares fund_group %q and no fund",
				row.Label, cp.FundGroup),
			"a counterpart names the fund at the far end of the movement; a group "+
				"on its own names no payer, and no check downstream can tell it "+
				"from one that was never declared")
	}
	// Counterpart has no department, so the far leg would silently take the near
	// leg's; refused rather than dropped.
	if row.Department != "" {
		return hint.With(
			errf(r.ID, "rows", "row %q carries department %q and declares a counterpart",
				row.Label, row.Department),
			"the far leg has no department of its own to declare, and inheriting "+
				"this one would attribute the paying fund's outflow to the "+
				"receiving department")
	}
	// A contra sign would mark the paying fund's outflow as reducing a category;
	// netted is inherited, and validateNettedSign holds both its ends.
	if row.Sign == vocab.SignContra {
		return hint.With(
			errf(r.ID, "rows", "row %q carries sign %q and declares a counterpart",
				row.Label, row.Sign),
			"the far leg would inherit the contra sign, and a reduction of the "+
				"receiving row's category says nothing about the paying fund's")
	}
	// The two legs must differ in category (row path) or fund (column path), or
	// the ids collide. The fund is compared against every cell cellPublishes
	// admits, since each such cell publishes both legs. fact.CheckUniqueIDs is the
	// backstop.
	if cp.Category != row.Category {
		return nil
	}
	for i := range r.Parts {
		p := &r.Parts[i]
		for j := range p.Columns {
			if !p.cellPublishes(j, row) {
				continue
			}
			near := row.EffectiveColumn(p.Columns[j])
			if cp.Fund == near.Fund && cp.FundGroup == near.FundGroup {
				return hint.With(
					errf(r.ID, "rows", "row %q: counterpart is the same category and the "+
						"same fund as the row itself in column %d of the part on page %d",
						row.Label, j+1, p.Page),
					"the two legs of one figure are told apart by their category and "+
						"their fund; identical on both, they are one fact published twice")
			}
		}
	}
	return nil
}

// resolveOmittedCells is the one place an omitted_cells entry's header is
// resolved to a column, by Part.columnHeaded, stored in Part.blank. The part
// must declare column_headers.
func resolveOmittedCells(r *Rule, p *Part, errf errFunc) error {
	if len(p.OmittedCells) == 0 {
		return nil
	}
	field := fmt.Sprintf("parts[page %d].omitted_cells", p.Page)
	if len(p.ColumnHeaders) == 0 {
		return hint.With(
			errf(r.ID, field, "omitted_cells needs column_headers on the same part"),
			"a row with a blank cell is read left to right under the columns it "+
				"prints, and only the column guard can say each figure is under "+
				"the column it is filed under")
	}
	blank := map[string]map[int]bool{}
	for _, o := range p.OmittedCells {
		if o.Column == "" {
			return errf(r.ID, field, "%q names no column", o.row().PrintedLabel())
		}
		at, err := p.columnHeadedAt(o.Column, o.ColumnOrdinal)
		if err != nil {
			return errf(r.ID, field, "%v", err)
		}
		id := o.row().Identity()
		if blank[id][at] {
			return errf(r.ID, field, "%s is declared twice", o.describe())
		}
		if blank[id] == nil {
			blank[id] = map[int]bool{}
		}
		blank[id][at] = true
	}
	p.blank = blank
	return nil
}

// validateOmittedCells holds each omitted_cells entry, resolved by
// resolveOmittedCells, to naming one cell the part reads: one row, by
// namesOneRow, that the part prints, leaving the row a figure to print.
func validateOmittedCells(r *Rule, p *Part,
	namesOneRow func(field string, j int, o Row, notARow string) error, errf errFunc) error {
	if len(p.OmittedCells) == 0 {
		return nil
	}
	field := fmt.Sprintf("parts[page %d].omitted_cells", p.Page)
	// The rule's row, not the entry's: the entry names the row by its anchors
	// and the row carries its page.
	rows := map[string]Row{}
	for _, row := range r.Rows {
		rows[row.Identity()] = row
	}
	for j, o := range p.OmittedCells {
		if err := namesOneRow(field, j, o.row(), "omitted_cells names a cell of a "+
			"row the rule lists"); err != nil {
			return err
		}
		id := o.row().Identity()
		switch {
		case !rows[id].OnPart(p):
			return errf(r.ID, field, "%q is printed by page %d, not by this part, "+
				"so it has no cell to leave blank", o.row().PrintedLabel(), rows[id].Page)
		case strings.TrimSpace(o.Note) == "":
			return hint.With(
				errf(r.ID, field, "%s has no note", o.describe()),
				"say what the page prints on the row and that this cell is blank")
		}
	}
	for _, row := range r.Rows {
		id := row.Identity()
		if p.blank[id] == nil {
			continue
		}
		// Counted over the columns the row still prints, skipped ones included: a row
		// printing no figure leaves the read nothing to end on. Budget Book p81's
		// 2046-2050 line prints only a skipped Grand Total; ACFR p121 leaves its
		// skipped Variance column blank.
		printed := 0
		for c := range p.Columns {
			if !p.blank[id][c] {
				printed++
			}
		}
		if printed == 0 {
			return hint.With(
				errf(r.ID, field, "declares every column of %q blank",
					row.PrintedLabel()),
				"a row this part prints no figure of is not a row of this part: "+
					"place it with page on the part that prints it, or remove it")
		}
	}
	return nil
}

// describeHeaders renders a header list for an error message, a null entry as
// the claim it is.
func describeHeaders(hs columnHeaders) string {
	parts := make([]string, len(hs))
	for i, h := range hs {
		parts[i] = h.String()
	}
	return "[" + strings.Join(parts, " ") + "]"
}

// validateSubtotals checks the rows a rule declares as printed subtotals. A
// subtotal must also be skip: true, and every part of a rule with a subtotal
// row, or in a chain, names each of its columns.
func validateSubtotals(r *Rule, errf errFunc) error {
	has := r.SubtotalChain != ""
	for _, row := range r.Rows {
		switch {
		case row.Subtotal < 0:
			return errf(r.ID, "rows", "row %q: subtotal is %d; levels count from 1",
				row.PrintedLabel(), row.Subtotal)
		case row.Subtotal > 0 && !row.Skip:
			return hint.With(
				errf(r.ID, "rows", "row %q is a subtotal and is not skip: true", row.PrintedLabel()),
				"a printed subtotal publishes nothing; publishing it would count its rows twice")
		case row.Subtotal > 0:
			has = true
		case len(row.SubtotalDeltas) > 0:
			return errf(r.ID, "rows", "row %q declares subtotal_deltas and is not a subtotal",
				row.PrintedLabel())
		}
		seen := map[string]bool{}
		for _, d := range row.SubtotalDeltas {
			switch {
			case d.Column == "":
				return errf(r.ID, "rows", "row %q: a subtotal_deltas entry names no column",
					row.PrintedLabel())
			case seen[d.Column]:
				return errf(r.ID, "rows", "row %q: subtotal_deltas names %q twice",
					row.PrintedLabel(), d.Column)
			case d.Cents == 0:
				return errf(r.ID, "rows", "row %q: the subtotal_deltas entry for %q is zero, "+
					"which is a column that ties and declares nothing", row.PrintedLabel(), d.Column)
			case strings.TrimSpace(d.Note) == "":
				return errf(r.ID, "rows", "row %q: the subtotal_deltas entry for %q has no note",
					row.PrintedLabel(), d.Column)
			}
			seen[d.Column] = true
		}
	}
	if !has {
		return nil
	}
	headers := map[string]int{}
	for _, p := range r.Parts {
		for _, h := range p.ColumnHeaders {
			headers[h.Text]++
		}
	}
	for _, row := range r.Rows {
		for _, d := range row.SubtotalDeltas {
			if headers[d.Column] != 1 {
				return errf(r.ID, "rows", "row %q: subtotal_deltas names %q, which %d of this "+
					"rule's columns print over them; a delta names exactly one",
					row.PrintedLabel(), d.Column, headers[d.Column])
			}
		}
	}
	for _, p := range r.Parts {
		if len(p.ColumnHeaders) == 0 {
			return hint.With(
				errf(r.ID, fmt.Sprintf("parts[page %d].column_headers", p.Page),
					"is empty, and this rule's subtotals compare figures by column header"),
				"declare the headers the page prints over each column")
		}
		for i, h := range p.ColumnHeaders {
			if h.Unheaded {
				return errf(r.ID, fmt.Sprintf("parts[page %d].column_headers", p.Page),
					"entry %d is null, and this rule's subtotals compare figures by column header", i+1)
			}
		}
	}
	return nil
}
