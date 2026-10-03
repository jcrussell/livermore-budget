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

	"github.com/jcrussell/livermore-budget/internal/export"
)

// proseDenial is the rule the served prose is held to: a step description,
// a residual or gap reason or a caveat says what its own chart draws and
// from which pages, and denies nothing about what any other schedule prints.
// It is a tripwire, not a parser: it does not chase paraphrases, and fisc-tf5u
// owns the claim that a sentence is true against the pages.
var proseDenial = regexp.MustCompile(`(?i)no published|not broken down|no schedule|nothing published|prints? none|in any published|nowhere`)

// reasonFigure is a figure in a residual or gap reason, once its page
// citations are gone. A residual reason is shown under every column, so any
// figure in it is one column's; a gap reason is printed beside the gap the
// client computes against its licence, so a figure in it is a second spelling
// of that licence which nothing holds to it.
var (
	pageCite     = regexp.MustCompile(`\bpp?\.\s?\d+(-\d+)?`)
	reasonFigure = regexp.MustCompile(`(?i)[\d$]|\b(hundred|thousand|million|billion|dollars?)\b`)
)

var (
	htmlStyle = regexp.MustCompile(`(?is)<style.*?</style>`)
	htmlTag   = regexp.MustCompile(`<[^>]*>`)
)

// servedText is one string the export serves, and where.
type servedText struct{ where, words string }

// servedProse is every string one export at dir serves in its own words: the
// text of each page's markup, every string of each page's FISC_CONFIG, and
// every string of each JSON file outside facts/ and extracted/, whose records
// and page texts are the city's words. residuals and gaps are the reasons the
// configs' steps carry, which prose also holds.
func servedProse(dir string) (prose, residuals, gaps []servedText, err error) {
	var strs func(where string, v any)
	strs = func(where string, v any) {
		switch v := v.(type) {
		case string:
			prose = append(prose, servedText{where, v})
		case []any:
			for _, e := range v {
				strs(where, e)
			}
		case map[string]any:
			for k, e := range v {
				strs(where+" "+k, e)
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
			markup, config, cerr := splitPage(raw)
			if cerr != nil {
				markup, config = raw, nil
			}
			prose = append(prose, servedText{rel, htmlTag.ReplaceAllString(htmlStyle.ReplaceAllString(string(markup), " "), " ")})
			if config == nil {
				return nil
			}
			var v any
			if err := json.Unmarshal(config, &v); err != nil {
				return fmt.Errorf("%s: FISC_CONFIG: %w", rel, err)
			}
			strs(rel+" FISC_CONFIG", v)
			var c struct {
				Steps []export.DrillStep `json:"steps"`
			}
			if err := json.Unmarshal(config, &c); err != nil {
				return fmt.Errorf("%s: FISC_CONFIG steps: %w", rel, err)
			}
			for _, s := range c.Steps {
				for id, why := range s.Residual {
					residuals = append(residuals, servedText{rel + " step " + s.Key + " residual " + id, why})
				}
				for id, licences := range s.Gaps {
					for _, g := range licences {
						gaps = append(gaps, servedText{fmt.Sprintf("%s step %s gap %s FY%d %s", rel, s.Key, id, g.FiscalYear, g.Basis), g.Reason})
					}
				}
			}
		case ".json":
			var v any
			if err := json.Unmarshal(raw, &v); err != nil {
				return fmt.Errorf("%s: %w", rel, err)
			}
			strs(rel, v)
		}
		return nil
	})
	return prose, residuals, gaps, err
}

// TestServedProseDeniesNothingAndReasonsQuoteNoFigure holds every string
// servedProse collects from one real export to proseDenial, and every
// residual and gap reason to reasonFigure.
//
// COVERAGE IS THE COUNT OF STRINGS, not of files parsed: a config key renamed
// out from under export.DrillStep decodes to no reasons from a page that still
// parses, and the count is what sees it.
func TestServedProseDeniesNothingAndReasonsQuoteNoFigure(t *testing.T) {
	prose, residuals, gaps, err := servedProse(exportedSite(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%d served strings, %d residual reasons and %d gap reasons collected", len(prose), len(residuals), len(gaps))
	if len(prose) == 0 || len(residuals) == 0 || len(gaps) == 0 {
		t.Fatalf("%d served strings, %d residual reasons and %d gap reasons collected; the rule reached nothing",
			len(prose), len(residuals), len(gaps))
	}
	for _, p := range prose {
		if m := proseDenial.FindString(p.words); m != "" {
			i := strings.Index(p.words, m)
			t.Errorf("%s says %q: ...%s...", p.where, m, p.words[max(0, i-120):min(len(p.words), i+120)])
		}
	}
	for _, r := range append(residuals, gaps...) {
		if m := reasonFigure.FindString(pageCite.ReplaceAllString(r.words, "")); m != "" {
			t.Errorf("%s quotes %q: %s", r.where, m, r.words)
		}
	}
}
