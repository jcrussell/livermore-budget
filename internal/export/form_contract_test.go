package export_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/schema"
)

// TestTheSchemaHoldsAChartToItsForm is the arm Go's validate does not spell: a
// hint foreign to the declared form, a Sankey step with no tiers, and a flank
// on the overview are refused by schema/page.schema.json at the write. Over the
// served page's own config, so the control is a shape this build produces.
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

// step is one of a decoded config's steps, as a map.
func step(cfg map[string]any, i int) map[string]any {
	return cfg["steps"].([]any)[i].(map[string]any)
}
