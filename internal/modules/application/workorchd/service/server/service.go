package server

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	cd "github.com/muidea/magicCommon/def"
	"github.com/muidea/magicCommon/task"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	eventstreamevents "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/events"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

type Service struct {
	config                 Config
	http                   *HTTPServer
	done                   chan error
	runStarted             atomic.Bool
	runMu                  sync.Mutex
	runExited              chan struct{}
	runErr                 error
	serviceStoppedNotifier func(workorchdevents.ServiceStoppedData)
}

func NewService(config Config) *Service {
	return &Service{config: normalizeConfig(config)}
}

// StartupWithApplicationPorts is the production Application Module path. Biz
// owns the runtime session and supplies only narrow typed callbacks; the HTTP
// service does not retain that session or an EventHub handle.
func (s *Service) StartupWithApplicationPorts(ctx context.Context, serviceName string, ports ApplicationPorts, backgroundRoutine task.BackgroundRoutine) *cd.Error {
	if s == nil {
		return cd.NewError(cd.IllegalParam, "workorch service is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if backgroundRoutine == nil {
		return cd.NewError(cd.IllegalParam, "workorch background routine is not configured")
	}
	s.runMu.Lock()
	defer s.runMu.Unlock()
	if s.http != nil {
		return cd.NewError(cd.IllegalParam, "workorch service already owns transport state")
	}
	s.http = NewHTTPServerWithApplicationPorts(s.config, ports, backgroundRoutine)
	s.done = make(chan error, 1)
	s.runExited = nil
	s.runErr = nil
	s.runStarted.Store(false)
	if err := s.http.Validate(ctx); err != nil {
		return cd.NewError(cd.Unexpected, err.Error())
	}
	slog.Info("workorch service startup completed", "service", serviceName, "addr", s.config.Addr)
	return nil
}

// SetWorkspaceExecutionAdmission installs the Biz-owned EventHub admission
// gateway. The typed production constructor never falls back to a
// server-local queue or direct owner call.
func (s *Service) SetWorkspaceExecutionAdmission(admission WorkspaceExecutionAdmission) {
	if s == nil || s.http == nil || s.http.state == nil {
		return
	}
	s.http.state.workspaceExecutionAdmission = admission
}

// SetWorkspaceProjection binds the single Biz-owned derived projection to the
// transport. The server may read and forward observations through this port,
// but it does not start or close the injected component.
func (s *Service) SetWorkspaceProjection(projection WorkspaceProjection) {
	if s == nil || s.http == nil || s.http.state == nil {
		return
	}
	s.http.state.workspaceProjection = projection
}

// SetWorkspaceProjectionWriter binds the Biz-owned derived-write port. The
// server uses it only after an owner command has succeeded; it cannot mutate
// Runtime/Run authority through this port.
func (s *Service) SetWorkspaceProjectionWriter(writer WorkspaceProjectionWriter) {
	if s == nil || s.http == nil || s.http.state == nil {
		return
	}
	s.http.state.workspaceProjectionWriter = writer
}

func (s *Service) SetServiceStoppedNotifier(notifier func(workorchdevents.ServiceStoppedData)) {
	if s != nil {
		s.serviceStoppedNotifier = notifier
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

// RecordRuntimeDriveObserved stores the Orchestrator's immutable SM-4
// telemetry in process-local transport metrics. The Biz-owned projection
// component consumes the same typed observation through its own port.
func (s *Service) RecordRuntimeDriveObserved(_ context.Context, data orchestratorevents.RuntimeDriveObservedData) {
	if s != nil && s.http != nil && s.http.state != nil {
		s.http.state.RecordRuntimeDriveObserved(data)
	}
}

func (s *Service) RecordEventRecordAppended(ctx context.Context, data eventstreamevents.EventRecordData) {
	if s != nil && s.http != nil && s.http.state != nil {
		s.http.state.RecordEventRecordAppended(ctx, data)
	}
}

// RecordWorkspaceDeleted forwards the Workspace owner's post-seal immutable
// receipt to process-local SSE transport. It does not append to Eventstream or
// mutate an application/owner projection.
func (s *Service) RecordWorkspaceDeleted(ctx context.Context, data workspaceevents.WorkspaceDeletedData) {
	if s != nil && s.http != nil && s.http.state != nil {
		s.http.state.RecordWorkspaceDeleted(ctx, data)
	}
}

func (s *Service) Run(_ context.Context) *cd.Error {
	if s == nil || s.http == nil {
		return cd.NewError(cd.IllegalParam, "workorch http server is not configured")
	}

	if s.http.state == nil || s.http.state.backgroundRoutine == nil {
		s.runStarted.Store(false)
		return cd.NewError(cd.IllegalParam, "workorch background routine is not configured")
	}
	if s.http.state.workspaceProjection == nil {
		s.runStarted.Store(false)
		return cd.NewError(cd.IllegalParam, "workspace projection is not configured")
	}
	// These ports are assembled by Biz and are part of the single application
	// boundary. Starting an HTTP listener with one missing would make handlers
	// fail later and could re-enable a second server-local path.
	if s.http.state.workspaceExecutionAdmission == nil {
		s.runStarted.Store(false)
		return cd.NewError(cd.IllegalParam, "workspace execution admission is not configured")
	}
	if s.http.state.workspaceProjectionWriter == nil {
		s.runStarted.Store(false)
		return cd.NewError(cd.IllegalParam, "workspace projection writer is not configured")
	}
	s.runMu.Lock()
	if s.runStarted.Load() || s.runExited != nil {
		s.runMu.Unlock()
		return cd.NewError(cd.IllegalParam, "workorch http server run is already entered")
	}
	s.http.mu.Lock()
	stopping := s.http.stopping
	s.http.mu.Unlock()
	if stopping {
		s.runMu.Unlock()
		return cd.NewError(cd.IllegalParam, "workorch http server is shutting down")
	}
	if s.done == nil {
		s.done = make(chan error, 1)
	}
	exited := make(chan struct{})
	s.runExited = exited
	s.runStarted.Store(true)
	s.runMu.Unlock()
	runHTTPServer := func() {
		// 完成回执独立于可被消费的公开 Done 通知，
		// 必须保留到已调度函数（包括 notifier）真实返回。
		defer close(exited)
		err := s.http.Start()
		s.runMu.Lock()
		s.runErr = err
		s.runMu.Unlock()
		if err != nil {
			slog.Error("workorch http server stopped with error", "err", err)
		}
		s.done <- err
		close(s.done)
		s.publishStopped(err)
	}
	if err := s.http.state.backgroundRoutine.AsyncFunction(runHTTPServer); err != nil {
		s.runMu.Lock()
		s.runStarted.Store(false)
		s.runExited = nil
		close(exited)
		s.runMu.Unlock()
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

// 旧 void 方法仅作为日志适配器；生产生命周期接线必须调用检查式方法，
// 并在其返回错误时保留 Service 以便重试。
func (s *Service) Shutdown(ctx context.Context) {
	if err := s.ShutdownChecked(ctx); err != nil {
		slog.Error("workorch service shutdown incomplete", "err", err)
	}
}

func (s *Service) BeginShutdown(ctx context.Context) error {
	if s == nil || s.http == nil {
		return nil
	}
	// 准入门禁与 Run 的任务登记串行化，但 listener I/O
	// 或等待 handler 时不得持有 service 锁。
	s.runMu.Lock()
	s.http.mu.Lock()
	s.http.stopping = true
	s.http.notifyLocked()
	s.http.mu.Unlock()
	s.runMu.Unlock()
	return s.http.BeginShutdown(ctx)
}

func (s *Service) Quiesce(ctx context.Context) error {
	return s.DrainHTTPChecked(ctx)
}

func (s *Service) ShutdownChecked(ctx context.Context) error {
	if err := s.DrainHTTPChecked(ctx); err != nil {
		return err
	}
	return s.CloseTransportStateChecked(ctx)
}

func (s *Service) DrainHTTP(ctx context.Context) {
	if err := s.DrainHTTPChecked(ctx); err != nil {
		slog.Error("workorch http server drain incomplete", "err", err)
	}
}

// DrainHTTPChecked 等待真实 handler、连接及已接纳 server 任务退出。
// context 超时既不消耗完成回执，也不释放状态。
func (s *Service) DrainHTTPChecked(ctx context.Context) error {
	if s == nil || s.http == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := s.BeginShutdown(ctx); err != nil {
		return err
	}
	if err := s.http.Drain(ctx); err != nil {
		return err
	}
	s.runMu.Lock()
	exited := s.runExited
	s.runMu.Unlock()
	if exited == nil {
		return nil
	}
	select {
	case <-exited:
		// Start/Serve 失败属于运行结果，已保留在 runErr 中并通过
		// Done/ServiceStopped 发布；它不表示 server 任务仍在运行，
		// 不能永久阻止已真实退出任务的资源清理。
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Service) CloseTransportState(ctx context.Context) {
	if err := s.CloseTransportStateChecked(ctx); err != nil {
		slog.Error("workorch transport state shutdown incomplete", "err", err)
	}
}

func (s *Service) CloseTransportStateChecked(ctx context.Context) error {
	if s == nil || s.http == nil {
		return nil
	}
	if err := s.DrainHTTPChecked(ctx); err != nil {
		return err
	}
	return s.http.CloseTransportState(ctx)
}

func (s *Service) ListWorkspaceView(ctx context.Context) ([]appmodelcore.Workspace, error) {
	if s == nil || s.http == nil {
		return nil, errors.New("workorch http server is not configured")
	}
	return s.http.ListWorkspaceView(ctx)
}

func (s *Service) ReadWorkspaceViewSnapshot(ctx context.Context, workspaceID string) (readmodelcore.WorkspaceSnapshotState, error) {
	if s == nil || s.http == nil {
		return readmodelcore.WorkspaceSnapshotState{}, errors.New("workorch http server is not configured")
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
	if s == nil || s.http == nil || s.http.state == nil {
		return 0, errors.New("workspace projection is not configured")
	}
	if s.http.state.workspaceProjectionWriter == nil {
		return 0, errors.New("workspace projection is not configured")
	}
	if err := s.http.state.workspaceProjectionWriter.RebuildAll(ctx); err != nil {
		return 0, err
	}
	return s.http.state.workspaceProjectionStatus().GlobalRevision, nil
}

func (s *Service) Done() <-chan error {
	if s == nil || s.done == nil {
		closed := make(chan error)
		close(closed)
		return closed
	}
	return s.done
}
