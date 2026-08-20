// Package app is the root Bubble Tea model: it owns screen orchestration
// (menu, new-debate form, bootstrap, live/archived session view, archive
// list) and wires the tui package's screen components to the orchestrator,
// archive, sandbox, and model engine.
package app

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/trollLemon/agon/internal/archive"
	"github.com/trollLemon/agon/internal/orchestrator"
	"github.com/trollLemon/agon/internal/prompts"
	"github.com/trollLemon/agon/internal/tools"
	"github.com/trollLemon/agon/internal/tui"
)

// AppState holds the current state of the app:
// - initializing: setting up required libs and agents
// - initialized: ready for a debate.
type AppState int

const (
	Initializing AppState = iota
	Initialized
)

// Options configures a new App.
type Options struct {
	ArchiveDir   string
	DefaultModel string
}

// App is the root Bubble Tea model.
type App struct {
	screen        tui.Screen
	width, height int

	archiveDir   string
	defaultModel string

	engine         orchestrator.Engine
	state          AppState
	initializedErr error
	bootLog        *tui.BootLog
	pending        *pendingDebate

	menu        tui.MenuModel
	form        tui.FormModel
	bootScreen  tui.BootstrapModel
	archiveList tui.ArchiveListModel
	session     tui.SessionModel

	live *tui.LiveDebate
}

// pendingDebate holds a validated form submission while the model is still
// being initialized.
type pendingDebate struct {
	topic, context string
	mode           prompts.Mode
	tone           prompts.Tone
	rounds         int
	model          string
	sandbox        *tools.Sandbox
	sandboxDirs    []string
	sandboxFiles   []string
}

// New creates the root App model. engine is the (uninitialized) model
// backend the app drives.
func New(opts Options, engine orchestrator.Engine) *App {
	if opts.DefaultModel == "" {
		opts.DefaultModel = orchestrator.DefaultModel
	}
	return &App{
		screen:       tui.ScreenMenu,
		archiveDir:   opts.ArchiveDir,
		defaultModel: opts.DefaultModel,
		engine:       engine,
		menu:         tui.NewMenuModel(),
		form:         tui.NewFormModel(opts.DefaultModel),
		bootScreen:   tui.NewBootstrapModel(),
		archiveList:  tui.NewArchiveListModel(opts.ArchiveDir),
		session:      tui.NewSessionModel(),
	}
}

// Run starts the Bubble Tea program and blocks until the user quits.
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
			if a.live != nil {
				a.live.Abort("app quit")
			}
			return a, tea.Quit
		}
		return a.handleKey(msg)

	case tui.SwitchScreenMsg:
		a.screen = msg.Screen
		switch msg.Screen {
		case tui.ScreenMenu:
			if a.live.IsDone() {
				a.live = nil // free the one-at-a-time slot once it's archived
			}
		case tui.ScreenForm:
			a.form = tui.NewFormModel(a.defaultModel)
		case tui.ScreenArchive:
			return a, a.archiveList.Reload()
		case tui.ScreenSession:
			if a.live != nil {
				a.session.ShowLive(a.live)
				return a, tui.WaitForLiveUpdate(a.live)
			}
		}
		return a, nil

	case tui.StartDebateMsg:
		return a.startDebate(msg)

	case tui.BootstrapDoneMsg:
		if msg.Err != nil {
			a.initializedErr = msg.Err
			a.bootScreen.SetError(msg.Err)
			return a, nil
		}

		a.state = Initialized
		return a.launchPendingDebate()

	case tui.OpenArchivedMsg:
		return a.openArchived(msg.SessionID)

	case tui.LiveUpdateMsg:
		return a.handleLiveUpdate(msg)

	case tui.BootLogTickMsg:
		if a.state == Initializing {
			a.bootScreen.Refresh()
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
		return a.archiveList.View()
	default:
		return a.menu.View(a.live)
	}
}

func (a *App) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch a.screen {
	case tui.ScreenMenu:
		a.menu, cmd = a.menu.Update(msg, a.live)
	case tui.ScreenForm:
		a.form, cmd = a.form.Update(msg)
	case tui.ScreenBootstrap:
		a.bootScreen, cmd = a.bootScreen.HandleKey(msg)
	case tui.ScreenSession:
		a.session, cmd = a.session.Update(msg, a.live)
	case tui.ScreenArchive:
		a.archiveList, cmd = a.archiveList.Update(msg)
	}
	return a, cmd
}

// startDebate handles a validated form submission: it builds a read-only
// sandbox from the user-listed paths, bootstraps the model client if needed,
// and either launches immediately or waits for bootstrap to finish.
func (a *App) startDebate(msg tui.StartDebateMsg) (tea.Model, tea.Cmd) {
	if a.live != nil && !a.live.IsDone() {
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

	a.pending = &pendingDebate{
		topic: msg.Topic, context: msg.Context, mode: msg.Mode,
		tone: msg.Tone, rounds: msg.Rounds, model: msg.Model,
		sandbox: sandbox, sandboxDirs: sandboxDirs, sandboxFiles: sandboxFiles,
	}
	if a.state == Initialized {
		return a.launchPendingDebate()
	}

	a.initializedErr = nil
	bl := tui.NewBootLog()
	a.bootLog = bl
	a.bootScreen.Start(bl)
	a.screen = tui.ScreenBootstrap
	modelSource := msg.Model
	engine := a.engine
	bootstrapCmd := func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		err := engine.Initialize(ctx, modelSource, bl.Append)
		return tui.BootstrapDoneMsg{Err: err}
	}
	return a, tea.Batch(bootstrapCmd, tui.WaitForBootLog())
}

func (a *App) launchPendingDebate() (tea.Model, tea.Cmd) {
	p := a.pending
	a.pending = nil
	if p == nil {
		return a, nil
	}

	now := time.Now()
	cfg := orchestrator.Config{
		SessionID:       archive.NewSessionID(p.topic, now),
		Title:           archive.SummarizeTitle(p.topic),
		Topic:           p.topic,
		StartingContext: p.context,
		Mode:            p.mode,
		Tone:            p.tone,
		Rounds:          p.rounds,
		Sides:           defaultSides(p.mode),
		Model:           p.model,
		SandboxDirs:     p.sandboxDirs,
		SandboxFiles:    p.sandboxFiles,
		CreatedAt:       now,
	}

	a.live = tui.StartLiveDebate(cfg, a.engine, p.sandbox, a.archiveDir)
	a.session.ShowLive(a.live)
	a.screen = tui.ScreenSession
	return a, tui.WaitForLiveUpdate(a.live)
}

func (a *App) openArchived(sessionID string) (tea.Model, tea.Cmd) {
	if a.live != nil && a.live.SessionID() == sessionID {
		a.session.ShowLive(a.live)
		a.screen = tui.ScreenSession
		return a, tui.WaitForLiveUpdate(a.live)
	}
	sess, err := archive.Load(a.archiveDir, sessionID)
	if err != nil {
		return a, nil
	}
	a.session.ShowArchived(sess)
	a.screen = tui.ScreenSession
	return a, nil
}

func (a *App) handleLiveUpdate(msg tui.LiveUpdateMsg) (tea.Model, tea.Cmd) {
	if a.live == nil || a.live.SessionID() != msg.SessionID {
		return a, nil
	}
	if a.screen == tui.ScreenSession && a.session.SessionID() == msg.SessionID {
		a.session.RefreshLive(a.live)
	}
	if a.live.IsDone() {
		return a, a.archiveList.Reload()
	}
	return a, tui.WaitForLiveUpdate(a.live)
}

// defaultSides returns the two sides for a mode. The current form has no
// side-naming fields (D8); versus debates rely on the topic text itself to
// name the two options.
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
