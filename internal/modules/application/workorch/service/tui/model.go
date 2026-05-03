package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/muidea/workorch/internal/modules/application/workorch/biz/backend"
)

type viewMode int

const (
	modeStatusDashboard viewMode = iota
	modeWorkspaceDetail
	modeRunsList
	modeQuit
)

type AppModel struct {
	be backend.Backend

	mode viewMode

	// Data
	workspaces []backend.WorkspaceInfo
	decisions  []backend.DecisionInfo
	approvals  []backend.ApprovalInfo
	runs       []backend.RunInfo

	// Current workspace detail
	currentWS  string
	wsSnapshot *backend.WorkspaceInfo

	// Cursors
	cursor  int
	alertOn bool

	// Dimensions
	width  int
	height int

	// Error
	lastError error
}

func NewAppModel(be backend.Backend) AppModel {
	return AppModel{
		be:      be,
		mode:    modeStatusDashboard,
		alertOn: true,
	}
}

func (m AppModel) Init() tea.Cmd {
	return FetchAll(m.be)
}

func (m AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case WorkspaceListMsg:
		if msg.Error != nil {
			m.lastError = msg.Error
		} else {
			m.workspaces = msg.Workspaces
			m.lastError = nil
		}
		return m, nil

	case DecisionListMsg:
		if msg.Error == nil {
			m.decisions = msg.Decisions
		}
		return m, nil

	case ApprovalListMsg:
		if msg.Error == nil {
			m.approvals = msg.Approvals
		}
		return m, nil

	case ErrorMsg:
		m.lastError = msg.Error
		return m, nil
	}

	return m, nil
}

func (m *AppModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	switch {
	case key == "q" || key == "ctrl+c":
		m.mode = modeQuit
		return m, tea.Quit

	case key == "down" || key == "j":
		if m.cursor < len(m.workspaces)-1 {
			m.cursor++
		}
		return m, nil

	case key == "up" || key == "k":
		if m.cursor > 0 {
			m.cursor--
		}
		return m, nil

	case key == "enter":
		if len(m.workspaces) > 0 && m.cursor < len(m.workspaces) {
			wsID := string(m.workspaces[m.cursor].ID)
			m.mode = modeWorkspaceDetail
			m.currentWS = wsID
			m.cursor = 0
		}
		return m, nil

	case key == "ctrl+d" || key == "ctrl+w":
		m.mode = modeStatusDashboard
		m.currentWS = ""
		m.cursor = 0
		return m, FetchAll(m.be)

	case key == "ctrl+s":
		m.alertOn = !m.alertOn
		return m, nil
	}

	return m, nil
}

// View renders the TUI.
func (m AppModel) View() string {
	if m.mode == modeQuit {
		return ""
	}

	mainWidth := m.width
	alertWidth := 0
	if m.alertOn {
		alertWidth = 18
		mainWidth -= alertWidth
	}
	if mainWidth < 40 {
		mainWidth = 40
	}

	var mainContent string
	if m.mode == modeStatusDashboard {
		mainContent = StatusView(m.workspaces, m.cursor, mainWidth)
	} else {
		ws := findWorkspace(m.workspaces, m.currentWS)
		mainContent = WorkspaceDetailView(ws, m.cursor, mainWidth)
	}

	var alertPanel string
	if m.alertOn {
		alertPanel = AlertView(m.workspaces, m.decisions, m.approvals, true, alertWidth)
	}

	if alertPanel != "" {
		mainLines := strings.Split(mainContent, "\n")
		alertLines := strings.Split(alertPanel, "\n")
		maxLines := len(mainLines)
		if len(alertLines) > maxLines {
			maxLines = len(alertLines)
		}
		var combined strings.Builder
		for i := 0; i < maxLines; i++ {
			left := ""
			if i < len(mainLines) {
				left = mainLines[i]
			}
			right := ""
			if i < len(alertLines) {
				right = alertLines[i]
			}
			combined.WriteString(fmt.Sprintf("%-*s%s\n", mainWidth, left, right))
		}
		mainContent = combined.String()
	}

	helpBar := Dim.Render("j/k: navigate | enter: select | ctrl+w: list | q: quit")
	if m.lastError != nil {
		helpBar += "\n" + Error.Render(fmt.Sprintf("Error: %v", m.lastError))
	}

	return mainContent + "\n" + helpBar + "\n"
}

func findWorkspace(workspaces []backend.WorkspaceInfo, id string) *backend.WorkspaceInfo {
	for i := range workspaces {
		if string(workspaces[i].ID) == id {
			return &workspaces[i]
		}
	}
	return nil
}
