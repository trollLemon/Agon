// Package tui implements the Bubble Tea application: menu, new-debate form,
// session view (live or archived), and archive list (see docs/SPEC.md D8).
package tui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/agentdisc/goagentdisc/internal/archive"
	"github.com/agentdisc/goagentdisc/internal/orchestrator"
	"github.com/agentdisc/goagentdisc/internal/prompts"
	"github.com/agentdisc/goagentdisc/internal/tools"
)

// screen identifies which of the app's four screens is rendered. Session
// view is exclusive: while it is active, nothing else is drawn (D8).
type screen int

const (
	screenMenu screen = iota
	screenForm
	screenBootstrap
	screenSession
	screenArchive
)

// Bootstrapper loads a chat-capable model client, e.g. via
// orchestrator.Bootstrap + orchestrator.NewKronkClient. Tests inject a fake
// one so the TUI never needs a real model. log receives progress/log lines
// as they happen (native library detection/download, model download, model
// load) so a slow first run isn't a silent black box; it may be nil.
type Bootstrapper func(ctx context.Context, modelSource string, log BootLogFunc) (orchestrator.ChatClient, error)

// BootLogFunc receives one bootstrap log line. It mirrors kronk.Logger's
// msg+args shape (minus the context parameter, which the tui package has
// no reason to depend on kronk to spell) so defaultBootstrapper can adapt
// it directly.
type BootLogFunc func(msg string, args ...any)

func defaultBootstrapper(ctx context.Context, modelSource string, log BootLogFunc) (orchestrator.ChatClient, error) {
	logger := func(_ context.Context, msg string, args ...any) {
		if log != nil {
			log(msg, args...)
		}
	}
	krn, err := orchestrator.Bootstrap(ctx, modelSource, logger)
	if err != nil {
		return nil, err
	}
	return orchestrator.NewKronkClient(krn), nil
}

// Options configures a new App.
type Options struct {
	ArchiveDir   string
	DefaultModel string
	Bootstrap    Bootstrapper // nil selects the real kronk-backed bootstrapper
}

// App is the root Bubble Tea model.
type App struct {
	screen        screen
	width, height int

	archiveDir   string
	defaultModel string
	bootstrap    Bootstrapper

	client        orchestrator.ChatClient
	bootstrapping bool
	bootstrapErr  error
	bootLog       *bootLog
	pending       *pendingDebate

	menu        menuModel
	form        formModel
	bootScreen  bootstrapModel
	archiveList archiveListModel
	session     sessionModel

	live *liveDebate // at most one at a time (D9)
}

// pendingDebate holds a validated form submission while the model client is
// still being bootstrapped.
type pendingDebate struct {
	topic, context string
	mode           prompts.Mode
	tone           prompts.Tone
	rounds         int
	model          string
}

// New creates the root App model.
func New(opts Options) *App {
	if opts.Bootstrap == nil {
		opts.Bootstrap = defaultBootstrapper
	}
	if opts.DefaultModel == "" {
		opts.DefaultModel = orchestrator.DefaultModel
	}
	return &App{
		screen:       screenMenu,
		archiveDir:   opts.ArchiveDir,
		defaultModel: opts.DefaultModel,
		bootstrap:    opts.Bootstrap,
		menu:         newMenuModel(),
		form:         newFormModel(opts.DefaultModel),
		bootScreen:   newBootstrapModel(),
		archiveList:  newArchiveListModel(opts.ArchiveDir),
		session:      newSessionModel(),
	}
}

// Run starts the Bubble Tea program and blocks until the user quits.
func Run(opts Options) error {
	p := tea.NewProgram(New(opts), tea.WithAltScreen())
	_, err := p.Run()
	return err
}

func (a *App) Init() tea.Cmd {
	return a.archiveList.reload()
}

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width, a.height = msg.Width, msg.Height
		a.session.setSize(msg.Width, msg.Height)
		a.bootScreen.setSize(msg.Width, msg.Height)
		return a, nil

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			if a.live != nil {
				a.live.debate.Abort("app quit")
			}
			return a, tea.Quit
		}
		return a.handleKey(msg)

	case switchScreenMsg:
		a.screen = msg.screen
		if msg.screen == screenMenu && a.live.isDone() {
			a.live = nil // free the one-at-a-time slot once it's archived
		}
		if msg.screen == screenArchive {
			return a, a.archiveList.reload()
		}
		return a, nil

	case startDebateMsg:
		return a.startDebate(msg)

	case bootstrapDoneMsg:
		a.bootstrapping = false
		if msg.err != nil {
			a.bootstrapErr = msg.err
			a.bootScreen.setError(msg.err)
			return a, nil
		}
		a.client = msg.client
		return a.launchPendingDebate()

	case openArchivedMsg:
		return a.openArchived(msg.sessionID)

	case liveUpdateMsg:
		return a.handleLiveUpdate(msg)

	case bootLogTickMsg:
		if a.bootstrapping {
			a.bootScreen.refresh()
			return a, waitForBootLog()
		}
		return a, nil

	case archiveListLoadedMsg:
		a.archiveList.setItems(msg.items)
		return a, nil
	}
	return a, nil
}

func (a *App) View() string {
	switch a.screen {
	case screenSession:
		return a.session.View()
	case screenBootstrap:
		return a.bootScreen.View()
	case screenForm:
		return a.form.View()
	case screenArchive:
		return a.archiveList.View()
	default:
		return a.menu.View(a.live)
	}
}

func (a *App) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch a.screen {
	case screenMenu:
		return a.menu.update(msg, a)
	case screenForm:
		var cmd tea.Cmd
		a.form, cmd = a.form.update(msg)
		return a, cmd
	case screenBootstrap:
		return a.bootScreen.handleKey(msg, a)
	case screenSession:
		return a.session.update(msg, a)
	case screenArchive:
		return a.archiveList.update(msg, a)
	}
	return a, nil
}

// startDebate handles a validated form submission: it detects a repo root
// from the starting context, bootstraps the model client if needed, and
// either launches immediately or waits for bootstrap to finish.
func (a *App) startDebate(msg startDebateMsg) (tea.Model, tea.Cmd) {
	if a.live != nil && !a.live.isDone() {
		a.form.errMsg = "a debate is already running; finish or abort it first"
		return a, nil
	}
	a.pending = &pendingDebate{
		topic: msg.topic, context: msg.context, mode: msg.mode,
		tone: msg.tone, rounds: msg.rounds, model: msg.model,
	}
	if a.client != nil {
		return a.launchPendingDebate()
	}

	a.bootstrapping = true
	a.bootstrapErr = nil
	bl := newBootLog()
	a.bootLog = bl
	a.bootScreen.start(bl)
	a.screen = screenBootstrap
	modelSource := msg.model
	bootstrap := a.bootstrap
	bootstrapCmd := func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		client, err := bootstrap(ctx, modelSource, bl.append)
		return bootstrapDoneMsg{client: client, err: err}
	}
	return a, tea.Batch(bootstrapCmd, waitForBootLog())
}

func (a *App) launchPendingDebate() (tea.Model, tea.Cmd) {
	p := a.pending
	a.pending = nil
	if p == nil {
		return a, nil
	}

	var sandbox *tools.Sandbox
	var sandboxDirs []string
	if paths, err := tools.DetectPaths(p.context); err == nil && len(paths) > 0 {
		if sb, err := tools.NewSandbox(paths); err == nil {
			sandbox = sb
			sandboxDirs = paths
		}
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
		SandboxDirs:     sandboxDirs,
		CreatedAt:       now,
	}

	a.live = startLiveDebate(cfg, a.client, sandbox, a.archiveDir)
	a.session.showLive(a.live)
	a.screen = screenSession
	return a, waitForLiveUpdate(a.live)
}

func (a *App) openArchived(sessionID string) (tea.Model, tea.Cmd) {
	if a.live != nil && a.live.sessionID == sessionID {
		a.session.showLive(a.live)
		a.screen = screenSession
		return a, waitForLiveUpdate(a.live)
	}
	sess, err := archive.Load(a.archiveDir, sessionID)
	if err != nil {
		return a, nil
	}
	a.session.showArchived(sess)
	a.screen = screenSession
	return a, nil
}

func (a *App) handleLiveUpdate(msg liveUpdateMsg) (tea.Model, tea.Cmd) {
	if a.live == nil || a.live.sessionID != msg.sessionID {
		return a, nil
	}
	if a.screen == screenSession && a.session.sessionID() == msg.sessionID {
		a.session.refreshLive(a.live)
	}
	if a.live.isDone() {
		return a, a.archiveList.reload()
	}
	return a, waitForLiveUpdate(a.live)
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
