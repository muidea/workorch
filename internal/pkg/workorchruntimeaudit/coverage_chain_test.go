package workorchruntimeaudit

import (
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

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
