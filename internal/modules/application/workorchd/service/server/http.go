package server

import (
	"net/http"
	"time"
)

const (
	configServerAddr              = "server.addr"
	configServerReadHeaderTimeout = "server.readHeaderTimeout"
	configServerReadTimeout       = "server.readTimeout"
	configServerWriteTimeout      = "server.writeTimeout"
	configServerIdleTimeout       = "server.idleTimeout"
	configServerShutdownTimeout   = "server.shutdownTimeout"
	configServerMaxConcurrentRuns = "server.maxConcurrentRuns"

	defaultServerAddr                 = ":8080"
	defaultReadHeaderTimeout          = 5 * time.Second
	defaultReadTimeout                = 15 * time.Second
	defaultWriteTimeout               = 180 * time.Second
	defaultIdleTimeout                = 60 * time.Second
	defaultShutdownTimeout            = 5 * time.Second
	defaultMaxConcurrentRuns    int64 = 1
	runSnapshotCacheTTL               = 2 * time.Second
	defaultEventRecordPageLimit       = 100
	maxEventRecordPageLimit           = 500
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
	MaxConcurrentRuns int64
}

type HTTPServer struct {
	config Config
	http   *http.Server
	state  *serverState
}
