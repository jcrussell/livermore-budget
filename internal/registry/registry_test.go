package registry

import (
	"fmt"
	"os"
	"slices"
	"strings"
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
		ContraRows:   []contraRow{{Term: "ERAF", Page: 127}},
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
	if diff := cmp.Diff([]alias{{Term: "Use of Money & Prop", Pages: []int{131, 132}}}, money.Aliases); diff != "" {
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
	// Plus one line entry per distinct revenue row pp.127-140 print: 101,
	// measured off facts/facts.jsonl on 2026-09-12.
	if got, want := len(cats), 41+101; got != want {
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
	if got, want := len(derived), 17; got != want {
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
	// Three categories carry no pages, and they are exactly the three
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

// The committed department registry, checked against what the budget book
// prints. The counts are the assertion that an edit which still parses has not
// quietly changed the axis: a dropped division is invisible to every
// object-category sum, because its money simply moves to no other row.
//
// The division counts come from pp.85-125, which print the axis whole.
// TestEveryDivisionIsPrintedOnItsPages is what ties each entry to a page.
func TestDepartmentsRegistryMatchesThePages(t *testing.T) {
	r, err := Load(os.DirFS(realData))
	if err != nil {
		t.Fatalf("Load(%s): %v", realData, err)
	}

	if got, want := len(r.Departments()), 11; got != want {
		t.Errorf("len(Departments()) = %d, want %d", got, want)
	}
	if got, want := len(r.Divisions()), 29; got != want {
		t.Errorf("len(Divisions()) = %d, want %d", got, want)
	}

	// Divisions per department, counted off the printed `Division Total` rows
	// on pp.85-125, which print the whole axis: p85 1, p89 2, p93 2, p97 5,
	// p101 5, p105 1, p108 1, p111 2, p115 1, p119 5, p124 4. pp.167-170 print
	// 23 of these 29 — the General Fund slice — so a count taken there is
	// smaller by the six the registry header names.
	want := map[string]int{
		"administrative-services":             5,
		"city-attorney":                       2,
		"city-council":                        1,
		"city-manager":                        2,
		"community-development":               5,
		"fire-department":                     1,
		"general-services":                    1,
		"innovation-and-economic-development": 2,
		"library-department":                  1,
		"police-department":                   5,
		"public-works":                        4,
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
		if _, ok := r.departmentEntry(slug); !ok {
			t.Errorf("Department(%q) not found; it names both tiers", slug)
		}
		if _, ok := r.Division(slug); !ok {
			t.Errorf("Division(%q) not found; it names both tiers", slug)
		}
	}

	// The cross-axis refusal over both tiers, against the committed data.
	for _, d := range r.Departments() {
		if _, isCategory := r.Category(d.Slug); isCategory {
			t.Errorf("department %q is also a taxonomy category slug", d.Slug)
		}
	}
	for _, d := range r.Divisions() {
		if _, isCategory := r.Category(d.Slug); isCategory {
			t.Errorf("division %q is also a taxonomy category slug", d.Slug)
		}
		if d.Label == "" {
			t.Errorf("division %q has no label", d.Slug)
		}
		if _, ok := r.departmentEntry(d.Department); !ok {
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
	d, ok := r.departmentEntry("public-works")
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

// A division's `pages` asserts that the city prints its label on those pages.
// This checks that against the extracted text, so a page number typed from
// memory fails rather than sitting in the registry looking authoritative. It is
// TestFundAliasesArePrintedOnTheirPages applied to the other axis, and it is
// what makes the six pp.85-125-only entries a claim rather than an assertion.
//
// THE LABELS WRAP, THE TWO SCHEDULES WRAP THEM AT DIFFERENT POINTS, AND THE
// CONTINUATION IS NOT ADJACENT TO WHAT IT CONTINUES. Four divisions print
// across two lines wherever they appear — "Community Development Admin",
// "Housing & Human Services", "Innovation & Economic Devel" and "Public Works
// Administration". Collapsing the whole page finds none of them, because the
// second half sits BELOW the first object row rather than beside the first
// half, with that row's four figures in between:
//
//	p0101:11  Community Development Wages & Benefits   $803,509  ...
//	p0101:12  Admin
//	p0169:15  Community             Wages & Benefits    803,509  ...
//	p0169:16  Development Admin
//
// Note the two schedules break the same label in different places. So the
// search reads the LABEL COLUMN: each line is cut at the first object-category
// name, which is the closed vocabulary that delimits that column on both
// schedules, and the remainders are joined in order.
//
// AND THE SEARCH STOPS AT "Department Funding Sources", which is the arm that
// makes this substantiate anything. pp.85-125 print a division block ABOVE a
// funding-source block, and four of the labels name a division in one and a FUND
// in the other -- Information Technology, Airport, Library and Horizons, the
// same four whose rules need a section_ordinal for the same reason. Searching
// the whole page, three of the six divisions this test was written to
// substantiate passed on a fund row rather than on a division row, and a
// fabricated `general-liability` division on p93 would have passed too. Cutting
// the page at that heading is what tells the two blocks apart.
//
// This asserts presence and not position — the same claim
// TestFundAliasesArePrintedOnTheirPages makes, "this is printed there, go and
// look". Joining the column can in principle spell a label across two unrelated
// divisions; what it cannot do is find one on a page that has no row for it,
// which is the error being guarded against.
//
// Mutation: change any division's pages to a page that does not print it — 167
// for water-resources, say, whose whole point is that pp.167-170 have no row
// for it — and this fails naming the division and the page.
func TestEveryDivisionIsPrintedOnItsPages(t *testing.T) {
	r := realRegistry(t)

	pages := map[int]string{}
	readPage := func(t *testing.T, n int) string {
		t.Helper()
		if body, ok := pages[n]; ok {
			return body
		}
		b, err := os.ReadFile(fmt.Sprintf("%s/p%04d.txt", budgetPages, n))
		if err != nil {
			t.Fatalf("read page %d: %v", n, err)
		}
		// Everything below "Department Funding Sources" is the other block on
		// the page, which names FUNDS. pp.167-170 carry no such heading, so the
		// cut is a no-op there.
		text, _, _ := strings.Cut(string(b), "Department Funding Sources")
		pages[n] = labelColumn(text)
		return pages[n]
	}

	var claims int
	for _, d := range r.Divisions() {
		want := strings.Join(strings.Fields(d.Label), " ")
		for _, p := range d.Pages {
			claims++
			if !strings.Contains(readPage(t, p), want) {
				t.Errorf("division %q label %q is not printed on p%04d", d.Slug, d.Label, p)
			}
		}
	}

	// THE FLOOR IS A COUNT AGAINST THE DOCUMENTS, and it has to be, because the
	// two obvious alternatives are both defective and both were tried.
	//
	// A floor of 29 -- one claim per division -- lets 26 of the 55 vanish with
	// the guard green, which is most of the second schedule.
	//
	// Recomputing the expected total from r.Divisions() is WORSE: `claims` is
	// accumulated by a loop over r.Divisions() and the total would be summed
	// from the same call in the same run, so the two move together and the
	// comparison can never fire. Measured -- with Divisions() truncated to 3 of
	// 29 that version still reported ok, where even the too-low floor failed.
	// It is check.go's own doctrine, that two functions over identical input
	// inside one process cannot witness a wrong amount, in the package next
	// door.
	//
	// So the number is extrinsic: pp.85-125 print all 29 divisions and
	// pp.167-170 print 23 of them, which is 52 page claims the CITY's pages
	// require before any entry cites a third page. It goes stale loudly when a
	// division is added, which is the intended cost.
	if got := len(r.Divisions()); got != 29 {
		t.Errorf("the registry holds %d divisions, want 29; the floor below is a count "+
			"against the pages and means nothing if the axis changed", got)
	}
	if claims < 52 {
		t.Errorf("the loop examined %d page claims, want at least 52: pp.85-125 print all "+
			"29 divisions and pp.167-170 print 23 of them, so that many citations are "+
			"required before any entry names a further page", claims)
	}
}

// TestEveryRevenueLineIsPrintedOnItsPages ties each revenue line entry in
// data/taxonomy.yaml (a child of an assignable category) to the Budget Book
// pages it cites. The match is a row match: the term begins a line and ends at
// a space or the line's end, so "Franchise Tax- Gas" is not satisfied by
// "Franchise Tax- Garbage".
//
// Mutation: change any line's pages to a page that does not print it -- 127 for
// miscellaneous-revenue/cardroom-revenue, which p130 prints -- and this fails
// naming the line and the page.
func TestEveryRevenueLineIsPrintedOnItsPages(t *testing.T) {
	r := realRegistry(t)

	pages := map[int][]string{}
	readPage := func(t *testing.T, n int) []string {
		t.Helper()
		if body, ok := pages[n]; ok {
			return body
		}
		b, err := os.ReadFile(fmt.Sprintf("%s/p%04d.txt", budgetPages, n))
		if err != nil {
			t.Fatalf("read page %d: %v", n, err)
		}
		pages[n] = strings.Split(string(b), "\n")
		return pages[n]
	}
	printsRow := func(page []string, term string) bool {
		for _, line := range page {
			line = strings.TrimLeft(line, " ")
			if strings.HasPrefix(line, term) && (len(line) == len(term) || line[len(term)] == ' ') {
				return true
			}
		}
		return false
	}

	var lines, claims int
	for _, c := range r.Categories() {
		if c.Parent == "" || !r.Assignable(c.Parent) {
			continue
		}
		lines++
		if c.DocumentTerm == "" {
			t.Errorf("line %q has no document_term, so no printed row can resolve to it", c.Slug)
		}
		if len(c.Pages) == 0 {
			t.Errorf("line %q cites no page; a printed row is printed somewhere", c.Slug)
		}
		for _, p := range c.Pages {
			claims++
			if !printsRow(readPage(t, p), c.DocumentTerm) {
				t.Errorf("line %q document_term %q does not begin a row of p%04d", c.Slug, c.DocumentTerm, p)
			}
		}
		for _, a := range c.Aliases {
			for _, p := range a.Pages {
				claims++
				if !printsRow(readPage(t, p), a.Term) {
					t.Errorf("line %q alias %q does not begin a row of p%04d", c.Slug, a.Term, p)
				}
			}
		}
	}

	// Floors counted against the pages, not summed from r.Categories() in the
	// same run. Measured off facts/facts.jsonl on 2026-09-12: 101 distinct
	// (category, row_label) pairs on 139 distinct (pair, page) combinations.
	if lines != 101 {
		t.Errorf("the registry declares %d revenue lines, want 101; the floor below is a "+
			"count against the pages and means nothing if the schedule changed", lines)
	}
	if claims < 139 {
		t.Errorf("the loop examined %d page claims, want at least 139: that many (row, page) "+
			"pairs are printed before any entry cites a further page", claims)
	}
}

// labelColumn is the left-hand column of a departmentwide or major-category
// schedule: every line cut at the EARLIEST object-category name on it, joined in
// order. objectColumn is the closed vocabulary the budget book prints in the
// column to its right.
//
// The cut is the minimum index over the whole vocabulary, so the slice's order
// is inert -- "Total" matching inside "Division Total" cannot win, because
// "Division Total" starts nine characters earlier on the same line.
func labelColumn(page string) string {
	objectColumn := []string{
		"Wages & Benefits", "Services & Supplies", "Capital Outlay",
		"Debt Services", "Transfers Out", "Division Total", "Total",
	}
	var out []string
	for _, line := range strings.Split(page, "\n") {
		cut := len(line)
		for _, o := range objectColumn {
			if i := strings.Index(line, o); i >= 0 && i < cut {
				cut = i
			}
		}
		if f := strings.Fields(line[:cut]); len(f) > 0 {
			out = append(out, strings.Join(f, " "))
		}
	}
	return strings.Join(out, " ")
}

// linesTaxonomy is validTaxonomy with lines under taxes/property: two spellings
// of one row, one row two lines claim, and one line of another kind.
const linesTaxonomy = validTaxonomy + `
  - slug: taxes/property/eraf
    label: "ERAF"
    document_term: "ERAF"
    parent: taxes/property
    kinds: [revenue]
    pages: [127]
    aliases:
      - term: "E.R.A.F."
        pages: [128]
  - slug: taxes/property/rpttf-reduction
    label: "RPTTF Reduction"
    document_term: "RPTTF Reduction"
    parent: taxes/property
    kinds: [revenue]
    pages: [127]
  - slug: taxes/property/rpttf-reduction-again
    label: "RPTTF Reduction"
    document_term: "RPTTF Reduction"
    parent: taxes/property
    kinds: [revenue]
    pages: [127]
  - slug: taxes/property/refunds
    label: "Refunds"
    document_term: "Refunds"
    parent: taxes/property
    kinds: [expenditure]
    pages: [127]
`

// TestLinesPrintedAsAnswersWithEveryClaimant: ambiguity is not resolved here;
// every claimant comes back and the caller refuses.
func TestLinesPrintedAsAnswersWithEveryClaimant(t *testing.T) {
	r := load(t, "", linesTaxonomy, "")

	cases := []struct {
		name                  string
		parent, printed, kind string
		want                  []string
	}{
		{"the document term", "taxes/property", "ERAF", "revenue",
			[]string{"taxes/property/eraf"}},
		{"an alias, because a re-typeset row is the same node", "taxes/property",
			"E.R.A.F.", "revenue", []string{"taxes/property/eraf"}},
		{"two lines claiming one spelling", "taxes/property", "RPTTF Reduction", "revenue",
			[]string{"taxes/property/rpttf-reduction", "taxes/property/rpttf-reduction-again"}},
		{"a line of another kind", "taxes/property", "Refunds", "revenue", nil},
		{"a label is never a match key", "taxes/property", "ERAF ", "revenue", nil},
		{"a spelling under another category", "taxes", "ERAF", "revenue", nil},
		{"an empty spelling matches nothing", "taxes/property", "", "revenue", nil},
		// The method is structural and knows nothing about "line".
		{"a child that is itself a category", "taxes", "Property Taxes", "revenue",
			[]string{"taxes/property"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if diff := cmp.Diff(c.want, r.LinesPrintedAs(c.parent, c.printed, c.kind)); diff != "" {
				t.Errorf("LinesPrintedAs(%q, %q, %q) (-want +got):\n%s",
					c.parent, c.printed, c.kind, diff)
			}
		})
	}
}
