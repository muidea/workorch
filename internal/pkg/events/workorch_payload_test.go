package events

import (
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestWorkspacePayloadIncludesExpectedFields(t *testing.T) {
	payload := WorkspacePayload("ws-title", "workspace description", "/tmp/workorch/ws-title", common.WorkspaceStateActive, common.RunID("run-1"))

	if payload[PayloadKeyTitle] != "ws-title" {
		t.Fatalf("title = %v, want ws-title", payload[PayloadKeyTitle])
	}
	if payload[PayloadKeyDescription] != "workspace description" {
		t.Fatalf("description = %v, want workspace description", payload[PayloadKeyDescription])
	}
	if payload[PayloadKeyWorkingDirectory] != "/tmp/workorch/ws-title" {
		t.Fatalf("workingDirectory = %v, want /tmp/workorch/ws-title", payload[PayloadKeyWorkingDirectory])
	}
	if payload[PayloadKeyState] != string(common.WorkspaceStateActive) {
		t.Fatalf("state = %v, want %s", payload[PayloadKeyState], common.WorkspaceStateActive)
	}
	if payload[PayloadKeyActiveRunID] != "run-1" {
		t.Fatalf("activeRunId = %v, want run-1", payload[PayloadKeyActiveRunID])
	}
}

func TestStepPayloadClonesMaps(t *testing.T) {
	input := map[string]any{"workspaceId": "ws-1"}
	output := map[string]any{"status": "completed"}

	payload := StepPayload("llm/workspace_execution", common.StepStateCompleted, input, output)
	input["workspaceId"] = "ws-mutated"
	output["status"] = "mutated"

	gotInput, ok := payload[PayloadKeyInput].(map[string]any)
	if !ok {
		t.Fatalf("input payload type = %T, want map[string]any", payload[PayloadKeyInput])
	}
	gotOutput, ok := payload[PayloadKeyOutput].(map[string]any)
	if !ok {
		t.Fatalf("output payload type = %T, want map[string]any", payload[PayloadKeyOutput])
	}
	if gotInput["workspaceId"] != "ws-1" {
		t.Fatalf("input workspaceId = %v, want ws-1", gotInput["workspaceId"])
	}
	if gotOutput["status"] != "completed" {
		t.Fatalf("output status = %v, want completed", gotOutput["status"])
	}
}

func TestDecisionResolutionPayloadIncludesResolutionFields(t *testing.T) {
	payload := DecisionResolutionPayload(
		"manual gate",
		common.DecisionTicketStateResolved,
		common.ResolutionID("resolution-1"),
		"approve-plan",
		map[string]any{"operator": "workorch"},
	)

	if payload[PayloadKeyReason] != "manual gate" {
		t.Fatalf("reason = %v, want manual gate", payload[PayloadKeyReason])
	}
	if payload[PayloadKeyState] != string(common.DecisionTicketStateResolved) {
		t.Fatalf("state = %v, want %s", payload[PayloadKeyState], common.DecisionTicketStateResolved)
	}
	if payload[PayloadKeyResolutionID] != "resolution-1" {
		t.Fatalf("resolutionId = %v, want resolution-1", payload[PayloadKeyResolutionID])
	}
	if payload[PayloadKeyType] != "approve-plan" {
		t.Fatalf("type = %v, want approve-plan", payload[PayloadKeyType])
	}
	resolutionPayload, ok := payload[PayloadKeyPayload].(map[string]any)
	if !ok {
		t.Fatalf("payload type = %T, want map[string]any", payload[PayloadKeyPayload])
	}
	if resolutionPayload["operator"] != "workorch" {
		t.Fatalf("resolution payload operator = %v, want workorch", resolutionPayload["operator"])
	}
}

func TestDecodeStepPayloadRejectsInvalidOutputType(t *testing.T) {
	_, ok := DecodeStepPayload(map[string]any{
		PayloadKeyKind:   "llm/workspace_execution",
		PayloadKeyState:  string(common.StepStateCompleted),
		PayloadKeyOutput: "invalid",
	})
	if ok {
		t.Fatalf("DecodeStepPayload should reject non-map output")
	}
}

func TestDecodeDecisionResolutionPayloadRoundTrip(t *testing.T) {
	payload := DecisionResolutionPayload(
		"manual gate",
		common.DecisionTicketStateResolved,
		common.ResolutionID("resolution-1"),
		"approve-plan",
		map[string]any{"operator": "workorch"},
	)

	decoded, ok := DecodeDecisionResolutionPayload(payload)
	if !ok {
		t.Fatalf("DecodeDecisionResolutionPayload returned !ok")
	}
	if decoded.Reason != "manual gate" {
		t.Fatalf("reason = %s, want manual gate", decoded.Reason)
	}
	if decoded.State != common.DecisionTicketStateResolved {
		t.Fatalf("state = %s, want %s", decoded.State, common.DecisionTicketStateResolved)
	}
	if decoded.ResolutionID != common.ResolutionID("resolution-1") {
		t.Fatalf("resolutionId = %s, want resolution-1", decoded.ResolutionID)
	}
	if decoded.Type != "approve-plan" {
		t.Fatalf("type = %s, want approve-plan", decoded.Type)
	}
	if decoded.Payload["operator"] != "workorch" {
		t.Fatalf("payload operator = %v, want workorch", decoded.Payload["operator"])
	}
}
