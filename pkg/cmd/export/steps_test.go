package export

import (
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/export"
	"github.com/jcrussell/livermore-budget/internal/project"
)

// stepGraph is as much of a built document as the sentence test reads: each
// node's parent, and each ribbon's ends and cents.
type stepGraph struct {
	Nodes []struct {
		ID     string `json:"id"`
		Parent string `json:"parent"`
	} `json:"nodes"`
	Links []struct {
		Source     string `json:"source"`
		Target     string `json:"target"`
		ValueCents int64  `json:"value_cents"`
	} `json:"links"`
}

// TestTheFundStepsSentenceIsItsArithmetic holds the fund step's Description
// to the identity it states in every published column: what fund/100 takes in
// less what its divisions draw equals what pp.66-67 print leaving the general
// group other than through its divisions, less the money the group takes in
// that no fund receives -- every spine ribbon into the group whose source
// sends nothing into any node under the group in the document the group opens
// into. Read off the two documents, not off a drawn chart. The words and the
// arithmetic are pinned together.
func TestTheFundStepsSentenceIsItsArithmetic(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	built, err := buildProjections(root)
	if err != nil {
		t.Fatalf("buildProjections: %v", err)
	}
	var spine export.View
	for _, v := range mustViews(t, result{Projections: built}) {
		if v.Path == export.IndexPath {
			spine = v
		}
	}
	si := slices.IndexFunc(spine.Steps, func(s export.DrillStep) bool { return s.Key == "fund" })
	if si < 0 {
		t.Fatal("no fund step on the spine")
	}
	const twoTerms = "less the money the group takes in that no fund receives"
	if !strings.Contains(spine.Steps[si].Description, twoTerms) {
		t.Fatalf("the fund step's description does not say %q, so the arithmetic below is not what the reader is told", twoTerms)
	}
	_, ix, err := export.ColumnsOf(built, "")
	if err != nil {
		t.Fatalf("ColumnsOf: %v", err)
	}
	decode := func(stem string) stepGraph {
		var g stepGraph
		if err := json.Unmarshal(built[stem], &g); err != nil {
			t.Fatalf("decode %s: %v", stem, err)
		}
		return g
	}
	const group, fund = "fund-group/general", "fund/100"
	checked := 0
	for _, d := range project.PublishedDocuments() {
		if len(d.Columns) != 1 || d.Projection != spine.Projection {
			continue
		}
		stems, err := export.StepStems(spine.Steps, d.Stem, ix)
		if err != nil {
			t.Fatalf("StepStems for %q: %v", d.Stem, err)
		}
		year := decode(d.Stem)
		drawn := decode(stems[si])
		// The group's subtree in the document it opens into, by parent chain.
		parent := map[string]string{}
		for _, n := range drawn.Nodes {
			parent[n.ID] = n.Parent
		}
		inside := func(id string) bool {
			for hops := 0; id != "" && hops < 9; hops++ {
				if id == group {
					return true
				}
				id = parent[id]
			}
			return false
		}
		// A spine source is received when it, or a node under it here, feeds the
		// group's subtree: the drill document's sources are revenue lines, which
		// the spine names by their category.
		received := map[string]bool{}
		var in, out int64
		for _, l := range drawn.Links {
			if inside(l.Target) {
				for id, hops := l.Source, 0; id != "" && hops < 9; hops++ {
					received[id] = true
					id = parent[id]
				}
			}
			switch {
			case l.Source == group && l.Target == fund:
				in += l.ValueCents
			case l.Source == fund:
				out += l.ValueCents
			}
		}
		var leaving, unreceived int64
		for _, l := range year.Links {
			if l.Source == group && !strings.HasPrefix(l.Target, "expenditure/") {
				leaving += l.ValueCents
			}
			if l.Target == group && !received[l.Source] {
				unreceived += l.ValueCents
			}
		}
		if in == 0 || out == 0 || leaving == 0 || unreceived == 0 {
			t.Fatalf("%s: in %d, out %d, leaving %d, unreceived %d; a zero term proves nothing", d.Stem, in, out, leaving, unreceived)
		}
		t.Logf("%s: fund/100 takes %d and its divisions draw %d, a difference of %d; the group's transfers out and fund-balance rows come to %d less %d no fund receives, which is %d",
			d.Stem, in, out, in-out, leaving, unreceived, leaving-unreceived)
		if in-out != leaving-unreceived {
			t.Errorf("%s: the fund step's sentence states in - out = leaving - unreceived, and %d - %d != %d - %d",
				d.Stem, in, out, leaving, unreceived)
		}
		checked++
	}
	if checked == 0 {
		t.Fatal("no published column is the spine's, so nothing above was checked")
	}
}
