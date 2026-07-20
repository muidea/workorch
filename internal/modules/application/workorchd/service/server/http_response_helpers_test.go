package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	decisioncommon "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/common"
)

func TestWriteBusinessErrorMapsTicketActionConflictToActionableHTTP409(t *testing.T) {
	recorder := httptest.NewRecorder()
	writeBusinessError(recorder, decisioncommon.NewResolutionActionConflict("ticket-runtime-409", decisioncommon.ResolutionTypeReplan, []string{decisioncommon.ResolutionTypeProvideMissingInfo, decisioncommon.ResolutionTypeRejectCurrentPath}))
	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409, body=%s", recorder.Code, recorder.Body.String())
	}
	var response errorResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Code != "runtime_decision_action_unsupported" || response.TicketID != "ticket-runtime-409" || response.RequestedResolutionType != decisioncommon.ResolutionTypeReplan || len(response.AllowedActions) != 2 || response.AllowedActions[0] != decisioncommon.ResolutionTypeProvideMissingInfo || response.Remediation == "" {
		t.Fatalf("response = %+v, want actionable ticket conflict", response)
	}
}
