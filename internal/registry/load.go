package registry

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
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

// categoryEntry decodes a category. It exists only to carry `assignable`,
// whose default is TRUE: an absent YAML key leaves a bool false, and a false
// default here would make every ordinary category unassignable and every
// mapping rule invalid. The pointer separates "absent" from "explicitly
// false"; Load folds it into Category.Assignable so no caller has to.
type categoryEntry struct {
	Category      `yaml:",inline"`
	RawAssignable *bool `yaml:"assignable"`
}

// Load reads and validates both registries from the root of fsys, so a caller
// passes os.DirFS("data") and tests pass an in-memory tree (byob-interfaces.3).
//
// Both files are required: they are two halves of one vocabulary, and a
// half-loaded registry would answer FundGroup confidently while answering
// every Category with "unknown".
func Load(fsys fs.FS) (*Registry, error) {
	r := &Registry{}
	if err := r.loadFunds(fsys); err != nil {
		return nil, err
	}
	if err := r.loadTaxonomy(fsys); err != nil {
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
		for j, a := range f.Aliases {
			if err := validateFundAlias(f, j, a, fundf); err != nil {
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

// errFunc formats an error against one named entry.
type errFunc func(entry, field, format string, args ...any) error

// fundErrFunc formats an error against one fund, which is named by its number
// rather than by a slug.
type fundErrFunc func(number int, field, format string, args ...any) error

// validateFundAlias checks one published spelling. Uniqueness is not checked
// here — that is claimLabel's job, because it spans the whole file — so this
// is only the shape of the entry itself.
func validateFundAlias(f Fund, i int, a Alias, fundf fundErrFunc) error {
	at := fmt.Sprintf("aliases[%d]", i)
	if a.Term == "" {
		return fundf(f.Number, at+".term", "is required")
	}
	// An alias asserts that the city prints this string. Without a page that
	// assertion cannot be checked, and an alias nobody can check is a rename
	// we have made up: the whole channel exists so a reader can go and look.
	if len(a.Pages) == 0 {
		return fundf(f.Number, at+".pages",
			"is required; alias %q must say which page it was read from", a.Term)
	}
	for j, p := range a.Pages {
		if p <= 0 {
			return fundf(f.Number, at+".pages",
				"is %d for alias %q; pages are 1-based PDF page numbers", p, a.Term)
		}
		if j > 0 && p <= a.Pages[j-1] {
			return fundf(f.Number, at+".pages",
				"%d follows %d for alias %q; pages are listed once each, in ascending order",
				p, a.Pages[j-1], a.Term)
		}
	}

	// The fourth provenance invariant, on the alias channel. Pages says the
	// string is printed; derived says the binding to THIS fund is ours. A
	// reader must be able to tell the two apart without re-doing the work.
	switch {
	case a.Derived && a.Rationale == "":
		return fundf(f.Number, at+".rationale",
			"is required when derived is true; alias %q must say what the binding rests on",
			a.Term)
	case !a.Derived && a.Rationale != "":
		// Same asymmetry validateCategory records: a forgotten `derived: true`
		// is far likelier than a stray rationale, and it fails open — the entry
		// reads as justified while nothing ever demands the justification.
		return fundf(f.Number, at+".derived",
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
	if n := strings.Count(c.Slug, "/"); n > 1 {
		return catf(c.Slug, "slug",
			"has %d parent segments; the taxonomy is one level deep", n)
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
