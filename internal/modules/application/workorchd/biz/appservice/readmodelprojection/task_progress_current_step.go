package readmodelprojection

import (
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

// WD5 mechanical consolidation: same import contract; source list follows.

// source: progress_current_step.go

func applyTaskProgressCurrentStep(ret *projectioncontract.TaskProgress, snapshot readmodelcore.RunSnapshotState) {
	if snapshot.Step != nil {
		ret.CurrentStepID = snapshot.Step.ID
		ret.CurrentStepKind = llmcommon.CanonicalRuntimeBlockKind(snapshot.Step.Kind)
		ret.CurrentStepState = snapshot.Step.State
		ret.Recovery.StepID = snapshot.Step.ID
	}
	ret.DiagnosticLineage = ret.CurrentStepID
	if result := terminalTaskProgressResult(snapshot); result != nil {
		ret.CurrentStepID = firstNonEmptyTimelineString(result.SourceRef, result.ID)
		ret.CurrentStepKind = "result/" + strings.TrimSpace(firstNonEmptyTimelineString(result.SourceType, "terminal"))
		ret.CurrentStepState = result.State
		ret.Recovery.StepID = ret.CurrentStepID
		ret.DiagnosticLineage = ret.CurrentStepID
	}
}

// source: progress_workspace_current_step.go

func applyWorkspaceProgressCurrentStep(ret *projectioncontract.TaskProgress, snapshot readmodelcore.WorkspaceSnapshotState) {
	if ret == nil || snapshot.Step == nil {
		return
	}
	ret.CurrentStepID = snapshot.Step.ID
	ret.CurrentStepKind = llmcommon.CanonicalRuntimeBlockKind(snapshot.Step.Kind)
	ret.CurrentStepState = snapshot.Step.State
	ret.DiagnosticLineage = ret.CurrentStepID
	if ret.Recovery.StepID == "" {
		ret.Recovery.StepID = snapshot.Step.ID
	}
}
