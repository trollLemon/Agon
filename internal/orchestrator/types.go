package orchestrator

import (
	"context"
	"time"

	"github.com/trollLemon/agon/internal/archive"
	"github.com/trollLemon/agon/internal/prompts"
	"github.com/trollLemon/agon/internal/tools"
)

// Role identifies which pseudo-agent a chat turn belongs to: a debater's
// side id, or RoleJudge.
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

// ChatClient sends a role's full message history to a model and streams back its response.
type ChatClient interface {
	ChatStreaming(ctx context.Context, role Role, messages []ChatMessage, toolSpecs []tools.Spec) (<-chan StreamEvent, error)
}

type LogFunc func(msg string, args ...any)

// Engine is a ChatClient whose model must be loaded via Initialize before
// any ChatStreaming call to allow for lazy initialization.
type Engine interface {
	ChatClient
	Initialize(ctx context.Context, modelSource string, log LogFunc) error
}

// Config holds the configuration of a debate.
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
	// SandboxFiles are individually allowed files tool grounding may read.
	SandboxFiles []string
	CreatedAt    time.Time
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
