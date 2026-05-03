package tui

import "github.com/charmbracelet/lipgloss"

var (
	Primary   = lipgloss.NewStyle().Foreground(lipgloss.Color("63"))
	Success   = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	Warning   = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	Error     = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	Dim       = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	Alert     = lipgloss.NewStyle().Foreground(lipgloss.Color("208"))
	Info      = lipgloss.NewStyle().Foreground(lipgloss.Color("63"))

	Title   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("63")).Padding(0, 1)
	Heading = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("240"))

	Selection = lipgloss.NewStyle().Background(lipgloss.Color("63")).Foreground(lipgloss.Color("0"))

	Border = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("63"))

	AlertBorder = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("208"))
)

func StateStyle(state string) string {
	switch state {
	case "running":
		return Success.Render("✓") + " " + state
	case "completed":
		return Success.Render("✓") + " " + state
	case "blocked_on_decision":
		return Warning.Render("⚠") + " " + state
	case "blocked_on_approval":
		return Alert.Render("⚡") + " " + state
	case "failed":
		return Error.Render("✗") + " " + state
	case "waiting_decision":
		return Warning.Render("⚠") + " " + state
	case "draft":
		return Dim.Render("○") + " " + state
	default:
		return state
	}
}
