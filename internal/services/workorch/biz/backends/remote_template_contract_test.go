package backend

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func TestRemoteTemplateCreateRemainsAvailable(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/workorch/workspace-templates" {
			t.Errorf("模板创建请求 = %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		var template workorchmanagementcontract.WorkspaceTemplate
		if err := json.NewDecoder(r.Body).Decode(&template); err != nil {
			t.Errorf("decode template: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"workspaceTemplate": template})
	}))
	defer server.Close()
	created, err := NewRemote(server.URL).Template().Create(context.Background(), &workorchmanagementcontract.WorkspaceTemplate{ID: "template-terminal", Name: "终端模板"})
	if err != nil || created == nil || created.ID != "template-terminal" || created.Name != "终端模板" || requests != 1 {
		t.Fatalf("保留模板创建失败: created=%+v requests=%d error=%v", created, requests, err)
	}
}

func TestRemoteTemplateContractRoundTripPreservesContextHints(t *testing.T) {
	hints := map[string]any{
		"profileContracts": map[string]any{
			"software-dev": map[string]any{"evidenceRequirements": []any{map[string]any{"scope": "dodItem", "id": "dod-exact"}}},
		},
	}

	wire, err := json.Marshal(&workorchmanagementcontract.WorkspaceTemplate{ID: "template", Name: "Template", ContextHints: hints})
	if err != nil {
		t.Fatalf("marshal remote template: %v", err)
	}
	var decoded workorchmanagementcontract.WorkspaceTemplate
	if err := json.Unmarshal(wire, &decoded); err != nil {
		t.Fatalf("unmarshal remote template: %v", err)
	}
	profiles, ok := decoded.ContextHints["profileContracts"].(map[string]any)
	if !ok || profiles["software-dev"] == nil {
		t.Fatalf("ContextHints = %#v, want profileContracts preserved", decoded.ContextHints)
	}
}
