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

package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/trollLemon/agon/internal/prompts"
	"github.com/trollLemon/agon/internal/tools"
	"github.com/trollLemon/agon/internal/types"
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

	initial *types.Session
}

// New creates a Debate. If cfg.SandboxDirs or cfg.SandboxFiles is set, sandbox
// must be a *tools.Sandbox over those paths (nil disables tool grounding
// regardless of the configured paths).
func New(cfg Config, client ChatClient, sandbox *tools.Sandbox) *Debate {
	return &Debate{
		cfg:     cfg,
		client:  client,
		sandbox: sandbox,
		events:  make(chan Event, 512),
		initial: cfg.Session(),
	}
}

// ConfigFromSession rebuilds a Config from a persisted Session, used when
// resuming an interrupted debate. StartingContext is taken from the session
// if present.
func ConfigFromSession(sess *types.Session) Config {
	var sides [2]types.Side
	if len(sess.Sides) >= 2 {
		sides = [2]types.Side{sess.Sides[0], sess.Sides[1]}
	} else if len(sess.Sides) == 1 {
		sides[0] = sess.Sides[0]
	}
	return Config{
		SessionID:       sess.SessionID,
		Title:           sess.Title,
		Topic:           sess.Topic,
		StartingContext: sess.StartingContext,
		Mode:            prompts.Mode(sess.Mode),
		Tone:            prompts.Tone(sess.Tone),
		Rounds:          sess.Rounds,
		Sides:           sides,
		Model:           sess.Model,
		SandboxDirs:     sess.Dirs,
		SandboxFiles:    sess.Files,
		CreatedAt:       sess.CreatedAt,
	}
}

// NewResumed creates a Debate that will resume from sess, continuing from
// the first incomplete round. The caller is responsible for recreating the
// sandbox from sess.Dirs/sess.Files if needed.
func NewResumed(sess *types.Session, client ChatClient, sandbox *tools.Sandbox) *Debate {
	cfg := ConfigFromSession(sess)
	return &Debate{
		cfg:     cfg,
		client:  client,
		sandbox: sandbox,
		events:  make(chan Event, 512),
		initial: sess,
	}
}

// Events returns the channel of streamed turn/token/tool/verdict events. It
// is closed when Run returns.
func (d *Debate) Events() <-chan Event { return d.events }

// SessionID returns the session identifier for this debate.
func (d *Debate) SessionID() string { return d.cfg.SessionID }

// Config returns the debate configuration.
func (d *Debate) Config() Config { return d.cfg }

// Abort requests that the running debate stop as soon as possible. The
// in-memory transcript is discarded — Run returns an *AbortedError and a
// zero-value types.Session. Safe to call before Run starts or multiple
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
	d.events <- ev
}

// resumePlan describes where a debate should resume. StartRound is the
// first round that still needs to be executed (1 for a fresh debate).
type resumePlan struct {
	Session      *types.Session
	StartRound   int
	LastFollower string
	Done         bool
}

// Run executes the full debate: advocate/critic rounds, then a single judge
// turn. On success it returns the completed types.Session, ready to be
// written exactly once by the caller. On error or
// abort it returns a zero-value Session and a non-nil error.
func (d *Debate) Run(parent context.Context) (*types.Session, error) {
	ctx, cancel := context.WithCancel(parent)
	d.mu.Lock()
	d.cancel = cancel
	d.mu.Unlock()
	defer cancel()
	defer close(d.events)

	plan, lead, follow := prepareSession(d.cfg, d.initial, d.sandbox)
	if plan.Done {
		d.emit(Event{Kind: EventVerdict, Text: plan.Session.Verdict})
		return plan.Session, nil
	}

	if err := d.executeRounds(ctx, lead, follow, plan.Session, plan.StartRound, plan.LastFollower); err != nil {
		return d.fail(err)
	}
	return d.completeWithVerdict(ctx, plan.Session)
}

func prepareSession(cfg Config, initial *types.Session, sandbox *tools.Sandbox) (resumePlan, *sideRuntime, *sideRuntime) {
	lead := newSideRuntime(cfg, sandbox, cfg.Sides[0], cfg.Sides[1], true)
	follow := newSideRuntime(cfg, sandbox, cfg.Sides[1], cfg.Sides[0], false)
	rebuildHistories(initial, lead, follow, cfg)
	startRound := nextTurn(initial, cfg.Rounds)
	return resumePlan{
		// rounds+1 once every round is complete, so executeRounds skips to the judge.
		Session:      initial,
		StartRound:   startRound,
		LastFollower: lastFollowerFromSess(initial, cfg.Rounds),
		Done:         startRound > cfg.Rounds && initial.Verdict != "",
	}, lead, follow
}

func (d *Debate) executeRounds(ctx context.Context, lead, follow *sideRuntime, sess *types.Session, startRound int, lastFollower string) error {
	for round := startRound; round <= d.cfg.Rounds; round++ {
		if err := d.checkAbort(ctx); err != nil {
			return err
		}
		leadIdx := (round - 1) * 2
		followIdx := leadIdx + 1
		leadDone := leadIdx < len(sess.Messages)
		followDone := followIdx < len(sess.Messages)
		if leadDone && followDone {
			lastFollower = sess.Messages[followIdx].Content
			continue
		}
		if !leadDone {
			if err := d.runFullRound(ctx, lead, follow, sess, round, lastFollower); err != nil {
				return err
			}
			lastFollower = sess.Messages[followIdx].Content
			continue
		}
		content := sess.Messages[leadIdx].Content
		followMsg := prompts.PeerMessage(lead.label, content, round, d.cfg.Rounds)
		followContent, err := d.runTurn(ctx, follow, round, sess, followMsg)
		if err != nil {
			return err
		}
		lastFollower = followContent
	}
	return nil
}

func (d *Debate) runFullRound(ctx context.Context, lead, follow *sideRuntime, sess *types.Session, round int, lastFollower string) error {
	leadMsg := prompts.OpeningMessage(d.cfg.Topic, d.cfg.StartingContext, round, d.cfg.Rounds)
	if round > 1 {
		leadMsg = prompts.PeerMessage(follow.label, lastFollower, round, d.cfg.Rounds)
	}
	content, err := d.runTurn(ctx, lead, round, sess, leadMsg)
	if err != nil {
		return err
	}
	followMsg := prompts.PeerMessage(lead.label, content, round, d.cfg.Rounds)
	_, err = d.runTurn(ctx, follow, round, sess, followMsg)
	return err
}

func (d *Debate) completeWithVerdict(ctx context.Context, sess *types.Session) (*types.Session, error) {
	if err := d.checkAbort(ctx); err != nil {
		return d.fail(err)
	}
	if sess.Verdict != "" {
		d.emit(Event{Kind: EventVerdict, Text: sess.Verdict})
		return sess, nil
	}
	verdict, err := d.runJudgeTurn(ctx, sess)
	if err != nil {
		return d.fail(err)
	}
	sess.Verdict = verdict
	d.emit(Event{Kind: EventVerdict, Text: verdict})
	return sess, nil
}

// nextTurn returns the first incomplete round, or rounds+1 when all are done.
func nextTurn(sess *types.Session, rounds int) int {
	for round := 1; round <= rounds; round++ {
		if (round-1)*2+1 >= len(sess.Messages) {
			return round
		}
	}
	return rounds + 1
}

func lastFollowerFromSess(sess *types.Session, rounds int) string {
	last := ""
	for round := 1; round <= rounds; round++ {
		followIdx := (round-1)*2 + 1
		if followIdx < len(sess.Messages) {
			last = sess.Messages[followIdx].Content
		}
	}
	return last
}

func rebuildHistories(sess *types.Session, lead, follow *sideRuntime, cfg Config) {
	var lastFollower string
	for round := 1; round <= cfg.Rounds; round++ {
		leadIdx := (round - 1) * 2
		followIdx := leadIdx + 1
		if leadIdx >= len(sess.Messages) {
			break
		}
		leadMsg := prompts.OpeningMessage(cfg.Topic, cfg.StartingContext, round, cfg.Rounds)
		if round > 1 {
			leadMsg = prompts.PeerMessage(follow.label, lastFollower, round, cfg.Rounds)
		}
		lead.history = append(lead.history, ChatMessage{Role: RoleUser, Content: leadMsg})
		mLead := sess.Messages[leadIdx]
		lead.history = append(lead.history, ChatMessage{Role: RoleAssistant, Content: mLead.Content})
		if followIdx >= len(sess.Messages) {
			break
		}
		followMsg := prompts.PeerMessage(lead.label, mLead.Content, round, cfg.Rounds)
		follow.history = append(follow.history, ChatMessage{Role: RoleUser, Content: followMsg})
		mFollow := sess.Messages[followIdx]
		follow.history = append(follow.history, ChatMessage{Role: RoleAssistant, Content: mFollow.Content})
		lastFollower = mFollow.Content
	}
}

func (d *Debate) fail(err error) (*types.Session, error) {
	var aerr *AbortedError
	if errors.As(err, &aerr) {
		d.emit(Event{Kind: EventAborted, Text: aerr.Reason})
	} else {
		d.emit(Event{Kind: EventError, Text: err.Error()})
	}
	return nil, err
}

func newSideRuntime(cfg Config, sandbox *tools.Sandbox, side, opponent types.Side, leads bool) *sideRuntime {
	var dirs, files []string
	if sandbox != nil {
		dirs = cfg.SandboxDirs
		files = cfg.SandboxFiles
	}
	sys := prompts.DebaterSystem(prompts.DebaterParams{
		Mode:          cfg.Mode,
		Tone:          cfg.Tone,
		Label:         side.Label,
		Stance:        side.Stance,
		OpponentLabel: opponent.Label,
		Leads:         leads,
		Rounds:        cfg.Rounds,
		Dirs:          dirs,
		Files:         files,
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
func (d *Debate) runTurn(ctx context.Context, s *sideRuntime, round int, sess *types.Session, userContent string) (string, error) {
	s.history = append(s.history, ChatMessage{Role: RoleUser, Content: userContent})
	d.emit(Event{Kind: EventTurnStart, Role: s.roleName, Round: round})

	var toolCallLog []types.ToolCall
	var toolSpecs []tools.Spec
	if d.sandbox != nil {
		toolSpecs = tools.Specs()
	}
	for i := 0; i < maxToolIterations; i++ {
		if err := d.checkAbort(ctx); err != nil {
			return "", err
		}

		ch, err := d.client.ChatStreaming(ctx, Role(s.roleName), s.history, toolSpecs)
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
			sess.Messages = append(sess.Messages, types.Message{
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
			te := types.ToolCall{Name: tc.Name, Args: string(argsJSON), ResultSummary: summarize(result)}
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
		return "", fmt.Errorf("tool %q is unavailable: this debate has no sandbox, add files or directories in the Sandbox field to give the debaters read-only access", tc.Name)
	}
	return tools.Call(d.sandbox, tc.Name, tc.Arguments)
}

func (d *Debate) runJudgeTurn(ctx context.Context, sess *types.Session) (string, error) {
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
func renderTranscript(sess *types.Session) string {
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
