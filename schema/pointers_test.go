package schema_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// schemaCitation is a schema file, and optionally a JSON pointer into it, as
// site/app.js names the shape a typedef stands for.
var schemaCitation = regexp.MustCompile(`schema/([a-z-]+\.schema\.json)(#[A-Za-z0-9_/-]*)?`)

// TestEverySchemaPathSiteAppJSNamesResolves holds the client's typedefs to
// schema/: each names a shape by its schema path and states none of its
// fields, so a path the schema no longer has is the only way one can drift.
func TestEverySchemaPathSiteAppJSNamesResolves(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "site", "app.js"))
	if err != nil {
		t.Fatal(err)
	}
	cited := schemaCitation.FindAllStringSubmatch(string(src), -1)
	if len(cited) == 0 {
		t.Fatal("site/app.js names no schema path, so nothing here was held")
	}
	for _, c := range cited {
		raw, err := os.ReadFile(c[1])
		if err != nil {
			t.Errorf("%s: %v", c[0], err)
			continue
		}
		var doc any
		if err := json.Unmarshal(raw, &doc); err != nil {
			t.Fatalf("%s: %v", c[1], err)
		}
		at := doc
		for _, key := range strings.Split(strings.TrimPrefix(c[2], "#"), "/") {
			if key == "" {
				continue
			}
			m, ok := at.(map[string]any)
			if !ok {
				at = nil
				break
			}
			at = m[key]
		}
		if at == nil {
			t.Errorf("site/app.js names %s, which %s does not have", c[0], c[1])
		}
	}
	t.Logf("%d schema paths named in site/app.js, each resolved", len(cited))
}
