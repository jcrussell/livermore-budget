package export

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// proseDenial is the tripwire the served prose is held to: a sentence says what
// its own chart draws and denies nothing about other schedules.
var proseDenial = regexp.MustCompile(`(?i)no published|not broken down|no schedule|nothing published|prints? none|in any published|nowhere`)

var (
	htmlStyle = regexp.MustCompile(`(?is)<style.*?</style>`)
	htmlTag   = regexp.MustCompile(`<[^>]*>`)
)

// servedText is one string the export serves, and where.
type servedText struct{ where, words string }

// servedProse is every string one export at dir serves in its own words; kinds
// counts them by source: "markup", "config" and "json".
func servedProse(dir string) (prose []servedText, kinds map[string]int, err error) {
	kinds = map[string]int{}
	var strs func(kind, where string, v any)
	strs = func(kind, where string, v any) {
		switch v := v.(type) {
		case string:
			prose = append(prose, servedText{where, v})
			kinds[kind]++
		case []any:
			for _, e := range v {
				strs(kind, where, e)
			}
		case map[string]any:
			for k, e := range v {
				strs(kind, where+" "+k, e)
			}
		}
	}
	err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, path)
		if d.IsDir() {
			if rel == "facts" || rel == "extracted" {
				return filepath.SkipDir
			}
			return nil
		}
		raw, err := os.ReadFile(path) // #nosec G304 -- a temp dir this test wrote.
		if err != nil {
			return err
		}
		switch filepath.Ext(path) {
		case ".html":
			markup, config, cerr := flowsConfig.split(raw)
			if cerr != nil {
				markup, config = raw, nil
			}
			prose = append(prose, servedText{rel, htmlTag.ReplaceAllString(htmlStyle.ReplaceAllString(string(markup), " "), " ")})
			kinds["markup"]++
			if config == nil {
				return nil
			}
			var v any
			if err := json.Unmarshal(config, &v); err != nil {
				return fmt.Errorf("%s: FISC_CONFIG: %w", rel, err)
			}
			strs("config", rel+" FISC_CONFIG", v)
		case ".json":
			var v any
			if err := json.Unmarshal(raw, &v); err != nil {
				return fmt.Errorf("%s: %w", rel, err)
			}
			strs("json", rel, v)
		}
		return nil
	})
	return prose, kinds, err
}

// TestServedProseDeniesNothing holds every string servedProse collects from
// one real export to proseDenial; coverage is the count per kind.
func TestServedProseDeniesNothing(t *testing.T) {
	prose, kinds, err := servedProse(exportedSite(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%d served strings collected: %d of page markup, %d of page configs, %d of JSON files",
		len(prose), kinds["markup"], kinds["config"], kinds["json"])
	for _, kind := range []string{"markup", "config", "json"} {
		if kinds[kind] == 0 {
			t.Fatalf("no served string of %s was collected; the rule reached none of them", kind)
		}
	}
	for _, p := range prose {
		if m := proseDenial.FindString(p.words); m != "" {
			i := strings.Index(p.words, m)
			t.Errorf("%s says %q: ...%s...", p.where, m, p.words[max(0, i-120):min(len(p.words), i+120)])
		}
	}
}
