// Command testreport renders a `go test -json` stream (read on stdin, as
// written by gotestsum's --jsonfile) into the Markdown panel that
// scripts/test-report.sh publishes to the GitHub Actions job summary and prints
// locally. It is a developer/CI tool and is not part of the shipped csync
// binary. The environment facts the test run can't know are passed as flags.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/dpassarelli/cherry-sync/internal/testreport"
)

// main parses the environment flags, summarizes stdin, and prints the panel.
func main() {
	var m testreport.Meta
	flag.StringVar(&m.Label, "label", "local", "panel heading label (the matrix leg name in CI)")
	flag.StringVar(&m.Rsync, "rsync", "", "rsync identity line for the Environment block")
	flag.StringVar(&m.Go, "go", "", "Go toolchain identity for the Environment block")
	flag.StringVar(&m.Runner, "runner", "", "runner/host identity for the Environment block")
	flag.Parse()

	summary, err := testreport.Parse(os.Stdin)
	if err != nil {
		fmt.Fprintln(os.Stderr, "testreport:", err)
		os.Exit(1)
	}
	fmt.Print(testreport.Render(summary, m))
}
