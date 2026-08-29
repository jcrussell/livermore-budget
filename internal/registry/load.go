package registry

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"regexp"
	"slices"
	"strings"

	yaml "go.yaml.in/yaml/v3"

	"github.com/jcrussell/livermore-budget/pkg/cmdutil"
)

// schemaVersionErr reports a version this package cannot read. A file written
// for a newer fisc is a different failure from a malformed one — nothing is
// wrong with the file — so it gets the hint that says so, as mapping.Parse
// does for rule files.
func schemaVersionErr(file string, got, want int) error {
	err := &Error{File: file, Field: "schema_version",
		Msg: fmt.Sprintf("got %d, want %d", got, want)}
	if got > want {
		return cmdutil.WithHint(err,
			"this registry was written for a newer fisc; upgrade the binary")
	}
	return err
}

// fundsDoc is the whole of funds.yaml.
type fundsDoc struct {
	SchemaVersion int    `yaml:"schema_version"`
	Funds         []Fund `yaml:"funds"`
}

// taxonomyDoc is the whole of taxonomy.yaml.
type taxonomyDoc struct {
	SchemaVersion int             `yaml:"schema_version"`
	Categories    []categoryEntry `yaml:"categories"`
}

// departmentsDoc is the whole of departments.yaml. The two tiers are two
// top-level lists rather than divisions nested inside their department,
// because they are two NAMESPACES: nesting would read as one hierarchy and
// invite the cross-tier uniqueness rule the file cannot satisfy.
type departmentsDoc struct {
	SchemaVersion int          `yaml:"schema_version"`
	Departments   []Department `yaml:"departments"`
	Divisions     []Division   `yaml:"divisions"`
}

// categoryEntry decodes a category. It exists only to carry `assignable`,
// whose default is TRUE: an absent YAML key leaves a bool false, and a false
// default here would make every ordinary category unassignable and every
// mapping rule invalid. The pointer separates "absent" from "explicitly
// false"; Load folds it into Category.Assignable so no caller has to.
type categoryEntry struct {
	Category      `yaml:",inline"`
	RawAssignable *bool `yaml:"assignable"`
}

// Load reads and validates all three registries from the root of fsys, so a
// caller passes os.DirFS("data") and tests pass an in-memory tree
// (byob-interfaces.3).
//
// ALL THREE FILES ARE REQUIRED: they are three parts of one vocabulary, and a
// partly loaded registry would answer FundGroup confidently while answering
// every Category with "unknown". departments.yaml joined them when the
// pp.167-170 lane made the department axis real; before that a fact carrying a
// department resolved against nothing and the check for it could only report
// that it had no file to ask.
//
// The cost of "required" is that every in-memory fixture has to carry all
// three. That is the intended trade: an optional registry is one a test can
// forget, and the invariant this package sells — a registry that loaded is one
// whose invariants already hold — is only worth anything if it covers the whole
// vocabulary.
func Load(fsys fs.FS) (*Registry, error) {
	r := &Registry{}
	if err := r.loadFunds(fsys); err != nil {
		return nil, err
	}
	if err := r.loadTaxonomy(fsys); err != nil {
		return nil, err
	}
	if err := r.loadDepartments(fsys); err != nil {
		return nil, err
	}
	return r, nil
}

// decodeFile reads one YAML file whole. It reads rather than streams so there
// is no Close to get wrong, matching corpus's use of fs.ReadFile.
func decodeFile(fsys fs.FS, name string, v any) error {
	b, err := fs.ReadFile(fsys, name)
	if err != nil {
		return fmt.Errorf("read %s: %w", name, err)
	}
	dec := yaml.NewDecoder(bytes.NewReader(b))
	// An unknown key is a hard error, as in internal/mapping. These files are
	// the vocabulary itself: a misspelled `assignabel: false` that decoded
	// quietly would publish a rollup node as a spendable category.
	dec.KnownFields(true)
	if err := dec.Decode(v); err != nil {
		if errors.Is(err, io.EOF) {
			return &Error{File: name, Msg: "file is empty"}
		}
		return &Error{File: name, Msg: err.Error()}
	}
	// The decoder is a stream, so stopping here would silently discard a
	// second `---` document — the same quiet loss KnownFields exists to
	// prevent, one level up.
	var extra any
	if err := dec.Decode(&extra); err == nil {
		return &Error{File: name, Msg: "file contains more than one YAML document"}
	} else if !errors.Is(err, io.EOF) {
		return &Error{File: name, Msg: err.Error()}
	}
	return nil
}

func (r *Registry) loadFunds(fsys fs.FS) error {
	var doc fundsDoc
	if err := decodeFile(fsys, FundsFile, &doc); err != nil {
		return err
	}

	errf := func(entry, field, format string, args ...any) error {
		return &Error{File: FundsFile, Entry: entry, Field: field,
			Msg: fmt.Sprintf(format, args...)}
	}
	fundf := func(number int, field, format string, args ...any) error {
		return errf(fmt.Sprintf("fund %d", number), field, format, args...)
	}

	if doc.SchemaVersion != FundsSchemaVersion {
		return schemaVersionErr(FundsFile, doc.SchemaVersion, FundsSchemaVersion)
	}
	if len(doc.Funds) == 0 {
		return errf("", "funds", "is empty")
	}

	r.funds = make(map[int]Fund, len(doc.Funds))
	r.fundNumbers = make([]int, 0, len(doc.Funds))
	r.fundLabels = make(map[string]int, 2*len(doc.Funds))
	groups := map[string]bool{}

	// claimLabel records the one fund a published spelling may resolve to.
	// The second claimant is an error naming both, because a label two funds
	// answer to is a label no schedule can be mapped by: this is where
	// "Measure D" on the operating fund and on its CIP twin stops being a
	// coin flip at lookup time and becomes a file that will not load.
	aliased := map[string]bool{}
	claimLabel := func(label string, f Fund, isAlias bool) error {
		prev, dup := r.fundLabels[label]
		if !dup {
			r.fundLabels[label] = f.Number
			aliased[label] = isAlias
			return nil
		}
		kind, field := "name", "name"
		if isAlias {
			kind, field = "alias", "aliases"
		}
		prevKind := "name"
		if aliased[label] {
			prevKind = "alias"
		}
		return fundf(f.Number, field,
			"%s %q is already the %s of fund %d (%q); a published label must name exactly one fund",
			kind, label, prevKind, prev, r.funds[prev].Name)
	}
	for i, f := range doc.Funds {
		// The fund number is the join key every fact carries, so an entry
		// without one is not addressable at all.
		if f.Number <= 0 {
			return errf(fmt.Sprintf("funds[%d]", i), "number",
				"is %d; a fund is identified by its number (name %q)", f.Number, f.Name)
		}
		if prev, dup := r.funds[f.Number]; dup {
			return fundf(f.Number, "number",
				"duplicate fund number, already defined as %q", prev.Name)
		}
		if f.Name == "" {
			return fundf(f.Number, "name", "is required")
		}
		if !slices.Contains(fundTypes, f.Type) {
			return fundf(f.Number, "type", "got %q, want one of %s",
				f.Type, strings.Join(fundTypes, ", "))
		}
		if !slices.Contains(constraintTiers, f.ConstraintTier) {
			return fundf(f.Number, "constraint_tier", "got %q, want one of %s",
				f.ConstraintTier, strings.Join(constraintTiers, ", "))
		}
		// The tier is our reading, not the city's, so the note is its
		// evidence. A tier with nothing behind it is an assertion about
		// public money that no reader can check — including the `unknown`
		// tier, whose note says why the document settles nothing.
		if f.RestrictionNote == "" {
			return fundf(f.Number, "restriction_note",
				"is required; constraint_tier %q is derived and must say what it is derived from",
				f.ConstraintTier)
		}
		r.funds[f.Number] = f
		r.fundNumbers = append(r.fundNumbers, f.Number)
		groups[f.Type] = true

		if err := claimLabel(f.Name, f, false); err != nil {
			return err
		}
		fundAliasf := func(field, format string, args ...any) error {
			return fundf(f.Number, field, format, args...)
		}
		for j, a := range f.Aliases {
			if err := validateAlias(j, a, fundAliasf); err != nil {
				return err
			}
			if err := claimLabel(a.Term, f, true); err != nil {
				return err
			}
		}
	}
	slices.Sort(r.fundNumbers)

	r.fundGroups = slices.Sorted(maps.Keys(groups))
	return nil
}

func (r *Registry) loadTaxonomy(fsys fs.FS) error {
	var doc taxonomyDoc
	if err := decodeFile(fsys, TaxonomyFile, &doc); err != nil {
		return err
	}

	errf := func(entry, field, format string, args ...any) error {
		return &Error{File: TaxonomyFile, Entry: entry, Field: field,
			Msg: fmt.Sprintf(format, args...)}
	}
	catf := func(slug, field, format string, args ...any) error {
		return errf(fmt.Sprintf("category %q", slug), field, format, args...)
	}

	if doc.SchemaVersion != TaxonomySchemaVersion {
		return schemaVersionErr(TaxonomyFile, doc.SchemaVersion, TaxonomySchemaVersion)
	}
	if len(doc.Categories) == 0 {
		return errf("", "categories", "is empty")
	}

	r.categories = make(map[string]Category, len(doc.Categories))
	r.slugs = make([]string, 0, len(doc.Categories))
	for i, e := range doc.Categories {
		c := e.Category
		if c.Slug == "" {
			return errf(fmt.Sprintf("categories[%d]", i), "slug",
				"is required (label %q)", c.Label)
		}
		if _, dup := r.categories[c.Slug]; dup {
			return catf(c.Slug, "slug", "duplicate slug")
		}
		if err := validateCategory(c, catf); err != nil {
			return err
		}
		// Absent means assignable: the exceptions are the three rollup nodes,
		// and requiring the other twenty-two to opt in would make the common
		// case the one that is easy to get wrong.
		c.Assignable = e.RawAssignable == nil || *e.RawAssignable
		r.categories[c.Slug] = c
		r.slugs = append(r.slugs, c.Slug)
	}
	slices.Sort(r.slugs)

	// Parents resolve in a second pass so the file's order stays free: the
	// taxonomy groups entries by revenue, expenditure, transfer and fund
	// balance for a reader, not by definition-before-use.
	for _, slug := range r.slugs {
		if err := validateParent(r.categories[slug], r.categories, catf); err != nil {
			return err
		}
	}
	return nil
}

// slugShape is the form a departments.yaml slug must take: lowercase,
// digits, single hyphens, ONE segment.
//
// One segment is not a style rule. A fact's row_path is `<division>/<category>`
// (internal/fact.RowPath), so a slug carrying its own "/" would emit a
// two-slash row_path that no reader could split back into its two axes. It is
// the same shape internal/check enforces on the value a fact carries; checked
// in both places because they are two different claims — that the file is well
// formed, and that a published fact is.
var slugShape = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// loadDepartments reads departments.yaml.
//
// IT RUNS AFTER loadTaxonomy AND DEPENDS ON IT: the cross-axis check below asks
// whether a division slug is already a category slug, and it can only ask that
// of a loaded taxonomy. Reordering Load would turn that check silently vacuous
// rather than failing, which is why this says so rather than relying on the
// call order looking deliberate.
func (r *Registry) loadDepartments(fsys fs.FS) error {
	var doc departmentsDoc
	if err := decodeFile(fsys, DepartmentsFile, &doc); err != nil {
		return err
	}

	errf := func(entry, field, format string, args ...any) error {
		return &Error{File: DepartmentsFile, Entry: entry, Field: field,
			Msg: fmt.Sprintf(format, args...)}
	}
	deptf := func(slug, field, format string, args ...any) error {
		return errf(fmt.Sprintf("department %q", slug), field, format, args...)
	}
	divf := func(slug, field, format string, args ...any) error {
		return errf(fmt.Sprintf("division %q", slug), field, format, args...)
	}

	if doc.SchemaVersion != DepartmentsSchemaVersion {
		return schemaVersionErr(DepartmentsFile, doc.SchemaVersion, DepartmentsSchemaVersion)
	}
	if len(doc.Departments) == 0 {
		return errf("", "departments", "is empty")
	}
	if len(doc.Divisions) == 0 {
		return errf("", "divisions", "is empty")
	}

	r.departments = make(map[string]Department, len(doc.Departments))
	r.departmentSlugs = make([]string, 0, len(doc.Departments))
	for i, d := range doc.Departments {
		if d.Slug == "" {
			return errf(fmt.Sprintf("departments[%d]", i), "slug",
				"is required (label %q)", d.Label)
		}
		if _, dup := r.departments[d.Slug]; dup {
			return deptf(d.Slug, "slug", "duplicate slug")
		}
		if err := validateSlug(d.Slug, deptf); err != nil {
			return err
		}
		if d.Label == "" {
			return deptf(d.Slug, "label", "is required")
		}
		// The heading is how a reader finds the entry on the page. Without it
		// the parentage recorded here is an assertion about the document that
		// nobody can go and check.
		if d.DocumentTerm == "" {
			return deptf(d.Slug, "document_term",
				"is required; it is the ALL-CAPS heading pp.167-170 print for this department")
		}
		if err := validatePages(d.Pages, d.Slug, "", deptf); err != nil {
			return err
		}
		if err := validateProvenance(d.Slug, d.Derived, d.Rationale, d.SourceNote, deptf); err != nil {
			return err
		}
		r.departments[d.Slug] = d
		r.departmentSlugs = append(r.departmentSlugs, d.Slug)
	}
	slices.Sort(r.departmentSlugs)

	r.divisions = make(map[string]Division, len(doc.Divisions))
	r.divisionSlugs = make([]string, 0, len(doc.Divisions))
	claimed := map[string]bool{}
	for i, d := range doc.Divisions {
		if d.Slug == "" {
			return errf(fmt.Sprintf("divisions[%d]", i), "slug",
				"is required (label %q)", d.Label)
		}
		if _, dup := r.divisions[d.Slug]; dup {
			return divf(d.Slug, "slug", "duplicate slug")
		}
		if err := validateSlug(d.Slug, divf); err != nil {
			return err
		}
		if d.Label == "" {
			return divf(d.Slug, "label", "is required")
		}
		// THE CROSS-AXIS REFUSAL, and it is the one collision in this file that
		// matters. A department and a division may share a name -- the city
		// prints five such pairs -- but a division and a CATEGORY may not,
		// because row_path joins exactly those two on a "/" and a reader
		// splitting `planning/services-and-supplies` has no way to know which
		// half is which if one string can be either.
		if _, isCategory := r.categories[d.Slug]; isCategory {
			return divf(d.Slug, "slug",
				"is also a %s category slug; a division and a category are two axes and a "+
					"fact's row_path joins them, so one string may not be both",
				TaxonomyFile)
		}
		if d.Department == "" {
			return divf(d.Slug, "department",
				"is required; every division is printed under one heading")
		}
		if _, ok := r.departments[d.Department]; !ok {
			return divf(d.Slug, "department", "unknown department %q", d.Department)
		}
		claimed[d.Department] = true
		if err := validatePages(d.Pages, d.Slug, "", divf); err != nil {
			return err
		}
		if err := validateProvenance(d.Slug, d.Derived, d.Rationale, d.SourceNote, divf); err != nil {
			return err
		}
		r.divisions[d.Slug] = d
		r.divisionSlugs = append(r.divisionSlugs, d.Slug)
	}
	slices.Sort(r.divisionSlugs)

	// A department with no divisions is a heading over nothing. On pp.167-170
	// every heading has at least one row group beneath it, so an unclaimed
	// department is a division that was dropped or misfiled -- and a dropped
	// division is invisible to the object-category sums, which is the whole
	// reason the department tier is recorded at all.
	for _, slug := range r.departmentSlugs {
		if !claimed[slug] {
			return deptf(slug, "", "no division names this department")
		}
	}
	return nil
}

// validateSlug checks one slug's shape against slugShape.
func validateSlug(slug string, ef errFunc) error {
	if !slugShape.MatchString(slug) {
		return ef(slug, "slug",
			"is not a slug: lower case, digits and single hyphens, one segment")
	}
	return nil
}

// validatePages checks a `pages` list: present, 1-based, ascending, each page
// once. The rules are validateAlias's, applied to an entry rather than to
// an alias, because the claim is the same one -- this entry is printed there,
// go and look.
func validatePages(pages []int, slug, at string, ef errFunc) error {
	if len(pages) == 0 {
		return ef(slug, pagesField(at), "is required; say which page the entry is printed on")
	}
	return validatePagesShape(pages, slug, at, ef)
}

// validatePagesShape is validatePages WITHOUT the presence rule: 1-based,
// ascending, each page once.
//
// The split exists because a taxonomy category's `pages` is optional and its
// shape is not. Three of the twenty-five categories carry none, and they are
// exactly the three `assignable: false` rollups -- so "required when
// assignable" is defensible on today's data and is filed as its own bead
// rather than taken here, because it would churn forty inline fixtures for a
// rule with no live violation. What is NOT defensible is the state this
// replaces, where a category could claim page 0 or list a page twice and load
// clean.
func validatePagesShape(pages []int, slug, at string, ef errFunc) error {
	field := pagesField(at)
	for i, p := range pages {
		if p <= 0 {
			return ef(slug, field, "is %d; pages are 1-based PDF page numbers", p)
		}
		if i > 0 && p <= pages[i-1] {
			return ef(slug, field,
				"%d follows %d; pages are listed once each, in ascending order", p, pages[i-1])
		}
	}
	return nil
}

func pagesField(at string) string {
	if at == "" {
		return "pages"
	}
	return at + ".pages"
}

// validateProvenance is the fourth invariant on an entry that carries derived,
// rationale and source_note -- the same three-way check validateCategory makes,
// including the asymmetry: a forgotten `derived: true` is far likelier than a
// stray rationale, and it fails open.
func validateProvenance(slug string, derived bool, rationale, sourceNote string, ef errFunc) error {
	switch {
	case derived && rationale == "":
		return ef(slug, "rationale", "is required when derived is true")
	case derived && sourceNote == "":
		return ef(slug, "source_note", "is required when derived is true")
	case !derived && rationale != "":
		return ef(slug, "derived",
			"is not set, but a rationale is given; a published name needs no rationale")
	case !derived && sourceNote != "":
		return ef(slug, "derived",
			"is not set, but a source_note is given; a published name needs no source_note")
	}
	return nil
}

// errFunc formats an error against one named entry.
type errFunc func(entry, field, format string, args ...any) error

// entryErrFunc formats an error against a field of one already-named entry.
// It is what lets validateAlias serve both channels: a fund is named by its
// number and a category by its slug, and the alias rules care about neither.
type entryErrFunc func(field, format string, args ...any) error

// validateAlias checks one published spelling. Uniqueness is not checked
// here — for funds that is claimLabel's job, because it spans the whole file —
// so this is only the shape of the entry itself.
//
// IT DELIBERATELY MAKES NO CLAIM ABOUT THE OWNING ENTRY'S OWN `pages`.
// contra_rows gets exactly that cross-field arm below and aliases must never
// get it: measured over the committed taxonomy, all 8 of 8 category alias
// blocks are WHOLLY DISJOINT from their category's pages. That is not a defect
// -- a contra row is a detail line inside the category's own printed subtotal,
// so it is on a page the category claims; an alias is the OTHER SPELLING, and
// the reason a spelling needs recording at all is that some other schedule
// prints it. Adding the arm here "by symmetry" would reject the whole
// committed alias channel.
func validateAlias(i int, a Alias, ef entryErrFunc) error {
	at := fmt.Sprintf("aliases[%d]", i)
	if a.Term == "" {
		return ef(at+".term", "is required")
	}
	// An alias asserts that the city prints this string. Without a page that
	// assertion cannot be checked, and an alias nobody can check is a rename
	// we have made up: the whole channel exists so a reader can go and look.
	if len(a.Pages) == 0 {
		return ef(at+".pages",
			"is required; alias %q must say which page it was read from", a.Term)
	}
	for j, p := range a.Pages {
		if p <= 0 {
			return ef(at+".pages",
				"is %d for alias %q; pages are 1-based PDF page numbers", p, a.Term)
		}
		if j > 0 && p <= a.Pages[j-1] {
			return ef(at+".pages",
				"%d follows %d for alias %q; pages are listed once each, in ascending order",
				p, a.Pages[j-1], a.Term)
		}
	}

	// The fourth provenance invariant, on the alias channel. Pages says the
	// string is printed; derived says the binding to THIS fund is ours. A
	// reader must be able to tell the two apart without re-doing the work.
	switch {
	case a.Derived && a.Rationale == "":
		return ef(at+".rationale",
			"is required when derived is true; alias %q must say what the binding rests on",
			a.Term)
	case !a.Derived && a.Rationale != "":
		// Same asymmetry validateCategory records: a forgotten `derived: true`
		// is far likelier than a stray rationale, and it fails open — the entry
		// reads as justified while nothing ever demands the justification.
		return ef(at+".derived",
			"is not set for alias %q, but a rationale is given; a binding the city prints needs none",
			a.Term)
	}
	return nil
}

func validateCategory(c Category, catf errFunc) error {
	// The label is the city's word for the slug, and it is the only field
	// `internal/project` publishes. A category without one would put our
	// machine identifier on the site.
	if c.Label == "" {
		return catf(c.Slug, "label", "is required")
	}
	// The kind decides which total a fact may be summed into. An entry with
	// none classifies nothing.
	if len(c.Kinds) == 0 {
		return catf(c.Slug, "kinds", "is required")
	}
	// AND A MEMBER MUST BE A REAL KIND. Until this landed, `kinds: [banana]`
	// loaded clean and left `fisc verify` fully green -- 38 passed, 0 failed --
	// because internal/check's fact-kind-matches-category compares a fact's
	// kind against this list and a category no fact has reached is never
	// consulted. Four of the twenty-five categories are in that state today,
	// including an assignable one, so the typo would surface years later as a
	// mass failure instead of now as a one-line file error. It is the shape
	// fisc-ttq already cost this repo once, when all four transfer categories
	// declared a `transfer` kind that mapping.Kind has never defined.
	seen := make(map[string]bool, len(c.Kinds))
	for _, k := range c.Kinds {
		if !slices.Contains(factKinds, k) {
			return catf(c.Slug, "kinds", "got %q, want one of %s",
				k, strings.Join(factKinds, ", "))
		}
		if seen[k] {
			return catf(c.Slug, "kinds", "%q is listed twice", k)
		}
		seen[k] = true
	}
	if n := strings.Count(c.Slug, "/"); n > 1 {
		return catf(c.Slug, "slug",
			"has %d parent segments; the taxonomy is one level deep", n)
	}
	// A category's pages are optional (see validatePagesShape) and their shape
	// is not.
	if err := validatePagesShape(c.Pages, c.Slug, "", catf); err != nil {
		return err
	}
	catAliasf := func(field, format string, args ...any) error {
		return catf(c.Slug, field, format, args...)
	}
	for i, a := range c.Aliases {
		if err := validateAlias(i, a, catAliasf); err != nil {
			return err
		}
	}
	for i, cr := range c.ContraRows {
		at := fmt.Sprintf("contra_rows[%d]", i)
		if cr.Term == "" {
			return catf(c.Slug, at+".term", "is required")
		}
		if cr.Page <= 0 {
			return catf(c.Slug, at+".page",
				"is %d for %q; pages are 1-based PDF page numbers", cr.Page, cr.Term)
		}
		// THE CROSS-FIELD ARM, and it is the one worth having: a contra row is
		// a detail line printed INSIDE this category's own subtotal, so it can
		// only be on a page this category claims. A row citing some other page
		// means one of the two records is wrong, and today neither is read by
		// anything in production -- so nothing else would ever notice.
		//
		// Note the arm is proved by n=2, both in taxes/property, both on p127
		// which that category lists. It is the only rule here that can become
		// false as data grows rather than only as a file is mistyped. If a
		// contra row ever needs a page its category does not claim, the answer
		// is to widen `pages`, because the category IS printed there.
		//
		// It must NOT be copied onto aliases -- see validateAlias.
		if len(c.Pages) > 0 && !slices.Contains(c.Pages, cr.Page) {
			return catf(c.Slug, at+".page",
				"is %d for %q, which is not one of the category's pages %v",
				cr.Page, cr.Term, c.Pages)
		}
	}

	// The fourth provenance invariant: a classification we inferred and a
	// term the city printed must not be presented alike.
	switch {
	case c.Derived && c.Rationale == "":
		return catf(c.Slug, "rationale", "is required when derived is true")
	case c.Derived && c.SourceNote == "":
		return catf(c.Slug, "source_note", "is required when derived is true")
	case !c.Derived && c.Rationale != "":
		// The likelier mistake by far is a forgotten `derived: true` rather
		// than a stray rationale, and it fails open: the entry reads as
		// justified while `verify` never demands the justification.
		return catf(c.Slug, "derived",
			"is not set, but a rationale is given; a published term needs no rationale")
	case !c.Derived && c.SourceNote != "":
		return catf(c.Slug, "derived",
			"is not set, but a source_note is given; a published term needs no source_note")
	}
	return nil
}

// validateParent checks the hierarchy, which taxonomy.yaml states twice: the
// slug rule lifts a shared head noun into a parent segment, and the file is
// one level deep. Checking both means a mis-parented entry — the head noun
// saying one thing and `parent:` another — cannot load, and so cannot put a
// row under a group it does not belong to in a rollup view.
func validateParent(c Category, byslug map[string]Category, catf errFunc) error {
	stem, _, nested := strings.Cut(c.Slug, "/")
	if c.Parent == "" {
		if nested {
			return catf(c.Slug, "parent",
				"is required; the slug's head noun %q names one", stem)
		}
		return nil
	}
	parent, ok := byslug[c.Parent]
	if !ok {
		return catf(c.Slug, "parent", "unknown category %q", c.Parent)
	}
	if !nested {
		return catf(c.Slug, "parent",
			"is %q, but the slug has no parent segment", c.Parent)
	}
	if c.Parent != stem {
		return catf(c.Slug, "parent",
			"is %q, but the slug's head noun is %q", c.Parent, stem)
	}
	if parent.Parent != "" {
		return catf(c.Slug, "parent",
			"%q is itself a child of %q; the taxonomy is one level deep",
			c.Parent, parent.Parent)
	}
	return nil
}
