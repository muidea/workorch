package tui

import (
	"reflect"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/muidea/workorch/internal/pkg/common"
	backend "github.com/muidea/workorch/internal/pkg/workorchport"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type workspaceDisplayState struct {
	ActiveRunID           string
	DisplayRunID          string
	SealedLLMSelection    *runtimecontract.ProviderInferenceSelection
	CurrentTask           string
	DoDItems              []string
	Transcript            []transcriptItem
	Result                string
	Artifacts             []string
	Sources               []string
	Patches               []projectioncontract.Patch
	ArtifactContinuations []projectioncontract.ArtifactContinuation
	TaskDecompositions    []projectioncontract.TaskDecomposition
	TaskPlans             []projectioncontract.TaskPlan
	MemoryRecalls         []projectioncontract.MemoryRecallTrace
	LLMUsage              projectioncontract.LLMUsageSummary
	LLMContextSummary     projectioncontract.LLMContextBudgetSummary
	LLMContext            map[string]any
	Acceptance            projectioncontract.AcceptanceProjection
	RunHeartbeat          projectioncontract.RunHeartbeat
	ConvergenceStatus     *projectioncontract.ConvergenceStatus
	Runtime               *projectioncontract.RuntimeProgress
	Activity              projectioncontract.WorkspaceActivityFeed
	Progress              progressState
	ProgressText          string
	RepairLoop            string
	FailureReason         string
	FailureHint           string
	Running               bool
	BlockingDecision      *projectioncontract.Decision
	BlockingApproval      *projectioncontract.Approval
	GovernanceDisplay     projectioncontract.GovernanceDisplay
	ConsistencyDiagnostic *projectioncontract.ConsistencyDiagnostic
	Decisions             []projectioncontract.Decision
	Approvals             []projectioncontract.Approval
	LastError             error
}

// workspaceRuntimeState is the TUI ViewModel for one workspace.
// Server projection remains authoritative; local fields only protect UI
// command intent while projection refresh is catching up.
type workspaceRuntimeState struct {
	Overview              projectioncontract.Workspace
	HasOverview           bool
	Projection            projectioncontract.ProjectionStatus
	RuntimeView           projectioncontract.WorkspaceRuntimeView
	HasRuntimeView        bool
	AppliedRevision       projectioncontract.WorkspaceRefreshRevision
	DirtyRevision         projectioncontract.WorkspaceRefreshRevision
	PromptMode            string
	PromptModeRevision    int64
	Display               workspaceDisplayState
	HasDisplay            bool
	DisplayRevision       int64
	LastRevision          int64
	RevisionGapCount      int64
	LastRevisionGap       int64
	ContextRequestSeq     int64
	ContextInFlight       bool
	ContextPending        bool
	ActivityDirtyRevision int64
	FollowTail            bool
	UnreadUpdateCount     int
	GovernanceCommand     governanceCommandTechnicalState
}

const (
	workspacePromptModeWorkspace  = "workspace"
	workspacePromptModeRunning    = "running"
	workspacePromptModeDecision   = "decision"
	workspacePromptModeApproval   = "approval"
	workspacePromptModeDefinition = "definition_clarification"
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
		entry = &workspaceRuntimeState{FollowTail: true}
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

func findWorkspace(workspaces []projectioncontract.Workspace, id string) *projectioncontract.Workspace {
	for i := range workspaces {
		if string(workspaces[i].ID) == id {
			return &workspaces[i]
		}
	}
	return nil
}

func projectionRevision(input projectioncontract.ProjectionStatus) int64 {
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

func viewModelRevision(input projectioncontract.WorkspaceViewModel) int64 {
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
	if msg.RuntimeView != nil {
		if revision := msg.RuntimeView.Revision.WorkspaceRevision; revision > 0 {
			return revision
		}
		if revision := projectionRevision(msg.RuntimeView.Projection); revision > 0 {
			return revision
		}
	}
	if msg.Snapshot != nil {
		if revision := viewModelRevision(msg.Snapshot.ViewModel); revision > 0 {
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

func mergeProjectionInfo(current, incoming projectioncontract.ProjectionStatus) projectioncontract.ProjectionStatus {
	if projectionRevision(incoming) == 0 && incoming.GlobalRevision == 0 && incoming.State == "" {
		return current
	}
	if projectionRevision(current) > 0 && projectionRevision(incoming) > 0 && projectionRevision(incoming) < projectionRevision(current) {
		return current
	}
	return incoming
}

func (m *AppModel) activeWorkspaceOverview() *projectioncontract.Workspace {
	if entry := m.workspaceRuntimeState(m.activeWorkspaceID); entry != nil && entry.HasOverview {
		return &entry.Overview
	}
	return findWorkspace(m.workspaces, m.activeWorkspaceID)
}

// activeWorkspaceSealedTarget is display-only data copied from the current
// Run read model. It is deliberately not inferred from the mutable workspace
// preference and is never used to construct a Resume request.
func (m *AppModel) activeWorkspaceSealedSelection() *runtimecontract.ProviderInferenceSelection {
	entry := m.workspaceRuntimeState(m.activeWorkspaceID)
	if entry == nil {
		return nil
	}
	return runtimecontract.CloneProviderInferenceSelection(entry.Display.SealedLLMSelection)
}

func workspaceOverviewHasProjectionAuthority(ws *projectioncontract.Workspace) bool {
	if ws == nil {
		return false
	}
	if viewModelRevision(ws.ViewModel) > 0 {
		return true
	}
	return ws.Projection.WorkspaceRevision > 0 || ws.Projection.ProjectedSourceRevision > 0
}

func promptModeFromWorkspaceOverview(ws *projectioncontract.Workspace) string {
	if ws == nil {
		return workspacePromptModeWorkspace
	}
	if workspaceOverviewHasProjectionAuthority(ws) {
		if mode := normalizePromptMode(ws.ViewModel.PromptMode); mode != workspacePromptModeWorkspace {
			return mode
		}
	}
	if ws.State == string(common.WorkspaceStateActive) && ws.ActiveRunID != "" {
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

func (m *AppModel) applyWorkspaceViewModel(viewModel projectioncontract.WorkspaceViewModel) {
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
	if revision == 0 {
		return
	}
	if entry.PromptModeRevision > 0 && revision > 0 && revision < entry.PromptModeRevision {
		return
	}
	entry.PromptMode = normalizePromptMode(viewModel.PromptMode)
	entry.PromptModeRevision = revision
}

func (m *AppModel) activeWorkspacePromptMode() string {
	if m.activeDefinitionClarificationDecision() != nil {
		return workspacePromptModeDefinition
	}
	entry := m.workspaceRuntimeState(m.activeWorkspaceID)
	if entry != nil && entry.PromptMode != "" && entry.PromptModeRevision > 0 {
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

func (m *AppModel) activeWorkspaceNeedsContextRefresh(previousActive string) bool {
	workspaceID := strings.TrimSpace(m.activeWorkspaceID)
	if workspaceID == "" {
		return false
	}
	entry := m.workspaceRuntimeState(workspaceID)
	if entry == nil {
		return true
	}
	if entry.ContextInFlight || entry.ContextPending {
		return false
	}
	if previousActive != "" && previousActive != workspaceID {
		return true
	}
	if !entry.HasDisplay {
		return true
	}
	overviewRevision := projectionRevision(entry.Overview.Projection)
	if overviewRevision == 0 {
		return false
	}
	return entry.DisplayRevision == 0 || entry.DisplayRevision < overviewRevision
}

func (m *AppModel) mergeWorkspaceOverview(ws projectioncontract.Workspace) projectioncontract.Workspace {
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

func (entry *workspaceRuntimeState) normalizeWorkspaceOverview(ws projectioncontract.Workspace) projectioncontract.Workspace {
	// Owner fields are copied exactly. Governance/lifecycle handoff lives in the
	// typed display projection and is never normalized back into Workspace.
	return ws
}

func (m *AppModel) replaceWorkspaceList(workspaces []projectioncontract.Workspace, projection projectioncontract.ProjectionStatus) {
	next := make([]projectioncontract.Workspace, 0, len(workspaces))
	for _, ws := range workspaces {
		if ws.Projection.State == "" && projectionRevision(ws.Projection) == 0 {
			ws.Projection = projection
		}
		next = append(next, m.mergeWorkspaceOverview(ws))
	}
	m.workspaces = next
}

func workspaceIndex(workspaces []projectioncontract.Workspace, id string) int {
	for i := range workspaces {
		if string(workspaces[i].ID) == id {
			return i
		}
	}
	return -1
}

func (m *AppModel) removeWorkspace(id common.WorkspaceID) {
	if id == "" {
		return
	}
	if m.modelSelectWorkspaceID == string(id) {
		m.cancelModelSelection()
	}
	if m.workspaceRuntimeStates != nil {
		delete(m.workspaceRuntimeStates, string(id))
	}
	next := m.workspaces[:0]
	for _, ws := range m.workspaces {
		if ws.ID != string(id) {
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
	m.cancelModelSelection()
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
		if ws.State == string(common.WorkspaceStateActive) {
			return string(ws.ActiveRunID)
		}
		return ""
	}
	if strings.TrimSpace(m.activeRunID) == "" {
		return ""
	}
	return m.activeRunID
}

func (m *AppModel) activeWorkspaceStateIsLive() bool {
	ws := m.activeWorkspaceOverview()
	if ws == nil {
		return false
	}
	return ws.State == string(common.WorkspaceStateActive) && ws.ActiveRunID != ""
}

func (m *AppModel) activeGovernanceDisplay() projectioncontract.GovernanceDisplay {
	entry := m.workspaceRuntimeState(m.activeWorkspaceID)
	if entry == nil {
		return projectioncontract.GovernanceDisplay{}
	}
	if entry.HasRuntimeView {
		return entry.RuntimeView.GovernanceDisplay
	}
	return entry.Display.GovernanceDisplay
}

func (m *AppModel) activeGovernancePromptMode() string {
	if mode := normalizePromptMode(m.activeGovernanceDisplay().PromptMode); mode != workspacePromptModeWorkspace {
		return mode
	}
	if entry := m.workspaceRuntimeState(m.activeWorkspaceID); entry != nil {
		if mode := normalizePromptMode(entry.PromptMode); mode != workspacePromptModeWorkspace {
			return mode
		}
	}
	return promptModeFromWorkspaceOverview(m.activeWorkspaceOverview())
}

func (m *AppModel) activeWorkspaceBlockerProgress() (progressState, bool) {
	if m.activeBlockingDecision() != nil {
		return progressWaitingDecision, true
	}
	if m.activeDefinitionClarificationDecision() != nil {
		return progressWaitingDecision, true
	}
	if m.activeBlockingApproval() != nil {
		return progressWaitingApproval, true
	}
	// Workspace is the sole owner of this lifecycle state. During the bounded
	// handoff window the Workspace projection can already be blocked while the
	// exact Approval resource has not reached the TUI read model yet. Treat that
	// owner fact as a refresh-only barrier: it must never fall through to Goal,
	// DoD, Restart, or Resume mutation selection.
	if workspace := m.activeWorkspaceOverview(); workspace != nil &&
		common.WorkspaceState(strings.TrimSpace(workspace.State)) == common.WorkspaceStateBlockedOnApproval &&
		strings.TrimSpace(workspace.ActiveRunID) != "" {
		return progressWaitingApproval, true
	}
	return "", false
}

func (m *AppModel) activeWorkspaceHasDecisionBlocker() bool {
	return m.activeBlockingDecision() != nil
}

func (m *AppModel) activeWorkspaceHasApprovalBlocker() bool {
	return m.activeBlockingApproval() != nil
}

func (m *AppModel) ensureActiveBlockingDecisionFromOverview() bool {
	if m.activeGovernancePromptMode() != workspacePromptModeDecision {
		return false
	}
	m.running = false
	m.progress = progressWaitingDecision
	if definition := m.activeDefinitionClarificationDecision(); definition != nil {
		m.progressText = definitionClarificationInstruction(definition)
	} else if decision := m.activeBlockingDecision(); decision != nil {
		m.progressText = nonEmpty(blockingDecisionLabel(decision), "Waiting for decision")
	} else {
		m.progressText = "Decision lifecycle handoff pending; refreshing owner projections"
	}
	return true
}

func (m *AppModel) ensureActiveBlockingApprovalFromOverview() bool {
	if m.activeGovernancePromptMode() != workspacePromptModeApproval {
		return false
	}
	m.running = false
	m.progress = progressWaitingApproval
	if approval := m.activeBlockingApproval(); approval != nil {
		m.progressText = nonEmpty(blockingApprovalLabel(approval), "Waiting for approval")
	} else {
		m.progressText = "Approval lifecycle handoff pending; refreshing owner projections"
	}
	return true
}

func (m *AppModel) syncActiveWorkspaceBlockerOverview() tea.Cmd {
	ws := m.activeWorkspaceOverview()
	if ws == nil {
		return nil
	}
	switch ws.State {
	case string(common.WorkspaceStateActive), string(common.WorkspaceStateBlockedOnApproval):
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
	case string(common.WorkspaceStateActive):
		return m.ensureActiveBlockingDecisionFromOverview()
	case string(common.WorkspaceStateBlockedOnApproval):
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
		if definition := m.activeDefinitionClarificationDecision(); definition != nil {
			m.progressText = nonEmpty(reason, definitionClarificationInstruction(definition))
		} else {
			m.progressText = nonEmpty(reason, blockingDecisionLabel(m.activeBlockingDecision()), "Waiting for decision; refreshing blocker details")
		}
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
	defer m.markWorkspaceContextDisplayLoaded(workspaceID, msg)
	defer m.settleGovernanceCommandFromProjection(workspaceID)
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
		m.decisions = append([]projectioncontract.Decision(nil), msg.Decisions...)
	}
	if msg.Approvals != nil {
		m.approvals = append([]projectioncontract.Approval(nil), msg.Approvals...)
	}

	workspaceHasLive := msg.WorkspaceHasLive || msg.Workspace.ActiveRunID != ""
	activeRun := msg.ActiveRun
	displayRun := msg.DisplayRun
	if entry := m.ensureWorkspaceRuntimeState(workspaceID); entry != nil {
		if workspaceHasLive && activeRun != nil {
			entry.Display.SealedLLMSelection = runtimecontract.CloneProviderInferenceSelection(activeRun.SealedLLMSelection)
		} else {
			entry.Display.SealedLLMSelection = nil
		}
	}
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
		m.applyWorkspaceProgress(common.WorkspaceState(msg.Workspace.State))
		if msg.Snapshot != nil {
			if entry := m.ensureWorkspaceRuntimeState(workspaceID); entry != nil {
				entry.Display.GovernanceDisplay = msg.Snapshot.GovernanceDisplay
				entry.Display.ConsistencyDiagnostic = projectioncontract.CloneConsistencyDiagnostic(msg.Snapshot.ConsistencyDiagnostic)
			}
			m.applyRunHeartbeat(msg.Snapshot.RunHeartbeat)
			m.applyConvergenceStatus(msg.Snapshot.ConvergenceStatus)
			m.applyRunCommandStatus(msg.Snapshot.RunCommand)
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
		if entry := m.ensureWorkspaceRuntimeState(workspaceID); entry != nil {
			entry.Display.GovernanceDisplay = msg.Snapshot.GovernanceDisplay
			entry.Display.ConsistencyDiagnostic = projectioncontract.CloneConsistencyDiagnostic(msg.Snapshot.ConsistencyDiagnostic)
		}
		if runStateIsTerminal(common.RunState(run.State)) {
			m.result = snapshotDisplayResult(msg.Snapshot, common.RunID(run.ID))
		} else {
			m.result = ""
		}
		m.artifacts = snapshotArtifactLabels(msg.Snapshot, common.RunID(run.ID))
		m.sources = snapshotWebSourceLabels(msg.Snapshot, common.RunID(run.ID))
		m.patches = snapshotPatches(msg.Snapshot, common.RunID(run.ID))
		m.artifactContinuations = snapshotArtifactContinuations(msg.Snapshot, common.RunID(run.ID))
		m.taskDecomp = snapshotTaskDecompositions(msg.Snapshot, common.RunID(run.ID))
		m.taskPlans = snapshotTaskPlans(msg.Snapshot, common.RunID(run.ID))
		m.memoryRecalls = append([]projectioncontract.MemoryRecallTrace(nil), msg.Snapshot.MemoryRecallTraces...)
		m.llmUsage = msg.Snapshot.LLMUsageSummary
		m.llmContextSummary = msg.Snapshot.LLMContextBudgetSummary
		m.llmContext = latestLLMContextBudget(msg.Snapshot.LLMContextBudgets, msg.Snapshot.LLMUsageTraces)
		m.acceptance = msg.Snapshot.Acceptance
		m.applyRunHeartbeat(msg.Snapshot.RunHeartbeat)
		m.applyConvergenceStatus(msg.Snapshot.ConvergenceStatus)
		m.applyRunCommandStatus(msg.Snapshot.RunCommand)
		m.repairLoop = repairLoopSummary(msg.Snapshot.TaskProgress)
	}

	state := common.RunState(run.State)
	payload := map[string]any{
		backend.PayloadKeyFailureReason:      run.FailureReason,
		backend.PayloadKeyBlockingTicketID:   string(run.BlockingTicketID),
		backend.PayloadKeyBlockingApprovalID: string(run.BlockingApprovalID),
	}
	if msg.Status != nil {
		state = common.RunState(msg.Status.State)
		payload[backend.PayloadKeyFailureReason] = msg.Status.FailureReason
		payload[backend.PayloadKeyBlockingTicketID] = string(msg.Status.BlockingTicketID)
		payload[backend.PayloadKeyBlockingApprovalID] = string(msg.Status.BlockingApprovalID)
	}
	if msg.Snapshot != nil && payload[backend.PayloadKeyFailureReason] == "" {
		payload[backend.PayloadKeyFailureReason] = msg.Snapshot.DisplayOutcome.Reason
	}
	if workspaceHasLive || runStateIsTerminal(state) {
		m.applyRunState(state, payload)
	} else {
		m.applyWorkspaceProgress(common.WorkspaceState(msg.Workspace.State))
	}
	if msg.Status != nil && msg.Status.CurrentStepKind != "" && m.progress == progressRunning {
		m.progressText = stepProgressText("Working", msg.Status.CurrentStepKind, "Working; waiting for final result")
	}
	if msg.Status != nil {
		m.applyConvergenceStatus(msg.Status.ConvergenceStatus)
	}
	if msg.Snapshot != nil && strings.TrimSpace(msg.Snapshot.TaskProgress.Summary) != "" && m.progress == progressRunning {
		m.progressText = strings.TrimSpace(msg.Snapshot.TaskProgress.Summary)
	}
	m.applyCanonicalRunBlockers(activeRun)
	m.syncBlockingDecisionFromList()
	m.syncBlockingApprovalFromList()
	if msg.RuntimeView != nil {
		m.applyWorkspaceRuntimeView(*msg.RuntimeView, msg.ResetMainViewport)
	} else if msg.Snapshot != nil && msg.Snapshot.TaskProgress.Runtime != nil {
		m.runtime = msg.Snapshot.TaskProgress.Runtime
		if summary := strings.TrimSpace(msg.Snapshot.TaskProgress.Summary); summary != "" && m.progress == progressRunning {
			m.progressText = summary
		}
	}
	if msg.ResetMainViewport {
		m.followMainViewportTail()
	} else if strings.TrimSpace(m.result) != "" {
		m.followMainViewportTailUnlessUserScrolled()
	} else {
		m.followMainViewportTailUnlessUserScrolled()
	}
}

func (m *AppModel) applyWorkspaceRuntimeView(view projectioncontract.WorkspaceRuntimeView, resetMainViewport bool) {
	// Scope must come from the typed RuntimeView response itself. Empty or
	// mismatched WorkspaceID must never fall back to active workspace cache.
	workspaceID := strings.TrimSpace(string(view.WorkspaceID))
	if workspaceID == "" {
		return
	}
	entry := m.ensureWorkspaceRuntimeState(workspaceID)
	if entry == nil {
		return
	}
	if !runtimeViewRevisionAcceptable(entry.AppliedRevision, view.Revision) {
		return
	}
	// CR04: equal revision + equal effective display content is a full no-op.
	// Do not update viewport, unread count, dirty state, or cache identity.
	if entry.HasRuntimeView &&
		runtimeRefreshRevisionEqual(entry.AppliedRevision, view.Revision) &&
		runtimeViewDisplayContentEqual(entry.RuntimeView, view) {
		return
	}
	defer m.settleGovernanceCommandFromProjection(workspaceID)
	entry.RuntimeView = view
	entry.HasRuntimeView = true
	entry.AppliedRevision = view.Revision
	entry.DirtyRevision = clearDirtyAtOrBelow(entry.DirtyRevision, view.Revision)
	entry.Projection = view.Projection
	if view.PromptMode != "" {
		entry.PromptMode = view.PromptMode
		if view.Revision.WorkspaceRevision > 0 {
			entry.PromptModeRevision = view.Revision.WorkspaceRevision
		}
	}
	if workspaceID != m.activeWorkspaceID {
		// non-active workspace: only update typed cache, not main view.
		entry.Display.Runtime = cloneRuntime(view.TaskProgress.Runtime)
		entry.Display.Activity = cloneActivityFeed(view.Activity)
		entry.Display.Acceptance = view.Acceptance
		entry.Display.RunHeartbeat = view.RunHeartbeat
		entry.Display.ConvergenceStatus = cloneConvergenceStatusInfoPtr(view.ConvergenceStatus)
		entry.Display.BlockingDecision = cloneDecisionPtr(view.BlockingDecision)
		entry.Display.BlockingApproval = cloneApprovalPtr(view.BlockingApproval)
		entry.Display.GovernanceDisplay = view.GovernanceDisplay
		entry.Display.ConsistencyDiagnostic = projectioncontract.CloneConsistencyDiagnostic(view.ConsistencyDiagnostic)
		entry.Display.ActiveRunID = string(view.ActiveRunID)
		entry.Display.DisplayRunID = string(view.DisplayRunID)
		entry.HasDisplay = true
		entry.ActivityDirtyRevision = 0
		return
	}

	m.runtime = cloneRuntime(view.TaskProgress.Runtime)
	m.activityFeed = cloneActivityFeed(view.Activity)
	m.acceptance = view.Acceptance
	m.runHeartbeat = view.RunHeartbeat
	m.convergenceStatus = cloneConvergenceStatusInfoPtr(view.ConvergenceStatus)
	m.runCommand = cloneRunCommandPtr(view.RunCommand)
	// RuntimeView blocker resources are emitted only from the canonical active
	// Run's exact blocker reference. Always apply nil as well, so a newer owner
	// projection can revoke a stale local mutation target.
	m.applyCanonicalRuntimeViewBlockers(view)
	if string(view.ActiveRunID) != "" {
		m.activeRunID = string(view.ActiveRunID)
		m.running = !runStateIsTerminal(common.RunState(view.RunState))
	}
	if summary := strings.TrimSpace(view.TaskProgress.Summary); summary != "" {
		if m.progress == progressRunning || m.running {
			m.progressText = summary
		}
	}
	if view.DisplayOutcome.Terminal && strings.TrimSpace(view.DisplayOutcome.Summary) != "" {
		m.result = strings.TrimSpace(view.DisplayOutcome.Summary)
	}
	if view.DisplayOutcome.Terminal && !view.DisplayOutcome.Accepted {
		m.failureReason = nonEmpty(view.DisplayOutcome.Reason, view.DisplayOutcome.Summary)
	}
	m.repairLoop = repairLoopSummary(view.TaskProgress)

	entry.Display.Runtime = cloneRuntime(view.TaskProgress.Runtime)
	entry.Display.Activity = cloneActivityFeed(view.Activity)
	entry.Display.Acceptance = view.Acceptance
	entry.Display.RunHeartbeat = view.RunHeartbeat
	entry.Display.ConvergenceStatus = cloneConvergenceStatusInfoPtr(view.ConvergenceStatus)
	entry.Display.BlockingDecision = cloneDecisionPtr(view.BlockingDecision)
	entry.Display.BlockingApproval = cloneApprovalPtr(view.BlockingApproval)
	entry.Display.GovernanceDisplay = view.GovernanceDisplay
	entry.Display.ConsistencyDiagnostic = projectioncontract.CloneConsistencyDiagnostic(view.ConsistencyDiagnostic)
	entry.Display.ActiveRunID = string(view.ActiveRunID)
	entry.Display.DisplayRunID = string(view.DisplayRunID)
	entry.Display.ProgressText = m.progressText
	entry.Display.Running = m.running
	entry.HasDisplay = true
	entry.ActivityDirtyRevision = 0
	if view.Revision.WorkspaceRevision > entry.DisplayRevision {
		entry.DisplayRevision = view.Revision.WorkspaceRevision
	}

	if resetMainViewport {
		m.followMainViewportTail()
		return
	}
	m.followMainViewportTailUnlessUserScrolled()
}

func clearDirtyAtOrBelow(dirty, applied projectioncontract.WorkspaceRefreshRevision) projectioncontract.WorkspaceRefreshRevision {
	if revisionGenerationChanged(dirty, applied) {
		// A RuntimeView from a new server generation is the authority for a new
		// comparison domain. Old-generation dirty counters cannot be compared.
		return projectioncontract.WorkspaceRefreshRevision{}
	}
	if dirty.WorkspaceRevision > 0 && dirty.WorkspaceRevision <= applied.WorkspaceRevision {
		dirty.WorkspaceRevision = 0
	}
	if dirty.RuntimeStateRevision > 0 && dirty.RuntimeStateRevision <= applied.RuntimeStateRevision {
		dirty.RuntimeStateRevision = 0
	}
	if dirty.ActivityRevision > 0 && dirty.ActivityRevision <= applied.ActivityRevision {
		dirty.ActivityRevision = 0
	}
	if dirty.ProjectedSourceRevision > 0 && dirty.ProjectedSourceRevision <= applied.ProjectedSourceRevision {
		dirty.ProjectedSourceRevision = 0
	}
	if dirty.EvidenceSequence > 0 && dirty.EvidenceSequence <= applied.EvidenceSequence {
		dirty.EvidenceSequence = 0
	}
	if dirty.StreamRevision > 0 && dirty.StreamRevision <= applied.StreamRevision {
		dirty.StreamRevision = 0
	}
	return dirty
}

func runtimeRefreshRevisionEqual(a, b projectioncontract.WorkspaceRefreshRevision) bool {
	return strings.TrimSpace(a.StreamGeneration) == strings.TrimSpace(b.StreamGeneration) &&
		a.StreamRevision == b.StreamRevision &&
		a.WorkspaceRevision == b.WorkspaceRevision &&
		a.ProjectedSourceRevision == b.ProjectedSourceRevision &&
		a.RuntimeStateRevision == b.RuntimeStateRevision &&
		a.EvidenceSequence == b.EvidenceSequence &&
		a.ActivityRevision == b.ActivityRevision
}

// runtimeViewDisplayContentEqual compares every field that applyWorkspaceRuntimeView
// applies or that the main view can render. Equal revision responses that still
// change any applied display field are accepted; fully identical content is a
// no-op (CR04/FR02).
func runtimeViewDisplayContentEqual(a, b projectioncontract.WorkspaceRuntimeView) bool {
	if strings.TrimSpace(string(a.WorkspaceID)) != strings.TrimSpace(string(b.WorkspaceID)) ||
		strings.TrimSpace(string(a.ActiveRunID)) != strings.TrimSpace(string(b.ActiveRunID)) ||
		strings.TrimSpace(string(a.DisplayRunID)) != strings.TrimSpace(string(b.DisplayRunID)) ||
		strings.TrimSpace(a.PromptMode) != strings.TrimSpace(b.PromptMode) ||
		string(a.RunState) != string(b.RunState) ||
		strings.TrimSpace(a.TaskProgress.Summary) != strings.TrimSpace(b.TaskProgress.Summary) ||
		strings.TrimSpace(a.TaskProgress.State) != strings.TrimSpace(b.TaskProgress.State) ||
		strings.TrimSpace(repairLoopSummary(a.TaskProgress)) != strings.TrimSpace(repairLoopSummary(b.TaskProgress)) ||
		!projectionInfoDisplayEqual(a.Projection, b.Projection) {
		return false
	}
	if !workspaceOutcomeEqual(a.DisplayOutcome, b.DisplayOutcome) {
		return false
	}
	if a.GovernanceDisplay != b.GovernanceDisplay ||
		!reflect.DeepEqual(a.ConsistencyDiagnostic, b.ConsistencyDiagnostic) {
		return false
	}
	if !runtimeProgressEqual(a.TaskProgress.Runtime, b.TaskProgress.Runtime) {
		return false
	}
	if !activityFeedDisplayEqual(a.Activity, b.Activity) {
		return false
	}
	if !decisionInfoEqual(a.BlockingDecision, b.BlockingDecision) {
		return false
	}
	if !approvalInfoEqual(a.BlockingApproval, b.BlockingApproval) {
		return false
	}
	if !acceptanceProjectionEqual(a.Acceptance, b.Acceptance) {
		return false
	}
	if !runHeartbeatEqual(a.RunHeartbeat, b.RunHeartbeat) {
		return false
	}
	if !convergenceStatusEqual(a.ConvergenceStatus, b.ConvergenceStatus) {
		return false
	}
	if !runCommandStatusEqual(a.RunCommand, b.RunCommand) {
		return false
	}
	return true
}

// projectionInfoDisplayEqual covers the projection state copied into the
// workspace cache by applyWorkspaceRuntimeView. A same-revision transition to
// stale/degraded must not be swallowed by the RuntimeView no-op gate.
func projectionInfoDisplayEqual(a, b projectioncontract.ProjectionStatus) bool {
	return strings.TrimSpace(a.State) == strings.TrimSpace(b.State) &&
		a.GlobalRevision == b.GlobalRevision &&
		a.WorkspaceRevision == b.WorkspaceRevision &&
		a.SourceEventRevision == b.SourceEventRevision &&
		a.ProjectedSourceRevision == b.ProjectedSourceRevision &&
		strings.TrimSpace(a.UpdatedAt) == strings.TrimSpace(b.UpdatedAt) &&
		strings.TrimSpace(a.RebuiltAt) == strings.TrimSpace(b.RebuiltAt) &&
		a.Degraded == b.Degraded &&
		strings.TrimSpace(a.StaleReason) == strings.TrimSpace(b.StaleReason) &&
		strings.TrimSpace(a.Error) == strings.TrimSpace(b.Error)
}

func workspaceOutcomeEqual(a, b projectioncontract.WorkspaceDisplayOutcome) bool {
	if strings.TrimSpace(string(a.WorkspaceID)) != strings.TrimSpace(string(b.WorkspaceID)) ||
		strings.TrimSpace(a.State) != strings.TrimSpace(b.State) ||
		strings.TrimSpace(string(a.RunID)) != strings.TrimSpace(string(b.RunID)) ||
		strings.TrimSpace(string(a.ResultID)) != strings.TrimSpace(string(b.ResultID)) ||
		a.Accepted != b.Accepted ||
		a.Terminal != b.Terminal ||
		strings.TrimSpace(a.Summary) != strings.TrimSpace(b.Summary) ||
		strings.TrimSpace(a.Reason) != strings.TrimSpace(b.Reason) {
		return false
	}
	return stringSliceEqualDisplay(a.EvidenceRefs, b.EvidenceRefs) &&
		stringSliceEqualDisplay(a.ArtifactRefs, b.ArtifactRefs)
}

func acceptanceProjectionEqual(a, b projectioncontract.AcceptanceProjection) bool {
	if strings.TrimSpace(a.ActiveSliceID) != strings.TrimSpace(b.ActiveSliceID) ||
		a.Summary.AcceptedSlices != b.Summary.AcceptedSlices ||
		a.Summary.ActiveSlices != b.Summary.ActiveSlices ||
		a.Summary.OpenSlices != b.Summary.OpenSlices ||
		a.Summary.FailedSlices != b.Summary.FailedSlices {
		return false
	}
	if !coverageFocusEqual(a.CurrentFocus, b.CurrentFocus) {
		return false
	}
	if !convergenceBudgetEqual(a.Budget, b.Budget) {
		return false
	}
	if !runtimeRecoveryBudgetEqual(a.RecoveryBudget, b.RecoveryBudget) {
		return false
	}
	if len(a.Slices) != len(b.Slices) {
		return false
	}
	for i := range a.Slices {
		left, right := a.Slices[i], b.Slices[i]
		if strings.TrimSpace(left.ID) != strings.TrimSpace(right.ID) ||
			strings.TrimSpace(left.Title) != strings.TrimSpace(right.Title) ||
			strings.TrimSpace(left.State) != strings.TrimSpace(right.State) ||
			left.Priority != right.Priority ||
			strings.TrimSpace(left.Scope) != strings.TrimSpace(right.Scope) ||
			strings.TrimSpace(left.CompletionPolicy) != strings.TrimSpace(right.CompletionPolicy) ||
			strings.TrimSpace(left.ResultPolicy) != strings.TrimSpace(right.ResultPolicy) ||
			!stringSliceEqualDisplay(left.CoverageWorkItemIDs, right.CoverageWorkItemIDs) ||
			!stringSliceEqualDisplay(left.RequiredEvidenceKinds, right.RequiredEvidenceKinds) {
			return false
		}
	}
	return runtimeRepairDirectiveEqual(a.RepairDirective, b.RepairDirective) &&
		runtimeAuditEqual(a.Audit, b.Audit)
}

func runtimeRepairDirectiveEqual(a, b *projectioncontract.RuntimeRepairDirective) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.ID != b.ID || a.FocusID != b.FocusID || a.Trigger != b.Trigger || a.RequiredAction != b.RequiredAction ||
		a.SupersedesEvidenceRevision != b.SupersedesEvidenceRevision || a.RepairAttempt != b.RepairAttempt ||
		!stringSliceEqualDisplay(a.RequiredEffectScopes, b.RequiredEffectScopes) ||
		!stringSliceEqualDisplay(a.RequiredEvidenceKinds, b.RequiredEvidenceKinds) ||
		!stringSliceEqualDisplay(a.AllowedOperationKinds, b.AllowedOperationKinds) ||
		!stringSliceEqualDisplay(a.CoverageWorkItemIDs, b.CoverageWorkItemIDs) ||
		!stringSliceEqualDisplay(a.TargetRefs, b.TargetRefs) {
		return false
	}
	if a.Closure == nil || b.Closure == nil {
		return a.Closure == b.Closure
	}
	return a.Closure.ID == b.Closure.ID && a.Closure.GapSignature == b.Closure.GapSignature &&
		a.Closure.BaselineEvidenceRevision == b.Closure.BaselineEvidenceRevision &&
		a.Closure.RequireValidationAfterStateChange == b.Closure.RequireValidationAfterStateChange &&
		stringSliceEqualDisplay(a.Closure.CoverageWorkItemIDs, b.Closure.CoverageWorkItemIDs) &&
		stringSliceEqualDisplay(a.Closure.RequiredEvidenceKinds, b.Closure.RequiredEvidenceKinds) &&
		stringSliceEqualDisplay(a.Closure.GroundedTargetRefs, b.Closure.GroundedTargetRefs)
}

func runtimeAuditEqual(a, b *projectioncontract.RuntimeAudit) bool {
	// The canonical audit projection intentionally carries the complete
	// revision, grounding, authority and terminal lineage. Compare the full
	// projection before the focused field checks below so newly added audit
	// feedback can never be ignored by TUI refresh detection.
	if !reflect.DeepEqual(a, b) {
		return false
	}
	if a == nil || b == nil {
		return a == b
	}
	if a.Repair.FocusID != b.Repair.FocusID || a.Repair.SliceID != b.Repair.SliceID ||
		a.Repair.RepairAttempts != b.Repair.RepairAttempts || a.Repair.ValidationOnlyRetries != b.Repair.ValidationOnlyRetries ||
		a.Repair.NoProgressCount != b.Repair.NoProgressCount || a.Repair.NoProgressOperationShapeCount != b.Repair.NoProgressOperationShapeCount ||
		a.Repair.ActiveGapFailureClass != b.Repair.ActiveGapFailureClass || a.Repair.ActiveGapFailureCode != b.Repair.ActiveGapFailureCode ||
		a.Repair.ActiveGapFailureSignature != b.Repair.ActiveGapFailureSignature || a.Repair.LastProgressRevision != b.Repair.LastProgressRevision ||
		a.Repair.Exhausted != b.Repair.Exhausted ||
		!stringSliceEqualDisplay(a.Repair.ActiveGapCoverageWorkItemIDs, b.Repair.ActiveGapCoverageWorkItemIDs) ||
		!runtimeAuditFailureEqual(a.Failure, b.Failure) || len(a.Assessments) != len(b.Assessments) ||
		len(a.CoverageBindings) != len(b.CoverageBindings) || len(a.Dispatches) != len(b.Dispatches) || len(a.AcceptedEvidence) != len(b.AcceptedEvidence) ||
		len(a.RejectedEvidence) != len(b.RejectedEvidence) || len(a.OperationReceipts) != len(b.OperationReceipts) {
		return false
	}
	for idx := range a.Assessments {
		if !runtimeAuditAssessmentEqual(a.Assessments[idx], b.Assessments[idx]) {
			return false
		}
	}
	for idx := range a.CoverageBindings {
		if !runtimeAuditCoverageBindingEqual(a.CoverageBindings[idx], b.CoverageBindings[idx]) {
			return false
		}
	}
	for idx := range a.Dispatches {
		if !runtimeAuditDispatchEqual(a.Dispatches[idx], b.Dispatches[idx]) {
			return false
		}
	}
	for idx := range a.AcceptedEvidence {
		if !runtimeAuditEvidenceEqual(a.AcceptedEvidence[idx], b.AcceptedEvidence[idx]) {
			return false
		}
	}
	for idx := range a.RejectedEvidence {
		if !runtimeAuditEvidenceEqual(a.RejectedEvidence[idx], b.RejectedEvidence[idx]) {
			return false
		}
	}
	for idx := range a.OperationReceipts {
		if !runtimeAuditOperationReceiptEqual(a.OperationReceipts[idx], b.OperationReceipts[idx]) {
			return false
		}
	}
	return true
}

func runtimeAuditDispatchEqual(a, b projectioncontract.RuntimeAuditDispatch) bool {
	if a.ID != b.ID || a.AuthoritySnapshotID != b.AuthoritySnapshotID || a.State != b.State ||
		a.CandidateHash != b.CandidateHash || a.CompletedOperations != b.CompletedOperations || len(a.Operations) != len(b.Operations) {
		return false
	}
	for idx := range a.Operations {
		left, right := a.Operations[idx], b.Operations[idx]
		if left.ID != right.ID || left.Kind != right.Kind || !stringSliceEqualDisplay(left.BindingRefs, right.BindingRefs) ||
			!stringSliceEqualDisplay(left.TargetRefs, right.TargetRefs) {
			return false
		}
	}
	return true
}

func runtimeAuditCoverageBindingEqual(a, b projectioncontract.RuntimeAuditCoverageBinding) bool {
	if a.ID != b.ID || a.CoverageWorkItemID != b.CoverageWorkItemID || a.FocusID != b.FocusID ||
		a.DefinitionRevision != b.DefinitionRevision || a.Hash != b.Hash || a.TargetSelectorRef != b.TargetSelectorRef ||
		a.TargetSelectorOwnerRef != b.TargetSelectorOwnerRef || a.TargetSelectorSubjectRef != b.TargetSelectorSubjectRef ||
		a.ProspectiveTargetRef != b.ProspectiveTargetRef || a.DeclarationRef != b.DeclarationRef ||
		!stringSliceEqualDisplay(a.TargetRefs, b.TargetRefs) || !stringSliceEqualDisplay(a.PrerequisiteRefs, b.PrerequisiteRefs) ||
		len(a.Proofs) != len(b.Proofs) {
		return false
	}
	for idx := range a.Proofs {
		left, right := a.Proofs[idx], b.Proofs[idx]
		if left.EvidenceKind != right.EvidenceKind || left.ValidationRecipeID != right.ValidationRecipeID ||
			!stringSliceEqualDisplay(left.RequiredEffectScopes, right.RequiredEffectScopes) ||
			!stringSliceEqualDisplay(left.AllowedOperationKinds, right.AllowedOperationKinds) ||
			!stringSliceEqualDisplay(left.AllowedOperationVariants, right.AllowedOperationVariants) ||
			!stringSliceEqualDisplay(left.RequiredCheckRefs, right.RequiredCheckRefs) ||
			!stringSliceEqualDisplay(left.MetricConstraints, right.MetricConstraints) ||
			!stringSliceEqualDisplay(left.ArtifactRefs, right.ArtifactRefs) {
			return false
		}
	}
	return true
}

func runtimeAuditEvidenceEqual(a, b projectioncontract.RuntimeAuditEvidence) bool {
	return a.ID == b.ID && a.Kind == b.Kind && a.AuthoritySnapshotID == b.AuthoritySnapshotID &&
		a.DispatchID == b.DispatchID && a.OperationID == b.OperationID && a.Revision == b.Revision &&
		a.FailureClass == b.FailureClass && a.FailureCode == b.FailureCode && a.FailureSignature == b.FailureSignature &&
		a.Quality == b.Quality && a.ZeroWork == b.ZeroWork && a.EvidenceReason == b.EvidenceReason &&
		a.ValidationRecipeID == b.ValidationRecipeID &&
		stringSliceEqualDisplay(a.TargetRefs, b.TargetRefs) && stringSliceEqualDisplay(a.BindingRefs, b.BindingRefs) &&
		stringSliceEqualDisplay(a.CoverageWorkItemIDs, b.CoverageWorkItemIDs) && stringSliceEqualDisplay(a.EvidenceRefs, b.EvidenceRefs) &&
		stringSliceEqualDisplay(a.EvidenceTags, b.EvidenceTags) && stringSliceEqualDisplay(a.ExecutedChecks, b.ExecutedChecks) &&
		stringSliceEqualDisplay(a.PassedChecks, b.PassedChecks) && stringSliceEqualDisplay(a.FailedChecks, b.FailedChecks) &&
		stringSliceEqualDisplay(a.SkippedChecks, b.SkippedChecks)
}

func runtimeAuditOperationReceiptEqual(a, b projectioncontract.RuntimeAuditOperationReceipt) bool {
	return a.ExecutionKey == b.ExecutionKey && a.Fence == b.Fence && a.OperationID == b.OperationID &&
		a.Outcome == b.Outcome && a.ObservedEffectScope == b.ObservedEffectScope && a.Changed == b.Changed &&
		a.FeedbackDomain == b.FeedbackDomain && a.StableCategory == b.StableCategory &&
		reflect.DeepEqual(a.OriginalDiagnostic, b.OriginalDiagnostic) && a.ValidationRecipeID == b.ValidationRecipeID &&
		stringSliceEqualDisplay(a.TargetRefs, b.TargetRefs) &&
		stringSliceEqualDisplay(a.EvidenceRefs, b.EvidenceRefs) && stringSliceEqualDisplay(a.EvidenceTags, b.EvidenceTags) &&
		stringSliceEqualDisplay(a.ExecutedChecks, b.ExecutedChecks) && stringSliceEqualDisplay(a.PassedChecks, b.PassedChecks) &&
		stringSliceEqualDisplay(a.FailedChecks, b.FailedChecks) && stringSliceEqualDisplay(a.SkippedChecks, b.SkippedChecks)
}

func runtimeAuditFailureEqual(a, b *projectioncontract.RuntimeAuditFailure) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.Class != b.Class || a.Code != b.Code || a.Signature != b.Signature || a.Message != b.Message ||
		!stringSliceEqualDisplay(a.TargetRefs, b.TargetRefs) {
		return false
	}
	return reflect.DeepEqual(a.OriginalDiagnostic, b.OriginalDiagnostic)
}

func runtimeAuditAssessmentEqual(a, b projectioncontract.RuntimeAuditAssessment) bool {
	if a.ID != b.ID || a.FocusID != b.FocusID || a.SliceID != b.SliceID || a.Scope != b.Scope ||
		a.RequirementID != b.RequirementID || a.RequirementText != b.RequirementText ||
		a.ClaimedDisposition != b.ClaimedDisposition || a.CanonicalDisposition != b.CanonicalDisposition ||
		a.Summary != b.Summary || !stringSliceEqualDisplay(a.CoverageWorkItemIDs, b.CoverageWorkItemIDs) ||
		!stringSliceEqualDisplay(a.EvidenceRefs, b.EvidenceRefs) || !stringSliceEqualDisplay(a.ArtifactRefs, b.ArtifactRefs) ||
		len(a.Items) != len(b.Items) {
		return false
	}
	for idx := range a.Items {
		left, right := a.Items[idx], b.Items[idx]
		if left.CoverageWorkItemID != right.CoverageWorkItemID || left.Satisfied != right.Satisfied || left.Reason != right.Reason ||
			!stringSliceEqualDisplay(left.EvidenceRefs, right.EvidenceRefs) || !stringSliceEqualDisplay(left.ArtifactRefs, right.ArtifactRefs) {
			return false
		}
	}
	if a.Gap == nil || b.Gap == nil {
		return a.Gap == b.Gap
	}
	return a.Gap.Reason == b.Gap.Reason && a.Gap.FailureClass == b.Gap.FailureClass && a.Gap.FailureCode == b.Gap.FailureCode &&
		a.Gap.FailureSignature == b.Gap.FailureSignature && stringSliceEqualDisplay(a.Gap.CoverageWorkItemIDs, b.Gap.CoverageWorkItemIDs) &&
		stringSliceEqualDisplay(a.Gap.MissingEvidenceKinds, b.Gap.MissingEvidenceKinds) && stringSliceEqualDisplay(a.Gap.TargetRefs, b.Gap.TargetRefs) &&
		stringSliceEqualDisplay(a.Gap.EvidenceRefs, b.Gap.EvidenceRefs)
}

func coverageFocusEqual(a, b *projectioncontract.CoverageFocus) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return strings.TrimSpace(a.ID) == strings.TrimSpace(b.ID) &&
		strings.TrimSpace(a.SliceID) == strings.TrimSpace(b.SliceID) &&
		strings.TrimSpace(a.FailureClass) == strings.TrimSpace(b.FailureClass) &&
		strings.TrimSpace(a.FailureSignature) == strings.TrimSpace(b.FailureSignature) &&
		strings.TrimSpace(a.State) == strings.TrimSpace(b.State) &&
		a.Attempt == b.Attempt &&
		stringSliceEqualDisplay(a.CoverageWorkItemIDs, b.CoverageWorkItemIDs) &&
		stringSliceEqualDisplay(a.RequiredEvidenceKinds, b.RequiredEvidenceKinds) &&
		stringSliceEqualDisplay(a.AcceptedEvidenceRefs, b.AcceptedEvidenceRefs) &&
		stringSliceEqualDisplay(a.RejectedEvidenceRefs, b.RejectedEvidenceRefs) &&
		stringSliceEqualDisplay(a.TargetRefs, b.TargetRefs)
}

func convergenceBudgetEqual(a, b *projectioncontract.ConvergenceBudget) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return strings.TrimSpace(a.FocusID) == strings.TrimSpace(b.FocusID) &&
		strings.TrimSpace(a.SliceID) == strings.TrimSpace(b.SliceID) &&
		strings.TrimSpace(a.State) == strings.TrimSpace(b.State) &&
		strings.TrimSpace(a.Decision) == strings.TrimSpace(b.Decision) &&
		a.MaxRepairAttemptsPerFocus == b.MaxRepairAttemptsPerFocus &&
		a.MaxLLMRoundsPerSlice == b.MaxLLMRoundsPerSlice &&
		a.MaxRepeatedFailureSignature == b.MaxRepeatedFailureSignature &&
		a.MaxValidationOnlyRetries == b.MaxValidationOnlyRetries &&
		a.MaxNoCoverageDeltaRounds == b.MaxNoCoverageDeltaRounds &&
		a.MaxContextBytesPerRepair == b.MaxContextBytesPerRepair &&
		a.RepairAttempts == b.RepairAttempts &&
		a.LLMRounds == b.LLMRounds &&
		a.RepeatedFailureSignatures == b.RepeatedFailureSignatures &&
		a.ValidationOnlyRetries == b.ValidationOnlyRetries &&
		a.NoCoverageDeltaRounds == b.NoCoverageDeltaRounds &&
		strings.TrimSpace(a.LastNoDeltaReason) == strings.TrimSpace(b.LastNoDeltaReason) &&
		strings.TrimSpace(a.LastDecisionReason) == strings.TrimSpace(b.LastDecisionReason) &&
		a.MaxPrerequisiteRefreshes == b.MaxPrerequisiteRefreshes &&
		a.PrerequisiteRefreshes == b.PrerequisiteRefreshes
}

func runtimeRecoveryBudgetEqual(a, b *projectioncontract.RuntimeRecoveryBudget) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return a.MaxProviderTransportAttempts == b.MaxProviderTransportAttempts &&
		a.ProviderTransportAttempts == b.ProviderTransportAttempts &&
		a.MaxOperationRecoveryAttempts == b.MaxOperationRecoveryAttempts &&
		a.OperationRecoveryAttempts == b.OperationRecoveryAttempts &&
		a.MaxCASReloadAttempts == b.MaxCASReloadAttempts &&
		a.CASReloadAttempts == b.CASReloadAttempts &&
		a.MaxTerminalDeliveryAttempts == b.MaxTerminalDeliveryAttempts &&
		a.TerminalDeliveryAttempts == b.TerminalDeliveryAttempts
}

func runHeartbeatEqual(a, b projectioncontract.RunHeartbeat) bool {
	return strings.TrimSpace(string(a.WorkspaceID)) == strings.TrimSpace(string(b.WorkspaceID)) &&
		strings.TrimSpace(string(a.RunID)) == strings.TrimSpace(string(b.RunID)) &&
		strings.TrimSpace(a.State) == strings.TrimSpace(b.State) &&
		strings.TrimSpace(a.ActivityState) == strings.TrimSpace(b.ActivityState) &&
		strings.TrimSpace(string(a.LatestStepID)) == strings.TrimSpace(string(b.LatestStepID)) &&
		strings.TrimSpace(a.LatestStepKind) == strings.TrimSpace(b.LatestStepKind) &&
		strings.TrimSpace(a.LatestStepState) == strings.TrimSpace(b.LatestStepState) &&
		strings.TrimSpace(a.LatestEvidenceRef) == strings.TrimSpace(b.LatestEvidenceRef) &&
		strings.TrimSpace(a.LatestEventTopic) == strings.TrimSpace(b.LatestEventTopic) &&
		a.LatestEventRevision == b.LatestEventRevision &&
		strings.TrimSpace(a.LatestEventAt) == strings.TrimSpace(b.LatestEventAt) &&
		strings.TrimSpace(a.LatestLLMUsageAt) == strings.TrimSpace(b.LatestLLMUsageAt) &&
		strings.TrimSpace(a.LatestResultAt) == strings.TrimSpace(b.LatestResultAt) &&
		strings.TrimSpace(a.LatestArtifactAt) == strings.TrimSpace(b.LatestArtifactAt) &&
		strings.TrimSpace(a.LatestPatchAt) == strings.TrimSpace(b.LatestPatchAt) &&
		strings.TrimSpace(a.LatestEffectiveProgressAt) == strings.TrimSpace(b.LatestEffectiveProgressAt) &&
		a.IneffectiveRetry == b.IneffectiveRetry &&
		a.Stalled == b.Stalled &&
		strings.TrimSpace(a.Summary) == strings.TrimSpace(b.Summary)
}

func convergenceStatusEqual(a, b *projectioncontract.ConvergenceStatus) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return strings.TrimSpace(string(a.WorkspaceID)) == strings.TrimSpace(string(b.WorkspaceID)) &&
		strings.TrimSpace(string(a.RunID)) == strings.TrimSpace(string(b.RunID)) &&
		string(a.RunState) == string(b.RunState) &&
		strings.TrimSpace(a.CurrentSliceID) == strings.TrimSpace(b.CurrentSliceID) &&
		strings.TrimSpace(a.CurrentFocusID) == strings.TrimSpace(b.CurrentFocusID) &&
		strings.TrimSpace(a.ConvergenceState) == strings.TrimSpace(b.ConvergenceState) &&
		strings.TrimSpace(a.CurrentGap) == strings.TrimSpace(b.CurrentGap) &&
		a.AcceptedEvidenceCount == b.AcceptedEvidenceCount &&
		a.RejectedEvidenceCount == b.RejectedEvidenceCount &&
		a.ClosedCoverageItemCount == b.ClosedCoverageItemCount &&
		a.RemainingCoverageItemCount == b.RemainingCoverageItemCount &&
		strings.TrimSpace(a.AssessmentDisposition) == strings.TrimSpace(b.AssessmentDisposition) &&
		a.RepairAttempts == b.RepairAttempts &&
		a.MaxRepairAttempts == b.MaxRepairAttempts &&
		a.RepeatedFailureSignatureCount == b.RepeatedFailureSignatureCount &&
		strings.TrimSpace(a.LatestAcceptedEvidenceRef) == strings.TrimSpace(b.LatestAcceptedEvidenceRef) &&
		a.NextActivationRequired == b.NextActivationRequired &&
		strings.TrimSpace(a.GapSummary) == strings.TrimSpace(b.GapSummary) &&
		strings.TrimSpace(a.LatestEffectiveProgressAt) == strings.TrimSpace(b.LatestEffectiveProgressAt) &&
		strings.TrimSpace(a.LastProgressAt) == strings.TrimSpace(b.LastProgressAt) &&
		strings.TrimSpace(a.DecisionCause) == strings.TrimSpace(b.DecisionCause) &&
		a.Stalled == b.Stalled &&
		a.IneffectiveRetry == b.IneffectiveRetry &&
		stringSliceEqualDisplay(a.MissingEvidenceKinds, b.MissingEvidenceKinds)
}

func runCommandStatusEqual(a, b *projectioncontract.RunCommandStatus) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return strings.TrimSpace(string(a.WorkspaceID)) == strings.TrimSpace(string(b.WorkspaceID)) &&
		strings.TrimSpace(string(a.RunID)) == strings.TrimSpace(string(b.RunID)) &&
		strings.TrimSpace(a.State) == strings.TrimSpace(b.State) &&
		strings.TrimSpace(a.Intent) == strings.TrimSpace(b.Intent) &&
		strings.TrimSpace(a.RequestID) == strings.TrimSpace(b.RequestID) &&
		strings.TrimSpace(a.RequestReason) == strings.TrimSpace(b.RequestReason) &&
		strings.TrimSpace(a.Source) == strings.TrimSpace(b.Source) &&
		strings.TrimSpace(string(a.Outcome)) == strings.TrimSpace(string(b.Outcome)) &&
		strings.TrimSpace(string(a.AdmissionReason)) == strings.TrimSpace(string(b.AdmissionReason)) &&
		strings.TrimSpace(a.Diagnostic) == strings.TrimSpace(b.Diagnostic) &&
		strings.TrimSpace(a.AcceptedAt) == strings.TrimSpace(b.AcceptedAt) &&
		strings.TrimSpace(a.RejectedAt) == strings.TrimSpace(b.RejectedAt) &&
		a.Revision == b.Revision
}

func stringSliceEqualDisplay(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if strings.TrimSpace(a[i]) != strings.TrimSpace(b[i]) {
			return false
		}
	}
	return true
}

func runtimeProgressEqual(a, b *projectioncontract.RuntimeProgress) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if a.StateRevision != b.StateRevision ||
		a.EvidenceSequence != b.EvidenceSequence ||
		strings.TrimSpace(a.CurrentFocusID) != strings.TrimSpace(b.CurrentFocusID) ||
		strings.TrimSpace(a.ConvergenceState) != strings.TrimSpace(b.ConvergenceState) ||
		strings.TrimSpace(a.TerminalCode) != strings.TrimSpace(b.TerminalCode) ||
		strings.TrimSpace(a.ActiveDispatchID) != strings.TrimSpace(b.ActiveDispatchID) ||
		a.CompletedDispatchCount != b.CompletedDispatchCount ||
		a.CandidateAttemptCount != b.CandidateAttemptCount ||
		a.AcceptedEvidenceCount != b.AcceptedEvidenceCount ||
		a.AcceptedValidationEvidenceCount != b.AcceptedValidationEvidenceCount ||
		a.RejectedEvidenceCount != b.RejectedEvidenceCount ||
		strings.TrimSpace(a.FailureClass) != strings.TrimSpace(b.FailureClass) ||
		strings.TrimSpace(a.FailureCode) != strings.TrimSpace(b.FailureCode) ||
		strings.TrimSpace(a.FailureSignature) != strings.TrimSpace(b.FailureSignature) ||
		strings.TrimSpace(a.FailureMessage) != strings.TrimSpace(b.FailureMessage) ||
		strings.TrimSpace(a.PrerequisiteRefreshState) != strings.TrimSpace(b.PrerequisiteRefreshState) ||
		a.PrerequisiteRefreshUsed != b.PrerequisiteRefreshUsed ||
		a.PrerequisiteRefreshLimit != b.PrerequisiteRefreshLimit {
		return false
	}
	if len(a.MissingEvidenceKinds) != len(b.MissingEvidenceKinds) {
		return false
	}
	for i := range a.MissingEvidenceKinds {
		if a.MissingEvidenceKinds[i] != b.MissingEvidenceKinds[i] {
			return false
		}
	}
	return stringSliceEqualDisplay(a.RepairHintCodes, b.RepairHintCodes) &&
		prerequisiteRefreshInvalidationsEqual(a.PrerequisiteInvalidations, b.PrerequisiteInvalidations)
}

func prerequisiteRefreshInvalidationsEqual(a, b []projectioncontract.RuntimePrerequisiteRefreshInvalidation) bool {
	if len(a) != len(b) {
		return false
	}
	for idx := range a {
		left, right := a[idx], b[idx]
		if left.Signature != right.Signature || left.Source != right.Source || left.OwnerModuleID != right.OwnerModuleID ||
			left.ResourceDomain != right.ResourceDomain || left.SubjectRef != right.SubjectRef || left.ExpectedRevision != right.ExpectedRevision ||
			left.ObservedRevision != right.ObservedRevision || left.Reason != right.Reason || left.DispatchID != right.DispatchID ||
			left.OperationID != right.OperationID || left.ReceiptHash != right.ReceiptHash ||
			!stringSliceEqualDisplay(left.PrerequisiteIDs, right.PrerequisiteIDs) || !stringSliceEqualDisplay(left.ObservationIDs, right.ObservationIDs) ||
			len(left.ResourceChanges) != len(right.ResourceChanges) {
			return false
		}
		for changeIdx := range left.ResourceChanges {
			if left.ResourceChanges[changeIdx] != right.ResourceChanges[changeIdx] {
				return false
			}
		}
	}
	return true
}

func activityFeedDisplayEqual(a, b projectioncontract.WorkspaceActivityFeed) bool {
	if strings.TrimSpace(string(a.WorkspaceID)) != strings.TrimSpace(string(b.WorkspaceID)) ||
		strings.TrimSpace(string(a.DisplayRunID)) != strings.TrimSpace(string(b.DisplayRunID)) ||
		a.ActivityRevision != b.ActivityRevision ||
		a.Truncated != b.Truncated ||
		strings.TrimSpace(a.NextCursor) != strings.TrimSpace(b.NextCursor) ||
		len(a.Items) != len(b.Items) {
		return false
	}
	for i := range a.Items {
		if strings.TrimSpace(a.Items[i].ID) != strings.TrimSpace(b.Items[i].ID) ||
			strings.TrimSpace(a.Items[i].Summary) != strings.TrimSpace(b.Items[i].Summary) ||
			strings.TrimSpace(a.Items[i].Title) != strings.TrimSpace(b.Items[i].Title) ||
			strings.TrimSpace(a.Items[i].State) != strings.TrimSpace(b.Items[i].State) ||
			string(a.Items[i].Kind) != string(b.Items[i].Kind) ||
			strings.TrimSpace(a.Items[i].OccurredAt) != strings.TrimSpace(b.Items[i].OccurredAt) {
			return false
		}
	}
	return true
}

func decisionInfoEqual(a, b *projectioncontract.Decision) bool {
	return reflect.DeepEqual(a, b)
}

func approvalInfoEqual(a, b *projectioncontract.Approval) bool {
	return reflect.DeepEqual(a, b)
}

func runtimeViewRevisionAcceptable(current, incoming projectioncontract.WorkspaceRefreshRevision) bool {
	if revisionGenerationChanged(current, incoming) {
		// Cross-generation RuntimeView readback is authoritative. Numeric
		// revisions are intentionally incomparable across this boundary.
		return true
	}
	// Non-zero applied revisions reject older responses.
	if current.WorkspaceRevision > 0 {
		if incoming.WorkspaceRevision == 0 || incoming.WorkspaceRevision < current.WorkspaceRevision {
			return false
		}
	}
	if current.RuntimeStateRevision > 0 {
		if incoming.RuntimeStateRevision == 0 || incoming.RuntimeStateRevision < current.RuntimeStateRevision {
			return false
		}
	}
	if current.ActivityRevision > 0 {
		if incoming.ActivityRevision == 0 || incoming.ActivityRevision < current.ActivityRevision {
			return false
		}
	}
	return true
}

func revisionGenerationChanged(current, incoming projectioncontract.WorkspaceRefreshRevision) bool {
	left := strings.TrimSpace(current.StreamGeneration)
	right := strings.TrimSpace(incoming.StreamGeneration)
	return left != "" && right != "" && left != right
}

func cloneRuntime(item *projectioncontract.RuntimeProgress) *projectioncontract.RuntimeProgress {
	if item == nil {
		return nil
	}
	ret := *item
	ret.MissingEvidenceKinds = append([]string(nil), item.MissingEvidenceKinds...)
	ret.RepairHintCodes = append([]string(nil), item.RepairHintCodes...)
	if len(item.PrerequisiteInvalidations) > 0 {
		ret.PrerequisiteInvalidations = make([]projectioncontract.RuntimePrerequisiteRefreshInvalidation, 0, len(item.PrerequisiteInvalidations))
		for _, value := range item.PrerequisiteInvalidations {
			clone := value
			clone.PrerequisiteIDs = append([]string(nil), value.PrerequisiteIDs...)
			clone.ObservationIDs = append([]string(nil), value.ObservationIDs...)
			clone.ResourceChanges = append([]projectioncontract.RuntimePrerequisiteResourceChange(nil), value.ResourceChanges...)
			ret.PrerequisiteInvalidations = append(ret.PrerequisiteInvalidations, clone)
		}
	}
	return &ret
}

func cloneActivityFeed(item projectioncontract.WorkspaceActivityFeed) projectioncontract.WorkspaceActivityFeed {
	ret := item
	if len(item.Items) > 0 {
		ret.Items = append([]projectioncontract.WorkspaceActivityItem(nil), item.Items...)
	}
	return ret
}

func cloneRunCommandPtr(item *projectioncontract.RunCommandStatus) *projectioncontract.RunCommandStatus {
	if item == nil {
		return nil
	}
	ret := *item
	return &ret
}

func (m *AppModel) markWorkspaceContextDisplayLoaded(workspaceID string, msg WorkspaceContextMsg) {
	entry := m.ensureWorkspaceRuntimeState(workspaceID)
	if entry == nil {
		return
	}
	entry.HasDisplay = true
	if revision := workspaceContextRevision(msg); revision > 0 {
		entry.DisplayRevision = revision
		return
	}
	if revision := projectionRevision(entry.Overview.Projection); revision > entry.DisplayRevision {
		entry.DisplayRevision = revision
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
	m.llmUsage = projectioncontract.LLMUsageSummary{}
	m.llmContextSummary = projectioncontract.LLMContextBudgetSummary{}
	m.llmContext = nil
	m.acceptance = projectioncontract.AcceptanceProjection{}
	m.runHeartbeat = projectioncontract.RunHeartbeat{}
	m.runCommand = nil
	m.runtime = nil
	m.activityFeed = projectioncontract.WorkspaceActivityFeed{}
	m.repairLoop = ""
	m.failureReason = ""
	m.failureHint = ""
	m.clearCanonicalBlockingTargets()
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
	return false
}

func effectiveGoalText(run *projectioncontract.Run) string {
	return ""
}

func effectiveGoalTranscript(run *projectioncontract.Run, fallback string) []transcriptItem {
	if run == nil {
		return initialTranscript(fallback)
	}
	return initialTranscript(fallback)
}

func effectiveDoDItems(run *projectioncontract.Run) []string {
	if run == nil {
		return nil
	}
	return nil
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
	m.llmUsage = projectioncontract.LLMUsageSummary{}
	m.llmContext = nil
	m.acceptance = projectioncontract.AcceptanceProjection{}
	m.runHeartbeat = projectioncontract.RunHeartbeat{}
	m.repairLoop = ""
	m.failureReason = ""
	m.failureHint = ""
	m.clearCanonicalBlockingTargets()
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
		ActiveRunID:           m.activeRunID,
		DisplayRunID:          string(entry.RuntimeView.DisplayRunID),
		SealedLLMSelection:    runtimecontract.CloneProviderInferenceSelection(entry.Display.SealedLLMSelection),
		CurrentTask:           m.currentTask,
		DoDItems:              append([]string(nil), m.dodItems...),
		Transcript:            append([]transcriptItem(nil), m.transcript...),
		Result:                m.result,
		Artifacts:             append([]string(nil), m.artifacts...),
		Sources:               append([]string(nil), m.sources...),
		Patches:               append([]projectioncontract.Patch(nil), m.patches...),
		ArtifactContinuations: append([]projectioncontract.ArtifactContinuation(nil), m.artifactContinuations...),
		TaskDecompositions:    append([]projectioncontract.TaskDecomposition(nil), m.taskDecomp...),
		TaskPlans:             append([]projectioncontract.TaskPlan(nil), m.taskPlans...),
		MemoryRecalls:         append([]projectioncontract.MemoryRecallTrace(nil), m.memoryRecalls...),
		LLMUsage:              m.llmUsage,
		LLMContextSummary:     m.llmContextSummary,
		LLMContext:            clonePayloadMap(m.llmContext),
		Acceptance:            m.acceptance,
		RunHeartbeat:          m.runHeartbeat,
		ConvergenceStatus:     cloneConvergenceStatusInfoPtr(m.convergenceStatus),
		Runtime:               cloneRuntime(m.runtime),
		Activity:              cloneActivityFeed(m.activityFeed),
		Progress:              m.progress,
		ProgressText:          m.progressText,
		RepairLoop:            m.repairLoop,
		FailureReason:         m.failureReason,
		FailureHint:           m.failureHint,
		Running:               m.running,
		BlockingDecision:      cloneDecisionPtr(m.blockingDecision),
		BlockingApproval:      cloneApprovalPtr(m.blockingApproval),
		GovernanceDisplay:     entry.Display.GovernanceDisplay,
		ConsistencyDiagnostic: projectioncontract.CloneConsistencyDiagnostic(entry.Display.ConsistencyDiagnostic),
		Decisions:             append([]projectioncontract.Decision(nil), m.decisions...),
		Approvals:             append([]projectioncontract.Approval(nil), m.approvals...),
		LastError:             m.lastError,
	}
	entry.FollowTail = !m.mainViewportUserScrolled
	entry.UnreadUpdateCount = m.mainViewportUnreadCount
	entry.HasDisplay = true
	if revision := projectionRevision(entry.Overview.Projection); revision > entry.DisplayRevision {
		entry.DisplayRevision = revision
	}
}

func (m *AppModel) restoreWorkspaceDisplayState(workspaceID string) bool {
	cached, ok := m.cachedWorkspaceDisplayState(workspaceID)
	if !ok {
		return false
	}
	m.activeRunID = cached.ActiveRunID
	m.currentTask = cached.CurrentTask
	m.dodItems = append([]string(nil), cached.DoDItems...)
	m.transcript = append([]transcriptItem(nil), cached.Transcript...)
	m.result = cached.Result
	m.artifacts = append([]string(nil), cached.Artifacts...)
	m.sources = append([]string(nil), cached.Sources...)
	m.patches = append([]projectioncontract.Patch(nil), cached.Patches...)
	m.artifactContinuations = append([]projectioncontract.ArtifactContinuation(nil), cached.ArtifactContinuations...)
	m.taskDecomp = append([]projectioncontract.TaskDecomposition(nil), cached.TaskDecompositions...)
	m.taskPlans = append([]projectioncontract.TaskPlan(nil), cached.TaskPlans...)
	m.memoryRecalls = append([]projectioncontract.MemoryRecallTrace(nil), cached.MemoryRecalls...)
	m.llmUsage = cached.LLMUsage
	m.llmContextSummary = cached.LLMContextSummary
	m.llmContext = clonePayloadMap(cached.LLMContext)
	m.acceptance = cached.Acceptance
	m.runHeartbeat = cached.RunHeartbeat
	m.convergenceStatus = cloneConvergenceStatusInfoPtr(cached.ConvergenceStatus)
	m.runtime = cloneRuntime(cached.Runtime)
	m.activityFeed = cloneActivityFeed(cached.Activity)
	m.repairLoop = cached.RepairLoop
	// Progress/running/blocker remain projection-authoritative after switch;
	// cache restores historical content to avoid empty screen only.
	m.failureReason = ""
	m.failureHint = ""
	m.running = false
	m.progress = progressIdle
	m.progressText = ""
	m.clearCanonicalBlockingTargets()
	m.decisions = nil
	m.approvals = nil
	m.lastError = nil
	if entry := m.workspaceRuntimeState(workspaceID); entry != nil {
		m.mainViewportUserScrolled = !entry.FollowTail
		m.mainViewportUnreadCount = entry.UnreadUpdateCount
		m.mainViewportPendingUpdate = entry.UnreadUpdateCount > 0 && !entry.FollowTail
	} else {
		m.resetMainViewport()
	}
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

func cloneDecisionPtr(input *projectioncontract.Decision) *projectioncontract.Decision {
	if input == nil {
		return nil
	}
	ret := projectioncontract.CloneDecision(*input)
	return &ret
}

func cloneApprovalPtr(input *projectioncontract.Approval) *projectioncontract.Approval {
	if input == nil {
		return nil
	}
	ret := projectioncontract.CloneApproval(*input)
	return &ret
}

func cloneConvergenceStatusInfoPtr(input *projectioncontract.ConvergenceStatus) *projectioncontract.ConvergenceStatus {
	if input == nil {
		return nil
	}
	ret := *input
	ret.MissingEvidenceKinds = append([]string(nil), input.MissingEvidenceKinds...)
	return &ret
}

func (m *AppModel) applyRunHeartbeat(item projectioncontract.RunHeartbeat) {
	m.runHeartbeat = item
	switch item.ActivityState {
	case "ineffective_retry", "no_activity", "stalled":
		if m.progress == progressRunning || m.progress == progressIdle {
			m.progressText = nonEmpty(item.Summary, string(item.ActivityState))
		}
	}
}

func (m *AppModel) applyConvergenceStatus(item *projectioncontract.ConvergenceStatus) {
	m.convergenceStatus = cloneConvergenceStatusInfoPtr(item)
}

func latestLLMContextBudget(budgets []projectioncontract.LLMContextBudgetTrace, traces []projectioncontract.LLMUsageTrace) map[string]any {
	for idx := len(budgets) - 1; idx >= 0; idx-- {
		if budget := llmContextBudgetTraceMap(budgets[idx]); len(budget) > 0 {
			return budget
		}
	}
	for idx := len(traces) - 1; idx >= 0; idx-- {
		budget := traces[idx].ContextBudget
		if len(budget.Fields) == 0 {
			continue
		}
		return budget.Map()
	}
	return nil
}

func llmContextBudgetTraceMap(item projectioncontract.LLMContextBudgetTrace) map[string]any {
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
		"profile":               item.Profile.Map(),
		"cachePlan":             item.CachePlan.Map(),
	}
	return ret
}

func (m *AppModel) upsertWorkspace(ws projectioncontract.Workspace) {
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
	case common.WorkspaceStateActive:
		m.running = false
		m.progress = progressIdle
		m.progressText = ""
	case common.WorkspaceStateBlockedOnApproval:
		m.running = false
		m.progress = progressWaitingApproval
		m.progressText = "Waiting for workspace approval"
	default:
		m.running = false
		m.progress = progressIdle
		m.progressText = ""
	}
}

func (m *AppModel) applyRunCommandStatus(status *projectioncontract.RunCommandStatus) {
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

func runCommandProjectionSettled(status projectioncontract.RunCommandStatus) bool {
	state := strings.ToLower(strings.TrimSpace(status.State))
	return state == "accepted" || state == "rejected"
}

func (m *AppModel) clearBlockingInputState() {
	m.clearCanonicalBlockingTargets()
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
	} else if !workspaceStateIsLive(common.WorkspaceState(ws.State)) && ws.State != string(common.WorkspaceStateActive) && ws.State != string(common.WorkspaceStateBlockedOnApproval) {
		m.activeRunID = ""
	}
	m.applyWorkspaceProgress(common.WorkspaceState(ws.State))
	switch m.activeGovernancePromptMode() {
	case workspacePromptModeDecision:
		m.ensureActiveBlockingDecisionFromOverview()
		m.followBlockerTailUnlessUserScrolled()
		return m.requestWorkspaceContextRefresh(false)
	case workspacePromptModeApproval:
		m.ensureActiveBlockingApprovalFromOverview()
		m.followBlockerTailUnlessUserScrolled()
		return m.requestWorkspaceContextRefresh(false)
	}
	if ws.State == string(common.WorkspaceStateActive) && ws.ActiveRunID != "" {
		m.clearBlockingInputState()
		m.running = true
		m.progress = progressRunning
		m.progressText = "Running"
	} else if !workspaceStateIsLive(common.WorkspaceState(ws.State)) {
		m.clearCanonicalBlockingTargets()
	}
	return nil
}

func refreshRevisionBelow(current, target projectioncontract.WorkspaceRefreshRevision) bool {
	if revisionGenerationChanged(current, target) {
		return false
	}
	// Response is still below DirtyRevision when any dirty domain is non-zero and
	// the response is missing (zero) or lower than that dirty value. Zero response
	// while dirty is set must keep dirty and schedule another pending readback.
	if target.WorkspaceRevision > 0 && current.WorkspaceRevision < target.WorkspaceRevision {
		return true
	}
	if target.RuntimeStateRevision > 0 && current.RuntimeStateRevision < target.RuntimeStateRevision {
		return true
	}
	if target.ActivityRevision > 0 && current.ActivityRevision < target.ActivityRevision {
		return true
	}
	if target.EvidenceSequence > 0 && current.EvidenceSequence < target.EvidenceSequence {
		return true
	}
	if target.ProjectedSourceRevision > 0 && current.ProjectedSourceRevision < target.ProjectedSourceRevision {
		return true
	}
	return false
}
