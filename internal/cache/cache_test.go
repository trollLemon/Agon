package cache

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/trollLemon/agon/internal/types"
)

func sampleSession(id string) types.Session {
	sid := id
	if sid == "" {
		sid = "sample-default"
	}
	return types.Session{
		SessionID: sid,
		Title:     "Adopt Event Sourcing",
		Topic:     "Should we adopt event sourcing for the orders service?",
		Mode:      "proposition",
		Tone:      "formal",
		Rounds:    2,
		Sides: []types.Side{
			{ID: "advocate", Label: "Advocate", Stance: "for"},
			{ID: "critic", Label: "Critic", Stance: "against"},
		},
		Model:     "unsloth/Qwen3-0.6B-Q8_0",
		CreatedAt: time.Date(2026, 8, 12, 19, 0, 0, 0, time.UTC),
		Verdict:   "adopt",
	}
}

func sampleMessage(role string, round int, content string) types.Message {
	return types.Message{
		Role:    role,
		Round:   round,
		Content: content,
		TS:      float64(time.Now().UnixMilli()) / 1000.0,
	}
}

func TestInitCacheCreatesStructure(t *testing.T) {
	root := t.TempDir()
	want := sampleSession("s1")

	if err := InitCache(root, want.SessionID, &want); err != nil {
		t.Fatalf("InitCache: %v", err)
	}

	// Check all files exist
	dirPath := filepath.Join(root, want.SessionID)
	for _, name := range []string{MetaFileName, MessagesLogName, PromptsLogName, InProgressMarker} {
		path := filepath.Join(dirPath, name)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("missing %s: %v", name, err)
		}
	}

	ok, err := WasInterrupted(root, want.SessionID)
	if err != nil {
		t.Fatalf("WasInterrupted: %v", err)
	}
	if !ok {
		t.Fatal("expected IN_PROGRESS marker right after InitCache")
	}

	// Load meta (no messages yet)
	loaded, err := Load(root, want.SessionID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Title != want.Title {
		t.Errorf("title mismatch: got %q, want %q", loaded.Title, want.Title)
	}
	if len(loaded.Messages) != 0 {
		t.Errorf("expected no messages initially, got %d", len(loaded.Messages))
	}
}

func TestAppendMessageAndLoad(t *testing.T) {
	root := t.TempDir()
	s := sampleSession("s1")
	if err := InitCache(root, s.SessionID, &s); err != nil {
		t.Fatal(err)
	}

	// Append messages
	msg1 := sampleMessage("advocate", 1, "opening")
	msg2 := sampleMessage("critic", 1, "rebuttal")
	if err := AppendMessage(root, s.SessionID, msg1); err != nil {
		t.Fatalf("AppendMessage 1: %v", err)
	}
	if err := AppendMessage(root, s.SessionID, msg2); err != nil {
		t.Fatalf("AppendMessage 2: %v", err)
	}

	// LoadWithPrompts should return messages
	loaded, _, err := LoadWithPrompts(root, s.SessionID)
	if err != nil {
		t.Fatalf("LoadWithPrompts: %v", err)
	}
	if len(loaded.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(loaded.Messages))
	}
	if loaded.Messages[0].Content != "opening" || loaded.Messages[1].Content != "rebuttal" {
		t.Errorf("message content mismatch: %+v", loaded.Messages)
	}
}

func TestAppendPromptAndLoad(t *testing.T) {
	root := t.TempDir()
	s := sampleSession("s1")
	if err := InitCache(root, s.SessionID, &s); err != nil {
		t.Fatal(err)
	}

	prompt1 := PromptEntry{Role: "advocate", Round: 1, Prompt: "opening prompt", IsLead: true}
	prompt2 := PromptEntry{Role: "critic", Round: 1, Prompt: "rebuttal prompt", IsLead: false}
	if err := AppendPrompt(root, s.SessionID, prompt1); err != nil {
		t.Fatalf("AppendPrompt 1: %v", err)
	}
	if err := AppendPrompt(root, s.SessionID, prompt2); err != nil {
		t.Fatalf("AppendPrompt 2: %v", err)
	}

	_, prompts, err := LoadWithPrompts(root, s.SessionID)
	if err != nil {
		t.Fatalf("LoadWithPrompts: %v", err)
	}
	if len(prompts) != 2 {
		t.Fatalf("expected 2 prompts, got %d", len(prompts))
	}
	if prompts[0].Prompt != "opening prompt" || prompts[1].Prompt != "rebuttal prompt" {
		t.Errorf("prompt content mismatch: %+v", prompts)
	}
}

func TestUpdateMetaOverwritesAndKeepsMarker(t *testing.T) {
	root := t.TempDir()
	s := sampleSession("s1")
	if err := InitCache(root, s.SessionID, &s); err != nil {
		t.Fatal(err)
	}

	meta, err := loadMeta(filepath.Join(root, s.SessionID))
	if err != nil {
		t.Fatal(err)
	}
	meta.Verdict = "changed"
	if err := UpdateMeta(root, s.SessionID, meta); err != nil {
		t.Fatalf("UpdateMeta: %v", err)
	}

	loaded, err := Load(root, s.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Verdict != "changed" {
		t.Errorf("got verdict %q, want %q", loaded.Verdict, "changed")
	}
	ok, err := WasInterrupted(root, s.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Error("UpdateMeta must not clear the IN_PROGRESS marker")
	}
}

func TestRemoveDeletesSessionDir(t *testing.T) {
	root := t.TempDir()
	s := sampleSession("s1")
	if err := InitCache(root, s.SessionID, &s); err != nil {
		t.Fatal(err)
	}

	if err := Remove(root, s.SessionID); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	ok, err := WasInterrupted(root, s.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Error("expected IN_PROGRESS marker gone after Remove")
	}
	if _, err := os.Stat(filepath.Join(root, s.SessionID)); !os.IsNotExist(err) {
		t.Errorf("expected session dir to be deleted, stat err = %v", err)
	}
}

func TestListReturnsNewestFirst(t *testing.T) {
	root := t.TempDir()
	older := sampleSession("s-older")
	older.CreatedAt = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	newer := sampleSession("s-newer")
	newer.CreatedAt = time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	for _, s := range []types.Session{older, newer} {
		if err := InitCache(root, s.SessionID, &s); err != nil {
			t.Fatal(err)
		}
	}

	got, err := List(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].SessionID != newer.SessionID || got[1].SessionID != older.SessionID {
		t.Fatalf("got %+v, want newest first", got)
	}
}

func TestListInterruptedOnlyReturnsMarked(t *testing.T) {
	root := t.TempDir()
	interrupted := sampleSession("s-interrupted")
	interrupted.Verdict = "" // no verdict = interrupted
	finished := sampleSession("s-finished")
	finished.Verdict = "adopt" // finished with verdict

	if err := InitCache(root, interrupted.SessionID, &interrupted); err != nil {
		t.Fatal(err)
	}
	if err := InitCache(root, finished.SessionID, &finished); err != nil {
		t.Fatal(err)
	}

	got, err := ListInterrupted(root)
	if err != nil {
		t.Fatal(err)
	}
	// Finished debates (with verdict) are filtered out, even if interrupted marker exists
	if len(got) != 1 || got[0].SessionID != interrupted.SessionID {
		t.Fatalf("expected only the interrupted session, got %+v", got)
	}

	// WasInterrupted still returns true if marker exists, regardless of verdict
	hasFinished, err := WasInterrupted(root, finished.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if !hasFinished {
		t.Error("finished session still has IN_PROGRESS marker (WasInterrupted checks marker only)")
	}
}

func TestMissingRootNotAnError(t *testing.T) {
	root := filepath.Join(t.TempDir(), "does-not-exist")
	if got, err := List(root); err != nil || len(got) != 0 {
		t.Errorf("List: got %v, err %v", got, err)
	}
	if got, err := ListInterrupted(root); err != nil || len(got) != 0 {
		t.Errorf("ListInterrupted: got %v, err %v", got, err)
	}
	if ok, err := WasInterrupted(root, "nope"); err != nil || ok {
		t.Errorf("WasInterrupted: ok=%v, err %v", ok, err)
	}
}

func TestMarkCompletedRemovesMarker(t *testing.T) {
	root := t.TempDir()
	s := sampleSession("s1")
	if err := InitCache(root, s.SessionID, &s); err != nil {
		t.Fatal(err)
	}

	if err := MarkCompleted(root, s.SessionID); err != nil {
		t.Fatalf("MarkCompleted: %v", err)
	}

	ok, err := WasInterrupted(root, s.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Error("expected IN_PROGRESS marker gone after MarkCompleted")
	}
}

func TestUpdateMetaPersistsAborted(t *testing.T) {
	root := t.TempDir()
	s := sampleSession("s1")
	if err := InitCache(root, s.SessionID, &s); err != nil {
		t.Fatal(err)
	}

	meta, err := LoadMeta(root, s.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	meta.Aborted = map[string]string{"reason": "test abort reason"}
	if err := UpdateMeta(root, s.SessionID, meta); err != nil {
		t.Fatalf("UpdateMeta: %v", err)
	}

	// Reload and verify
	loaded, err := Load(root, s.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Aborted == nil {
		t.Fatal("expected Aborted to be set, got nil")
	}
	if loaded.Aborted["reason"] != "test abort reason" {
		t.Errorf("expected reason 'test abort reason', got %q", loaded.Aborted["reason"])
	}
}
