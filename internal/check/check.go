// Package check is the engine behind `fisc verify`.
//
// A [Check] is one claim. It concludes a [Result] carrying a [Status] over a
// [Subject], loaded once. [Run] collects results into a [Report]; exit codes
// and streams belong to pkg/cmd/verify.
//
// A check that ran over nothing concludes [StatusVacuous], counted apart from
// the passes and failing only under --strict (fisc-4rh, fisc-1wr.4).
// [Result.Subjects] is what makes a vacuous result legible.
//
// # What these checks can witness
//
// [Load] builds the projection graphs from the same facts it hands the checks,
// so a check comparing internal/project's figure with a re-derivation here
// cannot witness a wrong amount; it catches output that contradicts a rule the
// producer states. A wrong definition moves both sides together and is caught
// only by internal/project's tests and goldens. The independent witnesses are
// fact-token-reparses and fact-offset-points-at-token. The structural checks
// (tier 0) witness no figure; they say whether the substrate is still the one
// the rules were written against.
//
// [Load] reads only committed artifacts; without Full it never opens data/pdf/
// (AGENTS.md, "The extraction boundary"). A check that needs the source PDFs
// says so with Full and is skipped, not failed, without --full.
package check

import (
	"context"
	"fmt"

	"github.com/jcrussell/livermore-budget/internal/english"
)

// Status is what a check concluded.
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

// Check is one claim about the corpus. A check does no I/O: everything it
// reads is on the [Subject].
type Check interface {
	// ID is the check's stable name and a contract: reports are grepped for
	// it and vacuity.go declarations name it. Kebab-case, naming the claim.
	ID() string
	// Tier: 1 exact, 2 reconciliation within a document-derived tolerance, 3
	// cross-document, 4 the coverage ratchet; 0 is structural, not arithmetic.
	Tier() int
	// Description is the claim in one sentence for a report reader.
	Description() string
	// Full reports whether the check needs the source PDFs only `--full`
	// supplies. [Run] skips such a check rather than failing it.
	Full() bool
	// Run evaluates the claim over s. It returns an error only when the
	// harness failed; a claim that does not hold is a Result with StatusFail.
	Run(ctx context.Context, s *Subject) (Result, error)
}

// All returns every check, in report order, as a fresh slice no caller can
// append to the published set through.
//
// The order is not alphabetical: structural checks first, since a drifted
// substrate invalidates everything below (the completeness pair right after
// the manifest sweep, since a matching hash over an emptied extraction passes
// it saying nothing); then the fact store and its two independent witnesses;
// then projection coverage; then the published graph, shape before
// arithmetic; then the checks with nothing to check yet, in one place.
func All() []Check {
	return publish(
		&artifactsMatchManifest{},
		&extractionEmittedEveryPage{},
		&extractorReportedNoErrors{},
		&manifestMatchesSourceRegistry{},
		&extractionToolchainPinned{},
		&sourcePDFsMatchBothRecords{},

		&factsSorted{},
		&factIDsUnique{},
		&factIDsRecompute{},
		&factTokenReparses{},
		&factOffsetPointsAtToken{},
		&factCitationsAreDeclared{},
		&factOffsetIsNotAStatedTotal{},
		&factTransferOrientationIsDeclared{},
		&factVocabulary{},
		&factKindMatchesCategory{},
		&factDepartmentsResolve{},
		&factFundsResolve{},
		&factRevenueLinesResolve{},
		&ruleFundsMatchTheirHeadings{},
		&rowFundsMatchTheirAnchors{},
		&fundBalanceIdentity{},
		&excessOfRevenuesIdentity{},
		&fundGroupSourcesEqualUses{},

		&projectionsBuild{},
		&publishedProjectionBuilt{},
		&documentsAreChecked{},
		&cutsTieAlongTheLattice{},
		&peersOverlapOnlyByDeclaredIdentity{},
		&trendPointsTieToFacts{},
		&trendSeriesAreComplete{},
		&debtServiceTiesToFacts{},

		&graphAcyclic{},
		&nodeTiersAreDeclared{},
		&derivedNodesJustified{},
		&linkValuesTieToFacts{},
		&linkLocatorsMatchTheirFacts{},
		&nodeBalancesTieToFacts{},
		&fundGroupsAreTheirPrintedRows{},
		&linkEndsMatchTheirFacts{},
		&linkKindsMatchTheirFacts{},
		&contraLinksNameTheirSchedule{},
		&countsReconcile{},
		&revenueLinesTieToTheirCategories{},
		&headlineTiesToFacts{},
		&headlineTransferResidual{},
		&headlineNaiveExpenditure{},

		&transferLegsPair{},
		&nodeHierarchyWellFormed{},
		&constraintTierVocabulary{},
		&expansionOpensAsTheSpine{},
		&expansionSplitsTie{},
	)
}

// meta is what a check of this package declares about itself, once, in its
// meta method; [Check] documents each field as the method it answers.
type meta struct {
	id          string
	tier        int
	full        bool
	description string
}

// local is a check of this package: its claim and the meta it declares.
type local interface {
	meta() meta
	Run(ctx context.Context, s *Subject) (Result, error)
}

// published is a local check as [All] hands it out, answering [Check]'s
// identity methods from its meta.
type published struct{ local }

func (p published) ID() string          { return p.meta().id }
func (p published) Tier() int           { return p.meta().tier }
func (p published) Full() bool          { return p.meta().full }
func (p published) Description() string { return p.meta().description }

func publish(cs ...local) []Check {
	out := make([]Check, len(cs))
	for i, c := range cs {
		out[i] = published{c}
	}
	return out
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

	// A RESULT CARRIES NO RECONCILIATION ID AND NO DELTA. Tier 2's comparison
	// is internal/mapping's compareTotals, at BUILD time, where a total that
	// does not tie fails the build. A verify-time re-run would be two
	// functions over identical input in one process -- the distinction this
	// package's own doc comment draws -- so it would add availability and not
	// independence, and CI already rebuilds facts.jsonl and diffs it byte for
	// byte. The slack a tolerance allowed is reported by `fisc
	// build` as columns_tied_by_tolerance and tolerance_slack.
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
				english.Plural(len(c.findings), "finding", "findings"), c.subjects, c.unit),
			Findings: c.findings,
		}
	case c.subjects == 0:
		return Result{Status: StatusVacuous, Summary: c.nothing, Findings: []Finding{}}
	default:
		return Result{Status: StatusPass, Subjects: c.subjects, Summary: c.held, Findings: []Finding{}}
	}
}

// finding is shorthand for one entry, formatted.
func finding(subject, format string, args ...any) Finding {
	return Finding{Subject: subject, Detail: fmt.Sprintf(format, args...)}
}
