package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/ardanlabs/kronk/sdk/tools/models"

	kronktools "github.com/agentdisc/goagentdisc/internal/tools"
)

// DefaultModel is the built-in model source pre-filled in the new-debate
// form (see docs/SPEC.md D7). Qwen3-8B is a meaningful step up in
// instruction-following over the smaller 0.6B/1.7B tiers — small local
// models are prone to ignoring structural constraints in the system prompt
// (e.g. writing "Round 2:" headers, or drafting several turns at once) —
// while still comfortably fitting in memory on a typical modern machine
// (~8.7GB at Q8_0). The form's model field is always editable, so anyone
// with more RAM to spare can type in a bigger one (e.g.
// "unsloth/gpt-oss-20b-Q8_0" at ~12GB).
const DefaultModel = "Qwen/Qwen3-8B-Q8_0"

// Bootstrap installs (if needed) the native inference libraries and the
// given model, then loads it in-process with the incremental message cache
// enabled — one loaded model instance plays every role (see docs/SPEC.md
// D2, D7). Progress is reported through logger (kronk.FmtLogger writes to
// stdout; callers driving a UI can supply their own). First run may take a
// while (downloads); callers should give ctx a generous deadline.
func Bootstrap(ctx context.Context, modelSource string, logger kronk.Logger) (*kronk.Kronk, error) {
	if modelSource == "" {
		modelSource = DefaultModel
	}
	if logger == nil {
		logger = kronk.DiscardLogger
	}

	libMgr, err := libs.New(libs.WithDetect(ctx, logger))
	if err != nil {
		return nil, fmt.Errorf("detect native libraries: %w", err)
	}
	if _, err := libMgr.Download(ctx, logger); err != nil {
		return nil, fmt.Errorf("install native libraries: %w", err)
	}
	if err := kronk.Init(kronk.WithLibPath(libMgr.LibsPath())); err != nil {
		return nil, fmt.Errorf("initialize kronk: %w", err)
	}

	modelMgr, err := models.New()
	if err != nil {
		return nil, fmt.Errorf("initialize model manager: %w", err)
	}
	mp, err := modelMgr.Download(ctx, logger, modelSource)
	if err != nil {
		return nil, fmt.Errorf("install model %q: %w", modelSource, err)
	}

	krn, err := kronk.New(
		model.WithModelFiles(mp.ModelFiles),
		model.WithAutoTune(true),
		model.WithIncrementalCache(true),
		model.WithLog(logger),
	)
	if err != nil {
		return nil, fmt.Errorf("load model: %w", err)
	}
	return krn, nil
}

// KronkClient adapts a single loaded *kronk.Kronk instance into the
// ChatClient interface the debate loop drives. Every role shares this one
// client: kronk's incremental message cache matches each call's message
// prefix and reuses KV automatically, so per-role histories stay cheap
// without the app ever passing an explicit session id (docs/SPEC.md D2).
type KronkClient struct {
	krn *kronk.Kronk
}

var _ ChatClient = (*KronkClient)(nil)

// NewKronkClient wraps an already-loaded kronk instance.
func NewKronkClient(krn *kronk.Kronk) *KronkClient {
	return &KronkClient{krn: krn}
}

// ChatStreaming implements ChatClient.
func (k *KronkClient) ChatStreaming(ctx context.Context, role Role, messages []ChatMessage, toolSpecs []kronktools.Spec) (<-chan StreamEvent, error) {
	doc := model.D{
		"messages":    toModelMessages(messages),
		"temperature": 0.7,
		"top_p":       0.9,
		"top_k":       40,
		"max_tokens":  4096,
	}
	if len(toolSpecs) > 0 {
		doc["tools"] = toToolDocuments(toolSpecs)
		doc["tool_selection"] = "auto"
	}

	ch, err := k.krn.ChatStreaming(ctx, doc)
	if err != nil {
		return nil, err
	}

	out := make(chan StreamEvent, 32)
	go func() {
		defer close(out)
		for resp := range ch {
			if len(resp.Choices) == 0 {
				continue
			}
			choice := resp.Choices[0]
			switch choice.FinishReason() {
			case model.FinishReasonError:
				msg := "unknown model error"
				if choice.Delta != nil && choice.Delta.Content != "" {
					msg = choice.Delta.Content
				}
				out <- StreamEvent{Err: fmt.Errorf("%s", msg)}
				return

			case model.FinishReasonStop, model.FinishReasonLength:
				continue

			case model.FinishReasonTool:
				out <- StreamEvent{ToolCalls: toToolCallRequests(choice.Message.ToolCalls)}

			default:
				if choice.Delta != nil && choice.Delta.Content != "" {
					out <- StreamEvent{ContentDelta: choice.Delta.Content}
				}
			}
		}
	}()
	return out, nil
}

// toModelMessages converts a role's ChatMessage history into kronk's chat
// document format.
func toModelMessages(msgs []ChatMessage) []model.D {
	out := make([]model.D, 0, len(msgs))
	for _, m := range msgs {
		switch m.Role {
		case RoleTool:
			out = append(out, model.D{
				"role": model.RoleTool, "name": m.ToolName,
				"tool_call_id": m.ToolCallID, "content": m.Content,
			})

		case RoleAssistant:
			if len(m.ToolCalls) == 0 {
				out = append(out, model.TextMessage(model.RoleAssistant, m.Content))
				continue
			}
			calls := make([]model.D, 0, len(m.ToolCalls))
			for _, tc := range m.ToolCalls {
				argsJSON, _ := json.Marshal(tc.Arguments)
				calls = append(calls, model.D{
					"id": tc.ID, "type": "function",
					"function": model.D{"name": tc.Name, "arguments": string(argsJSON)},
				})
			}
			out = append(out, model.D{"role": model.RoleAssistant, "tool_calls": calls})

		case RoleSystem:
			out = append(out, model.TextMessage(model.RoleSystem, m.Content))

		default:
			out = append(out, model.TextMessage(model.RoleUser, m.Content))
		}
	}
	return out
}

// toToolDocuments converts tool specs into kronk's tool-document format.
func toToolDocuments(specs []kronktools.Spec) []model.D {
	out := make([]model.D, 0, len(specs))
	for _, s := range specs {
		out = append(out, model.D{
			"type": "function",
			"function": model.D{
				"name":        s.Name,
				"description": s.Description,
				"parameters":  s.Parameters,
			},
		})
	}
	return out
}

// toToolCallRequests converts kronk's tool-call responses into the
// orchestrator's transport-neutral shape.
func toToolCallRequests(calls []model.ResponseToolCall) []ToolCallRequest {
	out := make([]ToolCallRequest, 0, len(calls))
	for _, c := range calls {
		out = append(out, ToolCallRequest{ID: c.ID, Name: c.Function.Name, Arguments: c.Function.Arguments})
	}
	return out
}

// Unload releases the model from memory. Call it once, on app shutdown.
func Unload(ctx context.Context, krn *kronk.Kronk) error {
	shutdownCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return krn.Unload(shutdownCtx)
}
