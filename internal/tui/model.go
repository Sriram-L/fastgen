package tui

import (
	"context"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"fastgen/internal/compile"
	"fastgen/internal/runner"
	"fastgen/internal/store"
)

type state int

const (
	stateCompiling state = iota
	stateRunning
	statePassed
	stateFailed
	stateError
)

// Options configures the TUI.
type Options struct {
	Solution  string
	Generator string
	Brute     string
	Checker   string

	Tests     int
	Seed      int64
	Timeout   time.Duration
	Exact     bool
	KeepGoing bool
	Jobs      int
	MaxOutput int
	SaveDir   string

	Compile compile.Config
}

// Model is the Bubble Tea model for fastgen.
type Model struct {
	ctx    context.Context
	opts   Options
	cfg    compile.Config
	state  state
	width  int
	height int

	spinner spinner.Model
	bar     progress.Model

	workCtx    context.Context
	workCancel context.CancelFunc

	bins        compile.Binaries
	compileErr  error
	compileTime time.Duration
	errPane     viewport.Model

	events chan runner.Event
	runCtx context.Context
	cancel context.CancelFunc

	total        int
	done         int
	counts       map[runner.Verdict]int
	last         runner.Result
	hasLast      bool
	hasRunning   bool
	runningIndex int
	runningSeed  int64

	firstFailure  runner.Result
	hasFailure    bool
	extraFailures int

	sumSol      time.Duration
	sumBrute    time.Duration
	maxSol      time.Duration
	maxBrute    time.Duration
	sampleCount int

	started  time.Time
	finished runner.Finished

	pane       int
	panes      []viewport.Model
	rerunning  bool
	saveStatus string
}

// New creates a model in the compiling state.
func New(ctx context.Context, opts Options) Model {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = accentStyle

	bar := progress.New(progress.WithDefaultGradient())
	bar.Width = 40

	workCtx, cancel := context.WithCancel(ctx)

	m := Model{
		ctx:        ctx,
		opts:       opts,
		cfg:        opts.Compile,
		state:      stateCompiling,
		spinner:    sp,
		bar:        bar,
		workCtx:    workCtx,
		workCancel: cancel,
		counts:     map[runner.Verdict]int{},
		total:      opts.Tests,
		panes: []viewport.Model{
			viewport.New(80, 12),
			viewport.New(80, 12),
			viewport.New(80, 12),
		},
		errPane: viewport.New(80, 12),
	}
	return m
}

// Init starts compilation and the UI tick loop.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.compileCmd(), m.spinner.Tick, tick())
}

func (m Model) compileCmd() tea.Cmd {
	cfg := m.cfg
	targets := compile.Targets{
		Solution:  m.opts.Solution,
		Generator: m.opts.Generator,
		Brute:     m.opts.Brute,
	}
	ctx := m.workCtx
	return func() tea.Msg {
		start := time.Now()
		bins, err := compile.Build(ctx, cfg, targets)
		return CompileDone{Bins: bins, Err: err, Dur: time.Since(start)}
	}
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.layout()
		return m, nil

	case CompileDone:
		m.compileTime = msg.Dur
		if msg.Err != nil {
			m.state = stateError
			m.compileErr = msg.Err
			m.errPane.SetContent(msg.Err.Error())
			return m, nil
		}
		m.bins = msg.Bins
		m.state = stateRunning
		m.started = time.Now()
		m.runCtx, m.cancel = context.WithCancel(m.workCtx)
		m.events = make(chan runner.Event, 1024)
		cfg := m.runnerConfig()
		go runner.Run(m.runCtx, cfg, m.events)
		return m, drainEvents(m.events)

	case runnerBatch:
		var cmds []tea.Cmd
		finished := false
		for _, ev := range msg {
			switch e := ev.(type) {
			case runner.TestStarted:
				m.applyStarted(e.Index, e.Seed)
			case runner.TestDone:
				if cmd := m.applyTest(e.Result); cmd != nil {
					cmds = append(cmds, cmd)
				}
			case runner.Finished:
				m.applyFinished(e)
				finished = true
			case runner.Fatal:
				m.state = stateError
				m.compileErr = e.Err
				m.errPane.SetContent(e.Err.Error())
				finished = true
			}
		}
		if !finished && m.state != stateError {
			cmds = append(cmds, drainEvents(m.events))
		}
		return m, tea.Batch(cmds...)

	case spinner.TickMsg:
		if m.state == stateCompiling || m.state == stateRunning {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
		return m, nil

	case TickMsg:
		if m.state == stateCompiling || m.state == stateRunning {
			return m, tick()
		}
		return m, nil

	case SavedMsg:
		if msg.Err != nil {
			m.saveStatus = "save failed: " + msg.Err.Error()
		} else {
			m.saveStatus = "saved " + joinPaths(msg.Paths)
		}
		return m, nil

	case RerunDoneMsg:
		m.rerunning = false
		if msg.Result.Canceled {
			m.saveStatus = "rerun canceled"
			return m, nil
		}
		if m.hasFailure {
			m.firstFailure = msg.Result
			if msg.Result.Failed() {
				m.saveStatus = "rerun still failing: " + string(msg.Result.Verdict)
				if m.opts.SaveDir != "" {
					return m, m.saveCmd()
				}
			} else {
				m.saveStatus = "rerun passed"
			}
			m.refreshFailure()
		}
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if key == "ctrl+c" {
		m.stop()
		return m, tea.Quit
	}

	switch m.state {
	case stateFailed:
		switch key {
		case "q":
			m.stop()
			return m, tea.Quit
		case "tab", "right", "l":
			m.pane = (m.pane + 1) % len(m.panes)
			return m, nil
		case "shift+tab", "left", "h":
			m.pane = (m.pane - 1 + len(m.panes)) % len(m.panes)
			return m, nil
		case "s":
			return m, m.saveCmd()
		case "r":
			if m.rerunning {
				return m, nil
			}
			m.rerunning = true
			m.saveStatus = "rerunning pinned failure..."
			return m, m.rerunCmd()
		default:
			var cmd tea.Cmd
			m.panes[m.pane], cmd = m.panes[m.pane].Update(msg)
			return m, cmd
		}
	case stateError:
		if key == "q" || key == "esc" {
			m.stop()
			return m, tea.Quit
		}
		var cmd tea.Cmd
		m.errPane, cmd = m.errPane.Update(msg)
		return m, cmd
	default:
		if key == "q" || key == "esc" {
			m.stop()
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m *Model) stop() {
	if m.cancel != nil {
		m.cancel()
	}
	if m.workCancel != nil {
		m.workCancel()
	}
}

func (m *Model) applyStarted(index int, seed int64) {
	m.hasRunning = true
	m.runningIndex = index
	m.runningSeed = seed
	m.hasLast = false
	m.last = runner.Result{}
}

func (m *Model) applyTest(r runner.Result) tea.Cmd {
	m.hasRunning = false
	m.done++
	m.counts[r.Verdict]++
	m.last = r
	m.hasLast = true

	if r.Verdict == runner.AC {
		m.sampleCount++
		m.sumSol += r.SolutionTime
		m.sumBrute += r.BruteTime
		if r.SolutionTime > m.maxSol {
			m.maxSol = r.SolutionTime
		}
		if r.BruteTime > m.maxBrute {
			m.maxBrute = r.BruteTime
		}
	}

	if !r.Failed() {
		return nil
	}

	var cmd tea.Cmd
	if m.opts.SaveDir != "" {
		cmd = m.saveCmdFor(r)
	}

	if m.hasFailure {
		m.extraFailures++
		return cmd
	}
	m.firstFailure = r
	m.hasFailure = true
	m.pane = 0
	m.refreshFailure()
	return cmd
}

func (m *Model) applyFinished(f runner.Finished) {
	m.finished = f
	m.hasRunning = false
	if f.Total > m.total {
		m.total = f.Total
	}
	if f.Done > m.done {
		m.done = f.Done
	}
	if f.Aborted && !m.hasFailure && m.ctx.Err() == nil {
		m.state = stateError
		m.errPane.SetContent("run aborted")
		return
	}
	if m.hasFailure {
		m.state = stateFailed
		return
	}
	m.state = statePassed
}

func (m Model) runnerConfig() runner.Config {
	return runner.Config{
		Generator: m.bins.Generator,
		Solution:  m.bins.Solution,
		Brute:     m.bins.Brute,
		Checker:   m.opts.Checker,
		StartSeed: m.opts.Seed,
		Tests:     m.opts.Tests,
		Timeout:   m.opts.Timeout,
		Exact:     m.opts.Exact,
		KeepGoing: m.opts.KeepGoing,
		Jobs:      m.opts.Jobs,
		MaxOutput: m.opts.MaxOutput,
	}
}

func (m Model) saveCmd() tea.Cmd {
	return m.saveCmdFor(m.firstFailure)
}

func (m Model) saveCmdFor(r runner.Result) tea.Cmd {
	dir := m.opts.SaveDir
	return func() tea.Msg {
		paths, err := store.Save(dir, r)
		return SavedMsg{Paths: paths, Err: err}
	}
}

func (m Model) rerunCmd() tea.Cmd {
	cfg := m.runnerConfig()
	prev := m.firstFailure
	ctx := m.runCtx
	if ctx == nil {
		ctx = m.ctx
	}
	return func() tea.Msg {
		return RerunDoneMsg{Result: runner.Repeat(ctx, cfg, prev)}
	}
}

func (m *Model) layout() {
	if m.width <= 0 || m.height <= 0 {
		return
	}
	w := m.contentWidth()
	m.bar.Width = clamp(w-8, 16, 72)

	paneW := m.paneInnerWidth()
	h := clamp(m.height-9, 5, 40)
	m.errPane.Width = m.contentWidth() - 2
	m.errPane.Height = h
	for i := range m.panes {
		m.panes[i].Width = paneW
		m.panes[i].Height = h
	}
	if m.hasFailure {
		m.refreshFailure()
	}
}

func (m Model) paneWidth() int {
	return clamp(m.width-8, 20, 160)
}

func (m Model) paneInnerWidth() int {
	return m.paneWidth() - 2
}

func (m Model) contentWidth() int {
	return clamp(m.width-4, 24, 160)
}

func (m *Model) refreshFailure() {
	if !m.hasFailure {
		return
	}
	r := m.firstFailure
	contents := []string{
		inputText(r),
		sideBySide(string(r.Expected), string(r.Got), m.paneInnerWidth()),
		detailsText(r),
	}
	for i := range m.panes {
		if i < len(contents) {
			m.panes[i].SetContent(contents[i])
		}
	}
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func joinPaths(paths []string) string {
	out := ""
	for i, p := range paths {
		if i > 0 {
			out += ", "
		}
		out += p
	}
	return out
}
