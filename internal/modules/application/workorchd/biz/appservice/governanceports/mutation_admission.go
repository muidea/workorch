package governanceports

import (
	"context"
	"errors"
	"fmt"
	"strings"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerreadports"
	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

// admitDecisionMutation is a read-only application admission. It intersects
// the concrete Decision-owner resource with the Workspace owner's canonical
// active Run and the Run owner's exact blocker reference before any Decision
// mutation command is published.
func admitDecisionMutation(ctx context.Context, hub magicevent.Hub, ticket *decisionmodel.Ticket, scope common.WorkspaceID) error {
	if ticket == nil || ticket.ID == "" || ticket.WorkspaceID == "" || ticket.RunID == "" {
		return governanceMutationConflict("decision", "", "resource scope is incomplete")
	}
	if definitionDecisionUsesDedicatedAdmission(ticket) {
		return governanceMutationConflict("decision", string(ticket.ID), "definition admission and clarification require their dedicated owner contract")
	}
	if scope != "" && ticket.WorkspaceID != scope {
		return governanceMutationConflict("decision", string(ticket.ID), "workspace scope does not match")
	}
	if ticket.State == common.DecisionTicketStateResolved {
		// A response-loss retry is not a new state transition. Forward it to
		// Decision so the owner can compare the complete resolution identity and
		// payload and either return its exact result or reject the mismatch.
		return nil
	}
	if ticket.State != common.DecisionTicketStateOpen {
		return governanceMutationConflict("decision", string(ticket.ID), "owner resource is neither open nor an idempotent terminal candidate")
	}
	workspace, run, err := readCanonicalGovernanceRun(ctx, hub, ticket.WorkspaceID, ticket.RunID)
	if err != nil {
		return err
	}
	if workspace.ActiveRunID != ticket.RunID || run.State != common.RunStateBlockedOnDecision ||
		run.BlockingTicketID != ticket.ID || run.BlockingApprovalID != "" {
		return governanceMutationConflict("decision", string(ticket.ID), "resource is not the canonical active Run blocker")
	}
	return nil
}

// admitApprovalMutation is the Approval counterpart of
// admitDecisionMutation. It performs owner reads only and cannot transition
// Workspace, Run, Decision, or Approval state.
func admitApprovalMutation(ctx context.Context, hub magicevent.Hub, item *approvalmodel.Approval, requestedAction string, scope common.WorkspaceID) error {
	if item == nil || item.ID == "" || item.WorkspaceID == "" || item.RunID == "" {
		return governanceMutationConflict("approval", "", "resource scope is incomplete")
	}
	if scope != "" && item.WorkspaceID != scope {
		return governanceMutationConflict("approval", string(item.ID), "workspace scope does not match")
	}
	if (item.State == common.ApprovalStateApproved && requestedAction == "approve") ||
		(item.State == common.ApprovalStateRejected && requestedAction == "reject") {
		// Approval owns the exact action/principal/context comparison for
		// response-loss retries. Application admission must not reimplement it.
		return nil
	}
	if item.State == common.ApprovalStateApproved || item.State == common.ApprovalStateRejected {
		return governanceMutationConflict("approval", string(item.ID), "terminal owner state does not match the requested action")
	}
	if item.State != common.ApprovalStatePending {
		return governanceMutationConflict("approval", string(item.ID), "owner resource is neither pending nor an idempotent terminal candidate")
	}
	workspace, run, err := readCanonicalGovernanceRun(ctx, hub, item.WorkspaceID, item.RunID)
	if err != nil {
		return err
	}
	if workspace.ActiveRunID != item.RunID || run.State != common.RunStateBlockedOnApproval ||
		run.BlockingApprovalID != item.ID || run.BlockingTicketID != "" {
		return governanceMutationConflict("approval", string(item.ID), "resource is not the canonical active Run blocker")
	}
	return nil
}

func readCanonicalGovernanceRun(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID, runID common.RunID) (*workspaceGovernanceAdmission, *runGovernanceAdmission, error) {
	readCtx := common.ContextWithRuntimeWorkspaceID(ctx, workspaceID)
	workspace, err := ownerreadports.ReadWorkspaceByOwnerCommand(readCtx, hub, workspaceID)
	if err != nil {
		return nil, nil, governanceOwnerReadError("workspace", string(workspaceID), err)
	}
	if workspace == nil || workspace.ID != workspaceID || workspace.ActiveRunID != runID {
		return nil, nil, governanceMutationConflict("run", string(runID), "resource is not the Workspace owner's active Run")
	}
	run, err := ownerreadports.ReadRunByWorkspaceOwnerCommand(readCtx, hub, workspaceID, runID)
	if err != nil {
		return nil, nil, governanceOwnerReadError("run", string(runID), err)
	}
	if run == nil || run.ID != runID || run.WorkspaceID != workspaceID {
		return nil, nil, governanceMutationConflict("run", string(runID), "Run owner scope does not match")
	}
	return &workspaceGovernanceAdmission{ActiveRunID: workspace.ActiveRunID}, &runGovernanceAdmission{
		State:              run.State,
		BlockingTicketID:   run.BlockingTicketID,
		BlockingApprovalID: run.BlockingApprovalID,
	}, nil
}

// These bounded private values prevent the admission helper from retaining or
// returning owner aggregates after the two typed reads complete.
type workspaceGovernanceAdmission struct {
	ActiveRunID common.RunID
}

type runGovernanceAdmission struct {
	State              common.RunState
	BlockingTicketID   common.TicketID
	BlockingApprovalID common.ApprovalID
}

func definitionDecisionUsesDedicatedAdmission(ticket *decisionmodel.Ticket) bool {
	return ticket != nil && ticket.DefinitionClarification != nil
}

func governanceOwnerReadError(resourceKind, resourceID string, err error) error {
	if errors.Is(err, common.ErrNotFound) {
		return governanceMutationConflict(resourceKind, resourceID, "canonical owner resource is missing")
	}
	return err
}

func governanceMutationConflict(resourceKind, resourceID, reason string) error {
	resourceKind = strings.TrimSpace(resourceKind)
	resourceID = strings.TrimSpace(resourceID)
	reason = strings.TrimSpace(reason)
	if resourceID == "" {
		return fmt.Errorf("%w: %s governance mutation is not admitted: %s", common.ErrConflict, resourceKind, reason)
	}
	return fmt.Errorf("%w: %s %s governance mutation is not admitted: %s", common.ErrConflict, resourceKind, resourceID, reason)
}
