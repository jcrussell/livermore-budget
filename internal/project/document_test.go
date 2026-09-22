package project

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// jsonTags is the JSON key each field of a struct publishes, in declaration
// order, which is the order encoding/json emits them in.
func jsonTags(t *testing.T, v any) []string {
	t.Helper()
	rt := reflect.TypeOf(v)
	out := make([]string, 0, rt.NumField())
	for i := range rt.NumField() {
		f := rt.Field(i)
		if f.Anonymous {
			out = append(out, jsonTags(t, reflect.New(f.Type).Elem().Interface())...)
			continue
		}
		tag := f.Tag.Get("json")
		if tag == "" {
			t.Fatalf("%s.%s carries no json tag", rt.Name(), f.Name)
		}
		out = append(out, tag)
	}
	return out
}

// TestValidateCaveatsRefusesEveryShapeThatWouldRender, and that it refuses
// EVERY such shape is the point of it.
//
// THE GUARD HAD NO TEST AT ALL: adding `if true { return nil }` as its first
// statement left `go test ./...` entirely green.
// All five refusals were unverified in a function three
// builders call.
//
// EVERY CASE HERE IS A FAILURE THAT RENDERS, which is why the guard exists at
// build time rather than being left to a page to notice. An empty id publishes
// an anchor of "#caveat-<stem>--", which every summary on the site would then
// share; an empty summary a blank line in a list; a repeated id two paragraphs
// under one anchor, of which the reader sees whichever the browser finds first;
// and an AppliesTo naming an absent node marks nothing, which is
// indistinguishable from having nothing to mark.
func TestValidateCaveatsRefusesEveryShapeThatWouldRender(t *testing.T) {
	ok := Caveat{ID: "a", Summary: "s", Text: "t", AppliesTo: []string{}}
	nodes := map[string]struct{}{"fund-group/general": {}}

	for _, tc := range []struct {
		name    string
		in      []Caveat
		nodes   map[string]struct{}
		wantErr string
	}{
		{"a well-formed set passes", []Caveat{ok}, nodes, ""},
		{"no id", []Caveat{{Summary: "s", Text: "t"}}, nodes, "has no id"},
		{"no summary", []Caveat{{ID: "a", Text: "t"}}, nodes, "has no summary"},
		{"no text", []Caveat{{ID: "a", Summary: "s"}}, nodes, "has no text"},
		{
			"two caveats under one anchor",
			[]Caveat{ok, {ID: "a", Summary: "s2", Text: "t2"}},
			nodes, "used twice in one document",
		},
		{
			"applies to a node the document does not carry",
			[]Caveat{{ID: "a", Summary: "s", Text: "t", AppliesTo: []string{"fund-group/nope"}}},
			nodes, "which this document does not carry",
		},
		{
			"applies to a node the document does carry",
			[]Caveat{{ID: "a", Summary: "s", Text: "t", AppliesTo: []string{"fund-group/general"}}},
			nodes, "",
		},
		// nil NODES IS THE not-a-graph CALLER, and the arm has to be skipped
		// rather than made to fail against an empty set: internal/project's
		// trends document publishes series and has no node for an AppliesTo to
		// name. Passing an EMPTY map instead is the other case, and it must
		// still refuse -- a graph that drew nothing is not a document exempt
		// from the rule.
		{
			"a document with no nodes to check against",
			[]Caveat{{ID: "a", Summary: "s", Text: "t", AppliesTo: []string{"anything"}}},
			nil, "",
		},
		{
			"a graph that drew no nodes still refuses",
			[]Caveat{{ID: "a", Summary: "s", Text: "t", AppliesTo: []string{"anything"}}},
			map[string]struct{}{}, "which this document does not carry",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validateCaveats(tc.in, tc.nodes)
			switch {
			case tc.wantErr == "" && err != nil:
				t.Errorf("ValidateCaveats = %v, want no error", err)
			case tc.wantErr != "" && err == nil:
				t.Errorf("ValidateCaveats = nil, want an error naming %q", tc.wantErr)
			case tc.wantErr != "" && !strings.Contains(err.Error(), tc.wantErr):
				t.Errorf("ValidateCaveats = %q, want it to name %q", err, tc.wantErr)
			}
		})
	}
}

// TestSharedMetadataTagsHaveNotDrifted is what couples the two metadata structs,
// because nothing else does.
//
// fisc-2u4 chose to let Metadata and TrendsMetadata share field names WITHOUT
// sharing a declaration, and the reason is bytes: encoding/json emits fields in
// declaration order, Metadata's shared fields are interleaved with the spine's
// own (generated_by, fiscal_year, fiscal_year_label, basis, scope, currency,
// units, ...), so embedding Envelope there would reorder the keys and change
// testdata/sankey.golden.json.
//
// The cost of that choice is exactly this: two structs that must agree and no
// compiler that makes them. A renamed key on one side would publish two
// different names for one concept across two documents on one site, and every
// test in this package would still pass.
func TestSharedMetadataTagsHaveNotDrifted(t *testing.T) {
	spine := jsonTags(t, metadata{})
	trends := jsonTags(t, trendsMetadata{})

	have := func(tags []string, want string) bool {
		for _, tag := range tags {
			if tag == want {
				return true
			}
		}
		return false
	}
	// Every key Envelope declares must appear under BOTH metadata blocks, spelled
	// the same. That is the whole of the shared contract; the rest of each struct
	// is its own document's business.
	for _, tag := range jsonTags(t, Envelope{}) {
		if !have(spine, tag) {
			t.Errorf("Envelope publishes %q and the spine's Metadata does not", tag)
		}
		if !have(trends, tag) {
			t.Errorf("Envelope publishes %q and TrendsMetadata does not", tag)
		}
	}
	// counts is shared only in its FIRST key, and that is deliberate: facts means
	// the same thing in both documents and must be spelled the same, while
	// facts_cited is the spine's alone because only there is it a different
	// number from facts. See TrendCounts.
	if got, want := jsonTags(t, trendCounts{})[0], jsonTags(t, counts{})[0]; got != want {
		t.Errorf("TrendCounts leads with %q and Counts with %q; the two documents would "+
			"report the same quantity under different names", got, want)
	}
	for _, tag := range jsonTags(t, trendCounts{}) {
		if tag == "facts_cited" {
			t.Error("TrendCounts publishes facts_cited, which equals facts and points here; " +
				"three names for one number is a key for a concept the code does not compute")
		}
	}
}

// TestTheSpineMetadataKeyOrderIsFrozen is the other half: the reason Metadata
// does not embed Envelope is that its key order is published, so that order is
// asserted rather than left to whoever next edits the struct.
//
// testdata/sankey.golden.json is compared byte for byte elsewhere and would
// catch a reorder too. This catches it with a message that says WHAT moved.
func TestTheSpineMetadataKeyOrderIsFrozen(t *testing.T) {
	want := []string{
		"generated_by", "fiscal_year", "fiscal_year_label", "basis", "scope",
		"currency", "units", "sources", "headline", "counts", "caveats",
	}
	if got := jsonTags(t, metadata{}); !reflect.DeepEqual(got, want) {
		t.Errorf("metadata key order (-want +got):\n%v\n%v", want, got)
	}
}

// TestTheTrendsMetadataKeyOrderMatchesTheContract pins
// docs/revenue-trends-contract.md against the code, the same way the golden file
// pins the spine's.
func TestTheTrendsMetadataKeyOrderMatchesTheContract(t *testing.T) {
	want := []string{
		"generated_by", "scope", "currency", "units",
		"columns", "sources", "counts", "caveats",
	}
	if got := jsonTags(t, trendsMetadata{}); !reflect.DeepEqual(got, want) {
		t.Errorf("metadata key order (-want +got):\n%v\n%v", want, got)
	}
}

// TestEncodeLeavesHTMLAlone is the one extraction the byte test genuinely earns
// (fisc-2u4): a second projection that built its own encoder would ship with
// escaping ON, because that is json.Marshal's default, and nothing would say so
// until a reader saw an ampersand entity on the page.
func TestEncodeLeavesHTMLAlone(t *testing.T) {
	got, err := marshal(map[string]string{"label": "Fines & Forfeitures"}, "test")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(got), "Fines & Forfeitures") {
		t.Errorf("got %s, want the ampersand unescaped", got)
	}
	var back map[string]string
	if err := json.Unmarshal(got, &back); err != nil {
		t.Fatalf("the encoder produced undecodable JSON: %v", err)
	}
}

// TestTheFundFlowsMetadataKeyOrderMatchesTheContract pins
// docs/general-fund-drilldown-contract.md against the code.
//
// A THIRD DOCUMENT NEEDS A THIRD TEST, and that is not obvious from the two
// above it. TestSharedMetadataTagsHaveNotDrifted compares exactly two types and
// asserts only that every Envelope tag APPEARS in both; key ORDER is pinned per
// type, here and in the two tests above, and a new shape inherits neither.
func TestTheFundFlowsMetadataKeyOrderMatchesTheContract(t *testing.T) {
	want := []string{
		"generated_by", "scopes", "currency", "units",
		"fiscal_year", "fiscal_year_label", "basis", "sources", "counts", "caveats",
	}
	if got := jsonTags(t, FundFlowsMetadata{}); !reflect.DeepEqual(got, want) {
		t.Errorf("metadata key order (-want +got):\n%v\n%v", want, got)
	}
}

// TestTheMultiScopeEnvelopeIsTheEnvelopeWithOneKeyPluralised is what keeps the
// sibling from drifting into a second, differently-shaped preamble.
//
// MultiScopeEnvelope exists because Envelope.Scope is one string and a document
// of two schedules cannot write Scopes[0] into it without publishing one
// schedule as the whole of it. That is a reason to change ONE key, and the test
// says so: the other three are identical, in the same positions, so a reader who
// knows where generated_by and units are in one document finds them in the other.
func TestTheMultiScopeEnvelopeIsTheEnvelopeWithOneKeyPluralised(t *testing.T) {
	single := jsonTags(t, Envelope{})
	multi := jsonTags(t, MultiScopeEnvelope{})
	if len(single) != len(multi) {
		t.Fatalf("the two envelopes have %d and %d keys; they differ by one key's TYPE, "+
			"not by their contents:\n%v\n%v", len(single), len(multi), single, multi)
	}
	for i := range single {
		switch {
		case single[i] == "scope" && multi[i] == "scopes":
			// The one intended difference.
		case single[i] != multi[i]:
			t.Errorf("key %d is %q in Envelope and %q in MultiScopeEnvelope; only scope "+
				"may differ, and only by becoming plural", i, single[i], multi[i])
		}
	}
}

// TestTheFundFlowsCountsPublishTheirOwnIdentity pins the shape of a count block
// that deliberately is NOT Counts.
//
// The spine's identity assumes each fact is behind at most one link and that
// some rows are stocks. Neither holds here, so borrowing Counts would publish a
// facts_cited a reader would subtract from facts and get the wrong answer.
func TestTheFundFlowsCountsPublishTheirOwnIdentity(t *testing.T) {
	want := []string{"facts", "facts_cited", "facts_uncited", "facts_cited_twice",
		"nodes", "links"}
	if got := jsonTags(t, FundFlowsCounts{}); !reflect.DeepEqual(got, want) {
		t.Errorf("counts key order (-want +got):\n%v\n%v", want, got)
	}
	// It must NOT be mistakable for the spine's block, whose facts_cited means
	// something else.
	if reflect.DeepEqual(jsonTags(t, FundFlowsCounts{}), jsonTags(t, counts{})) {
		t.Error("the two count blocks have the same keys; a reader would apply the spine's " +
			"identity to a document that does not hold it")
	}
}
