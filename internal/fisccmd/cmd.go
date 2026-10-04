// Package fisccmd holds the top-level runner: the one place that turns an
// error into an exit code, and the only place outside main that knows exit
// codes exist at all.
package fisccmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/jcrussell/livermore-budget/internal/hint"
	"github.com/jcrussell/livermore-budget/pkg/cmdutil"
	"github.com/jcrussell/livermore-budget/pkg/iostreams"
)

// Exit codes. These are a public contract: scripts branch on them.
const (
	exitOK    = 0
	exitError = 1
	exitUsage = 2
	// 128+SIGINT, the shell convention. Distinct from exitUsage so a script
	// can tell "the user pressed Ctrl-C" from "you called me wrong".
	exitCancel = 130
)

// Run executes root with args and returns the process exit code. Commands
// return errors; this function is what decides what those errors mean.
func Run(root *cobra.Command, args []string, ios *iostreams.IOStreams) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	root.SetArgs(args)
	root.SetIn(ios.In)
	root.SetOut(ios.Out)
	root.SetErr(ios.ErrOut)

	err := root.ExecuteContext(ctx)

	// A cancelled context beats whatever error the interrupt produced.
	if ctx.Err() != nil && errors.Is(ctx.Err(), context.Canceled) {
		fmt.Fprintln(ios.ErrOut, "cancelled")
		return exitCancel
	}
	return classify(err, ios)
}

// classify maps an error to an exit code and prints whatever the user needs
// to see. It is separate from Run so it can be tested without a live cobra
// tree or signal handling.
func classify(err error, ios *iostreams.IOStreams) int {
	if err == nil {
		return exitOK
	}

	// A hint only makes sense alongside the error it annotates. The silent
	// and explicit-code paths deliberately print nothing, so a bare "hint:"
	// line with no preceding error would be worse than none.
	printed := false
	defer func() {
		var h *hint.ErrHint
		if printed && errors.As(err, &h) && h.Hint != "" {
			fmt.Fprintln(ios.ErrOut, "hint:", h.Hint)
		}
	}()

	var exitCode *cmdutil.ExitCodeError
	if errors.As(err, &exitCode) {
		return exitCode.Code
	}

	switch {
	case errors.Is(err, cmdutil.ErrCancel):
		return exitCancel
	case errors.Is(err, cmdutil.ErrSilent):
		return exitError
	}

	var flagErr *cmdutil.FlagError
	if errors.As(err, &flagErr) || isUsageError(err) {
		fmt.Fprintln(ios.ErrOut, "error:", err)
		printed = true
		return exitUsage
	}

	fmt.Fprintln(ios.ErrOut, "error:", err)
	printed = true
	return exitError
}

// isUsageError recognizes the errors cobra produces as untyped strings.
// SetFlagErrorFunc covers parse failures, but an unknown command and the
// MarkFlagsMutuallyExclusive family both bypass it, so they are matched on
// their message shape here rather than at each call site.
func isUsageError(err error) bool {
	msg := err.Error()
	return strings.HasPrefix(msg, "unknown command ") ||
		strings.HasPrefix(msg, "unknown flag") ||
		strings.HasPrefix(msg, "unknown shorthand flag") ||
		strings.Contains(msg, "flags in the group [")
}
