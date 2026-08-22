package server

import (
	"context"
	"errors"
	"fmt"
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
	server.http.ConnState = server.observeConnection
	return server
}

func (s *HTTPServer) Start() error {
	if s == nil || s.http == nil {
		return errors.New("http server is not configured")
	}
	err := s.http.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (s *HTTPServer) Validate(ctx context.Context) error {
	if s == nil {
		return errors.New("http server is not configured")
	}
	return s.state.validate(ctx, s.config.Backend, s.config.DataRoot)
}

func (s *HTTPServer) Shutdown(ctx context.Context) error {
	if s == nil {
		return nil
	}
	return errors.Join(s.Drain(ctx), s.CloseTransportState(ctx))
}

func (s *HTTPServer) Drain(ctx context.Context) error {
	if s == nil || s.http == nil {
		return nil
	}
	shutdownErr := s.http.Shutdown(ctx)
	var closeErr error
	if shutdownErr != nil {
		closeErr = s.http.Close()
	}
	// Close terminates connections but does not wait for their handlers. Keep
	// every Biz-owned dependency live until each accepted connection has
	// actually left net/http, including the forced-close timeout path.
	s.connections.Wait()
	return errors.Join(shutdownErr, closeErr)
}

func (s *HTTPServer) CloseTransportState(ctx context.Context) error {
	if s == nil || s.state == nil {
		return nil
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
