package readmodelports

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelprojection"
	approvalmodel "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/models"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	eventstreammodel "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/models"
	resultmodel "github.com/muidea/workorch/internal/modules/kernel/result/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
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
	Patches               []*runmodel.Patch
	EventRecords          []*eventstreammodel.EventRecord
	LLMUsageEventRecords  []*eventstreammodel.EventRecord
	Tickets               []*decisionmodel.Ticket
	ApprovalsRaw          []*approvalmodel.Approval
	Decisions             []readmodelcore.Decision
	Approvals             []readmodelcore.Approval
}

func (models workspaceSnapshotModels) selectProjectionRun() *runmodel.Run {
	return readmodelprojection.SelectWorkspaceProjectionRun(models.Workspace, models.Runs, models.Results, models.EventRecords)
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
		Decisions:             models.Decisions,
		Approvals:             models.Approvals,
	}
}
