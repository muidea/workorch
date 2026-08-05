package workorchport

import (
	"testing"

	"github.com/muidea/workorch/internal/pkg/llmaudit"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	"github.com/muidea/workorch/internal/pkg/workorchresultcontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestCompleteProjectionContractsPreserveOriginalFieldsAndDetachStorage(t *testing.T) {
	acceptance := projectioncontract.AcceptanceProjection{
		CapabilityStates: []projectioncontract.RuntimeCapabilityState{{
			RequirementID: "requirement-1", InvalidatedByReceiptRefs: []string{"receipt-1"},
			RequiredOperationSequence: []projectioncontract.RuntimeRequiredOperationState{{RequirementID: "operation-1", DependsOnRequirementIDs: []string{"operation-0"}}},
		}},
		Governance: []projectioncontract.RuntimeGovernanceState{{TicketID: "ticket-1"}},
		Operations: []projectioncontract.RuntimeOperationStatus{{OperationID: "operation-1"}},
	}
	gotAcceptance := AcceptanceProjectionInfoFromProjection(acceptance)
	acceptance.CapabilityStates[0].InvalidatedByReceiptRefs[0] = "mutated"
	acceptance.CapabilityStates[0].RequiredOperationSequence[0].DependsOnRequirementIDs[0] = "mutated"
	if gotAcceptance.CapabilityStates[0].InvalidatedByReceiptRefs[0] != "receipt-1" || gotAcceptance.CapabilityStates[0].RequiredOperationSequence[0].DependsOnRequirementIDs[0] != "operation-0" || gotAcceptance.Governance[0].TicketID != "ticket-1" || gotAcceptance.Operations[0].OperationID != "operation-1" {
		t.Fatalf("acceptance projection lost or shared audit fields: %+v", gotAcceptance)
	}

	gap := GapBaselineInfoFromProjection(&projectioncontract.GapBaseline{Diagnostics: []projectioncontract.GapDiagnostic{{Code: "contract", Kind: "schema", Severity: "error", Message: "original diagnostic", Source: "owner", Candidate: "candidate", CurrentAuthority: "authority"}}})
	if gap == nil || len(gap.Diagnostics) != 1 || gap.Diagnostics[0].CurrentAuthority != "authority" || gap.Diagnostics[0].Message != "original diagnostic" {
		t.Fatalf("gap diagnostics = %+v", gap)
	}

	convergence := ConvergenceStatusInfoFromProjection(&projectioncontract.ConvergenceStatus{DecisionEligibilityDenied: true, DecisionNotEligibleReason: "not actionable", DecisionNotEligibleStepID: "step-1"})
	if !convergence.DecisionEligibilityDenied || convergence.DecisionNotEligibleReason != "not actionable" || convergence.DecisionNotEligibleStepID != "step-1" {
		t.Fatalf("convergence decision eligibility = %+v", convergence)
	}

	budgets := []projectioncontract.LLMContextBudgetTrace{{CapabilityRevision: "cap-1", ConfigurationRevision: "config-1", ProviderSubmitted: true, ContextGateAction: "submit", ContextGateReason: "within budget", BudgetFailureClass: "none", RedactedPaths: []string{"secret"}, ProtectedFields: []string{"goal"}, IntegrityWarnings: []string{"warning"}, SectionHashes: []llmaudit.ContextSection{{ID: "c0", SourceRefs: []string{"contract"}}}}}
	gotBudgets := LLMContextBudgetTraceInfosFromProjection(budgets)
	budgets[0].RedactedPaths[0] = "mutated"
	budgets[0].SectionHashes[0].SourceRefs[0] = "mutated"
	if gotBudgets[0].CapabilityRevision != "cap-1" || !gotBudgets[0].ProviderSubmitted || gotBudgets[0].RedactedPaths[0] != "secret" || gotBudgets[0].SectionHashes[0].SourceRefs[0] != "contract" {
		t.Fatalf("LLM budget projection = %+v", gotBudgets[0])
	}

	usage := LLMUsageTraceInfosFromProjection([]projectioncontract.LLMUsageTrace{{APISurface: "responses", CacheMarkers: workorcheventpayload.New(workorcheventpayload.StringField("marker", "sent")), StartedAt: "start", CompletedAt: "end", DurationMs: 12}})
	if usage[0].APISurface != "responses" || usage[0].DurationMs != 12 || usage[0].StartedAt != "start" {
		t.Fatalf("LLM usage projection = %+v", usage[0])
	}
	breakdownInput := projectioncontract.LLMUsageSummary{CacheReadInputRatio: .5, CacheableInputRatio: .75, NonCacheableInputRatio: .25, ProviderModelBreakdown: []projectioncontract.LLMUsageProviderModelBreakdown{{APISurface: "responses", ProviderMode: "proxy", CacheCreationTokens: 7, CacheablePromptCount: 2, ActualCacheHitCount: 1, CacheCreationCount: 1, PromptCacheKeySentCount: 2, CacheControlSentCount: 2, AutoPrefixExpectedCount: 2, CacheReadInputRatio: .5}}}
	breakdown := LLMUsageSummaryInfoFromProjection(breakdownInput)
	breakdownInput.ProviderModelBreakdown[0].PromptCacheKeySentCount = 99
	if breakdown.CacheableInputRatio != .75 || breakdown.ProviderModelBreakdown[0].PromptCacheKeySentCount != 2 {
		t.Fatalf("LLM usage summary projection = %+v", breakdown)
	}

	assessment := &workorchresultcontract.DoDAssessment{Items: []workorchresultcontract.DoDAssessmentItem{{EvidenceRefs: []string{"evidence-1"}}}}
	results := []projectioncontract.Result{{DoDAssessment: assessment, CreatedAt: "created", UpdatedAt: "updated"}}
	gotResults := ResultInfosFromProjection(results)
	assessment.Items[0].EvidenceRefs[0] = "mutated"
	if gotResults[0].DoDAssessment.Items[0].EvidenceRefs[0] != "evidence-1" || gotResults[0].CreatedAt != "created" || gotResults[0].UpdatedAt != "updated" {
		t.Fatalf("result projection = %+v", gotResults[0])
	}
	artifacts := ArtifactInfosFromProjection([]projectioncontract.Artifact{{StepID: "step-1", ToolCallID: "tool-1", PatchID: "patch-1", ValidationResultID: "validation-1", Metadata: map[string]any{"original": "kept"}, CreatedAt: "created", UpdatedAt: "updated"}})
	if artifacts[0].StepID != "step-1" || artifacts[0].ValidationResultID != "validation-1" || artifacts[0].CreatedAt != "created" {
		t.Fatalf("artifact projection = %+v", artifacts[0])
	}

	target := &runtimecontract.ProviderModelTarget{ProviderID: "provider", ModelID: "model"}
	plans := []projectioncontract.TaskPlan{{SealedLLMTarget: target, RuntimeActivation: &projectioncontract.RuntimeActivationStatus{Disposition: "ready"}}}
	gotPlans := TaskPlanInfosFromProjection(plans)
	target.ModelID = "mutated"
	if gotPlans[0].SealedLLMTarget.ModelID != "model" || gotPlans[0].RuntimeActivation.Disposition != "ready" {
		t.Fatalf("task plan projection = %+v", gotPlans[0])
	}

	records := EventRecordInfosFromProjection([]projectioncontract.EventRecord{{Revision: 7, PayloadSummary: workorcheventpayload.New(workorcheventpayload.StringField("message", "original"))}})
	if len(records) != 1 || records[0].Revision != 7 {
		t.Fatalf("event record projection = %+v", records)
	}
}
