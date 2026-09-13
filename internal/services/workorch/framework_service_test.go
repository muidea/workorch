package workorch

import (
	"context"
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

// The delegate may expose the new interface; the WorkOrch wrapper must not
// advertise split quiescence until its owners implement that lifecycle.
func (*stopOwnedWorkerService) Quiesce(context.Context) *cd.Error { return nil }

func TestFrameworkModuleServiceStopsOwnerWorkersBeforeInfrastructureDrain(t *testing.T) {
	hub, background := event.NewHub(16), task.NewBackgroundRoutine(4)
	delegate := &stopOwnedWorkerService{}
	svc := &frameworkModuleService{delegate: delegate}
	if _, ok := any(svc).(frameworkservice.Quiescer); ok {
		t.Fatal("workorch advertises a split shutdown phase its owners do not implement")
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
