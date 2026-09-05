package readmodelprojection

import (
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestMapRuntimeAuditSeparatesActiveAndRetiredPrerequisiteLifecycle(t *testing.T) {
	state := runtimecontract.RuntimeStateRecord{Facts: runtimecontract.RuntimeFacts{
		WorkspaceID: "ws-audit", RunID: "run-audit", DefinitionRevision: "definition-audit", EvidenceRevision: "evidence-audit",
		Focus: runtimecontract.CoverageFocus{ID: "focus-current"},
		Prerequisites: runtimecontract.PrerequisiteState{
			Pending: []runtimecontract.RuntimePrerequisite{{
				ID: "active-selection", Kind: runtimecontract.PrerequisiteKindTargetSelection,
				Scope: runtimecontract.RuntimePrerequisiteScopeFocus, FocusID: "focus-current", OwnerModuleID: runtimecontract.RuntimeModuleFile,
			}},
			Retired: []runtimecontract.RuntimePrerequisite{{
				ID: "retired-selection", Kind: runtimecontract.PrerequisiteKindTargetSelection,
				Scope: runtimecontract.RuntimePrerequisiteScopeFocus, FocusID: "focus-prior", OwnerModuleID: runtimecontract.RuntimeModuleFile,
			}},
		},
	}}

	audit := mapRuntimeAudit(&state)
	if audit == nil || len(audit.Grounding.ActivePrerequisites) != 1 || len(audit.Grounding.RetiredPrerequisites) != 1 {
		t.Fatalf("prerequisite lifecycle audit = %+v", audit)
	}
	if active := audit.Grounding.ActivePrerequisites[0]; active.ID != "active-selection" || active.Scope != string(runtimecontract.RuntimePrerequisiteScopeFocus) || active.FocusID != "focus-current" {
		t.Fatalf("active prerequisite audit = %+v", active)
	}
	if retired := audit.Grounding.RetiredPrerequisites[0]; retired.ID != "retired-selection" || retired.FocusID != "focus-prior" {
		t.Fatalf("retired prerequisite audit = %+v", retired)
	}
}

func TestMapRuntimeAuditPreservesFailureOriginalDiagnosticWithoutAction(t *testing.T) {
	state := runtimecontract.RuntimeStateRecord{Facts: runtimecontract.RuntimeFacts{
		WorkspaceID: "ws-audit", RunID: "run-audit", DefinitionRevision: "definition-audit", EvidenceRevision: "evidence-audit",
		Focus: runtimecontract.CoverageFocus{ID: "focus-current"},
		Failure: &runtimecontract.FailureFact{
			Class: runtimecontract.FailureClassRuntimeContractMisalignment, Code: "definition_mismatch",
			Signature: "runtime-state-update/ws-audit/run-audit/4/4/semantic_rejection", Message: "definition revision is stale",
			OriginalDiagnostic: &runtimecontract.RuntimeOriginalDiagnostic{
				SchemaVersion: runtimecontract.RuntimeOriginalDiagnosticSchemaCurrent, OwnerModuleID: "blocks/run",
				OriginalClass: "semantic_rejection", OriginalCode: "definition_mismatch", OriginalMessage: "definition revision is stale",
				Signature: "runtime-state-update/ws-audit/run-audit/4/4/semantic_rejection", ExpectedRevision: "4", ObservedRevision: "4",
			},
		},
	}}

	audit := mapRuntimeAudit(&state)
	if audit == nil || audit.Failure == nil || audit.Failure.Code != "definition_mismatch" ||
		audit.Failure.OriginalDiagnostic == nil || audit.Failure.OriginalDiagnostic.OwnerModuleID != "blocks/run" ||
		audit.Failure.OriginalDiagnostic.ObservedRevision != "4" {
		t.Fatalf("failure audit lost original diagnostic: %+v", audit)
	}
}

func TestMapRuntimeAuditPreservesValidationSurfaceReadinessWithoutExecutableInput(t *testing.T) {
	recipeID := runtimecontract.RuntimeMCPToolValidationRecipeID("mcp-lsp-bridge", "lsp_definition")
	catalog := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: "mcp-catalog", OwnerModuleID: runtimecontract.RuntimeModuleMCP,
		OperationKinds: []string{runtimecontract.StepKindMCPToolCall}, SelectorRefs: []string{"mcp-lsp-bridge"},
		ValidationSurfaces: []runtimecontract.RuntimeValidationSurface{{
			ID: recipeID, OwnerModuleID: runtimecontract.RuntimeModuleMCP, OperationKind: runtimecontract.StepKindMCPToolCall,
			Readiness: runtimecontract.RuntimeValidationSurfacePrerequisiteRequired, ReadinessReason: "input_binding_required",
			InputSchemaHash: "definition-schema", PrerequisiteRef: "mcp-validation-input/definition",
			CheckRefs: []string{recipeID}, ProofApplicability: runtimecontract.RuntimeValidationProofDirect, EffectScope: runtimecontract.RuntimeEffectNone, IdempotencyClass: runtimecontract.RuntimeOperationIdempotencySafeToReplay,
		}},
		ObservedRevision: "mcp-revision-v1",
	}
	catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	state := runtimecontract.RuntimeStateRecord{Facts: runtimecontract.RuntimeFacts{
		WorkspaceID: "ws-audit", RunID: "run-audit", DefinitionRevision: "definition-audit", EvidenceRevision: "evidence-audit",
		Focus:     runtimecontract.CoverageFocus{ID: "focus-current"},
		Resources: runtimecontract.RuntimeResourceFacts{OperationAdmissibilities: []runtimecontract.RuntimeOperationAdmissibility{catalog}},
	}}

	audit := mapRuntimeAudit(&state)
	if audit == nil || len(audit.Grounding.OperationAdmissibilities) != 1 || len(audit.Grounding.OperationAdmissibilities[0].ValidationSurfaces) != 1 {
		t.Fatalf("validation readiness audit missing: %+v", audit)
	}
	surface := audit.Grounding.OperationAdmissibilities[0].ValidationSurfaces[0]
	if surface.ID != recipeID || surface.Readiness != string(runtimecontract.RuntimeValidationSurfacePrerequisiteRequired) ||
		surface.ReadinessReason != "input_binding_required" || surface.PrerequisiteRef == "" || surface.InputBindingRef != "" ||
		surface.ProofApplicability != string(runtimecontract.RuntimeValidationProofDirect) ||
		surface.EffectScope != string(runtimecontract.RuntimeEffectNone) || surface.IdempotencyClass != string(runtimecontract.RuntimeOperationIdempotencySafeToReplay) {
		t.Fatalf("validation readiness audit = %+v", surface)
	}
}

func TestMapRuntimeAuditProjectsBindingReceiptAndDerivedCreditSeparately(t *testing.T) {
	binding := runtimecontract.CoverageEvidenceBinding{
		ID: "binding/cwi-audit", CoverageWorkItemID: "cwi-audit", FocusID: "focus-current", DefinitionRevision: "definition-audit",
		Proofs: []runtimecontract.CoverageEvidenceProof{{
			EvidenceKind: runtimecontract.EvidenceKindValidation, RequiredEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectNone},
			AllowedOperationKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession}, ValidationRecipeID: "recipe/audit",
			ValidationProofRequirementHash: "runtime-validation-proof-requirement-audit", RequiredCheckRefs: []string{"check/audit"},
			ValidationCheckAttestations: []runtimecontract.RuntimeValidationCheckAttestationBinding{{
				RequiredCheckRef: "check/audit", ObservationProtocolRef: "shell/audit/v1", NativeCheckRef: "native/check/audit",
			}},
			MetricConstraints: []string{"metric/audit"}, ArtifactRefs: []string{"artifact/audit"},
		}}, TargetRefs: []string{"main.go"}, DeclarationRef: "profile/audit",
	}.Normalized()
	binding.Hash = runtimecontract.StableCoverageEvidenceBindingHash(binding)
	completedBinding := binding
	completedBinding.ID = "binding/cwi-prior"
	completedBinding.CoverageWorkItemID = "cwi-prior"
	completedBinding.FocusID = "focus-prior"
	completedBinding.TargetRefs = []string{"prior.go"}
	completedBinding.Hash = runtimecontract.StableCoverageEvidenceBindingHash(completedBinding)
	observation := &runtimecontract.RuntimeValidationObservation{
		RecipeID: "recipe/audit", ObservationProtocolRef: "shell/audit/v1",
		ExecutedChecks: []string{"native/check/audit"}, PassedChecks: []string{"native/check/audit"},
	}
	state := runtimecontract.RuntimeStateRecord{Facts: runtimecontract.RuntimeFacts{
		WorkspaceID: "ws-audit", RunID: "run-audit", DefinitionRevision: "definition-audit", EvidenceRevision: "evidence-audit",
		Focus: runtimecontract.CoverageFocus{ID: "focus-current", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-audit"}, CoverageEvidenceBindings: []runtimecontract.CoverageEvidenceBinding{binding}},
		CompletedFocuses: []runtimecontract.CoverageFocus{{
			ID: "focus-prior", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-prior"}, CoverageEvidenceBindings: []runtimecontract.CoverageEvidenceBinding{completedBinding},
		}},
		Evidence: runtimecontract.EvidenceLedger{Accepted: []runtimecontract.EvidenceFact{{
			ID: "evidence-audit", Kind: runtimecontract.EvidenceKindValidation, DispatchID: "dispatch-audit", OperationID: "validate-audit",
			BindingRefs: []string{"binding/cwi-audit"}, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-audit"}, TargetRefs: []string{"main.go"},
			EvidenceRefs: []string{"artifact/audit"}, EvidenceTags: []string{"metric/audit"}, Validation: observation,
		}}},
	}, CompletedDispatches: []runtimecontract.RuntimeDispatchRecord{{
		ID: "dispatch-audit", State: runtimecontract.RuntimeDispatchStateCommitted,
		Candidate: runtimecontract.RuntimeCandidate{Validation: &runtimecontract.ValidationCandidate{Operations: []runtimecontract.ValidationOperation{{
			Operation: runtimecontract.RuntimeOperation{ID: "validate-audit", Kind: runtimecontract.RuntimeBlockKindManagedValidationSession, BindingRefs: []string{binding.ID}, TargetRefs: []string{"main.go"}},
		}}}},
		OperationExecutions: []runtimecontract.RuntimeOperationExecutionRecord{{
			OperationID: "validate-audit", ExecutionKey: "exec/ws-audit/run-audit/original-dispatch/validate-audit", Fence: 2,
		}},
	}}, OperationReceipts: []runtimecontract.RuntimeOperationReceipt{{
		ExecutionKey: "exec/ws-audit/run-audit/original-dispatch/validate-audit", Fence: 2, OperationID: "validate-audit",
		Outcome: runtimecontract.RuntimeOperationOutcomeAccepted, ObservedEffectScope: runtimecontract.RuntimeEffectNone, TargetRefs: []string{"main.go"},
		Evidence: runtimecontract.RuntimeOperationEvidence{EvidenceRefs: []string{"artifact/audit"}, EvidenceTags: []string{"metric/audit"}, Validation: observation},
	}}}

	audit := mapRuntimeAudit(&state)
	if audit == nil || len(audit.Focus.CoverageBindings) != 1 || len(audit.CoverageBindings) != 2 || len(audit.Dispatches) != 1 || len(audit.AcceptedEvidence) != 1 || len(audit.OperationReceipts) != 1 {
		t.Fatalf("binding audit missing chain: %+v", audit)
	}
	projectedBinding := audit.CoverageBindings[0]
	if projectedBinding.ID != binding.ID || projectedBinding.Hash != binding.Hash || len(projectedBinding.Proofs) != 1 {
		t.Fatalf("binding declaration projection = %+v", projectedBinding)
	}
	projectedProof := projectedBinding.Proofs[0]
	if projectedProof.ValidationRecipeID != "recipe/audit" || projectedProof.ValidationProofRequirementHash != "runtime-validation-proof-requirement-audit" ||
		len(projectedProof.RequiredCheckRefs) != 1 || len(projectedProof.ValidationCheckAttestations) != 1 ||
		projectedProof.ValidationCheckAttestations[0].RequiredCheckRef != "check/audit" ||
		projectedProof.ValidationCheckAttestations[0].ObservationProtocolRef != "shell/audit/v1" ||
		projectedProof.ValidationCheckAttestations[0].NativeCheckRef != "native/check/audit" {
		t.Fatalf("binding declaration projection = %+v", projectedBinding)
	}
	evidence := audit.AcceptedEvidence[0]
	if len(evidence.BindingRefs) != 1 || evidence.BindingRefs[0] != binding.ID || len(evidence.CoverageWorkItemIDs) != 1 || evidence.CoverageWorkItemIDs[0] != "cwi-audit" || evidence.ValidationRecipeID != "recipe/audit" || evidence.ObservationProtocolRef != "shell/audit/v1" || len(evidence.PassedChecks) != 1 {
		t.Fatalf("derived credit projection = %+v", evidence)
	}
	receipt := audit.OperationReceipts[0]
	if receipt.ObservedEffectScope != string(runtimecontract.RuntimeEffectNone) || receipt.ValidationRecipeID != "recipe/audit" || receipt.ObservationProtocolRef != "shell/audit/v1" || len(receipt.PassedChecks) != 1 || len(receipt.EvidenceTags) != 1 {
		t.Fatalf("owner receipt projection = %+v", receipt)
	}
	dispatch := audit.Dispatches[0]
	if dispatch.ID != "dispatch-audit" || len(dispatch.Operations) != 1 || dispatch.Operations[0].ID != "validate-audit" ||
		dispatch.Operations[0].ExecutionKey != "exec/ws-audit/run-audit/original-dispatch/validate-audit" || dispatch.Operations[0].Fence != 2 ||
		len(dispatch.Operations[0].BindingRefs) != 1 || dispatch.Operations[0].BindingRefs[0] != binding.ID || dispatch.Operations[0].TargetRefs[0] != "main.go" {
		t.Fatalf("sealed operation audit projection = %+v", dispatch)
	}
}
