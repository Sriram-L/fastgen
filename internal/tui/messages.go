package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"fastgen/internal/compile"
	"fastgen/internal/runner"
)

// CompileDone carries the result of the compile stage.
type CompileDone struct {
	Bins compile.Binaries
	Err  error
	Dur  time.Duration
}

// runnerBatch groups several runner events into one TUI update.
type runnerBatch []runner.Event

// TickMsg drives throttled redraws.
type TickMsg time.Time

// SavedMsg reports the result of saving a failing testcase.
type SavedMsg struct {
	Paths []string
	Err   error
}

// RerunDoneMsg carries the outcome of re-running the pinned failure.
type RerunDoneMsg struct {
	Result runner.Result
}

func tick() tea.Cmd {
	return tea.Tick(time.Second/30, func(t time.Time) tea.Msg {
		return TickMsg(t)
	})
}

func drainEvents(ch <-chan runner.Event) tea.Cmd {
	return func() tea.Msg {
		first, ok := <-ch
		if !ok {
			return nil
		}
		batch := runnerBatch{first}
		for len(batch) < 512 {
			select {
			case e, ok := <-ch:
				if !ok {
					return batch
				}
				batch = append(batch, e)
			default:
				return batch
			}
		}
		return batch
	}
}
