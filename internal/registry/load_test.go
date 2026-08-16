package registry

import (
	"errors"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/jcrussell/livermore-budget/pkg/cmdutil"
)

// TestLoadRejects covers every way a registry file can be wrong. Each case
// asserts the message names the offending entry: the files are 112 and 25
// entries long, and "duplicate slug" without a slug is a search.
func TestLoadRejects(t *testing.T) {
	tests := []struct {
		name     string
		funds    string // empty means the valid fixture
		taxonomy string
		blank    string // a file to truncate, for the cases an empty body cannot express
		want     string
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
  - {slug: transfers, label: "Transfers", kinds: [transfer]}
  - {slug: taxes/property, label: "Property Taxes", parent: transfers, kinds: [revenue]}
`,
			want: `taxonomy.yaml: category "taxes/property": parent: is "transfers", but the slug's head noun is "taxes"`,
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
			name: "two levels deep",
			taxonomy: `
schema_version: 1
categories:
  - {slug: taxes, label: "Taxes", kinds: [revenue]}
  - {slug: taxes/property/secured, label: "Current Year - Secured", parent: taxes, kinds: [revenue]}
`,
			want: `taxonomy.yaml: category "taxes/property/secured": slug: has 2 parent segments; the taxonomy is one level deep`,
		}, {
			name: "grandchild",
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
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fsys := registryFS(t, tt.funds, tt.taxonomy)
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

// A file from a newer fisc is not a malformed file, and telling the reader to
// upgrade rather than to go hunting for a typo is the difference.
func TestLoadHintsAtANewerSchema(t *testing.T) {
	_, err := Load(registryFS(t, "schema_version: 2\nfunds: []\n", ""))
	var hint *cmdutil.ErrHint
	if !errors.As(err, &hint) {
		t.Fatalf("Load error = %v (%T), want a hinted error", err, err)
	}
	if !strings.Contains(hint.Hint, "upgrade") {
		t.Errorf("hint = %q, want it to mention upgrading", hint.Hint)
	}

	// An older or missing version is a broken file, not an old binary.
	_, err = Load(registryFS(t, "funds: []\n", ""))
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
`)
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
