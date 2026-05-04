package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/muidea/workorch/internal/modules/application/workorch/biz/backend"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

type viewMode int

const (
	modeAgentSession viewMode = iota
	modeQuit
)

type progressState string

const (
	progressIdle            progressState = "idle"
	progressSubmitting      progressState = "submitting"
	progressRunning         progressState = "running"
	progressWaitingDecision progressState = "waiting_decision"
	progressWaitingApproval progressState = "waiting_approval"
	progressCompleted       progressState = "completed"
	progressFailed          progressState = "failed"
	progressInterrupted     progressState = "interrupted"
)

type Options struct {
	OnActiveWorkspaceChanged func(workspaceID string)
}

type transcriptItem struct {
	Kind string
	Text string
}

const (
	maxInputHistory = 100
	maxResultLines  = 1000
	ctrlCQuitWindow = 750 * time.Millisecond
)

type AppModel struct {
	be backend.Backend

	mode viewMode

	workspaces []backend.WorkspaceInfo
	decisions  []backend.DecisionInfo
	approvals  []backend.ApprovalInfo

	defaultWorkspaceID  string
	activeWorkspaceID   string
	selectedWorkspaceID string
	workspaceSelectMode bool
	activeRunID         string
	cursor              int

	input         string
	inputCursor   int
	inputHistory  []string
	historyCursor int
	currentTask   string
	dodItems      []string
	transcript    []transcriptItem
	result        string
	artifacts     []string
	memoryRecalls []backend.MemoryRecallTraceInfo
	progress      progressState
	progressText  string
	failureReason string
	failureHint   string
	running       bool

	blockingDecision *backend.DecisionInfo
	blockingApproval *backend.ApprovalInfo
	helpOpen         bool
	mainViewport     *viewport.Model

	workspaceListRefreshInFlight bool
	workspaceListRefreshPending  bool

	eventCh     chan backend.Event
	unsubscribe func()
	initialGoal string
	options     Options

	width  int
	height int

	lastError error
	lastCtrlC time.Time
}

func NewAppModel(be backend.Backend, defaultWorkspaceID string, initialGoal string) AppModel {
	return NewAppModelWithOptions(be, defaultWorkspaceID, initialGoal, Options{})
}

func NewAppModelWithOptions(be backend.Backend, defaultWorkspaceID string, initialGoal string, options Options) AppModel {
	eventCh := make(chan backend.Event, 64)
	unsubscribe := func() {}
	if be != nil {
		if unsub, err := be.Events().Subscribe(context.Background(), func(_ context.Context, evt backend.Event) {
			select {
			case eventCh <- evt:
			default:
			}
		}); err == nil {
			unsubscribe = unsub
		}
	}
	return AppModel{
		be:                  be,
		mode:                modeAgentSession,
		defaultWorkspaceID:  defaultWorkspaceID,
		activeWorkspaceID:   defaultWorkspaceID,
		selectedWorkspaceID: defaultWorkspaceID,
		eventCh:             eventCh,
		unsubscribe:         unsubscribe,
		initialGoal:         strings.TrimSpace(initialGoal),
		currentTask:         strings.TrimSpace(initialGoal),
		transcript:          initialTranscript(initialGoal),
		options:             options,
		progress:            progressIdle,
		historyCursor:       -1,
		mainViewport:        newMainViewport(),
	}
}

func (m AppModel) Init() tea.Cmd {
	cmds := []tea.Cmd{FetchAll(m.be), WaitForBackendEvent(m.eventCh)}
	if m.initialGoal != "" && m.activeWorkspaceID != "" {
		cmds = append(cmds, SubmitGoal(m.be, m.activeWorkspaceID, m.initialGoal, m.dodItems))
	}
	return tea.Batch(cmds...)
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
		pendingRefresh := m.workspaceListRefreshPending
		m.workspaceListRefreshInFlight = false
		m.workspaceListRefreshPending = false
		if msg.Error != nil {
			m.setLastError(msg.Error)
			return m, nil
		}
		previousActive := m.activeWorkspaceID
		m.workspaces = msg.Workspaces
		m.ensureActiveWorkspace()
		cmds := []tea.Cmd{}
		if m.activeWorkspaceID != "" && (previousActive != m.activeWorkspaceID || m.activeRunID != "" || len(msg.Workspaces) > 0) {
			cmds = append(cmds, FetchWorkspaceContext(m.be, m.activeWorkspaceID))
		}
		if pendingRefresh {
			cmds = append(cmds, m.requestWorkspaceListRefresh())
		}
		return m, tea.Batch(cmds...)
	case WorkspaceContextMsg:
		if msg.Error != nil {
			m.setLastError(msg.Error)
			return m, nil
		}
		m.applyWorkspaceContext(msg)
		return m, m.fetchActiveBlockerDetails()
	case WorkspaceCreatedMsg:
		if msg.Error != nil {
			m.setLastError(msg.Error)
			return m, nil
		}
		if msg.Workspace != nil {
			m.upsertWorkspace(*msg.Workspace)
			m.setActiveWorkspace(string(msg.Workspace.ID))
			m.progress = progressIdle
			m.progressText = "Workspace created"
			m.result = fmt.Sprintf("Workspace created: %s", workspaceLabel(*msg.Workspace))
			m.lastError = nil
			return m, tea.Batch(FetchAll(m.be), FetchWorkspaceContext(m.be, string(msg.Workspace.ID)))
		}
		return m, nil
	case DecisionListMsg:
		if msg.Error != nil {
			m.setLastError(msg.Error)
			return m, nil
		}
		m.decisions = msg.Decisions
		m.syncBlockingDecisionFromList()
		return m, nil
	case DecisionDetailMsg:
		if msg.Error != nil {
			m.setLastError(msg.Error)
			return m, nil
		}
		if msg.Decision != nil {
			m.upsertDecision(*msg.Decision)
			if m.blockingDecision != nil && m.blockingDecision.ID == msg.Decision.ID {
				m.blockingDecision = msg.Decision
				m.progressText = nonEmpty(msg.Decision.Question, msg.Decision.Reason, string(msg.Decision.ID))
			}
		}
		return m, nil
	case ApprovalListMsg:
		if msg.Error != nil {
			m.setLastError(msg.Error)
			return m, nil
		}
		m.approvals = msg.Approvals
		m.syncBlockingApprovalFromList()
		return m, nil
	case ApprovalDetailMsg:
		if msg.Error != nil {
			m.setLastError(msg.Error)
			return m, nil
		}
		if msg.Approval != nil {
			m.upsertApproval(*msg.Approval)
			if m.blockingApproval != nil && m.blockingApproval.ID == msg.Approval.ID {
				m.blockingApproval = msg.Approval
				m.progressText = nonEmpty(msg.Approval.Reason, string(msg.Approval.ID))
			}
		}
		return m, nil
	case BackendEventMsg:
		cmd := m.applyBackendEvent(msg.Event)
		return m, tea.Batch(WaitForBackendEvent(m.eventCh), cmd)
	case RunStartedMsg:
		if msg.Error != nil {
			if isTransientContextError(msg.Error) {
				return m, nil
			}
			m.setLastError(msg.Error)
			m.running = false
			m.progress = progressFailed
			m.progressText = "Command failed"
			return m, nil
		}
		if msg.Run != nil {
			m.running = true
			m.activeRunID = string(msg.Run.ID)
			m.applyRunStarted(msg.Run)
			return m, m.fetchActiveBlockerDetails()
		}
		return m, nil
	case DecisionResolvedMsg:
		if msg.Error != nil {
			m.setLastError(msg.Error)
			return m, nil
		}
		m.blockingDecision = nil
		m.clearInput()
		m.progress = progressRunning
		m.progressText = "Decision resolved; resuming run"
		return m, nil
	case ApprovalResolvedMsg:
		if msg.Error != nil {
			m.setLastError(msg.Error)
			return m, nil
		}
		m.blockingApproval = nil
		m.clearInput()
		m.progress = progressRunning
		m.progressText = "Approval resolved; resuming run"
		return m, nil
	case ErrorMsg:
		m.setLastError(msg.Error)
		return m, nil
	}
	return m, nil
}

func (m *AppModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if m.helpOpen {
		switch key {
		case "esc", "ctrl+h", KeyQuit, KeyEnter:
			m.helpOpen = false
			return m, nil
		case "pgup":
			m.scrollMain(-6)
			return m, nil
		case "pgdown":
			m.scrollMain(6)
			return m, nil
		default:
			return m, nil
		}
	}
	width, contentHeight := m.contentDimensions()
	switch key {
	case KeyQuit, "ctrl+q":
		m.mode = modeQuit
		m.unsubscribe()
		return m, tea.Quit
	case KeyCtrlC:
		if m.running {
			m.running = false
			m.progress = progressInterrupted
			m.progressText = "Interrupted"
			return m, nil
		}
		now := time.Now()
		if m.lastCtrlC.IsZero() || now.Sub(m.lastCtrlC) > ctrlCQuitWindow {
			m.lastCtrlC = now
			m.progressText = "Press Ctrl+C again to quit, or use /exit"
			m.lastError = nil
			return m, nil
		}
		m.mode = modeQuit
		m.unsubscribe()
		return m, tea.Quit
	case "esc":
		if m.workspaceSelectMode {
			m.cancelWorkspaceSelection()
			return m, nil
		}
		m.clearInput()
		m.lastError = nil
		return m, nil
	case "ctrl+h":
		m.helpOpen = true
		return m, nil
	case "backspace":
		m.deleteBeforeCursor()
		return m, nil
	case "delete":
		m.deleteAtCursor()
		return m, nil
	case "left", "ctrl+b":
		m.moveInputCursor(-1)
		return m, nil
	case "right", "ctrl+f":
		m.moveInputCursor(1)
		return m, nil
	case "home", "ctrl+a":
		m.inputCursor = 0
		return m, nil
	case "end", "ctrl+e":
		m.inputCursor = len([]rune(m.input))
		return m, nil
	case " ", "space":
		m.insertInput(" ")
		return m, nil
	case "tab":
		m.completeSlashCommand()
		return m, nil
	case "ctrl+p":
		m.previousInputHistory()
		return m, nil
	case "ctrl+n":
		m.nextInputHistory()
		return m, nil
	case "pgup":
		m.refreshMainViewport(width, contentHeight)
		m.scrollMain(-6)
		return m, nil
	case "pgdown":
		m.refreshMainViewport(width, contentHeight)
		m.scrollMain(6)
		return m, nil
	case KeyShowWSList:
		m.toggleWorkspaceSelectMode()
		return m, nil
	case "down":
		if m.workspaceSelectMode {
			m.nextWorkspace()
		}
		return m, nil
	case "up":
		if m.workspaceSelectMode {
			m.prevWorkspace()
		}
		return m, nil
	case KeyDown:
		if m.workspaceSelectMode {
			m.nextWorkspace()
			return m, nil
		}
		m.insertInput(KeyDown)
		return m, nil
	case KeyUp:
		if m.workspaceSelectMode {
			m.prevWorkspace()
			return m, nil
		}
		m.insertInput(KeyUp)
		return m, nil
	case "alt+enter":
		m.insertInput("\n")
		return m, nil
	case KeyEnter:
		if msg.Alt {
			m.insertInput("\n")
			return m, nil
		}
		return m.submitInput()
	default:
		if msg.Type == tea.KeyRunes {
			m.insertInput(string(msg.Runes))
		}
		return m, nil
	}
}

func (m *AppModel) submitInput() (tea.Model, tea.Cmd) {
	text := strings.TrimSpace(m.input)
	if text == "" {
		if m.workspaceSelectMode {
			return m.confirmWorkspaceSelection()
		}
		return m, nil
	}
	if strings.HasPrefix(text, "/") {
		if handledModel, cmd, handled := m.handleSlashCommand(text); handled {
			return handledModel, cmd
		}
	}
	m.addInputHistory(text)
	if m.blockingDecision != nil {
		ticketID := string(m.blockingDecision.ID)
		m.clearInput()
		m.lastError = nil
		m.appendTranscript("Decision", text)
		return m, ResolveDecision(m.be, ticketID, text)
	}
	if m.blockingApproval != nil {
		approvalID := string(m.blockingApproval.ID)
		approve, ok := parseApprovalInputWithContext(text, m.blockingApproval.Reason)
		if !ok {
			m.progress = progressWaitingApproval
			m.progressText = "Approval intent unclear; use a clear approval or rejection"
			m.lastError = nil
			return m, nil
		}
		m.clearInput()
		m.lastError = nil
		m.appendTranscript("Approval", text)
		return m, ResolveApproval(m.be, approvalID, approve)
	}
	if m.activeWorkspaceID == "" {
		m.lastError = fmt.Errorf("no active workspace")
		return m, nil
	}
	m.result = ""
	m.artifacts = nil
	m.failureReason = ""
	m.lastError = nil
	m.clearInput()
	m.cancelWorkspaceSelection()
	m.currentTask = text
	m.appendTranscript("Goal", text)
	m.running = true
	m.progress = progressSubmitting
	m.progressText = "Submitting goal; waiting for backend acknowledgment"
	m.resetMainViewport()
	return m, SubmitGoal(m.be, m.activeWorkspaceID, text, m.dodItems)
}

func (m *AppModel) handleSlashCommand(text string) (tea.Model, tea.Cmd, bool) {
	cmd := strings.TrimSpace(strings.TrimPrefix(text, "/"))
	if cmd == "" {
		return m, nil, false
	}
	name := strings.ToLower(strings.Fields(cmd)[0])
	switch name {
	case "exit", "quit", "q":
		m.addInputHistory(text)
		m.mode = modeQuit
		m.unsubscribe()
		return m, tea.Quit, true
	case "help", "h", "?":
		m.addInputHistory(text)
		m.clearInput()
		m.helpOpen = true
		return m, nil, true
	case "clear", "reset":
		m.addInputHistory(text)
		m.clearInput()
		m.transcript = nil
		m.result = ""
		m.artifacts = nil
		m.failureReason = ""
		m.lastError = nil
		m.resetMainViewport()
		return m, nil, true
	case "workspace", "ws":
		m.addInputHistory(text)
		args := strings.TrimSpace(strings.TrimPrefix(cmd, strings.Fields(cmd)[0]))
		if strings.HasPrefix(strings.ToLower(args), "create") {
			req, err := parseWorkspaceCreateArgs(strings.TrimSpace(strings.TrimPrefix(args, strings.Fields(args)[0])))
			if err != nil {
				m.lastError = err
				return m, nil, true
			}
			m.clearInput()
			m.cancelWorkspaceSelection()
			m.progress = progressSubmitting
			m.progressText = "Creating workspace"
			m.lastError = nil
			return m, CreateWorkspace(m.be, req), true
		}
		m.clearInput()
		m.toggleWorkspaceSelectMode()
		return m, nil, true
	case "result", "refresh":
		m.addInputHistory(text)
		m.clearInput()
		m.resetMainViewport()
		m.progressText = "Refreshing workspace result"
		return m, FetchWorkspaceContext(m.be, m.activeWorkspaceID), true
	case "memory", "mem":
		m.addInputHistory(text)
		m.clearInput()
		m.result = renderMemoryRecallSummary(m.memoryRecalls)
		m.resetMainViewport()
		m.progressText = "Showing memory recall summary"
		return m, nil, true
	case "dod":
		m.addInputHistory(text)
		args := strings.TrimSpace(strings.TrimPrefix(cmd, strings.Fields(cmd)[0]))
		if args == "" {
			m.clearInput()
			m.result = renderPendingDoDResult(m.dodItems)
			m.resetMainViewport()
			m.progressText = "Showing pending DoD"
			return m, nil, true
		}
		items := parseDoDItems(args)
		if len(items) == 0 {
			m.lastError = fmt.Errorf("dod requires at least one non-empty item")
			return m, nil, true
		}
		m.dodItems = items
		m.clearInput()
		m.lastError = nil
		m.appendTranscript("DoD", strings.Join(items, "; "))
		m.result = renderPendingDoDResult(items)
		m.progressText = "DoD updated for next goal"
		m.resetMainViewport()
		return m, nil, true
	default:
		m.lastError = fmt.Errorf("unknown slash command: /%s", name)
		return m, nil, true
	}
}

func (m AppModel) View() string {
	if m.mode == modeQuit {
		return ""
	}
	width, height := m.viewSize()
	if m.helpOpen {
		return m.renderHelpOverlay(width, height)
	}

	input := m.renderInput(width)
	help := m.renderHelp(width)
	contentHeight := height - lineCount(input) - lineCount(help) - 1
	if contentHeight < 8 {
		contentHeight = 8
	}
	content := m.renderContent(width, contentHeight)
	return padLines(content, contentHeight) + "\n" + input + "\n" + help
}

func (m AppModel) viewSize() (int, int) {
	width := m.width
	if width == 0 {
		width = 100
	}
	height := m.height
	if height == 0 {
		height = 24
	}
	return width, height
}

func (m AppModel) contentDimensions() (int, int) {
	width, height := m.viewSize()
	input := m.renderInput(width)
	help := m.renderHelp(width)
	contentHeight := height - lineCount(input) - lineCount(help) - 1
	if contentHeight < 8 {
		contentHeight = 8
	}
	if width >= 96 {
		return width - 32, contentHeight
	}
	return width, contentHeight
}

func (m *AppModel) refreshMainViewport(width int, height int) {
	vp := m.ensureMainViewport()
	vp.Width = width
	vp.Height = height
	vp.MouseWheelEnabled = false
	vp.SetContent(strings.TrimRight(m.renderMain(width, 0), "\n"))
}

func (m AppModel) renderContent(width, height int) string {
	if width >= 96 {
		rightWidth := 30
		mainWidth := width - rightWidth - 2
		main := m.renderMainViewport(m.renderMain(mainWidth, 0), mainWidth, height)
		return joinColumns(main, m.renderWorkspacePanel(rightWidth, height), mainWidth+2)
	}

	content := strings.TrimRight(m.renderMainViewport(m.renderMain(width, 0), width, height), "\n")
	if len(m.workspaces) > 1 {
		remaining := height - lineCount(content) - 1
		if remaining >= 4 {
			content += "\n\n" + strings.TrimRight(m.renderWorkspacePanel(width, remaining), "\n")
		}
	}
	return content + "\n"
}

func (m AppModel) renderMainViewport(content string, width int, height int) string {
	vp := m.mainViewport
	if vp == nil {
		v := viewport.New(width, height)
		v.MouseWheelEnabled = false
		vp = &v
	}
	vp.Width = width
	vp.Height = height
	vp.MouseWheelEnabled = false
	vp.SetContent(strings.TrimRight(content, "\n"))
	return trimRightDisplaySpaceLines(vp.View())
}

func trimRightDisplaySpaceLines(value string) string {
	lines := strings.Split(value, "\n")
	for i := range lines {
		lines[i] = trimRightDisplaySpace(lines[i])
	}
	return strings.Join(lines, "\n")
}

func (m AppModel) renderMain(width, height int) string {
	var b strings.Builder
	b.WriteString(Title.Render("WorkOrch Agent"))
	b.WriteString("\n")
	b.WriteString(m.renderActiveWorkspace(width))
	b.WriteString("\n")
	if line := m.renderWorkspaceTemplateInfo(width); line != "" {
		b.WriteString(line)
		b.WriteString("\n")
	}
	b.WriteString(m.renderProgress(width))
	b.WriteString("\n")
	if len(m.transcript) > 0 {
		b.WriteString(renderTranscript(m.transcript, width-2, 5))
	}
	if m.currentTask != "" {
		b.WriteString(truncateDisplay("Task: "+m.currentTask, width-2))
		b.WriteString("\n")
	}
	if len(m.dodItems) > 0 {
		b.WriteString(renderDoDItems(m.dodItems, width-2, 4))
	}
	b.WriteString(Dim.Render(strings.Repeat("-", max(10, width-2))))
	b.WriteString("\n")

	if m.blockingDecision != nil {
		b.WriteString(Warning.Render("Decision required"))
		b.WriteString("\n")
		b.WriteString(truncateDisplay(blockingDecisionLabel(m.blockingDecision), width-2))
		b.WriteString("\n\n")
	}
	if m.blockingApproval != nil {
		b.WriteString(Alert.Render("Approval required"))
		b.WriteString("\n")
		b.WriteString(truncateDisplay(blockingApprovalLabel(m.blockingApproval), width-2))
		b.WriteString("\n\n")
	}

	b.WriteString(Heading.Render("Result"))
	b.WriteString("\n")
	resultLines := m.resultLineLimit(height, lineCount(b.String()))
	switch {
	case m.result != "":
		b.WriteString(renderRichText(m.result, width-2, resultLines))
		b.WriteString("\n")
	case m.failureReason != "":
		b.WriteString(Error.Render(truncateDisplay(m.failureReason, width-2)))
		b.WriteString("\n")
	default:
		b.WriteString(Dim.Render(m.emptyResultText()))
		b.WriteString("\n")
	}

	if artifacts := m.visibleArtifacts(); len(artifacts) > 0 {
		b.WriteString("\n")
		b.WriteString(Heading.Render("Artifacts"))
		b.WriteString("\n")
		for _, artifact := range artifacts {
			b.WriteString(truncateDisplay("- "+artifact, width-2))
			b.WriteString("\n")
		}
	}
	if m.lastError != nil {
		b.WriteString("\n")
		b.WriteString(Error.Render("Error: " + m.lastError.Error()))
		b.WriteString("\n")
	}
	return b.String()
}

func (m AppModel) resultLineLimit(height int, usedLines int) int {
	if height <= 0 {
		return maxResultLines
	}
	reserved := 0
	if artifacts := m.visibleArtifacts(); len(artifacts) > 0 {
		reserved += 2 + len(artifacts)
	}
	if m.lastError != nil {
		reserved += 2
	}
	limit := height - usedLines - reserved
	if limit < 4 {
		return 4
	}
	return limit
}

func (m AppModel) visibleArtifacts() []string {
	if len(m.artifacts) == 0 {
		return nil
	}
	result := strings.ToLower(m.result)
	if strings.Contains(result, "交付物索引") || strings.Contains(result, "artifact index") || strings.Contains(result, "artifacts") {
		return nil
	}
	return m.artifacts
}

func (m AppModel) renderActiveWorkspace(width int) string {
	ws := findWorkspace(m.workspaces, m.activeWorkspaceID)
	if ws == nil {
		return Dim.Render("Workspace: " + shortID(m.activeWorkspaceID))
	}
	label := workspaceLabel(*ws)
	if ws.WorkingDirectory != "" {
		label += "  " + ws.WorkingDirectory
	}
	return truncateDisplay("Workspace: "+label, width-2)
}

func (m AppModel) renderWorkspaceTemplateInfo(width int) string {
	ws := findWorkspace(m.workspaces, m.activeWorkspaceID)
	if ws == nil {
		return ""
	}
	parts := make([]string, 0, 2)
	if ws.DefaultSoulID != "" {
		parts = append(parts, "default soul: "+string(ws.DefaultSoulID))
	}
	if len(ws.SoulBindings) > 0 {
		parts = append(parts, "bindings: "+workspaceBindingSummary(ws.SoulBindings))
	}
	if len(parts) == 0 {
		return ""
	}
	return Dim.Render(truncateDisplay(strings.Join(parts, " | "), width-2))
}

func (m AppModel) renderProgress(width int) string {
	label := progressLabel(m.progress)
	if m.progressText != "" && !strings.EqualFold(m.progressText, label) {
		label += " - " + m.progressText
	}
	label = truncateDisplay(label, width-2)
	switch m.progress {
	case progressCompleted:
		return Success.Render(label)
	case progressFailed:
		return Error.Render(label)
	case progressWaitingDecision:
		return Warning.Render(label)
	case progressWaitingApproval:
		return Alert.Render(label)
	default:
		return Info.Render(label)
	}
}

func (m AppModel) renderWorkspacePanel(width, height int) string {
	var b strings.Builder
	b.WriteString(Heading.Render("Workspaces"))
	b.WriteString("\n")
	if len(m.workspaces) == 0 {
		b.WriteString(Dim.Render("No workspaces"))
		b.WriteString("\n")
		return b.String()
	}

	limit := max(2, (height-5)/2)
	if limit > len(m.workspaces) {
		limit = len(m.workspaces)
	}
	counts := workspaceNameCounts(m.workspaces)
	for _, ws := range visibleWorkspaces(m.workspaces, m.cursor, limit) {
		prefix := "  "
		if m.workspaceSelectMode && string(ws.ID) == m.selectedWorkspaceID {
			prefix = "> "
		} else if !m.workspaceSelectMode && string(ws.ID) == m.activeWorkspaceID {
			prefix = "> "
		}
		b.WriteString(workspacePanelLine(prefix, ws, counts, width-1))
		b.WriteString("\n")
	}
	if len(m.workspaces) > limit {
		b.WriteString(Dim.Render(fmt.Sprintf("+%d more", len(m.workspaces)-limit)))
		b.WriteString("\n")
	}

	openDecisions := len(filterOpenDecisions(m.decisions))
	pendingApprovals := len(filterPendingApprovals(m.approvals))
	b.WriteString("\n")
	b.WriteString(Warning.Render(fmt.Sprintf("%d decisions", openDecisions)))
	b.WriteString("\n")
	b.WriteString(Alert.Render(fmt.Sprintf("%d approvals", pendingApprovals)))
	b.WriteString("\n")
	return b.String()
}

func (m AppModel) renderInput(width int) string {
	prompt := shortID(m.activeWorkspaceID)
	if prompt == "none" {
		prompt = "workspace"
	}
	if m.blockingDecision != nil {
		prompt += " decision"
	} else if m.blockingApproval != nil {
		prompt += " approval"
	}
	input := m.renderInputBuffer(prompt, width)
	if suggestions := m.renderSlashCommandSuggestions(width); suggestions != "" {
		return suggestions + "\n" + input
	}
	return input
}

func (m AppModel) renderHelp(width int) string {
	help := "enter: submit | /help: help | /workspace: select/create ws | /dod: update DoD | /memory: recall | /exit: quit | ctrl+c: interrupt/quit x2"
	if width < 120 {
		help = "enter: submit | /help | /workspace | /dod | /memory | /exit | ctrl+c x2 quit"
	}
	if m.workspaceSelectMode {
		help = "ws select: up/down/j/k move | enter confirm | ctrl+w cancel | esc cancel"
	}
	return Dim.Render(truncateDisplay(help, width-2))
}

func (m AppModel) renderHelpOverlay(width, height int) string {
	lines := []string{
		"WorkOrch Help",
		"",
		"Enter: submit current goal or response",
		"Alt+Enter: insert newline",
		"Ctrl+P / Ctrl+N: input history",
		"Tab: slash command completion",
		"Ctrl+W: workspace selector",
		"Ctrl+H: help",
		"Ctrl+C: interrupt active run; press twice outside a run to quit",
		"PgUp / PgDn: scroll result",
		"Esc: clear input or close help",
		"/help /clear /workspace /workspace create /dod /memory /result /exit",
	}
	panelWidth := min(width-4, 72)
	if panelWidth < 30 {
		panelWidth = max(20, width-2)
	}
	panelHeight := len(lines) + 4
	if panelHeight > height-2 {
		panelHeight = height - 2
	}
	b := strings.Builder{}
	padTop := max(0, (height-panelHeight)/2)
	for i := 0; i < padTop; i++ {
		b.WriteString("\n")
	}
	b.WriteString(Border.Width(panelWidth).Render(strings.Join(lines[:min(len(lines), panelHeight-4)], "\n")))
	return b.String()
}

func initialTranscript(initialGoal string) []transcriptItem {
	if strings.TrimSpace(initialGoal) == "" {
		return nil
	}
	return []transcriptItem{{Kind: "Goal", Text: strings.TrimSpace(initialGoal)}}
}

func (m *AppModel) appendTranscript(kind string, text string) {
	kind = strings.TrimSpace(kind)
	text = strings.TrimSpace(text)
	if kind == "" || text == "" {
		return
	}
	m.transcript = append(m.transcript, transcriptItem{Kind: kind, Text: text})
	if len(m.transcript) > 20 {
		m.transcript = append([]transcriptItem(nil), m.transcript[len(m.transcript)-20:]...)
	}
}

func renderTranscript(items []transcriptItem, width int, limit int) string {
	if len(items) == 0 || width <= 0 || limit <= 0 {
		return ""
	}
	start := len(items) - limit
	if start < 0 {
		start = 0
	}
	var b strings.Builder
	b.WriteString(Heading.Render("Conversation"))
	b.WriteString("\n")
	for _, item := range items[start:] {
		b.WriteString(truncateDisplay(item.Kind+": "+item.Text, width))
		b.WriteString("\n")
	}
	return b.String()
}

func parseDoDItems(value string) []string {
	fields := strings.FieldsFunc(value, func(r rune) bool {
		return r == ';' || r == '；' || r == '\n'
	})
	items := make([]string, 0, len(fields))
	for _, field := range fields {
		item := strings.TrimSpace(field)
		if item != "" {
			items = append(items, item)
		}
	}
	return items
}

func renderPendingDoDResult(items []string) string {
	if len(items) == 0 {
		return "No custom DoD set. The next goal will use the Workspace template DoD when available, otherwise the default generic DoD."
	}
	var b strings.Builder
	b.WriteString("Pending DoD\n")
	for _, item := range items {
		b.WriteString("- ")
		b.WriteString(item)
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func renderMemoryRecallSummary(items []backend.MemoryRecallTraceInfo) string {
	if len(items) == 0 {
		return "No memory recall traces recorded for the current run."
	}
	var b strings.Builder
	b.WriteString("Memory recall traces\n\n")
	for idx, item := range items {
		if idx >= 12 {
			b.WriteString(fmt.Sprintf("- +%d more traces\n", len(items)-idx))
			break
		}
		status := nonEmpty(item.Status, "unknown")
		source := nonEmpty(item.Source, "unknown")
		stage := nonEmpty(item.Stage, "unknown")
		b.WriteString(fmt.Sprintf("- %s/%s: %s", source, stage, status))
		if len(item.RetrievedMemoryIDs) > 0 {
			b.WriteString(fmt.Sprintf(" (%d memories)", len(item.RetrievedMemoryIDs)))
		}
		if item.Reason != "" {
			b.WriteString(" - ")
			b.WriteString(item.Reason)
		}
		b.WriteString("\n")
		for _, memoryID := range item.RetrievedMemoryIDs {
			b.WriteString("  - ")
			b.WriteString(string(memoryID))
			if summary := memoryRecallHitSummary(item.RetrievedMemoryHits, memoryID); summary != "" {
				b.WriteString(": ")
				b.WriteString(summary)
			}
			b.WriteString("\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func memoryRecallHitSummary(items []backend.MemoryRecallHitInfo, memoryID common.MemoryID) string {
	for _, item := range items {
		if item.ID == memoryID {
			return item.Summary
		}
	}
	return ""
}

func renderDoDItems(items []string, width int, limit int) string {
	if len(items) == 0 || width <= 0 || limit <= 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(Heading.Render("DoD"))
	b.WriteString("\n")
	for idx, item := range items {
		if idx >= limit {
			b.WriteString(truncateDisplay(fmt.Sprintf("+%d more DoD items", len(items)-idx), width))
			b.WriteString("\n")
			break
		}
		b.WriteString(truncateDisplay("- "+item, width))
		b.WriteString("\n")
	}
	return b.String()
}

func parseWorkspaceCreateArgs(value string) (backend.CreateWorkspaceRequest, error) {
	fields := strings.Fields(value)
	req := backend.CreateWorkspaceRequest{}
	titleParts := make([]string, 0, len(fields))
	for idx := 0; idx < len(fields); idx++ {
		field := fields[idx]
		switch {
		case field == "--template" || field == "-template":
			idx++
			if idx >= len(fields) {
				return req, fmt.Errorf("workspace create requires a template value")
			}
			req.TemplateID = fields[idx]
		case strings.HasPrefix(field, "--template="):
			req.TemplateID = strings.TrimPrefix(field, "--template=")
		case strings.HasPrefix(field, "template="):
			req.TemplateID = strings.TrimPrefix(field, "template=")
		case field == "--dir" || field == "--working-dir" || field == "-dir":
			idx++
			if idx >= len(fields) {
				return req, fmt.Errorf("workspace create requires a directory value")
			}
			req.WorkingDirectory = fields[idx]
		case strings.HasPrefix(field, "--dir="):
			req.WorkingDirectory = strings.TrimPrefix(field, "--dir=")
		case strings.HasPrefix(field, "--working-dir="):
			req.WorkingDirectory = strings.TrimPrefix(field, "--working-dir=")
		case strings.HasPrefix(field, "dir="):
			req.WorkingDirectory = strings.TrimPrefix(field, "dir=")
		case field == "--description" || field == "--desc":
			idx++
			if idx >= len(fields) {
				return req, fmt.Errorf("workspace create requires a description value")
			}
			req.Description = fields[idx]
		case strings.HasPrefix(field, "--description="):
			req.Description = strings.TrimPrefix(field, "--description=")
		case strings.HasPrefix(field, "--desc="):
			req.Description = strings.TrimPrefix(field, "--desc=")
		case strings.HasPrefix(field, "description="):
			req.Description = strings.TrimPrefix(field, "description=")
		default:
			titleParts = append(titleParts, field)
		}
	}
	req.Title = strings.TrimSpace(strings.Join(titleParts, " "))
	if req.Title == "" {
		return req, fmt.Errorf("workspace create requires a title")
	}
	if req.Description == "" {
		req.Description = req.Title
	}
	return req, nil
}

func workspaceBindingSummary(bindings map[string]common.SoulID) string {
	stages := []string{"planning", "execution", "decision", "approval", "validation", "artifact"}
	parts := make([]string, 0, len(bindings))
	for _, stage := range stages {
		if bindings[stage] != "" {
			parts = append(parts, stage)
		}
	}
	for stage := range bindings {
		if !stringInSlice(stage, stages) {
			parts = append(parts, stage)
		}
	}
	return strings.Join(parts, ",")
}

func stringInSlice(value string, items []string) bool {
	for _, item := range items {
		if item == value {
			return true
		}
	}
	return false
}

func parseApprovalInput(value string) (bool, bool) {
	normalized := semanticApprovalText(value)
	if normalized == "" {
		return false, false
	}
	negative := containsAnySemanticCue(normalized, []string{
		"不同意", "不批准", "不确认", "不允许", "不是", "不要", "别执行", "不执行", "不继续", "不需要", "无需执行", "不能", "不可以", "不行", "拒绝", "取消", "停止", "终止", "驳回", "否决", "算了",
		"reject", "rejected", "deny", "denied", "disapprove", "cancel", "stop", "abort", "refuse", "refused", "decline", "declined", "notapprove", "notagree", "notok", "nothanks",
	})
	positive := containsAnySemanticCue(normalized, []string{
		"同意", "批准", "确认", "允许", "通过", "可以", "继续", "继续执行", "执行", "没问题", "没意见", "好的", "好", "是", "赞成", "请继续", "确认执行",
		"yes", "approve", "approved", "allow", "allowed", "ok", "okay", "confirm", "confirmed", "accept", "accepted", "agree", "agreed", "proceed", "continue", "goahead", "noproblem",
	})
	switch {
	case normalized == "n" || normalized == "no" || negative:
		return false, true
	case normalized == "y" || positive:
		return true, true
	default:
		return false, false
	}
}

func parseApprovalInputWithContext(value string, reason string) (bool, bool) {
	if approve, ok := parseApprovalInput(value); ok {
		return approve, true
	}
	if !approvalReasonAsksForConfirmation(reason) || !looksLikeContextualApproval(value, reason) {
		return false, false
	}
	return true, true
}

func approvalReasonAsksForConfirmation(reason string) bool {
	normalized := semanticApprovalText(reason)
	if normalized == "" {
		return false
	}
	return containsAnySemanticCue(normalized, []string{
		"确认", "核对", "检查", "参数", "目标", "城市", "调用", "工具", "接口", "外部api", "mcp",
		"confirm", "check", "parameter", "target", "city", "call", "tool", "api",
	})
}

func looksLikeContextualApproval(value string, reason string) bool {
	normalized := semanticApprovalText(value)
	if normalized == "" {
		return false
	}
	if containsAnySemanticCue(normalized, []string{
		"不同意", "不批准", "不确认", "不允许", "不要", "别", "拒绝", "取消", "停止", "终止",
		"reject", "deny", "cancel", "stop", "abort", "refuse", "decline", "notapprove", "notagree", "notok",
	}) {
		return false
	}
	if containsAnySemanticCue(normalized, []string{
		"查询", "调用", "使用", "执行", "继续", "城市", "参数", "天气", "接口", "工具", "mcp", "api",
		"query", "call", "use", "execute", "run", "continue", "city", "weather", "parameter", "tool",
	}) {
		return true
	}
	if containsAnySemanticCue(semanticApprovalText(reason), []string{"城市", "city"}) {
		return hasLettersOrDigits(value) && len([]rune(strings.TrimSpace(value))) >= 2
	}
	return false
}

func semanticApprovalText(value string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(value)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func containsAnySemanticCue(value string, cues []string) bool {
	for _, cue := range cues {
		normalizedCue := semanticApprovalText(cue)
		if normalizedCue != "" && strings.Contains(value, normalizedCue) {
			return true
		}
	}
	return false
}

func hasLettersOrDigits(value string) bool {
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

func (m *AppModel) setLastError(err error) {
	if err == nil || isTransientContextError(err) {
		return
	}
	m.lastError = err
}

func isTransientContextError(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

func (m *AppModel) insertInput(value string) {
	if value == "" {
		return
	}
	runes := []rune(m.input)
	m.clampInputCursor(len(runes))
	inserted := []rune(value)
	next := make([]rune, 0, len(runes)+len(inserted))
	next = append(next, runes[:m.inputCursor]...)
	next = append(next, inserted...)
	next = append(next, runes[m.inputCursor:]...)
	m.input = string(next)
	m.inputCursor += len(inserted)
	m.historyCursor = -1
}

func (m *AppModel) deleteBeforeCursor() {
	runes := []rune(m.input)
	m.clampInputCursor(len(runes))
	if m.inputCursor == 0 {
		return
	}
	next := make([]rune, 0, len(runes)-1)
	next = append(next, runes[:m.inputCursor-1]...)
	next = append(next, runes[m.inputCursor:]...)
	m.input = string(next)
	m.inputCursor--
}

func (m *AppModel) deleteAtCursor() {
	runes := []rune(m.input)
	m.clampInputCursor(len(runes))
	if m.inputCursor >= len(runes) {
		return
	}
	next := make([]rune, 0, len(runes)-1)
	next = append(next, runes[:m.inputCursor]...)
	next = append(next, runes[m.inputCursor+1:]...)
	m.input = string(next)
}

func (m *AppModel) moveInputCursor(delta int) {
	runes := []rune(m.input)
	m.inputCursor += delta
	m.clampInputCursor(len(runes))
}

func (m *AppModel) clearInput() {
	m.input = ""
	m.inputCursor = 0
	m.historyCursor = -1
}

func (m *AppModel) clampInputCursor(length int) {
	if m.inputCursor < 0 {
		m.inputCursor = 0
	}
	if m.inputCursor > length {
		m.inputCursor = length
	}
}

func (m AppModel) renderEditableInput() string {
	runes := []rune(m.input)
	cursor := m.inputCursor
	if cursor < 0 {
		cursor = 0
	}
	if cursor > len(runes) {
		cursor = len(runes)
	}
	return string(runes[:cursor]) + "|" + string(runes[cursor:])
}

func (m AppModel) renderInputBuffer(prompt string, width int) string {
	prefix := prompt + " > "
	editable := strings.ReplaceAll(m.renderEditableInput(), "\r\n", "\n")
	lines := strings.Split(editable, "\n")
	if len(lines) == 0 {
		lines = []string{""}
	}
	limit := 4
	if len(lines) > limit {
		lines = append([]string{"..."}, lines[len(lines)-limit+1:]...)
	}
	indent := strings.Repeat(" ", lipgloss.Width(prefix))
	out := make([]string, 0, len(lines))
	for idx, line := range lines {
		linePrefix := indent
		if idx == 0 {
			linePrefix = prefix
		}
		out = append(out, truncateDisplay(linePrefix+line, width-2))
	}
	return strings.Join(out, "\n")
}

func (m *AppModel) addInputHistory(value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	if len(m.inputHistory) > 0 && m.inputHistory[len(m.inputHistory)-1] == value {
		m.historyCursor = -1
		return
	}
	m.inputHistory = append(m.inputHistory, value)
	if len(m.inputHistory) > maxInputHistory {
		m.inputHistory = append([]string(nil), m.inputHistory[len(m.inputHistory)-maxInputHistory:]...)
	}
	m.historyCursor = -1
}

func (m *AppModel) previousInputHistory() {
	if len(m.inputHistory) == 0 {
		return
	}
	if m.historyCursor < 0 {
		m.historyCursor = len(m.inputHistory) - 1
	} else if m.historyCursor > 0 {
		m.historyCursor--
	}
	m.setInput(m.inputHistory[m.historyCursor])
}

func (m *AppModel) nextInputHistory() {
	if len(m.inputHistory) == 0 || m.historyCursor < 0 {
		return
	}
	if m.historyCursor >= len(m.inputHistory)-1 {
		m.historyCursor = -1
		m.setInput("")
		return
	}
	m.historyCursor++
	m.setInput(m.inputHistory[m.historyCursor])
}

func (m *AppModel) setInput(value string) {
	m.input = value
	m.inputCursor = len([]rune(value))
}

func (m *AppModel) completeSlashCommand() {
	trimmed := strings.TrimSpace(m.input)
	if !strings.HasPrefix(trimmed, "/") {
		return
	}
	matches := slashCommandMatches(trimmed)
	if len(matches) > 0 {
		m.setInput(matches[0])
	}
}

func (m AppModel) renderSlashCommandSuggestions(width int) string {
	trimmed := strings.TrimSpace(m.input)
	if !strings.HasPrefix(trimmed, "/") {
		return ""
	}
	matches := slashCommandMatches(trimmed)
	if len(matches) == 0 {
		return ""
	}
	limit := min(4, len(matches))
	lines := make([]string, 0, limit+1)
	lines = append(lines, Dim.Render("Commands:"))
	for idx, command := range matches[:limit] {
		line := "  " + command
		if idx == 0 {
			line = Selection.Render("  " + command + "  ")
		}
		lines = append(lines, truncateDisplay(line, width-2))
	}
	return strings.Join(lines, "\n")
}

func slashCommandMatches(input string) []string {
	trimmed := strings.TrimSpace(input)
	if !strings.HasPrefix(trimmed, "/") {
		return nil
	}
	prefix := strings.ToLower(strings.TrimPrefix(trimmed, "/"))
	commands := []string{"/help", "/clear", "/workspace", "/workspace create", "/dod", "/memory", "/result", "/exit"}
	matches := make([]string, 0, len(commands))
	for _, command := range commands {
		if strings.HasPrefix(strings.TrimPrefix(command, "/"), prefix) {
			matches = append(matches, command)
		}
	}
	return matches
}

func newMainViewport() *viewport.Model {
	vp := viewport.New(0, 0)
	vp.MouseWheelEnabled = false
	return &vp
}

func (m *AppModel) ensureMainViewport() *viewport.Model {
	if m.mainViewport == nil {
		m.mainViewport = newMainViewport()
	}
	m.mainViewport.MouseWheelEnabled = false
	return m.mainViewport
}

func (m *AppModel) resetMainViewport() {
	m.ensureMainViewport().GotoTop()
}

func (m *AppModel) scrollMain(delta int) {
	vp := m.ensureMainViewport()
	if delta < 0 {
		vp.ScrollUp(-delta)
		return
	}
	vp.ScrollDown(delta)
}

func (m *AppModel) setBlockingDecision(id common.TicketID, workspaceID common.WorkspaceID, runID common.RunID, reason string, question string) {
	if id == "" {
		return
	}
	item := backend.DecisionInfo{
		ID:          id,
		WorkspaceID: workspaceID,
		RunID:       runID,
		Reason:      reason,
		Question:    question,
		State:       "open",
	}
	m.blockingDecision = &item
	m.upsertDecision(item)
}

func (m *AppModel) setBlockingApproval(id common.ApprovalID, workspaceID common.WorkspaceID, runID common.RunID, reason string, action string, context map[string]any) {
	if id == "" {
		return
	}
	item := backend.ApprovalInfo{
		ID:          id,
		WorkspaceID: workspaceID,
		RunID:       runID,
		Reason:      reason,
		Action:      action,
		Context:     clonePayloadMap(context),
		State:       "pending",
	}
	m.blockingApproval = &item
	m.upsertApproval(item)
}

func (m *AppModel) fetchActiveBlockerDetails() tea.Cmd {
	var cmds []tea.Cmd
	if m.blockingDecision != nil && m.blockingDecision.ID != "" {
		cmds = append(cmds, FetchDecision(m.be, string(m.blockingDecision.ID)))
	}
	if m.blockingApproval != nil && m.blockingApproval.ID != "" {
		cmds = append(cmds, FetchApproval(m.be, string(m.blockingApproval.ID)))
	}
	if m.activeWorkspaceID != "" {
		cmds = append(cmds, FetchBlockers(m.be, m.activeWorkspaceID))
	}
	return tea.Batch(cmds...)
}

func (m *AppModel) upsertDecision(item backend.DecisionInfo) {
	if item.ID == "" {
		return
	}
	for i := range m.decisions {
		if m.decisions[i].ID == item.ID {
			m.decisions[i] = mergeDecisionInfo(m.decisions[i], item)
			return
		}
	}
	m.decisions = append(m.decisions, item)
}

func (m *AppModel) upsertApproval(item backend.ApprovalInfo) {
	if item.ID == "" {
		return
	}
	for i := range m.approvals {
		if m.approvals[i].ID == item.ID {
			m.approvals[i] = mergeApprovalInfo(m.approvals[i], item)
			return
		}
	}
	m.approvals = append(m.approvals, item)
}

func (m *AppModel) syncBlockingDecisionFromList() {
	if m.blockingDecision != nil {
		for _, item := range m.decisions {
			if item.ID == m.blockingDecision.ID {
				merged := mergeDecisionInfo(*m.blockingDecision, item)
				if merged.State != "open" {
					m.blockingDecision = nil
					return
				}
				m.blockingDecision = &merged
				if m.progress == progressWaitingDecision {
					m.progressText = blockingDecisionLabel(m.blockingDecision)
				}
				return
			}
		}
	}
	for _, item := range filterOpenDecisions(m.decisions) {
		if m.activeRunID == "" || item.RunID == common.RunID(m.activeRunID) {
			m.blockingDecision = &item
			if m.progress == progressWaitingDecision {
				m.progressText = blockingDecisionLabel(m.blockingDecision)
			}
			return
		}
	}
}

func (m *AppModel) syncBlockingApprovalFromList() {
	if m.blockingApproval != nil {
		for _, item := range m.approvals {
			if item.ID == m.blockingApproval.ID {
				merged := mergeApprovalInfo(*m.blockingApproval, item)
				if merged.State != "pending" {
					m.blockingApproval = nil
					return
				}
				m.blockingApproval = &merged
				if m.progress == progressWaitingApproval {
					m.progressText = blockingApprovalLabel(m.blockingApproval)
				}
				return
			}
		}
	}
	for _, item := range filterPendingApprovals(m.approvals) {
		if m.activeRunID == "" || item.RunID == common.RunID(m.activeRunID) {
			m.blockingApproval = &item
			if m.progress == progressWaitingApproval {
				m.progressText = blockingApprovalLabel(m.blockingApproval)
			}
			return
		}
	}
}

func (m *AppModel) markDecisionResolved(id common.TicketID) {
	if id == "" {
		return
	}
	for i := range m.decisions {
		if m.decisions[i].ID == id {
			m.decisions[i].State = "resolved"
			return
		}
	}
}

func (m *AppModel) markApprovalResolved(id common.ApprovalID) {
	if id == "" {
		return
	}
	for i := range m.approvals {
		if m.approvals[i].ID == id {
			m.approvals[i].State = "resolved"
			return
		}
	}
}

func (m *AppModel) applyBackendEvent(evt backend.Event) tea.Cmd {
	switch evt.Type {
	case backend.EventWorkspaceCreated:
		m.upsertWorkspaceFromEvent(evt)
		return m.requestWorkspaceListRefresh()
	case backend.EventWorkspaceDeleted:
		m.removeWorkspace(evt.WorkspaceID)
		if string(evt.WorkspaceID) == m.activeWorkspaceID {
			m.activeWorkspaceID = ""
			m.selectedWorkspaceID = ""
			m.cursor = 0
			m.resetWorkspaceView()
			m.ensureActiveWorkspace()
			m.progressText = "Workspace removed; refreshing state"
		}
		return m.requestWorkspaceListRefresh()
	}
	knownWorkspace := evt.WorkspaceID == "" || workspaceIndex(m.workspaces, string(evt.WorkspaceID)) >= 0
	if evt.WorkspaceID != "" {
		m.upsertWorkspaceFromEvent(evt)
	}
	if evt.WorkspaceID != "" && m.activeWorkspaceID != "" && string(evt.WorkspaceID) != m.activeWorkspaceID {
		if !knownWorkspace {
			return m.requestWorkspaceListRefresh()
		}
		return nil
	}
	if evt.RunID != "" && m.activeRunID != "" && string(evt.RunID) != m.activeRunID {
		return nil
	}
	switch evt.Type {
	case backend.EventWorkspaceStateChanged:
		state := common.WorkspaceState(firstPayloadString(evt.Payload, "state", "State", events.PayloadKeyState))
		activeRunID := firstPayloadString(evt.Payload, "activeRunId", "ActiveRunID", events.PayloadKeyActiveRunID)
		if activeRunID != "" {
			m.activeRunID = activeRunID
		}
		m.applyWorkspaceProgress(state)
		if !knownWorkspace {
			return m.requestWorkspaceListRefresh()
		}
		switch state {
		case common.WorkspaceStateCompleted, common.WorkspaceStateFailed, common.WorkspaceStateWaitingDecision, common.WorkspaceStateBlocked:
			return FetchWorkspaceContext(m.be, m.activeWorkspaceID)
		default:
			return nil
		}
	case backend.EventRunStateChanged:
		state := firstPayloadString(evt.Payload, "state", "State", events.PayloadKeyState)
		if evt.RunID != "" {
			m.activeRunID = string(evt.RunID)
		}
		m.applyRunState(common.RunState(state), evt.Payload)
		return m.fetchActiveBlockerDetails()
	case backend.EventStepDispatched:
		m.progress = progressRunning
		kind := firstPayloadString(evt.Payload, "kind", "Kind", events.PayloadKeyKind)
		m.progressText = stepProgressText("Working", kind, "Working; waiting for final result")
	case backend.EventStepCompleted:
		if state := common.StepState(firstPayloadString(evt.Payload, "state", "State", events.PayloadKeyState)); state == common.StepStateFailed {
			m.rememberFailureHint(failureMessageFromPayload(evt.Payload))
		}
		if m.progress == progressRunning {
			kind := firstPayloadString(evt.Payload, "kind", "Kind", events.PayloadKeyKind)
			m.progressText = stepProgressText("Step completed", kind, "Step completed; waiting for next update")
		}
	case backend.EventDecisionCreated:
		decision := backend.DecisionInfo{
			ID:          common.TicketID(firstPayloadString(evt.Payload, "ticketId", "ID", "id", events.PayloadKeyBlockingTicketID)),
			WorkspaceID: evt.WorkspaceID,
			RunID:       evt.RunID,
			Reason:      firstPayloadString(evt.Payload, "reason", "Reason", events.PayloadKeyReason),
			Question:    firstPayloadString(evt.Payload, "question", "Question"),
			State:       "open",
		}
		m.blockingDecision = &decision
		m.upsertDecision(decision)
		m.running = false
		m.progress = progressWaitingDecision
		m.progressText = nonEmpty(m.blockingDecision.Question, m.blockingDecision.Reason, string(m.blockingDecision.ID))
		return m.fetchActiveBlockerDetails()
	case backend.EventApprovalCreated:
		approval := backend.ApprovalInfo{
			ID:          common.ApprovalID(firstPayloadString(evt.Payload, "approvalId", "ID", "id", events.PayloadKeyBlockingApprovalID)),
			WorkspaceID: evt.WorkspaceID,
			RunID:       evt.RunID,
			Reason:      firstPayloadString(evt.Payload, "reason", "Reason", events.PayloadKeyReason),
			Action:      firstPayloadString(evt.Payload, "action", "Action", events.PayloadKeyAction),
			Context:     payloadMapValue(evt.Payload, "context", "Context", events.PayloadKeyContext),
			State:       "pending",
		}
		m.blockingApproval = &approval
		m.upsertApproval(approval)
		m.running = false
		m.progress = progressWaitingApproval
		m.progressText = nonEmpty(blockingApprovalLabel(m.blockingApproval), string(m.blockingApproval.ID))
		return m.fetchActiveBlockerDetails()
	case backend.EventDecisionResolved:
		m.blockingDecision = nil
		m.progress = progressRunning
		m.progressText = "Decision resolved; resuming run"
		m.markDecisionResolved(common.TicketID(firstPayloadString(evt.Payload, "ticketId", "ID", "id", events.PayloadKeyBlockingTicketID)))
	case backend.EventApprovalResolved:
		m.blockingApproval = nil
		m.progress = progressRunning
		m.progressText = "Approval resolved; resuming run"
		m.markApprovalResolved(common.ApprovalID(firstPayloadString(evt.Payload, "approvalId", "ID", "id", events.PayloadKeyBlockingApprovalID)))
	case backend.EventResultAccepted:
		m.result = firstPayloadString(evt.Payload, "summary", "Summary", events.PayloadKeySummary)
		m.artifacts = artifactPayloadStrings(evt.Payload)
		m.resetMainViewport()
		return FetchWorkspaceContext(m.be, m.activeWorkspaceID)
	case backend.EventMemoryRecallRecorded:
		status := firstPayloadString(evt.Payload, "status", "Status", events.PayloadKeyStatus)
		ids := payloadStringSliceValue(evt.Payload, "retrievedMemoryIds", "RetrievedMemoryIDs", events.PayloadKeyRetrievedMemoryIDs)
		if status == "degraded" {
			m.progressText = "Memory recall: degraded"
		} else {
			m.progressText = fmt.Sprintf("Memory recall: %s %d", nonEmpty(status, "selected"), len(ids))
		}
		return FetchWorkspaceContext(m.be, m.activeWorkspaceID)
	case backend.EventMemoryCreated, backend.EventMemoryUpdated, backend.EventMemoryDeleted:
		m.progressText = "Workspace memory changed; refreshing context"
		return FetchWorkspaceContext(m.be, m.activeWorkspaceID)
	}
	return nil
}

func (m *AppModel) requestWorkspaceListRefresh() tea.Cmd {
	if m.workspaceListRefreshInFlight {
		m.workspaceListRefreshPending = true
		return nil
	}
	m.workspaceListRefreshInFlight = true
	return FetchAll(m.be)
}

func (m *AppModel) applyRunState(state common.RunState, payload map[string]any) {
	switch state {
	case common.RunStateCompleted:
		m.running = false
		m.blockingDecision = nil
		m.blockingApproval = nil
		m.progress = progressCompleted
		m.progressText = "Completed"
	case common.RunStateFailed:
		m.running = false
		m.blockingDecision = nil
		m.blockingApproval = nil
		m.progress = progressFailed
		m.failureReason = nonEmpty(failureMessageFromPayload(payload), m.failureHint)
		m.progressText = nonEmpty(m.failureReason, "Run failed")
	case common.RunStateBlockedOnDecision:
		m.running = false
		m.progress = progressWaitingDecision
		if ticketID := common.TicketID(firstPayloadString(payload, "ticketId", "ID", "id", events.PayloadKeyBlockingTicketID)); ticketID != "" {
			m.setBlockingDecision(ticketID, common.WorkspaceID(m.activeWorkspaceID), common.RunID(m.activeRunID), "", "")
		}
		m.progressText = nonEmpty(blockingDecisionLabel(m.blockingDecision), "Waiting for decision")
	case common.RunStateBlockedOnApproval:
		m.running = false
		m.progress = progressWaitingApproval
		if approvalID := common.ApprovalID(firstPayloadString(payload, "approvalId", "ID", "id", events.PayloadKeyBlockingApprovalID)); approvalID != "" {
			m.setBlockingApproval(approvalID, common.WorkspaceID(m.activeWorkspaceID), common.RunID(m.activeRunID), "", "", nil)
		}
		m.progressText = nonEmpty(blockingApprovalLabel(m.blockingApproval), "Waiting for approval")
	default:
		if state != "" {
			m.running = true
			m.progress = progressRunning
			if state == common.RunStateRunning {
				m.progressText = "Running; waiting for progress or final result"
			} else {
				m.progressText = string(state) + "; waiting for progress or final result"
			}
		}
	}
}

func (m *AppModel) applyRunStarted(run *backend.RunInfo) {
	if run == nil {
		return
	}
	switch run.State {
	case common.RunStateCompleted, common.RunStateFailed, common.RunStateBlockedOnDecision, common.RunStateBlockedOnApproval:
		m.applyRunState(run.State, map[string]any{
			events.PayloadKeyFailureReason:      string(run.FailureReason),
			events.PayloadKeyBlockingTicketID:   string(run.BlockingTicketID),
			events.PayloadKeyBlockingApprovalID: string(run.BlockingApprovalID),
		})
	case "":
		if m.progress == progressIdle || m.progress == progressSubmitting {
			m.progress = progressRunning
			m.progressText = "Run started; waiting for progress or final result"
		}
	default:
		if m.progress == progressIdle || m.progress == progressSubmitting {
			m.progress = progressRunning
			if strings.TrimSpace(m.progressText) == "" {
				m.progressText = "Run started; waiting for progress or final result"
			}
		}
	}
}

func (m AppModel) emptyResultText() string {
	if len(m.workspaces) == 0 && strings.TrimSpace(m.activeWorkspaceID) == "" {
		return "No workspaces found.\n\nCreate one from this TUI:\n  /workspace create <title> --dir <path>\n  /workspace create <title> --template generic-governed --dir <path>\n  /workspace create <title> --template software-dev --dir <path>\n\nUseful commands:\n  /help     Show commands\n  /dod      Set acceptance criteria\n  /exit     Quit"
	}
	switch m.progress {
	case progressSubmitting:
		return "Submitting goal. Waiting for backend acknowledgment..."
	case progressRunning:
		return "Run is active. Long tasks may stay quiet while backend works; final result will appear here."
	case progressWaitingDecision:
		return "Answer the decision prompt below before a result can be produced."
	case progressWaitingApproval:
		return "Approve or reject below before a result can be produced."
	case progressInterrupted:
		return "Run was interrupted before a final result was produced."
	case progressFailed:
		return "Run failed before a final result was accepted."
	default:
		return "No result yet. Submit a goal from the input area below."
	}
}

func (m *AppModel) upsertWorkspaceFromEvent(evt backend.Event) {
	state := common.WorkspaceState(firstPayloadString(evt.Payload, "state", "State", events.PayloadKeyState))
	activeRunID := common.RunID(firstPayloadString(evt.Payload, "activeRunId", "ActiveRunID", events.PayloadKeyActiveRunID))
	title := firstPayloadString(evt.Payload, "title", "Title", events.PayloadKeyTitle)
	description := firstPayloadString(evt.Payload, "description", "Description", events.PayloadKeyDescription)
	workingDirectory := firstPayloadString(evt.Payload, "workingDirectory", "WorkingDirectory", events.PayloadKeyWorkingDirectory)
	for i := range m.workspaces {
		if m.workspaces[i].ID == evt.WorkspaceID {
			if title != "" {
				m.workspaces[i].Title = title
			}
			if description != "" {
				m.workspaces[i].Description = description
			}
			if workingDirectory != "" {
				m.workspaces[i].WorkingDirectory = workingDirectory
			}
			if state != "" {
				m.workspaces[i].State = state
			}
			if activeRunID != "" || hasPayloadKey(evt.Payload, events.PayloadKeyActiveRunID, "ActiveRunID", "activeRunId") {
				m.workspaces[i].ActiveRunID = activeRunID
			}
			return
		}
	}
	m.workspaces = append(m.workspaces, backend.WorkspaceInfo{
		ID:               evt.WorkspaceID,
		Title:            title,
		Description:      description,
		WorkingDirectory: workingDirectory,
		State:            state,
		ActiveRunID:      activeRunID,
	})
}

func (m *AppModel) removeWorkspace(id common.WorkspaceID) {
	if id == "" {
		return
	}
	next := m.workspaces[:0]
	for _, ws := range m.workspaces {
		if ws.ID != id {
			next = append(next, ws)
		}
	}
	m.workspaces = next
	if len(m.workspaces) == 0 {
		m.activeWorkspaceID = ""
		m.selectedWorkspaceID = ""
		m.cursor = 0
		return
	}
	if m.cursor >= len(m.workspaces) {
		m.cursor = len(m.workspaces) - 1
	}
}

func (m *AppModel) ensureActiveWorkspace() {
	if len(m.workspaces) == 0 {
		return
	}
	if m.activeWorkspaceID == "" {
		m.activeWorkspaceID = m.defaultWorkspaceID
	}
	if m.workspaceSelectMode {
		if idx := workspaceIndex(m.workspaces, m.selectedWorkspaceID); idx >= 0 {
			m.cursor = idx
			return
		}
	}
	if idx := workspaceIndex(m.workspaces, m.activeWorkspaceID); idx >= 0 {
		m.selectedWorkspaceID = m.activeWorkspaceID
		m.cursor = idx
		return
	}
	if idx := workspaceIndex(m.workspaces, m.defaultWorkspaceID); idx >= 0 {
		m.activeWorkspaceID = m.defaultWorkspaceID
		m.selectedWorkspaceID = m.defaultWorkspaceID
		m.cursor = idx
		return
	}
	m.activeWorkspaceID = string(m.workspaces[0].ID)
	m.selectedWorkspaceID = m.activeWorkspaceID
	m.cursor = 0
}

func (m *AppModel) nextWorkspace() {
	if len(m.workspaces) == 0 {
		return
	}
	idx := m.selectedWorkspaceIndex()
	idx = (idx + 1) % len(m.workspaces)
	m.cursor = idx
	m.selectedWorkspaceID = string(m.workspaces[idx].ID)
}

func (m *AppModel) prevWorkspace() {
	if len(m.workspaces) == 0 {
		return
	}
	idx := m.selectedWorkspaceIndex()
	idx--
	if idx < 0 {
		idx = len(m.workspaces) - 1
	}
	m.cursor = idx
	m.selectedWorkspaceID = string(m.workspaces[idx].ID)
}

func (m *AppModel) setActiveWorkspace(id string) {
	if id == "" || id == m.activeWorkspaceID {
		return
	}
	m.activeWorkspaceID = id
	m.selectedWorkspaceID = id
	if idx := workspaceIndex(m.workspaces, id); idx >= 0 {
		m.cursor = idx
	}
	if m.options.OnActiveWorkspaceChanged != nil {
		m.options.OnActiveWorkspaceChanged(id)
	}
}

func (m *AppModel) toggleWorkspaceSelectMode() {
	if m.workspaceSelectMode {
		m.cancelWorkspaceSelection()
		return
	}
	m.workspaceSelectMode = true
	m.selectedWorkspaceID = m.activeWorkspaceID
	if idx := workspaceIndex(m.workspaces, m.selectedWorkspaceID); idx >= 0 {
		m.cursor = idx
	}
}

func (m *AppModel) cancelWorkspaceSelection() {
	m.workspaceSelectMode = false
	m.selectedWorkspaceID = m.activeWorkspaceID
	if idx := workspaceIndex(m.workspaces, m.activeWorkspaceID); idx >= 0 {
		m.cursor = idx
	}
}

func (m *AppModel) selectedWorkspaceIndex() int {
	if idx := workspaceIndex(m.workspaces, m.selectedWorkspaceID); idx >= 0 {
		return idx
	}
	if m.cursor >= 0 && m.cursor < len(m.workspaces) {
		return m.cursor
	}
	if idx := workspaceIndex(m.workspaces, m.activeWorkspaceID); idx >= 0 {
		return idx
	}
	return 0
}

func (m *AppModel) confirmWorkspaceSelection() (tea.Model, tea.Cmd) {
	if m.selectedWorkspaceID == "" {
		m.workspaceSelectMode = false
		return m, nil
	}
	if m.selectedWorkspaceID == m.activeWorkspaceID {
		m.workspaceSelectMode = false
		return m, nil
	}
	m.setActiveWorkspace(m.selectedWorkspaceID)
	m.workspaceSelectMode = false
	m.resetWorkspaceView()
	m.progressText = "Workspace switched; refreshing state"
	return m, FetchAll(m.be)
}

func (m *AppModel) applyWorkspaceContext(msg WorkspaceContextMsg) {
	if msg.Workspace == nil {
		return
	}
	workspaceID := string(msg.Workspace.ID)
	if workspaceID == "" || workspaceID != m.activeWorkspaceID {
		return
	}
	m.upsertWorkspace(*msg.Workspace)
	m.resetWorkspaceView()
	m.decisions = append([]backend.DecisionInfo(nil), msg.Decisions...)
	m.approvals = append([]backend.ApprovalInfo(nil), msg.Approvals...)

	run := msg.ActiveRun
	if run == nil {
		m.applyWorkspaceProgress(msg.Workspace.State)
		m.syncBlockingDecisionFromList()
		m.syncBlockingApprovalFromList()
		return
	}
	m.activeRunID = string(run.ID)
	if msg.Goal != nil && strings.TrimSpace(msg.Goal.Text) != "" {
		m.currentTask = strings.TrimSpace(msg.Goal.Text)
		m.transcript = initialTranscript(msg.Goal.Text)
	}
	if msg.Snapshot != nil {
		m.result = snapshotDisplayResult(msg.Snapshot, run.ID)
		m.artifacts = snapshotArtifactLabels(msg.Snapshot, run.ID)
		m.memoryRecalls = append([]backend.MemoryRecallTraceInfo(nil), msg.Snapshot.MemoryRecallTraces...)
	}

	state := run.State
	payload := map[string]any{
		events.PayloadKeyFailureReason:      run.FailureReason,
		events.PayloadKeyBlockingTicketID:   string(run.BlockingTicketID),
		events.PayloadKeyBlockingApprovalID: string(run.BlockingApprovalID),
	}
	if msg.Status != nil {
		state = msg.Status.State
		payload[events.PayloadKeyFailureReason] = msg.Status.FailureReason
		payload[events.PayloadKeyBlockingTicketID] = string(msg.Status.BlockingTicketID)
		payload[events.PayloadKeyBlockingApprovalID] = string(msg.Status.BlockingApprovalID)
	}
	if msg.Snapshot != nil && payload[events.PayloadKeyFailureReason] == "" {
		payload[events.PayloadKeyFailureReason] = msg.Snapshot.Outcome.Reason
	}
	m.applyRunState(state, payload)
	if msg.Status != nil && msg.Status.CurrentStepKind != "" && m.progress == progressRunning {
		m.progressText = stepProgressText("Working", msg.Status.CurrentStepKind, "Working; waiting for final result")
	}
	m.syncBlockingDecisionFromList()
	m.syncBlockingApprovalFromList()
}

func (m *AppModel) resetWorkspaceView() {
	m.activeRunID = ""
	m.currentTask = ""
	m.transcript = nil
	m.result = ""
	m.artifacts = nil
	m.memoryRecalls = nil
	m.failureReason = ""
	m.failureHint = ""
	m.blockingDecision = nil
	m.blockingApproval = nil
	m.decisions = nil
	m.approvals = nil
	m.running = false
	m.progress = progressIdle
	m.progressText = ""
	m.lastError = nil
	m.resetMainViewport()
	m.clearInput()
}

func (m *AppModel) upsertWorkspace(ws backend.WorkspaceInfo) {
	for i := range m.workspaces {
		if m.workspaces[i].ID == ws.ID {
			m.workspaces[i] = ws
			return
		}
	}
	m.workspaces = append(m.workspaces, ws)
}

func (m *AppModel) applyWorkspaceProgress(state common.WorkspaceState) {
	switch state {
	case common.WorkspaceStateRunning, common.WorkspaceStateValidating:
		m.running = true
		m.progress = progressRunning
		m.progressText = string(state) + "; waiting for progress or final result"
	case common.WorkspaceStateWaitingDecision, common.WorkspaceStateBlocked:
		m.progress = progressWaitingDecision
		m.progressText = "Waiting for workspace input"
	case common.WorkspaceStateCompleted:
		m.progress = progressCompleted
		m.progressText = "Completed"
	case common.WorkspaceStateFailed:
		m.progress = progressFailed
		m.progressText = "Workspace failed"
	default:
		m.progress = progressIdle
		m.progressText = ""
	}
}

func findWorkspace(workspaces []backend.WorkspaceInfo, id string) *backend.WorkspaceInfo {
	for i := range workspaces {
		if string(workspaces[i].ID) == id {
			return &workspaces[i]
		}
	}
	return nil
}

func workspaceIndex(workspaces []backend.WorkspaceInfo, id string) int {
	for i := range workspaces {
		if string(workspaces[i].ID) == id {
			return i
		}
	}
	return -1
}

func joinColumns(left, right string, leftWidth int) string {
	leftLines := strings.Split(strings.TrimRight(left, "\n"), "\n")
	rightLines := strings.Split(strings.TrimRight(right, "\n"), "\n")
	lineCount := len(leftLines)
	if len(rightLines) > lineCount {
		lineCount = len(rightLines)
	}
	var b strings.Builder
	for i := 0; i < lineCount; i++ {
		l := ""
		if i < len(leftLines) {
			l = leftLines[i]
		}
		r := ""
		if i < len(rightLines) {
			r = rightLines[i]
		}
		padding := leftWidth - lipgloss.Width(l)
		if padding < 1 {
			padding = 1
		}
		b.WriteString(l)
		b.WriteString(strings.Repeat(" ", padding))
		b.WriteString(r)
		b.WriteString("\n")
	}
	return b.String()
}

func visibleWorkspaces(items []backend.WorkspaceInfo, cursor, limit int) []backend.WorkspaceInfo {
	if limit <= 0 || len(items) <= limit {
		return items
	}
	if cursor < 0 || cursor >= len(items) {
		cursor = 0
	}
	start := cursor - limit/2
	if start < 0 {
		start = 0
	}
	if start+limit > len(items) {
		start = len(items) - limit
	}
	return items[start : start+limit]
}

func workspaceLabel(ws backend.WorkspaceInfo) string {
	name := workspaceName(ws)
	if name != "" && name != shortID(string(ws.ID)) {
		return name + " (" + shortID(string(ws.ID)) + ")"
	}
	return shortID(string(ws.ID))
}

func workspaceName(ws backend.WorkspaceInfo) string {
	if strings.TrimSpace(ws.Title) != "" {
		return strings.TrimSpace(ws.Title)
	}
	if strings.TrimSpace(ws.WorkingDirectory) != "" {
		parts := strings.Split(strings.TrimRight(ws.WorkingDirectory, "/"), "/")
		return parts[len(parts)-1]
	}
	return shortID(string(ws.ID))
}

func workspaceNameCounts(items []backend.WorkspaceInfo) map[string]int {
	counts := map[string]int{}
	for _, ws := range items {
		counts[workspaceName(ws)]++
	}
	return counts
}

func workspacePanelLine(prefix string, ws backend.WorkspaceInfo, counts map[string]int, width int) string {
	name := workspaceName(ws)
	state := string(ws.State)
	if strings.TrimSpace(state) == "" {
		state = "unknown"
	}
	id := shortID(string(ws.ID))
	suffix := "  " + state
	if counts[name] > 1 || name == id {
		suffix = " " + id + suffix
	}
	nameWidth := width - lipgloss.Width(prefix) - lipgloss.Width(suffix)
	if nameWidth < 4 {
		return truncateDisplay(prefix+id+"  "+state, width)
	}
	return prefix + truncateDisplay(name, nameWidth) + suffix
}

func workspaceStatusLine(ws backend.WorkspaceInfo) string {
	state := string(ws.State)
	if strings.TrimSpace(state) == "" {
		state = "unknown"
	}
	return shortID(string(ws.ID)) + "  " + state
}

func progressLabel(state progressState) string {
	switch state {
	case progressIdle:
		return "Idle"
	case progressSubmitting:
		return "Submitting"
	case progressRunning:
		return "Running"
	case progressWaitingDecision:
		return "Waiting for decision"
	case progressWaitingApproval:
		return "Waiting for approval"
	case progressCompleted:
		return "Completed"
	case progressFailed:
		return "Failed"
	case progressInterrupted:
		return "Interrupted"
	default:
		return string(state)
	}
}

func firstPayloadString(payload map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := payload[key].(string); ok && value != "" {
			return value
		}
	}
	return ""
}

func payloadStringSliceValue(payload map[string]any, keys ...string) []string {
	for _, key := range keys {
		value, ok := payload[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case []string:
			return compactStrings(typed)
		case []common.MemoryID:
			ret := make([]string, 0, len(typed))
			for _, item := range typed {
				ret = append(ret, string(item))
			}
			return compactStrings(ret)
		case []any:
			ret := make([]string, 0, len(typed))
			for _, item := range typed {
				if text := strings.TrimSpace(fmt.Sprint(item)); text != "" {
					ret = append(ret, text)
				}
			}
			return compactStrings(ret)
		case string:
			if strings.TrimSpace(typed) != "" {
				return []string{strings.TrimSpace(typed)}
			}
		}
	}
	return nil
}

func payloadMap(payload map[string]any, keys ...string) map[string]any {
	for _, key := range keys {
		if value, ok := payload[key].(map[string]any); ok {
			return value
		}
	}
	return nil
}

func payloadMapValue(payload map[string]any, keys ...string) map[string]any {
	return clonePayloadMap(payloadMap(payload, keys...))
}

func clonePayloadMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	ret := make(map[string]any, len(input))
	for key, value := range input {
		ret[key] = value
	}
	return ret
}

func (m *AppModel) rememberFailureHint(value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	if m.failureHint == "" || !isTransientFailureText(value) {
		m.failureHint = value
	}
}

func failureMessageFromPayload(payload map[string]any) string {
	if payload == nil {
		return ""
	}
	if output := payloadMap(payload, "output", "Output", events.PayloadKeyOutput); output != nil {
		if value := failureMessageFromPayload(output); value != "" {
			return value
		}
	}
	reason := firstPayloadString(payload, "reason", "Reason", "error", "Error", "failureReason", "FailureReason", events.PayloadKeyReason, events.PayloadKeyFailureReason)
	if sshMissingCredentialFailure(reason) {
		return "SSH 命令缺少 password 或 privateKey/privateKeyPath，请补充凭证后重试。"
	}
	if isTransientFailureText(reason) {
		if hint := firstPayloadString(payload, "resolutionHint", "ResolutionHint"); strings.TrimSpace(hint) != "" {
			return "LLM provider timeout. " + strings.TrimSpace(hint)
		}
		return ""
	}
	return strings.TrimSpace(reason)
}

func sshMissingCredentialFailure(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	return strings.Contains(normalized, "ssh/command") &&
		strings.Contains(normalized, "requires password") &&
		strings.Contains(normalized, "privatekey")
}

func isTransientFailureText(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" {
		return false
	}
	return strings.Contains(normalized, "context deadline exceeded") ||
		strings.Contains(normalized, "context cancellation while reading body") ||
		strings.Contains(normalized, "client.timeout")
}

func hasPayloadKey(payload map[string]any, keys ...string) bool {
	for _, key := range keys {
		if _, ok := payload[key]; ok {
			return true
		}
	}
	return false
}

func artifactPayloadStrings(payload map[string]any) []string {
	for _, key := range []string{"artifacts", "artifactRefs", "artifactReferences", "ArtifactReferences"} {
		value, ok := payload[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case []string:
			return typed
		case []any:
			out := make([]string, 0, len(typed))
			for _, item := range typed {
				switch v := item.(type) {
				case string:
					out = append(out, v)
				case map[string]any:
					out = append(out, nonEmpty(firstPayloadString(v, "name", "Name"), firstPayloadString(v, "path", "Path"), firstPayloadString(v, "id", "ID")))
				}
			}
			return compactStrings(out)
		case string:
			if strings.TrimSpace(typed) != "" {
				return []string{typed}
			}
		}
	}
	return nil
}

func snapshotResultSummary(snapshot *backend.WorkspaceSnapshotInfo, runID common.RunID) string {
	if snapshot == nil {
		return ""
	}
	if snapshot.Outcome.Summary != "" && (runID == "" || snapshot.Outcome.RunID == "" || snapshot.Outcome.RunID == runID) {
		return snapshot.Outcome.Summary
	}
	for i := len(snapshot.Results) - 1; i >= 0; i-- {
		item := snapshot.Results[i]
		if runID != "" && item.RunID != runID {
			continue
		}
		if item.Summary != "" {
			return item.Summary
		}
		if item.SummaryDetail != nil {
			if item.SummaryDetail.GoalOutcome != "" {
				return item.SummaryDetail.GoalOutcome
			}
			if item.SummaryDetail.ImplementationSummary != "" {
				return item.SummaryDetail.ImplementationSummary
			}
			if item.SummaryDetail.VerificationSummary != "" {
				return item.SummaryDetail.VerificationSummary
			}
		}
	}
	return ""
}

func snapshotDisplayResult(snapshot *backend.WorkspaceSnapshotInfo, runID common.RunID) string {
	if snapshot == nil {
		return ""
	}
	if value := finalSummaryArtifactDisplay(snapshot.Artifacts, runID); value != "" {
		return value
	}
	return snapshotResultSummary(snapshot, runID)
}

func finalSummaryArtifactDisplay(items []backend.ArtifactInfo, runID common.RunID) string {
	for i := len(items) - 1; i >= 0; i-- {
		item := items[i]
		if runID != "" && item.RunID != "" && item.RunID != runID {
			continue
		}
		if item.Summary == "" {
			continue
		}
		category := strings.TrimSpace(strings.ToLower(item.Category))
		isFinalSummary := category == "final_summary" ||
			strings.Contains(strings.ToLower(string(item.ID)), "/artifact/result-summary") ||
			strings.EqualFold(item.Name, "Accepted Result Summary")
		if !isFinalSummary {
			continue
		}
		return compactFinalSummaryForTUI(item.Summary)
	}
	return ""
}

func compactFinalSummaryForTUI(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	stdout := markdownCodeBlockAfterLabel(value, []string{"标准输出:", "Stdout:", "stdout:"})
	if stdout == "" {
		return stripTopMarkdownHeading(value)
	}
	var parts []string
	if goal := markdownSectionText(value, []string{"目标成果", "Goal Outcome", "Outcome"}); goal != "" {
		parts = append(parts, goal)
	}
	if verification := markdownSectionText(value, []string{"验证摘要", "Verification Summary", "Verification"}); verification != "" {
		parts = append(parts, verification)
	}
	parts = append(parts, "标准输出:\n"+compactEvidenceStdout(stdout))
	return strings.Join(parts, "\n\n")
}

func markdownSectionText(value string, headings []string) string {
	lines := strings.Split(strings.ReplaceAll(value, "\r\n", "\n"), "\n")
	for i, line := range lines {
		level, title, ok := markdownHeading(line)
		if !ok || level != 2 || !matchesHeading(title, headings) {
			continue
		}
		var section []string
		for _, next := range lines[i+1:] {
			nextLevel, _, nextHeading := markdownHeading(next)
			if nextHeading && nextLevel <= level {
				break
			}
			section = append(section, next)
		}
		return trimBlankLines(strings.Join(section, "\n"))
	}
	return ""
}

func markdownCodeBlockAfterLabel(value string, labels []string) string {
	normalized := strings.ReplaceAll(value, "\r\n", "\n")
	start := -1
	lower := strings.ToLower(normalized)
	for _, label := range labels {
		idx := strings.Index(lower, strings.ToLower(label))
		if idx >= 0 && (start < 0 || idx < start) {
			start = idx + len(label)
		}
	}
	if start < 0 {
		return ""
	}
	rest := normalized[start:]
	fence := strings.Index(rest, "```")
	if fence < 0 {
		return ""
	}
	rest = rest[fence+3:]
	if newline := strings.Index(rest, "\n"); newline >= 0 {
		rest = rest[newline+1:]
	}
	end := strings.Index(rest, "```")
	if end < 0 {
		return strings.TrimSpace(rest)
	}
	return strings.TrimSpace(rest[:end])
}

func compactEvidenceStdout(value string) string {
	lines := compactEvidenceLines(strings.Split(strings.ReplaceAll(value, "\r\n", "\n"), "\n"))
	if len(lines) == 0 {
		return ""
	}
	sections := splitEvidenceSections(lines)
	if len(sections) > 1 {
		var out []string
		for idx, section := range sections {
			if idx > 0 {
				out = append(out, "---")
			}
			out = append(out, takeEvidenceSectionLines(section, 10)...)
		}
		return strings.Join(out, "\n")
	}
	if len(lines) <= 28 {
		return strings.Join(lines, "\n")
	}
	out := append([]string{}, lines[:20]...)
	out = append(out, "... omitted ...")
	out = append(out, lines[len(lines)-6:]...)
	return strings.Join(out, "\n")
}

func compactEvidenceLines(lines []string) []string {
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		lower := strings.ToLower(trimmed)
		if strings.HasPrefix(lower, "flags:") || strings.HasPrefix(lower, "vulnerability ") {
			continue
		}
		out = append(out, strings.TrimRight(line, " \t"))
	}
	return out
}

func splitEvidenceSections(lines []string) [][]string {
	var sections [][]string
	var current []string
	for _, line := range lines {
		if strings.TrimSpace(line) == "---" {
			sections = append(sections, current)
			current = nil
			continue
		}
		current = append(current, line)
	}
	sections = append(sections, current)
	out := sections[:0]
	for _, section := range sections {
		if len(section) > 0 {
			out = append(out, section)
		}
	}
	return out
}

func takeEvidenceSectionLines(lines []string, limit int) []string {
	if limit <= 0 || len(lines) <= limit {
		return lines
	}
	out := append([]string{}, lines[:limit]...)
	out = append(out, "...")
	return out
}

func stripTopMarkdownHeading(value string) string {
	lines := strings.Split(strings.ReplaceAll(value, "\r\n", "\n"), "\n")
	if len(lines) == 0 {
		return ""
	}
	if level, _, ok := markdownHeading(lines[0]); ok && level == 1 {
		return trimBlankLines(strings.Join(lines[1:], "\n"))
	}
	return value
}

func markdownHeading(line string) (int, string, bool) {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "#") {
		return 0, "", false
	}
	level := 0
	for _, r := range trimmed {
		if r != '#' {
			break
		}
		level++
	}
	if level == 0 || level > 6 || len(trimmed) <= level || trimmed[level] != ' ' {
		return 0, "", false
	}
	return level, strings.TrimSpace(trimmed[level:]), true
}

func matchesHeading(title string, candidates []string) bool {
	title = strings.TrimSpace(strings.ToLower(title))
	for _, candidate := range candidates {
		if title == strings.TrimSpace(strings.ToLower(candidate)) {
			return true
		}
	}
	return false
}

func trimBlankLines(value string) string {
	lines := strings.Split(value, "\n")
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	return strings.Join(lines, "\n")
}

func snapshotArtifactLabels(snapshot *backend.WorkspaceSnapshotInfo, runID common.RunID) []string {
	if snapshot == nil {
		return nil
	}
	out := make([]string, 0, len(snapshot.Artifacts)+len(snapshot.Outcome.ArtifactRefs))
	seen := map[string]struct{}{}
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		if _, ok := seen[value]; ok {
			return
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	for _, item := range snapshot.Artifacts {
		if runID != "" && item.RunID != "" && item.RunID != runID {
			continue
		}
		add(nonEmpty(string(item.ID), item.Name, item.URI, item.Summary))
	}
	for _, ref := range snapshot.Outcome.ArtifactRefs {
		add(ref)
	}
	for _, result := range snapshot.Results {
		if runID != "" && result.RunID != "" && result.RunID != runID {
			continue
		}
		for _, ref := range result.ArtifactRefs {
			add(ref)
		}
		if result.SummaryDetail != nil {
			for _, ref := range result.SummaryDetail.ArtifactRefs {
				add(ref)
			}
		}
	}
	return out
}

func compactStrings(items []string) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item) != "" {
			out = append(out, item)
		}
	}
	return out
}

func nonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func stepProgressText(prefix string, kind string, fallback string) string {
	if strings.TrimSpace(kind) == "" {
		return fallback
	}
	if strings.TrimSpace(prefix) == "" {
		return kind
	}
	return prefix + ": " + kind
}

func blockingDecisionLabel(item *backend.DecisionInfo) string {
	if item == nil {
		return ""
	}
	return nonEmpty(item.Question, item.Reason, string(item.ID))
}

func blockingApprovalLabel(item *backend.ApprovalInfo) string {
	if item == nil {
		return ""
	}
	if strings.TrimSpace(item.Action) == "" {
		return nonEmpty(item.Reason, string(item.ID))
	}
	if strings.TrimSpace(item.Reason) == "" {
		return item.Action
	}
	return item.Reason + " | " + item.Action
}

func mergeDecisionInfo(existing backend.DecisionInfo, next backend.DecisionInfo) backend.DecisionInfo {
	if next.WorkspaceID != "" {
		existing.WorkspaceID = next.WorkspaceID
	}
	if next.RunID != "" {
		existing.RunID = next.RunID
	}
	if next.Reason != "" {
		existing.Reason = next.Reason
	}
	if next.Question != "" {
		existing.Question = next.Question
	}
	if next.State != "" {
		existing.State = next.State
	}
	if next.ResolutionType != "" {
		existing.ResolutionType = next.ResolutionType
	}
	return existing
}

func mergeApprovalInfo(existing backend.ApprovalInfo, next backend.ApprovalInfo) backend.ApprovalInfo {
	if next.WorkspaceID != "" {
		existing.WorkspaceID = next.WorkspaceID
	}
	if next.RunID != "" {
		existing.RunID = next.RunID
	}
	if next.Reason != "" {
		existing.Reason = next.Reason
	}
	if next.Action != "" {
		existing.Action = next.Action
	}
	if next.Context != nil {
		existing.Context = clonePayloadMap(next.Context)
	}
	if next.State != "" {
		existing.State = next.State
	}
	return existing
}

func renderRichText(value string, width int, maxLines int) string {
	if width <= 0 || maxLines <= 0 {
		return ""
	}
	if useGlamourMarkdownRenderer() {
		if rendered, ok := renderGlamourRichText(value, width, maxLines); ok {
			return rendered
		}
	}
	var out []string
	inCode := false
	for _, raw := range strings.Split(strings.ReplaceAll(value, "\r\n", "\n"), "\n") {
		trimmed := strings.TrimSpace(raw)
		if strings.HasPrefix(trimmed, "```") {
			inCode = !inCode
			if inCode {
				out = append(out, Dim.Render("  Code"))
			}
			if len(out) >= maxLines {
				return strings.Join(out[:maxLines], "\n")
			}
			continue
		}
		line := raw
		if !inCode {
			line = renderMarkdownLine(raw)
		} else if strings.TrimSpace(line) != "" {
			line = "  | " + line
		}
		for _, wrapped := range splitDisplayLine(line, width) {
			if inCode && strings.TrimSpace(wrapped) != "" {
				out = append(out, Dim.Render(wrapped))
			} else {
				out = append(out, wrapped)
			}
			if len(out) >= maxLines {
				return strings.Join(out, "\n")
			}
		}
	}
	return strings.Join(out, "\n")
}

func renderMarkdownLine(value string) string {
	trimmed := strings.TrimSpace(value)
	if level, title, ok := markdownHeading(trimmed); ok {
		prefix := strings.Repeat("#", level) + " "
		return Heading.Render(prefix + title)
	}
	if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") {
		return "  " + trimmed
	}
	return stripInlineMarkdown(value)
}

func stripInlineMarkdown(value string) string {
	replacements := []string{"**", "__", "~~", "`"}
	for _, old := range replacements {
		value = strings.ReplaceAll(value, old, "")
	}
	return value
}

func splitDisplayLine(value string, width int) []string {
	if lipgloss.Width(value) <= width {
		return []string{value}
	}
	var lines []string
	remaining := value
	for lipgloss.Width(remaining) > width {
		part := takeDisplayPrefix(remaining, width)
		lines = append(lines, part)
		remaining = strings.TrimPrefix(remaining, part)
	}
	lines = append(lines, remaining)
	return lines
}

func wrapLines(value string, width int, maxLines int) string {
	if width <= 0 {
		return ""
	}
	var lines []string
	for _, raw := range strings.Split(value, "\n") {
		remaining := raw
		for lipgloss.Width(remaining) > width {
			part := takeDisplayPrefix(remaining, width)
			lines = append(lines, part)
			remaining = strings.TrimPrefix(remaining, part)
			if len(lines) >= maxLines {
				return strings.Join(lines, "\n")
			}
		}
		lines = append(lines, remaining)
		if len(lines) >= maxLines {
			break
		}
	}
	return strings.Join(lines, "\n")
}

func truncateDisplay(value string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
	if lipgloss.Width(value) <= maxWidth {
		return value
	}
	if maxWidth <= 3 {
		return takeDisplayPrefix(value, maxWidth)
	}
	return takeDisplayPrefix(value, maxWidth-3) + "..."
}

func takeDisplayPrefix(value string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
	var b strings.Builder
	for _, r := range value {
		next := b.String() + string(r)
		if lipgloss.Width(next) > maxWidth {
			break
		}
		b.WriteRune(r)
	}
	return b.String()
}

func shortID(id string) string {
	if id == "" {
		return "none"
	}
	if len(id) <= 10 {
		return id
	}
	return id[:10]
}

func lineCount(value string) int {
	if value == "" {
		return 0
	}
	return strings.Count(strings.TrimRight(value, "\n"), "\n") + 1
}

func padLines(value string, target int) string {
	value = strings.TrimRight(value, "\n")
	count := lineCount(value)
	if count >= target {
		return value
	}
	return value + strings.Repeat("\n", target-count)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
