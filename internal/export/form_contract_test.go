package export_test

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/export"
	"github.com/jcrussell/livermore-budget/schema"
)

// TestTheSchemaHoldsAChartToItsForm is the arm Go's validate does not spell: a
// hint foreign to the declared form, and a form's hints on a chart of another
// form, are refused by schema/page.schema.json at the write. Over the config
// of testdata/index.golden.html, the page this build writes over the client
// subset, so the control is a shape this build produces.
func TestTheSchemaHoldsAChartToItsForm(t *testing.T) {
	page, err := os.ReadFile("../../testdata/index.golden.html")
	if err != nil {
		t.Fatal(err)
	}
	blob := configBlob(t, string(page))
	overview := func(cfg map[string]any) map[string]any { return cfg["overview"].(map[string]any) }
	hints := func(cfg map[string]any) map[string]any { return overview(cfg)["sankey"].(map[string]any) }
	for _, c := range []struct {
		name, want string
		breaks     func(cfg map[string]any)
	}{
		{"the served config", "", func(map[string]any) {}},
		{"a sankey chart with no sankey hints", "sankey",
			func(cfg map[string]any) { delete(overview(cfg), "sankey") }},
		{"an overview keeping a flank", "keep",
			func(cfg map[string]any) { hints(cfg)["keep"] = []any{0.0} }},
		{"a hint no chart declares", "widen",
			func(cfg map[string]any) { hints(cfg)["widen"] = []any{4.0} }},
		{"a sankey chart carrying bars hints", "bars",
			func(cfg map[string]any) { overview(cfg)["bars"] = map[string]any{"tier": 3.0} }},
		{"a bars chart with no bars hints", "bars",
			func(cfg map[string]any) { overview(cfg)["form"] = "bars"; delete(overview(cfg), "sankey") }},
		{"bars hints naming no tier", "tier", func(cfg map[string]any) {
			overview(cfg)["form"] = "bars"
			delete(overview(cfg), "sankey")
			overview(cfg)["bars"] = map[string]any{}
		}},
		{"a step, which no page declares", "steps",
			func(cfg map[string]any) { cfg["steps"] = []any{map[string]any{"key": "group"}} }},
	} {
		var cfg map[string]any
		if err := json.Unmarshal(blob, &cfg); err != nil {
			t.Fatal(err)
		}
		c.breaks(cfg)
		err := schema.Validate(schema.Page, cfg)
		switch {
		case c.want == "" && err != nil:
			t.Errorf("%s: refused: %v", c.name, err)
		case c.want != "" && err == nil:
			t.Errorf("%s: accepted, and the schema should have named %q", c.name, c.want)
		case c.want != "" && !strings.Contains(err.Error(), c.want):
			t.Errorf("%s: refused without naming %q: %v", c.name, c.want, err)
		}
	}
}

// TestTheSchemaHoldsTheWholePagesCanvas is the same arm over
// testdata/whole.golden.html's overview, which declares a canvas height and a
// dropped tier: a malformed one of either is refused at the write.
func TestTheSchemaHoldsTheWholePagesCanvas(t *testing.T) {
	page, err := os.ReadFile("../../testdata/whole.golden.html")
	if err != nil {
		t.Fatal(err)
	}
	blob := configBlob(t, string(page))
	hints := func(cfg map[string]any) map[string]any {
		return cfg["overview"].(map[string]any)["sankey"].(map[string]any)
	}
	for _, c := range []struct {
		name, want string
		breaks     func(cfg map[string]any)
	}{
		{"the served config", "", func(map[string]any) {}},
		{"a canvas of no height", "height", func(cfg map[string]any) { hints(cfg)["height"] = 0.0 }},
		{"a canvas of negative height", "height", func(cfg map[string]any) { hints(cfg)["height"] = -4800.0 }},
		{"a canvas of fractional height", "height", func(cfg map[string]any) { hints(cfg)["height"] = 4800.5 }},
		{"a canvas height as a string", "height", func(cfg map[string]any) { hints(cfg)["height"] = "4800" }},
		{"an empty drop", "drop", func(cfg map[string]any) { hints(cfg)["drop"] = []any{} }},
		{"a tier dropped twice", "drop", func(cfg map[string]any) { hints(cfg)["drop"] = []any{2.0, 2.0} }},
		{"a negative tier dropped", "drop", func(cfg map[string]any) { hints(cfg)["drop"] = []any{-2.0} }},
		{"an overview cap naming no tail", "tail", func(cfg map[string]any) {
			delete(hints(cfg)["caps"].([]any)[0].(map[string]any), "tail")
		}},
	} {
		var cfg map[string]any
		if err := json.Unmarshal(blob, &cfg); err != nil {
			t.Fatal(err)
		}
		if _, ok := hints(cfg)["height"]; !ok {
			t.Fatal("the whole page's overview declares no canvas height, so no row below breaks one")
		}
		c.breaks(cfg)
		err := schema.Validate(schema.Page, cfg)
		switch {
		case c.want == "" && err != nil:
			t.Errorf("%s: refused: %v", c.name, err)
		case c.want != "" && err == nil:
			t.Errorf("%s: accepted, and the schema should have named %q", c.name, c.want)
		case c.want != "" && !strings.Contains(err.Error(), c.want):
			t.Errorf("%s: refused without naming %q: %v", c.name, c.want, err)
		}
	}
}

// TestTheSchemaHoldsTheExpansionsCanvasAndCaps: the index page's overview
// grows its canvas and caps the expansion's splits, and schema/page.schema.json
// refuses each hint broken one way, and a growth beside a fixed height.
func TestTheSchemaHoldsTheExpansionsCanvasAndCaps(t *testing.T) {
	page, err := os.ReadFile("../../testdata/index.golden.html")
	if err != nil {
		t.Fatal(err)
	}
	blob := configBlob(t, string(page))
	hints := func(cfg map[string]any) map[string]any {
		return cfg["overview"].(map[string]any)["sankey"].(map[string]any)
	}
	grow := func(cfg map[string]any) map[string]any { return hints(cfg)["grow"].(map[string]any) }
	firstCap := func(cfg map[string]any) map[string]any {
		return hints(cfg)["split_caps"].([]any)[0].(map[string]any)
	}
	for _, c := range []struct {
		name, want string
		breaks     func(cfg map[string]any)
	}{
		{"the served config", "", func(map[string]any) {}},
		{"a growth of no px", "node_px", func(cfg map[string]any) { grow(cfg)["node_px"] = 0.0 }},
		{"a growth with no ceiling", "ceiling", func(cfg map[string]any) { delete(grow(cfg), "ceiling") }},
		{"a growth beside a height", "not", func(cfg map[string]any) { hints(cfg)["height"] = 4800.0 }},
		{"a split cap of no member", "cap", func(cfg map[string]any) { firstCap(cfg)["cap"] = 0.0 }},
		{"a split cap naming no split", "split", func(cfg map[string]any) { delete(firstCap(cfg), "split") }},
		{"a split cap naming no tail", "tail", func(cfg map[string]any) { delete(firstCap(cfg), "tail") }},
	} {
		var cfg map[string]any
		if err := json.Unmarshal(blob, &cfg); err != nil {
			t.Fatal(err)
		}
		if _, ok := hints(cfg)["grow"]; !ok {
			t.Fatal("the index page's overview declares no canvas growth, so no row below breaks one")
		}
		if _, ok := hints(cfg)["split_caps"]; !ok {
			t.Fatal("the index page's overview declares no split caps, so no row below breaks one")
		}
		c.breaks(cfg)
		err := schema.Validate(schema.Page, cfg)
		switch {
		case c.want == "" && err != nil:
			t.Errorf("%s: refused: %v", c.name, err)
		case c.want != "" && err == nil:
			t.Errorf("%s: accepted, and the schema should have named %q", c.name, c.want)
		case c.want != "" && !strings.Contains(err.Error(), c.want):
			t.Errorf("%s: refused without naming %q: %v", c.name, c.want, err)
		}
	}
}

// TestAHintIsSpelledOnceInTheFileAndOnTheWire holds every field of the
// declared hints to one key: data/views.yaml decodes the hints as these types,
// and the page config marshals them, so a field whose yaml and json keys
// differed would be authored under one name and shipped under another.
// Mutation: give SankeyHints.Drop the yaml key "dropped", and it goes red.
func TestAHintIsSpelledOnceInTheFileAndOnTheWire(t *testing.T) {
	for _, typ := range []reflect.Type{reflect.TypeOf(export.SankeyHints{}), reflect.TypeOf(export.TierCap{}), reflect.TypeOf(export.BarsHints{})} {
		for i := range typ.NumField() {
			f := typ.Field(i)
			j, _, _ := strings.Cut(f.Tag.Get("json"), ",")
			y, _, _ := strings.Cut(f.Tag.Get("yaml"), ",")
			if j == "" || j != y {
				t.Errorf("%s.%s is %q on the wire and %q in data/views.yaml", typ.Name(), f.Name, j, y)
			}
		}
	}
}
