package export

import (
	"encoding/json"
	"io/fs"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/jcrussell/livermore-budget/internal/mapping"
	"github.com/jcrussell/livermore-budget/schema"
	"github.com/jcrussell/livermore-budget/site"
)

// TestTheSchemaStatesWhatThePageConfigCarries holds schema/page.schema.json to
// the structs [encodeConfig] marshals, both ways, over the whole tree. The
// template reads Go field names and never sees the JSON, so a dropped tag is
// silent until a reader switches year. The schema's additionalProperties: false
// is also what refuses a `text` on caveatRef.
func TestTheSchemaStatesWhatThePageConfigCarries(t *testing.T) {
	stated, err := schema.Names(schema.Page)
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

// TestBasisAuditedIsPinnedToTheEnum keeps [basisLabelFor]'s subject in step with
// the producer's basis value.
//
// This package does not import internal/project, and basisAudited is a second
// copy of what internal/mapping declares. The copy is what makes the drift
// silent: if the enum's value moved, basisLabelFor would simply stop matching
// and both ACFR ten-year pages would go back to printing the word their own
// caveat withdraws, with every other test still green.
func TestBasisAuditedIsPinnedToTheEnum(t *testing.T) {
	if basisAudited != string(mapping.BasisAudited) {
		t.Errorf("basisAudited is %q but mapping.BasisAudited is %q; basisLabelFor "+
			"matches on this string, so a mismatch silently stops relabelling the "+
			"columns the statistical-section-unaudited caveat covers",
			basisAudited, mapping.BasisAudited)
	}
}

// TestBasisLabelForRewritesOnlyTheAuditedBasisOfAnUnauditedDocument covers the
// three answers separately, because two of them are the ones a careless
// widening would break: a document without the caveat must keep its word, and a
// document with it must keep a basis that was never "audited".
func TestBasisLabelForRewritesOnlyTheAuditedBasisOfAnUnauditedDocument(t *testing.T) {
	unaudited := []caveatMeta{{ID: "some-other-caveat"}, {ID: UnauditedCaveatID}}
	other := []caveatMeta{{ID: "some-other-caveat"}}

	cases := []struct {
		name    string
		caveats []caveatMeta
		basis   string
		want    string
	}{
		{name: "audited under the caveat", caveats: unaudited, basis: basisAudited, want: "unaudited"},
		{name: "audited without the caveat", caveats: other, basis: basisAudited, want: basisAudited},
		{name: "audited with no caveats at all", caveats: nil, basis: basisAudited, want: basisAudited},
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
		"counts_carried":      {"links", "nodes", "own", "cited", "facts", "carried"},
		"counts_carried_from": {"above", "theirs"},
		"opened_hint":         {"label"},
		"open_further":        {"where"},
		"open_into":           {"where"},
		"in_column":           {"columns"},
		"back_control":        {"back"},
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
