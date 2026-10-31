package tui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"fastgen/internal/runner"
)

func TestSideBySideFitsPane(t *testing.T) {
	for _, width := range []int{18, 24, 60, 102} {
		out := sideBySide("562\n", "563\n", width)
		for _, line := range strings.Split(out, "\n") {
			if got := lipgloss.Width(line); got > width {
				t.Fatalf("width %d: line is %d cells wide: %q", width, got, line)
			}
		}
		if width >= 40 && (!strings.Contains(out, "562") || !strings.Contains(out, "563")) {
			t.Fatalf("width %d: missing outputs:\n%s", width, out)
		}
	}
}

func TestFailureViewRendersAllPanes(t *testing.T) {
	m := New(context.Background(), Options{Tests: 10, Seed: 1})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 110, Height: 30})
	m = updated.(Model)
	m.state = stateFailed
	m.hasFailure = true
	m.firstFailure = runner.Result{
		Index:    2,
		Seed:     3,
		Verdict:  runner.WA,
		Note:     "outputs differ",
		Input:    []byte("10\n"),
		Expected: []byte("562\n"),
		Got:      []byte("563\n"),
	}
	m.refreshFailure()

	if got := lipgloss.Width(m.panes[0].View()); got > m.paneInnerWidth() {
		t.Fatalf("viewport width %d exceeds pane %d", got, m.paneInnerWidth())
	}

	for pane, want := range map[int]string{
		0: "10",
		1: "563",
		2: "outputs differ",
	} {
		m.pane = pane
		if !strings.Contains(m.View(), want) {
			t.Fatalf("pane %d view missing %q:\n%s", pane, want, m.View())
		}
	}
}

func TestRunningViewShowsAndClearsInput(t *testing.T) {
	m := New(context.Background(), Options{Tests: 100, Seed: 1})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 110, Height: 30})
	m = updated.(Model)
	m.state = stateRunning

	m.applyTest(runner.Result{
		Index:   4,
		Seed:    5,
		Verdict: runner.AC,
		Input:   []byte("3\n7 8 9\n"),
	})
	view := m.View()
	if !strings.Contains(view, "7 8 9") {
		t.Fatalf("running view should show generated input:\n%s", view)
	}

	m.applyStarted(5, 6)
	view = m.View()
	if strings.Contains(view, "7 8 9") {
		t.Fatalf("input should be cleared when the next test starts:\n%s", view)
	}
	if !strings.Contains(view, "waiting for the generator") {
		t.Fatalf("expected waiting message after start:\n%s", view)
	}
}
