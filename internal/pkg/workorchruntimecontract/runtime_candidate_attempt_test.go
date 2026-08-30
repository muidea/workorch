package workorchruntimecontract

import (
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func runtimeCandidateAttemptCorrectableDirective(manifestHash string) *RuntimeCandidateCorrectionDirective {
	return (&RuntimeCandidateCorrectionDirective{
		Disposition:  RuntimeCandidateCorrectionCorrectableOnce,
		Objectives:   []RuntimeCandidateCorrectionObjective{RuntimeCandidateCorrectionSupplyRequiredField},
		ManifestHash: manifestHash,
		Witness: &RuntimeCandidateCorrectionWitness{
			Dimension: RuntimeCandidateCorrectionWitnessSchemaField,
			Path:      "$.operations", RejectedValue: "field_absent", AlternativeValue: "field_present",
		},
	}).Clone()
}

func runtimeCandidateAttemptCorrectableFailure(manifestHash string) *RuntimeCandidateFailure {
	return &RuntimeCandidateFailure{
		Class: RuntimeCandidateFailureClassSchema, Code: "candidate_schema_invalid", Signature: "candidate-schema-invalid", Reason: "required operations field is missing",
		Diagnostic: &RuntimeCandidateFailureDiagnostic{
			Path: "$.operations", Actual: "missing_operations", Required: []string{"operations"}, ManifestHash: manifestHash,
			Objectives: []RuntimeCandidateCorrectionObjective{RuntimeCandidateCorrectionSupplyRequiredField},
		},
	}
}

func TestRuntimeCandidateAttemptScopeValidationRequiresExplicitContractAndRejectsDuplicate(t *testing.T) {
	facts := testRuntimeFacts()
	decision := validDecisionForState(ConvergenceStateNeedStateChange)
	current := decision.AuthorityTransaction()
	resealed := current
	contractID := StableAuthorityContractID(facts.WorkspaceID, facts.RunID, current)
	failure := runtimeCandidateAttemptCorrectableFailure("manifest-1")
	values := []RuntimeCandidateAttemptRecord{
		{Transaction: resealed, Attempt: 0, StablePrefixHash: "stable-prefix", ManifestHash: "manifest-1", LogicalRequestID: "request-0", State: RuntimeCandidateAttemptStateFailed, Failure: failure, Correction: runtimeCandidateAttemptCorrectableDirective("manifest-1")},
		{AuthorityContractID: contractID, Transaction: current, Attempt: 1, StablePrefixHash: "stable-prefix", ManifestHash: "manifest-1", LogicalRequestID: "request-1", State: RuntimeCandidateAttemptStatePending},
	}
	if err := ValidateRuntimeCandidateAttemptRecordsForScope(facts.WorkspaceID, facts.RunID, values); err == nil {
		t.Fatal("candidate attempt without authority contract id was accepted")
	}
	values[0].AuthorityContractID = contractID
	if err := ValidateRuntimeCandidateAttemptRecordsForScope(facts.WorkspaceID, facts.RunID, values); err != nil {
		t.Fatalf("explicit same-contract retry rejected: %v", err)
	}
	duplicate := current
	duplicate.AuthoritySnapshotID = "duplicate-snapshot"
	duplicate.Attempt = 1
	values = []RuntimeCandidateAttemptRecord{
		{AuthorityContractID: contractID, Transaction: current, Attempt: 0, StablePrefixHash: "stable-prefix", ManifestHash: "manifest-1", LogicalRequestID: "request-0", State: RuntimeCandidateAttemptStateFailed, Failure: failure, Correction: runtimeCandidateAttemptCorrectableDirective("manifest-1")},
		{AuthorityContractID: contractID, Transaction: duplicate, Attempt: 0, StablePrefixHash: "stable-prefix", ManifestHash: "manifest-1", LogicalRequestID: "request-duplicate", State: RuntimeCandidateAttemptStateFailed, Failure: failure, Correction: runtimeCandidateAttemptCorrectableDirective("manifest-1")},
	}
	if err := ValidateRuntimeCandidateAttemptRecordsForScope(facts.WorkspaceID, facts.RunID, values); err == nil {
		t.Fatal("new duplicate attempt 0 was accepted across snapshots")
	}
}

func TestRuntimeCandidateAttemptAcceptedRequiresIndependentTargetAuthorizationCommitment(t *testing.T) {
	facts := testRuntimeFacts()
	transaction := validDecisionForState(ConvergenceStateNeedStateChange).AuthorityTransaction()
	projection := []RuntimeDispatchOperationTargetAuthorization{{
		OperationID: "op-1", OperationHash: "operation-hash-1", RequireTargets: true,
		Target: RuntimeOperationTargetAuthorization{Policy: RuntimeOperationTargetPolicyExactTarget, ExactTargetRefs: []string{"target.txt"}},
	}}
	record := RuntimeCandidateAttemptRecord{
		AuthorityContractID: StableAuthorityContractID(facts.WorkspaceID, facts.RunID, transaction),
		Transaction:         transaction, Attempt: 0, StablePrefixHash: "stable-prefix", ManifestHash: "manifest-1",
		LogicalRequestID: "request-0", State: RuntimeCandidateAttemptStateAccepted,
		CandidateHash: "candidate-1", DispatchID: "dispatch-1",
		AdmissionTargetAuthorizations: projection,
		TargetAuthorizationHash:       StableRuntimeDispatchOperationTargetAuthorizationsHash(projection),
		AdmissionCoverageBindingHash:  StableRuntimeCoverageEvidenceBindingSnapshotHash(nil),
		DispatchAuthorityHash:         "dispatch-authority-1",
	}
	if err := ValidateRuntimeCandidateAttemptRecordsForScope(facts.WorkspaceID, facts.RunID, []RuntimeCandidateAttemptRecord{record}); err != nil {
		t.Fatalf("accepted attempt target commitment rejected: %v", err)
	}

	missing := record
	missing.TargetAuthorizationHash = ""
	if err := ValidateRuntimeCandidateAttemptRecordsForScope(facts.WorkspaceID, facts.RunID, []RuntimeCandidateAttemptRecord{missing}); err == nil ||
		!strings.Contains(err.Error(), "target-authorization") {
		t.Fatalf("accepted attempt missing target commitment error = %v", err)
	}

	pending := record
	pending.State = RuntimeCandidateAttemptStatePending
	pending.CandidateHash = ""
	pending.DispatchID = ""
	pending.DispatchAuthorityHash = ""
	if err := ValidateRuntimeCandidateAttemptRecordsForScope(facts.WorkspaceID, facts.RunID, []RuntimeCandidateAttemptRecord{pending}); err == nil ||
		!strings.Contains(err.Error(), "cannot carry dispatch authority") {
		t.Fatalf("pending attempt retained target commitment error = %v", err)
	}
	pendingProjectionOnly := pending
	pendingProjectionOnly.TargetAuthorizationHash = ""
	if err := ValidateRuntimeCandidateAttemptRecordsForScope(facts.WorkspaceID, facts.RunID, []RuntimeCandidateAttemptRecord{pendingProjectionOnly}); err == nil ||
		!strings.Contains(err.Error(), "cannot carry dispatch authority") {
		t.Fatalf("pending attempt retained admission projection error = %v", err)
	}

	failedProjectionOnly := record.Clone()
	failedProjectionOnly.State = RuntimeCandidateAttemptStateFailed
	failedProjectionOnly.CandidateHash = ""
	failedProjectionOnly.DispatchID = ""
	failedProjectionOnly.TargetAuthorizationHash = ""
	failedProjectionOnly.DispatchAuthorityHash = ""
	failedProjectionOnly.Failure = runtimeCandidateAttemptCorrectableFailure(record.ManifestHash)
	failedProjectionOnly.Correction = runtimeCandidateAttemptCorrectableDirective(record.ManifestHash)
	if err := ValidateRuntimeCandidateAttemptRecordsForScope(facts.WorkspaceID, facts.RunID, []RuntimeCandidateAttemptRecord{failedProjectionOnly}); err == nil ||
		!strings.Contains(err.Error(), "cannot carry dispatch authority") {
		t.Fatalf("failed attempt retained admission projection error = %v", err)
	}

	invalidatedProjectionOnly := record.Clone()
	invalidatedProjectionOnly.State = RuntimeCandidateAttemptStateInvalidated
	invalidatedProjectionOnly.CandidateHash = ""
	invalidatedProjectionOnly.DispatchID = ""
	invalidatedProjectionOnly.TargetAuthorizationHash = ""
	invalidatedProjectionOnly.DispatchAuthorityHash = ""
	invalidatedProjectionOnly.Stale = &RuntimePrerequisiteStale{
		PrerequisiteID: "target-snapshot", ObservationID: "observation-1",
		Domain: RuntimeResourceWorkspaceTree, OwnerModuleID: RuntimeModuleFile,
		SubjectRef: "workspace", ExpectedRevision: "tree-v1", ObservedRevision: "tree-v2", Reason: "workspace changed",
	}
	if err := ValidateRuntimeCandidateAttemptRecordsForScope(facts.WorkspaceID, facts.RunID, []RuntimeCandidateAttemptRecord{invalidatedProjectionOnly}); err == nil ||
		!strings.Contains(err.Error(), "cannot carry dispatch authority") {
		t.Fatalf("invalidated attempt retained admission projection error = %v", err)
	}

	tampered := record.Clone()
	tampered.AdmissionTargetAuthorizations[0].Target.ExactTargetRefs = []string{"other.txt"}
	if err := ValidateRuntimeCandidateAttemptRecordsForScope(facts.WorkspaceID, facts.RunID, []RuntimeCandidateAttemptRecord{tampered}); err == nil ||
		!strings.Contains(err.Error(), "does not match admission projection") {
		t.Fatalf("accepted attempt projection/hash mismatch error = %v", err)
	}

	cloned := record.Clone()
	cloned.AdmissionTargetAuthorizations[0].Target.ExactTargetRefs[0] = "clone.txt"
	if record.AdmissionTargetAuthorizations[0].Target.ExactTargetRefs[0] != "target.txt" {
		t.Fatal("candidate attempt clone aliased admission target projection")
	}
}

func TestRuntimeCandidateAttemptAcceptedManagedValidationRequiresIndependentRecipeCommitment(t *testing.T) {
	facts := testRuntimeFacts()
	transaction := validDecisionForState(ConvergenceStateNeedValidation).AuthorityTransaction()
	targetProjection := []RuntimeDispatchOperationTargetAuthorization{{
		OperationID: "validate-1", OperationHash: "operation-hash-1", RequireTargets: true,
		Target: RuntimeOperationTargetAuthorization{Policy: RuntimeOperationTargetPolicyExactTarget, ExactTargetRefs: []string{"target.txt"}},
	}}
	recipeProjection := []RuntimeValidationRecipe{{
		ID: "recipe-1", Command: "go", Args: []string{"test", "./..."}, TimeoutMillis: 5000,
	}}
	record := RuntimeCandidateAttemptRecord{
		AuthorityContractID: StableAuthorityContractID(facts.WorkspaceID, facts.RunID, transaction),
		Transaction:         transaction, Attempt: 0, StablePrefixHash: "stable-prefix", ManifestHash: "manifest-1",
		LogicalRequestID: "request-0", State: RuntimeCandidateAttemptStateAccepted,
		CandidateHash: "candidate-1", DispatchID: "dispatch-1",
		AdmissionTargetAuthorizations: targetProjection,
		TargetAuthorizationHash:       StableRuntimeDispatchOperationTargetAuthorizationsHash(targetProjection),
		AdmissionCoverageBindingHash:  StableRuntimeCoverageEvidenceBindingSnapshotHash(nil),
		AdmissionValidationRecipes:    recipeProjection,
		ValidationRecipeHash:          StableRuntimeValidationRecipeSnapshotHash(recipeProjection),
		DispatchAuthorityHash:         "dispatch-authority-1",
	}
	if err := ValidateRuntimeCandidateAttemptRecordsForScope(facts.WorkspaceID, facts.RunID, []RuntimeCandidateAttemptRecord{record}); err != nil {
		t.Fatalf("accepted attempt validation-recipe commitment rejected: %v", err)
	}

	missingHash := record.Clone()
	missingHash.ValidationRecipeHash = ""
	if err := ValidateRuntimeCandidateAttemptRecordsForScope(facts.WorkspaceID, facts.RunID, []RuntimeCandidateAttemptRecord{missingHash}); err == nil ||
		!strings.Contains(err.Error(), "validation-recipe hash") {
		t.Fatalf("accepted attempt missing validation-recipe commitment error = %v", err)
	}

	tampered := record.Clone()
	tampered.AdmissionValidationRecipes[0].Args = []string{"test", "./changed/..."}
	if err := ValidateRuntimeCandidateAttemptRecordsForScope(facts.WorkspaceID, facts.RunID, []RuntimeCandidateAttemptRecord{tampered}); err == nil ||
		!strings.Contains(err.Error(), "validation-recipe hash does not match") {
		t.Fatalf("accepted attempt recipe projection/hash mismatch error = %v", err)
	}

	pending := record.Clone()
	pending.State = RuntimeCandidateAttemptStatePending
	pending.CandidateHash = ""
	pending.DispatchID = ""
	pending.AdmissionTargetAuthorizations = nil
	pending.TargetAuthorizationHash = ""
	pending.DispatchAuthorityHash = ""
	if err := ValidateRuntimeCandidateAttemptRecordsForScope(facts.WorkspaceID, facts.RunID, []RuntimeCandidateAttemptRecord{pending}); err == nil ||
		!strings.Contains(err.Error(), "cannot carry dispatch authority") {
		t.Fatalf("pending attempt retained validation-recipe commitment error = %v", err)
	}

	cloned := record.Clone()
	cloned.AdmissionValidationRecipes[0].Args[0] = "changed"
	if record.AdmissionValidationRecipes[0].Args[0] != "test" {
		t.Fatal("candidate attempt clone aliased admission validation-recipe projection")
	}
}

func TestRuntimeCandidateAttemptFailedRequiresManifestBoundTotalCorrection(t *testing.T) {
	facts := testRuntimeFacts()
	transaction := validDecisionForState(ConvergenceStateNeedStateChange).AuthorityTransaction()
	record := RuntimeCandidateAttemptRecord{
		AuthorityContractID: StableAuthorityContractID(facts.WorkspaceID, facts.RunID, transaction), Transaction: transaction,
		Attempt: 0, StablePrefixHash: "stable-prefix", ManifestHash: "manifest-1", LogicalRequestID: "request-0",
		State:   RuntimeCandidateAttemptStateFailed,
		Failure: &RuntimeCandidateFailure{Class: RuntimeCandidateFailureClassContract, Code: "candidate_schema_invalid", Reason: "missing operations"},
	}
	if err := ValidateRuntimeCandidateAttemptRecordsForScope(facts.WorkspaceID, facts.RunID, []RuntimeCandidateAttemptRecord{record}); err == nil || !strings.Contains(err.Error(), "total correction disposition") {
		t.Fatalf("failed attempt without correction disposition err=%v", err)
	}
	record.Correction = runtimeCandidateAttemptCorrectableDirective(record.ManifestHash)
	record.Failure = runtimeCandidateAttemptCorrectableFailure(record.ManifestHash)
	if err := ValidateRuntimeCandidateAttemptRecordsForScope(facts.WorkspaceID, facts.RunID, []RuntimeCandidateAttemptRecord{record}); err != nil {
		t.Fatalf("manifest-bound correctable attempt rejected: %v", err)
	}
	state := RuntimeStateRecord{CandidateAttempts: []RuntimeCandidateAttemptRecord{record}}
	clone := state.Clone()
	record.Correction.Witness.AlternativeValue = "mutated"
	if clone.CandidateAttempts[0].Correction == nil || clone.CandidateAttempts[0].Correction.Witness == nil || clone.CandidateAttempts[0].Correction.Witness.AlternativeValue != "field_present" {
		t.Fatalf("candidate correction witness clone shares mutable state: %+v", clone.CandidateAttempts[0].Correction)
	}
}

func TestRuntimeCandidateAttemptRejectsFailureCorrectionLineageMismatch(t *testing.T) {
	facts := testRuntimeFacts()
	transaction := validDecisionForState(ConvergenceStateNeedStateChange).AuthorityTransaction()
	newRecord := func() RuntimeCandidateAttemptRecord {
		return RuntimeCandidateAttemptRecord{
			AuthorityContractID: StableAuthorityContractID(facts.WorkspaceID, facts.RunID, transaction), Transaction: transaction,
			Attempt: 0, StablePrefixHash: "stable-prefix", ManifestHash: "manifest-1", LogicalRequestID: "request-0",
			State: RuntimeCandidateAttemptStateFailed, Failure: runtimeCandidateAttemptCorrectableFailure("manifest-1"),
			Correction: runtimeCandidateAttemptCorrectableDirective("manifest-1"),
		}
	}
	if err := ValidateRuntimeCandidateAttemptRecordsForScope(facts.WorkspaceID, facts.RunID, []RuntimeCandidateAttemptRecord{newRecord()}); err != nil {
		t.Fatalf("valid failure/correction lineage rejected: %v", err)
	}

	tests := map[string]func(*RuntimeCandidateAttemptRecord){
		"missing field as semantic target": func(record *RuntimeCandidateAttemptRecord) {
			record.Failure.Diagnostic.Path = "$.operations[0].targetRefs"
			record.Failure.Diagnostic.Actual = "missing_targetRefs"
			record.Failure.Diagnostic.Required = []string{"targetRefs"}
			record.Correction.Witness = &RuntimeCandidateCorrectionWitness{
				Dimension: RuntimeCandidateCorrectionWitnessTarget, OperationKind: RuntimeBlockKindPatchApply,
				Path: "$.operations[0].targetRefs", RejectedValue: "", AlternativeValue: "README.md",
				RejectedShapeHash: "rejected-shape", AlternativeShapeHash: "alternative-shape",
			}
		},
		"field present without absence observation": func(record *RuntimeCandidateAttemptRecord) {
			record.Failure.Diagnostic.Path = "$.operations[0].targetRefs"
			record.Failure.Diagnostic.Actual = "invalid_targetRefs"
			record.Failure.Diagnostic.Required = []string{"targetRefs"}
			record.Correction.Witness = &RuntimeCandidateCorrectionWitness{
				Dimension: RuntimeCandidateCorrectionWitnessSchemaField,
				Path:      "$.operations[0].targetRefs", RejectedValue: "field_absent", AlternativeValue: "field_present",
			}
		},
		"schema path": func(record *RuntimeCandidateAttemptRecord) {
			record.Correction.Witness.Path = "$.unexpected"
		},
		"objective": func(record *RuntimeCandidateAttemptRecord) {
			record.Correction.Objectives = []RuntimeCandidateCorrectionObjective{RuntimeCandidateCorrectionRemoveForbiddenField}
		},
		"diagnostic manifest": func(record *RuntimeCandidateAttemptRecord) {
			record.Failure.Diagnostic.ManifestHash = "other-manifest"
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			record := newRecord()
			mutate(&record)
			if err := ValidateRuntimeCandidateAttemptRecordsForScope(facts.WorkspaceID, facts.RunID, []RuntimeCandidateAttemptRecord{record}); err == nil ||
				!strings.Contains(err.Error(), "failure/correction lineage") {
				t.Fatalf("mismatched persisted pair error = %v", err)
			}
		})
	}

	semanticFailure := &RuntimeCandidateFailure{
		Class: RuntimeCandidateFailureClassContract, Code: "candidate_target_outside_grounding", Signature: "target-outside", Reason: "target is outside grounding",
		Diagnostic: &RuntimeCandidateFailureDiagnostic{
			Path: "$.operations[0].targetRefs", Actual: "main.go", Allowed: []string{"support.go"}, ManifestHash: "manifest-1", RejectedShapeHash: "rejected-shape",
			Objectives: []RuntimeCandidateCorrectionObjective{RuntimeCandidateCorrectionReplaceTarget},
		},
	}
	semanticCorrection := &RuntimeCandidateCorrectionDirective{
		Disposition: RuntimeCandidateCorrectionCorrectableOnce, ManifestHash: "manifest-1",
		Objectives: []RuntimeCandidateCorrectionObjective{RuntimeCandidateCorrectionReplaceTarget},
		Witness: &RuntimeCandidateCorrectionWitness{
			Dimension: RuntimeCandidateCorrectionWitnessTarget, OperationKind: RuntimeBlockKindPatchApply,
			Path: "$.operations[0].targetRefs", RejectedValue: "main.go", AlternativeValue: "support.go",
			RejectedShapeHash: "rejected-shape", AlternativeShapeHash: "alternative-shape",
		},
	}
	if err := ValidateRuntimeCandidateFailureCorrectionPair(semanticFailure, semanticCorrection); err != nil {
		t.Fatalf("valid semantic failure/correction lineage rejected: %v", err)
	}
	semanticFailure.Diagnostic.Objectives = []RuntimeCandidateCorrectionObjective{RuntimeCandidateCorrectionSelectOperationKind}
	semanticCorrection.Objectives = []RuntimeCandidateCorrectionObjective{RuntimeCandidateCorrectionSelectOperationKind}
	if err := ValidateRuntimeCandidateFailureCorrectionPair(semanticFailure, semanticCorrection); err == nil || !strings.Contains(err.Error(), "incompatible with witness dimension") {
		t.Fatalf("same wrong objective/witness mapping error = %v", err)
	}
	semanticFailure.Diagnostic.Objectives = []RuntimeCandidateCorrectionObjective{RuntimeCandidateCorrectionReplaceTarget}
	semanticCorrection.Objectives = []RuntimeCandidateCorrectionObjective{RuntimeCandidateCorrectionReplaceTarget}
	semanticFailure.Diagnostic.Actual = "other.go"
	if err := ValidateRuntimeCandidateFailureCorrectionPair(semanticFailure, semanticCorrection); err == nil || !strings.Contains(err.Error(), "rejected value differs") {
		t.Fatalf("semantic actual mismatch error = %v", err)
	}
	semanticFailure.Diagnostic.Actual = "main.go"
	semanticFailure.Diagnostic.RejectedShapeHash = "other-shape"
	if err := ValidateRuntimeCandidateFailureCorrectionPair(semanticFailure, semanticCorrection); err == nil || !strings.Contains(err.Error(), "rejected shape") || !strings.Contains(err.Error(), "differs") {
		t.Fatalf("semantic shape mismatch error = %v", err)
	}

	rejectedFinal := &RuntimeCandidateCorrectionDirective{Disposition: RuntimeCandidateCorrectionRejectedFinal, ManifestHash: "manifest-1", TerminalCode: "candidate_authority_exhausted"}
	if err := ValidateRuntimeCandidateFailureCorrectionPair(&RuntimeCandidateFailure{Class: RuntimeCandidateFailureClassContract, Reason: "terminal"}, rejectedFinal); err != nil {
		t.Fatalf("rejected-final pair unexpectedly required correction diagnostic: %v", err)
	}
}

func TestRuntimeCandidateAttemptBindingContradictionValidatesAndClones(t *testing.T) {
	facts := testRuntimeFacts()
	transaction := validDecisionForState(ConvergenceStateNeedStateChange).AuthorityTransaction()
	failure := FailureFact{
		Class: FailureClassCandidateContractFailure, Code: "operation_target_exists", Signature: "file/target-exists",
		Message: "file/write is create-only", TargetRefs: []string{"target.txt"},
	}
	feedback, err := RuntimeFeedbackFromFailureFact(RuntimeFeedbackScope{
		WorkspaceID: facts.WorkspaceID, RunID: facts.RunID, AuthoritySnapshotID: transaction.AuthoritySnapshotID,
	}, RuntimeFeedbackDomainFeasibility, RuntimeFeedbackCategory(RuntimeOwnerFeasibilityCategoryPolicyDisallowed), RuntimeModuleFile, failure)
	if err != nil {
		t.Fatal(err)
	}
	feedback.OriginalDiagnostic.OperationID = "write-target"
	feedback.OriginalDiagnostic.ObservedRevision = "workspace-2"
	policy := RuntimeOwnerPolicyIdentity{OwnerModuleID: RuntimeModuleFile, PolicyRevision: "file-owner-feasibility", CatalogSnapshotID: "catalog-1"}
	observation := RuntimeOwnerFeasibilityObservation{
		Outcome: RuntimeOwnerFeasibilityInfeasible, Feedback: &feedback, OwnerPolicy: policy,
		ConstraintImpact: RuntimeOwnerFeasibilityConstraintImpactSealedTargetAuthorizationContradicted,
	}
	contradiction := RuntimeBindingContradictionIdentity{
		WorkspaceID: facts.WorkspaceID, RunID: facts.RunID, AuthorityInputID: transaction.AuthorityInputID,
		BindingID: "binding-1", BindingSetHash: "binding-set-1", ProposalHash: "proposal-1", ObservationHash: "observation-1",
		OwnerPolicy: policy, ObservedRevision: "workspace-2",
	}
	record := RuntimeCandidateAttemptRecord{
		AuthorityContractID: StableAuthorityContractID(facts.WorkspaceID, facts.RunID, transaction), Transaction: transaction,
		Attempt: 0, StablePrefixHash: "stable-prefix", ManifestHash: "manifest-1", LogicalRequestID: "request-0",
		State: RuntimeCandidateAttemptStateInvalidated, OwnerFeasibility: &observation, BindingContradiction: &contradiction,
	}
	if err := ValidateRuntimeCandidateAttemptRecordsForScope(facts.WorkspaceID, facts.RunID, []RuntimeCandidateAttemptRecord{record}); err != nil {
		t.Fatal(err)
	}
	state := RuntimeStateRecord{CandidateAttempts: []RuntimeCandidateAttemptRecord{record}}
	clone := state.Clone()
	record.BindingContradiction.BindingID = "mutated"
	record.OwnerFeasibility.Feedback.OriginalDiagnostic.OriginalMessage = "mutated"
	if clone.CandidateAttempts[0].BindingContradiction == nil || clone.CandidateAttempts[0].BindingContradiction.BindingID != "binding-1" ||
		clone.CandidateAttempts[0].OwnerFeasibility == nil || clone.CandidateAttempts[0].OwnerFeasibility.Feedback.OriginalDiagnostic.OriginalMessage != "file/write is create-only" {
		t.Fatalf("binding contradiction clone shared mutable identity: %+v", clone.CandidateAttempts[0])
	}
	tampered := clone.CandidateAttempts[0]
	tampered.BindingContradiction.AuthorityInputID = "other-authority-input"
	if err := ValidateRuntimeCandidateAttemptRecordsForScope(facts.WorkspaceID, facts.RunID, []RuntimeCandidateAttemptRecord{tampered}); err == nil {
		t.Fatal("candidate attempt accepted a contradiction from another authority input")
	}
}

func TestRuntimeCandidateAttemptScopeValidationRejectsRunawayAttempts(t *testing.T) {
	facts := testRuntimeFacts()
	transaction := validDecisionForState(ConvergenceStateNeedStateChange).AuthorityTransaction()
	failure := runtimeCandidateAttemptCorrectableFailure("manifest-1")
	values := make([]RuntimeCandidateAttemptRecord, 0, 3)
	contractID := StableAuthorityContractID(facts.WorkspaceID, facts.RunID, transaction)
	for attempt := 0; attempt < 3; attempt++ {
		resealed := transaction
		resealed.AuthoritySnapshotID = "resealed-snapshot-" + string(rune('a'+attempt))
		resealed.Attempt = attempt
		values = append(values, RuntimeCandidateAttemptRecord{
			AuthorityContractID: contractID, Transaction: resealed, Attempt: 0, StablePrefixHash: "stable-prefix", ManifestHash: "manifest-1", LogicalRequestID: "request-runaway",
			State: RuntimeCandidateAttemptStateFailed, Failure: failure, Correction: runtimeCandidateAttemptCorrectableDirective("manifest-1"),
		})
	}
	if err := ValidateRuntimeCandidateAttemptRecordsForScope(facts.WorkspaceID, facts.RunID, values); err == nil {
		t.Fatal("runaway candidate attempts were accepted")
	}
}

func TestRuntimeCandidateFeasibilityRecordValidationRejectsAmbiguousOrAcceptedLedger(t *testing.T) {
	failure := &FailureFact{Class: FailureClassCandidateContractFailure, Code: "target_invalid", Signature: "file/target_invalid", Message: "target is invalid"}
	feedback, err := RuntimeFeedbackFromFailureFact(RuntimeFeedbackScope{WorkspaceID: "ws-1", RunID: "run-1"}, RuntimeFeedbackDomainFeasibility, RuntimeFeedbackCategory(RuntimeOwnerFeasibilityCategoryTargetNotRegular), RuntimeModuleFile, *failure)
	if err != nil {
		t.Fatal(err)
	}
	observation, err := RuntimeOwnerFeasibilityObservationFromOutcome(RuntimeOwnerFeasibilityInfeasible, &feedback, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	values := []RuntimeCandidateFeasibilityRecord{
		{OperationID: "operation-1", AdmissibilitySnapshotID: "inventory-1", CandidateManifestHash: "manifest-1", Observation: &observation},
		{OperationID: "operation-1", AdmissibilitySnapshotID: "inventory-1", CandidateManifestHash: "manifest-1", Observation: &RuntimeOwnerFeasibilityObservation{Outcome: RuntimeOwnerFeasibilityFeasible}},
	}
	if err := ValidateRuntimeCandidateFeasibilityRecords(values); err == nil {
		t.Fatal("duplicate candidate feasibility operation was accepted")
	}
	facts := testRuntimeFacts()
	transaction := validDecisionForState(ConvergenceStateNeedStateChange).AuthorityTransaction()
	values = values[:1]
	attempt := RuntimeCandidateAttemptRecord{
		AuthorityContractID: StableAuthorityContractID(facts.WorkspaceID, facts.RunID, transaction), Transaction: transaction,
		Attempt: 0, StablePrefixHash: "stable-prefix", ManifestHash: "manifest-1", LogicalRequestID: "request-0", State: RuntimeCandidateAttemptStateAccepted, Feasibilities: values,
	}
	if err := ValidateRuntimeCandidateAttemptRecordsForScope(facts.WorkspaceID, facts.RunID, []RuntimeCandidateAttemptRecord{attempt}); err == nil {
		t.Fatal("accepted candidate attempt retained an unsealed feasibility ledger")
	}
}

func TestRuntimeStateRecordRejectsTerminalDecisionWithPendingCandidateAttempt(t *testing.T) {
	facts := testRuntimeFacts()
	facts.EvidenceRevision = InitialRuntimeEvidenceRevision
	binding := CoverageEvidenceBinding{
		ID: "binding/cwi-1", CoverageWorkItemID: "cwi-1", FocusID: facts.Focus.ID, DefinitionRevision: facts.DefinitionRevision,
		Proofs:     []CoverageEvidenceProof{{EvidenceKind: EvidenceKindStateChange, RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable}, AllowedOperationKinds: []string{RuntimeBlockKindPatchApply}}},
		TargetRefs: []string{"target.txt"}, DeclarationRef: "test/runtime-state",
	}.Normalized()
	binding.Hash = StableCoverageEvidenceBindingHash(binding)
	facts.Focus.RequiredEvidenceKinds = []string{EvidenceKindStateChange}
	facts.Focus.TargetRefs = []string{"target.txt"}
	facts.Focus.AcceptedStepKinds = []string{RuntimeBlockKindPatchApply}
	facts.Focus.CoverageEvidenceBindings = []CoverageEvidenceBinding{binding}
	currentBinding := RuntimeCWIBindingRefForCoverageBinding(binding, "manifest-1")
	facts.CWILedgerSchemaVersion = RuntimeCWILedgerSchemaCurrent
	facts.Coverage = []RuntimeCoverageItemFact{{
		ID:                  common.CoverageWorkItemID("cwi-1"),
		State:               RuntimeCoverageItemStateOpen,
		CWIConvergenceState: RuntimeCWIConvergenceStateBound,
		CurrentBindingRef:   &currentBinding,
		BindingLineage:      []RuntimeCWIBindingRef{currentBinding},
		FocusID:             facts.Focus.ID,
	}}
	facts.DefinitionSeed = RuntimeDefinitionSeed{
		SchemaVersion: RuntimeDefinitionSeedSchemaCurrent, DefinitionRevision: facts.DefinitionRevision, PolicyHash: "policy-1",
		CoverageItems: []RuntimeCoverageItem{{ID: "cwi-1", SliceID: "slice-1", SourceScope: "dodItem", SourceID: "dod-1", TargetText: "deliver"}},
		Slices:        []RuntimeAcceptanceSlice{{ID: "slice-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}}},
	}
	facts.DefinitionSeed.Hash = StableRuntimeDefinitionSeedHash(facts.DefinitionSeed)
	facts.DefinitionFocuses = []RuntimeDefinitionFocus{{
		ID: facts.Focus.ID, SliceID: "slice-1", AssessmentScope: "dodItem", AssessmentID: "dod-1", AssessmentText: "deliver", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
		RequiredEvidenceKinds: []string{EvidenceKindStateChange},
	}}
	reconciliation, err := CompileRuntimeCWIReconciliationContract(facts.DefinitionRevision, []CoverageFocus{facts.Focus})
	if err != nil {
		t.Fatal(err)
	}
	facts.ReconciliationContract = reconciliation
	decision := validDecisionForState(ConvergenceStateFailed)
	transaction := decision.AuthorityTransaction()
	semanticPlanning, err := NewRuntimeSemanticPlanningLedger(facts.DefinitionSeed)
	if err != nil {
		t.Fatal(err)
	}
	facts = facts.Normalized()
	executionBudget, err := NewRuntimeExecutionBudgetLedger(facts.ExecutionBudgetPolicy)
	if err != nil {
		t.Fatal(err)
	}
	record := RuntimeStateRecord{
		StateRevision: 1,
		Definition: RuntimeDefinitionContext{StartupInputHash: "startup-input-test",
			Revision: facts.DefinitionRevision, GoalID: "goal-1", GoalDefinition: "goal",
			DoDID: "dod-1", DoDDefinition: "dod", CapabilityContract: []string{"file/write"},
		},
		Facts: facts, SemanticPlanning: semanticPlanning, ExecutionBudget: executionBudget, Decision: decision,
		CandidateAttempts: []RuntimeCandidateAttemptRecord{{
			AuthorityContractID: StableAuthorityContractID(facts.WorkspaceID, facts.RunID, transaction),
			Transaction:         transaction,
			Attempt:             0,
			StablePrefixHash:    "stable-prefix",
			ManifestHash:        "manifest-1",
			LogicalRequestID:    "request-0",
			State:               RuntimeCandidateAttemptStatePending,
		}},
	}
	if err := record.Validate(); err == nil || !strings.Contains(err.Error(), "terminal runtime state cannot retain a pending candidate attempt") {
		t.Fatalf("terminal runtime state with pending candidate attempt err=%v", err)
	}
}
