package workspace

import (
	"errors"
	"testing"
	"time"

	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	deletioncontract "github.com/muidea/workorch/internal/pkg/workorchdeletioncontract"
)

func TestWorkspaceDeletionAdmissionIsIdempotentAndPayloadBound(t *testing.T) {
	now := time.Now().UTC()
	workspaceID := common.WorkspaceID("ws-deletion-admission")
	current := &workspacemodel.Workspace{ID: workspaceID, Revision: 7, State: common.WorkspaceStateActive, DeliverableDirectory: "/tmp/ws-deletion-admission", InternalDirectory: "/tmp/ws-deletion-internal"}
	command := workspaceevents.DeleteWorkspaceCommand{WorkspaceID: workspaceID, RequestID: "request-1", ExpectedRevision: 7}
	event := workspaceDeletionInternalEvent{Kind: workspaceDeletionInternalEventRequested, Requested: &workspaceDeletionRequestedFact{Command: command, OccurredAt: now}}
	first, err := reduceWorkspaceDeletionInternalEvent(current, event)
	if err != nil || first.Next == nil || first.Next.Deletion == nil {
		t.Fatalf("first admission transition=%+v err=%v", first, err)
	}
	duplicate, err := reduceWorkspaceDeletionInternalEvent(first.Next, event)
	if err != nil || !duplicate.Noop || !duplicate.Duplicate {
		t.Fatalf("duplicate transition=%+v err=%v", duplicate, err)
	}
	changed := event
	changed.Requested = &workspaceDeletionRequestedFact{Command: workspaceevents.DeleteWorkspaceCommand{WorkspaceID: workspaceID, RequestID: "request-1", Purge: true, ExpectedRevision: 7}, OccurredAt: now}
	if _, err := reduceWorkspaceDeletionInternalEvent(first.Next, changed); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("changed payload err=%v, want conflict", err)
	}
}

func TestWorkspaceDeletionAdmissionWithActiveRunWritesNoIntent(t *testing.T) {
	workspaceID := common.WorkspaceID("ws-deletion-active")
	current := &workspacemodel.Workspace{ID: workspaceID, Revision: 2, State: common.WorkspaceStateActive, ActiveRunID: "run-active"}
	event := workspaceDeletionInternalEvent{Kind: workspaceDeletionInternalEventRequested, Requested: &workspaceDeletionRequestedFact{
		Command: workspaceevents.DeleteWorkspaceCommand{WorkspaceID: workspaceID, RequestID: "request-active"}, OccurredAt: time.Now().UTC(),
	}}
	if _, err := reduceWorkspaceDeletionInternalEvent(current, event); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("active Run admission err=%v, want conflict", err)
	}
	if current.Deletion != nil {
		t.Fatalf("active Run admission mutated current Workspace: %+v", current.Deletion)
	}
}

func TestWorkspaceDeletionRejectsForgedOrStaleRunDrainingFact(t *testing.T) {
	now := time.Now().UTC()
	workspaceID := common.WorkspaceID("ws-deletion-fence")
	current := &workspacemodel.Workspace{ID: workspaceID, Revision: 1, State: common.WorkspaceStateActive}
	request := workspaceDeletionInternalEvent{Kind: workspaceDeletionInternalEventRequested, Requested: &workspaceDeletionRequestedFact{
		Command: workspaceevents.DeleteWorkspaceCommand{WorkspaceID: workspaceID, RequestID: "request-fence"}, OccurredAt: now,
	}}
	requested, err := reduceWorkspaceDeletionInternalEvent(current, request)
	if err != nil {
		t.Fatal(err)
	}
	fence := workspaceDeletionInternalEvent{Kind: workspaceDeletionInternalEventRunDrainingObserved, RunDraining: &workspaceDeletionRunDrainingFact{
		WorkspaceID: workspaceID, DeletionID: requested.Next.Deletion.DeletionID, FenceRevision: 1, WorkspaceRunGeneration: 4, OccurredAt: now.Add(time.Second),
	}}
	fenced, err := reduceWorkspaceDeletionInternalEvent(requested.Next, fence)
	if err != nil {
		t.Fatal(err)
	}
	forged := fence
	forged.RunDraining = &workspaceDeletionRunDrainingFact{WorkspaceID: workspaceID, DeletionID: requested.Next.Deletion.DeletionID, FenceRevision: 1, WorkspaceRunGeneration: 5, OccurredAt: now.Add(2 * time.Second)}
	if _, err := reduceWorkspaceDeletionInternalEvent(fenced.Next, forged); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("forged fence err=%v, want conflict", err)
	}
	quiescence := workspaceDeletionInternalEvent{Kind: workspaceDeletionInternalEventRuntimeQuiescenceObserved, RuntimeQuiescence: &workspaceDeletionRuntimeQuiescenceFact{
		WorkspaceID: workspaceID, DeletionID: requested.Next.Deletion.DeletionID,
		Token: workspaceDeletionQuiescenceTokenForTest(requested.Next.Deletion, workspaceID, 3), OccurredAt: now.Add(2 * time.Second),
	}}
	if _, err := reduceWorkspaceDeletionInternalEvent(fenced.Next, quiescence); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("stale quiescence err=%v, want conflict", err)
	}
}

func TestWorkspaceDeletionWaitingForIdleBlocksIngressAndExactRetryIsStable(t *testing.T) {
	now := time.Now().UTC()
	workspaceID := common.WorkspaceID("ws-deletion-waiting")
	current := &workspacemodel.Workspace{ID: workspaceID, Revision: 1, State: common.WorkspaceStateActive}
	command := workspaceevents.DeleteWorkspaceCommand{WorkspaceID: workspaceID, RequestID: "request-waiting"}
	requested, err := reduceWorkspaceDeletionInternalEvent(current, workspaceDeletionInternalEvent{Kind: workspaceDeletionInternalEventRequested, Requested: &workspaceDeletionRequestedFact{Command: command, OccurredAt: now}})
	if err != nil {
		t.Fatal(err)
	}
	waiting, err := reduceWorkspaceDeletionInternalEvent(requested.Next, workspaceDeletionInternalEvent{Kind: workspaceDeletionInternalEventRunBusyObserved, RunBusy: &workspaceDeletionRunBusyFact{WorkspaceID: workspaceID, DeletionID: requested.Next.Deletion.DeletionID, OccurredAt: now.Add(time.Second)}})
	if err != nil || !waiting.Next.Deletion.BlocksIngress() {
		t.Fatalf("waiting transition=%+v err=%v", waiting, err)
	}
	if err := workspaceDeletionRejectsMutation(waiting.Next); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("waiting mutation fence err=%v, want conflict", err)
	}
	projection, err := reduceRunProjectionFact(waiting.Next, workspaceRunProjectionFact{
		WorkspaceID: workspaceID, WorkspaceRunGeneration: 2, RunID: "run-busy", RunRevision: 1,
		SourceOwner: runevents.CommandDestination, CausationID: runevents.RunStateChangedCausationID("run-busy", 2, 1, 0), RunState: common.RunStatePending,
	})
	if err != nil || projection.RunProjection == nil {
		t.Fatalf("waiting private Run projection=%+v err=%v, want convergence allowed", projection, err)
	}
	retry, err := reduceWorkspaceDeletionInternalEvent(waiting.Next, workspaceDeletionInternalEvent{Kind: workspaceDeletionInternalEventRequested, Requested: &workspaceDeletionRequestedFact{Command: command, OccurredAt: now.Add(2 * time.Second)}})
	if err != nil || retry.Next.Deletion.Phase != workspacemodel.WorkspaceDeletionPhaseWaitingForIdle || !retry.Noop || !retry.Duplicate || retry.Next.Deletion.Revision != waiting.Next.Deletion.Revision {
		t.Fatalf("retry transition=%+v err=%v", retry, err)
	}
	fenced, err := reduceWorkspaceDeletionInternalEvent(retry.Next, workspaceDeletionInternalEvent{Kind: workspaceDeletionInternalEventRunDrainingObserved, RunDraining: &workspaceDeletionRunDrainingFact{
		WorkspaceID: workspaceID, DeletionID: retry.Next.Deletion.DeletionID, FenceRevision: 1, WorkspaceRunGeneration: 2, OccurredAt: now.Add(3 * time.Second),
	}})
	if err != nil || fenced.Next.Deletion.Phase != workspacemodel.WorkspaceDeletionPhaseRunDraining {
		t.Fatalf("waiting fence transition=%+v err=%v", fenced, err)
	}
}

func TestWorkspaceDeletionTransitionTimeClampsClockRegression(t *testing.T) {
	now := time.Now().UTC()
	workspaceID := common.WorkspaceID("ws-deletion-clock-regression")
	current := &workspacemodel.Workspace{ID: workspaceID, Revision: 1, State: common.WorkspaceStateActive}
	requested, err := reduceWorkspaceDeletionInternalEvent(current, workspaceDeletionInternalEvent{Kind: workspaceDeletionInternalEventRequested, Requested: &workspaceDeletionRequestedFact{
		Command: workspaceevents.DeleteWorkspaceCommand{WorkspaceID: workspaceID, RequestID: "request-clock"}, OccurredAt: now,
	}})
	if err != nil {
		t.Fatal(err)
	}
	waiting, err := reduceWorkspaceDeletionInternalEvent(requested.Next, workspaceDeletionInternalEvent{Kind: workspaceDeletionInternalEventRunBusyObserved, RunBusy: &workspaceDeletionRunBusyFact{
		WorkspaceID: workspaceID, DeletionID: requested.Next.Deletion.DeletionID, OccurredAt: now.Add(-time.Hour),
	}})
	if err != nil {
		t.Fatal(err)
	}
	if !waiting.Next.Deletion.UpdatedAt.Equal(now) {
		t.Fatalf("UpdatedAt=%s, want clamped %s", waiting.Next.Deletion.UpdatedAt, now)
	}
	fenced, err := reduceWorkspaceDeletionInternalEvent(waiting.Next, workspaceDeletionInternalEvent{Kind: workspaceDeletionInternalEventRunDrainingObserved, RunDraining: &workspaceDeletionRunDrainingFact{
		WorkspaceID: workspaceID, DeletionID: waiting.Next.Deletion.DeletionID, FenceRevision: 1, WorkspaceRunGeneration: 1, OccurredAt: now.Add(-2 * time.Hour),
	}})
	if err != nil || fenced.Next.Deletion.Validate(workspaceID) != nil || !fenced.Next.Deletion.UpdatedAt.Equal(now) {
		t.Fatalf("fenced deletion=%+v err=%v", fenced.Next.Deletion, err)
	}
}

func TestWorkspaceDeletionRejectsOutOfOrderAndMismatchedOwnerAcknowledgements(t *testing.T) {
	now := time.Now().UTC()
	workspaceID := common.WorkspaceID("ws-deletion-owner-ack")
	requested, err := reduceWorkspaceDeletionInternalEvent(&workspacemodel.Workspace{ID: workspaceID, State: common.WorkspaceStateActive}, workspaceDeletionInternalEvent{Kind: workspaceDeletionInternalEventRequested, Requested: &workspaceDeletionRequestedFact{
		Command: workspaceevents.DeleteWorkspaceCommand{WorkspaceID: workspaceID, RequestID: "request-owner-ack"}, OccurredAt: now,
	}})
	if err != nil {
		t.Fatal(err)
	}
	fenced, err := reduceWorkspaceDeletionInternalEvent(requested.Next, workspaceDeletionInternalEvent{Kind: workspaceDeletionInternalEventRunDrainingObserved, RunDraining: &workspaceDeletionRunDrainingFact{
		WorkspaceID: workspaceID, DeletionID: requested.Next.Deletion.DeletionID, FenceRevision: 1, WorkspaceRunGeneration: 2, OccurredAt: now,
	}})
	if err != nil {
		t.Fatal(err)
	}
	token := workspaceDeletionQuiescenceTokenForTest(requested.Next.Deletion, workspaceID, 2)
	quiescent, err := reduceWorkspaceDeletionInternalEvent(fenced.Next, workspaceDeletionInternalEvent{Kind: workspaceDeletionInternalEventRuntimeQuiescenceObserved, RuntimeQuiescence: &workspaceDeletionRuntimeQuiescenceFact{
		WorkspaceID: workspaceID, DeletionID: requested.Next.Deletion.DeletionID, Token: token, OccurredAt: now,
	}})
	if err != nil {
		t.Fatal(err)
	}
	retired, err := reduceWorkspaceDeletionInternalEvent(quiescent.Next, workspaceDeletionInternalEvent{Kind: workspaceDeletionInternalEventRunRetiredObserved, RunRetired: &workspaceDeletionRunRetiredFact{
		WorkspaceID: workspaceID, DeletionID: requested.Next.Deletion.DeletionID, FenceRevision: 2, WorkspaceRunGeneration: 2,
		QuiescenceID: token.QuiescenceID, RuntimeInventoryHash: token.RuntimeInventoryHash, OccurredAt: now,
	}})
	if err != nil {
		t.Fatal(err)
	}
	sealing, err := reduceWorkspaceDeletionInternalEvent(retired.Next, workspaceDeletionInternalEvent{Kind: workspaceDeletionInternalEventOwnerSealingStarted, OwnerSealing: &workspaceDeletionOwnerSealingFact{
		WorkspaceID: workspaceID, DeletionID: requested.Next.Deletion.DeletionID, OccurredAt: now,
	}})
	if err != nil {
		t.Fatal(err)
	}
	forgedMemory := workspaceDeletionInternalEvent{Kind: workspaceDeletionInternalEventOwnerSealObserved, OwnerSeal: &workspaceDeletionOwnerSealFact{
		WorkspaceID: workspaceID, DeletionID: requested.Next.Deletion.DeletionID, OwnerModuleID: "blocks/memory", FenceID: "memory-fence", Revision: 1, AckHash: "memory-ack", OccurredAt: now,
	}}
	if _, err := reduceWorkspaceDeletionInternalEvent(sealing.Next, forgedMemory); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("out-of-order owner ack err=%v, want conflict", err)
	}
	if len(sealing.Next.Deletion.OwnerSealAcks) != 0 {
		t.Fatalf("out-of-order owner ack mutated current=%+v", sealing.Next.Deletion.OwnerSealAcks)
	}
	orchestratorAck := workspaceDeletionInternalEvent{Kind: workspaceDeletionInternalEventOwnerSealObserved, OwnerSeal: &workspaceDeletionOwnerSealFact{
		WorkspaceID: workspaceID, DeletionID: requested.Next.Deletion.DeletionID, OwnerModuleID: "kernel/orchestrator", FenceID: "orchestrator-fence", Revision: 1, AckHash: "orchestrator-ack", OccurredAt: now,
	}}
	orchestratorSealed, err := reduceWorkspaceDeletionInternalEvent(sealing.Next, orchestratorAck)
	if err != nil {
		t.Fatal(err)
	}
	resultAck := workspaceDeletionInternalEvent{Kind: workspaceDeletionInternalEventOwnerSealObserved, OwnerSeal: &workspaceDeletionOwnerSealFact{
		WorkspaceID: workspaceID, DeletionID: requested.Next.Deletion.DeletionID, OwnerModuleID: "blocks/result", FenceID: "result-fence", Revision: 1, AckHash: "result-ack", OccurredAt: now,
	}}
	resultSealed, err := reduceWorkspaceDeletionInternalEvent(orchestratorSealed.Next, resultAck)
	if err != nil {
		t.Fatal(err)
	}
	mismatchedReplay := resultAck
	mismatchedReplay.OwnerSeal = &workspaceDeletionOwnerSealFact{
		WorkspaceID: workspaceID, DeletionID: requested.Next.Deletion.DeletionID, OwnerModuleID: "blocks/result", FenceID: "forged-result-fence", Revision: 1, AckHash: "result-ack", OccurredAt: now.Add(time.Second),
	}
	if _, err := reduceWorkspaceDeletionInternalEvent(resultSealed.Next, mismatchedReplay); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("mismatched replay err=%v, want conflict", err)
	}
	if len(resultSealed.Next.Deletion.OwnerSealAcks) != 2 || resultSealed.Next.Deletion.OwnerSealAcks[1].FenceID != "result-fence" {
		t.Fatalf("mismatched replay mutated persisted ack=%+v", resultSealed.Next.Deletion.OwnerSealAcks)
	}
}

func workspaceDeletionQuiescenceTokenForTest(record *workspacemodel.WorkspaceDeletionRecord, workspaceID common.WorkspaceID, generation uint64) deletioncontract.WorkspaceRuntimeQuiescenceToken {
	if record == nil {
		return deletioncontract.WorkspaceRuntimeQuiescenceToken{}
	}
	token := deletioncontract.WorkspaceRuntimeQuiescenceToken{
		WorkspaceID: workspaceID, DeletionID: record.DeletionID, DeletionGeneration: record.DeletionGeneration,
		DataDispositionIntent: record.Mode, WorkspaceIntentHash: record.IntentHash,
		RunDrainingRevision: 1, WorkspaceRunGeneration: generation, OrchestratorQuiescenceRevision: 2,
	}
	token.RuntimeInventoryHash = deletioncontract.StableWorkspaceRuntimeInventoryHash(token)
	token.QuiescenceID = deletioncontract.StableWorkspaceRuntimeQuiescenceID(token)
	return token
}

func TestWorkspaceDeletionCompletionTimeClampsClockRegression(t *testing.T) {
	now := time.Now().UTC()
	workspaceID := common.WorkspaceID("ws-deletion-completion-clock")
	record := completedRetainWorkspaceDeletionForTest(workspaceID, now)
	record.OwnerSealAcks = record.OwnerSealAcks[:len(record.OwnerSealAcks)-1]
	record.Phase = workspacemodel.WorkspaceDeletionPhaseEventStreamFinalizing
	record.Completed = false
	record.CompletedAt = time.Time{}
	record.Revision--
	current := &workspacemodel.Workspace{ID: workspaceID, State: common.WorkspaceStateActive, Deletion: &record}
	transition, err := reduceWorkspaceDeletionInternalEvent(current, workspaceDeletionInternalEvent{Kind: workspaceDeletionInternalEventOwnerSealObserved, OwnerSeal: &workspaceDeletionOwnerSealFact{
		WorkspaceID: workspaceID, DeletionID: record.DeletionID, OwnerModuleID: "blocks/eventstream", FenceID: "eventstream-fence", Revision: 1, AckHash: "eventstream-ack", OccurredAt: now.Add(-time.Hour),
	}})
	if err != nil {
		t.Fatal(err)
	}
	if transition.Next.Deletion == nil || !transition.Next.Deletion.Completed || !transition.Next.Deletion.UpdatedAt.Equal(now) || !transition.Next.Deletion.CompletedAt.Equal(now) {
		t.Fatalf("completion clock was not clamped: %+v", transition.Next.Deletion)
	}
	last := transition.Next.Deletion.OwnerSealAcks[len(transition.Next.Deletion.OwnerSealAcks)-1]
	if !last.SealedAt.Equal(now) || transition.Next.Deletion.Validate(workspaceID) != nil {
		t.Fatalf("final ack clock=%s deletion=%+v", last.SealedAt, transition.Next.Deletion)
	}
}

func workspaceWithWaitingDeletionForTest(t *testing.T, current *workspacemodel.Workspace, requestID string) *workspacemodel.Workspace {
	t.Helper()
	now := time.Now().UTC()
	requested, err := reduceWorkspaceDeletionInternalEvent(current, workspaceDeletionInternalEvent{Kind: workspaceDeletionInternalEventRequested, Requested: &workspaceDeletionRequestedFact{
		Command: workspaceevents.DeleteWorkspaceCommand{WorkspaceID: current.ID, RequestID: requestID}, OccurredAt: now,
	}})
	if err != nil {
		t.Fatal(err)
	}
	waiting, err := reduceWorkspaceDeletionInternalEvent(requested.Next, workspaceDeletionInternalEvent{Kind: workspaceDeletionInternalEventRunBusyObserved, RunBusy: &workspaceDeletionRunBusyFact{
		WorkspaceID: current.ID, DeletionID: requested.Next.Deletion.DeletionID, OccurredAt: now.Add(time.Second),
	}})
	if err != nil {
		t.Fatal(err)
	}
	return waiting.Next
}
