package export

import (
	"encoding/json"
	"io/fs"
	"maps"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/jcrussell/livermore-budget/internal/project"
	"github.com/jcrussell/livermore-budget/internal/vocab"
	"github.com/jcrussell/livermore-budget/schema"
	"github.com/jcrussell/livermore-budget/site"
)

// TestTheSchemaStatesWhatThePageConfigCarries holds schema/page.schema.json to
// the structs [encodeConfig] marshals, both ways, over the whole tree. The
// template reads Go field names and never sees the JSON, so a dropped tag is
// silent until a reader switches year. The schema's additionalProperties: false
// is also what refuses a `text` on caveatRef.
func TestTheSchemaStatesWhatThePageConfigCarries(t *testing.T) {
	// Deep, because the Sankey hints are a $def, and a $ref is a leaf to
	// Names.
	stated, err := schema.NamesDeep(schema.Page)
	if err != nil {
		t.Fatalf("read %s: %v", schema.Page, err)
	}
	emitted := schema.StructNames(reflect.TypeOf(clientConfig{}), "")
	slices.Sort(stated)
	slices.Sort(emitted)
	if len(stated) == 0 {
		t.Fatalf("%s states no property, so this test compares nothing", schema.Page)
	}
	if diff := cmp.Diff(emitted, stated); diff != "" {
		t.Errorf("%s and window.FISC_CONFIG name different fields (-emitted +stated):\n%s\n"+
			"site/app.js reads these off CONFIG; a tag dropped on either side is a key "+
			"the page renders server-side and blanks after a year switch.", diff, schema.Page)
	}
}

// TestBasisLabelForRewritesOnlyTheAuditedBasisOfAnUnauditedDocument covers the
// three answers separately, because two of them are the ones a careless
// widening would break: a document without the caveat must keep its word, and a
// document with it must keep a basis that was never "audited".
func TestBasisLabelForRewritesOnlyTheAuditedBasisOfAnUnauditedDocument(t *testing.T) {
	unaudited := []caveatMeta{{ID: "some-other-caveat"}, {ID: project.UnauditedCaveatID}}
	other := []caveatMeta{{ID: "some-other-caveat"}}

	cases := []struct {
		name    string
		caveats []caveatMeta
		basis   string
		want    string
	}{
		{name: "audited under the caveat", caveats: unaudited, basis: string(vocab.BasisAudited), want: "unaudited"},
		{name: "audited without the caveat", caveats: other, basis: string(vocab.BasisAudited), want: string(vocab.BasisAudited)},
		{name: "audited with no caveats at all", caveats: nil, basis: string(vocab.BasisAudited), want: string(vocab.BasisAudited)},
		{name: "another basis under the caveat", caveats: unaudited, basis: "adopted", want: "adopted"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := basisLabelFor(tc.caveats, tc.basis); got != tc.want {
				t.Errorf("basisLabelFor(%v, %q) = %q, want %q", tc.caveats, tc.basis, got, tc.want)
			}
		})
	}
}

// TestTheWordingFillsEveryPlaceholderTheClientHands holds each wording
// template to the variables site/app.js fills it with: a placeholder the
// client never hands is left on the page as written, and a variable the
// template never names is a figure the reader is not shown.
func TestTheWordingFillsEveryPlaceholderTheClientHands(t *testing.T) {
	hands := map[string][]string{
		"counts":              {"links", "nodes", "facts"},
		"counts_partial":      {"links", "nodes", "cited", "facts"},
		"chip_fold":           {"label"},
		"chip_fold_label":     {"label", "split"},
		"split_chooser":       {"label"},
		"tail_chip":           {"label", "n", "word"},
		"tail_chip_label":     {"label", "n", "word"},
		"caveat_chip":         {"n"},
		"inferred_flows":      {"flows", "figure", "ends"},
		"aggregate_label":     {"folded", "word"},
		"aggregate_rationale": {"folded", "word"},
		"aggregate_note":      {"folded", "total", "cap"},
		"aggregate_together":  {"figure"},
		"balance_beginning":   {"figure"},
		"balance_ending":      {"figure"},
		"balance_change":      {"figure"},
		"filter_hidden":       {"cents", "flows"},
		"filter_folded":       {"flows", "ribbons", "threshold"},
		"filter_hidden_at":    {"cents", "flows"},
		"filter_drawn_at":     {"cents"},
		"smaller_label":       {"flows", "threshold"},
		"smaller_rationale":   {"flows", "threshold"},
		"smaller_note":        {"flows", "figure"},
		"smaller_flow":        {"flows", "threshold"},
		"search_count":        {"n"},
		"search_none":         {"query"},
		"hairline":            {"n"},
		"splits_dropped":      {"year", "nodes", "n"},
	}
	blob, err := json.Marshal(defaultWording())
	if err != nil {
		t.Fatal(err)
	}
	var templates map[string]string
	if err := json.Unmarshal(blob, &templates); err != nil {
		t.Fatal(err)
	}
	placeholder := regexp.MustCompile(`\{(\w+)(?::[^|}]*\|[^}]*)?\}`)
	for key, template := range templates {
		if template == "" {
			t.Errorf("wording %q is empty", key)
		}
		named := map[string]bool{}
		for _, m := range placeholder.FindAllStringSubmatch(template, -1) {
			named[m[1]] = true
		}
		want := map[string]bool{}
		for _, v := range hands[key] {
			want[v] = true
			if !named[v] {
				t.Errorf("wording %q never names {%s}, which the client hands it; the figure would not reach the page", key, v)
			}
		}
		for v := range named {
			if !want[v] {
				t.Errorf("wording %q names {%s}, which the client never hands it; the page would print the placeholder", key, v)
			}
		}
	}
}

// TestEverySplitKeyHasItsWords holds the page's words for what a node splits
// into to the split keys the expansion schedule declares, both ways: a key
// with no words is a chooser button the client cannot name, and words for no
// key are a promise about a split that is never drawn.
func TestEverySplitKeyHasItsWords(t *testing.T) {
	labels := splitLabels()
	got := slices.Sorted(maps.Keys(labels))
	want := slices.Sorted(slices.Values(project.SplitKeys()))
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("the page's split words and project.SplitKeys differ (-keys +words):\n%s", diff)
	}
	for k, w := range labels {
		if strings.TrimSpace(w) == "" {
			t.Errorf("split %q is said in no words", k)
		}
	}
}

// TestTheTemplatesCountsHeadIsTheWordingsCounts holds the counts line
// site/index.html.tmpl renders before app.js runs to the wording app.js
// repaints it with: the same sentence, the plural fixed as the template
// prints it, so a reworded default reaches both or neither.
func TestTheTemplatesCountsHeadIsTheWordingsCounts(t *testing.T) {
	tmpl, err := fs.ReadFile(site.FS(), "index.html.tmpl")
	if err != nil {
		t.Fatal(err)
	}
	served := strings.NewReplacer(
		"{links:flow|flows}", "{{.Links}} flows",
		"{nodes:node|nodes}", "{{.Nodes}} nodes",
		"{facts:fact|facts}", "{{.Facts}} facts",
	).Replace(defaultWording().Counts)
	if !strings.Contains(string(tmpl), `<span id="counts-line">`+served+`</span>`) {
		t.Errorf("site/index.html.tmpl does not render the counts line as %q; the page's first sentence and its repaint would differ", served)
	}
}
