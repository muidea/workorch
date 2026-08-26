package repository

import (
	"context"

	runtimeevidencemodel "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type fencedToolCallRepository struct {
	base      ToolCallRepository
	deletions OrchestratorWorkspaceDeletionRepository
}

func (r *fencedToolCallRepository) Create(ctx context.Context, item *runtimeevidencemodel.ToolCall) error {
	if item == nil || item.ID == "" || item.WorkspaceID == "" || item.RunID == "" {
		return common.ErrInvalidArgument
	}
	_, err := withOrchestratorActive(ctx, r.deletions, item.WorkspaceID, func(guarded context.Context) (struct{}, error) {
		return struct{}{}, r.base.Create(guarded, item)
	})
	return err
}

func (r *fencedToolCallRepository) Get(ctx context.Context, id common.ToolCallID) (*runtimeevidencemodel.ToolCall, error) {
	probe, err := r.base.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	workspaceID, err := exactOrchestratorWorkspace(ctx, probe.WorkspaceID)
	if err != nil {
		return nil, err
	}
	return withOrchestratorActive(ctx, r.deletions, workspaceID, func(guarded context.Context) (*runtimeevidencemodel.ToolCall, error) {
		item, err := r.base.Get(guarded, id)
		if err == nil && item.WorkspaceID != workspaceID {
			return nil, common.ErrConflict
		}
		return item, err
	})
}

func (r *fencedToolCallRepository) ListByRun(ctx context.Context, runID common.RunID) ([]*runtimeevidencemodel.ToolCall, error) {
	return fencedToolCallList(ctx, r.deletions, func(callCtx context.Context) ([]*runtimeevidencemodel.ToolCall, error) {
		return r.base.ListByRun(callCtx, runID)
	})
}

func (r *fencedToolCallRepository) ListByStep(ctx context.Context, stepID common.StepID) ([]*runtimeevidencemodel.ToolCall, error) {
	return fencedToolCallList(ctx, r.deletions, func(callCtx context.Context) ([]*runtimeevidencemodel.ToolCall, error) {
		return r.base.ListByStep(callCtx, stepID)
	})
}

func fencedToolCallList(ctx context.Context, deletions OrchestratorWorkspaceDeletionRepository, list func(context.Context) ([]*runtimeevidencemodel.ToolCall, error)) ([]*runtimeevidencemodel.ToolCall, error) {
	probe, err := list(ctx)
	if err != nil {
		return nil, err
	}
	_, workspaceID, err := filterOrchestratorWorkspaceList(ctx, probe, func(item *runtimeevidencemodel.ToolCall) common.WorkspaceID {
		return item.WorkspaceID
	})
	if err != nil || workspaceID == "" {
		return nil, err
	}
	return withOrchestratorActive(ctx, deletions, workspaceID, func(guarded context.Context) ([]*runtimeevidencemodel.ToolCall, error) {
		items, err := list(guarded)
		if err != nil {
			return nil, err
		}
		filtered, _, err := filterOrchestratorWorkspaceList(guarded, items, func(item *runtimeevidencemodel.ToolCall) common.WorkspaceID {
			return item.WorkspaceID
		})
		return filtered, err
	})
}

func (r *fencedToolCallRepository) Update(ctx context.Context, item *runtimeevidencemodel.ToolCall) error {
	if item == nil || item.ID == "" || item.WorkspaceID == "" || item.RunID == "" {
		return common.ErrInvalidArgument
	}
	current, err := r.base.Get(ctx, item.ID)
	if err != nil {
		return err
	}
	_, err = withOrchestratorActive(ctx, r.deletions, current.WorkspaceID, func(guarded context.Context) (struct{}, error) {
		persisted, err := r.base.Get(guarded, item.ID)
		if err != nil {
			return struct{}{}, err
		}
		if err := immutableOrchestratorEvidenceScope(persisted.WorkspaceID, persisted.RunID, item.WorkspaceID, item.RunID); err != nil {
			return struct{}{}, err
		}
		return struct{}{}, r.base.Update(guarded, item)
	})
	return err
}

func (r *fencedToolCallRepository) Delete(ctx context.Context, id common.ToolCallID) error {
	item, err := r.Get(ctx, id)
	if err != nil {
		return err
	}
	_, err = withOrchestratorActive(ctx, r.deletions, item.WorkspaceID, func(guarded context.Context) (struct{}, error) {
		return struct{}{}, r.base.Delete(guarded, id)
	})
	return err
}

type fencedPatchRepository struct {
	base      PatchRepository
	deletions OrchestratorWorkspaceDeletionRepository
}

func (r *fencedPatchRepository) Create(ctx context.Context, item *runtimeevidencemodel.Patch) error {
	if item == nil || item.ID == "" || item.WorkspaceID == "" || item.RunID == "" {
		return common.ErrInvalidArgument
	}
	_, err := withOrchestratorActive(ctx, r.deletions, item.WorkspaceID, func(guarded context.Context) (struct{}, error) {
		return struct{}{}, r.base.Create(guarded, item)
	})
	return err
}

func (r *fencedPatchRepository) Get(ctx context.Context, id common.PatchID) (*runtimeevidencemodel.Patch, error) {
	probe, err := r.base.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	workspaceID, err := exactOrchestratorWorkspace(ctx, probe.WorkspaceID)
	if err != nil {
		return nil, err
	}
	return withOrchestratorActive(ctx, r.deletions, workspaceID, func(guarded context.Context) (*runtimeevidencemodel.Patch, error) {
		item, err := r.base.Get(guarded, id)
		if err == nil && item.WorkspaceID != workspaceID {
			return nil, common.ErrConflict
		}
		return item, err
	})
}

func (r *fencedPatchRepository) ListByRun(ctx context.Context, runID common.RunID) ([]*runtimeevidencemodel.Patch, error) {
	return r.list(ctx, func(callCtx context.Context) ([]*runtimeevidencemodel.Patch, error) {
		return r.base.ListByRun(callCtx, runID)
	})
}

func (r *fencedPatchRepository) ListByStep(ctx context.Context, stepID common.StepID) ([]*runtimeevidencemodel.Patch, error) {
	return r.list(ctx, func(callCtx context.Context) ([]*runtimeevidencemodel.Patch, error) {
		return r.base.ListByStep(callCtx, stepID)
	})
}

func (r *fencedPatchRepository) list(ctx context.Context, list func(context.Context) ([]*runtimeevidencemodel.Patch, error)) ([]*runtimeevidencemodel.Patch, error) {
	probe, err := list(ctx)
	if err != nil {
		return nil, err
	}
	_, workspaceID, err := filterOrchestratorWorkspaceList(ctx, probe, func(item *runtimeevidencemodel.Patch) common.WorkspaceID {
		return item.WorkspaceID
	})
	if err != nil || workspaceID == "" {
		return nil, err
	}
	return withOrchestratorActive(ctx, r.deletions, workspaceID, func(guarded context.Context) ([]*runtimeevidencemodel.Patch, error) {
		items, err := list(guarded)
		if err != nil {
			return nil, err
		}
		filtered, _, err := filterOrchestratorWorkspaceList(guarded, items, func(item *runtimeevidencemodel.Patch) common.WorkspaceID {
			return item.WorkspaceID
		})
		return filtered, err
	})
}

func (r *fencedPatchRepository) Update(ctx context.Context, item *runtimeevidencemodel.Patch) error {
	if item == nil || item.ID == "" || item.WorkspaceID == "" || item.RunID == "" {
		return common.ErrInvalidArgument
	}
	current, err := r.base.Get(ctx, item.ID)
	if err != nil {
		return err
	}
	_, err = withOrchestratorActive(ctx, r.deletions, current.WorkspaceID, func(guarded context.Context) (struct{}, error) {
		persisted, err := r.base.Get(guarded, item.ID)
		if err != nil {
			return struct{}{}, err
		}
		if err := immutableOrchestratorEvidenceScope(persisted.WorkspaceID, persisted.RunID, item.WorkspaceID, item.RunID); err != nil {
			return struct{}{}, err
		}
		return struct{}{}, r.base.Update(guarded, item)
	})
	return err
}

func (r *fencedPatchRepository) Delete(ctx context.Context, id common.PatchID) error {
	item, err := r.Get(ctx, id)
	if err != nil {
		return err
	}
	_, err = withOrchestratorActive(ctx, r.deletions, item.WorkspaceID, func(guarded context.Context) (struct{}, error) {
		return struct{}{}, r.base.Delete(guarded, id)
	})
	return err
}

type fencedValidationResultRepository struct {
	base      ValidationResultRepository
	deletions OrchestratorWorkspaceDeletionRepository
}

func (r *fencedValidationResultRepository) Create(ctx context.Context, item *runtimeevidencemodel.ValidationResult) error {
	if item == nil || item.ID == "" || item.WorkspaceID == "" || item.RunID == "" {
		return common.ErrInvalidArgument
	}
	_, err := withOrchestratorActive(ctx, r.deletions, item.WorkspaceID, func(guarded context.Context) (struct{}, error) {
		return struct{}{}, r.base.Create(guarded, item)
	})
	return err
}

func (r *fencedValidationResultRepository) Get(ctx context.Context, id common.ValidationResultID) (*runtimeevidencemodel.ValidationResult, error) {
	probe, err := r.base.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	workspaceID, err := exactOrchestratorWorkspace(ctx, probe.WorkspaceID)
	if err != nil {
		return nil, err
	}
	return withOrchestratorActive(ctx, r.deletions, workspaceID, func(guarded context.Context) (*runtimeevidencemodel.ValidationResult, error) {
		item, err := r.base.Get(guarded, id)
		if err == nil && item.WorkspaceID != workspaceID {
			return nil, common.ErrConflict
		}
		return item, err
	})
}

func (r *fencedValidationResultRepository) ListByRun(ctx context.Context, runID common.RunID) ([]*runtimeevidencemodel.ValidationResult, error) {
	return r.list(ctx, func(callCtx context.Context) ([]*runtimeevidencemodel.ValidationResult, error) {
		return r.base.ListByRun(callCtx, runID)
	})
}

func (r *fencedValidationResultRepository) ListByStep(ctx context.Context, stepID common.StepID) ([]*runtimeevidencemodel.ValidationResult, error) {
	return r.list(ctx, func(callCtx context.Context) ([]*runtimeevidencemodel.ValidationResult, error) {
		return r.base.ListByStep(callCtx, stepID)
	})
}

func (r *fencedValidationResultRepository) list(ctx context.Context, list func(context.Context) ([]*runtimeevidencemodel.ValidationResult, error)) ([]*runtimeevidencemodel.ValidationResult, error) {
	probe, err := list(ctx)
	if err != nil {
		return nil, err
	}
	_, workspaceID, err := filterOrchestratorWorkspaceList(ctx, probe, func(item *runtimeevidencemodel.ValidationResult) common.WorkspaceID {
		return item.WorkspaceID
	})
	if err != nil || workspaceID == "" {
		return nil, err
	}
	return withOrchestratorActive(ctx, r.deletions, workspaceID, func(guarded context.Context) ([]*runtimeevidencemodel.ValidationResult, error) {
		items, err := list(guarded)
		if err != nil {
			return nil, err
		}
		filtered, _, err := filterOrchestratorWorkspaceList(guarded, items, func(item *runtimeevidencemodel.ValidationResult) common.WorkspaceID {
			return item.WorkspaceID
		})
		return filtered, err
	})
}

func (r *fencedValidationResultRepository) Update(ctx context.Context, item *runtimeevidencemodel.ValidationResult) error {
	if item == nil || item.ID == "" || item.WorkspaceID == "" || item.RunID == "" {
		return common.ErrInvalidArgument
	}
	current, err := r.base.Get(ctx, item.ID)
	if err != nil {
		return err
	}
	_, err = withOrchestratorActive(ctx, r.deletions, current.WorkspaceID, func(guarded context.Context) (struct{}, error) {
		persisted, err := r.base.Get(guarded, item.ID)
		if err != nil {
			return struct{}{}, err
		}
		if err := immutableOrchestratorEvidenceScope(persisted.WorkspaceID, persisted.RunID, item.WorkspaceID, item.RunID); err != nil {
			return struct{}{}, err
		}
		return struct{}{}, r.base.Update(guarded, item)
	})
	return err
}

func (r *fencedValidationResultRepository) Delete(ctx context.Context, id common.ValidationResultID) error {
	item, err := r.Get(ctx, id)
	if err != nil {
		return err
	}
	_, err = withOrchestratorActive(ctx, r.deletions, item.WorkspaceID, func(guarded context.Context) (struct{}, error) {
		return struct{}{}, r.base.Delete(guarded, id)
	})
	return err
}
