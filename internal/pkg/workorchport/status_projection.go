package workorchport

import (
	"github.com/muidea/workorch/internal/pkg/common"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func RunStatusInfoFromProjection(item projectioncontract.RunStatus) RunStatusInfo {
	return RunStatusInfo{
		ID: common.RunID(item.ID), WorkspaceID: common.WorkspaceID(item.WorkspaceID), State: common.RunState(item.State),
		CurrentStepID: common.StepID(item.CurrentStepID), CurrentStepKind: CanonicalRuntimeBlockKind(item.CurrentStepKind),
		CurrentStepState: common.StepState(item.CurrentStepState), CurrentActionLabel: item.CurrentActionLabel,
		DiagnosticLineage: common.StepID(item.DiagnosticLineage), StepCount: item.StepCount,
		DecisionCount: item.DecisionCount, ApprovalCount: item.ApprovalCount,
		BlockingTicketID: common.TicketID(item.BlockingTicketID), BlockingApprovalID: common.ApprovalID(item.BlockingApprovalID),
		FailureReason: item.FailureReason, ConvergenceStatus: ConvergenceStatusInfoFromProjection(item.ConvergenceStatus),
		SchedulerDiagnostics: projectioncontract.CloneSchedulerDiagnostics(item.SchedulerDiagnostics),
	}
}

func WorkspaceOutcomeInfoFromProjection(item projectioncontract.WorkspaceOutcome) WorkspaceOutcomeInfo {
	return WorkspaceOutcomeInfo{
		WorkspaceID:       common.WorkspaceID(item.WorkspaceID),
		State:             item.State,
		RawWorkspaceState: item.RawWorkspaceState,
		RunID:             common.RunID(item.RunID),
		ResultID:          common.ResultID(item.ResultID),
		Accepted:          item.Accepted,
		Terminal:          item.Terminal,
		Summary:           item.Summary,
		Reason:            item.Reason,
		EvidenceRefs:      append([]string(nil), item.EvidenceRefs...),
		ArtifactRefs:      append([]string(nil), item.ArtifactRefs...),
	}
}

func RunHeartbeatInfoFromProjection(item projectioncontract.RunHeartbeat) RunHeartbeatInfo {
	return RunHeartbeatInfo{
		WorkspaceID:               common.WorkspaceID(item.WorkspaceID),
		RunID:                     common.RunID(item.RunID),
		State:                     item.State,
		ActivityState:             item.ActivityState,
		LatestStepID:              common.StepID(item.LatestStepID),
		LatestStepKind:            item.LatestStepKind,
		LatestStepState:           item.LatestStepState,
		LatestEvidenceRef:         item.LatestEvidenceRef,
		LatestEventTopic:          item.LatestEventTopic,
		LatestEventRevision:       item.LatestEventRevision,
		LatestEventAt:             item.LatestEventAt,
		LatestLLMUsageAt:          item.LatestLLMUsageAt,
		LatestResultAt:            item.LatestResultAt,
		LatestArtifactAt:          item.LatestArtifactAt,
		LatestPatchAt:             item.LatestPatchAt,
		LatestEffectiveProgressAt: item.LatestEffectiveProgressAt,
		IneffectiveRetry:          item.IneffectiveRetry,
		Stalled:                   item.Stalled,
		Summary:                   item.Summary,
	}
}

func ConvergenceStatusInfoFromProjection(item *projectioncontract.ConvergenceStatus) *ConvergenceStatusInfo {
	if item == nil {
		return nil
	}
	return &ConvergenceStatusInfo{
		WorkspaceID:                   common.WorkspaceID(item.WorkspaceID),
		RunID:                         common.RunID(item.RunID),
		RunState:                      common.RunState(item.RunState),
		CurrentSliceID:                item.CurrentSliceID,
		CurrentFocusID:                item.CurrentFocusID,
		ConvergenceState:              item.ConvergenceState,
		SuggestedNextAction:           item.SuggestedNextAction,
		CurrentGap:                    item.CurrentGap,
		MissingEvidenceKinds:          append([]string(nil), item.MissingEvidenceKinds...),
		AcceptedEvidenceCount:         item.AcceptedEvidenceCount,
		RejectedEvidenceCount:         item.RejectedEvidenceCount,
		ClosedCoverageItemCount:       item.ClosedCoverageItemCount,
		RemainingCoverageItemCount:    item.RemainingCoverageItemCount,
		AssessmentDisposition:         item.AssessmentDisposition,
		RepairAttempts:                item.RepairAttempts,
		MaxRepairAttempts:             item.MaxRepairAttempts,
		RepeatedFailureSignatureCount: item.RepeatedFailureSignatureCount,
		LatestAcceptedEvidenceRef:     item.LatestAcceptedEvidenceRef,
		NextActivationRequired:        item.NextActivationRequired,
		GapSummary:                    item.GapSummary,
		LatestEffectiveProgressAt:     item.LatestEffectiveProgressAt,
		LastProgressAt:                item.LastProgressAt,
		DecisionCause:                 item.DecisionCause,
		DecisionEligibilityDenied:     item.DecisionEligibilityDenied,
		DecisionNotEligibleReason:     item.DecisionNotEligibleReason,
		DecisionNotEligibleStepID:     item.DecisionNotEligibleStepID,
		Stalled:                       item.Stalled,
		IneffectiveRetry:              item.IneffectiveRetry,
	}
}

func RunCommandStatusInfoFromProjection(item *projectioncontract.RunCommandStatus) *RunCommandStatusInfo {
	if item == nil {
		return nil
	}
	return &RunCommandStatusInfo{
		WorkspaceID:        common.WorkspaceID(item.WorkspaceID),
		RunID:              common.RunID(item.RunID),
		State:              item.State,
		Intent:             item.Intent,
		Reason:             item.Reason,
		Disposition:        item.Disposition,
		Diagnostic:         item.Diagnostic,
		RequestID:          item.RequestID,
		IdempotencyKey:     item.IdempotencyKey,
		Source:             item.Source,
		DefinitionRevision: item.DefinitionRevision,
		DecisionID:         common.TicketID(item.DecisionID),
		QueuedAt:           item.QueuedAt,
		AcceptedAt:         item.AcceptedAt,
		RejectedAt:         item.RejectedAt,
		Error:              item.Error,
		Revision:           item.Revision,
	}
}

func CloneRuntimeActivationStatus(item *projectioncontract.RuntimeActivationStatus) *projectioncontract.RuntimeActivationStatus {
	if item == nil {
		return nil
	}
	ret := *item
	return &ret
}
