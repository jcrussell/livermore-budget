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
	spine := jsonTags(t, Metadata{})
	trends := jsonTags(t, TrendsMetadata{})

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
	if got, want := jsonTags(t, TrendCounts{})[0], jsonTags(t, Counts{})[0]; got != want {
		t.Errorf("TrendCounts leads with %q and Counts with %q; the two documents would "+
			"report the same quantity under different names", got, want)
	}
	for _, tag := range jsonTags(t, TrendCounts{}) {
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
	if got := jsonTags(t, Metadata{}); !reflect.DeepEqual(got, want) {
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
	if got := jsonTags(t, TrendsMetadata{}); !reflect.DeepEqual(got, want) {
		t.Errorf("metadata key order (-want +got):\n%v\n%v", want, got)
	}
}

// TestEncodeLeavesHTMLAlone is the one extraction the byte test genuinely earns
// (fisc-2u4): a second projection that built its own encoder would ship with
// escaping ON, because that is json.Marshal's default, and nothing would say so
// until a reader saw an ampersand entity on the page.
func TestEncodeLeavesHTMLAlone(t *testing.T) {
	got, err := encode(map[string]string{"label": "Fines & Forfeitures"}, "test")
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if !strings.Contains(string(got), "Fines & Forfeitures") {
		t.Errorf("got %s, want the ampersand unescaped", got)
	}
	var back map[string]string
	if err := json.Unmarshal(got, &back); err != nil {
		t.Fatalf("the encoder produced undecodable JSON: %v", err)
	}
}
