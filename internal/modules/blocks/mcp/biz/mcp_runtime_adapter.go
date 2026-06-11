package mcp

import (
	"bufio"
	"bytes"
	"context"
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
	mcpmodel "github.com/muidea/workorch/internal/modules/kernel/mcp/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchmanagedconfig"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type RuntimeAdapter struct {
	service serverGetter
	client  *http.Client

	sessionMu sync.Mutex
	sessions  map[string]*stdioSession
}

type serverGetter interface {
	GetServer(context.Context, common.MCPServerID) (*mcpmodel.Server, error)
}

const (
	mcpProtocolVersion = "2025-11-25"
	mcpClientName      = "workorch"
	mcpClientVersion   = "mvp"

	sessionPolicyLongLived = "long_lived"
)

type operationSpec struct {
	Operation string
	ServerID  string
	Tool      string
	URI       string
	Prompt    string
	Input     map[string]any
	Arguments map[string]any
	Summary   string
	Stream    bool
}

type jsonRPCResponse struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      json.RawMessage  `json:"id,omitempty"`
	Result  any              `json:"result,omitempty"`
	Error   *jsonRPCError    `json:"error,omitempty"`
	Method  string           `json:"method,omitempty"`
	Params  *json.RawMessage `json:"params,omitempty"`
}

type jsonRPCError struct {
	Code    int    `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
	Data    any    `json:"data,omitempty"`
}

type stdioSession struct {
	serverID  string
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	reader    *bufio.Reader
	stderr    *limitedBuffer
	done      chan error
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
	if client == nil {
		client = http.DefaultClient
	}
	return &RuntimeAdapter{service: service, client: client, sessions: map[string]*stdioSession{}}
}

func (a *RuntimeAdapter) ExecuteRuntimeStep(ctx context.Context, request runtimemodel.StepExecutionRequest) (runtimemodel.StepExecutionResult, error) {
	spec, err := parseOperationSpec(request.Kind, request.Input)
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

func (a *RuntimeAdapter) executeHTTPOperation(ctx context.Context, request runtimemodel.StepExecutionRequest, server *mcpmodel.Server, spec operationSpec) (runtimemodel.StepExecutionResult, error) {
	endpoint := strings.TrimSpace(server.Endpoint)
	if endpoint == "" {
		err := fmt.Errorf("%w: mcp endpoint is required", common.ErrInvalidArgument)
		return failedResult(spec, invalidEndpointStage(spec), err), err
	}
	if _, err := url.ParseRequestURI(endpoint); err != nil {
		err = fmt.Errorf("%w: invalid mcp endpoint: %v", common.ErrInvalidArgument, err)
		return failedResult(spec, invalidEndpointStage(spec), err), err
	}
	timeout := timeoutFromServer(server)
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	payload := map[string]any{
		"workspaceId": request.WorkspaceID,
		"runId":       request.RunID,
		"stepId":      request.StepID,
		"serverId":    spec.ServerID,
		"operation":   spec.Operation,
		"input":       cloneMap(spec.Input),
	}
	switch spec.Operation {
	case blockcommon.StepKindToolCall:
		payload["tool"] = spec.Tool
	case blockcommon.StepKindResourceRead:
		payload["uri"] = spec.URI
	case blockcommon.StepKindPromptGet:
		payload["prompt"] = spec.Prompt
		payload["arguments"] = cloneMap(spec.Arguments)
	}
	if spec.Stream {
		payload["stream"] = true
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return failedResult(spec, invalidStage(spec), err), err
	}
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
		output["durationMillis"] = duration.Milliseconds()
		return runtimemodel.StepExecutionResult{Output: output}, err
	}
	defer resp.Body.Close()

	raw, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if readErr != nil {
		output := failureOutput(spec, failedStage(spec), readErr)
		output["statusCode"] = resp.StatusCode
		output["durationMillis"] = duration.Milliseconds()
		return runtimemodel.StepExecutionResult{Output: output}, readErr
	}
	responsePayload, streamEvents := parseHTTPResponsePayload(raw, resp.Header.Get("Content-Type"), spec.Stream)
	output := successOutput(spec, server, duration, responsePayload)
	output["statusCode"] = resp.StatusCode
	if len(streamEvents) > 0 {
		output["streamEvents"] = streamEvents
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		err := fmt.Errorf("mcp endpoint returned status %d", resp.StatusCode)
		output["status"] = "failed"
		output["stage"] = failedStage(spec)
		output["error"] = err.Error()
		return runtimemodel.StepExecutionResult{Output: output}, err
	}
	if err := toolResultError(spec, responsePayload); err != nil {
		output["status"] = "failed"
		output["stage"] = failedStage(spec)
		output["error"] = err.Error()
		if diagnostics := capabilityFailureDiagnostics(spec); len(diagnostics) > 0 {
			output["runtimeDiagnostics"] = diagnostics
		}
		return runtimemodel.StepExecutionResult{Output: output}, err
	}
	return runtimemodel.StepExecutionResult{Output: output}, nil
}

func (a *RuntimeAdapter) executeStdioOperation(ctx context.Context, request runtimemodel.StepExecutionRequest, server *mcpmodel.Server, spec operationSpec) (runtimemodel.StepExecutionResult, error) {
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
	output := successOutput(spec, server, duration, response.Result)
	if len(streamEvents) > 0 {
		output["streamEvents"] = streamEvents
	}
	if stderrText != "" {
		output["stderr"] = stderrText
	}
	if err := toolResultError(spec, response.Result); err != nil {
		output["status"] = "failed"
		output["stage"] = failedStage(spec)
		output["error"] = err.Error()
		if diagnostics := capabilityFailureDiagnostics(spec); len(diagnostics) > 0 {
			output["runtimeDiagnostics"] = diagnostics
		}
		return runtimemodel.StepExecutionResult{Output: output}, err
	}
	return runtimemodel.StepExecutionResult{Output: output}, nil
}

func (a *RuntimeAdapter) executeLongLivedStdioOperation(ctx context.Context, request runtimemodel.StepExecutionRequest, server *mcpmodel.Server, spec operationSpec) (runtimemodel.StepExecutionResult, error) {
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
	output := successOutput(spec, server, duration, response.Result)
	if len(streamEvents) > 0 {
		output["streamEvents"] = streamEvents
	}
	if stderrText := session.Stderr(); stderrText != "" {
		output["stderr"] = stderrText
	}
	if err := toolResultError(spec, response.Result); err != nil {
		output["status"] = "failed"
		output["stage"] = failedStage(spec)
		output["error"] = err.Error()
		if diagnostics := capabilityFailureDiagnostics(spec); len(diagnostics) > 0 {
			output["runtimeDiagnostics"] = diagnostics
		}
		return runtimemodel.StepExecutionResult{Output: output}, err
	}
	return runtimemodel.StepExecutionResult{Output: output}, nil
}

func (a *RuntimeAdapter) getOrCreateStdioSession(ctx context.Context, request runtimemodel.StepExecutionRequest, server *mcpmodel.Server, serverID string) (*stdioSession, error) {
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
	return session, nil
}

func (a *RuntimeAdapter) evictStdioSession(serverID string, target *stdioSession) {
	a.sessionMu.Lock()
	if a.sessions[serverID] == target {
		delete(a.sessions, serverID)
	}
	a.sessionMu.Unlock()
}

func newInitializedStdioSession(ctx context.Context, request runtimemodel.StepExecutionRequest, server *mcpmodel.Server, serverID string) (*stdioSession, error) {
	command, args, env, err := stdioLaunchConfig(server, request)
	if err != nil {
		return nil, err
	}
	callCtx, cancel := context.WithTimeout(ctx, timeoutFromServer(server))
	defer cancel()

	cmd := exec.Command(command, args...)
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
	}
	go func() {
		_, _ = io.Copy(session.stderr, stderr)
	}()
	go func() {
		session.done <- cmd.Wait()
	}()

	initializeRequest := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": mcpProtocolVersion,
			"capabilities":    map[string]any{},
			"clientInfo": map[string]any{
				"name":    mcpClientName,
				"version": mcpClientVersion,
			},
		},
	}
	if err := writeJSONRPCLine(stdin, initializeRequest); err != nil {
		session.Close()
		return nil, err
	}
	if _, _, err := readJSONRPCResponseWithContext(callCtx, session.reader, 1, session); err != nil {
		session.Close()
		return nil, err
	}
	initializedNotification := map[string]any{
		"jsonrpc": "2.0",
		"method":  "notifications/initialized",
		"params":  map[string]any{},
	}
	if err := writeJSONRPCLine(stdin, initializedNotification); err != nil {
		session.Close()
		return nil, err
	}
	return session, nil
}

func (s *stdioSession) Execute(ctx context.Context, spec operationSpec, id int) (jsonRPCResponse, []any, error) {
	if s == nil {
		return jsonRPCResponse{}, nil, common.ErrInvalidArgument
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	request := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
	}
	switch spec.Operation {
	case blockcommon.StepKindToolCall:
		request["method"] = "tools/call"
		request["params"] = map[string]any{
			"name":      spec.Tool,
			"arguments": cloneMap(spec.Input),
		}
	case blockcommon.StepKindResourceRead:
		request["method"] = "resources/read"
		request["params"] = map[string]any{"uri": spec.URI}
	case blockcommon.StepKindPromptGet:
		request["method"] = "prompts/get"
		request["params"] = map[string]any{
			"name":      spec.Prompt,
			"arguments": cloneMap(spec.Arguments),
		}
	default:
		return jsonRPCResponse{}, nil, common.ErrInvalidArgument
	}
	if err := writeJSONRPCLine(s.stdin, request); err != nil {
		return jsonRPCResponse{}, nil, err
	}
	return readJSONRPCResponseWithContext(ctx, s.reader, id, s)
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

func parseOperationSpec(kind string, input map[string]any) (operationSpec, error) {
	spec := operationSpec{Operation: strings.TrimSpace(kind)}
	if spec.Operation == "" {
		spec.Operation = blockcommon.StepKindToolCall
	}
	if input == nil {
		return spec, common.ErrInvalidArgument
	}
	if value, _ := input["serverId"].(string); strings.TrimSpace(value) != "" {
		spec.ServerID = strings.TrimSpace(value)
	}
	if value, _ := input["tool"].(string); strings.TrimSpace(value) != "" {
		spec.Tool = strings.TrimSpace(value)
	}
	if value, _ := input["uri"].(string); strings.TrimSpace(value) != "" {
		spec.URI = strings.TrimSpace(value)
	}
	if value, _ := input["resourceUri"].(string); strings.TrimSpace(value) != "" {
		spec.URI = strings.TrimSpace(value)
	}
	if value, _ := input["prompt"].(string); strings.TrimSpace(value) != "" {
		spec.Prompt = strings.TrimSpace(value)
	}
	if value, _ := input["name"].(string); spec.Prompt == "" && strings.TrimSpace(value) != "" {
		spec.Prompt = strings.TrimSpace(value)
	}
	if value, ok := input["input"].(map[string]any); ok {
		spec.Input = cloneMap(value)
	}
	if value, ok := input["arguments"].(map[string]any); ok {
		spec.Arguments = cloneMap(value)
	}
	if value, _ := input["summary"].(string); strings.TrimSpace(value) != "" {
		spec.Summary = strings.TrimSpace(value)
	}
	if value, ok := input["stream"].(bool); ok {
		spec.Stream = value
	}
	if spec.ServerID == "" {
		return spec, fmt.Errorf("%w: serverId is required", common.ErrInvalidArgument)
	}
	if spec.Input == nil {
		spec.Input = map[string]any{}
	}
	if spec.Arguments == nil {
		spec.Arguments = map[string]any{}
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

func failedResult(spec operationSpec, stage string, err error) runtimemodel.StepExecutionResult {
	return runtimemodel.StepExecutionResult{Output: failureOutput(spec, stage, err)}
}

func failureOutput(spec operationSpec, stage string, err error) map[string]any {
	output := baseOutput(spec)
	output["status"] = "failed"
	output["stage"] = stage
	if err != nil {
		output["error"] = err.Error()
	}
	if diagnostics := capabilityFailureDiagnostics(spec); len(diagnostics) > 0 {
		output["runtimeDiagnostics"] = diagnostics
	}
	return output
}

func successOutput(spec operationSpec, server *mcpmodel.Server, duration time.Duration, result any) map[string]any {
	output := baseOutput(spec)
	output["status"] = "completed"
	output["stage"] = completedStage(spec)
	output["transport"] = server.Transport
	output["endpoint"] = server.Endpoint
	output["durationMillis"] = duration.Milliseconds()
	output["result"] = result
	return output
}

func baseOutput(spec operationSpec) map[string]any {
	output := map[string]any{
		"operation": spec.Operation,
		"serverId":  spec.ServerID,
	}
	switch spec.Operation {
	case blockcommon.StepKindToolCall:
		output["tool"] = spec.Tool
		output["input"] = cloneMap(spec.Input)
	case blockcommon.StepKindResourceRead:
		output["uri"] = spec.URI
	case blockcommon.StepKindPromptGet:
		output["prompt"] = spec.Prompt
		output["arguments"] = cloneMap(spec.Arguments)
	}
	if spec.Summary != "" {
		output["summary"] = spec.Summary
	}
	return output
}

func capabilityFailureDiagnostics(spec operationSpec) map[string]any {
	diagnostics := map[string]any{
		"operation": spec.Operation,
		"serverId":  spec.ServerID,
	}
	if spec.Tool != "" {
		diagnostics["tool"] = spec.Tool
	}
	if spec.URI != "" {
		diagnostics["uri"] = spec.URI
		diagnostics["targetRef"] = spec.URI
	}
	if spec.Prompt != "" {
		diagnostics["prompt"] = spec.Prompt
	}
	if target := firstCapabilityInputTarget(spec); target != "" {
		diagnostics["targetRef"] = target
		diagnostics["failureTargets"] = []string{target}
	}
	return diagnostics
}

func firstCapabilityInputTarget(spec operationSpec) string {
	for _, key := range []string{"targetRef", "path", "file", "uri", "resourceUri"} {
		if target := normalizedCapabilityInputTarget(spec.Input[key]); target != "" {
			return target
		}
	}
	return ""
}

func normalizedCapabilityInputTarget(value any) string {
	text, ok := value.(string)
	if !ok {
		return ""
	}
	text = strings.TrimSpace(text)
	if text == "" || strings.Contains(text, "://") {
		return text
	}
	text = filepath.ToSlash(text)
	for strings.HasPrefix(text, "./") {
		text = strings.TrimPrefix(text, "./")
	}
	return strings.Trim(text, "/")
}

func parseHTTPResponsePayload(raw []byte, contentType string, requestedStream bool) (any, []any) {
	if !requestedStream && !strings.Contains(strings.ToLower(contentType), "text/event-stream") {
		return parseResponsePayload(raw), nil
	}
	events := parseStreamEvents(raw)
	if len(events) == 0 {
		return parseResponsePayload(raw), nil
	}
	return events[len(events)-1], events
}

func parseStreamEvents(raw []byte) []any {
	lines := bytes.Split(raw, []byte{'\n'})
	events := make([]any, 0, len(lines))
	var sseData []string
	flushSSE := func() {
		if len(sseData) == 0 {
			return
		}
		events = append(events, parseResponsePayload([]byte(strings.Join(sseData, "\n"))))
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
		events = append(events, parseResponsePayload([]byte(trimmed)))
	}
	flushSSE()
	return events
}

func parseResponsePayload(raw []byte) any {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return map[string]any{}
	}
	var payload any
	if err := json.Unmarshal(trimmed, &payload); err == nil {
		return payload
	}
	return string(raw)
}

func toolResultError(spec operationSpec, result any) error {
	if spec.Operation != blockcommon.StepKindToolCall {
		return nil
	}
	payload, ok := result.(map[string]any)
	if !ok {
		return nil
	}
	isError, _ := payload["isError"].(bool)
	if !isError {
		return nil
	}
	message := strings.TrimSpace(firstToolResultText(payload))
	if message == "" {
		message = "mcp tool returned isError=true"
	}
	return fmt.Errorf("mcp tool %s failed: %s", spec.Tool, message)
}

func firstToolResultText(payload map[string]any) string {
	content, ok := payload["content"].([]any)
	if !ok {
		return ""
	}
	for _, item := range content {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if text, _ := entry["text"].(string); strings.TrimSpace(text) != "" {
			return text
		}
	}
	return ""
}

func cloneMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	ret := make(map[string]any, len(input))
	for key, value := range input {
		ret[key] = value
	}
	return ret
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

func stdioLaunchConfig(server *mcpmodel.Server, request runtimemodel.StepExecutionRequest) (string, []string, map[string]string, error) {
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

func cloneStringEnvForWorkspace(input map[string]string, request runtimemodel.StepExecutionRequest) map[string]string {
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

func expandWorkspaceStringSlice(input []string, request runtimemodel.StepExecutionRequest) []string {
	if input == nil {
		return nil
	}
	ret := make([]string, 0, len(input))
	for _, value := range input {
		ret = append(ret, expandWorkspaceString(value, request))
	}
	return ret
}

func expandWorkspaceString(value string, request runtimemodel.StepExecutionRequest) string {
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

func writeJSONRPCLine(w io.Writer, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	_, err = w.Write(raw)
	return err
}

func readJSONRPCResponseWithContext(ctx context.Context, reader *bufio.Reader, expectedID int, session *stdioSession) (jsonRPCResponse, []any, error) {
	type responseResult struct {
		response jsonRPCResponse
		events   []any
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

func readJSONRPCResponse(reader *bufio.Reader, expectedID int) (jsonRPCResponse, []any, error) {
	events := []any{}
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
			events = append(events, jsonRPCNotificationEvent(response))
		}
	}
}

func jsonRPCNotificationEvent(response jsonRPCResponse) any {
	event := map[string]any{"method": response.Method}
	if response.Params != nil && len(*response.Params) > 0 {
		var params any
		if err := json.Unmarshal(*response.Params, &params); err == nil {
			event["params"] = params
		}
	}
	return event
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

func failedStdioResult(spec operationSpec, stage string, err error, duration time.Duration, stderrText string) (runtimemodel.StepExecutionResult, error) {
	output := failureOutput(spec, stage, err)
	output["durationMillis"] = duration.Milliseconds()
	if stderrText != "" {
		output["stderr"] = stderrText
	}
	return runtimemodel.StepExecutionResult{Output: output}, err
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
