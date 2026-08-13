package tui

import (
	"github.com/trollLemon/agon/internal/archive"
	"github.com/trollLemon/agon/internal/prompts"
)

// switchScreenMsg requests a screen change.
type switchScreenMsg struct{ screen screen }

// startDebateMsg carries a validated new-debate form submission.
type startDebateMsg struct {
	topic   string
	context string
	mode    prompts.Mode
	tone    prompts.Tone
	rounds  int
	model   string
}

// bootstrapDoneMsg reports the result of lazily initializing the model
// engine (loading the chosen model). The engine itself is held on the App.
type bootstrapDoneMsg struct {
	err error
}

// openArchivedMsg requests opening a session (live or archived) by id.
type openArchivedMsg struct{ sessionID string }

// liveUpdateMsg signals that a live debate has new events or finished; the
// App re-renders whichever screen is displaying that session.
type liveUpdateMsg struct{ sessionID string }

// archiveListLoadedMsg carries a freshly reloaded archive listing.
type archiveListLoadedMsg struct{ items []archive.Session }
