// Package registry reads the three curated registries under data/: the fund
// list (funds.yaml), the category taxonomy (taxonomy.yaml) and the department
// axis (departments.yaml).
//
// These files are the controlled vocabulary behind three of a fact's fields.
// Until something reads them, a mapping rule's `category:` and a column's
// `fund_group:` are free strings, and a rule that writes `tax/property` where
// the taxonomy says `taxes/property` produces a fact that is confidently
// wrong and joins to nothing. This package is what makes those fields
// checkable (fisc-1wr.1), and it is the only place any of them is parsed.
//
// Loading validates. A registry that loaded is one whose invariants already
// hold — no duplicate slug or fund number, every parent resolving, every
// derived entry carrying its rationale and source note — so a caller looks up
// without re-checking. The strictness is deliberate: both files are
// hand-written, and a typo that survives load becomes a wrong classification
// on a published page rather than a build failure.
//
// Consumers are expected to declare their own narrow interface over the
// handful of methods they use (byob-interfaces.2) rather than importing
// Registry itself; `internal/project` takes Label that way.
package registry

import (
	"fmt"
	"slices"
	"strings"
)

// These are the names Load reads from the root of the filesystem it is given,
// so a caller passes os.DirFS("data") rather than composing paths this package
// would have to agree with.
const (
	FundsFile       = "funds.yaml"
	TaxonomyFile    = "taxonomy.yaml"
	DepartmentsFile = "departments.yaml"
)

// The three registries version independently: a change to the fund schema says
// nothing about the taxonomy schema, and one shared constant would force a
// pointless bump on the others every time.
const (
	// FundsSchemaVersion is the only funds.yaml version this package reads.
	FundsSchemaVersion = 1
	// TaxonomySchemaVersion is the only taxonomy.yaml version this package reads.
	TaxonomySchemaVersion = 1
	// DepartmentsSchemaVersion is the only departments.yaml version this
	// package reads.
	DepartmentsSchemaVersion = 1
)

// The near miss this project is most likely to make: `debt-service` is a fund
// TYPE in funds.yaml, `debt-services` an expenditure CATEGORY in
// taxonomy.yaml. They are different axes — a fund's classification versus what
// a dollar was spent on — and taxonomy.yaml renamed its category to the plural
// specifically to keep one string from spanning both.
const (
	fundTypeDebtService  = "debt-service"
	categoryDebtServices = "debt-services"
)

// Duplicate constant keys in a map literal are a compile-time error, so this
// declaration stops building the day someone "normalizes" one of the two
// spellings into the other — which is exactly the edit that would silently
// cross the axes, and which no test over today's data would catch.
var _ = map[string]struct{}{
	fundTypeDebtService:  {},
	categoryDebtServices: {},
}

// fundTypes is the closed set of `type` values funds.yaml may use: the seven
// the appendix prints (p258, plus the proprietary types on p261). `permanent`
// has no column on the citywide spine, which is a gap in the spine and not a
// reason to leave the type out here.
var fundTypes = []string{
	"general",
	"special-revenue",
	"capital",
	fundTypeDebtService,
	"enterprise",
	"internal-service",
	"permanent",
}

// constraintTiers is the closed set of constraint_tier values. Unlike the
// fund types these are DERIVED — our reading of the Description of Funds
// narrative — which is why `unknown` is a member: it records that the
// document does not support a classification, and it must stay
// distinguishable from a tier we simply failed to write down.
var constraintTiers = []string{
	"discretionary",
	"restricted-by-law",
	"committed",
	"unknown",
}

// Alias is one string the city prints for a thing this registry names
// differently. Term is the printed spelling, verbatim -- abbreviations and
// typos included, because normalizing is what would let "Measure D" collapse
// onto two different funds -- and Pages says where to go and look. An alias
// nobody can check is a rename we have made up.
//
// Derived draws the fourth provenance invariant through this channel: binding a
// printed string to an entry is sometimes a mechanical reading and sometimes an
// inference, and the two must not be presented alike. "Cal Home Reuse" for "CAL
// Home Reuse" is a reading — the page names the fund and the only difference is
// case. "Police Evidence" for fund 211 is an inference: the city never prints
// that the two names are one fund, and the binding rests on arithmetic done
// here. The second obliges a Rationale saying what the inference rests on, and
// Load refuses it without one.
//
// There is deliberately no SourceNote to match Category's: Pages already is the
// source note for an alias, because an alias's whole claim is "this string
// appears on these pages". Rationale carries the different question of why the
// string binds to THIS entry.
type Alias struct {
	Term    string `yaml:"term"`
	Pages   []int  `yaml:"pages"`
	Derived bool   `yaml:"derived"`
	// Rationale is required when Derived, refused otherwise.
	Rationale string `yaml:"rationale"`
	Note      string `yaml:"note"`
}

// ContraRow names a detail line that is negative inside its own printed
// subtotal. A consumer that re-sums the detail must keep the sign, and a flow
// diagram must not render one as its own inbound flow.
type ContraRow struct {
	Term string `yaml:"term"`
	Page int    `yaml:"page"`
}

// Category is one entry in taxonomy.yaml — the only place a `category:` value
// is defined.
//
// Slug is ours and no reader should be shown it; Label and DocumentTerm and
// every Alias are the city's words, which is what makes Label the field
// `internal/project` publishes.
type Category struct {
	Slug         string      `yaml:"slug"`
	Label        string      `yaml:"label"`
	DocumentTerm string      `yaml:"document_term"`
	Parent       string      `yaml:"parent"`
	Kinds        []string    `yaml:"kinds"`
	Pages        []int       `yaml:"pages"`
	Aliases      []Alias     `yaml:"aliases"`
	ContraRows   []ContraRow `yaml:"contra_rows"`
	Note         string      `yaml:"note"`

	// Derived marks a slug that is not a mechanical transform of a printed
	// string. It obliges Rationale and SourceNote, and Load refuses an entry
	// that claims one without the others: published and derived must not be
	// presented alike.
	Derived    bool   `yaml:"derived"`
	Rationale  string `yaml:"rationale"`
	SourceNote string `yaml:"source_note"`

	// Assignable reports whether a mapping rule may write this slug as a
	// `category:`. It is not decoded directly — the YAML key defaults to true
	// by absence, which a bool field cannot express — but is set by Load from
	// categoryEntry.
	Assignable bool `yaml:"-"`
}

// clone deep-copies the slice fields. Category is returned by value, which
// copies slice headers only, so without this a caller could write through
// Pages or Aliases into the registry's own state and change what every later
// lookup sees.
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

// Fund is one entry in funds.yaml.
//
// Number, Name and Type are published (appendix pp. 253-257).
// ConstraintTier and RestrictionNote are DERIVED from the Description of
// Funds narrative and must not be presented as something the city printed.
type Fund struct {
	Number          int    `yaml:"number"`
	Name            string `yaml:"name"`
	Type            string `yaml:"type"`
	ConstraintTier  string `yaml:"constraint_tier"`
	RestrictionNote string `yaml:"restriction_note"`

	// Aliases are the other spellings the city prints for this fund. Name is
	// the appendix's (pp. 253-257) and the schedules do not repeat it: p76
	// prints "Low Income Hsng", p136 "Police Evidence", p140 "Facilities
	// Rehab Pgm". Each is published text with the page it was read from, and
	// FundByLabel resolves it — a schedule that names a fund in prose has no
	// fund number to join on otherwise.
	Aliases []Alias `yaml:"aliases"`

	// Major is set only where the budget book itself says so, which is why it
	// is absent rather than false for the internal service funds: those
	// schedules do group them under "Major Funds", but the label there is
	// presentational.
	Major bool `yaml:"major"`
}

// clone deep-copies the slice fields, for the same reason Category.clone
// does: a caller must not be able to write through Aliases into the
// registry's own state.
func (f Fund) clone() Fund {
	f.Aliases = slices.Clone(f.Aliases)
	for i := range f.Aliases {
		f.Aliases[i].Pages = slices.Clone(f.Aliases[i].Pages)
	}
	return f
}

// Department is one of the eleven ALL-CAPS headings Budget Book pp.167-170
// print over their division rows.
//
// A FACT NEVER NAMES ONE. A mapping row's `department:` carries a DIVISION
// slug, because the division is the level the object rows sit under; the
// department is reached through Division.Department, and it is what the
// `<DEPARTMENT> TOTAL` rollups assert. This tier exists so that grouping is
// recorded once, in the file, rather than re-derived from the page by every
// reader.
//
// DocumentTerm is the heading as printed. It is not always the total row's
// wording: PUBLIC WORKS (p169:47) closes as "PUBLIC WORKS DEPARTMENT TOTAL"
// (p170:11), and INNOVATION & ECONOMIC DEVELOPMENT (p167:53) closes with the
// city's own misspelling. An anchor derived from this field would be wrong
// twice, which is why the rule file spells its anchors out.
type Department struct {
	Slug         string `yaml:"slug"`
	Label        string `yaml:"label"`
	DocumentTerm string `yaml:"document_term"`
	Pages        []int  `yaml:"pages"`
	Note         string `yaml:"note"`

	// Derived carries the fourth provenance invariant onto this axis, on the
	// same terms Category states it. Nothing in departments.yaml is derived
	// today — the slugs are transforms of printed labels and the parentage is
	// printed — but a later department that has to be inferred must not be
	// presented like one the city printed.
	Derived    bool   `yaml:"derived"`
	Rationale  string `yaml:"rationale"`
	SourceNote string `yaml:"source_note"`
}

// clone deep-copies the slice fields, for the reason Category.clone does: the
// value is returned by copy, which copies a slice header only, so without this
// a caller could write through Pages into the registry's own state.
func (d Department) clone() Department {
	d.Pages = slices.Clone(d.Pages)
	return d
}

// Division is one of the twenty-three mixed-case row groups pp.167-170 print
// beneath a Department, and it is what a fact's `department` field holds.
//
// The field and this type disagree in name, and deliberately: `department` is
// published in facts.jsonl and in every mapping rule, so renaming it would
// rewrite the audit trail to fix a word. What the string means is recorded
// here instead.
//
// Department names the heading above this division. It is a separate NAMESPACE
// from Slug rather than a parent segment of it: five departments share a name
// with a division beneath them — three exactly, City Council, City Manager and
// City Attorney — so one namespace would force five invented names. It is also
// a field rather than a `police/patrol` slug because check's departmentSlug
// rule is single-segment and fact.RowPath composes `<division>/<category>`; a
// two-segment slug would emit a two-slash row_path.
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
	// fundLabels maps every published spelling — each fund's name and each
	// of its aliases — to the one fund that may claim it. Load rejects a
	// second claimant, so this is a function, not a set of candidates.
	fundLabels map[string]int

	departments     map[string]Department
	departmentSlugs []string // sorted, for enumeration in a stable order
	divisions       map[string]Division
	divisionSlugs   []string // sorted, for enumeration in a stable order
}

// Category returns the taxonomy entry for slug.
//
// The second result is the answer to "does this slug exist", which callers
// need separately from Assignable: a rule naming a rollup and a rule naming a
// typo are both unassignable, and the fixes differ.
func (r *Registry) Category(slug string) (Category, bool) {
	c, ok := r.categories[slug]
	if !ok {
		return Category{}, false
	}
	return c.clone(), true
}

// Label returns the human label for slug — the city's printed words, which is
// what a reader should see in place of our machine identifier.
//
// This is the whole of the interface `internal/project` declares over this
// package, so a change to its signature is a change to the site's contract.
func (r *Registry) Label(slug string) (string, bool) {
	c, ok := r.categories[slug]
	if !ok {
		return "", false
	}
	return c.Label, true
}

// Assignable reports whether a mapping rule may classify a fact as slug.
//
// It is false for a slug that does not exist AND for a rollup node such as
// `taxes`, which exists so a view can name the group and which the taxonomy
// says a rule may never write. Use Category to tell the two apart: "you named
// a rollup, use one of its children" and "you typo'd a slug" need different
// fixes, and a checker that reports only "unassignable" makes the reader
// work the difference out.
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

// Fund returns the registry entry for a fund number.
func (r *Registry) Fund(number int) (Fund, bool) {
	f, ok := r.funds[number]
	if !ok {
		return Fund{}, false
	}
	return f.clone(), true
}

// FundName is the city's own name for a fund number, and whether the registry
// knows the number at all.
//
// It exists beside Fund, which returns the whole entry, because it is the half
// of it that internal/project's Labels interface declares. That interface is
// kept to the methods actually used (byob-interfaces.2) AND to types that do
// not drag this package with them: Fund returns a registry.Fund, so a consumer
// declaring it would import internal/registry, which is the coupling the
// narrow interface exists to avoid.
//
// A miss is not an error, for the same reason Label's is not: an unknown fund
// renders as its number rather than failing a build.
func (r *Registry) FundName(number int) (string, bool) {
	f, ok := r.funds[number]
	if !ok {
		return "", false
	}
	return f.Name, true
}

// FundByLabel returns the fund the city prints as label — its name in
// funds.yaml, or one of the aliases declared there — matched EXACTLY.
//
// Exactness is the point. The schedules abbreviate, and the abbreviations are
// ambiguous rather than merely short: p76 prints "Low Income Hsng", "Traffic
// Imp Fee", "Host Comm Impact", "Measure D" and "State Gas Tax", and each of
// those heads both an operating fund and its CIP twin (200/812, 510/823,
// 282/820, 550/828, 560/834). A prefix or fuzzy match picks one of each pair
// silently, which is the plausible-wrong-value failure this project exists to
// prevent. Which fund each label names is settled by reading the document and
// written down as an alias, once, in funds.yaml.
//
// Ambiguity is therefore rejected when funds.yaml loads and cannot arise
// here: no two funds may claim one label. A label that names no fund is an
// UnknownFundError naming the label, never a nearest match.
func (r *Registry) FundByLabel(label string) (Fund, error) {
	number, ok := r.fundLabels[label]
	if !ok {
		return Fund{}, &UnknownFundError{Label: label}
	}
	return r.funds[number].clone(), nil
}

// Funds returns every fund, ordered by number. The slice and every slice
// inside it are copies, so a caller may sort or trim the result.
//
// Callers need this to enumerate rather than probe: fund numbers are sparse
// (100 to 840, with gaps), so counting or listing them by scanning a range is
// a guess about the document's numbering scheme.
func (r *Registry) Funds() []Fund {
	out := make([]Fund, 0, len(r.fundNumbers))
	for _, n := range r.fundNumbers {
		out = append(out, r.funds[n].clone())
	}
	return out
}

// ConstraintTier is the spending constraint recorded for a fund: how tightly
// the money is tied down, which is the question a reader of the published
// site actually has about a large balance.
//
// It returns "" for a fund the registry does not list. That is a different
// answer from the tier "unknown", which is a real classification meaning the
// budget document does not establish a restriction — do not collapse them.
func (r *Registry) ConstraintTier(fund int) string {
	return r.funds[fund].ConstraintTier
}

// FundGroup reports whether name is a fund type funds.yaml actually uses.
//
// The answer comes from the loaded file rather than from this package's known
// set, so a group that no longer has any fund in it stops validating. That is
// the intended reading of `fund_group:` on a spine column: it names a block of
// funds in the document, and a block with no funds is not a block.
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
//
// The name mismatch is deliberate and is explained on [Division]: the fact
// field is published and the type says what the string means.
func (r *Registry) Division(slug string) (Division, bool) {
	d, ok := r.divisions[slug]
	if !ok {
		return Division{}, false
	}
	return d.clone(), true
}

// Department returns the departments.yaml entry for slug — the ALL-CAPS tier,
// which no fact names directly.
//
// It exists because Division.Department is a slug, and a parent nothing can
// resolve is a field a consumer cannot use: whoever holds a Division and wants
// the heading above it needs this.
func (r *Registry) Department(slug string) (Department, bool) {
	d, ok := r.departments[slug]
	if !ok {
		return Department{}, false
	}
	return d.clone(), true
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
// The files are long — 112 funds, 25 categories, both hand-written — so an
// error that says only "duplicate slug" costs the reader a search through a
// file whose whole point is that its entries are hard to tell apart.
type Error struct {
	// File is FundsFile, TaxonomyFile, DepartmentsFile or SourcesFile.
	File string
	// Entry names the offending fund, category, division or source, already
	// rendered ("fund 291", `category "taxes/property"`). It is empty for a
	// file-level problem such as a bad schema_version.
	Entry string
	Field string
	Msg   string
}

// UnknownFundError reports a printed label that names no fund. It is its own
// type so a caller mapping a schedule can tell "this page names a fund we
// have not written down" from any other failure, and can report every such
// label at once instead of stopping at the first: the fix is an edit to
// funds.yaml, and a reader wants the whole list.
type UnknownFundError struct {
	// Label is the string as printed, unnormalized.
	Label string
}

func (e *UnknownFundError) Error() string {
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

// labeler is the interface `internal/project` declares to get the city's
// words onto the site. Asserting it here means a signature change breaks this
// package's own build instead of a consumer's, which is the only way a narrow
// interface owned by the consumer stays honest.
type labeler interface {
	Label(slug string) (string, bool)
}

var _ labeler = (*Registry)(nil)
