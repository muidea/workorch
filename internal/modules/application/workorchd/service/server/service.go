package server

import (
	"context"
	"errors"
	"log/slog"
	"sync/atomic"

	cd "github.com/muidea/magicCommon/def"
	"github.com/muidea/magicCommon/event"
	"github.com/muidea/magicCommon/task"
	workorchevents "github.com/muidea/workorch/internal/pkg/events"
)

type Service struct {
	config            Config
	http              *HTTPServer
	eventHub          event.Hub
	backgroundRoutine task.BackgroundRoutine
	done              chan error
	runStarted        atomic.Bool
}

func NewService(config Config) *Service {
	return &Service{config: normalizeConfig(config)}
}

func (s *Service) Startup(ctx context.Context, serviceName string, eventHub event.Hub, backgroundRoutine task.BackgroundRoutine) *cd.Error {
	if s == nil {
		return cd.NewError(cd.IllegalParam, "workorch service is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	s.eventHub = eventHub
	s.backgroundRoutine = backgroundRoutine
	s.http = NewHTTPServerWithEventHub(s.config, workorchevents.NewMagicHubWithHubOwner(eventHub, "workorchd/http"))
	s.done = make(chan error, 1)
	s.runStarted.Store(false)
	if err := s.http.Validate(ctx); err != nil {
		return cd.NewError(cd.Unexpected, err.Error())
	}
	slog.Info("workorch service startup completed", "service", serviceName, "addr", s.config.Addr)
	return nil
}

func (s *Service) Run(_ context.Context) *cd.Error {
	if s == nil || s.http == nil {
		return cd.NewError(cd.IllegalParam, "workorch http server is not configured")
	}
	s.runStarted.Store(true)
	go func() {
		err := s.http.Start()
		if err != nil {
			slog.Error("workorch http server stopped with error", "err", err)
		}
		s.done <- err
		close(s.done)
	}()
	return nil
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

func (s *Service) Done() <-chan error {
	if s == nil || s.done == nil {
		closed := make(chan error)
		close(closed)
		return closed
	}
	return s.done
}
