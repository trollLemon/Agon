package archive

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Side is one debater in a session.
type Side struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Stance string `json:"stance"`
}

// ToolCall records one tool invocation made while producing a message, kept
// alongside it so citations stay auditable.
type ToolCall struct {
	Name          string `json:"name"`
	Args          string `json:"args"`
	ResultSummary string `json:"result_summary"`
}

// Message is a single transcript entry.
type Message struct {
	Role      string     `json:"role"`
	Round     int        `json:"round"`
	Content   string     `json:"content"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
	TS        float64    `json:"ts"`
}

// Session is the full on-disk shape of one debate: metadata, transcript, and
// verdict.
type Session struct {
	SessionID string            `json:"session_id"`
	Title     string            `json:"title"`
	Topic     string            `json:"topic"`
	Mode      string            `json:"mode"`
	Tone      string            `json:"tone"`
	Rounds    int               `json:"rounds"`
	Sides     []Side            `json:"sides"`
	Model     string            `json:"model"`
	Dirs      []string          `json:"dirs,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
	Messages  []Message         `json:"messages"`
	Verdict   string            `json:"verdict,omitempty"`
	Aborted   map[string]string `json:"aborted,omitempty"`
}

// path returns the archive file path for a session id within dir.
func path(dir, sessionID string) string {
	return filepath.Join(dir, sessionID+".json")
}

// Write persists s atomically: it writes to a temp file in dir and renames
// it into place, so a reader never observes a partial file. It is meant to
// be called exactly once per session, when the verdict completes.
func Write(dir string, s Session) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create archive dir: %w", err)
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal session: %w", err)
	}

	tmp, err := os.CreateTemp(dir, s.SessionID+".*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath) // no-op once renamed

	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}

	if err := os.Rename(tmpPath, path(dir, s.SessionID)); err != nil {
		return fmt.Errorf("rename into place: %w", err)
	}
	return nil
}

// Load reads and parses one archived session.
func Load(dir, sessionID string) (Session, error) {
	b, err := os.ReadFile(path(dir, sessionID))
	if err != nil {
		return Session{}, err
	}
	var s Session
	if err := json.Unmarshal(b, &s); err != nil {
		return Session{}, fmt.Errorf("parse session %q: %w", sessionID, err)
	}
	return s, nil
}

// List returns every archived session in dir, newest first. A missing dir
// is treated as empty, not an error.
func List(dir string) ([]Session, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read archive dir: %w", err)
	}

	out := make([]Session, 0, len(entries))
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".json") {
			continue
		}
		sessionID := strings.TrimSuffix(name, ".json")
		s, err := Load(dir, sessionID)
		if err != nil {
			continue // skip unreadable/partial files rather than failing the whole listing
		}
		out = append(out, s)
	}

	sort.Slice(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.After(out[j].CreatedAt)
		}
		return out[i].SessionID > out[j].SessionID
	})
	return out, nil
}
