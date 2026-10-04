package export

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/project"
	"github.com/jcrussell/livermore-budget/internal/structure"
)

// TestEveryPublishedDocumentSelectsExactlyItsCutsFootprint is the measurement
// fisc-4qv2.4 rests on: a graph document's fact selection -- scope, kind and
// column, project.Options' three selectors -- is exactly the facts the cuts and
// residues declared on its scopes admit, in its column and its kinds. The
// selection is the builders' own, project.SelectFacts; the footprint is
// structure's Cut.Admits and Residue.Matches, a different question (which
// cells a schedule prints) asked of the same facts. So a builder that narrowed
// or widened its selection past the cut model, or a cut pinned to fund groups
// its scope's document still draws, is red here. It names the documents that
// draw a declared residue.
func TestEveryPublishedDocumentSelectsExactlyItsCutsFootprint(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	_, facts, err := readFactStore(root)
	if err != nil {
		t.Fatalf("read the fact store: %v", err)
	}
	residues := structure.BudgetBookResidue()
	documents, residueDrawers := 0, 0
	for _, d := range project.PublishedDocuments() {
		if len(d.Columns) != 1 {
			continue
		}
		o := project.Options{Columns: d.Columns, Scopes: d.Scopes, Kinds: d.Kinds,
			ThroughCuts: d.ThroughCuts, Version: "t"}
		cuts := structure.CutsOf(d.Scopes)
		if len(cuts) == 0 {
			t.Errorf("%s: no declared cut reads any of its scopes %v", d.Stem, d.Scopes)
			continue
		}
		documents++
		chosen := map[string]bool{}
		slice, err := project.SelectFacts(facts, o)
		if err != nil {
			t.Fatalf("%s: %v", d.Stem, err)
		}
		for _, f := range slice {
			chosen[f.ID] = true
		}
		selected, drawnResidue := 0, 0
		for i := range facts {
			f := &facts[i]
			if !slices.Contains(o.Columns, project.Column{FiscalYear: f.FiscalYear, Basis: f.Basis}) || !o.HasKind(f.Kind) {
				continue
			}
			inScope := chosen[f.ID]
			inCut := slices.ContainsFunc(cuts, func(c structure.Cut) bool { return c.Admits(f) })
			// A document selecting through its cuts draws no residue.
			inResidue := !d.ThroughCuts && slices.ContainsFunc(residues, func(r structure.Residue) bool {
				return r.Matches(f) && slices.Contains(d.Scopes, r.Scope)
			})
			if inScope {
				selected++
			}
			if inResidue {
				drawnResidue++
			}
			switch {
			case inScope && !inCut && !inResidue:
				t.Errorf("%s selects fact %s (%s %s %q) and no cut or residue of its scopes admits it; "+
					"the document draws money the structure does not place", d.Stem, f.ID, f.Scope, f.Kind, f.RowLabel)
			case !inScope && (inCut || inResidue):
				t.Errorf("%s does not select fact %s (%s %s) and a cut or residue of its scopes admits it",
					d.Stem, f.ID, f.Scope, f.Kind)
			}
		}
		if selected == 0 {
			t.Errorf("%s selects no fact, so the equality above is of nothing", d.Stem)
		}
		if drawnResidue > 0 {
			residueDrawers++
			t.Logf("%s: %d facts selected over %d cut(s), %d of them a declared residue", d.Stem, selected, len(cuts), drawnResidue)
		}
	}
	if documents == 0 {
		t.Fatal("no published document is a single column, so nothing above compared")
	}
	// department-spending draws pp.85-125's one Transfers Out row, which sits
	// in no cut: the residue is drawn, and it is the only one.
	if residueDrawers != 4 {
		t.Errorf("%d documents draw a declared residue, want the four department-spending columns", residueDrawers)
	}
}
