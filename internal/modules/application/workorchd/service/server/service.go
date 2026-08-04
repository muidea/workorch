package server

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync/atomic"
	"time"

	cd "github.com/muidea/magicCommon/def"
	"github.com/muidea/magicCommon/task"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspaceviewcore"
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	eventstreamevents "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/events"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

type Service struct {
	config                 Config
	http                   *HTTPServer
	done                   chan error
	runStarted             atomic.Bool
	serviceStoppedNotifier func(workorchdevents.ServiceStoppedData)
}

func NewService(config Config) *Service {
	return &Service{config: normalizeConfig(config)}
}

func (s *Service) Startup(ctx context.Context, serviceName string, session *runtimecore.Session, backgroundRoutine task.BackgroundRoutine) *cd.Error {
	if s == nil {
		return cd.NewError(cd.IllegalParam, "workorch service is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if session == nil {
		return cd.NewError(cd.IllegalParam, "workorch runtime session is not configured")
	}
	if backgroundRoutine == nil {
		return cd.NewError(cd.IllegalParam, "workorch background routine is not configured")
	}
	s.http = NewHTTPServerWithRuntimeSessionAndBackground(s.config, session, backgroundRoutine)
	s.done = make(chan error, 1)
	s.runStarted.Store(false)
	if err := s.http.Validate(ctx); err != nil {
		return cd.NewError(cd.Unexpected, err.Error())
	}
	slog.Info("workorch service startup completed", "service", serviceName, "addr", s.config.Addr)
	return nil
}

// SetWorkspaceRunScheduler injects the application Biz port after server
// construction. HTTP handlers use it for typed scheduling without retaining
// or dispatching through EventHub themselves.
func (s *Service) SetWorkspaceRunScheduler(scheduler WorkspaceRunScheduler) {
	if s == nil || s.http == nil || s.http.state == nil {
		return
	}
	s.http.state.workspaceRunScheduler = scheduler
}

func (s *Service) SetServiceStoppedNotifier(notifier func(workorchdevents.ServiceStoppedData)) {
	if s != nil {
		s.serviceStoppedNotifier = notifier
	}
}

func (s *Service) SetWorkspaceProjectionEventPorts(ports workspaceviewcore.EventPorts) {
	if s != nil && s.http != nil && s.http.state != nil && s.http.state.workspaceProjection != nil {
		s.http.state.workspaceProjection.SetEventPorts(ports)
	}
}

func (s *Service) RecordWorkspaceProjectionChanged(ctx context.Context, data workorchdevents.WorkspaceProjectionChangedData) {
	if s != nil && s.http != nil && s.http.state != nil {
		s.http.state.RecordWorkspaceProjectionChanged(ctx, data)
	}
}

// RecordRunStateChanged accepts the Run owner's typed notification for
// process-local metrics and status wakeups. It does not write owner state or
// derive Runtime authority.
func (s *Service) RecordRunStateChanged(ctx context.Context, data runevents.RunStateChangedData) {
	if s != nil && s.http != nil && s.http.state != nil {
		s.http.state.RecordRunStateChanged(ctx, data)
	}
}

// RecordRuntimeDriveObserved stores only the Orchestrator's immutable SM-4
// telemetry in process-local metrics. It neither persists data nor invokes a
// Runtime command, so replay/reconnect cannot affect state transitions.
func (s *Service) RecordRuntimeDriveObserved(_ context.Context, data orchestratorevents.RuntimeDriveObservedData) {
	if s != nil && s.http != nil && s.http.state != nil {
		s.http.state.RecordRuntimeDriveObserved(data)
	}
}

func (s *Service) RecordEventRecordAppended(ctx context.Context, data eventstreamevents.EventRecordData) {
	if s != nil && s.http != nil && s.http.state != nil {
		s.http.state.RecordEventRecordAppended(ctx, data)
		if s.http.state.workspaceProjection != nil {
			_ = s.http.state.workspaceProjection.HandleEventRecord(ctx, data)
		}
	}
}

func (s *Service) Run(_ context.Context) *cd.Error {
	if s == nil || s.http == nil {
		return cd.NewError(cd.IllegalParam, "workorch http server is not configured")
	}

	runHTTPServer := func() {
		err := s.http.Start()
		if err != nil {
			slog.Error("workorch http server stopped with error", "err", err)
		}
		s.done <- err
		close(s.done)
		s.publishStopped(err)
	}

	s.runStarted.Store(true)
	if s.http.state == nil || s.http.state.backgroundRoutine == nil {
		s.runStarted.Store(false)
		return cd.NewError(cd.IllegalParam, "workorch background routine is not configured")
	}
	if err := s.http.state.startWorkspaceProjection(s.http.state.backgroundContext()); err != nil {
		s.runStarted.Store(false)
		return cd.NewError(cd.Unexpected, "workspace projection startup failed: "+err.Error())
	}
	if err := s.http.state.backgroundRoutine.AsyncFunction(runHTTPServer); err != nil {
		s.runStarted.Store(false)
		return cd.NewError(cd.Unexpected, "http server background scheduling failed: "+err.Error())
	}
	return nil
}

func (s *Service) publishStopped(err error) {
	if s == nil || s.serviceStoppedNotifier == nil {
		return
	}
	data := workorchdevents.ServiceStoppedData{ServiceName: "workorch", State: "stopped", ErrorCode: "server_stopped", StoppedAt: time.Now().UTC()}
	if err != nil {
		data.State = "failed"
		data.ErrorCode = "server_failed"
		data.ErrorMessage = err.Error()
		if len(data.ErrorMessage) > 512 {
			data.ErrorMessage = strings.TrimSpace(data.ErrorMessage[:512])
		}
	}
	s.serviceStoppedNotifier(data)
}

func (s *Service) Shutdown(ctx context.Context) {
	if s == nil || s.http == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := s.http.Shutdown(ctx); err != nil && !errors.Is(err, context.Canceled) {
		slog.Error("workorch http server shutdown failed", "err", err)
	}
	if !s.runStarted.Load() {
		return
	}
	if s.done == nil {
		return
	}
	select {
	case err, ok := <-s.done:
		if ok && err != nil {
			slog.Error("workorch http server run failed", "err", err)
		}
	case <-ctx.Done():
		slog.Warn("workorch http server shutdown timed out", "err", ctx.Err())
	}
}

func (s *Service) ListWorkspaceView(ctx context.Context) ([]appmodelcore.Workspace, error) {
	if s == nil || s.http == nil {
		return nil, errors.New("workorch http server is not configured")
	}
	return s.http.ListWorkspaceView(ctx)
}

func (s *Service) ReadWorkspaceViewSnapshot(ctx context.Context, workspaceID string) (readmodelcore.WorkspaceSnapshot, error) {
	if s == nil || s.http == nil {
		return readmodelcore.WorkspaceSnapshot{}, errors.New("workorch http server is not configured")
	}
	return s.http.ReadWorkspaceViewSnapshot(ctx, workspaceID)
}

func (s *Service) ReadWorkspaceActivity(ctx context.Context, workspaceID string) (projectioncontract.WorkspaceActivityFeed, bool, error) {
	if s == nil || s.http == nil {
		return projectioncontract.WorkspaceActivityFeed{}, false, errors.New("workorch http server is not configured")
	}
	return s.http.ReadWorkspaceActivity(ctx, workspaceID)
}

func (s *Service) RebuildWorkspaceProjection(ctx context.Context) (int64, error) {
	if s == nil || s.http == nil || s.http.state == nil || s.http.state.workspaceProjection == nil {
		return 0, errors.New("workspace projection is not configured")
	}
	if err := s.http.state.workspaceProjection.RebuildAll(ctx); err != nil {
		return 0, err
	}
	return s.http.state.workspaceProjection.Status().GlobalRevision, nil
}

func (s *Service) Done() <-chan error {
	if s == nil || s.done == nil {
		closed := make(chan error)
		close(closed)
		return closed
	}
	return s.done
}
