package events

import (
	"testing"

	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
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

func TestStepPayloadCanonicalizesWorkspacePlanKind(t *testing.T) {
	payload := StepPayload(llmcommon.StepKindWorkspacePlan, common.StepStateCompleted, nil, nil)
	if payload[PayloadKeyKind] != llmcommon.StepKindWorkspaceExecution {
		t.Fatalf("kind = %v, want %s", payload[PayloadKeyKind], llmcommon.StepKindWorkspaceExecution)
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

func TestDecodeStepPayloadCanonicalizesWorkspacePlanKind(t *testing.T) {
	decoded, ok := DecodeStepPayload(map[string]any{
		PayloadKeyKind:  llmcommon.StepKindWorkspacePlan,
		PayloadKeyState: string(common.StepStateCompleted),
	})
	if !ok {
		t.Fatalf("DecodeStepPayload returned !ok")
	}
	if decoded.Kind != llmcommon.StepKindWorkspaceExecution {
		t.Fatalf("decoded.Kind = %q, want %q", decoded.Kind, llmcommon.StepKindWorkspaceExecution)
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

func TestDecodeLLMStreamProgressPayloadRoundTrip(t *testing.T) {
	payload := LLMStreamProgressPayload(
		common.WorkspaceID("ws-1"),
		common.RunID("run-1"),
		common.StepID("run-1/step-1"),
		"provider-a",
		"model-a",
		6,
		1024,
		380,
		"completed",
		"llm_execution_graph_plan - completed",
	)

	decoded, ok := DecodeLLMStreamProgressPayload(payload)
	if !ok {
		t.Fatalf("DecodeLLMStreamProgressPayload returned !ok")
	}
	if decoded.WorkspaceID != "ws-1" ||
		decoded.RunID != "run-1" ||
		decoded.StepID != "run-1/step-1" ||
		decoded.ProviderID != "provider-a" ||
		decoded.Model != "model-a" ||
		decoded.ChunkCount != 6 ||
		decoded.ReceivedBytes != 1024 ||
		decoded.ElapsedMs != 380 ||
		decoded.Status != "completed" ||
		decoded.Summary != "llm_execution_graph_plan - completed" {
		t.Fatalf("decoded llm stream progress payload = %+v", decoded)
	}
}

func TestDecodeLLMUsagePayloadRoundTrip(t *testing.T) {
	payload := LLMUsagePayload(LLMUsagePayloadData{
		WorkspaceID:   common.WorkspaceID("ws-1"),
		RunID:         common.RunID("run-1"),
		StepID:        common.StepID("run-1/step-1"),
		ProviderID:    "provider-a",
		Provider:      "compatible",
		APISurface:    "openai_compatible_http",
		Model:         "model-a",
		Operation:     "chat_completions",
		InputTokens:   120,
		OutputTokens:  32,
		TotalTokens:   152,
		Estimated:     true,
		UsageSource:   "estimated",
		RequestBytes:  2048,
		ResponseBytes: 512,
		Streaming:     true,
		ContextBudget: map[string]any{"maxBytes": 4096},
		CacheUsage:    map[string]any{"cacheReadTokens": 12, "source": "provider_native"},
		CacheMarkers:  map[string]any{"autoPrefixCacheExpected": true, "renderedStablePrefixHash": "sha256:test"},
		StartedAt:     "2026-05-07T07:00:00Z",
		CompletedAt:   "2026-05-07T07:00:01Z",
		DurationMs:    1000,
		Status:        "completed",
	})
	decoded, ok := DecodeLLMUsagePayload(payload)
	if !ok {
		t.Fatalf("DecodeLLMUsagePayload returned !ok")
	}
	if decoded.WorkspaceID != "ws-1" ||
		decoded.RunID != "run-1" ||
		decoded.StepID != "run-1/step-1" ||
		decoded.ProviderID != "provider-a" ||
		decoded.Provider != "compatible" ||
		decoded.APISurface != "openai_compatible_http" ||
		decoded.Model != "model-a" ||
		decoded.Operation != "chat_completions" ||
		decoded.InputTokens != 120 ||
		decoded.OutputTokens != 32 ||
		decoded.TotalTokens != 152 ||
		!decoded.Estimated ||
		decoded.UsageSource != "estimated" ||
		decoded.RequestBytes != 2048 ||
		decoded.ResponseBytes != 512 ||
		!decoded.Streaming ||
		decoded.CacheUsage["cacheReadTokens"] != 12 ||
		decoded.CacheUsage["source"] != "provider_native" ||
		decoded.CacheMarkers["autoPrefixCacheExpected"] != true ||
		decoded.CacheMarkers["renderedStablePrefixHash"] != "sha256:test" ||
		decoded.DurationMs != 1000 ||
		decoded.Status != "completed" {
		t.Fatalf("decoded llm usage payload = %+v", decoded)
	}
}
