package llm

import (
	"context"
	"fmt"
	"strings"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	magictask "github.com/muidea/magicCommon/task"
	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"

	llmbiz "github.com/muidea/workorch/internal/modules/blocks/llm/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	llmproviderconfig "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/providerconfig"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/runtimeevent"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

const ModuleID = modulecommon.ModuleID
const StepKindWorkspaceExecution = modulecommon.StepKindWorkspaceExecution

type Module struct {
	runtimeAdapter *llmbiz.RuntimeAdapter
	executor       *runtimeevent.Executor
	eventHub       magicevent.Hub
	rawEventHub    magicevent.Hub
	background     magictask.BackgroundRoutine
	observer       magicevent.SimpleObserver
	topics         []string
}

func New() *Module { return &Module{} }

func (m *Module) ID() string { return ModuleID }

type Dependencies struct {
	EventHub   magicevent.Hub
	Background magictask.BackgroundRoutine
}

func (m *Module) Configure(deps ...Dependencies) error {
	if len(deps) > 0 {
		m.rawEventHub = deps[0].EventHub
		m.eventHub = deps[0].EventHub
		m.background = deps[0].Background
	}
	m.runtimeAdapter = llmbiz.NewRuntimeAdapterFromConfigLoaderWithRevision(loadProviderConfigsFromWorkorchConfig, workorchconfig.Revision)
	return nil
}

func (m *Module) Start() error {
	if m == nil || m.runtimeAdapter == nil || m.eventHub == nil || m.rawEventHub == nil || m.background == nil {
		return common.ErrInvalidArgument
	}
	if m.executor != nil || m.observer != nil {
		return nil
	}
	m.executor = runtimeevent.NewExecutor(ModuleID, m.eventHub, m.background, m.runtimeAdapter, StepKindWorkspaceExecution)
	if err := m.executor.Run(); err != nil {
		return err
	}
	m.observer = magicevent.NewSimpleObserver(ModuleID, m.rawEventHub)
	m.topics = []string{
		llmevents.TopicLLMCommandCompleteProvider,
		llmevents.TopicLLMCommandGenerateEmbedding,
	}
	for _, topic := range m.topics {
		m.observer.Subscribe(topic, m.handleCommand)
	}
	return nil
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
	if m.executor != nil {
		m.executor.Teardown()
	}
	m.executor = nil
	m.runtimeAdapter = nil
	m.eventHub = nil
	m.rawEventHub = nil
	m.background = nil
	return nil
}

func (m *Module) handleCommand(ev magicevent.Event, result magicevent.Result) {
	response, err := m.executeCommand(ev.Context(), ev.ID(), ev.Data())
	setLLMMagicResult(result, response, err)
}

func (m *Module) executeCommand(ctx context.Context, topic string, data any) (llmevents.LLMCommandResult, error) {
	if m == nil {
		return llmevents.LLMCommandResult{}, common.ErrInvalidArgument
	}
	switch topic {
	case llmevents.TopicLLMCommandCompleteProvider:
		cmd, ok := data.(llmevents.CompleteProviderCommand)
		if !ok {
			return llmevents.LLMCommandResult{}, common.ErrInvalidArgument
		}
		completion, providerID, model, err := m.completeProvider(ctx, cmd.ProviderID, cmd.Request)
		if err != nil {
			return llmevents.LLMCommandResult{}, err
		}
		return llmevents.LLMCommandResult{ProviderID: providerID, Model: model, Completion: &completion}, nil
	case llmevents.TopicLLMCommandGenerateEmbedding:
		cmd, ok := data.(llmevents.GenerateEmbeddingCommand)
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
	defaultProviderID, configs, err := loadProviderConfigsFromWorkorchConfig()
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
	defaultProviderID, configs, err := loadProviderConfigsFromWorkorchConfig()
	if err != nil {
		return llmmodels.ProviderEmbeddingResult{}, err
	}
	adapter := llmbiz.NewRuntimeAdapterFromConfigs(defaultProviderID, configs)
	return adapter.GenerateEmbedding(ctx, providerID, input)
}

func loadProviderConfigsFromWorkorchConfig() (string, map[string]llmmodels.ProviderConfig, error) {
	cfg, err := workorchconfig.Current()
	if err != nil {
		return "", nil, err
	}
	return llmproviderconfig.LoadProviderConfigs(cfg.LLM.DefaultProvider, cfg.LLMProviderValueSets())
}
