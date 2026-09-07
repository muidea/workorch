package readmodelprojection

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestOwnerInfeasibilityCauseSurvivesAllPublicProjectionsWithoutAuthority(t *testing.T) {
	diagnostic := &runtimecontract.RuntimeOriginalDiagnostic{
		SchemaVersion: runtimecontract.RuntimeOriginalDiagnosticSchemaCurrent, OwnerModuleID: runtimecontract.RuntimeModuleFile,
		OriginalClass: "candidate_contract_failure", OriginalCode: "patch_basehash_mismatch", OriginalMessage: "conflict: patch baseHash mismatch",
		Signature: "file-preflight/mismatch", OperationID: "operation-1", TargetRefs: []string{"session.txt"},
	}
	diagnostic = diagnostic.CloneOriginal()
	state := runtimecontract.RuntimeStateRecord{Facts: runtimecontract.RuntimeFacts{
		WorkspaceID: "ws-1", RunID: "run-1", Failure: &runtimecontract.FailureFact{
			Class: runtimecontract.FailureClassNoProgressExhausted, Code: "equivalent_authority_no_progress", Signature: "unchanged-authority", Message: "unchanged inputs", OriginalDiagnostic: diagnostic,
		},
	}, CandidateAttempts: []runtimecontract.RuntimeCandidateAttemptRecord{{
		AuthorityContractID: "contract-1", State: runtimecontract.RuntimeCandidateAttemptStateInvalidated,
		OwnerFeasibility: &runtimecontract.RuntimeOwnerFeasibilityObservation{
			Outcome:     runtimecontract.RuntimeOwnerFeasibilityInfeasible,
			OwnerPolicy: runtimecontract.RuntimeOwnerPolicyIdentity{OwnerModuleID: runtimecontract.RuntimeModuleFile, CatalogSnapshotID: "catalog-1"},
			Feedback:    &runtimecontract.RuntimeFeedback{Domain: runtimecontract.RuntimeFeedbackDomainFeasibility, StableCategory: runtimecontract.RuntimeFeedbackCategory(runtimecontract.RuntimeOwnerFeasibilityCategoryObservationUnknown), OriginalDiagnostic: diagnostic},
		},
	}}}
	audit := mapRuntimeAudit(&state)
	progress := mapRuntimeProgress(state)
	result := mapResult(&resultmodel.Result{RuntimeCloseout: &runtimecontract.RuntimeResultCloseout{OriginalDiagnostic: diagnostic}})
	resultView := projectResultView(result)
	for _, got := range []*runtimecontract.RuntimeOriginalDiagnostic{audit.Failure.OriginalDiagnostic, audit.CandidateAttempts[0].OriginalDiagnostic, progress.OriginalDiagnostic, result.OriginalDiagnostic, resultView.OriginalDiagnostic} {
		if !reflect.DeepEqual(got, diagnostic) {
			t.Fatalf("public projection lost original cause: %+v", got)
		}
		if got == diagnostic || &got.TargetRefs[0] == &diagnostic.TargetRefs[0] {
			t.Fatal("public projection aliases authority")
		}
	}
	attempt := audit.CandidateAttempts[0]
	if attempt.OwnerFeasibilityOutcome != "infeasible" || attempt.OwnerCatalogSnapshotID != "catalog-1" || attempt.ProposalPersisted || attempt.FeasibilityCount != 0 || attempt.FailureCode != "" {
		t.Fatalf("invalidated attempt invented proposal or correction failure: %+v", attempt)
	}
	if progress.FailureCode != "equivalent_authority_no_progress" || progress.DurableEffectCount != 0 || progress.AcceptedValidationEvidenceCount != 0 || progress.SatisfiedCWICount != 0 {
		t.Fatalf("diagnostic altered progress: %+v", progress)
	}
	clonedAudit := projectioncontract.CloneRuntimeAudit(audit)
	clonedAudit.CandidateAttempts[0].OriginalDiagnostic.TargetRefs[0] = "mutated"
	clonedProgress := projectioncontract.CloneRuntimeProgress(&progress)
	clonedProgress.OriginalDiagnostic.TargetRefs[0] = "mutated"
	clonedResult := projectioncontract.CloneResult(result)
	clonedResult.OriginalDiagnostic.TargetRefs[0] = "mutated"
	if diagnostic.TargetRefs[0] != "session.txt" || audit.CandidateAttempts[0].OriginalDiagnostic.TargetRefs[0] != "session.txt" || progress.OriginalDiagnostic.TargetRefs[0] != "session.txt" || result.OriginalDiagnostic.TargetRefs[0] != "session.txt" {
		t.Fatal("clone mutation leaked across projection boundary")
	}
	encoded, _ := json.Marshal(audit)
	if strings.Contains(string(encoded), "nextAction") || strings.Contains(string(encoded), "parameters") {
		t.Fatal("public diagnostics exposed execution authority")
	}
}
