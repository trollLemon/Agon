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

package cache

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/trollLemon/agon/internal/types"
)

const (
	MetaFileName     = "SESSION_META"
	MessagesLogName  = "MESSAGES_LOG"
	PromptsLogName   = "PROMPTS_LOG"
	InProgressMarker = "IN_PROGRESS"
)

// SessionMeta holds the immutable metadata for a cached session.
// Messages and Verdict are stored separately in append-only logs.
type SessionMeta struct {
	SessionID       string            `json:"session_id"`
	Title           string            `json:"title"`
	Topic           string            `json:"topic"`
	StartingContext string            `json:"starting_context,omitempty"`
	Mode            string            `json:"mode"`
	Tone            string            `json:"tone"`
	Rounds          int               `json:"rounds"`
	Sides           []types.Side      `json:"sides"`
	Model           string            `json:"model"`
	CreatedAt       string            `json:"created_at"` // RFC3339 for JSON compatibility
	Dirs            []string          `json:"dirs,omitempty"`
	Files           []string          `json:"files,omitempty"`
	Verdict         string            `json:"verdict,omitempty"`
	Aborted         map[string]string `json:"aborted,omitempty"`
}

// PromptEntry records a prompt sent to the model for a turn.
type PromptEntry struct {
	Role   string `json:"role"`
	Round  int    `json:"round"`
	Prompt string `json:"prompt"`
	IsLead bool   `json:"is_lead"`
}

// InitCache initializes a new cache directory structure for a session.
// Creates SESSION_META (without messages), empty MESSAGES_LOG, empty PROMPTS_LOG,
// and IN_PROGRESS marker.
func InitCache(rootDir string, id string, s *types.Session) error {
	if err := os.MkdirAll(rootDir, 0o755); err != nil {
		return fmt.Errorf("create cache root dir: %w", err)
	}

	dirPath := filepath.Join(rootDir, id)
	if err := os.MkdirAll(dirPath, 0o755); err != nil {
		return fmt.Errorf("create session dir: %w", err)
	}

	meta := sessionToMeta(s)
	b, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal meta: %w", err)
	}

	metaPath := filepath.Join(dirPath, MetaFileName)
	tmp, err := os.CreateTemp(dirPath, ".*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Rename(tmpPath, metaPath); err != nil {
		return fmt.Errorf("rename meta into place: %w", err)
	}

	// Create empty log files
	for _, name := range []string{MessagesLogName, PromptsLogName} {
		logPath := filepath.Join(dirPath, name)
		if _, err := os.Create(logPath); err != nil {
			return fmt.Errorf("create %s: %w", name, err)
		}
	}

	ipPath := filepath.Join(dirPath, InProgressMarker)
	if err := os.WriteFile(ipPath, []byte(""), 0o644); err != nil {
		return fmt.Errorf("create in-progress marker: %w", err)
	}

	return nil
}

// AppendMessage appends a single message to the MESSAGES_LOG (JSONL).
func AppendMessage(rootDir, sessionID string, msg types.Message) error {
	dirPath := filepath.Join(rootDir, sessionID)
	logPath := filepath.Join(dirPath, MessagesLogName)

	f, err := os.OpenFile(logPath, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o644)
	if err != nil {
		return fmt.Errorf("open messages log: %w", err)
	}
	defer f.Close()

	b, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}
	if _, err := f.Write(append(b, '\n')); err != nil {
		return fmt.Errorf("write message: %w", err)
	}
	return nil
}

// AppendPrompt appends a prompt entry to the PROMPTS_LOG (JSONL).
func AppendPrompt(rootDir, sessionID string, entry PromptEntry) error {
	dirPath := filepath.Join(rootDir, sessionID)
	logPath := filepath.Join(dirPath, PromptsLogName)

	f, err := os.OpenFile(logPath, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o644)
	if err != nil {
		return fmt.Errorf("open prompts log: %w", err)
	}
	defer f.Close()

	b, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal prompt: %w", err)
	}
	if _, err := f.Write(append(b, '\n')); err != nil {
		return fmt.Errorf("write prompt: %w", err)
	}
	return nil
}

// UpdateMeta atomically replaces the SESSION_META file (for verdict/aborted updates).
func UpdateMeta(rootDir, sessionID string, meta *SessionMeta) error {
	dirPath := filepath.Join(rootDir, sessionID)
	metaPath := filepath.Join(dirPath, MetaFileName)

	b, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal meta: %w", err)
	}

	tmp, err := os.CreateTemp(dirPath, ".*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Rename(tmpPath, metaPath); err != nil {
		return fmt.Errorf("rename meta into place: %w", err)
	}
	return nil
}

// WasInterrupted checks whether a session was in progress before the app was
// closed by looking for the IN_PROGRESS marker file.
func WasInterrupted(rootDir string, id string) (bool, error) {
	ipPath := filepath.Join(rootDir, id, InProgressMarker)
	_, err := os.Stat(ipPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// Load reads and parses a cached session from meta + messages log + prompts log.
func Load(rootDir, sessionID string) (*types.Session, error) {
	dirPath := filepath.Join(rootDir, sessionID)

	meta, err := loadMeta(dirPath)
	if err != nil {
		return nil, err
	}

	messages, err := loadMessages(dirPath)
	if err != nil {
		return nil, err
	}

	sess := metaToSession(meta)
	sess.Messages = messages
	return sess, nil
}

// LoadWithPrompts loads a session and its prompt log for resume.
func LoadWithPrompts(rootDir, sessionID string) (*types.Session, []PromptEntry, error) {
	dirPath := filepath.Join(rootDir, sessionID)

	meta, err := loadMeta(dirPath)
	if err != nil {
		return nil, nil, err
	}

	messages, err := loadMessages(dirPath)
	if err != nil {
		return nil, nil, err
	}

	prompts, err := loadPrompts(dirPath)
	if err != nil {
		return nil, nil, err
	}

	sess := metaToSession(meta)
	sess.Messages = messages
	return sess, prompts, nil
}

// LoadMeta loads just the session metadata (for updating verdict/aborted).
func LoadMeta(rootDir, sessionID string) (*SessionMeta, error) {
	dirPath := filepath.Join(rootDir, sessionID)
	return loadMeta(dirPath)
}

func loadMeta(dirPath string) (*SessionMeta, error) {
	metaPath := filepath.Join(dirPath, MetaFileName)
	b, err := os.ReadFile(metaPath)
	if err != nil {
		return nil, err
	}
	var meta SessionMeta
	if err := json.Unmarshal(b, &meta); err != nil {
		return nil, fmt.Errorf("parse meta: %w", err)
	}
	return &meta, nil
}

func loadMessages(dirPath string) ([]types.Message, error) {
	logPath := filepath.Join(dirPath, MessagesLogName)
	f, err := os.Open(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var messages []types.Message
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var msg types.Message
		if err := json.Unmarshal(line, &msg); err != nil {
			continue // skip corrupt lines
		}
		messages = append(messages, msg)
	}
	return messages, scanner.Err()
}

func loadPrompts(dirPath string) ([]PromptEntry, error) {
	logPath := filepath.Join(dirPath, PromptsLogName)
	f, err := os.Open(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var prompts []PromptEntry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var entry PromptEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}
		prompts = append(prompts, entry)
	}
	return prompts, scanner.Err()
}

// List returns every cached session in root, newest first.
func List(rootDir string) ([]*types.Session, error) {
	entries, err := os.ReadDir(rootDir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read cache root dir: %w", err)
	}

	out := make([]*types.Session, 0, len(entries))
	for _, e := range entries {
		name := e.Name()
		if !e.IsDir() {
			continue
		}
		s, err := Load(rootDir, name)
		if err != nil {
			continue
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

// ListInterrupted returns only sessions that still have the IN_PROGRESS sentinel.
func ListInterrupted(rootDir string) ([]*types.Session, error) {
	all, err := List(rootDir)
	if err != nil {
		return nil, err
	}
	out := make([]*types.Session, 0, len(all))
	for _, s := range all {
		ok, err := WasInterrupted(rootDir, s.SessionID)
		if err != nil {
			continue
		}
		if s.Verdict != "" {
			continue
		}
		if ok {
			out = append(out, s)
		}
	}
	return out, nil
}

// Remove deletes the cache directory for a session.
func Remove(rootDir, sessionID string) error {
	dirPath := filepath.Join(rootDir, sessionID)
	if err := os.RemoveAll(dirPath); err != nil {
		return fmt.Errorf("remove cache dir %q: %w", dirPath, err)
	}
	return nil
}

// MarkCompleted removes the IN_PROGRESS marker, indicating the debate finished successfully.
func MarkCompleted(rootDir, sessionID string) error {
	ipPath := filepath.Join(rootDir, sessionID, InProgressMarker)
	return os.Remove(ipPath)
}

// sessionToMeta converts a types.Session to SessionMeta (without messages).
func sessionToMeta(s *types.Session) *SessionMeta {
	return &SessionMeta{
		SessionID:       s.SessionID,
		Title:           s.Title,
		Topic:           s.Topic,
		StartingContext: s.StartingContext,
		Mode:            s.Mode,
		Tone:            s.Tone,
		Rounds:          s.Rounds,
		Sides:           s.Sides,
		Model:           s.Model,
		CreatedAt:       s.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		Dirs:            s.Dirs,
		Files:           s.Files,
		Verdict:         s.Verdict,
		Aborted:         s.Aborted,
	}
}

// metaToSession converts SessionMeta back to types.Session (without messages).
func metaToSession(m *SessionMeta) *types.Session {
	createdAt, _ := time.Parse("2006-01-02T15:04:05Z07:00", m.CreatedAt)
	return &types.Session{
		SessionID:       m.SessionID,
		Title:           m.Title,
		Topic:           m.Topic,
		StartingContext: m.StartingContext,
		Mode:            m.Mode,
		Tone:            m.Tone,
		Rounds:          m.Rounds,
		Sides:           m.Sides,
		Model:           m.Model,
		CreatedAt:       createdAt,
		Dirs:            m.Dirs,
		Files:           m.Files,
		Verdict:         m.Verdict,
		Aborted:         m.Aborted,
		Messages:        nil, // loaded separately
	}
}
