package configcore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

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
