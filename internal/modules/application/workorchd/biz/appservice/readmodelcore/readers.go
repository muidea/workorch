package readmodelcore

import (
	"context"

	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	orchestratormodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type WorkspaceReader interface {
	Get(context.Context, common.WorkspaceID) (*workspacemodel.Workspace, error)
	List(context.Context) ([]*workspacemodel.Workspace, error)
}

type GoalReader interface {
	ListByWorkspaceID(context.Context, common.WorkspaceID) ([]*workspacemodel.Goal, error)
}

type DoDReader interface {
	ListByWorkspaceID(context.Context, common.WorkspaceID) ([]*workspacemodel.DoD, error)
}

type RunReader interface {
	Get(context.Context, common.RunID) (*runmodel.Run, error)
	GetByWorkspaceID(context.Context, common.WorkspaceID, common.RunID) (*runmodel.Run, error)
	List(context.Context) ([]*runmodel.Run, error)
	ListByWorkspaceID(context.Context, common.WorkspaceID) ([]*runmodel.Run, error)
}

type ResultReader interface {
	ListByWorkspace(context.Context, common.WorkspaceID) ([]*resultmodel.Result, error)
	ListByRun(context.Context, common.RunID) ([]*resultmodel.Result, error)
}

type StepReader interface {
	ListByRun(context.Context, common.RunID) ([]*runmodel.Step, error)
}

type RuntimeCheckpointReader interface {
	ListByRun(context.Context, common.RunID) ([]*runmodel.RuntimeCheckpoint, error)
}

type ArtifactReader interface {
	ListByWorkspace(context.Context, common.WorkspaceID) ([]*resultmodel.Artifact, error)
	ListByRun(context.Context, common.RunID) ([]*resultmodel.Artifact, error)
	ListByResult(context.Context, common.ResultID) ([]*resultmodel.Artifact, error)
}

type ArtifactContinuationReader interface {
	ListByRun(context.Context, common.RunID) ([]*orchestratormodel.ArtifactContinuation, error)
}

type CoverageWorkItemReader interface {
	ListByRun(context.Context, common.RunID) ([]*orchestratormodel.CoverageWorkItem, error)
}

type ToolCallReader interface {
	ListByRun(context.Context, common.RunID) ([]*runmodel.ToolCall, error)
}

type PatchReader interface {
	ListByRun(context.Context, common.RunID) ([]*runmodel.Patch, error)
}

type ValidationResultReader interface {
	ListByRun(context.Context, common.RunID) ([]*runmodel.ValidationResult, error)
}

type MCPCapabilityReader interface {
	List(context.Context) ([]*mcpmodel.Capability, error)
}
