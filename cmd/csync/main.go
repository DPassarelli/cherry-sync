package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/dpassarelli/cherry-sync/internal/cli"
)

func main() {
	a, err := cli.Parse(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, "usage: csync SOURCE DESTINATION")
		os.Exit(2)
	}

	fmt.Println("Source:", a.Source)
	fmt.Println("Destination:", a.Destination)

	rsyncArgs := []string{
		"--dry-run",
		"--itemize-changes",
		"--recursive",
		a.Source + "/",
		a.Destination + "/",
	}
	out, err := exec.Command("rsync", rsyncArgs...).Output()
	if err != nil {
		fmt.Fprintln(os.Stderr, "rsync:", err)
		os.Exit(1)
	}

	fmt.Println("Changes:", countChanges(string(out)))
}

// countChanges returns the number of rsync --itemize-changes lines that
// represent an actual file movement. The itemize prefix character tells us
// the kind of change: `>` send, `<` receive, `c` create, `h` hardlink,
// `*` message (e.g., deletion). Lines starting with `.` are attribute-only
// (no data change) and don't count; rsync's summary lines (e.g., "sent N
// bytes") don't start with any of these markers.
func countChanges(rsyncOut string) int {
	n := 0
	for line := range strings.SplitSeq(rsyncOut, "\n") {
		if len(line) < 12 {
			continue
		}
		switch line[0] {
		case '>', '<', 'c', 'h', '*':
			n++
		}
	}
	return n
}
