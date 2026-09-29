package verify

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/jcrussell/livermore-budget/internal/check"
	"github.com/jcrussell/livermore-budget/pkg/cmdutil"
	"github.com/jcrussell/livermore-budget/pkg/iostreams"
)

// printReport writes the report.
//
// The verdict of this command is its exit code, and the report is how a human
// reads it, so the text form goes to ErrOut and only --json goes to Out
// (byob-iostreams.3). A caller piping `fisc verify --json | jq` gets the
// document and nothing else.
func printReport(ios *iostreams.IOStreams, rep *check.Report, asJSON bool) error {
	if asJSON {
		enc := json.NewEncoder(ios.Out)
		enc.SetEscapeHTML(false)
		enc.SetIndent("", "  ")
		if err := enc.Encode(rep); err != nil {
			return fmt.Errorf("write report: %w", err)
		}
		return nil
	}
	printText(ios.ErrOut, rep)
	return nil
}

// statusWidth is the column the check id starts in. The words are aligned so
// that a reader scans one column for anything that is not PASS.
const statusWidth = 9

func printText(w io.Writer, rep *check.Report) {
	idWidth := 0
	for _, res := range rep.Results {
		if n := len(res.CheckID); n > idWidth {
			idWidth = n
		}
	}

	for _, res := range rep.Results {
		fmt.Fprintf(w, "%-*s %-*s %s\n", statusWidth, label(res.Status), idWidth, res.CheckID, res.Summary)
		// The claim is printed for everything that is not a pass. A reader
		// looking at a word like VACUOUS needs to know what did not happen, and
		// the check's own sentence is the answer; repeating it under every pass
		// would bury the ones that matter.
		if res.Status != check.StatusPass {
			fmt.Fprintf(w, "%-*s claims: %s\n", statusWidth, "", res.Description)
		}
		for _, f := range res.Findings {
			fmt.Fprintf(w, "%-*s - %s: %s\n", statusWidth, "", f.Subject, f.Detail)
		}
	}

	c := rep.Counts
	// The parenthetical is not decoration. This is the one line a reader will
	// quote, and "9 passed, 3 vacuous" invites reading twelve checks as twelve
	// verdicts about the corpus when three of them looked at nothing.
	fmt.Fprintf(w, "%d passed, %d failed, %d vacuous (nothing to check), %d skipped, %d errored\n",
		c.Pass, c.Fail, c.Vacuous, c.Skipped, c.Error)

	if c.Skipped > 0 {
		// The verb agrees along with the noun. This line was unreachable until the
		// structural checks landed the first check that needs --full, and it read
		// "1 check need --full" the first time anything printed it.
		fmt.Fprintf(w, "%d %s --full and did not run\n", c.Skipped,
			cmdutil.Plural(c.Skipped, "check needs", "checks need"))
	}
	printDeclarations(w, rep)

	stale := rep.StaleDeclarations()
	switch {
	case c.Fail > 0 || c.Error > 0:
		fmt.Fprintln(w, "verify failed")
	case len(stale) > 0:
		fmt.Fprintf(w, "verify failed: %d vacuity %s no longer describes this run\n",
			len(stale), cmdutil.Plural(len(stale), "declaration", "declarations"))
	case rep.Strict && len(rep.Undeclared) > 0:
		fmt.Fprintf(w, "verify failed: --strict, and %d vacuous %s undeclared\n",
			len(rep.Undeclared), cmdutil.Plural(len(rep.Undeclared), "check is", "checks are"))
	case c.Vacuous > 0 && len(rep.Undeclared) > 0:
		// NOT a green summary. --strict was not asked for, so the run passes,
		// but saying only "N had nothing to check" here would read as the
		// declared-and-accounted-for case below.
		fmt.Fprintf(w, "%d %s had nothing to check, %d of them undeclared; --strict "+
			"fails on those\n", c.Vacuous, cmdutil.Plural(c.Vacuous, "check", "checks"),
			len(rep.Undeclared))
	case c.Vacuous > 0:
		fmt.Fprintf(w, "%d %s had nothing to check; every one is declared with the bead "+
			"that retires it, so --strict passes\n",
			c.Vacuous, cmdutil.Plural(c.Vacuous, "check", "checks"))
	}
}

// printDeclarations prints the exemption surface on every run, whether or not
// anything is wrong with it.
//
// A declaration only visible when it breaks is one nobody re-reads. Each line
// names the bead whose landing deletes the entry.
func printDeclarations(w io.Writer, rep *check.Report) {
	for _, d := range rep.Declared {
		// A declaration for a check this run did not include says nothing about
		// the run. Whether the id names a real check is a claim about
		// check.All(), asserted in that package's tests.
		if !d.Ran() {
			continue
		}
		if d.Stale() {
			fmt.Fprintf(w, "%-*s %s: %s\n", statusWidth, "STALE", d.CheckID, d.StaleReason())
			continue
		}
		// A check that errored or was skipped reached no verdict, so this run
		// established neither that the declaration still holds nor that it has
		// gone stale. Printing the reason as though it had would assert
		// something about a corpus nothing looked at.
		if d.Status != check.StatusVacuous {
			fmt.Fprintf(w, "%-*s %s (%s): reported %s, so this run says nothing "+
				"about the declaration either way\n",
				statusWidth, "declared", d.CheckID, d.Bead, d.Status)
			continue
		}
		fmt.Fprintf(w, "%-*s %s (%s): %s\n", statusWidth, "declared", d.CheckID, d.Bead, d.Reason)
	}
	for _, id := range rep.Undeclared {
		fmt.Fprintf(w, "%-*s %s: vacuous and named by no entry in declaredVacuous; "+
			"add one with its reason and the bead that retires it, or find the check "+
			"a subject\n", statusWidth, "UNDECL", id)
	}
}

// label is the word a status is printed as. Upper case so the column reads as a
// verdict rather than as prose.
func label(s check.Status) string {
	switch s {
	case check.StatusPass:
		return "PASS"
	case check.StatusFail:
		return "FAIL"
	case check.StatusVacuous:
		return "VACUOUS"
	case check.StatusSkipped:
		return "SKIPPED"
	case check.StatusError:
		return "ERROR"
	default:
		// internal/check turns an unrecognized status into an error before it
		// reaches a report, so this is unreachable; printing the raw value beats
		// printing a blank column if that ever stops being true.
		return string(s)
	}
}
