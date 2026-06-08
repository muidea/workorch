package llm

import (
	"context"
	"fmt"
	"strings"

	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"

	llmbiz "github.com/muidea/workorch/internal/modules/blocks/llm/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	llmproviderconfig "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/providerconfig"
	"github.com/muidea/workorch/internal/modules/blocks/pkg/runtimeevent"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

const ModuleID = modulecommon.ModuleID
const StepKindWorkspaceExecution = modulecommon.StepKindWorkspaceExecution

type Module struct {
	runtimeAdapter *llmbiz.RuntimeAdapter
	executor       *runtimeevent.Executor
	eventHub       events.Hub
	unsub          []func()
}

func New() *Module { return &Module{} }

func (m *Module) ID() string { return ModuleID }

type Dependencies struct {
	EventHub events.Hub
}

func (m *Module) Configure(deps ...Dependencies) error {
	if len(deps) > 0 {
		m.eventHub = deps[0].EventHub
	}
	m.runtimeAdapter = llmbiz.NewRuntimeAdapterFromConfigLoaderWithRevision(loadProviderConfigsFromWorkorchConfig, workorchconfig.Revision)
	return nil
}

func (m *Module) Start() error {
	if m == nil || m.runtimeAdapter == nil || m.eventHub == nil {
		return common.ErrInvalidArgument
	}
	if m.executor != nil || len(m.unsub) > 0 {
		return nil
	}
	m.executor = runtimeevent.NewExecutor(ModuleID, m.eventHub, m.runtimeAdapter, StepKindWorkspaceExecution)
	if err := m.executor.Run(); err != nil {
		return err
	}
	m.unsub = append(m.unsub, m.eventHub.Subscribe(llmevents.TopicLLMCommandCompleteProvider, m.handleProviderCompleteCommand))
	m.unsub = append(m.unsub, m.eventHub.Subscribe(llmevents.TopicLLMCommandGenerateEmbedding, m.handleGenerateEmbeddingCommand))
	return nil
}

func (m *Module) Stop(ctx context.Context) error {
	_ = ctx
	for idx := len(m.unsub) - 1; idx >= 0; idx-- {
		if m.unsub[idx] != nil {
			m.unsub[idx]()
		}
	}
	m.unsub = nil
	if m.executor != nil {
		m.executor.Teardown()
	}
	m.executor = nil
	m.runtimeAdapter = nil
	m.eventHub = nil
	return nil
}

func (m *Module) handleProviderCompleteCommand(ctx context.Context, envelope events.Envelope) error {
	if m == nil {
		return common.ErrInvalidArgument
	}
	request, ok := envelope.Payload[events.PayloadKeyProviderRequest].(llmmodels.ProviderRequest)
	if !ok {
		return common.ErrInvalidArgument
	}
	defaultProviderID, configs, err := loadProviderConfigsFromWorkorchConfig()
	if err != nil {
		return err
	}
	requestedProviderID, _ := envelope.Payload[events.PayloadKeyProviderID].(string)
	runtime, providerID, err := selectProviderConfig(strings.TrimSpace(requestedProviderID), defaultProviderID, configs)
	if err != nil {
		return err
	}
	provider := llmbiz.NewProviderFromConfig(runtime)
	if provider == nil {
		return fmt.Errorf("%w: llm provider client is not configured", common.ErrInvalidArgument)
	}
	if strings.TrimSpace(request.Model) == "" {
		request.Model = runtime.Model
	}
	request.Config = llmproviderconfig.RedactedProviderConfig(runtime)
	completion, err := provider.Complete(ctx, request)
	if err != nil {
		return err
	}
	return events.Response(events.Envelope{
		Topic:        envelope.Topic + "/response",
		SourceModule: ModuleID,
		SubjectType:  envelope.SubjectType,
		SubjectID:    envelope.SubjectID,
		WorkspaceID:  envelope.WorkspaceID,
		RunID:        envelope.RunID,
		StepID:       envelope.StepID,
		Payload: map[string]any{
			events.PayloadKeyProviderCompletion: completion,
			events.PayloadKeyProviderID:         providerID,
			events.PayloadKeyLLMProviderID:      providerID,
			events.PayloadKeyLLMModel:           runtime.Model,
		},
	})
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

func (m *Module) handleGenerateEmbeddingCommand(ctx context.Context, envelope events.Envelope) error {
	if m == nil {
		return common.ErrInvalidArgument
	}
	providerID, _ := envelope.Payload[events.PayloadKeyProviderID].(string)
	providerID = strings.TrimSpace(providerID)
	input, _ := envelope.Payload[events.PayloadKeyInput].(string)
	input = strings.TrimSpace(input)
	if providerID == "" || input == "" {
		return common.ErrInvalidArgument
	}
	defaultProviderID, configs, err := loadProviderConfigsFromWorkorchConfig()
	if err != nil {
		return err
	}
	adapter := llmbiz.NewRuntimeAdapterFromConfigs(defaultProviderID, configs)
	embedding, err := adapter.GenerateEmbedding(ctx, providerID, input)
	if err != nil {
		return err
	}
	return events.Response(events.Envelope{
		Topic:        envelope.Topic + "/response",
		SourceModule: ModuleID,
		SubjectType:  envelope.SubjectType,
		SubjectID:    envelope.SubjectID,
		WorkspaceID:  envelope.WorkspaceID,
		RunID:        envelope.RunID,
		StepID:       envelope.StepID,
		Payload: map[string]any{
			events.PayloadKeyProviderEmbedding: embedding,
			events.PayloadKeyProviderID:        embedding.ProviderID,
			events.PayloadKeyLLMProviderID:     embedding.ProviderID,
			events.PayloadKeyLLMModel:          embedding.Model,
		},
	})
}

func loadProviderConfigsFromWorkorchConfig() (string, map[string]llmmodels.ProviderConfig, error) {
	cfg, err := workorchconfig.Current()
	if err != nil {
		return "", nil, err
	}
	return llmproviderconfig.LoadProviderConfigs(cfg.LLM.DefaultProvider, cfg.LLMProviderValueSets())
}
