package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	decisioncommon "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/common"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestWriteBusinessErrorMarksOnlyExplicitPreDispatchCreationRejection(t *testing.T) {
	for _, marked := range []bool{false, true} {
		var failure error = common.ErrInvalidArgument
		if marked {
			failure = &workorchmanagementcontract.DefinitionAuthoringCreationRejectedBeforeDispatchError{Cause: failure}
		}
		recorder := httptest.NewRecorder()
		writeBusinessError(recorder, failure)
		var response errorResponse
		if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		if recorder.Code != http.StatusBadRequest || (response.Code == "workspace_creation_rejected_before_dispatch") != marked {
			t.Fatalf("status must not imply no persistence: %d %+v", recorder.Code, response)
		}
	}
}

func TestWriteBusinessErrorPreservesDeterministicSourceCauseAndKeepsOtherFailuresPrivate(t *testing.T) {
	cause := errors.New("required_evidence_purpose_invalid: 原来源\r\n完整错误")
	failure := runtimecontract.NewRuntimeSourceObligationConflict("historical_source_authentication", cause)
	if !errors.Is(failure, cause) || errors.Is(failure, common.ErrConflict) {
		t.Fatal("source cause lost or deterministic failure became a CAS/stale signal")
	}
	recorder := httptest.NewRecorder()
	writeBusinessError(recorder, failure)
	var response errorResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != http.StatusConflict || response.Error != failure.Error() || !strings.Contains(response.Error, cause.Error()) || response.Code != runtimecontract.RuntimeSourceObligationConflictCode || response.Stage != "historical_source_authentication" {
		t.Fatalf("source error not preserved: %d %+v", recorder.Code, response)
	}
	recorder = httptest.NewRecorder()
	writeBusinessError(recorder, errors.New("unclassified owner transport credential"))
	if recorder.Code != http.StatusInternalServerError || strings.Contains(recorder.Body.String(), "credential") {
		t.Fatal("source conflict classification exposed unrelated owner failure")
	}
	recorder = httptest.NewRecorder()
	creation := &workorchmanagementcontract.DefinitionAuthoringCreationError{Recovery: workorchmanagementcontract.DefinitionAuthoringCreationRecovery{WorkspaceID: "ws-create", RequestID: "original-create", Persistence: "persisted", Phase: "owner_persistence", RetrySameRequest: true}, Cause: failure}
	writeBusinessError(recorder, creation)
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != http.StatusServiceUnavailable || response.Code != "workspace_creation_incomplete" || response.CreationRecovery == nil || *response.CreationRecovery != creation.Recovery {
		t.Fatal("nested source error replaced original creation recovery identity")
	}
}

func TestWriteBusinessErrorPreservesCreationRecoveryWithoutPublishingCause(t *testing.T) {
	cause := errors.New("原始诊断 sk_test_do_not_publish\r\n")
	for _, persistence := range []string{"unknown", "persisted"} {
		t.Run(persistence, func(t *testing.T) {
			failure := &workorchmanagementcontract.DefinitionAuthoringCreationError{
				Recovery: workorchmanagementcontract.DefinitionAuthoringCreationRecovery{WorkspaceID: "ws-create", RequestID: "create-request", Persistence: persistence, Phase: "owner_persistence", RetrySameRequest: true}, Cause: cause,
			}
			if !errors.Is(failure, cause) {
				t.Fatal("internal cause lost")
			}
			recorder := httptest.NewRecorder()
			writeBusinessError(recorder, failure)
			var response errorResponse
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if recorder.Code != http.StatusServiceUnavailable || response.Code != "workspace_creation_incomplete" || response.CreationRecovery == nil || *response.CreationRecovery != failure.Recovery || response.Remediation == "" {
				t.Fatalf("missing recovery response: %d %+v", recorder.Code, response)
			}
			if strings.Contains(recorder.Body.String(), "sk_test_do_not_publish") {
				t.Fatal("diagnostic cause published through generic HTTP error")
			}
		})
	}
}

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
