package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	eventstreammodel "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/models"
	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

// workspaceProjector is a typed, stateless read-only facade table.
type workspaceProjector struct {
	Activation                      func(*workspacemodel.Workspace, *runmodel.Run, *workspacemodel.Goal, *workspacemodel.DoD) readmodelcore.WorkspaceSnapshotState
	MapWorkspace                    func(*workspacemodel.Workspace, *runmodel.Run) appmodelcore.Workspace
	RuntimeView                     func(readmodelcore.WorkspaceSnapshotState, int, string) projectioncontract.WorkspaceRuntimeView
	RuntimeViewWithActivity         func(readmodelcore.WorkspaceSnapshotState, projectioncontract.WorkspaceActivityFeed, bool, int, string) projectioncontract.WorkspaceRuntimeView
	Detail                          func(readmodelcore.WorkspaceSnapshotState) readmodelcore.WorkspaceSnapshotState
	Summary                         func(readmodelcore.WorkspaceSnapshotState) readmodelcore.WorkspaceSnapshotState
	SelectDisplayRunID              func(readmodelcore.WorkspaceSnapshotState) string
	SelectProjectionRun             func(*workspacemodel.Workspace, []*runmodel.Run, []*resultmodel.Result, []*eventstreammodel.EventRecord) *runmodel.Run
	SnapshotFromModels              func(WorkspaceSnapshotModelInput) readmodelcore.WorkspaceSnapshotState
	SnapshotTaskProgress            func(readmodelcore.WorkspaceSnapshotState) projectioncontract.TaskProgress
	SnapshotWithRunSnapshot         func(readmodelcore.WorkspaceSnapshotState, *readmodelcore.RunSnapshotState) readmodelcore.WorkspaceSnapshotState
	TaskDecompositionsFromSnapshots func(readmodelcore.WorkspaceSnapshotState, *readmodelcore.RunSnapshotState) []projectioncontract.TaskDecomposition
	DisplayOutcome                  func(readmodelcore.WorkspaceSnapshotState) projectioncontract.WorkspaceDisplayOutcome
	PublicSnapshot                  func(readmodelcore.WorkspaceSnapshotState) projectioncontract.WorkspaceSnapshot
	PublicExecutionActivation       func(readmodelcore.WorkspaceExecutionActivationState) projectioncontract.WorkspaceExecutionActivation
	DiffSections                    func(readmodelcore.WorkspaceSnapshotState, readmodelcore.WorkspaceSnapshotState, string) []string
}

// WorkspaceProjection returns the one typed application-facing workspace
// projection facade. It owns no Store, revision, high-watermark, or command.
func WorkspaceProjection() workspaceProjector {
	return workspaceProjector{
		Activation:                      activationSnapshot,
		MapWorkspace:                    mapWorkspaceWithRunProjection,
		RuntimeView:                     projectWorkspaceRuntimeView,
		RuntimeViewWithActivity:         projectWorkspaceRuntimeViewWithActivity,
		Detail:                          projectWorkspaceSnapshotDetail,
		Summary:                         projectWorkspaceSnapshotSummary,
		SelectDisplayRunID:              selectDisplayRunID,
		SelectProjectionRun:             selectWorkspaceProjectionRun,
		SnapshotFromModels:              workspaceSnapshotFromModels,
		SnapshotTaskProgress:            workspaceSnapshotTaskProgress,
		SnapshotWithRunSnapshot:         workspaceSnapshotWithRunSnapshot,
		TaskDecompositionsFromSnapshots: workspaceTaskDecompositionsFromSnapshots,
		DisplayOutcome:                  mapWorkspaceOutcome,
		PublicSnapshot:                  publicWorkspaceSnapshot,
		PublicExecutionActivation:       publicWorkspaceExecutionActivation,
		DiffSections:                    diffWorkspaceProjectionSections,
	}
}
