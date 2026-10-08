package result

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	resultevents "github.com/muidea/workorch/internal/modules/blocks/result/pkg/events"
	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	deletioncontract "github.com/muidea/workorch/internal/pkg/workorchdeletioncontract"
	"github.com/muidea/workorch/internal/pkg/workorchrepo"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestResultPrivilegedIngressRejectsForgedSourcesWithoutOwnerMutation(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	repositories := memory.NewResultOwnerRepositorySet()
	owner, err := New(hub, nil, Dependencies{OwnerRepositories: repositories.OwnerRepositories(), EventHub: hub})
	if err != nil {
		t.Fatal(err)
	}
	terminal := terminalDeliveryFixture("delivery-forged-source", nil)
	terminalEvent := magicevent.NewEventWithContext(
		resultevents.TopicResultFactTerminalDelivery, "blocks/run", resultevents.CommandDestination,
		magicevent.NewHeader(), ctx, terminal,
	)
	if _, err := owner.executeLifecycleCommand(ctx, terminalEvent); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("forged terminal source error = %v, want invalid argument", err)
	}
	if items, listErr := repositories.Results.ListByWorkspace(ctx, terminal.WorkspaceID); listErr != nil || len(items) != 0 {
		t.Fatalf("forged terminal source persisted Results: len=%d err=%v", len(items), listErr)
	}

	scope := deletioncontract.Scope{
		WorkspaceID: terminal.WorkspaceID, DeletionID: "delete-result-source", DeletionGeneration: 1,
		Mode: deletioncontract.ModePurge, QuiescenceID: "quiescence-result-source",
	}
	scope.RequestHash = deletioncontract.StableRequestHash(scope)
	seal := resultevents.SealWorkspaceResultDeletionCommand{
		WorkspaceID: scope.WorkspaceID, DeletionID: scope.DeletionID, DeletionGeneration: scope.DeletionGeneration,
		DataDispositionIntent: scope.Mode, QuiescenceID: scope.QuiescenceID, RequestHash: scope.RequestHash,
	}
	sealEvent := magicevent.NewEventWithContext(
		resultevents.TopicResultCommandSealWorkspaceDeletion, "application/workorchd", resultevents.CommandDestination,
		magicevent.NewHeader(), ctx, seal,
	)
	if _, err := owner.executeWorkspaceDeletionCommand(ctx, sealEvent); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("forged seal source error = %v, want invalid argument", err)
	}
	if _, err := repositories.WorkspaceDeletions.Get(ctx, scope.WorkspaceID); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("forged seal source persisted tombstone: %v", err)
	}
	if _, err := owner.sealWorkspaceDeletion(ctx, scope); err != nil {
		t.Fatal(err)
	}
	purge := resultevents.PurgeWorkspaceResultsCommand{
		WorkspaceID: scope.WorkspaceID, DeletionID: scope.DeletionID, DeletionGeneration: scope.DeletionGeneration,
		QuiescenceID: scope.QuiescenceID, RequestHash: scope.RequestHash,
	}
	purgeEvent := magicevent.NewEventWithContext(
		resultevents.TopicResultCommandPurgeWorkspace, "kernel/orchestrator", resultevents.CommandDestination,
		magicevent.NewHeader(), ctx, purge,
	)
	if _, err := owner.executeWorkspaceDeletionCommand(ctx, purgeEvent); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("forged purge source error = %v, want invalid argument", err)
	}
	tombstone, err := repositories.WorkspaceDeletions.Get(ctx, scope.WorkspaceID)
	if err != nil || tombstone.Phase != resultmodel.WorkspaceDeletionPhaseFenced || tombstone.Revision != 1 {
		t.Fatalf("forged purge source changed tombstone: value=%+v err=%v", tombstone, err)
	}
}

func TestResultFileOwnerReplaysTerminalAndResumesDeletionAfterReopen(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	firstRepositories := workorchrepo.NewWorkspaceScopedFileRepositorySet(root)
	firstHub := magicevent.NewHub(16)
	first, err := New(firstHub, nil, Dependencies{OwnerRepositories: firstRepositories.ResultOwner, EventHub: firstHub})
	if err != nil {
		t.Fatal(err)
	}
	command := terminalDeliveryFixture("delivery-file-reopen", nil)
	created, err := first.applyTerminalDelivery(ctx, command)
	if err != nil || created.Result == nil {
		t.Fatalf("persist terminal Result: result=%+v err=%v", created, err)
	}
	first.Teardown(ctx)
	firstHub.Terminate(ctx)

	secondRepositories := workorchrepo.NewWorkspaceScopedFileRepositorySet(root)
	secondHub := magicevent.NewHub(16)
	const observerID = "/result/consumers/file-reopen-test"
	observer := magicevent.NewSimpleObserverWithMatchID(observerID, observerID, secondHub)
	received := make(chan resultevents.ResultChangedData, 1)
	observer.Subscribe(resultevents.TopicResultAccepted, func(event magicevent.Event, _ magicevent.Result) {
		if data, ok := event.Data().(resultevents.ResultChangedData); ok {
			received <- data
		}
	})
	second, err := New(secondHub, nil, Dependencies{OwnerRepositories: secondRepositories.ResultOwner, EventHub: secondHub})
	if err != nil {
		t.Fatal(err)
	}
	if err := second.Start(); err != nil {
		t.Fatal(err)
	}
	replayed := receiveChangedData(t, received)
	if replayed.ResultID != created.Result.ID || replayed.NotificationID != resultevents.ResultNotificationID(created.Result) {
		t.Fatalf("file terminal replay = %+v, want Result %q", replayed, created.Result.ID)
	}

	scope := deletioncontract.Scope{
		WorkspaceID: command.WorkspaceID, DeletionID: "delete-result-file", DeletionGeneration: 1,
		Mode: deletioncontract.ModePurge, QuiescenceID: "quiescence-result-file",
	}
	scope.RequestHash = deletioncontract.StableRequestHash(scope)
	seal := resultevents.SealWorkspaceResultDeletionCommand{
		WorkspaceID: scope.WorkspaceID, DeletionID: scope.DeletionID, DeletionGeneration: scope.DeletionGeneration,
		DataDispositionIntent: scope.Mode, QuiescenceID: scope.QuiescenceID, RequestHash: scope.RequestHash,
	}
	sealEvent := magicevent.NewEventWithContext(
		resultevents.TopicResultCommandSealWorkspaceDeletion, workspaceDeletionCommandSource, resultevents.CommandDestination,
		magicevent.NewHeader(), ctx, seal,
	)
	sealed, err := second.executeWorkspaceDeletionCommand(ctx, sealEvent)
	if err != nil || sealed.(resultevents.SealWorkspaceResultDeletionResult).ValidateFor(seal) != nil {
		t.Fatalf("seal before reopen = %#v err=%v", sealed, err)
	}
	second.Teardown(ctx)
	observer.Unsubscribe(resultevents.TopicResultAccepted)
	secondHub.Terminate(ctx)

	thirdRepositories := workorchrepo.NewWorkspaceScopedFileRepositorySet(root)
	thirdHub := magicevent.NewHub(16)
	t.Cleanup(func() { thirdHub.Terminate(ctx) })
	third, err := New(thirdHub, nil, Dependencies{OwnerRepositories: thirdRepositories.ResultOwner, EventHub: thirdHub})
	if err != nil {
		t.Fatal(err)
	}
	purge := resultevents.PurgeWorkspaceResultsCommand{
		WorkspaceID: scope.WorkspaceID, DeletionID: scope.DeletionID, DeletionGeneration: scope.DeletionGeneration,
		QuiescenceID: scope.QuiescenceID, RequestHash: scope.RequestHash,
	}
	purgeEvent := magicevent.NewEventWithContext(
		resultevents.TopicResultCommandPurgeWorkspace, workspaceDeletionCommandSource, resultevents.CommandDestination,
		magicevent.NewHeader(), ctx, purge,
	)
	purgedValue, err := third.executeWorkspaceDeletionCommand(ctx, purgeEvent)
	if err != nil {
		t.Fatal(err)
	}
	purged := purgedValue.(resultevents.PurgeWorkspaceResultsResult)
	if err := purged.ValidateFor(purge); err != nil {
		t.Fatalf("purge after reopen: %+v err=%v", purged, err)
	}

	finalRepositories := workorchrepo.NewWorkspaceScopedFileRepositorySet(root)
	items, listErr := finalRepositories.Results.ListByWorkspace(ctx, command.WorkspaceID)
	artifacts, artifactErr := finalRepositories.Artifacts.ListByWorkspace(ctx, command.WorkspaceID)
	tombstone, tombstoneErr := finalRepositories.ResultDeletions.Get(ctx, command.WorkspaceID)
	if listErr != nil || artifactErr != nil || len(items) != 0 || len(artifacts) != 0 || tombstoneErr != nil || tombstone.Phase != resultmodel.WorkspaceDeletionPhasePurged {
		t.Fatalf("file purge reopen items=%d/%d tombstone=%+v errors=%v/%v/%v", len(items), len(artifacts), tombstone, listErr, artifactErr, tombstoneErr)
	}
}

func TestTerminalDeliveryDerivesCanonicalResultAndRejectsMaterialDrift(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	results := memory.NewResultRepository()
	biz, err := New(hub, nil, Dependencies{OwnerRepositories: resultBizTestOwnerRepositories(results, memory.NewArtifactRepository(), memory.NewResultWorkspaceDeletionRepository()), EventHub: hub})
	if err != nil {
		t.Fatal(err)
	}
	command := terminalDeliveryFixture("delivery-material", []common.ArtifactID{"workorch://artifact/report"})

	created, err := biz.applyTerminalDelivery(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	wantID := canonicalRuntimeResultID(command.WorkspaceID, command.RunID)
	if created.Result == nil || created.Result.ID != wantID || created.Result.TerminalMaterialHash == "" || !created.Transitioned {
		t.Fatalf("created result = %+v", created)
	}
	stored, err := results.Get(common.ContextWithRuntimeWorkspaceID(ctx, command.WorkspaceID), wantID)
	if err != nil || stored.State != common.ResultStateAccepted || stored.ID != wantID {
		t.Fatalf("stored result = %+v err=%v", stored, err)
	}

	replayed, err := biz.applyTerminalDelivery(ctx, command)
	if err != nil || replayed.Transitioned || replayed.Result == nil || replayed.Result.ID != wantID {
		t.Fatalf("replayed result = %+v err=%v", replayed, err)
	}
	drifted := command
	drifted.Material.Summary = "changed sealed material"
	if _, err := biz.applyTerminalDelivery(ctx, drifted); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("material drift err = %v, want conflict", err)
	}
	differentIdentity := command
	differentIdentity.TerminalDeliveryID = "delivery-other"
	differentIdentity.CausationID = "terminal-delivery/delivery-other"
	if _, err := biz.applyTerminalDelivery(ctx, differentIdentity); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("second terminal identity err = %v, want conflict", err)
	}
}

func TestTerminalDeliveryRecoversDraftAndPartialArtifactMaterialization(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	results := memory.NewResultRepository()
	artifacts := memory.NewArtifactRepository()
	biz, err := New(hub, nil, Dependencies{OwnerRepositories: resultBizTestOwnerRepositories(results, artifacts, memory.NewResultWorkspaceDeletionRepository()), EventHub: hub})
	if err != nil {
		t.Fatal(err)
	}
	command := terminalDeliveryFixture("delivery-recovery", []common.ArtifactID{"artifact/source-a", "artifact/source-b"})
	material := command.Material.Normalized()
	hash, err := resultevents.CanonicalResultMaterialHash(material)
	if err != nil {
		t.Fatal(err)
	}
	resultID := canonicalRuntimeResultID(command.WorkspaceID, command.RunID)
	expectedArtifacts := terminalArtifacts(command, resultID, material.ArtifactRefs)
	artifactIDs := []common.ArtifactID{expectedArtifacts[0].ID, expectedArtifacts[1].ID}
	pending := resultFromTerminalMaterial(material, resultID, artifactIDs, hash)
	identity := command.Identity()
	pending.TerminalDelivery = &identity
	pending.State = common.ResultStateDraft
	pending.CreatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	pending.UpdatedAt = pending.CreatedAt
	scoped := common.ContextWithRuntimeWorkspaceID(ctx, command.WorkspaceID)
	if err := results.Create(scoped, pending); err != nil {
		t.Fatal(err)
	}
	if err := artifacts.Create(scoped, expectedArtifacts[0]); err != nil {
		t.Fatal(err)
	}

	completed, err := biz.applyTerminalDelivery(ctx, command)
	if err != nil || completed.Result == nil || completed.Result.State != common.ResultStateAccepted || !completed.Transitioned {
		t.Fatalf("recovered result = %+v err=%v", completed, err)
	}
	children, err := artifacts.ListByResult(scoped, resultID)
	if err != nil || len(children) != 2 {
		t.Fatalf("recovered artifacts = %+v err=%v", children, err)
	}
	for _, child := range children {
		if child.WorkspaceID != command.WorkspaceID || child.RunID != command.RunID || child.ResultID != resultID || child.SourceRef == "" || child.MaterialHash == "" {
			t.Fatalf("invalid child artifact = %+v", child)
		}
	}
}

func TestTerminalDeliveryPreservesFormalCloseoutArtifactHashAndMapsChildProjection(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	results := memory.NewResultRepository()
	artifacts := memory.NewArtifactRepository()
	biz, err := New(hub, nil, Dependencies{OwnerRepositories: resultBizTestOwnerRepositories(results, artifacts, memory.NewResultWorkspaceDeletionRepository()), EventHub: hub})
	if err != nil {
		t.Fatal(err)
	}
	const sourceRef common.ArtifactID = "workorch://artifact/formal-report"
	command := terminalDeliveryFixture("delivery-formal-closeout", []common.ArtifactID{sourceRef})
	revisions := (runtimecontract.RuntimeOwnerRevisionSet{DefinitionRevision: command.DefinitionRevision, Entries: []runtimecontract.RuntimeOwnerRevisionRef{{
		OwnerKind: "capability", OwnerInstanceID: "blocks/file", ResourceKind: "workspace_tree", ResourceID: string(command.WorkspaceID), Revision: "tree-2", DependencyHash: "dependency-hash-1",
	}}}).WithHash()
	closeout := runtimecontract.RuntimeResultCloseout{
		SchemaVersion: runtimecontract.RuntimeResultCloseoutSchemaVersionCurrent,
		CloseoutID:    "closeout-formal-artifact", StartupInputHash: "startup-formal-artifact", DefinitionRevision: command.DefinitionRevision, RuntimeStateRevision: 7,
		TerminalOutcome: command.Outcome,
		RequirementOutcomes: []runtimecontract.RuntimeRequirementOutcome{{
			RequirementRef: "requirement-1", SourceItemRef: "dod-item-1", CoverageWorkItemID: "cwi-1", DefinitionRevision: command.DefinitionRevision,
			CoverageState: runtimecontract.RuntimeRequirementCovered, AssessmentState: runtimecontract.RuntimeRequirementSatisfied,
			EvidenceItems:        []runtimecontract.RuntimeRequirementEvidenceItem{{EvidenceRef: "evidence-1", BindingRef: "binding-1", State: runtimecontract.RuntimeRequirementEvidenceAccepted, OwnerRevisionSetHash: revisions.SetHash}},
			AcceptedEvidenceRefs: []string{"evidence-1"}, OwnerRevisionSetHash: revisions.SetHash,
		}},
		AcceptedEvidenceRefs: []string{"evidence-1"}, OwnerRevisionSet: revisions, ArtifactRefs: []common.ArtifactID{sourceRef},
		RecoveryPolicy: runtimecontract.DefaultRuntimeRecoveryPolicy(),
	}
	closeout = closeout.WithCloseoutHash()
	sealedHash := closeout.CloseoutHash
	command.Material.RuntimeCloseout = &closeout
	drifted := command
	driftedCloseout := closeout.Normalized()
	driftedCloseout.TerminalOutcome = runtimecontract.TerminalOutcome{Kind: runtimecontract.TerminalOutcomeKindFailed, Code: "failed", Summary: "different terminal authority"}
	driftedCloseout = driftedCloseout.WithCloseoutHash()
	drifted.Material.RuntimeCloseout = &driftedCloseout
	if _, err := biz.applyTerminalDelivery(ctx, drifted); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("mismatched formal closeout terminal err=%v, want conflict", err)
	}

	created, err := biz.applyTerminalDelivery(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	if created.Result == nil || created.Result.RuntimeCloseout == nil || created.Result.RuntimeCloseout.CloseoutHash != sealedHash ||
		len(created.Result.RuntimeCloseout.ArtifactRefs) != 1 || created.Result.RuntimeCloseout.ArtifactRefs[0] != sourceRef ||
		len(created.Result.ArtifactRefs) != 1 || created.Result.ArtifactRefs[0] == sourceRef {
		t.Fatalf("formal closeout/projected artifacts = %+v", created.Result)
	}
	if err := created.Result.RuntimeCloseout.Validate(); err != nil {
		t.Fatalf("stored formal closeout no longer validates: %v", err)
	}
	if created.DeliveryAck == nil || created.DeliveryAck.ResultCloseout == nil ||
		!reflect.DeepEqual(*created.DeliveryAck.ResultCloseout, *created.Result.RuntimeCloseout) {
		t.Fatalf("Result 回执未携带实际持久化的完整 closeout: %+v", created.DeliveryAck)
	}
	created.DeliveryAck.ResultCloseout.RequirementOutcomes[0].AcceptedEvidenceRefs[0] = "caller-tampered"
	stored, err := results.Get(common.ContextWithRuntimeWorkspaceID(ctx, command.WorkspaceID), created.Result.ID)
	if err != nil || stored.RuntimeCloseout == nil || stored.RuntimeCloseout.CloseoutHash != sealedHash ||
		stored.RuntimeCloseout.RequirementOutcomes[0].AcceptedEvidenceRefs[0] != "evidence-1" {
		t.Fatalf("回执别名污染了 Result owner 存储: stored=%+v err=%v", stored, err)
	}
	replayed, err := biz.applyTerminalDelivery(ctx, command)
	if err != nil || replayed.DeliveryAck == nil || replayed.DeliveryAck.ResultCloseout == nil ||
		replayed.DeliveryAck.AcknowledgedState != runtimecontract.RuntimeDeliveryAcknowledgedNoop ||
		!reflect.DeepEqual(*replayed.DeliveryAck.ResultCloseout, *stored.RuntimeCloseout) {
		t.Fatalf("noop 回执未重放相同持久材料: %+v err=%v", replayed, err)
	}
	children, err := artifacts.ListByResult(common.ContextWithRuntimeWorkspaceID(ctx, command.WorkspaceID), created.Result.ID)
	if err != nil || len(children) != 1 || children[0].SourceRef != string(sourceRef) || children[0].ID != created.Result.ArtifactRefs[0] {
		t.Fatalf("formal closeout artifact child = %+v err=%v", children, err)
	}
}

func TestTerminalDeliveryReplayReemitsStableNotification(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	biz, err := New(hub, nil, Dependencies{OwnerRepositories: memory.NewResultOwnerRepositorySet().OwnerRepositories(), EventHub: hub})
	if err != nil {
		t.Fatal(err)
	}
	const observerID = "/result/consumers/result-replay-test"
	observer := magicevent.NewSimpleObserverWithMatchID(observerID, observerID, hub)
	received := make(chan resultevents.ResultChangedData, 2)
	observer.Subscribe(resultevents.TopicResultAccepted, func(event magicevent.Event, _ magicevent.Result) {
		if data, ok := event.Data().(resultevents.ResultChangedData); ok {
			received <- data
		}
	})
	t.Cleanup(func() { observer.Unsubscribe(resultevents.TopicResultAccepted) })
	command := terminalDeliveryFixture("delivery-notification", nil)
	if _, err := biz.applyTerminalDelivery(ctx, command); err != nil {
		t.Fatal(err)
	}
	if _, err := biz.applyTerminalDelivery(ctx, command); err != nil {
		t.Fatal(err)
	}
	first := receiveChangedData(t, received)
	second := receiveChangedData(t, received)
	if first.NotificationID == "" || first.NotificationID != second.NotificationID || first.TerminalMaterialHash != second.TerminalMaterialHash {
		t.Fatalf("notification identities differ: first=%+v second=%+v", first, second)
	}
}

func TestResultOwnerStartReplaysDurableTerminalNotification(t *testing.T) {
	ctx := context.Background()
	ownerSet := memory.NewResultOwnerRepositorySet()
	firstHub := magicevent.NewHub(16)
	ownerRepositories := ownerSet.OwnerRepositories()
	first, err := New(firstHub, nil, Dependencies{OwnerRepositories: ownerRepositories, EventHub: firstHub})
	if err != nil {
		t.Fatal(err)
	}
	command := terminalDeliveryFixture("delivery-startup-replay", nil)
	created, err := first.applyTerminalDelivery(ctx, command)
	if err != nil || created.Result == nil {
		t.Fatalf("persist terminal result: %+v err=%v", created, err)
	}
	firstHub.Terminate(ctx)

	restartedHub := magicevent.NewHub(16)
	t.Cleanup(func() { restartedHub.Terminate(ctx) })
	const observerID = "/result/consumers/result-startup-replay-test"
	observer := magicevent.NewSimpleObserverWithMatchID(observerID, observerID, restartedHub)
	received := make(chan resultevents.ResultChangedData, 1)
	observer.Subscribe(resultevents.TopicResultAccepted, func(event magicevent.Event, _ magicevent.Result) {
		if data, ok := event.Data().(resultevents.ResultChangedData); ok {
			received <- data
		}
	})
	t.Cleanup(func() { observer.Unsubscribe(resultevents.TopicResultAccepted) })
	restarted, err := New(restartedHub, nil, Dependencies{OwnerRepositories: ownerRepositories, EventHub: restartedHub})
	if err != nil {
		t.Fatal(err)
	}
	if err := restarted.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { restarted.Teardown(ctx) })

	replayed := receiveChangedData(t, received)
	if replayed.ResultID != created.Result.ID || replayed.NotificationID != resultevents.ResultNotificationID(created.Result) {
		t.Fatalf("startup replay = %+v, want result %q", replayed, created.Result.ID)
	}
}

func receiveChangedData(t *testing.T, values <-chan resultevents.ResultChangedData) resultevents.ResultChangedData {
	t.Helper()
	select {
	case value := <-values:
		return value
	case <-time.After(time.Second):
		t.Fatal("result notification was not delivered")
		return resultevents.ResultChangedData{}
	}
}

type resultBizTestBoundResults struct {
	repository.ResultRepository
	binding repository.OwnerRepositoryBinding
}

func (r resultBizTestBoundResults) OwnerRepositoryBinding() repository.OwnerRepositoryBinding {
	return r.binding
}

type resultBizTestBoundArtifacts struct {
	repository.ArtifactRepository
	binding repository.OwnerRepositoryBinding
}

func (r resultBizTestBoundArtifacts) OwnerRepositoryBinding() repository.OwnerRepositoryBinding {
	return r.binding
}

type resultBizTestBoundDeletions struct {
	repository.ResultWorkspaceDeletionRepository
	binding repository.OwnerRepositoryBinding
}

func (r resultBizTestBoundDeletions) OwnerRepositoryBinding() repository.OwnerRepositoryBinding {
	return r.binding
}

type resultBizTestBackend struct {
	binding   repository.OwnerRepositoryBinding
	results   repository.ResultRepository
	artifacts repository.ArtifactRepository
	deletions repository.ResultWorkspaceDeletionRepository
}

func (b resultBizTestBackend) ResultOwnerBinding() repository.OwnerRepositoryBinding {
	return b.binding
}
func (b resultBizTestBackend) ResultOwnerResults() repository.ResultRepository { return b.results }
func (b resultBizTestBackend) ResultOwnerArtifacts() repository.ArtifactRepository {
	return b.artifacts
}
func (b resultBizTestBackend) ResultOwnerWorkspaceDeletions() repository.ResultWorkspaceDeletionRepository {
	return b.deletions
}

func resultBizTestOwnerRepositories(results repository.ResultRepository, artifacts repository.ArtifactRepository, deletions repository.ResultWorkspaceDeletionRepository) repository.ResultOwnerRepositories {
	binding := repository.NewResultOwnerRepositoryBinding()
	owner, err := repository.NewResultOwnerRepositories(resultBizTestBackend{
		binding:   binding,
		results:   resultBizTestBoundResults{ResultRepository: results, binding: binding},
		artifacts: resultBizTestBoundArtifacts{ArtifactRepository: artifacts, binding: binding},
		deletions: resultBizTestBoundDeletions{ResultWorkspaceDeletionRepository: deletions, binding: binding},
	})
	if err != nil {
		panic(err)
	}
	return owner
}

func terminalDeliveryFixture(deliveryID string, artifactRefs []common.ArtifactID) resultevents.RuntimeTerminalDeliveryFact {
	return resultevents.RuntimeTerminalDeliveryFact{
		RuntimeTerminalDeliveryFact: runtimecontract.RuntimeTerminalDeliveryFact{
			RuntimeTerminalDeliveryIdentity: runtimecontract.RuntimeTerminalDeliveryIdentity{
				WorkspaceID: "ws-result-lifecycle", RunID: "run-result-lifecycle", TerminalDeliveryID: deliveryID,
				AuthoritySnapshotID: "authority-result", DefinitionRevision: "definition-result",
				EvidenceRevision: "evidence-result", DispatchID: "dispatch-result",
				Outcome: runtimecontract.TerminalOutcome{Kind: runtimecontract.TerminalOutcomeKindCompleted, Code: "completed", Summary: "done"},
			},
			SourceOwner: "kernel/orchestrator", CausationID: "terminal-delivery/" + deliveryID,
		},
		Material: resultevents.ResultMaterial{
			WorkspaceID: "ws-result-lifecycle", RunID: "run-result-lifecycle", GoalID: "goal-result", DoDID: "dod-result",
			Summary: "done", ArtifactRefs: artifactRefs,
		},
	}
}
