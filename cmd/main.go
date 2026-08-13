// Command agon opens the debate TUI at the main menu. It takes no
// flags: everything (topic, mode, tone, rounds, model) is entered through
// the on-screen new-debate form (see docs/SPEC.md D7, D8).
package main

import (
	"fmt"
	"os"

	"github.com/trollLemon/agon/internal/orchestrator"
	"github.com/trollLemon/agon/internal/tui"
)

// archiveDir is where finished debates are written, one JSON file per
// session (docs/SPEC.md D5).
const archiveDir = "debates"

func main() {
	if err := tui.Run(tui.Options{ArchiveDir: archiveDir}, orchestrator.NewKronkEngine()); err != nil {
		fmt.Fprintln(os.Stderr, "agon:", err)
		os.Exit(1)
	}
}
