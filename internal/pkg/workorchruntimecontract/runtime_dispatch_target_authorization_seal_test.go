package workorchruntimecontract

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRuntimeDispatchTargetAuthorizationFreshLoadRejectsTamper(t *testing.T) {
	baseline := runtimeDispatchIntrinsicFixture(t, 0)
	if err := runtimeDispatchTargetAuthorizationFreshLoad(baseline); err != nil {
		t.Fatalf("valid sealed target authorization did not survive fresh load: %v", err)
	}

	for _, test := range []struct {
		name    string
		mutate  func(*RuntimeDispatchRecord)
		wantErr string
	}{
		{
			name: "missing manifest identity",
			mutate: func(dispatch *RuntimeDispatchRecord) {
				dispatch.CandidateManifestHash = ""
			},
			wantErr: "requires id, scope, authority and candidate hash",
		},
		{
			name: "manifest identity drift without authority rehash",
			mutate: func(dispatch *RuntimeDispatchRecord) {
				dispatch.CandidateManifestHash = "manifest-drifted"
			},
			wantErr: "dispatch authority hash mismatch",
		},
		{
			name: "missing per-operation snapshot",
			mutate: func(dispatch *RuntimeDispatchRecord) {
				dispatch.OperationTargetAuthorizations = nil
				dispatch.DispatchAuthorityHash = StableRuntimeDispatchAuthorityHash(*dispatch)
			},
			wantErr: "exactly one target authorization",
		},
		{
			name: "snapshot excluded sealed target with coordinated authority rehash",
			mutate: func(dispatch *RuntimeDispatchRecord) {
				dispatch.OperationTargetAuthorizations[0].Target.ExactTargetRefs = []string{"other.txt"}
				dispatch.DispatchAuthorityHash = StableRuntimeDispatchAuthorityHash(*dispatch)
			},
			wantErr: "target_outside_sealed_set",
		},
		{
			name: "snapshot expansion without authority rehash",
			mutate: func(dispatch *RuntimeDispatchRecord) {
				dispatch.OperationTargetAuthorizations[0].Target.ExactTargetRefs = []string{"other.txt", "target.txt"}
			},
			wantErr: "dispatch authority hash mismatch",
		},
		{
			name: "operation identity drift",
			mutate: func(dispatch *RuntimeDispatchRecord) {
				dispatch.OperationTargetAuthorizations[0].OperationHash = "operation-hash-drift"
				dispatch.DispatchAuthorityHash = StableRuntimeDispatchAuthorityHash(*dispatch)
			},
			wantErr: "target authorization hash mismatch",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			dispatch := baseline.Clone()
			test.mutate(&dispatch)
			err := runtimeDispatchTargetAuthorizationFreshLoad(dispatch)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("fresh-load error = %v, want containing %q", err, test.wantErr)
			}
		})
	}
}

func TestRuntimeDispatchCoverageBindingSnapshotFreshLoadRejectsTamper(t *testing.T) {
	baseline := runtimeManagedValidationDispatchIntrinsicFixture(t)
	if err := runtimeDispatchTargetAuthorizationFreshLoad(baseline); err != nil {
		t.Fatalf("valid sealed coverage binding did not survive fresh load: %v", err)
	}

	for _, test := range []struct {
		name    string
		mutate  func(*RuntimeDispatchRecord)
		wantErr string
	}{
		{
			name: "missing snapshot hash",
			mutate: func(dispatch *RuntimeDispatchRecord) {
				dispatch.CoverageBindingHash = ""
			},
			wantErr: "coverage-binding hash",
		},
		{
			name: "binding body drift",
			mutate: func(dispatch *RuntimeDispatchRecord) {
				dispatch.CoverageBindings[0].DeclarationRef = "test/drifted"
			},
			wantErr: "coverage-binding projection",
		},
		{
			name: "binding snapshot hash drift",
			mutate: func(dispatch *RuntimeDispatchRecord) {
				dispatch.CoverageBindingHash = StableRuntimeCoverageEvidenceBindingSnapshotHash(nil)
				dispatch.DispatchAuthorityHash = StableRuntimeDispatchAuthorityHash(*dispatch)
			},
			wantErr: "coverage-binding hash mismatch",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			dispatch := baseline.Clone()
			test.mutate(&dispatch)
			err := runtimeDispatchTargetAuthorizationFreshLoad(dispatch)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("fresh-load error = %v, want containing %q", err, test.wantErr)
			}
		})
	}
}

func TestRuntimeDispatchAdmissionLineageBindsCandidateManifestHash(t *testing.T) {
	dispatch := runtimeDispatchIntrinsicFixture(t, 0)
	attempt := RuntimeCandidateAttemptRecord{
		Transaction: dispatch.Transaction, State: RuntimeCandidateAttemptStateAccepted,
		ManifestHash: dispatch.CandidateManifestHash, DispatchID: dispatch.ID,
		CandidateHash: dispatch.CandidateHash,
		AdmissionTargetAuthorizations: CloneRuntimeDispatchOperationTargetAuthorizations(
			dispatch.OperationTargetAuthorizations,
		),
		TargetAuthorizationHash: StableRuntimeDispatchOperationTargetAuthorizationsHash(
			dispatch.OperationTargetAuthorizations,
		),
		AdmissionCoverageBindings:    CloneCoverageEvidenceBindings(dispatch.CoverageBindings),
		AdmissionCoverageBindingHash: StableRuntimeCoverageEvidenceBindingSnapshotHash(dispatch.CoverageBindings),
		DispatchAuthorityHash:        dispatch.DispatchAuthorityHash,
	}
	if err := ValidateRuntimeDispatchAdmissionLineage(&dispatch, nil, []RuntimeCandidateAttemptRecord{attempt}, nil); err != nil {
		t.Fatalf("matching manifest lineage rejected: %v", err)
	}

	drifted := dispatch.Clone()
	drifted.CandidateManifestHash = "manifest-drifted"
	drifted.DispatchAuthorityHash = StableRuntimeDispatchAuthorityHash(drifted)
	attempt.DispatchAuthorityHash = drifted.DispatchAuthorityHash
	if err := ValidateRuntimeDispatchAdmissionLineage(&drifted, nil, []RuntimeCandidateAttemptRecord{attempt}, nil); err == nil || !strings.Contains(err.Error(), "exactly one accepted-attempt") {
		t.Fatalf("manifest-drifted admission lineage error = %v", err)
	}

	expanded := dispatch.Clone()
	expanded.OperationTargetAuthorizations[0].Target.Policy = RuntimeOperationTargetPolicyExistingOwnerScope
	expanded.OperationTargetAuthorizations[0].Target.ExactTargetRefs = nil
	expanded.OperationTargetAuthorizations[0].Target.OwnerScopeTargetRefs = []string{"other.txt", "target.txt"}
	expanded.DispatchAuthorityHash = StableRuntimeDispatchAuthorityHash(expanded)
	attempt.ManifestHash = expanded.CandidateManifestHash
	attempt.TargetAuthorizationHash = StableRuntimeDispatchOperationTargetAuthorizationsHash(expanded.OperationTargetAuthorizations)
	attempt.DispatchAuthorityHash = expanded.DispatchAuthorityHash
	if err := ValidateRuntimeDispatchAdmissionLineage(&expanded, nil, []RuntimeCandidateAttemptRecord{attempt}, nil); err == nil || !strings.Contains(err.Error(), "exactly one accepted-attempt") {
		t.Fatalf("coordinated target-authorization rehash lineage error = %v", err)
	}
}

func TestRuntimeDispatchAdmissionLineageBindsIndependentManagedValidationRecipe(t *testing.T) {
	dispatch := runtimeManagedValidationDispatchIntrinsicFixture(t)
	attempt := RuntimeCandidateAttemptRecord{
		Transaction: dispatch.Transaction, State: RuntimeCandidateAttemptStateAccepted,
		ManifestHash: dispatch.CandidateManifestHash, DispatchID: dispatch.ID,
		CandidateHash: dispatch.CandidateHash,
		AdmissionTargetAuthorizations: CloneRuntimeDispatchOperationTargetAuthorizations(
			dispatch.OperationTargetAuthorizations,
		),
		TargetAuthorizationHash: StableRuntimeDispatchOperationTargetAuthorizationsHash(
			dispatch.OperationTargetAuthorizations,
		),
		AdmissionCoverageBindings:    CloneCoverageEvidenceBindings(dispatch.CoverageBindings),
		AdmissionCoverageBindingHash: StableRuntimeCoverageEvidenceBindingSnapshotHash(dispatch.CoverageBindings),
		AdmissionValidationRecipes:   CloneRuntimeValidationRecipes(dispatch.ValidationRecipes),
		ValidationRecipeHash:         StableRuntimeValidationRecipeSnapshotHash(dispatch.ValidationRecipes),
		DispatchAuthorityHash:        dispatch.DispatchAuthorityHash,
	}
	if err := ValidateRuntimeDispatchAdmissionLineage(&dispatch, nil, []RuntimeCandidateAttemptRecord{attempt}, nil); err != nil {
		t.Fatalf("matching managed-validation admission lineage rejected: %v", err)
	}

	driftedCoverage := dispatch.Clone()
	driftedCoverage.CoverageBindings[0].DeclarationRef = "test/coordinated-dispatch-rewrite"
	driftedCoverage.CoverageBindings = NormalizeCoverageEvidenceBindings(driftedCoverage.CoverageBindings)
	driftedCoverage.CoverageBindingHash = StableRuntimeCoverageEvidenceBindingSnapshotHash(driftedCoverage.CoverageBindings)
	driftedCoverage.DispatchAuthorityHash = StableRuntimeDispatchAuthorityHash(driftedCoverage)
	coordinatedDispatchSeal := attempt.Clone()
	coordinatedDispatchSeal.DispatchAuthorityHash = driftedCoverage.DispatchAuthorityHash
	if err := ValidateRuntimeDispatchAdmissionLineage(&driftedCoverage, nil, []RuntimeCandidateAttemptRecord{coordinatedDispatchSeal}, nil); err == nil ||
		!strings.Contains(err.Error(), "exactly one accepted-attempt") {
		t.Fatalf("coordinated dispatch-only coverage-binding rewrite lineage error = %v", err)
	}

	missingAdmissionProjection := attempt.Clone()
	missingAdmissionProjection.AdmissionValidationRecipes = nil
	missingAdmissionProjection.ValidationRecipeHash = ""
	if err := ValidateRuntimeDispatchAdmissionLineage(&dispatch, nil, []RuntimeCandidateAttemptRecord{missingAdmissionProjection}, nil); err == nil ||
		!strings.Contains(err.Error(), "exactly one accepted-attempt") {
		t.Fatalf("missing validation-recipe admission projection lineage error = %v", err)
	}

	drifted := dispatch.Clone()
	drifted.ValidationRecipes[0].Args = []string{"-c", "echo coordinated-rewrite"}
	drifted.DispatchAuthorityHash = StableRuntimeDispatchAuthorityHash(drifted)
	// Reproduce the coordinated rewrite that the independent admission
	// projection must defeat: the dispatch self-seal and the attempt's copied
	// dispatch seal move together, but the manifest-derived recipe commitment
	// remains unchanged.
	attempt.DispatchAuthorityHash = drifted.DispatchAuthorityHash
	if err := ValidateRuntimeDispatchAdmissionLineage(&drifted, nil, []RuntimeCandidateAttemptRecord{attempt}, nil); err == nil ||
		!strings.Contains(err.Error(), "exactly one accepted-attempt") {
		t.Fatalf("coordinated validation-recipe rewrite lineage error = %v", err)
	}
}

func TestRuntimeGovernanceResumeAdmissionInheritsManagedValidationRecipePerHop(t *testing.T) {
	source := runtimeManagedValidationDispatchIntrinsicFixture(t)
	source.State = RuntimeDispatchStateCommitted
	firstResume := source.Clone()
	firstResume.ID = "resume-1"
	firstResume.State = RuntimeDispatchStateCommitted
	secondResume := firstResume.Clone()
	secondResume.ID = "resume-2"
	secondResume.State = RuntimeDispatchStateSealed
	dispatches := []RuntimeDispatchRecord{source, firstResume, secondResume}
	firstBinding := RuntimeOperationGovernanceBinding{
		OperationDispatchID: source.ID, State: RuntimeOperationGovernanceBindingGranted,
		Grant: &RuntimeOperationGovernanceGrant{ResumeDispatchID: firstResume.ID},
	}
	secondBinding := RuntimeOperationGovernanceBinding{
		OperationDispatchID: firstResume.ID, State: RuntimeOperationGovernanceBindingGranted,
		Grant: &RuntimeOperationGovernanceGrant{ResumeDispatchID: secondResume.ID},
	}
	attempt := RuntimeCandidateAttemptRecord{
		Transaction: source.Transaction, State: RuntimeCandidateAttemptStateAccepted,
		ManifestHash: source.CandidateManifestHash, DispatchID: source.ID,
		CandidateHash: source.CandidateHash,
		AdmissionTargetAuthorizations: CloneRuntimeDispatchOperationTargetAuthorizations(
			source.OperationTargetAuthorizations,
		),
		TargetAuthorizationHash: StableRuntimeDispatchOperationTargetAuthorizationsHash(
			source.OperationTargetAuthorizations,
		),
		AdmissionCoverageBindings:    CloneCoverageEvidenceBindings(source.CoverageBindings),
		AdmissionCoverageBindingHash: StableRuntimeCoverageEvidenceBindingSnapshotHash(source.CoverageBindings),
		AdmissionValidationRecipes:   CloneRuntimeValidationRecipes(source.ValidationRecipes),
		ValidationRecipeHash:         StableRuntimeValidationRecipeSnapshotHash(source.ValidationRecipes),
		DispatchAuthorityHash:        source.DispatchAuthorityHash,
	}

	if !runtimeGovernanceResumeAdmissionInheritsAuthorityCommitments(firstBinding, firstResume, dispatches) {
		t.Fatal("first governance resume did not inherit the source recipe commitment")
	}
	if !runtimeGovernanceResumeAdmissionInheritsAuthorityCommitments(secondBinding, secondResume, dispatches) {
		t.Fatal("second governance resume did not inherit the immediate recipe commitment")
	}
	if err := ValidateRuntimeDispatchAdmissionLineage(
		&secondResume,
		[]RuntimeDispatchRecord{source, firstResume},
		[]RuntimeCandidateAttemptRecord{attempt},
		[]RuntimeOperationGovernanceBinding{firstBinding, secondBinding},
	); err != nil {
		t.Fatalf("exact two-hop governance admission lineage rejected: %v", err)
	}

	for _, test := range []struct {
		name   string
		mutate func(*RuntimeValidationRecipe)
	}{
		{name: "command", mutate: func(recipe *RuntimeValidationRecipe) { recipe.Command = "bash" }},
		{name: "ordered args", mutate: func(recipe *RuntimeValidationRecipe) { recipe.Args = []string{"-c", "false"} }},
		{name: "timeout", mutate: func(recipe *RuntimeValidationRecipe) { recipe.TimeoutMillis = 9000 }},
	} {
		t.Run(test.name, func(t *testing.T) {
			drifted := secondResume.Clone()
			test.mutate(&drifted.ValidationRecipes[0])
			if runtimeGovernanceResumeAdmissionInheritsAuthorityCommitments(secondBinding, drifted, []RuntimeDispatchRecord{source, firstResume, drifted}) {
				t.Fatal("governance resume inherited a drifted managed-validation recipe")
			}
			if err := ValidateRuntimeDispatchAdmissionLineage(
				&drifted,
				[]RuntimeDispatchRecord{source, firstResume},
				[]RuntimeCandidateAttemptRecord{attempt},
				[]RuntimeOperationGovernanceBinding{firstBinding, secondBinding},
			); err == nil || !strings.Contains(err.Error(), "governance-resume admission") {
				t.Fatalf("drifted two-hop governance admission lineage error = %v", err)
			}
		})
	}
}

func TestRuntimeDispatchTargetAuthorizationRequiresOperationBijectionOnlyForOperationCandidates(t *testing.T) {
	first := RuntimeOperation{ID: "op-1", Kind: StepKindFileWrite, TargetRefs: []string{"one.txt"}}
	second := RuntimeOperation{ID: "op-2", Kind: StepKindFileWrite, TargetRefs: []string{"two.txt"}}
	candidate := RuntimeCandidate{StateChange: &StateChangeCandidate{Operations: []StateChangingOperation{{Operation: first}, {Operation: second}}}}
	authorizations := []RuntimeDispatchOperationTargetAuthorization{
		{OperationID: first.ID, OperationHash: StableRuntimeOperationHash(first), RequireTargets: true, Target: RuntimeOperationTargetAuthorization{Policy: RuntimeOperationTargetPolicyExactTarget, ExactTargetRefs: []string{"one.txt", "two.txt"}}},
		{OperationID: second.ID, OperationHash: StableRuntimeOperationHash(second), RequireTargets: true, Target: RuntimeOperationTargetAuthorization{Policy: RuntimeOperationTargetPolicyExactTarget, ExactTargetRefs: []string{"one.txt", "two.txt"}}},
	}
	if err := ValidateRuntimeDispatchOperationTargetAuthorizations(candidate, authorizations); err != nil {
		t.Fatalf("one target snapshot per operation rejected: %v", err)
	}
	duplicate := CloneRuntimeDispatchOperationTargetAuthorizations(authorizations)
	duplicate[1] = duplicate[0]
	if err := ValidateRuntimeDispatchOperationTargetAuthorizations(candidate, duplicate); err == nil || !strings.Contains(err.Error(), "duplicated") {
		t.Fatalf("duplicate target snapshot error = %v", err)
	}

	for name, candidate := range map[string]RuntimeCandidate{
		"assessment": {Assessment: &AssessmentCandidate{}},
		"governance": {Governance: &GovernanceCandidate{}},
	} {
		t.Run(name, func(t *testing.T) {
			if err := ValidateRuntimeDispatchOperationTargetAuthorizations(candidate, nil); err != nil {
				t.Fatalf("non-operation candidate with empty target snapshots: %v", err)
			}
			if err := ValidateRuntimeDispatchOperationTargetAuthorizations(candidate, authorizations[:1]); err == nil || !strings.Contains(err.Error(), "cannot carry") {
				t.Fatalf("non-operation candidate target snapshot error = %v", err)
			}
		})
	}

	hybrid := RuntimeCandidate{
		StateChange: &StateChangeCandidate{Operations: []StateChangingOperation{{Operation: first}}},
		Assessment:  &AssessmentCandidate{},
	}
	if err := ValidateRuntimeDispatchOperationTargetAuthorizations(hybrid, authorizations[:1]); err == nil || !strings.Contains(err.Error(), "exactly one candidate kind") {
		t.Fatalf("hybrid candidate target authorization error = %v", err)
	}
}

func TestRuntimeDispatchOperationTargetAuthorizationForOperationRequiresExactSealedOperation(t *testing.T) {
	operation := RuntimeOperation{ID: "validate-1", Kind: RuntimeBlockKindManagedValidationSession, TargetRefs: []string{"entry_test.go"}}
	candidate := RuntimeCandidate{Validation: &ValidationCandidate{Operations: []ValidationOperation{{Operation: operation}}}}
	authorizations := []RuntimeDispatchOperationTargetAuthorization{{
		OperationID: operation.ID, OperationHash: StableRuntimeOperationHash(operation), RequireTargets: true,
		Target: RuntimeOperationTargetAuthorization{
			Policy:               RuntimeOperationTargetPolicyExistingOwnerScope,
			OwnerScopeTargetRefs: []string{"entry_test.go", "implementation.go"},
		},
	}}

	resolved, err := RuntimeDispatchOperationTargetAuthorizationForOperation(candidate, authorizations, operation)
	if err != nil {
		t.Fatalf("resolve exact sealed operation target authorization: %v", err)
	}
	if got := resolved.Target.AllowedTargetRefs(); len(got) != 2 || got[0] != "entry_test.go" || got[1] != "implementation.go" {
		t.Fatalf("resolved sealed validation scope = %v", got)
	}

	drifted := operation
	drifted.TargetRefs = []string{"implementation.go"}
	if _, err := RuntimeDispatchOperationTargetAuthorizationForOperation(candidate, authorizations, drifted); err == nil || !strings.Contains(err.Error(), "does not match sealed candidate operation") {
		t.Fatalf("operation payload drift lookup error = %v", err)
	}
	missing := operation
	missing.ID = "validate-other"
	if _, err := RuntimeDispatchOperationTargetAuthorizationForOperation(candidate, authorizations, missing); err == nil || !strings.Contains(err.Error(), "not present in sealed candidate") {
		t.Fatalf("missing operation lookup error = %v", err)
	}
	tampered := CloneRuntimeDispatchOperationTargetAuthorizations(authorizations)
	tampered[0].OperationHash = "operation-hash-drift"
	if _, err := RuntimeDispatchOperationTargetAuthorizationForOperation(candidate, tampered, operation); err == nil || !strings.Contains(err.Error(), "target authorization hash mismatch") {
		t.Fatalf("tampered operation authorization lookup error = %v", err)
	}
	for name, mutate := range map[string]func(*RuntimeDispatchOperationTargetAuthorization){
		"empty operation id":   func(value *RuntimeDispatchOperationTargetAuthorization) { value.OperationID = "" },
		"empty operation hash": func(value *RuntimeDispatchOperationTargetAuthorization) { value.OperationHash = "" },
	} {
		t.Run(name, func(t *testing.T) {
			invalid := CloneRuntimeDispatchOperationTargetAuthorizations(authorizations)
			mutate(&invalid[0])
			if _, err := RuntimeDispatchOperationTargetAuthorizationForOperation(candidate, invalid, operation); err == nil ||
				!strings.Contains(err.Error(), "requires operation id and hash") {
				t.Fatalf("invalid operation authorization lookup error = %v", err)
			}
		})
	}
}

func runtimeDispatchTargetAuthorizationFreshLoad(dispatch RuntimeDispatchRecord) error {
	record := RuntimeStateRecord{ActiveDispatch: &dispatch}
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	var loaded RuntimeStateRecord
	if err := json.Unmarshal(data, &loaded); err != nil {
		return err
	}
	return ValidateRuntimeDispatchHistoryShape(dispatch.WorkspaceID, dispatch.RunID, loaded.ActiveDispatch, nil)
}
