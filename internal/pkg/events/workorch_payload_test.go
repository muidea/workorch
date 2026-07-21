package events

import (
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/llmaudit"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func TestTypedEventPayloadDecodersRoundTrip(t *testing.T) {
	workspace := WorkspacePayloadData{
		Title:                "typed workspace",
		Description:          "typed description",
		DeliverableDirectory: "/tmp/typed",
		InternalDirectory:    "/tmp/typed/.workorch",
		State:                common.WorkspaceStateActive,
		ActiveRunID:          "run-typed",
	}
	decodedWorkspace, ok := DecodeWorkspaceEventPayload(workspace.EventPayload())
	if !ok || decodedWorkspace != workspace {
		t.Fatalf("workspace round trip = %+v/%v, want %+v", decodedWorkspace, ok, workspace)
	}

	run := RunPayloadData{
		GoalID:                  "goal-typed",
		DoDID:                   "dod-typed",
		State:                   common.RunStateBlockedOnDecision,
		BlockingTicketID:        "ticket-typed",
		BlockingApprovalID:      "approval-typed",
		FailureReason:           "waiting",
		RuntimeStateRevision:    7,
		RuntimeConvergenceState: "need_governance",
		RuntimeAction:           "request_governance",
		RuntimeFocusID:          "focus-typed",
		AuthoritySnapshotID:     "authority-typed",
		RuntimeRepairAttempts:   3,
	}
	decodedRun, ok := DecodeRunEventPayload(run.EventPayload())
	if !ok || decodedRun != run {
		t.Fatalf("run round trip = %+v/%v, want %+v", decodedRun, ok, run)
	}

	resolution := DecisionResolutionPayloadData{
		DecisionTicketPayloadData: DecisionTicketPayloadData{Reason: "choose", State: common.DecisionTicketStateResolved},
		ResolutionID:              "resolution-typed",
		Type:                      "resume",
		Payload:                   workorcheventpayload.New(workorcheventpayload.StringField("actor", "operator")),
		ContinuationAction:        "resume",
		ResumeRunID:               "run-typed",
		SuccessorRunID:            "run-successor",
	}
	decodedResolution, ok := DecodeDecisionResolutionEventPayload(resolution.EventPayload())
	if !ok || decodedResolution.ResolutionID != resolution.ResolutionID || decodedResolution.Type != resolution.Type || !decodedResolution.Payload.Equal(resolution.Payload) {
		t.Fatalf("resolution round trip = %+v/%v, want %+v", decodedResolution, ok, resolution)
	}

	approval := ApprovalPayloadData{
		Reason:  "approve",
		Action:  "apply",
		Context: workorcheventpayload.New(workorcheventpayload.StringField("risk", "medium")),
		State:   common.ApprovalStateApproved,
	}
	decodedApproval, ok := DecodeApprovalEventPayload(approval.EventPayload())
	if !ok || decodedApproval.Reason != approval.Reason || decodedApproval.Action != approval.Action || decodedApproval.State != approval.State || !decodedApproval.Context.Equal(approval.Context) {
		t.Fatalf("approval round trip = %+v/%v, want %+v", decodedApproval, ok, approval)
	}
}

func TestTypedPatchAndGovernancePayloadDecodersRoundTrip(t *testing.T) {
	patch := PatchPayloadData{
		PatchID:      "patch-1",
		StepID:       "run-1/step-1",
		ToolCallID:   "run-1/step-1/tool-call",
		PatchType:    "unified_diff",
		TargetRef:    "main.go",
		State:        common.PatchStateApplied,
		AddedLines:   2,
		DeletedLines: 1,
		DiffHash:     "diff-sha",
		ApprovalID:   "approval-1",
		Risk:         "sensitive",
	}
	decodedPatch, ok := decodePatchEventPayload(patch.EventPayload())
	if !ok || decodedPatch != patch {
		t.Fatalf("patch round trip = %+v/%v, want %+v", decodedPatch, ok, patch)
	}

	governance := AutoGovernancePayloadData{
		TraceID: "gov-trace-1", ItemType: "approval", ItemID: "approval-1", Action: "approve",
		RiskLevel: "L1", Confidence: 0.91, PolicyMatched: "read_only", BudgetBefore: 2, BudgetAfter: 1,
		LLMProviderID: "local", LLMModel: "qwen", LLMDecisionPreview: "allow read-only action",
		PromptFingerprint: "prompt-fingerprint", Status: "resolved", Reason: "policy accepted",
	}
	decodedGovernance, ok := decodeGovernanceEventPayload(governance.EventPayload())
	if !ok || decodedGovernance != governance {
		t.Fatalf("governance round trip = %+v/%v, want %+v", decodedGovernance, ok, governance)
	}
}

func TestLLMUsagePayloadUsesTypedAuditDTOs(t *testing.T) {
	payload := (LLMUsagePayloadData{
		WorkspaceID: "ws-audit", RunID: "run-audit", Status: "completed",
		ContextBudget: llmaudit.ContextBudget{
			ProviderID: "ai-proxy", ModelID: "chat-a", ActualTokens: 321,
			StablePrefixHash: "stable-audit", ProviderSubmitted: true,
		},
		CacheUsage: llmaudit.CacheUsage{CacheReadTokens: 120, Estimated: false, Source: "provider_native"},
	}).EventPayload()
	budget, ok := payload.Object(PayloadKeyContextBudget)
	if !ok {
		t.Fatal("context budget object is missing")
	}
	if providerID, _ := budget.String("providerId"); providerID != "ai-proxy" {
		t.Fatalf("providerId = %q", providerID)
	}
	if submitted, _ := budget.Bool("providerSubmitted"); !submitted {
		t.Fatal("providerSubmitted was not preserved")
	}
	cache, ok := payload.Object(PayloadKeyCacheUsage)
	if !ok {
		t.Fatal("cache usage object is missing")
	}
	if read, _ := cache.Int64("cacheReadTokens"); read != 120 {
		t.Fatalf("cacheReadTokens = %d", read)
	}
}

func decodePatchEventPayload(payload workorcheventpayload.Payload) (PatchPayloadData, bool) {
	state, ok := payload.String(PayloadKeyState)
	if !ok {
		return PatchPayloadData{}, false
	}
	patchType, _ := payload.String(PayloadKeyPatchType)
	targetRef, _ := payload.String(PayloadKeyTargetRef)
	patchID, _ := payload.String(PayloadKeyPatchID)
	stepID, _ := payload.String(PayloadKeyStepID)
	toolCallID, _ := payload.String(PayloadKeyToolCallID)
	diffHash, _ := payload.String(PayloadKeyDiffHash)
	approvalID, _ := payload.String(PayloadKeyApprovalID)
	risk, _ := payload.String(PayloadKeyRisk)
	addedLines, _ := payload.Int64(PayloadKeyAddedLines)
	deletedLines, _ := payload.Int64(PayloadKeyDeletedLines)
	return PatchPayloadData{
		PatchID: common.PatchID(patchID), StepID: common.StepID(stepID), ToolCallID: common.ToolCallID(toolCallID),
		PatchType: patchType, TargetRef: targetRef, State: common.PatchState(state), AddedLines: int(addedLines),
		DeletedLines: int(deletedLines), DiffHash: diffHash, ApprovalID: common.ApprovalID(approvalID), Risk: risk,
	}, true
}

func decodeGovernanceEventPayload(payload workorcheventpayload.Payload) (AutoGovernancePayloadData, bool) {
	traceID, _ := payload.String(PayloadKeyTraceID)
	itemType, _ := payload.String(PayloadKeyItemType)
	itemID, _ := payload.String(PayloadKeyItemID)
	action, _ := payload.String(PayloadKeyAction)
	riskLevel, _ := payload.String(PayloadKeyRisk)
	confidence, _ := payload.Float64(PayloadKeyConfidenceScore)
	policyMatched, _ := payload.String(PayloadKeyPolicyMatched)
	policyBlocked, _ := payload.String(PayloadKeyPolicyBlocked)
	budgetBefore, _ := payload.Int64(PayloadKeyBudgetBefore)
	budgetAfter, _ := payload.Int64(PayloadKeyBudgetAfter)
	providerID, _ := payload.String(PayloadKeyLLMProviderID)
	model, _ := payload.String(PayloadKeyLLMModel)
	preview, _ := payload.String(PayloadKeyLLMDecisionPreview)
	fingerprint, _ := payload.String(PayloadKeyPromptFingerprint)
	status, _ := payload.String(PayloadKeyStatus)
	reason, _ := payload.String(PayloadKeyReason)
	return AutoGovernancePayloadData{
		TraceID: common.AutoGovernanceTraceID(traceID), ItemType: itemType, ItemID: itemID, Action: action,
		RiskLevel: riskLevel, Confidence: confidence, PolicyMatched: policyMatched, PolicyBlocked: policyBlocked,
		BudgetBefore: int(budgetBefore), BudgetAfter: int(budgetAfter), LLMProviderID: providerID, LLMModel: model,
		LLMDecisionPreview: preview, PromptFingerprint: fingerprint, Status: status, Reason: reason,
	}, true
}
