package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	runtimemodel "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

const decisionSchemaName = "workorch.llmExecutionDecision.v1"

type Provider interface {
	Complete(context.Context, llmmodels.ProviderRequest) ([]byte, error)
}

type RuntimeAdapter struct {
	config   llmmodels.ProviderConfig
	provider Provider
}

func NewRuntimeAdapter(config llmmodels.ProviderConfig, provider Provider) *RuntimeAdapter {
	return &RuntimeAdapter{config: config, provider: provider}
}

func NewRuntimeAdapterFromEnv() (*RuntimeAdapter, error) {
	config, err := LoadProviderConfigFromEnv()
	if err != nil {
		return nil, err
	}
	return NewRuntimeAdapter(config, nil), nil
}

func (a *RuntimeAdapter) ExecuteRuntimeStep(ctx context.Context, request runtimemodel.StepExecutionRequest) (runtimemodel.StepExecutionResult, error) {
	if a == nil {
		return runtimemodel.StepExecutionResult{}, common.ErrInvalidArgument
	}
	if a.config.Unavailable {
		output := failureOutput(request, "llm_provider_unavailable", a.config.UnavailableReason, a.config)
		return runtimemodel.StepExecutionResult{Output: output}, fmt.Errorf("%w: %s", common.ErrInvalidArgument, a.config.UnavailableReason)
	}
	if a.provider == nil {
		output := failureOutput(request, "llm_provider_client_unavailable", "llm provider client is not configured", a.config)
		return runtimemodel.StepExecutionResult{Output: output}, fmt.Errorf("%w: llm provider client is not configured", common.ErrInvalidArgument)
	}

	payload, err := a.provider.Complete(ctx, providerRequest(request, a.config))
	if err != nil {
		output := failureOutput(request, "llm_provider_error", err.Error(), a.config)
		return runtimemodel.StepExecutionResult{Output: output}, err
	}
	decision, err := ParseLLMExecutionDecision(payload)
	if err != nil {
		output := failureOutput(request, "llm_schema_invalid", err.Error(), a.config)
		return runtimemodel.StepExecutionResult{Output: output}, err
	}
	output, err := NormalizeDecisionOutput(request, decision)
	if err != nil {
		failure := failureOutput(request, "llm_decision_invalid", err.Error(), a.config)
		return runtimemodel.StepExecutionResult{Output: failure}, err
	}
	return runtimemodel.StepExecutionResult{Output: output}, nil
}

func ParseLLMExecutionDecision(payload []byte) (llmmodels.LLMExecutionDecision, error) {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var decision llmmodels.LLMExecutionDecision
	if err := decoder.Decode(&decision); err != nil {
		return llmmodels.LLMExecutionDecision{}, fmt.Errorf("%w: invalid llm decision json: %v", common.ErrInvalidArgument, err)
	}
	var extra any
	if err := decoder.Decode(&extra); err == nil {
		return llmmodels.LLMExecutionDecision{}, fmt.Errorf("%w: invalid llm decision json: multiple json values", common.ErrInvalidArgument)
	}
	if err := validateDecision(decision); err != nil {
		return llmmodels.LLMExecutionDecision{}, err
	}
	return decision, nil
}

func NormalizeDecisionOutput(request runtimemodel.StepExecutionRequest, decision llmmodels.LLMExecutionDecision) (map[string]any, error) {
	if err := validateDecision(decision); err != nil {
		return nil, err
	}
	output := map[string]any{
		"runId":       string(request.RunID),
		"stepId":      string(request.StepID),
		"status":      "completed",
		"stage":       "llm_plan",
		"llmDecision": decisionMap(decision),
	}
	if decision.ThoughtSummary != "" {
		output["summary"] = decision.ThoughtSummary
	}
	if len(decision.PatchRequests) > 0 {
		output["patches"] = patchOutput(decision.PatchRequests)
	}
	if len(decision.MemoryWrites) > 0 {
		output["memoryWrites"] = memoryWriteOutput(decision.MemoryWrites)
	}
	if decision.RequiredDecision != nil {
		output["stage"] = "llm_decision_required"
		output["blockDecisionReason"] = decision.RequiredDecision.Reason
		output["nextStep"] = fmt.Sprintf("%s/llm-after-decision", request.RunID)
		output["nextKind"] = llmcommon.StepKindWorkspaceExecution
		return output, nil
	}
	if decision.RequiredApproval != nil {
		output["stage"] = "llm_approval_required"
		output["blockApprovalReason"] = decision.RequiredApproval.Reason
		output["nextStep"] = fmt.Sprintf("%s/llm-after-approval", request.RunID)
		output["nextKind"] = llmcommon.StepKindWorkspaceExecution
		return output, nil
	}
	if len(decision.ShellCommands) > 0 {
		command := decision.ShellCommands[0]
		output["stage"] = "llm_shell_plan"
		output["nextStep"] = fmt.Sprintf("%s/shell-1", request.RunID)
		output["nextKind"] = "shell/command"
		output["nextInput"] = map[string]any{
			"command": command.Command,
			"args":    append([]string(nil), command.Args...),
		}
		return output, nil
	}
	if len(decision.MCPToolCalls) > 0 {
		call := decision.MCPToolCalls[0]
		output["stage"] = "llm_mcp_plan"
		output["nextStep"] = fmt.Sprintf("%s/mcp-1", request.RunID)
		output["nextKind"] = "mcp/tool_call"
		output["nextInput"] = map[string]any{
			"serverId": call.ServerID,
			"tool":     call.Tool,
			"input":    cloneMap(call.Input),
		}
		return output, nil
	}
	if decision.CompletionState == "completed" {
		output["stage"] = "llm_dod_assessment"
		return output, nil
	}
	return output, nil
}

func validateDecision(decision llmmodels.LLMExecutionDecision) error {
	switch decision.CompletionState {
	case "continue", "blocked_on_decision", "blocked_on_approval", "completed", "failed":
	default:
		return fmt.Errorf("%w: unsupported llm completionState", common.ErrInvalidArgument)
	}
	if decision.CompletionState == "blocked_on_decision" && (decision.RequiredDecision == nil || strings.TrimSpace(decision.RequiredDecision.Reason) == "") {
		return fmt.Errorf("%w: requiredDecision.reason is required", common.ErrInvalidArgument)
	}
	if decision.CompletionState == "blocked_on_approval" && (decision.RequiredApproval == nil || strings.TrimSpace(decision.RequiredApproval.Reason) == "") {
		return fmt.Errorf("%w: requiredApproval.reason is required", common.ErrInvalidArgument)
	}
	for _, command := range decision.ShellCommands {
		if strings.TrimSpace(command.Command) == "" {
			return fmt.Errorf("%w: shellCommands.command is required", common.ErrInvalidArgument)
		}
	}
	for _, patch := range decision.PatchRequests {
		if strings.TrimSpace(patch.TargetRef) == "" {
			return fmt.Errorf("%w: patchRequests.targetRef is required", common.ErrInvalidArgument)
		}
	}
	for _, call := range decision.MCPToolCalls {
		if strings.TrimSpace(call.ServerID) == "" || strings.TrimSpace(call.Tool) == "" {
			return fmt.Errorf("%w: mcpToolCalls serverId and tool are required", common.ErrInvalidArgument)
		}
	}
	if decision.CompletionState == "completed" {
		if decision.DoDAssessment == nil {
			return fmt.Errorf("%w: dodAssessment is required for completed decision", common.ErrInvalidArgument)
		}
		for _, item := range decision.DoDAssessment.Items {
			if item.State == "satisfied" && len(item.EvidenceRefs) == 0 {
				return fmt.Errorf("%w: satisfied dodAssessment item requires evidenceRefs", common.ErrInvalidArgument)
			}
		}
	}
	return nil
}

func providerRequest(request runtimemodel.StepExecutionRequest, config llmmodels.ProviderConfig) llmmodels.ProviderRequest {
	return llmmodels.ProviderRequest{
		WorkspaceID: string(request.WorkspaceID),
		RunID:       string(request.RunID),
		StepID:      string(request.StepID),
		Kind:        request.Kind,
		Model:       config.Model,
		Input:       cloneMap(request.Input),
		Schema:      decisionSchemaName,
		TraceRefs: map[string]string{
			"workspaceId": string(request.WorkspaceID),
			"runId":       string(request.RunID),
			"stepId":      string(request.StepID),
		},
		Config: RedactedProviderConfig(config),
	}
}

func failureOutput(request runtimemodel.StepExecutionRequest, stage string, reason string, config llmmodels.ProviderConfig) map[string]any {
	return map[string]any{
		"runId":    string(request.RunID),
		"stepId":   string(request.StepID),
		"status":   "failed",
		"stage":    stage,
		"error":    reason,
		"provider": RedactedProviderConfig(config),
	}
}

func patchOutput(requests []llmmodels.PatchRequest) []map[string]any {
	ret := make([]map[string]any, 0, len(requests))
	for _, request := range requests {
		ret = append(ret, map[string]any{
			"patchType": request.PatchType,
			"targetRef": request.TargetRef,
			"content":   request.Content,
			"diff":      request.Diff,
			"summary":   request.Summary,
		})
	}
	return ret
}

func memoryWriteOutput(requests []llmmodels.MemoryWriteRequest) []map[string]any {
	ret := make([]map[string]any, 0, len(requests))
	for _, request := range requests {
		ret = append(ret, map[string]any{
			"memoryType": request.MemoryType,
			"summary":    request.Summary,
			"content":    request.Content,
			"tags":       append([]string(nil), request.Tags...),
		})
	}
	return ret
}

func decisionMap(decision llmmodels.LLMExecutionDecision) map[string]any {
	data, err := json.Marshal(decision)
	if err != nil {
		return map[string]any{"completionState": decision.CompletionState}
	}
	var ret map[string]any
	if err := json.Unmarshal(data, &ret); err != nil {
		return map[string]any{"completionState": decision.CompletionState}
	}
	return ret
}

func cloneMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}
