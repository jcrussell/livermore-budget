// Package fisccmd holds the top-level runner, the one place that turns an
// error into an exit code.
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
	// 128+SIGINT, the shell convention.
	exitCancel = 130
)

// Run executes root with args and returns the process exit code. The first
// SIGINT or SIGTERM cancels the commands' context; a second kills the process.
func Run(root *cobra.Command, args []string, ios *iostreams.IOStreams) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		stop()
	}()
	return run(ctx, root, args, ios)
}

// run is Run under a context the caller controls. A command that succeeded
// exits 0 however late the signal came. One that failed while the context was
// cancelled exits 130, an ExitCodeError included; an error that is neither the
// cancellation nor an exit code may be a raced failure, so it is printed.
func run(ctx context.Context, root *cobra.Command, args []string, ios *iostreams.IOStreams) int {
	root.SetArgs(args)
	root.SetIn(ios.In)
	root.SetOut(ios.Out)
	root.SetErr(ios.ErrOut)
	err := root.ExecuteContext(ctx)
	if err != nil && ctx.Err() != nil {
		var code *cmdutil.ExitCodeError
		if !errors.Is(err, context.Canceled) && !errors.As(err, &code) {
			classify(err, ios)
		}
		fmt.Fprintln(ios.ErrOut, "cancelled")
		return exitCancel
	}
	return classify(err, ios)
}

// classify maps an error to an exit code and prints what the user needs. A
// wrapped context.Canceled is an ordinary error here: a command's own child
// context being cancelled is not the user pressing Ctrl-C.
func classify(err error, ios *iostreams.IOStreams) int {
	if err == nil {
		return exitOK
	}

	// A hint prints only after a printed error, never as a bare "hint:" line.
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

// isUsageError recognizes cobra's untyped usage errors: an unknown command and
// the MarkFlagsMutuallyExclusive family bypass SetFlagErrorFunc.
func isUsageError(err error) bool {
	msg := err.Error()
	return strings.HasPrefix(msg, "unknown command ") ||
		strings.HasPrefix(msg, "unknown flag") ||
		strings.HasPrefix(msg, "unknown shorthand flag") ||
		strings.Contains(msg, "flags in the group [")
}
