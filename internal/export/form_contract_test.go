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
// hint foreign to the declared form, a Sankey step with no tiers, and a flank
// on the overview are refused by schema/page.schema.json at the write. Over the
// config of testdata/index.golden.html, the page this build writes over the
// client subset, so the control is a shape this build produces.
func TestTheSchemaHoldsAChartToItsForm(t *testing.T) {
	page, err := os.ReadFile("../../testdata/index.golden.html")
	if err != nil {
		t.Fatal(err)
	}
	blob := configBlob(t, string(page))
	for _, c := range []struct {
		name, want string
		breaks     func(cfg map[string]any)
	}{
		{"the served config", "", func(map[string]any) {}},
		{"a sankey step with no sankey hints", "sankey",
			func(cfg map[string]any) { delete(step(cfg, 0), "sankey") }},
		{"a sankey step whose hints name no tiers", "tiers",
			func(cfg map[string]any) { delete(step(cfg, 0)["sankey"].(map[string]any), "tiers") }},
		{"an overview keeping a flank", "keep",
			func(cfg map[string]any) {
				cfg["overview"].(map[string]any)["sankey"].(map[string]any)["keep"] = []any{0.0}
			}},
		{"a hint outside the form's own key", "widen",
			func(cfg map[string]any) { step(cfg, 0)["widen"] = []any{4.0} }},
		{"a bars step with no bars hints", "bars",
			func(cfg map[string]any) { delete(barsStep(t, cfg), "bars") }},
		{"a bars step carrying sankey hints", "sankey",
			func(cfg map[string]any) { barsStep(t, cfg)["sankey"] = map[string]any{"tiers": []any{2.0, 3.0}} }},
		{"a sankey step carrying bars hints", "bars",
			func(cfg map[string]any) { step(cfg, 0)["bars"] = map[string]any{"tier": 3.0} }},
		{"bars hints naming no tier", "tier",
			func(cfg map[string]any) { delete(barsStep(t, cfg)["bars"].(map[string]any), "tier") }},
		{"a key the bars hints do not declare", "caps",
			func(cfg map[string]any) { barsStep(t, cfg)["bars"].(map[string]any)["caps"] = []any{} }},
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

// barsStep is the first step of a decoded config drawn as bars, failing the
// test where the served config declares none: a control that broke nothing
// would pass as a refusal the schema never made.
func barsStep(t *testing.T, cfg map[string]any) map[string]any {
	t.Helper()
	for _, s := range cfg["steps"].([]any) {
		if m := s.(map[string]any); m["form"] == export.BarsForm {
			return m
		}
	}
	t.Fatal("the served config declares no bars step")
	return nil
}

// step is one of a decoded config's steps, as a map.
func step(cfg map[string]any, i int) map[string]any {
	return cfg["steps"].([]any)[i].(map[string]any)
}

// TestAHintIsSpelledOnceInTheFileAndOnTheWire holds every field of the
// declared hints to one key: data/views.yaml decodes the hints as these types,
// and the page config marshals them, so a field whose yaml and json keys
// differed would be authored under one name and shipped under another.
// Mutation: give SankeyHints.Keep the yaml key "kept", and it goes red.
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
