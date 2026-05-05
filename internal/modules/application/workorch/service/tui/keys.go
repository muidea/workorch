package tui

// Key bindings for the TUI. Keys are compared as strings in handleKey().
const (
	KeyUp          = "k"
	KeyDown        = "j"
	KeyEnter       = "enter"
	KeyQuit        = "q"
	KeyCtrlC       = "ctrl+c"
	KeyNextTicket  = "ctrl+t"
	KeyLeaveWS     = "ctrl+d"
	KeyToggleAlert = "ctrl+s"
	KeyShowRuns    = "ctrl+r"
	KeyShowWSList  = "ctrl+w"
)

func HelpText(mode string) string {
	switch mode {
	case "workspace":
		return Dim.Render("j/k: navigate | ctrl+d: back | ctrl+t: next ticket | q: quit")
	default:
		return Dim.Render("j/k: navigate | enter: select | ctrl+t: next ticket | q: quit")
	}
}
