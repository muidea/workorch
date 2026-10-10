package decision

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	cd "github.com/muidea/magicCommon/def"
	"github.com/muidea/magicCommon/event"
	"github.com/muidea/magicCommon/task"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	"github.com/muidea/workorch/internal/pkg/repository"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
)

type lifecycleHub struct {
	event.Hub
	subs           int
	failAt         int
	unsubscribeErr *cd.Error
	panicValue     any
	removed        []string
}

func (h *lifecycleHub) Subscribe(topic string, observer event.Observer) *cd.Error {
	h.subs++
	if h.panicValue != nil {
		panic(h.panicValue)
	}
	if h.subs == h.failAt {
		return cd.NewError(cd.ResourceExhausted, "subscription failed")
	}
	return h.Hub.Subscribe(topic, observer)
}
func (h *lifecycleHub) Unsubscribe(topic string, observer event.Observer) *cd.Error {
	if h.panicValue != nil {
		panic(h.panicValue)
	}
	if h.unsubscribeErr != nil {
		return h.unsubscribeErr
	}
	if err := h.Hub.Unsubscribe(topic, observer); err != nil {
		return err
	}
	h.removed = append(h.removed, topic)
	return nil
}
func lifecycleOwner(t *testing.T, hub event.Hub, background task.BackgroundRoutine) *Biz {
	t.Helper()
	repos := memory.NewDecisionOwnerRepositorySet()
	owner, err := New(hub, background, Dependencies{EventHub: hub, OwnerRepositories: repos.OwnerRepositories()})
	if err != nil {
		t.Fatal(err)
	}
	return owner
}

type blockedTicketInventory struct {
	repository.DecisionTicketRepository
	started chan struct{}
	release chan struct{}
}

func (r *blockedTicketInventory) List(context.Context) ([]*decisionmodel.Ticket, error) {
	close(r.started)
	<-r.release
	return nil, nil
}
func TestOwnerQuiesceTimeoutPreservesWorkerAndDependenciesUntilActualExit(t *testing.T) {
	hub := event.NewHub(16)
	background := task.NewBackgroundRoutine(2)
	owner := lifecycleOwner(t, hub, background)
	inventory := &blockedTicketInventory{DecisionTicketRepository: owner.tickets, started: make(chan struct{}), release: make(chan struct{})}
	owner.tickets = inventory
	var release sync.Once
	t.Cleanup(func() {
		release.Do(func() { close(inventory.release) })
		owner.Teardown(context.Background())
		background.Shutdown(context.Background())
		hub.Terminate(context.Background())
	})
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-inventory.started:
	case <-time.After(time.Second):
		t.Fatal("worker did not enter inventory")
	}
	done, service := owner.reconcileDone, owner.service
	owner.BeginShutdown()
	if err := owner.Start(); !errors.Is(err, basebiz.ErrLifecycleClosing) {
		t.Fatalf("Start reopened closing owner: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := owner.Teardown(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("unfinished shutdown = %v", err)
	}
	if owner.reconcileDone != done || owner.reconcileCancel == nil || owner.service != service || owner.tickets != inventory || len(owner.topics) == 0 {
		t.Fatal("timeout discarded live resources")
	}
	if leave, err := owner.enterOwner(false); leave != nil || !errors.Is(err, basebiz.ErrLifecycleClosing) {
		t.Fatal("fresh admitted after shutdown")
	}
	leave, err := owner.enterOwner(true)
	if err != nil {
		t.Fatal(err)
	}
	leave()
	release.Do(func() { close(inventory.release) })
	if err := owner.Teardown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if owner.service != nil || owner.reconcileDone != nil || owner.reconcileCancel != nil || len(owner.topics) != 0 {
		t.Fatal("successful cleanup retained resources")
	}
}
func TestOwnerPartialStartAndFinalUnsubscribeFailureAreRetryable(t *testing.T) {
	hub := &lifecycleHub{Hub: event.NewHub(16), failAt: 3}
	defer hub.Terminate(context.Background())
	owner := lifecycleOwner(t, hub, nil)
	if err := owner.Start(); err == nil {
		t.Fatal("partial Start succeeded")
	}
	if len(owner.topics) != 2 {
		t.Fatalf("ledger = %v", owner.topics)
	}
	if err := owner.Start(); err == nil {
		t.Fatal("partial Start reopened generation")
	}
	rejection := cd.NewError(cd.ResourceExhausted, "cancel failed")
	hub.unsubscribeErr = rejection
	if err := owner.Teardown(context.Background()); err != rejection {
		t.Fatalf("cancel error = %v", err)
	}
	if len(owner.topics) != 2 || owner.service == nil {
		t.Fatal("failed cancel lost resources")
	}
	hub.unsubscribeErr = nil
	if err := owner.Teardown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(hub.removed) != 2 || hub.removed[0] != decisionCommandTopics()[1] || hub.removed[1] != decisionCommandTopics()[0] {
		t.Fatalf("cleanup order = %v", hub.removed)
	}
}
func TestOwnerUnknownHubPanicPropagatesWithoutPoisoningLifecycleLock(t *testing.T) {
	hub := &lifecycleHub{Hub: event.NewHub(16)}
	defer hub.Terminate(context.Background())
	owner := lifecycleOwner(t, hub, nil)
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	hub.panicValue = "unknown Hub panic"
	func() {
		defer func() {
			if recover() != hub.panicValue {
				t.Fatal("panic replaced")
			}
		}()
		_ = owner.Teardown(context.Background())
	}()
	if len(owner.topics) == 0 || owner.service == nil {
		t.Fatal("panic lost cleanup state")
	}
	hub.panicValue = nil
	if err := owner.Teardown(context.Background()); err != nil {
		t.Fatal(err)
	}
}
func TestOwnerShutdownTopicClassificationDoesNotAuthorizeNewRootBySource(t *testing.T) {
	for _, topic := range []string{decisionevents.TopicOpenRuntimeGovernanceRequest, decisionevents.TopicOpenRuntimeDefinitionClarification, decisionevents.TopicOpenWorkspaceDefinitionClarification, decisionevents.TopicReconcileWorkspaceDefinitionClarificationCandidate, decisionevents.TopicAcquireRuntimeCapabilities} {
		if decisionDrainTopic(topic) {
			t.Fatalf("fresh topic classified drain: %s", topic)
		}
	}
	for _, topic := range []string{decisionevents.TopicDecisionCommandGetTicket, decisionevents.TopicDecisionCommandEnsureResolved, decisionevents.TopicResolveRuntimeDefinitionClarificationByReplacement} {
		if !decisionDrainTopic(topic) {
			t.Fatalf("existing drain removed: %s", topic)
		}
	}
}
