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
		Messages: []types.Message{
			{Role: "advocate", Round: 1, Content: "opening case", TS: 1000.1},
			{Role: "critic", Round: 1, Content: "rebuttal", TS: 1000.2},
		},
		Verdict: "adopt",
	}
}

func TestInitCacheWritesMarkerAndLoads(t *testing.T) {
	root := t.TempDir()
	want := sampleSession("s1")

	if err := InitCache(root, want.SessionID, &want); err != nil {
		t.Fatalf("InitCache: %v", err)
	}

	ok, err := WasInterrupted(root, want.SessionID)
	if err != nil {
		t.Fatalf("WasInterrupted: %v", err)
	}
	if !ok {
		t.Fatal("expected IN_PROGRESS marker right after InitCache")
	}

	loaded, err := Load(root, want.SessionID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Title != want.Title || len(loaded.Messages) != len(want.Messages) {
		t.Errorf("round trip mismatch: got %+v", loaded)
	}
}

func TestUpdateCacheOverwritesAndKeepsMarker(t *testing.T) {
	root := t.TempDir()
	s := sampleSession("s1")
	if err := InitCache(root, s.SessionID, &s); err != nil {
		t.Fatal(err)
	}

	s.Verdict = "changed"
	if err := UpdateCache(root, s.SessionID, &s); err != nil {
		t.Fatalf("UpdateCache: %v", err)
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
		t.Error("UpdateCache must not clear the IN_PROGRESS marker; a debate mid-write is still resumable")
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
	finished := sampleSession("s-finished")

	if err := InitCache(root, interrupted.SessionID, &interrupted); err != nil {
		t.Fatal(err)
	}
	if err := InitCache(root, finished.SessionID, &finished); err != nil {
		t.Fatal(err)
	}
	// A finished session has been archived and its cache dir removed.
	if err := Remove(root, finished.SessionID); err != nil {
		t.Fatal(err)
	}

	got, err := ListInterrupted(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].SessionID != interrupted.SessionID {
		t.Fatalf("expected only the interrupted session, got %+v", got)
	}

	hasFinished, err := WasInterrupted(root, finished.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if hasFinished {
		t.Error("finished session should not be marked interrupted")
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
