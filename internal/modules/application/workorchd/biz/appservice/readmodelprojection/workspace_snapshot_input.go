package readmodelprojection

import (
	eventstreammodel "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/models"
	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	runtimeevidencemodel "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/models"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	orchestratormodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type WorkspaceSnapshotModelInput struct {
	Workspace             *workspacemodel.Workspace
	ProjectionRun         *runmodel.Run
	Runs                  []*runmodel.Run
	Goals                 []*workspacemodel.Goal
	DoDs                  []*workspacemodel.DoD
	Results               []*resultmodel.Result
	Artifacts             []*resultmodel.Artifact
	ArtifactContinuations []*orchestratormodel.ArtifactContinuation
	Patches               []*runtimeevidencemodel.Patch
	EventRecords          []*eventstreammodel.EventRecord
	LLMUsageEventRecords  []*eventstreammodel.EventRecord
	Decisions             []projectioncontract.Decision
	Approvals             []projectioncontract.Approval
}
