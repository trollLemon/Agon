package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/agentdisc/goagentdisc/internal/archive"
	"github.com/agentdisc/goagentdisc/internal/prompts"
	"github.com/agentdisc/goagentdisc/internal/tools"
)

// maxToolIterations bounds how many tool-call round-trips a single turn may
// make, so a small model looping on tool calls can't stall a debate forever.
const maxToolIterations = 8

// maxToolResultSummary bounds how much of a tool result is kept in the
// archived ToolCall.ResultSummary.
const maxToolResultSummary = 120

// AbortedError is returned by Run when a debate was explicitly aborted via
// Debate.Abort.
type AbortedError struct{ Reason string }

func (e *AbortedError) Error() string { return "debate aborted: " + e.Reason }

// sideRuntime is a debater's persistent per-role conversation.
type sideRuntime struct {
	roleName      string
	label         string
	opponentLabel string
	leads         bool
	history       []ChatMessage
}

// Debate runs one two-agent debate to a verdict. Create with New, drive with
// Run, and optionally cut it short with Abort from another goroutine.
type Debate struct {
	cfg     Config
	client  ChatClient
	sandbox *tools.Sandbox

	events chan Event

	mu      sync.Mutex
	aborted string
	cancel  context.CancelFunc
}

// New creates a Debate. If cfg.SandboxDirs is set, sandbox must be a *tools.
// Sandbox over those directories (nil disables tool grounding regardless of
// SandboxDirs).
func New(cfg Config, client ChatClient, sandbox *tools.Sandbox) *Debate {
	return &Debate{
		cfg:     cfg,
		client:  client,
		sandbox: sandbox,
		events:  make(chan Event, 256),
	}
}

// Events returns the channel of streamed turn/token/tool/verdict events. It
// is closed when Run returns.
func (d *Debate) Events() <-chan Event { return d.events }

// Abort requests that the running debate stop as soon as possible. The
// in-memory transcript is discarded — Run returns an *AbortedError and a
// zero-value archive.Session. Safe to call before Run starts or multiple
// times; only the first reason sticks.
func (d *Debate) Abort(reason string) {
	d.mu.Lock()
	if d.aborted == "" {
		d.aborted = reason
	}
	cancel := d.cancel
	d.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// checkAbort reports an error if Abort was called or ctx was
// otherwise canceled.
func (d *Debate) checkAbort(ctx context.Context) error {
	d.mu.Lock()
	reason := d.aborted
	d.mu.Unlock()

	if reason != "" {
		return &AbortedError{Reason: reason}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

func (d *Debate) emit(ev Event) {
	select {
	case d.events <- ev:
	default:
		// Drop rather than block a debate that has outrun its listener; the
		// final archive.Session is authoritative regardless.
	}
}

// Run executes the full debate: advocate/critic rounds, then a single judge
// turn. On success it returns the completed archive.Session, ready to be
// written exactly once by the caller (see docs/SPEC.md D5). On error or
// abort it returns a zero-value Session and a non-nil error — nothing
// should be archived in that case.
func (d *Debate) Run(parent context.Context) (archive.Session, error) {
	ctx, cancel := context.WithCancel(parent)
	d.mu.Lock()
	d.cancel = cancel
	d.mu.Unlock()
	defer cancel()
	defer close(d.events)

	lead := d.newSideRuntime(d.cfg.Sides[0], d.cfg.Sides[1], true)
	follow := d.newSideRuntime(d.cfg.Sides[1], d.cfg.Sides[0], false)

	sess := archive.Session{
		SessionID: d.cfg.SessionID,
		Title:     d.cfg.Title,
		Topic:     d.cfg.Topic,
		Mode:      string(d.cfg.Mode),
		Tone:      string(d.cfg.Tone),
		Rounds:    d.cfg.Rounds,
		Sides:     []archive.Side{d.cfg.Sides[0], d.cfg.Sides[1]},
		Model:     d.cfg.Model,
		Dirs:      d.cfg.SandboxDirs,
		CreatedAt: d.cfg.CreatedAt,
	}

	var lastFollowerContent string
	for round := 1; round <= d.cfg.Rounds; round++ {
		if err := d.checkAbort(ctx); err != nil {
			return d.fail(err)
		}

		leadMsg := prompts.OpeningMessage(d.cfg.Topic, d.cfg.StartingContext, round, d.cfg.Rounds)
		if round > 1 {
			leadMsg = prompts.PeerMessage(follow.label, lastFollowerContent, round, d.cfg.Rounds)
		}
		content, err := d.runTurn(ctx, lead, round, &sess, leadMsg)
		if err != nil {
			return d.fail(err)
		}

		followMsg := prompts.PeerMessage(lead.label, content, round, d.cfg.Rounds)
		followContent, err := d.runTurn(ctx, follow, round, &sess, followMsg)
		if err != nil {
			return d.fail(err)
		}
		lastFollowerContent = followContent
	}

	if err := d.checkAbort(ctx); err != nil {
		return d.fail(err)
	}

	verdict, err := d.runJudgeTurn(ctx, sess)
	if err != nil {
		return d.fail(err)
	}
	sess.Verdict = verdict
	d.emit(Event{Kind: EventVerdict, Text: verdict})

	return sess, nil
}

func (d *Debate) fail(err error) (archive.Session, error) {
	var aerr *AbortedError
	if errors.As(err, &aerr) {
		d.emit(Event{Kind: EventAborted, Text: aerr.Reason})
	} else {
		d.emit(Event{Kind: EventError, Text: err.Error()})
	}
	return archive.Session{}, err
}

func (d *Debate) newSideRuntime(side, opponent archive.Side, leads bool) *sideRuntime {
	sys := prompts.DebaterSystem(prompts.DebaterParams{
		Mode:          d.cfg.Mode,
		Tone:          d.cfg.Tone,
		Label:         side.Label,
		Stance:        side.Stance,
		OpponentLabel: opponent.Label,
		Leads:         leads,
		Rounds:        d.cfg.Rounds,
		Dirs:          d.cfg.SandboxDirs,
	})
	return &sideRuntime{
		roleName:      side.ID,
		label:         side.Label,
		opponentLabel: opponent.Label,
		leads:         leads,
		history:       []ChatMessage{{Role: RoleSystem, Content: sys}},
	}
}

// runTurn appends userContent to s's history, drives the model (including
// any tool-call sub-loop), records the resulting message onto sess, and
// returns the assistant's final text.
func (d *Debate) runTurn(ctx context.Context, s *sideRuntime, round int, sess *archive.Session, userContent string) (string, error) {
	s.history = append(s.history, ChatMessage{Role: RoleUser, Content: userContent})
	d.emit(Event{Kind: EventTurnStart, Role: s.roleName, Round: round})

	var toolCallLog []archive.ToolCall
	for i := 0; i < maxToolIterations; i++ {
		if err := d.checkAbort(ctx); err != nil {
			return "", err
		}

		ch, err := d.client.ChatStreaming(ctx, Role(s.roleName), s.history, tools.Specs())
		if err != nil {
			return "", fmt.Errorf("%s: chat streaming: %w", s.roleName, err)
		}

		var content strings.Builder
		var toolCalls []ToolCallRequest
		for ev := range ch {
			if ev.Err != nil {
				return "", fmt.Errorf("%s: %w", s.roleName, ev.Err)
			}
			if ev.ContentDelta != "" {
				content.WriteString(ev.ContentDelta)
				d.emit(Event{Kind: EventToken, Role: s.roleName, Round: round, Text: ev.ContentDelta})
			}
			if len(ev.ToolCalls) > 0 {
				toolCalls = ev.ToolCalls
			}
		}

		if len(toolCalls) == 0 {
			text := content.String()
			s.history = append(s.history, ChatMessage{Role: RoleAssistant, Content: text})
			sess.Messages = append(sess.Messages, archive.Message{
				Role: s.roleName, Round: round, Content: text,
				ToolCalls: toolCallLog, TS: nowSeconds(),
			})
			d.emit(Event{Kind: EventTurnEnd, Role: s.roleName, Round: round})
			return text, nil
		}

		s.history = append(s.history, ChatMessage{Role: RoleAssistant, ToolCalls: toolCalls})
		for _, tc := range toolCalls {
			result, callErr := d.callTool(tc)
			if callErr != nil {
				result = "ERROR: " + callErr.Error()
			}
			argsJSON, _ := json.Marshal(tc.Arguments)
			te := archive.ToolCall{Name: tc.Name, Args: string(argsJSON), ResultSummary: summarize(result)}
			toolCallLog = append(toolCallLog, te)
			d.emit(Event{Kind: EventToolCall, Role: s.roleName, Round: round, Tool: &te})
			s.history = append(s.history, ChatMessage{
				Role: RoleTool, Content: result, ToolCallID: tc.ID, ToolName: tc.Name,
			})
		}
	}
	return "", fmt.Errorf("%s: exceeded max tool iterations (%d)", s.roleName, maxToolIterations)
}

func (d *Debate) callTool(tc ToolCallRequest) (string, error) {
	if d.sandbox == nil {
		return "", fmt.Errorf("no repository is in scope for this debate")
	}
	return tools.Call(d.sandbox, tc.Name, tc.Arguments)
}

func (d *Debate) runJudgeTurn(ctx context.Context, sess archive.Session) (string, error) {
	if err := d.checkAbort(ctx); err != nil {
		return "", err
	}
	d.emit(Event{Kind: EventTurnStart, Role: string(RoleJudge)})

	history := []ChatMessage{
		{Role: RoleSystem, Content: prompts.JudgeSystem(d.cfg.Mode)},
		{Role: RoleUser, Content: prompts.JudgeUserMessage(d.cfg.Topic, renderTranscript(sess))},
	}
	ch, err := d.client.ChatStreaming(ctx, RoleJudge, history, nil)
	if err != nil {
		return "", fmt.Errorf("judge: chat streaming: %w", err)
	}

	var content strings.Builder
	for ev := range ch {
		if ev.Err != nil {
			return "", fmt.Errorf("judge: %w", ev.Err)
		}
		if ev.ContentDelta != "" {
			content.WriteString(ev.ContentDelta)
			d.emit(Event{Kind: EventToken, Role: string(RoleJudge), Text: ev.ContentDelta})
		}
	}
	d.emit(Event{Kind: EventTurnEnd, Role: string(RoleJudge)})
	return content.String(), nil
}

// renderTranscript renders a session's messages as plain text, in the order
// they were produced, for the judge's single read.
func renderTranscript(sess archive.Session) string {
	labels := make(map[string]string, len(sess.Sides))
	for _, s := range sess.Sides {
		labels[s.ID] = s.Label
	}
	var b strings.Builder
	for _, m := range sess.Messages {
		label := labels[m.Role]
		if label == "" {
			label = m.Role
		}
		fmt.Fprintf(&b, "[Round %d] %s:\n%s\n\n", m.Round, label, m.Content)
	}
	return b.String()
}

// summarize trims a tool result down to a short, archivable summary.
func summarize(s string) string {
	s = strings.TrimSpace(s)
	runes := []rune(s)
	if len(runes) <= maxToolResultSummary {
		return fmt.Sprintf("%d bytes: %s", len(s), s)
	}
	return fmt.Sprintf("%d bytes: %s…", len(s), string(runes[:maxToolResultSummary]))
}

func nowSeconds() float64 { return float64(time.Now().UnixNano()) / 1e9 }
