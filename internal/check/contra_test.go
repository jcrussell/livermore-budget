package check

import (
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/project"
)

// TestContraLinksNameTheirScheduleIsFailable damages both halves of the
// biconditional and the sentence itself. The fixture publishes no negative
// link, so this test makes one.
func TestContraLinksNameTheirScheduleIsFailable(t *testing.T) {
	const id = "contra-links-name-their-schedule"

	// Vacuous, not a pass over an empty set.
	if res := resultFor(t, runChecks(t, testSubject(t)), id); res.Status != StatusVacuous {
		t.Fatalf("the undamaged fixture is %s, want vacuous: %v", res.Status, res.Findings)
	}

	// reduce turns one link into a printed reduction and answers with the
	// sentence this document's own hierarchy puts on it. It installs a
	// hierarchy, which the fixture spine lacks; without one every reduction
	// takes the orphan sentence and the tree comparison is never posed.
	reduce := func(t *testing.T, g *project.Document) (*project.Link, string) {
		t.Helper()
		if len(g.Links) == 0 {
			t.Fatal("the fixture graph draws no link; this test cannot pose its question")
		}
		const up, label = "category/testing", "Property Taxes"
		g.Nodes = append(g.Nodes, project.Node{ID: up, Label: label, Tier: 0})
		src := g.Links[0].Source
		for i := range g.Nodes {
			if g.Nodes[i].ID == src {
				g.Nodes[i].Parent = up
			}
		}
		g.Links[0].ValueCents = -g.Links[0].ValueCents
		return &g.Links[0], "printed as a reduction of " + label
	}

	// A reduction that names its own category is what the corpus looks like.
	s := testSubject(t)
	l, want := reduce(t, s.Projections[0].Graph)
	l.Contra = want
	if res := resultFor(t, runChecks(t, s), id); res.Status != StatusPass {
		t.Fatalf("a correctly named reduction is %s, want pass: %v", res.Status, res.Findings)
	}

	for _, c := range []struct {
		name   string
		damage func(l *project.Link, want string)
		want   string
	}{
		{
			// With no sentence the ribbon reads as an addition.
			name:   "a reduction that names no schedule",
			damage: func(l *project.Link, _ string) { l.Contra = "" },
			want:   "names no schedule",
		},
		{
			// The converse.
			name: "an addition that claims to be a reduction",
			damage: func(l *project.Link, want string) {
				l.ValueCents = -l.ValueCents
				l.Contra = want
			},
			want: "asserts a subtraction no page makes",
		},
		{
			// A sentence composed off a folded chart rather than this document.
			name:   "a reduction naming a category the document does not put it under",
			damage: func(l *project.Link, _ string) { l.Contra = "printed as a reduction of Gravy" },
			want:   "names a category they are not looking at",
		},
		{
			name:   "a reduction whose parent was blanked before the words were composed",
			damage: func(l *project.Link, _ string) { l.Contra = "printed rows netting to a reduction" },
			want:   "this document's own hierarchy puts the source under",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			s := testSubject(t)
			l, want := reduce(t, s.Projections[0].Graph)
			l.Contra = want
			c.damage(l, want)
			res := resultFor(t, runChecks(t, s), id)
			if res.Status != StatusFail {
				t.Fatalf("status = %s, want fail", res.Status)
			}
			if !strings.Contains(findingDetails(res), c.want) {
				t.Errorf("findings %v do not mention %q", res.Findings, c.want)
			}
		})
	}
}
