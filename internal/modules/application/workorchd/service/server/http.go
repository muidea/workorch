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
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

const (
	configServerAddr              = "server.addr"
	configServerReadHeaderTimeout = "server.readHeaderTimeout"
	configServerReadTimeout       = "server.readTimeout"
	configServerWriteTimeout      = "server.writeTimeout"
	configServerIdleTimeout       = "server.idleTimeout"
	configServerShutdownTimeout   = "server.shutdownTimeout"
	configServerMaxConcurrentRuns = "server.maxConcurrentRuns"

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

type configurationResponse struct {
	Service       string                          `json:"service"`
	Configuration appservice.ServiceConfiguration `json:"configuration"`
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

type goalResponse struct {
	Service  string          `json:"service"`
	Backend  string          `json:"backend,omitempty"`
	DataRoot string          `json:"dataRoot,omitempty"`
	Goal     appservice.Goal `json:"goal"`
}

type goalsResponse struct {
	Service  string            `json:"service"`
	Backend  string            `json:"backend,omitempty"`
	DataRoot string            `json:"dataRoot,omitempty"`
	Goals    []appservice.Goal `json:"goals"`
}

type dodResponse struct {
	Service  string         `json:"service"`
	Backend  string         `json:"backend,omitempty"`
	DataRoot string         `json:"dataRoot,omitempty"`
	DoD      appservice.DoD `json:"dod"`
}

type dodsResponse struct {
	Service  string           `json:"service"`
	Backend  string           `json:"backend,omitempty"`
	DataRoot string           `json:"dataRoot,omitempty"`
	DoDs     []appservice.DoD `json:"dods"`
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

type runObservabilityResponse struct {
	Service       string                      `json:"service"`
	Backend       string                      `json:"backend,omitempty"`
	DataRoot      string                      `json:"dataRoot,omitempty"`
	Observability appservice.RunObservability `json:"observability"`
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

type workspaceExecutionActivationResponse struct {
	Service    string                                  `json:"service"`
	Backend    string                                  `json:"backend,omitempty"`
	DataRoot   string                                  `json:"dataRoot,omitempty"`
	Activation appservice.WorkspaceExecutionActivation `json:"activation"`
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

type soulResponse struct {
	Service  string          `json:"service"`
	Backend  string          `json:"backend,omitempty"`
	DataRoot string          `json:"dataRoot,omitempty"`
	Soul     appservice.Soul `json:"soul"`
}

type soulsResponse struct {
	Service  string            `json:"service"`
	Backend  string            `json:"backend,omitempty"`
	DataRoot string            `json:"dataRoot,omitempty"`
	Souls    []appservice.Soul `json:"souls"`
}

type skillResponse struct {
	Service  string           `json:"service"`
	Backend  string           `json:"backend,omitempty"`
	DataRoot string           `json:"dataRoot,omitempty"`
	Skill    appservice.Skill `json:"skill"`
}

type skillsResponse struct {
	Service  string             `json:"service"`
	Backend  string             `json:"backend,omitempty"`
	DataRoot string             `json:"dataRoot,omitempty"`
	Skills   []appservice.Skill `json:"skills"`
}

type mcpServerResponse struct {
	Service  string               `json:"service"`
	Backend  string               `json:"backend,omitempty"`
	DataRoot string               `json:"dataRoot,omitempty"`
	Server   appservice.MCPServer `json:"server"`
}

type mcpServersResponse struct {
	Service  string                 `json:"service"`
	Backend  string                 `json:"backend,omitempty"`
	DataRoot string                 `json:"dataRoot,omitempty"`
	Servers  []appservice.MCPServer `json:"servers"`
}

type mcpToolsResponse struct {
	Service  string               `json:"service"`
	Backend  string               `json:"backend,omitempty"`
	DataRoot string               `json:"dataRoot,omitempty"`
	Tools    []appservice.MCPTool `json:"tools"`
}

type memoryItemResponse struct {
	Service  string                `json:"service"`
	Backend  string                `json:"backend,omitempty"`
	DataRoot string                `json:"dataRoot,omitempty"`
	Memory   appservice.MemoryItem `json:"memory"`
}

type memoryItemsResponse struct {
	Service  string                  `json:"service"`
	Backend  string                  `json:"backend,omitempty"`
	DataRoot string                  `json:"dataRoot,omitempty"`
	Memory   []appservice.MemoryItem `json:"memory"`
}

type createWorkspaceRequest struct {
	WorkspaceID      string `json:"workspaceId"`
	Title            string `json:"title"`
	Description      string `json:"description"`
	WorkingDirectory string `json:"workingDirectory"`
}

type createRunRequest struct {
	RunID  string `json:"runId"`
	GoalID string `json:"goalId,omitempty"`
	DoDID  string `json:"doDId,omitempty"`
}

type activateWorkspaceExecutionRequest struct {
	GoalID        string   `json:"goalId,omitempty"`
	DoDID         string   `json:"doDId,omitempty"`
	LLMProviderID string   `json:"llmProviderId,omitempty"`
	SoulID        string   `json:"soulId,omitempty"`
	SkillIDs      []string `json:"skillIds,omitempty"`
	MCPServerIDs  []string `json:"mcpServerIds,omitempty"`
	MemoryIDs     []string `json:"memoryIds,omitempty"`
}

type createSoulRequest = appservice.CreateSoulCommand
type createSkillRequest = appservice.CreateSkillCommand
type createMCPServerRequest = appservice.CreateMCPServerCommand
type createMemoryItemRequest = appservice.CreateMemoryItemCommand

type resolveDecisionRequest struct {
	ResolutionType    string         `json:"resolutionType"`
	ResolutionPayload map[string]any `json:"resolutionPayload,omitempty"`
}

type defineGoalRequest struct {
	Text string `json:"text"`
}

type defineDoDRequest struct {
	Items []string `json:"items"`
}

type errorResponse struct {
	Error string `json:"error"`
}

type createWorkspaceFunc func(context.Context, string, string, string, string, string) (appservice.Workspace, error)
type readWorkspaceFunc func(context.Context, string, string, string) (appservice.Workspace, error)
type readWorkspaceSnapshotFunc func(context.Context, string, string, string) (appservice.WorkspaceSnapshot, error)
type listWorkspacesFunc func(context.Context, string, string) ([]appservice.Workspace, error)
type defineGoalFunc func(context.Context, string, string, string, string) (appservice.Goal, error)
type readGoalFunc func(context.Context, string, string, string) (appservice.Goal, error)
type listGoalsFunc func(context.Context, string, string, string) ([]appservice.Goal, error)
type defineDoDFunc func(context.Context, string, string, string, []string) (appservice.DoD, error)
type readDoDFunc func(context.Context, string, string, string) (appservice.DoD, error)
type listDoDsFunc func(context.Context, string, string, string) ([]appservice.DoD, error)
type createRunInstanceFunc func(context.Context, string, string, string, string, string, string) (appservice.Run, error)
type readRunInstanceFunc func(context.Context, string, string, string) (appservice.Run, error)
type readRunSnapshotFunc func(context.Context, string, string, string) (appservice.RunSnapshot, error)
type readRunStatusFunc func(context.Context, string, string, string) (appservice.RunStatus, error)
type readRunObservabilityFunc func(context.Context, string, string, string) (appservice.RunObservability, error)
type listRunInstancesFunc func(context.Context, string, string, string) ([]appservice.Run, error)
type startRunInstanceFunc func(context.Context, string, string, string) (appservice.Run, error)
type resumeRunInstanceFunc func(context.Context, string, string, string) (appservice.Run, error)
type activateWorkspaceExecutionFunc func(context.Context, string, string, string, appservice.ActivateWorkspaceExecutionCommand) (appservice.WorkspaceExecutionActivation, error)
type listDecisionsFunc func(context.Context, string, string, string, string) ([]appservice.Decision, error)
type readDecisionFunc func(context.Context, string, string, string) (appservice.Decision, error)
type resolveDecisionFunc func(context.Context, string, string, string, appservice.ResolveDecisionCommand) (appservice.Decision, error)
type listApprovalsFunc func(context.Context, string, string, string, string) ([]appservice.Approval, error)
type readApprovalFunc func(context.Context, string, string, string) (appservice.Approval, error)
type approveApprovalFunc func(context.Context, string, string, string) (appservice.Approval, error)
type rejectApprovalFunc func(context.Context, string, string, string) (appservice.Approval, error)
type createSoulFunc func(context.Context, string, string, appservice.CreateSoulCommand) (appservice.Soul, error)
type readSoulFunc func(context.Context, string, string, string) (appservice.Soul, error)
type listSoulsFunc func(context.Context, string, string) ([]appservice.Soul, error)
type createSkillFunc func(context.Context, string, string, appservice.CreateSkillCommand) (appservice.Skill, error)
type readSkillFunc func(context.Context, string, string, string) (appservice.Skill, error)
type listSkillsFunc func(context.Context, string, string) ([]appservice.Skill, error)
type createMCPServerFunc func(context.Context, string, string, appservice.CreateMCPServerCommand) (appservice.MCPServer, error)
type readMCPServerFunc func(context.Context, string, string, string) (appservice.MCPServer, error)
type listMCPServersFunc func(context.Context, string, string) ([]appservice.MCPServer, error)
type listMCPServerToolsFunc func(context.Context, string, string, string) ([]appservice.MCPTool, error)
type createMemoryItemFunc func(context.Context, string, string, string, appservice.CreateMemoryItemCommand) (appservice.MemoryItem, error)
type listMemoryByWorkspaceFunc func(context.Context, string, string, string, string) ([]appservice.MemoryItem, error)
type validateFunc func(context.Context, string, string) error
type readConfigurationFunc func() (appservice.ServiceConfiguration, error)
type updateLLMConfigurationFunc func(appservice.UpdateLLMConfigurationCommand) (appservice.ServiceConfiguration, error)

type serverState struct {
	config                 Config
	eventHub               events.Hub
	metrics                *serverMetrics
	validate               validateFunc
	createWorkspace        createWorkspaceFunc
	readWorkspace          readWorkspaceFunc
	readWorkspaceSnapshot  readWorkspaceSnapshotFunc
	listWorkspaces         listWorkspacesFunc
	defineGoal             defineGoalFunc
	readGoal               readGoalFunc
	listGoals              listGoalsFunc
	defineDoD              defineDoDFunc
	readDoD                readDoDFunc
	listDoDs               listDoDsFunc
	createRun              createRunInstanceFunc
	readRun                readRunInstanceFunc
	readRunSnapshot        readRunSnapshotFunc
	readRunStatus          readRunStatusFunc
	readRunObservability   readRunObservabilityFunc
	listRuns               listRunInstancesFunc
	startRun               startRunInstanceFunc
	resumeRun              resumeRunInstanceFunc
	activateWorkspace      activateWorkspaceExecutionFunc
	listDecisions          listDecisionsFunc
	readDecision           readDecisionFunc
	resolveDecision        resolveDecisionFunc
	listApprovals          listApprovalsFunc
	readApproval           readApprovalFunc
	approveApproval        approveApprovalFunc
	rejectApproval         rejectApprovalFunc
	createSoul             createSoulFunc
	readSoul               readSoulFunc
	listSouls              listSoulsFunc
	createSkill            createSkillFunc
	readSkill              readSkillFunc
	listSkills             listSkillsFunc
	createMCPServer        createMCPServerFunc
	readMCPServer          readMCPServerFunc
	listMCPServers         listMCPServersFunc
	listMCPServerTools     listMCPServerToolsFunc
	createMemoryItem       createMemoryItemFunc
	listMemoryByWorkspace  listMemoryByWorkspaceFunc
	readConfiguration      readConfigurationFunc
	updateLLMConfiguration updateLLMConfigurationFunc
	runQueueMu             sync.Mutex
	runQueueWaiters        []chan struct{}
	activeRunSlots         int64
	sharedSession          *appservice.RuntimeSession
}

type serverMetrics struct {
	healthzRequests             atomic.Uint64
	readyzRequests              atomic.Uint64
	metricsRequests             atomic.Uint64
	workspaceCreateRequests     atomic.Uint64
	workspaceListRequests       atomic.Uint64
	workspaceGetRequests        atomic.Uint64
	workspaceSnapshotRequests   atomic.Uint64
	goalDefineRequests          atomic.Uint64
	goalListRequests            atomic.Uint64
	goalGetRequests             atomic.Uint64
	dodDefineRequests           atomic.Uint64
	dodListRequests             atomic.Uint64
	dodGetRequests              atomic.Uint64
	runCreateRequests           atomic.Uint64
	runListRequests             atomic.Uint64
	runGetRequests              atomic.Uint64
	runSnapshotRequests         atomic.Uint64
	runStatusRequests           atomic.Uint64
	runObservabilityRequests    atomic.Uint64
	runStartRequests            atomic.Uint64
	runResumeRequests           atomic.Uint64
	workspaceActivationRequests atomic.Uint64
	decisionListRequests        atomic.Uint64
	decisionGetRequests         atomic.Uint64
	decisionResolveRequests     atomic.Uint64
	approvalListRequests        atomic.Uint64
	approvalGetRequests         atomic.Uint64
	approvalApproveRequests     atomic.Uint64
	approvalRejectRequests      atomic.Uint64
	soulCreateRequests          atomic.Uint64
	soulListRequests            atomic.Uint64
	soulGetRequests             atomic.Uint64
	skillCreateRequests         atomic.Uint64
	skillListRequests           atomic.Uint64
	skillGetRequests            atomic.Uint64
	mcpServerCreateRequests     atomic.Uint64
	mcpServerListRequests       atomic.Uint64
	mcpServerGetRequests        atomic.Uint64
	mcpToolListRequests         atomic.Uint64
	memoryCreateRequests        atomic.Uint64
	memoryListRequests          atomic.Uint64
	configurationReadRequests   atomic.Uint64
	configurationUpdateRequests atomic.Uint64
	runSucceeded                atomic.Uint64
	runFailed                   atomic.Uint64
	runRejected                 atomic.Uint64
	runCanceled                 atomic.Uint64
	runInFlight                 atomic.Int64
	runQueueWaiting             atomic.Int64
	runQueuedTotal              atomic.Uint64
}

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
	maxConcurrentRuns := cfg.Server.MaxConcurrentRuns
	if maxConcurrentRuns == 0 {
		maxConcurrentRuns = defaultMaxConcurrentRuns
	}
	if maxConcurrentRuns < 0 {
		return Config{}, fmt.Errorf("invalid integer for %s: must be positive", configServerMaxConcurrentRuns)
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
		MaxConcurrentRuns: maxConcurrentRuns,
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
	maxConcurrentRuns, err := int64FromValues(values, configServerMaxConcurrentRuns, defaultMaxConcurrentRuns)
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
	router.AddHandler("/api/v1/workorch/configuration", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		state.metrics.configurationReadRequests.Add(1)
		item, err := state.readConfiguration()
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, configurationResponse{
			Service:       "workorch",
			Configuration: item,
		})
	})
	router.AddHandler("/api/v1/workorch/configuration/llm", http.MethodPut, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		state.metrics.configurationUpdateRequests.Add(1)
		var req appservice.UpdateLLMConfigurationCommand
		if err := decodeJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		item, err := state.updateLLMConfiguration(req)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, configurationResponse{
			Service:       "workorch",
			Configuration: item,
		})
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
	router.AddHandler("/api/v1/workorch/workspaces/:workspaceId/goals", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		workspaceID, action, ok := workspaceActionFromPath(r.URL.Path)
		if !ok || action != "goals" {
			http.NotFound(w, r)
			return
		}
		state.metrics.goalListRequests.Add(1)
		items, err := state.listGoals(r.Context(), state.config.Backend, state.config.DataRoot, workspaceID)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, goalsResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Goals:    items,
		})
	})
	router.AddHandler("/api/v1/workorch/goals/:goalId", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		goalID, ok := resourceIDFromPath(r.URL.Path, "/api/v1/workorch/goals/")
		if !ok {
			http.NotFound(w, r)
			return
		}
		state.metrics.goalGetRequests.Add(1)
		item, err := state.readGoal(r.Context(), state.config.Backend, state.config.DataRoot, goalID)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, goalResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Goal:     item,
		})
	})
	router.AddHandler("/api/v1/workorch/workspaces/:workspaceId/dods", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		workspaceID, action, ok := workspaceActionFromPath(r.URL.Path)
		if !ok || action != "dods" {
			http.NotFound(w, r)
			return
		}
		state.metrics.dodListRequests.Add(1)
		items, err := state.listDoDs(r.Context(), state.config.Backend, state.config.DataRoot, workspaceID)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, dodsResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			DoDs:     items,
		})
	})
	router.AddHandler("/api/v1/workorch/dods/:dodId", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		dodID, ok := resourceIDFromPath(r.URL.Path, "/api/v1/workorch/dods/")
		if !ok {
			http.NotFound(w, r)
			return
		}
		state.metrics.dodGetRequests.Add(1)
		item, err := state.readDoD(r.Context(), state.config.Backend, state.config.DataRoot, dodID)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, dodResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			DoD:      item,
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
	router.AddHandler("/api/v1/workorch/runs/:runId/observability", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		runID, action, ok := runActionFromPath(r.URL.Path)
		if !ok || action != "observability" {
			http.NotFound(w, r)
			return
		}
		state.metrics.runObservabilityRequests.Add(1)
		item, err := state.readRunObservability(r.Context(), state.config.Backend, state.config.DataRoot, runID)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, runObservabilityResponse{
			Service:       "workorch",
			Backend:       state.config.Backend,
			DataRoot:      state.config.DataRoot,
			Observability: item,
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
	router.AddHandler("/api/v1/workorch/souls", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		state.metrics.soulListRequests.Add(1)
		items, err := state.listSouls(r.Context(), state.config.Backend, state.config.DataRoot)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, soulsResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Souls:    items,
		})
	})
	router.AddHandler("/api/v1/workorch/souls/:soulId", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		soulID, ok := resourceIDFromPath(r.URL.Path, "/api/v1/workorch/souls/")
		if !ok {
			http.NotFound(w, r)
			return
		}
		state.metrics.soulGetRequests.Add(1)
		item, err := state.readSoul(r.Context(), state.config.Backend, state.config.DataRoot, soulID)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, soulResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Soul:     item,
		})
	})
	router.AddHandler("/api/v1/workorch/skills", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		state.metrics.skillListRequests.Add(1)
		items, err := state.listSkills(r.Context(), state.config.Backend, state.config.DataRoot)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, skillsResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Skills:   items,
		})
	})
	router.AddHandler("/api/v1/workorch/skills/:skillId", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		skillID, ok := resourceIDFromPath(r.URL.Path, "/api/v1/workorch/skills/")
		if !ok {
			http.NotFound(w, r)
			return
		}
		state.metrics.skillGetRequests.Add(1)
		item, err := state.readSkill(r.Context(), state.config.Backend, state.config.DataRoot, skillID)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, skillResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Skill:    item,
		})
	})
	router.AddHandler("/api/v1/workorch/mcp/servers", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		state.metrics.mcpServerListRequests.Add(1)
		items, err := state.listMCPServers(r.Context(), state.config.Backend, state.config.DataRoot)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, mcpServersResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Servers:  items,
		})
	})
	router.AddHandler("/api/v1/workorch/mcp/servers/:serverId", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		serverID, action, ok := mcpServerActionFromPath(r.URL.Path)
		if !ok {
			http.NotFound(w, r)
			return
		}
		if action != "" {
			http.NotFound(w, r)
			return
		}
		state.metrics.mcpServerGetRequests.Add(1)
		item, err := state.readMCPServer(r.Context(), state.config.Backend, state.config.DataRoot, serverID)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, mcpServerResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Server:   item,
		})
	})
	router.AddHandler("/api/v1/workorch/mcp/servers/:serverId/tools", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		serverID, action, ok := mcpServerActionFromPath(r.URL.Path)
		if !ok || action != "tools" {
			http.NotFound(w, r)
			return
		}
		state.metrics.mcpToolListRequests.Add(1)
		items, err := state.listMCPServerTools(r.Context(), state.config.Backend, state.config.DataRoot, serverID)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, mcpToolsResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Tools:    items,
		})
	})
	router.AddHandler("/api/v1/workorch/memory", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		workspaceID := r.URL.Query().Get("workspaceId")
		if workspaceID == "" {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "workspaceId is required"})
			return
		}
		state.metrics.memoryListRequests.Add(1)
		items, err := state.listMemoryByWorkspace(r.Context(), state.config.Backend, state.config.DataRoot, workspaceID, r.URL.Query().Get("q"))
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, memoryItemsResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Memory:   items,
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
		if req.WorkspaceID != "" {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "workspaceId is generated by workorch"})
			return
		}
		item, err := state.createWorkspace(r.Context(), state.config.Backend, state.config.DataRoot, req.Title, req.Description, req.WorkingDirectory)
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
		if req.GoalID == "" || req.DoDID == "" {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "goalId and doDId must reference defined workspace goal and DoD"})
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
	router.AddHandler("/api/v1/workorch/workspaces/:workspaceId/execution-activations", http.MethodPost, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		workspaceID, action, ok := workspaceActionFromPath(r.URL.Path)
		if !ok || action != "execution-activations" {
			http.NotFound(w, r)
			return
		}
		var req activateWorkspaceExecutionRequest
		if err := decodeOptionalJSON(r, &req); err != nil {
			writeBusinessError(w, err)
			return
		}
		state.metrics.workspaceActivationRequests.Add(1)
		release, err := state.acquireRunSlot(r.Context())
		if err != nil {
			state.metrics.runCanceled.Add(1)
			writeRunQueueError(w, err)
			return
		}
		defer release()
		item, err := state.activateWorkspace(r.Context(), state.config.Backend, state.config.DataRoot, workspaceID, appservice.ActivateWorkspaceExecutionCommand{
			GoalID:        req.GoalID,
			DoDID:         req.DoDID,
			LLMProviderID: req.LLMProviderID,
			SoulID:        req.SoulID,
			SkillIDs:      req.SkillIDs,
			MCPServerIDs:  req.MCPServerIDs,
			MemoryIDs:     req.MemoryIDs,
		})
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		state.metrics.runSucceeded.Add(1)
		writeJSON(w, http.StatusOK, workspaceExecutionActivationResponse{
			Service:    "workorch",
			Backend:    state.config.Backend,
			DataRoot:   state.config.DataRoot,
			Activation: item,
		})
	})
	router.AddHandler("/api/v1/workorch/workspaces/:workspaceId/goals", http.MethodPost, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		workspaceID, action, ok := workspaceActionFromPath(r.URL.Path)
		if !ok || action != "goals" {
			http.NotFound(w, r)
			return
		}
		state.metrics.goalDefineRequests.Add(1)
		var req defineGoalRequest
		if err := decodeJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		item, err := state.defineGoal(r.Context(), state.config.Backend, state.config.DataRoot, workspaceID, req.Text)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, goalResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Goal:     item,
		})
	})
	router.AddHandler("/api/v1/workorch/workspaces/:workspaceId/dods", http.MethodPost, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		workspaceID, action, ok := workspaceActionFromPath(r.URL.Path)
		if !ok || action != "dods" {
			http.NotFound(w, r)
			return
		}
		state.metrics.dodDefineRequests.Add(1)
		var req defineDoDRequest
		if err := decodeJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		item, err := state.defineDoD(r.Context(), state.config.Backend, state.config.DataRoot, workspaceID, req.Items)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, dodResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			DoD:      item,
		})
	})
	router.AddHandler("/api/v1/workorch/workspaces/:workspaceId/memory", http.MethodPost, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		workspaceID, action, ok := workspaceActionFromPath(r.URL.Path)
		if !ok || action != "memory" {
			http.NotFound(w, r)
			return
		}
		state.metrics.memoryCreateRequests.Add(1)
		var req createMemoryItemRequest
		if err := decodeJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		item, err := state.createMemoryItem(r.Context(), state.config.Backend, state.config.DataRoot, workspaceID, req)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, memoryItemResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Memory:   item,
		})
	})
	router.AddHandler("/api/v1/workorch/souls", http.MethodPost, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		state.metrics.soulCreateRequests.Add(1)
		var req createSoulRequest
		if err := decodeJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		item, err := state.createSoul(r.Context(), state.config.Backend, state.config.DataRoot, req)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, soulResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Soul:     item,
		})
	})
	router.AddHandler("/api/v1/workorch/skills", http.MethodPost, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		state.metrics.skillCreateRequests.Add(1)
		var req createSkillRequest
		if err := decodeJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		item, err := state.createSkill(r.Context(), state.config.Backend, state.config.DataRoot, req)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, skillResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Skill:    item,
		})
	})
	router.AddHandler("/api/v1/workorch/mcp/servers", http.MethodPost, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		state.metrics.mcpServerCreateRequests.Add(1)
		var req createMCPServerRequest
		if err := decodeJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		item, err := state.createMCPServer(r.Context(), state.config.Backend, state.config.DataRoot, req)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, mcpServerResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Server:   item,
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
		var req resolveDecisionRequest
		if err := decodeOptionalJSON(r, &req); err != nil {
			writeBusinessError(w, err)
			return
		}
		state.metrics.decisionResolveRequests.Add(1)
		item, err := state.resolveDecision(r.Context(), state.config.Backend, state.config.DataRoot, ticketID, appservice.ResolveDecisionCommand{
			ResolutionType:    req.ResolutionType,
			ResolutionPayload: req.ResolutionPayload,
		})
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
		config:                 config,
		eventHub:               eventHub,
		metrics:                &serverMetrics{},
		validate:               defaultValidate,
		createWorkspace:        defaultCreateWorkspace,
		readWorkspace:          defaultReadWorkspace,
		readWorkspaceSnapshot:  defaultReadWorkspaceSnapshot,
		listWorkspaces:         defaultListWorkspaces,
		defineGoal:             defaultDefineGoal,
		readGoal:               defaultReadGoal,
		listGoals:              defaultListGoals,
		defineDoD:              defaultDefineDoD,
		readDoD:                defaultReadDoD,
		listDoDs:               defaultListDoDs,
		createRun:              defaultCreateRun,
		readRun:                defaultReadRun,
		readRunSnapshot:        defaultReadRunSnapshot,
		readRunStatus:          defaultReadRunStatus,
		readRunObservability:   defaultReadRunObservability,
		listRuns:               defaultListRuns,
		startRun:               defaultStartRun,
		resumeRun:              defaultResumeRun,
		activateWorkspace:      defaultActivateWorkspaceExecution,
		listDecisions:          defaultListDecisions,
		readDecision:           defaultReadDecision,
		resolveDecision:        defaultResolveDecision,
		listApprovals:          defaultListApprovals,
		readApproval:           defaultReadApproval,
		approveApproval:        defaultApproveApproval,
		rejectApproval:         defaultRejectApproval,
		createSoul:             defaultCreateSoul,
		readSoul:               defaultReadSoul,
		listSouls:              defaultListSouls,
		createSkill:            defaultCreateSkill,
		readSkill:              defaultReadSkill,
		listSkills:             defaultListSkills,
		createMCPServer:        defaultCreateMCPServer,
		readMCPServer:          defaultReadMCPServer,
		listMCPServers:         defaultListMCPServers,
		listMCPServerTools:     defaultListMCPServerTools,
		createMemoryItem:       defaultCreateMemoryItem,
		listMemoryByWorkspace:  defaultListMemoryByWorkspace,
		readConfiguration:      defaultReadConfiguration,
		updateLLMConfiguration: appservice.UpdateLLMConfiguration,
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
	s.createWorkspace = func(ctx context.Context, _, _, title, description, workingDirectory string) (appservice.Workspace, error) {
		var ret appservice.Workspace
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.CreateWorkspaceWithSession(ctx, session, title, description, workingDirectory)
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
	s.defineGoal = func(ctx context.Context, _, _, workspaceID, text string) (appservice.Goal, error) {
		var ret appservice.Goal
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.DefineGoalWithSession(ctx, session, workspaceID, text)
			return err
		})
		return ret, err
	}
	s.readGoal = func(ctx context.Context, _, _, goalID string) (appservice.Goal, error) {
		var ret appservice.Goal
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ReadGoalWithSession(ctx, session, goalID)
			return err
		})
		return ret, err
	}
	s.listGoals = func(ctx context.Context, _, _, workspaceID string) ([]appservice.Goal, error) {
		var ret []appservice.Goal
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ListGoalsWithSession(ctx, session, workspaceID)
			return err
		})
		return ret, err
	}
	s.defineDoD = func(ctx context.Context, _, _, workspaceID string, items []string) (appservice.DoD, error) {
		var ret appservice.DoD
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.DefineDoDWithSession(ctx, session, workspaceID, items)
			return err
		})
		return ret, err
	}
	s.readDoD = func(ctx context.Context, _, _, dodID string) (appservice.DoD, error) {
		var ret appservice.DoD
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ReadDoDWithSession(ctx, session, dodID)
			return err
		})
		return ret, err
	}
	s.listDoDs = func(ctx context.Context, _, _, workspaceID string) ([]appservice.DoD, error) {
		var ret []appservice.DoD
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ListDoDsWithSession(ctx, session, workspaceID)
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
	s.readRunObservability = func(ctx context.Context, _, _, runID string) (appservice.RunObservability, error) {
		var ret appservice.RunObservability
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ReadRunObservabilityWithSession(ctx, session, runID)
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
	s.activateWorkspace = func(ctx context.Context, _, _, workspaceID string, cmd appservice.ActivateWorkspaceExecutionCommand) (appservice.WorkspaceExecutionActivation, error) {
		var ret appservice.WorkspaceExecutionActivation
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ActivateWorkspaceExecutionWithSession(ctx, session, workspaceID, cmd)
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
	s.resolveDecision = func(ctx context.Context, _, _, ticketID string, cmd appservice.ResolveDecisionCommand) (appservice.Decision, error) {
		var ret appservice.Decision
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ResolveDecisionWithSession(ctx, session, ticketID, cmd)
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
	s.createSoul = func(ctx context.Context, _, _ string, cmd appservice.CreateSoulCommand) (appservice.Soul, error) {
		var ret appservice.Soul
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.CreateSoulWithSession(ctx, session, cmd)
			return err
		})
		return ret, err
	}
	s.readSoul = func(ctx context.Context, _, _, soulID string) (appservice.Soul, error) {
		var ret appservice.Soul
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ReadSoulWithSession(ctx, session, soulID)
			return err
		})
		return ret, err
	}
	s.listSouls = func(ctx context.Context, _, _ string) ([]appservice.Soul, error) {
		var ret []appservice.Soul
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ListSoulsWithSession(ctx, session)
			return err
		})
		return ret, err
	}
	s.createSkill = func(ctx context.Context, _, _ string, cmd appservice.CreateSkillCommand) (appservice.Skill, error) {
		var ret appservice.Skill
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.CreateSkillWithSession(ctx, session, cmd)
			return err
		})
		return ret, err
	}
	s.readSkill = func(ctx context.Context, _, _, skillID string) (appservice.Skill, error) {
		var ret appservice.Skill
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ReadSkillWithSession(ctx, session, skillID)
			return err
		})
		return ret, err
	}
	s.listSkills = func(ctx context.Context, _, _ string) ([]appservice.Skill, error) {
		var ret []appservice.Skill
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ListSkillsWithSession(ctx, session)
			return err
		})
		return ret, err
	}
	s.createMCPServer = func(ctx context.Context, _, _ string, cmd appservice.CreateMCPServerCommand) (appservice.MCPServer, error) {
		var ret appservice.MCPServer
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.CreateMCPServerWithSession(ctx, session, cmd)
			return err
		})
		return ret, err
	}
	s.readMCPServer = func(ctx context.Context, _, _, serverID string) (appservice.MCPServer, error) {
		var ret appservice.MCPServer
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ReadMCPServerWithSession(ctx, session, serverID)
			return err
		})
		return ret, err
	}
	s.listMCPServers = func(ctx context.Context, _, _ string) ([]appservice.MCPServer, error) {
		var ret []appservice.MCPServer
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ListMCPServersWithSession(ctx, session)
			return err
		})
		return ret, err
	}
	s.listMCPServerTools = func(ctx context.Context, _, _, serverID string) ([]appservice.MCPTool, error) {
		var ret []appservice.MCPTool
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ListMCPServerToolsWithSession(ctx, session, serverID)
			return err
		})
		return ret, err
	}
	s.createMemoryItem = func(ctx context.Context, _, _, workspaceID string, cmd appservice.CreateMemoryItemCommand) (appservice.MemoryItem, error) {
		var ret appservice.MemoryItem
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.CreateMemoryItemWithSession(ctx, session, workspaceID, cmd)
			return err
		})
		return ret, err
	}
	s.listMemoryByWorkspace = func(ctx context.Context, _, _, workspaceID string, query string) ([]appservice.MemoryItem, error) {
		var ret []appservice.MemoryItem
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ListMemoryByWorkspaceWithSession(ctx, session, workspaceID, query)
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

func defaultCreateWorkspace(ctx context.Context, backend string, dataRoot string, title string, description string, workingDirectory string) (appservice.Workspace, error) {
	return appservice.CreateWorkspaceWithBackend(ctx, backend, dataRoot, title, description, workingDirectory)
}

func defaultDefineGoal(ctx context.Context, backend string, dataRoot string, workspaceID string, text string) (appservice.Goal, error) {
	return appservice.DefineGoalWithBackend(ctx, backend, dataRoot, workspaceID, text)
}

func defaultReadGoal(ctx context.Context, backend string, dataRoot string, goalID string) (appservice.Goal, error) {
	return appservice.ReadGoalWithBackend(ctx, backend, dataRoot, goalID)
}

func defaultListGoals(ctx context.Context, backend string, dataRoot string, workspaceID string) ([]appservice.Goal, error) {
	return appservice.ListGoalsWithBackend(ctx, backend, dataRoot, workspaceID)
}

func defaultDefineDoD(ctx context.Context, backend string, dataRoot string, workspaceID string, items []string) (appservice.DoD, error) {
	return appservice.DefineDoDWithBackend(ctx, backend, dataRoot, workspaceID, items)
}

func defaultReadDoD(ctx context.Context, backend string, dataRoot string, dodID string) (appservice.DoD, error) {
	return appservice.ReadDoDWithBackend(ctx, backend, dataRoot, dodID)
}

func defaultListDoDs(ctx context.Context, backend string, dataRoot string, workspaceID string) ([]appservice.DoD, error) {
	return appservice.ListDoDsWithBackend(ctx, backend, dataRoot, workspaceID)
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

func defaultReadRunObservability(ctx context.Context, backend string, dataRoot string, runID string) (appservice.RunObservability, error) {
	return appservice.ReadRunObservabilityWithBackend(ctx, backend, dataRoot, runID)
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

func defaultActivateWorkspaceExecution(ctx context.Context, backend string, dataRoot string, workspaceID string, cmd appservice.ActivateWorkspaceExecutionCommand) (appservice.WorkspaceExecutionActivation, error) {
	return appservice.ActivateWorkspaceExecutionWithBackend(ctx, backend, dataRoot, workspaceID, cmd)
}

func defaultListDecisions(ctx context.Context, backend string, dataRoot string, workspaceID string, runID string) ([]appservice.Decision, error) {
	return appservice.ListDecisionsWithBackend(ctx, backend, dataRoot, workspaceID, runID)
}

func defaultReadDecision(ctx context.Context, backend string, dataRoot string, ticketID string) (appservice.Decision, error) {
	return appservice.ReadDecisionWithBackend(ctx, backend, dataRoot, ticketID)
}

func defaultResolveDecision(ctx context.Context, backend string, dataRoot string, ticketID string, cmd appservice.ResolveDecisionCommand) (appservice.Decision, error) {
	return appservice.ResolveDecisionWithBackend(ctx, backend, dataRoot, ticketID, cmd)
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

func defaultCreateSoul(ctx context.Context, backend string, dataRoot string, cmd appservice.CreateSoulCommand) (appservice.Soul, error) {
	return appservice.CreateSoulWithBackend(ctx, backend, dataRoot, cmd)
}

func defaultReadSoul(ctx context.Context, backend string, dataRoot string, soulID string) (appservice.Soul, error) {
	return appservice.ReadSoulWithBackend(ctx, backend, dataRoot, soulID)
}

func defaultListSouls(ctx context.Context, backend string, dataRoot string) ([]appservice.Soul, error) {
	return appservice.ListSoulsWithBackend(ctx, backend, dataRoot)
}

func defaultCreateSkill(ctx context.Context, backend string, dataRoot string, cmd appservice.CreateSkillCommand) (appservice.Skill, error) {
	return appservice.CreateSkillWithBackend(ctx, backend, dataRoot, cmd)
}

func defaultReadSkill(ctx context.Context, backend string, dataRoot string, skillID string) (appservice.Skill, error) {
	return appservice.ReadSkillWithBackend(ctx, backend, dataRoot, skillID)
}

func defaultListSkills(ctx context.Context, backend string, dataRoot string) ([]appservice.Skill, error) {
	return appservice.ListSkillsWithBackend(ctx, backend, dataRoot)
}

func defaultCreateMCPServer(ctx context.Context, backend string, dataRoot string, cmd appservice.CreateMCPServerCommand) (appservice.MCPServer, error) {
	return appservice.CreateMCPServerWithBackend(ctx, backend, dataRoot, cmd)
}

func defaultReadMCPServer(ctx context.Context, backend string, dataRoot string, serverID string) (appservice.MCPServer, error) {
	return appservice.ReadMCPServerWithBackend(ctx, backend, dataRoot, serverID)
}

func defaultListMCPServers(ctx context.Context, backend string, dataRoot string) ([]appservice.MCPServer, error) {
	return appservice.ListMCPServersWithBackend(ctx, backend, dataRoot)
}

func defaultListMCPServerTools(ctx context.Context, backend string, dataRoot string, serverID string) ([]appservice.MCPTool, error) {
	return appservice.ListMCPServerToolsWithBackend(ctx, backend, dataRoot, serverID)
}

func defaultCreateMemoryItem(ctx context.Context, backend string, dataRoot string, workspaceID string, cmd appservice.CreateMemoryItemCommand) (appservice.MemoryItem, error) {
	return appservice.CreateMemoryItemWithBackend(ctx, backend, dataRoot, workspaceID, cmd)
}

func defaultListMemoryByWorkspace(ctx context.Context, backend string, dataRoot string, workspaceID string, query string) ([]appservice.MemoryItem, error) {
	return appservice.ListMemoryByWorkspaceWithBackend(ctx, backend, dataRoot, workspaceID, query)
}

func defaultReadConfiguration() (appservice.ServiceConfiguration, error) {
	return appservice.ReadServiceConfiguration()
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
			"workorch_http_requests_total{endpoint=\"goal_define\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"goal_list\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"goal_get\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"dod_define\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"dod_list\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"dod_get\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"run_create\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"run_list\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"run_get\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"run_snapshot\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"run_status\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"run_observability\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"run_start\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"run_resume\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"workspace_execution_activation\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"decision_list\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"decision_get\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"decision_resolve\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"approval_list\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"approval_get\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"approval_approve\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"approval_reject\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"configuration_read\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"configuration_update\"} %d\n"+
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
		m.goalDefineRequests.Load(),
		m.goalListRequests.Load(),
		m.goalGetRequests.Load(),
		m.dodDefineRequests.Load(),
		m.dodListRequests.Load(),
		m.dodGetRequests.Load(),
		m.runCreateRequests.Load(),
		m.runListRequests.Load(),
		m.runGetRequests.Load(),
		m.runSnapshotRequests.Load(),
		m.runStatusRequests.Load(),
		m.runObservabilityRequests.Load(),
		m.runStartRequests.Load(),
		m.runResumeRequests.Load(),
		m.workspaceActivationRequests.Load(),
		m.decisionListRequests.Load(),
		m.decisionGetRequests.Load(),
		m.decisionResolveRequests.Load(),
		m.approvalListRequests.Load(),
		m.approvalGetRequests.Load(),
		m.approvalApproveRequests.Load(),
		m.approvalRejectRequests.Load(),
		m.configurationReadRequests.Load(),
		m.configurationUpdateRequests.Load(),
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

func resourceIDFromPath(path string, prefix string) (string, bool) {
	if !strings.HasPrefix(path, prefix) {
		return "", false
	}
	trimmed := strings.TrimPrefix(path, prefix)
	if trimmed == "" || strings.Contains(trimmed, "/") {
		return "", false
	}
	return trimmed, true
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

func mcpServerActionFromPath(path string) (serverID string, action string, ok bool) {
	if !strings.HasPrefix(path, "/api/v1/workorch/mcp/servers/") {
		return "", "", false
	}
	trimmed := strings.TrimPrefix(path, "/api/v1/workorch/mcp/servers/")
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
	cfg, err := workorchconfig.Current()
	if err != nil {
		return 0, err
	}
	serverConfig, err := ConfigFromWorkorch(cfg)
	if err != nil {
		return 0, err
	}
	switch name {
	case configServerReadHeaderTimeout:
		return serverConfig.ReadHeaderTimeout, nil
	case configServerReadTimeout:
		return serverConfig.ReadTimeout, nil
	case configServerWriteTimeout:
		return serverConfig.WriteTimeout, nil
	case configServerIdleTimeout:
		return serverConfig.IdleTimeout, nil
	case configServerShutdownTimeout:
		return serverConfig.ShutdownTimeout, nil
	default:
		return fallback, nil
	}
}

func durationFromValues(values map[string]string, name string, fallback time.Duration) (time.Duration, error) {
	return durationFromValue(name, values[name], fallback)
}

func durationFromValue(name string, value string, fallback time.Duration) (time.Duration, error) {
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
	cfg, err := workorchconfig.Current()
	if err != nil {
		return 0, err
	}
	if name == configServerMaxConcurrentRuns {
		serverConfig, err := ConfigFromWorkorch(cfg)
		if err != nil {
			return 0, err
		}
		return serverConfig.MaxConcurrentRuns, nil
	}
	return fallback, nil
}

func int64FromValues(values map[string]string, name string, fallback int64) (int64, error) {
	value := values[name]
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

func firstNonEmpty(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
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

func decodeOptionalJSON(r *http.Request, dst any) error {
	if r.Body == nil || r.ContentLength == 0 {
		return nil
	}
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
