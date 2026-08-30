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
	"github.com/trollLemon/agon/internal/archive"
	"github.com/trollLemon/agon/internal/orchestrator"
	"github.com/trollLemon/agon/internal/prompts"
)

// Screen identifies which of the app's screens is rendered. Session view is
// exclusive: while it is active, nothing else is drawn (D8).
type Screen int

const (
	ScreenMenu Screen = iota
	ScreenForm
	ScreenBootstrap
	ScreenSession
	ScreenArchive
	ScreenQueue
)

// SwitchScreenMsg requests a screen change.
type SwitchScreenMsg struct{ Screen Screen }

// StartDebateMsg carries a validated new-debate form submission.
type StartDebateMsg struct {
	Topic   string
	Context string
	Sandbox string
	Mode    prompts.Mode
	Tone    prompts.Tone
	Rounds  int
	Model   string
}

// BootstrapDoneMsg reports the result of lazily initializing the model
// engine (loading the chosen model).
type BootstrapDoneMsg struct {
	Err error
}

// OpenArchivedMsg requests opening a session (live or archived) by id.
type OpenArchivedMsg struct{ SessionID string }

// LiveUpdateMsg signals that a live debate has new events or finished; the
// root model re-renders whichever screen is displaying that session.
type LiveUpdateMsg struct{ SessionID string }

// DebateProgressMsg carries one orchestrator Event for the live view.
type DebateProgressMsg struct {
	SessionID string
	Event     orchestrator.Event
}

// ArchiveListLoadedMsg carries a freshly reloaded archive listing.
type ArchiveListLoadedMsg struct{ Items []archive.Session }
