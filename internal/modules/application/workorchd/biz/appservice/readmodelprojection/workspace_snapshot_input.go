package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	eventstreammodel "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/models"
	resultmodel "github.com/muidea/workorch/internal/modules/kernel/result/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
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
	Patches               []*runmodel.Patch
	EventRecords          []*eventstreammodel.EventRecord
	LLMUsageEventRecords  []*eventstreammodel.EventRecord
	Decisions             []readmodelcore.Decision
	Approvals             []readmodelcore.Approval
}
