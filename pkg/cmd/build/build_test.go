package build

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/jcrussell/livermore-budget/internal/amount"
	"github.com/jcrussell/livermore-budget/internal/corpus"
	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/mapping"
	"github.com/jcrussell/livermore-budget/pkg/cmdutil"
	"github.com/jcrussell/livermore-budget/pkg/iostreams"
)

// docID is the document every fixture here belongs to. It is the real one:
// the rule files declare it, and OpenDoc requires the extraction directory to
// be named for it.
const docID = "livermore-budget-fy2026-2027"

// allFundsFY2026Revenues is the control total the Budget Book prints for
// FY2025-26 citywide revenues, in dollars. Every revenue fact the spine
// produces for that year must add up to it.
const allFundsFY2026Revenues = 299_969_007

// The synthetic transfer schedule transfers.yaml maps. It is written here
// rather than copied from data/extracted/ because the corpus contains no page
// of this shape: a two-page schedule whose second page carries no labels and
// whose totals the document never prints. Real pages are preferred wherever
// one exercises the behaviour (see spinePages); this is a case where none does.
const (
	transfersOutPage = `TRANSFERS OUT:
General Fund 10,037,797 10,146,598
Enterprise Funds 19,813,147 24,680,000
END OF SCHEDULE
`
	transfersOutContinuationPage = `TRANSFERS OUT:
1,000 2,000
END OF SCHEDULE
`
	transfersInPage = `TRANSFERS IN:
General Fund 480,400 486,735
TOTAL TRANSFERS IN: $480,400 $486,735
`
	transfersInContinuationPage = `TRANSFERS IN:
5,000 6,000
`

	// roundedPage's first column prints a total one dollar above its rows;
	// the second ties exactly. See testdata/rounding.yaml.
	roundedPage = `TAXES:
Secured 100 1,000
Unsecured 200 2,000
TOTAL TAXES: $301 $3,000
`

	// The two halves of one block, split across a page break: the printed
	// Total on the second page covers a row that is only on the first.
	// 2,056 + 132,186 = 134,242. See testdata/spanning.yaml.
	straddleHead = `SERVICES:
Wages & Benefits 2,056
END OF PAGE
`
	straddleTail = `SERVICES:
Services & Supplies 132,186
Total Services $134,242
`

	// Two divisions and the department total printed over both, plus a total
	// the page prints that nothing on it sums to. 300 + 700 = 1,000.
	// See testdata/rollup.yaml.
	// A block and its printed subtotal, figures right-aligned under their
	// header so the geometry monoGeometry derives places each in its column.
	// See testdata/subtotal.yaml.
	subtotalPage = `      FY A
Alpha       10
Beta        20
SUBTOTAL  $ 30
END
`

	rollupPage = `Division One
Wages 100
Supplies 200
Total $300
Division Two
Wages 300
Supplies 400
Total $700
DEPARTMENT TOTAL $1,000
Total Sources $58,000
`
)

func writeRepoFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// testRepo stands up a repository root: an extraction holding the given pages,
// and a mapping directory holding the named fixtures. The mapping directory is
// always created, so a test can ask what an empty one does.
func testRepo(t *testing.T, pages map[int]string, rules ...string) string {
	t.Helper()
	return testRepoWithGeometry(t, pages, nil, rules...)
}

// testRepoWithGeometry is testRepo with each page's geometry too, for a rule
// declaring column_headers, whose column guard reads it.
func testRepoWithGeometry(t *testing.T, pages, geometry map[int]string, rules ...string) string {
	t.Helper()
	root := t.TempDir()

	extracted := filepath.Join(root, "data", "extracted", docID)
	artifacts := map[string]corpus.Artifact{}
	for n, body := range pages {
		name := corpus.PagePath(n)
		writeRepoFile(t, filepath.Join(extracted, filepath.FromSlash(name)), body)
		artifacts[name] = corpus.Artifact{Bytes: int64(len(body))}
	}
	for n, body := range geometry {
		name := corpus.GeometryPath(n)
		writeRepoFile(t, filepath.Join(extracted, filepath.FromSlash(name)), body)
		artifacts[name] = corpus.Artifact{Bytes: int64(len(body))}
	}
	man, err := json.Marshal(map[string]any{
		"schema_version": corpus.SchemaVersion,
		"doc_id":         docID,
		"artifacts":      artifacts,
	})
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	writeRepoFile(t, filepath.Join(extracted, "manifest.json"), string(man))

	if err := os.MkdirAll(filepath.Join(root, defaultMappings), 0o750); err != nil {
		t.Fatalf("mkdir mappings: %v", err)
	}
	for _, name := range rules {
		body, err := os.ReadFile(filepath.Join("testdata", name))
		if err != nil {
			t.Fatalf("read rule fixture: %v", err)
		}
		writeRepoFile(t, filepath.Join(root, defaultMappings, name), string(body))
	}
	return root
}

// fixtureCopy is one committed fixture and the extraction artifact it must
// equal byte for byte.
//
// There is no document column, unlike the table in internal/mapping: testRepo
// stands up exactly one extraction directory, named for the single docID const
// above, so this package is single-document by construction rather than by
// omission.
type fixtureCopy struct{ fixture, artifact string }

// pageCopy names the page-text fixture for one page.
func pageCopy(n int) fixtureCopy {
	return fixtureCopy{
		fixture:  filepath.Join("testdata", "pages", fmt.Sprintf("p%04d.txt", n)),
		artifact: corpus.PagePath(n),
	}
}

// extraction is the artifact this fixture was copied from.
func (f fixtureCopy) extraction() string {
	return filepath.Join("..", "..", "..", "data", "extracted", docID,
		filepath.FromSlash(f.artifact))
}

// fixtureCopies is every fixture this package reads. Adding one means adding a
// row here and nothing else (fisc-28u).
func fixtureCopies() []fixtureCopy {
	var out []fixtureCopy
	for _, n := range []int{66, 67} {
		out = append(out, pageCopy(n))
	}
	return out
}

// pageFixture reads one page of the Budget Book. The fixtures under testdata/
// are byte-identical copies of data/extracted/, so a build that passes here
// passes against the committed corpus, and none of this needs Python or the
// source PDF.
func pageFixture(t *testing.T, n int) string {
	t.Helper()
	b, err := os.ReadFile(pageCopy(n).fixture)
	if err != nil {
		t.Fatalf("read page fixture: %v", err)
	}
	return string(b)
}

// TestFixturesAreVerbatimCopies keeps the claim in pageFixture's comment
// honest. These pages are a second copy of testdata/pages/, which is itself a
// copy of data/extracted/, and nothing regenerates either: a build that passes
// here only tells you about the committed corpus for as long as the copies
// still match it. They silently stopped matching once already (fisc-yqv.5).
func TestFixturesAreVerbatimCopies(t *testing.T) {
	copies := fixtureCopies()
	if len(copies) == 0 {
		t.Fatal("no fixtures listed, so this test asserts nothing")
	}
	for _, f := range copies {
		t.Run(f.fixture, func(t *testing.T) {
			fixture, err := os.ReadFile(f.fixture)
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}
			extracted, err := os.ReadFile(f.extraction())
			if err != nil {
				t.Fatalf("read extraction: %v", err)
			}
			if !bytes.Equal(fixture, extracted) {
				t.Errorf("%s is not a verbatim copy of %s; "+
					"re-copy it rather than adjusting whatever now fails",
					f.fixture, f.extraction())
			}
		})
	}
}

func spinePages(t *testing.T) map[int]string {
	t.Helper()
	return map[int]string{66: pageFixture(t, 66), 67: pageFixture(t, 67)}
}

func transferPages() map[int]string {
	return map[int]string{
		76: transfersOutPage,
		77: transfersOutContinuationPage,
		78: transfersInPage,
		79: transfersInContinuationPage,
	}
}

// testOptions returns Options wired to root with the flag defaults, plus the
// two streams a test asserts on.
func testOptions(t *testing.T, root string) (*Options, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	ios, _, out, errOut := iostreams.Test()
	return &Options{
		IO:       ios,
		RepoRoot: func() (string, error) { return root, nil },
		Output:   defaultOutput,
		Mappings: defaultMappings,
	}, out, errOut
}

func readFacts(t *testing.T, path string) []fact.Fact {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	t.Cleanup(func() { _ = f.Close() })
	facts, err := fact.Read(f)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return facts
}

// TestBuildTiesToTheDocumentsOwnTotals is the check this command exists to
// make. It resolves the citywide spine end to end and asserts the published
// file against the figures the city printed, not against a golden blob: the
// arithmetic is what a reader of this project is being asked to trust.
func TestBuildTiesToTheDocumentsOwnTotals(t *testing.T) {
	root := testRepo(t, spinePages(t), "spine.yaml")
	opts, out, errOut := testOptions(t, root)

	if err := buildRun(opts); err != nil {
		t.Fatalf("buildRun: %v", err)
	}

	facts := readFacts(t, filepath.Join(root, defaultOutput))
	// 40 revenue values on p66, 16 expenditure values on p66, 80 on p67 —
	// every one a cell the city printed.
	if got, want := len(facts), 136; got != want {
		t.Errorf("wrote %d facts, want %d", got, want)
	}
	if err := fact.CheckSorted(facts); err != nil {
		t.Errorf("the written file is not in canonical order: %v", err)
	}
	if err := fact.CheckUniqueIDs(facts); err != nil {
		t.Errorf("the written file has colliding ids: %v", err)
	}

	var revenues int64
	for _, f := range facts {
		if f.Kind == mapping.KindRevenue && f.FiscalYear == 2026 {
			revenues += f.AmountCents
		}
	}
	if want := int64(allFundsFY2026Revenues) * 100; revenues != want {
		t.Errorf("FY2025-26 citywide revenues = %d cents, want %d", revenues, want)
	}

	// The facts went to a file, so stdout must stay empty: nothing this
	// command prints belongs in a pipe (byob-iostreams.3).
	if got := out.String(); got != "" {
		t.Errorf("got stdout %q, want it empty", got)
	}
	want := "wrote 136 facts to facts/facts.jsonl\n" +
		"2 rules over 3 parts in 1 rule file\n" +
		"3 of 3 parts tie to a total the document prints, covering 16 columns\n"
	if diff := cmp.Diff(want, errOut.String()); diff != "" {
		t.Errorf("summary mismatch (-want +got):\n%s", diff)
	}
}

// TestBuildReportsWhatItCouldNotCheck covers the totals policy from the
// command's side: the parts it checked, the parts it could not, and why.
func TestBuildReportsWhatItCouldNotCheck(t *testing.T) {
	root := testRepo(t, transferPages(), "transfers.yaml")
	opts, out, errOut := testOptions(t, root)
	opts.JSON = true

	if err := buildRun(opts); err != nil {
		t.Fatalf("buildRun: %v", err)
	}

	var got report
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("decode report: %v\n%s", err, out)
	}
	want := report{
		Output:    "facts/facts.jsonl",
		Facts:     10,
		RuleFiles: 1,
		Rules:     2,
		Parts:     4,
		// Only p78 has a total the document prints and a rule that names it.
		PartsChecked: 1,
		ColumnsTied:  2,
		PartsUnchecked: []uncheckedPart{
			{RuleID: "transfers-out", Page: 76, Reason: reasonNoTotalRow},
			{RuleID: "transfers-out", Page: 77, Reason: reasonNoTotalRow},
			{RuleID: "transfers-in", Page: 79, Reason: reasonNoStatedTotals},
		},
		// Empty rather than nil: a key that becomes null when a list is empty
		// makes a consumer handle two shapes for one meaning, which is the
		// contract newReport states. ToleranceSlack is here for the same
		// reason -- no rule in this fixture declares printed_decimals, and the
		// key must still round-trip as [] rather than null.
		ToleranceSlack:    []amount.Cents{},
		RollupsUnasserted: []unassertedRollup{},
		Omissions: []declaredOmission{
			{RuleID: "transfers-out", Page: 77, RowLabel: "Enterprise Funds"},
		},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("report mismatch (-want +got):\n%s", diff)
	}

	// --json is the machine-readable form, so it replaces the chatter rather
	// than accompanying it.
	if got := errOut.String(); got != "" {
		t.Errorf("got stderr %q, want it empty under --json", got)
	}
}

// TestBuildSeparatesColumnsThatTieOnlyToADeclaredDelta guards the claim the
// report makes. "2 columns tie" and "2 tie, one of them only to a declared
// dollar of the city's rounding" are different statements about the evidence,
// and fisc-2sd exists precisely so the second is never silently written as the
// first.
func TestBuildSeparatesColumnsThatTieOnlyToADeclaredDelta(t *testing.T) {
	root := testRepo(t, map[int]string{80: roundedPage}, "rounding.yaml")
	opts, out, errOut := testOptions(t, root)
	opts.JSON = true

	if err := buildRun(opts); err != nil {
		t.Fatalf("buildRun: %v", err)
	}
	var got report
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("decode report: %v\n%s", err, out)
	}
	if got.ColumnsTied != 2 {
		t.Errorf("ColumnsTied = %d, want 2", got.ColumnsTied)
	}
	if got.ColumnsTiedByDeclaration != 1 {
		t.Errorf("ColumnsTiedByDeclaration = %d, want 1", got.ColumnsTiedByDeclaration)
	}
	if got := errOut.String(); got != "" {
		t.Errorf("got stderr %q, want it empty under --json", got)
	}

	// And the human-readable form says so too, since that is the one anybody
	// building the site actually reads.
	opts, _, errOut = testOptions(t, root)
	if err := buildRun(opts); err != nil {
		t.Fatalf("buildRun without --json: %v", err)
	}
	want := "1 of those columns ties only to a declared delta in the document's own arithmetic"
	if !strings.Contains(errOut.String(), want) {
		t.Errorf("report does not say %q:\n%s", want, errOut)
	}
}

// TestBuildNamesEveryUncheckedPart is the human-readable half of the same
// policy. An unchecked part that is merely counted is an unchecked part nobody
// will ever look up.
func TestBuildNamesEveryUncheckedPart(t *testing.T) {
	root := testRepo(t, transferPages(), "transfers.yaml")
	opts, _, errOut := testOptions(t, root)

	if err := buildRun(opts); err != nil {
		t.Fatalf("buildRun: %v", err)
	}

	for _, want := range []string{
		"UNCHECKED transfers-out p76: " + reasonNoTotalRow,
		"UNCHECKED transfers-out p77: " + reasonNoTotalRow,
		"UNCHECKED transfers-in p79: " + reasonNoStatedTotals,
		`DECLARED OMISSION transfers-out p77: the page does not print row "Enterprise Funds"`,
	} {
		if !strings.Contains(errOut.String(), want) {
			t.Errorf("summary %q does not contain %q", errOut, want)
		}
	}
}

func ruleByID(t *testing.T, f *mapping.File, id string) *mapping.Rule {
	t.Helper()
	for i := range f.Rules {
		if f.Rules[i].ID == id {
			return &f.Rules[i]
		}
	}
	t.Fatalf("no rule %q in %s", id, f.Path)
	return nil
}

func partOn(t *testing.T, r *mapping.Rule, page int) *mapping.Part {
	t.Helper()
	for i := range r.Parts {
		if r.Parts[i].Page == page {
			return &r.Parts[i]
		}
	}
	t.Fatalf("rule %q has no part for page %d", r.ID, page)
	return nil
}

// TestTheTotalsPolicyCannotBeWrittenAsAnErrorCheck pins the reason
// Report.checkTotals asks the rule rather than the error it gets back.
//
// A part of a rule that declares no total_row does not report "no stated
// totals": it has a stop_at anchor, StatedTotals looks for an amount run after
// it, and finding none reports ErrNotFound — the very error a rule WITH a
// total row produces when that row has moved, which must fail the build. If
// this test ever fails, the policy in checkTotals has to be revisited, because
// treating ErrNotFound as "unchecked" would silence real breakage.
func TestTheTotalsPolicyCannotBeWrittenAsAnErrorCheck(t *testing.T) {
	root := testRepo(t, transferPages(), "transfers.yaml")
	files, err := mapping.LoadDir(os.DirFS(root), defaultMappings)
	if err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	doc, err := corpus.OpenDoc(root, docID)
	if err != nil {
		t.Fatalf("OpenDoc: %v", err)
	}
	r, err := mapping.NewResolver(doc, files[0])
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}

	noTotalRow := ruleByID(t, files[0], "transfers-out")
	_, err = r.CheckTotals(noTotalRow, partOn(t, noTotalRow, 77))
	if !errors.Is(err, mapping.ErrNotFound) {
		t.Errorf("CheckTotals on a rule with no total_row = %v, want ErrNotFound", err)
	}
	if errors.Is(err, mapping.ErrNoStatedTotals) {
		t.Error("CheckTotals reported ErrNoStatedTotals for a rule with no total_row; " +
			"the policy could then be written as an error check")
	}

	hasTotalRow := ruleByID(t, files[0], "transfers-in")
	_, err = r.CheckTotals(hasTotalRow, partOn(t, hasTotalRow, 79))
	if !errors.Is(err, mapping.ErrNoStatedTotals) {
		t.Errorf("CheckTotals on a part with no anchor = %v, want ErrNoStatedTotals", err)
	}
}

// TestBuildFailsWhenAColumnDoesNotTie is the check the whole project leans on:
// the document checking our work. A figure that has drifted must fail the
// build, and must not replace the fact store on its way out.
func TestBuildFailsWhenAColumnDoesNotTie(t *testing.T) {
	pages := spinePages(t)
	// Property Taxes, General Fund, FY2025-26, one dollar out. Everything else
	// about the page is untouched, so the only thing that can fail is the tie.
	const printed, drifted = "64,143,762", "64,143,763"
	if !strings.Contains(pages[66], printed) {
		t.Fatalf("page 66 fixture no longer prints %s", printed)
	}
	pages[66] = strings.Replace(pages[66], printed, drifted, 1)

	root := testRepo(t, pages, "spine.yaml")
	opts, _, errOut := testOptions(t, root)

	// A previous build's output must survive a failed one.
	target := filepath.Join(root, defaultOutput)
	writeRepoFile(t, target, "previous build\n")

	err := buildRun(opts)
	if err == nil {
		t.Fatal("buildRun = nil error, want the totals check to fail")
	}
	for _, want := range []string{"spine-revenues", "off by"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not contain %q", err, want)
		}
	}
	if got, want := readFileString(t, target), "previous build\n"; got != want {
		t.Errorf("fact store = %q, want the failed build to have left %q", got, want)
	}
	if got := errOut.String(); got != "" {
		t.Errorf("got summary %q for a failed build, want none", got)
	}
}

func readFileString(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

// TestBuildRefusesToWriteAnEmptyFactStore covers the mistyped --mappings: a
// directory with no rules in it produces no facts, and writing them over a
// good fact store would look like a successful build that deleted everything.
func TestBuildRefusesToWriteAnEmptyFactStore(t *testing.T) {
	root := testRepo(t, spinePages(t))
	opts, _, _ := testOptions(t, root)

	err := buildRun(opts)
	if err == nil {
		t.Fatal("buildRun = nil error, want a refusal")
	}
	if !strings.Contains(err.Error(), "no facts") {
		t.Errorf("error %q does not say that nothing was produced", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, defaultOutput)); !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf("stat facts file = %v, want it never to have been written", statErr)
	}
}

func TestBuildWithNoMappingDirectory(t *testing.T) {
	root := t.TempDir()
	opts, _, _ := testOptions(t, root)

	err := buildRun(opts)
	if err == nil {
		t.Fatal("buildRun = nil error, want a failure")
	}
	var hint *cmdutil.ErrHint
	if !errors.As(err, &hint) {
		t.Errorf("error %q carries no hint, and the user needs to know the path is repo-relative", err)
	}
}

// TestValidateRejectsPathsBeforeAnythingHappens covers byob-input-validation.5:
// a bad flag must be a usage error, and must be reported before the command has
// resolved a repository root, opened an extraction, or written a byte.
func TestValidateRejectsPathsBeforeAnythingHappens(t *testing.T) {
	tests := []struct {
		name             string
		output, mappings string
		want             string
	}{
		{"empty output", "", defaultMappings, "--output is empty"},
		{"empty mappings", defaultOutput, "", "--mappings is empty"},
		{"absolute output", "/etc/facts.jsonl", defaultMappings, "absolute path"},
		{"absolute mappings", defaultOutput, "/etc", "absolute path"},
		{"escaping output", "../../facts.jsonl", defaultMappings, "climbs above"},
		{"escaping mappings", defaultOutput, "..", "climbs above"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			opts := &Options{
				IO: ios,
				RepoRoot: func() (string, error) {
					t.Error("the repository root was resolved for a flag that never validated")
					return "", nil
				},
				Output:   tt.output,
				Mappings: tt.mappings,
			}

			err := buildRun(opts)
			if err == nil {
				t.Fatalf("buildRun with output %q mappings %q = nil error, want %q",
					tt.output, tt.mappings, tt.want)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error %q does not contain %q", err, tt.want)
			}
			// A FlagError is what makes this exit 2 rather than 1, which is
			// how a script tells "you called me wrong" from "it broke".
			var flagErr *cmdutil.FlagError
			if !errors.As(err, &flagErr) {
				t.Errorf("error %v is not a FlagError", err)
			}
		})
	}
}

// TestNewCmdBuildParsesItsFlags exercises the parsing layer alone: runF takes
// the test path, so no build runs and nothing is written.
func TestNewCmdBuildParsesItsFlags(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	f := &cmdutil.Factory{
		IOStreams: ios,
		RepoRoot: func() (string, error) {
			t.Error("RepoRoot resolved during flag parsing")
			return "", nil
		},
	}

	var got *Options
	cmd := NewCmdBuild(f, func(o *Options) error {
		got = o
		return nil
	})
	cmd.SetOut(ios.Out)
	cmd.SetErr(ios.ErrOut)
	cmd.SetArgs([]string{"--output", "facts/draft.jsonl", "--mappings", "mappings/draft", "--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got == nil {
		t.Fatal("runF was not called")
	}
	want := Options{Output: "facts/draft.jsonl", Mappings: "mappings/draft", JSON: true}
	if diff := cmp.Diff(want, Options{Output: got.Output, Mappings: got.Mappings, JSON: got.JSON}); diff != "" {
		t.Errorf("parsed flags mismatch (-want +got):\n%s", diff)
	}
	// The group is what puts build under "Data commands" in `fisc --help`;
	// cobra panics at AddCommand time if it names a group root does not define.
	if got, want := cmd.GroupID, "data"; got != want {
		t.Errorf("GroupID = %q, want %q", got, want)
	}
}

// TestNewCmdBuildRejectsPositionalArgs is the same class of bug as the
// unknown-command one: `fisc build facts.jsonl` must not silently ignore the
// argument and rebuild the default file.
func TestNewCmdBuildRejectsPositionalArgs(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	cmd := NewCmdBuild(&cmdutil.Factory{IOStreams: ios}, func(*Options) error {
		t.Error("the command ran with a stray positional argument")
		return nil
	})
	cmd.SetOut(ios.Out)
	cmd.SetErr(ios.ErrOut)
	cmd.SetArgs([]string{"facts.jsonl"})

	if err := cmd.Execute(); err == nil {
		t.Fatal("Execute = nil error, want a usage failure")
	}
}

// TestAStraddlingRuleCreditsEveryPartItCovers pins the build report's
// accounting for a total that spans its parts.
//
// The load-bearing assertion is parts_unchecked being EMPTY. The report's whole
// purpose is to separate figures the document corroborates from figures resting
// on our arithmetic alone, and before fisc-w0o a straddling block could only be
// reported as the second while actually being the first. Crediting one part and
// listing the other as "the rule declares no total_row" would be a false
// statement about the rule, printed by the thing whose job is to be believed.
func TestAStraddlingRuleCreditsEveryPartItCovers(t *testing.T) {
	root := testRepo(t, map[int]string{81: straddleHead, 82: straddleTail}, "spanning.yaml")
	opts, out, _ := testOptions(t, root)
	opts.JSON = true

	if err := buildRun(opts); err != nil {
		t.Fatalf("buildRun: %v", err)
	}
	var rep report
	if err := json.Unmarshal(out.Bytes(), &rep); err != nil {
		t.Fatalf("decode report: %v", err)
	}

	if len(rep.PartsUnchecked) != 0 {
		t.Errorf("parts_unchecked = %+v, want none; both parts are covered by the "+
			"printed total the rule declares it spans", rep.PartsUnchecked)
	}
	if rep.PartsChecked != 2 {
		t.Errorf("parts_checked = %d, want 2", rep.PartsChecked)
	}
	if rep.SpanningRulesChecked != 1 {
		t.Errorf("spanning_rules_checked = %d, want 1", rep.SpanningRulesChecked)
	}
	// One comparison, not two: the check ran once for the block. This is the
	// number that would silently double if a later change credited the columns
	// per part instead of per check.
	if rep.ColumnsTied != 1 {
		t.Errorf("columns_tied = %d, want 1; the rule is checked once, not once per part",
			rep.ColumnsTied)
	}
	if rep.ColumnsTiedByDeclaration != 0 {
		t.Errorf("columns_tied_by_declaration = %d, want 0; the block ties exactly",
			rep.ColumnsTiedByDeclaration)
	}
	if rep.Facts != 2 {
		t.Errorf("facts = %d, want 2", rep.Facts)
	}
}

// TestAStraddlingRuleWhoseTotalMovedFailsTheBuild is the other half: the flag
// is a declaration that one of the rule's pages prints the total, and a
// declaration that has stopped being true must fail the build rather than
// quietly downgrade itself to an unchecked part.
//
// Which guard catches it is deliberately not asserted. This fixture mirrors
// production, where the block's stop_at IS the total line (Budget Book p170),
// so moving the total trips the block boundary before the total lookup. Both
// are fail-closed and the choice between them is not a promise this test should
// freeze; that the build STOPS, and that nothing is written, is. The total
// lookup's own two failure modes are pinned in internal/mapping, where the rule
// can be mutated without moving the page.
func TestAStraddlingRuleWhoseTotalMovedFailsTheBuild(t *testing.T) {
	moved := strings.Replace(straddleTail, "Total Services", "Total Servcies", 1)
	root := testRepo(t, map[int]string{81: straddleHead, 82: moved}, "spanning.yaml")
	opts, out, errOut := testOptions(t, root)

	err := buildRun(opts)
	if err == nil {
		t.Fatal("buildRun succeeded; the page no longer prints the declared total row")
	}
	if !strings.Contains(err.Error(), "straddling-services") {
		t.Errorf("error = %v, want it to name the rule", err)
	}
	if out.Len() != 0 || errOut.Len() != 0 {
		t.Errorf("a failed build reported %q / %q; it must write nothing",
			out.String(), errOut.String())
	}
	if _, err := os.Stat(filepath.Join(root, defaultOutput)); !os.IsNotExist(err) {
		t.Errorf("the fact store exists after a failed build (%v); nothing is written "+
			"when a rule fails to resolve", err)
	}
}

// TestARollupIsReportedAsItsOwnClaim pins the build report's account of a total
// covering several rules.
//
// Two claims, kept apart. An ASSERTED rollup is the document checking work no
// single rule's total_row could check; a DECLARED-UNASSERTABLE one is a total
// the document prints that this structure cannot check, said out loud. Folding
// the second into the first would overstate the evidence; dropping it would be
// the silence fisc-3bl says is the one unacceptable answer.
func TestARollupIsReportedAsItsOwnClaim(t *testing.T) {
	root := testRepo(t, map[int]string{90: rollupPage}, "rollup.yaml")
	opts, out, _ := testOptions(t, root)
	opts.JSON = true

	if err := buildRun(opts); err != nil {
		t.Fatalf("buildRun: %v", err)
	}
	var rep report
	if err := json.Unmarshal(out.Bytes(), &rep); err != nil {
		t.Fatalf("decode report: %v", err)
	}

	if rep.RollupsAsserted != 1 {
		t.Errorf("rollups_asserted = %d, want 1", rep.RollupsAsserted)
	}
	if rep.RollupColumnsTied != 1 {
		t.Errorf("rollup_columns_tied = %d, want 1", rep.RollupColumnsTied)
	}
	want := []unassertedRollup{{ID: "total-sources", Page: 90,
		Reason: "exceeds the pages it closes by an amount that differs per column, " +
			"so nothing on them sums to it (fisc-wev)"}}
	if diff := cmp.Diff(want, rep.RollupsUnasserted); diff != "" {
		t.Errorf("rollups_unasserted mismatch (-want +got):\n%s", diff)
	}
	// The rollup does not inflate the per-part counters: those measure a
	// different link of the chain.
	if rep.PartsChecked != 2 || rep.ColumnsTied != 2 {
		t.Errorf("parts_checked = %d, columns_tied = %d, want 2 and 2",
			rep.PartsChecked, rep.ColumnsTied)
	}
}

// TestARollupThatDoesNotTieFailsTheBuild keeps the rollup a gate rather than a
// note, the same way a rule's own total_row is one.
func TestARollupThatDoesNotTieFailsTheBuild(t *testing.T) {
	moved := strings.Replace(rollupPage, "DEPARTMENT TOTAL $1,000", "DEPARTMENT TOTAL $1,001", 1)
	root := testRepo(t, map[int]string{90: moved}, "rollup.yaml")
	opts, _, _ := testOptions(t, root)

	err := buildRun(opts)
	if err == nil {
		t.Fatal("buildRun succeeded; the divisions state 1,000 and the page now prints 1,001")
	}
	for _, want := range []string{"dept-total", "$1,000.00", "$1,001.00"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %v, want it to name %s", err, want)
		}
	}
}

// monoGeometry is the geometry a monospaced page implies, in the extractor's
// format: each word at its character column, six points a character, one line
// every twelve points.
func monoGeometry(page int, text string) string {
	var words []string
	for i, line := range strings.Split(text, "\n") {
		for c := 0; c < len(line); {
			if line[c] == ' ' {
				c++
				continue
			}
			e := c
			for e < len(line) && line[e] != ' ' {
				e++
			}
			w, err := json.Marshal(line[c:e])
			if err != nil {
				panic(err)
			}
			words = append(words, fmt.Sprintf("[%d,%d,%d,%d,%s]", c*6, i*12, e*6, i*12+10, w))
			c = e
		}
	}
	return fmt.Sprintf(`{"doc_id": %q, "height": 792.0, "page": %d, "schema_version": 1, `+
		`"width": 612.0, "words": [%s]}`, docID, page, strings.Join(words, ","))
}

// TestASubtotalIsReportedAndCreditsItsPart: the build runs the subtotal
// chain, counts what it tied, and credits the part it checked rather than
// naming it unchecked for want of a total_row.
//
// Mutation: drop the chain loop from resolve, and the subtotal counters stay
// zero and the part is reported unchecked.
func TestASubtotalIsReportedAndCreditsItsPart(t *testing.T) {
	root := testRepoWithGeometry(t, map[int]string{95: subtotalPage},
		map[int]string{95: monoGeometry(95, subtotalPage)}, "subtotal.yaml")
	opts, out, _ := testOptions(t, root)
	opts.JSON = true

	if err := buildRun(opts); err != nil {
		t.Fatalf("buildRun: %v", err)
	}
	var rep report
	if err := json.Unmarshal(out.Bytes(), &rep); err != nil {
		t.Fatalf("decode report: %v", err)
	}
	if rep.SubtotalLinesTied != 1 || rep.SubtotalCellsTied != 1 {
		t.Errorf("subtotal_lines_tied = %d, subtotal_cells_tied = %d, want 1 and 1",
			rep.SubtotalLinesTied, rep.SubtotalCellsTied)
	}
	if rep.PartsChecked != 1 || len(rep.PartsUnchecked) != 0 {
		t.Errorf("parts_checked = %d, parts_unchecked = %+v, want the one part checked",
			rep.PartsChecked, rep.PartsUnchecked)
	}
	if rep.Facts != 2 {
		t.Errorf("facts = %d, want 2; the subtotal publishes nothing", rep.Facts)
	}
}

// TestASubtotalThatDoesNotTieFailsTheBuild keeps the subtotal a gate, as a
// rule's total_row is one.
func TestASubtotalThatDoesNotTieFailsTheBuild(t *testing.T) {
	moved := strings.Replace(subtotalPage, "SUBTOTAL  $ 30", "SUBTOTAL  $ 31", 1)
	root := testRepoWithGeometry(t, map[int]string{95: moved},
		map[int]string{95: monoGeometry(95, moved)}, "subtotal.yaml")
	opts, _, _ := testOptions(t, root)

	err := buildRun(opts)
	if err == nil {
		t.Fatal("buildRun succeeded; the rows sum to 30 and the page now prints 31")
	}
	for _, want := range []string{`"SUBTOTAL"`, "31", "$30.00"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %v, want it to name %s", err, want)
		}
	}
}
