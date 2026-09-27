package project

import (
	"encoding/json"
	"io/fs"
	"slices"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/jcrussell/livermore-budget/schema"
)

// TestEveryDeclaredRoleIsInTheSchemasEnum holds the role vocabulary this
// package composes to the one schema/column.schema.json publishes.
//
// Both directions: a role missing from the enum ships a document `fisc export`
// refuses, and an enum value nothing composes is a promise about nothing.
func TestEveryDeclaredRoleIsInTheSchemasEnum(t *testing.T) {
	// Listed by hand: an unexported const block cannot be enumerated.
	declared := []string{
		roleRevenueSource, roleRevenueLine, roleFundGroup, roleFund,
		roleGeneralFund, roleDepartment, roleWholeDepartment, roleObjectCategory,
		roleTransferIn, roleTransferOut, RoleTransferSource, RoleTransferSink,
		roleReserveIncrease, roleFundBalanceDraw, roleFundBalanceContribution,
	}
	slices.Sort(declared)

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
	stated := slices.Clone(doc.Properties.Nodes.Items.Properties.Role.Enum)
	slices.Sort(stated)
	if len(stated) == 0 {
		t.Fatalf("%s states no role enum, so this test compares nothing", schema.Column)
	}
	if diff := cmp.Diff(declared, stated); diff != "" {
		t.Errorf("%s's role enum and this package's roles differ (-declared +stated):\n%s\n"+
			"A role this package writes and the enum omits is a document fisc export refuses.",
			schema.Column, diff)
	}
}
