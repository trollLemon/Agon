package orchestrator

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/agentdisc/goagentdisc/internal/archive"
	"github.com/agentdisc/goagentdisc/internal/prompts"
	"github.com/agentdisc/goagentdisc/internal/tools"
)

func baseConfig(rounds int) Config {
	return Config{
		SessionID: "s1",
		Title:     "Adopt X",
		Topic:     "Should we adopt X?",
		Mode:      prompts.ModeProposition,
		Tone:      prompts.ToneFormal,
		Rounds:    rounds,
		Sides: [2]archive.Side{
			{ID: "advocate", Label: "Advocate", Stance: "for"},
			{ID: "critic", Label: "Critic", Stance: "against"},
		},
		Model:     "unsloth/Qwen3-0.6B-Q8_0",
		CreatedAt: time.Date(2026, 8, 12, 20, 0, 0, 0, time.UTC),
	}
}

func TestRunTwoRoundDebateProducesOrderedTranscriptAndVerdict(t *testing.T) {
	client := newFakeClient(map[Role][]scriptStep{
		"advocate": {{content: "advocate r1"}, {content: "advocate r2"}},
		"critic":   {{content: "critic r1"}, {content: "critic r2"}},
		RoleJudge:  {{content: "verdict: adopt"}},
	})

	d := New(baseConfig(2), client, nil)
	sess, err := d.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if sess.Verdict != "verdict: adopt" {
		t.Errorf("got verdict %q", sess.Verdict)
	}
	if len(sess.Messages) != 4 {
		t.Fatalf("expected 4 messages, got %d: %+v", len(sess.Messages), sess.Messages)
	}
	want := []struct {
		role    string
		round   int
		content string
	}{
		{"advocate", 1, "advocate r1"},
		{"critic", 1, "critic r1"},
		{"advocate", 2, "advocate r2"},
		{"critic", 2, "critic r2"},
	}
	for i, w := range want {
		m := sess.Messages[i]
		if m.Role != w.role || m.Round != w.round || m.Content != w.content {
			t.Errorf("message %d: got %+v, want %+v", i, m, w)
		}
	}
	if sess.SessionID != "s1" || sess.Title != "Adopt X" || sess.Mode != "proposition" {
		t.Errorf("unexpected metadata: %+v", sess)
	}
}

func TestRunSendsOpeningContextOnlyOnRoundOne(t *testing.T) {
	client := newFakeClient(map[Role][]scriptStep{
		"advocate": {{content: "a1"}, {content: "a2"}},
		"critic":   {{content: "c1"}, {content: "c2"}},
		RoleJudge:  {{content: "v"}},
	})
	cfg := baseConfig(2)
	cfg.StartingContext = "we currently use Y"
	d := New(cfg, client, nil)
	if _, err := d.Run(context.Background()); err != nil {
		t.Fatal(err)
	}

	// advocate's history: [system, user(r1 topic+context), assistant(a1), user(r2 peer msg), assistant(a2)]
	hist := client.lastHistory["advocate"]
	if len(hist) < 3 {
		t.Fatalf("expected advocate history to include at least 3 entries, got %d", len(hist))
	}
}

func TestVersusModeUsesStanceLabels(t *testing.T) {
	client := newFakeClient(map[Role][]scriptStep{
		"rust":    {{content: "rust case"}},
		"go":      {{content: "go case"}},
		RoleJudge: {{content: "go wins"}},
	})
	cfg := baseConfig(1)
	cfg.Mode = prompts.ModeVersus
	cfg.Sides = [2]archive.Side{
		{ID: "rust", Label: "Team Rust", Stance: "Rust"},
		{ID: "go", Label: "Team Go", Stance: "Go"},
	}
	d := New(cfg, client, nil)
	sess, err := d.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if sess.Sides[0].ID != "rust" || sess.Sides[1].ID != "go" {
		t.Errorf("unexpected sides: %+v", sess.Sides)
	}
}

func TestRunEmitsEventsInOrder(t *testing.T) {
	client := newFakeClient(map[Role][]scriptStep{
		"advocate": {{content: "a1"}},
		"critic":   {{content: "c1"}},
		RoleJudge:  {{content: "v"}},
	})
	d := New(baseConfig(1), client, nil)
	if _, err := d.Run(context.Background()); err != nil {
		t.Fatal(err)
	}

	var kinds []EventKind
	for ev := range d.Events() {
		kinds = append(kinds, ev.Kind)
	}
	want := []EventKind{
		EventTurnStart, EventToken, EventTurnEnd, // advocate
		EventTurnStart, EventToken, EventTurnEnd, // critic
		EventTurnStart, EventToken, EventTurnEnd, // judge
		EventVerdict,
	}
	if len(kinds) != len(want) {
		t.Fatalf("got %d events %v, want %d %v", len(kinds), kinds, len(want), want)
	}
	for i := range want {
		if kinds[i] != want[i] {
			t.Errorf("event %d: got %v, want %v", i, kinds[i], want[i])
		}
	}
}

func TestToolCallSubLoopBoundedAtMax(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "f.txt"), []byte("hi"), 0o644)
	sb, err := tools.NewSandbox([]string{root})
	if err != nil {
		t.Fatal(err)
	}

	client := &alwaysToolClient{}
	cfg := baseConfig(1)
	cfg.SandboxDirs = []string{root}
	d := New(cfg, client, sb)

	_, err = d.Run(context.Background())
	if err == nil {
		t.Fatal("expected error from unbounded tool looping")
	}
	if client.calls != maxToolIterations {
		t.Errorf("expected exactly %d chat calls before giving up, got %d", maxToolIterations, client.calls)
	}
}

func TestToolCallExecutesAgainstSandboxAndAppendsToArchive(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "f.txt"), []byte("hello world"), 0o644)
	sb, err := tools.NewSandbox([]string{root})
	if err != nil {
		t.Fatal(err)
	}

	client := newFakeClient(map[Role][]scriptStep{
		"advocate": {
			{toolCalls: []ToolCallRequest{{ID: "1", Name: "read_file", Arguments: map[string]any{"path": "f.txt"}}}},
			{content: "cited from f.txt"},
		},
		"critic":  {{content: "c1"}},
		RoleJudge: {{content: "v"}},
	})
	cfg := baseConfig(1)
	cfg.SandboxDirs = []string{root}
	d := New(cfg, client, sb)

	sess, err := d.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(sess.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(sess.Messages))
	}
	advocateMsg := sess.Messages[0]
	if advocateMsg.Content != "cited from f.txt" {
		t.Errorf("got content %q", advocateMsg.Content)
	}
	if len(advocateMsg.ToolCalls) != 1 || advocateMsg.ToolCalls[0].Name != "read_file" {
		t.Fatalf("expected recorded tool call, got %+v", advocateMsg.ToolCalls)
	}

	specs := client.toolSpecsSeen("advocate")
	if len(specs) == 0 {
		t.Error("expected advocate to be offered tool specs when a repo is in scope")
	}
}

func TestToolCallWithoutSandboxErrors(t *testing.T) {
	client := newFakeClient(map[Role][]scriptStep{
		"advocate": {{toolCalls: []ToolCallRequest{{ID: "1", Name: "read_file", Arguments: map[string]any{"path": "f.txt"}}}}},
	})
	cfg := baseConfig(1)
	d := New(cfg, client, nil)
	sess, err := d.Run(context.Background())
	if err == nil {
		t.Fatal("expected error when a tool call is requested with no sandbox")
	}
	if sess.SessionID != "" {
		t.Errorf("expected zero-value session on failure, got %+v", sess)
	}
}

func TestModelErrorAbortsWithNoArchive(t *testing.T) {
	client := newFakeClient(map[Role][]scriptStep{
		"advocate": {{err: errBoom}},
	})
	d := New(baseConfig(1), client, nil)
	sess, err := d.Run(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if sess.SessionID != "" {
		t.Errorf("expected zero-value session on error, got %+v", sess)
	}

	var kinds []EventKind
	for ev := range d.Events() {
		kinds = append(kinds, ev.Kind)
	}
	if len(kinds) == 0 || kinds[len(kinds)-1] != EventError {
		t.Errorf("expected a trailing EventError, got %v", kinds)
	}
}

func TestAbortDuringTurnDiscardsTranscript(t *testing.T) {
	client := newBlockingClient()
	d := New(baseConfig(3), client, nil)

	type result struct {
		sess archive.Session
		err  error
	}
	resultCh := make(chan result, 1)
	go func() {
		sess, err := d.Run(context.Background())
		resultCh <- result{sess, err}
	}()

	waitFor(t, func() bool {
		select {
		case <-client.started:
			return true
		default:
			return false
		}
	}, time.Second, "model call to start")

	d.Abort("user requested stop")

	select {
	case r := <-resultCh:
		if r.err == nil {
			t.Fatal("expected an error after abort")
		}
		var aerr *AbortedError
		if !isAbortedError(r.err, &aerr) {
			t.Fatalf("expected *AbortedError, got %T: %v", r.err, r.err)
		}
		if aerr.Reason != "user requested stop" {
			t.Errorf("got reason %q", aerr.Reason)
		}
		if r.sess.SessionID != "" {
			t.Errorf("expected zero-value session after abort, got %+v", r.sess)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for aborted Run to return")
	}
}

func TestAbortBeforeRunStillStopsIt(t *testing.T) {
	client := newBlockingClient()
	d := New(baseConfig(1), client, nil)
	d.Abort("changed my mind")

	sess, err := d.Run(context.Background())
	if err == nil {
		t.Fatal("expected immediate abort error")
	}
	if sess.SessionID != "" {
		t.Errorf("expected zero-value session, got %+v", sess)
	}
}

var errBoom = fakeErr("boom")

type fakeErr string

func (e fakeErr) Error() string { return string(e) }

func isAbortedError(err error, target **AbortedError) bool {
	ae, ok := err.(*AbortedError)
	if ok {
		*target = ae
	}
	return ok
}
