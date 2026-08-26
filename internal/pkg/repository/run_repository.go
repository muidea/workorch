package repository

import (
	"context"

	run "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	deletioncontract "github.com/muidea/workorch/internal/pkg/workorchdeletioncontract"
)

type RunRepository interface {
	Create(context.Context, *run.Run) error
	// CreateNextWorkspaceRun atomically allocates the Run owner's durable
	// Workspace-scoped generation and RunID. The input must carry neither.
	CreateNextWorkspaceRun(context.Context, *run.Run) error
	Get(context.Context, common.RunID) (*run.Run, error)
	GetByWorkspaceID(context.Context, common.WorkspaceID, common.RunID) (*run.Run, error)
	List(context.Context) ([]*run.Run, error)
	ListByWorkspaceID(context.Context, common.WorkspaceID) ([]*run.Run, error)
	Update(context.Context, *run.Run) error
	CompareAndSwap(context.Context, *run.Run, uint64) error
	// CompareAndSwapRuntimeState is the sole physical persistence lane for an
	// Orchestrator-authored opaque RuntimeState/LifecycleFence. It preserves all
	// Run-owned fields, checks both physical and aggregate revisions, remains
	// available only to terminal Runs while Workspace retirement is draining,
	// and is closed permanently once retired.
	CompareAndSwapRuntimeState(context.Context, *run.Run, uint64, uint64) error
	Delete(context.Context, common.RunID) error
	// PrepareWorkspaceRetirement atomically verifies the Run-owned terminal /
	// replacement invariant and persists the draining phase beside the
	// Workspace generation head. Exact intent replay is idempotent.
	PrepareWorkspaceRetirement(context.Context, run.WorkspaceRetirementIntent) (*run.WorkspaceRetirementFence, error)
	// FinalizeWorkspaceRetirement compares an Orchestrator quiescence token with
	// the exact generation head and every Run physical/runtime revision, then
	// atomically advances the Run-private fence from draining to retired.
	FinalizeWorkspaceRetirement(context.Context, deletioncontract.WorkspaceRuntimeQuiescenceToken) (*run.WorkspaceRetirementFence, error)
	GetWorkspaceRetirement(context.Context, common.WorkspaceID) (*run.WorkspaceRetirementFence, error)
	ListWorkspaceRetirements(context.Context) ([]run.WorkspaceRetirementFence, error)
}
