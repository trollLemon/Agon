package tui

import (
	"context"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/trollLemon/agon/internal/archive"
	"github.com/trollLemon/agon/internal/orchestrator"
	"github.com/trollLemon/agon/internal/tools"
)

// liveSnapshot is a rendering-friendly, point-in-time view of a debate:
// completed messages plus whatever turn is currently in progress.
type liveSnapshot struct {
	sessionID string
	title     string
	topic     string
	mode      string
	tone      string
	rounds    int
	sides     []archive.Side

	messages []archive.Message

	currentRole    string
	currentRound   int
	currentContent string
	currentTools   []archive.ToolCall

	verdict string
	live    bool
	done    bool
	err     error
}

// liveDebate runs an orchestrator.Debate in a background goroutine that is
// independent of whether any screen is watching it, continuously drains its
// events into an accumulated snapshot, and writes the archive exactly once
// on success.
type liveDebate struct {
	sessionID  string
	cfg        orchestrator.Config
	debate     *orchestrator.Debate
	archiveDir string

	drained chan struct{} // closed once drainEvents has processed every event

	mu             sync.Mutex
	messages       []archive.Message
	currentRole    string
	currentRound   int
	currentContent strings.Builder
	currentTools   []archive.ToolCall
	verdict        string
	done           bool
	result         archive.Session
	err            error
}

// startLiveDebate creates a Debate from cfg and launches it, along with the
// goroutines that drain its events and persist the archive on completion.
func startLiveDebate(cfg orchestrator.Config, client orchestrator.ChatClient, sandbox *tools.Sandbox, archiveDir string) *liveDebate {
	ld := &liveDebate{
		sessionID:  cfg.SessionID,
		cfg:        cfg,
		debate:     orchestrator.New(cfg, client, sandbox),
		archiveDir: archiveDir,
		drained:    make(chan struct{}),
	}
	go ld.drainEvents()
	go ld.run()
	return ld
}

// drainEvents continuously consumes the debate's event stream and folds it
// into the accumulated snapshot. It runs for the debate's whole lifetime,
// regardless of whether a screen is currently displaying it.
func (ld *liveDebate) drainEvents() {
	defer close(ld.drained)
	for ev := range ld.debate.Events() {
		ld.mu.Lock()
		switch ev.Kind {
		case orchestrator.EventTurnStart:
			ld.currentRole = ev.Role
			ld.currentRound = ev.Round
			ld.currentContent.Reset()
			ld.currentTools = nil
		case orchestrator.EventToken:
			ld.currentContent.WriteString(ev.Text)
		case orchestrator.EventToolCall:
			if ev.Tool != nil {
				ld.currentTools = append(ld.currentTools, *ev.Tool)
			}
		case orchestrator.EventTurnEnd:
			if ev.Role != string(orchestrator.RoleJudge) {
				ld.messages = append(ld.messages, archive.Message{
					Role: ev.Role, Round: ev.Round, Content: ld.currentContent.String(),
					ToolCalls: append([]archive.ToolCall(nil), ld.currentTools...),
				})
			}
			ld.currentRole = ""
			ld.currentRound = 0
			ld.currentContent.Reset()
			ld.currentTools = nil
		case orchestrator.EventVerdict:
			ld.verdict = ev.Text
		case orchestrator.EventAborted, orchestrator.EventError:
			// Terminal state is recorded by run() once Run() returns.
		}
		ld.mu.Unlock()
	}
}

// run drives the debate to completion and, on success, writes the archive
// exactly once before marking the snapshot done. It waits
// for drainEvents to finish folding every event first, so a caller that
// sees done=true always sees the final verdict too.
func (ld *liveDebate) run() {
	sess, err := ld.debate.Run(context.Background())
	<-ld.drained

	ld.mu.Lock()
	ld.done = true
	ld.err = err
	if err == nil {
		ld.result = sess
	}
	ld.mu.Unlock()

	if err == nil {
		_ = archive.Write(ld.archiveDir, sess)
	}
}

func (ld *liveDebate) isDone() bool {
	if ld == nil {
		return true
	}
	ld.mu.Lock()
	defer ld.mu.Unlock()
	return ld.done
}

func (ld *liveDebate) snapshot() liveSnapshot {
	ld.mu.Lock()
	defer ld.mu.Unlock()
	sides := make([]archive.Side, 2)
	sides[0], sides[1] = ld.cfg.Sides[0], ld.cfg.Sides[1]

	// Once the debate has finished successfully, ld.result is the
	// authoritative Session returned by Debate.Run(). Prefer it over the
	// incrementally-folded event data: emit() drops events under a full
	// buffer rather than blocking a debate on a slow listener (see
	// orchestrator.Debate.emit), so the event-derived messages/verdict can
	// in rare cases be incomplete even though the run itself succeeded.
	messages := ld.messages
	verdict := ld.verdict
	if ld.done && ld.err == nil {
		messages = ld.result.Messages
		verdict = ld.result.Verdict
	}

	return liveSnapshot{
		sessionID:      ld.sessionID,
		title:          ld.cfg.Title,
		topic:          ld.cfg.Topic,
		mode:           string(ld.cfg.Mode),
		tone:           string(ld.cfg.Tone),
		rounds:         ld.cfg.Rounds,
		sides:          sides,
		messages:       append([]archive.Message(nil), messages...),
		currentRole:    ld.currentRole,
		currentRound:   ld.currentRound,
		currentContent: ld.currentContent.String(),
		currentTools:   append([]archive.ToolCall(nil), ld.currentTools...),
		verdict:        verdict,
		live:           true,
		done:           ld.done,
		err:            ld.err,
	}
}

// liveUpdateInterval is how often the UI polls a running debate for a
// fresh snapshot. Polling (rather than waking on a per-event signal) avoids
// a whole class of coalescing races: Debate.emit() drops events under a
// full buffer instead of blocking a debate on a slow listener, and a
// one-shot "wake on notify" channel can similarly miss a wake-up if it
// arrives while nothing is listening yet, freezing the live view until
// some unrelated tea.Msg (a keypress, a resize) forces a re-render. A
// steady poll always re-reads the latest state, so it can't get stuck.
const liveUpdateInterval = 80 * time.Millisecond

// waitForLiveUpdate schedules the next poll of ld's snapshot.
func waitForLiveUpdate(ld *liveDebate) tea.Cmd {
	return tea.Tick(liveUpdateInterval, func(time.Time) tea.Msg {
		return liveUpdateMsg{sessionID: ld.sessionID}
	})
}
