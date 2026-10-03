package server

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	management "github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func TestDefinitionAuthoringPreparationDiagnosticHTTPUsesIndependentScope(t *testing.T) {
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}))
	raw := []byte("原始资源错误\n\x00\xff")
	want := management.DefinitionAuthoringPreparationDiagnosticView{
		DefinitionAuthoringPreparationDiagnosticMetadata: management.DefinitionAuthoringPreparationDiagnosticMetadata{
			WorkspaceID: "ws", DiagnosticID: "preparation-sha256:exact", InputHash: "input", TemplateSnapshotHash: "template",
			ObservedWorkspaceRevision: 2, Stage: "derivation", TargetRequestID: "derive/input", PhaseCode: "resources",
			ResourceRef: "prd.md", CauseSHA256: "exact-hash", CauseBytes: uint64(len(raw)), CauseSaved: true, Current: true,
		},
		CauseBase64: base64.StdEncoding.EncodeToString(raw),
	}
	calls := 0
	state.authoring.ReadPreparationDiagnostic = func(_ context.Context, workspaceID, diagnosticID string) (management.DefinitionAuthoringPreparationDiagnosticView, error) {
		calls++
		if workspaceID != want.WorkspaceID || diagnosticID != want.DiagnosticID {
			t.Fatalf("前置诊断 scope 被改写：%q %q", workspaceID, diagnosticID)
		}
		return want, nil
	}
	state.authoring.ReadDiagnostic = func(context.Context, string, string) (management.DefinitionAuthoringDiagnosticView, error) {
		t.Fatal("非 permit 诊断不能进入 provider permit 查询")
		return management.DefinitionAuthoringDiagnosticView{}, nil
	}
	handler := newMuxWithState(state)
	path := "/api/v1/workorch/workspaces/ws/authoring/preparation-diagnostics/" + want.DiagnosticID
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
	if recorder.Code != http.StatusOK || recorder.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("前置诊断响应：%d %v %s", recorder.Code, recorder.Header(), recorder.Body.String())
	}
	var response definitionAuthoringPreparationDiagnosticResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil || !reflect.DeepEqual(response.Diagnostic, want) {
		t.Fatalf("诊断原文或身份变化：%+v %v", response, err)
	}
	if calls != 1 || strings.Contains(recorder.Body.String(), "permitId") || strings.Contains(recorder.Body.String(), "parseError") {
		t.Fatalf("前置诊断混入规格请求身份：%s，calls=%d", recorder.Body.String(), calls)
	}

	state.authoring.ReadPreparationDiagnostic = func(context.Context, string, string) (management.DefinitionAuthoringPreparationDiagnosticView, error) {
		return management.DefinitionAuthoringPreparationDiagnosticView{}, common.ErrNotFound
	}
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
	if recorder.Code != http.StatusNotFound || recorder.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("缺失诊断应明确返回无缓存404：%d %v", recorder.Code, recorder.Header())
	}
}

func TestDefinitionAuthoringPreparationDiagnosticPathRejectsScopeDrift(t *testing.T) {
	for _, path := range []string{
		"/api/v1/workorch/workspaces/ws/authoring/diagnostics/diagnostic",
		"/api/v1/workorch/workspaces/ws/authoring/preparation-diagnostics",
		"/api/v1/workorch/workspaces/ws/authoring/preparation-diagnostics/",
		"/api/v1/workorch/workspaces//authoring/preparation-diagnostics/diagnostic",
		"/api/v1/workorch/workspaces/ws/authoring/preparation-diagnostics/diagnostic/extra",
	} {
		if _, _, ok := definitionAuthoringPreparationDiagnosticFromPath(path); ok {
			t.Fatalf("非精确诊断路径被接受：%s", path)
		}
	}
	workspaceID, diagnosticID, ok := definitionAuthoringPreparationDiagnosticFromPath("/api/v1/workorch/workspaces/ws/authoring/preparation-diagnostics/preparation-sha256:exact")
	if !ok || workspaceID != "ws" || diagnosticID != "preparation-sha256:exact" {
		t.Fatalf("精确诊断路径丢失身份：%q %q %v", workspaceID, diagnosticID, ok)
	}
}
