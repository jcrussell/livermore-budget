package fisccmd

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/spf13/cobra"

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
			err:  cmdutil.ErrCancel, wantCode: 2, wantErr: "",
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
			err:      cmdutil.WithHint(errors.New("no manifest"), "run make extract"),
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
