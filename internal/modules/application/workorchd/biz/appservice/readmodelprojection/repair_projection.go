package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func MapTaskProgressRepairLoop(snapshot readmodelcore.RunSnapshotState, recovery projectioncontract.RecoveryPlan) projectioncontract.RepairLoop {
	ret := projectioncontract.RepairLoop{
		State:      "inactive",
		NextAction: recovery.NextAction,
	}
	attemptCount, lastAttempt, lastRepairStepID := repairAttemptProjection(snapshot.Steps)
	ret.AttemptCount = attemptCount
	ret.LastAttempt = lastAttempt
	ret.LastRepairStepID = lastRepairStepID

	lastFailure := latestRepairFailureProjection(snapshot)
	if lastFailure != nil {
		ret.LastFailureStepID = lastFailure.stepID
		ret.LastFailureClass = lastFailure.class
		ret.LastFailureSignature = lastFailure.signature
		ret.LastFailureAt = lastFailure.occurredAt
		ret.EvidenceRefs = append(ret.EvidenceRefs, lastFailure.evidenceRef)
		if failedStep := stepByID(snapshot.Steps, lastFailure.stepID); failedStep != nil {
			ret.MissingDoDItems = MissingDoDItemsFromStepOutput(failedStep.Output)
		}
	}
	projectRepairGroundingObservability(&ret, snapshot)

	if ticketID, reason, escalationContext, ok := repairEscalationProjection(snapshot, recovery); ok {
		applyRepairEscalationProjection(&ret, ticketID, reason, escalationContext)
	} else {
		applyRepairLoopRecoveryState(&ret, recovery, attemptCount, lastFailure != nil)
	}

	ret.EvidenceRefs = compactUniqueStrings(append(ret.EvidenceRefs, ret.LastRepairStepID, ret.LastFailureStepID))
	return ret
}
