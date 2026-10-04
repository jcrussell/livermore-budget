package project

import "testing"

// wordsFor answers the four questions nodeLabel asks, from maps; the rest of
// the labels interface is never asked and is left nil.
type wordsFor struct {
	labels
	funds                              map[int]string
	divisions, departments, categories map[string]string
}

func (w wordsFor) FundName(n int) (string, bool) { s, ok := w.funds[n]; return s, ok }
func (w wordsFor) DivisionLabel(d string) (string, bool) {
	s, ok := w.divisions[d]
	return s, ok
}
func (w wordsFor) DepartmentLabel(d string) (string, bool) {
	s, ok := w.departments[d]
	return s, ok
}
func (w wordsFor) Label(slug string) (string, bool) { s, ok := w.categories[slug]; return s, ok }

// TestEveryDocumentNamesANodeByOneCascade holds nodeLabel, the one cascade
// every projection's label reads: a built-in before the registry, a fund's
// name under each id form a fund takes, a division's and a department's words
// under their own prefixes, the registry's words for the slug, and the id
// otherwise. Mutation: drop the department arm, and the department/ case goes
// red.
func TestEveryDocumentNamesANodeByOneCascade(t *testing.T) {
	w := wordsFor{
		funds:       map[int]string{100: "General Fund"},
		divisions:   map[string]string{"city-clerk": "City Clerk"},
		departments: map[string]string{"city-manager": "CITY MANAGER"},
		categories: map[string]string{
			"taxes/property":                "Property Taxes",
			"fund-balance/reserve-increase": "Reserve Increase / (Use)",
		},
	}
	for _, c := range []struct{ id, slug, want string }{
		{PrefixFundGroup + "general", "", "General Fund"},
		{CategoryFundBalanceReserveIncrease, CategoryFundBalanceReserveIncrease, "Addition to Reserves"},
		{PrefixFund + "100", "", "General Fund"},
		{PrefixTransferFrom + "100", "", "General Fund"},
		{PrefixTransferTo + "100", "", "General Fund"},
		{PrefixDept + "city-clerk", "", "City Clerk"},
		{PrefixDepartment + "city-manager", "", "CITY MANAGER"},
		{"revenue/taxes/property", "taxes/property", "Property Taxes"},
		{PrefixFund + "999", "", slugLabel(PrefixFund + "999")},
		{"revenue/taxes/unknown", "taxes/unknown", slugLabel("revenue/taxes/unknown")},
	} {
		if got := nodeLabel(w, c.id, c.slug); got != c.want {
			t.Errorf("nodeLabel(%q, %q) = %q, want %q", c.id, c.slug, got, c.want)
		}
	}
	if got := nodeLabel(nil, PrefixFund+"100", ""); got != slugLabel(PrefixFund+"100") {
		t.Errorf("nodeLabel with no registry = %q, want the id's own words", got)
	}
}
