package server

import (
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

	defaultServerAddr           = ":8080"
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
	config      Config
	http        *http.Server
	state       *serverState
	connections sync.WaitGroup
}

// observeConnection tracks transport lifetime only. It does not classify a
// request or retain business state; Drain uses it solely to avoid tearing
// down handler dependencies before a force-closed connection has returned.
func (s *HTTPServer) observeConnection(_ net.Conn, state http.ConnState) {
	if s == nil {
		return
	}
	switch state {
	case http.StateNew:
		s.connections.Add(1)
	case http.StateHijacked, http.StateClosed:
		s.connections.Done()
	}
}
