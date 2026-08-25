package runtime

import (
	"context"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestSemanticFailureClassForFailureCodePreservesBoundedTerminalCategories(t *testing.T) {
	tests := []struct {
		name string
		code string
		want runtimecontract.FailureClass
	}{
		{name: "owner infeasible", code: "semantic_owner_feasibility_infeasible", want: runtimecontract.FailureClassCandidateInfeasible},
		{name: "prospective owner fact unsatisfied", code: "prospective_owner_fact_unsatisfied", want: runtimecontract.FailureClassCandidateInfeasible},
		{name: "catalog refresh exhausted", code: "semantic_catalog_refresh_exhausted", want: runtimecontract.FailureClassPrerequisiteRefreshExhausted},
		{name: "candidate correction exhausted", code: "semantic_plan_retry_exhausted", want: runtimecontract.FailureClassCandidateExhausted},
		{name: "document recovery exhausted", code: "semantic_document_recovery_exhausted", want: runtimecontract.FailureClassCandidateDocumentRecoveryExhausted},
		{name: "provider body timeout", code: "semantic_provider_body_idle_timeout", want: runtimecontract.FailureClassProviderTransient},
		{name: "provider protocol", code: "semantic_provider_outcome_invalid", want: runtimecontract.FailureClassProviderProtocol},
		{name: "MCP validation input prerequisite", code: "mcp_validation_input_binding_required", want: runtimecontract.FailureClassCapabilityUnavailable},
		{name: "runtime contract", code: "runtime_contract_defect", want: runtimecontract.FailureClassRuntimeContractMisalignment},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := semanticFailureClassForFailureCode(tt.code, runtimecontract.RuntimeOriginalDiagnostic{
				OriginalCode:  "mcp_catalog_unavailable",
				OriginalClass: "semantic_feasibility",
			})
			if got != tt.want {
				t.Fatalf("semantic failure class for %q = %q, want %q", tt.code, got, tt.want)
			}
			if got == runtimecontract.FailureClassNoProgressExhausted {
				t.Fatal("semantic terminal must not infer no-progress exhaustion")
			}
		})
	}
}

func TestSemanticFailureClassForDiagnosticKeepsMCPInputBindingAsCapabilityPrerequisite(t *testing.T) {
	got := semanticFailureClassForDiagnostic(runtimecontract.RuntimeOriginalDiagnostic{
		OriginalClass: "semantic_feasibility", OriginalCode: "mcp_validation_input_binding_required",
	})
	if got != runtimecontract.FailureClassCapabilityUnavailable {
		t.Fatalf("MCP validation input prerequisite class = %q, want %q", got, runtimecontract.FailureClassCapabilityUnavailable)
	}
}

func TestSemanticFailureClassForDiagnosticPreservesProviderCategories(t *testing.T) {
	tests := []struct {
		name  string
		class string
		want  runtimecontract.FailureClass
	}{
		{name: "transient", class: string(runtimecontract.RuntimeCandidateFailureClassProviderTransient), want: runtimecontract.FailureClassProviderTransient},
		{name: "authentication", class: string(runtimecontract.RuntimeCandidateFailureClassProviderAuthentication), want: runtimecontract.FailureClassProviderAuthentication},
		{name: "configuration", class: string(runtimecontract.RuntimeCandidateFailureClassProviderConfiguration), want: runtimecontract.FailureClassProviderConfiguration},
		{name: "protocol", class: string(runtimecontract.RuntimeCandidateFailureClassProviderProtocol), want: runtimecontract.FailureClassProviderProtocol},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := semanticFailureClassForDiagnostic(runtimecontract.RuntimeOriginalDiagnostic{
				OriginalClass: tt.class, OriginalCode: "provider-specific-code",
			})
			if got != tt.want {
				t.Fatalf("semantic diagnostic class %q = %q, want %q", tt.class, got, tt.want)
			}
		})
	}
}

func TestFailSemanticContractRetainsNestedOriginalDiagnosticAndClass(t *testing.T) {
	controller, _, aggregate := newSemanticPlanningController(t)
	diagnostic := runtimecontract.RuntimeOriginalDiagnostic{
		SchemaVersion:   runtimecontract.RuntimeOriginalDiagnosticSchemaCurrent,
		OwnerModuleID:   runtimecontract.RuntimeModuleFile,
		OriginalClass:   "semantic_feasibility",
		OriginalCode:    "file_semantic_proof_unavailable",
		OriginalMessage: "owner cannot independently prove the requested target",
		Signature:       "file/semantic-proof-unavailable",
		TargetRefs:      []string{"target.txt"},
	}
	failed, err := controller.failSemanticContract(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, "semantic_owner_feasibility_infeasible", diagnostic)
	if err != nil {
		t.Fatal(err)
	}
	if failed.Facts.Failure == nil {
		t.Fatal("semantic terminal omitted failure fact")
	}
	if failed.Facts.Failure.Class != runtimecontract.FailureClassCandidateInfeasible {
		t.Fatalf("semantic failure class = %q, want %q", failed.Facts.Failure.Class, runtimecontract.FailureClassCandidateInfeasible)
	}
	if failed.Facts.Failure.OriginalDiagnostic == nil || failed.Facts.Failure.OriginalDiagnostic.OriginalCode != diagnostic.OriginalCode ||
		failed.Facts.Failure.OriginalDiagnostic.OriginalMessage != diagnostic.OriginalMessage ||
		len(failed.Facts.Failure.OriginalDiagnostic.TargetRefs) != 1 || failed.Facts.Failure.OriginalDiagnostic.TargetRefs[0] != "target.txt" {
		t.Fatalf("nested semantic diagnostic was not preserved: %+v", failed.Facts.Failure.OriginalDiagnostic)
	}
}

func TestFailSemanticContractUsesTerminalCodeWithoutRewritingDiagnostic(t *testing.T) {
	controller, _, aggregate := newSemanticPlanningController(t)
	diagnostic := runtimecontract.RuntimeOriginalDiagnostic{
		SchemaVersion:   runtimecontract.RuntimeOriginalDiagnosticSchemaCurrent,
		OwnerModuleID:   runtimecontract.RuntimeModuleMCP,
		OriginalClass:   "semantic_feasibility",
		OriginalCode:    "mcp_catalog_unavailable",
		OriginalMessage: "MCP catalog refresh remained unavailable",
		Signature:       "mcp/catalog-unavailable",
	}
	failed, err := controller.failSemanticContract(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, "semantic_catalog_refresh_exhausted", diagnostic)
	if err != nil {
		t.Fatal(err)
	}
	if failed.Facts.Failure == nil || failed.Facts.Failure.Class != runtimecontract.FailureClassPrerequisiteRefreshExhausted {
		t.Fatalf("refresh exhaustion class = %+v", failed.Facts.Failure)
	}
	if failed.Facts.Failure.OriginalDiagnostic == nil || failed.Facts.Failure.OriginalDiagnostic.OriginalCode != diagnostic.OriginalCode ||
		failed.Facts.Failure.OriginalDiagnostic.OriginalMessage != diagnostic.OriginalMessage {
		t.Fatalf("refresh exhaustion rewrote owner diagnostic: %+v", failed.Facts.Failure.OriginalDiagnostic)
	}
}
