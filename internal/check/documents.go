package check

import (
	"context"
	"fmt"
)

// uncheckedDocuments are the projection shapes no structural check reads yet,
// each with the reason and the bead that retires it.
//
// It is the same declaration this package already makes about checks in
// declaredVacuous, for the same reason and with the same danger. A projection
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
// THE TEST IS "DOES THIS PROJECTION CARRY A DOCUMENT SOME CHECK READS", and it
// is applied by asking which SHAPE the projection built, because a precise test
// is not available: nothing in Go lets this ask "which checks read this
// projection".
//
// It used to ask only "does this carry a graph", and that was sound while the
// graph checks were all the structural checks there were -- its own doc comment
// said "the day that stops being true is the day a second document shape gets
// its own checks and its own arm here". That day arrived with the revenue
// trends (fisc-5ep). What makes the widened test still sound is that each arm
// below names the checks that read the shape, so adding a shape without adding
// checks does not silently widen the exemption -- it fails here.
//
// The arm is NOT an uncheckedDocuments entry, and the difference matters. That
// map is for a shape nothing checks YET, and its own doc comment calls an entry
// "a promise that someone is coming back". A shape that IS checked needs the
// opposite: a statement that it is covered, which expires by failing when the
// coverage goes away rather than when someone remembers.
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
	shapes := map[string]int{}
	checked := 0
	unread := 0

	for _, p := range s.Projections {
		if shape := documentShape(p); shape != "" {
			checked++
			shapes[shape]++
			continue
		}
		if _, ok := uncheckedDocuments[p.Name]; ok {
			declared[p.Name]++
			continue
		}
		unread++
		findings = append(findings, finding(p.String(),
			"this projection produced no document of any shape the structural checks read, "+
				"and no entry in uncheckedDocuments declares that. `fisc export` would "+
				"publish a document that verify has not looked at"))
	}

	// A REGISTERED PROJECTION THAT PRODUCED NOTHING WAS REPORTED HERE AND IS NOT
	// ANY MORE, because the premise it rested on has gone. The finding read
	// "nothing built and nothing failed -- `fisc export` would still publish
	// it", and that was true of the export command's old build loop: it ran the
	// cartesian product of the published years and the registry and called
	// Build on every projection whatever slices it declared. It now loops the
	// slices each projection says it is of (fisc-neh), so a projection
	// declaring none writes no file, and there is no document to be wrong.
	//
	// Keeping the arm would have made every projection of a schedule a FIXTURE
	// does not carry into a defect -- internal/check/fixture_test.go is
	// spine-only, so the trends projection declares nothing there and 70-odd
	// test call sites would report a corpus finding about a test's own scope.
	// project.Sliced's doc comment already settles what that state means: "an
	// empty result means the store carries nothing this projection is of, which
	// is a statement about the corpus and not an error".
	//
	// WHAT IT LEAVES UNCOVERED IS REAL AND IS FILED. A document the site
	// PUBLISHES that silently stops being built is still a defect, and only the
	// spine is guarded against it today, by publishedProjectionBuilt. See
	// fisc-w7d.

	// The other direction, and it is the one that expires an exemption rather
	// than leaving it for whoever forgets: an entry naming a shape nothing
	// builds is either a projection that was removed or a name that has drifted.
	//
	// THE FINDING NAMES THE SHAPE THE PROJECTION ACTUALLY CARRIES, and it used
	// not to: it said "now carries a graph" whatever was built, in the check
	// that was widened precisely so a document need not be a graph. A reader
	// handed that about a trends document goes looking for a graph that does not
	// exist. The shapes are collected per NAME rather than as a single bool,
	// because two projections can share a name and build different documents.
	built := map[string]map[string]bool{}
	for _, p := range s.Projections {
		shape := documentShape(p)
		if shape == "" {
			continue
		}
		if built[p.Name] == nil {
			built[p.Name] = map[string]bool{}
		}
		built[p.Name][shape] = true
	}
	for _, name := range sortedStrings(uncheckedDocuments) {
		if declared[name] > 0 {
			continue
		}
		what := "no projection of that name was built"
		if shapes := built[name]; len(shapes) > 0 {
			what = fmt.Sprintf("every projection of that name now carries a %s, so a "+
				"structural check reads it and the declaration is exempting nothing",
				joinComma(sortedStrings(shapes)))
		}
		findings = append(findings, finding(name,
			"uncheckedDocuments declares this projection unchecked, but %s; the "+
				"declaration must be removed", what))
	}

	// The summary names the SHAPES as well as the count. A number alone would
	// read the same whether both document shapes were covered or one of them had
	// quietly stopped being built, which is the state publishedProjectionBuilt
	// exists for on the other axis.
	// `held` is rendered on the PASS path only, and a pass means every built
	// projection was either read or declared. So `checked > 0` whenever
	// len(declared) > 0 here -- a run with declarations and nothing read has a
	// zero denominator and takes the `nothing:` branch instead -- and the shape
	// list below cannot come out empty. That was not true while the count was
	// len(s.Projections): a fully-declared run passed, and printed "0
	// projections structurally checked ()". The count is the fix; there is
	// nothing left for a guard here to catch.
	held := fmt.Sprintf("%d %s, each read by the structural checks: %s",
		checked, plural(checked, "projection", "projections"), describeShapes(shapes))
	if len(declared) > 0 {
		held = fmt.Sprintf("%d %s structurally checked (%s), plus %s declared unchecked",
			checked, plural(checked, "projection", "projections"),
			describeShapes(shapes), describeUnchecked(declared))
	}
	return conclusion{
		// COUNTED OVER WHAT WAS EXAMINED, which is narrower than what was built
		// and narrower still than the registry.
		//
		// It counted the registry once, because this check also reported a
		// registered projection that produced nothing and that projection had to
		// be in its own denominator. That arm is gone (see above). It then
		// counted len(s.Projections) -- everything BUILT -- and that was the
		// defect fisc-rwo names: a projection declared in uncheckedDocuments is
		// built and is NOT examined, so a run in which every projection was
		// declared reported a PASS while the `nothing:` branch below sat
		// unreachable. A pass over a denominator the check did not look at is
		// the one thing this package exists to prevent, and this check was
		// producing one.
		//
		// EXAMINED, NOT READ. `checked` is the projections that reached a shape
		// some structural check reads; `unread` is the ones that reached none
		// and were reported for it. Both were looked at, so both are in the
		// denominator -- counting only `checked` would report "2 findings over 0
		// projections", a numerator with no denominator under it.
		//
		// Only the DECLARED ones are excluded, because a declaration is the
		// statement that nothing examined them. They are named in the summary
		// instead, where a reader can watch the exemption grow.
		subjects: checked + unread,
		unit:     "projections",
		held:     held,
		nothing:  "no projection built a document, so no document shape has been examined",
		findings: findings,
	}.result(), nil
}

// documentShape names the shape a projection built, and the checks that read
// it, or "" for a projection that built no document any check reads.
//
// EVERY ARM NAMES ITS CHECKS, and that is what keeps this honest rather than a
// list of shapes someone remembers to extend. A shape added here without checks
// behind it is a claim a reader can falsify by grepping for the names.
func documentShape(p projection) string {
	switch {
	case p.Graph != nil:
		// graph.go: acyclic, link values tie to facts, headline ties to facts,
		// counts reconcile, aggregation invariance, and the rest.
		return "graph"
	case p.Trends != nil:
		// trend-points-tie-to-facts and trend-series-are-complete.
		return "series"
	case p.FundFlows != nil:
		// The six structural checks that read Subject.Linked: graph-acyclic,
		// node-tiers-are-declared, derived-nodes-justified,
		// link-values-tie-to-facts, node-hierarchy-well-formed and
		// constraint-tier-vocabulary. NOT the three headline ones, which is the
		// whole point of the shape: this document publishes no headline.
		return "linked graph, no headline"
	case p.DepartmentSpending != nil:
		// The same six structural checks that read Subject.Linked. The
		// arithmetic this document rests on is cuts-tie-along-the-lattice,
		// which reads the FACTS and needs no graph.
		return "cross-tab, no headline"
	case p.DepartmentFunding != nil:
		// The same six structural checks that read Subject.Linked. The
		// arithmetic this document rests on is cuts-tie-along-the-lattice,
		// which reads the FACTS and needs no graph -- so there is no check of
		// this shape alone, and the shape is named apart from the cross-tab's
		// because they are two readings of the same eleven pages.
		return "funding graph, no headline"
	case p.TransfersByFund != nil:
		// The same six structural checks that read Subject.Linked, plus
		// transfer-legs-pair, which is the only check in the tree that reads
		// Link.TransferID and had no subject at all until this shape existed.
		// The arithmetic this document rests on is cuts-tie-along-the-lattice,
		// which reads the FACTS and is therefore not named here.
		return "paired legs, no headline"
	default:
		return ""
	}
}

// describeShapes renders the document shapes built, sorted, with their counts.
func describeShapes(byShape map[string]int) string {
	out := make([]string, 0, len(byShape))
	for _, shape := range sortedStrings(byShape) {
		out = append(out, fmt.Sprintf("%d %s", byShape[shape], shape))
	}
	return joinComma(out)
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
