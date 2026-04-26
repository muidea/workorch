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

const (
	outputKeyStatus                = "status"
	outputKeyError                 = "error"
	outputKeyBlockDecisionKind     = "blockDecisionKind"
	outputKeyBlockDecisionQuestion = "blockDecisionQuestion"
	outputKeyBlockDecisionContext  = "blockDecisionContext"
)

type Provider interface {
	Complete(context.Context, llmmodels.ProviderRequest) ([]byte, error)
}

type ProviderRuntime struct {
	Config   llmmodels.ProviderConfig
	Provider Provider
}

type RuntimeAdapter struct {
	defaultProviderID string
	providers         map[string]ProviderRuntime
}

func NewRuntimeAdapter(config llmmodels.ProviderConfig, provider Provider) *RuntimeAdapter {
	providerID := strings.TrimSpace(config.ID)
	if providerID == "" {
		providerID = llmcommon.DefaultProviderID
		config.ID = providerID
	}
	return NewRuntimeAdapterWithProviders(providerID, map[string]ProviderRuntime{
		providerID: {
			Config:   config,
			Provider: provider,
		},
	})
}

func NewRuntimeAdapterWithProviders(defaultProviderID string, providers map[string]ProviderRuntime) *RuntimeAdapter {
	defaultProviderID = strings.TrimSpace(defaultProviderID)
	if defaultProviderID == "" {
		defaultProviderID = llmcommon.DefaultProviderID
	}
	normalized := make(map[string]ProviderRuntime, len(providers))
	for id, runtime := range providers {
		id = strings.TrimSpace(id)
		if id == "" {
			id = strings.TrimSpace(runtime.Config.ID)
		}
		if id == "" {
			continue
		}
		runtime.Config.ID = id
		normalized[id] = runtime
	}
	return &RuntimeAdapter{defaultProviderID: defaultProviderID, providers: normalized}
}

func NewRuntimeAdapterFromConfig(config llmmodels.ProviderConfig) *RuntimeAdapter {
	var provider Provider
	if !config.Unavailable {
		provider = NewHTTPProvider(config)
	}
	return NewRuntimeAdapter(config, provider)
}

func NewRuntimeAdapterFromConfigs(defaultProviderID string, configs map[string]llmmodels.ProviderConfig) *RuntimeAdapter {
	providers := make(map[string]ProviderRuntime, len(configs))
	for id, config := range configs {
		id = strings.TrimSpace(id)
		if id == "" {
			id = strings.TrimSpace(config.ID)
		}
		if id == "" {
			continue
		}
		config.ID = id
		var provider Provider
		if !config.Unavailable {
			provider = NewHTTPProvider(config)
		}
		providers[id] = ProviderRuntime{
			Config:   config,
			Provider: provider,
		}
	}
	return NewRuntimeAdapterWithProviders(defaultProviderID, providers)
}

func (a *RuntimeAdapter) ExecuteRuntimeStep(ctx context.Context, request runtimemodel.StepExecutionRequest) (runtimemodel.StepExecutionResult, error) {
	if a == nil {
		return runtimemodel.StepExecutionResult{}, common.ErrInvalidArgument
	}
	runtime, err := a.selectProviderRuntime(request)
	if err != nil {
		cfg := llmmodels.ProviderConfig{ID: requestedProviderID(request.Input, a.defaultProviderID), Unavailable: true, UnavailableReason: err.Error()}
		output := failureOutput(request, "llm_provider_unavailable", err.Error(), cfg)
		return runtimemodel.StepExecutionResult{Output: output}, err
	}
	if runtime.Config.Unavailable {
		output := failureOutput(request, "llm_provider_unavailable", runtime.Config.UnavailableReason, runtime.Config)
		return runtimemodel.StepExecutionResult{Output: output}, fmt.Errorf("%w: %s", common.ErrInvalidArgument, runtime.Config.UnavailableReason)
	}
	if runtime.Provider == nil {
		output := failureOutput(request, "llm_provider_client_unavailable", "llm provider client is not configured", runtime.Config)
		return runtimemodel.StepExecutionResult{Output: output}, fmt.Errorf("%w: llm provider client is not configured", common.ErrInvalidArgument)
	}

	payload, err := runtime.Provider.Complete(ctx, providerRequest(request, runtime.Config))
	if err != nil {
		output := failureOutput(request, "llm_provider_error", err.Error(), runtime.Config)
		return runtimemodel.StepExecutionResult{Output: output}, err
	}
	decision, err := ParseLLMExecutionDecision(payload)
	if err != nil {
		output := failureOutput(request, "llm_schema_invalid", err.Error(), runtime.Config)
		return runtimemodel.StepExecutionResult{Output: output}, err
	}
	output, err := NormalizeDecisionOutput(request, decision)
	if err != nil {
		failure := failureOutput(request, "llm_decision_invalid", err.Error(), runtime.Config)
		return runtimemodel.StepExecutionResult{Output: failure}, err
	}
	if status, _ := output[outputKeyStatus].(string); status == "failed" {
		reason, _ := output[outputKeyError].(string)
		if strings.TrimSpace(reason) == "" {
			reason = "llm decision reported failure"
		}
		return runtimemodel.StepExecutionResult{Output: output}, fmt.Errorf("%w: %s", common.ErrInvalidArgument, reason)
	}
	return runtimemodel.StepExecutionResult{Output: output}, nil
}

func (a *RuntimeAdapter) selectProviderRuntime(request runtimemodel.StepExecutionRequest) (ProviderRuntime, error) {
	providerID := requestedProviderID(request.Input, a.defaultProviderID)
	if providerID == "" {
		return ProviderRuntime{}, fmt.Errorf("%w: llm provider is not configured", common.ErrInvalidArgument)
	}
	runtime, ok := a.providers[providerID]
	if !ok {
		return ProviderRuntime{}, fmt.Errorf("%w: llm provider %q is not configured", common.ErrInvalidArgument, providerID)
	}
	return runtime, nil
}

func requestedProviderID(input map[string]any, defaultProviderID string) string {
	if providerID := stringValue(input[llmcommon.ExecutionContextLLMProviderID]); providerID != "" {
		return providerID
	}
	if executionContext, ok := input["executionContext"].(map[string]any); ok {
		if providerID := stringValue(executionContext[llmcommon.ExecutionContextLLMProviderID]); providerID != "" {
			return providerID
		}
	}
	return strings.TrimSpace(defaultProviderID)
}

func stringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	default:
		return ""
	}
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
	if decision.CompletionState == "failed" {
		reason := strings.TrimSpace(decision.ThoughtSummary)
		if reason == "" {
			reason = "llm decision reported failure"
		}
		output["status"] = "failed"
		output["stage"] = "llm_failed"
		output["error"] = reason
		return output, nil
	}
	if decision.RequiredDecision != nil {
		output["stage"] = "llm_decision_required"
		output["blockDecisionReason"] = decision.RequiredDecision.Reason
		output[outputKeyBlockDecisionKind] = decision.RequiredDecision.DecisionKind
		output[outputKeyBlockDecisionQuestion] = decision.RequiredDecision.Question
		output[outputKeyBlockDecisionContext] = cloneMap(decision.RequiredDecision.Context)
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
		output["dodAssessment"] = doDAssessmentOutput(decision.DoDAssessment)
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
	for _, write := range decision.MemoryWrites {
		if strings.TrimSpace(write.MemoryType) == "" || strings.TrimSpace(write.Summary) == "" {
			return fmt.Errorf("%w: memoryWrites memoryType and summary are required", common.ErrInvalidArgument)
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

func doDAssessmentOutput(assessment *llmmodels.DoDAssessment) map[string]any {
	if assessment == nil {
		return nil
	}
	items := make([]map[string]any, 0, len(assessment.Items))
	for _, item := range assessment.Items {
		items = append(items, map[string]any{
			"item":         item.Item,
			"state":        item.State,
			"evidenceRefs": append([]string(nil), item.EvidenceRefs...),
		})
	}
	return map[string]any{
		"satisfied": assessment.Satisfied,
		"items":     items,
		"summary":   assessment.Summary,
	}
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
