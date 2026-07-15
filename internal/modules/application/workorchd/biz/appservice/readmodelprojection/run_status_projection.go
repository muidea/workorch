package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
)

func MapRunStatus(snapshot readmodelcore.RunSnapshot) readmodelcore.RunStatus {
	ret := readmodelcore.RunStatus{
		ID:          snapshot.Run.ID,
		WorkspaceID: snapshot.Run.WorkspaceID,
		State:       snapshot.Run.State,
		StepCount:   runtimeV2AwareStepCount(snapshot),

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
	ret.ConvergenceStatus = MapConvergenceStatus(snapshot)
	return ret
}

func runtimeV2AwareStepCount(snapshot readmodelcore.RunSnapshot) int {
	if len(snapshot.Steps) > 0 {
		return len(snapshot.Steps)
	}
	if snapshot.TaskProgress.RuntimeV2 == nil {
		return 0
	}
	// Candidate attempts are not steps; keep dispatch/operation units only.
	summary := snapshot.TaskProgress.RuntimeV2
	count := summary.CompletedDispatchCount
	if summary.ActiveDispatchID != "" {
		count++
	}
	return count
}
