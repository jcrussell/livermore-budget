// Command fisc builds and verifies the Livermore budget fact store and the
// static site rendered from it.
package main

import (
	"os"

	"github.com/jcrussell/livermore-budget/internal/fisccmd"
	"github.com/jcrussell/livermore-budget/pkg/cmd/root"
	"github.com/jcrussell/livermore-budget/pkg/cmdutil"
)

func main() {
	f := cmdutil.New()
	os.Exit(fisccmd.Run(root.NewCmdRoot(f), os.Args[1:], f.IOStreams))
}
