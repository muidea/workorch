package orchestrator

import (
	"context"
	"errors"

	approvalmodel "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/models"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func (o *RuntimeOrchestrator) NextActionForRun(ctx context.Context, runID common.RunID) (runtimemodel.NextAction, error) {
	if o == nil || runID == "" {
		return runtimemodel.NextAction{}, common.ErrInvalidArgument
	}
	run, err := o.runtimeEventRun(ctx, runID)
	if err != nil {
		return runtimemodel.NextAction{}, err
	}
	if run.State == common.RunStateBlockedOnApproval && run.BlockingApprovalID != "" && o.approvals != nil {
		scopedCtx := common.ContextWithRuntimeWorkspaceID(ctx, run.WorkspaceID)
		approval, err := o.approvals.Get(scopedCtx, run.BlockingApprovalID)
		if err != nil {
			return runtimemodel.NextAction{}, err
		}
		if action, ok := derivePendingApprovalNextAction(run, approval); ok {
			return action, nil
		}
	}
	if run.State == common.RunStateBlockedOnDecision && run.BlockingTicketID != "" && o.tickets != nil {
		scopedCtx := common.ContextWithRuntimeWorkspaceID(ctx, run.WorkspaceID)
		ticket, err := o.tickets.Get(scopedCtx, run.BlockingTicketID)
		if err != nil {
			return runtimemodel.NextAction{}, err
		}
		if action, ok := deriveOpenDecisionNextAction(run, ticket); ok {
			return action, nil
		}
	}
	if action, ok := deriveMissingGapBaselineNextAction(run); ok {
		return action, nil
	}
	if action, ok, err := o.missingStateChangeNextAction(ctx, run); err != nil {
		return runtimemodel.NextAction{}, err
	} else if ok {
		return action, nil
	}
	if action, ok, err := o.missingValidationNextAction(ctx, run); err != nil {
		return runtimemodel.NextAction{}, err
	} else if ok {
		return action, nil
	}
	return runtimemodel.NextAction{}, nil
}

func deriveMissingGapBaselineNextAction(run *runmodel.Run) (runtimemodel.NextAction, bool) {
	if run == nil || run.ID == "" || run.WorkspaceID == "" {
		return runtimemodel.NextAction{}, false
	}
	if !runStateAllowsGapAssessment(run.State) {
		return runtimemodel.NextAction{}, false
	}
	if len(mapFromValue(run.ExecutionContext[gapBaselineKey])) > 0 {
		return runtimemodel.NextAction{}, false
	}
	return runtimemodel.NextAction{
		WorkspaceID: run.WorkspaceID,
		RunID:       run.ID,
		Type:        runtimemodel.NextActionTypeAssessGap,
		SubjectType: "run",
		SubjectID:   string(run.ID),
		Stage:       "gap_analysis",
		StageMode:   "baseline",
		RequiredOutput: map[string]any{
			"gapBaselineRequired": true,
		},
		AllowedActions:   []string{runtimemodel.NextActionTypeAssessGap},
		ForbiddenActions: []string{runtimemodel.NextActionTypeStateChange},
		Reason:           "gap baseline missing",
	}.Normalized(), true
}

func runStateAllowsGapAssessment(state common.RunState) bool {
	switch state {
	case common.RunStateCreated, common.RunStateRunning, common.RunStateValidating:
		return true
	default:
		return false
	}
}

func (o *RuntimeOrchestrator) missingStateChangeNextAction(ctx context.Context, run *runmodel.Run) (runtimemodel.NextAction, bool, error) {
	return o.missingCoverageEvidenceNextAction(ctx, run, missingCoverageEvidenceNextActionSpec{
		ActionType:   runtimemodel.NextActionTypeStateChange,
		EvidenceKind: runtimemodel.EvidenceKindStateChange,
		ForbiddenActions: []string{
			runtimemodel.NextActionTypeComplete,
			runtimemodel.NextActionTypeValidate,
			runtimemodel.NextActionTypeAssessment,
		},
		Reason: "missing state_change evidence",
	})
}

func (o *RuntimeOrchestrator) missingValidationNextAction(ctx context.Context, run *runmodel.Run) (runtimemodel.NextAction, bool, error) {
	return o.missingCoverageEvidenceNextAction(ctx, run, missingCoverageEvidenceNextActionSpec{
		ActionType:   runtimemodel.NextActionTypeValidate,
		EvidenceKind: runtimemodel.EvidenceKindValidation,
		ForbiddenActions: []string{
			runtimemodel.NextActionTypeComplete,
			runtimemodel.NextActionTypeAssessment,
			runtimemodel.NextActionTypeStateChange,
		},
		Reason: "missing validation evidence",
	})
}

type missingCoverageEvidenceNextActionSpec struct {
	ActionType       string
	EvidenceKind     string
	ForbiddenActions []string
	Reason           string
}

func (o *RuntimeOrchestrator) missingCoverageEvidenceNextAction(ctx context.Context, run *runmodel.Run, spec missingCoverageEvidenceNextActionSpec) (runtimemodel.NextAction, bool, error) {
	if run == nil || !runStateAllowsGapAssessment(run.State) {
		return runtimemodel.NextAction{}, false, nil
	}
	if len(mapFromValue(run.ExecutionContext[gapBaselineKey])) == 0 {
		return runtimemodel.NextAction{}, false, nil
	}
	items, err := o.coverageItemsForNextAction(ctx, run)
	if err != nil {
		return runtimemodel.NextAction{}, false, err
	}
	action, ok := deriveMissingCoverageEvidenceNextAction(run, items, spec)
	return action, ok, nil
}

func (o *RuntimeOrchestrator) coverageItemsForNextAction(ctx context.Context, run *runmodel.Run) ([]map[string]any, error) {
	if run == nil || run.ID == "" {
		return nil, nil
	}
	scopedCtx := coverageRepositoryContext(run, ctx)
	if o != nil && o.coverageLedger() != nil {
		if _, err := o.coverageLedger().hydrateProjectionFromRepository(scopedCtx, run); err != nil {
			return nil, err
		}
	}
	if items := mapSliceFromValue(run.ExecutionContext[coverageWorkItemsKey]); len(items) > 0 {
		return items, nil
	}
	if o == nil || o.evidence.CoverageWorkItems == nil {
		return nil, nil
	}
	items, err := o.evidence.CoverageWorkItems.ListActiveByRun(scopedCtx, run.ID)
	if err != nil {
		if errors.Is(err, common.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return coverageWorkItemProjectionFromModels(items), nil
}

func deriveMissingStateChangeNextAction(run *runmodel.Run, items []map[string]any) (runtimemodel.NextAction, bool) {
	return deriveMissingCoverageEvidenceNextAction(run, items, missingCoverageEvidenceNextActionSpec{
		ActionType:   runtimemodel.NextActionTypeStateChange,
		EvidenceKind: runtimemodel.EvidenceKindStateChange,
		ForbiddenActions: []string{
			runtimemodel.NextActionTypeComplete,
			runtimemodel.NextActionTypeValidate,
			runtimemodel.NextActionTypeAssessment,
		},
		Reason: "missing state_change evidence",
	})
}

func deriveMissingValidationNextAction(run *runmodel.Run, items []map[string]any) (runtimemodel.NextAction, bool) {
	return deriveMissingCoverageEvidenceNextAction(run, items, missingCoverageEvidenceNextActionSpec{
		ActionType:   runtimemodel.NextActionTypeValidate,
		EvidenceKind: runtimemodel.EvidenceKindValidation,
		ForbiddenActions: []string{
			runtimemodel.NextActionTypeComplete,
			runtimemodel.NextActionTypeAssessment,
			runtimemodel.NextActionTypeStateChange,
		},
		Reason: "missing validation evidence",
	})
}

func deriveMissingCoverageEvidenceNextAction(run *runmodel.Run, items []map[string]any, spec missingCoverageEvidenceNextActionSpec) (runtimemodel.NextAction, bool) {
	if run == nil || run.ID == "" || run.WorkspaceID == "" || !runStateAllowsGapAssessment(run.State) {
		return runtimemodel.NextAction{}, false
	}
	for _, item := range items {
		if !runtimemodel.CoverageWorkItemStatusActive(stringFromMap(item, "status")) {
			continue
		}
		state := deriveCoverageWorkItemState(item)
		if !stringSliceContains(state.MissingEvidenceKinds, spec.EvidenceKind) {
			continue
		}
		itemID := common.CoverageWorkItemID(firstNonEmptyString(stringFromMap(item, "id"), stringFromMap(item, "repositoryId")))
		if itemID == "" {
			continue
		}
		return runtimemodel.NextAction{
			WorkspaceID:          run.WorkspaceID,
			RunID:                run.ID,
			Type:                 spec.ActionType,
			SubjectType:          "coverage",
			SubjectID:            string(itemID),
			FocusCoverageItemIDs: []common.CoverageWorkItemID{itemID},
			RequiredOutput: map[string]any{
				"coverageWorkItemIds":   []string{string(itemID)},
				"requiredEvidenceKinds": []string{spec.EvidenceKind},
				"mustBindCoverageFocus": true,
			},
			AllowedActions:        []string{spec.ActionType},
			ForbiddenActions:      append([]string(nil), spec.ForbiddenActions...),
			RequiredEvidenceKinds: []string{spec.EvidenceKind},
			Stage:                 spec.ActionType,
			StageMode:             "evidence",
			Reason:                spec.Reason,
		}.Normalized(), true
	}
	return runtimemodel.NextAction{}, false
}

func deriveOpenDecisionNextAction(run *runmodel.Run, ticket *decisionmodel.Ticket) (runtimemodel.NextAction, bool) {
	if run == nil || ticket == nil {
		return runtimemodel.NextAction{}, false
	}
	if run.ID == "" || run.WorkspaceID == "" || run.State != common.RunStateBlockedOnDecision || run.BlockingTicketID == "" {
		return runtimemodel.NextAction{}, false
	}
	if ticket.ID != run.BlockingTicketID || ticket.WorkspaceID != run.WorkspaceID || ticket.RunID != run.ID || ticket.State != common.DecisionTicketStateOpen {
		return runtimemodel.NextAction{}, false
	}
	return runtimemodel.NextAction{
		WorkspaceID: run.WorkspaceID,
		RunID:       run.ID,
		Type:        runtimemodel.NextActionTypeRequestDecision,
		SubjectType: "decision",
		SubjectID:   string(ticket.ID),
		Stage:       "decision",
		StageMode:   "governance",
		RequiredOutput: map[string]any{
			"ticketId": string(ticket.ID),
		},
		AllowedActions:   []string{"resolve_decision"},
		ForbiddenActions: []string{runtimemodel.NextActionTypeStateChange},
		Reason:           "open decision blocks run",
	}.Normalized(), true
}

func derivePendingApprovalNextAction(run *runmodel.Run, approval *approvalmodel.Approval) (runtimemodel.NextAction, bool) {
	if run == nil || approval == nil {
		return runtimemodel.NextAction{}, false
	}
	if run.ID == "" || run.WorkspaceID == "" || run.State != common.RunStateBlockedOnApproval || run.BlockingApprovalID == "" {
		return runtimemodel.NextAction{}, false
	}
	if approval.ID != run.BlockingApprovalID || approval.WorkspaceID != run.WorkspaceID || approval.RunID != run.ID || approval.State != common.ApprovalStatePending {
		return runtimemodel.NextAction{}, false
	}
	return runtimemodel.NextAction{
		WorkspaceID: run.WorkspaceID,
		RunID:       run.ID,
		Type:        runtimemodel.NextActionTypeRequestApproval,
		SubjectType: "approval",
		SubjectID:   string(approval.ID),
		Stage:       "approval",
		StageMode:   "governance",
		RequiredOutput: map[string]any{
			"approvalId": string(approval.ID),
			"action":     approval.Action,
		},
		AllowedActions:   []string{"approve", "reject"},
		ForbiddenActions: []string{runtimemodel.NextActionTypeStateChange},
		Reason:           "pending approval blocks run",
	}.Normalized(), true
}
