package workorchclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	management "github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func TestDefinitionAuthoringClientPreservesExactTransactions(t *testing.T) {
	confirmation := management.DefinitionAuthoringConfirm{InputHash: "input", DerivationPermitID: "permit", ProposalHash: "proposal", SpecificationHash: "spec", ExpectedWorkspaceRevision: 8}
	directory := "/project"
	save := management.DefinitionAuthoringSave{RequestID: "original-save", TemplateID: "t", DeliverableDirectory: &directory, Input: management.DefinitionAuthoringInput{GoalText: "目标", AcceptanceRequirements: []string{"验收"}, ResourceRefs: []string{"input.txt"}}, ExpectedInputHash: "input", ExpectedWorkspaceRevision: 8}
	requests := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.Path)
		if r.URL.Path == authoringPath("ws")+"/confirm" {
			var got management.DefinitionAuthoringConfirm
			if err := json.NewDecoder(r.Body).Decode(&got); err != nil || !reflect.DeepEqual(got, confirmation) {
				t.Errorf("原确认身份变化: %+v %v", got, err)
			}
		}
		if r.Method == http.MethodPut && r.URL.Path == authoringPath("ws") {
			var got management.DefinitionAuthoringSave
			if err := json.NewDecoder(r.Body).Decode(&got); err != nil || !reflect.DeepEqual(got, save) {
				t.Errorf("原保存身份变化: %+v %v", got, err)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == authoringPath("ws")+"/compile" {
			w.WriteHeader(http.StatusAccepted)
		}
		_ = json.NewEncoder(w).Encode(struct {
			Authoring management.DefinitionAuthoringView `json:"authoring"`
		}{Authoring: management.DefinitionAuthoringView{WorkspaceID: "ws", WorkspaceRevision: 8, State: "draft"}})
	}))
	defer server.Close()
	client := New(server.URL)
	ctx := context.Background()
	calls := []func() (*management.DefinitionAuthoringView, error){
		func() (*management.DefinitionAuthoringView, error) {
			return client.CreateDefinitionAuthoring(ctx, management.DefinitionAuthoringCreate{RequestID: "stable"})
		},
		func() (*management.DefinitionAuthoringView, error) { return client.ReadDefinitionAuthoring(ctx, "ws") },
		func() (*management.DefinitionAuthoringView, error) {
			return client.SaveDefinitionAuthoring(ctx, "ws", save)
		},
		func() (*management.DefinitionAuthoringView, error) {
			return client.CompileDefinitionAuthoring(ctx, "ws")
		},
		func() (*management.DefinitionAuthoringView, error) {
			return client.ConfirmDefinitionAuthoring(ctx, "ws", confirmation)
		},
	}
	for _, call := range calls {
		view, err := call()
		if err != nil || view.WorkspaceID != "ws" || view.State != "draft" {
			t.Fatalf("创作响应被重判: %+v %v", view, err)
		}
	}
	want := []string{"POST /api/v1/workorch/workspace-authoring", "GET " + authoringPath("ws"), "PUT " + authoringPath("ws"), "POST " + authoringPath("ws") + "/compile", "POST " + authoringPath("ws") + "/confirm"}
	if !reflect.DeepEqual(requests, want) {
		t.Fatalf("创作请求路径变化: %v", requests)
	}
}
