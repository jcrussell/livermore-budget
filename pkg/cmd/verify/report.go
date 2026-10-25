package verify

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/jcrussell/livermore-budget/internal/check"
	"github.com/jcrussell/livermore-budget/internal/english"
	"github.com/jcrussell/livermore-budget/pkg/iostreams"
)

// printReport writes the report: text to ErrOut, only --json to Out
// (byob-iostreams.3).
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

// statusWidth is the column the check id starts in, so the status words align.
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
		// The claim is printed for everything that is not a pass.
		if res.Status != check.StatusPass {
			fmt.Fprintf(w, "%-*s claims: %s\n", statusWidth, "", res.Description)
		}
		for _, f := range res.Findings {
			fmt.Fprintf(w, "%-*s - %s: %s\n", statusWidth, "", f.Subject, f.Detail)
		}
	}

	c := rep.Counts
	// The parenthetical stops three vacuous checks reading as verdicts.
	fmt.Fprintf(w, "%d passed, %d failed, %d vacuous (nothing to check), %d skipped, %d errored\n",
		c.Pass, c.Fail, c.Vacuous, c.Skipped, c.Error)

	if c.Skipped > 0 {
		// The verb agrees along with the noun.
		fmt.Fprintf(w, "%d %s --full and did not run\n", c.Skipped,
			english.Plural(c.Skipped, "check needs", "checks need"))
	}
	printDeclarations(w, rep)

	stale := rep.StaleDeclarations()
	switch {
	case c.Fail > 0 || c.Error > 0:
		fmt.Fprintln(w, "verify failed")
	case len(stale) > 0:
		fmt.Fprintf(w, "verify failed: %d vacuity %s no longer describes this run\n",
			len(stale), english.Plural(len(stale), "declaration", "declarations"))
	case rep.Strict && len(rep.Undeclared) > 0:
		fmt.Fprintf(w, "verify failed: --strict, and %d vacuous %s undeclared\n",
			len(rep.Undeclared), english.Plural(len(rep.Undeclared), "check is", "checks are"))
	case c.Vacuous > 0 && len(rep.Undeclared) > 0:
		// Not a green summary: it must not read as the declared case below.
		fmt.Fprintf(w, "%d %s had nothing to check, %d of them undeclared; --strict "+
			"fails on those\n", c.Vacuous, english.Plural(c.Vacuous, "check", "checks"),
			len(rep.Undeclared))
	case c.Vacuous > 0:
		fmt.Fprintf(w, "%d %s had nothing to check; every one is declared with the bead "+
			"that retires it, so --strict passes\n",
			c.Vacuous, english.Plural(c.Vacuous, "check", "checks"))
	}
}

// printDeclarations prints the exemption surface on every run, whether or not
// anything is wrong with it, each with the bead whose landing deletes it.
func printDeclarations(w io.Writer, rep *check.Report) {
	for _, d := range rep.Declared {
		// A declaration for a check this run did not include says nothing;
		// that the id names a real check is asserted in internal/check.
		if !d.Ran() {
			continue
		}
		if d.Stale() {
			fmt.Fprintf(w, "%-*s %s: %s\n", statusWidth, "STALE", d.CheckID, d.StaleReason())
			continue
		}
		// An errored or skipped check reached no verdict, so the run says
		// nothing about the declaration either way.
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

// label is the word a status is printed as, upper case so it reads as a
// verdict.
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
		// Unreachable: internal/check errors on an unknown status first.
		return string(s)
	}
}
