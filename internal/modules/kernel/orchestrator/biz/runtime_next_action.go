package orchestrator

import (
	"context"
	"errors"
	"strings"

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
	if action, ok, err := o.recoverableDiagnosticNextAction(ctx, run); err != nil {
		return runtimemodel.NextAction{}, err
	} else if ok {
		return action, nil
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
	if action, ok, err := o.allSatisfiedCompleteNextAction(ctx, run); err != nil {
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

func (o *RuntimeOrchestrator) recoverableDiagnosticNextAction(ctx context.Context, run *runmodel.Run) (runtimemodel.NextAction, bool, error) {
	if run == nil || !runStateAllowsGapAssessment(run.State) {
		return runtimemodel.NextAction{}, false, nil
	}
	if o == nil || o.evidence.EvidenceRecords == nil {
		return runtimemodel.NextAction{}, false, nil
	}
	records, err := o.evidence.EvidenceRecords.ListByRun(common.ContextWithRuntimeWorkspaceID(ctx, run.WorkspaceID), run.ID)
	if err != nil {
		if errors.Is(err, common.ErrNotFound) {
			return runtimemodel.NextAction{}, false, nil
		}
		return runtimemodel.NextAction{}, false, err
	}
	action, ok := deriveRecoverableDiagnosticNextAction(run, records)
	return action, ok, nil
}

func deriveRecoverableDiagnosticNextAction(run *runmodel.Run, records []*runtimemodel.EvidenceRecord) (runtimemodel.NextAction, bool) {
	if run == nil || run.ID == "" || run.WorkspaceID == "" || !runStateAllowsGapAssessment(run.State) {
		return runtimemodel.NextAction{}, false
	}
	superseded := supersededEvidenceRecordSet(records)
	for idx := len(records) - 1; idx >= 0; idx-- {
		record := records[idx]
		if record == nil ||
			record.WorkspaceID != run.WorkspaceID ||
			record.RunID != run.ID ||
			record.Kind != runtimemodel.EvidenceKindDiagnostic ||
			record.Status != runtimemodel.EvidenceStatusRejected ||
			superseded[record.ID] {
			continue
		}
		recoverability := recoverabilityFromDiagnosticEvidence(record)
		if !contractViolationRecoverabilityCanRetry(recoverability) {
			continue
		}
		required := map[string]any{
			"diagnosticEvidenceId":           string(record.ID),
			"diagnosticSourceRef":            record.SourceRef,
			"recoverability":                 recoverability,
			"mustUseRejectedDiagnostic":      true,
			"mustNotPromoteRejectedEvidence": true,
		}
		if failureClass := strings.TrimSpace(stringFromMap(record.Metadata, "failureClass")); failureClass != "" {
			required["failureClass"] = failureClass
		}
		if failureSignature := strings.TrimSpace(stringFromMap(record.Metadata, "failureSignature")); failureSignature != "" {
			required["failureSignature"] = failureSignature
		}
		return runtimemodel.NextAction{
			WorkspaceID:    run.WorkspaceID,
			RunID:          run.ID,
			Type:           runtimemodel.NextActionTypeRepair,
			SubjectType:    "diagnostic",
			SubjectID:      string(record.ID),
			RequiredOutput: required,
			AllowedActions: []string{runtimemodel.NextActionTypeRepair},
			ForbiddenActions: []string{
				runtimemodel.NextActionTypeRequestDecision,
				runtimemodel.NextActionTypeComplete,
			},
			RequiredEvidenceKinds: []string{runtimemodel.EvidenceKindDiagnostic},
			Stage:                 runtimemodel.NextActionTypeRepair,
			StageMode:             "diagnostic_retry",
			Reason:                "recoverable rejected diagnostic",
		}.Normalized(), true
	}
	return runtimemodel.NextAction{}, false
}

func supersededEvidenceRecordSet(records []*runtimemodel.EvidenceRecord) map[common.EvidenceRecordID]bool {
	ret := map[common.EvidenceRecordID]bool{}
	for _, record := range records {
		if record == nil {
			continue
		}
		for _, id := range record.Supersedes {
			if id != "" {
				ret[id] = true
			}
		}
	}
	return ret
}

func recoverabilityFromDiagnosticEvidence(record *runtimemodel.EvidenceRecord) string {
	if record == nil {
		return ""
	}
	if value := strings.TrimSpace(stringFromMap(record.Metadata, "recoverability")); value != "" {
		return value
	}
	return strings.TrimSpace(stringFromMap(mapFromValue(record.Metadata["contractViolation"]), "recoverability"))
}

func contractViolationRecoverabilityCanRetry(value string) bool {
	switch strings.TrimSpace(value) {
	case runtimemodel.ContractViolationRecoverabilityRetrySameContract,
		runtimemodel.ContractViolationRecoverabilityNarrowContract,
		runtimemodel.ContractViolationRecoverabilityNormalizeThenRetry:
		return true
	default:
		return false
	}
}

func (o *RuntimeOrchestrator) allSatisfiedCompleteNextAction(ctx context.Context, run *runmodel.Run) (runtimemodel.NextAction, bool, error) {
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
	action, ok := deriveAllSatisfiedCompleteNextAction(run, items)
	return action, ok, nil
}

func deriveAllSatisfiedCompleteNextAction(run *runmodel.Run, items []map[string]any) (runtimemodel.NextAction, bool) {
	if run == nil || run.ID == "" || run.WorkspaceID == "" || !runStateAllowsGapAssessment(run.State) || len(items) == 0 {
		return runtimemodel.NextAction{}, false
	}
	coverageIDs := make([]common.CoverageWorkItemID, 0, len(items))
	requiredKinds := make([]string, 0)
	for _, item := range items {
		itemID := common.CoverageWorkItemID(firstNonEmptyString(stringFromMap(item, "id"), stringFromMap(item, "repositoryId")))
		if itemID == "" || !coverageWorkItemCompletionStatusSatisfied(item) || !coverageWorkItemHasAcceptedEvidence(item) {
			return runtimemodel.NextAction{}, false
		}
		state := deriveCoverageWorkItemState(item)
		if len(state.MissingEvidenceKinds) > 0 || state.NeedsAssessment() {
			return runtimemodel.NextAction{}, false
		}
		coverageIDs = append(coverageIDs, itemID)
		requiredKinds = append(requiredKinds, state.RequiredEvidenceKinds...)
	}
	coverageIDStrings := make([]string, 0, len(coverageIDs))
	for _, id := range coverageIDs {
		coverageIDStrings = append(coverageIDStrings, string(id))
	}
	return runtimemodel.NextAction{
		WorkspaceID:          run.WorkspaceID,
		RunID:                run.ID,
		Type:                 runtimemodel.NextActionTypeComplete,
		SubjectType:          "run",
		SubjectID:            string(run.ID),
		FocusCoverageItemIDs: coverageIDs,
		RequiredOutput: map[string]any{
			"completionReady":     true,
			"coverageWorkItemIds": coverageIDStrings,
			"mustFreezeEvidence":  true,
		},
		AllowedActions: []string{runtimemodel.NextActionTypeComplete},
		ForbiddenActions: []string{
			runtimemodel.NextActionTypeStateChange,
			runtimemodel.NextActionTypeValidate,
			runtimemodel.NextActionTypeRepair,
			runtimemodel.NextActionTypeRequestDecision,
			runtimemodel.NextActionTypeRequestApproval,
		},
		RequiredEvidenceKinds: compactUniqueStrings(requiredKinds),
		Stage:                 runtimemodel.NextActionTypeComplete,
		StageMode:             "accepted_evidence",
		Reason:                "all coverage work items satisfied",
	}.Normalized(), true
}

func coverageWorkItemCompletionStatusSatisfied(item map[string]any) bool {
	switch strings.TrimSpace(stringFromMap(item, "status")) {
	case "satisfied", "closed", "completed":
		return true
	default:
		return false
	}
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
