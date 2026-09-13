package llm

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeSemanticExecutionHistoryStaysVolatileAndIntegrityProtected(t *testing.T) {
	request := runtimeSemanticProviderTestRequest(t)
	before, err := runtimeProviderRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	c := &request.SemanticContext
	c.ExecutionHistory, err = runtimecontract.BuildRuntimeSemanticExecutionHistory(*c, request.SemanticManifest, 1, []runtimecontract.RuntimeSemanticReceiptObservation{{
		ReceiptSequence: 1, DispatchID: "dispatch", OperationID: "write", OperationKind: runtimecontract.StepKindFileWrite, OwnerModuleID: runtimecontract.RuntimeModuleFile,
		ReceiptRef: "runtime-receipt/execution", ReceiptHash: "receipt-hash", Fence: 1,
		BoundCoverageWorkItemIDs: []common.CoverageWorkItemID{c.DefinitionSeed.CoverageItems[0].ID}, ObservedState: runtimecontract.RuntimeOperationOutcomeAccepted,
		ObservedEffectScope: runtimecontract.RuntimeEffectWorkspaceDurable, Changed: true, TargetRefs: []string{"source.txt"}, AfterHash: "after",
		ExpectedEffectScope: runtimecontract.RuntimeEffectWorkspaceDurable, RequestedTargetRefs: []string{"source.txt"},
		Diagnostic: &runtimecontract.RuntimeSemanticExecutionDiagnostic{FeedbackHash: "feedback-hash", OwnerModuleID: runtimecontract.RuntimeModuleFile,
			OriginalClass: "contract", OriginalCode: "owner_fact", Signature: "signature", Truncated: true, SummaryOnly: true,
			ValidationDiagnostic: &runtimecontract.RuntimeValidationDiagnosticFeedback{CommandKind: "managed_validation", ReportedExitCode: 2}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	after, err := runtimeProviderRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	for i := range 4 {
		if !reflect.DeepEqual(before.PromptContext.Sections[i], after.PromptContext.Sections[i]) {
			t.Fatalf("history changed C%d", i)
		}
	}
	if before.ContextBudget.CachePlan.StablePrefixHash != after.ContextBudget.CachePlan.StablePrefixHash {
		t.Fatal("history changed stable prefix")
	}
	var tail runtimeSemanticTailProjection
	if err := json.Unmarshal([]byte(after.PromptContext.Sections[4].Content), &tail); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(tail.ExecutionHistory, c.ExecutionHistory) || tail.CoverageProgress.Satisfied != 0 {
		t.Fatal("receipt lost or promoted to CWI credit")
	}
	for _, mutation := range []string{"missing", "drop", "outcome", "hash", "scope", "expected", "requested", "diagnostic", "exit", "truncated"} {
		t.Run(mutation, func(t *testing.T) {
			provider, err := runtimeProviderRequest(request)
			if err != nil {
				t.Fatal(err)
			}
			value := tail
			value.ExecutionHistory = tail.ExecutionHistory.Clone()
			switch mutation {
			case "missing":
				value.ExecutionHistory = runtimecontract.RuntimeSemanticExecutionHistory{}
			case "drop":
				value.ExecutionHistory.Receipts = nil
			case "outcome":
				value.ExecutionHistory.Receipts[0].ObservedState = runtimecontract.RuntimeOperationOutcomeRejected
			case "hash":
				value.ExecutionHistory.Receipts[0].ReceiptHash = "other"
			case "scope":
				value.ExecutionHistory.RunID = "foreign"
			case "expected":
				value.ExecutionHistory.Receipts[0].ExpectedEffectScope = runtimecontract.RuntimeEffectNone
			case "requested":
				value.ExecutionHistory.Receipts[0].RequestedTargetRefs[0] = "unrequested.txt"
			case "diagnostic":
				value.ExecutionHistory.Receipts[0].Diagnostic = nil
			case "exit":
				value.ExecutionHistory.Receipts[0].Diagnostic.ValidationDiagnostic.ReportedExitCode = 0
			case "truncated":
				value.ExecutionHistory.Receipts[0].Diagnostic.Truncated = false
			}
			value.ExecutionHistory.Hash = runtimecontract.StableRuntimeSemanticExecutionHistoryHash(value.ExecutionHistory)
			encoded, _ := json.Marshal(value)
			provider.PromptContext.Sections[4].Content = string(encoded)
			err = validateRuntimeSemanticContextIntegrity(*c, request.SemanticManifest, request.CandidateAttempt, request.SemanticCorrectionObservation, request.SemanticCorrectionDirective, provider.ContextBudget, provider.PromptContext)
			if err == nil || !strings.Contains(err.Error(), "P3.semantic_execution_history") {
				t.Fatalf("post-render mutation accepted: %v", err)
			}
		})
	}
	c.ExecutionHistory = runtimecontract.RuntimeSemanticExecutionHistory{}
	if _, err := runtimeProviderRequest(request); err == nil {
		t.Fatal("missing owner observation accepted")
	}
}
