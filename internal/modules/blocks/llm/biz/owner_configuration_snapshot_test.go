package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

func TestBizStartFreezesLLMConfigurationForCatalogAndStatus(t *testing.T) {
	ctx := context.Background()
	var authorization string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization = r.Header.Get("Authorization")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"object": "list",
			"data": []map[string]any{{
				"id": "model-a", "object": "model",
				"supported_endpoints": []string{"/v1/responses"},
				"contextWindowTokens": 128000, "maxOutputTokens": 8192,
			}},
		})
	}))
	defer server.Close()

	configured := workorchconfig.DefaultLLMConfig()
	configured.AIProxy.BaseURL = server.URL
	configured.AIProxy.APIKey = "old-key"
	configured.DefaultChat = workorchconfig.ProviderModelTargetConfig{ProviderID: workorchconfig.BuiltinAIProxyProviderID, ModelID: "model-a"}
	configured.DefaultEmbedding = &workorchconfig.ProviderModelTargetConfig{ProviderID: workorchconfig.BuiltinAIProxyProviderID, ModelID: "embedding-a"}
	expectedRevision, err := workorchconfig.LLMConfigurationRevision(configured)
	if err != nil {
		t.Fatal(err)
	}

	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	owner, err := New(hub, nil, Dependencies{
		LoadLLMConfig: func() (workorchconfig.LLMConfig, error) {
			return configured, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { owner.Teardown(ctx) })

	// Simulate a configuration watcher observing a later TOML save. The owner
	// must retain the startup snapshot rather than rereading this variable.
	configured.AIProxy.APIKey = "new-key"
	configured.DefaultChat.ModelID = "model-b"
	configured.DefaultEmbedding.ModelID = "embedding-b"
	if _, err := owner.getCatalog(ctx, true); err != nil {
		t.Fatal(err)
	}
	if authorization != "Bearer old-key" {
		t.Fatalf("catalog authorization = %q, want startup key", authorization)
	}

	result := hub.Send(magicevent.NewEventWithContext(
		llmevents.TopicLLMCommandGetActiveConfiguration,
		"test",
		llmevents.CommandDestination,
		magicevent.NewHeader(),
		ctx,
		llmevents.GetActiveConfigurationCommand{},
	))
	if result == nil {
		t.Fatal("active configuration query returned nil result")
	}
	data, queryErr := result.Get()
	if queryErr != nil {
		t.Fatal(queryErr)
	}
	status, ok := data.(llmevents.GetActiveConfigurationResult)
	if !ok {
		t.Fatalf("active configuration result = %T, want typed LLM result", data)
	}
	if status.ConfigurationRevision != expectedRevision || status.ProviderID != workorchconfig.BuiltinAIProxyProviderID || status.DefaultChat.ModelID != "model-a" || status.DefaultEmbedding == nil || status.DefaultEmbedding.ModelID != "embedding-a" {
		t.Fatalf("active configuration status = %+v, want startup revision", status)
	}
}
