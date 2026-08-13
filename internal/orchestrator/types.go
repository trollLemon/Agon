// Package orchestrator runs the debate loop: one advocate and one critic
// each keep a persistent per-role message history against a single loaded
// model, tool calls are executed against a sandboxed repo when one is in
// scope, and a neutral judge reads the full transcript once at the end. See
// docs/SPEC.md D2, D3, D6, D11.
package orchestrator

import (
	"context"
	"time"

	"github.com/agentdisc/goagentdisc/internal/archive"
	"github.com/agentdisc/goagentdisc/internal/prompts"
	"github.com/agentdisc/goagentdisc/internal/tools"
)

// Role identifies which pseudo-agent a chat turn belongs to: a debater's
// side id, or RoleJudge. Kept as a distinct type (rather than a bare
// string) so a ChatClient can key per-role behavior — e.g. the deferred
// per-role model override noted in docs/SPEC.md D2.
type Role string

// RoleJudge is the fixed role id for the neutral adjudicator.
const RoleJudge Role = "judge"

// ChatMessageRole is the role field of a single ChatMessage.
type ChatMessageRole string

const (
	RoleSystem    ChatMessageRole = "system"
	RoleUser      ChatMessageRole = "user"
	RoleAssistant ChatMessageRole = "assistant"
	RoleTool      ChatMessageRole = "tool"
)

// ToolCallRequest is one tool invocation a model has asked for.
type ToolCallRequest struct {
	ID        string
	Name      string
	Arguments map[string]any
}

// ChatMessage is one entry in a role's persistent message history.
type ChatMessage struct {
	Role    ChatMessageRole
	Content string

	// ToolCalls is set on an assistant message that requested tool calls.
	ToolCalls []ToolCallRequest

	// ToolCallID and ToolName are set on a tool-role message, echoing the
	// request it answers.
	ToolCallID string
	ToolName   string
}

// StreamEvent is one increment of a ChatClient turn. A turn ends with either
// a StreamEvent carrying Done=true, tool calls, or a non-nil Err.
type StreamEvent struct {
	ContentDelta string
	ToolCalls    []ToolCallRequest
	Done         bool
	Err          error
}

// ChatClient sends a role's full message history to a model and streams back
// its response. Implementations decide how (or whether) history for
// different roles shares an underlying model instance; the orchestrator
// only depends on this interface, which keeps it unit-testable without a
// real model.
type ChatClient interface {
	ChatStreaming(ctx context.Context, role Role, messages []ChatMessage, toolSpecs []tools.Spec) (<-chan StreamEvent, error)
}

// Config configures one debate run. SessionID, Title, and CreatedAt are
// assigned by the caller (see internal/archive naming helpers) so the
// orchestrator itself stays free of clock/ID concerns and easy to test
// deterministically.
type Config struct {
	SessionID       string
	Title           string
	Topic           string
	StartingContext string
	Mode            prompts.Mode
	Tone            prompts.Tone
	Rounds          int
	// Sides holds exactly two entries; Sides[0] leads each round.
	Sides [2]archive.Side
	Model string
	// SandboxDirs are the directories tool grounding is confined to.
	SandboxDirs []string // nil disables tool grounding
	CreatedAt   time.Time
}

// EventKind identifies the shape of an Event.
type EventKind int

const (
	EventTurnStart EventKind = iota
	EventToken
	EventToolCall
	EventTurnEnd
	EventVerdict
	EventAborted
	EventError
)

// Event is one increment streamed out of a running Debate for a UI to
// render: a token chunk, a completed tool call, a turn boundary, the
// verdict, or a terminal abort/error.
type Event struct {
	Kind  EventKind
	Role  string
	Round int
	Text  string
	Tool  *archive.ToolCall
}
