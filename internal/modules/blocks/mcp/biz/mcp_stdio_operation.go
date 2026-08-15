package mcp

import (
	"context"
	"strings"
	"time"

	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
)

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
