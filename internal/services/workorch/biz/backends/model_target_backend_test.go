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
	observer.Subscribe(workorchdevents.TopicCLIWorkspaceUpdateLLMSelection, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(workorchdevents.CLIWorkspaceUpdateLLMSelectionCommand)
		result.Set(workorchdevents.CLIWorkspaceUpdateLLMSelectionResult{Workspace: workorchdevents.CLIWorkspaceInfo{ID: command.WorkspaceID, DefaultLLMSelection: runtimecontract.CloneProviderInferenceSelection(command.Selection)}}, nil)
	})
	t.Cleanup(func() {
		observer.Unsubscribe(workorchdevents.TopicCLIConfigurationGet)
		observer.Unsubscribe(workorchdevents.TopicCLIConfigurationRefreshChatModels)
		observer.Unsubscribe(workorchdevents.TopicCLIWorkspaceUpdateLLMSelection)
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/workorch/configuration":
			_, _ = w.Write([]byte(`{"configuration":{"llm":{"defaultChat":{"providerId":"ai-proxy","modelId":"model-a"},"aiProxy":{},"budget":{},"customProviders":[{"id":"custom","apiKeyConfigured":false,"models":[{"id":"model-b","operations":["chat_completions"]},{"id":"embedding-only","operations":["embeddings"]}]}],"catalog":{}}}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/workorch/configuration/llm/models":
			_, _ = w.Write([]byte(`{"catalog":{"providerId":"ai-proxy","state":"ready","source":"catalog","models":[{"providerId":"ai-proxy","modelId":"model-a","operations":["chat_completions"],"capabilityReady":true},{"providerId":"ai-proxy","modelId":"embedding-only","operations":["embeddings"],"capabilityReady":true},{"providerId":"ai-proxy","modelId":"not-ready","operations":["chat_completions"],"capabilityReady":false}]}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/workorch/configuration/llm/models/refresh":
			_, _ = w.Write([]byte(`{"catalog":{"providerId":"ai-proxy","state":"ready","source":"catalog","models":[{"providerId":"ai-proxy","modelId":"model-a","operations":["chat_completions"],"capabilityReady":true},{"providerId":"ai-proxy","modelId":"embedding-only","operations":["embeddings"],"capabilityReady":true},{"providerId":"ai-proxy","modelId":"not-ready","operations":["chat_completions"],"capabilityReady":false}]}}`))
		case (r.Method == http.MethodPut || r.Method == http.MethodDelete) && r.URL.Path == "/api/v1/workorch/workspaces/ws-model/llm-selection":
			var selection *runtimecontract.ProviderInferenceSelection
			if r.Method == http.MethodPut {
				var value struct{ ProviderID, ModelID, ReasoningEffort string }
				if err := json.NewDecoder(r.Body).Decode(&value); err != nil {
					t.Fatalf("decode target: %v", err)
				}
				selection = &runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: value.ProviderID, ModelID: value.ModelID}, ReasoningEffort: value.ReasoningEffort}
			}
			body := map[string]any{"workspace": map[string]any{"id": "ws-model", "revision": 2, "state": "active", "defaultLLMSelection": selection}}
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
			selection := runtimecontract.ProviderInferenceSelection{Target: target, ReasoningEffort: "high"}
			workspace, err := be.Workspace().UpdateLLMSelection(ctx, "ws-model", &selection, 1)
			if err != nil || workspace.DefaultLLMSelection == nil || !workspace.DefaultLLMSelection.Same(selection) {
				t.Fatalf("update workspace = %+v err=%v", workspace, err)
			}
			workspace, err = be.Workspace().UpdateLLMSelection(ctx, common.WorkspaceID("ws-model"), nil, 2)
			if err != nil || workspace.DefaultLLMSelection != nil {
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
