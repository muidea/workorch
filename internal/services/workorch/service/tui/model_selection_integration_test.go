package tui

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	magicevent "github.com/muidea/magicCommon/event"

	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	port "github.com/muidea/workorch/internal/pkg/workorchport"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
	processbackend "github.com/muidea/workorch/internal/services/workorch/biz/backends"
)

type modelSelectionTransportBackend struct {
	port.Backend
}

func (modelSelectionTransportBackend) Events() port.EventsBackend { return nil }

func (modelSelectionTransportBackend) ProjectionEvents() port.WorkspaceProjectionEventsBackend {
	return nil
}

func TestModelSelectionFlowMatchesEmbeddedAndRemoteBackends(t *testing.T) {
	options := []port.ChatModelTargetOption{
		{ProviderID: "ai-proxy", ModelID: "model-a", DisplayName: "model-a", CapabilityReady: true, Source: "catalog"},
		{ProviderID: "ai-proxy", ModelID: "model-b", DisplayName: "model-b", CapabilityReady: true, Source: "catalog"},
	}

	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(context.Background()) })
	observer := magicevent.NewSimpleObserver(workorchdevents.CommandDestination, hub)
	observer.Subscribe(workorchdevents.TopicCLIConfigurationGet, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(workorchdevents.CLIConfigurationGetResult{Configuration: port.ServiceConfig{
			DefaultChat:      runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "model-a"},
			ChatModelTargets: options,
		}}, nil)
	})
	var embeddedTarget *runtimecontract.ProviderInferenceSelection
	observer.Subscribe(workorchdevents.TopicCLIWorkspaceUpdateLLMSelection, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(workorchdevents.CLIWorkspaceUpdateLLMSelectionCommand)
		embeddedTarget = runtimecontract.CloneProviderInferenceSelection(command.Selection)
		result.Set(workorchdevents.CLIWorkspaceUpdateLLMSelectionResult{Workspace: workorchdevents.CLIWorkspaceInfo{
			ID: command.WorkspaceID, Title: "Workspace", State: common.WorkspaceStateActive,
			DefaultLLMSelection: runtimecontract.CloneProviderInferenceSelection(command.Selection),
		}}, nil)
	})
	t.Cleanup(func() {
		observer.Unsubscribe(workorchdevents.TopicCLIConfigurationGet)
		observer.Unsubscribe(workorchdevents.TopicCLIWorkspaceUpdateLLMSelection)
	})

	var remoteTarget *runtimecontract.ProviderModelTarget
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/workorch/configuration":
			_, _ = w.Write([]byte(`{"configuration":{"llm":{"defaultChat":{"providerId":"ai-proxy","modelId":"model-a"},"aiProxy":{},"budget":{},"catalog":{}}}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/workorch/configuration/llm/models":
			_, _ = w.Write([]byte(`{"catalog":{"providerId":"ai-proxy","state":"ready","source":"catalog","models":[{"providerId":"ai-proxy","modelId":"model-a","operations":["responses"],"capabilityReady":true},{"providerId":"ai-proxy","modelId":"model-b","operations":["responses"],"capabilityReady":true}]}}`))
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/workorch/workspaces/ws-model/llm-selection":
			var target runtimecontract.ProviderModelTarget
			if err := json.NewDecoder(r.Body).Decode(&target); err != nil {
				t.Fatalf("decode remote target: %v", err)
			}
			remoteTarget = runtimecontract.CloneProviderModelTarget(&target)
			_ = json.NewEncoder(w).Encode(map[string]any{"workspace": map[string]any{
				"id": "ws-model", "title": "Workspace", "state": "active", "defaultLLMSelection": target,
			}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	for name, be := range map[string]port.Backend{
		"embedded": modelSelectionTransportBackend{Backend: processbackend.NewEventHub(hub)},
		"remote":   modelSelectionTransportBackend{Backend: processbackend.NewRemote(server.URL)},
	} {
		t.Run(name, func(t *testing.T) {
			embeddedTarget = nil
			remoteTarget = nil
			m := NewAppModel(be, "ws-model", "")
			m.workspaces = []projectioncontract.Workspace{{ID: "ws-model", Title: "Workspace", State: string(common.WorkspaceStateActive)}}
			m.setInput("/model")
			next, loadCmd := m.submitInput()
			m = appModelFromTea(t, next)
			if loadCmd == nil || !m.modelSelectMode {
				t.Fatalf("selection did not start: mode=%v cmd=%v", m.modelSelectMode, loadCmd)
			}

			next, _ = m.Update(loadCmd())
			m = appModelFromTea(t, next)
			if len(m.modelSelectOptions) != 2 || m.modelSelectOptions[0].ProviderID != "ai-proxy" || m.modelSelectOptions[1].ProviderID != "ai-proxy" {
				t.Fatalf("listed options = %+v", m.modelSelectOptions)
			}

			next, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyDown})
			m = appModelFromTea(t, next)
			next, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
			m = appModelFromTea(t, next)
			next, saveCmd := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
			m = appModelFromTea(t, next)
			if saveCmd == nil {
				t.Fatal("second Enter did not submit target")
			}
			next, _ = m.Update(saveCmd())
			m = appModelFromTea(t, next)
			if m.modelSelectMode || !strings.Contains(m.result, "ai-proxy/model-b") || !strings.Contains(m.result, "current-run sealed target: no active sealed selection") {
				t.Fatalf("saved selection result = mode:%v result:%q", m.modelSelectMode, m.result)
			}
			if name == "embedded" && (embeddedTarget == nil || embeddedTarget.Target.ProviderID != "ai-proxy" || embeddedTarget.Target.ModelID != "model-b") {
				t.Fatalf("embedded persisted target = %+v", embeddedTarget)
			}
			if name == "remote" && (remoteTarget == nil || remoteTarget.ProviderID != "ai-proxy" || remoteTarget.ModelID != "model-b") {
				t.Fatalf("remote persisted target = %+v", remoteTarget)
			}
		})
	}
}
