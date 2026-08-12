package readmodelprojection

import (
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeSemanticPlanningProjectionPreservesRunScopeAndPhase(t *testing.T) {
	state := semanticPlanningProjectionState("ws-semantic-a", "run-semantic-a", 17)
	state.SemanticPlanning.Phase = runtimecontract.RuntimeSemanticPlanningRequired

	got := runtimeSemanticPlanningProjection(state)
	if got == nil || got.WorkspaceID != "ws-semantic-a" || got.RunID != "run-semantic-a" ||
		got.DefinitionRevision != "definition-semantic" || got.StateRevision != 17 ||
		got.State != "semantic_contract_required" || got.Phase != string(runtimecontract.RuntimeSemanticPlanningRequired) {
		t.Fatalf("required semantic projection = %+v", got)
	}
	if len(got.Items) != 2 || got.Items[0].CoverageWorkItemID != "cwi-a" || got.Items[0].Status != "pending" ||
		got.Items[1].CoverageWorkItemID != "cwi-b" || got.Items[1].Status != "pending" {
		t.Fatalf("required semantic items = %+v", got.Items)
	}

	other := semanticPlanningProjectionState("ws-semantic-b", "run-semantic-b", 4)
	other.SemanticPlanning.Phase = runtimecontract.RuntimeSemanticPlanningRequired
	otherProjection := runtimeSemanticPlanningProjection(other)
	if otherProjection == nil || otherProjection.WorkspaceID != "ws-semantic-b" || otherProjection.RunID != "run-semantic-b" || otherProjection.StateRevision != 4 {
		t.Fatalf("other semantic projection = %+v", otherProjection)
	}
	if got.WorkspaceID != "ws-semantic-a" || got.RunID != "run-semantic-a" || got.StateRevision != 17 {
		t.Fatalf("semantic projection was aliased across run scope: %+v", got)
	}
}

func TestRuntimeSemanticPlanningProjectionPreservesProposalOwnerFactsAndOriginalDiagnostics(t *testing.T) {
	state := semanticPlanningProjectionState("ws-semantic", "run-semantic", 23)
	proposalA := semanticPlanningProjectionProposal("cwi-a", runtimecontract.RuntimeModuleFile, runtimecontract.StepKindFileWrite, "source/a.go")
	proposalB := semanticPlanningProjectionProposal("cwi-b", runtimecontract.RuntimeModuleGit, runtimecontract.StepKindGitWrite, "")
	diagnostic := semanticPlanningProjectionDiagnostic()
	state.SemanticPlanning.Phase = runtimecontract.RuntimeSemanticPlanningFeasibilityRequired
	state.SemanticPlanning.Attempt = 3
	state.SemanticPlanning.CatalogRefreshAttempts = 1
	state.SemanticPlanning.LogicalRequestID = "semantic-request-1"
	state.SemanticPlanning.Manifest = &runtimecontract.RuntimeSemanticPlanManifest{Hash: "manifest-current"}
	state.SemanticPlanning.Document = &runtimecontract.RuntimeSemanticPlanDocument{
		PlanStep:  runtimecontract.RuntimeSemanticPlanStepProofBatch,
		Proposals: []runtimecontract.RuntimeSemanticProofProposal{proposalA, proposalB},
	}
	state.SemanticPlanning.DocumentHash = "document-current"
	state.SemanticPlanning.FeasibilityBatchID = "batch-current"
	state.SemanticPlanning.FeasibilitySetHash = "set-current"
	state.SemanticPlanning.Feasibilities = []runtimecontract.RuntimeSemanticProofFeasibility{{
		CoverageWorkItemID: "cwi-a", ProposalHash: runtimecontract.StableRuntimeSemanticProofProposalHash(proposalA), OwnerModuleID: runtimecontract.RuntimeModuleFile,
		Status: runtimecontract.RuntimeSemanticFeasibilityPrerequisiteRequired, ObservedRevision: "tree-9",
		TargetRefs: []string{"source/a.go"}, ValidationRecipeID: "recipe-a", RequiredCheckRefs: []string{"check-a"},
		OriginalDiagnostic: &diagnostic,
	}}
	state.SemanticPlanning.Rejections = []runtimecontract.RuntimeOriginalDiagnostic{diagnostic}
	state.SemanticPlanning.FeasibilityGaps = []runtimecontract.RuntimeSemanticFeasibilityGap{{
		RefreshGeneration: 1, ManifestHash: "manifest-old", DocumentHash: "document-old",
		FeasibilityBatchID: "batch-old", FeasibilitySetHash: "set-old",
		Feasibilities: []runtimecontract.RuntimeSemanticProofFeasibility{{
			CoverageWorkItemID: "cwi-b", OwnerModuleID: runtimecontract.RuntimeModuleGit,
			Status: runtimecontract.RuntimeSemanticFeasibilityStale, ObservedRevision: "repo-3", OriginalDiagnostic: &diagnostic,
		}},
		Rejections: []runtimecontract.RuntimeOriginalDiagnostic{diagnostic},
	}}

	got := runtimeSemanticPlanningProjection(state)
	if got == nil || got.State != "feasibility" || got.Attempt != 3 || got.CatalogRefreshAttempts != 1 ||
		got.LogicalRequestID != "semantic-request-1" || got.ManifestHash != "manifest-current" ||
		got.DocumentHash != "document-current" || got.FeasibilityBatchID != "batch-current" || got.FeasibilitySetHash != "set-current" {
		t.Fatalf("semantic feasibility projection = %+v", got)
	}
	if len(got.Items) != 2 {
		t.Fatalf("semantic items = %+v", got.Items)
	}
	first := got.Items[0]
	if first.Status != string(runtimecontract.RuntimeSemanticFeasibilityPrerequisiteRequired) || len(first.Proofs) != 1 {
		t.Fatalf("observed semantic item = %+v", first)
	}
	firstProof := first.Proofs[0]
	if firstProof.OwnerModuleID != runtimecontract.RuntimeModuleFile || firstProof.OperationKind != runtimecontract.StepKindFileWrite ||
		len(firstProof.TargetRefs) != 1 || firstProof.TargetRefs[0] != "source/a.go" ||
		firstProof.ValidationRecipeID != "recipe-a" || len(firstProof.RequiredCheckRefs) != 1 || firstProof.RequiredCheckRefs[0] != "check-a" ||
		firstProof.ObservedRevision != "tree-9" || firstProof.Diagnostic == nil {
		t.Fatalf("observed semantic proof = %+v", firstProof)
	}
	second := got.Items[1]
	if second.Status != "awaiting_feasibility" || len(second.Proofs) != 1 ||
		second.Proofs[0].OwnerModuleID != runtimecontract.RuntimeModuleGit || second.Proofs[0].OperationKind != runtimecontract.StepKindGitWrite {
		t.Fatalf("awaiting semantic item = %+v", second)
	}
	assertSemanticProjectionDiagnostic(t, *firstProof.Diagnostic)
	if len(got.Rejections) != 1 {
		t.Fatalf("semantic rejections = %+v", got.Rejections)
	}
	assertSemanticProjectionDiagnostic(t, got.Rejections[0])
	if len(got.FeasibilityGaps) != 1 || got.FeasibilityGaps[0].RefreshGeneration != 1 ||
		got.FeasibilityGaps[0].ManifestHash != "manifest-old" || got.FeasibilityGaps[0].DocumentHash != "document-old" ||
		got.FeasibilityGaps[0].FeasibilityBatchID != "batch-old" || got.FeasibilityGaps[0].FeasibilitySetHash != "set-old" ||
		len(got.FeasibilityGaps[0].Items) != 1 || len(got.FeasibilityGaps[0].Items[0].Proofs) != 1 ||
		got.FeasibilityGaps[0].Items[0].Proofs[0].ObservedRevision != "repo-3" ||
		len(got.FeasibilityGaps[0].Rejections) != 1 {
		t.Fatalf("semantic gap history = %+v", got.FeasibilityGaps)
	}
}

func TestRuntimeSemanticPlanningProjectionRetainsIndependentProofsForOneCWI(t *testing.T) {
	state := semanticPlanningProjectionState("ws-semantic", "run-semantic", 25)
	state.SemanticPlanning.Phase = runtimecontract.RuntimeSemanticPlanningFeasibilityRequired
	stateChange := semanticPlanningProjectionProposal("cwi-a", runtimecontract.RuntimeModuleFile, runtimecontract.StepKindFileWrite, "source/a.go")
	stateChange.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindStateChange}
	validation := semanticPlanningProjectionProposal("cwi-a", runtimecontract.RuntimeModuleShell, runtimecontract.RuntimeBlockKindManagedValidationSession, "source/a.go")
	validation.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindValidation}
	validation.ValidationRecipeID = "recipe-validate"
	validation.RequiredCheckRefs = []string{"check-validate"}
	state.SemanticPlanning.Document = &runtimecontract.RuntimeSemanticPlanDocument{
		PlanStep:  runtimecontract.RuntimeSemanticPlanStepProofBatch,
		Proposals: []runtimecontract.RuntimeSemanticProofProposal{stateChange, validation},
	}
	state.SemanticPlanning.Feasibilities = []runtimecontract.RuntimeSemanticProofFeasibility{
		{
			CoverageWorkItemID: "cwi-a", ProposalHash: runtimecontract.StableRuntimeSemanticProofProposalHash(stateChange),
			OwnerModuleID: runtimecontract.RuntimeModuleFile, Status: runtimecontract.RuntimeSemanticFeasibilityFeasible,
			ObservedRevision: "tree-10", TargetRefs: []string{"source/a.go"},
		},
		{
			CoverageWorkItemID: "cwi-a", ProposalHash: runtimecontract.StableRuntimeSemanticProofProposalHash(validation),
			OwnerModuleID: runtimecontract.RuntimeModuleShell, Status: runtimecontract.RuntimeSemanticFeasibilityFeasible,
			ObservedRevision: "validation-10", TargetRefs: []string{"source/a.go"},
			ValidationRecipeID: "recipe-validate", RequiredCheckRefs: []string{"check-validate"},
		},
	}

	got := runtimeSemanticPlanningProjection(state)
	if got == nil || len(got.Items) != 2 || got.Items[0].CoverageWorkItemID != "cwi-a" ||
		len(got.Items[0].Proofs) != 2 || got.Items[0].Status != string(runtimecontract.RuntimeSemanticFeasibilityFeasible) {
		t.Fatalf("independent semantic proofs were collapsed: %+v", got)
	}
	proofs := got.Items[0].Proofs
	if proofs[0].EvidenceKind != runtimecontract.EvidenceKindStateChange || proofs[0].OwnerModuleID != runtimecontract.RuntimeModuleFile ||
		proofs[1].EvidenceKind != runtimecontract.EvidenceKindValidation || proofs[1].OwnerModuleID != runtimecontract.RuntimeModuleShell ||
		proofs[1].ValidationRecipeID != "recipe-validate" || len(proofs[1].RequiredCheckRefs) != 1 || proofs[1].RequiredCheckRefs[0] != "check-validate" {
		t.Fatalf("independent semantic proof details = %+v", proofs)
	}
}

func TestRuntimeSemanticPlanningProjectionPreservesFailureAndSealedBinding(t *testing.T) {
	state := semanticPlanningProjectionState("ws-semantic", "run-semantic", 29)
	failure := semanticPlanningProjectionDiagnostic()
	state.SemanticPlanning.Phase = runtimecontract.RuntimeSemanticPlanningFailed
	state.SemanticPlanning.Failure = &failure
	failed := runtimeSemanticPlanningProjection(state)
	if failed == nil || failed.State != "rejected" || failed.Failure == nil || len(failed.Items) != 2 || failed.Items[0].Status != "rejected" {
		t.Fatalf("failed semantic projection = %+v", failed)
	}
	assertSemanticProjectionDiagnostic(t, *failed.Failure)

	state.SemanticPlanning.Phase = runtimecontract.RuntimeSemanticPlanningSealed
	state.SemanticPlanning.Failure = nil
	state.SemanticPlanning.SealedBindingSetHash = "binding-set-final"
	sealed := runtimeSemanticPlanningProjection(state)
	if sealed == nil || sealed.State != "sealed" || sealed.SealedBindingSetHash != "binding-set-final" || sealed.Items[0].Status != "sealed" {
		t.Fatalf("sealed semantic projection = %+v", sealed)
	}
}

func semanticPlanningProjectionState(workspaceID, runID string, revision uint64) runtimecontract.RuntimeStateRecord {
	return runtimecontract.RuntimeStateRecord{
		StateRevision: revision,
		Facts: runtimecontract.RuntimeFacts{
			WorkspaceID: common.WorkspaceID(workspaceID), RunID: common.RunID(runID), DefinitionRevision: "definition-semantic",
		},
		SemanticPlanning: runtimecontract.RuntimeSemanticPlanningLedger{
			SchemaVersion:  runtimecontract.RuntimeSemanticPlanningLedgerSchemaCurrent,
			DefinitionSeed: runtimecontract.RuntimeDefinitionSeed{CoverageItems: []runtimecontract.RuntimeCoverageItem{{ID: "cwi-a"}, {ID: "cwi-b"}}},
		},
	}
}

func semanticPlanningProjectionProposal(coverageID, owner, kind, targetRef string) runtimecontract.RuntimeSemanticProofProposal {
	proposal := runtimecontract.RuntimeSemanticProofProposal{
		CoverageWorkItemID: common.CoverageWorkItemID(coverageID), OwnerModuleID: owner, OperationKind: kind,
		ValidationRecipeID: "recipe-proposal", RequiredCheckRefs: []string{"check-proposal"},
	}
	if targetRef != "" {
		proposal.Target.Policy = runtimecontract.RuntimeOperationTargetPolicyExactTarget
		proposal.Target.ExactTargetRefs = []string{targetRef}
	}
	return proposal
}

func semanticPlanningProjectionDiagnostic() runtimecontract.RuntimeOriginalDiagnostic {
	return runtimecontract.RuntimeOriginalDiagnostic{
		SchemaVersion: "diagnostic-v1", OwnerModuleID: runtimecontract.RuntimeModuleFile,
		OriginalClass: "owner_contract", OriginalCode: "target_stale", OriginalMessage: " original owner message ", Signature: "signature-1",
		OperationID: "op-1", TargetRefs: []string{"source/a.go", "source/b.go"}, ExpectedRevision: "tree-8", ObservedRevision: "tree-9",
		FieldViolations: []runtimecontract.RuntimeFieldViolation{{
			Path: "$.operations[0].parameters", Actual: "summary", Allowed: []string{"targetRef", "content"},
			Required: []string{"content"}, Forbidden: []string{"payload", "summary"},
		}},
		DetailsRef: "runtime-diagnostic/ws/run/op-1", DetailsHash: "details-hash", DetailsBytes: 8193,
		ContentType: "application/json", OmittedReason: "bounded_payload", Truncated: true, Redacted: true,
	}
}

func assertSemanticProjectionDiagnostic(t *testing.T, got projectioncontract.RuntimeSemanticDiagnostic) {
	t.Helper()
	if got.SchemaVersion != "diagnostic-v1" || got.OwnerModuleID != runtimecontract.RuntimeModuleFile ||
		got.OriginalClass != "owner_contract" || got.OriginalCode != "target_stale" || got.OriginalMessage != " original owner message " ||
		got.Signature != "signature-1" || got.OperationID != "op-1" || len(got.TargetRefs) != 2 ||
		got.ExpectedRevision != "tree-8" || got.ObservedRevision != "tree-9" || len(got.FieldViolations) != 1 ||
		got.FieldViolations[0].Path != "$.operations[0].parameters" || got.FieldViolations[0].Actual != "summary" ||
		len(got.FieldViolations[0].Allowed) != 2 || got.FieldViolations[0].Allowed[0] != "targetRef" ||
		len(got.FieldViolations[0].Required) != 1 || got.FieldViolations[0].Required[0] != "content" ||
		len(got.FieldViolations[0].Forbidden) != 2 || got.FieldViolations[0].Forbidden[1] != "summary" ||
		got.DetailsRef != "runtime-diagnostic/ws/run/op-1" || got.DetailsHash != "details-hash" || got.DetailsBytes != 8193 ||
		got.ContentType != "application/json" || got.OmittedReason != "bounded_payload" || !got.Truncated || !got.Redacted {
		t.Fatalf("semantic diagnostic lost original fields: %+v", got)
	}
}
