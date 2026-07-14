package llm

import (
	"context"
	"fmt"
	"strings"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"

	llmbiz "github.com/muidea/workorch/internal/modules/blocks/llm/biz"
	llmruntimev2 "github.com/muidea/workorch/internal/modules/blocks/llm/biz/runtimev2"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	llmproviderconfig "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/providerconfig"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const ModuleID = modulecommon.ModuleID
const StepKindWorkspaceExecution = modulecommon.StepKindWorkspaceExecution

type Module struct {
	rawEventHub            magicevent.Hub
	observer               magicevent.SimpleObserver
	prerequisiteObserver   magicevent.SimpleObserver
	providerConfigLoader   llmbiz.ProviderConfigLoader
	providerConfigRevision llmbiz.ProviderConfigRevision
	candidateProvider      llmruntimev2.CandidateProvider
	topics                 []string
}

func New() *Module { return &Module{} }

func (m *Module) ID() string { return ModuleID }

type Dependencies struct {
	EventHub               magicevent.Hub
	ProviderConfigLoader   llmbiz.ProviderConfigLoader
	ProviderConfigRevision llmbiz.ProviderConfigRevision
	CandidateProvider      llmruntimev2.CandidateProvider
}

func (m *Module) Configure(deps ...Dependencies) error {
	if len(deps) > 0 {
		m.rawEventHub = deps[0].EventHub
		m.providerConfigLoader = deps[0].ProviderConfigLoader
		m.providerConfigRevision = deps[0].ProviderConfigRevision
		m.candidateProvider = deps[0].CandidateProvider
	}
	if m.providerConfigLoader == nil {
		m.providerConfigLoader = loadProviderConfigsFromWorkorchConfig
	}
	if m.providerConfigRevision == nil {
		m.providerConfigRevision = workorchconfig.Revision
	}
	if m.candidateProvider == nil {
		m.candidateProvider = m
	}
	return nil
}

func (m *Module) Start() error {
	if m == nil || m.providerConfigLoader == nil || m.rawEventHub == nil {
		return common.ErrInvalidArgument
	}
	if m.observer != nil {
		return nil
	}
	m.observer = magicevent.NewSimpleObserver(ModuleID, m.rawEventHub)
	m.topics = []string{
		llmevents.TopicLLMCommandCompleteProvider,
		llmevents.TopicLLMCommandGenerateEmbedding,
	}
	for _, topic := range m.topics {
		m.observer.Subscribe(topic, m.handleCommand)
	}
	m.observer.Subscribe(orchestratorevents.TopicOrchestratorAcquireRuntimeCapabilities, m.handleRuntimeCapabilities)
	m.topics = append(m.topics, orchestratorevents.TopicOrchestratorAcquireRuntimeCapabilities)
	if m.candidateProvider != nil {
		m.observer.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, m.handleRuntimeCandidate)
		m.topics = append(m.topics, orchestratorevents.TopicOrchestratorRequestRuntimeCandidate)
	}
	m.prerequisiteObserver = magicevent.NewSimpleObserverWithMatchID(ModuleID+"/prerequisite", orchestratorevents.PrerequisiteCommandDestination, m.rawEventHub)
	m.prerequisiteObserver.Subscribe(orchestratorevents.TopicOrchestratorAcquireProviderCeiling, m.handleProviderCeilingPrerequisite)
	return nil
}

func (m *Module) handleRuntimeCapabilities(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(orchestratorevents.AcquireRuntimeCapabilitiesCommand)
	if !ok {
		setPrerequisiteMagicError(result, common.ErrInvalidArgument)
		return
	}
	response, err := orchestratorevents.ResolveRuntimeCapabilities(command, ModuleID, StepKindWorkspaceExecution)
	if err != nil {
		setPrerequisiteMagicError(result, err)
		return
	}
	result.Set(response, nil)
}

func (m *Module) Stop(ctx context.Context) error {
	_ = ctx
	if m.observer != nil {
		for idx := len(m.topics) - 1; idx >= 0; idx-- {
			m.observer.Unsubscribe(m.topics[idx])
		}
	}
	m.topics = nil
	m.observer = nil
	if m.prerequisiteObserver != nil {
		m.prerequisiteObserver.Unsubscribe(orchestratorevents.TopicOrchestratorAcquireProviderCeiling)
	}
	m.prerequisiteObserver = nil
	m.rawEventHub = nil
	m.providerConfigLoader = nil
	m.providerConfigRevision = nil
	m.candidateProvider = nil
	return nil
}

func (m *Module) handleRuntimeCandidate(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(orchestratorevents.RequestRuntimeCandidateCommand)
	if !ok || command.WorkspaceID == "" || command.RunID == "" || m.candidateProvider == nil ||
		command.Decision.AuthoritySnapshot.WorkspaceID != command.WorkspaceID || command.Decision.AuthoritySnapshot.RunID != command.RunID {
		setPrerequisiteMagicError(result, common.ErrInvalidArgument)
		return
	}
	response, err := llmruntimev2.RequestCandidate(event.Context(), m.candidateProvider, llmmodels.RuntimeCandidateRequest{
		Decision:          command.Decision,
		Transaction:       command.Transaction,
		CandidateAttempt:  command.CandidateAttempt,
		PreviousFailure:   command.PreviousFailure,
		StablePrefixHash:  command.StablePrefixHash,
		DefinitionContext: command.DefinitionContext,
		CandidateContext:  command.CandidateContext,
	})
	if err != nil {
		failure := llmruntimev2.CandidateFailureFromError(err)
		result.Set(orchestratorevents.RequestRuntimeCandidateResult{Transaction: command.Transaction, Failure: &failure}, nil)
		return
	}
	result.Set(orchestratorevents.RequestRuntimeCandidateResult{Transaction: response.Transaction, Candidate: response.Candidate}, nil)
}

func (m *Module) handleProviderCeilingPrerequisite(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(orchestratorevents.AcquireProviderCeilingCommand)
	if !ok || command.WorkspaceID == "" || command.RunID == "" || command.AuthoritySnapshotID == "" || command.PrerequisiteID == "" {
		setPrerequisiteMagicError(result, common.ErrInvalidArgument)
		return
	}
	defaultProviderID, configs, err := m.providerConfigLoader()
	if err != nil {
		setPrerequisiteMagicError(result, err)
		return
	}
	config, providerID, err := selectProviderConfig(strings.TrimSpace(command.ProviderID), defaultProviderID, configs)
	if err != nil {
		setPrerequisiteMagicError(result, err)
		return
	}
	maxBytes := config.ContextProfile.MaxRequestBytes
	if maxBytes <= 0 {
		setPrerequisiteMagicError(result, common.ErrInvalidArgument)
		return
	}
	if command.MinimumBytes > 0 && maxBytes < command.MinimumBytes {
		setPrerequisiteMagicError(result, fmt.Errorf("provider %q request ceiling %d is below required minimum %d", providerID, maxBytes, command.MinimumBytes))
		return
	}
	result.Set(orchestratorevents.AcquireProviderCeilingResult{
		PrerequisiteID: command.PrerequisiteID,
		Ceiling: runtimecontract.ProviderCeiling{
			ProviderID: providerID,
			MaxBytes:   maxBytes,
			Revision:   fmt.Sprintf("config-%d", m.providerConfigRevision()),
		},
	}, nil)
}

func setPrerequisiteMagicError(result magicevent.Result, err error) {
	if result == nil {
		return
	}
	result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
}

func (m *Module) handleCommand(ev magicevent.Event, result magicevent.Result) {
	response, err := m.executeCommand(ev)
	setLLMMagicResult(result, response, err)
}

func (m *Module) executeCommand(ev magicevent.Event) (llmevents.LLMCommandResult, error) {
	if m == nil {
		return llmevents.LLMCommandResult{}, common.ErrInvalidArgument
	}
	ctx := ev.Context()
	switch ev.ID() {
	case llmevents.TopicLLMCommandCompleteProvider:
		cmd, ok := ev.Data().(llmevents.CompleteProviderCommand)
		if !ok {
			return llmevents.LLMCommandResult{}, common.ErrInvalidArgument
		}
		completion, providerID, model, err := m.completeProvider(ctx, cmd.ProviderID, cmd.Request)
		if err != nil {
			return llmevents.LLMCommandResult{}, err
		}
		return llmevents.LLMCommandResult{ProviderID: providerID, Model: model, Completion: llmCompletionEventData(completion)}, nil
	case llmevents.TopicLLMCommandGenerateEmbedding:
		cmd, ok := ev.Data().(llmevents.GenerateEmbeddingCommand)
		if !ok || strings.TrimSpace(cmd.ProviderID) == "" || strings.TrimSpace(cmd.Input) == "" {
			return llmevents.LLMCommandResult{}, common.ErrInvalidArgument
		}
		embedding, err := m.generateEmbedding(ctx, cmd.ProviderID, cmd.Input)
		if err != nil {
			return llmevents.LLMCommandResult{}, err
		}
		return llmevents.LLMCommandResult{ProviderID: embedding.ProviderID, Model: embedding.Model, Embedding: &embedding}, nil
	default:
		return llmevents.LLMCommandResult{}, common.ErrInvalidArgument
	}
}

func llmCompletionEventData(completion llmmodels.ProviderCompletionResult) *llmevents.ProviderCompletionData {
	ret := &llmevents.ProviderCompletionData{Content: string(completion.Payload)}
	if completion.Usage != nil {
		ret.Usage = &llmevents.ProviderUsageData{
			InputTokens: completion.Usage.InputTokens, OutputTokens: completion.Usage.OutputTokens,
			TotalTokens: completion.Usage.TotalTokens, CacheReadTokens: completion.Usage.CacheReadTokens,
			CacheWriteTokens: completion.Usage.CacheWriteTokens, CacheCreationTokens: completion.Usage.CacheCreationTokens,
			CacheableInputTokens: completion.Usage.CacheableInputTokens, NonCacheableInputTokens: completion.Usage.NonCacheableInputTokens,
		}
	}
	return ret
}

func setLLMMagicResult(result magicevent.Result, data llmevents.LLMCommandResult, err error) {
	if result == nil {
		return
	}
	if err != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
		return
	}
	result.Set(data, nil)
}

func (m *Module) completeProvider(ctx context.Context, requestedProviderID string, request llmmodels.ProviderRequest) (llmmodels.ProviderCompletionResult, string, string, error) {
	if m == nil || m.providerConfigLoader == nil {
		return llmmodels.ProviderCompletionResult{}, "", "", common.ErrInvalidArgument
	}
	defaultProviderID, configs, err := m.providerConfigLoader()
	if err != nil {
		return llmmodels.ProviderCompletionResult{}, "", "", err
	}
	runtime, providerID, err := selectProviderConfig(strings.TrimSpace(requestedProviderID), defaultProviderID, configs)
	if err != nil {
		return llmmodels.ProviderCompletionResult{}, "", "", err
	}
	provider := llmbiz.NewProviderFromConfig(runtime)
	if provider == nil {
		return llmmodels.ProviderCompletionResult{}, "", "", fmt.Errorf("%w: llm provider client is not configured", common.ErrInvalidArgument)
	}
	if strings.TrimSpace(request.Model) == "" {
		request.Model = runtime.Model
	}
	request.Config = llmproviderconfig.RedactedProviderConfig(runtime)
	completion, err := provider.Complete(ctx, request)
	if err != nil {
		return llmmodels.ProviderCompletionResult{}, "", "", err
	}
	return completion, providerID, runtime.Model, nil
}

func selectProviderConfig(requestedProviderID string, defaultProviderID string, configs map[string]llmmodels.ProviderConfig) (llmmodels.ProviderConfig, string, error) {
	for _, providerID := range []string{requestedProviderID, strings.TrimSpace(defaultProviderID)} {
		if providerID == "" {
			continue
		}
		if cfg, ok := configs[providerID]; ok {
			if cfg.Unavailable {
				return llmmodels.ProviderConfig{}, "", fmt.Errorf("%w: %s", common.ErrInvalidArgument, cfg.UnavailableReason)
			}
			return cfg, providerID, nil
		}
	}
	for id, cfg := range configs {
		if cfg.Unavailable {
			continue
		}
		return cfg, id, nil
	}
	return llmmodels.ProviderConfig{}, "", fmt.Errorf("%w: no available llm provider is configured", common.ErrInvalidArgument)
}

func (m *Module) generateEmbedding(ctx context.Context, providerID string, input string) (llmmodels.ProviderEmbeddingResult, error) {
	providerID = strings.TrimSpace(providerID)
	input = strings.TrimSpace(input)
	if providerID == "" || input == "" {
		return llmmodels.ProviderEmbeddingResult{}, common.ErrInvalidArgument
	}
	if m == nil || m.providerConfigLoader == nil {
		return llmmodels.ProviderEmbeddingResult{}, common.ErrInvalidArgument
	}
	defaultProviderID, configs, err := m.providerConfigLoader()
	if err != nil {
		return llmmodels.ProviderEmbeddingResult{}, err
	}
	config, selectedProviderID, err := selectProviderConfig(providerID, defaultProviderID, configs)
	if err != nil {
		return llmmodels.ProviderEmbeddingResult{}, err
	}
	return llmbiz.GenerateEmbedding(ctx, selectedProviderID, config, input)
}

func loadProviderConfigsFromWorkorchConfig() (string, map[string]llmmodels.ProviderConfig, error) {
	cfg, err := workorchconfig.Current()
	if err != nil {
		return "", nil, err
	}
	return llmproviderconfig.LoadProviderConfigs(cfg.LLM.DefaultProvider, cfg.LLMProviderValueSets())
}
