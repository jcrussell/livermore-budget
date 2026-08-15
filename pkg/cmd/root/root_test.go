package root

import (
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/fisccmd"
	"github.com/jcrussell/livermore-budget/pkg/cmdutil"
	"github.com/jcrussell/livermore-budget/pkg/iostreams"
)

// newTestFactory returns a Factory whose lazy dependencies would fail loudly
// if a help or version path touched them.
func newTestFactory(t *testing.T, ios *iostreams.IOStreams) *cmdutil.Factory {
	t.Helper()
	return &cmdutil.Factory{
		IOStreams: ios,
		RepoRoot: func() (string, error) {
			t.Error("RepoRoot resolved on a path that should not need it")
			return "", nil
		},
	}
}

// TestHelpTouchesNoLazyDependencies is the test that keeps the factory's
// laziness honest: --help must not pay for anything it does not use.
func TestHelpTouchesNoLazyDependencies(t *testing.T) {
	ios, _, out, _ := iostreams.Test()
	if got := fisccmd.Run(NewCmdRoot(newTestFactory(t, ios)), []string{"--help"}, ios); got != 0 {
		t.Errorf("got exit code %d, want 0", got)
	}
	if !strings.Contains(out.String(), "fisc") {
		t.Errorf("got stdout %q, want the help text", out)
	}
}

func TestVersion(t *testing.T) {
	ios, _, out, _ := iostreams.Test()
	if got := fisccmd.Run(NewCmdRoot(newTestFactory(t, ios)), []string{"--version"}, ios); got != 0 {
		t.Errorf("got exit code %d, want 0", got)
	}
	if got := strings.TrimSpace(out.String()); got == "" {
		t.Error("got empty version output, want a version line")
	}
}

// TestUnknownCommandExitsTwo is a regression test. With no subcommands
// registered, cobra treats an unknown command as a positional argument to
// root, prints the help text, and exits 0 -- so a typo'd command looks like
// success. cobra.NoArgs on root is what makes it a usage error instead.
func TestUnknownCommandExitsTwo(t *testing.T) {
	ios, _, _, errOut := iostreams.Test()
	if got := fisccmd.Run(NewCmdRoot(newTestFactory(t, ios)), []string{"biuld"}, ios); got != 2 {
		t.Errorf("got exit code %d, want 2", got)
	}
	if !strings.Contains(errOut.String(), "unknown command") {
		t.Errorf("got stderr %q, want it to report an unknown command", errOut)
	}
}

// TestNoArgsPrintsHelp confirms the bare invocation is still friendly.
func TestNoArgsPrintsHelp(t *testing.T) {
	ios, _, out, _ := iostreams.Test()
	if got := fisccmd.Run(NewCmdRoot(newTestFactory(t, ios)), nil, ios); got != 0 {
		t.Errorf("got exit code %d, want 0", got)
	}
	if !strings.Contains(out.String(), "Usage:") {
		t.Errorf("got stdout %q, want the help text", out)
	}
}

// TestUnknownFlagExitsTwo confirms SetFlagErrorFunc routes pflag's parse
// errors into the typed vocabulary rather than leaving them as exit 1.
func TestUnknownFlagExitsTwo(t *testing.T) {
	ios, _, _, errOut := iostreams.Test()
	if got := fisccmd.Run(NewCmdRoot(newTestFactory(t, ios)), []string{"--nope"}, ios); got != 2 {
		t.Errorf("got exit code %d, want 2", got)
	}
	if !strings.Contains(errOut.String(), "nope") {
		t.Errorf("got stderr %q, want it to name the offending flag", errOut)
	}
}
