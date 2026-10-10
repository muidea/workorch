package workspace

import (
	"testing"
	"time"

	"github.com/muidea/workorch/internal/pkg/common"
	deletioncontract "github.com/muidea/workorch/internal/pkg/workorchdeletioncontract"
)

func TestWorkspaceDeletionRecordRejectsFinalAckWithoutCompletedPhase(t *testing.T) {
	now := time.Now().UTC()
	workspaceID := commonWorkspaceIDForDeletionTest()
	base := workspaceDeletionRecordForTest(workspaceID, deletioncontract.ModePurge, now)
	base.Phase = WorkspaceDeletionPhaseEventStreamFinalizing
	workspaceDeletionRetirementAcksForTest(&base, workspaceID, 3)
	base.OwnerSealAcks = workspaceDeletionSealAcksForTest(workspaceDeletionRequiredSealOwners, now)
	base.OwnerPurgeAcks = workspaceDeletionPurgeAcksForTest(workspaceDeletionRequiredPurgeOwners, now)
	base.DirectoryCleanupAck = &WorkspaceDeletionDirectoryCleanupAck{CleanupID: "cleanup-1", CleanedAt: now}
	if err := base.Validate(workspaceID); err == nil {
		t.Fatal("final Eventstream purge ack with incomplete phase was accepted")
	}

	retain := workspaceDeletionRecordForTest(workspaceID, deletioncontract.ModeRetain, now)
	retain.Phase = WorkspaceDeletionPhaseEventStreamFinalizing
	workspaceDeletionRetirementAcksForTest(&retain, workspaceID, 3)
	retain.OwnerSealAcks = workspaceDeletionSealAcksForTest(workspaceDeletionRequiredSealOwners, now)
	if err := retain.Validate(workspaceID); err == nil {
		t.Fatal("retain final Eventstream seal with incomplete phase was accepted")
	}
}

func TestWorkspaceDeletionRecordCompletedRequiresExactOwnerSetsAndTimes(t *testing.T) {
	now := time.Now().UTC()
	workspaceID := commonWorkspaceIDForDeletionTest()
	record := workspaceDeletionRecordForTest(workspaceID, deletioncontract.ModePurge, now)
	record.Phase = WorkspaceDeletionPhaseCompleted
	workspaceDeletionRetirementAcksForTest(&record, workspaceID, 5)
	record.OwnerSealAcks = workspaceDeletionSealAcksForTest(workspaceDeletionRequiredSealOwners, now)
	record.OwnerPurgeAcks = workspaceDeletionPurgeAcksForTest(workspaceDeletionRequiredPurgeOwners, now)
	record.DirectoryCleanupAck = &WorkspaceDeletionDirectoryCleanupAck{CleanupID: "cleanup-1", CleanedAt: now}
	record.Completed = true
	record.CompletedAt = now
	if err := record.Validate(workspaceID); err != nil {
		t.Fatalf("valid completed record: %v", err)
	}

	missing := record.Clone()
	missing.OwnerPurgeAcks = missing.OwnerPurgeAcks[:len(missing.OwnerPurgeAcks)-1]
	if err := missing.Validate(workspaceID); err == nil {
		t.Fatal("completed record missing required owner was accepted")
	}
	badTime := record.Clone()
	badTime.CompletedAt = now.Add(-time.Second)
	if err := badTime.Validate(workspaceID); err == nil {
		t.Fatal("completed record with divergent CompletedAt/UpdatedAt was accepted")
	}
}

func workspaceDeletionRecordForTest(workspaceID common.WorkspaceID, mode deletioncontract.Mode, now time.Time) WorkspaceDeletionRecord {
	record := WorkspaceDeletionRecord{
		DeletionID: "deletion-1", DeletionGeneration: 1, RequestID: "request-1", Mode: mode,
		Phase: WorkspaceDeletionPhaseRequested, Revision: 1, RequestedAt: now, UpdatedAt: now,
		SealedDeliverableDirectory: "/tmp/workspace-deletion-test", SealedInternalDirectory: "/tmp/workspace-deletion-internal-test",
	}
	record.IntentHash = StableWorkspaceDeletionIntentHash(workspaceID, record)
	return record
}

func workspaceDeletionRetirementAcksForTest(record *WorkspaceDeletionRecord, workspaceID common.WorkspaceID, generation uint64) {
	if record == nil {
		return
	}
	record.RunDrainingAck = &WorkspaceDeletionRunDrainingAck{FenceRevision: 1, WorkspaceRunGeneration: generation}
	token := deletioncontract.WorkspaceRuntimeQuiescenceToken{
		WorkspaceID: workspaceID, DeletionID: record.DeletionID, DeletionGeneration: record.DeletionGeneration,
		DataDispositionIntent: record.Mode, WorkspaceIntentHash: record.IntentHash,
		RunDrainingRevision: 1, WorkspaceRunGeneration: generation, OrchestratorQuiescenceRevision: 2,
	}
	token.RuntimeInventoryHash = deletioncontract.StableWorkspaceRuntimeInventoryHash(token)
	token.QuiescenceID = deletioncontract.StableWorkspaceRuntimeQuiescenceID(token)
	record.RuntimeQuiescenceAck = &WorkspaceDeletionRuntimeQuiescenceAck{Token: token}
	record.RunRetiredAck = &WorkspaceDeletionRunRetiredAck{
		FenceRevision: 2, WorkspaceRunGeneration: generation,
		QuiescenceID: token.QuiescenceID, RuntimeInventoryHash: token.RuntimeInventoryHash,
	}
}

func workspaceDeletionSealAcksForTest(owners []string, now time.Time) []WorkspaceDeletionOwnerSealAck {
	ret := make([]WorkspaceDeletionOwnerSealAck, 0, len(owners))
	for _, owner := range owners {
		ret = append(ret, WorkspaceDeletionOwnerSealAck{OwnerModuleID: owner, FenceID: owner + "-fence", Revision: 1, AckHash: owner + "-ack", SealedAt: now})
	}
	return ret
}

func workspaceDeletionPurgeAcksForTest(owners []string, now time.Time) []WorkspaceDeletionOwnerPurgeAck {
	ret := make([]WorkspaceDeletionOwnerPurgeAck, 0, len(owners))
	for _, owner := range owners {
		ret = append(ret, WorkspaceDeletionOwnerPurgeAck{OwnerModuleID: owner, PurgeID: owner + "-purge", Revision: 1, AckHash: owner + "-ack", PurgeProofHash: owner + "-proof", PurgedAt: now})
	}
	return ret
}

func commonWorkspaceIDForDeletionTest() common.WorkspaceID { return "ws-deletion-model" }
