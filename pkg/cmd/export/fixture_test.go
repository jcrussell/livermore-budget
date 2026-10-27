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

// The client fixtures are the page configs and column files under
// testdata/client/: what `fisc export` writes over the synthetic city
// (buildSynthCity), held decoded, build stamps aside. The markup around a
// config is schema/bindings.json's (TestEveryPageCarriesTheDOMItsModulesBind).

// clientFixtureColumns are the column files the page fixture's years fetch,
// as stems of testdata/client/<stem>.column.json.
var clientFixtureColumns = []string{"fy2026-adopted", "fy2027-adopted"}

// packagedBy is the footer's build stamp, blanked from the markup split returns.
var packagedBy = regexp.MustCompile(`Packaged by [^<\n]*`)

const configOpen, configClose = "window.FISC_CONFIG = ", ";</script>"

// pageConfig is where a page carries its config, the schema the config
// answers to, and the testdata/ file the client tests read it from.
type pageConfig struct{ open, close, schema, fixture string }

var (
	flowsConfig      = pageConfig{configOpen, configClose, schema.Page, "client/flows.config.json"}
	departmentConfig = pageConfig{`<script type="application/json" id="department-config">`, "</script>",
		schema.Department, "client/department.config.json"}
	sidesConfig = pageConfig{`<script type="application/json" id="sides-config">`, "</script>",
		schema.Sides, "client/ledger.config.json"}
)

// split cuts a page into its markup either side of its config, build stamps
// blanked, and the config itself.
func (c pageConfig) split(page []byte) (markup []byte, config []byte, err error) {
	before, rest, ok := bytes.Cut(page, []byte(c.open))
	if !ok {
		return nil, nil, fmt.Errorf("carries no %q", c.open)
	}
	body, after, ok := bytes.Cut(rest, []byte(c.close))
	if !ok {
		return nil, nil, fmt.Errorf("never closes %q", c.open)
	}
	markup = slices.Concat(before, []byte(c.open+"…"+c.close), after)
	return packagedBy.ReplaceAll(markup, []byte("Packaged by")), body, nil
}

// faults is every way the fixture config differs from the one in the page
// served: decoded whole (build stamps aside), and one its schema accepts.
func (c pageConfig) faults(served, fixture []byte) []string {
	var faults []string
	_, sBody, err := c.split(served)
	if err != nil {
		return []string{"the served page " + err.Error()}
	}
	var sCfg, fCfg map[string]any
	if err := json.Unmarshal(sBody, &sCfg); err != nil {
		return append(faults, "the served config does not decode: "+err.Error())
	}
	if err := json.Unmarshal(fixture, &fCfg); err != nil {
		return append(faults, "the fixture's config does not decode: "+err.Error())
	}
	if err := schema.Validate(c.schema, fCfg); err != nil {
		faults = append(faults, "the fixture's config does not match "+c.schema+": "+err.Error())
	}
	delete(sCfg, "exported_by")
	delete(fCfg, "exported_by")
	if diff := cmp.Diff(sCfg, fCfg); diff != "" {
		faults = append(faults, "the fixture's config is not the one Go writes (-go +fixture):\n"+diff)
	}
	return faults
}

// canonical is a config as testdata/client/ carries it: indented, keys
// sorted, so a regenerated fixture's diff is one line per changed value.
func canonical(body []byte) ([]byte, error) {
	var v any
	if err := json.Unmarshal(body, &v); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// holdConfig holds c's fixture to the config in the page Go wrote at served
// under dir; a failing t leaves Go's config in bin/client-fixtures with the
// cp -f that takes it.
func holdConfig(t *testing.T, dir, served string, c pageConfig) {
	t.Helper()
	page := []byte(readFile(t, dir, served))
	t.Cleanup(func() {
		if !t.Failed() {
			return
		}
		_, body, err := c.split(page)
		if err != nil {
			t.Error(err)
			return
		}
		out := filepath.Join(repoRootForTest(t), "bin", "client-fixtures", filepath.Base(c.fixture))
		b, err := canonical(body)
		if err == nil {
			err = os.MkdirAll(filepath.Dir(out), 0o750)
		}
		if err == nil {
			err = os.WriteFile(out, b, 0o600)
		}
		if err != nil {
			t.Error(err)
			return
		}
		t.Logf("Go's config is in %s -- regenerate with\n  cp -f %s testdata/%s\nand read the diff before committing it", out, out, c.fixture)
	})
	for _, f := range c.faults(page, c.read(t)) {
		t.Errorf("testdata/%s: %s", c.fixture, f)
	}
}

// read is c's fixture.
func (c pageConfig) read(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(repoRootForTest(t), "testdata", filepath.FromSlash(c.fixture))) // #nosec G304 -- testdata/.
	if err != nil {
		t.Fatalf("read the fixture: %v", err)
	}
	return b
}

// leaveCopies has a failing t leave each served file under dir (keyed to its
// testdata/ name) in bin/client-fixtures, and print the cp -f that takes it.
func leaveCopies(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	out := filepath.Join(repoRootForTest(t), "bin", "client-fixtures")
	t.Cleanup(func() {
		if !t.Failed() {
			return
		}
		var copies []string
		for _, served := range slices.Sorted(maps.Keys(files)) {
			b, err := os.ReadFile(filepath.Join(dir, served)) // #nosec G304 -- the temp site the test wrote.
			if err == nil {
				err = os.MkdirAll(filepath.Dir(filepath.Join(out, files[served])), 0o750)
			}
			if err == nil {
				err = os.WriteFile(filepath.Join(out, files[served]), b, 0o600)
			}
			if err != nil {
				t.Error(err)
				return
			}
			copies = append(copies, "cp -f "+filepath.Join(out, files[served])+" testdata/"+files[served])
		}
		t.Logf("the export's files are in %s -- regenerate with\n  %s\nand read the diff before committing it",
			out, strings.Join(copies, "\n  "))
	})
}

// TestTheSidesFixtureIsWhatGoWrites holds testdata/client/ledger.config.json
// to the config of the page Go writes over the synthetic city.
func TestTheSidesFixtureIsWhatGoWrites(t *testing.T) {
	holdConfig(t, cityExport(t), "ledger.html", sidesConfig)
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
				faults = append(faults, fmt.Sprintf("%s draws %s more than once", at, id))
			}
			drawn[id] = true
			parentOf[id] = n.Parent
			faults = append(faults, balanceFaults(at+" "+id, n.Balances, id, col.Column, facts)...)
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
				l, src, dst, col.Column, facts)...)
		}
	}

	listed := map[string]bool{}
	for _, g := range col.FundGroups {
		listed[g.ID] = true
	}
	for _, n := range col.Nodes {
		if n.Role == project.RoleFundGroup && !listed[n.ID] {
			faults = append(faults, fmt.Sprintf("%s carries fund-group node %s and its fund_groups omit it", stem, n.ID))
		}
		delete(listed, n.ID)
	}
	for _, id := range slices.Sorted(maps.Keys(listed)) {
		faults = append(faults, fmt.Sprintf("%s lists fund group %s and carries no such node", stem, id))
	}
	return faults
}

// linkFactFaults holds one link to its cited facts: in the store, of the
// column's year and basis, summing to its value (the draw leg negated), on the
// pages its locators name.
func linkFactFaults(at string, l export.ColumnLink, src, dst string, column export.ColumnKey, facts map[string]fact.Fact) []string {
	if len(l.FactIDs) == 0 {
		if !l.Derived {
			return []string{at + " cites no fact and is not derived"}
		}
		return nil
	}
	var faults []string
	var cited []fact.Fact
	missing := false
	pages := map[string]bool{}
	for _, id := range l.FactIDs {
		f, ok := facts[id]
		if !ok {
			faults = append(faults, fmt.Sprintf("%s cites %s, which facts/facts.jsonl does not carry", at, id))
			missing = true
			continue
		}
		if f.FiscalYear != column.FiscalYear || string(f.Basis) != column.Basis {
			faults = append(faults, fmt.Sprintf("%s cites %s of FY%d %s", at, id, f.FiscalYear, f.Basis))
		}
		cited = append(cited, f)
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
	if want, got := slices.Sorted(maps.Keys(pages)), slices.Sorted(maps.Keys(located)); !missing && !slices.Equal(want, got) {
		faults = append(faults, fmt.Sprintf("%s locates %v and its facts are on %v", at, got, want))
	}
	sum := project.LinkCents(src, dst, l.Repeats, cited)
	if !missing && sum != l.ValueCents {
		faults = append(faults, fmt.Sprintf("%s carries %d and its facts come to %d", at, l.ValueCents, sum))
	}
	return faults
}

// balanceFaults holds one node's balances to the facts they cite: each in the
// store, of the column's year and basis, that fund's beginning or ending
// balance, at the value printed and on the page its locators name.
func balanceFaults(at string, b *export.ColumnBalances, node string, column export.ColumnKey, facts map[string]fact.Fact) []string {
	if b == nil {
		return nil
	}
	var faults []string
	for _, slot := range []struct {
		b        *export.ColumnBalance
		category string
	}{{b.Beginning, project.CategoryFundBalanceBeginning}, {b.Ending, project.CategoryFundBalanceEnding}} {
		if slot.b == nil {
			continue
		}
		var locs []project.Source
		if err := json.Unmarshal(slot.b.Locators, &locs); err != nil {
			faults = append(faults, fmt.Sprintf("%s carries locators that do not decode: %v", at, err))
			continue
		}
		f, ok := facts[slot.b.FactID]
		switch {
		case !ok:
			faults = append(faults, fmt.Sprintf("%s cites %s, which facts/facts.jsonl does not carry", at, slot.b.FactID))
		case f.FiscalYear != column.FiscalYear || string(f.Basis) != column.Basis:
			faults = append(faults, fmt.Sprintf("%s cites %s of FY%d %s", at, f.ID, f.FiscalYear, f.Basis))
		case f.Category != slot.category || project.PrefixFund+fact.FundString(f.Fund) != node:
			faults = append(faults, fmt.Sprintf("%s cites %s, fund %s's %s, as its %s", at, f.ID,
				fact.FundString(f.Fund), f.Category, slot.category))
		case f.AmountCents != slot.b.ValueCents:
			faults = append(faults, fmt.Sprintf("%s carries %d and %s prints %d", at, slot.b.ValueCents, f.ID, f.AmountCents))
		case len(locs) != 1 || locs[0].DocID != f.DocID || !slices.Equal(locs[0].Pages, []int{f.Page}):
			faults = append(faults, fmt.Sprintf("%s locates %v and %s is on %s p%d", at, locs, f.ID, f.DocID, f.Page))
		}
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

// pageStamp is the exported_by a config carries.
func pageStamp(t *testing.T, body []byte) string {
	t.Helper()
	var cfg struct {
		ExportedBy string `json:"exported_by"`
	}
	if err := json.Unmarshal(body, &cfg); err != nil {
		t.Fatal(err)
	}
	return cfg.ExportedBy
}

// pageColumns is the column stems a config's years fetch.
func pageColumns(t *testing.T, body []byte) []string {
	t.Helper()
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
// fixture and each column fixture to it.
func TestTheClientFixturesHoldWhatTheClientReads(t *testing.T) {
	dir := cityExport(t)
	root := repoRootForTest(t)
	// On any failure, a missing or unreadable fixture included, the export's
	// files are left where a cp -f takes them.
	holdConfig(t, dir, "flows.html", flowsConfig)
	files := map[string]string{}
	for _, stem := range clientFixtureColumns {
		files[stem+".json"] = "client/" + stem + ".column.json"
	}
	leaveCopies(t, dir, files)
	read := func(path string) []byte {
		t.Helper()
		b, err := os.ReadFile(path) // #nosec G304 -- the temp site and testdata/.
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	fixtureConfig := flowsConfig.read(t)
	var faults []string
	if got := pageColumns(t, fixtureConfig); !slices.Equal(got, clientFixtureColumns) {
		faults = append(faults, fmt.Sprintf("the page fixture's years fetch %v and testdata/ carries %v", got, clientFixtureColumns))
	}
	stamp := pageStamp(t, fixtureConfig)
	facts := cityByID(t)
	for _, stem := range clientFixtureColumns {
		faults = append(faults, columnFixtureFaults(stem,
			read(filepath.Join(dir, stem+".json")),
			read(filepath.Join(root, "testdata", "client", stem+".column.json")),
			stamp, facts)...)
	}
	for _, f := range faults {
		t.Error(f)
	}
}

// TestTheClientFixtureChecksCanFail breaks one thing per row, in the fixture
// the check above is run over, and requires the check to name it.
func TestTheClientFixtureChecksCanFail(t *testing.T) {
	dir := cityExport(t)
	root := repoRootForTest(t)
	servedPage, err := os.ReadFile(filepath.Join(dir, "flows.html"))
	if err != nil {
		t.Fatal(err)
	}
	fixtureConfig := flowsConfig.read(t)
	if faults := flowsConfig.faults(servedPage, fixtureConfig); len(faults) != 0 {
		t.Fatalf("the unbroken page fixture is refused, so no row below means anything: %v", faults)
	}
	reconfigured := func(t *testing.T, edit func(cfg map[string]any)) []byte {
		t.Helper()
		var cfg map[string]any
		if decodeErr := json.Unmarshal(fixtureConfig, &cfg); decodeErr != nil {
			t.Fatal(decodeErr)
		}
		edit(cfg)
		enc, encodeErr := json.Marshal(cfg)
		if encodeErr != nil {
			t.Fatal(encodeErr)
		}
		return enc
	}
	firstPage := func(cfg map[string]any) map[string]any {
		docs := cfg["docs"].(map[string]any)
		for _, id := range slices.Sorted(maps.Keys(docs)) {
			pages := docs[id].(map[string]any)["pages"].(map[string]any)
			for _, n := range slices.Sorted(maps.Keys(pages)) {
				return pages[n].(map[string]any)
			}
		}
		t.Fatal("the config links no page")
		return nil
	}
	for _, c := range []struct {
		name, want string
		page       func(t *testing.T) []byte
	}{
		{"a wording key dropped", "not the one Go writes", func(t *testing.T) []byte {
			return reconfigured(t, func(cfg map[string]any) { delete(cfg["wording"].(map[string]any), "open_into") })
		}},
		{"a cap moved", "not the one Go writes", func(t *testing.T) []byte {
			return reconfigured(t, func(cfg map[string]any) {
				caps := cfg["overview"].(map[string]any)["sankey"].(map[string]any)["split_caps"].([]any)
				caps[0].(map[string]any)["cap"] = 9.0
			})
		}},
		{"a page linked elsewhere", "not the one Go writes", func(t *testing.T) []byte {
			return reconfigured(t, func(cfg map[string]any) { firstPage(cfg)["pdf"] = "https://example.invalid/" })
		}},
		{"a config the schema refuses", "does not match", func(t *testing.T) []byte {
			return reconfigured(t, func(cfg map[string]any) { cfg["overview"].(map[string]any)["form"] = "pie" })
		}},
		{"a hero figure changed", "not the one Go writes", func(t *testing.T) []byte {
			return reconfigured(t, func(cfg map[string]any) {
				cfg["years"].([]any)[0].(map[string]any)["hero"].(map[string]any)["value"] = "$1"
			})
		}},
		{"a page the fixture does not cite", "not the one Go writes", func(t *testing.T) []byte {
			return reconfigured(t, func(cfg map[string]any) {
				docs := cfg["docs"].(map[string]any)
				for _, id := range slices.Sorted(maps.Keys(docs)) {
					pages := docs[id].(map[string]any)["pages"].(map[string]any)
					for _, n := range slices.Sorted(maps.Keys(pages)) {
						delete(pages, n)
						return
					}
				}
			})
		}},
	} {
		t.Run("page/"+c.name, func(t *testing.T) {
			faults := strings.Join(flowsConfig.faults(servedPage, c.page(t)), "\n")
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
	fixture, err := os.ReadFile(filepath.Join(root, "testdata", "client", stem+".column.json"))
	if err != nil {
		t.Fatal(err)
	}
	stamp := pageStamp(t, fixtureConfig)
	facts := cityByID(t)
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
		{"a fact of another year", " of FY", func(col map[string]any) {
			year := int(col["column"].(map[string]any)["fiscal_year"].(float64))
			for _, id := range slices.Sorted(maps.Keys(facts)) {
				if facts[id].FiscalYear != year {
					firstLink(col)["fact_ids"] = []any{id}
					return
				}
			}
			t.Fatal("the store carries no fact of another year")
		}, ""},
		{"a cited link that cites nothing", "cites no fact and is not derived", func(col map[string]any) {
			for _, l := range spine(col)["links"].([]any) {
				if derived, _ := l.(map[string]any)["derived"].(bool); !derived {
					l.(map[string]any)["fact_ids"] = []any{}
					return
				}
			}
			t.Fatal("the spine carries no cited link that is not derived")
		}, ""},
		{"two marks each other's parent", "in a cycle", func(col map[string]any) {
			nodes := spine(col)["nodes"].([]any)
			ids := col["nodes"].([]any)
			a, b := nodes[0].(map[string]any), nodes[1].(map[string]any)
			a["parent"] = ids[int(b["node"].(float64))].(map[string]any)["id"]
			b["parent"] = ids[int(a["node"].(float64))].(map[string]any)["id"]
		}, ""},
		{"the column of another year", "states the column of", func(col map[string]any) {
			col["column"].(map[string]any)["fiscal_year"] = 2025.0
		}, ""},
		{"a fund group with no node", "carries no such node", func(col map[string]any) {
			col["fund_groups"] = append(col["fund_groups"].([]any), map[string]any{"id": "fund-group/ghost", "slot": 0.0})
		}, ""},
		{"a schedule drawing a node past the table", "draws node", func(col map[string]any) {
			spine(col)["nodes"] = append(spine(col)["nodes"].([]any), map[string]any{"node": 99999.0})
		}, ""},
		{"a schedule drawing one node twice", "more than once", func(col map[string]any) {
			nodes := spine(col)["nodes"].([]any)
			spine(col)["nodes"] = append(nodes, map[string]any{"node": nodes[0].(map[string]any)["node"]})
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
