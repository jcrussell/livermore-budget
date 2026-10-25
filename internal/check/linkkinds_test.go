package check

import (
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/project"
)

// TestLinkKindsMatchTheirFactsIsFailable damages the fixture's link kinds: a
// transfer published as "external" passes every check that reads only amounts.
func TestLinkKindsMatchTheirFactsIsFailable(t *testing.T) {
	t.Parallel()
	const id = "link-kinds-match-their-facts"

	if res := resultFor(t, runChecks(t, testSubject(t)), id); res.Status != StatusPass {
		t.Fatalf("the undamaged fixture is %s, want pass: %v", res.Status, res.Findings)
	}

	cases := []struct {
		name   string
		damage func(t *testing.T, g *project.Document)
		want   string
	}{
		{
			name: "a transfer link published as external",
			damage: func(t *testing.T, g *project.Document) {
				linkWithPrefix(t, g, "transfers/").Kind = project.KindExternal
			},
			want: "crosses no boundary",
		},
		{
			name: "a fund-balance link published as external",
			damage: func(t *testing.T, g *project.Document) {
				linkWithPrefix(t, g, project.NodeFundBalanceDraw).Kind = project.KindExternal
			},
			want: "fund-balance row",
		},
		{
			name: "a kind outside the contract's closed set",
			damage: func(t *testing.T, g *project.Document) {
				g.Links[0].Kind = "grant"
			},
			want: "not one of",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := testSubject(t)
			c.damage(t, s.Projections[0].Graph)
			res := resultFor(t, runChecks(t, s), id)
			wantFail(t, res, c.want)
		})
	}
}

// TestAnExternalLinkIsNotAssertedToBeExternal states the check's limit: a
// revenue fact carries no marker telling it from a transfer, so an external
// link is not re-derived.
func TestAnExternalLinkIsNotAssertedToBeExternal(t *testing.T) {
	t.Parallel()
	s := testSubject(t)
	// Relabel a genuinely external revenue link as an internal transfer. The
	// check does not object, and that is the documented limit.
	linkWithPrefix(t, s.Projections[0].Graph, "revenue/").Kind = project.KindInternalTransfer
	res := resultFor(t, runChecks(t, s), "link-kinds-match-their-facts")
	if res.Status != StatusPass {
		t.Errorf("status = %s, want pass: the one-way constraint says nothing about a "+
			"revenue link wearing a transfer's kind, and the check must not pretend "+
			"otherwise: %v", res.Status, res.Findings)
	}
}

// linkWithPrefix is the first link whose source starts with prefix, which is
// what a test damaging "some transfer link" needs; linkFrom takes an exact
// node id and is the right tool when the test names one.
func linkWithPrefix(t *testing.T, g *project.Document, prefix string) *project.Link {
	t.Helper()
	for i := range g.Links {
		if strings.HasPrefix(g.Links[i].Source, prefix) {
			return &g.Links[i]
		}
	}
	t.Fatalf("the fixture carries no link from %q", prefix)
	return nil
}
