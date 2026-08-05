package workorchport

import (
	"github.com/muidea/workorch/internal/pkg/common"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

// TaskProgressInfoFromProjection is the canonical product-boundary conversion
// for local and remote adapters. It preserves the projection as display-only
// data and never selects or triggers Runtime work.
func TaskProgressInfoFromProjection(item projectioncontract.TaskProgress) TaskProgressInfo {
	return TaskProgressInfo{
		WorkspaceID: common.WorkspaceID(item.WorkspaceID), RunID: common.RunID(item.RunID), State: item.State, Summary: item.Summary,
		CurrentStepID: common.StepID(item.CurrentStepID), CurrentStepKind: item.CurrentStepKind,
		CurrentStepState: item.CurrentStepState, CurrentActionLabel: item.CurrentActionLabel,
		DiagnosticLineage: common.StepID(item.DiagnosticLineage), CurrentMilestone: item.CurrentMilestone,
		BlockingTicketID: common.TicketID(item.BlockingTicketID), BlockingApprovalID: common.ApprovalID(item.BlockingApprovalID),
		Milestones: cloneProgressMilestones(item.Milestones), Items: cloneProgressItems(item.Items), Blockers: cloneProgressBlockers(item.Blockers),
		Recovery: RecoveryPlanInfoFromProjection(item.Recovery), RepairLoop: RepairLoopInfoFromProjection(item.RepairLoop),
		AssessmentOutcome: AssessmentOutcomeInfo{
			State: item.AssessmentOutcome.State, AcceptedSlices: item.AssessmentOutcome.AcceptedSlices,
			AcceptedResultCount:         item.AssessmentOutcome.AcceptedResultCount,
			RejectedGlobalAssessments:   item.AssessmentOutcome.RejectedGlobalAssessments,
			SatisfiedCoverageCount:      item.AssessmentOutcome.SatisfiedCoverageCount,
			RemainingCoverageCount:      item.AssessmentOutcome.RemainingCoverageCount,
			NewlySatisfiedCoverageCount: item.AssessmentOutcome.NewlySatisfiedCoverageCount,
			LatestAcceptedResultID:      common.ResultID(item.AssessmentOutcome.LatestAcceptedResultID),
			LatestRejectedResultID:      common.ResultID(item.AssessmentOutcome.LatestRejectedResultID),
			LatestRejectedReason:        item.AssessmentOutcome.LatestRejectedReason,
		},
		EvidenceRefs: cloneProjectionStrings(item.EvidenceRefs), Runtime: RuntimeProgressInfoFromProjection(item.Runtime),
	}
}

func RuntimeProgressInfoFromProjection(item *projectioncontract.RuntimeProgress) *RuntimeProgressInfo {
	if item == nil {
		return nil
	}
	return &RuntimeProgressInfo{
		StateRevision: item.StateRevision, EvidenceSequence: item.EvidenceSequence,
		CompletedDispatchCount: item.CompletedDispatchCount, ActiveDispatchID: item.ActiveDispatchID,
		CandidateAttemptCount: item.CandidateAttemptCount, AcceptedEvidenceCount: item.AcceptedEvidenceCount,
		AcceptedValidationEvidenceCount: item.AcceptedValidationEvidenceCount, RejectedEvidenceCount: item.RejectedEvidenceCount,
		MissingEvidenceKinds: cloneProjectionStrings(item.MissingEvidenceKinds), CurrentFocusID: item.CurrentFocusID,
		ConvergenceState: item.ConvergenceState, TerminalCode: item.TerminalCode, FailureClass: item.FailureClass,
		FailureCode: item.FailureCode, FailureSignature: item.FailureSignature, FailureMessage: item.FailureMessage,
		RepairHintCodes: cloneProjectionStrings(item.RepairHintCodes), PrerequisiteRefreshState: item.PrerequisiteRefreshState,
		PrerequisiteRefreshUsed: item.PrerequisiteRefreshUsed, PrerequisiteRefreshLimit: item.PrerequisiteRefreshLimit,
		PrerequisiteInvalidations: prerequisiteRefreshInvalidationsFromProjection(item.PrerequisiteInvalidations),
	}
}

func RecoveryPlanInfoFromProjection(item projectioncontract.RecoveryPlan) RecoveryPlanInfo {
	return RecoveryPlanInfo{
		Mode: item.Mode, NextAction: item.NextAction, RunID: common.RunID(item.RunID), StepID: common.StepID(item.StepID),
		BlockingTicketID: common.TicketID(item.BlockingTicketID), BlockingApprovalID: common.ApprovalID(item.BlockingApprovalID),
		CheckpointID: common.CheckpointID(item.CheckpointID), FailureClass: item.FailureClass,
		FailureSignature: item.FailureSignature, Reason: item.Reason,
	}
}

func RepairLoopInfoFromProjection(item projectioncontract.RepairLoop) RepairLoopInfo {
	return RepairLoopInfo{
		State: item.State, AttemptCount: item.AttemptCount, LastAttempt: item.LastAttempt,
		LastRepairStepID: common.StepID(item.LastRepairStepID), LastFailureStepID: common.StepID(item.LastFailureStepID),
		LastFailureClass: item.LastFailureClass, LastFailureSignature: item.LastFailureSignature,
		LastFailureTargetScope: item.LastFailureTargetScope, LastFailureAt: item.LastFailureAt,
		StructuredFailureSignals: projectionPayload(item.StructuredFailureSignals),
		RepairInputCompaction:    projectionPayload(item.RepairInputCompaction), LastRepairInputBytes: item.LastRepairInputBytes,
		RepeatedFailureReason: item.RepeatedFailureReason, MissingDoDItems: projectionPayloads(item.MissingDoDItems),
		RecentAttempts: projectionPayloads(item.RecentAttempts), RecommendedNextSteps: cloneProjectionStrings(item.RecommendedNextSteps),
		NextAction: item.NextAction, Escalated: item.Escalated, EscalationReason: item.EscalationReason,
		EscalationTicketID: common.TicketID(item.EscalationTicketID), EvidenceRefs: cloneProjectionStrings(item.EvidenceRefs),
	}
}

func prerequisiteRefreshInvalidationsFromProjection(items []projectioncontract.RuntimePrerequisiteRefreshInvalidation) []RuntimePrerequisiteRefreshInvalidationInfo {
	if len(items) == 0 {
		return nil
	}
	ret := make([]RuntimePrerequisiteRefreshInvalidationInfo, 0, len(items))
	for _, item := range items {
		changes := append([]projectioncontract.RuntimePrerequisiteResourceChange(nil), item.ResourceChanges...)
		ret = append(ret, RuntimePrerequisiteRefreshInvalidationInfo{
			Signature: item.Signature, Source: item.Source, PrerequisiteIDs: cloneProjectionStrings(item.PrerequisiteIDs),
			ObservationIDs: cloneProjectionStrings(item.ObservationIDs), DispatchID: item.DispatchID, OperationID: item.OperationID,
			ReceiptHash: item.ReceiptHash, OwnerModuleID: item.OwnerModuleID, ResourceDomain: item.ResourceDomain,
			SubjectRef: item.SubjectRef, ExpectedRevision: item.ExpectedRevision, ObservedRevision: item.ObservedRevision,
			Reason: item.Reason, ResourceChanges: changes,
		})
	}
	return ret
}

func cloneProgressMilestones(items []projectioncontract.ProgressMilestone) []projectioncontract.ProgressMilestone {
	if len(items) == 0 {
		return nil
	}
	ret := make([]projectioncontract.ProgressMilestone, 0, len(items))
	for _, item := range items {
		item.EvidenceRefs = cloneProjectionStrings(item.EvidenceRefs)
		ret = append(ret, item)
	}
	return ret
}

func cloneProgressItems(items []projectioncontract.ProgressItem) []projectioncontract.ProgressItem {
	if len(items) == 0 {
		return nil
	}
	ret := make([]projectioncontract.ProgressItem, 0, len(items))
	for _, item := range items {
		item.EvidenceRefs = cloneProjectionStrings(item.EvidenceRefs)
		ret = append(ret, item)
	}
	return ret
}

func cloneProgressBlockers(items []projectioncontract.ProgressBlocker) []projectioncontract.ProgressBlocker {
	return append([]projectioncontract.ProgressBlocker(nil), items...)
}
