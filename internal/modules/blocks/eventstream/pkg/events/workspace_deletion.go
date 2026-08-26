package events

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/muidea/workorch/internal/pkg/common"
	deletioncontract "github.com/muidea/workorch/internal/pkg/workorchdeletioncontract"
)

type SealWorkspaceEventStreamDeletionCommand struct {
	WorkspaceID           common.WorkspaceID
	DeletionID            string
	DeletionGeneration    uint64
	DataDispositionIntent deletioncontract.Mode
	QuiescenceID          string
	RequestHash           string
}

func (c SealWorkspaceEventStreamDeletionCommand) Scope() deletioncontract.Scope {
	return deletioncontract.Scope{
		WorkspaceID: c.WorkspaceID, DeletionID: c.DeletionID, DeletionGeneration: c.DeletionGeneration,
		Mode: c.DataDispositionIntent, QuiescenceID: c.QuiescenceID, RequestHash: c.RequestHash,
	}.Normalize()
}

func (c SealWorkspaceEventStreamDeletionCommand) Validate() error { return c.Scope().Validate() }

type SealWorkspaceEventStreamDeletionResult struct {
	WorkspaceID        common.WorkspaceID
	DeletionID         string
	DeletionGeneration uint64
	FenceID            string
	Revision           uint64
	AckHash            string
}

type PurgeWorkspaceCommand struct {
	WorkspaceID        common.WorkspaceID
	DeletionID         string
	DeletionGeneration uint64
	QuiescenceID       string
	RequestHash        string
}

func (c PurgeWorkspaceCommand) Scope() deletioncontract.Scope {
	return deletioncontract.Scope{
		WorkspaceID: c.WorkspaceID, DeletionID: c.DeletionID, DeletionGeneration: c.DeletionGeneration,
		Mode: deletioncontract.ModePurge, QuiescenceID: c.QuiescenceID, RequestHash: c.RequestHash,
	}.Normalize()
}

func (c PurgeWorkspaceCommand) Validate() error {
	return c.Scope().Validate()
}

type PurgeWorkspaceResult struct {
	WorkspaceID        common.WorkspaceID
	DeletionID         string
	DeletionGeneration uint64
	FenceID            string
	PurgeID            string
	Revision           uint64
	AckHash            string
	PurgeProofHash     string
}

func StableEventStreamDeletionFenceID(scope deletioncontract.Scope) string {
	return eventStreamDeletionHash("fence", scope, "")
}

func StableEventStreamDeletionSealAckHash(scope deletioncontract.Scope) string {
	return eventStreamDeletionHash("seal-ack", scope, StableEventStreamDeletionFenceID(scope))
}

func StableEventStreamDeletionPurgeProofHash(scope deletioncontract.Scope) string {
	return eventStreamDeletionHash("purge-proof-empty", scope, StableEventStreamDeletionFenceID(scope))
}

func StableEventStreamDeletionPurgeID(scope deletioncontract.Scope) string {
	return eventStreamDeletionHash("purge", scope, StableEventStreamDeletionFenceID(scope))
}

func StableEventStreamDeletionPurgeAckHash(scope deletioncontract.Scope) string {
	return eventStreamDeletionHash("purge-ack", scope, StableEventStreamDeletionPurgeProofHash(scope))
}

func eventStreamDeletionHash(kind string, scope deletioncontract.Scope, extra string) string {
	scope = scope.Normalize()
	material := fmt.Sprintf("eventstream-workspace-deletion\x00%s\x00%s\x00%d\x00%s\x00%s\x00%s\x00%s", kind, scope.WorkspaceID, scope.DeletionGeneration, scope.DeletionID, scope.Mode, scope.QuiescenceID, scope.RequestHash+"\x00"+extra)
	digest := sha256.Sum256([]byte(material))
	return "eventstream-workspace-deletion-sha256:" + hex.EncodeToString(digest[:])
}

func (r SealWorkspaceEventStreamDeletionResult) ValidateFor(command SealWorkspaceEventStreamDeletionCommand) error {
	if err := command.Validate(); err != nil {
		return err
	}
	scope := command.Scope()
	if r.WorkspaceID != scope.WorkspaceID || r.DeletionID != scope.DeletionID || r.DeletionGeneration != scope.DeletionGeneration ||
		r.FenceID != StableEventStreamDeletionFenceID(scope) || r.Revision != 1 || r.AckHash != StableEventStreamDeletionSealAckHash(scope) {
		return common.ErrConflict
	}
	return nil
}

func (r PurgeWorkspaceResult) ValidateFor(command PurgeWorkspaceCommand) error {
	if err := command.Validate(); err != nil {
		return err
	}
	scope := command.Scope()
	if r.WorkspaceID != scope.WorkspaceID || r.DeletionID != scope.DeletionID || r.DeletionGeneration != scope.DeletionGeneration ||
		r.FenceID != StableEventStreamDeletionFenceID(scope) || r.PurgeID != StableEventStreamDeletionPurgeID(scope) ||
		r.Revision != 2 || r.PurgeProofHash != StableEventStreamDeletionPurgeProofHash(scope) || r.AckHash != StableEventStreamDeletionPurgeAckHash(scope) {
		return common.ErrConflict
	}
	return nil
}
