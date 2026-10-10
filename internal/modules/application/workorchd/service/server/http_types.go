package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"

	"github.com/muidea/magicCommon/task"
	engine "github.com/muidea/magicEngine/http"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

func ConfigFromEnv() (Config, error) {
	cfg, err := workorchconfig.Current()
	if err != nil {
		return Config{}, err
	}
	return ConfigFromWorkorch(cfg)
}

func ConfigFromWorkorch(cfg workorchconfig.Config) (Config, error) {
	readHeaderTimeout, err := durationFromValue(configServerReadHeaderTimeout, cfg.Server.ReadHeaderTimeout, defaultReadHeaderTimeout)
	if err != nil {
		return Config{}, err
	}
	readTimeout, err := durationFromValue(configServerReadTimeout, cfg.Server.ReadTimeout, defaultReadTimeout)
	if err != nil {
		return Config{}, err
	}
	writeTimeout, err := durationFromValue(configServerWriteTimeout, cfg.Server.WriteTimeout, defaultWriteTimeout)
	if err != nil {
		return Config{}, err
	}
	idleTimeout, err := durationFromValue(configServerIdleTimeout, cfg.Server.IdleTimeout, defaultIdleTimeout)
	if err != nil {
		return Config{}, err
	}
	shutdownTimeout, err := durationFromValue(configServerShutdownTimeout, cfg.Server.ShutdownTimeout, defaultShutdownTimeout)
	if err != nil {
		return Config{}, err
	}
	return Config{
		Addr:              firstNonEmpty(cfg.Server.Addr, defaultServerAddr),
		Backend:           cfg.Repository.Backend,
		DataRoot:          cfg.Repository.DataDir,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
		ShutdownTimeout:   shutdownTimeout,
	}, nil
}

func ConfigFromValues(values map[string]string) (Config, error) {
	addr := values[configServerAddr]
	if addr == "" {
		addr = defaultServerAddr
	}
	readHeaderTimeout, err := durationFromValues(values, configServerReadHeaderTimeout, defaultReadHeaderTimeout)
	if err != nil {
		return Config{}, err
	}
	readTimeout, err := durationFromValues(values, configServerReadTimeout, defaultReadTimeout)
	if err != nil {
		return Config{}, err
	}
	writeTimeout, err := durationFromValues(values, configServerWriteTimeout, defaultWriteTimeout)
	if err != nil {
		return Config{}, err
	}
	idleTimeout, err := durationFromValues(values, configServerIdleTimeout, defaultIdleTimeout)
	if err != nil {
		return Config{}, err
	}
	shutdownTimeout, err := durationFromValues(values, configServerShutdownTimeout, defaultShutdownTimeout)
	if err != nil {
		return Config{}, err
	}
	return Config{
		Addr:              addr,
		Backend:           values["repository.backend"],
		DataRoot:          values["repository.dataDir"],
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
		ShutdownTimeout:   shutdownTimeout,
	}, nil
}

func normalizeConfig(config Config) Config {
	if config.Addr == "" {
		config.Addr = defaultServerAddr
	}
	if config.ReadHeaderTimeout <= 0 {
		config.ReadHeaderTimeout = defaultReadHeaderTimeout
	}
	if config.ReadTimeout <= 0 {
		config.ReadTimeout = defaultReadTimeout
	}
	if config.WriteTimeout <= 0 {
		config.WriteTimeout = defaultWriteTimeout
	}
	if config.IdleTimeout <= 0 {
		config.IdleTimeout = defaultIdleTimeout
	}
	if config.ShutdownTimeout <= 0 {
		config.ShutdownTimeout = defaultShutdownTimeout
	}
	return config
}

func NewHTTPServer(config Config) *HTTPServer {
	return newHTTPServerWithApplicationPorts(normalizeConfig(config), ApplicationPorts{}, nil, errors.New("typed application ports are required"))
}

// NewHTTPServerWithApplicationPorts constructs the transport adapter from a
// Biz-owned, typed application boundary. The server does not retain a runtime
// session or EventHub on this production path.
func NewHTTPServerWithApplicationPorts(config Config, ports ApplicationPorts, backgroundRoutine task.BackgroundRoutine) *HTTPServer {
	return newHTTPServerWithApplicationPorts(normalizeConfig(config), ports, backgroundRoutine, nil)
}

func newHTTPServerWithApplicationPorts(config Config, ports ApplicationPorts, backgroundRoutine task.BackgroundRoutine, portsErr error) *HTTPServer {
	config = normalizeConfig(config)
	state := newServerStateWithApplicationPorts(config, ports, backgroundRoutine, portsErr)
	return newHTTPServerWithState(config, state)
}

func newHTTPServerWithState(config Config, state *serverState) *HTTPServer {
	handler := newMuxWithStateNoStart(state)
	return newHTTPServerWithHandler(config, state, handler)
}

func newHTTPServerWithHandler(config Config, state *serverState, handler http.Handler) *HTTPServer {
	server := &HTTPServer{
		config: config,
		state:  state,
		http: &http.Server{
			Addr:              config.Addr,
			Handler:           handler,
			ReadHeaderTimeout: config.ReadHeaderTimeout,
			ReadTimeout:       config.ReadTimeout,
			WriteTimeout:      config.WriteTimeout,
			IdleTimeout:       config.IdleTimeout,
		},
	}
	server.http.Handler = server.trackHandler(handler)
	server.http.ConnState = server.observeConnection
	return server
}

func (s *HTTPServer) Start() error {
	if s == nil || s.http == nil {
		return errors.New("http server is not configured")
	}
	s.mu.Lock()
	stopping := s.stopping
	s.mu.Unlock()
	if stopping {
		return nil
	}
	addr := s.http.Addr
	if addr == "" {
		addr = ":http"
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	return s.serve(listener)
}

func (s *HTTPServer) serve(listener net.Listener) error {
	tracked := &shutdownListener{Listener: listener, server: s}
	s.mu.Lock()
	if s.stopping {
		// Start 可能恰在 BeginShutdown 关闭门禁时打开 socket；
		// 即使首次关闭失败，也必须保留该句柄。
		if s.listener == nil {
			s.listener = tracked
		}
		s.mu.Unlock()
		return tracked.Close()
	}
	if s.listener != nil {
		s.mu.Unlock()
		return errors.Join(errors.New("http server is already started"), tracked.Close())
	}
	s.listener = tracked
	s.mu.Unlock()
	err := s.http.Serve(tracked)
	// listener 自然关闭失败也必须保留为检查式生命周期错误，
	// 不能只作为异步 Serve 结果；net/http 会忽略 deferred Close 的错误。
	closeErr := tracked.Close()
	if errors.Is(err, http.ErrServerClosed) || errors.Is(err, net.ErrClosed) {
		err = nil
	}
	return errors.Join(err, closeErr)
}

func (s *HTTPServer) Validate(ctx context.Context) error {
	if s == nil {
		return errors.New("http server is not configured")
	}
	return s.state.validate(ctx, s.config.Backend, s.config.DataRoot)
}

func (s *HTTPServer) Shutdown(ctx context.Context) error {
	if err := s.Drain(ctx); err != nil {
		return err
	}
	return s.CloseTransportState(ctx)
}

// BeginShutdown 关闭准入与 listener，不等待已接纳 handler 退出。
// context 取消不能成为保持入站开放的理由；仅返回真实 listener 关闭错误，
// 并保留句柄供后续重试。
func (s *HTTPServer) BeginShutdown(_ context.Context) error {
	if s == nil || s.http == nil {
		return nil
	}
	s.mu.Lock()
	s.stopping = true
	listener := s.listener
	s.notifyLocked()
	s.mu.Unlock()
	s.http.SetKeepAlivesEnabled(false)
	if listener != nil {
		return listener.Close()
	}
	return nil
}

func (s *HTTPServer) Quiesce(ctx context.Context) error {
	return s.Drain(ctx)
}

func (s *HTTPServer) Drain(ctx context.Context) error {
	if s == nil || s.http == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := s.BeginShutdown(ctx); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// net/http 的 listenerGroup.Wait 不响应 context。由本实例持有唯一的真实
	// Shutdown 操作，并在调用方超时后保留完成回执；
	// 不因超时创建替代等待操作，也不强制 Close。
	s.mu.Lock()
	if s.shutdown == nil {
		s.shutdown = &httpShutdown{done: make(chan struct{})}
		operation := s.shutdown
		go func() {
			operation.err = s.http.Shutdown(context.Background())
			close(operation.done)
		}()
	}
	operation := s.shutdown
	s.mu.Unlock()
	select {
	case <-operation.done:
		if operation.err != nil {
			s.mu.Lock()
			if s.shutdown == operation {
				// 只有失败操作已真实退出后，才允许重试。
				s.shutdown = nil
			}
			s.mu.Unlock()
			return operation.err
		}
	case <-ctx.Done():
		return ctx.Err()
	}
	return s.waitExits(ctx)
}

func (s *HTTPServer) CloseTransportState(ctx context.Context) error {
	if s == nil || s.state == nil {
		return nil
	}
	// 即使调用方跳过显式排空阶段，也不能在已接纳 handler 或连接
	// 仍使用依赖时释放它们。
	if err := s.Drain(ctx); err != nil {
		return err
	}
	return s.state.close(ctx)
}

func newMuxWithStateNoStart(state *serverState) http.Handler {
	router := engine.NewRouteRegistry()
	registerSystemRoutes(router, state)
	registerConfigurationRoutes(router, state)
	registerReadRoutes(router, state)
	registerBusinessWriteRoutes(router, state)
	registerConsoleRoutes(router)
	engineServer := engine.NewHTTPServer()
	engineServer.Bind(router)
	handler, ok := engineServer.(http.Handler)
	if !ok {
		return http.NotFoundHandler()
	}
	return handler
}

func newServerStateWithApplicationPorts(config Config, ports ApplicationPorts, backgroundRoutine task.BackgroundRoutine, portsErr error) *serverState {
	if portsErr == nil {
		portsErr = ports.Validate()
	}
	state := newServerStateBase(config, backgroundRoutine, applicationOperationsFromPorts(ports))
	if portsErr != nil {
		applicationPortsErr := fmt.Errorf("workorch application ports unavailable: %w", portsErr)
		state.validate = func(context.Context, string, string) error { return applicationPortsErr }
		return state
	}
	if state.validate == nil {
		applicationPortsErr := errors.New("workorch application ports are not configured")
		state.validate = func(context.Context, string, string) error { return applicationPortsErr }
	}
	return state
}
