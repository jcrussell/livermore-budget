package projecttest

// Labels stands in for internal/registry, keyed by data/taxonomy.yaml
// category slug; every fund, division and department lookup misses.
type Labels map[string]string

// Label is the transcribed label for a category slug.
func (s Labels) Label(slug string) (string, bool) {
	l, ok := s[slug]
	return l, ok
}

// FundName misses: the spine names no fund.
func (Labels) FundName(int) (string, bool) { return "", false }

// FundType misses, inventing no parent edge.
func (Labels) FundType(int) (string, bool) { return "", false }

// FundGroup knows no fund type.
func (Labels) FundGroup(string) bool { return false }

// ConstraintTier is no tier.
func (Labels) ConstraintTier(int) string { return "" }

// RestrictionNote is no note.
func (Labels) RestrictionNote(int) string { return "" }

// DivisionLabel misses: the spine keys on no division.
func (Labels) DivisionLabel(string) (string, bool) { return "", false }

// DepartmentLabel misses, as DivisionLabel does.
func (Labels) DepartmentLabel(string) (string, bool) { return "", false }

// LinesPrintedAs returns no line: the spine prints no revenue rows under its
// categories.
func (Labels) LinesPrintedAs(string, string, string) []string { return nil }

// Terms answers no spelling, so no line is its category printed whole.
func (Labels) Terms(string) []string { return nil }

// SpineLabels is every category label data/taxonomy.yaml carries for the
// spine, transcribed; a projection's own built-in labels answer for the rest.
var SpineLabels = Labels{
	"taxes/property":            "Property Taxes",
	"taxes/sales":               "Sales Taxes",
	"taxes/other":               "Other Taxes",
	"licenses-and-permits":      "Licenses & Permits",
	"fines-and-forfeitures":     "Fines & Forfeitures",
	"intergovernmental":         "Intergovernmental",
	"charges-for-services":      "Charges for Services",
	"use-of-money-and-property": "Use of Money And Property",
	"contributions-outsourced":  "Contributions Outsourced",
	"miscellaneous-revenue":     "Miscellaneous Revenue",
	"wages-and-benefits":        "Wages & Benefits",
	"services-and-supplies":     "Services & Supplies",
	"capital-outlay":            "Capital Outlay",
	"debt-services":             "Debt Services",
	"transfers/in":              "Transfers In",
	"transfers/out":             "Transfers Out",
}
