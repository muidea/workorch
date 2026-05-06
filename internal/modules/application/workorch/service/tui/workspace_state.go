package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/muidea/workorch/internal/modules/application/workorch/biz/backend"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

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
		m.sources = snapshotWebSourceLabels(msg.Snapshot, run.ID)
		m.patches = snapshotPatches(msg.Snapshot, run.ID)
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
	m.patches = nil
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
