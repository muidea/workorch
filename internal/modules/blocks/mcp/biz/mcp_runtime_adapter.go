package mcp

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	blockcommon "github.com/muidea/workorch/internal/modules/blocks/mcp/pkg/common"
	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchmanagedconfig"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type RuntimeAdapter struct {
	service serverGetter
	client  *http.Client
	// idleSessionTimeout belongs to this adapter instance. It must not be a
	// package-global test knob because delayed eviction callbacks can outlive a
	// test and race with another adapter's configuration.
	idleSessionTimeout time.Duration

	sessionMu sync.Mutex
	sessions  map[string]*stdioSession
}

type serverGetter interface {
	GetServer(context.Context, common.MCPServerID) (*mcpmodel.Server, error)
}

type serverLister interface {
	ListServers(context.Context) ([]*mcpmodel.Server, error)
}

const (
	mcpProtocolVersion = "2025-11-25"
	mcpClientName      = "workorch"
	mcpClientVersion   = "mvp"

	sessionPolicyLongLived = "long_lived"

	defaultLongLivedStdioSessionIdleTimeout = 5 * time.Minute

	// maxMCPObservedFailureTargets bounds a capability owner's structured
	// failure observation before it crosses the typed runtime receipt boundary.
	maxMCPObservedFailureTargets = 16
)

type operationSpec struct {
	Operation  string
	ServerID   string
	Tool       string
	URI        string
	Prompt     string
	Input      []orchestratorevents.MCPArgument
	Arguments  []orchestratorevents.MCPArgument
	Summary    string
	Stream     bool
	TargetRefs []string
	TargetRole string
}

type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonRPCError   `json:"error,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type jsonRPCError struct {
	Code    int             `json:"code,omitempty"`
	Message string          `json:"message,omitempty"`
	Data    json.RawMessage `json:"data,omitempty"`
}

type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type mcpHTTPPayload struct {
	WorkspaceID common.WorkspaceID         `json:"workspaceId"`
	RunID       common.RunID               `json:"runId"`
	StepID      common.StepID              `json:"stepId"`
	ServerID    string                     `json:"serverId"`
	Operation   string                     `json:"operation"`
	Tool        string                     `json:"tool,omitempty"`
	URI         string                     `json:"uri,omitempty"`
	Prompt      string                     `json:"prompt,omitempty"`
	Input       map[string]json.RawMessage `json:"input,omitempty"`
	Arguments   map[string]json.RawMessage `json:"arguments,omitempty"`
	Stream      bool                       `json:"stream,omitempty"`
}

type stdioSession struct {
	serverID  string
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	reader    *bufio.Reader
	stderr    *limitedBuffer
	done      chan error
	lastUsed  time.Time
	mu        sync.Mutex
	closeOnce sync.Once
}

type limitedBuffer struct {
	mu    sync.Mutex
	buf   bytes.Buffer
	limit int
}

func NewRuntimeAdapter(service serverGetter) *RuntimeAdapter {
	return NewRuntimeAdapterWithHTTPClient(service, http.DefaultClient)
}

func NewRuntimeAdapterWithHTTPClient(service serverGetter, client *http.Client) *RuntimeAdapter {
	return newRuntimeAdapterWithIdleSessionTimeout(service, client, defaultLongLivedStdioSessionIdleTimeout)
}

func newRuntimeAdapterWithIdleSessionTimeout(service serverGetter, client *http.Client, idleSessionTimeout time.Duration) *RuntimeAdapter {
	if client == nil {
		client = http.DefaultClient
	}
	return &RuntimeAdapter{service: service, client: client, idleSessionTimeout: idleSessionTimeout, sessions: map[string]*stdioSession{}}
}

func (a *RuntimeAdapter) ExecuteOperation(ctx context.Context, request RuntimeOperationRequest) (RuntimeOperationResult, error) {
	spec, err := parseOperationSpec(request)
	if err != nil {
		return failedResult(spec, invalidStage(spec), err), err
	}
	if a == nil || a.service == nil {
		err := common.ErrInvalidArgument
		return failedResult(spec, unavailableStage(spec), err), err
	}
	server, err := a.service.GetServer(ctx, common.MCPServerID(spec.ServerID))
	if err != nil {
		return failedResult(spec, serverUnavailableStage(spec), err), err
	}
	if err := validateServerPolicy(server, spec); err != nil {
		return failedResult(spec, rejectedStage(spec), err), err
	}
	switch strings.ToLower(strings.TrimSpace(server.Transport)) {
	case "http", "https":
		return a.executeHTTPOperation(ctx, request, server, spec)
	case "stdio":
		return a.executeStdioOperation(ctx, request, server, spec)
	default:
		err := fmt.Errorf("%w: unsupported mcp transport %q", common.ErrInvalidArgument, server.Transport)
		return failedResult(spec, unsupportedTransportStage(spec), err), err
	}
}

// ProbeOperation validates the selected MCP server and its local transport
// launch prerequisites without creating a stdio session or sending an HTTP
// request. Runtime V2 uses it before dispatch seal; execution remains the only
// path that can call a tool or create a receipt.
func (a *RuntimeAdapter) ProbeOperation(ctx context.Context, request RuntimeOperationRequest) error {
	spec, err := parseOperationSpec(request)
	if err != nil {
		return err
	}
	if a == nil || a.service == nil {
		return common.ErrInvalidArgument
	}
	server, err := a.service.GetServer(ctx, common.MCPServerID(spec.ServerID))
	if err != nil {
		return err
	}
	if err := validateServerPolicy(server, spec); err != nil {
		return err
	}
	switch strings.ToLower(strings.TrimSpace(server.Transport)) {
	case "http", "https":
		endpoint := strings.TrimSpace(server.Endpoint)
		if endpoint == "" {
			return fmt.Errorf("%w: mcp endpoint is required", common.ErrInvalidArgument)
		}
		if _, err := url.ParseRequestURI(endpoint); err != nil {
			return fmt.Errorf("%w: invalid mcp endpoint: %v", common.ErrInvalidArgument, err)
		}
		return nil
	case "stdio":
		command, _, _, err := stdioLaunchConfig(server, request)
		if err != nil {
			return err
		}
		if strings.ContainsAny(command, `/\\`) {
			info, statErr := os.Stat(command)
			if statErr != nil {
				return statErr
			}
			if !info.Mode().IsRegular() {
				return fmt.Errorf("%w: mcp stdio command is not a regular file", common.ErrInvalidArgument)
			}
			return nil
		}
		_, err = exec.LookPath(command)
		return err
	default:
		return fmt.Errorf("%w: unsupported mcp transport %q", common.ErrInvalidArgument, server.Transport)
	}
}

// OperationAdmissibility returns enabled MCP server IDs and a stable hash of
// their effective registry state. It performs only the typed registry query;
// no MCP transport is opened and no capability operation is invoked.
func (a *RuntimeAdapter) OperationAdmissibility(ctx context.Context) ([]string, string, error) {
	if a == nil || a.service == nil {
		return nil, "", common.ErrInvalidArgument
	}
	lister, ok := a.service.(serverLister)
	if !ok {
		return nil, "", fmt.Errorf("%w: mcp server catalog is unavailable", common.ErrInvalidArgument)
	}
	servers, err := lister.ListServers(ctx)
	if err != nil {
		return nil, "", err
	}
	orderedServers := append([]*mcpmodel.Server(nil), servers...)
	sort.Slice(orderedServers, func(i, j int) bool {
		if orderedServers[i] == nil {
			return false
		}
		if orderedServers[j] == nil {
			return true
		}
		return string(orderedServers[i].ID) < string(orderedServers[j].ID)
	})
	ids := make([]string, 0, len(orderedServers))
	persisted := make([]mcpmodel.Server, 0, len(orderedServers))
	for _, server := range orderedServers {
		if server == nil || !server.Enabled || strings.TrimSpace(string(server.ID)) == "" {
			continue
		}
		ids = append(ids, string(server.ID))
		persisted = append(persisted, *server)
	}
	if len(ids) == 0 {
		return nil, "", fmt.Errorf("%w: no enabled mcp server is available", common.ErrNotFound)
	}
	raw, err := json.Marshal(persisted)
	if err != nil {
		return nil, "", err
	}
	sum := sha256.Sum256(raw)
	return ids, "mcp-admissibility-" + hex.EncodeToString(sum[:])[:20], nil
}

func (a *RuntimeAdapter) Close() {
	if a == nil {
		return
	}
	a.sessionMu.Lock()
	sessions := a.sessions
	a.sessions = map[string]*stdioSession{}
	a.sessionMu.Unlock()
	for _, session := range sessions {
		session.Close()
	}
}

func (a *RuntimeAdapter) executeHTTPOperation(ctx context.Context, request RuntimeOperationRequest, server *mcpmodel.Server, spec operationSpec) (RuntimeOperationResult, error) {
	endpoint := strings.TrimSpace(server.Endpoint)
	if endpoint == "" {
		err := fmt.Errorf("%w: mcp endpoint is required", common.ErrInvalidArgument)
		return failedResult(spec, invalidEndpointStage(spec), err), err
	}
	if _, err := url.ParseRequestURI(endpoint); err != nil {
		err = fmt.Errorf("%w: invalid mcp endpoint: %v", common.ErrInvalidArgument, err)
		return failedResult(spec, invalidEndpointStage(spec), err), err
	}
	input, err := mcpArgumentsJSON(spec.Input)
	if err != nil {
		return failedResult(spec, invalidStage(spec), err), err
	}
	arguments, err := mcpArgumentsJSON(spec.Arguments)
	if err != nil {
		return failedResult(spec, invalidStage(spec), err), err
	}
	payload := mcpHTTPPayload{
		WorkspaceID: request.WorkspaceID, RunID: request.RunID, StepID: request.StepID,
		ServerID: spec.ServerID, Operation: spec.Operation, Tool: spec.Tool, URI: spec.URI,
		Prompt: spec.Prompt, Input: input, Arguments: arguments, Stream: spec.Stream,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return failedResult(spec, invalidStage(spec), err), err
	}
	callCtx, cancel := context.WithTimeout(ctx, timeoutFromServer(server))
	defer cancel()
	httpReq, err := http.NewRequestWithContext(callCtx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		err = fmt.Errorf("%w: create mcp request: %v", common.ErrInvalidArgument, err)
		return failedResult(spec, invalidEndpointStage(spec), err), err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json, text/event-stream")
	startedAt := time.Now().UTC()
	resp, err := a.client.Do(httpReq)
	duration := time.Since(startedAt)
	if err != nil {
		stage := failedStage(spec)
		if callCtx.Err() == context.DeadlineExceeded {
			stage = timeoutStage(spec)
			err = context.DeadlineExceeded
		}
		output := failureOutput(spec, stage, err)
		output.DurationMillis = duration.Milliseconds()
		return finalizeMCPResult(output, err)
	}
	defer resp.Body.Close()
	raw, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if readErr != nil {
		output := failureOutput(spec, failedStage(spec), readErr)
		output.StatusCode = int64(resp.StatusCode)
		output.DurationMillis = duration.Milliseconds()
		return finalizeMCPResult(output, readErr)
	}
	responsePayload, streamEvents := parseHTTPResponsePayload(raw, resp.Header.Get("Content-Type"), spec.Stream)
	output := successOutput(spec, server, duration, responsePayload)
	output.StatusCode = int64(resp.StatusCode)
	output.StreamEvents = append([]MCPStreamEvent(nil), streamEvents...)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		err := fmt.Errorf("mcp endpoint returned status %d", resp.StatusCode)
		output.Status = "failed"
		output.Stage = failedStage(spec)
		output.FailureMessage = err.Error()
		output.Diagnostics = capabilityFailureDiagnostics(spec)
		a.bindMCPFailureObservationRevision(ctx, &output)
		return finalizeMCPResult(output, err)
	}
	if err := toolResultError(spec, responsePayload); err != nil {
		output.Status = "failed"
		output.Stage = failedStage(spec)
		output.FailureMessage = err.Error()
		output.Diagnostics = capabilityFailureDiagnostics(spec)
		a.bindMCPFailureObservationRevision(ctx, &output)
		return finalizeMCPResult(output, err)
	}
	return finalizeMCPResult(output, nil)
}

func (a *RuntimeAdapter) executeStdioOperation(ctx context.Context, request RuntimeOperationRequest, server *mcpmodel.Server, spec operationSpec) (RuntimeOperationResult, error) {
	callCtx, cancel := context.WithTimeout(ctx, timeoutFromServer(server))
	defer cancel()
	if strings.EqualFold(strings.TrimSpace(server.SessionPolicy), sessionPolicyLongLived) {
		return a.executeLongLivedStdioOperation(callCtx, request, server, spec)
	}
	session, err := newInitializedStdioSession(callCtx, request, server, spec.ServerID)
	startedAt := time.Now().UTC()
	if err != nil {
		return failedResult(spec, failedStage(spec), err), err
	}
	response, streamEvents, execErr := session.Execute(callCtx, spec, 2)
	duration := time.Since(startedAt)
	stderrText := session.Close()
	if execErr != nil {
		stage := failedStage(spec)
		if errorsIsDeadline(callCtx) {
			stage = timeoutStage(spec)
			execErr = context.DeadlineExceeded
		}
		return failedStdioResult(spec, stage, execErr, duration, stderrText)
	}
	output := successOutput(spec, server, duration, parseResponsePayload(response.Result))
	if len(streamEvents) > 0 {
		output.StreamEvents = append([]MCPStreamEvent(nil), streamEvents...)
	}
	if stderrText != "" {
		output.Stderr = stderrText
	}
	if err := toolResultError(spec, parseResponsePayload(response.Result)); err != nil {
		output.Status = "failed"
		output.Stage = failedStage(spec)
		output.FailureMessage = err.Error()
		output.Diagnostics = capabilityFailureDiagnostics(spec)
		a.bindMCPFailureObservationRevision(ctx, &output)
		return finalizeMCPResult(output, err)
	}
	return finalizeMCPResult(output, nil)
}

func (a *RuntimeAdapter) executeLongLivedStdioOperation(ctx context.Context, request RuntimeOperationRequest, server *mcpmodel.Server, spec operationSpec) (RuntimeOperationResult, error) {
	startedAt := time.Now().UTC()
	sessionKey := stdioSessionKey(request.WorkspaceID, spec.ServerID)
	session, err := a.getOrCreateStdioSession(ctx, request, server, sessionKey)
	if err != nil {
		return failedResult(spec, failedStage(spec), err), err
	}
	response, streamEvents, execErr := session.Execute(ctx, spec, nextJSONRPCID())
	duration := time.Since(startedAt)
	if execErr != nil {
		a.evictStdioSession(sessionKey, session)
		stderrText := session.Close()
		stage := failedStage(spec)
		if errorsIsDeadline(ctx) {
			stage = timeoutStage(spec)
			execErr = context.DeadlineExceeded
		}
		return failedStdioResult(spec, stage, execErr, duration, stderrText)
	}
	output := successOutput(spec, server, duration, parseResponsePayload(response.Result))
	if len(streamEvents) > 0 {
		output.StreamEvents = append([]MCPStreamEvent(nil), streamEvents...)
	}
	if stderrText := session.Stderr(); stderrText != "" {
		output.Stderr = stderrText
	}
	if err := toolResultError(spec, parseResponsePayload(response.Result)); err != nil {
		output.Status = "failed"
		output.Stage = failedStage(spec)
		output.FailureMessage = err.Error()
		output.Diagnostics = capabilityFailureDiagnostics(spec)
		a.bindMCPFailureObservationRevision(ctx, &output)
		return finalizeMCPResult(output, err)
	}
	return finalizeMCPResult(output, nil)
}

func (a *RuntimeAdapter) getOrCreateStdioSession(ctx context.Context, request RuntimeOperationRequest, server *mcpmodel.Server, serverID string) (*stdioSession, error) {
	a.sessionMu.Lock()
	if session := a.sessions[serverID]; session != nil {
		a.sessionMu.Unlock()
		return session, nil
	}
	a.sessionMu.Unlock()

	session, err := newInitializedStdioSession(ctx, request, server, serverID)
	if err != nil {
		return nil, err
	}
	a.sessionMu.Lock()
	if existing := a.sessions[serverID]; existing != nil {
		a.sessionMu.Unlock()
		session.Close()
		return existing, nil
	}
	a.sessions[serverID] = session
	a.sessionMu.Unlock()
	a.scheduleStdioSessionIdleEviction(serverID, session)
	return session, nil
}

func (a *RuntimeAdapter) evictStdioSession(serverID string, target *stdioSession) bool {
	a.sessionMu.Lock()
	if a.sessions[serverID] == target {
		delete(a.sessions, serverID)
		a.sessionMu.Unlock()
		return true
	}
	a.sessionMu.Unlock()
	return false
}

func (a *RuntimeAdapter) scheduleStdioSessionIdleEviction(serverID string, session *stdioSession) {
	if a == nil || session == nil || a.idleSessionTimeout <= 0 {
		return
	}
	time.AfterFunc(a.idleSessionTimeout, func() {
		a.evictIdleStdioSession(serverID, session)
	})
}

func (a *RuntimeAdapter) evictIdleStdioSession(serverID string, session *stdioSession) {
	if a == nil || session == nil {
		return
	}
	ttl := a.idleSessionTimeout
	if ttl <= 0 {
		return
	}
	session.mu.Lock()
	idleFor := time.Since(session.lastUsed)
	if idleFor < ttl {
		delay := ttl - idleFor
		session.mu.Unlock()
		time.AfterFunc(delay, func() {
			a.evictIdleStdioSession(serverID, session)
		})
		return
	}
	session.mu.Unlock()
	if a.evictStdioSession(serverID, session) {
		session.Close()
	}
}

func newInitializedStdioSession(ctx context.Context, request RuntimeOperationRequest, server *mcpmodel.Server, serverID string) (*stdioSession, error) {
	command, args, env, err := stdioLaunchConfig(server, request)
	if err != nil {
		return nil, err
	}
	initCtx, cancel := context.WithTimeout(ctx, timeoutFromServer(server))
	defer cancel()

	launchCtx := ctx
	if strings.EqualFold(strings.TrimSpace(server.SessionPolicy), sessionPolicyLongLived) {
		launchCtx = context.WithoutCancel(ctx)
	}
	cmd := exec.CommandContext(launchCtx, command, args...)
	if dir := strings.TrimSpace(request.WorkspaceDeliverableDirectory); dir != "" {
		cmd.Dir = dir
	}
	if len(env) > 0 {
		cmd.Env = mergeEnv(os.Environ(), env)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	session := &stdioSession{
		serverID: serverID,
		cmd:      cmd,
		stdin:    stdin,
		reader:   bufio.NewReader(stdout),
		stderr:   &limitedBuffer{limit: 64 << 10},
		done:     make(chan error, 1),
		lastUsed: time.Now().UTC(),
	}
	go func() {
		_, _ = io.Copy(session.stderr, stderr)
	}()
	go func() {
		session.done <- cmd.Wait()
	}()

	initializeRaw, err := json.Marshal(struct {
		ProtocolVersion string   `json:"protocolVersion"`
		Capabilities    struct{} `json:"capabilities"`
		ClientInfo      struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"clientInfo"`
	}{
		ProtocolVersion: mcpProtocolVersion,
		ClientInfo: struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		}{Name: mcpClientName, Version: mcpClientVersion},
	})
	if err != nil {
		session.Close()
		return nil, err
	}
	initializeParams := json.RawMessage(initializeRaw)
	if err := writeJSONRPCLine(stdin, jsonRPCRequest{JSONRPC: "2.0", ID: 1, Method: "initialize", Params: initializeParams}); err != nil {
		session.Close()
		return nil, err
	}
	if _, _, err := readJSONRPCResponseWithContext(initCtx, session.reader, 1, session); err != nil {
		session.Close()
		return nil, err
	}
	initializedParams := json.RawMessage("{}")
	if err := writeJSONRPCLine(stdin, jsonRPCRequest{JSONRPC: "2.0", Method: "notifications/initialized", Params: initializedParams}); err != nil {
		session.Close()
		return nil, err
	}
	return session, nil
}

func (s *stdioSession) Execute(ctx context.Context, spec operationSpec, id int) (jsonRPCResponse, []MCPStreamEvent, error) {
	if s == nil {
		return jsonRPCResponse{}, nil, common.ErrInvalidArgument
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	request := jsonRPCRequest{JSONRPC: "2.0", ID: id}
	var params json.RawMessage
	var err error
	switch spec.Operation {
	case blockcommon.StepKindToolCall:
		arguments, encodeErr := mcpArgumentsJSON(spec.Input)
		if encodeErr != nil {
			return jsonRPCResponse{}, nil, encodeErr
		}
		request.Method = "tools/call"
		var raw []byte
		raw, err = json.Marshal(struct {
			Name      string                     `json:"name"`
			Arguments map[string]json.RawMessage `json:"arguments"`
		}{Name: spec.Tool, Arguments: arguments})
		params = json.RawMessage(raw)
	case blockcommon.StepKindResourceRead:
		request.Method = "resources/read"
		var raw []byte
		raw, err = json.Marshal(struct {
			URI string `json:"uri"`
		}{URI: spec.URI})
		params = json.RawMessage(raw)
	case blockcommon.StepKindPromptGet:
		arguments, encodeErr := mcpArgumentsJSON(spec.Arguments)
		if encodeErr != nil {
			return jsonRPCResponse{}, nil, encodeErr
		}
		request.Method = "prompts/get"
		var raw []byte
		raw, err = json.Marshal(struct {
			Name      string                     `json:"name"`
			Arguments map[string]json.RawMessage `json:"arguments"`
		}{Name: spec.Prompt, Arguments: arguments})
		params = json.RawMessage(raw)
	default:
		return jsonRPCResponse{}, nil, common.ErrInvalidArgument
	}
	if err != nil {
		return jsonRPCResponse{}, nil, err
	}
	request.Params = params
	if err := writeJSONRPCLine(s.stdin, request); err != nil {
		return jsonRPCResponse{}, nil, err
	}
	response, streamEvents, err := readJSONRPCResponseWithContext(ctx, s.reader, id, s)
	s.lastUsed = time.Now().UTC()
	return response, streamEvents, err
}

func (s *stdioSession) Close() string {
	if s == nil {
		return ""
	}
	s.closeOnce.Do(func() {
		_ = s.stdin.Close()
		select {
		case <-s.done:
		case <-time.After(2 * time.Second):
			if s.cmd != nil && s.cmd.Process != nil {
				_ = s.cmd.Process.Signal(syscall.SIGTERM)
			}
			select {
			case <-s.done:
			case <-time.After(2 * time.Second):
				if s.cmd != nil && s.cmd.Process != nil {
					_ = s.cmd.Process.Kill()
				}
				<-s.done
			}
		}
	})
	return s.Stderr()
}

func (s *stdioSession) Stderr() string {
	if s == nil || s.stderr == nil {
		return ""
	}
	return s.stderr.String()
}

func parseOperationSpec(request RuntimeOperationRequest) (operationSpec, error) {
	spec := operationSpec{
		Operation:  strings.TrimSpace(request.Kind),
		ServerID:   strings.TrimSpace(request.Data.ServerID),
		Tool:       strings.TrimSpace(request.Data.Tool),
		URI:        strings.TrimSpace(request.Data.URI),
		Prompt:     strings.TrimSpace(request.Data.Prompt),
		Input:      append([]orchestratorevents.MCPArgument(nil), request.Data.Input...),
		Arguments:  append([]orchestratorevents.MCPArgument(nil), request.Data.Arguments...),
		Summary:    strings.TrimSpace(request.Data.Summary),
		Stream:     request.Data.Stream,
		TargetRefs: append([]string(nil), request.TargetRefs...), TargetRole: strings.TrimSpace(request.TargetRole),
	}
	if spec.Operation == "" {
		spec.Operation = blockcommon.StepKindToolCall
	}
	if spec.ServerID == "" {
		return spec, fmt.Errorf("%w: serverId is required", common.ErrInvalidArgument)
	}
	switch spec.Operation {
	case blockcommon.StepKindToolCall:
		if spec.Tool == "" {
			return spec, fmt.Errorf("%w: tool is required", common.ErrInvalidArgument)
		}
	case blockcommon.StepKindResourceRead:
		if spec.URI == "" {
			return spec, fmt.Errorf("%w: uri is required", common.ErrInvalidArgument)
		}
	case blockcommon.StepKindPromptGet:
		if spec.Prompt == "" {
			return spec, fmt.Errorf("%w: prompt is required", common.ErrInvalidArgument)
		}
	default:
		return spec, fmt.Errorf("%w: unsupported mcp operation %q", common.ErrInvalidArgument, spec.Operation)
	}
	return spec, nil
}

func validateServerPolicy(server *mcpmodel.Server, spec operationSpec) error {
	if server == nil || server.ID == "" {
		return common.ErrNotFound
	}
	if !server.Enabled {
		return fmt.Errorf("%w: mcp server %s is disabled", common.ErrInvalidArgument, spec.ServerID)
	}
	switch spec.Operation {
	case blockcommon.StepKindToolCall:
		if containsTrimmed(server.AllowedTools, spec.Tool) {
			return nil
		}
		return fmt.Errorf("%w: tool %s is not allowed for mcp server %s", common.ErrInvalidArgument, spec.Tool, spec.ServerID)
	case blockcommon.StepKindResourceRead:
		if containsTrimmed(server.AllowedResources, spec.URI) {
			return nil
		}
		return fmt.Errorf("%w: resource %s is not allowed for mcp server %s", common.ErrInvalidArgument, spec.URI, spec.ServerID)
	case blockcommon.StepKindPromptGet:
		if containsTrimmed(server.AllowedPrompts, spec.Prompt) {
			return nil
		}
		return fmt.Errorf("%w: prompt %s is not allowed for mcp server %s", common.ErrInvalidArgument, spec.Prompt, spec.ServerID)
	default:
		return common.ErrInvalidArgument
	}
}

func containsTrimmed(values []string, target string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == target {
			return true
		}
	}
	return false
}

func failedResult(spec operationSpec, stage string, err error) RuntimeOperationResult {
	output := failureOutput(spec, stage, err)
	output, _ = finalizeMCPResult(output, err)
	return output
}

func failureOutput(spec operationSpec, stage string, err error) RuntimeOperationResult {
	output := baseOutput(spec)
	output.Status = "failed"
	output.Stage = stage
	if err != nil {
		output.FailureMessage = err.Error()
	}
	output.Diagnostics = capabilityFailureDiagnostics(spec)
	return output
}

func successOutput(spec operationSpec, server *mcpmodel.Server, duration time.Duration, result MCPResponsePayload) RuntimeOperationResult {
	output := baseOutput(spec)
	output.Status = "completed"
	output.Stage = completedStage(spec)
	output.Transport = server.Transport
	output.Endpoint = server.Endpoint
	output.DurationMillis = duration.Milliseconds()
	output.Response = result
	return output
}

// bindMCPFailureObservationRevision ties a structured failure target to the
// same owner-catalog revision used by MCP admissibility. If that catalog is no
// longer readable, the result intentionally carries no target observation:
// an unbound target cannot become repair authority.
func (a *RuntimeAdapter) bindMCPFailureObservationRevision(ctx context.Context, output *RuntimeOperationResult) {
	if a == nil || output == nil {
		return
	}
	if len(output.Diagnostics.FailureTargets) == 0 {
		output.Diagnostics.FailureTargets = observedMCPFailureTargets(output.Response)
	}
	if len(output.Diagnostics.FailureTargets) == 0 {
		return
	}
	_, revision, err := a.OperationAdmissibility(ctx)
	if err == nil {
		output.OwnerRevision = revision
	}
}

func baseOutput(spec operationSpec) RuntimeOperationResult {
	return RuntimeOperationResult{
		Operation: spec.Operation, ServerID: spec.ServerID, Tool: spec.Tool, URI: spec.URI,
		Prompt: spec.Prompt, Summary: spec.Summary, TargetRefs: append([]string(nil), spec.TargetRefs...),
	}
}

func capabilityFailureDiagnostics(spec operationSpec) MCPCapabilityFailureDiagnostics {
	diagnostics := MCPCapabilityFailureDiagnostics{
		Operation: spec.Operation, ServerID: spec.ServerID, Tool: spec.Tool, URI: spec.URI, Prompt: spec.Prompt, TargetRole: spec.TargetRole,
	}
	if spec.URI != "" {
		diagnostics.TargetRef = spec.URI
	}
	if target := firstCapabilityInputTarget(spec); target != "" {
		diagnostics.TargetRef = target
	}
	return diagnostics
}

func firstCapabilityInputTarget(spec operationSpec) string {
	for _, name := range []string{"targetRef", "path", "file", "uri", "resourceUri"} {
		for _, argument := range spec.Input {
			if strings.TrimSpace(argument.Name) != name || argument.String == nil {
				continue
			}
			if target := normalizedCapabilityInputTarget(*argument.String); target != "" {
				return target
			}
		}
	}
	return ""
}

func normalizedCapabilityInputTarget(value string) string {
	text := strings.TrimSpace(value)
	if text == "" || strings.Contains(text, "://") {
		return text
	}
	text = filepath.ToSlash(text)
	for strings.HasPrefix(text, "./") {
		text = strings.TrimPrefix(text, "./")
	}
	return strings.Trim(text, "/")
}

func parseHTTPResponsePayload(raw []byte, contentType string, requestedStream bool) (MCPResponsePayload, []MCPStreamEvent) {
	if !requestedStream && !strings.Contains(strings.ToLower(contentType), "text/event-stream") {
		return parseResponsePayload(raw), nil
	}
	events := parseStreamEvents(raw)
	if len(events) == 0 {
		return parseResponsePayload(raw), nil
	}
	return events[len(events)-1].Payload, events
}

func parseStreamEvents(raw []byte) []MCPStreamEvent {
	lines := bytes.Split(raw, []byte{'\n'})
	events := make([]MCPStreamEvent, 0, len(lines))
	var sseData []string
	flushSSE := func() {
		if len(sseData) == 0 {
			return
		}
		events = append(events, MCPStreamEvent{Payload: parseResponsePayload([]byte(strings.Join(sseData, "\n")))})
		sseData = nil
	}
	for _, line := range lines {
		trimmed := strings.TrimSpace(string(line))
		if trimmed == "" {
			flushSSE()
			continue
		}
		if strings.HasPrefix(trimmed, ":") || strings.HasPrefix(trimmed, "event:") {
			continue
		}
		if strings.HasPrefix(trimmed, "data:") {
			sseData = append(sseData, strings.TrimSpace(strings.TrimPrefix(trimmed, "data:")))
			continue
		}
		flushSSE()
		events = append(events, MCPStreamEvent{Payload: parseResponsePayload([]byte(trimmed))})
	}
	flushSSE()
	return events
}

func parseResponsePayload(raw []byte) MCPResponsePayload {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return MCPResponsePayload{JSON: json.RawMessage("{}")}
	}
	if json.Valid(trimmed) {
		return MCPResponsePayload{JSON: append(json.RawMessage(nil), trimmed...)}
	}
	return MCPResponsePayload{Text: string(raw)}
}

func toolResultError(spec operationSpec, result MCPResponsePayload) error {
	if spec.Operation != blockcommon.StepKindToolCall || len(result.JSON) == 0 {
		return nil
	}
	var payload struct {
		IsError bool `json:"isError"`
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(result.JSON, &payload); err != nil || !payload.IsError {
		return nil
	}
	message := ""
	for _, item := range payload.Content {
		if strings.TrimSpace(item.Text) != "" {
			message = strings.TrimSpace(item.Text)
			break
		}
	}
	if message == "" {
		message = "mcp tool returned isError=true"
	}
	return fmt.Errorf("mcp tool %s failed: %s", spec.Tool, message)
}

func mcpArgumentsJSON(values []orchestratorevents.MCPArgument) (map[string]json.RawMessage, error) {
	if len(values) == 0 {
		return nil, nil
	}
	ret := make(map[string]json.RawMessage, len(values))
	for _, value := range values {
		name := strings.TrimSpace(value.Name)
		if name == "" {
			continue
		}
		var raw []byte
		var err error
		switch {
		case value.String != nil:
			raw, err = json.Marshal(*value.String)
		case value.Integer != nil:
			raw, err = json.Marshal(*value.Integer)
		case value.Boolean != nil:
			raw, err = json.Marshal(*value.Boolean)
		case value.StringList != nil:
			raw, err = json.Marshal(value.StringList)
		default:
			continue
		}
		if err != nil {
			return nil, err
		}
		ret[name] = raw
	}
	return ret, nil
}

func finalizeMCPResult(output RuntimeOperationResult, executeErr error) (RuntimeOperationResult, error) {
	if executeErr != nil && strings.TrimSpace(output.FailureMessage) == "" {
		output.FailureMessage = executeErr.Error()
	}
	if executeErr != nil {
		output.FailureCode = firstMCPNonEmpty(output.Stage, "mcp_operation_failed")
		// A request target is diagnostic context, not an observed failing
		// implementation target. Only an explicit structured target returned by
		// the failing MCP capability may cross this owner boundary as a failure
		// target.
		output.Diagnostics.FailureTargets = observedMCPFailureTargets(output.Response)
	}
	if strings.TrimSpace(output.Summary) == "" {
		output.Summary = firstMCPNonEmpty(output.Status, fmt.Sprintf("mcp operation %s", output.Operation))
	}
	output.Evidence = runtimecontract.RuntimeOperationEvidence{Summary: output.Summary}
	return output, executeErr
}

func observedMCPFailureTargets(payload MCPResponsePayload) []string {
	if len(payload.JSON) == 0 {
		return nil
	}
	var observation struct {
		FailureTargets []string `json:"failureTargets"`
	}
	if err := json.Unmarshal(payload.JSON, &observation); err != nil {
		return nil
	}
	return normalizeMCPTargetRefs(observation.FailureTargets)
}

func normalizeMCPTargetRefs(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	targets := make([]string, 0, min(len(values), maxMCPObservedFailureTargets))
	for _, value := range values {
		value = normalizedCapabilityInputTarget(value)
		if value == "" {
			continue
		}
		if _, found := seen[value]; found {
			continue
		}
		seen[value] = struct{}{}
		targets = append(targets, value)
		if len(targets) == maxMCPObservedFailureTargets {
			break
		}
	}
	return targets
}

func firstMCPNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func parseStdioEndpoint(endpoint string) (string, []string, error) {
	fields := strings.Fields(strings.TrimSpace(endpoint))
	if len(fields) == 0 {
		return "", nil, fmt.Errorf("%w: mcp stdio endpoint command is required", common.ErrInvalidArgument)
	}
	command := fields[0]
	if strings.ContainsAny(command, `/\`) && !filepath.IsAbs(command) {
		if abs, err := filepath.Abs(command); err == nil {
			command = abs
		}
	}
	return command, fields[1:], nil
}

func stdioLaunchConfig(server *mcpmodel.Server, request RuntimeOperationRequest) (string, []string, map[string]string, error) {
	if server == nil {
		return "", nil, nil, common.ErrInvalidArgument
	}
	if strings.TrimSpace(server.Endpoint) != "" {
		command, args, err := parseStdioEndpoint(server.Endpoint)
		return command, args, nil, err
	}
	command := strings.TrimSpace(server.Command)
	if command == "" {
		return "", nil, nil, fmt.Errorf("%w: stdio MCP server requires endpoint or command", common.ErrInvalidArgument)
	}
	return command, expandWorkspaceStringSlice(server.Args, request), cloneStringEnvForWorkspace(server.Env, request), nil
}

func stdioSessionKey(workspaceID common.WorkspaceID, serverID string) string {
	if workspaceID == "" {
		return strings.TrimSpace(serverID)
	}
	return string(workspaceID) + "\x00" + strings.TrimSpace(serverID)
}

func mergeEnv(base []string, extra map[string]string) []string {
	values := make(map[string]string, len(base)+len(extra))
	for _, item := range base {
		key, value, ok := strings.Cut(item, "=")
		if !ok || key == "" {
			continue
		}
		values[key] = value
	}
	for key, value := range extra {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		values[key] = value
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	ret := make([]string, 0, len(keys))
	for _, key := range keys {
		ret = append(ret, key+"="+values[key])
	}
	return ret
}

func cloneStringEnv(input map[string]string) map[string]string {
	if input == nil {
		return nil
	}
	ret := make(map[string]string, len(input))
	for key, value := range input {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		ret[key] = value
	}
	return ret
}

func cloneStringEnvForWorkspace(input map[string]string, request RuntimeOperationRequest) map[string]string {
	if input == nil {
		return nil
	}
	ret := make(map[string]string, len(input))
	for key, value := range input {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		ret[key] = expandWorkspaceString(value, request)
	}
	return ret
}

func expandWorkspaceStringSlice(input []string, request RuntimeOperationRequest) []string {
	if input == nil {
		return nil
	}
	ret := make([]string, 0, len(input))
	for _, value := range input {
		ret = append(ret, expandWorkspaceString(value, request))
	}
	return ret
}

func expandWorkspaceString(value string, request RuntimeOperationRequest) string {
	value = workorchmanagedconfig.ExpandPlaceholders(value, request.WorkspaceInternalDirectory)
	replacements := map[string]string{
		"{workspaceId}":                   string(request.WorkspaceID),
		"{runId}":                         string(request.RunID),
		"{stepId}":                        string(request.StepID),
		"{workspaceDeliverableDirectory}": request.WorkspaceDeliverableDirectory,
		"{workspaceInternalDirectory}":    request.WorkspaceInternalDirectory,
	}
	for old, replacement := range replacements {
		value = strings.ReplaceAll(value, old, replacement)
	}
	return value
}

func writeJSONRPCLine(w io.Writer, payload jsonRPCRequest) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	_, err = w.Write(raw)
	return err
}

func readJSONRPCResponseWithContext(ctx context.Context, reader *bufio.Reader, expectedID int, session *stdioSession) (jsonRPCResponse, []MCPStreamEvent, error) {
	type responseResult struct {
		response jsonRPCResponse
		events   []MCPStreamEvent
		err      error
	}
	done := make(chan responseResult, 1)
	go func() {
		response, events, err := readJSONRPCResponse(reader, expectedID)
		done <- responseResult{response: response, events: events, err: err}
	}()
	select {
	case result := <-done:
		return result.response, result.events, result.err
	case <-ctx.Done():
		if session != nil {
			session.Close()
		}
		return jsonRPCResponse{}, nil, ctx.Err()
	}
}

func readJSONRPCResponse(reader *bufio.Reader, expectedID int) (jsonRPCResponse, []MCPStreamEvent, error) {
	events := []MCPStreamEvent{}
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			return jsonRPCResponse{}, events, err
		}
		if len(line) > 1<<20 {
			return jsonRPCResponse{}, events, fmt.Errorf("%w: mcp stdio response line exceeds 1MiB", common.ErrInvalidArgument)
		}
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		var response jsonRPCResponse
		if err := json.Unmarshal(line, &response); err != nil {
			return jsonRPCResponse{}, events, fmt.Errorf("%w: decode mcp stdio json-rpc response: %v", common.ErrInvalidArgument, err)
		}
		if jsonRPCIDMatches(response.ID, expectedID) {
			if response.Error != nil {
				return response, events, fmt.Errorf("mcp json-rpc error %d: %s", response.Error.Code, response.Error.Message)
			}
			return response, events, nil
		}
		if response.Method != "" {
			events = append(events, MCPStreamEvent{Method: response.Method, Params: append(json.RawMessage(nil), response.Params...)})
		}
	}
}

func jsonRPCIDMatches(raw json.RawMessage, expected int) bool {
	if len(raw) == 0 {
		return false
	}
	var number int
	if err := json.Unmarshal(raw, &number); err == nil {
		return number == expected
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text == fmt.Sprintf("%d", expected)
	}
	var floating float64
	if err := json.Unmarshal(raw, &floating); err == nil {
		return int(floating) == expected
	}
	return false
}

func failedStdioResult(spec operationSpec, stage string, err error, duration time.Duration, stderrText string) (RuntimeOperationResult, error) {
	output := failureOutput(spec, stage, err)
	output.DurationMillis = duration.Milliseconds()
	output.Stderr = stderrText
	return finalizeMCPResult(output, err)
}

func timeoutFromServer(server *mcpmodel.Server) time.Duration {
	timeout := time.Duration(server.TimeoutMillis) * time.Millisecond
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return timeout
}

func errorsIsDeadline(ctx context.Context) bool {
	return ctx != nil && ctx.Err() == context.DeadlineExceeded
}

func nextJSONRPCID() int {
	return int(time.Now().UnixNano() & 0x3fffffff)
}

func completedStage(spec operationSpec) string {
	return stageForOperation(spec, "completed")
}

func invalidStage(spec operationSpec) string {
	return stageForOperation(spec, "invalid")
}

func unavailableStage(spec operationSpec) string {
	return stageForOperation(spec, "unavailable")
}

func serverUnavailableStage(spec operationSpec) string {
	return stageForOperation(spec, "server_unavailable")
}

func rejectedStage(spec operationSpec) string {
	return stageForOperation(spec, "rejected")
}

func unsupportedTransportStage(spec operationSpec) string {
	return stageForOperation(spec, "unsupported_transport")
}

func invalidEndpointStage(spec operationSpec) string {
	return stageForOperation(spec, "invalid_endpoint")
}

func failedStage(spec operationSpec) string {
	return stageForOperation(spec, "failed")
}

func timeoutStage(spec operationSpec) string {
	return stageForOperation(spec, "timeout")
}

func stageForOperation(spec operationSpec, suffix string) string {
	switch spec.Operation {
	case blockcommon.StepKindResourceRead:
		return "mcp_resource_read_" + suffix
	case blockcommon.StepKindPromptGet:
		return "mcp_prompt_get_" + suffix
	default:
		return "mcp_tool_call_" + suffix
	}
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b == nil || b.limit <= 0 {
		return len(p), nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	remaining := b.limit - b.buf.Len()
	if remaining > 0 {
		if len(p) < remaining {
			remaining = len(p)
		}
		_, _ = b.buf.Write(p[:remaining])
	}
	return len(p), nil
}

func (b *limitedBuffer) String() string {
	if b == nil {
		return ""
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return strings.TrimSpace(b.buf.String())
}
