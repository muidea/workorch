package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"unicode"

	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	mcpcommon "github.com/muidea/workorch/internal/modules/blocks/mcp/pkg/common"
	shellcommon "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/common"
	sshcommon "github.com/muidea/workorch/internal/modules/blocks/ssh/pkg/common"
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
	outputKeyExecutionGraph        = "executionGraph"
	maxLLMOutputPreviewBytes       = 4096
	unavailableWorkorchCLICommand  = "workorch"
	localServiceHTTPDeclaredOnly   = "declared_service_apis_only"
)

var unavailableIfMissingShellCommands = []string{"sshpass", "expect"}

var (
	httpURLPattern              = regexp.MustCompile(`https?://[^\s'"<>\\]+`)
	bareLocalServicePathPattern = regexp.MustCompile(`(?:^|[\s'"(=])((?:/api/v1/workorch/[A-Za-z0-9._~:/?#\[\]@!$&()*+,;=%-]+)|(?:/(?:runs|observability|metrics)(?:[A-Za-z0-9._~:/?#\[\]@!$&()*+,;=%-]*)?))`)
)

type Provider interface {
	Complete(context.Context, llmmodels.ProviderRequest) ([]byte, error)
}

type StreamingProvider interface {
	StreamComplete(context.Context, llmmodels.ProviderRequest, func(llmmodels.ProviderStreamEvent) error) ([]byte, error)
}

type EmbeddingProvider interface {
	Embed(context.Context, llmmodels.ProviderEmbeddingRequest) (llmmodels.ProviderEmbeddingResult, error)
}

type ProviderRuntime struct {
	Config   llmmodels.ProviderConfig
	Provider Provider
}

type ProviderConfigLoader func() (string, map[string]llmmodels.ProviderConfig, error)

type RuntimeAdapter struct {
	defaultProviderID string
	providers         map[string]ProviderRuntime
	configLoader      ProviderConfigLoader
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
		runtime.Config.Capabilities = effectiveProviderCapabilities(runtime.Config.Capabilities)
		normalized[id] = runtime
	}
	return &RuntimeAdapter{defaultProviderID: defaultProviderID, providers: normalized}
}

func NewRuntimeAdapterFromConfig(config llmmodels.ProviderConfig) *RuntimeAdapter {
	var provider Provider
	if !config.Unavailable {
		provider = NewProviderFromConfig(config)
	}
	return NewRuntimeAdapter(config, provider)
}

func NewRuntimeAdapterFromConfigs(defaultProviderID string, configs map[string]llmmodels.ProviderConfig) *RuntimeAdapter {
	return NewRuntimeAdapterWithProviders(defaultProviderID, providerRuntimesFromConfigs(configs))
}

func NewRuntimeAdapterFromConfigLoader(loader ProviderConfigLoader) *RuntimeAdapter {
	return &RuntimeAdapter{
		defaultProviderID: llmcommon.DefaultProviderID,
		configLoader:      loader,
	}
}

func providerRuntimesFromConfigs(configs map[string]llmmodels.ProviderConfig) map[string]ProviderRuntime {
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
			provider = NewProviderFromConfig(config)
		}
		config.Capabilities = effectiveProviderCapabilities(config.Capabilities)
		providers[id] = ProviderRuntime{
			Config:   config,
			Provider: provider,
		}
	}
	return providers
}

func NewProviderFromConfig(config llmmodels.ProviderConfig) Provider {
	switch strings.TrimSpace(config.Provider) {
	case llmcommon.ProviderOpenAI:
		return NewOpenAIProvider(config)
	case llmcommon.ProviderAnthropic:
		return NewAnthropicProvider(config)
	default:
		return NewHTTPProvider(config)
	}
}

func (a *RuntimeAdapter) ExecuteRuntimeStep(ctx context.Context, request runtimemodel.StepExecutionRequest) (runtimemodel.StepExecutionResult, error) {
	if a == nil {
		return runtimemodel.StepExecutionResult{}, common.ErrInvalidArgument
	}
	runtime, providerID, err := a.selectProviderRuntime(request)
	if err != nil {
		cfg := llmmodels.ProviderConfig{ID: providerID, Unavailable: true, UnavailableReason: err.Error()}
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
	operation := requestedProviderOperation(request.Input)
	if err := validateProviderOperation(operation, runtime.Config.Capabilities); err != nil {
		output := failureOutput(request, "llm_provider_operation_unsupported", err.Error(), runtime.Config)
		output["operation"] = operation
		return runtimemodel.StepExecutionResult{Output: output}, err
	}

	providerReq := providerRequest(request, runtime.Config)
	payload, streamEvents, err := completeProvider(ctx, runtime, providerReq)
	if err != nil {
		stage := "llm_provider_error"
		if len(streamEvents) > 0 {
			stage = "llm_provider_stream_error"
		}
		output := failureOutput(request, stage, err.Error(), runtime.Config)
		attachStreamEvents(output, streamEvents)
		return runtimemodel.StepExecutionResult{Output: output}, err
	}
	decision, err := ParseLLMExecutionDecision(payload)
	var repairMetadata map[string]any
	if err != nil {
		parseErr := err
		repairedDecision, repairPayload, repairErr := repairLLMExecutionDecision(ctx, runtime.Provider, providerReq, payload, err)
		if repairErr != nil {
			output := failureOutput(request, "llm_schema_invalid", parseErr.Error(), runtime.Config)
			output["repairAttempted"] = true
			output["parseError"] = parseErr.Error()
			output["repairError"] = repairErr.Error()
			output["rawContentPreview"] = previewLLMOutput(payload)
			if len(repairPayload) > 0 {
				output["repairRawContentPreview"] = previewLLMOutput(repairPayload)
			}
			attachStreamEvents(output, streamEvents)
			return runtimemodel.StepExecutionResult{Output: output}, parseErr
		}
		decision = repairedDecision
		repairMetadata = map[string]any{
			"attempted":  true,
			"attempts":   1,
			"parseError": parseErr.Error(),
		}
	}
	output, err := NormalizeDecisionOutput(request, decision)
	if err != nil {
		failure := failureOutput(request, "llm_decision_invalid", err.Error(), runtime.Config)
		attachStreamEvents(failure, streamEvents)
		return runtimemodel.StepExecutionResult{Output: failure}, err
	}
	attachStreamEvents(output, streamEvents)
	if repairMetadata != nil {
		output["llmRepair"] = repairMetadata
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

func completeProvider(ctx context.Context, runtime ProviderRuntime, request llmmodels.ProviderRequest) ([]byte, []llmmodels.ProviderStreamEvent, error) {
	if runtime.Config.Capabilities.Streaming {
		streaming, ok := runtime.Provider.(StreamingProvider)
		if !ok {
			return nil, nil, fmt.Errorf("%w: llm provider does not implement streaming", common.ErrInvalidArgument)
		}
		events := []llmmodels.ProviderStreamEvent{}
		payload, err := streaming.StreamComplete(ctx, request, func(event llmmodels.ProviderStreamEvent) error {
			if strings.TrimSpace(event.Type) == "" {
				event.Type = "delta"
			}
			events = append(events, event)
			return nil
		})
		return payload, events, err
	}
	payload, err := runtime.Provider.Complete(ctx, request)
	return payload, nil, err
}

func (a *RuntimeAdapter) GenerateEmbedding(ctx context.Context, providerID string, input string) (llmmodels.ProviderEmbeddingResult, error) {
	if a == nil {
		return llmmodels.ProviderEmbeddingResult{}, common.ErrInvalidArgument
	}
	runtime, selectedProviderID, err := a.selectProviderRuntimeByID(providerID)
	if err != nil {
		return llmmodels.ProviderEmbeddingResult{}, err
	}
	if runtime.Config.Unavailable {
		return llmmodels.ProviderEmbeddingResult{}, fmt.Errorf("%w: %s", common.ErrInvalidArgument, runtime.Config.UnavailableReason)
	}
	if err := validateNonChatProviderOperation(llmcommon.ProviderOperationEmbeddings, runtime.Config.Capabilities); err != nil {
		return llmmodels.ProviderEmbeddingResult{}, err
	}
	provider, ok := runtime.Provider.(EmbeddingProvider)
	if !ok {
		return llmmodels.ProviderEmbeddingResult{}, fmt.Errorf("%w: llm provider does not implement embeddings", common.ErrInvalidArgument)
	}
	result, err := provider.Embed(ctx, llmmodels.ProviderEmbeddingRequest{Input: input, Model: runtime.Config.Model})
	if err != nil {
		return llmmodels.ProviderEmbeddingResult{}, err
	}
	result.ProviderID = selectedProviderID
	if strings.TrimSpace(result.Model) == "" {
		result.Model = runtime.Config.Model
	}
	return result, nil
}

func attachStreamEvents(output map[string]any, events []llmmodels.ProviderStreamEvent) {
	if len(events) == 0 || output == nil {
		return
	}
	streamEvents := make([]map[string]any, 0, len(events))
	for _, event := range events {
		item := map[string]any{"type": event.Type}
		if event.Delta != "" {
			item["delta"] = event.Delta
		}
		if len(event.Payload) > 0 {
			item["payload"] = cloneMap(event.Payload)
		}
		streamEvents = append(streamEvents, item)
	}
	output["streamEvents"] = streamEvents
}

func validateNonChatProviderOperation(operation string, capabilities llmmodels.ProviderCapabilities) error {
	operation = strings.TrimSpace(operation)
	if operation == "" {
		return common.ErrInvalidArgument
	}
	capabilities = effectiveProviderCapabilities(capabilities)
	for _, supported := range capabilities.NonChatOperations {
		if operation == supported {
			return nil
		}
	}
	return fmt.Errorf("%w: llm provider operation %q is not declared by provider capabilities", common.ErrInvalidArgument, operation)
}

func validateProviderOperation(operation string, capabilities llmmodels.ProviderCapabilities) error {
	capabilities = effectiveProviderCapabilities(capabilities)
	switch operation {
	case "", llmcommon.ProviderOperationChatCompletions:
		if capabilities.ChatCompletions {
			return nil
		}
	default:
		for _, supported := range capabilities.NonChatOperations {
			if operation == supported {
				return fmt.Errorf("%w: llm provider operation %q is capability-declared but not supported by workspace execution steps", common.ErrInvalidArgument, operation)
			}
		}
	}
	return fmt.Errorf("%w: llm provider operation %q is not declared by provider capabilities", common.ErrInvalidArgument, operation)
}

func repairLLMExecutionDecision(
	ctx context.Context,
	provider Provider,
	request llmmodels.ProviderRequest,
	originalPayload []byte,
	parseErr error,
) (llmmodels.LLMExecutionDecision, []byte, error) {
	if provider == nil || parseErr == nil {
		return llmmodels.LLMExecutionDecision{}, nil, common.ErrInvalidArgument
	}
	repairReq := request
	repairReq.Repair = &llmmodels.ProviderRepairRequest{
		Attempt:        1,
		OriginalOutput: previewLLMOutput(originalPayload),
		ParseError:     parseErr.Error(),
	}
	payload, err := provider.Complete(ctx, repairReq)
	if err != nil {
		return llmmodels.LLMExecutionDecision{}, payload, err
	}
	decision, err := ParseLLMExecutionDecision(payload)
	if err != nil {
		return llmmodels.LLMExecutionDecision{}, payload, err
	}
	return decision, payload, nil
}

func (a *RuntimeAdapter) selectProviderRuntime(request runtimemodel.StepExecutionRequest) (ProviderRuntime, string, error) {
	defaultProviderID := a.defaultProviderID
	providers := a.providers
	if a.configLoader != nil {
		loadedDefaultProviderID, configs, err := a.configLoader()
		if err != nil {
			providerID := requestedProviderID(request.Input, defaultProviderID)
			return ProviderRuntime{}, providerID, err
		}
		defaultProviderID = strings.TrimSpace(loadedDefaultProviderID)
		if defaultProviderID == "" {
			defaultProviderID = llmcommon.DefaultProviderID
		}
		providers = providerRuntimesFromConfigs(configs)
	}
	providerID := requestedProviderID(request.Input, defaultProviderID)
	if providerID == "" {
		return ProviderRuntime{}, providerID, fmt.Errorf("%w: llm provider is not configured", common.ErrInvalidArgument)
	}
	runtime, ok := providers[providerID]
	if !ok {
		return ProviderRuntime{}, providerID, fmt.Errorf("%w: llm provider %q is not configured", common.ErrInvalidArgument, providerID)
	}
	return runtime, providerID, nil
}

func (a *RuntimeAdapter) selectProviderRuntimeByID(providerID string) (ProviderRuntime, string, error) {
	defaultProviderID := a.defaultProviderID
	providers := a.providers
	if a.configLoader != nil {
		loadedDefaultProviderID, configs, err := a.configLoader()
		if err != nil {
			return ProviderRuntime{}, strings.TrimSpace(providerID), err
		}
		defaultProviderID = strings.TrimSpace(loadedDefaultProviderID)
		if defaultProviderID == "" {
			defaultProviderID = llmcommon.DefaultProviderID
		}
		providers = providerRuntimesFromConfigs(configs)
	}
	selectedProviderID := strings.TrimSpace(providerID)
	if selectedProviderID == "" {
		selectedProviderID = defaultProviderID
	}
	if selectedProviderID == "" {
		return ProviderRuntime{}, selectedProviderID, fmt.Errorf("%w: llm provider is not configured", common.ErrInvalidArgument)
	}
	runtime, ok := providers[selectedProviderID]
	if !ok {
		return ProviderRuntime{}, selectedProviderID, fmt.Errorf("%w: llm provider %q is not configured", common.ErrInvalidArgument, selectedProviderID)
	}
	return runtime, selectedProviderID, nil
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

func requestedProviderOperation(input map[string]any) string {
	if operation := stringValue(input["llmOperation"]); operation != "" {
		return operation
	}
	if executionContext, ok := input["executionContext"].(map[string]any); ok {
		if operation := stringValue(executionContext["llmOperation"]); operation != "" {
			return operation
		}
	}
	return llmcommon.ProviderOperationChatCompletions
}

func effectiveProviderCapabilities(capabilities llmmodels.ProviderCapabilities) llmmodels.ProviderCapabilities {
	if !capabilities.ChatCompletions && !capabilities.Streaming && len(capabilities.NonChatOperations) == 0 {
		capabilities.ChatCompletions = true
	}
	return capabilities
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
	normalized, err := NormalizeLLMDecisionPayload(payload)
	if err != nil {
		return llmmodels.LLMExecutionDecision{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(normalized))
	var raw map[string]json.RawMessage
	if err := decoder.Decode(&raw); err != nil {
		return llmmodels.LLMExecutionDecision{}, fmt.Errorf("%w: invalid llm decision json: %v", common.ErrInvalidArgument, err)
	}
	var extra any
	if err := decoder.Decode(&extra); err == nil {
		return llmmodels.LLMExecutionDecision{}, fmt.Errorf("%w: invalid llm decision json: multiple json values", common.ErrInvalidArgument)
	} else if err != io.EOF {
		return llmmodels.LLMExecutionDecision{}, fmt.Errorf("%w: invalid llm decision json: %v", common.ErrInvalidArgument, err)
	}
	if err := validateLLMDecisionTopLevelFields(raw); err != nil {
		return llmmodels.LLMExecutionDecision{}, err
	}
	var decision llmmodels.LLMExecutionDecision
	if err := json.Unmarshal(normalized, &decision); err != nil {
		return llmmodels.LLMExecutionDecision{}, fmt.Errorf("%w: invalid llm decision json: %v", common.ErrInvalidArgument, err)
	}
	if err := validateDecision(decision); err != nil {
		return llmmodels.LLMExecutionDecision{}, err
	}
	return decision, nil
}

func validateLLMDecisionTopLevelFields(raw map[string]json.RawMessage) error {
	for key := range raw {
		switch key {
		case "thoughtSummary",
			"plan",
			"executionGraph",
			"nextAction",
			"requiredDecision",
			"requiredApproval",
			"patchRequests",
			"mcpToolCalls",
			"shellCommands",
			"memoryWrites",
			"dodAssessment",
			"completionState":
			continue
		default:
			return fmt.Errorf("%w: invalid llm decision json: json: unknown field %q", common.ErrInvalidArgument, key)
		}
	}
	return nil
}

func NormalizeLLMDecisionPayload(payload []byte) ([]byte, error) {
	text := strings.TrimSpace(strings.TrimPrefix(string(payload), "\ufeff"))
	if text == "" {
		return nil, fmt.Errorf("%w: invalid llm decision json: empty payload", common.ErrInvalidArgument)
	}
	if strings.HasPrefix(text, "```") {
		text = stripMarkdownFence(text)
	}
	jsonText, ok := extractFirstJSONObject(text)
	if !ok {
		return nil, fmt.Errorf("%w: invalid llm decision json: no json object found", common.ErrInvalidArgument)
	}
	return []byte(jsonText), nil
}

func stripMarkdownFence(text string) string {
	lines := strings.Split(text, "\n")
	if len(lines) < 2 || !strings.HasPrefix(strings.TrimSpace(lines[0]), "```") {
		return text
	}
	if strings.HasPrefix(strings.TrimSpace(lines[len(lines)-1]), "```") {
		return strings.TrimSpace(strings.Join(lines[1:len(lines)-1], "\n"))
	}
	return text
}

func extractFirstJSONObject(text string) (string, bool) {
	start := strings.Index(text, "{")
	if start < 0 {
		return "", false
	}
	depth := 0
	inString := false
	escaped := false
	for idx, r := range text[start:] {
		pos := start + idx
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if r == '\\' {
				escaped = true
				continue
			}
			if r == '"' {
				inString = false
			}
			continue
		}
		switch r {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return strings.TrimSpace(text[start : pos+len(string(r))]), true
			}
			if depth < 0 {
				return "", false
			}
		default:
			if depth == 0 && !unicode.IsSpace(r) {
				return "", false
			}
		}
	}
	return "", false
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
	if decision.ExecutionGraph != nil && len(decision.ExecutionGraph.Nodes) > 0 {
		output["stage"] = "llm_execution_graph_plan"
		output[outputKeyExecutionGraph] = executionGraphOutput(decision.ExecutionGraph)
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
		if err := validateShellCommandBoundary(command.Command, command.Args, "shellCommands.command"); err != nil {
			return err
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
	if decision.ExecutionGraph != nil {
		if err := validateExecutionGraph(*decision.ExecutionGraph); err != nil {
			return err
		}
	}
	if decision.CompletionState == "completed" {
		if decision.DoDAssessment == nil {
			return fmt.Errorf("%w: dodAssessment is required for completed decision", common.ErrInvalidArgument)
		}
		if strings.TrimSpace(decision.DoDAssessment.Summary) == "" {
			return fmt.Errorf("%w: dodAssessment.summary is required for completed decision", common.ErrInvalidArgument)
		}
		for _, item := range decision.DoDAssessment.Items {
			if item.State == "satisfied" && len(item.EvidenceRefs) == 0 {
				return fmt.Errorf("%w: satisfied dodAssessment item requires evidenceRefs", common.ErrInvalidArgument)
			}
		}
	}
	return nil
}

func validateExecutionGraph(graph llmmodels.BlockExecutionGraph) error {
	if len(graph.Nodes) == 0 {
		return fmt.Errorf("%w: executionGraph.nodes is required", common.ErrInvalidArgument)
	}
	known := map[string]bool{}
	for _, node := range graph.Nodes {
		nodeID := strings.TrimSpace(node.ID)
		if nodeID == "" || strings.TrimSpace(node.Kind) == "" {
			return fmt.Errorf("%w: executionGraph node id and kind are required", common.ErrInvalidArgument)
		}
		if strings.TrimSpace(node.Kind) == shellcommon.StepKindCommand {
			if command, _ := node.Input["command"].(string); strings.TrimSpace(command) != "" {
				if err := validateShellCommandBoundary(command, stringSliceValue(node.Input["args"]), "executionGraph shell command"); err != nil {
					return err
				}
			}
		}
		if !isSafeGraphNodeID(nodeID) {
			return fmt.Errorf("%w: executionGraph node id contains unsupported characters", common.ErrInvalidArgument)
		}
		if known[nodeID] {
			return fmt.Errorf("%w: executionGraph node id must be unique", common.ErrInvalidArgument)
		}
		known[nodeID] = true
	}
	for _, node := range graph.Nodes {
		nodeID := strings.TrimSpace(node.ID)
		for _, dependency := range node.DependsOn {
			dependency = strings.TrimSpace(dependency)
			if dependency == "" || !known[dependency] || dependency == nodeID {
				return fmt.Errorf("%w: executionGraph dependsOn references invalid node", common.ErrInvalidArgument)
			}
		}
	}
	if hasExecutionGraphCycle(graph) {
		return fmt.Errorf("%w: executionGraph must be acyclic", common.ErrInvalidArgument)
	}
	return nil
}

func validateShellCommandBoundary(command string, args []string, field string) error {
	command = strings.TrimSpace(command)
	commandLine := shellCommandLine(command, args)
	for _, unavailable := range unavailableRuntimeCommands() {
		if commandReferencesShellCommand(command, commandLine, unavailable) {
			return fmt.Errorf("%w: %s references unavailable runtime command %q", common.ErrInvalidArgument, field, unavailable)
		}
	}
	for _, ref := range localWorkorchServiceHTTPReferences(commandLine) {
		if !isDeclaredServiceAPI(ref, declaredRuntimeCapabilities().ServiceAPIs) {
			return fmt.Errorf("%w: %s references undeclared local workorch service API %s %s", common.ErrInvalidArgument, field, ref.Method, ref.Raw)
		}
	}
	return nil
}

func unavailableRuntimeCommands() []string {
	ret := []string{unavailableWorkorchCLICommand}
	for _, command := range unavailableIfMissingShellCommands {
		if _, err := exec.LookPath(command); err != nil {
			ret = append(ret, command)
		}
	}
	sort.Strings(ret)
	return ret
}

func commandReferencesShellCommand(command string, commandLine string, target string) bool {
	target = strings.TrimSpace(target)
	if target == "" {
		return false
	}
	if strings.TrimSpace(command) == target {
		return true
	}
	pattern := regexp.MustCompile(`(^|[\s;&|()])` + regexp.QuoteMeta(target) + `($|[\s;&|()])`)
	return pattern.MatchString(commandLine)
}

func shellCommandLine(command string, args []string) string {
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, strings.TrimSpace(command))
	parts = append(parts, args...)
	return strings.Join(parts, " ")
}

func stringSliceValue(input any) []string {
	switch typed := input.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		ret := make([]string, 0, len(typed))
		for _, item := range typed {
			if value, ok := item.(string); ok {
				ret = append(ret, value)
			}
		}
		return ret
	default:
		return nil
	}
}

type localServiceAPIReference struct {
	Method   string
	Path     string
	Raw      string
	HasQuery bool
}

func localWorkorchServiceHTTPReferences(commandLine string) []localServiceAPIReference {
	if !strings.Contains(commandLine, "localhost") &&
		!strings.Contains(commandLine, "127.0.0.1") &&
		!strings.Contains(commandLine, "/api/v1/workorch/") &&
		!strings.Contains(commandLine, "/runs") &&
		!strings.Contains(commandLine, "/observability") &&
		!strings.Contains(commandLine, "/metrics") {
		return nil
	}
	method := inferShellHTTPMethod(commandLine)
	refs := make([]localServiceAPIReference, 0)
	for _, raw := range httpURLPattern.FindAllString(commandLine, -1) {
		parsed, err := url.Parse(raw)
		if err != nil || !isLocalWorkorchServiceHost(parsed.Host) {
			continue
		}
		path := parsed.EscapedPath()
		if path == "" {
			path = "/"
		}
		refs = append(refs, localServiceAPIReference{
			Method:   method,
			Path:     path,
			Raw:      raw,
			HasQuery: parsed.RawQuery != "",
		})
	}
	for _, match := range bareLocalServicePathPattern.FindAllStringSubmatch(commandLine, -1) {
		if len(match) < 2 || strings.TrimSpace(match[1]) == "" {
			continue
		}
		raw := strings.TrimSpace(match[1])
		pathPart, queryPart, _ := strings.Cut(raw, "?")
		refs = append(refs, localServiceAPIReference{
			Method:   method,
			Path:     pathPart,
			Raw:      raw,
			HasQuery: queryPart != "",
		})
	}
	return refs
}

func inferShellHTTPMethod(commandLine string) string {
	upper := strings.ToUpper(commandLine)
	for _, method := range []string{"DELETE", "PATCH", "POST", "PUT", "GET"} {
		if strings.Contains(upper, "-X "+method) ||
			strings.Contains(upper, "-X"+method) ||
			strings.Contains(upper, "--REQUEST "+method) {
			return method
		}
	}
	lower := strings.ToLower(commandLine)
	if strings.Contains(lower, " --data") ||
		strings.Contains(lower, " --form") ||
		strings.Contains(lower, " -d ") {
		return "POST"
	}
	return "GET"
}

func isLocalWorkorchServiceHost(host string) bool {
	parsedHost := host
	port := ""
	if strings.Contains(host, ":") {
		if value, err := url.Parse("//" + host); err == nil {
			parsedHost = value.Hostname()
			port = value.Port()
		}
	}
	if parsedHost != "localhost" && parsedHost != "127.0.0.1" {
		return false
	}
	return port == "" || port == "8080"
}

func isDeclaredServiceAPI(ref localServiceAPIReference, apis []llmmodels.ServiceAPICapability) bool {
	for _, api := range apis {
		if !strings.EqualFold(ref.Method, strings.TrimSpace(api.Method)) {
			continue
		}
		if ref.HasQuery && !api.AllowQuery {
			continue
		}
		if serviceAPIPathMatches(api.PathTemplate, ref.Path) {
			return true
		}
	}
	return false
}

func serviceAPIPathMatches(template string, path string) bool {
	template = strings.Trim(strings.TrimSpace(template), "/")
	path = strings.Trim(strings.TrimSpace(path), "/")
	if template == path {
		return true
	}
	templateParts := strings.Split(template, "/")
	pathParts := strings.Split(path, "/")
	if len(templateParts) != len(pathParts) {
		return false
	}
	for idx, templatePart := range templateParts {
		if strings.HasPrefix(templatePart, "{") && strings.HasSuffix(templatePart, "}") {
			if pathParts[idx] == "" {
				return false
			}
			continue
		}
		if templatePart != pathParts[idx] {
			return false
		}
	}
	return true
}

func isSafeGraphNodeID(input string) bool {
	for _, r := range input {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-' || r == '.' {
			continue
		}
		return false
	}
	return true
}

func hasExecutionGraphCycle(graph llmmodels.BlockExecutionGraph) bool {
	dependencies := make(map[string][]string, len(graph.Nodes))
	for _, node := range graph.Nodes {
		nodeID := strings.TrimSpace(node.ID)
		dependencies[nodeID] = append([]string(nil), node.DependsOn...)
	}
	visiting := map[string]bool{}
	visited := map[string]bool{}
	var visit func(string) bool
	visit = func(nodeID string) bool {
		if visiting[nodeID] {
			return true
		}
		if visited[nodeID] {
			return false
		}
		visiting[nodeID] = true
		for _, dependency := range dependencies[nodeID] {
			if visit(strings.TrimSpace(dependency)) {
				return true
			}
		}
		visiting[nodeID] = false
		visited[nodeID] = true
		return false
	}
	for nodeID := range dependencies {
		if visit(nodeID) {
			return true
		}
	}
	return false
}

func providerRequest(request runtimemodel.StepExecutionRequest, config llmmodels.ProviderConfig) llmmodels.ProviderRequest {
	input := cloneMap(request.Input)
	enrichProviderInputWithSoulRouting(input)
	return llmmodels.ProviderRequest{
		WorkspaceID:         string(request.WorkspaceID),
		RunID:               string(request.RunID),
		StepID:              string(request.StepID),
		Kind:                request.Kind,
		Model:               config.Model,
		Input:               input,
		Schema:              decisionSchemaName,
		RuntimeCapabilities: declaredRuntimeCapabilities(),
		TraceRefs: map[string]string{
			"workspaceId": string(request.WorkspaceID),
			"runId":       string(request.RunID),
			"stepId":      string(request.StepID),
		},
		Config: RedactedProviderConfig(config),
	}
}

func enrichProviderInputWithSoulRouting(input map[string]any) {
	if len(input) == 0 {
		return
	}
	executionContext := recordFromAny(input["executionContext"])
	if len(executionContext) == 0 {
		return
	}
	stage := inferProviderSoulStage(input)
	if stage == "" {
		return
	}
	input["currentSoulStage"] = stage

	routing := recordFromAny(executionContext["soulRouting"])
	if len(routing) == 0 {
		if soul := recordFromAny(executionContext["soul"]); len(soul) > 0 {
			input["currentSoul"] = soul
		}
		return
	}
	bindings := recordFromAny(routing["bindings"])
	binding := recordFromAny(bindings[stage])
	if len(binding) == 0 {
		return
	}
	input["currentSoulBinding"] = binding
	if soul := recordFromAny(binding["soul"]); len(soul) > 0 {
		input["currentSoul"] = soul
		return
	}
	if soulID := stringValue(binding["soulId"]); soulID != "" {
		input["currentSoul"] = map[string]any{
			"id":     soulID,
			"source": stringValue(binding["source"]),
		}
	}
}

func inferProviderSoulStage(input map[string]any) string {
	if _, ok := input["decisionResolution"]; ok {
		return "decision"
	}
	if _, ok := input["approvalResolution"]; ok {
		return "approval"
	}
	if boolValue(input["graphFollowUp"]) || boolValue(input["completionFollowUp"]) {
		return "validation"
	}
	if _, ok := input["previousStepOutput"]; ok {
		return "validation"
	}
	return "planning"
}

func recordFromAny(value any) map[string]any {
	switch typed := value.(type) {
	case map[string]any:
		return typed
	default:
		if value == nil {
			return nil
		}
		payload, err := json.Marshal(value)
		if err != nil {
			return nil
		}
		var ret map[string]any
		if err := json.Unmarshal(payload, &ret); err != nil {
			return nil
		}
		return ret
	}
}

func boolValue(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	default:
		return false
	}
}

func declaredRuntimeCapabilities() llmmodels.RuntimeCapabilities {
	return llmmodels.RuntimeCapabilities{
		BlockKinds: []llmmodels.RuntimeBlockCapability{
			{
				Kind:        llmcommon.StepKindWorkspaceExecution,
				Description: "Plan the next workspace execution step from workspace goal, DoD, memory, skills, and MCP context.",
			},
			{
				Kind:        shellcommon.StepKindCommand,
				Description: "Execute one bounded local shell command in the workspace working directory.",
				Input:       map[string]any{"command": "string", "args": "[]string"},
			},
			{
				Kind:        sshcommon.StepKindCommand,
				Description: "Execute one bounded non-interactive SSH command on a remote host using supplied password or private key credentials.",
				Input:       map[string]any{"host": "string", "port": "number", "username": "string", "password": "string", "privateKey": "string", "privateKeyPath": "string", "command": "string", "timeoutMillis": "number"},
			},
			{
				Kind:        mcpcommon.StepKindToolCall,
				Description: "Call an explicitly configured MCP server tool.",
				Input:       map[string]any{"serverId": "string", "tool": "string", "input": "object"},
			},
			{
				Kind:        mcpcommon.StepKindResourceRead,
				Description: "Read an explicitly allowed MCP resource from a configured server.",
				Input:       map[string]any{"serverId": "string", "uri": "string"},
			},
			{
				Kind:        mcpcommon.StepKindPromptGet,
				Description: "Get an explicitly allowed MCP prompt from a configured server.",
				Input:       map[string]any{"serverId": "string", "prompt": "string", "arguments": "object"},
			},
		},
		ServiceAPIs: []llmmodels.ServiceAPICapability{
			{
				Method:       "GET",
				PathTemplate: "/api/v1/workorch/runs/{runId}/snapshot",
				Description:  "Read the current run snapshot. Use the active runId from the provider request.",
				ReadOnly:     true,
			},
			{
				Method:       "GET",
				PathTemplate: "/api/v1/workorch/runs/{runId}/observability",
				Description:  "Read the current run observability timeline and evidence counters.",
				ReadOnly:     true,
			},
			{
				Method:       "GET",
				PathTemplate: "/metrics",
				Description:  "Read service-level Prometheus metrics. This endpoint has no per-run query contract.",
				ReadOnly:     true,
			},
		},
		ShellPolicy: llmmodels.ShellRuntimePolicy{
			Enabled:                 true,
			LocalServiceHTTP:        localServiceHTTPDeclaredOnly,
			UnavailableCommands:     unavailableRuntimeCommands(),
			AllowedLocalServiceHost: []string{"localhost:8080", "127.0.0.1:8080"},
		},
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
	for _, request := range normalizedPatchRequests(requests) {
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

func normalizedPatchRequests(requests []llmmodels.PatchRequest) []llmmodels.PatchRequest {
	ret := make([]llmmodels.PatchRequest, 0, len(requests))
	for _, request := range requests {
		request.PatchType = normalizedPatchType(request)
		request.Operation = ""
		ret = append(ret, request)
	}
	return ret
}

func normalizedPatchType(request llmmodels.PatchRequest) string {
	patchType := strings.TrimSpace(request.PatchType)
	if patchType == "" {
		patchType = strings.TrimSpace(request.Operation)
	}
	switch patchType {
	case "", "create", "write", "replace", "file":
		return "file_write"
	default:
		return patchType
	}
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

func executionGraphOutput(graph *llmmodels.BlockExecutionGraph) map[string]any {
	if graph == nil {
		return nil
	}
	nodes := make([]map[string]any, 0, len(graph.Nodes))
	for _, node := range graph.Nodes {
		item := map[string]any{
			"id":   node.ID,
			"kind": node.Kind,
		}
		if len(node.DependsOn) > 0 {
			dependsOn := append([]string(nil), node.DependsOn...)
			sort.Strings(dependsOn)
			item["dependsOn"] = dependsOn
		}
		if strings.TrimSpace(node.Branch) != "" {
			item["branch"] = strings.TrimSpace(node.Branch)
		}
		if node.Input != nil {
			item["input"] = cloneMap(node.Input)
		}
		nodes = append(nodes, item)
	}
	return map[string]any{"nodes": nodes}
}

func decisionMap(decision llmmodels.LLMExecutionDecision) map[string]any {
	decision.PatchRequests = normalizedPatchRequests(decision.PatchRequests)
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

func previewLLMOutput(payload []byte) string {
	text := strings.TrimSpace(string(payload))
	if len(text) <= maxLLMOutputPreviewBytes {
		return text
	}
	return text[:maxLLMOutputPreviewBytes]
}
