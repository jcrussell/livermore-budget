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
// page fetches. They are held to what the client relies on, not to Go's bytes:
// the markup outside FISC_CONFIG is the template's, every declaration in the
// config is the one Go makes, and each column validates, resolves and cites
// facts that sum to its figures. A figure may differ from the served site's;
// a declaration may not.

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
	"fund/513": "a capital fund whose label runs past the gutter (site/layout.test.mjs OVER_GUTTER)",
	"fund/551": "a capital fund whose label runs past the gutter (OVER_GUTTER)",
	"fund/552": "a capital fund whose label runs past the gutter (OVER_GUTTER)",
	"fund/600": "pays department/innovation-and-economic-development, whose label runs past " +
		"the gutter only in the department window fund/600 opens (OVER_GUTTER)",
	"fund/610": "with fund/622 and fund/642, the enterprise funds fund-flows carries transfers/in " +
		"into, so the enterprise residual is all-leaving (site/columns.test.mjs)",
	"fund/622": "see fund/610",
	"fund/642": "see fund/610",
	"fund/623": "an enterprise fund whose label runs past the gutter (OVER_GUTTER)",
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

// configFigures are the parts of FISC_CONFIG that are figures of the fixture's
// own facts rather than declarations: the doc page index is checked entry by
// entry instead, and each year's figures are free.
var configFigures = []string{"hero", "figures", "caveats", "counts"}

// pageFixtureFaults is every way fixture fails to be the page the template
// renders with the declarations served carries.
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
	if s, _ := sCfg["exported_by"].(string); s == "" {
		faults = append(faults, "the served config carries no exported_by, so a fixture's stamp could not be told stale")
	}
	sDocs, fDocs := sCfg["docs"], fCfg["docs"]
	if diff := cmp.Diff(declarations(sCfg), declarations(fCfg)); diff != "" {
		faults = append(faults, "the fixture's config declares what Go does not (-go +fixture):\n"+diff)
	}
	faults = append(faults, docFaults(sDocs, fDocs)...)
	return faults
}

// declarations is a decoded config with its figures removed. It copies, so
// the caller's config keeps its docs.
func declarations(cfg map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range cfg {
		if k != "docs" && k != "exported_by" {
			out[k] = v
		}
	}
	years, _ := cfg["years"].([]any)
	stripped := make([]any, 0, len(years))
	for _, y := range years {
		year, _ := y.(map[string]any)
		kept := map[string]any{}
		for k, v := range year {
			if !slices.Contains(configFigures, k) {
				kept[k] = v
			}
		}
		stripped = append(stripped, kept)
	}
	if years != nil {
		out["years"] = stripped
	}
	return out
}

// docFaults holds the fixture's page index to the served one: the same
// documents, and every page the fixture carries carried the same way. The
// fixture may cite fewer pages; it may not cite one Go would not link.
func docFaults(served, fixture any) []string {
	s, _ := served.(map[string]any)
	f, _ := fixture.(map[string]any)
	var faults []string
	if diff := cmp.Diff(slices.Sorted(maps.Keys(s)), slices.Sorted(maps.Keys(f))); diff != "" {
		faults = append(faults, "the fixture's config describes other documents than Go's (-go +fixture):\n"+diff)
	}
	for id, fd := range f {
		sd, _ := s[id].(map[string]any)
		fdoc, _ := fd.(map[string]any)
		for k, v := range fdoc {
			if k == "pages" {
				continue
			}
			if !cmp.Equal(sd[k], v) {
				faults = append(faults, fmt.Sprintf("the fixture's %s carries %s %v and Go's %v", id, k, v, sd[k]))
			}
		}
		sPages, _ := sd["pages"].(map[string]any)
		fPages, _ := fdoc["pages"].(map[string]any)
		for n, p := range fPages {
			if !cmp.Equal(sPages[n], p) {
				faults = append(faults, fmt.Sprintf("the fixture's %s p%s is %v and Go links %v", id, n, p, sPages[n]))
			}
		}
	}
	return faults
}

// columnFixtureFaults is every way one column fixture fails what the client
// relies on, with served the column Go writes at the same path and facts the
// committed store by id.
func columnFixtureFaults(stem string, served, fixture []byte, stamp string, facts map[string]fact.Fact) []string {
	var faults []string
	var raw any
	if err := json.Unmarshal(fixture, &raw); err != nil {
		return []string{stem + " does not decode: " + err.Error()}
	}
	if err := schema.Validate(schema.Column, raw); err != nil {
		faults = append(faults, stem+" does not match "+schema.Column+": "+err.Error())
	}
	var col, goCol export.ColumnDoc
	if err := json.Unmarshal(fixture, &col); err != nil {
		return append(faults, stem+" does not decode as a column: "+err.Error())
	}
	if err := json.Unmarshal(served, &goCol); err != nil {
		return append(faults, "the served "+stem+" does not decode as a column: "+err.Error())
	}
	if want := fmt.Sprintf("fy%d-%s", col.Column.FiscalYear, col.Column.Basis); want != stem {
		faults = append(faults, fmt.Sprintf("%s states the column of %s", stem, want))
	}
	if col.GeneratedBy != stamp {
		faults = append(faults, fmt.Sprintf("%s is stamped %q and the page %q, so the client would refuse it as a stale copy",
			stem, col.GeneratedBy, stamp))
	}
	if diff := cmp.Diff(slices.Sorted(maps.Keys(goCol.Schedules)), slices.Sorted(maps.Keys(col.Schedules))); diff != "" {
		faults = append(faults, stem+" serves other schedules than Go's (-go +fixture):\n"+diff)
	}
	for _, name := range slices.Sorted(maps.Keys(col.Schedules)) {
		goSched, ok := goCol.Schedules[name]
		if !ok {
			continue
		}
		if diff := cmp.Diff(drawnIDs(goCol, goSched), drawnIDs(col, col.Schedules[name])); diff != "" {
			faults = append(faults, fmt.Sprintf("%s %s draws other marks than Go's (-go +fixture):\n%s", stem, name, diff))
		}
	}
	if !cmp.Equal(goCol.Column, col.Column) {
		faults = append(faults, fmt.Sprintf("%s is the column %+v and Go's %+v", stem, col.Column, goCol.Column))
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

// drawnIDs is the sorted node ids one schedule draws, any index past the
// table skipped: the arm that refuses one is below.
func drawnIDs(col export.ColumnDoc, sched export.ColumnSched) []string {
	var ids []string
	for _, n := range sched.Nodes {
		if n.Node >= 0 && n.Node < len(col.Nodes) {
			ids = append(ids, col.Nodes[n.Node].ID)
		}
	}
	sort.Strings(ids)
	return ids
}

// linkFactFaults holds one fixture link to the facts it cites: each in the
// store, of the column's year and basis, and summing to the link's value, the
// draw leg carrying their negation. A link citing nothing must say so.
func linkFactFaults(at string, l export.ColumnLink, src string, column export.ColumnKey, facts map[string]fact.Fact) []string {
	if len(l.FactIDs) == 0 {
		if !l.Derived {
			return []string{at + " cites no fact and is not derived"}
		}
		return nil
	}
	var faults []string
	var sum int64
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
// the check above is run over, and requires the check to name it; the rows
// that change a figure are the arms that must stay green.
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
		{"a wording key dropped", "declares what Go does not", func(t *testing.T) []byte {
			return reconfigured(t, func(cfg map[string]any) { delete(cfg["wording"].(map[string]any), "opened_hint") })
		}},
		{"a cap moved", "declares what Go does not", func(t *testing.T) []byte {
			return reconfigured(t, func(cfg map[string]any) {
				caps := stepOf(cfg, "fund-group")["sankey"].(map[string]any)["caps"].([]any)
				caps[0].(map[string]any)["cap"] = 9.0
			})
		}},
		{"a page linked elsewhere", "Go links", func(t *testing.T) []byte {
			return reconfigured(t, func(cfg map[string]any) { firstPage(cfg)["pdf"] = "https://example.invalid/" })
		}},
		{"a config the schema refuses", "does not match", func(t *testing.T) []byte {
			return reconfigured(t, func(cfg map[string]any) { delete(stepOf(cfg, "fund-group"), "sankey") })
		}},
		{"a hero figure changed", "", func(t *testing.T) []byte {
			return reconfigured(t, func(cfg map[string]any) {
				cfg["years"].([]any)[0].(map[string]any)["hero"].(map[string]any)["value"] = "$1"
			})
		}},
		{"a page the fixture does not cite", "", func(t *testing.T) []byte {
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
			switch {
			case c.want == "" && faults != "":
				t.Errorf("refused a change to a figure, which the fixture is free to make:\n%s", faults)
			case c.want != "" && !strings.Contains(faults, c.want):
				t.Errorf("not refused for %q; faults:\n%s", c.want, faults)
			}
		})
	}

	t.Run("page/a served page with no build stamp", func(t *testing.T) {
		unstamped := regexp.MustCompile(`"exported_by":"[^"]*",?`).ReplaceAll(servedPage, nil)
		if bytes.Equal(unstamped, servedPage) {
			t.Fatal("the served page carries no exported_by to remove, so this row tests nothing")
		}
		faults := strings.Join(pageFixtureFaults(unstamped, fixturePage), "\n")
		if !strings.Contains(faults, "carries no exported_by") {
			t.Errorf("a served page with no stamp was not refused; faults:\n%s", faults)
		}
	})

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
	spine := func(col map[string]any) map[string]any {
		return col["schedules"].(map[string]any)["sankey"].(map[string]any)
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
		{"a mark the subset does not draw", "draws other marks than Go's", func(col map[string]any) {
			nodes := col["schedules"].(map[string]any)["fund-flows"].(map[string]any)["nodes"].([]any)
			col["schedules"].(map[string]any)["fund-flows"].(map[string]any)["nodes"] = nodes[:len(nodes)-1]
		}, ""},
		{"a schedule dropped", "other schedules", func(col map[string]any) {
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

// scheduleOf is one schedule's marks and links by id, for comparing two
// columns whose node tables are indexed differently.
func scheduleOf(col export.ColumnDoc, name string) (ids []string, links []string) {
	sched := col.Schedules[name]
	ids = drawnIDs(col, sched)
	for _, l := range sched.Links {
		links = append(links, fmt.Sprintf("%s -> %s %d %s %v", col.Nodes[l.From].ID, col.Nodes[l.To].ID,
			l.ValueCents, l.Kind, l.FactIDs))
	}
	sort.Strings(links)
	return ids, links
}

// TestTheClientSubsetLeavesTheLicensedSchedulesWhole exports the store and the
// subset and requires every schedule a licence is stated against to be the
// same in both: a fund the subset drops from one of them is a difference the
// client refuses, or worse, one it is licensed to draw.
func TestTheClientSubsetLeavesTheLicensedSchedulesWhole(t *testing.T) {
	full, subset := exportedSite(t), clientExport(t)
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
			fIDs, fLinks := scheduleOf(f, name)
			sIDs, sLinks := scheduleOf(s, name)
			if diff := cmp.Diff(fIDs, sIDs); diff != "" {
				t.Errorf("%s %s draws other marks over the subset (-store +subset):\n%s", stem, name, diff)
			}
			if diff := cmp.Diff(fLinks, sLinks); diff != "" {
				t.Errorf("%s %s draws other links over the subset (-store +subset):\n%s", stem, name, diff)
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
