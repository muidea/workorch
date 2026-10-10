package approval

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
	approvalevents "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"
	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	"github.com/muidea/workorch/internal/pkg/repository"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
)

type lifecycleHub struct {
	event.Hub
	mu             sync.Mutex
	subs           int
	failAt         int
	unsubscribeErr *cd.Error
	panicValue     any
	removed        []string
}

func (h *lifecycleHub) Subscribe(topic string, observer event.Observer) *cd.Error {
	h.mu.Lock()
	h.subs++
	fail := h.subs == h.failAt
	value := h.panicValue
	h.mu.Unlock()
	if value != nil {
		panic(value)
	}
	if fail {
		return cd.NewError(cd.ResourceExhausted, "injected subscription failure")
	}
	return h.Hub.Subscribe(topic, observer)
}
func (h *lifecycleHub) Unsubscribe(topic string, observer event.Observer) *cd.Error {
	h.mu.Lock()
	err, value := h.unsubscribeErr, h.panicValue
	h.mu.Unlock()
	if value != nil {
		panic(value)
	}
	if err != nil {
		return err
	}
	if err := h.Hub.Unsubscribe(topic, observer); err != nil {
		return err
	}
	h.mu.Lock()
	h.removed = append(h.removed, topic)
	h.mu.Unlock()
	return nil
}
func lifecycleOwner(t *testing.T, hub event.Hub, background task.BackgroundRoutine) *Biz {
	t.Helper()
	repos := memory.NewApprovalOwnerRepositorySet()
	owner, err := New(hub, background, Dependencies{EventHub: hub, OwnerRepositories: repos.OwnerRepositories()})
	if err != nil {
		t.Fatal(err)
	}
	return owner
}

type blockedApprovalInventory struct {
	repository.ApprovalRepository
	started chan struct{}
	release chan struct{}
}

func (r *blockedApprovalInventory) List(context.Context) ([]*approvalmodel.Approval, error) {
	close(r.started)
	<-r.release
	return nil, nil
}
func TestOwnerQuiesceTimeoutPreservesWorkerAndDependenciesUntilActualExit(t *testing.T) {
	hub := event.NewHub(16)
	background := task.NewBackgroundRoutine(2)
	owner := lifecycleOwner(t, hub, background)
	inventory := &blockedApprovalInventory{ApprovalRepository: owner.approvals, started: make(chan struct{}), release: make(chan struct{})}
	owner.approvals = inventory
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
	done, cancel, service := owner.reconcileDone, owner.reconcileCancel, owner.service
	owner.BeginShutdown()
	if err := owner.Start(); !errors.Is(err, basebiz.ErrLifecycleClosing) {
		t.Fatalf("Start reopened closing owner: %v", err)
	}
	ctx, stop := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer stop()
	if err := owner.Teardown(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("unfinished shutdown = %v", err)
	}
	if owner.reconcileDone != done || owner.reconcileCancel == nil || cancel == nil || owner.service != service || owner.approvals != inventory || len(owner.topics) == 0 {
		t.Fatal("timeout discarded live resources")
	}
	if leave, err := owner.enterOwner(false); leave != nil || !errors.Is(err, basebiz.ErrLifecycleClosing) {
		t.Fatal("fresh admitted after BeginShutdown")
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
		t.Fatal("successful shutdown retained released resources")
	}
	if err := owner.Teardown(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestOwnerPartialStartAndFinalUnsubscribeFailureAreRetryable(t *testing.T) {
	hub := &lifecycleHub{Hub: event.NewHub(16), failAt: 3}
	defer hub.Terminate(context.Background())
	owner := lifecycleOwner(t, hub, nil)
	if err := owner.Start(); err == nil {
		t.Fatal("partial subscription Start succeeded")
	}
	if len(owner.topics) != 2 {
		t.Fatalf("ledger = %v", owner.topics)
	}
	if err := owner.Start(); err == nil {
		t.Fatal("partial Start reopened generation")
	}
	owner.BeginShutdown()
	if err := owner.Quiesce(context.Background()); err != nil {
		t.Fatal(err)
	}
	rejection := cd.NewError(cd.ResourceExhausted, "cancel failed")
	hub.unsubscribeErr = rejection
	if err := owner.Teardown(context.Background()); err != rejection {
		t.Fatalf("cancel error = %v", err)
	}
	if len(owner.topics) != 2 || owner.service == nil {
		t.Fatal("failed cancellation lost ledger/dependencies")
	}
	hub.unsubscribeErr = nil
	if err := owner.Teardown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(hub.removed) != 2 || hub.removed[0] != approvalCommandTopics()[1] || hub.removed[1] != approvalCommandTopics()[0] {
		t.Fatalf("reverse cleanup = %v", hub.removed)
	}
}

func TestOwnerUnknownHubPanicPropagatesWithoutPoisoningLifecycleLock(t *testing.T) {
	hub := &lifecycleHub{Hub: event.NewHub(16)}
	defer hub.Terminate(context.Background())
	owner := lifecycleOwner(t, hub, nil)
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	hub.panicValue = "unknown cancellation panic"
	func() {
		defer func() {
			if recover() != hub.panicValue {
				t.Fatal("unknown panic replaced")
			}
		}()
		_ = owner.Teardown(context.Background())
	}()
	if len(owner.topics) == 0 || owner.service == nil {
		t.Fatal("panic discarded cleanup state")
	}
	hub.panicValue = nil
	if err := owner.Teardown(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestOwnerShutdownTopicClassificationDoesNotAuthorizeNewRootBySource(t *testing.T) {
	if approvalDrainTopic(approvalevents.TopicOpenRuntimeApprovalRequest) || approvalDrainTopic(approvalevents.TopicAcquireRuntimeCapabilities) {
		t.Fatal("fresh runtime ingress classified as drain")
	}
	if !approvalDrainTopic(approvalevents.TopicApprovalCommandGet) || !approvalDrainTopic(approvalevents.TopicApprovalCommandEnsureApproved) {
		t.Fatal("existing query/resolution drain removed")
	}
}
