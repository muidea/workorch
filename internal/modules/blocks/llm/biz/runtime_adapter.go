package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	filecommon "github.com/muidea/workorch/internal/modules/blocks/file/pkg/common"
	gitcommon "github.com/muidea/workorch/internal/modules/blocks/git/pkg/common"
	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	mcpcommon "github.com/muidea/workorch/internal/modules/blocks/mcp/pkg/common"
	shellcommon "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/common"
	sshcommon "github.com/muidea/workorch/internal/modules/blocks/ssh/pkg/common"
	webcommon "github.com/muidea/workorch/internal/modules/blocks/web/pkg/common"
	runtimemodel "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/dialoglanguage"
	"github.com/muidea/workorch/internal/pkg/runtimepath"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

const decisionSchemaName = "workorch.llmExecutionDecision.v1"

const (
	outputKeyStatus                = "status"
	outputKeyError                 = "error"
	outputKeyBlockDecisionKind     = "blockDecisionKind"
	outputKeyBlockDecisionQuestion = "blockDecisionQuestion"
	outputKeyBlockDecisionContext  = "blockDecisionContext"
	outputKeyBlockApprovalAction   = "blockApprovalAction"
	outputKeyBlockApprovalContext  = "blockApprovalContext"
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

var providerSoulStages = map[string]struct{}{
	"planning":   {},
	"execution":  {},
	"decision":   {},
	"approval":   {},
	"validation": {},
	"artifact":   {},
}

type Provider interface {
	Complete(context.Context, llmmodels.ProviderRequest) (llmmodels.ProviderCompletionResult, error)
}

type StreamingProvider interface {
	StreamComplete(context.Context, llmmodels.ProviderRequest, func(llmmodels.ProviderStreamEvent) error) (llmmodels.ProviderCompletionResult, error)
}

type EmbeddingProvider interface {
	Embed(context.Context, llmmodels.ProviderEmbeddingRequest) (llmmodels.ProviderEmbeddingResult, error)
}

type ProviderRuntime struct {
	Config   llmmodels.ProviderConfig
	Provider Provider
}

type ProviderConfigLoader func() (string, map[string]llmmodels.ProviderConfig, error)
type ProviderConfigRevision func() uint64
type ProviderFactory func(llmmodels.ProviderConfig) Provider

type providerRuntimeCache struct {
	ready             bool
	revision          uint64
	defaultProviderID string
	providers         map[string]ProviderRuntime
}

type RuntimeAdapter struct {
	defaultProviderID string
	providers         map[string]ProviderRuntime
	configLoader      ProviderConfigLoader
	configRevision    ProviderConfigRevision
	providerFactory   ProviderFactory
	cacheMu           sync.RWMutex
	cache             providerRuntimeCache
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
	return &RuntimeAdapter{defaultProviderID: defaultProviderID, providers: normalized, providerFactory: NewProviderFromConfig}
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
		providerFactory:   NewProviderFromConfig,
	}
}

func NewRuntimeAdapterFromConfigLoaderWithRevision(loader ProviderConfigLoader, revision ProviderConfigRevision) *RuntimeAdapter {
	adapter := NewRuntimeAdapterFromConfigLoader(loader)
	adapter.configRevision = revision
	return adapter
}

func providerRuntimesFromConfigs(configs map[string]llmmodels.ProviderConfig) map[string]ProviderRuntime {
	return providerRuntimesFromConfigsWithFactory(configs, NewProviderFromConfig)
}

func providerRuntimesFromConfigsWithFactory(configs map[string]llmmodels.ProviderConfig, factory ProviderFactory) map[string]ProviderRuntime {
	if factory == nil {
		factory = NewProviderFromConfig
	}
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
			provider = newLazyProvider(config, factory)
		}
		config.Capabilities = effectiveProviderCapabilities(config.Capabilities)
		providers[id] = ProviderRuntime{
			Config:   config,
			Provider: provider,
		}
	}
	return providers
}

type lazyProvider struct {
	config   llmmodels.ProviderConfig
	factory  ProviderFactory
	mu       sync.Mutex
	provider Provider
}

func newLazyProvider(config llmmodels.ProviderConfig, factory ProviderFactory) *lazyProvider {
	if factory == nil {
		factory = NewProviderFromConfig
	}
	return &lazyProvider{config: config, factory: factory}
}

func (p *lazyProvider) Complete(ctx context.Context, request llmmodels.ProviderRequest) (llmmodels.ProviderCompletionResult, error) {
	provider := p.runtimeProvider()
	if provider == nil {
		return llmmodels.ProviderCompletionResult{}, fmt.Errorf("%w: llm provider client is not configured", common.ErrInvalidArgument)
	}
	return provider.Complete(ctx, request)
}

func (p *lazyProvider) StreamComplete(ctx context.Context, request llmmodels.ProviderRequest, emit func(llmmodels.ProviderStreamEvent) error) (llmmodels.ProviderCompletionResult, error) {
	provider := p.runtimeProvider()
	streaming, ok := provider.(StreamingProvider)
	if !ok {
		return llmmodels.ProviderCompletionResult{}, fmt.Errorf("%w: llm provider does not implement streaming", common.ErrInvalidArgument)
	}
	return streaming.StreamComplete(ctx, request, emit)
}

func (p *lazyProvider) Embed(ctx context.Context, request llmmodels.ProviderEmbeddingRequest) (llmmodels.ProviderEmbeddingResult, error) {
	provider := p.runtimeProvider()
	embedding, ok := provider.(EmbeddingProvider)
	if !ok {
		return llmmodels.ProviderEmbeddingResult{}, fmt.Errorf("%w: llm provider does not implement embeddings", common.ErrInvalidArgument)
	}
	return embedding.Embed(ctx, request)
}

func (p *lazyProvider) runtimeProvider() Provider {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.provider == nil {
		p.provider = p.factory(p.config)
	}
	return p.provider
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
	operation := requestedProviderOperation(request.Input)
	invokeStartedAt := time.Now().UTC()
	runtime, providerID, err := a.selectProviderRuntime(request)
	if err != nil {
		cfg := llmmodels.ProviderConfig{ID: providerID, Unavailable: true, UnavailableReason: err.Error()}
		output := failureOutput(request, "llm_provider_unavailable", err.Error(), cfg)
		attachLLMUsage(output, buildLLMUsageOutput(request, providerCallResult{
			runtime: ProviderRuntime{Config: cfg},
		}, operation, invokeStartedAt, time.Now().UTC(), providerFailureClassUnavailable, "failed"))
		return runtimemodel.StepExecutionResult{Output: output}, err
	}
	if runtime.Config.Unavailable {
		output := failureOutput(request, "llm_provider_unavailable", runtime.Config.UnavailableReason, runtime.Config)
		attachLLMUsage(output, buildLLMUsageOutput(request, providerCallResult{
			runtime: ProviderRuntime{Config: runtime.Config},
		}, operation, invokeStartedAt, time.Now().UTC(), providerFailureClassUnavailable, "failed"))
		return runtimemodel.StepExecutionResult{Output: output}, fmt.Errorf("%w: %s", common.ErrInvalidArgument, runtime.Config.UnavailableReason)
	}
	if runtime.Provider == nil {
		output := failureOutput(request, "llm_provider_client_unavailable", "llm provider client is not configured", runtime.Config)
		attachLLMUsage(output, buildLLMUsageOutput(request, providerCallResult{
			runtime: ProviderRuntime{Config: runtime.Config},
		}, operation, invokeStartedAt, time.Now().UTC(), providerFailureClassUnavailable, "failed"))
		return runtimemodel.StepExecutionResult{Output: output}, fmt.Errorf("%w: llm provider client is not configured", common.ErrInvalidArgument)
	}
	if err := validateProviderOperation(operation, runtime.Config.Capabilities); err != nil {
		output := failureOutput(request, "llm_provider_operation_unsupported", err.Error(), runtime.Config)
		output["operation"] = operation
		attachLLMUsage(output, buildLLMUsageOutput(request, providerCallResult{
			runtime: ProviderRuntime{Config: runtime.Config},
		}, operation, invokeStartedAt, time.Now().UTC(), "operation_unsupported", "failed"))
		return runtimemodel.StepExecutionResult{Output: output}, err
	}

	call := a.executeProviderWithFailover(ctx, request, runtime, providerID, operation)
	invokeCompletedAt := time.Now().UTC()
	llmRefs, llmRefErr := persistLLMCallEvidence(request, call)
	if call.err != nil {
		output := failureOutput(request, call.failureStage, call.err.Error(), call.runtime.Config)
		attachContextBudget(output, call.request.ContextBudget, call.runtime.Config.ID, call.runtime.Config.Model)
		attachStreamDiagnostics(output, call.streamEvents)
		attachFailoverMetadata(output, call.failover)
		usage := buildLLMUsageOutput(request, call, operation, invokeStartedAt, invokeCompletedAt, classifyProviderInvocationFailure(call.err), "failed")
		llmRefErr = persistLLMCallUsageEvidence(request, &llmRefs, usage, llmRefErr)
		attachLLMUsage(output, usage)
		attachLLMCallRefs(output, llmRefs, llmRefErr)
		return runtimemodel.StepExecutionResult{Output: output}, call.err
	}
	payload := call.payload
	streamEvents := call.streamEvents
	providerReq := call.request
	runtime = call.runtime
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
				llmRefErr = persistLLMCallRepairResponseEvidence(request, &llmRefs, repairPayload, llmRefErr)
			}
			attachContextBudget(output, providerReq.ContextBudget, runtime.Config.ID, runtime.Config.Model)
			attachStreamDiagnostics(output, streamEvents)
			attachFailoverMetadata(output, call.failover)
			usage := buildLLMUsageOutput(request, call, operation, invokeStartedAt, invokeCompletedAt, "llm_schema_invalid", "failed")
			llmRefErr = persistLLMCallUsageEvidence(request, &llmRefs, usage, llmRefErr)
			attachLLMUsage(output, usage)
			attachLLMCallRefs(output, llmRefs, llmRefErr)
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
		attachContextBudget(failure, providerReq.ContextBudget, runtime.Config.ID, runtime.Config.Model)
		attachStreamDiagnostics(failure, streamEvents)
		attachFailoverMetadata(failure, call.failover)
		usage := buildLLMUsageOutput(request, call, operation, invokeStartedAt, invokeCompletedAt, "llm_decision_invalid", "failed")
		llmRefErr = persistLLMCallUsageEvidence(request, &llmRefs, usage, llmRefErr)
		attachLLMUsage(failure, usage)
		attachLLMCallRefs(failure, llmRefs, llmRefErr)
		return runtimemodel.StepExecutionResult{Output: failure}, err
	}
	attachContextBudget(output, providerReq.ContextBudget, runtime.Config.ID, runtime.Config.Model)
	attachStreamDiagnostics(output, streamEvents)
	attachFailoverMetadata(output, call.failover)
	llmUsageStatus := "completed"
	if repairMetadata != nil {
		output["llmRepair"] = repairMetadata
	}
	if outputStatus, _ := output[outputKeyStatus].(string); strings.TrimSpace(outputStatus) != "" {
		llmUsageStatus = outputStatus
	}
	if llmUsageStatus == "failed" {
		reason, _ := output[outputKeyError].(string)
		if strings.TrimSpace(reason) == "" {
			reason = "llm decision reported failure"
		}
		usage := buildLLMUsageOutput(request, call, operation, invokeStartedAt, invokeCompletedAt, "llm_decision_failed", "failed")
		llmRefErr = persistLLMCallUsageEvidence(request, &llmRefs, usage, llmRefErr)
		attachLLMUsage(output, usage)
		attachLLMCallRefs(output, llmRefs, llmRefErr)
		return runtimemodel.StepExecutionResult{Output: output}, fmt.Errorf("%w: %s", common.ErrInvalidArgument, reason)
	}
	usage := buildLLMUsageOutput(request, call, operation, invokeStartedAt, invokeCompletedAt, "", llmUsageStatus)
	llmRefErr = persistLLMCallUsageEvidence(request, &llmRefs, usage, llmRefErr)
	attachLLMUsage(output, usage)
	attachLLMCallRefs(output, llmRefs, llmRefErr)
	return runtimemodel.StepExecutionResult{Output: output}, nil
}

const (
	providerFailureClassTimeout          = "provider_timeout"
	providerFailureClassContextLarge     = "context_too_large"
	providerFailureClassRateLimited      = "rate_limited"
	providerFailureClassAuthOrConfig     = "auth_or_config_error"
	providerFailureClassStreamIncomplete = "provider_stream_incomplete"
	providerFailureClassUnavailable      = "provider_unavailable"
	providerFailureClassContextIntegrity = "context_integrity_failed"
)

type providerCallResult struct {
	runtime      ProviderRuntime
	request      llmmodels.ProviderRequest
	payload      []byte
	usage        *llmmodels.ProviderUsage
	streamEvents []llmmodels.ProviderStreamEvent
	failover     map[string]any
	failureStage string
	err          error
}

func (a *RuntimeAdapter) executeProviderWithFailover(
	ctx context.Context,
	request runtimemodel.StepExecutionRequest,
	primaryRuntime ProviderRuntime,
	primaryProviderID string,
	operation string,
) providerCallResult {
	providerReq := providerRequest(request, primaryRuntime.Config)
	payload, usage, streamEvents, callErr := completeProvider(ctx, primaryRuntime, providerReq)
	result := providerCallResult{
		runtime:      primaryRuntime,
		request:      providerReq,
		payload:      payload,
		usage:        usage,
		streamEvents: streamEvents,
	}
	if callErr == nil {
		return result
	}

	failureClass := classifyProviderInvocationFailure(callErr)
	result.failureStage = providerFailureStage(failureClass, len(streamEvents) > 0)
	result.err = callErr
	if !shouldAttemptProviderFailover(primaryRuntime.Config, failureClass) {
		return result
	}

	failover := map[string]any{
		"attempted":       true,
		"success":         false,
		"failureClass":    failureClass,
		"primaryError":    callErr.Error(),
		"primaryStage":    result.failureStage,
		"primaryBudget":   providerContextBudgetOutput(providerReq.ContextBudget, primaryRuntime.Config.ID, primaryRuntime.Config.Model),
		"retryWithBackup": true,
	}
	backupRuntime, _, ok := a.selectFailoverRuntime(primaryProviderID, primaryRuntime.Config, operation)
	if !ok {
		failover["reason"] = "no failover provider is available"
		result.failover = failover
		return result
	}

	backupReq := providerRequest(request, backupRuntime.Config)
	backupPayload, backupUsage, backupEvents, backupErr := completeProvider(ctx, backupRuntime, backupReq)
	failover["toBudget"] = providerContextBudgetOutput(backupReq.ContextBudget, backupRuntime.Config.ID, backupRuntime.Config.Model)
	if backupErr != nil {
		failover["backupError"] = backupErr.Error()
		failover["backupFailureClass"] = classifyProviderInvocationFailure(backupErr)
		failover["backupStage"] = providerFailureStage(classifyProviderInvocationFailure(backupErr), len(backupEvents) > 0)
		combinedEvents := append([]llmmodels.ProviderStreamEvent{}, streamEvents...)
		combinedEvents = append(combinedEvents, backupEvents...)
		result.runtime = backupRuntime
		result.request = backupReq
		result.streamEvents = combinedEvents
		result.payload = backupPayload
		result.usage = backupUsage
		result.failureStage = providerFailureStage(classifyProviderInvocationFailure(backupErr), len(combinedEvents) > 0)
		result.err = backupErr
		result.failover = failover
		return result
	}

	failover["success"] = true
	result.runtime = backupRuntime
	result.request = backupReq
	result.payload = backupPayload
	result.usage = backupUsage
	result.streamEvents = backupEvents
	result.failover = failover
	result.err = nil
	result.failureStage = ""
	return result
}

func shouldAttemptProviderFailover(config llmmodels.ProviderConfig, failureClass string) bool {
	if !config.FailoverEnabled {
		return false
	}
	switch strings.TrimSpace(failureClass) {
	case providerFailureClassTimeout, providerFailureClassContextLarge, providerFailureClassRateLimited, providerFailureClassStreamIncomplete, providerFailureClassUnavailable:
		return true
	default:
		return false
	}
}

func (a *RuntimeAdapter) selectFailoverRuntime(primaryProviderID string, primaryConfig llmmodels.ProviderConfig, operation string) (ProviderRuntime, string, bool) {
	defaultProviderID, providers, err := a.providerRuntimes()
	if err != nil || len(providers) == 0 {
		return ProviderRuntime{}, "", false
	}
	primaryProviderID = strings.TrimSpace(primaryProviderID)
	if primaryProviderID == "" {
		primaryProviderID = strings.TrimSpace(primaryConfig.ID)
	}
	candidates := []string{}
	seen := map[string]bool{}
	for _, candidate := range primaryConfig.FailoverProviders {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" || seen[candidate] {
			continue
		}
		seen[candidate] = true
		candidates = append(candidates, candidate)
	}
	if defaultProviderID != "" && !seen[defaultProviderID] {
		seen[defaultProviderID] = true
		candidates = append(candidates, defaultProviderID)
	}
	ids := make([]string, 0, len(providers))
	for id := range providers {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if !seen[id] {
			candidates = append(candidates, id)
		}
	}
	for _, candidate := range candidates {
		if strings.TrimSpace(candidate) == "" || candidate == primaryProviderID {
			continue
		}
		runtime, ok := providers[candidate]
		if !ok || runtime.Config.Unavailable || runtime.Provider == nil {
			continue
		}
		if validateProviderOperation(operation, runtime.Config.Capabilities) != nil {
			continue
		}
		return runtime, candidate, true
	}
	return ProviderRuntime{}, "", false
}

func classifyProviderInvocationFailure(err error) string {
	text := ""
	if err != nil {
		text = strings.ToLower(strings.TrimSpace(err.Error()))
	}
	switch {
	case strings.Contains(text, "context length"),
		strings.Contains(text, "maximum context"),
		strings.Contains(text, "context window"),
		strings.Contains(text, "too many tokens"),
		strings.Contains(text, "token limit"),
		strings.Contains(text, "request too large"),
		strings.Contains(text, "input is too long"),
		strings.Contains(text, "prompt is too long"),
		strings.Contains(text, "413"):
		return providerFailureClassContextLarge
	case strings.Contains(text, "stream incomplete"),
		strings.Contains(text, "ended before [done]"),
		strings.Contains(text, "without [done]"):
		return providerFailureClassStreamIncomplete
	case strings.Contains(text, "context gate"),
		strings.Contains(text, "context integrity"):
		return providerFailureClassContextIntegrity
	case strings.Contains(text, "deadline exceeded"), strings.Contains(text, "timeout"):
		return providerFailureClassTimeout
	case strings.Contains(text, "rate limit"),
		strings.Contains(text, "too many requests"),
		strings.Contains(text, "quota exceeded"),
		strings.Contains(text, "429"):
		return providerFailureClassRateLimited
	case strings.Contains(text, "unauthorized"),
		strings.Contains(text, "forbidden"),
		strings.Contains(text, "invalid api key"),
		strings.Contains(text, "authentication"),
		strings.Contains(text, "api key"),
		strings.Contains(text, "401"),
		strings.Contains(text, "403"):
		return providerFailureClassAuthOrConfig
	default:
		return providerFailureClassUnavailable
	}
}

func providerFailureStage(failureClass string, stream bool) string {
	switch strings.TrimSpace(failureClass) {
	case providerFailureClassTimeout:
		return "llm_provider_timeout"
	case providerFailureClassContextLarge:
		return "llm_provider_request_too_large"
	case providerFailureClassRateLimited:
		return "llm_provider_rate_limited"
	case providerFailureClassAuthOrConfig:
		return "llm_provider_auth_or_config_error"
	case providerFailureClassStreamIncomplete:
		return "llm_provider_stream_incomplete"
	case providerFailureClassContextIntegrity:
		return "llm_context_integrity_failed"
	case providerFailureClassUnavailable:
		if stream {
			return "llm_provider_stream_error"
		}
		return "llm_provider_unavailable"
	default:
		if stream {
			return "llm_provider_stream_error"
		}
		return "llm_provider_error"
	}
}

func completeProvider(ctx context.Context, runtime ProviderRuntime, request llmmodels.ProviderRequest) ([]byte, *llmmodels.ProviderUsage, []llmmodels.ProviderStreamEvent, error) {
	if request.ContextBudget != nil {
		if request.ContextBudget.P0Overflow {
			return nil, nil, nil, fmt.Errorf("%w: llm provider request too large: p0 overflow with protected goal/dod contract", common.ErrInvalidArgument)
		}
		if request.ContextBudget.OverBudget {
			return nil, nil, nil, fmt.Errorf("%w: llm provider request too large after context budget compaction", common.ErrInvalidArgument)
		}
		if err := validateProviderContextGate(request.ContextBudget); err != nil {
			return nil, nil, nil, err
		}
	}
	if runtime.Config.Capabilities.Streaming {
		streaming, ok := runtime.Provider.(StreamingProvider)
		if !ok {
			return nil, nil, nil, fmt.Errorf("%w: llm provider does not implement streaming", common.ErrInvalidArgument)
		}
		events := []llmmodels.ProviderStreamEvent{}
		completion, err := streaming.StreamComplete(ctx, request, func(event llmmodels.ProviderStreamEvent) error {
			if strings.TrimSpace(event.Type) == "" {
				event.Type = "delta"
			}
			events = append(events, event)
			return nil
		})
		return completion.Payload, completion.Usage, events, err
	}
	completion, err := runtime.Provider.Complete(ctx, request)
	return completion.Payload, completion.Usage, nil, err
}

func validateProviderContextGate(budget *llmmodels.ProviderContextBudget) error {
	if budget == nil {
		return nil
	}
	action := strings.TrimSpace(budget.ContextGateAction)
	if action == "" || action == providerContextGatePass {
		return nil
	}
	reason := strings.TrimSpace(budget.ContextGateReason)
	if reason == "" && len(budget.IntegrityWarnings) > 0 {
		reason = strings.Join(budget.IntegrityWarnings, ",")
	}
	if reason == "" {
		reason = "required LLM context is incomplete"
	}
	return fmt.Errorf("%w: llm context gate %s: %s", common.ErrInvalidArgument, action, reason)
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

type llmCallEvidenceRefs struct {
	BaseRef           string
	RequestRef        string
	ResponseRef       string
	StreamRef         string
	UsageRef          string
	RepairResponseRef string
}

func persistLLMCallEvidence(request runtimemodel.StepExecutionRequest, call providerCallResult) (llmCallEvidenceRefs, error) {
	root, refs, ok := llmCallEvidenceRootAndRefs(request)
	if !ok {
		return llmCallEvidenceRefs{}, nil
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return refs, err
	}
	if err := writeLLMJSONFileAtomic(filepath.Join(root, "request.json"), call.request); err != nil {
		return refs, err
	}
	refs.RequestRef = refs.BaseRef + "/request.json"
	if len(call.payload) > 0 {
		if err := writeLLMBytesFileAtomic(filepath.Join(root, "response.json"), call.payload); err != nil {
			return refs, err
		}
		refs.ResponseRef = refs.BaseRef + "/response.json"
	}
	if len(call.streamEvents) > 0 {
		if err := writeLLMBytesFileAtomic(filepath.Join(root, "response.sse"), llmStreamEventsSSE(call.streamEvents)); err != nil {
			return refs, err
		}
		refs.StreamRef = refs.BaseRef + "/response.sse"
	}
	return refs, nil
}

func persistLLMCallRepairResponseEvidence(request runtimemodel.StepExecutionRequest, refs *llmCallEvidenceRefs, payload []byte, previous error) error {
	if previous != nil || len(payload) == 0 {
		return previous
	}
	root, resolvedRefs, ok := llmCallEvidenceRootAndRefs(request)
	if !ok {
		return nil
	}
	if refs != nil && refs.BaseRef == "" {
		*refs = resolvedRefs
	}
	if err := writeLLMBytesFileAtomic(filepath.Join(root, "repair-response.json"), payload); err != nil {
		return err
	}
	if refs != nil {
		refs.RepairResponseRef = resolvedRefs.BaseRef + "/repair-response.json"
	}
	return nil
}

func persistLLMCallUsageEvidence(request runtimemodel.StepExecutionRequest, refs *llmCallEvidenceRefs, usage map[string]any, previous error) error {
	if previous != nil || len(usage) == 0 {
		return previous
	}
	root, resolvedRefs, ok := llmCallEvidenceRootAndRefs(request)
	if !ok {
		return nil
	}
	if refs != nil && refs.BaseRef == "" {
		*refs = resolvedRefs
	}
	if err := writeLLMJSONFileAtomic(filepath.Join(root, "usage.json"), usage); err != nil {
		return err
	}
	if refs != nil {
		refs.UsageRef = resolvedRefs.BaseRef + "/usage.json"
	}
	return nil
}

func llmCallEvidenceRootAndRefs(request runtimemodel.StepExecutionRequest) (string, llmCallEvidenceRefs, bool) {
	internalRoot := strings.TrimSpace(request.WorkspaceInternalDirectory)
	if internalRoot == "" || request.WorkspaceID == "" || request.RunID == "" || request.StepID == "" {
		return "", llmCallEvidenceRefs{}, false
	}
	callPath := llmCallEvidencePath(internalRoot, string(request.RunID), string(request.StepID))
	root := filepath.Join(internalRoot, "runs", string(request.RunID), "llm-calls", filepath.FromSlash(callPath))
	baseRef := "workorch://workspaces/" + string(request.WorkspaceID) + "/runs/" + string(request.RunID) + "/llm-calls/" + callPath
	return root, llmCallEvidenceRefs{BaseRef: baseRef}, true
}

func llmCallEvidencePath(internalRoot string, runID string, stepID string) string {
	return runtimepath.SequenceScopedPath(filepath.Join(internalRoot, "runs", runID, "llm-calls"), runID, stepID)
}

func llmStreamEventsSSE(events []llmmodels.ProviderStreamEvent) []byte {
	var builder strings.Builder
	for _, event := range events {
		payload := map[string]any{"type": strings.TrimSpace(event.Type)}
		if payload["type"] == "" {
			payload["type"] = "delta"
		}
		if event.Delta != "" {
			payload["delta"] = event.Delta
		}
		if len(event.Payload) > 0 {
			payload["payload"] = cloneMap(event.Payload)
		}
		raw, err := json.Marshal(payload)
		if err != nil {
			continue
		}
		builder.WriteString("data: ")
		builder.Write(raw)
		builder.WriteString("\n\n")
	}
	return []byte(builder.String())
}

func writeLLMJSONFileAtomic(path string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return writeLLMBytesFileAtomic(path, data)
}

func writeLLMBytesFileAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	cleanup = false
	return nil
}

func attachLLMCallRefs(output map[string]any, refs llmCallEvidenceRefs, persistErr error) {
	if output == nil || (refs.BaseRef == "" && persistErr == nil) {
		return
	}
	item := map[string]any{}
	if refs.BaseRef != "" {
		item["baseRef"] = refs.BaseRef
		if refs.RequestRef != "" {
			item["requestRef"] = refs.RequestRef
		}
		if refs.ResponseRef != "" {
			item["responseRef"] = refs.ResponseRef
		}
		if refs.StreamRef != "" {
			item["streamRef"] = refs.StreamRef
		}
		if refs.UsageRef != "" {
			item["usageRef"] = refs.UsageRef
		}
		if refs.RepairResponseRef != "" {
			item["repairResponseRef"] = refs.RepairResponseRef
		}
	}
	if persistErr != nil {
		item["persistenceError"] = persistErr.Error()
	}
	output["llmCallRefs"] = item
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

func attachStreamDiagnostics(output map[string]any, events []llmmodels.ProviderStreamEvent) {
	if output == nil || len(events) == 0 {
		return
	}
	deltaCount := 0
	receivedBytes := 0
	doneReceived := false
	for _, event := range events {
		switch strings.TrimSpace(event.Type) {
		case "done":
			doneReceived = true
		case "", "delta":
			if event.Delta != "" {
				deltaCount++
				receivedBytes += len(event.Delta)
			}
		default:
			if event.Delta != "" {
				receivedBytes += len(event.Delta)
			}
		}
	}
	output["streamDiagnostics"] = map[string]any{
		"streaming":          true,
		"receivedEventCount": len(events),
		"receivedDeltaCount": deltaCount,
		"receivedBytes":      receivedBytes,
		"doneReceived":       doneReceived,
		"streamIncomplete":   !doneReceived,
	}
}

func attachContextBudget(output map[string]any, budget *llmmodels.ProviderContextBudget, providerID, model string) {
	if output == nil || budget == nil {
		return
	}
	output["contextBudget"] = providerContextBudgetOutput(budget, providerID, model)
}

func providerContextBudgetOutput(budget *llmmodels.ProviderContextBudget, providerID, model string) map[string]any {
	if budget == nil {
		return nil
	}
	_ = providerID
	_ = model
	item := map[string]any{
		"maxBytes":                budget.MaxBytes,
		"actualBytes":             budget.ActualBytes,
		"maxTokens":               budget.MaxTokens,
		"estimatedInputTokens":    budget.EstimatedInputTokens,
		"estimatedOutputTokens":   budget.EstimatedOutputTokens,
		"actualTokens":            budget.ActualTokens,
		"shrinkLevel":             budget.ShrinkLevel,
		"truncated":               budget.Truncated,
		"rules":                   append([]string(nil), budget.Rules...),
		"sourceRefs":              append([]string(nil), budget.SourceRefs...),
		"includedSections":        append([]string(nil), budget.IncludedSections...),
		"normalizedSections":      append([]string(nil), budget.NormalizedSections...),
		"compressedSections":      append([]string(nil), budget.CompressedSections...),
		"omittedSections":         append([]string(nil), budget.OmittedSections...),
		"goalDoDCompressed":       budget.GoalDoDCompressed,
		"overBudget":              budget.OverBudget,
		"p0Overflow":              budget.P0Overflow,
		"profileMode":             budget.ProfileMode,
		"capabilitySource":        budget.CapabilitySource,
		"contextUtilizationRatio": budget.ContextUtilization,
		"maxRequestBytes":         budget.MaxRequestBytes,
		"stageBudgetRatio":        budget.StageBudgetRatio,
		"stageBudgetSource":       budget.StageBudgetSource,
		"tracePolicy":             budget.TracePolicy,
		"promptCaptured":          budget.PromptCaptured,
		"estimator":               budget.Estimator,
		"redactionApplied":        budget.RedactionApplied,
		"redactedPaths":           append([]string(nil), budget.RedactedPaths...),
		"protectedFields":         append([]string(nil), budget.ProtectedFields...),
		"integrityWarnings":       append([]string(nil), budget.IntegrityWarnings...),
		"contextGateAction":       budget.ContextGateAction,
		"contextGateReason":       budget.ContextGateReason,
	}
	if budget.CachePlan != nil {
		item["cachePlan"] = providerCachePlanOutput(budget.CachePlan)
	}
	return item
}

func providerCachePlanOutput(plan *llmmodels.ProviderCachePlan) map[string]any {
	if plan == nil {
		return nil
	}
	sectionHashes := make([]map[string]any, 0, len(plan.SectionHashes))
	for _, item := range plan.SectionHashes {
		sectionHashes = append(sectionHashes, map[string]any{
			"sectionId":          item.SectionID,
			"name":               item.Name,
			"cacheability":       item.Cacheability,
			"hash":               item.Hash,
			"sizeBytes":          item.SizeBytes,
			"sourceRefs":         append([]string(nil), item.SourceRefs...),
			"definitionRevision": item.DefinitionRevision,
		})
	}
	breakers := make([]map[string]any, 0, len(plan.CacheBreakers))
	for _, item := range plan.CacheBreakers {
		breakers = append(breakers, map[string]any{
			"sectionId": item.SectionID,
			"reason":    item.Reason,
		})
	}
	return map[string]any{
		"schemaVersion":           plan.SchemaVersion,
		"strategy":                plan.Strategy,
		"cacheKey":                plan.CacheKey,
		"stablePrefixHash":        plan.StablePrefixHash,
		"workspaceDefinitionHash": plan.WorkspaceDefinitionHash,
		"capabilityContractHash":  plan.CapabilityContractHash,
		"stageBundleHash":         plan.StageBundleHash,
		"volatileTailHash":        plan.VolatileTailHash,
		"largeRefsHash":           plan.LargeRefsHash,
		"sectionHashes":           sectionHashes,
		"cacheBreakers":           breakers,
		"canonicalization": map[string]any{
			"sectionOrderPolicy": plan.Canonicalization.SectionOrderPolicy,
			"sectionOrder":       append([]string(nil), plan.Canonicalization.SectionOrder...),
			"orderedDataPolicy":  plan.Canonicalization.OrderedDataPolicy,
			"mapKeyPolicy":       plan.Canonicalization.MapKeyPolicy,
			"providerMapping":    plan.Canonicalization.ProviderMapping,
		},
		"observedRequestShape": map[string]any{
			"providerFacingMode":  plan.ObservedRequestShape.ProviderFacingMode,
			"turnsHaveSectionIds": plan.ObservedRequestShape.TurnsHaveSectionIDs,
			"typedEventTape":      plan.ObservedRequestShape.TypedEventTape,
		},
		"adapterHint": map[string]any{
			"enabled":          plan.AdapterHint.Enabled,
			"mode":             plan.AdapterHint.Mode,
			"providerMapping":  plan.AdapterHint.ProviderMapping,
			"cacheablePrefix":  append([]string(nil), plan.AdapterHint.CacheablePrefix...),
			"volatileSections": append([]string(nil), plan.AdapterHint.VolatileSections...),
			"cacheBreakers":    append([]string(nil), plan.AdapterHint.CacheBreakers...),
		},
	}
}

func attachFailoverMetadata(output map[string]any, metadata map[string]any) {
	if output == nil || len(metadata) == 0 {
		return
	}
	output["llmFailover"] = cloneMap(metadata)
}

func attachLLMUsage(output map[string]any, usage map[string]any) {
	if output == nil || len(usage) == 0 {
		return
	}
	output["llmUsage"] = cloneMap(usage)
}

func buildLLMUsageOutput(
	request runtimemodel.StepExecutionRequest,
	call providerCallResult,
	operation string,
	startedAt time.Time,
	completedAt time.Time,
	failureClass string,
	status string,
) map[string]any {
	if status == "" {
		status = "completed"
	}
	requestBytes := 0
	inputTokens := 0
	outputTokens := 0
	totalTokens := 0
	estimated := true
	usageSource := llmcommon.UsageSourceEstimated
	responseBytes := len(call.payload)
	if responseBytes == 0 {
		responseBytes = streamResponseBytes(call.streamEvents)
	}
	contextBudget := providerContextBudgetOutput(call.request.ContextBudget, call.runtime.Config.ID, call.runtime.Config.Model)
	if call.request.ContextBudget != nil {
		requestBytes = call.request.ContextBudget.ActualBytes
		if requestBytes == 0 {
			requestBytes = call.request.ContextBudget.MaxBytes
		}
		inputTokens = call.request.ContextBudget.EstimatedInputTokens
	}
	if requestBytes == 0 {
		requestBytes = providerRequestBytes(call.request)
	}
	if inputTokens == 0 && requestBytes > 0 {
		inputTokens = estimateUsageTokensFromBytes(requestBytes, call.runtime.Config.ContextProfile.EstimatedTokensPerByte)
	}
	if call.usage != nil {
		estimated = false
		usageSource = llmcommon.UsageSourceProviderNative
		inputTokens = max(0, call.usage.InputTokens)
		outputTokens = max(0, call.usage.OutputTokens)
		totalTokens = max(0, call.usage.TotalTokens)
		if totalTokens == 0 && (inputTokens > 0 || outputTokens > 0) {
			totalTokens = inputTokens + outputTokens
		}
	} else if responseBytes > 0 {
		outputTokens = estimateUsageTokensFromBytes(responseBytes, call.runtime.Config.ContextProfile.EstimatedTokensPerByte)
	}
	if totalTokens == 0 && (inputTokens > 0 || outputTokens > 0) {
		totalTokens = inputTokens + outputTokens
	}
	cacheUsage := providerCacheUsageOutput(call, inputTokens)
	ret := map[string]any{
		"workspaceId":   string(request.WorkspaceID),
		"runId":         string(request.RunID),
		"stepId":        string(request.StepID),
		"providerId":    strings.TrimSpace(call.runtime.Config.ID),
		"provider":      strings.TrimSpace(call.runtime.Config.Provider),
		"model":         strings.TrimSpace(call.runtime.Config.Model),
		"operation":     strings.TrimSpace(operation),
		"inputTokens":   inputTokens,
		"outputTokens":  outputTokens,
		"totalTokens":   totalTokens,
		"estimated":     estimated,
		"usageSource":   usageSource,
		"requestBytes":  requestBytes,
		"responseBytes": responseBytes,
		"streaming":     len(call.streamEvents) > 0,
		"contextBudget": contextBudget,
		"startedAt":     startedAt.UTC().Format(time.RFC3339Nano),
		"completedAt":   completedAt.UTC().Format(time.RFC3339Nano),
		"durationMs":    int(completedAt.Sub(startedAt).Milliseconds()),
		"status":        status,
	}
	if len(cacheUsage) > 0 {
		ret["cacheUsage"] = cacheUsage
	}
	if strings.TrimSpace(failureClass) != "" {
		ret["failureClass"] = strings.TrimSpace(failureClass)
	}
	return ret
}

func providerCacheUsageOutput(call providerCallResult, inputTokens int) map[string]any {
	plan := providerCachePlanFromBudget(call.request.ContextBudget)
	cacheReadTokens := 0
	cacheWriteTokens := 0
	cacheCreationTokens := 0
	cacheableInputTokens := 0
	nonCacheableInputTokens := 0
	source := "cache_plan_estimated"
	estimated := true
	if call.usage != nil {
		cacheReadTokens = max(0, call.usage.CacheReadTokens)
		cacheWriteTokens = max(0, call.usage.CacheWriteTokens)
		cacheCreationTokens = max(0, call.usage.CacheCreationTokens)
		cacheableInputTokens = max(0, call.usage.CacheableInputTokens)
		nonCacheableInputTokens = max(0, call.usage.NonCacheableInputTokens)
		if cacheReadTokens > 0 || cacheWriteTokens > 0 || cacheCreationTokens > 0 || cacheableInputTokens > 0 || nonCacheableInputTokens > 0 {
			source = llmcommon.UsageSourceProviderNative
			estimated = false
		}
	}
	if cacheableInputTokens == 0 && plan != nil {
		cacheableInputTokens = estimateCacheableInputTokensFromPlan(plan, inputTokens)
	}
	if nonCacheableInputTokens == 0 && inputTokens > 0 && cacheableInputTokens > 0 {
		nonCacheableInputTokens = max(0, inputTokens-cacheableInputTokens)
	}
	if cacheReadTokens == 0 && cacheWriteTokens == 0 && cacheCreationTokens == 0 && cacheableInputTokens == 0 && nonCacheableInputTokens == 0 {
		return nil
	}
	ret := map[string]any{
		"estimated":               estimated,
		"source":                  source,
		"cacheReadTokens":         cacheReadTokens,
		"cacheWriteTokens":        cacheWriteTokens,
		"cacheCreationTokens":     cacheCreationTokens,
		"cacheableInputTokens":    cacheableInputTokens,
		"nonCacheableInputTokens": nonCacheableInputTokens,
	}
	if plan != nil {
		ret["cacheKey"] = plan.CacheKey
		ret["stablePrefixHash"] = plan.StablePrefixHash
		ret["workspaceDefinitionHash"] = plan.WorkspaceDefinitionHash
		ret["capabilityContractHash"] = plan.CapabilityContractHash
		ret["stageBundleHash"] = plan.StageBundleHash
		ret["volatileTailHash"] = plan.VolatileTailHash
		ret["cacheableSections"] = append([]string(nil), plan.AdapterHint.CacheablePrefix...)
		ret["volatileSections"] = append([]string(nil), plan.AdapterHint.VolatileSections...)
		ret["cacheBreakers"] = append([]string(nil), plan.AdapterHint.CacheBreakers...)
	}
	return ret
}

func providerCachePlanFromBudget(budget *llmmodels.ProviderContextBudget) *llmmodels.ProviderCachePlan {
	if budget == nil || budget.CachePlan == nil {
		return nil
	}
	return budget.CachePlan
}

func estimateCacheableInputTokensFromPlan(plan *llmmodels.ProviderCachePlan, inputTokens int) int {
	if plan == nil || inputTokens <= 0 || len(plan.SectionHashes) == 0 {
		return 0
	}
	cacheable := map[string]bool{}
	for _, sectionID := range plan.AdapterHint.CacheablePrefix {
		cacheable[strings.TrimSpace(sectionID)] = true
	}
	if len(cacheable) == 0 {
		return 0
	}
	cacheableBytes := 0
	totalBytes := 0
	for _, section := range plan.SectionHashes {
		size := providerCacheSectionSize(section)
		if size <= 0 {
			continue
		}
		totalBytes += size
		if cacheable[strings.TrimSpace(section.SectionID)] {
			cacheableBytes += size
		}
	}
	if totalBytes <= 0 || cacheableBytes <= 0 {
		return 0
	}
	estimated := int(float64(inputTokens) * float64(cacheableBytes) / float64(totalBytes))
	if estimated > inputTokens {
		return inputTokens
	}
	return estimated
}

func providerCacheSectionSize(section llmmodels.ProviderCacheSectionHash) int {
	if section.SizeBytes > 0 {
		return section.SizeBytes
	}
	return 1
}

func providerRequestBytes(request llmmodels.ProviderRequest) int {
	if request.WorkspaceID == "" && request.RunID == "" && request.StepID == "" && request.Kind == "" && request.Schema == "" && len(request.Input) == 0 {
		return 0
	}
	raw, err := json.Marshal(request)
	if err != nil {
		return 0
	}
	return len(raw)
}

func streamResponseBytes(events []llmmodels.ProviderStreamEvent) int {
	if len(events) == 0 {
		return 0
	}
	ret := 0
	for _, item := range events {
		ret += len(item.Delta)
	}
	return ret
}

func estimateUsageTokensFromBytes(bytes int, tokensPerByte float64) int {
	if bytes <= 0 {
		return 0
	}
	if tokensPerByte <= 0 {
		tokensPerByte = defaultEstimatedTokensPerByte
	}
	return int(math.Ceil(float64(bytes) * tokensPerByte))
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
	completion, err := provider.Complete(ctx, repairReq)
	payload := completion.Payload
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
	defaultProviderID, providers, err := a.providerRuntimes()
	if err != nil {
		providerID := requestedProviderID(request.Input, defaultProviderID)
		return ProviderRuntime{}, providerID, err
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
	defaultProviderID, providers, err := a.providerRuntimes()
	if err != nil {
		return ProviderRuntime{}, strings.TrimSpace(providerID), err
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

func (a *RuntimeAdapter) providerRuntimes() (string, map[string]ProviderRuntime, error) {
	defaultProviderID := a.defaultProviderID
	providers := a.providers
	if a.configLoader == nil {
		return defaultProviderID, providers, nil
	}
	if a.configRevision != nil {
		revision := a.configRevision()
		a.cacheMu.RLock()
		if a.cache.ready && a.cache.revision == revision {
			cachedDefaultProviderID := a.cache.defaultProviderID
			cachedProviders := a.cache.providers
			a.cacheMu.RUnlock()
			return cachedDefaultProviderID, cachedProviders, nil
		}
		a.cacheMu.RUnlock()

		a.cacheMu.Lock()
		defer a.cacheMu.Unlock()
		if a.cache.ready && a.cache.revision == revision {
			return a.cache.defaultProviderID, a.cache.providers, nil
		}
		loadedDefaultProviderID, configs, err := a.configLoader()
		if err != nil {
			return defaultProviderID, nil, err
		}
		loadedDefaultProviderID = normalizedRuntimeDefaultProviderID(loadedDefaultProviderID)
		loadedProviders := providerRuntimesFromConfigsWithFactory(configs, a.providerFactory)
		a.cache = providerRuntimeCache{
			ready:             true,
			revision:          revision,
			defaultProviderID: loadedDefaultProviderID,
			providers:         loadedProviders,
		}
		return loadedDefaultProviderID, loadedProviders, nil
	}
	loadedDefaultProviderID, configs, err := a.configLoader()
	if err != nil {
		return defaultProviderID, nil, err
	}
	defaultProviderID = normalizedRuntimeDefaultProviderID(loadedDefaultProviderID)
	return defaultProviderID, providerRuntimesFromConfigsWithFactory(configs, a.providerFactory), nil
}

func normalizedRuntimeDefaultProviderID(providerID string) string {
	providerID = strings.TrimSpace(providerID)
	if providerID == "" {
		return llmcommon.DefaultProviderID
	}
	return providerID
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
		repaired := repairMalformedShellCommandArgsJSON(normalized)
		repaired = repairMalformedKnownDecisionKeyJSON(repaired)
		repaired = repairMalformedQuotedObjectElementsJSON(repaired)
		if bytes.Equal(repaired, normalized) {
			return llmmodels.LLMExecutionDecision{}, fmt.Errorf("%w: invalid llm decision json: %v", common.ErrInvalidArgument, err)
		}
		normalized = repaired
		decoder = json.NewDecoder(bytes.NewReader(normalized))
		if repairErr := decoder.Decode(&raw); repairErr != nil {
			return llmmodels.LLMExecutionDecision{}, fmt.Errorf("%w: invalid llm decision json: %v", common.ErrInvalidArgument, err)
		}
	}
	var extra any
	if err := decoder.Decode(&extra); err == nil {
		return llmmodels.LLMExecutionDecision{}, fmt.Errorf("%w: invalid llm decision json: multiple json values", common.ErrInvalidArgument)
	} else if err != io.EOF {
		return llmmodels.LLMExecutionDecision{}, fmt.Errorf("%w: invalid llm decision json: %v", common.ErrInvalidArgument, err)
	}
	normalized, err = normalizeLLMDecisionSchemaPayload(normalized)
	if err != nil {
		return llmmodels.LLMExecutionDecision{}, err
	}
	raw = nil
	decoder = json.NewDecoder(bytes.NewReader(normalized))
	if err := decoder.Decode(&raw); err != nil {
		return llmmodels.LLMExecutionDecision{}, fmt.Errorf("%w: invalid llm decision json: %v", common.ErrInvalidArgument, err)
	}
	if err := validateLLMDecisionTopLevelFields(raw); err != nil {
		return llmmodels.LLMExecutionDecision{}, err
	}
	var decision llmmodels.LLMExecutionDecision
	if err := json.Unmarshal(normalized, &decision); err != nil {
		return llmmodels.LLMExecutionDecision{}, fmt.Errorf("%w: invalid llm decision json: %v", common.ErrInvalidArgument, err)
	}
	normalizeDecisionExecutionGraph(&decision)
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

func normalizeLLMDecisionSchemaPayload(payload []byte) ([]byte, error) {
	var doc map[string]any
	if err := json.Unmarshal(payload, &doc); err != nil {
		return payload, nil
	}
	changed := false
	if value, ok := doc["plan"]; ok {
		if plan := advisoryDecisionPlanValue(value); len(plan) > 0 {
			doc["plan"] = plan
			changed = true
		}
	}
	if value, ok := doc["nextAction"]; ok {
		if normalized, keep := normalizedNextActionValue(value); keep {
			doc["nextAction"] = normalized
			changed = true
		} else {
			delete(doc, "nextAction")
			changed = true
		}
	}
	if value, ok := doc["validationPlan"]; ok {
		if _, hasPlan := doc["plan"]; !hasPlan {
			if plan := advisoryDecisionPlanValue(value); len(plan) > 0 {
				doc["plan"] = plan
			}
		}
		delete(doc, "validationPlan")
		changed = true
	}
	if value, ok := doc["strategyReassessment"]; ok {
		if attachTopLevelStrategyReassessmentToPatchNodes(doc, value) {
			delete(doc, "strategyReassessment")
			changed = true
		}
	}
	if flattenNestedStrategyReassessmentInPatchNodes(doc) {
		changed = true
	}
	assessment, ok := doc["dodAssessment"].(map[string]any)
	if !ok || len(assessment) == 0 {
		if !changed {
			return payload, nil
		}
		normalized, err := json.Marshal(doc)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid llm decision json: %v", common.ErrInvalidArgument, err)
		}
		return normalized, nil
	}
	for _, key := range []string{"goalCoverage", "acceptanceDimensions", "deliverableCoverage"} {
		if value, ok := doc[key]; ok {
			if _, exists := assessment[key]; !exists {
				assessment[key] = value
			}
			delete(doc, key)
			changed = true
		}
		if normalized, ok := normalizeDecisionCoverageValue(assessment[key]); ok {
			assessment[key] = normalized
			changed = true
		}
	}
	if !changed {
		return payload, nil
	}
	doc["dodAssessment"] = assessment
	normalized, err := json.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid llm decision json: %v", common.ErrInvalidArgument, err)
	}
	return normalized, nil
}

func normalizeDecisionCoverageValue(value any) (map[string]any, bool) {
	if items, ok := value.([]any); ok {
		return map[string]any{
			"satisfied": coverageItemsSatisfied(items),
			"items":     items,
		}, true
	}
	coverage, ok := value.(map[string]any)
	if !ok || len(coverage) == 0 {
		return nil, false
	}
	itemsContainer, ok := coverage["items"].(map[string]any)
	if !ok {
		return nil, false
	}
	items, ok := itemsContainer["items"].([]any)
	if !ok {
		return nil, false
	}
	coverage["items"] = items
	if _, ok := coverage["satisfied"]; !ok {
		coverage["satisfied"] = coverageItemsSatisfied(items)
	}
	return coverage, true
}

func attachTopLevelStrategyReassessmentToPatchNodes(doc map[string]any, value any) bool {
	reassessment, ok := value.(map[string]any)
	if !ok || len(reassessment) == 0 {
		return false
	}
	graph, ok := doc["executionGraph"].(map[string]any)
	if !ok || len(graph) == 0 {
		return false
	}
	nodes, ok := graph["nodes"].([]any)
	if !ok || len(nodes) == 0 {
		return false
	}
	attached := false
	for _, item := range nodes {
		node, ok := item.(map[string]any)
		if !ok || strings.TrimSpace(stringValue(node["kind"])) != "patch/apply" {
			continue
		}
		input, ok := node["input"].(map[string]any)
		if !ok {
			input = map[string]any{}
			node["input"] = input
		}
		for key, reassessmentValue := range reassessment {
			key = strings.TrimSpace(key)
			if key == "" {
				continue
			}
			if _, exists := input[key]; exists {
				continue
			}
			input[key] = reassessmentValue
		}
		attached = true
	}
	if attached {
		doc["executionGraph"] = graph
	}
	return attached
}

func flattenNestedStrategyReassessmentInPatchNodes(doc map[string]any) bool {
	graph, ok := doc["executionGraph"].(map[string]any)
	if !ok || len(graph) == 0 {
		return false
	}
	nodes, ok := graph["nodes"].([]any)
	if !ok || len(nodes) == 0 {
		return false
	}
	changed := false
	for _, item := range nodes {
		node, ok := item.(map[string]any)
		if !ok || strings.TrimSpace(stringValue(node["kind"])) != "patch/apply" {
			continue
		}
		input, ok := node["input"].(map[string]any)
		if !ok || len(input) == 0 {
			continue
		}
		nested, ok := input["strategyReassessment"].(map[string]any)
		if !ok || len(nested) == 0 {
			continue
		}
		for key, value := range nested {
			key = strings.TrimSpace(key)
			if key == "" {
				continue
			}
			if _, exists := input[key]; exists {
				continue
			}
			input[key] = value
			changed = true
		}
	}
	if changed {
		doc["executionGraph"] = graph
	}
	return changed
}

func normalizedNextActionValue(value any) (map[string]any, bool) {
	switch typed := value.(type) {
	case map[string]any:
		kind := strings.TrimSpace(stringValue(typed["kind"]))
		if kind == "" {
			return nil, false
		}
		if typed["kind"] != kind {
			typed["kind"] = kind
		}
		return typed, true
	case string:
		kind := strings.TrimSpace(typed)
		if kind == "" {
			return nil, false
		}
		return map[string]any{"kind": kind}, true
	default:
		return nil, false
	}
}

func advisoryDecisionPlanValue(value any) []string {
	switch typed := value.(type) {
	case string:
		if text := strings.TrimSpace(typed); text != "" {
			return []string{text}
		}
	case []any:
		ret := make([]string, 0, len(typed))
		for _, item := range typed {
			switch v := item.(type) {
			case string:
				if text := strings.TrimSpace(v); text != "" {
					ret = append(ret, text)
				}
			default:
				if encoded, err := json.Marshal(v); err == nil && len(encoded) > 0 {
					ret = append(ret, string(encoded))
				}
			}
		}
		return ret
	case []string:
		ret := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := strings.TrimSpace(item); text != "" {
				ret = append(ret, text)
			}
		}
		return ret
	default:
		if encoded, err := json.Marshal(typed); err == nil && len(encoded) > 0 && string(encoded) != "null" {
			return []string{string(encoded)}
		}
	}
	return nil
}

func coverageItemsSatisfied(items []any) bool {
	if len(items) == 0 {
		return false
	}
	for _, item := range items {
		entry, ok := item.(map[string]any)
		if !ok {
			return false
		}
		if !strings.EqualFold(strings.TrimSpace(stringValue(entry["state"])), "satisfied") {
			return false
		}
	}
	return true
}

func NormalizeLLMDecisionPayload(payload []byte) ([]byte, error) {
	text := strings.TrimSpace(strings.TrimPrefix(string(payload), "\ufeff"))
	if text == "" {
		return nil, fmt.Errorf("%w: invalid llm decision json: empty payload", common.ErrInvalidArgument)
	}
	if strings.HasPrefix(text, "```") {
		text = stripMarkdownFence(text)
	}
	text = string(repairMalformedKnownDecisionKeyJSON([]byte(text)))
	text = string(repairMalformedQuotedObjectElementsJSON([]byte(text)))
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

func repairMalformedShellCommandArgsJSON(payload []byte) []byte {
	text := string(payload)
	var builder strings.Builder
	last := 0
	searchFrom := 0
	for {
		idx := strings.Index(text[searchFrom:], `"command"`)
		if idx < 0 {
			break
		}
		idx += searchFrom
		start, end, replacement, ok := malformedShellCommandArgsReplacement(text, idx)
		if !ok {
			searchFrom = idx + len(`"command"`)
			continue
		}
		builder.WriteString(text[last:start])
		builder.WriteString(replacement)
		last = end
		searchFrom = end
	}
	if last == 0 {
		return payload
	}
	builder.WriteString(text[last:])
	return []byte(builder.String())
}

func repairMalformedKnownDecisionKeyJSON(payload []byte) []byte {
	text := string(payload)
	keys := map[string]bool{
		"acceptanceDimensions":  true,
		"action":                true,
		"approvalId":            true,
		"args":                  true,
		"artifactRefs":          true,
		"baseHash":              true,
		"command":               true,
		"completionState":       true,
		"content":               true,
		"context":               true,
		"coverageTargets":       true,
		"deliverable":           true,
		"deliverableCoverage":   true,
		"diff":                  true,
		"dodAssessment":         true,
		"evidenceRefs":          true,
		"executionGraph":        true,
		"goal":                  true,
		"goalCoverage":          true,
		"goalItem":              true,
		"goalOutcome":           true,
		"id":                    true,
		"implementationSummary": true,
		"impact":                true,
		"input":                 true,
		"item":                  true,
		"items":                 true,
		"knownLimitations":      true,
		"kind":                  true,
		"mcpToolCalls":          true,
		"memoryType":            true,
		"memoryWrites":          true,
		"name":                  true,
		"nextAction":            true,
		"nextActions":           true,
		"nodes":                 true,
		"operation":             true,
		"patchRequests":         true,
		"patchType":             true,
		"path":                  true,
		"plan":                  true,
		"reason":                true,
		"requiredApproval":      true,
		"requiredDecision":      true,
		"requirement":           true,
		"residualRisks":         true,
		"satisfied":             true,
		"serverId":              true,
		"shellCommands":         true,
		"state":                 true,
		"summary":               true,
		"tags":                  true,
		"targetRef":             true,
		"thoughtSummary":        true,
		"tool":                  true,
		"verificationSummary":   true,
	}
	var builder strings.Builder
	changed := false
	inString := false
	escaped := false
	for i := 0; i < len(text); {
		ch := text[i]
		if inString {
			builder.WriteByte(ch)
			if escaped {
				escaped = false
			} else if ch == '\\' {
				escaped = true
			} else if ch == '"' {
				inString = false
			}
			i++
			continue
		}
		if ch == '"' {
			inString = true
			builder.WriteByte(ch)
			i++
			continue
		}
		if ch != ',' && ch != '{' {
			builder.WriteByte(ch)
			i++
			continue
		}
		builder.WriteByte(ch)
		i++
		for i < len(text) && unicode.IsSpace(rune(text[i])) {
			builder.WriteByte(text[i])
			i++
		}
		start := i
		for i < len(text) && (text[i] == '_' || text[i] >= 'a' && text[i] <= 'z' || text[i] >= 'A' && text[i] <= 'Z' || text[i] >= '0' && text[i] <= '9') {
			i++
		}
		if start == i || i >= len(text) || text[i] != '"' {
			builder.WriteString(text[start:i])
			continue
		}
		key := text[start:i]
		quoteEnd := i + 1
		j := quoteEnd
		for j < len(text) && unicode.IsSpace(rune(text[j])) {
			j++
		}
		if j >= len(text) || text[j] != ':' || !keys[key] {
			builder.WriteString(text[start:i])
			continue
		}
		builder.WriteByte('"')
		builder.WriteString(key)
		builder.WriteByte('"')
		i = quoteEnd
		changed = true
	}
	if !changed {
		return payload
	}
	return []byte(builder.String())
}

func repairMalformedQuotedObjectElementsJSON(payload []byte) []byte {
	text := string(payload)
	var builder strings.Builder
	changed := false
	inString := false
	escaped := false
	last := 0
	for i := 0; i < len(text); i++ {
		ch := text[i]
		if inString {
			if escaped {
				escaped = false
			} else if ch == '\\' {
				escaped = true
			} else if ch == '"' {
				inString = false
			}
			continue
		}
		if ch == '"' {
			if i+1 < len(text) && text[i+1] == '{' && quotedObjectElementContext(text, i) {
				objectEnd, ok := findJSONClosingDelimiter(text, i+1, '{', '}')
				if !ok {
					inString = true
					continue
				}
				afterObject := skipJSONSpaces(text, objectEnd+1)
				resume := objectEnd + 1
				if afterObject < len(text) && text[afterObject] == '"' {
					next := skipJSONSpaces(text, afterObject+1)
					if next < len(text) && (text[next] == ',' || text[next] == ']' || text[next] == '}') {
						resume = afterObject + 1
					}
				} else if afterObject < len(text) && (text[afterObject] == ',' || text[afterObject] == ']' || text[afterObject] == '}') {
					resume = objectEnd + 1
				} else {
					inString = true
					continue
				}
				builder.WriteString(text[last:i])
				builder.WriteString(text[i+1 : objectEnd+1])
				last = resume
				i = resume - 1
				changed = true
				continue
			}
			inString = true
		}
	}
	if !changed {
		return payload
	}
	builder.WriteString(text[last:])
	return []byte(builder.String())
}

func quotedObjectElementContext(text string, quoteStart int) bool {
	prev := quoteStart - 1
	for prev >= 0 {
		switch text[prev] {
		case ' ', '\n', '\r', '\t':
			prev--
			continue
		case '[', ',':
			return true
		default:
			return false
		}
	}
	return false
}

func malformedShellCommandArgsReplacement(text string, commandKeyStart int) (int, int, string, bool) {
	keyEnd, ok := scanJSONStringEnd(text, commandKeyStart)
	if !ok {
		return 0, 0, "", false
	}
	key, err := strconv.Unquote(text[commandKeyStart:keyEnd])
	if err != nil || key != "command" {
		return 0, 0, "", false
	}
	pos := skipJSONSpaces(text, keyEnd)
	if pos >= len(text) || text[pos] != ':' {
		return 0, 0, "", false
	}
	pos = skipJSONSpaces(text, pos+1)
	commandValueEnd, ok := scanJSONStringEnd(text, pos)
	if !ok {
		return 0, 0, "", false
	}
	firstComma := skipJSONSpaces(text, commandValueEnd)
	if firstComma >= len(text) || text[firstComma] != ',' {
		return 0, 0, "", false
	}
	pos = skipJSONSpaces(text, firstComma+1)
	tokenEnd, ok := scanJSONStringEnd(text, pos)
	if !ok {
		return 0, 0, "", false
	}
	afterToken := skipJSONSpaces(text, tokenEnd)
	if afterToken < len(text) && text[afterToken] == ':' {
		return 0, 0, "", false
	}

	extras := []string{}
	for {
		tokenEnd, ok := scanJSONStringEnd(text, pos)
		if !ok {
			return 0, 0, "", false
		}
		token := text[pos:tokenEnd]
		afterToken = skipJSONSpaces(text, tokenEnd)
		if afterToken < len(text) && text[afterToken] == ':' {
			key, err := strconv.Unquote(token)
			if err != nil || len(extras) == 0 {
				return 0, 0, "", false
			}
			if key == "args" {
				arrayStart := skipJSONSpaces(text, afterToken+1)
				if arrayStart >= len(text) || text[arrayStart] != '[' {
					return 0, 0, "", false
				}
				arrayEnd, ok := findJSONClosingDelimiter(text, arrayStart, '[', ']')
				if !ok {
					return 0, 0, "", false
				}
				return firstComma, arrayEnd + 1, "," + shellArgsArrayWithExisting(extras, text[arrayStart+1:arrayEnd]), true
			}
			return firstComma, pos, "," + shellArgsArray(extras) + ",", true
		}
		extras = append(extras, token)
		if afterToken >= len(text) {
			return 0, 0, "", false
		}
		switch text[afterToken] {
		case ',':
			pos = skipJSONSpaces(text, afterToken+1)
		case ']':
			return firstComma, afterToken + 1, "," + shellArgsArray(extras), true
		case '}':
			return firstComma, afterToken + 1, "," + shellArgsArray(extras) + "}", true
		default:
			return 0, 0, "", false
		}
	}
}

func shellArgsArray(args []string) string {
	return `"args":[` + strings.Join(args, ",") + `]`
}

func shellArgsArrayWithExisting(args []string, existing string) string {
	existing = strings.TrimSpace(existing)
	if existing == "" {
		return shellArgsArray(args)
	}
	return `"args":[` + strings.Join(args, ",") + "," + existing + `]`
}

func scanJSONStringEnd(text string, start int) (int, bool) {
	if start >= len(text) || text[start] != '"' {
		return 0, false
	}
	escaped := false
	for idx := start + 1; idx < len(text); idx++ {
		ch := text[idx]
		if escaped {
			escaped = false
			continue
		}
		if ch == '\\' {
			escaped = true
			continue
		}
		if ch == '"' {
			return idx + 1, true
		}
	}
	return 0, false
}

func skipJSONSpaces(text string, start int) int {
	for start < len(text) {
		switch text[start] {
		case ' ', '\n', '\r', '\t':
			start++
		default:
			return start
		}
	}
	return start
}

func findJSONClosingDelimiter(text string, start int, open byte, close byte) (int, bool) {
	if start >= len(text) || text[start] != open {
		return 0, false
	}
	depth := 0
	inString := false
	escaped := false
	for idx := start; idx < len(text); idx++ {
		ch := text[idx]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}
		switch ch {
		case '"':
			inString = true
		case open:
			depth++
		case close:
			depth--
			if depth == 0 {
				return idx, true
			}
		}
	}
	return 0, false
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
	if decision.CompletionState == "failed" && decision.DoDAssessment != nil {
		output["stage"] = "llm_dod_assessment"
		output["dodAssessment"] = doDAssessmentOutput(decision.DoDAssessment)
		return output, nil
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
		if strings.TrimSpace(decision.RequiredApproval.Action) != "" {
			output[outputKeyBlockApprovalAction] = decision.RequiredApproval.Action
		}
		if len(decision.RequiredApproval.Context) > 0 {
			output[outputKeyBlockApprovalContext] = cloneMap(decision.RequiredApproval.Context)
		}
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
		if isReservedLLMReportPatchTarget(patch.TargetRef) {
			return fmt.Errorf("%w: patchRequests.targetRef %q is reserved for runtime governance/report artifacts", common.ErrInvalidArgument, patch.TargetRef)
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
	allowedKinds := declaredRuntimeBlockKinds()
	known := map[string]bool{}
	for _, node := range graph.Nodes {
		nodeID := strings.TrimSpace(node.ID)
		if nodeID == "" || strings.TrimSpace(node.Kind) == "" {
			return fmt.Errorf("%w: executionGraph node id and kind are required", common.ErrInvalidArgument)
		}
		if !allowedKinds[strings.TrimSpace(node.Kind)] {
			return fmt.Errorf("%w: executionGraph node kind %q is not declared in runtimeCapabilities.blockKinds", common.ErrInvalidArgument, strings.TrimSpace(node.Kind))
		}
		if strings.TrimSpace(node.Kind) == shellcommon.StepKindCommand {
			if command, _ := node.Input["command"].(string); strings.TrimSpace(command) != "" {
				if err := validateShellCommandBoundary(command, stringSliceValue(node.Input["args"]), "executionGraph shell command"); err != nil {
					return err
				}
			}
		}
		switch strings.TrimSpace(node.Kind) {
		case filecommon.StepKindRead:
			path, _ := node.Input["path"].(string)
			if strings.TrimSpace(path) == "" {
				return fmt.Errorf("%w: executionGraph file/read path is required", common.ErrInvalidArgument)
			}
		case filecommon.StepKindEdit:
			path, _ := node.Input["path"].(string)
			oldText, _ := node.Input["oldText"].(string)
			if strings.TrimSpace(path) == "" || oldText == "" {
				return fmt.Errorf("%w: executionGraph file/edit path and oldText are required", common.ErrInvalidArgument)
			}
		case filecommon.StepKindList:
			if path, ok := node.Input["path"].(string); ok && filepath.IsAbs(strings.TrimSpace(path)) {
				return fmt.Errorf("%w: executionGraph file/list path must be workspace relative", common.ErrInvalidArgument)
			}
		case llmcommon.RuntimeBlockKindPatchApply, llmcommon.RuntimeBlockKindPatchPreview:
			targetRef, _ := node.Input["targetRef"].(string)
			if strings.TrimSpace(targetRef) == "" {
				return fmt.Errorf("%w: executionGraph %s targetRef is required", common.ErrInvalidArgument, strings.TrimSpace(node.Kind))
			}
			if isReservedLLMReportPatchTarget(targetRef) {
				return fmt.Errorf("%w: executionGraph %s targetRef %q is reserved for runtime governance/report artifacts", common.ErrInvalidArgument, strings.TrimSpace(node.Kind), targetRef)
			}
		case llmcommon.RuntimeBlockKindPatchRevert:
			patchID, _ := node.Input["patchId"].(string)
			if strings.TrimSpace(patchID) == "" {
				return fmt.Errorf("%w: executionGraph patch/revert patchId is required", common.ErrInvalidArgument)
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
	if graphHasPatchApplyDependingOnFileRead(graph) {
		return fmt.Errorf("%w: executionGraph patch/apply must not depend on same-graph file/read; read current content in an earlier turn and submit a separate state-changing graph", common.ErrInvalidArgument)
	}
	return nil
}

func graphHasPatchApplyDependingOnFileRead(graph llmmodels.BlockExecutionGraph) bool {
	nodes := make(map[string]llmmodels.BlockExecutionNode, len(graph.Nodes))
	for _, node := range graph.Nodes {
		nodeID := strings.TrimSpace(node.ID)
		if nodeID != "" {
			nodes[nodeID] = node
		}
	}
	for _, node := range graph.Nodes {
		if strings.TrimSpace(node.Kind) != llmcommon.RuntimeBlockKindPatchApply {
			continue
		}
		if graphNodeHasAncestorBlockKind(node, nodes, filecommon.StepKindRead, map[string]bool{}) {
			return true
		}
	}
	return false
}

func graphNodeHasAncestorBlockKind(node llmmodels.BlockExecutionNode, nodes map[string]llmmodels.BlockExecutionNode, kind string, seen map[string]bool) bool {
	for _, dependency := range node.DependsOn {
		dependency = strings.TrimSpace(dependency)
		if dependency == "" || seen[dependency] {
			continue
		}
		seen[dependency] = true
		parent, ok := nodes[dependency]
		if !ok {
			continue
		}
		if strings.TrimSpace(parent.Kind) == kind {
			return true
		}
		if graphNodeHasAncestorBlockKind(parent, nodes, kind, seen) {
			return true
		}
	}
	return false
}

func normalizeDecisionExecutionGraph(decision *llmmodels.LLMExecutionDecision) {
	if decision == nil || decision.ExecutionGraph == nil {
		return
	}
	for idx := range decision.ExecutionGraph.Nodes {
		switch strings.TrimSpace(decision.ExecutionGraph.Nodes[idx].Kind) {
		case "patchRequests", "patch/request":
			decision.ExecutionGraph.Nodes[idx].Kind = llmcommon.RuntimeBlockKindPatchApply
		}
	}
}

func declaredRuntimeBlockKinds() map[string]bool {
	capabilities := declaredRuntimeCapabilities()
	ret := make(map[string]bool, len(capabilities.BlockKinds))
	for _, block := range capabilities.BlockKinds {
		if kind := strings.TrimSpace(block.Kind); kind != "" {
			ret[kind] = true
		}
	}
	return ret
}

func validateShellCommandBoundary(command string, args []string, field string) error {
	command = strings.TrimSpace(command)
	if reason := invalidDirectShellCommandNameReason(command); reason != "" {
		return fmt.Errorf("%w: %s command must be an executable name or path only; %s; use args, sh -c, or patchRequests for shell syntax", common.ErrInvalidArgument, field, reason)
	}
	commandLine := shellCommandLine(command, args)
	if absoluteWorkspaceCD := absoluteWorkspaceCDReference(commandLine); absoluteWorkspaceCD != "" {
		return fmt.Errorf("%w: %s must not hard-code workspace directory change %q; shell/command already runs in the workspace deliverable directory", common.ErrInvalidArgument, field, absoluteWorkspaceCD)
	}
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

func invalidDirectShellCommandNameReason(command string) string {
	if command == "" {
		return ""
	}
	if strings.ContainsAny(command, " \t\r\n") {
		return "command contains whitespace"
	}
	if strings.ContainsAny(command, "<>|&;`$") {
		return "command contains shell metacharacters"
	}
	return ""
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

func absoluteWorkspaceCDReference(commandLine string) string {
	commandLine = strings.TrimSpace(commandLine)
	if commandLine == "" {
		return ""
	}
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(?:^|[\s;&|()])cd\s+(/home/user/workspace)(?:$|[\s;&|()])`),
		regexp.MustCompile(`(?:^|[\s;&|()])cd\s+(/[^\s;&|()]*\.workorch-workspaces/[^\s;&|()]+)(?:$|[\s;&|()])`),
	}
	for _, pattern := range patterns {
		match := pattern.FindStringSubmatch(commandLine)
		if len(match) > 1 {
			return strings.TrimSpace(match[1])
		}
	}
	return ""
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
	systemLocale := currentSystemLocale()
	dialogLanguage := inferDialogLanguage(input, systemLocale)
	providerReq := llmmodels.ProviderRequest{
		WorkspaceID:         string(request.WorkspaceID),
		RunID:               string(request.RunID),
		StepID:              string(request.StepID),
		Kind:                request.Kind,
		Model:               config.Model,
		SystemLocale:        systemLocale,
		DialogLanguage:      dialogLanguage,
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
	providerReq = enforceProviderRequestBudget(providerReq)
	providerReq = redactProviderRequestForLLM(providerReq)
	providerReq.PromptContext = buildPromptContextFromRequest(providerReq)
	if providerReq.ContextBudget != nil && providerReq.PromptContext != nil && providerReq.PromptContext.Normalization != nil {
		providerReq.ContextBudget.NormalizedSections = uniqueStrings(append(providerReq.ContextBudget.NormalizedSections, providerReq.PromptContext.Normalization.NormalizedSections...))
		providerReq.ContextBudget.IncludedSections = uniqueStrings(append(providerReq.ContextBudget.IncludedSections, promptContextSectionIDs(providerReq.PromptContext)...))
	}
	attachProviderCachePlan(&providerReq)
	providerReq = applyProviderPromptContractGate(providerReq)
	return providerReq
}

func applyProviderPromptContractGate(request llmmodels.ProviderRequest) llmmodels.ProviderRequest {
	if request.ContextBudget == nil {
		return request
	}
	warnings := providerPromptContractWarnings(request)
	if len(warnings) == 0 {
		return request
	}
	request.ContextBudget.IntegrityWarnings = uniqueStrings(append(request.ContextBudget.IntegrityWarnings, warnings...))
	if strings.TrimSpace(request.ContextBudget.ContextGateAction) == "" || request.ContextBudget.ContextGateAction == providerContextGatePass {
		request.ContextBudget.ContextGateAction = providerContextGateFailFast
		request.ContextBudget.ContextGateReason = warnings[0]
	}
	return request
}

func providerPromptContractWarnings(request llmmodels.ProviderRequest) []string {
	if !stateChangingDecisionRequest(request.Input) {
		return nil
	}
	currentAction := currentActionPromptContent(request.PromptContext)
	if currentAction == "" {
		return []string{"state_changing_current_action_missing"}
	}
	warnings := []string{}
	lowerAction := strings.ToLower(currentAction)
	if strings.Contains(lowerAction, `"type":"plan_next_step"`) ||
		strings.Contains(lowerAction, `"type": "plan_next_step"`) {
		warnings = append(warnings, "state_changing_current_action_conflicts_with_plan_next_step")
	}
	required := stateChangingDecisionRequiredOutput(request.Input)
	for _, accepted := range stringSliceFromValue(required["acceptedKinds"]) {
		if accepted == "" {
			continue
		}
		if !strings.Contains(currentAction, accepted) {
			warnings = append(warnings, "state_changing_current_action_missing_accepted_kind")
			break
		}
	}
	return uniqueStrings(warnings)
}

func currentActionPromptContent(context *llmmodels.PromptContext) string {
	if context == nil {
		return ""
	}
	for _, turn := range context.Turns {
		if strings.Contains(turn.Content, "Current Requested Action") {
			return turn.Content
		}
	}
	return ""
}

func isAssessmentOnlyDecisionRequest(request llmmodels.ProviderRequest) bool {
	if !isDecisionSchemaRequest(request) {
		return false
	}
	return boolFromProviderInput(request.Input, "assessmentOnly") ||
		boolFromProviderInput(request.Input, "graphExpansionLimitFollowUp") ||
		boolFromProviderInput(request.Input, "providerRetryAssessmentOnly") ||
		boolFromProviderInput(request.Input, "assessmentOnlyContractCorrection")
}

func boolFromProviderInput(input map[string]any, key string) bool {
	if input == nil {
		return false
	}
	switch typed := input[key].(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(strings.TrimSpace(typed), "true")
	default:
		return false
	}
}

func promptContextSectionIDs(context *llmmodels.PromptContext) []string {
	if context == nil || len(context.Sections) == 0 {
		return nil
	}
	out := make([]string, 0, len(context.Sections))
	for _, item := range context.Sections {
		sectionID := strings.TrimSpace(item.ID)
		if sectionID == "" {
			continue
		}
		out = append(out, sectionID)
	}
	return uniqueStrings(out)
}

func currentSystemLocale() string {
	for _, key := range []string{"LC_ALL", "LC_MESSAGES", "LANGUAGE", "LANG"} {
		value := strings.TrimSpace(os.Getenv(key))
		if value == "" {
			continue
		}
		if key == "LANGUAGE" {
			value = strings.TrimSpace(strings.Split(value, ":")[0])
		}
		if value != "" {
			return value
		}
	}
	return "C"
}

func inferDialogLanguage(input map[string]any, systemLocale string) string {
	for _, value := range []any{
		input["dialogLanguage"],
		input["language"],
		input["locale"],
	} {
		if language := languageNameFromLocale(stringValue(value)); language != "" {
			return language
		}
	}
	executionContext := recordFromAny(input["executionContext"])
	for _, value := range []any{
		executionContext["dialogLanguage"],
		executionContext["language"],
		executionContext["locale"],
		executionContext["systemLanguage"],
		executionContext["systemLocale"],
	} {
		if language := languageNameFromLocale(stringValue(value)); language != "" {
			return language
		}
	}
	if language := languageNameFromLocale(systemLocale); language != "" {
		return language
	}
	return "English"
}

func languageNameFromLocale(locale string) string {
	if strings.TrimSpace(locale) == "" {
		return ""
	}
	return dialoglanguage.DisplayName(locale)
}

func enrichProviderInputWithSoulRouting(input map[string]any) {
	if len(input) == 0 {
		return
	}
	if stage := strings.TrimSpace(stringValue(input["runtimeStage"])); stage != "" {
		input["currentRuntimeStage"] = stage
	}
	if mode := strings.TrimSpace(stringValue(input["runtimeStageMode"])); mode != "" {
		input["currentRuntimeStageMode"] = mode
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
	if stage := strings.TrimSpace(stringValue(input["runtimeStage"])); stage != "" {
		switch stage {
		case "planning":
			return "planning"
		case "execution", "repair":
			return "execution"
		case "verification", "acceptance":
			return "validation"
		case "decision":
			return "decision"
		case "approval":
			return "approval"
		}
	}
	if stage := normalizedProviderSoulStage(input["currentSoulStage"]); stage != "" {
		return stage
	}
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

func normalizedProviderSoulStage(value any) string {
	stage := strings.ToLower(strings.TrimSpace(stringValue(value)))
	if stage == "" {
		return ""
	}
	if _, ok := providerSoulStages[stage]; !ok {
		return ""
	}
	return stage
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

func stringSliceFromValue(value any) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		ret := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := strings.TrimSpace(stringValue(item)); text != "" {
				ret = append(ret, text)
			}
		}
		return ret
	default:
		return nil
	}
}

func declaredRuntimeCapabilities() llmmodels.RuntimeCapabilities {
	blockKinds := []llmmodels.RuntimeBlockCapability{
		{
			Kind:        llmcommon.StepKindWorkspaceExecution,
			Description: "Plan the next workspace execution step from workspace goal, DoD, memory, skills, and MCP context.",
		},
		{
			Kind:        shellcommon.StepKindCommand,
			Description: "Execute one bounded local shell command in the workspace deliverable directory.",
			Input:       map[string]any{"command": "string", "args": "[]string"},
		},
		{
			Kind:        filecommon.StepKindList,
			Description: "List files and directories under the workspace deliverable directory without modifying workspace state. Paths must be workspace-relative; large generated directories are skipped.",
			Input:       map[string]any{"path": "relative directory path", "recursive": "boolean", "maxEntries": "number"},
		},
		{
			Kind:        filecommon.StepKindRead,
			Description: "Read bounded UTF-8 text content or binary metadata for one workspace-relative file. Use contentHash as the baseHash for later patch/apply or patch/preview when guarding against stale writes.",
			Input:       map[string]any{"path": "relative file path", "startLine": "number", "endLine": "number", "maxBytes": "number"},
		},
		{
			Kind:        filecommon.StepKindEdit,
			Description: "Create a non-mutating unified_diff patch preview by replacing a unique oldText with newText in one workspace-relative text file. This does not write files and must be followed by patch/preview or patch/apply to persist changes. oldText and newText must be exact, complete text fragments, not omitted placeholders.",
			Input:       map[string]any{"path": "relative file path", "oldText": "exact text to replace", "newText": "replacement text", "baseHash": "sha256 content hash", "occurrence": "number when oldText is not unique"},
		},
		{
			Kind:        sshcommon.StepKindCommand,
			Description: "Execute one bounded non-interactive SSH command on a remote host using supplied password or private key credentials.",
			Input:       map[string]any{"host": "string", "port": "number", "username": "string", "password": "string", "privateKey": "string", "privateKeyPath": "string", "command": "string", "timeoutMillis": "number"},
		},
		{
			Kind:        webcommon.StepKindSearch,
			Description: "Search external web sources through a governed Web Search provider. Search results are candidate sources only; important facts should be verified by web/fetch before final use.",
			Input:       map[string]any{"query": "string", "locale": "string", "recency": "day|week|month|year", "domains": "[]string", "excludeDomains": "[]string", "maxResults": "number", "safeSearch": "boolean", "reason": "string", "approvalId": "approved approval id when required"},
		},
		{
			Kind:        webcommon.StepKindFetch,
			Description: "Fetch one HTTP(S) URL through a governed Web Search provider and return bounded content or metadata. Include approvalId when the source requires approval.",
			Input:       map[string]any{"url": "string", "maxBytes": "number", "extractMode": "text|metadata|summary", "reason": "string", "approvalId": "approved approval id when required"},
		},
		{
			Kind:        llmcommon.RuntimeBlockKindPatchApply,
			Description: "Apply one file_write, unified_diff, or text_replace patch inside the workspace deliverable directory and record patch evidence. targetRef is a file path; an existing file path component cannot be used as a directory. Include baseHash from file/read or current repair snapshots when changing an existing file. Prefer text_replace with exact oldText/newText for stale diff/context mismatch repairs. When the runtime input carries missing coverage targets, include coverageTargets that cite the exact Goal, DoD, or deliverable coverage item this state change intends to close. Patch content, diffs, and exact replacements must be complete, concrete, and directly applicable to the current workspace content; do not use omitted sections or placeholder bodies. Governance/report files such as decision_record.md, approval_record.md, delivery_summary.md, result_summary.md, final_summary.md, deliverable_manifest.md, or evidence_chain.md are reserved for WorkOrch-managed Artifact records and must not be written through patch/apply.",
			Input:       map[string]any{"patchType": "file_write|unified_diff|text_replace", "targetRef": "relative path", "content": "string for file_write", "diff": "string for unified_diff", "oldText": "exact text for text_replace", "newText": "replacement text for text_replace", "occurrence": "optional 1-based occurrence when oldText is not unique", "summary": "string", "baseHash": "sha256 content hash", "coverageTargets": "array of Goal/DoD/deliverable coverage target strings or objects", "approvalId": "approved approval id when required"},
		},
		{
			Kind:        llmcommon.RuntimeBlockKindPatchPreview,
			Description: "Validate and summarize one file_write, unified_diff, or text_replace patch without writing files. Use this before high-risk edits when the user or approval flow needs a non-mutating diff preview. Patch content, diffs, and exact replacements must be complete, concrete, and directly applicable to the current workspace content; do not use omitted sections or placeholder bodies.",
			Input:       map[string]any{"patchType": "file_write|unified_diff|text_replace", "targetRef": "relative path", "content": "string for file_write", "diff": "string for unified_diff", "oldText": "exact text for text_replace", "newText": "replacement text for text_replace", "occurrence": "optional 1-based occurrence when oldText is not unique", "summary": "string", "baseHash": "sha256 content hash", "approvalId": "approved approval id when required"},
		},
		{
			Kind:        llmcommon.RuntimeBlockKindPatchRevert,
			Description: "Revert a WorkOrch-applied patch by patchId only when the current file hash still matches the original patch afterHash. Sensitive or destructive reverts may require approvalId.",
			Input:       map[string]any{"patchId": "WorkOrch patch id", "approvalId": "approved approval id when required", "strict": "boolean"},
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
	}
	blockKinds = appendGitRuntimeCapabilities(blockKinds)
	return llmmodels.RuntimeCapabilities{
		BlockKinds: blockKinds,
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

func appendGitRuntimeCapabilities(blockKinds []llmmodels.RuntimeBlockCapability) []llmmodels.RuntimeBlockCapability {
	cfg, err := workorchconfig.Current()
	enabled := err != nil || workorchconfig.GitRuntimeEnabled(cfg.Runtime.Git)
	writeEnabled := err != nil || workorchconfig.GitRuntimeWriteEnabled(cfg.Runtime.Git)
	if enabled {
		blockKinds = append(blockKinds, llmmodels.RuntimeBlockCapability{
			Kind:        gitcommon.StepKindRead,
			Description: "Read local Git repository state for the workspace deliverable repository. Supported operations: status, diff, log, show, branch. This block is read-only and path-scoped; it is not a general Git CLI.",
			Input:       map[string]any{"operation": "status|diff|log|show|branch", "pathspecs": "[]relative path", "ref": "string", "baseRef": "string", "limit": "number", "reason": "string"},
		})
	}
	if enabled && writeEnabled {
		blockKinds = append(blockKinds, llmmodels.RuntimeBlockCapability{
			Kind:        gitcommon.StepKindWrite,
			Description: "Perform minimal local Git state changes in the workspace deliverable repository. Supported operations: init, add, commit. init initializes the workspace deliverable directory when it is not yet a Git repository; add requires explicit pathspecs; commit only commits already staged changes, rejects detached HEAD, does not run hooks, and does not support remote operations or history rewrite.",
			Input:       map[string]any{"operation": "init|add|commit", "pathspecs": "[]relative path for add only", "message": "commit message for commit", "expectedHead": "optional full HEAD hash guard", "reason": "string"},
		})
	}
	return blockKinds
}

func failureOutput(request runtimemodel.StepExecutionRequest, stage string, reason string, config llmmodels.ProviderConfig) map[string]any {
	_ = config
	return map[string]any{
		"runId":           string(request.RunID),
		"stepId":          string(request.StepID),
		"status":          "failed",
		"stage":           stage,
		"error":           reason,
		"failureCategory": llmFailureCategory(stage),
		"resolutionHint":  llmFailureResolutionHint(stage),
	}
}

func llmFailureCategory(stage string) string {
	switch stage {
	case "llm_provider_unavailable",
		"llm_provider_client_unavailable",
		"llm_provider_operation_unsupported",
		"llm_provider_timeout",
		"llm_provider_request_too_large",
		"llm_provider_rate_limited",
		"llm_provider_auth_or_config_error",
		"llm_provider_stream_incomplete",
		"llm_provider_error",
		"llm_provider_stream_error":
		return "llm_provider"
	case "llm_schema_invalid", "llm_decision_invalid", "llm_failed":
		return "llm_decision"
	default:
		return "llm_runtime"
	}
}

func llmFailureResolutionHint(stage string) string {
	switch stage {
	case "llm_provider_unavailable",
		"llm_provider_client_unavailable",
		"llm_provider_operation_unsupported",
		"llm_provider_timeout",
		"llm_provider_request_too_large",
		"llm_provider_rate_limited",
		"llm_provider_auth_or_config_error",
		"llm_provider_stream_incomplete",
		"llm_provider_error",
		"llm_provider_stream_error":
		return "check LLM provider baseURL, model, API key, protocol compatibility, and network availability before rerunning the workspace"
	case "llm_schema_invalid", "llm_decision_invalid":
		return "check LLM decision JSON format and schema constraints"
	default:
		return "check runtime step output and execution evidence"
	}
}

func patchOutput(requests []llmmodels.PatchRequest) []map[string]any {
	ret := make([]map[string]any, 0, len(requests))
	for _, request := range normalizedPatchRequests(requests) {
		patch := map[string]any{
			"patchType":  request.PatchType,
			"targetRef":  request.TargetRef,
			"content":    request.Content,
			"diff":       request.Diff,
			"summary":    request.Summary,
			"baseHash":   request.BaseHash,
			"approvalId": request.ApprovalID,
		}
		if request.CoverageTargets != nil {
			patch["coverageTargets"] = request.CoverageTargets
		}
		ret = append(ret, patch)
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

func isReservedLLMReportPatchTarget(targetRef string) bool {
	cleanTarget := strings.TrimSpace(filepath.ToSlash(filepath.Clean(strings.TrimSpace(targetRef))))
	cleanTarget = strings.TrimPrefix(cleanTarget, "./")
	if cleanTarget == "" || cleanTarget == "." {
		return false
	}
	lowerTarget := strings.ToLower(cleanTarget)
	if strings.HasPrefix(lowerTarget, ".workorch-data/") || strings.HasPrefix(lowerTarget, "delivery/") {
		return true
	}
	base := strings.ToLower(filepath.Base(cleanTarget))
	ext := strings.ToLower(filepath.Ext(base))
	switch ext {
	case ".md", ".txt", ".adoc", ".rst", ".json":
	default:
		return false
	}
	stem := strings.TrimSuffix(base, ext)
	stem = strings.NewReplacer("-", "_", " ", "_", ".", "_").Replace(stem)
	terms := strings.FieldsFunc(stem, func(r rune) bool { return r == '_' })
	has := func(term string) bool {
		for _, item := range terms {
			if item == term {
				return true
			}
		}
		return false
	}
	switch {
	case has("decision") && has("record"):
		return true
	case has("approval") && has("record"):
		return true
	case has("delivery") && has("summary"):
		return true
	case has("deliverable") && has("manifest"):
		return true
	case has("workspace") && has("deliverable") && has("manifest"):
		return true
	case has("result") && has("summary"):
		return true
	case has("run") && has("summary"):
		return true
	case has("acceptance") && has("evidence"):
		return true
	case has("evidence") && has("chain"):
		return true
	case has("dod") && has("assessment"):
		return true
	case has("final") && (has("summary") || has("result") || has("report")):
		return true
	default:
		return false
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
	ret := map[string]any{
		"satisfied":             assessment.Satisfied,
		"items":                 items,
		"summary":               assessment.Summary,
		"goalOutcome":           assessment.GoalOutcome,
		"implementationSummary": assessment.ImplementationSummary,
		"verificationSummary":   assessment.VerificationSummary,
		"knownLimitations":      append([]string(nil), assessment.KnownLimitations...),
		"residualRisks":         append([]string(nil), assessment.ResidualRisks...),
		"nextActions":           append([]string(nil), assessment.NextActions...),
	}
	if assessment.GoalCoverage != nil {
		ret["goalCoverage"] = doDCoverageOutput(assessment.GoalCoverage)
	}
	if assessment.AcceptanceDimensions != nil {
		ret["acceptanceDimensions"] = doDCoverageOutput(assessment.AcceptanceDimensions)
	}
	if assessment.DeliverableCoverage != nil {
		ret["deliverableCoverage"] = doDCoverageOutput(assessment.DeliverableCoverage)
	}
	return ret
}

func doDCoverageOutput(coverage *llmmodels.DoDCoverage) map[string]any {
	if coverage == nil {
		return nil
	}
	items := make([]map[string]any, 0, len(coverage.Items))
	for _, item := range coverage.Items {
		entry := map[string]any{
			"state":        item.State,
			"evidenceRefs": append([]string(nil), item.EvidenceRefs...),
		}
		for key, value := range map[string]string{
			"id":          item.ID,
			"deliverable": item.Deliverable,
			"name":        item.Name,
			"path":        item.Path,
			"goal":        item.Goal,
			"goalItem":    item.GoalItem,
			"item":        item.Item,
			"requirement": item.Requirement,
			"reason":      item.Reason,
			"summary":     item.Summary,
		} {
			if strings.TrimSpace(value) != "" {
				entry[key] = value
			}
		}
		if len(item.ArtifactRefs) > 0 {
			entry["artifactRefs"] = append([]string(nil), item.ArtifactRefs...)
		}
		items = append(items, entry)
	}
	return map[string]any{
		"satisfied": coverage.Satisfied,
		"items":     items,
		"summary":   coverage.Summary,
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
