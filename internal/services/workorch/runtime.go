package workorch

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	magicevent "github.com/muidea/magicCommon/event"

	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	workorchservicecommon "github.com/muidea/workorch/internal/services/workorch/pkg/common"

	"github.com/muidea/magicCommon/framework/application"
	frameworkservice "github.com/muidea/magicCommon/framework/service"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

const (
	defaultApplicationShutdownTimeout = 30 * time.Second
	serverStopObserverID              = workorchservicecommon.ModuleID + "/server-stop"
)

type frameworkRuntime struct {
	configDir          string
	started            bool
	serverStopped      chan struct{}
	serverStopOnce     sync.Once
	serverStopObserver magicevent.SimpleObserver
	serverStopData     workorchdevents.ServiceStoppedData
}

func (s *frameworkRuntime) Startup(ctx context.Context) error {
	ConfigureDirectory(s.configDir)
	if err := EnsureConfigFile(s.configDir); err != nil {
		return err
	}
	err := application.StartupWithOptions(ctx, frameworkservice.DefaultService(), application.Options{
		ConfigDir:   s.configDir,
		ServiceName: "workorch",
	})
	if err != nil {
		return err
	}
	s.started = true
	s.serverStopped = make(chan struct{})
	s.serverStopOnce = sync.Once{}
	hub := application.Get().EventHub()
	if hub == nil {
		return common.ErrInvalidArgument
	}
	s.subscribeServerStopped(hub)
	return nil
}

func (s *frameworkRuntime) subscribeServerStopped(hub magicevent.Hub) {
	s.serverStopObserver = magicevent.NewSimpleObserverWithMatchID(serverStopObserverID, workorchdevents.NotificationDestination, hub)
	s.serverStopObserver.Subscribe(workorchdevents.TopicWorkorchDServiceStopped, s.handleServerStopped)
}

func (s *frameworkRuntime) Run(ctx context.Context) error {
	if !s.started {
		return common.ErrInvalidArgument
	}
	if err := application.Run(ctx); err != nil {
		return err
	}
	return nil
}

func (s *frameworkRuntime) Shutdown(ctx context.Context) error {
	if !s.started {
		return nil
	}
	timeout := applicationShutdownTimeout()
	if timeout <= 0 {
		application.Shutdown(ctx)
		s.started = false
		return nil
	}
	shutdownCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if s.serverStopObserver != nil {
		s.serverStopObserver.Unsubscribe(workorchdevents.TopicWorkorchDServiceStopped)
		s.serverStopObserver = nil
	}
	s.serverStopOnce.Do(func() { close(s.serverStopped) })
	application.Shutdown(shutdownCtx)
	s.started = false
	if err := shutdownCtx.Err(); err != nil {
		slog.Error("workorch application shutdown context finished", "timeout", timeout, "err", err)
	}
	return nil
}

func applicationShutdownTimeout() time.Duration {
	timeout := defaultApplicationShutdownTimeout
	cfg, err := workorchconfig.Current()
	if err != nil {
		return timeout
	}
	raw := strings.TrimSpace(cfg.Server.ShutdownTimeout)
	if raw == "" {
		return timeout
	}
	parsed, err := time.ParseDuration(raw)
	if err != nil || parsed <= 0 {
		return timeout
	}
	return parsed + 10*time.Second
}

func (s *frameworkRuntime) waitServer(ctx context.Context) error {
	if s == nil || s.serverStopped == nil {
		return common.ErrInvalidArgument
	}
	select {
	case <-s.serverStopped:
		if s.serverStopData.State == "failed" {
			return fmt.Errorf("%s: %s", s.serverStopData.ErrorCode, s.serverStopData.ErrorMessage)
		}
		return nil
	case <-ctx.Done():
		return nil
	}
}

func (s *frameworkRuntime) handleServerStopped(ev magicevent.Event, _ magicevent.Result) {
	data, ok := ev.Data().(workorchdevents.ServiceStoppedData)
	if !ok || s == nil || s.serverStopped == nil {
		return
	}
	s.serverStopOnce.Do(func() {
		s.serverStopData = data
		close(s.serverStopped)
	})
}
