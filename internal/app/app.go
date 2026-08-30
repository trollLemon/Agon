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

package app

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/trollLemon/agon/internal/archive"
	"github.com/trollLemon/agon/internal/orchestrator"
	"github.com/trollLemon/agon/internal/prompts"
	"github.com/trollLemon/agon/internal/tools"
	"github.com/trollLemon/agon/internal/tui"
)

type Options struct {
	ArchiveDir   string
	DefaultModel string
}

type App struct {
	screen        tui.Screen
	width, height int

	archiveDir   string
	defaultModel string

	engine orchestrator.Engine

	ctx        context.Context
	cancel     context.CancelFunc
	readyChan  chan error
	queue      *orchestrator.DebateQueue
	progressCh chan orchestrator.Event
	initOnce   sync.Once
	bootLog    *tui.BootLog

	mu          sync.Mutex
	view        tui.SessionView
	curDebate   *orchestrator.Debate
	initialized bool

	currentContent strings.Builder
	currentTools   []archive.ToolCall

	menu        tui.MenuModel
	form        tui.FormModel
	bootScreen  tui.BootstrapModel
	archiveList tui.ArchiveListModel
	session     tui.SessionModel
	queueList   tui.QueueListModel
}

func New(opts Options, engine orchestrator.Engine) *App {
	if opts.DefaultModel == "" {
		opts.DefaultModel = orchestrator.DefaultModel
	}
	ctx, cancel := context.WithCancel(context.Background())
	readyChan := make(chan error, 1)
	queue := orchestrator.NewDebateQueue()
	progressCh := make(chan orchestrator.Event, 512)
	a := &App{
		screen:       tui.ScreenMenu,
		archiveDir:   opts.ArchiveDir,
		defaultModel: opts.DefaultModel,
		engine:       engine,
		ctx:          ctx,
		cancel:       cancel,
		readyChan:    readyChan,
		queue:        queue,
		progressCh:   progressCh,
		menu:         tui.NewMenuModel(),
		form:         tui.NewFormModel(opts.DefaultModel),
		bootScreen:   tui.NewBootstrapModel(),
		archiveList:  tui.NewArchiveListModel(opts.ArchiveDir),
		session:      tui.NewSessionModel(),
		queueList:    tui.NewQueueListModel(),
	}
	onEvent := func(ev orchestrator.Event) {
		a.accumulateEvent(ev)
		select {
		case a.progressCh <- ev:
		default:
		}
	}
	go orchestrator.RunDebates(ctx, readyChan, queue.Chan(), opts.ArchiveDir, onEvent)
	return a
}

func Run(opts Options, engine orchestrator.Engine) error {
	p := tea.NewProgram(New(opts, engine), tea.WithAltScreen())
	_, err := p.Run()
	return err
}

func (a *App) Init() tea.Cmd {
	return a.archiveList.Reload()
}

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width, a.height = msg.Width, msg.Height
		a.session.SetSize(msg.Width, msg.Height)
		a.bootScreen.SetSize(msg.Width, msg.Height)
		return a, nil

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			a.mu.Lock()
			cur := a.curDebate
			a.mu.Unlock()
			if cur != nil {
				cur.Abort("app quit")
			}
			a.cancel()
			return a, tea.Quit
		}
		return a.handleKey(msg)

	case tui.SwitchScreenMsg:
		a.screen = msg.Screen
		switch msg.Screen {
		case tui.ScreenMenu:
		case tui.ScreenForm:
			a.form = tui.NewFormModel(a.defaultModel)
		case tui.ScreenArchive:
			return a, a.archiveList.Reload()
		case tui.ScreenSession:
			a.refreshSession()
			return a, a.waitForProgress()
		case tui.ScreenQueue:
			a.queueList.SetItems(a.allQueued())
			return a, nil
		}
		return a, nil

	case tui.StartDebateMsg:
		return a.handleStartDebate(msg)

	case tui.BootstrapDoneMsg:
		return a.handleBootstrapDone(msg)

	case tui.OpenArchivedMsg:
		return a.openArchived(msg.SessionID)

	case tui.DebateProgressMsg:
		return a.handleDebateProgress(msg)

	case tui.BootLogTickMsg:
		a.bootScreen.Refresh()
		if a.screen == tui.ScreenBootstrap {
			return a, tui.WaitForBootLog()
		}
		return a, nil

	case tui.ArchiveListLoadedMsg:
		a.archiveList.SetItems(msg.Items)
		return a, nil
	}
	return a, nil
}

func (a *App) View() string {
	switch a.screen {
	case tui.ScreenSession:
		return a.session.View()
	case tui.ScreenBootstrap:
		return a.bootScreen.View()
	case tui.ScreenForm:
		return a.form.View()
	case tui.ScreenArchive:
		a.archiveList.SetQueued(a.queue.Peek(), a.queue.QueuedItems())
		return a.archiveList.View()
	case tui.ScreenQueue:
		a.queueList.SetItems(a.allQueued())
		return a.queueList.View()
	default:
		return a.menu.View(a.isLive(), a.queue.QueuedCount())
	}
}

func (a *App) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch a.screen {
	case tui.ScreenMenu:
		a.menu, cmd = a.menu.Update(msg, a.isLive(), a.queue.QueuedCount())
	case tui.ScreenForm:
		a.form, cmd = a.form.Update(msg)
	case tui.ScreenBootstrap:
		a.bootScreen, cmd = a.bootScreen.HandleKey(msg)
	case tui.ScreenSession:
		a.mu.Lock()
		cur := a.curDebate
		a.mu.Unlock()
		a.session, cmd = a.session.Update(msg, cur)
	case tui.ScreenArchive:
		a.archiveList, cmd = a.archiveList.Update(msg)
	case tui.ScreenQueue:
		a.queueList, cmd = a.queueList.Update(msg)
	}
	return a, cmd
}

func (a *App) handleStartDebate(msg tui.StartDebateMsg) (tea.Model, tea.Cmd) {
	var sandbox *tools.Sandbox
	var sandboxDirs, sandboxFiles []string
	if paths := tools.ParsePathList(msg.Sandbox); len(paths) > 0 {
		sb, err := tools.NewSandbox(paths)
		if err != nil {
			a.form.SetError("sandbox: " + err.Error())
			return a, nil
		}
		sandbox = sb
		sandboxDirs = sb.Dirs()
		sandboxFiles = sb.Files()
	}
	now := time.Now()
	cfg := orchestrator.Config{
		SessionID:       archive.NewSessionID(msg.Topic, now),
		Title:           archive.SummarizeTitle(msg.Topic),
		Topic:           msg.Topic,
		StartingContext: msg.Context,
		Mode:            msg.Mode,
		Tone:            msg.Tone,
		Rounds:          msg.Rounds,
		Sides:           defaultSides(msg.Mode),
		Model:           msg.Model,
		SandboxDirs:     sandboxDirs,
		SandboxFiles:    sandboxFiles,
		CreatedAt:       now,
	}
	d := orchestrator.New(cfg, a.engine, sandbox)

	a.mu.Lock()
	alreadyInitialized := a.initialized
	a.mu.Unlock()

	if !alreadyInitialized {
		bl := tui.NewBootLog()
		a.bootLog = bl
		a.bootScreen.Start(bl)
		a.screen = tui.ScreenBootstrap
		modelSource := msg.Model
		eng := a.engine
		ctx := a.ctx
		readyCh := a.readyChan
		var bootstrapCmd tea.Cmd
		a.initOnce.Do(func() {
			bootstrapCmd = func() tea.Msg {
				c, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
				defer cancel()
				_ = ctx
				err := eng.Initialize(c, modelSource, bl.Append)
				readyCh <- err
				if err != nil {
					a.initOnce = sync.Once{}
				} else {
					a.mu.Lock()
					a.initialized = true
					a.mu.Unlock()
				}
				return tui.BootstrapDoneMsg{Err: err}
			}
		})
		a.queue.Enqueue(d)
		if d.IsLive() {
			a.mu.Lock()
			a.view = tui.SessionView{
				SessionID: cfg.SessionID,
				Title:     cfg.Title,
				Topic:     cfg.Topic,
				Mode:      string(cfg.Mode),
				Tone:      string(cfg.Tone),
				Rounds:    cfg.Rounds,
				Sides:     []archive.Side{cfg.Sides[0], cfg.Sides[1]},
				Live:      true,
			}
			a.curDebate = d
			a.currentContent.Reset()
			a.currentTools = nil
			a.mu.Unlock()
		}
		if bootstrapCmd != nil {
			return a, tea.Batch(bootstrapCmd, tui.WaitForBootLog())
		}
		return a, tui.WaitForBootLog()
	}

	a.queue.Enqueue(d)
	if d.IsLive() {
		a.mu.Lock()
		a.view = tui.SessionView{
			SessionID: cfg.SessionID,
			Title:     cfg.Title,
			Topic:     cfg.Topic,
			Mode:      string(cfg.Mode),
			Tone:      string(cfg.Tone),
			Rounds:    cfg.Rounds,
			Sides:     []archive.Side{cfg.Sides[0], cfg.Sides[1]},
			Live:      true,
		}
		a.curDebate = d
		a.currentContent.Reset()
		a.currentTools = nil
		a.mu.Unlock()
	}
	a.screen = tui.ScreenSession
	a.refreshSession()
	return a, a.waitForProgress()
}

func (a *App) handleBootstrapDone(msg tui.BootstrapDoneMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		a.bootScreen.SetError(msg.Err)
		return a, nil
	}
	a.mu.Lock()
	a.initialized = true
	if a.queue.Len() > 0 && !a.view.Live {
		cfg := a.queue.Peek().Config()
		a.view = tui.SessionView{
			SessionID: cfg.SessionID,
			Title:     cfg.Title,
			Topic:     cfg.Topic,
			Mode:      string(cfg.Mode),
			Tone:      string(cfg.Tone),
			Rounds:    cfg.Rounds,
			Sides:     []archive.Side{cfg.Sides[0], cfg.Sides[1]},
			Live:      true,
		}
		a.curDebate = a.queue.Peek()
	}
	a.mu.Unlock()
	a.screen = tui.ScreenSession
	a.refreshSession()
	return a, tea.Batch(a.archiveList.Reload(), a.waitForProgress())
}

func (a *App) handleDebateProgress(msg tui.DebateProgressMsg) (tea.Model, tea.Cmd) {
	a.refreshSession()
	if a.screen == tui.ScreenQueue {
		a.queueList.SetItems(a.allQueued())
	}
	if a.screen == tui.ScreenSession {
		a.mu.Lock()
		done := a.view.Done
		a.mu.Unlock()
		if done {
			return a, tea.Batch(a.archiveList.Reload(), a.waitForProgress())
		}
		return a, a.waitForProgress()
	}
	return a, a.waitForProgress()
}

func (a *App) openArchived(sessionID string) (tea.Model, tea.Cmd) {
	a.mu.Lock()
	curID := a.view.SessionID
	live := a.view.Live
	a.mu.Unlock()
	if live && curID == sessionID {
		a.refreshSession()
		a.screen = tui.ScreenSession
		return a, a.waitForProgress()
	}
	sess, err := archive.Load(a.archiveDir, sessionID)
	if err != nil {
		return a, nil
	}
	a.session.ShowArchived(sess)
	a.screen = tui.ScreenSession
	return a, nil
}

func (a *App) refreshSession() {
	a.mu.Lock()
	defer a.mu.Unlock()
	v := a.view
	v.Queued = a.queue.QueuedCount()
	a.session.SetView(v)
}

func (a *App) accumulateEvent(ev orchestrator.Event) {
	// lock covers view + currentContent/currentTools; held only for
	// string copy / slice append (microseconds). Events are serialized
	// (buffer 512, blocking emit) and at most a few per turn, so
	// contention is negligible.
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.view.Done {
		return
	}
	switch ev.Kind {
	case orchestrator.EventTurnStart:
		a.view.CurrentRole = ev.Role
		a.view.CurrentRound = ev.Round
		a.currentContent.Reset()
		a.currentTools = nil
		a.view.CurrentContent = ""
		a.view.CurrentTools = nil
	case orchestrator.EventToken:
		a.currentContent.WriteString(ev.Text)
		a.view.CurrentContent = a.currentContent.String()
	case orchestrator.EventToolCall:
		if ev.Tool != nil {
			a.currentTools = append(a.currentTools, *ev.Tool)
			a.view.CurrentTools = append([]archive.ToolCall(nil), a.currentTools...)
		}
	case orchestrator.EventTurnEnd:
		if ev.Role != string(orchestrator.RoleJudge) {
			a.view.Messages = append(a.view.Messages, archive.Message{
				Role: ev.Role, Round: ev.Round, Content: a.currentContent.String(),
				ToolCalls: append([]archive.ToolCall(nil), a.currentTools...),
			})
		}
		a.view.CurrentRole = ""
		a.view.CurrentRound = 0
		a.currentContent.Reset()
		a.currentTools = nil
		a.view.CurrentContent = ""
		a.view.CurrentTools = nil
	case orchestrator.EventVerdict:
		a.view.Verdict = ev.Text
		a.view.Done = true
	case orchestrator.EventAborted:
		a.view.Err = &orchestrator.AbortedError{Reason: ev.Text}
		a.view.Done = true
	case orchestrator.EventError:
		if ev.Text != "" {
			a.view.Err = fmt.Errorf("%s", ev.Text)
		} else {
			a.view.Err = fmt.Errorf("debate error")
		}
		a.view.Done = true
	}
}

func (a *App) waitForProgress() tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-a.progressCh
		if !ok {
			return nil
		}
		a.mu.Lock()
		sid := a.view.SessionID
		a.mu.Unlock()
		return tui.DebateProgressMsg{SessionID: sid, Event: ev}
	}
}

func (a *App) isLive() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.view.Live && !a.view.Done
}

func (a *App) allQueued() []*orchestrator.Debate {
	items := []*orchestrator.Debate{}
	if live := a.queue.Peek(); live != nil {
		items = append(items, live)
		items = append(items, a.queue.QueuedItems()...)
	}
	return items
}

func defaultSides(mode prompts.Mode) [2]archive.Side {
	if mode == prompts.ModeVersus {
		return [2]archive.Side{
			{ID: "optiona", Label: "Option A", Stance: "Option A"},
			{ID: "optionb", Label: "Option B", Stance: "Option B"},
		}
	}
	return [2]archive.Side{
		{ID: "advocate", Label: "Advocate", Stance: "for"},
		{ID: "critic", Label: "Critic", Stance: "against"},
	}
}
