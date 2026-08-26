package memory

import (
	"context"
	"sort"
	"sync"

	orchestratormodel "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	deletioncontract "github.com/muidea/workorch/internal/pkg/workorchdeletioncontract"
)

type OrchestratorWorkspaceDeletionRepository struct {
	mu    sync.RWMutex
	items map[common.WorkspaceID]*orchestratormodel.WorkspaceDeletion
}

func NewOrchestratorWorkspaceDeletionRepository() *OrchestratorWorkspaceDeletionRepository {
	return &OrchestratorWorkspaceDeletionRepository{items: map[common.WorkspaceID]*orchestratormodel.WorkspaceDeletion{}}
}

func NewOrchestratorOwnerRepositorySet() repository.OrchestratorEvidenceOwnerRepositories {
	ret, _ := newOrchestratorOwnerRepositorySet()
	return ret
}

func newOrchestratorOwnerRepositorySet() (repository.OrchestratorEvidenceOwnerRepositories, *ArtifactContinuationRepository) {
	deletions := NewOrchestratorWorkspaceDeletionRepository()
	toolCalls := NewToolCallRepository()
	patches := NewPatchRepository()
	validationResults := NewValidationResultRepository()
	evidenceRecords := NewEvidenceRecordRepository()
	coverageWorkItems := NewCoverageWorkItemRepository()
	artifactContinuations := NewArtifactContinuationRepository()
	purger := &orchestratorWorkspaceEvidencePurger{
		deletions: deletions, toolCalls: toolCalls, patches: patches, validationResults: validationResults,
		evidenceRecords: evidenceRecords, coverageWorkItems: coverageWorkItems, artifactContinuations: artifactContinuations,
		purgeReceipts: map[common.WorkspaceID]repository.OrchestratorWorkspaceEvidencePurgeReceipt{},
	}
	ret, _ := repository.NewOrchestratorEvidenceOwnerRepositories(purger)
	return ret, artifactContinuations
}

type orchestratorWorkspaceEvidencePurger struct {
	deletions             *OrchestratorWorkspaceDeletionRepository
	toolCalls             *ToolCallRepository
	patches               *PatchRepository
	validationResults     *ValidationResultRepository
	evidenceRecords       *EvidenceRecordRepository
	coverageWorkItems     *CoverageWorkItemRepository
	artifactContinuations *ArtifactContinuationRepository
	purgeReceipts         map[common.WorkspaceID]repository.OrchestratorWorkspaceEvidencePurgeReceipt
}

func (p *orchestratorWorkspaceEvidencePurger) OrchestratorWorkspaceDeletions() repository.OrchestratorWorkspaceDeletionRepository {
	return p.deletions
}

func (p *orchestratorWorkspaceEvidencePurger) OrchestratorToolCalls() repository.ToolCallRepository {
	return p.toolCalls
}

func (p *orchestratorWorkspaceEvidencePurger) OrchestratorPatches() repository.PatchRepository {
	return p.patches
}

func (p *orchestratorWorkspaceEvidencePurger) OrchestratorValidationResults() repository.ValidationResultRepository {
	return p.validationResults
}

func (p *orchestratorWorkspaceEvidencePurger) OrchestratorEvidenceRecords() repository.EvidenceRecordRepository {
	return p.evidenceRecords
}

func (p *orchestratorWorkspaceEvidencePurger) OrchestratorCoverageWorkItems() repository.CoverageWorkItemRepository {
	return p.coverageWorkItems
}

func (p *orchestratorWorkspaceEvidencePurger) OrchestratorArtifactContinuations() repository.ArtifactContinuationRepository {
	return p.artifactContinuations
}

func (p *orchestratorWorkspaceEvidencePurger) OrchestratorWorkspacePurger() repository.OrchestratorWorkspaceEvidencePurger {
	return p
}

func (p *orchestratorWorkspaceEvidencePurger) PurgeWorkspaceEvidence(ctx context.Context, scope deletioncontract.Scope) (repository.OrchestratorWorkspaceEvidencePurgeReceipt, error) {
	if p == nil || p.deletions == nil || scope.Validate() != nil || scope.Mode != deletioncontract.ModePurge {
		return repository.OrchestratorWorkspaceEvidencePurgeReceipt{}, common.ErrInvalidArgument
	}
	var receipt repository.OrchestratorWorkspaceEvidencePurgeReceipt
	err := p.deletions.WithWorkspacePurgeFence(ctx, scope, func(context.Context) error {
		p.toolCalls.mu.Lock()
		defer p.toolCalls.mu.Unlock()
		p.patches.mu.Lock()
		defer p.patches.mu.Unlock()
		p.validationResults.mu.Lock()
		defer p.validationResults.mu.Unlock()
		p.evidenceRecords.mu.Lock()
		defer p.evidenceRecords.mu.Unlock()
		p.coverageWorkItems.mu.Lock()
		defer p.coverageWorkItems.mu.Unlock()
		p.artifactContinuations.mu.Lock()
		defer p.artifactContinuations.mu.Unlock()
		if existing, ok := p.purgeReceipts[scope.WorkspaceID]; ok {
			if existing.ValidateFor(scope) != nil || !orchestratorWorkspaceEvidenceCountsEmpty(p.observeWorkspaceEvidenceLocked(scope.WorkspaceID)) {
				return common.ErrConflict
			}
			receipt = existing.Clone()
			return nil
		}

		before := p.observeWorkspaceEvidenceLocked(scope.WorkspaceID)

		for key, item := range p.toolCalls.items {
			if item != nil && item.WorkspaceID == scope.WorkspaceID {
				delete(p.toolCalls.items, key)
			}
		}
		for key, item := range p.patches.items {
			if item != nil && item.WorkspaceID == scope.WorkspaceID {
				delete(p.patches.items, key)
			}
		}
		for key, item := range p.validationResults.items {
			if item != nil && item.WorkspaceID == scope.WorkspaceID {
				delete(p.validationResults.items, key)
			}
		}
		for key, item := range p.evidenceRecords.items {
			if item != nil && item.WorkspaceID == scope.WorkspaceID {
				delete(p.evidenceRecords.items, key)
			}
		}
		for key, item := range p.coverageWorkItems.items {
			if item != nil && item.WorkspaceID == scope.WorkspaceID {
				delete(p.coverageWorkItems.items, key)
			}
		}
		targetContinuationIDSet := make(map[common.ArtifactContinuationID]struct{})
		for id, workspaceID := range p.artifactContinuations.workspaceScopes {
			if workspaceID == scope.WorkspaceID {
				targetContinuationIDSet[id] = struct{}{}
			}
		}
		for key, item := range p.artifactContinuations.items {
			if item != nil && item.WorkspaceID == scope.WorkspaceID {
				targetContinuationIDSet[key] = struct{}{}
				delete(p.artifactContinuations.items, key)
			}
		}
		for id := range targetContinuationIDSet {
			delete(p.artifactContinuations.items, id)
			delete(p.artifactContinuations.chunks, id)
			delete(p.artifactContinuations.chunkContent, id)
			delete(p.artifactContinuations.assembledMeta, id)
			delete(p.artifactContinuations.assembledContent, id)
			delete(p.artifactContinuations.workspaceScopes, id)
		}

		after := p.observeWorkspaceEvidenceLocked(scope.WorkspaceID)
		observations := repository.EmptyOrchestratorWorkspaceEvidencePurgeObservations()
		for idx := range observations {
			observedBefore := before[observations[idx].Surface]
			remaining := after[observations[idx].Surface]
			if remaining > observedBefore {
				return common.ErrConflict
			}
			observations[idx].ObservedBefore = observedBefore
			observations[idx].Deleted = observedBefore - remaining
			observations[idx].Remaining = remaining
		}
		var err error
		receipt, err = repository.NewOrchestratorWorkspaceEvidencePurgeReceipt(scope, observations)
		if err == nil {
			p.purgeReceipts[scope.WorkspaceID] = receipt.Clone()
		}
		return err
	})
	if err != nil {
		return repository.OrchestratorWorkspaceEvidencePurgeReceipt{}, err
	}
	return receipt, receipt.ValidateFor(scope)
}

func orchestratorWorkspaceEvidenceCountsEmpty(counts map[string]uint64) bool {
	for _, count := range counts {
		if count != 0 {
			return false
		}
	}
	return true
}

func (p *orchestratorWorkspaceEvidencePurger) observeWorkspaceEvidenceLocked(workspaceID common.WorkspaceID) map[string]uint64 {
	counts := map[string]uint64{}
	for _, item := range p.toolCalls.items {
		if item != nil && item.WorkspaceID == workspaceID {
			counts[repository.OrchestratorPurgeSurfaceToolCalls]++
		}
	}
	for _, item := range p.patches.items {
		if item != nil && item.WorkspaceID == workspaceID {
			counts[repository.OrchestratorPurgeSurfacePatches]++
		}
	}
	for _, item := range p.validationResults.items {
		if item != nil && item.WorkspaceID == workspaceID {
			counts[repository.OrchestratorPurgeSurfaceValidationResults]++
		}
	}
	for _, item := range p.evidenceRecords.items {
		if item != nil && item.WorkspaceID == workspaceID {
			counts[repository.OrchestratorPurgeSurfaceEvidenceRecords]++
		}
	}
	for _, item := range p.coverageWorkItems.items {
		if item != nil && item.WorkspaceID == workspaceID {
			counts[repository.OrchestratorPurgeSurfaceCoverageWorkItems]++
		}
	}
	targetContinuationIDs := make(map[common.ArtifactContinuationID]struct{})
	for id, scope := range p.artifactContinuations.workspaceScopes {
		if scope == workspaceID {
			targetContinuationIDs[id] = struct{}{}
		}
	}
	for id, item := range p.artifactContinuations.items {
		if item != nil && item.WorkspaceID == workspaceID {
			counts[repository.OrchestratorPurgeSurfaceArtifactContinuations]++
			targetContinuationIDs[id] = struct{}{}
		}
	}
	for id := range targetContinuationIDs {
		counts[repository.OrchestratorPurgeSurfaceArtifactChunkMetadata] += uint64(len(p.artifactContinuations.chunks[id]))
		counts[repository.OrchestratorPurgeSurfaceArtifactChunkContent] += uint64(len(p.artifactContinuations.chunkContent[id]))
		if p.artifactContinuations.assembledMeta[id] != nil {
			counts[repository.OrchestratorPurgeSurfaceArtifactAssembledMetadata]++
		}
		if _, ok := p.artifactContinuations.assembledContent[id]; ok {
			counts[repository.OrchestratorPurgeSurfaceArtifactAssembledContent]++
		}
	}
	return counts
}

func (r *OrchestratorWorkspaceDeletionRepository) Create(_ context.Context, item *orchestratormodel.WorkspaceDeletion) error {
	if r == nil || item == nil || item.Validate() != nil || item.Revision != 1 {
		return common.ErrInvalidArgument
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.items[item.WorkspaceID]; exists {
		return common.ErrConflict
	}
	r.items[item.WorkspaceID] = orchestratormodel.CloneWorkspaceDeletion(item)
	return nil
}

func (r *OrchestratorWorkspaceDeletionRepository) Get(_ context.Context, workspaceID common.WorkspaceID) (*orchestratormodel.WorkspaceDeletion, error) {
	if r == nil || workspaceID == "" {
		return nil, common.ErrInvalidArgument
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	item := r.items[workspaceID]
	if item == nil {
		return nil, common.ErrNotFound
	}
	return orchestratormodel.CloneWorkspaceDeletion(item), nil
}

func (r *OrchestratorWorkspaceDeletionRepository) List(_ context.Context) ([]*orchestratormodel.WorkspaceDeletion, error) {
	if r == nil {
		return nil, common.ErrInvalidArgument
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	ret := make([]*orchestratormodel.WorkspaceDeletion, 0, len(r.items))
	for _, item := range r.items {
		ret = append(ret, orchestratormodel.CloneWorkspaceDeletion(item))
	}
	sort.Slice(ret, func(i, j int) bool { return ret[i].WorkspaceID < ret[j].WorkspaceID })
	return ret, nil
}

func (r *OrchestratorWorkspaceDeletionRepository) CompareAndSwap(_ context.Context, item *orchestratormodel.WorkspaceDeletion, expectedRevision uint64) error {
	if r == nil || item == nil || item.Validate() != nil || expectedRevision == 0 || item.Revision != expectedRevision+1 {
		return common.ErrInvalidArgument
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	current := r.items[item.WorkspaceID]
	if current == nil {
		return common.ErrNotFound
	}
	if current.Revision != expectedRevision || current.QuiescenceIntent() != item.QuiescenceIntent() {
		return common.ErrConflict
	}
	r.items[item.WorkspaceID] = orchestratormodel.CloneWorkspaceDeletion(item)
	return nil
}

func (r *OrchestratorWorkspaceDeletionRepository) WithWorkspaceActiveFence(ctx context.Context, workspaceID common.WorkspaceID, operation func(context.Context) error) error {
	if r == nil || workspaceID == "" || operation == nil {
		return common.ErrInvalidArgument
	}
	if deletionContextGuards(ctx, "orchestrator", workspaceID) {
		return operation(ctx)
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if item := r.items[workspaceID]; item != nil {
		return common.ErrNotFound
	}
	return operation(context.WithValue(ctx, workspaceDeletionGuardContextKey{}, workspaceDeletionGuardContext{owner: "orchestrator", workspaceID: workspaceID}))
}

func (r *OrchestratorWorkspaceDeletionRepository) WithWorkspacePurgeFence(ctx context.Context, scope deletioncontract.Scope, operation func(context.Context) error) error {
	scope = scope.Normalize()
	if r == nil || scope.Validate() != nil || scope.Mode != deletioncontract.ModePurge || operation == nil {
		return common.ErrInvalidArgument
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	item := r.items[scope.WorkspaceID]
	if item == nil || (item.Phase != orchestratormodel.WorkspaceDeletionPhaseFenced && item.Phase != orchestratormodel.WorkspaceDeletionPhasePurged) {
		return common.ErrNotFound
	}
	if item.Scope() != scope {
		return common.ErrConflict
	}
	return operation(context.WithValue(ctx, workspaceDeletionGuardContextKey{}, workspaceDeletionGuardContext{owner: "orchestrator", workspaceID: scope.WorkspaceID}))
}
