package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/doditems"
	"github.com/muidea/workorch/internal/pkg/governancesemantics"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
	backend "github.com/muidea/workorch/internal/pkg/workorchport"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
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

	input                  string
	inputCursor            int
	slashCursor            int
	inputHistory           []string
	historyCursor          int
	currentTask            string
	dodItems               []string
	transcript             []transcriptItem
	result                 string
	artifacts              []string
	sources                []string
	patches                []backend.PatchInfo
	artifactContinuations  []backend.ArtifactContinuationInfo
	taskDecomp             []backend.TaskDecompositionInfo
	taskPlans              []backend.TaskPlanInfo
	memoryRecalls          []backend.MemoryRecallTraceInfo
	llmUsage               backend.LLMUsageSummaryInfo
	llmContextSummary      projectioncontract.LLMContextBudgetSummary
	llmContext             map[string]any
	acceptance             backend.AcceptanceProjectionInfo
	runHeartbeat           backend.RunHeartbeatInfo
	convergenceStatus      *backend.ConvergenceStatusInfo
	runCommand             *backend.RunCommandStatusInfo
	schedulerDiagnostics   *projectioncontract.SchedulerDiagnostics
	runtime                *backend.RuntimeProgressInfo
	activityFeed           backend.WorkspaceActivityFeedInfo
	progress               progressState
	progressText           string
	repairLoop             string
	failureReason          string
	failureHint            string
	running                bool
	workspaceRuntimeStates map[string]*workspaceRuntimeState

	blockingDecision          *backend.DecisionInfo
	blockingApproval          *backend.ApprovalInfo
	helpOpen                  bool
	mainViewport              *viewport.Model
	mainViewportUserScrolled  bool
	mainViewportPendingUpdate bool
	mainViewportUnreadCount   int

	workspaceListRefreshInFlight bool
	workspaceListRefreshPending  bool
	workspaceListRequestSeq      int64
	modelSelectMode              bool
	modelSelectLoading           bool
	modelSelectRefreshing        bool
	modelSelectConfirming        bool
	modelSelectSubmitting        bool
	modelSelectClearConfirming   bool
	modelSelectCursor            int
	modelSelectOptions           []backend.ChatModelTargetOption
	modelSelectCurrent           *runtimecontract.ProviderModelTarget
	modelSelectPending           *runtimecontract.ProviderModelTarget
	modelSelectDefault           runtimecontract.ProviderModelTarget
	modelSelectError             error
	modelSelectRequestSeq        int64
	modelSelectWorkspaceID       string

	projectionCh            chan backend.WorkspaceProjectionChangedInfo
	subscriptionStatusCh    chan backend.SubscriptionStatusInfo
	eventSubscriptionActive bool
	subscriptionStatus      backend.SubscriptionStatusInfo
	subscription            *projectionSubscription
	initialGoal             string
	options                 Options

	width  int
	height int

	lastError error
}

// projectionSubscription owns the local projection observer shared by the
// value copies Bubble Tea makes of AppModel.  Its cleanup must never run on
// the Bubble Tea update loop: EventHub observer removal can wait for a busy
// shared hub worker queue.
type projectionSubscription struct {
	once        sync.Once
	unsubscribe func()
	done        chan struct{}
}

const projectionSubscriptionCleanupTimeout = 100 * time.Millisecond

func NewAppModel(be backend.Backend, defaultWorkspaceID string, initialGoal string) AppModel {
	return NewAppModelWithOptions(be, defaultWorkspaceID, initialGoal, Options{})
}

func NewAppModelWithOptions(be backend.Backend, defaultWorkspaceID string, initialGoal string, options Options) AppModel {
	projectionCh := make(chan backend.WorkspaceProjectionChangedInfo, 256)
	statusCh := make(chan backend.SubscriptionStatusInfo, 64)
	unsubscribe := func() {}
	eventSubscriptionActive := false
	subscriptionStatus := backend.SubscriptionStatusInfo{State: backend.SubscriptionConnecting}
	if be != nil {
		if projectionBackend := be.ProjectionEvents(); projectionBackend != nil {
			if unsub, err := projectionBackend.SubscribeWorkspaceProjection(
				context.Background(),
				func(_ context.Context, info backend.WorkspaceProjectionChangedInfo) {
					enqueueProjectionChanged(projectionCh, info)
				},
				func(_ context.Context, status backend.SubscriptionStatusInfo) {
					enqueueSubscriptionStatus(statusCh, status)
				},
			); err == nil {
				unsubscribe = unsub
				eventSubscriptionActive = true
				// Subscribe return does not imply live; wait for status callback.
			} else {
				subscriptionStatus = backend.SubscriptionStatusInfo{
					State:     backend.SubscriptionDegraded,
					LastError: err.Error(),
				}
			}
		} else {
			subscriptionStatus = backend.SubscriptionStatusInfo{State: backend.SubscriptionDegraded, LastError: "projection events backend unavailable"}
		}
	} else {
		subscriptionStatus = backend.SubscriptionStatusInfo{State: backend.SubscriptionDegraded, LastError: "backend unavailable"}
	}
	return AppModel{
		be:                      be,
		mode:                    modeAgentSession,
		defaultWorkspaceID:      defaultWorkspaceID,
		activeWorkspaceID:       defaultWorkspaceID,
		selectedWorkspaceID:     defaultWorkspaceID,
		projectionCh:            projectionCh,
		subscriptionStatusCh:    statusCh,
		eventSubscriptionActive: eventSubscriptionActive,
		subscriptionStatus:      subscriptionStatus,
		subscription: &projectionSubscription{
			unsubscribe: unsubscribe,
			done:        make(chan struct{}),
		},
		initialGoal:            strings.TrimSpace(initialGoal),
		currentTask:            strings.TrimSpace(initialGoal),
		transcript:             initialTranscript(initialGoal),
		options:                options,
		progress:               progressIdle,
		workspaceRuntimeStates: map[string]*workspaceRuntimeState{},
		historyCursor:          -1,
		mainViewport:           newMainViewport(),
	}
}

// CloseProjectionSubscription releases the local EventHub observer after the
// TUI has stopped.  It has a bounded wait so a congested EventHub cannot hold
// process exit hostage.  A later framework teardown owns the remaining hub
// cleanup if the observer removal is still in flight.
func (m *AppModel) CloseProjectionSubscription(timeout time.Duration) bool {
	if m == nil || m.subscription == nil {
		return true
	}
	subscription := m.subscription
	subscription.once.Do(func() {
		go func() {
			defer close(subscription.done)
			if subscription.unsubscribe != nil {
				subscription.unsubscribe()
			}
		}()
	})
	if timeout <= 0 {
		select {
		case <-subscription.done:
			return true
		default:
			return false
		}
	}
	select {
	case <-subscription.done:
		return true
	case <-time.After(timeout):
		return false
	}
}

// ProjectionSubscriptionCleanupTimeout is the lifecycle wait budget used by
// the interactive TUI after Bubble Tea has already restored the terminal.
func ProjectionSubscriptionCleanupTimeout() time.Duration {
	return projectionSubscriptionCleanupTimeout
}

func enqueueProjectionChanged(ch chan backend.WorkspaceProjectionChangedInfo, info backend.WorkspaceProjectionChangedInfo) {
	if ch == nil {
		return
	}
	select {
	case ch <- info:
	default:
		// drop oldest then enqueue latest projection invalidation
		select {
		case <-ch:
		default:
		}
		select {
		case ch <- info:
		default:
		}
	}
}

func enqueueSubscriptionStatus(ch chan backend.SubscriptionStatusInfo, status backend.SubscriptionStatusInfo) {
	if ch == nil {
		return
	}
	select {
	case ch <- status:
	default:
		select {
		case <-ch:
		default:
		}
		select {
		case ch <- status:
		default:
		}
	}
}

func (m AppModel) Init() tea.Cmd {
	cmds := []tea.Cmd{FetchAll(m.be)}
	if m.eventSubscriptionActive {
		cmds = append(cmds, WaitForProjectionSignal(m.projectionCh, m.subscriptionStatusCh))
	}
	if m.initialGoal != "" && m.activeWorkspaceID != "" {
		if isRunControlIntent(m.initialGoal) {
			cmds = append(cmds, ResumeRun(m.be, m.activeWorkspaceID))
		} else {
			cmds = append(cmds, SubmitGoal(m.be, m.activeWorkspaceID, m.initialGoal, m.dodItems))
		}
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
		if msg.RequestSeq > 0 && msg.RequestSeq != m.workspaceListRequestSeq {
			return m, nil
		}
		pendingRefresh := m.workspaceListRefreshPending
		m.workspaceListRefreshInFlight = false
		m.workspaceListRefreshPending = false
		if msg.Error != nil {
			if isTransientProjectionError(msg.Error) {
				m.lastError = nil
				m.progressText = "Workspace projection is refreshing; retrying workspace list"
				return m, RetryWorkspaceList()
			}
			m.setLastError(msg.Error)
			return m, nil
		}
		previousActive := m.activeWorkspaceID
		m.replaceWorkspaceList(msg.Workspaces, msg.Projection)
		m.ensureActiveWorkspace()
		cmds := []tea.Cmd{}
		if cmd := m.syncActiveWorkspaceOverview(); cmd != nil {
			cmds = append(cmds, cmd)
		}
		if m.activeWorkspaceNeedsContextRefresh(previousActive) {
			cmds = append(cmds, m.requestWorkspaceContextRefresh(false))
		}
		if pendingRefresh {
			cmds = append(cmds, m.requestWorkspaceListRefresh())
		}
		return m, tea.Batch(cmds...)
	case WorkspaceListRetryMsg:
		return m, m.requestWorkspaceListRefresh()
	case WorkspaceContextMsg:
		if msg.RequestSeq > 0 && msg.RequestSeq != m.workspaceContextRequestSeqFor(msg.WorkspaceID) {
			return m, nil
		}
		contextPendingRefresh := false
		if entry := m.workspaceRuntimeState(msg.WorkspaceID); entry != nil {
			contextPendingRefresh = entry.ContextPending
			entry.ContextInFlight = false
			entry.ContextPending = false
		}
		if !m.workspaceMessageBelongsToActiveWorkspace(msg.WorkspaceID) {
			return m, nil
		}
		if msg.Error != nil {
			if isTransientProjectionError(msg.Error) {
				m.lastError = nil
				if !m.restoreActiveWorkspaceBlockerFromOverview() {
					m.progressText = "Workspace projection is refreshing; retrying state load"
				}
				return m, RetryWorkspaceContext(msg.WorkspaceID, msg.ResetMainViewport)
			}
			m.setLastError(msg.Error)
			return m, nil
		}
		if m.workspaceContextOlderThanOverview(msg) {
			m.lastError = nil
			m.progressText = "Workspace projection detail is older than list; refreshing state"
			return m, RetryWorkspaceContext(msg.WorkspaceID, msg.ResetMainViewport)
		}
		m.applyWorkspaceContext(msg)
		m.lastError = nil
		if msg.ResetMainViewport {
			if m.activeBlockingDecision() != nil || m.activeBlockingApproval() != nil {
				m.followMainViewportTail()
			} else {
				m.syncMainViewportToCurrentContent()
			}
		}
		cmds := []tea.Cmd{m.fetchActiveBlockerDetails()}
		if contextPendingRefresh {
			cmds = append(cmds, m.requestWorkspaceContextFor(msg.WorkspaceID, false))
		}
		return m, tea.Batch(cmds...)
	case WorkspaceContextRetryMsg:
		if !m.workspaceMessageBelongsToActiveWorkspace(msg.WorkspaceID) {
			return m, nil
		}
		return m, m.requestWorkspaceContextFor(msg.WorkspaceID, msg.ResetMainViewport)
	case WorkspaceRuntimeViewMsg:
		requestWorkspaceID := strings.TrimSpace(msg.WorkspaceID)
		if msg.RequestSeq > 0 && msg.RequestSeq != m.workspaceContextRequestSeqFor(requestWorkspaceID) {
			// Stale request sequence: do not touch in-flight/pending for the
			// current sequence, and never write cache from an obsolete response.
			return m, nil
		}
		// CR03: scope/nil validation must complete BEFORE clearing
		// ContextInFlight/ContextPending. Mismatch or nil view must keep dirty
		// and preserve any already-merged pending readback.
		if msg.Error == nil && msg.View != nil {
			viewWorkspaceID := strings.TrimSpace(string(msg.View.WorkspaceID))
			if requestWorkspaceID == "" || viewWorkspaceID == "" || requestWorkspaceID != viewWorkspaceID {
				return m, nil
			}
		}
		if msg.Error == nil && msg.View == nil {
			// Nil view with no error: do not clear pending/in-flight/dirty.
			return m, nil
		}
		contextPendingRefresh := false
		if entry := m.workspaceRuntimeState(requestWorkspaceID); entry != nil {
			contextPendingRefresh = entry.ContextPending
			entry.ContextInFlight = false
			entry.ContextPending = false
		}
		if msg.Error != nil {
			if isTransientProjectionError(msg.Error) {
				m.lastError = nil
				m.progressText = "Workspace runtime view is refreshing; retrying state load"
				return m, RetryWorkspaceContext(requestWorkspaceID, msg.ResetMainViewport)
			}
			// Keep active view; surface degraded status without wiping history.
			m.setLastError(msg.Error)
			return m, nil
		}
		// Zero revision while DirtyRevision is non-zero: do not apply, keep dirty,
		// and schedule exactly one pending readback.
		if entry := m.workspaceRuntimeState(requestWorkspaceID); entry != nil {
			if refreshRevisionBelow(msg.View.Revision, entry.DirtyRevision) &&
				msg.View.Revision.WorkspaceRevision == 0 &&
				msg.View.Revision.RuntimeStateRevision == 0 &&
				msg.View.Revision.ActivityRevision == 0 {
				if !entry.ContextInFlight {
					return m, m.requestWorkspaceContextFor(requestWorkspaceID, false)
				}
				entry.ContextPending = true
				return m, nil
			}
		}
		m.applyWorkspaceRuntimeView(*msg.View, msg.ResetMainViewport)
		m.lastError = nil
		cmds := []tea.Cmd{}
		if m.workspaceMessageBelongsToActiveWorkspace(requestWorkspaceID) {
			cmds = append(cmds, m.fetchActiveBlockerDetails())
		}
		if entry := m.workspaceRuntimeState(requestWorkspaceID); entry != nil {
			if refreshRevisionBelow(msg.View.Revision, entry.DirtyRevision) {
				// response still below dirty notification; schedule one more readback
				contextPendingRefresh = true
			}
		}
		if contextPendingRefresh {
			cmds = append(cmds, m.requestWorkspaceContextFor(requestWorkspaceID, false))
		}
		return m, tea.Batch(cmds...)
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
			return m, tea.Batch(m.requestWorkspaceListRefresh(), m.requestWorkspaceContextFor(string(msg.Workspace.ID), true))
		}
		return m, nil
	case DecisionListMsg:
		if !m.workspaceMessageBelongsToActiveWorkspace(msg.WorkspaceID) {
			return m, nil
		}
		if msg.Error != nil {
			m.setLastError(msg.Error)
			return m, nil
		}
		m.decisions = msg.Decisions
		m.syncBlockingDecisionFromList()
		m.followBlockerTailUnlessUserScrolled()
		return m, nil
	case DecisionDetailMsg:
		if !m.workspaceMessageBelongsToActiveWorkspace(msg.WorkspaceID) {
			return m, nil
		}
		if msg.Error != nil {
			m.setLastError(msg.Error)
			return m, nil
		}
		if msg.Decision != nil {
			m.upsertDecision(*msg.Decision)
			if m.blockingDecision != nil && m.blockingDecision.ID == msg.Decision.ID {
				m.blockingDecision = msg.Decision
				m.progressText = nonEmpty(msg.Decision.Question, msg.Decision.Reason, string(msg.Decision.ID))
				m.followBlockerTailUnlessUserScrolled()
			}
		}
		return m, nil
	case ApprovalListMsg:
		if !m.workspaceMessageBelongsToActiveWorkspace(msg.WorkspaceID) {
			return m, nil
		}
		if msg.Error != nil {
			m.setLastError(msg.Error)
			return m, nil
		}
		m.approvals = msg.Approvals
		m.syncBlockingApprovalFromList()
		m.followBlockerTailUnlessUserScrolled()
		return m, nil
	case ApprovalDetailMsg:
		if !m.workspaceMessageBelongsToActiveWorkspace(msg.WorkspaceID) {
			return m, nil
		}
		if msg.Error != nil {
			m.setLastError(msg.Error)
			return m, nil
		}
		if msg.Approval != nil {
			m.upsertApproval(*msg.Approval)
			if m.blockingApproval != nil && m.blockingApproval.ID == msg.Approval.ID {
				m.blockingApproval = msg.Approval
				m.progressText = nonEmpty(msg.Approval.Reason, string(msg.Approval.ID))
				m.followBlockerTailUnlessUserScrolled()
			}
		}
		return m, nil
	case SubscriptionStatusMsg:
		cmd := m.applySubscriptionStatus(msg.Status)
		if m.eventSubscriptionActive {
			return m, tea.Batch(WaitForProjectionSignal(m.projectionCh, m.subscriptionStatusCh), cmd)
		}
		return m, cmd
	case WorkspaceProjectionChangedMsg:
		cmd := m.applyWorkspaceProjectionChanged(msg.Info)
		if m.eventSubscriptionActive {
			return m, tea.Batch(WaitForProjectionSignal(m.projectionCh, m.subscriptionStatusCh), cmd)
		}
		return m, cmd
	case RunStartedMsg:
		if !m.runMessageBelongsToActiveWorkspace(msg.WorkspaceID, msg.Run) {
			return m, nil
		}
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
	case DoDUpdatedMsg:
		if msg.Error != nil {
			m.setLastError(msg.Error)
			m.progress = progressFailed
			m.progressText = "DoD save failed"
			return m, nil
		}
		if msg.DoD != nil {
			m.dodItems = append([]string(nil), msg.DoD.Items...)
			m.result = renderPendingDoDResult(m.dodItems)
			m.progress = progressIdle
			m.progressText = "DoD saved"
			m.lastError = nil
			m.resetMainViewport()
			return m, m.requestWorkspaceContextRefresh(false)
		}
		return m, nil
	case RunControlMsg:
		if !m.runMessageBelongsToActiveWorkspace(msg.WorkspaceID, msg.Run) {
			return m, nil
		}
		if msg.Error != nil {
			if isTransientContextError(msg.Error) {
				m.progressText = "Workspace state refresh timed out; retrying"
				return m, m.refreshActiveWorkspaceState()
			}
			if errors.Is(msg.Error, common.ErrBlocked) {
				if msg.Run != nil {
					m.activeRunID = string(msg.Run.ID)
					m.applyRunControlState(msg.Run, msg.Message)
				} else {
					m.running = false
					m.progress = progressWaitingDecision
					m.progressText = "Run is blocked; refreshing blocker details"
				}
				m.lastError = nil
				return m, tea.Batch(m.refreshActiveWorkspaceState(), m.fetchActiveBlockerDetails())
			}
			m.setLastError(msg.Error)
			return m, m.refreshActiveWorkspaceState()
		}
		if msg.Run != nil {
			m.activeRunID = string(msg.Run.ID)
			m.applyRunControlState(msg.Run, msg.Message)
		} else if msg.Message != "" {
			m.running = false
			if m.progress == progressSubmitting {
				m.progress = progressIdle
			}
			m.progressText = msg.Message
		}
		m.lastError = nil
		return m, m.refreshActiveWorkspaceState()
	case DecisionResolvedMsg:
		if msg.Error != nil {
			m.setLastError(msg.Error)
			if msg.WorkspaceID != "" {
				return m, tea.Batch(FetchWorkspaceContext(m.be, string(msg.WorkspaceID)), FetchDecisions(m.be, string(msg.WorkspaceID)))
			}
			return m, nil
		}
		refreshWorkspaceID := msg.WorkspaceID
		if msg.Decision != nil {
			if refreshWorkspaceID == "" {
				refreshWorkspaceID = msg.Decision.WorkspaceID
			}
			if string(refreshWorkspaceID) == m.activeWorkspaceID && msg.Decision.RunID != "" {
				m.activeRunID = string(msg.Decision.RunID)
			}
			m.markDecisionResolvedForWorkspace(common.WorkspaceID(refreshWorkspaceID), msg.Decision.ID, msg.Decision.RunID)
			m.markDecisionResolved(msg.Decision.ID, common.WorkspaceID(refreshWorkspaceID), msg.Decision.RunID)
		}
		if string(refreshWorkspaceID) == m.activeWorkspaceID {
			m.blockingDecision = nil
			m.clearInput()
			m.progress = progressSubmitting
			m.progressText = "Decision resolved; waiting for server continuation"
		}
		cmds := []tea.Cmd{m.refreshActiveWorkspaceState()}
		if refreshWorkspaceID != "" {
			cmds = append(cmds, FetchWorkspaceContext(m.be, string(refreshWorkspaceID)), FetchDecisions(m.be, string(refreshWorkspaceID)))
		}
		return m, tea.Batch(cmds...)
	case ApprovalResolvedMsg:
		if msg.Error != nil {
			m.setLastError(msg.Error)
			return m, nil
		}
		refreshWorkspaceID := msg.WorkspaceID
		if msg.Approval != nil {
			if refreshWorkspaceID == "" {
				refreshWorkspaceID = msg.Approval.WorkspaceID
			}
			if string(refreshWorkspaceID) == m.activeWorkspaceID && msg.Approval.RunID != "" {
				m.activeRunID = string(msg.Approval.RunID)
			}
			m.markApprovalResolvedForWorkspace(refreshWorkspaceID, msg.Approval.ID, msg.Approval.RunID)
			m.markApprovalResolved(msg.Approval.ID, refreshWorkspaceID, msg.Approval.RunID)
		}
		if string(refreshWorkspaceID) == m.activeWorkspaceID {
			m.blockingApproval = nil
			m.clearInput()
			m.progress = progressSubmitting
			m.progressText = "Approval resolved; waiting for server continuation"
		}
		cmds := []tea.Cmd{m.refreshActiveWorkspaceState()}
		if refreshWorkspaceID != "" {
			cmds = append(cmds, FetchWorkspaceContext(m.be, string(refreshWorkspaceID)), FetchApprovals(m.be, string(refreshWorkspaceID)))
		}
		return m, tea.Batch(cmds...)
	case ConfigMsg:
		if msg.Error != nil {
			m.setLastError(msg.Error)
			return m, nil
		}
		m.result = renderConfigResult(msg.Config)
		m.progressText = "Showing system configuration"
		m.lastError = nil
		m.resetMainViewport()
		return m, nil
	case ChatModelTargetsLoadedMsg:
		return m.applyChatModelTargetsLoaded(msg)
	case WorkspaceLLMTargetUpdatedMsg:
		return m.applyWorkspaceLLMTargetUpdated(msg)
	case WorkspaceLLMTargetShowMsg:
		return m.applyWorkspaceLLMTargetShow(msg)
	case ErrorMsg:
		m.setLastError(msg.Error)
		return m, nil
	}
	return m, nil
}

func (m *AppModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if m.modelSelectMode {
		return m.handleModelSelectionKey(msg)
	}
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
	case KeyCtrlC:
		if m.running {
			m.running = false
			m.progress = progressInterrupted
			m.progressText = "Interrupted"
			return m, nil
		}
		m.progressText = "Use /exit to quit"
		m.lastError = nil
		return m, nil
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
		} else if m.slashCommandSuggestionActive() {
			m.moveSlashSuggestionCursor(1)
		}
		return m, nil
	case "up":
		if m.workspaceSelectMode {
			m.prevWorkspace()
		} else if m.slashCommandSuggestionActive() {
			m.moveSlashSuggestionCursor(-1)
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
	m.ensureActiveBlockingDecisionFromOverview()
	if decision := m.activeBlockingDecision(); decision != nil {
		ticketID := string(decision.ID)
		m.clearInput()
		m.lastError = nil
		m.appendTranscript("Decision", text)
		return m, ResolveDecision(m.be, m.activeWorkspaceID, ticketID, text)
	}
	m.ensureActiveBlockingApprovalFromOverview()
	if approval := m.activeBlockingApproval(); approval != nil {
		approvalID := string(approval.ID)
		approve, ok := parseApprovalInputWithContext(text, approval.Reason)
		if !ok {
			m.progress = progressWaitingApproval
			m.progressText = "Approval intent unclear; use a clear approval or rejection"
			m.lastError = nil
			return m, nil
		}
		m.clearInput()
		m.lastError = nil
		m.appendTranscript("Approval", text)
		return m, ResolveApproval(m.be, m.activeWorkspaceID, approvalID, approve, approvalResolutionPayload(text))
	}
	if m.activeWorkspaceID == "" {
		m.lastError = fmt.Errorf("no active workspace")
		return m, nil
	}
	if cmd := m.blockActiveWorkspaceResume(""); cmd != nil {
		m.lastError = nil
		return m, cmd
	}
	m.clearInput()
	m.lastError = nil
	if runID := m.activeWorkspaceRunID(); runID != "" {
		m.cancelWorkspaceSelection()
		m.clearRunDisplayState()
		m.appendTranscript("Input", text)
		m.progress = progressSubmitting
		m.progressText = "Resuming active run"
		return m, ResumeRun(m.be, m.activeWorkspaceID)
	}
	m.cancelWorkspaceSelection()
	return m.submitGoalText(text)
}

func (m *AppModel) submitGoalText(text string) (tea.Model, tea.Cmd) {
	text = strings.TrimSpace(text)
	if text == "" {
		m.lastError = fmt.Errorf("goal requires text")
		return m, nil
	}
	m.clearRunDisplayState()
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
	case "exit":
		m.addInputHistory(text)
		m.mode = modeQuit
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
		m.patches = nil
		m.llmUsage = backend.LLMUsageSummaryInfo{}
		m.llmContextSummary = projectioncontract.LLMContextBudgetSummary{}
		m.llmContext = nil
		m.acceptance = backend.AcceptanceProjectionInfo{}
		m.failureReason = ""
		m.lastError = nil
		m.resetMainViewport()
		return m, nil, true
	case "config", "configuration":
		m.addInputHistory(text)
		m.clearInput()
		m.progressText = "Loading system configuration"
		m.lastError = nil
		return m, FetchConfiguration(m.be), true
	case "model":
		m.addInputHistory(text)
		args := strings.TrimSpace(strings.TrimPrefix(cmd, strings.Fields(cmd)[0]))
		subcommand, _ := splitSlashSubcommand(args)
		switch subcommand {
		case "":
			m.clearInput()
			cmd := m.beginModelSelection(false)
			return m, cmd, true
		case "show":
			m.clearInput()
			if m.activeWorkspaceID == "" {
				m.lastError = fmt.Errorf("no active workspace")
				return m, nil, true
			}
			m.progressText = "Loading workspace model configuration"
			return m, FetchWorkspaceLLMTargetShow(m.be, m.activeWorkspaceID), true
		case "clear":
			m.clearInput()
			cmd := m.beginModelSelection(true)
			return m, cmd, true
		default:
			m.lastError = fmt.Errorf("unknown model subcommand: %s", subcommand)
			return m, nil, true
		}
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
	case "run":
		m.addInputHistory(text)
		args := strings.TrimSpace(strings.TrimPrefix(cmd, strings.Fields(cmd)[0]))
		subcommand, _ := splitSlashSubcommand(args)
		switch subcommand {
		case "resume":
			m.clearInput()
			m.cancelWorkspaceSelection()
			if cmd := m.blockActiveWorkspaceResume(""); cmd != nil {
				m.lastError = nil
				return m, cmd, true
			}
			m.clearRunDisplayState()
			m.lastError = nil
			m.progress = progressSubmitting
			if m.activeWorkspaceRunID() != "" {
				m.progressText = "Resuming active run"
			} else {
				m.progressText = "Checking workspace run state"
			}
			return m, ResumeRun(m.be, m.activeWorkspaceID), true
		case "restart":
			m.clearInput()
			m.cancelWorkspaceSelection()
			if m.activeWorkspaceID == "" {
				m.lastError = fmt.Errorf("no active workspace")
				return m, nil, true
			}
			m.clearRunDisplayState()
			m.lastError = nil
			m.progress = progressSubmitting
			m.progressText = "Starting fresh workspace activation"
			return m, RestartRun(m.be, m.activeWorkspaceID), true
		default:
			m.lastError = fmt.Errorf("unknown run subcommand: %s", subcommand)
			return m, nil, true
		}
	case "decision":
		m.addInputHistory(text)
		args := strings.TrimSpace(strings.TrimPrefix(cmd, strings.Fields(cmd)[0]))
		subcommand, rest := splitSlashSubcommand(args)
		switch subcommand {
		case "", "show":
			m.clearInput()
			m.result = renderDecisionCommandResult(m.blockingDecision, m.decisions)
			m.resetMainViewport()
			m.progressText = "Showing decision tickets"
			m.lastError = nil
			return m, nil, true
		case "list":
			m.clearInput()
			m.result = renderDecisionListResult(m.decisions)
			m.resetMainViewport()
			m.progressText = "Showing decision tickets"
			m.lastError = nil
			return m, FetchDecisions(m.be, m.activeWorkspaceID), true
		case "answer", "resolve":
			if !m.ensureActiveBlockingDecisionFromOverview() {
				m.lastError = fmt.Errorf("no blocking decision")
				return m, nil, true
			}
			if rest == "" {
				m.lastError = fmt.Errorf("decision answer requires text")
				return m, nil, true
			}
			ticketID := string(m.blockingDecision.ID)
			m.clearInput()
			m.lastError = nil
			m.appendTranscript("Decision", rest)
			return m, ResolveDecision(m.be, m.activeWorkspaceID, ticketID, rest), true
		case "terminate", "reject":
			if !m.ensureActiveBlockingDecisionFromOverview() {
				m.lastError = fmt.Errorf("no blocking decision")
				return m, nil, true
			}
			answer := nonEmpty(rest, "terminate current path")
			ticketID := string(m.blockingDecision.ID)
			m.clearInput()
			m.lastError = nil
			m.appendTranscript("Decision", answer)
			return m, ResolveDecisionWithType(m.be, m.activeWorkspaceID, ticketID, backend.DecisionResolutionTypeRejectCurrentPath, answer), true
		default:
			m.lastError = fmt.Errorf("unknown decision subcommand: %s", subcommand)
			return m, nil, true
		}
	case "approval":
		m.addInputHistory(text)
		args := strings.TrimSpace(strings.TrimPrefix(cmd, strings.Fields(cmd)[0]))
		subcommand, _ := splitSlashSubcommand(args)
		switch subcommand {
		case "", "show":
			m.clearInput()
			m.result = renderApprovalCommandResult(m.blockingApproval, m.approvals)
			m.resetMainViewport()
			m.progressText = "Showing approvals"
			m.lastError = nil
			return m, nil, true
		case "list":
			m.clearInput()
			m.result = renderApprovalListResult(m.approvals)
			m.resetMainViewport()
			m.progressText = "Showing approvals"
			m.lastError = nil
			return m, FetchApprovals(m.be, m.activeWorkspaceID), true
		case "approve", "yes":
			if !m.ensureActiveBlockingApprovalFromOverview() {
				m.lastError = fmt.Errorf("no blocking approval")
				return m, nil, true
			}
			approvalID := string(m.blockingApproval.ID)
			m.clearInput()
			m.lastError = nil
			m.appendTranscript("Approval", "approve")
			return m, ResolveApproval(m.be, m.activeWorkspaceID, approvalID, true, approvalResolutionPayload("approve")), true
		case "reject", "no":
			if !m.ensureActiveBlockingApprovalFromOverview() {
				m.lastError = fmt.Errorf("no blocking approval")
				return m, nil, true
			}
			approvalID := string(m.blockingApproval.ID)
			m.clearInput()
			m.lastError = nil
			m.appendTranscript("Approval", "reject")
			return m, ResolveApproval(m.be, m.activeWorkspaceID, approvalID, false, approvalResolutionPayload("reject")), true
		default:
			m.lastError = fmt.Errorf("unknown approval subcommand: %s", subcommand)
			return m, nil, true
		}
	case "goal":
		m.addInputHistory(text)
		args := strings.TrimSpace(strings.TrimPrefix(cmd, strings.Fields(cmd)[0]))
		if m.activeWorkspaceID == "" {
			m.lastError = fmt.Errorf("no active workspace")
			return m, nil, true
		}
		if args == "" {
			m.lastError = fmt.Errorf("goal requires text")
			return m, nil, true
		}
		next, submitCmd := m.submitGoalText(args)
		return next, submitCmd, true
	case "result", "refresh":
		m.addInputHistory(text)
		m.clearInput()
		m.resetMainViewport()
		m.progressText = "Refreshing workspace result"
		return m, m.requestWorkspaceContextRefresh(false), true
	case "memory", "mem":
		m.addInputHistory(text)
		m.clearInput()
		m.result = renderMemoryRecallSummary(m.memoryRecalls)
		m.resetMainViewport()
		m.progressText = "Showing memory recall summary"
		return m, nil, true
	case "status":
		m.addInputHistory(text)
		m.clearInput()
		m.result = m.renderInspectStatus()
		m.resetMainViewport()
		m.progressText = "Showing workspace status"
		m.lastError = nil
		return m, nil, true
	case "coverage":
		m.addInputHistory(text)
		m.clearInput()
		m.result = m.renderInspectCoverage()
		m.resetMainViewport()
		m.progressText = "Showing coverage progress"
		m.lastError = nil
		return m, nil, true
	case "steps":
		m.addInputHistory(text)
		m.clearInput()
		m.result = m.renderInspectSteps()
		m.resetMainViewport()
		m.progressText = "Showing recent steps"
		m.lastError = nil
		return m, nil, true
	case "timeline":
		m.addInputHistory(text)
		m.clearInput()
		m.result = renderInspectActivityFeed(m.activityFeed, 30)
		m.resetMainViewport()
		m.progressText = "Showing runtime timeline"
		m.lastError = nil
		return m, nil, true
	case "patches":
		m.addInputHistory(text)
		m.clearInput()
		m.result = renderInspectPatches(m.patches)
		m.resetMainViewport()
		m.progressText = "Showing patches"
		m.lastError = nil
		return m, nil, true
	case "artifacts":
		m.addInputHistory(text)
		m.clearInput()
		m.result = m.renderInspectArtifacts()
		m.resetMainViewport()
		m.progressText = "Showing artifacts"
		m.lastError = nil
		return m, nil, true
	case "dod":
		m.addInputHistory(text)
		args := strings.TrimSpace(strings.TrimPrefix(cmd, strings.Fields(cmd)[0]))
		if m.activeWorkspaceID == "" {
			m.lastError = fmt.Errorf("no active workspace")
			return m, nil, true
		}
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
		m.progress = progressSubmitting
		m.progressText = "Saving DoD"
		m.resetMainViewport()
		return m, PersistDoD(m.be, m.activeWorkspaceID, items), true
	default:
		m.lastError = fmt.Errorf("unknown slash command: /%s", name)
		return m, nil, true
	}
}

func splitSlashSubcommand(args string) (string, string) {
	args = strings.TrimSpace(args)
	if args == "" {
		return "", ""
	}
	fields := strings.Fields(args)
	if len(fields) == 0 {
		return "", ""
	}
	subcommand := strings.ToLower(fields[0])
	rest := strings.TrimSpace(strings.TrimPrefix(args, fields[0]))
	return subcommand, rest
}

func renderDecisionCommandResult(current *backend.DecisionInfo, items []backend.DecisionInfo) string {
	if current != nil {
		var b strings.Builder
		b.WriteString("Current decision\n")
		b.WriteString("- id: ")
		b.WriteString(string(current.ID))
		b.WriteString("\n")
		if current.State != "" {
			b.WriteString("- state: ")
			b.WriteString(current.State)
			b.WriteString("\n")
		}
		if current.Question != "" {
			b.WriteString("- question: ")
			b.WriteString(current.Question)
			b.WriteString("\n")
		}
		if current.Reason != "" {
			b.WriteString("- reason: ")
			b.WriteString(current.Reason)
			b.WriteString("\n")
		}
		b.WriteString("\nCommands\n")
		b.WriteString("- /decision answer <text>\n")
		b.WriteString("- /decision terminate [note]")
		return b.String()
	}
	return renderDecisionListResult(items)
}

func renderDecisionListResult(items []backend.DecisionInfo) string {
	if len(items) == 0 {
		return "No decision tickets for the current workspace."
	}
	var b strings.Builder
	b.WriteString("Decision tickets\n")
	for _, item := range items {
		b.WriteString("- ")
		b.WriteString(string(item.ID))
		if item.State != "" {
			b.WriteString(" [")
			b.WriteString(item.State)
			b.WriteString("]")
		}
		if text := nonEmpty(item.Question, item.Reason); text != "" {
			b.WriteString(": ")
			b.WriteString(text)
		}
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func renderApprovalCommandResult(current *backend.ApprovalInfo, items []backend.ApprovalInfo) string {
	if current != nil {
		var b strings.Builder
		b.WriteString("Current approval\n")
		b.WriteString("- id: ")
		b.WriteString(string(current.ID))
		b.WriteString("\n")
		if current.State != "" {
			b.WriteString("- state: ")
			b.WriteString(current.State)
			b.WriteString("\n")
		}
		if current.Action != "" {
			b.WriteString("- action: ")
			b.WriteString(current.Action)
			b.WriteString("\n")
		}
		if current.Reason != "" {
			b.WriteString("- reason: ")
			b.WriteString(current.Reason)
			b.WriteString("\n")
		}
		b.WriteString("\nCommands\n")
		b.WriteString("- /approval approve\n")
		b.WriteString("- /approval reject")
		return b.String()
	}
	return renderApprovalListResult(items)
}

func renderApprovalListResult(items []backend.ApprovalInfo) string {
	if len(items) == 0 {
		return "No approvals for the current workspace."
	}
	var b strings.Builder
	b.WriteString("Approvals\n")
	for _, item := range items {
		b.WriteString("- ")
		b.WriteString(string(item.ID))
		if item.State != "" {
			b.WriteString(" [")
			b.WriteString(item.State)
			b.WriteString("]")
		}
		if text := nonEmpty(item.Action, item.Reason); text != "" {
			b.WriteString(": ")
			b.WriteString(text)
		}
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
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
	return doditems.Normalize([]string{value})
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

func renderConfigResult(cfg *backend.ServiceConfig) string {
	if cfg == nil {
		return "System configuration is unavailable."
	}
	output := cfg.LLMOutput
	var b strings.Builder
	b.WriteString("System Configuration\n")
	b.WriteString(fmt.Sprintf("- Server: %s\n", nonEmpty(cfg.ServerAddr, "-")))
	b.WriteString(fmt.Sprintf("- Repository: %s %s\n", nonEmpty(cfg.Backend, "-"), nonEmpty(cfg.DataDir, "-")))
	b.WriteString(fmt.Sprintf("- Default LLM provider: %s\n", nonEmpty(cfg.DefaultChat.ProviderID, "-")))
	b.WriteString(fmt.Sprintf("- Max concurrent runs: %d\n", cfg.MaxConcurrentRuns))
	b.WriteString("\nLLM Output Budget\n")
	b.WriteString(fmt.Sprintf("- maxPatchContentBytes: %d\n", output.MaxPatchContentBytes))
	b.WriteString(fmt.Sprintf("- maxGraphBytes: %d\n", output.MaxGraphBytes))
	b.WriteString(fmt.Sprintf("- maxGraphFileWrites: %d\n", output.MaxGraphFileWrites))
	b.WriteString(fmt.Sprintf("- schemaInvalidLargePreviewThreshold: %d\n", output.SchemaInvalidLargePreviewThreshold))
	b.WriteString(fmt.Sprintf("- maxArtifactChunkBytes: %d\n", output.MaxArtifactChunkBytes))
	b.WriteString(fmt.Sprintf("- previousChunkTailBytes: %d\n", output.PreviousChunkTailBytes))
	b.WriteString(fmt.Sprintf("- maxChunkRetryAttempts: %d\n", output.MaxChunkRetryAttempts))
	b.WriteString(fmt.Sprintf("- maxArtifactPlanAttempts: %d\n", output.MaxArtifactPlanAttempts))
	b.WriteString(fmt.Sprintf("- maxArtifactReplanAttempts: %d\n", output.MaxArtifactReplanAttempts))
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

func (m *AppModel) upsertPatch(next backend.PatchInfo) {
	if next.ID == "" && next.TargetRef == "" && next.State == "" {
		return
	}
	for idx := range m.patches {
		if next.ID != "" && m.patches[idx].ID == next.ID {
			m.patches[idx] = mergePatchInfo(m.patches[idx], next)
			return
		}
	}
	m.patches = append(m.patches, next)
}

func mergePatchInfo(existing backend.PatchInfo, next backend.PatchInfo) backend.PatchInfo {
	if next.WorkspaceID != "" {
		existing.WorkspaceID = next.WorkspaceID
	}
	if next.RunID != "" {
		existing.RunID = next.RunID
	}
	if next.StepID != "" {
		existing.StepID = next.StepID
	}
	if next.ToolCallID != "" {
		existing.ToolCallID = next.ToolCallID
	}
	if next.ApprovalID != "" {
		existing.ApprovalID = next.ApprovalID
	}
	if next.PatchType != "" {
		existing.PatchType = next.PatchType
	}
	if next.TargetRef != "" {
		existing.TargetRef = next.TargetRef
	}
	if next.Summary != "" {
		existing.Summary = next.Summary
	}
	if next.State != "" {
		existing.State = next.State
	}
	if next.AddedLines != 0 {
		existing.AddedLines = next.AddedLines
	}
	if next.DeletedLines != 0 {
		existing.DeletedLines = next.DeletedLines
	}
	return existing
}

func (m *AppModel) upsertArtifactContinuation(next backend.ArtifactContinuationInfo) {
	if next.ID == "" && next.TargetRef == "" && next.State == "" {
		return
	}
	for idx := range m.artifactContinuations {
		if next.ID != "" && m.artifactContinuations[idx].ID == next.ID {
			m.artifactContinuations[idx] = mergeArtifactContinuationInfo(m.artifactContinuations[idx], next)
			return
		}
	}
	m.artifactContinuations = append(m.artifactContinuations, next)
}

func mergeArtifactContinuationInfo(existing backend.ArtifactContinuationInfo, next backend.ArtifactContinuationInfo) backend.ArtifactContinuationInfo {
	if next.WorkspaceID != "" {
		existing.WorkspaceID = next.WorkspaceID
	}
	if next.RunID != "" {
		existing.RunID = next.RunID
	}
	if next.RootStepID != "" {
		existing.RootStepID = next.RootStepID
	}
	if next.ArtifactID != "" {
		existing.ArtifactID = next.ArtifactID
	}
	if next.TargetRef != "" {
		existing.TargetRef = next.TargetRef
	}
	if next.Operation != "" {
		existing.Operation = next.Operation
	}
	if next.State != "" {
		existing.State = next.State
	}
	if next.ExpectedSequence != 0 {
		existing.ExpectedSequence = next.ExpectedSequence
	}
	if len(next.ReceivedSequences) > 0 {
		existing.ReceivedSequences = append([]int(nil), next.ReceivedSequences...)
	}
	if next.FinalReceived {
		existing.FinalReceived = true
	}
	if next.BytesWritten != 0 {
		existing.BytesWritten = next.BytesWritten
	}
	if next.FailureReason != "" {
		existing.FailureReason = next.FailureReason
	}
	if len(next.Progress.Fields) > 0 {
		existing.Progress = next.Progress.Clone()
	}
	return existing
}

func eventPayloadFromMap(input map[string]any) workorcheventpayload.Payload {
	payload, _ := workorcheventpayload.FromMap(input)
	return payload
}

func memoryIDsFromStrings(items []string) []common.MemoryID {
	ret := make([]common.MemoryID, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item) != "" {
			ret = append(ret, common.MemoryID(strings.TrimSpace(item)))
		}
	}
	return ret
}

func (m *AppModel) upsertMemoryRecall(next backend.MemoryRecallTraceInfo) {
	if next.ID == "" && next.Status == "" && len(next.RetrievedMemoryIDs) == 0 {
		return
	}
	for idx := range m.memoryRecalls {
		if next.ID != "" && m.memoryRecalls[idx].ID == next.ID {
			m.memoryRecalls[idx] = next
			return
		}
	}
	m.memoryRecalls = append(m.memoryRecalls, next)
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

func parseWorkspaceCreateArgs(value string) (workorchmanagementcontract.WorkspaceCreateRequest, error) {
	fields := strings.Fields(value)
	req := workorchmanagementcontract.WorkspaceCreateRequest{}
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
			req.DeliverableDirectory = fields[idx]
		case strings.HasPrefix(field, "--dir="):
			req.DeliverableDirectory = strings.TrimPrefix(field, "--dir=")
		case strings.HasPrefix(field, "--working-dir="):
			req.DeliverableDirectory = strings.TrimPrefix(field, "--working-dir=")
		case strings.HasPrefix(field, "dir="):
			req.DeliverableDirectory = strings.TrimPrefix(field, "dir=")
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
	semantic := governancesemantics.NormalizeApprovalAction("", value, "")
	switch semantic.Action {
	case "approve":
		return true, true
	case "reject":
		return false, true
	default:
		return false, false
	}
}

func parseApprovalInputWithContext(value string, reason string) (bool, bool) {
	semantic := governancesemantics.NormalizeApprovalAction("", value, reason)
	switch semantic.Action {
	case "approve":
		return true, true
	case "reject":
		return false, true
	default:
		return false, false
	}
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
	typedContext, err := workorcheventpayload.FromMap(context)
	if err != nil {
		return
	}
	item := backend.ApprovalInfo{
		ID:          id,
		WorkspaceID: workspaceID,
		RunID:       runID,
		Reason:      reason,
		Action:      action,
		Context:     typedContext,
		State:       "pending",
	}
	m.blockingApproval = &item
	m.upsertApproval(item)
}

func (m *AppModel) fetchActiveBlockerDetails() tea.Cmd {
	var cmds []tea.Cmd
	if decision := m.activeBlockingDecision(); decision != nil && decision.ID != "" {
		cmds = append(cmds, FetchDecision(m.be, m.activeWorkspaceID, string(decision.ID)))
	}
	if approval := m.activeBlockingApproval(); approval != nil && approval.ID != "" {
		cmds = append(cmds, FetchApproval(m.be, m.activeWorkspaceID, string(approval.ID)))
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
		matched := false
		for _, item := range m.decisions {
			if item.ID == m.blockingDecision.ID {
				matched = true
				merged := mergeDecisionInfo(*m.blockingDecision, item)
				if merged.State != "open" {
					m.blockingDecision = nil
					m.clearWaitingProgressIfUnblocked()
					return
				}
				if !m.decisionMatchesActiveScope(merged) {
					m.blockingDecision = nil
					if !m.ensureActiveBlockingDecisionFromOverview() {
						m.clearWaitingProgressIfUnblocked()
					}
					return
				}
				m.blockingDecision = &merged
				if m.progress == progressWaitingDecision {
					m.progressText = blockingDecisionLabel(m.blockingDecision)
				}
				return
			}
		}
		if !matched {
			m.blockingDecision = nil
			if !m.ensureActiveBlockingDecisionFromOverview() {
				m.clearWaitingProgressIfUnblocked()
			}
		}
	}
	for _, item := range filterOpenDecisions(m.decisions) {
		if m.decisionMatchesActiveScope(item) {
			m.blockingDecision = &item
			if m.progress == progressWaitingDecision {
				m.progressText = blockingDecisionLabel(m.blockingDecision)
			}
			return
		}
	}
	m.ensureActiveBlockingDecisionFromOverview()
}

func (m *AppModel) syncBlockingApprovalFromList() {
	if m.blockingApproval != nil {
		matched := false
		for _, item := range m.approvals {
			if item.ID == m.blockingApproval.ID {
				matched = true
				merged := mergeApprovalInfo(*m.blockingApproval, item)
				if merged.State != "pending" {
					m.blockingApproval = nil
					m.clearWaitingProgressIfUnblocked()
					return
				}
				if !m.approvalMatchesActiveScope(merged) {
					m.blockingApproval = nil
					if !m.ensureActiveBlockingApprovalFromOverview() {
						m.clearWaitingProgressIfUnblocked()
					}
					return
				}
				m.blockingApproval = &merged
				if m.progress == progressWaitingApproval {
					m.progressText = blockingApprovalLabel(m.blockingApproval)
				}
				return
			}
		}
		if !matched {
			m.blockingApproval = nil
			if !m.ensureActiveBlockingApprovalFromOverview() {
				m.clearWaitingProgressIfUnblocked()
			}
		}
	}
	for _, item := range filterPendingApprovals(m.approvals) {
		if m.approvalMatchesActiveScope(item) {
			m.blockingApproval = &item
			if m.progress == progressWaitingApproval {
				m.progressText = blockingApprovalLabel(m.blockingApproval)
			}
			return
		}
	}
	m.ensureActiveBlockingApprovalFromOverview()
}

func (m *AppModel) clearWaitingProgressIfUnblocked() {
	if m.blockingDecision != nil || m.blockingApproval != nil {
		return
	}
	switch m.progress {
	case progressWaitingDecision, progressWaitingApproval:
		m.progress = progressIdle
		m.progressText = ""
	}
}

func (m *AppModel) followBlockerTailUnlessUserScrolled() {
	if m.activeBlockingDecision() == nil && m.activeBlockingApproval() == nil {
		return
	}
	m.followMainViewportTailUnlessUserScrolled()
}

func (m *AppModel) activeBlockingDecision() *backend.DecisionInfo {
	ws := m.activeWorkspaceOverview()
	if ws != nil {
		if ws.State == common.WorkspaceStateActive {
			if ws.BlockingTicketID == "" || !workspaceOverviewHasProjectionAuthority(ws) {
				return nil
			}
			if m.blockingDecision != nil && m.blockingDecision.ID == ws.BlockingTicketID && m.decisionMatchesActiveScope(*m.blockingDecision) {
				return m.blockingDecision
			}
			item := backend.DecisionInfo{
				ID:          ws.BlockingTicketID,
				WorkspaceID: ws.ID,
				RunID:       ws.ActiveRunID,
				State:       "open",
			}
			return &item
		}
	}
	return nil
}

func (m *AppModel) activeBlockingApproval() *backend.ApprovalInfo {
	ws := m.activeWorkspaceOverview()
	if ws != nil {
		if ws.State == common.WorkspaceStateBlockedOnApproval {
			if ws.BlockingApprovalID == "" || !workspaceOverviewHasProjectionAuthority(ws) {
				return nil
			}
			if m.blockingApproval != nil && m.blockingApproval.ID == ws.BlockingApprovalID && m.approvalMatchesActiveScope(*m.blockingApproval) {
				return m.blockingApproval
			}
			item := backend.ApprovalInfo{
				ID:          ws.BlockingApprovalID,
				WorkspaceID: ws.ID,
				RunID:       ws.ActiveRunID,
				State:       "pending",
			}
			return &item
		}
	}
	return nil
}

func (m *AppModel) decisionMatchesActiveScope(item backend.DecisionInfo) bool {
	if m.activeWorkspaceID != "" && item.WorkspaceID != "" && item.WorkspaceID != common.WorkspaceID(m.activeWorkspaceID) {
		return false
	}
	if m.activeRunID != "" {
		return item.RunID == common.RunID(m.activeRunID)
	}
	ws := m.activeWorkspaceOverview()
	if ws == nil || ws.State != common.WorkspaceStateActive || ws.BlockingTicketID == "" || !workspaceOverviewHasProjectionAuthority(ws) {
		return false
	}
	if item.ID != ws.BlockingTicketID {
		return false
	}
	if ws.ActiveRunID != "" {
		return item.RunID == ws.ActiveRunID
	}
	return true
}

func (m *AppModel) approvalMatchesActiveScope(item backend.ApprovalInfo) bool {
	if m.activeWorkspaceID != "" && item.WorkspaceID != "" && item.WorkspaceID != common.WorkspaceID(m.activeWorkspaceID) {
		return false
	}
	if m.activeRunID != "" {
		return item.RunID == common.RunID(m.activeRunID)
	}
	ws := m.activeWorkspaceOverview()
	if ws == nil || ws.State != common.WorkspaceStateBlockedOnApproval || ws.BlockingApprovalID == "" || !workspaceOverviewHasProjectionAuthority(ws) {
		return false
	}
	if item.ID != ws.BlockingApprovalID {
		return false
	}
	if ws.ActiveRunID != "" {
		return item.RunID == ws.ActiveRunID
	}
	return true
}

func (m AppModel) activeScopeDecisions() []backend.DecisionInfo {
	var out []backend.DecisionInfo
	for _, item := range m.decisions {
		if m.activeWorkspaceID != "" && item.WorkspaceID != "" && item.WorkspaceID != common.WorkspaceID(m.activeWorkspaceID) {
			continue
		}
		if !m.decisionMatchesActiveScope(item) {
			continue
		}
		out = append(out, item)
	}
	return out
}

func (m AppModel) activeScopeApprovals() []backend.ApprovalInfo {
	var out []backend.ApprovalInfo
	for _, item := range m.approvals {
		if m.activeWorkspaceID != "" && item.WorkspaceID != "" && item.WorkspaceID != common.WorkspaceID(m.activeWorkspaceID) {
			continue
		}
		if !m.approvalMatchesActiveScope(item) {
			continue
		}
		out = append(out, item)
	}
	return out
}

func (m *AppModel) markDecisionResolved(id common.TicketID, workspaceID common.WorkspaceID, runID common.RunID) {
	if id == "" {
		return
	}
	for i := range m.decisions {
		if m.decisions[i].ID == id && decisionScopeMatches(m.decisions[i], workspaceID, runID) {
			m.decisions[i].State = "resolved"
			return
		}
	}
}

func (m *AppModel) markApprovalResolved(id common.ApprovalID, workspaceID common.WorkspaceID, runID common.RunID) {
	if id == "" {
		return
	}
	for i := range m.approvals {
		if m.approvals[i].ID == id && approvalScopeMatches(m.approvals[i], workspaceID, runID) {
			m.approvals[i].State = "resolved"
			return
		}
	}
}

func decisionScopeMatches(item backend.DecisionInfo, workspaceID common.WorkspaceID, runID common.RunID) bool {
	if workspaceID != "" && item.WorkspaceID != "" && item.WorkspaceID != workspaceID {
		return false
	}
	if runID != "" && item.RunID != "" && item.RunID != runID {
		return false
	}
	return true
}

func approvalScopeMatches(item backend.ApprovalInfo, workspaceID common.WorkspaceID, runID common.RunID) bool {
	if workspaceID != "" && item.WorkspaceID != "" && item.WorkspaceID != workspaceID {
		return false
	}
	if runID != "" && item.RunID != "" && item.RunID != runID {
		return false
	}
	return true
}

func (m *AppModel) applySubscriptionStatus(status backend.SubscriptionStatusInfo) tea.Cmd {
	previous := m.subscriptionStatus.State
	m.subscriptionStatus = status
	if status.State == backend.SubscriptionLive && previous != backend.SubscriptionLive {
		// reconnect / first live: list + active RuntimeView readback
		return tea.Batch(m.requestWorkspaceListRefresh(), m.requestWorkspaceContextRefresh(false))
	}
	return nil
}

func (m *AppModel) applyWorkspaceProjectionChanged(info backend.WorkspaceProjectionChangedInfo) tea.Cmd {
	workspaceID := strings.TrimSpace(string(info.WorkspaceID))
	if workspaceID == "" {
		return m.requestWorkspaceListRefresh()
	}
	entry := m.ensureWorkspaceRuntimeState(workspaceID)
	if entry == nil {
		return m.requestWorkspaceListRefresh()
	}
	// notification only advances DirtyRevision, never AppliedRevision.
	generationChanged := revisionGenerationChanged(entry.DirtyRevision, info.Revision)
	entry.DirtyRevision = mergeRefreshRevisionMax(entry.DirtyRevision, info.Revision)
	if generationChanged || info.Revision.ActivityRevision > entry.ActivityDirtyRevision {
		entry.ActivityDirtyRevision = info.Revision.ActivityRevision
	}
	if strings.TrimSpace(info.Revision.StreamGeneration) != "" &&
		strings.TrimSpace(info.Revision.StreamGeneration) != strings.TrimSpace(m.subscriptionStatus.StreamGeneration) {
		m.subscriptionStatus.StreamGeneration = info.Revision.StreamGeneration
		m.subscriptionStatus.StreamRevision = info.Revision.StreamRevision
		m.subscriptionStatus.LastEventAt = info.UpdatedAt
		m.subscriptionStatus.State = backend.SubscriptionLive
	} else if info.Revision.StreamRevision > m.subscriptionStatus.StreamRevision {
		m.subscriptionStatus.StreamRevision = info.Revision.StreamRevision
		m.subscriptionStatus.LastEventAt = info.UpdatedAt
		if m.subscriptionStatus.State == "" || m.subscriptionStatus.State == backend.SubscriptionConnecting {
			m.subscriptionStatus.State = backend.SubscriptionLive
		}
	}
	if m.activeWorkspaceID != "" && workspaceID == m.activeWorkspaceID {
		if projectionSectionsNeedRuntimeView(info.ChangedSections) {
			return tea.Batch(m.requestWorkspaceListRefresh(), m.requestWorkspaceContextRefresh(false))
		}
		return m.requestWorkspaceListRefresh()
	}
	return m.requestWorkspaceListRefresh()
}

func projectionSectionsNeedRuntimeView(sections []backend.WorkspaceProjectionSection) bool {
	if len(sections) == 0 {
		// empty sections may represent health/state-only changes; still readback once.
		return true
	}
	for _, section := range sections {
		switch section {
		case backend.WorkspaceProjectionSectionRuntimeProgress,
			backend.WorkspaceProjectionSectionActivity,
			backend.WorkspaceProjectionSectionGovernance,
			backend.WorkspaceProjectionSectionEvidence,
			backend.WorkspaceProjectionSectionResult:
			return true
		}
	}
	return false
}

func mergeRefreshRevisionMax(current, incoming projectioncontract.WorkspaceRefreshRevision) projectioncontract.WorkspaceRefreshRevision {
	if revisionGenerationChanged(current, incoming) {
		return incoming
	}
	if strings.TrimSpace(current.StreamGeneration) == "" {
		current.StreamGeneration = strings.TrimSpace(incoming.StreamGeneration)
	}
	if incoming.StreamRevision > current.StreamRevision {
		current.StreamRevision = incoming.StreamRevision
	}
	if incoming.WorkspaceRevision > current.WorkspaceRevision {
		current.WorkspaceRevision = incoming.WorkspaceRevision
	}
	if incoming.ProjectedSourceRevision > current.ProjectedSourceRevision {
		current.ProjectedSourceRevision = incoming.ProjectedSourceRevision
	}
	if incoming.RuntimeStateRevision > current.RuntimeStateRevision {
		current.RuntimeStateRevision = incoming.RuntimeStateRevision
	}
	if incoming.EvidenceSequence > current.EvidenceSequence {
		current.EvidenceSequence = incoming.EvidenceSequence
	}
	if incoming.ActivityRevision > current.ActivityRevision {
		current.ActivityRevision = incoming.ActivityRevision
	}
	return current
}

func (m *AppModel) requestWorkspaceListRefresh() tea.Cmd {
	if m.workspaceListRefreshInFlight {
		m.workspaceListRefreshPending = true
		return nil
	}
	m.workspaceListRefreshInFlight = true
	m.workspaceListRequestSeq++
	return FetchAll(m.be, m.workspaceListRequestSeq)
}

func (m *AppModel) requestWorkspaceContextRefresh(resetMainViewport bool) tea.Cmd {
	return m.requestWorkspaceContextFor(m.activeWorkspaceID, resetMainViewport)
}

func (m *AppModel) requestWorkspaceContextFor(workspaceID string, resetMainViewport bool) tea.Cmd {
	if strings.TrimSpace(workspaceID) == "" {
		return nil
	}
	entry := m.ensureWorkspaceRuntimeState(workspaceID)
	if entry == nil {
		return nil
	}
	if entry.ContextInFlight {
		entry.ContextPending = true
		return nil
	}
	entry.ContextInFlight = true
	entry.ContextRequestSeq++
	requestSeq := entry.ContextRequestSeq
	if resetMainViewport {
		return FetchWorkspaceContextForSwitch(m.be, workspaceID, requestSeq)
	}
	return FetchWorkspaceContext(m.be, workspaceID, requestSeq)
}

func (m *AppModel) workspaceContextRequestSeqFor(workspaceID string) int64 {
	if strings.TrimSpace(workspaceID) == "" {
		workspaceID = m.activeWorkspaceID
	}
	entry := m.workspaceRuntimeState(workspaceID)
	if entry == nil {
		return 0
	}
	return entry.ContextRequestSeq
}

func (m *AppModel) refreshActiveWorkspaceState() tea.Cmd {
	return tea.Batch(
		m.requestWorkspaceContextRefresh(false),
		m.requestWorkspaceListRefresh(),
	)
}

func isTransientProjectionError(err error) bool {
	if err == nil {
		return false
	}
	if isTransientContextError(err) {
		return true
	}
	text := strings.ToLower(err.Error())
	return (errors.Is(err, common.ErrConflict) || strings.Contains(text, "status 409") || strings.Contains(text, "conflict")) &&
		strings.Contains(text, "workspace projection")
}

func (m *AppModel) workspaceMessageBelongsToActiveWorkspace(workspaceID string) bool {
	if strings.TrimSpace(workspaceID) == "" || strings.TrimSpace(m.activeWorkspaceID) == "" {
		return true
	}
	return workspaceID == m.activeWorkspaceID
}

func (m *AppModel) runMessageBelongsToActiveWorkspace(workspaceID string, run *backend.RunInfo) bool {
	if strings.TrimSpace(m.activeWorkspaceID) == "" {
		return true
	}
	if strings.TrimSpace(workspaceID) != "" {
		return workspaceID == m.activeWorkspaceID
	}
	if run != nil && run.WorkspaceID != "" {
		return string(run.WorkspaceID) == m.activeWorkspaceID
	}
	return true
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
	case common.RunStatePending:
		m.running = false
		m.progress = progressIdle
		m.progressText = "Run created"
	case common.RunStateCancelled, common.RunStateSuperseded:
		m.running = false
		m.blockingDecision = nil
		m.blockingApproval = nil
		m.progress = progressInterrupted
		m.progressText = string(state)
	case common.RunStateBlockedOnDecision:
		m.running = false
		m.progress = progressWaitingDecision
		if ticketID := common.TicketID(firstPayloadString(payload, "ticketId", "ID", "id", backend.PayloadKeyBlockingTicketID)); ticketID != "" {
			m.setBlockingDecision(ticketID, common.WorkspaceID(m.activeWorkspaceID), common.RunID(m.activeRunID), "", "")
		}
		m.progressText = nonEmpty(blockingDecisionLabel(m.blockingDecision), "Waiting for decision")
	case common.RunStateBlockedOnApproval:
		m.running = false
		m.progress = progressWaitingApproval
		if approvalID := common.ApprovalID(firstPayloadString(payload, "approvalId", "ID", "id", backend.PayloadKeyBlockingApprovalID)); approvalID != "" {
			m.setBlockingApproval(approvalID, common.WorkspaceID(m.activeWorkspaceID), common.RunID(m.activeRunID), "", "", nil)
		}
		m.progressText = nonEmpty(blockingApprovalLabel(m.blockingApproval), "Waiting for approval")
	default:
		if state != "" {
			if runStateIsLive(state) {
				m.clearBlockingInputState()
				m.clearResultDisplayState()
				m.syncMainViewportToCurrentContent()
			}
			m.running = true
			m.progress = progressRunning
			if state == common.RunStateActive {
				m.progressText = "Running; waiting for progress or final result"
			} else {
				m.progressText = string(state) + "; waiting for progress or final result"
			}
		}
	}
}

func (m *AppModel) applyRunControlState(run *backend.RunInfo, message string) {
	if run == nil {
		if strings.TrimSpace(message) != "" {
			m.progressText = message
		}
		return
	}
	if runRequiresFreshActivation(run) {
		m.running = false
		m.progress = progressIdle
		m.progressText = "Run requires a fresh activation"
		m.result = freshActivationInstruction(run)
		m.syncMainViewportToCurrentContent()
		return
	}
	// A command reply is an owner admission outcome, not a Run state snapshot.
	// In particular, restart_required/governance_required/replacement_pending
	// must not take the UI into the running state merely because transport
	// delivered the request successfully.
	if disposition := strings.TrimSpace(run.ExecutionDisposition); disposition != "" && disposition != "reconciliation_requested" {
		m.running = false
		m.progress = progressIdle
		m.progressText = nonEmpty(run.ExecutionDiagnostic, disposition)
		return
	}
	if runStateIsLive(run.State) {
		m.clearRunDisplayState()
	}
	switch run.State {
	case common.RunStateCompleted:
		m.running = false
		m.blockingDecision = nil
		m.blockingApproval = nil
		m.progress = progressCompleted
		m.progressText = nonEmpty(message, "Completed")
	case common.RunStateFailed:
		m.running = false
		m.blockingDecision = nil
		m.blockingApproval = nil
		m.progress = progressFailed
		m.failureReason = run.FailureReason
		m.progressText = nonEmpty(message, m.failureReason, "Run failed")
	case common.RunStatePending:
		m.running = false
		m.progress = progressIdle
		m.progressText = nonEmpty(message, "Run created")
	case common.RunStateCancelled, common.RunStateSuperseded:
		m.running = false
		m.blockingDecision = nil
		m.blockingApproval = nil
		m.progress = progressInterrupted
		m.progressText = nonEmpty(message, string(run.State))
	case common.RunStateBlockedOnDecision:
		m.running = false
		m.progress = progressWaitingDecision
		if run.BlockingTicketID != "" {
			m.setBlockingDecision(run.BlockingTicketID, run.WorkspaceID, run.ID, "", "")
		}
		m.progressText = nonEmpty(message, blockingDecisionLabel(m.blockingDecision), "Waiting for decision")
	case common.RunStateBlockedOnApproval:
		m.running = false
		m.progress = progressWaitingApproval
		if run.BlockingApprovalID != "" {
			m.setBlockingApproval(run.BlockingApprovalID, run.WorkspaceID, run.ID, "", "", nil)
		}
		m.progressText = nonEmpty(message, blockingApprovalLabel(m.blockingApproval), "Waiting for approval")
	default:
		m.running = true
		m.progress = progressRunning
		m.progressText = nonEmpty(message, "Resuming active run")
	}
}

func runRequiresFreshActivation(run *backend.RunInfo) bool {
	return run != nil && run.RuntimeActivation != nil && run.RuntimeActivation.Disposition == "fresh_activation_required"
}

func freshActivationInstruction(run *backend.RunInfo) string {
	if run == nil || run.RuntimeActivation == nil {
		return "This run cannot be resumed by this binary. Use /run restart to create a fresh activation."
	}
	status := run.RuntimeActivation
	if status.ActualSchemaVersion > 0 && status.SupportedSchemaVersion > 0 {
		return fmt.Sprintf("This run uses persisted runtime schema v%d, while this binary supports v%d. It is intentionally not resumed. Use /run restart to create a fresh activation.", status.ActualSchemaVersion, status.SupportedSchemaVersion)
	}
	return "This run cannot be resumed by this binary. Use /run restart to create a fresh activation."
}

func (m *AppModel) applyRunStarted(run *backend.RunInfo) {
	if run == nil {
		return
	}
	switch run.State {
	case common.RunStateCompleted, common.RunStateFailed, common.RunStateBlockedOnDecision, common.RunStateBlockedOnApproval:
		m.applyRunState(run.State, map[string]any{
			backend.PayloadKeyFailureReason:      string(run.FailureReason),
			backend.PayloadKeyBlockingTicketID:   string(run.BlockingTicketID),
			backend.PayloadKeyBlockingApprovalID: string(run.BlockingApprovalID),
		})
	case "":
		if m.progress == progressIdle || m.progress == progressSubmitting {
			m.progress = progressRunning
			m.progressText = "Run started; waiting for progress or final result"
		}
	default:
		if m.progress == progressIdle || m.progress == progressSubmitting {
			m.progress = progressRunning
			if strings.TrimSpace(m.progressText) == "" || strings.HasPrefix(m.progressText, "Resuming active run") {
				m.progressText = "Run started; waiting for progress or final result"
			}
		}
	}
}

func (m *AppModel) clearRunDisplayState() {
	m.clearResultDisplayState()
	m.patches = nil
	m.artifactContinuations = nil
	m.taskDecomp = nil
	m.taskPlans = nil
	m.memoryRecalls = nil
	m.llmUsage = backend.LLMUsageSummaryInfo{}
	m.llmContextSummary = projectioncontract.LLMContextBudgetSummary{}
	m.llmContext = nil
	m.acceptance = backend.AcceptanceProjectionInfo{}
	m.runCommand = nil
	m.runtime = nil
	m.activityFeed = backend.WorkspaceActivityFeedInfo{}
	m.repairLoop = ""
	m.failureReason = ""
	m.failureHint = ""
	m.blockingDecision = nil
	m.blockingApproval = nil
	m.decisions = nil
	m.approvals = nil
	m.syncMainViewportToCurrentContent()
}

func (m *AppModel) clearResultDisplayState() {
	m.result = ""
	m.artifacts = nil
	m.sources = nil
	m.failureReason = ""
	m.failureHint = ""
}

func (m *AppModel) syncMainViewportToCurrentContent() {
	width, height := m.contentDimensions()
	if width > 0 && height > 0 {
		m.refreshMainViewport(width, height)
	}
	m.resetMainViewport()
}

func (m AppModel) emptyResultText() string {
	if len(m.workspaces) == 0 && strings.TrimSpace(m.activeWorkspaceID) == "" {
		return "No workspaces found.\n\nCreate one from this TUI:\n  /workspace create <title> --dir <path>\n  /workspace create <title> --template <template-id> --dir <path>\n\nUseful commands:\n  /help          Show commands\n  /goal          Submit a new goal\n  /dod           Set acceptance criteria\n  /run resume    Resume active run\n  /run restart   Create a fresh activation\n  /exit          Quit"
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

func workspaceLabel(ws backend.WorkspaceInfo) string {
	name := workspaceName(ws)
	if name != "" && name != shortID(string(ws.ID)) {
		return name + " (" + shortID(string(ws.ID)) + ")"
	}
	return shortID(string(ws.ID))
}

func governanceProgressText(fallbackStatus string, payload map[string]any) string {
	itemType := strings.TrimSpace(firstPayloadString(payload, backend.PayloadKeyItemType, "itemType", "ItemType"))
	action := strings.TrimSpace(firstPayloadString(payload, backend.PayloadKeyAction, "action", "Action"))
	status := strings.TrimSpace(firstPayloadString(payload, backend.PayloadKeyStatus, "status", "Status"))
	risk := strings.TrimSpace(firstPayloadString(payload, backend.PayloadKeyRisk, "risk", "Risk"))
	if status == "" {
		status = fallbackStatus
	}
	if itemType == "" {
		itemType = "item"
	}
	if action == "" {
		action = "process"
	}
	base := fmt.Sprintf("Auto governance %s: %s/%s", status, itemType, action)
	if risk != "" {
		base += " (" + risk + ")"
	}
	return base
}

func workspaceName(ws backend.WorkspaceInfo) string {
	if strings.TrimSpace(ws.Title) != "" {
		return strings.TrimSpace(ws.Title)
	}
	if strings.TrimSpace(ws.DeliverableDirectory) != "" {
		parts := strings.Split(strings.TrimRight(ws.DeliverableDirectory, "/"), "/")
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

func visibleWorkspaces(items []backend.WorkspaceInfo, cursor int, limit int) []backend.WorkspaceInfo {
	if len(items) == 0 || limit <= 0 {
		return nil
	}
	if limit >= len(items) {
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

func workspacePanelLine(prefix string, ws backend.WorkspaceInfo, counts map[string]int, width int) string {
	name := workspaceName(ws)
	state := string(ws.State)
	if strings.TrimSpace(state) == "" {
		state = "unknown"
	}
	id := shortID(string(ws.ID))
	suffix := "  " + state
	if ws.ActiveRunID != "" {
		suffix += "  " + shortID(string(ws.ActiveRunID))
	}
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

func payloadIntValue(payload map[string]any, keys ...string) int {
	for _, key := range keys {
		value, ok := payload[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case int:
			return typed
		case int64:
			return int(typed)
		case float64:
			return int(typed)
		case float32:
			return int(typed)
		}
	}
	return 0
}

func payloadIntSliceValue(payload map[string]any, keys ...string) []int {
	for _, key := range keys {
		value, ok := payload[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case []int:
			return append([]int(nil), typed...)
		case []int64:
			ret := make([]int, 0, len(typed))
			for _, item := range typed {
				ret = append(ret, int(item))
			}
			return ret
		case []any:
			ret := make([]int, 0, len(typed))
			for _, item := range typed {
				switch v := item.(type) {
				case int:
					ret = append(ret, v)
				case int64:
					ret = append(ret, int(v))
				case float64:
					ret = append(ret, int(v))
				case float32:
					ret = append(ret, int(v))
				}
			}
			if len(ret) > 0 {
				return ret
			}
		}
	}
	return nil
}

func payloadBoolValue(payload map[string]any, keys ...string) bool {
	if len(payload) == 0 {
		return false
	}
	for _, key := range keys {
		value, ok := payload[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case bool:
			return typed
		case string:
			switch strings.ToLower(strings.TrimSpace(typed)) {
			case "true", "1", "yes":
				return true
			}
		}
	}
	return false
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
	if output := payloadMap(payload, "output", "Output", backend.PayloadKeyOutput); output != nil {
		if value := failureMessageFromPayload(output); value != "" {
			return value
		}
	}
	reason := firstPayloadString(payload, "reason", "Reason", "error", "Error", "failureReason", "FailureReason", backend.PayloadKeyReason, backend.PayloadKeyFailureReason)
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
			add(string(ref))
		}
		if result.SummaryDetail != nil {
			for _, ref := range result.SummaryDetail.ArtifactRefs {
				add(string(ref))
			}
		}
	}
	return out
}

func snapshotWebSourceLabels(snapshot *backend.WorkspaceSnapshotInfo, runID common.RunID) []string {
	if snapshot == nil || len(snapshot.WebSources) == 0 {
		return nil
	}
	out := make([]string, 0)
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
	_ = runID
	for _, trace := range snapshot.WebSources {
		for _, source := range trace.FetchedSources {
			url := nonEmpty(source.FinalURL, source.URL)
			if url == "" {
				continue
			}
			add(webSourceLabel("fetch", source.Title, source.Source, url, source.EvidenceRef))
		}
		for _, source := range trace.Sources {
			if source.URL == "" {
				continue
			}
			add(webSourceLabel("search", source.Title, source.Source, source.URL, source.EvidenceRef))
		}
	}
	if len(out) > 8 {
		return out[:8]
	}
	return out
}

func webSourceLabel(kind string, title string, source string, url string, evidenceRef string) string {
	label := strings.TrimSpace(nonEmpty(title, source, url))
	if label != url {
		label += " - " + url
	}
	if evidenceRef != "" {
		label += " (" + evidenceRef + ")"
	}
	return kind + ": " + label
}

func snapshotPatches(snapshot *backend.WorkspaceSnapshotInfo, runID common.RunID) []backend.PatchInfo {
	if snapshot == nil || len(snapshot.Patches) == 0 {
		return nil
	}
	out := make([]backend.PatchInfo, 0, len(snapshot.Patches))
	for _, item := range snapshot.Patches {
		if runID != "" && item.RunID != "" && item.RunID != runID {
			continue
		}
		out = append(out, item)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func snapshotArtifactContinuations(snapshot *backend.WorkspaceSnapshotInfo, runID common.RunID) []backend.ArtifactContinuationInfo {
	if snapshot == nil || len(snapshot.ArtifactContinuations) == 0 {
		return nil
	}
	out := make([]backend.ArtifactContinuationInfo, 0, len(snapshot.ArtifactContinuations))
	for _, item := range snapshot.ArtifactContinuations {
		if runID != "" && item.RunID != "" && item.RunID != runID {
			continue
		}
		out = append(out, item)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func snapshotTaskDecompositions(snapshot *backend.WorkspaceSnapshotInfo, runID common.RunID) []backend.TaskDecompositionInfo {
	if snapshot == nil || len(snapshot.TaskDecompositions) == 0 {
		return nil
	}
	out := make([]backend.TaskDecompositionInfo, 0, len(snapshot.TaskDecompositions))
	for _, item := range snapshot.TaskDecompositions {
		if runID != "" && item.RunID != "" && item.RunID != runID {
			continue
		}
		out = append(out, item)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func snapshotTaskPlans(snapshot *backend.WorkspaceSnapshotInfo, runID common.RunID) []backend.TaskPlanInfo {
	if snapshot == nil || len(snapshot.TaskPlans) == 0 {
		return nil
	}
	out := make([]backend.TaskPlanInfo, 0, len(snapshot.TaskPlans))
	for _, item := range snapshot.TaskPlans {
		if runID != "" && item.RunID != "" && item.RunID != runID {
			continue
		}
		out = append(out, item)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func artifactContinuationDisplayLabel(item backend.ArtifactContinuationInfo) string {
	parts := make([]string, 0, 5)
	if item.State != "" {
		parts = append(parts, string(item.State))
	}
	if item.TargetRef != "" {
		parts = append(parts, item.TargetRef)
	} else if item.ArtifactID != "" {
		parts = append(parts, item.ArtifactID)
	} else if item.ID != "" {
		parts = append(parts, string(item.ID))
	}
	if item.ExpectedSequence > 0 || len(item.ReceivedSequences) > 0 {
		parts = append(parts, fmt.Sprintf("seq %d/%s", len(item.ReceivedSequences), artifactExpectedSequenceText(item.ExpectedSequence)))
	}
	if item.BytesWritten > 0 {
		parts = append(parts, fmt.Sprintf("%dB", item.BytesWritten))
	}
	if item.FailureReason != "" {
		parts = append(parts, item.FailureReason)
	}
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(parts, " ")
}

func artifactExpectedSequenceText(value int) string {
	if value <= 0 {
		return "-"
	}
	return fmt.Sprintf("%d", value)
}

func patchDisplayLabel(item backend.PatchInfo) string {
	parts := make([]string, 0, 5)
	if item.State != "" {
		parts = append(parts, string(item.State))
	}
	if item.TargetRef != "" {
		parts = append(parts, item.TargetRef)
	} else if item.ID != "" {
		parts = append(parts, string(item.ID))
	}
	if item.AddedLines != 0 || item.DeletedLines != 0 {
		parts = append(parts, fmt.Sprintf("+%d/-%d", item.AddedLines, item.DeletedLines))
	}
	if item.ApprovalID != "" {
		parts = append(parts, "approval:"+string(item.ApprovalID))
	} else if item.RevertApprovalID != "" {
		parts = append(parts, "approval:"+string(item.RevertApprovalID))
	}
	if item.Summary != "" {
		parts = append(parts, item.Summary)
	}
	return strings.Join(parts, "  ")
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

func repairLoopSummary(progress backend.TaskProgressInfo) string {
	loop := progress.RepairLoop
	parts := make([]string, 0, 4)
	if loop.AttemptCount > 0 {
		attempt := loop.LastAttempt
		if attempt <= 0 {
			attempt = loop.AttemptCount
		}
		parts = append(parts, fmt.Sprintf("repair #%d", attempt))
	}
	if loop.LastFailureClass != "" {
		parts = append(parts, loop.LastFailureClass)
	}
	if loop.LastFailureTargetScope != "" {
		parts = append(parts, loop.LastFailureTargetScope)
	}
	if loop.RepeatedFailureReason != "" {
		parts = append(parts, loop.RepeatedFailureReason)
	}
	if loop.NextAction != "" {
		parts = append(parts, loop.NextAction)
	}
	if loop.Escalated {
		parts = append(parts, "escalated")
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, " | ")
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
	if len(next.Context.Fields) > 0 {
		existing.Context = next.Context.Clone()
	}
	if next.State != "" {
		existing.State = next.State
	}
	if next.ResolutionType != "" {
		existing.ResolutionType = next.ResolutionType
	}
	if next.ContinuationAction != "" {
		existing.ContinuationAction = next.ContinuationAction
	}
	if next.ResumeRunID != "" {
		existing.ResumeRunID = next.ResumeRunID
	}
	if next.SuccessorRunID != "" {
		existing.SuccessorRunID = next.SuccessorRunID
	}
	if next.AutoGovernance != nil {
		existing.AutoGovernance = next.AutoGovernance
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
	if len(next.Context.Fields) > 0 {
		existing.Context = next.Context.Clone()
	}
	if next.State != "" {
		existing.State = next.State
	}
	if next.AutoGovernance != nil {
		existing.AutoGovernance = next.AutoGovernance
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

func payloadInt64(payload map[string]any, keys ...string) int64 {
	for _, key := range keys {
		switch value := payload[key].(type) {
		case int64:
			return value
		case int:
			return int64(value)
		case float64:
			return int64(value)
		case float32:
			return int64(value)
		}
	}
	return 0
}

func payloadUint64(payload map[string]any, keys ...string) uint64 {
	for _, key := range keys {
		switch value := payload[key].(type) {
		case uint64:
			return value
		case uint:
			return uint64(value)
		case int64:
			if value >= 0 {
				return uint64(value)
			}
		case int:
			if value >= 0 {
				return uint64(value)
			}
		case float64:
			if value >= 0 {
				return uint64(value)
			}
		}
	}
	return 0
}
