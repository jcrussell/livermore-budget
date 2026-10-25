package check

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/hint"
	"github.com/jcrussell/livermore-budget/internal/project"
	"github.com/jcrussell/livermore-budget/internal/structure"
	"github.com/jcrussell/livermore-budget/internal/vocab"
)

// repoRoot is this repository, which the tests below read committed artifacts
// from. Nothing here runs the extractor or opens a PDF.
func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := repoRootDir()
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func repoRootDir() (string, error) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		return "", fmt.Errorf("resolve the repository root: %w", err)
	}
	if _, err := os.Stat(filepath.Join(root, "data", "sources.yaml")); err != nil {
		return "", fmt.Errorf("%q does not look like the repository root: %w", root, err)
	}
	return root, nil
}

// loadCommitted loads the committed corpus once per test binary. The load is
// the whole cost of a test over the corpus, and the subject is read-only once
// built, so one serves every reader.
var loadCommitted = sync.OnceValues(func() (*Subject, error) {
	root, err := repoRootDir()
	if err != nil {
		return nil, err
	}
	return Load(LoadOptions{Root: root, Version: testVersion})
})

// committed is the committed corpus, loaded once and shared by parallel tests:
// a test that reassigns a field or edits a fact takes [mutable], and one that
// writes through a pointer the subject holds takes [isolated].
func committed(t *testing.T) *Subject {
	t.Helper()
	s, err := loadCommitted()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return s
}

// mutable is the committed corpus with its own Facts, Files and Projections
// slices; a write through what those slices point at is [isolated]'s.
func mutable(t *testing.T) *Subject {
	t.Helper()
	s := *committed(t)
	s.Facts = slices.Clone(s.Facts)
	s.Files = slices.Clone(s.Files)
	s.Projections = slices.Clone(s.Projections)
	return &s
}

// isolated is a fresh load of the committed corpus for one test that writes
// through a pointer the subject holds, so its rule files and documents are
// read by no other test.
func isolated(t *testing.T) *Subject {
	t.Helper()
	s, err := Load(LoadOptions{Root: repoRoot(t), Version: testVersion})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return s
}

// copyRepoFile copies one repository-relative file into dst, creating parents.
func copyRepoFile(t *testing.T, src, dst, rel string) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(src, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	out := filepath.Join(dst, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(out), 0o750); err != nil {
		t.Fatalf("mkdir for %s: %v", rel, err)
	}
	if err := os.WriteFile(out, b, 0o600); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}

// repoWithoutPDFs copies everything Load reads into a temporary tree -- the fact
// store, rule files, registry files and the whole committed extraction, which
// artifacts-match-manifest requires -- and no data/pdf.
func repoWithoutPDFs(t *testing.T) string {
	t.Helper()
	src, dst := repoRoot(t), t.TempDir()

	copyRepoFile(t, src, dst, factsFile)
	copyRepoFile(t, src, dst, dataDir+"/funds.yaml")
	copyRepoFile(t, src, dst, dataDir+"/taxonomy.yaml")
	copyRepoFile(t, src, dst, dataDir+"/departments.yaml")
	copyRepoFile(t, src, dst, sourcesFile)

	rules, err := os.ReadDir(filepath.Join(src, mappingsDir))
	if err != nil {
		t.Fatalf("read the mapping directory: %v", err)
	}
	for _, e := range rules {
		if !e.IsDir() {
			copyRepoFile(t, src, dst, mappingsDir+"/"+e.Name())
		}
	}
	for _, rel := range extractedFiles(t, src) {
		copyRepoFile(t, src, dst, rel)
	}

	// The premise of the test, asserted rather than assumed.
	if _, err := os.Stat(filepath.Join(dst, dataDir, "pdf")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stat data/pdf in the copied tree = %v, want it absent", err)
	}
	return dst
}

// extractedFiles is every file under data/extracted/, as repository-relative
// slash-separated paths.
func extractedFiles(t *testing.T, root string) []string {
	t.Helper()
	dir := filepath.Join(root, filepath.FromSlash(extractedDir))
	var out []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", extractedDir, err)
	}
	if len(out) == 0 {
		t.Fatalf("%s holds no files, so a copy of it proves nothing", extractedDir)
	}
	sort.Strings(out)
	return out
}

// readFacts reads a tree's fact store.
func readFacts(t *testing.T, root string) []fact.Fact {
	t.Helper()
	f, err := os.Open(filepath.Join(root, filepath.FromSlash(factsFile)))
	if err != nil {
		t.Fatalf("open the fact store: %v", err)
	}
	defer f.Close()
	facts, err := fact.Read(f)
	if err != nil {
		t.Fatalf("read the fact store: %v", err)
	}
	return facts
}

// mutateFacts rewrites a copied tree's fact store, so a test makes a check fail
// from an input a repository could contain.
func mutateFacts(t *testing.T, root string, mutate func([]fact.Fact) []fact.Fact) {
	t.Helper()
	facts := mutate(readFacts(t, root))
	path := filepath.Join(root, filepath.FromSlash(factsFile))
	var buf bytes.Buffer
	if err := fact.Write(&buf, facts); err != nil {
		t.Fatalf("write the fact store: %v", err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// loadAndRun loads a tree and runs every check over it.
func loadAndRun(t *testing.T, root string) *Report {
	t.Helper()
	s, err := Load(LoadOptions{Root: root, Version: testVersion})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return Run(t.Context(), s, All(), ReportOptions{GeneratedBy: testVersion})
}

// TestVerifyNeedsNoPDFs: every input of a default run is a committed artifact;
// the one --full check is SKIPPED with its reason and the run still exits clean.
func TestVerifyNeedsNoPDFs(t *testing.T) {
	t.Parallel()
	s, err := Load(LoadOptions{Root: repoWithoutPDFs(t), Version: testVersion})
	if err != nil {
		t.Fatalf("Load over a tree with no data/pdf: %v", err)
	}
	rep := Run(t.Context(), s, All(), ReportOptions{GeneratedBy: testVersion})

	if rep.Counts.Fail > 0 || rep.Counts.Error > 0 {
		t.Errorf("counts = %+v over a PDF-less tree, want no failure or error:\n%v",
			rep.Counts, rep.Results)
	}
	if rep.Failed() {
		t.Error("Failed() = true with the source documents absent")
	}

	// Skipped is exactly the checks that say they need --full, and nothing else.
	var wantSkipped []string
	for _, c := range All() {
		if c.Full() {
			wantSkipped = append(wantSkipped, c.ID())
		}
	}
	var gotSkipped []string
	for _, res := range rep.Results {
		if res.Status == StatusSkipped {
			gotSkipped = append(gotSkipped, res.CheckID)
			if !strings.Contains(res.Summary, "--full") {
				t.Errorf("%s was skipped without saying why: %q", res.CheckID, res.Summary)
			}
		}
	}
	if diff := cmp.Diff(wantSkipped, gotSkipped); diff != "" {
		t.Errorf("skipped checks (-want +got):\n%s", diff)
	}
	if len(wantSkipped) == 0 {
		t.Error("no check declares Full(), so this test no longer covers the skip path")
	}
}

// TestLoadWiresOneResolverPerRuleFile covers the seam tier 2 and the structural
// sweep will read. The resolvers are memoized, and the property that matters is
// behavioural: asking twice gives the same answer, so a check that corroborates a
// figure corroborates the read the build published rather than a second one.
func TestLoadWiresOneResolverPerRuleFile(t *testing.T) {
	t.Parallel()
	s, err := Load(LoadOptions{Root: repoRoot(t), Version: testVersion})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(s.Files) == 0 {
		t.Fatal("no rule files were loaded")
	}
	if got, want := len(s.Resolvers), len(s.Files); got != want {
		t.Errorf("%d resolvers for %d rule files", got, want)
	}
	for _, f := range s.Files {
		r, ok := s.Resolvers[f.Path]
		if !ok {
			t.Fatalf("no resolver for %s", f.Path)
		}
		if _, ok := s.Docs[f.DocID]; !ok {
			t.Errorf("no extraction opened for %s", f.DocID)
		}
		rule := &f.Rules[0]
		first, _, err := r.Values(rule, &rule.Parts[0])
		if err != nil {
			t.Fatalf("resolve %s %s: %v", f.Path, rule.ID, err)
		}
		second, _, err := r.Values(rule, &rule.Parts[0])
		if err != nil {
			t.Fatalf("resolve %s %s a second time: %v", f.Path, rule.ID, err)
		}
		if diff := cmp.Diff(first, second); diff != "" {
			t.Errorf("%s %s resolved differently the second time (-first +second):\n%s",
				f.Path, rule.ID, diff)
		}
	}
}

// TestLoadCarriesTheTreesDeclarations holds each field Load fills to its
// producer's own list; a hand-built subject carries only what its builder sets.
func TestLoadCarriesTheTreesDeclarations(t *testing.T) {
	t.Parallel()
	s, err := Load(LoadOptions{Root: repoRoot(t), Version: testVersion})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for _, tc := range []struct {
		field     string
		tree, got any
	}{
		{"BalanceExceptions", structure.BalanceExceptions(), s.BalanceExceptions},
		{"Residue", structure.BudgetBookResidue(), s.Residue},
		{"Exceptions", structure.BudgetBookExceptions(), s.Exceptions},
		{"Splits", structure.BudgetBookSplits(), s.Splits},
		{"Restatements", structure.BudgetBookRestatements(), s.Restatements},
		{"Identities", structure.BudgetBookIdentities(), s.Identities},
		{"UncheckedDocuments", uncheckedDocuments, s.UncheckedDocuments},
		{"IncompleteSeries", incompleteSeries, s.IncompleteSeries},
		{"Vacancies", declaredVacuous, s.Vacancies},
	} {
		if diff := cmp.Diff(tc.tree, tc.got, cmp.AllowUnexported(vacancy{})); diff != "" {
			t.Errorf("%s (-tree +subject):\n%s", tc.field, diff)
		}
	}
	// The tree declares at least one of each list, so a subject carrying none
	// is a subject Load did not fill rather than a tree with nothing to say.
	for field, n := range map[string]int{
		"BalanceExceptions": len(s.BalanceExceptions), "Residue": len(s.Residue),
		"Exceptions": len(s.Exceptions), "Splits": len(s.Splits),
		"Restatements": len(s.Restatements), "Identities": len(s.Identities),
	} {
		if n == 0 {
			t.Errorf("the subject carries no %s", field)
		}
	}
}

// TestLoadRefusesAnUnreadableCorpus: a failure to load is a failure of the
// harness, not a failed check, so it comes back as an error rather than as a
// report full of red.
func TestLoadRefusesAnUnreadableCorpus(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		opts LoadOptions
		want string
	}{
		{"no root", LoadOptions{Version: testVersion}, "repository root is required"},
		{"no version", LoadOptions{Root: repoRoot(t)}, "version is required"},
		{"empty tree", LoadOptions{Root: t.TempDir(), Version: testVersion}, "read the fact store"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Load(tt.opts)
			if err == nil {
				t.Fatalf("Load(%+v) = nil error, want %q", tt.opts, tt.want)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error %q does not contain %q", err, tt.want)
			}
		})
	}
}

// TestLoadHintsAtBuildWhenTheFactStoreIsMissing: the fact store is a build
// product, and the fix for a missing one is a command.
func TestLoadHintsAtBuildWhenTheFactStoreIsMissing(t *testing.T) {
	t.Parallel()
	_, err := Load(LoadOptions{Root: t.TempDir(), Version: testVersion})
	var h *hint.ErrHint
	if !errors.As(err, &h) {
		t.Fatalf("error %v carries no hint", err)
	}
	if !strings.Contains(h.Hint, "fisc build") {
		t.Errorf("hint %q does not name the command that writes the fact store", h.Hint)
	}
}

// TestProjectionsCoverEveryYearTheFactsCarry pins the reason the projected slices
// are read off the fact store rather than hard-coded: both budget years live in
// one facts.jsonl, and a graph built over both doubles every figure and still
// balances, so each year gets its own graph and each is checked.
func TestProjectionsCoverEveryYearTheFactsCarry(t *testing.T) {
	t.Parallel()
	s := committed(t)

	years := map[int]bool{}
	for _, f := range s.Facts {
		if f.Scope == spineScope {
			years[f.FiscalYear] = true
		}
	}
	if len(years) < 2 {
		t.Fatalf("the fact store carries %d fiscal years in scope %q; this test needs the "+
			"two-year budget book to mean anything", len(years), spineScope)
	}

	got := map[int]int{}
	for _, p := range s.Projections {
		// The SPINE projection is what this test is about. A projection of
		// another schedule is not a defect and is not evidence either: it is of
		// its own scope, over its own columns, and published-projection-built is
		// what asserts anything about it.
		if !p.Options.HasScope(spineScope) {
			continue
		}
		if p.Graph == nil {
			t.Fatalf("%s carries no graph", p)
		}
		if len(p.Options.Columns) != 1 {
			t.Fatalf("%s was built over %d columns, want one per document", p, len(p.Options.Columns))
		}
		if p.Graph.Metadata.FiscalYear != p.Options.Columns[0].FiscalYear {
			t.Errorf("%s published fiscal year %d", p, p.Graph.Metadata.FiscalYear)
		}
		got[p.Options.Columns[0].FiscalYear]++
	}
	for year := range years {
		if got[year] == 0 {
			t.Errorf("no projection was built for FY%d, so nothing checks it", year)
		}
	}
}

// The tests below change an INPUT, through Load, and assert a check fails.

// TestAWrongAmountFails is the check the whole tier was missing. Perturbing one
// amount by a dollar moves the facts AND every figure internal/project derives from
// them together, so the link, headline and counts checks all still pass; the token
// is the only witness that does not move.
func TestAWrongAmountFails(t *testing.T) {
	t.Parallel()
	root := repoWithoutPDFs(t)
	var victim fact.Fact
	mutateFacts(t, root, func(facts []fact.Fact) []fact.Fact {
		facts[7].AmountCents += 100
		victim = facts[7]
		return facts
	})

	rep := loadAndRun(t, root)
	if !rep.Failed() {
		t.Error("a $1 perturbation of a published figure did not fail the run")
	}
	res := resultFor(t, rep, "fact-token-reparses")
	if res.Status != StatusFail {
		t.Fatalf("fact-token-reparses = %s (%s), want fail", res.Status, res.Summary)
	}
	if got := len(res.Findings); got != 1 {
		t.Fatalf("findings = %d, want exactly the perturbed fact: %v", got, res.Findings)
	}
	if res.Findings[0].Subject != victim.ID {
		t.Errorf("finding names %q, want the perturbed fact %s", res.Findings[0].Subject, victim.ID)
	}
	for _, want := range []string{victim.Token, "off by $1.00"} {
		if !strings.Contains(res.Findings[0].Detail, want) {
			t.Errorf("finding %q does not contain %q", res.Findings[0].Detail, want)
		}
	}
}

// TestACentsValueUnderADollarsUnitFails covers the shape review demonstrated: a
// figure ending in fractional cents on a schedule printed in whole dollars. It
// passed ten checks, because every one of them was summing the same corrupted
// number.
func TestACentsValueUnderADollarsUnitFails(t *testing.T) {
	t.Parallel()
	root := repoWithoutPDFs(t)
	mutateFacts(t, root, func(facts []fact.Fact) []fact.Fact {
		facts[0].AmountCents += 69
		return facts
	})

	res := resultFor(t, loadAndRun(t, root), "fact-token-reparses")
	if res.Status != StatusFail {
		t.Errorf("fact-token-reparses = %s (%s), want fail", res.Status, res.Summary)
	}
	if !hasFinding(res, "off by $0.69") {
		t.Errorf("findings %v do not state the 69-cent difference", res.Findings)
	}
}

// TestACorruptedTokenFails is the same check from the other side: the amount is
// untouched and the text it claims to come from is not.
func TestACorruptedTokenFails(t *testing.T) {
	t.Parallel()
	root := repoWithoutPDFs(t)
	mutateFacts(t, root, func(facts []fact.Fact) []fact.Fact {
		facts[3].Token = "1,234"
		return facts
	})

	rep := loadAndRun(t, root)
	res := resultFor(t, rep, "fact-token-reparses")
	if res.Status != StatusFail {
		t.Fatalf("fact-token-reparses = %s, want fail", res.Status)
	}
	if !hasFinding(res, `token "1,234"`) {
		t.Errorf("findings %v do not name the token", res.Findings)
	}
	// And the page no longer says what the fact says it says, which is a second,
	// independent failure rather than the same one twice.
	if got := resultFor(t, rep, "fact-offset-points-at-token").Status; got != StatusFail {
		t.Errorf("fact-offset-points-at-token = %s, want fail for a token the page does not carry", got)
	}
}

// TestAMovedOffsetFails is the provenance pointer on its own. The amount is right,
// the token is right, and the citation lands somewhere else on the page — which no
// other check in the report can see.
func TestAMovedOffsetFails(t *testing.T) {
	t.Parallel()
	root := repoWithoutPDFs(t)
	var victim fact.Fact
	mutateFacts(t, root, func(facts []fact.Fact) []fact.Fact {
		facts[11].Offset += 3
		victim = facts[11]
		return facts
	})

	rep := loadAndRun(t, root)
	if !rep.Failed() {
		t.Error("a citation pointing at the wrong bytes did not fail the run")
	}
	res := resultFor(t, rep, "fact-offset-points-at-token")
	if res.Status != StatusFail {
		t.Fatalf("fact-offset-points-at-token = %s (%s), want fail", res.Status, res.Summary)
	}
	if got := len(res.Findings); got != 1 || res.Findings[0].Subject != victim.ID {
		t.Fatalf("findings = %v, want exactly the moved fact %s", res.Findings, victim.ID)
	}
	if !strings.Contains(res.Findings[0].Detail, victim.Token) {
		t.Errorf("finding %q does not name the token the citation claims", res.Findings[0].Detail)
	}
	// The amount is untouched, so the token check has nothing to say. Two axes,
	// two checks.
	if got := resultFor(t, rep, "fact-token-reparses").Status; got != StatusPass {
		t.Errorf("fact-token-reparses = %s, want pass: only the offset moved", got)
	}
}

// TestANegativeOffsetIsRefusedBeforeTheChecksRun: the schema refuses a
// negative offset before fact-offset-points-at-token sees it. That check's
// remaining-length subtraction guards a LARGE offset, still driven below.
func TestANegativeOffsetIsRefusedBeforeTheChecksRun(t *testing.T) {
	t.Parallel()
	root := repoWithoutPDFs(t)
	mutateFacts(t, root, func(facts []fact.Fact) []fact.Fact {
		facts[0].Offset = -1
		return facts
	})
	_, err := Load(LoadOptions{Root: root, Version: "test"})
	if err == nil {
		t.Fatal("Load accepted a fact with a negative offset")
	}
	for _, want := range []string{"fact.schema.json", "offset"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Load error %q does not name %q", err, want)
		}
	}
}

// TestAnOffsetPastTheEndOfThePageFails guards the bounds check, including
// MaxInt64, where offset+len(token) > len(text) wraps negative; hence the
// remaining-length subtraction.
func TestAnOffsetPastTheEndOfThePageFails(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name   string
		offset int
	}{
		{"past the end", 1 << 30},
		{"maxint64, where the naive bound overflows", math.MaxInt64},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := repoWithoutPDFs(t)
			mutateFacts(t, root, func(facts []fact.Fact) []fact.Fact {
				facts[0].Offset = tt.offset
				return facts
			})

			res := resultFor(t, loadAndRun(t, root), "fact-offset-points-at-token")
			if res.Status != StatusFail {
				t.Fatalf("fact-offset-points-at-token = %s, want fail", res.Status)
			}
			if !hasFinding(res, "runs past the end of") {
				t.Errorf("findings %v do not report the offset as out of bounds", res.Findings)
			}
		})
	}
}

// TestFactsMovedOutOfEveryProjectionFail: a one-word scope edit in a rule file
// takes facts out of every graph, invisibly to the graph checks.
// cuts-tie-along-the-lattice's coverage arm names each fact in no cut and no
// declared residue.
func TestFactsMovedOutOfEveryProjectionFail(t *testing.T) {
	t.Parallel()
	root := repoWithoutPDFs(t)
	const otherScope = "all-funds-gross-detail"
	moved := map[string]bool{}
	mutateFacts(t, root, func(facts []fact.Fact) []fact.Fact {
		for i := range facts {
			if facts[i].Kind == "revenue" && facts[i].FundGroup == "general" {
				facts[i].Scope = otherScope
				moved[facts[i].ID] = true
			}
		}
		return facts
	})
	if len(moved) == 0 {
		t.Fatal("no fact matched the mutation, so this test covers nothing")
	}

	rep := loadAndRun(t, root)
	if !rep.Failed() {
		t.Error("moving facts out of every cut did not fail the run")
	}
	res := resultFor(t, rep, "cuts-tie-along-the-lattice")
	if res.Status != StatusFail {
		t.Fatalf("cuts-tie-along-the-lattice = %s (%s), want fail", res.Status, res.Summary)
	}
	named := map[string]bool{}
	for _, f := range res.Findings {
		if f.Subject != "coverage" {
			continue
		}
		for id := range moved {
			if strings.Contains(f.Detail, id) {
				named[id] = true
				if !strings.Contains(f.Detail, otherScope) {
					t.Errorf("finding %q does not name the scope no cut reads", f.Detail)
				}
			}
		}
	}
	if len(named) != len(moved) {
		t.Errorf("%d of the %d moved facts are named by a coverage finding", len(named), len(moved))
	}
	// The published slice still exists, so this is the coverage arm firing and
	// not a side effect of the projection disappearing.
	if got := resultFor(t, rep, "published-projection-built").Status; got != StatusPass {
		t.Errorf("published-projection-built = %s, want pass", got)
	}
}

// TestEveryFactMovedOutOfScopeRefusesToLoad is the whole-store version. It cannot
// reach a report at all: with no projection there is nothing for ten of the checks
// to read, they would all go vacuous at once, and the run would exit 0 having
// checked the fact store and nothing else.
func TestEveryFactMovedOutOfScopeRefusesToLoad(t *testing.T) {
	t.Parallel()
	root := repoWithoutPDFs(t)
	mutateFacts(t, root, func(facts []fact.Fact) []fact.Fact {
		for i := range facts {
			facts[i].Scope = "all-funds-gross-v2"
		}
		return facts
	})

	_, err := Load(LoadOptions{Root: root, Version: testVersion})
	if err == nil {
		t.Fatal("Load = nil error over a fact store no projection covers")
	}
	if !strings.Contains(err.Error(), spineScope) {
		t.Errorf("error %q does not name the scope nothing was found in", err)
	}
	var h *hint.ErrHint
	if !errors.As(err, &h) {
		t.Errorf("error %v carries no hint, and the fix is to look at a rule's scope", err)
	}
}

// TestTheYearTheSitePublishesMustBeBuilt is the other half of the same hole: the
// store can carry facts, project them, and pass every graph check over a slice the
// site does not publish.
func TestTheYearTheSitePublishesMustBeBuilt(t *testing.T) {
	t.Parallel()
	root := repoWithoutPDFs(t)
	mutateFacts(t, root, func(facts []fact.Fact) []fact.Fact {
		out := facts[:0]
		for _, f := range facts {
			if f.FiscalYear != project.PublishedFiscalYear {
				out = append(out, f)
			}
		}
		return out
	})

	rep := loadAndRun(t, root)
	if !rep.Failed() {
		t.Error("dropping the published fiscal year did not fail the run")
	}
	res := resultFor(t, rep, "published-projection-built")
	if res.Status != StatusFail {
		t.Fatalf("published-projection-built = %s (%s), want fail", res.Status, res.Summary)
	}
	detail := said(res)
	for _, want := range []string{"unexamined", "FY2027"} {
		if !strings.Contains(detail, want) {
			t.Errorf("findings %v do not contain %q", res.Findings, want)
		}
	}
	// The other year is still checked, which is what makes this a report about a
	// missing slice rather than an empty one.
	if got := resultFor(t, rep, "counts-reconcile").Status; got != StatusPass {
		t.Errorf("counts-reconcile = %s, want pass over the remaining year", got)
	}
}

// TestASpineDocumentWithoutAHeadlineIsAFinding: one published spine document
// loses its headline; every check reading one names it, and the other document
// stays a subject.
func TestASpineDocumentWithoutAHeadlineIsAFinding(t *testing.T) {
	t.Parallel()
	s := isolated(t)
	docs, findings := s.spine()
	if len(findings) != 0 {
		t.Fatalf("the committed spine is not whole: %+v", findings)
	}
	if len(docs) < 2 {
		t.Fatalf("the corpus publishes %d spine document(s); with fewer than two, losing "+
			"one headline cannot be told from losing them all", len(docs))
	}
	victim := docs[len(docs)-1]
	victim.Graph.Metadata.Headline = nil

	rep := runChecks(t, s)
	for _, id := range []string{
		"headline-ties-to-facts", "headline-transfer-residual", "headline-naive-expenditure",
	} {
		res := resultFor(t, rep, id)
		if res.Status != StatusFail {
			t.Errorf("%s = %s (%s), want fail with %s's headline gone", id, res.Status,
				res.Summary, victim)
			continue
		}
		if len(res.Findings) != 1 || res.Findings[0].Subject != victim.String() {
			t.Errorf("%s findings = %+v, want exactly one naming %s", id, res.Findings, victim)
		}
		if res.Subjects == 0 {
			t.Errorf("%s examined nothing, but the other spine document still carries "+
				"its headline and must still be a subject", id)
		}
	}
}

// TestARetargetedScopeUnbuildsThePublishedTrendsDocument retargets the scope
// (the facts survive, no projection is of them) and asserts on
// published-projection-built's own result, since other checks also go red.
func TestARetargetedScopeUnbuildsThePublishedTrendsDocument(t *testing.T) {
	t.Parallel()
	root := repoWithoutPDFs(t)
	moved := 0
	mutateFacts(t, root, func(facts []fact.Fact) []fact.Fact {
		for i := range facts {
			if facts[i].Scope == project.TrendsScope {
				facts[i].Scope = project.TrendsScope + "s"
				moved++
			}
		}
		return facts
	})
	if moved == 0 {
		t.Fatalf("no fact carries scope %q, so this test asserts nothing", project.TrendsScope)
	}

	rep := loadAndRun(t, root)
	res := resultFor(t, rep, "published-projection-built")
	if res.Status != StatusFail {
		t.Fatalf("published-projection-built = %s (%s), want fail: %d facts of the published "+
			"trends document were retargeted and no document was built from them",
			res.Status, res.Summary, moved)
	}
	// Every document of the retargeted schedule is named, and the spine is not.
	var named []string
	for _, f := range res.Findings {
		named = append(named, f.Subject)
		if strings.Contains(f.Subject, project.PublishedProjection) {
			t.Errorf("finding %q names the spine, which this mutation did not touch", f.Subject)
		}
	}
	if !slices.ContainsFunc(res.Findings, func(f Finding) bool {
		return strings.Contains(f.Subject, project.TrendsProjection)
	}) {
		t.Errorf("no finding names %q, the document that stopped being built: %v",
			project.TrendsProjection, named)
	}

	// The spine is still checked. That is what makes this a report about ONE
	// missing document rather than a run that fell over.
	if got := resultFor(t, rep, "counts-reconcile").Status; got != StatusPass {
		t.Errorf("counts-reconcile = %s, want pass over the untouched spine", got)
	}
	// The two trend checks stay green, passing over the ACFR history documents;
	// published-projection-built is what turns the vanished one into a finding.
	for _, id := range []string{"trend-points-tie-to-facts", "trend-series-are-complete"} {
		if got := resultFor(t, rep, id).Status; got != StatusPass {
			t.Errorf("%s = %s, want pass over the remaining series documents, with "+
				"published-projection-built alone reporting the vanished one", id, got)
		}
	}
}

// TestASliceNoDocumentClaimsIsReported builds the spine with an FY2026 revised
// column beside the adopted one, so a slice no document publishes is reported
// by name and by the stem it would take.
func TestASliceNoDocumentClaimsIsReported(t *testing.T) {
	t.Parallel()
	root := repoWithoutPDFs(t)
	added := 0
	mutateFacts(t, root, func(facts []fact.Fact) []fact.Fact {
		out := append([]fact.Fact{}, facts...)
		for _, f := range facts {
			if f.Scope != project.PublishedScope || f.FiscalYear != project.PublishedFiscalYear ||
				f.Basis != project.PublishedBasis {
				continue
			}
			// A second BASIS for the same year, which is the whole scenario.
			// The id is recomputed because it hashes the basis, and two facts
			// sharing one id is a different failure that would mask this one.
			f.Basis = vocab.BasisRevised
			f.ID = fact.MakeID(f.DocID, f.RuleID, f.RowPath, f.RowLabel, f.ColumnPath,
				f.FiscalYear, f.Basis)
			out = append(out, f)
			added++
		}
		fact.Sort(out)
		return out
	})
	if added == 0 {
		t.Fatal("no spine fact carries the published year and basis")
	}

	rep := loadAndRun(t, root)
	res := resultFor(t, rep, "published-projection-built")
	if res.Status != StatusFail {
		t.Fatalf("published-projection-built = %s (%s), want fail: %d facts made a third "+
			"spine slice that no published document covers", res.Status, res.Summary, added)
	}
	if len(res.Findings) != 1 {
		t.Fatalf("findings = %v, want exactly one", res.Findings)
	}
	// The finding names the SLICE and the STEM, because those are two different
	// repairs: the slice says which columns nobody publishes, and the stem says
	// which file would appear in dist/ that no reader is offered.
	got := res.Findings[0].Subject + " " + res.Findings[0].Detail
	for _, want := range []string{"FY2026 revised", `"sankey-2026-revised"`, "nothing published claims"} {
		if !strings.Contains(got, want) {
			t.Errorf("finding = %q, want it to contain %q", got, want)
		}
	}
}

// TestAPublishedDocumentShortAColumnIsReported: a corpus losing a column that
// project.TrendsColumns declares goes red naming the column, though every
// series is complete over the columns that survive.
func TestAPublishedDocumentShortAColumnIsReported(t *testing.T) {
	t.Parallel()
	dropped := project.TrendsColumns()[0]
	root := repoWithoutPDFs(t)
	gone := 0
	mutateFacts(t, root, func(facts []fact.Fact) []fact.Fact {
		out := facts[:0]
		for _, f := range facts {
			if f.Scope == project.TrendsScope && f.FiscalYear == dropped.FiscalYear &&
				f.Basis == dropped.Basis {
				gone++
				continue
			}
			out = append(out, f)
		}
		return out
	})
	if gone == 0 {
		t.Fatalf("no fact carries %s in scope %q", project.Describe([]project.Column{dropped}),
			project.TrendsScope)
	}

	rep := loadAndRun(t, root)
	res := resultFor(t, rep, "published-projection-built")
	if res.Status != StatusFail {
		t.Fatalf("published-projection-built = %s (%s), want fail: %d facts of one published "+
			"column are gone and the document was built without it", res.Status, res.Summary, gone)
	}
	// The finding says "built without", not "nothing checked it". The
	// drill-down's FY2024 document stops existing too, so this looks for the
	// finding naming the trends stem rather than counting.
	var detail string
	for _, f := range res.Findings {
		if strings.Contains(f.Subject, project.TrendsProjection) {
			detail = f.Detail
		}
	}
	if detail == "" {
		t.Fatalf("no finding names %q: %v", project.TrendsProjection, res.Findings)
	}
	if !strings.Contains(detail, "built without") ||
		!strings.Contains(detail, project.Describe([]project.Column{dropped})) {
		t.Errorf("finding %q does not say the document was built without %s",
			detail, project.Describe([]project.Column{dropped}))
	}

	// The check this one exists to backstop still passes, which is the whole
	// point: every remaining series IS complete over the columns that remain.
	if got := resultFor(t, rep, "trend-series-are-complete").Status; got != StatusPass {
		t.Errorf("trend-series-are-complete = %s, want pass: it compares each series against "+
			"the columns the document was built over, and it cannot see a whole column go",
			got)
	}
}

// TestContestedTotalsAreStillContested: over the committed corpus every
// contested-total entry's column is published, its figure drawn, and that
// figure differs from the one printed elsewhere.
func TestContestedTotalsAreStillContested(t *testing.T) {
	t.Parallel()
	entries, err := project.ContestedTotals()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatal("project.ContestedTotals() declares nothing, so this test holds nothing; " +
			"remove it with the last entry")
	}

	s := committed(t)

	// One graph per published (fiscal year, basis) of the spine, keyed so a
	// missing column is reported as a missing column rather than as a zero sum.
	graphs := map[project.Column]*project.Document{}
	docs, findings := s.spine()
	if len(findings) != 0 {
		t.Fatalf("the committed spine is not whole: %+v", findings)
	}
	for _, p := range docs {
		for _, c := range p.Options.Columns {
			graphs[c] = p.Graph
		}
	}

	for _, e := range entries {
		g, ok := graphs[e.Column]
		if !ok {
			t.Errorf("%s declares column %s, which this corpus publishes no graph for; "+
				"the entry names a column that has gone away", e.Exception, e.Column)
			continue
		}
		got := project.GroupExpenditure(g.Links, e.FundGroup)
		if got != e.Published {
			t.Errorf("%s declares %s %s expenditure of %d cents and the published graph "+
				"draws %d. The caveat is no longer printed; re-derive this entry against "+
				"the pages and either update it or remove it with whatever decided it. "+
				"Do NOT simply delete it -- the figure moving is not the same event as "+
				"the contradiction being resolved",
				e.Exception, e.Column, e.FundGroup, e.Published, got)
		}
		// A declaration whose two figures agree is not a contested total at
		// all, and would print a caveat saying a figure differs from itself.
		if e.Published == e.Elsewhere {
			t.Errorf("%s declares the same figure as both published and elsewhere", e.Exception)
		}
	}
}
