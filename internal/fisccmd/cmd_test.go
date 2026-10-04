package fisccmd

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/spf13/cobra"

	"github.com/jcrussell/livermore-budget/internal/hint"
	"github.com/jcrussell/livermore-budget/pkg/cmdutil"
	"github.com/jcrussell/livermore-budget/pkg/iostreams"
)

func TestClassify(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantCode int
		wantErr  string // substring required on ErrOut; "" means nothing printed
	}{
		{name: "nil is success", err: nil, wantCode: 0},
		{
			name: "plain error exits 1 and is reported",
			err:  errors.New("disk on fire"), wantCode: 1, wantErr: "error: disk on fire",
		},
		{
			name:     "flag error exits 2",
			err:      cmdutil.FlagErrorf("--year must be 2026 or 2027, got %d", 1999),
			wantCode: 2, wantErr: "--year must be 2026 or 2027, got 1999",
		},
		{
			name:     "wrapped flag error is still recognized",
			err:      fmt.Errorf("validate: %w", cmdutil.FlagErrorf("bad --scope")),
			wantCode: 2, wantErr: "bad --scope",
		},
		{
			name: "silent error exits 1 without printing",
			err:  cmdutil.ErrSilent, wantCode: 1, wantErr: "",
		},
		{
			name: "cancel exits 2",
			err:  cmdutil.ErrCancel, wantCode: 130, wantErr: "",
		},
		{
			name: "explicit exit code prints nothing",
			err:  &cmdutil.ExitCodeError{Code: 7}, wantCode: 7, wantErr: "",
		},
		{
			name: "unknown command is a usage error",
			err:  errors.New("unknown command \"biuld\" for \"fisc\""), wantCode: 2,
			wantErr: "unknown command",
		},
		{
			name:     "mutually exclusive flags are a usage error",
			err:      errors.New("if any flags in the group [a b] are set none of the others can be"),
			wantCode: 2, wantErr: "flags in the group",
		},
		{
			name:     "hint is printed after the error",
			err:      hint.With(errors.New("no manifest"), "run make extract"),
			wantCode: 1, wantErr: "hint: run make extract",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, errOut := iostreams.Test()
			got := classify(tt.err, ios)
			if diff := cmp.Diff(tt.wantCode, got); diff != "" {
				t.Errorf("exit code mismatch (-want +got):\n%s", diff)
			}
			stderr := errOut.String()
			switch {
			case tt.wantErr == "" && stderr != "":
				t.Errorf("got stderr %q, want nothing printed", stderr)
			case tt.wantErr != "" && !strings.Contains(stderr, tt.wantErr):
				t.Errorf("got stderr %q, want it to contain %q", stderr, tt.wantErr)
			}
		})
	}
}

// TestClassifySilentErrorStaysSilent guards the property that matters most
// about ErrSilent: the caller already told the user what happened, so the
// runner must not tell them again.
func TestClassifySilentErrorStaysSilent(t *testing.T) {
	ios, _, out, errOut := iostreams.Test()
	if got := classify(fmt.Errorf("wrapped: %w", cmdutil.ErrSilent), ios); got != 1 {
		t.Errorf("got exit code %d, want 1", got)
	}
	if errOut.Len() != 0 || out.Len() != 0 {
		t.Errorf("got stdout %q and stderr %q, want both empty", out, errOut)
	}
}

func TestRunReportsUnknownCommandAsUsageError(t *testing.T) {
	ios, _, _, errOut := iostreams.Test()
	root := &cobra.Command{Use: "fisc", SilenceErrors: true, SilenceUsage: true}
	root.AddCommand(&cobra.Command{Use: "build", RunE: func(*cobra.Command, []string) error { return nil }})

	if got := Run(root, []string{"biuld"}, ios); got != 2 {
		t.Errorf("got exit code %d, want 2", got)
	}
	if !strings.Contains(errOut.String(), "unknown command") {
		t.Errorf("got stderr %q, want it to mention the unknown command", errOut)
	}
}

func TestRunSucceeds(t *testing.T) {
	ios, _, out, _ := iostreams.Test()
	root := &cobra.Command{Use: "fisc", SilenceErrors: true, SilenceUsage: true}
	root.AddCommand(&cobra.Command{
		Use: "build",
		RunE: func(c *cobra.Command, _ []string) error {
			fmt.Fprintln(c.OutOrStdout(), "built")
			return nil
		},
	})

	if got := Run(root, []string{"build"}, ios); got != 0 {
		t.Errorf("got exit code %d, want 0", got)
	}
	if diff := cmp.Diff("built\n", out.String()); diff != "" {
		t.Errorf("stdout mismatch (-want +got):\n%s", diff)
	}
}

// TestHintIsNotPrintedWithoutAnError guards the contract that ErrSilent and
// ExitCodeError print nothing. A hint is an annotation on a reported error;
// emitted alone it is a bare "hint:" line with nothing to annotate.
func TestHintIsNotPrintedWithoutAnError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantCode int
	}{
		{"silent", hint.With(cmdutil.ErrSilent, "run make extract"), 1},
		{"explicit exit code", hint.With(&cmdutil.ExitCodeError{Code: 3}, "see the log"), 3},
		{"cancel", hint.With(cmdutil.ErrCancel, "nothing to undo"), 130},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, out, errOut := iostreams.Test()
			if got := classify(tt.err, ios); got != tt.wantCode {
				t.Errorf("got exit code %d, want %d", got, tt.wantCode)
			}
			if out.Len() != 0 || errOut.Len() != 0 {
				t.Errorf("got stdout %q and stderr %q, want both empty", out, errOut)
			}
		})
	}
}

// TestCancelIsDistinctFromUsage: the exit codes are a public contract, so a
// Ctrl-C must not look like a typo'd flag.
func TestCancelIsDistinctFromUsage(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	cancel := classify(cmdutil.ErrCancel, ios)
	usage := classify(cmdutil.FlagErrorf("bad flag"), ios)
	if cancel == usage {
		t.Errorf("cancel and usage both exit %d; they must differ", cancel)
	}
	if cancel != 130 {
		t.Errorf("got cancel exit %d, want 130 (128+SIGINT)", cancel)
	}
}
