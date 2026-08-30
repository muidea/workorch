package workspace

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	approvalevents "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	eventstreamevents "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/events"
	memoryevents "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/events"
	resultevents "github.com/muidea/workorch/internal/modules/blocks/result/pkg/events"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	filerepo "github.com/muidea/workorch/internal/pkg/repository/file"
	memoryrepo "github.com/muidea/workorch/internal/pkg/repository/memory"
	deletioncontract "github.com/muidea/workorch/internal/pkg/workorchdeletioncontract"
)

func TestWorkspaceDeletionCoordinatorUsesExactRetainAndPurgeOrder(t *testing.T) {
	for _, tc := range []struct {
		name     string
		purge    bool
		expected []string
	}{
		{
			name: "retain",
			expected: []string{
				runevents.TopicRunCommandPrepareWorkspaceRetirement,
				orchestratorevents.TopicOrchestratorCommandBeginWorkspaceRuntimeQuiescence,
				orchestratorevents.TopicOrchestratorCommandConfirmWorkspaceRuntimeQuiescence,
				runevents.TopicRunCommandFinalizeWorkspaceRetirement,
				orchestratorevents.TopicOrchestratorCommandSealWorkspaceDeletion,
				resultevents.TopicResultCommandSealWorkspaceDeletion,
				memoryevents.TopicMemoryCommandSealWorkspaceDeletion,
				decisionevents.TopicDecisionCommandSealWorkspaceDeletion,
				approvalevents.TopicApprovalCommandSealWorkspaceDeletion,
				eventstreamevents.TopicEventStreamCommandSealWorkspaceDeletion,
			},
		},
		{
			name: "purge", purge: true,
			expected: []string{
				runevents.TopicRunCommandPrepareWorkspaceRetirement,
				orchestratorevents.TopicOrchestratorCommandBeginWorkspaceRuntimeQuiescence,
				orchestratorevents.TopicOrchestratorCommandConfirmWorkspaceRuntimeQuiescence,
				runevents.TopicRunCommandFinalizeWorkspaceRetirement,
				orchestratorevents.TopicOrchestratorCommandSealWorkspaceDeletion,
				resultevents.TopicResultCommandSealWorkspaceDeletion,
				memoryevents.TopicMemoryCommandSealWorkspaceDeletion,
				decisionevents.TopicDecisionCommandSealWorkspaceDeletion,
				approvalevents.TopicApprovalCommandSealWorkspaceDeletion,
				orchestratorevents.TopicOrchestratorCommandPurgeWorkspaceEvidence,
				resultevents.TopicResultCommandPurgeWorkspace,
				memoryevents.TopicMemoryCommandPurgeWorkspace,
				decisionevents.TopicDecisionCommandPurgeWorkspace,
				approvalevents.TopicApprovalCommandPurgeWorkspace,
				runevents.TopicRunCommandPurgeWorkspace,
				eventstreamevents.TopicEventStreamCommandSealWorkspaceDeletion,
				eventstreamevents.TopicEventStreamCommandPurgeWorkspace,
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			owner, workspaceRepo, sequence, workspaceID := newWorkspaceDeletionCoordinatorFixture(t, tc.purge)
			receipts := make(chan workspaceevents.WorkspaceDeletedData, 1)
			observerID := "/workspace/consumers/blocks/workspace/deletion-receipt-test/" + tc.name
			observer := magicevent.NewSimpleObserverWithMatchID(observerID, observerID, owner.EventHub())
			observer.Subscribe(workspaceevents.TopicWorkspaceDeleted, func(event magicevent.Event, _ magicevent.Result) {
				if data, ok := event.Data().(workspaceevents.WorkspaceDeletedData); ok {
					receipts <- data
				}
			})
			t.Cleanup(func() { observer.Unsubscribe(workspaceevents.TopicWorkspaceDeleted) })
			command := workspaceevents.DeleteWorkspaceCommand{WorkspaceID: workspaceID, RequestID: "request-" + tc.name, Purge: tc.purge}
			if _, err := owner.submitWorkspaceDeletionInternalEvent(context.Background(), workspaceDeletionInternalEvent{Kind: workspaceDeletionInternalEventRequested, Requested: &workspaceDeletionRequestedFact{Command: command, OccurredAt: time.Now().UTC()}}); err != nil {
				t.Fatal(err)
			}
			if _, err := owner.driveWorkspaceDeletion(context.Background(), workspaceID); err != nil {
				t.Fatalf("drive err=%v", err)
			}
			if !reflect.DeepEqual(*sequence, tc.expected) {
				t.Fatalf("command order=\n%v\nwant=\n%v", *sequence, tc.expected)
			}
			current, err := workspaceRepo.Get(context.Background(), workspaceID)
			if err != nil {
				t.Fatal(err)
			}
			if current.Deletion == nil || current.Deletion.Phase != workspacemodel.WorkspaceDeletionPhaseCompleted || !current.Deletion.Completed {
				t.Fatalf("deletion=%+v, want completed", current.Deletion)
			}
			select {
			case receipt := <-receipts:
				if err := receipt.Validate(); err != nil {
					t.Fatalf("deleted receipt validation: %v", err)
				}
				if receipt.WorkspaceID != workspaceID || receipt.DeletionID != current.Deletion.DeletionID || receipt.RequestID != command.RequestID || receipt.Mode != command.Mode() || receipt.Disposition != workspaceevents.DeleteWorkspaceCompleted || receipt.DeletionRevision != current.Deletion.Revision || receipt.WorkspaceRevision != current.Revision || !receipt.CompletedAt.Equal(current.Deletion.CompletedAt) {
					t.Fatalf("deleted receipt=%+v, workspace deletion=%+v revision=%d", receipt, current.Deletion, current.Revision)
				}
			case <-time.After(time.Second):
				t.Fatal("completed deletion receipt was not delivered")
			}
			if tc.purge {
				if len(current.Deletion.OwnerSealAcks) != 6 || len(current.Deletion.OwnerPurgeAcks) != 7 || current.Deletion.DirectoryCleanupAck == nil {
					t.Fatalf("purge progress=%+v, want exact completed owner acknowledgements and cleanup", current.Deletion)
				}
			} else if len(current.Deletion.OwnerSealAcks) != 6 || len(current.Deletion.OwnerPurgeAcks) != 0 || current.Deletion.DirectoryCleanupAck != nil {
				t.Fatalf("retain progress=%+v, want exact completed seal acknowledgements only", current.Deletion)
			}
		})
	}
}

func TestWorkspaceDeletionRejectsNonApplicationSourceWithoutIntentOrRetirementCommand(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	t.Cleanup(func() { hub.Terminate(context.Background()) })
	prepareCalls := 0
	observer := magicevent.NewSimpleObserver(runevents.CommandDestination, hub)
	observer.Subscribe(runevents.TopicRunCommandPrepareWorkspaceRetirement, func(_ magicevent.Event, result magicevent.Result) {
		prepareCalls++
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: "must not be called", Cause: common.ErrConflict})
	})
	workspaceRepo := memoryrepo.NewWorkspaceRepository()
	owner, err := New(hub, nil, workspaceOwnerTestDependencies(hub, nil, workspaceRepo, workspaceRepo, memoryrepo.NewGoalRepository(), memoryrepo.NewDoDRepository(), memoryrepo.NewWorkspaceTemplateRepository()))
	if err != nil {
		t.Fatal(err)
	}
	workspaceID := common.WorkspaceID("ws-delete-forged-source")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "source", State: common.WorkspaceStateActive}); err != nil {
		t.Fatal(err)
	}
	command := workspaceevents.DeleteWorkspaceCommand{WorkspaceID: workspaceID, RequestID: "request-source"}
	for _, source := range []string{"blocks/run", "kernel/orchestrator", "blocks/workspace"} {
		event := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceCommandDelete, source, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, command)
		if _, err := owner.deleteWorkspace(ctx, event); !errors.Is(err, common.ErrInvalidArgument) {
			t.Fatalf("source %q delete err=%v, want invalid argument", source, err)
		}
	}
	current, err := workspaceRepo.Get(ctx, workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Deletion != nil || prepareCalls != 0 {
		t.Fatalf("forged-source deletion=%+v prepareCalls=%d, want zero intent/command", current.Deletion, prepareCalls)
	}
}

func TestWorkspaceDeletionResponseLossResumesFromPersistedPhaseOnExactDeleteRetry(t *testing.T) {
	ctx := context.Background()
	owner, workspaceRepo, sequence, workspaceID := newWorkspaceDeletionCoordinatorFixture(t, false, eventstreamevents.TopicEventStreamCommandSealWorkspaceDeletion)
	command := workspaceevents.DeleteWorkspaceCommand{WorkspaceID: workspaceID, RequestID: "request-response-loss"}
	request := workspaceDeletionInternalEvent{Kind: workspaceDeletionInternalEventRequested, Requested: &workspaceDeletionRequestedFact{Command: command, OccurredAt: time.Now().UTC()}}
	if _, err := owner.submitWorkspaceDeletionInternalEvent(ctx, request); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.driveWorkspaceDeletion(ctx, workspaceID); err == nil {
		t.Fatal("first drive succeeded despite injected Eventstream response loss")
	}
	interrupted, err := workspaceRepo.Get(ctx, workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if interrupted.Deletion == nil || interrupted.Deletion.Phase != workspacemodel.WorkspaceDeletionPhaseEventStreamFinalizing || len(interrupted.Deletion.OwnerSealAcks) != 5 {
		t.Fatalf("interrupted deletion=%+v, want persisted finalization cut", interrupted.Deletion)
	}
	interruptedRevision := interrupted.Deletion.Revision
	event := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceCommandDelete, workspaceDeletionIngressSource, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	receipt, err := owner.deleteWorkspace(ctx, event)
	if err != nil {
		t.Fatalf("exact retry: %v", err)
	}
	if receipt.Disposition != workspaceevents.DeleteWorkspaceDuplicate || !receipt.Completed {
		t.Fatalf("retry receipt=%+v, want completed duplicate", receipt)
	}
	completed, err := workspaceRepo.Get(ctx, workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if completed.Deletion == nil || !completed.Deletion.Completed || completed.Deletion.DeletionID != interrupted.Deletion.DeletionID || completed.Deletion.Revision != interruptedRevision+1 {
		t.Fatalf("completed deletion=%+v, want same journal with one final transition", completed.Deletion)
	}
	sealAttempts := 0
	for _, topic := range *sequence {
		if topic == eventstreamevents.TopicEventStreamCommandSealWorkspaceDeletion {
			sealAttempts++
		}
	}
	if sealAttempts != 2 {
		t.Fatalf("Eventstream seal attempts=%d sequence=%v, want exact re-drive after response loss", sealAttempts, *sequence)
	}
}

func TestWorkspaceDeletionFreshOwnerStartRecoversPersistedFileFinalization(t *testing.T) {
	ctx := context.Background()
	original, memoryWorkspaceRepo, _, workspaceID := newWorkspaceDeletionCoordinatorFixture(t, false, eventstreamevents.TopicEventStreamCommandSealWorkspaceDeletion)
	command := workspaceevents.DeleteWorkspaceCommand{WorkspaceID: workspaceID, RequestID: "request-fresh-owner"}
	if _, err := original.submitWorkspaceDeletionInternalEvent(ctx, workspaceDeletionInternalEvent{Kind: workspaceDeletionInternalEventRequested, Requested: &workspaceDeletionRequestedFact{Command: command, OccurredAt: time.Now().UTC()}}); err != nil {
		t.Fatal(err)
	}
	if _, err := original.driveWorkspaceDeletion(ctx, workspaceID); err == nil {
		t.Fatal("first drive succeeded despite injected Eventstream response loss")
	}
	interrupted, err := memoryWorkspaceRepo.Get(ctx, workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if interrupted.Deletion == nil || interrupted.Deletion.Phase != workspacemodel.WorkspaceDeletionPhaseEventStreamFinalizing {
		t.Fatalf("interrupted deletion=%+v, want finalization cut", interrupted.Deletion)
	}

	dir := t.TempDir()
	workspacePath := filepath.Join(dir, "workspaces.json")
	if err := filerepo.NewWorkspaceRepository(workspacePath).Create(ctx, interrupted); err != nil {
		t.Fatal(err)
	}
	hub := original.EventHub()
	inventory := magicevent.NewSimpleObserver(runevents.CommandDestination, hub)
	inventory.Subscribe(runevents.TopicRunCommandLifecycleFactInventory, func(event magicevent.Event, result magicevent.Result) {
		if _, ok := event.Data().(runevents.QueryRunLifecycleFactInventoryCommand); !ok {
			result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: common.ErrInvalidArgument.Error(), Cause: common.ErrInvalidArgument})
			return
		}
		result.Set(runevents.QueryRunLifecycleFactInventoryResult{}, nil)
	})
	t.Cleanup(func() { inventory.Unsubscribe(runevents.TopicRunCommandLifecycleFactInventory) })
	freshWorkspaceRepo := filerepo.NewWorkspaceRepository(workspacePath)
	fresh, err := New(hub, nil, workspaceOwnerTestDependencies(
		hub,
		nil,
		freshWorkspaceRepo,
		freshWorkspaceRepo,
		filerepo.NewGoalRepository(filepath.Join(dir, "goals.json")),
		filerepo.NewDoDRepository(filepath.Join(dir, "dods.json")),
		filerepo.NewWorkspaceTemplateRepository(filepath.Join(dir, "templates.json")),
	))
	if err != nil {
		t.Fatal(err)
	}
	if err := fresh.Start(); err != nil {
		t.Fatalf("fresh owner Start: %v", err)
	}
	t.Cleanup(func() { fresh.Teardown(context.Background()) })
	recovered, err := filerepo.NewWorkspaceRepository(workspacePath).Get(ctx, workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Deletion == nil || !recovered.Deletion.Completed || recovered.Deletion.DeletionID != interrupted.Deletion.DeletionID || len(recovered.Deletion.OwnerSealAcks) != 6 {
		t.Fatalf("fresh-owner recovery deletion=%+v", recovered.Deletion)
	}
}

func TestWorkspaceDeletionStartupKeepsDependencyFailurePendingForHeartbeatRetry(t *testing.T) {
	ctx := context.Background()
	owner, workspaceRepo, _, workspaceID := newWorkspaceDeletionCoordinatorFixture(t, false, runevents.TopicRunCommandPrepareWorkspaceRetirement)
	command := workspaceevents.DeleteWorkspaceCommand{WorkspaceID: workspaceID, RequestID: "request-startup-dependency"}
	if _, err := owner.submitWorkspaceDeletionInternalEvent(ctx, workspaceDeletionInternalEvent{Kind: workspaceDeletionInternalEventRequested, Requested: &workspaceDeletionRequestedFact{Command: command, OccurredAt: time.Now().UTC()}}); err != nil {
		t.Fatal(err)
	}
	inventory := magicevent.NewSimpleObserver(runevents.CommandDestination, owner.EventHub())
	inventory.Subscribe(runevents.TopicRunCommandLifecycleFactInventory, func(event magicevent.Event, result magicevent.Result) {
		if _, ok := event.Data().(runevents.QueryRunLifecycleFactInventoryCommand); !ok {
			result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: common.ErrInvalidArgument.Error(), Cause: common.ErrInvalidArgument})
			return
		}
		result.Set(runevents.QueryRunLifecycleFactInventoryResult{}, nil)
	})
	t.Cleanup(func() { inventory.Unsubscribe(runevents.TopicRunCommandLifecycleFactInventory) })
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { owner.Teardown(context.Background()) })
	pending, err := workspaceRepo.Get(ctx, workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if pending.Deletion == nil || pending.Deletion.Phase != workspacemodel.WorkspaceDeletionPhaseRequested || pending.Deletion.Revision != 1 {
		t.Fatalf("startup failure did not retain requested journal: %+v", pending.Deletion)
	}
	owner.reconcileWorkspaceDeletionInventory(ctx)
	completed, err := workspaceRepo.Get(ctx, workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if completed.Deletion == nil || !completed.Deletion.Completed {
		t.Fatalf("heartbeat retry did not complete pending journal: %+v", completed.Deletion)
	}
}

func TestSafeWorkspaceDeletionDeliverablePathRejectsProtectedAndInternalOverlap(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for name, path := range map[string]string{
		"root":      string(filepath.Separator),
		"home":      home,
		"workdir":   wd,
		"temp-root": os.TempDir(),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := safeWorkspaceDeletionDeliverablePath(path, ""); !errors.Is(err, common.ErrInvalidArgument) {
				t.Fatalf("path=%q err=%v, want invalid argument", path, err)
			}
		})
	}
	root := t.TempDir()
	internal := filepath.Join(root, "internal")
	for name, deliverable := range map[string]string{
		"exact":      internal,
		"ancestor":   root,
		"descendant": filepath.Join(internal, "deliverable"),
	} {
		t.Run("internal-"+name, func(t *testing.T) {
			if _, err := safeWorkspaceDeletionDeliverablePath(deliverable, internal); !errors.Is(err, common.ErrConflict) {
				t.Fatalf("deliverable=%q internal=%q err=%v, want conflict", deliverable, internal, err)
			}
		})
	}
	normal := filepath.Join(root, "deliverable")
	got, err := safeWorkspaceDeletionDeliverablePath(normal, internal)
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.Abs(normal)
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Clean(want) {
		t.Fatalf("normal path=%q, want %q", got, filepath.Clean(want))
	}
}

func TestWorkspaceDeletionCoordinatorBusyRetryDoesNotBurnRevision(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	t.Cleanup(func() { hub.Terminate(context.Background()) })
	observer := magicevent.NewSimpleObserver(runevents.CommandDestination, hub)
	observer.Subscribe(runevents.TopicRunCommandPrepareWorkspaceRetirement, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: runevents.ErrWorkspaceRetirementBusy.Error(), Cause: runevents.ErrWorkspaceRetirementBusy})
	})
	workspaceRepo := memoryrepo.NewWorkspaceRepository()
	owner, err := New(hub, nil, workspaceOwnerTestDependencies(hub, nil, workspaceRepo, workspaceRepo, memoryrepo.NewGoalRepository(), memoryrepo.NewDoDRepository(), memoryrepo.NewWorkspaceTemplateRepository()))
	if err != nil {
		t.Fatal(err)
	}
	workspaceID := common.WorkspaceID("ws-deletion-busy-stable")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "busy", State: common.WorkspaceStateActive}); err != nil {
		t.Fatal(err)
	}
	command := workspaceevents.DeleteWorkspaceCommand{WorkspaceID: workspaceID, RequestID: "request-busy"}
	request := workspaceDeletionInternalEvent{Kind: workspaceDeletionInternalEventRequested, Requested: &workspaceDeletionRequestedFact{Command: command, OccurredAt: time.Now().UTC()}}
	if _, err := owner.submitWorkspaceDeletionInternalEvent(ctx, request); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.driveWorkspaceDeletion(ctx, workspaceID); !errors.Is(err, runevents.ErrWorkspaceRetirementBusy) {
		t.Fatalf("first drive err=%v, want busy", err)
	}
	stable, err := workspaceRepo.Get(ctx, workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if stable.Deletion == nil || stable.Deletion.Phase != workspacemodel.WorkspaceDeletionPhaseWaitingForIdle {
		t.Fatalf("deletion=%+v, want waiting", stable.Deletion)
	}
	workspaceRevision := stable.Revision
	deletionRevision := stable.Deletion.Revision
	for attempt := 0; attempt < 20; attempt++ {
		request.Requested.OccurredAt = time.Now().UTC()
		if transition, err := owner.submitWorkspaceDeletionInternalEvent(ctx, request); err != nil || !transition.Noop {
			t.Fatalf("attempt %d retry transition=%+v err=%v", attempt, transition, err)
		}
		if _, err := owner.driveWorkspaceDeletion(ctx, workspaceID); !errors.Is(err, runevents.ErrWorkspaceRetirementBusy) {
			t.Fatalf("attempt %d drive err=%v, want busy", attempt, err)
		}
	}
	current, err := workspaceRepo.Get(ctx, workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Revision != workspaceRevision || current.Deletion.Revision != deletionRevision || current.Deletion.Phase != workspacemodel.WorkspaceDeletionPhaseWaitingForIdle {
		t.Fatalf("revision changed across busy retries: workspace %d->%d deletion %d->%d phase=%s", workspaceRevision, current.Revision, deletionRevision, current.Deletion.Revision, current.Deletion.Phase)
	}
}

func TestWorkspaceDeletionWithActiveRunCreatesNoIntentOrRetirementCommand(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	t.Cleanup(func() { hub.Terminate(context.Background()) })
	prepareCalls := 0
	observer := magicevent.NewSimpleObserver(runevents.CommandDestination, hub)
	observer.Subscribe(runevents.TopicRunCommandPrepareWorkspaceRetirement, func(_ magicevent.Event, result magicevent.Result) {
		prepareCalls++
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: "must not be called", Cause: common.ErrConflict})
	})
	workspaceRepo := memoryrepo.NewWorkspaceRepository()
	owner, err := New(hub, nil, workspaceOwnerTestDependencies(hub, nil, workspaceRepo, workspaceRepo, memoryrepo.NewGoalRepository(), memoryrepo.NewDoDRepository(), memoryrepo.NewWorkspaceTemplateRepository()))
	if err != nil {
		t.Fatal(err)
	}
	workspaceID := common.WorkspaceID("ws-delete-active-no-intent")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "active", State: common.WorkspaceStateActive, ActiveRunID: "run-active"}); err != nil {
		t.Fatal(err)
	}
	command := workspaceevents.DeleteWorkspaceCommand{WorkspaceID: workspaceID, RequestID: "request-active"}
	event := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceCommandDelete, "application/workorchd", workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	if _, err := owner.deleteWorkspace(ctx, event); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("delete err=%v, want conflict", err)
	}
	current, err := workspaceRepo.Get(ctx, workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Deletion != nil || prepareCalls != 0 {
		t.Fatalf("active deletion=%+v prepareCalls=%d, want zero intent/command", current.Deletion, prepareCalls)
	}
}

func newWorkspaceDeletionCoordinatorFixture(t *testing.T, purge bool, failOnceTopics ...string) (*Biz, *memoryrepo.WorkspaceRepository, *[]string, common.WorkspaceID) {
	t.Helper()
	hub := magicevent.NewHub(128)
	t.Cleanup(func() { hub.Terminate(context.Background()) })
	workspaceRepo := memoryrepo.NewWorkspaceRepository()
	goalRepo := memoryrepo.NewGoalRepository()
	dodRepo := memoryrepo.NewDoDRepository()
	owner, err := New(hub, nil, workspaceOwnerTestDependencies(hub, nil, workspaceRepo, workspaceRepo, goalRepo, dodRepo, memoryrepo.NewWorkspaceTemplateRepository()))
	if err != nil {
		t.Fatal(err)
	}
	workspaceID := common.WorkspaceID("ws-coordinator-")
	if purge {
		workspaceID += "purge"
	} else {
		workspaceID += "retain"
	}
	deliverable := filepath.Join(t.TempDir(), "deliverable")
	internal := filepath.Join(t.TempDir(), "internal")
	if err := os.MkdirAll(deliverable, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := workspaceRepo.Create(context.Background(), &workspacemodel.Workspace{ID: workspaceID, Title: "coordinator", State: common.WorkspaceStateActive, DeliverableDirectory: deliverable, InternalDirectory: internal}); err != nil {
		t.Fatal(err)
	}
	sequence := []string{}
	failOnce := make(map[string]bool, len(failOnceTopics))
	for _, topic := range failOnceTopics {
		failOnce[topic] = true
	}
	observers := map[string]magicevent.SimpleObserver{}
	subscribe := func(destination, topic string, handler func(any) any) {
		observer := observers[destination]
		if observer == nil {
			observer = magicevent.NewSimpleObserver(destination, hub)
			observers[destination] = observer
		}
		observer.Subscribe(topic, func(ev magicevent.Event, result magicevent.Result) {
			sequence = append(sequence, topic)
			if failOnce[topic] {
				failOnce[topic] = false
				result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: "injected owner response loss", Cause: errors.New("injected owner response loss")})
				return
			}
			value := handler(ev.Data())
			if value == nil {
				result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: "invalid fixture command", Cause: common.ErrInvalidArgument})
				return
			}
			result.Set(value, nil)
		})
	}
	subscribe(runevents.CommandDestination, runevents.TopicRunCommandPrepareWorkspaceRetirement, func(value any) any {
		command := value.(runevents.PrepareWorkspaceRetirementCommand)
		return runevents.PrepareWorkspaceRetirementResult{Fence: runevents.RunWorkspaceRetirementFence{
			WorkspaceID: command.WorkspaceID, DeletionID: command.DeletionID, DeletionGeneration: command.DeletionGeneration,
			DataDispositionIntent: command.DataDispositionIntent, WorkspaceIntentHash: command.WorkspaceIntentHash,
			Phase: runmodel.WorkspaceRetirementPhaseDraining, FenceRevision: 1, WorkspaceRunGeneration: 3,
		}}
	})
	subscribe(orchestratorevents.CommandDestination, orchestratorevents.TopicOrchestratorCommandBeginWorkspaceRuntimeQuiescence, func(value any) any {
		command := value.(orchestratorevents.BeginWorkspaceRuntimeQuiescenceCommand)
		return orchestratorevents.BeginWorkspaceRuntimeQuiescenceResult{
			WorkspaceID: command.Intent.WorkspaceID, DeletionID: command.Intent.DeletionID,
			GateID: orchestratorevents.StableWorkspaceRuntimeQuiescingGateID(command.Intent), OrchestratorQuiescenceRevision: 1,
		}
	})
	subscribe(orchestratorevents.CommandDestination, orchestratorevents.TopicOrchestratorCommandConfirmWorkspaceRuntimeQuiescence, func(value any) any {
		command := value.(orchestratorevents.ConfirmWorkspaceRuntimeQuiescenceCommand)
		token := deletioncontract.WorkspaceRuntimeQuiescenceToken{
			WorkspaceID: command.Intent.WorkspaceID, DeletionID: command.Intent.DeletionID, DeletionGeneration: command.Intent.DeletionGeneration,
			DataDispositionIntent: command.Intent.DataDispositionIntent, WorkspaceIntentHash: command.Intent.WorkspaceIntentHash,
			RunDrainingRevision: command.Intent.RunDrainingRevision, WorkspaceRunGeneration: command.Intent.WorkspaceRunGeneration,
			OrchestratorQuiescenceRevision: 2,
		}
		token.RuntimeInventoryHash = deletioncontract.StableWorkspaceRuntimeInventoryHash(token)
		token.QuiescenceID = deletioncontract.StableWorkspaceRuntimeQuiescenceID(token)
		return orchestratorevents.ConfirmWorkspaceRuntimeQuiescenceResult{Token: token, OrchestratorQuiescenceRevision: 2}
	})
	subscribe(runevents.CommandDestination, runevents.TopicRunCommandFinalizeWorkspaceRetirement, func(value any) any {
		command := value.(runevents.FinalizeWorkspaceRetirementCommand)
		token := command.QuiescenceToken
		return runevents.FinalizeWorkspaceRetirementResult{Fence: runevents.RunWorkspaceRetirementFence{
			WorkspaceID: token.WorkspaceID, DeletionID: token.DeletionID, DeletionGeneration: token.DeletionGeneration,
			DataDispositionIntent: token.DataDispositionIntent, WorkspaceIntentHash: token.WorkspaceIntentHash,
			Phase: runmodel.WorkspaceRetirementPhaseRetired, FenceRevision: token.RunDrainingRevision + 1,
			WorkspaceRunGeneration: token.WorkspaceRunGeneration, QuiescenceID: token.QuiescenceID, RuntimeInventoryHash: token.RuntimeInventoryHash,
		}}
	})
	subscribe(orchestratorevents.CommandDestination, orchestratorevents.TopicOrchestratorCommandSealWorkspaceDeletion, func(value any) any {
		command := value.(orchestratorevents.SealWorkspaceOrchestratorDeletionCommand)
		scope := command.Scope()
		return orchestratorevents.SealWorkspaceOrchestratorDeletionResult{WorkspaceID: scope.WorkspaceID, DeletionID: scope.DeletionID, DeletionGeneration: scope.DeletionGeneration, FenceID: orchestratorevents.StableOrchestratorDeletionFenceID(scope), Revision: 1, AckHash: orchestratorevents.StableOrchestratorDeletionSealAckHash(scope)}
	})
	subscribe(resultevents.CommandDestination, resultevents.TopicResultCommandSealWorkspaceDeletion, func(value any) any {
		command := value.(resultevents.SealWorkspaceResultDeletionCommand)
		scope := command.Scope()
		return resultevents.SealWorkspaceResultDeletionResult{WorkspaceID: scope.WorkspaceID, DeletionID: scope.DeletionID, DeletionGeneration: scope.DeletionGeneration, FenceID: resultevents.StableResultDeletionFenceID(scope), Revision: 1, AckHash: resultevents.StableResultDeletionSealAckHash(scope)}
	})
	subscribe(memoryevents.CommandDestination, memoryevents.TopicMemoryCommandSealWorkspaceDeletion, func(value any) any {
		command := value.(memoryevents.SealWorkspaceMemoryDeletionCommand)
		scope := command.Scope()
		return memoryevents.SealWorkspaceMemoryDeletionResult{WorkspaceID: scope.WorkspaceID, DeletionID: scope.DeletionID, DeletionGeneration: scope.DeletionGeneration, FenceID: memoryevents.StableMemoryDeletionFenceID(scope), Revision: 1, AckHash: memoryevents.StableMemoryDeletionSealAckHash(scope)}
	})
	subscribe(decisionevents.CommandDestination, decisionevents.TopicDecisionCommandSealWorkspaceDeletion, func(value any) any {
		command := value.(decisionevents.SealWorkspaceDecisionDeletionCommand)
		scope := command.Scope()
		return decisionevents.SealWorkspaceDecisionDeletionResult{WorkspaceID: scope.WorkspaceID, DeletionID: scope.DeletionID, DeletionGeneration: scope.DeletionGeneration, FenceID: decisionevents.StableDecisionDeletionFenceID(scope), Revision: 1, AckHash: decisionevents.StableDecisionDeletionSealAckHash(scope)}
	})
	subscribe(approvalevents.CommandDestination, approvalevents.TopicApprovalCommandSealWorkspaceDeletion, func(value any) any {
		command := value.(approvalevents.SealWorkspaceApprovalDeletionCommand)
		scope := command.Scope()
		return approvalevents.SealWorkspaceApprovalDeletionResult{WorkspaceID: scope.WorkspaceID, DeletionID: scope.DeletionID, DeletionGeneration: scope.DeletionGeneration, FenceID: approvalevents.StableApprovalDeletionFenceID(scope), Revision: 1, AckHash: approvalevents.StableApprovalDeletionSealAckHash(scope)}
	})
	subscribe(eventstreamevents.CommandDestination, eventstreamevents.TopicEventStreamCommandSealWorkspaceDeletion, func(value any) any {
		command := value.(eventstreamevents.SealWorkspaceEventStreamDeletionCommand)
		scope := command.Scope()
		return eventstreamevents.SealWorkspaceEventStreamDeletionResult{WorkspaceID: scope.WorkspaceID, DeletionID: scope.DeletionID, DeletionGeneration: scope.DeletionGeneration, FenceID: eventstreamevents.StableEventStreamDeletionFenceID(scope), Revision: 1, AckHash: eventstreamevents.StableEventStreamDeletionSealAckHash(scope)}
	})
	if purge {
		subscribeWorkspaceDeletionPurgeFixture(subscribe)
	}
	return owner, workspaceRepo, &sequence, workspaceID
}

func subscribeWorkspaceDeletionPurgeFixture(subscribe func(string, string, func(any) any)) {
	subscribe(orchestratorevents.CommandDestination, orchestratorevents.TopicOrchestratorCommandPurgeWorkspaceEvidence, func(value any) any {
		command := value.(orchestratorevents.PurgeWorkspaceOrchestratorEvidenceCommand)
		scope := command.Scope()
		return orchestratorevents.PurgeWorkspaceOrchestratorEvidenceResult{WorkspaceID: scope.WorkspaceID, DeletionID: scope.DeletionID, DeletionGeneration: scope.DeletionGeneration, FenceID: orchestratorevents.StableOrchestratorDeletionFenceID(scope), PurgeID: orchestratorevents.StableOrchestratorDeletionPurgeID(scope), Revision: 2, AckHash: orchestratorevents.StableOrchestratorDeletionPurgeAckHash(scope), PurgeProofHash: orchestratorevents.StableOrchestratorDeletionPurgeProofHash(scope)}
	})
	subscribe(resultevents.CommandDestination, resultevents.TopicResultCommandPurgeWorkspace, func(value any) any {
		command := value.(resultevents.PurgeWorkspaceResultsCommand)
		scope := command.Scope()
		return resultevents.PurgeWorkspaceResultsResult{WorkspaceID: scope.WorkspaceID, DeletionID: scope.DeletionID, DeletionGeneration: scope.DeletionGeneration, FenceID: resultevents.StableResultDeletionFenceID(scope), PurgeID: resultevents.StableResultDeletionPurgeID(scope), Revision: 2, AckHash: resultevents.StableResultDeletionPurgeAckHash(scope), PurgeProofHash: resultevents.StableResultDeletionPurgeProofHash(scope)}
	})
	subscribe(memoryevents.CommandDestination, memoryevents.TopicMemoryCommandPurgeWorkspace, func(value any) any {
		command := value.(memoryevents.PurgeWorkspaceMemoryCommand)
		scope := command.Scope()
		return memoryevents.PurgeWorkspaceMemoryResult{WorkspaceID: scope.WorkspaceID, DeletionID: scope.DeletionID, DeletionGeneration: scope.DeletionGeneration, FenceID: memoryevents.StableMemoryDeletionFenceID(scope), PurgeID: memoryevents.StableMemoryDeletionPurgeID(scope), Revision: 2, AckHash: memoryevents.StableMemoryDeletionPurgeAckHash(scope), PurgeProofHash: memoryevents.StableMemoryDeletionPurgeProofHash(scope)}
	})
	subscribe(decisionevents.CommandDestination, decisionevents.TopicDecisionCommandPurgeWorkspace, func(value any) any {
		command := value.(decisionevents.PurgeWorkspaceDecisionsCommand)
		scope := command.Scope()
		return decisionevents.PurgeWorkspaceDecisionsResult{WorkspaceID: scope.WorkspaceID, DeletionID: scope.DeletionID, DeletionGeneration: scope.DeletionGeneration, FenceID: decisionevents.StableDecisionDeletionFenceID(scope), PurgeID: decisionevents.StableDecisionDeletionPurgeID(scope), Revision: 2, AckHash: decisionevents.StableDecisionDeletionPurgeAckHash(scope), PurgeProofHash: decisionevents.StableDecisionDeletionPurgeProofHash(scope)}
	})
	subscribe(approvalevents.CommandDestination, approvalevents.TopicApprovalCommandPurgeWorkspace, func(value any) any {
		command := value.(approvalevents.PurgeWorkspaceApprovalsCommand)
		scope := command.Scope()
		return approvalevents.PurgeWorkspaceApprovalsResult{WorkspaceID: scope.WorkspaceID, DeletionID: scope.DeletionID, DeletionGeneration: scope.DeletionGeneration, FenceID: approvalevents.StableApprovalDeletionFenceID(scope), PurgeID: approvalevents.StableApprovalDeletionPurgeID(scope), Revision: 2, AckHash: approvalevents.StableApprovalDeletionPurgeAckHash(scope), PurgeProofHash: approvalevents.StableApprovalDeletionPurgeProofHash(scope)}
	})
	subscribe(runevents.CommandDestination, runevents.TopicRunCommandPurgeWorkspace, func(value any) any {
		command := value.(runevents.PurgeWorkspaceRunsCommand)
		observations := runmodel.EmptyWorkspaceRetirementPurgeObservations()
		return runevents.PurgeWorkspaceRunsResult{WorkspaceID: command.WorkspaceID, DeletionID: command.DeletionID, DeletionGeneration: command.DeletionGeneration, FenceRevision: command.FenceRevision, WorkspaceRunGeneration: command.WorkspaceRunGeneration, QuiescenceID: command.QuiescenceID, PurgeID: runevents.StableRunWorkspacePurgeID(command), AckHash: runevents.StableRunWorkspacePurgeAckHashForObservations(command, observations), PurgeProofHash: runevents.StableRunWorkspacePurgeProofHashForObservations(command, observations), Observations: observations}
	})
	subscribe(eventstreamevents.CommandDestination, eventstreamevents.TopicEventStreamCommandPurgeWorkspace, func(value any) any {
		command := value.(eventstreamevents.PurgeWorkspaceCommand)
		scope := command.Scope()
		return eventstreamevents.PurgeWorkspaceResult{WorkspaceID: scope.WorkspaceID, DeletionID: scope.DeletionID, DeletionGeneration: scope.DeletionGeneration, FenceID: eventstreamevents.StableEventStreamDeletionFenceID(scope), PurgeID: eventstreamevents.StableEventStreamDeletionPurgeID(scope), Revision: 2, AckHash: eventstreamevents.StableEventStreamDeletionPurgeAckHash(scope), PurgeProofHash: eventstreamevents.StableEventStreamDeletionPurgeProofHash(scope)}
	})
}
