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
	"os/exec"
	"strings"
	"syscall"
	"time"

	mcpmodel "github.com/muidea/workorch/internal/modules/kernel/mcp/pkg/models"
	mcpservice "github.com/muidea/workorch/internal/modules/kernel/mcp/service"
	runtimemodel "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type RuntimeAdapter struct {
	service mcpservice.Service
	client  *http.Client
}

const (
	mcpProtocolVersion = "2025-11-25"
	mcpClientName      = "workorch"
	mcpClientVersion   = "mvp"
)

type toolCallSpec struct {
	ServerID string
	Tool     string
	Input    map[string]any
	Summary  string
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

type limitedBuffer struct {
	buf   bytes.Buffer
	limit int
}

func NewRuntimeAdapter(service mcpservice.Service) *RuntimeAdapter {
	return NewRuntimeAdapterWithHTTPClient(service, http.DefaultClient)
}

func NewRuntimeAdapterWithHTTPClient(service mcpservice.Service, client *http.Client) *RuntimeAdapter {
	if client == nil {
		client = http.DefaultClient
	}
	return &RuntimeAdapter{service: service, client: client}
}

func (a *RuntimeAdapter) ExecuteRuntimeStep(ctx context.Context, request runtimemodel.StepExecutionRequest) (runtimemodel.StepExecutionResult, error) {
	spec, err := parseToolCallSpec(request.Input)
	if err != nil {
		return failedResult(spec, "mcp_tool_call_invalid", err), err
	}
	if a == nil || a.service == nil {
		err := common.ErrInvalidArgument
		return failedResult(spec, "mcp_tool_call_unavailable", err), err
	}
	server, err := a.service.GetServer(ctx, common.MCPServerID(spec.ServerID))
	if err != nil {
		return failedResult(spec, "mcp_tool_call_server_unavailable", err), err
	}
	if err := validateServerPolicy(server, spec); err != nil {
		return failedResult(spec, "mcp_tool_call_rejected", err), err
	}
	switch strings.ToLower(strings.TrimSpace(server.Transport)) {
	case "http", "https":
		return a.executeHTTPToolCall(ctx, request, server, spec)
	case "stdio":
		return a.executeStdioToolCall(ctx, request, server, spec)
	default:
		err := fmt.Errorf("%w: unsupported mcp transport %q", common.ErrInvalidArgument, server.Transport)
		return failedResult(spec, "mcp_tool_call_unsupported_transport", err), err
	}
}

func (a *RuntimeAdapter) executeHTTPToolCall(ctx context.Context, request runtimemodel.StepExecutionRequest, server *mcpmodel.Server, spec toolCallSpec) (runtimemodel.StepExecutionResult, error) {
	endpoint := strings.TrimSpace(server.Endpoint)
	if endpoint == "" {
		err := fmt.Errorf("%w: mcp endpoint is required", common.ErrInvalidArgument)
		return failedResult(spec, "mcp_tool_call_invalid_endpoint", err), err
	}
	if _, err := url.ParseRequestURI(endpoint); err != nil {
		err = fmt.Errorf("%w: invalid mcp endpoint: %v", common.ErrInvalidArgument, err)
		return failedResult(spec, "mcp_tool_call_invalid_endpoint", err), err
	}
	timeout := time.Duration(server.TimeoutMillis) * time.Millisecond
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	payload := map[string]any{
		"workspaceId": request.WorkspaceID,
		"runId":       request.RunID,
		"stepId":      request.StepID,
		"serverId":    spec.ServerID,
		"tool":        spec.Tool,
		"input":       cloneMap(spec.Input),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return failedResult(spec, "mcp_tool_call_invalid", err), err
	}
	httpReq, err := http.NewRequestWithContext(callCtx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		err = fmt.Errorf("%w: create mcp request: %v", common.ErrInvalidArgument, err)
		return failedResult(spec, "mcp_tool_call_invalid_endpoint", err), err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")

	startedAt := time.Now().UTC()
	resp, err := a.client.Do(httpReq)
	duration := time.Since(startedAt)
	if err != nil {
		stage := "mcp_tool_call_failed"
		if callCtx.Err() == context.DeadlineExceeded {
			stage = "mcp_tool_call_timeout"
			err = context.DeadlineExceeded
		}
		output := failureOutput(spec, stage, err)
		output["durationMillis"] = duration.Milliseconds()
		return runtimemodel.StepExecutionResult{Output: output}, err
	}
	defer resp.Body.Close()

	raw, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if readErr != nil {
		output := failureOutput(spec, "mcp_tool_call_failed", readErr)
		output["statusCode"] = resp.StatusCode
		output["durationMillis"] = duration.Milliseconds()
		return runtimemodel.StepExecutionResult{Output: output}, readErr
	}
	responsePayload := parseResponsePayload(raw)
	output := map[string]any{
		"status":         "completed",
		"stage":          "mcp_tool_call_completed",
		"serverId":       spec.ServerID,
		"tool":           spec.Tool,
		"input":          cloneMap(spec.Input),
		"transport":      server.Transport,
		"endpoint":       server.Endpoint,
		"statusCode":     resp.StatusCode,
		"durationMillis": duration.Milliseconds(),
		"result":         responsePayload,
	}
	if spec.Summary != "" {
		output["summary"] = spec.Summary
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		err := fmt.Errorf("mcp endpoint returned status %d", resp.StatusCode)
		output["status"] = "failed"
		output["stage"] = "mcp_tool_call_failed"
		output["error"] = err.Error()
		return runtimemodel.StepExecutionResult{Output: output}, err
	}
	return runtimemodel.StepExecutionResult{Output: output}, nil
}

func (a *RuntimeAdapter) executeStdioToolCall(ctx context.Context, _ runtimemodel.StepExecutionRequest, server *mcpmodel.Server, spec toolCallSpec) (runtimemodel.StepExecutionResult, error) {
	command, args, err := parseStdioEndpoint(server.Endpoint)
	if err != nil {
		return failedResult(spec, "mcp_tool_call_invalid_endpoint", err), err
	}
	timeout := time.Duration(server.TimeoutMillis) * time.Millisecond
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(callCtx, command, args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return failedResult(spec, "mcp_tool_call_failed", err), err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return failedResult(spec, "mcp_tool_call_failed", err), err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return failedResult(spec, "mcp_tool_call_failed", err), err
	}

	startedAt := time.Now().UTC()
	if err := cmd.Start(); err != nil {
		return failedResult(spec, "mcp_tool_call_failed", err), err
	}
	stderrBuffer := &limitedBuffer{limit: 64 << 10}
	stderrDone := make(chan struct{})
	go func() {
		_, _ = io.Copy(stderrBuffer, stderr)
		close(stderrDone)
	}()
	reader := bufio.NewReader(stdout)
	cleanup := func() string {
		_ = stdin.Close()
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			if cmd.Process != nil {
				_ = cmd.Process.Signal(syscall.SIGTERM)
			}
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				if cmd.Process != nil {
					_ = cmd.Process.Kill()
				}
				<-done
			}
		}
		<-stderrDone
		return stderrBuffer.String()
	}

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
		stderrText := cleanup()
		return failedStdioResult(spec, "mcp_tool_call_failed", err, time.Since(startedAt), stderrText)
	}
	if _, err := readJSONRPCResponse(reader, 1); err != nil {
		stderrText := cleanup()
		stage := "mcp_tool_call_failed"
		if callCtx.Err() == context.DeadlineExceeded {
			stage = "mcp_tool_call_timeout"
			err = context.DeadlineExceeded
		}
		return failedStdioResult(spec, stage, err, time.Since(startedAt), stderrText)
	}
	initializedNotification := map[string]any{
		"jsonrpc": "2.0",
		"method":  "notifications/initialized",
		"params":  map[string]any{},
	}
	if err := writeJSONRPCLine(stdin, initializedNotification); err != nil {
		stderrText := cleanup()
		return failedStdioResult(spec, "mcp_tool_call_failed", err, time.Since(startedAt), stderrText)
	}

	toolCallRequest := map[string]any{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "tools/call",
		"params": map[string]any{
			"name":      spec.Tool,
			"arguments": cloneMap(spec.Input),
		},
	}
	if err := writeJSONRPCLine(stdin, toolCallRequest); err != nil {
		stderrText := cleanup()
		return failedStdioResult(spec, "mcp_tool_call_failed", err, time.Since(startedAt), stderrText)
	}
	response, err := readJSONRPCResponse(reader, 2)
	duration := time.Since(startedAt)
	stderrText := cleanup()
	if err != nil {
		stage := "mcp_tool_call_failed"
		if callCtx.Err() == context.DeadlineExceeded {
			stage = "mcp_tool_call_timeout"
			err = context.DeadlineExceeded
		}
		return failedStdioResult(spec, stage, err, duration, stderrText)
	}
	output := map[string]any{
		"status":         "completed",
		"stage":          "mcp_tool_call_completed",
		"serverId":       spec.ServerID,
		"tool":           spec.Tool,
		"input":          cloneMap(spec.Input),
		"transport":      server.Transport,
		"endpoint":       server.Endpoint,
		"durationMillis": duration.Milliseconds(),
		"result":         response.Result,
	}
	if spec.Summary != "" {
		output["summary"] = spec.Summary
	}
	if stderrText != "" {
		output["stderr"] = stderrText
	}
	return runtimemodel.StepExecutionResult{Output: output}, nil
}

func parseToolCallSpec(input map[string]any) (toolCallSpec, error) {
	spec := toolCallSpec{}
	if input == nil {
		return spec, common.ErrInvalidArgument
	}
	if value, _ := input["serverId"].(string); strings.TrimSpace(value) != "" {
		spec.ServerID = strings.TrimSpace(value)
	}
	if value, _ := input["tool"].(string); strings.TrimSpace(value) != "" {
		spec.Tool = strings.TrimSpace(value)
	}
	if value, ok := input["input"].(map[string]any); ok {
		spec.Input = cloneMap(value)
	}
	if value, _ := input["summary"].(string); strings.TrimSpace(value) != "" {
		spec.Summary = strings.TrimSpace(value)
	}
	if spec.ServerID == "" || spec.Tool == "" {
		return spec, fmt.Errorf("%w: serverId and tool are required", common.ErrInvalidArgument)
	}
	if spec.Input == nil {
		spec.Input = map[string]any{}
	}
	return spec, nil
}

func validateServerPolicy(server *mcpmodel.Server, spec toolCallSpec) error {
	if server == nil || server.ID == "" {
		return common.ErrNotFound
	}
	if !server.Enabled {
		return fmt.Errorf("%w: mcp server %s is disabled", common.ErrInvalidArgument, spec.ServerID)
	}
	for _, allowed := range server.AllowedTools {
		if strings.TrimSpace(allowed) == spec.Tool {
			return nil
		}
	}
	return fmt.Errorf("%w: tool %s is not allowed for mcp server %s", common.ErrInvalidArgument, spec.Tool, spec.ServerID)
}

func failedResult(spec toolCallSpec, stage string, err error) runtimemodel.StepExecutionResult {
	return runtimemodel.StepExecutionResult{Output: failureOutput(spec, stage, err)}
}

func failureOutput(spec toolCallSpec, stage string, err error) map[string]any {
	output := map[string]any{
		"status":   "failed",
		"stage":    stage,
		"serverId": spec.ServerID,
		"tool":     spec.Tool,
		"input":    cloneMap(spec.Input),
	}
	if err != nil {
		output["error"] = err.Error()
	}
	if spec.Summary != "" {
		output["summary"] = spec.Summary
	}
	return output
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
	return fields[0], fields[1:], nil
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

func readJSONRPCResponse(reader *bufio.Reader, expectedID int) (jsonRPCResponse, error) {
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			return jsonRPCResponse{}, err
		}
		if len(line) > 1<<20 {
			return jsonRPCResponse{}, fmt.Errorf("%w: mcp stdio response line exceeds 1MiB", common.ErrInvalidArgument)
		}
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		var response jsonRPCResponse
		if err := json.Unmarshal(line, &response); err != nil {
			return jsonRPCResponse{}, fmt.Errorf("%w: decode mcp stdio json-rpc response: %v", common.ErrInvalidArgument, err)
		}
		if !jsonRPCIDMatches(response.ID, expectedID) {
			continue
		}
		if response.Error != nil {
			return response, fmt.Errorf("mcp json-rpc error %d: %s", response.Error.Code, response.Error.Message)
		}
		return response, nil
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

func failedStdioResult(spec toolCallSpec, stage string, err error, duration time.Duration, stderrText string) (runtimemodel.StepExecutionResult, error) {
	output := failureOutput(spec, stage, err)
	output["durationMillis"] = duration.Milliseconds()
	if stderrText != "" {
		output["stderr"] = stderrText
	}
	return runtimemodel.StepExecutionResult{Output: output}, err
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b == nil || b.limit <= 0 {
		return len(p), nil
	}
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
	return strings.TrimSpace(b.buf.String())
}
