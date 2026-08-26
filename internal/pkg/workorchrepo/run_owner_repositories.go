package workorchrepo

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	fileRepo "github.com/muidea/workorch/internal/pkg/repository/file"
)

type workspaceScopedRunWorkspaceResourcePurger struct {
	store *workspaceScopedFileStore
	runs  *workspaceScopedRunRepository
}

func (p *workspaceScopedRunWorkspaceResourcePurger) RunOwnerRepositoryBinding() repository.RunOwnerRepositoryBinding {
	if p == nil || p.store == nil {
		return repository.RunOwnerRepositoryBinding{}
	}
	return p.store.runOwnerBinding
}

func (p *workspaceScopedRunWorkspaceResourcePurger) PurgeWorkspaceResources(ctx context.Context, expected runmodel.WorkspaceRetirementFence) (repository.RunWorkspaceResourcePurgeReceipt, error) {
	expected = expected.Normalized()
	if p == nil || p.store == nil || p.runs == nil || expected.Validate() != nil || !expected.IsRetired() || expected.DataDispositionIntent != "purge" {
		return repository.RunWorkspaceResourcePurgeReceipt{}, common.ErrInvalidArgument
	}
	if scoped := common.RuntimeWorkspaceIDFromContext(ctx); scoped != "" && scoped != expected.WorkspaceID {
		return repository.RunWorkspaceResourcePurgeReceipt{}, common.ErrConflict
	}
	headLock := workspaceRunGenerationLock(p.store.workspaceRunGenerationPath(expected.WorkspaceID))
	headLock.Lock()
	defer headLock.Unlock()
	head, err := p.store.loadWorkspaceRunGenerationHead(expected.WorkspaceID)
	if err != nil {
		return repository.RunWorkspaceResourcePurgeReceipt{}, err
	}
	if head.Retirement == nil || !head.Retirement.IsRetired() || !runmodel.SameWorkspaceRetirementFence(*head.Retirement, expected) {
		return repository.RunWorkspaceResourcePurgeReceipt{}, common.ErrConflict
	}

	var receipt repository.RunWorkspaceResourcePurgeReceipt
	if head.Purge != nil {
		if err := head.Purge.Receipt.ValidateFor(expected); err != nil {
			return repository.RunWorkspaceResourcePurgeReceipt{}, common.ErrConflict
		}
		receipt = head.Purge.Receipt.Clone()
		if head.Purge.Phase == workspaceRunResourcePurgePhaseCompleted {
			empty, err := p.workspacePurgeSurfacesEmpty(expected.WorkspaceID)
			if err != nil {
				return repository.RunWorkspaceResourcePurgeReceipt{}, err
			}
			if !empty {
				return repository.RunWorkspaceResourcePurgeReceipt{}, common.ErrConflict
			}
			return receipt, nil
		}
	} else {
		observedRuns, observedSteps, observedCheckpoints, err := p.observeWorkspacePurgeSurfaces(expected.WorkspaceID)
		if err != nil {
			return repository.RunWorkspaceResourcePurgeReceipt{}, err
		}
		receipt, err = repository.NewRunWorkspaceResourcePurgeReceipt(expected, []runmodel.WorkspaceRetirementPurgeObservation{
			{Surface: runmodel.WorkspaceRetirementPurgeSurfaceRuns, ObservedBefore: observedRuns, DeletedCount: observedRuns},
			{Surface: runmodel.WorkspaceRetirementPurgeSurfaceSteps, ObservedBefore: observedSteps, DeletedCount: observedSteps},
			{Surface: runmodel.WorkspaceRetirementPurgeSurfaceCheckpoints, ObservedBefore: observedCheckpoints, DeletedCount: observedCheckpoints},
		})
		if err != nil {
			return repository.RunWorkspaceResourcePurgeReceipt{}, err
		}
		// Persist the original observations before touching any physical
		// surface. Recovery from every cut point can now finish the deletion
		// and return this exact receipt/proof.
		head.Purge = &workspaceRunResourcePurgeJournalRecord{Phase: workspaceRunResourcePurgePhasePending, Receipt: receipt.Clone()}
		if err := p.store.saveWorkspaceRunGenerationHead(expected.WorkspaceID, head); err != nil {
			return repository.RunWorkspaceResourcePurgeReceipt{}, err
		}
	}

	roots := p.store.workspaceRunRoots(expected.WorkspaceID)
	for _, root := range roots {
		for _, surface := range []string{workspaceScopedRunFile, workspaceScopedStepFile, workspaceScopedRuntimeCheckpointFile} {
			path := p.store.runPath(root, surface)
			if err := os.RemoveAll(path); err != nil {
				return repository.RunWorkspaceResourcePurgeReceipt{}, err
			}
			fileRepo.EvictMapFileCache(path)
		}
	}
	// Drop process-local raw repository caches for physically removed surfaces;
	// a response-loss retry or fresh read must observe disk, not a pre-purge
	// Step full-run cache.
	p.store.mu.Lock()
	for _, root := range roots {
		delete(p.store.runs, p.store.runPath(root, workspaceScopedRunFile))
		delete(p.store.steps, p.store.runPath(root, workspaceScopedStepFile))
		delete(p.store.runtimeCheckpoints, p.store.runPath(root, workspaceScopedRuntimeCheckpointFile))
	}
	p.store.mu.Unlock()

	empty, err := p.workspacePurgeSurfacesEmpty(expected.WorkspaceID)
	if err != nil {
		return repository.RunWorkspaceResourcePurgeReceipt{}, err
	}
	if !empty {
		return repository.RunWorkspaceResourcePurgeReceipt{}, common.ErrConflict
	}
	head.Purge = &workspaceRunResourcePurgeJournalRecord{Phase: workspaceRunResourcePurgePhaseCompleted, Receipt: receipt.Clone()}
	if err := p.store.saveWorkspaceRunGenerationHead(expected.WorkspaceID, head); err != nil {
		return repository.RunWorkspaceResourcePurgeReceipt{}, err
	}
	return receipt, nil
}

func (p *workspaceScopedRunWorkspaceResourcePurger) observeWorkspacePurgeSurfaces(workspaceID common.WorkspaceID) (uint64, uint64, uint64, error) {
	var observedRuns, observedSteps, observedCheckpoints uint64
	for _, root := range p.store.workspaceRunRoots(workspaceID) {
		observed, err := countRunOwnerPurgeSurface(p.store.runPath(root, workspaceScopedRunFile), workspaceScopedRunFile)
		if err != nil {
			return 0, 0, 0, err
		}
		observedRuns += observed
		observed, err = countRunOwnerPurgeSurface(p.store.runPath(root, workspaceScopedStepFile), workspaceScopedStepFile)
		if err != nil {
			return 0, 0, 0, err
		}
		observedSteps += observed
		observed, err = countRunOwnerPurgeSurface(p.store.runPath(root, workspaceScopedRuntimeCheckpointFile), workspaceScopedRuntimeCheckpointFile)
		if err != nil {
			return 0, 0, 0, err
		}
		observedCheckpoints += observed
	}
	return observedRuns, observedSteps, observedCheckpoints, nil
}

func (p *workspaceScopedRunWorkspaceResourcePurger) workspacePurgeSurfacesEmpty(workspaceID common.WorkspaceID) (bool, error) {
	runs, steps, checkpoints, err := p.observeWorkspacePurgeSurfaces(workspaceID)
	return runs == 0 && steps == 0 && checkpoints == 0, err
}

func countRunOwnerPurgeSurface(path, surface string) (uint64, error) {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	if surface == workspaceScopedRunFile {
		if info.IsDir() {
			return 0, common.ErrConflict
		}
		return 1, nil
	}
	if !info.IsDir() {
		return 0, common.ErrConflict
	}
	count := uint64(0)
	err = filepath.WalkDir(path, func(current string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		switch surface {
		case workspaceScopedStepFile:
			if entry.Name() == "metadata.json" {
				count++
			}
		case workspaceScopedRuntimeCheckpointFile:
			if filepath.Ext(current) == ".json" {
				count++
			}
		default:
			return common.ErrConflict
		}
		return nil
	})
	return count, err
}
