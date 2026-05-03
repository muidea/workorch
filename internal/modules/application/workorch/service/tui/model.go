package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/muidea/workorch/internal/modules/application/workorch/biz/client"
)

type viewMode int

const (
	modeStatusDashboard viewMode = iota
	modeWorkspaceDetail
	modeRunsList
	modeQuit
)

type AppModel struct {
	cl *client.Client

	mode viewMode

	// Data
	workspaces []client.Workspace
	decisions  []client.Decision
	approvals  []client.Approval
	runs       []client.Run

	// Current workspace detail
	currentWS     string
	wsSnapshot    *client.WorkspaceSnapshot

	// Cursors
	cursor    int
	alertOn   bool

	// Dimensions
	width  int
	height int

	// Config
	pollInterval time.Duration
	watchMode    bool

	// Error
	lastError error
}

func NewAppModel(cl *client.Client, watchMode bool) AppModel {
	return AppModel{
		cl:           cl,
		mode:         modeStatusDashboard,
		alertOn:      true,
		pollInterval: 2 * time.Second,
		watchMode:    watchMode,
	}
}

func (m AppModel) Init() tea.Cmd {
	return tea.Batch(
		FetchAll(m.cl),
		PollTick(m.pollInterval),
	)
}

func (m AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	// Data messages
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

	case WorkspaceSnapshotMsg:
		if msg.Error == nil {
			m.wsSnapshot = msg.Snapshot
		}
		return m, nil

	case RunListMsg:
		if msg.Error == nil {
			m.runs = msg.Runs
		}
		return m, nil

	case RunSnapshotMsg:
		return m, nil

	case RunObservabilityMsg:
		return m, nil

	case TickMsg:
		if m.watchMode {
			return m, tea.Batch(
				FetchAll(m.cl),
				PollTick(m.pollInterval),
			)
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

	switch m.mode {
	case modeStatusDashboard:
		switch {
		case key == "q" || key == "ctrl+c":
			m.mode = modeQuit
			return m, tea.Quit

		case key == "ctrl+t":
			return m, m.nextTicketCmd()

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
				wsID := m.workspaces[m.cursor].ID
				m.mode = modeWorkspaceDetail
				m.currentWS = wsID
				m.cursor = 0
				return m, FetchWorkspaceSnapshot(m.cl, wsID)
			}
			return m, nil

		case key == "ctrl+r":
			return m, m.fetchRunsCmd()

		case key == "ctrl+s":
			m.alertOn = !m.alertOn
			return m, nil
		}

	case modeWorkspaceDetail:
		switch {
		case key == "ctrl+d" || key == "ctrl+w":
			m.mode = modeStatusDashboard
			m.currentWS = ""
			m.wsSnapshot = nil
			m.cursor = 0
			return m, FetchAll(m.cl)

		case key == "ctrl+t":
			return m, m.nextTicketInWSCmd()

		case key == "down" || key == "j":
			m.cursor++
			return m, nil

		case key == "up" || key == "k":
			if m.cursor > 0 {
				m.cursor--
			}
			return m, nil

		case key == "ctrl+s":
			m.alertOn = !m.alertOn
			return m, nil

		case key == "ctrl+r":
			return m, m.fetchRunsCmd()
		}
	}

	return m, nil
}

func (m *AppModel) nextTicketCmd() tea.Cmd {
	if len(m.workspaces) == 0 {
		return nil
	}
	// Find the next workspace with an open ticket
	for i := 1; i <= len(m.workspaces); i++ {
		idx := (m.cursor + i) % len(m.workspaces)
		wsID := m.workspaces[idx].ID
		if wsHasOpenItems(wsID, m.decisions, m.approvals) {
			m.cursor = idx
			break
		}
	}
	return nil
}

func (m *AppModel) nextTicketInWSCmd() tea.Cmd {
	total := len(filterOpenDecisionsForWS(m.currentWS, m.decisions)) +
		len(filterPendingApprovalsForWS(m.currentWS, m.approvals))
	if total == 0 {
		return nil
	}
	m.cursor = (m.cursor + 1) % total
	return nil
}

func (m *AppModel) fetchRunsCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		runs, err := m.cl.ListRuns(ctx, "")
		return RunListMsg{Runs: runs, Error: err}
	}
}

// View renders the TUI.
func (m AppModel) View() string {
	if m.mode == modeQuit {
		return ""
	}

	// Calculate layout
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
	switch m.mode {
	case modeStatusDashboard:
		mainContent = StatusView(m.workspaces, m.cursor, mainWidth)
	case modeWorkspaceDetail:
		ws := findWorkspace(m.workspaces, m.currentWS)
		mainContent = WorkspaceDetailView(ws, m.wsSnapshot, m.cursor, mainWidth)
	}

	var alertPanel string
	if m.alertOn {
		alertPanel = AlertView(m.workspaces, m.decisions, m.approvals, true, alertWidth)
	}

	// Combine left (main) and right (alert) panels
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

	// Bottom help bar
	helpBar := Dim.Render("j/k: navigate | enter: select | ctrl+t: next ticket | q: quit")
	if m.mode == modeWorkspaceDetail {
		helpBar = Dim.Render("j/k: navigate | ctrl+d: back | ctrl+t: next ticket | q: quit")
	}

	// Error bar
	if m.lastError != nil {
		helpBar += "\n" + Error.Render(fmt.Sprintf("Error: %v", m.lastError))
	}

	return mainContent + "\n" + helpBar + "\n"
}

func findWorkspace(workspaces []client.Workspace, id string) *client.Workspace {
	for i := range workspaces {
		if workspaces[i].ID == id {
			return &workspaces[i]
		}
	}
	return nil
}

func wsHasOpenItems(wsID string, decisions []client.Decision, approvals []client.Approval) bool {
	for _, d := range decisions {
		if d.WorkspaceID == wsID && d.State == "open" {
			return true
		}
	}
	for _, a := range approvals {
		if a.WorkspaceID == wsID && a.State == "pending" {
			return true
		}
	}
	return false
}

func filterOpenDecisionsForWS(wsID string, decisions []client.Decision) []client.Decision {
	var out []client.Decision
	for _, d := range decisions {
		if d.WorkspaceID == wsID && d.State == "open" {
			out = append(out, d)
		}
	}
	return out
}

func filterPendingApprovalsForWS(wsID string, approvals []client.Approval) []client.Approval {
	var out []client.Approval
	for _, a := range approvals {
		if a.WorkspaceID == wsID && a.State == "pending" {
			out = append(out, a)
		}
	}
	return out
}
