package export

import (
	"encoding/json"
	"io/fs"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
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

// TestTheSchemaStatesWhatAColumnCarries holds schema/column.schema.json to the
// struct encodeColumn marshals: every JSON name the artifact can carry is a
// property there, and nothing is a property there the artifact cannot carry.
//
// THE VALIDATOR ALONE DOES NOT COVER THIS DIRECTION. encodeColumn refuses bytes
// the schema rejects, so a key ADDED to the struct is caught by
// additionalProperties. A key the schema stops REQUIRING is not: every column
// this corpus produces still carries it, so the export stays green and the only
// thing that noticed was a hand-written list in the client's tests.
func TestTheSchemaStatesWhatAColumnCarries(t *testing.T) {
	stated, err := schema.Names(schema.Column)
	if err != nil {
		t.Fatalf("read %s: %v", schema.Column, err)
	}
	emitted := schema.StructNames(reflect.TypeOf(ColumnDoc{}), "")

	// WHERE THE SCHEMA KNOWS MORE THAN THIS PACKAGE, NAMED RATHER THAN
	// TOLERATED. `counts` is a [json.RawMessage]: internal/project composes it
	// and this package copies the bytes, so the struct cannot state the keys
	// inside and [schema.StructNames] rightly stops. The SCHEMA can state them,
	// because it is applied to the emitted bytes rather than to the struct, and
	// site/app.js reads them -- so holding it opaque here to make two lists
	// match would delete a real check to pass a test about a different thing.
	//
	// The prefix itself must be in both, which is what keeps this an exemption
	// for one blob rather than a hole the next pass-through field falls into.
	const passThrough = "schedules.counts"
	if !slices.Contains(stated, passThrough) || !slices.Contains(emitted, passThrough) {
		t.Fatalf("%q is exempted below and is not a field of both the schema and the struct",
			passThrough)
	}
	stated = slices.DeleteFunc(stated, func(n string) bool {
		return strings.HasPrefix(n, passThrough+".")
	})

	slices.Sort(stated)
	slices.Sort(emitted)
	if len(stated) == 0 {
		t.Fatalf("%s states no property, so this test compares nothing", schema.Column)
	}
	if diff := cmp.Diff(emitted, stated); diff != "" {
		t.Errorf("%s and the emitted column name different fields (-emitted +stated):\n%s\n"+
			"Every name a column carries is a property of the schema, and only those.",
			schema.Column, diff)
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

// TestAReductionsSentenceSurvivesTheFoldIntoAColumn is the seam test for the one
// field whose loss is silent.
//
// THE PACKAGER DECODES INTO AN ANONYMOUS STRUCT, so a key it does not name is
// dropped without an error, without a warning and without failing any schema:
// the column it writes is valid, every link is present, every figure is right,
// and the only thing missing is the sentence that tells a reader the ribbon
// drawn at $16,985,339 is a subtraction. Every other guard in this package
// compares what the struct DOES carry against something, so none of them can
// see a field that never arrived.
//
// Drop Contra from decoded.Links and this is what goes red.
func TestAReductionsSentenceSurvivesTheFoldIntoAColumn(t *testing.T) {
	const note = "printed as a reduction of Property Taxes"
	doc := []byte(`{
	  "schema_version": "1.0.0",
	  "projection": "fund-flows",
	  "metadata": {
	    "fiscal_year": 2026, "fiscal_year_label": "FY2025-26", "basis": "adopted",
	    "generated_by": "t", "currency": "USD", "units": "cents",
	    "scopes": ["revenue-by-fund"], "sources": [], "counts": {}, "caveats": []
	  },
	  "nodes": [
	    {"id": "a", "label": "ERAF", "tier": 1},
	    {"id": "b", "label": "Property Taxes", "tier": 0}
	  ],
	  "links": [
	    {"source": "a", "target": "b", "value_cents": -250, "kind": "external",
	     "transfer_id": "", "fact_ids": ["fisc-f-0000000000aa"], "locators": [],
	     "derived": false, "partition": false, "contra": ` + strconv.Quote(note) + `},
	    {"source": "b", "target": "a", "value_cents": 250, "kind": "external",
	     "transfer_id": "", "fact_ids": ["fisc-f-0000000000bb"], "locators": [],
	     "derived": false, "partition": false, "contra": ""}
	  ]
	}`)

	cols, _, err := ColumnsOf(map[string][]byte{"fund-flows": doc}, "t")
	if err != nil {
		t.Fatalf("ColumnsOf: %v", err)
	}
	col, ok := cols["fy2026-adopted.json"]
	if !ok {
		t.Fatalf("no fy2026-adopted column in %v", keysOf(cols))
	}
	sched, ok := col.Schedules["fund-flows"]
	if !ok {
		t.Fatalf("no fund-flows schedule in the column")
	}
	if len(sched.Links) != 2 {
		t.Fatalf("%d link(s) folded, want 2", len(sched.Links))
	}
	var negative, positive ColumnLink
	for _, l := range sched.Links {
		if l.ValueCents < 0 {
			negative = l
		} else {
			positive = l
		}
	}
	if negative.Contra != note {
		t.Errorf("the negative link's Contra is %q, want %q. A reduction that reaches the "+
			"column without its sentence is drawn forward at its magnitude with nothing on "+
			"the page saying it is a subtraction", negative.Contra, note)
	}
	// THE CONVERSE, because omitempty makes the empty case look like the absent
	// one and a copy that filled it in would be inventing a reduction.
	if positive.Contra != "" {
		t.Errorf("the positive link's Contra is %q, want empty", positive.Contra)
	}
}

func keysOf(m map[string]ColumnDoc) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// TestStepStemsResolvesAStepWithNoScheduleThroughItsAfter is a step that names
// no schedule declared after a sibling that does: it draws the document of
// the chart its After names, as validateSteps and site/app.js's stepDocument
// resolve it, and not the document of the step declared just before it.
func TestStepStemsResolvesAStepWithNoScheduleThroughItsAfter(t *testing.T) {
	doc := func() []byte {
		return []byte(`{"nodes":[{"id":"n","label":"N","tier":0}],"links":[],` +
			`"metadata":{"fiscal_year":2026,"basis":"adopted"}}`)
	}
	_, ix, err := ColumnsOf(map[string][]byte{"year": doc(), "first": doc(), "second": doc()}, "")
	if err != nil {
		t.Fatal(err)
	}
	steps := []DrillStep{
		{Key: "a", After: []string{""}, Projection: "first"},
		{Key: "b", After: []string{""}, Projection: "second"},
		{Key: "c", After: []string{"a"}},
		{Key: "d", After: []string{""}},
	}
	got, err := StepStems(steps, "year", ix)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff([]string{"first", "second", "first", "year"}, got); diff != "" {
		t.Errorf("StepStems (-want +got):\n%s", diff)
	}
	steps = append(steps, DrillStep{Key: "e", After: []string{"a", "b"}})
	if _, err := StepStems(steps, "year", ix); err == nil || !strings.Contains(err.Error(), "opens from charts drawing") {
		t.Errorf("StepStems with two parents drawing two documents: err = %v", err)
	}
}

// columnTestDoc is one schedule's document for the refusal tests below: a
// printed ribbon a -> b and the nodes named by nodes, spliced in verbatim.
func columnTestDoc(nodes, links string) []byte {
	return []byte(`{
	  "metadata": {
	    "fiscal_year": 2026, "fiscal_year_label": "FY 2025-26", "basis": "adopted",
	    "scopes": ["revenue-by-fund"], "sources": [{"doc_id": "d", "pages": [1]}], "caveats": [],
	    "counts": {"facts": 1, "nodes": 2, "links": 1}
	  },
	  "nodes": [` + nodes + `],
	  "links": [` + links + `]
	}`)
}

const columnTestLink = `{"source": "a", "target": "b", "value_cents": 250, "kind": "external",
	 "fact_ids": ["fisc-f-0000000000bb"], "locators": [{"doc_id": "d", "pages": [1]}]}`

// TestColumnsOfRefusesWhatItCannotFold is the two refusals the fold makes on
// its own: two schedules meaning different things by one id, and a ribbon
// naming a node its document does not carry -- which, unrefused, would be
// written against index 0 of the node table.
func TestColumnsOfRefusesWhatItCannotFold(t *testing.T) {
	ab := `{"id": "a", "label": "A", "tier": 0}, {"id": "b", "label": "B", "tier": 1}`
	for _, tc := range []struct {
		name string
		docs map[string][]byte
		want string
	}{
		{"two schedules disagree about a node", map[string][]byte{
			"one": columnTestDoc(ab, columnTestLink),
			"two": columnTestDoc(`{"id": "a", "label": "Not A", "tier": 0}, {"id": "b", "label": "B", "tier": 1}`, columnTestLink),
		}, `"a" disagrees between schedules about the same node`},
		{"a ribbon names a node the document does not carry", map[string][]byte{
			"one": columnTestDoc(`{"id": "b", "label": "B", "tier": 1}`, columnTestLink),
		}, "link a -> b names a node the document does not carry"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := ColumnsOf(tc.docs, "t")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("ColumnsOf: err = %v, want one containing %q", err, tc.want)
			}
		})
	}
}

// TestEncodeColumnRefusesWhatItMayNotServe is the write's two refusals: bytes
// the published schema does not accept, and a derived node a schedule draws
// without the words that say why it exists. Each starts from a column that
// encodes, so a refusal is the perturbation's and not the fixture's.
func TestEncodeColumnRefusesWhatItMayNotServe(t *testing.T) {
	derived := `{"id": "a", "label": "A", "tier": 0, "derived": true,
	  "rationale": "why it exists", "source_note": "what it was read from"},
	  {"id": "b", "label": "B", "tier": 1}`
	column := func(t *testing.T) ColumnDoc {
		t.Helper()
		cols, _, err := ColumnsOf(map[string][]byte{"one": columnTestDoc(derived, columnTestLink)}, "t")
		if err != nil {
			t.Fatal(err)
		}
		return cols["fy2026-adopted.json"]
	}
	if b, err := encodeColumn(column(t)); err != nil {
		t.Fatalf("encodeColumn refused the unperturbed column: %v", err)
	} else if !strings.Contains(string(b), "what it was read from") {
		t.Fatalf("the derived node's source note did not reach the column: %s", b)
	}
	for _, tc := range []struct {
		name    string
		perturb func(*ColumnDoc)
		want    string
	}{
		{"a basis the schema does not list", func(c *ColumnDoc) { c.Column.Basis = "guessed" },
			"does not match " + schema.Column},
		{"a derived node with no source note", func(c *ColumnDoc) {
			c.Schedules["one"].Nodes[0].SourceNote = ""
		}, `draws derived node "a" without both a rationale and a source note`},
		{"a derived node with no rationale", func(c *ColumnDoc) {
			c.Schedules["one"].Nodes[0].Rationale = ""
		}, `draws derived node "a" without both a rationale and a source note`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := column(t)
			tc.perturb(&c)
			if _, err := encodeColumn(c); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("encodeColumn: err = %v, want one containing %q", err, tc.want)
			}
		})
	}
}
