package events

import (
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestWorkspacePayloadIncludesExpectedFields(t *testing.T) {
	payload := WorkspacePayload("ws-title", "workspace description", "/tmp/workorch/ws-title", "", common.WorkspaceStateActive, common.RunID("run-1"))

	if payload[PayloadKeyTitle] != "ws-title" {
		t.Fatalf("title = %v, want ws-title", payload[PayloadKeyTitle])
	}
	if payload[PayloadKeyDescription] != "workspace description" {
		t.Fatalf("description = %v, want workspace description", payload[PayloadKeyDescription])
	}
	if payload[PayloadKeyDeliverableDirectory] != "/tmp/workorch/ws-title" {
		t.Fatalf("deliverableDirectory = %v, want /tmp/workorch/ws-title", payload[PayloadKeyDeliverableDirectory])
	}
	if payload[PayloadKeyState] != string(common.WorkspaceStateActive) {
		t.Fatalf("state = %v, want %s", payload[PayloadKeyState], common.WorkspaceStateActive)
	}
	if payload[PayloadKeyActiveRunID] != "run-1" {
		t.Fatalf("activeRunId = %v, want run-1", payload[PayloadKeyActiveRunID])
	}
}

func TestDecisionResolutionPayloadIncludesResolutionFields(t *testing.T) {
	payload := DecisionResolutionPayload(
		"manual gate",
		common.DecisionTicketStateResolved,
		common.ResolutionID("resolution-1"),
		"approve-plan",
		map[string]any{"operator": "workorch"},
		DecisionResolutionPayloadData{
			ContinuationAction: "resume",
			ResumeRunID:        "run-current",
			SuccessorRunID:     "run-successor",
		},
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
	decoded, ok := DecodeDecisionResolutionPayload(payload)
	if !ok {
		t.Fatalf("DecodeDecisionResolutionPayload failed")
	}
	if decoded.ContinuationAction != "resume" || decoded.ResumeRunID != "run-current" || decoded.SuccessorRunID != "run-successor" {
		t.Fatalf("decoded refs = %+v, want recovery refs", decoded)
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

func TestDecodePatchPayloadRoundTrip(t *testing.T) {
	payload := PatchPayload(
		common.PatchID("patch-1"),
		common.StepID("run-1/step-1"),
		common.ToolCallID("run-1/step-1/tool-call"),
		"unified_diff",
		"main.go",
		"update main",
		common.PatchStateApplied,
		2,
		1,
		"",
		"diff-sha",
		common.ApprovalID("approval-1"),
		"sensitive",
		"",
	)

	decoded, ok := DecodePatchPayload(payload)
	if !ok {
		t.Fatalf("DecodePatchPayload returned !ok")
	}
	if decoded.PatchID != "patch-1" ||
		decoded.StepID != "run-1/step-1" ||
		decoded.ToolCallID != "run-1/step-1/tool-call" ||
		decoded.PatchType != "unified_diff" ||
		decoded.TargetRef != "main.go" ||
		decoded.State != common.PatchStateApplied ||
		decoded.AddedLines != 2 ||
		decoded.DeletedLines != 1 ||
		decoded.DiffHash != "diff-sha" ||
		decoded.ApprovalID != "approval-1" ||
		decoded.Risk != "sensitive" {
		t.Fatalf("decoded patch payload = %+v", decoded)
	}
}

func TestDecodeAutoGovernancePayloadRoundTrip(t *testing.T) {
	payload := AutoGovernancePayload(
		common.AutoGovernanceTraceID("gov-trace-1"),
		"approval",
		"approval-1",
		"approve",
		"L1",
		0.91,
		"read_only",
		"",
		2,
		1,
		"local",
		"qwen",
		"allow read-only action",
		"prompt-fingerprint",
		"resolved",
		"policy accepted",
		"",
	)

	decoded, ok := DecodeAutoGovernancePayload(payload)
	if !ok {
		t.Fatalf("DecodeAutoGovernancePayload returned !ok")
	}
	if decoded.TraceID != "gov-trace-1" ||
		decoded.ItemType != "approval" ||
		decoded.ItemID != "approval-1" ||
		decoded.Action != "approve" ||
		decoded.RiskLevel != "L1" ||
		decoded.Confidence != 0.91 ||
		decoded.PolicyMatched != "read_only" ||
		decoded.BudgetBefore != 2 ||
		decoded.BudgetAfter != 1 ||
		decoded.PromptFingerprint != "prompt-fingerprint" ||
		decoded.LLMProviderID != "local" ||
		decoded.LLMModel != "qwen" ||
		decoded.Status != "resolved" ||
		decoded.Reason != "policy accepted" {
		t.Fatalf("decoded auto governance payload = %+v", decoded)
	}
}
