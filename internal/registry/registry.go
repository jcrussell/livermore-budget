// Package registry reads the three curated registries under data/: the fund
// list (funds.yaml), the category taxonomy (taxonomy.yaml) and the department
// axis (departments.yaml), the controlled vocabulary behind a fact's
// `category`, `fund_group` and `department`. It is the only place any of them
// is parsed.
//
// Loading validates: a loaded registry has no duplicate slug or fund number,
// every parent resolves, and every derived entry carries its rationale and
// source note, so a caller looks up without re-checking. Consumers declare
// their own narrow interface over it (byob-interfaces.2).
package registry

import (
	"fmt"
	"slices"
	"strings"
)

// These are the names Load reads from the root of the filesystem it is given.
const (
	FundsFile       = "funds.yaml"
	TaxonomyFile    = "taxonomy.yaml"
	DepartmentsFile = "departments.yaml"
)

const (
	// fundsSchemaVersion is the only funds.yaml version this package reads.
	fundsSchemaVersion = 1
	// taxonomySchemaVersion is the only taxonomy.yaml version this package reads.
	taxonomySchemaVersion = 1
	// departmentsSchemaVersion is the only departments.yaml version this
	// package reads.
	departmentsSchemaVersion = 1
)

// `debt-service` is a fund TYPE (FundTypeDebtService) and `debt-services` an
// expenditure CATEGORY: different axes, kept apart by spelling.
const categoryDebtServices = "debt-services"

// Duplicate constant keys are a compile-time error, so this stops building the
// day someone normalizes one spelling into the other.
var _ = map[string]struct{}{
	FundTypeDebtService:  {},
	categoryDebtServices: {},
}

// fundTypes is the closed set of `type` values funds.yaml may use: the seven
// the appendix prints (p258, plus the proprietary types on p261).
var fundTypes = []string{
	FundTypeGeneral,
	FundTypeSpecialRevenue,
	FundTypeCapital,
	FundTypeDebtService,
	FundTypeEnterprise,
	FundTypeInternalService,
	FundTypePermanent,
}

// The fund types, each the fund group a fund of that type is drawn in.
const (
	FundTypeGeneral         = "general"
	FundTypeSpecialRevenue  = "special-revenue"
	FundTypeCapital         = "capital"
	FundTypeDebtService     = "debt-service"
	FundTypeEnterprise      = "enterprise"
	FundTypeInternalService = "internal-service"
	FundTypePermanent       = "permanent"
)

// FundTypes is every fund type data/funds.yaml may declare, in the appendix's
// order.
func FundTypes() []string { return slices.Clone(fundTypes) }

// constraintTiers is the closed set of constraint_tier values. They are
// DERIVED from the Description of Funds narrative; `unknown` records that the
// document does not support a classification.
var constraintTiers = []string{
	"discretionary",
	"restricted-by-law",
	"committed",
	"unknown",
}

// alias is one string the city prints for a thing this registry names
// differently. Term is the printed spelling verbatim, typos included, and Pages
// says where it is printed.
//
// Derived marks a binding that is an inference rather than a reading ("Police
// Evidence" for fund 211 rests on arithmetic; "Cal Home Reuse" differs only in
// case). Load refuses a derived alias without a Rationale.
type alias struct {
	Term    string `yaml:"term"`
	Pages   []int  `yaml:"pages"`
	Derived bool   `yaml:"derived"`
	// Rationale is required when Derived, refused otherwise.
	Rationale string `yaml:"rationale"`
	Note      string `yaml:"note"`
}

// contraRow names a detail line that is negative inside its own printed
// subtotal. A consumer that re-sums the detail must keep the sign, and a flow
// diagram must not render one as its own inbound flow.
type contraRow struct {
	Term string `yaml:"term"`
	Page int    `yaml:"page"`
}

// Category is one entry in taxonomy.yaml. Slug is ours; Label, DocumentTerm and
// every Alias are the city's words.
type Category struct {
	Slug         string      `yaml:"slug"`
	Label        string      `yaml:"label"`
	DocumentTerm string      `yaml:"document_term"`
	Parent       string      `yaml:"parent"`
	Kinds        []string    `yaml:"kinds"`
	Pages        []int       `yaml:"pages"`
	Aliases      []alias     `yaml:"aliases"`
	ContraRows   []contraRow `yaml:"contra_rows"`
	Note         string      `yaml:"note"`

	// Derived marks a slug that is not a mechanical transform of a printed
	// string. Load refuses it without Rationale and SourceNote.
	Derived    bool   `yaml:"derived"`
	Rationale  string `yaml:"rationale"`
	SourceNote string `yaml:"source_note"`

	// Assignable reports whether a mapping rule may write this slug. Load sets
	// it from categoryEntry, because the YAML key defaults to true by absence.
	Assignable bool `yaml:"-"`
}

// clone deep-copies the slice fields so a caller cannot write through them into
// the registry's state.
func (c Category) clone() Category {
	c.Kinds = slices.Clone(c.Kinds)
	c.Pages = slices.Clone(c.Pages)
	c.ContraRows = slices.Clone(c.ContraRows)
	c.Aliases = slices.Clone(c.Aliases)
	for i := range c.Aliases {
		c.Aliases[i].Pages = slices.Clone(c.Aliases[i].Pages)
	}
	return c
}

// Fund is one entry in funds.yaml. Number, Name and Type are published
// (appendix pp. 253-257); ConstraintTier and RestrictionNote are DERIVED.
type Fund struct {
	Number          int    `yaml:"number"`
	Name            string `yaml:"name"`
	Type            string `yaml:"type"`
	ConstraintTier  string `yaml:"constraint_tier"`
	RestrictionNote string `yaml:"restriction_note"`

	// Aliases are the other spellings the schedules print for this fund (p76
	// "Low Income Hsng", p136 "Police Evidence"), each with its page.
	// FundByLabel resolves them.
	Aliases []alias `yaml:"aliases"`

	// Major is set only where the budget book says so.
	Major bool `yaml:"major"`
}

// clone deep-copies the slice fields, as Category.clone does.
func (f Fund) clone() Fund {
	f.Aliases = slices.Clone(f.Aliases)
	for i := range f.Aliases {
		f.Aliases[i].Pages = slices.Clone(f.Aliases[i].Pages)
	}
	return f
}

// Department is one of the eleven ALL-CAPS headings Budget Book pp.167-170
// print over their division rows. A FACT NEVER NAMES ONE: a fact's
// `department:` carries a Division slug, reached up through
// Division.Department.
//
// DocumentTerm is the heading as printed, not always the total row's wording
// (PUBLIC WORKS p169:47 closes as "PUBLIC WORKS DEPARTMENT TOTAL"), so the rule
// file spells its anchors out.
type Department struct {
	Slug         string `yaml:"slug"`
	Label        string `yaml:"label"`
	DocumentTerm string `yaml:"document_term"`
	Pages        []int  `yaml:"pages"`
	Note         string `yaml:"note"`

	// Derived marks an inferred department, on the same terms as Category.
	Derived    bool   `yaml:"derived"`
	Rationale  string `yaml:"rationale"`
	SourceNote string `yaml:"source_note"`
}

// clone deep-copies the slice fields, as Category.clone does.
func (d Department) clone() Department {
	d.Pages = slices.Clone(d.Pages)
	return d
}

// Division is one of the mixed-case row groups printed beneath a Department,
// and it is what a fact's published `department` field holds.
//
// pp.167-170 (General Fund only) print 23 and pp.85-125 print 29; the registry
// holds the union, and Pages says which schedules show each.
//
// Department is a separate namespace from Slug, not a parent segment: five
// departments share a name with a division beneath them, and fact.RowPath
// composes `<division>/<category>` from a single-segment slug.
type Division struct {
	Slug       string `yaml:"slug"`
	Label      string `yaml:"label"`
	Department string `yaml:"department"`
	Pages      []int  `yaml:"pages"`
	Note       string `yaml:"note"`

	Derived    bool   `yaml:"derived"`
	Rationale  string `yaml:"rationale"`
	SourceNote string `yaml:"source_note"`
}

// clone deep-copies the slice fields. See Department.clone.
func (d Division) clone() Division {
	d.Pages = slices.Clone(d.Pages)
	return d
}

// Registry is the loaded set of files. It is read-only after Load and safe
// for concurrent use.
type Registry struct {
	categories map[string]Category
	slugs      []string // sorted, for enumeration in a stable order

	funds       map[int]Fund
	fundNumbers []int    // sorted, for enumeration in a stable order
	fundGroups  []string // sorted, deduplicated, as observed in funds.yaml
	// fundLabels maps every published spelling to the one fund that may claim
	// it; Load rejects a second claimant.
	fundLabels map[string]int

	departments     map[string]Department
	departmentSlugs []string // sorted, for enumeration in a stable order
	divisions       map[string]Division
	divisionSlugs   []string // sorted, for enumeration in a stable order
}

// Category returns the taxonomy entry for slug. The bool answers "does it
// exist", which Assignable cannot: a rollup and a typo are both unassignable.
func (r *Registry) Category(slug string) (Category, bool) {
	c, ok := r.categories[slug]
	if !ok {
		return Category{}, false
	}
	return c.clone(), true
}

// Label returns the city's printed words for slug. internal/project's labels
// interface declares it, so its signature is part of the site's contract.
func (r *Registry) Label(slug string) (string, bool) {
	c, ok := r.categories[slug]
	if !ok {
		return "", false
	}
	return c.Label, true
}

// Assignable reports whether a mapping rule may classify a fact as slug. It is
// false for an unknown slug and for a rollup such as `taxes`; use Category to
// tell the two apart.
func (r *Registry) Assignable(slug string) bool {
	c, ok := r.categories[slug]
	return ok && c.Assignable
}

// Categories returns every entry, ordered by slug. The slice and every slice
// inside it are copies, so a caller may sort or trim the result.
func (r *Registry) Categories() []Category {
	out := make([]Category, 0, len(r.slugs))
	for _, s := range r.slugs {
		out = append(out, r.categories[s].clone())
	}
	return out
}

// LinesPrintedAs is every entry under parent, for kind, whose document_term or
// alias equals printed byte for byte. Exact, because pp.127-140 print "Current
// Year - Secured" beside "Current Year - Unsecured"; a slice, because
// fact-revenue-lines-resolve refuses the ambiguity with the rows it resolves to.
func (r *Registry) LinesPrintedAs(parent, printed, kind string) []string {
	// An empty spelling would match every entry declaring no document_term.
	if printed == "" {
		return nil
	}
	var out []string
	for _, s := range r.slugs {
		c := r.categories[s]
		if c.Parent != parent || !slices.Contains(c.Kinds, kind) {
			continue
		}
		if c.DocumentTerm == printed {
			out = append(out, c.Slug)
			continue
		}
		for _, a := range c.Aliases {
			if a.Term == printed {
				out = append(out, c.Slug)
				break
			}
		}
	}
	return out
}

// Terms is every spelling the city prints entry slug as: its document_term,
// then its aliases in declared order, and nil for a slug the registry does not
// know. A revenue line sharing one with its category is the category printed
// whole under its own name, which is how the projection tells it from a row.
func (r *Registry) Terms(slug string) []string {
	c, ok := r.categories[slug]
	if !ok {
		return nil
	}
	var out []string
	if c.DocumentTerm != "" {
		out = append(out, c.DocumentTerm)
	}
	for _, a := range c.Aliases {
		out = append(out, a.Term)
	}
	return out
}

// Fund returns the registry entry for a fund number.
func (r *Registry) Fund(number int) (Fund, bool) {
	f, ok := r.funds[number]
	if !ok {
		return Fund{}, false
	}
	return f.clone(), true
}

// FundName is the city's name for a fund number, and whether the registry
// knows it. It is the half of Fund that internal/project's Labels interface can
// declare without importing this package. A miss renders as the number.
func (r *Registry) FundName(number int) (string, bool) {
	f, ok := r.funds[number]
	if !ok {
		return "", false
	}
	return f.Name, true
}

// FundByLabel returns the fund the city prints as label, matched EXACTLY
// against its name or an alias. The abbreviations are ambiguous ("Measure D"
// heads both 550 and its CIP twin 828), so no prefix or fuzzy match: a label
// naming no fund is an UnknownFundError, and Load rejects two funds claiming
// one label.
func (r *Registry) FundByLabel(label string) (Fund, error) {
	number, ok := r.fundLabels[label]
	if !ok {
		return Fund{}, &unknownFundError{Label: label}
	}
	return r.funds[number].clone(), nil
}

// Funds returns a copy of every fund, ordered by number. Fund numbers are
// sparse, so callers enumerate rather than probe.
func (r *Registry) Funds() []Fund {
	out := make([]Fund, 0, len(r.fundNumbers))
	for _, n := range r.fundNumbers {
		out = append(out, r.funds[n].clone())
	}
	return out
}

// ConstraintTier is the spending constraint recorded for a fund. It returns ""
// for an unlisted fund, which is not the tier "unknown" (the document
// establishes no restriction).
func (r *Registry) ConstraintTier(fund int) string {
	return r.funds[fund].ConstraintTier
}

// FundType is the type funds.yaml records for a number, and whether the
// registry knows it. It is the parent edge of the tier hierarchy: taking the
// type from the appendix (pp.253-257) rather than from the fact's fund_group
// (pp.131-140) is what keeps the fold from being a tautology.
func (r *Registry) FundType(number int) (string, bool) {
	f, ok := r.funds[number]
	if !ok {
		return "", false
	}
	return f.Type, true
}

// RestrictionNote is our DERIVED reading of what ties a fund's money down
// (pp. 258-261). It travels beside ConstraintTier as its rationale; see project.Node.
func (r *Registry) RestrictionNote(fund int) string {
	return r.funds[fund].RestrictionNote
}

// DivisionLabel is the city's words for a division slug, declarable in
// [project.Labels] without importing this package.
func (r *Registry) DivisionLabel(slug string) (string, bool) {
	d, ok := r.divisions[slug]
	if !ok || d.Label == "" {
		return "", false
	}
	return d.Label, true
}

// DepartmentLabel is the city's words for a department slug -- the ALL-CAPS
// tier -- and whether departments.yaml lists it.
func (r *Registry) DepartmentLabel(slug string) (string, bool) {
	d, ok := r.departments[slug]
	if !ok {
		return "", false
	}
	return d.Label, true
}

// FundGroup reports whether name is a fund type the loaded funds.yaml actually
// uses, so a group with no fund left in it stops validating.
func (r *Registry) FundGroup(name string) bool {
	return slices.Contains(r.fundGroups, name)
}

// FundGroups returns the fund types present in funds.yaml, sorted. The result
// is a copy.
func (r *Registry) FundGroups() []string {
	return slices.Clone(r.fundGroups)
}

// Division returns the departments.yaml entry for slug, which is what a fact's
// `department` field holds.
func (r *Registry) Division(slug string) (Division, bool) {
	d, ok := r.divisions[slug]
	if !ok {
		return Division{}, false
	}
	return d.clone(), true
}

// Department reports whether slug is a department departments.yaml lists.
// Budget Book pp.85-125's rows name a department where other rules name a
// division.
func (r *Registry) Department(slug string) bool {
	_, ok := r.departments[slug]
	return ok
}

// Divisions returns every division, ordered by slug. The slice and every slice
// inside it are copies.
func (r *Registry) Divisions() []Division {
	out := make([]Division, 0, len(r.divisionSlugs))
	for _, s := range r.divisionSlugs {
		out = append(out, r.divisions[s].clone())
	}
	return out
}

// Departments returns every department, ordered by slug. The slice and every
// slice inside it are copies.
func (r *Registry) Departments() []Department {
	out := make([]Department, 0, len(r.departmentSlugs))
	for _, s := range r.departmentSlugs {
		out = append(out, r.departments[s].clone())
	}
	return out
}

// Error reports a problem in one registry file, naming the entry at fault.
type Error struct {
	// File is FundsFile, TaxonomyFile, DepartmentsFile or SourcesFile.
	File string
	// Entry names the offending entry, already rendered ("fund 291"); empty
	// for a file-level problem.
	Entry string
	Field string
	Msg   string
}

// unknownFundError reports a printed label that names no fund, as its own type
// so a caller can collect every such label at once.
type unknownFundError struct {
	// Label is the string as printed, unnormalized.
	Label string
}

func (e *unknownFundError) Error() string {
	return fmt.Sprintf("no fund is named %q in %s; a printed label resolves only by an exact match on a fund's name or a declared alias",
		e.Label, FundsFile)
}

func (e *Error) Error() string {
	var b strings.Builder
	b.WriteString(e.File)
	if e.Entry != "" {
		fmt.Fprintf(&b, ": %s", e.Entry)
	}
	if e.Field != "" {
		fmt.Fprintf(&b, ": %s", e.Field)
	}
	fmt.Fprintf(&b, ": %s", e.Msg)
	return b.String()
}

// labeler mirrors the Label method `internal/project` declares, so a change to
// its signature breaks this package's build. It covers Label alone.
type labeler interface {
	Label(slug string) (string, bool)
}

var _ labeler = (*Registry)(nil)
