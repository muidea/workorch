package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	engine "github.com/muidea/magicEngine/http"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice"
	"github.com/muidea/workorch/internal/modules/application/workorchd/service/server/consoleassets"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

const (
	envServerAddr              = "WORKORCH_SERVER_ADDR"
	envServerReadHeaderTimeout = "WORKORCH_SERVER_READ_HEADER_TIMEOUT"
	envServerReadTimeout       = "WORKORCH_SERVER_READ_TIMEOUT"
	envServerWriteTimeout      = "WORKORCH_SERVER_WRITE_TIMEOUT"
	envServerIdleTimeout       = "WORKORCH_SERVER_IDLE_TIMEOUT"
	envServerShutdownTimeout   = "WORKORCH_SERVER_SHUTDOWN_TIMEOUT"
	envServerMaxConcurrentRuns = "WORKORCH_SERVER_MAX_CONCURRENT_RUNS"

	defaultServerAddr              = ":8080"
	defaultReadHeaderTimeout       = 5 * time.Second
	defaultReadTimeout             = 15 * time.Second
	defaultWriteTimeout            = 30 * time.Second
	defaultIdleTimeout             = 60 * time.Second
	defaultShutdownTimeout         = 5 * time.Second
	defaultMaxConcurrentRuns int64 = 1
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

type statusResponse struct {
	Service string `json:"service"`
	Status  string `json:"status"`
}

type runResponse struct {
	Service  string             `json:"service"`
	Backend  string             `json:"backend,omitempty"`
	DataRoot string             `json:"dataRoot,omitempty"`
	Slices   []appservice.Slice `json:"slices"`
}

type workspaceResponse struct {
	Service   string               `json:"service"`
	Backend   string               `json:"backend,omitempty"`
	DataRoot  string               `json:"dataRoot,omitempty"`
	Workspace appservice.Workspace `json:"workspace"`
}

type workspacesResponse struct {
	Service    string                 `json:"service"`
	Backend    string                 `json:"backend,omitempty"`
	DataRoot   string                 `json:"dataRoot,omitempty"`
	Workspaces []appservice.Workspace `json:"workspaces"`
}

type runInstanceResponse struct {
	Service  string         `json:"service"`
	Backend  string         `json:"backend,omitempty"`
	DataRoot string         `json:"dataRoot,omitempty"`
	Run      appservice.Run `json:"run"`
}

type runSnapshotResponse struct {
	Service  string                 `json:"service"`
	Backend  string                 `json:"backend,omitempty"`
	DataRoot string                 `json:"dataRoot,omitempty"`
	Snapshot appservice.RunSnapshot `json:"snapshot"`
}

type runStatusResponse struct {
	Service  string               `json:"service"`
	Backend  string               `json:"backend,omitempty"`
	DataRoot string               `json:"dataRoot,omitempty"`
	Status   appservice.RunStatus `json:"status"`
}

type runsResponse struct {
	Service  string           `json:"service"`
	Backend  string           `json:"backend,omitempty"`
	DataRoot string           `json:"dataRoot,omitempty"`
	Runs     []appservice.Run `json:"runs"`
}

type workspaceSnapshotResponse struct {
	Service  string                       `json:"service"`
	Backend  string                       `json:"backend,omitempty"`
	DataRoot string                       `json:"dataRoot,omitempty"`
	Snapshot appservice.WorkspaceSnapshot `json:"snapshot"`
}

type decisionResponse struct {
	Service  string              `json:"service"`
	Backend  string              `json:"backend,omitempty"`
	DataRoot string              `json:"dataRoot,omitempty"`
	Decision appservice.Decision `json:"decision"`
}

type decisionsResponse struct {
	Service   string                `json:"service"`
	Backend   string                `json:"backend,omitempty"`
	DataRoot  string                `json:"dataRoot,omitempty"`
	Decisions []appservice.Decision `json:"decisions"`
}

type approvalResponse struct {
	Service  string              `json:"service"`
	Backend  string              `json:"backend,omitempty"`
	DataRoot string              `json:"dataRoot,omitempty"`
	Approval appservice.Approval `json:"approval"`
}

type approvalsResponse struct {
	Service   string                `json:"service"`
	Backend   string                `json:"backend,omitempty"`
	DataRoot  string                `json:"dataRoot,omitempty"`
	Approvals []appservice.Approval `json:"approvals"`
}

type createWorkspaceRequest struct {
	WorkspaceID string `json:"workspaceId"`
	Title       string `json:"title"`
}

type createRunRequest struct {
	RunID  string `json:"runId"`
	GoalID string `json:"goalId,omitempty"`
	DoDID  string `json:"doDId,omitempty"`
}

type sliceCatalogResponse struct {
	Service string                       `json:"service"`
	Slices  []appservice.SliceDescriptor `json:"slices"`
}

type sliceSnapshotsResponse struct {
	Service string             `json:"service"`
	Slices  []appservice.Slice `json:"slices"`
}

type sliceStatusesResponse struct {
	Service  string                   `json:"service"`
	Statuses []appservice.SliceStatus `json:"statuses"`
}

type sliceRunResponse struct {
	Service  string           `json:"service"`
	Backend  string           `json:"backend,omitempty"`
	DataRoot string           `json:"dataRoot,omitempty"`
	Slice    appservice.Slice `json:"slice"`
}

type sliceStatusResponse struct {
	Service  string                 `json:"service"`
	Backend  string                 `json:"backend,omitempty"`
	DataRoot string                 `json:"dataRoot,omitempty"`
	Status   appservice.SliceStatus `json:"status"`
}

type errorResponse struct {
	Error string `json:"error"`
}

type runFunc func(context.Context, string, string) ([]appservice.Slice, error)
type createWorkspaceFunc func(context.Context, string, string, string, string) (appservice.Workspace, error)
type readWorkspaceFunc func(context.Context, string, string, string) (appservice.Workspace, error)
type readWorkspaceSnapshotFunc func(context.Context, string, string, string) (appservice.WorkspaceSnapshot, error)
type listWorkspacesFunc func(context.Context, string, string) ([]appservice.Workspace, error)
type createRunInstanceFunc func(context.Context, string, string, string, string, string, string) (appservice.Run, error)
type readRunInstanceFunc func(context.Context, string, string, string) (appservice.Run, error)
type readRunSnapshotFunc func(context.Context, string, string, string) (appservice.RunSnapshot, error)
type readRunStatusFunc func(context.Context, string, string, string) (appservice.RunStatus, error)
type listRunInstancesFunc func(context.Context, string, string, string) ([]appservice.Run, error)
type startRunInstanceFunc func(context.Context, string, string, string) (appservice.Run, error)
type resumeRunInstanceFunc func(context.Context, string, string, string) (appservice.Run, error)
type listDecisionsFunc func(context.Context, string, string, string, string) ([]appservice.Decision, error)
type readDecisionFunc func(context.Context, string, string, string) (appservice.Decision, error)
type resolveDecisionFunc func(context.Context, string, string, string) (appservice.Decision, error)
type rejectDecisionFunc func(context.Context, string, string, string) (appservice.Decision, error)
type listApprovalsFunc func(context.Context, string, string, string, string) ([]appservice.Approval, error)
type readApprovalFunc func(context.Context, string, string, string) (appservice.Approval, error)
type approveApprovalFunc func(context.Context, string, string, string) (appservice.Approval, error)
type rejectApprovalFunc func(context.Context, string, string, string) (appservice.Approval, error)
type runOneFunc func(context.Context, string, string, string) (appservice.Slice, error)
type readOneFunc func(context.Context, string, string, string) (appservice.Slice, error)
type statusOneFunc func(context.Context, string, string, string) (appservice.SliceStatus, error)
type listSnapshotsFunc func(context.Context, string, string) ([]appservice.Slice, error)
type listStatusesFunc func(context.Context, string, string) ([]appservice.SliceStatus, error)
type listSlicesFunc func() []appservice.SliceDescriptor
type validateFunc func(context.Context, string, string) error

type serverState struct {
	config                Config
	eventHub              events.Hub
	metrics               *serverMetrics
	validate              validateFunc
	createWorkspace       createWorkspaceFunc
	readWorkspace         readWorkspaceFunc
	readWorkspaceSnapshot readWorkspaceSnapshotFunc
	listWorkspaces        listWorkspacesFunc
	createRun             createRunInstanceFunc
	readRun               readRunInstanceFunc
	readRunSnapshot       readRunSnapshotFunc
	readRunStatus         readRunStatusFunc
	listRuns              listRunInstancesFunc
	startRun              startRunInstanceFunc
	resumeRun             resumeRunInstanceFunc
	listDecisions         listDecisionsFunc
	readDecision          readDecisionFunc
	resolveDecision       resolveDecisionFunc
	rejectDecision        rejectDecisionFunc
	listApprovals         listApprovalsFunc
	readApproval          readApprovalFunc
	approveApproval       approveApprovalFunc
	rejectApproval        rejectApprovalFunc
	listSlices            listSlicesFunc
	listSnapshots         listSnapshotsFunc
	listStatuses          listStatusesFunc
	run                   runFunc
	runOne                runOneFunc
	readOne               readOneFunc
	statusOne             statusOneFunc
	runQueueMu            sync.Mutex
	runQueueWaiters       []chan struct{}
	activeRunSlots        int64
	sharedSession         *appservice.RuntimeSession
}

type serverMetrics struct {
	healthzRequests             atomic.Uint64
	readyzRequests              atomic.Uint64
	metricsRequests             atomic.Uint64
	workspaceCreateRequests     atomic.Uint64
	workspaceListRequests       atomic.Uint64
	workspaceGetRequests        atomic.Uint64
	workspaceSnapshotRequests   atomic.Uint64
	runCreateRequests           atomic.Uint64
	runListRequests             atomic.Uint64
	runGetRequests              atomic.Uint64
	runSnapshotRequests         atomic.Uint64
	runStatusRequests           atomic.Uint64
	runStartRequests            atomic.Uint64
	runResumeRequests           atomic.Uint64
	decisionListRequests        atomic.Uint64
	decisionGetRequests         atomic.Uint64
	decisionResolveRequests     atomic.Uint64
	decisionRejectRequests      atomic.Uint64
	approvalListRequests        atomic.Uint64
	approvalGetRequests         atomic.Uint64
	approvalApproveRequests     atomic.Uint64
	approvalRejectRequests      atomic.Uint64
	sliceCatalogRequests        atomic.Uint64
	sliceSnapshotsRequests      atomic.Uint64
	sliceStatusesRequests       atomic.Uint64
	singleSliceSnapshotRequests atomic.Uint64
	sliceStatusRequests         atomic.Uint64
	runRequests                 atomic.Uint64
	singleRunRequests           atomic.Uint64
	runSucceeded                atomic.Uint64
	runFailed                   atomic.Uint64
	runRejected                 atomic.Uint64
	runCanceled                 atomic.Uint64
	runInFlight                 atomic.Int64
	runQueueWaiting             atomic.Int64
	runQueuedTotal              atomic.Uint64
}

func ConfigFromEnv() (Config, error) {
	addr := os.Getenv(envServerAddr)
	if addr == "" {
		addr = defaultServerAddr
	}
	readHeaderTimeout, err := durationFromEnv(envServerReadHeaderTimeout, defaultReadHeaderTimeout)
	if err != nil {
		return Config{}, err
	}
	readTimeout, err := durationFromEnv(envServerReadTimeout, defaultReadTimeout)
	if err != nil {
		return Config{}, err
	}
	writeTimeout, err := durationFromEnv(envServerWriteTimeout, defaultWriteTimeout)
	if err != nil {
		return Config{}, err
	}
	idleTimeout, err := durationFromEnv(envServerIdleTimeout, defaultIdleTimeout)
	if err != nil {
		return Config{}, err
	}
	shutdownTimeout, err := durationFromEnv(envServerShutdownTimeout, defaultShutdownTimeout)
	if err != nil {
		return Config{}, err
	}
	maxConcurrentRuns, err := int64FromEnv(envServerMaxConcurrentRuns, defaultMaxConcurrentRuns)
	if err != nil {
		return Config{}, err
	}
	return Config{
		Addr:              addr,
		Backend:           os.Getenv("WORKORCH_REPOSITORY_BACKEND"),
		DataRoot:          os.Getenv("WORKORCH_DATA_DIR"),
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
		ShutdownTimeout:   shutdownTimeout,
		MaxConcurrentRuns: maxConcurrentRuns,
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
	if config.MaxConcurrentRuns <= 0 {
		config.MaxConcurrentRuns = defaultMaxConcurrentRuns
	}
	return config
}

func NewHTTPServer(config Config) *HTTPServer {
	return NewHTTPServerWithEventHub(config, nil)
}

func NewHTTPServerWithEventHub(config Config, eventHub events.Hub) *HTTPServer {
	config = normalizeConfig(config)
	state := newServerState(config, eventHub)
	handler := newMuxWithState(state)
	return &HTTPServer{
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
	ctx = appservice.ContextWithEventHub(ctx, s.state.eventHub)
	return s.state.validate(ctx, s.config.Backend, s.config.DataRoot)
}

func (s *HTTPServer) Shutdown(ctx context.Context) error {
	if s == nil || s.http == nil {
		return nil
	}
	return errors.Join(s.http.Shutdown(ctx), s.state.close(ctx))
}

func newMux(config Config) http.Handler {
	return newMuxWithState(newServerState(normalizeConfig(config), nil))
}

func newMuxWithState(state *serverState) http.Handler {
	router := engine.NewRouteRegistry()
	registerSystemRoutes(router, state)
	registerReadRoutes(router, state)
	registerBusinessWriteRoutes(router, state)
	registerSliceRoutes(router, state)
	registerConsoleRoutes(router)
	engineServer := engine.NewHTTPServer()
	engineServer.Bind(router)
	handler, ok := engineServer.(http.Handler)
	if !ok {
		return http.NotFoundHandler()
	}
	return handler
}

func registerSystemRoutes(router engine.RouteRegistry, state *serverState) {
	router.AddHandler("/healthz", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		state.metrics.healthzRequests.Add(1)
		writeJSON(w, http.StatusOK, statusResponse{
			Service: "workorch",
			Status:  "ok",
		})
	})
	router.AddHandler("/readyz", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		state.metrics.readyzRequests.Add(1)
		if err := state.validate(r.Context(), state.config.Backend, state.config.DataRoot); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, statusResponse{
			Service: "workorch",
			Status:  "ready",
		})
	})
	router.AddHandler("/metrics", http.MethodGet, func(_ context.Context, w http.ResponseWriter, _ *http.Request) {
		state.metrics.metricsRequests.Add(1)
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		_, _ = fmt.Fprint(w, state.metrics.render(state.config))
	})
}

func registerReadRoutes(router engine.RouteRegistry, state *serverState) {
	router.AddHandler("/api/v1/workorch/workspaces", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		state.metrics.workspaceListRequests.Add(1)
		items, err := state.listWorkspaces(r.Context(), state.config.Backend, state.config.DataRoot)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, workspacesResponse{
			Service:    "workorch",
			Backend:    state.config.Backend,
			DataRoot:   state.config.DataRoot,
			Workspaces: items,
		})
	})
	router.AddHandler("/api/v1/workorch/runs", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		state.metrics.runListRequests.Add(1)
		items, err := state.listRuns(r.Context(), state.config.Backend, state.config.DataRoot, r.URL.Query().Get("workspaceId"))
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, runsResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Runs:     items,
		})
	})
	router.AddHandler("/api/v1/workorch/workspaces/:workspaceId", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		workspaceID, action, ok := workspaceActionFromPath(r.URL.Path)
		if !ok || action != "" {
			http.NotFound(w, r)
			return
		}
		state.metrics.workspaceGetRequests.Add(1)
		item, err := state.readWorkspace(r.Context(), state.config.Backend, state.config.DataRoot, workspaceID)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, workspaceResponse{
			Service:   "workorch",
			Backend:   state.config.Backend,
			DataRoot:  state.config.DataRoot,
			Workspace: item,
		})
	})
	router.AddHandler("/api/v1/workorch/workspaces/:workspaceId/snapshot", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		workspaceID, action, ok := workspaceActionFromPath(r.URL.Path)
		if !ok || action != "snapshot" {
			http.NotFound(w, r)
			return
		}
		state.metrics.workspaceSnapshotRequests.Add(1)
		item, err := state.readWorkspaceSnapshot(r.Context(), state.config.Backend, state.config.DataRoot, workspaceID)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, workspaceSnapshotResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Snapshot: item,
		})
	})
	router.AddHandler("/api/v1/workorch/runs/:runId", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		runID, action, ok := runActionFromPath(r.URL.Path)
		if !ok || action != "" {
			http.NotFound(w, r)
			return
		}
		state.metrics.runGetRequests.Add(1)
		item, err := state.readRun(r.Context(), state.config.Backend, state.config.DataRoot, runID)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, runInstanceResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Run:      item,
		})
	})
	router.AddHandler("/api/v1/workorch/runs/:runId/snapshot", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		runID, action, ok := runActionFromPath(r.URL.Path)
		if !ok || action != "snapshot" {
			http.NotFound(w, r)
			return
		}
		state.metrics.runSnapshotRequests.Add(1)
		item, err := state.readRunSnapshot(r.Context(), state.config.Backend, state.config.DataRoot, runID)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, runSnapshotResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Snapshot: item,
		})
	})
	router.AddHandler("/api/v1/workorch/runs/:runId/status", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		runID, action, ok := runActionFromPath(r.URL.Path)
		if !ok || action != "status" {
			http.NotFound(w, r)
			return
		}
		state.metrics.runStatusRequests.Add(1)
		item, err := state.readRunStatus(r.Context(), state.config.Backend, state.config.DataRoot, runID)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, runStatusResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Status:   item,
		})
	})
	router.AddHandler("/api/v1/workorch/decisions", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		state.metrics.decisionListRequests.Add(1)
		items, err := state.listDecisions(r.Context(), state.config.Backend, state.config.DataRoot, r.URL.Query().Get("workspaceId"), r.URL.Query().Get("runId"))
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, decisionsResponse{
			Service:   "workorch",
			Backend:   state.config.Backend,
			DataRoot:  state.config.DataRoot,
			Decisions: items,
		})
	})
	router.AddHandler("/api/v1/workorch/decisions/:ticketId", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		ticketID, action, ok := decisionActionFromPath(r.URL.Path)
		if !ok || action != "" {
			http.NotFound(w, r)
			return
		}
		state.metrics.decisionGetRequests.Add(1)
		item, err := state.readDecision(r.Context(), state.config.Backend, state.config.DataRoot, ticketID)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, decisionResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Decision: item,
		})
	})
	router.AddHandler("/api/v1/workorch/approvals", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		state.metrics.approvalListRequests.Add(1)
		items, err := state.listApprovals(r.Context(), state.config.Backend, state.config.DataRoot, r.URL.Query().Get("workspaceId"), r.URL.Query().Get("runId"))
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, approvalsResponse{
			Service:   "workorch",
			Backend:   state.config.Backend,
			DataRoot:  state.config.DataRoot,
			Approvals: items,
		})
	})
	router.AddHandler("/api/v1/workorch/approvals/:approvalId", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		approvalID, action, ok := approvalActionFromPath(r.URL.Path)
		if !ok || action != "" {
			http.NotFound(w, r)
			return
		}
		state.metrics.approvalGetRequests.Add(1)
		item, err := state.readApproval(r.Context(), state.config.Backend, state.config.DataRoot, approvalID)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, approvalResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Approval: item,
		})
	})
}

func registerBusinessWriteRoutes(router engine.RouteRegistry, state *serverState) {
	router.AddHandler("/api/v1/workorch/workspaces", http.MethodPost, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		state.metrics.workspaceCreateRequests.Add(1)
		var req createWorkspaceRequest
		if err := decodeJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		item, err := state.createWorkspace(r.Context(), state.config.Backend, state.config.DataRoot, req.WorkspaceID, req.Title)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, workspaceResponse{
			Service:   "workorch",
			Backend:   state.config.Backend,
			DataRoot:  state.config.DataRoot,
			Workspace: item,
		})
	})
	router.AddHandler("/api/v1/workorch/workspaces/:workspaceId/runs", http.MethodPost, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		workspaceID, action, ok := workspaceActionFromPath(r.URL.Path)
		if !ok || action != "runs" {
			http.NotFound(w, r)
			return
		}
		state.metrics.runCreateRequests.Add(1)
		var req createRunRequest
		if err := decodeJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		item, err := state.createRun(r.Context(), state.config.Backend, state.config.DataRoot, workspaceID, req.RunID, req.GoalID, req.DoDID)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, runInstanceResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Run:      item,
		})
	})
	router.AddHandler("/api/v1/workorch/workspaces/:workspaceId/runs", http.MethodGet, func(_ context.Context, w http.ResponseWriter, _ *http.Request) {
		writeMethodNotAllowed(w)
	})
	router.AddHandler("/api/v1/workorch/runs/:runId/start", http.MethodPost, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		runID, action, ok := runActionFromPath(r.URL.Path)
		if !ok || action != "start" {
			http.NotFound(w, r)
			return
		}
		state.metrics.runStartRequests.Add(1)
		release, err := state.acquireRunSlot(r.Context())
		if err != nil {
			state.metrics.runCanceled.Add(1)
			writeRunQueueError(w, err)
			return
		}
		defer release()
		item, err := state.startRun(r.Context(), state.config.Backend, state.config.DataRoot, runID)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		state.metrics.runSucceeded.Add(1)
		writeJSON(w, http.StatusOK, runInstanceResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Run:      item,
		})
	})
	router.AddHandler("/api/v1/workorch/runs/:runId/resume", http.MethodPost, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		runID, action, ok := runActionFromPath(r.URL.Path)
		if !ok || action != "resume" {
			http.NotFound(w, r)
			return
		}
		state.metrics.runResumeRequests.Add(1)
		release, err := state.acquireRunSlot(r.Context())
		if err != nil {
			state.metrics.runCanceled.Add(1)
			writeRunQueueError(w, err)
			return
		}
		defer release()
		item, err := state.resumeRun(r.Context(), state.config.Backend, state.config.DataRoot, runID)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		state.metrics.runSucceeded.Add(1)
		writeJSON(w, http.StatusOK, runInstanceResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Run:      item,
		})
	})
	router.AddHandler("/api/v1/workorch/decisions/:ticketId/resolve", http.MethodPost, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		ticketID, action, ok := decisionActionFromPath(r.URL.Path)
		if !ok || action != "resolve" {
			http.NotFound(w, r)
			return
		}
		state.metrics.decisionResolveRequests.Add(1)
		item, err := state.resolveDecision(r.Context(), state.config.Backend, state.config.DataRoot, ticketID)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, decisionResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Decision: item,
		})
	})
	router.AddHandler("/api/v1/workorch/decisions/:ticketId/reject", http.MethodPost, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		ticketID, action, ok := decisionActionFromPath(r.URL.Path)
		if !ok || action != "reject" {
			http.NotFound(w, r)
			return
		}
		state.metrics.decisionRejectRequests.Add(1)
		item, err := state.rejectDecision(r.Context(), state.config.Backend, state.config.DataRoot, ticketID)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, decisionResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Decision: item,
		})
	})
	router.AddHandler("/api/v1/workorch/approvals/:approvalId/approve", http.MethodPost, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		approvalID, action, ok := approvalActionFromPath(r.URL.Path)
		if !ok || action != "approve" {
			http.NotFound(w, r)
			return
		}
		state.metrics.approvalApproveRequests.Add(1)
		item, err := state.approveApproval(r.Context(), state.config.Backend, state.config.DataRoot, approvalID)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, approvalResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Approval: item,
		})
	})
	router.AddHandler("/api/v1/workorch/approvals/:approvalId/reject", http.MethodPost, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		approvalID, action, ok := approvalActionFromPath(r.URL.Path)
		if !ok || action != "reject" {
			http.NotFound(w, r)
			return
		}
		state.metrics.approvalRejectRequests.Add(1)
		item, err := state.rejectApproval(r.Context(), state.config.Backend, state.config.DataRoot, approvalID)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, approvalResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Approval: item,
		})
	})
	router.AddHandler("/api/v1/workorch/runs", http.MethodPost, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		state.metrics.runRequests.Add(1)
		release, err := state.acquireRunSlot(r.Context())
		if err != nil {
			state.metrics.runCanceled.Add(1)
			writeRunQueueError(w, err)
			return
		}
		defer release()

		slices, err := state.run(r.Context(), state.config.Backend, state.config.DataRoot)
		if err != nil {
			state.metrics.runFailed.Add(1)
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
			return
		}
		state.metrics.runSucceeded.Add(1)
		writeJSON(w, http.StatusOK, runResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Slices:   slices,
		})
	})
}

func registerSliceRoutes(router engine.RouteRegistry, state *serverState) {
	router.AddHandler("/api/v1/workorch/slices", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		state.metrics.sliceCatalogRequests.Add(1)
		writeJSON(w, http.StatusOK, sliceCatalogResponse{
			Service: "workorch",
			Slices:  state.listSlices(),
		})
	})
	router.AddHandler("/api/v1/workorch/slices/snapshots", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		state.metrics.sliceSnapshotsRequests.Add(1)
		slices, err := state.listSnapshots(r.Context(), state.config.Backend, state.config.DataRoot)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, sliceSnapshotsResponse{
			Service: "workorch",
			Slices:  slices,
		})
	})
	router.AddHandler("/api/v1/workorch/slices/statuses", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		state.metrics.sliceStatusesRequests.Add(1)
		statuses, err := state.listStatuses(r.Context(), state.config.Backend, state.config.DataRoot)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, sliceStatusesResponse{
			Service:  "workorch",
			Statuses: statuses,
		})
	})
	router.AddHandler("/api/v1/workorch/slices/:sliceId/status", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		sliceID, action, ok := sliceActionFromPath(r.URL.Path)
		if !ok || action != "status" {
			http.NotFound(w, r)
			return
		}
		state.metrics.sliceStatusRequests.Add(1)
		status, err := state.statusOne(r.Context(), state.config.Backend, state.config.DataRoot, sliceID)
		if err != nil {
			if errors.Is(err, common.ErrNotFound) {
				writeJSON(w, http.StatusNotFound, errorResponse{Error: err.Error()})
				return
			}
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, sliceStatusResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Status:   status,
		})
	})
	router.AddHandler("/api/v1/workorch/slices/:sliceId/snapshot", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		sliceID, action, ok := sliceActionFromPath(r.URL.Path)
		if !ok || action != "snapshot" {
			http.NotFound(w, r)
			return
		}
		state.metrics.singleSliceSnapshotRequests.Add(1)
		slice, err := state.readOne(r.Context(), state.config.Backend, state.config.DataRoot, sliceID)
		if err != nil {
			if errors.Is(err, common.ErrNotFound) {
				writeJSON(w, http.StatusNotFound, errorResponse{Error: err.Error()})
				return
			}
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, sliceRunResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Slice:    slice,
		})
	})
	router.AddHandler("/api/v1/workorch/slices/:sliceId/runs", http.MethodPost, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		sliceID, action, ok := sliceActionFromPath(r.URL.Path)
		if !ok || action != "runs" {
			http.NotFound(w, r)
			return
		}
		state.metrics.singleRunRequests.Add(1)
		release, err := state.acquireRunSlot(r.Context())
		if err != nil {
			state.metrics.runCanceled.Add(1)
			writeRunQueueError(w, err)
			return
		}
		defer release()

		slice, err := state.runOne(r.Context(), state.config.Backend, state.config.DataRoot, sliceID)
		if err != nil {
			if errors.Is(err, common.ErrNotFound) {
				writeJSON(w, http.StatusNotFound, errorResponse{Error: err.Error()})
				return
			}
			state.metrics.runFailed.Add(1)
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
			return
		}
		state.metrics.runSucceeded.Add(1)
		writeJSON(w, http.StatusOK, sliceRunResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Slice:    slice,
		})
	})
}

func registerConsoleRoutes(router engine.RouteRegistry) {
	handler := func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		serveConsoleAsset(w, r)
	}
	router.AddHandler("/console", http.MethodGet, handler)
	router.AddHandler("/console/", http.MethodGet, handler)
	router.AddHandler("/console/**", http.MethodGet, handler)
}

func serveConsoleAsset(w http.ResponseWriter, r *http.Request) {
	staticFS, err := fs.Sub(consoleassets.Assets, "dist")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
		return
	}

	assetPath := strings.TrimPrefix(r.URL.Path, "/console")
	switch {
	case assetPath == "" || assetPath == "/":
		serveConsoleFile(w, r, staticFS, "index.html")
	case strings.HasPrefix(assetPath, "/assets/"):
		serveConsoleFile(w, r, staticFS, strings.TrimPrefix(assetPath, "/"))
	default:
		serveConsoleFile(w, r, staticFS, "index.html")
	}
}

func serveConsoleFile(w http.ResponseWriter, r *http.Request, staticFS fs.FS, name string) {
	content, err := fs.ReadFile(staticFS, path.Clean(name))
	if errors.Is(err, fs.ErrNotExist) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
		return
	}

	if contentType := mime.TypeByExtension(path.Ext(name)); contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(content))
}

func newServerState(config Config, eventHub events.Hub) *serverState {
	state := &serverState{
		config:                config,
		eventHub:              eventHub,
		metrics:               &serverMetrics{},
		validate:              defaultValidate,
		createWorkspace:       defaultCreateWorkspace,
		readWorkspace:         defaultReadWorkspace,
		readWorkspaceSnapshot: defaultReadWorkspaceSnapshot,
		listWorkspaces:        defaultListWorkspaces,
		createRun:             defaultCreateRun,
		readRun:               defaultReadRun,
		readRunSnapshot:       defaultReadRunSnapshot,
		readRunStatus:         defaultReadRunStatus,
		listRuns:              defaultListRuns,
		startRun:              defaultStartRun,
		resumeRun:             defaultResumeRun,
		listDecisions:         defaultListDecisions,
		readDecision:          defaultReadDecision,
		resolveDecision:       defaultResolveDecision,
		rejectDecision:        defaultRejectDecision,
		listApprovals:         defaultListApprovals,
		readApproval:          defaultReadApproval,
		approveApproval:       defaultApproveApproval,
		rejectApproval:        defaultRejectApproval,
		listSlices:            defaultListSlices,
		listSnapshots:         defaultListSnapshots,
		listStatuses:          defaultListStatuses,
		run:                   defaultRun,
		runOne:                defaultRunOne,
		readOne:               defaultReadOne,
		statusOne:             defaultStatusOne,
	}
	state.configureSharedMemorySession()
	return state
}

func (s *serverState) configureSharedMemorySession() {
	if s == nil || !isMemoryBackend(s.config.Backend) {
		return
	}
	session, err := appservice.NewRuntimeSessionWithBackendAndEventHub(s.config.Backend, s.config.DataRoot, s.eventHub)
	if err != nil {
		return
	}
	s.sharedSession = session
	s.validate = func(ctx context.Context, _, _ string) error {
		return s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			_, err := session.Exports()
			return err
		})
	}
	s.createWorkspace = func(ctx context.Context, _, _, workspaceID, title string) (appservice.Workspace, error) {
		var ret appservice.Workspace
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.CreateWorkspaceWithSession(ctx, session, workspaceID, title)
			return err
		})
		return ret, err
	}
	s.readWorkspace = func(ctx context.Context, _, _, workspaceID string) (appservice.Workspace, error) {
		var ret appservice.Workspace
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ReadWorkspaceWithSession(ctx, session, workspaceID)
			return err
		})
		return ret, err
	}
	s.readWorkspaceSnapshot = func(ctx context.Context, _, _, workspaceID string) (appservice.WorkspaceSnapshot, error) {
		var ret appservice.WorkspaceSnapshot
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ReadWorkspaceSnapshotWithSession(ctx, session, workspaceID)
			return err
		})
		return ret, err
	}
	s.listWorkspaces = func(ctx context.Context, _, _ string) ([]appservice.Workspace, error) {
		var ret []appservice.Workspace
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ListWorkspacesWithSession(ctx, session)
			return err
		})
		return ret, err
	}
	s.createRun = func(ctx context.Context, _, _, workspaceID, runID, goalID, dodID string) (appservice.Run, error) {
		var ret appservice.Run
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.CreateRunWithSession(ctx, session, workspaceID, runID, goalID, dodID)
			return err
		})
		return ret, err
	}
	s.readRun = func(ctx context.Context, _, _, runID string) (appservice.Run, error) {
		var ret appservice.Run
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ReadRunWithSession(ctx, session, runID)
			return err
		})
		return ret, err
	}
	s.readRunSnapshot = func(ctx context.Context, _, _, runID string) (appservice.RunSnapshot, error) {
		var ret appservice.RunSnapshot
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ReadRunSnapshotWithSession(ctx, session, runID)
			return err
		})
		return ret, err
	}
	s.readRunStatus = func(ctx context.Context, _, _, runID string) (appservice.RunStatus, error) {
		var ret appservice.RunStatus
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ReadRunStatusWithSession(ctx, session, runID)
			return err
		})
		return ret, err
	}
	s.listRuns = func(ctx context.Context, _, _, workspaceID string) ([]appservice.Run, error) {
		var ret []appservice.Run
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ListRunsWithSession(ctx, session, workspaceID)
			return err
		})
		return ret, err
	}
	s.startRun = func(ctx context.Context, _, _, runID string) (appservice.Run, error) {
		var ret appservice.Run
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.StartRunWithSession(ctx, session, runID)
			return err
		})
		return ret, err
	}
	s.resumeRun = func(ctx context.Context, _, _, runID string) (appservice.Run, error) {
		var ret appservice.Run
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ResumeRunWithSession(ctx, session, runID)
			return err
		})
		return ret, err
	}
	s.listDecisions = func(ctx context.Context, _, _, workspaceID, runID string) ([]appservice.Decision, error) {
		var ret []appservice.Decision
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ListDecisionsWithSession(ctx, session, workspaceID, runID)
			return err
		})
		return ret, err
	}
	s.readDecision = func(ctx context.Context, _, _, ticketID string) (appservice.Decision, error) {
		var ret appservice.Decision
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ReadDecisionWithSession(ctx, session, ticketID)
			return err
		})
		return ret, err
	}
	s.resolveDecision = func(ctx context.Context, _, _, ticketID string) (appservice.Decision, error) {
		var ret appservice.Decision
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ResolveDecisionWithSession(ctx, session, ticketID)
			return err
		})
		return ret, err
	}
	s.rejectDecision = func(ctx context.Context, _, _, ticketID string) (appservice.Decision, error) {
		var ret appservice.Decision
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.RejectDecisionWithSession(ctx, session, ticketID)
			return err
		})
		return ret, err
	}
	s.listApprovals = func(ctx context.Context, _, _, workspaceID, runID string) ([]appservice.Approval, error) {
		var ret []appservice.Approval
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ListApprovalsWithSession(ctx, session, workspaceID, runID)
			return err
		})
		return ret, err
	}
	s.readApproval = func(ctx context.Context, _, _, approvalID string) (appservice.Approval, error) {
		var ret appservice.Approval
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ReadApprovalWithSession(ctx, session, approvalID)
			return err
		})
		return ret, err
	}
	s.approveApproval = func(ctx context.Context, _, _, approvalID string) (appservice.Approval, error) {
		var ret appservice.Approval
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ApproveApprovalWithSession(ctx, session, approvalID)
			return err
		})
		return ret, err
	}
	s.rejectApproval = func(ctx context.Context, _, _, approvalID string) (appservice.Approval, error) {
		var ret appservice.Approval
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.RejectApprovalWithSession(ctx, session, approvalID)
			return err
		})
		return ret, err
	}
	s.run = func(ctx context.Context, _, _ string) ([]appservice.Slice, error) {
		var ret []appservice.Slice
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.RunAllWorkorchWithSession(ctx, session)
			return err
		})
		return ret, err
	}
	s.runOne = func(ctx context.Context, _, _, sliceID string) (appservice.Slice, error) {
		var ret appservice.Slice
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.RunWorkorchSliceWithSession(ctx, session, sliceID)
			return err
		})
		return ret, err
	}
	s.readOne = func(ctx context.Context, _, _, sliceID string) (appservice.Slice, error) {
		var ret appservice.Slice
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ReadWorkorchSliceSnapshotWithSession(ctx, session, sliceID)
			return err
		})
		return ret, err
	}
	s.statusOne = func(ctx context.Context, _, _, sliceID string) (appservice.SliceStatus, error) {
		var ret appservice.SliceStatus
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.SummarizeWorkorchSliceStatusWithSession(ctx, session, sliceID)
			return err
		})
		return ret, err
	}
	s.listSnapshots = func(ctx context.Context, _, _ string) ([]appservice.Slice, error) {
		var ret []appservice.Slice
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ListAvailableWorkorchSliceSnapshotsWithSession(ctx, session)
			return err
		})
		return ret, err
	}
	s.listStatuses = func(ctx context.Context, _, _ string) ([]appservice.SliceStatus, error) {
		var ret []appservice.SliceStatus
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ListAvailableWorkorchSliceStatusesWithSession(ctx, session)
			return err
		})
		return ret, err
	}
}

func isMemoryBackend(backend string) bool {
	return backend == "" || backend == "memory"
}

func (s *serverState) withSharedSession(ctx context.Context, fn func(*appservice.RuntimeSession) error) error {
	if s == nil || s.sharedSession == nil {
		return common.ErrInvalidArgument
	}
	session := s.sharedSession.Borrow()
	err := fn(session)
	return errors.Join(err, session.Close(ctx))
}

func (s *serverState) close(ctx context.Context) error {
	if s == nil || s.sharedSession == nil {
		return nil
	}
	err := s.sharedSession.Close(ctx)
	s.sharedSession = nil
	return err
}

func defaultValidate(ctx context.Context, backend string, dataRoot string) error {
	session, err := appservice.NewRuntimeSessionWithBackend(backend, dataRoot)
	if err != nil {
		return err
	}
	return session.Close(ctx)
}

func defaultRun(ctx context.Context, backend string, dataRoot string) ([]appservice.Slice, error) {
	return appservice.RunAllWorkorchWithBackend(ctx, backend, dataRoot)
}

func defaultCreateWorkspace(ctx context.Context, backend string, dataRoot string, workspaceID string, title string) (appservice.Workspace, error) {
	return appservice.CreateWorkspaceWithBackend(ctx, backend, dataRoot, workspaceID, title)
}

func defaultCreateRun(ctx context.Context, backend string, dataRoot string, workspaceID string, runID string, goalID string, dodID string) (appservice.Run, error) {
	return appservice.CreateRunWithBackend(ctx, backend, dataRoot, workspaceID, runID, goalID, dodID)
}

func defaultReadWorkspace(ctx context.Context, backend string, dataRoot string, workspaceID string) (appservice.Workspace, error) {
	return appservice.ReadWorkspaceWithBackend(ctx, backend, dataRoot, workspaceID)
}

func defaultReadWorkspaceSnapshot(ctx context.Context, backend string, dataRoot string, workspaceID string) (appservice.WorkspaceSnapshot, error) {
	return appservice.ReadWorkspaceSnapshotWithBackend(ctx, backend, dataRoot, workspaceID)
}

func defaultReadRun(ctx context.Context, backend string, dataRoot string, runID string) (appservice.Run, error) {
	return appservice.ReadRunWithBackend(ctx, backend, dataRoot, runID)
}

func defaultReadRunSnapshot(ctx context.Context, backend string, dataRoot string, runID string) (appservice.RunSnapshot, error) {
	return appservice.ReadRunSnapshotWithBackend(ctx, backend, dataRoot, runID)
}

func defaultReadRunStatus(ctx context.Context, backend string, dataRoot string, runID string) (appservice.RunStatus, error) {
	return appservice.ReadRunStatusWithBackend(ctx, backend, dataRoot, runID)
}

func defaultListWorkspaces(ctx context.Context, backend string, dataRoot string) ([]appservice.Workspace, error) {
	return appservice.ListWorkspacesWithBackend(ctx, backend, dataRoot)
}

func defaultListRuns(ctx context.Context, backend string, dataRoot string, workspaceID string) ([]appservice.Run, error) {
	return appservice.ListRunsWithBackend(ctx, backend, dataRoot, workspaceID)
}

func defaultStartRun(ctx context.Context, backend string, dataRoot string, runID string) (appservice.Run, error) {
	return appservice.StartRunWithBackend(ctx, backend, dataRoot, runID)
}

func defaultResumeRun(ctx context.Context, backend string, dataRoot string, runID string) (appservice.Run, error) {
	return appservice.ResumeRunWithBackend(ctx, backend, dataRoot, runID)
}

func defaultListDecisions(ctx context.Context, backend string, dataRoot string, workspaceID string, runID string) ([]appservice.Decision, error) {
	return appservice.ListDecisionsWithBackend(ctx, backend, dataRoot, workspaceID, runID)
}

func defaultReadDecision(ctx context.Context, backend string, dataRoot string, ticketID string) (appservice.Decision, error) {
	return appservice.ReadDecisionWithBackend(ctx, backend, dataRoot, ticketID)
}

func defaultResolveDecision(ctx context.Context, backend string, dataRoot string, ticketID string) (appservice.Decision, error) {
	return appservice.ResolveDecisionWithBackend(ctx, backend, dataRoot, ticketID)
}

func defaultRejectDecision(ctx context.Context, backend string, dataRoot string, ticketID string) (appservice.Decision, error) {
	return appservice.RejectDecisionWithBackend(ctx, backend, dataRoot, ticketID)
}

func defaultListApprovals(ctx context.Context, backend string, dataRoot string, workspaceID string, runID string) ([]appservice.Approval, error) {
	return appservice.ListApprovalsWithBackend(ctx, backend, dataRoot, workspaceID, runID)
}

func defaultReadApproval(ctx context.Context, backend string, dataRoot string, approvalID string) (appservice.Approval, error) {
	return appservice.ReadApprovalWithBackend(ctx, backend, dataRoot, approvalID)
}

func defaultApproveApproval(ctx context.Context, backend string, dataRoot string, approvalID string) (appservice.Approval, error) {
	return appservice.ApproveApprovalWithBackend(ctx, backend, dataRoot, approvalID)
}

func defaultRejectApproval(ctx context.Context, backend string, dataRoot string, approvalID string) (appservice.Approval, error) {
	return appservice.RejectApprovalWithBackend(ctx, backend, dataRoot, approvalID)
}

func defaultRunOne(ctx context.Context, backend string, dataRoot string, sliceID string) (appservice.Slice, error) {
	return appservice.RunWorkorchSliceWithBackend(ctx, backend, dataRoot, sliceID)
}

func defaultReadOne(ctx context.Context, backend string, dataRoot string, sliceID string) (appservice.Slice, error) {
	return appservice.ReadWorkorchSliceSnapshotWithBackend(ctx, backend, dataRoot, sliceID)
}

func defaultStatusOne(ctx context.Context, backend string, dataRoot string, sliceID string) (appservice.SliceStatus, error) {
	return appservice.SummarizeWorkorchSliceStatusWithBackend(ctx, backend, dataRoot, sliceID)
}

func defaultListSlices() []appservice.SliceDescriptor {
	return appservice.ListWorkorchSlices()
}

func defaultListSnapshots(ctx context.Context, backend string, dataRoot string) ([]appservice.Slice, error) {
	return appservice.ListAvailableWorkorchSliceSnapshotsWithBackend(ctx, backend, dataRoot)
}

func defaultListStatuses(ctx context.Context, backend string, dataRoot string) ([]appservice.SliceStatus, error) {
	return appservice.ListAvailableWorkorchSliceStatusesWithBackend(ctx, backend, dataRoot)
}

func (s *serverState) acquireRunSlot(ctx context.Context) (func(), error) {
	s.runQueueMu.Lock()
	if s.activeRunSlots < s.config.MaxConcurrentRuns && len(s.runQueueWaiters) == 0 {
		s.activeRunSlots++
		s.metrics.runInFlight.Add(1)
		s.runQueueMu.Unlock()
		return s.releaseRunSlot, nil
	}

	waiter := make(chan struct{})
	s.runQueueWaiters = append(s.runQueueWaiters, waiter)
	s.metrics.runQueueWaiting.Add(1)
	s.metrics.runQueuedTotal.Add(1)
	s.runQueueMu.Unlock()

	select {
	case <-waiter:
		if ctx.Err() != nil {
			s.releaseRunSlot()
			return nil, ctx.Err()
		}
		return s.releaseRunSlot, nil
	case <-ctx.Done():
		if s.cancelQueuedRunWaiter(waiter) {
			return nil, ctx.Err()
		}
		s.releaseRunSlot()
		return nil, ctx.Err()
	}
}

func (s *serverState) releaseRunSlot() {
	s.runQueueMu.Lock()
	if len(s.runQueueWaiters) > 0 {
		waiter := s.runQueueWaiters[0]
		s.runQueueWaiters = s.runQueueWaiters[1:]
		s.metrics.runQueueWaiting.Add(-1)
		s.runQueueMu.Unlock()
		close(waiter)
		return
	}
	if s.activeRunSlots > 0 {
		s.activeRunSlots--
		s.metrics.runInFlight.Add(-1)
	}
	s.runQueueMu.Unlock()
}

func (s *serverState) cancelQueuedRunWaiter(target chan struct{}) bool {
	s.runQueueMu.Lock()
	defer s.runQueueMu.Unlock()

	for idx, waiter := range s.runQueueWaiters {
		if waiter != target {
			continue
		}
		s.runQueueWaiters = append(s.runQueueWaiters[:idx], s.runQueueWaiters[idx+1:]...)
		s.metrics.runQueueWaiting.Add(-1)
		return true
	}
	return false
}

func (m *serverMetrics) render(config Config) string {
	return fmt.Sprintf(
		"# TYPE workorch_http_requests_total counter\n"+
			"workorch_http_requests_total{endpoint=\"healthz\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"readyz\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"metrics\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"workspace_create\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"workspace_list\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"workspace_get\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"workspace_snapshot\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"run_create\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"run_list\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"run_get\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"run_snapshot\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"run_status\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"run_start\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"run_resume\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"decision_list\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"decision_get\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"decision_resolve\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"decision_reject\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"approval_list\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"approval_get\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"approval_approve\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"approval_reject\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"slices\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"slice_snapshots\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"slice_statuses\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"slice_snapshot\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"slice_status\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"runs\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"slice_runs\"} %d\n"+
			"# TYPE workorch_run_requests_in_flight gauge\n"+
			"workorch_run_requests_in_flight %d\n"+
			"# TYPE workorch_run_queue_waiting gauge\n"+
			"workorch_run_queue_waiting %d\n"+
			"# TYPE workorch_run_queue_total counter\n"+
			"workorch_run_queue_total %d\n"+
			"# TYPE workorch_run_outcomes_total counter\n"+
			"workorch_run_outcomes_total{status=\"succeeded\"} %d\n"+
			"workorch_run_outcomes_total{status=\"failed\"} %d\n"+
			"workorch_run_outcomes_total{status=\"rejected\"} %d\n"+
			"workorch_run_outcomes_total{status=\"canceled\"} %d\n"+
			"# TYPE workorch_server_max_concurrent_runs gauge\n"+
			"workorch_server_max_concurrent_runs %d\n",
		m.healthzRequests.Load(),
		m.readyzRequests.Load(),
		m.metricsRequests.Load(),
		m.workspaceCreateRequests.Load(),
		m.workspaceListRequests.Load(),
		m.workspaceGetRequests.Load(),
		m.workspaceSnapshotRequests.Load(),
		m.runCreateRequests.Load(),
		m.runListRequests.Load(),
		m.runGetRequests.Load(),
		m.runSnapshotRequests.Load(),
		m.runStatusRequests.Load(),
		m.runStartRequests.Load(),
		m.runResumeRequests.Load(),
		m.decisionListRequests.Load(),
		m.decisionGetRequests.Load(),
		m.decisionResolveRequests.Load(),
		m.decisionRejectRequests.Load(),
		m.approvalListRequests.Load(),
		m.approvalGetRequests.Load(),
		m.approvalApproveRequests.Load(),
		m.approvalRejectRequests.Load(),
		m.sliceCatalogRequests.Load(),
		m.sliceSnapshotsRequests.Load(),
		m.sliceStatusesRequests.Load(),
		m.singleSliceSnapshotRequests.Load(),
		m.sliceStatusRequests.Load(),
		m.runRequests.Load(),
		m.singleRunRequests.Load(),
		m.runInFlight.Load(),
		m.runQueueWaiting.Load(),
		m.runQueuedTotal.Load(),
		m.runSucceeded.Load(),
		m.runFailed.Load(),
		m.runRejected.Load(),
		m.runCanceled.Load(),
		config.MaxConcurrentRuns,
	)
}

func sliceActionFromPath(path string) (sliceID string, action string, ok bool) {
	if !strings.HasPrefix(path, "/api/v1/workorch/slices/") {
		return "", "", false
	}
	trimmed := strings.TrimPrefix(path, "/api/v1/workorch/slices/")
	parts := strings.Split(trimmed, "/")
	if len(parts) != 2 {
		return "", "", false
	}
	if parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func workspaceActionFromPath(path string) (workspaceID string, action string, ok bool) {
	if !strings.HasPrefix(path, "/api/v1/workorch/workspaces/") {
		return "", "", false
	}
	trimmed := strings.TrimPrefix(path, "/api/v1/workorch/workspaces/")
	if trimmed == "" {
		return "", "", false
	}
	parts := strings.Split(trimmed, "/")
	if len(parts) == 1 && parts[0] != "" {
		return parts[0], "", true
	}
	if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
		return parts[0], parts[1], true
	}
	return "", "", false
}

func runActionFromPath(path string) (runID string, action string, ok bool) {
	if !strings.HasPrefix(path, "/api/v1/workorch/runs/") {
		return "", "", false
	}
	trimmed := strings.TrimPrefix(path, "/api/v1/workorch/runs/")
	if trimmed == "" {
		return "", "", false
	}
	parts := strings.Split(trimmed, "/")
	if len(parts) == 1 && parts[0] != "" {
		return parts[0], "", true
	}
	if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
		return parts[0], parts[1], true
	}
	return "", "", false
}

func decisionActionFromPath(path string) (ticketID string, action string, ok bool) {
	if !strings.HasPrefix(path, "/api/v1/workorch/decisions/") {
		return "", "", false
	}
	trimmed := strings.TrimPrefix(path, "/api/v1/workorch/decisions/")
	if trimmed == "" {
		return "", "", false
	}
	parts := strings.Split(trimmed, "/")
	if len(parts) == 1 && parts[0] != "" {
		return parts[0], "", true
	}
	if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
		return parts[0], parts[1], true
	}
	return "", "", false
}

func approvalActionFromPath(path string) (approvalID string, action string, ok bool) {
	if !strings.HasPrefix(path, "/api/v1/workorch/approvals/") {
		return "", "", false
	}
	trimmed := strings.TrimPrefix(path, "/api/v1/workorch/approvals/")
	if trimmed == "" {
		return "", "", false
	}
	parts := strings.Split(trimmed, "/")
	if len(parts) == 1 && parts[0] != "" {
		return parts[0], "", true
	}
	if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
		return parts[0], parts[1], true
	}
	return "", "", false
}

func durationFromEnv(name string, fallback time.Duration) (time.Duration, error) {
	value := os.Getenv(name)
	if value == "" {
		return fallback, nil
	}
	duration, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("invalid duration for %s: %w", name, err)
	}
	return duration, nil
}

func int64FromEnv(name string, fallback int64) (int64, error) {
	value := os.Getenv(name)
	if value == "" {
		return fallback, nil
	}
	number, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid integer for %s: %w", name, err)
	}
	if number <= 0 {
		return 0, fmt.Errorf("invalid integer for %s: must be positive", name)
	}
	return number, nil
}

func writeMethodNotAllowed(w http.ResponseWriter) {
	writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "method not allowed"})
}

func writeBusinessError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, common.ErrInvalidArgument):
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
	case errors.Is(err, common.ErrBlocked):
		writeJSON(w, http.StatusConflict, errorResponse{Error: err.Error()})
	case errors.Is(err, common.ErrConflict):
		writeJSON(w, http.StatusConflict, errorResponse{Error: err.Error()})
	case errors.Is(err, common.ErrNotFound):
		writeJSON(w, http.StatusNotFound, errorResponse{Error: err.Error()})
	default:
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
	}
}

func writeRunQueueError(w http.ResponseWriter, err error) {
	writeJSON(w, http.StatusRequestTimeout, errorResponse{
		Error: fmt.Sprintf("run execution request canceled while waiting for available execution slot: %v", err),
	})
}

func decodeJSON(r *http.Request, dst any) error {
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return fmt.Errorf("invalid request body: %w", err)
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(payload)
}
