package export

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/export"
	"github.com/jcrussell/livermore-budget/internal/registry"
)

// committedViews is data/views.yaml as committed, and the registry its rows
// resolve in.
func committedViews(t *testing.T) ([]byte, *registry.Registry) {
	t.Helper()
	root := repoRootForTest(t)
	b, err := os.ReadFile(filepath.Join(root, "data", viewsFile))
	if err != nil {
		t.Fatalf("read %s: %v", viewsFile, err)
	}
	reg, err := loadRegistry(root)
	if err != nil {
		t.Fatal(err)
	}
	return b, reg
}

// TestLoadViewsRefusesWhatItCannotResolve holds the loader to the file's own
// faults: a key it does not know, and a name that resolves against nothing.
// Each case is the committed file with one edit, so the edit is the only
// thing wrong with it, and the refusal must name the entry it is about: the
// fix is an edit to data/views.yaml, and the reader wants to know where.
func TestLoadViewsRefusesWhatItCannotResolve(t *testing.T) {
	b, reg := committedViews(t)
	if _, err := decodeViews(b, reg); err != nil {
		t.Fatalf("the committed %s does not load, so no case below is about its edit: %v", viewsFile, err)
	}
	for _, tc := range []struct {
		name, old, edit string
		want            []string
	}{
		{"a step's projection", "projection: fund-flows\n", "projection: fund-flow\n",
			[]string{`step "fund-group"`, "projection", `"fund-flow" is not one of`}},
		{"a step's role", "role: general_fund\n", "role: general-fund\n",
			[]string{`step "fund"`, "role", `"general-fund" is not one of`}},
		{"a residual derivation", "residual: fund-flows\n", "residual: fund-flow\n",
			[]string{`step "fund-group"`, "residual", `"fund-flow" is not one of fund-flows`}},
		{"a gaps derivation", "gaps: department-spending\n", "gaps: spending\n",
			[]string{`step "object-category"`, "gaps", `"spending" is not one of department-spending`}},
		{"a view's projection", "projection: revenue-trends\n", "projection: revenue-trend\n",
			[]string{`view "trends.html"`, "projection", `"revenue-trend" is not one of`}},
		{"a row's category", "fund-balance/excess-of-revenues\n", "fund-balance/excess\n",
			[]string{`view "history.html"`, `section "Excess of revenues over (under) expenditures"`,
				`row "over (under) expenditures"`, `"fund-balance/excess" names no category`}},
		{"an unknown key", "widen: [4, 5]\n", "widne: [4, 5]\n", []string{"widne"}},
		{"a schema version this command does not read", "schema_version: 1\n", "schema_version: 2\n",
			[]string{"schema_version", "got 2, want 1"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if !strings.Contains(string(b), tc.old) {
				t.Fatalf("the committed %s has no %q, so this case edits nothing", viewsFile, tc.old)
			}
			edited := strings.Replace(string(b), tc.old, tc.edit, 1)
			_, err := decodeViews([]byte(edited), reg)
			if err == nil {
				t.Fatalf("%s loaded with %q in place of %q", viewsFile, tc.edit, tc.old)
			}
			for _, w := range tc.want {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("got %v, want it to say %q", err, w)
				}
			}
		})
	}
	t.Run("a second document", func(t *testing.T) {
		_, err := decodeViews([]byte(string(b)+"---\nschema_version: 1\n"), reg)
		if err == nil || !strings.Contains(err.Error(), "more than one YAML document") {
			t.Errorf("got %v, want the second document refused", err)
		}
	})
	t.Run("an empty file", func(t *testing.T) {
		_, err := decodeViews(nil, reg)
		if err == nil || !strings.Contains(err.Error(), "file is empty") {
			t.Errorf("got %v, want the empty file refused", err)
		}
	})
}

// TestExportRefusesAViewsFileItCannotLoad is the loader's refusal as `fisc
// export` reports it: before anything is written, naming the step.
func TestExportRefusesAViewsFileItCannotLoad(t *testing.T) {
	opts, _, _, _ := testOptions(t)
	root, err := opts.RepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "data", viewsFile)
	b, err := os.ReadFile(path) // #nosec G304 -- the test's own fake repository.
	if err != nil {
		t.Fatal(err)
	}
	edited := strings.Replace(string(b), "projection: fund-flows\n", "projection: fund-flow\n", 1)
	if edited == string(b) {
		t.Fatal("the edit changed nothing")
	}
	if werr := os.WriteFile(path, []byte(edited), 0o600); werr != nil {
		t.Fatal(werr)
	}
	err = exportRun(opts)
	if err == nil {
		t.Fatal("exportRun wrote a site from a views file naming a projection that does not exist")
	}
	for _, w := range []string{viewsFile, `step "fund-group"`, `"fund-flow"`} {
		if !strings.Contains(err.Error(), w) {
			t.Errorf("got %v, want it to say %q", err, w)
		}
	}
	if _, serr := os.Stat(filepath.Join(opts.OutputDir, export.IndexPath)); serr == nil {
		t.Error("the refusal came after index.html was written")
	}
}

// TestTheHistoryRowLabelIsTheRegistrys holds the one closed row of the
// history view to data/taxonomy.yaml: data/views.yaml names the category, and
// the label the Line cell displays is that category's document term, read
// from the registry rather than spelled a second time.
func TestTheHistoryRowLabelIsTheRegistrys(t *testing.T) {
	b, reg := committedViews(t)
	const slug = "fund-balance/excess-of-revenues"
	if !strings.Contains(string(b), slug) {
		t.Fatalf("%s does not name %s, so the label below is not resolved from it", viewsFile, slug)
	}
	c, ok := reg.Category(slug)
	if !ok || c.DocumentTerm == "" {
		t.Fatalf("the registry has no document term for %s", slug)
	}
	var history export.View
	for _, v := range mustViews(t, result{Projections: builtStemsForTest(t)}) {
		if v.Path == "history.html" {
			history = v
		}
	}
	if history.Path == "" {
		t.Fatal("no history.html view")
	}
	labels := map[string]string{}
	for _, s := range history.Sections {
		for printed, label := range s.Rows {
			labels[printed] = label
		}
	}
	if len(labels) == 0 {
		t.Fatal("the history view closes no section, so there is no row label to hold")
	}
	for printed, label := range labels {
		if label != c.DocumentTerm {
			t.Errorf("row %q displays %q, want the registry's %q", printed, label, c.DocumentTerm)
		}
	}
}

// TestAStepOpeningFromAnUndeclaredKeyIsRefusedNotDropped holds declaredSteps
// to dropping a step only for a parent IT dropped: a document that was not
// built. An After naming a key no step declares is a fault in the file, and
// `fisc export` must refuse it by name rather than ship the view without the
// step and everything below it.
func TestAStepOpeningFromAnUndeclaredKeyIsRefusedNotDropped(t *testing.T) {
	opts, _, _, _ := testOptions(t)
	root, err := opts.RepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "data", viewsFile)
	b, err := os.ReadFile(path) // #nosec G304 -- the test's own fake repository.
	if err != nil {
		t.Fatal(err)
	}
	const old, typo = "after: [fund]\n", "after: [fnud]\n"
	if strings.Count(string(b), old) != 1 {
		t.Fatalf("the committed %s has %d of %q, want one, so this case edits nothing certain",
			viewsFile, strings.Count(string(b), old), old)
	}
	if werr := os.WriteFile(path, []byte(strings.Replace(string(b), old, typo, 1)), 0o600); werr != nil {
		t.Fatal(werr)
	}
	built := builtStemsForTest(t)
	opts.Build = func(string) (result, error) { return result{Projections: built}, nil }
	err = exportRun(opts)
	if err == nil {
		t.Fatal("exportRun wrote a site whose step opens from a key no step declares")
	}
	if !strings.Contains(err.Error(), `opens from "fnud", which no step declares as its key`) {
		t.Errorf("got %v, want the refusal naming the undeclared key", err)
	}
}
