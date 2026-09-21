package main

import (
	"os"
	"path/filepath"
	"strings"
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

func TestUnrecognisableFlagsAnIDTheScannerWouldSilentlyDrop(t *testing.T) {
	// bd chooses the token and this repo does not. A two-hyphen id like
	// fisc-drill-down is real in the export and invisible to citedIn, whose
	// boundary refuses a match a hyphen runs into -- so every citation to it
	// would be dropped with no report, inside the gate meant to report exactly
	// that. This assertion is what turns the arrival of such an id into a red
	// run instead of a blind spot.
	known := map[string]bool{
		"fisc-abc":        true,
		"fisc-abc.1":      true,
		"fisc-1wr.5.1":    true,
		"byob-layout.1":   true, // a row of this graph like any other, and seen whole
		"fisc-drill-down": true, // the shape citedIn drops
		"fisc-Abc":        true, // idPattern never matches an uppercase token
		"fisc-abc.x":      true, // citedIn returns only the fisc-abc prefix: not whole, not seen
	}
	want := []string{"fisc-Abc", "fisc-abc.x", "fisc-drill-down"}
	if diff := cmp.Diff(want, unrecognisable(known)); diff != "" {
		t.Errorf("unrecognisable (-want +got):\n%s", diff)
	}
}

func TestUnrecognisableIsSilentOnAnExportOfTodaysShapes(t *testing.T) {
	// Every id shape the export carries today, measured there: fisc-<token> with
	// optional dotted children, and byob ids whose slugs carry hyphens. A false
	// positive here would take every commit red for nothing.
	known := map[string]bool{
		"fisc-kc3j":     true,
		"fisc-yj4w.7":   true,
		"fisc-1wr.5.1":  true,
		"byob-layout.1": true,
		"byob-errors":   true,
	}
	if got := unrecognisable(known); got != nil {
		t.Errorf("unrecognisable(recognisable ids) = %v, want nil", got)
	}
}

func TestUnrecognisableJudgesTheRealExport(t *testing.T) {
	// The committed export is the artifact the gate resolves against, so its ids
	// are asserted here too: if bd mints a shape citedIn cannot see, this test
	// and the gate go red together, in the commit that lands the export.
	known, err := knownIDs(filepath.Join("..", "..", ".beads", "issues.jsonl"))
	if err != nil {
		t.Fatalf("knownIDs(committed export): %v", err)
	}
	if got := unrecognisable(known); got != nil {
		t.Errorf("the committed export carries ids the citation scanner cannot see: %v", got)
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
	// AGENTS.md cites the same handful of ids over and over, so a per-CITATION
	// shell-out would be a process per occurrence.
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

func TestAnExemptionIsKeptAliveOnlyByITSOWNFile(t *testing.T) {
	// A sighting anywhere else must not count. It did once, and the consequence
	// is that the token could vanish from the file the declaration names while
	// any other occurrence -- including this command's own declaration of it --
	// kept the exemption alive. Measured on the tree at the time: fisc-year was
	// declared as living in site/app.js and actually lives in the templates, and
	// nothing said so until this rule tightened.
	scanned := map[string]bool{}
	var elsewhere []ref
	for id, e := range exemptIDs {
		scanned[e.file] = true
		elsewhere = append(elsewhere, ref{id: id, file: "AGENTS.md", line: 1})
	}
	if _, err := dropExemptIDs(elsewhere, scanned); err == nil {
		t.Error("dropExemptIDs(sightings only in another file) = nil error, want a refusal")
	}
}

func TestEveryIDExemptionNamesAFileThatReallyCarriesIt(t *testing.T) {
	// The declaration is a claim about the tree like any other.
	root := filepath.Join("..", "..")
	for id, e := range exemptIDs {
		body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(e.file)))
		if err != nil {
			t.Errorf("exemption for %s: %v", id, err)
			continue
		}
		if !strings.Contains(string(body), id) {
			t.Errorf("exemption for %s says it lives in %s, which does not contain it", id, e.file)
		}
	}
}

func TestAnswersForRejectsAPrefixResolution(t *testing.T) {
	// bd resolves a unique prefix and exits 0, so the reply has to be checked
	// against the id that was asked for. This is a function of bytes precisely
	// so that it can be tested: inside bdKnows it could not be, and reducing it
	// to "did bd return anything" left every test green.
	reply := []byte(`[{"id":"fisc-i38","title":"x"}]`)
	if answersFor(reply, "fisc-i3") {
		t.Error(`answersFor(reply about fisc-i38, "fisc-i3") = true, want false`)
	}
	if !answersFor(reply, "fisc-i38") {
		t.Error(`answersFor(reply about fisc-i38, "fisc-i38") = false, want true`)
	}
	// bd absent, bd erroring, or a reply this cannot parse are all "no".
	for _, out := range [][]byte{nil, []byte("not json"), []byte(`[]`), []byte(`{"id":"fisc-i38"}`)} {
		if answersFor(out, "fisc-i38") {
			t.Errorf("answersFor(%q) = true, want false", out)
		}
	}
}

func TestDropExemptIDsRefusesADeclarationWhoseFileNoLongerCarriesIt(t *testing.T) {
	// An exemption whose token has gone is a declaration outliving its subject,
	// which is a hole nobody can see. Staleness is measured against the file the
	// declaration NAMES, so this reports a stale one only when that file was
	// actually read -- a narrower run is a narrower run.
	scanned := map[string]bool{}
	for _, e := range exemptIDs {
		scanned[e.file] = true
	}
	if _, err := dropExemptIDs(nil, scanned); err == nil {
		t.Error("dropExemptIDs(file read, token absent) = nil error, want a refusal")
	}
}

func TestDropExemptIDsIsSilentWhenTheDeclaredFileWasNotRead(t *testing.T) {
	// This is the case that made the first version wrong: it reported every
	// exemption stale on any run narrower than the Makefile's path list, and
	// following its advice would have broken the gate.
	if _, err := dropExemptIDs(nil, map[string]bool{}); err != nil {
		t.Errorf("dropExemptIDs(nothing read) = %v, want no error", err)
	}
}

func TestAnExemptionCannotSatisfyItselfFromItsOwnDeclaration(t *testing.T) {
	// The declaration is in a file this command scans, so every exempt id occurs
	// there by construction. Counting that as a sighting made the staleness
	// check unfalsifiable -- measured: a key renamed to a token in no other file
	// left a full run green.
	scanned := map[string]bool{declarationFile: true}
	var refs []ref
	for id, e := range exemptIDs {
		scanned[e.file] = true
		refs = append(refs, ref{id: id, file: declarationFile, line: 1})
	}
	if _, err := dropExemptIDs(refs, scanned); err == nil {
		t.Error("dropExemptIDs(sightings only in the declaration) = nil error, want a refusal")
	}

	// One real sighting in the file the declaration names is what it is for.
	for id, e := range exemptIDs {
		refs = append(refs, ref{id: id, file: e.file, line: 2})
	}
	if _, err := dropExemptIDs(refs, scanned); err != nil {
		t.Errorf("dropExemptIDs(sighting in the declared file) = %v, want no error", err)
	}
}

func TestDropExemptIDsKeepsEverythingItDoesNotDeclare(t *testing.T) {
	var refs []ref
	scanned := map[string]bool{}
	for id, e := range exemptIDs {
		refs = append(refs, ref{id: id, file: e.file, line: 1})
		scanned[e.file] = true
	}
	refs = append(refs, ref{id: "fisc-kc3j", file: "AGENTS.md", line: 2})

	kept, err := dropExemptIDs(refs, scanned)
	if err != nil {
		t.Fatalf("dropExemptIDs: %v", err)
	}
	want := []ref{{id: "fisc-kc3j", file: "AGENTS.md", line: 2}}
	if diff := cmp.Diff(want, kept, cmp.AllowUnexported(ref{})); diff != "" {
		t.Errorf("kept (-want +got):\n%s", diff)
	}
}

func TestScannableCoversWhatTheSiteAndTheToolingAreWrittenIn(t *testing.T) {
	// site/app.js and site/*.html.tmpl carry bead citations and are what
	// AGENTS.md calls the least-defended files in the repo; tools/extract.py is
	// the only Python. Every one was outside the first version of this filter.
	for _, p := range []string{
		"site/app.js", "site/index.html.tmpl", "site/style.css", "tools/extract.py",
		"AGENTS.md", "internal/x.go", "tools/jscheck/a.mjs", "data/funds.yaml", "ci.yml",
	} {
		if !scannable(p) {
			t.Errorf("scannable(%q) = false, want true", p)
		}
	}
	// A golden carries a prose note citing a bead, so .json is in.
	if !scannable("testdata/sankey.golden.json") {
		t.Error(`scannable("testdata/sankey.golden.json") = false, want true`)
	}
	// The page fixtures are the city's own printed text and cannot carry a claim
	// about this tracker.
	for _, p := range []string{
		"testdata/pages/p0067.txt", "data/pdf/x.pdf",
	} {
		if scannable(p) {
			t.Errorf("scannable(%q) = true, want false", p)
		}
	}
}

func TestRefsUnderScansAFileNamedDirectlyWhateverItIsCalled(t *testing.T) {
	// Makefile has no extension and is in the path list, so the extension filter
	// must apply to walking a DIRECTORY and not to an argument.
	dir := t.TempDir()
	named := filepath.Join(dir, "Makefile")
	if err := os.WriteFile(named, []byte("# fisc-named\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "walked"), []byte("fisc-walked\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	direct, _, err := refsUnder([]string{named})
	if err != nil {
		t.Fatalf("refsUnder(file): %v", err)
	}
	if len(direct) != 1 || direct[0].id != "fisc-named" {
		t.Errorf("refsUnder(%q) = %v, want the one citation it holds", named, direct)
	}
	walked, _, err := refsUnder([]string{dir})
	if err != nil {
		t.Fatalf("refsUnder(dir): %v", err)
	}
	for _, r := range walked {
		if r.id == "fisc-walked" {
			t.Error("walking picked up an extensionless file; the filter must still apply there")
		}
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
	refs, _, err := refsUnder([]string{dir})
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
		// A byob id is a row of this graph like any other, so a citation of one
		// resolves. THE WHOLE HYPHENATED SLUG IS THE ID: a two-word slug is one
		// id and not two, and a trailing hyphen is not part of it.
		{"byob-layout.1 is a decision", []string{"byob-layout.1"}},
		{"byob-command-shape.1 is one id", []string{"byob-command-shape.1"}},
		{"byob-layout -- and then", []string{"byob-layout"}},
		{"byob-release.8 and byob-release.10", []string{"byob-release.8", "byob-release.10"}},
		// Not a citation: something longer runs into it on one side.
		{"a-byob-layout.1 thing", nil},
		{"byob-layout.1x", nil},

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

func TestCheckExemptionsCoversTheIDDeclarationsToo(t *testing.T) {
	// The file arm and the id arm are separate loops over separate maps, and the
	// id arm could be deleted with every other test still green. It is
	// load-bearing: a renamed file under an id exemption is caught by nothing
	// else, because dropExemptIDs keys on scanned[e.file] and a path that has
	// gone is never in that set.
	if err := checkExemptions(t.TempDir()); err == nil {
		t.Fatal("checkExemptions(empty dir) = nil, want a refusal")
	}
	if len(exempt) == 0 || len(exemptIDs) == 0 {
		t.Skip("one of the two declarations is empty; this test cannot tell the arms apart")
	}
	// With only the FILE declarations satisfied, the id arm must still object.
	root := t.TempDir()
	touch(t, root, declarationFile)
	for file := range exempt {
		touch(t, root, file)
	}
	if err := checkExemptions(root); err == nil {
		t.Error("checkExemptions with only the file exemptions present = nil, want the id arm to object")
	}
	for _, e := range exemptIDs {
		touch(t, root, e.file)
	}
	if err := checkExemptions(root); err != nil {
		t.Errorf("checkExemptions with everything present = %v, want no error", err)
	}
}

func TestAnIDExemptionMayNotAnchorItselfToTheDeclaration(t *testing.T) {
	// Anchoring an exemption to the file the declaration is written in would let
	// the declaration be its own sighting, and the staleness check could never
	// fire again. It is refused where the message can explain it.
	saved := exemptIDs
	t.Cleanup(func() { exemptIDs = saved })
	exemptIDs = map[string]exemptID{
		"fisc-selfanchored": {file: declarationFile, what: "a token declared to live in the declaration"},
	}
	// Every OTHER declaration has to be satisfied, or checkExemptions errors out
	// of the file loop first and this passes on an unrelated message. That is
	// how the first version of this test could not fail at all.
	root := t.TempDir()
	for file := range exempt {
		touch(t, root, file)
	}
	touch(t, root, declarationFile)
	err := checkExemptions(root)
	if err == nil {
		t.Fatal("checkExemptions(exemption anchored to the declaration) = nil, want a refusal")
	}
	if !strings.Contains(err.Error(), "fisc-selfanchored") {
		t.Errorf("checkExemptions = %v, want a refusal naming fisc-selfanchored rather than something else", err)
	}
}

func TestTheDeclarationFileIsItselfAClaimAboutTheTree(t *testing.T) {
	// declarationFile is a hardcoded path in a tool whose principle is that a
	// stale declaration must fail. Renaming main.go would silently disarm the
	// self-anchor guard, since nothing else ever resolves that path.
	if _, err := os.Stat(filepath.Join("..", "..", filepath.FromSlash(declarationFile))); err != nil {
		t.Errorf("declarationFile %q: %v", declarationFile, err)
	}
}

func touch(t *testing.T, root, rel string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, nil, 0o600); err != nil {
		t.Fatal(err)
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
	if _, _, err := refsUnder([]string{t.TempDir()}); err != nil {
		t.Errorf("refsUnder does not check exemptions and must not: %v", err)
	}
}

func TestRefsUnderFailsOnAPathItCannotRead(t *testing.T) {
	// A path that cannot be read is a path that cannot be checked, and the
	// argument list is hand-maintained in the Makefile.
	if _, _, err := refsUnder([]string{filepath.Join(t.TempDir(), "nope")}); err == nil {
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
