package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/muidea/workorch/internal/pkg/common"
	backend "github.com/muidea/workorch/internal/pkg/workorchport"
)

type workspaceDisplayState struct {
	ActiveRunID           string
	CurrentTask           string
	DoDItems              []string
	Transcript            []transcriptItem
	Result                string
	Artifacts             []string
	Sources               []string
	Patches               []backend.PatchInfo
	ArtifactContinuations []backend.ArtifactContinuationInfo
	TaskDecompositions    []backend.TaskDecompositionInfo
	TaskPlans             []backend.TaskPlanInfo
	MemoryRecalls         []backend.MemoryRecallTraceInfo
	LLMUsage              backend.LLMUsageSummaryInfo
	LLMContextSummary     backend.LLMContextBudgetSummaryInfo
	LLMContext            map[string]any
	Acceptance            backend.AcceptanceProjectionInfo
	RunHeartbeat          backend.RunHeartbeatInfo
	RuntimeExecution      *backend.RuntimeExecutionInfo
	RuntimeEvents         []string
	Progress              progressState
	ProgressText          string
	RepairLoop            string
	FailureReason         string
	FailureHint           string
	Running               bool
	BlockingDecision      *backend.DecisionInfo
	BlockingApproval      *backend.ApprovalInfo
	Decisions             []backend.DecisionInfo
	Approvals             []backend.ApprovalInfo
	LastError             error
}

// workspaceRuntimeState is the TUI ViewModel for one workspace.
// Server projection remains authoritative; local fields only protect UI
// command intent while projection refresh is catching up.
type workspaceRuntimeState struct {
	Overview           backend.WorkspaceInfo
	HasOverview        bool
	Projection         backend.ProjectionInfo
	PromptMode         string
	PromptModeRevision int64
	Display            workspaceDisplayState
	HasDisplay         bool
	LastRevision       int64
	RevisionGapCount   int64
	LastRevisionGap    int64
	ContextRequestSeq  int64
	ContextInFlight    bool
	ContextPending     bool
	ResolvedTickets    map[common.TicketID]struct{}
	ResolvedApprovals  map[common.ApprovalID]struct{}
	OptimisticRunID    common.RunID
}

const (
	workspacePromptModeWorkspace = "workspace"
	workspacePromptModeRunning   = "running"
	workspacePromptModeDecision  = "decision"
	workspacePromptModeApproval  = "approval"
)

func (m *AppModel) ensureWorkspaceRuntimeState(workspaceID string) *workspaceRuntimeState {
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return nil
	}
	if m.workspaceRuntimeStates == nil {
		m.workspaceRuntimeStates = map[string]*workspaceRuntimeState{}
	}
	entry := m.workspaceRuntimeStates[workspaceID]
	if entry == nil {
		entry = &workspaceRuntimeState{}
		m.workspaceRuntimeStates[workspaceID] = entry
	}
	return entry
}

func (m *AppModel) workspaceRuntimeState(workspaceID string) *workspaceRuntimeState {
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" || m.workspaceRuntimeStates == nil {
		return nil
	}
	return m.workspaceRuntimeStates[workspaceID]
}

func findWorkspace(workspaces []backend.WorkspaceInfo, id string) *backend.WorkspaceInfo {
	for i := range workspaces {
		if string(workspaces[i].ID) == id {
			return &workspaces[i]
		}
	}
	return nil
}

func projectionRevision(input backend.ProjectionInfo) int64 {
	for _, value := range []int64{
		input.WorkspaceRevision,
		input.ProjectedSourceRevision,
		input.SourceEventRevision,
	} {
		if value > 0 {
			return value
		}
	}
	return 0
}

func viewModelRevision(input backend.WorkspaceViewModelInfo) int64 {
	for _, value := range []int64{
		input.Revision,
		input.WorkspaceRevision,
		input.ProjectedSourceRevision,
		input.SourceEventRevision,
	} {
		if value > 0 {
			return value
		}
	}
	return 0
}

func workspaceContextRevision(msg WorkspaceContextMsg) int64 {
	if msg.Snapshot != nil {
		if revision := viewModelRevision(msg.Snapshot.ViewModel); revision > 0 {
			return revision
		}
		if revision := projectionRevision(msg.Snapshot.Projection); revision > 0 {
			return revision
		}
		if revision := projectionRevision(msg.Snapshot.Workspace.Projection); revision > 0 {
			return revision
		}
	}
	if revision := projectionRevision(msg.Projection); revision > 0 {
		return revision
	}
	if msg.Workspace != nil {
		return projectionRevision(msg.Workspace.Projection)
	}
	return 0
}

func mergeProjectionInfo(current, incoming backend.ProjectionInfo) backend.ProjectionInfo {
	if projectionRevision(incoming) == 0 && incoming.GlobalRevision == 0 && incoming.State == "" {
		return current
	}
	if projectionRevision(current) > 0 && projectionRevision(incoming) > 0 && projectionRevision(incoming) < projectionRevision(current) {
		return current
	}
	return incoming
}

func (m *AppModel) activeWorkspaceOverview() *backend.WorkspaceInfo {
	if entry := m.workspaceRuntimeState(m.activeWorkspaceID); entry != nil && entry.HasOverview {
		return &entry.Overview
	}
	return findWorkspace(m.workspaces, m.activeWorkspaceID)
}

func promptModeFromWorkspaceOverview(ws *backend.WorkspaceInfo) string {
	if ws == nil {
		return workspacePromptModeWorkspace
	}
	switch ws.State {
	case common.WorkspaceStateWaitingDecision:
		if ws.BlockingTicketID != "" {
			return workspacePromptModeDecision
		}
	case common.WorkspaceStateBlocked:
		if ws.BlockingApprovalID != "" {
			return workspacePromptModeApproval
		}
	case common.WorkspaceStateRunning, common.WorkspaceStateValidating:
		return workspacePromptModeRunning
	}
	return workspacePromptModeWorkspace
}

func normalizePromptMode(mode string) string {
	switch strings.TrimSpace(mode) {
	case workspacePromptModeRunning, workspacePromptModeDecision, workspacePromptModeApproval:
		return strings.TrimSpace(mode)
	default:
		return workspacePromptModeWorkspace
	}
}

func (m *AppModel) applyWorkspaceViewModel(viewModel backend.WorkspaceViewModelInfo) {
	if strings.TrimSpace(viewModel.PromptMode) == "" {
		return
	}
	workspaceID := strings.TrimSpace(string(viewModel.WorkspaceID))
	if workspaceID == "" {
		workspaceID = m.activeWorkspaceID
	}
	if workspaceID == "" {
		return
	}
	entry := m.ensureWorkspaceRuntimeState(workspaceID)
	if entry == nil {
		return
	}
	revision := viewModelRevision(viewModel)
	if entry.PromptModeRevision > 0 && revision > 0 && revision < entry.PromptModeRevision {
		return
	}
	entry.PromptMode = normalizePromptMode(viewModel.PromptMode)
	entry.PromptModeRevision = revision
}

func (m *AppModel) activeWorkspacePromptMode() string {
	entry := m.workspaceRuntimeState(m.activeWorkspaceID)
	if entry != nil && entry.PromptMode != "" {
		return normalizePromptMode(entry.PromptMode)
	}
	ws := m.activeWorkspaceOverview()
	overviewMode := promptModeFromWorkspaceOverview(ws)
	return overviewMode
}

func (m *AppModel) workspaceContextOlderThanOverview(msg WorkspaceContextMsg) bool {
	entry := m.workspaceRuntimeState(msg.WorkspaceID)
	if entry == nil || !entry.HasOverview {
		return false
	}
	overviewRevision := projectionRevision(entry.Overview.Projection)
	if overviewRevision == 0 {
		return false
	}
	contextRevision := workspaceContextRevision(msg)
	return contextRevision > 0 && contextRevision < overviewRevision
}

func (m *AppModel) mergeWorkspaceOverview(ws backend.WorkspaceInfo) backend.WorkspaceInfo {
	entry := m.ensureWorkspaceRuntimeState(string(ws.ID))
	if entry == nil {
		return ws
	}
	m.applyWorkspaceViewModel(ws.ViewModel)
	if ws.Projection.State == "" && projectionRevision(ws.Projection) == 0 && entry.Projection.State != "" {
		ws.Projection = entry.Projection
	}
	ws = entry.normalizeWorkspaceOverview(ws)
	currentRevision := projectionRevision(entry.Overview.Projection)
	incomingRevision := projectionRevision(ws.Projection)
	if entry.HasOverview && currentRevision > 0 && incomingRevision > 0 && incomingRevision < currentRevision {
		return entry.Overview
	}
	entry.Projection = mergeProjectionInfo(entry.Projection, ws.Projection)
	if ws.Projection.State == "" && projectionRevision(ws.Projection) == 0 {
		ws.Projection = entry.Projection
	}
	entry.Overview = ws
	entry.HasOverview = true
	return ws
}

func (entry *workspaceRuntimeState) normalizeWorkspaceOverview(ws backend.WorkspaceInfo) backend.WorkspaceInfo {
	if entry == nil {
		return ws
	}
	if workspaceStateIsLive(ws.State) {
		ws.BlockingTicketID = ""
		ws.BlockingApprovalID = ""
	}
	return ws
}

func (m *AppModel) replaceWorkspaceList(workspaces []backend.WorkspaceInfo, projection backend.ProjectionInfo) {
	next := make([]backend.WorkspaceInfo, 0, len(workspaces))
	for _, ws := range workspaces {
		if ws.Projection.State == "" && projectionRevision(ws.Projection) == 0 {
			ws.Projection = projection
		}
		next = append(next, m.mergeWorkspaceOverview(ws))
	}
	m.workspaces = next
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
	state := common.WorkspaceState(firstPayloadString(evt.Payload, "state", "State", backend.PayloadKeyState))
	activeRunID := common.RunID(firstPayloadString(evt.Payload, "activeRunId", "ActiveRunID", backend.PayloadKeyActiveRunID))
	blockingTicketID := common.TicketID(firstPayloadString(evt.Payload, "blockingTicketId", "BlockingTicketID", backend.PayloadKeyBlockingTicketID))
	blockingApprovalID := common.ApprovalID(firstPayloadString(evt.Payload, "blockingApprovalId", "BlockingApprovalID", backend.PayloadKeyBlockingApprovalID))
	title := firstPayloadString(evt.Payload, "title", "Title", backend.PayloadKeyTitle)
	description := firstPayloadString(evt.Payload, "description", "Description", backend.PayloadKeyDescription)
	deliverableDirectory := firstPayloadString(evt.Payload, "deliverableDirectory", "DeliverableDirectory", backend.PayloadKeyDeliverableDirectory)
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
			if activeRunID != "" || hasPayloadKey(evt.Payload, backend.PayloadKeyActiveRunID, "ActiveRunID", "activeRunId") {
				m.workspaces[i].ActiveRunID = activeRunID
			}
			if blockingTicketID != "" || hasPayloadKey(evt.Payload, backend.PayloadKeyBlockingTicketID, "BlockingTicketID", "blockingTicketId") {
				m.workspaces[i].BlockingTicketID = blockingTicketID
			}
			if blockingApprovalID != "" || hasPayloadKey(evt.Payload, backend.PayloadKeyBlockingApprovalID, "BlockingApprovalID", "blockingApprovalId") {
				m.workspaces[i].BlockingApprovalID = blockingApprovalID
			}
			if evt.Revision > 0 {
				m.workspaces[i].Projection.SourceEventRevision = evt.Revision
			}
			m.workspaces[i] = m.mergeWorkspaceOverview(m.workspaces[i])
			return
		}
	}
	ws := backend.WorkspaceInfo{
		ID:                   evt.WorkspaceID,
		Title:                title,
		Description:          description,
		DeliverableDirectory: deliverableDirectory,
		State:                state,
		ActiveRunID:          activeRunID,
		BlockingTicketID:     blockingTicketID,
		BlockingApprovalID:   blockingApprovalID,
	}
	if evt.Revision > 0 {
		ws.Projection.SourceEventRevision = evt.Revision
	}
	m.workspaces = append(m.workspaces, m.mergeWorkspaceOverview(ws))
}

func (m *AppModel) removeWorkspace(id common.WorkspaceID) {
	if id == "" {
		return
	}
	if m.workspaceRuntimeStates != nil {
		delete(m.workspaceRuntimeStates, string(id))
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
	m.saveActiveWorkspaceDisplayState()
	m.activeWorkspaceID = id
	m.selectedWorkspaceID = id
	if !m.restoreWorkspaceDisplayState(id) {
		m.resetWorkspaceView()
	}
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

func (m *AppModel) activeWorkspaceRunID() string {
	ws := m.activeWorkspaceOverview()
	if ws != nil {
		if !workspaceStateIsLive(ws.State) {
			return ""
		}
		if ws.ActiveRunID != "" {
			return string(ws.ActiveRunID)
		}
	}
	if strings.TrimSpace(m.activeRunID) == "" {
		return ""
	}
	if ws != nil && !workspaceStateIsLive(ws.State) {
		return ""
	}
	return m.activeRunID
}

func (m *AppModel) activeWorkspaceStateIsLive() bool {
	ws := m.activeWorkspaceOverview()
	if ws == nil {
		return false
	}
	return workspaceStateIsLive(ws.State)
}

func (m *AppModel) activeWorkspaceBlockerProgress() (progressState, bool) {
	if m.activeBlockingDecision() != nil {
		return progressWaitingDecision, true
	}
	if m.activeBlockingApproval() != nil {
		return progressWaitingApproval, true
	}
	ws := findWorkspace(m.workspaces, m.activeWorkspaceID)
	if overview := m.activeWorkspaceOverview(); overview != nil {
		ws = overview
	}
	if ws == nil {
		return "", false
	}
	switch ws.State {
	case common.WorkspaceStateWaitingDecision:
		return progressWaitingDecision, true
	case common.WorkspaceStateBlocked:
		return progressWaitingApproval, true
	default:
		return "", false
	}
}

func (m *AppModel) activeWorkspaceHasDecisionBlocker() bool {
	return m.activeBlockingDecision() != nil
}

func (m *AppModel) activeWorkspaceHasApprovalBlocker() bool {
	return m.activeBlockingApproval() != nil
}

func (m *AppModel) ensureActiveBlockingDecisionFromOverview() bool {
	ws := m.activeWorkspaceOverview()
	if ws != nil && ws.State == common.WorkspaceStateWaitingDecision {
		if ws.BlockingTicketID == "" {
			return false
		}
		runID := ws.ActiveRunID
		if runID == "" {
			runID = common.RunID(m.activeRunID)
		}
		if m.blockingDecision == nil ||
			m.blockingDecision.ID != ws.BlockingTicketID ||
			m.blockingDecision.WorkspaceID != ws.ID ||
			(runID != "" && m.blockingDecision.RunID != runID) {
			m.setBlockingDecision(ws.BlockingTicketID, ws.ID, runID, "", "")
		}
		m.activeRunID = string(runID)
		m.running = false
		m.progress = progressWaitingDecision
		m.progressText = nonEmpty(blockingDecisionLabel(m.blockingDecision), "Waiting for decision")
		return true
	}
	return false
}

func (m *AppModel) ensureActiveBlockingApprovalFromOverview() bool {
	ws := m.activeWorkspaceOverview()
	if ws != nil && ws.State == common.WorkspaceStateBlocked {
		if ws.BlockingApprovalID == "" {
			return false
		}
		runID := ws.ActiveRunID
		if runID == "" {
			runID = common.RunID(m.activeRunID)
		}
		if m.blockingApproval == nil ||
			m.blockingApproval.ID != ws.BlockingApprovalID ||
			m.blockingApproval.WorkspaceID != ws.ID ||
			(runID != "" && m.blockingApproval.RunID != runID) {
			m.setBlockingApproval(ws.BlockingApprovalID, ws.ID, runID, "", "", nil)
		}
		m.activeRunID = string(runID)
		m.running = false
		m.progress = progressWaitingApproval
		m.progressText = nonEmpty(blockingApprovalLabel(m.blockingApproval), "Waiting for approval")
		return true
	}
	return false
}

func (m *AppModel) syncActiveWorkspaceBlockerOverview() tea.Cmd {
	ws := m.activeWorkspaceOverview()
	if ws == nil {
		return nil
	}
	switch ws.State {
	case common.WorkspaceStateWaitingDecision, common.WorkspaceStateBlocked:
		return m.syncActiveWorkspaceOverview()
	default:
		return nil
	}
}

func (m *AppModel) restoreActiveWorkspaceBlockerFromOverview() bool {
	ws := m.activeWorkspaceOverview()
	if ws == nil {
		return false
	}
	switch ws.State {
	case common.WorkspaceStateWaitingDecision:
		return m.ensureActiveBlockingDecisionFromOverview()
	case common.WorkspaceStateBlocked:
		return m.ensureActiveBlockingApprovalFromOverview()
	default:
		return false
	}
}

func (m *AppModel) blockActiveWorkspaceResume(reason string) tea.Cmd {
	blockerProgress, ok := m.activeWorkspaceBlockerProgress()
	if !ok {
		return nil
	}
	m.running = false
	m.progress = blockerProgress
	switch blockerProgress {
	case progressWaitingApproval:
		m.progressText = nonEmpty(reason, blockingApprovalLabel(m.activeBlockingApproval()), "Waiting for approval; refreshing blocker details")
	default:
		m.progressText = nonEmpty(reason, blockingDecisionLabel(m.activeBlockingDecision()), "Waiting for decision; refreshing blocker details")
	}
	return m.fetchActiveBlockerDetails()
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
	cmds := []tea.Cmd{}
	if cmd := m.syncActiveWorkspaceOverview(); cmd != nil {
		cmds = append(cmds, cmd)
	}
	if m.progress != progressWaitingDecision && m.progress != progressWaitingApproval {
		m.progressText = "Workspace switched; refreshing state"
	}
	cmds = append(cmds, m.requestWorkspaceListRefresh(), m.requestWorkspaceContextRefresh(true))
	return m, tea.Batch(cmds...)
}

func (m *AppModel) applyWorkspaceContext(msg WorkspaceContextMsg) {
	if msg.Workspace == nil {
		return
	}
	workspaceID := string(msg.Workspace.ID)
	if workspaceID == "" || workspaceID != m.activeWorkspaceID {
		return
	}
	previousTask := m.currentTask
	previousTranscript := append([]transcriptItem(nil), m.transcript...)
	previousDoDItems := append([]string(nil), m.dodItems...)
	if msg.Workspace.Projection.State == "" && projectionRevision(msg.Workspace.Projection) == 0 {
		msg.Workspace.Projection = msg.Projection
	}
	m.upsertWorkspace(*msg.Workspace)
	if msg.Snapshot != nil {
		m.applyWorkspaceViewModel(msg.Snapshot.ViewModel)
	}
	if msg.Decisions != nil {
		m.decisions = append([]backend.DecisionInfo(nil), msg.Decisions...)
	}
	if msg.Approvals != nil {
		m.approvals = append([]backend.ApprovalInfo(nil), msg.Approvals...)
	}

	workspaceHasLive := msg.WorkspaceHasLive || msg.Workspace.ActiveRunID != ""
	activeRun := msg.ActiveRun
	displayRun := msg.DisplayRun
	if displayRun == nil {
		displayRun = activeRun
	}
	run := displayRun
	if run == nil {
		m.activeRunID = ""
		m.running = false
		m.clearRunProjectionState()
		if msg.Goal != nil && strings.TrimSpace(msg.Goal.Text) != "" {
			goalText := strings.TrimSpace(msg.Goal.Text)
			m.currentTask = goalText
			m.transcript = initialTranscript(goalText)
		} else if msg.Snapshot != nil {
			m.currentTask = ""
			m.transcript = nil
		}
		if msg.DoD != nil && len(msg.DoD.Items) > 0 {
			m.dodItems = append([]string(nil), msg.DoD.Items...)
		} else if msg.Snapshot != nil {
			m.dodItems = nil
		}
		m.restoreDefinitionsWhenPartialContext(msg, previousTask, previousTranscript, previousDoDItems)
		m.applyWorkspaceProgress(msg.Workspace.State)
		if msg.Snapshot != nil {
			m.applyRunHeartbeat(msg.Snapshot.RunHeartbeat)
			m.applyRuntimeExecution(msg.Snapshot.RuntimeExecution)
			m.applyRunCommandStatus(msg.Snapshot.RunCommand)
			m.applySchedulerDiagnostics(msg.Snapshot.SchedulerDiagnostics)
		}
		m.syncBlockingDecisionFromList()
		m.syncBlockingApprovalFromList()
		return
	}
	if workspaceHasLive && activeRun != nil {
		m.activeRunID = string(activeRun.ID)
	} else {
		m.activeRunID = ""
	}
	goalText := effectiveGoalText(run)
	if goalText == "" && msg.Goal != nil {
		goalText = strings.TrimSpace(msg.Goal.Text)
	}
	if goalText != "" {
		m.currentTask = goalText
		m.transcript = effectiveGoalTranscript(run, goalText)
	} else if msg.Snapshot != nil {
		m.currentTask = ""
		m.transcript = nil
	}
	if items := effectiveDoDItems(run); len(items) > 0 {
		m.dodItems = items
	} else if msg.DoD != nil && len(msg.DoD.Items) > 0 {
		m.dodItems = append([]string(nil), msg.DoD.Items...)
	} else if msg.Snapshot != nil {
		m.dodItems = nil
	}
	m.restoreDefinitionsWhenPartialContext(msg, previousTask, previousTranscript, previousDoDItems)
	if msg.Snapshot != nil {
		if runStateIsTerminal(run.State) {
			m.result = snapshotDisplayResult(msg.Snapshot, run.ID)
		} else {
			m.result = ""
		}
		m.artifacts = snapshotArtifactLabels(msg.Snapshot, run.ID)
		m.sources = snapshotWebSourceLabels(msg.Snapshot, run.ID)
		m.patches = snapshotPatches(msg.Snapshot, run.ID)
		m.artifactContinuations = snapshotArtifactContinuations(msg.Snapshot, run.ID)
		m.taskDecomp = snapshotTaskDecompositions(msg.Snapshot, run.ID)
		m.taskPlans = snapshotTaskPlans(msg.Snapshot, run.ID)
		m.memoryRecalls = append([]backend.MemoryRecallTraceInfo(nil), msg.Snapshot.MemoryRecallTraces...)
		m.llmUsage = msg.Snapshot.LLMUsageSummary
		m.llmContextSummary = msg.Snapshot.LLMContextBudgetSummary
		m.llmContext = latestLLMContextBudget(msg.Snapshot.LLMContextBudgets, msg.Snapshot.LLMUsageTraces)
		m.acceptance = msg.Snapshot.Acceptance
		m.applyRunHeartbeat(msg.Snapshot.RunHeartbeat)
		m.applyRuntimeExecution(msg.Snapshot.RuntimeExecution)
		m.applyRunCommandStatus(msg.Snapshot.RunCommand)
		m.applySchedulerDiagnostics(msg.Snapshot.SchedulerDiagnostics)
		m.repairLoop = repairLoopSummary(msg.Snapshot.TaskProgress)
	}

	state := run.State
	payload := map[string]any{
		backend.PayloadKeyFailureReason:      run.FailureReason,
		backend.PayloadKeyBlockingTicketID:   string(run.BlockingTicketID),
		backend.PayloadKeyBlockingApprovalID: string(run.BlockingApprovalID),
	}
	if msg.Status != nil {
		state = msg.Status.State
		payload[backend.PayloadKeyFailureReason] = msg.Status.FailureReason
		payload[backend.PayloadKeyBlockingTicketID] = string(msg.Status.BlockingTicketID)
		payload[backend.PayloadKeyBlockingApprovalID] = string(msg.Status.BlockingApprovalID)
	}
	if msg.Snapshot != nil && payload[backend.PayloadKeyFailureReason] == "" {
		payload[backend.PayloadKeyFailureReason] = msg.Snapshot.Outcome.Reason
	}
	if workspaceHasLive {
		m.applyRunState(state, payload)
	} else {
		m.applyWorkspaceProgress(msg.Workspace.State)
	}
	if msg.Status != nil && msg.Status.CurrentStepKind != "" && m.progress == progressRunning {
		m.progressText = stepProgressText("Working", msg.Status.CurrentStepKind, "Working; waiting for final result")
	}
	if msg.Status != nil {
		m.applyRuntimeExecution(msg.Status.RuntimeExecution)
		if msg.Status.SchedulerDiagnostics != nil {
			m.applySchedulerDiagnostics(msg.Status.SchedulerDiagnostics)
		}
	}
	if msg.Snapshot != nil && strings.TrimSpace(msg.Snapshot.TaskProgress.Summary) != "" && m.progress == progressRunning {
		m.progressText = strings.TrimSpace(msg.Snapshot.TaskProgress.Summary)
	}
	m.syncBlockingDecisionFromList()
	m.syncBlockingApprovalFromList()
	if strings.TrimSpace(m.result) != "" {
		m.followMainViewportTail()
	}
}

func (m *AppModel) clearRunProjectionState() {
	m.result = ""
	m.artifacts = nil
	m.sources = nil
	m.patches = nil
	m.artifactContinuations = nil
	m.taskDecomp = nil
	m.taskPlans = nil
	m.memoryRecalls = nil
	m.llmUsage = backend.LLMUsageSummaryInfo{}
	m.llmContextSummary = backend.LLMContextBudgetSummaryInfo{}
	m.llmContext = nil
	m.acceptance = backend.AcceptanceProjectionInfo{}
	m.runHeartbeat = backend.RunHeartbeatInfo{}
	m.runtimeExecution = nil
	m.runCommand = nil
	m.repairLoop = ""
	m.failureReason = ""
	m.failureHint = ""
	m.blockingDecision = nil
	m.blockingApproval = nil
}

func (m *AppModel) restoreDefinitionsWhenPartialContext(msg WorkspaceContextMsg, previousTask string, previousTranscript []transcriptItem, previousDoDItems []string) {
	if msg.Snapshot != nil {
		return
	}
	if strings.TrimSpace(m.currentTask) == "" && strings.TrimSpace(previousTask) != "" {
		m.currentTask = previousTask
		m.transcript = append([]transcriptItem(nil), previousTranscript...)
	}
	if len(m.dodItems) == 0 && len(previousDoDItems) > 0 {
		m.dodItems = append([]string(nil), previousDoDItems...)
	}
}

func runStateIsTerminal(state common.RunState) bool {
	switch state {
	case common.RunStateCompleted, common.RunStateFailed, common.RunStateCancelled, common.RunStateSuperseded:
		return true
	default:
		return false
	}
}

func runStateIsLive(state common.RunState) bool {
	return state != "" && !runStateIsTerminal(state) &&
		state != common.RunStateBlockedOnDecision &&
		state != common.RunStateBlockedOnApproval
}

func workspaceStateIsLive(state common.WorkspaceState) bool {
	switch state {
	case common.WorkspaceStateRunning, common.WorkspaceStateValidating:
		return true
	default:
		return false
	}
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
	m.dodItems = nil
	m.transcript = nil
	m.result = ""
	m.artifacts = nil
	m.sources = nil
	m.patches = nil
	m.artifactContinuations = nil
	m.taskDecomp = nil
	m.taskPlans = nil
	m.memoryRecalls = nil
	m.llmUsage = backend.LLMUsageSummaryInfo{}
	m.llmContext = nil
	m.acceptance = backend.AcceptanceProjectionInfo{}
	m.runHeartbeat = backend.RunHeartbeatInfo{}
	m.runtimeExecution = nil
	m.schedulerDiagnostics = nil
	m.runtimeEvents = nil
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

func (m *AppModel) saveActiveWorkspaceDisplayState() {
	if strings.TrimSpace(m.activeWorkspaceID) == "" {
		return
	}
	entry := m.ensureWorkspaceRuntimeState(m.activeWorkspaceID)
	if entry == nil {
		return
	}
	entry.Display = workspaceDisplayState{
		CurrentTask:           m.currentTask,
		DoDItems:              append([]string(nil), m.dodItems...),
		Transcript:            append([]transcriptItem(nil), m.transcript...),
		Result:                m.result,
		Artifacts:             append([]string(nil), m.artifacts...),
		Sources:               append([]string(nil), m.sources...),
		Patches:               append([]backend.PatchInfo(nil), m.patches...),
		ArtifactContinuations: append([]backend.ArtifactContinuationInfo(nil), m.artifactContinuations...),
		TaskDecompositions:    append([]backend.TaskDecompositionInfo(nil), m.taskDecomp...),
		TaskPlans:             append([]backend.TaskPlanInfo(nil), m.taskPlans...),
		MemoryRecalls:         append([]backend.MemoryRecallTraceInfo(nil), m.memoryRecalls...),
		LLMUsage:              m.llmUsage,
		LLMContextSummary:     m.llmContextSummary,
		LLMContext:            clonePayloadMap(m.llmContext),
		Acceptance:            m.acceptance,
		RunHeartbeat:          m.runHeartbeat,
		RuntimeExecution:      cloneRuntimeExecutionInfoPtr(m.runtimeExecution),
		RuntimeEvents:         append([]string(nil), m.runtimeEvents...),
		RepairLoop:            m.repairLoop,
	}
	entry.HasDisplay = true
}

func (m *AppModel) restoreWorkspaceDisplayState(workspaceID string) bool {
	cached, ok := m.cachedWorkspaceDisplayState(workspaceID)
	if !ok {
		return false
	}
	m.activeRunID = ""
	m.currentTask = cached.CurrentTask
	m.dodItems = append([]string(nil), cached.DoDItems...)
	m.transcript = append([]transcriptItem(nil), cached.Transcript...)
	m.result = cached.Result
	m.artifacts = append([]string(nil), cached.Artifacts...)
	m.sources = append([]string(nil), cached.Sources...)
	m.patches = append([]backend.PatchInfo(nil), cached.Patches...)
	m.artifactContinuations = append([]backend.ArtifactContinuationInfo(nil), cached.ArtifactContinuations...)
	m.taskDecomp = append([]backend.TaskDecompositionInfo(nil), cached.TaskDecompositions...)
	m.taskPlans = append([]backend.TaskPlanInfo(nil), cached.TaskPlans...)
	m.memoryRecalls = append([]backend.MemoryRecallTraceInfo(nil), cached.MemoryRecalls...)
	m.llmUsage = cached.LLMUsage
	m.llmContextSummary = cached.LLMContextSummary
	m.llmContext = clonePayloadMap(cached.LLMContext)
	m.acceptance = cached.Acceptance
	m.runHeartbeat = cached.RunHeartbeat
	m.runtimeExecution = cloneRuntimeExecutionInfoPtr(cached.RuntimeExecution)
	m.schedulerDiagnostics = nil
	m.runtimeEvents = append([]string(nil), cached.RuntimeEvents...)
	m.repairLoop = cached.RepairLoop
	m.failureReason = ""
	m.failureHint = ""
	m.running = false
	m.progress = progressIdle
	m.progressText = ""
	m.blockingDecision = nil
	m.blockingApproval = nil
	m.decisions = nil
	m.approvals = nil
	m.lastError = nil
	m.resetMainViewport()
	m.clearInput()
	return true
}

func (m *AppModel) cachedWorkspaceDisplayState(workspaceID string) (workspaceDisplayState, bool) {
	entry := m.workspaceRuntimeState(workspaceID)
	if entry == nil || !entry.HasDisplay {
		return workspaceDisplayState{}, false
	}
	return entry.Display, true
}

func cloneDecisionPtr(input *backend.DecisionInfo) *backend.DecisionInfo {
	if input == nil {
		return nil
	}
	ret := *input
	return &ret
}

func cloneApprovalPtr(input *backend.ApprovalInfo) *backend.ApprovalInfo {
	if input == nil {
		return nil
	}
	ret := *input
	ret.Context = clonePayloadMap(input.Context)
	return &ret
}

func cloneRuntimeExecutionInfoPtr(input *backend.RuntimeExecutionInfo) *backend.RuntimeExecutionInfo {
	if input == nil {
		return nil
	}
	ret := *input
	if input.ContractViolation != nil {
		violation := *input.ContractViolation
		violation.ExpectedContract = clonePayloadMap(input.ContractViolation.ExpectedContract)
		violation.ActualOutputSummary = clonePayloadMap(input.ContractViolation.ActualOutputSummary)
		violation.ForbiddenRepeat = append([]string(nil), input.ContractViolation.ForbiddenRepeat...)
		violation.SourceRefs = append([]string(nil), input.ContractViolation.SourceRefs...)
		ret.ContractViolation = &violation
	}
	return &ret
}

func cloneSchedulerDiagnosticsInfoPtr(input *backend.SchedulerDiagnosticsInfo) *backend.SchedulerDiagnosticsInfo {
	if input == nil {
		return nil
	}
	ret := *input
	if input.LatestContinuation != nil {
		latest := *input.LatestContinuation
		latest.RecommendedActions = append([]string(nil), input.LatestContinuation.RecommendedActions...)
		ret.LatestContinuation = &latest
	}
	ret.RecentContinuations = append([]backend.ContinuationDiagnosticInfo(nil), input.RecentContinuations...)
	for idx := range ret.RecentContinuations {
		ret.RecentContinuations[idx].RecommendedActions = append([]string(nil), input.RecentContinuations[idx].RecommendedActions...)
	}
	ret.ContinuationFailureCounts = append([]backend.ContinuationDiagnosticCountInfo(nil), input.ContinuationFailureCounts...)
	ret.ContinuationNoActionCounts = append([]backend.ContinuationDiagnosticCountInfo(nil), input.ContinuationNoActionCounts...)
	ret.ContinuationCompletedCounts = append([]backend.ContinuationDiagnosticCountInfo(nil), input.ContinuationCompletedCounts...)
	return &ret
}

func (m *AppModel) applyRunHeartbeat(item backend.RunHeartbeatInfo) {
	m.runHeartbeat = item
	switch item.ActivityState {
	case "ineffective_retry", "no_activity", "stalled":
		if m.progress == progressRunning || m.progress == progressIdle {
			m.progressText = nonEmpty(item.Summary, string(item.ActivityState))
		}
	}
}

func (m *AppModel) applyRuntimeExecution(item *backend.RuntimeExecutionInfo) {
	if item == nil {
		m.runtimeExecution = nil
		return
	}
	m.runtimeExecution = cloneRuntimeExecutionInfoPtr(item)
}

func (m *AppModel) applySchedulerDiagnostics(item *backend.SchedulerDiagnosticsInfo) {
	m.schedulerDiagnostics = cloneSchedulerDiagnosticsInfoPtr(item)
}

func latestLLMContextBudget(budgets []backend.LLMContextBudgetTraceInfo, traces []backend.LLMUsageTraceInfo) map[string]any {
	for idx := len(budgets) - 1; idx >= 0; idx-- {
		if budget := llmContextBudgetTraceMap(budgets[idx]); len(budget) > 0 {
			return budget
		}
	}
	for idx := len(traces) - 1; idx >= 0; idx-- {
		budget := traces[idx].ContextBudget
		if len(budget) == 0 {
			continue
		}
		return clonePayloadMap(budget)
	}
	return nil
}

func llmContextBudgetTraceMap(item backend.LLMContextBudgetTraceInfo) map[string]any {
	if item.StepID == "" &&
		item.ProviderID == "" &&
		item.Model == "" &&
		item.MaxBytes == 0 &&
		item.ActualBytes == 0 &&
		item.MaxTokens == 0 &&
		item.EstimatedInputTokens == 0 &&
		item.EstimatedOutputTokens == 0 &&
		item.ActualTokens == 0 &&
		len(item.IncludedSections) == 0 &&
		len(item.NormalizedSections) == 0 &&
		len(item.CompressedSections) == 0 &&
		len(item.OmittedSections) == 0 {
		return nil
	}
	ret := map[string]any{
		"stepId":                string(item.StepID),
		"providerId":            item.ProviderID,
		"model":                 item.Model,
		"maxBytes":              item.MaxBytes,
		"actualBytes":           item.ActualBytes,
		"maxTokens":             item.MaxTokens,
		"estimatedInputTokens":  item.EstimatedInputTokens,
		"estimatedOutputTokens": item.EstimatedOutputTokens,
		"actualTokens":          item.ActualTokens,
		"shrinkLevel":           item.ShrinkLevel,
		"truncated":             item.Truncated,
		"rules":                 append([]string(nil), item.Rules...),
		"sourceRefs":            append([]string(nil), item.SourceRefs...),
		"includedSections":      append([]string(nil), item.IncludedSections...),
		"normalizedSections":    append([]string(nil), item.NormalizedSections...),
		"compressedSections":    append([]string(nil), item.CompressedSections...),
		"omittedSections":       append([]string(nil), item.OmittedSections...),
		"goalDoDCompressed":     item.GoalDoDCompressed,
		"overBudget":            item.OverBudget,
		"p0Overflow":            item.P0Overflow,
		"profileMode":           item.ProfileMode,
		"capabilitySource":      item.CapabilitySource,
		"contextUtilization":    item.ContextUtilization,
		"maxRequestBytes":       item.MaxRequestBytes,
		"stageBudgetRatio":      item.StageBudgetRatio,
		"stageBudgetSource":     item.StageBudgetSource,
		"tracePolicy":           item.TracePolicy,
		"promptCaptured":        item.PromptCaptured,
		"estimator":             item.Estimator,
		"profile":               clonePayloadMap(item.Profile),
		"cachePlan":             clonePayloadMap(item.CachePlan),
	}
	return ret
}

func (m *AppModel) upsertWorkspace(ws backend.WorkspaceInfo) {
	ws = m.mergeWorkspaceOverview(ws)
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
		m.clearBlockingInputState()
		m.running = true
		m.progress = progressRunning
		m.progressText = string(state) + "; waiting for progress or final result"
	case common.WorkspaceStateWaitingDecision:
		m.running = false
		m.progress = progressWaitingDecision
		m.progressText = "Waiting for workspace input"
	case common.WorkspaceStateBlocked:
		m.running = false
		m.progress = progressWaitingApproval
		m.progressText = "Waiting for workspace approval"
	case common.WorkspaceStateCompleted:
		m.running = false
		m.progress = progressCompleted
		m.progressText = "Completed"
	case common.WorkspaceStateFailed:
		m.running = false
		m.progress = progressFailed
		m.progressText = "Workspace failed"
	default:
		m.running = false
		m.progress = progressIdle
		m.progressText = ""
	}
}

func (m *AppModel) applyRunCommandStatus(status *backend.RunCommandStatusInfo) {
	if status == nil {
		m.runCommand = nil
		return
	}
	if status.WorkspaceID != "" && string(status.WorkspaceID) != m.activeWorkspaceID {
		return
	}
	copied := *status
	m.runCommand = &copied
	if m.progress == progressSubmitting && runCommandProjectionSettled(copied) {
		m.progress = progressIdle
		m.progressText = ""
	}
}

func runCommandProjectionSettled(status backend.RunCommandStatusInfo) bool {
	state := strings.ToLower(strings.TrimSpace(status.State))
	if state == "accepted" || state == "rejected" {
		return true
	}
	return strings.TrimSpace(status.AcceptedAt) != "" || strings.TrimSpace(status.RejectedAt) != ""
}

func (m *AppModel) clearBlockingInputState() {
	m.blockingDecision = nil
	m.blockingApproval = nil
	if m.progress == progressWaitingDecision || m.progress == progressWaitingApproval {
		m.progress = progressIdle
		m.progressText = ""
	}
}

func (m *AppModel) syncActiveWorkspaceOverview() tea.Cmd {
	ws := m.activeWorkspaceOverview()
	if ws == nil {
		return nil
	}
	if ws.ActiveRunID != "" {
		m.activeRunID = string(ws.ActiveRunID)
	} else if !workspaceStateIsLive(ws.State) && ws.State != common.WorkspaceStateWaitingDecision && ws.State != common.WorkspaceStateBlocked {
		m.activeRunID = ""
	}
	m.applyWorkspaceProgress(ws.State)
	switch ws.State {
	case common.WorkspaceStateWaitingDecision:
		m.running = false
		if ws.BlockingTicketID != "" {
			m.progress = progressWaitingDecision
			m.setBlockingDecision(ws.BlockingTicketID, ws.ID, ws.ActiveRunID, "", "")
			m.progressText = nonEmpty(blockingDecisionLabel(m.blockingDecision), "Waiting for decision")
			m.followBlockerTailUnlessUserScrolled()
			return m.fetchActiveBlockerDetails()
		}
		m.progress = progressWaitingDecision
		m.progressText = "Waiting for decision"
	case common.WorkspaceStateBlocked:
		m.running = false
		if ws.BlockingApprovalID != "" {
			m.progress = progressWaitingApproval
			m.setBlockingApproval(ws.BlockingApprovalID, ws.ID, ws.ActiveRunID, "", "", nil)
			m.progressText = nonEmpty(blockingApprovalLabel(m.blockingApproval), "Waiting for approval")
			m.followBlockerTailUnlessUserScrolled()
			return m.fetchActiveBlockerDetails()
		}
		m.progress = progressWaitingApproval
		m.progressText = "Waiting for approval"
	default:
		m.blockingDecision = nil
		m.blockingApproval = nil
	}
	return nil
}

func (m *AppModel) markDecisionResolvedForWorkspace(workspaceID common.WorkspaceID, id common.TicketID, runID common.RunID) {
	entry := m.ensureWorkspaceRuntimeState(string(workspaceID))
	if entry == nil || id == "" {
		return
	}
	if entry.ResolvedTickets == nil {
		entry.ResolvedTickets = map[common.TicketID]struct{}{}
	}
	entry.ResolvedTickets[id] = struct{}{}
	if runID != "" {
		entry.OptimisticRunID = runID
	}
}

func (m *AppModel) markApprovalResolvedForWorkspace(workspaceID common.WorkspaceID, id common.ApprovalID, runID common.RunID) {
	entry := m.ensureWorkspaceRuntimeState(string(workspaceID))
	if entry == nil || id == "" {
		return
	}
	if entry.ResolvedApprovals == nil {
		entry.ResolvedApprovals = map[common.ApprovalID]struct{}{}
	}
	entry.ResolvedApprovals[id] = struct{}{}
	if runID != "" {
		entry.OptimisticRunID = runID
	}
}
