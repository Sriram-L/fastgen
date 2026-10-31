package tui

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"fastgen/internal/runner"
)

// View implements tea.Model.
func (m Model) View() string {
	switch m.state {
	case stateCompiling:
		return m.compileView()
	case stateRunning:
		return m.runningView()
	case statePassed:
		return m.summaryView()
	case stateFailed:
		return m.failureView()
	case stateError:
		return m.errorView()
	}
	return ""
}

func (m Model) compileView() string {
	body := lipgloss.JoinVertical(
		lipgloss.Left,
		m.spinner.View()+" Compiling sources...",
		dimStyle.Render(fmt.Sprintf("%s  %s  %s",
			filepath.Base(m.opts.Solution),
			filepath.Base(m.opts.Generator),
			filepath.Base(m.opts.Brute),
		)),
		dimStyle.Render("unchanged files are reused from the content cache"),
	)
	return boxStyle.Width(m.contentWidth()).Render(body)
}

func (m Model) errorView() string {
	title := badStyle.Render("✗ compilation failed")
	panel := errorBoxStyle.Width(m.contentWidth()).Render(m.errPane.View())
	footer := dimStyle.Render("↑/↓ scroll · q quit")
	return lipgloss.JoinVertical(lipgloss.Left, title, panel, footer)
}

func (m Model) runningView() string {
	pct := 0.0
	if m.total > 0 {
		pct = float64(m.done) / float64(m.total)
	}
	if pct > 1 {
		pct = 1
	}

	header := fmt.Sprintf("%s Running   %d / %d", m.spinner.View(), m.done, m.total)
	counts := fmt.Sprintf("AC %d   WA %d   RE %d   TLE %d   GEN %d",
		m.counts[runner.AC], m.counts[runner.WA], m.counts[runner.RE],
		m.counts[runner.TLE], m.counts[runner.GENFAIL])

	timing := "Avg: sol -   |   brute -   |   Max: -"
	if m.sampleCount > 0 {
		avgSol := m.sumSol / time.Duration(m.sampleCount)
		avgBrute := m.sumBrute / time.Duration(m.sampleCount)
		timing = fmt.Sprintf("Avg: sol %s   |   brute %s   |   Max: %s",
			fmtDur(avgSol), fmtDur(avgBrute), fmtDur(max(m.maxSol, m.maxBrute)))
	}

	current := m.currentLine()

	parts := []string{
		header,
		m.bar.ViewAs(pct),
		"",
		counts,
		timing,
		"",
		"Current test",
		current,
		"",
		dimStyle.Render("Input"),
	}
	parts = append(parts, m.inputPreview(m.inputPreviewCount())...)

	body := lipgloss.JoinVertical(lipgloss.Left, parts...)
	return boxStyle.Width(m.contentWidth()).Render(body)
}

func (m Model) inputPreviewCount() int {
	n := m.height - 16
	if n < 1 {
		n = 1
	}
	if n > 3 {
		n = 3
	}
	return n
}

func (m Model) inputPreview(count int) []string {
	inner := m.contentWidth() - 2
	if inner < 1 {
		inner = 1
	}

	var lines []string
	switch {
	case m.hasLast:
		lines = inputLines(m.last.Input)
		more := m.last.InputTruncated || len(lines) > count
		if len(lines) > count {
			lines = lines[:count]
		}
		for i := range lines {
			lines[i] = truncate(strings.ReplaceAll(lines[i], "\t", "    "), inner)
		}
		if more && len(lines) > 0 {
			lines[len(lines)-1] = markMore(lines[len(lines)-1], inner)
		}
	case m.hasRunning:
		lines = []string{dimStyle.Render(fmt.Sprintf(
			"#%d seed %d: waiting for the generator...", m.runningIndex, m.runningSeed))}
	default:
		lines = []string{dimStyle.Render("waiting for first test...")}
	}

	for len(lines) < count {
		lines = append(lines, "")
	}
	return lines
}

func inputLines(b []byte) []string {
	if len(b) == 0 {
		return []string{"(empty input)"}
	}
	s := strings.ReplaceAll(string(b), "\r\n", "\n")
	s = strings.TrimRight(s, "\n")
	if s == "" {
		return []string{""}
	}
	return strings.Split(s, "\n")
}

func markMore(s string, width int) string {
	s = strings.TrimRight(s, " ")
	r := []rune(s)
	if len(r) >= width {
		if width <= 1 {
			return "…"
		}
		return string(r[:width-1]) + "…"
	}
	if len(r)+2 <= width {
		return s + " …"
	}
	return s + "…"
}

func (m Model) currentLine() string {
	if !m.hasLast {
		if m.hasRunning {
			return fmt.Sprintf("#%d  seed %d   %s",
				m.runningIndex, m.runningSeed, dimStyle.Render("running..."))
		}
		return dimStyle.Render("waiting for first test...")
	}
	r := m.last
	v := verdictStyle(string(r.Verdict)).Render(fmt.Sprintf("%-8s", r.Verdict))
	line := fmt.Sprintf("#%d  seed %d   %s %s / %s",
		r.Index, r.Seed, v, fmtDur(r.SolutionTime), fmtDur(r.BruteTime))
	if r.Failed() {
		line += "   " + badStyle.Render("failure: "+string(r.Verdict))
	}
	if m.extraFailures > 0 {
		line += dimStyle.Render(fmt.Sprintf("   (+%d more)", m.extraFailures))
	}
	return line
}

func (m Model) summaryView() string {
	f := m.finished
	wall := m.wall()
	title := okStyle.Render("✓ PASSED")
	if f.Done == 0 {
		title = warnStyle.Render("no tests ran")
	}
	body := lipgloss.JoinVertical(
		lipgloss.Left,
		title+"   "+fmt.Sprintf("%d / %d tests", f.Done, m.total),
		"",
		fmt.Sprintf("AC %d   WA %d   RE %d   TLE %d   GEN %d",
			m.counts[runner.AC], m.counts[runner.WA], m.counts[runner.RE],
			m.counts[runner.TLE], m.counts[runner.GENFAIL]),
		fmt.Sprintf("Wall %s   |   compile %s", fmtDur(wall), fmtDur(m.compileTime)),
		fmt.Sprintf("Avg: sol %s   |   brute %s   |   Max: %s",
			fmtDur(avg(m.sumSol, m.sampleCount)), fmtDur(avg(m.sumBrute, m.sampleCount)),
			fmtDur(max(m.maxSol, m.maxBrute))),
		"",
		dimStyle.Render("q quit"),
	)
	return boxStyle.Width(m.contentWidth()).Render(body)
}

func (m Model) failureView() string {
	r := m.firstFailure

	line := fmt.Sprintf("%s  %s   test #%d   seed %d",
		badStyle.Render("✗ "+string(r.Verdict)), dimStyle.Render("first failing case"), r.Index, r.Seed)
	if m.rerunning {
		line += "   " + m.spinner.View() + dimStyle.Render(" rerunning")
	}

	tabs := make([]string, 0, len(m.panes)*2)
	names := []string{"Input", "Expected vs Got", "Details"}
	for i, name := range names {
		if i == m.pane {
			tabs = append(tabs, activeTabStyle.Render(name))
		} else {
			tabs = append(tabs, inactiveTabStyle.Render(name))
		}
	}
	tabBar := lipgloss.JoinHorizontal(lipgloss.Left, tabs...)

	panel := paneStyle.Width(m.paneWidth()).Render(m.panes[m.pane].View())

	status := " "
	if m.saveStatus != "" {
		status = dimStyle.Render(m.saveStatus)
	}
	footer := "tab/←→ switch · ↑/↓ scroll · s save · r rerun · q quit"
	if m.extraFailures > 0 {
		footer += fmt.Sprintf(" · %d more failures (first pinned)", m.extraFailures)
	}

	return lipgloss.JoinVertical(
		lipgloss.Left,
		line,
		tabBar,
		panel,
		dimStyle.Render(footer),
		status,
	)
}

func (m Model) wall() time.Duration {
	if m.finished.Wall > 0 {
		return m.finished.Wall
	}
	if m.started.IsZero() {
		return 0
	}
	return time.Since(m.started)
}

func fmtDur(d time.Duration) string {
	if d <= 0 {
		return "-"
	}
	if d < time.Millisecond {
		return fmt.Sprintf("%dµs", d.Microseconds())
	}
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.2fs", d.Seconds())
}

func avg(sum time.Duration, n int) time.Duration {
	if n <= 0 {
		return 0
	}
	return sum / time.Duration(n)
}

func max(a, b time.Duration) time.Duration {
	if a > b {
		return a
	}
	return b
}

func inputText(r runner.Result) string {
	if len(r.Input) == 0 {
		if r.Verdict == runner.GENFAIL {
			return "(generator produced no input)"
		}
		return "(empty input)"
	}
	s := string(r.Input)
	if len(s) > 200_000 {
		return s[:200_000] + "\n... input truncated ..."
	}
	return s
}

func detailsText(r runner.Result) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Verdict: %s\n", r.Verdict)
	if r.Note != "" {
		fmt.Fprintf(&b, "Note:    %s\n", r.Note)
	}
	fmt.Fprintf(&b, "Test:    #%d   seed %d\n", r.Index, r.Seed)
	fmt.Fprintf(&b, "\nSolution    exit %-4d %s\n", r.SolutionExit, fmtDur(r.SolutionTime))
	fmt.Fprintf(&b, "Brute       exit %-4d %s\n", r.BruteExit, fmtDur(r.BruteTime))
	fmt.Fprintf(&b, "Generator   exit %-4d %s\n", r.GeneratorExit, fmtDur(r.GeneratorTime))

	if len(r.SolutionStderr) > 0 {
		fmt.Fprintf(&b, "\nSolution stderr:\n%s\n", trimOutput(r.SolutionStderr))
	}
	if len(r.BruteStderr) > 0 {
		fmt.Fprintf(&b, "\nBrute stderr:\n%s\n", trimOutput(r.BruteStderr))
	}
	if len(r.GeneratorStderr) > 0 {
		fmt.Fprintf(&b, "\nGenerator stderr:\n%s\n", trimOutput(r.GeneratorStderr))
	}

	fmt.Fprintf(&b, "\nDiff summary:\n%s\n",
		runner.DiffSummary(string(r.Expected), string(r.Got), 400))

	if r.Truncated {
		fmt.Fprintf(&b, "\n(output capture was truncated at the size limit)\n")
	}
	return b.String()
}

func trimOutput(b []byte) string {
	s := strings.TrimRight(string(b), "\n")
	if len(s) > 20_000 {
		return s[:20_000] + "\n... truncated ..."
	}
	return s
}

func sideBySide(expected, got string, width int) string {
	const maxLines = 2000

	exp := displayLines(expected, maxLines)
	gotLines := displayLines(got, maxLines)

	n := len(exp)
	if len(gotLines) > n {
		n = len(gotLines)
	}

	first := -1
	for i := 0; i < n; i++ {
		var a, b string
		if i < len(exp) {
			a = exp[i]
		}
		if i < len(gotLines) {
			b = gotLines[i]
		}
		if a != b {
			first = i
			break
		}
	}

	colW := (width - 13) / 2
	if colW < 1 {
		colW = 1
	}

	var out strings.Builder
	header := "     " + pad(truncate("expected", colW), colW) +
		" │ " +
		"     " + pad(truncate("got", colW), colW)
	out.WriteString(dimStyle.Render(header))
	out.WriteString("\n")

	for i := 0; i < n; i++ {
		var a, b string
		if i < len(exp) {
			a = exp[i]
		}
		if i < len(gotLines) {
			b = gotLines[i]
		}

		a = pad(truncate(a, colW), colW)
		b = pad(truncate(b, colW), colW)
		if i == first {
			a = diffHighlight.Render(a)
			b = diffHighlight.Render(b)
		}

		marker := " "
		if i == first {
			marker = ">"
		}
		fmt.Fprintf(&out, "%s%3d %s │ %4d %s", marker, i+1, a, i+1, b)
		if i < n-1 {
			out.WriteString("\n")
		}
	}

	if len(exp) >= maxLines || len(gotLines) >= maxLines {
		out.WriteString("\n... output truncated for display ...")
	}
	if n == 0 {
		return "(no output)"
	}
	return out.String()
}

func displayLines(s string, limit int) []string {
	s = strings.TrimRight(s, "\n")
	if s == "" {
		return []string{""}
	}
	lines := strings.Split(s, "\n")
	if limit > 0 && len(lines) > limit {
		lines = lines[:limit]
	}
	return lines
}

func truncate(s string, w int) string {
	s = strings.ReplaceAll(s, "\t", "    ")
	r := []rune(s)
	if len(r) <= w {
		return s
	}
	if w <= 1 {
		return string(r[:w])
	}
	return string(r[:w-1]) + "…"
}

func pad(s string, w int) string {
	l := len([]rune(s))
	if l >= w {
		return s
	}
	return s + strings.Repeat(" ", w-l)
}
