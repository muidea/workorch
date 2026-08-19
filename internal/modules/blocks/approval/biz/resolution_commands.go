package approval

import (
	"context"
	"errors"
	"strings"

	magicevent "github.com/muidea/magicCommon/event"
	approvalcontracts "github.com/muidea/workorch/internal/modules/blocks/approval/biz/contracts"
	approvalevents "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"
	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func (m *Biz) executeEnsureApprovedCommand(ctx context.Context, ev magicevent.Event) (approvalevents.EnsureApprovedResult, error) {
	cmd, ok := ev.Data().(approvalevents.EnsureApprovedCommand)
	if !ok {
		return approvalevents.EnsureApprovedResult{}, common.ErrInvalidArgument
	}
	item, err := m.ensureApprovalResolution(ctx, ev, approvalevents.ResolveApprovalCommand(cmd), common.ApprovalStateApproved)
	return approvalevents.EnsureApprovedResult{Approval: approvalmodel.CloneApproval(item)}, err
}

func (m *Biz) executeEnsureRejectedCommand(ctx context.Context, ev magicevent.Event) (approvalevents.EnsureRejectedResult, error) {
	cmd, ok := ev.Data().(approvalevents.EnsureRejectedCommand)
	if !ok {
		return approvalevents.EnsureRejectedResult{}, common.ErrInvalidArgument
	}
	item, err := m.ensureApprovalResolution(ctx, ev, approvalevents.ResolveApprovalCommand(cmd), common.ApprovalStateRejected)
	return approvalevents.EnsureRejectedResult{Approval: approvalmodel.CloneApproval(item)}, err
}

func (m *Biz) ensureApprovalResolution(ctx context.Context, ev magicevent.Event, cmd approvalevents.ResolveApprovalCommand, target common.ApprovalState) (*approvalmodel.Approval, error) {
	if cmd.ApprovalID == "" || cmd.WorkspaceID == "" || cmd.RunID == "" || validateApprovalResolutionSource(ev, cmd.Principal) != nil {
		return nil, common.ErrInvalidArgument
	}
	contract := resolveApprovalContract(cmd)
	item, proposal, err := m.service.EnsureResolutionProposal(ctx, contract, target)
	if err != nil {
		return nil, err
	}
	if item.State == common.ApprovalStatePending && item.RuntimeGovernance != nil {
		commit, commitErr := m.commitRuntimeApprovalProposal(ctx, item, proposal)
		if commitErr != nil {
			return nil, commitErr
		}
		switch commit.Disposition {
		case orchestratorevents.RuntimeGovernanceResolutionDispositionCommitted, orchestratorevents.RuntimeGovernanceResolutionDispositionDuplicate:
		case orchestratorevents.RuntimeGovernanceResolutionDispositionStale:
			_ = m.service.RejectResolutionProposal(ctx, item, "aggregate_commit_stale", "runtime approval proposal belongs to a stale aggregate authority")
			return nil, common.ErrConflict
		default:
			return nil, common.ErrConflict
		}
	}
	item, _, err = m.service.FinalizeResolutionProposal(ctx, contract, target)
	if err != nil {
		return nil, err
	}
	if err := m.deliverResolvedApproval(ctx, item); err != nil {
		return nil, err
	}
	return m.approvals.GetScoped(ctx, item.WorkspaceID, item.RunID, item.ID)
}

func (m *Biz) commitRuntimeApprovalProposal(ctx context.Context, item *approvalmodel.Approval, proposal *approvalmodel.ResolutionProposal) (orchestratorevents.CommitRuntimeGovernanceResolutionResult, error) {
	if m == nil || m.EventHub() == nil || item == nil || item.RuntimeGovernance == nil || proposal == nil {
		return orchestratorevents.CommitRuntimeGovernanceResolutionResult{}, common.ErrInvalidArgument
	}
	outcome := runtimecontract.RuntimeGovernanceResolutionOutcomeContinue
	if proposal.TargetState == common.ApprovalStateRejected {
		outcome = runtimecontract.RuntimeGovernanceResolutionOutcomeFail
	}
	event := magicevent.NewEventWithContext(
		orchestratorevents.TopicOrchestratorCommitRuntimeGovernanceApproval, ModuleID, orchestratorevents.CommandDestination,
		magicevent.NewHeader(), ctx, orchestratorevents.CommitRuntimeGovernanceApprovalResolutionCommand{
			ResolutionID: proposal.ResolutionID, ApprovalID: item.ID, WorkspaceID: item.WorkspaceID, RunID: item.RunID,
			Outcome: outcome, Summary: approvalResolutionSummary(item, proposal),
		},
	)
	result := m.SendEvent(event)
	if result == nil {
		return orchestratorevents.CommitRuntimeGovernanceResolutionResult{}, common.ErrInvalidArgument
	}
	if err := result.Error(); err != nil {
		if err.Cause != nil {
			return orchestratorevents.CommitRuntimeGovernanceResolutionResult{}, err.Cause
		}
		return orchestratorevents.CommitRuntimeGovernanceResolutionResult{}, err
	}
	data, resultErr := result.Get()
	if resultErr != nil {
		if resultErr.Cause != nil {
			return orchestratorevents.CommitRuntimeGovernanceResolutionResult{}, resultErr.Cause
		}
		return orchestratorevents.CommitRuntimeGovernanceResolutionResult{}, resultErr
	}
	response, ok := data.(orchestratorevents.CommitRuntimeGovernanceResolutionResult)
	if !ok || response.WorkspaceID != item.WorkspaceID || response.RunID != item.RunID || response.ResolutionID != proposal.ResolutionID || response.Validate() != nil {
		return orchestratorevents.CommitRuntimeGovernanceResolutionResult{}, common.ErrInvalidArgument
	}
	return response, nil
}

func approvalResolutionSummary(item *approvalmodel.Approval, proposal *approvalmodel.ResolutionProposal) string {
	if proposal != nil {
		for _, key := range []string{"summary", "reason", "answer"} {
			if value, ok := proposal.Context.String(key); ok && strings.TrimSpace(value) != "" {
				return strings.TrimSpace(value)
			}
		}
	}
	if item != nil && strings.TrimSpace(item.Reason) != "" {
		return strings.TrimSpace(item.Reason)
	}
	return "approval resolution committed"
}

func resolveApprovalContract(cmd approvalevents.ResolveApprovalCommand) approvalcontracts.ResolveApprovalCommand {
	return approvalcontracts.ResolveApprovalCommand{
		ApprovalID: cmd.ApprovalID, WorkspaceID: cmd.WorkspaceID, RunID: cmd.RunID,
		Principal: cmd.Principal.Normalized(), Context: workorcheventpayload.New(cmd.ResolutionContext...),
	}
}

func (m *Biz) executePurgeWorkspaceCommand(ctx context.Context, ev magicevent.Event) (approvalevents.PurgeWorkspaceApprovalsResult, error) {
	cmd, ok := ev.Data().(approvalevents.PurgeWorkspaceApprovalsCommand)
	if !ok || cmd.WorkspaceID == "" || ev.Source() != approvalApplicationSource {
		return approvalevents.PurgeWorkspaceApprovalsResult{}, common.ErrInvalidArgument
	}
	items, err := m.approvals.ListByWorkspaceID(ctx, cmd.WorkspaceID)
	if err != nil {
		return approvalevents.PurgeWorkspaceApprovalsResult{}, err
	}
	deleted := 0
	for _, item := range items {
		if item == nil {
			continue
		}
		if item.DeletionRequestedAt == "" {
			item, err = m.service.applyLifecycleInternalEvent(ctx, item, lifecycleInternalEvent{ApprovalID: item.ID, WorkspaceID: item.WorkspaceID, RunID: item.RunID, Kind: lifecycleInternalEventDeletionRequested})
			if err != nil && !errors.Is(err, common.ErrConflict) {
				return approvalevents.PurgeWorkspaceApprovalsResult{Deleted: deleted}, err
			}
		}
		if err := m.approvals.DeleteScoped(ctx, item.WorkspaceID, item.RunID, item.ID); err != nil && !errors.Is(err, common.ErrNotFound) {
			return approvalevents.PurgeWorkspaceApprovalsResult{Deleted: deleted}, err
		}
		deleted++
	}
	return approvalevents.PurgeWorkspaceApprovalsResult{Deleted: deleted}, nil
}
