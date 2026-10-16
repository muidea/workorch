package server

import (
	"context"
	"net"
	"net/http"
	"sync"
	"time"
)

const (
	configServerAddr              = "server.addr"
	configServerReadHeaderTimeout = "server.readHeaderTimeout"
	configServerReadTimeout       = "server.readTimeout"
	configServerWriteTimeout      = "server.writeTimeout"
	configServerIdleTimeout       = "server.idleTimeout"
	configServerShutdownTimeout   = "server.shutdownTimeout"

	defaultServerAddr           = "127.0.0.1:8080"
	defaultReadHeaderTimeout    = 5 * time.Second
	defaultReadTimeout          = 15 * time.Second
	defaultWriteTimeout         = 180 * time.Second
	defaultIdleTimeout          = 60 * time.Second
	defaultShutdownTimeout      = 5 * time.Second
	runSnapshotCacheTTL         = 2 * time.Second
	defaultEventRecordPageLimit = 100
	maxEventRecordPageLimit     = 500
)

type Config struct {
	Addr              string
	Backend           string
	DataRoot          string
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	ShutdownTimeout   time.Duration
}

type HTTPServer struct {
	config Config
	http   *http.Server
	state  *serverState

	// 准入登记与真实退出共用短锁，避免等待方刚观察到无在途工作，
	// 就又有新 handler 或连接被接纳的竞态。
	mu             sync.Mutex
	stopping       bool
	listener       *shutdownListener
	connections    map[net.Conn]struct{}
	activeHandlers int
	changed        chan struct{}
	shutdown       *httpShutdown
}

type httpShutdown struct {
	done chan struct{}
	err  error // 关闭 done 时发布，此后不可变
}

func (s *HTTPServer) notifyLocked() {
	if s.changed != nil {
		close(s.changed)
	}
	s.changed = make(chan struct{})
}

// trackHandler 仅在业务路由外接线一次，原样传递 Request 和 ResponseWriter，
// 保留 writer 的可选接口。ConnState 不是 handler 完成回执，
// 尤其不能将 hijacking 或 HTTP/2 的连接状态当作单个 handler 已退出。
func (s *HTTPServer) trackHandler(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		if s.stopping {
			s.mu.Unlock()
			http.Error(w, "http server is shutting down", http.StatusServiceUnavailable)
			return
		}
		s.activeHandlers++
		s.notifyLocked()
		s.mu.Unlock()
		defer func() {
			s.mu.Lock()
			s.activeHandlers--
			s.notifyLocked()
			s.mu.Unlock()
		}()
		handler.ServeHTTP(w, r)
	})
}

// 连接生命周期是独立的第二道屏障，不能替代 handler 的真实返回。
// net/http 在连接清理后报告 StateClosed；hijacking 会更早离开 net/http 管理，
// 此时 handler 仍由 wrapper 跟踪。
func (s *HTTPServer) observeConnection(conn net.Conn, state http.ConnState) {
	if s == nil {
		return
	}
	s.mu.Lock()
	switch state {
	case http.StateNew:
		if s.connections == nil {
			s.connections = make(map[net.Conn]struct{})
		}
		s.connections[conn] = struct{}{}
	case http.StateHijacked, http.StateClosed:
		delete(s.connections, conn)
	}
	s.notifyLocked()
	s.mu.Unlock()
}

func (s *HTTPServer) waitExits(ctx context.Context) error {
	for {
		s.mu.Lock()
		if s.activeHandlers == 0 && len(s.connections) == 0 {
			s.mu.Unlock()
			return nil
		}
		if s.changed == nil {
			s.changed = make(chan struct{})
		}
		changed := s.changed
		s.mu.Unlock()
		select {
		case <-changed:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// net/http 与 BeginShutdown 都会关闭 listener；成功关闭只执行一次，
// 失败则保留句柄供下次重试。Accept 与 handler 登记共用准入门禁，
// 避免与 BeginShutdown 并发接纳的连接在排空已观察到零在途后进入 net/http。
type shutdownListener struct {
	net.Listener
	server *HTTPServer
	mu     sync.Mutex
	closed bool
}

func (l *shutdownListener) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return nil
	}
	if err := l.Listener.Close(); err != nil {
		return err
	}
	l.closed = true
	return nil
}

func (l *shutdownListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	s := l.server
	s.mu.Lock()
	if s.stopping {
		s.mu.Unlock()
		_ = conn.Close()
		return nil, net.ErrClosed
	}
	if s.connections == nil {
		s.connections = make(map[net.Conn]struct{})
	}
	s.connections[conn] = struct{}{}
	s.notifyLocked()
	s.mu.Unlock()
	return conn, nil
}
