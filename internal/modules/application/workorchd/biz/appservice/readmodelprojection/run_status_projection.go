package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func mapRunStatus(snapshot readmodelcore.RunSnapshotState) projectioncontract.RunStatus {
	ret := projectioncontract.RunStatus{
		ID:          snapshot.Run.ID,
		WorkspaceID: snapshot.Run.WorkspaceID,
		State:       snapshot.Run.State,
		StepCount:   runtimeAwareStepCount(snapshot),

		DecisionCount:      len(snapshot.Decisions),
		ApprovalCount:      len(snapshot.Approvals),
		BlockingTicketID:   snapshot.Run.BlockingTicketID,
		BlockingApprovalID: snapshot.Run.BlockingApprovalID,
		FailureReason:      snapshot.Run.FailureReason,
	}
	if snapshot.Step != nil {
		ret.CurrentStepID = snapshot.Step.ID
		ret.CurrentStepKind = llmcommon.CanonicalRuntimeBlockKind(snapshot.Step.Kind)
		ret.CurrentStepState = snapshot.Step.State
		ret.DiagnosticLineage = snapshot.Step.ID
	}
	ret.CurrentActionLabel = currentActionLabelFromRunSnapshot(snapshot)
	ret.ConvergenceStatus = mapConvergenceStatus(snapshot)
	ret.Acceptance = projectioncontract.CloneAcceptanceProjection(snapshot.Acceptance)
	return ret
}

func runtimeAwareStepCount(snapshot readmodelcore.RunSnapshotState) int {
	if len(snapshot.Steps) > 0 {
		return len(snapshot.Steps)
	}
	if snapshot.TaskProgress.Runtime == nil {
		return 0
	}
	// Candidate attempts are not steps; keep dispatch/operation units only.
	summary := snapshot.TaskProgress.Runtime
	count := summary.CompletedDispatchCount
	if summary.ActiveDispatchID != "" {
		count++
	}
	return count
}
