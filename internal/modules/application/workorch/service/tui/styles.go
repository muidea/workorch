package tui

import (
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type Theme struct {
	Primary     lipgloss.Color
	Success     lipgloss.Color
	Warning     lipgloss.Color
	Error       lipgloss.Color
	Dim         lipgloss.Color
	Alert       lipgloss.Color
	Info        lipgloss.Color
	SelectionBG lipgloss.Color
	SelectionFG lipgloss.Color
}

func darkTheme() Theme {
	return Theme{
		Primary:     lipgloss.Color("63"),
		Success:     lipgloss.Color("42"),
		Warning:     lipgloss.Color("214"),
		Error:       lipgloss.Color("196"),
		Dim:         lipgloss.Color("240"),
		Alert:       lipgloss.Color("208"),
		Info:        lipgloss.Color("63"),
		SelectionBG: lipgloss.Color("63"),
		SelectionFG: lipgloss.Color("0"),
	}
}

func lightTheme() Theme {
	return Theme{
		Primary:     lipgloss.Color("25"),
		Success:     lipgloss.Color("28"),
		Warning:     lipgloss.Color("130"),
		Error:       lipgloss.Color("124"),
		Dim:         lipgloss.Color("244"),
		Alert:       lipgloss.Color("166"),
		Info:        lipgloss.Color("25"),
		SelectionBG: lipgloss.Color("25"),
		SelectionFG: lipgloss.Color("15"),
	}
}

func DetectTheme() Theme {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("WORKORCH_THEME"))) {
	case "light":
		return lightTheme()
	case "dark":
		return darkTheme()
	case "auto", "":
	}
	fgbg := os.Getenv("COLORFGBG")
	if strings.HasSuffix(fgbg, ";15") || strings.Contains(fgbg, ";15;") {
		return lightTheme()
	}
	return darkTheme()
}

var (
	Success   lipgloss.Style
	Warning   lipgloss.Style
	Error     lipgloss.Style
	Dim       lipgloss.Style
	Alert     lipgloss.Style
	Info      lipgloss.Style
	Title     lipgloss.Style
	Heading   lipgloss.Style
	Selection lipgloss.Style

	Border      lipgloss.Style
	AlertBorder lipgloss.Style
)

func init() {
	ApplyTheme(DetectTheme())
}

func ApplyTheme(theme Theme) {
	Success = lipgloss.NewStyle().Foreground(theme.Success)
	Warning = lipgloss.NewStyle().Foreground(theme.Warning)
	Error = lipgloss.NewStyle().Foreground(theme.Error)
	Dim = lipgloss.NewStyle().Foreground(theme.Dim)
	Alert = lipgloss.NewStyle().Foreground(theme.Alert)
	Info = lipgloss.NewStyle().Foreground(theme.Info)
	Title = lipgloss.NewStyle().Bold(true).Foreground(theme.Primary).Padding(0, 1)
	Heading = lipgloss.NewStyle().Bold(true).Foreground(theme.Dim)
	Selection = lipgloss.NewStyle().Background(theme.SelectionBG).Foreground(theme.SelectionFG)
	Border = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.Primary)
	AlertBorder = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.Alert)
}

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
