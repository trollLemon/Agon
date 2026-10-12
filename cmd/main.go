// Copyright (C) 2026  trolllemon
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

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

// cacheDir holds in-progress debates, one subdir per session with
// SESSION_CACHE and IN_PROGRESS sentinel.
const cacheDir = "cache"

func main() {
	if err := app.Run(app.Options{ArchiveDir: archiveDir, CacheDir: cacheDir}, orchestrator.NewKronkEngine()); err != nil {
		fmt.Fprintln(os.Stderr, "agon:", err)
		os.Exit(1)
	}
}
