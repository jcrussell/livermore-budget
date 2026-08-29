package registry

import (
	"os"
	"slices"
	"testing"
	"testing/fstest"

	"github.com/google/go-cmp/cmp"
)

// realData is the committed registry set. Tests read it directly: it is the
// only way to catch an edit to funds.yaml, taxonomy.yaml or departments.yaml
// that still parses but breaks a downstream consumer.
const realData = "../../data"

// Fixtures are inline rather than committed under testdata/ so the input and
// the expectation read on one screen.
const validFunds = `
schema_version: 1
funds:
  - number: 100
    name: "General Fund"
    type: general
    constraint_tier: discretionary
    restriction_note: "Available for any general city service."
    major: true
  - number: 610
    name: "Stormwater"
    type: enterprise
    constraint_tier: restricted-by-law
    restriction_note: "Ratepayer charges restricted to storm water service."
    major: true
  - number: 210
    name: "Horizons"
    type: special-revenue
    constraint_tier: unknown
    restriction_note: "The document lumps it with grants but establishes no restriction."
`

// The entries are deliberately out of slug order, so Categories() sorting is
// a real assertion rather than a restatement of the fixture.
const validTaxonomy = `
schema_version: 1
categories:
  - slug: taxes
    label: "Taxes"
    kinds: [revenue]
    assignable: false
    derived: true
    rationale: "A rollup node; the city prints no total over the three tax rows."
    source_note: "Budget Book p. 66."
  - slug: taxes/property
    label: "Property Taxes"
    document_term: "Property Taxes"
    parent: taxes
    kinds: [revenue]
    pages: [66, 127]
    contra_rows:
      - term: "ERAF"
        page: 127
    note: "Two of the detail lines are negative."
  - slug: use-of-money-and-property
    label: "Use of Money And Property"
    document_term: "Use of Money And Property"
    kinds: [revenue]
    pages: [66, 129]
    aliases:
      - term: "Use of Money & Prop"
        pages: [131, 132]
  - slug: debt-services
    label: "Debt Services"
    document_term: "Debt Services"
    kinds: [expenditure]
    pages: [66, 183]
`

// The fixture shares a name between a department and a division on purpose:
// `city-manager` is both, because pp.167-170 print CITY MANAGER over a City
// Manager division and a City Clerk one. If the two tiers ever collapse into a
// single namespace this fixture stops loading, which is the point.
const validDepartments = `
schema_version: 1
departments:
  - {slug: city-manager, label: "City Manager", document_term: "CITY MANAGER", pages: [167]}
  - {slug: police-department, label: "Police Department", document_term: "POLICE DEPARTMENT", pages: [168, 169]}
divisions:
  - {slug: city-manager, label: "City Manager", department: city-manager, pages: [167]}
  - {slug: city-clerk, label: "City Clerk", department: city-manager, pages: [167]}
  - {slug: patrol, label: "Patrol", department: police-department, pages: [168]}
`

// registryFS builds an in-memory data/ directory. An empty body means "use
// the valid fixture", so a rejection case shows only the file it breaks.
func registryFS(t *testing.T, funds, taxonomy, departments string) fstest.MapFS {
	t.Helper()
	if funds == "" {
		funds = validFunds
	}
	if taxonomy == "" {
		taxonomy = validTaxonomy
	}
	if departments == "" {
		departments = validDepartments
	}
	return fstest.MapFS{
		FundsFile:       &fstest.MapFile{Data: []byte(funds)},
		TaxonomyFile:    &fstest.MapFile{Data: []byte(taxonomy)},
		DepartmentsFile: &fstest.MapFile{Data: []byte(departments)},
	}
}

func load(t *testing.T, funds, taxonomy, departments string) *Registry {
	t.Helper()
	r, err := Load(registryFS(t, funds, taxonomy, departments))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return r
}

func TestCategoryReadsTheWholeEntry(t *testing.T) {
	r := load(t, "", "", "")

	got, ok := r.Category("taxes/property")
	if !ok {
		t.Fatal(`Category("taxes/property") not found`)
	}
	want := Category{
		Slug:         "taxes/property",
		Label:        "Property Taxes",
		DocumentTerm: "Property Taxes",
		Parent:       "taxes",
		Kinds:        []string{"revenue"},
		Pages:        []int{66, 127},
		ContraRows:   []ContraRow{{Term: "ERAF", Page: 127}},
		Note:         "Two of the detail lines are negative.",
		Assignable:   true,
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("Category(\"taxes/property\") mismatch (-want +got):\n%s", diff)
	}

	// An alias is published text and must survive load verbatim, ampersand
	// and all: a mapping rule matches the printed label.
	money, ok := r.Category("use-of-money-and-property")
	if !ok {
		t.Fatal(`Category("use-of-money-and-property") not found`)
	}
	if diff := cmp.Diff([]Alias{{Term: "Use of Money & Prop", Pages: []int{131, 132}}}, money.Aliases); diff != "" {
		t.Errorf("aliases mismatch (-want +got):\n%s", diff)
	}

	if _, ok := r.Category("taxes/propery"); ok {
		t.Error(`Category("taxes/propery") = ok, want a typo'd slug to be unknown`)
	}
}

func TestLabelIsTheCitysWord(t *testing.T) {
	r := load(t, "", "", "")

	got, ok := r.Label("use-of-money-and-property")
	if !ok {
		t.Fatal(`Label("use-of-money-and-property") not found`)
	}
	if want := "Use of Money And Property"; got != want {
		t.Errorf("Label = %q, want %q", got, want)
	}
	if _, ok := r.Label("nope"); ok {
		t.Error(`Label("nope") = ok, want false`)
	}
}

// A rollup and a typo are both unassignable, and the fixes differ: pick a
// child versus correct the spelling. Category is what separates them.
func TestAssignableSeparatesARollupFromATypo(t *testing.T) {
	r := load(t, "", "", "")

	if r.Assignable("taxes") {
		t.Error(`Assignable("taxes") = true, want false: it is a rollup node`)
	}
	if _, ok := r.Category("taxes"); !ok {
		t.Error(`Category("taxes") = not found, want the rollup to exist`)
	}

	if r.Assignable("taxs") {
		t.Error(`Assignable("taxs") = true, want false`)
	}
	if _, ok := r.Category("taxs"); ok {
		t.Error(`Category("taxs") = ok, want false`)
	}

	if !r.Assignable("taxes/property") {
		t.Error(`Assignable("taxes/property") = false, want true`)
	}
}

func TestCategoriesAreSortedCopies(t *testing.T) {
	r := load(t, "", "", "")

	got := r.Categories()
	var slugs []string
	for _, c := range got {
		slugs = append(slugs, c.Slug)
	}
	want := []string{"debt-services", "taxes", "taxes/property", "use-of-money-and-property"}
	if diff := cmp.Diff(want, slugs); diff != "" {
		t.Errorf("Categories() slugs mismatch (-want +got):\n%s", diff)
	}

	// Writing through a returned value must not reach the registry: the
	// site and `verify` share one Registry, and a consumer that trimmed a
	// Pages slice would change what the other one reads.
	for i := range got {
		got[i].Slug = "clobbered"
		got[i].Kinds = append(got[i].Kinds[:0], "clobbered")
		if len(got[i].Pages) > 0 {
			got[i].Pages[0] = -1
		}
		for j := range got[i].Aliases {
			got[i].Aliases[j].Pages[0] = -1
		}
	}
	again, ok := r.Category("taxes/property")
	if !ok {
		t.Fatal(`Category("taxes/property") not found after mutating a copy`)
	}
	if diff := cmp.Diff([]int{66, 127}, again.Pages); diff != "" {
		t.Errorf("pages mismatch after mutating a copy (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]string{"revenue"}, again.Kinds); diff != "" {
		t.Errorf("kinds mismatch after mutating a copy (-want +got):\n%s", diff)
	}
	money, _ := r.Category("use-of-money-and-property")
	if diff := cmp.Diff([]int{131, 132}, money.Aliases[0].Pages); diff != "" {
		t.Errorf("alias pages mismatch after mutating a copy (-want +got):\n%s", diff)
	}
}

func TestFunds(t *testing.T) {
	r := load(t, "", "", "")

	got, ok := r.Fund(610)
	if !ok {
		t.Fatal("Fund(610) not found")
	}
	want := Fund{
		Number:          610,
		Name:            "Stormwater",
		Type:            "enterprise",
		ConstraintTier:  "restricted-by-law",
		RestrictionNote: "Ratepayer charges restricted to storm water service.",
		Major:           true,
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("Fund(610) mismatch (-want +got):\n%s", diff)
	}

	// `major` is absent, not false, for most funds; absence reads as false
	// because the budget book only ever says so positively.
	horizons, ok := r.Fund(210)
	if !ok {
		t.Fatal("Fund(210) not found")
	}
	if horizons.Major {
		t.Error("Fund(210).Major = true, want false for an unlabelled fund")
	}

	if _, ok := r.Fund(999); ok {
		t.Error("Fund(999) = ok, want false")
	}
}

// "we could not classify this fund" and "this fund does not exist" are
// different failures and only the first is normal.
func TestConstraintTierSeparatesUnknownFromAbsent(t *testing.T) {
	r := load(t, "", "", "")

	if got, want := r.ConstraintTier(210), "unknown"; got != want {
		t.Errorf("ConstraintTier(210) = %q, want %q", got, want)
	}
	if got, want := r.ConstraintTier(100), "discretionary"; got != want {
		t.Errorf("ConstraintTier(100) = %q, want %q", got, want)
	}
	if got := r.ConstraintTier(999); got != "" {
		t.Errorf("ConstraintTier(999) = %q, want %q for a fund that is not listed", got, "")
	}
}

func TestFundGroupsComeFromTheFile(t *testing.T) {
	r := load(t, "", "", "")

	got := r.FundGroups()
	want := []string{"enterprise", "general", "special-revenue"}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("FundGroups() mismatch (-want +got):\n%s", diff)
	}

	for _, name := range want {
		if !r.FundGroup(name) {
			t.Errorf("FundGroup(%q) = false, want true", name)
		}
	}
	// A type this package knows about but the loaded file does not use is
	// not a group: `fund_group:` names a block of funds in the document.
	if r.FundGroup("permanent") {
		t.Error(`FundGroup("permanent") = true, want false when no fund has that type`)
	}
	if r.FundGroup("Enterprise") {
		t.Error(`FundGroup("Enterprise") = true, want false: the values are lowercase`)
	}

	got[0] = "clobbered"
	if again := r.FundGroups(); again[0] != "enterprise" {
		t.Errorf("FundGroups()[0] = %q after mutating a copy, want %q", again[0], "enterprise")
	}
}

// The renaming that taxonomy.yaml's header warns about, checked against the
// real files: `debt-service` is a fund TYPE and `debt-services` an
// expenditure CATEGORY. Neither string may resolve on the other axis.
func TestDebtServiceIsNotDebtServices(t *testing.T) {
	r, err := Load(os.DirFS(realData))
	if err != nil {
		t.Fatalf("Load(%s): %v", realData, err)
	}

	if !r.FundGroup(fundTypeDebtService) {
		t.Errorf("FundGroup(%q) = false, want true", fundTypeDebtService)
	}
	if _, ok := r.Category(fundTypeDebtService); ok {
		t.Errorf("Category(%q) = ok, want false: it is a fund type, not a category",
			fundTypeDebtService)
	}
	if !r.Assignable(categoryDebtServices) {
		t.Errorf("Assignable(%q) = false, want true", categoryDebtServices)
	}
	if r.FundGroup(categoryDebtServices) {
		t.Errorf("FundGroup(%q) = true, want false: it is a category, not a fund type",
			categoryDebtServices)
	}
}

func numbers(funds []Fund) []int {
	out := make([]int, 0, len(funds))
	for _, f := range funds {
		out = append(out, f.Number)
	}
	return out
}

// The counts are the check that a registry edit which still parses has not
// quietly changed what a downstream consumer sees.
func TestLoadRealRegistries(t *testing.T) {
	r, err := Load(os.DirFS(realData))
	if err != nil {
		t.Fatalf("Load(%s): %v", realData, err)
	}

	cats := r.Categories()
	if got, want := len(cats), 26; got != want {
		t.Errorf("len(Categories()) = %d, want %d", got, want)
	}
	if got, want := len(r.FundGroups()), 7; got != want {
		t.Errorf("len(FundGroups()) = %d, want %d", got, want)
	}
	wantGroups := []string{"capital", "debt-service", "enterprise", "general",
		"internal-service", "permanent", "special-revenue"}
	if diff := cmp.Diff(wantGroups, r.FundGroups()); diff != "" {
		t.Errorf("FundGroups() mismatch (-want +got):\n%s", diff)
	}

	// Every category must carry a label, because that is the string the site
	// prints in place of the slug.
	var unassignable, derived []string
	for _, c := range cats {
		if c.Label == "" {
			t.Errorf("category %q has no label", c.Slug)
		}
		if !c.Assignable {
			unassignable = append(unassignable, c.Slug)
		}
		if c.Derived {
			derived = append(derived, c.Slug)
			if c.Rationale == "" || c.SourceNote == "" {
				t.Errorf("derived category %q has rationale %q and source_note %q, want both set",
					c.Slug, c.Rationale, c.SourceNote)
			}
		}
	}
	// The three rollup nodes, and only those three: adding a fourth changes
	// what a mapping rule may write and must be a deliberate edit here too.
	wantUnassignable := []string{"fund-balance", "taxes", "transfers"}
	if diff := cmp.Diff(wantUnassignable, unassignable); diff != "" {
		t.Errorf("non-assignable slugs mismatch (-want +got):\n%s", diff)
	}
	if got, want := len(derived), 7; got != want {
		t.Errorf("derived categories = %d %v, want %d", got, derived, want)
	}

	// THE NEW VALIDATORS MUST BE EXERCISED BY THE REAL FILE, not only by
	// fixtures. Load already refuses a bad kind, a mis-shaped `pages`, a
	// mis-shaped alias and a contra row off its category's pages -- so
	// re-asserting those here over a registry that LOADED would be a
	// tautology. What is not a tautology is that the committed taxonomy
	// actually carries entries of each kind: an arm no real entry reaches is
	// dead code that happens to have a test.
	var withAliases, withContra, kindMembers int
	var pageless []string
	for _, c := range cats {
		kindMembers += len(c.Kinds)
		if len(c.Pages) == 0 {
			pageless = append(pageless, c.Slug)
		}
		withAliases += len(c.Aliases)
		withContra += len(c.ContraRows)
	}
	if kindMembers == 0 {
		t.Error("no category declares any kinds; the membership arm is unexercised")
	}
	// Three of the twenty-five carry no pages, and they are exactly the three
	// rollups above -- which is why the shape arm is applied and the presence
	// arm is not. If that stops being true, "required when assignable" becomes
	// landable and should be taken.
	//
	// COMPARE THE SLUGS, NOT THE COUNT. A count-only assertion stays green if
	// a rollup gains pages while an assignable category loses them -- and that
	// swap would also silently take a real category out of reach of the
	// contra-row arm, which is checked against `pages`.
	if diff := cmp.Diff(wantUnassignable, pageless); diff != "" {
		t.Errorf("categories carrying no pages mismatch (-want +got):\n%s", diff)
	}
	if withAliases == 0 {
		t.Error("no category carries an alias; validateAlias is unexercised on this channel")
	}
	if withContra == 0 {
		t.Error("no category carries a contra row; the contra_rows arms are unexercised")
	}

	// 112 funds, every one with a tier this package recognizes.
	funds := r.Funds()
	if got, want := len(funds), 112; got != want {
		t.Errorf("len(Funds()) = %d, want %d", got, want)
	}
	for _, f := range funds {
		if !slices.Contains(constraintTiers, f.ConstraintTier) {
			t.Errorf("fund %d has constraint_tier %q", f.Number, f.ConstraintTier)
		}
		if !slices.Contains(wantGroups, f.Type) {
			t.Errorf("fund %d has type %q", f.Number, f.Type)
		}
	}
	if !slices.IsSorted(numbers(funds)) {
		t.Error("Funds() is not ordered by number")
	}

	// Spot checks that the accessors read the file rather than a default:
	// fund 291's name follows p259 and not the appendix's "Community
	// Beneift Fund" typo, and the taxonomy's rollups stay unassignable.
	f291, ok := r.Fund(291)
	if !ok {
		t.Fatal("Fund(291) not found")
	}
	if got, want := f291.Name, "Community Benefit Fund"; got != want {
		t.Errorf("Fund(291).Name = %q, want %q", got, want)
	}
	if got, want := r.ConstraintTier(100), "discretionary"; got != want {
		t.Errorf("ConstraintTier(100) = %q, want %q", got, want)
	}
	label, ok := r.Label("use-of-money-and-property")
	if !ok {
		t.Fatal(`Label("use-of-money-and-property") not found`)
	}
	if want := "Use of Money And Property"; label != want {
		t.Errorf("Label = %q, want %q; the capital A is the document's", label, want)
	}
}

// The committed department registry, checked against what Budget Book
// pp.167-170 print. The counts are the assertion that an edit which still
// parses has not quietly changed the axis: a dropped division is invisible to
// every object-category sum, because its money simply moves to no other row.
func TestDepartmentsRegistryMatchesThePages(t *testing.T) {
	r, err := Load(os.DirFS(realData))
	if err != nil {
		t.Fatalf("Load(%s): %v", realData, err)
	}

	if got, want := len(r.Departments()), 11; got != want {
		t.Errorf("len(Departments()) = %d, want %d", got, want)
	}
	if got, want := len(r.Divisions()), 23; got != want {
		t.Errorf("len(Divisions()) = %d, want %d", got, want)
	}

	// Divisions per department, counted off the printed `Total` rows: p167 6,
	// p168 7, p169 8, p170 2. Six departments have exactly one division, which
	// is why six of the eleven <DEPARTMENT> TOTAL rows cover a single rule.
	want := map[string]int{
		"administrative-services":             3,
		"city-attorney":                       1,
		"city-council":                        1,
		"city-manager":                        2,
		"community-development":               5,
		"fire-department":                     1,
		"general-services":                    1,
		"innovation-and-economic-development": 1,
		"library-department":                  1,
		"police-department":                   4,
		"public-works":                        3,
	}
	got := map[string]int{}
	for _, d := range r.Divisions() {
		got[d.Department]++
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("divisions per department mismatch (-want +got):\n%s", diff)
	}

	// THE TWO-NAMESPACE DECISION, pinned against the real file. These five
	// strings name a department AND a division, three of them exactly as
	// printed. A single namespace would need five invented names, so if this
	// ever passes with a shared namespace it is because someone renamed the
	// city's words.
	for _, slug := range []string{
		"city-council", "city-manager", "city-attorney",
		"general-services", "administrative-services",
	} {
		if _, ok := r.Department(slug); !ok {
			t.Errorf("Department(%q) not found; it names both tiers", slug)
		}
		if _, ok := r.Division(slug); !ok {
			t.Errorf("Division(%q) not found; it names both tiers", slug)
		}
	}

	// The cross-AXIS refusal, which does not relax. Load enforces it; this
	// says so against the committed pair rather than against a fixture.
	for _, d := range r.Divisions() {
		if _, isCategory := r.Category(d.Slug); isCategory {
			t.Errorf("division %q is also a taxonomy category slug", d.Slug)
		}
		if d.Label == "" {
			t.Errorf("division %q has no label", d.Slug)
		}
		if _, ok := r.Department(d.Department); !ok {
			t.Errorf("division %q names department %q, which does not resolve", d.Slug, d.Department)
		}
	}

	// Nothing in this file is derived: the slugs are transforms of printed
	// labels and the parentage is printed. An entry that becomes derived must
	// carry its rationale, and this is what makes that visible rather than
	// letting the first one through unnoticed.
	for _, d := range r.Divisions() {
		if d.Derived {
			t.Errorf("division %q is derived; departments.yaml records only published names", d.Slug)
		}
	}
	for _, d := range r.Departments() {
		if d.Derived {
			t.Errorf("department %q is derived; departments.yaml records only published names", d.Slug)
		}
		if d.DocumentTerm == "" {
			t.Errorf("department %q has no document_term", d.Slug)
		}
	}
}

// PUBLIC WORKS is the one department whose printed total row is not its
// heading plus " TOTAL" (p169:47 against p170:11), so a rule that derived the
// rollup anchor from document_term would be wrong exactly here. The registry
// records the heading; this pins that it is the heading and not the total.
func TestPublicWorksHeadingIsNotItsTotalRow(t *testing.T) {
	r, err := Load(os.DirFS(realData))
	if err != nil {
		t.Fatalf("Load(%s): %v", realData, err)
	}
	d, ok := r.Department("public-works")
	if !ok {
		t.Fatal(`Department("public-works") not found`)
	}
	if got, want := d.DocumentTerm, "PUBLIC WORKS"; got != want {
		t.Errorf("DocumentTerm = %q, want %q", got, want)
	}
	if d.DocumentTerm+" TOTAL" == "PUBLIC WORKS DEPARTMENT TOTAL" {
		t.Error("document_term + \" TOTAL\" now equals the printed total row; " +
			"the asymmetry this test records has gone, so check p170:11")
	}
}
