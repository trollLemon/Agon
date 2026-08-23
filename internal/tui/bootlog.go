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

package tui

import (
	"fmt"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// maxBootLogLines caps how many lines BootLog retains, so a very chatty
// bootstrap (e.g. a slow download logging progress for minutes) can't grow
// its buffer unboundedly.
const maxBootLogLines = 500

// BootLog accumulates the model bootstrap's log lines (native library
// detection/download, model download, model load) so the bootstrap screen
// can show them live instead of leaving the user staring at a bare "Loading
// model…" with no way to tell whether it's stuck or just downloading a
// multi-gigabyte file. Safe for concurrent use: Append is called from
// the bootstrap goroutine while the UI reads Tail from the Bubble Tea
// event loop.
type BootLog struct {
	mu          sync.Mutex
	lines       []string
	lastIsPatch bool // true if the last line came from a '\r'-prefixed (in-place progress) message
}

func NewBootLog() *BootLog { return &BootLog{} }

// Append records one logger call. A leading '\r' on msg (kronk's convention
// for "overwrite the current terminal line", used for download-progress
// updates) replaces the previous line instead of appending a new one, so a
// rapid stream of progress percentages doesn't flood the visible tail with
// near-duplicate lines.
// Long messages are wrapped around so that smaller windows can see the full text.
func (b *BootLog) Append(msg string, args ...any) {
	patch := strings.HasPrefix(msg, "\r")
	if patch {
		msg = msg[1:]
	}

	var sb strings.Builder
	sb.WriteString(msg)
	for i := 0; i+1 < len(args); i += 2 {
		fmt.Fprintf(&sb, " %v[%v]", args[i], args[i+1])
	}

	line := sb.String()

	b.mu.Lock()
	defer b.mu.Unlock()
	if patch && b.lastIsPatch && len(b.lines) > 0 {
		b.lines[len(b.lines)-1] = line
	} else {
		b.lines = append(b.lines, line)
		if len(b.lines) > maxBootLogLines {
			b.lines = b.lines[len(b.lines)-maxBootLogLines:]
		}
	}
	b.lastIsPatch = patch
}

// Tail returns the most recent n lines (or fewer, if there aren't n yet).
func (b *BootLog) Tail(n int) []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.lines) <= n {
		out := make([]string, len(b.lines))
		copy(out, b.lines)
		return out
	}
	out := make([]string, n)
	copy(out, b.lines[len(b.lines)-n:])
	return out
}

// bootLogTickInterval mirrors liveUpdateInterval's reasoning (see live.go):
// the bootstrap goroutine writes to the BootLog from outside the Bubble Tea
// event loop, so the only reliable way to keep the screen refreshing while
// it's stuck on "Loading model…" is a steady poll, not a one-shot
// wake-on-write signal that could be missed.
const bootLogTickInterval = 150 * time.Millisecond

// BootLogTickMsg requests a re-render while a bootstrap is in flight.
type BootLogTickMsg struct{}

// WaitForBootLog schedules the next BootLog poll.
func WaitForBootLog() tea.Cmd {
	return tea.Tick(bootLogTickInterval, func(time.Time) tea.Msg {
		return BootLogTickMsg{}
	})
}
