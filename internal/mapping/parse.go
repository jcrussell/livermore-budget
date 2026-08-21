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
	"github.com/jcrussell/livermore-budget/pkg/cmdutil"
)

// ParseError reports a problem in a rule file, located precisely enough to fix
// without hunting. Rule files are hand-written, so the error message is the
// primary interface for most of the people who will ever see this package.
type ParseError struct {
	Path   string
	RuleID string
	Field  string
	Msg    string
}

func (e *ParseError) Error() string {
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
	// #nosec G304 -- p is a mapping file path chosen by the operator running
	// fisc, not attacker-controlled input; refusing variable paths here would
	// mean hard-coding the mapping directory.
	f, err := os.Open(p)
	if err != nil {
		return nil, fmt.Errorf("open rule file: %w", err)
	}
	defer f.Close()
	return Parse(f, p)
}

// LoadDir reads every *.yaml under dir, in sorted order so results are
// deterministic. It uses fs.FS so tests can supply an in-memory tree
// (byob-interfaces.3).
func LoadDir(fsys fs.FS, dir string) ([]*File, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, fmt.Errorf("read mapping dir: %w", err)
	}
	// A mapping file that is not read produces no facts and says nothing about
	// why, so an unexpected file is an error rather than a skip. Accept both
	// YAML spellings; refuse anything else.
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
			return nil, cmdutil.WithHint(
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
		parsed, err := Parse(r, full)
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
				return nil, &ParseError{Path: full, RuleID: rule.ID,
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

// Parse decodes and validates a rule file.
func Parse(r io.Reader, p string) (*File, error) {
	var f File
	dec := yaml.NewDecoder(r)
	// Unknown fields are errors: a typo'd key would otherwise be silently
	// ignored, and a rule that quietly lost its `omitted_rows` is exactly the
	// silent mismapping this schema exists to prevent.
	dec.KnownFields(true)
	if err := dec.Decode(&f); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, &ParseError{Path: p, Msg: "file is empty"}
		}
		return nil, &ParseError{Path: p, Msg: err.Error()}
	}

	// The decoder is a stream. Decoding once and stopping would accept a file
	// whose second `---` document is silently discarded — the same class of
	// quiet loss that KnownFields exists to prevent, one level up.
	var extra File
	if err := dec.Decode(&extra); err == nil {
		return nil, cmdutil.WithHint(
			&ParseError{Path: p, Msg: "file contains more than one YAML document"},
			"put each document in its own file; every rule in a file must "+
				"belong to the doc_id declared at the top")
	} else if !errors.Is(err, io.EOF) {
		return nil, &ParseError{Path: p, Msg: err.Error()}
	}
	f.Path = p
	if err := f.validate(); err != nil {
		return nil, err
	}
	return &f, nil
}

// Validate checks a rule file's internal consistency. Parse calls it, so a
// loaded file is already validated; it is exported for callers that build a
// File in memory, which would otherwise skip every check the schema relies on.
func (f *File) Validate() error { return f.validate() }

func (f *File) validate() error {
	errf := func(ruleID, field, format string, args ...any) error {
		return &ParseError{Path: f.Path, RuleID: ruleID, Field: field,
			Msg: fmt.Sprintf(format, args...)}
	}

	switch {
	case f.SchemaVersion == 0:
		return errf("", "schema_version", "is required (want %d)", SchemaVersion)
	case f.SchemaVersion > SchemaVersion:
		return cmdutil.WithHint(
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
	return checkColumnGrids([]*File{f})
}

type errFunc func(ruleID, field, format string, args ...any) error

func validateRule(r *Rule, errf errFunc) error {
	if !r.Kind.valid() {
		return errf(r.ID, "kind", "got %q, want one of revenue, expenditure, "+
			"transfer_in, transfer_out, fund_balance", r.Kind)
	}
	if !r.Basis.valid() {
		return errf(r.ID, "basis", "got %q, want one of adopted, revised, actual, "+
			"audited, projected", r.Basis)
	}
	if r.Units == "" {
		return cmdutil.WithHint(
			errf(r.ID, "units", "is required"),
			"units come from the table's caption, not the token: \"71.8\" is "+
				"meaningless until you know the schedule is printed in millions")
	}
	if _, _, ok := unitsValid(r.Units); !ok {
		return errf(r.ID, "units", "got %q, want one of dollars, thousands, millions", r.Units)
	}
	if len(r.Parts) == 0 {
		return errf(r.ID, "parts", "is empty; a rule must name at least one page")
	}
	if len(r.Rows) == 0 {
		return errf(r.ID, "rows", "is empty")
	}

	rowIndex := map[string]bool{}
	for i, row := range r.Rows {
		if row.Label == "" {
			return errf(r.ID, "rows", "row %d has no label", i)
		}
		if row.LabelTail != "" && strings.TrimSpace(row.LabelTail) != row.LabelTail {
			return errf(r.ID, "rows",
				"row %q: label_tail %q has leading or trailing whitespace",
				row.Label, row.LabelTail)
		}
		if row.LabelTail != "" && strings.TrimSpace(row.LabelTail) == "" {
			return errf(r.ID, "rows", "row %q: label_tail is blank", row.Label)
		}
		if rowIndex[row.Identity()] {
			return cmdutil.WithHint(
				errf(r.ID, "rows", "duplicate row label %q", row.PrintedLabel()),
				"row labels are positional identities; two rows cannot share one")
		}
		rowIndex[row.Identity()] = true
		if !row.Sign.valid() {
			return errf(r.ID, "rows", "row %q: sign %q, want positive or contra",
				row.Label, row.Sign)
		}
		// A row with no classification would emit facts into an empty
		// category, where they either vanish from the breakdown or silently
		// merge with every other uncategorised row.
		if !row.Skip && row.Category == "" && row.Department == "" {
			return cmdutil.WithHint(
				errf(r.ID, "rows", "row %q has neither category nor department", row.Label),
				"add a category, or skip: true if the row is a subtotal that "+
					"would double-count")
		}
	}
	if r.TotalRow != "" && rowIndex[r.TotalRow] {
		return cmdutil.WithHint(
			errf(r.ID, "total_row", "%q is also listed in rows", r.TotalRow),
			"the total row is what the mapped rows are checked against; "+
				"including it in rows would double-count it")
	}

	pages := map[int]bool{}
	for i := range r.Parts {
		p := &r.Parts[i]
		if p.Page <= 0 {
			return errf(r.ID, "parts", "part %d has no page", i)
		}
		if pages[p.Page] {
			return errf(r.ID, "parts", "page %d listed twice", p.Page)
		}
		pages[p.Page] = true
		if len(p.Columns) == 0 {
			return errf(r.ID, fmt.Sprintf("parts[page %d].columns", p.Page), "is empty")
		}
		// Columns are positional identities exactly as rows are, so duplicates
		// must be rejected for the same reason. A repeated column is easy to
		// write (four columns where only the fiscal year differs, and one
		// does not get bumped) and impossible to detect downstream: the column
		// COUNT still matches the page, so the value-count assertion passes
		// and the second year's figures are emitted as conflicting facts for
		// the first.
		cols := map[Column]bool{}
		for j, c := range p.Columns {
			if c.Basis != "" && !c.Basis.valid() {
				return errf(r.ID, fmt.Sprintf("parts[page %d].columns[%d].basis", p.Page, j),
					"got %q", c.Basis)
			}
			if c.Skip {
				continue
			}
			if c.FiscalYear == 0 {
				return errf(r.ID, fmt.Sprintf("parts[page %d].columns[%d]", p.Page, j),
					"needs a fiscal_year (or skip: true)")
			}
			if cols[c] {
				return cmdutil.WithHint(
					errf(r.ID, fmt.Sprintf("parts[page %d].columns[%d]", p.Page, j),
						"duplicates an earlier column (fund_group=%q fund=%d fiscal_year=%d basis=%q)",
						c.FundGroup, c.Fund, c.FiscalYear, c.Basis),
					"columns are positional identities; check whether a fiscal_year "+
						"or fund_group was left unchanged when the column was copied")
			}
			cols[c] = true
		}
		// A block boundary must not be a figure the totals check is meant to
		// verify: anchoring the block on the number proves nothing, and the
		// anchor vanishes the moment the city republishes with a revised
		// total. Anchor on a label instead.
		for _, field := range []struct{ name, val string }{
			{"section", p.Section}, {"stop_at", p.StopAt},
		} {
			if field.val == "" {
				continue
			}
			if _, err := amount.Parse(field.val, r.Units); err == nil {
				return cmdutil.WithHint(
					errf(r.ID, fmt.Sprintf("parts[page %d].%s", p.Page, field.name),
						"%q is a currency amount", field.val),
					"anchor the block on a label such as \"TOTAL REVENUES:\"; "+
						"a figure changes when the document is revised, and using "+
						"a total as the boundary makes the totals check circular")
			}
		}

		if err := validatePartAnchors(r, p, errf); err != nil {
			return err
		}

		for _, label := range p.OmittedRows {
			if !rowIndex[label] {
				return cmdutil.WithHint(
					errf(r.ID, fmt.Sprintf("parts[page %d].omitted_rows", p.Page),
						"%q is not one of this rule's rows", label),
					"omitted_rows names rows that exist in the rule but are "+
						"absent from this page")
			}
		}

		// A declared discrepancy is a claim about one column of one page, so
		// every way of writing one that does not say that is refused here
		// rather than reaching CheckTotals.
		deltas := map[int]bool{}
		for j, d := range p.StatedTotalDeltas {
			field := fmt.Sprintf("parts[page %d].stated_total_deltas[%d]", p.Page, j)
			if d.Column < 1 || d.Column > len(p.Columns) {
				return errf(r.ID, field, "column %d is out of range; this part has %d columns",
					d.Column, len(p.Columns))
			}
			if p.Columns[d.Column-1].Skip {
				return cmdutil.WithHint(
					errf(r.ID, field, "column %d is skipped", d.Column),
					"a skipped column produces no facts and is never totalled, "+
						"so there is nothing for a delta to describe")
			}
			if deltas[d.Column] {
				return errf(r.ID, field, "column %d already has a delta", d.Column)
			}
			deltas[d.Column] = true
			if d.Cents == 0 {
				return cmdutil.WithHint(
					errf(r.ID, field, "delta_cents is zero"),
					"a zero delta is what an undeclared column already asserts; "+
						"remove the entry")
			}
			if strings.TrimSpace(d.Note) == "" {
				return cmdutil.WithHint(
					errf(r.ID, field, "note is required"),
					"the note is why this is a declaration and not a tolerance: "+
						"say what was checked and why the difference is the "+
						"document's rather than ours")
			}
		}
	}

	// LabelsFrom must point at another part of the same rule, and that part
	// must itself carry labels — a chain of continuations has no anchor.
	for i := range r.Parts {
		p := &r.Parts[i]
		if p.LabelsFrom == 0 {
			continue
		}
		if p.LabelsFrom == p.Page {
			return errf(r.ID, fmt.Sprintf("parts[page %d].labels_from", p.Page),
				"points at itself")
		}
		src := r.LabelledPart(p)
		if src == nil {
			return cmdutil.WithHint(
				errf(r.ID, fmt.Sprintf("parts[page %d].labels_from", p.Page),
					"page %d is not a part of this rule", p.LabelsFrom),
				"a continuation page borrows row labels from another page in "+
					"the same rule")
		}
		if src.LabelsFrom != 0 {
			return errf(r.ID, fmt.Sprintf("parts[page %d].labels_from", p.Page),
				"page %d is itself a continuation; labels must come from a page that has them",
				p.LabelsFrom)
		}
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
		return cmdutil.WithHint(
			errf(r.ID, field("section_ordinal"), "is set but section is empty"),
			"section_ordinal picks which occurrence of section starts the "+
				"block, so it means nothing without one")
	}
	if len(p.ColumnHeaders) == 0 {
		return nil
	}
	// One entry per column, including skipped ones. A short list would build a
	// grid one band narrower than the page and file every figure right of the
	// missing column one place left -- silently, with the value count still
	// matching.
	if len(p.ColumnHeaders) != len(p.Columns) {
		return cmdutil.WithHint(
			errf(r.ID, field("column_headers"), "has %d entries but the part has %d columns",
				len(p.ColumnHeaders), len(p.Columns)),
			"name the printed header of every column left to right, including "+
				"any marked skip: true; the list is what says where each column "+
				"sits on the page")
	}
	for i, h := range p.ColumnHeaders {
		if strings.TrimSpace(h) == "" {
			return cmdutil.WithHint(
				errf(r.ID, field("column_headers"), "entry %d is empty", i+1),
				"a header is matched by joining the words printed on the header "+
					"line, so no page can produce an empty one")
		}
		// Deliberately NOT the circularity argument the section/stop_at refusal
		// makes: a header is not a total. A header that is a figure would match
		// a DATA row, and a grid built from a data row files that row perfectly
		// and every other row by luck.
		if _, err := amount.Parse(h, r.Units); err == nil {
			return cmdutil.WithHint(
				errf(r.ID, field("column_headers"), "entry %d is a currency amount: %q",
					i+1, h),
				"name more of the printed header -- \"FY 2026\" rather than "+
					"\"2026\" -- because a header that is a figure matches a data "+
					"row as readily as the header line")
		}
	}
	return nil
}

// checkColumnGrids checks the two things no per-part rule can see: that every
// part reading a page agrees about that page's column grid, and that they all
// opt into it or none do.
//
// The published spine has five rules, each with a part on p66 and a part on p67,
// so one page's header list is written out five times. A copy that drifted would
// build a DIFFERENT grid for the same page in one rule than in another, and both
// reads would go on tying against their own printed totals -- confident wrong
// figures carrying working-looking provenance, which is the class this guard
// exists to prevent.
//
// The all-or-none half closes the same hole one level up. A page where four
// parts declare a grid and the fifth does not would otherwise validate, leaving
// that part silently unguarded with nothing saying so -- which is exactly why
// the resolver's doc interface requires its geometry method rather than
// discovering it by type assertion.
//
// Grouping is by (doc_id, page) and not by file, because a document may be
// mapped by more than one file and nothing else compares them: LoadDir dedupes
// rule ids and never looks at doc_id.
//
// A page that printed two schedules side by side with different grids would be
// refused here. No page in this corpus does yet; when one appears, this is the
// check to relax.
func checkColumnGrids(files []*File) error {
	type declaration struct {
		path    string
		rule    string
		headers []string
	}
	type key struct {
		docID string
		page  int
	}

	declared := map[key]declaration{}
	for _, f := range files {
		for i := range f.Rules {
			r := &f.Rules[i]
			for j := range r.Parts {
				p := &r.Parts[j]
				k := key{docID: f.DocID, page: p.Page}
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
					return cmdutil.WithHint(
						&ParseError{Path: f.Path, RuleID: r.ID,
							Field: fmt.Sprintf("parts[page %d].column_headers", p.Page),
							Msg: fmt.Sprintf("is %q, but rule %q in %s declares %q for the same page",
								p.ColumnHeaders, prev.rule, prev.path, prev.headers)},
						"a page has one column grid; two parts describing it "+
							"differently would read the same figures into different "+
							"columns and each would still tie against its own total")
				}
			}
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
				prev, ok := declared[key{docID: f.DocID, page: p.Page}]
				if !ok {
					continue
				}
				return cmdutil.WithHint(
					&ParseError{Path: f.Path, RuleID: r.ID,
						Field: fmt.Sprintf("parts[page %d]", p.Page),
						Msg: fmt.Sprintf("declares no column_headers, but rule %q in %s "+
							"declares %q for the same page", prev.rule, prev.path, prev.headers)},
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
func unitsValid(u amount.Units) (int64, int, bool) {
	switch u {
	case amount.Dollars, amount.Thousands, amount.Millions:
		// Parse a zero to confirm the unit is wired up on the amount side too.
		if _, err := amount.Parse("0", u); err != nil {
			return 0, 0, false
		}
		return 0, 0, true
	}
	return 0, 0, false
}
