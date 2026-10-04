package check

import (
	"fmt"
	"sort"
)

// A VACUOUS CHECK IS DECLARED OR --strict FAILS ON IT.
//
// `fisc verify --strict` fails on Counts.Vacuous > 0, and a check blocked on
// work no mapping lane reaches would hold that gate red until it was commented
// out. A declaration is loud, carries its reason and the bead that retires it,
// and FAILS WHEN IT GOES STALE -- the shape of uncheckedDocuments,
// mapping.StatedTotalDeltas and mapping.Part.OmittedCells.
//
// It weakens nothing: a declared check is still reported VACUOUS, still counted
// in Counts.Vacuous, and still printed with its reason.
type vacancy struct {
	// reason is why the check has nothing to look at, said about the CORPUS or
	// the code rather than about our intentions. It is printed verbatim on
	// every run, so an overstatement here is published every time.
	reason string
	// bead is the work whose landing deletes this entry. An entry with no bead
	// is a permanent exemption, which is not what this mechanism is for.
	bead string
}

// declaredVacuous is the whole list, and every entry earns its place by naming
// what would have to exist for the check to have a subject. [Run] reads it off
// Subject.Vacancies, which [Load] fills from here.
var declaredVacuous = map[string]vacancy{}

// declaration is one declared vacancy and what its check actually reported on
// this run.
//
// Status is carried because it is the whole self-retiring half: a declaration
// whose check has stopped being vacuous is a statement that has stopped being
// true, and the report says so rather than leaving the entry to outlive the
// work it was waiting for.
type declaration struct {
	CheckID string `json:"check_id"`
	Reason  string `json:"reason"`
	Bead    string `json:"bead"`
	// Status is what the check reported, or "" when the run contained no such
	// check at all — a declaration for a check that has been renamed or
	// deleted.
	Status Status `json:"status"`
}

// Ran says whether this run contained the check at all.
//
// A run may legitimately be a subset: [Run] takes the checks it is given, and
// callers pass a few. So "absent" cannot mean stale, and the first version of
// this file made it mean exactly that -- every test running two checks reported
// three stale declarations and a clean run exited 3. Whether an id names a real
// check is a claim about [All] rather than about a run, and it is asserted
// statically by TestEveryDeclarationNamesACheckThatExists.
func (d declaration) Ran() bool { return d.Status != "" }

// Stale says whether this declaration has stopped describing the run: the check
// reached a verdict OTHER than vacuous, so the work it was waiting for has
// landed and the entry should have gone with it.
//
// This fails whether or not --strict was passed, because a declaration that has
// stopped being true is a false statement in this package's source rather than
// a shortfall in coverage -- the same standard staleDocumentDeclarations
// applies to uncheckedDocuments.
//
// ONLY PASS AND FAIL ARE VERDICTS, and the other two statuses are why this is
// a whitelist rather than "anything but vacuous". A run that reports SKIPPED or
// ERROR did not find the check's subject; it did not look.
//
//   - ERROR IS REACHABLE AND WAS THE LIVE BUG. run1 turns ctx.Err() into
//     errored(...), so a cancelled or timed-out `fisc verify` reported every
//     declaration stale and exited 3 saying "<bead> landed, so remove the
//     declaration" -- about work that had not landed, in answer to a Ctrl-C.
//     A genuine error still fails the run, through Counts.Error in
//     [Report.Failed], which says the checker could not tell rather than
//     claiming a bead landed.
//   - SKIPPED is not reachable today and is guarded anyway:
//     sourcePDFsMatchBothRecords is the only Full() check in the tree and
//     nobody would declare it vacuous, but the next --full check inherits the
//     hole, and ci.yml's --strict step passes no --full.
func (d declaration) Stale() bool {
	return d.Status == StatusPass || d.Status == StatusFail
}

// StaleReason says what to do about it, for the report to print.
func (d declaration) StaleReason() string {
	return fmt.Sprintf("declared vacuous and reported %s; %s landed, so remove the "+
		"declaration rather than leaving it to excuse a check that no longer "+
		"needs excusing", d.Status, d.Bead)
}

// resolveDeclarations settles every declaration in declared against what this
// run found, and lists the vacuous checks no declaration covers.
//
// Called once after every check has run, because it is a claim about the report
// rather than about the corpus and no Check could reach it: a Check is handed a
// Subject, not its siblings' verdicts.
func (r *Report) resolveDeclarations(declared map[string]vacancy) {
	status := make(map[string]Status, len(r.Results))
	for _, res := range r.Results {
		status[res.CheckID] = res.Status
	}

	ids := make([]string, 0, len(declared))
	for id := range declared {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	r.Declared = make([]declaration, 0, len(ids))
	for _, id := range ids {
		v := declared[id]
		r.Declared = append(r.Declared, declaration{
			CheckID: id, Reason: v.reason, Bead: v.bead, Status: status[id],
		})
	}

	r.Undeclared = []string{}
	for _, res := range r.Results {
		if res.Status != StatusVacuous {
			continue
		}
		if _, ok := declared[res.CheckID]; !ok {
			r.Undeclared = append(r.Undeclared, res.CheckID)
		}
	}
}

// StaleDeclarations are the entries that have stopped being true.
func (r *Report) StaleDeclarations() []declaration {
	var out []declaration
	for _, d := range r.Declared {
		if d.Stale() {
			out = append(out, d)
		}
	}
	return out
}
