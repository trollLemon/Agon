package orchestrator

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/trollLemon/agon/internal/archive"
	"github.com/trollLemon/agon/internal/prompts"
)

func newQueueTestDebate(id string, client ChatClient) *Debate {
	cfg := Config{
		SessionID: id,
		Title:     id,
		Topic:     "topic " + id,
		Mode:      prompts.ModeProposition,
		Tone:      prompts.ToneFormal,
		Rounds:    1,
		Sides: [2]archive.Side{
			{ID: "advocate", Label: "Advocate", Stance: "for"},
			{ID: "critic", Label: "Critic", Stance: "against"},
		},
		Model:     "test",
		CreatedAt: time.Now(),
	}
	return New(cfg, client, nil)
}

func TestRunDebatesTable(t *testing.T) {
	fakeOk := newFakeClient(map[Role][]scriptStep{
		"advocate": {{content: "a1"}},
		"critic":   {{content: "c1"}},
		RoleJudge:  {{content: "v1"}},
	})
	fakeOk2 := newFakeClient(map[Role][]scriptStep{
		"advocate": {{content: "a2"}},
		"critic":   {{content: "c2"}},
		RoleJudge:  {{content: "v2"}},
	})
	abortedDebate := func() *Debate {
		d := newQueueTestDebate("aborted", newBlockingClient())
		d.Abort("test abort")
		return d
	}()
	errorClient := newFakeClient(map[Role][]scriptStep{
		"advocate": {{err: fakeErr("boom")}},
	})
	errorDebate := newQueueTestDebate("err1", errorClient)

	tests := []struct {
		name       string
		readyErr   error
		debates    []*Debate
		wantWrites int
	}{
		{name: "single success writes exactly once", readyErr: nil, debates: []*Debate{newQueueTestDebate("s1", fakeOk)}, wantWrites: 1},
		{name: "fifo two debates in order", readyErr: nil, debates: []*Debate{newQueueTestDebate("fifo1", fakeOk), newQueueTestDebate("fifo2", fakeOk2)}, wantWrites: 2},
		{name: "ready error skips all", readyErr: fakeErr("init failed"), debates: []*Debate{newQueueTestDebate("skip1", fakeOk)}, wantWrites: 0},
		{name: "abort no write", readyErr: nil, debates: []*Debate{abortedDebate}, wantWrites: 0},
		{name: "error no write", readyErr: nil, debates: []*Debate{errorDebate}, wantWrites: 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			runQueueTableCase(t, tc.readyErr, tc.debates, tc.wantWrites)
		})
	}
}

func runQueueTableCase(t *testing.T, readyErr error, debates []*Debate, wantWrites int) {
	t.Helper()
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	readyCh := make(chan error, 1)
	debateCh := make(chan *Debate, 4)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = RunDebates(ctx, readyCh, debateCh, dir, nil)
	}()
	readyCh <- readyErr
	time.Sleep(5 * time.Millisecond)
	for _, d := range debates {
		debateCh <- d
	}
	waitForWrites(t, dir, wantWrites, 2*time.Second)
	cancel()
	wg.Wait()
	assertWriteCount(t, dir, wantWrites)
}

func waitForWrites(t *testing.T, dir string, want int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		list, _ := archive.List(dir)
		if len(list) == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func assertWriteCount(t *testing.T, dir string, want int) {
	t.Helper()
	list, err := archive.List(dir)
	if err != nil {
		t.Fatalf("archive.List: %v", err)
	}
	if len(list) != want {
		t.Fatalf("want %d writes, got %d", want, len(list))
	}
}

func TestHoldUntilReady(t *testing.T) {
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	readyCh := make(chan error, 1)
	debateCh := make(chan *Debate, 4)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = RunDebates(ctx, readyCh, debateCh, dir, nil)
	}()
	client := newFakeClient(map[Role][]scriptStep{
		"advocate": {{content: "a"}},
		"critic":   {{content: "c"}},
		RoleJudge:  {{content: "v"}},
	})
	d := newQueueTestDebate("hold1", client)
	debateCh <- d
	time.Sleep(50 * time.Millisecond)
	if list, _ := archive.List(dir); len(list) != 0 {
		t.Fatalf("expected no writes before ready, got %d", len(list))
	}
	readyCh <- nil
	waitForWrites(t, dir, 1, 2*time.Second)
	cancel()
	wg.Wait()
	assertWriteCount(t, dir, 1)
}

func TestCtxCancelWaitsForFinish(t *testing.T) {
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	readyCh := make(chan error, 1)
	debateCh := make(chan *Debate, 4)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = RunDebates(ctx, readyCh, debateCh, dir, nil)
	}()
	readyCh <- nil
	time.Sleep(10 * time.Millisecond)
	client := newFakeClient(map[Role][]scriptStep{
		"advocate": {{content: "a"}},
		"critic":   {{content: "c"}},
		RoleJudge:  {{content: "v"}},
	})
	d := newQueueTestDebate("ctxwait1", client)
	debateCh <- d
	time.Sleep(5 * time.Millisecond)
	cancel()
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("RunDebates did not return after ctx cancel")
	}
	list, _ := archive.List(dir)
	if len(list) != 1 {
		t.Fatalf("expected exactly one write after ctx cancel wait, got %d", len(list))
	}
}

func TestRunDebatesFifoOrder(t *testing.T) {
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	readyCh := make(chan error, 1)
	debateCh := make(chan *Debate, 4)
	var wg sync.WaitGroup
	wg.Add(1)
	var events []string
	var evMu sync.Mutex
	onEvent := func(ev Event) {
		if ev.Kind == EventVerdict {
			evMu.Lock()
			events = append(events, ev.Text)
			evMu.Unlock()
		}
	}
	go func() {
		defer wg.Done()
		_ = RunDebates(ctx, readyCh, debateCh, dir, onEvent)
	}()
	readyCh <- nil
	c1 := newFakeClient(map[Role][]scriptStep{
		"advocate": {{content: "a1"}},
		"critic":   {{content: "c1"}},
		RoleJudge:  {{content: "verdict1"}},
	})
	c2 := newFakeClient(map[Role][]scriptStep{
		"advocate": {{content: "a2"}},
		"critic":   {{content: "c2"}},
		RoleJudge:  {{content: "verdict2"}},
	})
	d1 := newQueueTestDebate("fifo-order1", c1)
	d2 := newQueueTestDebate("fifo-order2", c2)
	debateCh <- d1
	debateCh <- d2
	waitForWrites(t, dir, 2, 3*time.Second)
	cancel()
	wg.Wait()
	evMu.Lock()
	defer evMu.Unlock()
	if len(events) != 2 || events[0] != "verdict1" || events[1] != "verdict2" {
		t.Fatalf("fifo order violated, got events %v", events)
	}
}

func TestRunDebatesExactlyOnce(t *testing.T) {
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	readyCh := make(chan error, 1)
	debateCh := make(chan *Debate, 4)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = RunDebates(ctx, readyCh, debateCh, dir, nil)
	}()
	readyCh <- nil
	client := newFakeClient(map[Role][]scriptStep{
		"advocate": {{content: "a"}},
		"critic":   {{content: "c"}},
		RoleJudge:  {{content: "v"}},
	})
	d := newQueueTestDebate("once1", client)
	debateCh <- d
	waitForWrites(t, dir, 1, 2*time.Second)
	cancel()
	wg.Wait()
	list, _ := archive.List(dir)
	if len(list) != 1 {
		t.Fatalf("expected exactly once, got %d", len(list))
	}
	list2, _ := archive.List(dir)
	if len(list2) != 1 {
		t.Fatalf("second list mismatch")
	}
}

func TestRunDebates(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	readyChan := make(chan error, 1)
	debateChan := make(chan *Debate, 4)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = RunDebates(ctx, readyChan, debateChan, "/tmp/test", nil)
	}()
	readyChan <- nil
	time.Sleep(10 * time.Millisecond)
	cancel()
	wg.Wait()
}

func TestRunDebatesAPI(t *testing.T) {
	var fn func(context.Context, <-chan error, <-chan *Debate, string, func(Event)) error = RunDebates
	if fn == nil {
		t.Error("RunDebates function not found")
	}
}
