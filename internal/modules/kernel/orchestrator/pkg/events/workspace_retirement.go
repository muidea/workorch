package events

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/muidea/workorch/internal/pkg/common"
	deletioncontract "github.com/muidea/workorch/internal/pkg/workorchdeletioncontract"
)

type BeginWorkspaceRuntimeQuiescenceCommand struct {
	Intent deletioncontract.WorkspaceRuntimeQuiescenceIntent
}

func (c BeginWorkspaceRuntimeQuiescenceCommand) Validate() error { return c.Intent.Validate() }

type BeginWorkspaceRuntimeQuiescenceResult struct {
	WorkspaceID                    common.WorkspaceID
	DeletionID                     string
	GateID                         string
	OrchestratorQuiescenceRevision uint64
}

func (r BeginWorkspaceRuntimeQuiescenceResult) ValidateFor(command BeginWorkspaceRuntimeQuiescenceCommand) error {
	intent := command.Intent.Normalized()
	if err := intent.Validate(); err != nil {
		return err
	}
	if r.WorkspaceID != intent.WorkspaceID || r.DeletionID != intent.DeletionID || r.GateID != StableWorkspaceRuntimeQuiescingGateID(intent) || r.OrchestratorQuiescenceRevision != 1 {
		return common.ErrConflict
	}
	return nil
}

type ConfirmWorkspaceRuntimeQuiescenceCommand struct {
	Intent deletioncontract.WorkspaceRuntimeQuiescenceIntent
}

func (c ConfirmWorkspaceRuntimeQuiescenceCommand) Validate() error { return c.Intent.Validate() }

type ConfirmWorkspaceRuntimeQuiescenceResult struct {
	Token                          deletioncontract.WorkspaceRuntimeQuiescenceToken
	OrchestratorQuiescenceRevision uint64
}

func (r ConfirmWorkspaceRuntimeQuiescenceResult) ValidateFor(command ConfirmWorkspaceRuntimeQuiescenceCommand) error {
	if err := r.Token.ValidateFor(command.Intent); err != nil || r.OrchestratorQuiescenceRevision != 2 {
		if err != nil {
			return err
		}
		return common.ErrConflict
	}
	return nil
}

func StableWorkspaceRuntimeQuiescingGateID(intent deletioncontract.WorkspaceRuntimeQuiescenceIntent) string {
	intent = intent.Normalized()
	material := fmt.Sprintf("orchestrator-workspace-runtime-quiescing\x00%s\x00%s\x00%d\x00%s\x00%s\x00%d\x00%d", intent.WorkspaceID, intent.DeletionID, intent.DeletionGeneration, intent.DataDispositionIntent, intent.WorkspaceIntentHash, intent.RunDrainingRevision, intent.WorkspaceRunGeneration)
	digest := sha256.Sum256([]byte(material))
	return "orchestrator-workspace-runtime-quiescing-sha256:" + hex.EncodeToString(digest[:])
}

type SealWorkspaceOrchestratorDeletionCommand struct {
	WorkspaceID           common.WorkspaceID
	DeletionID            string
	DeletionGeneration    uint64
	DataDispositionIntent deletioncontract.Mode
	QuiescenceID          string
	RequestHash           string
}

func (c SealWorkspaceOrchestratorDeletionCommand) Scope() deletioncontract.Scope {
	return deletioncontract.Scope{
		WorkspaceID: c.WorkspaceID, DeletionID: c.DeletionID, DeletionGeneration: c.DeletionGeneration,
		Mode: c.DataDispositionIntent, QuiescenceID: c.QuiescenceID, RequestHash: c.RequestHash,
	}.Normalize()
}

func (c SealWorkspaceOrchestratorDeletionCommand) Validate() error { return c.Scope().Validate() }

type SealWorkspaceOrchestratorDeletionResult struct {
	WorkspaceID        common.WorkspaceID
	DeletionID         string
	DeletionGeneration uint64
	FenceID            string
	Revision           uint64
	AckHash            string
}

// PurgeWorkspaceOrchestratorEvidenceCommand has fixed purge semantics.  The
// caller cannot select an owner-private phase or replay a retain request as a
// purge because the exact purge Scope always derives ModePurge.
type PurgeWorkspaceOrchestratorEvidenceCommand struct {
	WorkspaceID        common.WorkspaceID
	DeletionID         string
	DeletionGeneration uint64
	QuiescenceID       string
	RequestHash        string
}

func (c PurgeWorkspaceOrchestratorEvidenceCommand) Scope() deletioncontract.Scope {
	return deletioncontract.Scope{
		WorkspaceID: c.WorkspaceID, DeletionID: c.DeletionID, DeletionGeneration: c.DeletionGeneration,
		Mode: deletioncontract.ModePurge, QuiescenceID: c.QuiescenceID, RequestHash: c.RequestHash,
	}.Normalize()
}

func (c PurgeWorkspaceOrchestratorEvidenceCommand) Validate() error { return c.Scope().Validate() }

type PurgeWorkspaceOrchestratorEvidenceResult struct {
	WorkspaceID        common.WorkspaceID
	DeletionID         string
	DeletionGeneration uint64
	FenceID            string
	PurgeID            string
	Revision           uint64
	AckHash            string
	PurgeProofHash     string
}

func StableOrchestratorDeletionFenceID(scope deletioncontract.Scope) string {
	return orchestratorWorkspaceDeletionHash("fence", scope, "")
}

func StableOrchestratorDeletionSealAckHash(scope deletioncontract.Scope) string {
	return orchestratorWorkspaceDeletionHash("seal-ack", scope, StableOrchestratorDeletionFenceID(scope))
}

func StableOrchestratorDeletionPurgeProofHash(scope deletioncontract.Scope) string {
	scope = scope.Normalize()
	material := fmt.Sprintf("orchestrator-workspace-evidence-purge\x00%s\x00%s\x00%d\x00%s\x00%s\x00%s", scope.WorkspaceID, scope.DeletionID, scope.DeletionGeneration, scope.Mode, scope.QuiescenceID, scope.RequestHash)
	for _, surface := range []string{
		"tool_calls", "patches", "validation_results", "evidence_records", "coverage_work_items",
		"artifact_continuations", "artifact_chunk_metadata", "artifact_chunk_content",
		"artifact_assembled_metadata", "artifact_assembled_content",
	} {
		material += fmt.Sprintf("\x00%s\x000\x000\x000", surface)
	}
	digest := sha256.Sum256([]byte(material))
	return "orchestrator-workspace-evidence-purge-sha256:" + hex.EncodeToString(digest[:])
}

func StableOrchestratorDeletionPurgeID(scope deletioncontract.Scope) string {
	return orchestratorWorkspaceDeletionHash("purge", scope, StableOrchestratorDeletionFenceID(scope))
}

func StableOrchestratorDeletionPurgeAckHash(scope deletioncontract.Scope, purgeProofHash ...string) string {
	proofHash := StableOrchestratorDeletionPurgeProofHash(scope)
	if len(purgeProofHash) == 1 && purgeProofHash[0] != "" {
		proofHash = purgeProofHash[0]
	}
	return orchestratorWorkspaceDeletionHash("purge-ack", scope, proofHash)
}

func (r SealWorkspaceOrchestratorDeletionResult) ValidateFor(command SealWorkspaceOrchestratorDeletionCommand) error {
	if err := command.Validate(); err != nil {
		return err
	}
	scope := command.Scope()
	if r.WorkspaceID != scope.WorkspaceID || r.DeletionID != scope.DeletionID || r.DeletionGeneration != scope.DeletionGeneration ||
		r.FenceID != StableOrchestratorDeletionFenceID(scope) || r.Revision != 1 || r.AckHash != StableOrchestratorDeletionSealAckHash(scope) {
		return common.ErrConflict
	}
	return nil
}

func (r PurgeWorkspaceOrchestratorEvidenceResult) ValidateFor(command PurgeWorkspaceOrchestratorEvidenceCommand) error {
	if err := command.Validate(); err != nil {
		return err
	}
	scope := command.Scope()
	if r.WorkspaceID != scope.WorkspaceID || r.DeletionID != scope.DeletionID || r.DeletionGeneration != scope.DeletionGeneration ||
		r.FenceID != StableOrchestratorDeletionFenceID(scope) || r.PurgeID != StableOrchestratorDeletionPurgeID(scope) || r.Revision != 2 ||
		r.PurgeProofHash == "" || r.AckHash != StableOrchestratorDeletionPurgeAckHash(scope, r.PurgeProofHash) {
		return common.ErrConflict
	}
	return nil
}

func orchestratorWorkspaceDeletionHash(kind string, scope deletioncontract.Scope, extra string) string {
	scope = scope.Normalize()
	material := fmt.Sprintf("orchestrator-workspace-deletion\x00%s\x00%s\x00%d\x00%s\x00%s\x00%s\x00%s", kind, scope.WorkspaceID, scope.DeletionGeneration, scope.DeletionID, scope.Mode, scope.QuiescenceID, scope.RequestHash+"\x00"+extra)
	digest := sha256.Sum256([]byte(material))
	return "orchestrator-workspace-deletion-sha256:" + hex.EncodeToString(digest[:])
}
