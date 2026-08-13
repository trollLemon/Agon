// Command goagentdisc opens the debate TUI at the main menu. It takes no
// flags: everything (topic, mode, tone, rounds, model) is entered through
// the on-screen new-debate form (see docs/SPEC.md D7, D8).
package main

import (
	"fmt"
	"os"

	"github.com/agentdisc/goagentdisc/internal/tui"
)

// archiveDir is where finished debates are written, one JSON file per
// session (docs/SPEC.md D5).
const archiveDir = "debates"

func main() {
	if err := tui.Run(tui.Options{ArchiveDir: archiveDir}); err != nil {
		fmt.Fprintln(os.Stderr, "goagentdisc:", err)
		os.Exit(1)
	}
}
