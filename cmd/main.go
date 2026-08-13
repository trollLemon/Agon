package main

import (
	"fmt"
	"os"

	"github.com/trollLemon/agon/internal/orchestrator"
	"github.com/trollLemon/agon/internal/tui"
)

// archiveDir is where finished debates are written, one JSON file per
// session.
const archiveDir = "debates"

func main() {
	if err := tui.Run(tui.Options{ArchiveDir: archiveDir}, orchestrator.NewKronkEngine()); err != nil {
		fmt.Fprintln(os.Stderr, "agon:", err)
		os.Exit(1)
	}
}
