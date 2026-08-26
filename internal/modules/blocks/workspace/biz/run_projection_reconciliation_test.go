package workspace

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	magictask "github.com/muidea/magicCommon/task"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	filerepository "github.com/muidea/workorch/internal/pkg/repository/file"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type failingWorkspaceCASRepository struct {
	repository.WorkspaceRepository

	mu       sync.Mutex
	failures []error
	calls    int
}

type selectiveWorkspaceCASFailureRepository struct {
	repository.WorkspaceRepository
	failingWorkspaceID common.WorkspaceID
}

func (r *selectiveWorkspaceCASFailureRepository) CompareAndSwap(ctx context.Context, item *workspacemodel.Workspace, expected uint64) error {
	if item != nil && item.ID == r.failingWorkspaceID {
		return common.ErrConflict
	}
	return r.WorkspaceRepository.CompareAndSwap(ctx, item, expected)
}

type projectionStartupFailureBackground struct {
	magictask.BackgroundRoutine
	timerErr error
	asyncErr error
}

type manualProjectionHeartbeatBackground struct {
	magictask.BackgroundRoutine
	mu        sync.Mutex
	timerTask magictask.Task
}

func (b *manualProjectionHeartbeatBackground) Timer(_ context.Context, task magictask.Task, _, _ time.Duration) error {
	b.mu.Lock()
	b.timerTask = task
	b.mu.Unlock()
	return nil
}

func (b *manualProjectionHeartbeatBackground) AsyncFunction(function func()) error {
	function()
	return nil
}

func (b *manualProjectionHeartbeatBackground) Shutdown(context.Context) bool { return true }

func (b *manualProjectionHeartbeatBackground) fireTimer(t *testing.T) {
	t.Helper()
	b.mu.Lock()
	task := b.timerTask
	b.mu.Unlock()
	if task == nil {
		t.Fatal("projection heartbeat task was not installed")
	}
	task.Run()
}

type behindCursorInjectingRunRepository struct {
	repository.RunRepository
	mu           sync.Mutex
	listCalls    int
	injectOnCall int
	inject       *runmodel.Run
	injectErr    error
}

func (r *behindCursorInjectingRunRepository) injectOnSecondFollowingList() {
	r.mu.Lock()
	r.injectOnCall = r.listCalls + 2
	r.mu.Unlock()
}

func (r *behindCursorInjectingRunRepository) List(ctx context.Context) ([]*runmodel.Run, error) {
	r.mu.Lock()
	r.listCalls++
	if r.listCalls == r.injectOnCall && r.inject != nil {
		r.injectErr = r.RunRepository.Create(ctx, r.inject)
	}
	err := r.injectErr
	r.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return r.RunRepository.List(ctx)
}

func (b *projectionStartupFailureBackground) Timer(ctx context.Context, task magictask.Task, interval, offset time.Duration) error {
	if b.timerErr != nil {
		return b.timerErr
	}
	return b.BackgroundRoutine.Timer(ctx, task, interval, offset)
}

func (b *projectionStartupFailureBackground) AsyncFunction(function func()) error {
	if b.asyncErr != nil {
		return b.asyncErr
	}
	return b.BackgroundRoutine.AsyncFunction(function)
}

func (r *failingWorkspaceCASRepository) CompareAndSwap(ctx context.Context, item *workspacemodel.Workspace, expected uint64) error {
	r.mu.Lock()
	r.calls++
	var failure error
	if len(r.failures) > 0 {
		failure = r.failures[0]
		r.failures = r.failures[1:]
	}
	r.mu.Unlock()
	if failure != nil {
		return failure
	}
	return r.WorkspaceRepository.CompareAndSwap(ctx, item, expected)
}

func (r *failingWorkspaceCASRepository) callCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

func newProjectionReliabilityOwners(
	t *testing.T,
	workspaceRepository repository.WorkspaceRepository,
	deletionMaterialCleaner repository.WorkspaceDeletionMaterialCleaner,
	runRepository repository.RunRepository,
	workspaceBackground magictask.BackgroundRoutine,
) (*Biz, magicevent.Hub) {
	t.Helper()
	hub := magicevent.NewHub(32)
	inventory := magicevent.NewSimpleObserver(runevents.CommandDestination, hub)
	inventory.Subscribe(runevents.TopicRunCommandLifecycleFactInventory, func(event magicevent.Event, result magicevent.Result) {
		command, ok := event.Data().(runevents.QueryRunLifecycleFactInventoryCommand)
		if !ok {
			result.Set(runevents.QueryRunLifecycleFactInventoryResult{}, nil)
			return
		}
		items, err := runRepository.List(event.Context())
		if err != nil {
			result.Set(nil, nil)
			return
		}
		result.Set(testRunLifecycleFactInventory(items, command), nil)
	})
	workspaceOwner, err := New(hub, workspaceBackground, Dependencies{
		Workspaces: workspaceRepository, DeletionMaterialCleaner: deletionMaterialCleaner, Goals: memory.NewGoalRepository(), DoDs: memory.NewDoDRepository(),
		Templates: memory.NewWorkspaceTemplateRepository(), EventHub: hub, BackgroundRoutine: workspaceBackground,
	})
	if err != nil {
		t.Fatalf("new Workspace owner: %v", err)
	}
	if err := workspaceOwner.Start(); err != nil {
		t.Fatalf("start Workspace owner: %v", err)
	}
	t.Cleanup(func() {
		workspaceOwner.Teardown(context.Background())
		inventory.Unsubscribe(runevents.TopicRunCommandLifecycleFactInventory)
		hub.Terminate(context.Background())
		if workspaceBackground != nil {
			workspaceBackground.Shutdown(context.Background())
		}
	})
	return workspaceOwner, hub
}

func testRunLifecycleFactInventory(items []*runmodel.Run, command runevents.QueryRunLifecycleFactInventoryCommand) runevents.QueryRunLifecycleFactInventoryResult {
	limit := command.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	type entry struct {
		key  string
		fact runevents.RunStateChangedData
	}
	entries := make([]entry, 0, len(items))
	for _, item := range items {
		if item == nil || runProjectionTerminalState(item.State) {
			continue
		}
		fact := runevents.RunStateChangedDataFromModel(item)
		key := runevents.RunLifecycleFactInventoryCursorKey(fact)
		if cursor := strings.TrimSpace(command.Cursor); cursor != "" && key <= cursor {
			continue
		}
		entries = append(entries, entry{key: key, fact: fact})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].key < entries[j].key })
	pageSize := len(entries)
	if pageSize > limit {
		pageSize = limit
	}
	response := runevents.QueryRunLifecycleFactInventoryResult{Items: make([]runevents.RunStateChangedData, 0, pageSize)}
	for idx := 0; idx < pageSize; idx++ {
		response.Items = append(response.Items, entries[idx].fact)
	}
	if len(entries) > limit {
		response.NextCursor = entries[limit-1].key
	}
	return response
}

func persistedRunProjectionFixture(t *testing.T, runs repository.RunRepository) *runmodel.Run {
	t.Helper()
	run := &runmodel.Run{
		Revision: 4, ID: "run-1", WorkspaceID: "ws-1", State: common.RunStateActive,
		RuntimeState: &runtimecontract.RuntimeStateRecord{StateRevision: 7},
	}
	if err := runs.Create(context.Background(), run); err != nil {
		t.Fatalf("create Run: %v", err)
	}
	return run
}

func TestWorkspaceRunProjectionHandlerPersistsExactOwnerFactIdentity(t *testing.T) {
	workspaces := memory.NewWorkspaceRepository()
	if err := workspaces.Create(context.Background(), &workspacemodel.Workspace{ID: "ws-1", State: common.WorkspaceStateActive}); err != nil {
		t.Fatal(err)
	}
	runs := memory.NewRunRepository()
	owner, _ := newProjectionReliabilityOwners(t, workspaces, workspaces, runs, nil)
	run := persistedRunProjectionFixture(t, runs)
	fact := runevents.RunStateChangedDataFromModel(run)

	owner.handleRunStateChanged(magicevent.NewEventWithContext(
		runevents.TopicRunStateChanged, runevents.CommandDestination, runevents.NotificationDestination,
		magicevent.NewHeader(), context.Background(), fact,
	), nil)

	stored, err := workspaces.Get(context.Background(), run.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	cursor := stored.RunProjectionCursor
	if stored.ActiveRunID != run.ID || cursor.WorkspaceRunGeneration != fact.WorkspaceRunGeneration || cursor.RunID != fact.RunID || cursor.RunRevision != fact.RunRevision ||
		cursor.RuntimeStateRevision != fact.RuntimeStateRevision || cursor.RuntimeStateRef != fact.RuntimeStateRef ||
		cursor.CausationID != fact.CausationID {
		t.Fatalf("handler projection=%+v Workspace=%+v, want exact Run owner fact=%+v", cursor, stored, fact)
	}
}

func TestWorkspaceRunProjectionFreshOwnerWithoutBackgroundRecoversSynchronously(t *testing.T) {
	workspaces := memory.NewWorkspaceRepository()
	if err := workspaces.Create(context.Background(), &workspacemodel.Workspace{ID: "ws-1", State: common.WorkspaceStateActive}); err != nil {
		t.Fatal(err)
	}
	runs := memory.NewRunRepository()
	run := persistedRunProjectionFixture(t, runs)
	_, _ = newProjectionReliabilityOwners(t, workspaces, workspaces, runs, nil)

	stored, err := workspaces.Get(context.Background(), run.WorkspaceID)
	if err != nil || stored.ActiveRunID != run.ID || stored.RunProjectionCursor.RunRevision != run.Revision {
		t.Fatalf("synchronous fresh-owner recovery Workspace=%+v err=%v", stored, err)
	}
}

func TestWorkspaceRunProjectionStartRollsBackWorkerInstallationFailure(t *testing.T) {
	for _, testCase := range []struct {
		name       string
		timerError bool
	}{
		{name: "timer", timerError: true},
		{name: "initial async"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			hub := magicevent.NewHub(16)
			baseBackground := magictask.NewBackgroundRoutine(4)
			failure := errors.New("projection worker installation failed")
			background := &projectionStartupFailureBackground{BackgroundRoutine: baseBackground}
			if testCase.timerError {
				background.timerErr = failure
			} else {
				background.asyncErr = failure
			}
			workspaces := memory.NewWorkspaceRepository()
			owner, err := New(hub, background, Dependencies{
				Workspaces: workspaces, DeletionMaterialCleaner: workspaces, Goals: memory.NewGoalRepository(), DoDs: memory.NewDoDRepository(),
				Templates: memory.NewWorkspaceTemplateRepository(), EventHub: hub, BackgroundRoutine: background,
			})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				owner.Teardown(context.Background())
				hub.Terminate(context.Background())
				baseBackground.Shutdown(context.Background())
			})

			if err := owner.Start(); !errors.Is(err, failure) {
				t.Fatalf("Start error=%v, want worker installation failure", err)
			}
			owner.runProjectionStateMu.Lock()
			projectionCtx := owner.runProjectionCtx
			owner.runProjectionStateMu.Unlock()
			if owner.started || len(owner.topics) != 0 || projectionCtx != nil {
				t.Fatalf("half-started owner: started=%t topics=%v projectionCtx=%v", owner.started, owner.topics, projectionCtx)
			}

			background.timerErr = nil
			background.asyncErr = nil
			if err := owner.Start(); err != nil || !owner.started || len(owner.topics) == 0 {
				t.Fatalf("clean retry Start error=%v started=%t topics=%v", err, owner.started, owner.topics)
			}
		})
	}
}

func TestWorkspaceRunProjectionHandlerMapsOwnerRevisionAndRetriesCASExhaustion(t *testing.T) {
	base := memory.NewWorkspaceRepository()
	if err := base.Create(context.Background(), &workspacemodel.Workspace{ID: "ws-1", State: common.WorkspaceStateActive}); err != nil {
		t.Fatal(err)
	}
	workspaces := &failingWorkspaceCASRepository{
		WorkspaceRepository: base,
		failures:            []error{common.ErrConflict, common.ErrConflict, common.ErrConflict},
	}
	runs := memory.NewRunRepository()
	owner, _ := newProjectionReliabilityOwners(t, workspaces, base, runs, nil)
	run := persistedRunProjectionFixture(t, runs)

	fact := runevents.RunStateChangedDataFromModel(run)
	owner.handleRunStateChanged(magicevent.NewEventWithContext(
		runevents.TopicRunStateChanged, runevents.CommandDestination, runevents.NotificationDestination,
		magicevent.NewHeader(), context.Background(), fact,
	), nil)

	owner.runProjectionStateMu.Lock()
	pending, observedErr := owner.runProjectionPending, owner.runProjectionLastError
	owner.runProjectionStateMu.Unlock()
	if pending || observedErr != nil || workspaces.callCount() != 4 {
		t.Fatalf("CAS exhaustion fallback pending=%t err=%v calls=%d", pending, observedErr, workspaces.callCount())
	}
	stored, err := base.Get(context.Background(), "ws-1")
	if err != nil || stored.ActiveRunID != run.ID || stored.RunProjectionCursor.RunRevision != run.Revision ||
		stored.RunProjectionCursor.RuntimeStateRevision != run.RuntimeState.StateRevision ||
		stored.RunProjectionCursor.CausationID != fact.CausationID {
		t.Fatalf("reconciled Workspace=%+v err=%v", stored, err)
	}
	owner.runProjectionStateMu.Lock()
	pending, observedErr = owner.runProjectionPending, owner.runProjectionLastError
	owner.runProjectionStateMu.Unlock()
	if pending || observedErr != nil {
		t.Fatalf("successful reconciliation pending=%t err=%v", pending, observedErr)
	}
}

func TestWorkspaceRunProjectionStorageFailureIsRetriedFromCurrentRunFact(t *testing.T) {
	base := memory.NewWorkspaceRepository()
	if err := base.Create(context.Background(), &workspacemodel.Workspace{ID: "ws-1", State: common.WorkspaceStateActive}); err != nil {
		t.Fatal(err)
	}
	storageErr := errors.New("workspace persistence unavailable")
	workspaces := &failingWorkspaceCASRepository{WorkspaceRepository: base, failures: []error{storageErr}}
	runs := memory.NewRunRepository()
	owner, _ := newProjectionReliabilityOwners(t, workspaces, base, runs, nil)
	run := persistedRunProjectionFixture(t, runs)

	fact := runevents.RunStateChangedDataFromModel(run)
	owner.handleRunStateChanged(magicevent.NewEventWithContext(
		runevents.TopicRunStateChanged, runevents.CommandDestination, runevents.NotificationDestination,
		magicevent.NewHeader(), context.Background(), fact,
	), nil)
	owner.runProjectionStateMu.Lock()
	pending, observedErr := owner.runProjectionPending, owner.runProjectionLastError
	owner.runProjectionStateMu.Unlock()
	if pending || observedErr != nil || workspaces.callCount() != 2 {
		t.Fatalf("storage fallback pending=%t err=%v calls=%d", pending, observedErr, workspaces.callCount())
	}

	stored, err := base.Get(context.Background(), "ws-1")
	if err != nil || stored.ActiveRunID != run.ID || stored.RunProjectionCursor.RunRevision != run.Revision {
		t.Fatalf("storage recovery Workspace=%+v err=%v", stored, err)
	}
}

func TestWorkspaceRunProjectionReconciliationDoesNotBlockLaterWorkspaceAfterCASExhaustion(t *testing.T) {
	base := memory.NewWorkspaceRepository()
	for _, workspaceID := range []common.WorkspaceID{"ws-a", "ws-b"} {
		if err := base.Create(context.Background(), &workspacemodel.Workspace{ID: workspaceID, State: common.WorkspaceStateActive}); err != nil {
			t.Fatal(err)
		}
	}
	workspaces := &selectiveWorkspaceCASFailureRepository{WorkspaceRepository: base, failingWorkspaceID: "ws-a"}
	runs := memory.NewRunRepository()
	owner, _ := newProjectionReliabilityOwners(t, workspaces, base, runs, nil)
	for _, run := range []*runmodel.Run{
		{Revision: 2, ID: "run-a", WorkspaceID: "ws-a", State: common.RunStateActive},
		{Revision: 3, ID: "run-b", WorkspaceID: "ws-b", State: common.RunStateActive},
	} {
		if err := runs.Create(context.Background(), run); err != nil {
			t.Fatal(err)
		}
	}
	err := owner.reconcileCurrentRunProjectionFacts(context.Background())
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("reconciliation error=%v, want retained first Workspace CAS exhaustion", err)
	}
	first, firstErr := base.Get(context.Background(), "ws-a")
	second, secondErr := base.Get(context.Background(), "ws-b")
	if firstErr != nil || first.ActiveRunID != "" {
		t.Fatalf("failed first Workspace=%+v err=%v", first, firstErr)
	}
	if secondErr != nil || second.ActiveRunID != "run-b" || second.RunProjectionCursor.RunRevision != 3 {
		t.Fatalf("later Workspace=%+v err=%v, want independent convergence", second, secondErr)
	}
}

func TestWorkspaceRunProjectionFreshOwnerRecoversWithoutHistoricalNotification(t *testing.T) {
	path := filepath.Join(t.TempDir(), "workspaces.json")
	workspaces := filerepository.NewWorkspaceRepository(path)
	if err := workspaces.Create(context.Background(), &workspacemodel.Workspace{ID: "ws-1", State: common.WorkspaceStateActive}); err != nil {
		t.Fatal(err)
	}
	runs := memory.NewRunRepository()
	run := persistedRunProjectionFixture(t, runs)
	background := magictask.NewBackgroundRoutine(4)
	_, _ = newProjectionReliabilityOwners(t, workspaces, workspaces, runs, background)

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		stored, err := workspaces.Get(context.Background(), "ws-1")
		if err == nil && stored.ActiveRunID == run.ID && stored.RunProjectionCursor.RunRevision == run.Revision {
			reopened, reopenErr := filerepository.NewWorkspaceRepository(path).Get(context.Background(), "ws-1")
			if reopenErr != nil || reopened.ActiveRunID != run.ID || reopened.RunProjectionCursor.WorkspaceRunGeneration != run.WorkspaceRunGeneration || reopened.RunProjectionCursor.RunRevision != run.Revision ||
				reopened.RunProjectionCursor.RuntimeStateRevision != run.RuntimeState.StateRevision {
				t.Fatalf("reopened Workspace=%+v err=%v", reopened, reopenErr)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	stored, err := workspaces.Get(context.Background(), "ws-1")
	t.Fatalf("fresh owner did not recover current Run fact: Workspace=%+v err=%v", stored, err)
}

func TestWorkspaceRunProjectionHeartbeatFindsLostNotificationAfterPendingClears(t *testing.T) {
	workspaces := memory.NewWorkspaceRepository()
	if err := workspaces.Create(context.Background(), &workspacemodel.Workspace{ID: "ws-1", State: common.WorkspaceStateActive}); err != nil {
		t.Fatal(err)
	}
	runs := memory.NewRunRepository()
	background := &manualProjectionHeartbeatBackground{}
	owner, _ := newProjectionReliabilityOwners(t, workspaces, workspaces, runs, background)
	owner.runProjectionStateMu.Lock()
	pending := owner.runProjectionPending
	owner.runProjectionStateMu.Unlock()
	if pending {
		t.Fatal("initial empty inventory did not clear pending state")
	}

	run := persistedRunProjectionFixture(t, runs)
	// Deliberately publish no RunStateChanged notification. The periodic tick
	// must be a source-of-truth scan even though no process-local retry is dirty.
	background.fireTimer(t)
	stored, err := workspaces.Get(context.Background(), run.WorkspaceID)
	if err != nil || stored.ActiveRunID != run.ID || stored.RunProjectionCursor.RunRevision != run.Revision {
		t.Fatalf("heartbeat projection Workspace=%+v err=%v", stored, err)
	}
}

func TestWorkspaceRunProjectionHeartbeatRecoversFactInsertedBehindInventoryCursor(t *testing.T) {
	workspaces := memory.NewWorkspaceRepository()
	if err := workspaces.Create(context.Background(), &workspacemodel.Workspace{ID: "ws-a", State: common.WorkspaceStateActive}); err != nil {
		t.Fatal(err)
	}
	baseRuns := memory.NewRunRepository()
	for idx := 0; idx < runProjectionInventoryPageLimit+1; idx++ {
		run := &runmodel.Run{
			Revision: 1, ID: common.RunID(fmt.Sprintf("run-%03d", idx)),
			WorkspaceID: common.WorkspaceID(fmt.Sprintf("ws-z%03d", idx)), State: common.RunStateActive,
		}
		if err := baseRuns.Create(context.Background(), run); err != nil {
			t.Fatal(err)
		}
	}
	target := &runmodel.Run{Revision: 2, ID: "run-target", WorkspaceID: "ws-a", State: common.RunStateActive}
	runs := &behindCursorInjectingRunRepository{
		RunRepository: baseRuns, inject: target,
	}
	background := &manualProjectionHeartbeatBackground{}
	runs.injectOnSecondFollowingList()
	owner, _ := newProjectionReliabilityOwners(t, workspaces, workspaces, runs, background)

	// The target was inserted before page two was read, but its key sorts
	// before page one's cursor, so the non-snapshot startup scan cannot see it.
	stored, err := workspaces.Get(context.Background(), target.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.ActiveRunID != "" {
		t.Fatalf("behind-cursor fact unexpectedly appeared in first scan: %+v", stored)
	}
	owner.runProjectionStateMu.Lock()
	pending := owner.runProjectionPending
	owner.runProjectionStateMu.Unlock()
	if pending {
		t.Fatal("successful non-snapshot scan did not clear pending state")
	}

	background.fireTimer(t)
	stored, err = workspaces.Get(context.Background(), target.WorkspaceID)
	if err != nil || stored.ActiveRunID != target.ID || stored.RunProjectionCursor.RunRevision != target.Revision {
		t.Fatalf("next full heartbeat did not recover behind-cursor fact: Workspace=%+v err=%v", stored, err)
	}
}

func TestWorkspaceRunProjectionRevisionFenceRejectsStaleAndConflictingFacts(t *testing.T) {
	base := &workspacemodel.Workspace{ID: "ws-1", State: common.WorkspaceStateActive}
	currentFact := workspaceRunProjectionTestFact("run-1", 5, 7, common.RunStateActive)
	transition, err := reduceRunProjectionFact(base, currentFact)
	if err != nil {
		t.Fatal(err)
	}
	current := applyWorkspaceProjection(base, transition)

	for _, stale := range []workspaceRunProjectionFact{
		workspaceRunProjectionTestFact("run-1", 4, 6, common.RunStateBlockedOnApproval),
		workspaceRunProjectionTestFact("run-1", 4, 7, common.RunStateBlockedOnApproval),
		currentFact,
	} {
		result, err := reduceRunProjectionFact(current, stale)
		if err != nil || !result.Noop {
			t.Fatalf("stale/duplicate fact=%+v transition=%+v err=%v", stale, result, err)
		}
	}

	conflicting := workspaceRunProjectionTestFact("run-1", 5, 8, common.RunStateActive)
	if _, err := reduceRunProjectionFact(current, conflicting); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("same Run revision with changed Runtime identity err=%v", err)
	}
	for _, unordered := range []workspaceRunProjectionFact{
		workspaceRunProjectionTestFact("run-1", 4, 8, common.RunStateBlockedOnApproval),
		workspaceRunProjectionTestFact("run-1", 6, 6, common.RunStateBlockedOnApproval),
	} {
		if _, err := reduceRunProjectionFact(current, unordered); !errors.Is(err, common.ErrConflict) {
			t.Fatalf("incomparable Run fact revisions=%+v err=%v", unordered, err)
		}
	}

	newer := workspaceRunProjectionTestFact("run-1", 6, 8, common.RunStateActive)
	result, err := reduceRunProjectionFact(current, newer)
	if err != nil || result.Noop || result.RunProjection == nil || result.RunProjection.RunRevision != 6 {
		t.Fatalf("newer fact transition=%+v err=%v", result, err)
	}
}

func TestWorkspaceTerminalDeliveryRetiresDelayedRunProjection(t *testing.T) {
	base := &workspacemodel.Workspace{ID: "ws-1", State: common.WorkspaceStateActive}
	fact := workspaceRunProjectionTestFact("run-1", 5, 7, common.RunStateActive)
	projection, err := reduceRunProjectionFact(base, fact)
	if err != nil {
		t.Fatal(err)
	}
	current := applyWorkspaceProjection(base, projection)
	delivery := workspaceTerminalDeliveryTestCommand(runtimecontract.RuntimeTerminalDeliveryIdentity{
		WorkspaceID: "ws-1", RunID: "run-1", TerminalDeliveryID: "td-1", AuthoritySnapshotID: "auth-1",
		DefinitionRevision: "def-1", EvidenceRevision: "evidence-1", DispatchID: "dispatch-1",
		Outcome: runtimecontract.TerminalOutcome{Kind: runtimecontract.TerminalOutcomeKindCompleted, Code: "completed", Summary: "done"},
	}, "terminal-delivery/td-1")
	terminal, err := reduceWorkspaceTerminalDelivery(current, delivery)
	if err != nil {
		t.Fatal(err)
	}
	released := applyWorkspaceProjection(current, terminal)
	if released.ActiveRunID != "" {
		t.Fatalf("terminal release retained active Run: %+v", released)
	}
	stale, err := reduceRunProjectionFact(released, fact)
	if err != nil || !stale.Noop || stale.ActiveRunID != "" {
		t.Fatalf("delayed fact reclaimed terminal slot: transition=%+v err=%v", stale, err)
	}
}

func TestWorkspaceTerminalRunHintClearsRetryWithoutReleasingActiveSlot(t *testing.T) {
	workspaces := memory.NewWorkspaceRepository()
	if err := workspaces.Create(context.Background(), &workspacemodel.Workspace{
		ID: "ws-1", State: common.WorkspaceStateActive, ActiveRunID: "run-1",
	}); err != nil {
		t.Fatal(err)
	}
	runs := memory.NewRunRepository()
	owner, _ := newProjectionReliabilityOwners(t, workspaces, workspaces, runs, nil)
	terminal := &runmodel.Run{Revision: 5, ID: "run-1", WorkspaceID: "ws-1", State: common.RunStateCompleted}
	if err := runs.Create(context.Background(), terminal); err != nil {
		t.Fatal(err)
	}

	owner.handleRunStateChanged(magicevent.NewEventWithContext(
		runevents.TopicRunStateChanged, runevents.CommandDestination, runevents.NotificationDestination,
		magicevent.NewHeader(), context.Background(), runevents.RunStateChangedDataFromModel(terminal),
	), nil)

	stored, err := workspaces.Get(context.Background(), "ws-1")
	owner.runProjectionStateMu.Lock()
	pending, observedErr := owner.runProjectionPending, owner.runProjectionLastError
	owner.runProjectionStateMu.Unlock()
	if err != nil || stored.ActiveRunID != "run-1" || pending || observedErr != nil {
		t.Fatalf("terminal hint Workspace=%+v err=%v pending=%t reconciliationErr=%v", stored, err, pending, observedErr)
	}
}
