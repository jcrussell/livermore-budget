package registry

import (
	"errors"
	"io/fs"
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/google/go-cmp/cmp"

	"github.com/jcrussell/livermore-budget/pkg/cmdutil"
)

// TestLoadRejects covers every way a registry file can be wrong. Each case
// asserts the message names the offending entry: the files are 112 and 25
// entries long, and "duplicate slug" without a slug is a search.
func TestLoadRejects(t *testing.T) {
	tests := []struct {
		name        string
		funds       string // empty means the valid fixture
		taxonomy    string
		departments string
		blank       string // a file to truncate, for the cases an empty body cannot express
		want        string
	}{
		{
			name:  "funds schema version from a newer fisc",
			funds: "schema_version: 2\nfunds: [{number: 100, name: x, type: general, constraint_tier: committed, restriction_note: y}]\n",
			want:  "funds.yaml: schema_version: got 2, want 1",
		}, {
			name:  "funds schema version missing",
			funds: "funds: [{number: 100, name: x, type: general, constraint_tier: committed, restriction_note: y}]\n",
			want:  "funds.yaml: schema_version: got 0, want 1",
		}, {
			name:  "no funds",
			funds: "schema_version: 1\nfunds: []\n",
			want:  "funds.yaml: funds: is empty",
		}, {
			name: "duplicate fund number",
			funds: `
schema_version: 1
funds:
  - {number: 100, name: "General Fund", type: general, constraint_tier: committed, restriction_note: y}
  - {number: 100, name: "General Fund CIP Reserves", type: capital, constraint_tier: committed, restriction_note: y}
`,
			want: `funds.yaml: fund 100: number: duplicate fund number, already defined as "General Fund"`,
		}, {
			name:  "fund with no number",
			funds: "schema_version: 1\nfunds: [{name: \"Water\", type: enterprise, constraint_tier: committed, restriction_note: y}]\n",
			want:  `funds.yaml: funds[0]: number: is 0; a fund is identified by its number (name "Water")`,
		}, {
			name:  "fund with no name",
			funds: "schema_version: 1\nfunds: [{number: 640, type: enterprise, constraint_tier: committed, restriction_note: y}]\n",
			want:  "funds.yaml: fund 640: name: is required",
		}, {
			// The near miss taxonomy.yaml warns about: the plural is the
			// expenditure category, and it is not a fund type.
			name:  "fund type from the other axis",
			funds: "schema_version: 1\nfunds: [{number: 400, name: x, type: debt-services, constraint_tier: committed, restriction_note: y}]\n",
			want:  `funds.yaml: fund 400: type: got "debt-services", want one of general, special-revenue, capital, debt-service`,
		}, {
			name:  "unknown fund type",
			funds: "schema_version: 1\nfunds: [{number: 100, name: x, type: fiduciary, constraint_tier: committed, restriction_note: y}]\n",
			want:  `funds.yaml: fund 100: type: got "fiduciary", want one of`,
		}, {
			name:  "unknown constraint tier",
			funds: "schema_version: 1\nfunds: [{number: 100, name: x, type: general, constraint_tier: restricted_by_law, restriction_note: y}]\n",
			want:  `funds.yaml: fund 100: constraint_tier: got "restricted_by_law", want one of discretionary, restricted-by-law, committed, unknown`,
		}, {
			name:  "constraint tier with nothing behind it",
			funds: "schema_version: 1\nfunds: [{number: 100, name: x, type: general, constraint_tier: discretionary}]\n",
			want:  `funds.yaml: fund 100: restriction_note: is required; constraint_tier "discretionary" is derived`,
		}, {
			// The failure the alias channel exists to prevent. "Measure D"
			// is printed on p76 and heads both funds; if both may claim it,
			// every later lookup is a coin flip, so the file does not load.
			name: "one label claimed by two funds",
			funds: `
schema_version: 1
funds:
  - {number: 550, name: "County Measure D", type: capital, constraint_tier: committed, restriction_note: y,
     aliases: [{term: "Measure D", pages: [76]}]}
  - {number: 828, name: "CIP County Measure D", type: capital, constraint_tier: committed, restriction_note: y,
     aliases: [{term: "Measure D", pages: [76]}]}
`,
			want: `funds.yaml: fund 828: aliases: alias "Measure D" is already the alias of fund 550 ("County Measure D"); a published label must name exactly one fund`,
		}, {
			name: "an alias that is another fund's name",
			funds: `
schema_version: 1
funds:
  - {number: 200, name: "Low Income Housing Fund", type: special-revenue, constraint_tier: committed, restriction_note: y}
  - {number: 812, name: "CIP Low Income Housing", type: capital, constraint_tier: committed, restriction_note: y,
     aliases: [{term: "Low Income Housing Fund", pages: [76]}]}
`,
			want: `funds.yaml: fund 812: aliases: alias "Low Income Housing Fund" is already the name of fund 200 ("Low Income Housing Fund"); a published label must name exactly one fund`,
		}, {
			name: "an alias that repeats its own fund's name",
			funds: `
schema_version: 1
funds:
  - {number: 640, name: "Water", type: enterprise, constraint_tier: committed, restriction_note: y,
     aliases: [{term: "Water", pages: [76]}]}
`,
			want: `funds.yaml: fund 640: aliases: alias "Water" is already the name of fund 640 ("Water")`,
		}, {
			name: "two funds with one name",
			funds: `
schema_version: 1
funds:
  - {number: 610, name: "Stormwater", type: enterprise, constraint_tier: committed, restriction_note: y}
  - {number: 611, name: "Stormwater", type: capital, constraint_tier: committed, restriction_note: y}
`,
			want: `funds.yaml: fund 611: name: name "Stormwater" is already the name of fund 610 ("Stormwater")`,
		}, {
			name: "an alias with no page",
			funds: `
schema_version: 1
funds:
  - {number: 200, name: "Low Income Housing Fund", type: special-revenue, constraint_tier: committed, restriction_note: y,
     aliases: [{term: "Low Income Hsng"}]}
`,
			want: `funds.yaml: fund 200: aliases[0].pages: is required; alias "Low Income Hsng" must say which page it was read from`,
		}, {
			name: "an alias with no term",
			funds: `
schema_version: 1
funds:
  - {number: 200, name: "Low Income Housing Fund", type: special-revenue, constraint_tier: committed, restriction_note: y,
     aliases: [{pages: [76]}]}
`,
			want: "funds.yaml: fund 200: aliases[0].term: is required",
		}, {
			name: "alias pages out of order",
			funds: `
schema_version: 1
funds:
  - {number: 470, name: "Doolan Canyon Preserve Endowment", type: permanent, constraint_tier: committed, restriction_note: y,
     aliases: [{term: "Doolan Canyon Preserve Endow", pages: [134, 76]}]}
`,
			want: `funds.yaml: fund 470: aliases[0].pages: 76 follows 134 for alias "Doolan Canyon Preserve Endow"; pages are listed once each, in ascending order`,
		}, {
			name: "alias page that is not a page",
			funds: `
schema_version: 1
funds:
  - {number: 470, name: "Doolan Canyon Preserve Endowment", type: permanent, constraint_tier: committed, restriction_note: y,
     aliases: [{term: "Doolan Canyon Preserve Endow", pages: [0]}]}
`,
			want: `funds.yaml: fund 470: aliases[0].pages: is 0 for alias "Doolan Canyon Preserve Endow"; pages are 1-based PDF page numbers`,
		}, {
			name:  "unknown key in an alias",
			funds: "schema_version: 1\nfunds: [{number: 100, name: x, type: general, constraint_tier: committed, restriction_note: y, aliases: [{term: z, page: 76}]}]\n",
			want:  "funds.yaml: yaml: unmarshal errors",
		}, {
			name:  "unknown key in funds",
			funds: "schema_version: 1\nfunds: [{number: 100, name: x, type: general, constraint_tier: committed, restriction_note: y, mayor: true}]\n",
			want:  "funds.yaml: yaml: unmarshal errors",
		}, {
			name:     "taxonomy schema version",
			taxonomy: "schema_version: 2\ncategories: [{slug: taxes/property, label: x, parent: taxes, kinds: [revenue]}]\n",
			want:     "taxonomy.yaml: schema_version: got 2, want 1",
		}, {
			name:     "no categories",
			taxonomy: "schema_version: 1\ncategories: []\n",
			want:     "taxonomy.yaml: categories: is empty",
		}, {
			name: "duplicate slug",
			taxonomy: `
schema_version: 1
categories:
  - {slug: taxes/sales, label: "Sales Taxes", parent: taxes, kinds: [revenue]}
  - {slug: taxes, label: "Taxes", kinds: [revenue]}
  - {slug: taxes/sales, label: "Sales Tax", parent: taxes, kinds: [revenue]}
`,
			want: `taxonomy.yaml: category "taxes/sales": slug: duplicate slug`,
		}, {
			name:     "category with no slug",
			taxonomy: `schema_version: 1` + "\n" + `categories: [{label: "Sales Taxes", kinds: [revenue]}]` + "\n",
			want:     `taxonomy.yaml: categories[0]: slug: is required (label "Sales Taxes")`,
		}, {
			name:     "category with no label",
			taxonomy: "schema_version: 1\ncategories: [{slug: intergovernmental, kinds: [revenue]}]\n",
			want:     `taxonomy.yaml: category "intergovernmental": label: is required`,
		}, {
			name:     "category with no kinds",
			taxonomy: "schema_version: 1\ncategories: [{slug: intergovernmental, label: x}]\n",
			want:     `taxonomy.yaml: category "intergovernmental": kinds: is required`,
		}, {
			name: "parent that does not resolve",
			taxonomy: `
schema_version: 1
categories:
  - {slug: taxes, label: "Taxes", kinds: [revenue]}
  - {slug: tax/property, label: "Property Taxes", parent: tax, kinds: [revenue]}
`,
			want: `taxonomy.yaml: category "tax/property": parent: unknown category "tax"`,
		}, {
			name: "parent that disagrees with the slug",
			taxonomy: `
schema_version: 1
categories:
  - {slug: transfers, label: "Transfers", kinds: [transfer_in]}
  - {slug: taxes/property, label: "Property Taxes", parent: transfers, kinds: [revenue]}
`,
			want: `taxonomy.yaml: category "taxes/property": parent: is "transfers", but the slug's head noun is "taxes"`,
		}, {
			// The defect this whole arm exists for. Before it landed this
			// file loaded clean and `fisc verify` was fully green.
			name: "category kind that mapping.Kind has never defined",
			taxonomy: `
schema_version: 1
categories:
  - {slug: transfers, label: "Transfers", kinds: [transfer]}
`,
			want: `taxonomy.yaml: category "transfers": kinds: got "transfer", want one of ` +
				`revenue, expenditure, transfer_in, transfer_out, fund_balance`,
		}, {
			name: "category kind listed twice",
			taxonomy: `
schema_version: 1
categories:
  - {slug: transfers, label: "Transfers", kinds: [transfer_in, transfer_in]}
`,
			want: `taxonomy.yaml: category "transfers": kinds: "transfer_in" is listed twice`,
		}, {
			name: "category page that is not 1-based",
			taxonomy: `
schema_version: 1
categories:
  - {slug: taxes, label: "Taxes", kinds: [revenue], pages: [0]}
`,
			want: `taxonomy.yaml: category "taxes": pages: is 0; pages are 1-based PDF page numbers`,
		}, {
			name: "category pages out of order",
			taxonomy: `
schema_version: 1
categories:
  - {slug: taxes, label: "Taxes", kinds: [revenue], pages: [66, 66]}
`,
			want: `taxonomy.yaml: category "taxes": pages: 66 follows 66; pages are listed once each, in ascending order`,
		}, {
			name: "category alias with no term",
			taxonomy: `
schema_version: 1
categories:
  - {slug: taxes, label: "Taxes", kinds: [revenue], aliases: [{pages: [66]}]}
`,
			want: `taxonomy.yaml: category "taxes": aliases[0].term: is required`,
		}, {
			name: "category alias with no page to check it against",
			taxonomy: `
schema_version: 1
categories:
  - {slug: taxes, label: "Taxes", kinds: [revenue], aliases: [{term: "TAXES:"}]}
`,
			want: `taxonomy.yaml: category "taxes": aliases[0].pages: is required; alias "TAXES:" must say which page it was read from`,
		}, {
			name: "category alias derived with no rationale",
			taxonomy: `
schema_version: 1
categories:
  - {slug: taxes, label: "Taxes", kinds: [revenue], aliases: [{term: "TAXES:", pages: [66], derived: true}]}
`,
			want: `taxonomy.yaml: category "taxes": aliases[0].rationale: is required when derived is true`,
		}, {
			name: "contra row with no term",
			taxonomy: `
schema_version: 1
categories:
  - {slug: taxes, label: "Taxes", kinds: [revenue], pages: [127], contra_rows: [{page: 127}]}
`,
			want: `taxonomy.yaml: category "taxes": contra_rows[0].term: is required`,
		}, {
			name: "contra row with no page",
			taxonomy: `
schema_version: 1
categories:
  - {slug: taxes, label: "Taxes", kinds: [revenue], pages: [127], contra_rows: [{term: "Refunds"}]}
`,
			want: `taxonomy.yaml: category "taxes": contra_rows[0].page: is 0 for "Refunds"; pages are 1-based PDF page numbers`,
		}, {
			name: "category alias repeating the label it is an alternative to",
			taxonomy: `
schema_version: 1
categories:
  - {slug: taxes, label: "Taxes", kinds: [revenue], aliases: [{term: "Taxes", pages: [66]}]}
`,
			want: `taxonomy.yaml: category "taxes": aliases[0].term: "Taxes" is the category's own label`,
		}, {
			// Five committed categories have a document_term that differs from
			// their label, so a rule comparing only the label would appear to
			// work while letting exactly those five list themselves.
			name: "category alias repeating its own document_term",
			taxonomy: `
schema_version: 1
categories:
  - {slug: fund-balance, label: "Fund Balance", kinds: [fund_balance]}
  - {slug: fund-balance/beginning, label: "Beginning Fund Balance", parent: fund-balance, document_term: "BEGINNING WORKING CAPITAL", kinds: [fund_balance], aliases: [{term: "BEGINNING WORKING CAPITAL", pages: [66]}]}
`,
			want: `taxonomy.yaml: category "fund-balance/beginning": aliases[0].term: "BEGINNING WORKING CAPITAL" is the category's own document_term`,
		}, {
			// Per category, NOT file-wide: fund-balance/beginning and
			// fund-balance/ending both publish "Fund Balance / Working
			// Capital" and both are right.
			name: "category alias listed twice in one category",
			taxonomy: `
schema_version: 1
categories:
  - {slug: taxes, label: "Taxes", kinds: [revenue], aliases: [{term: "TAXES:", pages: [66]}, {term: "TAXES:", pages: [127]}]}
`,
			want: `taxonomy.yaml: category "taxes": aliases[1].term: "TAXES:" is listed twice`,
		}, {
			name: "contra row repeated on the same page",
			taxonomy: `
schema_version: 1
categories:
  - {slug: taxes, label: "Taxes", kinds: [revenue], pages: [127], contra_rows: [{term: "ERAF", page: 127}, {term: "ERAF", page: 127}]}
`,
			want: `taxonomy.yaml: category "taxes": contra_rows[1]: repeats "ERAF" on page 127`,
		}, {
			// The arm that checks a contra row is checked AGAINST `pages`, so a
			// category declaring contra rows and no pages used to skip it
			// entirely -- fail-open, and green.
			name: "contra row on a category with no pages to check it against",
			taxonomy: `
schema_version: 1
categories:
  - {slug: taxes, label: "Taxes", kinds: [revenue], contra_rows: [{term: "Refunds", page: 127}]}
`,
			want: `taxonomy.yaml: category "taxes": pages: is required when contra_rows is set`,
		}, {
			// A contra row is a detail line inside this category's own printed
			// subtotal, so a page the category does not claim means one of the
			// two records is wrong -- and nothing else in the tree reads
			// contra_rows at all, so nothing else would notice.
			name: "contra row on a page its category does not claim",
			taxonomy: `
schema_version: 1
categories:
  - {slug: taxes, label: "Taxes", kinds: [revenue], pages: [66, 127], contra_rows: [{term: "Refunds", page: 139}]}
`,
			want: `taxonomy.yaml: category "taxes": contra_rows[0].page: is 139 for "Refunds", which is not one of the category's pages [66 127]`,
		}, {
			name: "nested slug with no parent",
			taxonomy: `
schema_version: 1
categories:
  - {slug: taxes, label: "Taxes", kinds: [revenue]}
  - {slug: taxes/property, label: "Property Taxes", kinds: [revenue]}
`,
			want: `taxonomy.yaml: category "taxes/property": parent: is required; the slug's head noun "taxes" names one`,
		}, {
			name: "parent on a slug with no parent segment",
			taxonomy: `
schema_version: 1
categories:
  - {slug: taxes, label: "Taxes", kinds: [revenue]}
  - {slug: intergovernmental, label: "Intergovernmental", parent: taxes, kinds: [revenue]}
`,
			want: `taxonomy.yaml: category "intergovernmental": parent: is "taxes", but the slug has no parent segment`,
		}, {
			// Refused for its grammar, never its depth: `taxes` is the
			// grandparent.
			name: "parent that is the grandparent",
			taxonomy: `
schema_version: 1
categories:
  - {slug: taxes, label: "Taxes", kinds: [revenue]}
  - {slug: taxes/property/secured, label: "Current Year - Secured", parent: taxes, kinds: [revenue]}
`,
			want: `taxonomy.yaml: category "taxes/property/secured": parent: is "taxes", but the slug's head noun is "taxes/property"`,
		}, {
			// The parent must be the slug's own prefix, whatever its depth.
			name: "parent that is not the slug's own prefix",
			taxonomy: `
schema_version: 1
categories:
  - {slug: taxes, label: "Taxes", kinds: [revenue]}
  - {slug: taxes/property, label: "Property Taxes", parent: taxes, kinds: [revenue]}
  - {slug: property/secured, label: "Current Year - Secured", parent: taxes/property, kinds: [revenue]}
`,
			want: `taxonomy.yaml: category "property/secured": parent: is "taxes/property", but the slug's head noun is "property"`,
		}, {
			name:     "derived without a rationale",
			taxonomy: "schema_version: 1\ncategories: [{slug: taxes, label: x, kinds: [revenue], derived: true, source_note: \"p. 66\"}]\n",
			want:     `taxonomy.yaml: category "taxes": rationale: is required when derived is true`,
		}, {
			name:     "derived without a source note",
			taxonomy: "schema_version: 1\ncategories: [{slug: taxes, label: x, kinds: [revenue], derived: true, rationale: \"a rollup\"}]\n",
			want:     `taxonomy.yaml: category "taxes": source_note: is required when derived is true`,
		}, {
			name:     "rationale without derived",
			taxonomy: "schema_version: 1\ncategories: [{slug: taxes, label: x, kinds: [revenue], rationale: \"a rollup\"}]\n",
			want:     `taxonomy.yaml: category "taxes": derived: is not set, but a rationale is given`,
		}, {
			name:     "source note without derived",
			taxonomy: "schema_version: 1\ncategories: [{slug: taxes, label: x, kinds: [revenue], source_note: \"p. 66\"}]\n",
			want:     `taxonomy.yaml: category "taxes": derived: is not set, but a source_note is given`,
		}, {
			// The key this file exists to get right: misspelled, it would
			// publish a rollup node as a spendable category.
			name:     "unknown key in taxonomy",
			taxonomy: "schema_version: 1\ncategories: [{slug: taxes, label: x, kinds: [revenue], assignabel: false}]\n",
			want:     "taxonomy.yaml: yaml: unmarshal errors",
		}, {
			name:  "empty file",
			blank: TaxonomyFile,
			want:  "taxonomy.yaml: file is empty",
		}, {
			name:     "two documents",
			taxonomy: "schema_version: 1\ncategories: [{slug: taxes, label: x, kinds: [revenue]}]\n---\nschema_version: 1\n",
			want:     "taxonomy.yaml: file contains more than one YAML document",
		}, {
			name:        "departments schema version",
			departments: "schema_version: 2\ndepartments: []\ndivisions: []\n",
			want:        "departments.yaml: schema_version: got 2, want 1",
		}, {
			name:        "no departments",
			departments: "schema_version: 1\ndepartments: []\ndivisions: [{slug: patrol, label: x, department: y, pages: [168]}]\n",
			want:        "departments.yaml: departments: is empty",
		}, {
			name:        "no divisions",
			departments: "schema_version: 1\ndepartments: [{slug: police-department, label: x, document_term: X, pages: [168]}]\ndivisions: []\n",
			want:        "departments.yaml: divisions: is empty",
		}, {
			name: "duplicate department slug",
			departments: `
schema_version: 1
departments:
  - {slug: police-department, label: "Police Department", document_term: "POLICE DEPARTMENT", pages: [168]}
  - {slug: police-department, label: "Police Dept", document_term: "POLICE DEPARTMENT", pages: [169]}
divisions:
  - {slug: patrol, label: "Patrol", department: police-department, pages: [168]}
`,
			want: `departments.yaml: department "police-department": slug: duplicate slug`,
		}, {
			name: "duplicate division slug",
			departments: `
schema_version: 1
departments:
  - {slug: police-department, label: "Police Department", document_term: "POLICE DEPARTMENT", pages: [168]}
divisions:
  - {slug: patrol, label: "Patrol", department: police-department, pages: [168]}
  - {slug: patrol, label: "Patrol Division", department: police-department, pages: [169]}
`,
			want: `departments.yaml: division "patrol": slug: duplicate slug`,
		}, {
			// The same refusal one tier up: pp.85-125 rows put a department
			// slug in row_path.
			name: "department slug that is also a category",
			departments: `
schema_version: 1
departments:
  - {slug: debt-services, label: "Debt Services", document_term: "DEBT SERVICES", pages: [168]}
divisions:
  - {slug: patrol, label: "Patrol", department: debt-services, pages: [168]}
`,
			want: `departments.yaml: department "debt-services": slug: is also a taxonomy.yaml category slug`,
		}, {
			// THE ONE COLLISION THAT MATTERS. A department and a division may
			// share a name -- the city prints five such pairs -- but a division
			// and a category may not, because row_path joins exactly those two.
			name: "division slug that is also a category",
			departments: `
schema_version: 1
departments:
  - {slug: general-services, label: "General Services", document_term: "GENERAL SERVICES", pages: [168]}
divisions:
  - {slug: debt-services, label: "Debt Services", department: general-services, pages: [168]}
`,
			want: `departments.yaml: division "debt-services": slug: is also a taxonomy.yaml category slug`,
		}, {
			name: "division under a department that does not exist",
			departments: `
schema_version: 1
departments:
  - {slug: police-department, label: "Police Department", document_term: "POLICE DEPARTMENT", pages: [168]}
divisions:
  - {slug: patrol, label: "Patrol", department: police, pages: [168]}
`,
			want: `departments.yaml: division "patrol": department: unknown department "police"`,
		}, {
			name: "division with no department",
			departments: `
schema_version: 1
departments:
  - {slug: police-department, label: "Police Department", document_term: "POLICE DEPARTMENT", pages: [168]}
divisions:
  - {slug: patrol, label: "Patrol", pages: [168]}
`,
			want: `departments.yaml: division "patrol": department: is required; every division is printed under one heading`,
		}, {
			// A heading over nothing is a division that was dropped, and a
			// dropped division is invisible to every object-category sum.
			name: "department no division claims",
			departments: `
schema_version: 1
departments:
  - {slug: police-department, label: "Police Department", document_term: "POLICE DEPARTMENT", pages: [168]}
  - {slug: fire-department, label: "Fire Department", document_term: "FIRE DEPARTMENT", pages: [170]}
divisions:
  - {slug: patrol, label: "Patrol", department: police-department, pages: [168]}
`,
			want: `departments.yaml: department "fire-department": no division names this department`,
		}, {
			name:        "department with no document term",
			departments: "schema_version: 1\ndepartments: [{slug: patrol-dept, label: x, pages: [168]}]\ndivisions: [{slug: patrol, label: y, department: patrol-dept, pages: [168]}]\n",
			want:        `departments.yaml: department "patrol-dept": document_term: is required`,
		}, {
			name:        "division with no label",
			departments: "schema_version: 1\ndepartments: [{slug: police-department, label: x, document_term: X, pages: [168]}]\ndivisions: [{slug: patrol, department: police-department, pages: [168]}]\n",
			want:        `departments.yaml: division "patrol": label: is required`,
		}, {
			name:        "division with no pages",
			departments: "schema_version: 1\ndepartments: [{slug: police-department, label: x, document_term: X, pages: [168]}]\ndivisions: [{slug: patrol, label: y, department: police-department}]\n",
			want:        `departments.yaml: division "patrol": pages: is required; say which page the entry is printed on`,
		}, {
			name:        "division pages out of order",
			departments: "schema_version: 1\ndepartments: [{slug: police-department, label: x, document_term: X, pages: [168]}]\ndivisions: [{slug: special-operations, label: y, department: police-department, pages: [169, 168]}]\n",
			want:        `departments.yaml: division "special-operations": pages: 168 follows 169; pages are listed once each, in ascending order`,
		}, {
			// A slug with a "/" would emit a two-slash row_path that no reader
			// could split back into its two axes.
			name:        "division slug with a parent segment",
			departments: "schema_version: 1\ndepartments: [{slug: police-department, label: x, document_term: X, pages: [168]}]\ndivisions: [{slug: police/patrol, label: y, department: police-department, pages: [168]}]\n",
			want:        `departments.yaml: division "police/patrol": slug: is not a slug: lower case, digits and single hyphens, one segment`,
		}, {
			name:        "division derived without a rationale",
			departments: "schema_version: 1\ndepartments: [{slug: police-department, label: x, document_term: X, pages: [168]}]\ndivisions: [{slug: patrol, label: y, department: police-department, pages: [168], derived: true, source_note: p}]\n",
			want:        `departments.yaml: division "patrol": rationale: is required when derived is true`,
		}, {
			name:        "division rationale without derived",
			departments: "schema_version: 1\ndepartments: [{slug: police-department, label: x, document_term: X, pages: [168]}]\ndivisions: [{slug: patrol, label: y, department: police-department, pages: [168], rationale: r}]\n",
			want:        `departments.yaml: division "patrol": derived: is not set, but a rationale is given`,
		}, {
			name:        "unknown key in departments",
			departments: "schema_version: 1\ndepartments: [{slug: police-department, label: x, document_term: X, pages: [168], parent: y}]\ndivisions: [{slug: patrol, label: y, department: police-department, pages: [168]}]\n",
			want:        "departments.yaml: yaml: unmarshal errors",
		}, {
			name:  "empty departments file",
			blank: DepartmentsFile,
			want:  "departments.yaml: file is empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fsys := registryFS(t, tt.funds, tt.taxonomy, tt.departments)
			if tt.blank != "" {
				fsys[tt.blank] = &fstest.MapFile{}
			}
			_, err := Load(fsys)
			if err == nil {
				t.Fatalf("Load = nil error, want one containing %q", tt.want)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Load error = %q, want it to contain %q", err, tt.want)
			}
			var re *Error
			if !errors.As(err, &re) {
				t.Fatalf("Load error is %T, want *registry.Error", err)
			}
			if re.File == "" {
				t.Errorf("Load error %q does not name the file it came from", err)
			}
		})
	}
}

// TestTheTaxonomyNestsToAnyDepth: no arm counts segments.
//
// Mutation: counting "/" in a slug, refusing a parent that has a parent, or
// cutting the slug at its first slash fails this at depth 2.
func TestTheTaxonomyNestsToAnyDepth(t *testing.T) {
	r := load(t, "", `
schema_version: 1
categories:
  - {slug: taxes, label: "Taxes", kinds: [revenue], assignable: false}
  - {slug: taxes/property, label: "Property Taxes", parent: taxes, kinds: [revenue]}
  - {slug: taxes/property/secured, label: "Current Year - Secured", parent: taxes/property, kinds: [revenue]}
  - {slug: taxes/property/secured/roll, label: "Secured Roll", parent: taxes/property/secured, kinds: [revenue]}
`, "")

	want := map[string]string{
		"taxes":                       "",
		"taxes/property":              "taxes",
		"taxes/property/secured":      "taxes/property",
		"taxes/property/secured/roll": "taxes/property/secured",
	}
	got := map[string]string{}
	for _, c := range r.Categories() {
		got[c.Slug] = c.Parent
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("parent of each slug (-want +got):\n%s", diff)
	}
	// Assignability is the flag and nothing else: a parent that is not a
	// rollup stays writable, and a leaf three levels down is writable too.
	for _, slug := range []string{"taxes/property", "taxes/property/secured", "taxes/property/secured/roll"} {
		if !r.Assignable(slug) {
			t.Errorf("Assignable(%q) = false; nothing infers assignable from depth", slug)
		}
	}
}

// A file from a newer fisc is not a malformed file, and telling the reader to
// upgrade rather than to go hunting for a typo is the difference.
func TestLoadHintsAtANewerSchema(t *testing.T) {
	_, err := Load(registryFS(t, "schema_version: 2\nfunds: []\n", "", ""))
	var hint *cmdutil.ErrHint
	if !errors.As(err, &hint) {
		t.Fatalf("Load error = %v (%T), want a hinted error", err, err)
	}
	if !strings.Contains(hint.Hint, "upgrade") {
		t.Errorf("hint = %q, want it to mention upgrading", hint.Hint)
	}

	// An older or missing version is a broken file, not an old binary.
	_, err = Load(registryFS(t, "funds: []\n", "", ""))
	if errors.As(err, &hint) {
		t.Errorf("Load error = %q carries hint %q, want none", err, hint.Hint)
	}
}

// A missing file is a wrapped fs error rather than a *registry.Error: nothing
// in the file is wrong, so there is no entry to name.
func TestLoadWithMissingFiles(t *testing.T) {
	tests := []struct {
		name string
		fsys fstest.MapFS
		want string
	}{
		{"nothing at all", fstest.MapFS{}, "read funds.yaml"},
		{
			"only funds",
			fstest.MapFS{FundsFile: &fstest.MapFile{Data: []byte(validFunds)}},
			"read taxonomy.yaml",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Load(tt.fsys)
			if err == nil {
				t.Fatalf("Load = nil error, want one containing %q", tt.want)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Load error = %q, want it to contain %q", err, tt.want)
			}
			if !errors.Is(err, fs.ErrNotExist) {
				t.Errorf("Load error = %v, want it to wrap fs.ErrNotExist", err)
			}
		})
	}
}

// The three rollup nodes are the reason `assignable` exists. Absence must
// read as true, or every ordinary category becomes unwritable.
func TestAssignableDefaultsToTrue(t *testing.T) {
	r := load(t, "", `
schema_version: 1
categories:
  - {slug: taxes, label: "Taxes", kinds: [revenue], assignable: false, derived: true, rationale: r, source_note: s}
  - {slug: taxes/sales, label: "Sales Taxes", parent: taxes, kinds: [revenue]}
  - {slug: intergovernmental, label: "Intergovernmental", kinds: [revenue], assignable: true}
`, "")
	for _, tt := range []struct {
		slug string
		want bool
	}{
		{"taxes", false},
		{"taxes/sales", true},
		{"intergovernmental", true},
	} {
		if got := r.Assignable(tt.slug); got != tt.want {
			t.Errorf("Assignable(%q) = %v, want %v", tt.slug, got, tt.want)
		}
		c, ok := r.Category(tt.slug)
		if !ok {
			t.Fatalf("Category(%q) not found", tt.slug)
		}
		if c.Assignable != tt.want {
			t.Errorf("Category(%q).Assignable = %v, want %v", tt.slug, c.Assignable, tt.want)
		}
	}
}

// TestOneAliasTermMaySpanTwoCategories pins the LIMIT of the uniqueness rule
// TestLoadRejects covers, and it is not a hypothetical.
//
// A fund alias is unique file-wide (claimLabel), because a label two funds
// answer to is a label no schedule can be mapped by. A category alias is not,
// and must not be: data/taxonomy.yaml has fund-balance/beginning and
// fund-balance/ending BOTH publishing "Fund Balance / Working Capital",
// because pp.66-67 print those same words on two rows and the surrounding
// structure says which is which. Tightening the rule to the file would reject
// the committed taxonomy.
func TestOneAliasTermMaySpanTwoCategories(t *testing.T) {
	r := load(t, "", `
schema_version: 1
categories:
  - {slug: taxes, label: "Taxes", kinds: [revenue], aliases: [{term: "Shared", pages: [66]}]}
  - {slug: rents, label: "Rents", kinds: [revenue], aliases: [{term: "Shared", pages: [66]}]}
`, "")
	for _, slug := range []string{"taxes", "rents"} {
		c, ok := r.Category(slug)
		if !ok {
			t.Fatalf("Category(%q) not found", slug)
		}
		if len(c.Aliases) != 1 || c.Aliases[0].Term != "Shared" {
			t.Errorf("category %q aliases = %v, want the one term %q", slug, c.Aliases, "Shared")
		}
	}
}

// TestTheRealTaxonomyShareOfFundBalanceIsTheReasonWhy names the committed
// entries the test above is generalising from, so a reader hitting the
// per-category rule can go and look rather than take it on trust.
func TestTheRealTaxonomyShareOfFundBalanceIsTheReasonWhy(t *testing.T) {
	r, err := Load(os.DirFS(realData))
	if err != nil {
		t.Fatalf("Load(%s): %v", realData, err)
	}
	const shared = "Fund Balance / Working Capital"
	var carriers []string
	for _, c := range r.Categories() {
		for _, a := range c.Aliases {
			if a.Term == shared {
				carriers = append(carriers, c.Slug)
			}
		}
	}
	want := []string{"fund-balance/beginning", "fund-balance/ending"}
	if diff := cmp.Diff(want, carriers); diff != "" {
		t.Errorf("categories publishing alias %q mismatch (-want +got):\n%s\n"+
			"if this is now one category, the alias uniqueness rule could be "+
			"tightened to the file as funds already are", shared, diff)
	}
}
