package check

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// committedVerdictsGolden is every registered check's verdict over the
// committed corpus, one check per line sorted by id. Its counts are re-measured
// by TestCommittedVerdicts on every run, so a check dropped from [All] or a
// population that shrinks without failing anything is a diff here.
const committedVerdictsGolden = "committed-verdicts.json"

// verdict is the part of a [Result] the golden pins: what verify reports per
// check, less the prose.
type verdict struct {
	CheckID  string `json:"check_id"`
	Tier     int    `json:"tier"`
	Status   Status `json:"status"`
	Subjects int    `json:"subjects"`
}

// verdictLines renders rep as the golden's bytes.
func verdictLines(t *testing.T, rep *Report) []byte {
	t.Helper()
	vs := make([]verdict, 0, len(rep.Results))
	for _, r := range rep.Results {
		vs = append(vs, verdict{r.CheckID, r.Tier, r.Status, r.Subjects})
	}
	slices.SortFunc(vs, func(a, b verdict) int { return strings.Compare(a.CheckID, b.CheckID) })
	lines := make([]string, 0, len(vs))
	for _, v := range vs {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		lines = append(lines, "  "+string(b))
	}
	return []byte("[\n" + strings.Join(lines, ",\n") + "\n]\n")
}

// TestCommittedVerdicts holds internal/check/testdata/committed-verdicts.json
// to the report every check in [All] gives over the committed corpus. Deleting
// a check from [All], or narrowing a check's population, leaves verify green
// and fails here. On a mismatch the actual report is written to bin/ and the
// failure prints the cp that regenerates the golden.
func TestCommittedVerdicts(t *testing.T) {
	t.Parallel()
	rep := Run(t.Context(), committed(t), All(), ReportOptions{GeneratedBy: testVersion})
	got := verdictLines(t, rep)

	root := repoRoot(t)
	golden := filepath.Join(root, "internal", "check", "testdata", committedVerdictsGolden)
	want, err := os.ReadFile(golden) // #nosec G304 -- testdata/.
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("read the golden: %v", err)
	}
	if bytes.Equal(got, want) {
		return
	}
	t.Errorf("internal/check/testdata/%s is not the committed corpus's verdicts (-golden +got):\n%s",
		committedVerdictsGolden,
		cmp.Diff(strings.Split(string(want), "\n"), strings.Split(string(got), "\n")))
	out := filepath.Join(root, "bin", committedVerdictsGolden)
	if err := os.MkdirAll(filepath.Dir(out), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(out, got, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Logf("the actual verdicts are in %s -- regenerate with\n  cp -f bin/%s internal/check/testdata/%s\nand read the diff before committing it",
		out, committedVerdictsGolden, committedVerdictsGolden)
}
