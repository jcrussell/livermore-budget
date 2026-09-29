package export

import (
	"bytes"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/jcrussell/livermore-budget/internal/project"
	"github.com/jcrussell/livermore-budget/schema"
)

// TestFundGroupsAreOrderedAndOpenEnded holds [fundGroupsOf] to its order and
// to sorting a fund type the order does not name after those it does, rather
// than to the top of the column (fisc-zojk).
func TestFundGroupsAreOrderedAndOpenEnded(t *testing.T) {
	nodes := []ColumnNode{
		{ID: "revenue/taxes", Role: "revenue_source"},
		{ID: "fund-group/debt-service", Role: project.RoleFundGroup},
		{ID: "fund-group/permanent", Role: project.RoleFundGroup},
		{ID: "fund-group/general", Role: project.RoleFundGroup},
		{ID: "fund-group/aardvark", Role: project.RoleFundGroup},
		{ID: "fund/100", Role: "fund"},
	}
	got, err := fundGroupsOf(nodes)
	if err != nil {
		t.Fatalf("fundGroupsOf: %v", err)
	}
	want := []ColumnFundGroup{
		{ID: "fund-group/general", Slot: 3},
		{ID: "fund-group/debt-service", Slot: 6},
		// Neither is in the declared sequence, so both land after, in id
		// order, and wear no slot.
		{ID: "fund-group/aardvark", Slot: 0},
		{ID: "fund-group/permanent", Slot: 0},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("fundGroupsOf (-want +got):\n%s\n"+
			"site/app.js's fundGroupPlace reads this order and nothing else does; a "+
			"group the sequence does not name belongs last, not nowhere.", diff)
	}
}

// TestAFundGroupWithNoFundTypeInItsIDIsRefused is the fail-closed arm: a slug
// that is really a whole id draws muted with no other symptom.
func TestAFundGroupWithNoFundTypeInItsIDIsRefused(t *testing.T) {
	for _, id := range []string{"fundgroup", "fund-group/"} {
		if _, err := fundGroupsOf([]ColumnNode{{ID: id, Role: project.RoleFundGroup}}); err == nil {
			t.Errorf("fundGroupsOf accepted %q as a fund group; it names no fund type", id)
		}
	}
}

// TestTheSchemaStatesWhatAColumnCarries holds schema/column.schema.json to the
// struct encodeColumn marshals, both ways. The validator catches a key added to
// the struct, not a key the schema stops requiring.
func TestTheSchemaStatesWhatAColumnCarries(t *testing.T) {
	stated, err := schema.Names(schema.Column)
	if err != nil {
		t.Fatalf("read %s: %v", schema.Column, err)
	}
	emitted := schema.StructNames(reflect.TypeOf(ColumnDoc{}), "")

	// `counts` is a [json.RawMessage] this package copies, so the struct cannot
	// state its keys and the schema can; the exemption is for that prefix alone.
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

// TestAReductionsSentenceSurvivesTheFoldIntoAColumn: the packager decodes into
// an anonymous struct, so a key it does not name is dropped with no error and
// no schema failure.
//
// Drop Contra from decoded.Links and this goes red.
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

	cols, _, err := ColumnsOf(named(map[string][]byte{"fund-flows": doc}), "t")
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
	// The converse: omitempty makes empty look absent, and filling it in would
	// invent a reduction.
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
	_, ix, err := ColumnsOf(named(map[string][]byte{"year": doc(), "first": doc(), "second": doc()}), "")
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

// TestColumnsOfRefusesWhatItCannotFold is the fold's two refusals: two
// schedules meaning different things by one id, and a ribbon naming a node its
// document does not carry.
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
		// An annotation is the node's, so a schedule stating one the other
		// leaves blank is refused, not merged.
		{"two schedules disagree about a node's constraint tier", map[string][]byte{
			"one": columnTestDoc(ab, columnTestLink),
			"two": columnTestDoc(`{"id": "a", "label": "A", "tier": 0, "constraint_tier": "committed",
			  "rationale": "read", "source_note": "from"}, {"id": "b", "label": "B", "tier": 1}`, columnTestLink),
		}, `"a" disagrees between schedules about the same node`},
		{"a ribbon names a node the document does not carry", map[string][]byte{
			"one": columnTestDoc(`{"id": "b", "label": "B", "tier": 1}`, columnTestLink),
		}, "link a -> b names a node the document does not carry"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := ColumnsOf(named(tc.docs), "t")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("ColumnsOf: err = %v, want one containing %q", err, tc.want)
			}
		})
	}
}

// TestTwoSchedulesMayHangOneNodeDifferently: the parent is the schedule's,
// so two schedules agreeing on a node's table entry and disagreeing about
// where it hangs fold into one node with two parent edges.
func TestTwoSchedulesMayHangOneNodeDifferently(t *testing.T) {
	cols, _, err := ColumnsOf(named(map[string][]byte{
		"one": columnTestDoc(`{"id": "a", "label": "A", "tier": 0}, {"id": "b", "label": "B", "tier": 1, "parent": "a"}`, columnTestLink),
		"two": columnTestDoc(`{"id": "a", "label": "A", "tier": 0}, {"id": "b", "label": "B", "tier": 1}`, columnTestLink),
	}), "t")
	if err != nil {
		t.Fatalf("ColumnsOf refused two schedules that differ only in a parent edge: %v", err)
	}
	col := cols["fy2026-adopted.json"]
	if len(col.Nodes) != 2 {
		t.Fatalf("the column carries %d nodes, want the two the schedules share", len(col.Nodes))
	}
	parents := map[string]string{}
	for key, s := range col.Schedules {
		for _, n := range s.Nodes {
			if col.Nodes[n.Node].ID == "b" {
				parents[key] = n.Parent
			}
		}
	}
	if parents["one"] != "a" || parents["two"] != "" {
		t.Errorf("b hangs under %q in one and %q in two; want a and none", parents["one"], parents["two"])
	}
}

// TestEncodeColumnRefusesWhatItMayNotServe is the write's refusals, all of
// them the schema's: a value it does not list, and a derived node in the table
// without its words. Each starts from a column that encodes.
func TestEncodeColumnRefusesWhatItMayNotServe(t *testing.T) {
	derived := `{"id": "a", "label": "A", "tier": 0, "derived": true,
	  "rationale": "why it exists", "source_note": "what it was read from"},
	  {"id": "b", "label": "B", "tier": 1}`
	column := func(t *testing.T) ColumnDoc {
		t.Helper()
		cols, _, err := ColumnsOf(named(map[string][]byte{"one": columnTestDoc(derived, columnTestLink)}), "t")
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
		{"a derived node with no source note", func(c *ColumnDoc) { c.Nodes[0].SourceNote = "" },
			"does not match " + schema.Column},
		{"a derived node with no rationale", func(c *ColumnDoc) { c.Nodes[0].Rationale = "" },
			"does not match " + schema.Column},
		{"a tier with no note it was read from", func(c *ColumnDoc) {
			c.Nodes[1].ConstraintTier = "committed"
		}, "does not match " + schema.Column},
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

// named gives each hand-built document the projection its stem names, as
// every built document states its own.
func named(docs map[string][]byte) map[string][]byte {
	out := make(map[string][]byte, len(docs))
	for stem, raw := range docs {
		out[stem] = append([]byte(`{"projection": "`+stem+`", `), bytes.TrimPrefix(bytes.TrimSpace(raw), []byte("{"))...)
	}
	return out
}

// TestColumnsOfRefusesADocumentNamingNoProjection: the schedule a document
// becomes is the projection it states, so one stating none has no schedule.
func TestColumnsOfRefusesADocumentNamingNoProjection(t *testing.T) {
	_, _, err := ColumnsOf(map[string][]byte{"one": columnTestDoc(`{"id": "a", "label": "A", "tier": 0}, {"id": "b", "label": "B", "tier": 1}`, columnTestLink)}, "t")
	if err == nil || !strings.Contains(err.Error(), "states no projection") {
		t.Fatalf("ColumnsOf: err = %v, want the missing projection named", err)
	}
}
