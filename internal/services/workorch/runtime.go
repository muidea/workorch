package workorch

import (
	"context"
	"fmt"
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
	// A failed Setup still requires checked cleanup of entered framework phases.
	s.started = true
	s.serverStopped = make(chan struct{})
	s.serverStopOnce = sync.Once{}
	err := application.StartupWithOptions(ctx, &frameworkModuleService{delegate: frameworkservice.DefaultService()}, application.Options{
		ConfigDir:   s.configDir,
		ServiceName: "workorch",
	})
	if err != nil {
		return err
	}
	hub := application.Get().EventHub()
	if hub == nil {
		return common.ErrInvalidArgument
	}
	return s.subscribeServerStopped(hub)
}

func (s *frameworkRuntime) subscribeServerStopped(hub magicevent.Hub) error {
	observer := magicevent.NewSimpleObserverWithMatchID(serverStopObserverID, workorchdevents.NotificationDestination, hub)
	if err := observer.Subscribe(workorchdevents.TopicWorkorchDServiceStopped, s.handleServerStopped); err != nil {
		return err
	}
	s.serverStopObserver = observer
	return nil
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
	shutdownCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if s.serverStopObserver != nil {
		if err := s.serverStopObserver.Unsubscribe(workorchdevents.TopicWorkorchDServiceStopped); err != nil {
			return err
		}
		s.serverStopObserver = nil
	}
	s.serverStopOnce.Do(func() { close(s.serverStopped) })
	if err := application.ShutdownChecked(shutdownCtx); err != nil {
		return err
	}
	s.started = false
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
