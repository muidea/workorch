package tui

import (
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

type governanceCommandKind string

const (
	governanceCommandDecision governanceCommandKind = "decision"
	governanceCommandApproval governanceCommandKind = "approval"
)

// governanceCommandTechnicalState is a UI-only delivery latch. It prevents a
// duplicate command while the owner call or its canonical projection readback
// is pending. It never substitutes a Decision, Approval, Run, or Workspace
// lifecycle state.
type governanceCommandTechnicalState struct {
	Kind            governanceCommandKind
	RunID           common.RunID
	ResourceID      string
	CommandInFlight bool
	RefreshPending  bool
}

func (s governanceCommandTechnicalState) pending() bool {
	return s.CommandInFlight || s.RefreshPending
}

func (m *AppModel) beginGovernanceCommand(kind governanceCommandKind, workspaceID common.WorkspaceID, runID common.RunID, resourceID string) bool {
	resourceID = strings.TrimSpace(resourceID)
	if m == nil || kind == "" || workspaceID == "" || runID == "" || resourceID == "" {
		return false
	}
	entry := m.ensureWorkspaceRuntimeState(string(workspaceID))
	if entry == nil {
		return false
	}
	if entry.GovernanceCommand.pending() {
		m.progress = progressSubmitting
		m.progressText = governanceCommandWaitText(entry.GovernanceCommand.Kind)
		m.lastError = nil
		return false
	}
	entry.GovernanceCommand = governanceCommandTechnicalState{
		Kind:            kind,
		RunID:           runID,
		ResourceID:      resourceID,
		CommandInFlight: true,
	}
	m.progress = progressSubmitting
	m.progressText = governanceCommandSubmitText(kind)
	return true
}

func (m *AppModel) markGovernanceCommandRefreshPending(kind governanceCommandKind, workspaceID common.WorkspaceID) bool {
	if m == nil || workspaceID == "" {
		return false
	}
	entry := m.workspaceRuntimeState(string(workspaceID))
	if entry == nil || !entry.GovernanceCommand.pending() || entry.GovernanceCommand.Kind != kind {
		return false
	}
	entry.GovernanceCommand.CommandInFlight = false
	entry.GovernanceCommand.RefreshPending = true
	if string(workspaceID) == m.activeWorkspaceID {
		m.progress = progressSubmitting
		m.progressText = governanceCommandWaitText(kind)
	}
	// A projection can overtake the command response. In that ordering, settle
	// against the already-applied canonical readback instead of waiting for an
	// additional notification.
	m.settleGovernanceCommandFromProjection(string(workspaceID))
	return entry.GovernanceCommand.RefreshPending
}

func (m *AppModel) clearGovernanceCommandAfterError(kind governanceCommandKind, workspaceID common.WorkspaceID) {
	if m == nil || workspaceID == "" {
		return
	}
	entry := m.workspaceRuntimeState(string(workspaceID))
	if entry == nil || entry.GovernanceCommand.Kind != kind {
		return
	}
	entry.GovernanceCommand = governanceCommandTechnicalState{}
	if string(workspaceID) != m.activeWorkspaceID {
		return
	}
	switch kind {
	case governanceCommandDecision:
		m.progress = progressWaitingDecision
		m.progressText = governanceCommandErrorWaitText(kind, blockingDecisionLabel(m.activeBlockingDecision()))
	case governanceCommandApproval:
		m.progress = progressWaitingApproval
		m.progressText = governanceCommandErrorWaitText(kind, blockingApprovalLabel(m.activeBlockingApproval()))
	}
}

func (m *AppModel) settleGovernanceCommandFromProjection(workspaceID string) {
	if m == nil || strings.TrimSpace(workspaceID) == "" {
		return
	}
	entry := m.workspaceRuntimeState(workspaceID)
	if entry == nil || !entry.GovernanceCommand.RefreshPending {
		return
	}
	pending := entry.GovernanceCommand
	if workspaceID != m.activeWorkspaceID {
		// A non-active workspace keeps its technical latch until it is selected
		// and a scoped canonical projection is applied to the main model.
		return
	}
	if strings.TrimSpace(m.activeRunID) != strings.TrimSpace(string(pending.RunID)) {
		entry.GovernanceCommand = governanceCommandTechnicalState{}
		return
	}
	switch pending.Kind {
	case governanceCommandDecision:
		if strings.TrimSpace(m.canonicalBlockingTicketID) != pending.ResourceID ||
			!decisionProjectionContainsOpenTarget(m.decisions, workspaceID, pending.RunID, pending.ResourceID) {
			entry.GovernanceCommand = governanceCommandTechnicalState{}
		}
	case governanceCommandApproval:
		if strings.TrimSpace(m.canonicalBlockingApprovalID) != pending.ResourceID ||
			!approvalProjectionContainsPendingTarget(m.approvals, workspaceID, pending.RunID, pending.ResourceID) {
			entry.GovernanceCommand = governanceCommandTechnicalState{}
		}
	}
}

func decisionProjectionContainsOpenTarget(items []projectioncontract.Decision, workspaceID string, runID common.RunID, resourceID string) bool {
	for _, item := range items {
		if strings.TrimSpace(item.ID) == resourceID &&
			strings.TrimSpace(item.WorkspaceID) == strings.TrimSpace(workspaceID) &&
			strings.TrimSpace(item.RunID) == strings.TrimSpace(string(runID)) &&
			common.DecisionTicketState(strings.TrimSpace(item.State)) == common.DecisionTicketStateOpen {
			return true
		}
	}
	return false
}

func approvalProjectionContainsPendingTarget(items []projectioncontract.Approval, workspaceID string, runID common.RunID, resourceID string) bool {
	for _, item := range items {
		if strings.TrimSpace(item.ID) == resourceID &&
			strings.TrimSpace(item.WorkspaceID) == strings.TrimSpace(workspaceID) &&
			strings.TrimSpace(item.RunID) == strings.TrimSpace(string(runID)) &&
			common.ApprovalState(strings.TrimSpace(item.State)) == common.ApprovalStatePending {
			return true
		}
	}
	return false
}

func governanceCommandSubmitText(kind governanceCommandKind) string {
	if kind == governanceCommandApproval {
		return "Submitting approval command"
	}
	return "Submitting decision command"
}

func governanceCommandWaitText(kind governanceCommandKind) string {
	if kind == governanceCommandApproval {
		return "Approval submitted; waiting for owner projection readback"
	}
	return "Decision submitted; waiting for owner projection readback"
}

func governanceCommandErrorWaitText(kind governanceCommandKind, blocker string) string {
	if strings.TrimSpace(blocker) != "" {
		return blocker
	}
	if kind == governanceCommandApproval {
		return "Approval command failed; waiting for owner projection"
	}
	return "Decision command failed; waiting for owner projection"
}
