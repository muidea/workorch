package eventstream_test

import (
	"testing"

	eventstreamevents "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/events"
	eventstreammodel "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/models"
	deletioncontract "github.com/muidea/workorch/internal/pkg/workorchdeletioncontract"
)

func TestWorkspaceDeletionCurrentPhasesValidate(t *testing.T) {
	scope := deletioncontract.Scope{
		WorkspaceID: "ws-eventstream-model", DeletionID: "deletion-eventstream-model", DeletionGeneration: 1,
		Mode: deletioncontract.ModePurge, QuiescenceID: "quiescence-eventstream-model",
	}
	scope.RequestHash = deletioncontract.StableRequestHash(scope)
	fenced := eventstreammodel.WorkspaceDeletion{
		WorkspaceID: scope.WorkspaceID, DeletionID: scope.DeletionID, DeletionGeneration: scope.DeletionGeneration,
		Mode: scope.Mode, QuiescenceID: scope.QuiescenceID, RequestHash: scope.RequestHash,
		FenceID: eventstreamevents.StableEventStreamDeletionFenceID(scope), Phase: eventstreammodel.WorkspaceDeletionPhaseFenced, Revision: 1,
		SealAckHash: eventstreamevents.StableEventStreamDeletionSealAckHash(scope), RequestedAt: "2026-08-25T00:00:00Z", UpdatedAt: "2026-08-25T00:00:00Z",
	}
	if err := fenced.Validate(); err != nil {
		t.Fatalf("validate fenced: %v", err)
	}
	purged := fenced
	purged.Phase = eventstreammodel.WorkspaceDeletionPhasePurged
	purged.Revision = 2
	purged.PurgeID = eventstreamevents.StableEventStreamDeletionPurgeID(scope)
	purged.PurgeAckHash = eventstreamevents.StableEventStreamDeletionPurgeAckHash(scope)
	purged.PurgeProofHash = eventstreamevents.StableEventStreamDeletionPurgeProofHash(scope)
	purged.PurgedAt = "2026-08-25T00:01:00Z"
	purged.UpdatedAt = purged.PurgedAt
	if err := purged.Validate(); err != nil {
		t.Fatalf("validate purged: %v", err)
	}
}
