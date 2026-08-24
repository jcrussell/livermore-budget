package check

import (
	"context"
	"fmt"
)

// uncheckedDocuments are the projection shapes no structural check reads yet,
// each with the reason and the bead that retires it.
//
// It is the same declaration this package already makes about facts in
// unprojectedScopes, for the same reason and with the same danger. A projection
// that produces no graph is not checked by anything in graph.go, and a document
// nobody checks is a document that can be wrong on the published site while
// `fisc verify` prints all-green. Before this map, such a projection could not
// be registered at all -- Load returned an error and the whole run died -- which
// stopped the wrong thing in the wrong way: it protected the corpus by refusing
// to report on it.
//
// AN ENTRY HERE IS A PROMISE THAT SOMEONE IS COMING BACK. Adding one is how a
// new document shape lands before its checks do; leaving one is how a document
// stays unchecked forever. staleDocumentDeclarations is the branch that refuses
// the second, by failing an entry that no longer names a built projection.
//
// It is empty today, and that is the state to keep it in: every projection the
// registry returns produces a graph, and every graph is checked.
var uncheckedDocuments = map[string]string{}

// documentsAreChecked asserts every built projection is one some check reads.
//
// The test it applies is deliberately crude -- does this projection carry a
// graph -- because a precise one is not available: nothing in Go lets this ask
// "which checks read this projection". What makes the crude test sound is that
// the graph checks are ALL the structural checks there are, so a projection
// without a graph is read by none of them, and the day that stops being true is
// the day a second document shape gets its own checks and its own arm here.
type documentsAreChecked struct{}

var _ Check = (*documentsAreChecked)(nil)

func (*documentsAreChecked) ID() string { return "documents-are-checked" }
func (*documentsAreChecked) Tier() int  { return 1 }
func (*documentsAreChecked) Full() bool { return false }
func (*documentsAreChecked) Description() string {
	return "every projection the site publishes is one some structural check reads, or its " +
		"shape is declared as one no check reads yet"
}

func (*documentsAreChecked) Run(_ context.Context, s *Subject) (Result, error) {
	var findings []Finding
	declared := map[string]int{}
	checked := 0

	for _, p := range s.Projections {
		if p.Graph != nil {
			checked++
			continue
		}
		if _, ok := uncheckedDocuments[p.Name]; ok {
			declared[p.Name]++
			continue
		}
		findings = append(findings, finding(p.String(),
			"this projection produces no graph, so no structural check reads it, and no "+
				"entry in uncheckedDocuments declares that. `fisc export` would publish a "+
				"document that verify has not looked at"))
	}

	// A registered projection that produced NOTHING is the case neither list
	// above can see. It is in Projections with no entry and in
	// ProjectionFailures with no entry, because it did not fail -- it declared
	// no slices. `fisc export` still publishes its document, so silence here
	// would be the whole defect this check exists for, wearing the one face the
	// obvious implementation misses.
	produced := map[string]bool{}
	for _, p := range s.Projections {
		produced[p.Name] = true
	}
	for _, f := range s.ProjectionFailures {
		produced[f.Name] = true
	}
	for _, name := range s.Registered {
		if !produced[name] {
			findings = append(findings, finding(name,
				"this projection is registered and produced no document at all: it declared "+
					"no slice of the fact store it is of, so nothing built and nothing "+
					"failed. `fisc export` would still publish it"))
		}
	}

	// The other direction, and it is the one that expires an exemption rather
	// than leaving it for whoever forgets: an entry naming a shape nothing
	// builds is either a projection that was removed or a name that has drifted.
	built := map[string]bool{}
	for _, p := range s.Projections {
		built[p.Name] = true
	}
	for _, name := range sortedStrings(uncheckedDocuments) {
		if declared[name] == 0 {
			what := "no projection of that name was built"
			if built[name] {
				what = "every projection of that name now carries a graph, so a structural " +
					"check reads it and the declaration is exempting nothing"
			}
			findings = append(findings, finding(name,
				"uncheckedDocuments declares this projection unchecked, but %s; the "+
					"declaration must be removed", what))
		}
	}

	held := fmt.Sprintf("%d %s, each read by the structural checks",
		checked, plural(checked, "projection", "projections"))
	if len(declared) > 0 {
		held = fmt.Sprintf("%d %s structurally checked, plus %s declared unchecked",
			checked, plural(checked, "projection", "projections"), describeUnchecked(declared))
	}
	return conclusion{
		// Counted over the REGISTRY, not over what built: a projection that
		// produced nothing is exactly what this check is looking for, and
		// counting only what built would leave it out of its own denominator.
		subjects: len(s.Registered),
		unit:     "projections",
		held:     held,
		nothing:  "no projection is registered",
		findings: findings,
	}.result(), nil
}

// describeUnchecked renders the declared-unchecked projections with their
// reasons: a count alone would let a growing exemption pass unread.
func describeUnchecked(byName map[string]int) string {
	out := make([]string, 0, len(byName))
	for _, name := range sortedStrings(byName) {
		out = append(out, fmt.Sprintf("%d of %q (%s)", byName[name], name, uncheckedDocuments[name]))
	}
	return joinComma(out)
}
