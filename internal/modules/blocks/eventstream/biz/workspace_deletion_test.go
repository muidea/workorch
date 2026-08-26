package eventstream

import (
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"sync"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	eventstreamevents "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/events"
	eventstreammodel "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	deletioncontract "github.com/muidea/workorch/internal/pkg/workorchdeletioncontract"
)

type eventStreamWorkspacePurgerSpy struct {
	mu       sync.Mutex
	calls    int
	expected []eventstreammodel.WorkspaceDeletion
	err      error
}

func (p *eventStreamWorkspacePurgerSpy) PurgeWorkspaceEventRecords(_ context.Context, expected eventstreammodel.WorkspaceDeletion) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	p.expected = append(p.expected, expected)
	if p.err != nil {
		return 0, p.err
	}
	return 1, nil
}

func (p *eventStreamWorkspacePurgerSpy) callCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

func TestEventStreamWorkspaceDeletionPhysicalPurgeUsesOwnerPrivateDispatcher(t *testing.T) {
	calls := eventStreamWorkspaceDeletionFunctionCalls(t, "workspace_deletion_internal_event.go")
	if got := calls["purgeWorkspace"]["PurgeWorkspaceEventRecords"]; got != 0 {
		t.Fatalf("command adapter directly calls physical purge sink %d times", got)
	}
	if got := calls["purgeWorkspace"]["dispatchWorkspaceDeletionInternalEvent"]; got != 1 {
		t.Fatalf("command adapter private dispatcher calls=%d, want 1", got)
	}
	if got := calls["dispatchWorkspaceDeletionInternalEvent"]["PurgeWorkspaceEventRecords"]; got != 1 {
		t.Fatalf("owner-private dispatcher physical purge sink calls=%d, want exactly 1", got)
	}
}

func eventStreamWorkspaceDeletionFunctionCalls(t *testing.T, path string) map[string]map[string]int {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	ret := map[string]map[string]int{}
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Body == nil {
			continue
		}
		called := map[string]int{}
		ast.Inspect(function.Body, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			switch target := call.Fun.(type) {
			case *ast.Ident:
				called[target.Name]++
			case *ast.SelectorExpr:
				called[target.Sel.Name]++
			}
			return true
		})
		ret[function.Name.Name] = called
	}
	return ret
}

func bizEventStreamDeletionScope(workspaceID common.WorkspaceID, mode deletioncontract.Mode) deletioncontract.Scope {
	scope := deletioncontract.Scope{
		WorkspaceID: workspaceID, DeletionID: "deletion-" + string(workspaceID), DeletionGeneration: 1,
		Mode: mode, QuiescenceID: "quiescence-" + string(workspaceID),
	}
	scope.RequestHash = deletioncontract.StableRequestHash(scope)
	return scope
}

func sendEventStreamDeletionCommand[Command any, Response any](ctx context.Context, hub magicevent.Hub, topic, source string, command Command) (Response, error) {
	var zero Response
	result := hub.Send(magicevent.NewEventWithContext(topic, source, eventstreamevents.CommandDestination, magicevent.NewHeader(), ctx, command))
	if result == nil {
		return zero, common.ErrInvalidArgument
	}
	data, err := result.Get()
	if err != nil {
		return zero, err
	}
	response, ok := data.(Response)
	if !ok {
		return zero, common.ErrInvalidArgument
	}
	return response, nil
}

func sealEventStreamWorkspace(t *testing.T, ctx context.Context, hub magicevent.Hub, scope deletioncontract.Scope) eventstreamevents.SealWorkspaceEventStreamDeletionResult {
	t.Helper()
	command := eventstreamevents.SealWorkspaceEventStreamDeletionCommand{
		WorkspaceID: scope.WorkspaceID, DeletionID: scope.DeletionID, DeletionGeneration: scope.DeletionGeneration,
		DataDispositionIntent: scope.Mode, QuiescenceID: scope.QuiescenceID, RequestHash: scope.RequestHash,
	}
	response, err := sendEventStreamDeletionCommand[eventstreamevents.SealWorkspaceEventStreamDeletionCommand, eventstreamevents.SealWorkspaceEventStreamDeletionResult](ctx, hub, eventstreamevents.TopicEventStreamCommandSealWorkspaceDeletion, workspaceDeletionCommandSource, command)
	if err != nil || response.ValidateFor(command) != nil {
		t.Fatalf("seal response=%+v err=%v", response, err)
	}
	return response
}

func TestEventStreamWorkspaceDeletionReducerOwnsFencedToPurgedTransition(t *testing.T) {
	scope := bizEventStreamDeletionScope("ws-eventstream-reducer", deletioncontract.ModePurge)
	fenced, err := reduceWorkspaceDeletionInternalEvent(nil, workspaceDeletionInternalEvent{
		Kind: workspaceDeletionInternalEventFenceObserved, Scope: scope,
	}, "2026-08-25T00:00:00Z")
	if err != nil || fenced.Phase != eventstreammodel.WorkspaceDeletionPhaseFenced || fenced.Revision != 1 || fenced.Validate() != nil {
		t.Fatalf("fenced reduction = %+v err=%v", fenced, err)
	}
	purged, err := reduceWorkspaceDeletionInternalEvent(fenced, workspaceDeletionInternalEvent{
		Kind: workspaceDeletionInternalEventPurgeCompleted, Scope: scope,
	}, "2026-08-25T00:01:00Z")
	if err != nil || purged.Phase != eventstreammodel.WorkspaceDeletionPhasePurged || purged.Revision != 2 || purged.Validate() != nil {
		t.Fatalf("purged reduction = %+v err=%v", purged, err)
	}
	if fenced.Phase != eventstreammodel.WorkspaceDeletionPhaseFenced || fenced.Revision != 1 || fenced.PurgeID != "" {
		t.Fatalf("reducer mutated prior state: %+v", fenced)
	}
	retainScope := bizEventStreamDeletionScope("ws-eventstream-retain-reducer", deletioncontract.ModeRetain)
	retained, err := reduceWorkspaceDeletionInternalEvent(nil, workspaceDeletionInternalEvent{
		Kind: workspaceDeletionInternalEventFenceObserved, Scope: retainScope,
	}, "2026-08-25T00:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reduceWorkspaceDeletionInternalEvent(retained, workspaceDeletionInternalEvent{
		Kind: workspaceDeletionInternalEventPurgeCompleted, Scope: retainScope,
	}, "2026-08-25T00:01:00Z"); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("retain purge reduction = %v, want conflict", err)
	}
}

func newEventStreamDeletionDispatcherFixture(t *testing.T, scope deletioncontract.Scope, purger *eventStreamWorkspacePurgerSpy) (*Biz, *memory.EventStreamWorkspaceDeletionRepository, *eventstreammodel.WorkspaceDeletion) {
	t.Helper()
	fenced, err := reduceWorkspaceDeletionInternalEvent(nil, workspaceDeletionInternalEvent{
		Kind: workspaceDeletionInternalEventFenceObserved, Scope: scope,
	}, "2026-08-25T00:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	deletions := memory.NewEventStreamWorkspaceDeletionRepository()
	if err := deletions.Create(context.Background(), fenced); err != nil {
		t.Fatal(err)
	}
	records := memory.NewEventRecordRepository()
	return New(Dependencies{OwnerRepositories: eventStreamBizTestOwnerRepositories(records, deletions, purger)}), deletions, fenced
}

func TestEventStreamWorkspaceDeletionPurgeRequestedOwnsEffectAndExactReplay(t *testing.T) {
	ctx := context.Background()
	scope := bizEventStreamDeletionScope("ws-eventstream-private-dispatch", deletioncontract.ModePurge)
	purger := &eventStreamWorkspacePurgerSpy{}
	owner, deletions, fenced := newEventStreamDeletionDispatcherFixture(t, scope, purger)

	purged, err := owner.dispatchWorkspaceDeletionInternalEvent(ctx, fenced, workspaceDeletionInternalEvent{
		Kind: workspaceDeletionInternalEventPurgeRequested, Scope: scope,
	})
	if err != nil || purged.Phase != eventstreammodel.WorkspaceDeletionPhasePurged || purged.Revision != 2 || purged.Validate() != nil {
		t.Fatalf("purge_requested result=%+v err=%v", purged, err)
	}
	if purger.callCount() != 1 {
		t.Fatalf("physical purge calls=%d, want 1", purger.callCount())
	}
	current, err := deletions.Get(ctx, scope.WorkspaceID)
	if err != nil || !reflect.DeepEqual(current, purged) {
		t.Fatalf("persisted purged tombstone=%+v err=%v, want %+v", current, err, purged)
	}

	// A response can be lost after the purged CAS. Re-driving the same private
	// request or replaying the public command must return the exact tombstone
	// without invoking the physical sink again.
	replayed, err := owner.dispatchWorkspaceDeletionInternalEvent(ctx, fenced, workspaceDeletionInternalEvent{
		Kind: workspaceDeletionInternalEventPurgeRequested, Scope: scope,
	})
	if err != nil || !reflect.DeepEqual(replayed, purged) {
		t.Fatalf("private response-loss replay=%+v err=%v, want %+v", replayed, err, purged)
	}
	first, err := owner.purgeWorkspace(ctx, scope)
	if err != nil {
		t.Fatal(err)
	}
	second, err := owner.purgeWorkspace(ctx, scope)
	if err != nil || !reflect.DeepEqual(first, second) || first.PurgeID != purged.PurgeID || first.AckHash != purged.PurgeAckHash {
		t.Fatalf("command exact replay first=%+v second=%+v err=%v", first, second, err)
	}
	if purger.callCount() != 1 {
		t.Fatalf("physical purge calls after replay=%d, want 1", purger.callCount())
	}
}

func TestEventStreamWorkspaceDeletionPurgeDispatcherRejectsInvalidTransitionsWithoutEffect(t *testing.T) {
	ctx := context.Background()
	purgeScope := bizEventStreamDeletionScope("ws-eventstream-private-guards", deletioncontract.ModePurge)
	purger := &eventStreamWorkspacePurgerSpy{}
	owner, _, fenced := newEventStreamDeletionDispatcherFixture(t, purgeScope, purger)

	if _, err := owner.dispatchWorkspaceDeletionInternalEvent(ctx, fenced, workspaceDeletionInternalEvent{
		Kind: workspaceDeletionInternalEventPurgeCompleted, Scope: purgeScope,
	}); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("direct purge_completed error=%v, want conflict", err)
	}
	drift := purgeScope
	drift.QuiescenceID = "quiescence-drift"
	drift.RequestHash = deletioncontract.StableRequestHash(drift)
	if _, err := owner.dispatchWorkspaceDeletionInternalEvent(ctx, fenced, workspaceDeletionInternalEvent{
		Kind: workspaceDeletionInternalEventPurgeRequested, Scope: drift,
	}); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("scope drift error=%v, want conflict", err)
	}
	stale := eventstreammodel.CloneWorkspaceDeletion(fenced)
	stale.UpdatedAt = "2026-08-25T00:00:01Z"
	if stale.Validate() != nil {
		t.Fatal("stale fixture must remain structurally valid")
	}
	if _, err := owner.dispatchWorkspaceDeletionInternalEvent(ctx, stale, workspaceDeletionInternalEvent{
		Kind: workspaceDeletionInternalEventPurgeRequested, Scope: purgeScope,
	}); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("stale fence error=%v, want conflict", err)
	}

	retainScope := bizEventStreamDeletionScope("ws-eventstream-private-retain", deletioncontract.ModeRetain)
	retainOwner, _, retained := newEventStreamDeletionDispatcherFixture(t, retainScope, purger)
	if _, err := retainOwner.dispatchWorkspaceDeletionInternalEvent(ctx, retained, workspaceDeletionInternalEvent{
		Kind: workspaceDeletionInternalEventPurgeRequested, Scope: retainScope,
	}); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("retain purge_requested error=%v, want conflict", err)
	}
	if purger.callCount() != 0 {
		t.Fatalf("invalid transitions invoked physical purge %d times", purger.callCount())
	}
}

func TestEventStreamWorkspaceDeletionPurgeFailureDoesNotCommitAndNilDependenciesFailClosed(t *testing.T) {
	ctx := context.Background()
	scope := bizEventStreamDeletionScope("ws-eventstream-private-failure", deletioncontract.ModePurge)
	sinkErr := errors.New("physical purge failed")
	purger := &eventStreamWorkspacePurgerSpy{err: sinkErr}
	owner, deletions, fenced := newEventStreamDeletionDispatcherFixture(t, scope, purger)

	if _, err := owner.dispatchWorkspaceDeletionInternalEvent(ctx, fenced, workspaceDeletionInternalEvent{
		Kind: workspaceDeletionInternalEventPurgeRequested, Scope: scope,
	}); !errors.Is(err, sinkErr) {
		t.Fatalf("sink failure error=%v, want %v", err, sinkErr)
	}
	current, err := deletions.Get(ctx, scope.WorkspaceID)
	if err != nil || current.Phase != eventstreammodel.WorkspaceDeletionPhaseFenced || current.Revision != 1 {
		t.Fatalf("tombstone after sink failure=%+v err=%v, want fenced revision 1", current, err)
	}
	if purger.callCount() != 1 {
		t.Fatalf("sink failure calls=%d, want 1", purger.callCount())
	}
	request := workspaceDeletionInternalEvent{Kind: workspaceDeletionInternalEventPurgeRequested, Scope: scope}
	if _, err := (*Biz)(nil).dispatchWorkspaceDeletionInternalEvent(ctx, fenced, request); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("nil owner error=%v, want invalid argument", err)
	}
	if _, err := (&Biz{}).dispatchWorkspaceDeletionInternalEvent(ctx, fenced, request); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("nil dependencies error=%v, want invalid argument", err)
	}
}

func TestEventStreamDeletionRetainExactReplayAndReadFence(t *testing.T) {
	ctx := context.Background()
	owner := memory.NewEventStreamOwnerRepositorySet()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	service := New(Dependencies{OwnerRepositories: eventStreamBizTestOwnerRepositories(owner.EventRecords, owner.WorkspaceDeletions, owner.EventRecords), EventHub: hub})
	if err := service.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(service.Stop)
	workspaceID := common.WorkspaceID("ws-eventstream-biz-retain")
	if err := service.appendRecord(ctx, &eventstreammodel.EventRecord{ID: "event-biz-retain", Topic: "/test", SourceModule: "blocks/test", SubjectType: "test", SubjectID: "retain", WorkspaceID: workspaceID}); err != nil {
		t.Fatal(err)
	}
	scope := bizEventStreamDeletionScope(workspaceID, deletioncontract.ModeRetain)
	command := eventstreamevents.SealWorkspaceEventStreamDeletionCommand{
		WorkspaceID: scope.WorkspaceID, DeletionID: scope.DeletionID, DeletionGeneration: scope.DeletionGeneration,
		DataDispositionIntent: scope.Mode, QuiescenceID: scope.QuiescenceID, RequestHash: scope.RequestHash,
	}
	if _, err := sendEventStreamDeletionCommand[eventstreamevents.SealWorkspaceEventStreamDeletionCommand, eventstreamevents.SealWorkspaceEventStreamDeletionResult](ctx, hub, eventstreamevents.TopicEventStreamCommandSealWorkspaceDeletion, "application/workorchd/appservice", command); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("application seal source = %v, want invalid argument", err)
	}
	first := sealEventStreamWorkspace(t, ctx, hub, scope)
	second := sealEventStreamWorkspace(t, ctx, hub, scope)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("exact replay changed response: first=%+v second=%+v", first, second)
	}
	changed := scope
	changed.QuiescenceID = "quiescence-changed"
	changed.RequestHash = deletioncontract.StableRequestHash(changed)
	changedCommand := eventstreamevents.SealWorkspaceEventStreamDeletionCommand{
		WorkspaceID: changed.WorkspaceID, DeletionID: changed.DeletionID, DeletionGeneration: changed.DeletionGeneration,
		DataDispositionIntent: changed.Mode, QuiescenceID: changed.QuiescenceID, RequestHash: changed.RequestHash,
	}
	if _, err := sendEventStreamDeletionCommand[eventstreamevents.SealWorkspaceEventStreamDeletionCommand, eventstreamevents.SealWorkspaceEventStreamDeletionResult](ctx, hub, eventstreamevents.TopicEventStreamCommandSealWorkspaceDeletion, workspaceDeletionCommandSource, changedCommand); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("changed seal scope = %v, want conflict", err)
	}
	list, err := service.listEvents(ctx, repositoryEventRecordQuery(eventstreamevents.EventQuery{WorkspaceID: workspaceID, Limit: 10}))
	if err != nil || len(list) != 0 {
		t.Fatalf("fenced list=%+v err=%v", list, err)
	}
	high, err := service.highWatermarks(ctx, []common.WorkspaceID{workspaceID})
	if err != nil || len(high) != 1 || high[0].Revision != 0 {
		t.Fatalf("fenced high watermarks=%+v err=%v", high, err)
	}
	if err := service.appendRecord(ctx, &eventstreammodel.EventRecord{ID: "event-biz-after-retain", Topic: "/test", SourceModule: "blocks/test", SubjectType: "test", SubjectID: "after", WorkspaceID: workspaceID}); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("append after retain = %v, want not found", err)
	}
	purge := eventstreamevents.PurgeWorkspaceCommand{
		WorkspaceID: scope.WorkspaceID, DeletionID: scope.DeletionID, DeletionGeneration: scope.DeletionGeneration,
		QuiescenceID: scope.QuiescenceID, RequestHash: scope.RequestHash,
	}
	if _, err := sendEventStreamDeletionCommand[eventstreamevents.PurgeWorkspaceCommand, eventstreamevents.PurgeWorkspaceResult](ctx, hub, eventstreamevents.TopicEventStreamCommandPurgeWorkspace, workspaceDeletionCommandSource, purge); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("retain purge = %v, want invalid argument", err)
	}
}

func TestEventStreamDeletionPurgeExactReplay(t *testing.T) {
	ctx := context.Background()
	owner := memory.NewEventStreamOwnerRepositorySet()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	service := New(Dependencies{OwnerRepositories: eventStreamBizTestOwnerRepositories(owner.EventRecords, owner.WorkspaceDeletions, owner.EventRecords), EventHub: hub})
	if err := service.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(service.Stop)
	workspaceID := common.WorkspaceID("ws-eventstream-biz-purge")
	for _, id := range []common.EventRecordID{"event-biz-purge-a", "event-biz-purge-b"} {
		if err := service.appendRecord(ctx, &eventstreammodel.EventRecord{ID: id, Topic: "/test", SourceModule: "blocks/test", SubjectType: "test", SubjectID: string(id), WorkspaceID: workspaceID}); err != nil {
			t.Fatal(err)
		}
	}
	scope := bizEventStreamDeletionScope(workspaceID, deletioncontract.ModePurge)
	sealEventStreamWorkspace(t, ctx, hub, scope)
	command := eventstreamevents.PurgeWorkspaceCommand{
		WorkspaceID: scope.WorkspaceID, DeletionID: scope.DeletionID, DeletionGeneration: scope.DeletionGeneration,
		QuiescenceID: scope.QuiescenceID, RequestHash: scope.RequestHash,
	}
	if _, err := sendEventStreamDeletionCommand[eventstreamevents.PurgeWorkspaceCommand, eventstreamevents.PurgeWorkspaceResult](ctx, hub, eventstreamevents.TopicEventStreamCommandPurgeWorkspace, "application/workorchd/appservice", command); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("application purge source = %v, want invalid argument", err)
	}
	first, err := sendEventStreamDeletionCommand[eventstreamevents.PurgeWorkspaceCommand, eventstreamevents.PurgeWorkspaceResult](ctx, hub, eventstreamevents.TopicEventStreamCommandPurgeWorkspace, workspaceDeletionCommandSource, command)
	if err != nil || first.ValidateFor(command) != nil {
		t.Fatalf("purge response=%+v err=%v", first, err)
	}
	second, err := sendEventStreamDeletionCommand[eventstreamevents.PurgeWorkspaceCommand, eventstreamevents.PurgeWorkspaceResult](ctx, hub, eventstreamevents.TopicEventStreamCommandPurgeWorkspace, workspaceDeletionCommandSource, command)
	if err != nil || !reflect.DeepEqual(first, second) {
		t.Fatalf("purge exact replay first=%+v second=%+v err=%v", first, second, err)
	}
	changed := scope
	changed.QuiescenceID = "quiescence-eventstream-purge-changed"
	changed.RequestHash = deletioncontract.StableRequestHash(changed)
	changedCommand := eventstreamevents.PurgeWorkspaceCommand{
		WorkspaceID: changed.WorkspaceID, DeletionID: changed.DeletionID, DeletionGeneration: changed.DeletionGeneration,
		QuiescenceID: changed.QuiescenceID, RequestHash: changed.RequestHash,
	}
	if _, err := sendEventStreamDeletionCommand[eventstreamevents.PurgeWorkspaceCommand, eventstreamevents.PurgeWorkspaceResult](ctx, hub, eventstreamevents.TopicEventStreamCommandPurgeWorkspace, workspaceDeletionCommandSource, changedCommand); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("changed purge scope = %v, want conflict", err)
	}
	current, err := owner.WorkspaceDeletions.Get(ctx, workspaceID)
	if err != nil || current.Phase != eventstreammodel.WorkspaceDeletionPhasePurged || current.Revision != 2 {
		t.Fatalf("purged tombstone=%+v err=%v", current, err)
	}
}

func TestEventStreamDeletionConcurrentExactPurgeConverges(t *testing.T) {
	for iteration := 0; iteration < 100; iteration++ {
		ctx := context.Background()
		owner := memory.NewEventStreamOwnerRepositorySet()
		ownerRepositories := owner.OwnerRepositories()
		first := New(Dependencies{OwnerRepositories: ownerRepositories})
		second := New(Dependencies{OwnerRepositories: ownerRepositories})
		scope := bizEventStreamDeletionScope("ws-eventstream-concurrent-purge", deletioncontract.ModePurge)
		if _, err := first.sealWorkspaceDeletion(ctx, scope); err != nil {
			t.Fatalf("iteration %d seal: %v", iteration, err)
		}
		start := make(chan struct{})
		var wait sync.WaitGroup
		wait.Add(2)
		responses := make([]eventstreamevents.PurgeWorkspaceResult, 2)
		errs := make([]error, 2)
		for idx, service := range []*Biz{first, second} {
			idx, service := idx, service
			go func() {
				defer wait.Done()
				<-start
				responses[idx], errs[idx] = service.purgeWorkspace(ctx, scope)
			}()
		}
		close(start)
		wait.Wait()
		if errs[0] != nil || errs[1] != nil || !reflect.DeepEqual(responses[0], responses[1]) {
			t.Fatalf("iteration %d concurrent purge responses=%+v errs=%v", iteration, responses, errs)
		}
	}
}
