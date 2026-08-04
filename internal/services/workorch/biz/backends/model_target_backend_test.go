package backend

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	port "github.com/muidea/workorch/internal/pkg/workorchport"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestEmbeddedAndRemoteModelTargetBackendsSharePortSemantics(t *testing.T) {
	ctx := context.Background()
	options := []port.ChatModelTargetOption{
		{ProviderID: "ai-proxy", ModelID: "model-a", DisplayName: "model-a", CapabilityReady: true, Source: "catalog"},
		{ProviderID: "custom", ModelID: "model-b", DisplayName: "model-b", CapabilityReady: true, Source: "configuration"},
	}

	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(context.Background()) })
	observer := magicevent.NewSimpleObserver(workorchdevents.CommandDestination, hub)
	observer.Subscribe(workorchdevents.TopicCLIConfigurationGet, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(workorchdevents.CLIConfigurationGetResult{Configuration: port.ServiceConfig{DefaultChat: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "model-a"}, ChatModelTargets: options}}, nil)
	})
	observer.Subscribe(workorchdevents.TopicCLIConfigurationRefreshChatModels, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(workorchdevents.CLIConfigurationRefreshChatModelsResult{Configuration: port.ServiceConfig{DefaultChat: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "model-a"}, ChatModelTargets: options}}, nil)
	})
	observer.Subscribe(workorchdevents.TopicCLIWorkspaceUpdateLLMTarget, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(workorchdevents.CLIWorkspaceUpdateLLMTargetCommand)
		result.Set(workorchdevents.CLIWorkspaceUpdateLLMTargetResult{Workspace: workorchdevents.CLIWorkspaceInfo{ID: command.WorkspaceID, DefaultLLMTarget: cloneBackendModelTarget(command.Target)}}, nil)
	})
	t.Cleanup(func() {
		observer.Unsubscribe(workorchdevents.TopicCLIConfigurationGet)
		observer.Unsubscribe(workorchdevents.TopicCLIConfigurationRefreshChatModels)
		observer.Unsubscribe(workorchdevents.TopicCLIWorkspaceUpdateLLMTarget)
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/workorch/configuration":
			_, _ = w.Write([]byte(`{"configuration":{"llm":{"defaultChat":{"providerId":"ai-proxy","modelId":"model-a"},"aiProxy":{},"budget":{},"customProviders":[{"id":"custom","apiKeyConfigured":false,"models":[{"id":"model-b","operations":["chat_completions"]},{"id":"embedding-only","operations":["embeddings"]}]}],"catalog":{}}}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/workorch/configuration/llm/models":
			_, _ = w.Write([]byte(`{"catalog":{"providerId":"ai-proxy","state":"ready","source":"catalog","models":[{"providerId":"ai-proxy","modelId":"model-a","operations":["chat_completions"],"capabilityReady":true},{"providerId":"ai-proxy","modelId":"embedding-only","operations":["embeddings"],"capabilityReady":true},{"providerId":"ai-proxy","modelId":"not-ready","operations":["chat_completions"],"capabilityReady":false}]}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/workorch/configuration/llm/models/refresh":
			_, _ = w.Write([]byte(`{"catalog":{"providerId":"ai-proxy","state":"ready","source":"catalog","models":[{"providerId":"ai-proxy","modelId":"model-a","operations":["chat_completions"],"capabilityReady":true},{"providerId":"ai-proxy","modelId":"embedding-only","operations":["embeddings"],"capabilityReady":true},{"providerId":"ai-proxy","modelId":"not-ready","operations":["chat_completions"],"capabilityReady":false}]}}`))
		case (r.Method == http.MethodPut || r.Method == http.MethodDelete) && r.URL.Path == "/api/v1/workorch/workspaces/ws-model/llm-target":
			var target *runtimecontract.ProviderModelTarget
			if r.Method == http.MethodPut {
				var value runtimecontract.ProviderModelTarget
				if err := json.NewDecoder(r.Body).Decode(&value); err != nil {
					t.Fatalf("decode target: %v", err)
				}
				target = &value
			}
			body := map[string]any{"workspace": map[string]any{"id": "ws-model", "state": "active", "defaultLLMTarget": target}}
			_ = json.NewEncoder(w).Encode(body)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	for name, be := range map[string]port.Backend{"embedded": NewEventHub(hub), "remote": NewRemote(server.URL)} {
		t.Run(name, func(t *testing.T) {
			listed, err := be.Config().ListChatModelTargets(ctx)
			if err != nil {
				t.Fatalf("list models: %v", err)
			}
			if !reflect.DeepEqual(listed, options) {
				t.Fatalf("listed = %+v, want %+v", listed, options)
			}
			refreshed, err := be.Config().RefreshChatModelTargets(ctx)
			if err != nil || !reflect.DeepEqual(refreshed, options) {
				t.Fatalf("refreshed = %+v err=%v", refreshed, err)
			}
			target := runtimecontract.ProviderModelTarget{ProviderID: "custom", ModelID: "model-b"}
			workspace, err := be.Workspace().UpdateLLMTarget(ctx, "ws-model", &target)
			if err != nil || workspace.DefaultLLMTarget == nil || !workspace.DefaultLLMTarget.Same(target) {
				t.Fatalf("update workspace = %+v err=%v", workspace, err)
			}
			workspace, err = be.Workspace().UpdateLLMTarget(ctx, common.WorkspaceID("ws-model"), nil)
			if err != nil || workspace.DefaultLLMTarget != nil {
				t.Fatalf("clear workspace = %+v err=%v", workspace, err)
			}
		})
	}
}

func cloneBackendModelTarget(input *runtimecontract.ProviderModelTarget) *runtimecontract.ProviderModelTarget {
	if input == nil {
		return nil
	}
	value := input.Clone()
	return &value
}
