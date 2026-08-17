package verify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/jcrussell/livermore-budget/internal/check"
	"github.com/jcrussell/livermore-budget/internal/fisccmd"
	"github.com/jcrussell/livermore-budget/internal/registry"
	"github.com/jcrussell/livermore-budget/pkg/cmdutil"
	"github.com/jcrussell/livermore-budget/pkg/iostreams"
)

// These tests are about this command and nothing else: the exit code it returns for
// each mix of verdicts, the two output shapes, and the flags it passes down. Both
// seams are used — a loader that hands back a subject nothing reads, and a check set
// the test chooses — because the alternative is contriving a corpus that produces a
// failure, an error and a skip, and internal/check already owns testing verdicts
// against real inputs (including a fact store this suite could not build: see the
// input-mutation tests there).

// noVocabulary is the registry view of an empty corpus. check.Subject documents
// Vocabulary as required, and the checks read it without a nil guard, so a test that
// runs the real check set has to supply one.
type noVocabulary struct{}

func (noVocabulary) Assignable(string) bool { return false }
func (noVocabulary) Category(string) (registry.Category, bool) {
	return registry.Category{}, false
}
func (noVocabulary) FundGroup(string) bool          { return false }
func (noVocabulary) Fund(int) (registry.Fund, bool) { return registry.Fund{}, false }
func (noVocabulary) Funds() []registry.Fund         { return nil }

var _ check.Vocabulary = noVocabulary{}

// emptySubject is a corpus with nothing in it, which is what this package's tests
// hand the checks: the fakes below ignore it, and the one test that runs the real
// set only counts how many results came back.
func emptySubject() *check.Subject {
	return &check.Subject{Vocabulary: noVocabulary{}}
}

// fake is a check whose verdict the test picks.
type fake struct {
	id   string
	full bool
	res  check.Result
	err  error
}

func (f *fake) ID() string          { return f.id }
func (*fake) Tier() int             { return 1 }
func (f *fake) Description() string { return "the " + f.id + " claim" }
func (f *fake) Full() bool          { return f.full }

func (f *fake) Run(context.Context, *check.Subject) (check.Result, error) {
	return f.res, f.err
}

var _ check.Check = (*fake)(nil)

func passing(id string) *fake {
	return &fake{id: id, res: check.Result{Status: check.StatusPass, Subjects: 7,
		Summary: "7 things, every one holding"}}
}

func failing(id string) *fake {
	return &fake{id: id, res: check.Result{Status: check.StatusFail, Subjects: 7,
		Summary:  "1 finding over 7 things",
		Findings: []check.Finding{{Subject: "fisc-f-0123456789ab", Detail: "is $3.00, want $4.00"}}}}
}

func vacuous(id string) *fake {
	return &fake{id: id, res: check.Result{Status: check.StatusVacuous,
		Summary: "no link carries a transfer_id"}}
}

func fullOnly(id string) *fake {
	return &fake{id: id, full: true, res: check.Result{Status: check.StatusPass, Subjects: 1}}
}

// runVerify executes the command through the runner that owns exit codes, so what
// these tests assert is what a script sees.
func runVerify(t *testing.T, checks []check.Check, args ...string) (int, string, string) {
	t.Helper()
	ios, _, out, errOut := iostreams.Test()
	f := &cmdutil.Factory{
		IOStreams: ios,
		RepoRoot:  func() (string, error) { return t.TempDir(), nil },
	}
	// runF takes the parsed Options, attaches the test's seams, and calls the real
	// run function: everything below Load is the shipped code path.
	cmd := NewCmdVerify(f, func(o *Options) error {
		o.Load = func(check.LoadOptions) (*check.Subject, error) { return emptySubject(), nil }
		o.Checks = func() []check.Check { return checks }
		return verifyRun(t.Context(), o)
	})
	cmd.SilenceErrors, cmd.SilenceUsage = true, true

	return fisccmd.Run(cmd, args, ios), out.String(), errOut.String()
}

// TestACleanReportExitsZero is the baseline: nothing wrong, nothing on stdout,
// because the report is chatter and only --json is data.
func TestACleanReportExitsZero(t *testing.T) {
	code, out, errOut := runVerify(t, []check.Check{passing("facts-sorted"), passing("graph-acyclic")})

	if code != 0 {
		t.Errorf("exit code = %d, want 0\n%s", code, errOut)
	}
	if out != "" {
		t.Errorf("stdout = %q, want it empty without --json", out)
	}
	if want := "2 passed, 0 failed, 0 vacuous (nothing to check), 0 skipped, 0 errored"; !strings.Contains(errOut, want) {
		t.Errorf("report does not contain the tally %q:\n%s", want, errOut)
	}
	if strings.Contains(errOut, "verify failed") {
		t.Errorf("a clean run says it failed:\n%s", errOut)
	}
}

// TestVacuousExitsZeroAndThreeUnderStrict is the exit-code contract for checks that
// had nothing to look at. It exits 0, because a gate that cannot go green until
// mapping finishes is a gate that gets commented out — and it says on its own output
// that those checks were not passes.
func TestVacuousExitsZeroAndThreeUnderStrict(t *testing.T) {
	checks := []check.Check{passing("facts-sorted"), vacuous("transfer-legs-pair")}

	code, _, errOut := runVerify(t, checks)
	if code != 0 {
		t.Errorf("exit code = %d without --strict, want 0\n%s", code, errOut)
	}
	if !strings.Contains(errOut, "VACUOUS") {
		t.Errorf("the report never says VACUOUS:\n%s", errOut)
	}
	if want := "1 passed, 0 failed, 1 vacuous (nothing to check), 0 skipped, 0 errored"; !strings.Contains(errOut, want) {
		t.Errorf("the report does not contain the tally %q:\n%s", want, errOut)
	}
	if !strings.Contains(errOut, "had nothing to check; that is not a pass") {
		t.Errorf("the report does not distinguish a vacuous check from a pass:\n%s", errOut)
	}
	// The claim is printed for a non-pass, because "VACUOUS" alone does not tell a
	// reader what did not happen.
	if !strings.Contains(errOut, "claims: the transfer-legs-pair claim") {
		t.Errorf("the report does not state the claim of a vacuous check:\n%s", errOut)
	}

	code, _, errOut = runVerify(t, checks, "--strict")
	if code != 3 {
		t.Errorf("exit code = %d under --strict, want 3\n%s", code, errOut)
	}
	if !strings.Contains(errOut, "verify failed: --strict") {
		t.Errorf("a failing --strict run does not say why it failed:\n%s", errOut)
	}
}

// TestFailingCheckExitsThreeAndNamesTheSubject: 3 is the code that says the report
// is not clean, and a report that does not name what is wrong is not actionable.
func TestFailingCheckExitsThreeAndNamesTheSubject(t *testing.T) {
	code, _, errOut := runVerify(t, []check.Check{
		passing("facts-sorted"), failing("link-values-tie-to-facts"),
	})

	if code != 3 {
		t.Fatalf("exit code = %d, want 3\n%s", code, errOut)
	}
	for _, want := range []string{"FAIL", "link-values-tie-to-facts", "fisc-f-0123456789ab", "want $4.00"} {
		if !strings.Contains(errOut, want) {
			t.Errorf("the report does not contain %q:\n%s", want, errOut)
		}
	}
	if !strings.Contains(errOut, "verify failed") {
		t.Errorf("a failing run does not say it failed:\n%s", errOut)
	}
	// Exit 3 is a verdict, not an error: the runner must not append its own
	// "error:" line to a report that has already explained itself.
	if strings.Contains(errOut, "error: exit code") {
		t.Errorf("the runner printed the exit code as an error:\n%s", errOut)
	}
}

// TestAnErroringCheckAlsoExitsThreeAndIsDistinguishable: both mean read the report,
// and the report has to say which happened.
func TestAnErroringCheckAlsoExitsThreeAndIsDistinguishable(t *testing.T) {
	code, _, errOut := runVerify(t, []check.Check{
		passing("facts-sorted"),
		&fake{id: "aggregation-invariance", err: errors.New("the fold is not implemented")},
	})

	if code != 3 {
		t.Fatalf("exit code = %d, want 3\n%s", code, errOut)
	}
	for _, want := range []string{"ERROR", "aggregation-invariance", "the fold is not implemented"} {
		if !strings.Contains(errOut, want) {
			t.Errorf("the report does not contain %q:\n%s", want, errOut)
		}
	}
	if strings.Contains(errOut, "FAIL") {
		t.Errorf("an erroring check is reported as a failure:\n%s", errOut)
	}
}

// TestSkippedIsReportedAndDoesNotFail wires --full end to end. No check needs it
// today, so this is where the path is exercised before one does.
func TestSkippedIsReportedAndDoesNotFail(t *testing.T) {
	checks := []check.Check{passing("facts-sorted"), fullOnly("manifest-sha256")}

	code, _, errOut := runVerify(t, checks)
	if code != 0 {
		t.Errorf("exit code = %d for a skipped check, want 0\n%s", code, errOut)
	}
	if !strings.Contains(errOut, "SKIPPED") {
		t.Errorf("the report never says SKIPPED:\n%s", errOut)
	}
	if !strings.Contains(errOut, "need --full and did not run") {
		t.Errorf("the report does not say what would have run it:\n%s", errOut)
	}
	// Not even under --strict: a skipped check is the documented consequence of a
	// flag, not a gap in the corpus.
	if code, _, _ := runVerify(t, checks, "--strict"); code != 0 {
		t.Errorf("exit code = %d under --strict for a skipped check, want 0", code)
	}
}

// TestLoadFailureExitsOne separates a broken harness from a wrong corpus. A missing
// fact store is not a failed check, and reporting it as one would send someone
// looking for a bad figure.
func TestLoadFailureExitsOne(t *testing.T) {
	ios, _, out, errOut := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios, RepoRoot: func() (string, error) { return t.TempDir(), nil }}
	cmd := NewCmdVerify(f, func(o *Options) error {
		o.Load = func(check.LoadOptions) (*check.Subject, error) {
			return nil, errors.New("read the fact store: no such file or directory")
		}
		o.Checks = func() []check.Check {
			t.Error("the checks ran after the corpus failed to load")
			return nil
		}
		return verifyRun(t.Context(), o)
	})
	cmd.SilenceErrors, cmd.SilenceUsage = true, true

	if code := fisccmd.Run(cmd, nil, ios); code != 1 {
		t.Errorf("exit code = %d for a load failure, want 1", code)
	}
	if !strings.Contains(errOut.String(), "read the fact store") {
		t.Errorf("stderr %q does not say why nothing could be checked", errOut)
	}
	if out.String() != "" {
		t.Errorf("stdout = %q, want nothing: there is no report", out)
	}
	if strings.Contains(errOut.String(), "PASS") || strings.Contains(errOut.String(), "VACUOUS") {
		t.Errorf("a load failure printed a report:\n%s", errOut)
	}
}

// TestJSONRoundTripsWithEveryKeyPresent covers the machine-readable form. The shape
// is the contract: every key present on every result, no omitempty, so a diff of two
// releases' reports reads as data rather than as a schema change.
func TestJSONRoundTripsWithEveryKeyPresent(t *testing.T) {
	checks := []check.Check{passing("facts-sorted"), failing("link-values-tie-to-facts"),
		vacuous("transfer-legs-pair"), fullOnly("manifest-sha256")}
	code, out, errOut := runVerify(t, checks, "--json")

	if code != 3 {
		t.Fatalf("exit code = %d, want 3", code)
	}
	// --json is the machine-readable form, so it replaces the report rather than
	// accompanying it.
	if errOut != "" {
		t.Errorf("stderr = %q, want it empty under --json", errOut)
	}

	var top map[string]json.RawMessage
	if err := json.Unmarshal([]byte(out), &top); err != nil {
		t.Fatalf("decode the report: %v\n%s", err, out)
	}
	if diff := cmp.Diff([]string{"counts", "full", "generated_by", "results", "strict"},
		sortedKeys(top)); diff != "" {
		t.Errorf("top-level keys (-want +got):\n%s", diff)
	}

	var counts map[string]json.RawMessage
	if err := json.Unmarshal(top["counts"], &counts); err != nil {
		t.Fatalf("decode counts: %v", err)
	}
	if diff := cmp.Diff([]string{"error", "fail", "pass", "skipped", "vacuous"},
		sortedKeys(counts)); diff != "" {
		t.Errorf("counts keys (-want +got):\n%s", diff)
	}

	var results []map[string]json.RawMessage
	if err := json.Unmarshal(top["results"], &results); err != nil {
		t.Fatalf("decode results: %v", err)
	}
	if len(results) != len(checks) {
		t.Errorf("results = %d, want one per check (%d)", len(results), len(checks))
	}
	for _, res := range results {
		if diff := cmp.Diff([]string{
			"check_id", "delta_cents", "description", "findings", "reconciliation_id",
			"status", "subjects", "summary", "tier",
		}, sortedKeys(res)); diff != "" {
			t.Errorf("result keys (-want +got):\n%s", diff)
		}
		if string(res["findings"]) == "null" {
			t.Errorf("%s: findings is null, want []", res["check_id"])
		}
	}

	// A round trip through the typed report has to come back byte-identical: the
	// document a consumer parses is the document this command wrote.
	var typed check.Report
	if err := json.Unmarshal([]byte(out), &typed); err != nil {
		t.Fatalf("decode into check.Report: %v", err)
	}
	var again bytes.Buffer
	enc := json.NewEncoder(&again)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(&typed); err != nil {
		t.Fatalf("re-encode: %v", err)
	}
	if diff := cmp.Diff(out, again.String()); diff != "" {
		t.Errorf("the report does not round-trip (-first +second):\n%s", diff)
	}
}

// TestTheDefaultChecksAreTheRealOnes keeps the seams from becoming the behaviour: a
// command with neither seam set must run every published check.
func TestTheDefaultChecksAreTheRealOnes(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios, RepoRoot: func() (string, error) { return t.TempDir(), nil }}

	var got *Options
	cmd := NewCmdVerify(f, func(o *Options) error {
		got = o
		return nil
	})
	cmd.SetOut(ios.Out)
	cmd.SetErr(ios.ErrOut)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got.Checks != nil {
		t.Error("Options.Checks is set before the run function defaults it")
	}
	if got.Load != nil {
		t.Error("Options.Load is set before the run function defaults it")
	}

	// And the default set is check.All: run it over an empty subject and count the
	// results, which is the only observable difference between All and any subset.
	ios, _, _, errOut := iostreams.Test()
	opts := &Options{
		IO:       ios,
		RepoRoot: func() (string, error) { return t.TempDir(), nil },
		Load:     func(check.LoadOptions) (*check.Subject, error) { return emptySubject(), nil },
	}
	_ = verifyRun(t.Context(), opts)
	for _, c := range check.All() {
		if !strings.Contains(errOut.String(), c.ID()) {
			t.Errorf("the default run does not report %s:\n%s", c.ID(), errOut)
		}
	}
}

// TestVerifyPassesItsFlagsToLoad: --full is the only flag Load reads, and the
// version it is given is what every projection stamps as metadata.generated_by.
func TestVerifyPassesItsFlagsToLoad(t *testing.T) {
	for _, full := range []bool{false, true} {
		ios, _, _, _ := iostreams.Test()
		var got check.LoadOptions
		opts := &Options{
			IO:       ios,
			RepoRoot: func() (string, error) { return "/repo", nil },
			Full:     full,
			Load: func(o check.LoadOptions) (*check.Subject, error) {
				got = o
				return &check.Subject{Full: o.Full, Vocabulary: noVocabulary{}}, nil
			},
			Checks: func() []check.Check { return []check.Check{passing("facts-sorted")} },
		}
		if err := verifyRun(t.Context(), opts); err != nil {
			t.Fatalf("verifyRun: %v", err)
		}
		if got.Full != full {
			t.Errorf("Load was asked for Full=%v, want %v", got.Full, full)
		}
		if got.Root != "/repo" {
			t.Errorf("Load was given root %q, want the repository root", got.Root)
		}
		if got.Version == "" {
			t.Error("Load was given no version, so every projection would refuse to build")
		}
	}
}

// TestFullIsReportedFromTheSubject: what decides whether a check ran is what the
// loader supplied, so that is what the report states.
func TestFullIsReportedFromTheSubject(t *testing.T) {
	ios, _, out, _ := iostreams.Test()
	opts := &Options{
		IO:       ios,
		RepoRoot: func() (string, error) { return "/repo", nil },
		Full:     true,
		JSON:     true,
		Load: func(o check.LoadOptions) (*check.Subject, error) {
			return &check.Subject{Full: o.Full, Vocabulary: noVocabulary{}}, nil
		},
		Checks: func() []check.Check { return []check.Check{fullOnly("manifest-sha256")} },
	}
	if err := verifyRun(t.Context(), opts); err != nil {
		t.Fatalf("verifyRun: %v", err)
	}

	var rep check.Report
	if err := json.Unmarshal(out.Bytes(), &rep); err != nil {
		t.Fatalf("decode the report: %v", err)
	}
	if !rep.Full {
		t.Error("the report does not record that --full was given")
	}
	if rep.Counts.Skipped != 0 || rep.Counts.Pass != 1 {
		t.Errorf("counts = %+v, want the full-only check to have run", rep.Counts)
	}
}

// TestNewCmdVerifyParsesItsFlags exercises the parsing layer alone: runF takes the
// test path, so nothing is loaded and nothing is checked.
func TestNewCmdVerifyParsesItsFlags(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	f := &cmdutil.Factory{
		IOStreams: ios,
		RepoRoot: func() (string, error) {
			t.Error("RepoRoot resolved during flag parsing")
			return "", nil
		},
	}

	var got *Options
	cmd := NewCmdVerify(f, func(o *Options) error {
		got = o
		return nil
	})
	cmd.SetOut(ios.Out)
	cmd.SetErr(ios.ErrOut)
	cmd.SetArgs([]string{"--json", "--full", "--strict"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got == nil {
		t.Fatal("runF was not called")
	}
	want := Options{JSON: true, Full: true, Strict: true}
	if diff := cmp.Diff(want, Options{JSON: got.JSON, Full: got.Full, Strict: got.Strict}); diff != "" {
		t.Errorf("parsed flags mismatch (-want +got):\n%s", diff)
	}
	// The group is what puts verify under "Data commands" in `fisc --help`; cobra
	// panics at AddCommand time if it names a group root does not define.
	if got, want := cmd.GroupID, "data"; got != want {
		t.Errorf("GroupID = %q, want %q", got, want)
	}
}

// TestNewCmdVerifyRejectsPositionalArgs is the class of bug that let
// `./bin/fisc biuld` exit 0 while printing help. `fisc verify facts-sorted` must not
// silently run everything.
func TestNewCmdVerifyRejectsPositionalArgs(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	cmd := NewCmdVerify(&cmdutil.Factory{IOStreams: ios}, func(*Options) error {
		t.Error("the command ran with a stray positional argument")
		return nil
	})
	cmd.SetOut(ios.Out)
	cmd.SetErr(ios.ErrOut)
	cmd.SetArgs([]string{"facts-sorted"})

	if err := cmd.Execute(); err == nil {
		t.Fatal("Execute = nil error, want a usage failure")
	}
}

// TestUnknownFlagExitsTwo keeps the third exit code in the contract covered: a flag
// this command does not know is a usage error, not a failed check.
func TestUnknownFlagExitsTwo(t *testing.T) {
	code, _, errOut := runVerify(t, nil, "--stricter")
	if code != 2 {
		t.Errorf("exit code = %d for an unknown flag, want 2", code)
	}
	if !strings.Contains(errOut, "stricter") {
		t.Errorf("stderr %q does not name the offending flag", errOut)
	}
}

// TestReportColumnsLineUp is not cosmetic: the status column is what a reader scans
// for anything that is not PASS, and it only works if every line starts one.
func TestReportColumnsLineUp(t *testing.T) {
	_, _, errOut := runVerify(t, []check.Check{
		passing("a"), failing("bbbbbbbbbbbbbbbbbbbb"), vacuous("cc"),
	})

	statuses := map[string]bool{"PASS": true, "FAIL": true, "VACUOUS": true, "SKIPPED": true, "ERROR": true}
	for _, line := range strings.Split(strings.TrimSpace(errOut), "\n") {
		word, rest, _ := strings.Cut(line, " ")
		if !statuses[word] {
			continue // a claim, a finding, or the tally
		}
		if !strings.HasPrefix(rest, strings.Repeat(" ", 9-len(word)-1)) {
			t.Errorf("line %q does not start with a padded status column", line)
		}
	}
	if got := strings.Count(errOut, "FAIL"); got != 1 {
		t.Errorf("the word FAIL appears %d times, want once: %s", got, errOut)
	}
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}
