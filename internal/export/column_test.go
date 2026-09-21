package export

import (
	"encoding/json"
	"io/fs"
	"slices"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/jcrussell/livermore-budget/schema"
)

// TestFundGroupsAreOrderedAndOpenEnded holds [fundGroupsOf] to the two claims
// site/app.js now depends on instead of holding lists of its own.
//
// THE OPEN-ENDED HALF IS THE ONE WITH A DEFECT BEHIND IT. data/funds.yaml
// declares seven fund types and fy2024-actual publishes all seven, so a rule
// that only ordered the ones [fundGroupDisplayOrder] names would leave the
// seventh's place undefined -- which is what an indexOf answering -1 does, and
// -1 sorts it to the TOP of the fund column ahead of every group the palette
// knows. fisc-zojk.
func TestFundGroupsAreOrderedAndOpenEnded(t *testing.T) {
	nodes := []ColumnNode{
		{ID: "revenue/taxes", Role: "revenue_source"},
		{ID: "fund-group/debt-service", Role: roleFundGroup},
		{ID: "fund-group/permanent", Role: roleFundGroup},
		{ID: "fund-group/general", Role: roleFundGroup},
		{ID: "fund-group/aardvark", Role: roleFundGroup},
		{ID: "fund/100", Role: "fund"},
	}
	got, err := fundGroupsOf(nodes)
	if err != nil {
		t.Fatalf("fundGroupsOf: %v", err)
	}
	want := []ColumnFundGroup{
		{ID: "fund-group/general", Slug: "general"},
		{ID: "fund-group/debt-service", Slug: "debt-service"},
		// Neither is in the declared sequence, so both land after every group
		// that is, in id order rather than in the order the table happened to
		// carry them.
		{ID: "fund-group/aardvark", Slug: "aardvark"},
		{ID: "fund-group/permanent", Slug: "permanent"},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("fundGroupsOf (-want +got):\n%s\n"+
			"site/app.js's fundGroupPlace reads this order and nothing else does; a "+
			"group the sequence does not name belongs last, not nowhere.", diff)
	}
}

// TestAFundGroupWithNoFundTypeInItsIDIsRefused is the fail-closed arm.
//
// The client composes --fund-<slug> and falls back to --muted when no such
// custom property exists, so a slug that is really a whole id draws muted with
// no other symptom -- a chart that is wrong rather than one that fails.
func TestAFundGroupWithNoFundTypeInItsIDIsRefused(t *testing.T) {
	for _, id := range []string{"fundgroup", "fund-group/"} {
		if _, err := fundGroupsOf([]ColumnNode{{ID: id, Role: roleFundGroup}}); err == nil {
			t.Errorf("fundGroupsOf accepted %q as a fund group; it names no fund type", id)
		}
	}
}

// TestRoleFundGroupIsOneOfTheSchemasRoles holds this package's copy of the
// value to the one list every speller of a role is checked against.
//
// This package reads projections as bytes and imports neither internal/project,
// which composes the value, nor pkg/cmd/export, which re-spells five roles in
// its step declarations. The enum in schema/column.schema.json is what makes
// those three copies one claim.
func TestRoleFundGroupIsOneOfTheSchemasRoles(t *testing.T) {
	roles := schemaRoles(t)
	if !slices.Contains(roles, roleFundGroup) {
		t.Errorf("%s's role enum does not list %q, which this package selects fund groups by: %v",
			schema.Column, roleFundGroup, roles)
	}
}

// schemaRoles is the `role` enum column.schema.json declares for a node.
func schemaRoles(t *testing.T) []string {
	t.Helper()
	raw, err := fs.ReadFile(schema.FS(), schema.Column)
	if err != nil {
		t.Fatalf("read %s: %v", schema.Column, err)
	}
	var doc struct {
		Properties struct {
			Nodes struct {
				Items struct {
					Properties struct {
						Role struct {
							Enum []string `json:"enum"`
						} `json:"role"`
					} `json:"properties"`
				} `json:"items"`
			} `json:"nodes"`
		} `json:"properties"`
	}
	if err = json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse %s: %v", schema.Column, err)
	}
	got := doc.Properties.Nodes.Items.Properties.Role.Enum
	if len(got) == 0 {
		t.Fatalf("%s states no role enum, so this test compares nothing", schema.Column)
	}
	return got
}
