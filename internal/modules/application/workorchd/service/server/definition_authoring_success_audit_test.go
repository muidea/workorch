package server

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	management "github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func TestDefinitionAuthoringSuccessAuditHTTPIsExplicitScopedAndNoStore(t *testing.T) {
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}))
	raw := []byte("完整审计\n\x00\xff")
	want := management.DefinitionAuthoringSuccessAuditView{
		WorkspaceID: "ws", PermitID: "proposal-permit", CanonicalHash: "canonical",
		Document: management.DefinitionProviderObservationView{
			DefinitionProviderObservationMetadata: management.DefinitionProviderObservationMetadata{Observed: true, Complete: true, Saved: true, ObservedBytes: uint64(len(raw)), SHA256: "exact-hash"},
			ContentBase64:                         base64.StdEncoding.EncodeToString(raw),
		},
	}
	calls := 0
	state.authoring.ReadSuccessAudit = func(_ context.Context, id, permit string) (management.DefinitionAuthoringSuccessAuditView, error) {
		calls++
		if id != want.WorkspaceID || permit != want.PermitID {
			t.Fatal("审计查询 scope 被改写")
		}
		return want, nil
	}
	state.authoring.ReadDiagnostic = func(context.Context, string, string) (management.DefinitionAuthoringDiagnosticView, error) {
		t.Fatal("成功原文不能借用失败诊断读取")
		return management.DefinitionAuthoringDiagnosticView{}, nil
	}
	handler := newMuxWithState(state)
	path := "/api/v1/workorch/workspaces/ws/authoring/success-byte-audits/proposal-permit"
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
	if recorder.Code != http.StatusOK || recorder.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("受限审计响应：%d %v", recorder.Code, recorder.Header())
	}
	var response definitionAuthoringSuccessAuditResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil || !reflect.DeepEqual(response.Audit, want) || calls != 1 {
		t.Fatalf("成功原文或审计身份变化：%+v %v，calls=%d", response, err, calls)
	}
	state.authoring.ReadSuccessAudit = func(context.Context, string, string) (management.DefinitionAuthoringSuccessAuditView, error) {
		return management.DefinitionAuthoringSuccessAuditView{}, common.ErrNotFound
	}
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
	if recorder.Code != http.StatusNotFound || recorder.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("缺失审计必须明确返回无缓存404：%d %v", recorder.Code, recorder.Header())
	}
}

func TestDefinitionAuthoringSuccessAuditPathRejectsScopeDrift(t *testing.T) {
	for _, path := range []string{
		"/api/v1/workorch/workspaces/ws/authoring/diagnostics/permit",
		"/api/v1/workorch/workspaces/ws/authoring/success-byte-audits",
		"/api/v1/workorch/workspaces/ws/authoring/success-byte-audits/",
		"/api/v1/workorch/workspaces//authoring/success-byte-audits/permit",
		"/api/v1/workorch/workspaces/ws/authoring/success-byte-audits/permit/extra",
	} {
		if _, _, ok := definitionAuthoringSuccessAuditFromPath(path); ok {
			t.Fatalf("非精确审计路径被接受：%s", path)
		}
	}
	id, permit, ok := definitionAuthoringSuccessAuditFromPath("/api/v1/workorch/workspaces/ws/authoring/success-byte-audits/permit")
	if !ok || id != "ws" || permit != "permit" {
		t.Fatal("精确审计 scope 丢失")
	}
}
