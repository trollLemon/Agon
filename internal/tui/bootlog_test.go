package tui

import "testing"

func TestBootLogAppendAndTail(t *testing.T) {
	tests := []struct {
		name     string
		appends  []struct {
			msg  string
			args []any
		}
		tail    int
		want    []string
		wantLen int
	}{
		{
			name: "basic append and tail",
			appends: []struct {
				msg  string
				args []any
			}{
				{"download-libraries: waiting to start download...", []any{"tag", "v1.2.3"}},
				{"download-model: installed", []any{"model", "unsloth/Qwen3-0.6B-Q8_0"}},
			},
			tail:    10,
			want:    []string{"download-libraries: waiting to start download... tag[v1.2.3]", "download-model: installed model[unsloth/Qwen3-0.6B-Q8_0]"},
			wantLen: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bl := NewBootLog()
			for _, a := range tt.appends {
				bl.Append(a.msg, a.args...)
			}
			got := bl.Tail(tt.tail)
			if len(got) != tt.wantLen {
				t.Fatalf("expected %d lines, got %d: %v", tt.wantLen, len(got), got)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("line %d: expected %q, got %q", i, tt.want[i], got[i])
				}
			}
		})
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

func TestBootLogTailLimits(t *testing.T) {
	tests := []struct {
		name       string
		numAppends int
		tail       int
		wantLen    int
	}{
		{"caps to requested count", 20, 5, 5},
		{"fewer than requested", 1, 10, 1},
		{"exact match", 5, 5, 5},
		{"zero tail", 3, 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bl := NewBootLog()
			for i := 0; i < tt.numAppends; i++ {
				bl.Append("line")
			}
			got := bl.Tail(tt.tail)
			if len(got) != tt.wantLen {
				t.Errorf("expected %d lines, got %d", tt.wantLen, len(got))
			}
		})
	}
}
