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
// THE ROLES ARE WHAT KEEPS A CLIENT FROM PARSING AN ID, which is what their own
// declaration says they are for, and site/app.js's isFundGroup now reads one.
// That only works while every speller agrees: this package writes the values,
// pkg/cmd/export re-spells five of them in its step declarations, and
// internal/export selects fund groups by a sixth copy. None of the three can
// see the others, and the enum is where they meet.
//
// BOTH DIRECTIONS. A role declared here and missing from the enum ships a
// document `fisc export` refuses; a role in the enum this package no longer
// composes is a value the contract still promises a reader.
//
// A ROLE COMPOSED AT A CALL SITE RATHER THAN THROUGH THE BLOCK NEEDS NO ARM
// HERE, and that is worth saying so it is not added: encodeColumn validates
// every published column against this enum before writing it, so a literal
// that reached a node would fail the export over the real corpus rather than
// slip past a test that compares two lists neither of which mentions it.
func TestEveryDeclaredRoleIsInTheSchemasEnum(t *testing.T) {
	// Every value the const block above declares, listed rather than reflected
	// because an unexported const block cannot be enumerated at run time. A
	// role added there and not here is caught by the count arm below.
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
