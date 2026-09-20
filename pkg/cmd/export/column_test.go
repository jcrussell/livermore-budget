package export

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/project"
)

// repoRootForTest is the checkout these tests build the site from.
func repoRootForTest(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	return root
}

// factRow is the part of a committed fact this file needs.
type factRow struct {
	ID          string `json:"id"`
	AmountCents int64  `json:"amount_cents"`
}

// committedFacts reads facts/facts.jsonl, which is the audit trail CI compares
// byte for byte.
func committedFacts(t *testing.T) map[string]int64 {
	t.Helper()
	path := filepath.Join("..", "..", "..", "facts", "facts.jsonl")
	f, err := os.Open(path) // #nosec G304 -- a fixed path inside the repository.
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer func() { _ = f.Close() }()

	out := map[string]int64{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var row factRow
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		out[row.ID] = row.AmountCents
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if len(out) == 0 {
		t.Fatalf("%s carries no facts, so this test asserts nothing", path)
	}
	return out
}

// THE COLUMN IS HELD TO THE FACTS, NOT TO THE FILES IT REPLACES.
//
// This is the check that makes the cutover provable. The per-projection
// documents are going, so comparing against them would be comparing against
// something about to be deleted -- and regenerating a golden in the same commit
// that changes the format is green by construction, which is the shape
// AGENTS.md's "Prove it can fail" names. facts/facts.jsonl is independent of any
// document format, is the audit trail every other check is measured against, and
// CI compares it byte for byte. So the column is held to THAT.
//
// A DERIVED LINK CITES NOTHING AND IS NOT EXEMPTED QUIETLY: it is counted and
// reported, because "no page prints this figure" is a claim worth seeing the
// size of rather than a branch that skips.
func TestEveryColumnLinkEqualsTheFactsItCites(t *testing.T) {
	facts := committedFacts(t)
	built, err := buildAll(repoRootForTest(t))
	if err != nil {
		t.Fatalf("buildAll: %v", err)
	}
	columns, err := columnsOf(built.Projections)
	if err != nil {
		t.Fatalf("columnsOf: %v", err)
	}
	if len(columns) == 0 {
		t.Fatal("no column document was built, so this test asserts nothing")
	}

	checked, derived, flipped := 0, 0, 0
	for name, col := range columns {
		for schedule, sched := range col.Schedules {
			for i, l := range sched.Links {
				if len(l.FactIDs) == 0 {
					if !l.Derived {
						t.Errorf("%s %s link %d cites no fact and is not marked derived; "+
							"every published figure carries a provenance pointer",
							name, schedule, i)
					}
					derived++
					continue
				}
				var sum int64
				missing := []string(nil)
				for _, id := range l.FactIDs {
					cents, ok := facts[id]
					if !ok {
						missing = append(missing, id)
						continue
					}
					sum += cents
				}
				if len(missing) > 0 {
					t.Errorf("%s %s link %d cites %v, which facts/facts.jsonl does not carry",
						name, schedule, i, missing)
					continue
				}
				// EXACTLY ONE LINK MAY DIFFER FROM THE PLAIN SUM, and the
				// condition is the SOURCE NODE ID rather than the sign of the
				// sum. internal/check's link-values-tie-to-facts makes the same
				// allowance in the same terms: the city prints CHANGE IN
				// WORKING CAPITAL once per column, signed, a Sankey cannot draw
				// a negative link, so internal/project decomposes the sign into
				// two nodes and the draw leg carries the NEGATION of its facts.
				// "equals the absolute sum" would accept a leg pointing the
				// wrong way, which is the error the decomposition can actually
				// make.
				want := sum
				if col.Nodes[l.From].ID == project.NodeFundBalanceDraw {
					want = -sum
					flipped++
				}
				if want != l.ValueCents {
					t.Errorf("%s %s link %d (%s -> %s) carries %d and its %d fact(s) come to %d",
						name, schedule, i, col.Nodes[l.From].ID, col.Nodes[l.To].ID,
						l.ValueCents, len(l.FactIDs), sum)
				}
				checked++
			}
		}
	}
	t.Logf("%d cited link(s) across %d column(s) equal the facts they cite, "+
		"%d of them the fund-balance draw carrying their negation; %d derived link(s) cite nothing",
		checked, len(columns), flipped, derived)
	// THE ALLOWANCE IS NOT VACUOUS. If no draw leg is in the corpus the rule
	// above is inert, and a reader of this test would not know.
	if flipped == 0 {
		t.Error("no fund-balance draw was checked, so the one allowance this test " +
			"makes was never exercised and could be wrong")
	}
}

// THE NODE TABLE IS REACHABLE AND THE INDICES MEAN WHAT THEY SAY. A link
// referencing its ends by index is only as good as the table, and an off-by-one
// here would draw a chart whose ribbons join the wrong marks while every figure
// stayed correct -- a defect no fact check can see.
func TestEveryColumnIndexResolves(t *testing.T) {
	built, err := buildAll(repoRootForTest(t))
	if err != nil {
		t.Fatalf("buildAll: %v", err)
	}
	columns, err := columnsOf(built.Projections)
	if err != nil {
		t.Fatalf("columnsOf: %v", err)
	}
	for name, col := range columns {
		seen := map[int]bool{}
		for _, tier := range col.Tiers {
			for _, i := range tier.Nodes {
				if i < 0 || i >= len(col.Nodes) {
					t.Fatalf("%s tier %d names node %d of %d", name, tier.Tier, i, len(col.Nodes))
				}
				if col.Nodes[i].Tier != tier.Tier {
					t.Errorf("%s tier %d holds %s, which is tier %d",
						name, tier.Tier, col.Nodes[i].ID, col.Nodes[i].Tier)
				}
				if seen[i] {
					t.Errorf("%s lists node %s in two tiers", name, col.Nodes[i].ID)
				}
				seen[i] = true
			}
		}
		if len(seen) != len(col.Nodes) {
			t.Errorf("%s: the tier lists reach %d of %d nodes, so some node stands in no column",
				name, len(seen), len(col.Nodes))
		}
		for schedule, sched := range col.Schedules {
			for _, n := range sched.Nodes {
				if n.Node < 0 || n.Node >= len(col.Nodes) {
					t.Fatalf("%s %s draws node %d of %d", name, schedule, n.Node, len(col.Nodes))
				}
			}
			for i, l := range sched.Links {
				if l.From < 0 || l.From >= len(col.Nodes) || l.To < 0 || l.To >= len(col.Nodes) {
					t.Fatalf("%s %s link %d joins %d -> %d, and the table holds %d",
						name, schedule, i, l.From, l.To, len(col.Nodes))
				}
			}
		}
	}
}
