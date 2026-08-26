package repository

import (
	"context"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type fencedEvidenceRecordRepository struct {
	base      EvidenceRecordRepository
	deletions OrchestratorWorkspaceDeletionRepository
}

func (r *fencedEvidenceRecordRepository) Create(ctx context.Context, item *runtimecontract.EvidenceRecord) error {
	if item == nil || item.ID == "" || item.WorkspaceID == "" || item.RunID == "" {
		return common.ErrInvalidArgument
	}
	_, err := withOrchestratorActive(ctx, r.deletions, item.WorkspaceID, func(guarded context.Context) (struct{}, error) {
		return struct{}{}, r.base.Create(guarded, item)
	})
	return err
}

func (r *fencedEvidenceRecordRepository) Get(ctx context.Context, id common.EvidenceRecordID) (*runtimecontract.EvidenceRecord, error) {
	probe, err := r.base.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	workspaceID, err := exactOrchestratorWorkspace(ctx, probe.WorkspaceID)
	if err != nil {
		return nil, err
	}
	return withOrchestratorActive(ctx, r.deletions, workspaceID, func(guarded context.Context) (*runtimecontract.EvidenceRecord, error) {
		item, err := r.base.Get(guarded, id)
		if err == nil && item.WorkspaceID != workspaceID {
			return nil, common.ErrConflict
		}
		return item, err
	})
}

func (r *fencedEvidenceRecordRepository) ListByRun(ctx context.Context, runID common.RunID) ([]*runtimecontract.EvidenceRecord, error) {
	return r.list(ctx, func(callCtx context.Context) ([]*runtimecontract.EvidenceRecord, error) {
		return r.base.ListByRun(callCtx, runID)
	})
}

func (r *fencedEvidenceRecordRepository) ListByStep(ctx context.Context, stepID common.StepID) ([]*runtimecontract.EvidenceRecord, error) {
	return r.list(ctx, func(callCtx context.Context) ([]*runtimecontract.EvidenceRecord, error) {
		return r.base.ListByStep(callCtx, stepID)
	})
}

func (r *fencedEvidenceRecordRepository) ListBySourceRef(ctx context.Context, runID common.RunID, sourceRef string) ([]*runtimecontract.EvidenceRecord, error) {
	return r.list(ctx, func(callCtx context.Context) ([]*runtimecontract.EvidenceRecord, error) {
		return r.base.ListBySourceRef(callCtx, runID, sourceRef)
	})
}

func (r *fencedEvidenceRecordRepository) list(ctx context.Context, list func(context.Context) ([]*runtimecontract.EvidenceRecord, error)) ([]*runtimecontract.EvidenceRecord, error) {
	probe, err := list(ctx)
	if err != nil {
		return nil, err
	}
	_, workspaceID, err := filterOrchestratorWorkspaceList(ctx, probe, func(item *runtimecontract.EvidenceRecord) common.WorkspaceID {
		return item.WorkspaceID
	})
	if err != nil || workspaceID == "" {
		return nil, err
	}
	return withOrchestratorActive(ctx, r.deletions, workspaceID, func(guarded context.Context) ([]*runtimecontract.EvidenceRecord, error) {
		items, err := list(guarded)
		if err != nil {
			return nil, err
		}
		filtered, _, err := filterOrchestratorWorkspaceList(guarded, items, func(item *runtimecontract.EvidenceRecord) common.WorkspaceID {
			return item.WorkspaceID
		})
		return filtered, err
	})
}

func (r *fencedEvidenceRecordRepository) Update(ctx context.Context, item *runtimecontract.EvidenceRecord) error {
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

func (r *fencedEvidenceRecordRepository) Delete(ctx context.Context, id common.EvidenceRecordID) error {
	item, err := r.Get(ctx, id)
	if err != nil {
		return err
	}
	_, err = withOrchestratorActive(ctx, r.deletions, item.WorkspaceID, func(guarded context.Context) (struct{}, error) {
		return struct{}{}, r.base.Delete(guarded, id)
	})
	return err
}

type fencedCoverageWorkItemRepository struct {
	base      CoverageWorkItemRepository
	deletions OrchestratorWorkspaceDeletionRepository
}

func (r *fencedCoverageWorkItemRepository) Create(ctx context.Context, item *runtimecontract.CoverageWorkItem) error {
	if item == nil || item.ID == "" || item.WorkspaceID == "" || item.RunID == "" {
		return common.ErrInvalidArgument
	}
	_, err := withOrchestratorActive(ctx, r.deletions, item.WorkspaceID, func(guarded context.Context) (struct{}, error) {
		return struct{}{}, r.base.Create(guarded, item)
	})
	return err
}

func (r *fencedCoverageWorkItemRepository) Get(ctx context.Context, id common.CoverageWorkItemID) (*runtimecontract.CoverageWorkItem, error) {
	probe, err := r.base.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	workspaceID, err := exactOrchestratorWorkspace(ctx, probe.WorkspaceID)
	if err != nil {
		return nil, err
	}
	return withOrchestratorActive(ctx, r.deletions, workspaceID, func(guarded context.Context) (*runtimecontract.CoverageWorkItem, error) {
		item, err := r.base.Get(guarded, id)
		if err == nil && item.WorkspaceID != workspaceID {
			return nil, common.ErrConflict
		}
		return item, err
	})
}

func (r *fencedCoverageWorkItemRepository) ListByRun(ctx context.Context, runID common.RunID) ([]*runtimecontract.CoverageWorkItem, error) {
	return r.list(ctx, func(callCtx context.Context) ([]*runtimecontract.CoverageWorkItem, error) {
		return r.base.ListByRun(callCtx, runID)
	})
}

func (r *fencedCoverageWorkItemRepository) ListActiveByRun(ctx context.Context, runID common.RunID) ([]*runtimecontract.CoverageWorkItem, error) {
	return r.list(ctx, func(callCtx context.Context) ([]*runtimecontract.CoverageWorkItem, error) {
		return r.base.ListActiveByRun(callCtx, runID)
	})
}

func (r *fencedCoverageWorkItemRepository) list(ctx context.Context, list func(context.Context) ([]*runtimecontract.CoverageWorkItem, error)) ([]*runtimecontract.CoverageWorkItem, error) {
	probe, err := list(ctx)
	if err != nil {
		return nil, err
	}
	_, workspaceID, err := filterOrchestratorWorkspaceList(ctx, probe, func(item *runtimecontract.CoverageWorkItem) common.WorkspaceID {
		return item.WorkspaceID
	})
	if err != nil || workspaceID == "" {
		return nil, err
	}
	return withOrchestratorActive(ctx, r.deletions, workspaceID, func(guarded context.Context) ([]*runtimecontract.CoverageWorkItem, error) {
		items, err := list(guarded)
		if err != nil {
			return nil, err
		}
		filtered, _, err := filterOrchestratorWorkspaceList(guarded, items, func(item *runtimecontract.CoverageWorkItem) common.WorkspaceID {
			return item.WorkspaceID
		})
		return filtered, err
	})
}

func (r *fencedCoverageWorkItemRepository) Update(ctx context.Context, item *runtimecontract.CoverageWorkItem) error {
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

func (r *fencedCoverageWorkItemRepository) Delete(ctx context.Context, id common.CoverageWorkItemID) error {
	item, err := r.Get(ctx, id)
	if err != nil {
		return err
	}
	_, err = withOrchestratorActive(ctx, r.deletions, item.WorkspaceID, func(guarded context.Context) (struct{}, error) {
		return struct{}{}, r.base.Delete(guarded, id)
	})
	return err
}

type fencedArtifactContinuationRepository struct {
	base      ArtifactContinuationRepository
	deletions OrchestratorWorkspaceDeletionRepository
}

func (r *fencedArtifactContinuationRepository) Create(ctx context.Context, item *runtimecontract.ArtifactContinuation) error {
	if item == nil || item.ID == "" || item.WorkspaceID == "" || item.RunID == "" {
		return common.ErrInvalidArgument
	}
	_, err := withOrchestratorActive(ctx, r.deletions, item.WorkspaceID, func(guarded context.Context) (struct{}, error) {
		return struct{}{}, r.base.Create(guarded, item)
	})
	return err
}

func (r *fencedArtifactContinuationRepository) Get(ctx context.Context, id common.ArtifactContinuationID) (*runtimecontract.ArtifactContinuation, error) {
	probe, err := r.base.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	workspaceID, err := exactOrchestratorWorkspace(ctx, probe.WorkspaceID)
	if err != nil {
		return nil, err
	}
	return withOrchestratorActive(ctx, r.deletions, workspaceID, func(guarded context.Context) (*runtimecontract.ArtifactContinuation, error) {
		item, err := r.base.Get(guarded, id)
		if err == nil && item.WorkspaceID != workspaceID {
			return nil, common.ErrConflict
		}
		return item, err
	})
}

func (r *fencedArtifactContinuationRepository) ListByRun(ctx context.Context, runID common.RunID) ([]*runtimecontract.ArtifactContinuation, error) {
	return r.list(ctx, func(callCtx context.Context) ([]*runtimecontract.ArtifactContinuation, error) {
		return r.base.ListByRun(callCtx, runID)
	})
}

func (r *fencedArtifactContinuationRepository) ListOpenByRun(ctx context.Context, runID common.RunID) ([]*runtimecontract.ArtifactContinuation, error) {
	return r.list(ctx, func(callCtx context.Context) ([]*runtimecontract.ArtifactContinuation, error) {
		return r.base.ListOpenByRun(callCtx, runID)
	})
}

func (r *fencedArtifactContinuationRepository) list(ctx context.Context, list func(context.Context) ([]*runtimecontract.ArtifactContinuation, error)) ([]*runtimecontract.ArtifactContinuation, error) {
	probe, err := list(ctx)
	if err != nil {
		return nil, err
	}
	_, workspaceID, err := filterOrchestratorWorkspaceList(ctx, probe, func(item *runtimecontract.ArtifactContinuation) common.WorkspaceID {
		return item.WorkspaceID
	})
	if err != nil || workspaceID == "" {
		return nil, err
	}
	return withOrchestratorActive(ctx, r.deletions, workspaceID, func(guarded context.Context) ([]*runtimecontract.ArtifactContinuation, error) {
		items, err := list(guarded)
		if err != nil {
			return nil, err
		}
		filtered, _, err := filterOrchestratorWorkspaceList(guarded, items, func(item *runtimecontract.ArtifactContinuation) common.WorkspaceID {
			return item.WorkspaceID
		})
		return filtered, err
	})
}

func (r *fencedArtifactContinuationRepository) Update(ctx context.Context, item *runtimecontract.ArtifactContinuation) error {
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

func (r *fencedArtifactContinuationRepository) Delete(ctx context.Context, id common.ArtifactContinuationID) error {
	item, err := r.Get(ctx, id)
	if err != nil {
		return err
	}
	_, err = withOrchestratorActive(ctx, r.deletions, item.WorkspaceID, func(guarded context.Context) (struct{}, error) {
		return struct{}{}, r.base.Delete(guarded, id)
	})
	return err
}

func (r *fencedArtifactContinuationRepository) SaveChunk(ctx context.Context, chunk *runtimecontract.ArtifactContinuationChunk, content string) error {
	if chunk == nil {
		return common.ErrInvalidArgument
	}
	parent, err := r.Get(ctx, chunk.ContinuationID)
	if err != nil {
		return err
	}
	_, err = withOrchestratorActive(ctx, r.deletions, parent.WorkspaceID, func(guarded context.Context) (struct{}, error) {
		return struct{}{}, r.base.SaveChunk(guarded, chunk, content)
	})
	return err
}

func (r *fencedArtifactContinuationRepository) GetChunk(ctx context.Context, id common.ArtifactContinuationID, sequence int) (*runtimecontract.ArtifactContinuationChunk, string, error) {
	parent, err := r.Get(ctx, id)
	if err != nil {
		return nil, "", err
	}
	var chunk *runtimecontract.ArtifactContinuationChunk
	var content string
	_, err = withOrchestratorActive(ctx, r.deletions, parent.WorkspaceID, func(guarded context.Context) (struct{}, error) {
		var readErr error
		chunk, content, readErr = r.base.GetChunk(guarded, id, sequence)
		return struct{}{}, readErr
	})
	return chunk, content, err
}

func (r *fencedArtifactContinuationRepository) ListChunks(ctx context.Context, id common.ArtifactContinuationID) ([]*runtimecontract.ArtifactContinuationChunk, error) {
	parent, err := r.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return withOrchestratorActive(ctx, r.deletions, parent.WorkspaceID, func(guarded context.Context) ([]*runtimecontract.ArtifactContinuationChunk, error) {
		return r.base.ListChunks(guarded, id)
	})
}

func (r *fencedArtifactContinuationRepository) SaveAssembled(ctx context.Context, item *runtimecontract.ArtifactContinuationAssembledMetadata, content string) error {
	if item == nil {
		return common.ErrInvalidArgument
	}
	parent, err := r.Get(ctx, item.ContinuationID)
	if err != nil {
		return err
	}
	_, err = withOrchestratorActive(ctx, r.deletions, parent.WorkspaceID, func(guarded context.Context) (struct{}, error) {
		return struct{}{}, r.base.SaveAssembled(guarded, item, content)
	})
	return err
}

func (r *fencedArtifactContinuationRepository) GetAssembled(ctx context.Context, id common.ArtifactContinuationID) (*runtimecontract.ArtifactContinuationAssembledMetadata, string, error) {
	parent, err := r.Get(ctx, id)
	if err != nil {
		return nil, "", err
	}
	var item *runtimecontract.ArtifactContinuationAssembledMetadata
	var content string
	_, err = withOrchestratorActive(ctx, r.deletions, parent.WorkspaceID, func(guarded context.Context) (struct{}, error) {
		var readErr error
		item, content, readErr = r.base.GetAssembled(guarded, id)
		return struct{}{}, readErr
	})
	return item, content, err
}
