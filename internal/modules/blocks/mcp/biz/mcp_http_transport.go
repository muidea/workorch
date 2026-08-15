package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

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
