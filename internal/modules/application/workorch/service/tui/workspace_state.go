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
	deliverableDirectory := firstPayloadString(evt.Payload, "deliverableDirectory", "DeliverableDirectory", events.PayloadKeyDeliverableDirectory)
	for i := range m.workspaces {
		if m.workspaces[i].ID == evt.WorkspaceID {
			if title != "" {
				m.workspaces[i].Title = title
			}
			if description != "" {
				m.workspaces[i].Description = description
			}
			if deliverableDirectory != "" {
				m.workspaces[i].DeliverableDirectory = deliverableDirectory
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
		ID:                   evt.WorkspaceID,
		Title:                title,
		Description:          description,
		DeliverableDirectory: deliverableDirectory,
		State:                state,
		ActiveRunID:          activeRunID,
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
	goalText := effectiveGoalText(run)
	if goalText == "" && msg.Goal != nil {
		goalText = strings.TrimSpace(msg.Goal.Text)
	}
	if goalText != "" {
		m.currentTask = goalText
		m.transcript = effectiveGoalTranscript(run, goalText)
	}
	if items := effectiveDoDItems(run); len(items) > 0 {
		m.dodItems = items
	} else if msg.DoD != nil && len(msg.DoD.Items) > 0 {
		m.dodItems = append([]string(nil), msg.DoD.Items...)
	}
	if msg.Snapshot != nil {
		m.result = snapshotDisplayResult(msg.Snapshot, run.ID)
		m.artifacts = snapshotArtifactLabels(msg.Snapshot, run.ID)
		m.sources = snapshotWebSourceLabels(msg.Snapshot, run.ID)
		m.patches = snapshotPatches(msg.Snapshot, run.ID)
		m.taskDecomp = snapshotTaskDecompositions(msg.Snapshot, run.ID)
		m.memoryRecalls = append([]backend.MemoryRecallTraceInfo(nil), msg.Snapshot.MemoryRecallTraces...)
		m.llmUsage = msg.Snapshot.LLMUsageSummary
		m.llmContext = latestLLMContextBudget(msg.Snapshot.LLMUsageTraces)
		m.repairLoop = repairLoopSummary(msg.Snapshot.TaskProgress)
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
	if msg.Snapshot != nil && strings.TrimSpace(msg.Snapshot.TaskProgress.Summary) != "" && m.progress == progressRunning {
		m.progressText = strings.TrimSpace(msg.Snapshot.TaskProgress.Summary)
	}
	m.syncBlockingDecisionFromList()
	m.syncBlockingApprovalFromList()
}

func effectiveGoalText(run *backend.RunInfo) string {
	if run == nil {
		return ""
	}
	goal := payloadMap(run.ExecutionContext, "goal")
	if goal == nil {
		return ""
	}
	return strings.TrimSpace(firstPayloadString(goal, "text", "Text"))
}

func effectiveGoalTranscript(run *backend.RunInfo, fallback string) []transcriptItem {
	if run == nil {
		return initialTranscript(fallback)
	}
	goals := contextGoalMaps(run.ExecutionContext["goals"])
	if len(goals) == 0 {
		return initialTranscript(fallback)
	}
	items := make([]transcriptItem, 0, len(goals))
	seen := map[string]struct{}{}
	for _, goal := range goals {
		text := strings.TrimSpace(firstPayloadString(goal, "text", "Text"))
		if text == "" {
			continue
		}
		if _, ok := seen[text]; ok {
			continue
		}
		seen[text] = struct{}{}
		items = append(items, transcriptItem{Kind: "Goal", Text: text})
	}
	if len(items) == 0 {
		return initialTranscript(fallback)
	}
	return items
}

func effectiveDoDItems(run *backend.RunInfo) []string {
	if run == nil {
		return nil
	}
	dod := payloadMap(run.ExecutionContext, "dod")
	if dod == nil {
		return nil
	}
	return payloadStringSliceValue(dod, "items", "Items")
}

func contextGoalMaps(value any) []map[string]any {
	switch typed := value.(type) {
	case []map[string]any:
		return typed
	case []any:
		ret := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			if goal, ok := item.(map[string]any); ok {
				ret = append(ret, goal)
			}
		}
		return ret
	default:
		return nil
	}
}

func (m *AppModel) resetWorkspaceView() {
	m.activeRunID = ""
	m.currentTask = ""
	m.transcript = nil
	m.result = ""
	m.artifacts = nil
	m.patches = nil
	m.taskDecomp = nil
	m.memoryRecalls = nil
	m.llmUsage = backend.LLMUsageSummaryInfo{}
	m.llmContext = nil
	m.repairLoop = ""
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

func latestLLMContextBudget(traces []backend.LLMUsageTraceInfo) map[string]any {
	for idx := len(traces) - 1; idx >= 0; idx-- {
		budget := traces[idx].ContextBudget
		if len(budget) == 0 {
			continue
		}
		return clonePayloadMap(budget)
	}
	return nil
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
