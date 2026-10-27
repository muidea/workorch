package configcore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestCatalogProjectionPreservesIncompleteEmbeddingDiscovery(t *testing.T) {
	input := llmmodels.ProviderModelCatalog{ProviderID: "ai-proxy", Models: []llmmodels.ProviderModelDescriptor{{
		Target:             runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "embedding-model"},
		SupportedEndpoints: []string{"/v1/embeddings"}, Operations: []string{"embeddings"},
		CapabilityReady: false, UnavailableReason: "capability_incomplete",
	}}}
	view := catalogSummaryFromModel(input)
	if len(view.Models) != 1 {
		t.Fatalf("incomplete model was hidden: %+v", view)
	}
	model := view.Models[0]
	if model.ModelID != "embedding-model" || model.CapabilityReady || model.ContextWindowTokens != 0 || model.MaxOutputTokens != 0 || model.UnavailableReason != "capability_incomplete" || len(model.Operations) != 1 || model.Operations[0] != "embeddings" {
		t.Fatalf("discovery/diagnostic projection drifted: %+v", model)
	}
	view.Models[0].Operations[0] = "responses"
	if input.Models[0].Operations[0] != "embeddings" {
		t.Fatal("projection aliases owner operation data")
	}
}

func TestCatalogProjectionPreservesOperationSpecificEmbeddingReadiness(t *testing.T) {
	input := llmmodels.ProviderModelCatalog{Models: []llmmodels.ProviderModelDescriptor{{
		Target:     runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "mixed-model"},
		Operations: []string{"responses", "embeddings"}, EmbeddingReady: true,
		UnavailableReason: "capability_incomplete",
	}}}
	view := catalogSummaryFromModel(input)
	if len(view.Models) != 1 || !view.Models[0].EmbeddingReady || view.Models[0].CapabilityReady || view.Models[0].ContextWindowTokens != 0 {
		t.Fatalf("embedding support must not invent Responses capacity: %+v", view)
	}
}

func TestLLMCatalogCommandsRequireTopicSpecificResult(t *testing.T) {
	tests := []struct {
		name     string
		topic    string
		response any
		call     func(context.Context, magicevent.Hub) error
	}{
		{
			name: "get rejects refresh result", topic: llmevents.TopicLLMCommandGetProviderModelCatalog,
			response: llmevents.RefreshProviderModelCatalogResult{},
			call: func(ctx context.Context, hub magicevent.Hub) error {
				_, err := GetLLMModelCatalog(ctx, hub)
				return err
			},
		},
		{
			name: "get rejects bare catalog", topic: llmevents.TopicLLMCommandGetProviderModelCatalog,
			response: llmmodels.ProviderModelCatalog{},
			call: func(ctx context.Context, hub magicevent.Hub) error {
				_, err := GetLLMModelCatalog(ctx, hub)
				return err
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			hub := magicevent.NewHub(8)
			t.Cleanup(func() { hub.Terminate(ctx) })
			observer := magicevent.NewSimpleObserver(llmevents.CommandDestination, hub)
			observer.Subscribe(tt.topic, func(_ magicevent.Event, result magicevent.Result) { result.Set(tt.response, nil) })
			t.Cleanup(func() { observer.Unsubscribe(tt.topic) })
			if err := tt.call(ctx, hub); !errors.Is(err, common.ErrInvalidArgument) {
				t.Fatalf("error = %v, want exact result contract defect", err)
			}
		})
	}
}

func TestGetActiveLLMConfigurationRequiresTypedStatusResult(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(8)
	t.Cleanup(func() { hub.Terminate(ctx) })
	observer := magicevent.NewSimpleObserver(llmevents.CommandDestination, hub)
	observer.Subscribe(llmevents.TopicLLMCommandGetActiveConfiguration, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(llmevents.GetProviderModelCatalogResult{}, nil)
	})
	t.Cleanup(func() { observer.Unsubscribe(llmevents.TopicLLMCommandGetActiveConfiguration) })

	_, err := GetActiveLLMConfiguration(ctx, hub)
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("error = %v, want active configuration result contract defect", err)
	}
}

func TestRefreshLLMModelCatalogRequiresTypedRefreshResultAfterActiveConfigurationCheck(t *testing.T) {
	useLLMCatalogTestConfig(t)
	ctx := context.Background()
	active, err := workorchconfig.Current()
	if err != nil {
		t.Fatal(err)
	}
	revision, err := workorchconfig.LLMConfigurationRevision(active.LLM)
	if err != nil {
		t.Fatal(err)
	}
	hub := magicevent.NewHub(8)
	t.Cleanup(func() { hub.Terminate(ctx) })
	observer := magicevent.NewSimpleObserver(llmevents.CommandDestination, hub)
	observer.Subscribe(llmevents.TopicLLMCommandGetActiveConfiguration, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(llmevents.GetActiveConfigurationResult{
			ProviderID:            workorchconfig.BuiltinAIProxyProviderID,
			ConfigurationRevision: revision,
			DefaultChat:           active.LLM.DefaultChat.ToRuntimeTarget(),
		}, nil)
	})
	observer.Subscribe(llmevents.TopicLLMCommandRefreshProviderModelCatalog, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(llmevents.GetProviderModelCatalogResult{}, nil)
	})
	t.Cleanup(func() {
		observer.Unsubscribe(llmevents.TopicLLMCommandGetActiveConfiguration)
		observer.Unsubscribe(llmevents.TopicLLMCommandRefreshProviderModelCatalog)
	})

	_, err = RefreshLLMModelCatalog(ctx, hub)
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("error = %v, want refresh result contract defect", err)
	}
}

func useLLMCatalogTestConfig(t *testing.T) {
	t.Helper()
	_ = workorchconfig.Close()
	configDir := t.TempDir()
	content := `endpointName = "workorch"

[server]
addr = "127.0.0.1:18080"

[repository]
backend = "file"
dataDir = ".workorch-data"

[llm.aiProxy]
baseURL = "http://127.0.0.1:8080"
apiKey = "test-key"
timeout = "180s"
catalogRequestTimeout = "5s"
catalogTTL = "5m"
catalogMaxStaleAge = "24h"
maxCatalogResponseBytes = 1048576
maxCatalogModels = 1000

[llm.defaultChat]
providerID = "ai-proxy"
modelID = "test-model"

[llm.budget]
contextUtilizationRatio = 0.75
estimatedTokensPerByte = 0.25
`
	if err := os.WriteFile(filepath.Join(configDir, "application.toml"), []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv(workorchconfig.EnvConfigDir, configDir)
	t.Cleanup(func() { _ = workorchconfig.Close() })
	if _, err := workorchconfig.Current(); err != nil {
		t.Fatalf("load config: %v", err)
	}
}

func TestConfigurationReadSeparatesSavedAndObservedLLMTargets(t *testing.T) {
	for _, name := range []string{"same revision", "saved change", "owner unavailable"} {
		t.Run(name, func(t *testing.T) {
			useLLMCatalogTestConfig(t)
			persisted, err := workorchconfig.Persisted()
			if err != nil {
				t.Fatal(err)
			}
			revision, err := workorchconfig.LLMConfigurationRevision(persisted.LLM)
			if err != nil {
				t.Fatal(err)
			}
			embedding := runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "active-embedding"}
			active := llmevents.GetActiveConfigurationResult{ProviderID: "ai-proxy", ConfigurationRevision: revision, DefaultChat: persisted.LLM.DefaultChat.ToRuntimeTarget()}
			if name == "saved change" {
				active.ConfigurationRevision = "older-revision"
				active.DefaultChat.ModelID = "active-chat"
				active.DefaultEmbedding = &embedding
			}
			ctx := context.Background()
			hub := magicevent.NewHub(8)
			t.Cleanup(func() { hub.Terminate(ctx) })
			observer := magicevent.NewSimpleObserver(llmevents.CommandDestination, hub)
			observer.Subscribe(llmevents.TopicLLMCommandGetActiveConfiguration, func(_ magicevent.Event, result magicevent.Result) {
				if name == "owner unavailable" {
					result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: "owner unavailable", Cause: common.ErrInvalidArgument})
					return
				}
				result.Set(active, nil)
			})
			observer.Subscribe(llmevents.TopicLLMCommandGetProviderModelCatalog, func(_ magicevent.Event, result magicevent.Result) {
				result.Set(llmevents.GetProviderModelCatalogResult{}, nil)
			})
			t.Cleanup(func() {
				observer.Unsubscribe(llmevents.TopicLLMCommandGetActiveConfiguration)
				observer.Unsubscribe(llmevents.TopicLLMCommandGetProviderModelCatalog)
			})
			view, err := ReadServiceConfigurationWithCatalog(ctx, hub)
			if err != nil {
				t.Fatal(err)
			}
			if view.LLM.DefaultChat != persisted.LLM.DefaultChat.ToRuntimeTarget() || view.LLM.RestartRequired != (name != "same revision") {
				t.Fatalf("saved target or restart state changed: %+v", view.LLM)
			}
			if name == "owner unavailable" {
				if view.LLM.Active != nil {
					t.Fatal("unknown active model fell back to saved selection")
				}
				return
			}
			if view.LLM.Active == nil || view.LLM.Active.DefaultChat != active.DefaultChat {
				t.Fatalf("owner observation lost: %+v", view.LLM.Active)
			}
			if name == "same revision" {
				if view.LLM.Active.DefaultEmbedding != nil {
					t.Fatal("unconfigured active embedding invented")
				}
				return
			}
			if view.LLM.Active.DefaultEmbedding == nil || *view.LLM.Active.DefaultEmbedding != embedding {
				t.Fatal("observed active embedding lost")
			}
			view.LLM.Active.DefaultEmbedding.ModelID = "changed-view"
			if embedding.ModelID != "active-embedding" {
				t.Fatal("active projection aliases owner target")
			}
		})
	}
}
