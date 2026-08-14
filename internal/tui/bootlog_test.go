package tui

import "testing"

func TestBootLogAppendAndTail(t *testing.T) {
	bl := NewBootLog()
	bl.Append("download-libraries: waiting to start download...", "tag", "v1.2.3")
	bl.Append("download-model: installed", "model", "unsloth/Qwen3-0.6B-Q8_0")

	got := bl.Tail(10)
	want := []string{
		"download-libraries: waiting to start download... tag[v1.2.3]",
		"download-model: installed model[unsloth/Qwen3-0.6B-Q8_0]",
	}
	if len(got) != len(want) {
		t.Fatalf("expected %d lines, got %d: %v", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d: expected %q, got %q", i, want[i], got[i])
		}
	}
}

func TestBootLogProgressLinesReplaceInPlace(t *testing.T) {
	bl := NewBootLog()
	bl.Append("starting up")
	bl.Append("\rdownload-model: Downloading foo... 1 MB of 100 MB (1.00 MB/s)")
	bl.Append("\rdownload-model: Downloading foo... 50 MB of 100 MB (1.00 MB/s)")
	bl.Append("\rdownload-model: Downloading foo... 100 MB of 100 MB (1.00 MB/s)")
	bl.Append("done")

	got := bl.Tail(10)
	want := []string{
		"starting up",
		"download-model: Downloading foo... 100 MB of 100 MB (1.00 MB/s)",
		"done",
	}
	if len(got) != len(want) {
		t.Fatalf("expected progress lines to collapse to %d entries, got %d: %v", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d: expected %q, got %q", i, want[i], got[i])
		}
	}
}

func TestBootLogTailCapsToRequestedCount(t *testing.T) {
	bl := NewBootLog()
	for i := 0; i < 20; i++ {
		bl.Append("line")
	}
	if got := bl.Tail(5); len(got) != 5 {
		t.Errorf("expected Tail(5) to return 5 lines, got %d", len(got))
	}
}

func TestBootLogTailFewerThanRequested(t *testing.T) {
	bl := NewBootLog()
	bl.Append("only one line")
	if got := bl.Tail(10); len(got) != 1 {
		t.Errorf("expected 1 line when fewer than requested exist, got %d: %v", len(got), got)
	}
}
