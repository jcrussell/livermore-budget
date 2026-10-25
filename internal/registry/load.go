package registry

import (
	"bytes"
	"fmt"
	"io/fs"
	"maps"
	"regexp"
	"slices"
	"strings"

	"github.com/jcrussell/livermore-budget/internal/hint"
	"github.com/jcrussell/livermore-budget/internal/strictyaml"
	"github.com/jcrussell/livermore-budget/internal/vocab"
)

// schemaVersionErr reports a version this package cannot read, with the hint
// that a newer fisc wrote it rather than that it is malformed.
func schemaVersionErr(file string, got, want int) error {
	err := &Error{File: file, Field: "schema_version",
		Msg: fmt.Sprintf("got %d, want %d", got, want)}
	if got > want {
		return hint.With(err,
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
// top-level lists because they are two namespaces.
type departmentsDoc struct {
	SchemaVersion int          `yaml:"schema_version"`
	Departments   []Department `yaml:"departments"`
	Divisions     []Division   `yaml:"divisions"`
}

// categoryEntry decodes a category so the pointer can tell an absent
// `assignable` (default TRUE) from an explicit false.
type categoryEntry struct {
	Category      `yaml:",inline"`
	RawAssignable *bool `yaml:"assignable"`
}

// Load reads and validates all three registries from the root of fsys
// (byob-interfaces.3). ALL THREE FILES ARE REQUIRED: a partly loaded registry
// would answer some lookups confidently and others with "unknown".
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
	// An unknown key is a hard error: a misspelled `assignabel: false` would
	// publish a rollup as a spendable category.
	if err := strictyaml.Decode(bytes.NewReader(b), v); err != nil {
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

	if doc.SchemaVersion != fundsSchemaVersion {
		return schemaVersionErr(FundsFile, doc.SchemaVersion, fundsSchemaVersion)
	}
	if len(doc.Funds) == 0 {
		return errf("", "funds", "is empty")
	}

	r.funds = make(map[int]Fund, len(doc.Funds))
	r.fundNumbers = make([]int, 0, len(doc.Funds))
	r.fundLabels = make(map[string]int, 2*len(doc.Funds))
	groups := map[string]bool{}

	// claimLabel records the one fund a published spelling may resolve to; a
	// second claimant fails the load.
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
		// The tier is our reading, so the note is its evidence, including for
		// `unknown`.
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

	if doc.SchemaVersion != taxonomySchemaVersion {
		return schemaVersionErr(TaxonomyFile, doc.SchemaVersion, taxonomySchemaVersion)
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
		c.Assignable = e.RawAssignable == nil || *e.RawAssignable
		r.categories[c.Slug] = c
		r.slugs = append(r.slugs, c.Slug)
	}
	slices.Sort(r.slugs)

	// Parents resolve in a second pass so the file's order stays free.
	for _, slug := range r.slugs {
		if err := validateParent(r.categories[slug], r.categories, catf); err != nil {
			return err
		}
	}
	return nil
}

// slugShape is the form a departments.yaml slug must take: lowercase, digits,
// single hyphens, ONE segment, because row_path is `<division>/<category>`
// (fact.RowPath). internal/check enforces the same on facts via ValidSlug.
var slugShape = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// ValidSlug reports whether s has a department slug's shape.
func ValidSlug(s string) bool { return slugShape.MatchString(s) }

// loadDepartments reads departments.yaml. IT MUST RUN AFTER loadTaxonomy: the
// cross-axis check asks the loaded taxonomy, and would otherwise pass
// vacuously.
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

	if doc.SchemaVersion != departmentsSchemaVersion {
		return schemaVersionErr(DepartmentsFile, doc.SchemaVersion, departmentsSchemaVersion)
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
		// row_path joins `department` to a category on "/", and pp.85-125 put
		// a department slug there, so the axes may not share a slug.
		if _, isCategory := r.categories[d.Slug]; isCategory {
			return deptf(d.Slug, "slug",
				"is also a %s category slug; a department and a category are two axes and a "+
					"fact's row_path joins them, so one string may not be both",
				TaxonomyFile)
		}
		if d.DocumentTerm == "" {
			return deptf(d.Slug, "document_term",
				"is required; it is the ALL-CAPS heading pp.167-170 print for this department")
		}
		if err := validatePages(d.Pages, d.Slug, deptf); err != nil {
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
		// A department and a division may share a name; a division and a
		// CATEGORY may not, because row_path joins exactly those two.
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
		if err := validatePages(d.Pages, d.Slug, divf); err != nil {
			return err
		}
		if err := validateProvenance(d.Slug, d.Derived, d.Rationale, d.SourceNote, divf); err != nil {
			return err
		}
		r.divisions[d.Slug] = d
		r.divisionSlugs = append(r.divisionSlugs, d.Slug)
	}
	slices.Sort(r.divisionSlugs)

	// A department with no divisions is a dropped or misfiled division, which
	// would be invisible to the object-category sums.
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
// once.
func validatePages(pages []int, slug string, ef errFunc) error {
	if len(pages) == 0 {
		return ef(slug, "pages", "is required; say which page the entry is printed on")
	}
	return validatePagesShape(pages, slug, ef)
}

// validatePagesShape is validatePages without the presence rule, for a
// taxonomy category whose `pages` is optional (fisc-3did).
func validatePagesShape(pages []int, slug string, ef errFunc) error {
	const field = "pages"
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

// validateProvenance is the fourth invariant: a derived entry carries rationale
// and source_note, and a published one neither. A forgotten `derived: true`
// otherwise fails open.
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

// entryErrFunc formats an error against a field of one already-named entry, so
// validateAlias serves funds and categories alike.
type entryErrFunc func(field, format string, args ...any) error

// validateAlias checks the shape of one published spelling; fund-wide
// uniqueness is claimLabel's.
//
// It must make NO claim about the owning entry's `pages`: every committed
// category alias is printed on pages its category does not list, because an
// alias is the spelling some other schedule prints.
func validateAlias(i int, a alias, ef entryErrFunc) error {
	at := fmt.Sprintf("aliases[%d]", i)
	if a.Term == "" {
		return ef(at+".term", "is required")
	}
	// An alias without a page is a rename nobody can check.
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

	switch {
	case a.Derived && a.Rationale == "":
		return ef(at+".rationale",
			"is required when derived is true; alias %q must say what the binding rests on",
			a.Term)
	case !a.Derived && a.Rationale != "":
		return ef(at+".derived",
			"is not set for alias %q, but a rationale is given; a binding the city prints needs none",
			a.Term)
	}
	return nil
}

func validateCategory(c Category, catf errFunc) error {
	// Label is the only field `internal/project` publishes.
	if c.Label == "" {
		return catf(c.Slug, "label", "is required")
	}
	// The kind decides which total a fact may be summed into. An entry with
	// none classifies nothing.
	if len(c.Kinds) == 0 {
		return catf(c.Slug, "kinds", "is required")
	}
	// A member must be a real kind, or `kinds: [banana]` loads clean and
	// verify stays green until a fact reaches the category (fisc-ttq).
	seen := make(map[string]bool, len(c.Kinds))
	for _, k := range c.Kinds {
		if !vocab.Kind(k).Valid() {
			return catf(c.Slug, "kinds", "got %q, want one of %s", k, vocab.KindList())
		}
		if seen[k] {
			return catf(c.Slug, "kinds", "%q is listed twice", k)
		}
		seen[k] = true
	}
	// A category's pages are optional (see validatePagesShape) and their shape
	// is not.
	if err := validatePagesShape(c.Pages, c.Slug, catf); err != nil {
		return err
	}
	catAliasf := func(field, format string, args ...any) error {
		return catf(c.Slug, field, format, args...)
	}
	// Uniqueness is PER CATEGORY, not file-wide: fund-balance/beginning and
	// /ending both correctly publish "Fund Balance / Working Capital".
	terms := make(map[string]bool, len(c.Aliases))
	for i, a := range c.Aliases {
		if err := validateAlias(i, a, catAliasf); err != nil {
			return err
		}
		// Compare both published spellings: five categories have a
		// document_term that differs from their label.
		for _, own := range [...]struct{ field, value string }{
			{"label", c.Label},
			{"document_term", c.DocumentTerm},
		} {
			if own.value != "" && a.Term == own.value {
				return catf(c.Slug, fmt.Sprintf("aliases[%d].term", i),
					"%q is the category's own %s; an alias records a DIFFERENT "+
						"spelling the city prints", a.Term, own.field)
			}
		}
		if terms[a.Term] {
			return catf(c.Slug, fmt.Sprintf("aliases[%d].term", i),
				"%q is listed twice", a.Term)
		}
		terms[a.Term] = true
	}
	// Contra rows are checked against pages, so pages are required once any
	// contra row is declared; otherwise the check fails open.
	if len(c.ContraRows) > 0 && len(c.Pages) == 0 {
		return catf(c.Slug, "pages",
			"is required when contra_rows is set; a contra row is a detail line "+
				"inside this category's printed subtotal, and with no pages there is "+
				"nothing to check it against")
	}
	rows := make(map[contraRow]bool, len(c.ContraRows))
	for i, cr := range c.ContraRows {
		at := fmt.Sprintf("contra_rows[%d]", i)
		if cr.Term == "" {
			return catf(c.Slug, at+".term", "is required")
		}
		if cr.Page <= 0 {
			return catf(c.Slug, at+".page",
				"is %d for %q; pages are 1-based PDF page numbers", cr.Page, cr.Term)
		}
		// A contra row is printed inside this category's subtotal, so it must
		// be on a page the category claims. Internal consistency only: nothing
		// here bounds `pages` to the document (fisc-e0p8). Must NOT be copied
		// onto aliases -- see validateAlias.
		if !slices.Contains(c.Pages, cr.Page) {
			return catf(c.Slug, at+".page",
				"is %d for %q, which is not one of the category's pages %v",
				cr.Page, cr.Term, c.Pages)
		}
		// The same term on the same page twice would apply its sign flip
		// twice (p127 ERAF: -14,086,438). A different page is fine.
		if rows[cr] {
			return catf(c.Slug, at, "repeats %q on page %d", cr.Term, cr.Page)
		}
		rows[cr] = true
	}

	// The fourth provenance invariant: a classification we inferred and a
	// term the city printed must not be presented alike.
	return validateProvenance(c.Slug, c.Derived, c.Rationale, c.SourceNote, catf)
}

// validateParent checks that a nested slug's parent is the slug minus its last
// segment, so a mis-parented entry cannot load. A parent is strictly shorter,
// so no cycle check is needed.
func validateParent(c Category, byslug map[string]Category, catf errFunc) error {
	i := strings.LastIndex(c.Slug, "/")
	nested := i >= 0
	var stem string
	if nested {
		stem = c.Slug[:i]
	}
	if c.Parent == "" {
		if nested {
			return catf(c.Slug, "parent",
				"is required; the slug's head noun %q names one", stem)
		}
		return nil
	}
	if _, ok := byslug[c.Parent]; !ok {
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
	return nil
}
