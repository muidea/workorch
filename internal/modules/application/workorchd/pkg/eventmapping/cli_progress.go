package eventmapping

import (
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func repairLoopDTO(item projectioncontract.RepairLoop) (workorchdevents.CLIRepairLoopInfo, error) {
	signals, err := workorcheventpayload.FromMap(item.StructuredFailureSignals)
	if err != nil {
		return workorchdevents.CLIRepairLoopInfo{}, err
	}
	compaction, err := workorcheventpayload.FromMap(item.RepairInputCompaction)
	if err != nil {
		return workorchdevents.CLIRepairLoopInfo{}, err
	}
	missing, err := workorcheventpayload.FromMapSlice(item.MissingDoDItems)
	if err != nil {
		return workorchdevents.CLIRepairLoopInfo{}, err
	}
	attempts, err := workorcheventpayload.FromMapSlice(item.RecentAttempts)
	if err != nil {
		return workorchdevents.CLIRepairLoopInfo{}, err
	}
	return workorchdevents.CLIRepairLoopInfo{
		State: item.State, AttemptCount: item.AttemptCount, LastAttempt: item.LastAttempt,
		LastRepairStepID: item.LastRepairStepID, LastFailureStepID: item.LastFailureStepID,
		LastFailureClass: item.LastFailureClass, LastFailureSignature: item.LastFailureSignature,
		LastFailureTargetScope: item.LastFailureTargetScope, LastFailureAt: item.LastFailureAt,
		StructuredFailureSignals: signals, RepairInputCompaction: compaction, LastRepairInputBytes: item.LastRepairInputBytes,
		RepeatedFailureReason: item.RepeatedFailureReason, MissingDoDItems: missing, RecentAttempts: attempts,
		Escalated: item.Escalated, EscalationReason: item.EscalationReason, EscalationTicketID: item.EscalationTicketID,
		EvidenceRefs: append([]string(nil), item.EvidenceRefs...),
	}, nil
}

func repairLoopProjection(item workorchdevents.CLIRepairLoopInfo) projectioncontract.RepairLoop {
	return projectioncontract.RepairLoop{
		State: item.State, AttemptCount: item.AttemptCount, LastAttempt: item.LastAttempt,
		LastRepairStepID: item.LastRepairStepID, LastFailureStepID: item.LastFailureStepID,
		LastFailureClass: item.LastFailureClass, LastFailureSignature: item.LastFailureSignature,
		LastFailureTargetScope: item.LastFailureTargetScope, LastFailureAt: item.LastFailureAt,
		StructuredFailureSignals: item.StructuredFailureSignals.Map(), RepairInputCompaction: item.RepairInputCompaction.Map(),
		LastRepairInputBytes: item.LastRepairInputBytes, RepeatedFailureReason: item.RepeatedFailureReason,
		MissingDoDItems: workorcheventpayload.MapSlice(item.MissingDoDItems), RecentAttempts: workorcheventpayload.MapSlice(item.RecentAttempts),
		Escalated: item.Escalated, EscalationReason: item.EscalationReason, EscalationTicketID: item.EscalationTicketID,
		EvidenceRefs: append([]string(nil), item.EvidenceRefs...),
	}
}

func taskProgressDTO(item projectioncontract.TaskProgress) (workorchdevents.CLITaskProgressInfo, error) {
	item = projectioncontract.CloneTaskProgress(item)
	repair, err := repairLoopDTO(item.RepairLoop)
	if err != nil {
		return workorchdevents.CLITaskProgressInfo{}, err
	}
	return workorchdevents.CLITaskProgressInfo{
		WorkspaceID: item.WorkspaceID, RunID: item.RunID, State: item.State, Summary: item.Summary,
		CurrentStepID: item.CurrentStepID, CurrentStepKind: item.CurrentStepKind, CurrentStepState: item.CurrentStepState,
		DiagnosticLineage: item.DiagnosticLineage, CurrentMilestone: item.CurrentMilestone,
		BlockingTicketID: item.BlockingTicketID, BlockingApprovalID: item.BlockingApprovalID,
		Milestones: item.Milestones, Items: item.Items, Blockers: item.Blockers, Recovery: item.Recovery,
		RepairLoop: repair, AssessmentOutcome: item.AssessmentOutcome, EvidenceRefs: item.EvidenceRefs, Runtime: item.Runtime,
	}, nil
}

func taskProgressProjection(item workorchdevents.CLITaskProgressInfo) projectioncontract.TaskProgress {
	return projectioncontract.CloneTaskProgress(projectioncontract.TaskProgress{
		WorkspaceID: item.WorkspaceID, RunID: item.RunID, State: item.State, Summary: item.Summary,
		CurrentStepID: item.CurrentStepID, CurrentStepKind: item.CurrentStepKind, CurrentStepState: item.CurrentStepState,
		DiagnosticLineage: item.DiagnosticLineage, CurrentMilestone: item.CurrentMilestone,
		BlockingTicketID: item.BlockingTicketID, BlockingApprovalID: item.BlockingApprovalID,
		Milestones: item.Milestones, Items: item.Items, Blockers: item.Blockers, Recovery: item.Recovery,
		RepairLoop: repairLoopProjection(item.RepairLoop), AssessmentOutcome: item.AssessmentOutcome,
		EvidenceRefs: item.EvidenceRefs, Runtime: item.Runtime,
	})
}
