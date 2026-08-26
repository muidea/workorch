package repository

import (
	"context"

	"github.com/muidea/workorch/internal/pkg/common"
)

// OrchestratorEvidenceOwnerRepositories is the only production construction
// surface for Orchestrator-owned evidence repositories. Every read and write
// shares one durable Workspace deletion fence; purge callbacks use the same
// guard and receive no unrestricted repository escape hatch.
type OrchestratorEvidenceOwnerRepositories struct {
	toolCalls             ToolCallRepository
	patches               PatchRepository
	validationResults     ValidationResultRepository
	evidenceRecords       EvidenceRecordRepository
	coverageWorkItems     CoverageWorkItemRepository
	artifactContinuations ArtifactContinuationRepository
	workspaceDeletions    OrchestratorWorkspaceDeletionRepository
	workspacePurger       OrchestratorWorkspaceEvidencePurger
	ownerBinding          *orchestratorEvidenceOwnerBinding
}

type orchestratorEvidenceOwnerBinding struct{ marker byte }

func newOrchestratorEvidenceOwnerBinding() *orchestratorEvidenceOwnerBinding {
	return &orchestratorEvidenceOwnerBinding{marker: 1}
}

// OrchestratorEvidenceBackend is a single backend-minted capability. The
// canonical factory accepts this aggregate rather than independently supplied
// repositories, so an ordinary caller cannot accidentally pair fence A with
// resources B and purger C. Boundary verification restricts implementations
// and constructor calls to the memory/file backend factories.
type OrchestratorEvidenceBackend interface {
	OrchestratorWorkspaceDeletions() OrchestratorWorkspaceDeletionRepository
	OrchestratorToolCalls() ToolCallRepository
	OrchestratorPatches() PatchRepository
	OrchestratorValidationResults() ValidationResultRepository
	OrchestratorEvidenceRecords() EvidenceRecordRepository
	OrchestratorCoverageWorkItems() CoverageWorkItemRepository
	OrchestratorArtifactContinuations() ArtifactContinuationRepository
	OrchestratorWorkspacePurger() OrchestratorWorkspaceEvidencePurger
}

func (r OrchestratorEvidenceOwnerRepositories) Validate() error {
	if r.ownerBinding == nil || r.toolCalls == nil || r.patches == nil || r.validationResults == nil || r.evidenceRecords == nil || r.coverageWorkItems == nil || r.artifactContinuations == nil || r.workspaceDeletions == nil || r.workspacePurger == nil {
		return common.ErrInvalidArgument
	}
	return nil
}

func (r OrchestratorEvidenceOwnerRepositories) ToolCalls() ToolCallRepository {
	return r.toolCalls
}

func (r OrchestratorEvidenceOwnerRepositories) Patches() PatchRepository {
	return r.patches
}

func (r OrchestratorEvidenceOwnerRepositories) ValidationResults() ValidationResultRepository {
	return r.validationResults
}

func (r OrchestratorEvidenceOwnerRepositories) EvidenceRecords() EvidenceRecordRepository {
	return r.evidenceRecords
}

func (r OrchestratorEvidenceOwnerRepositories) CoverageWorkItems() CoverageWorkItemRepository {
	return r.coverageWorkItems
}

func (r OrchestratorEvidenceOwnerRepositories) ArtifactContinuations() ArtifactContinuationRepository {
	return r.artifactContinuations
}

func (r OrchestratorEvidenceOwnerRepositories) WorkspaceDeletions() OrchestratorWorkspaceDeletionRepository {
	return r.workspaceDeletions
}

func (r OrchestratorEvidenceOwnerRepositories) WorkspacePurger() OrchestratorWorkspaceEvidencePurger {
	return r.workspacePurger
}

func NewOrchestratorEvidenceOwnerRepositories(backend OrchestratorEvidenceBackend) (OrchestratorEvidenceOwnerRepositories, error) {
	if backend == nil {
		return OrchestratorEvidenceOwnerRepositories{}, common.ErrInvalidArgument
	}
	deletions := backend.OrchestratorWorkspaceDeletions()
	toolCalls := backend.OrchestratorToolCalls()
	patches := backend.OrchestratorPatches()
	validationResults := backend.OrchestratorValidationResults()
	evidenceRecords := backend.OrchestratorEvidenceRecords()
	coverageWorkItems := backend.OrchestratorCoverageWorkItems()
	artifactContinuations := backend.OrchestratorArtifactContinuations()
	workspacePurger := backend.OrchestratorWorkspacePurger()
	if deletions == nil || toolCalls == nil || patches == nil || validationResults == nil || evidenceRecords == nil || coverageWorkItems == nil || artifactContinuations == nil || workspacePurger == nil {
		return OrchestratorEvidenceOwnerRepositories{}, common.ErrInvalidArgument
	}
	return OrchestratorEvidenceOwnerRepositories{
		toolCalls:             &fencedToolCallRepository{base: toolCalls, deletions: deletions},
		patches:               &fencedPatchRepository{base: patches, deletions: deletions},
		validationResults:     &fencedValidationResultRepository{base: validationResults, deletions: deletions},
		evidenceRecords:       &fencedEvidenceRecordRepository{base: evidenceRecords, deletions: deletions},
		coverageWorkItems:     &fencedCoverageWorkItemRepository{base: coverageWorkItems, deletions: deletions},
		artifactContinuations: &fencedArtifactContinuationRepository{base: artifactContinuations, deletions: deletions},
		workspaceDeletions:    deletions,
		workspacePurger:       workspacePurger,
		ownerBinding:          newOrchestratorEvidenceOwnerBinding(),
	}, nil
}

func withOrchestratorActive[T any](ctx context.Context, deletions OrchestratorWorkspaceDeletionRepository, workspaceID common.WorkspaceID, operation func(context.Context) (T, error)) (T, error) {
	var value T
	if deletions == nil || workspaceID == "" || operation == nil {
		return value, common.ErrInvalidArgument
	}
	workspaceID, err := exactOrchestratorWorkspace(ctx, workspaceID)
	if err != nil {
		return value, err
	}
	err = deletions.WithWorkspaceActiveFence(ctx, workspaceID, func(guarded context.Context) error {
		var err error
		value, err = operation(guarded)
		return err
	})
	return value, err
}

func immutableOrchestratorEvidenceScope(
	currentWorkspaceID common.WorkspaceID,
	currentRunID common.RunID,
	incomingWorkspaceID common.WorkspaceID,
	incomingRunID common.RunID,
) error {
	if currentWorkspaceID == "" || currentRunID == "" || incomingWorkspaceID == "" || incomingRunID == "" {
		return common.ErrInvalidArgument
	}
	if currentWorkspaceID != incomingWorkspaceID || currentRunID != incomingRunID {
		return common.ErrConflict
	}
	return nil
}

func exactOrchestratorWorkspace(ctx context.Context, observed common.WorkspaceID) (common.WorkspaceID, error) {
	bound := common.RuntimeWorkspaceIDFromContext(ctx)
	if observed == "" {
		return "", common.ErrInvalidArgument
	}
	if bound != "" && bound != observed {
		return "", common.ErrNotFound
	}
	return observed, nil
}

func filterOrchestratorWorkspaceList[T any](ctx context.Context, items []T, workspaceOf func(T) common.WorkspaceID) ([]T, common.WorkspaceID, error) {
	bound := common.RuntimeWorkspaceIDFromContext(ctx)
	if bound != "" {
		ret := make([]T, 0, len(items))
		for _, item := range items {
			if workspaceOf(item) == bound {
				ret = append(ret, item)
			}
		}
		if len(ret) == 0 {
			return nil, bound, nil
		}
		return ret, bound, nil
	}
	var workspaceID common.WorkspaceID
	for _, item := range items {
		current := workspaceOf(item)
		if current == "" {
			return nil, "", common.ErrConflict
		}
		if workspaceID != "" && workspaceID != current {
			return nil, "", common.ErrConflict
		}
		workspaceID = current
	}
	return items, workspaceID, nil
}
