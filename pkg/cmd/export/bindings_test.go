package export

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

// bindingsDoc is schema/bindings.json: the DOM each client module binds, per
// page, which site/bindings.test.mjs holds the client's skeletons to.
type bindingsDoc struct {
	Comment string `json:"$comment"`
	Chrome  struct {
		Comment   string   `json:"$comment"`
		Selectors []string `json:"selectors"`
	} `json:"chrome"`
	Pages map[string]struct {
		Skeleton  string   `json:"skeleton"`
		Modules   []string `json:"modules"`
		Selectors []string `json:"selectors"`
	} `json:"pages"`
}

func readBindings(t *testing.T) bindingsDoc {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(repoRootForTest(t), "schema", "bindings.json"))
	if err != nil {
		t.Fatal(err)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var doc bindingsDoc
	if err := dec.Decode(&doc); err != nil {
		t.Fatalf("schema/bindings.json: %v", err)
	}
	if len(doc.Pages) == 0 || len(doc.Chrome.Selectors) == 0 {
		t.Fatal("schema/bindings.json binds nothing, so every page below would pass")
	}
	return doc
}

// compound is one selector part: tag, id, classes and attributes, all of
// which an element must carry.
type compound struct {
	tag, id string
	classes []string
	attrs   []selAttr
}

type selAttr struct {
	name, value string
	valued      bool
}

// selStep is a compound and how it joins the step before it: ' ' any
// ancestor, '>' the parent. The first step's join is unused.
type selStep struct {
	join byte
	part compound
}

var (
	compoundRE = regexp.MustCompile(`^([a-z][a-z0-9-]*)?((?:#[A-Za-z0-9_-]+|\.[A-Za-z0-9_-]+|\[[a-z-]+(?:="[^"]*")?\])*)$`)
	pieceRE    = regexp.MustCompile(`#[A-Za-z0-9_-]+|\.[A-Za-z0-9_-]+|\[[a-z-]+(?:="[^"]*")?\]`)
	attrRE     = regexp.MustCompile(`^\[([a-z-]+)(?:="([^"]*)")?\]$`)
)

// parseSelector reads the selector grammar schema/bindings.json states, and
// refuses any other: a selector this cannot read would match nothing here
// and something in the browser.
func parseSelector(s string) ([]selStep, error) {
	var steps []selStep
	join := byte(' ')
	for _, tok := range strings.Fields(s) {
		if tok == ">" {
			if len(steps) == 0 || join == '>' {
				return nil, fmt.Errorf("%q: a '>' with nothing on one side", s)
			}
			join = '>'
			continue
		}
		m := compoundRE.FindStringSubmatch(tok)
		if m == nil || tok == "" {
			return nil, fmt.Errorf("%q: %q is not tag, #id, .class, [attr] or [attr=\"value\"]", s, tok)
		}
		c := compound{tag: m[1]}
		for _, p := range pieceRE.FindAllString(m[2], -1) {
			switch p[0] {
			case '#':
				if c.id != "" {
					return nil, fmt.Errorf("%q: two ids in %q", s, tok)
				}
				c.id = p[1:]
			case '.':
				c.classes = append(c.classes, p[1:])
			default:
				a := attrRE.FindStringSubmatch(p)
				c.attrs = append(c.attrs, selAttr{name: a[1], value: a[2], valued: strings.Contains(p, "=")})
			}
		}
		steps = append(steps, selStep{join: join, part: c})
		join = ' '
	}
	if len(steps) == 0 || join == '>' {
		return nil, fmt.Errorf("%q: empty, or ends in '>'", s)
	}
	return steps, nil
}

func (c compound) matches(n *html.Node) bool {
	if n.Type != html.ElementNode || (c.tag != "" && n.Data != c.tag) {
		return false
	}
	attr := func(name string) (string, bool) {
		for _, a := range n.Attr {
			if a.Namespace == "" && a.Key == name {
				return a.Val, true
			}
		}
		return "", false
	}
	if c.id != "" {
		if id, _ := attr("id"); id != c.id {
			return false
		}
	}
	if len(c.classes) > 0 {
		class, _ := attr("class")
		have := strings.Fields(class)
		for _, want := range c.classes {
			if !slices.Contains(have, want) {
				return false
			}
		}
	}
	for _, a := range c.attrs {
		v, ok := attr(a.name)
		if !ok || (a.valued && v != a.value) {
			return false
		}
	}
	return true
}

// matchesAt: n matches steps[:i+1], with steps[i] at n.
func matchesAt(n *html.Node, steps []selStep, i int) bool {
	if !steps[i].part.matches(n) {
		return false
	}
	if i == 0 {
		return true
	}
	for p := n.Parent; p != nil; p = p.Parent {
		if matchesAt(p, steps, i-1) {
			return true
		}
		if steps[i].join == '>' {
			return false
		}
	}
	return false
}

// selects reports whether any element under root matches steps.
func selects(root *html.Node, steps []selStep) bool {
	for n := range root.Descendants() {
		if matchesAt(n, steps, len(steps)-1) {
			return true
		}
	}
	return false
}

// bindingFaults is every selector of selectors the page does not carry, and
// every selector the grammar refuses.
func bindingFaults(page []byte, selectors []string) []string {
	root, err := html.Parse(bytes.NewReader(page))
	if err != nil {
		return []string{"does not parse: " + err.Error()}
	}
	var faults []string
	for _, s := range selectors {
		steps, err := parseSelector(s)
		if err != nil {
			faults = append(faults, err.Error())
			continue
		}
		if !selects(root, steps) {
			faults = append(faults, "carries nothing matching "+s)
		}
	}
	return faults
}

// TestEveryPageCarriesTheDOMItsModulesBind holds each page schema/bindings.json
// names, as fisc export writes it over clientSubset, to its selectors and the
// chrome's, and every other page Go writes to the chrome's.
// Mutation: rename id="year-select" in site/chrome.html.tmpl and every page
// binding it is refused.
func TestEveryPageCarriesTheDOMItsModulesBind(t *testing.T) {
	doc := readBindings(t)
	dir := clientExport(t)
	pages, err := filepath.Glob(filepath.Join(dir, "*.html"))
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) < len(doc.Pages) {
		t.Fatalf("the export wrote %d pages and schema/bindings.json binds %d", len(pages), len(doc.Pages))
	}
	for name := range doc.Pages {
		if !slices.Contains(pages, filepath.Join(dir, name)) {
			t.Errorf("schema/bindings.json binds %s, which fisc export does not write", name)
		}
	}
	for _, path := range pages {
		name := filepath.Base(path)
		selectors := doc.Chrome.Selectors
		if p, ok := doc.Pages[name]; ok {
			selectors = append(slices.Clone(selectors), p.Selectors...)
		}
		for _, f := range bindingFaults([]byte(readFile(t, dir, name)), selectors) {
			t.Errorf("%s %s", name, f)
		}
	}
}

// TestTheBindingsCheckCanFail breaks one binding per row of the page fisc
// export writes, and requires bindingFaults to name it.
func TestTheBindingsCheckCanFail(t *testing.T) {
	doc := readBindings(t)
	page := []byte(readFile(t, clientExport(t), "flows.html"))
	selectors := append(slices.Clone(doc.Chrome.Selectors), doc.Pages["flows.html"].Selectors...)
	if faults := bindingFaults(page, selectors); len(faults) != 0 {
		t.Fatalf("the unbroken page is refused, so no row below means anything: %v", faults)
	}
	for _, c := range []struct {
		name, want string
		from, to   string
		selectors  []string
	}{
		{"an id renamed", "#rail-content div.legend#legend", `id="legend"`, `id="legend2"`, nil},
		{"a class dropped", ".chart-scroll > svg.sankey#chart", `class="sankey"`, `class="other"`, nil},
		{"an attribute's value changed", `#filters input#filter-smallest[type="range"]`,
			`type="range" id="filter-smallest"`, `type="number" id="filter-smallest"`, nil},
		{"an attribute dropped", "p#filter-line[hidden] span#filter-said",
			`id="filter-line" aria-live="polite" hidden`, `id="filter-line" aria-live="polite"`, nil},
		{"a child moved under a wrapper", "#chart-card > div.chart-body#chart-body",
			`<div class="chart-body" id="chart-body">`, `<div><div class="chart-body" id="chart-body">`, nil},
		{"a selector the grammar cannot read", "is not tag", "", "", []string{"a:hover"}},
		{"a dangling '>'", "ends in '>'", "", "", []string{"main >"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			broken := page
			if c.from != "" {
				if !bytes.Contains(page, []byte(c.from)) {
					t.Fatalf("the page carries no %q to break", c.from)
				}
				broken = bytes.Replace(page, []byte(c.from), []byte(c.to), 1)
			}
			sel := selectors
			if c.selectors != nil {
				sel = c.selectors
			}
			faults := strings.Join(bindingFaults(broken, sel), "\n")
			if !strings.Contains(faults, c.want) {
				t.Errorf("not refused for %q; faults:\n%s", c.want, faults)
			}
		})
	}
}
