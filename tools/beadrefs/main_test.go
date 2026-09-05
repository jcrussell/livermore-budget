package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestKnownIDsReadsTheExport(t *testing.T) {
	path := write(t, "issues.jsonl", `{"id":"fisc-abc","title":"x"}
{"id":"fisc-abc.1","title":"y"}
{"id":"byob-layout.1","title":"z"}

{"title":"a record with no id at all"}
`)
	known, err := knownIDs(path)
	if err != nil {
		t.Fatalf("knownIDs: %v", err)
	}
	want := map[string]bool{"fisc-abc": true, "fisc-abc.1": true, "byob-layout.1": true}
	if diff := cmp.Diff(want, known); diff != "" {
		t.Errorf("known (-want +got):\n%s", diff)
	}
}

func TestKnownIDsRefusesAnExportWithNoIDs(t *testing.T) {
	// An empty or id-less export would make EVERY citation in the tree look
	// dead, which is a wall of false findings rather than a useful failure --
	// and a truncated export is the likelier cause than a real regression.
	for name, body := range map[string]string{
		"empty":     "",
		"no ids":    "{\"title\":\"x\"}\n",
		"blanks":    "\n\n\n",
		"malformed": "not json\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := knownIDs(write(t, "issues.jsonl", body)); err == nil {
				t.Errorf("knownIDs(%q) = nil error, want a refusal", body)
			}
		})
	}
}

func TestRefsInFindsEveryCitationWithItsLine(t *testing.T) {
	path := write(t, "doc.md", `one fisc-abc here
nothing on this line
two on one: fisc-def and fisc-ghi.2
`)
	got, err := refsIn(path)
	if err != nil {
		t.Fatalf("refsIn: %v", err)
	}
	want := []ref{
		{id: "fisc-abc", file: path, line: 1},
		{id: "fisc-def", file: path, line: 3},
		{id: "fisc-ghi.2", file: path, line: 3},
	}
	if diff := cmp.Diff(want, got, cmp.AllowUnexported(ref{})); diff != "" {
		t.Errorf("refs (-want +got):\n%s", diff)
	}
}

func TestResolveReportsOnlyWhatNeitherSourceKnows(t *testing.T) {
	known := map[string]bool{"fisc-real": true}
	refs := []ref{
		{id: "fisc-real", file: "b.md", line: 2},
		{id: "fisc-new", file: "b.md", line: 1},
		{id: "fisc-dead", file: "a.md", line: 9},
	}
	asked := map[string]int{}
	// fisc-new is the export-lag case: filed, not yet exported, and bd knows it.
	secondOpinion := func(id string) bool { asked[id]++; return id == "fisc-new" }

	got := resolve(refs, known, secondOpinion)
	want := []ref{{id: "fisc-dead", file: "a.md", line: 9}}
	if diff := cmp.Diff(want, got, cmp.AllowUnexported(ref{})); diff != "" {
		t.Errorf("dead (-want +got):\n%s", diff)
	}
	if asked["fisc-real"] != 0 {
		t.Errorf("asked bd about an id the export knows %d times, want 0", asked["fisc-real"])
	}
}

func TestResolveAsksOncePerDistinctID(t *testing.T) {
	// AGENTS.md is 1,136 lines citing the same handful of ids repeatedly, so a
	// per-CITATION shell-out would be a process per occurrence.
	refs := []ref{
		{id: "fisc-dead", file: "a.md", line: 1},
		{id: "fisc-dead", file: "a.md", line: 2},
		{id: "fisc-dead", file: "b.md", line: 3},
	}
	asked := 0
	got := resolve(refs, map[string]bool{}, func(string) bool { asked++; return false })
	if asked != 1 {
		t.Errorf("asked %d times, want 1", asked)
	}
	if len(got) != 3 {
		t.Errorf("reported %d citations, want 3: every occurrence is its own finding", len(got))
	}
}

func TestResolveWithNoSecondOpinionDecidesOnTheExportAlone(t *testing.T) {
	// This is CI, where bd never exists. A miss must be a failure there, not a
	// silent pass -- the export is the only artifact CI has.
	got := resolve(
		[]ref{{id: "fisc-dead", file: "a.md", line: 1}},
		map[string]bool{},
		func(string) bool { return false },
	)
	if len(got) != 1 {
		t.Errorf("reported %d, want 1", len(got))
	}
}

func TestRefsUnderWalksAndFiltersByExtension(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{
		"a.md":          "fisc-md\n",
		"b.go":          "// fisc-go\n",
		"c.mjs":         "// fisc-mjs\n",
		"d.txt":         "fisc-txt\n",
		"sub/e.yaml":    "rule: fisc-yaml\n",
		"sub/f.golden":  "fisc-golden\n",
		"sub/deep/g.md": "fisc-deep\n",
	} {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	refs, err := refsUnder([]string{dir})
	if err != nil {
		t.Fatalf("refsUnder: %v", err)
	}
	var ids []string
	for _, r := range refs {
		ids = append(ids, r.id)
	}
	// .txt and .golden carry fixture text rather than claims about the tracker.
	want := []string{"fisc-deep", "fisc-go", "fisc-md", "fisc-mjs", "fisc-yaml"}
	sortStrings(ids)
	if diff := cmp.Diff(want, ids); diff != "" {
		t.Errorf("ids (-want +got):\n%s", diff)
	}
}

func TestCitedInTellsABeadIDFromTheOtherThingsNamedfisc(t *testing.T) {
	// `fisc-` is an overloaded prefix in this tree rather than a namespace, and
	// every one of these was a false finding on the first run over the tree.
	tests := []struct {
		line string
		want []string
	}{
		{"see fisc-kc3j for the lane", []string{"fisc-kc3j"}},
		{"`fisc-yj4w.7` declines two tier sets.", []string{"fisc-yj4w.7"}},
		{"fisc-1wr.5.1 is deferred", []string{"fisc-1wr.5.1"}},
		{"the pair (fisc-abc, fisc-def)", []string{"fisc-abc", "fisc-def"}},
		// The period ending a sentence is not part of the id, but a dotted
		// child's is; the pattern has already taken the second when it applies.
		{"see fisc-abc.", []string{"fisc-abc"}},
		// byob ids are reference material and are not this command's subject.
		{"byob-layout.1 is a decision", nil},

		// A fact id and a series id: the SECOND hyphen is what says so.
		{`"fact_id": "fisc-f-0ba7b00dfaf7"`, nil},
		{`"series_id": "fisc-s-ce9117881328"`, nil},
		{`link.FactIDs = []string{"fisc-f-notafact"}`, nil},
		{"const IDPrefix = \"fisc-f-\"", nil},

		// A marker file and a scratch directory: the leading dot says so.
		{`const ExportMarkerName = ".fisc-export"`, nil},
		{`os.MkdirTemp(target, ".fisc-write-probe-*")`, nil},
		{"every fixedPaths key -- .fisc-export, app.js", nil},

		// Neither end may run into a word.
		{"prefisc-abc", nil},
		{"fisc-abcZ", nil},
	}
	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			if diff := cmp.Diff(tt.want, citedIn(tt.line)); diff != "" {
				t.Errorf("citedIn(%q) (-want +got):\n%s", tt.line, diff)
			}
		})
	}
}

func TestEveryExemptionNamesAFileThatExists(t *testing.T) {
	// An exemption that outlives what it exempts is a hole nobody can see. The
	// staleness test is the FILESYSTEM and not the walk, because a run over a
	// narrower path set is a narrower run rather than a stale declaration --
	// so this passes a path list that reaches none of them.
	if err := checkExemptions(filepath.Join("..", "..")); err != nil {
		t.Errorf("checkExemptions(repo root) = %v, want no error", err)
	}
	if err := checkExemptions(t.TempDir()); err == nil {
		t.Error("checkExemptions(empty dir) = nil, want a refusal: the declaration has nothing to exempt")
	}
	if _, err := refsUnder([]string{t.TempDir()}); err != nil {
		t.Errorf("refsUnder does not check exemptions and must not: %v", err)
	}
}

func TestRefsUnderFailsOnAPathItCannotRead(t *testing.T) {
	// A path that cannot be read is a path that cannot be checked, and the
	// argument list is hand-maintained in the Makefile.
	if _, err := refsUnder([]string{filepath.Join(t.TempDir(), "nope")}); err == nil {
		t.Error("refsUnder(missing path) = nil error, want a failure")
	}
}

func write(t *testing.T, name, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func sortStrings(s []string) {
	for i := range s {
		for j := i + 1; j < len(s); j++ {
			if s[j] < s[i] {
				s[i], s[j] = s[j], s[i]
			}
		}
	}
}
