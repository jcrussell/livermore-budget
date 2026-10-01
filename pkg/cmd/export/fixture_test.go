package export

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/jcrussell/livermore-budget/internal/export"
	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/project"
	"github.com/jcrussell/livermore-budget/schema"
)

// The client fixtures are testdata/index.golden.html and the column files the
// page fetches: what `fisc export` writes over clientSubset rather than over
// the whole store, held decoded rather than byte for byte. The page's markup
// and config and each column are the ones Go writes over the subset, build
// stamps aside; each column also validates, resolves, and carries figures equal
// to the facts each link cites, on the pages its locators name.

// clientFixtureColumns are the column files the page fixture's years fetch,
// as stems of testdata/<stem>.column.json.
var clientFixtureColumns = []string{"fy2026-adopted", "fy2027-adopted"}

// clientSubset is the declared subset of the store the client fixtures are
// exported from, keyed by node id: a fund group kept whole, or one fund of a
// group that is not, each with the failure mode it is kept for. Every other
// fund is dropped from fundAxisScopes and from nothing else.
var clientSubset = map[string]string{
	"fund-group/general": "fund/100 and its divisions, the only fund pp.167-170 decompose, " +
		"which most of the client suite opens",
	"fund-group/special-revenue": "the column the tier-3 cap folds hardest: site/layout.test.mjs " +
		"draws all of it out and requires no two labels to stack, which only its real count tests; " +
		"fund/240 must fold into its tail; fund/207 and fund/290 print in FY2026 only, so the years' tails differ",
	"fund-group/capital": "the second column the tier-3 cap folds: site/fold.test.mjs requires the cap " +
		"to remove sub-pixel ribbons its whole column draws, which fewer funds than the cap never draw; " +
		"fund/513, fund/551 and fund/552 run past the gutter (site/layout.test.mjs OVER_GUTTER)",
	"fund/600": "pays department/innovation-and-economic-development, whose label runs past " +
		"the gutter only in the department window fund/600 opens (OVER_GUTTER)",
	"fund/610": "with fund/622 and fund/642, the enterprise funds fund-flows carries transfers/in " +
		"into, so the enterprise residual is all-leaving (site/columns.test.mjs)",
	"fund/622": "see fund/610",
	"fund/642": "see fund/610",
	"fund/623": "an enterprise fund whose label runs past the gutter (site/layout.test.mjs OVER_GUTTER)",
	"fund/730": "an internal-service fund whose label runs past the gutter (OVER_GUTTER)",
}

// fundAxisScopes are the scopes a fund is dropped from. Every other scope
// stays whole: the spine and the department pages are what a step's gap
// licence is stated against, in cents over the whole column, so a fund
// dropped from either would be a difference the client refuses.
var fundAxisScopes = []string{"revenue-by-fund", "expenditure-by-fund", "department-funding-sources"}

// clientFacts is the store less every fund-axis fact of a fund clientSubset
// keeps neither whole nor by its group.
func clientFacts(facts []fact.Fact) []fact.Fact {
	var out []fact.Fact
	for _, f := range facts {
		if f.Fund == nil || !slices.Contains(fundAxisScopes, f.Scope) {
			out = append(out, f)
			continue
		}
		_, group := clientSubset["fund-group/"+f.FundGroup]
		_, fund := clientSubset[fmt.Sprintf("fund/%d", *f.Fund)]
		if !group && !fund {
			continue
		}
		out = append(out, f)
	}
	return out
}

// buildClientSubset is buildAll over clientFacts.
func buildClientSubset(repoRoot string) (result, error) {
	_, facts, err := readFactStore(repoRoot)
	if err != nil {
		return result{}, err
	}
	subset := clientFacts(facts)
	var raw bytes.Buffer
	if err := fact.Write(&raw, subset); err != nil {
		return result{}, err
	}
	return buildFrom(repoRoot, raw.Bytes(), subset)
}

// clientExport writes the site the client fixtures are taken from.
func clientExport(t *testing.T) string {
	t.Helper()
	opts, _, _, _ := testOptions(t)
	root := repoRootForTest(t)
	opts.RepoRoot = func() (string, error) { return root, nil }
	opts.Build = buildClientSubset
	if err := exportRun(opts); err != nil {
		t.Fatalf("exportRun over the client subset: %v", err)
	}
	return opts.OutputDir
}

// packagedBy is the footer's build stamp, blanked before markup is compared.
var packagedBy = regexp.MustCompile(`Packaged by [^<\n]*`)

const configOpen, configClose = "window.FISC_CONFIG = ", ";</script>"

// splitPage cuts a page into its markup either side of FISC_CONFIG and the
// config itself.
func splitPage(page []byte) (markup []byte, config []byte, err error) {
	before, rest, ok := bytes.Cut(page, []byte(configOpen))
	if !ok {
		return nil, nil, fmt.Errorf("carries no %q", configOpen)
	}
	body, after, ok := bytes.Cut(rest, []byte(configClose))
	if !ok {
		return nil, nil, fmt.Errorf("never closes FISC_CONFIG")
	}
	markup = slices.Concat(before, []byte(configOpen+"…"+configClose), after)
	return packagedBy.ReplaceAll(markup, []byte("Packaged by")), body, nil
}

// pageFixtureFaults is every way fixture fails to be the page served is: the
// same markup and the same decoded config, build stamps aside. The config is
// compared whole because the markup renders its figures -- the hero, the
// caveats, the page links -- so a config free to differ would describe a page
// its own markup contradicts.
func pageFixtureFaults(served, fixture []byte) []string {
	var faults []string
	sMarkup, sBody, err := splitPage(served)
	if err != nil {
		return []string{"the served page " + err.Error()}
	}
	fMarkup, fBody, err := splitPage(fixture)
	if err != nil {
		return []string{"the fixture " + err.Error()}
	}
	if !bytes.Equal(sMarkup, fMarkup) {
		faults = append(faults, "the markup outside FISC_CONFIG is not the template's:\n"+
			cmp.Diff(strings.Split(string(sMarkup), "\n"), strings.Split(string(fMarkup), "\n")))
	}
	var sCfg, fCfg map[string]any
	if err := json.Unmarshal(sBody, &sCfg); err != nil {
		return append(faults, "the served config does not decode: "+err.Error())
	}
	if err := json.Unmarshal(fBody, &fCfg); err != nil {
		return append(faults, "the fixture's config does not decode: "+err.Error())
	}
	if err := schema.Validate(schema.Page, fCfg); err != nil {
		faults = append(faults, "the fixture's config does not match "+schema.Page+": "+err.Error())
	}
	delete(sCfg, "exported_by")
	delete(fCfg, "exported_by")
	if diff := cmp.Diff(sCfg, fCfg); diff != "" {
		faults = append(faults, "the fixture's config is not the one Go writes (-go +fixture):\n"+diff)
	}
	return faults
}

// columnFixtureFaults is every way one column fixture fails to be served, the
// column Go writes at the same path, or fails what the client relies on of
// any column, with facts the committed store by id.
func columnFixtureFaults(stem string, served, fixture []byte, stamp string, facts map[string]fact.Fact) []string {
	var faults []string
	var raw, goRaw map[string]any
	if err := json.Unmarshal(fixture, &raw); err != nil {
		return []string{stem + " does not decode: " + err.Error()}
	}
	if err := schema.Validate(schema.Column, raw); err != nil {
		faults = append(faults, stem+" does not match "+schema.Column+": "+err.Error())
	}
	if err := json.Unmarshal(served, &goRaw); err != nil {
		return append(faults, "the served "+stem+" does not decode: "+err.Error())
	}
	unstamped := func(m map[string]any) map[string]any {
		out := maps.Clone(m)
		delete(out, "generated_by")
		return out
	}
	if diff := cmp.Diff(unstamped(goRaw), unstamped(raw)); diff != "" {
		faults = append(faults, stem+" is not the column Go writes over the subset (-go +fixture):\n"+diff)
	}
	var col export.ColumnDoc
	if err := json.Unmarshal(fixture, &col); err != nil {
		return append(faults, stem+" does not decode as a column: "+err.Error())
	}
	if want := fmt.Sprintf("fy%d-%s", col.Column.FiscalYear, col.Column.Basis); want != stem {
		faults = append(faults, fmt.Sprintf("%s states the column of %s", stem, want))
	}
	if col.GeneratedBy != stamp {
		faults = append(faults, fmt.Sprintf("%s is stamped %q and the page %q, so the client would refuse it as a stale copy",
			stem, col.GeneratedBy, stamp))
	}
	inRange := func(i int) bool { return i >= 0 && i < len(col.Nodes) }
	for name, sched := range col.Schedules {
		at := stem + " " + name
		drawn := map[string]bool{}
		parentOf := map[string]string{}
		for _, n := range sched.Nodes {
			if !inRange(n.Node) {
				faults = append(faults, fmt.Sprintf("%s draws node %d of %d", at, n.Node, len(col.Nodes)))
				continue
			}
			id := col.Nodes[n.Node].ID
			if drawn[id] {
				faults = append(faults, fmt.Sprintf("%s draws %s twice", at, id))
			}
			drawn[id] = true
			parentOf[id] = n.Parent
		}
		for id, parent := range parentOf {
			for seen := 0; parent != ""; seen++ {
				if !drawn[parent] {
					faults = append(faults, fmt.Sprintf("%s hangs %s under %s, which it does not draw", at, id, parent))
					break
				}
				if seen > len(parentOf) {
					faults = append(faults, fmt.Sprintf("%s hangs %s in a cycle", at, id))
					break
				}
				parent = parentOf[parent]
			}
		}
		for i, l := range sched.Links {
			if !inRange(l.From) || !inRange(l.To) {
				faults = append(faults, fmt.Sprintf("%s link %d joins %d -> %d of %d nodes", at, i, l.From, l.To, len(col.Nodes)))
				continue
			}
			src, dst := col.Nodes[l.From].ID, col.Nodes[l.To].ID
			if !drawn[src] || !drawn[dst] {
				faults = append(faults, fmt.Sprintf("%s link %d joins %s -> %s, which it does not both draw", at, i, src, dst))
			}
			faults = append(faults, linkFactFaults(fmt.Sprintf("%s link %d (%s -> %s)", at, i, src, dst),
				l, src, col.Column, facts)...)
		}
	}

	listed := map[string]bool{}
	for _, g := range col.FundGroups {
		listed[g.ID] = true
	}
	for _, n := range col.Nodes {
		if n.Role == "fund_group" && !listed[n.ID] {
			faults = append(faults, fmt.Sprintf("%s carries fund-group node %s and its fund_groups omit it", stem, n.ID))
		}
		delete(listed, n.ID)
	}
	for _, id := range slices.Sorted(maps.Keys(listed)) {
		faults = append(faults, fmt.Sprintf("%s lists fund group %s and carries no such node", stem, id))
	}
	return faults
}

// scheduleOf is one schedule's marks and links, each spelled by node id so
// two columns whose tables are indexed differently compare: every field the
// client folds or labels by, and the figures as well where figures is set.
// An index past the table is skipped; the arm that refuses one is
// columnFixtureFaults'.
func scheduleOf(col export.ColumnDoc, sched export.ColumnSched, figures bool) (nodes, links []string) {
	id := func(i int) string {
		if i < 0 || i >= len(col.Nodes) {
			return fmt.Sprintf("#%d", i)
		}
		return col.Nodes[i].ID
	}
	for _, n := range sched.Nodes {
		if n.Node < 0 || n.Node >= len(col.Nodes) {
			continue
		}
		c := col.Nodes[n.Node]
		nodes = append(nodes, fmt.Sprintf("%s tier=%d role=%q label=%q parent=%q derived=%t constraint=%q rationale=%q source=%q",
			c.ID, c.Tier, c.Role, c.Label, n.Parent, c.Derived, c.ConstraintTier, c.Rationale, c.SourceNote))
	}
	for _, l := range sched.Links {
		link := fmt.Sprintf("%s -> %s kind=%s transfer=%q derived=%t partition=%t contra=%q",
			id(l.From), id(l.To), l.Kind, l.TransferID, l.Derived, l.Partition, l.Contra)
		if figures {
			link += fmt.Sprintf(" cents=%d facts=%v locators=%s", l.ValueCents, l.FactIDs, l.Locators)
		}
		links = append(links, link)
	}
	sort.Strings(nodes)
	sort.Strings(links)
	return nodes, links
}

// linkFactFaults holds one link to the facts it cites: each in the store, of
// the column's year and basis, summing to the link's value -- the draw leg
// carrying their negation -- and on exactly the pages its locators name. A
// link citing nothing must say so.
func linkFactFaults(at string, l export.ColumnLink, src string, column export.ColumnKey, facts map[string]fact.Fact) []string {
	if len(l.FactIDs) == 0 {
		if !l.Derived {
			return []string{at + " cites no fact and is not derived"}
		}
		return nil
	}
	var faults []string
	var sum int64
	pages := map[string]bool{}
	for _, id := range l.FactIDs {
		f, ok := facts[id]
		if !ok {
			faults = append(faults, fmt.Sprintf("%s cites %s, which facts/facts.jsonl does not carry", at, id))
			continue
		}
		if f.FiscalYear != column.FiscalYear || string(f.Basis) != column.Basis {
			faults = append(faults, fmt.Sprintf("%s cites %s of FY%d %s", at, id, f.FiscalYear, f.Basis))
		}
		sum += f.AmountCents
		pages[fmt.Sprintf("%s p%d", f.DocID, f.Page)] = true
	}
	var locs []struct {
		DocID string `json:"doc_id"`
		Pages []int  `json:"pages"`
	}
	if err := json.Unmarshal(l.Locators, &locs); err != nil {
		faults = append(faults, fmt.Sprintf("%s carries locators that do not decode: %v", at, err))
	}
	located := map[string]bool{}
	for _, loc := range locs {
		for _, pg := range loc.Pages {
			located[fmt.Sprintf("%s p%d", loc.DocID, pg)] = true
		}
	}
	if want, got := slices.Sorted(maps.Keys(pages)), slices.Sorted(maps.Keys(located)); len(faults) == 0 && !slices.Equal(want, got) {
		faults = append(faults, fmt.Sprintf("%s locates %v and its facts are on %v", at, got, want))
	}
	if src == project.NodeFundBalanceDraw {
		sum = -sum
	}
	if len(faults) == 0 && sum != l.ValueCents {
		faults = append(faults, fmt.Sprintf("%s carries %d and its facts come to %d", at, l.ValueCents, sum))
	}
	return faults
}

// storeByID is the committed store keyed by fact id.
func storeByID(t *testing.T) map[string]fact.Fact {
	t.Helper()
	_, facts, err := readFactStore(repoRootForTest(t))
	if err != nil {
		t.Fatal(err)
	}
	out := make(map[string]fact.Fact, len(facts))
	for _, f := range facts {
		out[f.ID] = f
	}
	return out
}

// pageStamp is the exported_by a page's config carries.
func pageStamp(t *testing.T, page []byte) string {
	t.Helper()
	_, body, err := splitPage(page)
	if err != nil {
		t.Fatalf("page %v", err)
	}
	var cfg struct {
		ExportedBy string `json:"exported_by"`
	}
	if err := json.Unmarshal(body, &cfg); err != nil {
		t.Fatal(err)
	}
	return cfg.ExportedBy
}

// pageColumns is the column stems a page's years fetch.
func pageColumns(t *testing.T, page []byte) []string {
	t.Helper()
	_, body, err := splitPage(page)
	if err != nil {
		t.Fatalf("page %v", err)
	}
	var cfg struct {
		Years []struct {
			Path string `json:"path"`
		} `json:"years"`
	}
	if err := json.Unmarshal(body, &cfg); err != nil {
		t.Fatal(err)
	}
	var stems []string
	for _, y := range cfg.Years {
		stems = append(stems, strings.TrimSuffix(y.Path, ".json"))
	}
	sort.Strings(stems)
	return stems
}

// TestTheClientFixturesHoldWhatTheClientReads exports once and holds the page
// fixture and each column fixture to it. On a fault it writes the export's
// files to bin/client-fixtures/ and prints the cp -f that would take them.
func TestTheClientFixturesHoldWhatTheClientReads(t *testing.T) {
	dir := clientExport(t)
	root := repoRootForTest(t)
	read := func(path string) []byte {
		t.Helper()
		b, err := os.ReadFile(path) // #nosec G304 -- the temp site and testdata/.
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	servedPage := read(filepath.Join(dir, "index.html"))
	fixturePage := read(filepath.Join(root, "testdata", "index.golden.html"))
	faults := pageFixtureFaults(servedPage, fixturePage)

	if got := pageColumns(t, fixturePage); !slices.Equal(got, clientFixtureColumns) {
		faults = append(faults, fmt.Sprintf("the page fixture's years fetch %v and testdata/ carries %v", got, clientFixtureColumns))
	}
	stamp := pageStamp(t, fixturePage)
	facts := storeByID(t)
	for _, stem := range clientFixtureColumns {
		faults = append(faults, columnFixtureFaults(stem,
			read(filepath.Join(dir, stem+".json")),
			read(filepath.Join(root, "testdata", stem+".column.json")),
			stamp, facts)...)
	}
	if len(faults) == 0 {
		return
	}
	out := filepath.Join(root, "bin", "client-fixtures")
	if err := os.MkdirAll(out, 0o750); err != nil {
		t.Fatal(err)
	}
	copies := []string{"cp -f " + filepath.Join(out, "index.golden.html") + " testdata/index.golden.html"}
	if err := os.WriteFile(filepath.Join(out, "index.golden.html"), servedPage, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, stem := range clientFixtureColumns {
		to := filepath.Join(out, stem+".column.json")
		if err := os.WriteFile(to, read(filepath.Join(dir, stem+".json")), 0o600); err != nil {
			t.Fatal(err)
		}
		copies = append(copies, "cp -f "+to+" testdata/"+stem+".column.json")
	}
	for _, f := range faults {
		t.Error(f)
	}
	t.Fatalf("the client fixtures are not what the client relies on; the export's files are in %s -- "+
		"regenerate with\n  %s\nand read the diff before committing it", out, strings.Join(copies, "\n  "))
}

// TestTheClientFixtureChecksCanFail breaks one thing per row, in the fixture
// the check above is run over, and requires the check to name it.
func TestTheClientFixtureChecksCanFail(t *testing.T) {
	dir := clientExport(t)
	root := repoRootForTest(t)
	servedPage, err := os.ReadFile(filepath.Join(dir, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	fixturePage, err := os.ReadFile(filepath.Join(root, "testdata", "index.golden.html"))
	if err != nil {
		t.Fatal(err)
	}
	if faults := pageFixtureFaults(servedPage, fixturePage); len(faults) != 0 {
		t.Fatalf("the unbroken page fixture is refused, so no row below means anything: %v", faults)
	}
	reconfigured := func(t *testing.T, edit func(cfg map[string]any)) []byte {
		t.Helper()
		before, rest, _ := bytes.Cut(fixturePage, []byte(configOpen))
		body, after, _ := bytes.Cut(rest, []byte(configClose))
		var cfg map[string]any
		if decodeErr := json.Unmarshal(body, &cfg); decodeErr != nil {
			t.Fatal(decodeErr)
		}
		edit(cfg)
		enc, encodeErr := json.Marshal(cfg)
		if encodeErr != nil {
			t.Fatal(encodeErr)
		}
		return slices.Concat(before, []byte(configOpen), enc, []byte(configClose), after)
	}
	stepOf := func(cfg map[string]any, key string) map[string]any {
		for _, s := range cfg["steps"].([]any) {
			if s.(map[string]any)["key"] == key {
				return s.(map[string]any)
			}
		}
		t.Fatalf("the config declares no %s step", key)
		return nil
	}
	firstPage := func(cfg map[string]any) map[string]any {
		for _, d := range cfg["docs"].(map[string]any) {
			for _, p := range d.(map[string]any)["pages"].(map[string]any) {
				return p.(map[string]any)
			}
		}
		t.Fatal("the config links no page")
		return nil
	}
	for _, c := range []struct {
		name, want string
		page       func(t *testing.T) []byte
	}{
		{"one byte of markup", "markup outside FISC_CONFIG", func(*testing.T) []byte {
			return bytes.Replace(fixturePage, []byte(`id="year-toggle"`), []byte(`id="year-toggl"`), 1)
		}},
		{"a wording key dropped", "not the one Go writes", func(t *testing.T) []byte {
			return reconfigured(t, func(cfg map[string]any) { delete(cfg["wording"].(map[string]any), "opened_hint") })
		}},
		{"a cap moved", "not the one Go writes", func(t *testing.T) []byte {
			return reconfigured(t, func(cfg map[string]any) {
				caps := stepOf(cfg, "fund-group")["sankey"].(map[string]any)["caps"].([]any)
				caps[0].(map[string]any)["cap"] = 9.0
			})
		}},
		{"a page linked elsewhere", "not the one Go writes", func(t *testing.T) []byte {
			return reconfigured(t, func(cfg map[string]any) { firstPage(cfg)["pdf"] = "https://example.invalid/" })
		}},
		{"a config the schema refuses", "does not match", func(t *testing.T) []byte {
			return reconfigured(t, func(cfg map[string]any) { delete(stepOf(cfg, "fund-group"), "sankey") })
		}},
		{"a hero figure changed", "not the one Go writes", func(t *testing.T) []byte {
			return reconfigured(t, func(cfg map[string]any) {
				cfg["years"].([]any)[0].(map[string]any)["hero"].(map[string]any)["value"] = "$1"
			})
		}},
		{"a page the fixture does not cite", "not the one Go writes", func(t *testing.T) []byte {
			return reconfigured(t, func(cfg map[string]any) {
				for _, d := range cfg["docs"].(map[string]any) {
					pages := d.(map[string]any)["pages"].(map[string]any)
					for n := range pages {
						delete(pages, n)
						return
					}
				}
			})
		}},
	} {
		t.Run("page/"+c.name, func(t *testing.T) {
			faults := strings.Join(pageFixtureFaults(servedPage, c.page(t)), "\n")
			if !strings.Contains(faults, c.want) {
				t.Errorf("not refused for %q; faults:\n%s", c.want, faults)
			}
		})
	}

	const stem = "fy2026-adopted"
	served, err := os.ReadFile(filepath.Join(dir, stem+".json"))
	if err != nil {
		t.Fatal(err)
	}
	fixture, err := os.ReadFile(filepath.Join(root, "testdata", stem+".column.json"))
	if err != nil {
		t.Fatal(err)
	}
	stamp := pageStamp(t, fixturePage)
	facts := storeByID(t)
	if faults := columnFixtureFaults(stem, served, fixture, stamp, facts); len(faults) != 0 {
		t.Fatalf("the unbroken %s fixture is refused, so no row below means anything: %v", stem, faults)
	}
	rewritten := func(t *testing.T, edit func(col map[string]any)) []byte {
		t.Helper()
		var col map[string]any
		if decodeErr := json.Unmarshal(fixture, &col); decodeErr != nil {
			t.Fatal(decodeErr)
		}
		edit(col)
		enc, encodeErr := json.Marshal(col)
		if encodeErr != nil {
			t.Fatal(encodeErr)
		}
		return enc
	}
	sched := func(col map[string]any, name string) map[string]any {
		return col["schedules"].(map[string]any)[name].(map[string]any)
	}
	spine := func(col map[string]any) map[string]any { return sched(col, "sankey") }
	node := func(col map[string]any, id string) map[string]any {
		for _, n := range col["nodes"].([]any) {
			if n.(map[string]any)["id"] == id {
				return n.(map[string]any)
			}
		}
		t.Fatalf("the column carries no %s", id)
		return nil
	}
	firstLink := func(col map[string]any) map[string]any {
		return spine(col)["links"].([]any)[0].(map[string]any)
	}
	for _, c := range []struct {
		name, want string
		edit       func(col map[string]any)
		stamp      string
	}{
		{"a link past the node table", "of 0 nodes", func(col map[string]any) {
			col["nodes"] = []any{}
		}, ""},
		{"a link the schedule does not draw", "which it does not both draw", func(col map[string]any) {
			spine(col)["nodes"] = spine(col)["nodes"].([]any)[1:]
		}, ""},
		{"a parent the schedule does not draw", "which it does not draw", func(col map[string]any) {
			spine(col)["nodes"].([]any)[0].(map[string]any)["parent"] = "nowhere"
		}, ""},
		{"a figure its facts do not sum to", "its facts come to", func(col map[string]any) {
			firstLink(col)["value_cents"] = firstLink(col)["value_cents"].(float64) + 1
		}, ""},
		{"a fact the store does not carry", "does not carry", func(col map[string]any) {
			firstLink(col)["fact_ids"] = []any{"no-such-fact"}
		}, ""},
		{"a fund group the order omits", "fund_groups omit it", func(col map[string]any) {
			col["fund_groups"] = col["fund_groups"].([]any)[1:]
		}, ""},
		{"a mark the subset does not draw", "not the column Go writes", func(col map[string]any) {
			nodes := col["schedules"].(map[string]any)["fund-flows"].(map[string]any)["nodes"].([]any)
			col["schedules"].(map[string]any)["fund-flows"].(map[string]any)["nodes"] = nodes[:len(nodes)-1]
		}, ""},
		{"a mark at another tier", "not the column Go writes", func(col map[string]any) {
			node(col, "fund/100")["tier"] = 4.0
		}, ""},
		{"a mark with another role", "not the column Go writes", func(col map[string]any) {
			node(col, "fund/100")["role"] = "fund"
		}, ""},
		{"a mark with another label", "not the column Go writes", func(col map[string]any) {
			node(col, "fund/100")["label"] = "Bogus"
		}, ""},
		{"a mark under another parent", "not the column Go writes", func(col map[string]any) {
			for _, n := range sched(col, "fund-flows")["nodes"].([]any) {
				if parent, _ := n.(map[string]any)["parent"].(string); parent == "fund-group/general" {
					n.(map[string]any)["parent"] = "fund-group/special-revenue"
					return
				}
			}
		}, ""},
		{"a link dropped", "not the column Go writes", func(col map[string]any) {
			spine(col)["links"] = spine(col)["links"].([]any)[1:]
		}, ""},
		{"a cited link passed off as derived", "not the column Go writes", func(col map[string]any) {
			for _, l := range spine(col)["links"].([]any) {
				if derived, _ := l.(map[string]any)["derived"].(bool); !derived {
					l.(map[string]any)["fact_ids"] = []any{}
					l.(map[string]any)["derived"] = true
					return
				}
			}
			t.Fatal("the spine carries no cited link that is not derived")
		}, ""},
		{"a link located on a page its facts are not on", "locates", func(col map[string]any) {
			firstLink(col)["locators"].([]any)[0].(map[string]any)["pages"] = []any{999.0}
		}, ""},
		{"two links' figures swapped, each still its facts' sum", "not the column Go writes", func(col map[string]any) {
			links := spine(col)["links"].([]any)
			a, b := links[0].(map[string]any), links[1].(map[string]any)
			for _, k := range []string{"value_cents", "fact_ids", "locators"} {
				a[k], b[k] = b[k], a[k]
			}
		}, ""},
		{"a fund group in another slot", "not the column Go writes", func(col map[string]any) {
			g := col["fund_groups"].([]any)[0].(map[string]any)
			g["slot"] = g["slot"].(float64) + 1
		}, ""},
		{"a schedule's caveats rewritten", "not the column Go writes", func(col map[string]any) {
			spine(col)["caveats"] = []any{}
		}, ""},
		{"a schedule dropped", "not the column Go writes", func(col map[string]any) {
			delete(col["schedules"].(map[string]any), "transfers-out")
		}, ""},
		{"a column the schema refuses", "does not match", func(col map[string]any) {
			firstLink(col)["value_cents"] = 1.5
		}, ""},
		{"a stamp the page does not carry", "stale copy", func(map[string]any) {}, "fisc elsewhere"},
	} {
		t.Run("column/"+c.name, func(t *testing.T) {
			s := stamp
			if c.stamp != "" {
				s = c.stamp
			}
			faults := strings.Join(columnFixtureFaults(stem, served, rewritten(t, c.edit), s, facts), "\n")
			if !strings.Contains(faults, c.want) {
				t.Errorf("not refused for %q; faults:\n%s", c.want, faults)
			}
		})
	}
}

// wholeSchedules are the schedules a step's gap licence or residual is stated
// against, which the subset must leave figure for figure as the store has them.
var wholeSchedules = []string{"sankey", "department-spending", "transfers-by-fund", "transfers-out"}

// TestTheClientSubsetLeavesTheLicensedSchedulesWhole exports the store and the
// subset and requires every schedule a licence is stated against to be the
// same in both: a fund the subset drops from one of them is a difference the
// client refuses, or worse, one it is licensed to draw. The page config is the
// same in both too, but for the pages it links.
func TestTheClientSubsetLeavesTheLicensedSchedulesWhole(t *testing.T) {
	full, subset := exportedSite(t), clientExport(t)
	config := func(dir string) map[string]any {
		t.Helper()
		page, err := os.ReadFile(filepath.Join(dir, "index.html")) // #nosec G304 -- a temp dir this test wrote.
		if err != nil {
			t.Fatal(err)
		}
		_, body, err := splitPage(page)
		if err != nil {
			t.Fatalf("page %v", err)
		}
		var cfg map[string]any
		if err := json.Unmarshal(body, &cfg); err != nil {
			t.Fatal(err)
		}
		delete(cfg, "docs")
		return cfg
	}
	if diff := cmp.Diff(config(full), config(subset)); diff != "" {
		t.Errorf("the page config differs over the subset beyond the pages it links (-store +subset):\n%s", diff)
	}
	for _, stem := range clientFixtureColumns {
		read := func(dir string) export.ColumnDoc {
			t.Helper()
			raw, err := os.ReadFile(filepath.Join(dir, stem+".json")) // #nosec G304 -- a temp dir this test wrote.
			if err != nil {
				t.Fatal(err)
			}
			var col export.ColumnDoc
			if err := json.Unmarshal(raw, &col); err != nil {
				t.Fatal(err)
			}
			return col
		}
		f, s := read(full), read(subset)
		for _, name := range wholeSchedules {
			if _, ok := f.Schedules[name]; !ok {
				t.Fatalf("%s serves no %s, so this test holds nothing of it", stem, name)
			}
			fIDs, fLinks := scheduleOf(f, f.Schedules[name], true)
			sIDs, sLinks := scheduleOf(s, s.Schedules[name], true)
			if diff := cmp.Diff(fIDs, sIDs); diff != "" {
				t.Errorf("%s %s draws other marks over the subset (-store +subset):\n%s", stem, name, diff)
			}
			if diff := cmp.Diff(fLinks, sLinks); diff != "" {
				t.Errorf("%s %s draws other links over the subset (-store +subset):\n%s", stem, name, diff)
			}
			fs, ss := f.Schedules[name], s.Schedules[name]
			fs.Nodes, fs.Links, ss.Nodes, ss.Links = nil, nil, nil, nil
			if diff := cmp.Diff(fs, ss); diff != "" {
				t.Errorf("%s %s states other scopes, counts, caveats or sources over the subset (-store +subset):\n%s", stem, name, diff)
			}
		}
	}
}

// TestTheClientSubsetIsDeclaredAgainstTheStore holds each member of
// clientSubset to the store: a reason, a fund group or fund the fund-axis
// scopes carry, and a fund only where its group is not kept whole. The cut is
// not vacuous: it drops facts and keeps facts.
func TestTheClientSubsetIsDeclaredAgainstTheStore(t *testing.T) {
	_, facts, err := readFactStore(repoRootForTest(t))
	if err != nil {
		t.Fatal(err)
	}
	groupOf := map[string]string{}
	groups := map[string]bool{}
	for _, f := range facts {
		if f.Fund != nil && slices.Contains(fundAxisScopes, f.Scope) {
			groupOf[fmt.Sprintf("fund/%d", *f.Fund)] = f.FundGroup
			groups["fund-group/"+f.FundGroup] = true
		}
	}
	for _, id := range slices.Sorted(maps.Keys(clientSubset)) {
		if strings.TrimSpace(clientSubset[id]) == "" {
			t.Errorf("%s is kept for no stated reason", id)
		}
		if strings.HasPrefix(id, "fund-group/") {
			if !groups[id] {
				t.Errorf("%s is kept whole and the fund-axis scopes carry no such group", id)
			}
			continue
		}
		group, ok := groupOf[id]
		switch {
		case !ok:
			t.Errorf("%s is kept and the fund-axis scopes carry no such fund", id)
		case clientSubset["fund-group/"+group] != "":
			t.Errorf("%s is kept by name and its group fund-group/%s is kept whole, so the entry says nothing", id, group)
		}
	}
	kept := len(clientFacts(facts))
	if kept == 0 || kept == len(facts) {
		t.Fatalf("the subset keeps %d of %d facts, so it is not a subset of anything", kept, len(facts))
	}
	t.Logf("the client subset keeps %d of the store's %d facts", kept, len(facts))
}
