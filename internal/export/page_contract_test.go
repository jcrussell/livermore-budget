package export

import (
	"encoding/json"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
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
		"split_chooser":       {"label"},
		"node_menu":           {"label"},
		"filter_only":         {"label"},
		"filter_out":          {"names"},
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
		"glyph_open":          {"label", "split"},
		"glyph_choose":        {"label"},
		"glyph_fold":          {"label"},
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

// TestTheFilterLineMarksItsCounts holds every sentence of the filter line
// that states how many flows the client took to the mark the client's
// arithmetic licence puts on a count of what is drawn (AGENTS.md, "Go vets,
// JavaScript renders"). Mutation: drop the ◇ from filter_hidden and this
// goes red.
func TestTheFilterLineMarksItsCounts(t *testing.T) {
	blob, err := json.Marshal(defaultWording())
	if err != nil {
		t.Fatal(err)
	}
	var templates map[string]string
	if err := json.Unmarshal(blob, &templates); err != nil {
		t.Fatal(err)
	}
	count := regexp.MustCompile(`\{\w+:[^|}]*\|[^}]*\}`)
	var counted []string
	for key, template := range templates {
		if !strings.HasPrefix(key, "filter_") || !count.MatchString(template) {
			continue
		}
		counted = append(counted, key)
		if !strings.Contains(template, "\u25c7") {
			t.Errorf("wording %q states a count the client takes and carries no \u25c7: %q", key, template)
		}
	}
	slices.Sort(counted)
	if diff := cmp.Diff([]string{"filter_folded", "filter_hidden", "filter_hidden_at"}, counted); diff != "" {
		t.Errorf("the filter line's counted sentences (-want +got):\n%s", diff)
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

// TestEverySplitKeyHasItsMemberNoun holds the page's noun for one member to
// the split keys the expansion schedule declares, both ways, and to the one
// variable the client fills it with.
func TestEverySplitKeyHasItsMemberNoun(t *testing.T) {
	nouns := memberNouns()
	got := slices.Sorted(maps.Keys(nouns))
	want := slices.Sorted(slices.Values(project.SplitKeys()))
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("the page's member nouns and project.SplitKeys differ (-keys +nouns):\n%s", diff)
	}
	for k, w := range nouns {
		if strings.TrimSpace(w) == "" {
			t.Errorf("a member of split %q is called nothing", k)
		}
		for _, m := range regexp.MustCompile(`\{(\w*)`).FindAllStringSubmatch(w, -1) {
			if m[1] != "fund" {
				t.Errorf("the member noun of split %q names {%s}; the client fills only {fund}", k, m[1])
			}
		}
	}
}

// TestAMemberNounNamingAFundIsSaidOfAFund holds each split whose member noun
// names {fund} to the committed column fixtures: every member of it the
// client qualifies, one labelled as its parent, carries the fund number the
// noun is filled with, so the client never meets a {fund} it has nothing for.
func TestAMemberNounNamingAFundIsSaidOfAFund(t *testing.T) {
	paths, err := filepath.Glob("../../testdata/*.column.json")
	if err != nil || len(paths) == 0 {
		t.Fatalf("no column fixture under testdata/ (%v), so this test reads nothing", err)
	}
	type colNode struct {
		ID    string `json:"id"`
		Label string `json:"label"`
		Fund  int    `json:"fund"`
	}
	type col struct {
		Nodes     []colNode `json:"nodes"`
		Schedules map[string]struct {
			Nodes []struct {
				Node        int    `json:"node"`
				Parent      string `json:"parent"`
				Alternative string `json:"alternative"`
			} `json:"nodes"`
			Splits []struct {
				Node        int    `json:"node"`
				Key         string `json:"key"`
				Alternative bool   `json:"alternative"`
			} `json:"splits"`
		} `json:"schedules"`
	}
	nouns := memberNouns()
	members := 0
	for _, p := range paths {
		b, err := os.ReadFile(p) // #nosec G304 -- a committed fixture.
		if err != nil {
			t.Fatal(err)
		}
		var c col
		if err := json.Unmarshal(b, &c); err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		for _, sched := range c.Schedules {
			for _, sp := range sched.Splits {
				if !strings.Contains(nouns[sp.Key], "{fund}") {
					continue
				}
				node := c.Nodes[sp.Node].ID
				for _, n := range sched.Nodes {
					if n.Parent != node || (sp.Alternative && n.Alternative != sp.Key) || (!sp.Alternative && n.Alternative != "") ||
						c.Nodes[n.Node].Label != c.Nodes[sp.Node].Label {
						continue
					}
					members++
					if c.Nodes[n.Node].Fund <= 0 {
						t.Errorf("%s: %s, a member of %s's split %q, carries no fund number for %q",
							filepath.Base(p), c.Nodes[n.Node].ID, node, sp.Key, nouns[sp.Key])
					}
				}
			}
		}
	}
	if members == 0 {
		t.Fatal("no member of a split whose noun names {fund} is labelled as its parent in any fixture, so this test holds nothing")
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

// TestEveryGestureSentenceSaysASecondClickFolds holds the two places the page
// says how a split node folds back -- an open hub's accessible name and the
// hint beside a chart with a split open -- to the activation that opened the
// node folding it: a sentence naming only ⊖ or the chip tells a reader the
// node's own click does something it does not.
func TestEveryGestureSentenceSaysASecondClickFolds(t *testing.T) {
	w := defaultWording()
	for name, s := range map[string]string{
		"desc_folds": w.DescFolds,
		"fold_back":  w.FoldBack,
	} {
		if !strings.Contains(s, "click") || !strings.Contains(s, "fold") {
			t.Errorf("%s does not say a click folds a split node back: %q", name, s)
		}
	}
	if !strings.Contains(w.FoldBack, "second click") {
		t.Errorf("fold_back does not name the second click: %q", w.FoldBack)
	}
	if w.DescFolds == w.DescFollows || w.DescFolds == w.DescOpens {
		t.Errorf("desc_folds %q says what another activation says", w.DescFolds)
	}
}
