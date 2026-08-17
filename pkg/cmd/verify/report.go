package verify

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/jcrussell/livermore-budget/internal/check"
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
			plural(c.Skipped, "check needs", "checks need"))
	}
	switch {
	case c.Fail > 0 || c.Error > 0:
		fmt.Fprintln(w, "verify failed")
	case rep.Strict && c.Vacuous > 0:
		fmt.Fprintf(w, "verify failed: --strict, and %d %s had nothing to check\n",
			c.Vacuous, plural(c.Vacuous, "check", "checks"))
	case c.Vacuous > 0:
		fmt.Fprintf(w, "%d %s had nothing to check; that is not a pass, and --strict fails on it\n",
			c.Vacuous, plural(c.Vacuous, "check", "checks"))
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

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
