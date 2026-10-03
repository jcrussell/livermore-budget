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

// parseError reports a problem in a rule file, located precisely enough to fix
// without hunting. Rule files are hand-written, so the error message is the
// primary interface for most of the people who will ever see this package.
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
	// #nosec G304 -- p is a mapping file path chosen by the operator running
	// fisc, not attacker-controlled input; refusing variable paths here would
	// mean hard-coding the mapping directory.
	f, err := os.Open(p)
	if err != nil {
		return nil, fmt.Errorf("open rule file: %w", err)
	}
	defer f.Close()
	return parse(f, p)
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
	// Unknown fields are errors: a typo'd key would otherwise be silently
	// ignored, and a rule that quietly lost its `omitted_rows` is exactly the
	// silent mismapping this schema exists to prevent.
	dec.KnownFields(true)
	if err := dec.Decode(&f); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, &parseError{Path: p, Msg: "file is empty"}
		}
		return nil, &parseError{Path: p, Msg: err.Error()}
	}

	// The decoder is a stream. Decoding once and stopping would accept a file
	// whose second `---` document is silently discarded — the same class of
	// quiet loss that KnownFields exists to prevent, one level up.
	var extra File
	if err := dec.Decode(&extra); err == nil {
		return nil, cmdutil.WithHint(
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

// Validate checks a rule file's internal consistency. Parse calls it, so a
// loaded file is already validated; it is exported for callers that build a
// File in memory, which would otherwise skip every check the schema relies on.
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
	if err := validateRollups(f, errf); err != nil {
		return err
	}
	return checkColumnGrids([]*File{f})
}

// validateRollups checks the declarations that a printed total covers several
// rules, or that the document prints one this rule structure cannot assert.
//
// Everything checkable without the pages is checked here. That the anchor
// occurs on the page, occurs once, and that the covered rules' totals actually
// add up are claims about the document and are made when the rollup resolves.
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
			return cmdutil.WithHint(
				errf("", "rollups", "%q is also a rule id", ro.ID),
				"a rollup and a rule are reported side by side, so one id must "+
					"not name both")
		}

		// Either it asserts or it says why it cannot. Neither would be the
		// silence this whole field exists to refuse; both would be a claim
		// that contradicts itself.
		// The declaration is compared trimmed here and untrimmed by the build,
		// so a whitespace-only reason would pass validation as "no reason" and
		// then suppress the check as "a reason". Refusing it outright keeps
		// one spelling of empty.
		if ro.Unassertable != "" && strings.TrimSpace(ro.Unassertable) == "" {
			return errf("", field("unassertable"), "is whitespace; omit it or give the reason")
		}
		asserts, declines := len(ro.Covers) > 0, ro.Unassertable != ""
		// CHECKED BEFORE THE `declines` SHORT-CIRCUIT BELOW, which is the whole
		// reason it is here and not in validateRollupKinds. That function runs
		// after `if declines { continue }`, so an unassertable rollup never
		// reached it and `kinds:` on one was accepted in silence -- and the
		// corpus's only unassertable rollup is p140's "Total Sources", the
		// mixed-kind line Rollup.Kinds' own doc comment is built around. The
		// most likely place for the field to be typed is the one place nothing
		// read it.
		if len(ro.Kinds) > 0 && declines {
			return cmdutil.WithHint(
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
			return cmdutil.WithHint(
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

		// A ROLLUP MAY COVER ONE RULE. This used to be refused, on the
		// reasoning that "a total covering a single rule is that rule's
		// total_row wearing a different name". Budget Book pp.167-170 show
		// that is false: six of the eleven <DEPARTMENT> TOTAL rows sit over a
		// department with exactly one division, and each is a SECOND PRINTED
		// LINE with its own figures -- CITY COUNCIL TOTAL on p0167:15, three
		// lines below City Council's own Total on :13. Refusing them left six
		// printed totals asserted by nothing, which is the one answer fisc-3bl
		// calls unacceptable.
		//
		// What the old rule was reaching for is the guard below: a rollup may
		// not name the same printed line as a covered rule's total. That is a
		// claim about the LINE and not about the number of rules, and the
		// string half of it is checked here. The page half -- same page, same
		// line -- needs the pages and is checked in CheckRollup, for the reason
		// the column guard below cannot be closed here either.
		covered := map[string]bool{}
		var first *Rule
		for _, id := range ro.Covers {
			rule := byID[id]
			if rule == nil {
				return errf("", field("covers"), "no rule %q in this file", id)
			}
			if covered[id] {
				return cmdutil.WithHint(
					errf("", field("covers"), "%q is listed twice", id),
					"a rule counted twice inflates the sum by its own total")
			}
			covered[id] = true
			// The chain's first link. Without a printed total of its own a
			// covered rule contributes nothing to sum, and the rollup would
			// silently be a check over fewer rules than it names.
			if rule.TotalRow == "" {
				return cmdutil.WithHint(
					errf("", field("covers"), "rule %q declares no total_row", id),
					"a rollup sums the totals its covered rules PRINT, so every "+
						"one of them must print one")
			}
			if rule.TotalRow == ro.TotalRow {
				return cmdutil.WithHint(
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
			// SCOPE IS REFUSED WHERE KIND IS DECLARED, and the asymmetry is the
			// point rather than an oversight.
			//
			// A scope is a claim about WHICH MONEY a rule's facts are, and two
			// scopes over one printed total is fisc-u2v's doubling mechanism
			// raised to the rollup: pp.85-125 and pp.66-67 are the same
			// expenditure counted two ways, so a rollup summing a detail-scope
			// total into a spine-scope one produces a figure that adds up and
			// means nothing. A tie there says the arithmetic worked, not that
			// the claim is true, and no printed line in this corpus wants it.
			//
			// It needs no pages and no registry, so it is refused here rather
			// than in CheckRollup.
			if first.Scope != rule.Scope {
				return cmdutil.WithHint(
					errf("", field("covers"),
						"rule %q is scope %q and rule %q is scope %q",
						first.ID, first.Scope, id, rule.Scope),
					"a rollup adds totals that are the same money counted once; "+
						"two scopes over one printed total is the same money "+
						"counted twice, which ties and still misstates the city")
			}
			if !slices.Equal(first.Parts[0].Columns, rule.Parts[0].Columns) {
				return cmdutil.WithHint(
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
// than one kind.
//
// It is the mirror of the scope refusal above and deliberately the opposite
// answer. Two scopes over one total is always wrong; two KINDS is sometimes what
// the page prints -- p140's "Total Sources" is revenue plus transfers in -- so
// this requires the author to say so rather than refusing them the shape.
//
// Both refusals here are of a declaration that asserts nothing, which is
// validateTotalRowKinds' discipline: a list that restates what the rules already
// agree on records a belief, and one that is absent where they disagree lets a
// mixed-kind rollup pass as an accident.
func validateRollupKinds(ro *Rollup, byID map[string]*Rule, field func(string) string,
	errf errFunc) error {

	span := map[Kind]bool{}
	for _, id := range ro.Covers {
		span[byID[id].Kind] = true
	}
	declared := map[Kind]bool{}
	for _, k := range ro.Kinds {
		if !k.valid() {
			return errf("", field("kinds"), "%q is not one of the five kinds", k)
		}
		if declared[k] {
			return errf("", field("kinds"), "%q is listed twice", k)
		}
		declared[k] = true
	}
	if len(span) == 1 {
		if len(ro.Kinds) > 0 {
			return cmdutil.WithHint(
				errf("", field("kinds"), "is declared and every covered rule is kind %q",
					sortedKinds(span)[0]),
				"the list exists to say that a printed total spans more than one "+
					"kind; where they agree it restates them and cannot fail")
		}
		return nil
	}
	if len(ro.Kinds) == 0 {
		return cmdutil.WithHint(
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

func sortedKinds(set map[Kind]bool) []Kind {
	out := make([]Kind, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

func describeKinds(ks []Kind) string {
	parts := make([]string, len(ks))
	for i, k := range ks {
		parts[i] = fmt.Sprintf("%q", k)
	}
	return strings.Join(parts, ", ")
}

type errFunc func(ruleID, field, format string, args ...any) error

func validateRule(r *Rule, errf errFunc) error {
	if !r.Kind.valid() {
		return errf(r.ID, "kind", "got %q, want one of %s", r.Kind, kindList())
	}
	if !r.Basis.Valid() {
		return errf(r.ID, "basis", "got %q, want one of %s", r.Basis, basisList())
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

	// The columns' categories and kinds are validated before any row, because
	// the sign check below reads a column-category row's kinds off them.
	byColumn := r.categoryOnColumns()
	for i := range r.Parts {
		for j := range r.Parts[i].Columns {
			if err := validateColumnClass(r, &r.Parts[i], j, byColumn, errf); err != nil {
				return err
			}
		}
	}

	// rowIndex is keyed on Identity(), which is what a row IS within its rule;
	// tailed collects the two-anchor rows by their first anchor alone, so a
	// declaration that names only that anchor can be told "which one?" rather
	// than "no such row". printed guards the other end: row_label is what a
	// fact publishes and what its id is hashed from (see fact.MakeID), so two
	// rows may share neither an identity nor a printed form.
	//
	// Except two SKIPPED rows: neither publishes, so no fact id is hashed from
	// either, and rows are matched in the order the rule lists them, so each
	// reads its own line. Budget Book p230 prints fund 611's projects twice,
	// once under its federal grant and once under its state grant, on lines
	// identical but for the figures. An omitted_rows entry naming such a row
	// could not say which one, so it is refused below.
	rowIndex := map[string]bool{}
	tailed := map[string][]string{}
	printed := map[string]bool{}
	skipped := map[string]bool{}
	repeated := map[string]bool{}
	for i, row := range r.Rows {
		if row.Label == "" {
			return errf(r.ID, "rows", "row %d has no label", i)
		}
		if row.LabelTail != "" {
			// Blank first: a whitespace-only tail IS blank, and reporting it
			// as stray whitespace would send the author looking for a
			// character to trim rather than for the anchor they meant to name.
			if strings.TrimSpace(row.LabelTail) == "" {
				return errf(r.ID, "rows", "row %q: label_tail is blank", row.Label)
			}
			if strings.TrimSpace(row.LabelTail) != row.LabelTail {
				return errf(r.ID, "rows",
					"row %q: label_tail %q has leading or trailing whitespace",
					row.Label, row.LabelTail)
			}
		}
		switch {
		case rowIndex[row.Identity()] && row.Skip && skipped[row.Identity()]:
			repeated[row.Identity()] = true
		case rowIndex[row.Identity()]:
			return cmdutil.WithHint(
				errf(r.ID, "rows", "duplicate row label %q", row.PrintedLabel()),
				"row labels are positional identities; two rows cannot share one "+
					"unless both are skip: true")
		case printed[row.PrintedLabel()]:
			return cmdutil.WithHint(
				errf(r.ID, "rows", "two rows print as %q", row.PrintedLabel()),
				"the two rows split that text differently between label and "+
					"label_tail, so they are two identities to the resolver "+
					"and one row_label to every reader of facts.jsonl, where "+
					"a fact's id is hashed from row_label")
		}
		if !rowIndex[row.Identity()] {
			skipped[row.Identity()] = row.Skip
		}
		rowIndex[row.Identity()] = true
		printed[row.PrintedLabel()] = true
		if row.LabelTail != "" {
			tailed[row.Label] = append(tailed[row.Label], row.PrintedLabel())
		}
		if !row.Sign.valid() {
			return errf(r.ID, "rows", "row %q: sign %q, want positive, contra or netted",
				row.Label, row.Sign)
		}
		if err := validateRowClass(r, row, byColumn, errf); err != nil {
			return err
		}
		if row.Quantity != "" {
			if row.Quantity == QuantityAmount {
				return cmdutil.WithHint(
					errf(r.ID, "rows", "row %q: quantity %q is the default",
						row.Label, row.Quantity),
					"an undeclared row already parses amounts; remove the declaration")
			}
			if !row.Quantity.valid() {
				return errf(r.ID, "rows", "row %q: quantity %q, want one of %s",
					row.Label, row.Quantity, quantityList())
			}
		}
		// ONE refusal for any declaration about a row's facts on a row that
		// has none -- skipped, non-amount, or under columns that all skip --
		// held to Row.Publishes through kindsOf, so it cannot disagree with
		// the facts. Above checkCounterpart, because a counterpart's own
		// defects are moot on a row that should not carry one.
		publishes := r.kindsOf(row)
		if len(publishes) == 0 {
			var declared string
			switch {
			case row.Sign != "":
				declared = "sign " + string(row.Sign)
			case row.Counterpart != nil:
				declared = "counterpart"
			}
			if declared != "" {
				return cmdutil.WithHint(
					errf(r.ID, "rows", "row %q: %s on a row that publishes no cell",
						row.Label, declared),
					"a sign says how a row's facts are printed and a counterpart "+
						"fans one into two; this row publishes no cell, so remove "+
						"the declaration")
			}
		}
		if err := checkCounterpart(r, row, errf); err != nil {
			return err
		}
		// SignNetted says a row is printed against its KIND's direction, so it
		// is meaningless on a kind that has no direction, and
		// fact-transfer-orientation-is-declared only witnesses transfers. Left
		// unvalidated, `sign: netted` on a revenue row publishes facts nothing
		// checks. Refused here rather than widened there: the check is right to
		// be narrow, because a negative revenue or fund balance is a magnitude
		// and not an orientation.
		//
		// BOTH ENDS, because fact.FromValues builds the counterpart leg from a
		// COPY of this row -- it overrides Category and Kind and inherits
		// everything else, Sign included. Inheriting it is right when both ends
		// are transfers: one printed figure, one orientation, and a negative
		// transfer_in is against its direction exactly as the transfer_out is.
		// It is wrong the moment the far end is not a transfer, which is how a
		// netted revenue fact reached the store past the near-row check alone.
		//
		// IT SITS BELOW BOTH KIND VALIDATORS -- row.Kind's above and
		// checkCounterpart's, which refuses a counterpart kind that is not one of
		// the five. Above either, {sign: netted, kind: income} is reported as a sign
		// on kind "income" and a mistyped counterpart kind as `sign netted on kind
		// "incom"` -- both naming the wrong field to whoever has to fix the YAML.
		if row.Sign == SignNetted {
			ends := publishes
			if row.Counterpart != nil {
				ends = append(ends, row.Counterpart.Kind)
			}
			for _, k := range ends {
				if k == KindTransferIn || k == KindTransferOut {
					continue
				}
				return cmdutil.WithHint(
					errf(r.ID, "rows", "row %q: sign netted on kind %q", row.Label, k),
					"netted says the document prints this row against its kind's "+
						"direction, which only transfer_in and transfer_out have; "+
						"a counterpart leg inherits the row's sign")
			}
		}
	}
	// The guard is against the row's LABEL, and deliberately not against
	// rowIndex: a two-anchor row's identity carries a \x1f, so a total_row --
	// which is a plain string the author types and the resolver finds in the
	// page text -- can never equal one, and testing the index silently
	// disabled this check for every row carrying a label_tail (fisc-gtv). The
	// printed form is tested too, because that is the other spelling an author
	// might reach for.
	if r.TotalRow != "" {
		for _, row := range r.Rows {
			if r.TotalRow != row.Label && r.TotalRow != row.PrintedLabel() {
				continue
			}
			return cmdutil.WithHint(
				errf(r.ID, "total_row", "%q is also listed in rows", r.TotalRow),
				"the total row is what the mapped rows are checked against; "+
					"including it in rows would double-count it")
		}
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
		// A wrapped label is matched against the FULLY TRIMMED text of a gap,
		// so a declaration carrying its own whitespace could never match and
		// would fail later as a stale declaration rather than here as the typo
		// it is. Blank and duplicate entries are refused for the same reason.
		//
		// A LABELS_FROM PART CANNOT WRAP A LABEL, because it carries none.
		// Row identity there is positional: the part routes to positionalValues,
		// which never reads this field and never staleness-checks it, so a
		// declaration was ACCEPTED AND INERT -- the one shape a declaration in
		// this repository must not have. Every other one fails when it stops
		// being true: a stated_total_delta that now ties exactly, an omitted-row
		// count that no longer matches, a wrapped label the page stopped
		// wrapping. Measured on the production mapping (fisc-ekj): adding a
		// fragment that appears nowhere on p67 built cleanly with byte-identical
		// facts, and five parts of the shipped rule file take labels_from.
		//
		// Refusing is right rather than honouring it. A wrapped label is a
		// property of the part that CARRIES the label, and this part has
		// delegated that to another page -- which is where the declaration
		// belongs.
		//
		// A WRAPPED LABEL IS A LABEL, and the amount refusal is what holds it
		// to that: without it, wrapped_labels: ["0.0"] on ACFR p41's revenue
		// block parses, resolves all ten rows and ties to the printed total
		// exactly, publishing the page's orphan figure under a declaration that
		// the page wrapped a label onto its own line, which it did not.
		// unmapped_text (fisc-hcus) is that figure's honest home, and is worth
		// nothing while the dishonest one still works.
		//
		// Every entry the committed rules declare is refused by amount.Parse,
		// which loading them re-measures, so this costs them nothing.
		if err := validateGapLines(r, p, "wrapped_labels", p.WrappedLabels, errf,
			"a wrapped label is a claim about the page that PRINTS the "+
				"label; declare it on that part, where it is checked",
			"wrapped_labels declares that the page wrapped a row's LABEL "+
				"onto its own line; a figure there belongs to no row and "+
				"is declared with unmapped_text, which says so"); err != nil {
			return err
		}
		// headings shares every arm above, for the same reasons: it is matched
		// the same way, read by the same gap test and staleness-checked the
		// same way.
		if err := validateGapLines(r, p, "headings", p.Headings, errf,
			"a heading is a claim about the page that prints it between "+
				"labelled rows; a label-less part reads no gaps",
			"headings declares a section heading printed between rows; a "+
				"figure there belongs to no row and is declared with "+
				"unmapped_text, which says so"); err != nil {
			return err
		}
		// One line, one claim: a line declared both a heading and a wrapped
		// label would say two incompatible things about what the page printed.
		for _, h := range p.Headings {
			if slices.Contains(p.WrappedLabels, h) {
				return errf(r.ID, fmt.Sprintf("parts[page %d].headings", p.Page),
					"%q is also declared in wrapped_labels; a line is a heading or "+
						"a wrapped label, not both", h)
			}
		}
		// unmapped_text is the mirror of the block above, and every arm is the
		// mirror of one of its arms: same trimmed matching, same blank and
		// duplicate refusals, same labels_from refusal for the same reason
		// (fisc-ekj -- a positional part reads no gaps, so the declaration
		// would be accepted and inert). The one INVERTED arm is amount.Parse:
		// a wrapped label must not be a figure and this must be one.
		if p.LabelsFrom != 0 && len(p.UnmappedText) > 0 {
			return cmdutil.WithHint(
				errf(r.ID, fmt.Sprintf("parts[page %d].unmapped_text", p.Page),
					"is declared on a part whose labels_from takes its row labels from page %d",
					p.LabelsFrom),
				"unmapped_text is checked against the gaps between a part's own "+
					"labelled rows; a label-less part reads none, so the "+
					"declaration would be accepted and never looked at")
		}
		seenUnmapped := map[string]bool{}
		for _, u := range p.UnmappedText {
			field := fmt.Sprintf("parts[page %d].unmapped_text", p.Page)
			if strings.TrimSpace(u.Text) == "" {
				return errf(r.ID, field, "has an entry with no text")
			}
			if strings.TrimSpace(u.Text) != u.Text {
				return errf(r.ID, field,
					"%q has leading or trailing whitespace; it is matched against "+
						"the trimmed text of the gap", u.Text)
			}
			if strings.TrimSpace(u.Note) == "" {
				return cmdutil.WithHint(
					errf(r.ID, field, "%q has no note", u.Text),
					"the note is why this is a declaration and not a silent skip; "+
						"say what the page prints there and why it belongs to no row")
			}
			if _, err := amount.Parse(u.Text, r.Units); err != nil {
				return cmdutil.WithHint(
					errf(r.ID, field, "%q is not a figure: %v", u.Text, err),
					"unmapped_text declares a FIGURE the page prints that belongs "+
						"to no row; text the page wrapped from a row's label is "+
						"declared with wrapped_labels, which says that instead")
			}
			if seenUnmapped[u.Text] {
				return errf(r.ID, field, "%q is listed twice", u.Text)
			}
			seenUnmapped[u.Text] = true
		}
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
			if c.Basis != "" && !c.Basis.Valid() {
				return errf(r.ID, fmt.Sprintf("parts[page %d].columns[%d].basis", p.Page, j),
					"got %q", c.Basis)
			}
			if c.Quantity != "" {
				if c.Quantity == QuantityAmount {
					return cmdutil.WithHint(
						errf(r.ID, fmt.Sprintf("parts[page %d].columns[%d]", p.Page, j),
							"quantity %q is the default", c.Quantity),
						"an undeclared column already parses amounts; remove the declaration")
				}
				if !c.Quantity.valid() {
					return errf(r.ID, fmt.Sprintf("parts[page %d].columns[%d]", p.Page, j),
						"quantity %q, want one of %s", c.Quantity, quantityList())
				}
				// A rule's total_row is read as one run of amounts, one per
				// column (see amountRun), and a table whose columns are not
				// all amounts prints no such line. Refused here so the
				// mismatch cannot surface later as a missing-anchor error.
				if r.TotalRow != "" {
					return cmdutil.WithHint(
						errf(r.ID, fmt.Sprintf("parts[page %d].columns[%d]", p.Page, j),
							"parses %s, but the rule declares total_row %q",
							c.Quantity, r.TotalRow),
						"a stated-totals line prints one amount per column; a "+
							"table with a non-amount column has no such line to read")
				}
				// A non-amount column never publishes, so like a skipped one
				// it needs no fiscal_year and no distinct identity.
				continue
			}
			if c.Skip {
				continue
			}
			if c.FiscalYear == 0 {
				return errf(r.ID, fmt.Sprintf("parts[page %d].columns[%d]", p.Page, j),
					"needs a fiscal_year (or skip: true)")
			}
			// KIND IS NOT IN THE KEY because it is not in the fact id: two
			// columns of one row differing only in kind hash to one id.
			key := c
			key.Kind = ""
			if cols[key] {
				return cmdutil.WithHint(
					errf(r.ID, fmt.Sprintf("parts[page %d].columns[%d]", p.Page, j),
						"duplicates an earlier column (fund_group=%q fund=%d fiscal_year=%d "+
							"basis=%q category=%q)",
						c.FundGroup, c.Fund, c.FiscalYear, c.Basis, c.Category),
					"columns are positional identities; check whether a fiscal_year, "+
						"fund_group or category was left unchanged when the column was "+
						"copied. A column's kind does not tell it apart: a fact's id is "+
						"not hashed over its kind")
			}
			cols[key] = true
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

		// An omitted_rows entry must name EXACTLY ONE row. Naming none is the
		// stale declaration this has always refused; naming more than one
		// would drop every row sharing a label, which is the same silent
		// mismapping the declaration exists to prevent, one page later. An
		// omitted_cells entry names its row the same way, and is held to the
		// same rule by the same function.
		namesOneRow := func(field string, j int, o omittedRow, notARow string) error {
			switch {
			case strings.TrimSpace(o.Label) == "":
				return errf(r.ID, field, "entry %d has no label", j)
			case strings.TrimSpace(o.LabelTail) != o.LabelTail:
				return errf(r.ID, field,
					"entry %d: label_tail %q has leading or trailing whitespace",
					j, o.LabelTail)
			}
			if repeated[o.identity()] {
				return errf(r.ID, field, "%q names rows the rule lists more than once, "+
					"so it cannot say which one the page omits", o.printedLabel())
			}
			if rowIndex[o.identity()] {
				return nil
			}
			if o.LabelTail == "" && len(tailed[o.Label]) > 0 {
				return cmdutil.WithHint(
					errf(r.ID, field, "%q names %d rows, which differ only in "+
						"their label_tail: %q", o.Label, len(tailed[o.Label]),
						tailed[o.Label]),
					"a row named by two anchors is named by both: "+
						"- {label: ..., label_tail: ...}")
			}
			return cmdutil.WithHint(
				errf(r.ID, field, "%q is not one of this rule's rows", o.printedLabel()),
				notARow)
		}
		field := fmt.Sprintf("parts[page %d].omitted_rows", p.Page)
		declared := map[string]bool{}
		for j, o := range p.OmittedRows {
			if err := namesOneRow(field, j, o, "omitted_rows names rows that exist "+
				"in the rule but are absent from this page"); err != nil {
				return err
			}
			if declared[o.identity()] {
				return errf(r.ID, field, "%q is declared twice", o.printedLabel())
			}
			declared[o.identity()] = true
		}
		if err := validateOmittedCells(r, p, namesOneRow, errf); err != nil {
			return err
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
			if q := p.Columns[d.Column-1].Quantity; q != "" {
				return cmdutil.WithHint(
					errf(r.ID, field, "column %d parses %s, not amounts", d.Column, q),
					"a non-amount column produces no facts and is never totalled, "+
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
		src := r.labelledPart(p)
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

	if err := validateTotalRowAbove(r, errf); err != nil {
		return err
	}
	if err := validateTotalRowTail(r, errf); err != nil {
		return err
	}
	if err := validatePrintedDecimals(r, errf); err != nil {
		return err
	}
	if err := validateTotalRowKinds(r, errf); err != nil {
		return err
	}
	if r.RowLabelsNameFunds && r.RowLabelsAreFundNumbers {
		return errf(r.ID, "row_labels_are_fund_numbers",
			"is declared with row_labels_name_funds; a label is a fund's name or its number, not both")
	}
	if err := validateRowLabelFunds(r, errf); err != nil {
		return err
	}
	if err := validateTotalSpansParts(r, errf); err != nil {
		return err
	}
	if err := validateSubtotals(r, errf); err != nil {
		return err
	}
	// Last, so a rule is refused for its own shape before a missing grain.
	return validateGrain(r, errf)
}

// validateClass checks one declaration of a category and a kind, wherever the
// rule carries them: on a row, or on a column. A declaration that publishes
// needs a category, which internal/check declines to require on purpose ("an
// absent value is the mapping's business").
func validateClass(r *Rule, field, owner, category string, kind Kind, publishes bool,
	errf errFunc, hint string) error {
	if kind != "" && !kind.valid() {
		return errf(r.ID, field, "%s: kind %q is not one of the five", owner, kind)
	}
	if publishes && category == "" {
		return cmdutil.WithHint(errf(r.ID, field, "%s has no category", owner), hint)
	}
	return nil
}

// validateRowClass holds a row to the axis its rule carries the category on.
//
// On the row axis EVERY ROW CARRIES A CATEGORY, and a department is a SECOND
// AXIS and not a substitute for one: pp.167-170 cross department against
// object category, so a department row still says what KIND of spending the
// figure is. A non-amount row is exempt exactly as a skipped one: it publishes
// nothing, so there is no fact for a category to classify.
func validateRowClass(r *Rule, row Row, byColumn bool, errf errFunc) error {
	if !byColumn {
		return validateClass(r, "rows", fmt.Sprintf("row %q", row.Label), row.Category,
			row.Kind, !row.Skip && row.Quantity == "", errf,
			"every row needs one, including a row that declares a department: "+
				"department is a second axis, not a substitute. Use skip: true if "+
				"the row is a subtotal that would double-count. On a page whose "+
				"columns are its budget lines, every column carries one instead")
	}
	for _, d := range []struct{ key, val string }{
		{"category", row.Category}, {"kind", string(row.Kind)},
	} {
		if d.val != "" {
			return cmdutil.WithHint(
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
// that publishes nothing classifies nothing, so a category or kind on one is
// a declaration no fact would ever read.
func validateColumnClass(r *Rule, p *Part, j int, byColumn bool, errf errFunc) error {
	c := p.Columns[j]
	field := fmt.Sprintf("parts[page %d]", p.Page)
	owner := fmt.Sprintf("columns[%d]", j)
	if !c.publishes() {
		if c.Category != "" || c.Kind != "" {
			return cmdutil.WithHint(
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
		return cmdutil.WithHint(
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
		return cmdutil.WithHint(
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
			return cmdutil.WithHint(errf(r.ID, field, "%q is a currency amount", e), amountHint)
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
		return cmdutil.WithHint(
			errf(r.ID, "grain", "is required on a rule that publishes facts"),
			"name the lattice level the table is printed at -- which of fund_group, "+
				"fund, department and category it has an axis for; internal/structure "+
				"checks the name against the facts the rule publishes")
	case !r.publishes() && r.Grain != "":
		return cmdutil.WithHint(
			errf(r.ID, "grain", "is %q, but every row or every column of this rule is "+
				"skipped or non-amount, so it publishes no fact", r.Grain),
			"a grain declared over zero facts cannot be checked against the store; "+
				"remove it until the rule publishes")
	}
	return nil
}

// publishes says whether any cell of this rule can become a fact: a row that
// is neither skipped nor a non-amount quantity, in a column that is neither.
func (r *Rule) publishes() bool {
	row := false
	for i := range r.Rows {
		if !r.Rows[i].Skip && r.Rows[i].Quantity == "" {
			row = true
			break
		}
	}
	if !row {
		return false
	}
	for i := range r.Parts {
		for _, c := range r.Parts[i].Columns {
			if c.publishes() {
				return true
			}
		}
	}
	return false
}

// validatePrintedDecimals checks the preconditions of a document-derived
// tolerance. What it cannot check is the claim itself -- whether the page really
// prints two decimals is a question about the document, and the resolver
// answers it against the tokens it read.
//
// See Rule.PrintedDecimals for why each of these is a refusal rather than a
// convention, and fisc-1wr.2 for the tier.
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
		return cmdutil.WithHint(
			errf(r.ID, "printed_decimals", "is %d, but %s can represent only %d decimal places exactly",
				d, string(r.Units), max),
			"the tolerance is a whole number of cents derived from this count; "+
				"a finer one would not be")
	}
	if r.Units == amount.Dollars {
		return cmdutil.WithHint(
			errf(r.ID, "printed_decimals", "declared on a rule printed in dollars"),
			"a dollar-precision tolerance has no consumer in this corpus, and the "+
				"dollar-level discrepancies it looks like it would cover are the "+
				"city's own arithmetic rather than rounding -- p127's Total Property "+
				"Taxes is $1 over 13 rows -- which stated_total_deltas names exactly "+
				"and this must not absorb silently (fisc-2sd)")
	}
	if r.TotalRow == "" {
		return cmdutil.WithHint(
			errf(r.ID, "printed_decimals", "declared without a total_row"),
			"the tolerance applies to the comparison between the mapped rows and "+
				"a printed total; with no total there is nothing for it to loosen")
	}
	if len(r.Parts) > 1 && !r.TotalSpansParts {
		return cmdutil.WithHint(
			errf(r.ID, "printed_decimals", "declared on a %d-part rule whose total does not span its parts", len(r.Parts)),
			"such a rule is compared once PER PART, so the refusal of a tolerance "+
				"no column needed would fire on whichever page happens to tie "+
				"exactly; declare total_spans_parts, which makes it one comparison, "+
				"or split the rule")
	}
	for i := range r.Parts {
		p := &r.Parts[i]
		if len(p.StatedTotalDeltas) > 0 {
			return cmdutil.WithHint(
				errf(r.ID, fmt.Sprintf("parts[page %d].stated_total_deltas", p.Page),
					"declared on a rule that also declares printed_decimals"),
				"the two are disjoint on purpose: a delta names one exact figure the "+
					"document is out by, a tolerance bounds an unnamed one, and a rule "+
					"holding both offers a place to hide the difference between them")
		}
		// SAME COMPOSITION, ONE DECLARATION FURTHER OUT.
		// unmapped_text takes a printed figure OUT of the
		// read on the author's word that it belongs to no row. If that word is
		// wrong the row it belonged to is short, and a tolerance beside it is
		// exactly what would absorb the shortfall -- the arithmetic that would
		// otherwise catch the misdeclaration, gone.
		//
		// unmapped_text is already the weakest declaration class here (see
		// Part.UnmappedText); pairing it with the only mechanism that softens
		// the check standing over it is the one combination to refuse outright.
		// It costs the corpus nothing: p41's revenue rule declares the orphan
		// and ties EXACTLY, and its General Government rule declares the
		// tolerance and has no orphan.
		if len(p.UnmappedText) > 0 {
			return cmdutil.WithHint(
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
// above its own rows.
//
// THE SECTION-ANCHOR REQUIREMENT IS THE WHOLE MECHANISM, not a convenience. A
// flag meaning only "the total is somewhere above" would be unbounded: no guard
// in this package could say how far above, and checkGap's leading arm -- the one
// thing standing between a block that starts too early and header figures read
// as data -- would have to be retired to admit it. Requiring section ==
// total_row pins the total to exactly one line, at a position Block already had
// to resolve through anchor(), which refuses an ambiguous or missing one. The
// resolver then skips that single line and refuses every digit after it.
//
// See Rule.TotalRowAbove and fisc-h96o.
func validateTotalRowAbove(r *Rule, errf errFunc) error {
	if !r.TotalRowAbove {
		return nil
	}
	if r.TotalRow == "" {
		return cmdutil.WithHint(
			errf(r.ID, "total_row_above", "declared without a total_row"),
			"total_row_above says WHERE the printed total is; total_row says "+
				"which line it is, and there is nothing to find without it")
	}
	if r.TotalSpansParts {
		return cmdutil.WithHint(
			errf(r.ID, "total_row_above", "declared with total_spans_parts"),
			"a total printed above its own rows is on the same page as them by "+
				"construction, so it cannot also be the one total of a block "+
				"that straddles a page break")
	}
	for i := range r.Parts {
		p := &r.Parts[i]
		if p.LabelsFrom != 0 {
			return cmdutil.WithHint(
				errf(r.ID, fmt.Sprintf("parts[page %d].labels_from", p.Page),
					"declared on a rule with total_row_above"),
				"a label-less part finds its totals from stop_at and reads no "+
					"section anchor, so it has nowhere to print a total above")
		}
		if p.Section != r.TotalRow {
			return cmdutil.WithHint(
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
// row labels are printed fund names.
//
// It cannot check the claim itself: whether "Water" is fund 640 is a question
// about data/funds.yaml, and whether the page prints that label on that line is
// a question about the document. Both belong to
// row-funds-match-their-anchors, which has the registry and the pages. What is
// checkable without either is that the declaration asserts something, and that
// is the same discipline validateTotalRowKinds applies: a declaration that
// cannot fail records an author's belief rather than a property of the page.
//
// So the one refusal is a row the declaration would say nothing about. A rule
// whose rows carry no fund declares nothing by declaring this, and a single
// such row is enough — the check reads every row of the rule, so one row
// without a fund is one row the claim silently does not cover.
func validateRowLabelFunds(r *Rule, errf errFunc) error {
	if !r.RowLabelsNameFunds {
		return nil
	}
	// THE ALL-SKIPPED CASE IS THE STRONGER VERSION OF THE ROW GUARD BELOW, and
	// it was missing while that one was present -- refusing a declaration that
	// says nothing about ONE row while accepting one that says nothing about
	// any. Measured when found: a rule whose rows are all skip: true parsed
	// clean, and row-funds-match-their-anchors drops skipped rows from every
	// arm, so the declaration stood over zero checked rows and the rule was
	// named nowhere in the summary.
	// A ROW IS COVERED ONLY IF SOME PART READS IT, and there are TWO ways not to
	// be: skip: true, and being omitted from every part that could carry it. The
	// first version of this guard counted only the first, so a rule whose every
	// row appears in each part's omitted_rows parsed clean and the check read
	// none of them -- reproducing exactly the vacuous declaration the guard was
	// added to refuse, one omission mechanism over.
	//
	// ActiveRows is the same function row-funds-match-their-anchors reaches
	// through activeInAnyPart, so this counts what the check will actually read
	// rather than a second opinion about it.
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
		return cmdutil.WithHint(
			errf(r.ID, "row_labels_name_funds",
				"no part of this rule reads any of its rows, so the declaration "+
					"covers none of them"),
			"a row that is skipped, or omitted from every part, is never read "+
				"from the page, so nothing reads its label and nothing checks "+
				"the fund typed on it")
	}
	for _, row := range r.Rows {
		if row.Skip {
			// A skipped row is not read, so the claim is not about it. This is
			// the same reading validateTotalRowKinds takes of `have`.
			continue
		}
		if row.Fund == 0 {
			return cmdutil.WithHint(
				errf(r.ID, "row_labels_name_funds",
					"row %q declares no fund, so the declaration says nothing about it",
					row.PrintedLabel()),
				"the declaration is what lets row-funds-match-their-anchors read "+
					"a fund off the printed label and check the number typed "+
					"beside it; a row with no number to check needs no label read")
		}
		// A COUNTERPART IS AT THE FAR END AND NO BARE LABEL NAMES IT. The
		// declaration says the row's label is the printed name of the fund the
		// row carries -- its OWN fund. A counterpart's fund sits at the other
		// end of the movement, and a bare fund name carries no direction to
		// reach it, which is the whole difference between this and p76's
		// "Transfer From X to Y" anchors.
		//
		// REFUSED RATHER THAN LEFT UNCHECKED, because leaving it made
		// row-funds-match-their-anchors print a false sentence: the counterpart
		// fell into the unphrased counter, whose clause named the rule as one
		// that does not declare row_labels_name_funds while it did. Latent --
		// no rule declares both today.
		if row.Counterpart != nil {
			return cmdutil.WithHint(
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
// some of the kinds beneath it.
//
// Every refusal here is of a declaration that asserts nothing, which is the
// same discipline a stale wrapped_label and a delta that now ties exactly get:
// a claim that cannot fail is one that records an author's belief rather than a
// property of the document.
func validateTotalRowKinds(r *Rule, errf errFunc) error {
	if len(r.TotalRowKinds) == 0 {
		return nil
	}
	if r.TotalRow == "" {
		return cmdutil.WithHint(
			errf(r.ID, "total_row_kinds", "is declared but the rule has no total_row"),
			"the list says which of the rule's kinds the PRINTED total covers, "+
				"so there must be a printed total for it to describe")
	}
	seen := map[Kind]bool{}
	for _, k := range r.TotalRowKinds {
		if !k.valid() {
			return errf(r.ID, "total_row_kinds", "%q is not one of the five kinds", k)
		}
		if seen[k] {
			return errf(r.ID, "total_row_kinds", "%q is listed twice", k)
		}
		seen[k] = true
	}
	// The kinds the rule's rows actually carry. A declaration naming a kind no
	// row has would silently narrow the check to fewer rows than the author
	// believed -- or, if it named ALL of them, to none.
	have := map[Kind]bool{}
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
			return cmdutil.WithHint(
				errf(r.ID, "total_row_kinds", "no row of this rule has kind %q", k),
				"the list names the kinds the printed total covers, and every "+
					"one of them must be a kind this rule maps")
		}
	}
	if len(seen) == len(have) {
		return cmdutil.WithHint(
			errf(r.ID, "total_row_kinds", "names every kind the rule maps, so it excludes nothing"),
			"a total that covers all of its rows needs no declaration; remove "+
				"it, and the check stays kind-blind as the mixed fund blocks "+
				"on pp.131-140 need")
	}
	return nil
}

// validateTotalSpansParts checks the declaration that a printed total covers
// every part's rows rather than one part's.
//
// Only what is knowable without the pages is checked here. That a part's page
// actually PRINTS the total row, and that the part carrying a declared delta
// is the one printing it, are claims about the document and are checked when
// the rule resolves -- the parser has no page text and guessing would be the
// fail-open this flag exists to close.
func validateTotalSpansParts(r *Rule, errf errFunc) error {
	if !r.TotalSpansParts {
		return nil
	}
	if r.TotalRow == "" {
		return cmdutil.WithHint(
			errf(r.ID, "total_spans_parts", "is set but the rule declares no total_row"),
			"the flag says WHICH rows the printed total covers; with no total "+
				"row declared there is nothing for it to say that about")
	}
	// One part cannot straddle a page break, so the flag would assert nothing
	// -- and a declaration that cannot fail is the shape this repo refuses
	// everywhere else (a stale wrapped_label, a delta that now ties).
	if len(r.Parts) < 2 {
		return cmdutil.WithHint(
			errf(r.ID, "total_spans_parts", "is set on a rule with %d part", len(r.Parts)),
			"the flag exists for a block whose rows straddle a page break; on "+
				"a single part it is the per-part check already, so remove it")
	}
	// Identical columns, field by field. This is the guard that keeps the flag
	// off the p66-67 spine, whose parts partition COLUMNS rather than ROWS:
	// p66 carries four and p67 eight, so a sum across them would be adding
	// General Fund to Capital Funds and reporting the result as a total.
	first := r.Parts[0]
	for i := 1; i < len(r.Parts); i++ {
		p := r.Parts[i]
		if slices.Equal(first.Columns, p.Columns) {
			continue
		}
		return cmdutil.WithHint(
			errf(r.ID, "total_spans_parts",
				"page %d declares %d columns and page %d declares %d, and they must be identical",
				first.Page, len(first.Columns), p.Page, len(p.Columns)),
			"summing rows across parts is only meaningful where the parts "+
				"split the same table by ROW; parts that split it by COLUMN "+
				"are not what this flag describes")
	}
	// One printed total means one place to declare a discrepancy against it.
	// Which part that is needs the page, so it is checked at resolve time;
	// what is checkable here is that the rule does not name two.
	declaring := make([]int, 0, len(r.Parts))
	for _, p := range r.Parts {
		if len(p.StatedTotalDeltas) > 0 {
			declaring = append(declaring, p.Page)
		}
	}
	if len(declaring) > 1 {
		return cmdutil.WithHint(
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
		return cmdutil.WithHint(
			errf(r.ID, field("section_ordinal"), "is set but section is empty"),
			"section_ordinal picks which occurrence of section starts the "+
				"block, so it means nothing without one")
	}
	if p.StopAtOrdinal < 0 {
		return errf(r.ID, field("stop_at_ordinal"), "is %d; ordinals count from 1",
			p.StopAtOrdinal)
	}
	if p.StopAtOrdinal > 0 && p.StopAt == "" {
		return cmdutil.WithHint(
			errf(r.ID, field("stop_at_ordinal"), "is set but stop_at is empty"),
			"stop_at_ordinal picks which occurrence of stop_at ends the block, "+
				"so it means nothing without one")
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
	// At least one column must be headed. A list that is null all the way
	// through names no header to find the page's grid with, and the grid is
	// what the guard IS -- geom.NewGrid would be handed an empty span list and
	// describeUnmatched an empty header list. Refusing it here says the real
	// thing: a part whose every column is headerless has not opted into the
	// guard, it has asked for one that cannot exist.
	if !slices.ContainsFunc(p.ColumnHeaders, func(h columnHeader) bool { return !h.Unheaded }) {
		return cmdutil.WithHint(
			errf(r.ID, field("column_headers"), "every entry is null"),
			"the grid is built from the headers the page prints, so at least one "+
				"column must name one; a part that can name none declares no "+
				"column_headers at all")
	}
	for i, h := range p.ColumnHeaders {
		if h.Unheaded {
			// A null says the page prints no header over this column, so the
			// column must be one the rule reads nothing from -- otherwise the
			// declaration is asking for a figure to be filed under a band that
			// does not exist.
			if !p.Columns[i].Skip {
				return cmdutil.WithHint(
					errf(r.ID, field("column_headers"), "entry %d is null but column %d "+
						"is not skipped", i+1, i+1),
					"null says the page prints no header over this column, which "+
						"leaves it no band; a column the rule actually reads must "+
						"name its printed header so its figures can be placed")
			}
			// AND ONLY AT THE END. Past the last header there is nothing to
			// check a token against; between two headers there is a gap with
			// known bounds, which the grid would have checked. Allowing a null
			// there would silently decline a check that was available.
			if i != len(p.ColumnHeaders)-1 {
				return cmdutil.WithHint(
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
			return cmdutil.WithHint(
				errf(r.ID, field("column_headers"), "entry %d is empty", i+1),
				"a header is matched by joining the words printed on the header "+
					"line, so no page can produce an empty one; write null, not "+
					"\"\", for a column the page heads with nothing")
		}
		// Deliberately NOT the circularity argument the section/stop_at refusal
		// makes: a header is not a total. A header that is a figure would match
		// a DATA row, and a grid built from a data row files that row perfectly
		// and every other row by luck.
		if _, err := amount.Parse(h.Text, r.Units); err == nil {
			return cmdutil.WithHint(
				errf(r.ID, field("column_headers"), "entry %d is a currency amount: %q",
					i+1, h.Text),
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
		headers columnHeaders
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

// checkCounterpart refuses a far leg that could not be told apart from its own
// near leg, or that would be filed nowhere.
//
// The stakes are higher here than for an ordinary row, because a counterpart is
// invisible to every check the DOCUMENT provides. CheckTotals sums resolved
// values and the fan-out happens after them (fact.FromValues), so a counterpart
// declared wrongly still ties to the page's own printed total. These four arms
// are the parse-time half of what replaces that.
func checkCounterpart(r *Rule, row Row, errf errFunc) error {
	cp := row.Counterpart
	if cp == nil {
		return nil
	}
	if r.categoryOnColumns() {
		return cmdutil.WithHint(
			errf(r.ID, "rows", "row %q declares a counterpart, but this rule's columns "+
				"carry the category", row.Label),
			"a counterpart names ONE far end for every figure of its row, and a row "+
				"of this rule holds a figure per budget line -- its revenue, its "+
				"transfers in, its balances -- which no single far end is the other "+
				"side of")
	}
	if cp.Category == "" {
		return cmdutil.WithHint(
			errf(r.ID, "rows", "row %q: counterpart has no category", row.Label),
			"the far leg classifies on its own account -- a transfer OUT of the "+
				"paying fund, where the near leg is a transfer IN to the receiving "+
				"one -- and nothing is inherited from the row")
	}
	if !cp.Kind.valid() {
		return errf(r.ID, "rows", "row %q: counterpart kind %q is not one of the five",
			row.Label, cp.Kind)
	}
	if cp.FundGroup == "" {
		return cmdutil.WithHint(
			errf(r.ID, "rows", "row %q: counterpart has no fund_group", row.Label),
			"without one the leg's column_path falls back to the rule's scope, "+
				"where it is indistinguishable from any other scope-filed fact")
	}
	// AND A GROUP IS NOT ENOUGH. A counterpart exists to name the fund at the
	// far end of a movement; with a group alone it names only which sixth of
	// the city, which is what the near leg's own column already says. Nothing
	// downstream catches it: fact.FromValues' guard fires only when BOTH the
	// group and the fund are absent, and row-funds-match-their-anchors has no
	// number to compare its printed anchor against, so the leg publishes
	// fund 0 and every check stays green.
	//
	// The near leg is deliberately different -- Budget Book p76's LAVWMA row
	// receives into a joint powers authority that is no City fund at all, and a
	// group-only column path is the honest reading of it. If a PAYER ever turns
	// out to be similarly unnameable, that needs a decision rather than this
	// arm being relaxed: the far leg's whole purpose is the payer.
	if cp.Fund == 0 {
		return cmdutil.WithHint(
			errf(r.ID, "rows", "row %q: counterpart declares fund_group %q and no fund",
				row.Label, cp.FundGroup),
			"a counterpart names the fund at the far end of the movement; a group "+
				"on its own names no payer, and no check downstream can tell it "+
				"from one that was never declared")
	}
	// A department is an axis Counterpart has no field for, so the far leg
	// would silently take the near leg's -- publishing, say, Police's transfer
	// OUT of the fund that paid Police. Refused rather than dropped: dropping
	// it would make the far leg's row_path quietly narrower than the near
	// leg's, and a schedule crossing departments with transfers needs a
	// decision, not a default.
	if row.Department != "" {
		return cmdutil.WithHint(
			errf(r.ID, "rows", "row %q carries department %q and declares a counterpart",
				row.Label, row.Department),
			"the far leg has no department of its own to declare, and inheriting "+
				"this one would attribute the paying fund's outflow to the "+
				"receiving department")
	}
	// The two legs must differ where a fact's identity is built from: its
	// category (the row path) and its fund (the column path). Equal on both,
	// the ids collide.
	//
	// IT COMPARES AGAINST EVERY COLUMN OF EVERY PART, and that is the fix for
	// fisc-i38 rather than a generalisation. The arm used to compare
	// row.EffectiveColumn(Column{}) -- the row's own fund and group with no
	// printed column behind them. Two guards above require cp.FundGroup != "",
	// so the group clause could only hold when the ROW also declared a group,
	// and no published rule does: on Budget Book p76 a section IS the receiving
	// group, so the group lives on the COLUMN and the row declares only a fund.
	// The arm was therefore unreachable on every rule in the tree, while its own
	// comment claimed it caught "the one an author writing a two-legged row
	// actually makes". Measured: a counterpart duplicating its near leg parsed
	// clean and failed much later in `fisc build` as "id ... claimed twice:
	// rule X ... rule X", which does not read as "your counterpart duplicates
	// its own near leg".
	//
	// A row applies to every column of every part, so "the row's effective fund
	// group" is not one value here -- which is what made this look unfixable.
	// The answer is that it does not have to be: a collision on ANY column is a
	// collision, because that column's figure publishes both legs.
	// fact.CheckUniqueIDs stays the backstop for whatever a parse-time check
	// cannot see; it is no longer the only thing that sees this.
	if cp.Category != row.Category {
		return nil
	}
	for i := range r.Parts {
		p := &r.Parts[i]
		// A PART THAT DOES NOT PRINT THIS ROW PUBLISHES NO FACT FROM IT, so it
		// cannot collide. Refusing on one would be a false refusal against a
		// cell the document does not have.
		if omittedSet(p)[row.Identity()] {
			continue
		}
		for j := range p.Columns {
			// Same for a skipped column: it consumes its position and yields no
			// fact, so there is nothing for the far leg to collide with. Both
			// arms are latent on the committed corpus -- no skipped column
			// carries a fund today -- and both were real false refusals,
			// reproduced through Parse before being fixed.
			if p.Columns[j].Skip {
				continue
			}
			near := row.EffectiveColumn(p.Columns[j])
			if cp.Fund == near.Fund && cp.FundGroup == near.FundGroup {
				return cmdutil.WithHint(
					errf(r.ID, "rows", "row %q: counterpart is the same category and the "+
						"same fund as the row itself in column %d of the part on page %d",
						row.Label, j+1, p.Page),
					"the two legs of one figure are told apart by their category and "+
						"their fund; identical on both, they are one fact published twice")
			}
		}
	}
	// THERE IS NO ROW-ONLY FALLBACK AFTER THIS LOOP, and there was one for two
	// commits. It compared row.EffectiveColumn(Column{}) unconditionally, so it
	// re-imposed the refusal on exactly the cells the exemptions above exist to
	// excuse: a row omitted from its only part, or colliding only with a skipped
	// column, was refused by the fallback after the loop had correctly passed
	// over it. Reproduced through Parse.
	//
	// It was written for "a rule with no parts", which cannot reach here:
	// validateRule refuses one ("is empty; a rule must name at least one page")
	// before the loop that calls this function. So its only reachable effect was
	// the false refusal. A guard for a state the CALLER has already excluded is
	// not defensive; it is a second, worse copy of the check, and this copy
	// disagreed with the first.
	//
	// No line numbers here on purpose: the relationship is caller-and-callee,
	// which does not move when lines do.
	return nil
}

// validateOmittedCells holds each omitted_cells entry to naming one cell the
// part reads: one row, by namesOneRow, that the part prints, under one header
// of a column the part does not skip. The part must declare column_headers,
// because the guard is what checks a row's figures sit under the columns they
// are filed under once one of them is blank.
func validateOmittedCells(r *Rule, p *Part,
	namesOneRow func(field string, j int, o omittedRow, notARow string) error, errf errFunc) error {
	if len(p.OmittedCells) == 0 {
		return nil
	}
	field := fmt.Sprintf("parts[page %d].omitted_cells", p.Page)
	if len(p.ColumnHeaders) == 0 {
		return cmdutil.WithHint(
			errf(r.ID, field, "omitted_cells needs column_headers on the same part"),
			"a row with a blank cell is read left to right under the columns it "+
				"prints, and only the column guard can say each figure is under "+
				"the column it is filed under")
	}
	omitted := omittedSet(p)
	declared := map[string]map[string]bool{}
	for j, o := range p.OmittedCells {
		if err := namesOneRow(field, j, o.row(), "omitted_cells names a cell of a "+
			"row the rule lists"); err != nil {
			return err
		}
		id := o.row().identity()
		switch {
		case omitted[id]:
			return errf(r.ID, field, "%q is omitted from this part by omitted_rows, "+
				"so it has no cell to leave blank", o.row().printedLabel())
		case o.Column == "":
			return errf(r.ID, field, "%q names no column", o.row().printedLabel())
		case strings.TrimSpace(o.Note) == "":
			return cmdutil.WithHint(
				errf(r.ID, field, "%s has no note", o.describe()),
				"say what the page prints on the row and that this cell is blank")
		case declared[id][o.Column]:
			return errf(r.ID, field, "%s is declared twice", o.describe())
		}
		at := -1
		for c, h := range p.ColumnHeaders {
			if !h.Unheaded && h.Text == o.Column {
				if at >= 0 {
					return errf(r.ID, field, "%q heads more than one of this part's "+
						"columns, so it cannot say which cell is blank", o.Column)
				}
				at = c
			}
		}
		switch {
		case at < 0:
			return errf(r.ID, field, "%q is not one of this part's column_headers: %s",
				o.Column, describeHeaders(p.ColumnHeaders))
		case p.Columns[at].Skip:
			return cmdutil.WithHint(
				errf(r.ID, field, "%q is a skipped column", o.Column),
				"a skipped column yields no figure to be absent; no page needs a "+
					"blank declared there yet, so the read refuses one")
		}
		if declared[id] == nil {
			declared[id] = map[string]bool{}
		}
		declared[id][o.Column] = true
		// Counted over the columns the row still prints, whether or not a fact
		// is made of them, and whether or not the row publishes: a row printing
		// none leaves the read nothing to end on. A skipped column can never
		// be declared blank, so counting it would let a row with every figure
		// blank pass as one with a cell blank.
		printed := 0
		for c, col := range p.Columns {
			if !col.Skip && !declared[id][p.ColumnHeaders[c].Text] {
				printed++
			}
		}
		if printed == 0 {
			return cmdutil.WithHint(
				errf(r.ID, field, "declares every column of %q blank",
					o.row().printedLabel()),
				"a row the page prints no figure of is declared in omitted_rows")
		}
	}
	return nil
}

// describeHeaders renders a header list for an error message. Written out
// rather than left to %q because a ColumnHeader is a struct, and a null entry
// has to read as the claim it is rather than as an empty string.
func describeHeaders(hs columnHeaders) string {
	parts := make([]string, len(hs))
	for i, h := range hs {
		parts[i] = h.String()
	}
	return "[" + strings.Join(parts, " ") + "]"
}

// validateSubtotals checks the rows a rule declares as printed subtotals.
//
// A subtotal publishes nothing, so it is a skipped row: spelling both keeps
// every reader of Row.Skip right without teaching each one about subtotals.
// Its figures are compared by column header, so every part of a rule with a
// subtotal row, or in a chain, names each of its columns.
func validateSubtotals(r *Rule, errf errFunc) error {
	has := r.SubtotalChain != ""
	for _, row := range r.Rows {
		switch {
		case row.Subtotal < 0:
			return errf(r.ID, "rows", "row %q: subtotal is %d; levels count from 1",
				row.PrintedLabel(), row.Subtotal)
		case row.Subtotal > 0 && !row.Skip:
			return cmdutil.WithHint(
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
			return cmdutil.WithHint(
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
