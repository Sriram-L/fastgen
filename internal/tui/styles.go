package tui

import "github.com/charmbracelet/lipgloss"

var (
	accentColor = lipgloss.AdaptiveColor{Light: "#5B21B6", Dark: "#A78BFA"}
	okColor     = lipgloss.AdaptiveColor{Light: "#047857", Dark: "#34D399"}
	badColor    = lipgloss.AdaptiveColor{Light: "#B91C1C", Dark: "#F87171"}
	warnColor   = lipgloss.AdaptiveColor{Light: "#B45309", Dark: "#FBBF24"}
	dimColor    = lipgloss.AdaptiveColor{Light: "#6B7280", Dark: "#6B7280"}
	textColor   = lipgloss.AdaptiveColor{Light: "#111827", Dark: "#E5E7EB"}

	accentStyle = lipgloss.NewStyle().Foreground(accentColor)
	okStyle     = lipgloss.NewStyle().Foreground(okColor)
	badStyle    = lipgloss.NewStyle().Foreground(badColor).Bold(true)
	warnStyle   = lipgloss.NewStyle().Foreground(warnColor)
	dimStyle    = lipgloss.NewStyle().Foreground(dimColor)
	textStyle   = lipgloss.NewStyle().Foreground(textColor)

	boxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(accentColor).
			Padding(0, 1)

	errorBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(badColor).
			Padding(0, 1)

	paneStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(accentColor).
			Padding(0, 1)

	activeTabStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(accentColor).
			Padding(0, 1)

	inactiveTabStyle = lipgloss.NewStyle().
				Foreground(dimColor).
				Padding(0, 1)

	diffHighlight = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#7F1D1D", Dark: "#FECACA"}).
			Background(lipgloss.AdaptiveColor{Light: "#FEF2F2", Dark: "#7A3B3B"})
)

func verdictStyle(v string) lipgloss.Style {
	switch v {
	case "AC":
		return okStyle
	case "WA":
		return badStyle
	case "TLE":
		return warnStyle
	default:
		return badStyle
	}
}
