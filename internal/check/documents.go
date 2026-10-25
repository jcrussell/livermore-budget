package check

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/jcrussell/livermore-budget/internal/english"
)

// uncheckedDocuments are the projection shapes no structural check reads yet,
// each with the reason and the bead that retires it, as declaredVacuous is for
// checks. staleDocumentDeclarations fails an entry that no longer names a
// built projection. Keep it empty. [Load] copies it to
// Subject.UncheckedDocuments.
var uncheckedDocuments = map[string]string{}

// documentsAreChecked asserts every built projection is one some check reads,
// tested by the shape it built: each arm of documentShape names the checks
// that read its shape, so a new shape without checks fails here.
type documentsAreChecked struct{}

func (*documentsAreChecked) meta() meta {
	return meta{id: "documents-are-checked", tier: 1,
		description: "every projection the site publishes is one some structural check reads, or its " +
			"shape is declared as one no check reads yet"}
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
		if _, ok := s.UncheckedDocuments[p.Name]; ok {
			declared[p.Name]++
			continue
		}
		unread++
		findings = append(findings, finding(p.String(),
			"this projection produced no document of any shape the structural checks read, "+
				"and no entry in uncheckedDocuments declares that. `fisc export` would "+
				"publish a document that verify has not looked at"))
	}

	// A registered projection that produced nothing writes no file and is not
	// reported here; a published one that stops being built is
	// publishedProjectionBuilt's finding.

	// The other direction expires an exemption: an entry naming nothing built
	// unchecked. Shapes are collected per name, since two projections can share
	// one.
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
	for _, name := range slices.Sorted(maps.Keys(s.UncheckedDocuments)) {
		if declared[name] > 0 {
			continue
		}
		what := "no projection of that name was built"
		if shapes := built[name]; len(shapes) > 0 {
			what = fmt.Sprintf("every projection of that name now carries a %s, so a "+
				"structural check reads it and the declaration is exempting nothing",
				strings.Join(slices.Sorted(maps.Keys(shapes)), ", "))
		}
		findings = append(findings, finding(name,
			"uncheckedDocuments declares this projection unchecked, but %s; the "+
				"declaration must be removed", what))
	}

	// The summary names the shapes as well as the count. `held` renders only
	// on a pass with checked > 0, so the shape list cannot come out empty.
	held := fmt.Sprintf("%d %s, each read by the structural checks: %s",
		checked, english.Plural(checked, "projection", "projections"), describeShapes(shapes))
	if len(declared) > 0 {
		held = fmt.Sprintf("%d %s structurally checked (%s), plus %s declared unchecked",
			checked, english.Plural(checked, "projection", "projections"),
			describeShapes(shapes), describeUnchecked(s.UncheckedDocuments, declared))
	}
	return conclusion{
		// Counted over what was examined, read or reported unread; declared
		// projections are excluded, else a fully-declared run would pass, and
		// are named in the summary instead.
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
	case p.Graph != nil && p.Graph.Metadata.Headline != nil:
		// Every check over Subject.linkedDocuments, plus the three headline
		// checks where Subject.spine publishes it: headline-ties-to-facts,
		// headline-transfer-residual and headline-naive-expenditure.
		return "graph with a headline"
	case p.Graph != nil:
		// Every check over Subject.linkedDocuments: graph-acyclic,
		// node-tiers-are-declared, link-values-tie-to-facts, counts-reconcile,
		// transfer-legs-pair and the rest.
		return "graph"
	case p.Trends != nil:
		// trend-points-tie-to-facts and trend-series-are-complete.
		return "series"
	case p.Debt != nil:
		// debt-service-ties-to-facts.
		return "debt schedule"
	default:
		return ""
	}
}

// describeShapes renders the document shapes built, sorted, with their counts.
func describeShapes(byShape map[string]int) string {
	out := make([]string, 0, len(byShape))
	for _, shape := range slices.Sorted(maps.Keys(byShape)) {
		out = append(out, fmt.Sprintf("%d %s", byShape[shape], shape))
	}
	return strings.Join(out, ", ")
}

// describeUnchecked renders the declared-unchecked projections with their
// reasons: a count alone would let a growing exemption pass unread.
func describeUnchecked(unchecked map[string]string, byName map[string]int) string {
	out := make([]string, 0, len(byName))
	for _, name := range slices.Sorted(maps.Keys(byName)) {
		out = append(out, fmt.Sprintf("%d of %q (%s)", byName[name], name, unchecked[name]))
	}
	return strings.Join(out, ", ")
}
