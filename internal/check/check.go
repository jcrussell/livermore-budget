// Package check is the engine behind `fisc verify`: the checks that turn this
// project's central claim — every published figure traces to a cell of a
// source document, and the arithmetic between them holds — into something a
// reader can run rather than something the README asserts.
//
// # Nouns
//
// A [Check] is one claim. It concludes a [Result] carrying a [Status] over a
// [Subject], which is everything the checks read, loaded once. [Run] collects
// those results into a [Report]. Nothing here knows about exit codes or output
// streams; pkg/cmd/verify owns both.
//
// # Five statuses, because "it passed" and "there was nothing to check" are
// different claims
//
// The whole design turns on [StatusVacuous]. "Every transfer_id has two equal
// legs" is true of a graph in which no link carries one, and reporting that as
// a pass would tell a reader the pairing had been verified when nothing looked
// at it (fisc-4rh, and internal/project's Link.TransferID doc comment). So a
// check that ran over nothing says so, is counted apart from the passes, and
// fails only under --strict — not by default, because a gate that is red until
// the corpus is finished gets commented out (fisc-1wr.4).
//
// [Result.Subjects] is what makes that legible. "0 subjects" is the whole
// explanation of a vacuous result; a bare word is not.
//
// # What these checks can witness, and what they cannot
//
// One distinction runs through the whole report and is stated on each check that
// depends on it. [Load] builds the projection graphs from the same fact slice it
// then hands the checks, so a check comparing a figure internal/project derived
// against this package's re-derivation of it is comparing two functions over
// identical input inside one process. Those two move together: they cannot witness
// a wrong amount, and a run of 61 perturbed amounts once passed every one of them.
// What they DO catch is a derivation rule that is wrong or has drifted — a link
// citing facts that are not the ones its value came from, a selector dropped from
// the fact selection, the internal service split applied to the wrong group, a
// published count that no longer matches its own graph.
//
// The independent witnesses are the two that leave the derivation entirely:
// fact-token-reparses re-parses the verbatim source text each amount was read
// from, and fact-offset-points-at-token reads the committed page and checks that
// the provenance pointer lands on that text. Those are what make a wrong figure
// fail. Everything else is a ratchet on the code that shapes the figures, which is
// worth having and is not the same claim.
//
// The structural checks (tier 0) are witnesses of a different kind again: they say
// nothing about any figure, and they are the only thing here that can tell whether
// the substrate every other check reads is still the substrate the rules were
// written against. A page whose bytes changed produces facts whose offsets land on
// whatever is there now, and every arithmetic check above passes over them.
//
// # No PDFs, no Python
//
// [Load] reads facts/facts.jsonl, mappings/, the three registries under data/, and
// every artifact under data/extracted/ — all committed. Without Full it touches
// data/pdf/ nowhere, which is what lets CI verify without an LFS checkout or a
// venv (docs/agents/conventions.md, "the extraction boundary"). A check that needs
// the source documents themselves says so with Full and is skipped, not failed,
// when --full is absent; today that is one check, source-pdfs-match-both-records,
// and Load is the one place allowed to open data/pdf/ for it.
package check

import (
	"context"
	"fmt"
	"slices"
	"strings"
)

// Status is what a check concluded. There are five, because "it passed" and
// "there was nothing to test" are different claims, and not letting the second
// be reported as the first is most of the point of this command.
type Status string

// The statuses a check can conclude.
const (
	// StatusPass ran over at least one subject; every one held.
	StatusPass Status = "pass"
	// StatusFail ran; something did not hold.
	StatusFail Status = "fail"
	// StatusVacuous ran; had nothing to check.
	StatusVacuous Status = "vacuous"
	// StatusSkipped did not run; its input was unavailable (--full absent, so
	// the source PDFs were never looked for).
	StatusSkipped Status = "skipped"
	// StatusError could not reach a verdict. Distinct from StatusFail: a fail
	// is a claim about the corpus, an error is a claim about the harness.
	StatusError Status = "error"
)

// Check is one claim about the corpus.
//
// Implementations are values in this package and are registered by [All]. A
// check does no I/O: everything it reads is on the [Subject] it is handed, so
// the cost of loading the corpus is paid once for the whole run.
type Check interface {
	// ID is the check's stable name, and it is a contract. A future
	// data/reconciliations.yaml entry names a check by this string to record a
	// documented tolerance against it, so renaming one silently detaches its
	// reconciliation. Kebab-case, and it says what is claimed rather than what
	// is done: "facts-sorted", not "check-sort".
	ID() string
	// Tier is the tier of the check: 1 exact and zero-tolerance, 2
	// reconciliation against a document-derived tolerance, 3 cross-document, 4
	// the coverage ratchet. Structural checks return 0, which is not a tier
	// because they are not about arithmetic — an artifact that drifted or a
	// manifest hash that no longer matches invalidates every figure below it.
	Tier() int
	// Description is what the check claims, in one sentence, for the reader of
	// a report who has not read this package.
	Description() string
	// Full reports whether the check needs an input only `--full` supplies:
	// today that means the source PDFs under data/pdf/, which are large, are
	// not in a plain clone, and are deliberately not required by anything CI
	// runs. [Run] skips such a check rather than failing it.
	Full() bool
	// Run evaluates the claim over s.
	//
	// It returns an error only when the harness itself failed — an input it
	// could not parse, a state it cannot make sense of. A claim that does not
	// hold is a Result with StatusFail, naming what did not hold. Returning an
	// error for a failed claim would collapse "the corpus is wrong" into "the
	// checker is broken", and those need different responses.
	//
	// ctx may be ignored by a check whose work is bounded and in memory, which
	// is all of them today; [Run] tests for cancellation between checks.
	Run(ctx context.Context, s *Subject) (Result, error)
}

// All returns every check, in report order.
//
// It is a function rather than a package variable so that no caller can append
// to the published set, and so each call hands back a fresh slice —
// project.Registry sets that precedent.
//
// The order is the order a reader should read them in, and it is not
// alphabetical:
//
//   - Structural checks (tier 0) come first. A drifted artifact or a manifest hash
//     that no longer matches invalidates every arithmetic result below it, so a
//     reader must see it at the top rather than after nine passes. Within them:
//     the artifacts against their manifest, then the two that ask whether the
//     manifest is COMPLETE — every page emitted, and the extractor reporting no
//     failure — then the manifests against the source registry and the toolchain
//     against its pin, and last the one that needs the source documents themselves
//     and is skipped without --full. The completeness pair comes second because a
//     matching hash over an emptied extraction is the one way the sweep above can
//     pass while saying nothing.
//   - Then the fact store: its order and identity, then the two independent
//     witnesses to its figures, then the vocabularies its classifications resolve
//     in. A projection built from an unsorted store, or from amounts that do not
//     match their own source text, is not worth checking.
//   - Then whether the projections that matter were built over every fact at all,
//     because a fact outside every slice is not failed by the graph checks below,
//     it is invisible to them.
//   - Then the graph the site publishes: its shape, then its arithmetic.
//   - Then the checks that have nothing to check yet, grouped so that what this
//     command does not yet know is visible in one place rather than scattered
//     among the passes.
func All() []Check {
	return []Check{
		&artifactsMatchManifest{},
		&extractionEmittedEveryPage{},
		&extractorReportedNoErrors{},
		&manifestMatchesSourceRegistry{},
		&extractionToolchainPinned{},
		&sourcePDFsMatchBothRecords{},

		&factsSorted{},
		&factIDsUnique{},
		&factTokenReparses{},
		&factOffsetPointsAtToken{},
		&factVocabulary{},
		&factKindMatchesCategory{},
		&factDepartmentsResolve{},
		&factFundsResolve{},

		&projectionsBuild{},
		&publishedProjectionBuilt{},
		&factsAreProjected{},
		&expenditureDetailTiesToSpine{},

		&graphAcyclic{},
		&derivedNodesJustified{},
		&linkValuesTieToFacts{},
		&countsReconcile{},
		&headlineTiesToFacts{},
		&headlineTransferResidual{},
		&headlineNaiveExpenditure{},

		&transferLegsPair{},
		&aggregationInvariance{},
		&constraintTierVocabulary{},
	}
}

// Result is what one check concluded.
//
// Every field is published on every result, in declaration order and with no
// omitempty, the same discipline as fact.Fact and pkg/cmd/build's Report: a key
// that vanishes when it is empty makes a diff between two runs read as a
// structural change.
//
// CheckID, Tier and Description are stamped by [Run] from the [Check] itself
// rather than filled in by the check, so a result cannot be labelled with an id
// its check does not answer to, and so a report explains itself to a reader who
// does not have this package open.
type Result struct {
	CheckID string `json:"check_id"`
	Tier    int    `json:"tier"`
	// Description is the claim, from Check.Description.
	Description string `json:"description"`
	Status      Status `json:"status"`
	// Subjects is how many things the check actually looked at — links, facts,
	// nodes, transfer ids; each check's Summary says which. Zero is what makes
	// a vacuous result legible: it is the difference between "the legs pair up"
	// and "there are no legs".
	Subjects int `json:"subjects"`
	// Summary is one line of prose about what was checked, in the check's own
	// units. For a vacuous result it says what was absent, not "0 subjects".
	Summary string `json:"summary"`
	// Findings name what did not hold, one entry per subject at fault. It is
	// non-nil and empty rather than null on a result that found nothing wrong.
	Findings []Finding `json:"findings"`

	// ReconciliationID names the data/reconciliations.yaml entry that documents
	// this result's tolerance, and DeltaCents the difference the check observed
	// against the figure the document prints. Both belong to tier 2
	// (fisc-1wr.2), which is not built yet: today every result carries "" and
	// 0. They are published now, empty, so that landing tier 2 changes values
	// in this report rather than its shape.
	ReconciliationID string `json:"reconciliation_id"`
	DeltaCents       int64  `json:"delta_cents"`
}

// Finding is one subject that did not hold.
//
// Subject names the thing at fault in whatever vocabulary the check works in —
// a fact id, a node id, a link's two endpoints — because a report that says
// only "3 links do not tie" is a report nobody can act on.
type Finding struct {
	Subject string `json:"subject"`
	Detail  string `json:"detail"`
}

// conclusion is what a check observed. It exists so that pass, fail and vacuous
// are decided in one place rather than by each check, because a check that
// forgot to notice it had nothing to look at would report a pass, and that is
// the one mistake this command exists to prevent.
type conclusion struct {
	// subjects is how many things the check looked at, and unit is what they
	// are: "facts", "links", "projections".
	//
	// The unit is carried rather than folded into the sentences below because a
	// failing check must be able to say what it examined without borrowing the
	// sentence written for the passing case. "FAIL facts-sorted: 240 facts, in
	// canonical order" is a contradiction, and it is the first thing this shape
	// got wrong.
	subjects int
	unit     string
	// held is the summary of a pass, in the check's own units and stating the
	// arithmetic where there is any.
	held string
	// nothing is the summary of a vacuous result, and it says what was ABSENT
	// ("no link carries a transfer_id") rather than restating the zero: the
	// count already says zero, and only the check knows zero of what.
	nothing  string
	findings []Finding
}

// result decides the verdict.
//
// Findings are tested before the subject count, so a check that produced a
// finding while counting no subjects reports a failure rather than a vacuous
// result. That combination is a bug in the check, and of the two ways to report
// a bug, the one that fails is the safe one.
func (c conclusion) result() Result {
	switch {
	case len(c.findings) > 0:
		return Result{
			Status:   StatusFail,
			Subjects: c.subjects,
			Summary: fmt.Sprintf("%d %s over %d %s", len(c.findings),
				plural(len(c.findings), "finding", "findings"), c.subjects, c.unit),
			Findings: c.findings,
		}
	case c.subjects == 0:
		return Result{Status: StatusVacuous, Summary: c.nothing, Findings: []Finding{}}
	default:
		return Result{Status: StatusPass, Subjects: c.subjects, Summary: c.held, Findings: []Finding{}}
	}
}

// joinComma renders a list for prose.
func joinComma(items []string) string { return strings.Join(items, ", ") }

// sortedStrings returns a map's keys in order, so a report over a map does not
// change between two runs over the same corpus.
func sortedStrings[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// finding is shorthand for one entry, formatted.
func finding(subject, format string, args ...any) Finding {
	return Finding{Subject: subject, Detail: fmt.Sprintf(format, args...)}
}
