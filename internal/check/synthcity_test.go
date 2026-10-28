package check

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/project"
	"github.com/jcrussell/livermore-budget/internal/registry"
	"github.com/jcrussell/livermore-budget/internal/structure"
	"github.com/jcrussell/livermore-budget/internal/synthcity"
)

// synthSubject is the Subject over synthcity.Default's facts, read with the
// real registries. The document's structure is the Budget Book's, so its
// splits, identities and restatements are; its exceptions, which pin the
// committed corpus's figures, are the city's own, declared for its Gaps.
func synthSubject(t *testing.T) *Subject {
	t.Helper()
	reg, err := registry.Load(os.DirFS(filepath.Join("..", "..", "data")))
	if err != nil {
		t.Fatal(err)
	}
	city, err := synthcity.Default().Facts(reg)
	if err != nil {
		t.Fatal(err)
	}
	facts, exceptions := city.Facts, city.Exceptions
	if err = structure.ValidateExceptions(exceptions); err != nil {
		t.Fatalf("the city declares an exception structure refuses: %v", err)
	}
	fact.Sort(facts)
	projections, failures, err := buildProjections(project.Registry(reg, exceptions), facts, testVersion)
	if err != nil {
		t.Fatal(err)
	}
	return &Subject{
		Facts: facts, Vocabulary: reg, Projections: projections, ProjectionFailures: failures,
		Published:    project.PublishedDocuments(),
		Splits:       structure.BudgetBookSplits(),
		Identities:   structure.BudgetBookIdentities(),
		Restatements: structure.BudgetBookRestatements(),
		Residue:      append(fundBalanceResidue(), city.Residue...),
		Exceptions:   exceptions,
	}
}

// fundBalanceResidue is the Budget Book's residue on pp.186-209, the CIP
// Funds block the city prints; the rest names schedules it does not.
func fundBalanceResidue() []structure.Residue {
	var out []structure.Residue
	for _, r := range structure.BudgetBookResidue() {
		if r.Scope == structure.ScopeFundBalancesByFund {
			out = append(out, r)
		}
	}
	return out
}

// corpusChecks are the checks that read what only the committed corpus has,
// each with what it reads; over the synthetic city they say nothing about
// the city.
var corpusChecks = map[string]string{
	"fact-offset-points-at-token":        "the extracted page text a fact's offset lands in",
	"fact-citations-are-declared":        "the rule files under mappings/ that declare a shared citation",
	"fact-revenue-lines-resolve":         "every data/taxonomy.yaml line being printed by some fact",
	"published-projection-built":         "the ACFR, CIP and debt documents the site publishes",
	"fund-groups-are-their-printed-rows": "the group summary rows pp.186-209 print, read off page text",
	"source-pdfs-match-both-records":     "the source PDFs, which only --full reads",
}

// TestTheSyntheticCityPassesEveryCheckThatReadsIt: over synthcity.Default
// every check passes, or is vacuous, or reads only the corpus (corpusChecks),
// each of which names a check; a skip is excused only by name;
// and the checks that hold the projections to their facts each find
// something to hold. That is what makes the city a Budget Book the pipeline
// would accept, and not a set of figures it happens to draw.
// Mutation: add a cent to every fund's beginning balance without its ending
// and fund-balance-identity fails; drop a transfer's receiving leg and the
// projections refuse to build; drop source-pdfs-match-both-records from
// corpusChecks and its skip goes red.
func TestTheSyntheticCityPassesEveryCheckThatReadsIt(t *testing.T) {
	rep := runChecks(t, synthSubject(t))
	for _, res := range rep.Results {
		if _, ok := corpusChecks[res.CheckID]; ok || res.Status == StatusPass || res.Status == StatusVacuous {
			continue
		}
		t.Errorf("%s %s over %d", res.CheckID, res.Status, res.Subjects)
		for i, f := range res.Findings {
			if i == 5 {
				t.Errorf("    ... and %d more", len(res.Findings)-i)
				break
			}
			t.Errorf("    %s: %s", f.Subject, f.Detail)
		}
	}
	for _, id := range []string{
		"projections-build", "cuts-tie-along-the-lattice", "link-values-tie-to-facts",
		"node-balances-tie-to-facts", "transfer-legs-pair", "expansion-opens-as-the-spine",
		"expansion-splits-tie", "fund-balance-identity", "fund-group-sources-equal-uses",
		"headline-ties-to-facts",
	} {
		res := resultFor(t, rep, id)
		if res.Status != StatusPass || res.Subjects == 0 {
			t.Errorf("%s is %s over %d subjects over the city; it must hold something", id, res.Status, res.Subjects)
		}
	}
	var excused []string
	for id := range corpusChecks {
		resultFor(t, rep, id)
		excused = append(excused, id)
	}
	slices.Sort(excused)
	t.Logf("excused as reading the corpus: %v", excused)
}
