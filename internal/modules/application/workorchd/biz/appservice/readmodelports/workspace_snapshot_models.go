package readmodelports

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelprojection"
	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	eventstreammodel "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/models"
	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	runtimeevidencemodel "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/models"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	orchestratormodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type workspaceSnapshotModels struct {
	Workspace             *workspacemodel.Workspace
	Runs                  []*runmodel.Run
	Goals                 []*workspacemodel.Goal
	DoDs                  []*workspacemodel.DoD
	Results               []*resultmodel.Result
	Artifacts             []*resultmodel.Artifact
	ArtifactContinuations []*orchestratormodel.ArtifactContinuation
	Patches               []*runtimeevidencemodel.Patch
	EventRecords          []*eventstreammodel.EventRecord
	LLMUsageEventRecords  []*eventstreammodel.EventRecord
	LLMUsageLoaded        bool
	LLMUsageLimited       bool
	Tickets               []*decisionmodel.Ticket
	ApprovalsRaw          []*approvalmodel.Approval
	Decisions             []projectioncontract.Decision
	Approvals             []projectioncontract.Approval
}

func (models workspaceSnapshotModels) selectProjectionRun() *runmodel.Run {
	return readmodelprojection.WorkspaceProjection().SelectProjectionRun(models.Workspace, models.Runs, models.Results, models.EventRecords)
}

func (models workspaceSnapshotModels) projectionInput(projectionRun *runmodel.Run) readmodelprojection.WorkspaceSnapshotModelInput {
	return readmodelprojection.WorkspaceSnapshotModelInput{
		Workspace:             models.Workspace,
		ProjectionRun:         projectionRun,
		Runs:                  models.Runs,
		Goals:                 models.Goals,
		DoDs:                  models.DoDs,
		Results:               models.Results,
		Artifacts:             models.Artifacts,
		ArtifactContinuations: models.ArtifactContinuations,
		Patches:               models.Patches,
		EventRecords:          models.EventRecords,
		LLMUsageEventRecords:  models.LLMUsageEventRecords,
		LLMUsageLoaded:        models.LLMUsageLoaded,
		LLMUsageLimited:       models.LLMUsageLimited,
		Decisions:             models.Decisions,
		Approvals:             models.Approvals,
	}
}
