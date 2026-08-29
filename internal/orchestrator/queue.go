package orchestrator

import (
	"context"
	"sync"

	"github.com/trollLemon/agon/internal/archive"
)

// DebateQueue is a FIFO for App TUI visibility that mirrors the worker
// channel. The channel is authoritative for RunDebates; the slice lets
// the UI render "queued" without draining the channel.
type DebateQueue struct {
	ch    chan *Debate
	mu    sync.Mutex
	items []*Debate
}

func NewDebateQueue() *DebateQueue {
	return &DebateQueue{ch: make(chan *Debate, 4)}
}

func (q *DebateQueue) Enqueue(d *Debate) {
	q.mu.Lock()
	wasEmpty := len(q.items) == 0
	q.items = append(q.items, d)
	if wasEmpty {
		d.SetLive(true)
	} else {
		d.SetLive(false)
	}
	q.mu.Unlock()
	q.ch <- d
}

func (q *DebateQueue) Chan() <-chan *Debate { return q.ch }

func (q *DebateQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.items)
}

func (q *DebateQueue) QueuedCount() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.items) <= 1 {
		return 0
	}
	return len(q.items) - 1
}

func (q *DebateQueue) Peek() *Debate {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.items) == 0 {
		return nil
	}
	return q.items[0]
}

func RunDebates(ctx context.Context, ready <-chan error, debates <-chan *Debate, archiveDir string, onEvent func(Event)) error {
	readyErr, err := waitReady(ctx, ready)
	if err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case d := <-debates:
			if readyErr != nil {
				// TODO: surface init error — sticky, fix in future PR
				continue
			}
			_ = runOne(ctx, d, archiveDir, onEvent)
		}
	}
}

func waitReady(ctx context.Context, ready <-chan error) (error, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case err := <-ready:
		return err, nil
	}
}

func runOne(ctx context.Context, d *Debate, archiveDir string, onEvent func(Event)) error {
	go drainAndNotify(d, onEvent)
	debateCtx := context.WithoutCancel(ctx)
	sess, err := d.Run(debateCtx)
	if err == nil {
		if werr := archive.Write(archiveDir, sess); werr != nil {
			// TODO: log archive write error
		}
	}
	// TODO: request termination logic in future PR
	return err
}

func drainAndNotify(d *Debate, onEvent func(Event)) {
	for ev := range d.Events() {
		if onEvent != nil {
			onEvent(ev)
		}
	}
}
