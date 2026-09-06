package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func TestBuiltinDefaultRoutesUseTypedApplicationPorts(t *testing.T) {
	ports := completeApplicationPorts(t)
	item := workorchmanagementcontract.BuiltinDefault{Kind: "soul", ID: "builtin", Present: true, CurrentContentHash: "old", BuiltinContentHash: "new", CurrentBuiltinRevision: "revision", UpgradeAvailable: true}
	ports.Registry.ListBuiltinDefaults = func(context.Context, string, string) ([]workorchmanagementcontract.BuiltinDefault, error) {
		return []workorchmanagementcontract.BuiltinDefault{item}, nil
	}
	calls := 0
	ports.Registry.UpgradeBuiltinDefault = func(_ context.Context, _, _ string, request workorchmanagementcontract.BuiltinDefaultUpgrade) (workorchmanagementcontract.BuiltinDefaultUpgradeResult, error) {
		calls++
		if request.Kind != item.Kind || request.ID != item.ID || request.ExpectedContentHash != "old" || request.TargetContentHash != "new" {
			t.Fatalf("request not preserved: %+v", request)
		}
		return workorchmanagementcontract.BuiltinDefaultUpgradeResult{}, common.ErrConflict
	}
	state := newServerStateWithApplicationPorts(normalizeConfig(Config{Backend: "memory"}), ports, nil, nil)
	mux := newMuxWithStateNoStart(state)
	read := httptest.NewRecorder()
	mux.ServeHTTP(read, httptest.NewRequest(http.MethodGet, "/api/v1/workorch/builtin-defaults", nil))
	var body struct {
		Defaults []workorchmanagementcontract.BuiltinDefault `json:"defaults"`
	}
	if err := json.Unmarshal(read.Body.Bytes(), &body); err != nil || read.Code != http.StatusOK || len(body.Defaults) != 1 || body.Defaults[0] != item {
		t.Fatalf("read status=%d body=%s err=%v", read.Code, read.Body, err)
	}
	write := httptest.NewRecorder()
	mux.ServeHTTP(write, httptest.NewRequest(http.MethodPost, "/api/v1/workorch/builtin-defaults/upgrades", strings.NewReader(`{"kind":"soul","id":"builtin","expectedContentHash":"old","targetContentHash":"new"}`)))
	if write.Code != http.StatusConflict || calls != 1 {
		t.Fatalf("conflict status=%d body=%s calls=%d", write.Code, write.Body, calls)
	}
	invalid := httptest.NewRecorder()
	mux.ServeHTTP(invalid, httptest.NewRequest(http.MethodPost, "/api/v1/workorch/builtin-defaults/upgrades", strings.NewReader(`{"invalid`)))
	if invalid.Code != http.StatusBadRequest || calls != 1 {
		t.Fatalf("invalid JSON reached application: status=%d calls=%d", invalid.Code, calls)
	}
}
