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
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/trollLemon/agon/internal/archive"
	"github.com/trollLemon/agon/internal/cache"
	"github.com/trollLemon/agon/internal/orchestrator"
	"github.com/trollLemon/agon/internal/prompts"
	"github.com/trollLemon/agon/internal/tools"
	"github.com/trollLemon/agon/internal/tui"
	"github.com/trollLemon/agon/internal/types"
)

type Options struct {
	ArchiveDir   string
	CacheDir     string
	DefaultModel string
}

type App struct {
	screen        tui.Screen
	width, height int

	archiveDir   string
	cacheDir     string
	defaultModel string

	engine orchestrator.Engine

	ctx        context.Context
	cancel     context.CancelFunc
	progressCh chan orchestrator.Event
	bootLog    *tui.BootLog

	view        tui.SessionView
	curDebate   *orchestrator.Debate
	initialized bool

	currentContent strings.Builder
	currentTools   []types.ToolCall

	menu        tui.MenuModel
	form        tui.FormModel
	bootScreen  tui.BootstrapModel
	archiveList tui.ArchiveListModel
	cacheList   tui.CacheListModel
	session     tui.SessionModel
}

func defaultCacheDir(archiveDir string) string {
	if archiveDir == "" {
		return "cache"
	}
	dir := filepath.Dir(archiveDir)
	if dir == "." || dir == "" {
		return "cache"
	}
	return filepath.Join(dir, "cache")
}

func New(opts Options, engine orchestrator.Engine) *App {
	if opts.DefaultModel == "" {
		opts.DefaultModel = orchestrator.DefaultModel
	}
	if opts.CacheDir == "" {
		opts.CacheDir = defaultCacheDir(opts.ArchiveDir)
	}
	if opts.ArchiveDir == "" {
		opts.ArchiveDir = "debates"
	}
	ctx, cancel := context.WithCancel(context.Background())
	progressCh := make(chan orchestrator.Event, 512)
	a := &App{
		screen:       tui.ScreenMenu,
		archiveDir:   opts.ArchiveDir,
		cacheDir:     opts.CacheDir,
		defaultModel: opts.DefaultModel,
		engine:       engine,
		ctx:          ctx,
		cancel:       cancel,
		progressCh:   progressCh,
		menu:         tui.NewMenuModel(),
		form:         tui.NewFormModel(opts.DefaultModel),
		bootScreen:   tui.NewBootstrapModel(),
		archiveList:  tui.NewArchiveListModel(opts.ArchiveDir),
		cacheList:    tui.NewCacheListModel(opts.CacheDir),
		session:      tui.NewSessionModel(),
	}
	return a
}

func Run(opts Options, engine orchestrator.Engine) error {
	p := tea.NewProgram(New(opts, engine), tea.WithAltScreen())
	_, err := p.Run()
	return err
}

func (a *App) Init() tea.Cmd {
	return tea.Batch(a.archiveList.Reload(), a.cacheList.Reload())
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
			// Quit without aborting: the debate stays resumable in the cache.
			a.cancel()
			return a, tea.Quit
		}
		return a.handleKey(msg)

	case tui.SwitchScreenMsg:
		a.screen = msg.Screen
		switch msg.Screen {
		case tui.ScreenMenu:
			return a, a.cacheList.Reload()
		case tui.ScreenForm:
			a.form = tui.NewFormModel(a.defaultModel)
		case tui.ScreenArchive:
			return a, a.archiveList.Reload()
		case tui.ScreenResume:
			return a, a.cacheList.Reload()
		case tui.ScreenSession:
			a.refreshSession()
			return a, a.waitForProgress()
		}
		return a, nil

	case tui.StartDebateMsg:
		return a.handleStartDebate(msg)

	case tui.BootstrapDoneMsg:
		return a.handleBootstrapDone(msg)

	case tui.OpenArchivedMsg:
		return a.openArchived(msg.SessionID)

	case tui.OpenCachedMsg:
		return a.loadCacheIntoSession(msg.SessionID)

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

	case tui.CacheListLoadedMsg:
		if a.isLive() {
			filtered := make([]*types.Session, 0, len(msg.Items))
			liveID := a.view.SessionID
			for _, s := range msg.Items {
				if s.SessionID != liveID {
					filtered = append(filtered, s)
				}
			}
			a.cacheList.SetItems(filtered)
		} else {
			a.cacheList.SetItems(msg.Items)
		}
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
		return a.archiveList.View()
	case tui.ScreenResume:
		return a.cacheList.View()
	default:
		return a.menu.View(a.isLive(), a.hasResumableDebates())
	}
}

func (a *App) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch a.screen {
	case tui.ScreenMenu:
		a.menu, cmd = a.menu.Update(msg, a.isLive(), a.hasResumableDebates())
	case tui.ScreenForm:
		a.form, cmd = a.form.Update(msg)
	case tui.ScreenBootstrap:
		a.bootScreen, cmd = a.bootScreen.HandleKey(msg)
	case tui.ScreenSession:
		cur := a.curDebate
		a.session, cmd = a.session.Update(msg, cur)
	case tui.ScreenArchive:
		a.archiveList, cmd = a.archiveList.Update(msg)
	case tui.ScreenResume:
		a.cacheList, cmd = a.cacheList.Update(msg)
	}
	return a, cmd
}

func (a *App) handleStartDebate(msg tui.StartDebateMsg) (tea.Model, tea.Cmd) {
	if a.isLive() {
		a.form.SetError("a debate is already running; finish or abort it first")
		return a, nil
	}

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

	if err := a.initCacheForNewDebate(cfg); err != nil {
		a.form.SetError("cache: " + err.Error())
		return a, nil
	}

	view := tui.SessionViewFromSession(cfg.Session(), true)

	if !a.initialized {
		return a, a.enterBootstrap(msg.Model, d, view)
	}

	a.view = view
	a.curDebate = d
	a.currentContent.Reset()
	a.currentTools = nil
	a.screen = tui.ScreenSession
	a.refreshSession()
	a.runDebate(d)
	return a, tea.Batch(a.cacheList.Reload(), a.waitForProgress())
}

func (a *App) initCacheForNewDebate(cfg orchestrator.Config) error {
	sess := cfg.Session()
	return cache.InitCache(a.cacheDir, cfg.SessionID, sess)
}

func (a *App) enterBootstrap(modelSource string, d *orchestrator.Debate, view tui.SessionView) tea.Cmd {
	bl := tui.NewBootLog()
	a.bootLog = bl
	a.bootScreen.Start(bl)
	a.screen = tui.ScreenBootstrap
	a.view = view
	a.curDebate = d
	a.currentContent.Reset()
	a.currentTools = nil
	return tea.Batch(a.bootstrapCmd(modelSource, bl), tui.WaitForBootLog())
}

func (a *App) bootstrapCmd(modelSource string, bl *tui.BootLog) tea.Cmd {
	eng := a.engine
	return func() tea.Msg {
		c, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		err := eng.Initialize(c, modelSource, bl.Append)
		return tui.BootstrapDoneMsg{Err: err}
	}
}

func (a *App) handleBootstrapDone(msg tui.BootstrapDoneMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		a.bootScreen.SetError(msg.Err)
		return a, nil
	}
	a.initialized = true
	cur := a.curDebate
	hasPending := cur != nil && a.view.Live && !a.view.Done
	if hasPending {
		a.screen = tui.ScreenSession
		a.refreshSession()
		a.runDebate(cur)
		return a, tea.Batch(a.archiveList.Reload(), a.cacheList.Reload(), a.waitForProgress())
	}
	a.screen = tui.ScreenSession
	a.refreshSession()
	return a, tea.Batch(a.archiveList.Reload(), a.cacheList.Reload(), a.waitForProgress())
}

func (a *App) handleDebateProgress(msg tui.DebateProgressMsg) (tea.Model, tea.Cmd) {
	prevDone := a.view.Done
	a.accumulateEvent(msg.Event)
	a.refreshSession()

	switch msg.Event.Kind {
	case orchestrator.EventTurnEnd:
		if msg.Event.Role == string(orchestrator.RoleJudge) {
			break
		}
		// Interim snapshot for crash recovery; a failed snapshot is not fatal.
		if err := a.persistCache(); err != nil && !a.view.Done {
			a.view.Err = fmt.Errorf("cache persist: %w", err)
			a.refreshSession()
		}
	case orchestrator.EventAborted:
		// An aborted debate stays resumable: snapshot it and keep the marker.
		if err := a.persistCache(); err != nil {
			a.view.Err = fmt.Errorf("cache persist: %w", err)
			a.refreshSession()
		}
	case orchestrator.EventError:
		if err := a.persistCache(); err != nil && !a.view.Done {
			a.view.Err = fmt.Errorf("cache persist: %w", err)
			a.refreshSession()
		}
	}

	if a.screen == tui.ScreenSession {
		done := a.view.Done
		if done && !prevDone {
			return a, tea.Batch(a.archiveList.Reload(), a.cacheList.Reload(), a.waitForProgress())
		}
		return a, a.waitForProgress()
	}
	return a, a.waitForProgress()
}

// persistCache snapshots the live view into the cache; it is the sole cache writer.
func (a *App) persistCache() error {
	if a.curDebate == nil {
		return nil
	}
	cfg := a.curDebate.Config()
	sess := cfg.Session()
	sess.Messages = append([]types.Message(nil), a.view.Messages...)
	sess.Verdict = a.view.Verdict
	if a.view.Err != nil {
		if _, ok := a.view.Err.(*orchestrator.AbortedError); ok {
			sess.Aborted = map[string]string{"reason": a.view.Err.Error()}
		}
	}
	return cache.UpdateCache(a.cacheDir, cfg.SessionID, sess)
}

func (a *App) openArchived(sessionID string) (tea.Model, tea.Cmd) {
	curID := a.view.SessionID
	live := a.view.Live
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

func (a *App) loadCacheIntoSession(sessionID string) (tea.Model, tea.Cmd) {
	if a.isLive() {
		return a, nil
	}
	sess, err := cache.Load(a.cacheDir, sessionID)
	if err != nil {
		return a, nil
	}
	interrupted, _ := cache.WasInterrupted(a.cacheDir, sessionID)
	if !interrupted {
		return a, nil
	}

	var sandbox *tools.Sandbox
	if len(sess.Dirs) > 0 || len(sess.Files) > 0 {
		paths := append([]string{}, sess.Dirs...)
		paths = append(paths, sess.Files...)
		sb, err := tools.NewSandbox(paths)
		if err == nil {
			sandbox = sb
		}
	}

	d := orchestrator.NewResumed(sess, a.engine, sandbox)
	view := tui.SessionViewFromSession(sess, true)

	if !a.initialized {
		return a, a.enterBootstrap(sess.Model, d, view)
	}

	a.view = view
	a.curDebate = d
	a.currentContent.Reset()
	a.currentTools = nil
	a.screen = tui.ScreenSession
	a.refreshSession()
	a.runDebate(d)
	return a, tea.Batch(a.cacheList.Reload(), a.waitForProgress())
}

func (a *App) refreshSession() {
	a.session.SetView(a.view)
}

func (a *App) accumulateEvent(ev orchestrator.Event) {
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
			a.view.CurrentTools = append([]types.ToolCall(nil), a.currentTools...)
		}
	case orchestrator.EventTurnEnd:
		if ev.Role != string(orchestrator.RoleJudge) {
			a.view.Messages = append(a.view.Messages, types.Message{
				Role: ev.Role, Round: ev.Round, Content: a.currentContent.String(),
				ToolCalls: append([]types.ToolCall(nil), a.currentTools...),
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

func (a *App) runDebate(d *orchestrator.Debate) {
	go func() {
		for ev := range d.Events() {
			a.progressCh <- ev
		}
	}()

	go func() {
		debateCtx := context.WithoutCancel(a.ctx)
		sess, err := d.Run(debateCtx)
		if err != nil {
			return
		}
		// The runner owns final state: archive from memory, then drop the cache entry.
		if err := archive.Write(a.archiveDir, sess); err != nil {
			a.progressCh <- orchestrator.Event{Kind: orchestrator.EventError, Text: fmt.Sprintf("archive write: %v", err)}
			return
		}
		if err := cache.Remove(a.cacheDir, sess.SessionID); err != nil {
			a.progressCh <- orchestrator.Event{Kind: orchestrator.EventError, Text: fmt.Sprintf("cache cleanup: %v", err)}
		}
	}()
}

func (a *App) waitForProgress() tea.Cmd {
	sid := a.view.SessionID
	return func() tea.Msg {
		ev, ok := <-a.progressCh
		if !ok {
			return nil
		}
		return tui.DebateProgressMsg{SessionID: sid, Event: ev}
	}
}

func (a *App) isLive() bool {
	return a.view.Live && !a.view.Done
}

func (a *App) hasResumableDebates() bool {
	items := a.cacheList.Items()
	if len(items) == 0 {
		return false
	}
	if !a.isLive() {
		return true
	}
	liveID := a.view.SessionID
	for _, s := range items {
		if s.SessionID != liveID {
			return true
		}
	}
	return false
}

func defaultSides(mode prompts.Mode) [2]types.Side {
	if mode == prompts.ModeVersus {
		return [2]types.Side{
			{ID: "optiona", Label: "Option A", Stance: "Option A"},
			{ID: "optionb", Label: "Option B", Stance: "Option B"},
		}
	}
	return [2]types.Side{
		{ID: "advocate", Label: "Advocate", Stance: "for"},
		{ID: "critic", Label: "Critic", Stance: "against"},
	}
}
