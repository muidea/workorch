package workorch

import (
	"context"
	"errors"
	"github.com/muidea/magicCommon/framework/application"
	"sync/atomic"
	"testing"
	"time"

	cd "github.com/muidea/magicCommon/def"
	"github.com/muidea/magicCommon/event"
	frameworkservice "github.com/muidea/magicCommon/framework/service"
	"github.com/muidea/magicCommon/task"
)

type stopOwnedWorkerService struct {
	cancel context.CancelFunc
	done   chan struct{}
	err    *cd.Error
}

func (s *stopOwnedWorkerService) Startup(_ context.Context, _ string, _ event.Hub, background task.BackgroundRoutine) *cd.Error {
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel, s.done = cancel, make(chan struct{})
	if err := background.AsyncFunction(func() { defer close(s.done); <-ctx.Done() }); err != nil {
		cancel()
		return cd.NewError(cd.Unexpected, err.Error())
	}
	return nil
}

func (*stopOwnedWorkerService) Run(context.Context) *cd.Error { return nil }
func (s *stopOwnedWorkerService) Shutdown(ctx context.Context) {
	if err := s.ShutdownChecked(ctx); err != nil {
		panic(err)
	}
}

func (s *stopOwnedWorkerService) ShutdownChecked(ctx context.Context) *cd.Error {
	if s.err != nil {
		return s.err
	}
	s.cancel()
	select {
	case <-s.done:
		return nil
	case <-ctx.Done():
		return cd.NewError(cd.Timeout, ctx.Err().Error())
	}
}

// 阶段排空先停止 producer，但不得在共享队列排空前释放 owner。
func (s *stopOwnedWorkerService) Quiesce(ctx context.Context) *cd.Error {
	if s.err != nil {
		return s.err
	}
	s.cancel()
	select {
	case <-s.done:
		return nil
	case <-ctx.Done():
		return cd.WrapError(cd.Timeout, ctx.Err(), "worker quiesce failed")
	}
}

type applicationBarrierService struct {
	stopOwnedWorkerService
	entered, release chan struct{}
	releases         atomic.Int32
}

func (s *applicationBarrierService) Startup(_ context.Context, _ string, _ event.Hub, background task.BackgroundRoutine) *cd.Error {
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel, s.done = cancel, make(chan struct{})
	if err := background.AsyncFunction(func() { defer close(s.done); close(s.entered); <-ctx.Done(); <-s.release }); err != nil {
		return cd.WrapError(cd.Unexpected, err, "worker submission failed")
	}
	return nil
}
func (s *applicationBarrierService) ShutdownChecked(ctx context.Context) *cd.Error {
	if err := s.Quiesce(ctx); err != nil {
		return err
	}
	s.releases.Add(1)
	return nil
}

func TestApplicationBarrierRetainsDependenciesUntilActualWorkerExit(t *testing.T) {
	delegate := &applicationBarrierService{entered: make(chan struct{}), release: make(chan struct{})}
	app := application.NewApplication(application.Options{ConfigDir: t.TempDir(), ServiceName: "workorch-barrier-test"})
	if err := app.Startup(context.Background(), &frameworkModuleService{delegate: delegate}); err != nil {
		t.Fatal(err)
	}
	<-delegate.entered
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	err := app.ShutdownChecked(ctx)
	cancel()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("incomplete shutdown=%v", err)
	}
	if delegate.releases.Load() != 0 {
		t.Fatal("Application released service before actual exit")
	}
	// 失败屏障必须保留可用 Hub 与未关闭的共享调度器。
	observer := event.NewSimpleObserver("barrier-query", app.EventHub())
	if err := observer.Subscribe("barrier-query", func(_ event.Event, result event.Result) { result.Set(struct{}{}, nil) }); err != nil {
		t.Fatal(err)
	}
	if err := app.EventHub().Send(event.NewEventWithContext("barrier-query", "test", "barrier-query", nil, context.Background(), nil)).Error(); err != nil {
		t.Fatal(err)
	}
	close(delegate.release)
	ctx, cancel = context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := app.ShutdownChecked(ctx); err != nil {
		t.Fatal(err)
	}
	if delegate.releases.Load() != 1 {
		t.Fatal("successful barrier did not release exactly once")
	}
	if err := app.ShutdownChecked(ctx); err != nil {
		t.Fatal(err)
	}
	if delegate.releases.Load() != 1 {
		t.Fatal("retry repeated completed release")
	}
}

func TestFrameworkModuleServiceStopsOwnerWorkersBeforeInfrastructureDrain(t *testing.T) {
	hub, background := event.NewHub(16), task.NewBackgroundRoutine(4)
	delegate := &stopOwnedWorkerService{}
	svc := &frameworkModuleService{delegate: delegate}
	if _, ok := any(svc).(frameworkservice.Quiescer); !ok {
		t.Fatal("workorch is missing the shared pre-release barrier")
	}
	if err := svc.Startup(context.Background(), "workorch-test", hub, background); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		delegate.cancel()
		background.Shutdown(context.Background())
		hub.Terminate(context.Background())
	})
	if err := svc.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	rejected := cd.NewError(cd.Timeout, "owner has not drained")
	delegate.err = rejected
	if err := svc.ShutdownChecked(context.Background()); err != rejected {
		t.Fatalf("lost checked shutdown error: %v", err)
	}
	select {
	case <-delegate.done:
		t.Fatal("failed shutdown was treated as permission to release the owner")
	default:
	}
	delegate.err = nil
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := svc.Quiesce(ctx); err != nil {
		t.Fatal(err)
	}
	if !background.Shutdown(ctx) {
		t.Fatal("shared queue did not drain")
	}
	if err := hub.(event.DrainingHub).Drain(ctx); err != nil {
		t.Fatal(err)
	}
	if err := svc.ShutdownChecked(ctx); err != nil {
		t.Fatal(err)
	}
	if !background.Shutdown(ctx) {
		t.Fatal("owner worker outlived checked module shutdown")
	}
	if err := hub.(event.DrainingHub).TerminateChecked(ctx); err != nil {
		t.Fatal(err)
	}
	if err := svc.ShutdownChecked(ctx); err != nil {
		t.Fatal(err)
	}
}
