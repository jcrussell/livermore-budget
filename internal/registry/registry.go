// Package registry reads the two curated registries under data/: the fund list
// (funds.yaml) and the category taxonomy (taxonomy.yaml).
//
// These files are the controlled vocabulary behind two of a fact's fields.
// Until something reads them, a mapping rule's `category:` and a column's
// `fund_group:` are free strings, and a rule that writes `tax/property` where
// the taxonomy says `taxes/property` produces a fact that is confidently
// wrong and joins to nothing. This package is what makes those fields
// checkable (fisc-1wr.1), and it is the only place either file is parsed.
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

// FundsFile and TaxonomyFile are the names Load reads from the root of the
// filesystem it is given, so a caller passes os.DirFS("data") rather than
// composing paths this package would have to agree with.
const (
	FundsFile    = "funds.yaml"
	TaxonomyFile = "taxonomy.yaml"
)

// The two registries version independently: a change to the fund schema says
// nothing about the taxonomy schema, and one shared constant would force a
// pointless bump on the other file every time.
const (
	// FundsSchemaVersion is the only funds.yaml version this package reads.
	FundsSchemaVersion = 1
	// TaxonomySchemaVersion is the only taxonomy.yaml version this package reads.
	TaxonomySchemaVersion = 1
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

// Alias is a spelling the CITY prints for a category, on the pages named.
// Every alias is published text, verbatim including its abbreviations, so a
// mapping rule can match printed labels without normalizing them.
type Alias struct {
	Term  string `yaml:"term"`
	Pages []int  `yaml:"pages"`
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

	// Major is set only where the budget book itself says so, which is why it
	// is absent rather than false for the internal service funds: those
	// schedules do group them under "Major Funds", but the label there is
	// presentational.
	Major bool `yaml:"major"`
}

// Registry is the loaded pair of files. It is read-only after Load and safe
// for concurrent use.
type Registry struct {
	categories map[string]Category
	slugs      []string // sorted, for enumeration in a stable order

	funds       map[int]Fund
	fundNumbers []int    // sorted, for enumeration in a stable order
	fundGroups  []string // sorted, deduplicated, as observed in funds.yaml
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
	return f, ok
}

// Funds returns every fund, ordered by number. The result is a copy, and Fund
// carries no slices, so it is safe to modify.
//
// Callers need this to enumerate rather than probe: fund numbers are sparse
// (100 to 840, with gaps), so counting or listing them by scanning a range is
// a guess about the document's numbering scheme.
func (r *Registry) Funds() []Fund {
	out := make([]Fund, 0, len(r.fundNumbers))
	for _, n := range r.fundNumbers {
		out = append(out, r.funds[n])
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

// Error reports a problem in one registry file, naming the entry at fault.
// The files are long — 112 funds, 25 categories, both hand-written — so an
// error that says only "duplicate slug" costs the reader a search through a
// file whose whole point is that its entries are hard to tell apart.
type Error struct {
	// File is FundsFile, TaxonomyFile or SourcesFile.
	File string
	// Entry names the offending fund, category or source, already rendered
	// ("fund 291", `category "taxes/property"`). It is empty for a
	// file-level problem such as a bad schema_version.
	Entry string
	Field string
	Msg   string
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
