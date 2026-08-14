package main

import (
	"fmt"
	"os"

	"github.com/trollLemon/agon/internal/app"
	"github.com/trollLemon/agon/internal/orchestrator"
)

// archiveDir is where finished debates are written, one JSON file per
// session.
const archiveDir = "debates"

func main() {
	if err := app.Run(app.Options{ArchiveDir: archiveDir}, orchestrator.NewKronkEngine()); err != nil {
		fmt.Fprintln(os.Stderr, "agon:", err)
		os.Exit(1)
	}
}
