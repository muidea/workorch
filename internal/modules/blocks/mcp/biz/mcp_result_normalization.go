package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	blockcommon "github.com/muidea/workorch/internal/modules/blocks/mcp/pkg/common"
	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

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
	_, _, revision, err := a.OperationAdmissibility(ctx)
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
