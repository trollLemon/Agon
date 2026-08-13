package archive

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func sampleSession(id string) Session {
	return Session{
		SessionID: id,
		Title:     "Adopt Event Sourcing",
		Topic:     "Should we adopt event sourcing for the orders service?",
		Mode:      "proposition",
		Tone:      "formal",
		Rounds:    2,
		Sides: []Side{
			{ID: "advocate", Label: "Advocate", Stance: "for"},
			{ID: "critic", Label: "Critic", Stance: "against"},
		},
		Model:     "unsloth/Qwen3-0.6B-Q8_0",
		CreatedAt: time.Date(2026, 8, 12, 19, 0, 0, 0, time.UTC),
		Messages: []Message{
			{Role: "advocate", Round: 1, Content: "opening case", TS: 1000.1},
			{Role: "critic", Round: 1, Content: "rebuttal", TS: 1000.2,
				ToolCalls: []ToolCall{{Name: "read_file", Args: `{"path":"a.go"}`, ResultSummary: "120 bytes"}}},
		},
		Verdict: "adopt",
	}
}

func TestWriteThenLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	want := sampleSession("adopt-event-sourcing-20260812-190000")

	if err := Write(dir, want); err != nil {
		t.Fatalf("Write: %v", err)
	}

	got, err := Load(dir, want.SessionID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Title != want.Title || got.Verdict != want.Verdict || len(got.Messages) != 2 {
		t.Errorf("round trip mismatch: got %+v", got)
	}
	if got.Messages[1].ToolCalls[0].Name != "read_file" {
		t.Errorf("expected tool call to survive round trip, got %+v", got.Messages[1])
	}
}

func TestWriteIsAtomicNoTempFilesLeftBehind(t *testing.T) {
	dir := t.TempDir()
	s := sampleSession("s1")
	if err := Write(dir, s); err != nil {
		t.Fatalf("Write: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected exactly one file after write, got %d: %v", len(entries), entries)
	}
	if entries[0].Name() != "s1.json" {
		t.Errorf("got %q, want %q", entries[0].Name(), "s1.json")
	}
}

func TestWriteOverwritesAtomically(t *testing.T) {
	dir := t.TempDir()
	s := sampleSession("s1")
	if err := Write(dir, s); err != nil {
		t.Fatal(err)
	}
	s.Verdict = "changed"
	if err := Write(dir, s); err != nil {
		t.Fatal(err)
	}
	got, err := Load(dir, "s1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Verdict != "changed" {
		t.Errorf("got verdict %q, want %q", got.Verdict, "changed")
	}
}

func TestLoadMissingSessionErrors(t *testing.T) {
	dir := t.TempDir()
	if _, err := Load(dir, "nope"); err == nil {
		t.Error("expected error loading missing session")
	}
}

func TestListEmptyDirNotError(t *testing.T) {
	got, err := List(filepath.Join(t.TempDir(), "does-not-exist"))
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty list, got %v", got)
	}
}

func TestListReturnsNewestFirst(t *testing.T) {
	dir := t.TempDir()
	older := sampleSession("s-older")
	older.CreatedAt = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	newer := sampleSession("s-newer")
	newer.CreatedAt = time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	if err := Write(dir, older); err != nil {
		t.Fatal(err)
	}
	if err := Write(dir, newer); err != nil {
		t.Fatal(err)
	}

	got, err := List(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].SessionID != "s-newer" || got[1].SessionID != "s-older" {
		t.Fatalf("got %+v, want newest first", got)
	}
	if got[0].Verdict == "" {
		t.Errorf("expected a verdict on the newest session")
	}
}

func TestListSkipsUnreadableFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "broken.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Write(dir, sampleSession("good")); err != nil {
		t.Fatal(err)
	}

	got, err := List(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].SessionID != "good" {
		t.Fatalf("expected only the good session, got %+v", got)
	}
}

func TestSummarizeTitle(t *testing.T) {
	cases := map[string]string{
		"Should we adopt event sourcing?":                                       "Should we adopt event sourcing?",
		"Adopt X. Context: currently we use Y and Z with lots of extra detail.": "Adopt X.",
		"": "Debate",
	}
	for in, want := range cases {
		if got := SummarizeTitle(in); got != want {
			t.Errorf("SummarizeTitle(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSummarizeTitleTruncatesLongText(t *testing.T) {
	long := "This is a very long proposition without any sentence-ending punctuation at all so it just keeps going and going past the summary limit for sure"
	got := SummarizeTitle(long)
	if utf8Len(got) > summaryLimit+1 {
		t.Errorf("expected truncated title, got %d runes: %q", utf8Len(got), got)
	}
}

func utf8Len(s string) int {
	n := 0
	for range s {
		n++
	}
	return n
}

func TestNewSessionIDIsSlugPlusTimestamp(t *testing.T) {
	now := time.Date(2026, 8, 12, 20, 15, 0, 0, time.UTC)
	id := NewSessionID("Should we adopt event sourcing?", now)
	want := "should-we-adopt-event-sourcing-20260812-201500"
	if id != want {
		t.Errorf("got %q, want %q", id, want)
	}
}

func TestNewSessionIDFallsBackWhenSlugEmpty(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	id := NewSessionID("???", now)
	if id != "debate-20260101-000000" {
		t.Errorf("got %q", id)
	}
}
