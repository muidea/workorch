package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	orchestratormodel "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	deletioncontract "github.com/muidea/workorch/internal/pkg/workorchdeletioncontract"
)

// OrchestratorWorkspaceDeletionRepository owns the permanent evidence fence.
// Active and purge callbacks share the same durable owner lock so a seal can
// neither race a late write nor be bypassed by a generic repository method.
type OrchestratorWorkspaceDeletionRepository interface {
	Create(context.Context, *orchestratormodel.WorkspaceDeletion) error
	Get(context.Context, common.WorkspaceID) (*orchestratormodel.WorkspaceDeletion, error)
	List(context.Context) ([]*orchestratormodel.WorkspaceDeletion, error)
	CompareAndSwap(context.Context, *orchestratormodel.WorkspaceDeletion, uint64) error
	WithWorkspaceActiveFence(context.Context, common.WorkspaceID, func(context.Context) error) error
	WithWorkspacePurgeFence(context.Context, deletioncontract.Scope, func(context.Context) error) error
}

type OrchestratorWorkspaceEvidencePurger interface {
	PurgeWorkspaceEvidence(context.Context, deletioncontract.Scope) (OrchestratorWorkspaceEvidencePurgeReceipt, error)
}

const (
	OrchestratorPurgeSurfaceToolCalls                 = "tool_calls"
	OrchestratorPurgeSurfacePatches                   = "patches"
	OrchestratorPurgeSurfaceValidationResults         = "validation_results"
	OrchestratorPurgeSurfaceEvidenceRecords           = "evidence_records"
	OrchestratorPurgeSurfaceCoverageWorkItems         = "coverage_work_items"
	OrchestratorPurgeSurfaceArtifactContinuations     = "artifact_continuations"
	OrchestratorPurgeSurfaceArtifactChunkMetadata     = "artifact_chunk_metadata"
	OrchestratorPurgeSurfaceArtifactChunkContent      = "artifact_chunk_content"
	OrchestratorPurgeSurfaceArtifactAssembledMetadata = "artifact_assembled_metadata"
	OrchestratorPurgeSurfaceArtifactAssembledContent  = "artifact_assembled_content"
)

var orchestratorWorkspaceEvidencePurgeSurfaces = []string{
	OrchestratorPurgeSurfaceToolCalls,
	OrchestratorPurgeSurfacePatches,
	OrchestratorPurgeSurfaceValidationResults,
	OrchestratorPurgeSurfaceEvidenceRecords,
	OrchestratorPurgeSurfaceCoverageWorkItems,
	OrchestratorPurgeSurfaceArtifactContinuations,
	OrchestratorPurgeSurfaceArtifactChunkMetadata,
	OrchestratorPurgeSurfaceArtifactChunkContent,
	OrchestratorPurgeSurfaceArtifactAssembledMetadata,
	OrchestratorPurgeSurfaceArtifactAssembledContent,
}

type OrchestratorWorkspaceEvidencePurgeObservation struct {
	Surface        string
	ObservedBefore uint64
	Deleted        uint64
	Remaining      uint64
}

type OrchestratorWorkspaceEvidencePurgeReceipt struct {
	Scope        deletioncontract.Scope
	Observations []OrchestratorWorkspaceEvidencePurgeObservation
	ProofHash    string
}

func (r OrchestratorWorkspaceEvidencePurgeReceipt) Clone() OrchestratorWorkspaceEvidencePurgeReceipt {
	cloned := r
	cloned.Observations = append([]OrchestratorWorkspaceEvidencePurgeObservation(nil), r.Observations...)
	return cloned
}

func EmptyOrchestratorWorkspaceEvidencePurgeObservations() []OrchestratorWorkspaceEvidencePurgeObservation {
	ret := make([]OrchestratorWorkspaceEvidencePurgeObservation, len(orchestratorWorkspaceEvidencePurgeSurfaces))
	for idx, surface := range orchestratorWorkspaceEvidencePurgeSurfaces {
		ret[idx] = OrchestratorWorkspaceEvidencePurgeObservation{Surface: surface}
	}
	return ret
}

func (r OrchestratorWorkspaceEvidencePurgeReceipt) ValidateFor(scope deletioncontract.Scope) error {
	scope = scope.Normalize()
	if scope.Validate() != nil || scope.Mode != deletioncontract.ModePurge || r.Scope.Normalize() != scope || len(r.Observations) != len(orchestratorWorkspaceEvidencePurgeSurfaces) {
		return common.ErrInvalidArgument
	}
	for idx, observation := range r.Observations {
		if observation.Surface != orchestratorWorkspaceEvidencePurgeSurfaces[idx] || observation.Remaining != 0 || observation.Deleted != observation.ObservedBefore {
			return common.ErrConflict
		}
	}
	if r.ProofHash != StableOrchestratorWorkspaceEvidencePurgeProofHash(scope, r.Observations) {
		return common.ErrConflict
	}
	return nil
}

func NewOrchestratorWorkspaceEvidencePurgeReceipt(scope deletioncontract.Scope, observations []OrchestratorWorkspaceEvidencePurgeObservation) (OrchestratorWorkspaceEvidencePurgeReceipt, error) {
	scope = scope.Normalize()
	if scope.Validate() != nil || scope.Mode != deletioncontract.ModePurge || len(observations) != len(orchestratorWorkspaceEvidencePurgeSurfaces) {
		return OrchestratorWorkspaceEvidencePurgeReceipt{}, common.ErrInvalidArgument
	}
	observations = append([]OrchestratorWorkspaceEvidencePurgeObservation(nil), observations...)
	receipt := OrchestratorWorkspaceEvidencePurgeReceipt{
		Scope: scope, Observations: observations,
		ProofHash: StableOrchestratorWorkspaceEvidencePurgeProofHash(scope, observations),
	}
	return receipt, receipt.ValidateFor(scope)
}

func StableOrchestratorWorkspaceEvidencePurgeProofHash(scope deletioncontract.Scope, observations []OrchestratorWorkspaceEvidencePurgeObservation) string {
	scope = scope.Normalize()
	material := fmt.Sprintf("orchestrator-workspace-evidence-purge\x00%s\x00%s\x00%d\x00%s\x00%s\x00%s", scope.WorkspaceID, scope.DeletionID, scope.DeletionGeneration, scope.Mode, scope.QuiescenceID, scope.RequestHash)
	for _, observation := range observations {
		material += fmt.Sprintf("\x00%s\x00%d\x00%d\x00%d", observation.Surface, observation.ObservedBefore, observation.Deleted, observation.Remaining)
	}
	digest := sha256.Sum256([]byte(material))
	return "orchestrator-workspace-evidence-purge-sha256:" + hex.EncodeToString(digest[:])
}
