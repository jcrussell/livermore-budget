package project

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/vocab"
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
		tag, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if tag == "" {
			t.Fatalf("%s.%s carries no json tag", rt.Name(), f.Name)
		}
		out = append(out, tag)
	}
	return out
}

// TestValidateCaveatsRefusesEveryShapeThatWouldRender: every case is a failure
// that renders. Mutation: `if true { return nil }` as validateCaveats' first
// statement fails each case.
func TestValidateCaveatsRefusesEveryShapeThatWouldRender(t *testing.T) {
	ok := Caveat{ID: "a", Theme: ThemeLeftOut, Summary: "s", Text: "t", AppliesTo: []string{}}
	nodes := map[string]struct{}{"fund-group/general": {}}

	for _, tc := range []struct {
		name    string
		in      []Caveat
		nodes   map[string]struct{}
		wantErr string
	}{
		{"a well-formed set passes", []Caveat{ok}, nodes, ""},
		{"no id", []Caveat{{Summary: "s", Text: "t"}}, nodes, "has no id"},
		{"no summary", []Caveat{{ID: "a", Theme: ThemeLeftOut, Text: "t"}}, nodes, "has no summary"},
		{"no text", []Caveat{{ID: "a", Theme: ThemeLeftOut, Summary: "s"}}, nodes, "has no text"},
		// Mutation: drop the theme arm of validateCaveats and both go red
		// with "validateCaveats = nil".
		{"no theme", []Caveat{{ID: "a", Summary: "s", Text: "t"}}, nodes, "which is none of CaveatThemes"},
		{"a theme of no set", []Caveat{{ID: "a", Theme: "caveats", Summary: "s", Text: "t"}}, nodes, "which is none of CaveatThemes"},
		// A data-files caveat is stated on no page, so no chart may mark one.
		// Mutation: drop the data-files arm -- "validateCaveats = nil".
		{
			"a data-files caveat naming a node",
			[]Caveat{{ID: "a", Theme: ThemeDataFiles, Summary: "s", Text: "t", AppliesTo: []string{"fund-group/general"}}},
			nodes, "is of theme data-files and names nodes",
		},
		// Mutation: drop ThemeChecked from that arm and this goes red with
		// "validateCaveats = nil".
		{
			"a checked caveat naming a node",
			[]Caveat{{ID: "a", Theme: ThemeChecked, Summary: "s", Text: "t", AppliesTo: []string{"fund-group/general"}}},
			nodes, "is of theme checked and names nodes",
		},
		{
			"a document-wide data-files caveat",
			[]Caveat{{ID: "a", Theme: ThemeDataFiles, Summary: "s", Text: "t", AppliesTo: []string{}}},
			nodes, "",
		},
		{
			"two caveats under one anchor",
			[]Caveat{ok, {ID: "a", Theme: ThemeLeftOut, Summary: "s2", Text: "t2"}},
			nodes, "used twice in one document",
		},
		{
			"a tracker id in the text",
			[]Caveat{{ID: "a", Theme: ThemeLeftOut, Summary: "s", Text: "Which figure to publish is open (fisc-av0w).", AppliesTo: []string{}}},
			nodes, `prints the tracker id "fisc-av0w"`,
		},
		{
			"a tracker id in the summary",
			[]Caveat{{ID: "a", Theme: ThemeLeftOut, Summary: "Neither block is published (fisc-ig9h).", Text: "t", AppliesTo: []string{}}},
			nodes, `prints the tracker id "fisc-ig9h"`,
		},
		// Mutation: drop the dotted tail from beadid's fisc pattern and this
		// goes red naming "fisc-yj4w" rather than the child.
		{
			"a dotted child's tracker id",
			[]Caveat{{ID: "a", Theme: ThemeLeftOut, Summary: "s", Text: "That is settled (fisc-yj4w.35.10).", AppliesTo: []string{}}},
			nodes, `prints the tracker id "fisc-yj4w.35.10"`,
		},
		// A byob decision is a tracker row a reader cannot open either.
		// Mutation: drop beadid.Cited's byob arm and both go red with
		// "validateCaveats = nil".
		{
			"a byob id",
			[]Caveat{{ID: "a", Theme: ThemeLeftOut, Summary: "s", Text: "The layout follows byob-layout.1.", AppliesTo: []string{}}},
			nodes, `prints the tracker id "byob-layout.1"`,
		},
		{
			"a hyphenated byob id",
			[]Caveat{{ID: "a", Theme: ThemeLeftOut, Summary: "Shaped by byob-command-shape.1, as decided.", Text: "t", AppliesTo: []string{}}},
			nodes, `prints the tracker id "byob-command-shape.1"`,
		},
		// A fact id, a series id and the export marker look like a bead id's head and
		// are not one.
		// Mutations: drop beadid.Cited's following-byte check on the fisc arm and the
		// fact and series cases go red; drop its preceding-byte check and the marker
		// case goes red naming the marker without its dot.
		{
			"a fact id",
			[]Caveat{{ID: "a", Theme: ThemeLeftOut, Summary: "s", Text: "Printed as fisc-f-0ba7b00dfaf7.", AppliesTo: []string{}}},
			nodes, "",
		},
		{
			"a series id",
			[]Caveat{{ID: "a", Theme: ThemeLeftOut, Summary: "Series fisc-s-1c2d3e4f5a6b.", Text: "t", AppliesTo: []string{}}},
			nodes, "",
		},
		{
			"the export marker",
			[]Caveat{{ID: "a", Theme: ThemeLeftOut, Summary: "s", Text: "The site carries .fisc-export.", AppliesTo: []string{}}},
			nodes, "",
		},
		{
			"applies to a node the document does not carry",
			[]Caveat{{ID: "a", Theme: ThemeLeftOut, Summary: "s", Text: "t", AppliesTo: []string{"fund-group/nope"}}},
			nodes, "which this document does not carry",
		},
		{
			"applies to a node the document does carry",
			[]Caveat{{ID: "a", Theme: ThemeLeftOut, Summary: "s", Text: "t", AppliesTo: []string{"fund-group/general"}}},
			nodes, "",
		},
		// nil nodes is the not-a-graph caller (the trends document), so the arm is
		// skipped; an empty map still refuses.
		{
			"a document with no nodes to check against",
			[]Caveat{{ID: "a", Theme: ThemeLeftOut, Summary: "s", Text: "t", AppliesTo: []string{"anything"}}},
			nil, "",
		},
		{
			"a graph that drew no nodes still refuses",
			[]Caveat{{ID: "a", Theme: ThemeLeftOut, Summary: "s", Text: "t", AppliesTo: []string{"anything"}}},
			map[string]struct{}{}, "which this document does not carry",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validateCaveats(tc.in, tc.nodes)
			switch {
			case tc.wantErr == "" && err != nil:
				t.Errorf("validateCaveats = %v, want no error", err)
			case tc.wantErr != "" && err == nil:
				t.Errorf("validateCaveats = nil, want an error naming %q", tc.wantErr)
			case tc.wantErr != "" && !strings.Contains(err.Error(), tc.wantErr):
				t.Errorf("validateCaveats = %q, want it to name %q", err, tc.wantErr)
			}
		})
	}
}

// TestTheEnvelopeAndTheGraphMetadataSpellTheirSharedKeysOnce: [Envelope] and
// [Metadata] spell their four shared keys alike; scope alone differs, singular
// against a list.
func TestTheEnvelopeAndTheGraphMetadataSpellTheirSharedKeysOnce(t *testing.T) {
	graph := jsonTags(t, Metadata{})
	for _, tag := range jsonTags(t, Envelope{}) {
		want := tag
		if tag == "scope" {
			want = "scopes"
		}
		if !slices.Contains(graph, want) {
			t.Errorf("Envelope publishes %q and Metadata does not publish %q", tag, want)
		}
	}
	// counts is shared only in its FIRST key: facts means the same thing in
	// both documents and must be spelled the same, while facts_cited is a
	// graph's alone because only there is it a different number from facts.
	if got, want := jsonTags(t, trendCounts{})[0], jsonTags(t, Counts{})[0]; got != want {
		t.Errorf("trendCounts leads with %q and Counts with %q; the two documents would "+
			"report the same quantity under different names", got, want)
	}
}

// TestTheGraphMetadataKeyOrderIsFrozen pins the published key order of every
// graph document's metadata and counts. The goldens compare bytes and would
// catch a reorder too; this catches it with a message that says WHAT moved.
func TestTheGraphMetadataKeyOrderIsFrozen(t *testing.T) {
	want := []string{
		"generated_by", "fiscal_year", "fiscal_year_label", "basis", "scopes",
		"currency", "units", "sources", "headline", "counts", "caveats",
	}
	if got := jsonTags(t, Metadata{}); !reflect.DeepEqual(got, want) {
		t.Errorf("metadata key order (-want +got):\n%v\n%v", want, got)
	}
	counts := []string{"facts", "facts_cited", "facts_uncited", "facts_cited_twice", "nodes", "links"}
	if got := jsonTags(t, Counts{}); !reflect.DeepEqual(got, counts) {
		t.Errorf("counts key order (-want +got):\n%v\n%v", counts, got)
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

// TestEncodeLeavesHTMLAlone: the shared encoder ships with HTML escaping off,
// unlike json.Marshal's default.
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

// TestRefuseUncitedAdmitsOnlyPrintedZeros: an uncited fact passes only as a
// printed zero or where the builder's allow names it.
func TestRefuseUncitedAdmitsOnlyPrintedZeros(t *testing.T) {
	zero := &fact.Fact{ID: "z", Kind: vocab.KindRevenue, Category: "taxes/property"}
	money := &fact.Fact{ID: "m", Kind: vocab.KindRevenue, Category: "taxes/property", AmountCents: 100}
	stock := &fact.Fact{ID: "s", Kind: vocab.KindFundBalance, Category: CategoryFundBalanceBeginning, AmountCents: 100}
	for _, tt := range []struct {
		name    string
		uncited []*fact.Fact
		allow   func(*fact.Fact) bool
		refused bool
	}{
		{"a printed zero", []*fact.Fact{zero}, nil, false},
		{"money no link carries", []*fact.Fact{zero, money}, nil, true},
		{"a stock row where the builder admits stocks", []*fact.Fact{stock}, isStock, false},
		{"a stock row where it does not", []*fact.Fact{stock}, nil, true},
		{"money where the builder admits only stocks", []*fact.Fact{money}, isStock, true},
	} {
		err := refuseUncited("t", tt.uncited, tt.allow)
		if (err != nil) != tt.refused {
			t.Errorf("%s: refuseUncited = %v, want refused %v", tt.name, err, tt.refused)
		}
	}
}

// TestEveryThemeHasItsWordsAndNoWordsAreOfNoTheme: caveatThemeWords and
// CaveatThemes name one set.
// Mutation: add a theme to caveatThemeWords alone and this goes red.
func TestEveryThemeHasItsWordsAndNoWordsAreOfNoTheme(t *testing.T) {
	var words []CaveatTheme
	for k := range caveatThemeWords {
		words = append(words, k)
	}
	slices.Sort(words)
	order := slices.Sorted(slices.Values(CaveatThemes()))
	if !slices.Equal(words, order) {
		t.Errorf("caveatThemeWords names %v and CaveatThemes %v", words, order)
	}
}
