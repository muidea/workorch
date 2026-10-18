package workorchclient

import (
	"context"
	"encoding/base64"
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
			var got management.DefinitionAuthoringCompile
			if err := json.NewDecoder(r.Body).Decode(&got); err != nil || got.ExpectedInputHash != "input" || got.ExpectedTemplateSnapshotHash != "template-snapshot" {
				t.Errorf("compile input precondition changed: %+v %v", got, err)
			}
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
			return client.CompileDefinitionAuthoring(ctx, "ws", management.DefinitionAuthoringCompile{ExpectedInputHash: "input", ExpectedTemplateSnapshotHash: "template-snapshot"})
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

func TestDefinitionAuthoringClientPreparationDiagnosticPreservesIndependentScope(t *testing.T) {
	raw := []byte("原始错误\n\xff\x00")
	want := management.DefinitionAuthoringPreparationDiagnosticView{
		DefinitionAuthoringPreparationDiagnosticMetadata: management.DefinitionAuthoringPreparationDiagnosticMetadata{
			WorkspaceID: "ws", DiagnosticID: "preparation-sha256:exact", InputHash: "input", TemplateSnapshotHash: "template",
			ObservedWorkspaceRevision: 4, Stage: "review", TargetRequestID: "review/dimension_coverage/derivation",
			DerivationPermitID: "derivation", ReviewKind: "dimension_coverage", ProposalHash: "proposal",
			PhaseCode: "request_render_failed", CauseBytes: uint64(len(raw)), CauseSHA256: "exact-hash", CauseSaved: true,
		},
		CauseBase64: base64.StdEncoding.EncodeToString(raw),
	}
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != http.MethodGet || r.URL.Path != authoringPath("ws")+"/preparation-diagnostics/"+want.DiagnosticID {
			t.Errorf("前置诊断不能混入 provider permit 路径：%s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(struct {
			Diagnostic management.DefinitionAuthoringPreparationDiagnosticView `json:"diagnostic"`
		}{Diagnostic: want})
	}))
	defer server.Close()
	got, err := New(server.URL).ReadDefinitionAuthoringPreparationDiagnostic(context.Background(), "ws", want.DiagnosticID)
	if err != nil || got == nil || !reflect.DeepEqual(*got, want) || calls != 1 {
		t.Fatalf("诊断字节或身份被改写：%+v %v calls=%d", got, err, calls)
	}
	decoded, err := base64.StdEncoding.DecodeString(got.CauseBase64)
	if err != nil || !reflect.DeepEqual(decoded, raw) {
		t.Fatalf("诊断原字节不能可逆恢复：%x %v", decoded, err)
	}
}
