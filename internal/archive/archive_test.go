package archive

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
			{Role: "critic", Round: 1, Content: "rebuttal", TS: 1000.2,
				ToolCalls: []types.ToolCall{{Name: "read_file", Args: `{"path":"a.go"}`, ResultSummary: "120 bytes"}}},
		},
		Verdict: "adopt",
	}
}

func TestWriteThenLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	want := sampleSession("s-roundtrip")

	if err := Write(dir, &want); err != nil {
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

	for range 2 {
		if err := Write(dir, &s); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}

	archivePath := filepath.Join(dir, "s1.json")
	if _, err := os.Stat(archivePath); err != nil {
		if os.IsNotExist(err) {
			t.Fatalf("expected archive file %q to exist after Write", archivePath)
		}
		t.Fatalf("stat archive path: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() != "s1.json" {
			t.Errorf("expected only the archive file, found %q (temp leftovers?)", e.Name())
		}
	}
}

func TestWriteOverwritesAtomically(t *testing.T) {
	dir := t.TempDir()
	s := sampleSession("s1")

	if err := Write(dir, &s); err != nil {
		t.Fatal(err)
	}
	s.Verdict = "changed"
	if err := Write(dir, &s); err != nil {
		t.Fatal(err)
	}
	got, err := Load(dir, s.SessionID)
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

	if err := Write(dir, &older); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := Write(dir, &newer); err != nil {
		t.Fatalf("Write: %v", err)
	}

	got, err := List(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].SessionID != newer.SessionID || got[1].SessionID != older.SessionID {
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
	s := sampleSession("good")

	if err := Write(dir, &s); err != nil {
		t.Fatal(err)
	}

	got, err := List(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].SessionID != s.SessionID {
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

func TestNewSessionID(t *testing.T) {
	tests := []struct {
		name     string
		topic    string
		now      time.Time
		expected string
	}{
		{
			name:     "slug plus timestamp",
			topic:    "Should we adopt event sourcing?",
			now:      time.Date(2026, 8, 12, 20, 15, 0, 0, time.UTC),
			expected: "should-we-adopt-event-sourcing-20260812-201500",
		},
		{
			name:     "falls back when slug empty",
			topic:    "???",
			now:      time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			expected: "debate-20260101-000000",
		},
		{
			name:     "special characters removed",
			topic:    "Hello, World! @#$%",
			now:      time.Date(2026, 6, 15, 10, 30, 0, 0, time.UTC),
			expected: "hello-world-20260615-103000",
		},
		{
			name:     "multiple spaces collapsed",
			topic:    "a   b    c",
			now:      time.Date(2026, 6, 15, 10, 30, 0, 0, time.UTC),
			expected: "a-b-c-20260615-103000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id := NewSessionID(tt.topic, tt.now)
			if id != tt.expected {
				t.Errorf("got %q, want %q", id, tt.expected)
			}
		})
	}
}
