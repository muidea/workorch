package workorchruntimeaudit

import (
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestAuditValidationFeedbackPreservesIncompleteProofAndExactDiagnostics(t *testing.T) {
	baseline := projectioncontract.RuntimeAuditOperationReceipt{ValidationRecipeID: "recipe", ObservationProtocolRef: "owner/checks/v1",
		ExecutedChecks: []string{"check-A"}, PassedChecks: []string{"check-A"}}
	proof := runtimecontract.CoverageEvidenceProof{EvidenceKind: runtimecontract.EvidenceKindValidation,
		RequiredEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectNone}, AllowedOperationKinds: []string{runtimecontract.StepKindMCPToolCall},
		ValidationRecipeID: "recipe", RequiredCheckRefs: []string{"check-A"}}
	if !runtimecontract.RuntimeValidationObservationSatisfiesCoverageProof(proof, *runtimeOperationReceipt(baseline).Evidence.Validation) {
		t.Fatal("valid audit fixture lost exact check proof")
	}
	for name, mutate := range map[string]func(*projectioncontract.RuntimeAuditOperationReceipt){
		"skipped subcheck": func(r *projectioncontract.RuntimeAuditOperationReceipt) { r.IncompleteChecks = []string{"check-A"} },
		"partial output":   func(r *projectioncontract.RuntimeAuditOperationReceipt) { r.OutputTruncated = true },
	} {
		t.Run(name, func(t *testing.T) {
			changed := baseline
			mutate(&changed)
			if runtimecontract.RuntimeValidationObservationSatisfiesCoverageProof(proof, *runtimeOperationReceipt(changed).Evidence.Validation) {
				t.Fatal("audit conversion upgraded incomplete evidence to proof")
			}
		})
	}
	baseline.PassedChecks, baseline.FailedChecks = nil, []string{"check-A"}
	baseline.CheckFailures = []runtimecontract.RuntimeValidationCheckFailure{{CheckRef: "check-A", TargetRef: "checks/a.test", Line: 47, DiagnosticHash: strings.Repeat("a", 64)}}
	baseline.FailureDetailProtocolRef = runtimecontract.RuntimeValidationFailureDetailProtocol
	baseline.CheckFailures[0].ComparisonHash = strings.Repeat("c", 64)
	baseline.CheckFailures[0].ReportedAssertion = runtimecontract.RuntimeValidationReportedAssertion{Code: "shape_mismatch", ExpectedShape: "object", ObservedShape: "array"}
	evidence := projectioncontract.RuntimeAuditEvidence{ValidationRecipeID: baseline.ValidationRecipeID, ObservationProtocolRef: baseline.ObservationProtocolRef,
		FailureDetailProtocolRef: baseline.FailureDetailProtocolRef,
		ExecutedChecks:           baseline.ExecutedChecks, FailedChecks: baseline.FailedChecks, CheckFailures: baseline.CheckFailures}
	receipt := runtimeOperationReceipt(baseline)
	if err := receipt.Evidence.Validation.ValidateFailureDetails(); err != nil {
		t.Fatal(err)
	}
	if !sameValidationObservation(receipt.Evidence.Validation, evidence) {
		t.Fatal("identical receipt/evidence diagnostics differ")
	}
	for name, mutate := range map[string]func(*projectioncontract.RuntimeAuditEvidence){
		"detail protocol": func(e *projectioncontract.RuntimeAuditEvidence) { e.FailureDetailProtocolRef = "" },
		"comparison": func(e *projectioncontract.RuntimeAuditEvidence) {
			e.CheckFailures[0].ComparisonHash = strings.Repeat("d", 64)
		},
		"reported assertion": func(e *projectioncontract.RuntimeAuditEvidence) {
			e.CheckFailures[0].ReportedAssertion.ObservedShape = "null"
		},
		"location":         func(e *projectioncontract.RuntimeAuditEvidence) { e.CheckFailures[0].Line++ },
		"omitted details":  func(e *projectioncontract.RuntimeAuditEvidence) { e.CheckFailures = nil },
		"partial details":  func(e *projectioncontract.RuntimeAuditEvidence) { e.FailureDetailsTruncated = true },
		"partial output":   func(e *projectioncontract.RuntimeAuditEvidence) { e.OutputTruncated = true },
		"incomplete check": func(e *projectioncontract.RuntimeAuditEvidence) { e.IncompleteChecks = []string{"check-A"} },
	} {
		t.Run(name, func(t *testing.T) {
			changed := projectioncontract.CloneRuntimeAuditEvidence([]projectioncontract.RuntimeAuditEvidence{evidence})[0]
			mutate(&changed)
			if sameValidationObservation(receipt.Evidence.Validation, changed) {
				t.Fatal("audit ignored mismatched owner feedback")
			}
		})
	}
}

func TestCoverageBindingChainUsesCanonicalExactTargetSubsetPolicy(t *testing.T) {
	binding := runtimecontract.CoverageEvidenceBinding{
		ID: "binding/cwi-1", CoverageWorkItemID: "cwi-1", FocusID: "focus-1", DefinitionRevision: "definition-1",
		Proofs: []runtimecontract.CoverageEvidenceProof{{
			EvidenceKind: runtimecontract.EvidenceKindStateChange, RequiredEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable},
			AllowedOperationKinds: []string{runtimecontract.StepKindFileWrite},
		}},
		TargetRefs: []string{"main.go", "support.go"}, DeclarationRef: "dod/cwi-1",
	}.Normalized()
	binding.Hash = runtimecontract.StableCoverageEvidenceBindingHash(binding)
	audit := &projectioncontract.RuntimeAudit{
		Coverage: []projectioncontract.RuntimeAuditCoverage{{ID: "cwi-1", State: "satisfied", EvidenceRefs: []string{"evidence-1"}}},
		CoverageBindings: []projectioncontract.RuntimeAuditCoverageBinding{{
			ID: binding.ID, CoverageWorkItemID: string(binding.CoverageWorkItemID), FocusID: binding.FocusID, DefinitionRevision: binding.DefinitionRevision,
			Hash: binding.Hash, TargetRefs: append([]string(nil), binding.TargetRefs...), DeclarationRef: binding.DeclarationRef,
			Proofs: []projectioncontract.RuntimeAuditCoverageProof{{
				EvidenceKind: runtimecontract.EvidenceKindStateChange, RequiredEffectScopes: []string{string(runtimecontract.RuntimeEffectWorkspaceDurable)},
				AllowedOperationKinds: []string{runtimecontract.StepKindFileWrite},
			}},
		}},
		AcceptedEvidence: []projectioncontract.RuntimeAuditEvidence{{
			ID: "evidence-1", Kind: runtimecontract.EvidenceKindStateChange, DispatchID: "dispatch-1", OperationID: "write-main",
			BindingRefs: []string{binding.ID}, CoverageWorkItemIDs: []string{"cwi-1"}, TargetRefs: []string{"main.go"}, Quality: string(common.EvidenceQualityValid),
		}},
		Dispatches: []projectioncontract.RuntimeAuditDispatch{{
			ID: "dispatch-1", Operations: []projectioncontract.RuntimeAuditSealedOperation{{
				ID: "write-main", Kind: runtimecontract.StepKindFileWrite, ExecutionKey: "exec/ws/run/dispatch-1/write-main", Fence: 1,
				BindingRefs: []string{binding.ID}, TargetRefs: []string{"main.go"},
			}},
		}},
		OperationReceipts: []projectioncontract.RuntimeAuditOperationReceipt{{
			ExecutionKey: "exec/ws/run/dispatch-1/write-main", Fence: 1, OperationID: "write-main",
			Outcome: string(runtimecontract.RuntimeOperationOutcomeAccepted), ObservedEffectScope: string(runtimecontract.RuntimeEffectWorkspaceDurable),
			Changed: true, TargetRefs: []string{"main.go"}, Quality: string(common.EvidenceQualityValid),
		}},
	}
	if errors := CoverageBindingChainErrors(audit, true); len(errors) != 0 {
		t.Fatalf("canonical exact-target subset rejected: %v", errors)
	}

	audit.AcceptedEvidence[0].TargetRefs = []string{"outside.go"}
	audit.Dispatches[0].Operations[0].TargetRefs = []string{"outside.go"}
	audit.OperationReceipts[0].TargetRefs = []string{"outside.go"}
	errors := CoverageBindingChainErrors(audit, true)
	if len(errors) == 0 || !strings.Contains(strings.Join(errors, "\n"), "target_outside_sealed_set") {
		t.Fatalf("outside target did not use canonical authorization finding: %v", errors)
	}
}

func TestCoverageBindingChainRequiresEverySealedProofKind(t *testing.T) {
	binding := runtimecontract.CoverageEvidenceBinding{
		ID: "binding/cwi-conjunction", CoverageWorkItemID: "cwi-conjunction", FocusID: "focus-conjunction", DefinitionRevision: "definition-conjunction",
		Proofs: []runtimecontract.CoverageEvidenceProof{
			{EvidenceKind: runtimecontract.EvidenceKindStateChange, RequiredEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable}, AllowedOperationKinds: []string{runtimecontract.StepKindFileWrite}},
			{EvidenceKind: runtimecontract.EvidenceKindValidation, RequiredEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectNone}, AllowedOperationKinds: []string{runtimecontract.StepKindMCPToolCall}, ValidationRecipeID: "mcp/server/tool", RequiredCheckRefs: []string{"check-conjunction"}},
		},
		TargetRefs: []string{"main.go"}, DeclarationRef: "dod/cwi-conjunction",
	}.Normalized()
	binding.Hash = runtimecontract.StableCoverageEvidenceBindingHash(binding)
	audit := &projectioncontract.RuntimeAudit{
		Coverage: []projectioncontract.RuntimeAuditCoverage{{ID: "cwi-conjunction", State: "satisfied", EvidenceRefs: []string{"evidence-state"}}},
		CoverageBindings: []projectioncontract.RuntimeAuditCoverageBinding{{
			ID: binding.ID, CoverageWorkItemID: string(binding.CoverageWorkItemID), FocusID: binding.FocusID, DefinitionRevision: binding.DefinitionRevision,
			Hash: binding.Hash, TargetRefs: append([]string(nil), binding.TargetRefs...), DeclarationRef: binding.DeclarationRef,
			Proofs: []projectioncontract.RuntimeAuditCoverageProof{
				{EvidenceKind: runtimecontract.EvidenceKindStateChange, RequiredEffectScopes: []string{string(runtimecontract.RuntimeEffectWorkspaceDurable)}, AllowedOperationKinds: []string{runtimecontract.StepKindFileWrite}},
				{EvidenceKind: runtimecontract.EvidenceKindValidation, RequiredEffectScopes: []string{string(runtimecontract.RuntimeEffectNone)}, AllowedOperationKinds: []string{runtimecontract.StepKindMCPToolCall}, ValidationRecipeID: "mcp/server/tool", RequiredCheckRefs: []string{"check-conjunction"}},
			},
		}},
		AcceptedEvidence: []projectioncontract.RuntimeAuditEvidence{{
			ID: "evidence-state", Kind: runtimecontract.EvidenceKindStateChange, DispatchID: "dispatch-state", OperationID: "write-main",
			BindingRefs: []string{binding.ID}, CoverageWorkItemIDs: []string{"cwi-conjunction"}, TargetRefs: []string{"main.go"}, Quality: string(common.EvidenceQualityValid),
		}},
		Dispatches: []projectioncontract.RuntimeAuditDispatch{{
			ID: "dispatch-state", Operations: []projectioncontract.RuntimeAuditSealedOperation{{
				ID: "write-main", Kind: runtimecontract.StepKindFileWrite, ExecutionKey: "exec/ws/run/dispatch-state/write-main", Fence: 1,
				BindingRefs: []string{binding.ID}, TargetRefs: []string{"main.go"},
			}},
		}},
		OperationReceipts: []projectioncontract.RuntimeAuditOperationReceipt{{
			ExecutionKey: "exec/ws/run/dispatch-state/write-main", Fence: 1, OperationID: "write-main",
			Outcome: string(runtimecontract.RuntimeOperationOutcomeAccepted), ObservedEffectScope: string(runtimecontract.RuntimeEffectWorkspaceDurable),
			Changed: true, TargetRefs: []string{"main.go"}, Quality: string(common.EvidenceQualityValid),
		}},
	}
	errors := CoverageBindingChainErrors(audit, true)
	if len(errors) == 0 || !strings.Contains(strings.Join(errors, "\n"), "lacks exact validation proof chain") {
		t.Fatalf("satisfied conjunction omitted validation proof without audit failure: %v", errors)
	}
}
